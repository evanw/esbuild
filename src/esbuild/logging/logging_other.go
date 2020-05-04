// +build !darwin
// +build !linux

package logging

const SupportsColorEscapes = false

func StderrTerminalInfo() TerminalInfo {
	return TerminalInfo{}
}
