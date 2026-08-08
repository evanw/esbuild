package logger

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

func msgToStringDefault(msg Msg, options OutputOptions, terminalInfo TerminalInfo) string {
	// Format the message
	var text strings.Builder
	text.WriteString(msgString(options.IncludeSource, options.PathStyle, terminalInfo, msg.ID, msg.Kind, msg.Data, msg.PluginName))

	// Format the notes
	var oldData MsgData
	for i, note := range msg.Notes {
		if options.IncludeSource && (i == 0 || strings.IndexByte(oldData.Text, '\n') >= 0 || oldData.Location != nil) {
			text.WriteString("\n")
		}
		text.WriteString(msgString(options.IncludeSource, options.PathStyle, terminalInfo, MsgID_None, Note, note, ""))
		oldData = note
	}

	// Add extra spacing between messages if source code is present
	if options.IncludeSource {
		text.WriteString("\n")
	}
	return text.String()
}

func marginWithLineText(maxMargin int, line int) string {
	number := fmt.Sprintf("%d", line)
	return fmt.Sprintf("      %s%s │ ", strings.Repeat(" ", maxMargin-len(number)), number)
}

func emptyMarginText(maxMargin int, isLast bool) string {
	space := strings.Repeat(" ", maxMargin)
	if isLast {
		return fmt.Sprintf("      %s ╵ ", space)
	}
	return fmt.Sprintf("      %s │ ", space)
}

func msgString(includeSource bool, pathStyle PathStyle, terminalInfo TerminalInfo, id MsgID, kind MsgKind, data MsgData, pluginName string) string {
	if !includeSource {
		if loc := data.Location; loc != nil {
			return fmt.Sprintf("%s: %s: %s\n", loc.File.Select(pathStyle), kind.String(), data.Text)
		}
		return fmt.Sprintf("%s: %s\n", kind.String(), data.Text)
	}

	var colors Colors
	if terminalInfo.UseColorEscapes {
		colors = TerminalColors
	}

	var iconColor string
	var kindColorBrackets string
	var kindColorText string

	location := ""

	if data.Location != nil {
		maxMargin := len(fmt.Sprintf("%d", data.Location.Line))
		d := detailStruct(data, pathStyle, terminalInfo, maxMargin)

		if d.Suggestion != "" {
			location = fmt.Sprintf("\n    %s:%d:%d:\n%s%s%s%s%s%s\n%s%s%s%s%s\n%s%s%s%s%s\n%s",
				d.Path, d.Line, d.Column,
				colors.Dim, d.SourceBefore, colors.Green, d.SourceMarked, colors.Dim, d.SourceAfter,
				emptyMarginText(maxMargin, false), d.Indent, colors.Green, d.Marker, colors.Dim,
				emptyMarginText(maxMargin, true), d.Indent, colors.Green, d.Suggestion, colors.Reset,
				d.ContentAfter,
			)
		} else {
			location = fmt.Sprintf("\n    %s:%d:%d:\n%s%s%s%s%s%s\n%s%s%s%s%s\n%s",
				d.Path, d.Line, d.Column,
				colors.Dim, d.SourceBefore, colors.Green, d.SourceMarked, colors.Dim, d.SourceAfter,
				emptyMarginText(maxMargin, true), d.Indent, colors.Green, d.Marker, colors.Reset,
				d.ContentAfter,
			)
		}
	}

	switch kind {
	case Verbose:
		iconColor = colors.Cyan
		kindColorBrackets = colors.CyanBgCyan
		kindColorText = colors.CyanBgBlack

	case Debug:
		iconColor = colors.Green
		kindColorBrackets = colors.GreenBgGreen
		kindColorText = colors.GreenBgWhite

	case Info:
		iconColor = colors.Blue
		kindColorBrackets = colors.BlueBgBlue
		kindColorText = colors.BlueBgWhite

	case Error:
		iconColor = colors.Red
		kindColorBrackets = colors.RedBgRed
		kindColorText = colors.RedBgWhite

	case Warning:
		iconColor = colors.Yellow
		kindColorBrackets = colors.YellowBgYellow
		kindColorText = colors.YellowBgBlack

	case Note:
		sb := strings.Builder{}

		for _, line := range strings.Split(data.Text, "\n") {
			// Special-case word wrapping
			if wrapWidth := terminalInfo.Width; wrapWidth > 2 {
				if !data.DisableMaximumWidth && wrapWidth > 100 {
					wrapWidth = 100 // Enforce a maximum paragraph width for readability
				}
				for _, run := range wrapWordsInString(line, wrapWidth-2) {
					sb.WriteString("  ")
					sb.WriteString(linkifyText(run, colors.Underline, colors.Reset))
					sb.WriteByte('\n')
				}
				continue
			}

			// Otherwise, just write an indented line
			sb.WriteString("  ")
			sb.WriteString(linkifyText(line, colors.Underline, colors.Reset))
			sb.WriteByte('\n')
		}

		sb.WriteString(location)
		return sb.String()
	}

	if pluginName != "" {
		pluginName = fmt.Sprintf(" %s%s[plugin %s]%s", colors.Bold, colors.Magenta, pluginName, colors.Reset)
	}

	msgID := MsgIDToString(id)
	if msgID != "" {
		msgID = fmt.Sprintf(" [%s]", msgID)
	}

	return fmt.Sprintf("%s%s %s[%s%s%s]%s %s%s%s%s%s\n%s",
		iconColor, kind.Icon(),
		kindColorBrackets, kindColorText, kind.String(), kindColorBrackets, colors.Reset,
		colors.Bold, data.Text, colors.Reset, pluginName, msgID,
		location,
	)
}

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

