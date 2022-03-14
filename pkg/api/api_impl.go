package api

import (
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/evanw/esbuild/internal/api_helpers"
	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/bundler"
	"github.com/evanw/esbuild/internal/cache"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/graph"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/js_parser"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/resolver"
)

func validatePathTemplate(template string) []config.PathTemplate {
	if template == "" {
		return nil
	}
	template = "./" + strings.ReplaceAll(template, "\\", "/")

	parts := make([]config.PathTemplate, 0, 4)
	search := 0

	// Split by placeholders
	for search < len(template) {
		// Jump to the next "["
		if found := strings.IndexByte(template[search:], '['); found == -1 {
			break
		} else {
			search += found
		}
		head, tail := template[:search], template[search:]
		placeholder := config.NoPlaceholder

		// Check for a placeholder
		switch {
		case strings.HasPrefix(tail, "[dir]"):
			placeholder = config.DirPlaceholder
			search += len("[dir]")

		case strings.HasPrefix(tail, "[name]"):
			placeholder = config.NamePlaceholder
			search += len("[name]")

		case strings.HasPrefix(tail, "[hash]"):
			placeholder = config.HashPlaceholder
			search += len("[hash]")

		case strings.HasPrefix(tail, "[ext]"):
			placeholder = config.ExtPlaceholder
			search += len("[ext]")

		default:
			// Skip past the "[" so we don't find it again
			search++
			continue
		}

		// Add a part for everything up to and including this placeholder
		parts = append(parts, config.PathTemplate{
			Data:        head,
			Placeholder: placeholder,
		})

		// Reset the search after this placeholder
		template = template[search:]
		search = 0
	}

	// Append any remaining data as a part without a placeholder
	if search < len(template) {
		parts = append(parts, config.PathTemplate{
			Data:        template,
			Placeholder: config.NoPlaceholder,
		})
	}

	return parts
}

func validatePlatform(value Platform) config.Platform {
	switch value {
	case PlatformBrowser:
		return config.PlatformBrowser
	case PlatformNode:
		return config.PlatformNode
	case PlatformNeutral:
		return config.PlatformNeutral
	default:
		panic("Invalid platform")
	}
}

func validateFormat(value Format) config.Format {
	switch value {
	case FormatDefault:
		return config.FormatPreserve
	case FormatIIFE:
		return config.FormatIIFE
	case FormatCommonJS:
		return config.FormatCommonJS
	case FormatESModule:
		return config.FormatESModule
	default:
		panic("Invalid format")
	}
}

func validateSourceMap(value SourceMap) config.SourceMap {
	switch value {
	case SourceMapNone:
		return config.SourceMapNone
	case SourceMapLinked:
		return config.SourceMapLinkedWithComment
	case SourceMapInline:
		return config.SourceMapInline
	case SourceMapExternal:
		return config.SourceMapExternalWithoutComment
	case SourceMapInlineAndExternal:
		return config.SourceMapInlineAndExternal
	default:
		panic("Invalid source map")
	}
}

func validateLegalComments(value LegalComments, bundle bool) config.LegalComments {
	switch value {
	case LegalCommentsDefault:
		if bundle {
			return config.LegalCommentsEndOfFile
		} else {
			return config.LegalCommentsInline
		}
	case LegalCommentsNone:
		return config.LegalCommentsNone
	case LegalCommentsInline:
		return config.LegalCommentsInline
	case LegalCommentsEndOfFile:
		return config.LegalCommentsEndOfFile
	case LegalCommentsLinked:
		return config.LegalCommentsLinkedWithComment
	case LegalCommentsExternal:
		return config.LegalCommentsExternalWithoutComment
	default:
		panic("Invalid source map")
	}
}

func validateColor(value StderrColor) logger.UseColor {
	switch value {
	case ColorIfTerminal:
		return logger.ColorIfTerminal
	case ColorNever:
		return logger.ColorNever
	case ColorAlways:
		return logger.ColorAlways
	default:
		panic("Invalid color")
	}
}

func validateLogLevel(value LogLevel) logger.LogLevel {
	switch value {
	case LogLevelVerbose:
		return logger.LevelVerbose
	case LogLevelDebug:
		return logger.LevelDebug
	case LogLevelInfo:
		return logger.LevelInfo
	case LogLevelWarning:
		return logger.LevelWarning
	case LogLevelError:
		return logger.LevelError
	case LogLevelSilent:
		return logger.LevelSilent
	default:
		panic("Invalid log level")
	}
}

func validateASCIIOnly(value Charset) bool {
	switch value {
	case CharsetDefault, CharsetASCII:
		return true
	case CharsetUTF8:
		return false
	default:
		panic("Invalid charset")
	}
}

func validateTreeShaking(value TreeShaking, bundle bool, format Format) bool {
	switch value {
	case TreeShakingDefault:
		// If we're in an IIFE then there's no way to concatenate additional code
		// to the end of our output so we assume tree shaking is safe. And when
		// bundling we assume that tree shaking is safe because if you want to add
		// code to the bundle, you should be doing that by including it in the
		// bundle instead of concatenating it afterward, so we also assume tree
		// shaking is safe then. Otherwise we assume tree shaking is not safe.
		return bundle || format == FormatIIFE
	case TreeShakingFalse:
		return false
	case TreeShakingTrue:
		return true
	default:
		panic("Invalid tree shaking")
	}
}

func validateLoader(value Loader) config.Loader {
	switch value {
	case LoaderNone:
		return config.LoaderNone
	case LoaderJS:
		return config.LoaderJS
	case LoaderJSX:
		return config.LoaderJSX
	case LoaderTS:
		return config.LoaderTS
	case LoaderTSX:
		return config.LoaderTSX
	case LoaderJSON:
		return config.LoaderJSON
	case LoaderText:
		return config.LoaderText
	case LoaderBase64:
		return config.LoaderBase64
	case LoaderDataURL:
		return config.LoaderDataURL
	case LoaderFile:
		return config.LoaderFile
	case LoaderBinary:
		return config.LoaderBinary
	case LoaderCSS:
		return config.LoaderCSS
	case LoaderDefault:
		return config.LoaderDefault
	default:
		panic("Invalid loader")
	}
}

func validateEngine(value EngineName) compat.Engine {
	switch value {
	case EngineChrome:
		return compat.Chrome
	case EngineEdge:
		return compat.Edge
	case EngineFirefox:
		return compat.Firefox
	case EngineIOS:
		return compat.IOS
	case EngineNode:
		return compat.Node
	case EngineSafari:
		return compat.Safari
	default:
		panic("Invalid loader")
	}
}

var versionRegex = regexp.MustCompile(`^([0-9]+)(?:\.([0-9]+))?(?:\.([0-9]+))?$`)

func validateFeatures(log logger.Log, target Target, engines []Engine) (config.TargetFromAPI, compat.JSFeature, compat.CSSFeature, string) {
	if target == DefaultTarget && len(engines) == 0 {
		return config.TargetWasUnconfigured, 0, 0, ""
	}

	constraints := make(map[compat.Engine][]int)
	targets := make([]string, 0, 1+len(engines))
	targetFromAPI := config.TargetWasConfigured

	switch target {
	case ES5:
		constraints[compat.ES] = []int{5}
	case ES2015:
		constraints[compat.ES] = []int{2015}
	case ES2016:
		constraints[compat.ES] = []int{2016}
	case ES2017:
		constraints[compat.ES] = []int{2017}
	case ES2018:
		constraints[compat.ES] = []int{2018}
	case ES2019:
		constraints[compat.ES] = []int{2019}
	case ES2020:
		constraints[compat.ES] = []int{2020}
	case ES2021:
		constraints[compat.ES] = []int{2021}
	case ES2022:
		constraints[compat.ES] = []int{2022}
	case ESNext:
		targetFromAPI = config.TargetWasConfiguredIncludingESNext
	case DefaultTarget:
	default:
		panic("Invalid target")
	}

	for _, engine := range engines {
		if match := versionRegex.FindStringSubmatch(engine.Version); match != nil {
			if major, err := strconv.Atoi(match[1]); err == nil {
				version := []int{major}
				if minor, err := strconv.Atoi(match[2]); err == nil {
					version = append(version, minor)
				}
				if patch, err := strconv.Atoi(match[3]); err == nil {
					version = append(version, patch)
				}
				switch engine.Name {
				case EngineChrome:
					constraints[compat.Chrome] = version
				case EngineEdge:
					constraints[compat.Edge] = version
				case EngineFirefox:
					constraints[compat.Firefox] = version
				case EngineIE:
					constraints[compat.IE] = version
				case EngineIOS:
					constraints[compat.IOS] = version
				case EngineNode:
					constraints[compat.Node] = version
				case EngineOpera:
					constraints[compat.Opera] = version
				case EngineSafari:
					constraints[compat.Safari] = version
				default:
					panic("Invalid engine name")
				}
				continue
			}
		}

		log.Add(logger.Error, nil, logger.Range{}, fmt.Sprintf("Invalid version: %q", engine.Version))
	}

	for engine, version := range constraints {
		var text string
		switch len(version) {
		case 1:
			text = fmt.Sprintf("%s%d", engine.String(), version[0])
		case 2:
			text = fmt.Sprintf("%s%d.%d", engine.String(), version[0], version[1])
		case 3:
			text = fmt.Sprintf("%s%d.%d.%d", engine.String(), version[0], version[1], version[2])
		}
		targets = append(targets, fmt.Sprintf("%q", text))
	}

	sort.Strings(targets)
	targetEnv := strings.Join(targets, ", ")

	return targetFromAPI, compat.UnsupportedJSFeatures(constraints), compat.UnsupportedCSSFeatures(constraints), targetEnv
}

