package cli

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/evanw/esbuild/internal/logging"
	"github.com/evanw/esbuild/pkg/api"
)

func newBuildOptions() api.BuildOptions {
	return api.BuildOptions{
		Loaders: make(map[string]api.Loader),
		Defines: make(map[string]string),
	}
}

func newTransformOptions() api.TransformOptions {
	return api.TransformOptions{
		Defines: make(map[string]string),
	}
}

func parseOptionsImpl(osArgs []string, buildOpts *api.BuildOptions, transformOpts *api.TransformOptions) error {
	hasBareSourceMapFlag := false

	// Parse the arguments now that we know what we're parsing
	for _, arg := range osArgs {
		switch {
		case arg == "--bundle" && buildOpts != nil:
			buildOpts.Bundle = true

		case arg == "--minify":
			if buildOpts != nil {
				buildOpts.MinifySyntax = true
				buildOpts.MinifyWhitespace = true
				buildOpts.MinifyIdentifiers = true
			} else {
				transformOpts.MinifySyntax = true
				transformOpts.MinifyWhitespace = true
				transformOpts.MinifyIdentifiers = true
			}

		case arg == "--minify-syntax":
			if buildOpts != nil {
				buildOpts.MinifySyntax = true
			} else {
				transformOpts.MinifySyntax = true
			}

		case arg == "--minify-whitespace":
			if buildOpts != nil {
				buildOpts.MinifyWhitespace = true
			} else {
				transformOpts.MinifyWhitespace = true
			}

		case arg == "--minify-identifiers":
			if buildOpts != nil {
				buildOpts.MinifyIdentifiers = true
			} else {
				transformOpts.MinifyIdentifiers = true
			}

		case arg == "--sourcemap":
			if buildOpts != nil {
				buildOpts.Sourcemap = api.SourceMapLinked
			} else {
				transformOpts.Sourcemap = api.SourceMapInline
			}
			hasBareSourceMapFlag = true

		case arg == "--sourcemap=external":
			if buildOpts != nil {
				buildOpts.Sourcemap = api.SourceMapExternal
			} else {
				transformOpts.Sourcemap = api.SourceMapExternal
			}
			hasBareSourceMapFlag = false

		case arg == "--sourcemap=inline":
			if buildOpts != nil {
				buildOpts.Sourcemap = api.SourceMapInline
			} else {
				transformOpts.Sourcemap = api.SourceMapInline
			}
			hasBareSourceMapFlag = false

		case strings.HasPrefix(arg, "--sourcefile=") && transformOpts != nil:
			transformOpts.Sourcefile = arg[len("--sourcefile="):]

		case strings.HasPrefix(arg, "--resolve-extensions=") && buildOpts != nil:
			buildOpts.ResolveExtensions = strings.Split(arg[len("--resolve-extensions="):], ",")

		case strings.HasPrefix(arg, "--global-name=") && buildOpts != nil:
			buildOpts.GlobalName = arg[len("--global-name="):]

		case strings.HasPrefix(arg, "--metafile=") && buildOpts != nil:
			buildOpts.Metafile = arg[len("--metafile="):]

		case strings.HasPrefix(arg, "--outfile=") && buildOpts != nil:
			buildOpts.Outfile = arg[len("--outfile="):]

		case strings.HasPrefix(arg, "--outdir=") && buildOpts != nil:
			buildOpts.Outdir = arg[len("--outdir="):]

		case strings.HasPrefix(arg, "--define:"):
			value := arg[len("--define:"):]
			equals := strings.IndexByte(value, '=')
			if equals == -1 {
				return fmt.Errorf("Missing \"=\": %q", value)
			}
			if buildOpts != nil {
				buildOpts.Defines[value[:equals]] = value[equals+1:]
			} else {
				transformOpts.Defines[value[:equals]] = value[equals+1:]
			}

		case strings.HasPrefix(arg, "--loader:") && buildOpts != nil:
			value := arg[len("--loader:"):]
			equals := strings.IndexByte(value, '=')
			if equals == -1 {
				return fmt.Errorf("Missing \"=\": %q", value)
			}
			ext, text := value[:equals], value[equals+1:]
			loader, err := parseLoader(text)
			if err != nil {
				return err
			}
			buildOpts.Loaders[ext] = loader

		case strings.HasPrefix(arg, "--loader=") && transformOpts != nil:
			value := arg[len("--loader="):]
			loader, err := parseLoader(value)
			if err != nil {
				return err
			}
			if loader == api.LoaderFile {
				return fmt.Errorf("Cannot transform using the \"file\" loader")
			}
			transformOpts.Loader = loader

		case strings.HasPrefix(arg, "--target="):
			value := arg[len("--target="):]
			var target api.Target
			switch value {
			case "esnext":
				target = api.ESNext
			case "es6", "es2015":
				target = api.ES2015
			case "es2016":
				target = api.ES2016
			case "es2017":
				target = api.ES2017
			case "es2018":
				target = api.ES2018
			case "es2019":
				target = api.ES2019
			case "es2020":
				target = api.ES2020
			default:
				return fmt.Errorf("Invalid target: %q (valid: "+
					"esnext, es6, es2015, es2016, es2017, es2018, es2019, es2020)", value)
			}
			if buildOpts != nil {
				buildOpts.Target = target
			} else {
				transformOpts.Target = target
			}

		case strings.HasPrefix(arg, "--platform=") && buildOpts != nil:
			value := arg[len("--platform="):]
			switch value {
			case "browser":
				buildOpts.Platform = api.PlatformBrowser
			case "node":
				buildOpts.Platform = api.PlatformNode
			default:
				return fmt.Errorf("Invalid platform: %q (valid: browser, node)", value)
			}

		case strings.HasPrefix(arg, "--format=") && buildOpts != nil:
			value := arg[len("--format="):]
			switch value {
			case "iife":
				buildOpts.Format = api.FormatIIFE
			case "cjs":
				buildOpts.Format = api.FormatCommonJS
			case "esm":
				buildOpts.Format = api.FormatESModule
			default:
				return fmt.Errorf("Invalid format: %q (valid: iife, cjs, esm)", value)
			}

		case strings.HasPrefix(arg, "--external:") && buildOpts != nil:
			buildOpts.Externals = append(buildOpts.Externals, arg[len("--external:"):])

		case strings.HasPrefix(arg, "--jsx-factory="):
			value := arg[len("--jsx-factory="):]
			if buildOpts != nil {
				buildOpts.JSXFactory = value
			} else {
				transformOpts.JSXFactory = value
			}

		case strings.HasPrefix(arg, "--jsx-fragment="):
			value := arg[len("--jsx-fragment="):]
			if buildOpts != nil {
				buildOpts.JSXFragment = value
			} else {
				transformOpts.JSXFragment = value
			}

		case strings.HasPrefix(arg, "--error-limit="):
			value := arg[len("--error-limit="):]
			limit, err := strconv.Atoi(value)
			if err != nil || limit < 0 {
				return fmt.Errorf("Invalid error limit: %q", value)
			}
			if buildOpts != nil {
				buildOpts.ErrorLimit = limit
			} else {
				transformOpts.ErrorLimit = limit
			}

			// Make sure this stays in sync with "PrintErrorToStderr"
		case strings.HasPrefix(arg, "--color="):
			value := arg[len("--color="):]
			var color api.StderrColor
			switch value {
			case "false":
				color = api.ColorNever
			case "true":
				color = api.ColorAlways
			default:
				return fmt.Errorf("Invalid color: %q (valid: false, true)", value)
			}
			if buildOpts != nil {
				buildOpts.Color = color
			} else {
				transformOpts.Color = color
			}

		// Make sure this stays in sync with "PrintErrorToStderr"
		case strings.HasPrefix(arg, "--log-level="):
			value := arg[len("--log-level="):]
			var logLevel api.LogLevel
			switch value {
			case "info":
				logLevel = api.LogLevelInfo
			case "warning":
				logLevel = api.LogLevelWarning
			case "error":
				logLevel = api.LogLevelError
			case "silent":
				logLevel = api.LogLevelSilent
			default:
				return fmt.Errorf("Invalid log level: %q (valid: info, warning, error, silent)", arg)
			}
			if buildOpts != nil {
				buildOpts.LogLevel = logLevel
			} else {
				transformOpts.LogLevel = logLevel
			}

		case !strings.HasPrefix(arg, "-") && buildOpts != nil:
			buildOpts.EntryPoints = append(buildOpts.EntryPoints, arg)

		default:
			if buildOpts != nil {
				return fmt.Errorf("Invalid build flag: %q", arg)
			} else {
				return fmt.Errorf("Invalid transform flag: %q", arg)
			}
		}
	}

	// If we're building, the last source map flag is "--sourcemap", and there
	// is no output path, change the source map option to "inline" because we're
	// going to be writing to stdout which can only represent a single file.
	if buildOpts != nil && hasBareSourceMapFlag && buildOpts.Outfile == "" && buildOpts.Outdir == "" {
		buildOpts.Sourcemap = api.SourceMapInline
	}

	return nil
}

