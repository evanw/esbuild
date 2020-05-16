package bundler

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/lexer"
	"github.com/evanw/esbuild/internal/logging"
	"github.com/evanw/esbuild/internal/printer"
)

type bitSet struct {
	entries []byte
}

func newBitSet(bitCount uint) bitSet {
	return bitSet{make([]byte, (bitCount+7)/8)}
}

func (bs bitSet) hasBit(bit uint) bool {
	return (bs.entries[bit/8] & (1 << (bit & 7))) != 0
}

func (bs bitSet) setBit(bit uint) {
	bs.entries[bit/8] |= 1 << (bit & 7)
}

func (bs bitSet) equals(other bitSet) bool {
	return bytes.Equal(bs.entries, other.entries)
}

type linkerContext struct {
	options     *BundleOptions
	log         logging.Log
	fs          fs.FS
	symbols     ast.SymbolMap
	entryPoints []uint32
	sources     []logging.Source
	files       []file
	fileMeta    []fileMeta
}

type entryPointStatus uint8

const (
	entryPointNone entryPointStatus = iota
	entryPointUserSpecified
	entryPointDynamicImport
)

type fileMeta struct {
	partMeta         []partMeta
	entryPointStatus entryPointStatus

	// If true, the module must be bundled CommonJS-style like this:
	//
	//   // foo.ts
	//   let require_foo = __commonJS((exports, module) => {
	//     ...
	//   });
	//
	//   // bar.ts
	//   let foo = flag ? require_foo() : null;
	//
	isCommonJS bool

	// The minimum number of links in the module graph to get from an entry point
	// to this file
	distanceFromEntryPoint uint32

	// This holds all entry points that can reach this file. It will be used to
	// assign the parts in this file to a chunk.
	entryBits bitSet
}

type partMeta struct {
	// This holds all entry points that can reach this part. It will be used to
	// assign this part to a chunk.
	entryBits bitSet

	// These are dependencies that come from other files via import statements.
	nonLocalDependencies []partRef
}

type partRef struct {
	sourceIndex uint32
	partIndex   uint32
}

type chunkMeta struct {
	name         string
	filesInChunk map[uint32]bool
	entryBits    bitSet
}

func newLinkerContext(options *BundleOptions, log logging.Log, fs fs.FS, sources []logging.Source, files []file, entryPoints []uint32) linkerContext {
	// Clone information about symbols and files so we don't mutate the input data
	c := linkerContext{
		options:     options,
		log:         log,
		fs:          fs,
		sources:     sources,
		entryPoints: append([]uint32{}, entryPoints...),
		files:       make([]file, len(files)),
		fileMeta:    make([]fileMeta, len(files)),
		symbols:     ast.NewSymbolMap(len(files)),
	}

	// Construct the file metadata arrays we will use later
	for sourceIndex, file := range files {
		c.symbols.Outer[sourceIndex] = append([]ast.Symbol{}, file.ast.Symbols.Outer[sourceIndex]...)
		file.ast.Symbols = c.symbols
		c.files[sourceIndex] = file
		c.fileMeta[sourceIndex] = fileMeta{
			distanceFromEntryPoint: ^uint32(0),
			isCommonJS:             file.ast.UsesCommonJSFeatures,
			partMeta:               make([]partMeta, len(file.ast.Parts)),
		}
	}

	// Mark all entry points so we don't add them again for import() expressions
	for _, sourceIndex := range entryPoints {
		c.fileMeta[sourceIndex].entryPointStatus = entryPointUserSpecified
	}

	return c
}

func (c *linkerContext) link() []BundleResult {
	c.scanImportsAndExports()
	c.markPartsReachableFromEntryPoints()
	c.renameOrMinifyAllSymbols()

	chunks := c.computeChunks()
	results := []BundleResult{}

	for _, chunk := range chunks {
		results = append(results, c.generateChunk(chunk))
	}

	return results
}