func validateGlobalName(log logger.Log, text string) []string {
	if text != "" {
		source := logger.Source{
			KeyPath:    logger.Path{Text: "(global path)"},
			PrettyPath: "(global name)",
			Contents:   text,
		}

		if result, ok := js_parser.ParseGlobalName(log, source); ok {
			return result
		}
	}

	return nil
}

func validateRegex(log logger.Log, what string, value string) *regexp.Regexp {
	if value == "" {
		return nil
	}
	regex, err := regexp.Compile(value)
	if err != nil {
		log.Add(logger.Error, nil, logger.Range{},
			fmt.Sprintf("The %q setting is not a valid Go regular expression: %s", what, value))
		return nil
	}
	return regex
}

func validateExternals(log logger.Log, fs fs.FS, paths []string) config.ExternalSettings {
	result := config.ExternalSettings{
		PreResolve:  config.ExternalMatchers{Exact: make(map[string]bool)},
		PostResolve: config.ExternalMatchers{Exact: make(map[string]bool)},
	}

	for _, path := range paths {
		if index := strings.IndexByte(path, '*'); index != -1 {
			// Wildcard behavior
			if strings.ContainsRune(path[index+1:], '*') {
				log.Add(logger.Error, nil, logger.Range{}, fmt.Sprintf("External path %q cannot have more than one \"*\" wildcard", path))
			} else {
				result.PreResolve.Patterns = append(result.PreResolve.Patterns, config.WildcardPattern{Prefix: path[:index], Suffix: path[index+1:]})
				if !resolver.IsPackagePath(path) {
					if absPath := validatePath(log, fs, path, "external path"); absPath != "" {
						if absIndex := strings.IndexByte(absPath, '*'); absIndex != -1 && !strings.ContainsRune(absPath[absIndex+1:], '*') {
							result.PostResolve.Patterns = append(result.PostResolve.Patterns, config.WildcardPattern{Prefix: absPath[:absIndex], Suffix: absPath[absIndex+1:]})
						}
					}
				}
			}
		} else {
			// Non-wildcard behavior
			result.PreResolve.Exact[path] = true
			if resolver.IsPackagePath(path) {
				result.PreResolve.Patterns = append(result.PreResolve.Patterns, config.WildcardPattern{Prefix: path + "/"})
			} else if absPath := validatePath(log, fs, path, "external path"); absPath != "" {
				result.PostResolve.Exact[absPath] = true
			}
		}
	}

	return result
}

func isValidExtension(ext string) bool {
	return len(ext) >= 2 && ext[0] == '.' && ext[len(ext)-1] != '.'
}

func validateResolveExtensions(log logger.Log, order []string) []string {
	if order == nil {
		return []string{".tsx", ".ts", ".jsx", ".js", ".css", ".json"}
	}
	for _, ext := range order {
		if !isValidExtension(ext) {
			log.Add(logger.Error, nil, logger.Range{}, fmt.Sprintf("Invalid file extension: %q", ext))
		}
	}
	return order
}

func validateLoaders(log logger.Log, loaders map[string]Loader) map[string]config.Loader {
	result := bundler.DefaultExtensionToLoaderMap()
	if loaders != nil {
		for ext, loader := range loaders {
			if !isValidExtension(ext) {
				log.Add(logger.Error, nil, logger.Range{}, fmt.Sprintf("Invalid file extension: %q", ext))
			}
			result[ext] = validateLoader(loader)
		}
	}
	return result
}

func validateJSXExpr(log logger.Log, text string, name string, kind js_parser.JSXExprKind) config.JSXExpr {
	if expr, ok := js_parser.ParseJSXExpr(text, kind); ok {
		return expr
	}
	log.Add(logger.Error, nil, logger.Range{}, fmt.Sprintf("Invalid JSX %s: %q", name, text))
	return config.JSXExpr{}
}

func validateDefines(
	log logger.Log,
	defines map[string]string,
	pureFns []string,
	platform Platform,
	minify bool,
	drop Drop,
) (*config.ProcessedDefines, []config.InjectedDefine) {
	rawDefines := make(map[string]config.DefineData)
	var valueToInject map[string]config.InjectedDefine
	var definesToInject []string

	for key, value := range defines {
		// The key must be a dot-separated identifier list
		for _, part := range strings.Split(key, ".") {
			if !js_lexer.IsIdentifier(part) {
				if part == key {
					log.Add(logger.Error, nil, logger.Range{}, fmt.Sprintf("The define key %q must be a valid identifier", key))
				} else {
					log.Add(logger.Error, nil, logger.Range{}, fmt.Sprintf("The define key %q contains invalid identifier %q", key, part))
				}
				continue
			}
		}

		// Allow substituting for an identifier
		if js_lexer.IsIdentifier(value) {
			if _, ok := js_lexer.Keywords[value]; !ok {
				switch value {
				case "undefined":
					rawDefines[key] = config.DefineData{
						DefineFunc: func(config.DefineArgs) js_ast.E { return js_ast.EUndefinedShared },
					}
				case "NaN":
					rawDefines[key] = config.DefineData{
						DefineFunc: func(config.DefineArgs) js_ast.E { return &js_ast.ENumber{Value: math.NaN()} },
					}
				case "Infinity":
					rawDefines[key] = config.DefineData{
						DefineFunc: func(config.DefineArgs) js_ast.E { return &js_ast.ENumber{Value: math.Inf(1)} },
					}
				default:
					name := value // The closure must close over a variable inside the loop
					rawDefines[key] = config.DefineData{
						DefineFunc: func(args config.DefineArgs) js_ast.E {
							return &js_ast.EIdentifier{Ref: args.FindSymbol(args.Loc, name)}
						},
					}
				}
				continue
			}
		}

		// Parse the value as JSON
		source := logger.Source{Contents: value}
		expr, ok := js_parser.ParseJSON(logger.NewDeferLog(logger.DeferLogAll), source, js_parser.JSONOptions{})
		if !ok {
			log.Add(logger.Error, nil, logger.Range{}, fmt.Sprintf("Invalid define value (must be valid JSON syntax or a single identifier): %s", value))
			continue
		}

		var fn config.DefineFunc
		switch e := expr.Data.(type) {
		// These values are inserted inline, and can participate in constant folding
		case *js_ast.ENull:
			fn = func(config.DefineArgs) js_ast.E { return js_ast.ENullShared }
		case *js_ast.EBoolean:
			fn = func(config.DefineArgs) js_ast.E { return &js_ast.EBoolean{Value: e.Value} }
		case *js_ast.EString:
			fn = func(config.DefineArgs) js_ast.E { return &js_ast.EString{Value: e.Value} }
		case *js_ast.ENumber:
			fn = func(config.DefineArgs) js_ast.E { return &js_ast.ENumber{Value: e.Value} }

		// These values are extracted into a shared symbol reference
		case *js_ast.EArray, *js_ast.EObject:
			definesToInject = append(definesToInject, key)
			if valueToInject == nil {
				valueToInject = make(map[string]config.InjectedDefine)
			}
			valueToInject[key] = config.InjectedDefine{Source: source, Data: e, Name: key}
			continue
		}

		rawDefines[key] = config.DefineData{DefineFunc: fn}
	}

	// Sort injected defines for determinism, since the imports will be injected
	// into every file in the order that we return them from this function
	var injectedDefines []config.InjectedDefine
	if len(definesToInject) > 0 {
		injectedDefines = make([]config.InjectedDefine, len(definesToInject))
		sort.Strings(definesToInject)
		for i, key := range definesToInject {
			index := i // Capture this for the closure below
			injectedDefines[i] = valueToInject[key]
			rawDefines[key] = config.DefineData{DefineFunc: func(args config.DefineArgs) js_ast.E {
				return &js_ast.EIdentifier{Ref: args.SymbolForDefine(index)}
			}}
		}
	}

	// If we're bundling for the browser, add a special-cased define for
	// "process.env.NODE_ENV" that is "development" when not minifying and
	// "production" when minifying. This is a convention from the React world
	// that must be handled to avoid all React code crashing instantly. This
	// is only done if it's not already defined so that you can override it if
	// necessary.
	if platform == PlatformBrowser {
		if _, process := rawDefines["process"]; !process {
			if _, processEnv := rawDefines["process.env"]; !processEnv {
				if _, processEnvNodeEnv := rawDefines["process.env.NODE_ENV"]; !processEnvNodeEnv {
					var value []uint16
					if minify {
						value = helpers.StringToUTF16("production")
					} else {
						value = helpers.StringToUTF16("development")
					}
					rawDefines["process.env.NODE_ENV"] = config.DefineData{
						DefineFunc: func(args config.DefineArgs) js_ast.E {
							return &js_ast.EString{Value: value}
						},
					}
				}
			}
		}
	}

	// If we're dropping all console API calls, replace each one with undefined
	if (drop & DropConsole) != 0 {
		define := rawDefines["console"]
		define.MethodCallsMustBeReplacedWithUndefined = true
		rawDefines["console"] = define
	}

	for _, key := range pureFns {
		// The key must be a dot-separated identifier list
		for _, part := range strings.Split(key, ".") {
			if !js_lexer.IsIdentifier(part) {
				log.Add(logger.Error, nil, logger.Range{}, fmt.Sprintf("Invalid pure function: %q", key))
				continue
			}
		}

		// Merge with any previously-specified defines
		define := rawDefines[key]
		define.CallCanBeUnwrappedIfUnused = true
		rawDefines[key] = define
	}

	// Processing defines is expensive. Process them once here so the same object
	// can be shared between all parsers we create using these arguments.
	processed := config.ProcessDefines(rawDefines)
	return &processed, injectedDefines
}

