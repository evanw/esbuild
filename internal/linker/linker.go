package linker

// This package implements the second phase of the bundling operation that
// generates the output files when given a module graph. It has been split off
// into separate package to allow two linkers to cleanly exist in the same code
// base. This will be useful when rewriting the linker because the new one can
// be off by default to minimize disruption, but can still be enabled by anyone
// to assist in giving feedback on the rewrite.

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"hash"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/bundler"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/css_parser"
	"github.com/evanw/esbuild/internal/css_printer"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/graph"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/js_printer"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/renamer"
	"github.com/evanw/esbuild/internal/resolver"
	"github.com/evanw/esbuild/internal/runtime"
	"github.com/evanw/esbuild/internal/sourcemap"
	"github.com/evanw/esbuild/internal/xxhash"
)

type linkerContext struct {
	options *config.Options
	timer   *helpers.Timer
	log     logger.Log
	fs      fs.FS
	res     *resolver.Resolver
	graph   graph.LinkerGraph
	chunks  []chunkInfo

	// This helps avoid an infinite loop when matching imports to exports
	cycleDetector []importTracker

	// This represents the parallel computation of source map related data.
	// Calling this will block until the computation is done. The resulting value
	// is shared between threads and must be treated as immutable.
	dataForSourceMaps func() []bundler.DataForSourceMap

	// This is passed to us from the bundling phase
	uniqueKeyPrefix      string
	uniqueKeyPrefixBytes []byte // This is just "uniqueKeyPrefix" in byte form

	// Property mangling results go here
	mangledProps map[ast.Ref]string

	// We may need to refer to the CommonJS "module" symbol for exports
	unboundModuleRef ast.Ref

	// We may need to refer to the "__esm" and/or "__commonJS" runtime symbols
	cjsRuntimeRef ast.Ref
	esmRuntimeRef ast.Ref
}

type partRange struct {
	sourceIndex    uint32
	partIndexBegin uint32
	partIndexEnd   uint32
}

type chunkInfo struct {
	// This is a random string and is used to represent the output path of this
	// chunk before the final output path has been computed.
	uniqueKey string

	filesWithPartsInChunk map[uint32]bool
	entryBits             helpers.BitSet

	// For code splitting
	crossChunkImports []chunkImport

	// This is the representation-specific information
	chunkRepr chunkRepr

	// This is the final path of this chunk relative to the output directory, but
	// without the substitution of the final hash (since it hasn't been computed).
	finalTemplate []config.PathTemplate

	// This is the final path of this chunk relative to the output directory. It
	// is the substitution of the final hash into "finalTemplate".
	finalRelPath string

	// If non-empty, this chunk needs to generate an external legal comments file.
	externalLegalComments []byte

	// This contains the hash for just this chunk without including information
	// from the hashes of other chunks. Later on in the linking process, the
	// final hash for this chunk will be constructed by merging the isolated
	// hashes of all transitive dependencies of this chunk. This is separated
	// into two phases like this to handle cycles in the chunk import graph.
	waitForIsolatedHash func() []byte

	// Other fields relating to the output file for this chunk
	jsonMetadataChunkCallback func(finalOutputSize int) helpers.Joiner
	outputSourceMap           sourcemap.SourceMapPieces

	// When this chunk is initially generated in isolation, the output pieces
	// will contain slices of the output with the unique keys of other chunks
	// omitted.
	intermediateOutput intermediateOutput

	// This information is only useful if "isEntryPoint" is true
	entryPointBit uint   // An index into "c.graph.EntryPoints"
	sourceIndex   uint32 // An index into "c.sources"
	isEntryPoint  bool

	isExecutable bool
}

type chunkImport struct {
	chunkIndex uint32
	importKind ast.ImportKind
}

type outputPieceIndexKind uint8

const (
	outputPieceNone outputPieceIndexKind = iota
	outputPieceAssetIndex
	outputPieceChunkIndex
)

// This is a chunk of source code followed by a reference to another chunk. For
// example, the file "@import 'CHUNK0001'; body { color: black; }" would be
// represented by two pieces, one with the data "@import '" and another with the
// data "'; body { color: black; }". The first would have the chunk index 1 and
// the second would have an invalid chunk index.
type outputPiece struct {
	data []byte

	// Note: The "kind" may be "outputPieceNone" in which case there is one piece
	// with data and no chunk index. For example, the chunk may not contain any
	// imports.
	index uint32
	kind  outputPieceIndexKind
}

type intermediateOutput struct {
	// If the chunk has references to other chunks, then "pieces" contains the
	// contents of the chunk and "joiner" should not be used. Another joiner
	// will have to be constructed later when merging the pieces together.
	pieces []outputPiece

	// If the chunk doesn't have any references to other chunks, then "pieces" is
	// nil and "joiner" contains the contents of the chunk. This is more efficient
	// because it avoids doing a join operation twice.
	joiner helpers.Joiner
}

type chunkRepr interface{ isChunk() }

func (*chunkReprJS) isChunk()  {}
func (*chunkReprCSS) isChunk() {}

type chunkReprJS struct {
	filesInChunkInOrder []uint32
	partsInChunkInOrder []partRange

	// For code splitting
	exportsToOtherChunks   map[ast.Ref]string
	importsFromOtherChunks map[uint32]crossChunkImportItemArray
	crossChunkPrefixStmts  []js_ast.Stmt
	crossChunkSuffixStmts  []js_ast.Stmt

	cssChunkIndex uint32
	hasCSSChunk   bool
}

type chunkReprCSS struct {
	importsInChunkInOrder []cssImportOrder
}

type externalImportCSS struct {
	path                   logger.Path
	conditions             []css_ast.ImportConditions
	conditionImportRecords []ast.ImportRecord
}

// Returns a log where "log.HasErrors()" only returns true if any errors have
// been logged since this call. This is useful when there have already been
// errors logged by other linkers that share the same log.
func wrappedLog(log logger.Log) logger.Log {
	var mutex sync.Mutex
	var hasErrors bool
	addMsg := log.AddMsg

	log.AddMsg = func(msg logger.Msg) {
		if msg.Kind == logger.Error {
			mutex.Lock()
			defer mutex.Unlock()
			hasErrors = true
		}
		addMsg(msg)
	}

	log.HasErrors = func() bool {
		mutex.Lock()
		defer mutex.Unlock()
		return hasErrors
	}

	return log
}

func Link(
	options *config.Options,
	timer *helpers.Timer,
	log logger.Log,
	fs fs.FS,
	res *resolver.Resolver,
	inputFiles []graph.InputFile,
	entryPoints []graph.EntryPoint,
	uniqueKeyPrefix string,
	reachableFiles []uint32,
	dataForSourceMaps func() []bundler.DataForSourceMap,
) []graph.OutputFile {
	timer.Begin("Link")
	defer timer.End("Link")

	log = wrappedLog(log)

	timer.Begin("Clone linker graph")
	c := linkerContext{
		options:              options,
		timer:                timer,
		log:                  log,
		fs:                   fs,
		res:                  res,
		dataForSourceMaps:    dataForSourceMaps,
		uniqueKeyPrefix:      uniqueKeyPrefix,
		uniqueKeyPrefixBytes: []byte(uniqueKeyPrefix),
		graph: graph.CloneLinkerGraph(
			inputFiles,
			reachableFiles,
			entryPoints,
			options.CodeSplitting,
		),
	}
	timer.End("Clone linker graph")

	// Use a smaller version of these functions if we don't need profiler names
	runtimeRepr := c.graph.Files[runtime.SourceIndex].InputFile.Repr.(*graph.JSRepr)
	if c.options.ProfilerNames {
		c.cjsRuntimeRef = runtimeRepr.AST.NamedExports["__commonJS"].Ref
		c.esmRuntimeRef = runtimeRepr.AST.NamedExports["__esm"].Ref
	} else {
		c.cjsRuntimeRef = runtimeRepr.AST.NamedExports["__commonJSMin"].Ref
		c.esmRuntimeRef = runtimeRepr.AST.NamedExports["__esmMin"].Ref
	}

	var additionalFiles []graph.OutputFile
	for _, entryPoint := range entryPoints {
		file := &c.graph.Files[entryPoint.SourceIndex].InputFile
		switch repr := file.Repr.(type) {
		case *graph.JSRepr:
			// Loaders default to CommonJS when they are the entry point and the output
			// format is not ESM-compatible since that avoids generating the ESM-to-CJS
			// machinery.
			if repr.AST.HasLazyExport && (c.options.Mode == config.ModePassThrough ||
				(c.options.Mode == config.ModeConvertFormat && !c.options.OutputFormat.KeepESMImportExportSyntax())) {
				repr.AST.ExportsKind = js_ast.ExportsCommonJS
			}

			// Entry points with ES6 exports must generate an exports object when
			// targeting non-ES6 formats. Note that the IIFE format only needs this
			// when the global name is present, since that's the only way the exports
			// can actually be observed externally.
			if repr.AST.ExportKeyword.Len > 0 && (options.OutputFormat == config.FormatCommonJS ||
				(options.OutputFormat == config.FormatIIFE && len(options.GlobalName) > 0)) {
				repr.AST.UsesExportsRef = true
				repr.Meta.ForceIncludeExportsForEntryPoint = true
			}

		case *graph.CopyRepr:
			// If an entry point uses the copy loader, then copy the file manually
			// here. Other uses of the copy loader will automatically be included
			// along with the corresponding bundled chunk but that doesn't happen
			// for entry points.
			additionalFiles = append(additionalFiles, file.AdditionalFiles...)
		}
	}

	// Allocate a new unbound symbol called "module" in case we need it later
	if c.options.OutputFormat == config.FormatCommonJS {
		c.unboundModuleRef = c.graph.GenerateNewSymbol(runtime.SourceIndex, ast.SymbolUnbound, "module")
	} else {
		c.unboundModuleRef = ast.InvalidRef
	}

	c.scanImportsAndExports()

	// Stop now if there were errors
	if c.log.HasErrors() {
		c.options.ExclusiveMangleCacheUpdate(func(map[string]interface{}, map[string]bool) {
			// Always do this so that we don't cause other entry points when there are errors
		})
		return []graph.OutputFile{}
	}

	c.treeShakingAndCodeSplitting()

	if c.options.Mode == config.ModePassThrough {
		for _, entryPoint := range c.graph.EntryPoints() {
			c.preventExportsFromBeingRenamed(entryPoint.SourceIndex)
		}
	}

	c.computeChunks()
	c.computeCrossChunkDependencies()

	// Merge mangled properties before chunks are generated since the names must
	// be consistent across all chunks, or the generated code will break
	c.timer.Begin("Waiting for mangle cache")
	c.options.ExclusiveMangleCacheUpdate(func(
		mangleCache map[string]interface{},
		cssUsedLocalNames map[string]bool,
	) {
		c.timer.End("Waiting for mangle cache")
		c.mangleProps(mangleCache)
		c.mangleLocalCSS(cssUsedLocalNames)
	})

	// Make sure calls to "ast.FollowSymbols()" in parallel goroutines after this
	// won't hit concurrent map mutation hazards
	ast.FollowAllSymbols(c.graph.Symbols)

	return c.generateChunksInParallel(additionalFiles)
}

func (c *linkerContext) mangleProps(mangleCache map[string]interface{}) {
	c.timer.Begin("Mangle props")
	defer c.timer.End("Mangle props")

	mangledProps := make(map[ast.Ref]string)
	c.mangledProps = mangledProps

	// Reserve all JS keywords
	reservedProps := make(map[string]bool)
	for keyword := range js_lexer.Keywords {
		reservedProps[keyword] = true
	}

	// Reserve all target properties in the cache
	for original, remapped := range mangleCache {
		if remapped == false {
			reservedProps[original] = true
		} else {
			reservedProps[remapped.(string)] = true
		}
	}

	// Merge all mangled property symbols together
	freq := ast.CharFreq{}
	mergedProps := make(map[string]ast.Ref)
	for _, sourceIndex := range c.graph.ReachableFiles {
		// Don't mangle anything in the runtime code
		if sourceIndex == runtime.SourceIndex {
			continue
		}

		// For each file
		if repr, ok := c.graph.Files[sourceIndex].InputFile.Repr.(*graph.JSRepr); ok {
			// Reserve all non-mangled properties
			for prop := range repr.AST.ReservedProps {
				reservedProps[prop] = true
			}

			// Merge each mangled property with other ones of the same name
			for name, ref := range repr.AST.MangledProps {
				if existing, ok := mergedProps[name]; ok {
					ast.MergeSymbols(c.graph.Symbols, ref, existing)
				} else {
					mergedProps[name] = ref
				}
			}

			// Include this file's frequency histogram, which affects the mangled names
			if repr.AST.CharFreq != nil {
				freq.Include(repr.AST.CharFreq)
			}
		}
	}

	// Sort by use count (note: does not currently account for live vs. dead code)
	sorted := make(renamer.StableSymbolCountArray, 0, len(mergedProps))
	stableSourceIndices := c.graph.StableSourceIndices
	for _, ref := range mergedProps {
		sorted = append(sorted, renamer.StableSymbolCount{
			StableSourceIndex: stableSourceIndices[ref.SourceIndex],
			Ref:               ref,
			Count:             c.graph.Symbols.Get(ref).UseCountEstimate,
		})
	}
	sort.Sort(sorted)

	// Assign names in order of use count
	minifier := ast.DefaultNameMinifierJS.ShuffleByCharFreq(freq)
	nextName := 0
	for _, symbolCount := range sorted {
		symbol := c.graph.Symbols.Get(symbolCount.Ref)

		// Don't change existing mappings
		if existing, ok := mangleCache[symbol.OriginalName]; ok {
			if existing != false {
				mangledProps[symbolCount.Ref] = existing.(string)
			}
			continue
		}

		// Generate a new name
		name := minifier.NumberToMinifiedName(nextName)
		nextName++

		// Avoid reserved properties
		for reservedProps[name] {
			name = minifier.NumberToMinifiedName(nextName)
			nextName++
		}

		// Track the new mapping
		if mangleCache != nil {
			mangleCache[symbol.OriginalName] = name
		}
		mangledProps[symbolCount.Ref] = name
	}
}

func (c *linkerContext) mangleLocalCSS(usedLocalNames map[string]bool) {
	c.timer.Begin("Mangle local CSS")
	defer c.timer.End("Mangle local CSS")

	mangledProps := c.mangledProps
	globalNames := make(map[string]bool)
	localNames := make(map[ast.Ref]struct{})

	// Collect all local and global CSS names
	freq := ast.CharFreq{}
	for _, sourceIndex := range c.graph.ReachableFiles {
		if repr, ok := c.graph.Files[sourceIndex].InputFile.Repr.(*graph.CSSRepr); ok {
			for innerIndex, symbol := range c.graph.Symbols.SymbolsForSource[sourceIndex] {
				if symbol.Kind == ast.SymbolGlobalCSS {
					globalNames[symbol.OriginalName] = true
				} else {
					ref := ast.Ref{SourceIndex: sourceIndex, InnerIndex: uint32(innerIndex)}
					ref = ast.FollowSymbols(c.graph.Symbols, ref)
					localNames[ref] = struct{}{}
				}
			}

			// Include this file's frequency histogram, which affects the mangled names
			if repr.AST.CharFreq != nil {
				freq.Include(repr.AST.CharFreq)
			}
		}
	}

	// Sort by use count (note: does not currently account for live vs. dead code)
	sorted := make(renamer.StableSymbolCountArray, 0, len(localNames))
	stableSourceIndices := c.graph.StableSourceIndices
	for ref := range localNames {
		sorted = append(sorted, renamer.StableSymbolCount{
			StableSourceIndex: stableSourceIndices[ref.SourceIndex],
			Ref:               ref,
			Count:             c.graph.Symbols.Get(ref).UseCountEstimate,
		})
	}
	sort.Sort(sorted)

	// Rename all local names to avoid collisions
	if c.options.MinifyIdentifiers {
		minifier := ast.DefaultNameMinifierCSS.ShuffleByCharFreq(freq)
		nextName := 0

		for _, symbolCount := range sorted {
			name := minifier.NumberToMinifiedName(nextName)
			for globalNames[name] || usedLocalNames[name] {
				nextName++
				name = minifier.NumberToMinifiedName(nextName)
			}

			// Turn this local name into a global one
			mangledProps[symbolCount.Ref] = name
			usedLocalNames[name] = true
		}
	} else {
		nameCounts := make(map[string]uint32)

		for _, symbolCount := range sorted {
			symbol := c.graph.Symbols.Get(symbolCount.Ref)
			name := fmt.Sprintf("%s_%s", c.graph.Files[symbolCount.Ref.SourceIndex].InputFile.Source.IdentifierName, symbol.OriginalName)

			// If the name is already in use, generate a new name by appending a number
			if globalNames[name] || usedLocalNames[name] {
				// To avoid O(n^2) behavior, the number must start off being the number
				// that we used last time there was a collision with this name. Otherwise
				// if there are many collisions with the same name, each name collision
				// would have to increment the counter past all previous name collisions
				// which is a O(n^2) time algorithm.
				tries, ok := nameCounts[name]
				if !ok {
					tries = 1
				}
				prefix := name

				// Keep incrementing the number until the name is unused
				for {
					tries++
					name = prefix + strconv.Itoa(int(tries))

					// Make sure this new name is unused
					if !globalNames[name] && !usedLocalNames[name] {
						// Store the count so we can start here next time instead of starting
						// from 1. This means we avoid O(n^2) behavior.
						nameCounts[prefix] = tries
						break
					}
				}
			}

			// Turn this local name into a global one
			mangledProps[symbolCount.Ref] = name
			usedLocalNames[name] = true
		}
	}
}

// Currently the automatic chunk generation algorithm should by construction
// never generate chunks that import each other since files are allocated to
// chunks based on which entry points they are reachable from.
//
// This will change in the future when we allow manual chunk labels. But before
// we allow manual chunk labels, we'll need to rework module initialization to
// allow code splitting chunks to be lazily-initialized.
//
// Since that work hasn't been finished yet, cycles in the chunk import graph
// can cause initialization bugs. So let's forbid these cycles for now to guard
// against code splitting bugs that could cause us to generate buggy chunks.
func (c *linkerContext) enforceNoCyclicChunkImports() {
	var validate func(int, map[int]int) bool

	// DFS memoization with 3-colors, more space efficient
	// 0: white (unvisited), 1: gray (visiting), 2: black (visited)
	colors := make(map[int]int)
	validate = func(chunkIndex int, colors map[int]int) bool {
		if colors[chunkIndex] == 1 {
			c.log.AddError(nil, logger.Range{}, "Internal error: generated chunks contain a circular import")
			return true
		}

		if colors[chunkIndex] == 2 {
			return false
		}

		colors[chunkIndex] = 1

		for _, chunkImport := range c.chunks[chunkIndex].crossChunkImports {
			// Ignore cycles caused by dynamic "import()" expressions. These are fine
			// because they don't necessarily cause initialization order issues and
			// they don't indicate a bug in our chunk generation algorithm. They arise
			// normally in real code (e.g. two files that import each other).
			if chunkImport.importKind != ast.ImportDynamic {

				// Recursively validate otherChunkIndex
				if validate(int(chunkImport.chunkIndex), colors) {
					return true
				}
			}
		}

		colors[chunkIndex] = 2
		return false
	}

	for i := range c.chunks {
		if validate(i, colors) {
			break
		}
	}
}

func (c *linkerContext) generateChunksInParallel(additionalFiles []graph.OutputFile) []graph.OutputFile {
	c.timer.Begin("Generate chunks")
	defer c.timer.End("Generate chunks")

	// Generate each chunk on a separate goroutine. When a chunk needs to
	// reference the path of another chunk, it will use a temporary path called
	// the "uniqueKey" since the final path hasn't been computed yet (and is
	// in general uncomputable at this point because paths have hashes that
	// include information about chunk dependencies, and chunk dependencies
	// can be cyclic due to dynamic imports).
	generateWaitGroup := sync.WaitGroup{}
	generateWaitGroup.Add(len(c.chunks))
	for chunkIndex := range c.chunks {
		switch c.chunks[chunkIndex].chunkRepr.(type) {
		case *chunkReprJS:
			go c.generateChunkJS(chunkIndex, &generateWaitGroup)
		case *chunkReprCSS:
			go c.generateChunkCSS(chunkIndex, &generateWaitGroup)
		}
	}
	c.enforceNoCyclicChunkImports()
	generateWaitGroup.Wait()

	// Compute the final hashes of each chunk, then use those to create the final
	// paths of each chunk. This can technically be done in parallel but it
	// probably doesn't matter so much because we're not hashing that much data.
	visited := make([]uint32, len(c.chunks))
	var finalBytes []byte
	for chunkIndex := range c.chunks {
		chunk := &c.chunks[chunkIndex]
		var hashSubstitution *string

		// Only wait for the hash if necessary
		if config.HasPlaceholder(chunk.finalTemplate, config.HashPlaceholder) {
			// Compute the final hash using the isolated hashes of the dependencies
			hash := xxhash.New()
			c.appendIsolatedHashesForImportedChunks(hash, uint32(chunkIndex), visited, ^uint32(chunkIndex))
			finalBytes = hash.Sum(finalBytes[:0])
			finalString := bundler.HashForFileName(finalBytes)
			hashSubstitution = &finalString
		}

		// Render the last remaining placeholder in the template
		chunk.finalRelPath = config.TemplateToString(config.SubstituteTemplate(chunk.finalTemplate, config.PathPlaceholders{
			Hash: hashSubstitution,
		}))
	}

	// Generate the final output files by joining file pieces together and
	// substituting the temporary paths for the final paths. This substitution
	// can be done in parallel for each chunk.
	c.timer.Begin("Generate final output files")
	var resultsWaitGroup sync.WaitGroup
	results := make([][]graph.OutputFile, len(c.chunks))
	resultsWaitGroup.Add(len(c.chunks))
	for chunkIndex, chunk := range c.chunks {
		go func(chunkIndex int, chunk chunkInfo) {
			var outputFiles []graph.OutputFile

			// Each file may optionally contain additional files to be copied to the
			// output directory. This is used by the "file" and "copy" loaders.
			var commentPrefix string
			var commentSuffix string
			switch chunkRepr := chunk.chunkRepr.(type) {
			case *chunkReprJS:
				for _, sourceIndex := range chunkRepr.filesInChunkInOrder {
					outputFiles = append(outputFiles, c.graph.Files[sourceIndex].InputFile.AdditionalFiles...)
				}
				commentPrefix = "//"

			case *chunkReprCSS:
				for _, entry := range chunkRepr.importsInChunkInOrder {
					if entry.kind == cssImportSourceIndex {
						outputFiles = append(outputFiles, c.graph.Files[entry.sourceIndex].InputFile.AdditionalFiles...)
					}
				}
				commentPrefix = "/*"
				commentSuffix = " */"
			}

			// Path substitution for the chunk itself
			finalRelDir := c.fs.Dir(chunk.finalRelPath)
			outputContentsJoiner, outputSourceMapShifts := c.substituteFinalPaths(chunk.intermediateOutput,
				func(finalRelPathForImport string) string {
					return c.pathBetweenChunks(finalRelDir, finalRelPathForImport)
				})

			// Generate the optional legal comments file for this chunk
			if len(chunk.externalLegalComments) > 0 {
				finalRelPathForLegalComments := chunk.finalRelPath + ".LEGAL.txt"

				// Link the file to the legal comments
				if c.options.LegalComments == config.LegalCommentsLinkedWithComment {
					importPath := c.pathBetweenChunks(finalRelDir, finalRelPathForLegalComments)
					importPath = strings.TrimPrefix(importPath, "./")
					outputContentsJoiner.EnsureNewlineAtEnd()
					outputContentsJoiner.AddString("/*! For license information please see ")
					outputContentsJoiner.AddString(importPath)
					outputContentsJoiner.AddString(" */\n")
				}

				// Write the external legal comments file
				outputFiles = append(outputFiles, graph.OutputFile{
					AbsPath:  c.fs.Join(c.options.AbsOutputDir, finalRelPathForLegalComments),
					Contents: chunk.externalLegalComments,
					JSONMetadataChunk: fmt.Sprintf(
						"{\n      \"imports\": [],\n      \"exports\": [],\n      \"inputs\": {},\n      \"bytes\": %d\n    }", len(chunk.externalLegalComments)),
				})
			}

			// Generate the optional source map for this chunk
			if c.options.SourceMap != config.SourceMapNone && chunk.outputSourceMap.HasContent() {
				outputSourceMap := chunk.outputSourceMap.Finalize(outputSourceMapShifts)
				finalRelPathForSourceMap := chunk.finalRelPath + ".map"

				// Potentially write a trailing source map comment
				switch c.options.SourceMap {
				case config.SourceMapLinkedWithComment:
					importPath := c.pathBetweenChunks(finalRelDir, finalRelPathForSourceMap)
					importPath = strings.TrimPrefix(importPath, "./")
					importURL := url.URL{Path: importPath}
					outputContentsJoiner.EnsureNewlineAtEnd()
					outputContentsJoiner.AddString(commentPrefix)
					outputContentsJoiner.AddString("# sourceMappingURL=")
					outputContentsJoiner.AddString(importURL.EscapedPath())
					outputContentsJoiner.AddString(commentSuffix)
					outputContentsJoiner.AddString("\n")

				case config.SourceMapInline, config.SourceMapInlineAndExternal:
					outputContentsJoiner.EnsureNewlineAtEnd()
					outputContentsJoiner.AddString(commentPrefix)
					outputContentsJoiner.AddString("# sourceMappingURL=data:application/json;base64,")
					outputContentsJoiner.AddString(base64.StdEncoding.EncodeToString(outputSourceMap))
					outputContentsJoiner.AddString(commentSuffix)
					outputContentsJoiner.AddString("\n")
				}

				// Potentially write the external source map file
				switch c.options.SourceMap {
				case config.SourceMapLinkedWithComment, config.SourceMapInlineAndExternal, config.SourceMapExternalWithoutComment:
					outputFiles = append(outputFiles, graph.OutputFile{
						AbsPath:  c.fs.Join(c.options.AbsOutputDir, finalRelPathForSourceMap),
						Contents: outputSourceMap,
						JSONMetadataChunk: fmt.Sprintf(
							"{\n      \"imports\": [],\n      \"exports\": [],\n      \"inputs\": {},\n      \"bytes\": %d\n    }", len(outputSourceMap)),
					})
				}
			}

			// Finalize the output contents
			outputContents := outputContentsJoiner.Done()

			// Path substitution for the JSON metadata
			var jsonMetadataChunk string
			if c.options.NeedsMetafile {
				jsonMetadataChunkPieces := c.breakJoinerIntoPieces(chunk.jsonMetadataChunkCallback(len(outputContents)))
				jsonMetadataChunkBytes, _ := c.substituteFinalPaths(jsonMetadataChunkPieces, func(finalRelPathForImport string) string {
					return resolver.PrettyPath(c.fs, logger.Path{Text: c.fs.Join(c.options.AbsOutputDir, finalRelPathForImport), Namespace: "file"})
				})
				jsonMetadataChunk = string(jsonMetadataChunkBytes.Done())
			}

			// Generate the output file for this chunk
			outputFiles = append(outputFiles, graph.OutputFile{
				AbsPath:           c.fs.Join(c.options.AbsOutputDir, chunk.finalRelPath),
				Contents:          outputContents,
				JSONMetadataChunk: jsonMetadataChunk,
				IsExecutable:      chunk.isExecutable,
			})

			results[chunkIndex] = outputFiles
			resultsWaitGroup.Done()
		}(chunkIndex, chunk)
	}
	resultsWaitGroup.Wait()
	c.timer.End("Generate final output files")

	// Merge the output files from the different goroutines together in order
	outputFilesLen := len(additionalFiles)
	for _, result := range results {
		outputFilesLen += len(result)
	}
	outputFiles := make([]graph.OutputFile, 0, outputFilesLen)
	outputFiles = append(outputFiles, additionalFiles...)
	for _, result := range results {
		outputFiles = append(outputFiles, result...)
	}
	return outputFiles
}

// Given a set of output pieces (i.e. a buffer already divided into the spans
// between import paths), substitute the final import paths in and then join
// everything into a single byte buffer.
func (c *linkerContext) substituteFinalPaths(
	intermediateOutput intermediateOutput,
	modifyPath func(string) string,
) (j helpers.Joiner, shifts []sourcemap.SourceMapShift) {
	// Optimization: If there can be no substitutions, just reuse the initial
	// joiner that was used when generating the intermediate chunk output
	// instead of creating another one and copying the whole file into it.
	if intermediateOutput.pieces == nil {
		return intermediateOutput.joiner, []sourcemap.SourceMapShift{{}}
	}

	var shift sourcemap.SourceMapShift
	shifts = make([]sourcemap.SourceMapShift, 0, len(intermediateOutput.pieces))
	shifts = append(shifts, shift)

	for _, piece := range intermediateOutput.pieces {
		var dataOffset sourcemap.LineColumnOffset
		j.AddBytes(piece.data)
		dataOffset.AdvanceBytes(piece.data)
		shift.Before.Add(dataOffset)
		shift.After.Add(dataOffset)

		switch piece.kind {
		case outputPieceAssetIndex:
			file := c.graph.Files[piece.index]
			if len(file.InputFile.AdditionalFiles) != 1 {
				panic("Internal error")
			}
			relPath, _ := c.fs.Rel(c.options.AbsOutputDir, file.InputFile.AdditionalFiles[0].AbsPath)

			// Make sure to always use forward slashes, even on Windows
			relPath = strings.ReplaceAll(relPath, "\\", "/")

			importPath := modifyPath(relPath)
			j.AddString(importPath)
			shift.Before.AdvanceString(file.InputFile.UniqueKeyForAdditionalFile)
			shift.After.AdvanceString(importPath)
			shifts = append(shifts, shift)

		case outputPieceChunkIndex:
			chunk := c.chunks[piece.index]
			importPath := modifyPath(chunk.finalRelPath)
			j.AddString(importPath)
			shift.Before.AdvanceString(chunk.uniqueKey)
			shift.After.AdvanceString(importPath)
			shifts = append(shifts, shift)
		}
	}

	return
}

func (c *linkerContext) accurateFinalByteCount(output intermediateOutput, chunkFinalRelDir string) int {
	count := 0

	// Note: The paths generated here must match "substituteFinalPaths" above
	for _, piece := range output.pieces {
		count += len(piece.data)

		switch piece.kind {
		case outputPieceAssetIndex:
			file := c.graph.Files[piece.index]
			if len(file.InputFile.AdditionalFiles) != 1 {
				panic("Internal error")
			}
			relPath, _ := c.fs.Rel(c.options.AbsOutputDir, file.InputFile.AdditionalFiles[0].AbsPath)

			// Make sure to always use forward slashes, even on Windows
			relPath = strings.ReplaceAll(relPath, "\\", "/")

			importPath := c.pathBetweenChunks(chunkFinalRelDir, relPath)
			count += len(importPath)

		case outputPieceChunkIndex:
			chunk := c.chunks[piece.index]
			importPath := c.pathBetweenChunks(chunkFinalRelDir, chunk.finalRelPath)
			count += len(importPath)
		}
	}

	return count
}

