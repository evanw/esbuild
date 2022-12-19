package helpers

import (
	"strings"
	"unicode/utf8"
)

func RemoveMultiLineCommentIndent(prefix string, text string) string {
	// Figure out the initial indent
	indent := 0
seekBackwardToNewline:
	for len(prefix) > 0 {
		c, size := utf8.DecodeLastRuneInString(prefix)
		switch c {
		case '\r', '\n', '\u2028', '\u2029':
			break seekBackwardToNewline
		}
		prefix = prefix[:len(prefix)-size]
		indent++
	}

	// Split the comment into lines
	var lines []string
	start := 0
	for i, c := range text {
		switch c {
		case '\r', '\n':
			// Don't double-append for Windows style "\r\n" newlines
			if start <= i {
				lines = append(lines, text[start:i])
			}

			start = i + 1

			// Ignore the second part of Windows style "\r\n" newlines
			if c == '\r' && start < len(text) && text[start] == '\n' {
				start++
			}

		case '\u2028', '\u2029':
			lines = append(lines, text[start:i])
			start = i + 3
		}
	}
	lines = append(lines, text[start:])

	// Find the minimum indent over all lines after the first line
	for _, line := range lines[1:] {
		lineIndent := 0
		for _, c := range line {
			if c != ' ' && c != '\t' {
				break
			}
			lineIndent++
		}
		if indent > lineIndent {
			indent = lineIndent
		}
	}

	// Trim the indent off of all lines after the first line
	for i, line := range lines {
		if i > 0 {
			lines[i] = line[indent:]
		}
	}
	return strings.Join(lines, "\n")
}

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
