package bundler

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/css_printer"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/js_printer"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/renamer"
	"github.com/evanw/esbuild/internal/resolver"
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
	log         logger.Log
	fs          fs.FS
	res         resolver.Resolver
	symbols     js_ast.SymbolMap
	entryPoints []uint32
	files       []file
	hasErrors   bool
	lcaAbsPath  string

	// We should avoid traversing all files in the bundle, because the linker
	// should be able to run a linking operation on a large bundle where only
	// a few files are needed (e.g. an incremental compilation scenario). This
	// holds all files that could possibly be reached through the entry points.
	// If you need to iterate over all files in the linking operation, iterate
	// over this array. This array is also sorted in a deterministic ordering
	// to help ensure deterministic builds (source indices are random).
	reachableFiles []uint32

	// This maps from unstable source index to stable reachable file index. This
	// is useful as a deterministic key for sorting if you need to sort something
	// containing a source index (such as "js_ast.Ref" symbol references).
	stableSourceIndices []uint32

	// We may need to refer to the CommonJS "module" symbol for exports
	unboundModuleRef js_ast.Ref
}

// This contains linker-specific metadata corresponding to a "file" struct
// from the initial scan phase of the bundler. It's separated out because it's
// conceptually only used for a single linking operation and because multiple
// linking operations may be happening in parallel with different metadata for
// the same file.
type fileMeta struct {
	partMeta []partMeta

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
	isProbablyTypeScriptType map[js_ast.Ref]bool

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
	importsToBind map[js_ast.Ref]importToBind

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

	// This is set when we need to pull in the "__export" symbol in to the part
	// at "nsExportPartIndex". This can't be done in "createExportsForFile"
	// because of concurrent map hazards. Instead, it must be done later.
	needsExportSymbolFromRuntime bool

	// The index of the automatically-generated part used to represent the
	// CommonJS wrapper. This part is empty and is only useful for tree shaking
	// and code splitting. The CommonJS wrapper can't be inserted into the part
	// because the wrapper contains other parts, which can't be represented by
	// the current part system.
	cjsWrapperPartIndex *uint32

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
	ref         js_ast.Ref
}