func parseLoader(text string) (api.Loader, error) {
	switch text {
	case "js":
		return api.LoaderJS, nil
	case "jsx":
		return api.LoaderJSX, nil
	case "ts":
		return api.LoaderTS, nil
	case "tsx":
		return api.LoaderTSX, nil
	case "json":
		return api.LoaderJSON, nil
	case "text":
		return api.LoaderText, nil
	case "base64":
		return api.LoaderBase64, nil
	case "dataurl":
		return api.LoaderDataURL, nil
	case "file":
		return api.LoaderFile, nil
	default:
		return ^api.Loader(0), fmt.Errorf("Invalid loader: %q (valid: "+
			"js, jsx, ts, tsx, json, text, base64, dataurl, file)", text)
	}
}

// This returns either BuildOptions, TransformOptions, or an error
func parseOptionsForRun(osArgs []string) (*api.BuildOptions, *api.TransformOptions, error) {
	// If there's an entry point, then we're building
	for _, arg := range osArgs {
		if !strings.HasPrefix(arg, "-") {
			options := newBuildOptions()

			// Apply defaults appropriate for the CLI
			options.ErrorLimit = 10
			options.LogLevel = api.LogLevelInfo

			err := parseOptionsImpl(osArgs, &options, nil)
			if err != nil {
				return nil, nil, err
			}
			return &options, nil, nil
		}
	}

	// Otherwise, we're transforming
	options := newTransformOptions()

	// Apply defaults appropriate for the CLI
	options.ErrorLimit = 10
	options.LogLevel = api.LogLevelInfo

	err := parseOptionsImpl(osArgs, nil, &options)
	if err != nil {
		return nil, nil, err
	}
	if options.Sourcemap != api.SourceMapNone && options.Sourcemap != api.SourceMapInline {
		return nil, nil, fmt.Errorf("Must use \"inline\" source map when transforming stdin")
	}
	return nil, &options, nil
}

