package logger

import (
	"fmt"
	"strings"
)

// This log style is intended to help integrate esbuild with Visual Studio's
// built-in problem matcher. More information:
//
// - https://learn.microsoft.com/en-us/cpp/build/formatting-the-output-of-a-custom-build-step-or-build-event
// - https://learn.microsoft.com/en-us/visualstudio/msbuild/msbuild-diagnostic-format-for-tasks
func msgToStringVisualStudio(msg Msg, options OutputOptions, terminalInfo TerminalInfo) string {
	var text strings.Builder

	// Write the origin
	if loc := msg.Data.Location; loc != nil {
		// Potentially this needs to always be an absolute path?
		text.WriteString(loc.File.Abs)

		// Note: Need to adjust the column from 0-based to 1-based
		if loc.Line > 0 {
			text.WriteString(fmt.Sprintf("(%d,%d)", loc.Line, loc.Column+1))
		}
		text.WriteString(": ")
	} else {
		// If there is no file, then we must write a tool name
		text.WriteString("esbuild: ")
	}

	// The only valid options for the category seem to be "error" and "warning"
	if msg.Kind == Error {
		text.WriteString("error: ")
	} else {
		text.WriteString("warning: ")
	}

	text.WriteString(msg.Data.Text)
	text.WriteByte('\n')
	return text.String()
}
