package api

import (
	"fmt"
	"strings"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/bundler"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/lexer"
	"github.com/evanw/esbuild/internal/logging"
	"github.com/evanw/esbuild/internal/parser"
	"github.com/evanw/esbuild/internal/resolver"
)

func validatePlatform(value Platform) config.Platform {
	switch value {
	case PlatformBrowser:
		return config.PlatformBrowser
	case PlatformNode:
		return config.PlatformNode
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
	default:
		panic("Invalid source map")
	}
}

func validateColor(value StderrColor) logging.StderrColor {
	switch value {
	case ColorIfTerminal:
		return logging.ColorIfTerminal
	case ColorNever:
		return logging.ColorNever
	case ColorAlways:
		return logging.ColorAlways
	default:
		panic("Invalid color")
	}
}

func validateLogLevel(value LogLevel) logging.LogLevel {
	switch value {
	case LogLevelInfo:
		return logging.LevelInfo
	case LogLevelWarning:
		return logging.LevelWarning
	case LogLevelError:
		return logging.LevelError
	default:
		panic("Invalid log level")
	}
}

func validateTarget(value Target) config.LanguageTarget {
	switch value {
	case ESNext:
		return config.ESNext
	case ES2015:
		return config.ES2015
	case ES2016:
		return config.ES2016
	case ES2017:
		return config.ES2017
	case ES2018:
		return config.ES2018
	case ES2019:
		return config.ES2019
	case ES2020:
		return config.ES2020
	default:
		panic("Invalid target")
	}
}

func validateStrict(value StrictOptions) config.StrictOptions {
	return config.StrictOptions{
		NullishCoalescing: value.NullishCoalescing,
		ClassFields:       value.ClassFields,
	}
}

func validateLoader(value Loader) config.Loader {
	switch value {
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
	default:
		panic("Invalid loader")
	}
}

func validateExternals(log logging.Log, paths []string) map[string]bool {
	result := make(map[string]bool)
	for _, path := range paths {
		if resolver.IsNonModulePath(path) {
			log.AddError(nil, ast.Loc{}, fmt.Sprintf("Invalid module name: %q", path))
		}
		result[path] = true
	}
	return result
}

func validateResolveExtensions(log logging.Log, order []string) []string {
	if order == nil {
		return []string{".tsx", ".ts", ".jsx", ".mjs", ".cjs", ".js", ".json"}
	}
	for _, ext := range order {
		if len(ext) < 2 || ext[0] != '.' {
			log.AddError(nil, ast.Loc{}, fmt.Sprintf("Invalid file extension: %q", ext))
		}
	}
	return order
}

func validateLoaders(log logging.Log, loaders map[string]Loader) map[string]config.Loader {
	result := bundler.DefaultExtensionToLoaderMap()
	if loaders != nil {
		for ext, loader := range loaders {
			if len(ext) < 2 || ext[0] != '.' || strings.ContainsRune(ext[1:], '.') {
				log.AddError(nil, ast.Loc{}, fmt.Sprintf("Invalid file extension: %q", ext))
			}
			result[ext] = validateLoader(loader)
		}
	}
	return result
}

func validateJSX(log logging.Log, text string, name string) []string {
	if text == "" {
		return nil
	}
	parts := strings.Split(text, ".")
	for _, part := range parts {
		if !lexer.IsIdentifier(part) {
			log.AddError(nil, ast.Loc{}, fmt.Sprintf("Invalid JSX %s: %q", name, text))
			return nil
		}
	}
	return parts
}