func (c *linkerContext) scanImportsAndExports() {
	for sourceIndex, file := range c.files {
		for _, part := range file.ast.Parts {
			// Handle require() and import()
			for _, importPath := range part.ImportPaths {
				switch importPath.Kind {
				case ast.ImportRequire:
					// Files that are imported with require() must be CommonJS modules
					if otherSourceIndex, ok := file.resolveImport(importPath.Path); ok {
						c.fileMeta[otherSourceIndex].isCommonJS = true
					}

				case ast.ImportDynamic:
					// Files that are imported with import() must be entry points
					if otherSourceIndex, ok := file.resolveImport(importPath.Path); ok {
						if c.fileMeta[otherSourceIndex].entryPointStatus == entryPointNone {
							c.entryPoints = append(c.entryPoints, otherSourceIndex)
							c.fileMeta[otherSourceIndex].entryPointStatus = entryPointDynamicImport
						}
					}
				}
			}
		}

		if len(file.ast.NamedImports) > 0 {
			// Sort imports for determinism. Otherwise our unit tests will randomly
			// fail sometimes when error messages are reordered.
			sortedImportRefs := make([]int, 0, len(file.ast.NamedImports))
			for ref, _ := range file.ast.NamedImports {
				sortedImportRefs = append(sortedImportRefs, int(ref.InnerIndex))
			}
			sort.Ints(sortedImportRefs)

			// Bind imports with their matching exports
			for _, innerIndex := range sortedImportRefs {
				importRef := ast.Ref{OuterIndex: uint32(sourceIndex), InnerIndex: uint32(innerIndex)}
				tracker := importTracker{uint32(sourceIndex), importRef}
				cycleDetector := tracker
				checkCycle := false
				for {
					// Make sure we avoid infinite loops trying to resolve cycles:
					//
					//   // foo.js
					//   export {a as b} from './foo.js'
					//   export {b as c} from './foo.js'
					//   export {c as a} from './foo.js'
					//
					if !checkCycle {
						checkCycle = true
					} else {
						checkCycle = false
						if cycleDetector == tracker {
							source := c.sources[sourceIndex]
							namedImport := c.files[sourceIndex].ast.NamedImports[importRef]
							r := lexer.RangeOfIdentifier(source, namedImport.AliasLoc)
							c.log.AddRangeError(source, r, fmt.Sprintf("Detected cycle while resolving import %q", namedImport.Alias))
							break
						}
						cycleDetector, _, _ = c.advanceImportTracker(cycleDetector)
					}

					// Resolve the import by one step
					nextTracker, localParts, status := c.advanceImportTracker(tracker)
					if status == importCommonJS {
						// If it's a CommonJS file, rewrite the import to a property access
						namedImport := c.files[tracker.sourceIndex].ast.NamedImports[tracker.importRef]
						c.symbols.SetNamespaceAlias(importRef, ast.NamespaceAlias{
							NamespaceRef: namedImport.NamespaceRef,
							Alias:        namedImport.Alias,
						})
						break
					} else if status == importMissing {
						// Report mismatched imports and exports
						source := c.sources[tracker.sourceIndex]
						namedImport := c.files[tracker.sourceIndex].ast.NamedImports[tracker.importRef]
						r := lexer.RangeOfIdentifier(source, namedImport.AliasLoc)
						c.log.AddRangeError(source, r, fmt.Sprintf("No matching export for import %q", namedImport.Alias))
						break
					} else if _, ok := c.files[nextTracker.sourceIndex].ast.NamedImports[nextTracker.importRef]; !ok {
						// If this is not a re-export of another import, add this import as
						// a dependency to all parts in this file that use this import
						for _, partIndex := range c.files[sourceIndex].ast.NamedImports[importRef].PartsWithUses {
							partMeta := &c.fileMeta[sourceIndex].partMeta[partIndex]
							for _, nonLocalPartIndex := range localParts {
								partMeta.nonLocalDependencies = append(partMeta.nonLocalDependencies, partRef{
									sourceIndex: nextTracker.sourceIndex,
									partIndex:   nonLocalPartIndex,
								})
							}
						}

						// Merge these symbols so they will share the same name
						ast.MergeSymbols(c.symbols, importRef, nextTracker.importRef)
						break
					} else {
						tracker = nextTracker
					}
				}
			}
		}
	}
}

type importTracker struct {
	sourceIndex uint32
	importRef   ast.Ref
}

type importStatus uint8

const (
	importMissing importStatus = iota
	importFound
	importCommonJS
)

