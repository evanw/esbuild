package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"runtime/pprof"
	"runtime/trace"
	"strings"
	"time"

	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/pkg/cli"
)

const helpText = `
Usage:
  esbuild [options] [entry points]


Options:
  --bundle              Bundle all dependencies into the output files
  --define:K=V          Substitute K with V while parsing
  --external:M          Exclude module M from the bundle
  --format=...          Output format (iife | cjs | esm, no default when not
                        bundling, otherwise default is iife when platform
                        is browser and cjs when platform is node)
  --global-name=...     The name of the global for the IIFE format
  --jsx-factory=...     What to use instead of React.createElement
  --jsx-fragment=...    What to use instead of React.Fragment
  --loader:X=L          Use loader L to load file extension X, where L is
                        one of: js | jsx | ts | tsx | json | text | base64 |
                        file | dataurl | binary
  --minify              Sets all --minify-* flags
  --minify-whitespace   Remove whitespace
  --minify-identifiers  Shorten identifiers
  --minify-syntax       Use equivalent but shorter syntax
  --outdir=...          The output directory (for multiple entry points)
  --outfile=...         The output file (for one entry point)
  --platform=...        Platform target (browser | node, default browser)
  --sourcemap           Emit a source map
  --splitting           Enable code splitting (currently only for esm)
  --target=...          Environment target (e.g. es2017, chrome58, firefox57,
                        safari11, edge16, node10, default esnext)

Advanced options:
  --avoid-tdz               An optimization for large bundles in Safari
  --banner=...              Text to be prepended to each output file
  --charset=utf8            Do not escape UTF-8 code points
  --color=...               Force use of color terminal escapes (true | false)
  --error-limit=...         Maximum error count or 0 to disable (default 10)
  --footer=...              Text to be appended to each output file
  --inject:F                Import the file F into all input files and
                            automatically replace matching globals with imports
  --keep-names              Preserve "name" on functions and classes
  --log-level=...           Disable logging (info | warning | error | silent,
                            default info)
  --main-fields=...         Override the main file order in package.json
                            (default "browser,module,main" when platform is
                            browser and "main,module" when platform is node)
  --metafile=...            Write metadata about the build to a JSON file
  --out-extension:.js=.mjs  Use a custom output extension instead of ".js"
  --outbase=...             The base path used to determine entry point output
                            paths (for multiple entry points)
  --public-path=...         Set the base URL for the "file" loader
  --pure:N                  Mark the name N as a pure function for tree shaking
  --resolve-extensions=...  A comma-separated list of implicit extensions
                            (default ".tsx,.ts,.jsx,.mjs,.cjs,.js,.css,.json")
  --sourcefile=...          Set the source file for the source map (for stdin)
  --sourcemap=external      Do not link to the source map with a comment
  --sourcemap=inline        Emit the source map with an inline data URL
  --tree-shaking=...        Set to "ignore-annotations" to work with packages
                            that have incorrect tree-shaking annotations
  --tsconfig=...            Use this tsconfig.json file instead of other ones
  --version                 Print the current version and exit (` + esbuildVersion + `)

Examples:
  # Produces dist/entry_point.js and dist/entry_point.js.map
  esbuild --bundle entry_point.js --outdir=dist --minify --sourcemap

  # Allow JSX syntax in .js files
  esbuild --bundle entry_point.js --outfile=out.js --loader:.js=jsx

  # Substitute the identifier RELEASE for the literal true
  esbuild example.js --outfile=out.js --define:RELEASE=true

  # Provide input via stdin, get output via stdout
  esbuild --minify --loader=ts < input.ts > output.js
`

func main() {
	osArgs := os.Args[1:]
	heapFile := ""
	traceFile := ""
	cpuprofileFile := ""
	isRunningService := false

	// Do an initial scan over the argument list
	argsEnd := 0
	for _, arg := range osArgs {
		switch {
		// Show help if a common help flag is provided
		case arg == "-h", arg == "-help", arg == "--help", arg == "/?":
			fmt.Fprintf(os.Stderr, "%s\n", helpText)
			os.Exit(0)

		// Special-case the version flag here
		case arg == "--version":
			fmt.Printf("%s\n", esbuildVersion)
			os.Exit(0)

		case strings.HasPrefix(arg, "--heap="):
			heapFile = arg[len("--heap="):]

		case strings.HasPrefix(arg, "--trace="):
			traceFile = arg[len("--trace="):]

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

		case arg == "--service":
			logger.PrintErrorToStderr(osArgs, "Cannot start service: No version number from host")
			os.Exit(1)

		default:
			// Strip any arguments that were handled above
			osArgs[argsEnd] = arg
			argsEnd++
		}
	}
	osArgs = osArgs[:argsEnd]

	// Run in service mode if requested
	if isRunningService {
		runService()
		return
	}

	// Print help text when there are no arguments
	if len(osArgs) == 0 && logger.GetTerminalInfo(os.Stdin).IsTTY {
		fmt.Fprintf(os.Stderr, "%s\n", helpText)
		os.Exit(0)
	}

	// Capture the defer statements below so the "done" message comes last
	exitCode := 1
	func() {
		// To view a CPU trace, use "go tool trace [file]". Note that the trace
		// viewer doesn't work under Windows Subsystem for Linux for some reason.
		if traceFile != "" {
			f, err := os.Create(traceFile)
			if err != nil {
				logger.PrintErrorToStderr(osArgs, fmt.Sprintf(
					"Failed to create trace file: %s", err.Error()))
				return
			}
			defer f.Close()
			trace.Start(f)
			defer trace.Stop()
		}

		// To view a heap trace, use "go tool pprof [file]" and type "top". You can
		// also drop it into https://speedscope.app and use the "left heavy" or
		// "sandwich" view modes.
		if heapFile != "" {
			f, err := os.Create(heapFile)
			if err != nil {
				logger.PrintErrorToStderr(osArgs, fmt.Sprintf(
					"Failed to create heap file: %s", err.Error()))
				return
			}
			defer func() {
				if err := pprof.WriteHeapProfile(f); err != nil {
					logger.PrintErrorToStderr(osArgs, fmt.Sprintf(
						"Failed to write heap profile: %s", err.Error()))
				}
				f.Close()
			}()
		}

		// To view a CPU profile, drop the file into https://speedscope.app.
		// Note: Running the CPU profiler doesn't work under Windows subsystem for
		// Linux. The profiler has to be built for native Windows and run using the
		// command prompt instead.
		if cpuprofileFile != "" {
			f, err := os.Create(cpuprofileFile)
			if err != nil {
				logger.PrintErrorToStderr(osArgs, fmt.Sprintf(
					"Failed to create cpuprofile file: %s", err.Error()))
				return
			}
			defer f.Close()
			pprof.StartCPUProfile(f)
			defer pprof.StopCPUProfile()
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
			// Disable the GC since we're just going to allocate a bunch of memory
			// and then exit anyway. This speedup is not insignificant. Make sure to
			// only do this here once we know that we're not going to be a long-lived
			// process though.
			debug.SetGCPercent(-1)

			exitCode = cli.Run(osArgs)
		}
	}()

	os.Exit(exitCode)
}
