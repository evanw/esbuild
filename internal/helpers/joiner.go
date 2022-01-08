package helpers

import (
	"bytes"
	"strings"
)

// This provides an efficient way to join lots of big string and byte slices
// together. It avoids the cost of repeatedly reallocating as the buffer grows
// by measuring exactly how big the buffer should be and then allocating once.
// This is a measurable speedup.
type Joiner struct {
	strings  []joinerString
	bytes    []joinerBytes
	length   uint32
	lastByte byte
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

func (j *Joiner) EnsureNewlineAtEnd() {
	if j.length > 0 && j.lastByte != '\n' {
		j.AddString("\n")
	}
}

func (j *Joiner) Done() []byte {
	if len(j.strings) == 0 && len(j.bytes) == 1 && j.bytes[0].offset == 0 {
		// No need to allocate if there was only a single byte array written
		return j.bytes[0].data
	}
	buffer := make([]byte, j.length)
	for _, item := range j.strings {
		copy(buffer[item.offset:], item.data)
	}
	for _, item := range j.bytes {
		copy(buffer[item.offset:], item.data)
	}
	return buffer
}

func (j *Joiner) Contains(s string, b []byte) bool {
	for _, item := range j.strings {
		if strings.Contains(item.data, s) {
			return true
		}
	}
	for _, item := range j.bytes {
		if bytes.Contains(item.data, b) {
			return true
		}
	}
	return false
}
