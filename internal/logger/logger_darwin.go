//go:build darwin
// +build darwin

package logger

import (
	"os"

	"golang.org/x/sys/unix"
)

const SupportsColorEscapes = true

func GetTerminalInfo(file *os.File) (info TerminalInfo) {
	fd := file.Fd()

	// Is this file descriptor a terminal?
	if _, err := unix.IoctlGetTermios(int(fd), unix.TIOCGETA); err == nil {
		info.IsTTY = true
		info.UseColorEscapes = !hasNoColorEnvironmentVariable()

		// Get the width of the window
		if w, err := unix.IoctlGetWinsize(int(fd), unix.TIOCGWINSZ); err == nil {
			info.Width = int(w.Col)
			info.Height = int(w.Row)
		}
	}

	return
}

func writeStringWithColor(file *os.File, text string) {
	file.WriteString(text)
}
