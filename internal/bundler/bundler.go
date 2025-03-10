package bundler

// The bundler is the core of the "build" and "transform" API calls. Each
// operation has two phases. The first phase scans the module graph, and is
// represented by the "ScanBundle" function. The second phase generates the
// output files from the module graph, and is implemented by the "Compile"
// function.

import (
	"bytes"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
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
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/resolver"
	"github.com/evanw/esbuild/internal/runtime"
	"github.com/evanw/esbuild/internal/sourcemap"
	"github.com/evanw/esbuild/internal/xxhash"
)

type scannerFile struct {
	// If "AbsMetadataFile" is present, this will be filled out with information
	// about this file in JSON format. This is a partial JSON file that will be
	// fully assembled later.
	jsonMetadataChunk string

	pluginData interface{}
	inputFile  graph.InputFile
}

// This is data related to source maps. It's computed in parallel with linking
// and must be ready by the time printing happens. This is beneficial because
// it is somewhat expensive to produce.
type DataForSourceMap struct {
	// This data is for the printer. It maps from byte offsets in the file (which
	// are stored at every AST node) to UTF-16 column offsets (required by source
	// maps).
	LineOffsetTables []sourcemap.LineOffsetTable

	// This contains the quoted contents of the original source file. It's what
	// needs to be embedded in the "sourcesContent" array in the final source
	// map. Quoting is precomputed because it's somewhat expensive.
	QuotedContents [][]byte
}

type Bundle struct {
	// The unique key prefix is a random string that is unique to every bundling
	// operation. It is used as a prefix for the unique keys assigned to every
	// chunk during linking. These unique keys are used to identify each chunk
	// before the final output paths have been computed.
	uniqueKeyPrefix string

	fs          fs.FS
	res         *resolver.Resolver
	files       []scannerFile
	entryPoints []graph.EntryPoint
	options     config.Options
}

type parseArgs struct {
	fs              fs.FS
	log             logger.Log
	res             *resolver.Resolver
	caches          *cache.CacheSet
	prettyPath      string
	importSource    *logger.Source
	importWith      *ast.ImportAssertOrWith
	sideEffects     graph.SideEffects
	pluginData      interface{}
	results         chan parseResult
	inject          chan config.InjectedFile
	uniqueKeyPrefix string
	keyPath         logger.Path
	options         config.Options
	importPathRange logger.Range
	sourceIndex     uint32
	skipResolve     bool
}

type parseResult struct {
	resolveResults     []*resolver.ResolveResult
	globResolveResults map[uint32]globResolveResult
	file               scannerFile
	tlaCheck           tlaCheck
	ok                 bool
}

type globResolveResult struct {
	resolveResults map[string]resolver.ResolveResult
	absPath        string
	prettyPath     string
	exportAlias    string
}

type tlaCheck struct {
	parent            ast.Index32
	depth             uint32
	importRecordIndex uint32
}