func (c *linkerContext) pathBetweenChunks(fromRelDir string, toRelPath string) string {
	// Join with the public path if it has been configured
	if c.options.PublicPath != "" {
		return joinWithPublicPath(c.options.PublicPath, toRelPath)
	}

	// Otherwise, return a relative path
	relPath, ok := c.fs.Rel(fromRelDir, toRelPath)
	if !ok {
		c.log.AddError(nil, logger.Range{},
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

func (c *linkerContext) computeCrossChunkDependencies() {
	c.timer.Begin("Compute cross-chunk dependencies")
	defer c.timer.End("Compute cross-chunk dependencies")

	if !c.options.CodeSplitting {
		// No need to compute cross-chunk dependencies if there can't be any
		return
	}

	type chunkMeta struct {
		imports        map[ast.Ref]bool
		exports        map[ast.Ref]bool
		dynamicImports map[int]bool
	}

	chunkMetas := make([]chunkMeta, len(c.chunks))

	// For each chunk, see what symbols it uses from other chunks. Do this in
	// parallel because it's the most expensive part of this function.
	waitGroup := sync.WaitGroup{}
	waitGroup.Add(len(c.chunks))
	for chunkIndex, chunk := range c.chunks {
		go func(chunkIndex int, chunk chunkInfo) {
			chunkMeta := &chunkMetas[chunkIndex]
			imports := make(map[ast.Ref]bool)
			chunkMeta.imports = imports
			chunkMeta.exports = make(map[ast.Ref]bool)

			// Go over each file in this chunk
			for sourceIndex := range chunk.filesWithPartsInChunk {
				// Go over each part in this file that's marked for inclusion in this chunk
				switch repr := c.graph.Files[sourceIndex].InputFile.Repr.(type) {
				case *graph.JSRepr:
					for partIndex, partMeta := range repr.AST.Parts {
						if !partMeta.IsLive {
							continue
						}
						part := &repr.AST.Parts[partIndex]

						// Rewrite external dynamic imports to point to the chunk for that entry point
						for _, importRecordIndex := range part.ImportRecordIndices {
							record := &repr.AST.ImportRecords[importRecordIndex]
							if record.SourceIndex.IsValid() && c.isExternalDynamicImport(record, sourceIndex) {
								otherChunkIndex := c.graph.Files[record.SourceIndex.GetIndex()].EntryPointChunkIndex
								record.Path.Text = c.chunks[otherChunkIndex].uniqueKey
								record.SourceIndex = ast.Index32{}
								record.Flags |= ast.ShouldNotBeExternalInMetafile | ast.ContainsUniqueKey

								// Track this cross-chunk dynamic import so we make sure to
								// include its hash when we're calculating the hashes of all
								// dependencies of this chunk.
								if int(otherChunkIndex) != chunkIndex {
									if chunkMeta.dynamicImports == nil {
										chunkMeta.dynamicImports = make(map[int]bool)
									}
									chunkMeta.dynamicImports[int(otherChunkIndex)] = true
								}
							}
						}

						// Remember what chunk each top-level symbol is declared in. Symbols
						// with multiple declarations such as repeated "var" statements with
						// the same name should already be marked as all being in a single
						// chunk. In that case this will overwrite the same value below which
						// is fine.
						for _, declared := range part.DeclaredSymbols {
							if declared.IsTopLevel {
								c.graph.Symbols.Get(declared.Ref).ChunkIndex = ast.MakeIndex32(uint32(chunkIndex))
							}
						}

						// Record each symbol used in this part. This will later be matched up
						// with our map of which chunk a given symbol is declared in to
						// determine if the symbol needs to be imported from another chunk.
						for ref := range part.SymbolUses {
							symbol := c.graph.Symbols.Get(ref)

							// Ignore unbound symbols, which don't have declarations
							if symbol.Kind == ast.SymbolUnbound {
								continue
							}

							// Ignore symbols that are going to be replaced by undefined
							if symbol.ImportItemStatus == ast.ImportItemMissing {
								continue
							}

							// If this is imported from another file, follow the import
							// reference and reference the symbol in that file instead
							if importData, ok := repr.Meta.ImportsToBind[ref]; ok {
								ref = importData.Ref
								symbol = c.graph.Symbols.Get(ref)
							} else if repr.Meta.Wrap == graph.WrapCJS && ref != repr.AST.WrapperRef {
								// The only internal symbol that wrapped CommonJS files export
								// is the wrapper itself.
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

			// Include the exports if this is an entry point chunk
			if chunk.isEntryPoint {
				if repr, ok := c.graph.Files[chunk.sourceIndex].InputFile.Repr.(*graph.JSRepr); ok {
					if repr.Meta.Wrap != graph.WrapCJS {
						for _, alias := range repr.Meta.SortedAndFilteredExportAliases {
							export := repr.Meta.ResolvedExports[alias]
							targetRef := export.Ref

							// If this is an import, then target what the import points to
							if importData, ok := c.graph.Files[export.SourceIndex].InputFile.Repr.(*graph.JSRepr).Meta.ImportsToBind[targetRef]; ok {
								targetRef = importData.Ref
							}

							// If this is an ES6 import from a CommonJS file, it will become a
							// property access off the namespace symbol instead of a bare
							// identifier. In that case we want to pull in the namespace symbol
							// instead. The namespace symbol stores the result of "require()".
							if symbol := c.graph.Symbols.Get(targetRef); symbol.NamespaceAlias != nil {
								targetRef = symbol.NamespaceAlias.NamespaceRef
							}

							imports[targetRef] = true
						}
					}

					// Ensure "exports" is included if the current output format needs it
					if repr.Meta.ForceIncludeExportsForEntryPoint {
						imports[repr.AST.ExportsRef] = true
					}

					// Include the wrapper if present
					if repr.Meta.Wrap != graph.WrapNone {
						imports[repr.AST.WrapperRef] = true
					}
				}
			}

			waitGroup.Done()
		}(chunkIndex, chunk)
	}
	waitGroup.Wait()

	// Mark imported symbols as exported in the chunk from which they are declared
	for chunkIndex := range c.chunks {
		chunk := &c.chunks[chunkIndex]
		chunkRepr, ok := chunk.chunkRepr.(*chunkReprJS)
		if !ok {
			continue
		}
		chunkMeta := chunkMetas[chunkIndex]

		// Find all uses in this chunk of symbols from other chunks
		chunkRepr.importsFromOtherChunks = make(map[uint32]crossChunkImportItemArray)
		for importRef := range chunkMeta.imports {
			// Ignore uses that aren't top-level symbols
			if otherChunkIndex := c.graph.Symbols.Get(importRef).ChunkIndex; otherChunkIndex.IsValid() {
				if otherChunkIndex := otherChunkIndex.GetIndex(); otherChunkIndex != uint32(chunkIndex) {
					chunkRepr.importsFromOtherChunks[otherChunkIndex] =
						append(chunkRepr.importsFromOtherChunks[otherChunkIndex], crossChunkImportItem{ref: importRef})
					chunkMetas[otherChunkIndex].exports[importRef] = true
				}
			}
		}

		// If this is an entry point, make sure we import all chunks belonging to
		// this entry point, even if there are no imports. We need to make sure
		// these chunks are evaluated for their side effects too.
		if chunk.isEntryPoint {
			for otherChunkIndex, otherChunk := range c.chunks {
				if _, ok := otherChunk.chunkRepr.(*chunkReprJS); ok && chunkIndex != otherChunkIndex && otherChunk.entryBits.HasBit(chunk.entryPointBit) {
					imports := chunkRepr.importsFromOtherChunks[uint32(otherChunkIndex)]
					chunkRepr.importsFromOtherChunks[uint32(otherChunkIndex)] = imports
				}
			}
		}

		// Make sure we also track dynamic cross-chunk imports. These need to be
		// tracked so we count them as dependencies of this chunk for the purpose
		// of hash calculation.
		if chunkMeta.dynamicImports != nil {
			sortedDynamicImports := make([]int, 0, len(chunkMeta.dynamicImports))
			for chunkIndex := range chunkMeta.dynamicImports {
				sortedDynamicImports = append(sortedDynamicImports, chunkIndex)
			}
			sort.Ints(sortedDynamicImports)
			for _, chunkIndex := range sortedDynamicImports {
				chunk.crossChunkImports = append(chunk.crossChunkImports, chunkImport{
					importKind: ast.ImportDynamic,
					chunkIndex: uint32(chunkIndex),
				})
			}
		}
	}

	// Generate cross-chunk exports. These must be computed before cross-chunk
	// imports because of export alias renaming, which must consider all export
	// aliases simultaneously to avoid collisions.
	for chunkIndex := range c.chunks {
		chunk := &c.chunks[chunkIndex]
		chunkRepr, ok := chunk.chunkRepr.(*chunkReprJS)
		if !ok {
			continue
		}

		chunkRepr.exportsToOtherChunks = make(map[ast.Ref]string)
		switch c.options.OutputFormat {
		case config.FormatESModule:
			r := renamer.ExportRenamer{}
			var items []js_ast.ClauseItem
			for _, export := range c.sortedCrossChunkExportItems(chunkMetas[chunkIndex].exports) {
				var alias string
				if c.options.MinifyIdentifiers {
					alias = r.NextMinifiedName()
				} else {
					alias = r.NextRenamedName(c.graph.Symbols.Get(export.Ref).OriginalName)
				}
				items = append(items, js_ast.ClauseItem{Name: ast.LocRef{Ref: export.Ref}, Alias: alias})
				chunkRepr.exportsToOtherChunks[export.Ref] = alias
			}
			if len(items) > 0 {
				chunkRepr.crossChunkSuffixStmts = []js_ast.Stmt{{Data: &js_ast.SExportClause{
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
	for chunkIndex := range c.chunks {
		chunk := &c.chunks[chunkIndex]
		chunkRepr, ok := chunk.chunkRepr.(*chunkReprJS)
		if !ok {
			continue
		}

		var crossChunkPrefixStmts []js_ast.Stmt

		for _, crossChunkImport := range c.sortedCrossChunkImports(chunkRepr.importsFromOtherChunks) {
			switch c.options.OutputFormat {
			case config.FormatESModule:
				var items []js_ast.ClauseItem
				for _, item := range crossChunkImport.sortedImportItems {
					items = append(items, js_ast.ClauseItem{Name: ast.LocRef{Ref: item.ref}, Alias: item.exportAlias})
				}
				importRecordIndex := uint32(len(chunk.crossChunkImports))
				chunk.crossChunkImports = append(chunk.crossChunkImports, chunkImport{
					importKind: ast.ImportStmt,
					chunkIndex: crossChunkImport.chunkIndex,
				})
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

		chunkRepr.crossChunkPrefixStmts = crossChunkPrefixStmts
	}
}

type crossChunkImport struct {
	sortedImportItems crossChunkImportItemArray
	chunkIndex        uint32
}

// This type is just so we can use Go's native sort function
type crossChunkImportArray []crossChunkImport

func (a crossChunkImportArray) Len() int          { return len(a) }
func (a crossChunkImportArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a crossChunkImportArray) Less(i int, j int) bool {
	return a[i].chunkIndex < a[j].chunkIndex
}

// Sort cross-chunk imports by chunk name for determinism
func (c *linkerContext) sortedCrossChunkImports(importsFromOtherChunks map[uint32]crossChunkImportItemArray) crossChunkImportArray {
	result := make(crossChunkImportArray, 0, len(importsFromOtherChunks))

	for otherChunkIndex, importItems := range importsFromOtherChunks {
		// Sort imports from a single chunk by alias for determinism
		otherChunk := &c.chunks[otherChunkIndex]
		exportsToOtherChunks := otherChunk.chunkRepr.(*chunkReprJS).exportsToOtherChunks
		for i, item := range importItems {
			importItems[i].exportAlias = exportsToOtherChunks[item.ref]
		}
		sort.Sort(importItems)
		result = append(result, crossChunkImport{
			chunkIndex:        otherChunkIndex,
			sortedImportItems: importItems,
		})
	}

	sort.Sort(result)
	return result
}

type crossChunkImportItem struct {
	exportAlias string
	ref         ast.Ref
}

// This type is just so we can use Go's native sort function
type crossChunkImportItemArray []crossChunkImportItem

func (a crossChunkImportItemArray) Len() int          { return len(a) }
func (a crossChunkImportItemArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a crossChunkImportItemArray) Less(i int, j int) bool {
	return a[i].exportAlias < a[j].exportAlias
}

// The sort order here is arbitrary but needs to be consistent between builds.
// The InnerIndex should be stable because the parser for a single file is
// single-threaded and deterministically assigns out InnerIndex values
// sequentially. But the SourceIndex should be unstable because the main thread
// assigns out source index values sequentially to newly-discovered dependencies
// in a multi-threaded producer/consumer relationship. So instead we use the
// index of the source in the DFS order over all entry points for stability.
type stableRef struct {
	StableSourceIndex uint32
	Ref               ast.Ref
}

// This type is just so we can use Go's native sort function
type stableRefArray []stableRef

func (a stableRefArray) Len() int          { return len(a) }
func (a stableRefArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }
func (a stableRefArray) Less(i int, j int) bool {
	ai, aj := a[i], a[j]
	return ai.StableSourceIndex < aj.StableSourceIndex ||
		(ai.StableSourceIndex == aj.StableSourceIndex && ai.Ref.InnerIndex < aj.Ref.InnerIndex)
}

// Sort cross-chunk exports by chunk name for determinism
func (c *linkerContext) sortedCrossChunkExportItems(exportRefs map[ast.Ref]bool) stableRefArray {
	result := make(stableRefArray, 0, len(exportRefs))
	for ref := range exportRefs {
		result = append(result, stableRef{
			StableSourceIndex: c.graph.StableSourceIndices[ref.SourceIndex],
			Ref:               ref,
		})
	}
	sort.Sort(result)
	return result
}

func (c *linkerContext) scanImportsAndExports() {
	c.timer.Begin("Scan imports and exports")
	defer c.timer.End("Scan imports and exports")

	// Step 1: Figure out what modules must be CommonJS
	c.timer.Begin("Step 1")
	for _, sourceIndex := range c.graph.ReachableFiles {
		file := &c.graph.Files[sourceIndex]
		additionalFiles := file.InputFile.AdditionalFiles

		switch repr := file.InputFile.Repr.(type) {
		case *graph.CSSRepr:
			// Inline URLs for non-CSS files into the CSS file
			for importRecordIndex := range repr.AST.ImportRecords {
				if record := &repr.AST.ImportRecords[importRecordIndex]; record.SourceIndex.IsValid() {
					otherFile := &c.graph.Files[record.SourceIndex.GetIndex()]
					if otherRepr, ok := otherFile.InputFile.Repr.(*graph.JSRepr); ok {
						record.Path.Text = otherRepr.AST.URLForCSS
						record.Path.Namespace = ""
						record.SourceIndex = ast.Index32{}
						if otherFile.InputFile.Loader == config.LoaderEmpty {
							record.Flags |= ast.WasLoadedWithEmptyLoader
						} else {
							record.Flags |= ast.ShouldNotBeExternalInMetafile
						}
						if strings.Contains(otherRepr.AST.URLForCSS, c.uniqueKeyPrefix) {
							record.Flags |= ast.ContainsUniqueKey
						}

						// Copy the additional files to the output directory
						additionalFiles = append(additionalFiles, otherFile.InputFile.AdditionalFiles...)
					}
				} else if record.CopySourceIndex.IsValid() {
					otherFile := &c.graph.Files[record.CopySourceIndex.GetIndex()]
					if otherRepr, ok := otherFile.InputFile.Repr.(*graph.CopyRepr); ok {
						record.Path.Text = otherRepr.URLForCode
						record.Path.Namespace = ""
						record.CopySourceIndex = ast.Index32{}
						record.Flags |= ast.ShouldNotBeExternalInMetafile | ast.ContainsUniqueKey

						// Copy the additional files to the output directory
						additionalFiles = append(additionalFiles, otherFile.InputFile.AdditionalFiles...)
					}
				}
			}

			// Validate cross-file "composes: ... from" named imports
			for _, composes := range repr.AST.Composes {
				for _, name := range composes.ImportedNames {
					if record := repr.AST.ImportRecords[name.ImportRecordIndex]; record.SourceIndex.IsValid() {
						otherFile := &c.graph.Files[record.SourceIndex.GetIndex()]
						if otherRepr, ok := otherFile.InputFile.Repr.(*graph.CSSRepr); ok {
							if _, ok := otherRepr.AST.LocalScope[name.Alias]; !ok {
								if global, ok := otherRepr.AST.GlobalScope[name.Alias]; ok {
									var hint string
									if otherFile.InputFile.Loader == config.LoaderCSS {
										hint = fmt.Sprintf("Use the \"local-css\" loader for %q to enable local names.", otherFile.InputFile.Source.PrettyPath)
									} else {
										hint = fmt.Sprintf("Use the \":local\" selector to change %q into a local name.", name.Alias)
									}
									c.log.AddErrorWithNotes(file.LineColumnTracker(),
										css_lexer.RangeOfIdentifier(file.InputFile.Source, name.AliasLoc),
										fmt.Sprintf("Cannot use global name %q with \"composes\"", name.Alias),
										[]logger.MsgData{
											otherFile.LineColumnTracker().MsgData(
												css_lexer.RangeOfIdentifier(otherFile.InputFile.Source, global.Loc),
												fmt.Sprintf("The global name %q is defined here:", name.Alias),
											),
											{Text: hint},
										})
								} else {
									c.log.AddError(file.LineColumnTracker(),
										css_lexer.RangeOfIdentifier(file.InputFile.Source, name.AliasLoc),
										fmt.Sprintf("The name %q never appears in %q",
											name.Alias, otherFile.InputFile.Source.PrettyPath))
								}
							}
						}
					}
				}
			}

			c.validateComposesFromProperties(file, repr)

		case *graph.JSRepr:
			for importRecordIndex := range repr.AST.ImportRecords {
				record := &repr.AST.ImportRecords[importRecordIndex]
				if !record.SourceIndex.IsValid() {
					if record.CopySourceIndex.IsValid() {
						otherFile := &c.graph.Files[record.CopySourceIndex.GetIndex()]
						if otherRepr, ok := otherFile.InputFile.Repr.(*graph.CopyRepr); ok {
							record.Path.Text = otherRepr.URLForCode
							record.Path.Namespace = ""
							record.CopySourceIndex = ast.Index32{}
							record.Flags |= ast.ShouldNotBeExternalInMetafile | ast.ContainsUniqueKey

							// Copy the additional files to the output directory
							additionalFiles = append(additionalFiles, otherFile.InputFile.AdditionalFiles...)
						}
					}
					continue
				}

				otherFile := &c.graph.Files[record.SourceIndex.GetIndex()]
				otherRepr := otherFile.InputFile.Repr.(*graph.JSRepr)

				switch record.Kind {
				case ast.ImportStmt:
					// Importing using ES6 syntax from a file without any ES6 syntax
					// causes that module to be considered CommonJS-style, even if it
					// doesn't have any CommonJS exports.
					//
					// That means the ES6 imports will become undefined instead of
					// causing errors. This is for compatibility with older CommonJS-
					// style bundlers.
					//
					// We emit a warning in this case but try to avoid turning the module
					// into a CommonJS module if possible. This is possible with named
					// imports (the module stays an ECMAScript module but the imports are
					// rewritten with undefined) but is not possible with star or default
					// imports:
					//
					//   import * as ns from './empty-file'
					//   import defVal from './empty-file'
					//   console.log(ns, defVal)
					//
					// In that case the module *is* considered a CommonJS module because
					// the namespace object must be created.
					if (record.Flags.Has(ast.ContainsImportStar) || record.Flags.Has(ast.ContainsDefaultAlias)) &&
						otherRepr.AST.ExportsKind == js_ast.ExportsNone && !otherRepr.AST.HasLazyExport {
						otherRepr.Meta.Wrap = graph.WrapCJS
						otherRepr.AST.ExportsKind = js_ast.ExportsCommonJS
					}

				case ast.ImportRequire:
					// Files that are imported with require() must be wrapped so that
					// they can be lazily-evaluated
					if otherRepr.AST.ExportsKind == js_ast.ExportsESM {
						otherRepr.Meta.Wrap = graph.WrapESM
					} else {
						otherRepr.Meta.Wrap = graph.WrapCJS
						otherRepr.AST.ExportsKind = js_ast.ExportsCommonJS
					}

				case ast.ImportDynamic:
					if !c.options.CodeSplitting {
						// If we're not splitting, then import() is just a require() that
						// returns a promise, so the imported file must also be wrapped
						if otherRepr.AST.ExportsKind == js_ast.ExportsESM {
							otherRepr.Meta.Wrap = graph.WrapESM
						} else {
							otherRepr.Meta.Wrap = graph.WrapCJS
							otherRepr.AST.ExportsKind = js_ast.ExportsCommonJS
						}
					}
				}
			}

			// If the output format doesn't have an implicit CommonJS wrapper, any file
			// that uses CommonJS features will need to be wrapped, even though the
			// resulting wrapper won't be invoked by other files. An exception is made
			// for entry point files in CommonJS format (or when in pass-through mode).
			if repr.AST.ExportsKind == js_ast.ExportsCommonJS && (!file.IsEntryPoint() ||
				c.options.OutputFormat == config.FormatIIFE || c.options.OutputFormat == config.FormatESModule) {
				repr.Meta.Wrap = graph.WrapCJS
			}
		}

		file.InputFile.AdditionalFiles = additionalFiles
	}
	c.timer.End("Step 1")

	// Step 2: Propagate dynamic export status for export star statements that
	// are re-exports from a module whose exports are not statically analyzable.
	// In this case the export star must be evaluated at run time instead of at
	// bundle time.
	c.timer.Begin("Step 2")
	for _, sourceIndex := range c.graph.ReachableFiles {
		repr, ok := c.graph.Files[sourceIndex].InputFile.Repr.(*graph.JSRepr)
		if !ok {
			continue
		}

		if repr.Meta.Wrap != graph.WrapNone {
			c.recursivelyWrapDependencies(sourceIndex)
		}

		if len(repr.AST.ExportStarImportRecords) > 0 {
			visited := make(map[uint32]bool)
			c.hasDynamicExportsDueToExportStar(sourceIndex, visited)
		}

		// Even if the output file is CommonJS-like, we may still need to wrap
		// CommonJS-style files. Any file that imports a CommonJS-style file will
		// cause that file to need to be wrapped. This is because the import
		// method, whatever it is, will need to invoke the wrapper. Note that
		// this can include entry points (e.g. an entry point that imports a file
		// that imports that entry point).
		for _, record := range repr.AST.ImportRecords {
			if record.SourceIndex.IsValid() {
				otherRepr := c.graph.Files[record.SourceIndex.GetIndex()].InputFile.Repr.(*graph.JSRepr)
				if otherRepr.AST.ExportsKind == js_ast.ExportsCommonJS {
					c.recursivelyWrapDependencies(record.SourceIndex.GetIndex())
				}
			}
		}
	}
	c.timer.End("Step 2")

	// Step 3: Resolve "export * from" statements. This must be done after we
	// discover all modules that can have dynamic exports because export stars
	// are ignored for those modules.
	c.timer.Begin("Step 3")
	exportStarStack := make([]uint32, 0, 32)
	for _, sourceIndex := range c.graph.ReachableFiles {
		repr, ok := c.graph.Files[sourceIndex].InputFile.Repr.(*graph.JSRepr)
		if !ok {
			continue
		}

		// Expression-style loaders defer code generation until linking. Code
		// generation is done here because at this point we know that the
		// "ExportsKind" field has its final value and will not be changed.
		if repr.AST.HasLazyExport {
			c.generateCodeForLazyExport(sourceIndex)
		}

		// Propagate exports for export star statements
		if len(repr.AST.ExportStarImportRecords) > 0 {
			c.addExportsForExportStar(repr.Meta.ResolvedExports, sourceIndex, exportStarStack)
		}

		// Also add a special export so import stars can bind to it. This must be
		// done in this step because it must come after CommonJS module discovery
		// but before matching imports with exports.
		repr.Meta.ResolvedExportStar = &graph.ExportData{
			Ref:         repr.AST.ExportsRef,
			SourceIndex: sourceIndex,
		}
	}
	c.timer.End("Step 3")

	// Step 4: Match imports with exports. This must be done after we process all
	// export stars because imports can bind to export star re-exports.
	c.timer.Begin("Step 4")
	for _, sourceIndex := range c.graph.ReachableFiles {
		file := &c.graph.Files[sourceIndex]
		repr, ok := file.InputFile.Repr.(*graph.JSRepr)
		if !ok {
			continue
		}

		if len(repr.AST.NamedImports) > 0 {
			c.matchImportsWithExportsForFile(uint32(sourceIndex))
		}

		// If we're exporting as CommonJS and this file was originally CommonJS,
		// then we'll be using the actual CommonJS "exports" and/or "module"
		// symbols. In that case make sure to mark them as such so they don't
		// get minified.
		if file.IsEntryPoint() && repr.AST.ExportsKind == js_ast.ExportsCommonJS && repr.Meta.Wrap == graph.WrapNone &&
			(c.options.OutputFormat == config.FormatPreserve || c.options.OutputFormat == config.FormatCommonJS) {
			exportsRef := ast.FollowSymbols(c.graph.Symbols, repr.AST.ExportsRef)
			moduleRef := ast.FollowSymbols(c.graph.Symbols, repr.AST.ModuleRef)
			c.graph.Symbols.Get(exportsRef).Kind = ast.SymbolUnbound
			c.graph.Symbols.Get(moduleRef).Kind = ast.SymbolUnbound
		} else if repr.Meta.ForceIncludeExportsForEntryPoint || repr.AST.ExportsKind != js_ast.ExportsCommonJS {
			repr.Meta.NeedsExportsVariable = true
		}

		// Create the wrapper part for wrapped files. This is needed by a later step.
		c.createWrapperForFile(uint32(sourceIndex))
	}
	c.timer.End("Step 4")

	// Step 5: Create namespace exports for every file. This is always necessary
	// for CommonJS files, and is also necessary for other files if they are
	// imported using an import star statement.
	c.timer.Begin("Step 5")
	waitGroup := sync.WaitGroup{}
	for _, sourceIndex := range c.graph.ReachableFiles {
		repr, ok := c.graph.Files[sourceIndex].InputFile.Repr.(*graph.JSRepr)
		if !ok {
			continue
		}

		// This is the slowest step and is also parallelizable, so do this in parallel.
		waitGroup.Add(1)
		go func(sourceIndex uint32, repr *graph.JSRepr) {
			// Now that all exports have been resolved, sort and filter them to create
			// something we can iterate over later.
			aliases := make([]string, 0, len(repr.Meta.ResolvedExports))
		nextAlias:
			for alias, export := range repr.Meta.ResolvedExports {
				otherFile := &c.graph.Files[export.SourceIndex].InputFile
				otherRepr := otherFile.Repr.(*graph.JSRepr)

				// Re-exporting multiple symbols with the same name causes an ambiguous
				// export. These names cannot be used and should not end up in generated code.
				if len(export.PotentiallyAmbiguousExportStarRefs) > 0 {
					mainRef := export.Ref
					mainLoc := export.NameLoc
					if imported, ok := otherRepr.Meta.ImportsToBind[export.Ref]; ok {
						mainRef = imported.Ref
						mainLoc = imported.NameLoc
					}

					for _, ambiguousExport := range export.PotentiallyAmbiguousExportStarRefs {
						ambiguousFile := &c.graph.Files[ambiguousExport.SourceIndex].InputFile
						ambiguousRepr := ambiguousFile.Repr.(*graph.JSRepr)
						ambiguousRef := ambiguousExport.Ref
						ambiguousLoc := ambiguousExport.NameLoc
						if imported, ok := ambiguousRepr.Meta.ImportsToBind[ambiguousExport.Ref]; ok {
							ambiguousRef = imported.Ref
							ambiguousLoc = imported.NameLoc
						}

						if mainRef != ambiguousRef {
							file := &c.graph.Files[sourceIndex].InputFile
							otherTracker := logger.MakeLineColumnTracker(&otherFile.Source)
							ambiguousTracker := logger.MakeLineColumnTracker(&ambiguousFile.Source)
							c.log.AddIDWithNotes(logger.MsgID_Bundler_AmbiguousReexport, logger.Debug, nil, logger.Range{},
								fmt.Sprintf("Re-export of %q in %q is ambiguous and has been removed", alias, file.Source.PrettyPath),
								[]logger.MsgData{
									otherTracker.MsgData(js_lexer.RangeOfIdentifier(otherFile.Source, mainLoc),
										fmt.Sprintf("One definition of %q comes from %q here:", alias, otherFile.Source.PrettyPath)),
									ambiguousTracker.MsgData(js_lexer.RangeOfIdentifier(ambiguousFile.Source, ambiguousLoc),
										fmt.Sprintf("Another definition of %q comes from %q here:", alias, ambiguousFile.Source.PrettyPath)),
								},
							)
							continue nextAlias
						}
					}
				}

				// Ignore re-exported imports in TypeScript files that failed to be
				// resolved. These are probably just type-only imports so the best thing to
				// do is to silently omit them from the export list.
				if otherRepr.Meta.IsProbablyTypeScriptType[export.Ref] {
					continue
				}

				if c.options.OutputFormat == config.FormatESModule && c.options.UnsupportedJSFeatures.Has(compat.ArbitraryModuleNamespaceNames) && c.graph.Files[sourceIndex].IsEntryPoint() {
					c.maybeForbidArbitraryModuleNamespaceIdentifier("export", export.SourceIndex, export.NameLoc, alias)
				}

				aliases = append(aliases, alias)
			}
			sort.Strings(aliases)
			repr.Meta.SortedAndFilteredExportAliases = aliases

			// Export creation uses "sortedAndFilteredExportAliases" so this must
			// come second after we fill in that array
			c.createExportsForFile(uint32(sourceIndex))

			// Each part tracks the other parts it depends on within this file
			localDependencies := make(map[uint32]uint32)
			parts := repr.AST.Parts
			namedImports := repr.AST.NamedImports
			graph := c.graph
			for partIndex := range parts {
				part := &parts[partIndex]

				// Now that all files have been parsed, determine which property
				// accesses off of imported symbols are inlined enum values and
				// which ones aren't
				for ref, properties := range part.ImportSymbolPropertyUses {
					use := part.SymbolUses[ref]

					// Rare path: this import is a TypeScript enum
					if importData, ok := repr.Meta.ImportsToBind[ref]; ok {
						if symbol := graph.Symbols.Get(importData.Ref); symbol.Kind == ast.SymbolTSEnum {
							if enum, ok := graph.TSEnums[importData.Ref]; ok {
								foundNonInlinedEnum := false
								for name, propertyUse := range properties {
									if _, ok := enum[name]; !ok {
										foundNonInlinedEnum = true
										use.CountEstimate += propertyUse.CountEstimate
									}
								}
								if foundNonInlinedEnum {
									part.SymbolUses[ref] = use
								}
							}
							continue
						}
					}

					// Common path: this import isn't a TypeScript enum
					for _, propertyUse := range properties {
						use.CountEstimate += propertyUse.CountEstimate
					}
					part.SymbolUses[ref] = use
				}

				// Also determine which function calls will be inlined (and so should
				// not count as uses), and which ones will not be (and so should count
				// as uses)
				for ref, callUse := range part.SymbolCallUses {
					use := part.SymbolUses[ref]

					// Find the symbol that was called
					symbol := graph.Symbols.Get(ref)
					if symbol.Kind == ast.SymbolImport {
						if importData, ok := repr.Meta.ImportsToBind[ref]; ok {
							symbol = graph.Symbols.Get(importData.Ref)
						}
					}
					flags := symbol.Flags

					// Rare path: this is a function that will be inlined
					if (flags & (ast.IsEmptyFunction | ast.CouldPotentiallyBeMutated)) == ast.IsEmptyFunction {
						// Every call will be inlined
						continue
					} else if (flags & (ast.IsIdentityFunction | ast.CouldPotentiallyBeMutated)) == ast.IsIdentityFunction {
						// Every single-argument call will be inlined as long as it's not a spread
						callUse.CallCountEstimate -= callUse.SingleArgNonSpreadCallCountEstimate
						if callUse.CallCountEstimate == 0 {
							continue
						}
					}

					// Common path: this isn't a function that will be inlined
					use.CountEstimate += callUse.CallCountEstimate
					part.SymbolUses[ref] = use
				}

				// Now that we know this, we can determine cross-part dependencies
				for ref := range part.SymbolUses {

					// Rare path: this import is an inlined const value
					if graph.ConstValues != nil {
						if importData, ok := repr.Meta.ImportsToBind[ref]; ok {
							if _, isConstValue := graph.ConstValues[importData.Ref]; isConstValue {
								delete(part.SymbolUses, importData.Ref)
								continue
							}
						}
					}

					for _, otherPartIndex := range repr.TopLevelSymbolToParts(ref) {
						if oldPartIndex, ok := localDependencies[otherPartIndex]; !ok || oldPartIndex != uint32(partIndex) {
							localDependencies[otherPartIndex] = uint32(partIndex)
							part.Dependencies = append(part.Dependencies, js_ast.Dependency{
								SourceIndex: sourceIndex,
								PartIndex:   otherPartIndex,
							})
						}
					}

					// Also map from imports to parts that use them
					if namedImport, ok := namedImports[ref]; ok {
						namedImport.LocalPartsWithUses = append(namedImport.LocalPartsWithUses, uint32(partIndex))
						namedImports[ref] = namedImport
					}
				}
			}

			waitGroup.Done()
		}(sourceIndex, repr)
	}
	waitGroup.Wait()
	c.timer.End("Step 5")

	// Step 6: Bind imports to exports. This adds non-local dependencies on the
	// parts that declare the export to all parts that use the import. Also
	// generate wrapper parts for wrapped files.
	c.timer.Begin("Step 6")
	for _, sourceIndex := range c.graph.ReachableFiles {
		file := &c.graph.Files[sourceIndex]
		repr, ok := file.InputFile.Repr.(*graph.JSRepr)
		if !ok {
			continue
		}

		// Pre-generate symbols for re-exports CommonJS symbols in case they
		// are necessary later. This is done now because the symbols map cannot be
		// mutated later due to parallelism.
		if file.IsEntryPoint() && c.options.OutputFormat == config.FormatESModule {
			copies := make([]ast.Ref, len(repr.Meta.SortedAndFilteredExportAliases))
			for i, alias := range repr.Meta.SortedAndFilteredExportAliases {
				copies[i] = c.graph.GenerateNewSymbol(sourceIndex, ast.SymbolOther, "export_"+alias)
			}
			repr.Meta.CJSExportCopies = copies
		}

		// Use "init_*" for ESM wrappers instead of "require_*"
		if repr.Meta.Wrap == graph.WrapESM {
			c.graph.Symbols.Get(repr.AST.WrapperRef).OriginalName = "init_" + file.InputFile.Source.IdentifierName
		}

		// If this isn't CommonJS, then rename the unused "exports" and "module"
		// variables to avoid them causing the identically-named variables in
		// actual CommonJS files from being renamed. This is purely about
		// aesthetics and is not about correctness. This is done here because by
		// this point, we know the CommonJS status will not change further.
		if repr.Meta.Wrap != graph.WrapCJS && repr.AST.ExportsKind != js_ast.ExportsCommonJS {
			name := file.InputFile.Source.IdentifierName
			c.graph.Symbols.Get(repr.AST.ExportsRef).OriginalName = name + "_exports"
			c.graph.Symbols.Get(repr.AST.ModuleRef).OriginalName = name + "_module"
		}

		// Include the "__export" symbol from the runtime if it was used in the
		// previous step. The previous step can't do this because it's running in
		// parallel and can't safely mutate the "importsToBind" map of another file.
		if repr.Meta.NeedsExportSymbolFromRuntime {
			runtimeRepr := c.graph.Files[runtime.SourceIndex].InputFile.Repr.(*graph.JSRepr)
			exportRef := runtimeRepr.AST.ModuleScope.Members["__export"].Ref
			c.graph.GenerateSymbolImportAndUse(sourceIndex, js_ast.NSExportPartIndex, exportRef, 1, runtime.SourceIndex)
		}

		for importRef, importData := range repr.Meta.ImportsToBind {
			resolvedRepr := c.graph.Files[importData.SourceIndex].InputFile.Repr.(*graph.JSRepr)
			partsDeclaringSymbol := resolvedRepr.TopLevelSymbolToParts(importData.Ref)

			for _, partIndex := range repr.AST.NamedImports[importRef].LocalPartsWithUses {
				part := &repr.AST.Parts[partIndex]

				// Depend on the file containing the imported symbol
				for _, resolvedPartIndex := range partsDeclaringSymbol {
					part.Dependencies = append(part.Dependencies, js_ast.Dependency{
						SourceIndex: importData.SourceIndex,
						PartIndex:   resolvedPartIndex,
					})
				}

				// Also depend on any files that re-exported this symbol in between the
				// file containing the import and the file containing the imported symbol
				part.Dependencies = append(part.Dependencies, importData.ReExports...)
			}

			// Merge these symbols so they will share the same name
			ast.MergeSymbols(c.graph.Symbols, importRef, importData.Ref)
		}

		// If this is an entry point, depend on all exports so they are included
		if file.IsEntryPoint() {
			var dependencies []js_ast.Dependency

			for _, alias := range repr.Meta.SortedAndFilteredExportAliases {
				export := repr.Meta.ResolvedExports[alias]
				targetSourceIndex := export.SourceIndex
				targetRef := export.Ref

				// If this is an import, then target what the import points to
				targetRepr := c.graph.Files[targetSourceIndex].InputFile.Repr.(*graph.JSRepr)
				if importData, ok := targetRepr.Meta.ImportsToBind[targetRef]; ok {
					targetSourceIndex = importData.SourceIndex
					targetRef = importData.Ref
					targetRepr = c.graph.Files[targetSourceIndex].InputFile.Repr.(*graph.JSRepr)
					dependencies = append(dependencies, importData.ReExports...)
				}

				// Pull in all declarations of this symbol
				for _, partIndex := range targetRepr.TopLevelSymbolToParts(targetRef) {
					dependencies = append(dependencies, js_ast.Dependency{
						SourceIndex: targetSourceIndex,
						PartIndex:   partIndex,
					})
				}
			}

			// Ensure "exports" is included if the current output format needs it
			if repr.Meta.ForceIncludeExportsForEntryPoint {
				dependencies = append(dependencies, js_ast.Dependency{
					SourceIndex: sourceIndex,
					PartIndex:   js_ast.NSExportPartIndex,
				})
			}

			// Include the wrapper if present
			if repr.Meta.Wrap != graph.WrapNone {
				dependencies = append(dependencies, js_ast.Dependency{
					SourceIndex: sourceIndex,
					PartIndex:   repr.Meta.WrapperPartIndex.GetIndex(),
				})
			}

			// Represent these constraints with a dummy part
			entryPointPartIndex := c.graph.AddPartToFile(sourceIndex, js_ast.Part{
				Dependencies:         dependencies,
				CanBeRemovedIfUnused: false,
			})
			repr.Meta.EntryPointPartIndex = ast.MakeIndex32(entryPointPartIndex)

			// Pull in the "__toCommonJS" symbol if we need it due to being an entry point
			if repr.Meta.ForceIncludeExportsForEntryPoint {
				c.graph.GenerateRuntimeSymbolImportAndUse(sourceIndex, entryPointPartIndex, "__toCommonJS", 1)
			}
		}

		// Encode import-specific constraints in the dependency graph
		for partIndex, part := range repr.AST.Parts {
			toESMUses := uint32(0)
			toCommonJSUses := uint32(0)
			runtimeRequireUses := uint32(0)

			// Imports of wrapped files must depend on the wrapper
			for _, importRecordIndex := range part.ImportRecordIndices {
				record := &repr.AST.ImportRecords[importRecordIndex]

				// Don't follow external imports (this includes import() expressions)
				if !record.SourceIndex.IsValid() || c.isExternalDynamicImport(record, sourceIndex) {
					// This is an external import. Check if it will be a "require()" call.
					if record.Kind == ast.ImportRequire || !c.options.OutputFormat.KeepESMImportExportSyntax() ||
						(record.Kind == ast.ImportDynamic && c.options.UnsupportedJSFeatures.Has(compat.DynamicImport)) {
						// We should use "__require" instead of "require" if we're not
						// generating a CommonJS output file, since it won't exist otherwise
						if config.ShouldCallRuntimeRequire(c.options.Mode, c.options.OutputFormat) {
							record.Flags |= ast.CallRuntimeRequire
							runtimeRequireUses++
						}

						// If this wasn't originally a "require()" call, then we may need
						// to wrap this in a call to the "__toESM" wrapper to convert from
						// CommonJS semantics to ESM semantics.
						//
						// Unfortunately this adds some additional code since the conversion
						// is somewhat complex. As an optimization, we can avoid this if the
						// following things are true:
						//
						// - The import is an ES module statement (e.g. not an "import()" expression)
						// - The ES module namespace object must not be captured
						// - The "default" and "__esModule" exports must not be accessed
						//
						if record.Kind != ast.ImportRequire &&
							(record.Kind != ast.ImportStmt ||
								record.Flags.Has(ast.ContainsImportStar) ||
								record.Flags.Has(ast.ContainsDefaultAlias) ||
								record.Flags.Has(ast.ContainsESModuleAlias)) {
							record.Flags |= ast.WrapWithToESM
							toESMUses++
						}
					}
					continue
				}

				otherSourceIndex := record.SourceIndex.GetIndex()
				otherRepr := c.graph.Files[otherSourceIndex].InputFile.Repr.(*graph.JSRepr)

				if otherRepr.Meta.Wrap != graph.WrapNone {
					// Depend on the automatically-generated require wrapper symbol
					wrapperRef := otherRepr.AST.WrapperRef
					c.graph.GenerateSymbolImportAndUse(sourceIndex, uint32(partIndex), wrapperRef, 1, otherSourceIndex)

					// This is an ES6 import of a CommonJS module, so it needs the
					// "__toESM" wrapper as long as it's not a bare "require()"
					if record.Kind != ast.ImportRequire && otherRepr.AST.ExportsKind == js_ast.ExportsCommonJS {
						record.Flags |= ast.WrapWithToESM
						toESMUses++
					}

					// If this is an ESM wrapper, also depend on the exports object
					// since the final code will contain an inline reference to it.
					// This must be done for "require()" and "import()" expressions
					// but does not need to be done for "import" statements since
					// those just cause us to reference the exports directly.
					if otherRepr.Meta.Wrap == graph.WrapESM && record.Kind != ast.ImportStmt {
						c.graph.GenerateSymbolImportAndUse(sourceIndex, uint32(partIndex), otherRepr.AST.ExportsRef, 1, otherSourceIndex)

						// If this is a "require()" call, then we should add the
						// "__esModule" marker to behave as if the module was converted
						// from ESM to CommonJS. This is done via a wrapper instead of
						// by modifying the exports object itself because the same ES
						// module may be simultaneously imported and required, and the
						// importing code should not see "__esModule" while the requiring
						// code should see "__esModule". This is an extremely complex
						// and subtle set of bundler interop issues. See for example
						// https://github.com/evanw/esbuild/issues/1591.
						if record.Kind == ast.ImportRequire {
							record.Flags |= ast.WrapWithToCJS
							toCommonJSUses++
						}
					}
				} else if record.Kind == ast.ImportStmt && otherRepr.AST.ExportsKind == js_ast.ExportsESMWithDynamicFallback {
					// This is an import of a module that has a dynamic export fallback
					// object. In that case we need to depend on that object in case
					// something ends up needing to use it later. This could potentially
					// be omitted in some cases with more advanced analysis if this
					// dynamic export fallback object doesn't end up being needed.
					c.graph.GenerateSymbolImportAndUse(sourceIndex, uint32(partIndex), otherRepr.AST.ExportsRef, 1, otherSourceIndex)
				}
			}

			// If there's an ES6 import of a non-ES6 module, then we're going to need the
			// "__toESM" symbol from the runtime to wrap the result of "require()"
			c.graph.GenerateRuntimeSymbolImportAndUse(sourceIndex, uint32(partIndex), "__toESM", toESMUses)

			// If there's a CommonJS require of an ES6 module, then we're going to need the
			// "__toCommonJS" symbol from the runtime to wrap the exports object
			c.graph.GenerateRuntimeSymbolImportAndUse(sourceIndex, uint32(partIndex), "__toCommonJS", toCommonJSUses)

			// If there are unbundled calls to "require()" and we're not generating
			// code for node, then substitute a "__require" wrapper for "require".
			c.graph.GenerateRuntimeSymbolImportAndUse(sourceIndex, uint32(partIndex), "__require", runtimeRequireUses)

			// If there's an ES6 export star statement of a non-ES6 module, then we're
			// going to need the "__reExport" symbol from the runtime
			reExportUses := uint32(0)
			for _, importRecordIndex := range repr.AST.ExportStarImportRecords {
				record := &repr.AST.ImportRecords[importRecordIndex]

				// Is this export star evaluated at run time?
				happensAtRunTime := !record.SourceIndex.IsValid() && (!file.IsEntryPoint() || !c.options.OutputFormat.KeepESMImportExportSyntax())
				if record.SourceIndex.IsValid() {
					otherSourceIndex := record.SourceIndex.GetIndex()
					otherRepr := c.graph.Files[otherSourceIndex].InputFile.Repr.(*graph.JSRepr)
					if otherSourceIndex != sourceIndex && otherRepr.AST.ExportsKind.IsDynamic() {
						happensAtRunTime = true
					}
					if otherRepr.AST.ExportsKind == js_ast.ExportsESMWithDynamicFallback {
						// This looks like "__reExport(exports_a, exports_b)". Make sure to
						// pull in the "exports_b" symbol into this export star. This matters
						// in code splitting situations where the "export_b" symbol might live
						// in a different chunk than this export star.
						c.graph.GenerateSymbolImportAndUse(sourceIndex, uint32(partIndex), otherRepr.AST.ExportsRef, 1, otherSourceIndex)
					}
				}
				if happensAtRunTime {
					// Depend on this file's "exports" object for the first argument to "__reExport"
					c.graph.GenerateSymbolImportAndUse(sourceIndex, uint32(partIndex), repr.AST.ExportsRef, 1, sourceIndex)
					record.Flags |= ast.CallsRunTimeReExportFn
					repr.AST.UsesExportsRef = true
					reExportUses++
				}
			}
			c.graph.GenerateRuntimeSymbolImportAndUse(sourceIndex, uint32(partIndex), "__reExport", reExportUses)
		}
	}
	c.timer.End("Step 6")
}

func (c *linkerContext) validateComposesFromProperties(rootFile *graph.LinkerFile, rootRepr *graph.CSSRepr) {
	for _, local := range rootRepr.AST.LocalSymbols {
		type propertyInFile struct {
			file *graph.LinkerFile
			loc  logger.Loc
		}

		visited := make(map[ast.Ref]bool)
		properties := make(map[string]propertyInFile)
		var visit func(*graph.LinkerFile, *graph.CSSRepr, ast.Ref)

		visit = func(file *graph.LinkerFile, repr *graph.CSSRepr, ref ast.Ref) {
			if visited[ref] {
				return
			}
			visited[ref] = true

			composes, ok := repr.AST.Composes[ref]
			if !ok {
				return
			}

			for _, name := range composes.ImportedNames {
				if record := repr.AST.ImportRecords[name.ImportRecordIndex]; record.SourceIndex.IsValid() {
					otherFile := &c.graph.Files[record.SourceIndex.GetIndex()]
					if otherRepr, ok := otherFile.InputFile.Repr.(*graph.CSSRepr); ok {
						if otherName, ok := otherRepr.AST.LocalScope[name.Alias]; ok {
							visit(otherFile, otherRepr, otherName.Ref)
						}
					}
				}
			}

			for _, name := range composes.Names {
				visit(file, repr, name.Ref)
			}

			// Warn about cross-file composition with the same CSS properties
			for keyText, keyLoc := range composes.Properties {
				property, ok := properties[keyText]
				if !ok {
					properties[keyText] = propertyInFile{file, keyLoc}
					continue
				}
				if property.file == file || property.file == nil {
					continue
				}

				localOriginalName := c.graph.Symbols.Get(local.Ref).OriginalName
				c.log.AddMsgID(logger.MsgID_CSS_UndefinedComposesFrom, logger.Msg{
					Kind: logger.Warning,
					Data: rootFile.LineColumnTracker().MsgData(
						css_lexer.RangeOfIdentifier(rootFile.InputFile.Source, local.Loc),
						fmt.Sprintf("The value of %q in the %q class is undefined", keyText, localOriginalName),
					),
					Notes: []logger.MsgData{
						property.file.LineColumnTracker().MsgData(
							css_lexer.RangeOfIdentifier(property.file.InputFile.Source, property.loc),
							fmt.Sprintf("The first definition of %q is here:", keyText),
						),
						file.LineColumnTracker().MsgData(
							css_lexer.RangeOfIdentifier(file.InputFile.Source, keyLoc),
							fmt.Sprintf("The second definition of %q is here:", keyText),
						),
						{Text: fmt.Sprintf("The specification of \"composes\" does not define an order when class declarations from separate files are composed together. "+
							"The value of the %q property for %q may change unpredictably as the code is edited. "+
							"Make sure that all definitions of %q for %q are in a single file.", keyText, localOriginalName, keyText, localOriginalName)},
					},
				})

				// Don't warn more than once
				property.file = nil
				properties[keyText] = property
			}
		}

		visit(rootFile, rootRepr, local.Ref)
	}
}

func (c *linkerContext) generateCodeForLazyExport(sourceIndex uint32) {
	file := &c.graph.Files[sourceIndex]
	repr := file.InputFile.Repr.(*graph.JSRepr)

	// Grab the lazy expression
	if len(repr.AST.Parts) < 1 {
		panic("Internal error")
	}
	part := &repr.AST.Parts[len(repr.AST.Parts)-1]
	if len(part.Stmts) != 1 {
		panic("Internal error")
	}
	lazyValue := part.Stmts[0].Data.(*js_ast.SLazyExport).Value

	// If this JavaScript file is a stub from a CSS file, populate the exports of
	// this JavaScript stub with the local names from that CSS file. This is done
	// now instead of earlier because we need the whole bundle to be present.
	if repr.CSSSourceIndex.IsValid() {
		cssSourceIndex := repr.CSSSourceIndex.GetIndex()
		if css, ok := c.graph.Files[cssSourceIndex].InputFile.Repr.(*graph.CSSRepr); ok {
			exports := js_ast.EObject{}

			for _, local := range css.AST.LocalSymbols {
				value := js_ast.Expr{Loc: local.Loc, Data: &js_ast.ENameOfSymbol{Ref: local.Ref}}
				visited := map[ast.Ref]bool{local.Ref: true}
				var parts []js_ast.TemplatePart
				var visitName func(*graph.CSSRepr, ast.Ref)
				var visitComposes func(*graph.CSSRepr, ast.Ref)

				visitName = func(repr *graph.CSSRepr, ref ast.Ref) {
					if !visited[ref] {
						visited[ref] = true
						visitComposes(repr, ref)
						parts = append(parts, js_ast.TemplatePart{
							Value:      js_ast.Expr{Data: &js_ast.ENameOfSymbol{Ref: ref}},
							TailCooked: []uint16{' '},
						})
					}
				}

				visitComposes = func(repr *graph.CSSRepr, ref ast.Ref) {
					if composes, ok := repr.AST.Composes[ref]; ok {
						for _, name := range composes.ImportedNames {
							if record := repr.AST.ImportRecords[name.ImportRecordIndex]; record.SourceIndex.IsValid() {
								otherFile := &c.graph.Files[record.SourceIndex.GetIndex()]
								if otherRepr, ok := otherFile.InputFile.Repr.(*graph.CSSRepr); ok {
									if otherName, ok := otherRepr.AST.LocalScope[name.Alias]; ok {
										visitName(otherRepr, otherName.Ref)
									}
								}
							}
						}

						for _, name := range composes.Names {
							visitName(repr, name.Ref)
						}
					}
				}

				visitComposes(css, local.Ref)

				if len(parts) > 0 {
					value.Data = &js_ast.ETemplate{Parts: append(parts, js_ast.TemplatePart{Value: value})}
				}

				exports.Properties = append(exports.Properties, js_ast.Property{
					Key:        js_ast.Expr{Loc: local.Loc, Data: &js_ast.EString{Value: helpers.StringToUTF16(c.graph.Symbols.Get(local.Ref).OriginalName)}},
					ValueOrNil: value,
				})
			}

			lazyValue.Data = &exports
		}
	}

	// Use "module.exports = value" for CommonJS-style modules
	if repr.AST.ExportsKind == js_ast.ExportsCommonJS {
		part.Stmts = []js_ast.Stmt{js_ast.AssignStmt(
			js_ast.Expr{Loc: lazyValue.Loc, Data: &js_ast.EDot{
				Target:  js_ast.Expr{Loc: lazyValue.Loc, Data: &js_ast.EIdentifier{Ref: repr.AST.ModuleRef}},
				Name:    "exports",
				NameLoc: lazyValue.Loc,
			}},
			lazyValue,
		)}
		c.graph.GenerateSymbolImportAndUse(sourceIndex, 0, repr.AST.ModuleRef, 1, sourceIndex)
		return
	}

	// Otherwise, generate ES6 export statements. These are added as additional
	// parts so they can be tree shaken individually.
	part.Stmts = nil

	// Generate a new symbol and link the export into the graph for tree shaking
	generateExport := func(loc logger.Loc, name string, alias string) (ast.Ref, uint32) {
		ref := c.graph.GenerateNewSymbol(sourceIndex, ast.SymbolOther, name)
		partIndex := c.graph.AddPartToFile(sourceIndex, js_ast.Part{
			DeclaredSymbols:      []js_ast.DeclaredSymbol{{Ref: ref, IsTopLevel: true}},
			CanBeRemovedIfUnused: true,
		})
		c.graph.GenerateSymbolImportAndUse(sourceIndex, partIndex, repr.AST.ModuleRef, 1, sourceIndex)
		repr.Meta.TopLevelSymbolToPartsOverlay[ref] = []uint32{partIndex}
		repr.Meta.ResolvedExports[alias] = graph.ExportData{
			Ref:         ref,
			NameLoc:     loc,
			SourceIndex: sourceIndex,
		}
		return ref, partIndex
	}

	// Unwrap JSON objects into separate top-level variables. This improves tree-
	// shaking by letting you only import part of a JSON file.
	//
	// But don't do this for files loaded via "with { type: 'json' }" as that
	// behavior is specified to not export anything except for the "default"
	// export: https://github.com/tc39/proposal-json-modules
	if object, ok := lazyValue.Data.(*js_ast.EObject); ok && file.InputFile.Loader != config.LoaderWithTypeJSON {
		for _, property := range object.Properties {
			if str, ok := property.Key.Data.(*js_ast.EString); ok &&
				(!file.IsEntryPoint() || js_ast.IsIdentifierUTF16(str.Value) ||
					!c.options.UnsupportedJSFeatures.Has(compat.ArbitraryModuleNamespaceNames)) {
				if name := helpers.UTF16ToString(str.Value); name != "default" {
					ref, partIndex := generateExport(property.Key.Loc, name, name)

					// This initializes the generated variable with a copy of the property
					// value, which is INCORRECT for values that are objects/arrays because
					// they will have separate object identity. This is fixed up later in
					// "generateCodeForFileInChunkJS" by changing the object literal to
					// reference this generated variable instead.
					//
					// Changing the object literal is deferred until that point instead of
					// doing it now because we only want to do this for top-level variables
					// that actually end up being used, and we don't know which ones will
					// end up actually being used at this point (since import binding hasn't
					// happened yet). So we need to wait until after tree shaking happens.
					repr.AST.Parts[partIndex].Stmts = []js_ast.Stmt{{Loc: property.Key.Loc, Data: &js_ast.SLocal{
						IsExport: true,
						Decls: []js_ast.Decl{{
							Binding:    js_ast.Binding{Loc: property.Key.Loc, Data: &js_ast.BIdentifier{Ref: ref}},
							ValueOrNil: property.ValueOrNil,
						}},
					}}}
				}
			}
		}
	}

	// Generate the default export
	ref, partIndex := generateExport(lazyValue.Loc, file.InputFile.Source.IdentifierName+"_default", "default")
	repr.AST.Parts[partIndex].Stmts = []js_ast.Stmt{{Loc: lazyValue.Loc, Data: &js_ast.SExportDefault{
		DefaultName: ast.LocRef{Loc: lazyValue.Loc, Ref: ref},
		Value:       js_ast.Stmt{Loc: lazyValue.Loc, Data: &js_ast.SExpr{Value: lazyValue}},
	}}}
}

func (c *linkerContext) createExportsForFile(sourceIndex uint32) {
	////////////////////////////////////////////////////////////////////////////////
	// WARNING: This method is run in parallel over all files. Do not mutate data
	// for other files within this method or you will create a data race.
	////////////////////////////////////////////////////////////////////////////////

	file := &c.graph.Files[sourceIndex]
	repr := file.InputFile.Repr.(*graph.JSRepr)

	// Generate a getter per export
	properties := []js_ast.Property{}
	nsExportDependencies := []js_ast.Dependency{}
	nsExportSymbolUses := make(map[ast.Ref]js_ast.SymbolUse)
	for _, alias := range repr.Meta.SortedAndFilteredExportAliases {
		export := repr.Meta.ResolvedExports[alias]

		// If this is an export of an import, reference the symbol that the import
		// was eventually resolved to. We need to do this because imports have
		// already been resolved by this point, so we can't generate a new import
		// and have that be resolved later.
		if importData, ok := c.graph.Files[export.SourceIndex].InputFile.Repr.(*graph.JSRepr).Meta.ImportsToBind[export.Ref]; ok {
			export.Ref = importData.Ref
			export.SourceIndex = importData.SourceIndex
			nsExportDependencies = append(nsExportDependencies, importData.ReExports...)
		}

		// Exports of imports need EImportIdentifier in case they need to be re-
		// written to a property access later on
		var value js_ast.Expr
		if c.graph.Symbols.Get(export.Ref).NamespaceAlias != nil {
			value = js_ast.Expr{Data: &js_ast.EImportIdentifier{Ref: export.Ref}}
		} else {
			value = js_ast.Expr{Data: &js_ast.EIdentifier{Ref: export.Ref}}
		}

		// Add a getter property
		var getter js_ast.Expr
		body := js_ast.FnBody{Block: js_ast.SBlock{Stmts: []js_ast.Stmt{{Loc: value.Loc, Data: &js_ast.SReturn{ValueOrNil: value}}}}}
		if c.options.UnsupportedJSFeatures.Has(compat.Arrow) {
			getter = js_ast.Expr{Data: &js_ast.EFunction{Fn: js_ast.Fn{Body: body}}}
		} else {
			getter = js_ast.Expr{Data: &js_ast.EArrow{PreferExpr: true, Body: body}}
		}
		properties = append(properties, js_ast.Property{
			Key:        js_ast.Expr{Data: &js_ast.EString{Value: helpers.StringToUTF16(alias)}},
			ValueOrNil: getter,
		})
		nsExportSymbolUses[export.Ref] = js_ast.SymbolUse{CountEstimate: 1}

		// Make sure the part that declares the export is included
		for _, partIndex := range c.graph.Files[export.SourceIndex].InputFile.Repr.(*graph.JSRepr).TopLevelSymbolToParts(export.Ref) {
			// Use a non-local dependency since this is likely from a different
			// file if it came in through an export star
			nsExportDependencies = append(nsExportDependencies, js_ast.Dependency{
				SourceIndex: export.SourceIndex,
				PartIndex:   partIndex,
			})
		}
	}

	declaredSymbols := []js_ast.DeclaredSymbol{}
	var nsExportStmts []js_ast.Stmt

	// Prefix this part with "var exports = {}" if this isn't a CommonJS entry point
	if repr.Meta.NeedsExportsVariable {
		nsExportStmts = append(nsExportStmts, js_ast.Stmt{Data: &js_ast.SLocal{Decls: []js_ast.Decl{{
			Binding:    js_ast.Binding{Data: &js_ast.BIdentifier{Ref: repr.AST.ExportsRef}},
			ValueOrNil: js_ast.Expr{Data: &js_ast.EObject{}},
		}}}})
		declaredSymbols = append(declaredSymbols, js_ast.DeclaredSymbol{
			Ref:        repr.AST.ExportsRef,
			IsTopLevel: true,
		})
	}

	// "__export(exports, { foo: () => foo })"
	exportRef := ast.InvalidRef
	if len(properties) > 0 {
		runtimeRepr := c.graph.Files[runtime.SourceIndex].InputFile.Repr.(*graph.JSRepr)
		exportRef = runtimeRepr.AST.ModuleScope.Members["__export"].Ref
		nsExportStmts = append(nsExportStmts, js_ast.Stmt{Data: &js_ast.SExpr{Value: js_ast.Expr{Data: &js_ast.ECall{
			Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: exportRef}},
			Args: []js_ast.Expr{
				{Data: &js_ast.EIdentifier{Ref: repr.AST.ExportsRef}},
				{Data: &js_ast.EObject{
					Properties: properties,
				}},
			},
		}}}})

		// Make sure this file depends on the "__export" symbol
		for _, partIndex := range runtimeRepr.TopLevelSymbolToParts(exportRef) {
			nsExportDependencies = append(nsExportDependencies, js_ast.Dependency{
				SourceIndex: runtime.SourceIndex,
				PartIndex:   partIndex,
			})
		}

		// Make sure the CommonJS closure, if there is one, includes "exports"
		repr.AST.UsesExportsRef = true
	}

	// Decorate "module.exports" with the "__esModule" flag to indicate that
	// we used to be an ES module. This is done by wrapping the exports object
	// instead of by mutating the exports object because other modules in the
	// bundle (including the entry point module) may do "import * as" to get
	// access to the exports object and should NOT see the "__esModule" flag.
	if repr.Meta.ForceIncludeExportsForEntryPoint &&
		c.options.OutputFormat == config.FormatCommonJS {

		runtimeRepr := c.graph.Files[runtime.SourceIndex].InputFile.Repr.(*graph.JSRepr)
		toCommonJSRef := runtimeRepr.AST.NamedExports["__toCommonJS"].Ref

		// "module.exports = __toCommonJS(exports);"
		nsExportStmts = append(nsExportStmts, js_ast.AssignStmt(
			js_ast.Expr{Data: &js_ast.EDot{
				Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: c.unboundModuleRef}},
				Name:   "exports",
			}},

			js_ast.Expr{Data: &js_ast.ECall{
				Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: toCommonJSRef}},
				Args:   []js_ast.Expr{{Data: &js_ast.EIdentifier{Ref: repr.AST.ExportsRef}}},
			}},
		))
	}

	// No need to generate a part if it'll be empty
	if len(nsExportStmts) > 0 {
		// Initialize the part that was allocated for us earlier. The information
		// here will be used after this during tree shaking.
		repr.AST.Parts[js_ast.NSExportPartIndex] = js_ast.Part{
			Stmts:           nsExportStmts,
			SymbolUses:      nsExportSymbolUses,
			Dependencies:    nsExportDependencies,
			DeclaredSymbols: declaredSymbols,

			// This can be removed if nothing uses it
			CanBeRemovedIfUnused: true,

			// Make sure this is trimmed if unused even if tree shaking is disabled
			ForceTreeShaking: true,
		}

		// Pull in the "__export" symbol if it was used
		if exportRef != ast.InvalidRef {
			repr.Meta.NeedsExportSymbolFromRuntime = true
		}
	}
}

func (c *linkerContext) createWrapperForFile(sourceIndex uint32) {
	repr := c.graph.Files[sourceIndex].InputFile.Repr.(*graph.JSRepr)

	switch repr.Meta.Wrap {
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
	case graph.WrapCJS:
		runtimeRepr := c.graph.Files[runtime.SourceIndex].InputFile.Repr.(*graph.JSRepr)
		commonJSParts := runtimeRepr.TopLevelSymbolToParts(c.cjsRuntimeRef)

		// Generate the dummy part
		dependencies := make([]js_ast.Dependency, len(commonJSParts))
		for i, partIndex := range commonJSParts {
			dependencies[i] = js_ast.Dependency{
				SourceIndex: runtime.SourceIndex,
				PartIndex:   partIndex,
			}
		}
		partIndex := c.graph.AddPartToFile(sourceIndex, js_ast.Part{
			SymbolUses: map[ast.Ref]js_ast.SymbolUse{
				repr.AST.WrapperRef: {CountEstimate: 1},
			},
			DeclaredSymbols: []js_ast.DeclaredSymbol{
				{Ref: repr.AST.ExportsRef, IsTopLevel: true},
				{Ref: repr.AST.ModuleRef, IsTopLevel: true},
				{Ref: repr.AST.WrapperRef, IsTopLevel: true},
			},
			Dependencies: dependencies,
		})
		repr.Meta.WrapperPartIndex = ast.MakeIndex32(partIndex)
		c.graph.GenerateSymbolImportAndUse(sourceIndex, partIndex, c.cjsRuntimeRef, 1, runtime.SourceIndex)

	// If this is a lazily-initialized ESM file, we're going to need to
	// generate a wrapper for the ESM closure. That will end up looking
	// something like this:
	//
	//   var init_foo = __esm(() => {
	//     ...
	//   });
	//
	// This depends on the "__esm" symbol and declares the "init_foo" symbol
	// for similar reasons to the CommonJS closure above.
	case graph.WrapESM:
		runtimeRepr := c.graph.Files[runtime.SourceIndex].InputFile.Repr.(*graph.JSRepr)
		esmParts := runtimeRepr.TopLevelSymbolToParts(c.esmRuntimeRef)

		// Generate the dummy part
		dependencies := make([]js_ast.Dependency, len(esmParts))
		for i, partIndex := range esmParts {
			dependencies[i] = js_ast.Dependency{
				SourceIndex: runtime.SourceIndex,
				PartIndex:   partIndex,
			}
		}
		partIndex := c.graph.AddPartToFile(sourceIndex, js_ast.Part{
			SymbolUses: map[ast.Ref]js_ast.SymbolUse{
				repr.AST.WrapperRef: {CountEstimate: 1},
			},
			DeclaredSymbols: []js_ast.DeclaredSymbol{
				{Ref: repr.AST.WrapperRef, IsTopLevel: true},
			},
			Dependencies: dependencies,
		})
		repr.Meta.WrapperPartIndex = ast.MakeIndex32(partIndex)
		c.graph.GenerateSymbolImportAndUse(sourceIndex, partIndex, c.esmRuntimeRef, 1, runtime.SourceIndex)
	}
}

func (c *linkerContext) matchImportsWithExportsForFile(sourceIndex uint32) {
	file := &c.graph.Files[sourceIndex]
	repr := file.InputFile.Repr.(*graph.JSRepr)

	// Sort imports for determinism. Otherwise our unit tests will randomly
	// fail sometimes when error messages are reordered.
	sortedImportRefs := make([]int, 0, len(repr.AST.NamedImports))
	for ref := range repr.AST.NamedImports {
		sortedImportRefs = append(sortedImportRefs, int(ref.InnerIndex))
	}
	sort.Ints(sortedImportRefs)

	// Pair imports with their matching exports
	for _, innerIndex := range sortedImportRefs {
		// Re-use memory for the cycle detector
		c.cycleDetector = c.cycleDetector[:0]

		importRef := ast.Ref{SourceIndex: sourceIndex, InnerIndex: uint32(innerIndex)}
		result, reExports := c.matchImportWithExport(importTracker{sourceIndex: sourceIndex, importRef: importRef}, nil)
		switch result.kind {
		case matchImportIgnore:

		case matchImportNormal:
			repr.Meta.ImportsToBind[importRef] = graph.ImportData{
				ReExports:   reExports,
				SourceIndex: result.sourceIndex,
				Ref:         result.ref,
			}

		case matchImportNamespace:
			c.graph.Symbols.Get(importRef).NamespaceAlias = &ast.NamespaceAlias{
				NamespaceRef: result.namespaceRef,
				Alias:        result.alias,
			}

		case matchImportNormalAndNamespace:
			repr.Meta.ImportsToBind[importRef] = graph.ImportData{
				ReExports:   reExports,
				SourceIndex: result.sourceIndex,
				Ref:         result.ref,
			}

			c.graph.Symbols.Get(importRef).NamespaceAlias = &ast.NamespaceAlias{
				NamespaceRef: result.namespaceRef,
				Alias:        result.alias,
			}

		case matchImportCycle:
			namedImport := repr.AST.NamedImports[importRef]
			c.log.AddError(file.LineColumnTracker(), js_lexer.RangeOfIdentifier(file.InputFile.Source, namedImport.AliasLoc),
				fmt.Sprintf("Detected cycle while resolving import %q", namedImport.Alias))

		case matchImportProbablyTypeScriptType:
			repr.Meta.IsProbablyTypeScriptType[importRef] = true

		case matchImportAmbiguous:
			namedImport := repr.AST.NamedImports[importRef]
			r := js_lexer.RangeOfIdentifier(file.InputFile.Source, namedImport.AliasLoc)
			var notes []logger.MsgData

			// Provide the locations of both ambiguous exports if possible
			if result.nameLoc.Start != 0 && result.otherNameLoc.Start != 0 {
				a := c.graph.Files[result.sourceIndex]
				b := c.graph.Files[result.otherSourceIndex]
				ra := js_lexer.RangeOfIdentifier(a.InputFile.Source, result.nameLoc)
				rb := js_lexer.RangeOfIdentifier(b.InputFile.Source, result.otherNameLoc)
				notes = []logger.MsgData{
					a.LineColumnTracker().MsgData(ra, "One matching export is here:"),
					b.LineColumnTracker().MsgData(rb, "Another matching export is here:"),
				}
			}

			symbol := c.graph.Symbols.Get(importRef)
			if symbol.ImportItemStatus == ast.ImportItemGenerated {
				// This is a warning instead of an error because although it appears
				// to be a named import, it's actually an automatically-generated
				// named import that was originally a property access on an import
				// star namespace object. Normally this property access would just
				// resolve to undefined at run-time instead of failing at binding-
				// time, so we emit a warning and rewrite the value to the literal
				// "undefined" instead of emitting an error.
				symbol.ImportItemStatus = ast.ImportItemMissing
				msg := fmt.Sprintf("Import %q will always be undefined because there are multiple matching exports", namedImport.Alias)
				c.log.AddIDWithNotes(logger.MsgID_Bundler_ImportIsUndefined, logger.Warning, file.LineColumnTracker(), r, msg, notes)
			} else {
				msg := fmt.Sprintf("Ambiguous import %q has multiple matching exports", namedImport.Alias)
				c.log.AddErrorWithNotes(file.LineColumnTracker(), r, msg, notes)
			}
		}
	}
}

type matchImportKind uint8

const (
	// The import is either external or undefined
	matchImportIgnore matchImportKind = iota

	// "sourceIndex" and "ref" are in use
	matchImportNormal

	// "namespaceRef" and "alias" are in use
	matchImportNamespace

	// Both "matchImportNormal" and "matchImportNamespace"
	matchImportNormalAndNamespace

	// The import could not be evaluated due to a cycle
	matchImportCycle

	// The import is missing but came from a TypeScript file
	matchImportProbablyTypeScriptType

	// The import resolved to multiple symbols via "export * from"
	matchImportAmbiguous
)

type matchImportResult struct {
	alias            string
	kind             matchImportKind
	namespaceRef     ast.Ref
	sourceIndex      uint32
	nameLoc          logger.Loc // Optional, goes with sourceIndex, ignore if zero
	otherSourceIndex uint32
	otherNameLoc     logger.Loc // Optional, goes with otherSourceIndex, ignore if zero
	ref              ast.Ref
}

func (c *linkerContext) matchImportWithExport(
	tracker importTracker, reExportsIn []js_ast.Dependency,
) (result matchImportResult, reExports []js_ast.Dependency) {
	var ambiguousResults []matchImportResult
	reExports = reExportsIn

loop:
	for {
		// Make sure we avoid infinite loops trying to resolve cycles:
		//
		//   // foo.js
		//   export {a as b} from './foo.js'
		//   export {b as c} from './foo.js'
		//   export {c as a} from './foo.js'
		//
		// This uses a O(n^2) array scan instead of a O(n) map because the vast
		// majority of cases have one or two elements and Go arrays are cheap to
		// reuse without allocating.
		for _, previousTracker := range c.cycleDetector {
			if tracker == previousTracker {
				result = matchImportResult{kind: matchImportCycle}
				break loop
			}
		}
		c.cycleDetector = append(c.cycleDetector, tracker)

		// Resolve the import by one step
		nextTracker, status, potentiallyAmbiguousExportStarRefs := c.advanceImportTracker(tracker)
		switch status {
		case importCommonJS, importCommonJSWithoutExports, importExternal, importDisabled:
			if status == importExternal && c.options.OutputFormat.KeepESMImportExportSyntax() {
				// Imports from external modules should not be converted to CommonJS
				// if the output format preserves the original ES6 import statements
				break
			}

			// If it's a CommonJS or external file, rewrite the import to a
			// property access. Don't do this if the namespace reference is invalid
			// though. This is the case for star imports, where the import is the
			// namespace.
			trackerFile := &c.graph.Files[tracker.sourceIndex]
			namedImport := trackerFile.InputFile.Repr.(*graph.JSRepr).AST.NamedImports[tracker.importRef]
			if namedImport.NamespaceRef != ast.InvalidRef {
				if result.kind == matchImportNormal {
					result.kind = matchImportNormalAndNamespace
					result.namespaceRef = namedImport.NamespaceRef
					result.alias = namedImport.Alias
				} else {
					result = matchImportResult{
						kind:         matchImportNamespace,
						namespaceRef: namedImport.NamespaceRef,
						alias:        namedImport.Alias,
					}
				}
			}

			// Warn about importing from a file that is known to not have any exports
			if status == importCommonJSWithoutExports {
				symbol := c.graph.Symbols.Get(tracker.importRef)
				symbol.ImportItemStatus = ast.ImportItemMissing
				kind := logger.Warning
				if helpers.IsInsideNodeModules(trackerFile.InputFile.Source.KeyPath.Text) {
					kind = logger.Debug
				}
				c.log.AddID(logger.MsgID_Bundler_ImportIsUndefined, kind,
					trackerFile.LineColumnTracker(),
					js_lexer.RangeOfIdentifier(trackerFile.InputFile.Source, namedImport.AliasLoc),
					fmt.Sprintf("Import %q will always be undefined because the file %q has no exports",
						namedImport.Alias, c.graph.Files[nextTracker.sourceIndex].InputFile.Source.PrettyPath))
			}

		case importDynamicFallback:
			// If it's a file with dynamic export fallback, rewrite the import to a property access
			trackerFile := &c.graph.Files[tracker.sourceIndex]
			namedImport := trackerFile.InputFile.Repr.(*graph.JSRepr).AST.NamedImports[tracker.importRef]
			if result.kind == matchImportNormal {
				result.kind = matchImportNormalAndNamespace
				result.namespaceRef = nextTracker.importRef
				result.alias = namedImport.Alias
			} else {
				result = matchImportResult{
					kind:         matchImportNamespace,
					namespaceRef: nextTracker.importRef,
					alias:        namedImport.Alias,
				}
			}

		case importNoMatch:
			symbol := c.graph.Symbols.Get(tracker.importRef)
			trackerFile := &c.graph.Files[tracker.sourceIndex]
			namedImport := trackerFile.InputFile.Repr.(*graph.JSRepr).AST.NamedImports[tracker.importRef]
			r := js_lexer.RangeOfIdentifier(trackerFile.InputFile.Source, namedImport.AliasLoc)

			// Report mismatched imports and exports
			if symbol.ImportItemStatus == ast.ImportItemGenerated {
				// This is not an error because although it appears to be a named
				// import, it's actually an automatically-generated named import
				// that was originally a property access on an import star
				// namespace object:
				//
				//   import * as ns from 'foo'
				//   const undefinedValue = ns.notAnExport
				//
				// If this code wasn't bundled, this property access would just resolve
				// to undefined at run-time instead of failing at binding-time, so we
				// emit rewrite the value to the literal "undefined" instead of
				// emitting an error.
				symbol.ImportItemStatus = ast.ImportItemMissing

				// Don't emit a log message if this symbol isn't used, since then the
				// log message isn't helpful. This can happen with "import" assignment
				// statements in TypeScript code since they are ambiguously either a
				// type or a value. We consider them to be a type if they aren't used.
				//
				//   import * as ns from 'foo'
				//
				//   // There's no warning here because this is dead code
				//   if (false) ns.notAnExport
				//
				//   // There's no warning here because this is never used
				//   import unused = ns.notAnExport
				//
				if symbol.UseCountEstimate > 0 {
					nextFile := &c.graph.Files[nextTracker.sourceIndex].InputFile
					msg := logger.Msg{
						Kind: logger.Warning,
						Data: trackerFile.LineColumnTracker().MsgData(r, fmt.Sprintf(
							"Import %q will always be undefined because there is no matching export in %q",
							namedImport.Alias, nextFile.Source.PrettyPath)),
					}
					if helpers.IsInsideNodeModules(trackerFile.InputFile.Source.KeyPath.Text) {
						msg.Kind = logger.Debug
					}
					c.maybeCorrectObviousTypo(nextFile.Repr.(*graph.JSRepr), namedImport.Alias, &msg)
					c.log.AddMsgID(logger.MsgID_Bundler_ImportIsUndefined, msg)
				}
			} else {
				nextFile := &c.graph.Files[nextTracker.sourceIndex].InputFile
				msg := logger.Msg{
					Kind: logger.Error,
					Data: trackerFile.LineColumnTracker().MsgData(r, fmt.Sprintf(
						"No matching export in %q for import %q",
						nextFile.Source.PrettyPath, namedImport.Alias)),
				}
				c.maybeCorrectObviousTypo(nextFile.Repr.(*graph.JSRepr), namedImport.Alias, &msg)
				c.log.AddMsg(msg)
			}

		case importProbablyTypeScriptType:
			// Omit this import from any namespace export code we generate for
			// import star statements (i.e. "import * as ns from 'path'")
			result = matchImportResult{kind: matchImportProbablyTypeScriptType}

		case importFound:
			// If there are multiple ambiguous results due to use of "export * from"
			// statements, trace them all to see if they point to different things.
			for _, ambiguousTracker := range potentiallyAmbiguousExportStarRefs {
				// If this is a re-export of another import, follow the import
				if _, ok := c.graph.Files[ambiguousTracker.SourceIndex].InputFile.Repr.(*graph.JSRepr).AST.NamedImports[ambiguousTracker.Ref]; ok {
					// Save and restore the cycle detector to avoid mixing information
					oldCycleDetector := c.cycleDetector
					ambiguousResult, newReExportFiles := c.matchImportWithExport(importTracker{
						sourceIndex: ambiguousTracker.SourceIndex,
						importRef:   ambiguousTracker.Ref,
					}, reExports)
					c.cycleDetector = oldCycleDetector
					ambiguousResults = append(ambiguousResults, ambiguousResult)
					reExports = newReExportFiles
				} else {
					ambiguousResults = append(ambiguousResults, matchImportResult{
						kind:        matchImportNormal,
						sourceIndex: ambiguousTracker.SourceIndex,
						ref:         ambiguousTracker.Ref,
						nameLoc:     ambiguousTracker.NameLoc,
					})
				}
			}

			// Defer the actual binding of this import until after we generate
			// namespace export code for all files. This has to be done for all
			// import-to-export matches, not just the initial import to the final
			// export, since all imports and re-exports must be merged together
			// for correctness.
			result = matchImportResult{
				kind:        matchImportNormal,
				sourceIndex: nextTracker.sourceIndex,
				ref:         nextTracker.importRef,
				nameLoc:     nextTracker.nameLoc,
			}

			// Depend on the statement(s) that declared this import symbol in the
			// original file
			for _, resolvedPartIndex := range c.graph.Files[tracker.sourceIndex].InputFile.Repr.(*graph.JSRepr).TopLevelSymbolToParts(tracker.importRef) {
				reExports = append(reExports, js_ast.Dependency{
					SourceIndex: tracker.sourceIndex,
					PartIndex:   resolvedPartIndex,
				})
			}

			// If this is a re-export of another import, continue for another
			// iteration of the loop to resolve that import as well
			if _, ok := c.graph.Files[nextTracker.sourceIndex].InputFile.Repr.(*graph.JSRepr).AST.NamedImports[nextTracker.importRef]; ok {
				tracker = nextTracker
				continue
			}

		default:
			panic("Internal error")
		}

		// Stop now if we didn't explicitly "continue" above
		break
	}

	// If there is a potential ambiguity, all results must be the same
	for _, ambiguousResult := range ambiguousResults {
		if ambiguousResult != result {
			if result.kind == matchImportNormal && ambiguousResult.kind == matchImportNormal &&
				result.nameLoc.Start != 0 && ambiguousResult.nameLoc.Start != 0 {
				return matchImportResult{
					kind:             matchImportAmbiguous,
					sourceIndex:      result.sourceIndex,
					nameLoc:          result.nameLoc,
					otherSourceIndex: ambiguousResult.sourceIndex,
					otherNameLoc:     ambiguousResult.nameLoc,
				}, nil
			}
			return matchImportResult{kind: matchImportAmbiguous}, nil
		}
	}

	return
}

func (c *linkerContext) maybeForbidArbitraryModuleNamespaceIdentifier(kind string, sourceIndex uint32, loc logger.Loc, alias string) {
	if !js_ast.IsIdentifier(alias) {
		file := &c.graph.Files[sourceIndex]
		where := config.PrettyPrintTargetEnvironment(c.options.OriginalTargetEnv, c.options.UnsupportedJSFeatureOverridesMask)
		c.log.AddError(file.LineColumnTracker(), file.InputFile.Source.RangeOfString(loc), fmt.Sprintf(
			"Using the string %q as an %s name is not supported in %s", alias, kind, where))
	}
}

// Attempt to correct an import name with a typo
func (c *linkerContext) maybeCorrectObviousTypo(repr *graph.JSRepr, name string, msg *logger.Msg) {
	if repr.Meta.ResolvedExportTypos == nil {
		valid := make([]string, 0, len(repr.Meta.ResolvedExports))
		for alias := range repr.Meta.ResolvedExports {
			valid = append(valid, alias)
		}
		sort.Strings(valid)
		typos := helpers.MakeTypoDetector(valid)
		repr.Meta.ResolvedExportTypos = &typos
	}

	if corrected, ok := repr.Meta.ResolvedExportTypos.MaybeCorrectTypo(name); ok {
		msg.Data.Location.Suggestion = corrected
		export := repr.Meta.ResolvedExports[corrected]
		importedFile := &c.graph.Files[export.SourceIndex]
		text := fmt.Sprintf("Did you mean to import %q instead?", corrected)
		var note logger.MsgData
		if export.NameLoc.Start == 0 {
			// Don't report a source location for definitions without one. This can
			// happen with automatically-generated exports from non-JavaScript files.
			note.Text = text
		} else {
			var r logger.Range
			if importedFile.InputFile.Loader.IsCSS() {
				r = css_lexer.RangeOfIdentifier(importedFile.InputFile.Source, export.NameLoc)
			} else {
				r = js_lexer.RangeOfIdentifier(importedFile.InputFile.Source, export.NameLoc)
			}
			note = importedFile.LineColumnTracker().MsgData(r, text)
		}
		msg.Notes = append(msg.Notes, note)
	}
}

func (c *linkerContext) recursivelyWrapDependencies(sourceIndex uint32) {
	repr := c.graph.Files[sourceIndex].InputFile.Repr.(*graph.JSRepr)
	if repr.Meta.DidWrapDependencies {
		return
	}
	repr.Meta.DidWrapDependencies = true

	// Never wrap the runtime file since it always comes first
	if sourceIndex == runtime.SourceIndex {
		return
	}

	// This module must be wrapped
	if repr.Meta.Wrap == graph.WrapNone {
		if repr.AST.ExportsKind == js_ast.ExportsCommonJS {
			repr.Meta.Wrap = graph.WrapCJS
		} else {
			repr.Meta.Wrap = graph.WrapESM
		}
	}

	// All dependencies must also be wrapped
	for _, record := range repr.AST.ImportRecords {
		if record.SourceIndex.IsValid() {
			c.recursivelyWrapDependencies(record.SourceIndex.GetIndex())
		}
	}
}

func (c *linkerContext) hasDynamicExportsDueToExportStar(sourceIndex uint32, visited map[uint32]bool) bool {
	// Terminate the traversal now if this file already has dynamic exports
	repr := c.graph.Files[sourceIndex].InputFile.Repr.(*graph.JSRepr)
	if repr.AST.ExportsKind == js_ast.ExportsCommonJS || repr.AST.ExportsKind == js_ast.ExportsESMWithDynamicFallback {
		return true
	}

	// Avoid infinite loops due to cycles in the export star graph
	if visited[sourceIndex] {
		return false
	}
	visited[sourceIndex] = true

	// Scan over the export star graph
	for _, importRecordIndex := range repr.AST.ExportStarImportRecords {
		record := &repr.AST.ImportRecords[importRecordIndex]

		// This file has dynamic exports if the exported imports are from a file
		// that either has dynamic exports directly or transitively by itself
		// having an export star from a file with dynamic exports.
		if (!record.SourceIndex.IsValid() && (!c.graph.Files[sourceIndex].IsEntryPoint() || !c.options.OutputFormat.KeepESMImportExportSyntax())) ||
			(record.SourceIndex.IsValid() && record.SourceIndex.GetIndex() != sourceIndex && c.hasDynamicExportsDueToExportStar(record.SourceIndex.GetIndex(), visited)) {
			repr.AST.ExportsKind = js_ast.ExportsESMWithDynamicFallback
			return true
		}
	}

	return false
}

func (c *linkerContext) addExportsForExportStar(
	resolvedExports map[string]graph.ExportData,
	sourceIndex uint32,
	sourceIndexStack []uint32,
) {
	// Avoid infinite loops due to cycles in the export star graph
	for _, prevSourceIndex := range sourceIndexStack {
		if prevSourceIndex == sourceIndex {
			return
		}
	}
	sourceIndexStack = append(sourceIndexStack, sourceIndex)
	repr := c.graph.Files[sourceIndex].InputFile.Repr.(*graph.JSRepr)

	for _, importRecordIndex := range repr.AST.ExportStarImportRecords {
		record := &repr.AST.ImportRecords[importRecordIndex]
		if !record.SourceIndex.IsValid() {
			// This will be resolved at run time instead
			continue
		}
		otherSourceIndex := record.SourceIndex.GetIndex()

		// Export stars from a CommonJS module don't work because they can't be
		// statically discovered. Just silently ignore them in this case.
		//
		// We could attempt to check whether the imported file still has ES6
		// exports even though it still uses CommonJS features. However, when
		// doing this we'd also have to rewrite any imports of these export star
		// re-exports as property accesses off of a generated require() call.
		otherRepr := c.graph.Files[otherSourceIndex].InputFile.Repr.(*graph.JSRepr)
		if otherRepr.AST.ExportsKind == js_ast.ExportsCommonJS {
			// All exports will be resolved at run time instead
			continue
		}

		// Accumulate this file's exports
	nextExport:
		for alias, name := range otherRepr.AST.NamedExports {
			// ES6 export star statements ignore exports named "default"
			if alias == "default" {
				continue
			}

			// This export star is shadowed if any file in the stack has a matching real named export
			for _, prevSourceIndex := range sourceIndexStack {
				prevRepr := c.graph.Files[prevSourceIndex].InputFile.Repr.(*graph.JSRepr)
				if _, ok := prevRepr.AST.NamedExports[alias]; ok {
					continue nextExport
				}
			}

			if existing, ok := resolvedExports[alias]; !ok {
				// Initialize the re-export
				resolvedExports[alias] = graph.ExportData{
					Ref:         name.Ref,
					SourceIndex: otherSourceIndex,
					NameLoc:     name.AliasLoc,
				}

				// Make sure the symbol is marked as imported so that code splitting
				// imports it correctly if it ends up being shared with another chunk
				repr.Meta.ImportsToBind[name.Ref] = graph.ImportData{
					Ref:         name.Ref,
					SourceIndex: otherSourceIndex,
				}
			} else if existing.SourceIndex != otherSourceIndex {
				// Two different re-exports colliding makes it potentially ambiguous
				existing.PotentiallyAmbiguousExportStarRefs =
					append(existing.PotentiallyAmbiguousExportStarRefs, graph.ImportData{
						SourceIndex: otherSourceIndex,
						Ref:         name.Ref,
						NameLoc:     name.AliasLoc,
					})
				resolvedExports[alias] = existing
			}
		}

		// Search further through this file's export stars
		c.addExportsForExportStar(resolvedExports, otherSourceIndex, sourceIndexStack)
	}
}

type importTracker struct {
	sourceIndex uint32
	nameLoc     logger.Loc // Optional, goes with sourceIndex, ignore if zero
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

	// The import is missing but there is a dynamic fallback object
	importDynamicFallback

	// The import was treated as a CommonJS import but the file is known to have no exports
	importCommonJSWithoutExports

	// The imported file was disabled by mapping it to false in the "browser"
	// field of package.json
	importDisabled

	// The imported file is external and has unknown exports
	importExternal

	// This is a missing re-export in a TypeScript file, so it's probably a type
	importProbablyTypeScriptType
)

func (c *linkerContext) advanceImportTracker(tracker importTracker) (importTracker, importStatus, []graph.ImportData) {
	file := &c.graph.Files[tracker.sourceIndex]
	repr := file.InputFile.Repr.(*graph.JSRepr)
	namedImport := repr.AST.NamedImports[tracker.importRef]

	// Is this an external file?
	record := &repr.AST.ImportRecords[namedImport.ImportRecordIndex]
	if !record.SourceIndex.IsValid() {
		return importTracker{}, importExternal, nil
	}

	// Is this a named import of a file without any exports?
	otherSourceIndex := record.SourceIndex.GetIndex()
	otherRepr := c.graph.Files[otherSourceIndex].InputFile.Repr.(*graph.JSRepr)
	if !namedImport.AliasIsStar && !otherRepr.AST.HasLazyExport &&
		// CommonJS exports
		otherRepr.AST.ExportKeyword.Len == 0 && namedImport.Alias != "default" &&
		// ESM exports
		!otherRepr.AST.UsesExportsRef && !otherRepr.AST.UsesModuleRef {
		// Just warn about it and replace the import with "undefined"
		return importTracker{sourceIndex: otherSourceIndex, importRef: ast.InvalidRef}, importCommonJSWithoutExports, nil
	}

	// Is this a CommonJS file?
	if otherRepr.AST.ExportsKind == js_ast.ExportsCommonJS {
		return importTracker{sourceIndex: otherSourceIndex, importRef: ast.InvalidRef}, importCommonJS, nil
	}

	// Match this import star with an export star from the imported file
	if matchingExport := otherRepr.Meta.ResolvedExportStar; namedImport.AliasIsStar && matchingExport != nil {
		// Check to see if this is a re-export of another import
		return importTracker{
			sourceIndex: matchingExport.SourceIndex,
			importRef:   matchingExport.Ref,
			nameLoc:     matchingExport.NameLoc,
		}, importFound, matchingExport.PotentiallyAmbiguousExportStarRefs
	}

	// Match this import up with an export from the imported file
	if matchingExport, ok := otherRepr.Meta.ResolvedExports[namedImport.Alias]; ok {
		// Check to see if this is a re-export of another import
		return importTracker{
			sourceIndex: matchingExport.SourceIndex,
			importRef:   matchingExport.Ref,
			nameLoc:     matchingExport.NameLoc,
		}, importFound, matchingExport.PotentiallyAmbiguousExportStarRefs
	}

	// Is this a file with dynamic exports?
	if otherRepr.AST.ExportsKind == js_ast.ExportsESMWithDynamicFallback {
		return importTracker{sourceIndex: otherSourceIndex, importRef: otherRepr.AST.ExportsRef}, importDynamicFallback, nil
	}

	// Missing re-exports in TypeScript files are indistinguishable from types
	if file.InputFile.Loader.IsTypeScript() && namedImport.IsExported {
		return importTracker{}, importProbablyTypeScriptType, nil
	}

	return importTracker{sourceIndex: otherSourceIndex}, importNoMatch, nil
}

func (c *linkerContext) treeShakingAndCodeSplitting() {
	// Tree shaking: Each entry point marks all files reachable from itself
	c.timer.Begin("Tree shaking")
	for _, entryPoint := range c.graph.EntryPoints() {
		c.markFileLiveForTreeShaking(entryPoint.SourceIndex)
	}
	c.timer.End("Tree shaking")

	// Code splitting: Determine which entry points can reach which files. This
	// has to happen after tree shaking because there is an implicit dependency
	// between live parts within the same file. All liveness has to be computed
	// first before determining which entry points can reach which files.
	c.timer.Begin("Code splitting")
	for i, entryPoint := range c.graph.EntryPoints() {
		c.markFileReachableForCodeSplitting(entryPoint.SourceIndex, uint(i), 0)
	}
	c.timer.End("Code splitting")
}

func (c *linkerContext) markFileReachableForCodeSplitting(sourceIndex uint32, entryPointBit uint, distanceFromEntryPoint uint32) {
	file := &c.graph.Files[sourceIndex]
	if !file.IsLive {
		return
	}
	traverseAgain := false

	// Track the minimum distance to an entry point
	if distanceFromEntryPoint < file.DistanceFromEntryPoint {
		file.DistanceFromEntryPoint = distanceFromEntryPoint
		traverseAgain = true
	}
	distanceFromEntryPoint++

	// Don't mark this file more than once
	if file.EntryBits.HasBit(entryPointBit) && !traverseAgain {
		return
	}
	file.EntryBits.SetBit(entryPointBit)

	switch repr := file.InputFile.Repr.(type) {
	case *graph.JSRepr:
		// If the JavaScript stub for a CSS file is included, also include the CSS file
		if repr.CSSSourceIndex.IsValid() {
			c.markFileReachableForCodeSplitting(repr.CSSSourceIndex.GetIndex(), entryPointBit, distanceFromEntryPoint)
		}

		// Traverse into all imported files
		for _, record := range repr.AST.ImportRecords {
			if record.SourceIndex.IsValid() && !c.isExternalDynamicImport(&record, sourceIndex) {
				c.markFileReachableForCodeSplitting(record.SourceIndex.GetIndex(), entryPointBit, distanceFromEntryPoint)
			}
		}

		// Traverse into all dependencies of all parts in this file
		for _, part := range repr.AST.Parts {
			for _, dependency := range part.Dependencies {
				if dependency.SourceIndex != sourceIndex {
					c.markFileReachableForCodeSplitting(dependency.SourceIndex, entryPointBit, distanceFromEntryPoint)
				}
			}
		}

	case *graph.CSSRepr:
		// Traverse into all dependencies
		for _, record := range repr.AST.ImportRecords {
			if record.SourceIndex.IsValid() {
				c.markFileReachableForCodeSplitting(record.SourceIndex.GetIndex(), entryPointBit, distanceFromEntryPoint)
			}
		}
	}
}

func (c *linkerContext) markFileLiveForTreeShaking(sourceIndex uint32) {
	file := &c.graph.Files[sourceIndex]

	// Don't mark this file more than once
	if file.IsLive {
		return
	}
	file.IsLive = true

	switch repr := file.InputFile.Repr.(type) {
	case *graph.JSRepr:
		// If the JavaScript stub for a CSS file is included, also include the CSS file
		if repr.CSSSourceIndex.IsValid() {
			c.markFileLiveForTreeShaking(repr.CSSSourceIndex.GetIndex())
		}

		for partIndex, part := range repr.AST.Parts {
			canBeRemovedIfUnused := part.CanBeRemovedIfUnused

			// Also include any statement-level imports
			for _, importRecordIndex := range part.ImportRecordIndices {
				record := &repr.AST.ImportRecords[importRecordIndex]
				if record.Kind != ast.ImportStmt {
					continue
				}

				if record.SourceIndex.IsValid() {
					otherSourceIndex := record.SourceIndex.GetIndex()

					// Don't include this module for its side effects if it can be
					// considered to have no side effects
					if otherFile := &c.graph.Files[otherSourceIndex]; otherFile.InputFile.SideEffects.Kind != graph.HasSideEffects && !c.options.IgnoreDCEAnnotations {
						continue
					}

					// Otherwise, include this module for its side effects
					c.markFileLiveForTreeShaking(otherSourceIndex)
				} else if record.Flags.Has(ast.IsExternalWithoutSideEffects) {
					// This can be removed if it's unused
					continue
				}

				// If we get here then the import was included for its side effects, so
				// we must also keep this part
				canBeRemovedIfUnused = false
			}

			// Include all parts in this file with side effects, or just include
			// everything if tree-shaking is disabled. Note that we still want to
			// perform tree-shaking on the runtime even if tree-shaking is disabled.
			if !canBeRemovedIfUnused || (!part.ForceTreeShaking && !c.options.TreeShaking && file.IsEntryPoint()) {
				c.markPartLiveForTreeShaking(sourceIndex, uint32(partIndex))
			}
		}

	case *graph.CSSRepr:
		// Include all "@import" rules
		for _, record := range repr.AST.ImportRecords {
			if record.SourceIndex.IsValid() {
				c.markFileLiveForTreeShaking(record.SourceIndex.GetIndex())
			}
		}
	}
}

func (c *linkerContext) isExternalDynamicImport(record *ast.ImportRecord, sourceIndex uint32) bool {
	return c.options.CodeSplitting &&
		record.Kind == ast.ImportDynamic &&
		c.graph.Files[record.SourceIndex.GetIndex()].IsEntryPoint() &&
		record.SourceIndex.GetIndex() != sourceIndex
}

func (c *linkerContext) markPartLiveForTreeShaking(sourceIndex uint32, partIndex uint32) {
	file := &c.graph.Files[sourceIndex]
	repr := file.InputFile.Repr.(*graph.JSRepr)
	part := &repr.AST.Parts[partIndex]

	// Don't mark this part more than once
	if part.IsLive {
		return
	}
	part.IsLive = true

	// Include the file containing this part
	c.markFileLiveForTreeShaking(sourceIndex)

	// Also include any dependencies
	for _, dep := range part.Dependencies {
		c.markPartLiveForTreeShaking(dep.SourceIndex, dep.PartIndex)
	}
}

// JavaScript modules are traversed in depth-first postorder. This is the
// order that JavaScript modules were evaluated in before the top-level await
// feature was introduced.
//
//	  A
//	 / \
//	B   C
//	 \ /
//	  D
//
// If A imports B and then C, B imports D, and C imports D, then the JavaScript
// traversal order is D B C A.
//
// This function may deviate from ESM import order for dynamic imports (both
// "require()" and "import()"). This is because the import order is impossible
// to determine since the imports happen at run-time instead of compile-time.
// In this case we just pick an arbitrary but consistent order.
func (c *linkerContext) findImportedCSSFilesInJSOrder(entryPoint uint32) (order []uint32) {
	visited := make(map[uint32]bool)
	var visit func(uint32)

	// Include this file and all files it imports
	visit = func(sourceIndex uint32) {
		if visited[sourceIndex] {
			return
		}
		visited[sourceIndex] = true
		file := &c.graph.Files[sourceIndex]
		repr := file.InputFile.Repr.(*graph.JSRepr)

		// Iterate over each part in the file in order
		for _, part := range repr.AST.Parts {
			// Traverse any files imported by this part. Note that CommonJS calls
			// to "require()" count as imports too, sort of as if the part has an
			// ESM "import" statement in it. This may seem weird because ESM imports
			// are a compile-time concept while CommonJS imports are a run-time
			// concept. But we don't want to manipulate <style> tags at run-time so
			// this is the only way to do it.
			for _, importRecordIndex := range part.ImportRecordIndices {
				if record := &repr.AST.ImportRecords[importRecordIndex]; record.SourceIndex.IsValid() {
					visit(record.SourceIndex.GetIndex())
				}
			}
		}

		// Iterate over the associated CSS imports in postorder
		if repr.CSSSourceIndex.IsValid() {
			order = append(order, repr.CSSSourceIndex.GetIndex())
		}
	}

	// Include all files reachable from the entry point
	visit(entryPoint)

	return
}

type cssImportKind uint8

const (
	cssImportNone cssImportKind = iota
	cssImportSourceIndex
	cssImportExternalPath
	cssImportLayers
)

type cssImportOrder struct {
	conditions             []css_ast.ImportConditions
	conditionImportRecords []ast.ImportRecord

	layers       [][]string  // kind == cssImportAtLayer
	externalPath logger.Path // kind == cssImportExternal
	sourceIndex  uint32      // kind == cssImportSourceIndex

	kind cssImportKind
}

// CSS files are traversed in depth-first postorder just like JavaScript. But
// unlike JavaScript import statements, CSS "@import" rules are evaluated every
// time instead of just the first time.
//
//	  A
//	 / \
//	B   C
//	 \ /
//	  D
//
// If A imports B and then C, B imports D, and C imports D, then the CSS
// traversal order is D B D C A.
//
// However, evaluating a CSS file multiple times is sort of equivalent to
// evaluating it once at the last location. So we basically drop all but the
// last evaluation in the order.
//
// The only exception to this is "@layer". Evaluating a CSS file multiple
// times is sort of equivalent to evaluating it once at the first location
// as far as "@layer" is concerned. So we may in some cases keep both the
// first and last locations and only write out the "@layer" information
// for the first location.
func (c *linkerContext) findImportedFilesInCSSOrder(entryPoints []uint32) (order []cssImportOrder) {
	var visit func(uint32, []uint32, []css_ast.ImportConditions, []ast.ImportRecord)
	hasExternalImport := false

	// Include this file and all files it imports
	visit = func(
		sourceIndex uint32,
		visited []uint32,
		wrappingConditions []css_ast.ImportConditions,
		wrappingImportRecords []ast.ImportRecord,
	) {
		// The CSS specification strangely does not describe what to do when there
		// is a cycle. So we are left with reverse-engineering the behavior from a
		// real browser. Here's what the WebKit code base has to say about this:
		//
		//   "Check for a cycle in our import chain. If we encounter a stylesheet
		//   in our parent chain with the same URL, then just bail."
		//
		// So that's what we do here. See "StyleRuleImport::requestStyleSheet()" in
		// WebKit for more information.
		for _, visitedSourceIndex := range visited {
			if visitedSourceIndex == sourceIndex {
				return
			}
		}
		visited = append(visited, sourceIndex)

		repr := c.graph.Files[sourceIndex].InputFile.Repr.(*graph.CSSRepr)
		topLevelRules := repr.AST.Rules

		// Any pre-import layers come first
		if len(repr.AST.LayersPreImport) > 0 {
			order = append(order, cssImportOrder{
				kind:                   cssImportLayers,
				layers:                 repr.AST.LayersPreImport,
				conditions:             wrappingConditions,
				conditionImportRecords: wrappingImportRecords,
			})
		}

		// Iterate over the top-level "@import" rules
		for _, rule := range topLevelRules {
			if atImport, ok := rule.Data.(*css_ast.RAtImport); ok {
				record := &repr.AST.ImportRecords[atImport.ImportRecordIndex]

				// Follow internal dependencies
				if record.SourceIndex.IsValid() {
					nestedConditions := wrappingConditions
					nestedImportRecords := wrappingImportRecords

					// If this import has conditions, fork our state so that the entire
					// imported stylesheet subtree is wrapped in all of the conditions
					if atImport.ImportConditions != nil {
						// Fork our state
						nestedConditions = append([]css_ast.ImportConditions{}, nestedConditions...)
						nestedImportRecords = append([]ast.ImportRecord{}, nestedImportRecords...)

						// Clone these import conditions and append them to the state
						var conditions css_ast.ImportConditions
						conditions, nestedImportRecords = atImport.ImportConditions.CloneWithImportRecords(repr.AST.ImportRecords, nestedImportRecords)
						nestedConditions = append(nestedConditions, conditions)
					}

					visit(record.SourceIndex.GetIndex(), visited, nestedConditions, nestedImportRecords)
					continue
				}

				// Record external dependencies
				if (record.Flags & ast.WasLoadedWithEmptyLoader) == 0 {
					allConditions := wrappingConditions
					allImportRecords := wrappingImportRecords

					// If this import has conditions, append it to the list of overall
					// conditions for this external import. Note that an external import
					// may actually have multiple sets of conditions that can't be
					// merged. When this happens we need to generate a nested imported
					// CSS file using a data URL.
					if atImport.ImportConditions != nil {
						var conditions css_ast.ImportConditions
						allConditions = append([]css_ast.ImportConditions{}, allConditions...)
						allImportRecords = append([]ast.ImportRecord{}, allImportRecords...)
						conditions, allImportRecords = atImport.ImportConditions.CloneWithImportRecords(repr.AST.ImportRecords, allImportRecords)
						allConditions = append(allConditions, conditions)
					}

					order = append(order, cssImportOrder{
						kind:                   cssImportExternalPath,
						externalPath:           record.Path,
						conditions:             allConditions,
						conditionImportRecords: allImportRecords,
					})
					hasExternalImport = true
				}
			}
		}

		// Iterate over the "composes" directives. Note that the order doesn't
		// matter for these because the output order is explicitly undefined
		// in the specification.
		for _, record := range repr.AST.ImportRecords {
			if record.Kind == ast.ImportComposesFrom && record.SourceIndex.IsValid() {
				visit(record.SourceIndex.GetIndex(), visited, wrappingConditions, wrappingImportRecords)
			}
		}

		// Accumulate imports in depth-first postorder
		order = append(order, cssImportOrder{
			kind:                   cssImportSourceIndex,
			sourceIndex:            sourceIndex,
			conditions:             wrappingConditions,
			conditionImportRecords: wrappingImportRecords,
		})
	}

	// Include all files reachable from any entry point
	var visited [16]uint32 // Preallocate some space for the visited set
	for _, sourceIndex := range entryPoints {
		visit(sourceIndex, visited[:], nil, nil)
	}

	// Create a temporary array that we can use for filtering
	wipOrder := make([]cssImportOrder, 0, len(order))

	// CSS syntax unfortunately only allows "@import" rules at the top of the
	// file. This means we must hoist all external "@import" rules to the top of
	// the file when bundling, even though doing so will change the order of CSS
	// evaluation.
	if hasExternalImport {
		// Pass 1: Pull out leading "@layer" and external "@import" rules
		isAtLayerPrefix := true
		for _, entry := range order {
			if (entry.kind == cssImportLayers && isAtLayerPrefix) || entry.kind == cssImportExternalPath {
				wipOrder = append(wipOrder, entry)
			}
			if entry.kind != cssImportLayers {
				isAtLayerPrefix = false
			}
		}

		// Pass 2: Append everything that we didn't pull out in pass 1
		isAtLayerPrefix = true
		for _, entry := range order {
			if (entry.kind != cssImportLayers || !isAtLayerPrefix) && entry.kind != cssImportExternalPath {
				wipOrder = append(wipOrder, entry)
			}
			if entry.kind != cssImportLayers {
				isAtLayerPrefix = false
			}
		}

		order, wipOrder = wipOrder, order[:0]
	}

	// Next, optimize import order. If there are duplicate copies of an imported
	// file, replace all but the last copy with just the layers that are in that
	// file. This works because in CSS, the last instance of a declaration
	// overrides all previous instances of that declaration.
	{
		sourceIndexDuplicates := make(map[uint32][]int)
		externalPathDuplicates := make(map[logger.Path][]int)

	nextBackward:
		for i := len(order) - 1; i >= 0; i-- {
			entry := order[i]
			switch entry.kind {
			case cssImportSourceIndex:
				duplicates := sourceIndexDuplicates[entry.sourceIndex]
				for _, j := range duplicates {
					if isConditionalImportRedundant(entry.conditions, order[j].conditions) {
						order[i].kind = cssImportLayers
						order[i].layers = c.graph.Files[entry.sourceIndex].InputFile.Repr.(*graph.CSSRepr).AST.LayersPostImport
						continue nextBackward
					}
				}
				sourceIndexDuplicates[entry.sourceIndex] = append(duplicates, i)

			case cssImportExternalPath:
				duplicates := externalPathDuplicates[entry.externalPath]
				for _, j := range duplicates {
					if isConditionalImportRedundant(entry.conditions, order[j].conditions) {
						// Don't remove duplicates entirely. The import conditions may
						// still introduce layers to the layer order. Represent this as a
						// file with an empty layer list.
						order[i].kind = cssImportLayers
						continue nextBackward
					}
				}
				externalPathDuplicates[entry.externalPath] = append(duplicates, i)
			}
		}
	}

	// Then optimize "@layer" rules by removing redundant ones. This loop goes
	// forward instead of backward because "@layer" takes effect at the first
	// copy instead of the last copy like other things in CSS.
	{
		type duplicateEntry struct {
			layers  [][]string
			indices []int
		}
		var layerDuplicates []duplicateEntry

	nextForward:
		for i := range order {
			entry := order[i]

			// Simplify the conditions since we know they only wrap "@layer"
			if entry.kind == cssImportLayers {
				// Truncate the conditions at the first anonymous layer
				for i, conditions := range entry.conditions {
					// The layer is anonymous if it's a "layer" token without any
					// children instead of a "layer(...)" token with children:
					//
					//   /* entry.css */
					//   @import "foo.css" layer;
					//
					//   /* foo.css */
					//   @layer foo;
					//
					// We don't need to generate this (as far as I can tell):
					//
					//   @layer {
					//     @layer foo;
					//   }
					//
					if conditions.Layers != nil && len(conditions.Layers) == 1 && conditions.Layers[0].Children == nil {
						entry.conditions = entry.conditions[:i]
						entry.layers = nil
						break
					}
				}

				// If there are no layer names for this file, trim all conditions
				// without layers because we know they have no effect.
				//
				//   /* entry.css */
				//   @import "foo.css" layer(foo) supports(display: flex);
				//
				//   /* foo.css */
				//   @import "empty.css" supports(display: grid);
				//
				// That would result in this:
				//
				//   @supports (display: flex) {
				//     @layer foo {
				//       @supports (display: grid) {}
				//     }
				//   }
				//
				// Here we can trim "supports(display: grid)" to generate this:
				//
				//   @supports (display: flex) {
				//     @layer foo;
				//   }
				//
				if len(entry.layers) == 0 {
					for i := len(entry.conditions) - 1; i >= 0; i-- {
						if len(entry.conditions[i].Layers) > 0 {
							break
						}
						entry.conditions = entry.conditions[:i]
					}
				}

				// Remove unnecessary entries entirely
				if len(entry.conditions) == 0 && len(entry.layers) == 0 {
					continue
				}
			}

			// Omit redundant "@layer" rules with the same set of layer names. Note
			// that this tests all import order entries (not just layer ones) because
			// sometimes non-layer ones can make following layer ones redundant.
			layersKey := entry.layers
			if entry.kind == cssImportSourceIndex {
				layersKey = c.graph.Files[entry.sourceIndex].InputFile.Repr.(*graph.CSSRepr).AST.LayersPostImport
			}
			index := 0
			for index < len(layerDuplicates) {
				if helpers.StringArrayArraysEqual(layersKey, layerDuplicates[index].layers) {
					break
				}
				index++
			}
			if index == len(layerDuplicates) {
				// This is the first time we've seen this combination of layer names.
				// Allocate a new set of duplicate indices to track this combination.
				layerDuplicates = append(layerDuplicates, duplicateEntry{layers: layersKey})
			}
			duplicates := layerDuplicates[index].indices
			for j := len(duplicates) - 1; j >= 0; j-- {
				if index := duplicates[j]; isConditionalImportRedundant(entry.conditions, wipOrder[index].conditions) {
					if entry.kind != cssImportLayers {
						// If an empty layer is followed immediately by a full layer and
						// everything else is identical, then we don't need to emit the
						// empty layer. For example:
						//
						//   @media screen {
						//     @supports (display: grid) {
						//       @layer foo;
						//     }
						//   }
						//   @media screen {
						//     @supports (display: grid) {
						//       @layer foo {
						//         div {
						//           color: red;
						//         }
						//       }
						//     }
						//   }
						//
						// This can be improved by dropping the empty layer. But we can
						// only do this if there's nothing in between these two rules.
						if j == len(duplicates)-1 && index == len(wipOrder)-1 {
							if other := wipOrder[index]; other.kind == cssImportLayers && importConditionsAreEqual(entry.conditions, other.conditions) {
								// Remove the previous entry and then overwrite it below
								duplicates = duplicates[:j]
								wipOrder = wipOrder[:index]
								break
							}
						}

						// Non-layer entries still need to be present because they have
						// other side effects beside inserting things in the layer order
						wipOrder = append(wipOrder, entry)
					}

					// Don't add this to the duplicate list below because it's redundant
					continue nextForward
				}
			}
			layerDuplicates[index].indices = append(duplicates, len(wipOrder))
			wipOrder = append(wipOrder, entry)
		}

		order, wipOrder = wipOrder, order[:0]
	}

	// Finally, merge adjacent "@layer" rules with identical conditions together.
	{
		didClone := -1
		for _, entry := range order {
			if entry.kind == cssImportLayers && len(wipOrder) > 0 {
				prevIndex := len(wipOrder) - 1
				prev := wipOrder[prevIndex]
				if prev.kind == cssImportLayers && importConditionsAreEqual(prev.conditions, entry.conditions) {
					if didClone != prevIndex {
						didClone = prevIndex
						prev.layers = append([][]string{}, prev.layers...)
					}
					wipOrder[prevIndex].layers = append(prev.layers, entry.layers...)
					continue
				}
			}
			wipOrder = append(wipOrder, entry)
		}
		order = wipOrder
	}

	return
}

func importConditionsAreEqual(a []css_ast.ImportConditions, b []css_ast.ImportConditions) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ai := a[i]
		bi := b[i]
		if !css_ast.TokensEqualIgnoringWhitespace(ai.Layers, bi.Layers) ||
			!css_ast.TokensEqualIgnoringWhitespace(ai.Supports, bi.Supports) ||
			!css_ast.TokensEqualIgnoringWhitespace(ai.Media, bi.Media) {
			return false
		}
	}
	return true
}

// Given two "@import" rules for the same source index (an earlier one and a
// later one), the earlier one is masked by the later one if the later one's
// condition list is a prefix of the earlier one's condition list.
//
// For example:
//
//	// entry.css
//	@import "foo.css" supports(display: flex);
//	@import "bar.css" supports(display: flex);
//
//	// foo.css
//	@import "lib.css" screen;
//
//	// bar.css
//	@import "lib.css";
//
// When we bundle this code we'll get an import order as follows:
//
//  1. lib.css [supports(display: flex), screen]
//  2. foo.css [supports(display: flex)]
//  3. lib.css [supports(display: flex)]
//  4. bar.css [supports(display: flex)]
//  5. entry.css []
//
// For "lib.css", the entry with the conditions [supports(display: flex)] should
// make the entry with the conditions [supports(display: flex), screen] redundant.
//
// Note that all of this deliberately ignores the existence of "@layer" because
// that is handled separately. All of this is only for handling unlayered styles.
func isConditionalImportRedundant(earlier []css_ast.ImportConditions, later []css_ast.ImportConditions) bool {
	if len(later) > len(earlier) {
		return false
	}

	for i := 0; i < len(later); i++ {
		a := earlier[i]
		b := later[i]

		// Only compare "@supports" and "@media" if "@layers" is equal
		if css_ast.TokensEqualIgnoringWhitespace(a.Layers, b.Layers) {
			sameSupports := css_ast.TokensEqualIgnoringWhitespace(a.Supports, b.Supports)
			sameMedia := css_ast.TokensEqualIgnoringWhitespace(a.Media, b.Media)

			// If the import conditions are exactly equal, then only keep
			// the later one. The earlier one is redundant. Example:
			//
			//   @import "foo.css" layer(abc) supports(display: flex) screen;
			//   @import "foo.css" layer(abc) supports(display: flex) screen;
			//
			// The later one makes the earlier one redundant.
			if sameSupports && sameMedia {
				continue
			}

			// If the media conditions are exactly equal and the later one
			// doesn't have any supports conditions, then the later one will
			// apply in all cases where the earlier one applies. Example:
			//
			//   @import "foo.css" layer(abc) supports(display: flex) screen;
			//   @import "foo.css" layer(abc) screen;
			//
			// The later one makes the earlier one redundant.
			if sameMedia && len(b.Supports) == 0 {
				continue
			}

			// If the supports conditions are exactly equal and the later one
			// doesn't have any media conditions, then the later one will
			// apply in all cases where the earlier one applies. Example:
			//
			//   @import "foo.css" layer(abc) supports(display: flex) screen;
			//   @import "foo.css" layer(abc) supports(display: flex);
			//
			// The later one makes the earlier one redundant.
			if sameSupports && len(b.Media) == 0 {
				continue
			}
		}

		return false
	}

	return true
}

func (c *linkerContext) computeChunks() {
	c.timer.Begin("Compute chunks")
	defer c.timer.End("Compute chunks")

	jsChunks := make(map[string]chunkInfo)
	cssChunks := make(map[string]chunkInfo)

	// Create chunks for entry points
	for i, entryPoint := range c.graph.EntryPoints() {
		file := &c.graph.Files[entryPoint.SourceIndex]

		// Create a chunk for the entry point here to ensure that the chunk is
		// always generated even if the resulting file is empty
		entryBits := helpers.NewBitSet(uint(len(c.graph.EntryPoints())))
		entryBits.SetBit(uint(i))
		key := entryBits.String()
		chunk := chunkInfo{
			entryBits:             entryBits,
			isEntryPoint:          true,
			sourceIndex:           entryPoint.SourceIndex,
			entryPointBit:         uint(i),
			filesWithPartsInChunk: make(map[uint32]bool),
		}

		switch file.InputFile.Repr.(type) {
		case *graph.JSRepr:
			chunkRepr := &chunkReprJS{}
			chunk.chunkRepr = chunkRepr
			jsChunks[key] = chunk

			// If this JS entry point has an associated CSS entry point, generate it
			// now. This is essentially done by generating a virtual CSS file that
			// only contains "@import" statements in the order that the files were
			// discovered in JS source order, where JS source order is arbitrary but
			// consistent for dynamic imports. Then we run the CSS import order
			// algorithm to determine the final CSS file order for the chunk.

			if cssSourceIndices := c.findImportedCSSFilesInJSOrder(entryPoint.SourceIndex); len(cssSourceIndices) > 0 {
				order := c.findImportedFilesInCSSOrder(cssSourceIndices)
				cssFilesWithPartsInChunk := make(map[uint32]bool)
				for _, entry := range order {
					if entry.kind == cssImportSourceIndex {
						cssFilesWithPartsInChunk[uint32(entry.sourceIndex)] = true
					}
				}
				cssChunks[key] = chunkInfo{
					entryBits:             entryBits,
					isEntryPoint:          true,
					sourceIndex:           entryPoint.SourceIndex,
					entryPointBit:         uint(i),
					filesWithPartsInChunk: cssFilesWithPartsInChunk,
					chunkRepr: &chunkReprCSS{
						importsInChunkInOrder: order,
					},
				}
				chunkRepr.hasCSSChunk = true
			}

		case *graph.CSSRepr:
			order := c.findImportedFilesInCSSOrder([]uint32{entryPoint.SourceIndex})
			for _, entry := range order {
				if entry.kind == cssImportSourceIndex {
					chunk.filesWithPartsInChunk[uint32(entry.sourceIndex)] = true
				}
			}
			chunk.chunkRepr = &chunkReprCSS{
				importsInChunkInOrder: order,
			}
			cssChunks[key] = chunk
		}
	}

	// Figure out which JS files are in which chunk
	for _, sourceIndex := range c.graph.ReachableFiles {
		if file := &c.graph.Files[sourceIndex]; file.IsLive {
			if _, ok := file.InputFile.Repr.(*graph.JSRepr); ok {
				key := file.EntryBits.String()
				chunk, ok := jsChunks[key]
				if !ok {
					chunk.entryBits = file.EntryBits
					chunk.filesWithPartsInChunk = make(map[uint32]bool)
					chunk.chunkRepr = &chunkReprJS{}
					jsChunks[key] = chunk
				}
				chunk.filesWithPartsInChunk[uint32(sourceIndex)] = true
			}
		}
	}

	// Sort the chunks for determinism. This matters because we use chunk indices
	// as sorting keys in a few places.
	sortedChunks := make([]chunkInfo, 0, len(jsChunks)+len(cssChunks))
	sortedKeys := make([]string, 0, len(jsChunks)+len(cssChunks))
	for key := range jsChunks {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)
	jsChunkIndicesForCSS := make(map[string]uint32)
	for _, key := range sortedKeys {
		chunk := jsChunks[key]
		if chunk.chunkRepr.(*chunkReprJS).hasCSSChunk {
			jsChunkIndicesForCSS[key] = uint32(len(sortedChunks))
		}
		sortedChunks = append(sortedChunks, chunk)
	}
	sortedKeys = sortedKeys[:0]
	for key := range cssChunks {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)
	for _, key := range sortedKeys {
		chunk := cssChunks[key]
		if jsChunkIndex, ok := jsChunkIndicesForCSS[key]; ok {
			sortedChunks[jsChunkIndex].chunkRepr.(*chunkReprJS).cssChunkIndex = uint32(len(sortedChunks))
		}
		sortedChunks = append(sortedChunks, chunk)
	}

	// Map from the entry point file to its chunk. We will need this later if
	// a file contains a dynamic import to this entry point, since we'll need
	// to look up the path for this chunk to use with the import.
	for chunkIndex, chunk := range sortedChunks {
		if chunk.isEntryPoint {
			file := &c.graph.Files[chunk.sourceIndex]

			// JS entry points that import CSS files generate two chunks, a JS chunk
			// and a CSS chunk. Don't link the CSS chunk to the JS file since the CSS
			// chunk is secondary (the JS chunk is primary).
			if _, ok := chunk.chunkRepr.(*chunkReprCSS); ok {
				if _, ok := file.InputFile.Repr.(*graph.JSRepr); ok {
					continue
				}
			}

			file.EntryPointChunkIndex = uint32(chunkIndex)
		}
	}

	// Determine the order of JS files (and parts) within the chunk ahead of time
	for _, chunk := range sortedChunks {
		if chunkRepr, ok := chunk.chunkRepr.(*chunkReprJS); ok {
			chunkRepr.filesInChunkInOrder, chunkRepr.partsInChunkInOrder = c.findImportedPartsInJSOrder(&chunk)
		}
	}

	// Assign general information to each chunk
	for chunkIndex := range sortedChunks {
		chunk := &sortedChunks[chunkIndex]

		// Assign a unique key to each chunk. This key encodes the index directly so
		// we can easily recover it later without needing to look it up in a map. The
		// last 8 numbers of the key are the chunk index.
		chunk.uniqueKey = fmt.Sprintf("%sC%08d", c.uniqueKeyPrefix, chunkIndex)

		// Determine the standard file extension
		var stdExt string
		switch chunk.chunkRepr.(type) {
		case *chunkReprJS:
			stdExt = c.options.OutputExtensionJS
		case *chunkReprCSS:
			stdExt = c.options.OutputExtensionCSS
		}

		// Compute the template substitutions
		var dir, base, ext string
		var template []config.PathTemplate
		if chunk.isEntryPoint {
			// Only use the entry path template for user-specified entry points
			file := &c.graph.Files[chunk.sourceIndex]
			if file.IsUserSpecifiedEntryPoint() {
				template = c.options.EntryPathTemplate
			} else {
				template = c.options.ChunkPathTemplate
			}

			if c.options.AbsOutputFile != "" {
				// If the output path was configured explicitly, use it verbatim
				dir = "/"
				base = c.fs.Base(c.options.AbsOutputFile)
				originalExt := c.fs.Ext(base)
				base = base[:len(base)-len(originalExt)]

				// Use the extension from the explicit output file path. However, don't do
				// that if this is a CSS chunk but the entry point file is not CSS. In that
				// case use the standard extension. This happens when importing CSS into JS.
				if _, ok := file.InputFile.Repr.(*graph.CSSRepr); ok || stdExt != c.options.OutputExtensionCSS {
					ext = originalExt
				} else {
					ext = stdExt
				}
			} else {
				// Otherwise, derive the output path from the input path
				dir, base = bundler.PathRelativeToOutbase(
					&c.graph.Files[chunk.sourceIndex].InputFile,
					c.options,
					c.fs,
					!file.IsUserSpecifiedEntryPoint(),
					c.graph.EntryPoints()[chunk.entryPointBit].OutputPath,
				)
				ext = stdExt
			}
		} else {
			dir = "/"
			base = "chunk"
			ext = stdExt
			template = c.options.ChunkPathTemplate
		}

		// Determine the output path template
		templateExt := strings.TrimPrefix(ext, ".")
		template = append(append(make([]config.PathTemplate, 0, len(template)+1), template...), config.PathTemplate{Data: ext})
		chunk.finalTemplate = config.SubstituteTemplate(template, config.PathPlaceholders{
			Dir:  &dir,
			Name: &base,
			Ext:  &templateExt,
		})
	}

	c.chunks = sortedChunks
}

type chunkOrder struct {
	sourceIndex uint32
	distance    uint32
	tieBreaker  uint32
}

// This type is just so we can use Go's native sort function
type chunkOrderArray []chunkOrder

func (a chunkOrderArray) Len() int          { return len(a) }
func (a chunkOrderArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a chunkOrderArray) Less(i int, j int) bool {
	ai := a[i]
	aj := a[j]
	return ai.distance < aj.distance || (ai.distance == aj.distance && ai.tieBreaker < aj.tieBreaker)
}

func appendOrExtendPartRange(ranges []partRange, sourceIndex uint32, partIndex uint32) []partRange {
	if i := len(ranges) - 1; i >= 0 {
		if r := &ranges[i]; r.sourceIndex == sourceIndex && r.partIndexEnd == partIndex {
			r.partIndexEnd = partIndex + 1
			return ranges
		}
	}

	return append(ranges, partRange{
		sourceIndex:    sourceIndex,
		partIndexBegin: partIndex,
		partIndexEnd:   partIndex + 1,
	})
}

func (c *linkerContext) shouldIncludePart(repr *graph.JSRepr, part js_ast.Part) bool {
	// As an optimization, ignore parts containing a single import statement to
	// an internal non-wrapped file. These will be ignored anyway and it's a
	// performance hit to spin up a goroutine only to discover this later.
	if len(part.Stmts) == 1 {
		if s, ok := part.Stmts[0].Data.(*js_ast.SImport); ok {
			record := &repr.AST.ImportRecords[s.ImportRecordIndex]
			if record.SourceIndex.IsValid() && c.graph.Files[record.SourceIndex.GetIndex()].InputFile.Repr.(*graph.JSRepr).Meta.Wrap == graph.WrapNone {
				return false
			}
		}
	}
	return true
}

func (c *linkerContext) findImportedPartsInJSOrder(chunk *chunkInfo) (js []uint32, jsParts []partRange) {
	sorted := make(chunkOrderArray, 0, len(chunk.filesWithPartsInChunk))

	// Attach information to the files for use with sorting
	for sourceIndex := range chunk.filesWithPartsInChunk {
		file := &c.graph.Files[sourceIndex]
		sorted = append(sorted, chunkOrder{
			sourceIndex: sourceIndex,
			distance:    file.DistanceFromEntryPoint,
			tieBreaker:  c.graph.StableSourceIndices[sourceIndex],
		})
	}

	// Sort so files closest to an entry point come first. If two files are
	// equidistant to an entry point, then break the tie by sorting on the
	// stable source index derived from the DFS over all entry points.
	sort.Sort(sorted)

	visited := make(map[uint32]bool)
	jsPartsPrefix := []partRange{}

	// Traverse the graph using this stable order and linearize the files with
	// dependencies before dependents
	var visit func(uint32)
	visit = func(sourceIndex uint32) {
		if visited[sourceIndex] {
			return
		}

		visited[sourceIndex] = true
		file := &c.graph.Files[sourceIndex]

		if repr, ok := file.InputFile.Repr.(*graph.JSRepr); ok {
			isFileInThisChunk := chunk.entryBits.Equals(file.EntryBits)

			// Wrapped files can't be split because they are all inside the wrapper
			canFileBeSplit := repr.Meta.Wrap == graph.WrapNone

			// Make sure the generated call to "__export(exports, ...)" comes first
			// before anything else in this file
			if canFileBeSplit && isFileInThisChunk && repr.AST.Parts[js_ast.NSExportPartIndex].IsLive {
				jsParts = appendOrExtendPartRange(jsParts, sourceIndex, js_ast.NSExportPartIndex)
			}

			for partIndex, part := range repr.AST.Parts {
				isPartInThisChunk := isFileInThisChunk && repr.AST.Parts[partIndex].IsLive

				// Also traverse any files imported by this part
				for _, importRecordIndex := range part.ImportRecordIndices {
					record := &repr.AST.ImportRecords[importRecordIndex]
					if record.SourceIndex.IsValid() && (record.Kind == ast.ImportStmt || isPartInThisChunk) {
						if c.isExternalDynamicImport(record, sourceIndex) {
							// Don't follow import() dependencies
							continue
						}
						visit(record.SourceIndex.GetIndex())
					}
				}

				// Then include this part after the files it imports
				if isPartInThisChunk {
					isFileInThisChunk = true
					if canFileBeSplit && uint32(partIndex) != js_ast.NSExportPartIndex && c.shouldIncludePart(repr, part) {
						if sourceIndex == runtime.SourceIndex {
							jsPartsPrefix = appendOrExtendPartRange(jsPartsPrefix, sourceIndex, uint32(partIndex))
						} else {
							jsParts = appendOrExtendPartRange(jsParts, sourceIndex, uint32(partIndex))
						}
					}
				}
			}

			if isFileInThisChunk {
				js = append(js, sourceIndex)

				// CommonJS files are all-or-nothing so all parts must be contiguous
				if !canFileBeSplit {
					jsPartsPrefix = append(jsPartsPrefix, partRange{
						sourceIndex:    sourceIndex,
						partIndexBegin: 0,
						partIndexEnd:   uint32(len(repr.AST.Parts)),
					})
				}
			}
		}
	}

	// Always put the runtime code first before anything else
	visit(runtime.SourceIndex)
	for _, data := range sorted {
		visit(data.sourceIndex)
	}
	jsParts = append(jsPartsPrefix, jsParts...)
	return
}

func (c *linkerContext) shouldRemoveImportExportStmt(
	sourceIndex uint32,
	stmtList *stmtList,
	loc logger.Loc,
	namespaceRef ast.Ref,
	importRecordIndex uint32,
) bool {
	repr := c.graph.Files[sourceIndex].InputFile.Repr.(*graph.JSRepr)
	record := &repr.AST.ImportRecords[importRecordIndex]

	// Is this an external import?
	if !record.SourceIndex.IsValid() {
		// Keep the "import" statement if "import" statements are supported
		if c.options.OutputFormat.KeepESMImportExportSyntax() {
			return false
		}

		// Otherwise, replace this statement with a call to "require()"
		stmtList.insideWrapperPrefix = append(stmtList.insideWrapperPrefix, js_ast.Stmt{
			Loc: loc,
			Data: &js_ast.SLocal{Decls: []js_ast.Decl{{
				Binding: js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: namespaceRef}},
				ValueOrNil: js_ast.Expr{Loc: record.Range.Loc, Data: &js_ast.ERequireString{
					ImportRecordIndex: importRecordIndex,
				}},
			}}},
		})
		return true
	}

	// We don't need a call to "require()" if this is a self-import inside a
	// CommonJS-style module, since we can just reference the exports directly.
	if repr.AST.ExportsKind == js_ast.ExportsCommonJS && ast.FollowSymbols(c.graph.Symbols, namespaceRef) == repr.AST.ExportsRef {
		return true
	}

	otherFile := &c.graph.Files[record.SourceIndex.GetIndex()]
	otherRepr := otherFile.InputFile.Repr.(*graph.JSRepr)
	switch otherRepr.Meta.Wrap {
	case graph.WrapNone:
		// Remove the statement entirely if this module is not wrapped

	case graph.WrapCJS:
		// Replace the statement with a call to "require()"
		stmtList.insideWrapperPrefix = append(stmtList.insideWrapperPrefix, js_ast.Stmt{
			Loc: loc,
			Data: &js_ast.SLocal{Decls: []js_ast.Decl{{
				Binding: js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: namespaceRef}},
				ValueOrNil: js_ast.Expr{Loc: record.Range.Loc, Data: &js_ast.ERequireString{
					ImportRecordIndex: importRecordIndex,
				}},
			}}},
		})

	case graph.WrapESM:
		// Ignore this file if it's not included in the bundle. This can happen for
		// wrapped ESM files but not for wrapped CommonJS files because we allow
		// tree shaking inside wrapped ESM files.
		if !otherFile.IsLive {
			break
		}

		// Replace the statement with a call to "init()"
		value := js_ast.Expr{Loc: loc, Data: &js_ast.ECall{Target: js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: otherRepr.AST.WrapperRef}}}}
		if otherRepr.Meta.IsAsyncOrHasAsyncDependency {
			// This currently evaluates sibling dependencies in serial instead of in
			// parallel, which is incorrect. This should be changed to store a promise
			// and await all stored promises after all imports but before any code.
			value.Data = &js_ast.EAwait{Value: value}
		}
		stmtList.insideWrapperPrefix = append(stmtList.insideWrapperPrefix, js_ast.Stmt{Loc: loc, Data: &js_ast.SExpr{Value: value}})
	}

	return true
}

func (c *linkerContext) convertStmtsForChunk(sourceIndex uint32, stmtList *stmtList, partStmts []js_ast.Stmt) {
	file := &c.graph.Files[sourceIndex]
	shouldStripExports := c.options.Mode != config.ModePassThrough || !file.IsEntryPoint()
	repr := file.InputFile.Repr.(*graph.JSRepr)
	shouldExtractESMStmtsForWrap := repr.Meta.Wrap != graph.WrapNone

	// If this file is a CommonJS entry point, double-write re-exports to the
	// external CommonJS "module.exports" object in addition to our internal ESM
	// export namespace object. The difference between these two objects is that
	// our internal one must not have the "__esModule" marker while the external
	// one must have the "__esModule" marker. This is done because an ES module
	// importing itself should not see the "__esModule" marker but a CommonJS module
	// importing us should see the "__esModule" marker.
	var moduleExportsForReExportOrNil js_ast.Expr
	if c.options.OutputFormat == config.FormatCommonJS && file.IsEntryPoint() {
		moduleExportsForReExportOrNil = js_ast.Expr{Data: &js_ast.EDot{
			Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: c.unboundModuleRef}},
			Name:   "exports",
		}}
	}

	for _, stmt := range partStmts {
		switch s := stmt.Data.(type) {
		case *js_ast.SImport:
			// "import * as ns from 'path'"
			// "import {foo} from 'path'"
			if c.shouldRemoveImportExportStmt(sourceIndex, stmtList, stmt.Loc, s.NamespaceRef, s.ImportRecordIndex) {
				continue
			}

			if c.options.UnsupportedJSFeatures.Has(compat.ArbitraryModuleNamespaceNames) && s.Items != nil {
				for _, item := range *s.Items {
					c.maybeForbidArbitraryModuleNamespaceIdentifier("import", sourceIndex, item.AliasLoc, item.Alias)
				}
			}

			// Make sure these don't end up in the wrapper closure
			if shouldExtractESMStmtsForWrap {
				stmtList.outsideWrapperPrefix = append(stmtList.outsideWrapperPrefix, stmt)
				continue
			}

		case *js_ast.SExportStar:
			// "export * as ns from 'path'"
			if s.Alias != nil {
				if c.shouldRemoveImportExportStmt(sourceIndex, stmtList, stmt.Loc, s.NamespaceRef, s.ImportRecordIndex) {
					continue
				}

				if c.options.UnsupportedJSFeatures.Has(compat.ArbitraryModuleNamespaceNames) {
					c.maybeForbidArbitraryModuleNamespaceIdentifier("export", sourceIndex, s.Alias.Loc, s.Alias.OriginalName)
				}

				if shouldStripExports {
					// Turn this statement into "import * as ns from 'path'"
					stmt.Data = &js_ast.SImport{
						NamespaceRef:      s.NamespaceRef,
						StarNameLoc:       &s.Alias.Loc,
						ImportRecordIndex: s.ImportRecordIndex,
					}
				}

				// Make sure these don't end up in the wrapper closure
				if shouldExtractESMStmtsForWrap {
					stmtList.outsideWrapperPrefix = append(stmtList.outsideWrapperPrefix, stmt)
					continue
				}
				break
			}

			// "export * from 'path'"
			if !shouldStripExports {
				break
			}
			record := &repr.AST.ImportRecords[s.ImportRecordIndex]

			// Is this export star evaluated at run time?
			if !record.SourceIndex.IsValid() && c.options.OutputFormat.KeepESMImportExportSyntax() {
				if record.Flags.Has(ast.CallsRunTimeReExportFn) {
					// Turn this statement into "import * as ns from 'path'"
					stmt.Data = &js_ast.SImport{
						NamespaceRef:      s.NamespaceRef,
						StarNameLoc:       &logger.Loc{Start: stmt.Loc.Start},
						ImportRecordIndex: s.ImportRecordIndex,
					}

					// Prefix this module with "__reExport(exports, ns, module.exports)"
					exportStarRef := c.graph.Files[runtime.SourceIndex].InputFile.Repr.(*graph.JSRepr).AST.ModuleScope.Members["__reExport"].Ref
					args := []js_ast.Expr{
						{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: repr.AST.ExportsRef}},
						{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: s.NamespaceRef}},
					}
					if moduleExportsForReExportOrNil.Data != nil {
						args = append(args, moduleExportsForReExportOrNil)
					}
					stmtList.insideWrapperPrefix = append(stmtList.insideWrapperPrefix, js_ast.Stmt{
						Loc: stmt.Loc,
						Data: &js_ast.SExpr{Value: js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.ECall{
							Target: js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: exportStarRef}},
							Args:   args,
						}}},
					})

					// Make sure these don't end up in the wrapper closure
					if shouldExtractESMStmtsForWrap {
						stmtList.outsideWrapperPrefix = append(stmtList.outsideWrapperPrefix, stmt)
						continue
					}
				}
			} else {
				if record.SourceIndex.IsValid() {
					if otherRepr := c.graph.Files[record.SourceIndex.GetIndex()].InputFile.Repr.(*graph.JSRepr); otherRepr.Meta.Wrap == graph.WrapESM {
						stmtList.insideWrapperPrefix = append(stmtList.insideWrapperPrefix, js_ast.Stmt{Loc: stmt.Loc,
							Data: &js_ast.SExpr{Value: js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.ECall{
								Target: js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: otherRepr.AST.WrapperRef}}}}}})
					}
				}

				if record.Flags.Has(ast.CallsRunTimeReExportFn) {
					var target js_ast.E
					if record.SourceIndex.IsValid() {
						if otherRepr := c.graph.Files[record.SourceIndex.GetIndex()].InputFile.Repr.(*graph.JSRepr); otherRepr.AST.ExportsKind == js_ast.ExportsESMWithDynamicFallback {
							// Prefix this module with "__reExport(exports, otherExports, module.exports)"
							target = &js_ast.EIdentifier{Ref: otherRepr.AST.ExportsRef}
						}
					}
					if target == nil {
						// Prefix this module with "__reExport(exports, require(path), module.exports)"
						target = &js_ast.ERequireString{
							ImportRecordIndex: s.ImportRecordIndex,
						}
					}
					exportStarRef := c.graph.Files[runtime.SourceIndex].InputFile.Repr.(*graph.JSRepr).AST.ModuleScope.Members["__reExport"].Ref
					args := []js_ast.Expr{
						{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: repr.AST.ExportsRef}},
						{Loc: record.Range.Loc, Data: target},
					}
					if moduleExportsForReExportOrNil.Data != nil {
						args = append(args, moduleExportsForReExportOrNil)
					}
					stmtList.insideWrapperPrefix = append(stmtList.insideWrapperPrefix, js_ast.Stmt{
						Loc: stmt.Loc,
						Data: &js_ast.SExpr{Value: js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.ECall{
							Target: js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: exportStarRef}},
							Args:   args,
						}}},
					})
				}

				// Remove the export star statement
				continue
			}

		case *js_ast.SExportFrom:
			// "export {foo} from 'path'"
			if c.shouldRemoveImportExportStmt(sourceIndex, stmtList, stmt.Loc, s.NamespaceRef, s.ImportRecordIndex) {
				continue
			}

			if c.options.UnsupportedJSFeatures.Has(compat.ArbitraryModuleNamespaceNames) {
				for _, item := range s.Items {
					c.maybeForbidArbitraryModuleNamespaceIdentifier("export", sourceIndex, item.AliasLoc, item.Alias)
					if item.AliasLoc != item.Name.Loc {
						c.maybeForbidArbitraryModuleNamespaceIdentifier("import", sourceIndex, item.Name.Loc, item.OriginalName)
					}
				}
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

			// Make sure these don't end up in the wrapper closure
			if shouldExtractESMStmtsForWrap {
				stmtList.outsideWrapperPrefix = append(stmtList.outsideWrapperPrefix, stmt)
				continue
			}

		case *js_ast.SExportClause:
			if shouldStripExports {
				// Remove export statements entirely
				continue
			}

			if c.options.UnsupportedJSFeatures.Has(compat.ArbitraryModuleNamespaceNames) {
				for _, item := range s.Items {
					c.maybeForbidArbitraryModuleNamespaceIdentifier("export", sourceIndex, item.AliasLoc, item.Alias)
				}
			}

			// Make sure these don't end up in the wrapper closure
			if shouldExtractESMStmtsForWrap {
				stmtList.outsideWrapperPrefix = append(stmtList.outsideWrapperPrefix, stmt)
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
			if shouldStripExports && s.IsExport {
				// Be careful to not modify the original statement
				clone := *s
				clone.IsExport = false
				stmt.Data = &clone
			}

		case *js_ast.SLocal:
			if shouldStripExports && s.IsExport {
				// Be careful to not modify the original statement
				clone := *s
				clone.IsExport = false
				stmt.Data = &clone
			}

		case *js_ast.SExportDefault:
			// If we're bundling, convert "export default" into a normal declaration
			if shouldStripExports {
				switch s2 := s.Value.Data.(type) {
				case *js_ast.SExpr:
					// "export default foo;" => "var default = foo;"
					stmt = js_ast.Stmt{Loc: stmt.Loc, Data: &js_ast.SLocal{Decls: []js_ast.Decl{
						{Binding: js_ast.Binding{Loc: s.DefaultName.Loc, Data: &js_ast.BIdentifier{Ref: s.DefaultName.Ref}}, ValueOrNil: s2.Value},
					}}}

				case *js_ast.SFunction:
					// "export default function() {}" => "function default() {}"
					// "export default function foo() {}" => "function foo() {}"

					// Be careful to not modify the original statement
					s2 = &js_ast.SFunction{Fn: s2.Fn}
					s2.Fn.Name = &s.DefaultName

					stmt = js_ast.Stmt{Loc: s.Value.Loc, Data: s2}

				case *js_ast.SClass:
					// "export default class {}" => "class default {}"
					// "export default class Foo {}" => "class Foo {}"

					// Be careful to not modify the original statement
					s2 = &js_ast.SClass{Class: s2.Class}
					s2.Class.Name = &s.DefaultName

					stmt = js_ast.Stmt{Loc: s.Value.Loc, Data: s2}

				default:
					panic("Internal error")
				}
			}
		}

		stmtList.insideWrapperSuffix = append(stmtList.insideWrapperSuffix, stmt)
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
	// These statements come first, and can be inside the wrapper
	insideWrapperPrefix []js_ast.Stmt

	// These statements come last, and can be inside the wrapper
	insideWrapperSuffix []js_ast.Stmt

	outsideWrapperPrefix []js_ast.Stmt
}