func (c *linkerContext) advanceImportTracker(tracker importTracker) (importTracker, []uint32, importStatus) {
	file := &c.files[tracker.sourceIndex]
	namedImport := file.ast.NamedImports[tracker.importRef]

	// Use a CommonJS import if this is either a bundled CommonJS file or an
	// external file (for example, built-in node modules are marked external)
	otherSourceIndex, ok := file.resolveImport(namedImport.ImportPath)
	if !ok || c.fileMeta[otherSourceIndex].isCommonJS {
		return importTracker{}, nil, importCommonJS
	}

	// Match this import up with an export from the imported file
	otherFile := &c.files[otherSourceIndex]
	matchingExport, ok := otherFile.ast.NamedExports[namedImport.Alias]
	if !ok {
		return importTracker{}, nil, importMissing
	}

	// Check to see if this is a re-export of another import
	return importTracker{otherSourceIndex, matchingExport.Ref}, matchingExport.LocalParts, importFound
}

func (c *linkerContext) markPartsReachableFromEntryPoints() {
	// Allocate bit sets
	bitCount := uint(len(c.entryPoints))
	for sourceIndex, _ := range c.fileMeta {
		fileMeta := &c.fileMeta[sourceIndex]
		fileMeta.entryBits = newBitSet(bitCount)
		for partIndex, _ := range fileMeta.partMeta {
			fileMeta.partMeta[partIndex].entryBits = newBitSet(bitCount)
		}
	}

	// Each entry point marks all files reachable from itself
	for _, entryPoint := range c.entryPoints {
		c.includeFile(entryPoint, uint(entryPoint), 0)
	}
}

func (c *linkerContext) includeFile(sourceIndex uint32, entryPoint uint, distanceFromEntryPoint uint32) {
	fileMeta := &c.fileMeta[sourceIndex]

	// Track the minimum distance to an entry point
	if distanceFromEntryPoint < fileMeta.distanceFromEntryPoint {
		fileMeta.distanceFromEntryPoint = distanceFromEntryPoint
	}
	distanceFromEntryPoint++

	// Don't mark this file more than once
	if fileMeta.entryBits.hasBit(entryPoint) {
		return
	}
	fileMeta.entryBits.setBit(entryPoint)

	file := &c.files[sourceIndex]
	needsToModule := false

	for partIndex, part := range file.ast.Parts {
		// Include all parts in this file with side effects, or just include
		// everything if tree-shaking is disabled. Note that we still want to
		// perform tree-shaking on the runtime even if tree-shaking is disabled.
		if !part.CanBeRemovedIfUnused || (!c.options.TreeShaking && sourceIndex != runtimeSourceIndex) {
			c.includePart(sourceIndex, uint32(partIndex), entryPoint, distanceFromEntryPoint)
		}

		// Also include any statement-level imports
		for _, importPath := range part.ImportPaths {
			switch importPath.Kind {
			case ast.ImportStmt, ast.ImportDynamic:
				if otherSourceIndex, ok := file.resolveImport(importPath.Path); ok {
					if importPath.Kind == ast.ImportStmt {
						c.includeFile(otherSourceIndex, entryPoint, distanceFromEntryPoint)
					}
					if c.fileMeta[otherSourceIndex].isCommonJS {
						// This is an ES6 import of a module that's potentially CommonJS
						needsToModule = true
					}
				} else {
					// This is an ES6 import of an external module that may be CommonJS
					needsToModule = true
				}
			}
		}
	}

	// If this is a CommonJS file, we're going to need the "__commonJS" symbol
	// from the runtime
	if fileMeta.isCommonJS {
		for _, partIndex := range c.files[runtimeSourceIndex].ast.NamedExports["__commonJS"].LocalParts {
			c.includePart(runtimeSourceIndex, partIndex, entryPoint, distanceFromEntryPoint)
		}
	}

	// If there's an ES6 import of a non-ES6 module, then we're going to need the
	// "__toModule" symbol from the runtime
	if needsToModule {
		for _, partIndex := range c.files[runtimeSourceIndex].ast.NamedExports["__toModule"].LocalParts {
			c.includePart(runtimeSourceIndex, partIndex, entryPoint, distanceFromEntryPoint)
		}
	}
}

