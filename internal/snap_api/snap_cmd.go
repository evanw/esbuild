package snap_api

import (
	"fmt"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/pkg/api"
	"os"
	"regexp"
	"strings"
)

const helpText = `
Usage:
  snapshot [options] entry point

Options:
  --outfile=...    The output file
  --metafile=...   Write metadata about the build to a JSON file
  --basedir=...    The full path project root relative to which modules are resolved 
  --deferred=...   Comma separated list of relative paths to defer
  --norewrite=...  Comma separated list of relative paths to files we should not rewrite
                   which are also automatically deferred
  --doctor         When set stricter validations are performed to detect problematic code
  --sourcemap      When set sourcemaps will be generated and included with the second outfile

Examples:
  snapshot entry_point.js --outfile=out.js --metafile --basedir /dev/foo/snap --deferred='./foo,./bar'
`

type SnapCmdArgs struct {
	EntryPoint string
	Outfile    string
	Basedir    string
	Metafile   bool
	Write      bool
	Deferred   []string
	Norewrite  []string
	Doctor     bool
	Sourcemap  bool
}

type ProcessCmdArgs = func(args *SnapCmdArgs) api.BuildResult

func extractArray(arr string) []string {
	return trimQuotes(strings.Split(arr, ","))
}

func trimQuotes(paths []string) []string {
	replaced := make([]string, len(paths))
	for i, p := range paths {
		replaced[i] = strings.Trim(p, "'")
	}
	return replaced
}

var rx = regexp.MustCompile(`^[.]?[.]?[/]`)

func trimPathPrefix(paths []string) []string {
	replaced := make([]string, len(paths))
	for i, p := range paths {
		replaced[i] = rx.ReplaceAllString(p, "")
	}
	return replaced
}

func extractRewriteDefs(paths []string) []string {
	var norewrites []string
	for _, p := range paths {
		norewrites = append(norewrites, p)
	}
	return trimPathPrefix(norewrites)
}

}

func SnapCmd(processArgs ProcessCmdArgs) {
	osArgs := os.Args[1:]
	cmdArgs := SnapCmdArgs{}

	// Print help text when there are no arguments
	if len(osArgs) == 0 && logger.GetTerminalInfo(os.Stdin).IsTTY {
		fmt.Fprintf(os.Stderr, "%s\n", helpText)
		os.Exit(0)
	}

	argsEnd := 0
	for _, arg := range osArgs {
		switch {
		case arg == "-h", arg == "-help", arg == "--help", arg == "/?":
			fmt.Fprintf(os.Stderr, "%s\n", helpText)
			os.Exit(0)

		case strings.HasPrefix(arg, "--outfile="):
			cmdArgs.Outfile = arg[len("--outfile="):]

		case strings.HasPrefix(arg, "--metafile"):
			cmdArgs.Metafile = true

		case strings.HasPrefix(arg, "--write="):
			cmdArgs.Write = true

		case strings.HasPrefix(arg, "--basedir="):
			cmdArgs.Basedir = arg[len("--basedir="):]

		case strings.HasPrefix(arg, "--deferred="):
			// --deferred='./foo,./bar' will include both `'` on windows, so we ensure to remove it
			cmdArgs.Deferred = extractArray(strings.Trim(arg[len("--deferred="):], "'"))

		case strings.HasPrefix(arg, "--norewrite="):
			norewrites := extractRewriteDefs(extractArray(strings.Trim(arg[len("--norewrite="):], "'")))
			cmdArgs.Norewrite = norewrites

		case strings.HasPrefix(arg, "--doctor"):
			cmdArgs.Doctor = true

		case strings.HasPrefix(arg, "--sourcemap"):
			cmdArgs.Sourcemap = true

		case !strings.HasPrefix(arg, "-"):
			cmdArgs.EntryPoint = arg

		default:
			osArgs[argsEnd] = arg
			argsEnd++
		}
	}
	osArgs = osArgs[:argsEnd]

	// Print help text when there are missing arguments
	if cmdArgs.EntryPoint == "" {
		fmt.Fprintf(os.Stderr, "Need entry point\n\n%s\n", helpText)
		os.Exit(1)
	}
	if cmdArgs.Write && cmdArgs.Outfile == "" {
		fmt.Fprintf(os.Stderr, "Need outfile when writing\n\n%s\n", helpText)
		os.Exit(1)
	}
	if cmdArgs.Basedir == "" {
		fmt.Fprintf(os.Stderr, "Need basedir\n\n%s\n", helpText)
		os.Exit(1)
	}
	if cmdArgs.Deferred == nil {
		cmdArgs.Deferred = []string{}
	}

	result := processArgs(&cmdArgs)
	_, prettyPrint := os.LookupEnv("SNAPSHOT_PRETTY_PRINT_CONTENTS")
	if prettyPrint {
		fmt.Printf("outfile:\n%s", string(result.OutputFiles[0].Contents))
		fmt.Printf("metafile:\n%s", result.Metafile)
	} else {
		json := resultToJSON(result, cmdArgs.Write)
		fmt.Fprintln(os.Stdout, json)
	}

	exitCode := len(result.Errors)
	if cmdArgs.Write && logger.GetTerminalInfo(os.Stdin).IsTTY {
		for _, warning := range result.Warnings {
			fmt.Fprintln(os.Stderr, warning)
		}
		for _, err := range result.Errors {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	os.Exit(exitCode)
}
