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
func AppendSourceMapChunk(j *helpers.Joiner, prevEndState SourceMapState, startState SourceMapState, sourceMap []byte) {
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
	startState.OriginalLine += originalLine
	startState.OriginalColumn += originalColumn
	j.AddBytes(appendMapping(nil, j.LastByte(), prevEndState, startState))

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
		c, width := js_lexer.DecodeWTF8Rune(text[i:])

		// Fast path: a run of characters that don't need escaping
		if canPrintWithoutEscape(c, asciiOnly) {
			start := i
			i += width
			for i < n {
				c, width = js_lexer.DecodeWTF8Rune(text[i:])
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

func (p *printer) printQuotedUTF16(text []uint16, quote rune) {
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

type printer struct {
	symbols                js_ast.SymbolMap
	renamer                renamer.Renamer
	importRecords          []ast.ImportRecord
	options                Options
	extractedLegalComments map[string]bool
	needsSemicolon         bool
	js                     []byte
	stmtStart              int
	exportDefaultStart     int
	arrowExprStart         int
	forOfInitStart         int
	prevOp                 js_ast.OpCode
	prevOpEnd              int
	prevNumEnd             int
	prevRegExpEnd          int
	callTarget             js_ast.E
	intToBytesBuffer       [64]byte

	// For source maps
	sourceMap           []byte
	prevLoc             logger.Loc
	prevState           SourceMapState
	lastGeneratedUpdate int
	generatedColumn     int
	hasPrevState        bool
	lineOffsetTables    []LineOffsetTable

	// This is a workaround for a bug in the popular "source-map" library:
	// https://github.com/mozilla/source-map/issues/261. The library will
	// sometimes return null when querying a source map unless every line
	// starts with a mapping at column zero.
	//
	// The workaround is to replicate the previous mapping if a line ends
	// up not starting with a mapping. This is done lazily because we want
	// to avoid replicating the previous mapping if we don't need to.
	lineStartsWithMapping     bool
	coverLinesWithoutMappings bool
}

type LineOffsetTable struct {
	byteOffsetToStartOfLine int32

	// The source map specification is very loose and does not specify what
	// column numbers actually mean. The popular "source-map" library from Mozilla
	// appears to interpret them as counts of UTF-16 code units, so we generate
	// those too for compatibility.
	//
	// We keep mapping tables around to accelerate conversion from byte offsets
	// to UTF-16 code unit counts. However, this mapping takes up a lot of memory
	// and generates a lot of garbage. Since most JavaScript is ASCII and the
	// mapping for ASCII is 1:1, we avoid creating a table for ASCII-only lines
	// as an optimization.
	byteOffsetToFirstNonASCII int32
	columnsForNonASCII        []int32
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
	value := js_lexer.StringToUTF16(text)
	c := p.bestQuoteCharForString(value, allowBacktick)
	p.print(c)
	p.printQuotedUTF16(value, rune(c[0]))
	p.print(c)
}

func (p *printer) addSourceMapping(loc logger.Loc) {
	if !p.options.AddSourceMappings || loc == p.prevLoc {
		return
	}
	p.prevLoc = loc

	// Binary search to find the line
	lineOffsetTables := p.lineOffsetTables
	count := len(lineOffsetTables)
	originalLine := 0
	for count > 0 {
		step := count / 2
		i := originalLine + step
		if lineOffsetTables[i].byteOffsetToStartOfLine <= loc.Start {
			originalLine = i + 1
			count = count - step - 1
		} else {
			count = step
		}
	}
	originalLine--

	// Use the line to compute the column
	line := &lineOffsetTables[originalLine]
	originalColumn := int(loc.Start - line.byteOffsetToStartOfLine)
	if line.columnsForNonASCII != nil && originalColumn >= int(line.byteOffsetToFirstNonASCII) {
		originalColumn = int(line.columnsForNonASCII[originalColumn-int(line.byteOffsetToFirstNonASCII)])
	}

	p.updateGeneratedLineAndColumn()

	// If this line doesn't start with a mapping and we're about to add a mapping
	// that's not at the start, insert a mapping first so the line starts with one.
	if p.coverLinesWithoutMappings && !p.lineStartsWithMapping && p.generatedColumn > 0 && p.hasPrevState {
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
		GeneratedColumn: p.generatedColumn,
		OriginalLine:    originalLine,
		OriginalColumn:  originalColumn,
	})

	// This line now has a mapping on it, so don't insert another one
	p.lineStartsWithMapping = true
}

// Scan over the printed text since the last source mapping and update the
// generated line and column numbers
func (p *printer) updateGeneratedLineAndColumn() {
	for i, c := range string(p.js[p.lastGeneratedUpdate:]) {
		switch c {
		case '\r', '\n', '\u2028', '\u2029':
			// Handle Windows-specific "\r\n" newlines
			if c == '\r' {
				newlineCheck := p.lastGeneratedUpdate + i + 1
				if newlineCheck < len(p.js) && p.js[newlineCheck] == '\n' {
					continue
				}
			}

			// If we're about to move to the next line and the previous line didn't have
			// any mappings, add a mapping at the start of the previous line.
			if p.coverLinesWithoutMappings && !p.lineStartsWithMapping && p.hasPrevState {
				p.appendMappingWithoutRemapping(SourceMapState{
					GeneratedLine:   p.prevState.GeneratedLine,
					GeneratedColumn: 0,
					SourceIndex:     p.prevState.SourceIndex,
					OriginalLine:    p.prevState.OriginalLine,
					OriginalColumn:  p.prevState.OriginalColumn,
				})
			}

			p.prevState.GeneratedLine++
			p.prevState.GeneratedColumn = 0
			p.generatedColumn = 0
			p.sourceMap = append(p.sourceMap, ';')

			// This new line doesn't have a mapping yet
			p.lineStartsWithMapping = false

		default:
			// Mozilla's "source-map" library counts columns using UTF-16 code units
			if c <= 0xFFFF {
				p.generatedColumn++
			} else {
				p.generatedColumn += 2
			}
		}
	}

	p.lastGeneratedUpdate = len(p.js)
}

func GenerateLineOffsetTables(contents string, approximateLineCount int32) []LineOffsetTable {
	var columnsForNonASCII []int32
	byteOffsetToFirstNonASCII := int32(0)
	lineByteOffset := 0
	columnByteOffset := 0
	column := int32(0)

	// Preallocate the top-level table using the approximate line count from the lexer
	lineOffsetTables := make([]LineOffsetTable, 0, approximateLineCount)

	for i, c := range contents {
		// Mark the start of the next line
		if column == 0 {
			lineByteOffset = i
		}

		// Start the mapping if this character is non-ASCII
		if c > 0x7F && columnsForNonASCII == nil {
			columnByteOffset = i - lineByteOffset
			byteOffsetToFirstNonASCII = int32(columnByteOffset)
			columnsForNonASCII = []int32{}
		}

		// Update the per-byte column offsets
		if columnsForNonASCII != nil {
			for lineBytesSoFar := i - lineByteOffset; columnByteOffset <= lineBytesSoFar; columnByteOffset++ {
				columnsForNonASCII = append(columnsForNonASCII, column)
			}
		}

		switch c {
		case '\r', '\n', '\u2028', '\u2029':
			// Handle Windows-specific "\r\n" newlines
			if c == '\r' && i+1 < len(contents) && contents[i+1] == '\n' {
				column++
				continue
			}

			lineOffsetTables = append(lineOffsetTables, LineOffsetTable{
				byteOffsetToStartOfLine:   int32(lineByteOffset),
				byteOffsetToFirstNonASCII: byteOffsetToFirstNonASCII,
				columnsForNonASCII:        columnsForNonASCII,
			})
			columnByteOffset = 0
			byteOffsetToFirstNonASCII = 0
			columnsForNonASCII = nil
			column = 0

		default:
			// Mozilla's "source-map" library counts columns using UTF-16 code units
			if c <= 0xFFFF {
				column++
			} else {
				column += 2
			}
		}
	}

	// Mark the start of the next line
	if column == 0 {
		lineByteOffset = len(contents)
	}

	// Do one last update for the column at the end of the file
	if columnsForNonASCII != nil {
		for lineBytesSoFar := len(contents) - lineByteOffset; columnByteOffset <= lineBytesSoFar; columnByteOffset++ {
			columnsForNonASCII = append(columnsForNonASCII, column)
		}
	}

	lineOffsetTables = append(lineOffsetTables, LineOffsetTable{
		byteOffsetToStartOfLine:   int32(lineByteOffset),
		byteOffsetToFirstNonASCII: byteOffsetToFirstNonASCII,
		columnsForNonASCII:        columnsForNonASCII,
	})
	return lineOffsetTables
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

func (p *printer) printIndent() {
	if !p.options.RemoveWhitespace {
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

func CanQuoteIdentifier(name string, unsupportedJSFeatures compat.JSFeature, asciiOnly bool) bool {
	return js_lexer.IsIdentifier(name) && (!asciiOnly ||
		!unsupportedJSFeatures.Has(compat.UnicodeEscapes) ||
		!js_lexer.ContainsNonBMPCodePoint(name))
}

func (p *printer) canPrintIdentifier(name string) bool {
	return js_lexer.IsIdentifier(name) && (!p.options.ASCIIOnly ||
		!p.options.UnsupportedFeatures.Has(compat.UnicodeEscapes) ||
		!js_lexer.ContainsNonBMPCodePoint(name))
}

func (p *printer) canPrintIdentifierUTF16(name []uint16) bool {
	return js_lexer.IsIdentifierUTF16(name) && (!p.options.ASCIIOnly ||
		!p.options.UnsupportedFeatures.Has(compat.UnicodeEscapes) ||
		!js_lexer.ContainsNonBMPCodePointUTF16(name))
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
	temp := make([]byte, utf8.UTFMax)
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

		width := utf8.EncodeRune(temp, c)
		p.js = append(p.js, temp[:width]...)
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

				if item.DefaultValue != nil {
					p.printSpace()
					p.print("=")
					p.printSpace()
					p.printExpr(*item.DefaultValue, js_ast.LComma, 0)
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
						p.printExpr(property.Key, js_ast.LComma, 0)
						p.print("]:")
						p.printSpace()
						p.printBinding(property.Value)

						if property.DefaultValue != nil {
							p.printSpace()
							p.print("=")
							p.printSpace()
							p.printExpr(*property.DefaultValue, js_ast.LComma, 0)
						}
						continue
					}

					if str, ok := property.Key.Data.(*js_ast.EString); ok && !property.PreferQuotedKey && p.canPrintIdentifierUTF16(str.Value) {
						p.addSourceMapping(property.Key.Loc)
						p.printSpaceBeforeIdentifier()
						p.printIdentifierUTF16(str.Value)

						// Use a shorthand property if the names are the same
						if id, ok := property.Value.Data.(*js_ast.BIdentifier); ok && js_lexer.UTF16EqualsString(str.Value, p.renamer.NameForSymbol(id.Ref)) {
							if property.DefaultValue != nil {
								p.printSpace()
								p.print("=")
								p.printSpace()
								p.printExpr(*property.DefaultValue, js_ast.LComma, 0)
							}
							continue
						}
					} else {
						p.printExpr(property.Key, js_ast.LLowest, 0)
					}

					p.print(":")
					p.printSpace()
				}
				p.printBinding(property.Value)

				if property.DefaultValue != nil {
					p.printSpace()
					p.print("=")
					p.printSpace()
					p.printExpr(*property.DefaultValue, js_ast.LComma, 0)
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
	if n > 0 && (js_lexer.IsIdentifierContinue(rune(buffer[n-1])) || n == p.prevRegExpEnd) {
		p.print(" ")
	}
}

func (p *printer) printFnArgs(args []js_ast.Arg, hasRestArg bool, isArrow bool) {
	wrap := true

	// Minify "(a) => {}" as "a=>{}"
	if p.options.RemoveWhitespace && !hasRestArg && isArrow && len(args) == 1 {
		if _, ok := args[0].Binding.Data.(*js_ast.BIdentifier); ok && args[0].Default == nil {
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
			p.printExpr(*arg.Default, js_ast.LComma, 0)
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
	if class.Extends != nil {
		p.print(" extends")
		p.printSpace()
		p.printExpr(*class.Extends, js_ast.LNew-1, 0)
	}
	p.printSpace()

	p.addSourceMapping(class.BodyLoc)
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

func (p *printer) printProperty(item js_ast.Property) {
	if item.Kind == js_ast.PropertySpread {
		p.print("...")
		p.printExpr(*item.Value, js_ast.LComma, 0)
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

	if item.Value != nil {
		if fn, ok := item.Value.Data.(*js_ast.EFunction); item.IsMethod && ok {
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
		p.printExpr(item.Key, js_ast.LComma, 0)
		p.print("]")

		if item.Value != nil {
			if fn, ok := item.Value.Data.(*js_ast.EFunction); item.IsMethod && ok {
				p.printFn(fn.Fn)
				return
			}

			p.print(":")
			p.printSpace()
			p.printExpr(*item.Value, js_ast.LComma, 0)
		}

		if item.Initializer != nil {
			p.printSpace()
			p.print("=")
			p.printSpace()
			p.printExpr(*item.Initializer, js_ast.LComma, 0)
		}
		return
	}

	switch key := item.Key.Data.(type) {
	case *js_ast.EPrivateIdentifier:
		p.printSymbol(key.Ref)

	case *js_ast.EString:
		p.addSourceMapping(item.Key.Loc)
		if !item.PreferQuotedKey && p.canPrintIdentifierUTF16(key.Value) {
			p.printSpaceBeforeIdentifier()
			p.printIdentifierUTF16(key.Value)

			// Use a shorthand property if the names are the same
			if !p.options.UnsupportedFeatures.Has(compat.ObjectExtensions) && item.Value != nil {
				switch e := item.Value.Data.(type) {
				case *js_ast.EIdentifier:
					if js_lexer.UTF16EqualsString(key.Value, p.renamer.NameForSymbol(e.Ref)) {
						if item.Initializer != nil {
							p.printSpace()
							p.print("=")
							p.printSpace()
							p.printExpr(*item.Initializer, js_ast.LComma, 0)
						}
						return
					}

				case *js_ast.EImportIdentifier:
					// Make sure we're not using a property access instead of an identifier
					ref := js_ast.FollowSymbols(p.symbols, e.Ref)
					symbol := p.symbols.Get(ref)
					if symbol.NamespaceAlias == nil && js_lexer.UTF16EqualsString(key.Value, p.renamer.NameForSymbol(e.Ref)) {
						if item.Initializer != nil {
							p.printSpace()
							p.print("=")
							p.printSpace()
							p.printExpr(*item.Initializer, js_ast.LComma, 0)
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
		p.printExpr(item.Key, js_ast.LLowest, 0)
	}

	if item.Kind != js_ast.PropertyNormal {
		f, ok := item.Value.Data.(*js_ast.EFunction)
		if ok {
			p.printFn(f.Fn)
			return
		}
	}

	if item.Value != nil {
		if fn, ok := item.Value.Data.(*js_ast.EFunction); item.IsMethod && ok {
			p.printFn(fn.Fn)
			return
		}

		p.print(":")
		p.printSpace()
		p.printExpr(*item.Value, js_ast.LComma, 0)
	}

	if item.Initializer != nil {
		p.printSpace()
		p.print("=")
		p.printSpace()
		p.printExpr(*item.Initializer, js_ast.LComma, 0)
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
			if p.options.MangleSyntax {
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

func (p *printer) printRequireOrImportExpr(importRecordIndex uint32, leadingInteriorComments []js_ast.Comment, level js_ast.L, flags int) {
	record := &p.importRecords[importRecordIndex]

	if level >= js_ast.LNew || (flags&forbidCall) != 0 {
		p.print("(")
		defer p.print(")")
		level = js_ast.LLowest
	}

	if !record.SourceIndex.IsValid() {
		// External "require()"
		if record.Kind != ast.ImportDynamic {
			if record.WrapWithToModule {
				p.printSymbol(p.options.ToModuleRef)
				p.print("(")
				defer p.print(")")
			}
			p.printSpaceBeforeIdentifier()
			p.print("require(")
			p.addSourceMapping(record.Range.Loc)
			p.printQuotedUTF8(record.Path.Text, true /* allowBacktick */)
			p.print(")")
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

			// Wrap this with a call to "__toModule()" if this is a CommonJS file
			if record.WrapWithToModule {
				p.printSymbol(p.options.ToModuleRef)
				p.print("(")
				defer p.print(")")
			}

			p.printSpaceBeforeIdentifier()
			p.print("require(")
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

	// Wrap this with a call to "__toModule()" if this is a CommonJS file
	if record.WrapWithToModule {
		p.printSymbol(p.options.ToModuleRef)
		p.print("(")
		defer p.print(")")
	}

	// Call the wrapper
	p.printSymbol(meta.WrapperRef)
	p.print("()")

	// Return the namespace object if this is an ESM file
	if meta.ExportsRef != js_ast.InvalidRef {
		p.print(",")
		p.printSpace()
		p.printSymbol(meta.ExportsRef)
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
		if !p.options.RemoveWhitespace {
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

const (
	forbidCall = 1 << iota
	forbidIn
	hasNonOptionalChainParent
	exprResultIsUnused
)

func (p *printer) printUndefined(level js_ast.L) {
	if level >= js_ast.LPrefix {
		p.print("(void 0)")
	} else {
		p.printSpaceBeforeIdentifier()
		p.print("void 0")
		p.prevNumEnd = len(p.js)
	}
}

func (p *printer) printExpr(expr js_ast.Expr, level js_ast.L, flags int) {
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

	case *js_ast.ENew:
		wrap := level >= js_ast.LCall

		hasPureComment := !p.options.RemoveWhitespace && e.CanBeUnwrappedIfUnused
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
		if !p.options.RemoveWhitespace || len(e.Args) > 0 || level >= js_ast.LPostfix {
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
		wrap := level >= js_ast.LNew || (flags&forbidCall) != 0
		targetFlags := 0
		if e.OptionalChain == js_ast.OptionalChainNone {
			targetFlags = hasNonOptionalChainParent
		} else if (flags & hasNonOptionalChainParent) != 0 {
			wrap = true
		}

		hasPureComment := !p.options.RemoveWhitespace && e.CanBeUnwrappedIfUnused
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
			if p.options.RemoveWhitespace {
				p.print("(0,")
			} else {
				p.print("(0, ")
			}
			p.printExpr(e.Target, js_ast.LPostfix, 0)
			p.print(")")
		} else {
			p.printExpr(e.Target, js_ast.LPostfix, targetFlags)
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

	case *js_ast.ERequire:
		p.printRequireOrImportExpr(e.ImportRecordIndex, nil, level, flags)

	case *js_ast.ERequireResolve:
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

	case *js_ast.EImport:
		var leadingInteriorComments []js_ast.Comment
		if !p.options.RemoveWhitespace {
			leadingInteriorComments = e.LeadingInteriorComments
		}

		if e.ImportRecordIndex.IsValid() {
			p.printRequireOrImportExpr(e.ImportRecordIndex.GetIndex(), leadingInteriorComments, level, flags)
		} else {
			// Handle non-string expressions
			if !e.ImportRecordIndex.IsValid() {
				wrap := level >= js_ast.LNew || (flags&forbidCall) != 0
				if wrap {
					p.print("(")
				}
				p.printSpaceBeforeIdentifier()
				p.print("import(")
				if len(leadingInteriorComments) > 0 {
					p.printNewline()
					p.options.Indent++
					for _, comment := range e.LeadingInteriorComments {
						p.printIndentedComment(comment.Text)
					}
					p.printIndent()
				}
				p.printExpr(e.Expr, js_ast.LComma, 0)
				if len(leadingInteriorComments) > 0 {
					p.printNewline()
					p.options.Indent--
					p.printIndent()
				}
				p.print(")")
				if wrap {
					p.print(")")
				}
			}
		}

	case *js_ast.EDot:
		wrap := false
		if e.OptionalChain == js_ast.OptionalChainNone {
			flags |= hasNonOptionalChainParent
		} else {
			if (flags & hasNonOptionalChainParent) != 0 {
				wrap = true
				p.print("(")
			}
			flags &= ^hasNonOptionalChainParent
		}
		p.printExpr(e.Target, js_ast.LPostfix, flags)
		if e.OptionalChain == js_ast.OptionalChainStart {
			p.print("?")
		}
		if p.canPrintIdentifier(e.Name) {
			if e.OptionalChain != js_ast.OptionalChainStart && p.prevNumEnd == len(p.js) {
				// "1.toString" is a syntax error, so print "1 .toString" instead
				p.print(" ")
			}
			p.print(".")
			p.addSourceMapping(e.NameLoc)
			p.printIdentifier(e.Name)
		} else {
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
		} else {
			if (flags & hasNonOptionalChainParent) != 0 {
				wrap = true
				p.print("(")
			}
			flags &= ^hasNonOptionalChainParent
		}
		p.printExpr(e.Target, js_ast.LPostfix, flags)
		if e.OptionalChain == js_ast.OptionalChainStart {
			p.print("?.")
		}
		if private, ok := e.Index.Data.(*js_ast.EPrivateIdentifier); ok {
			if e.OptionalChain != js_ast.OptionalChainStart {
				p.print(".")
			}
			p.printSymbol(private.Ref)
		} else {
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
			if s, ok := e.Body.Stmts[0].Data.(*js_ast.SReturn); ok && s.Value != nil {
				p.arrowExprStart = len(p.js)
				p.printExpr(*s.Value, js_ast.LComma, 0)
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

	case *js_ast.EBoolean:
		if p.options.MangleSyntax {
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
		if e.PreferTemplate && !p.options.MangleSyntax && !p.options.UnsupportedFeatures.Has(compat.TemplateLiteral) {
			p.print("`")
			p.printQuotedUTF16(e.Value, '`')
			p.print("`")
			return
		}

		c := p.bestQuoteCharForString(e.Value, true /* allowBacktick */)
		p.print(c)
		p.printQuotedUTF16(e.Value, rune(c[0]))
		p.print(c)

	case *js_ast.ETemplate:
		// Convert no-substitution template literals into strings if it's smaller
		if p.options.MangleSyntax && e.Tag == nil && len(e.Parts) == 0 {
			c := p.bestQuoteCharForString(e.Head, true /* allowBacktick */)
			p.print(c)
			p.printQuotedUTF16(e.Head, rune(c[0]))
			p.print(c)
			return
		}

		if e.Tag != nil {
			// Optional chains are forbidden in template tags
			if js_ast.IsOptionalChain(*e.Tag) {
				p.print("(")
				p.printExpr(*e.Tag, js_ast.LLowest, 0)
				p.print(")")
			} else {
				p.printExpr(*e.Tag, js_ast.LPostfix, 0)
			}
		}
		p.print("`")
		if e.Tag != nil {
			p.print(e.HeadRaw)
		} else {
			p.printQuotedUTF16(e.Head, '`')
		}
		for _, part := range e.Parts {
			p.print("${")
			p.printExpr(part.Value, js_ast.LLowest, 0)
			p.print("}")
			if e.Tag != nil {
				p.print(part.TailRaw)
			} else {
				p.printQuotedUTF16(part.Tail, '`')
			}
		}
		p.print("`")

	case *js_ast.ERegExp:
		buffer := p.js
		n := len(buffer)

		// Avoid forming a single-line comment
		if n > 0 && buffer[n-1] == '/' {
			p.print(" ")
		}
		p.print(e.Value)

		// Need a space before the next identifier to avoid it turning into flags
		p.prevRegExpEnd = len(p.js)

	case *js_ast.EBigInt:
		p.printSpaceBeforeIdentifier()
		p.print(e.Value)
		p.print("n")

	case *js_ast.ENumber:
		value := e.Value
		absValue := math.Abs(value)

		if value != value {
			p.printSpaceBeforeIdentifier()
			p.print("NaN")
		} else if value == positiveInfinity {
			p.printSpaceBeforeIdentifier()
			p.print("Infinity")
		} else if value == negativeInfinity {
			if level >= js_ast.LPrefix {
				p.print("(-Infinity)")
			} else {
				p.printSpaceBeforeOperator(js_ast.UnOpNeg)
				p.print("-Infinity")
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

	case *js_ast.EIdentifier:
		name := p.renamer.NameForSymbol(e.Ref)
		wrap := len(p.js) == p.forOfInitStart && name == "let"

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
				if p.options.RemoveWhitespace {
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

		if e.Value != nil {
			if e.IsStar {
				p.print("*")
			}
			p.printSpace()
			p.printExpr(*e.Value, js_ast.LYield, 0)
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
			p.printExpr(e.Value, js_ast.LPostfix-1, 0)
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
			p.printExpr(e.Value, js_ast.LPrefix-1, 0)
		}

		if wrap {
			p.print(")")
		}

	case *js_ast.EBinary:
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
			} else if p.options.MangleSyntax {
				// When minifying, booleans are printed as "!0 and "!1"
				if _, ok := e.Left.Data.(*js_ast.EBoolean); ok {
					leftLevel = js_ast.LCall
				}
			}
		}

		// Special-case "#foo in bar"
		if private, ok := e.Left.Data.(*js_ast.EPrivateIdentifier); ok && e.Op == js_ast.BinOpIn {
			p.printSymbol(private.Ref)
		} else {
			p.printExpr(e.Left, leftLevel, flags&forbidIn)
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

		p.printExpr(e.Right, rightLevel, flags&forbidIn)

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

func (p *printer) printDeclStmt(isExport bool, keyword string, decls []js_ast.Decl) {
	p.printIndent()
	p.printSpaceBeforeIdentifier()
	if isExport {
		p.print("export ")
	}
	p.printDecls(keyword, decls, 0)
	p.printSemicolonAfterStatement()
}

func (p *printer) printForLoopInit(init js_ast.Stmt) {
	switch s := init.Data.(type) {
	case *js_ast.SExpr:
		p.printExpr(s.Value, js_ast.LLowest, forbidIn|exprResultIsUnused)
	case *js_ast.SLocal:
		switch s.Kind {
		case js_ast.LocalVar:
			p.printDecls("var", s.Decls, forbidIn)
		case js_ast.LocalLet:
			p.printDecls("let", s.Decls, forbidIn)
		case js_ast.LocalConst:
			p.printDecls("const", s.Decls, forbidIn)
		}
	default:
		panic("Internal error")
	}
}

func (p *printer) printDecls(keyword string, decls []js_ast.Decl, flags int) {
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
			p.printExpr(*decl.Value, js_ast.LComma, flags)
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
		p.printStmt(body)
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
		p.printStmt(stmt)
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
			if current.No == nil {
				return true
			}
			s = current.No.Data

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

	if yes, ok := s.Yes.Data.(*js_ast.SBlock); ok {
		p.printSpace()
		p.printBlock(s.Yes.Loc, yes.Stmts)

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

		if no, ok := s.No.Data.(*js_ast.SBlock); ok {
			p.printSpace()
			p.printBlock(s.No.Loc, no.Stmts)
			p.printNewline()
		} else if no, ok := s.No.Data.(*js_ast.SIf); ok {
			p.printIf(no)
		} else {
			p.printNewline()
			p.options.Indent++
			p.printStmt(*s.No)
			p.options.Indent--
		}
	}
}

func (p *printer) printIndentedComment(text string) {
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

func (p *printer) printStmt(stmt js_ast.Stmt) {
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

		if s.Value.Expr != nil {
			// Functions and classes must be wrapped to avoid confusion with their statement forms
			p.exportDefaultStart = len(p.js)

			p.printExpr(*s.Value.Expr, js_ast.LComma, 0)
			p.printSemicolonAfterStatement()
			return
		}

		switch s2 := s.Value.Stmt.Data.(type) {
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
		p.printQuotedUTF8(p.importRecords[s.ImportRecordIndex].Path.Text, false /* allowBacktick */)
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
				if s.IsSingleLine {
					p.printSpace()
				}
			}

			if !s.IsSingleLine {
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
				if s.IsSingleLine {
					p.printSpace()
				}
			}

			if !s.IsSingleLine {
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
		}

		p.print("}")
		p.printSpace()
		p.print("from")
		p.printSpace()
		p.printQuotedUTF8(p.importRecords[s.ImportRecordIndex].Path.Text, false /* allowBacktick */)
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
			p.printStmt(s.Body)
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
		p.printForLoopInit(s.Init)
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
		p.printForLoopInit(s.Init)
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
			if s.Catch.Binding != nil {
				p.printSpace()
				p.print("(")
				p.printBinding(*s.Catch.Binding)
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
			p.printExpr(*s.Test, js_ast.LLowest, 0)
		}
		p.print(";")
		p.printSpace()
		if s.Update != nil {
			p.printExpr(*s.Update, js_ast.LLowest, 0)
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

			if c.Value != nil {
				p.print("case")
				p.printSpace()
				p.printExpr(*c.Value, js_ast.LLogicalAnd, 0)
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
				p.printStmt(stmt)
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
					if s.IsSingleLine {
						p.printSpace()
					}
				}

				if !s.IsSingleLine {
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

		p.printQuotedUTF8(p.importRecords[s.ImportRecordIndex].Path.Text, false /* allowBacktick */)
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
		c := p.bestQuoteCharForString(s.Value, false /* allowBacktick */)
		p.printIndent()
		p.printSpaceBeforeIdentifier()
		p.print(c)
		p.printQuotedUTF16(s.Value, rune(c[0]))
		p.print(c)
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
		if s.Value != nil {
			p.printSpace()
			p.printExpr(*s.Value, js_ast.LLowest, 0)
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
		p.printIndent()
		p.stmtStart = len(p.js)
		p.printExpr(s.Value, js_ast.LLowest, exprResultIsUnused)
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

type Options struct {
	OutputFormat                 config.Format
	RemoveWhitespace             bool
	MangleSyntax                 bool
	ASCIIOnly                    bool
	LegalComments                config.LegalComments
	AddSourceMappings            bool
	Indent                       int
	ToModuleRef                  js_ast.Ref
	UnsupportedFeatures          compat.JSFeature
	RequireOrImportMetaForSource func(uint32) RequireOrImportMeta

	// If we're writing out a source map, this table of line start indices lets
	// us do binary search on to figure out what line a given AST node came from
	LineOffsetTables []LineOffsetTable

	// This will be present if the input file had a source map. In that case we
	// want to map all the way back to the original input file(s).
	InputSourceMap *sourcemap.SourceMap
}

type RequireOrImportMeta struct {
	// CommonJS files will return the "require_*" wrapper function and an invalid
	// exports object reference. Lazily-initialized ESM files will return the
	// "init_*" wrapper function and the exports object for that file.
	WrapperRef     js_ast.Ref
	ExportsRef     js_ast.Ref
	IsWrapperAsync bool
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

	ShouldIgnore bool
}

type PrintResult struct {
	JS []byte

	// This source map chunk just contains the VLQ-encoded offsets for the "JS"
	// field above. It's not a full source map. The bundler will be joining many
	// source map chunks together to form the final source map.
	SourceMapChunk SourceMapChunk

	ExtractedLegalComments map[string]bool
}

func Print(tree js_ast.AST, symbols js_ast.SymbolMap, r renamer.Renamer, options Options) PrintResult {
	p := &printer{
		symbols:            symbols,
		renamer:            r,
		importRecords:      tree.ImportRecords,
		options:            options,
		stmtStart:          -1,
		exportDefaultStart: -1,
		arrowExprStart:     -1,
		forOfInitStart:     -1,
		prevOpEnd:          -1,
		prevNumEnd:         -1,
		prevRegExpEnd:      -1,
		prevLoc:            logger.Loc{Start: -1},
		lineOffsetTables:   options.LineOffsetTables,

		// We automatically repeat the previous source mapping if we ever generate
		// a line that doesn't start with a mapping. This helps give files more
		// complete mapping coverage without gaps.
		//
		// However, we probably shouldn't do this if the input file has a nested
		// source map that we will be remapping through. We have no idea what state
		// that source map is in and it could be pretty scrambled.
		//
		// I've seen cases where blindly repeating the last mapping for subsequent
		// lines gives very strange and unhelpful results with source maps from
		// other tools.
		coverLinesWithoutMappings: options.InputSourceMap == nil,
	}

	// Add the top-level directive if present
	if tree.Directive != "" {
		p.printQuotedUTF8(tree.Directive, options.ASCIIOnly)
		p.print(";")
		p.printNewline()
	}

	for _, part := range tree.Parts {
		for _, stmt := range part.Stmts {
			p.printStmt(stmt)
			p.printSemicolonIfNeeded()
		}
	}

	p.updateGeneratedLineAndColumn()

	return PrintResult{
		JS:                     p.js,
		ExtractedLegalComments: p.extractedLegalComments,
		SourceMapChunk: SourceMapChunk{
			Buffer:               p.sourceMap,
			EndState:             p.prevState,
			FinalGeneratedColumn: p.generatedColumn,
			ShouldIgnore:         p.shouldIgnoreSourceMap(),
		},
	}
}