func parseFile(args parseArgs) {
	pathForIdentifierName := args.keyPath.Text

	// Identifier name generation may use the name of the parent folder if the
	// file name starts with "index". However, this is problematic when the
	// parent folder includes the parent directory of what the developer
	// considers to be the root of the source tree. If that happens, strip the
	// parent folder to avoid including it in the generated name.
	if relative, ok := args.fs.Rel(args.options.AbsOutputBase, pathForIdentifierName); ok {
		for {
			next := strings.TrimPrefix(strings.TrimPrefix(relative, "../"), "..\\")
			if relative == next {
				break
			}
			relative = next
		}
		pathForIdentifierName = relative
	}

	source := logger.Source{
		Index:          args.sourceIndex,
		KeyPath:        args.keyPath,
		PrettyPath:     args.prettyPath,
		IdentifierName: js_ast.GenerateNonUniqueNameFromPath(pathForIdentifierName),
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
		loader = config.LoaderFromFileExtension(args.options.ExtensionToLoader, base+ext)
	}

	// Reject unsupported import attributes when the loader isn't "copy" (since
	// "copy" is kind of like "external"). But only do this if this file was not
	// loaded by a plugin. Plugins are allowed to assign whatever semantics they
	// want to import attributes.
	if loader != config.LoaderCopy && pluginName == "" {
		for _, attr := range source.KeyPath.ImportAttributes.DecodeIntoArray() {
			var errorText string
			var errorRange js_lexer.KeyOrValue

			// We only currently handle "type: json"
			if attr.Key != "type" {
				errorText = fmt.Sprintf("Importing with the %q attribute is not supported", attr.Key)
				errorRange = js_lexer.KeyRange
			} else if attr.Value == "json" {
				loader = config.LoaderWithTypeJSON
				continue
			} else {
				errorText = fmt.Sprintf("Importing with a type attribute of %q is not supported", attr.Value)
				errorRange = js_lexer.ValueRange
			}

			// Everything else is an error
			r := args.importPathRange
			if args.importWith != nil {
				r = js_lexer.RangeOfImportAssertOrWith(*args.importSource, *ast.FindAssertOrWithEntry(args.importWith.Entries, attr.Key), errorRange)
			}
			tracker := logger.MakeLineColumnTracker(args.importSource)
			args.log.AddError(&tracker, r, errorText)
			if args.inject != nil {
				args.inject <- config.InjectedFile{
					Source: source,
				}
			}
			args.results <- parseResult{}
			return
		}
	}

	if loader == config.LoaderEmpty {
		source.Contents = ""
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
			args.log.AddErrorWithNotes(nil, logger.Range{},
				fmt.Sprintf("panic: %v (while parsing %q)", r, source.PrettyPath),
				[]logger.MsgData{{Text: helpers.PrettyPrintedStack()}})
			args.results <- result
		}
	}()

	switch loader {
	case config.LoaderJS, config.LoaderEmpty:
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

	case config.LoaderCSS, config.LoaderGlobalCSS, config.LoaderLocalCSS:
		ast := args.caches.CSSCache.Parse(args.log, source, css_parser.OptionsFromConfig(loader, &args.options))
		result.file.inputFile.Repr = &graph.CSSRepr{AST: ast}
		result.ok = true

	case config.LoaderJSON, config.LoaderWithTypeJSON:
		expr, ok := args.caches.JSONCache.Parse(args.log, source, js_parser.JSONOptions{
			UnsupportedJSFeatures: args.options.UnsupportedJSFeatures,
		})
		ast := js_parser.LazyExportAST(args.log, source, js_parser.OptionsFromConfig(&args.options), expr, "")
		if loader == config.LoaderWithTypeJSON {
			// The exports kind defaults to "none", in which case the linker picks
			// either ESM or CommonJS depending on the situation. Dynamic imports
			// causes the linker to pick CommonJS which uses "require()" and then
			// converts the return value to ESM, which adds extra properties that
			// aren't supposed to be there when "{ with: { type: 'json' } }" is
			// present. So if there's an import attribute, we force the type to
			// be ESM to avoid this.
			ast.ExportsKind = js_ast.ExportsESM
		}
		if pluginName != "" {
			result.file.inputFile.SideEffects.Kind = graph.NoSideEffects_PureData_FromPlugin
		} else {
			result.file.inputFile.SideEffects.Kind = graph.NoSideEffects_PureData
		}
		result.file.inputFile.Repr = &graph.JSRepr{AST: ast}
		result.ok = ok

	case config.LoaderText:
		source.Contents = strings.TrimPrefix(source.Contents, "\xEF\xBB\xBF") // Strip any UTF-8 BOM from the text
		encoded := base64.StdEncoding.EncodeToString([]byte(source.Contents))
		expr := js_ast.Expr{Data: &js_ast.EString{Value: helpers.StringToUTF16(source.Contents)}}
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
		expr := js_ast.Expr{Data: &js_ast.EString{Value: helpers.StringToUTF16(encoded)}}
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
		expr := js_ast.Expr{Data: &js_ast.EString{Value: helpers.StringToUTF16(encoded)}}
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
		url := helpers.EncodeStringAsShortestDataURL(mimeType, source.Contents)
		expr := js_ast.Expr{Data: &js_ast.EString{Value: helpers.StringToUTF16(url)}}
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
		expr := js_ast.Expr{Data: &js_ast.EString{
			Value:             helpers.StringToUTF16(uniqueKeyPath),
			ContainsUniqueKey: true,
		}}
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
		result.file.inputFile.UniqueKeyForAdditionalFile = uniqueKey

	case config.LoaderCopy:
		uniqueKey := fmt.Sprintf("%sA%08d", args.uniqueKeyPrefix, args.sourceIndex)
		uniqueKeyPath := uniqueKey + source.KeyPath.IgnoredSuffix
		result.file.inputFile.Repr = &graph.CopyRepr{
			URLForCode: uniqueKeyPath,
		}
		result.ok = true

		// Mark that this file is from the "copy" loader
		result.file.inputFile.UniqueKeyForAdditionalFile = uniqueKey

	default:
		var message string
		if source.KeyPath.Namespace == "file" && ext != "" {
			message = fmt.Sprintf("No loader is configured for %q files: %s", ext, source.PrettyPath)
		} else {
			message = fmt.Sprintf("Do not know how to load path: %s", source.PrettyPath)
		}
		tracker := logger.MakeLineColumnTracker(args.importSource)
		args.log.AddError(&tracker, args.importPathRange, message)
	}

	// Only continue now if parsing was successful
	if result.ok {
		// Run the resolver on the parse thread so it's not run on the main thread.
		// That way the main thread isn't blocked if the resolver takes a while.
		if recordsPtr := result.file.inputFile.Repr.ImportRecords(); args.options.Mode == config.ModeBundle && !args.skipResolve && recordsPtr != nil {
			// Clone the import records because they will be mutated later
			records := append([]ast.ImportRecord{}, *recordsPtr...)
			*recordsPtr = records
			result.resolveResults = make([]*resolver.ResolveResult, len(records))

			if len(records) > 0 {
				type cacheEntry struct {
					resolveResult *resolver.ResolveResult
					debug         resolver.DebugMeta
					didLogError   bool
				}

				type cacheKey struct {
					kind  ast.ImportKind
					path  string
					attrs logger.ImportAttributes
				}
				resolverCache := make(map[cacheKey]cacheEntry)
				tracker := logger.MakeLineColumnTracker(&source)

				for importRecordIndex := range records {
					// Don't try to resolve imports that are already resolved
					record := &records[importRecordIndex]
					if record.SourceIndex.IsValid() {
						continue
					}

					// Encode the import attributes
					var attrs logger.ImportAttributes
					if record.AssertOrWith != nil && record.AssertOrWith.Keyword == ast.WithKeyword {
						data := make(map[string]string, len(record.AssertOrWith.Entries))
						for _, entry := range record.AssertOrWith.Entries {
							data[helpers.UTF16ToString(entry.Key)] = helpers.UTF16ToString(entry.Value)
						}
						attrs = logger.EncodeImportAttributes(data)
					}

					// Special-case glob pattern imports
					if record.GlobPattern != nil {
						prettyPath := helpers.GlobPatternToString(record.GlobPattern.Parts)
						switch record.GlobPattern.Kind {
						case ast.ImportRequire:
							prettyPath = fmt.Sprintf("require(%q)", prettyPath)
						case ast.ImportDynamic:
							prettyPath = fmt.Sprintf("import(%q)", prettyPath)
						}
						if results, msg := args.res.ResolveGlob(absResolveDir, record.GlobPattern.Parts, record.GlobPattern.Kind, prettyPath); results != nil {
							if msg != nil {
								args.log.AddID(msg.ID, msg.Kind, &tracker, record.Range, msg.Data.Text)
							}
							if result.globResolveResults == nil {
								result.globResolveResults = make(map[uint32]globResolveResult)
							}
							for key, result := range results {
								result.PathPair.Primary.ImportAttributes = attrs
								if result.PathPair.HasSecondary() {
									result.PathPair.Secondary.ImportAttributes = attrs
								}
								results[key] = result
							}
							result.globResolveResults[uint32(importRecordIndex)] = globResolveResult{
								resolveResults: results,
								absPath:        args.fs.Join(absResolveDir, "(glob)"),
								prettyPath:     fmt.Sprintf("%s in %s", prettyPath, result.file.inputFile.Source.PrettyPath),
								exportAlias:    record.GlobPattern.ExportAlias,
							}
						} else {
							args.log.AddError(&tracker, record.Range, fmt.Sprintf("Could not resolve %s", prettyPath))
						}
						continue
					}

					// Ignore records that the parser has discarded. This is used to remove
					// type-only imports in TypeScript files.
					if record.Flags.Has(ast.IsUnused) {
						continue
					}

					// Cache the path in case it's imported multiple times in this file
					cacheKey := cacheKey{
						kind:  record.Kind,
						path:  record.Path.Text,
						attrs: attrs,
					}
					entry, ok := resolverCache[cacheKey]
					if ok {
						result.resolveResults[importRecordIndex] = entry.resolveResult
					} else {
						// Run the resolver and log an error if the path couldn't be resolved
						resolveResult, didLogError, debug := RunOnResolvePlugins(
							args.options.Plugins,
							args.res,
							args.log,
							args.fs,
							&args.caches.FSCache,
							&source,
							record.Range,
							source.KeyPath,
							record.Path.Text,
							attrs,
							record.Kind,
							absResolveDir,
							pluginData,
						)
						if resolveResult != nil {
							resolveResult.PathPair.Primary.ImportAttributes = attrs
							if resolveResult.PathPair.HasSecondary() {
								resolveResult.PathPair.Secondary.ImportAttributes = attrs
							}
						}
						entry = cacheEntry{
							resolveResult: resolveResult,
							debug:         debug,
							didLogError:   didLogError,
						}
						resolverCache[cacheKey] = entry

						// All "require.resolve()" imports should be external because we don't
						// want to waste effort traversing into them
						if record.Kind == ast.ImportRequireResolve {
							if resolveResult != nil && resolveResult.PathPair.IsExternal {
								// Allow path substitution as long as the result is external
								result.resolveResults[importRecordIndex] = resolveResult
							} else if !record.Flags.Has(ast.HandlesImportErrors) {
								args.log.AddID(logger.MsgID_Bundler_RequireResolveNotExternal, logger.Warning, &tracker, record.Range,
									fmt.Sprintf("%q should be marked as external for use with \"require.resolve\"", record.Path.Text))
							}
							continue
						}
					}

					// Check whether we should log an error every time the result is nil,
					// even if it's from the cache. Do this because the error may not
					// have been logged for nil entries if the previous instances had
					// the "HandlesImportErrors" flag.
					if entry.resolveResult == nil {
						// Failed imports inside a try/catch are silently turned into
						// external imports instead of causing errors. This matches a common
						// code pattern for conditionally importing a module with a graceful
						// fallback.
						if !entry.didLogError && !record.Flags.Has(ast.HandlesImportErrors) {
							// Report an error
							text, suggestion, notes := ResolveFailureErrorTextSuggestionNotes(args.res, record.Path.Text, record.Kind,
								pluginName, args.fs, absResolveDir, args.options.Platform, source.PrettyPath, entry.debug.ModifiedImportPath)
							entry.debug.LogErrorMsg(args.log, &source, record.Range, text, suggestion, notes)

							// Only report this error once per unique import path in the file
							entry.didLogError = true
							resolverCache[cacheKey] = entry
						} else if !entry.didLogError && record.Flags.Has(ast.HandlesImportErrors) {
							// Report a debug message about why there was no error
							args.log.AddIDWithNotes(logger.MsgID_Bundler_IgnoredDynamicImport, logger.Debug, &tracker, record.Range,
								fmt.Sprintf("Importing %q was allowed even though it could not be resolved because dynamic import failures appear to be handled here:",
									record.Path.Text), []logger.MsgData{tracker.MsgData(js_lexer.RangeOfIdentifier(source, record.ErrorHandlerLoc),
									"The handler for dynamic import failures is here:")})
						}
						continue
					}

					result.resolveResults[importRecordIndex] = entry.resolveResult
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
				tracker := logger.MakeLineColumnTracker(&source)

				if path, contents := extractSourceMapFromComment(args.log, args.fs, &args.caches.FSCache,
					&source, &tracker, sourceMapComment, absResolveDir); contents != nil {
					prettyPath := resolver.PrettyPath(args.fs, path)
					log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, args.log.Overrides)

					sourceMap := js_parser.ParseSourceMap(log, logger.Source{
						KeyPath:    path,
						PrettyPath: prettyPath,
						Contents:   *contents,
					})

					if msgs := log.Done(); len(msgs) > 0 {
						var text string
						if path.Namespace == "file" {
							text = fmt.Sprintf("The source map %q was referenced by the file %q here:", prettyPath, args.prettyPath)
						} else {
							text = fmt.Sprintf("This source map came from the file %q here:", args.prettyPath)
						}
						note := tracker.MsgData(sourceMapComment.Range, text)
						for _, msg := range msgs {
							msg.Notes = append(msg.Notes, note)
							args.log.AddMsg(msg)
						}
					}

					// If "sourcesContent" entries aren't present, try filling them in
					// using the file system. This includes both generating the entire
					// "sourcesContent" array if it's absent as well as filling in
					// individual null entries in the array if the array is present.
					if sourceMap != nil && !args.options.ExcludeSourcesContent {
						// Make sure "sourcesContent" is big enough
						if len(sourceMap.SourcesContent) < len(sourceMap.Sources) {
							slice := make([]sourcemap.SourceContent, len(sourceMap.Sources))
							copy(slice, sourceMap.SourcesContent)
							sourceMap.SourcesContent = slice
						}

						for i, source := range sourceMap.Sources {
							// Convert absolute paths to "file://" URLs, which is especially important
							// for Windows where file paths don't look like URLs at all (they use "\"
							// as a path separator and start with a "C:\" volume label instead of "/").
							//
							// The new source map specification (https://tc39.es/ecma426/) says that
							// each source is "a string that is a (potentially relative) URL". So we
							// should technically not be finding absolute paths here in the first place.
							//
							// However, for a long time source maps was poorly-specified. The old source
							// map specification (https://sourcemaps.info/spec.html) only says "sources"
							// is "a list of original sources used by the mappings entry" which could
							// be anything, really.
							//
							// So it makes sense that software which predates the formal specification
							// of source maps might fill in the sources array with absolute file paths
							// instead of URLs. Here are some cases where that happened:
							//
							// - https://github.com/mozilla/source-map/issues/355
							// - https://github.com/webpack/webpack/issues/8226
							//
							if path.Namespace == "file" && args.fs.IsAbs(source) {
								source = helpers.FileURLFromFilePath(source).String()
								sourceMap.Sources[i] = source
							}

							// Attempt to fill in null entries using the file system
							if sourceMap.SourcesContent[i].Value == nil {
								if sourceURL, err := url.Parse(source); err == nil && helpers.IsFileURL(sourceURL) {
									if contents, err, _ := args.caches.FSCache.ReadFile(args.fs, helpers.FilePathFromFileURL(args.fs, sourceURL)); err == nil {
										sourceMap.SourcesContent[i].Value = helpers.StringToUTF16(contents)
									}
								}
							}
						}
					}

					result.file.inputFile.InputSourceMap = sourceMap
				}
			}
		}
	}

	// Note: We must always send on the "inject" channel before we send on the
	// "results" channel to avoid deadlock
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

		// Once we send on the "inject" channel, the main thread may mutate the
		// "options" object to populate the "InjectedFiles" field. So we must
		// only send on the "inject" channel after we're done using the "options"
		// object so we don't introduce a data race.
		isCopyLoader := loader == config.LoaderCopy
		if isCopyLoader && args.skipResolve {
			// This is not allowed because the import path would have to be rewritten,
			// but import paths are not rewritten when bundling isn't enabled.
			args.log.AddError(nil, logger.Range{},
				fmt.Sprintf("Cannot inject %q with the \"copy\" loader without bundling enabled", source.PrettyPath))
		}
		args.inject <- config.InjectedFile{
			Source:       source,
			Exports:      exports,
			IsCopyLoader: isCopyLoader,
		}
	}

	args.results <- result
}

