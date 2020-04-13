package main

import (
	"esbuild/ast"
	"esbuild/bundler"
	"esbuild/fs"
	"esbuild/lexer"
	"esbuild/logging"
	"esbuild/parser"
	"esbuild/resolver"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"strings"
	"time"
)

type args struct {
	traceFile      string
	cpuprofileFile string
	parseOptions   parser.ParseOptions
	bundleOptions  bundler.BundleOptions
	resolveOptions resolver.ResolveOptions
	logOptions     logging.StderrOptions
	entryPaths     []string
}

func (args *args) exitWithError(text string) {
	colorRed := ""
	colorBold := ""
	colorReset := ""

	if logging.StdinTerminalInfo().UseColor {
		colorRed = "\033[1;31m"
		colorBold = "\033[0;1m"
		colorReset = "\033[0m"
	}

	fmt.Fprintf(os.Stderr, "%serror: %s%s%s\n", colorRed, colorBold, text, colorReset)
	os.Exit(1)
}

func (args *args) parseDefine(key string, value string) bool {
	// The key must be a dot-separated identifier list
	for _, part := range strings.Split(key, ".") {
		if !lexer.IsIdentifier(part) {
			return false
		}
	}

	// Parse the value as JSON
	log, done := logging.NewDeferLog()
	source := logging.Source{Contents: value}
	expr, ok := parser.ParseJson(log, source)
	done()
	if !ok {
		return false
	}

	// Only allow atoms for now
	switch expr.Data.(type) {
	case *ast.ENull, *ast.EBoolean, *ast.EString, *ast.ENumber:
		args.parseOptions.Defines[key] = expr.Data
		return true
	}
	return false
}

