package api

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/evanw/esbuild/internal/bundler"
	"github.com/evanw/esbuild/internal/cache"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/js_parser"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/resolver"
)

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

func validateIgnoreDCEAnnotations(value TreeShaking) bool {
	switch value {
	case TreeShakingDefault:
		return false
	case TreeShakingIgnoreAnnotations:
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

func validateFeatures(log logger.Log, target Target, engines []Engine) (compat.JSFeature, compat.CSSFeature) {
	constraints := make(map[compat.Engine][]int)

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
	case ESNext:
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
				case EngineIOS:
					constraints[compat.IOS] = version
				case EngineNode:
					constraints[compat.Node] = version
				case EngineSafari:
					constraints[compat.Safari] = version
				default:
					panic("Invalid engine name")
				}
				continue
			}
		}

		log.AddError(nil, logger.Loc{}, fmt.Sprintf("Invalid version: %q", engine.Version))
	}

	return compat.UnsupportedJSFeatures(constraints), compat.UnsupportedCSSFeatures(constraints)
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

func validateExternals(log logger.Log, fs fs.FS, paths []string) config.ExternalModules {
	result := config.ExternalModules{
		NodeModules: make(map[string]bool),
		AbsPaths:    make(map[string]bool),
	}
	for _, path := range paths {
		if index := strings.IndexByte(path, '*'); index != -1 {
			if strings.ContainsRune(path[index+1:], '*') {
				log.AddError(nil, logger.Loc{}, fmt.Sprintf("External path %q cannot have more than one \"*\" wildcard", path))
			} else {
				result.Patterns = append(result.Patterns, config.WildcardPattern{
					Prefix: path[:index],
					Suffix: path[index+1:],
				})
			}
		} else if resolver.IsPackagePath(path) {
			result.NodeModules[path] = true
		} else if absPath := validatePath(log, fs, path, "external path"); absPath != "" {
			result.AbsPaths[absPath] = true
		}
	}
	return result
}

func isValidExtension(ext string) bool {
	return len(ext) >= 2 && ext[0] == '.' && ext[len(ext)-1] != '.'
}

func validateResolveExtensions(log logger.Log, order []string) []string {
	if order == nil {
		return []string{".tsx", ".ts", ".jsx", ".mjs", ".cjs", ".js", ".css", ".json"}
	}
	for _, ext := range order {
		if !isValidExtension(ext) {
			log.AddError(nil, logger.Loc{}, fmt.Sprintf("Invalid file extension: %q", ext))
		}
	}
	return order
}

func validateLoaders(log logger.Log, loaders map[string]Loader) map[string]config.Loader {
	result := bundler.DefaultExtensionToLoaderMap()
	if loaders != nil {
		for ext, loader := range loaders {
			if !isValidExtension(ext) {
				log.AddError(nil, logger.Loc{}, fmt.Sprintf("Invalid file extension: %q", ext))
			}
			result[ext] = validateLoader(loader)
		}
	}
	return result
}

func validateJSX(log logger.Log, text string, name string) []string {
	if text == "" {
		return nil
	}
	parts := strings.Split(text, ".")
	for _, part := range parts {
		if !js_lexer.IsIdentifier(part) {
			log.AddError(nil, logger.Loc{}, fmt.Sprintf("Invalid JSX %s: %q", name, text))
			return nil
		}
	}
	return parts
}

