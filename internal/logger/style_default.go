package logger

import (
	"fmt"
	"strings"
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

	if loc := data.Location; loc != nil {
		maxMargin := len(fmt.Sprintf("%d", loc.Line))
		margin := marginWithLineText(maxMargin, loc.Line)
		widthLimit := terminalInfo.Width
		if widthLimit < 1 {
			widthLimit = defaultTerminalWidth
		}
		widthLimit -= maxMargin + extraMarginChars

		span := formatLineSpan(loc.LineText, loc.Line, loc.Column, loc.Length, widthLimit)
		path := loc.File.Select(pathStyle)

		if loc.Suggestion != "" {
			location = fmt.Sprintf("\n    %s:%d:%d:\n%s%s%s%s%s%s%s\n%s%s%s%s%s\n%s%s%s%s%s\n%s",
				path, loc.Line, loc.Column,
				colors.Dim, margin, span.firstLineBefore, colors.Green, span.firstLineMarked, colors.Dim, span.firstLineAfter,
				emptyMarginText(maxMargin, false), span.indent, colors.Green, span.marker, colors.Dim,
				emptyMarginText(maxMargin, true), span.indent, colors.Green, loc.Suggestion, colors.Reset,
				span.afterFirstLine,
			)
		} else {
			location = fmt.Sprintf("\n    %s:%d:%d:\n%s%s%s%s%s%s%s\n%s%s%s%s%s\n%s",
				path, loc.Line, loc.Column,
				colors.Dim, margin, span.firstLineBefore, colors.Green, span.firstLineMarked, colors.Dim, span.firstLineAfter,
				emptyMarginText(maxMargin, true), span.indent, colors.Green, span.marker, colors.Reset,
				span.afterFirstLine,
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
