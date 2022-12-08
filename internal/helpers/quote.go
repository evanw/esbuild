package helpers

import "unicode/utf8"

const hexChars = "0123456789ABCDEF"
const firstASCII = 0x20
const lastASCII = 0x7E
const firstHighSurrogate = 0xD800
const firstLowSurrogate = 0xDC00
const lastLowSurrogate = 0xDFFF

func canPrintWithoutEscape(c rune, asciiOnly bool) bool {
	if c <= lastASCII {
		return c >= firstASCII && c != '\\' && c != '"'
	} else {
		return !asciiOnly && c != '\uFEFF' && (c < firstHighSurrogate || c > lastLowSurrogate)
	}
}

func QuoteSingle(text string, asciiOnly bool) []byte {
	return internalQuote(text, asciiOnly, '\'')
}

func QuoteForJSON(text string, asciiOnly bool) []byte {
	return internalQuote(text, asciiOnly, '"')
}

func internalQuote(text string, asciiOnly bool, quoteChar byte) []byte {
	// Estimate the required length
	lenEstimate := 2
	for _, c := range text {
		if canPrintWithoutEscape(c, asciiOnly) {
			lenEstimate += utf8.RuneLen(c)
		} else {
			switch c {
			case '\b', '\f', '\n', '\r', '\t', '\\':
				lenEstimate += 2
			case '"':
				if quoteChar == '"' {
					lenEstimate += 2
				}
			case '\'':
				if quoteChar == '\'' {
					lenEstimate += 2
				}
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
	bytes = append(bytes, quoteChar)

	for i < n {
		c, width := DecodeWTF8Rune(text[i:])

		// Fast path: a run of characters that don't need escaping
		if canPrintWithoutEscape(c, asciiOnly) {
			start := i
			i += width
			for i < n {
				c, width = DecodeWTF8Rune(text[i:])
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
			if quoteChar == '"' {
				bytes = append(bytes, "\\\""...)
			} else {
				bytes = append(bytes, '"')
			}
			i++

		case '\'':
			if quoteChar == '\'' {
				bytes = append(bytes, "\\'"...)
			} else {
				bytes = append(bytes, '\'')
			}
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

	return append(bytes, quoteChar)
}
