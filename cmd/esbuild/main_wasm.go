// +build js,wasm

package main

import (
	"github.com/evanw/esbuild/internal/logger"
)

// Remove this code from the WebAssembly binary to reduce size. This only removes 0.4mb of stuff.

func createTraceFile(osArgs []string, traceFile string) func() {
	logger.PrintErrorToStderr(osArgs, "The \"--trace\" flag is not supported when using WebAssembly")
	return nil
}

func createHeapFile(osArgs []string, heapFile string) func() {
	logger.PrintErrorToStderr(osArgs, "The \"--heap\" flag is not supported when using WebAssembly")
	return nil
}

func createCpuprofileFile(osArgs []string, cpuprofileFile string) func() {
	logger.PrintErrorToStderr(osArgs, "The \"--cpuprofile\" flag is not supported when using WebAssembly")
	return nil
}