type exportData struct {
	ref js_ast.Ref

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

// This contains linker-specific metadata corresponding to a "js_ast.Part" struct
// from the initial scan phase of the bundler. It's separated out because it's
// conceptually only used for a single linking operation and because multiple
// linking operations may be happening in parallel with different metadata for
// the same part in the same file.
type partMeta struct {
	// This holds all entry points that can reach this part. It will be used to
	// assign this part to a chunk.
	entryBits bitSet

	// If present, this is a circular doubly-linked list of all other parts in
	// this file that need to be in the same chunk as this part to avoid cross-
	// chunk assignments, which are not allowed in ES6 modules.
	//
	// This used to be an array but that was generating lots of allocations.
	// Changing this to a circular doubly-linked list was a substantial speedup.
	prevSibling uint32
	nextSibling uint32

	// These are dependencies that come from other files via import statements.
	nonLocalDependencies []partRef
}

type partRef struct {
	sourceIndex uint32
	partIndex   uint32
}

type chunkInfo struct {
	// The path of this chunk's directory relative to the output directory. Note:
	// this must have OS-independent path separators (i.e. '/' not '\').
	relDir string

	// The name of this chunk. This is initially empty for non-entry point chunks
	// because the file name contains a hash of the file contents, which haven't
	// been generated yet. Don't access this directly. Instead call "relPath()"
	// which first checks that the base name is not empty.
	baseNameOrEmpty string

	filesWithPartsInChunk map[uint32]bool
	entryBits             bitSet

	// This information is only useful if "isEntryPoint" is true
	isEntryPoint  bool
	sourceIndex   uint32 // An index into "c.sources"
	entryPointBit uint   // An index into "c.entryPoints"

	// For code splitting
	crossChunkImports []uint32
	// This is the representation-specific information
	repr chunkRepr
}

type chunkRepr interface {
	fileExt() string
	generate(c *linkerContext, chunk *chunkInfo) func(crossChunkImportRecords []ast.ImportRecord) []OutputFile
}

type chunkReprJS struct {
	// For code splitting
	crossChunkPrefixStmts  []js_ast.Stmt
	crossChunkSuffixStmts  []js_ast.Stmt
	exportsToOtherChunks   map[js_ast.Ref]string
	importsFromOtherChunks map[uint32]crossChunkImportItemArray
}

func (*chunkReprJS) fileExt() string { return ".js" }

type chunkReprCSS struct {
}

func (*chunkReprCSS) fileExt() string { return ".css" }

// Returns the path of this chunk relative to the output directory. Note:
// this must have OS-independent path separators (i.e. '/' not '\').
func (chunk *chunkInfo) relPath() string {
	if chunk.baseNameOrEmpty == "" {
		panic("Internal error")
	}
	return path.Join(chunk.relDir, chunk.baseNameOrEmpty)
}

func newLinkerContext(
	options *config.Options,
	log logger.Log,
	fs fs.FS,
	res resolver.Resolver,
	files []file,
	entryPoints []uint32,
	lcaAbsPath string,
) linkerContext {
	// Clone information about symbols and files so we don't mutate the input data
	c := linkerContext{
		options:        options,
		log:            log,
		fs:             fs,
		res:            res,
		entryPoints:    append([]uint32{}, entryPoints...),
		files:          make([]file, len(files)),
		symbols:        js_ast.NewSymbolMap(len(files)),
		reachableFiles: findReachableFiles(files, entryPoints),
		lcaAbsPath:     lcaAbsPath,
	}

	// Clone various things since we may mutate them later
	for _, sourceIndex := range c.reachableFiles {
		file := files[sourceIndex]

		switch repr := file.repr.(type) {
		case *reprJS:
			repr = &reprJS{ast: repr.ast}
			file.repr = repr

			// Clone the symbol map
			fileSymbols := append([]js_ast.Symbol{}, repr.ast.Symbols...)
			c.symbols.Outer[sourceIndex] = fileSymbols
			repr.ast.Symbols = nil

			// Clone the parts
			repr.ast.Parts = append([]js_ast.Part{}, repr.ast.Parts...)
			for i, part := range repr.ast.Parts {
				clone := make(map[js_ast.Ref]js_ast.SymbolUse, len(part.SymbolUses))
				for ref, uses := range part.SymbolUses {
					clone[ref] = uses
				}
				repr.ast.Parts[i].SymbolUses = clone
			}

			// Clone the import records
			repr.ast.ImportRecords = append([]ast.ImportRecord{}, repr.ast.ImportRecords...)

			// Clone the import map
			namedImports := make(map[js_ast.Ref]js_ast.NamedImport, len(repr.ast.NamedImports))
			for k, v := range repr.ast.NamedImports {
				namedImports[k] = v
			}
			repr.ast.NamedImports = namedImports

			// Clone the export map
			resolvedExports := make(map[string]exportData)
			for alias, ref := range repr.ast.NamedExports {
				resolvedExports[alias] = exportData{
					ref:         ref,
					sourceIndex: sourceIndex,
				}
			}

			// Clone the top-level symbol-to-parts map
			topLevelSymbolToParts := make(map[js_ast.Ref][]uint32)
			for ref, parts := range repr.ast.TopLevelSymbolToParts {
				topLevelSymbolToParts[ref] = parts
			}
			repr.ast.TopLevelSymbolToParts = topLevelSymbolToParts

			// Clone the top-level scope so we can generate more variables
			{
				new := &js_ast.Scope{}
				*new = *repr.ast.ModuleScope
				new.Generated = append([]js_ast.Ref{}, new.Generated...)
				repr.ast.ModuleScope = new
			}

			// Also associate some default metadata with the file
			file.distanceFromEntryPoint = ^uint32(0)
			repr.meta = fileMeta{
				cjsStyleExports: repr.ast.HasCommonJSFeatures() || (repr.ast.HasLazyExport && (c.options.Mode == config.ModePassThrough ||
					(c.options.Mode == config.ModeConvertFormat && !c.options.OutputFormat.KeepES6ImportExportSyntax()))),
				partMeta:                 make([]partMeta, len(repr.ast.Parts)),
				resolvedExports:          resolvedExports,
				isProbablyTypeScriptType: make(map[js_ast.Ref]bool),
				importsToBind:            make(map[js_ast.Ref]importToBind),
			}
		}

		// Update the file in our copy of the file array
		c.files[sourceIndex] = file
	}

	// Create a way to convert source indices to a stable ordering
	c.stableSourceIndices = make([]uint32, len(c.files))
	for stableIndex, sourceIndex := range c.reachableFiles {
		c.stableSourceIndices[sourceIndex] = uint32(stableIndex)
	}

	// Mark all entry points so we don't add them again for import() expressions
	for _, sourceIndex := range entryPoints {
		file := &c.files[sourceIndex]
		file.isEntryPoint = true

		// Entry points must be CommonJS-style if the output format doesn't support
		// ES6 export syntax
		if !options.OutputFormat.KeepES6ImportExportSyntax() {
			if repr, ok := file.repr.(*reprJS); ok && repr.ast.HasES6Exports {
				repr.meta.cjsStyleExports = true
			}
		}
	}

	// Allocate a new unbound symbol called "module" in case we need it later
	if c.options.OutputFormat == config.FormatCommonJS {
		runtimeSymbols := &c.symbols.Outer[runtime.SourceIndex]
		runtimeScope := c.files[runtime.SourceIndex].repr.(*reprJS).ast.ModuleScope
		c.unboundModuleRef = js_ast.Ref{OuterIndex: runtime.SourceIndex, InnerIndex: uint32(len(*runtimeSymbols))}
		runtimeScope.Generated = append(runtimeScope.Generated, c.unboundModuleRef)
		*runtimeSymbols = append(*runtimeSymbols, js_ast.Symbol{
			Kind:         js_ast.SymbolUnbound,
			OriginalName: "module",
			Link:         js_ast.InvalidRef,
		})
	} else {
		c.unboundModuleRef = js_ast.InvalidRef
	}

	return c
}

type indexAndPath struct {
	sourceIndex uint32
	path        logger.Path
}

// This type is just so we can use Go's native sort function
type indexAndPathArray []indexAndPath

func (a indexAndPathArray) Len() int          { return len(a) }
func (a indexAndPathArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a indexAndPathArray) Less(i int, j int) bool {
	return a[i].path.ComesBeforeInSortedOrder(a[j].path)
}

// Find all files reachable from all entry points
func findReachableFiles(files []file, entryPoints []uint32) []uint32 {
	visited := make(map[uint32]bool)
	sorted := indexAndPathArray{}
	var visit func(uint32)

	// Include this file and all files it imports
	visit = func(sourceIndex uint32) {
		if !visited[sourceIndex] {
			visited[sourceIndex] = true
			file := &files[sourceIndex]
			for _, record := range file.repr.importRecords() {
				if record.SourceIndex != nil {
					visit(*record.SourceIndex)
				}
			}
			sorted = append(sorted, indexAndPath{sourceIndex, file.source.KeyPath})
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

func (c *linkerContext) addRangeError(source logger.Source, r logger.Range, text string) {
	c.log.AddRangeError(&source, r, text)
	c.hasErrors = true
}

func (c *linkerContext) addPartToFile(sourceIndex uint32, part js_ast.Part, partMeta partMeta) uint32 {
	if part.LocalDependencies == nil {
		part.LocalDependencies = make(map[uint32]bool)
	}
	if part.SymbolUses == nil {
		part.SymbolUses = make(map[js_ast.Ref]js_ast.SymbolUse)
	}
	if partMeta.entryBits.entries == nil {
		partMeta.entryBits = newBitSet(uint(len(c.entryPoints)))
	}
	repr := c.files[sourceIndex].repr.(*reprJS)
	partIndex := uint32(len(repr.ast.Parts))
	partMeta.prevSibling = partIndex
	partMeta.nextSibling = partIndex
	repr.ast.Parts = append(repr.ast.Parts, part)
	repr.meta.partMeta = append(repr.meta.partMeta, partMeta)
	return partIndex
}

func (c *linkerContext) link() []OutputFile {
	c.scanImportsAndExports()

	// Stop now if there were errors
	if c.hasErrors {
		return []OutputFile{}
	}

	c.markPartsReachableFromEntryPoints()
	c.handleCrossChunkAssignments()

	if c.options.Mode == config.ModePassThrough {
		for _, entryPoint := range c.entryPoints {
			c.markExportsAsUnbound(entryPoint)
		}
	}

	chunks := c.computeChunks()
	c.computeCrossChunkDependencies(chunks)

	// Make sure calls to "js_ast.FollowSymbols()" in parallel goroutines after this
	// won't hit concurrent map mutation hazards
	js_ast.FollowAllSymbols(c.symbols)

	return c.generateChunksInParallel(chunks)
}

func (c *linkerContext) generateChunksInParallel(chunks []chunkInfo) []OutputFile {
	// We want to process chunks with as much parallelism as possible. However,
	// content hashing means chunks that import other chunks must be completed
	// after the imported chunks are completed because the import paths contain
	// the content hash. It's only safe to process a chunk when the dependency
	// count reaches zero.
	type ordering struct {
		dependencies sync.WaitGroup
		dependents   []uint32
	}
	chunkOrdering := make([]ordering, len(chunks))
	for chunkIndex, chunk := range chunks {
		chunkOrdering[chunkIndex].dependencies.Add(len(chunk.crossChunkImports))
		for _, otherChunkIndex := range chunk.crossChunkImports {
			dependents := &chunkOrdering[otherChunkIndex].dependents
			*dependents = append(*dependents, uint32(chunkIndex))
		}
	}

	// Check for loops in the dependency graph since they cause a deadlock
	var check func(int, []int)
	check = func(chunkIndex int, path []int) {
		for _, otherChunkIndex := range path {
			if chunkIndex == otherChunkIndex {
				panic("Internal error: Chunk import graph contains a cycle")
			}
		}
		path = append(path, chunkIndex)
		for _, otherChunkIndex := range chunks[chunkIndex].crossChunkImports {
			check(int(otherChunkIndex), path)
		}
	}
	for i := range chunks {
		check(i, nil)
	}

	results := make([][]OutputFile, len(chunks))
	resultsWaitGroup := sync.WaitGroup{}
	resultsWaitGroup.Add(len(chunks))

	// Generate each chunk on a separate goroutine
	for i := range chunks {
		go func(i int) {
			chunk := &chunks[i]
			order := &chunkOrdering[i]

			// Start generating the chunk without dependencies, but stop when
			// dependencies are needed. This returns a callback that is called
			// later to resume generating the chunk once dependencies are known.
			resume := chunk.repr.generate(c, chunk)

			// Wait for all dependencies to be resolved first
			order.dependencies.Wait()

			// Fill in the cross-chunk import records now that the paths are known
			crossChunkImportRecords := make([]ast.ImportRecord, len(chunk.crossChunkImports))
			for i, otherChunkIndex := range chunk.crossChunkImports {
				crossChunkImportRecords[i] = ast.ImportRecord{
					Kind: ast.ImportStmt,
					Path: logger.Path{Text: c.relativePathBetweenChunks(chunk.relDir, chunks[otherChunkIndex].relPath())},
				}
			}

			// Generate the chunk
			results[i] = resume(crossChunkImportRecords)

			// Wake up any dependents now that we're done
			for _, chunkIndex := range order.dependents {
				chunkOrdering[chunkIndex].dependencies.Done()
			}
			resultsWaitGroup.Done()
		}(i)
	}

	// Join the results in chunk order for determinism
	resultsWaitGroup.Wait()
	var outputFiles []OutputFile
	for _, group := range results {
		outputFiles = append(outputFiles, group...)
	}
	return outputFiles
}

func (c *linkerContext) relativePathBetweenChunks(fromRelDir string, toRelPath string) string {
	relPath, ok := c.fs.Rel(fromRelDir, toRelPath)
	if !ok {
		c.log.AddError(nil, logger.Loc{},
			fmt.Sprintf("Cannot traverse from directory %q to chunk %q", fromRelDir, toRelPath))
		return ""
	}

	// Make sure to always use forward slashes, even on Windows
	relPath = strings.ReplaceAll(relPath, "\\", "/")

	// Make sure the relative path doesn't start with a name, since that could
	// be interpreted as a package path instead of a relative path
	if !strings.HasPrefix(relPath, "./") && !strings.HasPrefix(relPath, "../") {
		relPath = "./" + relPath
	}

	return relPath
}

func (c *linkerContext) computeCrossChunkDependencies(chunks []chunkInfo) {
	if len(chunks) < 2 {
		// No need to compute cross-chunk dependencies if there can't be any
		return
	}

	type chunkMeta struct {
		imports map[js_ast.Ref]bool
		exports map[js_ast.Ref]bool
	}

	chunkMetas := make([]chunkMeta, len(chunks))

	// For each chunk, see what symbols it uses from other chunks. Do this in
	// parallel because it's the most expensive part of this function.
	waitGroup := sync.WaitGroup{}
	waitGroup.Add(len(chunks))
	for chunkIndex, chunk := range chunks {
		go func(chunkIndex int, chunk chunkInfo) {
			chunkKey := string(chunk.entryBits.entries)
			imports := make(map[js_ast.Ref]bool)
			chunkMetas[chunkIndex] = chunkMeta{imports: imports, exports: make(map[js_ast.Ref]bool)}

			// Go over each file in this chunk
			for sourceIndex := range chunk.filesWithPartsInChunk {
				// Go over each part in this file that's marked for inclusion in this chunk
				switch repr := c.files[sourceIndex].repr.(type) {
				case *reprJS:
					for partIndex, partMeta := range repr.meta.partMeta {
						if string(partMeta.entryBits.entries) != chunkKey {
							continue
						}
						part := &repr.ast.Parts[partIndex]

						// Rewrite external dynamic imports to point to the chunk for that entry point
						for _, importRecordIndex := range part.ImportRecordIndices {
							record := &repr.ast.ImportRecords[importRecordIndex]
							if record.SourceIndex != nil && c.isExternalDynamicImport(record) {
								record.Path.Text = c.relativePathBetweenChunks(chunk.relDir, c.files[*record.SourceIndex].entryPointRelPath)
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
								c.symbols.Get(declared.Ref).ChunkIndex = ^uint32(chunkIndex)
							}
						}

						// Record each symbol used in this part. This will later be matched up
						// with our map of which chunk a given symbol is declared in to
						// determine if the symbol needs to be imported from another chunk.
						for ref := range part.SymbolUses {
							symbol := c.symbols.Get(ref)

							// Ignore unbound symbols, which don't have declarations
							if symbol.Kind == js_ast.SymbolUnbound {
								continue
							}

							// Ignore symbols that are going to be replaced by undefined
							if symbol.ImportItemStatus == js_ast.ImportItemMissing {
								continue
							}

							// If this is imported from another file, follow the import
							// reference and reference the symbol in that file instead
							if importToBind, ok := repr.meta.importsToBind[ref]; ok {
								ref = importToBind.ref
								symbol = c.symbols.Get(ref)
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
			waitGroup.Done()
		}(chunkIndex, chunk)
	}
	waitGroup.Wait()

	// Mark imported symbols as exported in the chunk from which they are declared
	for chunkIndex := range chunks {
		chunk := &chunks[chunkIndex]
		repr, ok := chunk.repr.(*chunkReprJS)
		if !ok {
			continue
		}

		// Find all uses in this chunk of symbols from other chunks
		repr.importsFromOtherChunks = make(map[uint32]crossChunkImportItemArray)
		for importRef := range chunkMetas[chunkIndex].imports {
			// Ignore uses that aren't top-level symbols
			otherChunkIndex := ^c.symbols.Get(importRef).ChunkIndex
			if otherChunkIndex != ^uint32(0) && otherChunkIndex != uint32(chunkIndex) {
				repr.importsFromOtherChunks[otherChunkIndex] =
					append(repr.importsFromOtherChunks[otherChunkIndex], crossChunkImportItem{ref: importRef})
				chunkMetas[otherChunkIndex].exports[importRef] = true
			}
		}

		// If this is an entry point, make sure we import all chunks belonging to
		// this entry point, even if there are no imports. We need to make sure
		// these chunks are evaluated for their side effects too.
		if chunk.isEntryPoint {
			for otherChunkIndex, otherChunk := range chunks {
				if chunkIndex != otherChunkIndex && otherChunk.entryBits.hasBit(chunk.entryPointBit) {
					imports := repr.importsFromOtherChunks[uint32(otherChunkIndex)]
					repr.importsFromOtherChunks[uint32(otherChunkIndex)] = imports
				}
			}
		}
	}

	// Generate cross-chunk exports. These must be computed before cross-chunk
	// imports because of export alias renaming, which must consider all export
	// aliases simultaneously to avoid collisions.
	for chunkIndex := range chunks {
		chunk := &chunks[chunkIndex]
		repr, ok := chunk.repr.(*chunkReprJS)
		if !ok {
			continue
		}

		repr.exportsToOtherChunks = make(map[js_ast.Ref]string)
		switch c.options.OutputFormat {
		case config.FormatESModule:
			r := renamer.ExportRenamer{}
			var items []js_ast.ClauseItem
			for _, export := range c.sortedCrossChunkExportItems(chunkMetas[chunkIndex].exports) {
				var alias string
				if c.options.MinifyIdentifiers {
					alias = r.NextMinifiedName()
				} else {
					alias = r.NextRenamedName(c.symbols.Get(export.ref).OriginalName)
				}
				items = append(items, js_ast.ClauseItem{Name: js_ast.LocRef{Ref: export.ref}, Alias: alias})
				repr.exportsToOtherChunks[export.ref] = alias
			}
			if len(items) > 0 {
				repr.crossChunkSuffixStmts = []js_ast.Stmt{{Data: &js_ast.SExportClause{
					Items: items,
				}}}
			}

		default:
			panic("Internal error")
		}
	}

	// Generate cross-chunk imports. These must be computed after cross-chunk
	// exports because the export aliases must already be finalized so they can
	// be embedded in the generated import statements.
	for chunkIndex := range chunks {
		chunk := &chunks[chunkIndex]
		repr, ok := chunk.repr.(*chunkReprJS)
		if !ok {
			continue
		}

		var crossChunkImports []uint32
		var crossChunkPrefixStmts []js_ast.Stmt

		for _, crossChunkImport := range c.sortedCrossChunkImports(chunks, repr.importsFromOtherChunks) {
			switch c.options.OutputFormat {
			case config.FormatESModule:
				var items []js_ast.ClauseItem
				for _, item := range crossChunkImport.sortedImportItems {
					items = append(items, js_ast.ClauseItem{Name: js_ast.LocRef{Ref: item.ref}, Alias: item.exportAlias})
				}
				importRecordIndex := uint32(len(crossChunkImports))
				crossChunkImports = append(crossChunkImports, crossChunkImport.chunkIndex)
				if len(items) > 0 {
					// "import {a, b} from './chunk.js'"
					crossChunkPrefixStmts = append(crossChunkPrefixStmts, js_ast.Stmt{Data: &js_ast.SImport{
						Items:             &items,
						ImportRecordIndex: importRecordIndex,
					}})
				} else {
					// "import './chunk.js'"
					crossChunkPrefixStmts = append(crossChunkPrefixStmts, js_ast.Stmt{Data: &js_ast.SImport{
						ImportRecordIndex: importRecordIndex,
					}})
				}

			default:
				panic("Internal error")
			}
		}

		chunk.crossChunkImports = crossChunkImports
		repr.crossChunkPrefixStmts = crossChunkPrefixStmts
	}
}

type crossChunkImport struct {
	chunkIndex        uint32
	sortingKey        string
	sortedImportItems crossChunkImportItemArray
}

// This type is just so we can use Go's native sort function
type crossChunkImportArray []crossChunkImport

func (a crossChunkImportArray) Len() int          { return len(a) }
func (a crossChunkImportArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a crossChunkImportArray) Less(i int, j int) bool {
	return a[i].sortingKey < a[j].sortingKey
}

// Sort cross-chunk imports by chunk name for determinism
func (c *linkerContext) sortedCrossChunkImports(chunks []chunkInfo, importsFromOtherChunks map[uint32]crossChunkImportItemArray) crossChunkImportArray {
	result := make(crossChunkImportArray, 0, len(importsFromOtherChunks))

	for otherChunkIndex, importItems := range importsFromOtherChunks {
		// Sort imports from a single chunk by alias for determinism
		exportsToOtherChunks := chunks[otherChunkIndex].repr.(*chunkReprJS).exportsToOtherChunks
		for i, item := range importItems {
			importItems[i].exportAlias = exportsToOtherChunks[item.ref]
		}
		sort.Sort(importItems)
		result = append(result, crossChunkImport{
			chunkIndex:        otherChunkIndex,
			sortingKey:        string(chunks[otherChunkIndex].entryBits.entries),
			sortedImportItems: importItems,
		})
	}

	sort.Sort(result)
	return result
}

type crossChunkImportItem struct {
	ref         js_ast.Ref
	exportAlias string
}

// This type is just so we can use Go's native sort function
type crossChunkImportItemArray []crossChunkImportItem

func (a crossChunkImportItemArray) Len() int          { return len(a) }
func (a crossChunkImportItemArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a crossChunkImportItemArray) Less(i int, j int) bool {
	return a[i].exportAlias < a[j].exportAlias
}

type crossChunkExportItem struct {
	ref     js_ast.Ref
	keyPath logger.Path
}

// This type is just so we can use Go's native sort function
type crossChunkExportItemArray []crossChunkExportItem

func (a crossChunkExportItemArray) Len() int          { return len(a) }
func (a crossChunkExportItemArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a crossChunkExportItemArray) Less(i int, j int) bool {
	ai := a[i]
	aj := a[j]

	// The sort order here is arbitrary but needs to be consistent between builds.
	// The InnerIndex should be stable because the parser for a single file is
	// single-threaded and deterministically assigns out InnerIndex values
	// sequentially. But the OuterIndex (i.e. source index) should be unstable
	// because the main thread assigns out source index values sequentially to
	// newly-discovered dependencies in a multi-threaded producer/consumer
	// relationship. So instead we use the key path from the source at OuterIndex
	// for stability. This compares using the InnerIndex first before the key path
	// because it's a less expensive comparison test.
	return ai.ref.InnerIndex < aj.ref.InnerIndex ||
		(ai.ref.InnerIndex == aj.ref.InnerIndex && ai.keyPath.ComesBeforeInSortedOrder(aj.keyPath))
}

// Sort cross-chunk exports by chunk name for determinism
func (c *linkerContext) sortedCrossChunkExportItems(exportRefs map[js_ast.Ref]bool) crossChunkExportItemArray {
	result := make(crossChunkExportItemArray, 0, len(exportRefs))
	for ref := range exportRefs {
		result = append(result, crossChunkExportItem{ref: ref, keyPath: c.files[ref.OuterIndex].source.KeyPath})
	}
	sort.Sort(result)
	return result
}

func (c *linkerContext) scanImportsAndExports() {
	// Step 1: Figure out what modules must be CommonJS
	for _, sourceIndex := range c.reachableFiles {
		if repr, ok := c.files[sourceIndex].repr.(*reprJS); ok {
			for importRecordIndex := range repr.ast.ImportRecords {
				record := &repr.ast.ImportRecords[importRecordIndex]
				if record.SourceIndex == nil {
					continue
				}

				// Make sure the printer can require() CommonJS modules
				otherFile := &c.files[*record.SourceIndex]
				otherRepr := otherFile.repr.(*reprJS)

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
					if !record.DoesNotUseExports {
						if !otherRepr.ast.HasES6Syntax() && !otherRepr.ast.HasLazyExport {
							otherRepr.meta.cjsStyleExports = true
						}
					}

				case ast.ImportRequire:
					// Files that are imported with require() must be CommonJS modules
					otherRepr.meta.cjsStyleExports = true

				case ast.ImportDynamic:
					if c.options.CodeSplitting {
						// Files that are imported with import() must be entry points
						if !otherFile.isEntryPoint {
							c.entryPoints = append(c.entryPoints, *record.SourceIndex)
							otherFile.isEntryPoint = true
						}
					} else {
						// If we're not splitting, then import() is just a require() that
						// returns a promise, so the imported file must be a CommonJS module
						otherRepr.meta.cjsStyleExports = true
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
		if repr, ok := c.files[sourceIndex].repr.(*reprJS); ok && len(repr.ast.ExportStarImportRecords) > 0 {
			visited := make(map[uint32]bool)
			c.isCommonJSDueToExportStar(sourceIndex, visited)
		}
	}

	// Step 3: Resolve "export * from" statements. This must be done after we
	// discover all modules that can be CommonJS because export stars are ignored
	// for CommonJS modules.
	for _, sourceIndex := range c.reachableFiles {
		file := &c.files[sourceIndex]
		repr, ok := file.repr.(*reprJS)
		if !ok {
			continue
		}

		// Expression-style loaders defer code generation until linking. Code
		// generation is done here because at this point we know that the
		// "cjsStyleExports" flag has its final value and will not be changed.
		if repr.ast.HasLazyExport {
			c.generateCodeForLazyExport(sourceIndex)
		}

		// Even if the output file is CommonJS-like, we may still need to wrap
		// CommonJS-style files. Any file that imports a CommonJS-style file will
		// cause that file to need to be wrapped. This is because the import
		// method, whatever it is, will need to invoke the wrapper. Note that
		// this can include entry points (e.g. an entry point that imports a file
		// that imports that entry point).
		for _, record := range repr.ast.ImportRecords {
			if record.SourceIndex != nil {
				otherRepr := c.files[*record.SourceIndex].repr.(*reprJS)
				if otherRepr.meta.cjsStyleExports {
					otherRepr.meta.cjsWrap = true
				}
			}
		}

		// Propagate exports for export star statements
		if len(repr.ast.ExportStarImportRecords) > 0 {
			visited := make(map[uint32]bool)
			c.addExportsForExportStar(repr.meta.resolvedExports, sourceIndex, visited)
		}

		// Add an empty part for the namespace export that we can fill in later
		repr.meta.nsExportPartIndex = c.addPartToFile(sourceIndex, js_ast.Part{
			CanBeRemovedIfUnused: true,
			IsNamespaceExport:    true,
		}, partMeta{})

		// Also add a special export called "*" so import stars can bind to it.
		// This must be done in this step because it must come after CommonJS
		// module discovery but before matching imports with exports.
		repr.meta.resolvedExports["*"] = exportData{
			ref:         repr.ast.ExportsRef,
			sourceIndex: sourceIndex,
		}
		repr.ast.TopLevelSymbolToParts[repr.ast.ExportsRef] = []uint32{repr.meta.nsExportPartIndex}
	}

	// Step 4: Match imports with exports. This must be done after we process all
	// export stars because imports can bind to export star re-exports.
	for _, sourceIndex := range c.reachableFiles {
		file := &c.files[sourceIndex]
		repr, ok := file.repr.(*reprJS)
		if !ok {
			continue
		}

		if len(repr.ast.NamedImports) > 0 {
			c.matchImportsWithExportsForFile(uint32(sourceIndex))
		}

		// If the output format doesn't have an implicit CommonJS wrapper, any file
		// that uses CommonJS features will need to be wrapped, even though the
		// resulting wrapper won't be invoked by other files.
		if !repr.meta.cjsWrap && repr.meta.cjsStyleExports &&
			(c.options.OutputFormat == config.FormatIIFE ||
				c.options.OutputFormat == config.FormatESModule) {
			repr.meta.cjsWrap = true
		}

		// If we're exporting as CommonJS and this file doesn't need a wrapper,
		// then we'll be using the actual CommonJS "exports" and/or "module"
		// symbols. In that case make sure to mark them as such so they don't
		// get minified.
		if (c.options.OutputFormat == config.FormatPreserve || c.options.OutputFormat == config.FormatCommonJS) &&
			!repr.meta.cjsWrap && file.isEntryPoint {
			exportsRef := js_ast.FollowSymbols(c.symbols, repr.ast.ExportsRef)
			moduleRef := js_ast.FollowSymbols(c.symbols, repr.ast.ModuleRef)
			c.symbols.Get(exportsRef).Kind = js_ast.SymbolUnbound
			c.symbols.Get(moduleRef).Kind = js_ast.SymbolUnbound
		}
	}

	// Step 5: Create namespace exports for every file. This is always necessary
	// for CommonJS files, and is also necessary for other files if they are
	// imported using an import star statement.
	waitGroup := sync.WaitGroup{}
	for _, sourceIndex := range c.reachableFiles {
		repr, ok := c.files[sourceIndex].repr.(*reprJS)
		if !ok {
			continue
		}

		// This is the slowest step and is also parallelizable, so do this in parallel.
		waitGroup.Add(1)
		go func(sourceIndex uint32, repr *reprJS) {
			// Now that all exports have been resolved, sort and filter them to create
			// something we can iterate over later.
			aliases := make([]string, 0, len(repr.meta.resolvedExports))
			for alias, export := range repr.meta.resolvedExports {
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
				otherRepr := c.files[export.sourceIndex].repr.(*reprJS)
				if otherRepr.meta.isProbablyTypeScriptType[export.ref] {
					continue
				}

				aliases = append(aliases, alias)
			}
			sort.Strings(aliases)
			repr.meta.sortedAndFilteredExportAliases = aliases

			// Export creation uses "sortedAndFilteredExportAliases" so this must
			// come second after we fill in that array
			c.createExportsForFile(uint32(sourceIndex))
			waitGroup.Done()
		}(sourceIndex, repr)
	}
	waitGroup.Wait()

	// Step 6: Bind imports to exports. This adds non-local dependencies on the
	// parts that declare the export to all parts that use the import.
	for _, sourceIndex := range c.reachableFiles {
		repr, ok := c.files[sourceIndex].repr.(*reprJS)
		if !ok {
			continue
		}

		// If this isn't CommonJS, then rename the unused "exports" and "module"
		// variables to avoid them causing the identically-named variables in
		// actual CommonJS files from being renamed. This is purely about
		// aesthetics and is not about correctness. This is done here because by
		// this point, we know the CommonJS status will not change further.
		if !repr.meta.cjsWrap && !repr.meta.cjsStyleExports {
			name := c.files[sourceIndex].source.IdentifierName
			c.symbols.Get(repr.ast.ExportsRef).OriginalName = name + "_exports"
			c.symbols.Get(repr.ast.ModuleRef).OriginalName = name + "_module"
		}

		// Include the "__export" symbol from the runtime if it was used in the
		// previous step. The previous step can't do this because it's running in
		// parallel and can't safely mutate the "importsToBind" map of another file.
		if repr.meta.needsExportSymbolFromRuntime {
			runtimeRepr := c.files[runtime.SourceIndex].repr.(*reprJS)
			exportRef := runtimeRepr.ast.ModuleScope.Members["__export"].Ref
			exportPart := &repr.ast.Parts[repr.meta.nsExportPartIndex]
			c.generateUseOfSymbolForInclude(exportPart, &repr.meta, 1, exportRef, runtime.SourceIndex)
		}

		for importRef, importToBind := range repr.meta.importsToBind {
			resolvedRepr := c.files[importToBind.sourceIndex].repr.(*reprJS)
			partsDeclaringSymbol := resolvedRepr.ast.TopLevelSymbolToParts[importToBind.ref]

			for _, partIndex := range repr.ast.NamedImports[importRef].LocalPartsWithUses {
				partMeta := &repr.meta.partMeta[partIndex]

				for _, resolvedPartIndex := range partsDeclaringSymbol {
					partMeta.nonLocalDependencies = append(partMeta.nonLocalDependencies, partRef{
						sourceIndex: importToBind.sourceIndex,
						partIndex:   resolvedPartIndex,
					})
				}
			}

			// Merge these symbols so they will share the same name
			js_ast.MergeSymbols(c.symbols, importRef, importToBind.ref)
		}
	}
}

func (c *linkerContext) generateCodeForLazyExport(sourceIndex uint32) {
	file := &c.files[sourceIndex]
	repr := file.repr.(*reprJS)

	// Grab the lazy expression
	if len(repr.ast.Parts) < 1 {
		panic("Internal error")
	}
	part := &repr.ast.Parts[0]
	if len(part.Stmts) != 1 {
		panic("Internal error")
	}
	lazy, ok := part.Stmts[0].Data.(*js_ast.SLazyExport)
	if !ok {
		panic("Internal error")
	}

	// Use "module.exports = value" for CommonJS-style modules
	if repr.meta.cjsStyleExports {
		part.Stmts = []js_ast.Stmt{js_ast.AssignStmt(
			js_ast.Expr{Loc: lazy.Value.Loc, Data: &js_ast.EDot{
				Target:  js_ast.Expr{Loc: lazy.Value.Loc, Data: &js_ast.EIdentifier{Ref: repr.ast.ModuleRef}},
				Name:    "exports",
				NameLoc: lazy.Value.Loc,
			}},
			lazy.Value,
		)}
		part.SymbolUses[repr.ast.ModuleRef] = js_ast.SymbolUse{CountEstimate: 1}
		repr.ast.UsesModuleRef = true
		return
	}

	// Otherwise, generate ES6 export statements. These are added as additional
	// parts so they can be tree shaken individually.
	part.Stmts = nil

	type prevExport struct {
		ref       js_ast.Ref
		partIndex uint32
	}

	generateExport := func(name string, alias string, value js_ast.Expr, prevExports []prevExport) prevExport {
		// Generate a new symbol
		inner := &c.symbols.Outer[sourceIndex]
		ref := js_ast.Ref{OuterIndex: sourceIndex, InnerIndex: uint32(len(*inner))}
		*inner = append(*inner, js_ast.Symbol{Kind: js_ast.SymbolOther, OriginalName: name, Link: js_ast.InvalidRef})
		repr.ast.ModuleScope.Generated = append(repr.ast.ModuleScope.Generated, ref)

		// Generate an ES6 export
		var stmt js_ast.Stmt
		if alias == "default" {
			stmt = js_ast.Stmt{Loc: value.Loc, Data: &js_ast.SExportDefault{
				DefaultName: js_ast.LocRef{Loc: value.Loc, Ref: ref},
				Value:       js_ast.ExprOrStmt{Expr: &value},
			}}
		} else {
			stmt = js_ast.Stmt{Loc: value.Loc, Data: &js_ast.SLocal{
				IsExport: true,
				Decls: []js_ast.Decl{{
					Binding: js_ast.Binding{Loc: value.Loc, Data: &js_ast.BIdentifier{Ref: ref}},
					Value:   &value,
				}},
			}}
		}

		// Link the export into the graph for tree shaking
		partIndex := c.addPartToFile(sourceIndex, js_ast.Part{
			Stmts:                []js_ast.Stmt{stmt},
			SymbolUses:           map[js_ast.Ref]js_ast.SymbolUse{repr.ast.ModuleRef: js_ast.SymbolUse{CountEstimate: 1}},
			DeclaredSymbols:      []js_ast.DeclaredSymbol{{Ref: ref, IsTopLevel: true}},
			CanBeRemovedIfUnused: true,
		}, partMeta{})
		repr.ast.TopLevelSymbolToParts[ref] = []uint32{partIndex}
		repr.meta.resolvedExports[alias] = exportData{ref: ref, sourceIndex: sourceIndex}
		part := &repr.ast.Parts[partIndex]
		for _, export := range prevExports {
			part.SymbolUses[export.ref] = js_ast.SymbolUse{CountEstimate: 1}
			part.LocalDependencies[export.partIndex] = true
		}
		return prevExport{ref: ref, partIndex: partIndex}
	}

	// Unwrap JSON objects into separate top-level variables
	var prevExports []prevExport
	jsonValue := lazy.Value
	if object, ok := jsonValue.Data.(*js_ast.EObject); ok {
		clone := *object
		clone.Properties = append(make([]js_ast.Property, 0, len(clone.Properties)), clone.Properties...)
		for i, property := range clone.Properties {
			if str, ok := property.Key.Data.(*js_ast.EString); ok && (!file.isEntryPoint || js_lexer.IsIdentifierUTF16(str.Value)) {
				name := js_lexer.UTF16ToString(str.Value)
				export := generateExport(name, name, *property.Value, nil)
				prevExports = append(prevExports, export)
				clone.Properties[i].Value = &js_ast.Expr{Loc: property.Key.Loc, Data: &js_ast.EIdentifier{Ref: export.ref}}
			}
		}
		jsonValue.Data = &clone
	}

	// Generate the default export
	generateExport(file.source.IdentifierName+"_default", "default", jsonValue, prevExports)
}

func (c *linkerContext) createExportsForFile(sourceIndex uint32) {
	////////////////////////////////////////////////////////////////////////////////
	// WARNING: This method is run in parallel over all files. Do not mutate data
	// for other files within this method or you will create a data race.
	////////////////////////////////////////////////////////////////////////////////

	var entryPointES6ExportItems []js_ast.ClauseItem
	var entryPointExportStmts []js_ast.Stmt
	file := &c.files[sourceIndex]
	repr := file.repr.(*reprJS)

	// If the output format is ES6 modules and we're an entry point, generate an
	// ES6 export statement containing all exports. Except don't do that if this
	// entry point is a CommonJS-style module, since that would generate an ES6
	// export statement that's not top-level. Instead, we will export the CommonJS
	// exports as a default export later on.
	needsEntryPointES6ExportPart := file.isEntryPoint && !repr.meta.cjsWrap &&
		c.options.OutputFormat == config.FormatESModule && len(repr.meta.sortedAndFilteredExportAliases) > 0

	// Generate a getter per export
	properties := []js_ast.Property{}
	nsExportNonLocalDependencies := []partRef{}
	entryPointExportNonLocalDependencies := []partRef{}
	nsExportSymbolUses := make(map[js_ast.Ref]js_ast.SymbolUse)
	entryPointExportSymbolUses := make(map[js_ast.Ref]js_ast.SymbolUse)
	for _, alias := range repr.meta.sortedAndFilteredExportAliases {
		export := repr.meta.resolvedExports[alias]

		// If this is an export of an import, reference the symbol that the import
		// was eventually resolved to. We need to do this because imports have
		// already been resolved by this point, so we can't generate a new import
		// and have that be resolved later.
		if importToBind, ok := c.files[export.sourceIndex].repr.(*reprJS).meta.importsToBind[export.ref]; ok {
			export.ref = importToBind.ref
			export.sourceIndex = importToBind.sourceIndex
		}

		// Exports of imports need EImportIdentifier in case they need to be re-
		// written to a property access later on
		var value js_ast.Expr
		if c.symbols.Get(export.ref).NamespaceAlias != nil {
			value = js_ast.Expr{Data: &js_ast.EImportIdentifier{Ref: export.ref}}

			// Imported identifiers must be assigned to a local variable to be
			// exported using an ES6 export clause. The import needs to be an
			// EImportIdentifier in case it's imported from a CommonJS module.
			if needsEntryPointES6ExportPart {
				// Generate a temporary variable
				inner := &c.symbols.Outer[sourceIndex]
				tempRef := js_ast.Ref{OuterIndex: sourceIndex, InnerIndex: uint32(len(*inner))}
				*inner = append(*inner, js_ast.Symbol{
					Kind:         js_ast.SymbolOther,
					OriginalName: "export_" + alias,
					Link:         js_ast.InvalidRef,
				})

				// Stick it on the module scope so it gets renamed and minified
				generated := &repr.ast.ModuleScope.Generated
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
				entryPointExportStmts = append(entryPointExportStmts, js_ast.Stmt{Data: &js_ast.SLocal{
					Kind: js_ast.LocalConst,
					Decls: []js_ast.Decl{{
						Binding: js_ast.Binding{Data: &js_ast.BIdentifier{Ref: tempRef}},
						Value:   &js_ast.Expr{Data: &js_ast.EImportIdentifier{Ref: export.ref}},
					}},
				}})
				entryPointES6ExportItems = append(entryPointES6ExportItems, js_ast.ClauseItem{
					Name:  js_ast.LocRef{Ref: tempRef},
					Alias: alias,
				})
				entryPointExportSymbolUses[tempRef] = js_ast.SymbolUse{CountEstimate: 2}
			}
		} else {
			value = js_ast.Expr{Data: &js_ast.EIdentifier{Ref: export.ref}}

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
				entryPointES6ExportItems = append(entryPointES6ExportItems, js_ast.ClauseItem{
					Name:  js_ast.LocRef{Ref: export.ref},
					Alias: alias,
				})
			}
		}

		// Add a getter property
		properties = append(properties, js_ast.Property{
			Key: js_ast.Expr{Data: &js_ast.EString{Value: js_lexer.StringToUTF16(alias)}},
			Value: &js_ast.Expr{Data: &js_ast.EArrow{
				PreferExpr: true,
				Body:       js_ast.FnBody{Stmts: []js_ast.Stmt{{Loc: value.Loc, Data: &js_ast.SReturn{Value: &value}}}},
			}},
		})
		nsExportSymbolUses[export.ref] = js_ast.SymbolUse{CountEstimate: 1}
		if file.isEntryPoint {
			entryPointExportSymbolUses[export.ref] = js_ast.SymbolUse{CountEstimate: 1}
		}

		// Make sure the part that declares the export is included
		for _, partIndex := range c.files[export.sourceIndex].repr.(*reprJS).ast.TopLevelSymbolToParts[export.ref] {
			// Use a non-local dependency since this is likely from a different
			// file if it came in through an export star
			dep := partRef{sourceIndex: export.sourceIndex, partIndex: partIndex}
			nsExportNonLocalDependencies = append(nsExportNonLocalDependencies, dep)
			if file.isEntryPoint {
				entryPointExportNonLocalDependencies = append(entryPointExportNonLocalDependencies, dep)
			}
		}
	}

	// Prefix this part with "var exports = {}" if this isn't a CommonJS module
	declaredSymbols := []js_ast.DeclaredSymbol{}
	var nsExportStmts []js_ast.Stmt
	if !repr.meta.cjsStyleExports {
		nsExportStmts = append(nsExportStmts, js_ast.Stmt{Data: &js_ast.SLocal{Kind: js_ast.LocalConst, Decls: []js_ast.Decl{{
			Binding: js_ast.Binding{Data: &js_ast.BIdentifier{Ref: repr.ast.ExportsRef}},
			Value:   &js_ast.Expr{Data: &js_ast.EObject{}},
		}}}})
		declaredSymbols = append(declaredSymbols, js_ast.DeclaredSymbol{
			Ref:        repr.ast.ExportsRef,
			IsTopLevel: true,
		})
	}

	// "__export(exports, { foo: () => foo })"
	exportRef := js_ast.InvalidRef
	if len(properties) > 0 {
		runtimeRepr := c.files[runtime.SourceIndex].repr.(*reprJS)
		exportRef = runtimeRepr.ast.ModuleScope.Members["__export"].Ref
		nsExportStmts = append(nsExportStmts, js_ast.Stmt{Data: &js_ast.SExpr{Value: js_ast.Expr{Data: &js_ast.ECall{
			Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: exportRef}},
			Args: []js_ast.Expr{
				{Data: &js_ast.EIdentifier{Ref: repr.ast.ExportsRef}},
				{Data: &js_ast.EObject{
					Properties: properties,
				}},
			},
		}}}})

		// Make sure this file depends on the "__export" symbol
		for _, partIndex := range runtimeRepr.ast.TopLevelSymbolToParts[exportRef] {
			dep := partRef{sourceIndex: runtime.SourceIndex, partIndex: partIndex}
			nsExportNonLocalDependencies = append(nsExportNonLocalDependencies, dep)
		}

		// Make sure the CommonJS closure, if there is one, includes "exports"
		repr.ast.UsesExportsRef = true
	}

	// No need to generate a part if it'll be empty
	if len(nsExportStmts) > 0 {
		// Initialize the part that was allocated for us earlier. The information
		// here will be used after this during tree shaking.
		exportPart := &repr.ast.Parts[repr.meta.nsExportPartIndex]
		*exportPart = js_ast.Part{
			Stmts:             nsExportStmts,
			LocalDependencies: make(map[uint32]bool),
			SymbolUses:        nsExportSymbolUses,
			DeclaredSymbols:   declaredSymbols,

			// This can be removed if nothing uses it. Except if we're a CommonJS
			// module, in which case it's always necessary.
			CanBeRemovedIfUnused: !repr.meta.cjsStyleExports,

			// Put the export definitions first before anything else gets evaluated
			IsNamespaceExport: true,

			// Make sure this is trimmed if unused even if tree shaking is disabled
			ForceTreeShaking: true,
		}
		repr.meta.partMeta[repr.meta.nsExportPartIndex].nonLocalDependencies = nsExportNonLocalDependencies

		// Pull in the "__export" symbol if it was used
		if exportRef != js_ast.InvalidRef {
			repr.meta.needsExportSymbolFromRuntime = true
		}
	}

	if len(entryPointES6ExportItems) > 0 {
		entryPointExportStmts = append(entryPointExportStmts,
			js_ast.Stmt{Data: &js_ast.SExportClause{Items: entryPointES6ExportItems}})
	}

	// If we're an entry point, call the require function at the end of the
	// bundle right before bundle evaluation ends
	var cjsWrapStmt js_ast.Stmt
	if file.isEntryPoint && repr.meta.cjsWrap {
		switch c.options.OutputFormat {
		case config.FormatPreserve:
			// "require_foo();"
			cjsWrapStmt = js_ast.Stmt{Data: &js_ast.SExpr{Value: js_ast.Expr{Data: &js_ast.ECall{
				Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: repr.ast.WrapperRef}},
			}}}}

		case config.FormatIIFE:
			if c.options.ModuleName != "" {
				// "return require_foo();"
				cjsWrapStmt = js_ast.Stmt{Data: &js_ast.SReturn{Value: &js_ast.Expr{Data: &js_ast.ECall{
					Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: repr.ast.WrapperRef}},
				}}}}
			} else {
				// "require_foo();"
				cjsWrapStmt = js_ast.Stmt{Data: &js_ast.SExpr{Value: js_ast.Expr{Data: &js_ast.ECall{
					Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: repr.ast.WrapperRef}},
				}}}}
			}

		case config.FormatCommonJS:
			// "module.exports = require_foo();"
			cjsWrapStmt = js_ast.AssignStmt(
				js_ast.Expr{Data: &js_ast.EDot{
					Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: c.unboundModuleRef}},
					Name:   "exports",
				}},
				js_ast.Expr{Data: &js_ast.ECall{
					Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: repr.ast.WrapperRef}},
				}},
			)

		case config.FormatESModule:
			// "export default require_foo();"
			cjsWrapStmt = js_ast.Stmt{Data: &js_ast.SExportDefault{Value: js_ast.ExprOrStmt{Expr: &js_ast.Expr{Data: &js_ast.ECall{
				Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: repr.ast.WrapperRef}},
			}}}}}
		}
	}

	if len(entryPointExportStmts) > 0 || cjsWrapStmt.Data != nil {
		// Trigger evaluation of the CommonJS wrapper
		if cjsWrapStmt.Data != nil {
			entryPointExportSymbolUses[repr.ast.WrapperRef] = js_ast.SymbolUse{CountEstimate: 1}
			entryPointExportStmts = append(entryPointExportStmts, cjsWrapStmt)
		}

		// Add a part for this export clause
		partIndex := c.addPartToFile(sourceIndex, js_ast.Part{
			Stmts:      entryPointExportStmts,
			SymbolUses: entryPointExportSymbolUses,
		}, partMeta{
			nonLocalDependencies: append([]partRef{}, entryPointExportNonLocalDependencies...),
		})
		repr.meta.entryPointExportPartIndex = &partIndex
	}
}

