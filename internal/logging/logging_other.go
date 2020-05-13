// +build !darwin
// +build !linux

package logging

import "os"

const SupportsColorEscapes = false

func GetTerminalInfo(*os.File) TerminalInfo {
	return TerminalInfo{}
}