func (c *linkerContext) includePart(sourceIndex uint32, partIndex uint32, entryPoint uint, distanceFromEntryPoint uint32) {
	partMeta := &c.fileMeta[sourceIndex].partMeta[partIndex]

	// Don't mark this part more than once
	if partMeta.entryBits.hasBit(entryPoint) {
		return
	}
	partMeta.entryBits.setBit(entryPoint)

	// Also include any require() imports
	file := &c.files[sourceIndex]
	part := &file.ast.Parts[partIndex]
	for _, importPath := range part.ImportPaths {
		if importPath.Kind == ast.ImportRequire {
			if otherSourceIndex, ok := file.resolveImport(importPath.Path); ok {
				c.includeFile(otherSourceIndex, entryPoint, distanceFromEntryPoint)
			}
		}
	}

	// Also include any local dependencies
	for otherPartIndex, _ := range part.LocalDependencies {
		c.includePart(sourceIndex, otherPartIndex, entryPoint, distanceFromEntryPoint)
	}

	// Also include any non-local dependencies
	for _, nonLocalDependency := range partMeta.nonLocalDependencies {
		c.includePart(nonLocalDependency.sourceIndex, nonLocalDependency.partIndex, entryPoint, distanceFromEntryPoint)
	}
}

func (c *linkerContext) computeChunks() map[string]chunkMeta {
	chunks := make(map[string]chunkMeta)
	neverReachedKey := string(newBitSet(uint(len(c.entryPoints))).entries)

	// Compute entry point names
	entryPointNames := make([]string, len(c.entryPoints))
	for i, entryPoint := range c.entryPoints {
		if c.options.AbsOutputFile != "" && c.fileMeta[entryPoint].entryPointStatus == entryPointUserSpecified {
			entryPointNames[i] = c.fs.Base(c.options.AbsOutputFile)
		} else {
			name := c.fs.Base(c.sources[entryPoint].AbsolutePath)
			entryPointNames[i] = c.stripKnownFileExtension(name) + ".js"
		}
	}

	// Figure out which files are in which chunk
	for sourceIndex, fileMeta := range c.fileMeta {
		for _, partMeta := range fileMeta.partMeta {
			key := string(partMeta.entryBits.entries)
			if key == neverReachedKey {
				// Ignore this part if it was never reached
				continue
			}
			chunk := chunks[key]
			if chunk.filesInChunk == nil {
				// Give the chunk a name
				for i, entryPoint := range c.entryPoints {
					if partMeta.entryBits.hasBit(uint(entryPoint)) {
						if chunk.name != "" {
							chunk.name = c.stripKnownFileExtension(chunk.name) + "_"
						}
						chunk.name += entryPointNames[i]
					}
				}
				chunk.entryBits = partMeta.entryBits
				chunk.filesInChunk = make(map[uint32]bool)
				chunks[key] = chunk
			}
			chunk.filesInChunk[uint32(sourceIndex)] = true
		}
	}

	return chunks
}

func (c *linkerContext) stripKnownFileExtension(name string) string {
	for ext, _ := range c.options.ExtensionToLoader {
		if strings.HasSuffix(name, ext) {
			return name[:len(name)-len(ext)]
		}
	}
	return name
}

type chunkOrder struct {
	sourceIndex uint32
	distance    uint32
	path        string
}

// This type is just so we can use Go's native sort function
type chunkOrderArray []chunkOrder

func (a chunkOrderArray) Len() int          { return len(a) }
func (a chunkOrderArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a chunkOrderArray) Less(i int, j int) bool {
	return a[i].distance < a[j].distance || (a[i].distance == a[j].distance && a[i].path < a[j].path)
}

func (c *linkerContext) chunkFileOrder(chunk chunkMeta) []uint32 {
	sorted := make(chunkOrderArray, 0, len(chunk.filesInChunk))

	// Attach information to the files for use with sorting
	for sourceIndex, _ := range chunk.filesInChunk {
		sorted = append(sorted, chunkOrder{
			sourceIndex: sourceIndex,
			distance:    c.fileMeta[sourceIndex].distanceFromEntryPoint,
			path:        c.sources[sourceIndex].AbsolutePath,
		})
	}

	// Sort so files closest to an entry point come first. If two files are
	// equidistant to an entry point, then break the tie by sorting on the
	// absolute path.
	sort.Sort(sorted)

	visited := make(map[uint32]bool)
	order := []uint32{}

	// Traverse the graph using this stable order and linearize the files with
	// dependencies before dependents
	var visit func(uint32)
	visit = func(sourceIndex uint32) {
		if visited[sourceIndex] {
			return
		}
		visited[sourceIndex] = true
		file := &c.files[sourceIndex]
		fileMeta := &c.fileMeta[sourceIndex]
		for partIndex, part := range file.ast.Parts {
			for _, importPath := range part.ImportPaths {
				if importPath.Kind == ast.ImportStmt || (importPath.Kind == ast.ImportRequire &&
					chunk.entryBits.equals(fileMeta.partMeta[partIndex].entryBits)) {
					if otherSourceIndex, ok := file.resolveImport(importPath.Path); ok {
						visit(otherSourceIndex)
					}
				}
			}
		}
		order = append(order, sourceIndex)
	}

	// Always put the runtime code first before anything else
	visit(runtimeSourceIndex)
	for _, data := range sorted {
		visit(data.sourceIndex)
	}
	return order
}

