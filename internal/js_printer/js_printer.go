package js_printer

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/renamer"
	"github.com/evanw/esbuild/internal/sourcemap"
)

var positiveInfinity = math.Inf(1)
var negativeInfinity = math.Inf(-1)

const hexChars = "0123456789ABCDEF"
const firstASCII = 0x20
const lastASCII = 0x7E
const firstHighSurrogate = 0xD800
const lastHighSurrogate = 0xDBFF
const firstLowSurrogate = 0xDC00
const lastLowSurrogate = 0xDFFF

func canPrintWithoutEscape(c rune, asciiOnly bool) bool {
	if c <= lastASCII {
		return c >= firstASCII && c != '\\' && c != '"'
	} else {
		return !asciiOnly && c != '\uFEFF' && (c < firstHighSurrogate || c > lastLowSurrogate)
	}
}

func QuoteForJSON(text string, asciiOnly bool) []byte {
	// Estimate the required length
	lenEstimate := 2
	for _, c := range text {
		if canPrintWithoutEscape(c, asciiOnly) {
			lenEstimate += utf8.RuneLen(c)
		} else {
			switch c {
			case '\b', '\f', '\n', '\r', '\t', '\\', '"':
				lenEstimate += 2
			default:
				if c <= 0xFFFF {
					lenEstimate += 6
				} else {
					lenEstimate += 12
				}
			}
		}
	}

	// Preallocate the array
	bytes := make([]byte, 0, lenEstimate)
	i := 0
	n := len(text)
	bytes = append(bytes, '"')

	for i < n {
		c, width := helpers.DecodeWTF8Rune(text[i:])

		// Fast path: a run of characters that don't need escaping
		if canPrintWithoutEscape(c, asciiOnly) {
			start := i
			i += width
			for i < n {
				c, width = helpers.DecodeWTF8Rune(text[i:])
				if !canPrintWithoutEscape(c, asciiOnly) {
					break
				}
				i += width
			}
			bytes = append(bytes, text[start:i]...)
			continue
		}

		switch c {
		case '\b':
			bytes = append(bytes, "\\b"...)
			i++

		case '\f':
			bytes = append(bytes, "\\f"...)
			i++

		case '\n':
			bytes = append(bytes, "\\n"...)
			i++

		case '\r':
			bytes = append(bytes, "\\r"...)
			i++

		case '\t':
			bytes = append(bytes, "\\t"...)
			i++

		case '\\':
			bytes = append(bytes, "\\\\"...)
			i++

		case '"':
			bytes = append(bytes, "\\\""...)
			i++

		default:
			i += width
			if c <= 0xFFFF {
				bytes = append(
					bytes,
					'\\', 'u', hexChars[c>>12], hexChars[(c>>8)&15], hexChars[(c>>4)&15], hexChars[c&15],
				)
			} else {
				c -= 0x10000
				lo := firstHighSurrogate + ((c >> 10) & 0x3FF)
				hi := firstLowSurrogate + (c & 0x3FF)
				bytes = append(
					bytes,
					'\\', 'u', hexChars[lo>>12], hexChars[(lo>>8)&15], hexChars[(lo>>4)&15], hexChars[lo&15],
					'\\', 'u', hexChars[hi>>12], hexChars[(hi>>8)&15], hexChars[(hi>>4)&15], hexChars[hi&15],
				)
			}
		}
	}

	return append(bytes, '"')
}

func QuoteIdentifier(js []byte, name string, unsupportedFeatures compat.JSFeature) []byte {
	isASCII := false
	asciiStart := 0
	for i, c := range name {
		if c >= firstASCII && c <= lastASCII {
			// Fast path: a run of ASCII characters
			if !isASCII {
				isASCII = true
				asciiStart = i
			}
		} else {
			// Slow path: escape non-ACSII characters
			if isASCII {
				js = append(js, name[asciiStart:i]...)
				isASCII = false
			}
			if c <= 0xFFFF {
				js = append(js, '\\', 'u', hexChars[c>>12], hexChars[(c>>8)&15], hexChars[(c>>4)&15], hexChars[c&15])
			} else if !unsupportedFeatures.Has(compat.UnicodeEscapes) {
				js = append(js, fmt.Sprintf("\\u{%X}", c)...)
			} else {
				panic("Internal error: Cannot encode identifier: Unicode escapes are unsupported")
			}
		}
	}
	if isASCII {
		// Print one final run of ASCII characters
		js = append(js, name[asciiStart:]...)
	}
	return js
}

func (p *printer) printUnquotedUTF16(text []uint16, quote rune) {
	temp := make([]byte, utf8.UTFMax)
	js := p.js
	i := 0
	n := len(text)

	for i < n {
		c := text[i]
		i++

		switch c {
		// Special-case the null character since it may mess with code written in C
		// that treats null characters as the end of the string.
		case '\x00':
			// We don't want "\x001" to be written as "\01"
			if i < n && text[i] >= '0' && text[i] <= '9' {
				js = append(js, "\\x00"...)
			} else {
				js = append(js, "\\0"...)
			}

		// Special-case the bell character since it may cause dumping this file to
		// the terminal to make a sound, which is undesirable. Note that we can't
		// use an octal literal to print this shorter since octal literals are not
		// allowed in strict mode (or in template strings).
		case '\x07':
			js = append(js, "\\x07"...)

		case '\b':
			js = append(js, "\\b"...)

		case '\f':
			js = append(js, "\\f"...)

		case '\n':
			if quote == '`' {
				js = append(js, '\n')
			} else {
				js = append(js, "\\n"...)
			}

		case '\r':
			js = append(js, "\\r"...)

		case '\v':
			js = append(js, "\\v"...)

		case '\x1B':
			js = append(js, "\\x1B"...)

		case '\\':
			js = append(js, "\\\\"...)

		case '/':
			// Avoid generating the sequence "</script" in JS code
			if i >= 2 && text[i-2] == '<' && i+6 <= len(text) {
				script := "script"
				matches := true
				for j := 0; j < 6; j++ {
					a := text[i+j]
					b := uint16(script[j])
					if a >= 'A' && a <= 'Z' {
						a += 'a' - 'A'
					}
					if a != b {
						matches = false
						break
					}
				}
				if matches {
					js = append(js, '\\')
				}
			}
			js = append(js, '/')

		case '\'':
			if quote == '\'' {
				js = append(js, '\\')
			}
			js = append(js, '\'')

		case '"':
			if quote == '"' {
				js = append(js, '\\')
			}
			js = append(js, '"')

		case '`':
			if quote == '`' {
				js = append(js, '\\')
			}
			js = append(js, '`')

		case '$':
			if quote == '`' && i < n && text[i] == '{' {
				js = append(js, '\\')
			}
			js = append(js, '$')

		case '\u2028':
			js = append(js, "\\u2028"...)

		case '\u2029':
			js = append(js, "\\u2029"...)

		case '\uFEFF':
			js = append(js, "\\uFEFF"...)

		default:
			switch {
			// Common case: just append a single byte
			case c <= lastASCII:
				js = append(js, byte(c))

			// Is this a high surrogate?
			case c >= firstHighSurrogate && c <= lastHighSurrogate:
				// Is there a next character?
				if i < n {
					c2 := text[i]

					// Is it a low surrogate?
					if c2 >= firstLowSurrogate && c2 <= lastLowSurrogate {
						r := (rune(c) << 10) + rune(c2) + (0x10000 - (firstHighSurrogate << 10) - firstLowSurrogate)
						i++

						// Escape this character if UTF-8 isn't allowed
						if p.options.ASCIIOnly {
							if !p.options.UnsupportedFeatures.Has(compat.UnicodeEscapes) {
								js = append(js, fmt.Sprintf("\\u{%X}", r)...)
							} else {
								js = append(js,
									'\\', 'u', hexChars[c>>12], hexChars[(c>>8)&15], hexChars[(c>>4)&15], hexChars[c&15],
									'\\', 'u', hexChars[c2>>12], hexChars[(c2>>8)&15], hexChars[(c2>>4)&15], hexChars[c2&15],
								)
							}
							continue
						}

						// Otherwise, encode to UTF-8
						width := utf8.EncodeRune(temp, r)
						js = append(js, temp[:width]...)
						continue
					}
				}

				// Write an unpaired high surrogate
				js = append(js, '\\', 'u', hexChars[c>>12], hexChars[(c>>8)&15], hexChars[(c>>4)&15], hexChars[c&15])

			// Is this an unpaired low surrogate or four-digit hex escape?
			case (c >= firstLowSurrogate && c <= lastLowSurrogate) || (p.options.ASCIIOnly && c > 0xFF):
				js = append(js, '\\', 'u', hexChars[c>>12], hexChars[(c>>8)&15], hexChars[(c>>4)&15], hexChars[c&15])

			// Can this be a two-digit hex escape?
			case p.options.ASCIIOnly:
				js = append(js, '\\', 'x', hexChars[c>>4], hexChars[c&15])

			// Otherwise, just encode to UTF-8
			default:
				width := utf8.EncodeRune(temp, rune(c))
				js = append(js, temp[:width]...)
			}
		}
	}

	p.js = js
}

// Use JS strings for JSX attributes that need escape characters. Technically
// the JSX specification doesn't say anything about using XML character escape
// sequences, so JSX implementations may not be able to consume them. See
// https://facebook.github.io/jsx/ for the specification.
func (p *printer) canPrintTextAsJSXAttribute(text []uint16) (quote string, ok bool) {
	single := true
	double := true

	for _, c := range text {
		// Use JS strings for control characters
		if c < firstASCII {
			return "", false
		}

		// Use JS strings if we need to escape non-ASCII characters
		if p.options.ASCIIOnly && c > lastASCII {
			return "", false
		}

		switch c {
		case '&':
			// Use JS strings if the text would need to be escaped with "&amp;"
			return "", false

		case '"':
			double = false
			if !single {
				break
			}

		case '\'':
			single = false
			if !double {
				break
			}
		}
	}

	// Prefer duble quotes to single quotes
	if double {
		return "\"", true
	}
	if single {
		return "'", true
	}
	return "", false
}

// Use JS strings for text inside JSX elements that need escape characters.
// Technically the JSX specification doesn't say anything about using XML
// character escape sequences, so JSX implementations may not be able to
// consume them. See https://facebook.github.io/jsx/ for the specification.
func (p *printer) canPrintTextAsJSXChild(text []uint16) bool {
	for _, c := range text {
		// Use JS strings for control characters
		if c < firstASCII {
			return false
		}

		// Use JS strings if we need to escape non-ASCII characters
		if p.options.ASCIIOnly && c > lastASCII {
			return false
		}

		switch c {
		case '&', '<', '>', '{', '}':
			// Use JS strings if the text would need to be escaped
			return false
		}
	}

	return true
}

// JSX tag syntax doesn't support character escapes so non-ASCII identifiers
// must be printed as UTF-8 even when the charset is set to ASCII.
func (p *printer) printJSXTag(tagOrNil js_ast.Expr) {
	switch e := tagOrNil.Data.(type) {
	case *js_ast.EString:
		p.addSourceMapping(tagOrNil.Loc)
		p.print(helpers.UTF16ToString(e.Value))

	case *js_ast.EIdentifier:
		name := p.renamer.NameForSymbol(e.Ref)
		p.addSourceMapping(tagOrNil.Loc)
		p.print(name)

	case *js_ast.EDot:
		p.printJSXTag(e.Target)
		p.print(".")
		p.addSourceMapping(e.NameLoc)
		p.print(e.Name)

	default:
		if tagOrNil.Data != nil {
			p.printExpr(tagOrNil, js_ast.LLowest, 0)
		}
	}
}

type printer struct {
	symbols                js_ast.SymbolMap
	isUnbound              func(js_ast.Ref) bool
	renamer                renamer.Renamer
	importRecords          []ast.ImportRecord
	callTarget             js_ast.E
	extractedLegalComments map[string]bool
	js                     []byte
	options                Options
	builder                sourcemap.ChunkBuilder
	stmtStart              int
	exportDefaultStart     int
	arrowExprStart         int
	forOfInitStart         int
	prevOpEnd              int
	prevNumEnd             int
	prevRegExpEnd          int
	intToBytesBuffer       [64]byte
	needsSemicolon         bool
	prevOp                 js_ast.OpCode
	moduleType             js_ast.ModuleType
}

func (p *printer) print(text string) {
	p.js = append(p.js, text...)
}

// This is the same as "print(string(bytes))" without any unnecessary temporary
// allocations
func (p *printer) printBytes(bytes []byte) {
	p.js = append(p.js, bytes...)
}

func (p *printer) printQuotedUTF8(text string, allowBacktick bool) {
	p.printQuotedUTF16(helpers.StringToUTF16(text), allowBacktick)
}

func (p *printer) addSourceMapping(loc logger.Loc) {
	if p.options.AddSourceMappings {
		p.builder.AddSourceMapping(loc, p.js)
	}
}

func (p *printer) printIndent() {
	if !p.options.MinifyWhitespace {
		for i := 0; i < p.options.Indent; i++ {
			p.print("  ")
		}
	}
}

func (p *printer) printSymbol(ref js_ast.Ref) {
	name := p.renamer.NameForSymbol(ref)

	// Minify "return #foo in bar" to "return#foo in bar"
	if !strings.HasPrefix(name, "#") {
		p.printSpaceBeforeIdentifier()
	}

	p.printIdentifier(name)
}