func (c *linkerContext) matchImportsWithExportsForFile(sourceIndex uint32) {
	file := &c.files[sourceIndex]
	repr := file.repr.(*reprJS)

	// Sort imports for determinism. Otherwise our unit tests will randomly
	// fail sometimes when error messages are reordered.
	sortedImportRefs := make([]int, 0, len(repr.ast.NamedImports))
	for ref := range repr.ast.NamedImports {
		sortedImportRefs = append(sortedImportRefs, int(ref.InnerIndex))
	}
	sort.Ints(sortedImportRefs)

	// Pair imports with their matching exports
	for _, innerIndex := range sortedImportRefs {
		importRef := js_ast.Ref{OuterIndex: sourceIndex, InnerIndex: uint32(innerIndex)}
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
					namedImport := repr.ast.NamedImports[importRef]
					c.addRangeError(file.source, js_lexer.RangeOfIdentifier(file.source, namedImport.AliasLoc),
						fmt.Sprintf("Detected cycle while resolving import %q", namedImport.Alias))
					break
				}
				cycleDetector, _ = c.advanceImportTracker(cycleDetector)
			}

			// Resolve the import by one step
			nextTracker, status := c.advanceImportTracker(tracker)
			switch status {
			case importCommonJS, importCommonJSWithoutExports, importExternal, importDisabled:
				if status == importExternal && c.options.OutputFormat.KeepES6ImportExportSyntax() {
					// Imports from external modules should not be converted to CommonJS
					// if the output format preserves the original ES6 import statements
					break
				}

				// If it's a CommonJS or external file, rewrite the import to a
				// property access. Don't do this if the namespace reference is invalid
				// though. This is the case for star imports, where the import is the
				// namespace.
				namedImport := c.files[tracker.sourceIndex].repr.(*reprJS).ast.NamedImports[tracker.importRef]
				if namedImport.NamespaceRef != js_ast.InvalidRef {
					c.symbols.Get(importRef).NamespaceAlias = &js_ast.NamespaceAlias{
						NamespaceRef: namedImport.NamespaceRef,
						Alias:        namedImport.Alias,
					}
				}

				// Warn about importing from a file that is known to not have any exports
				if status == importCommonJSWithoutExports {
					source := c.files[tracker.sourceIndex].source
					c.log.AddRangeWarning(&source, js_lexer.RangeOfIdentifier(source, namedImport.AliasLoc),
						fmt.Sprintf("Import %q will always be undefined", namedImport.Alias))
				}

			case importNoMatch:
				symbol := c.symbols.Get(tracker.importRef)
				if symbol.ImportItemStatus == js_ast.ImportItemGenerated {
					symbol.ImportItemStatus = js_ast.ImportItemMissing
				} else {
					// Report mismatched imports and exports
					source := c.files[tracker.sourceIndex].source
					namedImport := c.files[tracker.sourceIndex].repr.(*reprJS).ast.NamedImports[tracker.importRef]
					c.addRangeError(source, js_lexer.RangeOfIdentifier(source, namedImport.AliasLoc),
						fmt.Sprintf("No matching export for import %q", namedImport.Alias))
				}

			case importAmbiguous:
				source := c.files[tracker.sourceIndex].source
				namedImport := c.files[tracker.sourceIndex].repr.(*reprJS).ast.NamedImports[tracker.importRef]
				c.addRangeError(source, js_lexer.RangeOfIdentifier(source, namedImport.AliasLoc),
					fmt.Sprintf("Ambiguous import %q has multiple matching exports", namedImport.Alias))

			case importProbablyTypeScriptType:
				// Omit this import from any namespace export code we generate for
				// import star statements (i.e. "import * as ns from 'path'")
				repr.meta.isProbablyTypeScriptType[importRef] = true

			case importFound:
				// Defer the actual binding of this import until after we generate
				// namespace export code for all files. This has to be done for all
				// import-to-export matches, not just the initial import to the final
				// export, since all imports and re-exports must be merged together
				// for correctness.
				repr.meta.importsToBind[importRef] = importToBind{
					sourceIndex: nextTracker.sourceIndex,
					ref:         nextTracker.importRef,
				}

				// If this is a re-export of another import, continue for another
				// iteration of the loop to resolve that import as well
				if _, ok := c.files[nextTracker.sourceIndex].repr.(*reprJS).ast.NamedImports[nextTracker.importRef]; ok {
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
	repr := c.files[sourceIndex].repr.(*reprJS)
	if repr.meta.cjsStyleExports {
		return true
	}

	// Avoid infinite loops due to cycles in the export star graph
	if visited[sourceIndex] {
		return false
	}
	visited[sourceIndex] = true

	// Scan over the export star graph
	for _, importRecordIndex := range repr.ast.ExportStarImportRecords {
		record := &repr.ast.ImportRecords[importRecordIndex]

		// This file is CommonJS if the exported imports are from a file that is
		// either CommonJS directly or transitively by itself having an export star
		// from a CommonJS file.
		if (record.SourceIndex == nil && (!c.files[sourceIndex].isEntryPoint || !c.options.OutputFormat.KeepES6ImportExportSyntax())) ||
			(record.SourceIndex != nil && *record.SourceIndex != sourceIndex && c.isCommonJSDueToExportStar(*record.SourceIndex, visited)) {
			repr.meta.cjsStyleExports = true
			return true
		}
	}

	return false
}

func (c *linkerContext) addExportsForExportStar(
	resolvedExports map[string]exportData,
	sourceIndex uint32,
	visited map[uint32]bool,
) {
	// Avoid infinite loops due to cycles in the export star graph
	if visited[sourceIndex] {
		return
	}
	visited[sourceIndex] = true
	repr := c.files[sourceIndex].repr.(*reprJS)

	for _, importRecordIndex := range repr.ast.ExportStarImportRecords {
		record := &repr.ast.ImportRecords[importRecordIndex]
		if record.SourceIndex == nil {
			// This will be resolved at run time instead
			continue
		}
		otherSourceIndex := *record.SourceIndex

		// Export stars from a CommonJS module don't work because they can't be
		// statically discovered. Just silently ignore them in this case.
		//
		// We could attempt to check whether the imported file still has ES6
		// exports even though it still uses CommonJS features. However, when
		// doing this we'd also have to rewrite any imports of these export star
		// re-exports as property accesses off of a generated require() call.
		otherRepr := c.files[otherSourceIndex].repr.(*reprJS)
		if otherRepr.meta.cjsStyleExports {
			// This will be resolved at run time instead
			continue
		}

		// Accumulate this file's exports
		for name, ref := range otherRepr.ast.NamedExports {
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
					isFromExportStar: true,
				}

				// Make sure the symbol is marked as imported so that code splitting
				// imports it correctly if it ends up being shared with another chunk
				repr.meta.importsToBind[ref] = importToBind{
					ref:         ref,
					sourceIndex: otherSourceIndex,
				}
			} else if existing.sourceIndex != otherSourceIndex {
				// Two different re-exports colliding makes it ambiguous
				existing.isAmbiguous = true
				resolvedExports[name] = existing
			}
		}

		// Search further through this file's export stars
		c.addExportsForExportStar(resolvedExports, otherSourceIndex, visited)
	}
}

