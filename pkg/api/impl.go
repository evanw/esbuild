package api

import (
	"fmt"
	"strings"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/bundler"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/lexer"
	"github.com/evanw/esbuild/internal/logging"
	"github.com/evanw/esbuild/internal/parser"
	"github.com/evanw/esbuild/internal/printer"
	"github.com/evanw/esbuild/internal/resolver"
)

func validatePlatform(value Platform) parser.Platform {
	switch value {
	case PlatformBrowser:
		return parser.PlatformBrowser
	case PlatformNode:
		return parser.PlatformNode
	default:
		panic("Invalid platform")
		return ^parser.Platform(0)
	}
}

func validateFormat(value Format) printer.Format {
	switch value {
	case FormatDefault:
		return printer.FormatPreserve
	case FormatIIFE:
		return printer.FormatIIFE
	case FormatCommonJS:
		return printer.FormatCommonJS
	case FormatESModule:
		return printer.FormatESModule
	default:
		panic("Invalid format")
		return ^printer.Format(0)
	}
}

func validateSourceMap(value SourceMap) bundler.SourceMap {
	switch value {
	case SourceMapNone:
		return bundler.SourceMapNone
	case SourceMapLinked:
		return bundler.SourceMapLinkedWithComment
	case SourceMapInline:
		return bundler.SourceMapInline
	case SourceMapExternal:
		return bundler.SourceMapExternalWithoutComment
	default:
		panic("Invalid source map")
		return ^bundler.SourceMap(0)
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
		return ^logging.StderrColor(0)
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
		return ^logging.LogLevel(0)
	}
}

func validateTarget(value Target) parser.LanguageTarget {
	switch value {
	case ESNext:
		return parser.ESNext
	case ES2015:
		return parser.ES2015
	case ES2016:
		return parser.ES2016
	case ES2017:
		return parser.ES2017
	case ES2018:
		return parser.ES2018
	case ES2019:
		return parser.ES2019
	case ES2020:
		return parser.ES2020
	default:
		panic("Invalid target")
		return ^parser.LanguageTarget(0)
	}
}

