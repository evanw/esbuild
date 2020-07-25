package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"runtime/pprof"
	"runtime/trace"
	"strings"
	"time"

	"github.com/evanw/esbuild/internal/logging"
	"github.com/evanw/esbuild/pkg/cli"
)

const helpText = `
Usage:
  esbuild [options] [entry points]

Options:
  --bundle              Bundle all dependencies into the output files
  --outfile=...         The output file (for one entry point)
  --outdir=...          The output directory (for multiple entry points)
  --sourcemap           Emit a source map
  --target=...          Environment target (e.g. es2017, chrome80)
  --platform=...        Platform target (browser or node, default browser)
  --external:M          Exclude module M from the bundle
  --format=...          Output format (iife, cjs, esm)
  --splitting           Enable code splitting (currently only for esm)
  --color=...           Force use of color terminal escapes (true or false)
  --global-name=...     The name of the global for the IIFE format

  --minify              Sets all --minify-* flags
  --minify-whitespace   Remove whitespace
  --minify-identifiers  Shorten identifiers
  --minify-syntax       Use equivalent but shorter syntax

  --define:K=V          Substitute K with V while parsing
  --jsx-factory=...     What to use instead of React.createElement
  --jsx-fragment=...    What to use instead of React.Fragment
  --loader:X=L          Use loader L to load file extension X, where L is
                        one of: js, jsx, ts, tsx, json, text, base64, file,
                        dataurl, binary

Advanced options:
  --version                 Print the current version and exit (` + esbuildVersion + `)
  --sourcemap=inline        Emit the source map with an inline data URL
  --sourcemap=external      Do not link to the source map with a comment
  --sourcefile=...          Set the source file for the source map (for stdin)
  --error-limit=...         Maximum error count or 0 to disable (default 10)
  --log-level=...           Disable logging (info, warning, error, silent)
  --resolve-extensions=...  A comma-separated list of implicit extensions
  --metafile=...            Write metadata about the build to a JSON file
  --strict                  Transforms handle edge cases but have more overhead
  --pure=N                  Mark the name N as a pure function for tree shaking
  --tsconfig=...            Use this tsconfig.json file instead of other ones
  --out-extension:.js=.mjs  Use a custom output extension instead of ".js"

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
			fmt.Fprintf(os.Stderr, "%s\n", esbuildVersion)
			os.Exit(0)

		case strings.HasPrefix(arg, "--trace="):
			traceFile = arg[len("--trace="):]

		case strings.HasPrefix(arg, "--cpuprofile="):
			cpuprofileFile = arg[len("--cpuprofile="):]

		// This flag turns the process into a long-running service that uses
		// message passing with the host process over stdin/stdout
		case arg == "--service":
			isRunningService = true

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
	if len(osArgs) == 0 && logging.GetTerminalInfo(os.Stdin).IsTTY {
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
				logging.PrintErrorToStderr(osArgs, fmt.Sprintf(
					"Failed to create trace file: %s", err.Error()))
				return
			}
			defer f.Close()
			trace.Start(f)
			defer trace.Stop()
		}

		// To view a CPU profile, drop the file into https://speedscope.app.
		// Note: Running the CPU profiler doesn't work under Windows subsystem for
		// Linux. The profiler has to be built for native Windows and run using the
		// command prompt instead.
		if cpuprofileFile != "" {
			f, err := os.Create(cpuprofileFile)
			if err != nil {
				logging.PrintErrorToStderr(osArgs, fmt.Sprintf(
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