type compileResultJS struct {
	js_printer.PrintResult

	sourceIndex uint32

	// This is the line and column offset since the previous JavaScript string
	// or the start of the file if this is the first JavaScript string.
	generatedOffset sourcemap.LineColumnOffset
}

func (c *linkerContext) requireOrImportMetaForSource(sourceIndex uint32) (meta js_printer.RequireOrImportMeta) {
	repr := c.graph.Files[sourceIndex].InputFile.Repr.(*graph.JSRepr)
	meta.WrapperRef = repr.AST.WrapperRef
	meta.IsWrapperAsync = repr.Meta.IsAsyncOrHasAsyncDependency
	if repr.Meta.Wrap == graph.WrapESM {
		meta.ExportsRef = repr.AST.ExportsRef
	} else {
		meta.ExportsRef = ast.InvalidRef
	}
	return
}

func (c *linkerContext) generateCodeForFileInChunkJS(
	r renamer.Renamer,
	waitGroup *sync.WaitGroup,
	partRange partRange,
	toCommonJSRef ast.Ref,
	toESMRef ast.Ref,
	runtimeRequireRef ast.Ref,
	result *compileResultJS,
	dataForSourceMaps []bundler.DataForSourceMap,
) {
	defer c.recoverInternalError(waitGroup, partRange.sourceIndex)

	file := &c.graph.Files[partRange.sourceIndex]
	repr := file.InputFile.Repr.(*graph.JSRepr)
	nsExportPartIndex := js_ast.NSExportPartIndex
	needsWrapper := false
	stmtList := stmtList{}

	// The top-level directive must come first (the non-wrapped case is handled
	// by the chunk generation code, although only for the entry point)
	if repr.Meta.Wrap != graph.WrapNone && !file.IsEntryPoint() {
		for _, directive := range repr.AST.Directives {
			stmtList.insideWrapperPrefix = append(stmtList.insideWrapperPrefix, js_ast.Stmt{
				Data: &js_ast.SDirective{Value: helpers.StringToUTF16(directive)},
			})
		}
	}

	// Make sure the generated call to "__export(exports, ...)" comes first
	// before anything else.
	if nsExportPartIndex >= partRange.partIndexBegin && nsExportPartIndex < partRange.partIndexEnd &&
		repr.AST.Parts[nsExportPartIndex].IsLive {
		c.convertStmtsForChunk(partRange.sourceIndex, &stmtList, repr.AST.Parts[nsExportPartIndex].Stmts)

		// Move everything to the prefix list
		if repr.Meta.Wrap == graph.WrapESM {
			stmtList.outsideWrapperPrefix = append(stmtList.outsideWrapperPrefix, stmtList.insideWrapperSuffix...)
		} else {
			stmtList.insideWrapperPrefix = append(stmtList.insideWrapperPrefix, stmtList.insideWrapperSuffix...)
		}
		stmtList.insideWrapperSuffix = nil
	}

	var partIndexForLazyDefaultExport ast.Index32
	if repr.AST.HasLazyExport {
		if defaultExport, ok := repr.Meta.ResolvedExports["default"]; ok {
			partIndexForLazyDefaultExport = ast.MakeIndex32(repr.TopLevelSymbolToParts(defaultExport.Ref)[0])
		}
	}

	// Add all other parts in this chunk
	for partIndex := partRange.partIndexBegin; partIndex < partRange.partIndexEnd; partIndex++ {
		part := repr.AST.Parts[partIndex]
		if !repr.AST.Parts[partIndex].IsLive {
			// Skip the part if it's not in this chunk
			continue
		}

		if uint32(partIndex) == nsExportPartIndex {
			// Skip the generated call to "__export()" that was extracted above
			continue
		}

		// Mark if we hit the dummy part representing the wrapper
		if uint32(partIndex) == repr.Meta.WrapperPartIndex.GetIndex() {
			needsWrapper = true
			continue
		}

		stmts := part.Stmts

		// If this could be a JSON file that exports a top-level object literal, go
		// over the non-default top-level properties that ended up being imported
		// and substitute references to them into the main top-level object literal.
		// So this JSON file:
		//
		//   {
		//     "foo": [1, 2, 3],
		//     "bar": [4, 5, 6],
		//   }
		//
		// is initially compiled into this:
		//
		//   export var foo = [1, 2, 3];
		//   export var bar = [4, 5, 6];
		//   export default {
		//     foo: [1, 2, 3],
		//     bar: [4, 5, 6],
		//   };
		//
		// But we turn it into this if both "foo" and "default" are imported:
		//
		//   export var foo = [1, 2, 3];
		//   export default {
		//     foo,
		//     bar: [4, 5, 6],
		//   };
		//
		if partIndexForLazyDefaultExport.IsValid() && partIndex == partIndexForLazyDefaultExport.GetIndex() {
			stmt := stmts[0]
			defaultExport := stmt.Data.(*js_ast.SExportDefault)
			defaultExpr := defaultExport.Value.Data.(*js_ast.SExpr)

			// Be careful: the top-level value in a JSON file is not necessarily an object
			if object, ok := defaultExpr.Value.Data.(*js_ast.EObject); ok {
				objectClone := *object
				objectClone.Properties = append([]js_ast.Property{}, objectClone.Properties...)

				// If any top-level properties ended up being imported directly, change
				// the property to just reference the corresponding variable instead
				for i, property := range object.Properties {
					if str, ok := property.Key.Data.(*js_ast.EString); ok {
						if name := helpers.UTF16ToString(str.Value); name != "default" {
							if export, ok := repr.Meta.ResolvedExports[name]; ok {
								if part := repr.AST.Parts[repr.TopLevelSymbolToParts(export.Ref)[0]]; part.IsLive {
									ref := part.Stmts[0].Data.(*js_ast.SLocal).Decls[0].Binding.Data.(*js_ast.BIdentifier).Ref
									objectClone.Properties[i].ValueOrNil = js_ast.Expr{Loc: property.Key.Loc, Data: &js_ast.EIdentifier{Ref: ref}}
								}
							}
						}
					}
				}

				// Avoid mutating the original AST
				defaultExprClone := *defaultExpr
				defaultExprClone.Value.Data = &objectClone
				defaultExportClone := *defaultExport
				defaultExportClone.Value.Data = &defaultExprClone
				stmt.Data = &defaultExportClone
				stmts = []js_ast.Stmt{stmt}
			}
		}

		c.convertStmtsForChunk(partRange.sourceIndex, &stmtList, stmts)
	}

	// Hoist all import statements before any normal statements. ES6 imports
	// are different than CommonJS imports. All modules imported via ES6 import
	// statements are evaluated before the module doing the importing is
	// evaluated (well, except for cyclic import scenarios). We need to preserve
	// these semantics even when modules imported via ES6 import statements end
	// up being CommonJS modules.
	stmts := stmtList.insideWrapperSuffix
	if len(stmtList.insideWrapperPrefix) > 0 {
		stmts = append(stmtList.insideWrapperPrefix, stmts...)
	}
	if c.options.MinifySyntax {
		stmts = mergeAdjacentLocalStmts(stmts)
	}

	// Optionally wrap all statements in a closure
	if needsWrapper {
		switch repr.Meta.Wrap {
		case graph.WrapCJS:
			// Only include the arguments that are actually used
			args := []js_ast.Arg{}
			if repr.AST.UsesExportsRef || repr.AST.UsesModuleRef {
				args = append(args, js_ast.Arg{Binding: js_ast.Binding{Data: &js_ast.BIdentifier{Ref: repr.AST.ExportsRef}}})
				if repr.AST.UsesModuleRef {
					args = append(args, js_ast.Arg{Binding: js_ast.Binding{Data: &js_ast.BIdentifier{Ref: repr.AST.ModuleRef}}})
				}
			}

			var cjsArgs []js_ast.Expr
			if c.options.ProfilerNames {
				// "__commonJS({ 'file.js'(exports, module) { ... } })"
				kind := js_ast.PropertyField
				if !c.options.UnsupportedJSFeatures.Has(compat.ObjectExtensions) {
					kind = js_ast.PropertyMethod
				}
				cjsArgs = []js_ast.Expr{{Data: &js_ast.EObject{Properties: []js_ast.Property{{
					Kind:       kind,
					Key:        js_ast.Expr{Data: &js_ast.EString{Value: helpers.StringToUTF16(file.InputFile.Source.PrettyPath)}},
					ValueOrNil: js_ast.Expr{Data: &js_ast.EFunction{Fn: js_ast.Fn{Args: args, Body: js_ast.FnBody{Block: js_ast.SBlock{Stmts: stmts}}}}},
				}}}}}
			} else if c.options.UnsupportedJSFeatures.Has(compat.Arrow) {
				// "__commonJS(function (exports, module) { ... })"
				cjsArgs = []js_ast.Expr{{Data: &js_ast.EFunction{Fn: js_ast.Fn{Args: args, Body: js_ast.FnBody{Block: js_ast.SBlock{Stmts: stmts}}}}}}
			} else {
				// "__commonJS((exports, module) => { ... })"
				cjsArgs = []js_ast.Expr{{Data: &js_ast.EArrow{Args: args, Body: js_ast.FnBody{Block: js_ast.SBlock{Stmts: stmts}}}}}
			}
			value := js_ast.Expr{Data: &js_ast.ECall{
				Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: c.cjsRuntimeRef}},
				Args:   cjsArgs,
			}}

			// "var require_foo = __commonJS(...);"
			stmts = append(stmtList.outsideWrapperPrefix, js_ast.Stmt{Data: &js_ast.SLocal{
				Decls: []js_ast.Decl{{
					Binding:    js_ast.Binding{Data: &js_ast.BIdentifier{Ref: repr.AST.WrapperRef}},
					ValueOrNil: value,
				}},
			}})

		case graph.WrapESM:
			// The wrapper only needs to be "async" if there is a transitive async
			// dependency. For correctness, we must not use "async" if the module
			// isn't async because then calling "require()" on that module would
			// swallow any exceptions thrown during module initialization.
			isAsync := repr.Meta.IsAsyncOrHasAsyncDependency

			// Hoist all top-level "var" and "function" declarations out of the closure
			var decls []js_ast.Decl
			end := 0
			for _, stmt := range stmts {
				switch s := stmt.Data.(type) {
				case *js_ast.SLocal:
					// Convert the declarations to assignments
					wrapIdentifier := func(loc logger.Loc, ref ast.Ref) js_ast.Expr {
						decls = append(decls, js_ast.Decl{Binding: js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: ref}}})
						return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: ref}}
					}
					var value js_ast.Expr
					for _, decl := range s.Decls {
						binding := js_ast.ConvertBindingToExpr(decl.Binding, wrapIdentifier)
						if decl.ValueOrNil.Data != nil {
							value = js_ast.JoinWithComma(value, js_ast.Assign(binding, decl.ValueOrNil))
						}
					}
					if value.Data == nil {
						continue
					}
					stmt = js_ast.Stmt{Loc: stmt.Loc, Data: &js_ast.SExpr{Value: value}}

				case *js_ast.SFunction:
					stmtList.outsideWrapperPrefix = append(stmtList.outsideWrapperPrefix, stmt)
					continue
				}

				stmts[end] = stmt
				end++
			}
			stmts = stmts[:end]

			var esmArgs []js_ast.Expr
			if c.options.ProfilerNames {
				// "__esm({ 'file.js'() { ... } })"
				kind := js_ast.PropertyField
				if !c.options.UnsupportedJSFeatures.Has(compat.ObjectExtensions) {
					kind = js_ast.PropertyMethod
				}
				esmArgs = []js_ast.Expr{{Data: &js_ast.EObject{Properties: []js_ast.Property{{
					Kind:       kind,
					Key:        js_ast.Expr{Data: &js_ast.EString{Value: helpers.StringToUTF16(file.InputFile.Source.PrettyPath)}},
					ValueOrNil: js_ast.Expr{Data: &js_ast.EFunction{Fn: js_ast.Fn{Body: js_ast.FnBody{Block: js_ast.SBlock{Stmts: stmts}}, IsAsync: isAsync}}},
				}}}}}
			} else if c.options.UnsupportedJSFeatures.Has(compat.Arrow) {
				// "__esm(function () { ... })"
				esmArgs = []js_ast.Expr{{Data: &js_ast.EFunction{Fn: js_ast.Fn{Body: js_ast.FnBody{Block: js_ast.SBlock{Stmts: stmts}}, IsAsync: isAsync}}}}
			} else {
				// "__esm(() => { ... })"
				esmArgs = []js_ast.Expr{{Data: &js_ast.EArrow{Body: js_ast.FnBody{Block: js_ast.SBlock{Stmts: stmts}}, IsAsync: isAsync}}}
			}
			value := js_ast.Expr{Data: &js_ast.ECall{
				Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: c.esmRuntimeRef}},
				Args:   esmArgs,
			}}

			// "var foo, bar;"
			if !c.options.MinifySyntax && len(decls) > 0 {
				stmtList.outsideWrapperPrefix = append(stmtList.outsideWrapperPrefix, js_ast.Stmt{Data: &js_ast.SLocal{
					Decls: decls,
				}})
				decls = nil
			}

			// "var init_foo = __esm(...);"
			stmts = append(stmtList.outsideWrapperPrefix, js_ast.Stmt{Data: &js_ast.SLocal{
				Decls: append(decls, js_ast.Decl{
					Binding:    js_ast.Binding{Data: &js_ast.BIdentifier{Ref: repr.AST.WrapperRef}},
					ValueOrNil: value,
				}),
			}})
		}
	}

	// Only generate a source map if needed
	var addSourceMappings bool
	var inputSourceMap *sourcemap.SourceMap
	var lineOffsetTables []sourcemap.LineOffsetTable
	if file.InputFile.Loader.CanHaveSourceMap() && c.options.SourceMap != config.SourceMapNone {
		addSourceMappings = true
		inputSourceMap = file.InputFile.InputSourceMap
		lineOffsetTables = dataForSourceMaps[partRange.sourceIndex].LineOffsetTables
	}

	// Indent the file if everything is wrapped in an IIFE
	indent := 0
	if c.options.OutputFormat == config.FormatIIFE {
		indent++
	}

	// Convert the AST to JavaScript code
	printOptions := js_printer.Options{
		Indent:                       indent,
		OutputFormat:                 c.options.OutputFormat,
		MinifyIdentifiers:            c.options.MinifyIdentifiers,
		MinifyWhitespace:             c.options.MinifyWhitespace,
		MinifySyntax:                 c.options.MinifySyntax,
		LineLimit:                    c.options.LineLimit,
		ASCIIOnly:                    c.options.ASCIIOnly,
		ToCommonJSRef:                toCommonJSRef,
		ToESMRef:                     toESMRef,
		RuntimeRequireRef:            runtimeRequireRef,
		TSEnums:                      c.graph.TSEnums,
		ConstValues:                  c.graph.ConstValues,
		LegalComments:                c.options.LegalComments,
		UnsupportedFeatures:          c.options.UnsupportedJSFeatures,
		SourceMap:                    c.options.SourceMap,
		AddSourceMappings:            addSourceMappings,
		InputSourceMap:               inputSourceMap,
		LineOffsetTables:             lineOffsetTables,
		RequireOrImportMetaForSource: c.requireOrImportMetaForSource,
		MangledProps:                 c.mangledProps,
		NeedsMetafile:                c.options.NeedsMetafile,
	}
	tree := repr.AST
	tree.Directives = nil // This is handled elsewhere
	tree.Parts = []js_ast.Part{{Stmts: stmts}}
	*result = compileResultJS{
		PrintResult: js_printer.Print(tree, c.graph.Symbols, r, printOptions),
		sourceIndex: partRange.sourceIndex,
	}

	if file.InputFile.Loader == config.LoaderFile {
		result.JSONMetadataImports = append(result.JSONMetadataImports, fmt.Sprintf("\n        {\n          \"path\": %s,\n          \"kind\": \"file-loader\"\n        }",
			helpers.QuoteForJSON(file.InputFile.UniqueKeyForAdditionalFile, c.options.ASCIIOnly)))
	}

	waitGroup.Done()
}

