package bundler

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/lexer"
	"github.com/evanw/esbuild/internal/logging"
	"github.com/evanw/esbuild/internal/printer"
	"github.com/evanw/esbuild/internal/runtime"
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

func (bs bitSet) copyFrom(other bitSet) {
	copy(bs.entries, other.entries)
}

func (bs *bitSet) bitwiseOrWith(other bitSet) {
	for i := range bs.entries {
		bs.entries[i] |= other.entries[i]
	}
}

type linkerContext struct {
	options     *config.Options
	log         logging.Log
	fs          fs.FS
	symbols     ast.SymbolMap
	entryPoints []uint32
	sources     []logging.Source
	files       []file
	fileMeta    []fileMeta
	hasErrors   bool

	// We should avoid traversing all files in the bundle, because the linker
	// should be able to run a linking operation on a large bundle where only
	// a few files are needed (e.g. an incremental compilation scenario). This
	// holds all files that could possibly be reached through the entry points.
	// If you need to iterate over all files in the linking operation, iterate
	// over this array. This array is also sorted in a deterministic ordering
	// to help ensure deterministic builds (source indices are random).
	reachableFiles []uint32

	// We may need to refer to the CommonJS "module" symbol for exports
	unboundModuleRef ast.Ref
}

type entryPointStatus uint8

const (
	entryPointNone entryPointStatus = iota
	entryPointUserSpecified
	entryPointDynamicImport
)

// This contains linker-specific metadata corresponding to a "file" struct
// from the initial scan phase of the bundler. It's separated out because it's
// conceptually only used for a single linking operation and because multiple
// linking operations may be happening in parallel with different metadata for
// the same file.
type fileMeta struct {
	partMeta         []partMeta
	entryPointName   string
	entryPointStatus entryPointStatus

	// This is the index to the automatically-generated part containing code that
	// calls "__export(exports, { ... getters ... })". This is used to generate
	// getters on an exports object for ES6 export statements, and is both for
	// ES6 star imports and CommonJS-style modules.
	nsExportPartIndex uint32

	// The index of the automatically-generated part containing export statements
	// for every export in the entry point. This also contains the call to the
	// require wrapper for CommonJS-style entry points.
	entryPointExportPartIndex *uint32

	// This is only for TypeScript files. If an import symbol is in this map, it
	// means the import couldn't be found and doesn't actually exist. This is not
	// an error in TypeScript because the import is probably just a type.
	//
	// Normally we remove all unused imports for TypeScript files during parsing,
	// which automatically removes type-only imports. But there are certain re-
	// export situations where it's impossible to tell if an import is a type or
	// not:
	//
	//   import {typeOrNotTypeWhoKnows} from 'path';
	//   export {typeOrNotTypeWhoKnows};
	//
	// Really people should be using the TypeScript "isolatedModules" flag with
	// bundlers like this one that compile TypeScript files independently without
	// type checking. That causes the TypeScript type checker to emit the error
	// "Re-exporting a type when the '--isolatedModules' flag is provided requires
	// using 'export type'." But we try to be robust to such code anyway.
	isProbablyTypeScriptType map[ast.Ref]bool

	// Imports are matched with exports in a separate pass from when the matched
	// exports are actually bound to the imports. Here "binding" means adding non-
	// local dependencies on the parts in the exporting file that declare the
	// exported symbol to all parts in the importing file that use the imported
	// symbol.
	//
	// This must be a separate pass because of the "probably TypeScript type"
	// check above. We can't generate the part for the export namespace until
	// we've matched imports with exports because the generated code must omit
	// type-only imports in the export namespace code. And we can't bind exports
	// to imports until the part for the export namespace is generated since that
	// part needs to participate in the binding.
	//
	// This array holds the deferred imports to bind so the pass can be split
	// into two separate passes.
	importsToBind map[ast.Ref]importToBind

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
	cjsWrap bool

	// If true, all exports must be reached via property accesses off a call to
	// the CommonJS wrapper for this module. In addition, all ES6 exports for
	// this module must be added as getters to the CommonJS "exports" object.
	cjsStyleExports bool

	// The index of the automatically-generated part used to represent the
	// CommonJS wrapper. This part is empty and is only useful for tree shaking
	// and code splitting. The CommonJS wrapper can't be inserted into the part
	// because the wrapper contains other parts, which can't be represented by
	// the current part system.
	cjsWrapperPartIndex *uint32

	// The minimum number of links in the module graph to get from an entry point
	// to this file
	distanceFromEntryPoint uint32

	// This holds all entry points that can reach this file. It will be used to
	// assign the parts in this file to a chunk.
	entryBits bitSet

	// This includes both named exports and re-exports.
	//
	// Named exports come from explicit export statements in the original file,
	// and are copied from the "NamedExports" field in the AST.
	//
	// Re-exports come from other files and are the result of resolving export
	// star statements (i.e. "export * from 'foo'").
	resolvedExports map[string]exportData

	// Never iterate over "resolvedExports" directly. Instead, iterate over this
	// array. Some exports in that map aren't meant to end up in generated code.
	// This array excludes these exports and is also sorted, which avoids non-
	// determinism due to random map iteration order.
	sortedAndFilteredExportAliases []string
}

type importToBind struct {
	sourceIndex uint32
	ref         ast.Ref
}

type exportData struct {
	ref ast.Ref

	// The location of the path string for error messages. This is only from re-
	// exports (i.e. "export * from 'foo'").
	pathLoc *ast.Loc

	// This is the file that the named export above came from. This will be
	// different from the file that contains this object if this is a re-export.
	sourceIndex uint32

	// Exports from export stars are shadowed by other exports. This flag helps
	// implement this behavior.
	isFromExportStar bool

	// If export star resolution finds two or more symbols with the same name,
	// then the name is a ambiguous and cannot be used. This causes the name to
	// be silently omitted from any namespace imports and causes a compile-time
	// failure if the name is used in an ES6 import statement.
	isAmbiguous bool
}

// This contains linker-specific metadata corresponding to an "ast.Part" struct
// from the initial scan phase of the bundler. It's separated out because it's
// conceptually only used for a single linking operation and because multiple
// linking operations may be happening in parallel with different metadata for
// the same part in the same file.
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
	name                  string
	filesWithPartsInChunk map[uint32]bool
	entryBits             bitSet

	// This information is only useful if "isEntryPoint" is true
	isEntryPoint  bool
	sourceIndex   uint32 // An index into "c.sources"
	entryPointBit uint   // An index into "c.entryPoints"

	// For code splitting
	crossChunkImportRecords []ast.ImportRecord
	crossChunkPrefixStmts   []ast.Stmt
	crossChunkSuffixStmts   []ast.Stmt
}

func newLinkerContext(options *config.Options, log logging.Log, fs fs.FS, sources []logging.Source, files []file, entryPoints []uint32) linkerContext {
	// Clone information about symbols and files so we don't mutate the input data
	c := linkerContext{
		options:        options,
		log:            log,
		fs:             fs,
		sources:        sources,
		entryPoints:    append([]uint32{}, entryPoints...),
		files:          make([]file, len(files)),
		fileMeta:       make([]fileMeta, len(files)),
		symbols:        ast.NewSymbolMap(len(files)),
		reachableFiles: findReachableFiles(sources, files, entryPoints),
	}

	// Clone various things since we may mutate them later
	for _, sourceIndex := range c.reachableFiles {
		file := files[sourceIndex]

		// Clone the symbol map
		fileSymbols := append([]ast.Symbol{}, file.ast.Symbols.Outer[sourceIndex]...)
		c.symbols.Outer[sourceIndex] = fileSymbols
		file.ast.Symbols = c.symbols

		// Zero out the use count statistics. These will be recomputed later after
		// taking tree shaking into account.
		for i := range fileSymbols {
			fileSymbols[i].UseCountEstimate = 0
		}

		// Clone the parts
		file.ast.Parts = append([]ast.Part{}, file.ast.Parts...)
		for i, part := range file.ast.Parts {
			clone := make(map[ast.Ref]ast.SymbolUse, len(part.SymbolUses))
			for ref, uses := range part.SymbolUses {
				clone[ref] = uses
			}
			file.ast.Parts[i].SymbolUses = clone
		}

		// Clone the import records
		file.ast.ImportRecords = append([]ast.ImportRecord{}, file.ast.ImportRecords...)

		// Clone the import map
		namedImports := make(map[ast.Ref]ast.NamedImport, len(file.ast.NamedImports))
		for k, v := range file.ast.NamedImports {
			namedImports[k] = v
		}
		file.ast.NamedImports = namedImports

		// Clone the export map
		resolvedExports := make(map[string]exportData)
		for alias, ref := range file.ast.NamedExports {
			resolvedExports[alias] = exportData{
				ref:         ref,
				sourceIndex: sourceIndex,
			}
		}

		// Clone the top-level symbol-to-parts map
		topLevelSymbolToParts := make(map[ast.Ref][]uint32)
		for ref, parts := range file.ast.TopLevelSymbolToParts {
			topLevelSymbolToParts[ref] = parts
		}
		file.ast.TopLevelSymbolToParts = topLevelSymbolToParts

		// Clone the top-level scope so we can generate more variables
		{
			new := &ast.Scope{}
			*new = *file.ast.ModuleScope
			new.Generated = append([]ast.Ref{}, new.Generated...)
			file.ast.ModuleScope = new
		}

		// Update the file in our copy of the file array
		c.files[sourceIndex] = file

		// Also associate some default metadata with the file
		c.fileMeta[sourceIndex] = fileMeta{
			distanceFromEntryPoint:   ^uint32(0),
			cjsStyleExports:          file.ast.HasCommonJSFeatures() || (file.ast.HasLazyExport && !c.options.IsBundling),
			partMeta:                 make([]partMeta, len(file.ast.Parts)),
			resolvedExports:          resolvedExports,
			isProbablyTypeScriptType: make(map[ast.Ref]bool),
			importsToBind:            make(map[ast.Ref]importToBind),
		}
	}

	// Mark all entry points so we don't add them again for import() expressions
	for _, sourceIndex := range entryPoints {
		fileMeta := &c.fileMeta[sourceIndex]
		fileMeta.entryPointStatus = entryPointUserSpecified

		// Entry points must be CommonJS-style if the output format doesn't support
		// ES6 export syntax
		if !options.OutputFormat.KeepES6ImportExportSyntax() && c.files[sourceIndex].ast.HasES6Exports {
			fileMeta.cjsStyleExports = true
		}
	}

	// Allocate a new unbound symbol called "module" in case we need it later
	{
		runtimeSymbols := &c.symbols.Outer[runtime.SourceIndex]
		c.unboundModuleRef = ast.Ref{OuterIndex: runtime.SourceIndex, InnerIndex: uint32(len(*runtimeSymbols))}
		*runtimeSymbols = append(*runtimeSymbols, ast.Symbol{
			Kind: ast.SymbolUnbound,
			Name: "module",
			Link: ast.InvalidRef,
		})
	}

	return c
}

type indexAndPath struct {
	sourceIndex uint32
	path        ast.Path
}

// This type is just so we can use Go's native sort function
type indexAndPathArray []indexAndPath

func (a indexAndPathArray) Len() int          { return len(a) }
func (a indexAndPathArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a indexAndPathArray) Less(i int, j int) bool {
	return a[i].path.ComesBeforeInSortedOrder(a[j].path)
}

// Find all files reachable from all entry points
func findReachableFiles(sources []logging.Source, files []file, entryPoints []uint32) []uint32 {
	visited := make(map[uint32]bool)
	sorted := indexAndPathArray{}
	var visit func(uint32)

	// Include this file and all files it imports
	visit = func(sourceIndex uint32) {
		if !visited[sourceIndex] {
			visited[sourceIndex] = true
			file := files[sourceIndex]
			for _, part := range file.ast.Parts {
				for _, importRecordIndex := range part.ImportRecordIndices {
					if record := &file.ast.ImportRecords[importRecordIndex]; record.SourceIndex != nil {
						visit(*record.SourceIndex)
					}
				}
			}
			sorted = append(sorted, indexAndPath{sourceIndex, sources[sourceIndex].KeyPath})
		}
	}

	// The runtime is always included in case it's needed
	visit(runtime.SourceIndex)

	// Include all files reachable from any entry point
	for _, entryPoint := range entryPoints {
		visit(entryPoint)
	}

	// Sort by absolute path for determinism
	sort.Sort(sorted)

	// Extract the source indices
	reachableFiles := make([]uint32, len(sorted))
	for i, item := range sorted {
		reachableFiles[i] = item.sourceIndex
	}
	return reachableFiles
}

func (c *linkerContext) addRangeError(source logging.Source, r ast.Range, text string) {
	c.log.AddRangeError(&source, r, text)
	c.hasErrors = true
}

func (c *linkerContext) link() []OutputFile {
	c.scanImportsAndExports()

	// Stop now if there were errors
	if c.hasErrors {
		return []OutputFile{}
	}

	c.markPartsReachableFromEntryPoints()

	if !c.options.IsBundling {
		for _, entryPoint := range c.entryPoints {
			c.markExportsAsUnbound(entryPoint)
		}
	}

	// Make sure calls to "ast.FollowSymbols()" in parallel goroutines after this
	// won't hit concurrent map mutation hazards
	ast.FollowAllSymbols(c.symbols)

	c.renameOrMinifyAllSymbols()

	chunks := c.computeChunks()
	c.computeCrossChunkDependencies(chunks)

	// Generate chunks in parallel
	results := make([][]OutputFile, len(chunks))
	waitGroup := sync.WaitGroup{}
	waitGroup.Add(len(chunks))
	for i, chunk := range chunks {
		go func(i int, chunk chunkMeta) {
			results[i] = c.generateChunk(chunk)
			waitGroup.Done()
		}(i, chunk)
	}
	waitGroup.Wait()

	// Join the results in chunk order for determinism
	var outputFiles []OutputFile
	for _, group := range results {
		outputFiles = append(outputFiles, group...)
	}
	return outputFiles
}