func (p *printer) printClauseAlias(alias string) {
	if js_lexer.IsIdentifier(alias) {
		p.printSpaceBeforeIdentifier()
		p.printIdentifier(alias)
	} else {
		p.printQuotedUTF8(alias, false /* allowBacktick */)
	}
}

// Note: The functions below check whether something can be printed as an
// identifier or if it needs to be quoted (e.g. "x.y" vs. "x['y']") using the
// ES5 identifier validity test to maximize cross-platform portability. Even
// though newer JavaScript environments can handle more Unicode characters,
// there isn't a published document that says which Unicode versions are
// supported by which browsers. Even if a character is considered valid in the
// latest version of Unicode, we don't know if the browser we're targeting
// contains an older version of Unicode or not. So for safety, we quote
// anything that isn't guaranteed to be compatible with ES5, the oldest
// JavaScript language target that we support.

func CanEscapeIdentifier(name string, unsupportedJSFeatures compat.JSFeature, asciiOnly bool) bool {
	return js_lexer.IsIdentifierES5AndESNext(name) && (!asciiOnly ||
		!unsupportedJSFeatures.Has(compat.UnicodeEscapes) ||
		!helpers.ContainsNonBMPCodePoint(name))
}

func (p *printer) canPrintIdentifier(name string) bool {
	return js_lexer.IsIdentifierES5AndESNext(name) && (!p.options.ASCIIOnly ||
		!p.options.UnsupportedFeatures.Has(compat.UnicodeEscapes) ||
		!helpers.ContainsNonBMPCodePoint(name))
}

func (p *printer) canPrintIdentifierUTF16(name []uint16) bool {
	return js_lexer.IsIdentifierES5AndESNextUTF16(name) && (!p.options.ASCIIOnly ||
		!p.options.UnsupportedFeatures.Has(compat.UnicodeEscapes) ||
		!helpers.ContainsNonBMPCodePointUTF16(name))
}

func (p *printer) printIdentifier(name string) {
	if p.options.ASCIIOnly {
		p.js = QuoteIdentifier(p.js, name, p.options.UnsupportedFeatures)
	} else {
		p.print(name)
	}
}

// This is the same as "printIdentifier(StringToUTF16(bytes))" without any
// unnecessary temporary allocations
func (p *printer) printIdentifierUTF16(name []uint16) {
	var temp [utf8.UTFMax]byte
	n := len(name)

	for i := 0; i < n; i++ {
		c := rune(name[i])

		if c >= firstHighSurrogate && c <= lastHighSurrogate && i+1 < n {
			if c2 := rune(name[i+1]); c2 >= firstLowSurrogate && c2 <= lastLowSurrogate {
				c = (c << 10) + c2 + (0x10000 - (firstHighSurrogate << 10) - firstLowSurrogate)
				i++
			}
		}

		if p.options.ASCIIOnly && c > lastASCII {
			if c <= 0xFFFF {
				p.js = append(p.js, '\\', 'u', hexChars[c>>12], hexChars[(c>>8)&15], hexChars[(c>>4)&15], hexChars[c&15])
			} else if !p.options.UnsupportedFeatures.Has(compat.UnicodeEscapes) {
				p.js = append(p.js, fmt.Sprintf("\\u{%X}", c)...)
			} else {
				panic("Internal error: Cannot encode identifier: Unicode escapes are unsupported")
			}
			continue
		}

		width := utf8.EncodeRune(temp[:], c)
		p.js = append(p.js, temp[:width]...)
	}
}

func (p *printer) printNumber(value float64, level js_ast.L) {
	absValue := math.Abs(value)

	if value != value {
		p.printSpaceBeforeIdentifier()
		p.print("NaN")
	} else if value == positiveInfinity || value == negativeInfinity {
		wrap := (p.options.MinifySyntax && level >= js_ast.LMultiply) ||
			(value == negativeInfinity && level >= js_ast.LPrefix)
		if wrap {
			p.print("(")
		}
		if value == negativeInfinity {
			p.printSpaceBeforeOperator(js_ast.UnOpNeg)
			p.print("-")
		} else {
			p.printSpaceBeforeIdentifier()
		}
		if !p.options.MinifySyntax {
			p.print("Infinity")
		} else if p.options.MinifyWhitespace {
			p.print("1/0")
		} else {
			p.print("1 / 0")
		}
		if wrap {
			p.print(")")
		}
	} else {
		if !math.Signbit(value) {
			p.printSpaceBeforeIdentifier()
			p.printNonNegativeFloat(absValue)

			// Remember the end of the latest number
			p.prevNumEnd = len(p.js)
		} else if level >= js_ast.LPrefix {
			// Expressions such as "(-1).toString" need to wrap negative numbers.
			// Instead of testing for "value < 0" we test for "signbit(value)" and
			// "!isNaN(value)" because we need this to be true for "-0" and "-0 < 0"
			// is false.
			p.print("(-")
			p.printNonNegativeFloat(absValue)
			p.print(")")
		} else {
			p.printSpaceBeforeOperator(js_ast.UnOpNeg)
			p.print("-")
			p.printNonNegativeFloat(absValue)

			// Remember the end of the latest number
			p.prevNumEnd = len(p.js)
		}
	}
}

func (p *printer) printBinding(binding js_ast.Binding) {
	p.addSourceMapping(binding.Loc)

	switch b := binding.Data.(type) {
	case *js_ast.BMissing:

	case *js_ast.BIdentifier:
		p.printSymbol(b.Ref)

	case *js_ast.BArray:
		p.print("[")
		if len(b.Items) > 0 {
			if !b.IsSingleLine {
				p.options.Indent++
			}

			for i, item := range b.Items {
				if i != 0 {
					p.print(",")
					if b.IsSingleLine {
						p.printSpace()
					}
				}
				if !b.IsSingleLine {
					p.printNewline()
					p.printIndent()
				}
				if b.HasSpread && i+1 == len(b.Items) {
					p.print("...")
				}
				p.printBinding(item.Binding)

				if item.DefaultValueOrNil.Data != nil {
					p.printSpace()
					p.print("=")
					p.printSpace()
					p.printExpr(item.DefaultValueOrNil, js_ast.LComma, 0)
				}

				// Make sure there's a comma after trailing missing items
				if _, ok := item.Binding.Data.(*js_ast.BMissing); ok && i == len(b.Items)-1 {
					p.print(",")
				}
			}

			if !b.IsSingleLine {
				p.options.Indent--
				p.printNewline()
				p.printIndent()
			}
		}
		p.print("]")

	case *js_ast.BObject:
		p.print("{")
		if len(b.Properties) > 0 {
			if !b.IsSingleLine {
				p.options.Indent++
			}

			for i, property := range b.Properties {
				if i != 0 {
					p.print(",")
				}
				if b.IsSingleLine {
					p.printSpace()
				} else {
					p.printNewline()
					p.printIndent()
				}

				if property.IsSpread {
					p.print("...")
				} else {
					if property.IsComputed {
						p.print("[")
						p.printExpr(property.Key, js_ast.LComma, 0)
						p.print("]:")
						p.printSpace()
						p.printBinding(property.Value)

						if property.DefaultValueOrNil.Data != nil {
							p.printSpace()
							p.print("=")
							p.printSpace()
							p.printExpr(property.DefaultValueOrNil, js_ast.LComma, 0)
						}
						continue
					}

					if str, ok := property.Key.Data.(*js_ast.EString); ok && !property.PreferQuotedKey && p.canPrintIdentifierUTF16(str.Value) {
						p.addSourceMapping(property.Key.Loc)
						p.printIdentifierUTF16(str.Value)

						// Use a shorthand property if the names are the same
						if id, ok := property.Value.Data.(*js_ast.BIdentifier); ok && helpers.UTF16EqualsString(str.Value, p.renamer.NameForSymbol(id.Ref)) {
							if property.DefaultValueOrNil.Data != nil {
								p.printSpace()
								p.print("=")
								p.printSpace()
								p.printExpr(property.DefaultValueOrNil, js_ast.LComma, 0)
							}
							continue
						}
					} else if mangled, ok := property.Key.Data.(*js_ast.EMangledProp); ok {
						p.addSourceMapping(property.Key.Loc)
						if name := p.renamer.NameForSymbol(mangled.Ref); p.canPrintIdentifier(name) {
							p.printIdentifier(name)

							// Use a shorthand property if the names are the same
							if id, ok := property.Value.Data.(*js_ast.BIdentifier); ok && name == p.renamer.NameForSymbol(id.Ref) {
								if property.DefaultValueOrNil.Data != nil {
									p.printSpace()
									p.print("=")
									p.printSpace()
									p.printExpr(property.DefaultValueOrNil, js_ast.LComma, 0)
								}
								continue
							}
						} else {
							p.printQuotedUTF8(name, false /* allowBacktick */)
						}
					} else {
						p.printExpr(property.Key, js_ast.LLowest, 0)
					}

					p.print(":")
					p.printSpace()
				}
				p.printBinding(property.Value)

				if property.DefaultValueOrNil.Data != nil {
					p.printSpace()
					p.print("=")
					p.printSpace()
					p.printExpr(property.DefaultValueOrNil, js_ast.LComma, 0)
				}
			}

			if !b.IsSingleLine {
				p.options.Indent--
				p.printNewline()
				p.printIndent()
			} else if len(b.Properties) > 0 {
				p.printSpace()
			}
		}
		p.print("}")

	default:
		panic(fmt.Sprintf("Unexpected binding of type %T", binding.Data))
	}
}

func (p *printer) printSpace() {
	if !p.options.MinifyWhitespace {
		p.print(" ")
	}
}

func (p *printer) printNewline() {
	if !p.options.MinifyWhitespace {
		p.print("\n")
	}
}

func (p *printer) printSpaceBeforeOperator(next js_ast.OpCode) {
	if p.prevOpEnd == len(p.js) {
		prev := p.prevOp

		// "+ + y" => "+ +y"
		// "+ ++ y" => "+ ++y"
		// "x + + y" => "x+ +y"
		// "x ++ + y" => "x+++y"
		// "x + ++ y" => "x+ ++y"
		// "-- >" => "-- >"
		// "< ! --" => "<! --"
		if ((prev == js_ast.BinOpAdd || prev == js_ast.UnOpPos) && (next == js_ast.BinOpAdd || next == js_ast.UnOpPos || next == js_ast.UnOpPreInc)) ||
			((prev == js_ast.BinOpSub || prev == js_ast.UnOpNeg) && (next == js_ast.BinOpSub || next == js_ast.UnOpNeg || next == js_ast.UnOpPreDec)) ||
			(prev == js_ast.UnOpPostDec && next == js_ast.BinOpGt) ||
			(prev == js_ast.UnOpNot && next == js_ast.UnOpPreDec && len(p.js) > 1 && p.js[len(p.js)-2] == '<') {
			p.print(" ")
		}
	}
}

func (p *printer) printSemicolonAfterStatement() {
	if !p.options.MinifyWhitespace {
		p.print(";\n")
	} else {
		p.needsSemicolon = true
	}
}

func (p *printer) printSemicolonIfNeeded() {
	if p.needsSemicolon {
		p.print(";")
		p.needsSemicolon = false
	}
}

func (p *printer) printSpaceBeforeIdentifier() {
	buffer := p.js
	n := len(buffer)
	if n > 0 && (js_lexer.IsIdentifierContinue(rune(buffer[n-1])) || n == p.prevRegExpEnd) {
		p.print(" ")
	}
}

func (p *printer) printFnArgs(args []js_ast.Arg, hasRestArg bool, isArrow bool) {
	wrap := true

	// Minify "(a) => {}" as "a=>{}"
	if p.options.MinifyWhitespace && !hasRestArg && isArrow && len(args) == 1 {
		if _, ok := args[0].Binding.Data.(*js_ast.BIdentifier); ok && args[0].DefaultOrNil.Data == nil {
			wrap = false
		}
	}

	if wrap {
		p.print("(")
	}

	for i, arg := range args {
		if i != 0 {
			p.print(",")
			p.printSpace()
		}
		if hasRestArg && i+1 == len(args) {
			p.print("...")
		}
		p.printBinding(arg.Binding)

		if arg.DefaultOrNil.Data != nil {
			p.printSpace()
			p.print("=")
			p.printSpace()
			p.printExpr(arg.DefaultOrNil, js_ast.LComma, 0)
		}
	}

	if wrap {
		p.print(")")
	}
}

func (p *printer) printFn(fn js_ast.Fn) {
	p.printFnArgs(fn.Args, fn.HasRestArg, false /* isArrow */)
	p.printSpace()
	p.printBlock(fn.Body.Loc, fn.Body.Stmts)
}

func (p *printer) printClass(class js_ast.Class) {
	if class.ExtendsOrNil.Data != nil {
		p.print(" extends")
		p.printSpace()
		p.printExpr(class.ExtendsOrNil, js_ast.LNew-1, 0)
	}
	p.printSpace()

	p.addSourceMapping(class.BodyLoc)
	p.print("{")
	p.printNewline()
	p.options.Indent++

	for _, item := range class.Properties {
		p.printSemicolonIfNeeded()
		p.printIndent()

		if item.Kind == js_ast.PropertyClassStaticBlock {
			p.print("static")
			p.printSpace()
			p.printBlock(item.ClassStaticBlock.Loc, item.ClassStaticBlock.Stmts)
			p.printNewline()
			continue
		}

		p.printProperty(item)

		// Need semicolons after class fields
		if item.ValueOrNil.Data == nil {
			p.printSemicolonAfterStatement()
		} else {
			p.printNewline()
		}
	}

	p.needsSemicolon = false
	p.options.Indent--
	p.printIndent()
	p.print("}")
}

