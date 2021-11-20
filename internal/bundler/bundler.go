package bundler

import (
	"bytes"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net/http"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/cache"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/css_parser"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/graph"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/js_parser"
	"github.com/evanw/esbuild/internal/js_printer"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/resolver"
	"github.com/evanw/esbuild/internal/runtime"
	"github.com/evanw/esbuild/internal/sourcemap"
	"github.com/evanw/esbuild/internal/xxhash"
)

type scannerFile struct {
	inputFile  graph.InputFile
	pluginData interface{}

	// If "AbsMetadataFile" is present, this will be filled out with information
	// about this file in JSON format. This is a partial JSON file that will be
	// fully assembled later.
	jsonMetadataChunk string
}

// This is data related to source maps. It's computed in parallel with linking
// and must be ready by the time printing happens. This is beneficial because
// it is somewhat expensive to produce.
type dataForSourceMap struct {
	// This data is for the printer. It maps from byte offsets in the file (which
	// are stored at every AST node) to UTF-16 column offsets (required by source
	// maps).
	lineOffsetTables []sourcemap.LineOffsetTable

	// This contains the quoted contents of the original source file. It's what
	// needs to be embedded in the "sourcesContent" array in the final source
	// map. Quoting is precomputed because it's somewhat expensive.
	quotedContents [][]byte
}

type Bundle struct {
	fs          fs.FS
	res         resolver.Resolver
	files       []scannerFile
	entryPoints []graph.EntryPoint

	// The unique key prefix is a random string that is unique to every bundling
	// operation. It is used as a prefix for the unique keys assigned to every
	// chunk during linking. These unique keys are used to identify each chunk
	// before the final output paths have been computed.
	uniqueKeyPrefix string
}

type parseArgs struct {
	fs              fs.FS
	log             logger.Log
	res             resolver.Resolver
	caches          *cache.CacheSet
	keyPath         logger.Path
	prettyPath      string
	sourceIndex     uint32
	importSource    *logger.Source
	sideEffects     graph.SideEffects
	importPathRange logger.Range
	pluginData      interface{}
	options         config.Options
	results         chan parseResult
	inject          chan config.InjectedFile
	skipResolve     bool
	uniqueKeyPrefix string
}

type parseResult struct {
	file           scannerFile
	resolveResults []*resolver.ResolveResult
	tlaCheck       tlaCheck
	ok             bool
}

type tlaCheck struct {
	parent            ast.Index32
	depth             uint32
	importRecordIndex uint32
}

func parseFile(args parseArgs) {
	source := logger.Source{
		Index:          args.sourceIndex,
		KeyPath:        args.keyPath,
		PrettyPath:     args.prettyPath,
		IdentifierName: js_ast.GenerateNonUniqueNameFromPath(args.keyPath.Text),
	}

	var loader config.Loader
	var absResolveDir string
	var pluginName string
	var pluginData interface{}

	if stdin := args.options.Stdin; stdin != nil {
		// Special-case stdin
		source.Contents = stdin.Contents
		loader = stdin.Loader
		if loader == config.LoaderNone {
			loader = config.LoaderJS
		}
		absResolveDir = args.options.Stdin.AbsResolveDir
	} else {
		result, ok := runOnLoadPlugins(
			args.options.Plugins,
			args.res,
			args.fs,
			&args.caches.FSCache,
			args.log,
			&source,
			args.importSource,
			args.importPathRange,
			args.pluginData,
			args.options.WatchMode,
		)
		if !ok {
			if args.inject != nil {
				args.inject <- config.InjectedFile{
					Source: source,
				}
			}
			args.results <- parseResult{}
			return
		}
		loader = result.loader
		absResolveDir = result.absResolveDir
		pluginName = result.pluginName
		pluginData = result.pluginData
	}

	_, base, ext := logger.PlatformIndependentPathDirBaseExt(source.KeyPath.Text)

	// The special "default" loader determines the loader from the file path
	if loader == config.LoaderDefault {
		loader = loaderFromFileExtension(args.options.ExtensionToLoader, base+ext)
	}

	result := parseResult{
		file: scannerFile{
			inputFile: graph.InputFile{
				Source:      source,
				Loader:      loader,
				SideEffects: args.sideEffects,
			},
			pluginData: pluginData,
		},
	}

	defer func() {
		r := recover()
		if r != nil {
			stack := strings.TrimSpace(string(debug.Stack()))
			tracker := logger.MakeLineColumnTracker(&source)
			data := logger.RangeData(&tracker, logger.Range{}, fmt.Sprintf("panic: %v", r))
			data.Location.LineText = fmt.Sprintf("%s\n%s", data.Location.LineText, stack)
			args.log.AddMsg(logger.Msg{
				Kind: logger.Error,
				Data: data,
			})
			args.results <- result
		}
	}()

	switch loader {
	case config.LoaderJS:
		ast, ok := args.caches.JSCache.Parse(args.log, source, js_parser.OptionsFromConfig(&args.options))
		if len(ast.Parts) <= 1 { // Ignore the implicitly-generated namespace export part
			result.file.inputFile.SideEffects.Kind = graph.NoSideEffects_EmptyAST
		}
		result.file.inputFile.Repr = &graph.JSRepr{AST: ast}
		result.ok = ok

	case config.LoaderJSX:
		args.options.JSX.Parse = true
		ast, ok := args.caches.JSCache.Parse(args.log, source, js_parser.OptionsFromConfig(&args.options))
		if len(ast.Parts) <= 1 { // Ignore the implicitly-generated namespace export part
			result.file.inputFile.SideEffects.Kind = graph.NoSideEffects_EmptyAST
		}
		result.file.inputFile.Repr = &graph.JSRepr{AST: ast}
		result.ok = ok

	case config.LoaderTS, config.LoaderTSNoAmbiguousLessThan:
		args.options.TS.Parse = true
		args.options.TS.NoAmbiguousLessThan = loader == config.LoaderTSNoAmbiguousLessThan
		ast, ok := args.caches.JSCache.Parse(args.log, source, js_parser.OptionsFromConfig(&args.options))
		if len(ast.Parts) <= 1 { // Ignore the implicitly-generated namespace export part
			result.file.inputFile.SideEffects.Kind = graph.NoSideEffects_EmptyAST
		}
		result.file.inputFile.Repr = &graph.JSRepr{AST: ast}
		result.ok = ok

	case config.LoaderTSX:
		args.options.TS.Parse = true
		args.options.JSX.Parse = true
		ast, ok := args.caches.JSCache.Parse(args.log, source, js_parser.OptionsFromConfig(&args.options))
		if len(ast.Parts) <= 1 { // Ignore the implicitly-generated namespace export part
			result.file.inputFile.SideEffects.Kind = graph.NoSideEffects_EmptyAST
		}
		result.file.inputFile.Repr = &graph.JSRepr{AST: ast}
		result.ok = ok

	case config.LoaderCSS:
		ast := args.caches.CSSCache.Parse(args.log, source, css_parser.Options{
			MangleSyntax:           args.options.MangleSyntax,
			RemoveWhitespace:       args.options.RemoveWhitespace,
			UnsupportedCSSFeatures: args.options.UnsupportedCSSFeatures,
		})
		result.file.inputFile.Repr = &graph.CSSRepr{AST: ast}
		result.ok = true

	case config.LoaderJSON:
		expr, ok := args.caches.JSONCache.Parse(args.log, source, js_parser.JSONOptions{})
		ast := js_parser.LazyExportAST(args.log, source, js_parser.OptionsFromConfig(&args.options), expr, "")
		if pluginName != "" {
			result.file.inputFile.SideEffects.Kind = graph.NoSideEffects_PureData_FromPlugin
		} else {
			result.file.inputFile.SideEffects.Kind = graph.NoSideEffects_PureData
		}
		result.file.inputFile.Repr = &graph.JSRepr{AST: ast}
		result.ok = ok

	case config.LoaderText:
		encoded := base64.StdEncoding.EncodeToString([]byte(source.Contents))
		expr := js_ast.Expr{Data: &js_ast.EString{Value: js_lexer.StringToUTF16(source.Contents)}}
		ast := js_parser.LazyExportAST(args.log, source, js_parser.OptionsFromConfig(&args.options), expr, "")
		ast.URLForCSS = "data:text/plain;base64," + encoded
		if pluginName != "" {
			result.file.inputFile.SideEffects.Kind = graph.NoSideEffects_PureData_FromPlugin
		} else {
			result.file.inputFile.SideEffects.Kind = graph.NoSideEffects_PureData
		}
		result.file.inputFile.Repr = &graph.JSRepr{AST: ast}
		result.ok = true

	case config.LoaderBase64:
		mimeType := guessMimeType(ext, source.Contents)
		encoded := base64.StdEncoding.EncodeToString([]byte(source.Contents))
		expr := js_ast.Expr{Data: &js_ast.EString{Value: js_lexer.StringToUTF16(encoded)}}
		ast := js_parser.LazyExportAST(args.log, source, js_parser.OptionsFromConfig(&args.options), expr, "")
		ast.URLForCSS = "data:" + mimeType + ";base64," + encoded
		if pluginName != "" {
			result.file.inputFile.SideEffects.Kind = graph.NoSideEffects_PureData_FromPlugin
		} else {
			result.file.inputFile.SideEffects.Kind = graph.NoSideEffects_PureData
		}
		result.file.inputFile.Repr = &graph.JSRepr{AST: ast}
		result.ok = true

	case config.LoaderBinary:
		encoded := base64.StdEncoding.EncodeToString([]byte(source.Contents))
		expr := js_ast.Expr{Data: &js_ast.EString{Value: js_lexer.StringToUTF16(encoded)}}
		helper := "__toBinary"
		if args.options.Platform == config.PlatformNode {
			helper = "__toBinaryNode"
		}
		ast := js_parser.LazyExportAST(args.log, source, js_parser.OptionsFromConfig(&args.options), expr, helper)
		ast.URLForCSS = "data:application/octet-stream;base64," + encoded
		if pluginName != "" {
			result.file.inputFile.SideEffects.Kind = graph.NoSideEffects_PureData_FromPlugin
		} else {
			result.file.inputFile.SideEffects.Kind = graph.NoSideEffects_PureData
		}
		result.file.inputFile.Repr = &graph.JSRepr{AST: ast}
		result.ok = true

	case config.LoaderDataURL:
		mimeType := guessMimeType(ext, source.Contents)
		encoded := base64.StdEncoding.EncodeToString([]byte(source.Contents))
		url := fmt.Sprintf("data:%s;base64,%s", mimeType, encoded)
		expr := js_ast.Expr{Data: &js_ast.EString{Value: js_lexer.StringToUTF16(url)}}
		ast := js_parser.LazyExportAST(args.log, source, js_parser.OptionsFromConfig(&args.options), expr, "")
		ast.URLForCSS = url
		if pluginName != "" {
			result.file.inputFile.SideEffects.Kind = graph.NoSideEffects_PureData_FromPlugin
		} else {
			result.file.inputFile.SideEffects.Kind = graph.NoSideEffects_PureData
		}
		result.file.inputFile.Repr = &graph.JSRepr{AST: ast}
		result.ok = true

	case config.LoaderFile:
		uniqueKey := fmt.Sprintf("%sA%08d", args.uniqueKeyPrefix, args.sourceIndex)
		uniqueKeyPath := uniqueKey + source.KeyPath.IgnoredSuffix
		expr := js_ast.Expr{Data: &js_ast.EString{Value: js_lexer.StringToUTF16(uniqueKeyPath)}}
		ast := js_parser.LazyExportAST(args.log, source, js_parser.OptionsFromConfig(&args.options), expr, "")
		ast.URLForCSS = uniqueKeyPath
		if pluginName != "" {
			result.file.inputFile.SideEffects.Kind = graph.NoSideEffects_PureData_FromPlugin
		} else {
			result.file.inputFile.SideEffects.Kind = graph.NoSideEffects_PureData
		}
		result.file.inputFile.Repr = &graph.JSRepr{AST: ast}
		result.ok = true

		// Mark that this file is from the "file" loader
		result.file.inputFile.UniqueKeyForFileLoader = uniqueKey

	default:
		var message string
		if source.KeyPath.Namespace == "file" && ext != "" {
			message = fmt.Sprintf("No loader is configured for %q files: %s", ext, source.PrettyPath)
		} else {
			message = fmt.Sprintf("Do not know how to load path: %s", source.PrettyPath)
		}
		tracker := logger.MakeLineColumnTracker(args.importSource)
		args.log.AddRangeError(&tracker, args.importPathRange, message)
	}

	// This must come before we send on the "results" channel to avoid deadlock
	if args.inject != nil {
		var exports []config.InjectableExport
		if repr, ok := result.file.inputFile.Repr.(*graph.JSRepr); ok {
			aliases := make([]string, 0, len(repr.AST.NamedExports))
			for alias := range repr.AST.NamedExports {
				aliases = append(aliases, alias)
			}
			sort.Strings(aliases) // Sort for determinism
			exports = make([]config.InjectableExport, len(aliases))
			for i, alias := range aliases {
				exports[i] = config.InjectableExport{
					Alias: alias,
					Loc:   repr.AST.NamedExports[alias].AliasLoc,
				}
			}
		}
		args.inject <- config.InjectedFile{
			Source:  source,
			Exports: exports,
		}
	}

	// Stop now if parsing failed
	if !result.ok {
		args.results <- result
		return
	}

	// Run the resolver on the parse thread so it's not run on the main thread.
	// That way the main thread isn't blocked if the resolver takes a while.
	if args.options.Mode == config.ModeBundle && !args.skipResolve {
		// Clone the import records because they will be mutated later
		recordsPtr := result.file.inputFile.Repr.ImportRecords()
		records := append([]ast.ImportRecord{}, *recordsPtr...)
		*recordsPtr = records
		result.resolveResults = make([]*resolver.ResolveResult, len(records))

		if len(records) > 0 {
			resolverCache := make(map[ast.ImportKind]map[string]*resolver.ResolveResult)
			tracker := logger.MakeLineColumnTracker(&source)

			for importRecordIndex := range records {
				// Don't try to resolve imports that are already resolved
				record := &records[importRecordIndex]
				if record.SourceIndex.IsValid() {
					continue
				}

				// Ignore records that the parser has discarded. This is used to remove
				// type-only imports in TypeScript files.
				if record.IsUnused {
					continue
				}

				// Cache the path in case it's imported multiple times in this file
				cache, ok := resolverCache[record.Kind]
				if !ok {
					cache = make(map[string]*resolver.ResolveResult)
					resolverCache[record.Kind] = cache
				}
				if resolveResult, ok := cache[record.Path.Text]; ok {
					result.resolveResults[importRecordIndex] = resolveResult
					continue
				}

				// Run the resolver and log an error if the path couldn't be resolved
				resolveResult, didLogError, debug := runOnResolvePlugins(
					args.options.Plugins,
					args.res,
					args.log,
					args.fs,
					&args.caches.FSCache,
					&source,
					record.Range,
					source.KeyPath.Namespace,
					record.Path.Text,
					record.Kind,
					absResolveDir,
					pluginData,
				)
				cache[record.Path.Text] = resolveResult

				// All "require.resolve()" imports should be external because we don't
				// want to waste effort traversing into them
				if record.Kind == ast.ImportRequireResolve {
					if !record.HandlesImportErrors && (resolveResult == nil || !resolveResult.IsExternal) {
						args.log.AddRangeWarning(&tracker, record.Range,
							fmt.Sprintf("%q should be marked as external for use with \"require.resolve\"", record.Path.Text))
					}
					continue
				}

				if resolveResult == nil {
					// Failed imports inside a try/catch are silently turned into
					// external imports instead of causing errors. This matches a common
					// code pattern for conditionally importing a module with a graceful
					// fallback.
					if !didLogError && !record.HandlesImportErrors {
						hint := ""
						if resolver.IsPackagePath(record.Path.Text) {
							if record.Kind == ast.ImportRequire {
								hint = ", or surround it with try/catch to handle the failure at run-time"
							} else if record.Kind == ast.ImportDynamic {
								hint = ", or add \".catch()\" to handle the failure at run-time"
							}
							hint = fmt.Sprintf(" (mark it as external to exclude it from the bundle%s)", hint)
							if pluginName == "" && !args.fs.IsAbs(record.Path.Text) {
								if query := args.res.ProbeResolvePackageAsRelative(absResolveDir, record.Path.Text, record.Kind); query != nil {
									hint = fmt.Sprintf(" (use %q to reference the file %q)", "./"+record.Path.Text, args.res.PrettyPath(query.PathPair.Primary))
								}
							}
						}
						if args.options.Platform != config.PlatformNode {
							if _, ok := resolver.BuiltInNodeModules[record.Path.Text]; ok {
								switch logger.API {
								case logger.CLIAPI:
									hint = " (use \"--platform=node\" when building for node)"
								case logger.JSAPI:
									hint = " (use \"platform: 'node'\" when building for node)"
								case logger.GoAPI:
									hint = " (use \"Platform: api.PlatformNode\" when building for node)"
								}
							}
						}
						if absResolveDir == "" && pluginName != "" {
							hint = fmt.Sprintf(" (the plugin %q didn't set a resolve directory)", pluginName)
						}
						debug.LogErrorMsg(args.log, &source, record.Range, fmt.Sprintf("Could not resolve %q%s", record.Path.Text, hint))
					} else if args.log.Level <= logger.LevelDebug && !didLogError && record.HandlesImportErrors {
						args.log.AddRangeDebug(&tracker, record.Range,
							fmt.Sprintf("Importing %q was allowed even though it could not be resolved because dynamic import failures appear to be handled here",
								record.Path.Text))
					}
					continue
				}

				result.resolveResults[importRecordIndex] = resolveResult
			}
		}
	}

	// Attempt to parse the source map if present
	if loader.CanHaveSourceMap() && args.options.SourceMap != config.SourceMapNone {
		var sourceMapComment logger.Span
		switch repr := result.file.inputFile.Repr.(type) {
		case *graph.JSRepr:
			sourceMapComment = repr.AST.SourceMapComment
		case *graph.CSSRepr:
			sourceMapComment = repr.AST.SourceMapComment
		}
		if sourceMapComment.Text != "" {
			if path, contents := extractSourceMapFromComment(args.log, args.fs, &args.caches.FSCache,
				args.res, &source, sourceMapComment, absResolveDir); contents != nil {
				result.file.inputFile.InputSourceMap = js_parser.ParseSourceMap(args.log, logger.Source{
					KeyPath:    path,
					PrettyPath: args.res.PrettyPath(path),
					Contents:   *contents,
				})
			}
		}
	}

	args.results <- result
}

