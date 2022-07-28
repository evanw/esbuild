package cli

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/evanw/esbuild/internal/cli_helpers"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/pkg/api"
)

func newBuildOptions() api.BuildOptions {
	return api.BuildOptions{
		Banner:      make(map[string]string),
		Define:      make(map[string]string),
		Footer:      make(map[string]string),
		Loader:      make(map[string]api.Loader),
		LogOverride: make(map[string]api.LogLevel),
		Supported:   make(map[string]bool),
	}
}

func newTransformOptions() api.TransformOptions {
	return api.TransformOptions{
		Define:      make(map[string]string),
		LogOverride: make(map[string]api.LogLevel),
		Supported:   make(map[string]bool),
	}
}

type parseOptionsKind uint8

const (
	// This means we're parsing it for our own internal use
	kindInternal parseOptionsKind = iota

	// This means the result is returned through a public API
	kindExternal
)

type parseOptionsExtras struct {
	metafile    *string
	mangleCache *string
}

func isBoolFlag(arg string, flag string) bool {
	if strings.HasPrefix(arg, flag) {
		remainder := arg[len(flag):]
		return len(remainder) == 0 || remainder[0] == '='
	}
	return false
}

func parseBoolFlag(arg string, defaultValue bool) (bool, *cli_helpers.ErrorWithNote) {
	equals := strings.IndexByte(arg, '=')
	if equals == -1 {
		return defaultValue, nil
	}
	value := arg[equals+1:]
	switch value {
	case "false":
		return false, nil
	case "true":
		return true, nil
	}
	return false, cli_helpers.MakeErrorWithNote(
		fmt.Sprintf("Invalid value %q in %q", value, arg),
		"Valid values are \"true\" or \"false\".",
	)
}

