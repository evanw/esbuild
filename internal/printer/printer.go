package printer

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
	"github.com/evanw/esbuild/internal/lexer"
	"github.com/evanw/esbuild/internal/logging"
	"github.com/evanw/esbuild/internal/sourcemap"
)

var positiveInfinity = math.Inf(1)
var negativeInfinity = math.Inf(-1)

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
func AppendSourceMapChunk(j *Joiner, prevEndState SourceMapState, startState SourceMapState, sourceMap []byte) {
	// Handle line breaks in between this mapping and the previous one
	if startState.GeneratedLine != 0 {
		j.AddBytes(bytes.Repeat([]byte{';'}, startState.GeneratedLine))
		prevEndState.GeneratedColumn = 0
	}

	// Skip past any leading semicolons, which indicate line breaks
	semicolons := 0
	for sourceMap[semicolons] == ';' {
		semicolons++
	}
	if semicolons > 0 {
		j.AddBytes(sourceMap[:semicolons])
		sourceMap = sourceMap[semicolons:]
		prevEndState.GeneratedColumn = 0
		startState.GeneratedColumn = 0
	}

	// Strip off the first mapping from the buffer. The first mapping should be
	// for the start of the original file (the printer always generates one for
	// the start of the file).
	generatedColumn, i := sourcemap.DecodeVLQ(sourceMap, 0)
	sourceIndex, i := sourcemap.DecodeVLQ(sourceMap, i)
	originalLine, i := sourcemap.DecodeVLQ(sourceMap, i)
	originalColumn, i := sourcemap.DecodeVLQ(sourceMap, i)
	sourceMap = sourceMap[i:]

	// Rewrite the first mapping to be relative to the end state of the previous
	// chunk. We now know what the end state is because we're in the second pass
	// where all chunks have already been generated.
	startState.SourceIndex += sourceIndex
	startState.GeneratedColumn += generatedColumn
	startState.OriginalLine = originalLine
	startState.OriginalColumn = originalColumn
	j.AddBytes(appendMapping(nil, j.lastByte, prevEndState, startState))

	// Then append everything after that without modification.
	j.AddBytes(sourceMap)
}

func appendMapping(buffer []byte, lastByte byte, prevState SourceMapState, currentState SourceMapState) []byte {
	// Put commas in between mappings
	if lastByte != 0 && lastByte != ';' && lastByte != '"' {
		buffer = append(buffer, ',')
	}

	// Record the generated column (the line is recorded using ';' elsewhere)
	buffer = append(buffer, sourcemap.EncodeVLQ(currentState.GeneratedColumn-prevState.GeneratedColumn)...)
	prevState.GeneratedColumn = currentState.GeneratedColumn

	// Record the generated source
	buffer = append(buffer, sourcemap.EncodeVLQ(currentState.SourceIndex-prevState.SourceIndex)...)
	prevState.SourceIndex = currentState.SourceIndex

	// Record the original line
	buffer = append(buffer, sourcemap.EncodeVLQ(currentState.OriginalLine-prevState.OriginalLine)...)
	prevState.OriginalLine = currentState.OriginalLine

	// Record the original column
	buffer = append(buffer, sourcemap.EncodeVLQ(currentState.OriginalColumn-prevState.OriginalColumn)...)
	prevState.OriginalColumn = currentState.OriginalColumn

	return buffer
}

// This provides an efficient way to join lots of big string and byte slices
// together. It avoids the cost of repeatedly reallocating as the buffer grows
// by measuring exactly how big the buffer should be and then allocating once.
// This is a measurable speedup.
type Joiner struct {
	lastByte byte
	strings  []joinerString
	bytes    []joinerBytes
	length   uint32
}

type joinerString struct {
	data   string
	offset uint32
}

type joinerBytes struct {
	data   []byte
	offset uint32
}

func (j *Joiner) AddString(data string) {
	if len(data) > 0 {
		j.lastByte = data[len(data)-1]
	}
	j.strings = append(j.strings, joinerString{data, j.length})
	j.length += uint32(len(data))
}

func (j *Joiner) AddBytes(data []byte) {
	if len(data) > 0 {
		j.lastByte = data[len(data)-1]
	}
	j.bytes = append(j.bytes, joinerBytes{data, j.length})
	j.length += uint32(len(data))
}

func (j *Joiner) LastByte() byte {
	return j.lastByte
}

func (j *Joiner) Length() uint32 {
	return j.length
}

