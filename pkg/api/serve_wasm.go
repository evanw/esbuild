//go:build js && wasm
// +build js,wasm

package api

import "fmt"

// Remove the serve API in the WebAssembly build. This removes 2.7mb of stuff.

func (*internalContext) Serve(ServeOptions) (ServeResult, error) {
	return ServeResult{}, fmt.Errorf("The \"serve\" API is not supported when using WebAssembly")
}

type apiHandler struct {
}

func (*apiHandler) broadcastBuildResult(BuildResult, map[string]string) {
}

func (*apiHandler) stop() {
}
