package bundler

import (
	"bytes"
	"crypto/sha1"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"
	"syscall"
	"unicode"
	"unicode/utf8"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_parser"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/js_parser"
	"github.com/evanw/esbuild/internal/js_printer"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/resolver"
	"github.com/evanw/esbuild/internal/runtime"
	"github.com/evanw/esbuild/internal/sourcemap"
)

type file struct {
	source    logger.Source
	repr      fileRepr
	loader    config.Loader
	sourceMap *sourcemap.SourceMap

	// The minimum number of links in the module graph to get from an entry point
	// to this file
	distanceFromEntryPoint uint32

	// This holds all entry points that can reach this file. It will be used to
	// assign the parts in this file to a chunk.
	entryBits bitSet

	// If "AbsMetadataFile" is present, this will be filled out with information
	// about this file in JSON format. This is a partial JSON file that will be
	// fully assembled later.
	jsonMetadataChunk []byte

	// The path of this entry point relative to the lowest common ancestor
	// directory containing all entry points. Note: this must have OS-independent
	// path separators (i.e. '/' not '\').
	entryPointRelPath string

	// If this file ends up being used in the bundle, these are additional files
	// that must be written to the output directory. It's used by the "file"
	// loader.
	additionalFiles []OutputFile

	isEntryPoint bool

	// If true, this file was listed as not having side effects by a package.json
	// file in one of our containing directories with a "sideEffects" field.
	ignoreIfUnused bool
}

type fileRepr interface {
	importRecords() []ast.ImportRecord
}

type reprJS struct {
	ast  js_ast.AST
	meta fileMeta

	// If present, this is the CSS file that this JavaScript stub corresponds to.
	// A JavaScript stub is automatically generated for a CSS file when it's
	// imported from a JavaScript file.
	cssSourceIndex *uint32
}

func (repr *reprJS) importRecords() []ast.ImportRecord {
	return repr.ast.ImportRecords
}

type reprCSS struct {
	ast css_ast.AST

	// If present, this is the JavaScript stub corresponding to this CSS file.
	// A JavaScript stub is automatically generated for a CSS file when it's
	// imported from a JavaScript file.
	jsSourceIndex *uint32
}

func (repr *reprCSS) importRecords() []ast.ImportRecord {
	return repr.ast.ImportRecords
}

type Bundle struct {
	fs          fs.FS
	res         resolver.Resolver
	files       []file
	entryPoints []uint32
}

type parseFlags struct {
	isEntryPoint   bool
	ignoreIfUnused bool
}

type parseArgs struct {
	fs              fs.FS
	log             logger.Log
	res             resolver.Resolver
	keyPath         logger.Path
	prettyPath      string
	baseName        string
	sourceIndex     uint32
	importSource    *logger.Source
	flags           parseFlags
	importPathRange logger.Range
	options         config.Options
	results         chan parseResult

	// If non-empty, this provides a fallback directory to resolve imports
	// against for virtual source files (i.e. those with no file system path).
	// This is used for stdin, for example.
	absResolveDir string
}

type parseResult struct {
	file file
	ok   bool

	resolveResults []*resolver.ResolveResult
}