type importTracker struct {
	sourceIndex uint32
	importRef   js_ast.Ref
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

	// The imported file was disabled by mapping it to false in the "browser"
	// field of package.json
	importDisabled

	// The imported file is external and has unknown exports
	importExternal

	// There are multiple re-exports with the same name due to "export * from"
	importAmbiguous

	// This is a missing re-export in a TypeScript file, so it's probably a type
	importProbablyTypeScriptType
)

func (c *linkerContext) advanceImportTracker(tracker importTracker) (importTracker, importStatus) {
	file := &c.files[tracker.sourceIndex]
	repr := file.repr.(*reprJS)
	namedImport := repr.ast.NamedImports[tracker.importRef]

	// Is this an external file?
	record := &repr.ast.ImportRecords[namedImport.ImportRecordIndex]
	if record.SourceIndex == nil {
		return importTracker{}, importExternal
	}

	// Is this a disabled file?
	otherSourceIndex := *record.SourceIndex
	if c.files[otherSourceIndex].source.KeyPath.Namespace == resolver.BrowserFalseNamespace {
		return importTracker{}, importDisabled
	}

	// Is this a CommonJS file?
	otherRepr := c.files[otherSourceIndex].repr.(*reprJS)
	if otherRepr.meta.cjsStyleExports {
		if !otherRepr.ast.UsesCommonJSExports() && !otherRepr.ast.HasES6Syntax() {
			return importTracker{}, importCommonJSWithoutExports
		}
		return importTracker{}, importCommonJS
	}

	// Match this import up with an export from the imported file
	if matchingExport, ok := otherRepr.meta.resolvedExports[namedImport.Alias]; ok {
		if matchingExport.isAmbiguous {
			return importTracker{}, importAmbiguous
		}

		// Check to see if this is a re-export of another import
		return importTracker{matchingExport.sourceIndex, matchingExport.ref}, importFound
	}

	// Missing re-exports in TypeScript files are indistinguishable from types
	if file.loader.IsTypeScript() && namedImport.IsExported {
		return importTracker{}, importProbablyTypeScriptType
	}

	return importTracker{}, importNoMatch
}

