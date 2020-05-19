package bundler

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"sync"

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
	hasErrors   bool

	// We should avoid traversing all files in the bundle, because the linker
	// should be able to run a linking operation on a large bundle where only
	// a few files are needed (e.g. an incremental compilation scenario). This
	// holds all files that could possibly be reached through the entry points.
	// If you need to iterate over all files in the linking operation, iterate
	// over this array. This array is also sorted in a deterministic ordering
	// to help ensure deterministic builds (source indices are random).
	reachableFiles []uint32
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

	// Re-exports happen because of "export * from" statements like this:
	//
	//   export * from 'path';
	//
	// Note that export stars with a namespace and are not considered re-exports:
	//
	//   export * as ns from 'path';
	//   export {a, b} from 'path';
	//
	// This is essentially the same as a star import followed by an export,
	// except of course that the namespace is never declared in the scope:
	//
	//   import * as ns from 'path';
	//   export {ns};
	//
	resolvedExportStars map[string]exportStarData
}

type exportStarData struct {
	ast.NamedExport

	// The location of the path string for error messages
	pathLoc ast.Loc

	// This is the file that the named export above came from
	sourceIndex uint32

	// If export star resolution finds two or more symbols with the same name,
	// then the name is a ambiguous and cannot be used. This causes the name to
	// be silently omitted from any namespace imports and causes a compile-time
	// failure if the name is used in an ES6 import statement.
	isAmbiguous bool
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
	name                  string
	hashbang              string
	filesWithPartsInChunk map[uint32]bool
	entryBits             bitSet
}

func newLinkerContext(options *BundleOptions, log logging.Log, fs fs.FS, sources []logging.Source, files []file, entryPoints []uint32) linkerContext {
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
		for i, _ := range fileSymbols {
			fileSymbols[i].UseCountEstimate = 0
		}

		// Clone the parts
		file.ast.Parts = append([]ast.Part{}, file.ast.Parts...)

		// Clone the import map
		namedImports := make(map[ast.Ref]ast.NamedImport, len(file.ast.NamedImports))
		for k, v := range file.ast.NamedImports {
			namedImports[k] = v
		}
		file.ast.NamedImports = namedImports

		// Clone the export map
		namedExports := make(map[string]ast.NamedExport, len(file.ast.NamedExports))
		for k, v := range file.ast.NamedExports {
			namedExports[k] = v
		}
		file.ast.NamedExports = namedExports

		// Update the file in our copy of the file array
		c.files[sourceIndex] = file

		// Also associate some default metadata with the file
		c.fileMeta[sourceIndex] = fileMeta{
			distanceFromEntryPoint: ^uint32(0),
			isCommonJS:             file.ast.UsesCommonJSFeatures,
			partMeta:               make([]partMeta, len(file.ast.Parts)),
			resolvedExportStars:    make(map[string]exportStarData),
		}
	}

	// Mark all entry points so we don't add them again for import() expressions
	for _, sourceIndex := range entryPoints {
		c.fileMeta[sourceIndex].entryPointStatus = entryPointUserSpecified
	}

	return c
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
				for _, importPath := range part.ImportPaths {
					if otherSourceIndex, ok := file.resolveImport(importPath.Path); ok {
						visit(otherSourceIndex)
					}
				}
			}
			sorted = append(sorted, indexAndPath{sourceIndex, sources[sourceIndex].AbsolutePath})
		}
	}

	// The runtime is always included in case it's needed
	visit(ast.RuntimeSourceIndex)

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
	c.log.AddRangeError(source, r, text)
	c.hasErrors = true
}

func (c *linkerContext) link() []BundleResult {
	c.scanImportsAndExports()

	// Stop now if there were errors
	if c.hasErrors {
		return []BundleResult{}
	}

	c.markPartsReachableFromEntryPoints()

	if !c.options.IsBundling {
		for _, entryPoint := range c.entryPoints {
			c.markExportsAsUnbound(entryPoint)
		}
	}

	c.renameOrMinifyAllSymbols()

	chunks := c.computeChunks()
	results := make([]BundleResult, 0, len(chunks))

	for _, chunk := range chunks {
		results = append(results, c.generateChunk(chunk))
	}

	return results
}

