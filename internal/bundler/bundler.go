package bundler

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"path"
	"sync"

	"github.com/evanw/esbuild/internal/ast"
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

	// This maps the non-unique import path present in the source file to the
	// unique source index for that module. This isn't unique because two paths
	// in the source file may refer to the same module:
	//
	//   import "../lib/util";
	//   import "./util";
	//
	// This is used by the printer to write out the source index for modules that
	// are referenced in the AST.
	resolvedImports map[string]uint32

	// If this file ends up being used in the bundle, this is an additional file
	// that must be written to the output directory. It's used by the "file"
	// loader.
	additionalFile *OutputFile
}

func (f *file) resolveImport(path ast.Path) (uint32, bool) {
	if path.UseSourceIndex {
		return path.SourceIndex, true
	}
	sourceIndex, ok := f.resolvedImports[path.Text]
	return sourceIndex, ok
}

type Bundle struct {
	fs          fs.FS
	sources     []logging.Source
	files       []file
	entryPoints []uint32
}

type parseResult struct {
	source         logging.Source
	ast            ast.AST
	ok             bool
	additionalFile *OutputFile
}

func parseFile(
	fs fs.FS,
	log logging.Log,
	res resolver.Resolver,
	sourcePath string,
	sourceIndex uint32,
	isStdin bool,
	importSource logging.Source,
	isDisabled bool,
	pathRange ast.Range,
	parseOptions parser.ParseOptions,
	bundleOptions BundleOptions,
	results chan parseResult,
) {
	prettyPath := sourcePath
	if !isStdin {
		prettyPath = res.PrettyPath(sourcePath)
	}
	contents := ""

	// Disabled files are left empty
	if !isDisabled {
		if isStdin {
			bytes, err := ioutil.ReadAll(os.Stdin)
			if err != nil {
				log.AddRangeError(importSource, pathRange, fmt.Sprintf("Could not read from stdin: %s", err.Error()))
				results <- parseResult{}
				return
			}
			contents = string(bytes)
		} else {
			var ok bool
			contents, ok = res.Read(sourcePath)
			if !ok {
				log.AddRangeError(importSource, pathRange, fmt.Sprintf("Could not read from file: %s", sourcePath))
				results <- parseResult{}
				return
			}
		}
	}

	source := logging.Source{
		Index:        sourceIndex,
		IsStdin:      isStdin,
		AbsolutePath: sourcePath,
		PrettyPath:   prettyPath,
		Contents:     contents,
	}

	// Get the file extension
	extension := path.Ext(sourcePath)

	// Pick the loader based on the file extension
	loader := bundleOptions.ExtensionToLoader[extension]

	// Special-case reading from stdin
	if bundleOptions.LoaderForStdin != LoaderNone && source.IsStdin {
		loader = bundleOptions.LoaderForStdin
	}

	switch loader {
	case LoaderJS:
		ast, ok := parser.Parse(log, source, parseOptions)
		results <- parseResult{source, ast, ok, nil}

	case LoaderJSX:
		parseOptions.JSX.Parse = true
		ast, ok := parser.Parse(log, source, parseOptions)
		results <- parseResult{source, ast, ok, nil}

	case LoaderTS:
		parseOptions.TS.Parse = true
		ast, ok := parser.Parse(log, source, parseOptions)
		results <- parseResult{source, ast, ok, nil}

	case LoaderTSX:
		parseOptions.TS.Parse = true
		parseOptions.JSX.Parse = true
		ast, ok := parser.Parse(log, source, parseOptions)
		results <- parseResult{source, ast, ok, nil}

	case LoaderJSON:
		expr, ok := parser.ParseJSON(log, source, parser.ParseJSONOptions{})
		ast := parser.ModuleExportsAST(log, source, parseOptions, expr)
		results <- parseResult{source, ast, ok, nil}

	case LoaderText:
		expr := ast.Expr{ast.Loc{0}, &ast.EString{lexer.StringToUTF16(source.Contents)}}
		ast := parser.ModuleExportsAST(log, source, parseOptions, expr)
		results <- parseResult{source, ast, true, nil}

	case LoaderBase64:
		encoded := base64.StdEncoding.EncodeToString([]byte(source.Contents))
		expr := ast.Expr{ast.Loc{0}, &ast.EString{lexer.StringToUTF16(encoded)}}
		ast := parser.ModuleExportsAST(log, source, parseOptions, expr)
		results <- parseResult{source, ast, true, nil}

	case LoaderDataURL:
		mimeType := mime.TypeByExtension(extension)
		if mimeType == "" {
			mimeType = http.DetectContentType([]byte(source.Contents))
		}
		encoded := base64.StdEncoding.EncodeToString([]byte(source.Contents))
		url := "data:" + mimeType + ";base64," + encoded
		expr := ast.Expr{ast.Loc{0}, &ast.EString{lexer.StringToUTF16(url)}}
		ast := parser.ModuleExportsAST(log, source, parseOptions, expr)
		results <- parseResult{source, ast, true, nil}

	case LoaderFile:
		// Get the file name, making sure to use the "fs" interface so we do the
		// right thing on Windows (Windows-style paths for the command-line
		// interface and Unix-style paths for tests, even on Windows)
		baseName := fs.Base(sourcePath)

		// Add a hash to the file name to prevent multiple files with the same name
		// but different contents from colliding
		bytes := []byte(source.Contents)
		hashBytes := sha1.Sum(bytes)
		hash := base64.StdEncoding.EncodeToString(hashBytes[:])[:8]
		baseName = baseName[:len(baseName)-len(extension)] + "." + hash + extension

		// Determine the destination folder
		targetFolder := bundleOptions.AbsOutputDir
		if targetFolder == "" {
			targetFolder = fs.Dir(bundleOptions.AbsOutputFile)
		}

		// Export the resulting relative path as a string
		expr := ast.Expr{ast.Loc{0}, &ast.EString{lexer.StringToUTF16(baseName)}}
		ast := parser.ModuleExportsAST(log, source, parseOptions, expr)

		// Copy the file using an additional file payload to make sure we only copy
		// the file if the module isn't removed due to tree shaking.
		results <- parseResult{source, ast, true, &OutputFile{
			AbsPath:  fs.Join(targetFolder, baseName),
			Contents: bytes,
		}}

	default:
		log.AddRangeError(importSource, pathRange, fmt.Sprintf("File extension not supported: %s", sourcePath))
		results <- parseResult{}
	}
}

