package snap_api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/pkg/api"
)

const helpText = `
Usage:
  snapshot <config>

Config is a JSON file with the following properties:

  entryfile  (string)    The snapshot entry file
  outfile    (string)    The snapshot bundle output file
  basedir    (string)    The full path project root relative to which modules are resolved 
  deferred   (string[])  List of relative paths to defer
  norewrite  (string[])  List of relative paths to files we should not rewrite
                         which are also automatically deferred
  metafile   (bool)      When true metadata about the build is written to a JSON file
  doctor     (bool)      When true stricter validations are performed to detect problematic code
  sourcemap  (bool)      When true sourcemaps will be generated and included with the second outfile

Examples:
  snapshot snapshot_config.json 
`

type SnapCmdArgs struct {
	Entryfile string
	Outfile   string
	Basedir   string
	Metafile  bool
	Write     bool
	Deferred  []string
	Norewrite []string
	Doctor    bool
	Sourcemap bool
}

func (args *SnapCmdArgs) toString() string {
	return fmt.Sprintf(`Args {
	Entryfile:  '%s',
	Outfile:    '%s',
	Basedir:    '%s',
	Deferred:   '%s'
	Norewrite:  '%s'
	Metafile:   '%t',
	Doctor:     '%t',
	Sourcemap:  '%t',
}`,
		args.Entryfile,
		args.Outfile,
		args.Basedir,
		strings.Join(args.Deferred, ", "),
		strings.Join(args.Norewrite, ", "),
		args.Metafile,
		args.Doctor,
		args.Sourcemap,
	)
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

func SnapCmd(processArgs ProcessCmdArgs) {
	osArgs := os.Args[1:]
	if len(osArgs) != 1 && logger.GetTerminalInfo(os.Stdin).IsTTY {
		fmt.Fprintf(os.Stderr, "%s\n", helpText)
		os.Exit(0)
	}

	filename := osArgs[0]
	jsonBytes, _ := ioutil.ReadFile(filename)
	var cmdArgs SnapCmdArgs
	json.Unmarshal(jsonBytes, &cmdArgs)
	fmt.Fprintln(os.Stderr, cmdArgs.toString())

	// Print help text when there are missing arguments
	if cmdArgs.Entryfile == "" {
		fmt.Fprintf(os.Stderr, "Need entry file\n\n%s\n", helpText)
		os.Exit(1)
	}
	if cmdArgs.Outfile != "" {
		cmdArgs.Write = true
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
