package logger

import (
	"strings"
	"unicode/utf8"
)

type LogStyle int8

const (
	StyleDefault LogStyle = iota
	StyleClang
	StyleVisualStudio
)

func linkifyText(text string, underline string, reset string) string {
	if underline == "" {
		return text
	}

	https := strings.Index(text, "https://")
	if https == -1 {
		return text
	}

	sb := strings.Builder{}
	for {
		https := strings.Index(text, "https://")
		if https == -1 {
			break
		}

		end := strings.IndexByte(text[https:], ' ')
		if end == -1 {
			end = len(text)
		} else {
			end += https
		}

		// Remove trailing punctuation
		if end > https {
			switch text[end-1] {
			case '.', ',', '?', '!', ')', ']', '}':
				end--
			}
		}

		sb.WriteString(text[:https])
		sb.WriteString(underline)
		sb.WriteString(text[https:end])
		sb.WriteString(reset)
		text = text[end:]
	}

	sb.WriteString(text)
	return sb.String()
}

func wrapWordsInString(text string, width int) []string {
	runs := []string{}

outer:
	for text != "" {
		i := 0
		x := 0
		wordEndI := 0

		// Skip over any leading spaces
		for i < len(text) && text[i] == ' ' {
			i++
			x++
		}

		// Find out how many words will fit in this run
		for i < len(text) {
			oldWordEndI := wordEndI
			wordStartI := i

			// Find the end of the word
			for i < len(text) {
				c, width := utf8.DecodeRuneInString(text[i:])
				if c == ' ' {
					break
				}
				i += width
				x += 1 // Naively assume that each unicode code point is a single column
			}
			wordEndI = i

			// Split into a new run if this isn't the first word in the run and the end is past the width
			if wordStartI > 0 && x > width {
				runs = append(runs, text[:oldWordEndI])
				text = text[wordStartI:]
				continue outer
			}

			// Skip over any spaces after the word
			for i < len(text) && text[i] == ' ' {
				i++
				x++
			}
		}

		// If we get here, this is the last run (i.e. everything fits)
		break
	}

	// Remove any trailing spaces on the last run
	for len(text) > 0 && text[len(text)-1] == ' ' {
		text = text[:len(text)-1]
	}
	runs = append(runs, text)
	return runs
}

type formattedLineSpan struct {
	firstLineBefore string
	firstLineMarked string
	firstLineAfter  string
	afterFirstLine  string
	marker          string
	indent          string
}

func formatLineSpan(lineText string, line int, column int, length int, widthLimit int) formattedLineSpan {
	endOfFirstLine := len(lineText)

	// Note: This uses "IndexByte" because Go implements this with SIMD, which
	// can matter a lot for really long lines. Some people pass huge >100mb
	// minified files as line text for the log message.
	if i := strings.IndexByte(lineText, '\n'); i >= 0 {
		endOfFirstLine = i
	}

	firstLine := lineText[:endOfFirstLine]
	afterFirstLine := lineText[endOfFirstLine:]
	if afterFirstLine != "" && !strings.HasSuffix(afterFirstLine, "\n") {
		afterFirstLine += "\n"
	}

	// Clamp values in range
	if line < 0 {
		line = 0
	}
	if column < 0 {
		column = 0
	}
	if length < 0 {
		length = 0
	}
	if column > endOfFirstLine {
		column = endOfFirstLine
	}
	if length > endOfFirstLine-column {
		length = endOfFirstLine - column
	}

	spacesPerTab := 2
	lineText = renderTabStops(firstLine, spacesPerTab)
	textUpToLoc := renderTabStops(firstLine[:column], spacesPerTab)
	markerStart := len(textUpToLoc)
	markerEnd := markerStart
	indent := strings.Repeat(" ", estimateWidthInTerminal(textUpToLoc))
	marker := "^"

	// Extend markers to cover the full range of the error
	if length > 0 {
		markerEnd = len(renderTabStops(firstLine[:column+length], spacesPerTab))
	}

	// Clip the marker to the bounds of the line
	if markerStart > len(lineText) {
		markerStart = len(lineText)
	}
	if markerEnd > len(lineText) {
		markerEnd = len(lineText)
	}
	if markerEnd < markerStart {
		markerEnd = markerStart
	}

	// Trim the line to fit the terminal width
	width := widthLimit
	if width < 1 {
		width = 1
	}
	if column == endOfFirstLine {
		// If the marker is at the very end of the line, the marker will be a "^"
		// character that extends one column past the end of the line. In this case
		// we should reserve a column at the end so the marker doesn't wrap.
		width -= 1
	}
	if len(lineText) > width {
		// Try to center the error
		sliceStart := (markerStart + markerEnd - width) / 2
		if sliceStart > markerStart-width/5 {
			sliceStart = markerStart - width/5
		}
		if sliceStart < 0 {
			sliceStart = 0
		}
		if sliceStart > len(lineText)-width {
			sliceStart = len(lineText) - width
		}
		sliceEnd := sliceStart + width

		// Slice the line
		slicedLine := lineText[sliceStart:sliceEnd]
		markerStart -= sliceStart
		markerEnd -= sliceStart
		if markerStart < 0 {
			markerStart = 0
		}
		if markerEnd > len(slicedLine) {
			markerEnd = len(slicedLine)
		}

		// Truncate the ends with "..."
		if len(slicedLine) > 3 && sliceStart > 0 {
			slicedLine = "..." + slicedLine[3:]
			if markerStart < 3 {
				markerStart = 3
			}
		}
		if len(slicedLine) > 3 && sliceEnd < len(lineText) {
			slicedLine = slicedLine[:len(slicedLine)-3] + "..."
			if markerEnd > len(slicedLine)-3 {
				markerEnd = len(slicedLine) - 3
			}
			if markerEnd < markerStart {
				markerEnd = markerStart
			}
		}

		// Now we can compute the indent
		lineText = slicedLine
		indent = strings.Repeat(" ", estimateWidthInTerminal(lineText[:markerStart]))
	}

	// If marker is still multi-character after clipping, make the marker wider
	if markerEnd-markerStart > 1 {
		marker = strings.Repeat("~", estimateWidthInTerminal(lineText[markerStart:markerEnd]))
	}

	return formattedLineSpan{
		firstLineBefore: lineText[:markerStart],
		firstLineMarked: lineText[markerStart:markerEnd],
		firstLineAfter:  lineText[markerEnd:],
		afterFirstLine:  afterFirstLine,
		marker:          marker,
		indent:          indent,
	}
}

// Estimate the number of columns this string will take when printed
func estimateWidthInTerminal(text string) int {
	// For now just assume each code point is one column. This is wrong but is
	// less wrong than assuming each code unit is one column.
	width := 0
	for text != "" {
		c, size := utf8.DecodeRuneInString(text)
		text = text[size:]

		// Ignore the Zero Width No-Break Space character (UTF-8 BOM)
		if c != 0xFEFF {
			width++
		}
	}
	return width
}

func renderTabStops(withTabs string, spacesPerTab int) string {
	if !strings.ContainsRune(withTabs, '\t') {
		return withTabs
	}

	withoutTabs := strings.Builder{}
	count := 0

	for _, c := range withTabs {
		if c == '\t' {
			spaces := spacesPerTab - count%spacesPerTab
			for i := 0; i < spaces; i++ {
				withoutTabs.WriteRune(' ')
				count++
			}
		} else {
			withoutTabs.WriteRune(c)
			count++
		}
	}

	return withoutTabs.String()
}
