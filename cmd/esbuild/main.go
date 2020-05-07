package main

import (
	"fmt"
	"io/ioutil"
	"os"
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
	"github.com/evanw/esbuild/internal/resolver"
)

type argsObject struct {
	traceFile      string
	cpuprofileFile string
	parseOptions   parser.ParseOptions
	bundleOptions  bundler.BundleOptions
	resolveOptions resolver.ResolveOptions
	logOptions     logging.StderrOptions
	entryPaths     []string
}

func exitWithError(text string) {
	colorRed := ""
	colorBold := ""
	colorReset := ""

	if logging.StderrTerminalInfo().UseColorEscapes {
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

	// Allow substituting for an identifier
	if lexer.IsIdentifier(value) {
		if _, ok := lexer.Keywords()[value]; !ok {
			args.parseOptions.Defines[key] = func(helper parser.DefineHelper) ast.E {
				return &ast.EIdentifier{helper.FindSymbol(value)}
			}
			return true
		}
	}

	// Parse the value as JSON
	log, done := logging.NewDeferLog()
	source := logging.Source{Contents: value}
	expr, ok := parser.ParseJSON(log, source)
	done()
	if !ok {
		return false
	}

	// Only allow atoms for now
	var fn parser.DefineFunc
	switch e := expr.Data.(type) {
	case *ast.ENull:
		fn = func(parser.DefineHelper) ast.E { return &ast.ENull{} }
	case *ast.EBoolean:
		fn = func(parser.DefineHelper) ast.E { return &ast.EBoolean{e.Value} }
	case *ast.EString:
		fn = func(parser.DefineHelper) ast.E { return &ast.EString{e.Value} }
	case *ast.ENumber:
		fn = func(parser.DefineHelper) ast.E { return &ast.ENumber{e.Value} }
	default:
		return false
	}

	args.parseOptions.Defines[key] = fn
	return true
}

func (args *argsObject) parseLoader(key string, value string) bool {
	var loader bundler.Loader

	switch value {
	case "js":
		loader = bundler.LoaderJS
	case "jsx":
		loader = bundler.LoaderJSX
	case "ts":
		loader = bundler.LoaderTS
	case "tsx":
		loader = bundler.LoaderTSX
	case "json":
		loader = bundler.LoaderJSON
	case "text":
		loader = bundler.LoaderText
	case "base64":
		loader = bundler.LoaderBase64
	default:
		return false
	}

	args.bundleOptions.ExtensionToLoader[key] = loader
	return true
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
		parseOptions: parser.ParseOptions{
			Defines: make(map[string]parser.DefineFunc),
		},
		bundleOptions: bundler.BundleOptions{
			ExtensionToLoader: bundler.DefaultExtensionToLoaderMap(),
		},
		resolveOptions: resolver.ResolveOptions{
			ExtensionOrder:  []string{".tsx", ".ts", ".jsx", ".js", ".json"},
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
			args.bundleOptions.SourceMap = true

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
			if !args.parseLoader(extension, loader) {
				return argsObject{}, fmt.Errorf("Invalid loader: %s", arg)
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
				args.bundleOptions.OutputFormat = bundler.FormatIIFE
			case "cjs":
				args.bundleOptions.OutputFormat = bundler.FormatCommonJS
			default:
				return argsObject{}, fmt.Errorf("Valid formats: iife, cjs")
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

	if args.bundleOptions.AbsOutputFile != "" && len(args.entryPaths) > 1 {
		return argsObject{}, fmt.Errorf("Use --outdir instead of --outfile when there are multiple entry points")
	}

	if args.bundleOptions.AbsOutputFile != "" && args.bundleOptions.AbsOutputDir != "" {
		return argsObject{}, fmt.Errorf("Cannot use both --outfile and --outdir")
	}

	if args.bundleOptions.AbsOutputFile != "" {
		// If the output file is specified, use it to derive the output directory
		args.bundleOptions.AbsOutputDir = fs.Dir(args.bundleOptions.AbsOutputFile)
	}

	if args.bundleOptions.OutputFormat == bundler.FormatNone {
		// If the format isn't specified, set the default format using the platform
		switch args.resolveOptions.Platform {
		case resolver.PlatformBrowser:
			args.bundleOptions.OutputFormat = bundler.FormatIIFE
		case resolver.PlatformNode:
			args.bundleOptions.OutputFormat = bundler.FormatCommonJS
		}
	}

	return args, nil
}

func main() {
	// Show usage information if called with no arguments
	showHelp := len(os.Args) < 2

	for _, arg := range os.Args {
		// Show help if a common help flag is provided
		if arg == "-h" || arg == "-help" || arg == "--help" || arg == "/?" {
			showHelp = true
			break
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

	// Show help and exit if requested
	if showHelp {
		fmt.Print(`
Usage:
  esbuild [options] [entry points]

Options:
  --name=...            The name of the module
  --bundle              Bundle all dependencies into the output files
  --outfile=...         The output file (for one entry point)
  --outdir=...          The output directory (for multiple entry points)
  --sourcemap           Emit a source map
  --error-limit=...     Maximum error count or 0 to disable (default 10)
  --target=...          Language target (default esnext)
  --platform=...        Platform target (browser or node, default browser)
  --external:M          Exclude module M from the bundle
  --format=...          Output format (iife or cjs)
  --color=...           Force use of color terminal escapes (true or false)

  --minify              Sets all --minify-* flags
  --minify-whitespace   Remove whitespace
  --minify-identifiers  Shorten identifiers
  --minify-syntax       Use equivalent but shorter syntax

  --define:K=V          Substitute K with V while parsing
  --jsx-factory=...     What to use instead of React.createElement
  --jsx-fragment=...    What to use instead of React.Fragment
  --loader:X=L          Use loader L to load file extension X, where L is
                        one of: js, jsx, ts, tsx, json, text, base64

  --trace=...           Write a CPU trace to this file
  --cpuprofile=...      Write a CPU profile to this file
  --version             Print the current version and exit (` + esbuildVersion + `)

Examples:
  # Produces dist/entry_point.js and dist/entry_point.js.map
  esbuild --bundle entry_point.js --outdir=dist --minify --sourcemap

  # Allow JSX syntax in .js files
  esbuild --bundle entry_point.js --outfile=out.js --loader:.js=jsx

  # Substitute the identifier RELEASE for the literal true
  esbuild example.js --outfile=out.js --define:RELEASE=true

`)
		os.Exit(0)
	}

	start := time.Now()
	fs := fs.RealFS()
	args, err := parseArgs(fs, os.Args[1:])
	if err != nil {
		exitWithError(err.Error())
	}

	// Show usage information if called with no files
	if len(args.entryPaths) == 0 {
		exitWithError("No files specified")
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
				fmt.Fprintf(os.Stderr, "Wrote to %s\n", args.traceFile)
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
				fmt.Fprintf(os.Stderr, "Wrote to %s\n", args.cpuprofileFile)
			}()
			pprof.StartCPUProfile(f)
			defer pprof.StopCPUProfile()
		}

		if args.cpuprofileFile != "" {
			// The CPU profiler in Go only runs at 100 Hz, which is far too slow to
			// return useful information for esbuild, since it's so fast. Let's keep
			// running for 30 seconds straight, which should give us 3,000 samples.
			seconds := 30.0
			fmt.Fprintf(os.Stderr, "Running for %g seconds straight due to --cpuprofile...\n", seconds)
			for time.Since(start).Seconds() < seconds {
				run(fs, args)
			}
		} else {
			run(fs, args)
		}
	}()

	fmt.Fprintf(os.Stderr, "Done in %dms\n", time.Since(start).Nanoseconds()/1000000)
}

func run(fs fs.FS, args argsObject) {
	// Parse all files in the bundle
	resolver := resolver.NewResolver(fs, args.resolveOptions)
	log, join := logging.NewStderrLog(args.logOptions)
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
		// Write out the JavaScript file
		err := ioutil.WriteFile(item.JsAbsPath, []byte(item.JsContents), 0644)
		path := resolver.PrettyPath(item.JsAbsPath)
		if err != nil {
			exitWithError(fmt.Sprintf("Failed to write to %s (%s)", path, err.Error()))
		}
		fmt.Fprintf(os.Stderr, "Wrote to %s (%s)\n", path, toSize(len(item.JsContents)))

		// Also write the source map
		if args.bundleOptions.SourceMap {
			err := ioutil.WriteFile(item.SourceMapAbsPath, item.SourceMapContents, 0644)
			path := resolver.PrettyPath(item.SourceMapAbsPath)
			if err != nil {
				exitWithError(fmt.Sprintf("Failed to write to %s: (%s)", path, err.Error()))
			}
			fmt.Fprintf(os.Stderr, "Wrote to %s (%s)\n", path, toSize(len(item.SourceMapContents)))
		}
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
