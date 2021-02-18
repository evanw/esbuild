// +build js,wasm

package api

import "fmt"

func serveImpl(serveOptions ServeOptions, buildOptions BuildOptions) (ServeResult, error) {
	return ServeResult{}, fmt.Errorf("The \"serve\" API is not supported when using WebAssembly")
}