func (c *linkerContext) computeCrossChunkDependencies(chunks []chunkMeta) {
	if len(chunks) < 2 {
		// No need to compute cross-chunk dependencies if there can't be any
		return
	}

	type chunkMeta struct {
		imports map[ast.Ref]bool
		exports map[ast.Ref]bool
	}

	topLevelDeclaredSymbolToChunk := make(map[ast.Ref]uint32)
	chunkMetas := make([]chunkMeta, len(chunks))

	// For each chunk, see what symbols it uses from other chunks
	for chunkIndex, chunk := range chunks {
		chunkKey := string(chunk.entryBits.entries)
		imports := make(map[ast.Ref]bool)
		chunkMetas[chunkIndex] = chunkMeta{imports: imports, exports: make(map[ast.Ref]bool)}

		// Go over each file in this chunk
		for sourceIndex := range chunk.filesWithPartsInChunk {
			file := &c.files[sourceIndex]
			fileMeta := &c.fileMeta[sourceIndex]

			// Go over each part in this file that's marked for inclusion in this chunk
			for partIndex, partMeta := range fileMeta.partMeta {
				if string(partMeta.entryBits.entries) != chunkKey {
					continue
				}
				part := &file.ast.Parts[partIndex]

				// Rewrite external dynamic imports to point to the chunk for that entry point
				for _, importRecordIndex := range part.ImportRecordIndices {
					record := &file.ast.ImportRecords[importRecordIndex]
					if record.SourceIndex != nil && c.isExternalDynamicImport(record) {
						record.Path.Text = "./" + c.fileMeta[*record.SourceIndex].entryPointName
						record.SourceIndex = nil
					}
				}

				// Remember what chunk each top-level symbol is declared in. Symbols
				// with multiple declarations such as repeated "var" statements with
				// the same name should already be marked as all being in a single
				// chunk. In that case this will overwrite the same value below which
				// is fine.
				for _, declared := range part.DeclaredSymbols {
					if declared.IsTopLevel {
						topLevelDeclaredSymbolToChunk[declared.Ref] = uint32(chunkIndex)
					}
				}

				// Record each symbol used in this part. This will later be matched up
				// with our map of which chunk a given symbol is declared in to
				// determine if the symbol needs to be imported from another chunk.
				for ref := range part.SymbolUses {
					symbol := c.symbols.Get(ref)

					// Ignore unbound symbols, which don't have declarations
					if symbol.Kind == ast.SymbolUnbound {
						continue
					}

					if importToBind, ok := fileMeta.importsToBind[ref]; ok {
						// If this is imported from another file, follow the import
						// reference and reference the symbol in that file instead
						ref = importToBind.ref
						symbol = c.symbols.Get(ref)
					} else if _, ok := file.ast.TopLevelSymbolToParts[ref]; !ok {
						// Skip symbols that aren't imports or top-level symbols
						continue
					}

					// If this is an ES6 import from a CommonJS file, it will become a
					// property access off the namespace symbol instead of a bare
					// identifier. In that case we want to pull in the namespace symbol
					// instead. The namespace symbol stores the result of "require()".
					if symbol.NamespaceAlias != nil {
						ref = symbol.NamespaceAlias.NamespaceRef
					}

					// We must record this relationship even for symbols that are not
					// imports. Due to code splitting, the definition of a symbol may
					// be moved to a separate chunk than the use of a symbol even if
					// the definition and use of that symbol are originally from the
					// same source file.
					imports[ref] = true
				}
			}
		}
	}

	// Generate cross-chunk imports
	for chunkIndex := range chunks {
		chunk := &chunks[chunkIndex]

		// Find all uses in this chunk of symbols from other chunks
		importsFromOtherChunks := make(map[uint32][]ast.Ref)
		for importRef := range chunkMetas[chunkIndex].imports {
			otherChunkIndex, ok := topLevelDeclaredSymbolToChunk[importRef]
			if !ok {
				panic("Internal error")
			}
			if otherChunkIndex != uint32(chunkIndex) {
				importsFromOtherChunks[otherChunkIndex] = append(importsFromOtherChunks[otherChunkIndex], importRef)
				chunkMetas[otherChunkIndex].exports[importRef] = true
			}
		}

		// If this is an entry point, make sure we import all chunks belonging to
		// this entry point, even if there are no imports. We need to make sure
		// these chunks are evaluated for their side effects too.
		if chunk.isEntryPoint {
			for otherChunkIndex, otherChunk := range chunks {
				if chunkIndex != otherChunkIndex && otherChunk.entryBits.hasBit(chunk.entryPointBit) {
					imports := importsFromOtherChunks[uint32(otherChunkIndex)]
					importsFromOtherChunks[uint32(otherChunkIndex)] = imports
				}
			}
		}

		var crossChunkImportRecords []ast.ImportRecord
		var crossChunkPrefixStmts []ast.Stmt

		for _, crossChunkImport := range c.sortedCrossChunkImports(chunks, importsFromOtherChunks) {
			switch c.options.OutputFormat {
			case config.FormatESModule:
				var items []ast.ClauseItem
				for _, alias := range crossChunkImport.sortedImportAliases {
					items = append(items, ast.ClauseItem{Name: ast.LocRef{Ref: alias.ref}, Alias: alias.name})
				}
				importRecordIndex := uint32(len(crossChunkImportRecords))
				crossChunkImportRecords = append(crossChunkImportRecords, ast.ImportRecord{
					Kind: ast.ImportStmt,
					Path: ast.Path{Text: "./" + chunks[crossChunkImport.chunkIndex].name},
				})
				if len(items) > 0 {
					// "import {a, b} from './chunk.js'"
					crossChunkPrefixStmts = append(crossChunkPrefixStmts, ast.Stmt{Data: &ast.SImport{
						Items:             &items,
						ImportRecordIndex: importRecordIndex,
					}})
				} else {
					// "import './chunk.js'"
					crossChunkPrefixStmts = append(crossChunkPrefixStmts, ast.Stmt{Data: &ast.SImport{
						ImportRecordIndex: importRecordIndex,
					}})
				}

			default:
				panic("Internal error")
			}
		}

		chunk.crossChunkImportRecords = crossChunkImportRecords
		chunk.crossChunkPrefixStmts = crossChunkPrefixStmts
	}

	// Generate cross-chunk exports
	for chunkIndex := range chunks {
		switch c.options.OutputFormat {
		case config.FormatESModule:
			var items []ast.ClauseItem
			for _, alias := range c.sortedCrossChunkExportRefs(chunkMetas[chunkIndex].exports) {
				items = append(items, ast.ClauseItem{Name: ast.LocRef{Ref: alias.ref}, Alias: alias.name})
			}
			if len(items) > 0 {
				chunks[chunkIndex].crossChunkSuffixStmts = []ast.Stmt{{Data: &ast.SExportClause{
					Items: items,
				}}}
			}

		default:
			panic("Internal error")
		}
	}
}

type crossChunkImport struct {
	chunkIndex          uint32
	chunkName           string
	sortedImportAliases crossChunkAliasArray
}

// This type is just so we can use Go's native sort function
type crossChunkImportArray []crossChunkImport

func (a crossChunkImportArray) Len() int          { return len(a) }
func (a crossChunkImportArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a crossChunkImportArray) Less(i int, j int) bool {
	return a[i].chunkName < a[j].chunkName
}

// Sort cross-chunk imports by chunk name for determinism
func (c *linkerContext) sortedCrossChunkImports(chunks []chunkMeta, importsFromOtherChunks map[uint32][]ast.Ref) crossChunkImportArray {
	result := make(crossChunkImportArray, 0, len(importsFromOtherChunks))

	for otherChunkIndex, importRefs := range importsFromOtherChunks {
		result = append(result, crossChunkImport{
			chunkIndex:          otherChunkIndex,
			chunkName:           chunks[otherChunkIndex].name,
			sortedImportAliases: c.sortedCrossChunkImportRefs(importRefs),
		})
	}

	sort.Sort(result)
	return result
}

type crossChunkAlias struct {
	name string
	ref  ast.Ref
}

// This type is just so we can use Go's native sort function
type crossChunkAliasArray []crossChunkAlias

func (a crossChunkAliasArray) Len() int          { return len(a) }
func (a crossChunkAliasArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a crossChunkAliasArray) Less(i int, j int) bool {
	return a[i].name < a[j].name
}

// Sort cross-chunk imports by chunk name for determinism
func (c *linkerContext) sortedCrossChunkImportRefs(importRefs []ast.Ref) crossChunkAliasArray {
	result := make(crossChunkAliasArray, 0, len(importRefs))
	for _, ref := range importRefs {
		result = append(result, crossChunkAlias{ref: ref, name: c.symbols.Get(ref).Name})
	}
	sort.Sort(result)
	return result
}

// Sort cross-chunk exports by chunk name for determinism
func (c *linkerContext) sortedCrossChunkExportRefs(exportRefs map[ast.Ref]bool) crossChunkAliasArray {
	result := make(crossChunkAliasArray, 0, len(exportRefs))
	for ref := range exportRefs {
		result = append(result, crossChunkAlias{ref: ref, name: c.symbols.Get(ref).Name})
	}
	sort.Sort(result)
	return result
}

