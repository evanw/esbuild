//go:build (!js || !wasm) && !windows
// +build !js !wasm
// +build !windows

package fs

func CheckIfWindows() bool {
	return false
}