func validateDefines(log logging.Log, defines map[string]string) *config.ProcessedDefines {
	if len(defines) == 0 {
		return nil
	}

	rawDefines := make(map[string]config.DefineFunc)

	for key, value := range defines {
		// The key must be a dot-separated identifier list
		for _, part := range strings.Split(key, ".") {
			if !lexer.IsIdentifier(part) {
				log.AddError(nil, ast.Loc{}, fmt.Sprintf("Invalid define key: %q", key))
				continue
			}
		}

		// Allow substituting for an identifier
		if lexer.IsIdentifier(value) {
			if _, ok := lexer.Keywords()[value]; !ok {
				name := value // The closure must close over a variable inside the loop
				rawDefines[key] = func(findSymbol config.FindSymbol) ast.E {
					return &ast.EIdentifier{Ref: findSymbol(name)}
				}
				continue
			}
		}

		// Parse the value as JSON
		source := logging.Source{Contents: value}
		expr, ok := parser.ParseJSON(logging.NewDeferLog(), source, parser.ParseJSONOptions{})
		if !ok {
			log.AddError(nil, ast.Loc{}, fmt.Sprintf("Invalid define value: %q", value))
			continue
		}

		// Only allow atoms for now
		var fn config.DefineFunc
		switch e := expr.Data.(type) {
		case *ast.ENull:
			fn = func(config.FindSymbol) ast.E { return &ast.ENull{} }
		case *ast.EBoolean:
			fn = func(config.FindSymbol) ast.E { return &ast.EBoolean{Value: e.Value} }
		case *ast.EString:
			fn = func(config.FindSymbol) ast.E { return &ast.EString{Value: e.Value} }
		case *ast.ENumber:
			fn = func(config.FindSymbol) ast.E { return &ast.ENumber{Value: e.Value} }
		default:
			log.AddError(nil, ast.Loc{}, fmt.Sprintf("Invalid define value: %q", value))
			continue
		}

		rawDefines[key] = fn
	}

	// Processing defines is expensive. Process them once here so the same object
	// can be shared between all parsers we create using these arguments.
	processed := config.ProcessDefines(rawDefines)
	return &processed
}

func validatePath(log logging.Log, fs fs.FS, relPath string) string {
	if relPath == "" {
		return ""
	}
	absPath, ok := fs.Abs(relPath)
	if !ok {
		log.AddError(nil, ast.Loc{}, fmt.Sprintf("Invalid path: %s", relPath))
	}
	return absPath
}

func messagesOfKind(kind logging.MsgKind, msgs []logging.Msg) []Message {
	var filtered []Message
	for _, msg := range msgs {
		if msg.Kind == kind {
			var location *Location

			if msg.Source != nil {
				line, column, _ := logging.ComputeLineAndColumn(msg.Source.Contents[0:msg.Start])
				line++

				// Extract the line text
				lineText := msg.Source.Contents[int(msg.Start)-column:]
				endOfLine := len(lineText)
				for i, c := range lineText {
					if c == '\n' || c == '\r' || c == '\u2028' || c == '\u2029' {
						endOfLine = i
						break
					}
				}
				lineText = lineText[:endOfLine]

				location = &Location{
					File:     msg.Source.PrettyPath,
					Line:     line,
					Column:   column,
					Length:   int(msg.Length),
					LineText: lineText,
				}
			}

			filtered = append(filtered, Message{
				Text:     msg.Text,
				Location: location,
			})
		}
	}
	return filtered
}

////////////////////////////////////////////////////////////////////////////////
// Build API