func (c *linkerContext) convertStmtsForExport(sourceIndex uint32, stmts []ast.Stmt, partStmts []ast.Stmt) []ast.Stmt {
	for _, stmt := range partStmts {
		switch s := stmt.Data.(type) {
		case *ast.SImport:
			// Turn imports of CommonJS files into require() calls
			if otherSourceIndex, ok := c.files[sourceIndex].resolveImport(s.Path); ok {
				if c.fileMeta[otherSourceIndex].isCommonJS {
					stmt.Data = &ast.SLocal{Kind: ast.LocalConst, Decls: []ast.Decl{ast.Decl{
						ast.Binding{stmt.Loc, &ast.BIdentifier{s.NamespaceRef}},
						&ast.Expr{s.Path.Loc, &ast.ERequire{Path: s.Path, IsES6Import: true}},
					}}}
					break
				}
			}

			// Remove import statements entirely
			continue

		case *ast.SExportStar, *ast.SExportFrom, *ast.SExportClause:
			// Remove export statements entirely
			continue

		case *ast.SFunction:
			// Strip the "export" keyword while bundling
			if s.IsExport {
				clone := *s
				clone.IsExport = false
				stmt.Data = &clone
			}

		case *ast.SClass:
			// Strip the "export" keyword while bundling
			if s.IsExport {
				clone := *s
				clone.IsExport = false
				stmt.Data = &clone
			}

		case *ast.SLocal:
			// Strip the "export" keyword while bundling
			if s.IsExport {
				clone := *s
				clone.IsExport = false
				stmt.Data = &clone
			}

		case *ast.SExportDefault:
			// If we're bundling, convert "export default" into a normal declaration
			if s.Value.Expr != nil {
				// "export default foo;" => "const default = foo;"
				stmt = ast.Stmt{stmt.Loc, &ast.SLocal{Kind: ast.LocalConst, Decls: []ast.Decl{
					ast.Decl{ast.Binding{s.DefaultName.Loc, &ast.BIdentifier{s.DefaultName.Ref}}, s.Value.Expr},
				}}}
			} else {
				switch s2 := s.Value.Stmt.Data.(type) {
				case *ast.SFunction:
					// "export default function() {}" => "function default() {}"
					// "export default function foo() {}" => "function foo() {}"
					if s2.Fn.Name == nil {
						s2 = &ast.SFunction{Fn: s2.Fn}
						s2.Fn.Name = &s.DefaultName
					}
					stmt = ast.Stmt{s.Value.Stmt.Loc, s2}

				case *ast.SClass:
					// "export default class {}" => "class default {}"
					// "export default class Foo {}" => "class Foo {}"
					if s2.Class.Name == nil {
						s2 = &ast.SClass{Class: s2.Class}
						s2.Class.Name = &s.DefaultName
					}
					stmt = ast.Stmt{s.Value.Stmt.Loc, s2}

				default:
					panic("Internal error")
				}
			}
		}

		stmts = append(stmts, stmt)
	}

	return stmts
}

