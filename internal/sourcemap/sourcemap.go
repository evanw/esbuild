package sourcemap

import (
	"bytes"
	"unicode/utf8"

	"github.com/evanw/esbuild/internal/helpers"
)

type Mapping struct {
	GeneratedLine   int32 // 0-based
	GeneratedColumn int32 // 0-based count of UTF-16 code units

	SourceIndex    int32 // 0-based
	OriginalLine   int32 // 0-based
	OriginalColumn int32 // 0-based count of UTF-16 code units
}

type SourceMap struct {
	Sources        []string
	SourcesContent []SourceContent
	Mappings       []Mapping
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
//   Continuation
//   |    Sign
//   |    |
//   V    V
//   101011
//
func EncodeVLQ(value int) []byte {
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

func DecodeVLQUTF16(encoded []uint16) (int, int, bool) {
	n := len(encoded)
	if n == 0 {
		return 0, 0, false
	}

	// Scan over the input
	current := 0
	shift := 0
	vlq := 0
	for {
		if current >= n {
			return 0, 0, false
		}
		index := bytes.IndexByte(base64, byte(encoded[current]))
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

		// Skip over the original position information
		_, current = DecodeVLQ(pieces.Mappings, current) // The original source
		_, current = DecodeVLQ(pieces.Mappings, current) // The original line
		_, current = DecodeVLQ(pieces.Mappings, current) // The original column

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
		j.AddBytes(EncodeVLQ(generatedColumnDelta + shiftColumnDelta - prevShiftColumnDelta))
		prevShiftColumnDelta = shiftColumnDelta

		// Finally, start the next run after the end of this generated column offset
		startOfRun = potentialStartOfRun
	}

	// Finish the source map
	j.AddBytes(pieces.Mappings[startOfRun:])
	j.AddBytes(pieces.Suffix)
	return j.Done()
}