func validatePath(log logger.Log, fs fs.FS, relPath string, pathKind string) string {
	if relPath == "" {
		return ""
	}
	absPath, ok := fs.Abs(relPath)
	if !ok {
		log.Add(logger.Error, nil, logger.Range{}, fmt.Sprintf("Invalid %s: %s", pathKind, relPath))
	}
	return absPath
}

func validateOutputExtensions(log logger.Log, outExtensions map[string]string) (js string, css string) {
	for key, value := range outExtensions {
		if !isValidExtension(value) {
			log.Add(logger.Error, nil, logger.Range{}, fmt.Sprintf("Invalid output extension: %q", value))
		}
		switch key {
		case ".js":
			js = value
		case ".css":
			css = value
		default:
			log.Add(logger.Error, nil, logger.Range{}, fmt.Sprintf("Invalid output extension: %q (valid: .css, .js)", key))
		}
	}
	return
}

func validateBannerOrFooter(log logger.Log, name string, values map[string]string) (js string, css string) {
	for key, value := range values {
		switch key {
		case "js":
			js = value
		case "css":
			css = value
		default:
			log.Add(logger.Error, nil, logger.Range{}, fmt.Sprintf("Invalid %s file type: %q (valid: css, js)", name, key))
		}
	}
	return
}

func convertLocationToPublic(loc *logger.MsgLocation) *Location {
	if loc != nil {
		return &Location{
			File:       loc.File,
			Namespace:  loc.Namespace,
			Line:       loc.Line,
			Column:     loc.Column,
			Length:     loc.Length,
			LineText:   loc.LineText,
			Suggestion: loc.Suggestion,
		}
	}
	return nil
}

func convertMessagesToPublic(kind logger.MsgKind, msgs []logger.Msg) []Message {
	var filtered []Message
	for _, msg := range msgs {
		if msg.Kind == kind {
			var notes []Note
			for _, note := range msg.Notes {
				notes = append(notes, Note{
					Text:     note.Text,
					Location: convertLocationToPublic(note.Location),
				})
			}
			filtered = append(filtered, Message{
				PluginName: msg.PluginName,
				Text:       msg.Data.Text,
				Location:   convertLocationToPublic(msg.Data.Location),
				Notes:      notes,
				Detail:     msg.Data.UserDetail,
			})
		}
	}
	return filtered
}

func convertLocationToInternal(loc *Location) *logger.MsgLocation {
	if loc != nil {
		namespace := loc.Namespace
		if namespace == "" {
			namespace = "file"
		}
		return &logger.MsgLocation{
			File:       loc.File,
			Namespace:  namespace,
			Line:       loc.Line,
			Column:     loc.Column,
			Length:     loc.Length,
			LineText:   loc.LineText,
			Suggestion: loc.Suggestion,
		}
	}
	return nil
}

func convertMessagesToInternal(msgs []logger.Msg, kind logger.MsgKind, messages []Message) []logger.Msg {
	for _, message := range messages {
		var notes []logger.MsgData
		for _, note := range message.Notes {
			notes = append(notes, logger.MsgData{
				Text:     note.Text,
				Location: convertLocationToInternal(note.Location),
			})
		}
		msgs = append(msgs, logger.Msg{
			PluginName: message.PluginName,
			Kind:       kind,
			Data: logger.MsgData{
				Text:       message.Text,
				Location:   convertLocationToInternal(message.Location),
				UserDetail: message.Detail,
			},
			Notes: notes,
		})
	}
	return msgs
}

func cloneMangleCache(log logger.Log, mangleCache map[string]interface{}) map[string]interface{} {
	if mangleCache == nil {
		return nil
	}
	clone := make(map[string]interface{}, len(mangleCache))
	for k, v := range mangleCache {
		if v == "__proto__" {
			// This could cause problems for our binary serialization protocol. It's
			// also unnecessary because we already avoid mangling this property name.
			log.Add(logger.Error, nil, logger.Range{},
				fmt.Sprintf("Invalid identifier name %q in mangle cache", k))
		} else if _, ok := v.(string); ok || v == false {
			clone[k] = v
		} else {
			log.Add(logger.Error, nil, logger.Range{},
				fmt.Sprintf("Expected %q in mangle cache to map to either a string or false", k))
		}
	}
	return clone
}

////////////////////////////////////////////////////////////////////////////////
// Build API

type internalBuildResult struct {
	result    BuildResult
	watchData fs.WatchData
	options   config.Options
}

func buildImpl(buildOpts BuildOptions) internalBuildResult {
	start := time.Now()
	logOptions := logger.OutputOptions{
		IncludeSource: true,
		MessageLimit:  buildOpts.LogLimit,
		Color:         validateColor(buildOpts.Color),
		LogLevel:      validateLogLevel(buildOpts.LogLevel),
	}
	log := logger.NewStderrLog(logOptions)

	// Validate that the current working directory is an absolute path
	realFS, err := fs.RealFS(fs.RealFSOptions{
		AbsWorkingDir: buildOpts.AbsWorkingDir,

		// This is a long-lived file system object so do not cache calls to
		// ReadDirectory() (they are normally cached for the duration of a build
		// for performance).
		DoNotCache: true,
	})
	if err != nil {
		log.Add(logger.Error, nil, logger.Range{}, err.Error())
		return internalBuildResult{result: BuildResult{Errors: convertMessagesToPublic(logger.Error, log.Done())}}
	}

	// Do not re-evaluate plugins when rebuilding. Also make sure the working
	// directory doesn't change, since breaking that invariant would break the
	// validation that we just did above.
	caches := cache.MakeCacheSet()
	oldAbsWorkingDir := buildOpts.AbsWorkingDir
	plugins, onEndCallbacks, finalizeBuildOptions := loadPlugins(&buildOpts, realFS, log, caches)
	if buildOpts.AbsWorkingDir != oldAbsWorkingDir {
		panic("Mutating \"AbsWorkingDir\" is not allowed")
	}

	internalResult := rebuildImpl(buildOpts, caches, plugins, finalizeBuildOptions, onEndCallbacks, logOptions, log, false /* isRebuild */)

	// Print a summary of the generated files to stderr. Except don't do
	// this if the terminal is already being used for something else.
	if logOptions.LogLevel <= logger.LevelInfo && len(internalResult.result.OutputFiles) > 0 &&
		buildOpts.Watch == nil && !buildOpts.Incremental && !internalResult.options.WriteToStdout {
		printSummary(logOptions, internalResult.result.OutputFiles, start)
	}

	return internalResult
}

func prettyPrintByteCount(n int) string {
	var size string
	if n < 1024 {
		size = fmt.Sprintf("%db ", n)
	} else if n < 1024*1024 {
		size = fmt.Sprintf("%.1fkb", float64(n)/(1024))
	} else if n < 1024*1024*1024 {
		size = fmt.Sprintf("%.1fmb", float64(n)/(1024*1024))
	} else {
		size = fmt.Sprintf("%.1fgb", float64(n)/(1024*1024*1024))
	}
	return size
}

func printSummary(logOptions logger.OutputOptions, outputFiles []OutputFile, start time.Time) {
	var table logger.SummaryTable = make([]logger.SummaryTableEntry, len(outputFiles))

	if len(outputFiles) > 0 {
		if cwd, err := os.Getwd(); err == nil {
			if realFS, err := fs.RealFS(fs.RealFSOptions{AbsWorkingDir: cwd}); err == nil {
				for i, file := range outputFiles {
					path, ok := realFS.Rel(realFS.Cwd(), file.Path)
					if !ok {
						path = file.Path
					}
					base := realFS.Base(path)
					n := len(file.Contents)
					table[i] = logger.SummaryTableEntry{
						Dir:         path[:len(path)-len(base)],
						Base:        base,
						Size:        prettyPrintByteCount(n),
						Bytes:       n,
						IsSourceMap: strings.HasSuffix(base, ".map"),
					}
				}
			}
		}
	}

	// Don't print the time taken by the build if we're running under Yarn 1
	// since Yarn 1 always prints its own copy of the time taken by each command
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "npm_config_user_agent=") && strings.Contains(env, "yarn/1.") {
			logger.PrintSummary(logOptions.Color, table, nil)
			return
		}
	}

	logger.PrintSummary(logOptions.Color, table, &start)
}