func (c *linkerContext) scanImportsAndExports() {
	// Step 1: Figure out what modules must be CommonJS
	for _, sourceIndex := range c.reachableFiles {
		file := &c.files[sourceIndex]
		for _, part := range file.ast.Parts {
			// Handle require() and import()
			for _, importRecordIndex := range part.ImportRecordIndices {
				record := &file.ast.ImportRecords[importRecordIndex]

				// Make sure the printer can require() CommonJS modules
				if record.SourceIndex != nil {
					record.WrapperRef = c.files[*record.SourceIndex].ast.WrapperRef
				}

				switch record.Kind {
				case ast.ImportStmt:
					// Importing using ES6 syntax from a file without any ES6 syntax
					// causes that module to be considered CommonJS-style, even if it
					// doesn't have any CommonJS exports.
					//
					// That means the ES6 imports will silently become undefined instead
					// of causing errors. This is for compatibility with older CommonJS-
					// style bundlers.
					//
					// I've only come across a single case where this mattered, in the
					// package https://github.com/megawac/MutationObserver.js. The library
					// used to look like this:
					//
					//   this.MutationObserver = this.MutationObserver || (function() {
					//     ...
					//     return MutationObserver;
					//   })();
					//
					// That is compatible with CommonJS since "this" is an alias for
					// "exports". The code in question used the package like this:
					//
					//   import MutationObserver from '@sheerun/mutationobserver-shim';
					//
					// Then the library was updated to do this instead:
					//
					//   window.MutationObserver = window.MutationObserver || (function() {
					//     ...
					//     return MutationObserver;
					//   })();
					//
					// The package was updated without the ES6 import being removed. The
					// code still has the import but "MutationObserver" is now undefined:
					//
					//   import MutationObserver from '@sheerun/mutationobserver-shim';
					//
					if !record.DoesNotUseExports && record.SourceIndex != nil {
						otherSourceIndex := *record.SourceIndex
						otherFile := &c.files[otherSourceIndex]
						if !otherFile.ast.HasES6Syntax() && !otherFile.ast.HasLazyExport {
							c.fileMeta[otherSourceIndex].cjsStyleExports = true
						}
					}

				case ast.ImportRequire:
					// Files that are imported with require() must be CommonJS modules
					if record.SourceIndex != nil {
						c.fileMeta[*record.SourceIndex].cjsStyleExports = true
					}

				case ast.ImportDynamic:
					if c.options.CodeSplitting {
						// Files that are imported with import() must be entry points
						if record.SourceIndex != nil {
							otherFileMeta := &c.fileMeta[*record.SourceIndex]
							if otherFileMeta.entryPointStatus == entryPointNone {
								c.entryPoints = append(c.entryPoints, *record.SourceIndex)
								otherFileMeta.entryPointStatus = entryPointDynamicImport
							}
						}
					} else {
						// If we're not splitting, then import() is just a require() that
						// returns a promise, so the imported file must be a CommonJS module
						if record.SourceIndex != nil {
							c.fileMeta[*record.SourceIndex].cjsStyleExports = true
						}
					}
				}
			}
		}
	}

	// Step 2: Propagate CommonJS status for export star statements that are re-
	// exports from a CommonJS module. Exports from a CommonJS module are not
	// statically analyzable, so the export star must be evaluated at run time
	// instead of at bundle time.
	for _, sourceIndex := range c.reachableFiles {
		if len(c.files[sourceIndex].ast.ExportStarImportRecords) > 0 {
			visited := make(map[uint32]bool)
			c.isCommonJSDueToExportStar(sourceIndex, visited)
		}
	}

	// Step 3: Resolve "export * from" statements. This must be done after we
	// discover all modules that can be CommonJS because export stars are ignored
	// for CommonJS modules.
	for _, sourceIndex := range c.reachableFiles {
		file := &c.files[sourceIndex]
		fileMeta := &c.fileMeta[sourceIndex]

		// Expression-style loaders defer code generation until linking. Code
		// generation is done here because at this point we know that the
		// "cjsStyleExports" flag has its final value and will not be changed.
		if file.ast.HasLazyExport {
			c.generateCodeForLazyExport(sourceIndex, file, fileMeta)
		}

		// Even if the output file is CommonJS-like, we may still need to wrap
		// CommonJS-style files. Any file that imports a CommonJS-style file will
		// cause that file to need to be wrapped. This is because the import
		// method, whatever it is, will need to invoke the wrapper. Note that
		// this can include entry points (e.g. an entry point that imports a file
		// that imports that entry point).
		for _, part := range file.ast.Parts {
			for _, importRecordIndex := range part.ImportRecordIndices {
				if record := &file.ast.ImportRecords[importRecordIndex]; record.SourceIndex != nil {
					otherFileMeta := &c.fileMeta[*record.SourceIndex]
					if otherFileMeta.cjsStyleExports {
						otherFileMeta.cjsWrap = true
					}
				}
			}
		}

		// Propagate exports for export star statements
		if len(file.ast.ExportStarImportRecords) > 0 {
			visited := make(map[uint32]bool)
			c.addExportsForExportStar(fileMeta.resolvedExports, sourceIndex, nil, visited)
		}

		// Add an empty part for the namespace export that we can fill in later
		fileMeta.nsExportPartIndex = uint32(len(file.ast.Parts))
		file.ast.Parts = append(file.ast.Parts, ast.Part{
			LocalDependencies:    make(map[uint32]bool),
			SymbolUses:           make(map[ast.Ref]ast.SymbolUse),
			CanBeRemovedIfUnused: true,
			IsNamespaceExport:    true,
		})
		fileMeta.partMeta = append(fileMeta.partMeta, partMeta{
			entryBits: newBitSet(uint(len(c.entryPoints))),
		})

		// Also add a special export called "*" so import stars can bind to it.
		// This must be done in this step because it must come after CommonJS
		// module discovery but before matching imports with exports.
		fileMeta.resolvedExports["*"] = exportData{
			ref:         file.ast.ExportsRef,
			sourceIndex: sourceIndex,
		}
		file.ast.TopLevelSymbolToParts[file.ast.ExportsRef] = []uint32{fileMeta.nsExportPartIndex}
	}

	// Step 4: Match imports with exports. This must be done after we process all
	// export stars because imports can bind to export star re-exports.
	for _, sourceIndex := range c.reachableFiles {
		file := &c.files[sourceIndex]
		fileMeta := &c.fileMeta[sourceIndex]

		if len(file.ast.NamedImports) > 0 {
			c.matchImportsWithExportsForFile(uint32(sourceIndex))
		}

		// If the output format doesn't have an implicit CommonJS wrapper, any file
		// that uses CommonJS features will need to be wrapped, even though the
		// resulting wrapper won't be invoked by other files.
		if !fileMeta.cjsWrap && fileMeta.cjsStyleExports &&
			(c.options.OutputFormat == config.FormatIIFE ||
				c.options.OutputFormat == config.FormatESModule) {
			fileMeta.cjsWrap = true
		}

		// If we're exporting as CommonJS and this file doesn't need a wrapper,
		// then we'll be using the actual CommonJS "exports" and/or "module"
		// symbols. In that case make sure to mark them as such so they don't
		// get minified.
		if c.options.OutputFormat == config.FormatCommonJS && !fileMeta.cjsWrap &&
			fileMeta.entryPointStatus == entryPointUserSpecified {
			exportsRef := ast.FollowSymbols(c.symbols, file.ast.ExportsRef)
			moduleRef := ast.FollowSymbols(c.symbols, file.ast.ModuleRef)
			c.symbols.Get(exportsRef).Kind = ast.SymbolUnbound
			c.symbols.Get(moduleRef).Kind = ast.SymbolUnbound
		}
	}

	// Step 5: Create namespace exports for every file. This is always necessary
	// for CommonJS files, and is also necessary for other files if they are
	// imported using an import star statement.
	for _, sourceIndex := range c.reachableFiles {
		// Now that all exports have been resolved, sort and filter them to create
		// something we can iterate over later.
		fileMeta := &c.fileMeta[sourceIndex]
		aliases := make([]string, 0, len(fileMeta.resolvedExports))
		for alias, export := range fileMeta.resolvedExports {
			// The automatically-generated namespace export is just for internal binding
			// purposes and isn't meant to end up in generated code.
			if alias == "*" {
				continue
			}

			// Re-exporting multiple symbols with the same name causes an ambiguous
			// export. These names cannot be used and should not end up in generated code.
			if export.isAmbiguous {
				continue
			}

			// Ignore re-exported imports in TypeScript files that failed to be
			// resolved. These are probably just type-only imports so the best thing to
			// do is to silently omit them from the export list.
			if c.fileMeta[export.sourceIndex].isProbablyTypeScriptType[export.ref] {
				continue
			}

			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		fileMeta.sortedAndFilteredExportAliases = aliases

		// Export creation uses "sortedAndFilteredExportAliases" so this must
		// come second after we fill in that array
		c.createExportsForFile(uint32(sourceIndex))
	}

	// Step 6: Bind imports to exports. This adds non-local dependencies on the
	// parts that declare the export to all parts that use the import.
	for _, sourceIndex := range c.reachableFiles {
		file := &c.files[sourceIndex]
		fileMeta := &c.fileMeta[sourceIndex]

		for importRef, importToBind := range fileMeta.importsToBind {
			resolvedFile := &c.files[importToBind.sourceIndex]
			partsDeclaringSymbol := resolvedFile.ast.TopLevelSymbolToParts[importToBind.ref]

			for _, partIndex := range file.ast.NamedImports[importRef].LocalPartsWithUses {
				partMeta := &fileMeta.partMeta[partIndex]

				for _, resolvedPartIndex := range partsDeclaringSymbol {
					partMeta.nonLocalDependencies = append(partMeta.nonLocalDependencies, partRef{
						sourceIndex: importToBind.sourceIndex,
						partIndex:   resolvedPartIndex,
					})
				}
			}

			// Merge these symbols so they will share the same name
			ast.MergeSymbols(c.symbols, importRef, importToBind.ref)
		}
	}
}

func (c *linkerContext) generateCodeForLazyExport(sourceIndex uint32, file *file, fileMeta *fileMeta) {
	// Grab the lazy expression
	if len(file.ast.Parts) < 1 {
		panic("Internal error")
	}
	part := &file.ast.Parts[0]
	if len(part.Stmts) != 1 {
		panic("Internal error")
	}
	lazy, ok := part.Stmts[0].Data.(*ast.SLazyExport)
	if !ok {
		panic("Internal error")
	}

	// Use "module.exports = value" for CommonJS-style modules
	if fileMeta.cjsStyleExports {
		part.Stmts = []ast.Stmt{ast.AssignStmt(
			ast.Expr{Loc: lazy.Value.Loc, Data: &ast.EDot{
				Target:  ast.Expr{Loc: lazy.Value.Loc, Data: &ast.EIdentifier{Ref: file.ast.ModuleRef}},
				Name:    "exports",
				NameLoc: lazy.Value.Loc,
			}},
			lazy.Value,
		)}
		part.SymbolUses[file.ast.ModuleRef] = ast.SymbolUse{CountEstimate: 1}
		file.ast.UsesModuleRef = true
		return
	}

	// Otherwise, generate ES6 export statements. These are added as additional
	// parts so they can be tree shaken individually.
	part.Stmts = nil

	type prevExport struct {
		ref       ast.Ref
		partIndex uint32
	}

	generateExport := func(name string, alias string, value ast.Expr, prevExports []prevExport) prevExport {
		// Generate a new symbol
		inner := &c.symbols.Outer[sourceIndex]
		ref := ast.Ref{OuterIndex: sourceIndex, InnerIndex: uint32(len(*inner))}
		*inner = append(*inner, ast.Symbol{Kind: ast.SymbolOther, Name: name, Link: ast.InvalidRef})
		file.ast.ModuleScope.Generated = append(file.ast.ModuleScope.Generated, ref)

		// Generate an ES6 export
		var stmt ast.Stmt
		if alias == "default" {
			stmt = ast.Stmt{Loc: value.Loc, Data: &ast.SExportDefault{
				DefaultName: ast.LocRef{Loc: value.Loc, Ref: ref},
				Value:       ast.ExprOrStmt{Expr: &value},
			}}
		} else {
			stmt = ast.Stmt{Loc: value.Loc, Data: &ast.SLocal{
				IsExport: true,
				Decls: []ast.Decl{{
					Binding: ast.Binding{Loc: value.Loc, Data: &ast.BIdentifier{Ref: ref}},
					Value:   &value,
				}},
			}}
		}

		// Link the export into the graph for tree shaking
		partIndex := uint32(len(file.ast.Parts))
		file.ast.Parts = append(file.ast.Parts, ast.Part{
			Stmts:                []ast.Stmt{stmt},
			LocalDependencies:    make(map[uint32]bool),
			SymbolUses:           map[ast.Ref]ast.SymbolUse{file.ast.ModuleRef: ast.SymbolUse{CountEstimate: 1}},
			DeclaredSymbols:      []ast.DeclaredSymbol{{Ref: ref, IsTopLevel: true}},
			CanBeRemovedIfUnused: true,
		})
		fileMeta.partMeta = append(fileMeta.partMeta, partMeta{
			entryBits: newBitSet(uint(len(c.entryPoints))),
		})
		file.ast.TopLevelSymbolToParts[ref] = []uint32{partIndex}
		file.ast.NamedExports[alias] = ref
		fileMeta.resolvedExports[alias] = exportData{ref: ref, sourceIndex: sourceIndex}
		part := &file.ast.Parts[partIndex]
		for _, export := range prevExports {
			part.SymbolUses[export.ref] = ast.SymbolUse{CountEstimate: 1}
			part.LocalDependencies[export.partIndex] = true
		}
		return prevExport{ref: ref, partIndex: partIndex}
	}

	// Unwrap JSON objects into separate top-level variables
	var prevExports []prevExport
	if object, ok := lazy.Value.Data.(*ast.EObject); ok {
		for i, property := range object.Properties {
			if str, ok := property.Key.Data.(*ast.EString); ok && lexer.IsIdentifierUTF16(str.Value) {
				name := lexer.UTF16ToString(str.Value)
				export := generateExport(name, name, *property.Value, nil)
				prevExports = append(prevExports, export)
				object.Properties[i].Value = &ast.Expr{Loc: property.Key.Loc, Data: &ast.EIdentifier{Ref: export.ref}}
			}
		}
	}

	// Generate the default export
	generateExport(c.sources[sourceIndex].IdentifierName+"_default", "default", lazy.Value, prevExports)
}

func (c *linkerContext) createExportsForFile(sourceIndex uint32) {
	var entryPointES6ExportItems []ast.ClauseItem
	var entryPointExportStmts []ast.Stmt
	fileMeta := &c.fileMeta[sourceIndex]
	file := &c.files[sourceIndex]
	isEntryPoint := fileMeta.entryPointStatus != entryPointNone

	// If the output format is ES6 modules and we're an entry point, generate an
	// ES6 export statement containing all exports. Except don't do that if this
	// entry point is a CommonJS-style module, since that would generate an ES6
	// export statement that's not top-level. Instead, we will export the CommonJS
	// exports as a default export later on.
	needsEntryPointES6ExportPart := isEntryPoint && !fileMeta.cjsWrap &&
		c.options.OutputFormat == config.FormatESModule && len(fileMeta.sortedAndFilteredExportAliases) > 0

	// Generate a getter per export
	properties := []ast.Property{}
	nsExportNonLocalDependencies := []partRef{}
	entryPointExportNonLocalDependencies := []partRef{}
	nsExportSymbolUses := make(map[ast.Ref]ast.SymbolUse)
	entryPointExportSymbolUses := make(map[ast.Ref]ast.SymbolUse)
	for _, alias := range fileMeta.sortedAndFilteredExportAliases {
		export := fileMeta.resolvedExports[alias]

		// If this is an export of an import, reference the symbol that the import
		// was eventually resolved to. We need to do this because imports have
		// already been resolved by this point, so we can't generate a new import
		// and have that be resolved later.
		otherFileMeta := &c.fileMeta[export.sourceIndex]
		if importToBind, ok := otherFileMeta.importsToBind[export.ref]; ok {
			export.ref = importToBind.ref
			export.sourceIndex = importToBind.sourceIndex
		}

		// Exports of imports need EImportIdentifier in case they need to be re-
		// written to a property access later on
		var value ast.Expr
		if c.symbols.Get(export.ref).NamespaceAlias != nil {
			value = ast.Expr{Data: &ast.EImportIdentifier{Ref: export.ref}}

			// Imported identifiers must be assigned to a local variable to be
			// exported using an ES6 export clause. The import needs to be an
			// EImportIdentifier in case it's imported from a CommonJS module.
			if needsEntryPointES6ExportPart {
				// Generate a temporary variable
				inner := &c.symbols.Outer[sourceIndex]
				tempRef := ast.Ref{OuterIndex: sourceIndex, InnerIndex: uint32(len(*inner))}
				*inner = append(*inner, ast.Symbol{
					Kind: ast.SymbolOther,
					Name: "export_" + alias,
					Link: ast.InvalidRef,
				})

				// Stick it on the module scope so it gets renamed and minified
				generated := &file.ast.ModuleScope.Generated
				*generated = append(*generated, tempRef)

				// Create both a local variable and an export clause for that variable.
				// The local variable is initialized with the initial value of the
				// export. This isn't fully correct because it's a "dead" binding and
				// doesn't update with the "live" value as it changes. But ES6 modules
				// don't have any syntax for bare named getter functions so this is the
				// best we can do.
				//
				// These input files:
				//
				//   // entry_point.js
				//   export {foo} from './cjs-format.js'
				//
				//   // cjs-format.js
				//   Object.defineProperty(exports, 'foo', {
				//     enumerable: true,
				//     get: () => Math.random(),
				//   })
				//
				// Become this output file:
				//
				//   // cjs-format.js
				//   var require_cjs_format = __commonJS((exports) => {
				//     Object.defineProperty(exports, "foo", {
				//       enumerable: true,
				//       get: () => Math.random()
				//     });
				//   });
				//
				//   // entry_point.js
				//   const cjs_format = __toModule(require_cjs_format());
				//   const export_foo = cjs_format.foo;
				//   export {
				//     export_foo as foo
				//   };
				//
				entryPointExportStmts = append(entryPointExportStmts, ast.Stmt{Data: &ast.SLocal{
					Kind: ast.LocalConst,
					Decls: []ast.Decl{{
						Binding: ast.Binding{Data: &ast.BIdentifier{Ref: tempRef}},
						Value:   &ast.Expr{Data: &ast.EImportIdentifier{Ref: export.ref}},
					}},
				}})
				entryPointES6ExportItems = append(entryPointES6ExportItems, ast.ClauseItem{
					Name:  ast.LocRef{Ref: tempRef},
					Alias: alias,
				})
			}
		} else {
			value = ast.Expr{Data: &ast.EIdentifier{Ref: export.ref}}

			if needsEntryPointES6ExportPart {
				// Local identifiers can be exported using an export clause. This is done
				// this way instead of leaving the "export" keyword on the local declaration
				// itself both because it lets the local identifier be minified and because
				// it works transparently for re-exports across files.
				//
				// These input files:
				//
				//   // entry_point.js
				//   export * from './esm-format.js'
				//
				//   // esm-format.js
				//   export let foo = 123
				//
				// Become this output file:
				//
				//   // esm-format.js
				//   let foo = 123;
				//
				//   // entry_point.js
				//   export {
				//     foo
				//   };
				//
				entryPointES6ExportItems = append(entryPointES6ExportItems, ast.ClauseItem{
					Name:  ast.LocRef{Ref: export.ref},
					Alias: alias,
				})
			}
		}

		// Add a getter property
		properties = append(properties, ast.Property{
			Key: ast.Expr{Data: &ast.EString{Value: lexer.StringToUTF16(alias)}},
			Value: &ast.Expr{Data: &ast.EArrow{
				PreferExpr: true,
				Body:       ast.FnBody{Stmts: []ast.Stmt{{Loc: value.Loc, Data: &ast.SReturn{Value: &value}}}},
			}},
		})
		nsExportSymbolUses[export.ref] = ast.SymbolUse{CountEstimate: 1}
		if isEntryPoint {
			entryPointExportSymbolUses[export.ref] = ast.SymbolUse{CountEstimate: 1}
		}

		// Make sure the part that declares the export is included
		for _, partIndex := range c.files[export.sourceIndex].ast.TopLevelSymbolToParts[export.ref] {
			// Use a non-local dependency since this is likely from a different
			// file if it came in through an export star
			dep := partRef{sourceIndex: export.sourceIndex, partIndex: partIndex}
			nsExportNonLocalDependencies = append(nsExportNonLocalDependencies, dep)
			if isEntryPoint {
				entryPointExportNonLocalDependencies = append(entryPointExportNonLocalDependencies, dep)
			}
		}
	}

	// Prefix this part with "var exports = {}" if this isn't a CommonJS module
	declaredSymbols := []ast.DeclaredSymbol{}
	var nsExportStmts []ast.Stmt
	if !fileMeta.cjsStyleExports {
		nsExportStmts = append(nsExportStmts, ast.Stmt{Data: &ast.SLocal{Kind: ast.LocalConst, Decls: []ast.Decl{{
			Binding: ast.Binding{Data: &ast.BIdentifier{Ref: file.ast.ExportsRef}},
			Value:   &ast.Expr{Data: &ast.EObject{}},
		}}}})
		declaredSymbols = append(declaredSymbols, ast.DeclaredSymbol{
			Ref:        file.ast.ExportsRef,
			IsTopLevel: true,
		})
	}

	// "__export(exports, { foo: () => foo })"
	exportRef := ast.InvalidRef
	if len(properties) > 0 {
		runtimeFile := &c.files[runtime.SourceIndex]
		exportRef = runtimeFile.ast.ModuleScope.Members["__export"]
		nsExportStmts = append(nsExportStmts, ast.Stmt{Data: &ast.SExpr{Value: ast.Expr{Data: &ast.ECall{
			Target: ast.Expr{Data: &ast.EIdentifier{Ref: exportRef}},
			Args: []ast.Expr{
				{Data: &ast.EIdentifier{Ref: file.ast.ExportsRef}},
				{Data: &ast.EObject{
					Properties: properties,
				}},
			},
		}}}})

		// Make sure this file depends on the "__export" symbol
		for _, partIndex := range runtimeFile.ast.TopLevelSymbolToParts[exportRef] {
			dep := partRef{sourceIndex: runtime.SourceIndex, partIndex: partIndex}
			nsExportNonLocalDependencies = append(nsExportNonLocalDependencies, dep)
		}

		// Make sure the CommonJS closure, if there is one, includes "exports"
		file.ast.UsesExportsRef = true
	}

	// No need to generate a part if it'll be empty
	if len(nsExportStmts) > 0 {
		// Initialize the part that was allocated for us earlier. The information
		// here will be used after this during tree shaking.
		exportPart := &file.ast.Parts[fileMeta.nsExportPartIndex]
		*exportPart = ast.Part{
			Stmts:             nsExportStmts,
			LocalDependencies: make(map[uint32]bool),
			SymbolUses:        nsExportSymbolUses,
			DeclaredSymbols:   declaredSymbols,

			// This can be removed if nothing uses it. Except if we're a CommonJS
			// module, in which case it's always necessary.
			CanBeRemovedIfUnused: !fileMeta.cjsStyleExports,

			// Put the export definitions first before anything else gets evaluated
			IsNamespaceExport: true,

			// Make sure this is trimmed if unused even if tree shaking is disabled
			ForceTreeShaking: true,
		}
		fileMeta.partMeta[fileMeta.nsExportPartIndex].nonLocalDependencies = nsExportNonLocalDependencies

		// Pull in the "__export" symbol if it was used
		if exportRef != ast.InvalidRef {
			c.generateUseOfSymbolForInclude(exportPart, fileMeta, 1, exportRef, runtime.SourceIndex)
		}
	}

	if len(entryPointES6ExportItems) > 0 {
		entryPointExportStmts = append(entryPointExportStmts,
			ast.Stmt{Data: &ast.SExportClause{Items: entryPointES6ExportItems}})
	}

	// If we're an entry point, call the require function at the end of the
	// bundle right before bundle evaluation ends
	var cjsWrapStmt ast.Stmt
	if isEntryPoint && fileMeta.cjsWrap {
		switch c.options.OutputFormat {
		case config.FormatPreserve:
			// "require_foo();"
			cjsWrapStmt = ast.Stmt{Data: &ast.SExpr{Value: ast.Expr{Data: &ast.ECall{
				Target: ast.Expr{Data: &ast.EIdentifier{Ref: file.ast.WrapperRef}},
			}}}}

		case config.FormatIIFE:
			if c.options.ModuleName != "" {
				// "return require_foo();"
				cjsWrapStmt = ast.Stmt{Data: &ast.SReturn{Value: &ast.Expr{Data: &ast.ECall{
					Target: ast.Expr{Data: &ast.EIdentifier{Ref: file.ast.WrapperRef}},
				}}}}
			} else {
				// "require_foo();"
				cjsWrapStmt = ast.Stmt{Data: &ast.SExpr{Value: ast.Expr{Data: &ast.ECall{
					Target: ast.Expr{Data: &ast.EIdentifier{Ref: file.ast.WrapperRef}},
				}}}}
			}

		case config.FormatCommonJS:
			// "module.exports = require_foo();"
			cjsWrapStmt = ast.AssignStmt(
				ast.Expr{Data: &ast.EDot{
					Target: ast.Expr{Data: &ast.EIdentifier{Ref: c.unboundModuleRef}},
					Name:   "exports",
				}},
				ast.Expr{Data: &ast.ECall{
					Target: ast.Expr{Data: &ast.EIdentifier{Ref: file.ast.WrapperRef}},
				}},
			)

		case config.FormatESModule:
			// "export default require_foo();"
			cjsWrapStmt = ast.Stmt{Data: &ast.SExportDefault{Value: ast.ExprOrStmt{Expr: &ast.Expr{Data: &ast.ECall{
				Target: ast.Expr{Data: &ast.EIdentifier{Ref: file.ast.WrapperRef}},
			}}}}}
		}
	}

	if len(entryPointExportStmts) > 0 || cjsWrapStmt.Data != nil {
		// Trigger evaluation of the CommonJS wrapper
		if cjsWrapStmt.Data != nil {
			entryPointExportSymbolUses[file.ast.WrapperRef] = ast.SymbolUse{CountEstimate: 1}
			entryPointExportStmts = append(entryPointExportStmts, cjsWrapStmt)
		}

		// Add a part for this export clause
		file := &c.files[sourceIndex]
		partIndex := uint32(len(file.ast.Parts))
		fileMeta.entryPointExportPartIndex = &partIndex
		file.ast.Parts = append(file.ast.Parts, ast.Part{
			Stmts:             entryPointExportStmts,
			LocalDependencies: make(map[uint32]bool),
			SymbolUses:        entryPointExportSymbolUses,
		})
		fileMeta.partMeta = append(fileMeta.partMeta, partMeta{
			entryBits:            newBitSet(uint(len(c.entryPoints))),
			nonLocalDependencies: append([]partRef{}, entryPointExportNonLocalDependencies...),
		})
	}
}

func (c *linkerContext) matchImportsWithExportsForFile(sourceIndex uint32) {
	file := c.files[sourceIndex]

	// Sort imports for determinism. Otherwise our unit tests will randomly
	// fail sometimes when error messages are reordered.
	sortedImportRefs := make([]int, 0, len(file.ast.NamedImports))
	for ref := range file.ast.NamedImports {
		sortedImportRefs = append(sortedImportRefs, int(ref.InnerIndex))
	}
	sort.Ints(sortedImportRefs)

	// Pair imports with their matching exports
	for _, innerIndex := range sortedImportRefs {
		importRef := ast.Ref{OuterIndex: sourceIndex, InnerIndex: uint32(innerIndex)}
		tracker := importTracker{sourceIndex, importRef}
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
					c.addRangeError(source, lexer.RangeOfIdentifier(source, namedImport.AliasLoc),
						fmt.Sprintf("Detected cycle while resolving import %q", namedImport.Alias))
					break
				}
				cycleDetector, _ = c.advanceImportTracker(cycleDetector)
			}

			// Resolve the import by one step
			nextTracker, status := c.advanceImportTracker(tracker)
			switch status {
			case importCommonJS, importCommonJSWithoutExports, importExternal:
				if status == importExternal && c.options.OutputFormat.KeepES6ImportExportSyntax() {
					// Imports from external modules should not be converted to CommonJS
					// if the output format preserves the original ES6 import statements
					break
				}

				// If it's a CommonJS or external file, rewrite the import to a
				// property access. Don't do this if the namespace reference is invalid
				// though. This is the case for star imports, where the import is the
				// namespace.
				namedImport := c.files[tracker.sourceIndex].ast.NamedImports[tracker.importRef]
				if namedImport.NamespaceRef != ast.InvalidRef {
					c.symbols.Get(importRef).NamespaceAlias = &ast.NamespaceAlias{
						NamespaceRef: namedImport.NamespaceRef,
						Alias:        namedImport.Alias,
					}
				}

				// Warn about importing from a file that is known to not have any exports
				if status == importCommonJSWithoutExports {
					source := c.sources[tracker.sourceIndex]
					c.log.AddRangeWarning(&source, lexer.RangeOfIdentifier(source, namedImport.AliasLoc),
						fmt.Sprintf("Import %q will always be undefined", namedImport.Alias))
				}

			case importNoMatch:
				symbol := c.symbols.Get(tracker.importRef)
				if symbol.ImportItemStatus == ast.ImportItemGenerated {
					symbol.ImportItemStatus = ast.ImportItemMissing
				} else {
					// Report mismatched imports and exports
					source := c.sources[tracker.sourceIndex]
					namedImport := c.files[tracker.sourceIndex].ast.NamedImports[tracker.importRef]
					c.addRangeError(source, lexer.RangeOfIdentifier(source, namedImport.AliasLoc),
						fmt.Sprintf("No matching export for import %q", namedImport.Alias))
				}

			case importAmbiguous:
				source := c.sources[tracker.sourceIndex]
				namedImport := c.files[tracker.sourceIndex].ast.NamedImports[tracker.importRef]
				c.addRangeError(source, lexer.RangeOfIdentifier(source, namedImport.AliasLoc),
					fmt.Sprintf("Ambiguous import %q has multiple matching exports", namedImport.Alias))

			case importProbablyTypeScriptType:
				// Omit this import from any namespace export code we generate for
				// import star statements (i.e. "import * as ns from 'path'")
				c.fileMeta[sourceIndex].isProbablyTypeScriptType[importRef] = true

			case importFound:
				// Defer the actual binding of this import until after we generate
				// namespace export code for all files. This has to be done for all
				// import-to-export matches, not just the initial import to the final
				// export, since all imports and re-exports must be merged together
				// for correctness.
				fileMeta := &c.fileMeta[sourceIndex]
				fileMeta.importsToBind[importRef] = importToBind{
					sourceIndex: nextTracker.sourceIndex,
					ref:         nextTracker.importRef,
				}

				// If this is a re-export of another import, continue for another
				// iteration of the loop to resolve that import as well
				if _, ok := c.files[nextTracker.sourceIndex].ast.NamedImports[nextTracker.importRef]; ok {
					tracker = nextTracker
					continue
				}

			default:
				panic("Internal error")
			}

			// Stop now if we didn't explicitly "continue" above
			break
		}
	}
}