func (p *printer) printProperty(item js_ast.Property) {
	if item.Kind == js_ast.PropertySpread {
		p.print("...")
		p.printExpr(item.ValueOrNil, js_ast.LComma, 0)
		return
	}

	if item.IsStatic {
		p.print("static")
		p.printSpace()
	}

	switch item.Kind {
	case js_ast.PropertyGet:
		p.printSpaceBeforeIdentifier()
		p.print("get")
		p.printSpace()

	case js_ast.PropertySet:
		p.printSpaceBeforeIdentifier()
		p.print("set")
		p.printSpace()
	}

	if fn, ok := item.ValueOrNil.Data.(*js_ast.EFunction); item.IsMethod && ok {
		if fn.Fn.IsAsync {
			p.printSpaceBeforeIdentifier()
			p.print("async")
			p.printSpace()
		}
		if fn.Fn.IsGenerator {
			p.print("*")
		}
	}

	if item.IsComputed {
		p.print("[")
		p.printExpr(item.Key, js_ast.LComma, 0)
		p.print("]")

		if item.ValueOrNil.Data != nil {
			if fn, ok := item.ValueOrNil.Data.(*js_ast.EFunction); item.IsMethod && ok {
				p.printFn(fn.Fn)
				return
			}

			p.print(":")
			p.printSpace()
			p.printExpr(item.ValueOrNil, js_ast.LComma, 0)
		}

		if item.InitializerOrNil.Data != nil {
			p.printSpace()
			p.print("=")
			p.printSpace()
			p.printExpr(item.InitializerOrNil, js_ast.LComma, 0)
		}
		return
	}

	switch key := item.Key.Data.(type) {
	case *js_ast.EPrivateIdentifier:
		p.addSourceMapping(item.Key.Loc)
		p.printSymbol(key.Ref)

	case *js_ast.EMangledProp:
		p.addSourceMapping(item.Key.Loc)
		if name := p.renamer.NameForSymbol(key.Ref); p.canPrintIdentifier(name) {
			p.printSpaceBeforeIdentifier()
			p.printIdentifier(name)

			// Use a shorthand property if the names are the same
			if !p.options.UnsupportedFeatures.Has(compat.ObjectExtensions) && item.ValueOrNil.Data != nil {
				switch e := item.ValueOrNil.Data.(type) {
				case *js_ast.EIdentifier:
					if name == p.renamer.NameForSymbol(e.Ref) {
						if item.InitializerOrNil.Data != nil {
							p.printSpace()
							p.print("=")
							p.printSpace()
							p.printExpr(item.InitializerOrNil, js_ast.LComma, 0)
						}
						return
					}

				case *js_ast.EImportIdentifier:
					// Make sure we're not using a property access instead of an identifier
					ref := js_ast.FollowSymbols(p.symbols, e.Ref)
					if symbol := p.symbols.Get(ref); symbol.NamespaceAlias == nil && name == p.renamer.NameForSymbol(ref) &&
						p.options.ConstValues[ref].Kind == js_ast.ConstValueNone {
						if item.InitializerOrNil.Data != nil {
							p.printSpace()
							p.print("=")
							p.printSpace()
							p.printExpr(item.InitializerOrNil, js_ast.LComma, 0)
						}
						return
					}
				}
			}
		} else {
			p.printQuotedUTF8(name, false /* allowBacktick */)
		}

	case *js_ast.EString:
		p.addSourceMapping(item.Key.Loc)
		if !item.PreferQuotedKey && p.canPrintIdentifierUTF16(key.Value) {
			p.printSpaceBeforeIdentifier()
			p.printIdentifierUTF16(key.Value)

			// Use a shorthand property if the names are the same
			if !p.options.UnsupportedFeatures.Has(compat.ObjectExtensions) && item.ValueOrNil.Data != nil {
				switch e := item.ValueOrNil.Data.(type) {
				case *js_ast.EIdentifier:
					if helpers.UTF16EqualsString(key.Value, p.renamer.NameForSymbol(e.Ref)) {
						if item.InitializerOrNil.Data != nil {
							p.printSpace()
							p.print("=")
							p.printSpace()
							p.printExpr(item.InitializerOrNil, js_ast.LComma, 0)
						}
						return
					}

				case *js_ast.EImportIdentifier:
					// Make sure we're not using a property access instead of an identifier
					ref := js_ast.FollowSymbols(p.symbols, e.Ref)
					if symbol := p.symbols.Get(ref); symbol.NamespaceAlias == nil && helpers.UTF16EqualsString(key.Value, p.renamer.NameForSymbol(ref)) &&
						p.options.ConstValues[ref].Kind == js_ast.ConstValueNone {
						if item.InitializerOrNil.Data != nil {
							p.printSpace()
							p.print("=")
							p.printSpace()
							p.printExpr(item.InitializerOrNil, js_ast.LComma, 0)
						}
						return
					}
				}
			}
		} else {
			p.printQuotedUTF16(key.Value, false /* allowBacktick */)
		}

	default:
		p.printExpr(item.Key, js_ast.LLowest, 0)
	}

	if item.Kind != js_ast.PropertyNormal {
		f, ok := item.ValueOrNil.Data.(*js_ast.EFunction)
		if ok {
			p.printFn(f.Fn)
			return
		}
	}

	if item.ValueOrNil.Data != nil {
		if fn, ok := item.ValueOrNil.Data.(*js_ast.EFunction); item.IsMethod && ok {
			p.printFn(fn.Fn)
			return
		}

		p.print(":")
		p.printSpace()
		p.printExpr(item.ValueOrNil, js_ast.LComma, 0)
	}

	if item.InitializerOrNil.Data != nil {
		p.printSpace()
		p.print("=")
		p.printSpace()
		p.printExpr(item.InitializerOrNil, js_ast.LComma, 0)
	}
}

func (p *printer) printQuotedUTF16(data []uint16, allowBacktick bool) {
	if p.options.UnsupportedFeatures.Has(compat.TemplateLiteral) {
		allowBacktick = false
	}

	singleCost := 0
	doubleCost := 0
	backtickCost := 0

	for i, c := range data {
		switch c {
		case '\n':
			if p.options.MinifySyntax {
				// The backslash for the newline costs an extra character for old-style
				// string literals when compared to a template literal
				backtickCost--
			}
		case '\'':
			singleCost++
		case '"':
			doubleCost++
		case '`':
			backtickCost++
		case '$':
			// "${" sequences need to be escaped in template literals
			if i+1 < len(data) && data[i+1] == '{' {
				backtickCost++
			}
		}
	}

	c := "\""
	if doubleCost > singleCost {
		c = "'"
		if singleCost > backtickCost && allowBacktick {
			c = "`"
		}
	} else if doubleCost > backtickCost && allowBacktick {
		c = "`"
	}

	p.print(c)
	p.printUnquotedUTF16(data, rune(c[0]))
	p.print(c)
}

func (p *printer) printRequireOrImportExpr(
	importRecordIndex uint32,
	leadingInteriorComments []js_ast.Comment,
	level js_ast.L,
	flags printExprFlags,
) {
	record := &p.importRecords[importRecordIndex]

	if level >= js_ast.LNew || (flags&forbidCall) != 0 {
		p.print("(")
		defer p.print(")")
		level = js_ast.LLowest
	}

	if !record.SourceIndex.IsValid() {
		// External "require()"
		if record.Kind != ast.ImportDynamic {
			// Wrap this with a call to "__toESM()" if this is a CommonJS file
			wrapWithToESM := record.Flags.Has(ast.WrapWithToESM)
			if wrapWithToESM {
				p.printSymbol(p.options.ToESMRef)
				p.print("(")
			}

			// Potentially substitute our own "__require" stub for "require"
			if record.Flags.Has(ast.CallRuntimeRequire) {
				p.printSymbol(p.options.RuntimeRequireRef)
			} else {
				p.printSpaceBeforeIdentifier()
				p.print("require")
			}

			p.print("(")
			p.addSourceMapping(record.Range.Loc)
			p.printQuotedUTF8(record.Path.Text, true /* allowBacktick */)
			p.print(")")

			// Finish the call to "__toESM()"
			if wrapWithToESM {
				if p.moduleType.IsESM() {
					p.print(",")
					p.printSpace()
					p.print("1")
				}
				p.print(")")
			}
			return
		}

		// External "import()"
		if !p.options.UnsupportedFeatures.Has(compat.DynamicImport) {
			p.printSpaceBeforeIdentifier()
			p.print("import(")
			defer p.print(")")
		} else {
			p.printSpaceBeforeIdentifier()
			p.print("Promise.resolve()")
			p.printDotThenPrefix()
			defer p.printDotThenSuffix()

			// Wrap this with a call to "__toESM()" if this is a CommonJS file
			if record.Flags.Has(ast.WrapWithToESM) {
				p.printSymbol(p.options.ToESMRef)
				p.print("(")
				defer func() {
					if p.moduleType.IsESM() {
						p.print(",")
						p.printSpace()
						p.print("1")
					}
					p.print(")")
				}()
			}

			// Potentially substitute our own "__require" stub for "require"
			if record.Flags.Has(ast.CallRuntimeRequire) {
				p.printSymbol(p.options.RuntimeRequireRef)
			} else {
				p.printSpaceBeforeIdentifier()
				p.print("require")
			}

			p.print("(")
			defer p.print(")")
		}
		if len(leadingInteriorComments) > 0 {
			p.printNewline()
			p.options.Indent++
			for _, comment := range leadingInteriorComments {
				p.printIndentedComment(comment.Text)
			}
			p.printIndent()
		}
		p.addSourceMapping(record.Range.Loc)
		p.printQuotedUTF8(record.Path.Text, true /* allowBacktick */)
		if !p.options.UnsupportedFeatures.Has(compat.DynamicImport) {
			p.printImportCallAssertions(record.Assertions)
		}
		if len(leadingInteriorComments) > 0 {
			p.printNewline()
			p.options.Indent--
			p.printIndent()
		}
		return
	}

	meta := p.options.RequireOrImportMetaForSource(record.SourceIndex.GetIndex())

	// Don't need the namespace object if the result is unused anyway
	if (flags & exprResultIsUnused) != 0 {
		meta.ExportsRef = js_ast.InvalidRef
	}

	// Internal "import()" of async ESM
	if record.Kind == ast.ImportDynamic && meta.IsWrapperAsync {
		p.printSymbol(meta.WrapperRef)
		p.print("()")
		if meta.ExportsRef != js_ast.InvalidRef {
			p.printDotThenPrefix()
			p.printSymbol(meta.ExportsRef)
			p.printDotThenSuffix()
		}
		return
	}

	// Internal "require()" or "import()"
	if record.Kind == ast.ImportDynamic {
		p.printSpaceBeforeIdentifier()
		p.print("Promise.resolve()")
		level = p.printDotThenPrefix()
		defer p.printDotThenSuffix()
	}

	// Make sure the comma operator is propertly wrapped
	if meta.ExportsRef != js_ast.InvalidRef && level >= js_ast.LComma {
		p.print("(")
		defer p.print(")")
	}

	// Wrap this with a call to "__toESM()" if this is a CommonJS file
	wrapWithToESM := record.Flags.Has(ast.WrapWithToESM)
	if wrapWithToESM {
		p.printSymbol(p.options.ToESMRef)
		p.print("(")
	}

	// Call the wrapper
	p.printSymbol(meta.WrapperRef)
	p.print("()")

	// Return the namespace object if this is an ESM file
	if meta.ExportsRef != js_ast.InvalidRef {
		p.print(",")
		p.printSpace()

		// Wrap this with a call to "__toCommonJS()" if this is an ESM file
		wrapWithTpCJS := record.Flags.Has(ast.WrapWithToCJS)
		if wrapWithTpCJS {
			p.printSymbol(p.options.ToCommonJSRef)
			p.print("(")
		}
		p.printSymbol(meta.ExportsRef)
		if wrapWithTpCJS {
			p.print(")")
		}
	}

	// Finish the call to "__toESM()"
	if wrapWithToESM {
		if p.moduleType.IsESM() {
			p.print(",")
			p.printSpace()
			p.print("1")
		}
		p.print(")")
	}
}

func (p *printer) printDotThenPrefix() js_ast.L {
	if p.options.UnsupportedFeatures.Has(compat.Arrow) {
		p.print(".then(function()")
		p.printSpace()
		p.print("{")
		p.printNewline()
		p.options.Indent++
		p.printIndent()
		p.print("return")
		p.printSpace()
		return js_ast.LLowest
	} else {
		p.print(".then(()")
		p.printSpace()
		p.print("=>")
		p.printSpace()
		return js_ast.LComma
	}
}

func (p *printer) printDotThenSuffix() {
	if p.options.UnsupportedFeatures.Has(compat.Arrow) {
		if !p.options.MinifyWhitespace {
			p.print(";")
		}
		p.printNewline()
		p.options.Indent--
		p.printIndent()
		p.print("})")
	} else {
		p.print(")")
	}
}

func (p *printer) printUndefined(level js_ast.L) {
	if level >= js_ast.LPrefix {
		p.print("(void 0)")
	} else {
		p.printSpaceBeforeIdentifier()
		p.print("void 0")
		p.prevNumEnd = len(p.js)
	}
}

