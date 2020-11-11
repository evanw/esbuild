package main

import (
	"fmt"
	"github.com/evanw/esbuild/internal/snap_api"
	"github.com/evanw/esbuild/pkg/api"
	"os"
	"strings"

	"github.com/evanw/esbuild/internal/logger"
)

const helpText = `
Usage:
  snapshot [options] entry point

Options:
  --outfile=...         The output file
  --metafile=...        Write metadata about the build to a JSON file

Examples:
  snapshot entry_point.js --outfile=out.js --metafile: meta.json 
`

func main() {
	osArgs := os.Args[1:]
	outfile := ""
	metafile := ""
	entryPoint := ""

	// Do an initial scan over the argument list
	argsEnd := 0
	for _, arg := range osArgs {
		switch {
		case arg == "-h", arg == "-help", arg == "--help", arg == "/?":
			fmt.Fprintf(os.Stderr, "%s\n", helpText)
			os.Exit(0)

		case strings.HasPrefix(arg, "--outfile="):
			outfile = arg[len("--outfile="):]

		case strings.HasPrefix(arg, "--metafile="):
			metafile = arg[len("--metafile="):]

		case !strings.HasPrefix(arg, "-"):
			entryPoint = arg

		default:
			osArgs[argsEnd] = arg
			argsEnd++
		}
	}
	osArgs = osArgs[:argsEnd]

	// Print help text when there are no or missing arguments
	if len(osArgs) == 0 && logger.GetTerminalInfo(os.Stdin).IsTTY {
		fmt.Fprintf(os.Stderr, "%s\n", helpText)
		os.Exit(0)
	}
	if entryPoint == "" {
		fmt.Fprintf(os.Stderr, "Need entry point\n\n%s\n", helpText)
		os.Exit(1)
	}
	if outfile == "" {
		fmt.Fprintf(os.Stderr, "Need outfile\n\n%s\n", helpText)
		os.Exit(1)
	}
	if metafile == "" {
		fmt.Fprintf(os.Stderr, "Need metafile\n\n%s\n", helpText)
		os.Exit(1)
	}

	result := nodeJavaScript(entryPoint, outfile, metafile)

	exitCode := len(result.Errors)
	if logger.GetTerminalInfo(os.Stdin).IsTTY {
		for _, warning := range result.Warnings {
			fmt.Fprintln(os.Stderr, warning)
		}
		for _, err := range result.Errors {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	os.Exit(exitCode)
}

func nodeJavaScript(entryPoint string, outfile string, metafile string) api.BuildResult {
	platform := api.PlatformNode
	external := []string{"inherits"}

	return api.Build(api.BuildOptions{
		// https://esbuild.github.io/api/#log-level
		LogLevel: api.LogLevelInfo,

		// https://esbuild.github.io/api/#target
		Target: api.ES2020,

		// inline any imported dependencies into the file itself
		// https://esbuild.github.io/api/#bundle
		Bundle: true,

		// write out a JSON file with metadata about the build
		// https://esbuild.github.io/api/#metafile
		Metafile: metafile,

		// Applies when one entry point is used.
		// https://esbuild.github.io/api/#outfile
		Outfile:     outfile,
		EntryPoints: []string{entryPoint},

		// https://esbuild.github.io/getting-started/#bundling-for-node
		// https://esbuild.github.io/api/#platform
		//
		// Setting to Node results in:
		// - the default output format is set to cjs
		// - built-in node modules such as fs are automatically marked as external
		// - disables the interpretation of the browser field in package.json
		Platform: platform,
		Engines: []api.Engine{
			{api.EngineNode, "12.4"},
		},

		// https://esbuild.github.io/api/#format
		// three possible values: iife, cjs, and esm
		Format: api.FormatCommonJS,

		// the import will be preserved and will be evaluated at run time instead
		// https://esbuild.github.io/api/#external
		External: external,

		//
		// Combination of the below two might be a better way to replace globals
		// while taking the snapshot
		// We'd copy the code for each from the electron blueprint and add it to
		// a module which we use to inject.
		//

		// replace a global variable with an import from another file.
		// https://esbuild.github.io/api/#inject
		// i.e. Inject:      []string{"./process-shim.js"},
		Inject: nil,

		// replace global identifiers with constant expressions
		// https://esbuild.github.io/api/#define
		// i.e.: Define: map[string]string{"DEBUG": "true"},
		Define: nil,

		// When `false` a buffer is returned instead
		// https://esbuild.github.io/api/#write
		Write: true,

		Snapshot: &api.SnapshotOptions{
			CreateSnapshot:       true,
			ShouldReplaceRequire: snap_api.IsExternalModule(platform, external),
		},

		//
		// Unused
		//

		// only matters when the format setting is iife
		GlobalName: "",

		Sourcemap: 0,

		// Only works with ESM modules
		// https://esbuild.github.io/api/#splitting
		Splitting: false,

		MinifyWhitespace:  false,
		MinifyIdentifiers: false,
		MinifySyntax:      false,

		JSXFactory:  "",
		JSXFragment: "",

		// Temporal Dead Zone related perf tweak (var vs let)
		// https://esbuild.github.io/api/#avoid-tdz
		AvoidTDZ: false,

		// https://esbuild.github.io/api/#charset
		Charset: 0,

		// https://esbuild.github.io/api/#color
		Color: 0,

		// https://esbuild.github.io/api/#error-limit
		ErrorLimit: 0,


		// additional package.json fields to try when resolving a package
		// https://esbuild.github.io/api/#main-fields
		MainFields: nil,

		// https://esbuild.github.io/api/#out-extension
		OutExtensions: nil,

		// useful in combination with the external file loader
		// https://esbuild.github.io/api/#public-path
		PublicPath: "",

		// /* #__PURE__ */ before a new or call expression means that that
		// expression can be removed
		// https://esbuild.github.io/api/#pure
		Pure: nil,

		// Tweak resolution algorithm used by node via implicit file extensions
		// https://esbuild.github.io/api/#resolve-extensions
		ResolveExtensions: nil,
		Loader:            nil,

		// Use stdin as input instead of a file
		// https://esbuild.github.io/api/#stdin
		Stdin: nil,

		Tsconfig: "",
	})
}
