//go:build !js || !wasm
// +build !js !wasm

package bundler

import "net/http"

func sniffMimeType(contents []byte) string {
	return http.DetectContentType(contents)
}
