package bundler

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"strings"
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

func (b *Bundle) compileFile(
	options *BundleOptions, sourceIndex uint32, f file, sourceIndexToOutputIndex []uint32,
) compileResult {
	sourceMapContents := &b.sources[sourceIndex].Contents
	if options.SourceMap == SourceMapNone {
		sourceMapContents = nil
	}
	tree := f.ast
	indent := 0
	if options.IsBundling {
		if options.OutputFormat == FormatIIFE {
			indent++
		}
		if sourceIndex != runtimeSourceIndex {
			indent++
			if !options.omitRuntimeForTests {
				indent++
			}
		}
	}

	// Remap source indices to make the output deterministic
	var remappedResolvedImports map[string]uint32
	if options.IsBundling {
		remappedResolvedImports = make(map[string]uint32)
		for k, v := range f.resolvedImports {
			remappedResolvedImports[k] = sourceIndexToOutputIndex[v]
		}
	}

	result := compileResult{PrintResult: printer.Print(tree, printer.PrintOptions{
		RemoveWhitespace:  options.RemoveWhitespace,
		SourceMapContents: sourceMapContents,
		Indent:            indent,
		ResolvedImports:   remappedResolvedImports,
	})}
	if options.SourceMap != SourceMapNone {
		result.quotedSource = printer.QuoteForJSON(b.sources[sourceIndex].Contents)
	}
	return result
}

func (b *Bundle) generateSourceMapForEntryPoint(
	compileResults map[uint32]*compileResult, generatedOffsets map[uint32]lineColumnOffset,
	groups [][]uint32, options *BundleOptions, item *BundleResult,
) {
	buffer := []byte{}
	buffer = append(buffer, "{\n  \"version\": 3"...)

	// Write the sources
	buffer = append(buffer, ",\n  \"sources\": ["...)
	comma := ""
	for _, group := range groups {
		for _, sourceIndex := range group {
			sourceFile := b.sources[sourceIndex].PrettyPath
			if options.SourceFile != "" {
				sourceFile = options.SourceFile
			}
			buffer = append(buffer, comma...)
			buffer = append(buffer, printer.QuoteForJSON(sourceFile)...)
			comma = ", "
		}
	}
	buffer = append(buffer, ']')

	// Write the sourcesContent
	buffer = append(buffer, ",\n  \"sourcesContent\": ["...)
	comma = ""
	for _, group := range groups {
		for _, sourceIndex := range group {
			buffer = append(buffer, comma...)
			buffer = append(buffer, compileResults[sourceIndex].quotedSource...)
			comma = ", "
		}
	}
	buffer = append(buffer, ']')

	// Write the mappings
	buffer = append(buffer, ",\n  \"mappings\": \""...)
	prevEndState := printer.SourceMapState{}
	prevColumnOffset := 0
	sourceMapIndex := 0
	for _, group := range groups {
		for _, sourceIndex := range group {
			chunk := compileResults[sourceIndex].SourceMapChunk
			offset := generatedOffsets[sourceIndex]

			// Because each file for the bundle is converted to a source map once,
			// the source maps are shared between all entry points in the bundle.
			// The easiest way of getting this to work is to have all source maps
			// generate as if their source index is 0. We then adjust the source
			// index per entry point by modifying the first source mapping. This
			// is done by AppendSourceMapChunk() using the source index passed
			// here.
			startState := printer.SourceMapState{SourceIndex: sourceMapIndex}

			// Advance the state by the line/column offset from the previous chunk
			startState.GeneratedColumn += offset.columns
			if offset.lines > 0 {
				buffer = append(buffer, bytes.Repeat([]byte{';'}, offset.lines)...)
			} else {
				startState.GeneratedColumn += prevColumnOffset
			}

			// Append the precomputed source map chunk
			buffer = printer.AppendSourceMapChunk(buffer, prevEndState, startState, chunk.Buffer)

			// Generate the relative offset to start from next time
			prevEndState = chunk.EndState
			prevEndState.SourceIndex = sourceMapIndex
			prevColumnOffset = chunk.FinalGeneratedColumn

			// If this was all one line, include the column offset from the start
			if prevEndState.GeneratedLine == 0 {
				prevEndState.GeneratedColumn += startState.GeneratedColumn
				prevColumnOffset += startState.GeneratedColumn
			}

			sourceMapIndex++
		}
	}
	buffer = append(buffer, '"')

	// Finish the source map
	buffer = append(buffer, ",\n  \"names\": []\n}\n"...)

	// Generate the output
	switch options.SourceMap {
	case SourceMapInline:
		if options.RemoveWhitespace {
			item.JsContents = removeTrailing(item.JsContents, '\n')
		}
		item.JsContents = append(item.JsContents,
			("//# sourceMappingURL=data:application/json;base64," + base64.StdEncoding.EncodeToString(buffer) + "\n")...)

	case SourceMapLinkedWithComment, SourceMapExternalWithoutComment:
		item.SourceMapAbsPath = item.JsAbsPath + ".map"
		item.SourceMapContents = buffer

		// Add a comment linking the source to its map
		if options.SourceMap == SourceMapLinkedWithComment {
			if options.RemoveWhitespace {
				item.JsContents = removeTrailing(item.JsContents, '\n')
			}
			item.JsContents = append(item.JsContents,
				("//# sourceMappingURL=" + b.fs.Base(item.SourceMapAbsPath) + "\n")...)
		}
	}
}

