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
	err = parseOptionsImpl(osArgs, &options, nil, nil)
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
	err = parseOptionsImpl(osArgs, nil, &options, nil)
	return
}

// This parses an array of strings into an options object suitable for passing
// to "api.Analyse()". Use this if you need to reuse the same argument parsing
// logic as the esbuild CLI.
//
// Example usage:
//
//     options, err := cli.ParseAnalyseOptions([]string{
//         "input.js",
//         "--analyse",
//         "--metafile=...",
//     })
//
//     result := api.Analyse(options)
//
func ParseAnalyseOptions(osArgs []string) (options api.AnalyseOptions, err error) {
	options = newAnalyseOptions()
	err = parseOptionsImpl(osArgs, nil, nil, &options)
	return
}
