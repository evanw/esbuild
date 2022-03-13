package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/evanw/esbuild/internal/api_helpers"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/pkg/cli"
)

var helpText = func(colors logger.Colors) string {
	// Read "NO_COLOR" from the environment. This is a convention that some
	// software follows. See https://no-color.org/ for more information.
	for _, key := range os.Environ() {
		if strings.HasPrefix(key, "NO_COLOR=") {
			colors = logger.Colors{}
			break
		}
	}

	return `
` + colors.Bold + `Usage:` + colors.Reset + `
  esbuild [options] [entry points]

` + colors.Bold + `Documentation:` + colors.Reset + `
  ` + colors.Underline + `https://esbuild.github.io/` + colors.Reset + `

` + colors.Bold + `Repository:` + colors.Reset + `
  ` + colors.Underline + `https://github.com/evanw/esbuild` + colors.Reset + `

` + colors.Bold + `Simple options:` + colors.Reset + `
  --bundle              Bundle all dependencies into the output files
  --define:K=V          Substitute K with V while parsing
  --external:M          Exclude module M from the bundle (can use * wildcards)
  --format=...          Output format (iife | cjs | esm, no default when not
                        bundling, otherwise default is iife when platform
                        is browser and cjs when platform is node)
  --loader:X=L          Use loader L to load file extension X, where L is
                        one of: js | jsx | ts | tsx | css | json | text |
                        base64 | file | dataurl | binary
  --minify              Minify the output (sets all --minify-* flags)
  --outdir=...          The output directory (for multiple entry points)
  --outfile=...         The output file (for one entry point)
  --platform=...        Platform target (browser | node | neutral,
                        default browser)
  --serve=...           Start a local HTTP server on this host:port for outputs
  --sourcemap           Emit a source map
  --splitting           Enable code splitting (currently only for esm)
  --target=...          Environment target (e.g. es2017, chrome58, firefox57,
                        safari11, edge16, node10, ie9, opera45, default esnext)
  --watch               Watch mode: rebuild on file system changes

` + colors.Bold + `Advanced options:` + colors.Reset + `
  --allow-overwrite         Allow output files to overwrite input files
  --analyze                 Print a report about the contents of the bundle
                            (use "--analyze=verbose" for a detailed report)
  --asset-names=...         Path template to use for "file" loader files
                            (default "[name]-[hash]")
  --banner:T=...            Text to be prepended to each output file of type T
                            where T is one of: css | js
  --charset=utf8            Do not escape UTF-8 code points
  --chunk-names=...         Path template to use for code splitting chunks
                            (default "[name]-[hash]")
  --color=...               Force use of color terminal escapes (true | false)
  --drop:...                Remove certain constructs (console | debugger)
  --entry-names=...         Path template to use for entry point output paths
                            (default "[dir]/[name]", can also use "[hash]")
  --footer:T=...            Text to be appended to each output file of type T
                            where T is one of: css | js
  --global-name=...         The name of the global for the IIFE format
  --ignore-annotations      Enable this to work with packages that have
                            incorrect tree-shaking annotations
  --inject:F                Import the file F into all input files and
                            automatically replace matching globals with imports
  --jsx-factory=...         What to use for JSX instead of React.createElement
  --jsx-fragment=...        What to use for JSX instead of React.Fragment
  --jsx=...                 Set to "preserve" to disable transforming JSX to JS
  --keep-names              Preserve "name" on functions and classes
  --legal-comments=...      Where to place legal comments (none | inline |
                            eof | linked | external, default eof when bundling
                            and inline otherwise)
  --log-level=...           Disable logging (verbose | debug | info | warning |
                            error | silent, default info)
  --log-limit=...           Maximum message count or 0 to disable (default 6)
  --main-fields=...         Override the main file order in package.json
                            (default "browser,module,main" when platform is
                            browser and "main,module" when platform is node)
  --mangle-cache=...        Save "mangle props" decisions to a JSON file
  --mangle-props=...        Rename all properties matching a regular expression
  --mangle-quoted=...       Enable renaming of quoted properties (true | false)
  --metafile=...            Write metadata about the build to a JSON file
  --minify-whitespace       Remove whitespace in output files
  --minify-identifiers      Shorten identifiers in output files
  --minify-syntax           Use equivalent but shorter syntax in output files
  --out-extension:.js=.mjs  Use a custom output extension instead of ".js"
  --outbase=...             The base path used to determine entry point output
                            paths (for multiple entry points)
  --preserve-symlinks       Disable symlink resolution for module lookup
  --public-path=...         Set the base URL for the "file" loader
  --pure:N                  Mark the name N as a pure function for tree shaking
  --reserve-props=...       Do not mangle these properties
  --resolve-extensions=...  A comma-separated list of implicit extensions
                            (default ".tsx,.ts,.jsx,.js,.css,.json")
  --servedir=...            What to serve in addition to generated output files
  --source-root=...         Sets the "sourceRoot" field in generated source maps
  --sourcefile=...          Set the source file for the source map (for stdin)
  --sourcemap=external      Do not link to the source map with a comment
  --sourcemap=inline        Emit the source map with an inline data URL
  --sources-content=false   Omit "sourcesContent" in generated source maps
  --tree-shaking=...        Force tree shaking on or off (false | true)
  --tsconfig=...            Use this tsconfig.json file instead of other ones
  --version                 Print the current version (` + esbuildVersion + `) and exit

` + colors.Bold + `Examples:` + colors.Reset + `
  ` + colors.Dim + `# Produces dist/entry_point.js and dist/entry_point.js.map` + colors.Reset + `
  esbuild --bundle entry_point.js --outdir=dist --minify --sourcemap

  ` + colors.Dim + `# Allow JSX syntax in .js files` + colors.Reset + `
  esbuild --bundle entry_point.js --outfile=out.js --loader:.js=jsx

  ` + colors.Dim + `# Substitute the identifier RELEASE for the literal true` + colors.Reset + `
  esbuild example.js --outfile=out.js --define:RELEASE=true

  ` + colors.Dim + `# Provide input via stdin, get output via stdout` + colors.Reset + `
  esbuild --minify --loader=ts < input.ts > output.js

  ` + colors.Dim + `# Automatically rebuild when input files are changed` + colors.Reset + `
  esbuild app.ts --bundle --watch

  ` + colors.Dim + `# Start a local HTTP server for everything in "www"` + colors.Reset + `
  esbuild app.ts --bundle --servedir=www --outdir=www/js

`
}

