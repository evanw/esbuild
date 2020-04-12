package printer

import (
	"bytes"
	"esbuild/ast"
	"esbuild/lexer"
	"fmt"
	"math"
	"strconv"
	"strings"
)

var base64 = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/")
var positiveInfinity = math.Inf(1)
var negativeInfinity = math.Inf(-1)

// A single base 64 digit can contain 6 bits of data. For the base 64 variable
// length quantities we use in the source map spec, the first bit is the sign,
// the next four bits are the actual value, and the 6th bit is the continuation
// bit. The continuation bit tells us whether there are more digits in this
// value following this digit.
//
//   Continuation
//   |    Sign
//   |    |
//   V    V
//   101011
//
func encodeVLQ(value int) []byte {
	var vlq int
	if value < 0 {
		vlq = ((-value) << 1) | 1
	} else {
		vlq = value << 1
	}

	// Handle the common case up front without allocations
	if (vlq >> 5) == 0 {
		digit := vlq & 31
		return base64[digit : digit+1]
	}

	encoded := []byte{}
	for {
		digit := vlq & 31
		vlq >>= 5

		// If there are still more digits in this value, we must make sure the
		// continuation bit is marked
		if vlq != 0 {
			digit |= 32
		}

		encoded = append(encoded, base64[digit])

		if vlq == 0 {
			break
		}
	}

	return encoded
}

func decodeVLQ(encoded []byte, start int) (int, int) {
	var vlq = 0

	// Scan over the input
	for {
		index := bytes.IndexByte(base64, encoded[start])
		if index < 0 {
			break
		}

		// Decode a single byte
		vlq = (vlq << 5) | (index & 31)
		start++

		// Stop if there's no continuation bit
		if (vlq & 32) == 0 {
			break
		}
	}

	// Recover the value
	var value = vlq >> 1
	if (vlq & 1) != 0 {
		value = -value
	}
	return value, start
}

// Coordinates in source maps are stored using relative offsets for size
// reasons. When joining together chunks of a source map that were emitted
// in parallel for different parts of a file, we need to fix up the first
// segment of each chunk to be relative to the end of the previous chunk.
type SourceMapState struct {
	// This isn't stored in the source map. It's only used by the bundler to join
	// source map chunks together correctly.
	GeneratedLine int

	// These are stored in the source map in VLQ format.
	GeneratedColumn int
	SourceIndex     int
	OriginalLine    int
	OriginalColumn  int
}

// Source map chunks are computed in parallel for speed. Each chunk is relative
// to the zero state instead of being relative to the end state of the previous
// chunk, since it's impossible to know the end state of the previous chunk in
// a parallel computation.
//
// After all chunks are computed, they are joined together in a second pass.
// This rewrites the first mapping in each chunk to be relative to the end
// state of the previous chunk.
func AppendSourceMapChunk(buffer []byte, prevEndState SourceMapState, startState SourceMapState, sourceMap []byte) []byte {
	// Strip off the first mapping from the buffer. The first mapping should be
	// for the start of the original file (the printer always generates one for
	// the start of the file).
	generatedColumn, i := decodeVLQ(sourceMap, 0)
	sourceIndex, i := decodeVLQ(sourceMap, i)
	originalLine, i := decodeVLQ(sourceMap, i)
	originalColumn, i := decodeVLQ(sourceMap, i)
	sourceMap = sourceMap[i:]

	// Enforce invariants. All source map chunks should be relative to a default
	// zero state. This is because they are computed in parallel and it's not
	// possible to know what they should be relative to when computing them.
	if sourceIndex != 0 || originalLine != 0 || originalColumn != 0 {
		panic("Internal error")
	}

	// Rewrite the first mapping to be relative to the end state of the previous
	// chunk. We now know what the end state is because we're in the second pass
	// where all chunks have already been generated.
	startState.GeneratedColumn += generatedColumn
	buffer = appendMapping(buffer, prevEndState, startState)

	// Then append everything after that without modification.
	buffer = append(buffer, sourceMap...)
	return buffer
}

func appendMapping(buffer []byte, prevState SourceMapState, currentState SourceMapState) []byte {
	// Put commas in between mappings
	if len(buffer) != 0 {
		c := buffer[len(buffer)-1]
		if c != ';' && c != '"' {
			buffer = append(buffer, ',')
		}
	}

	// Record the generated column (the line is recorded using ';' elsewhere)
	buffer = append(buffer, encodeVLQ(currentState.GeneratedColumn-prevState.GeneratedColumn)...)
	prevState.GeneratedColumn = currentState.GeneratedColumn

	// Record the generated source
	buffer = append(buffer, encodeVLQ(currentState.SourceIndex-prevState.SourceIndex)...)
	prevState.SourceIndex = currentState.SourceIndex

	// Record the original line
	buffer = append(buffer, encodeVLQ(currentState.OriginalLine-prevState.OriginalLine)...)
	prevState.OriginalLine = currentState.OriginalLine

	// Record the original column
	buffer = append(buffer, encodeVLQ(currentState.OriginalColumn-prevState.OriginalColumn)...)
	prevState.OriginalColumn = currentState.OriginalColumn

	return buffer
}

const hexChars = "0123456789ABCDEF"

func QuoteForJSON(text string) string {
	return quoteImpl(text, true)
}

func Quote(text string) string {
	return quoteImpl(text, false)
}

func quoteImpl(text string, forJSON bool) string {
	b := strings.Builder{}
	i := 0
	n := len(text)
	b.WriteByte('"')

	for i < n {
		c := text[i]

		// Fast path: a run of characters that don't need escaping
		if c >= 0x20 && c <= 0x7E && c != '\\' && c != '"' {
			start := i
			i += 1
			for i < n {
				c = text[i]
				if c < 0x20 || c > 0x7E || c == '\\' || c == '"' {
					break
				}
				i += 1
			}
			b.WriteString(text[start:i])
			continue
		}

		switch c {
		case '\b':
			b.WriteString("\\b")
			i++

		case '\f':
			b.WriteString("\\f")
			i++

		case '\n':
			b.WriteString("\\n")
			i++

		case '\r':
			b.WriteString("\\r")
			i++

		case '\t':
			b.WriteString("\\t")
			i++

		case '\v':
			b.WriteString("\\v")
			i++

		case '\\':
			b.WriteString("\\\\")
			i++

		case '"':
			b.WriteString("\\\"")
			i++

		default:
			r, width := lexer.DecodeUTF8Rune(text[i:])
			i += width
			if r <= 0xFF && !forJSON {
				b.WriteString("\\x")
				b.WriteByte(hexChars[r>>4])
				b.WriteByte(hexChars[r&15])
			} else if r <= 0xFFFF {
				b.WriteString("\\u")
				b.WriteByte(hexChars[r>>12])
				b.WriteByte(hexChars[(r>>8)&15])
				b.WriteByte(hexChars[(r>>4)&15])
				b.WriteByte(hexChars[r&15])
			} else {
				r -= 0x10000
				lo := 0xD800 + ((r >> 10) & 0x3FF)
				hi := 0xDC00 + (r & 0x3FF)
				b.WriteString("\\u")
				b.WriteByte(hexChars[lo>>12])
				b.WriteByte(hexChars[(lo>>8)&15])
				b.WriteByte(hexChars[(lo>>4)&15])
				b.WriteByte(hexChars[lo&15])
				b.WriteString("\\u")
				b.WriteByte(hexChars[hi>>12])
				b.WriteByte(hexChars[(hi>>8)&15])
				b.WriteByte(hexChars[(hi>>4)&15])
				b.WriteByte(hexChars[hi&15])
			}
		}
	}

	b.WriteByte('"')
	return b.String()
}