func (j *Joiner) Done() []byte {
	buffer := make([]byte, j.length)
	for _, item := range j.strings {
		copy(buffer[item.offset:], item.data)
	}
	for _, item := range j.bytes {
		copy(buffer[item.offset:], item.data)
	}
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
			r, width := lexer.DecodeWTF8Rune(text[i:])
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

// This is the same as "print(quoteUTF16(text))" without any unnecessary
// temporary allocations
func (p *printer) printQuotedUTF16(text []uint16, quote rune) {
	temp := make([]byte, utf8.UTFMax)
	js := p.js
	i := 0
	n := len(text)

	for i < n {
		c := text[i]
		i++

		switch c {
		case '\x00':
			// We don't want "\x001" to be written as "\01"
			if i >= n || text[i] < '0' || text[i] > '9' {
				js = append(js, "\\0"...)
			} else {
				js = append(js, "\\x00"...)
			}

		case '\b':
			js = append(js, "\\b"...)

		case '\f':
			js = append(js, "\\f"...)

		case '\n':
			if quote == '`' {
				js = append(js, '\n')

				// Make sure to do with print() does for newlines
				if p.options.SourceForSourceMap != nil {
					p.appendNewlineToSourceMap(len(js))
				}
			} else {
				js = append(js, "\\n"...)
			}

		case '\r':
			js = append(js, "\\r"...)

		case '\v':
			js = append(js, "\\v"...)

		case '\\':
			js = append(js, "\\\\"...)

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
						width := utf8.EncodeRune(temp, (rune(c)<<10)+rune(c2)+(0x10000-(0xD800<<10)-0xDC00))
						js = append(js, temp[:width]...)
						continue
					}
				}

				// Write an escaped character
				js = append(js, '\\', 'u', hexChars[c>>12], hexChars[(c>>8)&15], hexChars[(c>>4)&15], hexChars[c&15])

			// Is this a low surrogate?
			case c >= 0xDC00 && c <= 0xDFFF:
				// Write an escaped character
				js = append(js, '\\', 'u', hexChars[c>>12], hexChars[(c>>8)&15], hexChars[(c>>4)&15], hexChars[c&15])

			default:
				if c < utf8.RuneSelf {
					js = append(js, byte(c))
				} else {
					width := utf8.EncodeRune(temp, rune(c))
					js = append(js, temp[:width]...)
				}
			}
		}
	}

	p.js = js
}

type printer struct {
	symbols            ast.SymbolMap
	importRecords      []ast.ImportRecord
	options            PrintOptions
	extractedComments  map[string]bool
	needsSemicolon     bool
	js                 []byte
	stmtStart          int
	exportDefaultStart int
	arrowExprStart     int
	prevOp             ast.OpCode
	prevOpEnd          int
	prevNumEnd         int
	prevRegExpEnd      int
	intToBytesBuffer   [64]byte

	// For source maps
	sourceMap     []byte
	prevLoc       ast.Loc
	prevLineStart int
	prevState     SourceMapState
	hasPrevState  bool
	lineStarts    []int32

	// This is a workaround for a bug in the popular "source-map" library:
	// https://github.com/mozilla/source-map/issues/261. The library will
	// sometimes return null when querying a source map unless every line
	// starts with a mapping at column zero.
	//
	// The workaround is to replicate the previous mapping if a line ends
	// up not starting with a mapping. This is done lazily because we want
	// to avoid replicating the previous mapping if we don't need to.
	lineStartsWithMapping bool
}

func (p *printer) print(text string) {
	if p.options.SourceForSourceMap != nil {
		start := len(p.js)
		for i, c := range text {
			if c == '\n' {
				p.appendNewlineToSourceMap(start + i + 1)
			}
		}
	}

	p.js = append(p.js, text...)
}

// This is the same as "print(string(bytes))" without any unnecessary temporary
// allocations
func (p *printer) printBytes(bytes []byte) {
	if p.options.SourceForSourceMap != nil {
		start := len(p.js)
		for i, c := range bytes {
			if c == '\n' {
				p.appendNewlineToSourceMap(start + i + 1)
			}
		}
	}

	p.js = append(p.js, bytes...)
}