func (c *linkerContext) scanImportsAndExports() {
	// Step 1: Figure out what modules must be CommonJS
	for _, sourceIndex := range c.reachableFiles {
		file := c.files[sourceIndex]
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
	}

	// Step 2: Resolve "export * from" statements. This must be done after we
	// discover all modules that can be CommonJS because export stars are ignored
	// for CommonJS modules.
	for _, sourceIndex := range c.reachableFiles {
		file := c.files[sourceIndex]
		if len(file.ast.ExportStars) > 0 {
			visited := make(map[uint32]bool)
			c.addExportsForExportStar(c.fileMeta[sourceIndex].resolvedExportStars, uint32(sourceIndex), nil, visited)
		}
	}

	// Step 3: Create star exports for every file. This is always necessary for
	// CommonJS files, and is also necessary for other files if they are imported
	// using an import star statement.
	for _, sourceIndex := range c.reachableFiles {
		c.createStarExportForFile(uint32(sourceIndex))
	}

	// Step 4: Bind imports to exports. This must be done after we process all
	// export stars because imports can bind to export star re-exports.
	for _, sourceIndex := range c.reachableFiles {
		file := c.files[sourceIndex]
		if len(file.ast.NamedImports) > 0 {
			c.bindImportsToExportsForFile(uint32(sourceIndex))
		}
	}
}