func rebuildImpl(
	buildOpts BuildOptions,
	caches *cache.CacheSet,
	plugins []config.Plugin,
	finalizeBuildOptions func(*config.Options),
	onEndCallbacks []func(*BuildResult),
	logOptions logger.OutputOptions,
	log logger.Log,
	isRebuild bool,
) internalBuildResult {
	// Convert and validate the buildOpts
	realFS, err := fs.RealFS(fs.RealFSOptions{
		AbsWorkingDir: buildOpts.AbsWorkingDir,
		WantWatchData: buildOpts.Watch != nil,
	})
	if err != nil {
		// This should already have been checked above
		panic(err.Error())
	}
	targetFromAPI, jsFeatures, cssFeatures, targetEnv := validateFeatures(log, buildOpts.Target, buildOpts.Engines)
	outJS, outCSS := validateOutputExtensions(log, buildOpts.OutExtensions)
	bannerJS, bannerCSS := validateBannerOrFooter(log, "banner", buildOpts.Banner)
	footerJS, footerCSS := validateBannerOrFooter(log, "footer", buildOpts.Footer)
	minify := buildOpts.MinifyWhitespace && buildOpts.MinifyIdentifiers && buildOpts.MinifySyntax
	defines, injectedDefines := validateDefines(log, buildOpts.Define, buildOpts.Pure, buildOpts.Platform, minify, buildOpts.Drop)
	mangleCache := cloneMangleCache(log, buildOpts.MangleCache)
	options := config.Options{
		TargetFromAPI:          targetFromAPI,
		UnsupportedJSFeatures:  jsFeatures,
		UnsupportedCSSFeatures: cssFeatures,
		OriginalTargetEnv:      targetEnv,
		JSX: config.JSXOptions{
			Preserve: buildOpts.JSXMode == JSXModePreserve,
			Factory:  validateJSXExpr(log, buildOpts.JSXFactory, "factory", js_parser.JSXFactory),
			Fragment: validateJSXExpr(log, buildOpts.JSXFragment, "fragment", js_parser.JSXFragment),
		},
		Defines:               defines,
		InjectedDefines:       injectedDefines,
		Platform:              validatePlatform(buildOpts.Platform),
		SourceMap:             validateSourceMap(buildOpts.Sourcemap),
		LegalComments:         validateLegalComments(buildOpts.LegalComments, buildOpts.Bundle),
		SourceRoot:            buildOpts.SourceRoot,
		ExcludeSourcesContent: buildOpts.SourcesContent == SourcesContentExclude,
		MinifySyntax:          buildOpts.MinifySyntax,
		MinifyWhitespace:      buildOpts.MinifyWhitespace,
		MinifyIdentifiers:     buildOpts.MinifyIdentifiers,
		MangleProps:           validateRegex(log, "mangle props", buildOpts.MangleProps),
		ReserveProps:          validateRegex(log, "reserve props", buildOpts.ReserveProps),
		MangleQuoted:          buildOpts.MangleQuoted == MangleQuotedTrue,
		DropDebugger:          (buildOpts.Drop & DropDebugger) != 0,
		AllowOverwrite:        buildOpts.AllowOverwrite,
		ASCIIOnly:             validateASCIIOnly(buildOpts.Charset),
		IgnoreDCEAnnotations:  buildOpts.IgnoreAnnotations,
		TreeShaking:           validateTreeShaking(buildOpts.TreeShaking, buildOpts.Bundle, buildOpts.Format),
		GlobalName:            validateGlobalName(log, buildOpts.GlobalName),
		CodeSplitting:         buildOpts.Splitting,
		OutputFormat:          validateFormat(buildOpts.Format),
		AbsOutputFile:         validatePath(log, realFS, buildOpts.Outfile, "outfile path"),
		AbsOutputDir:          validatePath(log, realFS, buildOpts.Outdir, "outdir path"),
		AbsOutputBase:         validatePath(log, realFS, buildOpts.Outbase, "outbase path"),
		NeedsMetafile:         buildOpts.Metafile,
		EntryPathTemplate:     validatePathTemplate(buildOpts.EntryNames),
		ChunkPathTemplate:     validatePathTemplate(buildOpts.ChunkNames),
		AssetPathTemplate:     validatePathTemplate(buildOpts.AssetNames),
		OutputExtensionJS:     outJS,
		OutputExtensionCSS:    outCSS,
		ExtensionToLoader:     validateLoaders(log, buildOpts.Loader),
		ExtensionOrder:        validateResolveExtensions(log, buildOpts.ResolveExtensions),
		ExternalSettings:      validateExternals(log, realFS, buildOpts.External),
		TsConfigOverride:      validatePath(log, realFS, buildOpts.Tsconfig, "tsconfig path"),
		MainFields:            buildOpts.MainFields,
		Conditions:            append([]string{}, buildOpts.Conditions...),
		PublicPath:            buildOpts.PublicPath,
		KeepNames:             buildOpts.KeepNames,
		InjectAbsPaths:        make([]string, len(buildOpts.Inject)),
		AbsNodePaths:          make([]string, len(buildOpts.NodePaths)),
		JSBanner:              bannerJS,
		JSFooter:              footerJS,
		CSSBanner:             bannerCSS,
		CSSFooter:             footerCSS,
		PreserveSymlinks:      buildOpts.PreserveSymlinks,
		WatchMode:             buildOpts.Watch != nil,
		Plugins:               plugins,
	}
	if options.MainFields != nil {
		options.MainFields = append([]string{}, options.MainFields...)
	}
	for i, path := range buildOpts.Inject {
		options.InjectAbsPaths[i] = validatePath(log, realFS, path, "inject path")
	}
	for i, path := range buildOpts.NodePaths {
		options.AbsNodePaths[i] = validatePath(log, realFS, path, "node path")
	}
	entryPoints := make([]bundler.EntryPoint, 0, len(buildOpts.EntryPoints)+len(buildOpts.EntryPointsAdvanced))
	for _, ep := range buildOpts.EntryPoints {
		entryPoints = append(entryPoints, bundler.EntryPoint{InputPath: ep})
	}
	for _, ep := range buildOpts.EntryPointsAdvanced {
		entryPoints = append(entryPoints, bundler.EntryPoint{InputPath: ep.InputPath, OutputPath: ep.OutputPath})
	}
	entryPointCount := len(entryPoints)
	if buildOpts.Stdin != nil {
		entryPointCount++
		options.Stdin = &config.StdinInfo{
			Loader:        validateLoader(buildOpts.Stdin.Loader),
			Contents:      buildOpts.Stdin.Contents,
			SourceFile:    buildOpts.Stdin.Sourcefile,
			AbsResolveDir: validatePath(log, realFS, buildOpts.Stdin.ResolveDir, "resolve directory path"),
		}
	}

	if options.AbsOutputDir == "" && entryPointCount > 1 {
		log.Add(logger.Error, nil, logger.Range{},
			"Must use \"outdir\" when there are multiple input files")
	} else if options.AbsOutputDir == "" && options.CodeSplitting {
		log.Add(logger.Error, nil, logger.Range{},
			"Must use \"outdir\" when code splitting is enabled")
	} else if options.AbsOutputFile != "" && options.AbsOutputDir != "" {
		log.Add(logger.Error, nil, logger.Range{}, "Cannot use both \"outfile\" and \"outdir\"")
	} else if options.AbsOutputFile != "" {
		// If the output file is specified, use it to derive the output directory
		options.AbsOutputDir = realFS.Dir(options.AbsOutputFile)
	} else if options.AbsOutputDir == "" {
		options.WriteToStdout = true

		// Forbid certain features when writing to stdout
		if options.SourceMap != config.SourceMapNone && options.SourceMap != config.SourceMapInline {
			log.Add(logger.Error, nil, logger.Range{}, "Cannot use an external source map without an output path")
		}
		if options.LegalComments.HasExternalFile() {
			log.Add(logger.Error, nil, logger.Range{}, "Cannot use linked or external legal comments without an output path")
		}
		for _, loader := range options.ExtensionToLoader {
			if loader == config.LoaderFile {
				log.Add(logger.Error, nil, logger.Range{}, "Cannot use the \"file\" loader without an output path")
				break
			}
		}

		// Use the current directory as the output directory instead of an empty
		// string because external modules with relative paths need a base directory.
		options.AbsOutputDir = realFS.Cwd()
	}

	if !buildOpts.Bundle {
		// Disallow bundle-only options when not bundling
		if options.ExternalSettings.PreResolve.HasMatchers() || options.ExternalSettings.PostResolve.HasMatchers() {
			log.Add(logger.Error, nil, logger.Range{}, "Cannot use \"external\" without \"bundle\"")
		}
	} else if options.OutputFormat == config.FormatPreserve {
		// If the format isn't specified, set the default format using the platform
		switch options.Platform {
		case config.PlatformBrowser:
			options.OutputFormat = config.FormatIIFE
		case config.PlatformNode:
			options.OutputFormat = config.FormatCommonJS
		case config.PlatformNeutral:
			options.OutputFormat = config.FormatESModule
		}
	}

	// Set the output mode using other settings
	if buildOpts.Bundle {
		options.Mode = config.ModeBundle
	} else if options.OutputFormat != config.FormatPreserve {
		options.Mode = config.ModeConvertFormat
	}

	// Code splitting is experimental and currently only enabled for ES6 modules
	if options.CodeSplitting && options.OutputFormat != config.FormatESModule {
		log.Add(logger.Error, nil, logger.Range{}, "Splitting currently only works with the \"esm\" format")
	}

	var outputFiles []OutputFile
	var metafileJSON string
	var watchData fs.WatchData

	// Stop now if there were errors
	resolver := resolver.NewResolver(realFS, log, caches, options)
	if !log.HasErrors() {
		var timer *helpers.Timer
		if api_helpers.UseTimer {
			timer = &helpers.Timer{}
		}

		// Finalize the build options, which will enable API methods that need them such as the "resolve" API
		if finalizeBuildOptions != nil {
			finalizeBuildOptions(&options)
		}

		// Scan over the bundle
		bundle := bundler.ScanBundle(log, realFS, resolver, caches, entryPoints, options, timer)
		watchData = realFS.WatchData()

		// Stop now if there were errors
		if !log.HasErrors() {
			// Compile the bundle
			results, metafile := bundle.Compile(log, options, timer, mangleCache)

			// Stop now if there were errors
			if !log.HasErrors() {
				metafileJSON = metafile

				// Flush any deferred warnings now
				log.AlmostDone()

				if buildOpts.Write {
					timer.Begin("Write output files")
					if options.WriteToStdout {
						// Special-case writing to stdout
						if len(results) != 1 {
							log.Add(logger.Error, nil, logger.Range{}, fmt.Sprintf(
								"Internal error: did not expect to generate %d files when writing to stdout", len(results)))
						} else if _, err := os.Stdout.Write(results[0].Contents); err != nil {
							log.Add(logger.Error, nil, logger.Range{}, fmt.Sprintf(
								"Failed to write to stdout: %s", err.Error()))
						}
					} else {
						// Write out files in parallel
						waitGroup := sync.WaitGroup{}
						waitGroup.Add(len(results))
						for _, result := range results {
							go func(result graph.OutputFile) {
								fs.BeforeFileOpen()
								defer fs.AfterFileClose()
								if err := fs.MkdirAll(realFS, realFS.Dir(result.AbsPath), 0755); err != nil {
									log.Add(logger.Error, nil, logger.Range{}, fmt.Sprintf(
										"Failed to create output directory: %s", err.Error()))
								} else {
									var mode os.FileMode = 0644
									if result.IsExecutable {
										mode = 0755
									}
									if err := ioutil.WriteFile(result.AbsPath, result.Contents, mode); err != nil {
										log.Add(logger.Error, nil, logger.Range{}, fmt.Sprintf(
											"Failed to write to output file: %s", err.Error()))
									}
								}
								waitGroup.Done()
							}(result)
						}
						waitGroup.Wait()
					}
					timer.End("Write output files")
				}

				// Return the results
				outputFiles = make([]OutputFile, len(results))
				for i, result := range results {
					if options.WriteToStdout {
						result.AbsPath = "<stdout>"
					}
					outputFiles[i] = OutputFile{
						Path:     result.AbsPath,
						Contents: result.Contents,
					}
				}
			}
		}

		timer.Log(log)
	}

	// End the log now, which may print a message
	msgs := log.Done()

	// Start watching, but only for the top-level build
	var watch *watcher
	var stop func()
	if buildOpts.Watch != nil && !isRebuild {
		onRebuild := buildOpts.Watch.OnRebuild
		watch = &watcher{
			data:     watchData,
			resolver: resolver,
			rebuild: func() fs.WatchData {
				value := rebuildImpl(buildOpts, caches, plugins, nil, onEndCallbacks, logOptions, logger.NewStderrLog(logOptions), true /* isRebuild */)
				if onRebuild != nil {
					go onRebuild(value.result)
				}
				return value.watchData
			},
		}
		mode := *buildOpts.Watch
		watch.start(buildOpts.LogLevel, buildOpts.Color, mode)
		stop = func() {
			watch.stop()
		}
	}

	var rebuild func() BuildResult
	if buildOpts.Incremental {
		rebuild = func() BuildResult {
			value := rebuildImpl(buildOpts, caches, plugins, nil, onEndCallbacks, logOptions, logger.NewStderrLog(logOptions), true /* isRebuild */)
			if watch != nil {
				watch.setWatchData(value.watchData)
			}
			return value.result
		}
	}

	// Only return the mangle cache for a successful build
	if log.HasErrors() {
		mangleCache = nil
	}

	result := BuildResult{
		Errors:      convertMessagesToPublic(logger.Error, msgs),
		Warnings:    convertMessagesToPublic(logger.Warning, msgs),
		OutputFiles: outputFiles,
		Metafile:    metafileJSON,
		Rebuild:     rebuild,
		Stop:        stop,
		MangleCache: mangleCache,
	}

	for _, onEnd := range onEndCallbacks {
		onEnd(&result)
	}

	return internalBuildResult{
		result:    result,
		options:   options,
		watchData: watchData,
	}
}