func validateLoader(value Loader) bundler.Loader {
	switch value {
	case LoaderJS:
		return bundler.LoaderJS
	case LoaderJSX:
		return bundler.LoaderJSX
	case LoaderTS:
		return bundler.LoaderTS
	case LoaderTSX:
		return bundler.LoaderTSX
	case LoaderJSON:
		return bundler.LoaderJSON
	case LoaderText:
		return bundler.LoaderText
	case LoaderBase64:
		return bundler.LoaderBase64
	case LoaderDataURL:
		return bundler.LoaderDataURL
	case LoaderFile:
		return bundler.LoaderFile
	default:
		panic("Invalid loader")
		return ^bundler.Loader(0)
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

func validateLoaders(log logging.Log, loaders map[string]Loader) map[string]bundler.Loader {
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

func validateDefines(log logging.Log, defines map[string]string) *parser.ProcessedDefines {
	if len(defines) == 0 {
		return nil
	}

	rawDefines := make(map[string]parser.DefineFunc)

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
				rawDefines[key] = func(findSymbol parser.FindSymbol) ast.E {
					return &ast.EIdentifier{findSymbol(name)}
				}
				continue
			}
		}

		// Parse the value as JSON
		valueLog, done := logging.NewDeferLog()
		source := logging.Source{Contents: value}
		expr, ok := parser.ParseJSON(valueLog, source, parser.ParseJSONOptions{})
		done()
		if !ok {
			log.AddError(nil, ast.Loc{}, fmt.Sprintf("Invalid define value: %q", value))
			continue
		}

		// Only allow atoms for now
		var fn parser.DefineFunc
		switch e := expr.Data.(type) {
		case *ast.ENull:
			fn = func(parser.FindSymbol) ast.E { return &ast.ENull{} }
		case *ast.EBoolean:
			fn = func(parser.FindSymbol) ast.E { return &ast.EBoolean{e.Value} }
		case *ast.EString:
			fn = func(parser.FindSymbol) ast.E { return &ast.EString{e.Value} }
		case *ast.ENumber:
			fn = func(parser.FindSymbol) ast.E { return &ast.ENumber{e.Value} }
		default:
			log.AddError(nil, ast.Loc{}, fmt.Sprintf("Invalid define value: %q", value))
			continue
		}

		rawDefines[key] = fn
	}

	// Processing defines is expensive. Process them once here so the same object
	// can be shared between all parsers we create using these arguments.
	processed := parser.ProcessDefines(rawDefines)
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
	newLog := func() (logging.Log, func() []logging.Msg) {
		if options.LogLevel == LogLevelSilent {
			return logging.NewDeferLog()
		}
		return logging.NewStderrLog(logging.StderrOptions{
			IncludeSource: true,
			ErrorLimit:    options.ErrorLimit,
			Color:         validateColor(options.Color),
			LogLevel:      validateLogLevel(options.LogLevel),
		})
	}

	// Convert and validate the options
	realFS := fs.RealFS()
	validateLog, validateJoin := newLog()
	parseOptions := parser.ParseOptions{
		Target:       validateTarget(options.Target),
		MangleSyntax: options.MinifySyntax,
		JSX: parser.JSXOptions{
			Factory:  validateJSX(validateLog, options.JSXFactory, "factory"),
			Fragment: validateJSX(validateLog, options.JSXFragment, "fragment"),
		},
		Defines:    validateDefines(validateLog, options.Defines),
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
		OutputFormat:      validateFormat(options.Format),
		AbsOutputFile:     validatePath(validateLog, realFS, options.Outfile),
		AbsOutputDir:      validatePath(validateLog, realFS, options.Outdir),
		AbsMetadataFile:   validatePath(validateLog, realFS, options.Metafile),
		ExtensionToLoader: validateLoaders(validateLog, options.Loaders),
	}
	resolveOptions := resolver.ResolveOptions{
		Platform:        validatePlatform(options.Platform),
		ExtensionOrder:  validateResolveExtensions(validateLog, options.ResolveExtensions),
		ExternalModules: validateExternals(validateLog, options.Externals),
	}
	entryPaths := make([]string, len(options.EntryPoints))
	for i, entryPoint := range options.EntryPoints {
		entryPaths[i] = validatePath(validateLog, realFS, entryPoint)
	}

	if bundleOptions.AbsOutputDir == "" && len(entryPaths) > 1 {
		validateLog.AddError(nil, ast.Loc{},
			"Must use \"outdir\" when there are multiple input files")
	} else if bundleOptions.AbsOutputFile != "" && bundleOptions.AbsOutputDir != "" {
		validateLog.AddError(nil, ast.Loc{}, "Cannot use both \"outfile\" and \"outdir\"")
	} else if bundleOptions.AbsOutputFile != "" {
		// If the output file is specified, use it to derive the output directory
		bundleOptions.AbsOutputDir = realFS.Dir(bundleOptions.AbsOutputFile)
	} else if bundleOptions.AbsOutputDir == "" {
		// Forbid certain features when writing to stdout
		if bundleOptions.SourceMap != bundler.SourceMapNone && bundleOptions.SourceMap != bundler.SourceMapInline {
			validateLog.AddError(nil, ast.Loc{}, "Cannot use an external source map without an output path")
		}
		if bundleOptions.AbsMetadataFile != "" {
			validateLog.AddError(nil, ast.Loc{}, "Cannot use \"metafile\" without an output path")
		}
		for _, loader := range bundleOptions.ExtensionToLoader {
			if loader == bundler.LoaderFile {
				validateLog.AddError(nil, ast.Loc{}, "Cannot use the \"file\" loader without an output path")
				break
			}
		}
	}

	if !bundleOptions.IsBundling {
		// Disallow bundle-only options when not bundling
		if bundleOptions.OutputFormat != printer.FormatPreserve {
			validateLog.AddError(nil, ast.Loc{}, "Cannot use \"format\" without \"bundle\"")
		}
		if len(resolveOptions.ExternalModules) > 0 {
			validateLog.AddError(nil, ast.Loc{}, "Cannot use \"external\" without \"bundle\"")
		}
	} else if bundleOptions.OutputFormat == printer.FormatPreserve {
		// If the format isn't specified, set the default format using the platform
		switch resolveOptions.Platform {
		case parser.PlatformBrowser:
			bundleOptions.OutputFormat = printer.FormatIIFE
		case parser.PlatformNode:
			bundleOptions.OutputFormat = printer.FormatCommonJS
		}
	}

	// Stop now if there were errors
	validateMsgs := validateJoin()
	validateErrors := messagesOfKind(logging.Error, validateMsgs)
	if len(validateErrors) > 0 {
		return BuildResult{
			Errors:   validateErrors,
			Warnings: messagesOfKind(logging.Warning, validateMsgs),
		}
	}

	// Scan over the bundle
	scanLog, scanJoin := newLog()
	resolver := resolver.NewResolver(realFS, scanLog, resolveOptions)
	bundle := bundler.ScanBundle(scanLog, realFS, resolver, entryPaths, parseOptions, bundleOptions)

	// Stop now if there were errors
	scanMsgs := scanJoin()
	scanErrors := messagesOfKind(logging.Error, scanMsgs)
	if len(scanErrors) > 0 {
		return BuildResult{
			Errors: scanErrors,
			Warnings: append(
				messagesOfKind(logging.Warning, validateMsgs),
				messagesOfKind(logging.Warning, scanMsgs)...),
		}
	}

	// Compile the bundle
	compileLog, compileJoin := newLog()
	results := bundle.Compile(compileLog, bundleOptions)

	// Return the results
	compileMsgs := compileJoin()
	outputFiles := make([]OutputFile, len(results))
	for i, result := range results {
		outputFiles[i] = OutputFile{
			Path:     result.AbsPath,
			Contents: result.Contents,
		}
	}
	return BuildResult{
		Errors: messagesOfKind(logging.Error, compileMsgs),
		Warnings: append(append(
			messagesOfKind(logging.Warning, validateMsgs),
			messagesOfKind(logging.Warning, scanMsgs)...),
			messagesOfKind(logging.Warning, compileMsgs)...),
		OutputFiles: outputFiles,
	}
}