func (c *linkerContext) isCommonJSDueToExportStar(sourceIndex uint32, visited map[uint32]bool) bool {
	// Terminate the traversal now if this file is CommonJS
	fileMeta := &c.fileMeta[sourceIndex]
	if fileMeta.cjsStyleExports {
		return true
	}

	// Avoid infinite loops due to cycles in the export star graph
	if visited[sourceIndex] {
		return false
	}
	visited[sourceIndex] = true

	// Scan over the export star graph
	file := &c.files[sourceIndex]
	for _, importRecordIndex := range file.ast.ExportStarImportRecords {
		record := &file.ast.ImportRecords[importRecordIndex]

		// This file is CommonJS if the exported imports are from a file that is
		// either CommonJS directly or transitively by itself having an export star
		// from a CommonJS file.
		if record.SourceIndex == nil || (*record.SourceIndex != sourceIndex &&
			c.isCommonJSDueToExportStar(*record.SourceIndex, visited)) {
			fileMeta.cjsStyleExports = true
			return true
		}
	}

	return false
}

func (c *linkerContext) addExportsForExportStar(
	resolvedExports map[string]exportData,
	sourceIndex uint32,
	topLevelPathLoc *ast.Loc,
	visited map[uint32]bool,
) {
	// Avoid infinite loops due to cycles in the export star graph
	if visited[sourceIndex] {
		return
	}
	visited[sourceIndex] = true
	file := &c.files[sourceIndex]

	for _, importRecordIndex := range file.ast.ExportStarImportRecords {
		record := &file.ast.ImportRecords[importRecordIndex]
		if record.SourceIndex == nil {
			// This will be resolved at run time instead
			continue
		}
		otherSourceIndex := *record.SourceIndex

		// We need a location for error messages, but it must be in the top-level
		// file, not in any nested file. This will be passed to nested files.
		pathLoc := record.Loc
		if topLevelPathLoc != nil {
			pathLoc = *topLevelPathLoc
		}

		// Export stars from a CommonJS module don't work because they can't be
		// statically discovered. Just silently ignore them in this case.
		//
		// We could attempt to check whether the imported file still has ES6
		// exports even though it still uses CommonJS features. However, when
		// doing this we'd also have to rewrite any imports of these export star
		// re-exports as property accesses off of a generated require() call.
		if c.fileMeta[otherSourceIndex].cjsStyleExports {
			// This will be resolved at run time instead
			continue
		}

		// Accumulate this file's exports
		for name, ref := range c.files[otherSourceIndex].ast.NamedExports {
			// ES6 export star statements ignore exports named "default"
			if name == "default" {
				continue
			}

			existing, ok := resolvedExports[name]

			// Don't overwrite real exports, which shadow export stars
			if ok && !existing.isFromExportStar {
				continue
			}

			if !ok {
				// Initialize the re-export
				resolvedExports[name] = exportData{
					ref:              ref,
					sourceIndex:      otherSourceIndex,
					pathLoc:          &pathLoc,
					isFromExportStar: true,
				}
			} else if existing.sourceIndex != otherSourceIndex {
				// Two different re-exports colliding makes it ambiguous
				existing.isAmbiguous = true
				resolvedExports[name] = existing
			}
		}

		// Search further through this file's export stars
		c.addExportsForExportStar(resolvedExports, otherSourceIndex, &pathLoc, visited)
	}
}