// Call this before printing an expression to see if it turned out to be empty.
// We use this to do inlining of empty functions at print time. It can't happen
// during parse time because a) parse time only has two passes and we only know
// if a function can be inlined at the end of the second pass (due to is-mutated
// analysis) and b) we want to enable cross-module inlining of empty functions
// which has to happen after linking.
//
// This function returns "nil" to indicate that the expression should be removed
// completely.
//
// This function doesn't need to search everywhere inside the entire expression
// for calls to inline. Calls are automatically inlined when printed. However,
// the printer replaces the call with "undefined" since the result may still
// be needed by the caller. If the caller knows that it doesn't need the result,
// it should call this function first instead so we don't print "undefined".
//
// This is a separate function instead of trying to work this logic into the
// printer because it's too late to eliminate the expression entirely when we're
// in the printer. We may have already printed the leading indent, for example.
func (p *printer) simplifyUnusedExpr(expr js_ast.Expr) js_ast.Expr {
	switch e := expr.Data.(type) {
	case *js_ast.EBinary:
		// Calls to be inlined may be hidden inside a comma operator chain
		if e.Op == js_ast.BinOpComma {
			left := p.simplifyUnusedExpr(e.Left)
			right := p.simplifyUnusedExpr(e.Right)
			if left.Data != e.Left.Data || right.Data != e.Right.Data {
				return js_ast.JoinWithComma(left, right)
			}
		}

	case *js_ast.ECall:
		var symbolFlags js_ast.SymbolFlags
		switch target := e.Target.Data.(type) {
		case *js_ast.EIdentifier:
			symbolFlags = p.symbols.Get(target.Ref).Flags
		case *js_ast.EImportIdentifier:
			ref := js_ast.FollowSymbols(p.symbols, target.Ref)
			symbolFlags = p.symbols.Get(ref).Flags
		}

		// Replace non-mutated empty functions with their arguments at print time
		if (symbolFlags & (js_ast.IsEmptyFunction | js_ast.CouldPotentiallyBeMutated)) == js_ast.IsEmptyFunction {
			var replacement js_ast.Expr
			for _, arg := range e.Args {
				replacement = js_ast.JoinWithComma(replacement, js_ast.SimplifyUnusedExpr(p.simplifyUnusedExpr(arg), p.isUnbound))
			}
			return replacement // Don't add "undefined" here because the result isn't used
		}

		// Inline non-mutated identity functions at print time
		if (symbolFlags&(js_ast.IsIdentityFunction|js_ast.CouldPotentiallyBeMutated)) == js_ast.IsIdentityFunction && len(e.Args) == 1 {
			return js_ast.SimplifyUnusedExpr(p.simplifyUnusedExpr(e.Args[0]), p.isUnbound)
		}
	}

	return expr
}

// This assumes the original expression was some form of indirect value, such
// as a value returned from a function call or the result of a comma operator.
// In this case, there is no special behavior with the "delete" operator or
// with function calls. If we substitute this indirect value for another value
// due to inlining, we have to make sure we don't accidentally introduce special
// behavior.
func (p *printer) guardAgainstBehaviorChangeDueToSubstitution(expr js_ast.Expr, flags printExprFlags) js_ast.Expr {
	wrap := false

	if (flags & isDeleteTarget) != 0 {
		// "delete id(x)" must not become "delete x"
		// "delete (empty(), x)" must not become "delete x"
		if binary, ok := expr.Data.(*js_ast.EBinary); !ok || binary.Op != js_ast.BinOpComma {
			wrap = true
		}
	} else if (flags & isCallTargetOrTemplateTag) != 0 {
		// "id(x.y)()" must not become "x.y()"
		// "id(x.y)``" must not become "x.y``"
		// "(empty(), x.y)()" must not become "x.y()"
		// "(empty(), eval)()" must not become "eval()"
		switch expr.Data.(type) {
		case *js_ast.EDot, *js_ast.EIndex:
			wrap = true
		case *js_ast.EIdentifier:
			if p.isUnboundEvalIdentifier(expr) {
				wrap = true
			}
		}
	}

	if wrap {
		expr.Data = &js_ast.EBinary{
			Op:    js_ast.BinOpComma,
			Left:  js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: 0}},
			Right: expr,
		}
	}

	return expr
}

// Constant folding is already implemented once in the parser. A smaller form
// of constant folding (just for numbers) is implemented here to clean up cross-
// module numeric constants and bitwise operations. This is not an general-
// purpose/optimal approach and never will be. For example, we can't affect
// tree shaking at this stage because it has already happened.
func (p *printer) lateConstantFoldUnaryOrBinaryExpr(expr js_ast.Expr) js_ast.Expr {
	switch e := expr.Data.(type) {
	case *js_ast.EImportIdentifier:
		ref := js_ast.FollowSymbols(p.symbols, e.Ref)
		if value := p.options.ConstValues[ref]; value.Kind != js_ast.ConstValueNone {
			return js_ast.ConstValueToExpr(expr.Loc, value)
		}

	case *js_ast.EDot:
		if id, ok := e.Target.Data.(*js_ast.EImportIdentifier); ok {
			ref := js_ast.FollowSymbols(p.symbols, id.Ref)
			if symbol := p.symbols.Get(ref); symbol.Kind == js_ast.SymbolTSEnum {
				if enum, ok := p.options.TSEnums[ref]; ok {
					if value, ok := enum[e.Name]; ok && value.String == nil {
						value := js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: value.Number}}

						if strings.Contains(e.Name, "*/") {
							// Don't wrap with a comment
							return value
						}

						// Wrap with a comment
						return js_ast.Expr{Loc: value.Loc, Data: &js_ast.EInlinedEnum{
							Value:   value,
							Comment: e.Name,
						}}
					}
				}
			}
		}

	case *js_ast.EUnary:
		value := p.lateConstantFoldUnaryOrBinaryExpr(e.Value)

		// Only fold again if something chained
		if value.Data != e.Value.Data {
			// Only fold certain operations (just like the parser)
			if v, ok := js_ast.ToNumberWithoutSideEffects(value.Data); ok {
				switch e.Op {
				case js_ast.UnOpPos:
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: v}}

				case js_ast.UnOpNeg:
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: -v}}

				case js_ast.UnOpCpl:
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: float64(^js_ast.ToInt32(v))}}
				}
			}

			// Don't mutate the original AST
			expr.Data = &js_ast.EUnary{Op: e.Op, Value: value}
		}

	case *js_ast.EBinary:
		left := p.lateConstantFoldUnaryOrBinaryExpr(e.Left)
		right := p.lateConstantFoldUnaryOrBinaryExpr(e.Right)

		// Only fold again if something chained
		if left.Data != e.Left.Data || right.Data != e.Right.Data {
			// Only fold certain operations (just like the parser)
			if l, r, ok := js_ast.ExtractNumericValues(left, right); ok {
				switch e.Op {
				case js_ast.BinOpShr:
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: float64(js_ast.ToInt32(l) >> js_ast.ToInt32(r))}}

				case js_ast.BinOpBitwiseAnd:
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: float64(js_ast.ToInt32(l) & js_ast.ToInt32(r))}}

				case js_ast.BinOpBitwiseOr:
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: float64(js_ast.ToInt32(l) | js_ast.ToInt32(r))}}

				case js_ast.BinOpBitwiseXor:
					return js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: float64(js_ast.ToInt32(l) ^ js_ast.ToInt32(r))}}
				}
			}

			// Don't mutate the original AST
			expr.Data = &js_ast.EBinary{Op: e.Op, Left: left, Right: right}
		}
	}

	return expr
}

type printExprFlags uint16

const (
	forbidCall printExprFlags = 1 << iota
	forbidIn
	hasNonOptionalChainParent
	exprResultIsUnused
	didAlreadySimplifyUnusedExprs
	isFollowedByOf
	isInsideForAwait
	isDeleteTarget
	isCallTargetOrTemplateTag
	parentWasUnaryOrBinary
)