func (c *linkerContext) generateEntryPointTailJS(
	r renamer.Renamer,
	toCommonJSRef ast.Ref,
	toESMRef ast.Ref,
	sourceIndex uint32,
) (result compileResultJS) {
	file := &c.graph.Files[sourceIndex]
	repr := file.InputFile.Repr.(*graph.JSRepr)
	var stmts []js_ast.Stmt

	switch c.options.OutputFormat {
	case config.FormatPreserve:
		if repr.Meta.Wrap != graph.WrapNone {
			// "require_foo();"
			// "init_foo();"
			stmts = append(stmts, js_ast.Stmt{Data: &js_ast.SExpr{Value: js_ast.Expr{Data: &js_ast.ECall{
				Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: repr.AST.WrapperRef}},
			}}}})
		}

	case config.FormatIIFE:
		if repr.Meta.Wrap == graph.WrapCJS {
			if len(c.options.GlobalName) > 0 {
				// "return require_foo();"
				stmts = append(stmts, js_ast.Stmt{Data: &js_ast.SReturn{ValueOrNil: js_ast.Expr{Data: &js_ast.ECall{
					Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: repr.AST.WrapperRef}},
				}}}})
			} else {
				// "require_foo();"
				stmts = append(stmts, js_ast.Stmt{Data: &js_ast.SExpr{Value: js_ast.Expr{Data: &js_ast.ECall{
					Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: repr.AST.WrapperRef}},
				}}}})
			}
		} else {
			if repr.Meta.Wrap == graph.WrapESM {
				// "init_foo();"
				stmts = append(stmts, js_ast.Stmt{Data: &js_ast.SExpr{Value: js_ast.Expr{Data: &js_ast.ECall{
					Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: repr.AST.WrapperRef}},
				}}}})
			}

			if repr.Meta.ForceIncludeExportsForEntryPoint {
				// "return __toCommonJS(exports);"
				stmts = append(stmts, js_ast.Stmt{Data: &js_ast.SReturn{
					ValueOrNil: js_ast.Expr{Data: &js_ast.ECall{
						Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: toCommonJSRef}},
						Args:   []js_ast.Expr{{Data: &js_ast.EIdentifier{Ref: repr.AST.ExportsRef}}},
					}},
				}})
			}
		}

	case config.FormatCommonJS:
		if repr.Meta.Wrap == graph.WrapCJS {
			// "module.exports = require_foo();"
			stmts = append(stmts, js_ast.AssignStmt(
				js_ast.Expr{Data: &js_ast.EDot{
					Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: c.unboundModuleRef}},
					Name:   "exports",
				}},
				js_ast.Expr{Data: &js_ast.ECall{
					Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: repr.AST.WrapperRef}},
				}},
			))
		} else {
			if repr.Meta.Wrap == graph.WrapESM {
				// "init_foo();"
				stmts = append(stmts, js_ast.Stmt{Data: &js_ast.SExpr{Value: js_ast.Expr{Data: &js_ast.ECall{
					Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: repr.AST.WrapperRef}},
				}}}})
			}
		}

		// If we are generating CommonJS for node, encode the known export names in
		// a form that node can understand them. This relies on the specific behavior
		// of this parser, which the node project uses to detect named exports in
		// CommonJS files: https://github.com/guybedford/cjs-module-lexer. Think of
		// this code as an annotation for that parser.
		if c.options.Platform == config.PlatformNode {
			// Add a comment since otherwise people will surely wonder what this is.
			// This annotation means you can do this and have it work:
			//
			//   import { name } from './file-from-esbuild.cjs'
			//
			// when "file-from-esbuild.cjs" looks like this:
			//
			//   __export(exports, { name: () => name });
			//   0 && (module.exports = {name});
			//
			// The maintainer of "cjs-module-lexer" is receptive to adding esbuild-
			// friendly patterns to this library. However, this library has already
			// shipped in node and using existing patterns instead of defining new
			// patterns is maximally compatible.
			//
			// An alternative to doing this could be to use "Object.defineProperties"
			// instead of "__export" but support for that would need to be added to
			// "cjs-module-lexer" and then we would need to be ok with not supporting
			// older versions of node that don't have that newly-added support.

			// "{a, b, if: null}"
			var moduleExports []js_ast.Property
			for _, export := range repr.Meta.SortedAndFilteredExportAliases {
				if export == "default" {
					// In node the default export is always "module.exports" regardless of
					// what the annotation says. So don't bother generating "default".
					continue
				}

				// "{if: null}"
				var valueOrNil js_ast.Expr
				if _, ok := js_lexer.Keywords[export]; ok || !js_ast.IsIdentifier(export) {
					// Make sure keywords don't cause a syntax error. This has to map to
					// "null" instead of something shorter like "0" because the library
					// "cjs-module-lexer" only supports identifiers in this position, and
					// it thinks "null" is an identifier.
					valueOrNil = js_ast.Expr{Data: js_ast.ENullShared}
				}

				moduleExports = append(moduleExports, js_ast.Property{
					Key:        js_ast.Expr{Data: &js_ast.EString{Value: helpers.StringToUTF16(export)}},
					ValueOrNil: valueOrNil,
				})
			}

			// Add annotations for re-exports: "{...require('./foo')}"
			for _, importRecordIndex := range repr.AST.ExportStarImportRecords {
				if record := &repr.AST.ImportRecords[importRecordIndex]; !record.SourceIndex.IsValid() {
					moduleExports = append(moduleExports, js_ast.Property{
						Kind:       js_ast.PropertySpread,
						ValueOrNil: js_ast.Expr{Data: &js_ast.ERequireString{ImportRecordIndex: importRecordIndex}},
					})
				}
			}

			if len(moduleExports) > 0 {
				// "0 && (module.exports = {a, b, if: null});"
				expr := js_ast.Expr{Data: &js_ast.EBinary{
					Op:   js_ast.BinOpLogicalAnd,
					Left: js_ast.Expr{Data: &js_ast.ENumber{Value: 0}},
					Right: js_ast.Assign(
						js_ast.Expr{Data: &js_ast.EDot{
							Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: c.unboundModuleRef}},
							Name:   "exports",
						}},
						js_ast.Expr{Data: &js_ast.EObject{Properties: moduleExports}},
					),
				}}

				if !c.options.MinifyWhitespace {
					stmts = append(stmts,
						js_ast.Stmt{Data: &js_ast.SComment{Text: `// Annotate the CommonJS export names for ESM import in node:`}},
					)
				}

				stmts = append(stmts, js_ast.Stmt{Data: &js_ast.SExpr{Value: expr}})
			}
		}

	case config.FormatESModule:
		if repr.Meta.Wrap == graph.WrapCJS {
			// "export default require_foo();"
			stmts = append(stmts, js_ast.Stmt{
				Data: &js_ast.SExportDefault{Value: js_ast.Stmt{
					Data: &js_ast.SExpr{Value: js_ast.Expr{
						Data: &js_ast.ECall{Target: js_ast.Expr{
							Data: &js_ast.EIdentifier{Ref: repr.AST.WrapperRef}}}}}}}})
		} else {
			if repr.Meta.Wrap == graph.WrapESM {
				if repr.Meta.IsAsyncOrHasAsyncDependency {
					// "await init_foo();"
					stmts = append(stmts, js_ast.Stmt{
						Data: &js_ast.SExpr{Value: js_ast.Expr{
							Data: &js_ast.EAwait{Value: js_ast.Expr{
								Data: &js_ast.ECall{Target: js_ast.Expr{
									Data: &js_ast.EIdentifier{Ref: repr.AST.WrapperRef}}}}}}}})
				} else {
					// "init_foo();"
					stmts = append(stmts, js_ast.Stmt{
						Data: &js_ast.SExpr{
							Value: js_ast.Expr{Data: &js_ast.ECall{Target: js_ast.Expr{
								Data: &js_ast.EIdentifier{Ref: repr.AST.WrapperRef}}}}}})
				}
			}

			if len(repr.Meta.SortedAndFilteredExportAliases) > 0 {
				// If the output format is ES6 modules and we're an entry point, generate an
				// ES6 export statement containing all exports. Except don't do that if this
				// entry point is a CommonJS-style module, since that would generate an ES6
				// export statement that's not top-level. Instead, we will export the CommonJS
				// exports as a default export later on.
				var items []js_ast.ClauseItem

				for i, alias := range repr.Meta.SortedAndFilteredExportAliases {
					export := repr.Meta.ResolvedExports[alias]

					// If this is an export of an import, reference the symbol that the import
					// was eventually resolved to. We need to do this because imports have
					// already been resolved by this point, so we can't generate a new import
					// and have that be resolved later.
					if importData, ok := c.graph.Files[export.SourceIndex].InputFile.Repr.(*graph.JSRepr).Meta.ImportsToBind[export.Ref]; ok {
						export.Ref = importData.Ref
						export.SourceIndex = importData.SourceIndex
					}

					// Exports of imports need EImportIdentifier in case they need to be re-
					// written to a property access later on
					if c.graph.Symbols.Get(export.Ref).NamespaceAlias != nil {
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
						//   var cjs_format = __toESM(require_cjs_format());
						//   var export_foo = cjs_format.foo;
						//   export {
						//     export_foo as foo
						//   };
						//
						tempRef := repr.Meta.CJSExportCopies[i]
						stmts = append(stmts, js_ast.Stmt{Data: &js_ast.SLocal{
							Decls: []js_ast.Decl{{
								Binding:    js_ast.Binding{Data: &js_ast.BIdentifier{Ref: tempRef}},
								ValueOrNil: js_ast.Expr{Data: &js_ast.EImportIdentifier{Ref: export.Ref}},
							}},
						}})
						items = append(items, js_ast.ClauseItem{
							Name:  ast.LocRef{Ref: tempRef},
							Alias: alias,
						})
					} else {
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
						items = append(items, js_ast.ClauseItem{
							Name:  ast.LocRef{Ref: export.Ref},
							Alias: alias,
						})
					}
				}

				stmts = append(stmts, js_ast.Stmt{Data: &js_ast.SExportClause{Items: items}})
			}
		}
	}

	if len(stmts) == 0 {
		return
	}

	tree := repr.AST
	tree.Directives = nil
	tree.Parts = []js_ast.Part{{Stmts: stmts}}

	// Indent the file if everything is wrapped in an IIFE
	indent := 0
	if c.options.OutputFormat == config.FormatIIFE {
		indent++
	}

	// Convert the AST to JavaScript code
	printOptions := js_printer.Options{
		Indent:                       indent,
		OutputFormat:                 c.options.OutputFormat,
		MinifyIdentifiers:            c.options.MinifyIdentifiers,
		MinifyWhitespace:             c.options.MinifyWhitespace,
		MinifySyntax:                 c.options.MinifySyntax,
		LineLimit:                    c.options.LineLimit,
		ASCIIOnly:                    c.options.ASCIIOnly,
		ToCommonJSRef:                toCommonJSRef,
		ToESMRef:                     toESMRef,
		LegalComments:                c.options.LegalComments,
		UnsupportedFeatures:          c.options.UnsupportedJSFeatures,
		RequireOrImportMetaForSource: c.requireOrImportMetaForSource,
		MangledProps:                 c.mangledProps,
	}
	result.PrintResult = js_printer.Print(tree, c.graph.Symbols, r, printOptions)
	return
}

