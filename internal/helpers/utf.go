package helpers

import (
	"strings"
	"unicode/utf8"
)

func ContainsNonBMPCodePoint(text string) bool {
	for _, c := range text {
		if c > 0xFFFF {
			return true
		}
	}
	return false
}

// This does "ContainsNonBMPCodePoint(UTF16ToString(text))" without any allocations
func ContainsNonBMPCodePointUTF16(text []uint16) bool {
	if n := len(text); n > 0 {
		for i, c := range text[:n-1] {
			// Check for a high surrogate
			if c >= 0xD800 && c <= 0xDBFF {
				// Check for a low surrogate
				if c2 := text[i+1]; c2 >= 0xDC00 && c2 <= 0xDFFF {
					return true
				}
			}
		}
	}
	return false
}

func StringToUTF16(text string) []uint16 {
	decoded := make([]uint16, 0, len(text))
	for _, c := range text {
		if c <= 0xFFFF {
			decoded = append(decoded, uint16(c))
		} else {
			c -= 0x10000
			decoded = append(decoded, uint16(0xD800+((c>>10)&0x3FF)), uint16(0xDC00+(c&0x3FF)))
		}
	}
	return decoded
}

func UTF16ToString(text []uint16) string {
	var temp [utf8.UTFMax]byte
	b := strings.Builder{}
	n := len(text)
	for i := 0; i < n; i++ {
		r1 := rune(text[i])
		if r1 >= 0xD800 && r1 <= 0xDBFF && i+1 < n {
			if r2 := rune(text[i+1]); r2 >= 0xDC00 && r2 <= 0xDFFF {
				r1 = (r1-0xD800)<<10 | (r2 - 0xDC00) + 0x10000
				i++
			}
		}
		width := encodeWTF8Rune(temp[:], r1)
		b.Write(temp[:width])
	}
	return b.String()
}

func UTF16ToStringWithValidation(text []uint16) (string, uint16, bool) {
	var temp [utf8.UTFMax]byte
	b := strings.Builder{}
	n := len(text)
	for i := 0; i < n; i++ {
		r1 := rune(text[i])
		if r1 >= 0xD800 && r1 <= 0xDBFF {
			if i+1 < n {
				if r2 := rune(text[i+1]); r2 >= 0xDC00 && r2 <= 0xDFFF {
					r1 = (r1-0xD800)<<10 | (r2 - 0xDC00) + 0x10000
					i++
				} else {
					return "", uint16(r1), false
				}
			} else {
				return "", uint16(r1), false
			}
		} else if r1 >= 0xDC00 && r1 <= 0xDFFF {
			return "", uint16(r1), false
		}
		width := encodeWTF8Rune(temp[:], r1)
		b.Write(temp[:width])
	}
	return b.String(), 0, true
}

// Does "UTF16ToString(text) == str" without a temporary allocation
func UTF16EqualsString(text []uint16, str string) bool {
	if len(text) > len(str) {
		// Strings can't be equal if UTF-16 encoding is longer than UTF-8 encoding
		return false
	}
	var temp [utf8.UTFMax]byte
	n := len(text)
	j := 0
	for i := 0; i < n; i++ {
		r1 := rune(text[i])
		if r1 >= 0xD800 && r1 <= 0xDBFF && i+1 < n {
			if r2 := rune(text[i+1]); r2 >= 0xDC00 && r2 <= 0xDFFF {
				r1 = (r1-0xD800)<<10 | (r2 - 0xDC00) + 0x10000
				i++
			}
		}
		width := encodeWTF8Rune(temp[:], r1)
		if j+width > len(str) {
			return false
		}
		for k := 0; k < width; k++ {
			if temp[k] != str[j] {
				return false
			}
			j++
		}
	}
	return j == len(str)
}

func UTF16EqualsUTF16(a []uint16, b []uint16) bool {
	if len(a) == len(b) {
		for i, c := range a {
			if c != b[i] {
				return false
			}
		}
		return true
	}
	return false
}

// This is a clone of "utf8.EncodeRune" that has been modified to encode using
// WTF-8 instead. See https://simonsapin.github.io/wtf-8/ for more info.
func encodeWTF8Rune(p []byte, r rune) int {
	// Negative values are erroneous. Making it unsigned addresses the problem.
	switch i := uint32(r); {
	case i <= 0x7F:
		p[0] = byte(r)
		return 1
	case i <= 0x7FF:
		_ = p[1] // eliminate bounds checks
		p[0] = 0xC0 | byte(r>>6)
		p[1] = 0x80 | byte(r)&0x3F
		return 2
	case i > utf8.MaxRune:
		r = utf8.RuneError
		fallthrough
	case i <= 0xFFFF:
		_ = p[2] // eliminate bounds checks
		p[0] = 0xE0 | byte(r>>12)
		p[1] = 0x80 | byte(r>>6)&0x3F
		p[2] = 0x80 | byte(r)&0x3F
		return 3
	default:
		_ = p[3] // eliminate bounds checks
		p[0] = 0xF0 | byte(r>>18)
		p[1] = 0x80 | byte(r>>12)&0x3F
		p[2] = 0x80 | byte(r>>6)&0x3F
		p[3] = 0x80 | byte(r)&0x3F
		return 4
	}
}

// This is a clone of "utf8.DecodeRuneInString" that has been modified to
// decode using WTF-8 instead. See https://simonsapin.github.io/wtf-8/ for
// more info.
func DecodeWTF8Rune(s string) (rune, int) {
	n := len(s)
	if n < 1 {
		return utf8.RuneError, 0
	}

	s0 := s[0]
	if s0 < 0x80 {
		return rune(s0), 1
	}

	var sz int
	if (s0 & 0xE0) == 0xC0 {
		sz = 2
	} else if (s0 & 0xF0) == 0xE0 {
		sz = 3
	} else if (s0 & 0xF8) == 0xF0 {
		sz = 4
	} else {
		return utf8.RuneError, 1
	}

	if n < sz {
		return utf8.RuneError, 0
	}

	s1 := s[1]
	if (s1 & 0xC0) != 0x80 {
		return utf8.RuneError, 1
	}

	if sz == 2 {
		cp := rune(s0&0x1F)<<6 | rune(s1&0x3F)
		if cp < 0x80 {
			return utf8.RuneError, 1
		}
		return cp, 2
	}
	s2 := s[2]

	if (s2 & 0xC0) != 0x80 {
		return utf8.RuneError, 1
	}

	if sz == 3 {
		cp := rune(s0&0x0F)<<12 | rune(s1&0x3F)<<6 | rune(s2&0x3F)
		if cp < 0x0800 {
			return utf8.RuneError, 1
		}
		return cp, 3
	}
	s3 := s[3]

	if (s3 & 0xC0) != 0x80 {
		return utf8.RuneError, 1
	}

	cp := rune(s0&0x07)<<18 | rune(s1&0x3F)<<12 | rune(s2&0x3F)<<6 | rune(s3&0x3F)
	if cp < 0x010000 || cp > 0x10FFFF {
		return utf8.RuneError, 1
	}
	return cp, 4
}