func parseOptionsImpl(
	osArgs []string,
	buildOpts *api.BuildOptions,
	transformOpts *api.TransformOptions,
	kind parseOptionsKind,
) (extras parseOptionsExtras, err *cli_helpers.ErrorWithNote) {
	hasBareSourceMapFlag := false

	// Parse the arguments now that we know what we're parsing
	for _, arg := range osArgs {
		switch {
		case isBoolFlag(arg, "--bundle") && buildOpts != nil:
			if value, err := parseBoolFlag(arg, true); err != nil {
				return parseOptionsExtras{}, err
			} else {
				buildOpts.Bundle = value
			}

		case isBoolFlag(arg, "--preserve-symlinks") && buildOpts != nil:
			if value, err := parseBoolFlag(arg, true); err != nil {
				return parseOptionsExtras{}, err
			} else {
				buildOpts.PreserveSymlinks = value
			}

		case isBoolFlag(arg, "--splitting") && buildOpts != nil:
			if value, err := parseBoolFlag(arg, true); err != nil {
				return parseOptionsExtras{}, err
			} else {
				buildOpts.Splitting = value
			}

		case isBoolFlag(arg, "--allow-overwrite") && buildOpts != nil:
			if value, err := parseBoolFlag(arg, true); err != nil {
				return parseOptionsExtras{}, err
			} else {
				buildOpts.AllowOverwrite = value
			}

		case isBoolFlag(arg, "--watch") && buildOpts != nil:
			if value, err := parseBoolFlag(arg, true); err != nil {
				return parseOptionsExtras{}, err
			} else if value {
				buildOpts.Watch = &api.WatchMode{}
			} else {
				buildOpts.Watch = nil
			}

		case isBoolFlag(arg, "--minify"):
			if value, err := parseBoolFlag(arg, true); err != nil {
				return parseOptionsExtras{}, err
			} else if buildOpts != nil {
				buildOpts.MinifySyntax = value
				buildOpts.MinifyWhitespace = value
				buildOpts.MinifyIdentifiers = value
			} else {
				transformOpts.MinifySyntax = value
				transformOpts.MinifyWhitespace = value
				transformOpts.MinifyIdentifiers = value
			}

		case isBoolFlag(arg, "--minify-syntax"):
			if value, err := parseBoolFlag(arg, true); err != nil {
				return parseOptionsExtras{}, err
			} else if buildOpts != nil {
				buildOpts.MinifySyntax = value
			} else {
				transformOpts.MinifySyntax = value
			}

		case isBoolFlag(arg, "--minify-whitespace"):
			if value, err := parseBoolFlag(arg, true); err != nil {
				return parseOptionsExtras{}, err
			} else if buildOpts != nil {
				buildOpts.MinifyWhitespace = value
			} else {
				transformOpts.MinifyWhitespace = value
			}

		case isBoolFlag(arg, "--minify-identifiers"):
			if value, err := parseBoolFlag(arg, true); err != nil {
				return parseOptionsExtras{}, err
			} else if buildOpts != nil {
				buildOpts.MinifyIdentifiers = value
			} else {
				transformOpts.MinifyIdentifiers = value
			}

		case isBoolFlag(arg, "--mangle-quoted"):
			if value, err := parseBoolFlag(arg, true); err != nil {
				return parseOptionsExtras{}, err
			} else {
				var mangleQuoted *api.MangleQuoted
				if buildOpts != nil {
					mangleQuoted = &buildOpts.MangleQuoted
				} else {
					mangleQuoted = &transformOpts.MangleQuoted
				}
				if value {
					*mangleQuoted = api.MangleQuotedTrue
				} else {
					*mangleQuoted = api.MangleQuotedFalse
				}
			}

		case strings.HasPrefix(arg, "--mangle-props="):
			value := arg[len("--mangle-props="):]
			if buildOpts != nil {
				buildOpts.MangleProps = value
			} else {
				transformOpts.MangleProps = value
			}

		case strings.HasPrefix(arg, "--reserve-props="):
			value := arg[len("--reserve-props="):]
			if buildOpts != nil {
				buildOpts.ReserveProps = value
			} else {
				transformOpts.ReserveProps = value
			}

		case strings.HasPrefix(arg, "--mangle-cache=") && buildOpts != nil && kind == kindInternal:
			value := arg[len("--mangle-cache="):]
			extras.mangleCache = &value

		case strings.HasPrefix(arg, "--drop:"):
			value := arg[len("--drop:"):]
			switch value {
			case "console":
				if buildOpts != nil {
					buildOpts.Drop |= api.DropConsole
				} else {
					transformOpts.Drop |= api.DropConsole
				}
			case "debugger":
				if buildOpts != nil {
					buildOpts.Drop |= api.DropDebugger
				} else {
					transformOpts.Drop |= api.DropDebugger
				}
			default:
				return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(
					fmt.Sprintf("Invalid value %q in %q", value, arg),
					"Valid values are \"console\" or \"debugger\".",
				)
			}

		case strings.HasPrefix(arg, "--legal-comments="):
			value := arg[len("--legal-comments="):]
			var legalComments api.LegalComments
			switch value {
			case "none":
				legalComments = api.LegalCommentsNone
			case "inline":
				legalComments = api.LegalCommentsInline
			case "eof":
				legalComments = api.LegalCommentsEndOfFile
			case "linked":
				legalComments = api.LegalCommentsLinked
			case "external":
				legalComments = api.LegalCommentsExternal
			default:
				return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(
					fmt.Sprintf("Invalid value %q in %q", value, arg),
					"Valid values are \"none\", \"inline\", \"eof\", \"linked\", or \"external\".",
				)
			}
			if buildOpts != nil {
				buildOpts.LegalComments = legalComments
			} else {
				transformOpts.LegalComments = legalComments
			}

		case strings.HasPrefix(arg, "--charset="):
			var value *api.Charset
			if buildOpts != nil {
				value = &buildOpts.Charset
			} else {
				value = &transformOpts.Charset
			}
			name := arg[len("--charset="):]
			switch name {
			case "ascii":
				*value = api.CharsetASCII
			case "utf8":
				*value = api.CharsetUTF8
			default:
				return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(
					fmt.Sprintf("Invalid value %q in %q", name, arg),
					"Valid values are \"ascii\" or \"utf8\".",
				)
			}

		case isBoolFlag(arg, "--tree-shaking"):
			if value, err := parseBoolFlag(arg, true); err != nil {
				return parseOptionsExtras{}, err
			} else {
				var treeShaking *api.TreeShaking
				if buildOpts != nil {
					treeShaking = &buildOpts.TreeShaking
				} else {
					treeShaking = &transformOpts.TreeShaking
				}
				if value {
					*treeShaking = api.TreeShakingTrue
				} else {
					*treeShaking = api.TreeShakingFalse
				}
			}

		case isBoolFlag(arg, "--ignore-annotations"):
			if value, err := parseBoolFlag(arg, true); err != nil {
				return parseOptionsExtras{}, err
			} else if buildOpts != nil {
				buildOpts.IgnoreAnnotations = value
			} else {
				transformOpts.IgnoreAnnotations = value
			}

		case isBoolFlag(arg, "--keep-names"):
			if value, err := parseBoolFlag(arg, true); err != nil {
				return parseOptionsExtras{}, err
			} else if buildOpts != nil {
				buildOpts.KeepNames = value
			} else {
				transformOpts.KeepNames = value
			}

		case arg == "--sourcemap":
			if buildOpts != nil {
				buildOpts.Sourcemap = api.SourceMapLinked
			} else {
				transformOpts.Sourcemap = api.SourceMapInline
			}
			hasBareSourceMapFlag = true

		case strings.HasPrefix(arg, "--sourcemap="):
			value := arg[len("--sourcemap="):]
			var sourcemap api.SourceMap
			switch value {
			case "linked":
				sourcemap = api.SourceMapLinked
			case "inline":
				sourcemap = api.SourceMapInline
			case "external":
				sourcemap = api.SourceMapExternal
			case "both":
				sourcemap = api.SourceMapInlineAndExternal
			default:
				return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(
					fmt.Sprintf("Invalid value %q in %q", value, arg),
					"Valid values are \"inline\", \"external\", or \"both\".",
				)
			}
			if buildOpts != nil {
				buildOpts.Sourcemap = sourcemap
			} else {
				transformOpts.Sourcemap = sourcemap
			}
			hasBareSourceMapFlag = false

		case strings.HasPrefix(arg, "--source-root="):
			sourceRoot := arg[len("--source-root="):]
			if buildOpts != nil {
				buildOpts.SourceRoot = sourceRoot
			} else {
				transformOpts.SourceRoot = sourceRoot
			}

		case isBoolFlag(arg, "--sources-content"):
			if value, err := parseBoolFlag(arg, true); err != nil {
				return parseOptionsExtras{}, err
			} else {
				var sourcesContent *api.SourcesContent
				if buildOpts != nil {
					sourcesContent = &buildOpts.SourcesContent
				} else {
					sourcesContent = &transformOpts.SourcesContent
				}
				if value {
					*sourcesContent = api.SourcesContentInclude
				} else {
					*sourcesContent = api.SourcesContentExclude
				}
			}

		case strings.HasPrefix(arg, "--sourcefile="):
			if buildOpts != nil {
				if buildOpts.Stdin == nil {
					buildOpts.Stdin = &api.StdinOptions{}
				}
				buildOpts.Stdin.Sourcefile = arg[len("--sourcefile="):]
			} else {
				transformOpts.Sourcefile = arg[len("--sourcefile="):]
			}

		case strings.HasPrefix(arg, "--resolve-extensions=") && buildOpts != nil:
			buildOpts.ResolveExtensions = splitWithEmptyCheck(arg[len("--resolve-extensions="):], ",")

		case strings.HasPrefix(arg, "--main-fields=") && buildOpts != nil:
			buildOpts.MainFields = splitWithEmptyCheck(arg[len("--main-fields="):], ",")

		case strings.HasPrefix(arg, "--conditions=") && buildOpts != nil:
			buildOpts.Conditions = splitWithEmptyCheck(arg[len("--conditions="):], ",")

		case strings.HasPrefix(arg, "--public-path=") && buildOpts != nil:
			buildOpts.PublicPath = arg[len("--public-path="):]

		case strings.HasPrefix(arg, "--global-name="):
			if buildOpts != nil {
				buildOpts.GlobalName = arg[len("--global-name="):]
			} else {
				transformOpts.GlobalName = arg[len("--global-name="):]
			}

		case arg == "--metafile" && buildOpts != nil && kind == kindExternal:
			buildOpts.Metafile = true

		case strings.HasPrefix(arg, "--metafile=") && buildOpts != nil && kind == kindInternal:
			value := arg[len("--metafile="):]
			buildOpts.Metafile = true
			extras.metafile = &value

		case strings.HasPrefix(arg, "--outfile=") && buildOpts != nil:
			buildOpts.Outfile = arg[len("--outfile="):]

		case strings.HasPrefix(arg, "--outdir=") && buildOpts != nil:
			buildOpts.Outdir = arg[len("--outdir="):]

		case strings.HasPrefix(arg, "--outbase=") && buildOpts != nil:
			buildOpts.Outbase = arg[len("--outbase="):]

		case strings.HasPrefix(arg, "--tsconfig=") && buildOpts != nil:
			buildOpts.Tsconfig = arg[len("--tsconfig="):]

		case strings.HasPrefix(arg, "--tsconfig-raw=") && transformOpts != nil:
			transformOpts.TsconfigRaw = arg[len("--tsconfig-raw="):]

		case strings.HasPrefix(arg, "--entry-names=") && buildOpts != nil:
			buildOpts.EntryNames = arg[len("--entry-names="):]

		case strings.HasPrefix(arg, "--chunk-names=") && buildOpts != nil:
			buildOpts.ChunkNames = arg[len("--chunk-names="):]

		case strings.HasPrefix(arg, "--asset-names=") && buildOpts != nil:
			buildOpts.AssetNames = arg[len("--asset-names="):]

		case strings.HasPrefix(arg, "--define:"):
			value := arg[len("--define:"):]
			equals := strings.IndexByte(value, '=')
			if equals == -1 {
				return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(
					fmt.Sprintf("Missing \"=\" in %q", arg),
					"You need to use \"=\" to specify both the original value and the replacement value. "+
						"For example, \"--define:DEBUG=true\" replaces \"DEBUG\" with \"true\".",
				)
			}
			if buildOpts != nil {
				buildOpts.Define[value[:equals]] = value[equals+1:]
			} else {
				transformOpts.Define[value[:equals]] = value[equals+1:]
			}

		case strings.HasPrefix(arg, "--log-override:"):
			value := arg[len("--log-override:"):]
			equals := strings.IndexByte(value, '=')
			if equals == -1 {
				return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(
					fmt.Sprintf("Missing \"=\" in %q", arg),
					"You need to use \"=\" to specify both the message name and the log level. "+
						"For example, \"--log-override:css-syntax-error=error\" turns all \"css-syntax-error\" log messages into errors.",
				)
			}
			logLevel, err := parseLogLevel(value[equals+1:], arg)
			if err != nil {
				return parseOptionsExtras{}, err
			}
			if buildOpts != nil {
				buildOpts.LogOverride[value[:equals]] = logLevel
			} else {
				transformOpts.LogOverride[value[:equals]] = logLevel
			}

		case strings.HasPrefix(arg, "--supported:"):
			value := arg[len("--supported:"):]
			equals := strings.IndexByte(value, '=')
			if equals == -1 {
				return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(
					fmt.Sprintf("Missing \"=\" in %q", arg),
					"You need to use \"=\" to specify both the name of the feature and whether it is supported or not. "+
						"For example, \"--supported:arrow=false\" marks arrow functions as unsupported.",
				)
			}
			if isSupported, err := parseBoolFlag(arg, true); err != nil {
				return parseOptionsExtras{}, err
			} else if buildOpts != nil {
				buildOpts.Supported[value[:equals]] = isSupported
			} else {
				transformOpts.Supported[value[:equals]] = isSupported
			}

		case strings.HasPrefix(arg, "--pure:"):
			value := arg[len("--pure:"):]
			if buildOpts != nil {
				buildOpts.Pure = append(buildOpts.Pure, value)
			} else {
				transformOpts.Pure = append(transformOpts.Pure, value)
			}

		case strings.HasPrefix(arg, "--loader:") && buildOpts != nil:
			value := arg[len("--loader:"):]
			equals := strings.IndexByte(value, '=')
			if equals == -1 {
				return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(
					fmt.Sprintf("Missing \"=\" in %q", arg),
					"You need to specify the file extension that the loader applies to. "+
						"For example, \"--loader:.js=jsx\" applies the \"jsx\" loader to files with the \".js\" extension.",
				)
			}
			ext, text := value[:equals], value[equals+1:]
			loader, err := cli_helpers.ParseLoader(text)
			if err != nil {
				return parseOptionsExtras{}, err
			}
			buildOpts.Loader[ext] = loader

		case strings.HasPrefix(arg, "--loader="):
			value := arg[len("--loader="):]
			loader, err := cli_helpers.ParseLoader(value)
			if err != nil {
				return parseOptionsExtras{}, err
			}
			if loader == api.LoaderFile || loader == api.LoaderCopy {
				return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(
					fmt.Sprintf("%q is not supported when transforming stdin", arg),
					fmt.Sprintf("Using esbuild to transform stdin only generates one output file, so you cannot use the %q loader "+
						"since that needs to generate two output files.", value),
				)
			}
			if buildOpts != nil {
				if buildOpts.Stdin == nil {
					buildOpts.Stdin = &api.StdinOptions{}
				}
				buildOpts.Stdin.Loader = loader
			} else {
				transformOpts.Loader = loader
			}

		case strings.HasPrefix(arg, "--target="):
			target, engines, err := parseTargets(splitWithEmptyCheck(arg[len("--target="):], ","), arg)
			if err != nil {
				return parseOptionsExtras{}, err
			}
			if buildOpts != nil {
				buildOpts.Target = target
				buildOpts.Engines = engines
			} else {
				transformOpts.Target = target
				transformOpts.Engines = engines
			}

		case strings.HasPrefix(arg, "--out-extension:") && buildOpts != nil:
			value := arg[len("--out-extension:"):]
			equals := strings.IndexByte(value, '=')
			if equals == -1 {
				return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(
					fmt.Sprintf("Missing \"=\" in %q", arg),
					"You need to use either \"--out-extension:.js=...\" or \"--out-extension:.css=...\" "+
						"to specify the file type that the output extension applies to .",
				)
			}
			if buildOpts.OutExtensions == nil {
				buildOpts.OutExtensions = make(map[string]string)
			}
			buildOpts.OutExtensions[value[:equals]] = value[equals+1:]

		case strings.HasPrefix(arg, "--platform="):
			value := arg[len("--platform="):]
			var platform api.Platform
			switch value {
			case "browser":
				platform = api.PlatformBrowser
			case "node":
				platform = api.PlatformNode
			case "neutral":
				platform = api.PlatformNeutral
			default:
				return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(
					fmt.Sprintf("Invalid value %q in %q", value, arg),
					"Valid values are \"browser\", \"node\", or \"neutral\".",
				)
			}
			if buildOpts != nil {
				buildOpts.Platform = platform
			} else {
				transformOpts.Platform = platform
			}

		case strings.HasPrefix(arg, "--format="):
			value := arg[len("--format="):]
			var format api.Format
			switch value {
			case "iife":
				format = api.FormatIIFE
			case "cjs":
				format = api.FormatCommonJS
			case "esm":
				format = api.FormatESModule
			default:
				return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(
					fmt.Sprintf("Invalid value %q in %q", value, arg),
					"Valid values are \"iife\", \"cjs\", or \"esm\".",
				)
			}
			if buildOpts != nil {
				buildOpts.Format = format
			} else {
				transformOpts.Format = format
			}

		case strings.HasPrefix(arg, "--external:") && buildOpts != nil:
			buildOpts.External = append(buildOpts.External, arg[len("--external:"):])

		case strings.HasPrefix(arg, "--inject:") && buildOpts != nil:
			buildOpts.Inject = append(buildOpts.Inject, arg[len("--inject:"):])

		case strings.HasPrefix(arg, "--jsx="):
			value := arg[len("--jsx="):]
			var mode api.JSXMode
			switch value {
			case "transform":
				mode = api.JSXModeTransform
			case "preserve":
				mode = api.JSXModePreserve
			default:
				return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(
					fmt.Sprintf("Invalid value %q in %q", value, arg),
					"Valid values are \"transform\" or \"preserve\".",
				)
			}
			if buildOpts != nil {
				buildOpts.JSXMode = mode
			} else {
				transformOpts.JSXMode = mode
			}

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

		case strings.HasPrefix(arg, "--jsx-runtime="):
			value := arg[len("--jsx-runtime="):]
			var runtime api.JSXRuntime
			switch value {
			case "classic":
				runtime = api.JSXRuntimeClassic
			case "automatic":
				runtime = api.JSXRuntimeAutomatic
			default:
				return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(
					fmt.Sprintf("Invalid value %q in %q", value, arg),
					"Valid values are \"classic\" or \"automatic\".",
				)
			}
			if buildOpts != nil {
				buildOpts.JSXRuntime = runtime
			} else {
				transformOpts.JSXRuntime = runtime
			}

		case strings.HasPrefix(arg, "--jsx-import-source="):
			value := arg[len("--jsx-import-source="):]
			if buildOpts != nil {
				buildOpts.JSXImportSource = value
			} else {
				transformOpts.JSXImportSource = value
			}

		case isBoolFlag(arg, "--jsx-development"):
			if value, err := parseBoolFlag(arg, true); err != nil {
				return parseOptionsExtras{}, err
			} else if buildOpts != nil {
				buildOpts.JSXDevelopment = value
			} else {
				transformOpts.JSXDevelopment = value
			}

		case strings.HasPrefix(arg, "--banner=") && transformOpts != nil:
			transformOpts.Banner = arg[len("--banner="):]

		case strings.HasPrefix(arg, "--footer=") && transformOpts != nil:
			transformOpts.Footer = arg[len("--footer="):]

		case strings.HasPrefix(arg, "--banner:") && buildOpts != nil:
			value := arg[len("--banner:"):]
			equals := strings.IndexByte(value, '=')
			if equals == -1 {
				return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(
					fmt.Sprintf("Missing \"=\" in %q", arg),
					"You need to use either \"--banner:js=...\" or \"--banner:css=...\" to specify the language that the banner applies to.",
				)
			}
			buildOpts.Banner[value[:equals]] = value[equals+1:]

		case strings.HasPrefix(arg, "--footer:") && buildOpts != nil:
			value := arg[len("--footer:"):]
			equals := strings.IndexByte(value, '=')
			if equals == -1 {
				return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(
					fmt.Sprintf("Missing \"=\" in %q", arg),
					"You need to use either \"--footer:js=...\" or \"--footer:css=...\" to specify the language that the footer applies to.",
				)
			}
			buildOpts.Footer[value[:equals]] = value[equals+1:]

		case strings.HasPrefix(arg, "--log-limit="):
			value := arg[len("--log-limit="):]
			limit, err := strconv.Atoi(value)
			if err != nil || limit < 0 {
				return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(
					fmt.Sprintf("Invalid value %q in %q", value, arg),
					"The log limit must be a non-negative integer.",
				)
			}
			if buildOpts != nil {
				buildOpts.LogLimit = limit
			} else {
				transformOpts.LogLimit = limit
			}

			// Make sure this stays in sync with "PrintErrorToStderr"
		case isBoolFlag(arg, "--color"):
			if value, err := parseBoolFlag(arg, true); err != nil {
				return parseOptionsExtras{}, err
			} else {
				var color *api.StderrColor
				if buildOpts != nil {
					color = &buildOpts.Color
				} else {
					color = &transformOpts.Color
				}
				if value {
					*color = api.ColorAlways
				} else {
					*color = api.ColorNever
				}
			}

		// Make sure this stays in sync with "PrintErrorToStderr"
		case strings.HasPrefix(arg, "--log-level="):
			value := arg[len("--log-level="):]
			logLevel, err := parseLogLevel(value, arg)
			if err != nil {
				return parseOptionsExtras{}, err
			}
			if buildOpts != nil {
				buildOpts.LogLevel = logLevel
			} else {
				transformOpts.LogLevel = logLevel
			}

		case strings.HasPrefix(arg, "'--"):
			return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(
				fmt.Sprintf("Unexpected single quote character before flag: %s", arg),
				"This typically happens when attempting to use single quotes to quote arguments with a shell that doesn't recognize single quotes. "+
					"Try using double quote characters to quote arguments instead.",
			)

		case !strings.HasPrefix(arg, "-") && buildOpts != nil:
			if equals := strings.IndexByte(arg, '='); equals != -1 {
				buildOpts.EntryPointsAdvanced = append(buildOpts.EntryPointsAdvanced, api.EntryPoint{
					OutputPath: arg[:equals],
					InputPath:  arg[equals+1:],
				})
			} else {
				buildOpts.EntryPoints = append(buildOpts.EntryPoints, arg)
			}

		default:
			bare := map[string]bool{
				"allow-overwrite":    true,
				"bundle":             true,
				"ignore-annotations": true,
				"keep-names":         true,
				"minify-identifiers": true,
				"minify-syntax":      true,
				"minify-whitespace":  true,
				"minify":             true,
				"preserve-symlinks":  true,
				"sourcemap":          true,
				"splitting":          true,
				"watch":              true,
			}

			equals := map[string]bool{
				"allow-overwrite":    true,
				"asset-names":        true,
				"banner":             true,
				"bundle":             true,
				"charset":            true,
				"chunk-names":        true,
				"color":              true,
				"conditions":         true,
				"entry-names":        true,
				"footer":             true,
				"format":             true,
				"global-name":        true,
				"ignore-annotations": true,
				"jsx-factory":        true,
				"jsx-fragment":       true,
				"jsx":                true,
				"keep-names":         true,
				"legal-comments":     true,
				"loader":             true,
				"log-level":          true,
				"log-limit":          true,
				"main-fields":        true,
				"mangle-cache":       true,
				"mangle-props":       true,
				"mangle-quoted":      true,
				"metafile":           true,
				"minify-identifiers": true,
				"minify-syntax":      true,
				"minify-whitespace":  true,
				"minify":             true,
				"outbase":            true,
				"outdir":             true,
				"outfile":            true,
				"platform":           true,
				"preserve-symlinks":  true,
				"public-path":        true,
				"reserve-props":      true,
				"resolve-extensions": true,
				"source-root":        true,
				"sourcefile":         true,
				"sourcemap":          true,
				"sources-content":    true,
				"splitting":          true,
				"target":             true,
				"tree-shaking":       true,
				"tsconfig-raw":       true,
				"tsconfig":           true,
				"watch":              true,
			}

			colon := map[string]bool{
				"banner":        true,
				"define":        true,
				"drop":          true,
				"external":      true,
				"footer":        true,
				"inject":        true,
				"loader":        true,
				"log-override":  true,
				"out-extension": true,
				"pure":          true,
				"supported":     true,
			}

			note := ""

			// Try to provide helpful hints when we can recognize the mistake
			switch {
			case arg == "-o":
				note = "Use \"--outfile=\" to configure the output file instead of \"-o\"."

			case arg == "-v":
				note = "Use \"--log-level=verbose\" to generate verbose logs instead of \"-v\"."

			case strings.HasPrefix(arg, "--"):
				if i := strings.IndexByte(arg, '='); i != -1 && colon[arg[2:i]] {
					note = fmt.Sprintf("Use %q instead of %q. Flags that can be re-specified multiple times use \":\" instead of \"=\".",
						arg[:i]+":"+arg[i+1:], arg)
				}

				if i := strings.IndexByte(arg, ':'); i != -1 && equals[arg[2:i]] {
					note = fmt.Sprintf("Use %q instead of %q. Flags that can only be specified once use \"=\" instead of \":\".",
						arg[:i]+"="+arg[i+1:], arg)
				}

			case strings.HasPrefix(arg, "-"):
				isValid := bare[arg[1:]]
				fix := "-" + arg

				if i := strings.IndexByte(arg, '='); i != -1 && equals[arg[1:i]] {
					isValid = true
				} else if i != -1 && colon[arg[1:i]] {
					isValid = true
					fix = fmt.Sprintf("-%s:%s", arg[:i], arg[i+1:])
				} else if i := strings.IndexByte(arg, ':'); i != -1 && colon[arg[1:i]] {
					isValid = true
				} else if i != -1 && equals[arg[1:i]] {
					isValid = true
					fix = fmt.Sprintf("-%s=%s", arg[:i], arg[i+1:])
				}

				if isValid {
					note = fmt.Sprintf("Use %q instead of %q. Flags are always specified with two dashes instead of one dash.",
						fix, arg)
				}
			}

			if buildOpts != nil {
				return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(fmt.Sprintf("Invalid build flag: %q", arg), note)
			} else {
				return parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(fmt.Sprintf("Invalid transform flag: %q", arg), note)
			}
		}
	}

	// If we're building, the last source map flag is "--sourcemap", and there
	// is no output path, change the source map option to "inline" because we're
	// going to be writing to stdout which can only represent a single file.
	if buildOpts != nil && hasBareSourceMapFlag && buildOpts.Outfile == "" && buildOpts.Outdir == "" {
		buildOpts.Sourcemap = api.SourceMapInline
	}

	return
}