func (p *printer) printExpr(expr js_ast.Expr, level js_ast.L, flags printExprFlags) {
	// If syntax compression is enabled, do a pre-pass over unary and binary
	// operators to inline bitwise operations of cross-module inlined constants.
	// This makes the output a little tighter if people construct bit masks in
	// other files. This is not a general-purpose constant folding pass. In
	// particular, it has no effect on tree shaking because that pass has already
	// been run.
	//
	// This sets a flag to avoid doing this when the parent is a unary or binary
	// operator so that we don't trigger O(n^2) behavior when traversing over a
	// large expression tree.
	if p.options.MinifySyntax && (flags&parentWasUnaryOrBinary) == 0 {
		switch expr.Data.(type) {
		case *js_ast.EUnary, *js_ast.EBinary:
			expr = p.lateConstantFoldUnaryOrBinaryExpr(expr)
		}
	}

	p.addSourceMapping(expr.Loc)

	switch e := expr.Data.(type) {
	case *js_ast.EMissing:

	case *js_ast.EUndefined:
		p.printUndefined(level)

	case *js_ast.ESuper:
		p.printSpaceBeforeIdentifier()
		p.print("super")

	case *js_ast.ENull:
		p.printSpaceBeforeIdentifier()
		p.print("null")

	case *js_ast.EThis:
		p.printSpaceBeforeIdentifier()
		p.print("this")

	case *js_ast.ESpread:
		p.print("...")
		p.printExpr(e.Value, js_ast.LComma, 0)

	case *js_ast.ENewTarget:
		p.printSpaceBeforeIdentifier()
		p.print("new.target")

	case *js_ast.EImportMeta:
		p.printSpaceBeforeIdentifier()
		p.print("import.meta")

	case *js_ast.EMangledProp:
		p.printQuotedUTF8(p.renamer.NameForSymbol(e.Ref), true)

	case *js_ast.EJSXElement:
		// Start the opening tag
		p.print("<")
		p.printJSXTag(e.TagOrNil)

		// Print the attributes
		for _, property := range e.Properties {
			p.printSpace()

			if property.Kind == js_ast.PropertySpread {
				p.print("{...")
				p.printExpr(property.ValueOrNil, js_ast.LComma, 0)
				p.print("}")
				continue
			}

			p.printSpaceBeforeIdentifier()
			p.addSourceMapping(property.Key.Loc)
			if mangled, ok := property.Key.Data.(*js_ast.EMangledProp); ok {
				p.printSymbol(mangled.Ref)
			} else {
				p.print(helpers.UTF16ToString(property.Key.Data.(*js_ast.EString).Value))
			}

			// Special-case string values
			if str, ok := property.ValueOrNil.Data.(*js_ast.EString); ok {
				if quote, ok := p.canPrintTextAsJSXAttribute(str.Value); ok {
					p.print("=")
					p.addSourceMapping(property.ValueOrNil.Loc)
					p.print(quote)
					p.print(helpers.UTF16ToString(str.Value))
					p.print(quote)
					continue
				}
			}

			// Implicit "true" value
			if boolean, ok := property.ValueOrNil.Data.(*js_ast.EBoolean); ok && boolean.Value && property.WasShorthand {
				continue
			}

			// Generic JS value
			p.print("={")
			p.printExpr(property.ValueOrNil, js_ast.LComma, 0)
			p.print("}")
		}

		// End the opening tag
		if e.TagOrNil.Data != nil && len(e.Children) == 0 {
			p.printSpace()
			p.addSourceMapping(e.CloseLoc)
			p.print("/>")
			break
		}
		p.print(">")

		isSingleLine := true
		if !p.options.MinifyWhitespace {
			isSingleLine = len(e.Children) < 2
			if len(e.Children) == 1 {
				if _, ok := e.Children[0].Data.(*js_ast.EJSXElement); !ok {
					isSingleLine = true
				}
			}
		}
		if !isSingleLine {
			p.options.Indent++
		}

		// Print the children
		for _, child := range e.Children {
			if !isSingleLine {
				p.printNewline()
				p.printIndent()
			}
			if _, ok := child.Data.(*js_ast.EJSXElement); ok {
				p.printExpr(child, js_ast.LLowest, 0)
			} else if str, ok := child.Data.(*js_ast.EString); ok && isSingleLine && p.canPrintTextAsJSXChild(str.Value) {
				p.addSourceMapping(child.Loc)
				p.print(helpers.UTF16ToString(str.Value))
			} else {
				p.print("{")
				p.printExpr(child, js_ast.LComma, 0)
				p.print("}")
			}
		}

		// Print the closing tag
		if !isSingleLine {
			p.options.Indent--
			p.printNewline()
			p.printIndent()
		}
		p.addSourceMapping(e.CloseLoc)
		p.print("</")
		p.printJSXTag(e.TagOrNil)
		p.print(">")

	case *js_ast.ENew:
		wrap := level >= js_ast.LCall

		hasPureComment := !p.options.MinifyWhitespace && e.CanBeUnwrappedIfUnused
		if hasPureComment && level >= js_ast.LPostfix {
			wrap = true
		}

		if wrap {
			p.print("(")
		}

		if hasPureComment {
			p.print("/* @__PURE__ */ ")
		}

		p.printSpaceBeforeIdentifier()
		p.print("new")
		p.printSpace()
		p.printExpr(e.Target, js_ast.LNew, forbidCall)

		// Omit the "()" when minifying, but only when safe to do so
		if !p.options.MinifyWhitespace || len(e.Args) > 0 || level >= js_ast.LPostfix {
			p.print("(")
			for i, arg := range e.Args {
				if i != 0 {
					p.print(",")
					p.printSpace()
				}
				p.printExpr(arg, js_ast.LComma, 0)
			}
			p.print(")")
		}

		if wrap {
			p.print(")")
		}

	case *js_ast.ECall:
		if p.options.MinifySyntax {
			var symbolFlags js_ast.SymbolFlags
			switch target := e.Target.Data.(type) {
			case *js_ast.EIdentifier:
				symbolFlags = p.symbols.Get(target.Ref).Flags
			case *js_ast.EImportIdentifier:
				ref := js_ast.FollowSymbols(p.symbols, target.Ref)
				symbolFlags = p.symbols.Get(ref).Flags
			}

			// Replace non-mutated empty functions with their arguments at print time
			if (symbolFlags & (js_ast.IsEmptyFunction | js_ast.CouldPotentiallyBeMutated)) == js_ast.IsEmptyFunction {
				var replacement js_ast.Expr
				for _, arg := range e.Args {
					replacement = js_ast.JoinWithComma(replacement, js_ast.SimplifyUnusedExpr(arg, p.isUnbound))
				}
				if replacement.Data == nil || (flags&exprResultIsUnused) == 0 {
					replacement = js_ast.JoinWithComma(replacement, js_ast.Expr{Loc: expr.Loc, Data: js_ast.EUndefinedShared})
				}
				p.printExpr(p.guardAgainstBehaviorChangeDueToSubstitution(replacement, flags), level, flags)
				break
			}

			// Inline non-mutated identity functions at print time
			if (symbolFlags&(js_ast.IsIdentityFunction|js_ast.CouldPotentiallyBeMutated)) == js_ast.IsIdentityFunction && len(e.Args) == 1 {
				arg := e.Args[0]
				if (flags & exprResultIsUnused) != 0 {
					arg = js_ast.SimplifyUnusedExpr(arg, p.isUnbound)
				}
				p.printExpr(p.guardAgainstBehaviorChangeDueToSubstitution(arg, flags), level, flags)
				break
			}
		}

		wrap := level >= js_ast.LNew || (flags&forbidCall) != 0
		var targetFlags printExprFlags
		if e.OptionalChain == js_ast.OptionalChainNone {
			targetFlags = hasNonOptionalChainParent
		} else if (flags & hasNonOptionalChainParent) != 0 {
			wrap = true
		}

		hasPureComment := !p.options.MinifyWhitespace && e.CanBeUnwrappedIfUnused
		if hasPureComment && level >= js_ast.LPostfix {
			wrap = true
		}

		if wrap {
			p.print("(")
		}

		if hasPureComment {
			wasStmtStart := p.stmtStart == len(p.js)
			p.print("/* @__PURE__ */ ")
			if wasStmtStart {
				p.stmtStart = len(p.js)
			}
		}

		// We don't ever want to accidentally generate a direct eval expression here
		p.callTarget = e.Target.Data
		if !e.IsDirectEval && p.isUnboundEvalIdentifier(e.Target) {
			if p.options.MinifyWhitespace {
				p.print("(0,")
			} else {
				p.print("(0, ")
			}
			p.printExpr(e.Target, js_ast.LPostfix, isCallTargetOrTemplateTag)
			p.print(")")
		} else {
			p.printExpr(e.Target, js_ast.LPostfix, isCallTargetOrTemplateTag|targetFlags)
		}

		if e.OptionalChain == js_ast.OptionalChainStart {
			p.print("?.")
		}
		p.print("(")
		for i, arg := range e.Args {
			if i != 0 {
				p.print(",")
				p.printSpace()
			}
			p.printExpr(arg, js_ast.LComma, 0)
		}
		p.print(")")
		if wrap {
			p.print(")")
		}

	case *js_ast.ERequireString:
		p.printRequireOrImportExpr(e.ImportRecordIndex, nil, level, flags)

	case *js_ast.ERequireResolveString:
		wrap := level >= js_ast.LNew || (flags&forbidCall) != 0
		if wrap {
			p.print("(")
		}
		p.printSpaceBeforeIdentifier()
		p.print("require.resolve(")
		p.printQuotedUTF8(p.importRecords[e.ImportRecordIndex].Path.Text, true /* allowBacktick */)
		p.print(")")
		if wrap {
			p.print(")")
		}

	case *js_ast.EImportString:
		var leadingInteriorComments []js_ast.Comment
		if !p.options.MinifyWhitespace {
			leadingInteriorComments = e.LeadingInteriorComments
		}
		p.printRequireOrImportExpr(e.ImportRecordIndex, leadingInteriorComments, level, flags)

	case *js_ast.EImportCall:
		var leadingInteriorComments []js_ast.Comment
		if !p.options.MinifyWhitespace {
			leadingInteriorComments = e.LeadingInteriorComments
		}
		wrap := level >= js_ast.LNew || (flags&forbidCall) != 0
		if wrap {
			p.print("(")
		}
		p.printSpaceBeforeIdentifier()
		p.print("import(")
		if len(leadingInteriorComments) > 0 {
			p.printNewline()
			p.options.Indent++
			for _, comment := range leadingInteriorComments {
				p.printIndentedComment(comment.Text)
			}
			p.printIndent()
		}
		p.printExpr(e.Expr, js_ast.LComma, 0)
		if e.OptionsOrNil.Data != nil {
			p.print(",")
			p.printSpace()
			p.printExpr(e.OptionsOrNil, js_ast.LComma, 0)
		}
		if len(leadingInteriorComments) > 0 {
			p.printNewline()
			p.options.Indent--
			p.printIndent()
		}
		p.print(")")
		if wrap {
			p.print(")")
		}

	case *js_ast.EDot:
		wrap := false
		if e.OptionalChain == js_ast.OptionalChainNone {
			flags |= hasNonOptionalChainParent

			// Inline cross-module TypeScript enum references here
			if id, ok := e.Target.Data.(*js_ast.EImportIdentifier); ok {
				ref := js_ast.FollowSymbols(p.symbols, id.Ref)
				if symbol := p.symbols.Get(ref); symbol.Kind == js_ast.SymbolTSEnum {
					if enum, ok := p.options.TSEnums[ref]; ok {
						if value, ok := enum[e.Name]; ok {
							if value.String != nil {
								p.printQuotedUTF16(value.String, true /* allowBacktick */)
							} else {
								p.printNumber(value.Number, level)
							}
							if !p.options.MinifyWhitespace && !p.options.MinifyIdentifiers {
								p.print(" /* ")
								p.print(e.Name)
								p.print(" */")
							}
							break
						}
					}
				}
			}
		} else {
			if (flags & hasNonOptionalChainParent) != 0 {
				wrap = true
				p.print("(")
			}
			flags &= ^hasNonOptionalChainParent
		}
		p.printExpr(e.Target, js_ast.LPostfix, flags&(forbidCall|hasNonOptionalChainParent))
		if p.canPrintIdentifier(e.Name) {
			if e.OptionalChain != js_ast.OptionalChainStart && p.prevNumEnd == len(p.js) {
				// "1.toString" is a syntax error, so print "1 .toString" instead
				p.print(" ")
			}
			if e.OptionalChain == js_ast.OptionalChainStart {
				p.print("?.")
			} else {
				p.print(".")
			}
			p.addSourceMapping(e.NameLoc)
			p.printIdentifier(e.Name)
		} else {
			if e.OptionalChain == js_ast.OptionalChainStart {
				p.print("?.")
			}
			p.print("[")
			p.addSourceMapping(e.NameLoc)
			p.printQuotedUTF8(e.Name, true /* allowBacktick */)
			p.print("]")
		}
		if wrap {
			p.print(")")
		}

	case *js_ast.EIndex:
		wrap := false
		if e.OptionalChain == js_ast.OptionalChainNone {
			flags |= hasNonOptionalChainParent

			// Inline cross-module TypeScript enum references here
			if index, ok := e.Index.Data.(*js_ast.EString); ok {
				if id, ok := e.Target.Data.(*js_ast.EImportIdentifier); ok {
					ref := js_ast.FollowSymbols(p.symbols, id.Ref)
					if symbol := p.symbols.Get(ref); symbol.Kind == js_ast.SymbolTSEnum {
						if enum, ok := p.options.TSEnums[ref]; ok {
							name := helpers.UTF16ToString(index.Value)
							if value, ok := enum[name]; ok {
								if value.String != nil {
									p.printQuotedUTF16(value.String, true /* allowBacktick */)
								} else {
									p.printNumber(value.Number, level)
								}
								if !p.options.MinifyWhitespace && !p.options.MinifyIdentifiers {
									p.print(" /* ")
									p.print(name)
									p.print(" */")
								}
								break
							}
						}
					}
				}
			}
		} else {
			if (flags & hasNonOptionalChainParent) != 0 {
				wrap = true
				p.print("(")
			}
			flags &= ^hasNonOptionalChainParent
		}
		p.printExpr(e.Target, js_ast.LPostfix, flags&(forbidCall|hasNonOptionalChainParent))
		if e.OptionalChain == js_ast.OptionalChainStart {
			p.print("?.")
		}

		switch index := e.Index.Data.(type) {
		case *js_ast.EPrivateIdentifier:
			if e.OptionalChain != js_ast.OptionalChainStart {
				p.print(".")
			}
			p.printSymbol(index.Ref)

		case *js_ast.EMangledProp:
			if name := p.renamer.NameForSymbol(index.Ref); p.canPrintIdentifier(name) {
				if e.OptionalChain != js_ast.OptionalChainStart {
					p.print(".")
				}
				p.printIdentifier(name)
			} else {
				p.print("[")
				p.printQuotedUTF8(name, true /* allowBacktick */)
				p.print("]")
			}

		default:
			p.print("[")
			p.printExpr(e.Index, js_ast.LLowest, 0)
			p.print("]")
		}

		if wrap {
			p.print(")")
		}

	case *js_ast.EIf:
		wrap := level >= js_ast.LConditional
		if wrap {
			p.print("(")
			flags &= ^forbidIn
		}
		p.printExpr(e.Test, js_ast.LConditional, flags&forbidIn)
		p.printSpace()
		p.print("?")
		p.printSpace()
		p.printExpr(e.Yes, js_ast.LYield, 0)
		p.printSpace()
		p.print(":")
		p.printSpace()
		p.printExpr(e.No, js_ast.LYield, flags&forbidIn)
		if wrap {
			p.print(")")
		}

	case *js_ast.EArrow:
		wrap := level >= js_ast.LAssign

		if wrap {
			p.print("(")
		}
		if e.IsAsync {
			p.printSpaceBeforeIdentifier()
			p.print("async")
			p.printSpace()
		}

		p.printFnArgs(e.Args, e.HasRestArg, true /* isArrow */)
		p.printSpace()
		p.print("=>")
		p.printSpace()

		wasPrinted := false
		if len(e.Body.Stmts) == 1 && e.PreferExpr {
			if s, ok := e.Body.Stmts[0].Data.(*js_ast.SReturn); ok && s.ValueOrNil.Data != nil {
				p.arrowExprStart = len(p.js)
				p.printExpr(s.ValueOrNil, js_ast.LComma, flags&forbidIn)
				wasPrinted = true
			}
		}
		if !wasPrinted {
			p.printBlock(e.Body.Loc, e.Body.Stmts)
		}
		if wrap {
			p.print(")")
		}

	case *js_ast.EFunction:
		n := len(p.js)
		wrap := p.stmtStart == n || p.exportDefaultStart == n
		if wrap {
			p.print("(")
		}
		p.printSpaceBeforeIdentifier()
		if e.Fn.IsAsync {
			p.print("async ")
		}
		p.print("function")
		if e.Fn.IsGenerator {
			p.print("*")
			p.printSpace()
		}
		if e.Fn.Name != nil {
			p.printSymbol(e.Fn.Name.Ref)
		}
		p.printFn(e.Fn)
		if wrap {
			p.print(")")
		}

	case *js_ast.EClass:
		n := len(p.js)
		wrap := p.stmtStart == n || p.exportDefaultStart == n
		if wrap {
			p.print("(")
		}
		p.printSpaceBeforeIdentifier()
		p.print("class")
		if e.Class.Name != nil {
			p.printSymbol(e.Class.Name.Ref)
		}
		p.printClass(e.Class)
		if wrap {
			p.print(")")
		}

	case *js_ast.EArray:
		p.print("[")
		if len(e.Items) > 0 {
			if !e.IsSingleLine {
				p.options.Indent++
			}

			for i, item := range e.Items {
				if i != 0 {
					p.print(",")
					if e.IsSingleLine {
						p.printSpace()
					}
				}
				if !e.IsSingleLine {
					p.printNewline()
					p.printIndent()
				}
				p.printExpr(item, js_ast.LComma, 0)

				// Make sure there's a comma after trailing missing items
				_, ok := item.Data.(*js_ast.EMissing)
				if ok && i == len(e.Items)-1 {
					p.print(",")
				}
			}

			if !e.IsSingleLine {
				p.options.Indent--
				p.printNewline()
				p.printIndent()
			}
		}
		p.print("]")

	case *js_ast.EObject:
		n := len(p.js)
		wrap := p.stmtStart == n || p.arrowExprStart == n
		if wrap {
			p.print("(")
		}
		p.print("{")
		if len(e.Properties) != 0 {
			if !e.IsSingleLine {
				p.options.Indent++
			}

			for i, item := range e.Properties {
				if i != 0 {
					p.print(",")
				}
				if e.IsSingleLine {
					p.printSpace()
				} else {
					p.printNewline()
					p.printIndent()
				}
				p.printProperty(item)
			}

			if !e.IsSingleLine {
				p.options.Indent--
				p.printNewline()
				p.printIndent()
			} else if len(e.Properties) > 0 {
				p.printSpace()
			}
		}
		p.print("}")
		if wrap {
			p.print(")")
		}

	case *js_ast.EBoolean:
		if p.options.MinifySyntax {
			if level >= js_ast.LPrefix {
				if e.Value {
					p.print("(!0)")
				} else {
					p.print("(!1)")
				}
			} else {
				if e.Value {
					p.print("!0")
				} else {
					p.print("!1")
				}
			}
		} else {
			p.printSpaceBeforeIdentifier()
			if e.Value {
				p.print("true")
			} else {
				p.print("false")
			}
		}

	case *js_ast.EString:
		// If this was originally a template literal, print it as one as long as we're not minifying
		if e.PreferTemplate && !p.options.MinifySyntax && !p.options.UnsupportedFeatures.Has(compat.TemplateLiteral) {
			p.print("`")
			p.printUnquotedUTF16(e.Value, '`')
			p.print("`")
			return
		}

		p.printQuotedUTF16(e.Value, true /* allowBacktick */)

	case *js_ast.ETemplate:
		// Convert no-substitution template literals into strings if it's smaller
		if p.options.MinifySyntax && e.TagOrNil.Data == nil && len(e.Parts) == 0 {
			p.printQuotedUTF16(e.HeadCooked, true /* allowBacktick */)
			return
		}

		if e.TagOrNil.Data != nil {
			// Optional chains are forbidden in template tags
			if js_ast.IsOptionalChain(e.TagOrNil) {
				p.print("(")
				p.printExpr(e.TagOrNil, js_ast.LLowest, isCallTargetOrTemplateTag)
				p.print(")")
			} else {
				p.printExpr(e.TagOrNil, js_ast.LPostfix, isCallTargetOrTemplateTag)
			}
		}
		p.print("`")
		if e.TagOrNil.Data != nil {
			p.print(e.HeadRaw)
		} else {
			p.printUnquotedUTF16(e.HeadCooked, '`')
		}
		for _, part := range e.Parts {
			p.print("${")
			p.printExpr(part.Value, js_ast.LLowest, 0)
			p.print("}")
			if e.TagOrNil.Data != nil {
				p.print(part.TailRaw)
			} else {
				p.printUnquotedUTF16(part.TailCooked, '`')
			}
		}
		p.print("`")

	case *js_ast.ERegExp:
		buffer := p.js
		n := len(buffer)

		if n > 0 {
			// Avoid forming a single-line comment or "</script" sequence
			if last := buffer[n-1]; last == '/' || (last == '<' && len(e.Value) >= 7 && strings.EqualFold(e.Value[:7], "/script")) {
				p.print(" ")
			}
		}
		p.print(e.Value)

		// Need a space before the next identifier to avoid it turning into flags
		p.prevRegExpEnd = len(p.js)

	case *js_ast.EInlinedEnum:
		p.printExpr(e.Value, level, flags)

		if !p.options.MinifyWhitespace && !p.options.MinifyIdentifiers {
			p.print(" /* ")
			p.print(e.Comment)
			p.print(" */")
		}

	case *js_ast.EBigInt:
		p.printSpaceBeforeIdentifier()
		p.print(e.Value)
		p.print("n")

	case *js_ast.ENumber:
		p.printNumber(e.Value, level)

	case *js_ast.EIdentifier:
		name := p.renamer.NameForSymbol(e.Ref)
		wrap := len(p.js) == p.forOfInitStart && (name == "let" ||
			((flags&isFollowedByOf) != 0 && (flags&isInsideForAwait) == 0 && name == "async"))

		if wrap {
			p.print("(")
		}

		p.printSpaceBeforeIdentifier()
		p.printIdentifier(name)

		if wrap {
			p.print(")")
		}

	case *js_ast.EImportIdentifier:
		// Potentially use a property access instead of an identifier
		ref := js_ast.FollowSymbols(p.symbols, e.Ref)
		symbol := p.symbols.Get(ref)

		if symbol.ImportItemStatus == js_ast.ImportItemMissing {
			p.printUndefined(level)
		} else if symbol.NamespaceAlias != nil {
			wrap := p.callTarget == e && e.WasOriginallyIdentifier
			if wrap {
				if p.options.MinifyWhitespace {
					p.print("(0,")
				} else {
					p.print("(0, ")
				}
			}
			p.printSymbol(symbol.NamespaceAlias.NamespaceRef)
			alias := symbol.NamespaceAlias.Alias
			if !e.PreferQuotedKey && p.canPrintIdentifier(alias) {
				p.print(".")
				p.printIdentifier(alias)
			} else {
				p.print("[")
				p.printQuotedUTF8(alias, true /* allowBacktick */)
				p.print("]")
			}
			if wrap {
				p.print(")")
			}
		} else if value := p.options.ConstValues[ref]; value.Kind != js_ast.ConstValueNone {
			// Handle inlined constants
			p.printExpr(js_ast.ConstValueToExpr(expr.Loc, value), level, flags)
		} else {
			p.printSymbol(e.Ref)
		}

	case *js_ast.EAwait:
		wrap := level >= js_ast.LPrefix

		if wrap {
			p.print("(")
		}

		p.printSpaceBeforeIdentifier()
		p.print("await")
		p.printSpace()
		p.printExpr(e.Value, js_ast.LPrefix-1, 0)

		if wrap {
			p.print(")")
		}

	case *js_ast.EYield:
		wrap := level >= js_ast.LAssign

		if wrap {
			p.print("(")
		}

		p.printSpaceBeforeIdentifier()
		p.print("yield")

		if e.ValueOrNil.Data != nil {
			if e.IsStar {
				p.print("*")
			}
			p.printSpace()
			p.printExpr(e.ValueOrNil, js_ast.LYield, 0)
		}

		if wrap {
			p.print(")")
		}

	case *js_ast.EUnary:
		entry := js_ast.OpTable[e.Op]
		wrap := level >= entry.Level

		if wrap {
			p.print("(")
		}

		if !e.Op.IsPrefix() {
			p.printExpr(e.Value, js_ast.LPostfix-1, parentWasUnaryOrBinary)
		}

		if entry.IsKeyword {
			p.printSpaceBeforeIdentifier()
			p.print(entry.Text)
			p.printSpace()
		} else {
			p.printSpaceBeforeOperator(e.Op)
			p.print(entry.Text)
			p.prevOp = e.Op
			p.prevOpEnd = len(p.js)
		}

		if e.Op.IsPrefix() {
			valueFlags := parentWasUnaryOrBinary
			if e.Op == js_ast.UnOpDelete {
				valueFlags |= isDeleteTarget
			}
			p.printExpr(e.Value, js_ast.LPrefix-1, valueFlags)
		}

		if wrap {
			p.print(")")
		}

	case *js_ast.EBinary:
		// If this is a comma operator then either the result is unused (and we
		// should have already simplified unused expressions), or the result is used
		// (and we can still simplify unused expressions inside the left operand)
		if e.Op == js_ast.BinOpComma {
			if (flags & didAlreadySimplifyUnusedExprs) == 0 {
				left := p.simplifyUnusedExpr(e.Left)
				right := e.Right
				if (flags & exprResultIsUnused) != 0 {
					right = p.simplifyUnusedExpr(right)
				}
				if left.Data != e.Left.Data || right.Data != e.Right.Data {
					// Pass a flag so we don't needlessly re-simplify the same expression
					p.printExpr(p.guardAgainstBehaviorChangeDueToSubstitution(js_ast.JoinWithComma(left, right), flags), level, flags|didAlreadySimplifyUnusedExprs)
					break
				}
			} else {
				// Pass a flag so we don't needlessly re-simplify the same expression
				flags |= didAlreadySimplifyUnusedExprs
			}
		}

		entry := js_ast.OpTable[e.Op]
		wrap := level >= entry.Level || (e.Op == js_ast.BinOpIn && (flags&forbidIn) != 0)

		// Destructuring assignments must be parenthesized
		if n := len(p.js); p.stmtStart == n || p.arrowExprStart == n {
			if _, ok := e.Left.Data.(*js_ast.EObject); ok {
				wrap = true
			}
		}

		if wrap {
			p.print("(")
			flags &= ^forbidIn
		}

		leftLevel := entry.Level - 1
		rightLevel := entry.Level - 1

		if e.Op.IsRightAssociative() {
			leftLevel = entry.Level
		}
		if e.Op.IsLeftAssociative() {
			rightLevel = entry.Level
		}

		switch e.Op {
		case js_ast.BinOpNullishCoalescing:
			// "??" can't directly contain "||" or "&&" without being wrapped in parentheses
			if left, ok := e.Left.Data.(*js_ast.EBinary); ok && (left.Op == js_ast.BinOpLogicalOr || left.Op == js_ast.BinOpLogicalAnd) {
				leftLevel = js_ast.LPrefix
			}
			if right, ok := e.Right.Data.(*js_ast.EBinary); ok && (right.Op == js_ast.BinOpLogicalOr || right.Op == js_ast.BinOpLogicalAnd) {
				rightLevel = js_ast.LPrefix
			}

		case js_ast.BinOpPow:
			// "**" can't contain certain unary expressions
			if left, ok := e.Left.Data.(*js_ast.EUnary); ok && left.Op.UnaryAssignTarget() == js_ast.AssignTargetNone {
				leftLevel = js_ast.LCall
			} else if _, ok := e.Left.Data.(*js_ast.EAwait); ok {
				leftLevel = js_ast.LCall
			} else if _, ok := e.Left.Data.(*js_ast.EUndefined); ok {
				// Undefined is printed as "void 0"
				leftLevel = js_ast.LCall
			} else if _, ok := e.Left.Data.(*js_ast.ENumber); ok {
				// Negative numbers are printed using a unary operator
				leftLevel = js_ast.LCall
			} else if p.options.MinifySyntax {
				// When minifying, booleans are printed as "!0 and "!1"
				if _, ok := e.Left.Data.(*js_ast.EBoolean); ok {
					leftLevel = js_ast.LCall
				}
			}
		}

		// Special-case "#foo in bar"
		if private, ok := e.Left.Data.(*js_ast.EPrivateIdentifier); ok && e.Op == js_ast.BinOpIn {
			p.printSymbol(private.Ref)
		} else if e.Op == js_ast.BinOpComma {
			// The result of the left operand of the comma operator is unused
			p.printExpr(e.Left, leftLevel, (flags&forbidIn)|exprResultIsUnused|parentWasUnaryOrBinary)
		} else {
			p.printExpr(e.Left, leftLevel, (flags&forbidIn)|parentWasUnaryOrBinary)
		}

		if e.Op != js_ast.BinOpComma {
			p.printSpace()
		}

		if entry.IsKeyword {
			p.printSpaceBeforeIdentifier()
			p.print(entry.Text)
		} else {
			p.printSpaceBeforeOperator(e.Op)
			p.print(entry.Text)
			p.prevOp = e.Op
			p.prevOpEnd = len(p.js)
		}

		p.printSpace()

		if e.Op == js_ast.BinOpComma {
			// The result of the right operand of the comma operator is unused if the caller doesn't use it
			p.printExpr(e.Right, rightLevel, (flags&(forbidIn|exprResultIsUnused))|parentWasUnaryOrBinary)
		} else {
			p.printExpr(e.Right, rightLevel, (flags&forbidIn)|parentWasUnaryOrBinary)
		}

		if wrap {
			p.print(")")
		}

	default:
		panic(fmt.Sprintf("Unexpected expression of type %T", expr.Data))
	}
}