func buildImpl(options BuildOptions) BuildResult {
	var log logging.Log
	if options.LogLevel == LogLevelSilent {
		log = logging.NewDeferLog()
	} else {
		log = logging.NewStderrLog(logging.StderrOptions{
			IncludeSource: true,
			ErrorLimit:    options.ErrorLimit,
			Color:         validateColor(options.Color),
			LogLevel:      validateLogLevel(options.LogLevel),
		})
	}

	// Convert and validate the options
	realFS := fs.RealFS()
	parseOptions := parser.ParseOptions{
		Target:       validateTarget(options.Target),
		Strict:       validateStrict(options.Strict),
		MangleSyntax: options.MinifySyntax,
		JSX: config.JSXOptions{
			Factory:  validateJSX(log, options.JSXFactory, "factory"),
			Fragment: validateJSX(log, options.JSXFragment, "fragment"),
		},
		Defines:    validateDefines(log, options.Defines),
		Platform:   validatePlatform(options.Platform),
		IsBundling: options.Bundle,
	}
	bundleOptions := bundler.BundleOptions{
		SourceMap:         validateSourceMap(options.Sourcemap),
		MangleSyntax:      options.MinifySyntax,
		RemoveWhitespace:  options.MinifyWhitespace,
		MinifyIdentifiers: options.MinifyIdentifiers,
		ModuleName:        options.GlobalName,
		IsBundling:        options.Bundle,
		CodeSplitting:     options.Splitting,
		OutputFormat:      validateFormat(options.Format),
		AbsOutputFile:     validatePath(log, realFS, options.Outfile),
		AbsOutputDir:      validatePath(log, realFS, options.Outdir),
		AbsMetadataFile:   validatePath(log, realFS, options.Metafile),
		ExtensionToLoader: validateLoaders(log, options.Loaders),
	}
	resolveOptions := resolver.ResolveOptions{
		Platform:        validatePlatform(options.Platform),
		ExtensionOrder:  validateResolveExtensions(log, options.ResolveExtensions),
		ExternalModules: validateExternals(log, options.Externals),
	}
	entryPaths := make([]string, len(options.EntryPoints))
	for i, entryPoint := range options.EntryPoints {
		entryPaths[i] = validatePath(log, realFS, entryPoint)
	}

	if bundleOptions.AbsOutputDir == "" && len(entryPaths) > 1 {
		log.AddError(nil, ast.Loc{},
			"Must use \"outdir\" when there are multiple input files")
	} else if bundleOptions.AbsOutputDir == "" && bundleOptions.CodeSplitting {
		log.AddError(nil, ast.Loc{},
			"Must use \"outdir\" when code splitting is enabled")
	} else if bundleOptions.AbsOutputFile != "" && bundleOptions.AbsOutputDir != "" {
		log.AddError(nil, ast.Loc{}, "Cannot use both \"outfile\" and \"outdir\"")
	} else if bundleOptions.AbsOutputFile != "" {
		// If the output file is specified, use it to derive the output directory
		bundleOptions.AbsOutputDir = realFS.Dir(bundleOptions.AbsOutputFile)
	} else if bundleOptions.AbsOutputDir == "" {
		// Forbid certain features when writing to stdout
		if bundleOptions.SourceMap != config.SourceMapNone && bundleOptions.SourceMap != config.SourceMapInline {
			log.AddError(nil, ast.Loc{}, "Cannot use an external source map without an output path")
		}
		if bundleOptions.AbsMetadataFile != "" {
			log.AddError(nil, ast.Loc{}, "Cannot use \"metafile\" without an output path")
		}
		for _, loader := range bundleOptions.ExtensionToLoader {
			if loader == config.LoaderFile {
				log.AddError(nil, ast.Loc{}, "Cannot use the \"file\" loader without an output path")
				break
			}
		}
	}

	if !bundleOptions.IsBundling {
		// Disallow bundle-only options when not bundling
		if bundleOptions.OutputFormat != config.FormatPreserve {
			log.AddError(nil, ast.Loc{}, "Cannot use \"format\" without \"bundle\"")
		}
		if len(resolveOptions.ExternalModules) > 0 {
			log.AddError(nil, ast.Loc{}, "Cannot use \"external\" without \"bundle\"")
		}
	} else if bundleOptions.OutputFormat == config.FormatPreserve {
		// If the format isn't specified, set the default format using the platform
		switch resolveOptions.Platform {
		case config.PlatformBrowser:
			bundleOptions.OutputFormat = config.FormatIIFE
		case config.PlatformNode:
			bundleOptions.OutputFormat = config.FormatCommonJS
		}
	}

	// Code splitting is experimental and currently only enabled for ES6 modules
	if bundleOptions.CodeSplitting && bundleOptions.OutputFormat != config.FormatESModule {
		log.AddError(nil, ast.Loc{}, "Spltting currently only works with the \"esm\" format")
	}

	var outputFiles []OutputFile

	// Stop now if there were errors
	if !log.HasErrors() {
		// Scan over the bundle
		resolver := resolver.NewResolver(realFS, log, resolveOptions)
		bundle := bundler.ScanBundle(log, realFS, resolver, entryPaths, parseOptions, bundleOptions)

		// Stop now if there were errors
		if !log.HasErrors() {
			// Compile the bundle
			results := bundle.Compile(log, bundleOptions)

			// Return the results
			outputFiles = make([]OutputFile, len(results))
			for i, result := range results {
				outputFiles[i] = OutputFile{
					Path:     result.AbsPath,
					Contents: result.Contents,
				}
			}
		}
	}

	msgs := log.Done()
	return BuildResult{
		Errors:      messagesOfKind(logging.Error, msgs),
		Warnings:    messagesOfKind(logging.Warning, msgs),
		OutputFiles: outputFiles,
	}
}