func quoteUTF16(text []uint16, quote rune) string {
	b := strings.Builder{}
	i := 0
	n := len(text)

	for i < n {
		c := text[i]
		i++

		switch c {
		case '\x00':
			// We don't want "\x001" to be written as "\01"
			if i >= n || text[i] < '0' || text[i] > '9' {
				b.WriteString("\\0")
			} else {
				b.WriteString("\\x00")
			}

		case '\b':
			b.WriteString("\\b")

		case '\f':
			b.WriteString("\\f")

		case '\n':
			if quote == '`' {
				b.WriteByte('\n')
			} else {
				b.WriteString("\\n")
			}

		case '\r':
			b.WriteString("\\r")

		case '\v':
			b.WriteString("\\v")

		case '\\':
			b.WriteString("\\\\")

		case '\'':
			if quote == '\'' {
				b.WriteByte('\\')
			}
			b.WriteByte('\'')

		case '"':
			if quote == '"' {
				b.WriteByte('\\')
			}
			b.WriteByte('"')

		case '`':
			if quote == '`' {
				b.WriteByte('\\')
			}
			b.WriteByte('`')

		case '\u2028':
			b.WriteString("\\u2028")

		case '\u2029':
			b.WriteString("\\u2029")

		default:
			switch {
			// Is this a high surrogate?
			case c >= 0xD800 && c <= 0xDBFF:
				// Is there a next character?
				if i < n {
					c2 := text[i]

					// Is it a low surrogate?
					if c2 >= 0xDC00 && c2 <= 0xDFFF {
						i++
						b.WriteRune((rune(c) << 10) + rune(c2) + (0x10000 - (0xD800 << 10) - 0xDC00))
						continue
					}
				}

				// Write an escaped character
				b.WriteString("\\u")
				b.WriteByte(hexChars[c>>12])
				b.WriteByte(hexChars[(c>>8)&15])
				b.WriteByte(hexChars[(c>>4)&15])
				b.WriteByte(hexChars[c&15])
				continue

			// Is this a low surrogate?
			case c >= 0xDC00 && c <= 0xDFFF:
				// Write an escaped character
				b.WriteString("\\u")
				b.WriteByte(hexChars[c>>12])
				b.WriteByte(hexChars[(c>>8)&15])
				b.WriteByte(hexChars[(c>>4)&15])
				b.WriteByte(hexChars[c&15])
				continue

			default:
				b.WriteRune(rune(c))
			}
		}
	}

	return b.String()
}

type printer struct {
	symbols            *ast.SymbolMap
	minify             bool
	needsSemicolon     bool
	indent             int
	js                 []byte
	stmtStart          int
	exportDefaultStart int
	arrowExprStart     int
	prevOp             ast.OpCode
	prevOpEnd          int
	prevNumEnd         int
	prevRegExpEnd      int

	// For imports
	resolvedImports     map[string]uint32
	indirectImportItems map[ast.Ref]bool
	requireRef          ast.Ref

	// For source maps
	writeSourceMap bool
	sourceMap      []byte
	prevLoc        ast.Loc
	prevLineStart  int
	prevState      SourceMapState
	lineStarts     []int32
}

func (p *printer) print(text string) {
	if p.writeSourceMap {
		start := len(p.js)
		for i, c := range text {
			if c == '\n' {
				p.prevLineStart = start + i + 1
				p.prevState.GeneratedLine++
				p.prevState.GeneratedColumn = 0
				p.sourceMap = append(p.sourceMap, ';')
			}
		}
	}

	p.js = append(p.js, text...)
}

func (p *printer) addSourceMapping(loc ast.Loc) {
	if !p.writeSourceMap || loc == p.prevLoc {
		return
	}
	p.prevLoc = loc

	// Binary search to find the line
	lineStarts := p.lineStarts
	count := len(lineStarts)
	originalLine := 0
	for count > 0 {
		step := count / 2
		i := originalLine + step
		if lineStarts[i] <= loc.Start {
			originalLine = i + 1
			count = count - step - 1
		} else {
			count = step
		}
	}

	// Use the line to compute the column
	originalColumn := int(loc.Start)
	if originalLine > 0 {
		originalColumn -= int(lineStarts[originalLine-1])
	}

	generatedColumn := len(p.js) - p.prevLineStart

	currentState := SourceMapState{
		GeneratedLine:   p.prevState.GeneratedLine,
		GeneratedColumn: generatedColumn,
		SourceIndex:     0, // Pretend the source index is 0, and later substitute the right one in AppendSourceMapChunk()
		OriginalLine:    originalLine,
		OriginalColumn:  originalColumn,
	}

	p.sourceMap = appendMapping(p.sourceMap, p.prevState, currentState)
	p.prevState = currentState
}

func (p *printer) printIndent() {
	if !p.minify {
		for i := 0; i < p.indent; i++ {
			p.print("  ")
		}
	}
}

func (p *printer) symbolName(ref ast.Ref) string {
	ref = ast.FollowSymbols(p.symbols, ref)
	return p.symbols.Get(ref).Name
}

func (p *printer) printSymbol(ref ast.Ref) {
	ref = ast.FollowSymbols(p.symbols, ref)
	symbol := p.symbols.Get(ref)
	p.printSpaceBeforeIdentifier()
	p.print(symbol.Name)
}

