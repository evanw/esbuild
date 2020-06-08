package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime/debug"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"strings"
	"time"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/bundler"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/lexer"
	"github.com/evanw/esbuild/internal/logging"
	"github.com/evanw/esbuild/internal/parser"
	"github.com/evanw/esbuild/internal/printer"
	"github.com/evanw/esbuild/internal/resolver"
)

const helpText = `
Usage:
  esbuild [options] [entry points]

Options:
  --name=...            The name of the module
  --bundle              Bundle all dependencies into the output files
  --outfile=...         The output file (for one entry point)
  --outdir=...          The output directory (for multiple entry points)
  --sourcemap           Emit a source map
  --target=...          Language target (default esnext)
  --platform=...        Platform target (browser or node, default browser)
  --external:M          Exclude module M from the bundle
  --format=...          Output format (iife, cjs, esm)
  --origindir=...       Resolve imports starting with / relative to directory
  --color=...           Force use of color terminal escapes (true or false)

  --minify              Sets all --minify-* flags
  --minify-whitespace   Remove whitespace
  --minify-identifiers  Shorten identifiers
  --minify-syntax       Use equivalent but shorter syntax

  --define:K=V          Substitute K with V while parsing
  --jsx-factory=...     What to use instead of React.createElement
  --jsx-fragment=...    What to use instead of React.Fragment
  --loader:X=L          Use loader L to load file extension X, where L is
                        one of: js, jsx, ts, tsx, json, text, base64, file, dataurl

Advanced options:
  --version                 Print the current version and exit (` + esbuildVersion + `)
  --sourcemap=inline        Emit the source map with an inline data URL
  --sourcemap=external      Do not link to the source map with a comment
  --sourcefile=...          Set the source file for the source map (for stdin)
  --error-limit=...         Maximum error count or 0 to disable (default 10)
  --log-level=...           Disable logging (info, warning, error)
  --resolve-extensions=...  A comma-separated list of implicit extensions
  --metafile=...            Write metadata about the build to a JSON file

  --trace=...           Write a CPU trace to this file
  --cpuprofile=...      Write a CPU profile to this file

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

type argsObject struct {
	traceFile      string
	cpuprofileFile string
	rawDefines     map[string]parser.DefineFunc
	parseOptions   parser.ParseOptions
	bundleOptions  bundler.BundleOptions
	resolveOptions resolver.ResolveOptions
	logOptions     logging.StderrOptions
	entryPaths     []string
}

func (args argsObject) logInfo(text string) {
	if args.logOptions.LogLevel <= logging.LevelInfo {
		fmt.Fprintf(os.Stderr, "%s\n", text)
	}
}

func exitWithError(text string) {
	colorRed := ""
	colorBold := ""
	colorReset := ""

	if logging.GetTerminalInfo(os.Stderr).UseColorEscapes {
		colorRed = "\033[1;31m"
		colorBold = "\033[0;1m"
		colorReset = "\033[0m"
	}

	fmt.Fprintf(os.Stderr, "%serror: %s%s%s\n", colorRed, colorBold, text, colorReset)
	os.Exit(1)
}

func (args *argsObject) parseDefine(key string, value string) bool {
	// The key must be a dot-separated identifier list
	for _, part := range strings.Split(key, ".") {
		if !lexer.IsIdentifier(part) {
			return false
		}
	}

	// Lazily create the defines map
	if args.rawDefines == nil {
		args.rawDefines = make(map[string]parser.DefineFunc)
	}

	// Allow substituting for an identifier
	if lexer.IsIdentifier(value) {
		if _, ok := lexer.Keywords()[value]; !ok {
			args.rawDefines[key] = func(findSymbol parser.FindSymbol) ast.E {
				return &ast.EIdentifier{findSymbol(value)}
			}
			return true
		}
	}

	// Parse the value as JSON
	log, done := logging.NewDeferLog()
	source := logging.Source{Contents: value}
	expr, ok := parser.ParseJSON(log, source, parser.ParseJSONOptions{})
	done()
	if !ok {
		return false
	}

	// Only allow atoms for now
	var fn parser.DefineFunc
	switch e := expr.Data.(type) {
	case *ast.ENull:
		fn = func(parser.FindSymbol) ast.E { return &ast.ENull{} }
	case *ast.EBoolean:
		fn = func(parser.FindSymbol) ast.E { return &ast.EBoolean{e.Value} }
	case *ast.EString:
		fn = func(parser.FindSymbol) ast.E { return &ast.EString{e.Value} }
	case *ast.ENumber:
		fn = func(parser.FindSymbol) ast.E { return &ast.ENumber{e.Value} }
	default:
		return false
	}

	args.rawDefines[key] = fn
	return true
}

func (args *argsObject) parseLoader(text string) bundler.Loader {
	switch text {
	case "js":
		return bundler.LoaderJS
	case "jsx":
		return bundler.LoaderJSX
	case "ts":
		return bundler.LoaderTS
	case "tsx":
		return bundler.LoaderTSX
	case "json":
		return bundler.LoaderJSON
	case "text":
		return bundler.LoaderText
	case "base64":
		return bundler.LoaderBase64
	case "dataurl":
		return bundler.LoaderDataURL
	case "file":
		return bundler.LoaderFile
	default:
		return bundler.LoaderNone
	}
}

func (args *argsObject) parseMemberExpression(text string) ([]string, bool) {
	parts := strings.Split(text, ".")

	for _, part := range parts {
		if !lexer.IsIdentifier(part) {
			return parts, false
		}
	}

	return parts, true
}

func parseArgs(fs fs.FS, rawArgs []string) (argsObject, error) {
	args := argsObject{
		bundleOptions: bundler.BundleOptions{
			ExtensionToLoader: bundler.DefaultExtensionToLoaderMap(),
		},
		resolveOptions: resolver.ResolveOptions{
			ExtensionOrder:  []string{".tsx", ".ts", ".jsx", ".mjs", ".cjs", ".js", ".json"},
			ExternalModules: make(map[string]bool),
		},
		logOptions: logging.StderrOptions{
			IncludeSource:      true,
			ErrorLimit:         10,
			ExitWhenLimitIsHit: true,
		},
	}

	for _, arg := range rawArgs {
		switch {
		case arg == "--bundle":
			args.parseOptions.IsBundling = true
			args.bundleOptions.IsBundling = true

		case arg == "--minify":
			args.parseOptions.MangleSyntax = true
			args.bundleOptions.MangleSyntax = true
			args.bundleOptions.RemoveWhitespace = true
			args.bundleOptions.MinifyIdentifiers = true

		case arg == "--minify-syntax":
			args.parseOptions.MangleSyntax = true
			args.bundleOptions.MangleSyntax = true

		case arg == "--minify-whitespace":
			args.bundleOptions.RemoveWhitespace = true

		case arg == "--minify-identifiers":
			args.bundleOptions.MinifyIdentifiers = true

		case arg == "--sourcemap":
			args.bundleOptions.SourceMap = bundler.SourceMapLinkedWithComment

		case arg == "--sourcemap=external":
			args.bundleOptions.SourceMap = bundler.SourceMapExternalWithoutComment

		case arg == "--sourcemap=inline":
			args.bundleOptions.SourceMap = bundler.SourceMapInline

		case strings.HasPrefix(arg, "--sourcefile="):
			args.bundleOptions.SourceFile = arg[len("--sourcefile="):]

		case strings.HasPrefix(arg, "--resolve-extensions="):
			extensions := strings.Split(arg[len("--resolve-extensions="):], ",")
			for _, ext := range extensions {
				if !strings.HasPrefix(ext, ".") {
					return argsObject{}, fmt.Errorf("Invalid extension: %q", ext)
				}
			}
			args.resolveOptions.ExtensionOrder = extensions

		case strings.HasPrefix(arg, "--error-limit="):
			value, err := strconv.Atoi(arg[len("--error-limit="):])
			if err != nil {
				return argsObject{}, fmt.Errorf("Invalid error limit: %s", arg)
			}
			args.logOptions.ErrorLimit = value

		case strings.HasPrefix(arg, "--name="):
			value := arg[len("--name="):]
			if !lexer.IsIdentifier(value) {
				return argsObject{}, fmt.Errorf("Invalid name: %s", arg)
			}
			args.bundleOptions.ModuleName = value

		case strings.HasPrefix(arg, "--metafile="):
			value := arg[len("--metafile="):]
			file, ok := fs.Abs(value)
			if !ok {
				return argsObject{}, fmt.Errorf("Invalid metadata file: %s", arg)
			}
			args.bundleOptions.AbsMetadataFile = file

		case strings.HasPrefix(arg, "--outfile="):
			value := arg[len("--outfile="):]
			file, ok := fs.Abs(value)
			if !ok {
				return argsObject{}, fmt.Errorf("Invalid output file: %s", arg)
			}
			args.bundleOptions.AbsOutputFile = file

		case strings.HasPrefix(arg, "--outdir="):
			value := arg[len("--outdir="):]
			dir, ok := fs.Abs(value)
			if !ok {
				return argsObject{}, fmt.Errorf("Invalid output directory: %s", arg)
			}
			args.bundleOptions.AbsOutputDir = dir

		case strings.HasPrefix(arg, "--origindir="):
			value := arg[len("--origindir="):]
			dir, ok := fs.Abs(value)
			if !ok {
				return argsObject{}, fmt.Errorf("Invalid bundle origin directory: %s", arg)
			}
			args.resolveOptions.OriginDir = dir

		case strings.HasPrefix(arg, "--define:"):
			text := arg[len("--define:"):]
			equals := strings.IndexByte(text, '=')
			if equals == -1 {
				return argsObject{}, fmt.Errorf("Missing \"=\": %s", arg)
			}
			if !args.parseDefine(text[:equals], text[equals+1:]) {
				return argsObject{}, fmt.Errorf("Invalid define: %s", arg)
			}

		case strings.HasPrefix(arg, "--loader:"):
			text := arg[len("--loader:"):]
			equals := strings.IndexByte(text, '=')
			if equals == -1 {
				return argsObject{}, fmt.Errorf("Missing \"=\": %s", arg)
			}
			extension, loader := text[:equals], text[equals+1:]
			if !strings.HasPrefix(extension, ".") {
				return argsObject{}, fmt.Errorf("File extension must start with \".\": %s", arg)
			}
			if len(extension) < 2 || strings.ContainsRune(extension[1:], '.') {
				return argsObject{}, fmt.Errorf("Invalid file extension: %s", arg)
			}
			parsedLoader := args.parseLoader(loader)
			if parsedLoader == bundler.LoaderNone {
				return argsObject{}, fmt.Errorf("Invalid loader: %s", arg)
			} else {
				args.bundleOptions.ExtensionToLoader[extension] = parsedLoader
			}

		case strings.HasPrefix(arg, "--loader="):
			loader := arg[len("--loader="):]
			parsedLoader := args.parseLoader(loader)
			switch parsedLoader {
			// Forbid the "file" loader with stdin
			case bundler.LoaderNone, bundler.LoaderFile:
				return argsObject{}, fmt.Errorf("Invalid loader: %s", arg)
			default:
				args.bundleOptions.LoaderForStdin = parsedLoader
			}

		case strings.HasPrefix(arg, "--target="):
			switch arg[len("--target="):] {
			case "esnext":
				args.parseOptions.Target = parser.ESNext
			case "es6", "es2015":
				args.parseOptions.Target = parser.ES2015
			case "es2016":
				args.parseOptions.Target = parser.ES2016
			case "es2017":
				args.parseOptions.Target = parser.ES2017
			case "es2018":
				args.parseOptions.Target = parser.ES2018
			case "es2019":
				args.parseOptions.Target = parser.ES2019
			case "es2020":
				args.parseOptions.Target = parser.ES2020
			default:
				return argsObject{}, fmt.Errorf("Valid targets: es6, es2015, es2016, es2017, es2018, es2019, es2020, esnext")
			}

		case strings.HasPrefix(arg, "--platform="):
			switch arg[len("--platform="):] {
			case "browser":
				args.resolveOptions.Platform = resolver.PlatformBrowser
			case "node":
				args.resolveOptions.Platform = resolver.PlatformNode
			default:
				return argsObject{}, fmt.Errorf("Valid platforms: browser, node")
			}

		case strings.HasPrefix(arg, "--format="):
			switch arg[len("--format="):] {
			case "iife":
				args.bundleOptions.OutputFormat = printer.FormatIIFE
			case "cjs":
				args.bundleOptions.OutputFormat = printer.FormatCommonJS
			case "esm":
				args.bundleOptions.OutputFormat = printer.FormatESModule
			default:
				return argsObject{}, fmt.Errorf("Valid formats: iife, cjs, esm")
			}

		case strings.HasPrefix(arg, "--color="):
			switch arg[len("--color="):] {
			case "false":
				args.logOptions.Color = logging.ColorNever
			case "true":
				args.logOptions.Color = logging.ColorAlways
			default:
				return argsObject{}, fmt.Errorf("Valid values for color: false, true")
			}

		case strings.HasPrefix(arg, "--external:"):
			path := arg[len("--external:"):]
			if resolver.IsNonModulePath(path) {
				return argsObject{}, fmt.Errorf("Invalid module name: %s", arg)
			}
			args.resolveOptions.ExternalModules[path] = true

		case strings.HasPrefix(arg, "--jsx-factory="):
			if parts, ok := args.parseMemberExpression(arg[len("--jsx-factory="):]); ok {
				args.parseOptions.JSX.Factory = parts
			} else {
				return argsObject{}, fmt.Errorf("Invalid JSX factory: %s", arg)
			}

		case strings.HasPrefix(arg, "--jsx-fragment="):
			if parts, ok := args.parseMemberExpression(arg[len("--jsx-fragment="):]); ok {
				args.parseOptions.JSX.Fragment = parts
			} else {
				return argsObject{}, fmt.Errorf("Invalid JSX fragment: %s", arg)
			}

		case strings.HasPrefix(arg, "--log-level="):
			switch arg[len("--log-level="):] {
			case "info":
				args.logOptions.LogLevel = logging.LevelInfo
			case "warning":
				args.logOptions.LogLevel = logging.LevelWarning
			case "error":
				args.logOptions.LogLevel = logging.LevelError
			default:
				return argsObject{}, fmt.Errorf("Invalid log level: %s", arg)
			}

		case strings.HasPrefix(arg, "--trace="):
			args.traceFile = arg[len("--trace="):]

		case strings.HasPrefix(arg, "--cpuprofile="):
			args.cpuprofileFile = arg[len("--cpuprofile="):]

		case strings.HasPrefix(arg, "-"):
			return argsObject{}, fmt.Errorf("Invalid flag: %s", arg)

		default:
			arg, ok := fs.Abs(arg)
			if !ok {
				return argsObject{}, fmt.Errorf("Invalid path: %s", arg)
			}
			args.entryPaths = append(args.entryPaths, arg)
		}
	}

	if args.bundleOptions.AbsOutputDir == "" && len(args.entryPaths) > 1 {
		return argsObject{}, fmt.Errorf("Must provide --outdir when there are multiple input files")
	}

	if args.bundleOptions.AbsOutputFile != "" && args.bundleOptions.AbsOutputDir != "" {
		return argsObject{}, fmt.Errorf("Cannot use both --outfile and --outdir")
	}

	if args.bundleOptions.AbsOutputFile != "" {
		// If the output file is specified, use it to derive the output directory
		args.bundleOptions.AbsOutputDir = fs.Dir(args.bundleOptions.AbsOutputFile)
	}

	// Disallow bundle-only options when not bundling
	if !args.bundleOptions.IsBundling {
		if args.bundleOptions.OutputFormat != printer.FormatPreserve {
			return argsObject{}, fmt.Errorf("Cannot use --format without --bundle")
		}

		if len(args.resolveOptions.ExternalModules) > 0 {
			return argsObject{}, fmt.Errorf("Cannot use --external without --bundle")
		}

		if args.resolveOptions.OriginDir != "" {
			return argsObject{}, fmt.Errorf("Cannot use --origindir without --bundle")
		}
	}

	if args.bundleOptions.IsBundling && args.bundleOptions.OutputFormat == printer.FormatPreserve {
		// If the format isn't specified, set the default format using the platform
		switch args.resolveOptions.Platform {
		case resolver.PlatformBrowser:
			args.bundleOptions.OutputFormat = printer.FormatIIFE
		case resolver.PlatformNode:
			args.bundleOptions.OutputFormat = printer.FormatCommonJS
		}
	}

	if len(args.entryPaths) > 0 {
		// Disallow the "--loader=" form when not reading from stdin
		if args.bundleOptions.LoaderForStdin != bundler.LoaderNone {
			return argsObject{}, fmt.Errorf("Must provide file extension for --loader")
		}

		// Write to stdout by default if there's only one input file
		if len(args.entryPaths) == 1 && args.bundleOptions.AbsOutputFile == "" && args.bundleOptions.AbsOutputDir == "" {
			args.bundleOptions.WriteToStdout = true
		}
	} else if !logging.GetTerminalInfo(os.Stdin).IsTTY {
		// If called with no input files and we're not a TTY, read from stdin instead
		args.entryPaths = append(args.entryPaths, "<stdin>")

		// Default to reading JavaScript from stdin
		if args.bundleOptions.LoaderForStdin == bundler.LoaderNone {
			args.bundleOptions.LoaderForStdin = bundler.LoaderJS
		}

		// Write to stdout if no input file is provided
		if args.bundleOptions.AbsOutputFile == "" {
			if args.bundleOptions.AbsOutputDir != "" {
				return argsObject{}, fmt.Errorf("Cannot use --outdir when reading from stdin")
			}
			args.bundleOptions.WriteToStdout = true
		}
	}

	// Change the default value for some settings if we're writing to stdout
	if args.bundleOptions.WriteToStdout {
		if args.bundleOptions.SourceMap != bundler.SourceMapNone {
			args.bundleOptions.SourceMap = bundler.SourceMapInline
		}
		if args.logOptions.LogLevel == logging.LevelNone {
			args.logOptions.LogLevel = logging.LevelWarning
		}
		if args.bundleOptions.AbsMetadataFile != "" {
			return argsObject{}, fmt.Errorf("Cannot generate metadata when writing to stdout")
		}

		// Forbid the "file" loader since stdout only allows one output file
		for _, loader := range args.bundleOptions.ExtensionToLoader {
			if loader == bundler.LoaderFile {
				return argsObject{}, fmt.Errorf("Cannot use the \"file\" loader when writing to stdout")
			}
		}
	}

	// Processing defines is expensive. Process them once here so the same object
	// can be shared between all parsers we create using these arguments.
	processedDefines := parser.ProcessDefines(args.rawDefines)
	args.parseOptions.Defines = &processedDefines
	return args, nil
}

func main() {
	for _, arg := range os.Args {
		// Show help if a common help flag is provided
		if arg == "-h" || arg == "-help" || arg == "--help" || arg == "/?" {
			fmt.Fprintf(os.Stderr, "%s", helpText)
			os.Exit(0)
		}

		// Special-case the version flag here
		if arg == "--version" {
			fmt.Fprintf(os.Stderr, "%s\n", esbuildVersion)
			os.Exit(0)
		}

		// This flag turns the process into a long-running service that uses
		// message passing with the host process over stdin/stdout
		if arg == "--service" {
			runService()
			return
		}
	}

	start := time.Now()
	fs := fs.RealFS()
	args, err := parseArgs(fs, os.Args[1:])
	if err != nil {
		exitWithError(err.Error())
	}

	// Handle when there are no input files (including implicit stdin)
	if len(args.entryPaths) == 0 {
		if len(os.Args) < 2 {
			fmt.Fprintf(os.Stderr, "%s", helpText)
			os.Exit(0)
		} else {
			exitWithError("No input files")
		}
	}

	// Capture the defer statements below so the "done" message comes last
	func() {
		// To view a CPU trace, use "go tool trace [file]". Note that the trace
		// viewer doesn't work under Windows Subsystem for Linux for some reason.
		if args.traceFile != "" {
			f, err := os.Create(args.traceFile)
			if err != nil {
				exitWithError(fmt.Sprintf("Failed to create a file called '%s': %s", args.traceFile, err.Error()))
			}
			defer func() {
				f.Close()
				args.logInfo(fmt.Sprintf("Wrote to %s", args.traceFile))
			}()
			trace.Start(f)
			defer trace.Stop()
		}

		// To view a CPU profile, drop the file into https://speedscope.app.
		// Note: Running the CPU profiler doesn't work under Windows subsystem for
		// Linux. The profiler has to be built for native Windows and run using the
		// command prompt instead.
		if args.cpuprofileFile != "" {
			f, err := os.Create(args.cpuprofileFile)
			if err != nil {
				exitWithError(fmt.Sprintf("Failed to create a file called '%s': %s", args.cpuprofileFile, err.Error()))
			}
			defer func() {
				f.Close()
				args.logInfo(fmt.Sprintf("Wrote to %s", args.cpuprofileFile))
			}()
			pprof.StartCPUProfile(f)
			defer pprof.StopCPUProfile()
		}

		if args.cpuprofileFile != "" {
			// The CPU profiler in Go only runs at 100 Hz, which is far too slow to
			// return useful information for esbuild, since it's so fast. Let's keep
			// running for 30 seconds straight, which should give us 3,000 samples.
			seconds := 30.0
			args.logInfo(fmt.Sprintf("Running for %g seconds straight due to --cpuprofile...", seconds))
			for time.Since(start).Seconds() < seconds {
				run(fs, args)
			}
		} else {
			// Disable the GC since we're just going to allocate a bunch of memory
			// and then exit anyway. This speedup is not insignificant. Make sure to
			// only do this here once we know that we're not going to be a long-lived
			// process though.
			debug.SetGCPercent(-1)

			run(fs, args)
		}
	}()

	args.logInfo(fmt.Sprintf("Done in %dms", time.Since(start).Nanoseconds()/1000000))
}

func run(fs fs.FS, args argsObject) {
	// Parse all files in the bundle
	log, join := logging.NewStderrLog(args.logOptions)
	resolver := resolver.NewResolver(fs, log, args.resolveOptions)
	bundle := bundler.ScanBundle(log, fs, resolver, args.entryPaths, args.parseOptions, args.bundleOptions)

	// Stop now if there were errors
	if join().Errors != 0 {
		os.Exit(1)
	}

	// Generate the results
	log2, join2 := logging.NewStderrLog(args.logOptions)
	result := bundle.Compile(log2, args.bundleOptions)

	// Stop now if there were errors
	if join2().Errors != 0 {
		os.Exit(1)
	}

	// Create the output directory
	if args.bundleOptions.AbsOutputDir != "" {
		if err := os.MkdirAll(args.bundleOptions.AbsOutputDir, 0755); err != nil {
			exitWithError(fmt.Sprintf("Cannot create output directory: %s", err))
		}
	}

	// Write out the results
	for _, item := range result {
		// Special-case writing to stdout
		if args.bundleOptions.WriteToStdout {
			_, err := os.Stdout.Write(item.Contents)
			if err != nil {
				exitWithError(fmt.Sprintf("Failed to write to stdout: %s", err.Error()))
			}
			continue
		}

		// Write out the file
		err := ioutil.WriteFile(item.AbsPath, []byte(item.Contents), 0644)
		path := resolver.PrettyPath(item.AbsPath)
		if err != nil {
			exitWithError(fmt.Sprintf("Failed to write to %s (%s)", path, err.Error()))
		}
		args.logInfo(fmt.Sprintf("Wrote to %s (%s)", path, toSize(len(item.Contents))))
	}
}

func toSize(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d bytes", bytes)
	}

	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1fkb", float32(bytes)/float32(1024))
	}

	return fmt.Sprintf("%.1fmb", float32(bytes)/float32(1024*1024))
}
