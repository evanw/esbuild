package helpers

import "bytes"

type BitSet struct {
	entries []byte
}

func NewBitSet(bitCount uint) BitSet {
	return BitSet{make([]byte, (bitCount+7)/8)}
}

func (bs BitSet) HasBit(bit uint) bool {
	return (bs.entries[bit/8] & (1 << (bit & 7))) != 0
}

func (bs BitSet) SetBit(bit uint) {
	bs.entries[bit/8] |= 1 << (bit & 7)
}

func (bs BitSet) Equals(other BitSet) bool {
	return bytes.Equal(bs.entries, other.entries)
}

func (bs BitSet) String() string {
	return string(bs.entries)
}
