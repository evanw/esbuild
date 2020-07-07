package bundler

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/lexer"
	"github.com/evanw/esbuild/internal/logging"
	"github.com/evanw/esbuild/internal/parser"
	"github.com/evanw/esbuild/internal/printer"
	"github.com/evanw/esbuild/internal/resolver"
	"github.com/evanw/esbuild/internal/runtime"
)

type file struct {
	ast ast.AST

	// If this file ends up being used in the bundle, this is an additional file
	// that must be written to the output directory. It's used by the "file"
	// loader.
	additionalFile *OutputFile

	// If true, this file was listed as not having side effects by a package.json
	// file in one of our containing directories with a "sideEffects" field.
	ignoreIfUnused bool

	// If "AbsMetadataFile" is present, this will be filled out with information
	// about this file in JSON format. This is a partial JSON file that will be
	// fully assembled later.
	jsonMetadataChunk []byte
}

type Bundle struct {
	fs          fs.FS
	res         resolver.Resolver
	sources     []logging.Source
	files       []file
	entryPoints []uint32
}

type parseFlags struct {
	jsxFactory        []string
	jsxFragment       []string
	isEntryPoint      bool
	isDisabled        bool
	ignoreIfUnused    bool
	strictClassFields bool
}

type parseArgs struct {
	fs           fs.FS
	log          logging.Log
	res          resolver.Resolver
	absPath      string
	prettyPath   string
	sourceIndex  uint32
	importSource *logging.Source
	flags        parseFlags
	pathRange    ast.Range
	options      config.Options
	results      chan parseResult
}

type parseResult struct {
	source logging.Source
	file   file
	ok     bool
}

