// This API exposes the command-line interface for esbuild. It can be used to
// run esbuild from Go without the overhead of creating a child process.
//
// Example usage:
//
//     package main
//
//     import (
//         "os"
//
//         "github.com/evanw/esbuild/pkg/cli"
//     )
//
//     func main() {
//         os.Exit(cli.Run(os.Args[1:]))
//     }
//
package cli

import (
	"errors"

	"github.com/evanw/esbuild/pkg/api"
)

// This function invokes the esbuild CLI. It takes an array of command-line
// arguments (excluding the executable argument itself) and returns an exit
// code. There are some minor differences between this CLI and the actual
// "esbuild" executable such as the lack of auxiliary flags (e.g. "--help" and
// "--version") but it is otherwise exactly the same code.
func Run(osArgs []string) int {
	return runImpl(osArgs)
}

// This parses an array of strings into an options object suitable for passing
// to "api.Build()". Use this if you need to reuse the same argument parsing
// logic as the esbuild CLI.
//
// Example usage:
//
//     options, err := cli.ParseBuildOptions([]string{
//         "input.js",
//         "--bundle",
//         "--minify",
//     })
//
//     result := api.Build(options)
//
func ParseBuildOptions(osArgs []string) (options api.BuildOptions, err error) {
	options = newBuildOptions()
	_, errWithNote := parseOptionsImpl(osArgs, &options, nil, kindExternal)
	if errWithNote != nil {
		err = errors.New(errWithNote.Text)
	}
	return
}

// This parses an array of strings into an options object suitable for passing
// to "api.Transform()". Use this if you need to reuse the same argument
// parsing logic as the esbuild CLI.
//
// Example usage:
//
//     options, err := cli.ParseTransformOptions([]string{
//         "--minify",
//         "--loader=tsx",
//         "--define:DEBUG=false",
//     })
//
//     result := api.Transform(input, options)
//
func ParseTransformOptions(osArgs []string) (options api.TransformOptions, err error) {
	options = newTransformOptions()
	_, errWithNote := parseOptionsImpl(osArgs, nil, &options, kindExternal)
	if errWithNote != nil {
		err = errors.New(errWithNote.Text)
	}
	return
}

// This parses an array of strings into an options object suitable for passing
// to "api.Serve()". The remaining non-serve arguments are returned in another
// array to then be passed to "api.ParseBuildOptions()". Use this if you need
// to reuse the same argument parsing logic as the esbuild CLI.
//
// Example usage:
//
//     serveOptions, args, err := cli.ParseServeOptions([]string{
//         "--serve=8000",
//     })
//
//     buildOptions, err := cli.ParseBuildOptions(args)
//
//     result := api.Serve(serveOptions, buildOptions)
//
func ParseServeOptions(osArgs []string) (options api.ServeOptions, remainingArgs []string, err error) {
	return parseServeOptionsImpl(osArgs)
}