func main() {
	logger.API = logger.CLIAPI

	osArgs := os.Args[1:]
	heapFile := ""
	traceFile := ""
	cpuprofileFile := ""
	isRunningService := false
	sendPings := false

	// Do an initial scan over the argument list
	argsEnd := 0
	for _, arg := range osArgs {
		switch {
		// Show help if a common help flag is provided
		case arg == "-h", arg == "-help", arg == "--help", arg == "/?":
			logger.PrintText(os.Stdout, logger.LevelSilent, os.Args, helpText)
			os.Exit(0)

		// Special-case the version flag here
		case arg == "--version":
			fmt.Printf("%s\n", esbuildVersion)
			os.Exit(0)

		case strings.HasPrefix(arg, "--heap="):
			heapFile = arg[len("--heap="):]

		case strings.HasPrefix(arg, "--trace="):
			traceFile = arg[len("--trace="):]

		case strings.HasPrefix(arg, "--timing"):
			// This is a hidden flag because it's only intended for debugging esbuild
			// itself. The output is not documented and not stable.
			api_helpers.UseTimer = true

		case strings.HasPrefix(arg, "--cpuprofile="):
			cpuprofileFile = arg[len("--cpuprofile="):]

		// This flag turns the process into a long-running service that uses
		// message passing with the host process over stdin/stdout
		case strings.HasPrefix(arg, "--service="):
			hostVersion := arg[len("--service="):]
			isRunningService = true

			// Validate the host's version number to make sure esbuild was installed
			// correctly. This check was added because some people have reported
			// errors that appear to indicate an incorrect installation.
			if hostVersion != esbuildVersion {
				logger.PrintErrorToStderr(osArgs,
					fmt.Sprintf("Cannot start service: Host version %q does not match binary version %q",
						hostVersion, esbuildVersion))
				os.Exit(1)
			}

		case strings.HasPrefix(arg, "--ping"):
			sendPings = true

		default:
			// Strip any arguments that were handled above
			osArgs[argsEnd] = arg
			argsEnd++
		}
	}
	osArgs = osArgs[:argsEnd]

	// Run in service mode if requested
	if isRunningService {
		runService(sendPings)
		return
	}

	// Print help text when there are no arguments
	isStdinTTY := logger.GetTerminalInfo(os.Stdin).IsTTY
	if len(osArgs) == 0 && isStdinTTY {
		logger.PrintText(os.Stdout, logger.LevelSilent, osArgs, helpText)
		os.Exit(0)
	}

	// Capture the defer statements below so the "done" message comes last
	exitCode := 1
	func() {
		// To view a CPU trace, use "go tool trace [file]". Note that the trace
		// viewer doesn't work under Windows Subsystem for Linux for some reason.
		if traceFile != "" {
			if done := createTraceFile(osArgs, traceFile); done == nil {
				return
			} else {
				defer done()
			}
		}

		// To view a heap trace, use "go tool pprof [file]" and type "top". You can
		// also drop it into https://speedscope.app and use the "left heavy" or
		// "sandwich" view modes.
		if heapFile != "" {
			if done := createHeapFile(osArgs, heapFile); done == nil {
				return
			} else {
				defer done()
			}
		}

		// To view a CPU profile, drop the file into https://speedscope.app.
		// Note: Running the CPU profiler doesn't work under Windows subsystem for
		// Linux. The profiler has to be built for native Windows and run using the
		// command prompt instead.
		if cpuprofileFile != "" {
			if done := createCpuprofileFile(osArgs, cpuprofileFile); done == nil {
				return
			} else {
				defer done()
			}
		}

		if cpuprofileFile != "" {
			// The CPU profiler in Go only runs at 100 Hz, which is far too slow to
			// return useful information for esbuild, since it's so fast. Let's keep
			// running for 30 seconds straight, which should give us 3,000 samples.
			seconds := 30.0
			start := time.Now()
			for time.Since(start).Seconds() < seconds {
				exitCode = cli.Run(osArgs)
			}
		} else {
			// Don't disable the GC if this is a long-running process
			isServeOrWatch := false
			for _, arg := range osArgs {
				if arg == "--serve" || arg == "--watch" || strings.HasPrefix(arg, "--serve=") {
					isServeOrWatch = true
					break
				}
			}

			if !isServeOrWatch {
				// Disable the GC since we're just going to allocate a bunch of memory
				// and then exit anyway. This speedup is not insignificant. Make sure to
				// only do this here once we know that we're not going to be a long-lived
				// process though.
				debug.SetGCPercent(-1)
			} else if !isStdinTTY {
				// If stdin isn't a TTY, watch stdin and abort in case it is closed.
				// This is necessary when the esbuild binary executable is invoked via
				// the Erlang VM, which doesn't provide a way to exit a child process.
				// See: https://github.com/brunch/brunch/issues/920.
				//
				// We don't do this when stdin is a TTY because that interferes with
				// the Unix background job system. If we read from stdin then Ctrl+Z
				// to move the process to the background will incorrectly cause the
				// job to stop. See: https://github.com/brunch/brunch/issues/998.
				go func() {
					// This just discards information from stdin because we don't use
					// it and we can avoid unnecessarily allocating space for it
					buffer := make([]byte, 512)
					for {
						_, err := os.Stdin.Read(buffer)
						if err != nil {
							// Only exit cleanly if stdin was closed cleanly
							if err == io.EOF {
								os.Exit(0)
							} else {
								os.Exit(1)
							}
						}
					}
				}()
			}

			exitCode = cli.Run(osArgs)
		}
	}()

	os.Exit(exitCode)
}