type watcher struct {
	data              fs.WatchData
	resolver          resolver.Resolver
	rebuild           func() fs.WatchData
	recentItems       []string
	itemsToScan       []string
	mutex             sync.Mutex
	itemsPerIteration int
	shouldStop        int32
}

func (w *watcher) setWatchData(data fs.WatchData) {
	defer w.mutex.Unlock()
	w.mutex.Lock()
	w.data = data
	w.itemsToScan = w.itemsToScan[:0] // Reuse memory

	// Remove any recent items that weren't a part of the latest build
	end := 0
	for _, path := range w.recentItems {
		if data.Paths[path] != nil {
			w.recentItems[end] = path
			end++
		}
	}
	w.recentItems = w.recentItems[:end]
}

// The time to wait between watch intervals
const watchIntervalSleep = 100 * time.Millisecond

// The maximum number of recently-edited items to check every interval
const maxRecentItemCount = 16

// The minimum number of non-recent items to check every interval
const minItemCountPerIter = 64

// The maximum number of intervals before a change is detected
const maxIntervalsBeforeUpdate = 20

func (w *watcher) start(logLevel LogLevel, color StderrColor, mode WatchMode) {
	useColor := validateColor(color)

	go func() {
		shouldLog := logLevel == LogLevelInfo || logLevel == LogLevelDebug

		// Note: Do not change these log messages without a breaking version change.
		// People want to run regexes over esbuild's stderr stream to look for these
		// messages instead of using esbuild's API.

		if shouldLog {
			logger.PrintTextWithColor(os.Stderr, useColor, func(colors logger.Colors) string {
				return fmt.Sprintf("%s[watch] build finished, watching for changes...%s\n", colors.Dim, colors.Reset)
			})
		}

		for atomic.LoadInt32(&w.shouldStop) == 0 {
			// Sleep for the watch interval
			time.Sleep(watchIntervalSleep)

			// Rebuild if we're dirty
			if absPath := w.tryToFindDirtyPath(); absPath != "" {
				if shouldLog {
					logger.PrintTextWithColor(os.Stderr, useColor, func(colors logger.Colors) string {
						prettyPath := w.resolver.PrettyPath(logger.Path{Text: absPath, Namespace: "file"})
						return fmt.Sprintf("%s[watch] build started (change: %q)%s\n", colors.Dim, prettyPath, colors.Reset)
					})
				}

				// Run the build
				w.setWatchData(w.rebuild())

				if shouldLog {
					logger.PrintTextWithColor(os.Stderr, useColor, func(colors logger.Colors) string {
						return fmt.Sprintf("%s[watch] build finished%s\n", colors.Dim, colors.Reset)
					})
				}
			}
		}
	}()
}

func (w *watcher) stop() {
	atomic.StoreInt32(&w.shouldStop, 1)
}

func (w *watcher) tryToFindDirtyPath() string {
	defer w.mutex.Unlock()
	w.mutex.Lock()

	// If we ran out of items to scan, fill the items back up in a random order
	if len(w.itemsToScan) == 0 {
		items := w.itemsToScan[:0] // Reuse memory
		for path := range w.data.Paths {
			items = append(items, path)
		}
		rand.Seed(time.Now().UnixNano())
		for i := int32(len(items) - 1); i > 0; i-- { // Fisher-Yates shuffle
			j := rand.Int31n(i + 1)
			items[i], items[j] = items[j], items[i]
		}
		w.itemsToScan = items

		// Determine how many items to check every iteration, rounded up
		perIter := (len(items) + maxIntervalsBeforeUpdate - 1) / maxIntervalsBeforeUpdate
		if perIter < minItemCountPerIter {
			perIter = minItemCountPerIter
		}
		w.itemsPerIteration = perIter
	}

	// Always check all recent items every iteration
	for i, path := range w.recentItems {
		if dirtyPath := w.data.Paths[path](); dirtyPath != "" {
			// Move this path to the back of the list (i.e. the "most recent" position)
			copy(w.recentItems[i:], w.recentItems[i+1:])
			w.recentItems[len(w.recentItems)-1] = path
			return dirtyPath
		}
	}

	// Check a constant number of items every iteration
	remainingCount := len(w.itemsToScan) - w.itemsPerIteration
	if remainingCount < 0 {
		remainingCount = 0
	}
	toCheck, remaining := w.itemsToScan[remainingCount:], w.itemsToScan[:remainingCount]
	w.itemsToScan = remaining

	// Check if any of the entries in this iteration have been modified
	for _, path := range toCheck {
		if dirtyPath := w.data.Paths[path](); dirtyPath != "" {
			// Mark this item as recent by adding it to the back of the list
			w.recentItems = append(w.recentItems, path)
			if len(w.recentItems) > maxRecentItemCount {
				// Remove items from the front of the list when we hit the limit
				copy(w.recentItems, w.recentItems[1:])
				w.recentItems = w.recentItems[:maxRecentItemCount]
			}
			return dirtyPath
		}
	}
	return ""
}

////////////////////////////////////////////////////////////////////////////////
// Transform API