func ResolveFailureErrorTextSuggestionNotes(
	res *resolver.Resolver,
	path string,
	kind ast.ImportKind,
	pluginName string,
	fs fs.FS,
	absResolveDir string,
	platform config.Platform,
	originatingFilePath string,
	modifiedImportPath string,
) (text string, suggestion string, notes []logger.MsgData) {
	if modifiedImportPath != "" {
		text = fmt.Sprintf("Could not resolve %q (originally %q)", modifiedImportPath, path)
		notes = append(notes, logger.MsgData{Text: fmt.Sprintf(
			"The path %q was remapped to %q using the alias feature, which then couldn't be resolved. "+
				"Keep in mind that import path aliases are resolved in the current working directory.",
			path, modifiedImportPath)})
		path = modifiedImportPath
	} else {
		text = fmt.Sprintf("Could not resolve %q", path)
	}
	hint := ""

	if resolver.IsPackagePath(path) && !fs.IsAbs(path) {
		hint = fmt.Sprintf("You can mark the path %q as external to exclude it from the bundle, which will remove this error and leave the unresolved path in the bundle.", path)
		if kind == ast.ImportRequire {
			hint += " You can also surround this \"require\" call with a try/catch block to handle this failure at run-time instead of bundle-time."
		} else if kind == ast.ImportDynamic {
			hint += " You can also add \".catch()\" here to handle this failure at run-time instead of bundle-time."
		}
		if pluginName == "" && !fs.IsAbs(path) {
			if query, _ := res.ProbeResolvePackageAsRelative(absResolveDir, path, kind); query != nil {
				hint = fmt.Sprintf("Use the relative path %q to reference the file %q. "+
					"Without the leading \"./\", the path %q is being interpreted as a package path instead.",
					"./"+path, resolver.PrettyPath(fs, query.PathPair.Primary), path)
				suggestion = string(helpers.QuoteForJSON("./"+path, false))
			}
		}
	}

	if platform != config.PlatformNode {
		pkg := strings.TrimPrefix(path, "node:")
		if resolver.BuiltInNodeModules[pkg] {
			var how string
			switch logger.API {
			case logger.CLIAPI:
				how = "--platform=node"
			case logger.JSAPI:
				how = "platform: 'node'"
			case logger.GoAPI:
				how = "Platform: api.PlatformNode"
			}
			hint = fmt.Sprintf("The package %q wasn't found on the file system but is built into node. "+
				"Are you trying to bundle for node? You can use %q to do that, which will remove this error.", path, how)
		}
	}

	if absResolveDir == "" && pluginName != "" {
		where := ""
		if originatingFilePath != "" {
			where = fmt.Sprintf(" for the file %q", originatingFilePath)
		}
		hint = fmt.Sprintf("The plugin %q didn't set a resolve directory%s, "+
			"so esbuild did not search for %q on the file system.", pluginName, where, path)
	}

	if hint != "" {
		if modifiedImportPath != "" {
			// Add a newline if there's already a paragraph of text
			notes = append(notes, logger.MsgData{})

			// Don't add a suggestion if the path was rewritten using an alias
			suggestion = ""
		}
		notes = append(notes, logger.MsgData{Text: hint})
	}
	return
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
	source *logger.Source,
	tracker *logger.LineColumnTracker,
	comment logger.Span,
	absResolveDir string,
) (logger.Path, *string) {
	// Support data URLs
	if parsed, ok := resolver.ParseDataURL(comment.Text); ok {
		contents, err := parsed.DecodeData()
		if err != nil {
			log.AddID(logger.MsgID_SourceMap_UnsupportedSourceMapComment, logger.Warning, tracker, comment.Range,
				fmt.Sprintf("Unsupported source map comment: %s", err.Error()))
			return logger.Path{}, nil
		}
		path := source.KeyPath
		path.IgnoredSuffix = "#sourceMappingURL"
		return path, &contents
	}

	// Support file URLs of two forms:
	//
	//   Relative: "./foo.js.map"
	//   Absolute: "file:///Users/User/Desktop/foo.js.map"
	//
	var absPath string
	if commentURL, err := url.Parse(comment.Text); err != nil {
		// Show a warning if the comment can't be parsed as a URL
		log.AddID(logger.MsgID_SourceMap_UnsupportedSourceMapComment, logger.Warning, tracker, comment.Range,
			fmt.Sprintf("Unsupported source map comment: %s", err.Error()))
		return logger.Path{}, nil
	} else if commentURL.Scheme != "" && commentURL.Scheme != "file" {
		// URLs with schemes other than "file" are unsupported (e.g. "https"),
		// but don't warn the user about this because it's not a bug they can fix
		log.AddID(logger.MsgID_SourceMap_UnsupportedSourceMapComment, logger.Debug, tracker, comment.Range,
			fmt.Sprintf("Unsupported source map comment: Unsupported URL scheme %q", commentURL.Scheme))
		return logger.Path{}, nil
	} else if commentURL.Host != "" && commentURL.Host != "localhost" {
		// File URLs with hosts are unsupported (e.g. "file://foo.js.map")
		log.AddID(logger.MsgID_SourceMap_UnsupportedSourceMapComment, logger.Warning, tracker, comment.Range,
			fmt.Sprintf("Unsupported source map comment: Unsupported host %q in file URL", commentURL.Host))
		return logger.Path{}, nil
	} else if helpers.IsFileURL(commentURL) {
		// Handle absolute file URLs
		absPath = helpers.FilePathFromFileURL(fs, commentURL)
	} else if absResolveDir == "" {
		// Fail if plugins don't set a resolve directory
		log.AddID(logger.MsgID_SourceMap_UnsupportedSourceMapComment, logger.Debug, tracker, comment.Range,
			"Unsupported source map comment: Cannot resolve relative URL without a resolve directory")
		return logger.Path{}, nil
	} else {
		// Join the (potentially relative) URL path from the comment text
		// to the resolve directory path to form the final absolute path
		absResolveURL := helpers.FileURLFromFilePath(absResolveDir)
		if !strings.HasSuffix(absResolveURL.Path, "/") {
			absResolveURL.Path += "/"
		}
		absPath = helpers.FilePathFromFileURL(fs, absResolveURL.ResolveReference(commentURL))
	}

	// Try to read the file contents
	path := logger.Path{Text: absPath, Namespace: "file"}
	if contents, err, _ := fsCache.ReadFile(fs, absPath); err == syscall.ENOENT {
		log.AddID(logger.MsgID_SourceMap_MissingSourceMap, logger.Debug, tracker, comment.Range,
			fmt.Sprintf("Cannot read file: %s", absPath))
		return logger.Path{}, nil
	} else if err != nil {
		log.AddID(logger.MsgID_SourceMap_MissingSourceMap, logger.Warning, tracker, comment.Range,
			fmt.Sprintf("Cannot read file %q: %s", resolver.PrettyPath(fs, path), err.Error()))
		return logger.Path{}, nil
	} else {
		return path, &contents
	}
}

func sanitizeLocation(fs fs.FS, loc *logger.MsgLocation) {
	if loc != nil {
		if loc.Namespace == "" {
			loc.Namespace = "file"
		}
		if loc.File != "" {
			loc.File = resolver.PrettyPath(fs, logger.Path{Text: loc.File, Namespace: loc.Namespace})
		}
	}
}

func logPluginMessages(
	fs fs.FS,
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
			sanitizeLocation(fs, note.Location)
		}
		if msg.Data.Location == nil {
			msg.Data.Location = tracker.MsgLocationOrNil(importPathRange)
		} else {
			sanitizeLocation(fs, msg.Data.Location)
			if importSource != nil {
				if msg.Data.Location.File == "" {
					msg.Data.Location.File = importSource.PrettyPath
				}
				msg.Notes = append(msg.Notes, tracker.MsgData(importPathRange,
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
				Location:   tracker.MsgLocationOrNil(importPathRange),
				UserDetail: thrown,
			},
		})
	}

	return didLogError
}