func parseTargets(targets []string, arg string) (target api.Target, engines []api.Engine, err *cli_helpers.ErrorWithNote) {
	validTargets := map[string]api.Target{
		"esnext": api.ESNext,
		"es5":    api.ES5,
		"es6":    api.ES2015,
		"es2015": api.ES2015,
		"es2016": api.ES2016,
		"es2017": api.ES2017,
		"es2018": api.ES2018,
		"es2019": api.ES2019,
		"es2020": api.ES2020,
		"es2021": api.ES2021,
		"es2022": api.ES2022,
	}

outer:
	for _, value := range targets {
		if valid, ok := validTargets[strings.ToLower(value)]; ok {
			target = valid
			continue
		}

		for engine, name := range validEngines {
			if strings.HasPrefix(value, engine) {
				version := value[len(engine):]
				if version == "" {
					return 0, nil, cli_helpers.MakeErrorWithNote(
						fmt.Sprintf("Target %q is missing a version number in %q", value, arg),
						"",
					)
				}
				engines = append(engines, api.Engine{Name: name, Version: version})
				continue outer
			}
		}

		engines := make([]string, 0, len(validEngines))
		engines = append(engines, "\"esN\"")
		for key := range validEngines {
			engines = append(engines, fmt.Sprintf("%q", key+"N"))
		}
		sort.Strings(engines)
		return 0, nil, cli_helpers.MakeErrorWithNote(
			fmt.Sprintf("Invalid target %q in %q", value, arg),
			fmt.Sprintf("Valid values are %s, or %s where N is a version number.",
				strings.Join(engines[:len(engines)-1], ", "), engines[len(engines)-1]),
		)
	}
	return
}

