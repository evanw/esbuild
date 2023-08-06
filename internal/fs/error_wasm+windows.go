//go:build (js && wasm) || windows
// +build js,wasm windows

package fs

import "syscall"

// This check is here in a conditionally-compiled file because Go's standard
// library for Plan 9 doesn't define a type called "syscall.Errno". Plan 9 is
// not a supported operating system but someone wanted to be able to compile
// esbuild for Plan 9 anyway.
func is_ERROR_INVALID_NAME(err error) bool {
	// This has been copied from golang.org/x/sys/windows
	const ERROR_INVALID_NAME syscall.Errno = 123

	return err == ERROR_INVALID_NAME
}