func joinWithPublicPath(publicPath string, relPath string) string {
	if strings.HasPrefix(relPath, "./") {
		relPath = relPath[2:]

		// Strip any amount of further no-op slashes (i.e. ".///././/x/y" => "x/y")
		for {
			if strings.HasPrefix(relPath, "/") {
				relPath = relPath[1:]
			} else if strings.HasPrefix(relPath, "./") {
				relPath = relPath[2:]
			} else {
				break
			}
		}
	}

	// Use a relative path if there is no public path
	if publicPath == "" {
		publicPath = "."
	}

	// Join with a slash
	slash := "/"
	if strings.HasSuffix(publicPath, "/") {
		slash = ""
	}
	return fmt.Sprintf("%s%s%s", publicPath, slash, relPath)
}

func isASCIIOnly(text string) bool {
	for _, c := range text {
		if c < 0x20 || c > 0x7E {
			return false
		}
	}
	return true
}

func guessMimeType(extension string, contents string) string {
	mimeType := helpers.MimeTypeByExtension(extension)
	if mimeType == "" {
		mimeType = http.DetectContentType([]byte(contents))
	}

	// Turn "text/plain; charset=utf-8" into "text/plain;charset=utf-8"
	return strings.ReplaceAll(mimeType, "; ", ";")
}

func extractSourceMapFromComment(
	log logger.Log,
	fs fs.FS,
	fsCache *cache.FSCache,
	res resolver.Resolver,
	source *logger.Source,
	comment logger.Span,
	absResolveDir string,
) (logger.Path, *string) {
	tracker := logger.MakeLineColumnTracker(source)

	// Support data URLs
	if parsed, ok := resolver.ParseDataURL(comment.Text); ok {
		if contents, err := parsed.DecodeData(); err == nil {
			return logger.Path{Text: source.PrettyPath, IgnoredSuffix: "#sourceMappingURL"}, &contents
		} else {
			log.AddRangeWarning(&tracker, comment.Range, fmt.Sprintf("Unsupported source map comment: %s", err.Error()))
			return logger.Path{}, nil
		}
	}

	// Relative path in a file with an absolute path
	if absResolveDir != "" {
		absPath := fs.Join(absResolveDir, comment.Text)
		path := logger.Path{Text: absPath, Namespace: "file"}
		contents, err, originalError := fsCache.ReadFile(fs, absPath)
		if log.Level <= logger.LevelDebug && originalError != nil {
			log.AddRangeDebug(&tracker, comment.Range, fmt.Sprintf("Failed to read file %q: %s", res.PrettyPath(path), originalError.Error()))
		}
		if err != nil {
			if err == syscall.ENOENT {
				// Don't report a warning because this is likely unactionable
				return logger.Path{}, nil
			}
			log.AddRangeWarning(&tracker, comment.Range, fmt.Sprintf("Cannot read file %q: %s", res.PrettyPath(path), err.Error()))
			return logger.Path{}, nil
		}
		return path, &contents
	}

	// Anything else is unsupported
	return logger.Path{}, nil
}