func markExportsAsUnboundInDecls(decls []ast.Decl, symbols ast.SymbolMap) {
	var visitBinding func(ast.Binding)

	visitBinding = func(binding ast.Binding) {
		switch b := binding.Data.(type) {
		case *ast.BMissing:

		case *ast.BIdentifier:
			symbols.SetKind(b.Ref, ast.SymbolUnbound)

		case *ast.BArray:
			for _, i := range b.Items {
				visitBinding(i.Binding)
			}

		case *ast.BObject:
			for _, p := range b.Properties {
				visitBinding(p.Value)
			}

		default:
			panic(fmt.Sprintf("Unexpected binding of type %T", binding.Data))
		}
	}

	for _, decl := range decls {
		visitBinding(decl.Binding)
	}
}

// Marking a symbol as unbound prevents it from being renamed or minified.
// This is only used when a module is compiled independently. We use a very
// different way of handling exports and renaming/minifying when bundling.
func (b *Bundle) markExportsAsUnbound(f file, symbols ast.SymbolMap) {
	hasImportOrExport := false

	for _, part := range f.ast.Parts {
		for _, stmt := range part.Stmts {
			switch s := stmt.Data.(type) {
			case *ast.SImport:
				hasImportOrExport = true

			case *ast.SLocal:
				if s.IsExport {
					markExportsAsUnboundInDecls(s.Decls, symbols)
					hasImportOrExport = true
				}

			case *ast.SFunction:
				if s.IsExport {
					symbols.SetKind(s.Fn.Name.Ref, ast.SymbolUnbound)
					hasImportOrExport = true
				}

			case *ast.SClass:
				if s.IsExport {
					symbols.SetKind(s.Class.Name.Ref, ast.SymbolUnbound)
					hasImportOrExport = true
				}

			case *ast.SExportClause, *ast.SExportDefault, *ast.SExportStar, *ast.SExportFrom:
				hasImportOrExport = true
			}
		}
	}

	// Heuristic: If this module has top-level import or export statements, we
	// consider this an ES6 module and only preserve the names of the exported
	// symbols. Everything else is minified since the names are private.
	//
	// Otherwise, we consider this potentially a script-type file instead of an
	// ES6 module. In that case, preserve the names of all top-level symbols
	// since they are all potentially exported (e.g. if this is used in a
	// <script> tag). All symbols in nested scopes are still minified.
	if !hasImportOrExport {
		for _, ref := range f.ast.ModuleScope.Members {
			symbols.SetKind(ref, ast.SymbolUnbound)
		}
	}
}

// Ensures all symbol names are valid non-colliding identifiers
func (b *Bundle) renameOrMinifyAllSymbols(files []file, symbols ast.SymbolMap, group []uint32, options *BundleOptions) {
	// Operate on all module-level scopes in this module group
	moduleScopes := make([]*ast.Scope, len(group))
	for i, sourceIndex := range group {
		moduleScopes[i] = files[sourceIndex].ast.ModuleScope
	}

	// Rename all internal "exports" symbols to something more helpful. These
	// names don't have to be unique because the renaming pass below will
	// assign them unique names.
	for _, sourceIndex := range group[:len(group)-1] {
		ref := files[sourceIndex].ast.ExportsRef
		symbol := symbols.Get(ref)
		symbol.Name = ast.GenerateNonUniqueNameFromPath(b.sources[sourceIndex].AbsolutePath)
		symbols.Set(ref, symbol)
	}

	// Avoid collisions with any unbound symbols in this module group
	reservedNames := computeReservedNames(moduleScopes, symbols)
	if options.IsBundling {
		// These are used to implement bundling, and need to be free for use
		reservedNames["require"] = true
		reservedNames["Promise"] = true

		// Avoid collisions with symbols in the runtime's top-level scope
		for _, ref := range files[runtimeSourceIndex].ast.ModuleScope.Members {
			reservedNames[symbols.Get(ref).Name] = true
		}
	}

	nestedScopes := []*ast.Scope{}
	for _, scope := range moduleScopes {
		nestedScopes = append(nestedScopes, scope.Children...)
	}

	if options.MinifyIdentifiers {
		minifyAllSymbols(reservedNames, moduleScopes, nestedScopes, symbols)
	} else {
		renameAllSymbols(reservedNames, moduleScopes, nestedScopes, symbols)
	}
}

func (b *Bundle) Compile(log logging.Log, options BundleOptions) []BundleResult {
	if options.ExtensionToLoader == nil {
		options.ExtensionToLoader = DefaultExtensionToLoaderMap()
	}

	if options.OutputFormat == FormatNone {
		options.OutputFormat = FormatIIFE
	}

	if options.IsBundling {
		return b.compileBundle(log, &options)
	} else {
		return b.compileIndependent(log, &options)
	}
}