type importTracker struct {
	sourceIndex uint32
	importRef   ast.Ref
}

type importStatus uint8

const (
	// The imported file has no matching export
	importNoMatch importStatus = iota

	// The imported file has a matching export
	importFound

	// The imported file is CommonJS and has unknown exports
	importCommonJS

	// The imported file was treated as CommonJS but is known to have no exports
	importCommonJSWithoutExports

	// The imported file is external and has unknown exports
	importExternal

	// There are multiple re-exports with the same name due to "export * from"
	importAmbiguous

	// This is a missing re-export in a TypeScript file, so it's probably a type
	importProbablyTypeScriptType
)

func (c *linkerContext) advanceImportTracker(tracker importTracker) (importTracker, importStatus) {
	file := &c.files[tracker.sourceIndex]
	namedImport := file.ast.NamedImports[tracker.importRef]

	// Is this an external file?
	record := &file.ast.ImportRecords[namedImport.ImportRecordIndex]
	if record.SourceIndex == nil {
		return importTracker{}, importExternal
	}

	// Is this a CommonJS file?
	otherFileMeta := &c.fileMeta[*record.SourceIndex]
	if otherFileMeta.cjsStyleExports {
		otherFile := &c.files[*record.SourceIndex]
		if !otherFile.ast.UsesCommonJSExports() && !otherFile.ast.HasES6Syntax() {
			return importTracker{}, importCommonJSWithoutExports
		}
		return importTracker{}, importCommonJS
	}

	// Match this import up with an export from the imported file
	if matchingExport, ok := otherFileMeta.resolvedExports[namedImport.Alias]; ok {
		if matchingExport.isAmbiguous {
			return importTracker{}, importAmbiguous
		}

		// Check to see if this is a re-export of another import
		return importTracker{matchingExport.sourceIndex, matchingExport.ref}, importFound
	}

	// Missing re-exports in TypeScript files are indistinguishable from types
	if file.ast.WasTypeScript && namedImport.IsExported {
		return importTracker{}, importProbablyTypeScriptType
	}

	return importTracker{}, importNoMatch
}

func (c *linkerContext) markPartsReachableFromEntryPoints() {
	// Allocate bit sets
	bitCount := uint(len(c.entryPoints))
	for _, sourceIndex := range c.reachableFiles {
		fileMeta := &c.fileMeta[sourceIndex]
		fileMeta.entryBits = newBitSet(bitCount)
		for partIndex := range fileMeta.partMeta {
			fileMeta.partMeta[partIndex].entryBits = newBitSet(bitCount)
		}

		// If this is a CommonJS file, we're going to need to generate a wrapper
		// for the CommonJS closure. That will end up looking something like this:
		//
		//   var require_foo = __commonJS((exports, module) => {
		//     ...
		//   });
		//
		// However, that generation is special-cased for various reasons and is
		// done later on. Still, we're going to need to ensure that this file
		// both depends on the "__commonJS" symbol and declares the "require_foo"
		// symbol. Instead of special-casing this during the reachablity analysis
		// below, we just append a dummy part to the end of the file with these
		// dependencies and let the general-purpose reachablity analysis take care
		// of it.
		if fileMeta.cjsWrap {
			file := &c.files[sourceIndex]
			runtimeFile := &c.files[runtime.SourceIndex]
			commonJSRef := runtimeFile.ast.NamedExports["__commonJS"]
			commonJSParts := runtimeFile.ast.TopLevelSymbolToParts[commonJSRef]

			// Generate the dummy part
			partIndex := uint32(len(file.ast.Parts))
			fileMeta.cjsWrapperPartIndex = &partIndex
			file.ast.TopLevelSymbolToParts[file.ast.WrapperRef] = []uint32{partIndex}
			file.ast.Parts = append(file.ast.Parts, ast.Part{
				LocalDependencies: make(map[uint32]bool),
				SymbolUses: map[ast.Ref]ast.SymbolUse{
					file.ast.WrapperRef: {CountEstimate: 1},
					commonJSRef:         {CountEstimate: 1},
				},
				DeclaredSymbols: []ast.DeclaredSymbol{
					{Ref: file.ast.ExportsRef, IsTopLevel: true},
					{Ref: file.ast.ModuleRef, IsTopLevel: true},
					{Ref: file.ast.WrapperRef, IsTopLevel: true},
				},
			})
			nonLocalDependencies := make([]partRef, len(commonJSParts))
			for i, partIndex := range commonJSParts {
				nonLocalDependencies[i] = partRef{sourceIndex: runtime.SourceIndex, partIndex: partIndex}
			}
			fileMeta.partMeta = append(fileMeta.partMeta, partMeta{
				entryBits:            newBitSet(uint(len(c.entryPoints))),
				nonLocalDependencies: nonLocalDependencies,
			})
			fileMeta.importsToBind[commonJSRef] = importToBind{
				ref:         commonJSRef,
				sourceIndex: runtime.SourceIndex,
			}
		}
	}

	// Each entry point marks all files reachable from itself
	for i, entryPoint := range c.entryPoints {
		c.includeFile(entryPoint, uint(i), 0)
	}

	c.handleCrossChunkAssignments()
}

// Code splitting may cause an assignment to a local variable to end up in a
// separate chunk from the variable. This is bad because that will generate
// an assignment to an import, which will fail. Make sure these parts end up
// in the same chunk in these cases.
func (c *linkerContext) handleCrossChunkAssignments() {
	if len(c.entryPoints) < 2 {
		// No need to do this if there cannot be cross-chunk assignments
		return
	}
	neverReachedEntryBits := newBitSet(uint(len(c.entryPoints)))

	for _, sourceIndex := range c.reachableFiles {
		file := &c.files[sourceIndex]
		partMeta := c.fileMeta[sourceIndex].partMeta

		// Initialize a union-find data structure to merge parts together
		unionFind := make([]uint32, len(file.ast.Parts))
		for partIndex := range file.ast.Parts {
			unionFind[partIndex] = uint32(partIndex)
		}

		// Look up a merged label
		var find func(uint32) uint32
		find = func(label uint32) uint32 {
			next := unionFind[label]
			if next != label {
				next = find(next)
				unionFind[label] = next
			}
			return next
		}

		for partIndex, part := range file.ast.Parts {
			// Ignore this part if it's dead code
			if partMeta[partIndex].entryBits.equals(neverReachedEntryBits) {
				continue
			}

			// If this part assigns to a local variable, make sure the parts for the
			// variable's declaration are in the same chunk as this part
			for ref, use := range part.SymbolUses {
				if use.IsAssigned {
					if otherParts, ok := file.ast.TopLevelSymbolToParts[ref]; ok {
						for _, otherPartIndex := range otherParts {
							// Union the two labels together
							a := find(uint32(partIndex))
							b := find(otherPartIndex)
							unionFind[a] = b
							partMeta[b].entryBits.bitwiseOrWith(partMeta[a].entryBits)
						}
					}
				}
			}
		}

		// Update the entry bits of parts that were merged
		for partIndex := range file.ast.Parts {
			label := find(uint32(partIndex))
			if label != uint32(partIndex) {
				partMeta[partIndex].entryBits.copyFrom(partMeta[label].entryBits)
			}
		}
	}
}

func (c *linkerContext) accumulateSymbolCount(ref ast.Ref, count uint32) {
	ref = ast.FollowSymbols(c.symbols, ref)
	c.symbols.Get(ref).UseCountEstimate += count
}

func (c *linkerContext) includeFile(sourceIndex uint32, entryPointBit uint, distanceFromEntryPoint uint32) {
	fileMeta := &c.fileMeta[sourceIndex]

	// Track the minimum distance to an entry point
	if distanceFromEntryPoint < fileMeta.distanceFromEntryPoint {
		fileMeta.distanceFromEntryPoint = distanceFromEntryPoint
	}
	distanceFromEntryPoint++

	// Don't mark this file more than once
	if fileMeta.entryBits.hasBit(entryPointBit) {
		return
	}
	fileMeta.entryBits.setBit(entryPointBit)

	// Accumulate symbol usage counts
	file := &c.files[sourceIndex]
	if file.ast.UsesExportsRef {
		c.accumulateSymbolCount(file.ast.ExportsRef, 1)
	}
	if file.ast.UsesModuleRef {
		c.accumulateSymbolCount(file.ast.ModuleRef, 1)
	}

	for partIndex, part := range file.ast.Parts {
		canBeRemovedIfUnused := part.CanBeRemovedIfUnused

		// Don't include the entry point part if we're not the entry point
		if fileMeta.entryPointExportPartIndex != nil && uint32(partIndex) == *fileMeta.entryPointExportPartIndex &&
			sourceIndex != c.entryPoints[entryPointBit] {
			continue
		}

		// Also include any statement-level imports
		for _, importRecordIndex := range part.ImportRecordIndices {
			record := &file.ast.ImportRecords[importRecordIndex]
			if record.Kind != ast.ImportStmt {
				continue
			}

			if record.SourceIndex != nil {
				otherSourceIndex := *record.SourceIndex

				// Don't include this module for its side effects if it can be
				// considered to have no side effects
				if c.files[otherSourceIndex].ignoreIfUnused {
					continue
				}

				// Otherwise, include this module for its side effects
				c.includeFile(otherSourceIndex, entryPointBit, distanceFromEntryPoint)
			}

			// If we get here then the import was included for its side effects, so
			// we must also keep this part
			canBeRemovedIfUnused = false
		}

		// Include all parts in this file with side effects, or just include
		// everything if tree-shaking is disabled. Note that we still want to
		// perform tree-shaking on the runtime even if tree-shaking is disabled.
		if !canBeRemovedIfUnused || (!part.ForceTreeShaking && !c.options.IsBundling && sourceIndex != runtime.SourceIndex) {
			c.includePart(sourceIndex, uint32(partIndex), entryPointBit, distanceFromEntryPoint)
		}
	}

	// If this is an entry point, include all exports
	if fileMeta.entryPointStatus != entryPointNone {
		for _, alias := range fileMeta.sortedAndFilteredExportAliases {
			export := fileMeta.resolvedExports[alias]
			targetSourceIndex := export.sourceIndex
			targetRef := export.ref

			// If this is an import, then target what the import points to
			if importToBind, ok := c.fileMeta[targetSourceIndex].importsToBind[targetRef]; ok {
				targetSourceIndex = importToBind.sourceIndex
				targetRef = importToBind.ref
			}

			// Pull in all declarations of this symbol
			for _, partIndex := range c.files[targetSourceIndex].ast.TopLevelSymbolToParts[targetRef] {
				c.includePart(targetSourceIndex, partIndex, entryPointBit, distanceFromEntryPoint)
			}
		}
	}
}

func (c *linkerContext) includePartsForRuntimeSymbol(
	part *ast.Part, fileMeta *fileMeta, useCount uint32,
	name string, entryPointBit uint, distanceFromEntryPoint uint32,
) {
	if useCount > 0 {
		file := &c.files[runtime.SourceIndex]
		ref := file.ast.NamedExports[name]

		// Depend on the symbol from the runtime
		c.generateUseOfSymbolForInclude(part, fileMeta, useCount, ref, runtime.SourceIndex)

		// Since this part was included, also include the parts from the runtime
		// that declare this symbol
		for _, partIndex := range file.ast.TopLevelSymbolToParts[ref] {
			c.includePart(runtime.SourceIndex, partIndex, entryPointBit, distanceFromEntryPoint)
		}
	}
}

func (c *linkerContext) generateUseOfSymbolForInclude(
	part *ast.Part, fileMeta *fileMeta, useCount uint32,
	ref ast.Ref, otherSourceIndex uint32,
) {
	use := part.SymbolUses[ref]
	use.CountEstimate += useCount
	part.SymbolUses[ref] = use
	fileMeta.importsToBind[ref] = importToBind{
		sourceIndex: otherSourceIndex,
		ref:         ref,
	}
}

func (c *linkerContext) isExternalDynamicImport(record *ast.ImportRecord) bool {
	return record.Kind == ast.ImportDynamic && c.fileMeta[*record.SourceIndex].entryPointStatus == entryPointDynamicImport
}

