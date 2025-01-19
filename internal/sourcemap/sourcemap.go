package sourcemap

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/logger"
)

type Mapping struct {
	GeneratedLine   int32 // 0-based
	GeneratedColumn int32 // 0-based count of UTF-16 code units

	SourceIndex    int32       // 0-based
	OriginalLine   int32       // 0-based
	OriginalColumn int32       // 0-based count of UTF-16 code units
	OriginalName   ast.Index32 // 0-based, optional
}

type SourceMap struct {
	Sources        []string
	SourcesContent []SourceContent
	Mappings       []Mapping
	Names          []string
}

type SourceContent struct {
	// This stores both the unquoted and the quoted values. We try to use the
	// already-quoted value if possible so we don't need to re-quote it
	// unnecessarily for maximum performance.
	Quoted string

	// But sometimes we need to re-quote the value, such as when it contains
	// non-ASCII characters and we are in ASCII-only mode. In that case we quote
	// this parsed UTF-16 value.
	Value []uint16
}

func (sm *SourceMap) Find(line int32, column int32) *Mapping {
	mappings := sm.Mappings

	// Binary search
	count := len(mappings)
	index := 0
	for count > 0 {
		step := count / 2
		i := index + step
		mapping := mappings[i]
		if mapping.GeneratedLine < line || (mapping.GeneratedLine == line && mapping.GeneratedColumn <= column) {
			index = i + 1
			count -= step + 1
		} else {
			count = step
		}
	}

	// Handle search failure
	if index > 0 {
		mapping := &mappings[index-1]

		// Match the behavior of the popular "source-map" library from Mozilla
		if mapping.GeneratedLine == line {
			return mapping
		}
	}
	return nil
}

var base64 = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/")