func (c *linkerContext) generateChunk(chunk chunkMeta) BundleResult {
	js := []byte{}
	compileResults := make([]compileResult, 0, len(chunk.filesInChunk))
	runtimeMembers := c.files[runtimeSourceIndex].ast.ModuleScope.Members
	commonJSRef := ast.FollowSymbols(c.symbols, runtimeMembers["__commonJS"])
	toModuleRef := ast.FollowSymbols(c.symbols, runtimeMembers["__toModule"])
	prevOffset := 0

	// Make sure the printer can require() CommonJS modules
	wrapperRefs := make([]ast.Ref, len(c.files))
	for sourceIndex, file := range c.files {
		wrapperRefs[sourceIndex] = file.ast.WrapperRef
	}

	for _, sourceIndex := range c.chunkFileOrder(chunk) {
		file := &c.files[sourceIndex]
		fileMeta := &c.fileMeta[sourceIndex]
		stmts := []ast.Stmt{}

		// Skip the runtime in test output
		if sourceIndex == runtimeSourceIndex && c.options.omitRuntimeForTests {
			continue
		}

		// Add all parts in this file that belong in this chunk
		for partIndex, part := range file.ast.Parts {
			if chunk.entryBits.equals(fileMeta.partMeta[partIndex].entryBits) {
				stmts = c.convertStmtsForExport(sourceIndex, stmts, part.Stmts)
			}
		}

		// Optionally wrap all statements in a closure for CommonJS
		if fileMeta.isCommonJS {
			exportsRef := ast.FollowSymbols(c.symbols, file.ast.ExportsRef)
			moduleRef := ast.FollowSymbols(c.symbols, file.ast.ModuleRef)
			wrapperRef := ast.FollowSymbols(c.symbols, file.ast.WrapperRef)

			// Only include the arguments that are actually used
			args := []ast.Arg{}
			if file.ast.UsesExportsRef || file.ast.UsesModuleRef {
				args = append(args, ast.Arg{Binding: ast.Binding{Data: &ast.BIdentifier{exportsRef}}})
				if file.ast.UsesModuleRef {
					args = append(args, ast.Arg{Binding: ast.Binding{Data: &ast.BIdentifier{moduleRef}}})
				}
			}

			// "var require_foo = __commonJS((exports, module) => { ... });"
			stmts = []ast.Stmt{ast.Stmt{Data: &ast.SLocal{
				Decls: []ast.Decl{ast.Decl{
					Binding: ast.Binding{Data: &ast.BIdentifier{wrapperRef}},
					Value: &ast.Expr{Data: &ast.ECall{
						Target: ast.Expr{Data: &ast.EIdentifier{commonJSRef}},
						Args:   []ast.Expr{ast.Expr{Data: &ast.EArrow{Args: args, Body: ast.FnBody{Stmts: stmts}}}},
					}},
				}},
			}}}
		}

		sourceMapContents := &c.sources[sourceIndex].Contents
		if c.options.SourceMap == SourceMapNone {
			sourceMapContents = nil
		}
		tree := file.ast
		tree.Parts = []ast.Part{ast.Part{Stmts: stmts}}
		compileResult := compileResult{
			PrintResult: printer.Print(tree, printer.PrintOptions{
				RemoveWhitespace:  c.options.RemoveWhitespace,
				ResolvedImports:   file.resolvedImports,
				ToModuleRef:       toModuleRef,
				WrapperRefs:       wrapperRefs,
				SourceMapContents: sourceMapContents,
			}),
			sourceIndex: sourceIndex,
		}
		if c.options.SourceMap != SourceMapNone {
			compileResult.quotedSource = printer.QuoteForJSON(c.sources[sourceIndex].Contents)
		}

		if !c.options.RemoveWhitespace && sourceIndex != runtimeSourceIndex {
			if len(js) > 0 {
				js = append(js, '\n')
			}
			js = append(js, fmt.Sprintf("// %s\n", c.sources[sourceIndex].PrettyPath)...)
		}

		// Save the offset to the start of the stored JavaScript
		compileResult.generatedOffset = computeLineColumnOffset(js[prevOffset:])
		js = append(js, compileResult.JS...)
		prevOffset = len(js)

		compileResults = append(compileResults, compileResult)
	}

	// Make sure the file ends with a newline
	if len(js) > 0 && js[len(js)-1] != '\n' {
		js = append(js, '\n')
	}

	result := BundleResult{
		JsAbsPath:  c.fs.Join(c.options.AbsOutputDir, chunk.name),
		JsContents: js,
	}

	// Stop now if we don't need to generate a source map
	if c.options.SourceMap == SourceMapNone {
		return result
	}

	sourceMap := c.generateSourceMapForChunk(compileResults)

	// Store the generated source map
	switch c.options.SourceMap {
	case SourceMapInline:
		if c.options.RemoveWhitespace {
			result.JsContents = removeTrailing(result.JsContents, '\n')
		}
		result.JsContents = append(result.JsContents,
			("//# sourceMappingURL=data:application/json;base64," +
				base64.StdEncoding.EncodeToString(sourceMap) + "\n")...)

	case SourceMapLinkedWithComment, SourceMapExternalWithoutComment:
		result.SourceMapAbsPath = result.JsAbsPath + ".map"
		result.SourceMapContents = sourceMap

		// Add a comment linking the source to its map
		if c.options.SourceMap == SourceMapLinkedWithComment {
			if c.options.RemoveWhitespace {
				result.JsContents = removeTrailing(result.JsContents, '\n')
			}
			result.JsContents = append(result.JsContents,
				("//# sourceMappingURL=" + c.fs.Base(result.SourceMapAbsPath) + "\n")...)
		}
	}

	return result
}