func (p *printer) printBinding(binding ast.Binding) {
	p.addSourceMapping(binding.Loc)

	switch b := binding.Data.(type) {
	case *ast.BMissing:

	case *ast.BIdentifier:
		p.printSymbol(b.Ref)

	case *ast.BArray:
		p.print("[")
		for i, item := range b.Items {
			if i != 0 {
				p.print(",")
				p.printSpace()
			}
			if b.HasSpread && i+1 == len(b.Items) {
				p.print("...")
			}
			p.printBinding(item.Binding)

			if item.DefaultValue != nil {
				p.printSpace()
				p.print("=")
				p.printSpace()
				p.printExpr(*item.DefaultValue, ast.LComma, 0)
			}

			// Make sure there's a comma after trailing missing items
			if _, ok := item.Binding.Data.(*ast.BMissing); ok && i == len(b.Items)-1 {
				p.print(",")
			}
		}
		p.print("]")

	case *ast.BObject:
		p.print("{")
		for i, item := range b.Properties {
			if i != 0 {
				p.print(",")
				p.printSpace()
			}

			if item.IsSpread {
				p.print("...")
			} else {
				if item.IsComputed {
					p.print("[")
					p.printExpr(item.Key, ast.LLowest, 0)
					p.print("]:")
					p.printSpace()
					p.printBinding(item.Value)

					if item.DefaultValue != nil {
						p.printSpace()
						p.print("=")
						p.printSpace()
						p.printExpr(*item.DefaultValue, ast.LComma, 0)
					}
					continue
				}

				if str, ok := item.Key.Data.(*ast.EString); ok {
					text := lexer.UTF16ToString(str.Value)
					if lexer.IsIdentifier(text) {
						p.printSpaceBeforeIdentifier()
						p.print(text)

						// Use a shorthand property if the names are the same
						if id, ok := item.Value.Data.(*ast.BIdentifier); ok && text == p.symbolName(id.Ref) {
							if item.DefaultValue != nil {
								p.printSpace()
								p.print("=")
								p.printSpace()
								p.printExpr(*item.DefaultValue, ast.LComma, 0)
							}
							continue
						}
					} else {
						p.printExpr(item.Key, ast.LLowest, 0)
					}
				} else {
					p.printExpr(item.Key, ast.LLowest, 0)
				}

				p.print(":")
				p.printSpace()
			}
			p.printBinding(item.Value)

			if item.DefaultValue != nil {
				p.printSpace()
				p.print("=")
				p.printSpace()
				p.printExpr(*item.DefaultValue, ast.LComma, 0)
			}
		}

		p.print("}")

	default:
		panic(fmt.Sprintf("Unexpected binding of type %T", binding.Data))
	}
}

func (p *printer) printSpace() {
	if !p.minify {
		p.print(" ")
	}
}

func (p *printer) printNewline() {
	if !p.minify {
		p.print("\n")
	}
}

func (p *printer) printSpaceBeforeOperator(next ast.OpCode) {
	if p.prevOpEnd == len(p.js) {
		prev := p.prevOp

		// "+ + y" => "+ +y"
		// "+ ++ y" => "+ ++y"
		// "x + + y" => "x+ +y"
		// "x ++ + y" => "x+++y"
		// "x + ++ y" => "x+ ++y"
		// "-- >" => "-- >"
		// "< ! --" => "<! --"
		if ((prev == ast.BinOpAdd || prev == ast.UnOpPos) && (next == ast.BinOpAdd || next == ast.UnOpPos || next == ast.UnOpPreInc)) ||
			((prev == ast.BinOpSub || prev == ast.UnOpNeg) && (next == ast.BinOpSub || next == ast.UnOpNeg || next == ast.UnOpPreDec)) ||
			(prev == ast.UnOpPostDec && next == ast.BinOpGt) ||
			(prev == ast.UnOpNot && next == ast.UnOpPreDec && len(p.js) > 1 && p.js[len(p.js)-2] == '<') {
			p.print(" ")
		}
	}
}