func sanetizeLocation(res resolver.Resolver, loc *logger.MsgLocation) {
	if loc != nil {
		if loc.Namespace == "" {
			loc.Namespace = "file"
		}
		if loc.File != "" {
			loc.File = res.PrettyPath(logger.Path{Text: loc.File, Namespace: loc.Namespace})
		}
	}
}

func logPluginMessages(
	res resolver.Resolver,
	log logger.Log,
	name string,
	msgs []logger.Msg,
	thrown error,
	importSource *logger.Source,
	importPathRange logger.Range,
) bool {
	didLogError := false
	tracker := logger.MakeLineColumnTracker(importSource)

	// Report errors and warnings generated by the plugin
	for _, msg := range msgs {
		if msg.PluginName == "" {
			msg.PluginName = name
		}
		if msg.Kind == logger.Error {
			didLogError = true
		}

		// Sanitize the locations
		for _, note := range msg.Notes {
			sanetizeLocation(res, note.Location)
		}
		if msg.Data.Location == nil {
			msg.Data.Location = logger.LocationOrNil(&tracker, importPathRange)
		} else {
			sanetizeLocation(res, msg.Data.Location)
			if msg.Data.Location.File == "" && importSource != nil {
				msg.Data.Location.File = importSource.PrettyPath
			}
			if importSource != nil {
				msg.Notes = append(msg.Notes, logger.RangeData(&tracker, importPathRange,
					fmt.Sprintf("The plugin %q was triggered by this import", name)))
			}
		}

		log.AddMsg(msg)
	}

	// Report errors thrown by the plugin itself
	if thrown != nil {
		didLogError = true
		text := thrown.Error()
		log.AddMsg(logger.Msg{
			PluginName: name,
			Kind:       logger.Error,
			Data: logger.MsgData{
				Text:       text,
				Location:   logger.LocationOrNil(&tracker, importPathRange),
				UserDetail: thrown,
			},
		})
	}

	return didLogError
}

func runOnResolvePlugins(
	plugins []config.Plugin,
	res resolver.Resolver,
	log logger.Log,
	fs fs.FS,
	fsCache *cache.FSCache,
	importSource *logger.Source,
	importPathRange logger.Range,
	importNamespace string,
	path string,
	kind ast.ImportKind,
	absResolveDir string,
	pluginData interface{},
) (*resolver.ResolveResult, bool, resolver.DebugMeta) {
	resolverArgs := config.OnResolveArgs{
		Path:       path,
		ResolveDir: absResolveDir,
		Kind:       kind,
		PluginData: pluginData,
	}
	applyPath := logger.Path{
		Text:      path,
		Namespace: importNamespace,
	}
	if importSource != nil {
		resolverArgs.Importer = importSource.KeyPath
	} else {
		resolverArgs.Importer.Namespace = importNamespace
	}
	tracker := logger.MakeLineColumnTracker(importSource)

	// Apply resolver plugins in order until one succeeds
	for _, plugin := range plugins {
		for _, onResolve := range plugin.OnResolve {
			if !config.PluginAppliesToPath(applyPath, onResolve.Filter, onResolve.Namespace) {
				continue
			}

			result := onResolve.Callback(resolverArgs)
			pluginName := result.PluginName
			if pluginName == "" {
				pluginName = plugin.Name
			}
			didLogError := logPluginMessages(res, log, pluginName, result.Msgs, result.ThrownError, importSource, importPathRange)

			// Plugins can also provide additional file system paths to watch
			for _, file := range result.AbsWatchFiles {
				fsCache.ReadFile(fs, file)
			}
			for _, dir := range result.AbsWatchDirs {
				if entries, err, _ := fs.ReadDirectory(dir); err == nil {
					entries.SortedKeys()
				}
			}

			// Stop now if there was an error
			if didLogError {
				return nil, true, resolver.DebugMeta{}
			}

			// The "file" namespace is the default for non-external paths, but not
			// for external paths. External paths must explicitly specify the "file"
			// namespace.
			nsFromPlugin := result.Path.Namespace
			if result.Path.Namespace == "" && !result.External {
				result.Path.Namespace = "file"
			}

			// Otherwise, continue on to the next resolver if this loader didn't succeed
			if result.Path.Text == "" {
				if result.External {
					result.Path = logger.Path{Text: path}
				} else {
					continue
				}
			}

			// Paths in the file namespace must be absolute paths
			if result.Path.Namespace == "file" && !fs.IsAbs(result.Path.Text) {
				if nsFromPlugin == "file" {
					log.AddRangeError(&tracker, importPathRange,
						fmt.Sprintf("Plugin %q returned a path in the \"file\" namespace that is not an absolute path: %s", pluginName, result.Path.Text))
				} else {
					log.AddRangeError(&tracker, importPathRange,
						fmt.Sprintf("Plugin %q returned a non-absolute path: %s (set a namespace if this is not a file path)", pluginName, result.Path.Text))
				}
				return nil, true, resolver.DebugMeta{}
			}

			var sideEffectsData *resolver.SideEffectsData
			if result.IsSideEffectFree {
				sideEffectsData = &resolver.SideEffectsData{
					PluginName: pluginName,
				}
			}

			return &resolver.ResolveResult{
				PathPair:               resolver.PathPair{Primary: result.Path},
				IsExternal:             result.External,
				PluginData:             result.PluginData,
				PrimarySideEffectsData: sideEffectsData,
			}, false, resolver.DebugMeta{}
		}
	}

	// Resolve relative to the resolve directory by default. All paths in the
	// "file" namespace automatically have a resolve directory. Loader plugins
	// can also configure a custom resolve directory for files in other namespaces.
	result, debug := res.Resolve(absResolveDir, path, kind)

	// Warn when the case used for importing differs from the actual file name
	if result != nil && result.DifferentCase != nil && !helpers.IsInsideNodeModules(absResolveDir) {
		diffCase := *result.DifferentCase
		log.AddRangeWarning(&tracker, importPathRange, fmt.Sprintf(
			"Use %q instead of %q to avoid issues with case-sensitive file systems",
			res.PrettyPath(logger.Path{Text: fs.Join(diffCase.Dir, diffCase.Actual), Namespace: "file"}),
			res.PrettyPath(logger.Path{Text: fs.Join(diffCase.Dir, diffCase.Query), Namespace: "file"}),
		))
	}

	return result, false, debug
}

type loaderPluginResult struct {
	loader        config.Loader
	absResolveDir string
	pluginName    string
	pluginData    interface{}
}

func runOnLoadPlugins(
	plugins []config.Plugin,
	res resolver.Resolver,
	fs fs.FS,
	fsCache *cache.FSCache,
	log logger.Log,
	source *logger.Source,
	importSource *logger.Source,
	importPathRange logger.Range,
	pluginData interface{},
	isWatchMode bool,
) (loaderPluginResult, bool) {
	loaderArgs := config.OnLoadArgs{
		Path:       source.KeyPath,
		PluginData: pluginData,
	}
	tracker := logger.MakeLineColumnTracker(importSource)

	// Apply loader plugins in order until one succeeds
	for _, plugin := range plugins {
		for _, onLoad := range plugin.OnLoad {
			if !config.PluginAppliesToPath(source.KeyPath, onLoad.Filter, onLoad.Namespace) {
				continue
			}

			result := onLoad.Callback(loaderArgs)
			pluginName := result.PluginName
			if pluginName == "" {
				pluginName = plugin.Name
			}
			didLogError := logPluginMessages(res, log, pluginName, result.Msgs, result.ThrownError, importSource, importPathRange)

			// Plugins can also provide additional file system paths to watch
			for _, file := range result.AbsWatchFiles {
				fsCache.ReadFile(fs, file)
			}
			for _, dir := range result.AbsWatchDirs {
				if entries, err, _ := fs.ReadDirectory(dir); err == nil {
					entries.SortedKeys()
				}
			}

			// Stop now if there was an error
			if didLogError {
				if isWatchMode && source.KeyPath.Namespace == "file" {
					fsCache.ReadFile(fs, source.KeyPath.Text) // Read the file for watch mode tracking
				}
				return loaderPluginResult{}, false
			}

			// Otherwise, continue on to the next loader if this loader didn't succeed
			if result.Contents == nil {
				continue
			}

			source.Contents = *result.Contents
			loader := result.Loader
			if loader == config.LoaderNone {
				loader = config.LoaderJS
			}
			if result.AbsResolveDir == "" && source.KeyPath.Namespace == "file" {
				result.AbsResolveDir = fs.Dir(source.KeyPath.Text)
			}
			if isWatchMode && source.KeyPath.Namespace == "file" {
				fsCache.ReadFile(fs, source.KeyPath.Text) // Read the file for watch mode tracking
			}
			return loaderPluginResult{
				loader:        loader,
				absResolveDir: result.AbsResolveDir,
				pluginName:    pluginName,
				pluginData:    result.PluginData,
			}, true
		}
	}

	// Force disabled modules to be empty
	if source.KeyPath.IsDisabled() {
		return loaderPluginResult{loader: config.LoaderJS}, true
	}

	// Read normal modules from disk
	if source.KeyPath.Namespace == "file" {
		if contents, err, originalError := fsCache.ReadFile(fs, source.KeyPath.Text); err == nil {
			source.Contents = contents
			return loaderPluginResult{
				loader:        config.LoaderDefault,
				absResolveDir: fs.Dir(source.KeyPath.Text),
			}, true
		} else {
			if log.Level <= logger.LevelDebug && originalError != nil {
				log.AddDebug(nil, logger.Loc{}, fmt.Sprintf("Failed to read file %q: %s", source.KeyPath.Text, originalError.Error()))
			}
			if err == syscall.ENOENT {
				log.AddRangeError(&tracker, importPathRange,
					fmt.Sprintf("Could not read from file: %s", source.KeyPath.Text))
				return loaderPluginResult{}, false
			} else {
				log.AddRangeError(&tracker, importPathRange,
					fmt.Sprintf("Cannot read file %q: %s", res.PrettyPath(source.KeyPath), err.Error()))
				return loaderPluginResult{}, false
			}
		}
	}

	// Native support for data URLs. This is supported natively by node:
	// https://nodejs.org/docs/latest/api/esm.html#esm_data_imports
	if source.KeyPath.Namespace == "dataurl" {
		if parsed, ok := resolver.ParseDataURL(source.KeyPath.Text); ok {
			if mimeType := parsed.DecodeMIMEType(); mimeType != resolver.MIMETypeUnsupported {
				if contents, err := parsed.DecodeData(); err != nil {
					log.AddRangeError(&tracker, importPathRange,
						fmt.Sprintf("Could not load data URL: %s", err.Error()))
					return loaderPluginResult{loader: config.LoaderNone}, true
				} else {
					source.Contents = contents
					switch mimeType {
					case resolver.MIMETypeTextCSS:
						return loaderPluginResult{loader: config.LoaderCSS}, true
					case resolver.MIMETypeTextJavaScript:
						return loaderPluginResult{loader: config.LoaderJS}, true
					case resolver.MIMETypeApplicationJSON:
						return loaderPluginResult{loader: config.LoaderJSON}, true
					}
				}
			}
		}
	}

	// Otherwise, fail to load the path
	return loaderPluginResult{loader: config.LoaderNone}, true
}