// This returns either BuildOptions, TransformOptions, or an error
func parseOptionsForRun(osArgs []string) (*api.BuildOptions, *api.TransformOptions, parseOptionsExtras, *cli_helpers.ErrorWithNote) {
	// If there's an entry point or we're bundling, then we're building
	for _, arg := range osArgs {
		if !strings.HasPrefix(arg, "-") || arg == "--bundle" {
			options := newBuildOptions()

			// Apply defaults appropriate for the CLI
			options.LogLimit = 6
			options.LogLevel = api.LogLevelInfo
			options.Write = true

			extras, err := parseOptionsImpl(osArgs, &options, nil, kindInternal)
			if err != nil {
				return nil, nil, parseOptionsExtras{}, err
			}
			return &options, nil, extras, nil
		}
	}

	// Otherwise, we're transforming
	options := newTransformOptions()

	// Apply defaults appropriate for the CLI
	options.LogLimit = 6
	options.LogLevel = api.LogLevelInfo

	_, err := parseOptionsImpl(osArgs, nil, &options, kindInternal)
	if err != nil {
		return nil, nil, parseOptionsExtras{}, err
	}
	if options.Sourcemap != api.SourceMapNone && options.Sourcemap != api.SourceMapInline {
		var sourceMapMode string
		switch options.Sourcemap {
		case api.SourceMapExternal:
			sourceMapMode = "external"
		case api.SourceMapInlineAndExternal:
			sourceMapMode = "both"
		case api.SourceMapLinked:
			sourceMapMode = "linked"
		}
		return nil, nil, parseOptionsExtras{}, cli_helpers.MakeErrorWithNote(
			fmt.Sprintf("Use \"--sourcemap\" instead of \"--sourcemap=%s\" when transforming stdin", sourceMapMode),
			fmt.Sprintf("Using esbuild to transform stdin only generates one output file. You cannot use the %q source map mode "+
				"since that needs to generate two output files.", sourceMapMode),
		)
	}
	return nil, &options, parseOptionsExtras{}, nil
}

