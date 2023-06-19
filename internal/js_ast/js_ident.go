package js_ast

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func IsIdentifier(text string) bool {
	if len(text) == 0 {
		return false
	}
	for i, codePoint := range text {
		if i == 0 {
			if !IsIdentifierStart(codePoint) {
				return false
			}
		} else {
			if !IsIdentifierContinue(codePoint) {
				return false
			}
		}
	}
	return true
}

func IsIdentifierES5AndESNext(text string) bool {
	if len(text) == 0 {
		return false
	}
	for i, codePoint := range text {
		if i == 0 {
			if !IsIdentifierStartES5AndESNext(codePoint) {
				return false
			}
		} else {
			if !IsIdentifierContinueES5AndESNext(codePoint) {
				return false
			}
		}
	}
	return true
}

func ForceValidIdentifier(prefix string, text string) string {
	sb := strings.Builder{}

	// Private identifiers must be prefixed by "#"
	if prefix != "" {
		sb.WriteString(prefix)
	}

	// Identifier start
	c, width := utf8.DecodeRuneInString(text)
	text = text[width:]
	if IsIdentifierStart(c) {
		sb.WriteRune(c)
	} else {
		sb.WriteRune('_')
	}

	// Identifier continue
	for text != "" {
		c, width := utf8.DecodeRuneInString(text)
		text = text[width:]
		if IsIdentifierContinue(c) {
			sb.WriteRune(c)
		} else {
			sb.WriteRune('_')
		}
	}

	return sb.String()
}

// This does "IsIdentifier(UTF16ToString(text))" without any allocations
func IsIdentifierUTF16(text []uint16) bool {
	n := len(text)
	if n == 0 {
		return false
	}
	for i := 0; i < n; i++ {
		isStart := i == 0
		r1 := rune(text[i])
		if r1 >= 0xD800 && r1 <= 0xDBFF && i+1 < n {
			if r2 := rune(text[i+1]); r2 >= 0xDC00 && r2 <= 0xDFFF {
				r1 = (r1 << 10) + r2 + (0x10000 - (0xD800 << 10) - 0xDC00)
				i++
			}
		}
		if isStart {
			if !IsIdentifierStart(r1) {
				return false
			}
		} else {
			if !IsIdentifierContinue(r1) {
				return false
			}
		}
	}
	return true
}

// This does "IsIdentifierES5AndESNext(UTF16ToString(text))" without any allocations
func IsIdentifierES5AndESNextUTF16(text []uint16) bool {
	n := len(text)
	if n == 0 {
		return false
	}
	for i := 0; i < n; i++ {
		isStart := i == 0
		r1 := rune(text[i])
		if r1 >= 0xD800 && r1 <= 0xDBFF && i+1 < n {
			if r2 := rune(text[i+1]); r2 >= 0xDC00 && r2 <= 0xDFFF {
				r1 = (r1 << 10) + r2 + (0x10000 - (0xD800 << 10) - 0xDC00)
				i++
			}
		}
		if isStart {
			if !IsIdentifierStartES5AndESNext(r1) {
				return false
			}
		} else {
			if !IsIdentifierContinueES5AndESNext(r1) {
				return false
			}
		}
	}
	return true
}

func IsIdentifierStart(codePoint rune) bool {
	switch codePoint {
	case '_', '$',
		'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm',
		'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
		'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M',
		'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z':
		return true
	}

	// All ASCII identifier start code points are listed above
	if codePoint < 0x7F {
		return false
	}

	return unicode.Is(idStartES5OrESNext, codePoint)
}

func IsIdentifierContinue(codePoint rune) bool {
	switch codePoint {
	case '_', '$', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
		'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm',
		'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
		'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M',
		'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z':
		return true
	}

	// All ASCII identifier start code points are listed above
	if codePoint < 0x7F {
		return false
	}

	// ZWNJ and ZWJ are allowed in identifiers
	if codePoint == 0x200C || codePoint == 0x200D {
		return true
	}

	return unicode.Is(idContinueES5OrESNext, codePoint)
}

func IsIdentifierStartES5AndESNext(codePoint rune) bool {
	switch codePoint {
	case '_', '$',
		'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm',
		'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
		'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M',
		'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z':
		return true
	}

	// All ASCII identifier start code points are listed above
	if codePoint < 0x7F {
		return false
	}

	return unicode.Is(idStartES5AndESNext, codePoint)
}

func IsIdentifierContinueES5AndESNext(codePoint rune) bool {
	switch codePoint {
	case '_', '$', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
		'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm',
		'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
		'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M',
		'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z':
		return true
	}

	// All ASCII identifier start code points are listed above
	if codePoint < 0x7F {
		return false
	}

	// ZWNJ and ZWJ are allowed in identifiers
	if codePoint == 0x200C || codePoint == 0x200D {
		return true
	}

	return unicode.Is(idContinueES5AndESNext, codePoint)
}

// See the "White Space Code Points" table in the ECMAScript standard
func IsWhitespace(codePoint rune) bool {
	switch codePoint {
	case
		'\u0009', // character tabulation
		'\u000B', // line tabulation
		'\u000C', // form feed
		'\u0020', // space
		'\u00A0', // no-break space

		// Unicode "Space_Separator" code points
		'\u1680', // ogham space mark
		'\u2000', // en quad
		'\u2001', // em quad
		'\u2002', // en space
		'\u2003', // em space
		'\u2004', // three-per-em space
		'\u2005', // four-per-em space
		'\u2006', // six-per-em space
		'\u2007', // figure space
		'\u2008', // punctuation space
		'\u2009', // thin space
		'\u200A', // hair space
		'\u202F', // narrow no-break space
		'\u205F', // medium mathematical space
		'\u3000', // ideographic space

		'\uFEFF': // zero width non-breaking space
		return true

	default:
		return false
	}
}