func loaderFromFileExtension(extensionToLoader map[string]config.Loader, base string) config.Loader {
	// Pick the loader with the longest matching extension. So if there's an
	// extension for ".css" and for ".module.css", we want to match the one for
	// ".module.css" before the one for ".css".
	for {
		i := strings.IndexByte(base, '.')
		if i == -1 {
			break
		}
		if loader, ok := extensionToLoader[base[i:]]; ok {
			return loader
		}
		base = base[i+1:]
	}
	return config.LoaderNone
}

// Identify the path by its lowercase absolute path name with Windows-specific
// slashes substituted for standard slashes. This should hopefully avoid path
// issues on Windows where multiple different paths can refer to the same
// underlying file.
func canonicalFileSystemPathForWindows(absPath string) string {
	return strings.ReplaceAll(strings.ToLower(absPath), "\\", "/")
}

func hashForFileName(hashBytes []byte) string {
	return base32.StdEncoding.EncodeToString(hashBytes)[:8]
}

type scanner struct {
	log             logger.Log
	fs              fs.FS
	res             resolver.Resolver
	caches          *cache.CacheSet
	options         config.Options
	timer           *helpers.Timer
	uniqueKeyPrefix string

	// This is not guarded by a mutex because it's only ever modified by a single
	// thread. Note that not all results in the "results" array are necessarily
	// valid. Make sure to check the "ok" flag before using them.
	results       []parseResult
	visited       map[logger.Path]uint32
	resultChannel chan parseResult
	remaining     int
}

type EntryPoint struct {
	InputPath  string
	OutputPath string
	IsFile     bool
}

func generateUniqueKeyPrefix() (string, error) {
	var data [12]byte
	rand.Seed(time.Now().UnixNano())
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}

	// This is 16 bytes and shouldn't generate escape characters when put into strings
	return base64.URLEncoding.EncodeToString(data[:]), nil
}

func ScanBundle(
	log logger.Log,
	fs fs.FS,
	res resolver.Resolver,
	caches *cache.CacheSet,
	entryPoints []EntryPoint,
	options config.Options,
	timer *helpers.Timer,
) Bundle {
	timer.Begin("Scan phase")
	defer timer.End("Scan phase")

	applyOptionDefaults(&options)

	// Run "onStart" plugins in parallel
	onStartWaitGroup := sync.WaitGroup{}
	for _, plugin := range options.Plugins {
		for _, onStart := range plugin.OnStart {
			onStartWaitGroup.Add(1)
			go func(plugin config.Plugin, onStart config.OnStart) {
				result := onStart.Callback()
				logPluginMessages(res, log, plugin.Name, result.Msgs, result.ThrownError, nil, logger.Range{})
				onStartWaitGroup.Done()
			}(plugin, onStart)
		}
	}

	// Each bundling operation gets a separate unique key
	uniqueKeyPrefix, err := generateUniqueKeyPrefix()
	if err != nil {
		log.AddError(nil, logger.Loc{}, fmt.Sprintf("Failed to read from randomness source: %s", err.Error()))
	}

	s := scanner{
		log:             log,
		fs:              fs,
		res:             res,
		caches:          caches,
		options:         options,
		timer:           timer,
		results:         make([]parseResult, 0, caches.SourceIndexCache.LenHint()),
		visited:         make(map[logger.Path]uint32),
		resultChannel:   make(chan parseResult),
		uniqueKeyPrefix: uniqueKeyPrefix,
	}

	// Always start by parsing the runtime file
	s.results = append(s.results, parseResult{})
	s.remaining++
	go func() {
		source, ast, ok := globalRuntimeCache.parseRuntime(&options)
		s.resultChannel <- parseResult{
			file: scannerFile{
				inputFile: graph.InputFile{
					Source: source,
					Repr:   &graph.JSRepr{AST: ast},
				},
			},
			ok: ok,
		}
	}()

	s.preprocessInjectedFiles()
	entryPointMeta := s.addEntryPoints(entryPoints)
	s.scanAllDependencies()
	files := s.processScannedFiles()

	onStartWaitGroup.Wait()
	return Bundle{
		fs:              fs,
		res:             res,
		files:           files,
		entryPoints:     entryPointMeta,
		uniqueKeyPrefix: uniqueKeyPrefix,
	}
}

type inputKind uint8

const (
	inputKindNormal inputKind = iota
	inputKindEntryPoint
	inputKindStdin
)

// This returns the source index of the resulting file
func (s *scanner) maybeParseFile(
	resolveResult resolver.ResolveResult,
	prettyPath string,
	importSource *logger.Source,
	importPathRange logger.Range,
	pluginData interface{},
	kind inputKind,
	inject chan config.InjectedFile,
) uint32 {
	path := resolveResult.PathPair.Primary
	visitedKey := path
	if visitedKey.Namespace == "file" {
		visitedKey.Text = canonicalFileSystemPathForWindows(visitedKey.Text)
	}

	// Only parse a given file path once
	sourceIndex, ok := s.visited[visitedKey]
	if ok {
		return sourceIndex
	}

	sourceIndex = s.allocateSourceIndex(visitedKey, cache.SourceIndexNormal)
	s.visited[visitedKey] = sourceIndex
	s.remaining++
	optionsClone := s.options
	if kind != inputKindStdin {
		optionsClone.Stdin = nil
	}

	// Allow certain properties to be overridden
	if len(resolveResult.JSXFactory) > 0 {
		optionsClone.JSX.Factory = config.JSXExpr{Parts: resolveResult.JSXFactory}
	}
	if len(resolveResult.JSXFragment) > 0 {
		optionsClone.JSX.Fragment = config.JSXExpr{Parts: resolveResult.JSXFragment}
	}
	if resolveResult.UseDefineForClassFieldsTS != config.Unspecified {
		optionsClone.UseDefineForClassFields = resolveResult.UseDefineForClassFieldsTS
	}
	if resolveResult.PreserveUnusedImportsTS {
		optionsClone.PreserveUnusedImportsTS = true
	}
	optionsClone.TSTarget = resolveResult.TSTarget

	// Set the module type preference using node's module type rules
	if strings.HasSuffix(path.Text, ".mjs") || strings.HasSuffix(path.Text, ".mts") {
		optionsClone.ModuleType = config.ModuleESM
	} else if strings.HasSuffix(path.Text, ".cjs") || strings.HasSuffix(path.Text, ".cts") {
		optionsClone.ModuleType = config.ModuleCommonJS
	} else {
		optionsClone.ModuleType = resolveResult.ModuleType
	}

	// Enable bundling for injected files so we always do tree shaking. We
	// never want to include unnecessary code from injected files since they
	// are essentially bundled. However, if we do this we should skip the
	// resolving step when we're not bundling. It'd be strange to get
	// resolution errors when the top-level bundling controls are disabled.
	skipResolve := false
	if inject != nil && optionsClone.Mode != config.ModeBundle {
		optionsClone.Mode = config.ModeBundle
		skipResolve = true
	}

	// Special-case pretty-printed paths for data URLs
	if path.Namespace == "dataurl" {
		if _, ok := resolver.ParseDataURL(path.Text); ok {
			prettyPath = path.Text
			if len(prettyPath) > 64 {
				prettyPath = prettyPath[:64] + "..."
			}
			prettyPath = fmt.Sprintf("<%s>", prettyPath)
		}
	}

	var sideEffects graph.SideEffects
	if resolveResult.PrimarySideEffectsData != nil {
		sideEffects.Kind = graph.NoSideEffects_PackageJSON
		sideEffects.Data = resolveResult.PrimarySideEffectsData
	}

	go parseFile(parseArgs{
		fs:              s.fs,
		log:             s.log,
		res:             s.res,
		caches:          s.caches,
		keyPath:         path,
		prettyPath:      prettyPath,
		sourceIndex:     sourceIndex,
		importSource:    importSource,
		sideEffects:     sideEffects,
		importPathRange: importPathRange,
		pluginData:      pluginData,
		options:         optionsClone,
		results:         s.resultChannel,
		inject:          inject,
		skipResolve:     skipResolve,
		uniqueKeyPrefix: s.uniqueKeyPrefix,
	})

	return sourceIndex
}

func (s *scanner) allocateSourceIndex(path logger.Path, kind cache.SourceIndexKind) uint32 {
	// Allocate a source index using the shared source index cache so that
	// subsequent builds reuse the same source index and therefore use the
	// cached parse results for increased speed.
	sourceIndex := s.caches.SourceIndexCache.Get(path, kind)

	// Grow the results array to fit this source index
	if newLen := int(sourceIndex) + 1; len(s.results) < newLen {
		// Reallocate to a bigger array
		if cap(s.results) < newLen {
			s.results = append(make([]parseResult, 0, 2*newLen), s.results...)
		}

		// Grow in place
		s.results = s.results[:newLen]
	}

	return sourceIndex
}