func (b *Bundle) checkOverwrite(log logging.Log, sourceIndex uint32, path string) {
	if path == b.sources[sourceIndex].AbsolutePath {
		log.AddError(logging.Source{}, ast.Loc{},
			fmt.Sprintf("Refusing to overwrite input file %q (use --outfile or --outdir to configure the output)",
				b.sources[sourceIndex].PrettyPath))
	}
}

func (b *Bundle) compileIndependent(log logging.Log, options *BundleOptions) []BundleResult {
	// When spawning a new goroutine, make sure to manually forward all variables
	// that are different for every iteration of the loop. Otherwise each
	// goroutine will share the same copy of the closed-over variables and cause
	// correctness issues.

	// Spawn parallel jobs to print the AST of each file in the bundle
	results := make([]BundleResult, len(b.sources))
	waitGroup := sync.WaitGroup{}
	for sourceIndex, _ := range b.files {
		waitGroup.Add(1)
		go func(sourceIndex uint32) {
			// Don't emit the runtime to a file
			if sourceIndex == runtimeSourceIndex {
				waitGroup.Done()
				return
			}

			// Form a module group with just the runtime and this file
			group := []uint32{runtimeSourceIndex, sourceIndex}
			symbols := ast.NewSymbolMap(len(b.files))
			files := make([]file, len(b.files))
			for _, si := range group {
				files[si] = b.files[si]
				symbols.Outer[si] = append([]ast.Symbol{}, files[si].ast.Symbols.Outer[si]...)
				files[si].ast.Symbols = symbols
			}

			// Trim unused runtime code
			f := files[sourceIndex]

			// Make sure we don't rename exports
			b.markExportsAsUnbound(f, symbols)

			// Rename symbols
			b.renameOrMinifyAllSymbols(files, symbols, group, options)

			// Print the JavaScript code
			generatedOffsets := make(map[uint32]lineColumnOffset)
			runtimeResult := b.compileFile(options, runtimeSourceIndex, files[runtimeSourceIndex], []uint32{})
			result := b.compileFile(options, sourceIndex, f, []uint32{})
			js := []byte{}
			if f.ast.Hashbang != "" {
				js = append(js, []byte(f.ast.Hashbang+"\n")...)
			}
			js = append(js, runtimeResult.JS...)
			generatedOffsets[sourceIndex] = computeLineColumnOffset(js)
			js = append(js, result.JS...)

			// Make a filename for the resulting JavaScript file
			jsName := b.outputFileForEntryPoint(sourceIndex, options)

			// Generate the resulting JavaScript file
			item := &results[sourceIndex]
			item.JsAbsPath = b.outputPathForEntryPoint(sourceIndex, jsName, options)
			item.JsContents = addTrailing(js, '\n')

			// Optionally also generate a source map
			if options.SourceMap != SourceMapNone {
				compileResults := map[uint32]*compileResult{sourceIndex: &result}
				groups := [][]uint32{[]uint32{sourceIndex}}
				b.generateSourceMapForEntryPoint(compileResults, generatedOffsets, groups, options, item)
			}

			// Refuse to overwrite the input file
			b.checkOverwrite(log, sourceIndex, item.JsAbsPath)

			waitGroup.Done()
		}(uint32(sourceIndex))
	}

	// Wait for all jobs to finish
	waitGroup.Wait()

	// Skip over the slot for the runtime, which was never filled out
	return results[1:]
}

func (b *Bundle) compileBundle(log logging.Log, options *BundleOptions) []BundleResult {
	c := newLinkerContext(options, log, b.fs, b.sources, b.files, b.entryPoints)
	return c.link()
}

func (b *Bundle) outputFileForEntryPoint(entryPoint uint32, options *BundleOptions) string {
	if options.WriteToStdout {
		return "<stdout>"
	} else if options.AbsOutputFile != "" {
		return b.fs.Base(options.AbsOutputFile)
	}
	name := b.fs.Base(b.sources[entryPoint].AbsolutePath)

	// Strip known file extensions
	for ext, _ := range options.ExtensionToLoader {
		if strings.HasSuffix(name, ext) {
			name = name[:len(name)-len(ext)]
			break
		}
	}

	// Add the appropriate file extension
	name += ".js"
	return name
}

func (b *Bundle) outputPathForEntryPoint(entryPoint uint32, jsName string, options *BundleOptions) string {
	if options.WriteToStdout {
		return "<stdout>"
	} else if options.AbsOutputDir != "" {
		return b.fs.Join(options.AbsOutputDir, jsName)
	} else {
		return b.fs.Join(b.fs.Dir(b.sources[entryPoint].AbsolutePath), jsName)
	}
}

func addTrailing(x []byte, c byte) []byte {
	if len(x) > 0 && x[len(x)-1] != c {
		x = append(x, c)
	}
	return x
}

const runtimeSourceIndex = 0
