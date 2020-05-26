package main

import (
	"os"

	"github.com/evanw/esbuild/internal/exitcode"
	"github.com/evanw/esbuild/pkg/esbuild"
)

func main() {
	exitcode.Exit(esbuild.CLI(os.Args[1:]))
}