func (s *scanner) preprocessInjectedFiles() {
	s.timer.Begin("Preprocess injected files")
	defer s.timer.End("Preprocess injected files")

	injectedFiles := make([]config.InjectedFile, 0, len(s.options.InjectedDefines)+len(s.options.InjectAbsPaths))
	duplicateInjectedFiles := make(map[string]bool)
	injectWaitGroup := sync.WaitGroup{}

	// These are virtual paths that are generated for compound "--define" values.
	// They are special-cased and are not available for plugins to intercept.
	for _, define := range s.options.InjectedDefines {
		// These should be unique by construction so no need to check for collisions
		visitedKey := logger.Path{Text: fmt.Sprintf("<define:%s>", define.Name)}
		sourceIndex := s.allocateSourceIndex(visitedKey, cache.SourceIndexNormal)
		s.visited[visitedKey] = sourceIndex
		source := logger.Source{
			Index:          sourceIndex,
			KeyPath:        visitedKey,
			PrettyPath:     s.res.PrettyPath(visitedKey),
			IdentifierName: js_ast.EnsureValidIdentifier(visitedKey.Text),
		}

		// The first "len(InjectedDefine)" injected files intentionally line up
		// with the injected defines by index. The index will be used to import
		// references to them in the parser.
		injectedFiles = append(injectedFiles, config.InjectedFile{
			Source:     source,
			DefineName: define.Name,
		})

		// Generate the file inline here since it has already been parsed
		expr := js_ast.Expr{Data: define.Data}
		ast := js_parser.LazyExportAST(s.log, source, js_parser.OptionsFromConfig(&s.options), expr, "")
		result := parseResult{
			ok: true,
			file: scannerFile{
				inputFile: graph.InputFile{
					Source: source,
					Repr:   &graph.JSRepr{AST: ast},
					Loader: config.LoaderJSON,
					SideEffects: graph.SideEffects{
						Kind: graph.NoSideEffects_PureData,
					},
				},
			},
		}

		// Append to the channel on a goroutine in case it blocks due to capacity
		s.remaining++
		go func() { s.resultChannel <- result }()
	}

	results := make([]config.InjectedFile, len(s.options.InjectAbsPaths))
	j := 0
	for _, absPath := range s.options.InjectAbsPaths {
		prettyPath := s.res.PrettyPath(logger.Path{Text: absPath, Namespace: "file"})
		absPathKey := canonicalFileSystemPathForWindows(absPath)

		if duplicateInjectedFiles[absPathKey] {
			s.log.AddError(nil, logger.Loc{}, fmt.Sprintf("Duplicate injected file %q", prettyPath))
			continue
		}

		duplicateInjectedFiles[absPathKey] = true
		resolveResult := s.res.ResolveAbs(absPath)

		if resolveResult == nil {
			s.log.AddError(nil, logger.Loc{}, fmt.Sprintf("Could not resolve %q", prettyPath))
			continue
		}

		channel := make(chan config.InjectedFile)
		s.maybeParseFile(*resolveResult, prettyPath, nil, logger.Range{}, nil, inputKindNormal, channel)

		// Wait for the results in parallel. The results slice is large enough so
		// it is not reallocated during the computations.
		injectWaitGroup.Add(1)
		go func(i int) {
			results[i] = <-channel
			injectWaitGroup.Done()
		}(j)
		j++
	}

	injectWaitGroup.Wait()
	injectedFiles = append(injectedFiles, results[:j]...)

	s.options.InjectedFiles = injectedFiles
}

func (s *scanner) addEntryPoints(entryPoints []EntryPoint) []graph.EntryPoint {
	s.timer.Begin("Add entry points")
	defer s.timer.End("Add entry points")

	// Reserve a slot for each entry point
	entryMetas := make([]graph.EntryPoint, 0, len(entryPoints)+1)

	// Treat stdin as an extra entry point
	if stdin := s.options.Stdin; stdin != nil {
		stdinPath := logger.Path{Text: "<stdin>"}
		if stdin.SourceFile != "" {
			if stdin.AbsResolveDir == "" {
				stdinPath = logger.Path{Text: stdin.SourceFile}
			} else if s.fs.IsAbs(stdin.SourceFile) {
				stdinPath = logger.Path{Text: stdin.SourceFile, Namespace: "file"}
			} else {
				stdinPath = logger.Path{Text: s.fs.Join(stdin.AbsResolveDir, stdin.SourceFile), Namespace: "file"}
			}
		}
		resolveResult := resolver.ResolveResult{PathPair: resolver.PathPair{Primary: stdinPath}}
		sourceIndex := s.maybeParseFile(resolveResult, s.res.PrettyPath(stdinPath), nil, logger.Range{}, nil, inputKindStdin, nil)
		entryMetas = append(entryMetas, graph.EntryPoint{
			OutputPath:  "stdin",
			SourceIndex: sourceIndex,
		})
	}

	// Check each entry point ahead of time to see if it's a real file
	entryPointAbsResolveDir := s.fs.Cwd()
	for i := range entryPoints {
		entryPoint := &entryPoints[i]
		absPath := entryPoint.InputPath
		if !s.fs.IsAbs(absPath) {
			absPath = s.fs.Join(entryPointAbsResolveDir, absPath)
		}
		dir := s.fs.Dir(absPath)
		base := s.fs.Base(absPath)
		if entries, err, originalError := s.fs.ReadDirectory(dir); err == nil {
			if entry, _ := entries.Get(base); entry != nil && entry.Kind(s.fs) == fs.FileEntry {
				entryPoint.IsFile = true

				// Entry point paths without a leading "./" are interpreted as package
				// paths. This happens because they go through general path resolution
				// like all other import paths so that plugins can run on them. Requiring
				// a leading "./" for a relative path simplifies writing plugins because
				// entry points aren't a special case.
				//
				// However, requiring a leading "./" also breaks backward compatibility
				// and makes working with the CLI more difficult. So attempt to insert
				// "./" automatically when needed. We don't want to unconditionally insert
				// a leading "./" because the path may not be a file system path. For
				// example, it may be a URL. So only insert a leading "./" when the path
				// is an exact match for an existing file.
				if !s.fs.IsAbs(entryPoint.InputPath) && resolver.IsPackagePath(entryPoint.InputPath) {
					entryPoint.InputPath = "./" + entryPoint.InputPath
				}
			}
		} else if s.log.Level <= logger.LevelDebug && originalError != nil {
			s.log.AddDebug(nil, logger.Loc{}, fmt.Sprintf("Failed to read directory %q: %s", absPath, originalError.Error()))
		}
	}

	// Add any remaining entry points. Run resolver plugins on these entry points
	// so plugins can alter where they resolve to. These are run in parallel in
	// case any of these plugins block.
	entryPointResolveResults := make([]*resolver.ResolveResult, len(entryPoints))
	entryPointWaitGroup := sync.WaitGroup{}
	entryPointWaitGroup.Add(len(entryPoints))
	for i, entryPoint := range entryPoints {
		go func(i int, entryPoint EntryPoint) {
			namespace := ""
			if entryPoint.IsFile {
				namespace = "file"
			}

			// Run the resolver and log an error if the path couldn't be resolved
			resolveResult, didLogError, debug := runOnResolvePlugins(
				s.options.Plugins,
				s.res,
				s.log,
				s.fs,
				&s.caches.FSCache,
				nil,
				logger.Range{},
				namespace,
				entryPoint.InputPath,
				ast.ImportEntryPoint,
				entryPointAbsResolveDir,
				nil,
			)
			if resolveResult != nil {
				if resolveResult.IsExternal {
					s.log.AddError(nil, logger.Loc{}, fmt.Sprintf("The entry point %q cannot be marked as external", entryPoint.InputPath))
				} else {
					entryPointResolveResults[i] = resolveResult
				}
			} else if !didLogError {
				hint := ""
				if !s.fs.IsAbs(entryPoint.InputPath) {
					if strings.ContainsRune(entryPoint.InputPath, '*') {
						hint = " (glob syntax must be expanded first before passing the paths to esbuild)"
					} else if query := s.res.ProbeResolvePackageAsRelative(entryPointAbsResolveDir, entryPoint.InputPath, ast.ImportEntryPoint); query != nil {
						hint = fmt.Sprintf(" (use %q to reference the file %q)", "./"+entryPoint.InputPath, s.res.PrettyPath(query.PathPair.Primary))
					}
				}
				debug.LogErrorMsg(s.log, nil, logger.Range{}, fmt.Sprintf("Could not resolve %q%s", entryPoint.InputPath, hint))
			}
			entryPointWaitGroup.Done()
		}(i, entryPoint)
	}
	entryPointWaitGroup.Wait()

	// Parse all entry points that were resolved successfully
	for i, resolveResult := range entryPointResolveResults {
		if resolveResult != nil {
			prettyPath := s.res.PrettyPath(resolveResult.PathPair.Primary)
			sourceIndex := s.maybeParseFile(*resolveResult, prettyPath, nil, logger.Range{}, resolveResult.PluginData, inputKindEntryPoint, nil)
			outputPath := entryPoints[i].OutputPath
			outputPathWasAutoGenerated := false

			// If the output path is missing, automatically generate one from the input path
			if outputPath == "" {
				outputPath = entryPoints[i].InputPath
				windowsVolumeLabel := ""

				// The ":" character is invalid in file paths on Windows except when
				// it's used as a volume separator. Special-case that here so volume
				// labels don't break on Windows.
				if s.fs.IsAbs(outputPath) && len(outputPath) >= 3 && outputPath[1] == ':' {
					if c := outputPath[0]; (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
						if c := outputPath[2]; c == '/' || c == '\\' {
							windowsVolumeLabel = outputPath[:3]
							outputPath = outputPath[3:]
						}
					}
				}

				// For cross-platform robustness, do not allow characters in the output
				// path that are invalid on Windows. This is especially relevant when
				// the input path is something other than a file path, such as a URL.
				outputPath = sanitizeFilePathForVirtualModulePath(outputPath)
				if windowsVolumeLabel != "" {
					outputPath = windowsVolumeLabel + outputPath
				}
				outputPathWasAutoGenerated = true

				// Strip the file extension from the output path if there is one so the
				// "out extension" setting is used instead
				if last := strings.LastIndexAny(outputPath, "/.\\"); last != -1 && outputPath[last] == '.' {
					outputPath = outputPath[:last]
				}
			}

			entryMetas = append(entryMetas, graph.EntryPoint{
				OutputPath:                 outputPath,
				SourceIndex:                sourceIndex,
				OutputPathWasAutoGenerated: outputPathWasAutoGenerated,
			})
		}
	}

	// Turn all automatically-generated output paths into absolute paths
	for i := range entryMetas {
		entryPoint := &entryMetas[i]
		if entryPoint.OutputPathWasAutoGenerated && !s.fs.IsAbs(entryPoint.OutputPath) {
			entryPoint.OutputPath = s.fs.Join(entryPointAbsResolveDir, entryPoint.OutputPath)
		}
	}

	// Automatically compute "outbase" if it wasn't provided
	if s.options.AbsOutputBase == "" {
		s.options.AbsOutputBase = lowestCommonAncestorDirectory(s.fs, entryMetas)
		if s.options.AbsOutputBase == "" {
			s.options.AbsOutputBase = entryPointAbsResolveDir
		}
	}

	// Turn all output paths back into relative paths, but this time relative to
	// the "outbase" value we computed above
	for i := range entryMetas {
		entryPoint := &entryMetas[i]
		if s.fs.IsAbs(entryPoint.OutputPath) {
			if !entryPoint.OutputPathWasAutoGenerated {
				// If an explicit absolute output path was specified, use the path
				// relative to the "outdir" directory
				if relPath, ok := s.fs.Rel(s.options.AbsOutputDir, entryPoint.OutputPath); ok {
					entryPoint.OutputPath = relPath
				}
			} else {
				// Otherwise if the absolute output path was derived from the input
				// path, use the path relative to the "outbase" directory
				if relPath, ok := s.fs.Rel(s.options.AbsOutputBase, entryPoint.OutputPath); ok {
					entryPoint.OutputPath = relPath
				}
			}
		}
	}

	return entryMetas
}