func ScanBundle(
	log logging.Log, fs fs.FS, res resolver.Resolver, entryPaths []string,
	parseOptions parser.ParseOptions, bundleOptions BundleOptions,
) Bundle {
	sources := []logging.Source{}
	files := []file{}
	visited := make(map[string]uint32)
	results := make(chan parseResult)
	remaining := 0

	if bundleOptions.ExtensionToLoader == nil {
		bundleOptions.ExtensionToLoader = DefaultExtensionToLoaderMap()
	}

	// Always start by parsing the runtime file
	{
		source := logging.Source{
			Index:        ast.RuntimeSourceIndex,
			AbsolutePath: "<runtime>",
			PrettyPath:   "<runtime>",
			Contents:     runtime.Code,
		}
		sources = append(sources, source)
		files = append(files, file{})
		remaining++
		go func() {
			runtimeParseOptions := parseOptions

			// Always do tree shaking for the runtime because we never want to
			// include unnecessary runtime code
			runtimeParseOptions.IsBundling = true

			ast, ok := parser.Parse(log, source, runtimeParseOptions)
			results <- parseResult{source, ast, ok, nil}
		}()
	}

	type parseFileFlags struct {
		isEntryPoint bool
		isDisabled   bool
	}

	maybeParseFile := func(path string, importSource logging.Source, pathRange ast.Range, flags parseFileFlags) uint32 {
		sourceIndex, ok := visited[path]
		if !ok {
			sourceIndex = uint32(len(sources))
			isStdin := bundleOptions.LoaderForStdin != LoaderNone && flags.isEntryPoint
			if !isStdin {
				visited[path] = sourceIndex
			}
			sources = append(sources, logging.Source{})
			files = append(files, file{})
			remaining++
			go parseFile(
				fs,
				log,
				res,
				path,
				sourceIndex,
				isStdin,
				importSource,
				flags.isDisabled,
				pathRange,
				parseOptions,
				bundleOptions,
				results,
			)
		}
		return sourceIndex
	}

	entryPoints := []uint32{}
	for _, path := range entryPaths {
		flags := parseFileFlags{isEntryPoint: true}
		sourceIndex := maybeParseFile(path, logging.Source{}, ast.Range{}, flags)
		entryPoints = append(entryPoints, sourceIndex)
	}

	for remaining > 0 {
		result := <-results
		remaining--
		if !result.ok {
			continue
		}

		source := result.source
		resolvedImports := make(map[string]uint32)

		// Don't try to resolve paths if we're not bundling
		if bundleOptions.IsBundling {
			for _, part := range result.ast.Parts {
				for _, importPath := range part.ImportPaths {
					// Don't try to resolve imports of the special runtime path
					if importPath.Path.UseSourceIndex && importPath.Path.SourceIndex == ast.RuntimeSourceIndex {
						continue
					}

					sourcePath := source.AbsolutePath
					pathText := importPath.Path.Text
					pathRange := source.RangeOfString(importPath.Path.Loc)

					switch path, status := res.Resolve(sourcePath, pathText); status {
					case resolver.ResolveEnabled, resolver.ResolveDisabled:
						flags := parseFileFlags{isDisabled: status == resolver.ResolveDisabled}
						sourceIndex := maybeParseFile(path, source, pathRange, flags)
						resolvedImports[pathText] = sourceIndex

					case resolver.ResolveMissing:
						log.AddRangeError(source, pathRange, fmt.Sprintf("Could not resolve %q", pathText))
					}
				}
			}
		}

		sources[source.Index] = source
		files[source.Index] = file{result.ast, resolvedImports, result.additionalFile}
	}

	return Bundle{fs, sources, files, entryPoints}
}

