package bundler

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/evanw/esbuild/pkg/ast"
	"github.com/evanw/esbuild/pkg/fs"
	"github.com/evanw/esbuild/pkg/lexer"
	"github.com/evanw/esbuild/pkg/logging"
	"github.com/evanw/esbuild/pkg/parser"
	"github.com/evanw/esbuild/pkg/printer"
	"github.com/evanw/esbuild/pkg/resolver"
	"sort"
	"strings"
	"sync"
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

type Bundle struct {
	fs          fs.FS
	sources     []logging.Source
	files       []file
	entryPoints []uint32
}

type parseResult struct {
	sourceIndex uint32
	ast         ast.AST
	ok          bool
}

func parseFile(
	log logging.Log, source logging.Source, importSource logging.Source, pathRange ast.Range,
	parseOptions parser.ParseOptions, bundleOptions BundleOptions, results chan parseResult,
) {
	path := source.AbsolutePath

	// Get the file extension
	extension := ""
	if lastDot := strings.LastIndexByte(path, '.'); lastDot >= 0 {
		extension = path[lastDot:]
	}

	switch bundleOptions.ExtensionToLoader[extension] {
	case LoaderJS:
		ast, ok := parser.Parse(log, source, parseOptions)
		results <- parseResult{source.Index, ast, ok}

	case LoaderJSX:
		parseOptions.JSX.Parse = true
		ast, ok := parser.Parse(log, source, parseOptions)
		results <- parseResult{source.Index, ast, ok}

	case LoaderJSON:
		expr, ok := parser.ParseJson(log, source)
		ast := parser.ModuleExportsAST(log, source, expr)
		results <- parseResult{source.Index, ast, ok}

	case LoaderText:
		expr := ast.Expr{ast.Loc{0}, &ast.EString{lexer.StringToUTF16(source.Contents)}}
		ast := parser.ModuleExportsAST(log, source, expr)
		results <- parseResult{source.Index, ast, true}

	case LoaderBase64:
		encoded := base64.StdEncoding.EncodeToString([]byte(source.Contents))
		expr := ast.Expr{ast.Loc{0}, &ast.EString{lexer.StringToUTF16(encoded)}}
		ast := parser.ModuleExportsAST(log, source, expr)
		results <- parseResult{source.Index, ast, true}

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

	maybeParseFile := func(path string, importSource logging.Source, pathRange ast.Range, isDisabled bool) (uint32, bool) {
		sourceIndex, ok := visited[path]
		if !ok {
			sourceIndex = uint32(len(sources))
			visited[path] = sourceIndex
			contents := ""

			// Disabled files are left empty
			if !isDisabled {
				contents, ok = res.Read(path)
				if !ok {
					log.AddRangeError(importSource, pathRange, fmt.Sprintf("Could not read from file: %s", path))
					return 0, false
				}
			}

			source := logging.Source{
				Index:        sourceIndex,
				AbsolutePath: path,
				PrettyPath:   res.PrettyPath(path),
				Contents:     contents,
			}
			sources = append(sources, source)
			files = append(files, file{})
			remaining++
			go parseFile(log, source, importSource, pathRange, parseOptions, bundleOptions, results)
		}
		return sourceIndex, true
	}

	entryPoints := []uint32{}
	for _, path := range entryPaths {
		if sourceIndex, ok := maybeParseFile(path, logging.Source{}, ast.Range{}, false /* isDisabled */); ok {
			entryPoints = append(entryPoints, sourceIndex)
		}
	}

	for remaining > 0 {
		result := <-results
		remaining--
		if result.ok {
			resolvedImports := make(map[string]uint32)
			for _, importPath := range result.ast.ImportPaths {
				source := sources[result.sourceIndex]
				sourcePath := source.AbsolutePath
				pathText := importPath.Path.Text
				pathRange := source.RangeOfString(importPath.Path.Loc)

				if path, status := res.Resolve(sourcePath, pathText); status != resolver.ResolveMissing {
					if sourceIndex, ok := maybeParseFile(path, source, pathRange, status == resolver.ResolveDisabled); ok {
						resolvedImports[pathText] = sourceIndex
					}
				} else {
					log.AddRangeError(source, pathRange, fmt.Sprintf("Could not resolve %q", pathText))
				}
			}
			files[result.sourceIndex] = file{result.ast, resolvedImports}
		}
	}

	return Bundle{fs, sources, files, entryPoints}
}

type Loader int

const (
	LoaderNone Loader = iota
	LoaderJS
	LoaderJSX
	LoaderJSON
	LoaderText
	LoaderBase64
)

func DefaultExtensionToLoaderMap() map[string]Loader {
	return map[string]Loader{
		".js":   LoaderJS,
		".jsx":  LoaderJSX,
		".json": LoaderJSON,
		".txt":  LoaderText,
	}
}

type BundleOptions struct {
	Bundle                bool
	AbsOutputFile         string
	AbsOutputDir          string
	RemoveWhitespace      bool
	MinifyIdentifiers     bool
	MangleSyntax          bool
	SourceMap             bool
	ModuleName            string
	ExtensionToLoader     map[string]Loader
	omitBootstrapForTests bool
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
	// The JavaScript AST printed as a string. This is always filled in when
	// compileFile() is called.
	js []byte

	// This is filled in by the printer as it generates the JavaScript. It
	// contains encoded source map offsets which will later be joined together
	// to form the complete source map. This is only filled in if the SourceMap
	// option is enabled.
	sourceMapChunk printer.SourceMapChunk

	// The source map contains the original source code, which is quoted in
	// parallel for speed. This is only filled in if the SourceMap option is
	// enabled.
	quotedSource string
}

func (b *Bundle) compileFile(
	options *BundleOptions, sourceIndex uint32, f file, sourceIndexToOutputIndex []uint32,
) compileResult {
	sourceMapContents := &b.sources[sourceIndex].Contents
	if !options.SourceMap {
		sourceMapContents = nil
	}
	tree := f.ast
	indent := 0
	if options.Bundle {
		indent = 2
	}

	// Remap source indices to make the output deterministic
	var remappedResolvedImports map[string]uint32
	if options.Bundle {
		remappedResolvedImports = make(map[string]uint32)
		for k, v := range f.resolvedImports {
			remappedResolvedImports[k] = sourceIndexToOutputIndex[v]
		}
	}

	js, chunk := printer.Print(tree, printer.Options{
		RemoveWhitespace:  options.RemoveWhitespace,
		SourceMapContents: sourceMapContents,
		Indent:            indent,
		ResolvedImports:   remappedResolvedImports,
	})
	result := compileResult{js: js}
	if options.SourceMap {
		result.quotedSource = printer.QuoteForJSON(b.sources[sourceIndex].Contents)
		result.sourceMapChunk = chunk
	}
	return result
}

func (b *Bundle) generateJavaScriptForEntryPoint(
	files []file, symbols *ast.SymbolMap, compileResults map[uint32]*compileResult, groups [][]uint32, options *BundleOptions,
	jsPrefix []byte, entryPoint uint32, jsName string, sourceIndexToOutputIndex []uint32, moduleInfos []moduleInfo,
) (result BundleResult, generatedOffsets map[uint32]lineColumnOffset) {
	// Join the JavaScript files together into a bundle
	prevOffset := 0
	js := []byte{}
	if options.ModuleName != "" {
		equals := " = "
		if options.RemoveWhitespace {
			equals = "="
		}
		js = append(js, ("let " + options.ModuleName + equals)...)
	}
	if options.omitBootstrapForTests {
		js = append(js, "bootstrap({"...)
	} else {
		js = append(js, '(')
		js = append(js, jsPrefix...)
		js = append(js, ")({"...)
	}

	// This is the line and column offset since the previous JavaScript string
	// or the start of the file if this is the first JavaScript string.
	generatedOffsets = make(map[uint32]lineColumnOffset)

	for i, group := range groups {
		rootSourceIndex := group[len(group)-1]
		tree := files[rootSourceIndex].ast

		// Append the prefix
		if i > 0 {
			js = append(js, ",\n"...)
		}
		if !options.RemoveWhitespace {
			js = append(js, "\n  "...)
		}
		js = append(js, fmt.Sprintf("%d(", sourceIndexToOutputIndex[rootSourceIndex])...)
		requireSymbol := symbols.Get(ast.FollowSymbols(symbols, tree.RequireRef))
		exportsSymbol := symbols.Get(ast.FollowSymbols(symbols, tree.ExportsRef))
		moduleSymbol := symbols.Get(ast.FollowSymbols(symbols, tree.ModuleRef))
		if requireSymbol.UseCountEstimate > 0 || exportsSymbol.UseCountEstimate > 0 || moduleSymbol.UseCountEstimate > 0 {
			js = append(js, requireSymbol.Name...)
			if exportsSymbol.UseCountEstimate > 0 || moduleSymbol.UseCountEstimate > 0 {
				if options.RemoveWhitespace {
					js = append(js, ',')
				} else {
					js = append(js, ", "...)
				}
				js = append(js, exportsSymbol.Name...)
				if moduleSymbol.UseCountEstimate > 0 {
					if options.RemoveWhitespace {
						js = append(js, ',')
					} else {
						js = append(js, ", "...)
					}
					js = append(js, moduleSymbol.Name...)
				}
			}
		}
		if options.RemoveWhitespace {
			js = append(js, "){"...)
		} else {
			js = append(js, ") {\n"...)
		}

		// Append the modules in this group
		for j, sourceIndex := range group {
			// Append the prefix
			if !options.RemoveWhitespace {
				if j > 0 {
					js = append(js, '\n')
				}
				js = append(js, fmt.Sprintf("    // %s\n", b.sources[sourceIndex].PrettyPath)...)
			}

			// If we're an internal non-root module in this group and our exports are
			// used, then we'll need to generate an exports object.
			//
			// This is done here at the last-minute instead of being baked into the
			// generated JavaScript because this lets us use the same generated
			// JavaScript for a root and a non-root module. That way we can generate
			// JavaScript for each module exactly once while still allowing a module
			// to be both a root and a non-root module for different entry points.
			//
			// For example, consider a bundle with two entry points "simplelib.js"
			// and "deluxelib.js", and "deluxelib.js" imports "simplelib.js". This
			// means "simplelib.js" is both a root module for its own entry point
			// and a non-root module for the entry point of "deluxelib.js".
			if sourceIndex != rootSourceIndex && moduleInfos[sourceIndex].isExportsUsed() {
				name := symbols.Get(ast.FollowSymbols(symbols, files[sourceIndex].ast.ExportsRef)).Name
				if options.RemoveWhitespace {
					js = append(js, fmt.Sprintf("var %s={};", name)...)
				} else {
					js = append(js, fmt.Sprintf("    var %s = {};\n", name)...)
				}
			}

			// Save the offset to the start of the stored JavaScript
			generatedOffsets[sourceIndex] = computeLineColumnOffset(js[prevOffset:])

			// Append the stored JavaScript
			js = append(js, compileResults[sourceIndex].js...)
			prevOffset = len(js)
		}

		// Append the suffix
		if !options.RemoveWhitespace {
			js = append(js, "  }"...)
		} else {
			js = append(js, '}')
		}
	}

	// Append the suffix
	if options.RemoveWhitespace {
		js = append(js, fmt.Sprintf("},%d);\n", sourceIndexToOutputIndex[entryPoint])...)
	} else {
		js = append(js, fmt.Sprintf("\n}, %d);\n", sourceIndexToOutputIndex[entryPoint])...)
	}

	result = BundleResult{
		JsAbsPath:  b.outputPathForEntryPoint(entryPoint, jsName, options),
		JsContents: js,
	}
	return
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
			buffer = append(buffer, comma...)
			buffer = append(buffer, printer.QuoteForJSON(b.sources[sourceIndex].PrettyPath)...)
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
	for _, group := range groups {
		for i, sourceIndex := range group {
			chunk := compileResults[sourceIndex].sourceMapChunk
			offset := generatedOffsets[sourceIndex]

			// Because each file for the bundle is converted to a source map once,
			// the source maps are shared between all entry points in the bundle.
			// The easiest way of getting this to work is to have all source maps
			// generate as if their source index is 0. We then adjust the source
			// index per entry point by modifying the first source mapping. This
			// is done by AppendSourceMapChunk() using the source index passed
			// here.
			startState := printer.SourceMapState{SourceIndex: i}

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
			prevEndState.SourceIndex = i
			prevColumnOffset = chunk.FinalGeneratedColumn

			// If this was all one line, include the column offset from the start
			if prevEndState.GeneratedLine == 0 {
				prevEndState.GeneratedColumn += startState.GeneratedColumn
				prevColumnOffset += startState.GeneratedColumn
			}
		}
	}
	buffer = append(buffer, '"')

	// Finish the source map
	item.SourceMapAbsPath = item.JsAbsPath + ".map"
	item.SourceMapContents = append(buffer, ",\n  \"names\": []\n}\n"...)
	item.JsContents = append(item.JsContents, ("//# sourceMappingURL=" + b.fs.Base(item.SourceMapAbsPath) + "\n")...)
}

func (b *Bundle) mergeAllSymbolsIntoOneMap(files []file) *ast.SymbolMap {
	// Make sure the symbol map can hold all symbols
	maxSourceIndex := 0
	for sourceIndex, _ := range files {
		if sourceIndex > maxSourceIndex {
			maxSourceIndex = sourceIndex
		}
	}
	symbols := ast.NewSymbolMap(maxSourceIndex)

	// Clone a copy of each file's symbols into the bundle-level symbol map. It's
	// cloned so we can modify it without affecting the original file AST, which
	// must be treated as read-only so it can be reused between compilations.
	for sourceIndex, f := range files {
		symbols.Outer[sourceIndex] = append([]ast.Symbol{}, f.ast.Symbols.Outer[sourceIndex]...)
		files[sourceIndex].ast.Symbols = symbols
	}

	return symbols
}

type indexAndPath struct {
	sourceIndex uint32
	path        string
}

// This type is just so we can use Go's native sort function
type indexAndPathArray []indexAndPath

func (a indexAndPathArray) Len() int               { return len(a) }
func (a indexAndPathArray) Swap(i int, j int)      { a[i], a[j] = a[j], a[i] }
func (a indexAndPathArray) Less(i int, j int) bool { return a[i].path < a[j].path }

// The source indices are non-deterministic because they are assigned in a
// random order during the parallel bundle traversal. However, we want our
// output to be deterministic. To accomplish this, the source indices are
// remapped to an arbitrary ordering (the index of the sorted absolute path).
func (b *Bundle) computeDeterministicRemapping() (sourceIndexToOutputIndex []uint32, outputIndexToSourceIndex []uint32) {
	sortedFilePaths := indexAndPathArray{}
	for _, source := range b.sources {
		sortedFilePaths = append(sortedFilePaths, indexAndPath{source.Index, source.AbsolutePath})
	}
	sort.Sort(sortedFilePaths)
	sourceIndexToOutputIndex = make([]uint32, len(sortedFilePaths))
	outputIndexToSourceIndex = make([]uint32, len(sortedFilePaths))
	for i, item := range sortedFilePaths {
		sourceIndexToOutputIndex[item.sourceIndex] = uint32(i)
		outputIndexToSourceIndex[i] = item.sourceIndex
	}
	return
}

type moduleInfo struct {
	isEntryPoint bool

	// This is true if either a) this module is the target of a require() or
	// import() or b) this module uses the "exports" or "module" variables.
	isCommonJs bool

	// This is a number indicating which "group" this module is in. A group
	// is a collection of modules that only import modules in that group via
	// ES6 imports. This is useful because a group can be compiled by joining
	// all files together in topological order without any dynamic import/
	// export code. All imports and exports within a group can be bound
	// statically by referencing the names directly. All modules with the
	// same label have the same group. Every CommonJS module is in its own
	// group.
	groupLabel uint32

	isTargetOfImportStar bool
	imports              []importData
	exports              map[string]ast.Ref
	exportStars          []exportStar
}

// Returns true if the "exports" variable is needed by something. If it's not
// needed, then we can omit it from the output entirely.
func (moduleInfo *moduleInfo) isExportsUsed() bool {
	return moduleInfo.isCommonJs || moduleInfo.isEntryPoint || moduleInfo.isTargetOfImportStar
}

type importData struct {
	alias             string // This is "*" for import stars
	aliasLoc          ast.Loc
	importSourceIndex uint32
	name              ast.LocRef
}

type exportStar struct {
	importSourceIndex uint32
	path              ast.Path
}

func includeDecls(decls []ast.Decl, symbols *ast.SymbolMap, exports map[string]ast.Ref) {
	var visitBinding func(ast.Binding)

	visitBinding = func(binding ast.Binding) {
		switch b := binding.Data.(type) {
		case *ast.BMissing:

		case *ast.BIdentifier:
			exports[symbols.Get(b.Ref).Name] = b.Ref

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

func (b *Bundle) extractImportsAndExports(
	files []file, symbols *ast.SymbolMap, sourceIndex uint32,
	moduleInfos []moduleInfo, namespaceImportMap map[ast.Ref]ast.ENamespaceImport,
) {
	file := &files[sourceIndex]
	meta := &moduleInfos[sourceIndex]

	// Track import items, which must be converted to property accesses
	indirectImportItems := make(map[ast.Ref]bool, len(file.ast.IndirectImportItems))
	for ref, _ := range file.ast.IndirectImportItems {
		indirectImportItems[ref] = true
	}

	// Reserve two statements, one for imports and one for exports. We reserve
	// these now for two reasons: a) we don't want to reallocate later and b)
	// we don't want to have to shift all statements over by one to prepend
	// an import or export statement.
	stmtStart := 2
	stmts := make([]ast.Stmt, stmtStart, len(file.ast.Stmts)+stmtStart)
	importDecls := []ast.Decl{}

	// Certain import and export statements need to generate require() calls
	addRequireCall := func(loc ast.Loc, ref ast.Ref, path ast.Path) {
		importDecls = append(importDecls, ast.Decl{
			ast.Binding{loc, &ast.BIdentifier{ref}},
			&ast.Expr{path.Loc, &ast.ERequire{Path: path, IsES6Import: true}},
		})
		symbols.IncrementUseCountEstimate(file.ast.RequireRef)
	}

	for _, stmt := range file.ast.Stmts {
		switch s := stmt.Data.(type) {
		case *ast.SImport:
			otherSourceIndex, ok := file.resolvedImports[s.Path.Text]
			if !ok {
				panic("Internal error")
			}
			isInSameGroup := moduleInfos[otherSourceIndex].groupLabel == meta.groupLabel
			namespaceLoc := stmt.Loc
			if s.StarLoc != nil {
				namespaceLoc = *s.StarLoc
			}

			if isInSameGroup {
				// Add imports so we can bind symbols later
				if s.DefaultName != nil {
					meta.imports = append(meta.imports, importData{"default", s.DefaultName.Loc, otherSourceIndex, *s.DefaultName})
				}
				if s.Items != nil {
					for _, item := range *s.Items {
						meta.imports = append(meta.imports, importData{item.Alias, item.AliasLoc, otherSourceIndex, item.Name})
					}
				}
				if s.StarLoc != nil {
					meta.imports = append(meta.imports, importData{"*", stmt.Loc, otherSourceIndex,
						ast.LocRef{namespaceLoc, s.NamespaceRef}})
				}
			} else {
				// Add a require() call for this import
				addRequireCall(namespaceLoc, s.NamespaceRef, s.Path)

				// Store the ref in "indirectImportItems" to make sure the printer prints
				// these imports as property accesses. Also store information in the
				// "namespaceImportMap" map in case this import is re-exported.
				if s.DefaultName != nil {
					indirectImportItems[s.DefaultName.Ref] = true
					namespaceImportMap[s.DefaultName.Ref] = ast.ENamespaceImport{
						NamespaceRef: s.NamespaceRef,
						ItemRef:      s.DefaultName.Ref,
						Alias:        "default",
					}
				}
				if s.Items != nil {
					for _, item := range *s.Items {
						indirectImportItems[item.Name.Ref] = true
						namespaceImportMap[item.Name.Ref] = ast.ENamespaceImport{
							NamespaceRef: s.NamespaceRef,
							ItemRef:      item.Name.Ref,
							Alias:        item.Alias,
						}
					}
				}
			}
			continue

		case *ast.SExportClause:
			// "export { item1, item2 }"
			for _, item := range s.Items {
				meta.exports[item.Alias] = item.Name.Ref
			}
			continue

		case *ast.SExportFrom:
			// "export { item1, item2 } from 'path'"
			for _, item := range s.Items {
				meta.exports[item.Alias] = item.Name.Ref
			}

			otherSourceIndex, ok := file.resolvedImports[s.Path.Text]
			if !ok {
				panic("Internal error")
			}
			isInSameGroup := moduleInfos[otherSourceIndex].groupLabel == meta.groupLabel

			if isInSameGroup {
				// Add imports so we can bind symbols later
				for _, item := range s.Items {
					// Re-exporting involves importing as one name and exporting as another name
					importName := symbols.Get(item.Name.Ref).Name
					meta.imports = append(meta.imports, importData{importName, item.Name.Loc, otherSourceIndex, item.Name})
				}
			} else {
				// Add a require() call for this import
				addRequireCall(stmt.Loc, s.NamespaceRef, s.Path)

				// Store the ref in "indirectImportItems" to make sure the printer prints
				// these imports as property accesses. Also store information in the
				// "namespaceImportMap" map since this import is re-exported.
				for _, item := range s.Items {
					indirectImportItems[item.Name.Ref] = true
					namespaceImportMap[item.Name.Ref] = ast.ENamespaceImport{
						NamespaceRef: s.NamespaceRef,
						ItemRef:      item.Name.Ref,
						Alias:        item.Alias,
					}
				}
			}
			continue

		case *ast.SExportDefault:
			// "export default value"
			meta.exports["default"] = s.DefaultName.Ref
			if s.Value.Expr != nil {
				stmt = ast.Stmt{stmt.Loc, &ast.SLocal{Kind: ast.LocalConst, Decls: []ast.Decl{
					ast.Decl{ast.Binding{s.DefaultName.Loc, &ast.BIdentifier{s.DefaultName.Ref}}, s.Value.Expr},
				}}}
			} else {
				switch s2 := s.Value.Stmt.Data.(type) {
				case *ast.SFunction:
					if s2.Fn.Name == nil {
						s2 = &ast.SFunction{s2.Fn, false}
						s2.Fn.Name = &s.DefaultName
					} else {
						ast.MergeSymbols(symbols, s.DefaultName.Ref, s2.Fn.Name.Ref)
					}
					stmt = ast.Stmt{s.Value.Stmt.Loc, s2}
				case *ast.SClass:
					if s2.Class.Name == nil {
						s2 = &ast.SClass{s2.Class, false}
						s2.Class.Name = &s.DefaultName
					} else {
						ast.MergeSymbols(symbols, s.DefaultName.Ref, s2.Class.Name.Ref)
					}
					stmt = ast.Stmt{s.Value.Stmt.Loc, s2}
				default:
					panic("Internal error")
				}
			}

		case *ast.SExportStar:
			otherSourceIndex, ok := file.resolvedImports[s.Path.Text]
			if !ok {
				panic("Internal error")
			}
			isInSameGroup := moduleInfos[otherSourceIndex].groupLabel == meta.groupLabel

			if s.Item == nil {
				// "export * from 'path'"
				meta.exportStars = append(meta.exportStars, exportStar{otherSourceIndex, s.Path})
			} else {
				// "export * as ns from 'path'"
				meta.exports[s.Item.Alias] = s.Item.Name.Ref

				if isInSameGroup {
					// Add imports so we can bind symbols later
					meta.imports = append(meta.imports, importData{"*", stmt.Loc, otherSourceIndex, s.Item.Name})
				} else {
					// Add a require() call for this import
					addRequireCall(s.Item.Name.Loc, s.Item.Name.Ref, s.Path)
				}
			}
			continue

		case *ast.SLocal:
			if s.IsExport {
				includeDecls(s.Decls, symbols, meta.exports)
				stmt = ast.Stmt{stmt.Loc, &ast.SLocal{Kind: s.Kind, Decls: s.Decls}}
			}

		case *ast.SFunction:
			if s.IsExport {
				ref := s.Fn.Name.Ref
				meta.exports[symbols.Get(ref).Name] = ref
				stmt = ast.Stmt{stmt.Loc, &ast.SFunction{s.Fn, false}}
			}

		case *ast.SClass:
			if s.IsExport {
				ref := s.Class.Name.Ref
				meta.exports[symbols.Get(ref).Name] = ref
				stmt = ast.Stmt{stmt.Loc, &ast.SClass{s.Class, false}}
			}
		}

		stmts = append(stmts, stmt)
	}

	// Prepend imports if there are any
	if len(importDecls) > 0 {
		stmtStart--
		stmts[stmtStart] = ast.Stmt{ast.Loc{}, &ast.SLocal{Kind: ast.LocalConst, Decls: importDecls}}
	}

	// Reserve a slot at the beginning for our exports, which will be used or
	// discarded by our caller
	stmtStart--

	// Update the file
	file.ast.Stmts = stmts[stmtStart:]
	file.ast.IndirectImportItems = indirectImportItems
}

func addExportStar(moduleInfos []moduleInfo, visited map[uint32]bool, sourceIndex uint32, otherSourceIndex uint32) {
	if visited[otherSourceIndex] {
		return
	}
	visited[otherSourceIndex] = true

	moduleInfo := &moduleInfos[sourceIndex]
	otherModuleInfo := &moduleInfos[otherSourceIndex]
	isInSameGroup := moduleInfo.groupLabel == otherModuleInfo.groupLabel

	// Make sure the other imported file is in the same group
	if isInSameGroup {
		exports := moduleInfo.exports
		for name, ref := range otherModuleInfo.exports {
			exports[name] = ref
		}

		for _, exportStar := range otherModuleInfo.exportStars {
			addExportStar(moduleInfos, visited, sourceIndex, exportStar.importSourceIndex)
		}
	}
}

func (b *Bundle) bindImportsAndExports(
	log logging.Log, files []file, symbols *ast.SymbolMap, group []uint32, moduleInfos []moduleInfo,
) {
	// Track any imports that may be re-exported
	namespaceImportMap := make(map[ast.Ref]ast.ENamespaceImport)

	// Initialize the export maps
	for _, sourceIndex := range group {
		moduleInfos[sourceIndex].exports = make(map[string]ast.Ref)
	}

	// Scan for information about imports and exports
	for _, sourceIndex := range group {
		b.extractImportsAndExports(files, symbols, sourceIndex, moduleInfos, namespaceImportMap)
	}

	// Process "export *" statements
	for _, sourceIndex := range group {
		for _, exportStar := range moduleInfos[sourceIndex].exportStars {
			visited := map[uint32]bool{sourceIndex: true}
			addExportStar(moduleInfos, visited, sourceIndex, exportStar.importSourceIndex)
		}
	}

	// Process imports and merge symbols across modules
	for _, sourceIndex := range group {
		for _, i := range moduleInfos[sourceIndex].imports {
			if i.alias == "*" {
				moduleInfos[i.importSourceIndex].isTargetOfImportStar = true
				ast.MergeSymbols(symbols, i.name.Ref, files[i.importSourceIndex].ast.ExportsRef)
			} else {
				if target, ok := moduleInfos[i.importSourceIndex].exports[i.alias]; ok {
					ast.MergeSymbols(symbols, i.name.Ref, target)
				} else {
					source := b.sources[sourceIndex]
					r := lexer.RangeOfIdentifier(source, i.aliasLoc)
					log.AddRangeError(source, r, fmt.Sprintf("No matching export for import %q", i.alias))
				}
			}
		}
	}

	// Generate exports for modules that need them. Exports must come first
	// before the contents of the module because exports are live bindings to the
	// symbols within the module.
	//
	// The first statement in every file is a dummy statement that was reserved
	// for us when we called "extractImportsAndExports". This is done to avoid
	// an extra allocation and O(n) copy to prepend a statement. We must either
	// use or discard this slot.
	for _, sourceIndex := range group {
		file := &files[sourceIndex]
		stmts := file.ast.Stmts

		// Check to see if we can skip generating exports for this module
		if !moduleInfos[sourceIndex].isExportsUsed() {
			stmts = stmts[1:] // Discard the export slot
			file.ast.Stmts = stmts
			continue
		}

		// Sort exports by name for determinism
		exports := moduleInfos[sourceIndex].exports
		aliases := make([]string, 0, len(exports))
		for alias, _ := range exports {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)

		// Build up a list of all exports for this module
		properties := []ast.Property{}
		for _, alias := range aliases {
			exportRef := exports[alias]
			var value ast.Expr
			if importData, ok := namespaceImportMap[exportRef]; ok {
				// If this export is a namespace import then we need to generate a ENamespaceImport
				value = ast.Expr{ast.Loc{}, &importData}
				symbols.IncrementUseCountEstimate(importData.NamespaceRef)
			} else {
				value = ast.Expr{ast.Loc{}, &ast.EIdentifier{exportRef}}
				symbols.IncrementUseCountEstimate(exportRef)
			}
			properties = append(properties, ast.Property{
				Key:   ast.Expr{ast.Loc{}, &ast.EString{lexer.StringToUTF16(alias)}},
				Value: &ast.Expr{ast.Loc{}, &ast.EArrow{Expr: &value}},
			})
		}

		// Skip generating exports if there are none
		if len(properties) == 0 {
			stmts = stmts[1:] // Discard the export slot
			file.ast.Stmts = stmts
			continue
		}

		// Use the export slot
		stmts[0] = ast.Stmt{ast.Loc{}, &ast.SExpr{ast.Expr{ast.Loc{}, &ast.ECall{
			ast.Expr{ast.Loc{}, &ast.EIdentifier{file.ast.RequireRef}},
			[]ast.Expr{
				ast.Expr{ast.Loc{}, &ast.EIdentifier{file.ast.ExportsRef}},
				ast.Expr{ast.Loc{}, &ast.EObject{properties}},
			},
			false,
		}}}}
		symbols.IncrementUseCountEstimate(file.ast.RequireRef)
		symbols.IncrementUseCountEstimate(file.ast.ExportsRef)
		file.ast.Stmts = stmts
	}
}

func markExportsAsUnboundInDecls(decls []ast.Decl, symbols *ast.SymbolMap) {
	var visitBinding func(ast.Binding)

	visitBinding = func(binding ast.Binding) {
		switch b := binding.Data.(type) {
		case *ast.BMissing:

		case *ast.BIdentifier:
			symbol := symbols.Get(b.Ref)
			symbol.Kind = ast.SymbolUnbound
			symbols.Set(b.Ref, symbol)

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

func (b *Bundle) markExportsAsUnbound(f file, symbols *ast.SymbolMap) {
	for _, stmt := range f.ast.Stmts {
		switch s := stmt.Data.(type) {
		case *ast.SLocal:
			if s.IsExport {
				markExportsAsUnboundInDecls(s.Decls, symbols)
			}

		case *ast.SFunction:
			if s.IsExport {
				ref := s.Fn.Name.Ref
				symbol := symbols.Get(ref)
				symbol.Kind = ast.SymbolUnbound
				symbols.Set(ref, symbol)
			}

		case *ast.SClass:
			if s.IsExport {
				ref := s.Class.Name.Ref
				symbol := symbols.Get(ref)
				symbol.Kind = ast.SymbolUnbound
				symbols.Set(ref, symbol)
			}
		}
	}
}

// Ensures all symbol names are valid non-colliding identifiers
func (b *Bundle) renameOrMinifyAllSymbols(files []file, symbols *ast.SymbolMap, group []uint32, options *BundleOptions) {
	moduleScopes := make([]*ast.Scope, len(group))
	for i, sourceIndex := range group {
		moduleScopes[i] = files[sourceIndex].ast.ModuleScope
	}

	// Merge all "require" symbols together. This is necessary for correctness
	// because all modules in the same group will be joined together in the same
	// scope later on in the compilation pipeline.
	//
	// We don't need to worry about the "exports" and "module" symbols since any
	// use of those would cause the module using them to be flagged as a CommonJS
	// module and put into its own group without any other modules. In fact, it'd
	// be bad if we merged "exports" symbols together because each one may be
	// used for "import * as ns from 'foo'" statements and different modules in
	// the same group need to be distinct from one another.
	for _, sourceIndex := range group[1:] {
		ast.MergeSymbols(symbols, files[sourceIndex].ast.RequireRef, files[group[0]].ast.RequireRef)
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

	if options.MinifyIdentifiers {
		minifyAllSymbols(moduleScopes, symbols)
	} else {
		renameAllSymbols(moduleScopes, symbols)
	}
}

// See the comment on "groupLabel" above for the definition of a group
func (b *Bundle) computeModuleGroups(
	files []file, sourceIndexToOutputIndex []uint32, outputIndexToSourceIndex []uint32,
) (infos []moduleInfo, groups [][]uint32) {
	infos = make([]moduleInfo, len(b.sources))

	// Mark all entry points. This is used to ensure that we always generate
	// exports for all entry points, even if no other module imports them.
	for _, sourceIndex := range b.entryPoints {
		infos[sourceIndex].isEntryPoint = true
	}

	// Set the initial CommonJS status for known root modules
	for sourceIndex, f := range files {
		// Every module starts off in its own group
		infos[sourceIndex].groupLabel = uint32(sourceIndex)

		// A module is CommonJS if it contains CommonJS exports
		if f.ast.HasCommonJsExports {
			infos[sourceIndex].isCommonJs = true
		}

		// A module is CommonJS if it's the target of a require() or import()
		for _, importPath := range f.ast.ImportPaths {
			if importPath.Kind != ast.ImportStmt {
				importSourceIndex := f.resolvedImports[importPath.Path.Text]
				infos[importSourceIndex].isCommonJs = true
			}
		}
	}

	// Propagate CommonJS status to all transitive dependencies
	var visit func(sourceIndex uint32)
	visit = func(sourceIndex uint32) {
		infos[sourceIndex].isCommonJs = true
		f := &files[sourceIndex]

		// All dependencies of this module should also be CommonJS modules
		for _, importPath := range f.ast.ImportPaths {
			importSourceIndex := f.resolvedImports[importPath.Path.Text]
			if !infos[importSourceIndex].isCommonJs {
				visit(importSourceIndex)
			}
		}
	}
	for sourceIndex, info := range infos {
		if info.isCommonJs {
			visit(uint32(sourceIndex))
		}
	}

	// The remaining nodes are ES6 modules. Find the connected components in this
	// graph. This information will be used later to minify all modules belonging
	// to the same group together so that their symbol names are consistent. This
	// uses the union-find algorithm.
	var find func(uint32) uint32
	find = func(sourceIndex uint32) uint32 {
		if infos[sourceIndex].groupLabel != sourceIndex {
			infos[sourceIndex].groupLabel = find(infos[sourceIndex].groupLabel)
		}
		return infos[sourceIndex].groupLabel
	}
	union := func(a uint32, b uint32) {
		a = find(a)
		b = find(b)
		infos[a].groupLabel = b
	}
	for sourceIndex, f := range files {
		if !infos[sourceIndex].isCommonJs {
			for _, importPath := range f.ast.ImportPaths {
				if importPath.Kind == ast.ImportStmt {
					importSourceIndex := f.resolvedImports[importPath.Path.Text]
					if !infos[importSourceIndex].isCommonJs {
						union(uint32(sourceIndex), importSourceIndex)
					}
				}
			}
		}
	}

	// All modules with the same label are in the same group. Create an array of
	// groups, where each group is an array of the source indices for all modules
	// in that group. To ensure the determinism of the subsequent renaming step,
	// each group is sorted in ascending output index order (an arbitrary order
	// that is stable across different builds).
	groupMap := make(map[uint32][]int)
	for sourceIndex, _ := range files {
		outputIndices := groupMap[find(uint32(sourceIndex))]
		outputIndices = append(outputIndices, int(sourceIndexToOutputIndex[sourceIndex]))
		groupMap[find(uint32(sourceIndex))] = outputIndices
	}
	groups = make([][]uint32, 0, len(groupMap))
	for _, outputIndices := range groupMap {
		sort.Ints(outputIndices)
		group := make([]uint32, 0, len(outputIndices))
		for _, outputIndex := range outputIndices {
			group = append(group, outputIndexToSourceIndex[outputIndex])
		}
		groups = append(groups, group)
	}
	return
}

func (b *Bundle) Compile(log logging.Log, options BundleOptions) []BundleResult {
	if options.Bundle {
		return b.compileBundle(log, options)
	} else {
		return b.compileIndependent(log, options)
	}
}

func (b *Bundle) checkOverwrite(log logging.Log, sourceIndex uint32, path string) {
	if path == b.sources[sourceIndex].AbsolutePath {
		log.AddError(logging.Source{}, ast.Loc{},
			fmt.Sprintf("Refusing to overwrite input file %q (use --outfile or --outdir to configure the output)",
				b.sources[sourceIndex].PrettyPath))
	}
}

func (b *Bundle) compileIndependent(log logging.Log, options BundleOptions) []BundleResult {
	// When spawning a new goroutine, make sure to manually forward all variables
	// that are different for every iteration of the loop. Otherwise each
	// goroutine will share the same copy of the closed-over variables and cause
	// correctness issues.

	// Spawn parallel jobs to print the AST of each file in the bundle
	results := make([]BundleResult, len(b.sources))
	waitGroup := sync.WaitGroup{}
	files := []file(b.files)
	for sourceIndex, _ := range files {
		waitGroup.Add(1)
		go func(sourceIndex uint32) {
			group := []uint32{sourceIndex}

			// Make sure we don't rename exports
			symbols := b.mergeAllSymbolsIntoOneMap(files)
			b.markExportsAsUnbound(files[sourceIndex], symbols)

			// Rename symbols
			b.renameOrMinifyAllSymbols(files, symbols, group, &options)
			files[sourceIndex].ast.Symbols = symbols

			// Print the JavaScript code
			result := b.compileFile(&options, sourceIndex, files[sourceIndex], []uint32{})

			// Make a filename for the resulting JavaScript file
			jsName := b.outputFileForEntryPoint(sourceIndex, &options)

			// Generate the resulting JavaScript file
			item := &results[sourceIndex]
			item.JsAbsPath = b.outputPathForEntryPoint(sourceIndex, jsName, &options)
			item.JsContents = result.js

			// Optionally also generate a source map
			if options.SourceMap {
				compileResults := map[uint32]*compileResult{sourceIndex: &result}
				generatedOffsets := map[uint32]lineColumnOffset{sourceIndex: lineColumnOffset{}}
				groups := [][]uint32{group}
				b.generateSourceMapForEntryPoint(compileResults, generatedOffsets, groups, &options, item)
			}

			// Refuse to overwrite the input file
			b.checkOverwrite(log, sourceIndex, item.JsAbsPath)

			waitGroup.Done()
		}(uint32(sourceIndex))
	}

	// Wait for all jobs to finish
	waitGroup.Wait()

	return results
}

func (b *Bundle) compileBundle(log logging.Log, options BundleOptions) []BundleResult {
	// Make a shallow copy of all files in the bundle so we don't mutate the bundle
	files := append([]file{}, b.files...)

	symbols := b.mergeAllSymbolsIntoOneMap(files)
	sourceIndexToOutputIndex, outputIndexToSourceIndex := b.computeDeterministicRemapping()
	moduleInfos, moduleGroups := b.computeModuleGroups(
		files, sourceIndexToOutputIndex, outputIndexToSourceIndex)

	// When spawning a new goroutine, make sure to manually forward all variables
	// that are different for every iteration of the loop. Otherwise each
	// goroutine will share the same copy of the closed-over variables and cause
	// correctness issues.

	// Spawn parallel jobs to handle imports and exports for each group
	importExportGroup := sync.WaitGroup{}
	for _, group := range moduleGroups {
		importExportGroup.Add(1)
		go func(group []uint32) {
			// It's important to wait to rename symbols until after imports and
			// exports have been handled. Exports need to use the original un-renamed
			// names of the symbols.
			b.bindImportsAndExports(log, files, symbols, group, moduleInfos)
			b.renameOrMinifyAllSymbols(files, symbols, group, &options)
			importExportGroup.Done()
		}(group)
	}

	// Wait for all import/export jobs to finish
	importExportGroup.Wait()

	// Make sure calls to "ast.FollowSymbols()" below won't hit concurrent map
	// mutation hazards
	ast.FollowAllSymbols(symbols)

	// Spawn parallel jobs to print the AST of each file in the bundle
	compileResults := make(map[uint32]*compileResult, len(b.sources))
	compileGroup := sync.WaitGroup{}
	for sourceIndex, _ := range files {
		// Allocate all results on the same goroutine to avoid concurrent map hazards
		result := &compileResult{}
		compileResults[uint32(sourceIndex)] = result
		compileGroup.Add(1)
		go func(sourceIndex uint32, result *compileResult) {
			file := files[sourceIndex]
			*result = b.compileFile(&options, sourceIndex, file, sourceIndexToOutputIndex)
			compileGroup.Done()
		}(uint32(sourceIndex), result)
	}

	// All bundles use the same bootstrap prefix
	jsPrefix := generateBootstrapPrefix(&options)

	// Wait for all compile jobs to finish
	compileGroup.Wait()

	// Spawn parallel jobs to create files for each entry point
	results := make([]BundleResult, len(b.entryPoints))
	linkGroup := sync.WaitGroup{}
	for i, entryPoint := range b.entryPoints {
		linkGroup.Add(1)
		go func(i int, entryPoint uint32) {
			// Find all sources reachable from this entry point
			groups := b.deterministicDependenciesOfEntryPoint(files, entryPoint, moduleInfos)

			// Make a filename for the resulting JavaScript file
			jsName := b.outputFileForEntryPoint(entryPoint, &options)

			// Generate the resulting JavaScript file
			item, generatedOffsets := b.generateJavaScriptForEntryPoint(
				files, symbols, compileResults, groups, &options, jsPrefix,
				entryPoint, jsName, sourceIndexToOutputIndex, moduleInfos)

			// Optionally also generate a source map
			if options.SourceMap {
				b.generateSourceMapForEntryPoint(compileResults, generatedOffsets, groups, &options, &item)
			}

			// Refuse to overwrite the input file
			b.checkOverwrite(log, entryPoint, item.JsAbsPath)

			// Write the files to the output directory
			results[i] = item
			linkGroup.Done()
		}(i, entryPoint)
	}

	// Wait for all linking jobs to finish
	linkGroup.Wait()

	return results
}

func computeLineColumnOffset(bytes []byte) lineColumnOffset {
	offset := lineColumnOffset{}
	for _, c := range bytes {
		if c == '\n' {
			offset.lines++
			offset.columns = 0
		} else {
			offset.columns++
		}
	}
	return offset
}

// This returns the entry point and all modules it transitively depends on.
// These modules are categorized into their labeled groups in preparation for
// printing. Each group corresponds to a closure in the printed output. Each
// group is ordered such that dependencies come before dependents (so the
// root of the group will be last).
func (b *Bundle) deterministicDependenciesOfEntryPoint(
	files []file, entryPoint uint32, moduleInfos []moduleInfo,
) [][]uint32 {
	visited := make(map[uint32]bool)
	order := []uint32{}

	var visit func(uint32)
	visit = func(sourceIndex uint32) {
		// Only visit each module once
		if visited[sourceIndex] {
			return
		}
		visited[sourceIndex] = true

		// Include all dependencies. It's critical for determinism that this
		// iteration is deterministic, so we cannot iterate over a map here.
		f := &files[sourceIndex]
		for _, importPath := range f.ast.ImportPaths {
			visit(f.resolvedImports[importPath.Path.Text])
		}

		// Include this file after all dependencies
		order = append(order, sourceIndex)
	}
	visit(entryPoint)

	// Categorize into groups by label
	groupMap := make(map[uint32][]uint32)
	roots := []uint32{}
	for _, sourceIndex := range order {
		groupLabel := moduleInfos[sourceIndex].groupLabel
		group := groupMap[groupLabel]
		if len(group) == 0 {
			roots = append(roots, groupLabel)
		}
		group = append(group, sourceIndex)
		groupMap[groupLabel] = group
	}
	groups := make([][]uint32, 0, len(groupMap))
	for _, groupLabel := range roots {
		groups = append(groups, groupMap[groupLabel])
	}
	return groups
}

func (b *Bundle) outputFileForEntryPoint(entryPoint uint32, options *BundleOptions) string {
	if options.AbsOutputFile != "" {
		return b.fs.Base(options.AbsOutputFile)
	}
	name := b.fs.Base(b.sources[entryPoint].AbsolutePath)

	// Strip known file extensions
	for _, ext := range []string{".min.js", ".js", ".jsx", ".mjs"} {
		if strings.HasSuffix(name, ext) {
			name = name[:len(name)-len(ext)]
			break
		}
	}

	// Add the appropriate file extension
	if options.RemoveWhitespace {
		name += ".min"
	}
	name += ".js"
	return name
}

func (b *Bundle) outputPathForEntryPoint(entryPoint uint32, jsName string, options *BundleOptions) string {
	if options.AbsOutputDir != "" {
		return b.fs.Join(options.AbsOutputDir, jsName)
	} else {
		return b.fs.Join(b.fs.Dir(b.sources[entryPoint].AbsolutePath), jsName)
	}
}

func generateBootstrapPrefix(options *BundleOptions) []byte {
	// The require() function serves a few independent purposes:
	//
	//   // Import an exports object from another module. If "isES6Import" is
	//   // truthy and the module referenced by "sourceIndex" is a CommonJS
	//   // module, a conversion is done to correct for "default" exports.
	//   require(sourceIndex: number, isES6Import?: boolean): ExportsObject;
	//
	//   // Add properties to an exports object. These are added as ES6 getters
	//   // so the bindings are live and can't be overwritten.
	//   require(exports: ExportsObject, getters: {[name: string]: () => any}): void;
	//
	// It's overloaded like this to make the code slightly smaller as well as to
	// prevent the module from being able to mess with the export mechanism
	// (since the "require" symbol is special-cased by the parser).
	bootstrap := `
		((modules, entryPoint) => {
			let global = function() { return this }()
			let cache = {}

			let require = (target, arg) => {
				// If the first argument is a number, this is an import
				if (typeof target === 'number') {
					let module = cache[target], exports

					// Evaluate the module if needed
					if (!module) {
						module = cache[target] = {exports: {}}
						modules[target].call(global, require, module.exports, module)
					}

					// Return the exports object off the module in case it was overwritten
					exports = module.exports

					// Convert CommonJS exports to ES6 exports
					if (arg && (!exports || !exports.__esModule)) {
						if (!exports || typeof exports !== 'object') {
							exports = {}
						}
						if (!('default' in exports)) {
							Object.defineProperty(exports, 'default', {
								get: () => module.exports,
								enumerable: true,
							})
						}
					}

					return exports
				}

				// Mark this module as an ES6 module using a non-enumerable property
				Object.defineProperty(target, '__esModule', {
					value: true,
				})

				for (let name in arg) {
					Object.defineProperty(target, name, {
						get: arg[name],
						enumerable: true,
					})
				}
			}

			return require(entryPoint)
		})
	`

	// Parse the bootstrap code
	log := logging.Log{}
	source := logging.Source{
		Index:        0,
		AbsolutePath: "",
		PrettyPath:   "",
		Contents:     bootstrap,
	}
	result, ok := parser.Parse(log, source, parser.ParseOptions{
		MangleSyntax:         options.MangleSyntax,
		KeepSingleExpression: true,
	})
	if !ok {
		panic("Internal error")
	}

	// Optionally minify the symbol names
	if options.MinifyIdentifiers {
		minifyAllSymbols([]*ast.Scope{result.ModuleScope}, result.Symbols)
	}

	// Print the bootstrap code
	stmt, ok := result.Stmts[0].Data.(*ast.SExpr)
	if !ok {
		panic("Internal error")
	}
	prefix, _ := printer.PrintExpr(stmt.Value, result.Symbols, result.RequireRef, printer.Options{
		RemoveWhitespace:  options.RemoveWhitespace,
		SourceMapContents: nil,
		Indent:            0,
	})
	return prefix
}
