package logger

import (
	"fmt"
	"strings"
)

// This log style is intended to help integrate esbuild with tools that expect
// clang's output style. More information:
//
// - https://clang.llvm.org/docs/UsersManual.html#formatting-of-diagnostics
// - https://clang.llvm.org/diagnostics.html
func msgToStringClang(msg Msg, options OutputOptions, terminalInfo TerminalInfo) string {
	var text strings.Builder
	writeMsgDataClang(&text, msg.ID, msg.Data, options, terminalInfo, msg.Kind)
	for _, note := range msg.Notes {
		if note.Location != nil {
			writeMsgDataClang(&text, MsgID_None, note, options, terminalInfo, Note)
		}
	}
	return text.String()
}

func writeMsgDataClang(text *strings.Builder, id MsgID, data MsgData, options OutputOptions, terminalInfo TerminalInfo, kind MsgKind) {
	var colors Colors
	if terminalInfo.UseColorEscapes {
		colors = TerminalColors
	}
	text.WriteString(colors.Bold)

	if loc := data.Location; loc != nil {
		text.WriteString(loc.File.Select(options.PathStyle))

		// Note: Need to adjust the column from 0-based to 1-based
		if loc.Line > 0 {
			text.WriteString(fmt.Sprintf(":%d:%d", loc.Line, loc.Column+1))
		}
		text.WriteString(": ")
	}

	var kindColor string
	var kindText string
	switch kind {
	case Error:
		kindColor, kindText = colors.Red, "error"
	case Note:
		kindColor, kindText = colors.Dim, "note"
	default:
		kindColor, kindText = colors.Magenta, "warning"
	}

	msgID := MsgIDToString(id)
	if msgID != "" {
		msgID = fmt.Sprintf(" [%s]", msgID)
	}

	text.WriteString(fmt.Sprintf("%s%s:%s %s%s%s%s\n",
		kindColor, kindText, colors.Reset, colors.Bold, data.Text, msgID, colors.Reset))

	if loc := data.Location; loc != nil && loc.Line > 0 {
		widthLimit := terminalInfo.Width
		if widthLimit < 1 {
			widthLimit = defaultTerminalWidth
		}

		span := formatLineSpan(loc.LineText, loc.Line, loc.Column, loc.Length, widthLimit)
		text.WriteString(fmt.Sprintf("%s%s%s%s%s%s\n%s%s%s%s\n",
			colors.Dim, span.firstLineBefore, colors.Green, span.firstLineMarked, colors.Dim, span.firstLineAfter,
			span.indent, colors.Green, span.marker, colors.Reset,
		))

		if loc.Suggestion != "" {
			text.WriteString(fmt.Sprintf("%s%s%s%s\n", colors.Green, span.indent, loc.Suggestion, colors.Reset))
		}
	}
}