func transformImpl(input string, transformOpts TransformOptions) TransformResult {
	log := logger.NewStderrLog(logger.OutputOptions{
		IncludeSource: true,
		MessageLimit:  transformOpts.LogLimit,
		Color:         validateColor(transformOpts.Color),
		LogLevel:      validateLogLevel(transformOpts.LogLevel),
	})

	// Settings from the user come first
	unusedImportsTS := config.UnusedImportsRemoveStmt
	useDefineForClassFieldsTS := config.Unspecified
	jsx := config.JSXOptions{
		Preserve: transformOpts.JSXMode == JSXModePreserve,
		Factory:  validateJSXExpr(log, transformOpts.JSXFactory, "factory", js_parser.JSXFactory),
		Fragment: validateJSXExpr(log, transformOpts.JSXFragment, "fragment", js_parser.JSXFragment),
	}

	// Settings from "tsconfig.json" override those
	var tsTarget *config.TSTarget
	caches := cache.MakeCacheSet()
	if transformOpts.TsconfigRaw != "" {
		source := logger.Source{
			KeyPath:    logger.Path{Text: "tsconfig.json"},
			PrettyPath: "tsconfig.json",
			Contents:   transformOpts.TsconfigRaw,
		}
		if result := resolver.ParseTSConfigJSON(log, source, &caches.JSONCache, nil); result != nil {
			if len(result.JSXFactory) > 0 {
				jsx.Factory = config.JSXExpr{Parts: result.JSXFactory}
			}
			if len(result.JSXFragmentFactory) > 0 {
				jsx.Fragment = config.JSXExpr{Parts: result.JSXFragmentFactory}
			}
			if result.UseDefineForClassFields != config.Unspecified {
				useDefineForClassFieldsTS = result.UseDefineForClassFields
			}
			unusedImportsTS = config.UnusedImportsFromTsconfigValues(
				result.PreserveImportsNotUsedAsValues,
				result.PreserveValueImports,
			)
			tsTarget = result.TSTarget
		}
	}

	// Apply default values
	if transformOpts.Sourcefile == "" {
		transformOpts.Sourcefile = "<stdin>"
	}
	if transformOpts.Loader == LoaderNone {
		transformOpts.Loader = LoaderJS
	}

	// Convert and validate the transformOpts
	targetFromAPI, jsFeatures, cssFeatures, targetEnv := validateFeatures(log, transformOpts.Target, transformOpts.Engines)
	defines, injectedDefines := validateDefines(log, transformOpts.Define, transformOpts.Pure, PlatformNeutral, false /* minify */, transformOpts.Drop)
	mangleCache := cloneMangleCache(log, transformOpts.MangleCache)
	options := config.Options{
		TargetFromAPI:           targetFromAPI,
		UnsupportedJSFeatures:   jsFeatures,
		UnsupportedCSSFeatures:  cssFeatures,
		OriginalTargetEnv:       targetEnv,
		TSTarget:                tsTarget,
		JSX:                     jsx,
		Defines:                 defines,
		InjectedDefines:         injectedDefines,
		SourceMap:               validateSourceMap(transformOpts.Sourcemap),
		LegalComments:           validateLegalComments(transformOpts.LegalComments, false /* bundle */),
		SourceRoot:              transformOpts.SourceRoot,
		ExcludeSourcesContent:   transformOpts.SourcesContent == SourcesContentExclude,
		OutputFormat:            validateFormat(transformOpts.Format),
		GlobalName:              validateGlobalName(log, transformOpts.GlobalName),
		MinifySyntax:            transformOpts.MinifySyntax,
		MinifyWhitespace:        transformOpts.MinifyWhitespace,
		MinifyIdentifiers:       transformOpts.MinifyIdentifiers,
		MangleProps:             validateRegex(log, "mangle props", transformOpts.MangleProps),
		ReserveProps:            validateRegex(log, "reserve props", transformOpts.ReserveProps),
		MangleQuoted:            transformOpts.MangleQuoted == MangleQuotedTrue,
		DropDebugger:            (transformOpts.Drop & DropDebugger) != 0,
		ASCIIOnly:               validateASCIIOnly(transformOpts.Charset),
		IgnoreDCEAnnotations:    transformOpts.IgnoreAnnotations,
		TreeShaking:             validateTreeShaking(transformOpts.TreeShaking, false /* bundle */, transformOpts.Format),
		AbsOutputFile:           transformOpts.Sourcefile + "-out",
		KeepNames:               transformOpts.KeepNames,
		UseDefineForClassFields: useDefineForClassFieldsTS,
		UnusedImportsTS:         unusedImportsTS,
		Stdin: &config.StdinInfo{
			Loader:     validateLoader(transformOpts.Loader),
			Contents:   input,
			SourceFile: transformOpts.Sourcefile,
		},
	}
	if options.Stdin.Loader == config.LoaderCSS {
		options.CSSBanner = transformOpts.Banner
		options.CSSFooter = transformOpts.Footer
	} else {
		options.JSBanner = transformOpts.Banner
		options.JSFooter = transformOpts.Footer
	}
	if options.SourceMap == config.SourceMapLinkedWithComment {
		// Linked source maps don't make sense because there's no output file name
		log.Add(logger.Error, nil, logger.Range{}, "Cannot transform with linked source maps")
	}
	if options.SourceMap != config.SourceMapNone && options.Stdin.SourceFile == "" {
		log.Add(logger.Error, nil, logger.Range{},
			"Must use \"sourcefile\" with \"sourcemap\" to set the original file name")
	}
	if options.LegalComments.HasExternalFile() {
		log.Add(logger.Error, nil, logger.Range{}, "Cannot transform with linked or external legal comments")
	}

	// Set the output mode using other settings
	if options.OutputFormat != config.FormatPreserve {
		options.Mode = config.ModeConvertFormat
	}

	var results []graph.OutputFile

	// Stop now if there were errors
	if !log.HasErrors() {
		var timer *helpers.Timer
		if api_helpers.UseTimer {
			timer = &helpers.Timer{}
		}

		// Scan over the bundle
		mockFS := fs.MockFS(make(map[string]string))
		resolver := resolver.NewResolver(mockFS, log, caches, options)
		bundle := bundler.ScanBundle(log, mockFS, resolver, caches, nil, options, timer)

		// Stop now if there were errors
		if !log.HasErrors() {
			// Compile the bundle
			results, _ = bundle.Compile(log, options, timer, mangleCache)
		}

		timer.Log(log)
	}

	// Return the results
	var code []byte
	var sourceMap []byte

	// Unpack the JavaScript file and the source map file
	if len(results) == 1 {
		code = results[0].Contents
	} else if len(results) == 2 {
		a, b := results[0], results[1]
		if a.AbsPath == b.AbsPath+".map" {
			sourceMap, code = a.Contents, b.Contents
		} else if a.AbsPath+".map" == b.AbsPath {
			code, sourceMap = a.Contents, b.Contents
		}
	}

	// Only return the mangle cache for a successful build
	if log.HasErrors() {
		mangleCache = nil
	}

	msgs := log.Done()
	return TransformResult{
		Errors:      convertMessagesToPublic(logger.Error, msgs),
		Warnings:    convertMessagesToPublic(logger.Warning, msgs),
		Code:        code,
		Map:         sourceMap,
		MangleCache: mangleCache,
	}
}

////////////////////////////////////////////////////////////////////////////////
// Plugin API

type pluginImpl struct {
	log    logger.Log
	fs     fs.FS
	plugin config.Plugin
}

func (impl *pluginImpl) onStart(callback func() (OnStartResult, error)) {
	impl.plugin.OnStart = append(impl.plugin.OnStart, config.OnStart{
		Name: impl.plugin.Name,
		Callback: func() (result config.OnStartResult) {
			response, err := callback()

			if err != nil {
				result.ThrownError = err
				return
			}

			// Convert log messages
			if len(response.Errors)+len(response.Warnings) > 0 {
				msgs := make(logger.SortableMsgs, 0, len(response.Errors)+len(response.Warnings))
				msgs = convertMessagesToInternal(msgs, logger.Error, response.Errors)
				msgs = convertMessagesToInternal(msgs, logger.Warning, response.Warnings)
				sort.Stable(msgs)
				result.Msgs = msgs
			}
			return
		},
	})
}

func importKindToResolveKind(kind ast.ImportKind) ResolveKind {
	switch kind {
	case ast.ImportEntryPoint:
		return ResolveEntryPoint
	case ast.ImportStmt:
		return ResolveJSImportStatement
	case ast.ImportRequire:
		return ResolveJSRequireCall
	case ast.ImportDynamic:
		return ResolveJSDynamicImport
	case ast.ImportRequireResolve:
		return ResolveJSRequireResolve
	case ast.ImportAt, ast.ImportAtConditional:
		return ResolveCSSImportRule
	case ast.ImportURL:
		return ResolveCSSURLToken
	default:
		panic("Internal error")
	}
}

func resolveKindToImportKind(kind ResolveKind) ast.ImportKind {
	switch kind {
	case ResolveEntryPoint:
		return ast.ImportEntryPoint
	case ResolveJSImportStatement:
		return ast.ImportStmt
	case ResolveJSRequireCall:
		return ast.ImportRequire
	case ResolveJSDynamicImport:
		return ast.ImportDynamic
	case ResolveJSRequireResolve:
		return ast.ImportRequireResolve
	case ResolveCSSImportRule:
		return ast.ImportAt
	case ResolveCSSURLToken:
		return ast.ImportURL
	default:
		panic("Internal error")
	}
}