func splitWithEmptyCheck(s string, sep string) []string {
	// Special-case the empty string to return [] instead of [""]
	if s == "" {
		return []string{}
	}

	return strings.Split(s, sep)
}

func runImpl(osArgs []string) int {
	analyze := false
	analyzeVerbose := false
	end := 0

	for _, arg := range osArgs {
		// Special-case running a server
		if arg == "--serve" || strings.HasPrefix(arg, "--serve=") || strings.HasPrefix(arg, "--servedir=") {
			if err := serveImpl(osArgs); err != nil {
				logger.PrintErrorToStderr(osArgs, err.Error())
				return 1
			}
			return 0
		}

		// Special-case analyze just for our CLI
		if arg == "--analyze" {
			analyze = true
			analyzeVerbose = false
			continue
		}
		if arg == "--analyze=verbose" {
			analyze = true
			analyzeVerbose = true
			continue
		}

		osArgs[end] = arg
		end++
	}
	osArgs = osArgs[:end]

	buildOptions, transformOptions, extras, err := parseOptionsForRun(osArgs)

	switch {
	case buildOptions != nil:
		for _, key := range os.Environ() {
			// Read the "NODE_PATH" from the environment. This is part of node's
			// module resolution algorithm. Documentation for this can be found here:
			// https://nodejs.org/api/modules.html#modules_loading_from_the_global_folders
			if strings.HasPrefix(key, "NODE_PATH=") {
				value := key[len("NODE_PATH="):]
				separator := ":"
				if fs.CheckIfWindows() {
					// On Windows, NODE_PATH is delimited by semicolons instead of colons
					separator = ";"
				}
				buildOptions.NodePaths = splitWithEmptyCheck(value, separator)
				break
			}
		}

		// Read from stdin when there are no entry points
		if len(buildOptions.EntryPoints)+len(buildOptions.EntryPointsAdvanced) == 0 {
			if buildOptions.Stdin == nil {
				buildOptions.Stdin = &api.StdinOptions{}
			}
			bytes, err := ioutil.ReadAll(os.Stdin)
			if err != nil {
				logger.PrintErrorToStderr(osArgs, fmt.Sprintf(
					"Could not read from stdin: %s", err.Error()))
				return 1
			}
			buildOptions.Stdin.Contents = string(bytes)
			buildOptions.Stdin.ResolveDir, _ = os.Getwd()
		} else if buildOptions.Stdin != nil {
			if buildOptions.Stdin.Sourcefile != "" {
				logger.PrintErrorToStderr(osArgs,
					"\"sourcefile\" only applies when reading from stdin")
			} else {
				logger.PrintErrorToStderr(osArgs,
					"\"loader\" without extension only applies when reading from stdin")
			}
			return 1
		}

		// Validate the metafile absolute path and directory ahead of time so we
		// don't write any output files if it's incorrect. That makes this API
		// option consistent with how we handle all other API options.
		var writeMetafile func(string)
		if extras.metafile != nil {
			var metafileAbsPath string
			var metafileAbsDir string

			if buildOptions.Outfile == "" && buildOptions.Outdir == "" {
				// Cannot use "metafile" when writing to stdout
				logger.PrintErrorToStderr(osArgs, "Cannot use \"metafile\" without an output path")
				return 1
			}
			realFS, realFSErr := fs.RealFS(fs.RealFSOptions{AbsWorkingDir: buildOptions.AbsWorkingDir})
			if realFSErr == nil {
				absPath, ok := realFS.Abs(*extras.metafile)
				if !ok {
					logger.PrintErrorToStderr(osArgs, fmt.Sprintf("Invalid metafile path: %s", *extras.metafile))
					return 1
				}
				metafileAbsPath = absPath
				metafileAbsDir = realFS.Dir(absPath)
			} else {
				// Don't fail in this case since the error will be reported by "api.Build"
			}

			writeMetafile = func(json string) {
				if json == "" || realFSErr != nil {
					return // Don't write out the metafile on build errors
				}
				if err != nil {
					// This should already have been checked above
					panic(err.Text)
				}
				fs.BeforeFileOpen()
				defer fs.AfterFileClose()
				if err := fs.MkdirAll(realFS, metafileAbsDir, 0755); err != nil {
					logger.PrintErrorToStderr(osArgs, fmt.Sprintf(
						"Failed to create output directory: %s", err.Error()))
				} else {
					if err := ioutil.WriteFile(metafileAbsPath, []byte(json), 0644); err != nil {
						logger.PrintErrorToStderr(osArgs, fmt.Sprintf(
							"Failed to write to output file: %s", err.Error()))
					}
				}
			}
		}

		// Also validate the mangle cache absolute path and directory ahead of time
		// for the same reason
		var writeMangleCache func(map[string]interface{})
		if extras.mangleCache != nil {
			var mangleCacheAbsPath string
			var mangleCacheAbsDir string
			var mangleCacheOrder []string
			realFS, realFSErr := fs.RealFS(fs.RealFSOptions{AbsWorkingDir: buildOptions.AbsWorkingDir})
			if realFSErr == nil {
				absPath, ok := realFS.Abs(*extras.mangleCache)
				if !ok {
					logger.PrintErrorToStderr(osArgs, fmt.Sprintf("Invalid mangle cache path: %s", *extras.mangleCache))
					return 1
				}
				mangleCacheAbsPath = absPath
				mangleCacheAbsDir = realFS.Dir(absPath)
				buildOptions.MangleCache, mangleCacheOrder = parseMangleCache(osArgs, realFS, *extras.mangleCache)
				if buildOptions.MangleCache == nil {
					return 1 // Stop now if parsing failed
				}
			} else {
				// Don't fail in this case since the error will be reported by "api.Build"
			}

			writeMangleCache = func(mangleCache map[string]interface{}) {
				if mangleCache == nil || realFSErr != nil {
					return // Don't write out the metafile on build errors
				}
				if err != nil {
					// This should already have been checked above
					panic(err.Text)
				}
				fs.BeforeFileOpen()
				defer fs.AfterFileClose()
				if err := fs.MkdirAll(realFS, mangleCacheAbsDir, 0755); err != nil {
					logger.PrintErrorToStderr(osArgs, fmt.Sprintf(
						"Failed to create output directory: %s", err.Error()))
				} else {
					bytes := printMangleCache(mangleCache, mangleCacheOrder, buildOptions.Charset == api.CharsetASCII)
					if err := ioutil.WriteFile(mangleCacheAbsPath, bytes, 0644); err != nil {
						logger.PrintErrorToStderr(osArgs, fmt.Sprintf(
							"Failed to write to output file: %s", err.Error()))
					}
				}
			}

			// Write out the metafile whenever we rebuild
			if buildOptions.Watch != nil {
				buildOptions.Watch.OnRebuild = func(result api.BuildResult) {
					writeMetafile(result.Metafile)
				}
			}
		}

		// Always generate a metafile if we're analyzing, even if it won't be written out
		if analyze {
			buildOptions.Metafile = true
		}

		// Run the build
		result := api.Build(*buildOptions)

		// Print the analysis after the build
		if analyze {
			logger.PrintTextWithColor(os.Stderr, logger.OutputOptionsForArgs(osArgs).Color, func(colors logger.Colors) string {
				return api.AnalyzeMetafile(result.Metafile, api.AnalyzeMetafileOptions{
					Color:   colors != logger.Colors{},
					Verbose: analyzeVerbose,
				})
			})
			os.Stderr.WriteString("\n")
		}

		// Write the metafile to the file system
		if writeMetafile != nil {
			writeMetafile(result.Metafile)
		}

		// Write the mangle cache to the file system
		if writeMangleCache != nil {
			writeMangleCache(result.MangleCache)
		}

		// Do not exit if we're in watch mode
		if buildOptions.Watch != nil {
			<-make(chan bool)
		}

		// Stop if there were errors
		if len(result.Errors) > 0 {
			return 1
		}

	case transformOptions != nil:
		// Read the input from stdin
		bytes, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			logger.PrintErrorToStderr(osArgs, fmt.Sprintf(
				"Could not read from stdin: %s", err.Error()))
			return 1
		}

		// Run the transform and stop if there were errors
		result := api.Transform(string(bytes), *transformOptions)
		if len(result.Errors) > 0 {
			return 1
		}

		// Write the output to stdout
		os.Stdout.Write(result.Code)

	case err != nil:
		msg := logger.Msg{
			Kind: logger.Error,
			Data: logger.MsgData{Text: err.Text},
		}
		if err.Note != "" {
			msg.Notes = []logger.MsgData{{Text: err.Note}}
		}
		logger.PrintMessageToStderr(osArgs, msg)
		return 1
	}

	return 0
}

