package cli

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/internal/logging"
	"github.com/evanw/esbuild/pkg/api"
)

func Run(osArgs []string) int {
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