func (c *linkerContext) markPartsReachableFromEntryPoints() {
	// Allocate bit sets
	bitCount := uint(len(c.entryPoints))
	for _, sourceIndex := range c.reachableFiles {
		file := &c.files[sourceIndex]
		file.entryBits = newBitSet(bitCount)

		switch repr := file.repr.(type) {
		case *reprJS:
			for partIndex := range repr.meta.partMeta {
				partMeta := &repr.meta.partMeta[partIndex]
				partMeta.entryBits = newBitSet(bitCount)
				partMeta.prevSibling = uint32(partIndex)
				partMeta.nextSibling = uint32(partIndex)
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
			if repr.meta.cjsWrap {
				runtimeRepr := c.files[runtime.SourceIndex].repr.(*reprJS)
				commonJSRef := runtimeRepr.ast.NamedExports["__commonJS"]
				commonJSParts := runtimeRepr.ast.TopLevelSymbolToParts[commonJSRef]

				// Generate the dummy part
				nonLocalDependencies := make([]partRef, len(commonJSParts))
				for i, partIndex := range commonJSParts {
					nonLocalDependencies[i] = partRef{sourceIndex: runtime.SourceIndex, partIndex: partIndex}
				}
				partIndex := c.addPartToFile(sourceIndex, js_ast.Part{
					SymbolUses: map[js_ast.Ref]js_ast.SymbolUse{
						repr.ast.WrapperRef: {CountEstimate: 1},
						commonJSRef:         {CountEstimate: 1},
					},
					DeclaredSymbols: []js_ast.DeclaredSymbol{
						{Ref: repr.ast.ExportsRef, IsTopLevel: true},
						{Ref: repr.ast.ModuleRef, IsTopLevel: true},
						{Ref: repr.ast.WrapperRef, IsTopLevel: true},
					},
				}, partMeta{
					nonLocalDependencies: nonLocalDependencies,
				})
				repr.meta.cjsWrapperPartIndex = &partIndex
				repr.ast.TopLevelSymbolToParts[repr.ast.WrapperRef] = []uint32{partIndex}
				repr.meta.importsToBind[commonJSRef] = importToBind{
					ref:         commonJSRef,
					sourceIndex: runtime.SourceIndex,
				}
			}
		}
	}

	// Each entry point marks all files reachable from itself
	for i, entryPoint := range c.entryPoints {
		c.includeFile(entryPoint, uint(i), 0)
	}
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
		repr, ok := file.repr.(*reprJS)
		if !ok {
			continue
		}

		for partIndex, part := range repr.ast.Parts {
			// Ignore this part if it's dead code
			if repr.meta.partMeta[partIndex].entryBits.equals(neverReachedEntryBits) {
				continue
			}

			// If this part assigns to a local variable, make sure the parts for the
			// variable's declaration are in the same chunk as this part
			for ref, use := range part.SymbolUses {
				if use.IsAssigned {
					if otherParts, ok := repr.ast.TopLevelSymbolToParts[ref]; ok {
						for _, otherPartIndex := range otherParts {
							partMetaA := &repr.meta.partMeta[partIndex]
							partMetaB := &repr.meta.partMeta[otherPartIndex]

							// Make sure both sibling subsets have the same entry points
							for entryPointBit := range c.entryPoints {
								hasA := partMetaA.entryBits.hasBit(uint(entryPointBit))
								hasB := partMetaB.entryBits.hasBit(uint(entryPointBit))
								if hasA && !hasB {
									c.includePart(sourceIndex, otherPartIndex, uint(entryPointBit), file.distanceFromEntryPoint)
								} else if hasB && !hasA {
									c.includePart(sourceIndex, uint32(partIndex), uint(entryPointBit), file.distanceFromEntryPoint)
								}
							}

							// Perform the merge
							repr.meta.partMeta[partMetaA.nextSibling].prevSibling = partMetaB.prevSibling
							repr.meta.partMeta[partMetaB.prevSibling].nextSibling = partMetaA.nextSibling
							partMetaA.nextSibling = otherPartIndex
							partMetaB.prevSibling = uint32(partIndex)
						}
					}
				}
			}
		}
	}
}