func (p *printer) printSemicolonAfterStatement() {
	if !p.minify {
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
	if n > 0 && (lexer.IsIdentifierContinue(rune(buffer[n-1])) || n == p.prevRegExpEnd) {
		p.print(" ")
	}
}

func (p *printer) printFnArgs(args []ast.Arg, hasRestArg bool, isArrow bool) {
	wrap := true

	// Minify "(a) => {}" as "a=>{}"
	if p.minify && !hasRestArg && isArrow && len(args) == 1 {
		if _, ok := args[0].Binding.Data.(*ast.BIdentifier); ok && args[0].Default == nil {
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

		if arg.Default != nil {
			p.printSpace()
			p.print("=")
			p.printSpace()
			p.printExpr(*arg.Default, ast.LComma, 0)
		}
	}

	if wrap {
		p.print(")")
	}
}

func (p *printer) printFn(fn ast.Fn) {
	p.printFnArgs(fn.Args, fn.HasRestArg, false)
	p.printSpace()
	p.printBlock(fn.Stmts)
}

func (p *printer) printClass(class ast.Class) {
	if class.Extends != nil {
		p.print(" extends")
		p.printSpace()
		p.printExpr(*class.Extends, ast.LNew-1, 0)
	}
	p.printSpace()

	p.print("{")
	p.printNewline()
	p.indent++

	for _, item := range class.Properties {
		p.printSemicolonIfNeeded()
		p.printIndent()
		p.printProperty(item)

		// Need semicolons after class fields
		if item.Value == nil {
			p.printSemicolonAfterStatement()
		} else {
			p.printNewline()
		}
	}

	p.needsSemicolon = false
	p.indent--
	p.printIndent()
	p.print("}")
}

func (p *printer) printProperty(item ast.Property) {
	if item.Kind == ast.PropertySpread {
		p.print("...")
		p.printExpr(*item.Value, ast.LComma, 0)
		return
	}

	if item.IsStatic {
		p.print("static")
		p.printSpace()
	}

	switch item.Kind {
	case ast.PropertyGet:
		p.printSpaceBeforeIdentifier()
		p.print("get")
		p.printSpace()

	case ast.PropertySet:
		p.printSpaceBeforeIdentifier()
		p.print("set")
		p.printSpace()
	}

	if item.Value != nil {
		if fn, ok := item.Value.Data.(*ast.EFunction); item.IsMethod && ok {
			if fn.Fn.IsAsync {
				p.printSpaceBeforeIdentifier()
				p.print("async")
				p.printSpace()
			}
			if fn.Fn.IsGenerator {
				p.print("*")
			}
		}
	}

	if item.IsComputed {
		p.print("[")
		p.printExpr(item.Key, ast.LLowest, 0)
		p.print("]")

		if item.Value != nil {
			if fn, ok := item.Value.Data.(*ast.EFunction); item.IsMethod && ok {
				p.printFn(fn.Fn)
				return
			}

			p.print(":")
			p.printSpace()
			p.printExpr(*item.Value, ast.LComma, 0)
		}

		if item.Initializer != nil {
			p.printSpace()
			p.print("=")
			p.printSpace()
			p.printExpr(*item.Initializer, ast.LComma, 0)
		}
		return
	}

	if str, ok := item.Key.Data.(*ast.EString); ok {
		text := lexer.UTF16ToString(str.Value)
		if lexer.IsIdentifier(text) {
			p.printSpaceBeforeIdentifier()
			p.print(text)

			// Use a shorthand property if the names are the same
			if item.Value != nil {
				switch e := item.Value.Data.(type) {
				case *ast.EIdentifier:
					if text == p.symbolName(e.Ref) {
						if item.Initializer != nil {
							p.printSpace()
							p.print("=")
							p.printSpace()
							p.printExpr(*item.Initializer, ast.LComma, 0)
						}
						return
					}

				case *ast.ENamespaceImport:
					// Make sure we're not using a property access instead of an identifier
					if !p.indirectImportItems[e.ItemRef] && text == p.symbolName(e.ItemRef) {
						if item.Initializer != nil {
							p.printSpace()
							p.print("=")
							p.printSpace()
							p.printExpr(*item.Initializer, ast.LComma, 0)
						}
						return
					}
				}
			}
		} else {
			c := p.bestQuoteCharForString(str.Value, false /* allowBacktick */)
			p.print(c)
			p.print(quoteUTF16(str.Value, rune(c[0])))
			p.print(c)
		}
	} else {
		p.printExpr(item.Key, ast.LLowest, 0)
	}

	if item.Kind != ast.PropertyNormal {
		f, ok := item.Value.Data.(*ast.EFunction)
		if ok {
			p.printFn(f.Fn)
			return
		}
	}

	if item.Value != nil {
		if fn, ok := item.Value.Data.(*ast.EFunction); item.IsMethod && ok {
			p.printFn(fn.Fn)
			return
		}

		p.print(":")
		p.printSpace()
		p.printExpr(*item.Value, ast.LComma, 0)
	}

	if item.Initializer != nil {
		p.printSpace()
		p.print("=")
		p.printSpace()
		p.printExpr(*item.Initializer, ast.LComma, 0)
	}
}

func (p *printer) bestQuoteCharForString(data []uint16, allowBacktick bool) string {
	singleCost := 0
	doubleCost := 0
	backtickCost := 0

	for _, c := range data {
		switch c {
		case '\n':
			if p.minify {
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
	return c
}

const (
	forbidCall = 1 << iota
	forbidIn
)

func (p *printer) printExpr(expr ast.Expr, level ast.L, flags int) {
	p.addSourceMapping(expr.Loc)

	switch e := expr.Data.(type) {
	case *ast.EMissing:

	case *ast.EUndefined:
		if level >= ast.LPrefix {
			p.print("(void 0)")
		} else {
			p.printSpaceBeforeIdentifier()
			p.print("void 0")
			p.prevNumEnd = len(p.js)
		}

	case *ast.ESuper:
		p.printSpaceBeforeIdentifier()
		p.print("super")

	case *ast.ENull:
		p.printSpaceBeforeIdentifier()
		p.print("null")

	case *ast.EThis:
		p.printSpaceBeforeIdentifier()
		p.print("this")

	case *ast.ESpread:
		p.print("...")
		p.printExpr(e.Value, ast.LLowest, 0)

	case *ast.ENewTarget:
		p.printSpaceBeforeIdentifier()
		p.print("new.target")

	case *ast.EImportMeta:
		p.printSpaceBeforeIdentifier()
		p.print("import.meta")

	case *ast.ENew:
		wrap := level >= ast.LCall
		if wrap {
			p.print("(")
		}
		p.printSpaceBeforeIdentifier()
		p.print("new")
		p.printSpace()
		p.printExpr(e.Target, ast.LNew, forbidCall)

		// TODO: Omit this while minifying
		p.print("(")
		for i, arg := range e.Args {
			if i != 0 {
				p.print(",")
				p.printSpace()
			}
			p.printExpr(arg, ast.LComma, 0)
		}
		p.print(")")

		if wrap {
			p.print(")")
		}

	case *ast.ECall:
		wrap := level >= ast.LNew || (flags&forbidCall) != 0
		if wrap {
			p.print("(")
		}
		p.printExpr(e.Target, ast.LPostfix, 0)
		if e.IsOptionalChain {
			p.print("?.")
		}
		p.print("(")
		for i, arg := range e.Args {
			if i != 0 {
				p.print(",")
				p.printSpace()
			}
			p.printExpr(arg, ast.LComma, 0)
		}
		p.print(")")
		if wrap {
			p.print(")")
		}

	case *ast.ERequire:
		wrap := level >= ast.LNew
		if wrap {
			p.print("(")
		}
		p.printSymbol(p.requireRef)
		p.print("(")
		if p.resolvedImports != nil {
			// If we're bundling require calls, convert the string to a source index
			sourceIndex, ok := p.resolvedImports[e.Path.Text]
			if !ok {
				panic("Internal error")
			}
			p.print(fmt.Sprintf("%d", sourceIndex))
			if !p.minify {
				p.print(fmt.Sprintf(" /* %s */", e.Path.Text))
			}
		} else {
			p.print(Quote(e.Path.Text))
		}

		// If this require() call used to be an ES6 import, let the bootstrap code
		// know so it can convert the default export format from CommonJS to ES6
		if e.IsES6Import {
			p.print(",")
			p.printSpace()
			if p.minify {
				p.print("1")
			} else {
				p.print("true /* ES6 import */")
			}
		}

		p.print(")")
		if wrap {
			p.print(")")
		}

	case *ast.EImport:
		wrap := level >= ast.LNew || (flags&forbidCall) != 0
		if wrap {
			p.print("(")
		}
		p.printSpaceBeforeIdentifier()
		if s, ok := e.Expr.Data.(*ast.EString); ok && p.resolvedImports != nil {
			// If we're bundling require calls, convert the string to a source index
			path := lexer.UTF16ToString(s.Value)
			sourceIndex, ok := p.resolvedImports[path]
			if !ok {
				panic("Internal error")
			}
			if p.minify {
				p.print("Promise.resolve().then(()=>")
				p.printSymbol(p.requireRef)
				p.print(fmt.Sprintf("(%d))", sourceIndex))
			} else {
				p.print("Promise.resolve().then(() => ")
				p.printSymbol(p.requireRef)
				p.print(fmt.Sprintf("(%d /* %s */))", sourceIndex, path))
			}
		} else {
			p.print("import(")
			p.printExpr(e.Expr, ast.LComma, 0)
			p.print(")")
		}
		if wrap {
			p.print(")")
		}

	case *ast.EDot:
		p.printExpr(e.Target, ast.LPostfix, flags)
		if e.IsOptionalChain {
			p.print("?")
		} else if p.prevNumEnd == len(p.js) {
			// "1.toString" is a syntax error, so print "1 .toString" instead
			p.print(" ")
		}
		p.print(".")
		p.addSourceMapping(e.NameLoc)
		p.print(e.Name)

	case *ast.EIndex:
		p.printExpr(e.Target, ast.LPostfix, flags)
		if e.IsOptionalChain {
			p.print("?.")
		}
		p.print("[")
		p.printExpr(e.Index, ast.LLowest, 0)
		p.print("]")

	case *ast.EIf:
		wrap := level >= ast.LConditional
		if wrap {
			p.print("(")
			flags &= ^forbidIn
		}
		p.printExpr(e.Test, ast.LConditional, flags&forbidIn)
		p.printSpace()
		p.print("?")
		p.printSpace()
		p.printExpr(e.Yes, ast.LYield, 0)
		p.printSpace()
		p.print(":")
		p.printSpace()
		p.printExpr(e.No, ast.LYield, flags&forbidIn)
		if wrap {
			p.print(")")
		}

	case *ast.EArrow:
		wrap := level > ast.LAssign
		if wrap {
			p.print("(")
		}
		if e.IsAsync {
			p.printSpaceBeforeIdentifier()
			p.print("async")
			p.printSpace()
		}
		p.printFnArgs(e.Args, e.HasRestArg, true)
		p.printSpace()
		p.print("=>")
		p.printSpace()
		if e.Expr != nil {
			p.arrowExprStart = len(p.js)
			p.printExpr(*e.Expr, ast.LComma, 0)
		} else {
			p.printBlock(e.Stmts)
		}
		if wrap {
			p.print(")")
		}

	case *ast.EFunction:
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

	case *ast.EClass:
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

	case *ast.EArray:
		p.print("[")
		for i, item := range e.Items {
			if i != 0 {
				p.print(",")
				p.printSpace()
			}
			p.printExpr(item, ast.LComma, 0)

			// Make sure there's a comma after trailing missing items
			_, ok := item.Data.(*ast.EMissing)
			if ok && i == len(e.Items)-1 {
				p.print(",")
			}
		}
		p.print("]")

	case *ast.EObject:
		n := len(p.js)
		wrap := p.stmtStart == n || p.arrowExprStart == n
		if wrap {
			p.print("(")
		}
		p.print("{")
		if len(e.Properties) != 0 {
			p.indent++

			for i, item := range e.Properties {
				if i != 0 {
					p.print(",")
				}

				p.printNewline()
				p.printIndent()
				p.printProperty(item)
			}

			p.indent--
			p.printNewline()
			p.printIndent()
		}

		p.print("}")
		if wrap {
			p.print(")")
		}

	case *ast.EBoolean:
		if p.minify {
			if level >= ast.LPrefix {
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

	case *ast.EString:
		c := p.bestQuoteCharForString(e.Value, true /* allowBacktick */)
		p.print(c)
		p.print(quoteUTF16(e.Value, rune(c[0])))
		p.print(c)

	case *ast.ETemplate:
		if e.Tag != nil {
			p.printExpr(*e.Tag, ast.LPostfix, 0)
		}
		p.print("`")
		if e.Tag != nil {
			p.print(e.HeadRaw)
		} else {
			p.print(quoteUTF16(e.Head, '`'))
		}
		for _, part := range e.Parts {
			p.print("${")
			p.printExpr(part.Value, ast.LLowest, 0)
			p.print("}")
			if e.Tag != nil {
				p.print(part.TailRaw)
			} else {
				p.print(quoteUTF16(part.Tail, '`'))
			}
		}
		p.print("`")

	case *ast.ERegExp:
		buffer := p.js
		n := len(buffer)

		// Avoid forming a single-line comment
		if n > 0 && buffer[n-1] == '/' {
			p.print(" ")
		}
		p.print(e.Value)

		// Need a space before the next identifier to avoid it turning into flags
		p.prevRegExpEnd = len(p.js)

	case *ast.EBigInt:
		p.printSpaceBeforeIdentifier()
		p.print(e.Value)
		p.print("n")

	case *ast.ENumber:
		value := e.Value
		asUint32 := uint32(value)

		// Expressions such as "(-1).toString" need to wrap negative numbers.
		// Instead of testing for "value < 0" we test for "signbit(value)" and
		// "!isNaN(value)" because we need this to be true for "-0" and "-0 < 0"
		// is false.
		wrap := math.Signbit(value) && value == value && level >= ast.LPrefix
		if wrap {
			p.print("(")
		}

		// Go will print "4294967295" as "4.294967295e+09" if we use the float64
		// printer, so explicitly print integers using a separate code path. This
		// should also serve as a fast path for the common case, so do this first.
		if value == float64(asUint32) {
			text := strconv.FormatInt(int64(asUint32), 10)

			// Make sure to preserve negative zero so constant-folding doesn't change semantics
			if value == 0 && math.Signbit(value) && text[0] != '-' {
				p.printSpaceBeforeOperator(ast.UnOpNeg)
				p.print("-")
			}

			p.printSpaceBeforeIdentifier()
			p.print(text)

			// Remember the end of the latest number
			p.prevNumEnd = len(p.js)
		} else if value != value {
			p.printSpaceBeforeIdentifier()
			p.print("NaN")
		} else if value == positiveInfinity {
			p.printSpaceBeforeIdentifier()
			p.print("Infinity")
		} else if value == negativeInfinity {
			p.printSpaceBeforeOperator(ast.UnOpNeg)
			p.print("-Infinity")
		} else {
			text := fmt.Sprintf("%v", value)

			// Strip off the leading zero when minifying
			if p.minify && strings.HasPrefix(text, "0.") {
				text = text[1:]
			}

			if text[0] == '-' {
				p.printSpaceBeforeOperator(ast.UnOpNeg)
			}
			p.printSpaceBeforeIdentifier()
			p.print(text)

			// Remember the end of the latest number
			p.prevNumEnd = len(p.js)
		}

		if wrap {
			p.print(")")
		}

	case *ast.EIdentifier:
		p.printSpaceBeforeIdentifier()
		p.printSymbol(e.Ref)

	case *ast.ENamespaceImport:
		// Potentially use a property access instead of an identifier
		if p.indirectImportItems[e.ItemRef] {
			p.printSymbol(e.NamespaceRef)
			p.print(".")
			p.print(e.Alias)
		} else {
			p.printSymbol(e.ItemRef)
		}

	case *ast.EAwait:
		wrap := level >= ast.LPrefix

		if wrap {
			p.print("(")
		}

		p.printSpaceBeforeIdentifier()
		p.print("await")
		p.printSpace()
		p.printExpr(e.Value, ast.LPrefix, 0)

		if wrap {
			p.print(")")
		}

	case *ast.EYield:
		wrap := level >= ast.LAssign

		if wrap {
			p.print("(")
		}

		p.printSpaceBeforeIdentifier()
		p.print("yield")

		if e.Value != nil {
			if e.IsStar {
				p.print("*")
			}
			p.printSpace()
			p.printExpr(*e.Value, ast.LYield, 0)
		}

		if wrap {
			p.print(")")
		}

	case *ast.EUnary:
		entry := ast.OpTable[e.Op]
		wrap := level >= entry.Level

		if wrap {
			p.print("(")
		}

		if !e.Op.IsPrefix() {
			p.printExpr(e.Value, ast.LPostfix-1, 0)
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
			p.printExpr(e.Value, ast.LPrefix-1, 0)
		}

		if wrap {
			p.print(")")
		}

	case *ast.EBinary:
		entry := ast.OpTable[e.Op]
		wrap := level >= entry.Level || (e.Op == ast.BinOpIn && (flags&forbidIn) != 0)

		// Destructuring assignments must be parenthesized
		if p.stmtStart == len(p.js) {
			if _, ok := e.Left.Data.(*ast.EObject); ok {
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
		case ast.BinOpNullishCoalescing:
			// "??" can't directly contain "||" or "&&" without being wrapped in parentheses
			if left, ok := e.Left.Data.(*ast.EBinary); ok && (left.Op == ast.BinOpLogicalOr || left.Op == ast.BinOpLogicalAnd) {
				leftLevel = ast.LPrefix
			}
			if right, ok := e.Right.Data.(*ast.EBinary); ok && (right.Op == ast.BinOpLogicalOr || right.Op == ast.BinOpLogicalAnd) {
				rightLevel = ast.LPrefix
			}

		case ast.BinOpPow:
			// "**" can't contain certain unary expressions
			if left, ok := e.Left.Data.(*ast.EUnary); ok && !left.Op.IsUnaryUpdate() {
				leftLevel = ast.LCall
			} else if _, ok := e.Left.Data.(*ast.EUndefined); ok {
				// Undefined is printed as "void 0"
				leftLevel = ast.LCall
			} else if p.minify {
				// When minifying, booleans are printed as "!0 and "!1"
				if _, ok := e.Left.Data.(*ast.EBoolean); ok {
					leftLevel = ast.LCall
				}
			}
		}

		p.printExpr(e.Left, leftLevel, flags&forbidIn)

		if e.Op != ast.BinOpComma {
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

		p.printExpr(e.Right, rightLevel, flags&forbidIn)

		if wrap {
			p.print(")")
		}

	default:
		panic(fmt.Sprintf("Unexpected expression of type %T", expr.Data))
	}
}

func (p *printer) printDeclStmt(isExport bool, keyword string, decls []ast.Decl) {
	p.printIndent()
	p.printSpaceBeforeIdentifier()
	if isExport {
		p.print("export ")
	}
	p.printDecls(keyword, decls, 0)
	p.printSemicolonAfterStatement()
}

func (p *printer) printForLoopInit(init ast.Stmt) {
	switch s := init.Data.(type) {
	case *ast.SExpr:
		p.printExpr(s.Value, ast.LLowest, forbidIn)
	case *ast.SLocal:
		switch s.Kind {
		case ast.LocalVar:
			p.printDecls("var", s.Decls, forbidIn)
		case ast.LocalLet:
			p.printDecls("let", s.Decls, forbidIn)
		case ast.LocalConst:
			p.printDecls("const", s.Decls, forbidIn)
		}
	default:
		panic("Internal error")
	}
}

func (p *printer) printDecls(keyword string, decls []ast.Decl, flags int) {
	p.print(keyword)
	p.printSpace()

	for i, decl := range decls {
		if i != 0 {
			p.print(",")
			p.printSpace()
		}
		p.printBinding(decl.Binding)

		if decl.Value != nil {
			p.printSpace()
			p.print("=")
			p.printSpace()
			p.printExpr(*decl.Value, ast.LComma, flags)
		}
	}
}

func (p *printer) printBody(body ast.Stmt) {
	if block, ok := body.Data.(*ast.SBlock); ok {
		p.printSpace()
		p.printBlock(block.Stmts)
		p.printNewline()
	} else {
		p.printNewline()
		p.indent++
		p.printStmt(body)
		p.indent--
	}
}

func (p *printer) printBlock(stmts []ast.Stmt) {
	p.print("{")
	p.printNewline()

	p.indent++
	for _, stmt := range stmts {
		p.printSemicolonIfNeeded()
		p.printStmt(stmt)
	}
	p.indent--
	p.needsSemicolon = false

	p.printIndent()
	p.print("}")
}

func wrapToAvoidAmbiguousElse(s ast.S) bool {
	for {
		switch current := s.(type) {
		case *ast.SIf:
			if current.No == nil {
				return true
			}
			s = current.No.Data

		case *ast.SFor:
			s = current.Body.Data

		case *ast.SForIn:
			s = current.Body.Data

		case *ast.SForOf:
			s = current.Body.Data

		case *ast.SWhile:
			s = current.Body.Data

		case *ast.SWith:
			s = current.Body.Data

		default:
			return false
		}
	}
}

func (p *printer) printIf(s *ast.SIf) {
	p.printSpaceBeforeIdentifier()
	p.print("if")
	p.printSpace()
	p.print("(")
	p.printExpr(s.Test, ast.LLowest, 0)
	p.print(")")

	if yes, ok := s.Yes.Data.(*ast.SBlock); ok {
		p.printSpace()
		p.printBlock(yes.Stmts)

		if s.No != nil {
			p.printSpace()
		} else {
			p.printNewline()
		}
	} else if wrapToAvoidAmbiguousElse(s.Yes.Data) {
		p.printSpace()
		p.print("{")
		p.printNewline()

		p.indent++
		p.printStmt(s.Yes)
		p.indent--
		p.needsSemicolon = false

		p.printIndent()
		p.print("}")

		if s.No != nil {
			p.printSpace()
		} else {
			p.printNewline()
		}
	} else {
		p.printNewline()
		p.indent++
		p.printStmt(s.Yes)
		p.indent--

		if s.No != nil {
			p.printIndent()
		}
	}

	if s.No != nil {
		p.printSemicolonIfNeeded()
		p.printSpaceBeforeIdentifier()
		p.print("else")

		if no, ok := s.No.Data.(*ast.SBlock); ok {
			p.printSpace()
			p.printBlock(no.Stmts)
			p.printNewline()
		} else if no, ok := s.No.Data.(*ast.SIf); ok {
			p.printIf(no)
		} else {
			p.printNewline()
			p.indent++
			p.printStmt(*s.No)
			p.indent--
		}
	}
}

func (p *printer) printStmt(stmt ast.Stmt) {
	p.addSourceMapping(stmt.Loc)

	switch s := stmt.Data.(type) {
	case *ast.SFunction:
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

	case *ast.SClass:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		if s.IsExport {
			p.print("export ")
		}
		p.print("class")
		p.printSymbol(s.Class.Name.Ref)
		p.printClass(s.Class)
		p.printNewline()

	case *ast.SEmpty:
		p.printIndent()
		p.print(";")
		p.printNewline()

	case *ast.SExportDefault:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("export default")
		p.printSpace()

		if s.Value.Expr != nil {
			// Functions and classes must be wrapped to avoid confusion with their statement forms
			p.exportDefaultStart = len(p.js)

			p.printExpr(*s.Value.Expr, ast.LComma, 0)
			p.printSemicolonAfterStatement()
			return
		}

		switch s2 := s.Value.Stmt.Data.(type) {
		case *ast.SFunction:
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

		case *ast.SClass:
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

	case *ast.SExportStar:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("export")
		p.printSpace()
		p.print("*")
		p.printSpace()
		if s.Item != nil {
			p.print("as")
			p.printSpace()
			p.printSpaceBeforeIdentifier()
			p.print(s.Item.Alias)
			p.printSpace()
			p.printSpaceBeforeIdentifier()
		}
		p.print("from")
		p.printSpace()
		p.print(Quote(s.Path.Text))
		p.printSemicolonAfterStatement()

	case *ast.SExportClause:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("export")
		p.printSpace()

		p.print("{")
		for i, item := range s.Items {
			if i != 0 {
				p.print(",")
				p.printSpace()
			}
			name := p.symbolName(item.Name.Ref)
			p.print(name)
			if name != item.Alias {
				p.print(" as ")
				p.print(item.Alias)
			}
		}
		p.print("}")

		p.printSemicolonAfterStatement()

	case *ast.SExportFrom:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("export")
		p.printSpace()

		p.print("{")
		for i, item := range s.Items {
			if i != 0 {
				p.print(",")
				p.printSpace()
			}
			name := p.symbolName(item.Name.Ref)
			p.print(name)
			if name != item.Alias {
				p.print(" as ")
				p.print(item.Alias)
			}
		}
		p.print("}")

		p.printSpace()
		p.print("from")
		p.printSpace()
		p.print(Quote(s.Path.Text))

		p.printSemicolonAfterStatement()

	case *ast.SLocal:
		switch s.Kind {
		case ast.LocalConst:
			p.printDeclStmt(s.IsExport, "const", s.Decls)
		case ast.LocalLet:
			p.printDeclStmt(s.IsExport, "let", s.Decls)
		case ast.LocalVar:
			p.printDeclStmt(s.IsExport, "var", s.Decls)
		}

	case *ast.SIf:
		p.printIndent()
		p.printIf(s)

	case *ast.SDoWhile:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("do")
		if block, ok := s.Body.Data.(*ast.SBlock); ok {
			p.printSpace()
			p.printBlock(block.Stmts)
			p.printSpace()
		} else {
			p.printNewline()
			p.indent++
			p.printStmt(s.Body)
			p.printSemicolonIfNeeded()
			p.indent--
			p.printIndent()
		}
		p.print("while")
		p.printSpace()
		p.print("(")
		p.printExpr(s.Test, ast.LLowest, 0)
		p.print(")")
		p.printSemicolonAfterStatement()

	case *ast.SForIn:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("for")
		p.printSpace()
		p.print("(")
		p.printForLoopInit(s.Init)
		p.printSpace()
		p.printSpaceBeforeIdentifier()
		p.print("in")
		p.printSpace()
		p.printExpr(s.Value, ast.LLowest, 0)
		p.print(")")
		p.printBody(s.Body)

	case *ast.SForOf:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("for")
		if s.IsAwait {
			p.print(" await")
		}
		p.printSpace()
		p.print("(")
		p.printForLoopInit(s.Init)
		p.printSpace()
		p.printSpaceBeforeIdentifier()
		p.print("of")
		p.printSpace()
		p.printExpr(s.Value, ast.LLowest, 0)
		p.print(")")
		p.printBody(s.Body)

	case *ast.SWhile:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("while")
		p.printSpace()
		p.print("(")
		p.printExpr(s.Test, ast.LLowest, 0)
		p.print(")")
		p.printBody(s.Body)

	case *ast.SWith:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("with")
		p.printSpace()
		p.print("(")
		p.printExpr(s.Value, ast.LLowest, 0)
		p.print(")")
		p.printBody(s.Body)

	case *ast.SLabel:
		p.printIndent()
		p.printSymbol(s.Name.Ref)
		p.print(":")
		p.printBody(s.Stmt)

	case *ast.STry:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("try")
		p.printSpace()
		p.printBlock(s.Body)

		if s.Catch != nil {
			p.printSpace()
			p.print("catch")
			if s.Catch.Binding != nil {
				p.printSpace()
				p.print("(")
				p.printBinding(*s.Catch.Binding)
				p.print(")")
			}
			p.printSpace()
			p.printBlock(s.Catch.Body)
		}

		if s.Finally != nil {
			p.printSpace()
			p.print("finally")
			p.printSpace()
			p.printBlock(s.Finally.Stmts)
		}

		p.printNewline()

	case *ast.SFor:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("for")
		p.printSpace()
		p.print("(")
		if s.Init != nil {
			p.printForLoopInit(*s.Init)
		}
		p.print(";")
		p.printSpace()
		if s.Test != nil {
			p.printExpr(*s.Test, ast.LLowest, 0)
		}
		p.print(";")
		p.printSpace()
		if s.Update != nil {
			p.printExpr(*s.Update, ast.LLowest, 0)
		}
		p.print(")")
		p.printBody(s.Body)

	case *ast.SSwitch:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("switch")
		p.printSpace()
		p.print("(")
		p.printExpr(s.Test, ast.LLowest, 0)
		p.print(")")
		p.printSpace()
		p.print("{")
		p.printNewline()
		p.indent++

		for _, c := range s.Cases {
			p.printSemicolonIfNeeded()
			p.printIndent()

			if c.Value != nil {
				p.print("case")
				p.printSpace()
				p.printExpr(*c.Value, ast.LLogicalAnd, 0)
			} else {
				p.print("default")
			}
			p.print(":")

			if len(c.Body) == 1 {
				if block, ok := c.Body[0].Data.(*ast.SBlock); ok {
					p.printSpace()
					p.printBlock(block.Stmts)
					p.printNewline()
					continue
				}
			}

			p.printNewline()
			p.indent++
			for _, stmt := range c.Body {
				p.printSemicolonIfNeeded()
				p.printStmt(stmt)
			}
			p.indent--
		}

		p.indent--
		p.printIndent()
		p.print("}")
		p.printNewline()
		p.needsSemicolon = false

	case *ast.SImport:
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
			for i, item := range *s.Items {
				if i != 0 {
					p.print(",")
					p.printSpace()
				}

				p.print(item.Alias)
				name := p.symbolName(item.Name.Ref)
				if name != item.Alias {
					p.print(" as ")
					p.print(name)
				}
			}
			p.print("}")
			itemCount++
		}

		if s.StarLoc != nil {
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
			p.print(" from")
			p.printSpace()
		}

		p.print(Quote(s.Path.Text))
		p.printSemicolonAfterStatement()

	case *ast.SBlock:
		p.printIndent()
		p.printBlock(s.Stmts)
		p.printNewline()

	case *ast.SDebugger:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("debugger")
		p.printSemicolonAfterStatement()

	case *ast.SDirective:
		c := p.bestQuoteCharForString(s.Value, false /* allowBacktick */)
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print(c)
		p.print(quoteUTF16(s.Value, rune(c[0])))
		p.print(c)
		p.printSemicolonAfterStatement()

	case *ast.SBreak:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("break")
		if s.Name != nil {
			p.print(" ")
			p.printSymbol(s.Name.Ref)
		}
		p.printSemicolonAfterStatement()

	case *ast.SContinue:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("continue")
		if s.Name != nil {
			p.print(" ")
			p.printSymbol(s.Name.Ref)
		}
		p.printSemicolonAfterStatement()

	case *ast.SReturn:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("return")
		if s.Value != nil {
			p.printSpace()
			p.printExpr(*s.Value, ast.LLowest, 0)
		}
		p.printSemicolonAfterStatement()

	case *ast.SThrow:
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print("throw")
		p.printSpace()
		p.printExpr(s.Value, ast.LLowest, 0)
		p.printSemicolonAfterStatement()

	case *ast.SExpr:
		p.printIndent()
		p.stmtStart = len(p.js)
		p.printExpr(s.Value, ast.LLowest, 0)
		p.printSemicolonAfterStatement()

	default:
		panic(fmt.Sprintf("Unexpected statement of type %T", stmt.Data))
	}
}

type Options struct {
	RemoveWhitespace  bool
	SourceMapContents *string
	Indent            int
	ResolvedImports   map[string]uint32
}

type SourceMapChunk struct {
	Buffer []byte

	// This end state will be used to rewrite the start of the following source
	// map chunk so that the delta-encoded VLQ numbers are preserved.
	EndState SourceMapState

	// There probably isn't a source mapping at the end of the file (nor should
	// there be) but if we're appending another source map chunk after this one,
	// we'll need to know how many characters were in the last line we generated.
	FinalGeneratedColumn int
}

func createPrinter(
	symbols *ast.SymbolMap,
	indirectImportItems map[ast.Ref]bool,
	requireRef ast.Ref,
	options Options,
) *printer {
	p := &printer{
		symbols:             symbols,
		indirectImportItems: indirectImportItems,
		writeSourceMap:      options.SourceMapContents != nil,
		minify:              options.RemoveWhitespace,
		resolvedImports:     options.ResolvedImports,
		indent:              options.Indent,
		prevOpEnd:           -1,
		prevLoc:             ast.Loc{-1},
		requireRef:          requireRef,
	}

	// If we're writing out a source map, prepare a table of line start indices
	// to do binary search on to figure out what line a given AST node came from
	if options.SourceMapContents != nil {
		lineStarts := []int32{}
		var prevCodePoint rune
		for i, codePoint := range *options.SourceMapContents {
			switch codePoint {
			case '\n':
				if prevCodePoint == '\r' {
					lineStarts[len(lineStarts)-1] = int32(i + 1)
				} else {
					lineStarts = append(lineStarts, int32(i+1))
				}
			case '\r', '\u2028', '\u2029':
				lineStarts = append(lineStarts, int32(i+1))
			}
			prevCodePoint = codePoint
		}
		p.lineStarts = lineStarts
	}

	return p
}

func Print(tree ast.AST, options Options) ([]byte, SourceMapChunk) {
	p := createPrinter(tree.Symbols, tree.IndirectImportItems, tree.RequireRef, options)
	p.requireRef = tree.RequireRef

	// Always add a mapping at the beginning of the file
	p.addSourceMapping(ast.Loc{0})

	// Preserve the hashbang comment if present
	if tree.Hashbang != "" {
		p.print(tree.Hashbang + "\n")
	}

	for _, stmt := range tree.Stmts {
		p.printSemicolonIfNeeded()
		p.printStmt(stmt)
	}

	// Make sure each module ends in a semicolon so we don't have weird issues
	// with automatic semicolon insertion when concatenating modules together
	if options.RemoveWhitespace && len(p.js) > 0 && p.js[len(p.js)-1] != '\n' {
		p.printSemicolonIfNeeded()
		p.print("\n")
	}

	return p.js, SourceMapChunk{p.sourceMap, p.prevState, len(p.js) - p.prevLineStart}
}

func PrintExpr(expr ast.Expr, symbols *ast.SymbolMap, requireRef ast.Ref, options Options) ([]byte, SourceMapChunk) {
	p := createPrinter(symbols, make(map[ast.Ref]bool), requireRef, options)

	// Always add a mapping at the beginning of the file
	p.addSourceMapping(ast.Loc{0})

	p.printExpr(expr, ast.LLowest, 0)
	return p.js, SourceMapChunk{p.sourceMap, p.prevState, len(p.js) - p.prevLineStart}
}