func lowestCommonAncestorDirectory(fs fs.FS, entryPoints []graph.EntryPoint) string {
	// Ignore any explicitly-specified output paths
	absPaths := make([]string, 0, len(entryPoints))
	for _, entryPoint := range entryPoints {
		if entryPoint.OutputPathWasAutoGenerated {
			absPaths = append(absPaths, entryPoint.OutputPath)
		}
	}

	if len(absPaths) == 0 {
		return ""
	}

	lowestAbsDir := fs.Dir(absPaths[0])

	for _, absPath := range absPaths[1:] {
		absDir := fs.Dir(absPath)
		lastSlash := 0
		a := 0
		b := 0

		for {
			runeA, widthA := utf8.DecodeRuneInString(absDir[a:])
			runeB, widthB := utf8.DecodeRuneInString(lowestAbsDir[b:])
			boundaryA := widthA == 0 || runeA == '/' || runeA == '\\'
			boundaryB := widthB == 0 || runeB == '/' || runeB == '\\'

			if boundaryA && boundaryB {
				if widthA == 0 || widthB == 0 {
					// Truncate to the smaller path if one path is a prefix of the other
					lowestAbsDir = absDir[:a]
					break
				} else {
					// Track the longest common directory so far
					lastSlash = a
				}
			} else if boundaryA != boundaryB || unicode.ToLower(runeA) != unicode.ToLower(runeB) {
				// If both paths are different at this point, stop and set the lowest so
				// far to the common parent directory. Compare using a case-insensitive
				// comparison to handle paths on Windows.
				lowestAbsDir = absDir[:lastSlash]
				break
			}

			a += widthA
			b += widthB
		}
	}

	return lowestAbsDir
}

func (s *scanner) scanAllDependencies() {
	s.timer.Begin("Scan all dependencies")
	defer s.timer.End("Scan all dependencies")

	// Continue scanning until all dependencies have been discovered
	for s.remaining > 0 {
		result := <-s.resultChannel
		s.remaining--
		if !result.ok {
			continue
		}

		// Don't try to resolve paths if we're not bundling
		if s.options.Mode == config.ModeBundle {
			records := *result.file.inputFile.Repr.ImportRecords()
			for importRecordIndex := range records {
				record := &records[importRecordIndex]

				// Skip this import record if the previous resolver call failed
				resolveResult := result.resolveResults[importRecordIndex]
				if resolveResult == nil {
					continue
				}

				path := resolveResult.PathPair.Primary
				if !resolveResult.IsExternal {
					// Handle a path within the bundle
					sourceIndex := s.maybeParseFile(*resolveResult, s.res.PrettyPath(path),
						&result.file.inputFile.Source, record.Range, resolveResult.PluginData, inputKindNormal, nil)
					record.SourceIndex = ast.MakeIndex32(sourceIndex)
				} else {
					// If the path to the external module is relative to the source
					// file, rewrite the path to be relative to the working directory
					if path.Namespace == "file" {
						if relPath, ok := s.fs.Rel(s.options.AbsOutputDir, path.Text); ok {
							// Prevent issues with path separators being different on Windows
							relPath = strings.ReplaceAll(relPath, "\\", "/")
							if resolver.IsPackagePath(relPath) {
								relPath = "./" + relPath
							}
							record.Path.Text = relPath
						} else {
							record.Path = path
						}
					} else {
						record.Path = path
					}
				}
			}
		}

		s.results[result.file.inputFile.Source.Index] = result
	}
}

func (s *scanner) processScannedFiles() []scannerFile {
	s.timer.Begin("Process scanned files")
	defer s.timer.End("Process scanned files")

	// Now that all files have been scanned, process the final file import records
	for i, result := range s.results {
		if !result.ok {
			continue
		}

		sb := strings.Builder{}
		isFirstImport := true

		// Begin the metadata chunk
		if s.options.NeedsMetafile {
			sb.Write(js_printer.QuoteForJSON(result.file.inputFile.Source.PrettyPath, s.options.ASCIIOnly))
			sb.WriteString(fmt.Sprintf(": {\n      \"bytes\": %d,\n      \"imports\": [", len(result.file.inputFile.Source.Contents)))
		}

		// Don't try to resolve paths if we're not bundling
		if s.options.Mode == config.ModeBundle {
			records := *result.file.inputFile.Repr.ImportRecords()
			tracker := logger.MakeLineColumnTracker(&result.file.inputFile.Source)

			for importRecordIndex := range records {
				record := &records[importRecordIndex]

				// Skip this import record if the previous resolver call failed
				resolveResult := result.resolveResults[importRecordIndex]
				if resolveResult == nil || !record.SourceIndex.IsValid() {
					continue
				}

				// Now that all files have been scanned, look for packages that are imported
				// both with "import" and "require". Rewrite any imports that reference the
				// "module" package.json field to the "main" package.json field instead.
				//
				// This attempts to automatically avoid the "dual package hazard" where a
				// package has both a CommonJS module version and an ECMAScript module
				// version and exports a non-object in CommonJS (often a function). If we
				// pick the "module" field and the package is imported with "require" then
				// code expecting a function will crash.
				if resolveResult.PathPair.HasSecondary() {
					secondaryKey := resolveResult.PathPair.Secondary
					if secondaryKey.Namespace == "file" {
						secondaryKey.Text = canonicalFileSystemPathForWindows(secondaryKey.Text)
					}
					if secondarySourceIndex, ok := s.visited[secondaryKey]; ok {
						record.SourceIndex = ast.MakeIndex32(secondarySourceIndex)
					}
				}

				// Generate metadata about each import
				if s.options.NeedsMetafile {
					if isFirstImport {
						isFirstImport = false
						sb.WriteString("\n        ")
					} else {
						sb.WriteString(",\n        ")
					}
					sb.WriteString(fmt.Sprintf("{\n          \"path\": %s,\n          \"kind\": %s\n        }",
						js_printer.QuoteForJSON(s.results[record.SourceIndex.GetIndex()].file.inputFile.Source.PrettyPath, s.options.ASCIIOnly),
						js_printer.QuoteForJSON(record.Kind.StringForMetafile(), s.options.ASCIIOnly)))
				}

				switch record.Kind {
				case ast.ImportAt, ast.ImportAtConditional:
					// Using a JavaScript file with CSS "@import" is not allowed
					otherFile := &s.results[record.SourceIndex.GetIndex()].file
					if _, ok := otherFile.inputFile.Repr.(*graph.JSRepr); ok {
						s.log.AddRangeError(&tracker, record.Range,
							fmt.Sprintf("Cannot import %q into a CSS file", otherFile.inputFile.Source.PrettyPath))
					} else if record.Kind == ast.ImportAtConditional {
						s.log.AddRangeError(&tracker, record.Range,
							"Bundling with conditional \"@import\" rules is not currently supported")
					}

				case ast.ImportURL:
					// Using a JavaScript or CSS file with CSS "url()" is not allowed
					otherFile := &s.results[record.SourceIndex.GetIndex()].file
					switch otherRepr := otherFile.inputFile.Repr.(type) {
					case *graph.CSSRepr:
						s.log.AddRangeError(&tracker, record.Range,
							fmt.Sprintf("Cannot use %q as a URL", otherFile.inputFile.Source.PrettyPath))

					case *graph.JSRepr:
						if otherRepr.AST.URLForCSS == "" {
							s.log.AddRangeError(&tracker, record.Range,
								fmt.Sprintf("Cannot use %q as a URL", otherFile.inputFile.Source.PrettyPath))
						}
					}
				}

				// If an import from a JavaScript file targets a CSS file, generate a
				// JavaScript stub to ensure that JavaScript files only ever import
				// other JavaScript files.
				if _, ok := result.file.inputFile.Repr.(*graph.JSRepr); ok {
					otherFile := &s.results[record.SourceIndex.GetIndex()].file
					if css, ok := otherFile.inputFile.Repr.(*graph.CSSRepr); ok {
						if s.options.WriteToStdout {
							s.log.AddRangeError(&tracker, record.Range,
								fmt.Sprintf("Cannot import %q into a JavaScript file without an output path configured", otherFile.inputFile.Source.PrettyPath))
						} else if !css.JSSourceIndex.IsValid() {
							stubKey := otherFile.inputFile.Source.KeyPath
							if stubKey.Namespace == "file" {
								stubKey.Text = canonicalFileSystemPathForWindows(stubKey.Text)
							}
							sourceIndex := s.allocateSourceIndex(stubKey, cache.SourceIndexJSStubForCSS)
							source := logger.Source{
								Index:      sourceIndex,
								PrettyPath: otherFile.inputFile.Source.PrettyPath,
							}
							s.results[sourceIndex] = parseResult{
								file: scannerFile{
									inputFile: graph.InputFile{
										Source: source,
										Repr: &graph.JSRepr{
											AST: js_parser.LazyExportAST(s.log, source,
												js_parser.OptionsFromConfig(&s.options), js_ast.Expr{Data: &js_ast.EObject{}}, ""),
											CSSSourceIndex: ast.MakeIndex32(record.SourceIndex.GetIndex()),
										},
									},
								},
								ok: true,
							}
							css.JSSourceIndex = ast.MakeIndex32(sourceIndex)
						}
						record.SourceIndex = css.JSSourceIndex
						if !css.JSSourceIndex.IsValid() {
							continue
						}
					}
				}

				// Warn about this import if it's a bare import statement without any
				// imported names (i.e. a side-effect-only import) and the module has
				// been marked as having no side effects.
				//
				// Except don't do this if this file is inside "node_modules" since
				// it's a bug in the package and the user won't be able to do anything
				// about it. Note that this can result in esbuild silently generating
				// broken code. If this actually happens for people, it's probably worth
				// re-enabling the warning about code inside "node_modules".
				if record.WasOriginallyBareImport && !s.options.IgnoreDCEAnnotations &&
					!helpers.IsInsideNodeModules(result.file.inputFile.Source.KeyPath.Text) {
					if otherModule := &s.results[record.SourceIndex.GetIndex()].file.inputFile; otherModule.SideEffects.Kind != graph.HasSideEffects &&
						// Do not warn if this is from a plugin, since removing the import
						// would cause the plugin to not run, and running a plugin is a side
						// effect.
						otherModule.SideEffects.Kind != graph.NoSideEffects_PureData_FromPlugin &&

						// Do not warn if this has no side effects because the parsed AST
						// is empty. This is the case for ".d.ts" files, for example.
						otherModule.SideEffects.Kind != graph.NoSideEffects_EmptyAST {

						var notes []logger.MsgData
						var by string
						if data := otherModule.SideEffects.Data; data != nil {
							if data.PluginName != "" {
								by = fmt.Sprintf(" by plugin %q", data.PluginName)
							} else {
								var text string
								if data.IsSideEffectsArrayInJSON {
									text = "It was excluded from the \"sideEffects\" array in the enclosing \"package.json\" file"
								} else {
									text = "\"sideEffects\" is false in the enclosing \"package.json\" file"
								}
								tracker := logger.MakeLineColumnTracker(data.Source)
								notes = append(notes, logger.RangeData(&tracker, data.Range, text))
							}
						}
						s.log.AddRangeWarningWithNotes(&tracker, record.Range,
							fmt.Sprintf("Ignoring this import because %q was marked as having no side effects%s",
								otherModule.Source.PrettyPath, by), notes)
					}
				}
			}
		}

		// End the metadata chunk
		if s.options.NeedsMetafile {
			if !isFirstImport {
				sb.WriteString("\n      ")
			}
			sb.WriteString("]\n    }")
		}

		result.file.jsonMetadataChunk = sb.String()

		// If this file is from the "file" loader, generate an additional file
		if result.file.inputFile.UniqueKeyForFileLoader != "" {
			bytes := []byte(result.file.inputFile.Source.Contents)

			// Add a hash to the file name to prevent multiple files with the same name
			// but different contents from colliding
			var hash string
			if config.HasPlaceholder(s.options.AssetPathTemplate, config.HashPlaceholder) {
				h := xxhash.New()
				h.Write(bytes)
				hash = hashForFileName(h.Sum(nil))
			}

			// Generate the input for the template
			_, _, originalExt := logger.PlatformIndependentPathDirBaseExt(result.file.inputFile.Source.KeyPath.Text)
			dir, base, ext := pathRelativeToOutbase(
				&result.file.inputFile,
				&s.options,
				s.fs,
				originalExt,
				/* avoidIndex */ false,
				/* customFilePath */ "",
			)

			// Apply the asset path template
			relPath := config.TemplateToString(config.SubstituteTemplate(s.options.AssetPathTemplate, config.PathPlaceholders{
				Dir:  &dir,
				Name: &base,
				Hash: &hash,
			})) + ext

			// Optionally add metadata about the file
			var jsonMetadataChunk string
			if s.options.NeedsMetafile {
				inputs := fmt.Sprintf("{\n        %s: {\n          \"bytesInOutput\": %d\n        }\n      }",
					js_printer.QuoteForJSON(result.file.inputFile.Source.PrettyPath, s.options.ASCIIOnly),
					len(bytes),
				)
				jsonMetadataChunk = fmt.Sprintf(
					"{\n      \"imports\": [],\n      \"exports\": [],\n      \"inputs\": %s,\n      \"bytes\": %d\n    }",
					inputs,
					len(bytes),
				)
			}

			// Generate the additional file to copy into the output directory
			result.file.inputFile.AdditionalFiles = []graph.OutputFile{{
				AbsPath:           s.fs.Join(s.options.AbsOutputDir, relPath),
				Contents:          bytes,
				JSONMetadataChunk: jsonMetadataChunk,
			}}
		}

		s.results[i] = result
	}

	// The linker operates on an array of files, so construct that now. This
	// can't be constructed earlier because we generate new parse results for
	// JavaScript stub files for CSS imports above.
	files := make([]scannerFile, len(s.results))
	for sourceIndex := range s.results {
		if result := &s.results[sourceIndex]; result.ok {
			s.validateTLA(uint32(sourceIndex))
			files[sourceIndex] = result.file
		}
	}
	return files
}