func (c *linkerContext) includeFile(sourceIndex uint32, entryPointBit uint, distanceFromEntryPoint uint32) {
	file := &c.files[sourceIndex]

	// Track the minimum distance to an entry point
	if distanceFromEntryPoint < file.distanceFromEntryPoint {
		file.distanceFromEntryPoint = distanceFromEntryPoint
	}
	distanceFromEntryPoint++

	// Don't mark this file more than once
	if file.entryBits.hasBit(entryPointBit) {
		return
	}
	file.entryBits.setBit(entryPointBit)

	// Only follow dependencies for JavaScript files
	repr, ok := file.repr.(*reprJS)
	if !ok {
		return
	}

	for partIndex, part := range repr.ast.Parts {
		canBeRemovedIfUnused := part.CanBeRemovedIfUnused

		// Don't include the entry point part if we're not the entry point
		if repr.meta.entryPointExportPartIndex != nil && uint32(partIndex) == *repr.meta.entryPointExportPartIndex &&
			sourceIndex != c.entryPoints[entryPointBit] {
			continue
		}

		// Also include any statement-level imports
		for _, importRecordIndex := range part.ImportRecordIndices {
			record := &repr.ast.ImportRecords[importRecordIndex]
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
		if !canBeRemovedIfUnused || (!part.ForceTreeShaking && c.options.Mode != config.ModeBundle && sourceIndex != runtime.SourceIndex) {
			c.includePart(sourceIndex, uint32(partIndex), entryPointBit, distanceFromEntryPoint)
		}
	}

	// If this is an entry point, include all exports
	if file.isEntryPoint {
		for _, alias := range repr.meta.sortedAndFilteredExportAliases {
			export := repr.meta.resolvedExports[alias]
			targetSourceIndex := export.sourceIndex
			targetRef := export.ref

			// If this is an import, then target what the import points to
			targetRepr := c.files[targetSourceIndex].repr.(*reprJS)
			if importToBind, ok := targetRepr.meta.importsToBind[targetRef]; ok {
				targetSourceIndex = importToBind.sourceIndex
				targetRef = importToBind.ref
			}

			// Pull in all declarations of this symbol
			for _, partIndex := range targetRepr.ast.TopLevelSymbolToParts[targetRef] {
				c.includePart(targetSourceIndex, partIndex, entryPointBit, distanceFromEntryPoint)
			}
		}
	}
}

func (c *linkerContext) includePartsForRuntimeSymbol(
	part *js_ast.Part, fileMeta *fileMeta, useCount uint32,
	name string, entryPointBit uint, distanceFromEntryPoint uint32,
) {
	if useCount > 0 {
		runtimeRepr := c.files[runtime.SourceIndex].repr.(*reprJS)
		ref := runtimeRepr.ast.NamedExports[name]

		// Depend on the symbol from the runtime
		c.generateUseOfSymbolForInclude(part, fileMeta, useCount, ref, runtime.SourceIndex)

		// Since this part was included, also include the parts from the runtime
		// that declare this symbol
		for _, partIndex := range runtimeRepr.ast.TopLevelSymbolToParts[ref] {
			c.includePart(runtime.SourceIndex, partIndex, entryPointBit, distanceFromEntryPoint)
		}
	}
}

func (c *linkerContext) generateUseOfSymbolForInclude(
	part *js_ast.Part, fileMeta *fileMeta, useCount uint32,
	ref js_ast.Ref, otherSourceIndex uint32,
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
	return record.Kind == ast.ImportDynamic && c.files[*record.SourceIndex].isEntryPoint
}

func (c *linkerContext) includePart(sourceIndex uint32, partIndex uint32, entryPointBit uint, distanceFromEntryPoint uint32) {
	file := &c.files[sourceIndex]
	repr := file.repr.(*reprJS)
	partMeta := &repr.meta.partMeta[partIndex]

	// Don't mark this part more than once
	if partMeta.entryBits.hasBit(entryPointBit) {
		return
	}
	partMeta.entryBits.setBit(entryPointBit)

	part := &repr.ast.Parts[partIndex]

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

	// Also include any cross-chunk assignment siblings
	for i := partMeta.nextSibling; i != partIndex; i = repr.meta.partMeta[i].nextSibling {
		c.includePart(sourceIndex, i, entryPointBit, distanceFromEntryPoint)
	}

	// Also include any require() imports
	toModuleUses := uint32(0)
	for _, importRecordIndex := range part.ImportRecordIndices {
		record := &repr.ast.ImportRecords[importRecordIndex]

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
		otherRepr := c.files[otherSourceIndex].repr.(*reprJS)
		if record.Kind == ast.ImportStmt && !otherRepr.meta.cjsStyleExports {
			// Skip this since it's not a require() import
			continue
		}

		// This is a require() import
		c.includeFile(otherSourceIndex, entryPointBit, distanceFromEntryPoint)

		// Depend on the automatically-generated require wrapper symbol
		wrapperRef := otherRepr.ast.WrapperRef
		c.generateUseOfSymbolForInclude(part, &repr.meta, 1, wrapperRef, otherSourceIndex)

		// This is an ES6 import of a CommonJS module, so it needs the
		// "__toModule" wrapper as long as it's not a bare "require()"
		if record.Kind != ast.ImportRequire {
			record.WrapWithToModule = true
			toModuleUses++
		}
	}

	// If there's an ES6 import of a non-ES6 module, then we're going to need the
	// "__toModule" symbol from the runtime to wrap the result of "require()"
	c.includePartsForRuntimeSymbol(part, &repr.meta, toModuleUses, "__toModule", entryPointBit, distanceFromEntryPoint)

	// If there's an ES6 export star statement of a non-ES6 module, then we're
	// going to need the "__exportStar" symbol from the runtime
	exportStarUses := uint32(0)
	for _, importRecordIndex := range repr.ast.ExportStarImportRecords {
		record := &repr.ast.ImportRecords[importRecordIndex]

		// Is this export star evaluated at run time?
		if (record.SourceIndex == nil && (!file.isEntryPoint || !c.options.OutputFormat.KeepES6ImportExportSyntax())) ||
			(record.SourceIndex != nil && *record.SourceIndex != sourceIndex && c.files[*record.SourceIndex].repr.(*reprJS).meta.cjsStyleExports) {
			record.CallsRunTimeExportStarFn = true
			repr.ast.UsesExportsRef = true
			exportStarUses++
		}
	}
	c.includePartsForRuntimeSymbol(part, &repr.meta, exportStarUses, "__exportStar", entryPointBit, distanceFromEntryPoint)
}

func (c *linkerContext) computeChunks() []chunkInfo {
	chunks := make(map[string]chunkInfo)
	neverReachedKey := string(newBitSet(uint(len(c.entryPoints))).entries)

	// Compute entry point names
	for i, entryPoint := range c.entryPoints {
		var relDir string
		var baseName string
		var repr chunkRepr
		file := &c.files[entryPoint]

		switch file.repr.(type) {
		case *reprJS:
			repr = &chunkReprJS{}
		case *reprCSS:
			repr = &chunkReprCSS{}
		}

		if c.options.AbsOutputFile != "" {
			baseName = c.fs.Base(c.options.AbsOutputFile)
		} else {
			source := file.source
			if source.KeyPath.Namespace != "file" {
				baseName = source.IdentifierName
			} else if relPath, ok := c.fs.Rel(c.lcaAbsPath, source.KeyPath.Text); ok {
				relDir = c.fs.Dir(relPath)
				baseName = c.fs.Base(relPath)
			} else {
				baseName = c.fs.Base(source.KeyPath.Text)
			}

			// Swap the extension for the standard one
			ext := c.fs.Ext(baseName)
			baseName = baseName[:len(baseName)-len(ext)] + c.options.OutputExtensionFor(repr.fileExt())
		}

		// Always use cross-platform path separators to avoid problems with Windows
		relDir = strings.ReplaceAll(relDir, "\\", "/")
		file.entryPointRelPath = path.Join(relDir, baseName)

		// Create a chunk for the entry point here to ensure that the chunk is
		// always generated even if the resulting file is empty
		entryBits := newBitSet(uint(len(c.entryPoints)))
		entryBits.setBit(uint(i))
		chunks[string(entryBits.entries)] = chunkInfo{
			entryBits:             entryBits,
			isEntryPoint:          true,
			sourceIndex:           entryPoint,
			entryPointBit:         uint(i),
			relDir:                relDir,
			baseNameOrEmpty:       baseName,
			filesWithPartsInChunk: make(map[uint32]bool),
			repr:                  repr,
		}
	}

	// Figure out which files are in which chunk
	for _, sourceIndex := range c.reachableFiles {
		file := &c.files[sourceIndex]
		switch repr := file.repr.(type) {
		case *reprJS:
			for _, partMeta := range repr.meta.partMeta {
				key := string(partMeta.entryBits.entries)
				if key == neverReachedKey {
					// Ignore this part if it was never reached
					continue
				}
				chunk, ok := chunks[key]
				if !ok {
					chunk.entryBits = partMeta.entryBits
					chunk.filesWithPartsInChunk = make(map[uint32]bool)
					chunk.repr = &chunkReprJS{}
					chunks[key] = chunk
				}
				chunk.filesWithPartsInChunk[uint32(sourceIndex)] = true
			}

		case *reprCSS:
			key := string(file.entryBits.entries)
			if key == neverReachedKey {
				// Ignore this file if it was never reached
				continue
			}
			chunk, ok := chunks[key]
			if !ok {
				chunk.entryBits = file.entryBits
				chunk.filesWithPartsInChunk = make(map[uint32]bool)
				chunk.repr = &chunkReprJS{}
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
	sortedChunks := make([]chunkInfo, len(chunks))
	for i, key := range sortedKeys {
		sortedChunks[i] = chunks[key]
	}
	return sortedChunks
}

type chunkOrder struct {
	sourceIndex uint32
	distance    uint32
	path        logger.Path
}

// This type is just so we can use Go's native sort function
type chunkOrderArray []chunkOrder

func (a chunkOrderArray) Len() int          { return len(a) }
func (a chunkOrderArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a chunkOrderArray) Less(i int, j int) bool {
	return a[i].distance < a[j].distance || (a[i].distance == a[j].distance && a[i].path.ComesBeforeInSortedOrder(a[j].path))
}

func (c *linkerContext) chunkFileOrder(chunk *chunkInfo) []uint32 {
	sorted := make(chunkOrderArray, 0, len(chunk.filesWithPartsInChunk))

	// Attach information to the files for use with sorting
	for sourceIndex := range chunk.filesWithPartsInChunk {
		file := &c.files[sourceIndex]
		sorted = append(sorted, chunkOrder{
			sourceIndex: sourceIndex,
			distance:    file.distanceFromEntryPoint,
			path:        file.source.KeyPath,
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
		isFileInThisChunk := chunk.entryBits.equals(file.entryBits)

		switch repr := file.repr.(type) {
		case *reprJS:
			for partIndex, part := range repr.ast.Parts {
				isPartInThisChunk := chunk.entryBits.equals(repr.meta.partMeta[partIndex].entryBits)
				if isPartInThisChunk {
					isFileInThisChunk = true
				}

				// Also traverse any files imported by this part
				for _, importRecordIndex := range part.ImportRecordIndices {
					record := &repr.ast.ImportRecords[importRecordIndex]
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
				if sourceIndex == runtime.SourceIndex || repr.meta.cjsWrap {
					prefixOrder = append(prefixOrder, sourceIndex)
				} else {
					suffixOrder = append(suffixOrder, sourceIndex)
				}
			}

		case *reprCSS:
			if isFileInThisChunk {
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
	partStmts []js_ast.Stmt,
	loc logger.Loc,
	namespaceRef js_ast.Ref,
	importRecordIndex uint32,
) bool {
	// Is this an import from another module inside this bundle?
	repr := c.files[sourceIndex].repr.(*reprJS)
	record := &repr.ast.ImportRecords[importRecordIndex]
	if record.SourceIndex != nil {
		if !c.files[*record.SourceIndex].repr.(*reprJS).meta.cjsStyleExports {
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
	if repr.meta.cjsStyleExports && js_ast.FollowSymbols(c.symbols, namespaceRef) == repr.ast.ExportsRef {
		return true
	}

	// Replace the statement with a call to "require()"
	stmtList.prefixStmts = append(stmtList.prefixStmts, js_ast.Stmt{
		Loc: loc,
		Data: &js_ast.SLocal{Kind: js_ast.LocalConst, Decls: []js_ast.Decl{{
			Binding: js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: namespaceRef}},
			Value:   &js_ast.Expr{Loc: record.Range.Loc, Data: &js_ast.ERequire{ImportRecordIndex: importRecordIndex}},
		}}},
	})
	return true
}

func (c *linkerContext) convertStmtsForChunk(sourceIndex uint32, stmtList *stmtList, partStmts []js_ast.Stmt) {
	shouldStripExports := c.options.Mode != config.ModePassThrough || sourceIndex == runtime.SourceIndex
	repr := c.files[sourceIndex].repr.(*reprJS)
	shouldExtractES6StmtsForCJSWrap := repr.meta.cjsWrap

	for _, stmt := range partStmts {
		switch s := stmt.Data.(type) {
		case *js_ast.SImport:
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

		case *js_ast.SExportStar:
			if s.Alias == nil {
				// "export * from 'path'"
				if shouldStripExports {
					record := &repr.ast.ImportRecords[s.ImportRecordIndex]

					// Is this export star evaluated at run time?
					if record.SourceIndex == nil && c.options.OutputFormat.KeepES6ImportExportSyntax() {
						if record.CallsRunTimeExportStarFn {
							// Turn this statement into "import * as ns from 'path'"
							stmt.Data = &js_ast.SImport{
								NamespaceRef:      s.NamespaceRef,
								StarNameLoc:       &stmt.Loc,
								ImportRecordIndex: s.ImportRecordIndex,
							}

							// Prefix this module with "__exportStar(exports, ns)"
							exportStarRef := c.files[runtime.SourceIndex].repr.(*reprJS).ast.ModuleScope.Members["__exportStar"].Ref
							stmtList.prefixStmts = append(stmtList.prefixStmts, js_ast.Stmt{
								Loc: stmt.Loc,
								Data: &js_ast.SExpr{Value: js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.ECall{
									Target: js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: exportStarRef}},
									Args: []js_ast.Expr{
										{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: repr.ast.ExportsRef}},
										{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: s.NamespaceRef}},
									},
								}}},
							})

							// Make sure these don't end up in a CommonJS wrapper
							if shouldExtractES6StmtsForCJSWrap {
								stmtList.es6StmtsForCJSWrap = append(stmtList.es6StmtsForCJSWrap, stmt)
								continue
							}
						}
					} else {
						if record.CallsRunTimeExportStarFn {
							// Prefix this module with "__exportStar(exports, require(path))"
							exportStarRef := c.files[runtime.SourceIndex].repr.(*reprJS).ast.ModuleScope.Members["__exportStar"].Ref
							stmtList.prefixStmts = append(stmtList.prefixStmts, js_ast.Stmt{
								Loc: stmt.Loc,
								Data: &js_ast.SExpr{Value: js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.ECall{
									Target: js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: exportStarRef}},
									Args: []js_ast.Expr{
										{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: repr.ast.ExportsRef}},
										{Loc: record.Range.Loc, Data: &js_ast.ERequire{ImportRecordIndex: s.ImportRecordIndex}},
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
					stmt.Data = &js_ast.SImport{
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

		case *js_ast.SExportFrom:
			// "export {foo} from 'path'"
			if c.shouldRemoveImportExportStmt(sourceIndex, stmtList, partStmts, stmt.Loc, s.NamespaceRef, s.ImportRecordIndex) {
				continue
			}

			if shouldStripExports {
				// Turn this statement into "import {foo} from 'path'"
				for i, item := range s.Items {
					s.Items[i].Alias = item.OriginalName
				}
				stmt.Data = &js_ast.SImport{
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

		case *js_ast.SExportClause:
			if shouldStripExports {
				// Remove export statements entirely
				continue
			}

			// Make sure these don't end up in a CommonJS wrapper
			if shouldExtractES6StmtsForCJSWrap {
				stmtList.es6StmtsForCJSWrap = append(stmtList.es6StmtsForCJSWrap, stmt)
				continue
			}

		case *js_ast.SFunction:
			// Strip the "export" keyword while bundling
			if shouldStripExports && s.IsExport {
				// Be careful to not modify the original statement
				clone := *s
				clone.IsExport = false
				stmt.Data = &clone
			}

		case *js_ast.SClass:
			// Strip the "export" keyword while bundling
			if shouldStripExports && s.IsExport {
				// Be careful to not modify the original statement
				clone := *s
				clone.IsExport = false
				stmt.Data = &clone
			}

		case *js_ast.SLocal:
			// Strip the "export" keyword while bundling
			if shouldStripExports && s.IsExport {
				// Be careful to not modify the original statement
				clone := *s
				clone.IsExport = false
				stmt.Data = &clone
			}

		case *js_ast.SExportDefault:
			// If we're bundling, convert "export default" into a normal declaration
			if shouldStripExports {
				if s.Value.Expr != nil {
					// "export default foo;" => "var default = foo;"
					stmt = js_ast.Stmt{Loc: stmt.Loc, Data: &js_ast.SLocal{Decls: []js_ast.Decl{
						{Binding: js_ast.Binding{Loc: s.DefaultName.Loc, Data: &js_ast.BIdentifier{Ref: s.DefaultName.Ref}}, Value: s.Value.Expr},
					}}}
				} else {
					switch s2 := s.Value.Stmt.Data.(type) {
					case *js_ast.SFunction:
						// "export default function() {}" => "function default() {}"
						// "export default function foo() {}" => "function foo() {}"

						// Be careful to not modify the original statement
						s2 = &js_ast.SFunction{Fn: s2.Fn}
						s2.Fn.Name = &s.DefaultName

						stmt = js_ast.Stmt{Loc: s.Value.Stmt.Loc, Data: s2}

					case *js_ast.SClass:
						// "export default class {}" => "class default {}"
						// "export default class Foo {}" => "class Foo {}"

						// Be careful to not modify the original statement
						s2 = &js_ast.SClass{Class: s2.Class}
						s2.Class.Name = &s.DefaultName

						stmt = js_ast.Stmt{Loc: s.Value.Stmt.Loc, Data: s2}

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
func mergeAdjacentLocalStmts(stmts []js_ast.Stmt) []js_ast.Stmt {
	if len(stmts) == 0 {
		return stmts
	}

	didMergeWithPreviousLocal := false
	end := 1

	for _, stmt := range stmts[1:] {
		// Try to merge with the previous variable statement
		if after, ok := stmt.Data.(*js_ast.SLocal); ok {
			if before, ok := stmts[end-1].Data.(*js_ast.SLocal); ok {
				// It must be the same kind of variable statement (i.e. let/var/const)
				if before.Kind == after.Kind && before.IsExport == after.IsExport {
					if didMergeWithPreviousLocal {
						// Avoid O(n^2) behavior for repeated variable declarations
						before.Decls = append(before.Decls, after.Decls...)
					} else {
						// Be careful to not modify the original statement
						didMergeWithPreviousLocal = true
						clone := *before
						clone.Decls = make([]js_ast.Decl, 0, len(before.Decls)+len(after.Decls))
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
	prefixStmts []js_ast.Stmt

	// These statements come last, and can be inside the CommonJS wrapper
	normalStmts []js_ast.Stmt

	// Order doesn't matter for these statements, but they must be outside any
	// CommonJS wrapper since they are top-level ES6 import/export statements
	es6StmtsForCJSWrap []js_ast.Stmt

	// These statements are for an entry point and come at the end of the chunk
	entryPointTail []js_ast.Stmt
}

type lineColumnOffset struct {
	lines   int
	columns int
}

type compileResultJS struct {
	js_printer.PrintResult

	// If this is an entry point, this is optional code to stick on the end of
	// the chunk. This is used to for example trigger the lazily-evaluated
	// CommonJS wrapper for the entry point.
	entryPointTail *js_printer.PrintResult

	sourceIndex uint32

	// This is the line and column offset since the previous JavaScript string
	// or the start of the file if this is the first JavaScript string.
	generatedOffset lineColumnOffset
}

func (c *linkerContext) generateCodeForFileInChunkJS(
	r renamer.Renamer,
	waitGroup *sync.WaitGroup,
	sourceIndex uint32,
	entryBits bitSet,
	commonJSRef js_ast.Ref,
	toModuleRef js_ast.Ref,
	result *compileResultJS,
) {
	file := &c.files[sourceIndex]
	repr := file.repr.(*reprJS)
	needsWrapper := false
	stmtList := stmtList{}

	// Make sure the generated call to "__export(exports, ...)" comes first
	// before anything else.
	if entryBits.equals(repr.meta.partMeta[repr.meta.nsExportPartIndex].entryBits) {
		c.convertStmtsForChunk(sourceIndex, &stmtList, repr.ast.Parts[repr.meta.nsExportPartIndex].Stmts)

		// Move everything to the prefix list
		stmtList.prefixStmts = append(stmtList.prefixStmts, stmtList.normalStmts...)
		stmtList.normalStmts = nil
	}

	// Add all other parts in this chunk
	for partIndex, part := range repr.ast.Parts {
		if !entryBits.equals(repr.meta.partMeta[partIndex].entryBits) {
			// Skip the part if it's not in this chunk
			continue
		}

		if uint32(partIndex) == repr.meta.nsExportPartIndex {
			// Skip the generated call to "__export()" that was extracted above
			continue
		}

		// Mark if we hit the dummy part representing the CommonJS wrapper
		if repr.meta.cjsWrapperPartIndex != nil && uint32(partIndex) == *repr.meta.cjsWrapperPartIndex {
			needsWrapper = true
			continue
		}

		// Emit export statements in the entry point part verbatim
		if repr.meta.entryPointExportPartIndex != nil && uint32(partIndex) == *repr.meta.entryPointExportPartIndex {
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
		args := []js_ast.Arg{}
		if repr.ast.UsesExportsRef || repr.ast.UsesModuleRef {
			args = append(args, js_ast.Arg{Binding: js_ast.Binding{Data: &js_ast.BIdentifier{Ref: repr.ast.ExportsRef}}})
			if repr.ast.UsesModuleRef {
				args = append(args, js_ast.Arg{Binding: js_ast.Binding{Data: &js_ast.BIdentifier{Ref: repr.ast.ModuleRef}}})
			}
		}

		// "__commonJS((exports, module) => { ... })"
		value := js_ast.Expr{Data: &js_ast.ECall{
			Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: commonJSRef}},
			Args:   []js_ast.Expr{{Data: &js_ast.EArrow{Args: args, Body: js_ast.FnBody{Stmts: stmts}}}},
		}}

		// "var require_foo = __commonJS((exports, module) => { ... });"
		stmts = append(stmtList.es6StmtsForCJSWrap, js_ast.Stmt{Data: &js_ast.SLocal{
			Decls: []js_ast.Decl{{
				Binding: js_ast.Binding{Data: &js_ast.BIdentifier{Ref: repr.ast.WrapperRef}},
				Value:   &value,
			}},
		}})
	}

	// Only generate a source map if needed
	sourceForSourceMap := &c.files[sourceIndex].source
	if !file.loader.CanHaveSourceMap() || c.options.SourceMap == config.SourceMapNone {
		sourceForSourceMap = nil
	}

	// Indent the file if everything is wrapped in an IIFE
	indent := 0
	if c.options.OutputFormat == config.FormatIIFE {
		indent++
	}

	// Convert the AST to JavaScript code
	printOptions := js_printer.PrintOptions{
		Indent:              indent,
		OutputFormat:        c.options.OutputFormat,
		RemoveWhitespace:    c.options.RemoveWhitespace,
		MangleSyntax:        c.options.MangleSyntax,
		ToModuleRef:         toModuleRef,
		ExtractComments:     c.options.Mode == config.ModeBundle && c.options.RemoveWhitespace,
		UnsupportedFeatures: c.options.UnsupportedFeatures,
		SourceForSourceMap:  sourceForSourceMap,
		InputSourceMap:      file.sourceMap,
		WrapperRefForSource: func(sourceIndex uint32) js_ast.Ref {
			return c.files[sourceIndex].repr.(*reprJS).ast.WrapperRef
		},
	}
	tree := repr.ast
	tree.Parts = []js_ast.Part{{Stmts: stmts}}
	*result = compileResultJS{
		PrintResult: js_printer.Print(tree, c.symbols, r, printOptions),
		sourceIndex: sourceIndex,
	}

	// Write this separately as the entry point tail so it can be split off
	// from the main entry point code. This is sometimes required to deal with
	// CommonJS import cycles.
	if len(stmtList.entryPointTail) > 0 {
		tree := repr.ast
		tree.Parts = []js_ast.Part{{Stmts: stmtList.entryPointTail}}
		entryPointTail := js_printer.Print(tree, c.symbols, r, printOptions)
		result.entryPointTail = &entryPointTail
	}

	waitGroup.Done()
}

func (c *linkerContext) renameSymbolsInChunk(chunk *chunkInfo, filesInOrder []uint32) renamer.Renamer {
	// Determine the reserved names (e.g. can't generate the name "if")
	moduleScopes := make([]*js_ast.Scope, len(filesInOrder))
	for i, sourceIndex := range filesInOrder {
		moduleScopes[i] = c.files[sourceIndex].repr.(*reprJS).ast.ModuleScope
	}
	reservedNames := renamer.ComputeReservedNames(moduleScopes, c.symbols)

	// These are used to implement bundling, and need to be free for use
	if c.options.Mode != config.ModePassThrough {
		reservedNames["require"] = 1
		reservedNames["Promise"] = 1
	}

	// Minification uses frequency analysis to give shorter names to more frequent symbols
	if c.options.MinifyIdentifiers {
		// Determine the first top-level slot (i.e. not in a nested scope)
		var firstTopLevelSlots js_ast.SlotCounts
		for _, sourceIndex := range filesInOrder {
			firstTopLevelSlots.UnionMax(c.files[sourceIndex].repr.(*reprJS).ast.NestedScopeSlotCounts)
		}
		r := renamer.NewMinifyRenamer(c.symbols, firstTopLevelSlots, reservedNames)

		// Accumulate symbol usage counts into their slots
		freq := js_ast.CharFreq{}
		for _, sourceIndex := range filesInOrder {
			repr := c.files[sourceIndex].repr.(*reprJS)
			if repr.ast.CharFreq != nil {
				freq.Include(repr.ast.CharFreq)
			}
			if repr.ast.UsesExportsRef {
				r.AccumulateSymbolCount(repr.ast.ExportsRef, 1)
			}
			if repr.ast.UsesModuleRef {
				r.AccumulateSymbolCount(repr.ast.ModuleRef, 1)
			}

			for partIndex, part := range repr.ast.Parts {
				if !chunk.entryBits.equals(repr.meta.partMeta[partIndex].entryBits) {
					// Skip the part if it's not in this chunk
					continue
				}

				// Accumulate symbol use counts
				r.AccumulateSymbolUseCounts(part.SymbolUses, c.stableSourceIndices)

				// Make sure to also count the declaration in addition to the uses
				for _, declared := range part.DeclaredSymbols {
					r.AccumulateSymbolCount(declared.Ref, 1)
				}
			}
		}

		// Add all of the character frequency histograms for all files in this
		// chunk together, then use it to compute the character sequence used to
		// generate minified names. This results in slightly better gzip compression
		// over assigning minified names in order (i.e. "a b c ..."). Even though
		// it's a very small win, we still do it because it's simple to do and very
		// cheap to compute.
		minifier := freq.Compile()
		r.AssignNamesByFrequency(&minifier)
		return r
	}

	// When we're not minifying, just append numbers to symbol names to avoid collisions
	r := renamer.NewNumberRenamer(c.symbols, reservedNames)
	nestedScopes := make(map[uint32][]*js_ast.Scope)

	// Make sure imports get a chance to be renamed
	var sorted renamer.StableRefArray
	for _, imports := range chunk.repr.(*chunkReprJS).importsFromOtherChunks {
		for _, item := range imports {
			sorted = append(sorted, renamer.StableRef{
				StableOuterIndex: c.stableSourceIndices[item.ref.OuterIndex],
				Ref:              item.ref,
			})
		}
	}
	sort.Sort(sorted)
	for _, stable := range sorted {
		r.AddTopLevelSymbol(stable.Ref)
	}

	for _, sourceIndex := range filesInOrder {
		repr := c.files[sourceIndex].repr.(*reprJS)
		var scopes []*js_ast.Scope

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
		if repr.meta.cjsWrap {
			r.AddTopLevelSymbol(repr.ast.WrapperRef)
			nestedScopes[sourceIndex] = []*js_ast.Scope{repr.ast.ModuleScope}
			continue
		}

		// Rename each top-level symbol declaration in this chunk
		for partIndex, part := range repr.ast.Parts {
			if chunk.entryBits.equals(repr.meta.partMeta[partIndex].entryBits) {
				for _, declared := range part.DeclaredSymbols {
					if declared.IsTopLevel {
						r.AddTopLevelSymbol(declared.Ref)
					}
				}
				scopes = append(scopes, part.Scopes...)
			}
		}

		nestedScopes[sourceIndex] = scopes
	}

	// Recursively rename symbols in child scopes now that all top-level
	// symbols have been renamed. This is done in parallel because the symbols
	// inside nested scopes are independent and can't conflict.
	r.AssignNamesByScope(nestedScopes)
	return r
}

func (repr *chunkReprJS) generate(c *linkerContext, chunk *chunkInfo) func([]ast.ImportRecord) []OutputFile {
	var results []OutputFile
	filesInChunkInOrder := c.chunkFileOrder(chunk)
	compileResults := make([]compileResultJS, 0, len(filesInChunkInOrder))
	runtimeMembers := c.files[runtime.SourceIndex].repr.(*reprJS).ast.ModuleScope.Members
	commonJSRef := js_ast.FollowSymbols(c.symbols, runtimeMembers["__commonJS"].Ref)
	toModuleRef := js_ast.FollowSymbols(c.symbols, runtimeMembers["__toModule"].Ref)
	r := c.renameSymbolsInChunk(chunk, filesInChunkInOrder)

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
		compileResults = append(compileResults, compileResultJS{})
		compileResult := &compileResults[len(compileResults)-1]
		waitGroup.Add(1)
		go c.generateCodeForFileInChunkJS(
			r,
			&waitGroup,
			sourceIndex,
			chunk.entryBits,
			commonJSRef,
			toModuleRef,
			compileResult,
		)
	}

	// Wait for cross-chunk import records before continuing
	return func(crossChunkImportRecords []ast.ImportRecord) []OutputFile {
		// Also generate the cross-chunk binding code
		var crossChunkPrefix []byte
		var crossChunkSuffix []byte
		{
			// Indent the file if everything is wrapped in an IIFE
			indent := 0
			if c.options.OutputFormat == config.FormatIIFE {
				indent++
			}
			printOptions := js_printer.PrintOptions{
				Indent:           indent,
				OutputFormat:     c.options.OutputFormat,
				RemoveWhitespace: c.options.RemoveWhitespace,
				MangleSyntax:     c.options.MangleSyntax,
			}
			crossChunkPrefix = js_printer.Print(js_ast.AST{
				ImportRecords: crossChunkImportRecords,
				Parts:         []js_ast.Part{{Stmts: repr.crossChunkPrefixStmts}},
			}, c.symbols, r, printOptions).JS
			crossChunkSuffix = js_printer.Print(js_ast.AST{
				Parts: []js_ast.Part{{Stmts: repr.crossChunkSuffixStmts}},
			}, c.symbols, r, printOptions).JS
		}

		waitGroup.Wait()

		j := js_printer.Joiner{}
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
		isExecutable := false

		if chunk.isEntryPoint {
			repr := c.files[chunk.sourceIndex].repr.(*reprJS)

			// Start with the hashbang if there is one
			if repr.ast.Hashbang != "" {
				hashbang := repr.ast.Hashbang + "\n"
				prevOffset.advanceString(hashbang)
				j.AddString(hashbang)
				newlineBeforeComment = true
				isExecutable = true
			}

			// Add the top-level directive if present
			if repr.ast.Directive != "" {
				quoted := string(js_printer.QuoteForJSON(repr.ast.Directive)) + ";" + newline
				prevOffset.advanceString(quoted)
				j.AddString(quoted)
				newlineBeforeComment = true
			}
		}

		// Optionally wrap with an IIFE
		if c.options.OutputFormat == config.FormatIIFE {
			var text string
			indent = "  "
			if c.options.UnsupportedFeatures.Has(compat.Arrow) {
				text = "(function()" + space + "{" + newline
			} else {
				text = "(()" + space + "=>" + space + "{" + newline
			}
			if c.options.ModuleName != "" {
				text = "var " + c.options.ModuleName + space + "=" + space + text
			}
			prevOffset.advanceString(text)
			j.AddString(text)
			newlineBeforeComment = false
		}

		// Put the cross-chunk prefix inside the IIFE
		if len(crossChunkPrefix) > 0 {
			newlineBeforeComment = true
			prevOffset.advanceBytes(crossChunkPrefix)
			j.AddBytes(crossChunkPrefix)
		}

		// Start the metadata
		jMeta := js_printer.Joiner{}
		if c.options.AbsMetadataFile != "" {
			isFirstMeta := true
			jMeta.AddString("{\n      \"imports\": [")
			for _, record := range crossChunkImportRecords {
				if isFirstMeta {
					isFirstMeta = false
				} else {
					jMeta.AddString(",")
				}
				importAbsPath := c.fs.Join(c.options.AbsOutputDir, chunk.relDir, record.Path.Text)
				jMeta.AddString(fmt.Sprintf("\n        {\n          \"path\": %s\n        }",
					js_printer.QuoteForJSON(c.res.PrettyPath(logger.Path{Text: importAbsPath, Namespace: "file"}))))
			}
			if !isFirstMeta {
				jMeta.AddString("\n      ")
			}
			jMeta.AddString("],\n      \"inputs\": {")
		}
		isFirstMeta := true

		// Concatenate the generated JavaScript chunks together
		var compileResultsForSourceMap []compileResultJS
		var entryPointTail *js_printer.PrintResult
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
			if c.options.Mode == config.ModeBundle && !c.options.RemoveWhitespace && !isRuntime {
				if newlineBeforeComment {
					prevOffset.advanceString("\n")
					j.AddString("\n")
				}

				text := fmt.Sprintf("%s// %s\n", indent, c.files[compileResult.sourceIndex].source.PrettyPath)
				prevOffset.advanceString(text)
				j.AddString(text)
			}

			// Omit the trailing semicolon when minifying the last file in IIFE mode
			if !isRuntime || len(compileResult.JS) > 0 {
				newlineBeforeComment = true
			}

			// Don't include the runtime in source maps
			if isRuntime {
				prevOffset.advanceString(string(compileResult.JS))
				j.AddBytes(compileResult.JS)
			} else {
				// Save the offset to the start of the stored JavaScript
				compileResult.generatedOffset = prevOffset
				j.AddBytes(compileResult.JS)

				// Ignore empty source map chunks
				if compileResult.SourceMapChunk.ShouldIgnore {
					prevOffset.advanceBytes(compileResult.JS)
				} else {
					prevOffset = lineColumnOffset{}

					// Include this file in the source map
					if c.options.SourceMap != config.SourceMapNone {
						compileResultsForSourceMap = append(compileResultsForSourceMap, compileResult)
					}
				}

				// Include this file in the metadata
				if c.options.AbsMetadataFile != "" {
					if isFirstMeta {
						isFirstMeta = false
					} else {
						jMeta.AddString(",")
					}
					jMeta.AddString(fmt.Sprintf("\n        %s: {\n          \"bytesInOutput\": %d\n        }",
						js_printer.QuoteForJSON(c.files[compileResult.sourceIndex].source.PrettyPath),
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
						"{\n      \"imports\": [],\n      \"inputs\": {},\n      \"bytes\": %d\n    }", len(sourceMap)))
				}

				// Figure out the base name for the source map which may include the content hash
				var sourceMapBaseName string
				if chunk.baseNameOrEmpty == "" {
					hash := hashForFileName(sourceMap)
					sourceMapBaseName = "chunk." + hash + c.options.OutputExtensionFor(".js") + ".map"
				} else {
					sourceMapBaseName = chunk.baseNameOrEmpty + ".map"
				}

				// Add a comment linking the source to its map
				if c.options.SourceMap == config.SourceMapLinkedWithComment {
					j.AddString("//# sourceMappingURL=")
					j.AddString(sourceMapBaseName)
					j.AddString("\n")
				}

				results = append(results, OutputFile{
					AbsPath:           c.fs.Join(c.options.AbsOutputDir, chunk.relDir, sourceMapBaseName),
					Contents:          sourceMap,
					jsonMetadataChunk: jsonMetadataChunk,
				})
			}
		}

		// The JavaScript contents are done now that the source map comment is in
		jsContents := j.Done()

		// Figure out the base name for this chunk now that the content hash is known
		if chunk.baseNameOrEmpty == "" {
			hash := hashForFileName(jsContents)
			chunk.baseNameOrEmpty = "chunk." + hash + c.options.OutputExtensionFor(".js")
		}

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
			AbsPath:           c.fs.Join(c.options.AbsOutputDir, chunk.relPath()),
			Contents:          jsContents,
			jsonMetadataChunk: jsonMetadataChunk,
			IsExecutable:      isExecutable,
		})
		return results
	}
}

type compileResultCSS struct {
	printedCSS  string
	sourceIndex uint32
}

func (repr *chunkReprCSS) generate(c *linkerContext, chunk *chunkInfo) func([]ast.ImportRecord) []OutputFile {
	var results []OutputFile
	filesInChunkInOrder := c.chunkFileOrder(chunk)
	compileResults := make([]compileResultCSS, 0, len(filesInChunkInOrder))

	// Generate CSS for each file in parallel
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
		compileResults = append(compileResults, compileResultCSS{})
		compileResult := &compileResults[len(compileResults)-1]
		waitGroup.Add(1)
		go func(sourceIndex uint32, compileResult *compileResultCSS) {
			file := &c.files[sourceIndex]
			repr := file.repr.(*reprCSS)
			css := css_printer.Print(repr.ast, css_printer.Options{
				Contents:         file.source.Contents,
				RemoveWhitespace: c.options.RemoveWhitespace,
			})
			*compileResult = compileResultCSS{
				printedCSS:  css,
				sourceIndex: sourceIndex,
			}
			waitGroup.Done()
		}(sourceIndex, compileResult)
	}

	// Wait for cross-chunk import records before continuing
	return func(crossChunkImportRecords []ast.ImportRecord) []OutputFile {
		waitGroup.Wait()
		j := js_printer.Joiner{}

		// Start the metadata
		jMeta := js_printer.Joiner{}
		if c.options.AbsMetadataFile != "" {
			isFirstMeta := true
			jMeta.AddString("{\n      \"imports\": [")
			for _, record := range crossChunkImportRecords {
				if isFirstMeta {
					isFirstMeta = false
				} else {
					jMeta.AddString(",")
				}
				importAbsPath := c.fs.Join(c.options.AbsOutputDir, chunk.relDir, record.Path.Text)
				jMeta.AddString(fmt.Sprintf("\n        {\n          \"path\": %s\n        }",
					js_printer.QuoteForJSON(c.res.PrettyPath(logger.Path{Text: importAbsPath, Namespace: "file"}))))
			}
			if !isFirstMeta {
				jMeta.AddString("\n      ")
			}
			jMeta.AddString("],\n      \"inputs\": {")
		}
		isFirstMeta := true

		// Concatenate the generated CSS chunks together
		newlineBeforeComment := false
		for _, compileResult := range compileResults {
			if c.options.Mode == config.ModeBundle && !c.options.RemoveWhitespace {
				if newlineBeforeComment {
					j.AddString("\n")
				}
				j.AddString(fmt.Sprintf("/* %s */\n", c.files[compileResult.sourceIndex].source.PrettyPath))
			}
			if len(compileResult.printedCSS) > 0 {
				newlineBeforeComment = true
			}
			j.AddString(compileResult.printedCSS)

			// Include this file in the metadata
			if c.options.AbsMetadataFile != "" {
				if isFirstMeta {
					isFirstMeta = false
				} else {
					jMeta.AddString(",")
				}
				jMeta.AddString(fmt.Sprintf("\n        %s: {\n          \"bytesInOutput\": %d\n        }",
					js_printer.QuoteForJSON(c.files[compileResult.sourceIndex].source.PrettyPath),
					len(compileResult.printedCSS)))
			}
		}

		// Make sure the file ends with a newline
		if j.Length() > 0 && j.LastByte() != '\n' {
			j.AddString("\n")
		}

		// The CSS contents are done now that the source map comment is in
		cssContents := j.Done()

		// Figure out the base name for this chunk now that the content hash is known
		if chunk.baseNameOrEmpty == "" {
			hash := hashForFileName(cssContents)
			chunk.baseNameOrEmpty = "chunk." + hash + c.options.OutputExtensionFor(".css")
		}

		// End the metadata
		var jsonMetadataChunk []byte
		if c.options.AbsMetadataFile != "" {
			if !isFirstMeta {
				jMeta.AddString("\n      ")
			}
			jMeta.AddString(fmt.Sprintf("},\n      \"bytes\": %d\n    }", len(cssContents)))
			jsonMetadataChunk = jMeta.Done()
		}

		results = append(results, OutputFile{
			AbsPath:           c.fs.Join(c.options.AbsOutputDir, chunk.relPath()),
			Contents:          cssContents,
			jsonMetadataChunk: jsonMetadataChunk,
		})
		return results
	}
}

func (offset *lineColumnOffset) advanceBytes(bytes []byte) {
	for i, n := 0, len(bytes); i < n; i++ {
		if bytes[i] == '\n' {
			offset.lines++
			offset.columns = 0
		} else {
			offset.columns++
		}
	}
}

func (offset *lineColumnOffset) advanceString(text string) {
	for i, n := 0, len(text); i < n; i++ {
		if text[i] == '\n' {
			offset.lines++
			offset.columns = 0
		} else {
			offset.columns++
		}
	}
}

func markBindingAsUnbound(binding js_ast.Binding, symbols js_ast.SymbolMap) {
	switch b := binding.Data.(type) {
	case *js_ast.BMissing:

	case *js_ast.BIdentifier:
		symbols.Get(b.Ref).Kind = js_ast.SymbolUnbound

	case *js_ast.BArray:
		for _, i := range b.Items {
			markBindingAsUnbound(i.Binding, symbols)
		}

	case *js_ast.BObject:
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
	repr, ok := c.files[sourceIndex].repr.(*reprJS)
	if !ok {
		return
	}
	hasImportOrExport := false

	for _, part := range repr.ast.Parts {
		for _, stmt := range part.Stmts {
			switch s := stmt.Data.(type) {
			case *js_ast.SImport:
				// Ignore imports from the internal runtime code. These are generated
				// automatically and aren't part of the original source code. We
				// shouldn't consider the file a module if the only ES6 import or
				// export is the automatically generated one.
				record := &repr.ast.ImportRecords[s.ImportRecordIndex]
				if record.SourceIndex != nil && *record.SourceIndex == runtime.SourceIndex {
					continue
				}

				hasImportOrExport = true

			case *js_ast.SLocal:
				if s.IsExport {
					for _, decl := range s.Decls {
						markBindingAsUnbound(decl.Binding, c.symbols)
					}
					hasImportOrExport = true
				}

			case *js_ast.SFunction:
				if s.IsExport {
					c.symbols.Get(s.Fn.Name.Ref).Kind = js_ast.SymbolUnbound
					hasImportOrExport = true
				}

			case *js_ast.SClass:
				if s.IsExport {
					c.symbols.Get(s.Class.Name.Ref).Kind = js_ast.SymbolUnbound
					hasImportOrExport = true
				}

			case *js_ast.SExportClause, *js_ast.SExportDefault, *js_ast.SExportStar:
				hasImportOrExport = true

			case *js_ast.SExportFrom:
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
		for _, member := range repr.ast.ModuleScope.Members {
			c.symbols.Get(member.Ref).Kind = js_ast.SymbolUnbound
		}
	}
}

func (c *linkerContext) generateSourceMapForChunk(results []compileResultJS) []byte {
	j := js_printer.Joiner{}
	j.AddString("{\n  \"version\": 3")

	// Write the sources
	j.AddString(",\n  \"sources\": [")
	needComma := false
	for _, result := range results {
		for _, source := range result.SourceMapChunk.QuotedSources {
			if needComma {
				j.AddString(", ")
			} else {
				needComma = true
			}
			j.AddBytes(source.QuotedPath)
		}
	}
	j.AddString("]")

	// Write the sourcesContent
	j.AddString(",\n  \"sourcesContent\": [")
	needComma = false
	for _, result := range results {
		for _, source := range result.SourceMapChunk.QuotedSources {
			if needComma {
				j.AddString(", ")
			} else {
				needComma = true
			}
			j.AddBytes(source.QuotedContents)
		}
	}
	j.AddString("]")

	// Write the mappings
	j.AddString(",\n  \"mappings\": \"")
	prevEndState := js_printer.SourceMapState{}
	prevColumnOffset := 0
	sourceMapIndex := 0
	for _, result := range results {
		chunk := result.SourceMapChunk
		offset := result.generatedOffset

		// This should have already been checked earlier
		if chunk.ShouldIgnore {
			panic("Internal error")
		}

		// Because each file for the bundle is converted to a source map once,
		// the source maps are shared between all entry points in the bundle.
		// The easiest way of getting this to work is to have all source maps
		// generate as if their source index is 0. We then adjust the source
		// index per entry point by modifying the first source mapping. This
		// is done by AppendSourceMapChunk() using the source index passed
		// here.
		startState := js_printer.SourceMapState{
			SourceIndex:     sourceMapIndex,
			GeneratedLine:   offset.lines,
			GeneratedColumn: offset.columns,
		}
		if offset.lines == 0 {
			startState.GeneratedColumn += prevColumnOffset
		}

		// Append the precomputed source map chunk
		js_printer.AppendSourceMapChunk(&j, prevEndState, startState, chunk.Buffer)

		// Generate the relative offset to start from next time
		prevEndState = chunk.EndState
		prevEndState.SourceIndex += sourceMapIndex
		prevColumnOffset = chunk.FinalGeneratedColumn

		// If this was all one line, include the column offset from the start
		if prevEndState.GeneratedLine == 0 {
			prevEndState.GeneratedColumn += startState.GeneratedColumn
			prevColumnOffset += startState.GeneratedColumn
		}

		sourceMapIndex += len(result.SourceMapChunk.QuotedSources)
	}
	j.AddString("\"")

	// Finish the source map
	j.AddString(",\n  \"names\": []\n}\n")
	return j.Done()
}