func runImpl(osArgs []string) int {
	buildOptions, transformOptions, err := parseOptionsForRun(osArgs)

	switch {
	case buildOptions != nil:
		// Run the build and stop if there were errors
		result := api.Build(*buildOptions)
		if len(result.Errors) > 0 {
			return 1
		}

		// Special-case writing to stdout
		if buildOptions.Outfile == "" && buildOptions.Outdir == "" {
			if len(result.OutputFiles) != 1 {
				logging.PrintErrorToStderr(osArgs, fmt.Sprintf(
					"Internal error: did not expect to generate %d files when writing to stdout", len(result.OutputFiles)))
			} else if _, err := os.Stdout.Write(result.OutputFiles[0].Contents); err != nil {
				logging.PrintErrorToStderr(osArgs, fmt.Sprintf(
					"Failed to write to stdout: %s", err.Error()))
			}
		} else {
			for _, outputFile := range result.OutputFiles {
				if err := os.MkdirAll(filepath.Dir(outputFile.Path), 0755); err != nil {
					result.Errors = append(result.Errors, api.Message{Text: fmt.Sprintf(
						"Failed to create output directory: %s", err.Error())})
				} else if err := ioutil.WriteFile(outputFile.Path, outputFile.Contents, 0644); err != nil {
					logging.PrintErrorToStderr(osArgs, fmt.Sprintf(
						"Failed to write to output file: %s", err.Error()))
				}
			}
		}

	case transformOptions != nil:
		// Read the input from stdin
		bytes, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			logging.PrintErrorToStderr(osArgs, fmt.Sprintf(
				"Could not read from stdin: %s", err.Error()))
			return 1
		}

		// Run the transform and stop if there were errors
		result := api.Transform(string(bytes), *transformOptions)
		if len(result.Errors) > 0 {
			return 1
		}

		// Write the output to stdout
		os.Stdout.Write(result.JS)

	case err != nil:
		logging.PrintErrorToStderr(osArgs, err.Error())
		return 1
	}

	return 0
}
