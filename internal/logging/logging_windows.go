// +build windows

package logging

import (
	"os"
	"syscall"
	"unsafe"
)

const SupportsColorEscapes = false

var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var getConsoleMode = kernel32.NewProc("GetConsoleMode")
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
		IsTTY: isTTY != 0,
		Width: int(info.dwSizeX) - 1,
	}
}
