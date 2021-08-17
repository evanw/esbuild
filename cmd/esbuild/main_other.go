//go:build !js || !wasm
// +build !js !wasm

package main

import (
	"fmt"
	"os"
	"runtime/pprof"
	"runtime/trace"

	"github.com/evanw/esbuild/internal/logger"
)

func createTraceFile(osArgs []string, traceFile string) func() {
	f, err := os.Create(traceFile)
	if err != nil {
		logger.PrintErrorToStderr(osArgs, fmt.Sprintf(
			"Failed to create trace file: %s", err.Error()))
		return nil
	}
	trace.Start(f)
	return func() {
		trace.Stop()
		f.Close()
	}
}

func createHeapFile(osArgs []string, heapFile string) func() {
	f, err := os.Create(heapFile)
	if err != nil {
		logger.PrintErrorToStderr(osArgs, fmt.Sprintf(
			"Failed to create heap file: %s", err.Error()))
		return nil
	}
	return func() {
		if err := pprof.WriteHeapProfile(f); err != nil {
			logger.PrintErrorToStderr(osArgs, fmt.Sprintf(
				"Failed to write heap profile: %s", err.Error()))
		}
		f.Close()
	}
}

func createCpuprofileFile(osArgs []string, cpuprofileFile string) func() {
	f, err := os.Create(cpuprofileFile)
	if err != nil {
		logger.PrintErrorToStderr(osArgs, fmt.Sprintf(
			"Failed to create cpuprofile file: %s", err.Error()))
		return nil
	}
	pprof.StartCPUProfile(f)
	return func() {
		pprof.StopCPUProfile()
		f.Close()
	}
}