////////////////////////////////////////////////////////////////////////////////
// Transform API

func transformImpl(input string, options TransformOptions) TransformResult {
	var log logging.Log
	if options.LogLevel == LogLevelSilent {
		log = logging.NewDeferLog()
	} else {
		log = logging.NewStderrLog(logging.StderrOptions{
			IncludeSource: true,
			ErrorLimit:    options.ErrorLimit,
			Color:         validateColor(options.Color),
			LogLevel:      validateLogLevel(options.LogLevel),
		})
	}

	// Convert and validate the options
	parseOptions := parser.ParseOptions{
		Target:       validateTarget(options.Target),
		Strict:       validateStrict(options.Strict),
		MangleSyntax: options.MinifySyntax,
		JSX: config.JSXOptions{
			Factory:  validateJSX(log, options.JSXFactory, "factory"),
			Fragment: validateJSX(log, options.JSXFragment, "fragment"),
		},
		Defines: validateDefines(log, options.Defines),
	}
	bundleOptions := bundler.BundleOptions{
		SourceMap:         validateSourceMap(options.Sourcemap),
		MangleSyntax:      options.MinifySyntax,
		RemoveWhitespace:  options.MinifyWhitespace,
		MinifyIdentifiers: options.MinifyIdentifiers,
		AbsOutputFile:     options.Sourcefile + "-out",
		Stdin: &bundler.StdinInfo{
			Loader:     validateLoader(options.Loader),
			Contents:   input,
			SourceFile: options.Sourcefile,
		},
	}
	if bundleOptions.SourceMap == config.SourceMapLinkedWithComment {
		// Linked source maps don't make sense because there's no output file name
		log.AddError(nil, ast.Loc{}, "Cannot transform with linked source maps")
	}
	if bundleOptions.SourceMap != config.SourceMapNone && bundleOptions.Stdin.SourceFile == "" {
		log.AddError(nil, ast.Loc{},
			"Must use \"sourcefile\" with \"sourcemap\" to set the original file name")
	}

	var results []bundler.OutputFile

	// Stop now if there were errors
	if !log.HasErrors() {
		// Scan over the bundle
		mockFS := fs.MockFS(map[string]string{options.Sourcefile: input})
		resolver := resolver.NewResolver(mockFS, log, resolver.ResolveOptions{})
		bundle := bundler.ScanBundle(log, mockFS, resolver, []string{options.Sourcefile}, parseOptions, bundleOptions)

		// Stop now if there were errors
		if !log.HasErrors() {
			// Compile the bundle
			results = bundle.Compile(log, bundleOptions)
		}
	}

	// Return the results
	var js []byte
	var jsSourceMap []byte

	// Unpack the JavaScript file and the source map file
	if len(results) == 1 {
		js = results[0].Contents
	} else if len(results) == 2 {
		a, b := results[0], results[1]
		if a.AbsPath == b.AbsPath+".map" {
			jsSourceMap, js = a.Contents, b.Contents
		} else if a.AbsPath+".map" == b.AbsPath {
			js, jsSourceMap = a.Contents, b.Contents
		}
	}

	msgs := log.Done()
	return TransformResult{
		Errors:      messagesOfKind(logging.Error, msgs),
		Warnings:    messagesOfKind(logging.Warning, msgs),
		JS:          js,
		JSSourceMap: jsSourceMap,
	}
}