func (s *scanner) validateTLA(sourceIndex uint32) tlaCheck {
	result := &s.results[sourceIndex]

	if result.ok && result.tlaCheck.depth == 0 {
		if repr, ok := result.file.inputFile.Repr.(*graph.JSRepr); ok {
			result.tlaCheck.depth = 1
			if repr.AST.TopLevelAwaitKeyword.Len > 0 {
				result.tlaCheck.parent = ast.MakeIndex32(sourceIndex)
			}

			for importRecordIndex, record := range repr.AST.ImportRecords {
				if record.SourceIndex.IsValid() && (record.Kind == ast.ImportRequire || record.Kind == ast.ImportStmt) {
					parent := s.validateTLA(record.SourceIndex.GetIndex())
					if !parent.parent.IsValid() {
						continue
					}

					// Follow any import chains
					if record.Kind == ast.ImportStmt && (!result.tlaCheck.parent.IsValid() || parent.depth < result.tlaCheck.depth) {
						result.tlaCheck.depth = parent.depth + 1
						result.tlaCheck.parent = record.SourceIndex
						result.tlaCheck.importRecordIndex = uint32(importRecordIndex)
						continue
					}

					// Require of a top-level await chain is forbidden
					if record.Kind == ast.ImportRequire {
						var notes []logger.MsgData
						var tlaPrettyPath string
						otherSourceIndex := record.SourceIndex.GetIndex()

						// Build up a chain of relevant notes for all of the imports
						for {
							parentResult := &s.results[otherSourceIndex]
							parentRepr := parentResult.file.inputFile.Repr.(*graph.JSRepr)

							if parentRepr.AST.TopLevelAwaitKeyword.Len > 0 {
								tlaPrettyPath = parentResult.file.inputFile.Source.PrettyPath
								tracker := logger.MakeLineColumnTracker(&parentResult.file.inputFile.Source)
								notes = append(notes, logger.RangeData(&tracker, parentRepr.AST.TopLevelAwaitKeyword,
									fmt.Sprintf("The top-level await in %q is here", tlaPrettyPath)))
								break
							}

							if !parentResult.tlaCheck.parent.IsValid() {
								notes = append(notes, logger.MsgData{Text: "unexpected invalid index"})
								break
							}

							otherSourceIndex = parentResult.tlaCheck.parent.GetIndex()

							tracker := logger.MakeLineColumnTracker(&parentResult.file.inputFile.Source)
							notes = append(notes, logger.RangeData(&tracker,
								parentRepr.AST.ImportRecords[parent.importRecordIndex].Range,
								fmt.Sprintf("The file %q imports the file %q here",
									parentResult.file.inputFile.Source.PrettyPath, s.results[otherSourceIndex].file.inputFile.Source.PrettyPath)))
						}

						var text string
						importedPrettyPath := s.results[record.SourceIndex.GetIndex()].file.inputFile.Source.PrettyPath

						if importedPrettyPath == tlaPrettyPath {
							text = fmt.Sprintf("This require call is not allowed because the imported file %q contains a top-level await",
								importedPrettyPath)
						} else {
							text = fmt.Sprintf("This require call is not allowed because the transitive dependency %q contains a top-level await",
								tlaPrettyPath)
						}

						tracker := logger.MakeLineColumnTracker(&result.file.inputFile.Source)
						s.log.AddRangeErrorWithNotes(&tracker, record.Range, text, notes)
					}
				}
			}

			// Make sure that if we wrap this module in a closure, the closure is also
			// async. This happens when you call "import()" on this module and code
			// splitting is off.
			if result.tlaCheck.parent.IsValid() {
				repr.Meta.IsAsyncOrHasAsyncDependency = true
			}
		}
	}

	return result.tlaCheck
}

func DefaultExtensionToLoaderMap() map[string]config.Loader {
	return map[string]config.Loader{
		".js":   config.LoaderJS,
		".mjs":  config.LoaderJS,
		".cjs":  config.LoaderJS,
		".jsx":  config.LoaderJSX,
		".ts":   config.LoaderTS,
		".cts":  config.LoaderTSNoAmbiguousLessThan,
		".mts":  config.LoaderTSNoAmbiguousLessThan,
		".tsx":  config.LoaderTSX,
		".css":  config.LoaderCSS,
		".json": config.LoaderJSON,
		".txt":  config.LoaderText,
	}
}

func applyOptionDefaults(options *config.Options) {
	if options.ExtensionToLoader == nil {
		options.ExtensionToLoader = DefaultExtensionToLoaderMap()
	}
	if options.OutputExtensionJS == "" {
		options.OutputExtensionJS = ".js"
	}
	if options.OutputExtensionCSS == "" {
		options.OutputExtensionCSS = ".css"
	}

	// Configure default path templates
	if len(options.EntryPathTemplate) == 0 {
		options.EntryPathTemplate = []config.PathTemplate{
			{Data: "./", Placeholder: config.DirPlaceholder},
			{Data: "/", Placeholder: config.NamePlaceholder},
		}
	}
	if len(options.ChunkPathTemplate) == 0 {
		options.ChunkPathTemplate = []config.PathTemplate{
			{Data: "./", Placeholder: config.NamePlaceholder},
			{Data: "-", Placeholder: config.HashPlaceholder},
		}
	}
	if len(options.AssetPathTemplate) == 0 {
		options.AssetPathTemplate = []config.PathTemplate{
			{Data: "./", Placeholder: config.NamePlaceholder},
			{Data: "-", Placeholder: config.HashPlaceholder},
		}
	}

	options.ProfilerNames = !options.MinifyIdentifiers
}