func (c *linkerContext) createStarExportForFile(sourceIndex uint32) {
	fileMeta := &c.fileMeta[sourceIndex]
	file := &c.files[sourceIndex]
	exportPartIndex := uint32(len(file.ast.Parts))

	// Sort the imports for determinism
	aliases := make([]string, 0, len(file.ast.NamedExports)+len(fileMeta.resolvedExportStars))
	for alias, _ := range file.ast.NamedExports {
		aliases = append(aliases, alias)
	}
	for alias, export := range fileMeta.resolvedExportStars {
		// Make sure not to add re-exports that are shadowed due to an export.
		// Also don't add ambiguous re-exports, since they are silently hidden.
		if _, ok := file.ast.NamedExports[alias]; !ok && !export.isAmbiguous {
			aliases = append(aliases, alias)
		}
	}
	sort.Strings(aliases)

	// Generate a getter per export
	properties := []ast.Property{}
	nonLocalDependencies := []partRef{}
	useCountEstimates := make(map[ast.Ref]uint32)
	for _, alias := range aliases {
		var otherSourceIndex uint32
		var exportStarPathLoc ast.Loc

		// Look up the alias
		export, ok := file.ast.NamedExports[alias]
		if ok {
			otherSourceIndex = uint32(sourceIndex)
		} else {
			exportStar := fileMeta.resolvedExportStars[alias]
			exportStarPathLoc = exportStar.pathLoc
			otherSourceIndex = exportStar.sourceIndex
			export = exportStar.NamedExport
		}

		// Exports of imports need EImportIdentifier in case they need to be re-
		// written to a property access later on
		var value ast.Expr
		otherFile := &c.files[otherSourceIndex]
		if _, ok := otherFile.ast.NamedImports[export.Ref]; ok {
			value = ast.Expr{ast.Loc{}, &ast.EImportIdentifier{export.Ref}}

			// Mark that the import is used by the part we're about to generate.
			// That way when the import is later bound to its matching export,
			// the export will be assigned as a dependency of the part we're about
			// to generate.
			namedImport, ok := file.ast.NamedImports[export.Ref]
			if !ok {
				// The import may be in another file because of an export star, but
				// we want to add the dependency to the original file since the
				// dependency propagation uses local part references. In that case,
				// add a new import to the original file.
				namedImport = ast.NamedImport{
					Alias:        alias,
					AliasLoc:     exportStarPathLoc,
					NamespaceRef: ast.InvalidRef,
					ImportPath: ast.Path{
						UseSourceIndex: true,
						SourceIndex:    otherSourceIndex,
					},
				}
			}
			clone := append(make([]uint32, 0, len(namedImport.LocalPartsWithUses)+1), namedImport.LocalPartsWithUses...)
			namedImport.LocalPartsWithUses = append(clone, exportPartIndex)
			file.ast.NamedImports[export.Ref] = namedImport
		} else {
			value = ast.Expr{ast.Loc{}, &ast.EIdentifier{export.Ref}}
		}

		// Add a getter property
		properties = append(properties, ast.Property{
			Key: ast.Expr{ast.Loc{}, &ast.EString{lexer.StringToUTF16(alias)}},
			Value: &ast.Expr{ast.Loc{}, &ast.EArrow{
				PreferExpr: true,
				Body:       ast.FnBody{Stmts: []ast.Stmt{ast.Stmt{value.Loc, &ast.SReturn{&value}}}},
			}},
		})
		useCountEstimates[export.Ref] = useCountEstimates[export.Ref] + 1

		// Make sure the part that declares the export is included
		for _, partIndex := range export.LocalParts {
			// Use a non-local dependency since this is likely from a different
			// file if it came in through an export star
			nonLocalDependencies = append(nonLocalDependencies, partRef{
				sourceIndex: otherSourceIndex,
				partIndex:   partIndex,
			})
		}
	}

	// Prefix this part with "var exports = {}" if this isn't a CommonJS module
	declaredSymbols := []ast.DeclaredSymbol{}
	stmts := make([]ast.Stmt, 0, 2)
	if !fileMeta.isCommonJS {
		stmts = append(stmts, ast.Stmt{ast.Loc{}, &ast.SLocal{Kind: ast.LocalConst, Decls: []ast.Decl{ast.Decl{
			Binding: ast.Binding{ast.Loc{}, &ast.BIdentifier{file.ast.ExportsRef}},
			Value:   &ast.Expr{ast.Loc{}, &ast.EObject{}},
		}}}})
		declaredSymbols = append(declaredSymbols, ast.DeclaredSymbol{
			Ref:        file.ast.ExportsRef,
			IsTopLevel: true,
		})
	}

	// "__export(exports, { foo: () => foo })"
	if len(properties) > 0 {
		exportRef := c.files[ast.RuntimeSourceIndex].ast.ModuleScope.Members["__export"]
		stmts = append(stmts, ast.Stmt{ast.Loc{}, &ast.SExpr{ast.Expr{ast.Loc{}, &ast.ECall{
			Target: ast.Expr{ast.Loc{}, &ast.EIdentifier{exportRef}},
			Args: []ast.Expr{
				ast.Expr{ast.Loc{}, &ast.EIdentifier{file.ast.ExportsRef}},
				ast.Expr{ast.Loc{}, &ast.EObject{properties}},
			},
		}}}})
		useCountEstimates[exportRef] = useCountEstimates[exportRef] + 1

		// Make sure this file depends on the "__export" symbol
		for _, partIndex := range c.files[ast.RuntimeSourceIndex].ast.NamedExports["__export"].LocalParts {
			nonLocalDependencies = append(nonLocalDependencies, partRef{
				sourceIndex: ast.RuntimeSourceIndex,
				partIndex:   partIndex,
			})
		}

		// Make sure the CommonJS closure, if there is one, includes "exports"
		file.ast.UsesExportsRef = true
	}

	// No need to generate a part if it'll be empty
	if len(stmts) == 0 {
		return
	}

	// Clone the parts array to avoid mutating the original AST
	file.ast.Parts = append(file.ast.Parts, ast.Part{
		Stmts:             stmts,
		LocalDependencies: make(map[uint32]bool),
		UseCountEstimates: useCountEstimates,
		DeclaredSymbols:   declaredSymbols,

		// This can be removed if nothing uses it. Except if we're a CommonJS
		// module, in which case it's always necessary.
		CanBeRemovedIfUnused: !fileMeta.isCommonJS,

		// Put the export definitions first before anything else gets evaluated
		ShouldComeFirst: true,

		// Make sure this is trimmed if unused even if tree shaking is disabled
		ForceTreeShaking: true,
	})

	// Make sure the "partMeta" array matches the "Parts" array
	fileMeta.partMeta = append(fileMeta.partMeta, partMeta{
		entryBits:            newBitSet(uint(len(c.entryPoints))),
		nonLocalDependencies: nonLocalDependencies,
	})

	// Add a special export called "*"
	if !fileMeta.isCommonJS {
		file.ast.NamedExports["*"] = ast.NamedExport{
			Ref:        file.ast.ExportsRef,
			LocalParts: []uint32{exportPartIndex},
		}
	}
}

