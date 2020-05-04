// +build !darwin
// +build !linux

package logging

func StderrTerminalInfo() TerminalInfo {
	return TerminalInfo{}
}