func parseServeOptionsImpl(osArgs []string) (api.ServeOptions, []string, error) {
	host := ""
	portText := "0"
	servedir := ""

	// Filter out server-specific flags
	filteredArgs := make([]string, 0, len(osArgs))
	for _, arg := range osArgs {
		if arg == "--serve" {
			// Just ignore this flag
		} else if strings.HasPrefix(arg, "--serve=") {
			portText = arg[len("--serve="):]
		} else if strings.HasPrefix(arg, "--servedir=") {
			servedir = arg[len("--servedir="):]
		} else {
			filteredArgs = append(filteredArgs, arg)
		}
	}

	// Specifying the host is optional
	if strings.ContainsRune(portText, ':') {
		var err error
		host, portText, err = net.SplitHostPort(portText)
		if err != nil {
			return api.ServeOptions{}, nil, err
		}
	}

	// Parse the port
	port, err := strconv.ParseInt(portText, 10, 32)
	if err != nil {
		return api.ServeOptions{}, nil, err
	}
	if port < 0 || port > 0xFFFF {
		return api.ServeOptions{}, nil, fmt.Errorf("Invalid port number: %s", portText)
	}

	return api.ServeOptions{
		Port:     uint16(port),
		Host:     host,
		Servedir: servedir,
	}, filteredArgs, nil
}