func parseFile(args parseArgs) {
	source := logging.Source{
		Index:        args.sourceIndex,
		AbsolutePath: args.absPath,
		PrettyPath:   args.prettyPath,
	}

	// Disabled files are left empty
	stdin := args.options.Stdin
	if !args.flags.isDisabled {
		if stdin != nil {
			source.Contents = stdin.Contents
			source.PrettyPath = "<stdin>"
			if stdin.SourceFile != "" {
				source.PrettyPath = stdin.SourceFile
			}
		} else {
			var ok bool
			source.Contents, ok = args.res.Read(args.absPath)
			if !ok {
				args.log.AddRangeError(args.importSource, args.pathRange,
					fmt.Sprintf("Could not read from file: %s", args.absPath))
				args.results <- parseResult{}
				return
			}
		}
	}

	// Allow certain properties to be overridden
	if len(args.flags.jsxFactory) > 0 {
		args.options.JSX.Factory = args.flags.jsxFactory
	}
	if len(args.flags.jsxFragment) > 0 {
		args.options.JSX.Fragment = args.flags.jsxFragment
	}
	if args.flags.strictClassFields {
		args.options.Strict.ClassFields = true
	}

	// Pick the loader based on the file extension
	loader := loaderFromFileExtension(args.options.ExtensionToLoader, args.fs.Base(args.absPath))

	// Special-case reading from stdin
	if stdin != nil {
		loader = stdin.Loader
	}

	result := parseResult{
		source: source,
		file: file{
			ignoreIfUnused: args.flags.ignoreIfUnused,
		},
		ok: true,
	}

	switch loader {
	case config.LoaderJS:
		result.file.ast, result.ok = parser.Parse(args.log, source, args.options)

	case config.LoaderJSX:
		args.options.JSX.Parse = true
		result.file.ast, result.ok = parser.Parse(args.log, source, args.options)

	case config.LoaderTS:
		args.options.TS.Parse = true
		result.file.ast, result.ok = parser.Parse(args.log, source, args.options)

	case config.LoaderTSX:
		args.options.TS.Parse = true
		args.options.JSX.Parse = true
		result.file.ast, result.ok = parser.Parse(args.log, source, args.options)

	case config.LoaderJSON:
		var expr ast.Expr
		expr, result.ok = parser.ParseJSON(args.log, source, parser.ParseJSONOptions{})
		result.file.ast = parser.LazyExportAST(args.log, source, args.options, expr)
		result.file.ignoreIfUnused = true

	case config.LoaderText:
		expr := ast.Expr{Data: &ast.EString{Value: lexer.StringToUTF16(source.Contents)}}
		result.file.ast = parser.LazyExportAST(args.log, source, args.options, expr)
		result.file.ignoreIfUnused = true

	case config.LoaderBase64:
		encoded := base64.StdEncoding.EncodeToString([]byte(source.Contents))
		expr := ast.Expr{Data: &ast.EString{Value: lexer.StringToUTF16(encoded)}}
		result.file.ast = parser.LazyExportAST(args.log, source, args.options, expr)
		result.file.ignoreIfUnused = true

	case config.LoaderDataURL:
		mimeType := mime.TypeByExtension(args.fs.Ext(args.absPath))
		if mimeType == "" {
			mimeType = http.DetectContentType([]byte(source.Contents))
		}
		encoded := base64.StdEncoding.EncodeToString([]byte(source.Contents))
		url := "data:" + mimeType + ";base64," + encoded
		expr := ast.Expr{Data: &ast.EString{Value: lexer.StringToUTF16(url)}}
		result.file.ast = parser.LazyExportAST(args.log, source, args.options, expr)
		result.file.ignoreIfUnused = true

	case config.LoaderFile:
		// Add a hash to the file name to prevent multiple files with the same base
		// name from colliding. Avoid using the absolute path to prevent build
		// output from being different on different machines.
		baseName := baseNameForAvoidingCollisions(args.fs, args.absPath)

		// Determine the destination folder
		targetFolder := args.options.AbsOutputDir

		// Export the resulting relative path as a string
		expr := ast.Expr{Data: &ast.EString{Value: lexer.StringToUTF16(baseName)}}
		result.file.ast = parser.LazyExportAST(args.log, source, args.options, expr)
		result.file.ignoreIfUnused = true

		// Optionally add metadata about the file
		var jsonMetadataChunk []byte
		if args.options.AbsMetadataFile != "" {
			jsonMetadataChunk = []byte(fmt.Sprintf(
				"{\n      \"inputs\": {},\n      \"bytes\": %d\n    }", len(source.Contents)))
		}

		// Copy the file using an additional file payload to make sure we only copy
		// the file if the module isn't removed due to tree shaking.
		result.file.additionalFile = &OutputFile{
			AbsPath:           args.fs.Join(targetFolder, baseName),
			Contents:          []byte(source.Contents),
			jsonMetadataChunk: jsonMetadataChunk,
		}

	default:
		result.ok = false
		args.log.AddRangeError(args.importSource, args.pathRange,
			fmt.Sprintf("File extension not supported: %s", args.prettyPath))
	}

	args.results <- result
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

// Identify the path by its lowercase absolute path name. This should
// hopefully avoid path case issues on Windows, which has case-insensitive
// file system paths.
func lowerCaseAbsPathForWindows(absPath string) string {
	return strings.ToLower(absPath)
}

func baseNameForAvoidingCollisions(fs fs.FS, absPath string) string {
	var toHash []byte
	if relPath, ok := fs.Rel(fs.Cwd(), absPath); ok {
		// Attempt to generate the same base name regardless of what machine or
		// operating system we're on. We want to avoid absolute paths because they
		// will have different home directories. We also want to avoid path
		// separators because they are different on Windows.
		toHash = []byte(strings.ReplaceAll(relPath, "\\", "/"))
	} else {
		// Just use the absolute path if this environment doesn't have a current
		// directory. This is the case when running tests, for example.
		toHash = []byte(absPath)
	}

	// Use "URLEncoding" instead of "StdEncoding" to avoid introducing "/"
	hashBytes := sha1.Sum(toHash)
	hash := base64.URLEncoding.EncodeToString(hashBytes[:])[:8]

	// Insert the hash before the extension
	base := fs.Base(absPath)
	ext := fs.Ext(absPath)
	return base[:len(base)-len(ext)] + "." + hash + ext
}

func ScanBundle(log logging.Log, fs fs.FS, res resolver.Resolver, entryPaths []string, options config.Options) Bundle {
	sources := []logging.Source{}
	files := []file{}
	visited := make(map[string]uint32)
	results := make(chan parseResult)
	remaining := 0

	if options.ExtensionToLoader == nil {
		options.ExtensionToLoader = DefaultExtensionToLoaderMap()
	}

	// Always start by parsing the runtime file
	{
		source := runtime.Source
		sources = append(sources, source)
		files = append(files, file{})
		remaining++
		go func() {
			ast, ok := globalRuntimeCache.parseRuntime(&options)
			results <- parseResult{source: source, file: file{ast: ast}, ok: ok}
		}()
	}

	maybeParseFile := func(
		resolveResult resolver.ResolveResult,
		prettyPath string,
		importSource *logging.Source,
		pathRange ast.Range,
		isEntryPoint bool,
	) uint32 {
		lowerAbsPath := lowerCaseAbsPathForWindows(resolveResult.AbsolutePath)
		sourceIndex, ok := visited[lowerAbsPath]
		if !ok {
			sourceIndex = uint32(len(sources))
			visited[lowerAbsPath] = sourceIndex
			sources = append(sources, logging.Source{})
			files = append(files, file{})
			flags := parseFlags{
				isEntryPoint:      isEntryPoint,
				isDisabled:        resolveResult.Status == resolver.ResolveDisabled,
				ignoreIfUnused:    resolveResult.IgnoreIfUnused,
				jsxFactory:        resolveResult.JSXFactory,
				jsxFragment:       resolveResult.JSXFragment,
				strictClassFields: resolveResult.StrictClassFields,
			}
			remaining++
			go parseFile(parseArgs{
				fs:           fs,
				log:          log,
				res:          res,
				absPath:      resolveResult.AbsolutePath,
				prettyPath:   prettyPath,
				sourceIndex:  sourceIndex,
				importSource: importSource,
				flags:        flags,
				pathRange:    pathRange,
				options:      options,
				results:      results,
			})
		}
		return sourceIndex
	}

	entryPoints := []uint32{}
	duplicateEntryPoints := make(map[string]bool)
	for _, absPath := range entryPaths {
		prettyPath := res.PrettyPath(absPath)
		lowerAbsPath := lowerCaseAbsPathForWindows(absPath)
		if duplicateEntryPoints[lowerAbsPath] {
			log.AddError(nil, ast.Loc{}, "Duplicate entry point: "+prettyPath)
			continue
		}
		duplicateEntryPoints[lowerAbsPath] = true
		resolveResult := res.ResolveAbs(absPath)
		sourceIndex := maybeParseFile(resolveResult, prettyPath, nil, ast.Range{}, true /*isEntryPoint*/)
		entryPoints = append(entryPoints, sourceIndex)
	}

	for remaining > 0 {
		result := <-results
		remaining--
		if !result.ok {
			continue
		}

		source := result.source
		j := printer.Joiner{}
		isFirstImport := true

		// Begin the metadata chunk
		if options.AbsMetadataFile != "" {
			j.AddString(printer.QuoteForJSON(source.PrettyPath))
			j.AddString(fmt.Sprintf(": {\n      \"bytes\": %d,\n      \"imports\": [", len(source.Contents)))
		}

		// Don't try to resolve paths if we're not bundling
		if options.IsBundling {
			for _, part := range result.file.ast.Parts {
				for _, importRecordIndex := range part.ImportRecordIndices {
					record := &result.file.ast.ImportRecords[importRecordIndex]

					// Don't try to resolve imports that are already resolved
					if record.SourceIndex != nil {
						continue
					}

					pathText := record.Path.Text
					pathRange := source.RangeOfString(record.Path.Loc)
					resolveResult := res.Resolve(source.AbsolutePath, pathText)

					switch resolveResult.Status {
					case resolver.ResolveEnabled, resolver.ResolveDisabled:
						prettyPath := res.PrettyPath(resolveResult.AbsolutePath)
						sourceIndex := maybeParseFile(resolveResult, prettyPath, &source, pathRange, false /*isEntryPoint*/)
						record.SourceIndex = &sourceIndex

						// Generate metadata about each import
						if options.AbsMetadataFile != "" {
							if isFirstImport {
								isFirstImport = false
								j.AddString("\n        ")
							} else {
								j.AddString(",\n        ")
							}
							j.AddString(fmt.Sprintf("{\n          \"path\": %s\n        }",
								printer.QuoteForJSON(prettyPath)))
						}

					case resolver.ResolveMissing:
						log.AddRangeError(&source, pathRange, fmt.Sprintf("Could not resolve %q", pathText))

					case resolver.ResolveExternalRelative:
						// If the path to the external module is relative to the source
						// file, rewrite the path to be relative to the working directory
						if relPath, ok := fs.Rel(options.AbsOutputDir, resolveResult.AbsolutePath); ok {
							// Prevent issues with path separators being different on Windows
							record.Path.Text = strings.ReplaceAll(relPath, "\\", "/")
						}
					}
				}
			}
		}

		// End the metadata chunk
		if options.AbsMetadataFile != "" {
			if !isFirstImport {
				j.AddString("\n      ")
			}
			j.AddString("]\n    }")
		}

		result.file.jsonMetadataChunk = j.Done()
		sources[source.Index] = source
		files[source.Index] = result.file
	}

	return Bundle{fs, res, sources, files, entryPoints}
}

func DefaultExtensionToLoaderMap() map[string]config.Loader {
	return map[string]config.Loader{
		".js":   config.LoaderJS,
		".mjs":  config.LoaderJS,
		".cjs":  config.LoaderJS,
		".jsx":  config.LoaderJSX,
		".ts":   config.LoaderTS,
		".tsx":  config.LoaderTSX,
		".json": config.LoaderJSON,
		".txt":  config.LoaderText,
	}
}

type OutputFile struct {
	AbsPath  string
	Contents []byte

	// If "AbsMetadataFile" is present, this will be filled out with information
	// about this file in JSON format. This is a partial JSON file that will be
	// fully assembled later.
	jsonMetadataChunk []byte
}

type lineColumnOffset struct {
	lines   int
	columns int
}

type compileResult struct {
	printer.PrintResult

	// If this is an entry point, this is optional code to stick on the end of
	// the chunk. This is used to for example trigger the lazily-evaluated
	// CommonJS wrapper for the entry point.
	entryPointTail *printer.PrintResult

	sourceIndex uint32

	// This is the line and column offset since the previous JavaScript string
	// or the start of the file if this is the first JavaScript string.
	generatedOffset lineColumnOffset

	// The source map contains the original source code, which is quoted in
	// parallel for speed. This is only filled in if the SourceMap option is
	// enabled.
	quotedSource string
}

func (b *Bundle) Compile(log logging.Log, options config.Options) []OutputFile {
	if options.ExtensionToLoader == nil {
		options.ExtensionToLoader = DefaultExtensionToLoaderMap()
	}

	// The format can't be "preserve" while bundling
	if options.IsBundling && options.OutputFormat == config.FormatPreserve {
		options.OutputFormat = config.FormatESModule
	}

	type linkGroup struct {
		outputFiles    []OutputFile
		reachableFiles []uint32
	}

	var resultGroups []linkGroup
	if options.CodeSplitting {
		// If code splitting is enabled, link all entry points together
		c := newLinkerContext(&options, log, b.fs, b.sources, b.files, b.entryPoints)
		resultGroups = []linkGroup{{
			outputFiles:    c.link(),
			reachableFiles: c.reachableFiles,
		}}
	} else {
		// Otherwise, link each entry point with the runtime file separately
		waitGroup := sync.WaitGroup{}
		resultGroups = make([]linkGroup, len(b.entryPoints))
		for i, entryPoint := range b.entryPoints {
			waitGroup.Add(1)
			go func(i int, entryPoint uint32) {
				c := newLinkerContext(&options, log, b.fs, b.sources, b.files, []uint32{entryPoint})
				resultGroups[i] = linkGroup{
					outputFiles:    c.link(),
					reachableFiles: c.reachableFiles,
				}
				waitGroup.Done()
			}(i, entryPoint)
		}
		waitGroup.Wait()
	}

	// Join the results in entry point order for determinism
	var outputFiles []OutputFile
	for _, group := range resultGroups {
		outputFiles = append(outputFiles, group.outputFiles...)
	}

	// Also generate the metadata file if necessary
	if options.AbsMetadataFile != "" {
		outputFiles = append(outputFiles, OutputFile{
			AbsPath:  options.AbsMetadataFile,
			Contents: b.generateMetadataJSON(outputFiles),
		})
	}

	if !options.WriteToStdout {
		// Make sure an output file never overwrites an input file
		sourceAbsPaths := make(map[string]uint32)
		for _, group := range resultGroups {
			for _, sourceIndex := range group.reachableFiles {
				lowerAbsPath := lowerCaseAbsPathForWindows(b.sources[sourceIndex].AbsolutePath)
				sourceAbsPaths[lowerAbsPath] = sourceIndex
			}
		}
		for _, outputFile := range outputFiles {
			lowerAbsPath := lowerCaseAbsPathForWindows(outputFile.AbsPath)
			if sourceIndex, ok := sourceAbsPaths[lowerAbsPath]; ok {
				log.AddError(nil, ast.Loc{}, "Refusing to overwrite input file: "+b.sources[sourceIndex].PrettyPath)
			}
		}

		// Make sure an output file never overwrites another output file. This
		// is almost certainly unintentional and would otherwise happen silently.
		outputFileMap := make(map[string]bool)
		for _, outputFile := range outputFiles {
			lowerAbsPath := lowerCaseAbsPathForWindows(outputFile.AbsPath)
			if outputFileMap[lowerAbsPath] {
				outputPath := outputFile.AbsPath
				if relPath, ok := b.fs.Rel(b.fs.Cwd(), outputPath); ok {
					outputPath = relPath
				}
				log.AddError(nil, ast.Loc{}, "Two output files share the same path: "+outputPath)
			} else {
				outputFileMap[lowerAbsPath] = true
			}
		}
	}

	return outputFiles
}

func (b *Bundle) generateMetadataJSON(results []OutputFile) []byte {
	// Sort files by absolute path for determinism
	sorted := make(indexAndPathArray, 0, len(b.sources))
	for sourceIndex, source := range b.sources {
		if uint32(sourceIndex) != runtime.SourceIndex {
			sorted = append(sorted, indexAndPath{uint32(sourceIndex), source.PrettyPath})
		}
	}
	sort.Sort(sorted)

	j := printer.Joiner{}
	j.AddString("{\n  \"inputs\": {")

	// Write inputs
	for i, item := range sorted {
		if i > 0 {
			j.AddString(",\n    ")
		} else {
			j.AddString("\n    ")
		}
		j.AddBytes(b.files[item.sourceIndex].jsonMetadataChunk)
	}

	j.AddString("\n  },\n  \"outputs\": {")

	// Write outputs
	isFirst := true
	for _, result := range results {
		if len(result.jsonMetadataChunk) > 0 {
			if isFirst {
				isFirst = false
				j.AddString("\n    ")
			} else {
				j.AddString(",\n    ")
			}
			j.AddString(fmt.Sprintf("%s: ", printer.QuoteForJSON(b.res.PrettyPath(result.AbsPath))))
			j.AddBytes(result.jsonMetadataChunk)
		}
	}

	j.AddString("\n  }\n}\n")
	return j.Done()
}

type runtimeCacheKey struct {
	MangleSyntax bool
}

type runtimeCache struct {
	mutex    sync.Mutex
	keyToAST map[runtimeCacheKey]ast.AST
}

var globalRuntimeCache runtimeCache

func (cache *runtimeCache) parseRuntime(options *config.Options) (runtimeAST ast.AST, ok bool) {
	key := runtimeCacheKey{
		// All configuration options that the runtime code depends on must go here
		MangleSyntax: options.MangleSyntax,
	}

	// Cache hit?
	(func() {
		cache.mutex.Lock()
		defer cache.mutex.Unlock()
		if cache.keyToAST != nil {
			runtimeAST, ok = cache.keyToAST[key]
		}
	})()
	if ok {
		return
	}

	// Cache miss
	log := logging.NewDeferLog()
	runtimeAST, ok = parser.Parse(log, runtime.Source, config.Options{
		// These configuration options must only depend on the key
		MangleSyntax: key.MangleSyntax,

		// Always do tree shaking for the runtime because we never want to
		// include unnecessary runtime code
		IsBundling: true,
	})
	if log.HasErrors() {
		panic("Internal error: failed to parse runtime")
	}

	// Cache for next time
	if ok {
		cache.mutex.Lock()
		defer cache.mutex.Unlock()
		if cache.keyToAST == nil {
			cache.keyToAST = make(map[runtimeCacheKey]ast.AST)
		}
		cache.keyToAST[key] = runtimeAST
	}

	return
}