func (impl *pluginImpl) onResolve(options OnResolveOptions, callback func(OnResolveArgs) (OnResolveResult, error)) {
	filter, err := config.CompileFilterForPlugin(impl.plugin.Name, "OnResolve", options.Filter)
	if filter == nil {
		impl.log.Add(logger.Error, nil, logger.Range{}, err.Error())
		return
	}

	impl.plugin.OnResolve = append(impl.plugin.OnResolve, config.OnResolve{
		Name:      impl.plugin.Name,
		Filter:    filter,
		Namespace: options.Namespace,
		Callback: func(args config.OnResolveArgs) (result config.OnResolveResult) {
			response, err := callback(OnResolveArgs{
				Path:       args.Path,
				Importer:   args.Importer.Text,
				Namespace:  args.Importer.Namespace,
				ResolveDir: args.ResolveDir,
				Kind:       importKindToResolveKind(args.Kind),
				PluginData: args.PluginData,
			})
			result.PluginName = response.PluginName
			result.AbsWatchFiles = impl.validatePathsArray(response.WatchFiles, "watch file")
			result.AbsWatchDirs = impl.validatePathsArray(response.WatchDirs, "watch directory")

			// Restrict the suffix to start with "?" or "#" for now to match esbuild's behavior
			if err == nil && response.Suffix != "" && response.Suffix[0] != '?' && response.Suffix[0] != '#' {
				err = fmt.Errorf("Invalid path suffix %q returned from plugin (must start with \"?\" or \"#\")", response.Suffix)
			}

			if err != nil {
				result.ThrownError = err
				return
			}

			result.Path = logger.Path{
				Text:          response.Path,
				Namespace:     response.Namespace,
				IgnoredSuffix: response.Suffix,
			}
			result.External = response.External
			result.IsSideEffectFree = response.SideEffects == SideEffectsFalse
			result.PluginData = response.PluginData

			// Convert log messages
			if len(response.Errors)+len(response.Warnings) > 0 {
				msgs := make(logger.SortableMsgs, 0, len(response.Errors)+len(response.Warnings))
				msgs = convertMessagesToInternal(msgs, logger.Error, response.Errors)
				msgs = convertMessagesToInternal(msgs, logger.Warning, response.Warnings)
				sort.Stable(msgs)
				result.Msgs = msgs
			}
			return
		},
	})
}

func (impl *pluginImpl) onLoad(options OnLoadOptions, callback func(OnLoadArgs) (OnLoadResult, error)) {
	filter, err := config.CompileFilterForPlugin(impl.plugin.Name, "OnLoad", options.Filter)
	if filter == nil {
		impl.log.Add(logger.Error, nil, logger.Range{}, err.Error())
		return
	}

	impl.plugin.OnLoad = append(impl.plugin.OnLoad, config.OnLoad{
		Filter:    filter,
		Namespace: options.Namespace,
		Callback: func(args config.OnLoadArgs) (result config.OnLoadResult) {
			response, err := callback(OnLoadArgs{
				Path:       args.Path.Text,
				Namespace:  args.Path.Namespace,
				PluginData: args.PluginData,
				Suffix:     args.Path.IgnoredSuffix,
			})
			result.PluginName = response.PluginName
			result.AbsWatchFiles = impl.validatePathsArray(response.WatchFiles, "watch file")
			result.AbsWatchDirs = impl.validatePathsArray(response.WatchDirs, "watch directory")

			if err != nil {
				result.ThrownError = err
				return
			}

			result.Contents = response.Contents
			result.Loader = validateLoader(response.Loader)
			result.PluginData = response.PluginData
			pathKind := fmt.Sprintf("resolve directory path for plugin %q", impl.plugin.Name)
			if absPath := validatePath(impl.log, impl.fs, response.ResolveDir, pathKind); absPath != "" {
				result.AbsResolveDir = absPath
			}

			// Convert log messages
			if len(response.Errors)+len(response.Warnings) > 0 {
				msgs := make(logger.SortableMsgs, 0, len(response.Errors)+len(response.Warnings))
				msgs = convertMessagesToInternal(msgs, logger.Error, response.Errors)
				msgs = convertMessagesToInternal(msgs, logger.Warning, response.Warnings)
				sort.Stable(msgs)
				result.Msgs = msgs
			}
			return
		},
	})
}

func (impl *pluginImpl) validatePathsArray(pathsIn []string, name string) (pathsOut []string) {
	if len(pathsIn) > 0 {
		pathKind := fmt.Sprintf("%s path for plugin %q", name, impl.plugin.Name)
		for _, relPath := range pathsIn {
			if absPath := validatePath(impl.log, impl.fs, relPath, pathKind); absPath != "" {
				pathsOut = append(pathsOut, absPath)
			}
		}
	}
	return
}

func loadPlugins(initialOptions *BuildOptions, fs fs.FS, log logger.Log, caches *cache.CacheSet) (
	plugins []config.Plugin,
	onEndCallbacks []func(*BuildResult),
	finalizeBuildOptions func(*config.Options),
) {
	onEnd := func(callback func(*BuildResult)) {
		onEndCallbacks = append(onEndCallbacks, callback)
	}

	// Clone the plugin array to guard against mutation during iteration
	clone := append(make([]Plugin, 0, len(initialOptions.Plugins)), initialOptions.Plugins...)

	var resolveMutex sync.Mutex
	var optionsForResolve *config.Options

	// This is called when the build options are finalized
	finalizeBuildOptions = func(options *config.Options) {
		resolveMutex.Lock()
		if optionsForResolve == nil {
			optionsForResolve = options
		}
		resolveMutex.Unlock()
	}

	for i, item := range clone {
		if item.Name == "" {
			log.Add(logger.Error, nil, logger.Range{}, fmt.Sprintf("Plugin at index %d is missing a name", i))
			continue
		}

		impl := &pluginImpl{
			fs:     fs,
			log:    log,
			plugin: config.Plugin{Name: item.Name},
		}

		resolve := func(path string, options ResolveOptions) (result ResolveResult) {
			// Try to grab the resolver options
			resolveMutex.Lock()
			buildOptions := optionsForResolve
			resolveMutex.Unlock()

			// If we couldn't grab them, then this is being called before plugin setup
			// has finished. That isn't allowed because plugin setup is allowed to
			// change the initial options object, which can affect path resolution.
			if buildOptions == nil {
				return ResolveResult{Errors: []Message{{Text: "Cannot call \"resolve\" before plugin setup has completed"}}}
			}

			// Make a new resolver so it has its own log
			log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug)
			resolver := resolver.NewResolver(fs, log, caches, *buildOptions)

			// Make sure the resolve directory is an absolute path, which can fail
			absResolveDir := validatePath(log, fs, options.ResolveDir, "resolve directory")
			if log.HasErrors() {
				msgs := log.Done()
				result.Errors = convertMessagesToPublic(logger.Error, msgs)
				result.Warnings = convertMessagesToPublic(logger.Warning, msgs)
				return
			}

			// Run path resolution
			kind := resolveKindToImportKind(options.Kind)
			resolveResult, _, _ := bundler.RunOnResolvePlugins(
				plugins,
				resolver,
				log,
				fs,
				&caches.FSCache,
				nil,            // importSource
				logger.Range{}, // importPathRange
				logger.Path{Text: options.Importer, Namespace: options.Namespace},
				path,
				kind,
				absResolveDir,
				options.PluginData,
			)
			msgs := log.Done()

			// Populate the result
			result.Errors = convertMessagesToPublic(logger.Error, msgs)
			result.Warnings = convertMessagesToPublic(logger.Warning, msgs)
			if resolveResult != nil {
				result.Path = resolveResult.PathPair.Primary.Text
				result.External = resolveResult.IsExternal
				result.SideEffects = resolveResult.PrimarySideEffectsData == nil
				result.Namespace = resolveResult.PathPair.Primary.Namespace
				result.Suffix = resolveResult.PathPair.Primary.IgnoredSuffix
				result.PluginData = resolveResult.PluginData
			} else if len(result.Errors) == 0 {
				// Always fail with at least one error
				pluginName := item.Name
				if options.PluginName != "" {
					pluginName = options.PluginName
				}
				text, notes := bundler.ResolveFailureErrorTextAndNotes(resolver, path, kind, pluginName, fs, absResolveDir, buildOptions.Platform, "")
				result.Errors = append(result.Errors, convertMessagesToPublic(logger.Error, []logger.Msg{{
					Data:  logger.MsgData{Text: text},
					Notes: notes,
				}})...)
			}
			return
		}

		item.Setup(PluginBuild{
			InitialOptions: initialOptions,
			Resolve:        resolve,
			OnStart:        impl.onStart,
			OnEnd:          onEnd,
			OnResolve:      impl.onResolve,
			OnLoad:         impl.onLoad,
		})

		plugins = append(plugins, impl.plugin)
	}

	return
}

////////////////////////////////////////////////////////////////////////////////
// FormatMessages API

func formatMsgsImpl(msgs []Message, opts FormatMessagesOptions) []string {
	kind := logger.Error
	if opts.Kind == WarningMessage {
		kind = logger.Warning
	}
	logMsgs := convertMessagesToInternal(nil, kind, msgs)
	strings := make([]string, len(logMsgs))
	for i, msg := range logMsgs {
		strings[i] = msg.String(
			logger.OutputOptions{
				IncludeSource: true,
			},
			logger.TerminalInfo{
				UseColorEscapes: opts.Color,
				Width:           opts.TerminalWidth,
			},
		)
	}
	return strings
}

