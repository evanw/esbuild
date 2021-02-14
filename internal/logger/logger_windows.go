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
		UseColorEscapes: true,
	}
}

func writeStringWithColor(file *os.File, text string) {
	const FOREGROUND_BLUE = 1
	const FOREGROUND_GREEN = 2
	const FOREGROUND_RED = 4
	const FOREGROUND_INTENSITY = 8

	fd := file.Fd()
	i := 0

	for i < len(text) {
		var attributes uintptr
		end := i

		switch {
		case text[i] != 033:
			i++
			continue

		case strings.HasPrefix(text[i:], colorReset):
			i += len(colorReset)
			attributes = FOREGROUND_RED | FOREGROUND_GREEN | FOREGROUND_BLUE

		case strings.HasPrefix(text[i:], colorRed):
			i += len(colorRed)
			attributes = FOREGROUND_RED

		case strings.HasPrefix(text[i:], colorGreen):
			i += len(colorGreen)
			attributes = FOREGROUND_GREEN

		case strings.HasPrefix(text[i:], colorBlue):
			i += len(colorBlue)
			attributes = FOREGROUND_BLUE

		case strings.HasPrefix(text[i:], colorCyan):
			i += len(colorCyan)
			attributes = FOREGROUND_GREEN | FOREGROUND_BLUE

		case strings.HasPrefix(text[i:], colorMagenta):
			i += len(colorMagenta)
			attributes = FOREGROUND_RED | FOREGROUND_BLUE

		case strings.HasPrefix(text[i:], colorYellow):
			i += len(colorYellow)
			attributes = FOREGROUND_RED | FOREGROUND_GREEN

		case strings.HasPrefix(text[i:], colorResetDim):
			i += len(colorResetDim)
			attributes = FOREGROUND_RED | FOREGROUND_GREEN | FOREGROUND_BLUE

		case strings.HasPrefix(text[i:], colorBold):
			i += len(colorBold)
			attributes = FOREGROUND_RED | FOREGROUND_GREEN | FOREGROUND_BLUE | FOREGROUND_INTENSITY

		case strings.HasPrefix(text[i:], colorResetBold):
			i += len(colorResetBold)
			attributes = FOREGROUND_RED | FOREGROUND_GREEN | FOREGROUND_BLUE | FOREGROUND_INTENSITY

		// Apparently underlines only work with the CJK locale on Windows :(
		case strings.HasPrefix(text[i:], colorResetUnderline):
			i += len(colorResetUnderline)
			attributes = FOREGROUND_RED | FOREGROUND_GREEN | FOREGROUND_BLUE

		default:
			i++
			continue
		}

		file.WriteString(text[:end])
		text = text[i:]
		i = 0
		setConsoleTextAttribute.Call(fd, attributes)
	}

	file.WriteString(text)
}
