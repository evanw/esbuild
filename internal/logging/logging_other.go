// +build !darwin
// +build !linux
// +build !windows

package logging

import "os"

const SupportsColorEscapes = false

func GetTerminalInfo(*os.File) TerminalInfo {
	return TerminalInfo{}
}