type msgDetail struct {
	SourceBefore string
	SourceMarked string
	SourceAfter  string

	Indent     string
	Marker     string
	Suggestion string

	ContentAfter string

	Path   string
	Line   int
	Column int
}

func detailStruct(data MsgData, pathStyle PathStyle, terminalInfo TerminalInfo, maxMargin int) msgDetail {
	// Only highlight the first line of the line text
	loc := *data.Location
	endOfFirstLine := len(loc.LineText)

	// Note: This uses "IndexByte" because Go implements this with SIMD, which
	// can matter a lot for really long lines. Some people pass huge >100mb
	// minified files as line text for the log message.
	if i := strings.IndexByte(loc.LineText, '\n'); i >= 0 {
		endOfFirstLine = i
	}

	firstLine := loc.LineText[:endOfFirstLine]
	afterFirstLine := loc.LineText[endOfFirstLine:]
	if afterFirstLine != "" && !strings.HasSuffix(afterFirstLine, "\n") {
		afterFirstLine += "\n"
	}

	// Clamp values in range
	if loc.Line < 0 {
		loc.Line = 0
	}
	if loc.Column < 0 {
		loc.Column = 0
	}
	if loc.Length < 0 {
		loc.Length = 0
	}
	if loc.Column > endOfFirstLine {
		loc.Column = endOfFirstLine
	}
	if loc.Length > endOfFirstLine-loc.Column {
		loc.Length = endOfFirstLine - loc.Column
	}

	spacesPerTab := 2
	lineText := renderTabStops(firstLine, spacesPerTab)
	textUpToLoc := renderTabStops(firstLine[:loc.Column], spacesPerTab)
	markerStart := len(textUpToLoc)
	markerEnd := markerStart
	indent := strings.Repeat(" ", estimateWidthInTerminal(textUpToLoc))
	marker := "^"

	// Extend markers to cover the full range of the error
	if loc.Length > 0 {
		markerEnd = len(renderTabStops(firstLine[:loc.Column+loc.Length], spacesPerTab))
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
	width := terminalInfo.Width
	if width < 1 {
		width = defaultTerminalWidth
	}
	width -= maxMargin + extraMarginChars
	if width < 1 {
		width = 1
	}
	if loc.Column == endOfFirstLine {
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

	// Put a margin before the marker indent
	margin := marginWithLineText(maxMargin, loc.Line)

	return msgDetail{
		Path: loc.File.Select(pathStyle),

		// Note: We want to deliberately print the unclamped line and column, as it
		// may come from another tool that either didn't set "LineText" at all or
		// at least didn't calculate the column number correctly.
		Line:   data.Location.Line,
		Column: data.Location.Column,

		SourceBefore: margin + lineText[:markerStart],
		SourceMarked: lineText[markerStart:markerEnd],
		SourceAfter:  lineText[markerEnd:],

		Indent:     indent,
		Marker:     marker,
		Suggestion: loc.Suggestion,

		ContentAfter: afterFirstLine,
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
