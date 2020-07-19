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
	"unicode"
	"unicode/utf8"

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
	ignoreIfUnused    bool
	strictClassFields bool
}

type parseArgs struct {
	fs           fs.FS
	log          logging.Log
	res          resolver.Resolver
	keyPath      ast.Path
	prettyPath   string
	baseName     string
	sourceIndex  uint32
	importSource *logging.Source
	flags        parseFlags
	pathRange    ast.Range
	options      config.Options
	results      chan parseResult

	// If non-empty, this provides a fallback directory to resolve imports
	// against for virtual source files (i.e. those with no file system path).
	// This is used for stdin, for example.
	absResolveDir string
}

type parseResult struct {
	source logging.Source
	file   file
	ok     bool

	resolveResults []*resolver.ResolveResult
}

func parseFile(args parseArgs) {
	source := logging.Source{
		Index:      args.sourceIndex,
		KeyPath:    args.keyPath,
		PrettyPath: args.prettyPath,
	}

	// Try to determine the identifier name by the absolute path, since it may
	// need to look at the parent directory. But make sure to not treat the key
	// as a file system path if it's not marked as one.
	if args.keyPath.IsAbsolute {
		source.IdentifierName = ast.GenerateNonUniqueNameFromPath(args.keyPath.Text)
	} else {
		source.IdentifierName = ast.GenerateNonUniqueNameFromPath(args.baseName)
	}

	var loader config.Loader
	stdin := args.options.Stdin

	if stdin != nil {
		// Special-case stdin
		source.Contents = stdin.Contents
		source.PrettyPath = "<stdin>"
		if stdin.SourceFile != "" {
			source.PrettyPath = stdin.SourceFile
		}
		loader = stdin.Loader
	} else if args.keyPath.IsAbsolute {
		// Read normal modules from disk
		var ok bool
		source.Contents, ok = args.res.Read(args.keyPath.Text)
		if !ok {
			args.log.AddRangeError(args.importSource, args.pathRange,
				fmt.Sprintf("Could not read from file: %s", args.keyPath.Text))
			args.results <- parseResult{}
			return
		}
		loader = loaderFromFileExtension(args.options.ExtensionToLoader, args.baseName)
	} else {
		// Right now the only non-absolute modules are disabled ones
		if !strings.HasPrefix(args.keyPath.Text, "disabled:") {
			panic("Internal error")
		}
		loader = config.LoaderJS
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
		result.file.ast = parser.LazyExportAST(args.log, source, args.options, expr, "")
		result.file.ignoreIfUnused = true

	case config.LoaderText:
		expr := ast.Expr{Data: &ast.EString{Value: lexer.StringToUTF16(source.Contents)}}
		result.file.ast = parser.LazyExportAST(args.log, source, args.options, expr, "")
		result.file.ignoreIfUnused = true

	case config.LoaderBase64:
		encoded := base64.StdEncoding.EncodeToString([]byte(source.Contents))
		expr := ast.Expr{Data: &ast.EString{Value: lexer.StringToUTF16(encoded)}}
		result.file.ast = parser.LazyExportAST(args.log, source, args.options, expr, "")
		result.file.ignoreIfUnused = true

	case config.LoaderBinary:
		encoded := base64.StdEncoding.EncodeToString([]byte(source.Contents))
		expr := ast.Expr{Data: &ast.EString{Value: lexer.StringToUTF16(encoded)}}
		result.file.ast = parser.LazyExportAST(args.log, source, args.options, expr, "__toBinary")
		result.file.ignoreIfUnused = true

	case config.LoaderDataURL:
		mimeType := mime.TypeByExtension(args.fs.Ext(args.baseName))
		if mimeType == "" {
			mimeType = http.DetectContentType([]byte(source.Contents))
		}
		encoded := base64.StdEncoding.EncodeToString([]byte(source.Contents))
		url := "data:" + mimeType + ";base64," + encoded
		expr := ast.Expr{Data: &ast.EString{Value: lexer.StringToUTF16(url)}}
		result.file.ast = parser.LazyExportAST(args.log, source, args.options, expr, "")
		result.file.ignoreIfUnused = true

	case config.LoaderFile:
		// Add a hash to the file name to prevent multiple files with the same base
		// name from colliding. Avoid using the absolute path to prevent build
		// output from being different on different machines.
		baseName := baseNameForAvoidingCollisions(args.fs, args.keyPath, args.baseName)

		// Determine the destination folder
		targetFolder := args.options.AbsOutputDir

		// Export the resulting relative path as a string
		expr := ast.Expr{Data: &ast.EString{Value: lexer.StringToUTF16(baseName)}}
		result.file.ast = parser.LazyExportAST(args.log, source, args.options, expr, "")
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

	// Stop now if parsing failed
	if !result.ok {
		args.results <- result
		return
	}

	// Run the resolver on the parse thread so it's not run on the main thread.
	// That way the main thread isn't blocked if the resolver takes a while.
	if args.options.IsBundling {
		result.resolveResults = make([]*resolver.ResolveResult, len(result.file.ast.ImportRecords))

		if len(result.file.ast.ImportRecords) > 0 {
			cache := make(map[string]*resolver.ResolveResult)

			// Resolve relative to the parent directory of the source file with the
			// import path. Just use the current directory if the source file is virtual.
			var sourceDir string
			if source.KeyPath.IsAbsolute {
				sourceDir = args.fs.Dir(source.KeyPath.Text)
			} else if args.absResolveDir != "" {
				sourceDir = args.absResolveDir
			} else {
				sourceDir = args.fs.Cwd()
			}

			for _, part := range result.file.ast.Parts {
				for _, importRecordIndex := range part.ImportRecordIndices {
					// Don't try to resolve imports that are already resolved
					record := &result.file.ast.ImportRecords[importRecordIndex]
					if record.SourceIndex != nil {
						continue
					}

					// Cache the path in case it's imported multiple times in this file
					if resolveResult, ok := cache[record.Path.Text]; ok {
						result.resolveResults[importRecordIndex] = resolveResult
						continue
					}

					// Run the resolver and log an error if the path couldn't be resolved
					resolveResult := args.res.Resolve(sourceDir, record.Path.Text)
					cache[record.Path.Text] = resolveResult

					if resolveResult == nil {
						// Failed imports inside a try/catch are silently turned into
						// external imports instead of causing errors. This matches a common
						// code pattern for conditionally importing a module with a graceful
						// fallback.
						if !record.IsInsideTryBody {
							r := source.RangeOfString(record.Loc)
							args.log.AddRangeError(&source, r, fmt.Sprintf("Could not resolve %q", record.Path.Text))
						}
						continue
					}

					result.resolveResults[importRecordIndex] = resolveResult
				}
			}
		}
	}

	// Attempt to parse the source map if present
	if args.options.SourceMap != config.SourceMapNone && result.file.ast.SourceMapComment.Text != "" {
		if path, contents := extractSourceMapFromComment(args.log, args.fs, &source, result.file.ast.SourceMapComment); contents != nil {
			prettyPath := path.Text
			if path.IsAbsolute {
				prettyPath = args.res.PrettyPath(prettyPath)
			}
			result.file.ast.SourceMap = parser.ParseSourceMap(args.log, logging.Source{
				KeyPath:    path,
				PrettyPath: prettyPath,
				Contents:   *contents,
			})
		}
	}

	args.results <- result
}

func extractSourceMapFromComment(log logging.Log, fs fs.FS, source *logging.Source, comment ast.Span) (ast.Path, *string) {
	// Data URL
	if strings.HasPrefix(comment.Text, "data:") {
		if strings.HasPrefix(comment.Text, "data:application/json;base64,") {
			n := int32(len("data:application/json;base64,"))
			encoded := comment.Text[n:]
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				r := ast.Range{Loc: ast.Loc{Start: comment.Range.Loc.Start + n}, Len: comment.Range.Len - n}
				log.AddRangeWarning(source, r, "Invalid base64 data in source map")
				return ast.Path{}, nil
			}
			contents := string(decoded)
			return ast.Path{Text: "sourceMappingURL in " + source.PrettyPath}, &contents
		}
	}

	// Absolute path
	if source.KeyPath.IsAbsolute {
		absPath := fs.Join(fs.Dir(source.KeyPath.Text), comment.Text)
		contents, ok := fs.ReadFile(absPath)
		if !ok {
			log.AddRangeWarning(source, comment.Range, fmt.Sprintf("Could not find source map file: %s", absPath))
			return ast.Path{}, nil
		}
		return ast.Path{IsAbsolute: true, Text: absPath}, &contents
	}

	// Anything else is unsupported
	log.AddRangeWarning(source, comment.Range, "Unsupported source map comment")
	return ast.Path{}, nil
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

func baseNameForAvoidingCollisions(fs fs.FS, keyPath ast.Path, base string) string {
	var toHash []byte
	if keyPath.IsAbsolute {
		if relPath, ok := fs.Rel(fs.Cwd(), keyPath.Text); ok {
			// Attempt to generate the same base name regardless of what machine or
			// operating system we're on. We want to avoid absolute paths because they
			// will have different home directories. We also want to avoid path
			// separators because they are different on Windows.
			toHash = []byte(lowerCaseAbsPathForWindows(strings.ReplaceAll(relPath, "\\", "/")))
		}
	}
	if toHash == nil {
		// Just use the absolute path if this environment doesn't have a current
		// directory. This is the case when running tests, for example.
		toHash = []byte(keyPath.Text)
	}

	// Use "URLEncoding" instead of "StdEncoding" to avoid introducing "/"
	hashBytes := sha1.Sum(toHash)
	hash := base64.URLEncoding.EncodeToString(hashBytes[:])[:8]

	// Insert the hash before the extension
	ext := fs.Ext(base)
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
		sources = append(sources, logging.Source{})
		files = append(files, file{})
		remaining++
		go func() {
			source, ast, ok := globalRuntimeCache.parseRuntime(&options)
			results <- parseResult{source: source, file: file{ast: ast}, ok: ok}
		}()
	}

	type inputKind uint8

	const (
		inputKindNormal inputKind = iota
		inputKindEntryPoint
		inputKindStdin
	)

	maybeParseFile := func(
		resolveResult resolver.ResolveResult,
		prettyPath string,
		importSource *logging.Source,
		pathRange ast.Range,
		absResolveDir string,
		kind inputKind,
	) uint32 {
		visitedKey := resolveResult.Path.Text
		if resolveResult.Path.IsAbsolute {
			visitedKey = lowerCaseAbsPathForWindows(visitedKey)
		}
		sourceIndex, ok := visited[visitedKey]
		if !ok {
			sourceIndex = uint32(len(sources))
			visited[visitedKey] = sourceIndex
			sources = append(sources, logging.Source{})
			files = append(files, file{})
			flags := parseFlags{
				isEntryPoint:      kind == inputKindEntryPoint,
				ignoreIfUnused:    resolveResult.IgnoreIfUnused,
				jsxFactory:        resolveResult.JSXFactory,
				jsxFragment:       resolveResult.JSXFragment,
				strictClassFields: resolveResult.StrictClassFields,
			}
			remaining++
			optionsClone := options
			if kind != inputKindStdin {
				optionsClone.Stdin = nil
			}
			go parseFile(parseArgs{
				fs:            fs,
				log:           log,
				res:           res,
				keyPath:       resolveResult.Path,
				prettyPath:    prettyPath,
				baseName:      fs.Base(resolveResult.Path.Text),
				sourceIndex:   sourceIndex,
				importSource:  importSource,
				flags:         flags,
				pathRange:     pathRange,
				options:       optionsClone,
				results:       results,
				absResolveDir: absResolveDir,
			})
		}
		return sourceIndex
	}

	entryPoints := []uint32{}
	duplicateEntryPoints := make(map[string]bool)

	// Treat stdin as an extra entry point
	if options.Stdin != nil {
		resolveResult := resolver.ResolveResult{Path: ast.Path{Text: "<stdin>"}}
		sourceIndex := maybeParseFile(resolveResult, "<stdin>", nil, ast.Range{}, options.Stdin.AbsResolveDir, inputKindStdin)
		entryPoints = append(entryPoints, sourceIndex)
	}

	// Add any remaining entry points
	for _, absPath := range entryPaths {
		prettyPath := res.PrettyPath(absPath)
		lowerAbsPath := lowerCaseAbsPathForWindows(absPath)

		if duplicateEntryPoints[lowerAbsPath] {
			log.AddError(nil, ast.Loc{}, "Duplicate entry point: "+prettyPath)
			continue
		}

		duplicateEntryPoints[lowerAbsPath] = true
		resolveResult := res.ResolveAbs(absPath)

		if resolveResult == nil {
			log.AddError(nil, ast.Loc{}, "Could not resolve: "+prettyPath)
			continue
		}

		sourceIndex := maybeParseFile(*resolveResult, prettyPath, nil, ast.Range{}, "", inputKindEntryPoint)
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

					// Skip this import record if the previous resolver call failed
					resolveResult := result.resolveResults[importRecordIndex]
					if resolveResult == nil {
						continue
					}

					if !resolveResult.IsExternal {
						// Handle a path within the bundle
						prettyPath := resolveResult.Path.Text
						if resolveResult.Path.IsAbsolute {
							prettyPath = res.PrettyPath(prettyPath)
						}
						pathRange := source.RangeOfString(record.Loc)
						sourceIndex := maybeParseFile(*resolveResult, prettyPath, &source, pathRange, "", inputKindNormal)
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
					} else {
						// If the path to the external module is relative to the source
						// file, rewrite the path to be relative to the working directory
						if resolveResult.Path.IsAbsolute {
							if relPath, ok := fs.Rel(options.AbsOutputDir, resolveResult.Path.Text); ok {
								// Prevent issues with path separators being different on Windows
								record.Path.Text = strings.ReplaceAll(relPath, "\\", "/")
							}
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
}

func (b *Bundle) Compile(log logging.Log, options config.Options) []OutputFile {
	if options.ExtensionToLoader == nil {
		options.ExtensionToLoader = DefaultExtensionToLoaderMap()
	}

	// The format can't be "preserve" while bundling
	if options.IsBundling && options.OutputFormat == config.FormatPreserve {
		options.OutputFormat = config.FormatESModule
	}

	// Determine the lowest common ancestor of all entry points
	lcaAbsPath := b.lowestCommonAncestorDirectory(options.CodeSplitting)

	type linkGroup struct {
		outputFiles    []OutputFile
		reachableFiles []uint32
	}

	var resultGroups []linkGroup
	if options.CodeSplitting {
		// If code splitting is enabled, link all entry points together
		c := newLinkerContext(&options, log, b.fs, b.res, b.sources, b.files, b.entryPoints, lcaAbsPath)
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
				c := newLinkerContext(&options, log, b.fs, b.res, b.sources, b.files, []uint32{entryPoint}, lcaAbsPath)
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
				keyPath := b.sources[sourceIndex].KeyPath
				if keyPath.IsAbsolute {
					lowerAbsPath := lowerCaseAbsPathForWindows(keyPath.Text)
					sourceAbsPaths[lowerAbsPath] = sourceIndex
				}
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

func (b *Bundle) lowestCommonAncestorDirectory(codeSplitting bool) string {
	isEntryPoint := make(map[uint32]bool)
	for _, entryPoint := range b.entryPoints {
		isEntryPoint[entryPoint] = true
	}

	// If code splitting is enabled, also treat dynamic imports as entry points
	if codeSplitting {
		for _, sourceIndex := range findReachableFiles(b.sources, b.files, b.entryPoints) {
			file := b.files[sourceIndex]
			for _, part := range file.ast.Parts {
				for _, importRecordIndex := range part.ImportRecordIndices {
					if record := &file.ast.ImportRecords[importRecordIndex]; record.SourceIndex != nil && record.Kind == ast.ImportDynamic {
						isEntryPoint[*record.SourceIndex] = true
					}
				}
			}
		}
	}

	// Ignore any paths for virtual modules (that don't exist on the file system)
	absPaths := make([]string, 0, len(isEntryPoint))
	for entryPoint := range isEntryPoint {
		keyPath := b.sources[entryPoint].KeyPath
		if keyPath.IsAbsolute {
			absPaths = append(absPaths, keyPath.Text)
		}
	}

	if len(absPaths) == 0 {
		return ""
	}

	lowestAbsDir := b.fs.Dir(absPaths[0])

	for _, absPath := range absPaths[1:] {
		absDir := b.fs.Dir(absPath)
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

func (b *Bundle) generateMetadataJSON(results []OutputFile) []byte {
	// Sort files by key path for determinism
	sorted := make(indexAndPathArray, 0, len(b.sources))
	for sourceIndex, source := range b.sources {
		if uint32(sourceIndex) != runtime.SourceIndex {
			sorted = append(sorted, indexAndPath{uint32(sourceIndex), source.KeyPath})
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
	ES6          bool
	Platform     config.Platform
}

type runtimeCache struct {
	astMutex sync.Mutex
	astMap   map[runtimeCacheKey]ast.AST

	definesMutex sync.Mutex
	definesMap   map[config.Platform]*config.ProcessedDefines
}

var globalRuntimeCache runtimeCache

func (cache *runtimeCache) parseRuntime(options *config.Options) (source logging.Source, runtimeAST ast.AST, ok bool) {
	key := runtimeCacheKey{
		// All configuration options that the runtime code depends on must go here
		MangleSyntax: options.MangleSyntax,
		Platform:     options.Platform,
		ES6:          runtime.CanUseES6(options.UnsupportedFeatures),
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
	log := logging.NewDeferLog()
	runtimeAST, ok = parser.Parse(log, source, config.Options{
		// These configuration options must only depend on the key
		MangleSyntax: key.MangleSyntax,
		Platform:     key.Platform,
		Defines:      cache.processedDefines(key.Platform),

		// Always do tree shaking for the runtime because we never want to
		// include unnecessary runtime code
		IsBundling: true,
	})
	if log.HasErrors() {
		panic("Internal error: failed to parse runtime")
	}

	// Cache for next time
	if ok {
		cache.astMutex.Lock()
		defer cache.astMutex.Unlock()
		if cache.astMap == nil {
			cache.astMap = make(map[runtimeCacheKey]ast.AST)
		}
		cache.astMap[key] = runtimeAST
	}
	return
}

func (cache *runtimeCache) processedDefines(key config.Platform) (defines *config.ProcessedDefines) {
	ok := false

	// Cache hit?
	(func() {
		cache.definesMutex.Lock()
		defer cache.definesMutex.Unlock()
		if cache.definesMap != nil {
			defines, ok = cache.definesMap[key]
		}
	})()
	if ok {
		return
	}

	// Cache miss
	var platform string
	switch key {
	case config.PlatformBrowser:
		platform = "browser"
	case config.PlatformNode:
		platform = "node"
	}
	result := config.ProcessDefines(map[string]config.DefineData{
		"__platform": config.DefineData{
			DefineFunc: func(config.FindSymbol) ast.E {
				return &ast.EString{Value: lexer.StringToUTF16(platform)}
			},
		},
	})
	defines = &result

	// Cache for next time
	cache.definesMutex.Lock()
	defer cache.definesMutex.Unlock()
	if cache.definesMap == nil {
		cache.definesMap = make(map[config.Platform]*config.ProcessedDefines)
	}
	cache.definesMap[key] = defines
	return
}
