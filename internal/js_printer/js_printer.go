package js_printer

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/js_ast"
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

func (p *printer) printUnquotedUTF16(text []uint16, quote rune, flags printQuotedFlags) {
	temp := make([]byte, utf8.UTFMax)
	js := p.js
	i := 0
	n := len(text)

	// Only compute the line length if necessary
	var startLineLength int
	wrapLongLines := false
	if p.options.LineLimit > 0 && (flags&printQuotedNoWrap) == 0 {
		startLineLength = p.currentLineLength()
		if startLineLength > p.options.LineLimit {
			startLineLength = p.options.LineLimit
		}
		wrapLongLines = true
	}

	for i < n {
		// Wrap long lines that are over the limit using escaped newlines
		if wrapLongLines && startLineLength+i >= p.options.LineLimit {
			js = append(js, "\\\n"...)
			startLineLength -= p.options.LineLimit
		}

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
				startLineLength = -i // Printing a real newline resets the line length
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
			if !p.options.UnsupportedFeatures.Has(compat.InlineScript) && i >= 2 && text[i-2] == '<' && i+6 <= len(text) {
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

// JSX tag syntax doesn't support character escapes so non-ASCII identifiers
// must be printed as UTF-8 even when the charset is set to ASCII.
func (p *printer) printJSXTag(tagOrNil js_ast.Expr) {
	switch e := tagOrNil.Data.(type) {
	case *js_ast.EString:
		p.addSourceMapping(tagOrNil.Loc)
		p.print(helpers.UTF16ToString(e.Value))

	case *js_ast.EIdentifier:
		name := p.renamer.NameForSymbol(e.Ref)
		p.addSourceMappingForName(tagOrNil.Loc, name, e.Ref)
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
	symbols                ast.SymbolMap
	astHelpers             js_ast.HelperContext
	renamer                renamer.Renamer
	importRecords          []ast.ImportRecord
	callTarget             js_ast.E
	exprComments           map[logger.Loc][]string
	printedExprComments    map[logger.Loc]bool
	hasLegalComment        map[string]struct{}
	extractedLegalComments []string
	js                     []byte
	jsonMetadataImports    []string
	binaryExprStack        []binaryExprVisitor
	options                Options
	builder                sourcemap.ChunkBuilder
	printNextIndentAsSpace bool

	stmtStart          int
	exportDefaultStart int
	arrowExprStart     int
	forOfInitStart     int

	withNesting          int
	prevOpEnd            int
	needSpaceBeforeDot   int
	prevRegExpEnd        int
	noLeadingNewlineHere int
	oldLineStart         int
	oldLineEnd           int
	intToBytesBuffer     [64]byte
	needsSemicolon       bool
	wasLazyExport        bool
	prevOp               js_ast.OpCode
	moduleType           js_ast.ModuleType
}

func (p *printer) print(text string) {
	p.js = append(p.js, text...)
}

// This is the same as "print(string(bytes))" without any unnecessary temporary
// allocations
func (p *printer) printBytes(bytes []byte) {
	p.js = append(p.js, bytes...)
}

type printQuotedFlags uint8

const (
	printQuotedAllowBacktick printQuotedFlags = 1 << iota
	printQuotedNoWrap
)

func (p *printer) printQuotedUTF8(text string, flags printQuotedFlags) {
	p.printQuotedUTF16(helpers.StringToUTF16(text), flags)
}

func (p *printer) addSourceMapping(loc logger.Loc) {
	if p.options.AddSourceMappings {
		p.builder.AddSourceMapping(loc, "", p.js)
	}
}

func (p *printer) addSourceMappingForName(loc logger.Loc, name string, ref ast.Ref) {
	if p.options.AddSourceMappings {
		if originalName := p.symbols.Get(ast.FollowSymbols(p.symbols, ref)).OriginalName; originalName != name {
			p.builder.AddSourceMapping(loc, originalName, p.js)
		} else {
			p.builder.AddSourceMapping(loc, "", p.js)
		}
	}
}

func (p *printer) printIndent() {
	if p.options.MinifyWhitespace {
		return
	}

	if p.printNextIndentAsSpace {
		p.print(" ")
		p.printNextIndentAsSpace = false
		return
	}

	indent := p.options.Indent
	if p.options.LineLimit > 0 && indent*2 >= p.options.LineLimit {
		indent = p.options.LineLimit / 2
	}
	for i := 0; i < indent; i++ {
		p.print("  ")
	}
}

func (p *printer) mangledPropName(ref ast.Ref) string {
	ref = ast.FollowSymbols(p.symbols, ref)
	if name, ok := p.options.MangledProps[ref]; ok {
		return name
	}
	return p.renamer.NameForSymbol(ref)
}

func (p *printer) tryToGetImportedEnumValue(target js_ast.Expr, name string) (js_ast.TSEnumValue, bool) {
	if id, ok := target.Data.(*js_ast.EImportIdentifier); ok {
		ref := ast.FollowSymbols(p.symbols, id.Ref)
		if symbol := p.symbols.Get(ref); symbol.Kind == ast.SymbolTSEnum {
			if enum, ok := p.options.TSEnums[ref]; ok {
				value, ok := enum[name]
				return value, ok
			}
		}
	}
	return js_ast.TSEnumValue{}, false
}

func (p *printer) tryToGetImportedEnumValueUTF16(target js_ast.Expr, name []uint16) (js_ast.TSEnumValue, string, bool) {
	if id, ok := target.Data.(*js_ast.EImportIdentifier); ok {
		ref := ast.FollowSymbols(p.symbols, id.Ref)
		if symbol := p.symbols.Get(ref); symbol.Kind == ast.SymbolTSEnum {
			if enum, ok := p.options.TSEnums[ref]; ok {
				name := helpers.UTF16ToString(name)
				value, ok := enum[name]
				return value, name, ok
			}
		}
	}
	return js_ast.TSEnumValue{}, "", false
}

func (p *printer) printClauseAlias(loc logger.Loc, alias string) {
	if js_ast.IsIdentifier(alias) {
		p.printSpaceBeforeIdentifier()
		p.addSourceMapping(loc)
		p.printIdentifier(alias)
	} else {
		p.addSourceMapping(loc)
		p.printQuotedUTF8(alias, 0)
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

func CanEscapeIdentifier(name string, UnsupportedFeatures compat.JSFeature, asciiOnly bool) bool {
	return js_ast.IsIdentifierES5AndESNext(name) && (!asciiOnly ||
		!UnsupportedFeatures.Has(compat.UnicodeEscapes) ||
		!helpers.ContainsNonBMPCodePoint(name))
}

func (p *printer) canPrintIdentifier(name string) bool {
	return js_ast.IsIdentifierES5AndESNext(name) && (!p.options.ASCIIOnly ||
		!p.options.UnsupportedFeatures.Has(compat.UnicodeEscapes) ||
		!helpers.ContainsNonBMPCodePoint(name))
}

func (p *printer) canPrintIdentifierUTF16(name []uint16) bool {
	return js_ast.IsIdentifierES5AndESNextUTF16(name) && (!p.options.ASCIIOnly ||
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
		if p.withNesting != 0 {
			// "with (x) NaN" really means "x.NaN" so avoid identifiers when "with" is present
			wrap := level >= js_ast.LMultiply
			if wrap {
				p.print("(")
			}
			if p.options.MinifyWhitespace {
				p.print("0/0")
			} else {
				p.print("0 / 0")
			}
			if wrap {
				p.print(")")
			}
		} else {
			p.print("NaN")
		}
	} else if value == positiveInfinity || value == negativeInfinity {
		// "with (x) Infinity" really means "x.Infinity" so avoid identifiers when "with" is present
		wrap := ((p.options.MinifySyntax || p.withNesting != 0) && level >= js_ast.LMultiply) ||
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
		if !p.options.MinifySyntax && p.withNesting == 0 {
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
		}
	}
}

func (p *printer) willPrintExprCommentsAtLoc(loc logger.Loc) bool {
	return !p.options.MinifyWhitespace && p.exprComments[loc] != nil && !p.printedExprComments[loc]
}

func (p *printer) willPrintExprCommentsForAnyOf(exprs []js_ast.Expr) bool {
	for _, expr := range exprs {
		if p.willPrintExprCommentsAtLoc(expr.Loc) {
			return true
		}
	}
	return false
}

func (p *printer) printBinding(binding js_ast.Binding) {
	switch b := binding.Data.(type) {
	case *js_ast.BMissing:
		p.addSourceMapping(binding.Loc)

	case *js_ast.BIdentifier:
		name := p.renamer.NameForSymbol(b.Ref)
		p.printSpaceBeforeIdentifier()
		p.addSourceMappingForName(binding.Loc, name, b.Ref)
		p.printIdentifier(name)

	case *js_ast.BArray:
		isMultiLine := (len(b.Items) > 0 && !b.IsSingleLine) || p.willPrintExprCommentsAtLoc(b.CloseBracketLoc)
		if !p.options.MinifyWhitespace && !isMultiLine {
			for _, item := range b.Items {
				if p.willPrintExprCommentsAtLoc(item.Loc) {
					isMultiLine = true
					break
				}
			}
		}
		p.addSourceMapping(binding.Loc)
		p.print("[")
		if len(b.Items) > 0 || isMultiLine {
			if isMultiLine {
				p.options.Indent++
			}

			for i, item := range b.Items {
				if i != 0 {
					p.print(",")
				}
				if p.options.LineLimit <= 0 || !p.printNewlinePastLineLimit() {
					if isMultiLine {
						p.printNewline()
						p.printIndent()
					} else if i != 0 {
						p.printSpace()
					}
				}
				p.printExprCommentsAtLoc(item.Loc)
				if b.HasSpread && i+1 == len(b.Items) {
					p.addSourceMapping(item.Loc)
					p.print("...")
					p.printExprCommentsAtLoc(item.Binding.Loc)
				}
				p.printBinding(item.Binding)

				if item.DefaultValueOrNil.Data != nil {
					p.printSpace()
					p.print("=")
					p.printSpace()
					p.printExprWithoutLeadingNewline(item.DefaultValueOrNil, js_ast.LComma, 0)
				}

				// Make sure there's a comma after trailing missing items
				if _, ok := item.Binding.Data.(*js_ast.BMissing); ok && i == len(b.Items)-1 {
					p.print(",")
				}
			}

			if isMultiLine {
				p.printNewline()
				p.printExprCommentsAfterCloseTokenAtLoc(b.CloseBracketLoc)
				p.options.Indent--
				p.printIndent()
			}
		}
		p.addSourceMapping(b.CloseBracketLoc)
		p.print("]")

	case *js_ast.BObject:
		isMultiLine := (len(b.Properties) > 0 && !b.IsSingleLine) || p.willPrintExprCommentsAtLoc(b.CloseBraceLoc)
		if !p.options.MinifyWhitespace && !isMultiLine {
			for _, property := range b.Properties {
				if p.willPrintExprCommentsAtLoc(property.Loc) {
					isMultiLine = true
					break
				}
			}
		}
		p.addSourceMapping(binding.Loc)
		p.print("{")
		if len(b.Properties) > 0 || isMultiLine {
			if isMultiLine {
				p.options.Indent++
			}

			for i, property := range b.Properties {
				if i != 0 {
					p.print(",")
				}
				if p.options.LineLimit <= 0 || !p.printNewlinePastLineLimit() {
					if isMultiLine {
						p.printNewline()
						p.printIndent()
					} else {
						p.printSpace()
					}
				}

				p.printExprCommentsAtLoc(property.Loc)

				if property.IsSpread {
					p.addSourceMapping(property.Loc)
					p.print("...")
					p.printExprCommentsAtLoc(property.Value.Loc)
				} else {
					if property.IsComputed {
						p.addSourceMapping(property.Loc)
						isMultiLine := p.willPrintExprCommentsAtLoc(property.Key.Loc) || p.willPrintExprCommentsAtLoc(property.CloseBracketLoc)
						p.print("[")
						if isMultiLine {
							p.printNewline()
							p.options.Indent++
							p.printIndent()
						}
						p.printExpr(property.Key, js_ast.LComma, 0)
						if isMultiLine {
							p.printNewline()
							p.printExprCommentsAfterCloseTokenAtLoc(property.CloseBracketLoc)
							p.options.Indent--
							p.printIndent()
						}
						if property.CloseBracketLoc.Start > property.Loc.Start {
							p.addSourceMapping(property.CloseBracketLoc)
						}
						p.print("]:")
						p.printSpace()
						p.printBinding(property.Value)

						if property.DefaultValueOrNil.Data != nil {
							p.printSpace()
							p.print("=")
							p.printSpace()
							p.printExprWithoutLeadingNewline(property.DefaultValueOrNil, js_ast.LComma, 0)
						}
						continue
					}

					if str, ok := property.Key.Data.(*js_ast.EString); ok && !property.PreferQuotedKey && p.canPrintIdentifierUTF16(str.Value) {
						// Use a shorthand property if the names are the same
						if id, ok := property.Value.Data.(*js_ast.BIdentifier); ok &&
							!p.willPrintExprCommentsAtLoc(property.Value.Loc) &&
							helpers.UTF16EqualsString(str.Value, p.renamer.NameForSymbol(id.Ref)) {
							if p.options.AddSourceMappings {
								p.addSourceMappingForName(property.Key.Loc, helpers.UTF16ToString(str.Value), id.Ref)
							}
							p.printIdentifierUTF16(str.Value)
							if property.DefaultValueOrNil.Data != nil {
								p.printSpace()
								p.print("=")
								p.printSpace()
								p.printExprWithoutLeadingNewline(property.DefaultValueOrNil, js_ast.LComma, 0)
							}
							continue
						}

						p.addSourceMapping(property.Key.Loc)
						p.printIdentifierUTF16(str.Value)
					} else if mangled, ok := property.Key.Data.(*js_ast.ENameOfSymbol); ok {
						name := p.mangledPropName(mangled.Ref)
						if p.canPrintIdentifier(name) {
							p.addSourceMappingForName(property.Key.Loc, name, mangled.Ref)
							p.printIdentifier(name)

							// Use a shorthand property if the names are the same
							if id, ok := property.Value.Data.(*js_ast.BIdentifier); ok &&
								!p.willPrintExprCommentsAtLoc(property.Value.Loc) &&
								name == p.renamer.NameForSymbol(id.Ref) {
								if property.DefaultValueOrNil.Data != nil {
									p.printSpace()
									p.print("=")
									p.printSpace()
									p.printExprWithoutLeadingNewline(property.DefaultValueOrNil, js_ast.LComma, 0)
								}
								continue
							}
						} else {
							p.addSourceMapping(property.Key.Loc)
							p.printQuotedUTF8(name, 0)
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
					p.printExprWithoutLeadingNewline(property.DefaultValueOrNil, js_ast.LComma, 0)
				}
			}

			if isMultiLine {
				p.printNewline()
				p.printExprCommentsAfterCloseTokenAtLoc(b.CloseBraceLoc)
				p.options.Indent--
				p.printIndent()
			} else {
				// This block is only reached if len(b.Properties) > 0
				p.printSpace()
			}
		}
		p.addSourceMapping(b.CloseBraceLoc)
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

func (p *printer) currentLineLength() int {
	js := p.js
	n := len(js)
	stop := p.oldLineEnd

	// Update "oldLineStart" to the start of the current line
	for i := n; i > stop; i-- {
		if c := js[i-1]; c == '\r' || c == '\n' {
			p.oldLineStart = i
			break
		}
	}

	p.oldLineEnd = n
	return n - p.oldLineStart
}

func (p *printer) printNewlinePastLineLimit() bool {
	if p.currentLineLength() < p.options.LineLimit {
		return false
	}
	p.print("\n")
	p.printIndent()
	return true
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
	if c, _ := utf8.DecodeLastRune(p.js); js_ast.IsIdentifierContinue(c) || p.prevRegExpEnd == len(p.js) {
		p.print(" ")
	}
}

type fnArgsOpts struct {
	openParenLoc              logger.Loc
	addMappingForOpenParenLoc bool
	hasRestArg                bool
	isArrow                   bool
}

func (p *printer) printFnArgs(args []js_ast.Arg, opts fnArgsOpts) {
	wrap := true

	// Minify "(a) => {}" as "a=>{}"
	if p.options.MinifyWhitespace && !opts.hasRestArg && opts.isArrow && len(args) == 1 {
		if _, ok := args[0].Binding.Data.(*js_ast.BIdentifier); ok && args[0].DefaultOrNil.Data == nil {
			wrap = false
		}
	}

	if wrap {
		if opts.addMappingForOpenParenLoc {
			p.addSourceMapping(opts.openParenLoc)
		}
		p.print("(")
	}

	for i, arg := range args {
		if i != 0 {
			p.print(",")
			p.printSpace()
		}
		p.printDecorators(arg.Decorators, printSpaceAfterDecorator)
		if opts.hasRestArg && i+1 == len(args) {
			p.print("...")
		}
		p.printBinding(arg.Binding)

		if arg.DefaultOrNil.Data != nil {
			p.printSpace()
			p.print("=")
			p.printSpace()
			p.printExprWithoutLeadingNewline(arg.DefaultOrNil, js_ast.LComma, 0)
		}
	}

	if wrap {
		p.print(")")
	}
}

func (p *printer) printFn(fn js_ast.Fn) {
	p.printFnArgs(fn.Args, fnArgsOpts{hasRestArg: fn.HasRestArg})
	p.printSpace()
	p.printBlock(fn.Body.Loc, fn.Body.Block)
}

type printAfterDecorator uint8

const (
	printNewlineAfterDecorator printAfterDecorator = iota
	printSpaceAfterDecorator
)

func (p *printer) printDecorators(decorators []js_ast.Decorator, defaultMode printAfterDecorator) (omitIndentAfter bool) {
	oldMode := defaultMode

	for _, decorator := range decorators {
		wrap := false
		wasCallTarget := false
		expr := decorator.Value
		mode := defaultMode
		if decorator.OmitNewlineAfter {
			mode = printSpaceAfterDecorator
		}

	outer:
		for {
			isCallTarget := wasCallTarget
			wasCallTarget = false

			switch e := expr.Data.(type) {
			case *js_ast.EIdentifier:
				// "@foo"
				break outer

			case *js_ast.ECall:
				// "@foo()"
				expr = e.Target
				wasCallTarget = true
				continue

			case *js_ast.EDot:
				// "@foo.bar"
				if p.canPrintIdentifier(e.Name) {
					expr = e.Target
					continue
				}

				// "@foo.\u30FF" => "@(foo['\u30FF'])"
				break

			case *js_ast.EIndex:
				if _, ok := e.Index.Data.(*js_ast.EPrivateIdentifier); ok {
					// "@foo.#bar"
					expr = e.Target
					continue
				}

				// "@(foo[bar])"
				break

			case *js_ast.EImportIdentifier:
				ref := ast.FollowSymbols(p.symbols, e.Ref)
				symbol := p.symbols.Get(ref)

				if symbol.ImportItemStatus == ast.ImportItemMissing {
					// "@(void 0)"
					break
				}

				if symbol.NamespaceAlias != nil && isCallTarget && e.WasOriginallyIdentifier {
					// "@((0, import_ns.fn)())"
					break
				}

				if value := p.options.ConstValues[ref]; value.Kind != js_ast.ConstValueNone {
					// "@(<inlined constant>)"
					break
				}

				// "@foo"
				// "@import_ns.fn"
				break outer

			default:
				// "@(foo + bar)"
				// "@(() => {})"
				break
			}

			wrap = true
			break outer
		}

		p.addSourceMapping(decorator.AtLoc)
		if oldMode == printNewlineAfterDecorator {
			p.printIndent()
		}

		p.print("@")
		if wrap {
			p.print("(")
		}
		p.printExpr(decorator.Value, js_ast.LLowest, 0)
		if wrap {
			p.print(")")
		}

		switch mode {
		case printNewlineAfterDecorator:
			p.printNewline()

		case printSpaceAfterDecorator:
			p.printSpace()
		}
		oldMode = mode
	}

	omitIndentAfter = oldMode == printSpaceAfterDecorator
	return
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
		omitIndent := p.printDecorators(item.Decorators, printNewlineAfterDecorator)
		if !omitIndent {
			p.printIndent()
		}

		if item.Kind == js_ast.PropertyClassStaticBlock {
			p.addSourceMapping(item.Loc)
			p.print("static")
			p.printSpace()
			p.printBlock(item.ClassStaticBlock.Loc, item.ClassStaticBlock.Block)
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
	p.printExprCommentsAfterCloseTokenAtLoc(class.CloseBraceLoc)
	p.options.Indent--
	p.printIndent()
	if class.CloseBraceLoc.Start > class.BodyLoc.Start {
		p.addSourceMapping(class.CloseBraceLoc)
	}
	p.print("}")
}

func (p *printer) printProperty(property js_ast.Property) {
	p.printExprCommentsAtLoc(property.Loc)

	if property.Kind == js_ast.PropertySpread {
		p.addSourceMapping(property.Loc)
		p.print("...")
		p.printExpr(property.ValueOrNil, js_ast.LComma, 0)
		return
	}

	// Handle key syntax compression for cross-module constant inlining of enums
	if p.options.MinifySyntax && property.Flags.Has(js_ast.PropertyIsComputed) {
		if dot, ok := property.Key.Data.(*js_ast.EDot); ok {
			if value, ok := p.tryToGetImportedEnumValue(dot.Target, dot.Name); ok {
				if value.String != nil {
					property.Key.Data = &js_ast.EString{Value: value.String}

					// Problematic key names must stay computed for correctness
					if !helpers.UTF16EqualsString(value.String, "__proto__") &&
						!helpers.UTF16EqualsString(value.String, "constructor") &&
						!helpers.UTF16EqualsString(value.String, "prototype") {
						property.Flags &= ^js_ast.PropertyIsComputed
					}
				} else {
					property.Key.Data = &js_ast.ENumber{Value: value.Number}
					property.Flags &= ^js_ast.PropertyIsComputed
				}
			}
		}
	}

	if property.Flags.Has(js_ast.PropertyIsStatic) {
		p.printSpaceBeforeIdentifier()
		p.addSourceMapping(property.Loc)
		p.print("static")
		p.printSpace()
	}

	switch property.Kind {
	case js_ast.PropertyGetter:
		p.printSpaceBeforeIdentifier()
		p.addSourceMapping(property.Loc)
		p.print("get")
		p.printSpace()

	case js_ast.PropertySetter:
		p.printSpaceBeforeIdentifier()
		p.addSourceMapping(property.Loc)
		p.print("set")
		p.printSpace()

	case js_ast.PropertyAutoAccessor:
		p.printSpaceBeforeIdentifier()
		p.addSourceMapping(property.Loc)
		p.print("accessor")
		p.printSpace()
	}

	if fn, ok := property.ValueOrNil.Data.(*js_ast.EFunction); property.Kind.IsMethodDefinition() && ok {
		if fn.Fn.IsAsync {
			p.printSpaceBeforeIdentifier()
			p.addSourceMapping(property.Loc)
			p.print("async")
			p.printSpace()
		}
		if fn.Fn.IsGenerator {
			p.addSourceMapping(property.Loc)
			p.print("*")
		}
	}

	isComputed := property.Flags.Has(js_ast.PropertyIsComputed)

	// Automatically print numbers that would cause a syntax error as computed properties
	if !isComputed {
		if key, ok := property.Key.Data.(*js_ast.ENumber); ok {
			if math.Signbit(key.Value) || (key.Value == positiveInfinity && p.options.MinifySyntax) {
				// "{ -1: 0 }" must be printed as "{ [-1]: 0 }"
				// "{ 1/0: 0 }" must be printed as "{ [1/0]: 0 }"
				isComputed = true
			}
		}
	}

	if isComputed {
		p.addSourceMapping(property.Loc)
		isMultiLine := p.willPrintExprCommentsAtLoc(property.Key.Loc) || p.willPrintExprCommentsAtLoc(property.CloseBracketLoc)
		p.print("[")
		if isMultiLine {
			p.printNewline()
			p.options.Indent++
			p.printIndent()
		}
		p.printExpr(property.Key, js_ast.LComma, 0)
		if isMultiLine {
			p.printNewline()
			p.printExprCommentsAfterCloseTokenAtLoc(property.CloseBracketLoc)
			p.options.Indent--
			p.printIndent()
		}
		if property.CloseBracketLoc.Start > property.Loc.Start {
			p.addSourceMapping(property.CloseBracketLoc)
		}
		p.print("]")

		if property.ValueOrNil.Data != nil {
			if fn, ok := property.ValueOrNil.Data.(*js_ast.EFunction); property.Kind.IsMethodDefinition() && ok {
				p.printFn(fn.Fn)
				return
			}

			p.print(":")
			p.printSpace()
			p.printExprWithoutLeadingNewline(property.ValueOrNil, js_ast.LComma, 0)
		}

		if property.InitializerOrNil.Data != nil {
			p.printSpace()
			p.print("=")
			p.printSpace()
			p.printExprWithoutLeadingNewline(property.InitializerOrNil, js_ast.LComma, 0)
		}
		return
	}

	switch key := property.Key.Data.(type) {
	case *js_ast.EPrivateIdentifier:
		name := p.renamer.NameForSymbol(key.Ref)
		p.addSourceMappingForName(property.Key.Loc, name, key.Ref)
		p.printIdentifier(name)

	case *js_ast.ENameOfSymbol:
		name := p.mangledPropName(key.Ref)
		if p.canPrintIdentifier(name) {
			p.printSpaceBeforeIdentifier()
			p.addSourceMappingForName(property.Key.Loc, name, key.Ref)
			p.printIdentifier(name)

			// Use a shorthand property if the names are the same
			if !p.options.UnsupportedFeatures.Has(compat.ObjectExtensions) && property.ValueOrNil.Data != nil && !p.willPrintExprCommentsAtLoc(property.ValueOrNil.Loc) {
				switch e := property.ValueOrNil.Data.(type) {
				case *js_ast.EIdentifier:
					if name == p.renamer.NameForSymbol(e.Ref) {
						if property.InitializerOrNil.Data != nil {
							p.printSpace()
							p.print("=")
							p.printSpace()
							p.printExprWithoutLeadingNewline(property.InitializerOrNil, js_ast.LComma, 0)
						}
						return
					}

				case *js_ast.EImportIdentifier:
					// Make sure we're not using a property access instead of an identifier
					ref := ast.FollowSymbols(p.symbols, e.Ref)
					if symbol := p.symbols.Get(ref); symbol.NamespaceAlias == nil && name == p.renamer.NameForSymbol(ref) &&
						p.options.ConstValues[ref].Kind == js_ast.ConstValueNone {
						if property.InitializerOrNil.Data != nil {
							p.printSpace()
							p.print("=")
							p.printSpace()
							p.printExprWithoutLeadingNewline(property.InitializerOrNil, js_ast.LComma, 0)
						}
						return
					}
				}
			}
		} else {
			p.addSourceMapping(property.Key.Loc)
			p.printQuotedUTF8(name, 0)
		}

	case *js_ast.EString:
		if !property.Flags.Has(js_ast.PropertyPreferQuotedKey) && p.canPrintIdentifierUTF16(key.Value) {
			p.printSpaceBeforeIdentifier()

			// Use a shorthand property if the names are the same
			if !p.options.UnsupportedFeatures.Has(compat.ObjectExtensions) && property.ValueOrNil.Data != nil && !p.willPrintExprCommentsAtLoc(property.ValueOrNil.Loc) {
				switch e := property.ValueOrNil.Data.(type) {
				case *js_ast.EIdentifier:
					if canUseShorthandProperty(key.Value, p.renamer.NameForSymbol(e.Ref), property.Flags) {
						if p.options.AddSourceMappings {
							p.addSourceMappingForName(property.Key.Loc, helpers.UTF16ToString(key.Value), e.Ref)
						}
						p.printIdentifierUTF16(key.Value)
						if property.InitializerOrNil.Data != nil {
							p.printSpace()
							p.print("=")
							p.printSpace()
							p.printExprWithoutLeadingNewline(property.InitializerOrNil, js_ast.LComma, 0)
						}
						return
					}

				case *js_ast.EImportIdentifier:
					// Make sure we're not using a property access instead of an identifier
					ref := ast.FollowSymbols(p.symbols, e.Ref)
					if symbol := p.symbols.Get(ref); symbol.NamespaceAlias == nil && canUseShorthandProperty(key.Value, p.renamer.NameForSymbol(ref), property.Flags) &&
						p.options.ConstValues[ref].Kind == js_ast.ConstValueNone {
						if p.options.AddSourceMappings {
							p.addSourceMappingForName(property.Key.Loc, helpers.UTF16ToString(key.Value), ref)
						}
						p.printIdentifierUTF16(key.Value)
						if property.InitializerOrNil.Data != nil {
							p.printSpace()
							p.print("=")
							p.printSpace()
							p.printExprWithoutLeadingNewline(property.InitializerOrNil, js_ast.LComma, 0)
						}
						return
					}
				}
			}

			// The JavaScript specification special-cases the property identifier
			// "__proto__" with a colon after it to set the prototype of the object.
			// If we keep the identifier but add a colon then we'll cause a behavior
			// change because the prototype will now be set. Avoid using an identifier
			// by using a computed property with a string instead. For more info see:
			// https://tc39.es/ecma262/#sec-runtime-semantics-propertydefinitionevaluation
			if property.Flags.Has(js_ast.PropertyWasShorthand) && !p.options.UnsupportedFeatures.Has(compat.ObjectExtensions) &&
				helpers.UTF16EqualsString(key.Value, "__proto__") {
				p.print("[")
				p.addSourceMapping(property.Key.Loc)
				p.printQuotedUTF16(key.Value, 0)
				p.print("]")
				break
			}

			p.addSourceMapping(property.Key.Loc)
			p.printIdentifierUTF16(key.Value)
		} else {
			p.addSourceMapping(property.Key.Loc)
			p.printQuotedUTF16(key.Value, 0)
		}

	default:
		p.printExpr(property.Key, js_ast.LLowest, 0)
	}

	if fn, ok := property.ValueOrNil.Data.(*js_ast.EFunction); property.Kind.IsMethodDefinition() && ok {
		p.printFn(fn.Fn)
		return
	}

	if property.ValueOrNil.Data != nil {
		p.print(":")
		p.printSpace()
		p.printExprWithoutLeadingNewline(property.ValueOrNil, js_ast.LComma, 0)
	}

	if property.InitializerOrNil.Data != nil {
		p.printSpace()
		p.print("=")
		p.printSpace()
		p.printExprWithoutLeadingNewline(property.InitializerOrNil, js_ast.LComma, 0)
	}
}

func canUseShorthandProperty(key []uint16, name string, flags js_ast.PropertyFlags) bool {
	// The JavaScript specification special-cases the property identifier
	// "__proto__" with a colon after it to set the prototype of the object. If
	// we remove the colon then we'll cause a behavior change because the
	// prototype will no longer be set, but we also don't want to add a colon
	// if it was omitted. Always use a shorthand property if the property is not
	// "__proto__", otherwise try to preserve the original shorthand status. See:
	// https://tc39.es/ecma262/#sec-runtime-semantics-propertydefinitionevaluation
	if !helpers.UTF16EqualsString(key, name) {
		return false
	}
	return helpers.UTF16EqualsString(key, name) && (name != "__proto__" || flags.Has(js_ast.PropertyWasShorthand))
}

func (p *printer) printQuotedUTF16(data []uint16, flags printQuotedFlags) {
	if p.options.UnsupportedFeatures.Has(compat.TemplateLiteral) {
		flags &= ^printQuotedAllowBacktick
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
		if singleCost > backtickCost && (flags&printQuotedAllowBacktick) != 0 {
			c = "`"
		}
	} else if doubleCost > backtickCost && (flags&printQuotedAllowBacktick) != 0 {
		c = "`"
	}

	p.print(c)
	p.printUnquotedUTF16(data, rune(c[0]), flags)
	p.print(c)
}

func (p *printer) printRequireOrImportExpr(importRecordIndex uint32, level js_ast.L, flags printExprFlags, closeParenLoc logger.Loc) {
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
				p.printSpaceBeforeIdentifier()
				p.printIdentifier(p.renamer.NameForSymbol(p.options.ToESMRef))
				p.print("(")
			}

			// Potentially substitute our own "__require" stub for "require"
			p.printSpaceBeforeIdentifier()
			if record.Flags.Has(ast.CallRuntimeRequire) {
				p.printIdentifier(p.renamer.NameForSymbol(p.options.RuntimeRequireRef))
			} else {
				p.print("require")
			}

			isMultiLine := p.willPrintExprCommentsAtLoc(record.Range.Loc) || p.willPrintExprCommentsAtLoc(closeParenLoc)
			p.print("(")
			if isMultiLine {
				p.printNewline()
				p.options.Indent++
				p.printIndent()
			}
			p.printExprCommentsAtLoc(record.Range.Loc)
			p.printPath(importRecordIndex, ast.ImportRequire)
			if isMultiLine {
				p.printNewline()
				p.printExprCommentsAfterCloseTokenAtLoc(closeParenLoc)
				p.options.Indent--
				p.printIndent()
			}
			if closeParenLoc.Start > record.Range.Loc.Start {
				p.addSourceMapping(closeParenLoc)
			}
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
		kind := ast.ImportDynamic
		if !p.options.UnsupportedFeatures.Has(compat.DynamicImport) {
			p.printSpaceBeforeIdentifier()
			p.print("import(")
		} else {
			kind = ast.ImportRequire
			p.printSpaceBeforeIdentifier()
			p.print("Promise.resolve()")
			p.printDotThenPrefix()
			defer p.printDotThenSuffix()

			// Wrap this with a call to "__toESM()" if this is a CommonJS file
			if record.Flags.Has(ast.WrapWithToESM) {
				p.printSpaceBeforeIdentifier()
				p.printIdentifier(p.renamer.NameForSymbol(p.options.ToESMRef))
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
			p.printSpaceBeforeIdentifier()
			if record.Flags.Has(ast.CallRuntimeRequire) {
				p.printIdentifier(p.renamer.NameForSymbol(p.options.RuntimeRequireRef))
			} else {
				p.print("require")
			}

			p.print("(")
		}
		isMultiLine := p.willPrintExprCommentsAtLoc(record.Range.Loc) ||
			p.willPrintExprCommentsAtLoc(closeParenLoc) ||
			(record.AssertOrWith != nil &&
				!p.options.UnsupportedFeatures.Has(compat.DynamicImport) &&
				(!p.options.UnsupportedFeatures.Has(compat.ImportAssertions) ||
					!p.options.UnsupportedFeatures.Has(compat.ImportAttributes)) &&
				p.willPrintExprCommentsAtLoc(record.AssertOrWith.OuterOpenBraceLoc))
		if isMultiLine {
			p.printNewline()
			p.options.Indent++
			p.printIndent()
		}
		p.printExprCommentsAtLoc(record.Range.Loc)
		p.printPath(importRecordIndex, kind)
		if !p.options.UnsupportedFeatures.Has(compat.DynamicImport) {
			p.printImportCallAssertOrWith(record.AssertOrWith, isMultiLine)
		}
		if isMultiLine {
			p.printNewline()
			p.printExprCommentsAfterCloseTokenAtLoc(closeParenLoc)
			p.options.Indent--
			p.printIndent()
		}
		if closeParenLoc.Start > record.Range.Loc.Start {
			p.addSourceMapping(closeParenLoc)
		}
		p.print(")")
		return
	}

	meta := p.options.RequireOrImportMetaForSource(record.SourceIndex.GetIndex())

	// Don't need the namespace object if the result is unused anyway
	if (flags & exprResultIsUnused) != 0 {
		meta.ExportsRef = ast.InvalidRef
	}

	// Internal "import()" of async ESM
	if record.Kind == ast.ImportDynamic && meta.IsWrapperAsync {
		p.printSpaceBeforeIdentifier()
		p.printIdentifier(p.renamer.NameForSymbol(meta.WrapperRef))
		p.print("()")
		if meta.ExportsRef != ast.InvalidRef {
			p.printDotThenPrefix()
			p.printSpaceBeforeIdentifier()
			p.printIdentifier(p.renamer.NameForSymbol(meta.ExportsRef))
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

	// Make sure the comma operator is properly wrapped
	if meta.ExportsRef != ast.InvalidRef && level >= js_ast.LComma {
		p.print("(")
		defer p.print(")")
	}

	// Wrap this with a call to "__toESM()" if this is a CommonJS file
	wrapWithToESM := record.Flags.Has(ast.WrapWithToESM)
	if wrapWithToESM {
		p.printSpaceBeforeIdentifier()
		p.printIdentifier(p.renamer.NameForSymbol(p.options.ToESMRef))
		p.print("(")
	}

	// Call the wrapper
	p.printSpaceBeforeIdentifier()
	p.printIdentifier(p.renamer.NameForSymbol(meta.WrapperRef))
	p.print("()")

	// Return the namespace object if this is an ESM file
	if meta.ExportsRef != ast.InvalidRef {
		p.print(",")
		p.printSpace()

		// Wrap this with a call to "__toCommonJS()" if this is an ESM file
		wrapWithTpCJS := record.Flags.Has(ast.WrapWithToCJS)
		if wrapWithTpCJS {
			p.printIdentifier(p.renamer.NameForSymbol(p.options.ToCommonJSRef))
			p.print("(")
		}
		p.printIdentifier(p.renamer.NameForSymbol(meta.ExportsRef))
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

func (p *printer) printUndefined(loc logger.Loc, level js_ast.L) {
	if level >= js_ast.LPrefix {
		p.addSourceMapping(loc)
		p.print("(void 0)")
	} else {
		p.printSpaceBeforeIdentifier()
		p.addSourceMapping(loc)
		p.print("void 0")
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
		var symbolFlags ast.SymbolFlags
		switch target := e.Target.Data.(type) {
		case *js_ast.EIdentifier:
			symbolFlags = p.symbols.Get(target.Ref).Flags
		case *js_ast.EImportIdentifier:
			ref := ast.FollowSymbols(p.symbols, target.Ref)
			symbolFlags = p.symbols.Get(ref).Flags
		}

		// Replace non-mutated empty functions with their arguments at print time
		if (symbolFlags & (ast.IsEmptyFunction | ast.CouldPotentiallyBeMutated)) == ast.IsEmptyFunction {
			var replacement js_ast.Expr
			for _, arg := range e.Args {
				if _, ok := arg.Data.(*js_ast.ESpread); ok {
					arg.Data = &js_ast.EArray{Items: []js_ast.Expr{arg}, IsSingleLine: true}
				}
				replacement = js_ast.JoinWithComma(replacement, p.astHelpers.SimplifyUnusedExpr(p.simplifyUnusedExpr(arg), p.options.UnsupportedFeatures))
			}
			return replacement // Don't add "undefined" here because the result isn't used
		}

		// Inline non-mutated identity functions at print time
		if (symbolFlags&(ast.IsIdentityFunction|ast.CouldPotentiallyBeMutated)) == ast.IsIdentityFunction && len(e.Args) == 1 {
			arg := e.Args[0]
			if _, ok := arg.Data.(*js_ast.ESpread); !ok {
				return p.astHelpers.SimplifyUnusedExpr(p.simplifyUnusedExpr(arg), p.options.UnsupportedFeatures)
			}
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
func (p *printer) lateConstantFoldUnaryOrBinaryOrIfExpr(expr js_ast.Expr) js_ast.Expr {
	switch e := expr.Data.(type) {
	case *js_ast.EImportIdentifier:
		ref := ast.FollowSymbols(p.symbols, e.Ref)
		if value := p.options.ConstValues[ref]; value.Kind != js_ast.ConstValueNone {
			return js_ast.ConstValueToExpr(expr.Loc, value)
		}

	case *js_ast.EDot:
		if value, ok := p.tryToGetImportedEnumValue(e.Target, e.Name); ok {
			var inlinedValue js_ast.Expr
			if value.String != nil {
				inlinedValue = js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EString{Value: value.String}}
			} else {
				inlinedValue = js_ast.Expr{Loc: expr.Loc, Data: &js_ast.ENumber{Value: value.Number}}
			}

			if strings.Contains(e.Name, "*/") {
				// Don't wrap with a comment
				return inlinedValue
			}

			// Wrap with a comment
			return js_ast.Expr{Loc: inlinedValue.Loc, Data: &js_ast.EInlinedEnum{
				Value:   inlinedValue,
				Comment: e.Name,
			}}
		}

	case *js_ast.EUnary:
		value := p.lateConstantFoldUnaryOrBinaryOrIfExpr(e.Value)

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
		left := p.lateConstantFoldUnaryOrBinaryOrIfExpr(e.Left)
		right := p.lateConstantFoldUnaryOrBinaryOrIfExpr(e.Right)

		// Only fold again if something changed
		if left.Data != e.Left.Data || right.Data != e.Right.Data {
			binary := &js_ast.EBinary{Op: e.Op, Left: left, Right: right}

			// Only fold certain operations (just like the parser)
			if js_ast.ShouldFoldBinaryOperatorWhenMinifying(binary) {
				if result := js_ast.FoldBinaryOperator(expr.Loc, binary); result.Data != nil {
					return result
				}
			}

			// Don't mutate the original AST
			expr.Data = binary
		}

	case *js_ast.EIf:
		test := p.lateConstantFoldUnaryOrBinaryOrIfExpr(e.Test)

		// Only fold again if something changed
		if test.Data != e.Test.Data {
			if boolean, sideEffects, ok := js_ast.ToBooleanWithSideEffects(test.Data); ok && sideEffects == js_ast.NoSideEffects {
				if boolean {
					return p.lateConstantFoldUnaryOrBinaryOrIfExpr(e.Yes)
				} else {
					return p.lateConstantFoldUnaryOrBinaryOrIfExpr(e.No)
				}
			}

			// Don't mutate the original AST
			expr.Data = &js_ast.EIf{Test: test, Yes: e.Yes, No: e.No}
		}
	}

	return expr
}

func (p *printer) isUnboundIdentifier(expr js_ast.Expr) bool {
	id, ok := expr.Data.(*js_ast.EIdentifier)
	return ok && p.symbols.Get(ast.FollowSymbols(p.symbols, id.Ref)).Kind == ast.SymbolUnbound
}

func (p *printer) isIdentifierOrNumericConstantOrPropertyAccess(expr js_ast.Expr) bool {
	switch e := expr.Data.(type) {
	case *js_ast.EIdentifier, *js_ast.EDot, *js_ast.EIndex:
		return true
	case *js_ast.ENumber:
		return math.IsInf(e.Value, 1) || math.IsNaN(e.Value)
	}
	return false
}

type exprStartFlags uint8

const (
	stmtStartFlag exprStartFlags = 1 << iota
	exportDefaultStartFlag
	arrowExprStartFlag
	forOfInitStartFlag
)

func (p *printer) saveExprStartFlags() (flags exprStartFlags) {
	n := len(p.js)
	if p.stmtStart == n {
		flags |= stmtStartFlag
	}
	if p.exportDefaultStart == n {
		flags |= exportDefaultStartFlag
	}
	if p.arrowExprStart == n {
		flags |= arrowExprStartFlag
	}
	if p.forOfInitStart == n {
		flags |= forOfInitStartFlag
	}
	return
}

func (p *printer) restoreExprStartFlags(flags exprStartFlags) {
	if flags != 0 {
		n := len(p.js)
		if (flags & stmtStartFlag) != 0 {
			p.stmtStart = n
		}
		if (flags & exportDefaultStartFlag) != 0 {
			p.exportDefaultStart = n
		}
		if (flags & arrowExprStartFlag) != 0 {
			p.arrowExprStart = n
		}
		if (flags & forOfInitStartFlag) != 0 {
			p.forOfInitStart = n
		}
	}
}

// Print any stored comments that are associated with this location
func (p *printer) printExprCommentsAtLoc(loc logger.Loc) {
	if p.options.MinifyWhitespace {
		return
	}
	if comments := p.exprComments[loc]; comments != nil && !p.printedExprComments[loc] {
		flags := p.saveExprStartFlags()

		// We must never generate a newline before certain expressions. For example,
		// generating a newline before the expression in a "return" statement will
		// cause a semicolon to be inserted, which would change the code's behavior.
		if p.noLeadingNewlineHere == len(p.js) {
			for _, comment := range comments {
				if strings.HasPrefix(comment, "//") {
					p.print("/*")
					p.print(comment[2:])
					if strings.HasPrefix(comment, "// ") {
						p.print(" ")
					}
					p.print("*/")
				} else {
					p.print(strings.Join(strings.Split(comment, "\n"), ""))
				}
				p.printSpace()
			}
		} else {
			for _, comment := range comments {
				p.printIndentedComment(comment)
				p.printIndent()
			}
		}

		// Mark these comments as printed so we don't print them again
		p.printedExprComments[loc] = true

		p.restoreExprStartFlags(flags)
	}
}

func (p *printer) printExprCommentsAfterCloseTokenAtLoc(loc logger.Loc) {
	if comments := p.exprComments[loc]; comments != nil && !p.printedExprComments[loc] {
		flags := p.saveExprStartFlags()

		for _, comment := range comments {
			p.printIndent()
			p.printIndentedComment(comment)
		}

		// Mark these comments as printed so we don't print them again
		p.printedExprComments[loc] = true

		p.restoreExprStartFlags(flags)
	}
}

func (p *printer) printExprWithoutLeadingNewline(expr js_ast.Expr, level js_ast.L, flags printExprFlags) {
	if !p.options.MinifyWhitespace && p.willPrintExprCommentsAtLoc(expr.Loc) {
		p.print("(")
		p.printNewline()
		p.options.Indent++
		p.printIndent()
		p.printExpr(expr, level, flags)
		p.printNewline()
		p.options.Indent--
		p.printIndent()
		p.print(")")
		return
	}

	p.noLeadingNewlineHere = len(p.js)
	p.printExpr(expr, level, flags)
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
	isPropertyAccessTarget
	parentWasUnaryOrBinaryOrIfTest
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
	if p.options.MinifySyntax && (flags&parentWasUnaryOrBinaryOrIfTest) == 0 {
		switch expr.Data.(type) {
		case *js_ast.EUnary, *js_ast.EBinary, *js_ast.EIf:
			expr = p.lateConstantFoldUnaryOrBinaryOrIfExpr(expr)
		}
	}

	p.printExprCommentsAtLoc(expr.Loc)

	switch e := expr.Data.(type) {
	case *js_ast.EMissing:
		p.addSourceMapping(expr.Loc)

	case *js_ast.EAnnotation:
		p.printExpr(e.Value, level, flags)

	case *js_ast.EUndefined:
		p.printUndefined(expr.Loc, level)

	case *js_ast.ESuper:
		p.printSpaceBeforeIdentifier()
		p.addSourceMapping(expr.Loc)
		p.print("super")

	case *js_ast.ENull:
		p.printSpaceBeforeIdentifier()
		p.addSourceMapping(expr.Loc)
		p.print("null")

	case *js_ast.EThis:
		p.printSpaceBeforeIdentifier()
		p.addSourceMapping(expr.Loc)
		p.print("this")

	case *js_ast.ESpread:
		p.addSourceMapping(expr.Loc)
		p.print("...")
		p.printExpr(e.Value, js_ast.LComma, 0)

	case *js_ast.ENewTarget:
		p.printSpaceBeforeIdentifier()
		p.addSourceMapping(expr.Loc)
		p.print("new.target")

	case *js_ast.EImportMeta:
		p.printSpaceBeforeIdentifier()
		p.addSourceMapping(expr.Loc)
		p.print("import.meta")

	case *js_ast.ENameOfSymbol:
		name := p.mangledPropName(e.Ref)
		p.addSourceMappingForName(expr.Loc, name, e.Ref)

		if !p.options.MinifyWhitespace && e.HasPropertyKeyComment {
			p.print("/* @__KEY__ */ ")
		}

		p.printQuotedUTF8(name, printQuotedAllowBacktick)

	case *js_ast.EJSXElement:
		// Start the opening tag
		p.addSourceMapping(expr.Loc)
		p.print("<")
		p.printJSXTag(e.TagOrNil)
		if !e.IsTagSingleLine {
			p.options.Indent++
		}

		// Print the attributes
		for _, property := range e.Properties {
			if e.IsTagSingleLine {
				p.printSpace()
			} else {
				p.printNewline()
				p.printIndent()
			}

			if property.Kind == js_ast.PropertySpread {
				if p.willPrintExprCommentsAtLoc(property.Loc) {
					p.print("{")
					p.printNewline()
					p.options.Indent++
					p.printIndent()
					p.printExprCommentsAtLoc(property.Loc)
					p.print("...")
					p.printExpr(property.ValueOrNil, js_ast.LComma, 0)
					p.printNewline()
					p.options.Indent--
					p.printIndent()
					p.print("}")
				} else {
					p.print("{...")
					p.printExpr(property.ValueOrNil, js_ast.LComma, 0)
					p.print("}")
				}
				continue
			}

			p.printSpaceBeforeIdentifier()
			if mangled, ok := property.Key.Data.(*js_ast.ENameOfSymbol); ok {
				name := p.mangledPropName(mangled.Ref)
				p.addSourceMappingForName(property.Key.Loc, name, mangled.Ref)
				p.printIdentifier(name)
			} else if str, ok := property.Key.Data.(*js_ast.EString); ok {
				p.addSourceMapping(property.Key.Loc)
				p.print(helpers.UTF16ToString(str.Value))
			} else {
				p.print("{...{")
				p.printSpace()
				p.print("[")
				p.printExpr(property.Key, js_ast.LComma, 0)
				p.print("]:")
				p.printSpace()
				p.printExpr(property.ValueOrNil, js_ast.LComma, 0)
				p.printSpace()
				p.print("}}")
				continue
			}

			isMultiLine := p.willPrintExprCommentsAtLoc(property.ValueOrNil.Loc)

			if property.Flags.Has(js_ast.PropertyWasShorthand) {
				// Implicit "true" value
				if boolean, ok := property.ValueOrNil.Data.(*js_ast.EBoolean); ok && boolean.Value {
					continue
				}

				// JSX element as JSX attribute value
				if _, ok := property.ValueOrNil.Data.(*js_ast.EJSXElement); ok {
					p.print("=")
					p.printExpr(property.ValueOrNil, js_ast.LLowest, 0)
					continue
				}
			}

			// Special-case raw text
			if text, ok := property.ValueOrNil.Data.(*js_ast.EJSXText); ok {
				p.print("=")
				p.addSourceMapping(property.ValueOrNil.Loc)
				p.print(text.Raw)
				continue
			}

			// Generic JS value
			p.print("={")
			if isMultiLine {
				p.printNewline()
				p.options.Indent++
				p.printIndent()
			}
			p.printExpr(property.ValueOrNil, js_ast.LComma, 0)
			if isMultiLine {
				p.printNewline()
				p.options.Indent--
				p.printIndent()
			}
			p.print("}")
		}

		// End the opening tag
		if !e.IsTagSingleLine {
			p.options.Indent--
			if len(e.Properties) > 0 {
				p.printNewline()
				p.printIndent()
			}
		}
		if e.TagOrNil.Data != nil && len(e.NullableChildren) == 0 {
			if e.IsTagSingleLine || len(e.Properties) == 0 {
				p.printSpace()
			}
			p.addSourceMapping(e.CloseLoc)
			p.print("/>")
			break
		}
		p.print(">")

		// Print the children
		for _, childOrNil := range e.NullableChildren {
			if _, ok := childOrNil.Data.(*js_ast.EJSXElement); ok {
				p.printExpr(childOrNil, js_ast.LLowest, 0)
			} else if text, ok := childOrNil.Data.(*js_ast.EJSXText); ok {
				p.addSourceMapping(childOrNil.Loc)
				p.print(text.Raw)
			} else if childOrNil.Data != nil {
				isMultiLine := p.willPrintExprCommentsAtLoc(childOrNil.Loc)
				p.print("{")
				if isMultiLine {
					p.printNewline()
					p.options.Indent++
					p.printIndent()
				}
				p.printExpr(childOrNil, js_ast.LComma, 0)
				if isMultiLine {
					p.printNewline()
					p.options.Indent--
					p.printIndent()
				}
				p.print("}")
			} else {
				p.print("{")
				if p.willPrintExprCommentsAtLoc(childOrNil.Loc) {
					// Note: Some people use these comments for AST transformations
					p.printNewline()
					p.options.Indent++
					p.printExprCommentsAfterCloseTokenAtLoc(childOrNil.Loc)
					p.options.Indent--
					p.printIndent()
				}
				p.print("}")
			}
		}

		// Print the closing tag
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
			p.addSourceMapping(expr.Loc)
			p.print("/* @__PURE__ */ ")
		}

		p.printSpaceBeforeIdentifier()
		p.addSourceMapping(expr.Loc)
		p.print("new")
		p.printSpace()
		p.printExpr(e.Target, js_ast.LNew, forbidCall)

		// Omit the "()" when minifying, but only when safe to do so
		isMultiLine := !p.options.MinifyWhitespace && ((e.IsMultiLine && len(e.Args) > 0) ||
			p.willPrintExprCommentsForAnyOf(e.Args) ||
			p.willPrintExprCommentsAtLoc(e.CloseParenLoc))
		if !p.options.MinifyWhitespace || len(e.Args) > 0 || level >= js_ast.LPostfix || isMultiLine {
			needsNewline := true
			p.print("(")
			if isMultiLine {
				p.options.Indent++
			}
			for i, arg := range e.Args {
				if i != 0 {
					p.print(",")
				}
				if p.options.LineLimit <= 0 || !p.printNewlinePastLineLimit() {
					if isMultiLine {
						if needsNewline {
							p.printNewline()
						}
						p.printIndent()
					} else if i != 0 {
						p.printSpace()
					}
				}
				p.printExpr(arg, js_ast.LComma, 0)
				needsNewline = true
			}
			if isMultiLine {
				if needsNewline || p.willPrintExprCommentsAtLoc(e.CloseParenLoc) {
					p.printNewline()
				}
				p.printExprCommentsAfterCloseTokenAtLoc(e.CloseParenLoc)
				p.options.Indent--
				p.printIndent()
			}
			if e.CloseParenLoc.Start > expr.Loc.Start {
				p.addSourceMapping(e.CloseParenLoc)
			}
			p.print(")")
		}

		if wrap {
			p.print(")")
		}

	case *js_ast.ECall:
		if p.options.MinifySyntax {
			var symbolFlags ast.SymbolFlags
			switch target := e.Target.Data.(type) {
			case *js_ast.EIdentifier:
				symbolFlags = p.symbols.Get(target.Ref).Flags
			case *js_ast.EImportIdentifier:
				ref := ast.FollowSymbols(p.symbols, target.Ref)
				symbolFlags = p.symbols.Get(ref).Flags
			}

			// Replace non-mutated empty functions with their arguments at print time
			if (symbolFlags & (ast.IsEmptyFunction | ast.CouldPotentiallyBeMutated)) == ast.IsEmptyFunction {
				var replacement js_ast.Expr
				for _, arg := range e.Args {
					if _, ok := arg.Data.(*js_ast.ESpread); ok {
						arg.Data = &js_ast.EArray{Items: []js_ast.Expr{arg}, IsSingleLine: true}
					}
					replacement = js_ast.JoinWithComma(replacement, p.astHelpers.SimplifyUnusedExpr(arg, p.options.UnsupportedFeatures))
				}
				if replacement.Data == nil || (flags&exprResultIsUnused) == 0 {
					replacement = js_ast.JoinWithComma(replacement, js_ast.Expr{Loc: expr.Loc, Data: js_ast.EUndefinedShared})
				}
				p.printExpr(p.guardAgainstBehaviorChangeDueToSubstitution(replacement, flags), level, flags)
				break
			}

			// Inline non-mutated identity functions at print time
			if (symbolFlags&(ast.IsIdentityFunction|ast.CouldPotentiallyBeMutated)) == ast.IsIdentityFunction && len(e.Args) == 1 {
				arg := e.Args[0]
				if _, ok := arg.Data.(*js_ast.ESpread); !ok {
					if (flags & exprResultIsUnused) != 0 {
						arg = p.astHelpers.SimplifyUnusedExpr(arg, p.options.UnsupportedFeatures)
					}
					p.printExpr(p.guardAgainstBehaviorChangeDueToSubstitution(arg, flags), level, flags)
					break
				}
			}

			// Inline IIFEs that return expressions at print time
			if len(e.Args) == 0 {
				// Note: Do not inline async arrow functions as they are not IIFEs. In
				// particular, they are not necessarily invoked immediately, and any
				// exceptions involved in their evaluation will be swallowed without
				// bubbling up to the surrounding context.
				if arrow, ok := e.Target.Data.(*js_ast.EArrow); ok && len(arrow.Args) == 0 && !arrow.IsAsync {
					stmts := arrow.Body.Block.Stmts

					// "(() => {})()" => "void 0"
					if len(stmts) == 0 {
						value := js_ast.Expr{Loc: expr.Loc, Data: js_ast.EUndefinedShared}
						p.printExpr(p.guardAgainstBehaviorChangeDueToSubstitution(value, flags), level, flags)
						break
					}

					// "(() => 123)()" => "123"
					if len(stmts) == 1 {
						if stmt, ok := stmts[0].Data.(*js_ast.SReturn); ok {
							value := stmt.ValueOrNil
							if value.Data == nil {
								value.Data = js_ast.EUndefinedShared
							}
							p.printExpr(p.guardAgainstBehaviorChangeDueToSubstitution(value, flags), level, flags)
							break
						}
					}
				}
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
			flags := p.saveExprStartFlags()
			p.addSourceMapping(expr.Loc)
			p.print("/* @__PURE__ */ ")
			p.restoreExprStartFlags(flags)
		}

		// We don't ever want to accidentally generate a direct eval expression here
		p.callTarget = e.Target.Data
		if (e.Kind != js_ast.DirectEval && p.isUnboundEvalIdentifier(e.Target) && e.OptionalChain == js_ast.OptionalChainNone) ||
			(e.Kind != js_ast.TargetWasOriginallyPropertyAccess && js_ast.IsPropertyAccess(e.Target)) {
			p.print("(0,")
			p.printSpace()
			p.printExpr(e.Target, js_ast.LPostfix, isCallTargetOrTemplateTag)
			p.print(")")
		} else {
			p.printExpr(e.Target, js_ast.LPostfix, isCallTargetOrTemplateTag|targetFlags)
		}

		if e.OptionalChain == js_ast.OptionalChainStart {
			p.print("?.")
		}

		isMultiLine := !p.options.MinifyWhitespace && ((e.IsMultiLine && len(e.Args) > 0) ||
			p.willPrintExprCommentsForAnyOf(e.Args) ||
			p.willPrintExprCommentsAtLoc(e.CloseParenLoc))
		p.print("(")
		if isMultiLine {
			p.options.Indent++
		}
		for i, arg := range e.Args {
			if i != 0 {
				p.print(",")
			}
			if p.options.LineLimit <= 0 || !p.printNewlinePastLineLimit() {
				if isMultiLine {
					p.printNewline()
					p.printIndent()
				} else if i != 0 {
					p.printSpace()
				}
			}
			p.printExpr(arg, js_ast.LComma, 0)
		}
		if isMultiLine {
			p.printNewline()
			p.printExprCommentsAfterCloseTokenAtLoc(e.CloseParenLoc)
			p.options.Indent--
			p.printIndent()
		}
		if e.CloseParenLoc.Start > expr.Loc.Start {
			p.addSourceMapping(e.CloseParenLoc)
		}
		p.print(")")

		if wrap {
			p.print(")")
		}

	case *js_ast.ERequireString:
		p.addSourceMapping(expr.Loc)
		p.printRequireOrImportExpr(e.ImportRecordIndex, level, flags, e.CloseParenLoc)

	case *js_ast.ERequireResolveString:
		recordLoc := p.importRecords[e.ImportRecordIndex].Range.Loc
		isMultiLine := p.willPrintExprCommentsAtLoc(recordLoc) || p.willPrintExprCommentsAtLoc(e.CloseParenLoc)
		wrap := level >= js_ast.LNew || (flags&forbidCall) != 0
		if wrap {
			p.print("(")
		}
		p.printSpaceBeforeIdentifier()
		p.addSourceMapping(expr.Loc)
		p.print("require.resolve(")
		if isMultiLine {
			p.printNewline()
			p.options.Indent++
			p.printIndent()
			p.printExprCommentsAtLoc(recordLoc)
		}
		p.printPath(e.ImportRecordIndex, ast.ImportRequireResolve)
		if isMultiLine {
			p.printNewline()
			p.printExprCommentsAfterCloseTokenAtLoc(e.CloseParenLoc)
			p.options.Indent--
			p.printIndent()
		}
		if e.CloseParenLoc.Start > expr.Loc.Start {
			p.addSourceMapping(e.CloseParenLoc)
		}
		p.print(")")
		if wrap {
			p.print(")")
		}

	case *js_ast.EImportString:
		p.addSourceMapping(expr.Loc)
		p.printRequireOrImportExpr(e.ImportRecordIndex, level, flags, e.CloseParenLoc)

	case *js_ast.EImportCall:
		// Only print the second argument if either import assertions or import attributes are supported
		printImportAssertOrWith := e.OptionsOrNil.Data != nil && (!p.options.UnsupportedFeatures.Has(compat.ImportAssertions) || !p.options.UnsupportedFeatures.Has(compat.ImportAttributes))
		isMultiLine := !p.options.MinifyWhitespace &&
			(p.willPrintExprCommentsAtLoc(e.Expr.Loc) ||
				(printImportAssertOrWith && p.willPrintExprCommentsAtLoc(e.OptionsOrNil.Loc)) ||
				p.willPrintExprCommentsAtLoc(e.CloseParenLoc))
		wrap := level >= js_ast.LNew || (flags&forbidCall) != 0
		if wrap {
			p.print("(")
		}
		p.printSpaceBeforeIdentifier()
		p.addSourceMapping(expr.Loc)
		p.print("import(")
		if isMultiLine {
			p.printNewline()
			p.options.Indent++
			p.printIndent()
		}
		p.printExpr(e.Expr, js_ast.LComma, 0)

		if printImportAssertOrWith {
			p.print(",")
			if isMultiLine {
				p.printNewline()
				p.printIndent()
			} else {
				p.printSpace()
			}
			p.printExpr(e.OptionsOrNil, js_ast.LComma, 0)
		}

		if isMultiLine {
			p.printNewline()
			p.printExprCommentsAfterCloseTokenAtLoc(e.CloseParenLoc)
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
			if value, ok := p.tryToGetImportedEnumValue(e.Target, e.Name); ok {
				if value.String != nil {
					p.printQuotedUTF16(value.String, printQuotedAllowBacktick)
				} else {
					p.printNumber(value.Number, level)
				}
				if !p.options.MinifyWhitespace && !p.options.MinifyIdentifiers && !strings.Contains(e.Name, "*/") {
					p.print(" /* ")
					p.print(e.Name)
					p.print(" */")
				}
				break
			}
		} else {
			if (flags & hasNonOptionalChainParent) != 0 {
				wrap = true
				p.print("(")
			}
			flags &= ^hasNonOptionalChainParent
		}
		p.printExpr(e.Target, js_ast.LPostfix, (flags&(forbidCall|hasNonOptionalChainParent))|isPropertyAccessTarget)
		if p.canPrintIdentifier(e.Name) {
			if e.OptionalChain != js_ast.OptionalChainStart && p.needSpaceBeforeDot == len(p.js) {
				// "1.toString" is a syntax error, so print "1 .toString" instead
				p.print(" ")
			}
			if e.OptionalChain == js_ast.OptionalChainStart {
				p.print("?.")
			} else {
				p.print(".")
			}
			if p.options.LineLimit > 0 {
				p.printNewlinePastLineLimit()
			}
			p.addSourceMapping(e.NameLoc)
			p.printIdentifier(e.Name)
		} else {
			if e.OptionalChain == js_ast.OptionalChainStart {
				p.print("?.")
			}
			p.print("[")
			p.addSourceMapping(e.NameLoc)
			p.printQuotedUTF8(e.Name, printQuotedAllowBacktick)
			p.print("]")
		}
		if wrap {
			p.print(")")
		}

	case *js_ast.EIndex:
		if e.OptionalChain == js_ast.OptionalChainNone {
			flags |= hasNonOptionalChainParent

			// Inline cross-module TypeScript enum references here
			if index, ok := e.Index.Data.(*js_ast.EString); ok {
				if value, name, ok := p.tryToGetImportedEnumValueUTF16(e.Target, index.Value); ok {
					if value.String != nil {
						p.printQuotedUTF16(value.String, printQuotedAllowBacktick)
					} else {
						p.printNumber(value.Number, level)
					}
					if !p.options.MinifyWhitespace && !p.options.MinifyIdentifiers && !strings.Contains(name, "*/") {
						p.print(" /* ")
						p.print(name)
						p.print(" */")
					}
					break
				}
			}
		} else {
			if (flags & hasNonOptionalChainParent) != 0 {
				p.print("(")
				defer p.print(")")
			}
			flags &= ^hasNonOptionalChainParent
		}
		p.printExpr(e.Target, js_ast.LPostfix, (flags&(forbidCall|hasNonOptionalChainParent))|isPropertyAccessTarget)
		if e.OptionalChain == js_ast.OptionalChainStart {
			p.print("?.")
		}

		switch index := e.Index.Data.(type) {
		case *js_ast.EPrivateIdentifier:
			if e.OptionalChain != js_ast.OptionalChainStart {
				p.print(".")
			}
			name := p.renamer.NameForSymbol(index.Ref)
			p.addSourceMappingForName(e.Index.Loc, name, index.Ref)
			p.printIdentifier(name)
			return

		case *js_ast.ENameOfSymbol:
			if name := p.mangledPropName(index.Ref); p.canPrintIdentifier(name) {
				if e.OptionalChain != js_ast.OptionalChainStart {
					p.print(".")
				}
				p.addSourceMappingForName(e.Index.Loc, name, index.Ref)
				p.printIdentifier(name)
				return
			}

		case *js_ast.EInlinedEnum:
			if p.options.MinifySyntax {
				if str, ok := index.Value.Data.(*js_ast.EString); ok && p.canPrintIdentifierUTF16(str.Value) {
					if e.OptionalChain != js_ast.OptionalChainStart {
						p.print(".")
					}
					p.addSourceMapping(index.Value.Loc)
					p.printIdentifierUTF16(str.Value)
					return
				}
			}

		case *js_ast.EDot:
			if p.options.MinifySyntax {
				if value, ok := p.tryToGetImportedEnumValue(index.Target, index.Name); ok && value.String != nil && p.canPrintIdentifierUTF16(value.String) {
					if e.OptionalChain != js_ast.OptionalChainStart {
						p.print(".")
					}
					p.addSourceMapping(e.Index.Loc)
					p.printIdentifierUTF16(value.String)
					return
				}
			}
		}

		isMultiLine := p.willPrintExprCommentsAtLoc(e.Index.Loc) || p.willPrintExprCommentsAtLoc(e.CloseBracketLoc)
		p.print("[")
		if isMultiLine {
			p.printNewline()
			p.options.Indent++
			p.printIndent()
		}
		p.printExpr(e.Index, js_ast.LLowest, 0)
		if isMultiLine {
			p.printNewline()
			p.printExprCommentsAfterCloseTokenAtLoc(e.CloseBracketLoc)
			p.options.Indent--
			p.printIndent()
		}
		if e.CloseBracketLoc.Start > expr.Loc.Start {
			p.addSourceMapping(e.CloseBracketLoc)
		}
		p.print("]")

	case *js_ast.EIf:
		wrap := level >= js_ast.LConditional
		if wrap {
			p.print("(")
			flags &= ^forbidIn
		}
		p.printExpr(e.Test, js_ast.LConditional, (flags&forbidIn)|parentWasUnaryOrBinaryOrIfTest)
		p.printSpace()
		p.print("?")
		if p.options.LineLimit <= 0 || !p.printNewlinePastLineLimit() {
			p.printSpace()
		}
		p.printExprWithoutLeadingNewline(e.Yes, js_ast.LYield, 0)
		p.printSpace()
		p.print(":")
		if p.options.LineLimit <= 0 || !p.printNewlinePastLineLimit() {
			p.printSpace()
		}
		p.printExprWithoutLeadingNewline(e.No, js_ast.LYield, flags&forbidIn)
		if wrap {
			p.print(")")
		}

	case *js_ast.EArrow:
		wrap := level >= js_ast.LAssign

		if wrap {
			p.print("(")
		}
		if !p.options.MinifyWhitespace && e.HasNoSideEffectsComment {
			p.print("/* @__NO_SIDE_EFFECTS__ */ ")
		}
		if e.IsAsync {
			p.addSourceMapping(expr.Loc)
			p.printSpaceBeforeIdentifier()
			p.print("async")
			p.printSpace()
		}

		p.printFnArgs(e.Args, fnArgsOpts{
			openParenLoc:              expr.Loc,
			addMappingForOpenParenLoc: !e.IsAsync,
			hasRestArg:                e.HasRestArg,
			isArrow:                   true,
		})
		p.printSpace()
		p.print("=>")
		p.printSpace()

		wasPrinted := false
		if len(e.Body.Block.Stmts) == 1 && e.PreferExpr {
			if s, ok := e.Body.Block.Stmts[0].Data.(*js_ast.SReturn); ok && s.ValueOrNil.Data != nil {
				p.arrowExprStart = len(p.js)
				p.printExprWithoutLeadingNewline(s.ValueOrNil, js_ast.LComma, flags&forbidIn)
				wasPrinted = true
			}
		}
		if !wasPrinted {
			p.printBlock(e.Body.Loc, e.Body.Block)
		}
		if wrap {
			p.print(")")
		}

	case *js_ast.EFunction:
		n := len(p.js)
		wrap := p.stmtStart == n || p.exportDefaultStart == n ||
			((flags&isPropertyAccessTarget) != 0 && p.options.UnsupportedFeatures.Has(compat.FunctionOrClassPropertyAccess))
		if wrap {
			p.print("(")
		}
		if !p.options.MinifyWhitespace && e.Fn.HasNoSideEffectsComment {
			p.print("/* @__NO_SIDE_EFFECTS__ */ ")
		}
		p.printSpaceBeforeIdentifier()
		p.addSourceMapping(expr.Loc)
		if e.Fn.IsAsync {
			p.print("async ")
		}
		p.print("function")
		if e.Fn.IsGenerator {
			p.print("*")
			p.printSpace()
		}
		if e.Fn.Name != nil {
			p.printSpaceBeforeIdentifier()
			name := p.renamer.NameForSymbol(e.Fn.Name.Ref)
			p.addSourceMappingForName(e.Fn.Name.Loc, name, e.Fn.Name.Ref)
			p.printIdentifier(name)
		}
		p.printFn(e.Fn)
		if wrap {
			p.print(")")
		}

	case *js_ast.EClass:
		n := len(p.js)
		wrap := p.stmtStart == n || p.exportDefaultStart == n ||
			((flags&isPropertyAccessTarget) != 0 && p.options.UnsupportedFeatures.Has(compat.FunctionOrClassPropertyAccess))
		if wrap {
			p.print("(")
		}
		p.printDecorators(e.Class.Decorators, printSpaceAfterDecorator)
		p.printSpaceBeforeIdentifier()
		p.addSourceMapping(expr.Loc)
		p.print("class")
		if e.Class.Name != nil {
			p.print(" ")
			name := p.renamer.NameForSymbol(e.Class.Name.Ref)
			p.addSourceMappingForName(e.Class.Name.Loc, name, e.Class.Name.Ref)
			p.printIdentifier(name)
		}
		p.printClass(e.Class)
		if wrap {
			p.print(")")
		}

	case *js_ast.EArray:
		isMultiLine := (len(e.Items) > 0 && !e.IsSingleLine) || p.willPrintExprCommentsForAnyOf(e.Items) || p.willPrintExprCommentsAtLoc(e.CloseBracketLoc)
		p.addSourceMapping(expr.Loc)
		p.print("[")
		if len(e.Items) > 0 || isMultiLine {
			if isMultiLine {
				p.options.Indent++
			}

			for i, item := range e.Items {
				if i != 0 {
					p.print(",")
				}
				if p.options.LineLimit <= 0 || !p.printNewlinePastLineLimit() {
					if isMultiLine {
						p.printNewline()
						p.printIndent()
					} else if i != 0 {
						p.printSpace()
					}
				}
				p.printExpr(item, js_ast.LComma, 0)

				// Make sure there's a comma after trailing missing items
				_, ok := item.Data.(*js_ast.EMissing)
				if ok && i == len(e.Items)-1 {
					p.print(",")
				}
			}

			if isMultiLine {
				p.printNewline()
				p.printExprCommentsAfterCloseTokenAtLoc(e.CloseBracketLoc)
				p.options.Indent--
				p.printIndent()
			}
		}
		if e.CloseBracketLoc.Start > expr.Loc.Start {
			p.addSourceMapping(e.CloseBracketLoc)
		}
		p.print("]")

	case *js_ast.EObject:
		isMultiLine := (len(e.Properties) > 0 && !e.IsSingleLine) || p.willPrintExprCommentsAtLoc(e.CloseBraceLoc)
		if !p.options.MinifyWhitespace && !isMultiLine {
			for _, property := range e.Properties {
				if p.willPrintExprCommentsAtLoc(property.Loc) {
					isMultiLine = true
					break
				}
			}
		}
		n := len(p.js)
		wrap := p.stmtStart == n || p.arrowExprStart == n
		if wrap {
			p.print("(")
		}
		p.addSourceMapping(expr.Loc)
		p.print("{")
		if len(e.Properties) > 0 || isMultiLine {
			if isMultiLine {
				p.options.Indent++
			}

			for i, item := range e.Properties {
				if i != 0 {
					p.print(",")
				}
				if p.options.LineLimit <= 0 || !p.printNewlinePastLineLimit() {
					if isMultiLine {
						p.printNewline()
						p.printIndent()
					} else {
						p.printSpace()
					}
				}
				p.printProperty(item)
			}

			if isMultiLine {
				p.printNewline()
				p.printExprCommentsAfterCloseTokenAtLoc(e.CloseBraceLoc)
				p.options.Indent--
				p.printIndent()
			} else if len(e.Properties) > 0 {
				p.printSpace()
			}
		}
		if e.CloseBraceLoc.Start > expr.Loc.Start {
			p.addSourceMapping(e.CloseBraceLoc)
		}
		p.print("}")
		if wrap {
			p.print(")")
		}

	case *js_ast.EBoolean:
		p.addSourceMapping(expr.Loc)
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
		var flags printQuotedFlags
		if e.ContainsUniqueKey {
			flags = printQuotedNoWrap
		}
		p.addSourceMapping(expr.Loc)

		if !p.options.MinifyWhitespace && e.HasPropertyKeyComment {
			p.print("/* @__KEY__ */ ")
		}

		// If this was originally a template literal, print it as one as long as we're not minifying
		if e.PreferTemplate && !p.options.MinifySyntax && !p.options.UnsupportedFeatures.Has(compat.TemplateLiteral) {
			p.print("`")
			p.printUnquotedUTF16(e.Value, '`', flags)
			p.print("`")
			return
		}

		p.printQuotedUTF16(e.Value, flags|printQuotedAllowBacktick)

	case *js_ast.ETemplate:
		if e.TagOrNil.Data == nil && (p.options.MinifySyntax || p.wasLazyExport) {
			// Inline enums and mangled properties when minifying
			var replaced []js_ast.TemplatePart
			for i, part := range e.Parts {
				var inlinedValue js_ast.E
				switch e2 := part.Value.Data.(type) {
				case *js_ast.ENameOfSymbol:
					inlinedValue = &js_ast.EString{
						Value:                 helpers.StringToUTF16(p.mangledPropName(e2.Ref)),
						HasPropertyKeyComment: e2.HasPropertyKeyComment,
					}
				case *js_ast.EDot:
					if value, ok := p.tryToGetImportedEnumValue(e2.Target, e2.Name); ok {
						if value.String != nil {
							inlinedValue = &js_ast.EString{Value: value.String}
						} else {
							inlinedValue = &js_ast.ENumber{Value: value.Number}
						}
					}
				}
				if inlinedValue != nil {
					if replaced == nil {
						replaced = make([]js_ast.TemplatePart, 0, len(e.Parts))
						replaced = append(replaced, e.Parts[:i]...)
					}
					part.Value.Data = inlinedValue
					replaced = append(replaced, part)
				} else if replaced != nil {
					replaced = append(replaced, part)
				}
			}
			if replaced != nil {
				copy := *e
				copy.Parts = replaced
				switch e2 := js_ast.InlinePrimitivesIntoTemplate(logger.Loc{}, &copy).Data.(type) {
				case *js_ast.EString:
					p.printQuotedUTF16(e2.Value, printQuotedAllowBacktick)
					return
				case *js_ast.ETemplate:
					e = e2
				}
			}

			// Convert no-substitution template literals into strings if it's smaller
			if len(e.Parts) == 0 {
				p.addSourceMapping(expr.Loc)
				p.printQuotedUTF16(e.HeadCooked, printQuotedAllowBacktick)
				return
			}
		}

		if e.TagOrNil.Data != nil {
			tagIsPropertyAccess := false
			switch e.TagOrNil.Data.(type) {
			case *js_ast.EDot, *js_ast.EIndex:
				tagIsPropertyAccess = true
			}
			if !e.TagWasOriginallyPropertyAccess && tagIsPropertyAccess {
				// Prevent "x``" from becoming "y.z``"
				p.print("(0,")
				p.printSpace()
				p.printExpr(e.TagOrNil, js_ast.LLowest, isCallTargetOrTemplateTag)
				p.print(")")
			} else if js_ast.IsOptionalChain(e.TagOrNil) {
				// Optional chains are forbidden in template tags
				p.print("(")
				p.printExpr(e.TagOrNil, js_ast.LLowest, isCallTargetOrTemplateTag)
				p.print(")")
			} else {
				p.printExpr(e.TagOrNil, js_ast.LPostfix, isCallTargetOrTemplateTag)
			}
		} else {
			p.addSourceMapping(expr.Loc)
		}
		p.print("`")
		if e.TagOrNil.Data != nil {
			p.print(e.HeadRaw)
		} else {
			p.printUnquotedUTF16(e.HeadCooked, '`', 0)
		}
		for _, part := range e.Parts {
			p.print("${")
			p.printExpr(part.Value, js_ast.LLowest, 0)
			p.addSourceMapping(part.TailLoc)
			p.print("}")
			if e.TagOrNil.Data != nil {
				p.print(part.TailRaw)
			} else {
				p.printUnquotedUTF16(part.TailCooked, '`', 0)
			}
		}
		p.print("`")

	case *js_ast.ERegExp:
		buffer := p.js
		n := len(buffer)

		// Avoid forming a single-line comment or "</script" sequence
		if !p.options.UnsupportedFeatures.Has(compat.InlineScript) && n > 0 {
			if last := buffer[n-1]; last == '/' || (last == '<' && len(e.Value) >= 7 && strings.EqualFold(e.Value[:7], "/script")) {
				p.print(" ")
			}
		}

		p.addSourceMapping(expr.Loc)
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
		if !p.options.UnsupportedFeatures.Has(compat.Bigint) {
			p.printSpaceBeforeIdentifier()
			p.addSourceMapping(expr.Loc)
			p.print(e.Value)
			p.print("n")
			break
		}

		wrap := level >= js_ast.LNew || (flags&forbidCall) != 0
		hasPureComment := !p.options.MinifyWhitespace

		if hasPureComment && level >= js_ast.LPostfix {
			wrap = true
		}

		if wrap {
			p.print("(")
		}

		if hasPureComment {
			flags := p.saveExprStartFlags()
			p.addSourceMapping(expr.Loc)
			p.print("/* @__PURE__ */ ")
			p.restoreExprStartFlags(flags)
		}

		value := e.Value
		useQuotes := true

		// When minifying, try to convert to a shorter form
		if p.options.MinifySyntax {
			var i big.Int
			fmt.Sscan(value, &i)
			str := i.String()

			// Print without quotes if it can be converted exactly
			if num, err := strconv.ParseFloat(str, 64); err == nil && str == fmt.Sprintf("%.0f", num) {
				useQuotes = false
			}

			// Print the converted form if it's shorter (long hex strings may not be shorter)
			if len(str) < len(value) {
				value = str
			}
		}

		p.printSpaceBeforeIdentifier()
		p.addSourceMapping(expr.Loc)

		if useQuotes {
			p.print("BigInt(\"")
		} else {
			p.print("BigInt(")
		}

		p.print(value)

		if useQuotes {
			p.print("\")")
		} else {
			p.print(")")
		}

		if wrap {
			p.print(")")
		}

	case *js_ast.ENumber:
		p.addSourceMapping(expr.Loc)
		p.printNumber(e.Value, level)

	case *js_ast.EIdentifier:
		name := p.renamer.NameForSymbol(e.Ref)
		wrap := len(p.js) == p.forOfInitStart && (name == "let" ||
			((flags&isFollowedByOf) != 0 && (flags&isInsideForAwait) == 0 && name == "async"))

		if wrap {
			p.print("(")
		}

		p.printSpaceBeforeIdentifier()
		p.addSourceMappingForName(expr.Loc, name, e.Ref)
		p.printIdentifier(name)

		if wrap {
			p.print(")")
		}

	case *js_ast.EImportIdentifier:
		// Potentially use a property access instead of an identifier
		ref := ast.FollowSymbols(p.symbols, e.Ref)
		symbol := p.symbols.Get(ref)

		if symbol.ImportItemStatus == ast.ImportItemMissing {
			p.printUndefined(expr.Loc, level)
		} else if symbol.NamespaceAlias != nil {
			wrap := p.callTarget == e && e.WasOriginallyIdentifier
			if wrap {
				p.print("(0,")
				p.printSpace()
			}
			p.printSpaceBeforeIdentifier()
			p.addSourceMapping(expr.Loc)
			p.printIdentifier(p.renamer.NameForSymbol(symbol.NamespaceAlias.NamespaceRef))
			alias := symbol.NamespaceAlias.Alias
			if !e.PreferQuotedKey && p.canPrintIdentifier(alias) {
				p.print(".")
				p.addSourceMappingForName(expr.Loc, alias, ref)
				p.printIdentifier(alias)
			} else {
				p.print("[")
				p.addSourceMappingForName(expr.Loc, alias, ref)
				p.printQuotedUTF8(alias, printQuotedAllowBacktick)
				p.print("]")
			}
			if wrap {
				p.print(")")
			}
		} else if value := p.options.ConstValues[ref]; value.Kind != js_ast.ConstValueNone {
			// Handle inlined constants
			p.printExpr(js_ast.ConstValueToExpr(expr.Loc, value), level, flags)
		} else {
			p.printSpaceBeforeIdentifier()
			name := p.renamer.NameForSymbol(ref)
			p.addSourceMappingForName(expr.Loc, name, ref)
			p.printIdentifier(name)
		}

	case *js_ast.EAwait:
		wrap := level >= js_ast.LPrefix

		if wrap {
			p.print("(")
		}

		p.printSpaceBeforeIdentifier()
		p.addSourceMapping(expr.Loc)
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
		p.addSourceMapping(expr.Loc)
		p.print("yield")

		if e.ValueOrNil.Data != nil {
			if e.IsStar {
				p.print("*")
			}
			p.printSpace()
			p.printExprWithoutLeadingNewline(e.ValueOrNil, js_ast.LYield, 0)
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
			p.printExpr(e.Value, js_ast.LPostfix-1, parentWasUnaryOrBinaryOrIfTest)
		}

		if entry.IsKeyword {
			p.printSpaceBeforeIdentifier()
			if e.Op.IsPrefix() {
				p.addSourceMapping(expr.Loc)
			}
			p.print(entry.Text)
			p.printSpace()
		} else {
			p.printSpaceBeforeOperator(e.Op)
			if e.Op.IsPrefix() {
				p.addSourceMapping(expr.Loc)
			}
			p.print(entry.Text)
			p.prevOp = e.Op
			p.prevOpEnd = len(p.js)
		}

		if e.Op.IsPrefix() {
			valueFlags := parentWasUnaryOrBinaryOrIfTest
			if e.Op == js_ast.UnOpDelete {
				valueFlags |= isDeleteTarget
			}

			// Never turn "typeof (0, x)" into "typeof x" or "delete (0, x)" into "delete x"
			if (e.Op == js_ast.UnOpTypeof && !e.WasOriginallyTypeofIdentifier && p.isUnboundIdentifier(e.Value)) ||
				(e.Op == js_ast.UnOpDelete && !e.WasOriginallyDeleteOfIdentifierOrPropertyAccess && p.isIdentifierOrNumericConstantOrPropertyAccess(e.Value)) {
				p.print("(0,")
				p.printSpace()
				p.printExpr(e.Value, js_ast.LPrefix-1, valueFlags)
				p.print(")")
			} else {
				p.printExpr(e.Value, js_ast.LPrefix-1, valueFlags)
			}
		}

		if wrap {
			p.print(")")
		}

	case *js_ast.EBinary:
		// The handling of binary expressions is convoluted because we're using
		// iteration on the heap instead of recursion on the call stack to avoid
		// stack overflow for deeply-nested ASTs. See the comments for the similar
		// code in the JavaScript parser for details.
		v := binaryExprVisitor{
			e:     e,
			level: level,
			flags: flags,
		}

		// Use a single stack to reduce allocation overhead
		stackBottom := len(p.binaryExprStack)

		for {
			// Check whether this node is a special case, and stop if it is
			if !v.checkAndPrepare(p) {
				break
			}

			left := v.e.Left
			leftBinary, ok := left.Data.(*js_ast.EBinary)

			// Stop iterating if iteration doesn't apply to the left node
			if !ok {
				p.printExpr(left, v.leftLevel, v.leftFlags)
				v.visitRightAndFinish(p)
				break
			}

			// Manually run the code at the start of "printExpr"
			p.printExprCommentsAtLoc(left.Loc)

			// Only allocate heap memory on the stack for nested binary expressions
			p.binaryExprStack = append(p.binaryExprStack, v)
			v = binaryExprVisitor{
				e:     leftBinary,
				level: v.leftLevel,
				flags: v.leftFlags,
			}
		}

		// Process all binary operations from the deepest-visited node back toward
		// our original top-level binary operation
		for {
			n := len(p.binaryExprStack) - 1
			if n < stackBottom {
				break
			}
			v := p.binaryExprStack[n]
			p.binaryExprStack = p.binaryExprStack[:n]
			v.visitRightAndFinish(p)
		}

	default:
		panic(fmt.Sprintf("Unexpected expression of type %T", expr.Data))
	}
}

// The handling of binary expressions is convoluted because we're using
// iteration on the heap instead of recursion on the call stack to avoid
// stack overflow for deeply-nested ASTs. See the comments for the similar
// code in the JavaScript parser for details.
type binaryExprVisitor struct {
	// Inputs
	e     *js_ast.EBinary
	level js_ast.L
	flags printExprFlags

	// Input for visiting the left child
	leftLevel js_ast.L
	leftFlags printExprFlags

	// "Local variables" passed from "checkAndPrepare" to "visitRightAndFinish"
	entry      js_ast.OpTableEntry
	wrap       bool
	rightLevel js_ast.L
}

func (v *binaryExprVisitor) checkAndPrepare(p *printer) bool {
	e := v.e

	// If this is a comma operator then either the result is unused (and we
	// should have already simplified unused expressions), or the result is used
	// (and we can still simplify unused expressions inside the left operand)
	if e.Op == js_ast.BinOpComma {
		if (v.flags & didAlreadySimplifyUnusedExprs) == 0 {
			left := p.simplifyUnusedExpr(e.Left)
			right := e.Right
			if (v.flags & exprResultIsUnused) != 0 {
				right = p.simplifyUnusedExpr(right)
			}
			if left.Data != e.Left.Data || right.Data != e.Right.Data {
				// Pass a flag so we don't needlessly re-simplify the same expression
				p.printExpr(p.guardAgainstBehaviorChangeDueToSubstitution(js_ast.JoinWithComma(left, right), v.flags), v.level, v.flags|didAlreadySimplifyUnusedExprs)
				return false
			}
		} else {
			// Pass a flag so we don't needlessly re-simplify the same expression
			v.flags |= didAlreadySimplifyUnusedExprs
		}
	}

	v.entry = js_ast.OpTable[e.Op]
	v.wrap = v.level >= v.entry.Level || (e.Op == js_ast.BinOpIn && (v.flags&forbidIn) != 0)

	// Destructuring assignments must be parenthesized
	if n := len(p.js); p.stmtStart == n || p.arrowExprStart == n {
		if _, ok := e.Left.Data.(*js_ast.EObject); ok {
			v.wrap = true
		}
	}

	if v.wrap {
		p.print("(")
		v.flags &= ^forbidIn
	}

	v.leftLevel = v.entry.Level - 1
	v.rightLevel = v.entry.Level - 1

	if e.Op.IsRightAssociative() {
		v.leftLevel = v.entry.Level
	}
	if e.Op.IsLeftAssociative() {
		v.rightLevel = v.entry.Level
	}

	switch e.Op {
	case js_ast.BinOpNullishCoalescing:
		// "??" can't directly contain "||" or "&&" without being wrapped in parentheses
		if left, ok := e.Left.Data.(*js_ast.EBinary); ok && (left.Op == js_ast.BinOpLogicalOr || left.Op == js_ast.BinOpLogicalAnd) {
			v.leftLevel = js_ast.LPrefix
		}
		if right, ok := e.Right.Data.(*js_ast.EBinary); ok && (right.Op == js_ast.BinOpLogicalOr || right.Op == js_ast.BinOpLogicalAnd) {
			v.rightLevel = js_ast.LPrefix
		}

	case js_ast.BinOpPow:
		// "**" can't contain certain unary expressions
		if left, ok := e.Left.Data.(*js_ast.EUnary); ok && left.Op.UnaryAssignTarget() == js_ast.AssignTargetNone {
			v.leftLevel = js_ast.LCall
		} else if _, ok := e.Left.Data.(*js_ast.EAwait); ok {
			v.leftLevel = js_ast.LCall
		} else if _, ok := e.Left.Data.(*js_ast.EUndefined); ok {
			// Undefined is printed as "void 0"
			v.leftLevel = js_ast.LCall
		} else if _, ok := e.Left.Data.(*js_ast.ENumber); ok {
			// Negative numbers are printed using a unary operator
			v.leftLevel = js_ast.LCall
		} else if p.options.MinifySyntax {
			// When minifying, booleans are printed as "!0 and "!1"
			if _, ok := e.Left.Data.(*js_ast.EBoolean); ok {
				v.leftLevel = js_ast.LCall
			}
		}
	}

	// Special-case "#foo in bar"
	if private, ok := e.Left.Data.(*js_ast.EPrivateIdentifier); ok && e.Op == js_ast.BinOpIn {
		name := p.renamer.NameForSymbol(private.Ref)
		p.addSourceMappingForName(e.Left.Loc, name, private.Ref)
		p.printIdentifier(name)
		v.visitRightAndFinish(p)
		return false
	}

	if e.Op == js_ast.BinOpComma {
		// The result of the left operand of the comma operator is unused
		v.leftFlags = (v.flags & forbidIn) | exprResultIsUnused | parentWasUnaryOrBinaryOrIfTest
	} else {
		v.leftFlags = (v.flags & forbidIn) | parentWasUnaryOrBinaryOrIfTest
	}
	return true
}

func (v *binaryExprVisitor) visitRightAndFinish(p *printer) {
	e := v.e

	if e.Op != js_ast.BinOpComma {
		p.printSpace()
	}

	if v.entry.IsKeyword {
		p.printSpaceBeforeIdentifier()
		p.print(v.entry.Text)
	} else {
		p.printSpaceBeforeOperator(e.Op)
		p.print(v.entry.Text)
		p.prevOp = e.Op
		p.prevOpEnd = len(p.js)
	}

	if p.options.LineLimit <= 0 || !p.printNewlinePastLineLimit() {
		p.printSpace()
	}

	if e.Op == js_ast.BinOpComma {
		// The result of the right operand of the comma operator is unused if the caller doesn't use it
		p.printExpr(e.Right, v.rightLevel, (v.flags&(forbidIn|exprResultIsUnused))|parentWasUnaryOrBinaryOrIfTest)
	} else {
		p.printExpr(e.Right, v.rightLevel, (v.flags&forbidIn)|parentWasUnaryOrBinaryOrIfTest)
	}

	if v.wrap {
		p.print(")")
	}
}

func (p *printer) isUnboundEvalIdentifier(value js_ast.Expr) bool {
	if id, ok := value.Data.(*js_ast.EIdentifier); ok {
		// Using the original name here is ok since unbound symbols are not renamed
		symbol := p.symbols.Get(ast.FollowSymbols(p.symbols, id.Ref))
		return symbol.Kind == ast.SymbolUnbound && symbol.OriginalName == "eval"
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

			// Integers always need a space before "." to avoid making a decimal point
			p.needSpaceBeforeDot = len(p.js)
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

	// Numbers in this range can potentially be printed with one fewer byte as
	// hex. This compares against 0xFFFF_FFFF_FFFF_F800 instead of comparing
	// against 0xFFFF_FFFF_FFFF_FFFF because 0xFFFF_FFFF_FFFF_FFFF when converted
	// to float64 rounds up to 0x1_0000_0000_0000_0180, which can no longer fit
	// into uint64. In Go, the result of converting float64 to uint64 outside of
	// the uint64 range is implementation-dependent and is different on amd64 vs.
	// arm64. The float64 value 0xFFFF_FFFF_FFFF_F800 is the biggest value that
	// is below the float64 value 0x1_0000_0000_0000_0180, so we use that instead.
	if p.options.MinifyWhitespace && absValue >= 1_000_000_000_000 && absValue <= 0xFFFF_FFFF_FFFF_F800 {
		if asInt := uint64(absValue); absValue == float64(asInt) {
			if hex := strconv.FormatUint(asInt, 16); 2+len(hex) < len(result) {
				result = append(append(result[:0], '0', 'x'), hex...)
			}
		}
	}

	p.printBytes(result)

	// We'll need a space before "." if it could be parsed as a decimal point
	if !bytes.ContainsAny(result, ".ex") {
		p.needSpaceBeforeDot = len(p.js)
	}
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
		case js_ast.LocalAwaitUsing:
			p.printDecls("await using", s.Decls, flags)
		case js_ast.LocalConst:
			p.printDecls("const", s.Decls, flags)
		case js_ast.LocalLet:
			p.printDecls("let", s.Decls, flags)
		case js_ast.LocalUsing:
			p.printDecls("using", s.Decls, flags)
		case js_ast.LocalVar:
			p.printDecls("var", s.Decls, flags)
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
			if p.options.LineLimit <= 0 || !p.printNewlinePastLineLimit() {
				p.printSpace()
			}
		}
		p.printBinding(decl.Binding)

		if decl.ValueOrNil.Data != nil {
			p.printSpace()
			p.print("=")
			p.printSpace()
			p.printExprWithoutLeadingNewline(decl.ValueOrNil, js_ast.LComma, flags)
		}
	}
}

func (p *printer) printBody(body js_ast.Stmt, isSingleLine bool) {
	if block, ok := body.Data.(*js_ast.SBlock); ok {
		p.printSpace()
		p.printBlock(body.Loc, *block)
		p.printNewline()
	} else if isSingleLine {
		p.printNextIndentAsSpace = true
		p.printStmt(body, 0)
	} else {
		p.printNewline()
		p.options.Indent++
		p.printStmt(body, 0)
		p.options.Indent--
	}
}

func (p *printer) printBlock(loc logger.Loc, block js_ast.SBlock) {
	p.addSourceMapping(loc)
	p.print("{")
	p.printNewline()

	p.options.Indent++
	for _, stmt := range block.Stmts {
		p.printSemicolonIfNeeded()
		p.printStmt(stmt, canOmitStatement)
	}
	p.options.Indent--
	p.needsSemicolon = false

	p.printIndent()
	if block.CloseBraceLoc.Start > loc.Start {
		p.addSourceMapping(block.CloseBraceLoc)
	}
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

		case *js_ast.SLabel:
			s = current.Stmt.Data

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
	if p.willPrintExprCommentsAtLoc(s.Test.Loc) {
		p.printNewline()
		p.options.Indent++
		p.printIndent()
		p.printExpr(s.Test, js_ast.LLowest, 0)
		p.printNewline()
		p.options.Indent--
		p.printIndent()
	} else {
		p.printExpr(s.Test, js_ast.LLowest, 0)
	}
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
		p.printBlock(s.Yes.Loc, *yes)

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
		p.printBody(s.Yes, s.IsSingleLineYes)

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
			p.printBlock(no.Loc, *block)
			p.printNewline()
		} else if ifStmt, ok := no.Data.(*js_ast.SIf); ok {
			p.printIf(ifStmt)
		} else {
			p.printBody(no, s.IsSingleLineNo)
		}
	}
}

func (p *printer) printIndentedComment(text string) {
	// Avoid generating a comment containing the character sequence "</script"
	if !p.options.UnsupportedFeatures.Has(compat.InlineScript) {
		text = helpers.EscapeClosingTag(text, "/script")
	}

	if strings.HasPrefix(text, "/*") {
		// Re-indent multi-line comments
		for {
			newline := strings.IndexByte(text, '\n')
			if newline == -1 {
				break
			}
			p.print(text[:newline+1])
			p.printIndent()
			text = text[newline+1:]
		}
		p.print(text)
		p.printNewline()
	} else {
		// Print a mandatory newline after single-line comments
		p.print(text)
		p.print("\n")
	}
}

func (p *printer) printPath(importRecordIndex uint32, importKind ast.ImportKind) {
	record := p.importRecords[importRecordIndex]
	p.addSourceMapping(record.Range.Loc)
	p.printQuotedUTF8(record.Path.Text, printQuotedNoWrap)

	if p.options.NeedsMetafile {
		external := ""
		if (record.Flags & ast.ShouldNotBeExternalInMetafile) == 0 {
			external = ",\n          \"external\": true"
		}
		p.jsonMetadataImports = append(p.jsonMetadataImports, fmt.Sprintf("\n        {\n          \"path\": %s,\n          \"kind\": %s%s\n        }",
			helpers.QuoteForJSON(record.Path.Text, p.options.ASCIIOnly),
			helpers.QuoteForJSON(importKind.StringForMetafile(), p.options.ASCIIOnly),
			external))
	}

	if record.AssertOrWith != nil && importKind == ast.ImportStmt {
		feature := compat.ImportAttributes
		if record.AssertOrWith.Keyword == ast.AssertKeyword {
			feature = compat.ImportAssertions
		}

		// Omit import assertions/attributes on this import statement if they would cause a syntax error
		if p.options.UnsupportedFeatures.Has(feature) {
			return
		}

		p.printSpace()
		p.addSourceMapping(record.AssertOrWith.KeywordLoc)
		p.print(record.AssertOrWith.Keyword.String())
		p.printSpace()
		p.printImportAssertOrWithClause(*record.AssertOrWith)
	}
}

func (p *printer) printImportCallAssertOrWith(assertOrWith *ast.ImportAssertOrWith, outerIsMultiLine bool) {
	// Omit import assertions/attributes if we know the "import()" syntax doesn't
	// support a second argument (i.e. both import assertions and import
	// attributes aren't supported) and doing so would cause a syntax error
	if assertOrWith == nil || (p.options.UnsupportedFeatures.Has(compat.ImportAssertions) && p.options.UnsupportedFeatures.Has(compat.ImportAttributes)) {
		return
	}

	isMultiLine := p.willPrintExprCommentsAtLoc(assertOrWith.KeywordLoc) ||
		p.willPrintExprCommentsAtLoc(assertOrWith.InnerOpenBraceLoc) ||
		p.willPrintExprCommentsAtLoc(assertOrWith.OuterCloseBraceLoc)

	p.print(",")
	if outerIsMultiLine {
		p.printNewline()
		p.printIndent()
	} else {
		p.printSpace()
	}
	p.printExprCommentsAtLoc(assertOrWith.OuterOpenBraceLoc)
	p.addSourceMapping(assertOrWith.OuterOpenBraceLoc)
	p.print("{")

	if isMultiLine {
		p.printNewline()
		p.options.Indent++
		p.printIndent()
	} else {
		p.printSpace()
	}

	p.printExprCommentsAtLoc(assertOrWith.KeywordLoc)
	p.addSourceMapping(assertOrWith.KeywordLoc)
	p.print(assertOrWith.Keyword.String())
	p.print(":")

	if p.willPrintExprCommentsAtLoc(assertOrWith.InnerOpenBraceLoc) {
		p.printNewline()
		p.options.Indent++
		p.printIndent()
		p.printExprCommentsAtLoc(assertOrWith.InnerOpenBraceLoc)
		p.printImportAssertOrWithClause(*assertOrWith)
		p.options.Indent--
	} else {
		p.printSpace()
		p.printImportAssertOrWithClause(*assertOrWith)
	}

	if isMultiLine {
		p.printNewline()
		p.printExprCommentsAfterCloseTokenAtLoc(assertOrWith.OuterCloseBraceLoc)
		p.options.Indent--
		p.printIndent()
	} else {
		p.printSpace()
	}

	p.addSourceMapping(assertOrWith.OuterCloseBraceLoc)
	p.print("}")
}

func (p *printer) printImportAssertOrWithClause(assertOrWith ast.ImportAssertOrWith) {
	isMultiLine := p.willPrintExprCommentsAtLoc(assertOrWith.InnerCloseBraceLoc)
	if !isMultiLine {
		for _, entry := range assertOrWith.Entries {
			if p.willPrintExprCommentsAtLoc(entry.KeyLoc) || p.willPrintExprCommentsAtLoc(entry.ValueLoc) {
				isMultiLine = true
				break
			}
		}
	}

	p.addSourceMapping(assertOrWith.InnerOpenBraceLoc)
	p.print("{")
	if isMultiLine {
		p.options.Indent++
	}

	for i, entry := range assertOrWith.Entries {
		if i > 0 {
			p.print(",")
		}
		if isMultiLine {
			p.printNewline()
			p.printIndent()
		} else {
			p.printSpace()
		}

		p.printExprCommentsAtLoc(entry.KeyLoc)
		p.addSourceMapping(entry.KeyLoc)
		if !entry.PreferQuotedKey && p.canPrintIdentifierUTF16(entry.Key) {
			p.printSpaceBeforeIdentifier()
			p.printIdentifierUTF16(entry.Key)
		} else {
			p.printQuotedUTF16(entry.Key, 0)
		}

		p.print(":")

		if p.willPrintExprCommentsAtLoc(entry.ValueLoc) {
			p.printNewline()
			p.options.Indent++
			p.printIndent()
			p.printExprCommentsAtLoc(entry.ValueLoc)
			p.addSourceMapping(entry.ValueLoc)
			p.printQuotedUTF16(entry.Value, 0)
			p.options.Indent--
		} else {
			p.printSpace()
			p.addSourceMapping(entry.ValueLoc)
			p.printQuotedUTF16(entry.Value, 0)
		}
	}

	if isMultiLine {
		p.printNewline()
		p.printExprCommentsAfterCloseTokenAtLoc(assertOrWith.InnerCloseBraceLoc)
		p.options.Indent--
		p.printIndent()
	} else if len(assertOrWith.Entries) > 0 {
		p.printSpace()
	}

	p.addSourceMapping(assertOrWith.InnerCloseBraceLoc)
	p.print("}")
}

type printStmtFlags uint8

const (
	canOmitStatement printStmtFlags = 1 << iota
)

func (p *printer) printStmt(stmt js_ast.Stmt, flags printStmtFlags) {
	if p.options.LineLimit > 0 {
		p.printNewlinePastLineLimit()
	}

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

				// Don't record the same legal comment more than once per file
				if p.hasLegalComment == nil {
					p.hasLegalComment = make(map[string]struct{})
				} else if _, ok := p.hasLegalComment[text]; ok {
					return
				}
				p.hasLegalComment[text] = struct{}{}
				p.extractedLegalComments = append(p.extractedLegalComments, text)
				return
			}
		}

		p.printIndent()
		p.addSourceMapping(stmt.Loc)
		p.printIndentedComment(text)

	case *js_ast.SFunction:
		if !p.options.MinifyWhitespace && s.Fn.HasNoSideEffectsComment {
			p.printIndent()
			p.print("// @__NO_SIDE_EFFECTS__\n")
		}
		p.addSourceMapping(stmt.Loc)
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
		p.printSpaceBeforeIdentifier()
		name := p.renamer.NameForSymbol(s.Fn.Name.Ref)
		p.addSourceMappingForName(s.Fn.Name.Loc, name, s.Fn.Name.Ref)
		p.printIdentifier(name)
		p.printFn(s.Fn)
		p.printNewline()

	case *js_ast.SClass:
		omitIndent := p.printDecorators(s.Class.Decorators, printNewlineAfterDecorator)
		if !omitIndent {
			p.printIndent()
		}
		p.printSpaceBeforeIdentifier()
		p.addSourceMapping(stmt.Loc)
		if s.IsExport {
			p.print("export ")
		}
		p.print("class ")
		name := p.renamer.NameForSymbol(s.Class.Name.Ref)
		p.addSourceMappingForName(s.Class.Name.Loc, name, s.Class.Name.Ref)
		p.printIdentifier(name)
		p.printClass(s.Class)
		p.printNewline()

	case *js_ast.SEmpty:
		p.addSourceMapping(stmt.Loc)
		p.printIndent()
		p.print(";")
		p.printNewline()

	case *js_ast.SExportDefault:
		if !p.options.MinifyWhitespace {
			if s2, ok := s.Value.Data.(*js_ast.SFunction); ok && s2.Fn.HasNoSideEffectsComment {
				p.printIndent()
				p.print("// @__NO_SIDE_EFFECTS__\n")
			}
		}
		omitIndent := false
		if s2, ok := s.Value.Data.(*js_ast.SClass); ok {
			omitIndent = p.printDecorators(s2.Class.Decorators, printNewlineAfterDecorator)
		}
		p.addSourceMapping(stmt.Loc)
		if !omitIndent {
			p.printIndent()
		}
		p.printSpaceBeforeIdentifier()
		p.print("export default")
		p.printSpace()

		switch s2 := s.Value.Data.(type) {
		case *js_ast.SExpr:
			// Functions and classes must be wrapped to avoid confusion with their statement forms
			p.exportDefaultStart = len(p.js)

			p.printExprWithoutLeadingNewline(s2.Value, js_ast.LComma, 0)
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
				p.printSpaceBeforeIdentifier()
				name := p.renamer.NameForSymbol(s2.Fn.Name.Ref)
				p.addSourceMappingForName(s2.Fn.Name.Loc, name, s2.Fn.Name.Ref)
				p.printIdentifier(name)
			}
			p.printFn(s2.Fn)
			p.printNewline()

		case *js_ast.SClass:
			p.printSpaceBeforeIdentifier()
			p.print("class")
			if s2.Class.Name != nil {
				p.print(" ")
				name := p.renamer.NameForSymbol(s2.Class.Name.Ref)
				p.addSourceMappingForName(s2.Class.Name.Loc, name, s2.Class.Name.Ref)
				p.printIdentifier(name)
			}
			p.printClass(s2.Class)
			p.printNewline()

		default:
			panic("Internal error")
		}

	case *js_ast.SExportStar:
		p.addSourceMapping(stmt.Loc)
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("export")
		p.printSpace()
		p.print("*")
		p.printSpace()
		if s.Alias != nil {
			p.print("as")
			p.printSpace()
			p.printClauseAlias(s.Alias.Loc, s.Alias.OriginalName)
			p.printSpace()
			p.printSpaceBeforeIdentifier()
		}
		p.print("from")
		p.printSpace()
		p.printPath(s.ImportRecordIndex, ast.ImportStmt)
		p.printSemicolonAfterStatement()

	case *js_ast.SExportClause:
		p.addSourceMapping(stmt.Loc)
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

			if p.options.LineLimit <= 0 || !p.printNewlinePastLineLimit() {
				if s.IsSingleLine {
					p.printSpace()
				} else {
					p.printNewline()
					p.printIndent()
				}
			}

			name := p.renamer.NameForSymbol(item.Name.Ref)
			p.addSourceMappingForName(item.Name.Loc, name, item.Name.Ref)
			p.printIdentifier(name)
			if name != item.Alias {
				p.print(" as")
				p.printSpace()
				p.printClauseAlias(item.AliasLoc, item.Alias)
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
		p.addSourceMapping(stmt.Loc)
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

			if p.options.LineLimit <= 0 || !p.printNewlinePastLineLimit() {
				if s.IsSingleLine {
					p.printSpace()
				} else {
					p.printNewline()
					p.printIndent()
				}
			}

			p.printClauseAlias(item.Name.Loc, item.OriginalName)
			if item.OriginalName != item.Alias {
				p.printSpace()
				p.printSpaceBeforeIdentifier()
				p.print("as")
				p.printSpace()
				p.printClauseAlias(item.AliasLoc, item.Alias)
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
		p.printPath(s.ImportRecordIndex, ast.ImportStmt)
		p.printSemicolonAfterStatement()

	case *js_ast.SLocal:
		p.addSourceMapping(stmt.Loc)
		switch s.Kind {
		case js_ast.LocalAwaitUsing:
			p.printDeclStmt(s.IsExport, "await using", s.Decls)
		case js_ast.LocalConst:
			p.printDeclStmt(s.IsExport, "const", s.Decls)
		case js_ast.LocalLet:
			p.printDeclStmt(s.IsExport, "let", s.Decls)
		case js_ast.LocalUsing:
			p.printDeclStmt(s.IsExport, "using", s.Decls)
		case js_ast.LocalVar:
			p.printDeclStmt(s.IsExport, "var", s.Decls)
		}

	case *js_ast.SIf:
		p.addSourceMapping(stmt.Loc)
		p.printIndent()
		p.printIf(s)

	case *js_ast.SDoWhile:
		p.addSourceMapping(stmt.Loc)
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("do")
		if block, ok := s.Body.Data.(*js_ast.SBlock); ok {
			p.printSpace()
			p.printBlock(s.Body.Loc, *block)
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
		if p.willPrintExprCommentsAtLoc(s.Test.Loc) {
			p.printNewline()
			p.options.Indent++
			p.printIndent()
			p.printExpr(s.Test, js_ast.LLowest, 0)
			p.printNewline()
			p.options.Indent--
			p.printIndent()
		} else {
			p.printExpr(s.Test, js_ast.LLowest, 0)
		}
		p.print(")")
		p.printSemicolonAfterStatement()

	case *js_ast.SForIn:
		p.addSourceMapping(stmt.Loc)
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("for")
		p.printSpace()
		p.print("(")
		hasInitComment := p.willPrintExprCommentsAtLoc(s.Init.Loc)
		hasValueComment := p.willPrintExprCommentsAtLoc(s.Value.Loc)
		if hasInitComment || hasValueComment {
			p.printNewline()
			p.options.Indent++
			p.printIndent()
		}
		p.printForLoopInit(s.Init, forbidIn)
		p.printSpace()
		p.printSpaceBeforeIdentifier()
		p.print("in")
		if hasValueComment {
			p.printNewline()
			p.printIndent()
		} else {
			p.printSpace()
		}
		p.printExpr(s.Value, js_ast.LLowest, 0)
		if hasInitComment || hasValueComment {
			p.printNewline()
			p.options.Indent--
			p.printIndent()
		}
		p.print(")")
		p.printBody(s.Body, s.IsSingleLineBody)

	case *js_ast.SForOf:
		p.addSourceMapping(stmt.Loc)
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("for")
		if s.Await.Len > 0 {
			p.print(" await")
		}
		p.printSpace()
		p.print("(")
		hasInitComment := p.willPrintExprCommentsAtLoc(s.Init.Loc)
		hasValueComment := p.willPrintExprCommentsAtLoc(s.Value.Loc)
		flags := forbidIn | isFollowedByOf
		if s.Await.Len > 0 {
			flags |= isInsideForAwait
		}
		if hasInitComment || hasValueComment {
			p.printNewline()
			p.options.Indent++
			p.printIndent()
		}
		p.forOfInitStart = len(p.js)
		p.printForLoopInit(s.Init, flags)
		p.printSpace()
		p.printSpaceBeforeIdentifier()
		p.print("of")
		if hasValueComment {
			p.printNewline()
			p.printIndent()
		} else {
			p.printSpace()
		}
		p.printExpr(s.Value, js_ast.LComma, 0)
		if hasInitComment || hasValueComment {
			p.printNewline()
			p.options.Indent--
			p.printIndent()
		}
		p.print(")")
		p.printBody(s.Body, s.IsSingleLineBody)

	case *js_ast.SWhile:
		p.addSourceMapping(stmt.Loc)
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("while")
		p.printSpace()
		p.print("(")
		if p.willPrintExprCommentsAtLoc(s.Test.Loc) {
			p.printNewline()
			p.options.Indent++
			p.printIndent()
			p.printExpr(s.Test, js_ast.LLowest, 0)
			p.printNewline()
			p.options.Indent--
			p.printIndent()
		} else {
			p.printExpr(s.Test, js_ast.LLowest, 0)
		}
		p.print(")")
		p.printBody(s.Body, s.IsSingleLineBody)

	case *js_ast.SWith:
		p.addSourceMapping(stmt.Loc)
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("with")
		p.printSpace()
		p.print("(")
		if p.willPrintExprCommentsAtLoc(s.Value.Loc) {
			p.printNewline()
			p.options.Indent++
			p.printIndent()
			p.printExpr(s.Value, js_ast.LLowest, 0)
			p.printNewline()
			p.options.Indent--
			p.printIndent()
		} else {
			p.printExpr(s.Value, js_ast.LLowest, 0)
		}
		p.print(")")
		p.withNesting++
		p.printBody(s.Body, s.IsSingleLineBody)
		p.withNesting--

	case *js_ast.SLabel:
		// Avoid printing a source mapping that masks the one from the label
		if !p.options.MinifyWhitespace && (p.options.Indent > 0 || p.printNextIndentAsSpace) {
			p.addSourceMapping(stmt.Loc)
			p.printIndent()
		}

		p.printSpaceBeforeIdentifier()
		name := p.renamer.NameForSymbol(s.Name.Ref)
		p.addSourceMappingForName(s.Name.Loc, name, s.Name.Ref)
		p.printIdentifier(name)
		p.print(":")
		p.printBody(s.Stmt, s.IsSingleLineStmt)

	case *js_ast.STry:
		p.addSourceMapping(stmt.Loc)
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("try")
		p.printSpace()
		p.printBlock(s.BlockLoc, s.Block)

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
			p.printBlock(s.Catch.BlockLoc, s.Catch.Block)
		}

		if s.Finally != nil {
			p.printSpace()
			p.print("finally")
			p.printSpace()
			p.printBlock(s.Finally.Loc, s.Finally.Block)
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

		p.addSourceMapping(stmt.Loc)
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("for")
		p.printSpace()
		p.print("(")
		isMultiLine :=
			(init.Data != nil && p.willPrintExprCommentsAtLoc(init.Loc)) ||
				(s.TestOrNil.Data != nil && p.willPrintExprCommentsAtLoc(s.TestOrNil.Loc)) ||
				(update.Data != nil && p.willPrintExprCommentsAtLoc(update.Loc))
		if isMultiLine {
			p.printNewline()
			p.options.Indent++
			p.printIndent()
		}
		if init.Data != nil {
			p.printForLoopInit(init, forbidIn)
		}
		p.print(";")
		if isMultiLine {
			p.printNewline()
			p.printIndent()
		} else {
			p.printSpace()
		}
		if s.TestOrNil.Data != nil {
			p.printExpr(s.TestOrNil, js_ast.LLowest, 0)
		}
		p.print(";")
		if !isMultiLine {
			p.printSpace()
		} else if update.Data != nil {
			p.printNewline()
			p.printIndent()
		}
		if update.Data != nil {
			p.printExpr(update, js_ast.LLowest, exprResultIsUnused)
		}
		if isMultiLine {
			p.printNewline()
			p.options.Indent--
			p.printIndent()
		}
		p.print(")")
		p.printBody(s.Body, s.IsSingleLineBody)

	case *js_ast.SSwitch:
		p.addSourceMapping(stmt.Loc)
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("switch")
		p.printSpace()
		p.print("(")
		if p.willPrintExprCommentsAtLoc(s.Test.Loc) {
			p.printNewline()
			p.options.Indent++
			p.printIndent()
			p.printExpr(s.Test, js_ast.LLowest, 0)
			p.printNewline()
			p.options.Indent--
			p.printIndent()
		} else {
			p.printExpr(s.Test, js_ast.LLowest, 0)
		}
		p.print(")")
		p.printSpace()
		p.addSourceMapping(s.BodyLoc)
		p.print("{")
		p.printNewline()
		p.options.Indent++

		for _, c := range s.Cases {
			p.printSemicolonIfNeeded()
			p.printIndent()
			p.printExprCommentsAtLoc(c.Loc)
			p.addSourceMapping(c.Loc)

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
					p.printBlock(c.Body[0].Loc, *block)
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
		p.addSourceMapping(s.CloseBraceLoc)
		p.print("}")
		p.printNewline()
		p.needsSemicolon = false

	case *js_ast.SImport:
		itemCount := 0

		p.addSourceMapping(stmt.Loc)
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("import")
		p.printSpace()

		if s.DefaultName != nil {
			p.printSpaceBeforeIdentifier()
			name := p.renamer.NameForSymbol(s.DefaultName.Ref)
			p.addSourceMappingForName(s.DefaultName.Loc, name, s.DefaultName.Ref)
			p.printIdentifier(name)
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

				if p.options.LineLimit <= 0 || !p.printNewlinePastLineLimit() {
					if s.IsSingleLine {
						p.printSpace()
					} else {
						p.printNewline()
						p.printIndent()
					}
				}

				p.printClauseAlias(item.AliasLoc, item.Alias)

				name := p.renamer.NameForSymbol(item.Name.Ref)
				if name != item.Alias {
					p.printSpace()
					p.printSpaceBeforeIdentifier()
					p.print("as ")
					p.addSourceMappingForName(item.Name.Loc, name, item.Name.Ref)
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
			name := p.renamer.NameForSymbol(s.NamespaceRef)
			p.addSourceMappingForName(*s.StarNameLoc, name, s.NamespaceRef)
			p.printIdentifier(name)
			itemCount++
		}

		if itemCount > 0 {
			p.printSpace()
			p.printSpaceBeforeIdentifier()
			p.print("from")
			p.printSpace()
		}

		p.printPath(s.ImportRecordIndex, ast.ImportStmt)
		p.printSemicolonAfterStatement()

	case *js_ast.SBlock:
		p.addSourceMapping(stmt.Loc)
		p.printIndent()
		p.printBlock(stmt.Loc, *s)
		p.printNewline()

	case *js_ast.SDebugger:
		p.addSourceMapping(stmt.Loc)
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("debugger")
		p.printSemicolonAfterStatement()

	case *js_ast.SDirective:
		p.addSourceMapping(stmt.Loc)
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.printQuotedUTF16(s.Value, 0)
		p.printSemicolonAfterStatement()

	case *js_ast.SBreak:
		p.addSourceMapping(stmt.Loc)
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("break")
		if s.Label != nil {
			p.print(" ")
			name := p.renamer.NameForSymbol(s.Label.Ref)
			p.addSourceMappingForName(s.Label.Loc, name, s.Label.Ref)
			p.printIdentifier(name)
		}
		p.printSemicolonAfterStatement()

	case *js_ast.SContinue:
		p.addSourceMapping(stmt.Loc)
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("continue")
		if s.Label != nil {
			p.print(" ")
			name := p.renamer.NameForSymbol(s.Label.Ref)
			p.addSourceMappingForName(s.Label.Loc, name, s.Label.Ref)
			p.printIdentifier(name)
		}
		p.printSemicolonAfterStatement()

	case *js_ast.SReturn:
		p.addSourceMapping(stmt.Loc)
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("return")
		if s.ValueOrNil.Data != nil {
			p.printSpace()
			p.printExprWithoutLeadingNewline(s.ValueOrNil, js_ast.LLowest, 0)
		}
		p.printSemicolonAfterStatement()

	case *js_ast.SThrow:
		p.addSourceMapping(stmt.Loc)
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("throw")
		p.printSpace()
		p.printExprWithoutLeadingNewline(s.Value, js_ast.LLowest, 0)
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
					p.addSourceMapping(stmt.Loc)
					p.printIndent()
					p.print(";")
					p.printNewline()
				} else {
					// "if (x) { empty(); }" => "if (x) {}"
				}
				break
			}
		}

		// Avoid printing a source mapping when the expression would print one in
		// the same spot. We don't want to accidentally mask the mapping it emits.
		if !p.options.MinifyWhitespace && (p.options.Indent > 0 || p.printNextIndentAsSpace) {
			p.addSourceMapping(stmt.Loc)
			p.printIndent()
		}

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
	TSEnums map[ast.Ref]map[string]js_ast.TSEnumValue

	// Cross-module inlining of detected inlinable constants is also done during printing
	ConstValues map[ast.Ref]js_ast.ConstValue

	// Property mangling results go here
	MangledProps map[ast.Ref]string

	// This will be present if the input file had a source map. In that case we
	// want to map all the way back to the original input file(s).
	InputSourceMap *sourcemap.SourceMap

	// If we're writing out a source map, this table of line start indices lets
	// us do binary search on to figure out what line a given AST node came from
	LineOffsetTables []sourcemap.LineOffsetTable

	ToCommonJSRef       ast.Ref
	ToESMRef            ast.Ref
	RuntimeRequireRef   ast.Ref
	UnsupportedFeatures compat.JSFeature
	Indent              int
	LineLimit           int
	OutputFormat        config.Format
	MinifyWhitespace    bool
	MinifyIdentifiers   bool
	MinifySyntax        bool
	ASCIIOnly           bool
	LegalComments       config.LegalComments
	SourceMap           config.SourceMap
	AddSourceMappings   bool
	NeedsMetafile       bool
}

type RequireOrImportMeta struct {
	// CommonJS files will return the "require_*" wrapper function and an invalid
	// exports object reference. Lazily-initialized ESM files will return the
	// "init_*" wrapper function and the exports object for that file.
	WrapperRef     ast.Ref
	ExportsRef     ast.Ref
	IsWrapperAsync bool
}

type PrintResult struct {
	JS                     []byte
	ExtractedLegalComments []string
	JSONMetadataImports    []string

	// This source map chunk just contains the VLQ-encoded offsets for the "JS"
	// field above. It's not a full source map. The bundler will be joining many
	// source map chunks together to form the final source map.
	SourceMapChunk sourcemap.Chunk
}

func Print(tree js_ast.AST, symbols ast.SymbolMap, r renamer.Renamer, options Options) PrintResult {
	p := &printer{
		symbols:       symbols,
		renamer:       r,
		importRecords: tree.ImportRecords,
		options:       options,
		moduleType:    tree.ModuleTypeData.Type,
		exprComments:  tree.ExprComments,
		wasLazyExport: tree.HasLazyExport,

		stmtStart:          -1,
		exportDefaultStart: -1,
		arrowExprStart:     -1,
		forOfInitStart:     -1,

		prevOpEnd:            -1,
		needSpaceBeforeDot:   -1,
		prevRegExpEnd:        -1,
		noLeadingNewlineHere: -1,
		builder:              sourcemap.MakeChunkBuilder(options.InputSourceMap, options.LineOffsetTables, options.ASCIIOnly),
	}

	if p.exprComments != nil {
		p.printedExprComments = make(map[logger.Loc]bool)
	}

	p.astHelpers = js_ast.MakeHelperContext(func(ref ast.Ref) bool {
		ref = ast.FollowSymbols(symbols, ref)
		return symbols.Get(ref).Kind == ast.SymbolUnbound
	})

	// Add the top-level directive if present
	for _, directive := range tree.Directives {
		p.printIndent()
		p.printQuotedUTF8(directive, 0)
		p.print(";")
		p.printNewline()
	}

	for _, part := range tree.Parts {
		for _, stmt := range part.Stmts {
			p.printStmt(stmt, canOmitStatement)
			p.printSemicolonIfNeeded()
		}
	}

	result := PrintResult{
		JS:                     p.js,
		JSONMetadataImports:    p.jsonMetadataImports,
		ExtractedLegalComments: p.extractedLegalComments,
	}
	if options.SourceMap != config.SourceMapNone {
		// This is expensive. Only do this if it's necessary.
		result.SourceMapChunk = p.builder.GenerateChunk(p.js)
	}
	return result
}