func (c *linkerContext) includePart(sourceIndex uint32, partIndex uint32, entryPointBit uint, distanceFromEntryPoint uint32) {
	partMeta := &c.fileMeta[sourceIndex].partMeta[partIndex]

	// Don't mark this part more than once
	if partMeta.entryBits.hasBit(entryPointBit) {
		return
	}
	partMeta.entryBits.setBit(entryPointBit)

	file := &c.files[sourceIndex]
	part := &file.ast.Parts[partIndex]
	fileMeta := &c.fileMeta[sourceIndex]

	// Include the file containing this part
	c.includeFile(sourceIndex, entryPointBit, distanceFromEntryPoint)

	// Also include any local dependencies
	for otherPartIndex := range part.LocalDependencies {
		c.includePart(sourceIndex, otherPartIndex, entryPointBit, distanceFromEntryPoint)
	}

	// Also include any non-local dependencies
	for _, nonLocalDependency := range partMeta.nonLocalDependencies {
		c.includePart(nonLocalDependency.sourceIndex, nonLocalDependency.partIndex, entryPointBit, distanceFromEntryPoint)
	}

	// Also include any require() imports
	toModuleUses := uint32(0)
	for _, importRecordIndex := range part.ImportRecordIndices {
		record := &file.ast.ImportRecords[importRecordIndex]

		// Don't follow external imports (this includes import() expressions)
		if record.SourceIndex == nil || c.isExternalDynamicImport(record) {
			// This is an external import, so it needs the "__toModule" wrapper as
			// long as it's not a bare "require()"
			if record.Kind != ast.ImportRequire && !c.options.OutputFormat.KeepES6ImportExportSyntax() {
				record.WrapWithToModule = true
				toModuleUses++
			}
			continue
		}

		otherSourceIndex := *record.SourceIndex
		if record.Kind == ast.ImportStmt && !c.fileMeta[otherSourceIndex].cjsStyleExports {
			// Skip this since it's not a require() import
			continue
		}

		// This is a require() import
		c.includeFile(otherSourceIndex, entryPointBit, distanceFromEntryPoint)

		// Depend on the automatically-generated require wrapper symbol
		wrapperRef := c.files[otherSourceIndex].ast.WrapperRef
		c.generateUseOfSymbolForInclude(part, fileMeta, 1, wrapperRef, otherSourceIndex)

		// This is an ES6 import of a CommonJS module, so it needs the
		// "__toModule" wrapper as long as it's not a bare "require()"
		if record.Kind != ast.ImportRequire {
			record.WrapWithToModule = true
			toModuleUses++
		}
	}

	// If there's an ES6 import of a non-ES6 module, then we're going to need the
	// "__toModule" symbol from the runtime to wrap the result of "require()"
	c.includePartsForRuntimeSymbol(part, fileMeta, toModuleUses, "__toModule", entryPointBit, distanceFromEntryPoint)

	// If there's an ES6 export star statement of a non-ES6 module, then we're
	// going to need the "__exportStar" symbol from the runtime
	exportStarUses := uint32(0)
	for _, importRecordIndex := range file.ast.ExportStarImportRecords {
		record := &file.ast.ImportRecords[importRecordIndex]

		// Is this export star evaluated at run time?
		if record.SourceIndex == nil || (*record.SourceIndex != sourceIndex && c.fileMeta[*record.SourceIndex].cjsStyleExports) {
			record.IsExportStarRunTimeEval = true
			file.ast.UsesExportsRef = true
			exportStarUses++
		}
	}
	c.includePartsForRuntimeSymbol(part, fileMeta, exportStarUses, "__exportStar", entryPointBit, distanceFromEntryPoint)

	// Accumulate symbol usage counts. Do this last to also include
	// automatically-generated usages from the code above.
	for ref, use := range part.SymbolUses {
		c.accumulateSymbolCount(ref, use.CountEstimate)
	}
	for _, declared := range part.DeclaredSymbols {
		// Make sure to also count the declaration in addition to the uses
		c.accumulateSymbolCount(declared.Ref, 1)
	}
}

func (c *linkerContext) computeChunks() []chunkMeta {
	chunks := make(map[string]chunkMeta)
	neverReachedKey := string(newBitSet(uint(len(c.entryPoints))).entries)

	// Compute entry point names
	for i, entryPoint := range c.entryPoints {
		var entryPointName string
		if c.options.AbsOutputFile != "" && c.fileMeta[entryPoint].entryPointStatus == entryPointUserSpecified {
			entryPointName = c.fs.Base(c.options.AbsOutputFile)
		} else {
			name := c.fs.Base(c.sources[entryPoint].KeyPath.Text)
			entryPointName = c.stripKnownFileExtension(name) + ".js"
		}
		c.fileMeta[entryPoint].entryPointName = entryPointName

		// Create a chunk for the entry point here to ensure that the chunk is
		// always generated even if the resulting file is empty
		entryBits := newBitSet(uint(len(c.entryPoints)))
		entryBits.setBit(uint(i))
		chunks[string(entryBits.entries)] = chunkMeta{
			entryBits:             entryBits,
			isEntryPoint:          true,
			sourceIndex:           entryPoint,
			entryPointBit:         uint(i),
			name:                  entryPointName,
			filesWithPartsInChunk: make(map[uint32]bool),
		}
	}

	// Figure out which files are in which chunk
	for _, sourceIndex := range c.reachableFiles {
		for _, partMeta := range c.fileMeta[sourceIndex].partMeta {
			key := string(partMeta.entryBits.entries)
			if key == neverReachedKey {
				// Ignore this part if it was never reached
				continue
			}
			chunk, ok := chunks[key]
			if !ok {
				// Initialize the chunk for the first time
				isMultiPart := false
				for i, entryPoint := range c.entryPoints {
					if partMeta.entryBits.hasBit(uint(i)) {
						if chunk.name != "" {
							chunk.name = c.stripKnownFileExtension(chunk.name) + "_"
							isMultiPart = true
						}
						chunk.name += c.fileMeta[entryPoint].entryPointName
					}
				}

				// Avoid really long automatically-generated chunk names
				if isMultiPart {
					bytes := []byte(chunk.name)
					hashBytes := sha1.Sum(bytes)
					hash := base64.URLEncoding.EncodeToString(hashBytes[:])[:8]
					chunk.name = "chunk." + hash + ".js"
				}

				chunk.entryBits = partMeta.entryBits
				chunk.filesWithPartsInChunk = make(map[uint32]bool)
				chunks[key] = chunk
			}
			chunk.filesWithPartsInChunk[uint32(sourceIndex)] = true
		}
	}

	// Sort the chunks for determinism. This mostly doesn't matter because each
	// chunk is a separate file, but it matters for error messages in tests since
	// tests stop on the first output mismatch.
	sortedKeys := make([]string, 0, len(chunks))
	for key := range chunks {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)
	sortedChunks := make([]chunkMeta, len(chunks))
	for i, key := range sortedKeys {
		sortedChunks[i] = chunks[key]
	}
	return sortedChunks
}

func (c *linkerContext) stripKnownFileExtension(name string) string {
	for ext := range c.options.ExtensionToLoader {
		if strings.HasSuffix(name, ext) {
			return name[:len(name)-len(ext)]
		}
	}
	return name
}

type chunkOrder struct {
	sourceIndex uint32
	distance    uint32
	path        ast.Path
}

// This type is just so we can use Go's native sort function
type chunkOrderArray []chunkOrder

func (a chunkOrderArray) Len() int          { return len(a) }
func (a chunkOrderArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a chunkOrderArray) Less(i int, j int) bool {
	return a[i].distance < a[j].distance || (a[i].distance == a[j].distance && a[i].path.ComesBeforeInSortedOrder(a[j].path))
}

func (c *linkerContext) chunkFileOrder(chunk chunkMeta) []uint32 {
	sorted := make(chunkOrderArray, 0, len(chunk.filesWithPartsInChunk))

	// Attach information to the files for use with sorting
	for sourceIndex := range chunk.filesWithPartsInChunk {
		sorted = append(sorted, chunkOrder{
			sourceIndex: sourceIndex,
			distance:    c.fileMeta[sourceIndex].distanceFromEntryPoint,
			path:        c.sources[sourceIndex].KeyPath,
		})
	}

	// Sort so files closest to an entry point come first. If two files are
	// equidistant to an entry point, then break the tie by sorting on the
	// absolute path.
	sort.Sort(sorted)

	visited := make(map[uint32]bool)
	prefixOrder := []uint32{}
	suffixOrder := []uint32{}

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
		isFileInThisChunk := chunk.entryBits.equals(fileMeta.entryBits)

		for partIndex, part := range file.ast.Parts {
			isPartInThisChunk := chunk.entryBits.equals(fileMeta.partMeta[partIndex].entryBits)
			if isPartInThisChunk {
				isFileInThisChunk = true
			}

			// Also traverse any files imported by this part
			for _, importRecordIndex := range part.ImportRecordIndices {
				record := &file.ast.ImportRecords[importRecordIndex]
				if record.SourceIndex != nil && (record.Kind == ast.ImportStmt || isPartInThisChunk) {
					if c.isExternalDynamicImport(record) {
						// Don't follow import() dependencies
						continue
					}
					visit(*record.SourceIndex)
				}
			}
		}

		// Always put all CommonJS wrappers before any other non-runtime code to
		// deal with cycles in the module graph. CommonJS wrapper declarations
		// aren't hoisted so starting to evaluate them before they are all declared
		// could end up with one being used before being declared. These wrappers
		// don't have side effects so an easy fix is to just declare them all
		// before starting to evaluate them.
		if isFileInThisChunk {
			if sourceIndex == runtime.SourceIndex || fileMeta.cjsWrap {
				prefixOrder = append(prefixOrder, sourceIndex)
			} else {
				suffixOrder = append(suffixOrder, sourceIndex)
			}
		}
	}

	// Always put the runtime code first before anything else
	visit(runtime.SourceIndex)
	for _, data := range sorted {
		visit(data.sourceIndex)
	}
	return append(prefixOrder, suffixOrder...)
}

func (c *linkerContext) shouldRemoveImportExportStmt(
	sourceIndex uint32,
	stmtList *stmtList,
	partStmts []ast.Stmt,
	loc ast.Loc,
	namespaceRef ast.Ref,
	importRecordIndex uint32,
) bool {
	// Is this an import from another module inside this bundle?
	record := &c.files[sourceIndex].ast.ImportRecords[importRecordIndex]
	if record.SourceIndex != nil {
		if !c.fileMeta[*record.SourceIndex].cjsStyleExports {
			// Remove the statement entirely if this is not a CommonJS module
			return true
		}
	} else if c.options.OutputFormat.KeepES6ImportExportSyntax() {
		// If this is an external module and the output format allows ES6
		// import/export syntax, then just keep the statement
		return false
	}

	// We don't need a call to "require()" if this is a self-import inside a
	// CommonJS-style module, since we can just reference the exports directly.
	if c.fileMeta[sourceIndex].cjsStyleExports &&
		ast.FollowSymbols(c.symbols, namespaceRef) == c.files[sourceIndex].ast.ExportsRef {
		return true
	}

	// Replace the statement with a call to "require()"
	stmtList.prefixStmts = append(stmtList.prefixStmts, ast.Stmt{
		Loc: loc,
		Data: &ast.SLocal{Kind: ast.LocalConst, Decls: []ast.Decl{{
			Binding: ast.Binding{Loc: loc, Data: &ast.BIdentifier{Ref: namespaceRef}},
			Value:   &ast.Expr{Loc: record.Loc, Data: &ast.ERequire{ImportRecordIndex: importRecordIndex}},
		}}},
	})
	return true
}