func removeTrailing(x []byte, c byte) []byte {
	if len(x) > 0 && x[len(x)-1] == c {
		x = x[:len(x)-1]
	}
	return x
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

func (c *linkerContext) renameOrMinifyAllSymbols() {
	topLevelScopes := []*ast.Scope{}
	moduleScopes := []*ast.Scope{}
	nestedScopes := []*ast.Scope{}

	// Combine all file scopes
	for sourceIndex, file := range c.files {
		moduleScopes = append(moduleScopes, file.ast.ModuleScope)
		if c.fileMeta[sourceIndex].isCommonJS {
			nestedScopes = append(nestedScopes, file.ast.ModuleScope)
		} else {
			topLevelScopes = append(topLevelScopes, file.ast.ModuleScope)
			nestedScopes = append(nestedScopes, file.ast.ModuleScope.Children...)

			// If this isn't CommonJS, then rename the unused "exports" and "module"
			// variables to avoid them causing the identically-named variables in
			// actual CommonJS files from being renamed. This is purely about
			// aesthetics and is not about correctness.
			name := ast.GenerateNonUniqueNameFromPath(c.sources[sourceIndex].AbsolutePath)
			c.symbols.SetName(file.ast.ExportsRef, name+"_exports")
			c.symbols.SetName(file.ast.ModuleRef, name+"_module")
		}
	}

	// Avoid collisions with any unbound symbols in this module group
	reservedNames := computeReservedNames(moduleScopes, c.symbols)
	if c.options.IsBundling {
		// These are used to implement bundling, and need to be free for use
		reservedNames["require"] = true
		reservedNames["Promise"] = true
	}

	if c.options.MinifyIdentifiers {
		minifyAllSymbols(reservedNames, topLevelScopes, nestedScopes, c.symbols)
	} else {
		renameAllSymbols(reservedNames, topLevelScopes, nestedScopes, c.symbols)
	}
}

func (c *linkerContext) generateSourceMapForChunk(results []compileResult) []byte {
	buffer := []byte{}
	buffer = append(buffer, "{\n  \"version\": 3"...)

	// Write the sources
	buffer = append(buffer, ",\n  \"sources\": ["...)
	comma := ""
	for _, result := range results {
		sourceFile := c.sources[result.sourceIndex].PrettyPath
		if c.options.SourceFile != "" {
			sourceFile = c.options.SourceFile
		}
		buffer = append(buffer, comma...)
		buffer = append(buffer, printer.QuoteForJSON(sourceFile)...)
		comma = ", "
	}
	buffer = append(buffer, ']')

	// Write the sourcesContent
	buffer = append(buffer, ",\n  \"sourcesContent\": ["...)
	comma = ""
	for _, result := range results {
		buffer = append(buffer, comma...)
		buffer = append(buffer, result.quotedSource...)
		comma = ", "
	}
	buffer = append(buffer, ']')

	// Write the mappings
	buffer = append(buffer, ",\n  \"mappings\": \""...)
	prevEndState := printer.SourceMapState{}
	prevColumnOffset := 0
	sourceMapIndex := 0
	for _, result := range results {
		chunk := result.SourceMapChunk
		offset := result.generatedOffset

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
	buffer = append(buffer, '"')

	// Finish the source map
	buffer = append(buffer, ",\n  \"names\": []\n}\n"...)
	return buffer
}
