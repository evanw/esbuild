package helpers

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Returns the shorter of either a base64-encoded or percent-escaped data URL
func EncodeStringAsShortestDataURL(mimeType string, text string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	url := fmt.Sprintf("data:%s;base64,%s", mimeType, encoded)
	if percentURL, ok := EncodeStringAsPercentEscapedDataURL(mimeType, text); ok && len(percentURL) < len(url) {
		return percentURL
	}
	return url
}

// See "scripts/dataurl-escapes.html" for how this was derived
func EncodeStringAsPercentEscapedDataURL(mimeType string, text string) (string, bool) {
	hex := "0123456789ABCDEF"
	sb := strings.Builder{}
	n := len(text)
	i := 0
	runStart := 0
	sb.WriteString("data:")
	sb.WriteString(mimeType)
	sb.WriteByte(',')

	// Scan for trailing characters that need to be escaped
	trailingStart := n
	for trailingStart > 0 {
		if c := text[trailingStart-1]; c > 0x20 || c == '\t' || c == '\n' || c == '\r' {
			break
		}
		trailingStart--
	}

	for i < n {
		c, width := utf8.DecodeRuneInString(text[i:])

		// We can't encode invalid UTF-8 data
		if c == utf8.RuneError && width == 1 {
			return "", false
		}

		// Escape this character if needed
		if c == '\t' || c == '\n' || c == '\r' || c == '#' || i >= trailingStart ||
			(c == '%' && i+2 < n && isHex(text[i+1]) && isHex(text[i+2])) {
			if runStart < i {
				sb.WriteString(text[runStart:i])
			}
			sb.WriteByte('%')
			sb.WriteByte(hex[c>>4])
			sb.WriteByte(hex[c&15])
			runStart = i + width
		}

		i += width
	}

	if runStart < n {
		sb.WriteString(text[runStart:])
	}

	return sb.String(), true
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}