func (c *linkerContext) convertStmtsForChunk(sourceIndex uint32, stmtList *stmtList, partStmts []ast.Stmt) {
	shouldStripExports := c.options.IsBundling || sourceIndex == runtime.SourceIndex
	shouldExtractES6StmtsForCJSWrap := c.fileMeta[sourceIndex].cjsWrap

	for _, stmt := range partStmts {
		switch s := stmt.Data.(type) {
		case *ast.SImport:
			// "import * as ns from 'path'"
			// "import {foo} from 'path'"
			if c.shouldRemoveImportExportStmt(sourceIndex, stmtList, partStmts, stmt.Loc, s.NamespaceRef, s.ImportRecordIndex) {
				continue
			}

			// Make sure these don't end up in a CommonJS wrapper
			if shouldExtractES6StmtsForCJSWrap {
				stmtList.es6StmtsForCJSWrap = append(stmtList.es6StmtsForCJSWrap, stmt)
				continue
			}

		case *ast.SExportStar:
			if s.Alias == nil {
				// "export * from 'path'"
				if shouldStripExports {
					record := &c.files[sourceIndex].ast.ImportRecords[s.ImportRecordIndex]

					// Is this export star evaluated at run time?
					if record.SourceIndex == nil && c.options.OutputFormat.KeepES6ImportExportSyntax() {
						// Turn this statement into "import * as ns from 'path'"
						stmt.Data = &ast.SImport{
							NamespaceRef:      s.NamespaceRef,
							StarNameLoc:       &stmt.Loc,
							ImportRecordIndex: s.ImportRecordIndex,
						}

						// Prefix this module with "__exportStar(exports, ns)"
						exportStarRef := c.files[runtime.SourceIndex].ast.ModuleScope.Members["__exportStar"]
						stmtList.prefixStmts = append(stmtList.prefixStmts, ast.Stmt{
							Loc: stmt.Loc,
							Data: &ast.SExpr{Value: ast.Expr{Loc: stmt.Loc, Data: &ast.ECall{
								Target: ast.Expr{Loc: stmt.Loc, Data: &ast.EIdentifier{Ref: exportStarRef}},
								Args: []ast.Expr{
									{Loc: stmt.Loc, Data: &ast.EIdentifier{Ref: c.files[sourceIndex].ast.ExportsRef}},
									{Loc: stmt.Loc, Data: &ast.EIdentifier{Ref: s.NamespaceRef}},
								},
							}}},
						})

						// Make sure these don't end up in a CommonJS wrapper
						if shouldExtractES6StmtsForCJSWrap {
							stmtList.es6StmtsForCJSWrap = append(stmtList.es6StmtsForCJSWrap, stmt)
							continue
						}
					} else {
						if record.IsExportStarRunTimeEval {
							// Prefix this module with "__exportStar(exports, require(path))"
							exportStarRef := c.files[runtime.SourceIndex].ast.ModuleScope.Members["__exportStar"]
							stmtList.prefixStmts = append(stmtList.prefixStmts, ast.Stmt{
								Loc: stmt.Loc,
								Data: &ast.SExpr{Value: ast.Expr{Loc: stmt.Loc, Data: &ast.ECall{
									Target: ast.Expr{Loc: stmt.Loc, Data: &ast.EIdentifier{Ref: exportStarRef}},
									Args: []ast.Expr{
										{Loc: stmt.Loc, Data: &ast.EIdentifier{Ref: c.files[sourceIndex].ast.ExportsRef}},
										{Loc: record.Loc, Data: &ast.ERequire{ImportRecordIndex: s.ImportRecordIndex}},
									},
								}}},
							})
						}

						// Remove the export star statement
						continue
					}
				}
			} else {
				// "export * as ns from 'path'"
				if c.shouldRemoveImportExportStmt(sourceIndex, stmtList, partStmts, stmt.Loc, s.NamespaceRef, s.ImportRecordIndex) {
					continue
				}

				if shouldStripExports {
					// Turn this statement into "import * as ns from 'path'"
					stmt.Data = &ast.SImport{
						NamespaceRef:      s.NamespaceRef,
						StarNameLoc:       &s.Alias.Loc,
						ImportRecordIndex: s.ImportRecordIndex,
					}
				}

				// Make sure these don't end up in a CommonJS wrapper
				if shouldExtractES6StmtsForCJSWrap {
					stmtList.es6StmtsForCJSWrap = append(stmtList.es6StmtsForCJSWrap, stmt)
					continue
				}
			}

		case *ast.SExportFrom:
			// "export {foo} from 'path'"
			if c.shouldRemoveImportExportStmt(sourceIndex, stmtList, partStmts, stmt.Loc, s.NamespaceRef, s.ImportRecordIndex) {
				continue
			}

			if shouldStripExports {
				// Turn this statement into "import {foo} from 'path'"
				for i, item := range s.Items {
					s.Items[i].Alias = item.OriginalName
				}
				stmt.Data = &ast.SImport{
					NamespaceRef:      s.NamespaceRef,
					Items:             &s.Items,
					ImportRecordIndex: s.ImportRecordIndex,
					IsSingleLine:      s.IsSingleLine,
				}
			}

			// Make sure these don't end up in a CommonJS wrapper
			if shouldExtractES6StmtsForCJSWrap {
				stmtList.es6StmtsForCJSWrap = append(stmtList.es6StmtsForCJSWrap, stmt)
				continue
			}

		case *ast.SExportClause:
			if shouldStripExports {
				// Remove export statements entirely
				continue
			}

			// Make sure these don't end up in a CommonJS wrapper
			if shouldExtractES6StmtsForCJSWrap {
				stmtList.es6StmtsForCJSWrap = append(stmtList.es6StmtsForCJSWrap, stmt)
				continue
			}

		case *ast.SFunction:
			// Strip the "export" keyword while bundling
			if shouldStripExports && s.IsExport {
				// Be careful to not modify the original statement
				clone := *s
				clone.IsExport = false
				stmt.Data = &clone
			}

		case *ast.SClass:
			// Strip the "export" keyword while bundling
			if shouldStripExports && s.IsExport {
				// Be careful to not modify the original statement
				clone := *s
				clone.IsExport = false
				stmt.Data = &clone
			}

		case *ast.SLocal:
			// Strip the "export" keyword while bundling
			if shouldStripExports && s.IsExport {
				// Be careful to not modify the original statement
				clone := *s
				clone.IsExport = false
				stmt.Data = &clone
			}

		case *ast.SExportDefault:
			// If we're bundling, convert "export default" into a normal declaration
			if shouldStripExports {
				if s.Value.Expr != nil {
					// "export default foo;" => "var default = foo;"
					stmt = ast.Stmt{Loc: stmt.Loc, Data: &ast.SLocal{Decls: []ast.Decl{
						{Binding: ast.Binding{Loc: s.DefaultName.Loc, Data: &ast.BIdentifier{Ref: s.DefaultName.Ref}}, Value: s.Value.Expr},
					}}}
				} else {
					switch s2 := s.Value.Stmt.Data.(type) {
					case *ast.SFunction:
						// "export default function() {}" => "function default() {}"
						// "export default function foo() {}" => "function foo() {}"

						// Be careful to not modify the original statement
						s2 = &ast.SFunction{Fn: s2.Fn}
						s2.Fn.Name = &s.DefaultName

						stmt = ast.Stmt{Loc: s.Value.Stmt.Loc, Data: s2}

					case *ast.SClass:
						// "export default class {}" => "class default {}"
						// "export default class Foo {}" => "class Foo {}"

						// Be careful to not modify the original statement
						s2 = &ast.SClass{Class: s2.Class}
						s2.Class.Name = &s.DefaultName

						stmt = ast.Stmt{Loc: s.Value.Stmt.Loc, Data: s2}

					default:
						panic("Internal error")
					}
				}
			}
		}

		stmtList.normalStmts = append(stmtList.normalStmts, stmt)
	}
}

// "var a = 1; var b = 2;" => "var a = 1, b = 2;"
func mergeAdjacentLocalStmts(stmts []ast.Stmt) []ast.Stmt {
	if len(stmts) == 0 {
		return stmts
	}

	didMergeWithPreviousLocal := false
	end := 1

	for _, stmt := range stmts[1:] {
		// Try to merge with the previous variable statement
		if after, ok := stmt.Data.(*ast.SLocal); ok {
			if before, ok := stmts[end-1].Data.(*ast.SLocal); ok {
				// It must be the same kind of variable statement (i.e. let/var/const)
				if before.Kind == after.Kind && before.IsExport == after.IsExport {
					if didMergeWithPreviousLocal {
						// Avoid O(n^2) behavior for repeated variable declarations
						before.Decls = append(before.Decls, after.Decls...)
					} else {
						// Be careful to not modify the original statement
						didMergeWithPreviousLocal = true
						clone := *before
						clone.Decls = make([]ast.Decl, 0, len(before.Decls)+len(after.Decls))
						clone.Decls = append(clone.Decls, before.Decls...)
						clone.Decls = append(clone.Decls, after.Decls...)
						stmts[end-1].Data = &clone
					}
					continue
				}
			}
		}

		// Otherwise, append a normal statement
		didMergeWithPreviousLocal = false
		stmts[end] = stmt
		end++
	}

	return stmts[:end]
}

type stmtList struct {
	// These statements come first, and can be inside the CommonJS wrapper
	prefixStmts []ast.Stmt

	// These statements come last, and can be inside the CommonJS wrapper
	normalStmts []ast.Stmt

	// Order doesn't matter for these statements, but they must be outside any
	// CommonJS wrapper since they are top-level ES6 import/export statements
	es6StmtsForCJSWrap []ast.Stmt

	// These statements are for an entry point and come at the end of the chunk
	entryPointTail []ast.Stmt
}

func (c *linkerContext) generateCodeForFileInChunk(
	waitGroup *sync.WaitGroup,
	sourceIndex uint32,
	entryBits bitSet,
	commonJSRef ast.Ref,
	toModuleRef ast.Ref,
	result *compileResult,
) {
	file := &c.files[sourceIndex]
	fileMeta := &c.fileMeta[sourceIndex]
	needsWrapper := false
	stmtList := stmtList{}

	// Make sure the generated call to "__export(exports, ...)" comes first
	// before anything else.
	if entryBits.equals(fileMeta.partMeta[fileMeta.nsExportPartIndex].entryBits) {
		c.convertStmtsForChunk(sourceIndex, &stmtList, file.ast.Parts[fileMeta.nsExportPartIndex].Stmts)

		// Move everything to the prefix list
		stmtList.prefixStmts = append(stmtList.prefixStmts, stmtList.normalStmts...)
		stmtList.normalStmts = nil
	}

	// Add all other parts in this chunk
	for partIndex, part := range file.ast.Parts {
		if !entryBits.equals(fileMeta.partMeta[partIndex].entryBits) {
			// Skip the part if it's not in this chunk
			continue
		}

		if uint32(partIndex) == fileMeta.nsExportPartIndex {
			// Skip the generated call to "__export()" that was extracted above
			continue
		}

		// Mark if we hit the dummy part representing the CommonJS wrapper
		if fileMeta.cjsWrapperPartIndex != nil && uint32(partIndex) == *fileMeta.cjsWrapperPartIndex {
			needsWrapper = true
			continue
		}

		// Emit export statements in the entry point part verbatim
		if fileMeta.entryPointExportPartIndex != nil && uint32(partIndex) == *fileMeta.entryPointExportPartIndex {
			stmtList.entryPointTail = append(stmtList.entryPointTail, part.Stmts...)
			continue
		}

		c.convertStmtsForChunk(sourceIndex, &stmtList, part.Stmts)
	}

	// Hoist all import statements before any normal statements. ES6 imports
	// are different than CommonJS imports. All modules imported via ES6 import
	// statements are evaluated before the module doing the importing is
	// evaluated (well, except for cyclic import scenarios). We need to preserve
	// these semantics even when modules imported via ES6 import statements end
	// up being CommonJS modules.
	stmts := stmtList.normalStmts
	if len(stmtList.prefixStmts) > 0 {
		stmts = append(stmtList.prefixStmts, stmts...)
	}
	if c.options.MangleSyntax {
		stmts = mergeAdjacentLocalStmts(stmts)
	}

	// Optionally wrap all statements in a closure for CommonJS
	if needsWrapper {
		// Only include the arguments that are actually used
		args := []ast.Arg{}
		if file.ast.UsesExportsRef || file.ast.UsesModuleRef {
			args = append(args, ast.Arg{Binding: ast.Binding{Data: &ast.BIdentifier{Ref: file.ast.ExportsRef}}})
			if file.ast.UsesModuleRef {
				args = append(args, ast.Arg{Binding: ast.Binding{Data: &ast.BIdentifier{Ref: file.ast.ModuleRef}}})
			}
		}

		// "__commonJS((exports, module) => { ... })"
		value := ast.Expr{Data: &ast.ECall{
			Target: ast.Expr{Data: &ast.EIdentifier{Ref: commonJSRef}},
			Args:   []ast.Expr{{Data: &ast.EArrow{Args: args, Body: ast.FnBody{Stmts: stmts}}}},
		}}

		// "var require_foo = __commonJS((exports, module) => { ... });"
		stmts = append(stmtList.es6StmtsForCJSWrap, ast.Stmt{Data: &ast.SLocal{
			Decls: []ast.Decl{{
				Binding: ast.Binding{Data: &ast.BIdentifier{Ref: file.ast.WrapperRef}},
				Value:   &value,
			}},
		}})
	}

	// Only generate a source map if needed
	sourceMapContents := &c.sources[sourceIndex].Contents
	if c.options.SourceMap == config.SourceMapNone {
		sourceMapContents = nil
	}

	// Indent the file if everything is wrapped in an IIFE
	indent := 0
	if c.options.OutputFormat == config.FormatIIFE {
		indent++
	}

	// Convert the AST to JavaScript code
	printOptions := printer.PrintOptions{
		Indent:              indent,
		OutputFormat:        c.options.OutputFormat,
		RemoveWhitespace:    c.options.RemoveWhitespace,
		ToModuleRef:         toModuleRef,
		SourceMapContents:   sourceMapContents,
		ExtractComments:     c.options.IsBundling && c.options.RemoveWhitespace,
		UnsupportedFeatures: c.options.UnsupportedFeatures,
	}
	tree := file.ast
	tree.Parts = []ast.Part{{Stmts: stmts}}
	*result = compileResult{
		PrintResult: printer.Print(tree, printOptions),
		sourceIndex: sourceIndex,
	}

	// Write this separately as the entry point tail so it can be split off
	// from the main entry point code. This is sometimes required to deal with
	// CommonJS import cycles.
	if len(stmtList.entryPointTail) > 0 {
		tree := file.ast
		tree.Parts = []ast.Part{{Stmts: stmtList.entryPointTail}}
		entryPointTail := printer.Print(tree, printOptions)
		result.entryPointTail = &entryPointTail
	}

	// Also quote the source for the source map while we're running in parallel
	if c.options.SourceMap != config.SourceMapNone {
		result.quotedSource = printer.QuoteForJSON(c.sources[sourceIndex].Contents)
	}

	waitGroup.Done()
}

