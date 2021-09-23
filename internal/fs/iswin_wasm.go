//go:build js && wasm
// +build js,wasm

package fs

import (
	"os"
)

var checkedIfWindows bool
var cachedIfWindows bool

func CheckIfWindows() bool {
	if !checkedIfWindows {
		checkedIfWindows = true

		// Hack: Assume that we're on Windows if we're running WebAssembly and
		// the "C:\\" directory exists. This is a workaround for a bug in Go's
		// WebAssembly support: https://github.com/golang/go/issues/43768.
		_, err := os.Stat("C:\\")
		cachedIfWindows = err == nil
	}

	return cachedIfWindows
}
