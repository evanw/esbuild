package snap_api

import (
	"fmt"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/pkg/api"
	"os"
	"regexp"
	"strings"
)

// TODO(thlorenz): document no rewrite regex syntax
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
	EntryPoint  string
	Outfile     string
	Basedir     string
	Metafile    string
	Write       bool
	Deferred    []string
	Norewrite   []string
	NorewriteRx []*regexp.Regexp
	RegexMode   RegexMode
}

type ProcessCmdArgs = func(args *SnapCmdArgs) api.BuildResult

func extractArray(arr string) []string {
	return strings.Split(arr, ",")
}

var rx = regexp.MustCompile(`^[.]?[.]?[/]`)

func trimPathPrefix(paths []string) []string {
	replaced := make([]string, len(paths))
	for i, p := range paths {
		replaced[i] = rx.ReplaceAllString(p, "")
	}
	return replaced
}

func convertToRegex(paths []string) []*regexp.Regexp {
	regexs := make([]*regexp.Regexp, len(paths))

	for i, p := range paths {
		rx = regexp.MustCompile(p)
		regexs[i] = rx
	}
	return regexs
}

type RegexMode uint8

const (
	RegexNone RegexMode = iota
	RegexNormal
	RegexNegated
)

func extractRewriteDefs(paths []string) ([]string, []*regexp.Regexp, RegexMode) {
	var plains []string
	var regexs []string
	regexMode := RegexNone
	for _, p := range paths {
		if strings.HasPrefix(p, "rx:") {
			if regexMode == RegexNegated {
				panic("Can only handle normal or negated regexes, but no mix")
			}
			regexMode = RegexNormal
			regexs = append(regexs, strings.TrimSpace(p[3:]))
		} else if strings.HasPrefix(p, "rx!:") {
			if regexMode == RegexNormal {
				panic("Can only handle normal or negated regexes, but no mix")
			}
			regexMode = RegexNegated
			regexs = append(regexs, strings.TrimSpace(p[4:]))
		} else {
			plains = append(plains, p)
		}
	}
	return trimPathPrefix(plains), convertToRegex(regexs), regexMode
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

		case strings.HasPrefix(arg, "--write="):
			cmdArgs.Write = true

		case strings.HasPrefix(arg, "--basedir="):
			cmdArgs.Basedir = arg[len("--basedir="):]

		case strings.HasPrefix(arg, "--deferred="):
			cmdArgs.Deferred = extractArray(arg[len("--deferred="):])

		case strings.HasPrefix(arg, "--norewrite="):
			plains, regexs, regexMode := extractRewriteDefs(extractArray(arg[len("--norewrite="):]))
			cmdArgs.Norewrite = plains
			cmdArgs.NorewriteRx = regexs
			cmdArgs.RegexMode = regexMode

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
		fmt.Fprintf(os.Stderr, "Need outfil when writing\n\n%s\n", helpText)
		os.Exit(1)
	}
	if cmdArgs.Write && cmdArgs.Metafile == "" {
		fmt.Fprintf(os.Stderr, "Need metafile when writing\n\n%s\n", helpText)
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
	json := resultToJSON(result, cmdArgs.Write)
	if false {
		_ = resultToFile(result)
	}
	// fmt.Fprintln(os.Stdout, len(json))
	fmt.Fprintln(os.Stdout, json)

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
