package snap_api

import (
	"fmt"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/pkg/api"
	"os"
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

Examples:
  snapshot entry_point.js --outfile=out.js --metafile: meta.json --basedir /dev/foo/snap --deferred='./foo,./bar'
`

type SnapCmdArgs struct {
	EntryPoint string
	Outfile    string
	Basedir    string
	Metafile   string
	Deferred   []string
	Norewrite  []string
}

type ProcessCmdArgs = func(args *SnapCmdArgs) api.BuildResult

func extractArray(arr string) []string {
	return strings.Split(arr, ",")
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

		case strings.HasPrefix(arg, "--metafile="):
			cmdArgs.Metafile = arg[len("--metafile="):]

		case strings.HasPrefix(arg, "--basedir="):
			cmdArgs.Basedir = arg[len("--basedir="):]

		case strings.HasPrefix(arg, "--deferred="):
			cmdArgs.Deferred = extractArray(arg[len("--deferred="):])

		case strings.HasPrefix(arg, "--norewrite="):
			cmdArgs.Norewrite = extractArray(arg[len("--norewrite="):])

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
	if cmdArgs.Outfile == "" {
		fmt.Fprintf(os.Stderr, "Need outfile\n\n%s\n", helpText)
		os.Exit(1)
	}
	if cmdArgs.Metafile == "" {
		fmt.Fprintf(os.Stderr, "Need metafile\n\n%s\n", helpText)
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