func parseFile(args parseArgs) {
	source := logger.Source{
		Index:      args.sourceIndex,
		KeyPath:    args.keyPath,
		PrettyPath: args.prettyPath,
	}

	// Try to determine the identifier name by the absolute path, since it may
	// need to look at the parent directory. But make sure to not treat the key
	// as a file system path if it's not marked as one.
	if args.keyPath.Namespace == "file" {
		source.IdentifierName = js_ast.GenerateNonUniqueNameFromPath(args.keyPath.Text)
	} else {
		source.IdentifierName = js_ast.GenerateNonUniqueNameFromPath(args.baseName)
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
	} else if args.keyPath.Namespace == "file" {
		// Read normal modules from disk
		var err error
		source.Contents, err = args.fs.ReadFile(args.keyPath.Text)
		if err != nil {
			if err == syscall.ENOENT {
				args.log.AddRangeError(args.importSource, args.importPathRange,
					fmt.Sprintf("Could not read from file: %s", args.keyPath.Text))
			} else {
				args.log.AddRangeError(args.importSource, args.importPathRange,
					fmt.Sprintf("Cannot read file %q: %s", args.res.PrettyPath(args.keyPath), err.Error()))
			}
			args.results <- parseResult{}
			return
		}
		loader = loaderFromFileExtension(args.options.ExtensionToLoader, args.baseName)
	} else if source.KeyPath.Namespace == resolver.BrowserFalseNamespace {
		// Force disabled modules to be empty
		loader = config.LoaderJS
	}

	result := parseResult{
		file: file{
			source:         source,
			loader:         loader,
			ignoreIfUnused: args.flags.ignoreIfUnused,
		},
	}

	switch loader {
	case config.LoaderJS:
		ast, ok := js_parser.Parse(args.log, source, args.options)
		result.file.repr = &reprJS{ast: ast}
		result.ok = ok

	case config.LoaderJSX:
		args.options.JSX.Parse = true
		ast, ok := js_parser.Parse(args.log, source, args.options)
		result.file.repr = &reprJS{ast: ast}
		result.ok = ok

	case config.LoaderTS:
		args.options.TS.Parse = true
		ast, ok := js_parser.Parse(args.log, source, args.options)
		result.file.repr = &reprJS{ast: ast}
		result.ok = ok

	case config.LoaderTSX:
		args.options.TS.Parse = true
		args.options.JSX.Parse = true
		ast, ok := js_parser.Parse(args.log, source, args.options)
		result.file.repr = &reprJS{ast: ast}
		result.ok = ok

	case config.LoaderCSS:
		ast := css_parser.Parse(args.log, source, args.options)
		result.file.repr = &reprCSS{ast: ast}
		result.ok = true

	case config.LoaderJSON:
		expr, ok := js_parser.ParseJSON(args.log, source, js_parser.ParseJSONOptions{})
		ast := js_parser.LazyExportAST(args.log, source, args.options, expr, "")
		result.file.ignoreIfUnused = true
		result.file.repr = &reprJS{ast: ast}
		result.ok = ok

	case config.LoaderText:
		encoded := base64.StdEncoding.EncodeToString([]byte(source.Contents))
		expr := js_ast.Expr{Data: &js_ast.EString{Value: js_lexer.StringToUTF16(source.Contents)}}
		ast := js_parser.LazyExportAST(args.log, source, args.options, expr, "")
		ast.URLForCSS = "data:text/plain;base64," + encoded
		result.file.ignoreIfUnused = true
		result.file.repr = &reprJS{ast: ast}
		result.ok = true

	case config.LoaderBase64:
		mimeType := guessMimeType(args.fs.Ext(args.baseName), source.Contents)
		encoded := base64.StdEncoding.EncodeToString([]byte(source.Contents))
		expr := js_ast.Expr{Data: &js_ast.EString{Value: js_lexer.StringToUTF16(encoded)}}
		ast := js_parser.LazyExportAST(args.log, source, args.options, expr, "")
		ast.URLForCSS = "data:" + mimeType + ";base64," + encoded
		result.file.ignoreIfUnused = true
		result.file.repr = &reprJS{ast: ast}
		result.ok = true

	case config.LoaderBinary:
		encoded := base64.StdEncoding.EncodeToString([]byte(source.Contents))
		expr := js_ast.Expr{Data: &js_ast.EString{Value: js_lexer.StringToUTF16(encoded)}}
		ast := js_parser.LazyExportAST(args.log, source, args.options, expr, "__toBinary")
		ast.URLForCSS = "data:application/octet-stream;base64," + encoded
		result.file.ignoreIfUnused = true
		result.file.repr = &reprJS{ast: ast}
		result.ok = true

	case config.LoaderDataURL:
		mimeType := guessMimeType(args.fs.Ext(args.baseName), source.Contents)
		encoded := base64.StdEncoding.EncodeToString([]byte(source.Contents))
		url := "data:" + mimeType + ";base64," + encoded
		expr := js_ast.Expr{Data: &js_ast.EString{Value: js_lexer.StringToUTF16(url)}}
		ast := js_parser.LazyExportAST(args.log, source, args.options, expr, "")
		ast.URLForCSS = url
		result.file.ignoreIfUnused = true
		result.file.repr = &reprJS{ast: ast}
		result.ok = true

	case config.LoaderFile:
		// Add a hash to the file name to prevent multiple files with the same name
		// but different contents from colliding
		hash := hashForFileName([]byte(source.Contents))
		ext := path.Ext(args.baseName)
		baseName := args.baseName[:len(args.baseName)-len(ext)] + "." + hash + ext

		// Determine the destination folder
		targetFolder := args.options.AbsOutputDir

		// Export the resulting relative path as a string
		expr := js_ast.Expr{Data: &js_ast.EString{Value: js_lexer.StringToUTF16(baseName)}}
		ast := js_parser.LazyExportAST(args.log, source, args.options, expr, "")
		ast.URLForCSS = baseName
		result.file.ignoreIfUnused = true
		result.file.repr = &reprJS{ast: ast}
		result.ok = true

		// Optionally add metadata about the file
		var jsonMetadataChunk []byte
		if args.options.AbsMetadataFile != "" {
			jsonMetadataChunk = []byte(fmt.Sprintf(
				"{\n      \"inputs\": {},\n      \"bytes\": %d\n    }", len(source.Contents)))
		}

		// Copy the file using an additional file payload to make sure we only copy
		// the file if the module isn't removed due to tree shaking.
		result.file.additionalFiles = []OutputFile{{
			AbsPath:           args.fs.Join(targetFolder, baseName),
			Contents:          []byte(source.Contents),
			jsonMetadataChunk: jsonMetadataChunk,
		}}

	default:
		args.log.AddRangeError(args.importSource, args.importPathRange,
			fmt.Sprintf("File extension not supported: %s", args.prettyPath))
	}

	// Stop now if parsing failed
	if !result.ok {
		args.results <- result
		return
	}

	// Run the resolver on the parse thread so it's not run on the main thread.
	// That way the main thread isn't blocked if the resolver takes a while.
	if args.options.Mode == config.ModeBundle {
		records := result.file.repr.importRecords()
		result.resolveResults = make([]*resolver.ResolveResult, len(records))

		if len(records) > 0 {
			resolverCache := make(map[ast.ImportKind]map[string]*resolver.ResolveResult)

			// Resolve relative to the parent directory of the source file with the
			// import path. Just use the current directory if the source file is virtual.
			var sourceDir string
			if source.KeyPath.Namespace == "file" {
				sourceDir = args.fs.Dir(source.KeyPath.Text)
			} else if args.absResolveDir != "" {
				sourceDir = args.absResolveDir
			} else {
				sourceDir = args.fs.Cwd()
			}

			for importRecordIndex := range records {
				// Don't try to resolve imports that are already resolved
				record := &records[importRecordIndex]
				if record.SourceIndex != nil {
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
				resolveResult := args.res.Resolve(sourceDir, record.Path.Text, record.Kind)
				cache[record.Path.Text] = resolveResult

				// All "require.resolve()" imports should be external because we don't
				// want to waste effort traversing into them
				if record.Kind == ast.ImportRequireResolve {
					if !record.IsInsideTryBody && (resolveResult == nil || !resolveResult.IsExternal) {
						args.log.AddRangeWarning(&source, record.Range,
							fmt.Sprintf("%q should be marked as external for use with \"require.resolve\"", record.Path.Text))
					}
					continue
				}

				if resolveResult == nil {
					// Failed imports inside a try/catch are silently turned into
					// external imports instead of causing errors. This matches a common
					// code pattern for conditionally importing a module with a graceful
					// fallback.
					if !record.IsInsideTryBody {
						hint := ""
						if resolver.IsPackagePath(record.Path.Text) {
							hint = " (mark it as external to exclude it from the bundle)"
						}
						if args.options.Platform != config.PlatformNode {
							if _, ok := resolver.BuiltInNodeModules[record.Path.Text]; ok {
								hint = " (set platform to \"node\" when building for node)"
							}
						}
						args.log.AddRangeError(&source, record.Range,
							fmt.Sprintf("Could not resolve %q%s", record.Path.Text, hint))
					}
					continue
				}

				result.resolveResults[importRecordIndex] = resolveResult
			}
		}
	}

	// Attempt to parse the source map if present
	if loader.CanHaveSourceMap() && args.options.SourceMap != config.SourceMapNone {
		if repr, ok := result.file.repr.(*reprJS); ok && repr.ast.SourceMapComment.Text != "" {
			if path, contents := extractSourceMapFromComment(args.log, args.fs, args.res, &source, repr.ast.SourceMapComment); contents != nil {
				result.file.sourceMap = js_parser.ParseSourceMap(args.log, logger.Source{
					KeyPath:    path,
					PrettyPath: args.res.PrettyPath(path),
					Contents:   *contents,
				})
			}
		}
	}

	args.results <- result
}

func guessMimeType(extension string, contents string) string {
	mimeType := mime.TypeByExtension(extension)
	if mimeType == "" {
		mimeType = http.DetectContentType([]byte(contents))
	}

	// Turn "text/plain; charset=utf-8" into "text/plain;charset=utf-8"
	return strings.ReplaceAll(mimeType, "; ", ";")
}

func extractSourceMapFromComment(log logger.Log, fs fs.FS, res resolver.Resolver, source *logger.Source, comment js_ast.Span) (logger.Path, *string) {
	// Data URL
	if strings.HasPrefix(comment.Text, "data:") {
		if strings.HasPrefix(comment.Text, "data:application/json;") {
			// Scan for the base64 part to support URLs like "data:application/json;charset=utf-8;base64,"
			if index := strings.Index(comment.Text, ";base64,"); index != -1 {
				n := int32(index + len(";base64,"))
				encoded := comment.Text[n:]
				decoded, err := base64.StdEncoding.DecodeString(encoded)
				if err != nil {
					r := logger.Range{Loc: logger.Loc{Start: comment.Range.Loc.Start + n}, Len: comment.Range.Len - n}
					log.AddRangeWarning(source, r, "Invalid base64 data in source map")
					return logger.Path{}, nil
				}
				contents := string(decoded)
				return logger.Path{Text: source.PrettyPath + ".sourceMappingURL"}, &contents
			}
		}

		// Anything else is unsupported
		log.AddRangeWarning(source, comment.Range, "Unsupported source map comment")
		return logger.Path{}, nil
	}

	// Relative path in a file with an absolute path
	if source.KeyPath.Namespace == "file" {
		absPath := fs.Join(fs.Dir(source.KeyPath.Text), comment.Text)
		path := logger.Path{Text: absPath, Namespace: "file"}
		contents, err := fs.ReadFile(absPath)
		if err != nil {
			if err == syscall.ENOENT {
				// Don't report a warning because this is likely unactionable
				return logger.Path{}, nil
			}
			log.AddRangeError(source, comment.Range, fmt.Sprintf("Cannot read file %q: %s", res.PrettyPath(path), err.Error()))
			return logger.Path{}, nil
		}
		return path, &contents
	}

	// Anything else is unsupported
	log.AddRangeWarning(source, comment.Range, "Unsupported source map comment")
	return logger.Path{}, nil
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

func hashForFileName(bytes []byte) string {
	hashBytes := sha1.Sum(bytes)
	return base32.StdEncoding.EncodeToString(hashBytes[:])[:8]
}

func ScanBundle(log logger.Log, fs fs.FS, res resolver.Resolver, entryPaths []string, options config.Options) Bundle {
	results := []parseResult{}
	visited := make(map[string]uint32)
	resultChannel := make(chan parseResult)
	remaining := 0

	if options.ExtensionToLoader == nil {
		options.ExtensionToLoader = DefaultExtensionToLoaderMap()
	}

	// Always start by parsing the runtime file
	{
		results = append(results, parseResult{})
		remaining++
		go func() {
			source, ast, ok := globalRuntimeCache.parseRuntime(&options)
			resultChannel <- parseResult{file: file{source: source, repr: &reprJS{ast: ast}}, ok: ok}
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
		importSource *logger.Source,
		importPathRange logger.Range,
		absResolveDir string,
		kind inputKind,
	) uint32 {
		path := resolveResult.PathPair.Primary
		visitedKey := path.Text
		if path.Namespace == "file" {
			visitedKey = lowerCaseAbsPathForWindows(visitedKey)
		}
		sourceIndex, ok := visited[visitedKey]
		if !ok {
			sourceIndex = uint32(len(results))
			visited[visitedKey] = sourceIndex
			results = append(results, parseResult{})
			flags := parseFlags{
				isEntryPoint:   kind == inputKindEntryPoint,
				ignoreIfUnused: resolveResult.IgnoreIfUnused,
			}
			remaining++
			optionsClone := options
			if kind != inputKindStdin {
				optionsClone.Stdin = nil
			}
			optionsClone.SuppressWarningsAboutWeirdCode = resolveResult.SuppressWarningsAboutWeirdCode

			// Allow certain properties to be overridden
			if len(resolveResult.JSXFactory) > 0 {
				optionsClone.JSX.Factory = resolveResult.JSXFactory
			}
			if len(resolveResult.JSXFragment) > 0 {
				optionsClone.JSX.Fragment = resolveResult.JSXFragment
			}
			if resolveResult.StrictClassFields {
				optionsClone.Strict.ClassFields = true
			}

			go parseFile(parseArgs{
				fs:              fs,
				log:             log,
				res:             res,
				keyPath:         path,
				prettyPath:      prettyPath,
				baseName:        fs.Base(path.Text),
				sourceIndex:     sourceIndex,
				importSource:    importSource,
				flags:           flags,
				importPathRange: importPathRange,
				options:         optionsClone,
				results:         resultChannel,
				absResolveDir:   absResolveDir,
			})
		}
		return sourceIndex
	}

	entryPoints := []uint32{}
	duplicateEntryPoints := make(map[string]bool)

	// Treat stdin as an extra entry point
	if options.Stdin != nil {
		resolveResult := resolver.ResolveResult{PathPair: resolver.PathPair{Primary: logger.Path{Text: "<stdin>"}}}
		sourceIndex := maybeParseFile(resolveResult, "<stdin>", nil, logger.Range{}, options.Stdin.AbsResolveDir, inputKindStdin)
		entryPoints = append(entryPoints, sourceIndex)
	}

	// Add any remaining entry points
	for _, absPath := range entryPaths {
		prettyPath := res.PrettyPath(logger.Path{Text: absPath, Namespace: "file"})
		lowerAbsPath := lowerCaseAbsPathForWindows(absPath)

		if duplicateEntryPoints[lowerAbsPath] {
			log.AddError(nil, logger.Loc{}, fmt.Sprintf("Duplicate entry point %q", prettyPath))
			continue
		}

		duplicateEntryPoints[lowerAbsPath] = true
		resolveResult := res.ResolveAbs(absPath)

		if resolveResult == nil {
			log.AddError(nil, logger.Loc{}, fmt.Sprintf("Could not resolve %q", prettyPath))
			continue
		}

		sourceIndex := maybeParseFile(*resolveResult, prettyPath, nil, logger.Range{}, "", inputKindEntryPoint)
		entryPoints = append(entryPoints, sourceIndex)
	}

	// Continue scanning until all dependencies have been discovered
	for remaining > 0 {
		result := <-resultChannel
		remaining--
		if !result.ok {
			continue
		}

		// Don't try to resolve paths if we're not bundling
		if options.Mode == config.ModeBundle {
			records := result.file.repr.importRecords()
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
					prettyPath := res.PrettyPath(path)
					sourceIndex := maybeParseFile(*resolveResult, prettyPath, &result.file.source, record.Range, "", inputKindNormal)
					record.SourceIndex = &sourceIndex
				} else {
					// If the path to the external module is relative to the source
					// file, rewrite the path to be relative to the working directory
					if path.Namespace == "file" {
						if relPath, ok := fs.Rel(options.AbsOutputDir, path.Text); ok {
							// Prevent issues with path separators being different on Windows
							relPath = strings.ReplaceAll(relPath, "\\", "/")
							if resolver.IsPackagePath(relPath) {
								relPath = "./" + relPath
							}
							record.Path.Text = relPath
						}
					}
				}
			}
		}

		results[result.file.source.Index] = result
	}

	// Now that all files have been scanned, process the final file import records
	files := make([]file, len(results))
	for _, result := range results {
		if !result.ok {
			continue
		}

		j := js_printer.Joiner{}
		isFirstImport := true

		// Begin the metadata chunk
		if options.AbsMetadataFile != "" {
			j.AddBytes(js_printer.QuoteForJSON(result.file.source.PrettyPath))
			j.AddString(fmt.Sprintf(": {\n      \"bytes\": %d,\n      \"imports\": [", len(result.file.source.Contents)))
		}

		// Don't try to resolve paths if we're not bundling
		if options.Mode == config.ModeBundle {
			records := result.file.repr.importRecords()
			for importRecordIndex := range records {
				record := &records[importRecordIndex]

				// Skip this import record if the previous resolver call failed
				resolveResult := result.resolveResults[importRecordIndex]
				if resolveResult == nil || record.SourceIndex == nil {
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
					secondaryKey := resolveResult.PathPair.Secondary.Text
					if resolveResult.PathPair.Secondary.Namespace == "file" {
						secondaryKey = lowerCaseAbsPathForWindows(secondaryKey)
					}
					if secondarySourceIndex, ok := visited[secondaryKey]; ok {
						record.SourceIndex = &secondarySourceIndex
					}
				}

				// Generate metadata about each import
				if options.AbsMetadataFile != "" {
					if isFirstImport {
						isFirstImport = false
						j.AddString("\n        ")
					} else {
						j.AddString(",\n        ")
					}
					j.AddString(fmt.Sprintf("{\n          \"path\": %s\n        }",
						js_printer.QuoteForJSON(results[*record.SourceIndex].file.source.PrettyPath)))
				}

				// Importing a JavaScript file from a CSS file is not allowed.
				switch record.Kind {
				case ast.AtImport:
					otherFile := &results[*record.SourceIndex].file
					if _, ok := otherFile.repr.(*reprJS); ok {
						log.AddRangeError(&result.file.source, record.Range,
							fmt.Sprintf("Cannot import %q into a CSS file", otherFile.source.PrettyPath))
					}

				case ast.URLToken:
					otherFile := &results[*record.SourceIndex].file
					switch otherRepr := otherFile.repr.(type) {
					case *reprCSS:
						log.AddRangeError(&result.file.source, record.Range,
							fmt.Sprintf("Cannot use %q as a URL", otherFile.source.PrettyPath))

					case *reprJS:
						if otherRepr.ast.URLForCSS != "" {
							// Inline the URL if the loader supports URLs
							record.Path.Text = otherRepr.ast.URLForCSS
							record.Path.Namespace = ""
							record.SourceIndex = nil

							// Copy the additional files to the output directory
							result.file.additionalFiles = append(result.file.additionalFiles, otherFile.additionalFiles...)
						} else {
							log.AddRangeError(&result.file.source, record.Range,
								fmt.Sprintf("Cannot use %q as a URL", otherFile.source.PrettyPath))
						}
					}
				}

				// If an import from a JavaScript file targets a CSS file, generate a
				// JavaScript stub to ensure that JavaScript files only ever import
				// other JavaScript files.
				if _, ok := result.file.repr.(*reprJS); ok {
					otherFile := &results[*record.SourceIndex].file
					if css, ok := otherFile.repr.(*reprCSS); ok {
						if options.WriteToStdout {
							log.AddRangeError(&result.file.source, record.Range,
								fmt.Sprintf("Cannot import %q into a JavaScript file without an output path configured", otherFile.source.PrettyPath))
						} else if css.jsSourceIndex == nil {
							sourceIndex := uint32(len(files))
							source := logger.Source{
								Index:      sourceIndex,
								PrettyPath: otherFile.source.PrettyPath,
							}
							ast := js_parser.LazyExportAST(log, source, options, js_ast.Expr{Data: &js_ast.EObject{}}, "")
							f := file{
								repr: &reprJS{
									ast:            ast,
									cssSourceIndex: record.SourceIndex,
								},
								source: source,
							}
							files = append(files, f)
							results = append(results, parseResult{file: f})
							css.jsSourceIndex = &sourceIndex
						}
						record.SourceIndex = css.jsSourceIndex
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
		files[result.file.source.Index] = result.file
	}

	return Bundle{
		fs:          fs,
		res:         res,
		files:       files,
		entryPoints: entryPoints,
	}
}

func DefaultExtensionToLoaderMap() map[string]config.Loader {
	return map[string]config.Loader{
		".js":   config.LoaderJS,
		".mjs":  config.LoaderJS,
		".cjs":  config.LoaderJS,
		".jsx":  config.LoaderJSX,
		".ts":   config.LoaderTS,
		".tsx":  config.LoaderTSX,
		".css":  config.LoaderCSS,
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

	IsExecutable bool
}

func (b *Bundle) Compile(log logger.Log, options config.Options) []OutputFile {
	if options.ExtensionToLoader == nil {
		options.ExtensionToLoader = DefaultExtensionToLoaderMap()
	}

	// The format can't be "preserve" while bundling
	if options.Mode == config.ModeBundle && options.OutputFormat == config.FormatPreserve {
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
		c := newLinkerContext(&options, log, b.fs, b.res, b.files, b.entryPoints, lcaAbsPath)
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
				c := newLinkerContext(&options, log, b.fs, b.res, b.files, []uint32{entryPoint}, lcaAbsPath)
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
				keyPath := b.files[sourceIndex].source.KeyPath
				if keyPath.Namespace == "file" {
					lowerAbsPath := lowerCaseAbsPathForWindows(keyPath.Text)
					sourceAbsPaths[lowerAbsPath] = sourceIndex
				}
			}
		}
		for _, outputFile := range outputFiles {
			lowerAbsPath := lowerCaseAbsPathForWindows(outputFile.AbsPath)
			if sourceIndex, ok := sourceAbsPaths[lowerAbsPath]; ok {
				log.AddError(nil, logger.Loc{}, "Refusing to overwrite input file: "+b.files[sourceIndex].source.PrettyPath)
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
			lowerAbsPath := lowerCaseAbsPathForWindows(outputFile.AbsPath)
			contents, ok := outputFileMap[lowerAbsPath]

			// If this isn't a duplicate, keep the output file
			if !ok {
				outputFileMap[lowerAbsPath] = outputFile.Contents
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

	return outputFiles
}

func (b *Bundle) lowestCommonAncestorDirectory(codeSplitting bool) string {
	isEntryPoint := make(map[uint32]bool)
	for _, entryPoint := range b.entryPoints {
		isEntryPoint[entryPoint] = true
	}

	// If code splitting is enabled, also treat dynamic imports as entry points
	if codeSplitting {
		for _, sourceIndex := range findReachableFiles(b.files, b.entryPoints) {
			records := b.files[sourceIndex].repr.importRecords()
			for importRecordIndex := range records {
				if record := &records[importRecordIndex]; record.SourceIndex != nil && record.Kind == ast.ImportDynamic {
					isEntryPoint[*record.SourceIndex] = true
				}
			}
		}
	}

	// Ignore any paths for virtual modules (that don't exist on the file system)
	absPaths := make([]string, 0, len(isEntryPoint))
	for entryPoint := range isEntryPoint {
		keyPath := b.files[entryPoint].source.KeyPath
		if keyPath.Namespace == "file" {
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
	sorted := make(indexAndPathArray, 0, len(b.files))
	for sourceIndex, file := range b.files {
		if uint32(sourceIndex) != runtime.SourceIndex {
			sorted = append(sorted, indexAndPath{uint32(sourceIndex), file.source.KeyPath})
		}
	}
	sort.Sort(sorted)

	j := js_printer.Joiner{}
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
			j.AddString(fmt.Sprintf("%s: ", js_printer.QuoteForJSON(b.res.PrettyPath(
				logger.Path{Text: result.AbsPath, Namespace: "file"}))))
			j.AddBytes(result.jsonMetadataChunk)
		}
	}

	j.AddString("\n  }\n}\n")
	return j.Done()
}

type runtimeCacheKey struct {
	MangleSyntax      bool
	MinifyIdentifiers bool
	ES6               bool
	Platform          config.Platform
}

type runtimeCache struct {
	astMutex sync.Mutex
	astMap   map[runtimeCacheKey]js_ast.AST

	definesMutex sync.Mutex
	definesMap   map[config.Platform]*config.ProcessedDefines
}

var globalRuntimeCache runtimeCache

func (cache *runtimeCache) parseRuntime(options *config.Options) (source logger.Source, runtimeAST js_ast.AST, ok bool) {
	key := runtimeCacheKey{
		// All configuration options that the runtime code depends on must go here
		MangleSyntax:      options.MangleSyntax,
		MinifyIdentifiers: options.MinifyIdentifiers,
		Platform:          options.Platform,
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
	log := logger.NewDeferLog()
	runtimeAST, ok = js_parser.Parse(log, source, config.Options{
		// These configuration options must only depend on the key
		MangleSyntax:      key.MangleSyntax,
		MinifyIdentifiers: key.MinifyIdentifiers,
		Platform:          key.Platform,
		Defines:           cache.processedDefines(key.Platform),

		// Always do tree shaking for the runtime because we never want to
		// include unnecessary runtime code
		Mode: config.ModeBundle,
	})
	if log.HasErrors() {
		panic("Internal error: failed to parse runtime")
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
			DefineFunc: func(logger.Loc, config.FindSymbol) js_ast.E {
				return &js_ast.EString{Value: js_lexer.StringToUTF16(platform)}
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