func (args *args) parseLoader(key string, value string) bool {
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

func (args *args) parseMemberExpression(text string) ([]string, bool) {
	parts := strings.Split(text, ".")
	for _, part := range parts {
		if !lexer.IsIdentifier(part) {
			return parts, false
		}
	}
	return parts, true
}

func parseArgs() args {
	args := args{
		parseOptions: parser.ParseOptions{
			Defines: make(map[string]ast.E),
		},
		bundleOptions: bundler.BundleOptions{
			ExtensionToLoader: bundler.DefaultExtensionToLoaderMap(),
		},
		resolveOptions: resolver.ResolveOptions{
			ExtensionOrder: []string{".tsx", ".ts", ".jsx", ".js", ".json"},
		},
		logOptions: logging.StderrOptions{
			IncludeSource:      true,
			ErrorLimit:         10,
			ExitWhenLimitIsHit: true,
		},
	}

	// Show usage information if called with no arguments
	showHelp := len(os.Args) < 2

	// Show help if a common help flag is provided
	for _, arg := range os.Args {
		if arg == "-h" || arg == "-help" || arg == "--help" || arg == "/?" {
			showHelp = true
			break
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
  --loader:X=L          Use loader L to load file extension X, where L is
                        one of: js, jsx, ts, tsx, json, text, base64

  --minify              Sets all --minify-* flags
  --minify-whitespace   Remove whitespace
  --minify-identifiers  Shorten identifiers
  --minify-syntax       Use equivalent but shorter syntax

  --define:K=V          Substitute K with V while parsing
  --jsx-factory=...     What to use instead of React.createElement
  --jsx-fragment=...    What to use instead of React.Fragment

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

	for _, arg := range os.Args[1:] {
		switch {
		case arg == "--bundle":
			args.parseOptions.IsBundling = true
			args.bundleOptions.Bundle = true

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

		case arg == "--version":
			fmt.Fprintf(os.Stderr, "%s\n", esbuildVersion)
			os.Exit(0)

		case strings.HasPrefix(arg, "--error-limit="):
			value, err := strconv.Atoi(arg[len("--error-limit="):])
			if err != nil {
				args.exitWithError(fmt.Sprintf("Invalid error limit: %s", arg))
			}
			args.logOptions.ErrorLimit = value

		case strings.HasPrefix(arg, "--name="):
			value := arg[len("--name="):]
			if !lexer.IsIdentifier(value) {
				args.exitWithError(fmt.Sprintf("Invalid name: %s", arg))
			}
			args.bundleOptions.ModuleName = value

		case strings.HasPrefix(arg, "--outfile="):
			value := arg[len("--outfile="):]
			file, err := filepath.Abs(value)
			if err != nil {
				args.exitWithError(fmt.Sprintf("Invalid output file: %s", arg))
			}
			args.bundleOptions.AbsOutputFile = file

		case strings.HasPrefix(arg, "--outdir="):
			value := arg[len("--outdir="):]
			dir, err := filepath.Abs(value)
			if err != nil {
				args.exitWithError(fmt.Sprintf("Invalid output directory: %s", arg))
			}
			args.bundleOptions.AbsOutputDir = dir

		case strings.HasPrefix(arg, "--define:"):
			text := arg[len("--define:"):]
			equals := strings.IndexByte(text, '=')
			if equals == -1 {
				args.exitWithError(fmt.Sprintf("Missing \"=\": %s", arg))
			}
			if !args.parseDefine(text[:equals], text[equals+1:]) {
				args.exitWithError(fmt.Sprintf("Invalid define: %s", arg))
			}

		case strings.HasPrefix(arg, "--loader:"):
			text := arg[len("--loader:"):]
			equals := strings.IndexByte(text, '=')
			if equals == -1 {
				args.exitWithError(fmt.Sprintf("Missing \"=\": %s", arg))
			}
			extension, loader := text[:equals], text[equals+1:]
			if !strings.HasPrefix(extension, ".") {
				args.exitWithError(fmt.Sprintf("File extension must start with \".\": %s", arg))
			}
			if len(extension) < 2 || strings.ContainsRune(extension[1:], '.') {
				args.exitWithError(fmt.Sprintf("Invalid file extension: %s", arg))
			}
			if !args.parseLoader(extension, loader) {
				args.exitWithError(fmt.Sprintf("Invalid loader: %s", arg))
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
				args.exitWithError("Valid targets: es6, es2015, es2016, es2017, es2018, es2019, es2020, esnext")
			}

		case strings.HasPrefix(arg, "--platform="):
			switch arg[len("--platform="):] {
			case "browser":
				args.resolveOptions.Platform = resolver.PlatformBrowser
			case "node":
				args.resolveOptions.Platform = resolver.PlatformNode
			default:
				args.exitWithError("Valid platforms: browser, node")
			}

		case strings.HasPrefix(arg, "--jsx-factory="):
			if parts, ok := args.parseMemberExpression(arg[len("--jsx-factory="):]); ok {
				args.parseOptions.JSX.Factory = parts
			} else {
				args.exitWithError(fmt.Sprintf("Invalid JSX factory: %s", arg))
			}

		case strings.HasPrefix(arg, "--jsx-fragment="):
			if parts, ok := args.parseMemberExpression(arg[len("--jsx-fragment="):]); ok {
				args.parseOptions.JSX.Fragment = parts
			} else {
				args.exitWithError(fmt.Sprintf("Invalid JSX fragment: %s", arg))
			}

		case strings.HasPrefix(arg, "--trace="):
			args.traceFile = arg[len("--trace="):]

		case strings.HasPrefix(arg, "--cpuprofile="):
			args.cpuprofileFile = arg[len("--cpuprofile="):]

		case strings.HasPrefix(arg, "-"):
			args.exitWithError(fmt.Sprintf("Invalid flag: %s", arg))

		default:
			arg, err := filepath.Abs(arg)
			if err != nil {
				args.exitWithError(fmt.Sprintf("Invalid path: %s", arg))
			}
			args.entryPaths = append(args.entryPaths, arg)
		}
	}

	if args.bundleOptions.AbsOutputFile != "" && len(args.entryPaths) > 1 {
		args.exitWithError("Use --outdir instead of --outfile when there are multiple entry points")
	}

	if args.bundleOptions.AbsOutputFile != "" && args.bundleOptions.AbsOutputDir != "" {
		args.exitWithError("Cannot use both --outfile and --outdir")
	}

	if args.bundleOptions.AbsOutputFile != "" {
		// If the output file is specified, use it to derive the output directory
		args.bundleOptions.AbsOutputDir = filepath.Dir(args.bundleOptions.AbsOutputFile)
	}

	return args
}

func main() {
	start := time.Now()
	args := parseArgs()

	// Show usage information if called with no files
	if len(args.entryPaths) == 0 {
		args.exitWithError("No files specified")
	}

	// Capture the defer statements below so the "done" message comes last
	func() {
		// To view a CPU trace, use "go tool trace [file]". Note that the trace
		// viewer doesn't work under Windows Subsystem for Linux for some reason.
		if args.traceFile != "" {
			f, err := os.Create(args.traceFile)
			if err != nil {
				args.exitWithError(fmt.Sprintf("Failed to create a file called '%s': %s", args.traceFile, err.Error()))
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
				args.exitWithError(fmt.Sprintf("Failed to create a file called '%s': %s", args.cpuprofileFile, err.Error()))
			}
			defer func() {
				f.Close()
				fmt.Fprintf(os.Stderr, "Wrote to %s\n", args.cpuprofileFile)
			}()
			pprof.StartCPUProfile(f)
			defer pprof.StopCPUProfile()
		}

		// Parse all files in the bundle
		fs := fs.RealFS()
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
				args.exitWithError(fmt.Sprintf("Cannot create output directory: %s", err))
			}
		}

		// Write out the results
		for _, item := range result {
			// Write out the JavaScript file
			err := ioutil.WriteFile(item.JsAbsPath, []byte(item.JsContents), 0644)
			path := resolver.PrettyPath(item.JsAbsPath)
			if err != nil {
				args.exitWithError(fmt.Sprintf("Failed to write to %s (%s)", path, err.Error()))
			}
			fmt.Fprintf(os.Stderr, "Wrote to %s (%s)\n", path, toSize(len(item.JsContents)))

			// Also write the source map
			if args.bundleOptions.SourceMap {
				err := ioutil.WriteFile(item.SourceMapAbsPath, item.SourceMapContents, 0644)
				path := resolver.PrettyPath(item.SourceMapAbsPath)
				if err != nil {
					args.exitWithError(fmt.Sprintf("Failed to write to %s: (%s)", path, err.Error()))
				}
				fmt.Fprintf(os.Stderr, "Wrote to %s (%s)\n", path, toSize(len(item.SourceMapContents)))
			}
		}
	}()

	fmt.Fprintf(os.Stderr, "Done in %dms\n", time.Since(start).Nanoseconds()/1000000)
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
