package helpers

import (
	"strings"
)

func EscapeClosingTag(text string, slashTag string) string {
	if slashTag == "" {
		return text
	}
	i := strings.Index(text, "</")
	if i < 0 {
		return text
	}
	var b strings.Builder
	for {
		b.WriteString(text[:i+1])
		text = text[i+1:]
		if len(text) >= len(slashTag) && strings.EqualFold(text[:len(slashTag)], slashTag) {
			b.WriteByte('\\')
		}
		i = strings.Index(text, "</")
		if i < 0 {
			break
		}
	}
	b.WriteString(text)
	return b.String()
}
