// +build linux

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

func StderrTerminalInfo() (info TerminalInfo) {
	fd := os.Stderr.Fd()

	// Is stderr a terminal?
	if _, err := unix.IoctlGetTermios(int(fd), unix.TCGETS); err == nil {
		info.UseColorEscapes = true

		// Get the width of the window
		w := new(winsize)
		if _, _, err := unix.Syscall(unix.SYS_IOCTL, fd, unix.TIOCGWINSZ, uintptr(unsafe.Pointer(w))); err == 0 {
			info.Width = int(w.ws_col)
		}
	}

	return
}
