// +build js,wasm

package api

import "fmt"

// Remove the serve API in the WebAssembly build. This removes 2.7mb of stuff.

func serveImpl(serveOptions ServeOptions, buildOptions BuildOptions) (ServeResult, error) {
	return ServeResult{}, fmt.Errorf("The \"serve\" API is not supported when using WebAssembly")
}