func (p *printer) isUnboundEvalIdentifier(value js_ast.Expr) bool {
	if id, ok := value.Data.(*js_ast.EIdentifier); ok {
		// Using the original name here is ok since unbound symbols are not renamed
		symbol := p.symbols.Get(js_ast.FollowSymbols(p.symbols, id.Ref))
		return symbol.Kind == js_ast.SymbolUnbound && symbol.OriginalName == "eval"
	}
	return false
}

// Convert an integer to a byte slice without any allocations
func (p *printer) smallIntToBytes(n int) []byte {
	wasNegative := n < 0
	if wasNegative {
		// This assumes that -math.MinInt isn't a problem. This is fine because
		// these integers are floating-point exponents which never go up that high.
		n = -n
	}

	bytes := p.intToBytesBuffer[:]
	start := len(bytes)

	// Write out the number from the end to the front
	for {
		start--
		bytes[start] = '0' + byte(n%10)
		n /= 10
		if n == 0 {
			break
		}
	}

	// Stick a negative sign on the front if needed
	if wasNegative {
		start--
		bytes[start] = '-'
	}

	return bytes[start:]
}

func parseSmallInt(bytes []byte) int {
	wasNegative := bytes[0] == '-'
	if wasNegative {
		bytes = bytes[1:]
	}

	// Parse the integer without any error checking. This doesn't need to handle
	// integer overflow because these integers are floating-point exponents which
	// never go up that high.
	n := 0
	for _, c := range bytes {
		n = n*10 + int(c-'0')
	}

	if wasNegative {
		return -n
	}
	return n
}