// A single base 64 digit can contain 6 bits of data. For the base 64 variable
// length quantities we use in the source map spec, the first bit is the sign,
// the next four bits are the actual value, and the 6th bit is the continuation
// bit. The continuation bit tells us whether there are more digits in this
// value following this digit.
//
//	Continuation
//	|    Sign
//	|    |
//	V    V
//	101011
func encodeVLQ(encoded []byte, value int) []byte {
	var vlq int
	if value < 0 {
		vlq = ((-value) << 1) | 1
	} else {
		vlq = value << 1
	}

	// Handle the common case
	if (vlq >> 5) == 0 {
		digit := vlq & 31
		encoded = append(encoded, base64[digit])
		return encoded
	}

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

func DecodeVLQ(encoded []byte, start int) (int, int) {
	shift := 0
	vlq := 0

	// Scan over the input
	for {
		index := bytes.IndexByte(base64, encoded[start])
		if index < 0 {
			break
		}

		// Decode a single byte
		vlq |= (index & 31) << shift
		start++
		shift += 5

		// Stop if there's no continuation bit
		if (index & 32) == 0 {
			break
		}
	}

	// Recover the value
	value := vlq >> 1
	if (vlq & 1) != 0 {
		value = -value
	}
	return value, start
}

func DecodeVLQUTF16(encoded []uint16) (int32, int, bool) {
	n := len(encoded)
	if n == 0 {
		return 0, 0, false
	}

	// Scan over the input
	current := 0
	shift := 0
	var vlq int32
	for {
		if current >= n {
			return 0, 0, false
		}
		index := int32(bytes.IndexByte(base64, byte(encoded[current])))
		if index < 0 {
			return 0, 0, false
		}

		// Decode a single byte
		vlq |= (index & 31) << shift
		current++
		shift += 5

		// Stop if there's no continuation bit
		if (index & 32) == 0 {
			break
		}
	}

	// Recover the value
	var value = vlq >> 1
	if (vlq & 1) != 0 {
		value = -value
	}
	return value, current, true
}

type LineColumnOffset struct {
	Lines   int
	Columns int
}

func (a LineColumnOffset) ComesBefore(b LineColumnOffset) bool {
	return a.Lines < b.Lines || (a.Lines == b.Lines && a.Columns < b.Columns)
}

func (a *LineColumnOffset) Add(b LineColumnOffset) {
	if b.Lines == 0 {
		a.Columns += b.Columns
	} else {
		a.Lines += b.Lines
		a.Columns = b.Columns
	}
}

func (offset *LineColumnOffset) AdvanceBytes(bytes []byte) {
	columns := offset.Columns
	for len(bytes) > 0 {
		c, width := utf8.DecodeRune(bytes)
		bytes = bytes[width:]
		switch c {
		case '\r', '\n', '\u2028', '\u2029':
			// Handle Windows-specific "\r\n" newlines
			if c == '\r' && len(bytes) > 0 && bytes[0] == '\n' {
				columns++
				continue
			}

			offset.Lines++
			columns = 0

		default:
			// Mozilla's "source-map" library counts columns using UTF-16 code units
			if c <= 0xFFFF {
				columns++
			} else {
				columns += 2
			}
		}
	}
	offset.Columns = columns
}

func (offset *LineColumnOffset) AdvanceString(text string) {
	columns := offset.Columns
	for i, c := range text {
		switch c {
		case '\r', '\n', '\u2028', '\u2029':
			// Handle Windows-specific "\r\n" newlines
			if c == '\r' && i+1 < len(text) && text[i+1] == '\n' {
				columns++
				continue
			}

			offset.Lines++
			columns = 0

		default:
			// Mozilla's "source-map" library counts columns using UTF-16 code units
			if c <= 0xFFFF {
				columns++
			} else {
				columns += 2
			}
		}
	}
	offset.Columns = columns
}

type SourceMapPieces struct {
	Prefix   []byte
	Mappings []byte
	Suffix   []byte
}

func (pieces SourceMapPieces) HasContent() bool {
	return len(pieces.Prefix)+len(pieces.Mappings)+len(pieces.Suffix) > 0
}

type SourceMapShift struct {
	Before LineColumnOffset
	After  LineColumnOffset
}

func (pieces SourceMapPieces) Finalize(shifts []SourceMapShift) []byte {
	// An optimized path for when there are no shifts
	if len(shifts) == 1 {
		bytes := pieces.Prefix
		minCap := len(bytes) + len(pieces.Mappings) + len(pieces.Suffix)
		if cap(bytes) < minCap {
			bytes = append(make([]byte, 0, minCap), bytes...)
		}
		bytes = append(bytes, pieces.Mappings...)
		bytes = append(bytes, pieces.Suffix...)
		return bytes
	}

	startOfRun := 0
	current := 0
	generated := LineColumnOffset{}
	prevShiftColumnDelta := 0
	j := helpers.Joiner{}

	// Start the source map
	j.AddBytes(pieces.Prefix)

	// This assumes that a) all mappings are valid and b) all mappings are ordered
	// by increasing generated position. This should be the case for all mappings
	// generated by esbuild, which should be the only mappings we process here.
	for current < len(pieces.Mappings) {
		// Handle a line break
		if pieces.Mappings[current] == ';' {
			generated.Lines++
			generated.Columns = 0
			prevShiftColumnDelta = 0
			current++
			continue
		}

		potentialEndOfRun := current

		// Read the generated column
		generatedColumnDelta, next := DecodeVLQ(pieces.Mappings, current)
		generated.Columns += generatedColumnDelta
		current = next

		potentialStartOfRun := current

		// Skip over the original position information if present
		if current < len(pieces.Mappings) {
			_, current = DecodeVLQ(pieces.Mappings, current) // The original source
			_, current = DecodeVLQ(pieces.Mappings, current) // The original line
			_, current = DecodeVLQ(pieces.Mappings, current) // The original column

			// Skip over the original name if present
			if current < len(pieces.Mappings) {
				_, current = DecodeVLQ(pieces.Mappings, current)
			}
		}

		// Skip a trailing comma
		if current < len(pieces.Mappings) && pieces.Mappings[current] == ',' {
			current++
		}

		// Detect crossing shift boundaries
		didCrossBoundary := false
		for len(shifts) > 1 && shifts[1].Before.ComesBefore(generated) {
			shifts = shifts[1:]
			didCrossBoundary = true
		}
		if !didCrossBoundary {
			continue
		}

		// This shift isn't relevant if the next mapping after this shift is on a
		// following line. In that case, don't split and keep scanning instead.
		shift := shifts[0]
		if shift.After.Lines != generated.Lines {
			continue
		}

		// Add all previous mappings in a single run for efficiency. Since source
		// mappings are relative, no data needs to be modified inside this run.
		j.AddBytes(pieces.Mappings[startOfRun:potentialEndOfRun])

		// Then modify the first mapping across the shift boundary with the updated
		// generated column value. It's simplest to only support column shifts. This
		// is reasonable because import paths should not contain newlines.
		if shift.Before.Lines != shift.After.Lines {
			panic("Unexpected line change when shifting source maps")
		}
		shiftColumnDelta := shift.After.Columns - shift.Before.Columns
		j.AddBytes(encodeVLQ(nil, generatedColumnDelta+shiftColumnDelta-prevShiftColumnDelta))
		prevShiftColumnDelta = shiftColumnDelta

		// Finally, start the next run after the end of this generated column offset
		startOfRun = potentialStartOfRun
	}

	// Finish the source map
	j.AddBytes(pieces.Mappings[startOfRun:])
	j.AddBytes(pieces.Suffix)
	return j.Done()
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
	OriginalName    int
	HasOriginalName bool
}

// Source map chunks are computed in parallel for speed. Each chunk is relative
// to the zero state instead of being relative to the end state of the previous
// chunk, since it's impossible to know the end state of the previous chunk in
// a parallel computation.
//
// After all chunks are computed, they are joined together in a second pass.
// This rewrites the first mapping in each chunk to be relative to the end
// state of the previous chunk.
func AppendSourceMapChunk(j *helpers.Joiner, prevEndState SourceMapState, startState SourceMapState, buffer MappingsBuffer) {
	// Handle line breaks in between this mapping and the previous one
	if startState.GeneratedLine != 0 {
		j.AddBytes(bytes.Repeat([]byte{';'}, startState.GeneratedLine))
		prevEndState.GeneratedColumn = 0
	}

	// Skip past any leading semicolons, which indicate line breaks
	semicolons := 0
	for buffer.Data[semicolons] == ';' {
		semicolons++
	}
	if semicolons > 0 {
		j.AddBytes(buffer.Data[:semicolons])
		prevEndState.GeneratedColumn = 0
		startState.GeneratedColumn = 0
	}

	// Strip off the first mapping from the buffer. The first mapping should be
	// for the start of the original file (the printer always generates one for
	// the start of the file).
	//
	// Note that we do not want to strip off the original name, even though it
	// could be a part of the first mapping. This will be handled using a special
	// case below instead. Original names are optional and are often omitted, so
	// we handle it uniformly by saving an index to the first original name,
	// which may or may not be a part of the first mapping.
	var sourceIndex int
	var originalLine int
	var originalColumn int
	omitSource := false
	generatedColumn, i := DecodeVLQ(buffer.Data, semicolons)
	if i == len(buffer.Data) || strings.IndexByte(",;", buffer.Data[i]) != -1 {
		omitSource = true
	} else {
		sourceIndex, i = DecodeVLQ(buffer.Data, i)
		originalLine, i = DecodeVLQ(buffer.Data, i)
		originalColumn, i = DecodeVLQ(buffer.Data, i)
	}

	// Rewrite the first mapping to be relative to the end state of the previous
	// chunk. We now know what the end state is because we're in the second pass
	// where all chunks have already been generated.
	startState.GeneratedColumn += generatedColumn
	startState.SourceIndex += sourceIndex
	startState.OriginalLine += originalLine
	startState.OriginalColumn += originalColumn
	prevEndState.HasOriginalName = false // This is handled separately below
	rewritten, _ := appendMappingToBuffer(nil, j.LastByte(), prevEndState, startState, omitSource)
	j.AddBytes(rewritten)

	// Next, if there's an original name, we need to rewrite that as well to be
	// relative to that of the previous chunk.
	if buffer.FirstNameOffset.IsValid() {
		before := int(buffer.FirstNameOffset.GetIndex())
		originalName, after := DecodeVLQ(buffer.Data, before)
		originalName += startState.OriginalName - prevEndState.OriginalName
		j.AddBytes(buffer.Data[i:before])
		j.AddBytes(encodeVLQ(nil, originalName))
		j.AddBytes(buffer.Data[after:])
		return
	}

	// Otherwise, just append everything after that without modification
	j.AddBytes(buffer.Data[i:])
}

func appendMappingToBuffer(
	buffer []byte, lastByte byte, prevState SourceMapState, currentState SourceMapState, omitSource bool,
) ([]byte, ast.Index32) {
	// Put commas in between mappings
	if lastByte != 0 && lastByte != ';' && lastByte != '"' {
		buffer = append(buffer, ',')
	}

	// Record the mapping (note that the generated line is recorded using ';' elsewhere)
	buffer = encodeVLQ(buffer, currentState.GeneratedColumn-prevState.GeneratedColumn)
	if !omitSource {
		buffer = encodeVLQ(buffer, currentState.SourceIndex-prevState.SourceIndex)
		buffer = encodeVLQ(buffer, currentState.OriginalLine-prevState.OriginalLine)
		buffer = encodeVLQ(buffer, currentState.OriginalColumn-prevState.OriginalColumn)
	}

	// Record the optional original name
	var nameOffset ast.Index32
	if currentState.HasOriginalName {
		nameOffset = ast.MakeIndex32(uint32(len(buffer)))
		buffer = encodeVLQ(buffer, currentState.OriginalName-prevState.OriginalName)
	}

	return buffer, nameOffset
}

type LineOffsetTable struct {
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
	columnsForNonASCII        []int32
	byteOffsetToFirstNonASCII int32

	byteOffsetToStartOfLine int32
}

func GenerateLineOffsetTables(contents string, approximateLineCount int32) []LineOffsetTable {
	var ColumnsForNonASCII []int32
	ByteOffsetToFirstNonASCII := int32(0)
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
		if c > 0x7F && ColumnsForNonASCII == nil {
			columnByteOffset = i - lineByteOffset
			ByteOffsetToFirstNonASCII = int32(columnByteOffset)
			ColumnsForNonASCII = []int32{}
		}

		// Update the per-byte column offsets
		if ColumnsForNonASCII != nil {
			for lineBytesSoFar := i - lineByteOffset; columnByteOffset <= lineBytesSoFar; columnByteOffset++ {
				ColumnsForNonASCII = append(ColumnsForNonASCII, column)
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
				byteOffsetToFirstNonASCII: ByteOffsetToFirstNonASCII,
				columnsForNonASCII:        ColumnsForNonASCII,
			})
			columnByteOffset = 0
			ByteOffsetToFirstNonASCII = 0
			ColumnsForNonASCII = nil
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
	if ColumnsForNonASCII != nil {
		for lineBytesSoFar := len(contents) - lineByteOffset; columnByteOffset <= lineBytesSoFar; columnByteOffset++ {
			ColumnsForNonASCII = append(ColumnsForNonASCII, column)
		}
	}

	lineOffsetTables = append(lineOffsetTables, LineOffsetTable{
		byteOffsetToStartOfLine:   int32(lineByteOffset),
		byteOffsetToFirstNonASCII: ByteOffsetToFirstNonASCII,
		columnsForNonASCII:        ColumnsForNonASCII,
	})
	return lineOffsetTables
}

type MappingsBuffer struct {
	Data            []byte
	FirstNameOffset ast.Index32
}

type Chunk struct {
	Buffer      MappingsBuffer
	QuotedNames [][]byte

	// This end state will be used to rewrite the start of the following source
	// map chunk so that the delta-encoded VLQ numbers are preserved.
	EndState SourceMapState

	// There probably isn't a source mapping at the end of the file (nor should
	// there be) but if we're appending another source map chunk after this one,
	// we'll need to know how many characters were in the last line we generated.
	FinalGeneratedColumn int

	ShouldIgnore bool
}

type ChunkBuilder struct {
	inputSourceMap      *SourceMap
	sourceMap           []byte
	quotedNames         [][]byte
	namesMap            map[string]uint32
	lineOffsetTables    []LineOffsetTable
	prevOriginalName    string
	prevState           SourceMapState
	lastGeneratedUpdate int
	generatedColumn     int
	prevGeneratedLen    int
	prevOriginalLoc     logger.Loc
	firstNameOffset     ast.Index32
	hasPrevState        bool
	asciiOnly           bool

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

func MakeChunkBuilder(inputSourceMap *SourceMap, lineOffsetTables []LineOffsetTable, asciiOnly bool) ChunkBuilder {
	return ChunkBuilder{
		inputSourceMap:   inputSourceMap,
		prevOriginalLoc:  logger.Loc{Start: -1},
		lineOffsetTables: lineOffsetTables,
		asciiOnly:        asciiOnly,
		namesMap:         make(map[string]uint32),

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
		coverLinesWithoutMappings: inputSourceMap == nil,
	}
}

func (b *ChunkBuilder) AddSourceMapping(originalLoc logger.Loc, originalName string, output []byte) {
	// Avoid generating duplicate mappings
	if originalLoc == b.prevOriginalLoc && (b.prevGeneratedLen == len(output) || b.prevOriginalName == originalName) {
		return
	}

	b.prevOriginalLoc = originalLoc
	b.prevGeneratedLen = len(output)
	b.prevOriginalName = originalName

	// Binary search to find the line
	lineOffsetTables := b.lineOffsetTables
	count := len(lineOffsetTables)
	originalLine := 0
	for count > 0 {
		step := count / 2
		i := originalLine + step
		if lineOffsetTables[i].byteOffsetToStartOfLine <= originalLoc.Start {
			originalLine = i + 1
			count = count - step - 1
		} else {
			count = step
		}
	}
	originalLine--

	// Use the line to compute the column
	line := &lineOffsetTables[originalLine]
	originalColumn := int(originalLoc.Start - line.byteOffsetToStartOfLine)
	if line.columnsForNonASCII != nil && originalColumn >= int(line.byteOffsetToFirstNonASCII) {
		originalColumn = int(line.columnsForNonASCII[originalColumn-int(line.byteOffsetToFirstNonASCII)])
	}

	b.updateGeneratedLineAndColumn(output)

	// If this line doesn't start with a mapping and we're about to add a mapping
	// that's not at the start, insert a mapping first so the line starts with one.
	if b.coverLinesWithoutMappings && !b.lineStartsWithMapping && b.generatedColumn > 0 && b.hasPrevState {
		b.appendMappingWithoutRemapping(SourceMapState{
			GeneratedLine:   b.prevState.GeneratedLine,
			GeneratedColumn: 0,
			SourceIndex:     b.prevState.SourceIndex,
			OriginalLine:    b.prevState.OriginalLine,
			OriginalColumn:  b.prevState.OriginalColumn,
		})
	}

	b.appendMapping(originalName, SourceMapState{
		GeneratedLine:   b.prevState.GeneratedLine,
		GeneratedColumn: b.generatedColumn,
		OriginalLine:    originalLine,
		OriginalColumn:  originalColumn,
	})

	// This line now has a mapping on it, so don't insert another one
	b.lineStartsWithMapping = true
}

func (b *ChunkBuilder) GenerateChunk(output []byte) Chunk {
	b.updateGeneratedLineAndColumn(output)
	shouldIgnore := true
	for _, c := range b.sourceMap {
		if c != ';' {
			shouldIgnore = false
			break
		}
	}
	return Chunk{
		Buffer: MappingsBuffer{
			Data:            b.sourceMap,
			FirstNameOffset: b.firstNameOffset,
		},
		QuotedNames:          b.quotedNames,
		EndState:             b.prevState,
		FinalGeneratedColumn: b.generatedColumn,
		ShouldIgnore:         shouldIgnore,
	}
}

// Scan over the printed text since the last source mapping and update the
// generated line and column numbers
func (b *ChunkBuilder) updateGeneratedLineAndColumn(output []byte) {
	for i, c := range string(output[b.lastGeneratedUpdate:]) {
		switch c {
		case '\r', '\n', '\u2028', '\u2029':
			// Handle Windows-specific "\r\n" newlines
			if c == '\r' {
				newlineCheck := b.lastGeneratedUpdate + i + 1
				if newlineCheck < len(output) && output[newlineCheck] == '\n' {
					continue
				}
			}

			// If we're about to move to the next line and the previous line didn't have
			// any mappings, add a mapping at the start of the previous line.
			if b.coverLinesWithoutMappings && !b.lineStartsWithMapping && b.hasPrevState {
				b.appendMappingWithoutRemapping(SourceMapState{
					GeneratedLine:   b.prevState.GeneratedLine,
					GeneratedColumn: 0,
					SourceIndex:     b.prevState.SourceIndex,
					OriginalLine:    b.prevState.OriginalLine,
					OriginalColumn:  b.prevState.OriginalColumn,
				})
			}

			b.prevState.GeneratedLine++
			b.prevState.GeneratedColumn = 0
			b.generatedColumn = 0
			b.sourceMap = append(b.sourceMap, ';')

			// This new line doesn't have a mapping yet
			b.lineStartsWithMapping = false

		default:
			// Mozilla's "source-map" library counts columns using UTF-16 code units
			if c <= 0xFFFF {
				b.generatedColumn++
			} else {
				b.generatedColumn += 2
			}
		}
	}

	b.lastGeneratedUpdate = len(output)
}

func (b *ChunkBuilder) appendMapping(originalName string, currentState SourceMapState) {
	// If the input file had a source map, map all the way back to the original
	if b.inputSourceMap != nil {
		mapping := b.inputSourceMap.Find(
			int32(currentState.OriginalLine),
			int32(currentState.OriginalColumn))

		// Some locations won't have a mapping
		if mapping == nil {
			return
		}

		currentState.SourceIndex = int(mapping.SourceIndex)
		currentState.OriginalLine = int(mapping.OriginalLine)
		currentState.OriginalColumn = int(mapping.OriginalColumn)

		// Map all the way back to the original name if present. Otherwise, keep
		// the original name from esbuild, which corresponds to the name in the
		// intermediate source code. This is important for tools that only emit
		// a name mapping when the name is different than the original name.
		if mapping.OriginalName.IsValid() {
			originalName = b.inputSourceMap.Names[mapping.OriginalName.GetIndex()]
		}
	}

	// Optionally reference the original name
	if originalName != "" {
		i, ok := b.namesMap[originalName]
		if !ok {
			i = uint32(len(b.quotedNames))
			b.quotedNames = append(b.quotedNames, helpers.QuoteForJSON(originalName, b.asciiOnly))
			b.namesMap[originalName] = i
		}
		currentState.OriginalName = int(i)
		currentState.HasOriginalName = true
	}

	b.appendMappingWithoutRemapping(currentState)
}

func (b *ChunkBuilder) appendMappingWithoutRemapping(currentState SourceMapState) {
	var lastByte byte
	if len(b.sourceMap) != 0 {
		lastByte = b.sourceMap[len(b.sourceMap)-1]
	}

	var nameOffset ast.Index32
	b.sourceMap, nameOffset = appendMappingToBuffer(b.sourceMap, lastByte, b.prevState, currentState, false)
	prevOriginalName := b.prevState.OriginalName
	b.prevState = currentState
	if !currentState.HasOriginalName {
		// Revert the original name change if it's invalid
		b.prevState.OriginalName = prevOriginalName
	} else if !b.firstNameOffset.IsValid() {
		// Keep track of the first name offset so we can jump right to it later
		b.firstNameOffset = nameOffset
	}
	b.hasPrevState = true
}