type Loader int

const (
	LoaderNone Loader = iota
	LoaderJS
	LoaderJSX
	LoaderTS
	LoaderTSX
	LoaderJSON
	LoaderText
	LoaderBase64
	LoaderDataURL
	LoaderFile
)

func DefaultExtensionToLoaderMap() map[string]Loader {
	return map[string]Loader{
		".js":   LoaderJS,
		".mjs":  LoaderJS,
		".cjs":  LoaderJS,
		".jsx":  LoaderJSX,
		".ts":   LoaderTS,
		".tsx":  LoaderTSX,
		".json": LoaderJSON,
		".txt":  LoaderText,
	}
}

type SourceMap uint8

const (
	SourceMapNone SourceMap = iota
	SourceMapInline
	SourceMapLinkedWithComment
	SourceMapExternalWithoutComment
)

type BundleOptions struct {
	// true: imports are scanned and bundled along with the file
	// false: imports are left alone and the file is passed through as-is
	IsBundling bool

	AbsOutputFile     string
	AbsOutputDir      string
	RemoveWhitespace  bool
	MinifyIdentifiers bool
	MangleSyntax      bool
	ModuleName        string
	ExtensionToLoader map[string]Loader
	OutputFormat      printer.Format

	SourceMap  SourceMap
	SourceFile string // The "original file path" for the source map

	// If this isn't LoaderNone, all entry point contents are assumed to come
	// from stdin and must be loaded with this loader
	LoaderForStdin Loader

	// If true, make sure to generate a single file that can be written to stdout
	WriteToStdout bool

	omitRuntimeForTests bool
}

type OutputFile struct {
	AbsPath  string
	Contents []byte
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

func (b *Bundle) Compile(log logging.Log, options BundleOptions) []OutputFile {
	if options.ExtensionToLoader == nil {
		options.ExtensionToLoader = DefaultExtensionToLoaderMap()
	}

	// The format can't be "preserve" while bundling
	if options.IsBundling && options.OutputFormat == printer.FormatPreserve {
		options.OutputFormat = printer.FormatESModule
	}

	waitGroup := sync.WaitGroup{}
	resultGroups := make([][]OutputFile, len(b.entryPoints))

	// Link each file with the runtime file separately in parallel
	for i, entryPoint := range b.entryPoints {
		waitGroup.Add(1)
		go func(i int, entryPoint uint32) {
			c := newLinkerContext(&options, log, b.fs, b.sources, b.files, []uint32{entryPoint})
			resultGroups[i] = c.link()
			waitGroup.Done()
		}(i, entryPoint)
	}
	waitGroup.Wait()

	// Join the results in entry point order for determinism
	var results []OutputFile
	for _, group := range resultGroups {
		results = append(results, group...)
	}
	return results
}