func (c *linkerContext) renameSymbolsInChunk(chunk *chunkInfo, filesInOrder []uint32, timer *helpers.Timer) renamer.Renamer {
	if c.options.MinifyIdentifiers {
		timer.Begin("Minify symbols")
		defer timer.End("Minify symbols")
	} else {
		timer.Begin("Rename symbols")
		defer timer.End("Rename symbols")
	}

	// Determine the reserved names (e.g. can't generate the name "if")
	timer.Begin("Compute reserved names")
	moduleScopes := make([]*js_ast.Scope, len(filesInOrder))
	for i, sourceIndex := range filesInOrder {
		moduleScopes[i] = c.graph.Files[sourceIndex].InputFile.Repr.(*graph.JSRepr).AST.ModuleScope
	}
	reservedNames := renamer.ComputeReservedNames(moduleScopes, c.graph.Symbols)

	// Node contains code that scans CommonJS modules in an attempt to statically
	// detect the  set of export names that a module will use. However, it doesn't
	// do any scope analysis so it can be fooled by local variables with the same
	// name as the CommonJS module-scope variables "exports" and "module". Avoid
	// using these names in this case even if there is not a risk of a name
	// collision because there is still a risk of node incorrectly detecting
	// something in a nested scope as an top-level export. Here's a case where
	// this happened: https://github.com/evanw/esbuild/issues/3544
	if c.options.OutputFormat == config.FormatCommonJS && c.options.Platform == config.PlatformNode {
		reservedNames["exports"] = 1
		reservedNames["module"] = 1
	}

	// These are used to implement bundling, and need to be free for use
	if c.options.Mode != config.ModePassThrough {
		reservedNames["require"] = 1
		reservedNames["Promise"] = 1
	}
	timer.End("Compute reserved names")

	// Make sure imports get a chance to be renamed too
	var sortedImportsFromOtherChunks stableRefArray
	for _, imports := range chunk.chunkRepr.(*chunkReprJS).importsFromOtherChunks {
		for _, item := range imports {
			sortedImportsFromOtherChunks = append(sortedImportsFromOtherChunks, stableRef{
				StableSourceIndex: c.graph.StableSourceIndices[item.ref.SourceIndex],
				Ref:               item.ref,
			})
		}
	}
	sort.Sort(sortedImportsFromOtherChunks)

	// Minification uses frequency analysis to give shorter names to more frequent symbols
	if c.options.MinifyIdentifiers {
		// Determine the first top-level slot (i.e. not in a nested scope)
		var firstTopLevelSlots ast.SlotCounts
		for _, sourceIndex := range filesInOrder {
			firstTopLevelSlots.UnionMax(c.graph.Files[sourceIndex].InputFile.Repr.(*graph.JSRepr).AST.NestedScopeSlotCounts)
		}
		r := renamer.NewMinifyRenamer(c.graph.Symbols, firstTopLevelSlots, reservedNames)

		// Accumulate nested symbol usage counts
		timer.Begin("Accumulate symbol counts")
		timer.Begin("Parallel phase")
		allTopLevelSymbols := make([]renamer.StableSymbolCountArray, len(filesInOrder))
		stableSourceIndices := c.graph.StableSourceIndices
		freq := ast.CharFreq{}
		waitGroup := sync.WaitGroup{}
		waitGroup.Add(len(filesInOrder))
		for i, sourceIndex := range filesInOrder {
			repr := c.graph.Files[sourceIndex].InputFile.Repr.(*graph.JSRepr)

			// Do this outside of the goroutine because it's not atomic
			if repr.AST.CharFreq != nil {
				freq.Include(repr.AST.CharFreq)
			}

			go func(topLevelSymbols *renamer.StableSymbolCountArray, repr *graph.JSRepr) {
				if repr.AST.UsesExportsRef {
					r.AccumulateSymbolCount(topLevelSymbols, repr.AST.ExportsRef, 1, stableSourceIndices)
				}
				if repr.AST.UsesModuleRef {
					r.AccumulateSymbolCount(topLevelSymbols, repr.AST.ModuleRef, 1, stableSourceIndices)
				}

				for partIndex, part := range repr.AST.Parts {
					if !repr.AST.Parts[partIndex].IsLive {
						// Skip the part if it's not in this chunk
						continue
					}

					// Accumulate symbol use counts
					r.AccumulateSymbolUseCounts(topLevelSymbols, part.SymbolUses, stableSourceIndices)

					// Make sure to also count the declaration in addition to the uses
					for _, declared := range part.DeclaredSymbols {
						r.AccumulateSymbolCount(topLevelSymbols, declared.Ref, 1, stableSourceIndices)
					}
				}

				sort.Sort(topLevelSymbols)
				waitGroup.Done()
			}(&allTopLevelSymbols[i], repr)
		}
		waitGroup.Wait()
		timer.End("Parallel phase")

		// Accumulate top-level symbol usage counts
		timer.Begin("Serial phase")
		capacity := len(sortedImportsFromOtherChunks)
		for _, array := range allTopLevelSymbols {
			capacity += len(array)
		}
		topLevelSymbols := make(renamer.StableSymbolCountArray, 0, capacity)
		for _, stable := range sortedImportsFromOtherChunks {
			r.AccumulateSymbolCount(&topLevelSymbols, stable.Ref, 1, stableSourceIndices)
		}
		for _, array := range allTopLevelSymbols {
			topLevelSymbols = append(topLevelSymbols, array...)
		}
		r.AllocateTopLevelSymbolSlots(topLevelSymbols)
		timer.End("Serial phase")
		timer.End("Accumulate symbol counts")

		// Add all of the character frequency histograms for all files in this
		// chunk together, then use it to compute the character sequence used to
		// generate minified names. This results in slightly better gzip compression
		// over assigning minified names in order (i.e. "a b c ..."). Even though
		// it's a very small win, we still do it because it's simple to do and very
		// cheap to compute.
		minifier := ast.DefaultNameMinifierJS.ShuffleByCharFreq(freq)
		timer.Begin("Assign names by frequency")
		r.AssignNamesByFrequency(&minifier)
		timer.End("Assign names by frequency")
		return r
	}

	// When we're not minifying, just append numbers to symbol names to avoid collisions
	r := renamer.NewNumberRenamer(c.graph.Symbols, reservedNames)
	nestedScopes := make(map[uint32][]*js_ast.Scope)

	timer.Begin("Add top-level symbols")
	for _, stable := range sortedImportsFromOtherChunks {
		r.AddTopLevelSymbol(stable.Ref)
	}
	for _, sourceIndex := range filesInOrder {
		repr := c.graph.Files[sourceIndex].InputFile.Repr.(*graph.JSRepr)
		var scopes []*js_ast.Scope

		// Modules wrapped in a CommonJS closure look like this:
		//
		//   // foo.js
		//   var require_foo = __commonJS((exports, module) => {
		//     exports.foo = 123;
		//   });
		//
		// The symbol "require_foo" is stored in "file.ast.WrapperRef". We want
		// to be able to minify everything inside the closure without worrying
		// about collisions with other CommonJS modules. Set up the scopes such
		// that it appears as if the file was structured this way all along. It's
		// not completely accurate (e.g. we don't set the parent of the module
		// scope to this new top-level scope) but it's good enough for the
		// renaming code.
		if repr.Meta.Wrap == graph.WrapCJS {
			r.AddTopLevelSymbol(repr.AST.WrapperRef)

			// External import statements will be hoisted outside of the CommonJS
			// wrapper if the output format supports import statements. We need to
			// add those symbols to the top-level scope to avoid causing name
			// collisions. This code special-cases only those symbols.
			if c.options.OutputFormat.KeepESMImportExportSyntax() {
				for _, part := range repr.AST.Parts {
					for _, stmt := range part.Stmts {
						switch s := stmt.Data.(type) {
						case *js_ast.SImport:
							if !repr.AST.ImportRecords[s.ImportRecordIndex].SourceIndex.IsValid() {
								r.AddTopLevelSymbol(s.NamespaceRef)
								if s.DefaultName != nil {
									r.AddTopLevelSymbol(s.DefaultName.Ref)
								}
								if s.Items != nil {
									for _, item := range *s.Items {
										r.AddTopLevelSymbol(item.Name.Ref)
									}
								}
							}

						case *js_ast.SExportStar:
							if !repr.AST.ImportRecords[s.ImportRecordIndex].SourceIndex.IsValid() {
								r.AddTopLevelSymbol(s.NamespaceRef)
							}

						case *js_ast.SExportFrom:
							if !repr.AST.ImportRecords[s.ImportRecordIndex].SourceIndex.IsValid() {
								r.AddTopLevelSymbol(s.NamespaceRef)
								for _, item := range s.Items {
									r.AddTopLevelSymbol(item.Name.Ref)
								}
							}
						}
					}
				}
			}

			nestedScopes[sourceIndex] = []*js_ast.Scope{repr.AST.ModuleScope}
			continue
		}

		// Modules wrapped in an ESM closure look like this:
		//
		//   // foo.js
		//   var foo, foo_exports = {};
		//   __export(foo_exports, {
		//     foo: () => foo
		//   });
		//   let init_foo = __esm(() => {
		//     foo = 123;
		//   });
		//
		// The symbol "init_foo" is stored in "file.ast.WrapperRef". We need to
		// minify everything inside the closure without introducing a new scope
		// since all top-level variables will be hoisted outside of the closure.
		if repr.Meta.Wrap == graph.WrapESM {
			r.AddTopLevelSymbol(repr.AST.WrapperRef)
		}

		// Rename each top-level symbol declaration in this chunk
		for partIndex, part := range repr.AST.Parts {
			if repr.AST.Parts[partIndex].IsLive {
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
	timer.End("Add top-level symbols")

	// Recursively rename symbols in child scopes now that all top-level
	// symbols have been renamed. This is done in parallel because the symbols
	// inside nested scopes are independent and can't conflict.
	timer.Begin("Assign names by scope")
	r.AssignNamesByScope(nestedScopes)
	timer.End("Assign names by scope")
	return r
}

func (c *linkerContext) generateChunkJS(chunkIndex int, chunkWaitGroup *sync.WaitGroup) {
	defer c.recoverInternalError(chunkWaitGroup, runtime.SourceIndex)

	chunk := &c.chunks[chunkIndex]

	timer := c.timer.Fork()
	if timer != nil {
		timeName := fmt.Sprintf("Generate chunk %q", path.Clean(config.TemplateToString(chunk.finalTemplate)))
		timer.Begin(timeName)
		defer c.timer.Join(timer)
		defer timer.End(timeName)
	}

	chunkRepr := chunk.chunkRepr.(*chunkReprJS)
	compileResults := make([]compileResultJS, 0, len(chunkRepr.partsInChunkInOrder))
	runtimeMembers := c.graph.Files[runtime.SourceIndex].InputFile.Repr.(*graph.JSRepr).AST.ModuleScope.Members
	toCommonJSRef := ast.FollowSymbols(c.graph.Symbols, runtimeMembers["__toCommonJS"].Ref)
	toESMRef := ast.FollowSymbols(c.graph.Symbols, runtimeMembers["__toESM"].Ref)
	runtimeRequireRef := ast.FollowSymbols(c.graph.Symbols, runtimeMembers["__require"].Ref)
	r := c.renameSymbolsInChunk(chunk, chunkRepr.filesInChunkInOrder, timer)
	dataForSourceMaps := c.dataForSourceMaps()

	// Note: This contains placeholders instead of what the placeholders are
	// substituted with. That should be fine though because this should only
	// ever be used for figuring out how many "../" to add to a relative path
	// from a chunk whose final path hasn't been calculated yet to a chunk
	// whose final path has already been calculated. That and placeholders are
	// never substituted with something containing a "/" so substitution should
	// never change the "../" count.
	chunkAbsDir := c.fs.Dir(c.fs.Join(c.options.AbsOutputDir, config.TemplateToString(chunk.finalTemplate)))

	// Generate JavaScript for each file in parallel
	timer.Begin("Print JavaScript files")
	waitGroup := sync.WaitGroup{}
	for _, partRange := range chunkRepr.partsInChunkInOrder {
		// Skip the runtime in test output
		if partRange.sourceIndex == runtime.SourceIndex && c.options.OmitRuntimeForTests {
			continue
		}

		// Create a goroutine for this file
		compileResults = append(compileResults, compileResultJS{})
		compileResult := &compileResults[len(compileResults)-1]
		waitGroup.Add(1)
		go c.generateCodeForFileInChunkJS(
			r,
			&waitGroup,
			partRange,
			toCommonJSRef,
			toESMRef,
			runtimeRequireRef,
			compileResult,
			dataForSourceMaps,
		)
	}

	// Also generate the cross-chunk binding code
	var crossChunkPrefix []byte
	var crossChunkSuffix []byte
	var jsonMetadataImports []string
	{
		// Indent the file if everything is wrapped in an IIFE
		indent := 0
		if c.options.OutputFormat == config.FormatIIFE {
			indent++
		}
		printOptions := js_printer.Options{
			Indent:            indent,
			OutputFormat:      c.options.OutputFormat,
			MinifyIdentifiers: c.options.MinifyIdentifiers,
			MinifyWhitespace:  c.options.MinifyWhitespace,
			MinifySyntax:      c.options.MinifySyntax,
			LineLimit:         c.options.LineLimit,
			NeedsMetafile:     c.options.NeedsMetafile,
		}
		crossChunkImportRecords := make([]ast.ImportRecord, len(chunk.crossChunkImports))
		for i, chunkImport := range chunk.crossChunkImports {
			crossChunkImportRecords[i] = ast.ImportRecord{
				Kind:  chunkImport.importKind,
				Path:  logger.Path{Text: c.chunks[chunkImport.chunkIndex].uniqueKey},
				Flags: ast.ShouldNotBeExternalInMetafile | ast.ContainsUniqueKey,
			}
		}
		crossChunkResult := js_printer.Print(js_ast.AST{
			ImportRecords: crossChunkImportRecords,
			Parts:         []js_ast.Part{{Stmts: chunkRepr.crossChunkPrefixStmts}},
		}, c.graph.Symbols, r, printOptions)
		crossChunkPrefix = crossChunkResult.JS
		jsonMetadataImports = crossChunkResult.JSONMetadataImports
		crossChunkSuffix = js_printer.Print(js_ast.AST{
			Parts: []js_ast.Part{{Stmts: chunkRepr.crossChunkSuffixStmts}},
		}, c.graph.Symbols, r, printOptions).JS
	}

	// Generate the exports for the entry point, if there are any
	var entryPointTail compileResultJS
	if chunk.isEntryPoint {
		entryPointTail = c.generateEntryPointTailJS(
			r,
			toCommonJSRef,
			toESMRef,
			chunk.sourceIndex,
		)
	}

	waitGroup.Wait()
	timer.End("Print JavaScript files")
	timer.Begin("Join JavaScript files")

	j := helpers.Joiner{}
	prevOffset := sourcemap.LineColumnOffset{}

	// Optionally strip whitespace
	indent := ""
	space := " "
	newline := "\n"
	if c.options.MinifyWhitespace {
		space = ""
		newline = ""
	}
	newlineBeforeComment := false
	isExecutable := false

	// Start with the hashbang if there is one. This must be done before the
	// banner because it only works if it's literally the first character.
	if chunk.isEntryPoint {
		if repr := c.graph.Files[chunk.sourceIndex].InputFile.Repr.(*graph.JSRepr); repr.AST.Hashbang != "" {
			hashbang := repr.AST.Hashbang + "\n"
			prevOffset.AdvanceString(hashbang)
			j.AddString(hashbang)
			newlineBeforeComment = true
			isExecutable = true
		}
	}

	// Then emit the banner after the hashbang. This must come before the
	// "use strict" directive below because some people use the banner to
	// emit a hashbang, which must be the first thing in the file.
	if len(c.options.JSBanner) > 0 {
		prevOffset.AdvanceString(c.options.JSBanner)
		prevOffset.AdvanceString("\n")
		j.AddString(c.options.JSBanner)
		j.AddString("\n")
		newlineBeforeComment = true
	}

	// Add the top-level directive if present (but omit "use strict" in ES
	// modules because all ES modules are automatically in strict mode)
	if chunk.isEntryPoint {
		repr := c.graph.Files[chunk.sourceIndex].InputFile.Repr.(*graph.JSRepr)
		for _, directive := range repr.AST.Directives {
			if directive != "use strict" || c.options.OutputFormat != config.FormatESModule {
				quoted := string(helpers.QuoteForJSON(directive, c.options.ASCIIOnly)) + ";" + newline
				prevOffset.AdvanceString(quoted)
				j.AddString(quoted)
				newlineBeforeComment = true
			}
		}
	}

	// Optionally wrap with an IIFE
	if c.options.OutputFormat == config.FormatIIFE {
		var text string
		indent = "  "
		if len(c.options.GlobalName) > 0 {
			text = c.generateGlobalNamePrefix()
		}
		if c.options.UnsupportedJSFeatures.Has(compat.Arrow) {
			text += "(function()" + space + "{" + newline
		} else {
			text += "(()" + space + "=>" + space + "{" + newline
		}
		prevOffset.AdvanceString(text)
		j.AddString(text)
		newlineBeforeComment = false
	}

	// Put the cross-chunk prefix inside the IIFE
	if len(crossChunkPrefix) > 0 {
		newlineBeforeComment = true
		prevOffset.AdvanceBytes(crossChunkPrefix)
		j.AddBytes(crossChunkPrefix)
	}

	// Start the metadata
	jMeta := helpers.Joiner{}
	if c.options.NeedsMetafile {
		// Print imports
		isFirstMeta := true
		jMeta.AddString("{\n      \"imports\": [")
		for _, json := range jsonMetadataImports {
			if isFirstMeta {
				isFirstMeta = false
			} else {
				jMeta.AddString(",")
			}
			jMeta.AddString(json)
		}
		for _, compileResult := range compileResults {
			for _, json := range compileResult.JSONMetadataImports {
				if isFirstMeta {
					isFirstMeta = false
				} else {
					jMeta.AddString(",")
				}
				jMeta.AddString(json)
			}
		}
		if !isFirstMeta {
			jMeta.AddString("\n      ")
		}

		// Print exports
		jMeta.AddString("],\n      \"exports\": [")
		var aliases []string
		if c.options.OutputFormat.KeepESMImportExportSyntax() {
			if chunk.isEntryPoint {
				if fileRepr := c.graph.Files[chunk.sourceIndex].InputFile.Repr.(*graph.JSRepr); fileRepr.Meta.Wrap == graph.WrapCJS {
					aliases = []string{"default"}
				} else {
					resolvedExports := fileRepr.Meta.ResolvedExports
					aliases = make([]string, 0, len(resolvedExports))
					for alias := range resolvedExports {
						aliases = append(aliases, alias)
					}
				}
			} else {
				aliases = make([]string, 0, len(chunkRepr.exportsToOtherChunks))
				for _, alias := range chunkRepr.exportsToOtherChunks {
					aliases = append(aliases, alias)
				}
			}
		}
		isFirstMeta = true
		sort.Strings(aliases) // Sort for determinism
		for _, alias := range aliases {
			if isFirstMeta {
				isFirstMeta = false
			} else {
				jMeta.AddString(",")
			}
			jMeta.AddString(fmt.Sprintf("\n        %s",
				helpers.QuoteForJSON(alias, c.options.ASCIIOnly)))
		}
		if !isFirstMeta {
			jMeta.AddString("\n      ")
		}
		jMeta.AddString("],\n")
		if chunk.isEntryPoint {
			entryPoint := c.graph.Files[chunk.sourceIndex].InputFile.Source.PrettyPath
			jMeta.AddString(fmt.Sprintf("      \"entryPoint\": %s,\n", helpers.QuoteForJSON(entryPoint, c.options.ASCIIOnly)))
		}
		if chunkRepr.hasCSSChunk {
			jMeta.AddString(fmt.Sprintf("      \"cssBundle\": %s,\n", helpers.QuoteForJSON(c.chunks[chunkRepr.cssChunkIndex].uniqueKey, c.options.ASCIIOnly)))
		}
		jMeta.AddString("      \"inputs\": {")
	}

	// Concatenate the generated JavaScript chunks together
	var compileResultsForSourceMap []compileResultForSourceMap
	var legalCommentList []legalCommentEntry
	var metaOrder []uint32
	var metaBytes map[uint32][][]byte
	prevFileNameComment := uint32(0)
	if c.options.NeedsMetafile {
		metaOrder = make([]uint32, 0, len(compileResults))
		metaBytes = make(map[uint32][][]byte, len(compileResults))
	}
	for _, compileResult := range compileResults {
		if len(compileResult.ExtractedLegalComments) > 0 {
			legalCommentList = append(legalCommentList, legalCommentEntry{
				sourceIndex: compileResult.sourceIndex,
				comments:    compileResult.ExtractedLegalComments,
			})
		}

		// Add a comment with the file path before the file contents
		if c.options.Mode == config.ModeBundle && !c.options.MinifyWhitespace &&
			prevFileNameComment != compileResult.sourceIndex && len(compileResult.JS) > 0 {
			if newlineBeforeComment {
				prevOffset.AdvanceString("\n")
				j.AddString("\n")
			}

			path := c.graph.Files[compileResult.sourceIndex].InputFile.Source.PrettyPath

			// Make sure newlines in the path can't cause a syntax error. This does
			// not minimize allocations because it's expected that this case never
			// comes up in practice.
			path = strings.ReplaceAll(path, "\r", "\\r")
			path = strings.ReplaceAll(path, "\n", "\\n")
			path = strings.ReplaceAll(path, "\u2028", "\\u2028")
			path = strings.ReplaceAll(path, "\u2029", "\\u2029")

			text := fmt.Sprintf("%s// %s\n", indent, path)
			prevOffset.AdvanceString(text)
			j.AddString(text)
			prevFileNameComment = compileResult.sourceIndex
		}

		// Don't include the runtime in source maps
		if c.graph.Files[compileResult.sourceIndex].InputFile.OmitFromSourceMapsAndMetafile {
			prevOffset.AdvanceString(string(compileResult.JS))
			j.AddBytes(compileResult.JS)
		} else {
			// Save the offset to the start of the stored JavaScript
			compileResult.generatedOffset = prevOffset
			j.AddBytes(compileResult.JS)

			// Ignore empty source map chunks
			if compileResult.SourceMapChunk.ShouldIgnore {
				prevOffset.AdvanceBytes(compileResult.JS)

				// Include a null entry in the source map
				if len(compileResult.JS) > 0 && c.options.SourceMap != config.SourceMapNone {
					if n := len(compileResultsForSourceMap); n > 0 && !compileResultsForSourceMap[n-1].isNullEntry {
						compileResultsForSourceMap = append(compileResultsForSourceMap, compileResultForSourceMap{
							sourceIndex: compileResult.sourceIndex,
							isNullEntry: true,
						})
					}
				}
			} else {
				prevOffset = sourcemap.LineColumnOffset{}

				// Include this file in the source map
				if c.options.SourceMap != config.SourceMapNone {
					compileResultsForSourceMap = append(compileResultsForSourceMap, compileResultForSourceMap{
						sourceMapChunk:  compileResult.SourceMapChunk,
						generatedOffset: compileResult.generatedOffset,
						sourceIndex:     compileResult.sourceIndex,
					})
				}
			}

			// Include this file in the metadata
			if c.options.NeedsMetafile {
				// Accumulate file sizes since a given file may be split into multiple parts
				bytes, ok := metaBytes[compileResult.sourceIndex]
				if !ok {
					metaOrder = append(metaOrder, compileResult.sourceIndex)
				}
				metaBytes[compileResult.sourceIndex] = append(bytes, compileResult.JS)
			}
		}

		// Put a newline before the next file path comment
		if len(compileResult.JS) > 0 {
			newlineBeforeComment = true
		}
	}

	// Stick the entry point tail at the end of the file. Deliberately don't
	// include any source mapping information for this because it's automatically
	// generated and doesn't correspond to a location in the input file.
	j.AddBytes(entryPointTail.JS)

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
	j.EnsureNewlineAtEnd()
	slashTag := "/script"
	if c.options.UnsupportedJSFeatures.Has(compat.InlineScript) {
		slashTag = ""
	}
	c.maybeAppendLegalComments(c.options.LegalComments, legalCommentList, chunk, &j, slashTag)

	if len(c.options.JSFooter) > 0 {
		j.AddString(c.options.JSFooter)
		j.AddString("\n")
	}

	// The JavaScript contents are done now that the source map comment is in
	chunk.intermediateOutput = c.breakJoinerIntoPieces(j)
	timer.End("Join JavaScript files")

	if c.options.SourceMap != config.SourceMapNone {
		timer.Begin("Generate source map")
		canHaveShifts := chunk.intermediateOutput.pieces != nil
		chunk.outputSourceMap = c.generateSourceMapForChunk(compileResultsForSourceMap, chunkAbsDir, dataForSourceMaps, canHaveShifts)
		timer.End("Generate source map")
	}

	// End the metadata lazily. The final output size is not known until the
	// final import paths are substituted into the output pieces generated below.
	if c.options.NeedsMetafile {
		pieces := make([][]intermediateOutput, len(metaOrder))
		for i, sourceIndex := range metaOrder {
			slices := metaBytes[sourceIndex]
			outputs := make([]intermediateOutput, len(slices))
			for j, slice := range slices {
				outputs[j] = c.breakOutputIntoPieces(slice)
			}
			pieces[i] = outputs
		}
		chunk.jsonMetadataChunkCallback = func(finalOutputSize int) helpers.Joiner {
			finalRelDir := c.fs.Dir(chunk.finalRelPath)
			for i, sourceIndex := range metaOrder {
				if i > 0 {
					jMeta.AddString(",")
				}
				count := 0
				for _, output := range pieces[i] {
					count += c.accurateFinalByteCount(output, finalRelDir)
				}
				jMeta.AddString(fmt.Sprintf("\n        %s: {\n          \"bytesInOutput\": %d\n        %s}",
					helpers.QuoteForJSON(c.graph.Files[sourceIndex].InputFile.Source.PrettyPath, c.options.ASCIIOnly),
					count, c.generateExtraDataForFileJS(sourceIndex)))
			}
			if len(metaOrder) > 0 {
				jMeta.AddString("\n      ")
			}
			jMeta.AddString(fmt.Sprintf("},\n      \"bytes\": %d\n    }", finalOutputSize))
			return jMeta
		}
	}

	c.generateIsolatedHashInParallel(chunk)
	chunk.isExecutable = isExecutable
	chunkWaitGroup.Done()
}

func (c *linkerContext) generateGlobalNamePrefix() string {
	var text string
	globalName := c.options.GlobalName
	prefix, globalName := globalName[0], globalName[1:]
	space := " "
	join := ";\n"

	if c.options.MinifyWhitespace {
		space = ""
		join = ";"
	}

	// Assume the "this" and "import.meta" objects always exist
	isExistingObject := prefix == "this"
	if prefix == "import" && len(globalName) > 0 && globalName[0] == "meta" {
		prefix, globalName = "import.meta", globalName[1:]
		isExistingObject = true
	}

	// Use "||=" to make the code more compact when it's supported
	if len(globalName) > 0 && !c.options.UnsupportedJSFeatures.Has(compat.LogicalAssignment) {
		if isExistingObject {
			// Keep the prefix as it is
		} else if js_printer.CanEscapeIdentifier(prefix, c.options.UnsupportedJSFeatures, c.options.ASCIIOnly) {
			if c.options.ASCIIOnly {
				prefix = string(js_printer.QuoteIdentifier(nil, prefix, c.options.UnsupportedJSFeatures))
			}
			text = fmt.Sprintf("var %s%s", prefix, join)
		} else {
			prefix = fmt.Sprintf("this[%s]", helpers.QuoteForJSON(prefix, c.options.ASCIIOnly))
		}
		for _, name := range globalName {
			var dotOrIndex string
			if js_printer.CanEscapeIdentifier(name, c.options.UnsupportedJSFeatures, c.options.ASCIIOnly) {
				if c.options.ASCIIOnly {
					name = string(js_printer.QuoteIdentifier(nil, name, c.options.UnsupportedJSFeatures))
				}
				dotOrIndex = fmt.Sprintf(".%s", name)
			} else {
				dotOrIndex = fmt.Sprintf("[%s]", helpers.QuoteForJSON(name, c.options.ASCIIOnly))
			}
			if isExistingObject {
				prefix = fmt.Sprintf("%s%s", prefix, dotOrIndex)
				isExistingObject = false
			} else {
				prefix = fmt.Sprintf("(%s%s||=%s{})%s", prefix, space, space, dotOrIndex)
			}
		}
		return fmt.Sprintf("%s%s%s=%s", text, prefix, space, space)
	}

	if isExistingObject {
		text = fmt.Sprintf("%s%s=%s", prefix, space, space)
	} else if js_printer.CanEscapeIdentifier(prefix, c.options.UnsupportedJSFeatures, c.options.ASCIIOnly) {
		if c.options.ASCIIOnly {
			prefix = string(js_printer.QuoteIdentifier(nil, prefix, c.options.UnsupportedJSFeatures))
		}
		text = fmt.Sprintf("var %s%s=%s", prefix, space, space)
	} else {
		prefix = fmt.Sprintf("this[%s]", helpers.QuoteForJSON(prefix, c.options.ASCIIOnly))
		text = fmt.Sprintf("%s%s=%s", prefix, space, space)
	}

	for _, name := range globalName {
		oldPrefix := prefix
		if js_printer.CanEscapeIdentifier(name, c.options.UnsupportedJSFeatures, c.options.ASCIIOnly) {
			if c.options.ASCIIOnly {
				name = string(js_printer.QuoteIdentifier(nil, name, c.options.UnsupportedJSFeatures))
			}
			prefix = fmt.Sprintf("%s.%s", prefix, name)
		} else {
			prefix = fmt.Sprintf("%s[%s]", prefix, helpers.QuoteForJSON(name, c.options.ASCIIOnly))
		}
		text += fmt.Sprintf("%s%s||%s{}%s%s%s=%s", oldPrefix, space, space, join, prefix, space, space)
	}

	return text
}

type compileResultCSS struct {
	css_printer.PrintResult

	// This is the line and column offset since the previous CSS string
	// or the start of the file if this is the first CSS string.
	generatedOffset sourcemap.LineColumnOffset

	// The source index can be invalid for short snippets that aren't necessarily
	// tied to any one file and/or that don't really need source mappings. The
	// source index is really only valid for the compile result that contains the
	// main contents of a file, which we try to only ever write out once.
	sourceIndex ast.Index32
	hasCharset  bool
}

func (c *linkerContext) generateChunkCSS(chunkIndex int, chunkWaitGroup *sync.WaitGroup) {
	defer c.recoverInternalError(chunkWaitGroup, runtime.SourceIndex)

	chunk := &c.chunks[chunkIndex]

	timer := c.timer.Fork()
	if timer != nil {
		timeName := fmt.Sprintf("Generate chunk %q", path.Clean(config.TemplateToString(chunk.finalTemplate)))
		timer.Begin(timeName)
		defer c.timer.Join(timer)
		defer timer.End(timeName)
	}

	chunkRepr := chunk.chunkRepr.(*chunkReprCSS)
	compileResults := make([]compileResultCSS, len(chunkRepr.importsInChunkInOrder))
	dataForSourceMaps := c.dataForSourceMaps()

	// Note: This contains placeholders instead of what the placeholders are
	// substituted with. That should be fine though because this should only
	// ever be used for figuring out how many "../" to add to a relative path
	// from a chunk whose final path hasn't been calculated yet to a chunk
	// whose final path has already been calculated. That and placeholders are
	// never substituted with something containing a "/" so substitution should
	// never change the "../" count.
	chunkAbsDir := c.fs.Dir(c.fs.Join(c.options.AbsOutputDir, config.TemplateToString(chunk.finalTemplate)))

	// Remove duplicate rules across files. This must be done in serial, not
	// in parallel, and must be done from the last rule to the first rule.
	timer.Begin("Prepare CSS ASTs")
	asts := make([]css_ast.AST, len(chunkRepr.importsInChunkInOrder))
	var remover css_parser.DuplicateRuleRemover
	if c.options.MinifySyntax {
		remover = css_parser.MakeDuplicateRuleMangler(c.graph.Symbols)
	}
	for i := len(chunkRepr.importsInChunkInOrder) - 1; i >= 0; i-- {
		entry := chunkRepr.importsInChunkInOrder[i]
		switch entry.kind {
		case cssImportLayers:
			var rules []css_ast.Rule
			if len(entry.layers) > 0 {
				rules = append(rules, css_ast.Rule{Data: &css_ast.RAtLayer{Names: entry.layers}})
			}
			rules, importRecords := wrapRulesWithConditions(rules, nil, entry.conditions, entry.conditionImportRecords)
			asts[i] = css_ast.AST{Rules: rules, ImportRecords: importRecords}

		case cssImportExternalPath:
			var conditions *css_ast.ImportConditions
			if len(entry.conditions) > 0 {
				conditions = &entry.conditions[0]

				// Handling a chain of nested conditions is complicated. We can't
				// necessarily join them together because a) there may be multiple
				// layer names and b) layer names are only supposed to be inserted
				// into the layer order if the parent conditions are applied.
				//
				// Instead we handle them by preserving the "@import" nesting using
				// imports of data URL stylesheets. This may seem strange but I think
				// this is the only way to do this in CSS.
				for i := len(entry.conditions) - 1; i > 0; i-- {
					astImport := css_ast.AST{
						Rules: []css_ast.Rule{{Data: &css_ast.RAtImport{
							ImportRecordIndex: uint32(len(entry.conditionImportRecords)),
							ImportConditions:  &entry.conditions[i],
						}}},
						ImportRecords: append(entry.conditionImportRecords, ast.ImportRecord{
							Kind: ast.ImportAt,
							Path: entry.externalPath,
						}),
					}
					astResult := css_printer.Print(astImport, c.graph.Symbols, css_printer.Options{
						MinifyWhitespace: c.options.MinifyWhitespace,
						ASCIIOnly:        c.options.ASCIIOnly,
					})
					entry.externalPath = logger.Path{Text: helpers.EncodeStringAsShortestDataURL("text/css", string(bytes.TrimSpace(astResult.CSS)))}
				}
			}
			asts[i] = css_ast.AST{
				ImportRecords: append(append([]ast.ImportRecord{}, entry.conditionImportRecords...), ast.ImportRecord{
					Kind: ast.ImportAt,
					Path: entry.externalPath,
				}),
				Rules: []css_ast.Rule{{Data: &css_ast.RAtImport{
					ImportRecordIndex: uint32(len(entry.conditionImportRecords)),
					ImportConditions:  conditions,
				}}},
			}

		case cssImportSourceIndex:
			file := &c.graph.Files[entry.sourceIndex]
			ast := file.InputFile.Repr.(*graph.CSSRepr).AST

			// Filter out "@charset", "@import", and leading "@layer" rules
			rules := make([]css_ast.Rule, 0, len(ast.Rules))
			didFindAtImport := false
			didFindAtLayer := false
			for _, rule := range ast.Rules {
				switch rule.Data.(type) {
				case *css_ast.RAtCharset:
					compileResults[i].hasCharset = true
					continue
				case *css_ast.RAtLayer:
					didFindAtLayer = true
				case *css_ast.RAtImport:
					if !didFindAtImport {
						didFindAtImport = true
						if didFindAtLayer {
							// Filter out the pre-import layers once we see the first
							// "@import". These layers are special-cased by the linker
							// and already appear as a separate entry in import order.
							end := 0
							for _, rule := range rules {
								if _, ok := rule.Data.(*css_ast.RAtLayer); !ok {
									rules[end] = rule
									end++
								}
							}
							rules = rules[:end]
						}
					}
					continue
				}
				rules = append(rules, rule)
			}

			rules, ast.ImportRecords = wrapRulesWithConditions(rules, ast.ImportRecords, entry.conditions, entry.conditionImportRecords)

			// Remove top-level duplicate rules across files
			if c.options.MinifySyntax {
				rules = remover.RemoveDuplicateRulesInPlace(entry.sourceIndex, rules, ast.ImportRecords)
			}

			ast.Rules = rules
			asts[i] = ast
		}
	}
	timer.End("Prepare CSS ASTs")

	// Generate CSS for each file in parallel
	timer.Begin("Print CSS files")
	waitGroup := sync.WaitGroup{}
	for i, entry := range chunkRepr.importsInChunkInOrder {
		// Create a goroutine for this file
		waitGroup.Add(1)
		go func(i int, entry cssImportOrder, compileResult *compileResultCSS) {
			cssOptions := css_printer.Options{
				MinifyWhitespace:    c.options.MinifyWhitespace,
				LineLimit:           c.options.LineLimit,
				ASCIIOnly:           c.options.ASCIIOnly,
				LegalComments:       c.options.LegalComments,
				SourceMap:           c.options.SourceMap,
				UnsupportedFeatures: c.options.UnsupportedCSSFeatures,
				NeedsMetafile:       c.options.NeedsMetafile,
				LocalNames:          c.mangledProps,
			}

			if entry.kind == cssImportSourceIndex {
				defer c.recoverInternalError(&waitGroup, entry.sourceIndex)
				file := &c.graph.Files[entry.sourceIndex]

				// Only generate a source map if needed
				if file.InputFile.Loader.CanHaveSourceMap() && c.options.SourceMap != config.SourceMapNone {
					cssOptions.AddSourceMappings = true
					cssOptions.InputSourceMap = file.InputFile.InputSourceMap
					cssOptions.LineOffsetTables = dataForSourceMaps[entry.sourceIndex].LineOffsetTables
				}

				cssOptions.InputSourceIndex = entry.sourceIndex
				compileResult.sourceIndex = ast.MakeIndex32(entry.sourceIndex)
			}

			compileResult.PrintResult = css_printer.Print(asts[i], c.graph.Symbols, cssOptions)
			waitGroup.Done()
		}(i, entry, &compileResults[i])
	}

	waitGroup.Wait()
	timer.End("Print CSS files")
	timer.Begin("Join CSS files")
	j := helpers.Joiner{}
	prevOffset := sourcemap.LineColumnOffset{}
	newlineBeforeComment := false

	if len(c.options.CSSBanner) > 0 {
		prevOffset.AdvanceString(c.options.CSSBanner)
		j.AddString(c.options.CSSBanner)
		prevOffset.AdvanceString("\n")
		j.AddString("\n")
	}

	// Generate any prefix rules now
	var jsonMetadataImports []string
	{
		tree := css_ast.AST{}

		// "@charset" is the only thing that comes before "@import"
		for _, compileResult := range compileResults {
			if compileResult.hasCharset {
				tree.Rules = append(tree.Rules, css_ast.Rule{Data: &css_ast.RAtCharset{Encoding: "UTF-8"}})
				break
			}
		}

		if len(tree.Rules) > 0 {
			result := css_printer.Print(tree, c.graph.Symbols, css_printer.Options{
				MinifyWhitespace: c.options.MinifyWhitespace,
				LineLimit:        c.options.LineLimit,
				ASCIIOnly:        c.options.ASCIIOnly,
				NeedsMetafile:    c.options.NeedsMetafile,
			})
			jsonMetadataImports = result.JSONMetadataImports
			if len(result.CSS) > 0 {
				prevOffset.AdvanceBytes(result.CSS)
				j.AddBytes(result.CSS)
				newlineBeforeComment = true
			}
		}
	}

	// Start the metadata
	jMeta := helpers.Joiner{}
	if c.options.NeedsMetafile {
		isFirstMeta := true
		jMeta.AddString("{\n      \"imports\": [")
		for _, json := range jsonMetadataImports {
			if isFirstMeta {
				isFirstMeta = false
			} else {
				jMeta.AddString(",")
			}
			jMeta.AddString(json)
		}
		for _, compileResult := range compileResults {
			for _, json := range compileResult.JSONMetadataImports {
				if isFirstMeta {
					isFirstMeta = false
				} else {
					jMeta.AddString(",")
				}
				jMeta.AddString(json)
			}
		}
		if !isFirstMeta {
			jMeta.AddString("\n      ")
		}
		if chunk.isEntryPoint {
			file := &c.graph.Files[chunk.sourceIndex]

			// Do not generate "entryPoint" for CSS files that are the result of
			// importing CSS into JavaScript. We want this to be a 1:1 relationship
			// and there is already an output file for the JavaScript entry point.
			if _, ok := file.InputFile.Repr.(*graph.CSSRepr); ok {
				jMeta.AddString(fmt.Sprintf("],\n      \"entryPoint\": %s,\n      \"inputs\": {",
					helpers.QuoteForJSON(file.InputFile.Source.PrettyPath, c.options.ASCIIOnly)))
			} else {
				jMeta.AddString("],\n      \"inputs\": {")
			}
		} else {
			jMeta.AddString("],\n      \"inputs\": {")
		}
	}

	// Concatenate the generated CSS chunks together
	var compileResultsForSourceMap []compileResultForSourceMap
	var legalCommentList []legalCommentEntry
	for _, compileResult := range compileResults {
		if len(compileResult.ExtractedLegalComments) > 0 && compileResult.sourceIndex.IsValid() {
			legalCommentList = append(legalCommentList, legalCommentEntry{
				sourceIndex: compileResult.sourceIndex.GetIndex(),
				comments:    compileResult.ExtractedLegalComments,
			})
		}

		if c.options.Mode == config.ModeBundle && !c.options.MinifyWhitespace && compileResult.sourceIndex.IsValid() {
			var newline string
			if newlineBeforeComment {
				newline = "\n"
			}
			comment := fmt.Sprintf("%s/* %s */\n", newline, c.graph.Files[compileResult.sourceIndex.GetIndex()].InputFile.Source.PrettyPath)
			prevOffset.AdvanceString(comment)
			j.AddString(comment)
		}
		if len(compileResult.CSS) > 0 {
			newlineBeforeComment = true
		}

		// Save the offset to the start of the stored JavaScript
		compileResult.generatedOffset = prevOffset
		j.AddBytes(compileResult.CSS)

		// Ignore empty source map chunks
		if compileResult.SourceMapChunk.ShouldIgnore {
			prevOffset.AdvanceBytes(compileResult.CSS)

			// Include a null entry in the source map
			if len(compileResult.CSS) > 0 && c.options.SourceMap != config.SourceMapNone && compileResult.sourceIndex.IsValid() {
				if n := len(compileResultsForSourceMap); n > 0 && !compileResultsForSourceMap[n-1].isNullEntry {
					compileResultsForSourceMap = append(compileResultsForSourceMap, compileResultForSourceMap{
						sourceIndex: compileResult.sourceIndex.GetIndex(),
						isNullEntry: true,
					})
				}
			}
		} else {
			prevOffset = sourcemap.LineColumnOffset{}

			// Include this file in the source map
			if c.options.SourceMap != config.SourceMapNone && compileResult.sourceIndex.IsValid() {
				compileResultsForSourceMap = append(compileResultsForSourceMap, compileResultForSourceMap{
					sourceMapChunk:  compileResult.SourceMapChunk,
					generatedOffset: compileResult.generatedOffset,
					sourceIndex:     compileResult.sourceIndex.GetIndex(),
				})
			}
		}
	}

	// Make sure the file ends with a newline
	j.EnsureNewlineAtEnd()
	slashTag := "/style"
	if c.options.UnsupportedCSSFeatures.Has(compat.InlineStyle) {
		slashTag = ""
	}
	c.maybeAppendLegalComments(c.options.LegalComments, legalCommentList, chunk, &j, slashTag)

	if len(c.options.CSSFooter) > 0 {
		j.AddString(c.options.CSSFooter)
		j.AddString("\n")
	}

	// The CSS contents are done now that the source map comment is in
	chunk.intermediateOutput = c.breakJoinerIntoPieces(j)
	timer.End("Join CSS files")

	if c.options.SourceMap != config.SourceMapNone {
		timer.Begin("Generate source map")
		canHaveShifts := chunk.intermediateOutput.pieces != nil
		chunk.outputSourceMap = c.generateSourceMapForChunk(compileResultsForSourceMap, chunkAbsDir, dataForSourceMaps, canHaveShifts)
		timer.End("Generate source map")
	}

	// End the metadata lazily. The final output size is not known until the
	// final import paths are substituted into the output pieces generated below.
	if c.options.NeedsMetafile {
		pieces := make([]intermediateOutput, len(compileResults))
		for i, compileResult := range compileResults {
			pieces[i] = c.breakOutputIntoPieces(compileResult.CSS)
		}
		chunk.jsonMetadataChunkCallback = func(finalOutputSize int) helpers.Joiner {
			finalRelDir := c.fs.Dir(chunk.finalRelPath)
			isFirst := true
			for i, compileResult := range compileResults {
				if !compileResult.sourceIndex.IsValid() {
					continue
				}
				if isFirst {
					isFirst = false
				} else {
					jMeta.AddString(",")
				}
				jMeta.AddString(fmt.Sprintf("\n        %s: {\n          \"bytesInOutput\": %d\n        }",
					helpers.QuoteForJSON(c.graph.Files[compileResult.sourceIndex.GetIndex()].InputFile.Source.PrettyPath, c.options.ASCIIOnly),
					c.accurateFinalByteCount(pieces[i], finalRelDir)))
			}
			if len(compileResults) > 0 {
				jMeta.AddString("\n      ")
			}
			jMeta.AddString(fmt.Sprintf("},\n      \"bytes\": %d\n    }", finalOutputSize))
			return jMeta
		}
	}

	c.generateIsolatedHashInParallel(chunk)
	chunkWaitGroup.Done()
}

func wrapRulesWithConditions(
	rules []css_ast.Rule, importRecords []ast.ImportRecord,
	conditions []css_ast.ImportConditions, conditionImportRecords []ast.ImportRecord,
) ([]css_ast.Rule, []ast.ImportRecord) {
	for i := len(conditions) - 1; i >= 0; i-- {
		item := conditions[i]

		// Generate "@layer" wrappers. Note that empty "@layer" rules still have
		// a side effect (they set the layer order) so they cannot be removed.
		for _, t := range item.Layers {
			if len(rules) == 0 {
				if t.Children == nil {
					// Omit an empty "@layer {}" entirely
					continue
				} else {
					// Generate "@layer foo;" instead of "@layer foo {}"
					rules = nil
				}
			}
			var prelude []css_ast.Token
			if t.Children != nil {
				prelude = *t.Children
			}
			prelude, importRecords = css_ast.CloneTokensWithImportRecords(prelude, conditionImportRecords, nil, importRecords)
			rules = []css_ast.Rule{{Data: &css_ast.RKnownAt{
				AtToken: "layer",
				Prelude: prelude,
				Rules:   rules,
			}}}
		}

		// Generate "@supports" wrappers. This is not done if the rule block is
		// empty because empty "@supports" rules have no effect.
		if len(rules) > 0 {
			for _, t := range item.Supports {
				t.Kind = css_lexer.TOpenParen
				t.Text = "("
				var prelude []css_ast.Token
				prelude, importRecords = css_ast.CloneTokensWithImportRecords([]css_ast.Token{t}, conditionImportRecords, nil, importRecords)
				rules = []css_ast.Rule{{Data: &css_ast.RKnownAt{
					AtToken: "supports",
					Prelude: prelude,
					Rules:   rules,
				}}}
			}
		}

		// Generate "@media" wrappers. This is not done if the rule block is
		// empty because empty "@media" rules have no effect.
		if len(rules) > 0 && len(item.Media) > 0 {
			var prelude []css_ast.Token
			prelude, importRecords = css_ast.CloneTokensWithImportRecords(item.Media, conditionImportRecords, nil, importRecords)
			rules = []css_ast.Rule{{Data: &css_ast.RKnownAt{
				AtToken: "media",
				Prelude: prelude,
				Rules:   rules,
			}}}
		}
	}

	return rules, importRecords
}

type legalCommentEntry struct {
	sourceIndex uint32
	comments    []string
}

// Add all unique legal comments to the end of the file. These are
// deduplicated because some projects have thousands of files with the same
// comment. The comment must be preserved in the output for legal reasons but
// at the same time we want to generate a small bundle when minifying.
func (c *linkerContext) maybeAppendLegalComments(
	legalComments config.LegalComments,
	legalCommentList []legalCommentEntry,
	chunk *chunkInfo,
	j *helpers.Joiner,
	slashTag string,
) {
	switch legalComments {
	case config.LegalCommentsNone, config.LegalCommentsInline:
		return
	}

	type thirdPartyEntry struct {
		packagePath string
		comments    []string
	}

	var uniqueFirstPartyComments []string
	var thirdPartyComments []thirdPartyEntry
	hasFirstPartyComment := make(map[string]struct{})

	for _, entry := range legalCommentList {
		source := c.graph.Files[entry.sourceIndex].InputFile.Source
		packagePath := ""

		// Try to extract a package name from the source path. If we can find a
		// "node_modules" path component in the path, then assume this is a legal
		// comment in third-party code and that everything after "node_modules" is
		// the package name and subpath. If we can't, then assume this is a legal
		// comment in first-party code.
		//
		// The rationale for this behavior: If we just include third-party comments
		// as-is and the third-party comments don't say what package they're from
		// (which isn't uncommon), then it'll look like that comment applies to
		// all code in the file which is very wrong. So we need to somehow say
		// where the comment comes from. But we don't want to say where every
		// comment comes from because people probably won't appreciate this for
		// first-party comments. And we don't want to include the whole path to
		// each third-part module because a) that could contain information about
		// the local machine that people don't want in their bundle and b) that
		// could differ depending on unimportant details like the package manager
		// used to install the packages (npm vs. pnpm vs. yarn).
		if source.KeyPath.Namespace != "dataurl" {
			path := source.KeyPath.Text
			previous := len(path)
			for previous > 0 {
				slash := strings.LastIndexAny(path[:previous], "\\/")
				component := path[slash+1 : previous]
				if component == "node_modules" {
					if previous < len(path) {
						packagePath = strings.ReplaceAll(path[previous+1:], "\\", "/")
					}
					break
				}
				previous = slash
			}
		}

		if packagePath != "" {
			thirdPartyComments = append(thirdPartyComments, thirdPartyEntry{
				packagePath: packagePath,
				comments:    entry.comments,
			})
		} else {
			for _, comment := range entry.comments {
				if _, ok := hasFirstPartyComment[comment]; !ok {
					hasFirstPartyComment[comment] = struct{}{}
					uniqueFirstPartyComments = append(uniqueFirstPartyComments, comment)
				}
			}
		}
	}

	switch legalComments {
	case config.LegalCommentsEndOfFile:
		for _, comment := range uniqueFirstPartyComments {
			j.AddString(helpers.EscapeClosingTag(comment, slashTag))
			j.AddString("\n")
		}

		if len(thirdPartyComments) > 0 {
			j.AddString("/*! Bundled license information:\n")
			for _, entry := range thirdPartyComments {
				j.AddString(fmt.Sprintf("\n%s:\n", helpers.EscapeClosingTag(entry.packagePath, slashTag)))
				for _, comment := range entry.comments {
					comment = helpers.EscapeClosingTag(comment, slashTag)
					if strings.HasPrefix(comment, "//") {
						j.AddString(fmt.Sprintf("  (*%s *)\n", comment[2:]))
					} else if strings.HasPrefix(comment, "/*") && strings.HasSuffix(comment, "*/") {
						j.AddString(fmt.Sprintf("  (%s)\n", strings.ReplaceAll(comment[1:len(comment)-1], "\n", "\n  ")))
					}
				}
			}
			j.AddString("*/\n")
		}

	case config.LegalCommentsLinkedWithComment, config.LegalCommentsExternalWithoutComment:
		var jComments helpers.Joiner

		for _, comment := range uniqueFirstPartyComments {
			jComments.AddString(comment)
			jComments.AddString("\n")
		}

		if len(thirdPartyComments) > 0 {
			if len(uniqueFirstPartyComments) > 0 {
				jComments.AddString("\n")
			}
			jComments.AddString("Bundled license information:\n")
			for _, entry := range thirdPartyComments {
				jComments.AddString(fmt.Sprintf("\n%s:\n", entry.packagePath))
				for _, comment := range entry.comments {
					jComments.AddString(fmt.Sprintf("  %s\n", strings.ReplaceAll(comment, "\n", "\n  ")))
				}
			}
		}

		chunk.externalLegalComments = jComments.Done()
	}
}

func (c *linkerContext) appendIsolatedHashesForImportedChunks(
	hash hash.Hash,
	chunkIndex uint32,
	visited []uint32,
	visitedKey uint32,
) {
	// Only visit each chunk at most once. This is important because there may be
	// cycles in the chunk import graph. If there's a cycle, we want to include
	// the hash of every chunk involved in the cycle (along with all of their
	// dependencies). This depth-first traversal will naturally do that.
	if visited[chunkIndex] == visitedKey {
		return
	}
	visited[chunkIndex] = visitedKey
	chunk := &c.chunks[chunkIndex]

	// Visit the other chunks that this chunk imports before visiting this chunk
	for _, chunkImport := range chunk.crossChunkImports {
		c.appendIsolatedHashesForImportedChunks(hash, chunkImport.chunkIndex, visited, visitedKey)
	}

	// Mix in hashes for referenced asset paths (i.e. the "file" loader)
	for _, piece := range chunk.intermediateOutput.pieces {
		if piece.kind == outputPieceAssetIndex {
			file := c.graph.Files[piece.index]
			if len(file.InputFile.AdditionalFiles) != 1 {
				panic("Internal error")
			}
			relPath, _ := c.fs.Rel(c.options.AbsOutputDir, file.InputFile.AdditionalFiles[0].AbsPath)

			// Make sure to always use forward slashes, even on Windows
			relPath = strings.ReplaceAll(relPath, "\\", "/")

			// Mix in the hash for the relative path, which ends up as a JS string
			hashWriteLengthPrefixed(hash, []byte(relPath))
		}
	}

	// Mix in the hash for this chunk
	hash.Write(chunk.waitForIsolatedHash())
}

func (c *linkerContext) breakJoinerIntoPieces(j helpers.Joiner) intermediateOutput {
	// Optimization: If there can be no substitutions, just reuse the initial
	// joiner that was used when generating the intermediate chunk output
	// instead of creating another one and copying the whole file into it.
	if !j.Contains(c.uniqueKeyPrefix, c.uniqueKeyPrefixBytes) {
		return intermediateOutput{joiner: j}
	}
	return c.breakOutputIntoPieces(j.Done())
}

func (c *linkerContext) breakOutputIntoPieces(output []byte) intermediateOutput {
	var pieces []outputPiece
	prefix := c.uniqueKeyPrefixBytes
	for {
		// Scan for the next piece boundary
		boundary := bytes.Index(output, prefix)

		// Try to parse the piece boundary
		var kind outputPieceIndexKind
		var index uint32
		if boundary != -1 {
			if start := boundary + len(prefix); start+9 > len(output) {
				boundary = -1
			} else {
				switch output[start] {
				case 'A':
					kind = outputPieceAssetIndex
				case 'C':
					kind = outputPieceChunkIndex
				}
				for j := 1; j < 9; j++ {
					c := output[start+j]
					if c < '0' || c > '9' {
						boundary = -1
						break
					}
					index = index*10 + uint32(c) - '0'
				}
			}
		}

		// Validate the boundary
		switch kind {
		case outputPieceAssetIndex:
			if index >= uint32(len(c.graph.Files)) {
				boundary = -1
			}

		case outputPieceChunkIndex:
			if index >= uint32(len(c.chunks)) {
				boundary = -1
			}

		default:
			boundary = -1
		}

		// If we're at the end, generate one final piece
		if boundary == -1 {
			pieces = append(pieces, outputPiece{
				data: output,
			})
			break
		}

		// Otherwise, generate an interior piece and continue
		pieces = append(pieces, outputPiece{
			data:  output[:boundary],
			index: index,
			kind:  kind,
		})
		output = output[boundary+len(prefix)+9:]
	}
	return intermediateOutput{pieces: pieces}
}

func (c *linkerContext) generateIsolatedHashInParallel(chunk *chunkInfo) {
	// Compute the hash in parallel. This is a speedup when it turns out the hash
	// isn't needed (well, as long as there are threads to spare).
	channel := make(chan []byte, 1)
	chunk.waitForIsolatedHash = func() []byte {
		data := <-channel
		channel <- data
		return data
	}
	go c.generateIsolatedHash(chunk, channel)
}

func (c *linkerContext) generateIsolatedHash(chunk *chunkInfo, channel chan []byte) {
	hash := xxhash.New()

	// Mix the file names and part ranges of all of the files in this chunk into
	// the hash. Objects that appear identical but that live in separate files or
	// that live in separate parts in the same file must not be merged. This only
	// needs to be done for JavaScript files, not CSS files.
	if chunkRepr, ok := chunk.chunkRepr.(*chunkReprJS); ok {
		for _, partRange := range chunkRepr.partsInChunkInOrder {
			var filePath string
			file := &c.graph.Files[partRange.sourceIndex]
			if file.InputFile.Source.KeyPath.Namespace == "file" {
				// Use the pretty path as the file name since it should be platform-
				// independent (relative paths and the "/" path separator)
				filePath = file.InputFile.Source.PrettyPath
			} else {
				// If this isn't in the "file" namespace, just use the full path text
				// verbatim. This could be a source of cross-platform differences if
				// plugins are storing platform-specific information in here, but then
				// that problem isn't caused by esbuild itself.
				filePath = file.InputFile.Source.KeyPath.Text
			}

			// Include the path namespace in the hash
			hashWriteLengthPrefixed(hash, []byte(file.InputFile.Source.KeyPath.Namespace))

			// Then include the file path
			hashWriteLengthPrefixed(hash, []byte(filePath))

			// Also write the part range. These numbers are deterministic and allocated
			// per-file so this should be a well-behaved base for a hash.
			hashWriteUint32(hash, partRange.partIndexBegin)
			hashWriteUint32(hash, partRange.partIndexEnd)
		}
	}

	// Hash the output path template as part of the content hash because we want
	// any import to be considered different if the import's output path has changed.
	for _, part := range chunk.finalTemplate {
		hashWriteLengthPrefixed(hash, []byte(part.Data))
	}

	// Also hash the public path. If provided, this is used whenever files
	// reference each other such as cross-chunk imports, asset file references,
	// and source map comments. We always include the hash in all chunks instead
	// of trying to figure out which chunks will include the public path for
	// simplicity and for robustness to code changes in the future.
	if c.options.PublicPath != "" {
		hashWriteLengthPrefixed(hash, []byte(c.options.PublicPath))
	}

	// Include the generated output content in the hash. This excludes the
	// randomly-generated import paths (the unique keys) and only includes the
	// data in the spans between them.
	if chunk.intermediateOutput.pieces != nil {
		for _, piece := range chunk.intermediateOutput.pieces {
			hashWriteLengthPrefixed(hash, piece.data)
		}
	} else {
		bytes := chunk.intermediateOutput.joiner.Done()
		hashWriteLengthPrefixed(hash, bytes)
	}

	// Also include the source map data in the hash. The source map is named the
	// same name as the chunk name for ease of discovery. So we want the hash to
	// change if the source map data changes even if the chunk data doesn't change.
	// Otherwise the output path for the source map wouldn't change and the source
	// map wouldn't end up being updated.
	//
	// Note that this means the contents of all input files are included in the
	// hash because of "sourcesContent", so changing a comment in an input file
	// can now change the hash of the output file. This only happens when you
	// have source maps enabled (and "sourcesContent", which is on by default).
	//
	// The generated positions in the mappings here are in the output content
	// *before* the final paths have been substituted. This may seem weird.
	// However, I think this shouldn't cause issues because a) the unique key
	// values are all always the same length so the offsets are deterministic
	// and b) the final paths will be folded into the final hash later.
	hashWriteLengthPrefixed(hash, chunk.outputSourceMap.Prefix)
	hashWriteLengthPrefixed(hash, chunk.outputSourceMap.Mappings)
	hashWriteLengthPrefixed(hash, chunk.outputSourceMap.Suffix)

	// Store the hash so far. All other chunks that import this chunk will mix
	// this hash into their final hash to ensure that the import path changes
	// if this chunk (or any dependencies of this chunk) is changed.
	channel <- hash.Sum(nil)
}

func hashWriteUint32(hash hash.Hash, value uint32) {
	var lengthBytes [4]byte
	binary.LittleEndian.PutUint32(lengthBytes[:], value)
	hash.Write(lengthBytes[:])
}

// Hash the data in length-prefixed form because boundary locations are
// important. We don't want "a" + "bc" to hash the same as "ab" + "c".
func hashWriteLengthPrefixed(hash hash.Hash, bytes []byte) {
	hashWriteUint32(hash, uint32(len(bytes)))
	hash.Write(bytes)
}

// Marking a symbol as unbound prevents it from being renamed or minified.
// This is only used when a module is compiled independently. We use a very
// different way of handling exports and renaming/minifying when bundling.
func (c *linkerContext) preventExportsFromBeingRenamed(sourceIndex uint32) {
	repr, ok := c.graph.Files[sourceIndex].InputFile.Repr.(*graph.JSRepr)
	if !ok {
		return
	}
	hasImportOrExport := false

	for _, part := range repr.AST.Parts {
		for _, stmt := range part.Stmts {
			switch s := stmt.Data.(type) {
			case *js_ast.SImport:
				// Ignore imports from internal (i.e. non-external) code. Since this
				// function is only called when we're not bundling, these imports are
				// all for files that were generated automatically and aren't part of
				// the original source code (e.g. the runtime or an injected file).
				// We shouldn't consider the file a module if the only ESM imports or
				// exports are automatically generated ones.
				if repr.AST.ImportRecords[s.ImportRecordIndex].SourceIndex.IsValid() {
					continue
				}

				hasImportOrExport = true

			case *js_ast.SLocal:
				if s.IsExport {
					js_ast.ForEachIdentifierBindingInDecls(s.Decls, func(loc logger.Loc, b *js_ast.BIdentifier) {
						c.graph.Symbols.Get(b.Ref).Flags |= ast.MustNotBeRenamed
					})
					hasImportOrExport = true
				}

			case *js_ast.SFunction:
				if s.IsExport {
					c.graph.Symbols.Get(s.Fn.Name.Ref).Kind = ast.SymbolUnbound
					hasImportOrExport = true
				}

			case *js_ast.SClass:
				if s.IsExport {
					c.graph.Symbols.Get(s.Class.Name.Ref).Kind = ast.SymbolUnbound
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
		for _, member := range repr.AST.ModuleScope.Members {
			c.graph.Symbols.Get(member.Ref).Flags |= ast.MustNotBeRenamed
		}
	}
}

type compileResultForSourceMap struct {
	sourceMapChunk  sourcemap.Chunk
	generatedOffset sourcemap.LineColumnOffset
	sourceIndex     uint32
	isNullEntry     bool
}

func (c *linkerContext) generateSourceMapForChunk(
	results []compileResultForSourceMap,
	chunkAbsDir string,
	dataForSourceMaps []bundler.DataForSourceMap,
	canHaveShifts bool,
) (pieces sourcemap.SourceMapPieces) {
	j := helpers.Joiner{}
	j.AddString("{\n  \"version\": 3")

	// Only write out the sources for a given source index once
	sourceIndexToSourcesIndex := make(map[uint32]int)

	// Generate the "sources" and "sourcesContent" arrays
	type item struct {
		source         string
		quotedContents []byte
	}
	items := make([]item, 0, len(results))
	nextSourcesIndex := 0
	for _, result := range results {
		if result.isNullEntry {
			continue
		}
		if _, ok := sourceIndexToSourcesIndex[result.sourceIndex]; ok {
			continue
		}
		sourceIndexToSourcesIndex[result.sourceIndex] = nextSourcesIndex
		file := &c.graph.Files[result.sourceIndex]

		// Simple case: no nested source map
		if file.InputFile.InputSourceMap == nil {
			var quotedContents []byte
			if !c.options.ExcludeSourcesContent {
				quotedContents = dataForSourceMaps[result.sourceIndex].QuotedContents[0]
			}
			source := file.InputFile.Source.KeyPath.Text
			if file.InputFile.Source.KeyPath.Namespace == "file" {
				// Serialize the file path as a "file://" URL, since source maps encode
				// sources as URLs. While we could output absolute "file://" URLs, it
				// will be turned into a relative path when it's written out below for
				// better readability and to be independent of build directory.
				source = helpers.FileURLFromFilePath(source).String()
			} else {
				// If the path for this file isn't in the "file" namespace, then write
				// out something arbitrary instead. Source maps encode sources as URLs
				// but plugins are allowed to put almost anything in the "namespace"
				// and "path" fields, so we don't attempt to control whether this forms
				// a valid URL or not.
				//
				// The approach used here is to join the namespace with the path text
				// using a ":" character. It's important to include the namespace
				// because esbuild considers paths with different namespaces to have
				// separate identities. And using a ":" means that the path is more
				// likely to form a valid URL in the source map.
				//
				// For example, you could imagine a plugin that uses the "https"
				// namespace and path text like "//example.com/foo.js", which would
				// then be joined into the URL "https://example.com/foo.js" here.
				//
				// Note that this logic is currently mostly the same as the pretty-
				// printed paths that esbuild shows to humans in error messages.
				// However, this code has been forked below as these source map URLs
				// are intended for code instead of humans, and we don't want the
				// changes for humans to unintentionally break code that uses them.
				//
				// See https://github.com/evanw/esbuild/issues/4078 for more info.
				if ns := file.InputFile.Source.KeyPath.Namespace; ns != "" {
					source = fmt.Sprintf("%s:%s", ns, source)
				}
				source += file.InputFile.Source.KeyPath.IgnoredSuffix
			}
			items = append(items, item{
				source:         source,
				quotedContents: quotedContents,
			})
			nextSourcesIndex++
			continue
		}

		// Complex case: nested source map
		sm := file.InputFile.InputSourceMap
		for i, source := range sm.Sources {
			var quotedContents []byte
			if !c.options.ExcludeSourcesContent {
				quotedContents = dataForSourceMaps[result.sourceIndex].QuotedContents[i]
			}
			items = append(items, item{
				source:         source,
				quotedContents: quotedContents,
			})
		}
		nextSourcesIndex += len(sm.Sources)
	}

	// Write the sources
	j.AddString(",\n  \"sources\": [")
	for i, item := range items {
		if i != 0 {
			j.AddString(", ")
		}

		// Modify the absolute path to the original file to be relative to the
		// directory that will contain the output file for this chunk
		if sourceURL, err := url.Parse(item.source); err == nil && helpers.IsFileURL(sourceURL) {
			sourcePath := helpers.FilePathFromFileURL(c.fs, sourceURL)
			if relPath, ok := c.fs.Rel(chunkAbsDir, sourcePath); ok {
				// Make sure to always use forward slashes, even on Windows
				relativeURL := url.URL{Path: strings.ReplaceAll(relPath, "\\", "/")}
				item.source = relativeURL.String()

				// Replace certain percent encodings for better readability
				item.source = strings.ReplaceAll(item.source, "%20", " ")
			}
		}

		j.AddBytes(helpers.QuoteForJSON(item.source, c.options.ASCIIOnly))
	}
	j.AddString("]")

	if c.options.SourceRoot != "" {
		j.AddString(",\n  \"sourceRoot\": ")
		j.AddBytes(helpers.QuoteForJSON(c.options.SourceRoot, c.options.ASCIIOnly))
	}

	// Write the sourcesContent
	if !c.options.ExcludeSourcesContent {
		j.AddString(",\n  \"sourcesContent\": [")
		for i, item := range items {
			if i != 0 {
				j.AddString(", ")
			}
			j.AddBytes(item.quotedContents)
		}
		j.AddString("]")
	}

	j.AddString(",\n  \"mappings\": \"")

	// Write the mappings
	mappingsStart := j.Length()
	prevEndState := sourcemap.SourceMapState{}
	prevColumnOffset := 0
	totalQuotedNameLen := 0
	for _, result := range results {
		chunk := result.sourceMapChunk
		offset := result.generatedOffset
		sourcesIndex, ok := sourceIndexToSourcesIndex[result.sourceIndex]
		if !ok {
			// If there's no sourcesIndex, then every mapping for this result's
			// sourceIndex were null mappings. We still need to emit the null
			// mapping, but its source index won't matter.
			sourcesIndex = 0
			if !result.isNullEntry {
				panic("Internal error")
			}
		}

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
		startState := sourcemap.SourceMapState{
			SourceIndex:     sourcesIndex,
			GeneratedLine:   offset.Lines,
			GeneratedColumn: offset.Columns,
			OriginalName:    totalQuotedNameLen,
		}
		if offset.Lines == 0 {
			startState.GeneratedColumn += prevColumnOffset
		}

		if result.isNullEntry {
			// Emit a "null" mapping
			chunk.Buffer.Data = []byte("A")
			sourcemap.AppendSourceMapChunk(&j, prevEndState, startState, chunk.Buffer)

			// Only the generated position was advanced
			prevEndState.GeneratedLine = startState.GeneratedLine
			prevEndState.GeneratedColumn = startState.GeneratedColumn
		} else {
			// Append the precomputed source map chunk
			sourcemap.AppendSourceMapChunk(&j, prevEndState, startState, chunk.Buffer)

			// Generate the relative offset to start from next time
			prevOriginalName := prevEndState.OriginalName
			prevEndState = chunk.EndState
			prevEndState.SourceIndex += sourcesIndex
			if chunk.Buffer.FirstNameOffset.IsValid() {
				prevEndState.OriginalName += totalQuotedNameLen
			} else {
				// It's possible for a chunk to have mappings but for none of those
				// mappings to have an associated name. The name is optional and is
				// omitted when the mapping is for a non-name token or if the final
				// and original names are the same. In that case we need to restore
				// the previous original name end state since it wasn't modified after
				// all. If we don't do this, then files after this will adjust their
				// name offsets assuming that the previous generated mapping has this
				// file's offset, which is wrong.
				prevEndState.OriginalName = prevOriginalName
			}
			prevColumnOffset = chunk.FinalGeneratedColumn
			totalQuotedNameLen += len(chunk.QuotedNames)
		}

		// If this was all one line, include the column offset from the start
		if prevEndState.GeneratedLine == 0 {
			prevEndState.GeneratedColumn += startState.GeneratedColumn
			prevColumnOffset += startState.GeneratedColumn
		}
	}
	mappingsEnd := j.Length()

	// Write the names
	isFirstName := true
	j.AddString("\",\n  \"names\": [")
	for _, result := range results {
		for _, quotedName := range result.sourceMapChunk.QuotedNames {
			if isFirstName {
				isFirstName = false
			} else {
				j.AddString(", ")
			}
			j.AddBytes(quotedName)
		}
	}
	j.AddString("]")

	// Finish the source map
	j.AddString("\n}\n")
	bytes := j.Done()

	if !canHaveShifts {
		// If there cannot be any shifts, then we can avoid doing extra work later
		// on by preserving the source map as a single memory allocation throughout
		// the pipeline. That way we won't need to reallocate it.
		pieces.Prefix = bytes
	} else {
		// Otherwise if there can be shifts, then we need to split this into several
		// slices so that the shifts in the mappings array can be processed. This is
		// more expensive because everything will need to be recombined into a new
		// memory allocation at the end.
		pieces.Prefix = bytes[:mappingsStart]
		pieces.Mappings = bytes[mappingsStart:mappingsEnd]
		pieces.Suffix = bytes[mappingsEnd:]
	}
	return
}

// Recover from a panic by logging it as an internal error instead of crashing
func (c *linkerContext) recoverInternalError(waitGroup *sync.WaitGroup, sourceIndex uint32) {
	if r := recover(); r != nil {
		text := fmt.Sprintf("panic: %v", r)
		if sourceIndex != runtime.SourceIndex {
			text = fmt.Sprintf("%s (while printing %q)", text, c.graph.Files[sourceIndex].InputFile.Source.PrettyPath)
		}
		c.log.AddErrorWithNotes(nil, logger.Range{}, text,
			[]logger.MsgData{{Text: helpers.PrettyPrintedStack()}})
		waitGroup.Done()
	}
}

func joinWithPublicPath(publicPath string, relPath string) string {
	if strings.HasPrefix(relPath, "./") {
		relPath = relPath[2:]

		// Strip any amount of further no-op slashes (i.e. ".///././/x/y" => "x/y")
		for {
			if strings.HasPrefix(relPath, "/") {
				relPath = relPath[1:]
			} else if strings.HasPrefix(relPath, "./") {
				relPath = relPath[2:]
			} else {
				break
			}
		}
	}

	// Use a relative path if there is no public path
	if publicPath == "" {
		publicPath = "."
	}

	// Join with a slash
	slash := "/"
	if strings.HasSuffix(publicPath, "/") {
		slash = ""
	}
	return fmt.Sprintf("%s%s%s", publicPath, slash, relPath)
}
