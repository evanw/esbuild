// +build darwin

package logging

import (
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

const SupportsColorEscapes = true

type winsize struct {
	ws_row    uint16
	ws_col    uint16
	ws_xpixel uint16
	ws_ypixel uint16
}

func GetTerminalInfo(file *os.File) (info TerminalInfo) {
	fd := file.Fd()

	// Is this file descriptor a terminal?
	if _, err := unix.IoctlGetTermios(int(fd), unix.TIOCGETA); err == nil {
		info.IsTTY = true
		info.UseColorEscapes = true

		// Get the width of the window
		w := new(winsize)
		if _, _, err := unix.Syscall(unix.SYS_IOCTL, fd, unix.TIOCGWINSZ, uintptr(unsafe.Pointer(w))); err == 0 {
			info.Width = int(w.ws_col)
		}
	}

	return
}

func writeStringWithColor(file *os.File, text string) {
	file.WriteString(text)
}