// This is the same as "print(lexer.UTF16ToString(text))" without any
// unnecessary temporary allocations
func (p *printer) printUTF16(text []uint16) {
	if p.options.SourceForSourceMap != nil {
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

	p.js = lexer.AppendUTF16ToBytes(p.js, text)
}

func (p *printer) addSourceMapping(loc ast.Loc) {
	if p.options.SourceForSourceMap == nil || loc == p.prevLoc {
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

	// If this line doesn't start with a mapping and we're about to add a mapping
	// that's not at the start, insert a mapping first so the line starts with one.
	if !p.lineStartsWithMapping && generatedColumn > 0 && p.hasPrevState {
		p.appendMappingWithoutRemapping(SourceMapState{
			GeneratedLine:   p.prevState.GeneratedLine,
			GeneratedColumn: 0,
			SourceIndex:     p.prevState.SourceIndex,
			OriginalLine:    p.prevState.OriginalLine,
			OriginalColumn:  p.prevState.OriginalColumn,
		})
	}

	p.appendMapping(SourceMapState{
		GeneratedLine:   p.prevState.GeneratedLine,
		GeneratedColumn: generatedColumn,
		OriginalLine:    originalLine,
		OriginalColumn:  originalColumn,
	})

	// This line now has a mapping on it, so don't insert another one
	p.lineStartsWithMapping = true
}

func (p *printer) appendMapping(currentState SourceMapState) {
	// If the input file had a source map, map all the way back to the original
	if p.options.InputSourceMap != nil {
		mapping := p.options.InputSourceMap.Find(
			int32(currentState.OriginalLine),
			int32(currentState.OriginalColumn))

		// Some locations won't have a mapping
		if mapping == nil {
			return
		}

		currentState.SourceIndex = int(mapping.SourceIndex)
		currentState.OriginalLine = int(mapping.OriginalLine)
		currentState.OriginalColumn = int(mapping.OriginalColumn)
	}

	p.appendMappingWithoutRemapping(currentState)
}

func (p *printer) appendMappingWithoutRemapping(currentState SourceMapState) {
	var lastByte byte
	if len(p.sourceMap) != 0 {
		lastByte = p.sourceMap[len(p.sourceMap)-1]
	}

	p.sourceMap = appendMapping(p.sourceMap, lastByte, p.prevState, currentState)
	p.prevState = currentState
	p.hasPrevState = true
}

// Don't call this directly. This is called automatically by p.print("\n").
func (p *printer) appendNewlineToSourceMap(prevLineStart int) {
	// If we're about to move to the next line and the previous line didn't have
	// any mappings, add a mapping at the start of the previous line.
	if !p.lineStartsWithMapping && p.hasPrevState {
		p.appendMappingWithoutRemapping(SourceMapState{
			GeneratedLine:   p.prevState.GeneratedLine,
			GeneratedColumn: 0,
			SourceIndex:     p.prevState.SourceIndex,
			OriginalLine:    p.prevState.OriginalLine,
			OriginalColumn:  p.prevState.OriginalColumn,
		})
	}

	p.prevLineStart = prevLineStart
	p.prevState.GeneratedLine++
	p.prevState.GeneratedColumn = 0
	p.sourceMap = append(p.sourceMap, ';')

	// This new line doesn't have a mapping yet
	p.lineStartsWithMapping = false
}

func (p *printer) printIndent() {
	if !p.options.RemoveWhitespace {
		for i := 0; i < p.options.Indent; i++ {
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

			if !b.IsSingleLine {
				p.options.Indent--
				p.printNewline()
				p.printIndent()
			}
		}
		p.print("]")

	case *ast.BObject:
		p.print("{")
		if len(b.Properties) > 0 {
			if !b.IsSingleLine {
				p.options.Indent++
			}

			for i, property := range b.Properties {
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

				if property.IsSpread {
					p.print("...")
				} else {
					if property.IsComputed {
						p.print("[")
						p.printExpr(property.Key, ast.LComma, 0)
						p.print("]:")
						p.printSpace()
						p.printBinding(property.Value)

						if property.DefaultValue != nil {
							p.printSpace()
							p.print("=")
							p.printSpace()
							p.printExpr(*property.DefaultValue, ast.LComma, 0)
						}
						continue
					}

					if str, ok := property.Key.Data.(*ast.EString); ok {
						if lexer.IsIdentifierUTF16(str.Value) {
							p.addSourceMapping(property.Key.Loc)
							p.printSpaceBeforeIdentifier()
							p.printUTF16(str.Value)

							// Use a shorthand property if the names are the same
							if id, ok := property.Value.Data.(*ast.BIdentifier); ok && lexer.UTF16EqualsString(str.Value, p.symbolName(id.Ref)) {
								if property.DefaultValue != nil {
									p.printSpace()
									p.print("=")
									p.printSpace()
									p.printExpr(*property.DefaultValue, ast.LComma, 0)
								}
								continue
							}
						} else {
							p.printExpr(property.Key, ast.LLowest, 0)
						}
					} else {
						p.printExpr(property.Key, ast.LLowest, 0)
					}

					p.print(":")
					p.printSpace()
				}
				p.printBinding(property.Value)

				if property.DefaultValue != nil {
					p.printSpace()
					p.print("=")
					p.printSpace()
					p.printExpr(*property.DefaultValue, ast.LComma, 0)
				}
			}

			if !b.IsSingleLine {
				p.options.Indent--
				p.printNewline()
				p.printIndent()
			}
		}
		p.print("}")

	default:
		panic(fmt.Sprintf("Unexpected binding of type %T", binding.Data))
	}
}

func (p *printer) printSpace() {
	if !p.options.RemoveWhitespace {
		p.print(" ")
	}
}

func (p *printer) printNewline() {
	if !p.options.RemoveWhitespace {
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
	if !p.options.RemoveWhitespace {
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
	if p.options.RemoveWhitespace && !hasRestArg && isArrow && len(args) == 1 {
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
	p.printBlock(fn.Body.Stmts)
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
	p.options.Indent++

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
	p.options.Indent--
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
		p.printExpr(item.Key, ast.LComma, 0)
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

	switch key := item.Key.Data.(type) {
	case *ast.EPrivateIdentifier:
		p.printSymbol(key.Ref)

	case *ast.EString:
		p.addSourceMapping(item.Key.Loc)
		if lexer.IsIdentifierUTF16(key.Value) {
			p.printSpaceBeforeIdentifier()
			p.printUTF16(key.Value)

			// Use a shorthand property if the names are the same
			if !p.options.UnsupportedFeatures.Has(compat.ObjectExtensions) && item.Value != nil {
				switch e := item.Value.Data.(type) {
				case *ast.EIdentifier:
					if lexer.UTF16EqualsString(key.Value, p.symbolName(e.Ref)) {
						if item.Initializer != nil {
							p.printSpace()
							p.print("=")
							p.printSpace()
							p.printExpr(*item.Initializer, ast.LComma, 0)
						}
						return
					}

				case *ast.EImportIdentifier:
					// Make sure we're not using a property access instead of an identifier
					ref := ast.FollowSymbols(p.symbols, e.Ref)
					symbol := p.symbols.Get(ref)
					if symbol.NamespaceAlias == nil && lexer.UTF16EqualsString(key.Value, symbol.Name) {
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
			c := p.bestQuoteCharForString(key.Value, false /* allowBacktick */)
			p.print(c)
			p.printQuotedUTF16(key.Value, rune(c[0]))
			p.print(c)
		}

	default:
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
	if p.options.UnsupportedFeatures.Has(compat.TemplateLiteral) {
		allowBacktick = false
	}

	singleCost := 0
	doubleCost := 0
	backtickCost := 0

	for i, c := range data {
		switch c {
		case '\n':
			if p.options.RemoveWhitespace {
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
	return c
}

type requireCallArgs struct {
	isES6Import       bool
	mustReturnPromise bool
}

func (p *printer) printRequireOrImportExpr(importRecordIndex uint32) {
	space := " "
	if p.options.RemoveWhitespace {
		space = ""
	}

	record := &p.importRecords[importRecordIndex]
	p.printSpaceBeforeIdentifier()

	// Preserve "import()" expressions that don't point inside the bundle
	if record.SourceIndex == nil && record.Kind == ast.ImportDynamic && p.options.OutputFormat.KeepES6ImportExportSyntax() {
		p.print("import(")
		p.print(Quote(record.Path.Text))
		p.print(")")
		return
	}

	// Make sure "import()" expressions return promises
	if record.Kind == ast.ImportDynamic {
		p.print("Promise.resolve().then(()" + space + "=>" + space)
	}

	// Make sure CommonJS imports are converted to ES6 if necessary
	if record.WrapWithToModule {
		p.printSymbol(p.options.ToModuleRef)
		p.print("(")
	}

	// If this import points inside the bundle, then call the "require()"
	// function for that module directly. The linker must ensure that the
	// module's require function exists by this point. Otherwise, fall back to a
	// bare "require()" call. Then it's up to the user to provide it.
	if record.SourceIndex != nil {
		p.printSymbol(record.WrapperRef)
		p.print("()")
	} else {
		p.print("require(")
		p.print(Quote(record.Path.Text))
		p.print(")")
	}

	if record.WrapWithToModule {
		p.print(")")
	}

	if record.Kind == ast.ImportDynamic {
		p.print(")")
	}
}

const (
	forbidCall = 1 << iota
	forbidIn
	hasNonOptionalChainParent
)

func (p *printer) printUndefined(level ast.L) {
	if level >= ast.LPrefix {
		p.print("(void 0)")
	} else {
		p.printSpaceBeforeIdentifier()
		p.print("void 0")
		p.prevNumEnd = len(p.js)
	}
}

func (p *printer) printExpr(expr ast.Expr, level ast.L, flags int) {
	p.addSourceMapping(expr.Loc)

	switch e := expr.Data.(type) {
	case *ast.EMissing:

	case *ast.EUndefined:
		p.printUndefined(level)

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
		p.printExpr(e.Value, ast.LComma, 0)

	case *ast.ENewTarget:
		p.printSpaceBeforeIdentifier()
		p.print("new.target")

	case *ast.EImportMeta:
		p.printSpaceBeforeIdentifier()
		p.print("import.meta")

	case *ast.ENew:
		wrap := level >= ast.LCall

		hasPureComment := !p.options.RemoveWhitespace && e.CanBeUnwrappedIfUnused
		if hasPureComment && level >= ast.LPostfix {
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
		targetFlags := 0
		if e.OptionalChain == ast.OptionalChainNone {
			targetFlags = hasNonOptionalChainParent
		} else if (flags & hasNonOptionalChainParent) != 0 {
			wrap = true
		}

		hasPureComment := !p.options.RemoveWhitespace && e.CanBeUnwrappedIfUnused
		if hasPureComment && level >= ast.LPostfix {
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
		if !e.IsDirectEval && p.isUnboundEvalIdentifier(e.Target) {
			if p.options.RemoveWhitespace {
				p.print("(0,")
			} else {
				p.print("(0, ")
			}
			p.printExpr(e.Target, ast.LPostfix, 0)
			p.print(")")
		} else {
			p.printExpr(e.Target, ast.LPostfix, targetFlags)
		}

		if e.OptionalChain == ast.OptionalChainStart {
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
		p.printRequireOrImportExpr(e.ImportRecordIndex)
		if wrap {
			p.print(")")
		}

	case *ast.EImport:
		wrap := level >= ast.LNew || (flags&forbidCall) != 0
		if wrap {
			p.print("(")
		}
		if e.ImportRecordIndex != nil {
			p.printRequireOrImportExpr(*e.ImportRecordIndex)
		} else {
			// Handle non-string expressions
			p.printSpaceBeforeIdentifier()
			p.print("import(")
			p.printExpr(e.Expr, ast.LComma, 0)
			p.print(")")
		}
		if wrap {
			p.print(")")
		}

	case *ast.EDot:
		wrap := false
		if e.OptionalChain == ast.OptionalChainNone {
			flags |= hasNonOptionalChainParent
		} else {
			if (flags & hasNonOptionalChainParent) != 0 {
				wrap = true
				p.print("(")
			}
			flags &= ^hasNonOptionalChainParent
		}
		p.printExpr(e.Target, ast.LPostfix, flags)
		if e.OptionalChain == ast.OptionalChainStart {
			p.print("?")
		} else if p.prevNumEnd == len(p.js) {
			// "1.toString" is a syntax error, so print "1 .toString" instead
			p.print(" ")
		}
		p.print(".")
		p.addSourceMapping(e.NameLoc)
		p.print(e.Name)
		if wrap {
			p.print(")")
		}

	case *ast.EIndex:
		wrap := false
		if e.OptionalChain == ast.OptionalChainNone {
			flags |= hasNonOptionalChainParent
		} else {
			if (flags & hasNonOptionalChainParent) != 0 {
				wrap = true
				p.print("(")
			}
			flags &= ^hasNonOptionalChainParent
		}
		p.printExpr(e.Target, ast.LPostfix, flags)
		if e.OptionalChain == ast.OptionalChainStart {
			p.print("?.")
		}
		if private, ok := e.Index.Data.(*ast.EPrivateIdentifier); ok {
			if e.OptionalChain != ast.OptionalChainStart {
				p.print(".")
			}
			p.printSymbol(private.Ref)
		} else {
			p.print("[")
			p.printExpr(e.Index, ast.LLowest, 0)
			p.print("]")
		}
		if wrap {
			p.print(")")
		}

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
		n := len(p.js)
		useFunction := p.options.UnsupportedFeatures.Has(compat.Arrow)
		wrap := level >= ast.LAssign || (useFunction && (p.stmtStart == n || p.exportDefaultStart == n))

		if wrap {
			p.print("(")
		}
		if e.IsAsync {
			p.printSpaceBeforeIdentifier()
			p.print("async")
			p.printSpace()
		}

		if useFunction {
			p.printSpaceBeforeIdentifier()
			p.print("function")
		}

		p.printFnArgs(e.Args, e.HasRestArg, true)
		p.printSpace()

		if !useFunction {
			p.print("=>")
			p.printSpace()
		}

		wasPrinted := false
		if len(e.Body.Stmts) == 1 && e.PreferExpr && !useFunction {
			if s, ok := e.Body.Stmts[0].Data.(*ast.SReturn); ok && s.Value != nil {
				p.arrowExprStart = len(p.js)
				p.printExpr(*s.Value, ast.LComma, 0)
				wasPrinted = true
			}
		}
		if !wasPrinted {
			p.printBlock(e.Body.Stmts)
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
				p.printExpr(item, ast.LComma, 0)

				// Make sure there's a comma after trailing missing items
				_, ok := item.Data.(*ast.EMissing)
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

	case *ast.EObject:
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
					if e.IsSingleLine {
						p.printSpace()
					}
				}
				if !e.IsSingleLine {
					p.printNewline()
					p.printIndent()
				}
				p.printProperty(item)
			}

			if !e.IsSingleLine {
				p.options.Indent--
				p.printNewline()
				p.printIndent()
			}
		}
		p.print("}")
		if wrap {
			p.print(")")
		}

	case *ast.EBoolean:
		if p.options.RemoveWhitespace {
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
		p.printQuotedUTF16(e.Value, rune(c[0]))
		p.print(c)

	case *ast.ETemplate:
		// Convert no-substitution template literals into strings if it's smaller
		if p.options.RemoveWhitespace && e.Tag == nil && len(e.Parts) == 0 {
			c := p.bestQuoteCharForString(e.Head, true /* allowBacktick */)
			p.print(c)
			p.printQuotedUTF16(e.Head, rune(c[0]))
			p.print(c)
			return
		}

		if e.Tag != nil {
			p.printExpr(*e.Tag, ast.LPostfix, 0)
		}
		p.print("`")
		if e.Tag != nil {
			p.print(e.HeadRaw)
		} else {
			p.printQuotedUTF16(e.Head, '`')
		}
		for _, part := range e.Parts {
			p.print("${")
			p.printExpr(part.Value, ast.LLowest, 0)
			p.print("}")
			if e.Tag != nil {
				p.print(part.TailRaw)
			} else {
				p.printQuotedUTF16(part.Tail, '`')
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
		absValue := math.Abs(value)

		if value != value {
			p.printSpaceBeforeIdentifier()
			p.print("NaN")
		} else if value == positiveInfinity {
			p.printSpaceBeforeIdentifier()
			p.print("Infinity")
		} else if value == negativeInfinity {
			if level >= ast.LExponentiation {
				p.print("(-Infinity)")
			} else {
				p.printSpaceBeforeOperator(ast.UnOpNeg)
				p.print("-Infinity")
			}
		} else {
			if !math.Signbit(value) {
				p.printSpaceBeforeIdentifier()
				p.printNonNegativeFloat(absValue)

				// Remember the end of the latest number
				p.prevNumEnd = len(p.js)
			} else if level >= ast.LExponentiation {
				// Expressions such as "(-1).toString" need to wrap negative numbers.
				// Instead of testing for "value < 0" we test for "signbit(value)" and
				// "!isNaN(value)" because we need this to be true for "-0" and "-0 < 0"
				// is false.
				p.print("(-")
				p.printNonNegativeFloat(absValue)
				p.print(")")
			} else {
				p.printSpaceBeforeOperator(ast.UnOpNeg)
				p.print("-")
				p.printNonNegativeFloat(absValue)

				// Remember the end of the latest number
				p.prevNumEnd = len(p.js)
			}
		}

	case *ast.EIdentifier:
		p.printSpaceBeforeIdentifier()
		p.printSymbol(e.Ref)

	case *ast.EImportIdentifier:
		// Potentially use a property access instead of an identifier
		ref := ast.FollowSymbols(p.symbols, e.Ref)
		symbol := p.symbols.Get(ref)

		if symbol.ImportItemStatus == ast.ImportItemMissing {
			p.printUndefined(level)
		} else if symbol.NamespaceAlias != nil {
			p.printSymbol(symbol.NamespaceAlias.NamespaceRef)
			p.print(".")
			p.print(symbol.NamespaceAlias.Alias)
		} else {
			p.printSpaceBeforeIdentifier()
			p.print(symbol.Name)
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
			if left, ok := e.Left.Data.(*ast.EUnary); ok && left.Op.UnaryAssignTarget() == ast.AssignTargetNone {
				leftLevel = ast.LCall
			} else if _, ok := e.Left.Data.(*ast.EUndefined); ok {
				// Undefined is printed as "void 0"
				leftLevel = ast.LCall
			} else if p.options.RemoveWhitespace {
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

func (p *printer) isUnboundEvalIdentifier(value ast.Expr) bool {
	if id, ok := value.Data.(*ast.EIdentifier); ok {
		symbol := p.symbols.Get(ast.FollowSymbols(p.symbols, id.Ref))
		return symbol.Kind == ast.SymbolUnbound && symbol.Name == "eval"
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
		if p.options.RemoveWhitespace {
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
		p.options.Indent++
		p.printStmt(body)
		p.options.Indent--
	}
}

func (p *printer) printBlock(stmts []ast.Stmt) {
	p.print("{")
	p.printNewline()

	p.options.Indent++
	for _, stmt := range stmts {
		p.printSemicolonIfNeeded()
		p.printStmt(stmt)
	}
	p.options.Indent--
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

		p.options.Indent++
		p.printStmt(s.Yes)
		p.options.Indent--
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
		p.options.Indent++
		p.printStmt(s.Yes)
		p.options.Indent--

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
			p.options.Indent++
			p.printStmt(*s.No)
			p.options.Indent--
		}
	}
}

func (p *printer) printStmt(stmt ast.Stmt) {
	p.addSourceMapping(stmt.Loc)

	switch s := stmt.Data.(type) {
	case *ast.SComment:
		text := s.Text
		if p.options.ExtractComments {
			if p.extractedComments == nil {
				p.extractedComments = make(map[string]bool)
			}
			p.extractedComments[text] = true
			break
		}
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
		if s.Alias != nil {
			p.print("as")
			p.printSpace()
			p.printSpaceBeforeIdentifier()
			p.print(s.Alias.Name)
			p.printSpace()
			p.printSpaceBeforeIdentifier()
		}
		p.print("from")
		p.printSpace()
		p.print(Quote(p.importRecords[s.ImportRecordIndex].Path.Text))
		p.printSemicolonAfterStatement()

	case *ast.SExportClause:
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
				if s.IsSingleLine {
					p.printSpace()
				}
			}

			if !s.IsSingleLine {
				p.printNewline()
				p.printIndent()
			}
			name := p.symbolName(item.Name.Ref)
			p.print(name)
			if name != item.Alias {
				p.print(" as ")
				p.print(item.Alias)
			}
		}

		if !s.IsSingleLine {
			p.options.Indent--
			p.printNewline()
			p.printIndent()
		}

		p.print("}")
		p.printSemicolonAfterStatement()

	case *ast.SExportFrom:
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
				if s.IsSingleLine {
					p.printSpace()
				}
			}

			if !s.IsSingleLine {
				p.printNewline()
				p.printIndent()
			}
			p.print(item.OriginalName)
			if item.OriginalName != item.Alias {
				p.print(" as ")
				p.print(item.Alias)
			}
		}

		if !s.IsSingleLine {
			p.options.Indent--
			p.printNewline()
			p.printIndent()
		}

		p.print("}")
		p.printSpace()
		p.print("from")
		p.printSpace()
		p.print(Quote(p.importRecords[s.ImportRecordIndex].Path.Text))
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
			p.options.Indent++
			p.printStmt(s.Body)
			p.printSemicolonIfNeeded()
			p.options.Indent--
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
		p.printExpr(s.Value, ast.LComma, 0)
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
		p.printExpr(s.Value, ast.LComma, 0)
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
		p.options.Indent++

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
			p.options.Indent++
			for _, stmt := range c.Body {
				p.printSemicolonIfNeeded()
				p.printStmt(stmt)
			}
			p.options.Indent--
		}

		p.options.Indent--
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
			if !s.IsSingleLine {
				p.options.Indent++
			}

			for i, item := range *s.Items {
				if i != 0 {
					p.print(",")
					if s.IsSingleLine {
						p.printSpace()
					}
				}

				if !s.IsSingleLine {
					p.printNewline()
					p.printIndent()
				}
				p.print(item.Alias)
				name := p.symbolName(item.Name.Ref)
				if name != item.Alias {
					p.print(" as ")
					p.print(name)
				}
			}

			if !s.IsSingleLine {
				p.options.Indent--
				p.printNewline()
				p.printIndent()
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

		p.print(Quote(p.importRecords[s.ImportRecordIndex].Path.Text))
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
		p.printQuotedUTF16(s.Value, rune(c[0]))
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

func (p *printer) shouldIgnoreSourceMap() bool {
	for _, c := range p.sourceMap {
		if c != ';' {
			return false
		}
	}
	return true
}

type PrintOptions struct {
	OutputFormat        config.Format
	RemoveWhitespace    bool
	ExtractComments     bool
	Indent              int
	ToModuleRef         ast.Ref
	UnsupportedFeatures compat.Feature

	// This contains the contents of the input file to map back to in the source
	// map. If it's nil that means we're not generating source maps.
	SourceForSourceMap *logging.Source

	// This will be present if the input file had a source map. In that case we
	// want to map all the way back to the original input file(s).
	InputSourceMap *sourcemap.SourceMap
}

type QuotedSource struct {
	// These are quoted ahead of time instead of during source map generation so
	// the quoting happens in parallel instead of in serial
	QuotedPath     string
	QuotedContents string
}

type SourceMapChunk struct {
	Buffer []byte

	// There may be more than one source for this chunk if the file being printed
	// has an associated source map. In that case the "source index" values in
	// the buffer are 0-based indices into this array. The source index of the
	// first mapping will be adjusted when the chunks are joined together. Since
	// the source indices are encoded using a delta from the previous source
	// index, none of the other source indices need to be modified while joining.
	QuotedSources []QuotedSource

	// This end state will be used to rewrite the start of the following source
	// map chunk so that the delta-encoded VLQ numbers are preserved.
	EndState SourceMapState

	// There probably isn't a source mapping at the end of the file (nor should
	// there be) but if we're appending another source map chunk after this one,
	// we'll need to know how many characters were in the last line we generated.
	FinalGeneratedColumn int

	ShouldIgnore bool
}

func createPrinter(
	symbols ast.SymbolMap,
	importRecords []ast.ImportRecord,
	options PrintOptions,
) *printer {
	p := &printer{
		symbols:            symbols,
		importRecords:      importRecords,
		options:            options,
		stmtStart:          -1,
		exportDefaultStart: -1,
		arrowExprStart:     -1,
		prevOpEnd:          -1,
		prevNumEnd:         -1,
		prevRegExpEnd:      -1,
		prevLoc:            ast.Loc{Start: -1},
	}

	// If we're writing out a source map, prepare a table of line start indices
	// to do binary search on to figure out what line a given AST node came from
	if options.SourceForSourceMap != nil {
		lineStarts := []int32{}
		var prevCodePoint rune
		for i, codePoint := range options.SourceForSourceMap.Contents {
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

type PrintResult struct {
	JS []byte

	// This source map chunk just contains the VLQ-encoded offsets for the "JS"
	// field above. It's not a full source map. The bundler will be joining many
	// source map chunks together to form the final source map.
	SourceMapChunk SourceMapChunk

	ExtractedComments map[string]bool
}

func Print(tree ast.AST, options PrintOptions) PrintResult {
	p := createPrinter(tree.Symbols, tree.ImportRecords, options)

	for _, part := range tree.Parts {
		for _, stmt := range part.Stmts {
			p.printStmt(stmt)
			p.printSemicolonIfNeeded()
		}
	}

	return PrintResult{
		JS:                p.js,
		ExtractedComments: p.extractedComments,
		SourceMapChunk: SourceMapChunk{
			Buffer:               p.sourceMap,
			QuotedSources:        quotedSources(&tree, &options),
			EndState:             p.prevState,
			FinalGeneratedColumn: len(p.js) - p.prevLineStart,
			ShouldIgnore:         p.shouldIgnoreSourceMap(),
		},
	}
}

func PrintExpr(expr ast.Expr, symbols ast.SymbolMap, options PrintOptions) PrintResult {
	p := createPrinter(symbols, nil, options)

	p.printExpr(expr, ast.LLowest, 0)

	return PrintResult{
		JS:                p.js,
		ExtractedComments: p.extractedComments,
		SourceMapChunk: SourceMapChunk{
			Buffer:               p.sourceMap,
			QuotedSources:        quotedSources(nil, &options),
			EndState:             p.prevState,
			FinalGeneratedColumn: len(p.js) - p.prevLineStart,
			ShouldIgnore:         p.shouldIgnoreSourceMap(),
		},
	}
}

func quotedSources(tree *ast.AST, options *PrintOptions) []QuotedSource {
	if options.SourceForSourceMap == nil {
		return nil
	}

	if sm := options.InputSourceMap; sm != nil {
		results := make([]QuotedSource, len(sm.Sources))
		for i, source := range sm.Sources {
			contents := "null"
			if i < len(sm.SourcesContent) {
				if value := sm.SourcesContent[i]; value != nil {
					contents = QuoteForJSON(*value)
				}
			}
			results[i] = QuotedSource{
				QuotedPath:     QuoteForJSON(source),
				QuotedContents: contents,
			}
		}
		return results
	}

	return []QuotedSource{{
		QuotedPath:     QuoteForJSON(options.SourceForSourceMap.PrettyPath),
		QuotedContents: QuoteForJSON(options.SourceForSourceMap.Contents),
	}}
}
