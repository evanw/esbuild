//go:build (!js || !wasm) && !windows
// +build !js !wasm
// +build !windows

package fs

func is_ERROR_INVALID_NAME(err error) bool {
	return false
}