////////////////////////////////////////////////////////////////////////////////
// Transform API

func transformImpl(input string, options TransformOptions) TransformResult {
	newLog := func() (logging.Log, func() []logging.Msg) {
		if options.LogLevel == LogLevelSilent {
			return logging.NewDeferLog()
		}
		return logging.NewStderrLog(logging.StderrOptions{
			IncludeSource: true,
			ErrorLimit:    options.ErrorLimit,
			Color:         validateColor(options.Color),
			LogLevel:      validateLogLevel(options.LogLevel),
		})
	}

	// Convert and validate the options
	validateLog, validateJoin := newLog()
	parseOptions := parser.ParseOptions{
		Target:       validateTarget(options.Target),
		MangleSyntax: options.MinifySyntax,
		JSX: parser.JSXOptions{
			Factory:  validateJSX(validateLog, options.JSXFactory, "factory"),
			Fragment: validateJSX(validateLog, options.JSXFragment, "fragment"),
		},
		Defines: validateDefines(validateLog, options.Defines),
	}
	bundleOptions := bundler.BundleOptions{
		SourceMap:         validateSourceMap(options.Sourcemap),
		MangleSyntax:      options.MinifySyntax,
		RemoveWhitespace:  options.MinifyWhitespace,
		MinifyIdentifiers: options.MinifyIdentifiers,
		Stdin: &bundler.StdinInfo{
			Loader:     validateLoader(options.Loader),
			Contents:   input,
			SourceFile: options.Sourcefile,
		},
	}
	if bundleOptions.SourceMap == bundler.SourceMapLinkedWithComment {
		// Linked source maps don't make sense because there's no output file name
		validateLog.AddError(nil, ast.Loc{}, "Cannot transform with linked source maps")
	}
	if bundleOptions.SourceMap != bundler.SourceMapNone && bundleOptions.Stdin.SourceFile == "" {
		validateLog.AddError(nil, ast.Loc{},
			"Must use \"sourcefile\" with \"sourcemap\" to set the original file name")
	}

	// Stop now if there were errors
	validateMsgs := validateJoin()
	validateErrors := messagesOfKind(logging.Error, validateMsgs)
	if len(validateErrors) > 0 {
		return TransformResult{
			Errors:   validateErrors,
			Warnings: messagesOfKind(logging.Warning, validateMsgs),
		}
	}

	// Scan over the bundle
	scanLog, scanJoin := newLog()
	mockFS := fs.MockFS(map[string]string{options.Sourcefile: input})
	resolver := resolver.NewResolver(mockFS, scanLog, resolver.ResolveOptions{})
	bundle := bundler.ScanBundle(scanLog, mockFS, resolver, []string{options.Sourcefile}, parseOptions, bundleOptions)

	// Stop now if there were errors
	scanMsgs := scanJoin()
	scanErrors := messagesOfKind(logging.Error, scanMsgs)
	if len(scanErrors) > 0 {
		return TransformResult{
			Errors: scanErrors,
			Warnings: append(
				messagesOfKind(logging.Warning, validateMsgs),
				messagesOfKind(logging.Warning, scanMsgs)...),
		}
	}

	// Compile the bundle
	compileLog, compileJoin := newLog()
	results := bundle.Compile(compileLog, bundleOptions)

	// Return the results
	compileMsgs := compileJoin()
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

	return TransformResult{
		Errors: messagesOfKind(logging.Error, compileMsgs),
		Warnings: append(append(
			messagesOfKind(logging.Warning, validateMsgs),
			messagesOfKind(logging.Warning, scanMsgs)...),
			messagesOfKind(logging.Warning, compileMsgs)...),
		JS:          js,
		JSSourceMap: jsSourceMap,
	}
}
