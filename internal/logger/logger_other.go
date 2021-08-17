//go:build !darwin && !linux && !windows
// +build !darwin,!linux,!windows

package logger

import "os"

const SupportsColorEscapes = false

func GetTerminalInfo(*os.File) TerminalInfo {
	return TerminalInfo{}
}

func writeStringWithColor(file *os.File, text string) {
	file.WriteString(text)
}