func (b *Bundle) Compile(log logger.Log, options config.Options, timer *helpers.Timer) ([]graph.OutputFile, string) {
	timer.Begin("Compile phase")
	defer timer.End("Compile phase")

	applyOptionDefaults(&options)

	// The format can't be "preserve" while bundling
	if options.Mode == config.ModeBundle && options.OutputFormat == config.FormatPreserve {
		options.OutputFormat = config.FormatESModule
	}

	files := make([]graph.InputFile, len(b.files))
	for i, file := range b.files {
		files[i] = file.inputFile
	}

	// Get the base path from the options or choose the lowest common ancestor of all entry points
	allReachableFiles := findReachableFiles(files, b.entryPoints)

	// Compute source map data in parallel with linking
	timer.Begin("Spawn source map tasks")
	dataForSourceMaps := b.computeDataForSourceMapsInParallel(&options, allReachableFiles)
	timer.End("Spawn source map tasks")

	var resultGroups [][]graph.OutputFile
	if options.CodeSplitting || len(b.entryPoints) == 1 {
		// If code splitting is enabled or if there's only one entry point, link all entry points together
		resultGroups = [][]graph.OutputFile{link(
			&options, timer, log, b.fs, b.res, files, b.entryPoints, b.uniqueKeyPrefix, allReachableFiles, dataForSourceMaps)}
	} else {
		// Otherwise, link each entry point with the runtime file separately
		waitGroup := sync.WaitGroup{}
		resultGroups = make([][]graph.OutputFile, len(b.entryPoints))
		for i, entryPoint := range b.entryPoints {
			waitGroup.Add(1)
			go func(i int, entryPoint graph.EntryPoint) {
				entryPoints := []graph.EntryPoint{entryPoint}
				forked := timer.Fork()
				reachableFiles := findReachableFiles(files, entryPoints)
				resultGroups[i] = link(
					&options, forked, log, b.fs, b.res, files, entryPoints, b.uniqueKeyPrefix, reachableFiles, dataForSourceMaps)
				timer.Join(forked)
				waitGroup.Done()
			}(i, entryPoint)
		}
		waitGroup.Wait()
	}

	// Join the results in entry point order for determinism
	var outputFiles []graph.OutputFile
	for _, group := range resultGroups {
		outputFiles = append(outputFiles, group...)
	}

	// Also generate the metadata file if necessary
	var metafileJSON string
	if options.NeedsMetafile {
		timer.Begin("Generate metadata JSON")
		metafileJSON = b.generateMetadataJSON(outputFiles, allReachableFiles, options.ASCIIOnly)
		timer.End("Generate metadata JSON")
	}

	if !options.WriteToStdout {
		// Make sure an output file never overwrites an input file
		if !options.AllowOverwrite {
			sourceAbsPaths := make(map[string]uint32)
			for _, sourceIndex := range allReachableFiles {
				keyPath := b.files[sourceIndex].inputFile.Source.KeyPath
				if keyPath.Namespace == "file" {
					absPathKey := canonicalFileSystemPathForWindows(keyPath.Text)
					sourceAbsPaths[absPathKey] = sourceIndex
				}
			}
			for _, outputFile := range outputFiles {
				absPathKey := canonicalFileSystemPathForWindows(outputFile.AbsPath)
				if sourceIndex, ok := sourceAbsPaths[absPathKey]; ok {
					hint := ""
					switch logger.API {
					case logger.CLIAPI:
						hint = " (use \"--allow-overwrite\" to allow this)"
					case logger.JSAPI:
						hint = " (use \"allowOverwrite: true\" to allow this)"
					case logger.GoAPI:
						hint = " (use \"AllowOverwrite: true\" to allow this)"
					}
					log.AddError(nil, logger.Loc{},
						fmt.Sprintf("Refusing to overwrite input file %q%s",
							b.files[sourceIndex].inputFile.Source.PrettyPath, hint))
				}
			}
		}

		// Make sure an output file never overwrites another output file. This
		// is almost certainly unintentional and would otherwise happen silently.
		//
		// Make an exception for files that have identical contents. In that case
		// the duplicate is just silently filtered out. This can happen with the
		// "file" loader, for example.
		outputFileMap := make(map[string][]byte)
		end := 0
		for _, outputFile := range outputFiles {
			absPathKey := canonicalFileSystemPathForWindows(outputFile.AbsPath)
			contents, ok := outputFileMap[absPathKey]

			// If this isn't a duplicate, keep the output file
			if !ok {
				outputFileMap[absPathKey] = outputFile.Contents
				outputFiles[end] = outputFile
				end++
				continue
			}

			// If the names and contents are both the same, only keep the first one
			if bytes.Equal(contents, outputFile.Contents) {
				continue
			}

			// Otherwise, generate an error
			outputPath := outputFile.AbsPath
			if relPath, ok := b.fs.Rel(b.fs.Cwd(), outputPath); ok {
				outputPath = relPath
			}
			log.AddError(nil, logger.Loc{}, "Two output files share the same path but have different contents: "+outputPath)
		}
		outputFiles = outputFiles[:end]
	}

	return outputFiles, metafileJSON
}

// Find all files reachable from all entry points. This order should be
// deterministic given that the entry point order is deterministic, since the
// returned order is the postorder of the graph traversal and import record
// order within a given file is deterministic.
func findReachableFiles(files []graph.InputFile, entryPoints []graph.EntryPoint) []uint32 {
	visited := make(map[uint32]bool)
	var order []uint32
	var visit func(uint32)

	// Include this file and all files it imports
	visit = func(sourceIndex uint32) {
		if !visited[sourceIndex] {
			visited[sourceIndex] = true
			file := &files[sourceIndex]
			if repr, ok := file.Repr.(*graph.JSRepr); ok && repr.CSSSourceIndex.IsValid() {
				visit(repr.CSSSourceIndex.GetIndex())
			}
			for _, record := range *file.Repr.ImportRecords() {
				if record.SourceIndex.IsValid() {
					visit(record.SourceIndex.GetIndex())
				}
			}

			// Each file must come after its dependencies
			order = append(order, sourceIndex)
		}
	}

	// The runtime is always included in case it's needed
	visit(runtime.SourceIndex)

	// Include all files reachable from any entry point
	for _, entryPoint := range entryPoints {
		visit(entryPoint.SourceIndex)
	}

	return order
}

// This is done in parallel with linking because linking is a mostly serial
// phase and there are extra resources for parallelism. This could also be done
// during parsing but that would slow down parsing and delay the start of the
// linking phase, which then delays the whole bundling process.
//
// However, doing this during parsing would allow it to be cached along with
// the parsed ASTs which would then speed up incremental builds. In the future
// it could be good to optionally have this be computed during the parsing
// phase when incremental builds are active but otherwise still have it be
// computed during linking for optimal speed during non-incremental builds.
func (b *Bundle) computeDataForSourceMapsInParallel(options *config.Options, reachableFiles []uint32) func() []dataForSourceMap {
	if options.SourceMap == config.SourceMapNone {
		return func() []dataForSourceMap {
			return nil
		}
	}

	var waitGroup sync.WaitGroup
	results := make([]dataForSourceMap, len(b.files))

	for _, sourceIndex := range reachableFiles {
		if f := &b.files[sourceIndex]; f.inputFile.Loader.CanHaveSourceMap() {
			var approximateLineCount int32
			switch repr := f.inputFile.Repr.(type) {
			case *graph.JSRepr:
				approximateLineCount = repr.AST.ApproximateLineCount
			case *graph.CSSRepr:
				approximateLineCount = repr.AST.ApproximateLineCount
			}
			waitGroup.Add(1)
			go func(sourceIndex uint32, f *scannerFile, approximateLineCount int32) {
				result := &results[sourceIndex]
				result.lineOffsetTables = sourcemap.GenerateLineOffsetTables(f.inputFile.Source.Contents, approximateLineCount)
				sm := f.inputFile.InputSourceMap
				if !options.ExcludeSourcesContent {
					if sm == nil {
						// Simple case: no nested source map
						result.quotedContents = [][]byte{js_printer.QuoteForJSON(f.inputFile.Source.Contents, options.ASCIIOnly)}
					} else {
						// Complex case: nested source map
						result.quotedContents = make([][]byte, len(sm.Sources))
						nullContents := []byte("null")
						for i := range sm.Sources {
							// Missing contents become a "null" literal
							quotedContents := nullContents
							if i < len(sm.SourcesContent) {
								if value := sm.SourcesContent[i]; value.Quoted != "" {
									if options.ASCIIOnly && !isASCIIOnly(value.Quoted) {
										// Re-quote non-ASCII values if output is ASCII-only
										quotedContents = js_printer.QuoteForJSON(js_lexer.UTF16ToString(value.Value), options.ASCIIOnly)
									} else {
										// Otherwise just use the value directly from the input file
										quotedContents = []byte(value.Quoted)
									}
								}
							}
							result.quotedContents[i] = quotedContents
						}
					}
				}
				waitGroup.Done()
			}(sourceIndex, f, approximateLineCount)
		}
	}

	return func() []dataForSourceMap {
		waitGroup.Wait()
		return results
	}
}

func (b *Bundle) generateMetadataJSON(results []graph.OutputFile, allReachableFiles []uint32, asciiOnly bool) string {
	sb := strings.Builder{}
	sb.WriteString("{\n  \"inputs\": {")

	// Write inputs
	isFirst := true
	for _, sourceIndex := range allReachableFiles {
		if sourceIndex == runtime.SourceIndex {
			continue
		}
		if file := &b.files[sourceIndex]; len(file.jsonMetadataChunk) > 0 {
			if isFirst {
				isFirst = false
				sb.WriteString("\n    ")
			} else {
				sb.WriteString(",\n    ")
			}
			sb.WriteString(file.jsonMetadataChunk)
		}
	}

	sb.WriteString("\n  },\n  \"outputs\": {")

	// Write outputs
	isFirst = true
	paths := make(map[string]bool)
	for _, result := range results {
		if len(result.JSONMetadataChunk) > 0 {
			path := b.res.PrettyPath(logger.Path{Text: result.AbsPath, Namespace: "file"})
			if paths[path] {
				// Don't write out the same path twice (can happen with the "file" loader)
				continue
			}
			if isFirst {
				isFirst = false
				sb.WriteString("\n    ")
			} else {
				sb.WriteString(",\n    ")
			}
			paths[path] = true
			sb.WriteString(fmt.Sprintf("%s: ", js_printer.QuoteForJSON(path, asciiOnly)))
			sb.WriteString(result.JSONMetadataChunk)
		}
	}

	sb.WriteString("\n  }\n}\n")
	return sb.String()
}

type runtimeCacheKey struct {
	MangleSyntax      bool
	MinifyIdentifiers bool
	ES6               bool
}

type runtimeCache struct {
	astMutex sync.Mutex
	astMap   map[runtimeCacheKey]js_ast.AST
}

var globalRuntimeCache runtimeCache

func (cache *runtimeCache) parseRuntime(options *config.Options) (source logger.Source, runtimeAST js_ast.AST, ok bool) {
	key := runtimeCacheKey{
		// All configuration options that the runtime code depends on must go here
		MangleSyntax:      options.MangleSyntax,
		MinifyIdentifiers: options.MinifyIdentifiers,
		ES6:               runtime.CanUseES6(options.UnsupportedJSFeatures),
	}

	// Determine which source to use
	if key.ES6 {
		source = runtime.ES6Source
	} else {
		source = runtime.ES5Source
	}

	// Cache hit?
	(func() {
		cache.astMutex.Lock()
		defer cache.astMutex.Unlock()
		if cache.astMap != nil {
			runtimeAST, ok = cache.astMap[key]
		}
	})()
	if ok {
		return
	}

	// Cache miss
	var constraint int
	if key.ES6 {
		constraint = 2015
	} else {
		constraint = 5
	}
	log := logger.NewDeferLog(logger.DeferLogAll)
	runtimeAST, ok = js_parser.Parse(log, source, js_parser.OptionsFromConfig(&config.Options{
		// These configuration options must only depend on the key
		MangleSyntax:      key.MangleSyntax,
		MinifyIdentifiers: key.MinifyIdentifiers,
		UnsupportedJSFeatures: compat.UnsupportedJSFeatures(
			map[compat.Engine][]int{compat.ES: {constraint}}),

		// Always do tree shaking for the runtime because we never want to
		// include unnecessary runtime code
		TreeShaking: true,
	}))
	if log.HasErrors() {
		msgs := "Internal error: failed to parse runtime:\n"
		for _, msg := range log.Done() {
			msgs += msg.String(logger.OutputOptions{}, logger.TerminalInfo{})
		}
		panic(msgs[:len(msgs)-1])
	}

	// Cache for next time
	if ok {
		cache.astMutex.Lock()
		defer cache.astMutex.Unlock()
		if cache.astMap == nil {
			cache.astMap = make(map[runtimeCacheKey]js_ast.AST)
		}
		cache.astMap[key] = runtimeAST
	}
	return
}