func (c *linkerContext) generateChunk(chunk chunkMeta) (results []OutputFile) {
	filesInChunkInOrder := c.chunkFileOrder(chunk)
	compileResults := make([]compileResult, 0, len(filesInChunkInOrder))
	runtimeMembers := c.files[runtime.SourceIndex].ast.ModuleScope.Members
	commonJSRef := ast.FollowSymbols(c.symbols, runtimeMembers["__commonJS"])
	toModuleRef := ast.FollowSymbols(c.symbols, runtimeMembers["__toModule"])

	// Generate JavaScript for each file in parallel
	waitGroup := sync.WaitGroup{}
	for _, sourceIndex := range filesInChunkInOrder {
		// Skip the runtime in test output
		if sourceIndex == runtime.SourceIndex && c.options.OmitRuntimeForTests {
			continue
		}

		// Each file may optionally contain an additional file to be copied to the
		// output directory. This is used by the "file" loader.
		if additionalFile := c.files[sourceIndex].additionalFile; additionalFile != nil {
			results = append(results, *additionalFile)
		}

		// Create a goroutine for this file
		compileResults = append(compileResults, compileResult{})
		compileResult := &compileResults[len(compileResults)-1]
		waitGroup.Add(1)
		go c.generateCodeForFileInChunk(
			&waitGroup,
			sourceIndex,
			chunk.entryBits,
			commonJSRef,
			toModuleRef,
			compileResult,
		)
	}

	// Also generate the cross-chunk binding code
	var crossChunkPrefix []byte
	var crossChunkSuffix []byte
	{
		// Indent the file if everything is wrapped in an IIFE
		indent := 0
		if c.options.OutputFormat == config.FormatIIFE {
			indent++
		}
		printOptions := printer.PrintOptions{
			Indent:           indent,
			OutputFormat:     c.options.OutputFormat,
			RemoveWhitespace: c.options.RemoveWhitespace,
		}
		crossChunkPrefix = printer.Print(ast.AST{
			ImportRecords: chunk.crossChunkImportRecords,
			Parts:         []ast.Part{{Stmts: chunk.crossChunkPrefixStmts}},
			Symbols:       c.symbols,
		}, printOptions).JS
		crossChunkSuffix = printer.Print(ast.AST{
			Parts:   []ast.Part{{Stmts: chunk.crossChunkSuffixStmts}},
			Symbols: c.symbols,
		}, printOptions).JS
	}
	waitGroup.Wait()

	j := printer.Joiner{}
	prevOffset := lineColumnOffset{}

	// Optionally strip whitespace
	indent := ""
	space := " "
	newline := "\n"
	if c.options.RemoveWhitespace {
		space = ""
		newline = ""
	}
	newlineBeforeComment := false

	if chunk.isEntryPoint {
		file := &c.files[chunk.sourceIndex]

		// Start with the hashbang if there is one
		if file.ast.Hashbang != "" {
			hashbang := file.ast.Hashbang + "\n"
			prevOffset.advance(hashbang)
			j.AddString(hashbang)
			newlineBeforeComment = true
		}

		// Add the top-level directive if present
		if file.ast.Directive != "" {
			quoted := printer.Quote(file.ast.Directive) + ";" + newline
			prevOffset.advance(quoted)
			j.AddString(quoted)
			newlineBeforeComment = true
		}
	}

	// Optionally wrap with an IIFE
	if c.options.OutputFormat == config.FormatIIFE {
		indent = "  "
		text := "(()" + space + "=>" + space + "{" + newline
		if c.options.ModuleName != "" {
			text = "var " + c.options.ModuleName + space + "=" + space + text
		}
		prevOffset.advance(text)
		j.AddString(text)
		newlineBeforeComment = false
	}

	// Put the cross-chunk prefix inside the IIFE
	if len(crossChunkPrefix) > 0 {
		newlineBeforeComment = true
		j.AddBytes(crossChunkPrefix)
	}

	// Start the metadata
	jMeta := printer.Joiner{}
	isFirstMeta := true
	if c.options.AbsMetadataFile != "" {
		jMeta.AddString("{\n      \"inputs\": {")
	}

	// Concatenate the generated JavaScript chunks together
	var compileResultsForSourceMap []compileResult
	var entryPointTail *printer.PrintResult
	var commentList []string
	commentSet := make(map[string]bool)
	for _, compileResult := range compileResults {
		isRuntime := compileResult.sourceIndex == runtime.SourceIndex
		for text := range compileResult.ExtractedComments {
			if !commentSet[text] {
				commentSet[text] = true
				commentList = append(commentList, text)
			}
		}

		// If this is the entry point, it may have some extra code to stick at the
		// end of the chunk after all modules have evaluated
		if compileResult.entryPointTail != nil {
			entryPointTail = compileResult.entryPointTail
		}

		// Don't add a file name comment for the runtime
		if c.options.IsBundling && !c.options.RemoveWhitespace && !isRuntime {
			if newlineBeforeComment {
				prevOffset.advance("\n")
				j.AddString("\n")
			}

			text := fmt.Sprintf("%s// %s\n", indent, c.sources[compileResult.sourceIndex].PrettyPath)
			prevOffset.advance(text)
			j.AddString(text)
		}

		// Omit the trailing semicolon when minifying the last file in IIFE mode
		if !isRuntime || len(compileResult.JS) > 0 {
			newlineBeforeComment = true
		}

		// Don't include the runtime in source maps
		if isRuntime {
			prevOffset.advance(string(compileResult.JS))
			j.AddBytes(compileResult.JS)
		} else {
			// Save the offset to the start of the stored JavaScript
			compileResult.generatedOffset = prevOffset
			j.AddBytes(compileResult.JS)
			prevOffset = lineColumnOffset{}

			// Include this file in the source map
			if c.options.SourceMap != config.SourceMapNone {
				compileResultsForSourceMap = append(compileResultsForSourceMap, compileResult)
			}

			// Include this file in the metadata
			if c.options.AbsMetadataFile != "" {
				if isFirstMeta {
					isFirstMeta = false
					jMeta.AddString("\n")
				} else {
					jMeta.AddString(",\n")
				}
				jMeta.AddString(fmt.Sprintf("        %s: {\n          \"bytesInOutput\": %d\n        }",
					printer.QuoteForJSON(c.sources[compileResult.sourceIndex].PrettyPath),
					len(compileResult.JS)))
			}
		}
	}

	// Stick the entry point tail at the end of the file. Deliberately don't
	// include any source mapping information for this because it's automatically
	// generated and doesn't correspond to a location in the input file.
	if entryPointTail != nil {
		j.AddBytes(entryPointTail.JS)
	}

	// Put the cross-chunk suffix inside the IIFE
	if len(crossChunkSuffix) > 0 {
		if newlineBeforeComment {
			j.AddString(newline)
		}
		j.AddBytes(crossChunkSuffix)
	}

	// Optionally wrap with an IIFE
	if c.options.OutputFormat == config.FormatIIFE {
		j.AddString("})();" + newline)
	}

	// Make sure the file ends with a newline
	if j.Length() > 0 && j.LastByte() != '\n' {
		j.AddString("\n")
	}

	// Add all unique license comments to the end of the file. These are
	// deduplicated because some projects have thousands of files with the same
	// comment. The comment must be preserved in the output for legal reasons but
	// at the same time we want to generate a small bundle when minifying.
	sort.Strings(commentList)
	for _, text := range commentList {
		j.AddString(text)
		j.AddString("\n")
	}

	jsAbsPath := c.fs.Join(c.options.AbsOutputDir, chunk.name)

	if c.options.SourceMap != config.SourceMapNone {
		sourceMap := c.generateSourceMapForChunk(compileResultsForSourceMap)

		// Store the generated source map
		switch c.options.SourceMap {
		case config.SourceMapInline:
			j.AddString("//# sourceMappingURL=data:application/json;base64,")
			j.AddString(base64.StdEncoding.EncodeToString(sourceMap))
			j.AddString("\n")

		case config.SourceMapLinkedWithComment, config.SourceMapExternalWithoutComment:
			// Optionally add metadata about the file
			var jsonMetadataChunk []byte
			if c.options.AbsMetadataFile != "" {
				jsonMetadataChunk = []byte(fmt.Sprintf(
					"{\n      \"inputs\": {},\n      \"bytes\": %d\n    }", len(sourceMap)))
			}

			results = append(results, OutputFile{
				AbsPath:           jsAbsPath + ".map",
				Contents:          sourceMap,
				jsonMetadataChunk: jsonMetadataChunk,
			})

			// Add a comment linking the source to its map
			if c.options.SourceMap == config.SourceMapLinkedWithComment {
				j.AddString("//# sourceMappingURL=")
				j.AddString(c.fs.Base(jsAbsPath + ".map"))
				j.AddString("\n")
			}
		}
	}

	jsContents := j.Done()

	// End the metadata
	var jsonMetadataChunk []byte
	if c.options.AbsMetadataFile != "" {
		if !isFirstMeta {
			jMeta.AddString("\n      ")
		}
		jMeta.AddString(fmt.Sprintf("},\n      \"bytes\": %d\n    }", len(jsContents)))
		jsonMetadataChunk = jMeta.Done()
	}

	results = append(results, OutputFile{
		AbsPath:           jsAbsPath,
		Contents:          jsContents,
		jsonMetadataChunk: jsonMetadataChunk,
	})
	return
}

func (offset *lineColumnOffset) advance(text string) {
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			offset.lines++
			offset.columns = 0
		} else {
			offset.columns++
		}
	}
}

func markBindingAsUnbound(binding ast.Binding, symbols ast.SymbolMap) {
	switch b := binding.Data.(type) {
	case *ast.BMissing:

	case *ast.BIdentifier:
		symbols.Get(b.Ref).Kind = ast.SymbolUnbound

	case *ast.BArray:
		for _, i := range b.Items {
			markBindingAsUnbound(i.Binding, symbols)
		}

	case *ast.BObject:
		for _, p := range b.Properties {
			markBindingAsUnbound(p.Value, symbols)
		}

	default:
		panic(fmt.Sprintf("Unexpected binding of type %T", binding.Data))
	}
}

// Marking a symbol as unbound prevents it from being renamed or minified.
// This is only used when a module is compiled independently. We use a very
// different way of handling exports and renaming/minifying when bundling.
func (c *linkerContext) markExportsAsUnbound(sourceIndex uint32) {
	file := &c.files[sourceIndex]
	hasImportOrExport := false

	for _, part := range file.ast.Parts {
		for _, stmt := range part.Stmts {
			switch s := stmt.Data.(type) {
			case *ast.SImport:
				// Ignore imports from the internal runtime code. These are generated
				// automatically and aren't part of the original source code. We
				// shouldn't consider the file a module if the only ES6 import or
				// export is the automatically generated one.
				record := &file.ast.ImportRecords[s.ImportRecordIndex]
				if record.SourceIndex != nil && *record.SourceIndex == runtime.SourceIndex {
					continue
				}

				hasImportOrExport = true

			case *ast.SLocal:
				if s.IsExport {
					for _, decl := range s.Decls {
						markBindingAsUnbound(decl.Binding, c.symbols)
					}
					hasImportOrExport = true
				}

			case *ast.SFunction:
				if s.IsExport {
					c.symbols.Get(s.Fn.Name.Ref).Kind = ast.SymbolUnbound
					hasImportOrExport = true
				}

			case *ast.SClass:
				if s.IsExport {
					c.symbols.Get(s.Class.Name.Ref).Kind = ast.SymbolUnbound
					hasImportOrExport = true
				}

			case *ast.SExportClause, *ast.SExportDefault, *ast.SExportStar:
				hasImportOrExport = true

			case *ast.SExportFrom:
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
		for _, ref := range file.ast.ModuleScope.Members {
			c.symbols.Get(ref).Kind = ast.SymbolUnbound
		}
	}
}

func (c *linkerContext) renameOrMinifyAllSymbols() {
	topLevelScopes := make([]*ast.Scope, 0, len(c.files))
	moduleScopes := make([]*ast.Scope, 0, len(c.files))

	// Combine all file scopes
	for _, sourceIndex := range c.reachableFiles {
		file := &c.files[sourceIndex]
		fileMeta := &c.fileMeta[sourceIndex]
		moduleScopes = append(moduleScopes, file.ast.ModuleScope)
		if fileMeta.cjsWrap {
			// Modules wrapped in a CommonJS closure look like this:
			//
			//   // foo.js
			//   var require_foo = __commonJS((exports, module) => {
			//     ...
			//   });
			//
			// The symbol "require_foo" is stored in "file.ast.WrapperRef". We want
			// to be able to minify everything inside the closure without worrying
			// about collisions with other CommonJS modules. Set up the scopes such
			// that it appears as if the file was structured this way all along. It's
			// not completely accurate (e.g. we don't set the parent of the module
			// scope to this new top-level scope) but it's good enough for the
			// renaming code.
			//
			// Note: make sure to not mutate the original scope since it's supposed
			// to be immutable.
			fakeTopLevelScope := &ast.Scope{
				Members:   make(map[string]ast.Ref),
				Generated: []ast.Ref{file.ast.WrapperRef},
				Children:  []*ast.Scope{file.ast.ModuleScope},
			}

			// The unbound symbols are stored in the module scope. We need them for
			// computing reserved names. Avoid needing to copy them all into the new
			// fake top-level scope by still scanning the real module scope for
			// unbound symbols.
			topLevelScopes = append(topLevelScopes, fakeTopLevelScope)
		} else {
			topLevelScopes = append(topLevelScopes, file.ast.ModuleScope)

			// If this isn't CommonJS, then rename the unused "exports" and "module"
			// variables to avoid them causing the identically-named variables in
			// actual CommonJS files from being renamed. This is purely about
			// aesthetics and is not about correctness.
			if !fileMeta.cjsStyleExports {
				name := c.sources[sourceIndex].IdentifierName
				c.symbols.Get(file.ast.ExportsRef).Name = name + "_exports"
				c.symbols.Get(file.ast.ModuleRef).Name = name + "_module"
			}
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
		minifyAllSymbols(reservedNames, topLevelScopes, c.symbols)
	} else {
		renameAllSymbols(reservedNames, topLevelScopes, c.symbols)
	}
}

func (c *linkerContext) generateSourceMapForChunk(results []compileResult) []byte {
	j := printer.Joiner{}
	j.AddString("{\n  \"version\": 3")

	// Write the sources
	j.AddString(",\n  \"sources\": [")
	for i, result := range results {
		sourceFile := c.sources[result.sourceIndex].PrettyPath
		if i > 0 {
			j.AddString(", ")
		}
		j.AddString(printer.QuoteForJSON(sourceFile))
	}
	j.AddString("]")

	// Write the sourcesContent
	j.AddString(",\n  \"sourcesContent\": [")
	for i, result := range results {
		if i > 0 {
			j.AddString(", ")
		}
		j.AddString(result.quotedSource)
	}
	j.AddString("]")

	// Write the mappings
	j.AddString(",\n  \"mappings\": \"")
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
			j.AddBytes(bytes.Repeat([]byte{';'}, offset.lines))
		} else {
			startState.GeneratedColumn += prevColumnOffset
		}

		// Append the precomputed source map chunk
		printer.AppendSourceMapChunk(&j, prevEndState, startState, chunk.Buffer)

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
	j.AddString("\"")

	// Finish the source map
	j.AddString(",\n  \"names\": []\n}\n")
	return j.Done()
}