func (p *printer) printNonNegativeFloat(absValue float64) {
	// We can avoid the slow call to strconv.FormatFloat() for integers less than
	// 1000 because we know that exponential notation will always be longer than
	// the integer representation. This is not the case for 1000 which is "1e3".
	if absValue < 1000 {
		if asInt := int64(absValue); absValue == float64(asInt) {
			p.printBytes(p.smallIntToBytes(int(asInt)))
			return
		}
	}

	// Format this number into a byte slice so we can mutate it in place without
	// further reallocation
	result := []byte(strconv.FormatFloat(absValue, 'g', -1, 64))

	// Simplify the exponent
	// "e+05" => "e5"
	// "e-05" => "e-5"
	if e := bytes.LastIndexByte(result, 'e'); e != -1 {
		from := e + 1
		to := from

		switch result[from] {
		case '+':
			// Strip off the leading "+"
			from++

		case '-':
			// Skip past the leading "-"
			to++
			from++
		}

		// Strip off leading zeros
		for from < len(result) && result[from] == '0' {
			from++
		}

		result = append(result[:to], result[from:]...)
	}

	dot := bytes.IndexByte(result, '.')

	if dot == 1 && result[0] == '0' {
		// Simplify numbers starting with "0."
		afterDot := 2

		// Strip off the leading zero when minifying
		// "0.5" => ".5"
		if p.options.MinifyWhitespace {
			result = result[1:]
			afterDot--
		}

		// Try using an exponent
		// "0.001" => "1e-3"
		if result[afterDot] == '0' {
			i := afterDot + 1
			for result[i] == '0' {
				i++
			}
			remaining := result[i:]
			exponent := p.smallIntToBytes(afterDot - i - len(remaining))

			// Only switch if it's actually shorter
			if len(result) > len(remaining)+1+len(exponent) {
				result = append(append(remaining, 'e'), exponent...)
			}
		}
	} else if dot != -1 {
		// Try to get rid of a "." and maybe also an "e"
		if e := bytes.LastIndexByte(result, 'e'); e != -1 {
			integer := result[:dot]
			fraction := result[dot+1 : e]
			exponent := parseSmallInt(result[e+1:]) - len(fraction)

			// Handle small exponents by appending zeros instead
			if exponent >= 0 && exponent <= 2 {
				// "1.2e1" => "12"
				// "1.2e2" => "120"
				// "1.2e3" => "1200"
				if len(result) >= len(integer)+len(fraction)+exponent {
					result = append(integer, fraction...)
					for i := 0; i < exponent; i++ {
						result = append(result, '0')
					}
				}
			} else {
				// "1.2e4" => "12e3"
				exponent := p.smallIntToBytes(exponent)
				if len(result) >= len(integer)+len(fraction)+1+len(exponent) {
					result = append(append(append(integer, fraction...), 'e'), exponent...)
				}
			}
		}
	} else if result[len(result)-1] == '0' {
		// Simplify numbers ending with "0" by trying to use an exponent
		// "1000" => "1e3"
		i := len(result) - 1
		for i > 0 && result[i-1] == '0' {
			i--
		}
		remaining := result[:i]
		exponent := p.smallIntToBytes(len(result) - i)

		// Only switch if it's actually shorter
		if len(result) > len(remaining)+1+len(exponent) {
			result = append(append(remaining, 'e'), exponent...)
		}
	}

	p.printBytes(result)
}

func (p *printer) printDeclStmt(isExport bool, keyword string, decls []js_ast.Decl) {
	p.printIndent()
	p.printSpaceBeforeIdentifier()
	if isExport {
		p.print("export ")
	}
	p.printDecls(keyword, decls, 0)
	p.printSemicolonAfterStatement()
}

func (p *printer) printForLoopInit(init js_ast.Stmt, flags printExprFlags) {
	switch s := init.Data.(type) {
	case *js_ast.SExpr:
		p.printExpr(s.Value, js_ast.LLowest, flags|exprResultIsUnused)
	case *js_ast.SLocal:
		switch s.Kind {
		case js_ast.LocalVar:
			p.printDecls("var", s.Decls, flags)
		case js_ast.LocalLet:
			p.printDecls("let", s.Decls, flags)
		case js_ast.LocalConst:
			p.printDecls("const", s.Decls, flags)
		}
	default:
		panic("Internal error")
	}
}

func (p *printer) printDecls(keyword string, decls []js_ast.Decl, flags printExprFlags) {
	p.print(keyword)
	p.printSpace()

	for i, decl := range decls {
		if i != 0 {
			p.print(",")
			p.printSpace()
		}
		p.printBinding(decl.Binding)

		if decl.ValueOrNil.Data != nil {
			p.printSpace()
			p.print("=")
			p.printSpace()
			p.printExpr(decl.ValueOrNil, js_ast.LComma, flags)
		}
	}
}

func (p *printer) printBody(body js_ast.Stmt) {
	if block, ok := body.Data.(*js_ast.SBlock); ok {
		p.printSpace()
		p.printBlock(body.Loc, block.Stmts)
		p.printNewline()
	} else {
		p.printNewline()
		p.options.Indent++
		p.printStmt(body, 0)
		p.options.Indent--
	}
}

func (p *printer) printBlock(loc logger.Loc, stmts []js_ast.Stmt) {
	p.addSourceMapping(loc)
	p.print("{")
	p.printNewline()

	p.options.Indent++
	for _, stmt := range stmts {
		p.printSemicolonIfNeeded()
		p.printStmt(stmt, canOmitStatement)
	}
	p.options.Indent--
	p.needsSemicolon = false

	p.printIndent()
	p.print("}")
}

func wrapToAvoidAmbiguousElse(s js_ast.S) bool {
	for {
		switch current := s.(type) {
		case *js_ast.SIf:
			if current.NoOrNil.Data == nil {
				return true
			}
			s = current.NoOrNil.Data

		case *js_ast.SFor:
			s = current.Body.Data

		case *js_ast.SForIn:
			s = current.Body.Data

		case *js_ast.SForOf:
			s = current.Body.Data

		case *js_ast.SWhile:
			s = current.Body.Data

		case *js_ast.SWith:
			s = current.Body.Data

		default:
			return false
		}
	}
}

func (p *printer) printIf(s *js_ast.SIf) {
	p.printSpaceBeforeIdentifier()
	p.print("if")
	p.printSpace()
	p.print("(")
	p.printExpr(s.Test, js_ast.LLowest, 0)
	p.print(")")

	// Simplify the else branch, which may disappear entirely
	no := s.NoOrNil
	if expr, ok := no.Data.(*js_ast.SExpr); ok {
		if value := p.simplifyUnusedExpr(expr.Value); value.Data == nil {
			no.Data = nil
		} else if value.Data != expr.Value.Data {
			no.Data = &js_ast.SExpr{Value: value}
		}
	}

	if yes, ok := s.Yes.Data.(*js_ast.SBlock); ok {
		p.printSpace()
		p.printBlock(s.Yes.Loc, yes.Stmts)

		if no.Data != nil {
			p.printSpace()
		} else {
			p.printNewline()
		}
	} else if wrapToAvoidAmbiguousElse(s.Yes.Data) {
		p.printSpace()
		p.print("{")
		p.printNewline()

		p.options.Indent++
		p.printStmt(s.Yes, canOmitStatement)
		p.options.Indent--
		p.needsSemicolon = false

		p.printIndent()
		p.print("}")

		if no.Data != nil {
			p.printSpace()
		} else {
			p.printNewline()
		}
	} else {
		p.printNewline()
		p.options.Indent++
		p.printStmt(s.Yes, 0)
		p.options.Indent--

		if no.Data != nil {
			p.printIndent()
		}
	}

	if no.Data != nil {
		p.printSemicolonIfNeeded()
		p.printSpaceBeforeIdentifier()
		p.print("else")

		if block, ok := no.Data.(*js_ast.SBlock); ok {
			p.printSpace()
			p.printBlock(no.Loc, block.Stmts)
			p.printNewline()
		} else if ifStmt, ok := no.Data.(*js_ast.SIf); ok {
			p.printIf(ifStmt)
		} else {
			p.printNewline()
			p.options.Indent++
			p.printStmt(no, 0)
			p.options.Indent--
		}
	}
}

func (p *printer) printIndentedComment(text string) {
	// Avoid generating a comment containing the character sequence "</script"
	text = helpers.EscapeClosingTag(text, "/script")

	if strings.HasPrefix(text, "/*") {
		// Re-indent multi-line comments
		for {
			newline := strings.IndexByte(text, '\n')
			if newline == -1 {
				break
			}
			p.printIndent()
			p.print(text[:newline+1])
			text = text[newline+1:]
		}
		p.printIndent()
		p.print(text)
		p.printNewline()
	} else {
		// Print a mandatory newline after single-line comments
		p.printIndent()
		p.print(text)
		p.print("\n")
	}
}

func (p *printer) printPath(importRecordIndex uint32) {
	record := p.importRecords[importRecordIndex]
	p.printQuotedUTF8(record.Path.Text, false /* allowBacktick */)

	// Just omit import assertions if they aren't supported
	if p.options.UnsupportedFeatures.Has(compat.ImportAssertions) {
		return
	}

	if record.Assertions != nil {
		p.printSpace()
		p.print("assert")
		p.printSpace()
		p.printImportAssertionsClause(*record.Assertions)
	}
}

func (p *printer) printImportCallAssertions(assertions *[]ast.AssertEntry) {
	// Just omit import assertions if they aren't supported
	if p.options.UnsupportedFeatures.Has(compat.ImportAssertions) {
		return
	}

	if assertions != nil {
		p.print(",")
		p.printSpace()
		p.print("{")
		p.printSpace()
		p.print("assert:")
		p.printSpace()
		p.printImportAssertionsClause(*assertions)
		p.printSpace()
		p.print("}")
	}
}

func (p *printer) printImportAssertionsClause(assertions []ast.AssertEntry) {
	p.print("{")

	for i, entry := range assertions {
		if i > 0 {
			p.print(",")
		}

		p.printSpace()
		p.addSourceMapping(entry.KeyLoc)
		if !entry.PreferQuotedKey && p.canPrintIdentifierUTF16(entry.Key) {
			p.printSpaceBeforeIdentifier()
			p.printIdentifierUTF16(entry.Key)
		} else {
			p.printQuotedUTF16(entry.Key, false /* allowBacktick */)
		}

		p.print(":")
		p.printSpace()

		p.addSourceMapping(entry.ValueLoc)
		p.printQuotedUTF16(entry.Value, false /* allowBacktick */)
	}

	if len(assertions) > 0 {
		p.printSpace()
	}
	p.print("}")
}

type printStmtFlags uint8

const (
	canOmitStatement printStmtFlags = 1 << iota
)