func (c *linkerContext) bindImportsToExportsForFile(sourceIndex uint32) {
	file := c.files[sourceIndex]

	// Sort imports for determinism. Otherwise our unit tests will randomly
	// fail sometimes when error messages are reordered.
	sortedImportRefs := make([]int, 0, len(file.ast.NamedImports))
	for ref, _ := range file.ast.NamedImports {
		sortedImportRefs = append(sortedImportRefs, int(ref.InnerIndex))
	}
	sort.Ints(sortedImportRefs)

	// Bind imports with their matching exports
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
					r := lexer.RangeOfIdentifier(source, namedImport.AliasLoc)
					c.addRangeError(source, r, fmt.Sprintf("Detected cycle while resolving import %q", namedImport.Alias))
					break
				}
				cycleDetector, _, _ = c.advanceImportTracker(cycleDetector)
			}

			// Resolve the import by one step
			nextTracker, localParts, status := c.advanceImportTracker(tracker)
			if status == importCommonJS || status == importExternal {
				// If it's a CommonJS or external file, rewrite the import to a property access
				namedImport := c.files[tracker.sourceIndex].ast.NamedImports[tracker.importRef]
				c.symbols.Get(importRef).NamespaceAlias = &ast.NamespaceAlias{
					NamespaceRef: namedImport.NamespaceRef,
					Alias:        namedImport.Alias,
				}
				break
			} else if status == importNoMatch {
				// Report mismatched imports and exports
				source := c.sources[tracker.sourceIndex]
				namedImport := c.files[tracker.sourceIndex].ast.NamedImports[tracker.importRef]
				r := lexer.RangeOfIdentifier(source, namedImport.AliasLoc)
				c.addRangeError(source, r, fmt.Sprintf("No matching export for import %q", namedImport.Alias))
				break
			} else if status == importAmbiguous {
				source := c.sources[tracker.sourceIndex]
				namedImport := c.files[tracker.sourceIndex].ast.NamedImports[tracker.importRef]
				r := lexer.RangeOfIdentifier(source, namedImport.AliasLoc)
				c.addRangeError(source, r, fmt.Sprintf("Ambiguous import %q has multiple matching exports", namedImport.Alias))
				break
			} else if _, ok := c.files[nextTracker.sourceIndex].ast.NamedImports[nextTracker.importRef]; !ok {
				// If this is not a re-export of another import, add this import as
				// a dependency to all parts in this file that use this import
				for _, partIndex := range c.files[sourceIndex].ast.NamedImports[importRef].LocalPartsWithUses {
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

func (c *linkerContext) addExportsForExportStar(
	resolvedExportStars map[string]exportStarData,
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

	for _, path := range file.ast.ExportStars {
		if otherSourceIndex, ok := file.resolveImport(path); ok {
			// We need a location for error messages, but it must be in the top-level
			// file, not in any nested file. This will be passed to nested files.
			pathLoc := path.Loc
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
			if c.fileMeta[otherSourceIndex].isCommonJS {
				continue
			}

			// Accumulate this file's exports
			for name, export := range c.files[otherSourceIndex].ast.NamedExports {
				if existing, ok := resolvedExportStars[name]; ok && existing.sourceIndex != otherSourceIndex {
					existing.isAmbiguous = true
					resolvedExportStars[name] = existing
				} else {
					resolvedExportStars[name] = exportStarData{
						NamedExport: export,
						sourceIndex: otherSourceIndex,
						pathLoc:     pathLoc,
					}
				}
			}

			// Search further through this file's export stars
			c.addExportsForExportStar(resolvedExportStars, otherSourceIndex, &pathLoc, visited)
		}
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

	// The imported file is external and has unknown exports
	importExternal

	// There are multiple re-exports with the same name due to "export * from"
	importAmbiguous
)

func (c *linkerContext) advanceImportTracker(tracker importTracker) (importTracker, []uint32, importStatus) {
	file := &c.files[tracker.sourceIndex]
	namedImport := file.ast.NamedImports[tracker.importRef]

	// Is this an external file?
	otherSourceIndex, ok := file.resolveImport(namedImport.ImportPath)
	if !ok {
		return importTracker{}, nil, importExternal
	}

	// Is this a CommonJS file?
	otherFileMeta := &c.fileMeta[otherSourceIndex]
	if otherFileMeta.isCommonJS {
		return importTracker{}, nil, importCommonJS
	}

	// Match this import up with an export from the imported file
	otherFile := &c.files[otherSourceIndex]
	if matchingExport, ok := otherFile.ast.NamedExports[namedImport.Alias]; ok {
		// Check to see if this is a re-export of another import
		return importTracker{otherSourceIndex, matchingExport.Ref}, matchingExport.LocalParts, importFound
	}

	// If there was no named export, there may still be an export star
	if matchingExport, ok := c.fileMeta[otherSourceIndex].resolvedExportStars[namedImport.Alias]; ok {
		if matchingExport.isAmbiguous {
			return importTracker{}, nil, importAmbiguous
		}

		// Check to see if this is a re-export of another import
		return importTracker{matchingExport.sourceIndex, matchingExport.Ref}, matchingExport.LocalParts, importFound
	}

	return importTracker{}, nil, importNoMatch

}

func (c *linkerContext) markPartsReachableFromEntryPoints() {
	// Allocate bit sets
	bitCount := uint(len(c.entryPoints))
	for _, sourceIndex := range c.reachableFiles {
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

func (c *linkerContext) accumulateSymbolCount(ref ast.Ref, count uint32) {
	ref = ast.FollowSymbols(c.symbols, ref)
	c.symbols.Get(ref).UseCountEstimate += count
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

	// Accumulate symbol usage counts
	file := &c.files[sourceIndex]
	if file.ast.UsesExportsRef {
		c.accumulateSymbolCount(file.ast.ExportsRef, 1)
	}
	if file.ast.UsesModuleRef {
		c.accumulateSymbolCount(file.ast.ModuleRef, 1)
	}
	if fileMeta.isCommonJS {
		c.accumulateSymbolCount(file.ast.WrapperRef, 1)
	}

	needsToModule := false
	for partIndex, part := range file.ast.Parts {
		// Include all parts in this file with side effects, or just include
		// everything if tree-shaking is disabled. Note that we still want to
		// perform tree-shaking on the runtime even if tree-shaking is disabled.
		if !part.CanBeRemovedIfUnused || (!part.ForceTreeShaking && !c.options.TreeShaking && sourceIndex != ast.RuntimeSourceIndex) {
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
		c.includePartsForRuntimeSymbol("__commonJS", entryPoint, distanceFromEntryPoint)
	}

	// If there's an ES6 import of a non-ES6 module, then we're going to need the
	// "__toModule" symbol from the runtime
	if needsToModule {
		c.includePartsForRuntimeSymbol("__toModule", entryPoint, distanceFromEntryPoint)
	}
}

func (c *linkerContext) includePartsForRuntimeSymbol(name string, entryPoint uint, distanceFromEntryPoint uint32) {
	export := c.files[ast.RuntimeSourceIndex].ast.NamedExports[name]
	c.accumulateSymbolCount(export.Ref, 1)
	for _, partIndex := range export.LocalParts {
		c.includePart(ast.RuntimeSourceIndex, partIndex, entryPoint, distanceFromEntryPoint)
	}
}

func (c *linkerContext) includePart(sourceIndex uint32, partIndex uint32, entryPoint uint, distanceFromEntryPoint uint32) {
	partMeta := &c.fileMeta[sourceIndex].partMeta[partIndex]

	// Don't mark this part more than once
	if partMeta.entryBits.hasBit(entryPoint) {
		return
	}
	partMeta.entryBits.setBit(entryPoint)

	// Accumulate symbol usage counts
	file := &c.files[sourceIndex]
	part := &file.ast.Parts[partIndex]
	for ref, count := range part.UseCountEstimates {
		c.accumulateSymbolCount(ref, count)
	}
	for _, declared := range part.DeclaredSymbols {
		// Make sure to also count the declaration in addition to the uses
		c.accumulateSymbolCount(declared.Ref, 1)
	}

	// Also include any require() imports
	for _, importPath := range part.ImportPaths {
		if importPath.Kind == ast.ImportRequire {
			if otherSourceIndex, ok := file.resolveImport(importPath.Path); ok {
				c.accumulateSymbolCount(c.files[otherSourceIndex].ast.WrapperRef, 1)
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

func (c *linkerContext) computeChunks() []chunkMeta {
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
				entryPointCount := 0
				for i, entryPoint := range c.entryPoints {
					if partMeta.entryBits.hasBit(uint(entryPoint)) {
						if chunk.name != "" {
							chunk.name = c.stripKnownFileExtension(chunk.name) + "_"
						}
						chunk.hashbang = c.files[entryPoint].ast.Hashbang
						chunk.name += entryPointNames[i]
						entryPointCount++
					}
				}
				if entryPointCount > 1 {
					chunk.hashbang = ""
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
	for key, _ := range chunks {
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
	sorted := make(chunkOrderArray, 0, len(chunk.filesWithPartsInChunk))

	// Attach information to the files for use with sorting
	for sourceIndex, _ := range chunk.filesWithPartsInChunk {
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
	visit(ast.RuntimeSourceIndex)
	for _, data := range sorted {
		visit(data.sourceIndex)
	}
	return order
}

func (c *linkerContext) convertStmtsForChunk(sourceIndex uint32, stmtList *stmtList, partStmts []ast.Stmt) {
	shouldStripExports := c.options.IsBundling || sourceIndex == ast.RuntimeSourceIndex

	for _, stmt := range partStmts {
		switch s := stmt.Data.(type) {
		case *ast.SImport:
			// Turn imports of CommonJS files into require() calls
			if otherSourceIndex, ok := c.files[sourceIndex].resolveImport(s.Path); ok {
				if c.fileMeta[otherSourceIndex].isCommonJS {
					stmtList.importStmts = append(stmtList.importStmts, ast.Stmt{
						Loc: stmt.Loc,
						Data: &ast.SLocal{Kind: ast.LocalConst, Decls: []ast.Decl{ast.Decl{
							ast.Binding{stmt.Loc, &ast.BIdentifier{s.NamespaceRef}},
							&ast.Expr{s.Path.Loc, &ast.ERequire{Path: s.Path, IsES6Import: true}},
						}}},
					})
				}

				// Remove import statements entirely
				continue
			}

		case *ast.SExportStar, *ast.SExportFrom, *ast.SExportClause:
			if shouldStripExports {
				// Remove export statements entirely
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
							// Be careful to not modify the original statement
							s2 = &ast.SFunction{Fn: s2.Fn}
							s2.Fn.Name = &s.DefaultName
						}
						stmt = ast.Stmt{s.Value.Stmt.Loc, s2}

					case *ast.SClass:
						// "export default class {}" => "class default {}"
						// "export default class Foo {}" => "class Foo {}"
						if s2.Class.Name == nil {
							// Be careful to not modify the original statement
							s2 = &ast.SClass{Class: s2.Class}
							s2.Class.Name = &s.DefaultName
						}
						stmt = ast.Stmt{s.Value.Stmt.Loc, s2}

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
	importStmts []ast.Stmt
	normalStmts []ast.Stmt
}

func (c *linkerContext) generateCodeForFileInChunk(
	waitGroup *sync.WaitGroup,
	sourceIndex uint32,
	entryBits bitSet,
	commonJSRef ast.Ref,
	toModuleRef ast.Ref,
	wrapperRefs []ast.Ref,
	result *compileResult,
) {
	file := &c.files[sourceIndex]
	fileMeta := &c.fileMeta[sourceIndex]
	stmts := []ast.Stmt{}

	// Add all parts in this file that belong in this chunk. Make sure to move
	// all parts marked "ShouldComeFirst" up to the front. These are generated
	// parts that are supposed to be a prefix for the file.
	{
		stmtList := stmtList{}
		parts := file.ast.Parts
		split := len(parts)
		for split > 0 && parts[split-1].ShouldComeFirst {
			split--
		}

		// Everything with "ShouldComeFirst"
		for partIndex := split; partIndex < len(parts); partIndex++ {
			if entryBits.equals(fileMeta.partMeta[partIndex].entryBits) {
				c.convertStmtsForChunk(sourceIndex, &stmtList, parts[partIndex].Stmts)
			}
		}

		// Everything else
		for partIndex, part := range parts[:split] {
			if entryBits.equals(fileMeta.partMeta[partIndex].entryBits) {
				c.convertStmtsForChunk(sourceIndex, &stmtList, part.Stmts)
			}
		}

		// Hoist all import statements before any normal statements. ES6 imports
		// are different than CommonJS imports. All modules imported via ES6 import
		// statements are evaluated before the module doing the importing is
		// evaluated (well, except for cyclic import scenarios). We need to preserve
		// these semantics even when modules imported via ES6 import statements end
		// up being CommonJS modules.
		stmts = stmtList.normalStmts
		if len(stmtList.importStmts) > 0 {
			stmts = append(stmtList.importStmts, stmts...)
		}
		if c.options.MangleSyntax {
			stmts = mergeAdjacentLocalStmts(stmts)
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

		// "__commonJS((exports, module) => { ... })"
		value := ast.Expr{Data: &ast.ECall{
			Target: ast.Expr{Data: &ast.EIdentifier{commonJSRef}},
			Args:   []ast.Expr{ast.Expr{Data: &ast.EArrow{Args: args, Body: ast.FnBody{Stmts: stmts}}}},
		}}

		// Make sure that entry points are immediately evaluated
		switch fileMeta.entryPointStatus {
		case entryPointNone:
			// "var require_foo = __commonJS((exports, module) => { ... });"
			stmts = []ast.Stmt{ast.Stmt{Data: &ast.SLocal{
				Decls: []ast.Decl{ast.Decl{
					Binding: ast.Binding{Data: &ast.BIdentifier{wrapperRef}},
					Value:   &value,
				}},
			}}}

		case entryPointUserSpecified:
			// "__commonJS((exports, module) => { ... })();"
			stmts = []ast.Stmt{ast.Stmt{Data: &ast.SExpr{ast.Expr{Data: &ast.ECall{Target: value}}}}}

		case entryPointDynamicImport:
			// "var require_foo = __commonJS((exports, module) => { ... }); require_foo();"
			stmts = []ast.Stmt{
				ast.Stmt{Data: &ast.SLocal{
					Decls: []ast.Decl{ast.Decl{
						Binding: ast.Binding{Data: &ast.BIdentifier{wrapperRef}},
						Value:   &value,
					}},
				}},
				ast.Stmt{Data: &ast.SExpr{ast.Expr{Data: &ast.ECall{
					Target: ast.Expr{Data: &ast.EIdentifier{wrapperRef}},
				}}}},
			}
		}
	}

	// Only generate a source map if needed
	sourceMapContents := &c.sources[sourceIndex].Contents
	if c.options.SourceMap == SourceMapNone {
		sourceMapContents = nil
	}

	// Convert the AST to JavaScript code
	tree := file.ast
	tree.Parts = []ast.Part{ast.Part{Stmts: stmts}}
	*result = compileResult{
		PrintResult: printer.Print(tree, printer.PrintOptions{
			RemoveWhitespace:  c.options.RemoveWhitespace,
			ResolvedImports:   file.resolvedImports,
			ToModuleRef:       toModuleRef,
			WrapperRefs:       wrapperRefs,
			SourceMapContents: sourceMapContents,
		}),
		sourceIndex: sourceIndex,
	}

	// Also quote the source for the source map while we're running in parallel
	if c.options.SourceMap != SourceMapNone {
		result.quotedSource = printer.QuoteForJSON(c.sources[sourceIndex].Contents)
	}

	waitGroup.Done()
}

func (c *linkerContext) generateChunk(chunk chunkMeta) BundleResult {
	filesInChunkInOrder := c.chunkFileOrder(chunk)
	compileResults := make([]compileResult, 0, len(filesInChunkInOrder))
	runtimeMembers := c.files[ast.RuntimeSourceIndex].ast.ModuleScope.Members
	commonJSRef := ast.FollowSymbols(c.symbols, runtimeMembers["__commonJS"])
	toModuleRef := ast.FollowSymbols(c.symbols, runtimeMembers["__toModule"])

	// Make sure the printer can require() CommonJS modules
	wrapperRefs := make([]ast.Ref, len(c.files))
	for _, sourceIndex := range c.reachableFiles {
		wrapperRefs[sourceIndex] = c.files[sourceIndex].ast.WrapperRef
	}

	// Generate JavaScript for each file in parallel
	waitGroup := sync.WaitGroup{}
	for _, sourceIndex := range filesInChunkInOrder {
		// Skip the runtime in test output
		if sourceIndex == ast.RuntimeSourceIndex && c.options.omitRuntimeForTests {
			continue
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
			wrapperRefs,
			compileResult,
		)
	}
	waitGroup.Wait()

	// Start with the hashbang if there is one
	js := []byte{}
	if chunk.hashbang != "" {
		js = append(js, chunk.hashbang...)
		js = append(js, '\n')
	}

	// Concatenate the generated JavaScript chunks together
	prevOffset := 0
	for _, compileResult := range compileResults {
		if c.options.IsBundling && !c.options.RemoveWhitespace && compileResult.sourceIndex != ast.RuntimeSourceIndex {
			if len(js) > 0 {
				js = append(js, '\n')
			}
			js = append(js, fmt.Sprintf("// %s\n", c.sources[compileResult.sourceIndex].PrettyPath)...)
		}

		// Save the offset to the start of the stored JavaScript
		compileResult.generatedOffset = computeLineColumnOffset(js[prevOffset:])
		js = append(js, compileResult.JS...)
		prevOffset = len(js)
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
		file := c.files[sourceIndex]
		moduleScopes = append(moduleScopes, file.ast.ModuleScope)
		if c.fileMeta[sourceIndex].isCommonJS {
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
			name := ast.GenerateNonUniqueNameFromPath(c.sources[sourceIndex].AbsolutePath)
			c.symbols.Get(file.ast.ExportsRef).Name = name + "_exports"
			c.symbols.Get(file.ast.ModuleRef).Name = name + "_module"
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
