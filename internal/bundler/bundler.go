package bundler

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"strings"

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
}

func (f *file) resolveImport(path ast.Path) (uint32, bool) {
	if path.IsRuntime {
		return runtimeSourceIndex, true
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
	source logging.Source
	ast    ast.AST
	ok     bool
}

func parseFile(
	log logging.Log,
	res resolver.Resolver,
	path string,
	sourceIndex uint32,
	isStdin bool,
	importSource logging.Source,
	isDisabled bool,
	pathRange ast.Range,
	parseOptions parser.ParseOptions,
	bundleOptions BundleOptions,
	results chan parseResult,
) {
	prettyPath := path
	if !isStdin {
		prettyPath = res.PrettyPath(path)
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
			contents, ok = res.Read(path)
			if !ok {
				log.AddRangeError(importSource, pathRange, fmt.Sprintf("Could not read from file: %s", path))
				results <- parseResult{}
				return
			}
		}
	}

	source := logging.Source{
		Index:        sourceIndex,
		IsStdin:      isStdin,
		AbsolutePath: path,
		PrettyPath:   prettyPath,
		Contents:     contents,
	}

	// Get the file extension
	extension := ""
	if lastDot := strings.LastIndexByte(path, '.'); lastDot >= 0 {
		extension = path[lastDot:]
	}

	// Pick the loader based on the file extension
	loader := bundleOptions.ExtensionToLoader[extension]

	// Special-case reading from stdin
	if bundleOptions.LoaderForStdin != LoaderNone && source.IsStdin {
		loader = bundleOptions.LoaderForStdin
	}

	switch loader {
	case LoaderJS:
		ast, ok := parser.Parse(log, source, parseOptions)
		results <- parseResult{source, ast, ok}

	case LoaderJSX:
		parseOptions.JSX.Parse = true
		ast, ok := parser.Parse(log, source, parseOptions)
		results <- parseResult{source, ast, ok}

	case LoaderTS:
		parseOptions.TS.Parse = true
		ast, ok := parser.Parse(log, source, parseOptions)
		results <- parseResult{source, ast, ok}

	case LoaderTSX:
		parseOptions.TS.Parse = true
		parseOptions.JSX.Parse = true
		ast, ok := parser.Parse(log, source, parseOptions)
		results <- parseResult{source, ast, ok}

	case LoaderJSON:
		expr, ok := parser.ParseJSON(log, source, parser.ParseJSONOptions{})
		ast := parser.ModuleExportsAST(log, source, parseOptions, expr)
		results <- parseResult{source, ast, ok}

	case LoaderText:
		expr := ast.Expr{ast.Loc{0}, &ast.EString{lexer.StringToUTF16(source.Contents)}}
		ast := parser.ModuleExportsAST(log, source, parseOptions, expr)
		results <- parseResult{source, ast, true}

	case LoaderBase64:
		encoded := base64.StdEncoding.EncodeToString([]byte(source.Contents))
		expr := ast.Expr{ast.Loc{0}, &ast.EString{lexer.StringToUTF16(encoded)}}
		ast := parser.ModuleExportsAST(log, source, parseOptions, expr)
		results <- parseResult{source, ast, true}

	case LoaderDataURL:
		mimeType := mime.TypeByExtension(extension)
		if mimeType == "" {
			mimeType = http.DetectContentType([]byte(source.Contents))
		}
		encoded := base64.StdEncoding.EncodeToString([]byte(source.Contents))
		url := "data:" + mimeType + ";base64," + encoded
		expr := ast.Expr{ast.Loc{0}, &ast.EString{lexer.StringToUTF16(url)}}
		ast := parser.ModuleExportsAST(log, source, parseOptions, expr)
		results <- parseResult{source, ast, true}

	default:
		log.AddRangeError(importSource, pathRange, fmt.Sprintf("File extension not supported: %s", path))
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
			Index:        runtimeSourceIndex,
			AbsolutePath: "<runtime>",
			PrettyPath:   "<runtime>",
			Contents:     runtime.Code,
		}
		sources = append(sources, source)
		files = append(files, file{})
		remaining++
		go func() {
			ast, ok := parser.Parse(log, source, parseOptions)
			results <- parseResult{source, ast, ok}
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

		for i, part := range result.ast.Parts {
			importPathsEnd := 0
			for _, importPath := range part.ImportPaths {
				// Don't try to resolve imports of the special runtime path
				if importPath.Path.IsRuntime {
					part.ImportPaths[importPathsEnd] = importPath
					importPathsEnd++
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
					part.ImportPaths[importPathsEnd] = importPath
					importPathsEnd++

				case resolver.ResolveMissing:
					log.AddRangeError(source, pathRange, fmt.Sprintf("Could not resolve %q", pathText))
				}
			}

			result.ast.Parts[i].ImportPaths = part.ImportPaths[:importPathsEnd]
		}

		sources[source.Index] = source
		files[source.Index] = file{result.ast, resolvedImports}
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

type Format uint8

const (
	FormatNone Format = iota

	// IIFE stands for immediately-invoked function expression. That looks like
	// this:
	//
	//   (() => {
	//     ... bundled code ...
	//   })();
	//
	// If the optional ModuleName is configured, then we'll write out this:
	//
	//   let moduleName = (() => {
	//     ... bundled code ...
	//     return exports;
	//   })();
	//
	FormatIIFE

	// The CommonJS format looks like this:
	//
	//   ... bundled code ...
	//   module.exports = exports;
	//
	FormatCommonJS

	// The ES module format looks like this:
	//
	//   ... bundled code ...
	//   export {...};
	//
	FormatESModule
)

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

	// If true, unused code is removed. If false, all code is kept.
	TreeShaking bool

	AbsOutputFile     string
	AbsOutputDir      string
	RemoveWhitespace  bool
	MinifyIdentifiers bool
	MangleSyntax      bool
	ModuleName        string
	ExtensionToLoader map[string]Loader
	OutputFormat      Format

	SourceMap  SourceMap
	SourceFile string // The "original file path" for the source map

	// If this isn't LoaderNone, all entry point contents are assumed to come
	// from stdin and must be loaded with this loader
	LoaderForStdin Loader

	// If true, make sure to generate a single file that can be written to stdout
	WriteToStdout bool

	omitRuntimeForTests bool
}

type BundleResult struct {
	JsAbsPath         string
	JsContents        []byte
	SourceMapAbsPath  string
	SourceMapContents []byte
}

type lineColumnOffset struct {
	lines   int
	columns int
}

type compileResult struct {
	printer.PrintResult

	sourceIndex uint32

	// This is the line and column offset since the previous JavaScript string
	// or the start of the file if this is the first JavaScript string.
	generatedOffset lineColumnOffset

	// The source map contains the original source code, which is quoted in
	// parallel for speed. This is only filled in if the SourceMap option is
	// enabled.
	quotedSource string
}

func (b *Bundle) Compile(log logging.Log, options BundleOptions) []BundleResult {
	if options.ExtensionToLoader == nil {
		options.ExtensionToLoader = DefaultExtensionToLoaderMap()
	}

	if options.OutputFormat == FormatNone {
		options.OutputFormat = FormatIIFE
	}

	c := newLinkerContext(&options, log, b.fs, b.sources, b.files, b.entryPoints)
	return c.link()
}

const runtimeSourceIndex = 0