func serveImpl(osArgs []string) error {
	serveOptions, filteredArgs, err := parseServeOptionsImpl(osArgs)
	if err != nil {
		return err
	}

	options := newBuildOptions()

	// Apply defaults appropriate for the CLI
	options.LogLimit = 5
	options.LogLevel = api.LogLevelInfo

	if _, err := parseOptionsImpl(filteredArgs, &options, nil, kindInternal); err != nil {
		logger.PrintErrorToStderr(filteredArgs, err.Text)
		return errors.New(err.Text)
	}

	serveOptions.OnRequest = func(args api.ServeOnRequestArgs) {
		logger.PrintText(os.Stderr, logger.LevelInfo, filteredArgs, func(colors logger.Colors) string {
			statusColor := colors.Red
			if args.Status >= 200 && args.Status <= 299 {
				statusColor = colors.Green
			} else if args.Status >= 300 && args.Status <= 399 {
				statusColor = colors.Yellow
			}
			return fmt.Sprintf("%s%s - %q %s%d%s [%dms]%s\n",
				colors.Dim, args.RemoteAddress, args.Method+" "+args.Path,
				statusColor, args.Status, colors.Dim, args.TimeInMS, colors.Reset)
		})
	}

	result, err := api.Serve(serveOptions, options)
	if err != nil {
		return err
	}

	// Show what actually got bound if the port was 0
	logger.PrintText(os.Stderr, logger.LevelInfo, filteredArgs, func(colors logger.Colors) string {
		var hosts []string
		sb := strings.Builder{}
		sb.WriteString(colors.Reset)

		// If this is "0.0.0.0" or "::", list all relevant IP addresses
		if ip := net.ParseIP(result.Host); ip != nil && ip.IsUnspecified() {
			if addrs, err := net.InterfaceAddrs(); err == nil {
				for _, addr := range addrs {
					if addr, ok := addr.(*net.IPNet); ok && (addr.IP.To4() != nil) == (ip.To4() != nil) && !addr.IP.IsLinkLocalUnicast() {
						hosts = append(hosts, addr.IP.String())
					}
				}
			}
		}

		// Otherwise, just list the one IP address
		if len(hosts) == 0 {
			hosts = append(hosts, result.Host)
		}

		// Determine the host kinds
		kinds := make([]string, len(hosts))
		maxLen := 0
		for i, host := range hosts {
			kind := "Network"
			if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
				kind = "Local"
			}
			kinds[i] = kind
			if len(kind) > maxLen {
				maxLen = len(kind)
			}
		}

		// Pretty-print the host list
		for i, kind := range kinds {
			sb.WriteString(fmt.Sprintf("\n > %s:%s %shttp://%s/%s",
				kind, strings.Repeat(" ", maxLen-len(kind)), colors.Underline,
				net.JoinHostPort(hosts[i], fmt.Sprintf("%d", result.Port)), colors.Reset))
		}

		sb.WriteString("\n\n")
		return sb.String()
	})
	return result.Wait()
}

func parseLogLevel(value string, arg string) (api.LogLevel, *cli_helpers.ErrorWithNote) {
	switch value {
	case "verbose":
		return api.LogLevelVerbose, nil
	case "debug":
		return api.LogLevelDebug, nil
	case "info":
		return api.LogLevelInfo, nil
	case "warning":
		return api.LogLevelWarning, nil
	case "error":
		return api.LogLevelError, nil
	case "silent":
		return api.LogLevelSilent, nil
	default:
		return api.LogLevelSilent, cli_helpers.MakeErrorWithNote(
			fmt.Sprintf("Invalid value %q in %q", value, arg),
			"Valid values are \"verbose\", \"debug\", \"info\", \"warning\", \"error\", or \"silent\".",
		)
	}
}