func (p *printer) printStmt(stmt js_ast.Stmt, flags printStmtFlags) {
	p.addSourceMapping(stmt.Loc)

	switch s := stmt.Data.(type) {
	case *js_ast.SComment:
		text := s.Text

		if s.IsLegalComment {
			switch p.options.LegalComments {
			case config.LegalCommentsNone:
				return

			case config.LegalCommentsEndOfFile,
				config.LegalCommentsLinkedWithComment,
				config.LegalCommentsExternalWithoutComment:
				if p.extractedLegalComments == nil {
					p.extractedLegalComments = make(map[string]bool)
				}
				p.extractedLegalComments[text] = true
				return
			}
		}

		p.printIndentedComment(text)

	case *js_ast.SFunction:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		if s.IsExport {
			p.print("export ")
		}
		if s.Fn.IsAsync {
			p.print("async ")
		}
		p.print("function")
		if s.Fn.IsGenerator {
			p.print("*")
			p.printSpace()
		}
		p.printSymbol(s.Fn.Name.Ref)
		p.printFn(s.Fn)
		p.printNewline()

	case *js_ast.SClass:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		if s.IsExport {
			p.print("export ")
		}
		p.print("class")
		p.printSymbol(s.Class.Name.Ref)
		p.printClass(s.Class)
		p.printNewline()

	case *js_ast.SEmpty:
		p.printIndent()
		p.print(";")
		p.printNewline()

	case *js_ast.SExportDefault:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("export default")
		p.printSpace()

		switch s2 := s.Value.Data.(type) {
		case *js_ast.SExpr:
			// Functions and classes must be wrapped to avoid confusion with their statement forms
			p.exportDefaultStart = len(p.js)

			p.printExpr(s2.Value, js_ast.LComma, 0)
			p.printSemicolonAfterStatement()
			return

		case *js_ast.SFunction:
			p.printSpaceBeforeIdentifier()
			if s2.Fn.IsAsync {
				p.print("async ")
			}
			p.print("function")
			if s2.Fn.IsGenerator {
				p.print("*")
				p.printSpace()
			}
			if s2.Fn.Name != nil {
				p.printSymbol(s2.Fn.Name.Ref)
			}
			p.printFn(s2.Fn)
			p.printNewline()

		case *js_ast.SClass:
			p.printSpaceBeforeIdentifier()
			p.print("class")
			if s2.Class.Name != nil {
				p.printSymbol(s2.Class.Name.Ref)
			}
			p.printClass(s2.Class)
			p.printNewline()

		default:
			panic("Internal error")
		}

	case *js_ast.SExportStar:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("export")
		p.printSpace()
		p.print("*")
		p.printSpace()
		if s.Alias != nil {
			p.print("as")
			p.printSpace()
			p.printClauseAlias(s.Alias.OriginalName)
			p.printSpace()
			p.printSpaceBeforeIdentifier()
		}
		p.print("from")
		p.printSpace()
		p.printPath(s.ImportRecordIndex)
		p.printSemicolonAfterStatement()

	case *js_ast.SExportClause:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("export")
		p.printSpace()
		p.print("{")

		if !s.IsSingleLine {
			p.options.Indent++
		}

		for i, item := range s.Items {
			if i != 0 {
				p.print(",")
			}

			if s.IsSingleLine {
				p.printSpace()
			} else {
				p.printNewline()
				p.printIndent()
			}

			name := p.renamer.NameForSymbol(item.Name.Ref)
			p.printIdentifier(name)
			if name != item.Alias {
				p.print(" as")
				p.printSpace()
				p.printClauseAlias(item.Alias)
			}
		}

		if !s.IsSingleLine {
			p.options.Indent--
			p.printNewline()
			p.printIndent()
		} else if len(s.Items) > 0 {
			p.printSpace()
		}

		p.print("}")
		p.printSemicolonAfterStatement()

	case *js_ast.SExportFrom:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("export")
		p.printSpace()
		p.print("{")

		if !s.IsSingleLine {
			p.options.Indent++
		}

		for i, item := range s.Items {
			if i != 0 {
				p.print(",")
			}

			if s.IsSingleLine {
				p.printSpace()
			} else {
				p.printNewline()
				p.printIndent()
			}

			p.printClauseAlias(item.OriginalName)
			if item.OriginalName != item.Alias {
				p.printSpace()
				p.printSpaceBeforeIdentifier()
				p.print("as")
				p.printSpace()
				p.printClauseAlias(item.Alias)
			}
		}

		if !s.IsSingleLine {
			p.options.Indent--
			p.printNewline()
			p.printIndent()
		} else if len(s.Items) > 0 {
			p.printSpace()
		}

		p.print("}")
		p.printSpace()
		p.print("from")
		p.printSpace()
		p.printPath(s.ImportRecordIndex)
		p.printSemicolonAfterStatement()

	case *js_ast.SLocal:
		switch s.Kind {
		case js_ast.LocalConst:
			p.printDeclStmt(s.IsExport, "const", s.Decls)
		case js_ast.LocalLet:
			p.printDeclStmt(s.IsExport, "let", s.Decls)
		case js_ast.LocalVar:
			p.printDeclStmt(s.IsExport, "var", s.Decls)
		}

	case *js_ast.SIf:
		p.printIndent()
		p.printIf(s)

	case *js_ast.SDoWhile:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("do")
		if block, ok := s.Body.Data.(*js_ast.SBlock); ok {
			p.printSpace()
			p.printBlock(s.Body.Loc, block.Stmts)
			p.printSpace()
		} else {
			p.printNewline()
			p.options.Indent++
			p.printStmt(s.Body, 0)
			p.printSemicolonIfNeeded()
			p.options.Indent--
			p.printIndent()
		}
		p.print("while")
		p.printSpace()
		p.print("(")
		p.printExpr(s.Test, js_ast.LLowest, 0)
		p.print(")")
		p.printSemicolonAfterStatement()

	case *js_ast.SForIn:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("for")
		p.printSpace()
		p.print("(")
		p.printForLoopInit(s.Init, forbidIn)
		p.printSpace()
		p.printSpaceBeforeIdentifier()
		p.print("in")
		p.printSpace()
		p.printExpr(s.Value, js_ast.LLowest, 0)
		p.print(")")
		p.printBody(s.Body)

	case *js_ast.SForOf:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("for")
		if s.IsAwait {
			p.print(" await")
		}
		p.printSpace()
		p.print("(")
		p.forOfInitStart = len(p.js)
		flags := forbidIn | isFollowedByOf
		if s.IsAwait {
			flags |= isInsideForAwait
		}
		p.printForLoopInit(s.Init, flags)
		p.printSpace()
		p.printSpaceBeforeIdentifier()
		p.print("of")
		p.printSpace()
		p.printExpr(s.Value, js_ast.LComma, 0)
		p.print(")")
		p.printBody(s.Body)

	case *js_ast.SWhile:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("while")
		p.printSpace()
		p.print("(")
		p.printExpr(s.Test, js_ast.LLowest, 0)
		p.print(")")
		p.printBody(s.Body)

	case *js_ast.SWith:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("with")
		p.printSpace()
		p.print("(")
		p.printExpr(s.Value, js_ast.LLowest, 0)
		p.print(")")
		p.printBody(s.Body)

	case *js_ast.SLabel:
		p.printIndent()
		p.printSymbol(s.Name.Ref)
		p.print(":")
		p.printBody(s.Stmt)

	case *js_ast.STry:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("try")
		p.printSpace()
		p.printBlock(s.BodyLoc, s.Body)

		if s.Catch != nil {
			p.printSpace()
			p.print("catch")
			if s.Catch.BindingOrNil.Data != nil {
				p.printSpace()
				p.print("(")
				p.printBinding(s.Catch.BindingOrNil)
				p.print(")")
			}
			p.printSpace()
			p.printBlock(s.Catch.Loc, s.Catch.Body)
		}

		if s.Finally != nil {
			p.printSpace()
			p.print("finally")
			p.printSpace()
			p.printBlock(s.Finally.Loc, s.Finally.Stmts)
		}

		p.printNewline()

	case *js_ast.SFor:
		init := s.InitOrNil
		update := s.UpdateOrNil

		// Omit calls to empty functions from the output completely
		if p.options.MinifySyntax {
			if expr, ok := init.Data.(*js_ast.SExpr); ok {
				if value := p.simplifyUnusedExpr(expr.Value); value.Data == nil {
					init.Data = nil
				} else if value.Data != expr.Value.Data {
					init.Data = &js_ast.SExpr{Value: value}
				}
			}
			if update.Data != nil {
				update = p.simplifyUnusedExpr(update)
			}
		}

		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("for")
		p.printSpace()
		p.print("(")
		if init.Data != nil {
			p.printForLoopInit(init, forbidIn)
		}
		p.print(";")
		p.printSpace()
		if s.TestOrNil.Data != nil {
			p.printExpr(s.TestOrNil, js_ast.LLowest, 0)
		}
		p.print(";")
		p.printSpace()
		if update.Data != nil {
			p.printExpr(update, js_ast.LLowest, exprResultIsUnused)
		}
		p.print(")")
		p.printBody(s.Body)

	case *js_ast.SSwitch:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("switch")
		p.printSpace()
		p.print("(")
		p.printExpr(s.Test, js_ast.LLowest, 0)
		p.print(")")
		p.printSpace()
		p.print("{")
		p.printNewline()
		p.options.Indent++

		for _, c := range s.Cases {
			p.printSemicolonIfNeeded()
			p.printIndent()

			if c.ValueOrNil.Data != nil {
				p.print("case")
				p.printSpace()
				p.printExpr(c.ValueOrNil, js_ast.LLogicalAnd, 0)
			} else {
				p.print("default")
			}
			p.print(":")

			if len(c.Body) == 1 {
				if block, ok := c.Body[0].Data.(*js_ast.SBlock); ok {
					p.printSpace()
					p.printBlock(c.Body[0].Loc, block.Stmts)
					p.printNewline()
					continue
				}
			}

			p.printNewline()
			p.options.Indent++
			for _, stmt := range c.Body {
				p.printSemicolonIfNeeded()
				p.printStmt(stmt, canOmitStatement)
			}
			p.options.Indent--
		}

		p.options.Indent--
		p.printIndent()
		p.print("}")
		p.printNewline()
		p.needsSemicolon = false

	case *js_ast.SImport:
		itemCount := 0

		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("import")
		p.printSpace()

		if s.DefaultName != nil {
			p.printSymbol(s.DefaultName.Ref)
			itemCount++
		}

		if s.Items != nil {
			if itemCount > 0 {
				p.print(",")
				p.printSpace()
			}

			p.print("{")
			if !s.IsSingleLine {
				p.options.Indent++
			}

			for i, item := range *s.Items {
				if i != 0 {
					p.print(",")
				}

				if s.IsSingleLine {
					p.printSpace()
				} else {
					p.printNewline()
					p.printIndent()
				}

				p.printClauseAlias(item.Alias)
				name := p.renamer.NameForSymbol(item.Name.Ref)
				if name != item.Alias {
					p.printSpace()
					p.printSpaceBeforeIdentifier()
					p.print("as ")
					p.printIdentifier(name)
				}
			}

			if !s.IsSingleLine {
				p.options.Indent--
				p.printNewline()
				p.printIndent()
			} else if len(*s.Items) > 0 {
				p.printSpace()
			}

			p.print("}")
			itemCount++
		}

		if s.StarNameLoc != nil {
			if itemCount > 0 {
				p.print(",")
				p.printSpace()
			}

			p.print("*")
			p.printSpace()
			p.print("as ")
			p.printSymbol(s.NamespaceRef)
			itemCount++
		}

		if itemCount > 0 {
			p.printSpace()
			p.printSpaceBeforeIdentifier()
			p.print("from")
			p.printSpace()
		}

		p.printPath(s.ImportRecordIndex)
		p.printSemicolonAfterStatement()

	case *js_ast.SBlock:
		p.printIndent()
		p.printBlock(stmt.Loc, s.Stmts)
		p.printNewline()

	case *js_ast.SDebugger:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("debugger")
		p.printSemicolonAfterStatement()

	case *js_ast.SDirective:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.printQuotedUTF16(s.Value, false /* allowBacktick */)
		p.printSemicolonAfterStatement()

	case *js_ast.SBreak:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("break")
		if s.Label != nil {
			p.print(" ")
			p.printSymbol(s.Label.Ref)
		}
		p.printSemicolonAfterStatement()

	case *js_ast.SContinue:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("continue")
		if s.Label != nil {
			p.print(" ")
			p.printSymbol(s.Label.Ref)
		}
		p.printSemicolonAfterStatement()

	case *js_ast.SReturn:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("return")
		if s.ValueOrNil.Data != nil {
			p.printSpace()
			p.printExpr(s.ValueOrNil, js_ast.LLowest, 0)
		}
		p.printSemicolonAfterStatement()

	case *js_ast.SThrow:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("throw")
		p.printSpace()
		p.printExpr(s.Value, js_ast.LLowest, 0)
		p.printSemicolonAfterStatement()

	case *js_ast.SExpr:
		value := s.Value

		// Omit calls to empty functions from the output completely
		if p.options.MinifySyntax {
			value = p.simplifyUnusedExpr(value)
			if value.Data == nil {
				// If this statement is not in a block, then we still need to emit something
				if (flags & canOmitStatement) == 0 {
					// "if (x) empty();" => "if (x) ;"
					p.printIndent()
					p.print(";")
					p.printNewline()
				} else {
					// "if (x) { empty(); }" => "if (x) {}"
				}
				break
			}
		}

		p.printIndent()
		p.stmtStart = len(p.js)
		p.printExpr(value, js_ast.LLowest, exprResultIsUnused)
		p.printSemicolonAfterStatement()

	default:
		panic(fmt.Sprintf("Unexpected statement of type %T", stmt.Data))
	}
}

type Options struct {
	RequireOrImportMetaForSource func(uint32) RequireOrImportMeta

	// Cross-module inlining of TypeScript enums is actually done during printing
	TSEnums map[js_ast.Ref]map[string]js_ast.TSEnumValue

	// Cross-module inlining of detected inlinable constants is also done during printing
	ConstValues map[js_ast.Ref]js_ast.ConstValue

	// This will be present if the input file had a source map. In that case we
	// want to map all the way back to the original input file(s).
	InputSourceMap *sourcemap.SourceMap

	// If we're writing out a source map, this table of line start indices lets
	// us do binary search on to figure out what line a given AST node came from
	LineOffsetTables []sourcemap.LineOffsetTable

	ToCommonJSRef       js_ast.Ref
	ToESMRef            js_ast.Ref
	RuntimeRequireRef   js_ast.Ref
	UnsupportedFeatures compat.JSFeature
	Indent              int
	OutputFormat        config.Format
	MinifyWhitespace    bool
	MinifyIdentifiers   bool
	MinifySyntax        bool
	ASCIIOnly           bool
	LegalComments       config.LegalComments
	AddSourceMappings   bool
}

type RequireOrImportMeta struct {
	// CommonJS files will return the "require_*" wrapper function and an invalid
	// exports object reference. Lazily-initialized ESM files will return the
	// "init_*" wrapper function and the exports object for that file.
	WrapperRef     js_ast.Ref
	ExportsRef     js_ast.Ref
	IsWrapperAsync bool
}

type PrintResult struct {
	JS                     []byte
	ExtractedLegalComments map[string]bool

	// This source map chunk just contains the VLQ-encoded offsets for the "JS"
	// field above. It's not a full source map. The bundler will be joining many
	// source map chunks together to form the final source map.
	SourceMapChunk sourcemap.Chunk
}

func Print(tree js_ast.AST, symbols js_ast.SymbolMap, r renamer.Renamer, options Options) PrintResult {
	p := &printer{
		symbols:            symbols,
		renamer:            r,
		importRecords:      tree.ImportRecords,
		options:            options,
		moduleType:         tree.ModuleTypeData.Type,
		stmtStart:          -1,
		exportDefaultStart: -1,
		arrowExprStart:     -1,
		forOfInitStart:     -1,
		prevOpEnd:          -1,
		prevNumEnd:         -1,
		prevRegExpEnd:      -1,
		builder:            sourcemap.MakeChunkBuilder(options.InputSourceMap, options.LineOffsetTables),
	}

	p.isUnbound = func(ref js_ast.Ref) bool {
		ref = js_ast.FollowSymbols(symbols, ref)
		return symbols.Get(ref).Kind == js_ast.SymbolUnbound
	}

	// Add the top-level directive if present
	if tree.Directive != "" {
		p.printIndent()
		p.printQuotedUTF8(tree.Directive, options.ASCIIOnly)
		p.print(";")
		p.printNewline()
	}

	for _, part := range tree.Parts {
		for _, stmt := range part.Stmts {
			p.printStmt(stmt, canOmitStatement)
			p.printSemicolonIfNeeded()
		}
	}

	return PrintResult{
		JS:                     p.js,
		ExtractedLegalComments: p.extractedLegalComments,
		SourceMapChunk:         p.builder.GenerateChunk(p.js),
	}
}