////////////////////////////////////////////////////////////////////////////////
// AnalyzeMetafile API

type metafileEntry struct {
	name       string
	entryPoint string
	entries    []metafileEntry
	size       int
}

type metafileArray []metafileEntry

func (a metafileArray) Len() int          { return len(a) }
func (a metafileArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a metafileArray) Less(i int, j int) bool {
	ai := a[i]
	aj := a[j]
	return ai.size > aj.size || (ai.size == aj.size && ai.name < aj.name)
}

func getObjectProperty(expr js_ast.Expr, key string) js_ast.Expr {
	if obj, ok := expr.Data.(*js_ast.EObject); ok {
		for _, prop := range obj.Properties {
			if helpers.UTF16EqualsString(prop.Key.Data.(*js_ast.EString).Value, key) {
				return prop.ValueOrNil
			}
		}
	}
	return js_ast.Expr{}
}

func getObjectPropertyNumber(expr js_ast.Expr, key string) *js_ast.ENumber {
	value, _ := getObjectProperty(expr, key).Data.(*js_ast.ENumber)
	return value
}

func getObjectPropertyString(expr js_ast.Expr, key string) *js_ast.EString {
	value, _ := getObjectProperty(expr, key).Data.(*js_ast.EString)
	return value
}

func getObjectPropertyObject(expr js_ast.Expr, key string) *js_ast.EObject {
	value, _ := getObjectProperty(expr, key).Data.(*js_ast.EObject)
	return value
}

func getObjectPropertyArray(expr js_ast.Expr, key string) *js_ast.EArray {
	value, _ := getObjectProperty(expr, key).Data.(*js_ast.EArray)
	return value
}

func analyzeMetafileImpl(metafile string, opts AnalyzeMetafileOptions) string {
	log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug)
	source := logger.Source{Contents: metafile}

	if result, ok := js_parser.ParseJSON(log, source, js_parser.JSONOptions{}); ok {
		if outputs := getObjectPropertyObject(result, "outputs"); outputs != nil {
			var entries metafileArray
			var entryPoints []string

			// Scan over the "outputs" object
			for _, output := range outputs.Properties {
				if key := helpers.UTF16ToString(output.Key.Data.(*js_ast.EString).Value); !strings.HasSuffix(key, ".map") {
					entryPointPath := ""
					if entryPoint := getObjectPropertyString(output.ValueOrNil, "entryPoint"); entryPoint != nil {
						entryPointPath = helpers.UTF16ToString(entryPoint.Value)
						entryPoints = append(entryPoints, entryPointPath)
					}

					if bytes := getObjectPropertyNumber(output.ValueOrNil, "bytes"); bytes != nil {
						if inputs := getObjectPropertyObject(output.ValueOrNil, "inputs"); inputs != nil {
							var children metafileArray

							for _, input := range inputs.Properties {
								if bytesInOutput := getObjectPropertyNumber(input.ValueOrNil, "bytesInOutput"); bytesInOutput != nil && bytesInOutput.Value > 0 {
									children = append(children, metafileEntry{
										name: helpers.UTF16ToString(input.Key.Data.(*js_ast.EString).Value),
										size: int(bytesInOutput.Value),
									})
								}
							}

							sort.Sort(children)

							entries = append(entries, metafileEntry{
								name:       key,
								size:       int(bytes.Value),
								entries:    children,
								entryPoint: entryPointPath,
							})
						}
					}
				}
			}

			sort.Sort(entries)

			type importData struct {
				imports []string
			}

			type graphData struct {
				parent string
				depth  uint32
			}

			importsForPath := make(map[string]importData)

			// Scan over the "inputs" object
			if inputs := getObjectPropertyObject(result, "inputs"); inputs != nil {
				for _, prop := range inputs.Properties {
					if imports := getObjectPropertyArray(prop.ValueOrNil, "imports"); imports != nil {
						var data importData

						for _, item := range imports.Items {
							if path := getObjectPropertyString(item, "path"); path != nil {
								data.imports = append(data.imports, helpers.UTF16ToString(path.Value))
							}
						}

						importsForPath[helpers.UTF16ToString(prop.Key.Data.(*js_ast.EString).Value)] = data
					}
				}
			}

			// Returns a graph with links pointing from imports to importers
			graphForEntryPoints := func(worklist []string) map[string]graphData {
				if !opts.Verbose {
					return nil
				}

				graph := make(map[string]graphData)

				for _, entryPoint := range worklist {
					graph[entryPoint] = graphData{}
				}

				for len(worklist) > 0 {
					top := worklist[len(worklist)-1]
					worklist = worklist[:len(worklist)-1]
					childDepth := graph[top].depth + 1

					for _, importPath := range importsForPath[top].imports {
						imported, ok := graph[importPath]
						if !ok {
							imported.depth = math.MaxUint32
						}

						if imported.depth > childDepth {
							imported.depth = childDepth
							imported.parent = top
							graph[importPath] = imported
							worklist = append(worklist, importPath)
						}
					}
				}

				return graph
			}

			graphForAllEntryPoints := graphForEntryPoints(entryPoints)

			type tableEntry struct {
				first      string
				second     string
				third      string
				firstLen   int
				secondLen  int
				thirdLen   int
				isTopLevel bool
			}

			var table []tableEntry
			var colors logger.Colors

			if opts.Color {
				colors = logger.TerminalColors
			}

			// Build up the table with an entry for each output file (other than ".map" files)
			for _, entry := range entries {
				second := prettyPrintByteCount(entry.size)
				third := "100.0%"

				table = append(table, tableEntry{
					first:      fmt.Sprintf("%s%s%s", colors.Bold, entry.name, colors.Reset),
					firstLen:   utf8.RuneCountInString(entry.name),
					second:     fmt.Sprintf("%s%s%s", colors.Bold, second, colors.Reset),
					secondLen:  len(second),
					third:      fmt.Sprintf("%s%s%s", colors.Bold, third, colors.Reset),
					thirdLen:   len(third),
					isTopLevel: true,
				})

				graph := graphForAllEntryPoints
				if entry.entryPoint != "" {
					// If there are multiple entry points and this output file is from an
					// entry point, prefer import paths for this entry point. This is less
					// confusing than showing import paths for another entry point.
					graph = graphForEntryPoints([]string{entry.entryPoint})
				}

				// Add a sub-entry for each input file in this output file
				for j, child := range entry.entries {
					indent := " ├ "
					if j+1 == len(entry.entries) {
						indent = " └ "
					}
					percent := 100.0 * float64(child.size) / float64(entry.size)

					first := indent + child.name
					second := prettyPrintByteCount(child.size)
					third := fmt.Sprintf("%.1f%%", percent)

					table = append(table, tableEntry{
						first:     first,
						firstLen:  utf8.RuneCountInString(first),
						second:    second,
						secondLen: len(second),
						third:     third,
						thirdLen:  len(third),
					})

					// If we're in verbose mode, also print the import chain from this file
					// up toward an entry point to show why this file is in the bundle
					if opts.Verbose {
						indent = " │ "
						if j+1 == len(entry.entries) {
							indent = "   "
						}
						data := graph[child.name]
						depth := 0

						for data.depth != 0 {
							table = append(table, tableEntry{
								first: fmt.Sprintf("%s%s%s └ %s%s", indent, colors.Dim, strings.Repeat(" ", depth), data.parent, colors.Reset),
							})
							data = graph[data.parent]
							depth += 3
						}
					}
				}
			}

			maxFirstLen := 0
			maxSecondLen := 0
			maxThirdLen := 0

			// Calculate column widths
			for _, entry := range table {
				if maxFirstLen < entry.firstLen {
					maxFirstLen = entry.firstLen
				}
				if maxSecondLen < entry.secondLen {
					maxSecondLen = entry.secondLen
				}
				if maxThirdLen < entry.thirdLen {
					maxThirdLen = entry.thirdLen
				}
			}

			sb := strings.Builder{}

			// Render the columns now that we know the widths
			for _, entry := range table {
				prefix := "\n"
				if !entry.isTopLevel {
					prefix = ""
				}

				// Import paths don't have second and third columns
				if entry.second == "" && entry.third == "" {
					sb.WriteString(fmt.Sprintf("%s  %s\n",
						prefix,
						entry.first,
					))
					continue
				}

				second := entry.second
				secondTrimmed := strings.TrimRight(second, " ")
				lineChar := " "
				extraSpace := 0

				if opts.Verbose {
					lineChar = "─"
					extraSpace = 1
				}

				sb.WriteString(fmt.Sprintf("%s  %s %s%s%s %s %s%s%s %s\n",
					prefix,
					entry.first,
					colors.Dim,
					strings.Repeat(lineChar, extraSpace+maxFirstLen-entry.firstLen+maxSecondLen-entry.secondLen),
					colors.Reset,
					secondTrimmed,
					colors.Dim,
					strings.Repeat(lineChar, extraSpace+maxThirdLen-entry.thirdLen+len(second)-len(secondTrimmed)),
					colors.Reset,
					entry.third,
				))
			}

			return sb.String()
		}
	}

	return ""
}