func RunOnResolvePlugins(
	plugins []config.Plugin,
	res *resolver.Resolver,
	log logger.Log,
	fs fs.FS,
	fsCache *cache.FSCache,
	importSource *logger.Source,
	importPathRange logger.Range,
	importer logger.Path,
	path string,
	importAttributes logger.ImportAttributes,
	kind ast.ImportKind,
	absResolveDir string,
	pluginData interface{},
) (*resolver.ResolveResult, bool, resolver.DebugMeta) {
	resolverArgs := config.OnResolveArgs{
		Path:       path,
		ResolveDir: absResolveDir,
		Kind:       kind,
		PluginData: pluginData,
		Importer:   importer,
		With:       importAttributes,
	}
	applyPath := logger.Path{
		Text:      path,
		Namespace: importer.Namespace,
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
			didLogError := logPluginMessages(fs, log, pluginName, result.Msgs, result.ThrownError, importSource, importPathRange)

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
					log.AddError(&tracker, importPathRange,
						fmt.Sprintf("Plugin %q returned a path in the \"file\" namespace that is not an absolute path: %s", pluginName, result.Path.Text))
				} else {
					log.AddError(&tracker, importPathRange,
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
				PathPair:               resolver.PathPair{Primary: result.Path, IsExternal: result.External},
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
		log.AddID(logger.MsgID_Bundler_DifferentPathCase, logger.Warning, &tracker, importPathRange, fmt.Sprintf(
			"Use %q instead of %q to avoid issues with case-sensitive file systems",
			resolver.PrettyPath(fs, logger.Path{Text: fs.Join(diffCase.Dir, diffCase.Actual), Namespace: "file"}),
			resolver.PrettyPath(fs, logger.Path{Text: fs.Join(diffCase.Dir, diffCase.Query), Namespace: "file"}),
		))
	}

	return result, false, debug
}

type loaderPluginResult struct {
	pluginData    interface{}
	absResolveDir string
	pluginName    string
	loader        config.Loader
}

func runOnLoadPlugins(
	plugins []config.Plugin,
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
			didLogError := logPluginMessages(fs, log, pluginName, result.Msgs, result.ThrownError, importSource, importPathRange)

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
		return loaderPluginResult{loader: config.LoaderEmpty}, true
	}

	// Read normal modules from disk
	if source.KeyPath.Namespace == "file" {
		if contents, err, _ := fsCache.ReadFile(fs, source.KeyPath.Text); err == nil {
			source.Contents = contents
			return loaderPluginResult{
				loader:        config.LoaderDefault,
				absResolveDir: fs.Dir(source.KeyPath.Text),
			}, true
		} else {
			if err == syscall.ENOENT {
				log.AddError(&tracker, importPathRange,
					fmt.Sprintf("Cannot read file: %s", source.KeyPath.Text))
				return loaderPluginResult{}, false
			} else {
				log.AddError(&tracker, importPathRange,
					fmt.Sprintf("Cannot read file %q: %s", resolver.PrettyPath(fs, source.KeyPath), err.Error()))
				return loaderPluginResult{}, false
			}
		}
	}

	// Native support for data URLs. This is supported natively by node:
	// https://nodejs.org/docs/latest/api/esm.html#esm_data_imports
	if source.KeyPath.Namespace == "dataurl" {
		if parsed, ok := resolver.ParseDataURL(source.KeyPath.Text); ok {
			if contents, err := parsed.DecodeData(); err != nil {
				log.AddError(&tracker, importPathRange,
					fmt.Sprintf("Could not load data URL: %s", err.Error()))
				return loaderPluginResult{loader: config.LoaderNone}, true
			} else {
				source.Contents = contents
				if mimeType := parsed.DecodeMIMEType(); mimeType != resolver.MIMETypeUnsupported {
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

// Identify the path by its lowercase absolute path name with Windows-specific
// slashes substituted for standard slashes. This should hopefully avoid path
// issues on Windows where multiple different paths can refer to the same
// underlying file.
func canonicalFileSystemPathForWindows(absPath string) string {
	return strings.ReplaceAll(strings.ToLower(absPath), "\\", "/")
}

func HashForFileName(hashBytes []byte) string {
	return base32.StdEncoding.EncodeToString(hashBytes)[:8]
}

type scanner struct {
	log             logger.Log
	fs              fs.FS
	res             *resolver.Resolver
	caches          *cache.CacheSet
	timer           *helpers.Timer
	uniqueKeyPrefix string

	// These are not guarded by a mutex because it's only ever modified by a single
	// thread. Note that not all results in the "results" array are necessarily
	// valid. Make sure to check the "ok" flag before using them.
	results       []parseResult
	visited       map[logger.Path]visitedFile
	resultChannel chan parseResult

	options config.Options

	// Also not guarded by a mutex for the same reason
	remaining int
}

type visitedFile struct {
	sourceIndex uint32
}

type EntryPoint struct {
	InputPath                string
	OutputPath               string
	InputPathInFileNamespace bool
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

// This creates a bundle by scanning over the whole module graph starting from
// the entry points until all modules are reached. Each module has some number
// of import paths which are resolved to module identifiers (i.e. "onResolve"
// in the plugin API). Each unique module identifier is loaded once (i.e.
// "onLoad" in the plugin API).
func ScanBundle(
	call config.APICall,
	log logger.Log,
	fs fs.FS,
	caches *cache.CacheSet,
	entryPoints []EntryPoint,
	options config.Options,
	timer *helpers.Timer,
) Bundle {
	timer.Begin("Scan phase")
	defer timer.End("Scan phase")

	applyOptionDefaults(&options)

	// Run "onStart" plugins in parallel. IMPORTANT: We always need to run all
	// "onStart" callbacks even when the build is cancelled, because plugins may
	// rely on invariants that are started in "onStart" and ended in "onEnd".
	// This works because "onEnd" callbacks are always run as well.
	timer.Begin("On-start callbacks")
	onStartWaitGroup := sync.WaitGroup{}
	for _, plugin := range options.Plugins {
		for _, onStart := range plugin.OnStart {
			onStartWaitGroup.Add(1)
			go func(plugin config.Plugin, onStart config.OnStart) {
				result := onStart.Callback()
				logPluginMessages(fs, log, plugin.Name, result.Msgs, result.ThrownError, nil, logger.Range{})
				onStartWaitGroup.Done()
			}(plugin, onStart)
		}
	}

	// Each bundling operation gets a separate unique key
	uniqueKeyPrefix, err := generateUniqueKeyPrefix()
	if err != nil {
		log.AddError(nil, logger.Range{}, fmt.Sprintf("Failed to read from randomness source: %s", err.Error()))
	}

	// This may mutate "options" by the "tsconfig.json" override settings
	res := resolver.NewResolver(call, fs, log, caches, &options)

	s := scanner{
		log:             log,
		fs:              fs,
		res:             res,
		caches:          caches,
		options:         options,
		timer:           timer,
		results:         make([]parseResult, 0, caches.SourceIndexCache.LenHint()),
		visited:         make(map[logger.Path]visitedFile),
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
					Repr: &graph.JSRepr{
						AST: ast,
					},
					OmitFromSourceMapsAndMetafile: true,
				},
			},
			ok: ok,
		}
	}()

	// Wait for all "onStart" plugins here before continuing. People sometimes run
	// setup code in "onStart" that "onLoad" expects to be able to use without
	// "onLoad" needing to block on the completion of their "onStart" callback.
	//
	// We want to enable this:
	//
	//   let plugin = {
	//     name: 'example',
	//     setup(build) {
	//       let started = false
	//       build.onStart(() => started = true)
	//       build.onLoad({ filter: /.*/ }, () => {
	//         assert(started === true)
	//       })
	//     },
	//   }
	//
	// without people having to write something like this:
	//
	//   let plugin = {
	//     name: 'example',
	//     setup(build) {
	//       let started = {}
	//       started.promise = new Promise(resolve => {
	//         started.resolve = resolve
	//       })
	//       build.onStart(() => {
	//         started.resolve(true)
	//       })
	//       build.onLoad({ filter: /.*/ }, async () => {
	//         assert(await started.promise === true)
	//       })
	//     },
	//   }
	//
	onStartWaitGroup.Wait()
	timer.End("On-start callbacks")

	// We can check the cancel flag now that all "onStart" callbacks are done
	if options.CancelFlag.DidCancel() {
		return Bundle{options: options}
	}

	s.preprocessInjectedFiles()

	if options.CancelFlag.DidCancel() {
		return Bundle{options: options}
	}

	entryPointMeta := s.addEntryPoints(entryPoints)

	if options.CancelFlag.DidCancel() {
		return Bundle{options: options}
	}

	s.scanAllDependencies()

	if options.CancelFlag.DidCancel() {
		return Bundle{options: options}
	}

	files := s.processScannedFiles(entryPointMeta)

	if options.CancelFlag.DidCancel() {
		return Bundle{options: options}
	}

	return Bundle{
		fs:              fs,
		res:             s.res,
		files:           files,
		entryPoints:     entryPointMeta,
		uniqueKeyPrefix: uniqueKeyPrefix,
		options:         s.options,
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
	importWith *ast.ImportAssertOrWith,
	kind inputKind,
	inject chan config.InjectedFile,
) uint32 {
	path := resolveResult.PathPair.Primary
	visitedKey := path
	if visitedKey.Namespace == "file" {
		visitedKey.Text = canonicalFileSystemPathForWindows(visitedKey.Text)
	}

	// Only parse a given file path once
	visited, ok := s.visited[visitedKey]
	if ok {
		if inject != nil {
			inject <- config.InjectedFile{}
		}
		return visited.sourceIndex
	}

	visited = visitedFile{
		sourceIndex: s.allocateSourceIndex(visitedKey, cache.SourceIndexNormal),
	}
	s.visited[visitedKey] = visited
	s.remaining++
	optionsClone := s.options
	if kind != inputKindStdin {
		optionsClone.Stdin = nil
	}

	// Allow certain properties to be overridden by "tsconfig.json"
	resolveResult.TSConfigJSX.ApplyTo(&optionsClone.JSX)
	if resolveResult.TSConfig != nil {
		optionsClone.TS.Config = *resolveResult.TSConfig
	}
	if resolveResult.TSAlwaysStrict != nil {
		optionsClone.TSAlwaysStrict = resolveResult.TSAlwaysStrict
	}

	// Set the module type preference using node's module type rules
	if strings.HasSuffix(path.Text, ".mjs") {
		optionsClone.ModuleTypeData.Type = js_ast.ModuleESM_MJS
	} else if strings.HasSuffix(path.Text, ".mts") {
		optionsClone.ModuleTypeData.Type = js_ast.ModuleESM_MTS
	} else if strings.HasSuffix(path.Text, ".cjs") {
		optionsClone.ModuleTypeData.Type = js_ast.ModuleCommonJS_CJS
	} else if strings.HasSuffix(path.Text, ".cts") {
		optionsClone.ModuleTypeData.Type = js_ast.ModuleCommonJS_CTS
	} else if strings.HasSuffix(path.Text, ".js") || strings.HasSuffix(path.Text, ".jsx") ||
		strings.HasSuffix(path.Text, ".ts") || strings.HasSuffix(path.Text, ".tsx") {
		optionsClone.ModuleTypeData = resolveResult.ModuleTypeData
	} else {
		// The "type" setting in "package.json" only applies to ".js" files
		optionsClone.ModuleTypeData.Type = js_ast.ModuleUnknown
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
			if len(prettyPath) > 65 {
				prettyPath = prettyPath[:65]
			}
			prettyPath = strings.ReplaceAll(prettyPath, "\n", "\\n")
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
		sourceIndex:     visited.sourceIndex,
		importSource:    importSource,
		sideEffects:     sideEffects,
		importPathRange: importPathRange,
		importWith:      importWith,
		pluginData:      resolveResult.PluginData,
		options:         optionsClone,
		results:         s.resultChannel,
		inject:          inject,
		skipResolve:     skipResolve,
		uniqueKeyPrefix: s.uniqueKeyPrefix,
	})

	return visited.sourceIndex
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

func (s *scanner) allocateGlobSourceIndex(parentSourceIndex uint32, globIndex uint32) uint32 {
	// Allocate a source index using the shared source index cache so that
	// subsequent builds reuse the same source index and therefore use the
	// cached parse results for increased speed.
	sourceIndex := s.caches.SourceIndexCache.GetGlob(parentSourceIndex, globIndex)

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

	injectedFiles := make([]config.InjectedFile, 0, len(s.options.InjectedDefines)+len(s.options.InjectPaths))

	// These are virtual paths that are generated for compound "--define" values.
	// They are special-cased and are not available for plugins to intercept.
	for _, define := range s.options.InjectedDefines {
		// These should be unique by construction so no need to check for collisions
		visitedKey := logger.Path{Text: fmt.Sprintf("<define:%s>", define.Name)}
		sourceIndex := s.allocateSourceIndex(visitedKey, cache.SourceIndexNormal)
		s.visited[visitedKey] = visitedFile{sourceIndex: sourceIndex}
		source := logger.Source{
			Index:          sourceIndex,
			KeyPath:        visitedKey,
			PrettyPath:     resolver.PrettyPath(s.fs, visitedKey),
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

	// Add user-specified injected files. Run resolver plugins on these files
	// so plugins can alter where they resolve to. These are run in parallel in
	// case any of these plugins block.
	injectResolveResults := make([]*resolver.ResolveResult, len(s.options.InjectPaths))
	injectAbsResolveDir := s.fs.Cwd()
	injectResolveWaitGroup := sync.WaitGroup{}
	injectResolveWaitGroup.Add(len(s.options.InjectPaths))
	for i, importPath := range s.options.InjectPaths {
		go func(i int, importPath string) {
			var importer logger.Path

			// Add a leading "./" if it's missing, similar to entry points
			absPath := importPath
			if !s.fs.IsAbs(absPath) {
				absPath = s.fs.Join(injectAbsResolveDir, absPath)
			}
			dir := s.fs.Dir(absPath)
			base := s.fs.Base(absPath)
			if entries, err, originalError := s.fs.ReadDirectory(dir); err == nil {
				if entry, _ := entries.Get(base); entry != nil && entry.Kind(s.fs) == fs.FileEntry {
					importer.Namespace = "file"
					if !s.fs.IsAbs(importPath) && resolver.IsPackagePath(importPath) {
						importPath = "./" + importPath
					}
				}
			} else if s.log.Level <= logger.LevelDebug && originalError != nil {
				s.log.AddID(logger.MsgID_None, logger.Debug, nil, logger.Range{}, fmt.Sprintf("Failed to read directory %q: %s", absPath, originalError.Error()))
			}

			// Run the resolver and log an error if the path couldn't be resolved
			resolveResult, didLogError, debug := RunOnResolvePlugins(
				s.options.Plugins,
				s.res,
				s.log,
				s.fs,
				&s.caches.FSCache,
				nil,
				logger.Range{},
				importer,
				importPath,
				logger.ImportAttributes{},
				ast.ImportEntryPoint,
				injectAbsResolveDir,
				nil,
			)
			if resolveResult != nil {
				if resolveResult.PathPair.IsExternal {
					s.log.AddError(nil, logger.Range{}, fmt.Sprintf("The injected path %q cannot be marked as external", importPath))
				} else {
					injectResolveResults[i] = resolveResult
				}
			} else if !didLogError {
				debug.LogErrorMsg(s.log, nil, logger.Range{}, fmt.Sprintf("Could not resolve %q", importPath), "", nil)
			}
			injectResolveWaitGroup.Done()
		}(i, importPath)
	}
	injectResolveWaitGroup.Wait()

	if s.options.CancelFlag.DidCancel() {
		return
	}

	// Parse all entry points that were resolved successfully
	results := make([]config.InjectedFile, len(s.options.InjectPaths))
	j := 0
	var injectWaitGroup sync.WaitGroup
	for _, resolveResult := range injectResolveResults {
		if resolveResult != nil {
			channel := make(chan config.InjectedFile, 1)
			s.maybeParseFile(*resolveResult, resolver.PrettyPath(s.fs, resolveResult.PathPair.Primary), nil, logger.Range{}, nil, inputKindNormal, channel)
			injectWaitGroup.Add(1)

			// Wait for the results in parallel. The results slice is large enough so
			// it is not reallocated during the computations.
			go func(i int) {
				results[i] = <-channel
				injectWaitGroup.Done()
			}(j)
			j++
		}
	}
	injectWaitGroup.Wait()
	injectedFiles = append(injectedFiles, results[:j]...)

	// It's safe to mutate the options object to add the injected files here
	// because there aren't any concurrent "parseFile" goroutines at this point.
	// The only ones that were created by this point are the ones we created
	// above, and we've already waited for all of them to finish using the
	// "options" object.
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
		sourceIndex := s.maybeParseFile(resolveResult, resolver.PrettyPath(s.fs, stdinPath), nil, logger.Range{}, nil, inputKindStdin, nil)
		entryMetas = append(entryMetas, graph.EntryPoint{
			OutputPath:  "stdin",
			SourceIndex: sourceIndex,
		})
	}

	if s.options.CancelFlag.DidCancel() {
		return nil
	}

	// Check each entry point ahead of time to see if it's a real file
	entryPointAbsResolveDir := s.fs.Cwd()
	for i := range entryPoints {
		entryPoint := &entryPoints[i]
		absPath := entryPoint.InputPath
		if strings.ContainsRune(absPath, '*') {
			continue // Ignore glob patterns
		}
		if !s.fs.IsAbs(absPath) {
			absPath = s.fs.Join(entryPointAbsResolveDir, absPath)
		}
		dir := s.fs.Dir(absPath)
		base := s.fs.Base(absPath)
		if entries, err, originalError := s.fs.ReadDirectory(dir); err == nil {
			if entry, _ := entries.Get(base); entry != nil && entry.Kind(s.fs) == fs.FileEntry {
				entryPoint.InputPathInFileNamespace = true

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
			s.log.AddID(logger.MsgID_None, logger.Debug, nil, logger.Range{}, fmt.Sprintf("Failed to read directory %q: %s", absPath, originalError.Error()))
		}
	}

	if s.options.CancelFlag.DidCancel() {
		return nil
	}

	// Add any remaining entry points. Run resolver plugins on these entry points
	// so plugins can alter where they resolve to. These are run in parallel in
	// case any of these plugins block.
	type entryPointInfo struct {
		results []resolver.ResolveResult
		isGlob  bool
	}
	entryPointInfos := make([]entryPointInfo, len(entryPoints))
	entryPointWaitGroup := sync.WaitGroup{}
	entryPointWaitGroup.Add(len(entryPoints))
	for i, entryPoint := range entryPoints {
		go func(i int, entryPoint EntryPoint) {
			var importer logger.Path
			if entryPoint.InputPathInFileNamespace {
				importer.Namespace = "file"
			}

			// Special-case glob patterns here
			if strings.ContainsRune(entryPoint.InputPath, '*') {
				if pattern := helpers.ParseGlobPattern(entryPoint.InputPath); len(pattern) > 1 {
					prettyPattern := fmt.Sprintf("%q", entryPoint.InputPath)
					if results, msg := s.res.ResolveGlob(entryPointAbsResolveDir, pattern, ast.ImportEntryPoint, prettyPattern); results != nil {
						keys := make([]string, 0, len(results))
						for key := range results {
							keys = append(keys, key)
						}
						sort.Strings(keys)
						info := entryPointInfo{isGlob: true}
						for _, key := range keys {
							info.results = append(info.results, results[key])
						}
						entryPointInfos[i] = info
						if msg != nil {
							s.log.AddID(msg.ID, msg.Kind, nil, logger.Range{}, msg.Data.Text)
						}
					} else {
						s.log.AddError(nil, logger.Range{}, fmt.Sprintf("Could not resolve %q", entryPoint.InputPath))
					}
					entryPointWaitGroup.Done()
					return
				}
			}

			// Run the resolver and log an error if the path couldn't be resolved
			resolveResult, didLogError, debug := RunOnResolvePlugins(
				s.options.Plugins,
				s.res,
				s.log,
				s.fs,
				&s.caches.FSCache,
				nil,
				logger.Range{},
				importer,
				entryPoint.InputPath,
				logger.ImportAttributes{},
				ast.ImportEntryPoint,
				entryPointAbsResolveDir,
				nil,
			)
			if resolveResult != nil {
				if resolveResult.PathPair.IsExternal {
					s.log.AddError(nil, logger.Range{}, fmt.Sprintf("The entry point %q cannot be marked as external", entryPoint.InputPath))
				} else {
					entryPointInfos[i] = entryPointInfo{results: []resolver.ResolveResult{*resolveResult}}
				}
			} else if !didLogError {
				var notes []logger.MsgData
				if !s.fs.IsAbs(entryPoint.InputPath) {
					if query, _ := s.res.ProbeResolvePackageAsRelative(entryPointAbsResolveDir, entryPoint.InputPath, ast.ImportEntryPoint); query != nil {
						notes = append(notes, logger.MsgData{
							Text: fmt.Sprintf("Use the relative path %q to reference the file %q. "+
								"Without the leading \"./\", the path %q is being interpreted as a package path instead.",
								"./"+entryPoint.InputPath, resolver.PrettyPath(s.fs, query.PathPair.Primary), entryPoint.InputPath),
						})
					}
				}
				debug.LogErrorMsg(s.log, nil, logger.Range{}, fmt.Sprintf("Could not resolve %q", entryPoint.InputPath), "", notes)
			}
			entryPointWaitGroup.Done()
		}(i, entryPoint)
	}
	entryPointWaitGroup.Wait()

	if s.options.CancelFlag.DidCancel() {
		return nil
	}

	// Determine output paths for all entry points that were resolved successfully
	type entryPointToParse struct {
		index int
		parse func() uint32
	}
	var entryPointsToParse []entryPointToParse
	for i, info := range entryPointInfos {
		if info.results == nil {
			continue
		}

		for _, resolveResult := range info.results {
			resolveResult := resolveResult
			prettyPath := resolver.PrettyPath(s.fs, resolveResult.PathPair.Primary)
			outputPath := entryPoints[i].OutputPath
			outputPathWasAutoGenerated := false

			// If the output path is missing, automatically generate one from the input path
			if outputPath == "" {
				if info.isGlob {
					outputPath = prettyPath
				} else {
					outputPath = entryPoints[i].InputPath
				}
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
			}

			// Defer parsing for this entry point until later
			entryPointsToParse = append(entryPointsToParse, entryPointToParse{
				index: len(entryMetas),
				parse: func() uint32 {
					return s.maybeParseFile(resolveResult, prettyPath, nil, logger.Range{}, nil, inputKindEntryPoint, nil)
				},
			})

			entryMetas = append(entryMetas, graph.EntryPoint{
				OutputPath:                 outputPath,
				SourceIndex:                ast.InvalidRef.SourceIndex,
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

	// Only parse entry points after "AbsOutputBase" has been determined
	for _, toParse := range entryPointsToParse {
		entryMetas[toParse.index].SourceIndex = toParse.parse()
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

				// Strip the file extension from the output path if there is one so the
				// "out extension" setting is used instead
				if last := strings.LastIndexAny(entryPoint.OutputPath, "/.\\"); last != -1 && entryPoint.OutputPath[last] == '.' {
					entryPoint.OutputPath = entryPoint.OutputPath[:last]
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
				// If we're at the top-level directory, then keep the slash
				if lastSlash < len(absDir) && !strings.ContainsAny(absDir[:lastSlash], "\\/") {
					lastSlash++
				}

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
		if s.options.CancelFlag.DidCancel() {
			return
		}

		result := <-s.resultChannel
		s.remaining--
		if !result.ok {
			continue
		}

		// Don't try to resolve paths if we're not bundling
		if recordsPtr := result.file.inputFile.Repr.ImportRecords(); s.options.Mode == config.ModeBundle && recordsPtr != nil {
			records := *recordsPtr
			for importRecordIndex := range records {
				record := &records[importRecordIndex]

				// This is used for error messages
				var with *ast.ImportAssertOrWith
				if record.AssertOrWith != nil && record.AssertOrWith.Keyword == ast.WithKeyword {
					with = record.AssertOrWith
				}

				// Skip this import record if the previous resolver call failed
				resolveResult := result.resolveResults[importRecordIndex]
				if resolveResult == nil {
					if globResults := result.globResolveResults[uint32(importRecordIndex)]; globResults.resolveResults != nil {
						sourceIndex := s.allocateGlobSourceIndex(result.file.inputFile.Source.Index, uint32(importRecordIndex))
						record.SourceIndex = ast.MakeIndex32(sourceIndex)
						s.results[sourceIndex] = s.generateResultForGlobResolve(sourceIndex, globResults.absPath,
							&result.file.inputFile.Source, record.Range, with, record.GlobPattern.Kind, globResults, record.AssertOrWith)
					}
					continue
				}

				path := resolveResult.PathPair.Primary
				if !resolveResult.PathPair.IsExternal {
					// Handle a path within the bundle
					sourceIndex := s.maybeParseFile(*resolveResult, resolver.PrettyPath(s.fs, path),
						&result.file.inputFile.Source, record.Range, with, inputKindNormal, nil)
					record.SourceIndex = ast.MakeIndex32(sourceIndex)
				} else {
					// Allow this import statement to be removed if something marked it as "sideEffects: false"
					if resolveResult.PrimarySideEffectsData != nil {
						record.Flags |= ast.IsExternalWithoutSideEffects
					}

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

func (s *scanner) generateResultForGlobResolve(
	sourceIndex uint32,
	fakeSourcePath string,
	importSource *logger.Source,
	importRange logger.Range,
	importWith *ast.ImportAssertOrWith,
	kind ast.ImportKind,
	result globResolveResult,
	assertions *ast.ImportAssertOrWith,
) parseResult {
	keys := make([]string, 0, len(result.resolveResults))
	for key := range result.resolveResults {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	object := js_ast.EObject{Properties: make([]js_ast.Property, 0, len(result.resolveResults))}
	importRecords := make([]ast.ImportRecord, 0, len(result.resolveResults))
	resolveResults := make([]*resolver.ResolveResult, 0, len(result.resolveResults))

	for _, key := range keys {
		resolveResult := result.resolveResults[key]
		var value js_ast.Expr

		importRecordIndex := uint32(len(importRecords))
		var sourceIndex ast.Index32

		if !resolveResult.PathPair.IsExternal {
			sourceIndex = ast.MakeIndex32(s.maybeParseFile(
				resolveResult,
				resolver.PrettyPath(s.fs, resolveResult.PathPair.Primary),
				importSource,
				importRange,
				importWith,
				inputKindNormal,
				nil,
			))
		}

		path := resolveResult.PathPair.Primary

		// If the path to the external module is relative to the source
		// file, rewrite the path to be relative to the working directory
		if path.Namespace == "file" {
			if relPath, ok := s.fs.Rel(s.options.AbsOutputDir, path.Text); ok {
				// Prevent issues with path separators being different on Windows
				relPath = strings.ReplaceAll(relPath, "\\", "/")
				if resolver.IsPackagePath(relPath) {
					relPath = "./" + relPath
				}
				path.Text = relPath
			}
		}

		resolveResults = append(resolveResults, &resolveResult)
		importRecords = append(importRecords, ast.ImportRecord{
			Path:         path,
			SourceIndex:  sourceIndex,
			AssertOrWith: assertions,
			Kind:         kind,
		})

		switch kind {
		case ast.ImportDynamic:
			value.Data = &js_ast.EImportString{ImportRecordIndex: importRecordIndex}
		case ast.ImportRequire:
			value.Data = &js_ast.ERequireString{ImportRecordIndex: importRecordIndex}
		default:
			panic("Internal error")
		}

		object.Properties = append(object.Properties, js_ast.Property{
			Key: js_ast.Expr{Data: &js_ast.EString{Value: helpers.StringToUTF16(key)}},
			ValueOrNil: js_ast.Expr{Data: &js_ast.EArrow{
				Body:       js_ast.FnBody{Block: js_ast.SBlock{Stmts: []js_ast.Stmt{{Data: &js_ast.SReturn{ValueOrNil: value}}}}},
				PreferExpr: true,
			}},
		})
	}

	source := logger.Source{
		KeyPath:    logger.Path{Text: fakeSourcePath, Namespace: "file"},
		PrettyPath: result.prettyPath,
		Index:      sourceIndex,
	}
	ast := js_parser.GlobResolveAST(s.log, source, importRecords, &object, result.exportAlias)

	// Fill out "nil" for any additional imports (i.e. from the runtime)
	for len(resolveResults) < len(ast.ImportRecords) {
		resolveResults = append(resolveResults, nil)
	}

	return parseResult{
		resolveResults: resolveResults,
		file: scannerFile{
			inputFile: graph.InputFile{
				Source: source,
				Repr: &graph.JSRepr{
					AST: ast,
				},
				OmitFromSourceMapsAndMetafile: true,
			},
		},
		ok: true,
	}
}

func (s *scanner) processScannedFiles(entryPointMeta []graph.EntryPoint) []scannerFile {
	s.timer.Begin("Process scanned files")
	defer s.timer.End("Process scanned files")

	// Build a set of entry point source indices for quick lookup
	entryPointSourceIndexToMetaIndex := make(map[uint32]uint32, len(entryPointMeta))
	for i, meta := range entryPointMeta {
		entryPointSourceIndexToMetaIndex[meta.SourceIndex] = uint32(i)
	}

	// Check for pretty-printed path collisions
	importAttributeNameCollisions := make(map[string][]uint32)
	for sourceIndex := range s.results {
		if result := &s.results[sourceIndex]; result.ok {
			prettyPath := result.file.inputFile.Source.PrettyPath
			importAttributeNameCollisions[prettyPath] = append(importAttributeNameCollisions[prettyPath], uint32(sourceIndex))
		}
	}

	// Import attributes can result in the same file being imported multiple
	// times in different ways. If that happens, append the import attributes
	// to the pretty-printed file names to disambiguate them. This renaming
	// must happen before we construct the metafile JSON chunks below.
	for _, sourceIndices := range importAttributeNameCollisions {
		if len(sourceIndices) == 1 {
			continue
		}

		for _, sourceIndex := range sourceIndices {
			source := &s.results[sourceIndex].file.inputFile.Source
			attrs := source.KeyPath.ImportAttributes.DecodeIntoArray()
			if len(attrs) == 0 {
				continue
			}

			var sb strings.Builder
			sb.WriteString(" with {")
			for i, attr := range attrs {
				if i > 0 {
					sb.WriteByte(',')
				}
				sb.WriteByte(' ')
				if js_ast.IsIdentifier(attr.Key) {
					sb.WriteString(attr.Key)
				} else {
					sb.Write(helpers.QuoteSingle(attr.Key, false))
				}
				sb.WriteString(": ")
				sb.Write(helpers.QuoteSingle(attr.Value, false))
			}
			sb.WriteString(" }")
			source.PrettyPath += sb.String()
		}
	}

	// Now that all files have been scanned, process the final file import records
	for sourceIndex, result := range s.results {
		if !result.ok {
			continue
		}

		sb := strings.Builder{}
		isFirstImport := true

		// Begin the metadata chunk
		if s.options.NeedsMetafile {
			sb.Write(helpers.QuoteForJSON(result.file.inputFile.Source.PrettyPath, s.options.ASCIIOnly))
			sb.WriteString(fmt.Sprintf(": {\n      \"bytes\": %d,\n      \"imports\": [", len(result.file.inputFile.Source.Contents)))
		}

		// Don't try to resolve paths if we're not bundling
		if recordsPtr := result.file.inputFile.Repr.ImportRecords(); s.options.Mode == config.ModeBundle && recordsPtr != nil {
			records := *recordsPtr
			tracker := logger.MakeLineColumnTracker(&result.file.inputFile.Source)

			for importRecordIndex := range records {
				record := &records[importRecordIndex]

				// Save the import attributes to the metafile
				var metafileWith string
				if s.options.NeedsMetafile {
					if with := record.AssertOrWith; with != nil && with.Keyword == ast.WithKeyword && len(with.Entries) > 0 {
						data := strings.Builder{}
						data.WriteString(",\n          \"with\": {")
						for i, entry := range with.Entries {
							if i > 0 {
								data.WriteByte(',')
							}
							data.WriteString("\n            ")
							data.Write(helpers.QuoteForJSON(helpers.UTF16ToString(entry.Key), s.options.ASCIIOnly))
							data.WriteString(": ")
							data.Write(helpers.QuoteForJSON(helpers.UTF16ToString(entry.Value), s.options.ASCIIOnly))
						}
						data.WriteString("\n          }")
						metafileWith = data.String()
					}
				}

				// Skip this import record if the previous resolver call failed
				resolveResult := result.resolveResults[importRecordIndex]
				if resolveResult == nil || !record.SourceIndex.IsValid() {
					if s.options.NeedsMetafile {
						if isFirstImport {
							isFirstImport = false
							sb.WriteString("\n        ")
						} else {
							sb.WriteString(",\n        ")
						}
						sb.WriteString(fmt.Sprintf("{\n          \"path\": %s,\n          \"kind\": %s,\n          \"external\": true%s\n        }",
							helpers.QuoteForJSON(record.Path.Text, s.options.ASCIIOnly),
							helpers.QuoteForJSON(record.Kind.StringForMetafile(), s.options.ASCIIOnly),
							metafileWith))
					}
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
					if secondaryVisited, ok := s.visited[secondaryKey]; ok {
						record.SourceIndex = ast.MakeIndex32(secondaryVisited.sourceIndex)
					}
				}

				// Generate metadata about each import
				otherResult := &s.results[record.SourceIndex.GetIndex()]
				otherFile := &otherResult.file
				if s.options.NeedsMetafile {
					if isFirstImport {
						isFirstImport = false
						sb.WriteString("\n        ")
					} else {
						sb.WriteString(",\n        ")
					}
					sb.WriteString(fmt.Sprintf("{\n          \"path\": %s,\n          \"kind\": %s,\n          \"original\": %s%s\n        }",
						helpers.QuoteForJSON(otherFile.inputFile.Source.PrettyPath, s.options.ASCIIOnly),
						helpers.QuoteForJSON(record.Kind.StringForMetafile(), s.options.ASCIIOnly),
						helpers.QuoteForJSON(record.Path.Text, s.options.ASCIIOnly),
						metafileWith))
				}

				// Validate that imports with "assert { type: 'json' }" were imported
				// with the JSON loader. This is done to match the behavior of these
				// import assertions in a real JavaScript runtime. In addition, we also
				// allow the copy loader since this is sort of like marking the path
				// as external (the import assertions are kept and the real JavaScript
				// runtime evaluates them, not us).
				if record.Flags.Has(ast.AssertTypeJSON) && otherResult.ok && otherFile.inputFile.Loader != config.LoaderJSON && otherFile.inputFile.Loader != config.LoaderCopy {
					s.log.AddErrorWithNotes(&tracker, record.Range,
						fmt.Sprintf("The file %q was loaded with the %q loader", otherFile.inputFile.Source.PrettyPath, config.LoaderToString[otherFile.inputFile.Loader]),
						[]logger.MsgData{
							tracker.MsgData(js_lexer.RangeOfImportAssertOrWith(result.file.inputFile.Source,
								*ast.FindAssertOrWithEntry(record.AssertOrWith.Entries, "type"), js_lexer.KeyAndValueRange),
								"This import assertion requires the loader to be \"json\" instead:"),
							{Text: "You need to either reconfigure esbuild to ensure that the loader for this file is \"json\" or you need to remove this import assertion."}})
				}

				switch record.Kind {
				case ast.ImportComposesFrom:
					// Using a JavaScript file with CSS "composes" is not allowed
					if _, ok := otherFile.inputFile.Repr.(*graph.JSRepr); ok && otherFile.inputFile.Loader != config.LoaderEmpty {
						s.log.AddErrorWithNotes(&tracker, record.Range,
							fmt.Sprintf("Cannot use \"composes\" with %q", otherFile.inputFile.Source.PrettyPath),
							[]logger.MsgData{{Text: fmt.Sprintf(
								"You can only use \"composes\" with CSS files and %q is not a CSS file (it was loaded with the %q loader).",
								otherFile.inputFile.Source.PrettyPath, config.LoaderToString[otherFile.inputFile.Loader])}})
					}

				case ast.ImportAt:
					// Using a JavaScript file with CSS "@import" is not allowed
					if _, ok := otherFile.inputFile.Repr.(*graph.JSRepr); ok && otherFile.inputFile.Loader != config.LoaderEmpty {
						s.log.AddErrorWithNotes(&tracker, record.Range,
							fmt.Sprintf("Cannot import %q into a CSS file", otherFile.inputFile.Source.PrettyPath),
							[]logger.MsgData{{Text: fmt.Sprintf(
								"An \"@import\" rule can only be used to import another CSS file and %q is not a CSS file (it was loaded with the %q loader).",
								otherFile.inputFile.Source.PrettyPath, config.LoaderToString[otherFile.inputFile.Loader])}})
					}

				case ast.ImportURL:
					// Using a JavaScript or CSS file with CSS "url()" is not allowed
					switch otherRepr := otherFile.inputFile.Repr.(type) {
					case *graph.CSSRepr:
						s.log.AddErrorWithNotes(&tracker, record.Range,
							fmt.Sprintf("Cannot use %q as a URL", otherFile.inputFile.Source.PrettyPath),
							[]logger.MsgData{{Text: fmt.Sprintf(
								"You can't use a \"url()\" token to reference a CSS file, and %q is a CSS file (it was loaded with the %q loader).",
								otherFile.inputFile.Source.PrettyPath, config.LoaderToString[otherFile.inputFile.Loader])}})

					case *graph.JSRepr:
						if otherRepr.AST.URLForCSS == "" && otherFile.inputFile.Loader != config.LoaderEmpty {
							s.log.AddErrorWithNotes(&tracker, record.Range,
								fmt.Sprintf("Cannot use %q as a URL", otherFile.inputFile.Source.PrettyPath),
								[]logger.MsgData{{Text: fmt.Sprintf(
									"You can't use a \"url()\" token to reference the file %q because it was loaded with the %q loader, which doesn't provide a URL to embed in the resulting CSS.",
									otherFile.inputFile.Source.PrettyPath, config.LoaderToString[otherFile.inputFile.Loader])}})
						}
					}
				}

				// If the imported file uses the "copy" loader, then move it from
				// "SourceIndex" to "CopySourceIndex" so we don't end up bundling it.
				if _, ok := otherFile.inputFile.Repr.(*graph.CopyRepr); ok {
					record.CopySourceIndex = record.SourceIndex
					record.SourceIndex = ast.Index32{}
					continue
				}

				// If an import from a JavaScript file targets a CSS file, generate a
				// JavaScript stub to ensure that JavaScript files only ever import
				// other JavaScript files.
				if _, ok := result.file.inputFile.Repr.(*graph.JSRepr); ok {
					if css, ok := otherFile.inputFile.Repr.(*graph.CSSRepr); ok {
						if s.options.WriteToStdout {
							s.log.AddError(&tracker, record.Range,
								fmt.Sprintf("Cannot import %q into a JavaScript file without an output path configured", otherFile.inputFile.Source.PrettyPath))
						} else if !css.JSSourceIndex.IsValid() {
							stubKey := otherFile.inputFile.Source.KeyPath
							if stubKey.Namespace == "file" {
								stubKey.Text = canonicalFileSystemPathForWindows(stubKey.Text)
							}
							sourceIndex := s.allocateSourceIndex(stubKey, cache.SourceIndexJSStubForCSS)
							source := otherFile.inputFile.Source
							source.Index = sourceIndex
							s.results[sourceIndex] = parseResult{
								file: scannerFile{
									inputFile: graph.InputFile{
										Source: source,
										Loader: otherFile.inputFile.Loader,
										Repr: &graph.JSRepr{
											// Note: The actual export object will be filled in by the linker
											AST: js_parser.LazyExportAST(s.log, source,
												js_parser.OptionsFromConfig(&s.options), js_ast.Expr{Data: js_ast.ENullShared}, ""),
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
				if record.Flags.Has(ast.WasOriginallyBareImport) && !s.options.IgnoreDCEAnnotations &&
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
									text = "It was excluded from the \"sideEffects\" array in the enclosing \"package.json\" file:"
								} else {
									text = "\"sideEffects\" is false in the enclosing \"package.json\" file:"
								}
								tracker := logger.MakeLineColumnTracker(data.Source)
								notes = append(notes, tracker.MsgData(data.Range, text))
							}
						}
						s.log.AddIDWithNotes(logger.MsgID_Bundler_IgnoredBareImport, logger.Warning, &tracker, record.Range,
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
			if repr, ok := result.file.inputFile.Repr.(*graph.JSRepr); ok &&
				(repr.AST.ExportsKind == js_ast.ExportsCommonJS || repr.AST.ExportsKind == js_ast.ExportsESM) {
				format := "cjs"
				if repr.AST.ExportsKind == js_ast.ExportsESM {
					format = "esm"
				}
				sb.WriteString(fmt.Sprintf("],\n      \"format\": %q", format))
			} else {
				sb.WriteString("]")
			}
			if attrs := result.file.inputFile.Source.KeyPath.ImportAttributes.DecodeIntoArray(); len(attrs) > 0 {
				sb.WriteString(",\n      \"with\": {")
				for i, attr := range attrs {
					if i > 0 {
						sb.WriteByte(',')
					}
					sb.WriteString(fmt.Sprintf("\n        %s: %s",
						helpers.QuoteForJSON(attr.Key, s.options.ASCIIOnly),
						helpers.QuoteForJSON(attr.Value, s.options.ASCIIOnly),
					))
				}
				sb.WriteString("\n      }")
			}
			sb.WriteString("\n    }")
		}

		result.file.jsonMetadataChunk = sb.String()

		// If this file is from the "file" or "copy" loaders, generate an additional file
		if result.file.inputFile.UniqueKeyForAdditionalFile != "" {
			bytes := []byte(result.file.inputFile.Source.Contents)
			template := s.options.AssetPathTemplate

			// Use the entry path template instead of the asset path template if this
			// file is an entry point and uses the "copy" loader. With the "file" loader
			// the JS stub is the entry point, but with the "copy" loader the file is
			// the entry point itself.
			customFilePath := ""
			useOutputFile := false
			isEntryPoint := false
			if result.file.inputFile.Loader == config.LoaderCopy {
				if metaIndex, ok := entryPointSourceIndexToMetaIndex[uint32(sourceIndex)]; ok {
					template = s.options.EntryPathTemplate
					customFilePath = entryPointMeta[metaIndex].OutputPath
					useOutputFile = s.options.AbsOutputFile != ""
					isEntryPoint = true
				}
			}

			// Add a hash to the file name to prevent multiple files with the same name
			// but different contents from colliding
			var hash string
			if config.HasPlaceholder(template, config.HashPlaceholder) {
				h := xxhash.New()
				h.Write(bytes)
				hash = HashForFileName(h.Sum(nil))
			}

			// This should use similar logic to how the linker computes output paths
			var dir, base, ext string
			if useOutputFile {
				// If the output path was configured explicitly, use it verbatim
				dir = "/"
				base = s.fs.Base(s.options.AbsOutputFile)
				ext = s.fs.Ext(base)
				base = base[:len(base)-len(ext)]
			} else {
				// Otherwise, derive the output path from the input path
				// Generate the input for the template
				_, _, originalExt := logger.PlatformIndependentPathDirBaseExt(result.file.inputFile.Source.KeyPath.Text)
				dir, base = PathRelativeToOutbase(
					&result.file.inputFile,
					&s.options,
					s.fs,
					/* avoidIndex */ false,
					customFilePath,
				)
				ext = originalExt
			}

			// Apply the path template
			templateExt := strings.TrimPrefix(ext, ".")
			relPath := config.TemplateToString(config.SubstituteTemplate(template, config.PathPlaceholders{
				Dir:  &dir,
				Name: &base,
				Hash: &hash,
				Ext:  &templateExt,
			})) + ext

			// Optionally add metadata about the file
			var jsonMetadataChunk string
			if s.options.NeedsMetafile {
				inputs := fmt.Sprintf("{\n        %s: {\n          \"bytesInOutput\": %d\n        }\n      }",
					helpers.QuoteForJSON(result.file.inputFile.Source.PrettyPath, s.options.ASCIIOnly),
					len(bytes),
				)
				entryPointJSON := ""
				if isEntryPoint {
					entryPointJSON = fmt.Sprintf("\"entryPoint\": %s,\n      ",
						helpers.QuoteForJSON(result.file.inputFile.Source.PrettyPath, s.options.ASCIIOnly))
				}
				jsonMetadataChunk = fmt.Sprintf(
					"{\n      \"imports\": [],\n      \"exports\": [],\n      %s\"inputs\": %s,\n      \"bytes\": %d\n    }",
					entryPointJSON,
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

		s.results[sourceIndex] = result
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
			if repr.AST.LiveTopLevelAwaitKeyword.Len > 0 {
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

							if parentRepr.AST.LiveTopLevelAwaitKeyword.Len > 0 {
								tlaPrettyPath = parentResult.file.inputFile.Source.PrettyPath
								tracker := logger.MakeLineColumnTracker(&parentResult.file.inputFile.Source)
								notes = append(notes, tracker.MsgData(parentRepr.AST.LiveTopLevelAwaitKeyword,
									fmt.Sprintf("The top-level await in %q is here:", tlaPrettyPath)))
								break
							}

							if !parentResult.tlaCheck.parent.IsValid() {
								notes = append(notes, logger.MsgData{Text: "unexpected invalid index"})
								break
							}

							otherSourceIndex = parentResult.tlaCheck.parent.GetIndex()

							tracker := logger.MakeLineColumnTracker(&parentResult.file.inputFile.Source)
							notes = append(notes, tracker.MsgData(
								parentRepr.AST.ImportRecords[parentResult.tlaCheck.importRecordIndex].Range,
								fmt.Sprintf("The file %q imports the file %q here:",
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
						s.log.AddErrorWithNotes(&tracker, record.Range, text, notes)
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
		"":            config.LoaderJS, // This represents files without an extension
		".js":         config.LoaderJS,
		".mjs":        config.LoaderJS,
		".cjs":        config.LoaderJS,
		".jsx":        config.LoaderJSX,
		".ts":         config.LoaderTS,
		".cts":        config.LoaderTSNoAmbiguousLessThan,
		".mts":        config.LoaderTSNoAmbiguousLessThan,
		".tsx":        config.LoaderTSX,
		".css":        config.LoaderCSS,
		".module.css": config.LoaderLocalCSS,
		".json":       config.LoaderJSON,
		".txt":        config.LoaderText,
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

	// Automatically fix invalid configurations of unsupported features
	fixInvalidUnsupportedJSFeatureOverrides(options, compat.AsyncAwait, compat.AsyncGenerator|compat.ForAwait|compat.TopLevelAwait)
	fixInvalidUnsupportedJSFeatureOverrides(options, compat.Generator, compat.AsyncGenerator)
	fixInvalidUnsupportedJSFeatureOverrides(options, compat.ObjectAccessors, compat.ClassPrivateAccessor|compat.ClassPrivateStaticAccessor)
	fixInvalidUnsupportedJSFeatureOverrides(options, compat.ClassField, compat.ClassPrivateField)
	fixInvalidUnsupportedJSFeatureOverrides(options, compat.ClassStaticField, compat.ClassPrivateStaticField)
	fixInvalidUnsupportedJSFeatureOverrides(options, compat.Class,
		compat.ClassField|compat.ClassPrivateAccessor|compat.ClassPrivateBrandCheck|compat.ClassPrivateField|
			compat.ClassPrivateMethod|compat.ClassPrivateStaticAccessor|compat.ClassPrivateStaticField|
			compat.ClassPrivateStaticMethod|compat.ClassStaticBlocks|compat.ClassStaticField)

	// If we're not building for the browser, automatically disable support for
	// inline </script> and </style> tags if there aren't currently any overrides
	if options.Platform != config.PlatformBrowser {
		if !options.UnsupportedJSFeatureOverridesMask.Has(compat.InlineScript) {
			options.UnsupportedJSFeatures |= compat.InlineScript
		}
		if !options.UnsupportedCSSFeatureOverridesMask.Has(compat.InlineStyle) {
			options.UnsupportedCSSFeatures |= compat.InlineStyle
		}
	}
}

func fixInvalidUnsupportedJSFeatureOverrides(options *config.Options, implies compat.JSFeature, implied compat.JSFeature) {
	// If this feature is unsupported, that implies that the other features must also be unsupported
	if options.UnsupportedJSFeatureOverrides.Has(implies) {
		options.UnsupportedJSFeatures |= implied
		options.UnsupportedJSFeatureOverrides |= implied
		options.UnsupportedJSFeatureOverridesMask |= implied
	}
}

type Linker func(
	options *config.Options,
	timer *helpers.Timer,
	log logger.Log,
	fs fs.FS,
	res *resolver.Resolver,
	inputFiles []graph.InputFile,
	entryPoints []graph.EntryPoint,
	uniqueKeyPrefix string,
	reachableFiles []uint32,
	dataForSourceMaps func() []DataForSourceMap,
) []graph.OutputFile

func (b *Bundle) Compile(log logger.Log, timer *helpers.Timer, mangleCache map[string]interface{}, link Linker) ([]graph.OutputFile, string) {
	timer.Begin("Compile phase")
	defer timer.End("Compile phase")

	if b.options.CancelFlag.DidCancel() {
		return nil, ""
	}

	options := b.options

	// In most cases we don't need synchronized access to the mangle cache
	cssUsedLocalNames := make(map[string]bool)
	options.ExclusiveMangleCacheUpdate = func(cb func(
		mangleCache map[string]interface{},
		cssUsedLocalNames map[string]bool,
	)) {
		cb(mangleCache, cssUsedLocalNames)
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
		resultGroups = [][]graph.OutputFile{link(&options, timer, log, b.fs, b.res,
			files, b.entryPoints, b.uniqueKeyPrefix, allReachableFiles, dataForSourceMaps)}
	} else {
		// Otherwise, link each entry point with the runtime file separately
		waitGroup := sync.WaitGroup{}
		resultGroups = make([][]graph.OutputFile, len(b.entryPoints))
		serializer := helpers.MakeSerializer(len(b.entryPoints))
		for i, entryPoint := range b.entryPoints {
			waitGroup.Add(1)
			go func(i int, entryPoint graph.EntryPoint) {
				entryPoints := []graph.EntryPoint{entryPoint}
				forked := timer.Fork()

				// Each goroutine needs a separate options object
				optionsClone := options
				optionsClone.ExclusiveMangleCacheUpdate = func(cb func(
					mangleCache map[string]interface{},
					cssUsedLocalNames map[string]bool,
				)) {
					// Serialize all accesses to the mangle cache in entry point order for determinism
					serializer.Enter(i)
					defer serializer.Leave(i)
					cb(mangleCache, cssUsedLocalNames)
				}

				resultGroups[i] = link(&optionsClone, forked, log, b.fs, b.res, files, entryPoints,
					b.uniqueKeyPrefix, findReachableFiles(files, entryPoints), dataForSourceMaps)
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
					log.AddError(nil, logger.Range{},
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
			log.AddError(nil, logger.Range{}, "Two output files share the same path but have different contents: "+outputPath)
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
			if recordsPtr := file.Repr.ImportRecords(); recordsPtr != nil {
				for _, record := range *recordsPtr {
					if record.SourceIndex.IsValid() {
						visit(record.SourceIndex.GetIndex())
					} else if record.CopySourceIndex.IsValid() {
						visit(record.CopySourceIndex.GetIndex())
					}
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
func (b *Bundle) computeDataForSourceMapsInParallel(options *config.Options, reachableFiles []uint32) func() []DataForSourceMap {
	if options.SourceMap == config.SourceMapNone {
		return func() []DataForSourceMap {
			return nil
		}
	}

	var waitGroup sync.WaitGroup
	results := make([]DataForSourceMap, len(b.files))

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
				result.LineOffsetTables = sourcemap.GenerateLineOffsetTables(f.inputFile.Source.Contents, approximateLineCount)
				sm := f.inputFile.InputSourceMap
				if !options.ExcludeSourcesContent {
					if sm == nil {
						// Simple case: no nested source map
						result.QuotedContents = [][]byte{helpers.QuoteForJSON(f.inputFile.Source.Contents, options.ASCIIOnly)}
					} else {
						// Complex case: nested source map
						result.QuotedContents = make([][]byte, len(sm.Sources))
						nullContents := []byte("null")
						for i := range sm.Sources {
							// Missing contents become a "null" literal
							quotedContents := nullContents
							if i < len(sm.SourcesContent) {
								if value := sm.SourcesContent[i]; value.Quoted != "" && (!options.ASCIIOnly || !isASCIIOnly(value.Quoted)) {
									// Just use the value directly from the input file
									quotedContents = []byte(value.Quoted)
								} else if value.Value != nil {
									// Re-quote non-ASCII values if output is ASCII-only.
									// Also quote values that haven't been quoted yet
									// (happens when the entire "sourcesContent" array is
									// absent and the source has been found on the file
									// system using the "sources" array).
									quotedContents = helpers.QuoteForJSON(helpers.UTF16ToString(value.Value), options.ASCIIOnly)
								}
							}
							result.QuotedContents[i] = quotedContents
						}
					}
				}
				waitGroup.Done()
			}(sourceIndex, f, approximateLineCount)
		}
	}

	return func() []DataForSourceMap {
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
		if b.files[sourceIndex].inputFile.OmitFromSourceMapsAndMetafile {
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
			path := resolver.PrettyPath(b.fs, logger.Path{Text: result.AbsPath, Namespace: "file"})
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
			sb.WriteString(fmt.Sprintf("%s: ", helpers.QuoteForJSON(path, asciiOnly)))
			sb.WriteString(result.JSONMetadataChunk)
		}
	}

	sb.WriteString("\n  }\n}\n")
	return sb.String()
}

type runtimeCacheKey struct {
	unsupportedJSFeatures compat.JSFeature
	minifySyntax          bool
	minifyIdentifiers     bool
}

type runtimeCache struct {
	astMap   map[runtimeCacheKey]js_ast.AST
	astMutex sync.Mutex
}

var globalRuntimeCache runtimeCache

func (cache *runtimeCache) parseRuntime(options *config.Options) (source logger.Source, runtimeAST js_ast.AST, ok bool) {
	key := runtimeCacheKey{
		// All configuration options that the runtime code depends on must go here
		unsupportedJSFeatures: options.UnsupportedJSFeatures,
		minifySyntax:          options.MinifySyntax,
		minifyIdentifiers:     options.MinifyIdentifiers,
	}

	// Determine which source to use
	source = runtime.Source(key.unsupportedJSFeatures)

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
	log := logger.NewDeferLog(logger.DeferLogAll, nil)
	runtimeAST, ok = js_parser.Parse(log, source, js_parser.OptionsFromConfig(&config.Options{
		// These configuration options must only depend on the key
		UnsupportedJSFeatures: key.unsupportedJSFeatures,
		MinifySyntax:          key.minifySyntax,
		MinifyIdentifiers:     key.minifyIdentifiers,

		// Always do tree shaking for the runtime because we never want to
		// include unnecessary runtime code
		TreeShaking: true,
	}))
	if log.HasErrors() {
		msgs := "Internal error: failed to parse runtime:\n"
		for _, msg := range log.Done() {
			msgs += msg.String(logger.OutputOptions{IncludeSource: true}, logger.TerminalInfo{})
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

// Returns the path of this file relative to "outbase", which is then ready to
// be joined with the absolute output directory path. The directory and name
// components are returned separately for convenience.
func PathRelativeToOutbase(
	inputFile *graph.InputFile,
	options *config.Options,
	fs fs.FS,
	avoidIndex bool,
	customFilePath string,
) (relDir string, baseName string) {
	relDir = "/"
	absPath := inputFile.Source.KeyPath.Text

	if customFilePath != "" {
		// Use the configured output path if present
		absPath = customFilePath
		if !fs.IsAbs(absPath) {
			absPath = fs.Join(options.AbsOutputBase, absPath)
		}
	} else if inputFile.Source.KeyPath.Namespace != "file" {
		// Come up with a path for virtual paths (i.e. non-file-system paths)
		dir, base, _ := logger.PlatformIndependentPathDirBaseExt(absPath)
		if avoidIndex && base == "index" {
			_, base, _ = logger.PlatformIndependentPathDirBaseExt(dir)
		}
		baseName = sanitizeFilePathForVirtualModulePath(base)
		return
	} else {
		// Heuristic: If the file is named something like "index.js", then use
		// the name of the parent directory instead. This helps avoid the
		// situation where many chunks are named "index" because of people
		// dynamically-importing npm packages that make use of node's implicit
		// "index" file name feature.
		if avoidIndex {
			base := fs.Base(absPath)
			base = base[:len(base)-len(fs.Ext(base))]
			if base == "index" {
				absPath = fs.Dir(absPath)
			}
		}
	}

	// Try to get a relative path to the base directory
	relPath, ok := fs.Rel(options.AbsOutputBase, absPath)
	if !ok {
		// This can fail in some situations such as on different drives on
		// Windows. In that case we just use the file name.
		baseName = fs.Base(absPath)
	} else {
		// Now we finally have a relative path
		relDir = fs.Dir(relPath) + "/"
		baseName = fs.Base(relPath)

		// Use platform-independent slashes
		relDir = strings.ReplaceAll(relDir, "\\", "/")

		// Replace leading "../" so we don't try to write outside of the output
		// directory. This normally can't happen because "AbsOutputBase" is
		// automatically computed to contain all entry point files, but it can
		// happen if someone sets it manually via the "outbase" API option.
		//
		// Note that we can't just strip any leading "../" because that could
		// cause two separate entry point paths to collide. For example, there
		// could be both "src/index.js" and "../src/index.js" as entry points.
		dotDotCount := 0
		for strings.HasPrefix(relDir[dotDotCount*3:], "../") {
			dotDotCount++
		}
		if dotDotCount > 0 {
			// The use of "_.._" here is somewhat arbitrary but it is unlikely to
			// collide with a folder named by a human and it works on Windows
			// (Windows doesn't like names that end with a "."). And not starting
			// with a "." means that it will not be hidden on Unix.
			relDir = strings.Repeat("_.._/", dotDotCount) + relDir[dotDotCount*3:]
		}
		for strings.HasSuffix(relDir, "/") {
			relDir = relDir[:len(relDir)-1]
		}
		relDir = "/" + relDir
		if strings.HasSuffix(relDir, "/.") {
			relDir = relDir[:len(relDir)-1]
		}
	}

	// Strip the file extension if the output path is an input file
	if customFilePath == "" {
		ext := fs.Ext(baseName)
		baseName = baseName[:len(baseName)-len(ext)]
	}
	return
}

func sanitizeFilePathForVirtualModulePath(path string) string {
	// Convert it to a safe file path. See: https://stackoverflow.com/a/31976060
	sb := strings.Builder{}
	needsGap := false
	for _, c := range path {
		switch c {
		case 0:
			// These characters are forbidden on Unix and Windows

		case '<', '>', ':', '"', '|', '?', '*':
			// These characters are forbidden on Windows

		default:
			if c < 0x20 {
				// These characters are forbidden on Windows
				break
			}

			// Turn runs of invalid characters into a '_'
			if needsGap {
				sb.WriteByte('_')
				needsGap = false
			}

			sb.WriteRune(c)
			continue
		}

		if sb.Len() > 0 {
			needsGap = true
		}
	}

	// Make sure the name isn't empty
	if sb.Len() == 0 {
		return "_"
	}

	// Note: An extension will be added to this base name, so there is no need to
	// avoid forbidden file names such as ".." since ".js" is a valid file name.
	return sb.String()
}