func validateDefines(log logger.Log, defines map[string]string, pureFns []string) (*config.ProcessedDefines, []config.InjectedDefine) {
	if len(defines) == 0 && len(pureFns) == 0 {
		return nil, nil
	}

	rawDefines := make(map[string]config.DefineData)
	valueToInject := make(map[string]config.InjectedDefine)
	var definesToInject []string

	for key, value := range defines {
		// The key must be a dot-separated identifier list
		for _, part := range strings.Split(key, ".") {
			if !js_lexer.IsIdentifier(part) {
				log.AddError(nil, logger.Loc{}, fmt.Sprintf("Invalid define key: %q", key))
				continue
			}
		}

		// Allow substituting for an identifier
		if js_lexer.IsIdentifier(value) {
			if _, ok := js_lexer.Keywords[value]; !ok {
				name := value // The closure must close over a variable inside the loop
				rawDefines[key] = config.DefineData{
					DefineFunc: func(args config.DefineArgs) js_ast.E {
						return &js_ast.EIdentifier{Ref: args.FindSymbol(args.Loc, name)}
					},
				}

				// Try to be helpful for common mistakes
				if key == "process.env.NODE_ENV" {
					log.AddWarning(nil, logger.Loc{}, fmt.Sprintf(
						"%q is defined as an identifier instead of a string (surround %q with double quotes to get a string)", key, value))
				}
				continue
			}
		}

		// Parse the value as JSON
		source := logger.Source{Contents: value}
		expr, ok := js_parser.ParseJSON(logger.NewDeferLog(), source, js_parser.JSONOptions{})
		if !ok {
			log.AddError(nil, logger.Loc{}, fmt.Sprintf("Invalid define value (must be valid JSON syntax or a single identifier): %s", value))
			continue
		}

		var fn config.DefineFunc
		switch e := expr.Data.(type) {
		// These values are inserted inline, and can participate in constant folding
		case *js_ast.ENull:
			fn = func(config.DefineArgs) js_ast.E { return &js_ast.ENull{} }
		case *js_ast.EBoolean:
			fn = func(config.DefineArgs) js_ast.E { return &js_ast.EBoolean{Value: e.Value} }
		case *js_ast.EString:
			fn = func(config.DefineArgs) js_ast.E { return &js_ast.EString{Value: e.Value} }
		case *js_ast.ENumber:
			fn = func(config.DefineArgs) js_ast.E { return &js_ast.ENumber{Value: e.Value} }

		// These values are extracted into a shared symbol reference
		case *js_ast.EArray, *js_ast.EObject:
			definesToInject = append(definesToInject, key)
			valueToInject[key] = config.InjectedDefine{Source: source, Data: e, Name: key}
			continue
		}

		rawDefines[key] = config.DefineData{DefineFunc: fn}
	}

	// Sort injected defines for determinism, since the imports will be injected
	// into every file in the order that we return them from this function
	injectedDefines := make([]config.InjectedDefine, len(definesToInject))
	sort.Strings(definesToInject)
	for i, key := range definesToInject {
		index := i // Capture this for the closure below
		injectedDefines[i] = valueToInject[key]
		rawDefines[key] = config.DefineData{DefineFunc: func(args config.DefineArgs) js_ast.E {
			return &js_ast.EIdentifier{Ref: args.SymbolForDefine(index)}
		}}
	}

	for _, key := range pureFns {
		// The key must be a dot-separated identifier list
		for _, part := range strings.Split(key, ".") {
			if !js_lexer.IsIdentifier(part) {
				log.AddError(nil, logger.Loc{}, fmt.Sprintf("Invalid pure function: %q", key))
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
		log.AddError(nil, logger.Loc{}, fmt.Sprintf("Invalid %s: %s", pathKind, relPath))
	}
	return absPath
}

func validateOutputExtensions(log logger.Log, outExtensions map[string]string) (js string, css string) {
	for key, value := range outExtensions {
		if !isValidExtension(value) {
			log.AddError(nil, logger.Loc{}, fmt.Sprintf("Invalid output extension: %q", value))
		}
		switch key {
		case ".js":
			js = value
		case ".css":
			css = value
		default:
			log.AddError(nil, logger.Loc{}, fmt.Sprintf("Invalid output extension: %q (valid: .css, .js)", key))
		}
	}
	return
}

func convertLocationToPublic(loc *logger.MsgLocation) *Location {
	if loc != nil {
		return &Location{
			File:      loc.File,
			Namespace: loc.Namespace,
			Line:      loc.Line,
			Column:    loc.Column,
			Length:    loc.Length,
			LineText:  loc.LineText,
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
				Text:     msg.Data.Text,
				Location: convertLocationToPublic(msg.Data.Location),
				Notes:    notes,
				Detail:   msg.Data.UserDetail,
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
			File:      loc.File,
			Namespace: namespace,
			Line:      loc.Line,
			Column:    loc.Column,
			Length:    loc.Length,
			LineText:  loc.LineText,
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
			Kind: kind,
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

////////////////////////////////////////////////////////////////////////////////
// Build API

type internalBuildResult struct {
	result    BuildResult
	options   config.Options
	watchData fs.WatchData
}

func buildImpl(buildOpts BuildOptions) internalBuildResult {
	logOptions := logger.OutputOptions{
		IncludeSource: true,
		ErrorLimit:    buildOpts.ErrorLimit,
		Color:         validateColor(buildOpts.Color),
		LogLevel:      validateLogLevel(buildOpts.LogLevel),
	}
	log := logger.NewStderrLog(logOptions)

	// Validate that the current working directory is an absolute path
	realFS, err := fs.RealFS(fs.RealFSOptions{
		AbsWorkingDir: buildOpts.AbsWorkingDir,
	})
	if err != nil {
		log.AddError(nil, logger.Loc{}, err.Error())
		return internalBuildResult{result: BuildResult{Errors: convertMessagesToPublic(logger.Error, log.Done())}}
	}

	// Do not re-evaluate plugins when rebuilding
	plugins := loadPlugins(realFS, log, buildOpts.Plugins)
	return rebuildImpl(buildOpts, cache.MakeCacheSet(), plugins, logOptions, log, false /* isRebuild */)
}

func rebuildImpl(
	buildOpts BuildOptions,
	caches *cache.CacheSet,
	plugins []config.Plugin,
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
	jsFeatures, cssFeatures := validateFeatures(log, buildOpts.Target, buildOpts.Engines)
	outJS, outCSS := validateOutputExtensions(log, buildOpts.OutExtensions)
	defines, injectedDefines := validateDefines(log, buildOpts.Define, buildOpts.Pure)
	options := config.Options{
		UnsupportedJSFeatures:  jsFeatures,
		UnsupportedCSSFeatures: cssFeatures,
		JSX: config.JSXOptions{
			Factory:  validateJSX(log, buildOpts.JSXFactory, "factory"),
			Fragment: validateJSX(log, buildOpts.JSXFragment, "fragment"),
		},
		Defines:               defines,
		InjectedDefines:       injectedDefines,
		Platform:              validatePlatform(buildOpts.Platform),
		SourceMap:             validateSourceMap(buildOpts.Sourcemap),
		ExcludeSourcesContent: buildOpts.SourcesContent == SourcesContentExclude,
		MangleSyntax:          buildOpts.MinifySyntax,
		RemoveWhitespace:      buildOpts.MinifyWhitespace,
		MinifyIdentifiers:     buildOpts.MinifyIdentifiers,
		ASCIIOnly:             validateASCIIOnly(buildOpts.Charset),
		IgnoreDCEAnnotations:  validateIgnoreDCEAnnotations(buildOpts.TreeShaking),
		GlobalName:            validateGlobalName(log, buildOpts.GlobalName),
		CodeSplitting:         buildOpts.Splitting,
		OutputFormat:          validateFormat(buildOpts.Format),
		AbsOutputFile:         validatePath(log, realFS, buildOpts.Outfile, "outfile path"),
		AbsOutputDir:          validatePath(log, realFS, buildOpts.Outdir, "outdir path"),
		AbsOutputBase:         validatePath(log, realFS, buildOpts.Outbase, "outbase path"),
		AbsMetadataFile:       validatePath(log, realFS, buildOpts.Metafile, "metafile path"),
		OutputExtensionJS:     outJS,
		OutputExtensionCSS:    outCSS,
		ExtensionToLoader:     validateLoaders(log, buildOpts.Loader),
		ExtensionOrder:        validateResolveExtensions(log, buildOpts.ResolveExtensions),
		ExternalModules:       validateExternals(log, realFS, buildOpts.External),
		TsConfigOverride:      validatePath(log, realFS, buildOpts.Tsconfig, "tsconfig path"),
		MainFields:            buildOpts.MainFields,
		PublicPath:            buildOpts.PublicPath,
		KeepNames:             buildOpts.KeepNames,
		InjectAbsPaths:        make([]string, len(buildOpts.Inject)),
		AbsNodePaths:          make([]string, len(buildOpts.NodePaths)),
		Banner:                buildOpts.Banner,
		Footer:                buildOpts.Footer,
		PreserveSymlinks:      buildOpts.PreserveSymlinks,
		WatchMode:             buildOpts.Watch != nil,
		Plugins:               plugins,
	}
	for i, path := range buildOpts.Inject {
		options.InjectAbsPaths[i] = validatePath(log, realFS, path, "inject path")
	}
	for i, path := range buildOpts.NodePaths {
		options.AbsNodePaths[i] = validatePath(log, realFS, path, "node path")
	}
	if options.PublicPath != "" && !strings.HasSuffix(options.PublicPath, "/") && !strings.HasSuffix(options.PublicPath, "\\") {
		options.PublicPath += "/"
	}
	entryPoints := append([]string{}, buildOpts.EntryPoints...)
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
		log.AddError(nil, logger.Loc{},
			"Must use \"outdir\" when there are multiple input files")
	} else if options.AbsOutputDir == "" && options.CodeSplitting {
		log.AddError(nil, logger.Loc{},
			"Must use \"outdir\" when code splitting is enabled")
	} else if options.AbsOutputFile != "" && options.AbsOutputDir != "" {
		log.AddError(nil, logger.Loc{}, "Cannot use both \"outfile\" and \"outdir\"")
	} else if options.AbsOutputFile != "" {
		// If the output file is specified, use it to derive the output directory
		options.AbsOutputDir = realFS.Dir(options.AbsOutputFile)
	} else if options.AbsOutputDir == "" {
		options.WriteToStdout = true

		// Forbid certain features when writing to stdout
		if options.SourceMap != config.SourceMapNone && options.SourceMap != config.SourceMapInline {
			log.AddError(nil, logger.Loc{}, "Cannot use an external source map without an output path")
		}
		if options.AbsMetadataFile != "" {
			log.AddError(nil, logger.Loc{}, "Cannot use \"metafile\" without an output path")
		}
		for _, loader := range options.ExtensionToLoader {
			if loader == config.LoaderFile {
				log.AddError(nil, logger.Loc{}, "Cannot use the \"file\" loader without an output path")
				break
			}
		}

		// Use the current directory as the output directory instead of an empty
		// string because external modules with relative paths need a base directory.
		options.AbsOutputDir = realFS.Cwd()
	}

	if !buildOpts.Bundle {
		// Disallow bundle-only options when not bundling
		if len(options.ExternalModules.NodeModules) > 0 || len(options.ExternalModules.AbsPaths) > 0 {
			log.AddError(nil, logger.Loc{}, "Cannot use \"external\" without \"bundle\"")
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
		log.AddError(nil, logger.Loc{}, "Splitting currently only works with the \"esm\" format")
	}

	var outputFiles []OutputFile
	var watchData fs.WatchData

	// Stop now if there were errors
	resolver := resolver.NewResolver(realFS, log, caches, options)
	if !log.HasErrors() {
		// Scan over the bundle
		bundle := bundler.ScanBundle(log, realFS, resolver, caches, entryPoints, options)
		watchData = realFS.WatchData()

		// Stop now if there were errors
		if !log.HasErrors() {
			// Compile the bundle
			results := bundle.Compile(log, options)

			// Stop now if there were errors
			if !log.HasErrors() {
				if buildOpts.Write {
					// Special-case writing to stdout
					if options.WriteToStdout {
						if len(results) != 1 {
							log.AddError(nil, logger.Loc{}, fmt.Sprintf(
								"Internal error: did not expect to generate %d files when writing to stdout", len(results)))
						} else if _, err := os.Stdout.Write(results[0].Contents); err != nil {
							log.AddError(nil, logger.Loc{}, fmt.Sprintf(
								"Failed to write to stdout: %s", err.Error()))
						}
					} else {
						// Write out files in parallel
						waitGroup := sync.WaitGroup{}
						waitGroup.Add(len(results))
						for _, result := range results {
							go func(result bundler.OutputFile) {
								fs.BeforeFileOpen()
								defer fs.AfterFileClose()
								if err := os.MkdirAll(realFS.Dir(result.AbsPath), 0755); err != nil {
									log.AddError(nil, logger.Loc{}, fmt.Sprintf(
										"Failed to create output directory: %s", err.Error()))
								} else {
									var mode os.FileMode = 0644
									if result.IsExecutable {
										mode = 0755
									}
									if err := ioutil.WriteFile(result.AbsPath, result.Contents, mode); err != nil {
										log.AddError(nil, logger.Loc{}, fmt.Sprintf(
											"Failed to write to output file: %s", err.Error()))
									}
								}
								waitGroup.Done()
							}(result)
						}
						waitGroup.Wait()
					}
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
				value := rebuildImpl(buildOpts, caches, plugins, logOptions, logger.NewStderrLog(logOptions), true /* isRebuild */)
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
			value := rebuildImpl(buildOpts, caches, plugins, logOptions, logger.NewStderrLog(logOptions), true /* isRebuild */)
			if watch != nil {
				watch.setWatchData(value.watchData)
			}
			return value.result
		}
	}

	result := BuildResult{
		Errors:      convertMessagesToPublic(logger.Error, msgs),
		Warnings:    convertMessagesToPublic(logger.Warning, msgs),
		OutputFiles: outputFiles,
		Rebuild:     rebuild,
		Stop:        stop,
	}
	return internalBuildResult{
		result:    result,
		options:   options,
		watchData: watchData,
	}
}

type watcher struct {
	mutex             sync.Mutex
	data              fs.WatchData
	resolver          resolver.Resolver
	shouldStop        int32
	rebuild           func() fs.WatchData
	recentItems       []string
	itemsToScan       []string
	itemsPerIteration int
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
		// Note: Do not change these log messages without a breaking version change.
		// People want to run regexes over esbuild's stderr stream to look for these
		// messages instead of using esbuild's API.

		if logLevel == LogLevelInfo {
			logger.PrintTextWithColor(os.Stderr, useColor, func(colors logger.Colors) string {
				return fmt.Sprintf("%s[watch] build finished, watching for changes...%s\n", colors.Dim, colors.Default)
			})
		}

		for atomic.LoadInt32(&w.shouldStop) == 0 {
			// Sleep for the watch interval
			time.Sleep(watchIntervalSleep)

			// Rebuild if we're dirty
			if absPath := w.tryToFindDirtyPath(); absPath != "" {
				if logLevel == LogLevelInfo {
					logger.PrintTextWithColor(os.Stderr, useColor, func(colors logger.Colors) string {
						prettyPath := w.resolver.PrettyPath(logger.Path{Text: absPath, Namespace: "file"})
						return fmt.Sprintf("%s[watch] build started (change: %q)%s\n", colors.Dim, prettyPath, colors.Default)
					})
				}

				// Run the build
				w.setWatchData(w.rebuild())

				if logLevel == LogLevelInfo {
					logger.PrintTextWithColor(os.Stderr, useColor, func(colors logger.Colors) string {
						return fmt.Sprintf("%s[watch] build finished%s\n", colors.Dim, colors.Default)
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
		for i := int32(len(items) - 1); i > 0; i-- { // Fisher–Yates shuffle
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
		if w.data.Paths[path]() {
			// Move this path to the back of the list (i.e. the "most recent" position)
			copy(w.recentItems[i:], w.recentItems[i+1:])
			w.recentItems[len(w.recentItems)-1] = path
			return path
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
		if w.data.Paths[path]() {
			// Mark this item as recent by adding it to the back of the list
			w.recentItems = append(w.recentItems, path)
			if len(w.recentItems) > maxRecentItemCount {
				// Remove items from the front of the list when we hit the limit
				copy(w.recentItems, w.recentItems[1:])
				w.recentItems = w.recentItems[:maxRecentItemCount]
			}
			return path
		}
	}
	return ""
}

////////////////////////////////////////////////////////////////////////////////
// Transform API

func transformImpl(input string, transformOpts TransformOptions) TransformResult {
	log := logger.NewStderrLog(logger.OutputOptions{
		IncludeSource: true,
		ErrorLimit:    transformOpts.ErrorLimit,
		Color:         validateColor(transformOpts.Color),
		LogLevel:      validateLogLevel(transformOpts.LogLevel),
	})

	// Settings from the user come first
	preserveUnusedImportsTS := false
	useDefineForClassFieldsTS := false
	jsx := config.JSXOptions{
		Factory:  validateJSX(log, transformOpts.JSXFactory, "factory"),
		Fragment: validateJSX(log, transformOpts.JSXFragment, "fragment"),
	}

	// Settings from "tsconfig.json" override those
	caches := cache.MakeCacheSet()
	if transformOpts.TsconfigRaw != "" {
		source := logger.Source{
			KeyPath:    logger.Path{Text: "tsconfig.json"},
			PrettyPath: "tsconfig.json",
			Contents:   transformOpts.TsconfigRaw,
		}
		if result := resolver.ParseTSConfigJSON(log, source, &caches.JSONCache, nil); result != nil {
			if len(result.JSXFactory) > 0 {
				jsx.Factory = result.JSXFactory
			}
			if len(result.JSXFragmentFactory) > 0 {
				jsx.Fragment = result.JSXFragmentFactory
			}
			if result.UseDefineForClassFields {
				useDefineForClassFieldsTS = true
			}
			if result.PreserveImportsNotUsedAsValues {
				preserveUnusedImportsTS = true
			}
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
	jsFeatures, cssFeatures := validateFeatures(log, transformOpts.Target, transformOpts.Engines)
	defines, injectedDefines := validateDefines(log, transformOpts.Define, transformOpts.Pure)
	options := config.Options{
		UnsupportedJSFeatures:   jsFeatures,
		UnsupportedCSSFeatures:  cssFeatures,
		JSX:                     jsx,
		Defines:                 defines,
		InjectedDefines:         injectedDefines,
		SourceMap:               validateSourceMap(transformOpts.Sourcemap),
		ExcludeSourcesContent:   transformOpts.SourcesContent == SourcesContentExclude,
		OutputFormat:            validateFormat(transformOpts.Format),
		GlobalName:              validateGlobalName(log, transformOpts.GlobalName),
		MangleSyntax:            transformOpts.MinifySyntax,
		RemoveWhitespace:        transformOpts.MinifyWhitespace,
		MinifyIdentifiers:       transformOpts.MinifyIdentifiers,
		ASCIIOnly:               validateASCIIOnly(transformOpts.Charset),
		IgnoreDCEAnnotations:    validateIgnoreDCEAnnotations(transformOpts.TreeShaking),
		AbsOutputFile:           transformOpts.Sourcefile + "-out",
		KeepNames:               transformOpts.KeepNames,
		UseDefineForClassFields: useDefineForClassFieldsTS,
		PreserveUnusedImportsTS: preserveUnusedImportsTS,
		Stdin: &config.StdinInfo{
			Loader:     validateLoader(transformOpts.Loader),
			Contents:   input,
			SourceFile: transformOpts.Sourcefile,
		},
		Banner: transformOpts.Banner,
		Footer: transformOpts.Footer,
	}
	if options.SourceMap == config.SourceMapLinkedWithComment {
		// Linked source maps don't make sense because there's no output file name
		log.AddError(nil, logger.Loc{}, "Cannot transform with linked source maps")
	}
	if options.SourceMap != config.SourceMapNone && options.Stdin.SourceFile == "" {
		log.AddError(nil, logger.Loc{},
			"Must use \"sourcefile\" with \"sourcemap\" to set the original file name")
	}

	// Set the output mode using other settings
	if options.OutputFormat != config.FormatPreserve {
		options.Mode = config.ModeConvertFormat
	}

	var results []bundler.OutputFile

	// Stop now if there were errors
	if !log.HasErrors() {
		// Scan over the bundle
		mockFS := fs.MockFS(make(map[string]string))
		resolver := resolver.NewResolver(mockFS, log, caches, options)
		bundle := bundler.ScanBundle(log, mockFS, resolver, caches, nil, options)

		// Stop now if there were errors
		if !log.HasErrors() {
			// Compile the bundle
			results = bundle.Compile(log, options)
		}
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

	msgs := log.Done()
	return TransformResult{
		Errors:   convertMessagesToPublic(logger.Error, msgs),
		Warnings: convertMessagesToPublic(logger.Warning, msgs),
		Code:     code,
		Map:      sourceMap,
	}
}

////////////////////////////////////////////////////////////////////////////////
// Plugin API

type pluginImpl struct {
	log    logger.Log
	fs     fs.FS
	plugin config.Plugin
}

func (impl *pluginImpl) OnResolve(options OnResolveOptions, callback func(OnResolveArgs) (OnResolveResult, error)) {
	filter, err := config.CompileFilterForPlugin(impl.plugin.Name, "OnResolve", options.Filter)
	if filter == nil {
		impl.log.AddError(nil, logger.Loc{}, err.Error())
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
				PluginData: args.PluginData,
			})
			result.PluginName = response.PluginName

			if err != nil {
				result.ThrownError = err
				return
			}

			result.Path = logger.Path{Text: response.Path, Namespace: response.Namespace}
			result.External = response.External
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

func (impl *pluginImpl) OnLoad(options OnLoadOptions, callback func(OnLoadArgs) (OnLoadResult, error)) {
	filter, err := config.CompileFilterForPlugin(impl.plugin.Name, "OnLoad", options.Filter)
	if filter == nil {
		impl.log.AddError(nil, logger.Loc{}, err.Error())
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
			})
			result.PluginName = response.PluginName

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

func loadPlugins(fs fs.FS, log logger.Log, plugins []Plugin) (results []config.Plugin) {
	for i, item := range plugins {
		if item.Name == "" {
			log.AddError(nil, logger.Loc{}, fmt.Sprintf("Plugin at index %d is missing a name", i))
			continue
		}

		impl := &pluginImpl{
			fs:     fs,
			log:    log,
			plugin: config.Plugin{Name: item.Name},
		}

		item.Setup(impl)
		results = append(results, impl.plugin)
	}
	return
}

////////////////////////////////////////////////////////////////////////////////
// Serve API

type apiHandler struct {
	mutex            sync.Mutex
	outdirPathPrefix string
	servedir         string
	options          *config.Options
	onRequest        func(ServeOnRequestArgs)
	rebuild          func() BuildResult
	currentBuild     *runningBuild
	fs               fs.FS
}

type runningBuild struct {
	waitGroup sync.WaitGroup
	result    BuildResult
}

func (h *apiHandler) build() BuildResult {
	build := func() *runningBuild {
		h.mutex.Lock()
		defer h.mutex.Unlock()
		if h.currentBuild == nil {
			build := &runningBuild{}
			build.waitGroup.Add(1)
			h.currentBuild = build

			// Build on another thread
			go func() {
				result := h.rebuild()
				h.rebuild = result.Rebuild
				build.result = result
				build.waitGroup.Done()

				// Build results stay valid for a little bit afterward since a page
				// load may involve multiple requests and don't want to rebuild
				// separately for each of those requests.
				time.Sleep(250 * time.Millisecond)
				h.mutex.Lock()
				defer h.mutex.Unlock()
				h.currentBuild = nil
			}()
		}
		return h.currentBuild
	}()
	build.waitGroup.Wait()
	return build.result
}

func escapeForHTML(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}

func escapeForAttribute(text string) string {
	text = escapeForHTML(text)
	text = strings.ReplaceAll(text, "\"", "&quot;")
	text = strings.ReplaceAll(text, "'", "&apos;")
	return text
}

func (h *apiHandler) notifyRequest(duration time.Duration, req *http.Request, status int) {
	if h.onRequest != nil {
		h.onRequest(ServeOnRequestArgs{
			RemoteAddress: req.RemoteAddr,
			Method:        req.Method,
			Path:          req.URL.Path,
			Status:        status,
			TimeInMS:      int(duration.Milliseconds()),
		})
	}
}

func errorsToString(errors []Message) string {
	stderrOptions := logger.OutputOptions{IncludeSource: true}
	terminalOptions := logger.TerminalInfo{}
	sb := strings.Builder{}
	limit := 5
	for i, msg := range convertMessagesToInternal(nil, logger.Error, errors) {
		if i == limit {
			sb.WriteString(fmt.Sprintf("%d out of %d errors shown\n", limit, len(errors)))
			break
		}
		sb.WriteString(msg.String(stderrOptions, terminalOptions))
	}
	return sb.String()
}

func (h *apiHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	start := time.Now()

	// Handle get requests
	if req.Method == "GET" && strings.HasPrefix(req.URL.Path, "/") {
		res.Header().Set("Access-Control-Allow-Origin", "*")
		queryPath := path.Clean(req.URL.Path)[1:]
		result := h.build()

		// Requests fail if the build had errors
		if len(result.Errors) > 0 {
			go h.notifyRequest(time.Since(start), req, http.StatusServiceUnavailable)
			res.Header().Set("Content-Type", "text/plain; charset=utf-8")
			res.WriteHeader(http.StatusServiceUnavailable)
			res.Write([]byte(errorsToString(result.Errors)))
			return
		}

		var kind fs.EntryKind
		var fileContents []byte
		dirEntries := make(map[string]bool)
		fileEntries := make(map[string]bool)

		// Check for a match with the results if we're within the output directory
		if strings.HasPrefix(queryPath, h.outdirPathPrefix) {
			outdirQueryPath := queryPath[len(h.outdirPathPrefix):]
			if strings.HasPrefix(outdirQueryPath, "/") {
				outdirQueryPath = outdirQueryPath[1:]
			}
			kind, fileContents = h.matchQueryPathToResult(outdirQueryPath, &result, dirEntries, fileEntries)
		} else {
			// Create a fake directory entry for the output path so that it appears to be a real directory
			p := h.outdirPathPrefix
			for p != "" {
				var dir string
				var base string
				if slash := strings.IndexByte(p, '/'); slash == -1 {
					base = p
				} else {
					dir = p[:slash]
					base = p[slash+1:]
				}
				if dir == queryPath {
					kind = fs.DirEntry
					dirEntries[base] = true
					break
				}
				p = dir
			}
		}

		// Check for a file in the fallback directory
		if h.servedir != "" && kind != fs.FileEntry {
			contents, err := h.fs.ReadFile(h.fs.Join(h.servedir, queryPath))
			if err == nil {
				fileContents = []byte(contents)
				kind = fs.FileEntry
			} else if err != syscall.ENOENT && err != syscall.EISDIR {
				go h.notifyRequest(time.Since(start), req, http.StatusInternalServerError)
				res.WriteHeader(http.StatusInternalServerError)
				res.Write([]byte(fmt.Sprintf("Internal server error: %s", err.Error())))
				return
			}
		}

		// Check for a directory in the fallback directory
		var fallbackIndexName string
		if h.servedir != "" && kind != fs.FileEntry {
			entries, err := h.fs.ReadDirectory(h.fs.Join(h.servedir, queryPath))
			if err == nil {
				kind = fs.DirEntry
				for name, entry := range entries {
					switch entry.Kind(h.fs) {
					case fs.DirEntry:
						dirEntries[name] = true
					case fs.FileEntry:
						fileEntries[name] = true
						if name == "index.html" {
							fallbackIndexName = name
						}
					}
				}
			} else if err != syscall.ENOENT {
				go h.notifyRequest(time.Since(start), req, http.StatusInternalServerError)
				res.WriteHeader(http.StatusInternalServerError)
				res.Write([]byte(fmt.Sprintf("Internal server error: %s", err.Error())))
				return
			}
		}

		// Redirect to a trailing slash for directories
		if kind == fs.DirEntry && !strings.HasSuffix(req.URL.Path, "/") {
			res.Header().Set("Location", req.URL.Path+"/")
			go h.notifyRequest(time.Since(start), req, http.StatusFound)
			res.WriteHeader(http.StatusFound)
			res.Write(nil)
			return
		}

		// Serve a "index.html" file if present
		if kind == fs.DirEntry && fallbackIndexName != "" {
			queryPath += "/" + fallbackIndexName
			contents, err := h.fs.ReadFile(h.fs.Join(h.servedir, queryPath))
			if err == nil {
				fileContents = []byte(contents)
				kind = fs.FileEntry
			} else if err != syscall.ENOENT {
				go h.notifyRequest(time.Since(start), req, http.StatusInternalServerError)
				res.WriteHeader(http.StatusInternalServerError)
				res.Write([]byte(fmt.Sprintf("Internal server error: %s", err.Error())))
				return
			}
		}

		// Serve a file
		if kind == fs.FileEntry {
			if contentType := mime.TypeByExtension(path.Ext(queryPath)); contentType != "" {
				res.Header().Set("Content-Type", contentType)
			}
			go h.notifyRequest(time.Since(start), req, http.StatusOK)
			res.Write(fileContents)
			return
		}

		// Serve a directory listing
		if kind == fs.DirEntry {
			html := respondWithDirList(queryPath, dirEntries, fileEntries)
			res.Header().Set("Content-Type", "text/html; charset=utf-8")
			go h.notifyRequest(time.Since(start), req, http.StatusOK)
			res.Write(html)
			return
		}
	}

	// Default to a 404
	res.Header().Set("Content-Type", "text/plain; charset=utf-8")
	go h.notifyRequest(time.Since(start), req, http.StatusNotFound)
	res.WriteHeader(http.StatusNotFound)
	res.Write([]byte("404 - Not Found"))
}

func (h *apiHandler) matchQueryPathToResult(
	queryPath string,
	result *BuildResult,
	dirEntries map[string]bool,
	fileEntries map[string]bool,
) (fs.EntryKind, []byte) {
	queryIsDir := false
	queryDir := queryPath
	if queryDir != "" {
		queryDir += "/"
	}

	// Check the output files for a match
	for _, file := range result.OutputFiles {
		if relPath, ok := h.fs.Rel(h.options.AbsOutputDir, file.Path); ok {
			relPath = strings.ReplaceAll(relPath, "\\", "/")

			// An exact match
			if relPath == queryPath {
				return fs.FileEntry, file.Contents
			}

			// A match inside this directory
			if strings.HasPrefix(relPath, queryDir) {
				entry := relPath[len(queryDir):]
				queryIsDir = true
				if slash := strings.IndexByte(entry, '/'); slash == -1 {
					fileEntries[entry] = true
				} else if dir := entry[:slash]; !dirEntries[dir] {
					dirEntries[dir] = true
				}
			}
		}
	}

	// Treat this as a directory if it's non-empty
	if queryIsDir {
		return fs.DirEntry, nil
	}

	return 0, nil
}

func respondWithDirList(queryPath string, dirEntries map[string]bool, fileEntries map[string]bool) []byte {
	queryPath = "/" + queryPath
	queryDir := queryPath
	if queryDir != "/" {
		queryDir += "/"
	}
	html := strings.Builder{}
	html.WriteString(`<!doctype html>`)
	html.WriteString(`<meta charset="utf8">`)
	html.WriteString(`<title>Directory: `)
	html.WriteString(escapeForHTML(queryDir))
	html.WriteString(`</title>`)
	html.WriteString(`<h1>Directory: `)
	html.WriteString(escapeForHTML(queryDir))
	html.WriteString(`</h1>`)
	html.WriteString(`<ul>`)

	// Link to the parent directory
	if queryPath != "/" {
		parentDir := path.Dir(queryPath)
		if parentDir != "/" {
			parentDir += "/"
		}
		html.WriteString(fmt.Sprintf(`<li><a href="%s">../</a></li>`, escapeForAttribute(parentDir)))
	}

	// Link to child directories
	strings := make([]string, 0, len(dirEntries)+len(fileEntries))
	for entry := range dirEntries {
		strings = append(strings, entry)
	}
	sort.Strings(strings)
	for _, entry := range strings {
		html.WriteString(fmt.Sprintf(`<li><a href="%s/">%s/</a></li>`, escapeForAttribute(path.Join(queryPath, entry)), escapeForHTML(entry)))
	}

	// Link to files in the directory
	strings = strings[:0]
	for entry := range fileEntries {
		strings = append(strings, entry)
	}
	sort.Strings(strings)
	for _, entry := range strings {
		html.WriteString(fmt.Sprintf(`<li><a href="%s">%s</a></li>`, escapeForAttribute(path.Join(queryPath, entry)), escapeForHTML(entry)))
	}

	html.WriteString(`</ul>`)
	return []byte(html.String())
}

// This is used to make error messages platform-independent
func prettyPrintPath(fs fs.FS, path string) string {
	if relPath, ok := fs.Rel(fs.Cwd(), path); ok {
		return strings.ReplaceAll(relPath, "\\", "/")
	}
	return path
}

func serveImpl(serveOptions ServeOptions, buildOptions BuildOptions) (ServeResult, error) {
	realFS, err := fs.RealFS(fs.RealFSOptions{
		AbsWorkingDir: buildOptions.AbsWorkingDir,
	})
	if err != nil {
		return ServeResult{}, err
	}
	buildOptions.Incremental = true
	buildOptions.Write = false

	// Watch and serve are both different ways of rebuilding, and cannot be combined
	if buildOptions.Watch != nil {
		return ServeResult{}, fmt.Errorf("Cannot use \"watch\" with \"serve\"")
	}

	// Validate the fallback path
	if serveOptions.Servedir != "" {
		if absPath, ok := realFS.Abs(serveOptions.Servedir); ok {
			serveOptions.Servedir = absPath
		} else {
			return ServeResult{}, fmt.Errorf("Invalid serve path: %s", serveOptions.Servedir)
		}
	}

	// If there is no output directory, set the output directory to something so
	// the build doesn't try to write to stdout. Make sure not to set this to a
	// path that may contain the user's files in it since we don't want to get
	// errors about overwriting input files.
	outdirPathPrefix := ""
	if buildOptions.Outdir == "" && buildOptions.Outfile == "" {
		buildOptions.Outdir = realFS.Join(realFS.Cwd(), "...")
	} else if serveOptions.Servedir != "" {
		// Compute the output directory
		var outdir string
		if buildOptions.Outdir != "" {
			if absPath, ok := realFS.Abs(buildOptions.Outdir); ok {
				outdir = absPath
			} else {
				return ServeResult{}, fmt.Errorf("Invalid outdir path: %s", buildOptions.Outdir)
			}
		} else {
			if absPath, ok := realFS.Abs(buildOptions.Outfile); ok {
				outdir = realFS.Dir(absPath)
			} else {
				return ServeResult{}, fmt.Errorf("Invalid outdir path: %s", buildOptions.Outfile)
			}
		}

		// Make sure the output directory is contained in the fallback directory
		relPath, ok := realFS.Rel(serveOptions.Servedir, outdir)
		if !ok {
			return ServeResult{}, fmt.Errorf(
				"Cannot compute relative path from %q to %q\n", serveOptions.Servedir, outdir)
		}
		relPath = strings.ReplaceAll(relPath, "\\", "/") // Fix paths on Windows
		if relPath == ".." || strings.HasPrefix(relPath, "../") {
			return ServeResult{}, fmt.Errorf(
				"Output directory %q must be contained in serve directory %q",
				prettyPrintPath(realFS, outdir),
				prettyPrintPath(realFS, serveOptions.Servedir),
			)
		}
		if relPath != "." {
			outdirPathPrefix = relPath
		}
	}

	// Pick the port
	var listener net.Listener
	host := "127.0.0.1"
	if serveOptions.Host != "" {
		host = serveOptions.Host
	}
	if serveOptions.Port == 0 {
		// Default to picking a "800X" port
		for port := 8000; port <= 8009; port++ {
			if result, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port)); err == nil {
				listener = result
				break
			}
		}
	}
	if listener == nil {
		// Otherwise pick the provided port
		if result, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, serveOptions.Port)); err != nil {
			return ServeResult{}, err
		} else {
			listener = result
		}
	}

	// Try listening on the provided port
	addr := listener.Addr().String()

	// Extract the real port in case we passed a port of "0"
	var result ServeResult
	if host, text, err := net.SplitHostPort(addr); err == nil {
		if port, err := strconv.ParseInt(text, 10, 32); err == nil {
			result.Port = uint16(port)
			result.Host = host
		}
	}

	// The first build will just build normally
	var handler *apiHandler
	handler = &apiHandler{
		onRequest:        serveOptions.OnRequest,
		outdirPathPrefix: outdirPathPrefix,
		servedir:         serveOptions.Servedir,
		rebuild: func() BuildResult {
			build := buildImpl(buildOptions)
			if handler.options == nil {
				handler.options = &build.options
			}
			return build.result
		},
		fs: realFS,
	}

	// Start the server
	server := &http.Server{Addr: addr, Handler: handler}
	wait := make(chan error, 1)
	result.Wait = func() error { return <-wait }
	result.Stop = func() { server.Close() }
	go func() {
		if err := server.Serve(listener); err != http.ErrServerClosed {
			wait <- err
		} else {
			wait <- nil
		}
	}()

	// Start the first build shortly after this function returns (but not
	// immediately so that stuff we print right after this will come first)
	go func() {
		time.Sleep(10 * time.Millisecond)
		handler.build()
	}()
	return result, nil
}
