//go:build windows
// +build windows

package logger

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

const SupportsColorEscapes = true

var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var getConsoleMode = kernel32.NewProc("GetConsoleMode")
var setConsoleTextAttribute = kernel32.NewProc("SetConsoleTextAttribute")
var getConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")

type consoleScreenBufferInfo struct {
	dwSizeX              int16
	dwSizeY              int16
	dwCursorPositionX    int16
	dwCursorPositionY    int16
	wAttributes          uint16
	srWindowLeft         int16
	srWindowTop          int16
	srWindowRight        int16
	srWindowBottom       int16
	dwMaximumWindowSizeX int16
	dwMaximumWindowSizeY int16
}

func GetTerminalInfo(file *os.File) TerminalInfo {
	fd := file.Fd()

	// Is this file descriptor a terminal?
	var unused uint32
	isTTY, _, _ := syscall.Syscall(getConsoleMode.Addr(), 2, fd, uintptr(unsafe.Pointer(&unused)), 0)

	// Get the width of the window
	var info consoleScreenBufferInfo
	syscall.Syscall(getConsoleScreenBufferInfo.Addr(), 2, fd, uintptr(unsafe.Pointer(&info)), 0)

	return TerminalInfo{
		IsTTY:           isTTY != 0,
		Width:           int(info.dwSizeX) - 1,
		Height:          int(info.dwSizeY) - 1,
		UseColorEscapes: !hasNoColorEnvironmentVariable(),
	}
}

const (
	FOREGROUND_BLUE uint8 = 1 << iota
	FOREGROUND_GREEN
	FOREGROUND_RED
	FOREGROUND_INTENSITY
	BACKGROUND_BLUE
	BACKGROUND_GREEN
	BACKGROUND_RED
	BACKGROUND_INTENSITY
)

var windowsEscapeSequenceMap = map[string]uint8{
	TerminalColors.Reset: FOREGROUND_RED | FOREGROUND_GREEN | FOREGROUND_BLUE,
	TerminalColors.Dim:   FOREGROUND_RED | FOREGROUND_GREEN | FOREGROUND_BLUE,
	TerminalColors.Bold:  FOREGROUND_RED | FOREGROUND_GREEN | FOREGROUND_BLUE | FOREGROUND_INTENSITY,

	// Apparently underlines only work with the CJK locale on Windows :(
	TerminalColors.Underline: FOREGROUND_RED | FOREGROUND_GREEN | FOREGROUND_BLUE,

	TerminalColors.Red:   FOREGROUND_RED,
	TerminalColors.Green: FOREGROUND_GREEN,
	TerminalColors.Blue:  FOREGROUND_BLUE,

	TerminalColors.Cyan:    FOREGROUND_GREEN | FOREGROUND_BLUE,
	TerminalColors.Magenta: FOREGROUND_RED | FOREGROUND_BLUE,
	TerminalColors.Yellow:  FOREGROUND_RED | FOREGROUND_GREEN,

	TerminalColors.RedBgRed:     FOREGROUND_RED | BACKGROUND_RED,
	TerminalColors.RedBgWhite:   FOREGROUND_RED | FOREGROUND_GREEN | FOREGROUND_BLUE | BACKGROUND_RED,
	TerminalColors.GreenBgGreen: FOREGROUND_GREEN | BACKGROUND_GREEN,
	TerminalColors.GreenBgWhite: FOREGROUND_RED | FOREGROUND_GREEN | FOREGROUND_BLUE | BACKGROUND_GREEN,
	TerminalColors.BlueBgBlue:   FOREGROUND_BLUE | BACKGROUND_BLUE,
	TerminalColors.BlueBgWhite:  FOREGROUND_RED | FOREGROUND_GREEN | FOREGROUND_BLUE | BACKGROUND_BLUE,

	TerminalColors.CyanBgCyan:       FOREGROUND_GREEN | FOREGROUND_BLUE | BACKGROUND_GREEN | BACKGROUND_BLUE,
	TerminalColors.CyanBgBlack:      BACKGROUND_GREEN | BACKGROUND_BLUE,
	TerminalColors.MagentaBgMagenta: FOREGROUND_RED | FOREGROUND_BLUE | BACKGROUND_RED | BACKGROUND_BLUE,
	TerminalColors.MagentaBgBlack:   BACKGROUND_RED | BACKGROUND_BLUE,
	TerminalColors.YellowBgYellow:   FOREGROUND_RED | FOREGROUND_GREEN | BACKGROUND_RED | BACKGROUND_GREEN,
	TerminalColors.YellowBgBlack:    BACKGROUND_RED | BACKGROUND_GREEN,
}

func writeStringWithColor(file *os.File, text string) {
	fd := file.Fd()
	i := 0

	for i < len(text) {
		// Find the escape
		if text[i] != 033 {
			i++
			continue
		}

		// Find the 'm'
		window := text[i:]
		if len(window) > 8 {
			window = window[:8]
		}
		m := strings.IndexByte(window, 'm')
		if m == -1 {
			i++
			continue
		}
		m += i + 1

		// Find the escape sequence
		attributes, ok := windowsEscapeSequenceMap[text[i:m]]
		if !ok {
			i++
			continue
		}

		// Write out the text before the escape sequence
		file.WriteString(text[:i])

		// Apply the escape sequence
		text = text[m:]
		i = 0
		setConsoleTextAttribute.Call(fd, uintptr(attributes))
	}

	// Write out the remaining text
	file.WriteString(text)
}
