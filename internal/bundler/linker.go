package bundler

import (
	"fmt"
	"sort"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/lexer"
	"github.com/evanw/esbuild/internal/logging"
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

type linkerContext struct {
	log         logging.Log
	symbols     ast.SymbolMap
	entryPoints []uint32
	sources     []logging.Source
	files       []file
	fileMeta    []fileMeta
}

type fileMeta struct {
	partMeta []partMeta

	// True if this is a user-specified entry point or the target of an import()
	isEntryPoint bool

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
	reachableFromEntryPoints bitSet
}

type partMeta struct {
	// This holds all entry points that can reach this part. It will be used to
	// assign this part to a chunk.
	reachableFromEntryPoints bitSet

	// These are dependencies that come from other files via import statements.
	nonLocalDependencies []partRef
}

type partRef struct {
	sourceIndex uint32
	partIndex   uint32
}

type chunkMeta struct {
	filesInChunk map[uint32]bool
}

func newLinkerContext(log logging.Log, sources []logging.Source, files []file, entryPoints []uint32) linkerContext {
	// Clone information about symbols and files so we don't mutate the input data
	c := linkerContext{
		log:         log,
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
		c.fileMeta[sourceIndex].isEntryPoint = true
	}

	return c
}

func (c *linkerContext) link() {
	c.scanImportsAndExports()
	c.markPartsReachableFromEntryPoints()
	chunks := c.computeChunkAssignments()
	chunks = chunks
}

func (c *linkerContext) scanImportsAndExports() {
	for sourceIndex, file := range c.files {
		for _, part := range file.ast.Parts {
			// Handle require() and import()
			for _, importPath := range part.ImportPaths {
				switch importPath.Kind {
				case ast.ImportRequire:
					// Files that are imported with require() must be CommonJS modules
					if otherSourceIndex, ok := file.resolvedImports[importPath.Path.Text]; ok {
						c.fileMeta[otherSourceIndex].isCommonJS = true
					}

				case ast.ImportDynamic:
					// Files that are imported with import() must be entry points
					if otherSourceIndex, ok := file.resolvedImports[importPath.Path.Text]; ok {
						if !c.fileMeta[otherSourceIndex].isEntryPoint {
							c.entryPoints = append(c.entryPoints, otherSourceIndex)
							c.fileMeta[otherSourceIndex].isEntryPoint = true
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
					tracker = nextTracker
					namedImport := c.files[tracker.sourceIndex].ast.NamedImports[tracker.importRef]

					if status == importCommonJS {
						// If it's a CommonJS file, rewrite the import to a property access
						c.symbols.SetNamespaceAlias(importRef, ast.NamespaceAlias{
							NamespaceRef: namedImport.NamespaceRef,
							Alias:        namedImport.Alias,
						})
						break
					} else if status == importMissing {
						// Report mismatched imports and exports
						source := c.sources[tracker.sourceIndex]
						r := lexer.RangeOfIdentifier(source, namedImport.AliasLoc)
						c.log.AddRangeError(source, r, fmt.Sprintf("No matching export for import %q", namedImport.Alias))
						break
					} else if _, ok := c.files[tracker.sourceIndex].ast.NamedImports[tracker.importRef]; !ok {
						// If this is not a re-export of another import, add this import as
						// a dependency to all parts in this file that use this import
						for _, partIndex := range namedImport.PartsWithUses {
							partMeta := &c.fileMeta[sourceIndex].partMeta[partIndex]
							for _, nonLocalPartIndex := range localParts {
								partMeta.nonLocalDependencies = append(partMeta.nonLocalDependencies, partRef{
									sourceIndex: tracker.sourceIndex,
									partIndex:   nonLocalPartIndex,
								})
							}
						}
						break
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
	otherSourceIndex, ok := file.resolvedImports[namedImport.ImportPath]
	if !ok || c.fileMeta[otherSourceIndex].isCommonJS {
		return tracker, nil, importCommonJS
	}

	// Match this import up with an export from the imported file
	otherFile := &c.files[otherSourceIndex]
	matchingExport, ok := otherFile.ast.NamedExports[namedImport.Alias]
	if !ok {
		return tracker, nil, importMissing
	}

	// Check to see if this is a re-export of another import
	return importTracker{otherSourceIndex, matchingExport.Ref}, matchingExport.LocalParts, importFound
}

func (c *linkerContext) markPartsReachableFromEntryPoints() {
	// Allocate bit sets
	bitCount := uint(len(c.entryPoints))
	for sourceIndex, _ := range c.fileMeta {
		fileMeta := &c.fileMeta[sourceIndex]
		fileMeta.reachableFromEntryPoints = newBitSet(bitCount)
		for partIndex, _ := range fileMeta.partMeta {
			fileMeta.partMeta[partIndex].reachableFromEntryPoints = newBitSet(bitCount)
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
	if fileMeta.reachableFromEntryPoints.hasBit(entryPoint) {
		return
	}
	fileMeta.reachableFromEntryPoints.setBit(entryPoint)

	file := &c.files[sourceIndex]
	for partIndex, part := range file.ast.Parts {
		// Include all parts in this file with side effects
		if !part.CanBeRemovedIfUnused {
			c.includePart(sourceIndex, uint32(partIndex), entryPoint, distanceFromEntryPoint)
		}

		// Also include any statement-level imports
		for _, importPath := range part.ImportPaths {
			if importPath.Kind == ast.ImportStmt {
				if otherSourceIndex, ok := file.resolvedImports[importPath.Path.Text]; ok {
					c.includeFile(otherSourceIndex, entryPoint, distanceFromEntryPoint)
				}
			}
		}
	}
}

func (c *linkerContext) includePart(sourceIndex uint32, partIndex uint32, entryPoint uint, distanceFromEntryPoint uint32) {
	partMeta := &c.fileMeta[sourceIndex].partMeta[partIndex]

	// Don't mark this part more than once
	if partMeta.reachableFromEntryPoints.hasBit(entryPoint) {
		return
	}
	partMeta.reachableFromEntryPoints.setBit(entryPoint)

	// Also include any require() imports
	file := &c.files[sourceIndex]
	part := &file.ast.Parts[partIndex]
	for _, importPath := range part.ImportPaths {
		if importPath.Kind == ast.ImportRequire {
			if otherSourceIndex, ok := file.resolvedImports[importPath.Path.Text]; ok {
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

func (c *linkerContext) computeChunkAssignments() []chunkMeta {
	chunks := make(map[string]chunkMeta)
	neverReachedKey := string(newBitSet(uint(len(c.entryPoints))).entries)

	// Figure out which files are in which chunk
	for sourceIndex, fileMeta := range c.fileMeta {
		for _, partMeta := range fileMeta.partMeta {
			key := string(partMeta.reachableFromEntryPoints.entries)
			if key == neverReachedKey {
				// Ignore this part if it was never reached
				continue
			}
			chunk := chunks[key]
			if chunk.filesInChunk == nil {
				chunk.filesInChunk = make(map[uint32]bool)
				chunks[key] = chunk
			}
			chunk.filesInChunk[uint32(sourceIndex)] = true
		}
	}

	return []chunkMeta{}
}
