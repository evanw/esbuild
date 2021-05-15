package bundler

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"hash"
	"math/rand"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/css_ast"
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
	res     resolver.Resolver
	graph   graph.LinkerGraph

	// This helps avoid an infinite loop when matching imports to exports
	cycleDetector []importTracker

	// We may need to refer to the CommonJS "module" symbol for exports
	unboundModuleRef js_ast.Ref

	// This represents the parallel computation of source map related data.
	// Calling this will block until the computation is done. The resulting value
	// is shared between threads and must be treated as immutable.
	dataForSourceMaps func() []dataForSourceMap

	// The unique key prefix is a random string that is unique to every linking
	// operation. It is used as a prefix for the unique keys assigned to every
	// chunk. These unique keys are used to identify each chunk before the final
	// output paths have been computed.
	uniqueKeyPrefix      string
	uniqueKeyPrefixBytes []byte // This is just "uniqueKeyPrefix" in byte form
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
	filesInChunkInOrder   []uint32
	partsInChunkInOrder   []partRange
	entryBits             helpers.BitSet

	// This information is only useful if "isEntryPoint" is true
	isEntryPoint  bool
	sourceIndex   uint32 // An index into "c.sources"
	entryPointBit uint   // An index into "c.graph.EntryPoints"

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

	// When this chunk is initially generated in isolation, the output pieces
	// will contain slices of the output with the unique keys of other chunks
	// omitted.
	outputPieces []outputPiece

	// This contains the hash for just this chunk without including information
	// from the hashes of other chunks. Later on in the linking process, the
	// final hash for this chunk will be constructed by merging the isolated
	// hashes of all transitive dependencies of this chunk. This is separated
	// into two phases like this to handle cycles in the chunk import graph.
	waitForIsolatedHash func() []byte

	// Other fields relating to the output file for this chunk
	jsonMetadataChunkCallback func(finalOutputSize int) []byte
	outputSourceMap           sourcemap.SourceMapPieces
	isExecutable              bool
}

type chunkImport struct {
	chunkIndex uint32
	importKind ast.ImportKind
}

// This is a chunk of source code followed by a reference to another chunk. For
// example, the file "@import 'CHUNK0001'; body { color: black; }" would be
// represented by two pieces, one with the data "@import '" and another with the
// data "'; body { color: black; }". The first would have the chunk index 1 and
// the second would have an invalid chunk index.
type outputPiece struct {
	data []byte

	// Note: This may be invalid. For example, the chunk may not contain any
	// imports, in which case there is one piece with data and no chunk index.
	chunkIndex ast.Index32
}

type chunkRepr interface{ isChunk() }

func (*chunkReprJS) isChunk()  {}
func (*chunkReprCSS) isChunk() {}

type chunkReprJS struct {
	// For code splitting
	crossChunkPrefixStmts  []js_ast.Stmt
	crossChunkSuffixStmts  []js_ast.Stmt
	exportsToOtherChunks   map[js_ast.Ref]string
	importsFromOtherChunks map[uint32]crossChunkImportItemArray
}

type chunkReprCSS struct {
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

func link(
	options *config.Options,
	timer *helpers.Timer,
	log logger.Log,
	fs fs.FS,
	res resolver.Resolver,
	inputFiles []graph.InputFile,
	entryPoints []graph.EntryPoint,
	reachableFiles []uint32,
	dataForSourceMaps func() []dataForSourceMap,
) []graph.OutputFile {
	timer.Begin("Link")
	defer timer.End("Link")

	log = wrappedLog(log)

	timer.Begin("Clone linker graph")
	c := linkerContext{
		options:           options,
		timer:             timer,
		log:               log,
		fs:                fs,
		res:               res,
		dataForSourceMaps: dataForSourceMaps,
		graph: graph.CloneLinkerGraph(
			inputFiles,
			reachableFiles,
			entryPoints,
			options.CodeSplitting,
		),
	}
	timer.End("Clone linker graph")

	for _, entryPoint := range entryPoints {
		if repr, ok := c.graph.Files[entryPoint.SourceIndex].InputFile.Repr.(*graph.JSRepr); ok {
			// Loaders default to CommonJS when they are the entry point and the output
			// format is not ESM-compatible since that avoids generating the ESM-to-CJS
			// machinery.
			if repr.AST.HasLazyExport && (c.options.Mode == config.ModePassThrough ||
				(c.options.Mode == config.ModeConvertFormat && !c.options.OutputFormat.KeepES6ImportExportSyntax())) {
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
		}
	}

	// Allocate a new unbound symbol called "module" in case we need it later
	if c.options.OutputFormat == config.FormatCommonJS {
		c.unboundModuleRef = c.graph.GenerateNewSymbol(runtime.SourceIndex, js_ast.SymbolUnbound, "module")
	} else {
		c.unboundModuleRef = js_ast.InvalidRef
	}

	if !c.generateUniqueKeyPrefix() {
		return nil
	}
	c.scanImportsAndExports()

	// Stop now if there were errors
	if c.log.HasErrors() {
		return []graph.OutputFile{}
	}

	c.treeShakingAndCodeSplitting()

	if c.options.Mode == config.ModePassThrough {
		for _, entryPoint := range c.graph.EntryPoints() {
			c.preventExportsFromBeingRenamed(entryPoint.SourceIndex)
		}
	}

	chunks := c.computeChunks()
	c.computeCrossChunkDependencies(chunks)

	// Make sure calls to "js_ast.FollowSymbols()" in parallel goroutines after this
	// won't hit concurrent map mutation hazards
	js_ast.FollowAllSymbols(c.graph.Symbols)

	return c.generateChunksInParallel(chunks)
}

func (c *linkerContext) generateUniqueKeyPrefix() bool {
	var data [12]byte
	rand.Seed(time.Now().UnixNano())
	if _, err := rand.Read(data[:]); err != nil {
		c.log.AddError(nil, logger.Loc{}, fmt.Sprintf("Failed to read from randomness source: %s", err.Error()))
		return false
	}

	// This is 16 bytes and shouldn't generate escape characters when put into strings
	c.uniqueKeyPrefix = base64.URLEncoding.EncodeToString(data[:])
	c.uniqueKeyPrefixBytes = []byte(c.uniqueKeyPrefix)
	return true
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
func (c *linkerContext) enforceNoCyclicChunkImports(chunks []chunkInfo) {
	var validate func(int, []int)
	validate = func(chunkIndex int, path []int) {
		for _, otherChunkIndex := range path {
			if chunkIndex == otherChunkIndex {
				c.log.AddError(nil, logger.Loc{}, "Internal error: generated chunks contain a circular import")
				return
			}
		}
		path = append(path, chunkIndex)
		for _, chunkImport := range chunks[chunkIndex].crossChunkImports {
			// Ignore cycles caused by dynamic "import()" expressions. These are fine
			// because they don't necessarily cause initialization order issues and
			// they don't indicate a bug in our chunk generation algorithm. They arise
			// normally in real code (e.g. two files that import each other).
			if chunkImport.importKind != ast.ImportDynamic {
				validate(int(chunkImport.chunkIndex), path)
			}
		}
	}
	path := make([]int, 0, len(chunks))
	for i := range chunks {
		validate(i, path)
	}
}

func (c *linkerContext) generateChunksInParallel(chunks []chunkInfo) []graph.OutputFile {
	c.timer.Begin("Generate chunks")
	defer c.timer.End("Generate chunks")

	// Generate each chunk on a separate goroutine
	generateWaitGroup := sync.WaitGroup{}
	generateWaitGroup.Add(len(chunks))
	for chunkIndex := range chunks {
		switch chunks[chunkIndex].chunkRepr.(type) {
		case *chunkReprJS:
			go c.generateChunkJS(chunks, chunkIndex, &generateWaitGroup)
		case *chunkReprCSS:
			go c.generateChunkCSS(chunks, chunkIndex, &generateWaitGroup)
		}
	}
	c.enforceNoCyclicChunkImports(chunks)
	generateWaitGroup.Wait()

	// Compute the final hashes of each chunk. This can technically be done in
	// parallel but it probably doesn't matter so much because we're not hashing
	// that much data.
	visited := make([]uint32, len(chunks))
	var finalBytes []byte
	for chunkIndex := range chunks {
		chunk := &chunks[chunkIndex]
		var hashSubstitution *string

		// Only wait for the hash if necessary
		if config.HasPlaceholder(chunk.finalTemplate, config.HashPlaceholder) {
			// Compute the final hash using the isolated hashes of the dependencies
			hash := xxhash.New()
			appendIsolatedHashesForImportedChunks(hash, chunks, uint32(chunkIndex), visited, ^uint32(chunkIndex))
			finalBytes = hash.Sum(finalBytes[:0])
			finalString := hashForFileName(finalBytes)
			hashSubstitution = &finalString
		}

		// Render the last remaining placeholder in the template
		chunk.finalRelPath = config.TemplateToString(config.SubstituteTemplate(chunk.finalTemplate, config.PathPlaceholders{
			Hash: hashSubstitution,
		}))
	}

	// Generate the final output files by joining file pieces together
	c.timer.Begin("Generate final output files")
	var resultsWaitGroup sync.WaitGroup
	results := make([][]graph.OutputFile, len(chunks))
	resultsWaitGroup.Add(len(chunks))
	for chunkIndex, chunk := range chunks {
		go func(chunkIndex int, chunk chunkInfo) {
			var outputFiles []graph.OutputFile

			// Each file may optionally contain additional files to be copied to the
			// output directory. This is used by the "file" loader.
			for _, sourceIndex := range chunk.filesInChunkInOrder {
				outputFiles = append(outputFiles, c.graph.Files[sourceIndex].InputFile.AdditionalFiles...)
			}

			// Path substitution for the chunk itself
			finalRelDir := c.fs.Dir(chunk.finalRelPath)
			outputContentsJoiner, outputSourceMapShifts := c.substituteFinalPaths(chunks, chunk.outputPieces, func(finalRelPathForImport string) string {
				return c.pathBetweenChunks(finalRelDir, finalRelPathForImport)
			})

			// Generate the optional legal comments file for this chunk
			if chunk.externalLegalComments != nil {
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
			if c.options.SourceMap != config.SourceMapNone && chunk.outputSourceMap.Suffix != nil {
				outputSourceMap := chunk.outputSourceMap.Finalize(outputSourceMapShifts)
				finalRelPathForSourceMap := chunk.finalRelPath + ".map"

				// Potentially write a trailing source map comment
				switch c.options.SourceMap {
				case config.SourceMapLinkedWithComment:
					importPath := c.pathBetweenChunks(finalRelDir, finalRelPathForSourceMap)
					importPath = strings.TrimPrefix(importPath, "./")
					outputContentsJoiner.EnsureNewlineAtEnd()
					outputContentsJoiner.AddString("//# sourceMappingURL=")
					outputContentsJoiner.AddString(importPath)
					outputContentsJoiner.AddString("\n")

				case config.SourceMapInline, config.SourceMapInlineAndExternal:
					outputContentsJoiner.EnsureNewlineAtEnd()
					outputContentsJoiner.AddString("//# sourceMappingURL=data:application/json;base64,")
					outputContentsJoiner.AddString(base64.StdEncoding.EncodeToString(outputSourceMap))
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
				jsonMetadataChunkPieces := c.breakOutputIntoPieces(chunk.jsonMetadataChunkCallback(len(outputContents)), uint32(len(chunks)))
				jsonMetadataChunkBytes, _ := c.substituteFinalPaths(chunks, jsonMetadataChunkPieces, func(finalRelPathForImport string) string {
					return c.res.PrettyPath(logger.Path{Text: c.fs.Join(c.options.AbsOutputDir, finalRelPathForImport), Namespace: "file"})
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
	outputFilesLen := 0
	for _, result := range results {
		outputFilesLen += len(result)
	}
	outputFiles := make([]graph.OutputFile, 0, outputFilesLen)
	for _, result := range results {
		outputFiles = append(outputFiles, result...)
	}
	return outputFiles
}

// Given a set of output pieces (i.e. a buffer already divided into the spans
// between import paths), substitute the final import paths in and then join
// everything into a single byte buffer.
func (c *linkerContext) substituteFinalPaths(
	chunks []chunkInfo,
	pieces []outputPiece,
	modifyPath func(string) string,
) (j helpers.Joiner, shifts []sourcemap.SourceMapShift) {
	var shift sourcemap.SourceMapShift
	shifts = make([]sourcemap.SourceMapShift, 0, len(pieces))
	shifts = append(shifts, shift)

	for _, piece := range pieces {
		var dataOffset sourcemap.LineColumnOffset
		j.AddBytes(piece.data)
		dataOffset.AdvanceBytes(piece.data)
		shift.Before.Add(dataOffset)
		shift.After.Add(dataOffset)

		if piece.chunkIndex.IsValid() {
			chunk := chunks[piece.chunkIndex.GetIndex()]
			importPath := modifyPath(chunk.finalRelPath)
			j.AddString(importPath)
			shift.Before.AdvanceString(chunk.uniqueKey)
			shift.After.AdvanceString(importPath)
			shifts = append(shifts, shift)
		}
	}

	return
}

func (c *linkerContext) pathBetweenChunks(fromRelDir string, toRelPath string) string {
	// Join with the public path if it has been configured
	if c.options.PublicPath != "" {
		return joinWithPublicPath(c.options.PublicPath, toRelPath)
	}

	// Otherwise, return a relative path
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

// Returns the path of this file relative to "outbase", which is then ready to
// be joined with the absolute output directory path. The directory and name
// components are returned separately for convenience.
//
// This makes sure to have the directory end in a slash so that it can be
// substituted into a path template without necessarily having a "/" after it.
// Extra slashes should get cleaned up automatically when we join it with the
// output directory.
func (c *linkerContext) pathRelativeToOutbase(
	sourceIndex uint32,
	entryPointBit uint,
	stdExt string,
	avoidIndex bool,
) (relDir string, baseName string, baseExt string) {
	file := &c.graph.Files[sourceIndex]
	relDir = "/"
	baseExt = stdExt

	// If the output path was configured explicitly, use it verbatim
	if c.options.AbsOutputFile != "" {
		baseName = c.fs.Base(c.options.AbsOutputFile)

		// Strip off the extension
		ext := c.fs.Ext(baseName)
		baseName = baseName[:len(baseName)-len(ext)]

		// Use the extension from the explicit output file path. However, don't do
		// that if this is a CSS chunk but the entry point file is not CSS. In that
		// case use the standard extension. This happens when importing CSS into JS.
		if _, ok := file.InputFile.Repr.(*graph.CSSRepr); ok || stdExt != c.options.OutputExtensionCSS {
			baseExt = ext
		}
		return
	}

	absPath := file.InputFile.Source.KeyPath.Text
	isCustomOutputPath := false

	if outPath := c.graph.EntryPoints()[entryPointBit].OutputPath; outPath != "" {
		// Use the configured output path if present
		absPath = outPath
		if !c.fs.IsAbs(absPath) {
			absPath = c.fs.Join(c.options.AbsOutputBase, absPath)
		}
		isCustomOutputPath = true
	} else if file.InputFile.Source.KeyPath.Namespace != "file" {
		// Come up with a path for virtual paths (i.e. non-file-system paths)
		dir, base, _ := logger.PlatformIndependentPathDirBaseExt(absPath)
		if avoidIndex && base == "index" {
			_, base, _ = logger.PlatformIndependentPathDirBaseExt(dir)
		}
		baseName = sanitizeFilePathForVirtualModulePath(base)
		return
	} else {
		// Heuristic: If the file is named something like "index.js", then use
		// the name of the parent directory instead. This helps avoid the
		// situation where many chunks are named "index" because of people
		// dynamically-importing npm packages that make use of node's implicit
		// "index" file name feature.
		if avoidIndex {
			base := c.fs.Base(absPath)
			base = base[:len(base)-len(c.fs.Ext(base))]
			if base == "index" {
				absPath = c.fs.Dir(absPath)
			}
		}
	}

	// Try to get a relative path to the base directory
	relPath, ok := c.fs.Rel(c.options.AbsOutputBase, absPath)
	if !ok {
		// This can fail in some situations such as on different drives on
		// Windows. In that case we just use the file name.
		baseName = c.fs.Base(absPath)
	} else {
		// Now we finally have a relative path
		relDir = c.fs.Dir(relPath) + "/"
		baseName = c.fs.Base(relPath)

		// Use platform-independent slashes
		relDir = strings.ReplaceAll(relDir, "\\", "/")

		// Replace leading "../" so we don't try to write outside of the output
		// directory. This normally can't happen because "AbsOutputBase" is
		// automatically computed to contain all entry point files, but it can
		// happen if someone sets it manually via the "outbase" API option.
		//
		// Note that we can't just strip any leading "../" because that could
		// cause two separate entry point paths to collide. For example, there
		// could be both "src/index.js" and "../src/index.js" as entry points.
		dotDotCount := 0
		for strings.HasPrefix(relDir[dotDotCount*3:], "../") {
			dotDotCount++
		}
		if dotDotCount > 0 {
			// The use of "_.._" here is somewhat arbitrary but it is unlikely to
			// collide with a folder named by a human and it works on Windows
			// (Windows doesn't like names that end with a "."). And not starting
			// with a "." means that it will not be hidden on Unix.
			relDir = strings.Repeat("_.._/", dotDotCount) + relDir[dotDotCount*3:]
		}
		relDir = "/" + relDir
	}

	// Strip the file extension if the output path is an input file
	if !isCustomOutputPath {
		ext := c.fs.Ext(baseName)
		baseName = baseName[:len(baseName)-len(ext)]
	}
	return
}

func (c *linkerContext) computeCrossChunkDependencies(chunks []chunkInfo) {
	c.timer.Begin("Compute cross-chunk dependencies")
	defer c.timer.End("Compute cross-chunk dependencies")

	jsChunks := 0
	for _, chunk := range chunks {
		if _, ok := chunk.chunkRepr.(*chunkReprJS); ok {
			jsChunks++
		}
	}
	if jsChunks < 2 {
		// No need to compute cross-chunk dependencies if there can't be any
		return
	}

	type chunkMeta struct {
		imports        map[js_ast.Ref]bool
		exports        map[js_ast.Ref]bool
		dynamicImports map[int]bool
	}

	chunkMetas := make([]chunkMeta, len(chunks))

	// For each chunk, see what symbols it uses from other chunks. Do this in
	// parallel because it's the most expensive part of this function.
	waitGroup := sync.WaitGroup{}
	waitGroup.Add(len(chunks))
	for chunkIndex, chunk := range chunks {
		go func(chunkIndex int, chunk chunkInfo) {
			chunkMeta := &chunkMetas[chunkIndex]
			imports := make(map[js_ast.Ref]bool)
			chunkMeta.imports = imports
			chunkMeta.exports = make(map[js_ast.Ref]bool)

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
								record.Path.Text = chunks[otherChunkIndex].uniqueKey
								record.SourceIndex = ast.Index32{}

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
							if symbol.Kind == js_ast.SymbolUnbound {
								continue
							}

							// Ignore symbols that are going to be replaced by undefined
							if symbol.ImportItemStatus == js_ast.ImportItemMissing {
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
	for chunkIndex := range chunks {
		chunk := &chunks[chunkIndex]
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
			for otherChunkIndex, otherChunk := range chunks {
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
	for chunkIndex := range chunks {
		chunk := &chunks[chunkIndex]
		chunkRepr, ok := chunk.chunkRepr.(*chunkReprJS)
		if !ok {
			continue
		}

		chunkRepr.exportsToOtherChunks = make(map[js_ast.Ref]string)
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
				items = append(items, js_ast.ClauseItem{Name: js_ast.LocRef{Ref: export.Ref}, Alias: alias})
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
	for chunkIndex := range chunks {
		chunk := &chunks[chunkIndex]
		chunkRepr, ok := chunk.chunkRepr.(*chunkReprJS)
		if !ok {
			continue
		}

		var crossChunkPrefixStmts []js_ast.Stmt

		for _, crossChunkImport := range c.sortedCrossChunkImports(chunks, chunkRepr.importsFromOtherChunks) {
			switch c.options.OutputFormat {
			case config.FormatESModule:
				var items []js_ast.ClauseItem
				for _, item := range crossChunkImport.sortedImportItems {
					items = append(items, js_ast.ClauseItem{Name: js_ast.LocRef{Ref: item.ref}, Alias: item.exportAlias})
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
	chunkIndex        uint32
	sortedImportItems crossChunkImportItemArray
}

// This type is just so we can use Go's native sort function
type crossChunkImportArray []crossChunkImport

func (a crossChunkImportArray) Len() int          { return len(a) }
func (a crossChunkImportArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a crossChunkImportArray) Less(i int, j int) bool {
	return a[i].chunkIndex < a[j].chunkIndex
}

// Sort cross-chunk imports by chunk name for determinism
func (c *linkerContext) sortedCrossChunkImports(chunks []chunkInfo, importsFromOtherChunks map[uint32]crossChunkImportItemArray) crossChunkImportArray {
	result := make(crossChunkImportArray, 0, len(importsFromOtherChunks))

	for otherChunkIndex, importItems := range importsFromOtherChunks {
		// Sort imports from a single chunk by alias for determinism
		otherChunk := &chunks[otherChunkIndex]
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

// Sort cross-chunk exports by chunk name for determinism
func (c *linkerContext) sortedCrossChunkExportItems(exportRefs map[js_ast.Ref]bool) renamer.StableRefArray {
	result := make(renamer.StableRefArray, 0, len(exportRefs))
	for ref := range exportRefs {
		result = append(result, renamer.StableRef{
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
	for _, sourceIndex := range c.graph.ReachableFiles {
		file := &c.graph.Files[sourceIndex]
		switch repr := file.InputFile.Repr.(type) {
		case *graph.CSSRepr:
			// Inline URLs for non-CSS files into the CSS file
			var additionalFiles []graph.OutputFile
			for importRecordIndex := range repr.AST.ImportRecords {
				if record := &repr.AST.ImportRecords[importRecordIndex]; record.SourceIndex.IsValid() {
					otherFile := &c.graph.Files[record.SourceIndex.GetIndex()]
					if otherRepr, ok := otherFile.InputFile.Repr.(*graph.JSRepr); ok {
						record.Path.Text = otherRepr.AST.URLForCSS
						record.Path.Namespace = ""
						record.SourceIndex = ast.Index32{}

						// Copy the additional files to the output directory
						additionalFiles = append(additionalFiles, otherFile.InputFile.AdditionalFiles...)
					}
				}
			}
			file.InputFile.AdditionalFiles = additionalFiles

		case *graph.JSRepr:
			for importRecordIndex := range repr.AST.ImportRecords {
				record := &repr.AST.ImportRecords[importRecordIndex]
				if !record.SourceIndex.IsValid() {
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
					if (record.ContainsImportStar || record.ContainsDefaultAlias) && otherRepr.AST.ExportsKind == js_ast.ExportsNone && !otherRepr.AST.HasLazyExport {
						otherRepr.Meta.Wrap = graph.WrapCJS
						otherRepr.AST.ExportsKind = js_ast.ExportsCommonJS
					}

				case ast.ImportRequire:
					// Files that are imported with require() must be CommonJS modules
					if otherRepr.AST.ExportsKind == js_ast.ExportsESM {
						otherRepr.Meta.Wrap = graph.WrapESM
					} else {
						otherRepr.Meta.Wrap = graph.WrapCJS
						otherRepr.AST.ExportsKind = js_ast.ExportsCommonJS
					}

				case ast.ImportDynamic:
					if !c.options.CodeSplitting {
						// If we're not splitting, then import() is just a require() that
						// returns a promise, so the imported file must be a CommonJS module
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
	}

	// Step 2: Propagate dynamic export status for export star statements that
	// are re-exports from a module whose exports are not statically analyzable.
	// In this case the export star must be evaluated at run time instead of at
	// bundle time.
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

	// Step 3: Resolve "export * from" statements. This must be done after we
	// discover all modules that can have dynamic exports because export stars
	// are ignored for those modules.
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

		// Add an empty part for the namespace export that we can fill in later
		repr.Meta.NSExportPartIndex = c.graph.AddPartToFile(sourceIndex, js_ast.Part{
			CanBeRemovedIfUnused: true,
		})

		// Also add a special export so import stars can bind to it. This must be
		// done in this step because it must come after CommonJS module discovery
		// but before matching imports with exports.
		repr.Meta.ResolvedExportStar = &graph.ExportData{
			Ref:         repr.AST.ExportsRef,
			SourceIndex: sourceIndex,
		}
		repr.AST.TopLevelSymbolToParts[repr.AST.ExportsRef] = []uint32{repr.Meta.NSExportPartIndex}
	}

	// Step 4: Match imports with exports. This must be done after we process all
	// export stars because imports can bind to export star re-exports.
	for _, sourceIndex := range c.graph.ReachableFiles {
		file := &c.graph.Files[sourceIndex]
		repr, ok := file.InputFile.Repr.(*graph.JSRepr)
		if !ok {
			continue
		}

		if len(repr.AST.NamedImports) > 0 {
			c.matchImportsWithExportsForFile(uint32(sourceIndex))
		}

		// If we're exporting as CommonJS and this file doesn't need a wrapper,
		// then we'll be using the actual CommonJS "exports" and/or "module"
		// symbols. In that case make sure to mark them as such so they don't
		// get minified.
		if (c.options.OutputFormat == config.FormatPreserve || c.options.OutputFormat == config.FormatCommonJS) &&
			repr.Meta.Wrap != graph.WrapCJS && file.IsEntryPoint() {
			exportsRef := js_ast.FollowSymbols(c.graph.Symbols, repr.AST.ExportsRef)
			moduleRef := js_ast.FollowSymbols(c.graph.Symbols, repr.AST.ModuleRef)
			c.graph.Symbols.Get(exportsRef).Kind = js_ast.SymbolUnbound
			c.graph.Symbols.Get(moduleRef).Kind = js_ast.SymbolUnbound
		}

		// Create the wrapper part for wrapped files. This is needed by a later step.
		c.createWrapperForFile(uint32(sourceIndex))
	}

	// Step 5: Create namespace exports for every file. This is always necessary
	// for CommonJS files, and is also necessary for other files if they are
	// imported using an import star statement.
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
				// Re-exporting multiple symbols with the same name causes an ambiguous
				// export. These names cannot be used and should not end up in generated code.
				otherRepr := c.graph.Files[export.SourceIndex].InputFile.Repr.(*graph.JSRepr)
				if len(export.PotentiallyAmbiguousExportStarRefs) > 0 {
					mainRef := export.Ref
					if imported, ok := otherRepr.Meta.ImportsToBind[export.Ref]; ok {
						mainRef = imported.Ref
					}
					for _, ambiguousExport := range export.PotentiallyAmbiguousExportStarRefs {
						ambiguousRepr := c.graph.Files[ambiguousExport.SourceIndex].InputFile.Repr.(*graph.JSRepr)
						ambiguousRef := ambiguousExport.Ref
						if imported, ok := ambiguousRepr.Meta.ImportsToBind[ambiguousExport.Ref]; ok {
							ambiguousRef = imported.Ref
						}
						if mainRef != ambiguousRef {
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

				aliases = append(aliases, alias)
			}
			sort.Strings(aliases)
			repr.Meta.SortedAndFilteredExportAliases = aliases

			// Export creation uses "sortedAndFilteredExportAliases" so this must
			// come second after we fill in that array
			c.createExportsForFile(uint32(sourceIndex))

			waitGroup.Done()
		}(sourceIndex, repr)
	}
	waitGroup.Wait()

	// Step 6: Bind imports to exports. This adds non-local dependencies on the
	// parts that declare the export to all parts that use the import. Also
	// generate wrapper parts for wrapped files.
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
			copies := make([]js_ast.Ref, len(repr.Meta.SortedAndFilteredExportAliases))
			for i, alias := range repr.Meta.SortedAndFilteredExportAliases {
				copies[i] = c.graph.GenerateNewSymbol(sourceIndex, js_ast.SymbolOther, "export_"+alias)
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
		if repr.Meta.Wrap != graph.WrapCJS && repr.AST.ExportsKind != js_ast.ExportsCommonJS && (!file.IsEntryPoint() ||
			c.options.OutputFormat != config.FormatCommonJS) {
			name := file.InputFile.Source.IdentifierName
			c.graph.Symbols.Get(repr.AST.ExportsRef).OriginalName = name + "_exports"
			c.graph.Symbols.Get(repr.AST.ModuleRef).OriginalName = name + "_module"
		}

		// Include the "__export" symbol from the runtime if it was used in the
		// previous step. The previous step can't do this because it's running in
		// parallel and can't safely mutate the "importsToBind" map of another file.
		if repr.Meta.NeedsExportSymbolFromRuntime || repr.Meta.NeedsMarkAsModuleSymbolFromRuntime {
			runtimeRepr := c.graph.Files[runtime.SourceIndex].InputFile.Repr.(*graph.JSRepr)
			if repr.Meta.NeedsExportSymbolFromRuntime {
				exportRef := runtimeRepr.AST.ModuleScope.Members["__export"].Ref
				c.graph.GenerateSymbolImportAndUse(sourceIndex, repr.Meta.NSExportPartIndex, exportRef, 1, runtime.SourceIndex)
			}
			if repr.Meta.NeedsMarkAsModuleSymbolFromRuntime {
				markAsModuleRef := runtimeRepr.AST.ModuleScope.Members["__markAsModule"].Ref
				c.graph.GenerateSymbolImportAndUse(sourceIndex, repr.Meta.NSExportPartIndex, markAsModuleRef, 1, runtime.SourceIndex)
			}
		}

		for importRef, importData := range repr.Meta.ImportsToBind {
			resolvedRepr := c.graph.Files[importData.SourceIndex].InputFile.Repr.(*graph.JSRepr)
			partsDeclaringSymbol := resolvedRepr.AST.TopLevelSymbolToParts[importData.Ref]

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
			js_ast.MergeSymbols(c.graph.Symbols, importRef, importData.Ref)
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
				for _, partIndex := range targetRepr.AST.TopLevelSymbolToParts[targetRef] {
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
					PartIndex:   repr.Meta.NSExportPartIndex,
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
		}

		// Encode import-specific constraints in the dependency graph
		for partIndex, part := range repr.AST.Parts {
			toModuleUses := uint32(0)
			runtimeRequireUses := uint32(0)

			// Imports of wrapped files must depend on the wrapper
			for _, importRecordIndex := range part.ImportRecordIndices {
				record := &repr.AST.ImportRecords[importRecordIndex]

				// Don't follow external imports (this includes import() expressions)
				if !record.SourceIndex.IsValid() || c.isExternalDynamicImport(record, sourceIndex) {
					// This is an external import. Check if it will be a "require()" call.
					if record.Kind == ast.ImportRequire || !c.options.OutputFormat.KeepES6ImportExportSyntax() ||
						(record.Kind == ast.ImportDynamic && c.options.UnsupportedJSFeatures.Has(compat.DynamicImport)) {
						// We should use "__require" instead of "require" if we're not
						// generating a CommonJS output file, since it won't exist otherwise
						if config.ShouldCallRuntimeRequire(c.options.Mode, c.options.OutputFormat) {
							record.CallRuntimeRequire = true
							runtimeRequireUses++
						}

						// It needs the "__toModule" wrapper if it wasn't originally a
						// CommonJS import (i.e. it wasn't a "require()" call).
						if record.Kind != ast.ImportRequire {
							record.WrapWithToModule = true
							toModuleUses++
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
					// "__toModule" wrapper as long as it's not a bare "require()"
					if record.Kind != ast.ImportRequire && otherRepr.AST.ExportsKind == js_ast.ExportsCommonJS {
						record.WrapWithToModule = true
						toModuleUses++
					}

					// If this is an ESM wrapper, also depend on the exports object
					// since the final code will contain an inline reference to it.
					// This must be done for "require()" and "import()" expressions
					// but does not need to be done for "import" statements since
					// those just cause us to reference the exports directly.
					if otherRepr.Meta.Wrap == graph.WrapESM && record.Kind != ast.ImportStmt {
						c.graph.GenerateSymbolImportAndUse(sourceIndex, uint32(partIndex), otherRepr.AST.ExportsRef, 1, otherSourceIndex)
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
			// "__toModule" symbol from the runtime to wrap the result of "require()"
			c.graph.GenerateRuntimeSymbolImportAndUse(sourceIndex, uint32(partIndex), "__toModule", toModuleUses)

			// If there are unbundled calls to "require()" and we're not generating
			// code for node, then substitute a "__require" wrapper for "require".
			c.graph.GenerateRuntimeSymbolImportAndUse(sourceIndex, uint32(partIndex), "__require", runtimeRequireUses)

			// If there's an ES6 export star statement of a non-ES6 module, then we're
			// going to need the "__reExport" symbol from the runtime
			reExportUses := uint32(0)
			for _, importRecordIndex := range repr.AST.ExportStarImportRecords {
				record := &repr.AST.ImportRecords[importRecordIndex]

				// Is this export star evaluated at run time?
				happensAtRunTime := !record.SourceIndex.IsValid() && (!file.IsEntryPoint() || !c.options.OutputFormat.KeepES6ImportExportSyntax())
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
					record.CallsRunTimeReExportFn = true
					repr.AST.UsesExportsRef = true
					reExportUses++
				}
			}
			c.graph.GenerateRuntimeSymbolImportAndUse(sourceIndex, uint32(partIndex), "__reExport", reExportUses)
		}
	}
}

func (c *linkerContext) generateCodeForLazyExport(sourceIndex uint32) {
	file := &c.graph.Files[sourceIndex]
	repr := file.InputFile.Repr.(*graph.JSRepr)

	// Grab the lazy expression
	if len(repr.AST.Parts) < 1 {
		panic("Internal error")
	}
	part := &repr.AST.Parts[0]
	if len(part.Stmts) != 1 {
		panic("Internal error")
	}
	lazy, ok := part.Stmts[0].Data.(*js_ast.SLazyExport)
	if !ok {
		panic("Internal error")
	}

	// Use "module.exports = value" for CommonJS-style modules
	if repr.AST.ExportsKind == js_ast.ExportsCommonJS {
		part.Stmts = []js_ast.Stmt{js_ast.AssignStmt(
			js_ast.Expr{Loc: lazy.Value.Loc, Data: &js_ast.EDot{
				Target:  js_ast.Expr{Loc: lazy.Value.Loc, Data: &js_ast.EIdentifier{Ref: repr.AST.ModuleRef}},
				Name:    "exports",
				NameLoc: lazy.Value.Loc,
			}},
			lazy.Value,
		)}
		c.graph.GenerateSymbolImportAndUse(sourceIndex, 0, repr.AST.ModuleRef, 1, sourceIndex)
		return
	}

	// Otherwise, generate ES6 export statements. These are added as additional
	// parts so they can be tree shaken individually.
	part.Stmts = nil

	type prevExport struct {
		ref       js_ast.Ref
		partIndex uint32
	}

	generateExport := func(name string, alias string, value js_ast.Expr) prevExport {
		// Generate a new symbol
		ref := c.graph.GenerateNewSymbol(sourceIndex, js_ast.SymbolOther, name)

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
		partIndex := c.graph.AddPartToFile(sourceIndex, js_ast.Part{
			Stmts:                []js_ast.Stmt{stmt},
			DeclaredSymbols:      []js_ast.DeclaredSymbol{{Ref: ref, IsTopLevel: true}},
			CanBeRemovedIfUnused: true,
		})
		c.graph.GenerateSymbolImportAndUse(sourceIndex, partIndex, repr.AST.ModuleRef, 1, sourceIndex)
		repr.Meta.ResolvedExports[alias] = graph.ExportData{Ref: ref, SourceIndex: sourceIndex}
		return prevExport{ref: ref, partIndex: partIndex}
	}

	// Unwrap JSON objects into separate top-level variables
	var prevExports []js_ast.Ref
	jsonValue := lazy.Value
	if object, ok := jsonValue.Data.(*js_ast.EObject); ok {
		clone := *object
		clone.Properties = append(make([]js_ast.Property, 0, len(clone.Properties)), clone.Properties...)
		for i, property := range clone.Properties {
			if str, ok := property.Key.Data.(*js_ast.EString); ok &&
				(!file.IsEntryPoint() || js_lexer.IsIdentifierUTF16(str.Value) ||
					!c.options.UnsupportedJSFeatures.Has(compat.ArbitraryModuleNamespaceNames)) {
				name := js_lexer.UTF16ToString(str.Value)
				exportRef := generateExport(name, name, *property.Value).ref
				prevExports = append(prevExports, exportRef)
				clone.Properties[i].Value = &js_ast.Expr{Loc: property.Key.Loc, Data: &js_ast.EIdentifier{Ref: exportRef}}
			}
		}
		jsonValue.Data = &clone
	}

	// Generate the default export
	finalExportPartIndex := generateExport(file.InputFile.Source.IdentifierName+"_default", "default", jsonValue).partIndex

	// The default export depends on all of the previous exports
	for _, exportRef := range prevExports {
		c.graph.GenerateSymbolImportAndUse(sourceIndex, finalExportPartIndex, exportRef, 1, sourceIndex)
	}
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
	nsExportSymbolUses := make(map[js_ast.Ref]js_ast.SymbolUse)
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
		body := js_ast.FnBody{Stmts: []js_ast.Stmt{{Loc: value.Loc, Data: &js_ast.SReturn{Value: &value}}}}
		if c.options.UnsupportedJSFeatures.Has(compat.Arrow) {
			getter = js_ast.Expr{Data: &js_ast.EFunction{Fn: js_ast.Fn{Body: body}}}
		} else {
			getter = js_ast.Expr{Data: &js_ast.EArrow{PreferExpr: true, Body: body}}
		}
		properties = append(properties, js_ast.Property{
			Key:   js_ast.Expr{Data: &js_ast.EString{Value: js_lexer.StringToUTF16(alias)}},
			Value: &getter,
		})
		nsExportSymbolUses[export.Ref] = js_ast.SymbolUse{CountEstimate: 1}

		// Make sure the part that declares the export is included
		for _, partIndex := range c.graph.Files[export.SourceIndex].InputFile.Repr.(*graph.JSRepr).AST.TopLevelSymbolToParts[export.Ref] {
			// Use a non-local dependency since this is likely from a different
			// file if it came in through an export star
			nsExportDependencies = append(nsExportDependencies, js_ast.Dependency{
				SourceIndex: export.SourceIndex,
				PartIndex:   partIndex,
			})
		}
	}

	// Prefix this part with "var exports = {}" if this isn't a CommonJS module
	declaredSymbols := []js_ast.DeclaredSymbol{}
	var nsExportStmts []js_ast.Stmt
	if repr.AST.ExportsKind != js_ast.ExportsCommonJS && (!file.IsEntryPoint() || c.options.OutputFormat != config.FormatCommonJS) {
		nsExportStmts = append(nsExportStmts, js_ast.Stmt{Data: &js_ast.SLocal{Decls: []js_ast.Decl{{
			Binding: js_ast.Binding{Data: &js_ast.BIdentifier{Ref: repr.AST.ExportsRef}},
			Value:   &js_ast.Expr{Data: &js_ast.EObject{}},
		}}}})
		declaredSymbols = append(declaredSymbols, js_ast.DeclaredSymbol{
			Ref:        repr.AST.ExportsRef,
			IsTopLevel: true,
		})
	}

	// If this file was originally ESM but is now in CommonJS, add a call to
	// "__markAsModule" which sets the "__esModule" property to true. This must
	// be done before any to "require()" or circular imports of multiple modules
	// that have been each converted from ESM to CommonJS may not work correctly.
	if repr.AST.ExportKeyword.Len > 0 && (repr.AST.ExportsKind == js_ast.ExportsCommonJS ||
		(file.IsEntryPoint() && c.options.OutputFormat == config.FormatCommonJS)) {
		runtimeRepr := c.graph.Files[runtime.SourceIndex].InputFile.Repr.(*graph.JSRepr)
		markAsModuleRef := runtimeRepr.AST.ModuleScope.Members["__markAsModule"].Ref
		nsExportStmts = append(nsExportStmts, js_ast.Stmt{Data: &js_ast.SExpr{Value: js_ast.Expr{Data: &js_ast.ECall{
			Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: markAsModuleRef}},
			Args:   []js_ast.Expr{{Data: &js_ast.EIdentifier{Ref: repr.AST.ExportsRef}}},
		}}}})

		// Make sure this file depends on the "__markAsModule" symbol
		for _, partIndex := range runtimeRepr.AST.TopLevelSymbolToParts[markAsModuleRef] {
			nsExportDependencies = append(nsExportDependencies, js_ast.Dependency{
				SourceIndex: runtime.SourceIndex,
				PartIndex:   partIndex,
			})
		}

		// Pull in the "__markAsModule" symbol later. Also make sure the "exports"
		// variable is marked as used because we used it above.
		repr.Meta.NeedsMarkAsModuleSymbolFromRuntime = true
		repr.AST.UsesExportsRef = true
	}

	// "__export(exports, { foo: () => foo })"
	exportRef := js_ast.InvalidRef
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
		for _, partIndex := range runtimeRepr.AST.TopLevelSymbolToParts[exportRef] {
			nsExportDependencies = append(nsExportDependencies, js_ast.Dependency{
				SourceIndex: runtime.SourceIndex,
				PartIndex:   partIndex,
			})
		}

		// Make sure the CommonJS closure, if there is one, includes "exports"
		repr.AST.UsesExportsRef = true
	}

	// No need to generate a part if it'll be empty
	if len(nsExportStmts) > 0 {
		// Initialize the part that was allocated for us earlier. The information
		// here will be used after this during tree shaking.
		repr.AST.Parts[repr.Meta.NSExportPartIndex] = js_ast.Part{
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
		if exportRef != js_ast.InvalidRef {
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
		commonJSRef := runtimeRepr.AST.NamedExports["__commonJS"].Ref
		commonJSParts := runtimeRepr.AST.TopLevelSymbolToParts[commonJSRef]

		// Generate the dummy part
		dependencies := make([]js_ast.Dependency, len(commonJSParts))
		for i, partIndex := range commonJSParts {
			dependencies[i] = js_ast.Dependency{
				SourceIndex: runtime.SourceIndex,
				PartIndex:   partIndex,
			}
		}
		partIndex := c.graph.AddPartToFile(sourceIndex, js_ast.Part{
			SymbolUses: map[js_ast.Ref]js_ast.SymbolUse{
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
		c.graph.GenerateSymbolImportAndUse(sourceIndex, partIndex, commonJSRef, 1, runtime.SourceIndex)

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
		esmRef := runtimeRepr.AST.NamedExports["__esm"].Ref
		esmParts := runtimeRepr.AST.TopLevelSymbolToParts[esmRef]

		// Generate the dummy part
		dependencies := make([]js_ast.Dependency, len(esmParts))
		for i, partIndex := range esmParts {
			dependencies[i] = js_ast.Dependency{
				SourceIndex: runtime.SourceIndex,
				PartIndex:   partIndex,
			}
		}
		partIndex := c.graph.AddPartToFile(sourceIndex, js_ast.Part{
			SymbolUses: map[js_ast.Ref]js_ast.SymbolUse{
				repr.AST.WrapperRef: {CountEstimate: 1},
			},
			DeclaredSymbols: []js_ast.DeclaredSymbol{
				{Ref: repr.AST.WrapperRef, IsTopLevel: true},
			},
			Dependencies: dependencies,
		})
		repr.Meta.WrapperPartIndex = ast.MakeIndex32(partIndex)
		c.graph.GenerateSymbolImportAndUse(sourceIndex, partIndex, esmRef, 1, runtime.SourceIndex)
	}
}

func (c *linkerContext) matchImportsWithExportsForFile(sourceIndex uint32) {
	file := &c.graph.Files[sourceIndex]
	repr := file.InputFile.Repr.(*graph.JSRepr)
	tracker := logger.MakeLineColumnTracker(&file.InputFile.Source)

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

		importRef := js_ast.Ref{SourceIndex: sourceIndex, InnerIndex: uint32(innerIndex)}
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
			c.graph.Symbols.Get(importRef).NamespaceAlias = &js_ast.NamespaceAlias{
				NamespaceRef: result.namespaceRef,
				Alias:        result.alias,
			}

		case matchImportNormalAndNamespace:
			repr.Meta.ImportsToBind[importRef] = graph.ImportData{
				ReExports:   reExports,
				SourceIndex: result.sourceIndex,
				Ref:         result.ref,
			}

			c.graph.Symbols.Get(importRef).NamespaceAlias = &js_ast.NamespaceAlias{
				NamespaceRef: result.namespaceRef,
				Alias:        result.alias,
			}

		case matchImportCycle:
			namedImport := repr.AST.NamedImports[importRef]
			c.log.AddRangeError(&tracker, js_lexer.RangeOfIdentifier(file.InputFile.Source, namedImport.AliasLoc),
				fmt.Sprintf("Detected cycle while resolving import %q", namedImport.Alias))

		case matchImportProbablyTypeScriptType:
			repr.Meta.IsProbablyTypeScriptType[importRef] = true

		case matchImportAmbiguous:
			namedImport := repr.AST.NamedImports[importRef]
			r := js_lexer.RangeOfIdentifier(file.InputFile.Source, namedImport.AliasLoc)
			var notes []logger.MsgData

			// Provide the locations of both ambiguous exports if possible
			if result.nameLoc.Start != 0 && result.otherNameLoc.Start != 0 {
				a := c.graph.Files[result.sourceIndex].InputFile.Source
				b := c.graph.Files[result.otherSourceIndex].InputFile.Source
				trackerA := logger.MakeLineColumnTracker(&a)
				trackerB := logger.MakeLineColumnTracker(&b)
				notes = []logger.MsgData{
					logger.RangeData(&trackerA, js_lexer.RangeOfIdentifier(a, result.nameLoc), "One matching export is here"),
					logger.RangeData(&trackerB, js_lexer.RangeOfIdentifier(b, result.otherNameLoc), "Another matching export is here"),
				}
			}

			symbol := c.graph.Symbols.Get(importRef)
			if symbol.ImportItemStatus == js_ast.ImportItemGenerated {
				// This is a warning instead of an error because although it appears
				// to be a named import, it's actually an automatically-generated
				// named import that was originally a property access on an import
				// star namespace object. Normally this property access would just
				// resolve to undefined at run-time instead of failing at binding-
				// time, so we emit a warning and rewrite the value to the literal
				// "undefined" instead of emitting an error.
				symbol.ImportItemStatus = js_ast.ImportItemMissing
				msg := fmt.Sprintf("Import %q will always be undefined because there are multiple matching exports", namedImport.Alias)
				c.log.AddRangeWarningWithNotes(&tracker, r, msg, notes)
			} else {
				msg := fmt.Sprintf("Ambiguous import %q has multiple matching exports", namedImport.Alias)
				c.log.AddRangeErrorWithNotes(&tracker, r, msg, notes)
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
	kind             matchImportKind
	namespaceRef     js_ast.Ref
	alias            string
	sourceIndex      uint32
	nameLoc          logger.Loc // Optional, goes with sourceIndex, ignore if zero
	otherSourceIndex uint32
	otherNameLoc     logger.Loc // Optional, goes with otherSourceIndex, ignore if zero
	ref              js_ast.Ref
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
			if status == importExternal && c.options.OutputFormat.KeepES6ImportExportSyntax() {
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
			if namedImport.NamespaceRef != js_ast.InvalidRef {
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
				source := trackerFile.InputFile.Source
				symbol := c.graph.Symbols.Get(tracker.importRef)
				symbol.ImportItemStatus = js_ast.ImportItemMissing
				sourceTracker := logger.MakeLineColumnTracker(&source)
				c.log.AddRangeWarning(&sourceTracker, js_lexer.RangeOfIdentifier(source, namedImport.AliasLoc),
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
			source := trackerFile.InputFile.Source
			sourceTracker := logger.MakeLineColumnTracker(&source)
			namedImport := trackerFile.InputFile.Repr.(*graph.JSRepr).AST.NamedImports[tracker.importRef]
			r := js_lexer.RangeOfIdentifier(source, namedImport.AliasLoc)

			// Report mismatched imports and exports
			if symbol.ImportItemStatus == js_ast.ImportItemGenerated {
				// This is a warning instead of an error because although it appears
				// to be a named import, it's actually an automatically-generated
				// named import that was originally a property access on an import
				// star namespace object. Normally this property access would just
				// resolve to undefined at run-time instead of failing at binding-
				// time, so we emit a warning and rewrite the value to the literal
				// "undefined" instead of emitting an error.
				symbol.ImportItemStatus = js_ast.ImportItemMissing
				c.log.AddRangeWarning(&sourceTracker, r, fmt.Sprintf(
					"Import %q will always be undefined because there is no matching export", namedImport.Alias))
			} else {
				c.log.AddRangeError(&sourceTracker, r, fmt.Sprintf("No matching export in %q for import %q",
					c.graph.Files[nextTracker.sourceIndex].InputFile.Source.PrettyPath, namedImport.Alias))
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
			for _, resolvedPartIndex := range c.graph.Files[tracker.sourceIndex].InputFile.Repr.(*graph.JSRepr).AST.TopLevelSymbolToParts[tracker.importRef] {
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
		if (!record.SourceIndex.IsValid() && (!c.graph.Files[sourceIndex].IsEntryPoint() || !c.options.OutputFormat.KeepES6ImportExportSyntax())) ||
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

	// Is this a disabled file?
	otherSourceIndex := record.SourceIndex.GetIndex()
	if c.graph.Files[otherSourceIndex].InputFile.Source.KeyPath.IsDisabled() {
		return importTracker{sourceIndex: otherSourceIndex, importRef: js_ast.InvalidRef}, importDisabled, nil
	}

	// Is this a named import of a file without any exports?
	otherRepr := c.graph.Files[otherSourceIndex].InputFile.Repr.(*graph.JSRepr)
	if !namedImport.AliasIsStar && !otherRepr.AST.HasLazyExport &&
		// CommonJS exports
		otherRepr.AST.ExportKeyword.Len == 0 && namedImport.Alias != "default" &&
		// ESM exports
		!otherRepr.AST.UsesExportsRef && !otherRepr.AST.UsesModuleRef {
		// Just warn about it and replace the import with "undefined"
		return importTracker{sourceIndex: otherSourceIndex, importRef: js_ast.InvalidRef}, importCommonJSWithoutExports, nil
	}

	// Is this a CommonJS file?
	if otherRepr.AST.ExportsKind == js_ast.ExportsCommonJS {
		return importTracker{sourceIndex: otherSourceIndex, importRef: js_ast.InvalidRef}, importCommonJS, nil
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
		isTreeShakingEnabled := config.IsTreeShakingEnabled(c.options.Mode, c.options.OutputFormat)

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
				}

				// If we get here then the import was included for its side effects, so
				// we must also keep this part
				canBeRemovedIfUnused = false
			}

			// Include all parts in this file with side effects, or just include
			// everything if tree-shaking is disabled. Note that we still want to
			// perform tree-shaking on the runtime even if tree-shaking is disabled.
			if !canBeRemovedIfUnused || (!part.ForceTreeShaking && !isTreeShakingEnabled && file.IsEntryPoint()) {
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
	return record.Kind == ast.ImportDynamic && c.graph.Files[record.SourceIndex.GetIndex()].IsEntryPoint() && record.SourceIndex.GetIndex() != sourceIndex
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

func sanitizeFilePathForVirtualModulePath(path string) string {
	// Convert it to a safe file path. See: https://stackoverflow.com/a/31976060
	sb := strings.Builder{}
	needsGap := false
	for _, c := range path {
		switch c {
		case 0:
			// These characters are forbidden on Unix and Windows

		case '<', '>', ':', '"', '|', '?', '*':
			// These characters are forbidden on Windows

		default:
			if c < 0x20 {
				// These characters are forbidden on Windows
				break
			}

			// Turn runs of invalid characters into a '_'
			if needsGap {
				sb.WriteByte('_')
				needsGap = false
			}

			sb.WriteRune(c)
			continue
		}

		if sb.Len() > 0 {
			needsGap = true
		}
	}

	// Make sure the name isn't empty
	if sb.Len() == 0 {
		return "_"
	}

	// Note: An extension will be added to this base name, so there is no need to
	// avoid forbidden file names such as ".." since ".js" is a valid file name.
	return sb.String()
}

func (c *linkerContext) computeChunks() []chunkInfo {
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
		info := chunkInfo{
			entryBits:             entryBits,
			isEntryPoint:          true,
			sourceIndex:           entryPoint.SourceIndex,
			entryPointBit:         uint(i),
			filesWithPartsInChunk: make(map[uint32]bool),
		}

		switch file.InputFile.Repr.(type) {
		case *graph.JSRepr:
			info.chunkRepr = &chunkReprJS{}
			jsChunks[entryBits.String()] = info

		case *graph.CSSRepr:
			info.chunkRepr = &chunkReprCSS{}
			cssChunks[entryBits.String()] = info
		}
	}

	// Figure out which files are in which chunk
	for _, sourceIndex := range c.graph.ReachableFiles {
		file := &c.graph.Files[sourceIndex]
		if !file.IsLive {
			// Ignore this file if it's not included in the bundle
			continue
		}
		key := file.EntryBits.String()
		var chunk chunkInfo
		var ok bool
		switch file.InputFile.Repr.(type) {
		case *graph.JSRepr:
			chunk, ok = jsChunks[key]
			if !ok {
				chunk.entryBits = file.EntryBits
				chunk.filesWithPartsInChunk = make(map[uint32]bool)
				chunk.chunkRepr = &chunkReprJS{}
				jsChunks[key] = chunk
			}
		case *graph.CSSRepr:
			chunk, ok = cssChunks[key]
			if !ok {
				chunk.entryBits = file.EntryBits
				chunk.filesWithPartsInChunk = make(map[uint32]bool)
				chunk.chunkRepr = &chunkReprCSS{}

				// Check whether this is the CSS file to go with a JS entry point
				if jsChunk, ok := jsChunks[key]; ok && jsChunk.isEntryPoint {
					chunk.isEntryPoint = true
					chunk.sourceIndex = jsChunk.sourceIndex
					chunk.entryPointBit = jsChunk.entryPointBit
				}

				cssChunks[key] = chunk
			}
		}
		chunk.filesWithPartsInChunk[uint32(sourceIndex)] = true
	}

	// Sort the chunks for determinism. This matters because we use chunk indices
	// as sorting keys in a few places.
	sortedChunks := make([]chunkInfo, 0, len(jsChunks)+len(cssChunks))
	sortedKeys := make([]string, 0, len(jsChunks)+len(cssChunks))
	for key := range jsChunks {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)
	for _, key := range sortedKeys {
		sortedChunks = append(sortedChunks, jsChunks[key])
	}
	sortedKeys = sortedKeys[:0]
	for key := range cssChunks {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)
	for _, key := range sortedKeys {
		sortedChunks = append(sortedChunks, cssChunks[key])
	}

	// Map from the entry point file to this chunk. We will need this later if
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

	// Determine the order of files within the chunk ahead of time. This may
	// generate additional CSS chunks from JS chunks that import CSS files.
	{
		for chunkIndex, chunk := range sortedChunks {
			js, jsParts, css := c.chunkFileOrder(&chunk)

			switch chunk.chunkRepr.(type) {
			case *chunkReprJS:
				sortedChunks[chunkIndex].filesInChunkInOrder = js
				sortedChunks[chunkIndex].partsInChunkInOrder = jsParts

				// If JS files include CSS files, make a sibling chunk for the CSS
				if len(css) > 0 {
					sortedChunks = append(sortedChunks, chunkInfo{
						filesInChunkInOrder:   css,
						entryBits:             chunk.entryBits,
						isEntryPoint:          chunk.isEntryPoint,
						sourceIndex:           chunk.sourceIndex,
						entryPointBit:         chunk.entryPointBit,
						filesWithPartsInChunk: make(map[uint32]bool),
						chunkRepr:             &chunkReprCSS{},
					})
				}

			case *chunkReprCSS:
				sortedChunks[chunkIndex].filesInChunkInOrder = css
			}
		}
	}

	// Assign general information to each chunk
	for chunkIndex := range sortedChunks {
		chunk := &sortedChunks[chunkIndex]

		// Assign a unique key to each chunk. This key encodes the index directly so
		// we can easily recover it later without needing to look it up in a map. The
		// last 8 numbers of the key are the chunk index.
		chunk.uniqueKey = fmt.Sprintf("%s%08d", c.uniqueKeyPrefix, chunkIndex)

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
			if c.graph.Files[chunk.sourceIndex].IsUserSpecifiedEntryPoint() {
				dir, base, ext = c.pathRelativeToOutbase(chunk.sourceIndex, chunk.entryPointBit, stdExt, false /* avoidIndex */)
				template = c.options.EntryPathTemplate
			} else {
				dir, base, ext = c.pathRelativeToOutbase(chunk.sourceIndex, chunk.entryPointBit, stdExt, true /* avoidIndex */)
				template = c.options.ChunkPathTemplate
			}
		} else {
			dir = "/"
			base = "chunk"
			ext = stdExt
			template = c.options.ChunkPathTemplate
		}

		// Determine the output path template
		template = append(append(make([]config.PathTemplate, 0, len(template)+1), template...), config.PathTemplate{Data: ext})
		chunk.finalTemplate = config.SubstituteTemplate(template, config.PathPlaceholders{
			Dir:  &dir,
			Name: &base,
		})
	}

	return sortedChunks
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

func (c *linkerContext) chunkFileOrder(chunk *chunkInfo) (js []uint32, jsParts []partRange, css []uint32) {
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
		isFileInThisChunk := chunk.entryBits.Equals(file.EntryBits)

		switch repr := file.InputFile.Repr.(type) {
		case *graph.JSRepr:
			// Wrapped files can't be split because they are all inside the wrapper
			canFileBeSplit := repr.Meta.Wrap == graph.WrapNone

			// Make sure the generated call to "__export(exports, ...)" comes first
			// before anything else in this file
			if canFileBeSplit && isFileInThisChunk && repr.AST.Parts[repr.Meta.NSExportPartIndex].IsLive {
				jsParts = appendOrExtendPartRange(jsParts, sourceIndex, repr.Meta.NSExportPartIndex)
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
					if canFileBeSplit && uint32(partIndex) != repr.Meta.NSExportPartIndex && c.shouldIncludePart(repr, part) {
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

		case *graph.CSSRepr:
			if isFileInThisChunk {
				// All imported files come first
				for _, record := range repr.AST.ImportRecords {
					if record.SourceIndex.IsValid() {
						visit(record.SourceIndex.GetIndex())
					}
				}

				// Then this file comes afterward
				css = append(css, sourceIndex)
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
	namespaceRef js_ast.Ref,
	importRecordIndex uint32,
) bool {
	repr := c.graph.Files[sourceIndex].InputFile.Repr.(*graph.JSRepr)
	record := &repr.AST.ImportRecords[importRecordIndex]

	// Is this an external import?
	if !record.SourceIndex.IsValid() {
		// Keep the "import" statement if "import" statements are supported
		if c.options.OutputFormat.KeepES6ImportExportSyntax() {
			return false
		}

		// Otherwise, replace this statement with a call to "require()"
		stmtList.insideWrapperPrefix = append(stmtList.insideWrapperPrefix, js_ast.Stmt{
			Loc: loc,
			Data: &js_ast.SLocal{Decls: []js_ast.Decl{{
				Binding: js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: namespaceRef}},
				Value: &js_ast.Expr{Loc: record.Range.Loc, Data: &js_ast.ERequireString{
					ImportRecordIndex: importRecordIndex,
				}},
			}}},
		})
		return true
	}

	// We don't need a call to "require()" if this is a self-import inside a
	// CommonJS-style module, since we can just reference the exports directly.
	if repr.AST.ExportsKind == js_ast.ExportsCommonJS && js_ast.FollowSymbols(c.graph.Symbols, namespaceRef) == repr.AST.ExportsRef {
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
				Value: &js_ast.Expr{Loc: record.Range.Loc, Data: &js_ast.ERequireString{
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

	for _, stmt := range partStmts {
		switch s := stmt.Data.(type) {
		case *js_ast.SImport:
			// "import * as ns from 'path'"
			// "import {foo} from 'path'"
			if c.shouldRemoveImportExportStmt(sourceIndex, stmtList, stmt.Loc, s.NamespaceRef, s.ImportRecordIndex) {
				continue
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
			if !record.SourceIndex.IsValid() && c.options.OutputFormat.KeepES6ImportExportSyntax() {
				if record.CallsRunTimeReExportFn {
					// Turn this statement into "import * as ns from 'path'"
					stmt.Data = &js_ast.SImport{
						NamespaceRef:      s.NamespaceRef,
						StarNameLoc:       &stmt.Loc,
						ImportRecordIndex: s.ImportRecordIndex,
					}

					// Prefix this module with "__reExport(exports, ns)"
					exportStarRef := c.graph.Files[runtime.SourceIndex].InputFile.Repr.(*graph.JSRepr).AST.ModuleScope.Members["__reExport"].Ref
					stmtList.insideWrapperPrefix = append(stmtList.insideWrapperPrefix, js_ast.Stmt{
						Loc: stmt.Loc,
						Data: &js_ast.SExpr{Value: js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.ECall{
							Target: js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: exportStarRef}},
							Args: []js_ast.Expr{
								{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: repr.AST.ExportsRef}},
								{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: s.NamespaceRef}},
							},
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

				if record.CallsRunTimeReExportFn {
					var target js_ast.E
					if record.SourceIndex.IsValid() {
						if otherRepr := c.graph.Files[record.SourceIndex.GetIndex()].InputFile.Repr.(*graph.JSRepr); otherRepr.AST.ExportsKind == js_ast.ExportsESMWithDynamicFallback {
							// Prefix this module with "__reExport(exports, otherExports)"
							target = &js_ast.EIdentifier{Ref: otherRepr.AST.ExportsRef}
						}
					}
					if target == nil {
						// Prefix this module with "__reExport(exports, require(path))"
						target = &js_ast.ERequireString{
							ImportRecordIndex: s.ImportRecordIndex,
						}
					}
					exportStarRef := c.graph.Files[runtime.SourceIndex].InputFile.Repr.(*graph.JSRepr).AST.ModuleScope.Members["__reExport"].Ref
					stmtList.insideWrapperPrefix = append(stmtList.insideWrapperPrefix, js_ast.Stmt{
						Loc: stmt.Loc,
						Data: &js_ast.SExpr{Value: js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.ECall{
							Target: js_ast.Expr{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: exportStarRef}},
							Args: []js_ast.Expr{
								{Loc: stmt.Loc, Data: &js_ast.EIdentifier{Ref: repr.AST.ExportsRef}},
								{Loc: record.Range.Loc, Data: target},
							},
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
		meta.ExportsRef = js_ast.InvalidRef
	}
	return
}

func (c *linkerContext) generateCodeForFileInChunkJS(
	r renamer.Renamer,
	waitGroup *sync.WaitGroup,
	partRange partRange,
	entryBits helpers.BitSet,
	chunkAbsDir string,
	commonJSRef js_ast.Ref,
	esmRef js_ast.Ref,
	toModuleRef js_ast.Ref,
	runtimeRequireRef js_ast.Ref,
	result *compileResultJS,
	dataForSourceMaps []dataForSourceMap,
) {
	file := &c.graph.Files[partRange.sourceIndex]
	repr := file.InputFile.Repr.(*graph.JSRepr)
	nsExportPartIndex := repr.Meta.NSExportPartIndex
	needsWrapper := false
	stmtList := stmtList{}

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

		c.convertStmtsForChunk(partRange.sourceIndex, &stmtList, part.Stmts)
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
	if c.options.MangleSyntax {
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
				cjsArgs = []js_ast.Expr{{Data: &js_ast.EObject{Properties: []js_ast.Property{{
					IsMethod: !c.options.UnsupportedJSFeatures.Has(compat.ObjectExtensions),
					Key:      js_ast.Expr{Data: &js_ast.EString{Value: js_lexer.StringToUTF16(file.InputFile.Source.PrettyPath)}},
					Value:    &js_ast.Expr{Data: &js_ast.EFunction{Fn: js_ast.Fn{Args: args, Body: js_ast.FnBody{Stmts: stmts}}}},
				}}}}}
			} else if c.options.UnsupportedJSFeatures.Has(compat.Arrow) {
				// "__commonJS(function (exports, module) { ... })"
				cjsArgs = []js_ast.Expr{{Data: &js_ast.EFunction{Fn: js_ast.Fn{Args: args, Body: js_ast.FnBody{Stmts: stmts}}}}}
			} else {
				// "__commonJS((exports, module) => { ... })"
				cjsArgs = []js_ast.Expr{{Data: &js_ast.EArrow{Args: args, Body: js_ast.FnBody{Stmts: stmts}}}}
			}
			value := js_ast.Expr{Data: &js_ast.ECall{
				Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: commonJSRef}},
				Args:   cjsArgs,
			}}

			// "var require_foo = __commonJS(...);"
			stmts = append(stmtList.outsideWrapperPrefix, js_ast.Stmt{Data: &js_ast.SLocal{
				Decls: []js_ast.Decl{{
					Binding: js_ast.Binding{Data: &js_ast.BIdentifier{Ref: repr.AST.WrapperRef}},
					Value:   &value,
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
					wrapIdentifier := func(loc logger.Loc, ref js_ast.Ref) js_ast.Expr {
						decls = append(decls, js_ast.Decl{Binding: js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: ref}}})
						return js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: ref}}
					}
					var value js_ast.Expr
					for _, decl := range s.Decls {
						binding := js_ast.ConvertBindingToExpr(decl.Binding, wrapIdentifier)
						if decl.Value != nil {
							value = js_ast.JoinWithComma(value, js_ast.Assign(binding, *decl.Value))
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
				esmArgs = []js_ast.Expr{{Data: &js_ast.EObject{Properties: []js_ast.Property{{
					IsMethod: !c.options.UnsupportedJSFeatures.Has(compat.ObjectExtensions),
					Key:      js_ast.Expr{Data: &js_ast.EString{Value: js_lexer.StringToUTF16(file.InputFile.Source.PrettyPath)}},
					Value:    &js_ast.Expr{Data: &js_ast.EFunction{Fn: js_ast.Fn{Body: js_ast.FnBody{Stmts: stmts}, IsAsync: isAsync}}},
				}}}}}
			} else if c.options.UnsupportedJSFeatures.Has(compat.Arrow) {
				// "__esm(function () { ... })"
				esmArgs = []js_ast.Expr{{Data: &js_ast.EFunction{Fn: js_ast.Fn{Body: js_ast.FnBody{Stmts: stmts}, IsAsync: isAsync}}}}
			} else {
				// "__esm(() => { ... })"
				esmArgs = []js_ast.Expr{{Data: &js_ast.EArrow{Body: js_ast.FnBody{Stmts: stmts}, IsAsync: isAsync}}}
			}
			value := js_ast.Expr{Data: &js_ast.ECall{
				Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: esmRef}},
				Args:   esmArgs,
			}}

			// "var foo, bar;"
			if !c.options.MangleSyntax && len(decls) > 0 {
				stmtList.outsideWrapperPrefix = append(stmtList.outsideWrapperPrefix, js_ast.Stmt{Data: &js_ast.SLocal{
					Decls: decls,
				}})
				decls = nil
			}

			// "var init_foo = __esm(...);"
			stmts = append(stmtList.outsideWrapperPrefix, js_ast.Stmt{Data: &js_ast.SLocal{
				Decls: append(decls, js_ast.Decl{
					Binding: js_ast.Binding{Data: &js_ast.BIdentifier{Ref: repr.AST.WrapperRef}},
					Value:   &value,
				}),
			}})
		}
	}

	// Only generate a source map if needed
	var addSourceMappings bool
	var inputSourceMap *sourcemap.SourceMap
	var lineOffsetTables []js_printer.LineOffsetTable
	if file.InputFile.Loader.CanHaveSourceMap() && c.options.SourceMap != config.SourceMapNone {
		addSourceMappings = true
		inputSourceMap = file.InputFile.InputSourceMap
		lineOffsetTables = dataForSourceMaps[partRange.sourceIndex].lineOffsetTables
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
		RemoveWhitespace:             c.options.RemoveWhitespace,
		MangleSyntax:                 c.options.MangleSyntax,
		ASCIIOnly:                    c.options.ASCIIOnly,
		ToModuleRef:                  toModuleRef,
		RuntimeRequireRef:            runtimeRequireRef,
		LegalComments:                c.options.LegalComments,
		UnsupportedFeatures:          c.options.UnsupportedJSFeatures,
		AddSourceMappings:            addSourceMappings,
		InputSourceMap:               inputSourceMap,
		LineOffsetTables:             lineOffsetTables,
		RequireOrImportMetaForSource: c.requireOrImportMetaForSource,
	}
	tree := repr.AST
	tree.Directive = "" // This is handled elsewhere
	tree.Parts = []js_ast.Part{{Stmts: stmts}}
	*result = compileResultJS{
		PrintResult: js_printer.Print(tree, c.graph.Symbols, r, printOptions),
		sourceIndex: partRange.sourceIndex,
	}

	waitGroup.Done()
}

func (c *linkerContext) generateEntryPointTailJS(
	r renamer.Renamer,
	toModuleRef js_ast.Ref,
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
				stmts = append(stmts, js_ast.Stmt{Data: &js_ast.SReturn{Value: &js_ast.Expr{Data: &js_ast.ECall{
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
			if repr.Meta.ForceIncludeExportsForEntryPoint && len(c.options.GlobalName) > 0 {
				// "return exports;"
				stmts = append(stmts, js_ast.Stmt{Data: &js_ast.SReturn{
					Value: &js_ast.Expr{Data: &js_ast.EIdentifier{Ref: repr.AST.ExportsRef}},
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
		} else if repr.Meta.Wrap == graph.WrapESM {
			// "init_foo();"
			stmts = append(stmts, js_ast.Stmt{Data: &js_ast.SExpr{Value: js_ast.Expr{Data: &js_ast.ECall{
				Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: repr.AST.WrapperRef}},
			}}}})
		}

		// If we are generating CommonJS for node, encode the known export names in
		// a form that node can understand them. This relies on the specific behavior
		// of this parser, which the node project uses to detect named exports in
		// CommonJS files: https://github.com/guybedford/cjs-module-lexer. Think of
		// this code as an annotation for that parser.
		if c.options.Platform == config.PlatformNode && len(repr.Meta.SortedAndFilteredExportAliases) > 0 {
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
			if !c.options.RemoveWhitespace {
				stmts = append(stmts,
					js_ast.Stmt{Data: &js_ast.SComment{Text: `// Annotate the CommonJS export names for ESM import in node:`}},
				)
			}

			// "{a, b, if: null}"
			var moduleExports []js_ast.Property
			for _, export := range repr.Meta.SortedAndFilteredExportAliases {
				if export == "default" {
					// In node the default export is always "module.exports" regardless of
					// what the annotation says. So don't bother generating "default".
					continue
				}

				// "{if: null}"
				var value *js_ast.Expr
				if _, ok := js_lexer.Keywords[export]; ok {
					// Make sure keywords don't cause a syntax error. This has to map to
					// "null" instead of something shorter like "0" because the library
					// "cjs-module-lexer" only supports identifiers in this position, and
					// it thinks "null" is an identifier.
					value = &js_ast.Expr{Data: &js_ast.ENull{}}
				}

				moduleExports = append(moduleExports, js_ast.Property{
					Key:   js_ast.Expr{Data: &js_ast.EString{Value: js_lexer.StringToUTF16(export)}},
					Value: value,
				})
			}

			// "0 && (module.exports = {a, b, if: null});"
			expr := js_ast.Expr{Data: &js_ast.EBinary{
				Op:   js_ast.BinOpLogicalAnd,
				Left: js_ast.Expr{Data: &js_ast.ENumber{Value: 0}},
				Right: js_ast.Assign(
					js_ast.Expr{Data: &js_ast.EDot{
						Target: js_ast.Expr{Data: &js_ast.EIdentifier{Ref: repr.AST.ModuleRef}},
						Name:   "exports",
					}},
					js_ast.Expr{Data: &js_ast.EObject{Properties: moduleExports}},
				),
			}}

			stmts = append(stmts, js_ast.Stmt{Data: &js_ast.SExpr{Value: expr}})
		}

	case config.FormatESModule:
		if repr.Meta.Wrap == graph.WrapCJS {
			// "export default require_foo();"
			stmts = append(stmts, js_ast.Stmt{
				Data: &js_ast.SExportDefault{Value: js_ast.ExprOrStmt{Expr: &js_ast.Expr{
					Data: &js_ast.ECall{Target: js_ast.Expr{
						Data: &js_ast.EIdentifier{Ref: repr.AST.WrapperRef}}}}}}})
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
						//   var cjs_format = __toModule(require_cjs_format());
						//   var export_foo = cjs_format.foo;
						//   export {
						//     export_foo as foo
						//   };
						//
						tempRef := repr.Meta.CJSExportCopies[i]
						stmts = append(stmts, js_ast.Stmt{Data: &js_ast.SLocal{
							Decls: []js_ast.Decl{{
								Binding: js_ast.Binding{Data: &js_ast.BIdentifier{Ref: tempRef}},
								Value:   &js_ast.Expr{Data: &js_ast.EImportIdentifier{Ref: export.Ref}},
							}},
						}})
						items = append(items, js_ast.ClauseItem{
							Name:  js_ast.LocRef{Ref: tempRef},
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
							Name:  js_ast.LocRef{Ref: export.Ref},
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
		RemoveWhitespace:             c.options.RemoveWhitespace,
		MangleSyntax:                 c.options.MangleSyntax,
		ASCIIOnly:                    c.options.ASCIIOnly,
		ToModuleRef:                  toModuleRef,
		LegalComments:                c.options.LegalComments,
		UnsupportedFeatures:          c.options.UnsupportedJSFeatures,
		RequireOrImportMetaForSource: c.requireOrImportMetaForSource,
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

	// These are used to implement bundling, and need to be free for use
	if c.options.Mode != config.ModePassThrough {
		reservedNames["require"] = 1
		reservedNames["Promise"] = 1
	}
	timer.End("Compute reserved names")

	// Make sure imports get a chance to be renamed too
	var sortedImportsFromOtherChunks renamer.StableRefArray
	for _, imports := range chunk.chunkRepr.(*chunkReprJS).importsFromOtherChunks {
		for _, item := range imports {
			sortedImportsFromOtherChunks = append(sortedImportsFromOtherChunks, renamer.StableRef{
				StableSourceIndex: c.graph.StableSourceIndices[item.ref.SourceIndex],
				Ref:               item.ref,
			})
		}
	}
	sort.Sort(sortedImportsFromOtherChunks)

	// Minification uses frequency analysis to give shorter names to more frequent symbols
	if c.options.MinifyIdentifiers {
		// Determine the first top-level slot (i.e. not in a nested scope)
		var firstTopLevelSlots js_ast.SlotCounts
		for _, sourceIndex := range filesInOrder {
			firstTopLevelSlots.UnionMax(c.graph.Files[sourceIndex].InputFile.Repr.(*graph.JSRepr).AST.NestedScopeSlotCounts)
		}
		r := renamer.NewMinifyRenamer(c.graph.Symbols, firstTopLevelSlots, reservedNames)

		// Accumulate symbol usage counts into their slots
		timer.Begin("Accumulate symbol counts")
		freq := js_ast.CharFreq{}
		for _, stable := range sortedImportsFromOtherChunks {
			r.AccumulateSymbolCount(stable.Ref, 1)
		}
		for _, sourceIndex := range filesInOrder {
			repr := c.graph.Files[sourceIndex].InputFile.Repr.(*graph.JSRepr)
			if repr.AST.CharFreq != nil {
				freq.Include(repr.AST.CharFreq)
			}
			if repr.AST.UsesExportsRef {
				r.AccumulateSymbolCount(repr.AST.ExportsRef, 1)
			}
			if repr.AST.UsesModuleRef {
				r.AccumulateSymbolCount(repr.AST.ModuleRef, 1)
			}

			for partIndex, part := range repr.AST.Parts {
				if !repr.AST.Parts[partIndex].IsLive {
					// Skip the part if it's not in this chunk
					continue
				}

				// Accumulate symbol use counts
				r.AccumulateSymbolUseCounts(part.SymbolUses, c.graph.StableSourceIndices)

				// Make sure to also count the declaration in addition to the uses
				for _, declared := range part.DeclaredSymbols {
					r.AccumulateSymbolCount(declared.Ref, 1)
				}
			}
		}
		timer.End("Accumulate symbol counts")

		// Add all of the character frequency histograms for all files in this
		// chunk together, then use it to compute the character sequence used to
		// generate minified names. This results in slightly better gzip compression
		// over assigning minified names in order (i.e. "a b c ..."). Even though
		// it's a very small win, we still do it because it's simple to do and very
		// cheap to compute.
		minifier := freq.Compile()
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
			if c.options.OutputFormat.KeepES6ImportExportSyntax() {
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
		//   __exports(foo_exports, {
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

func (c *linkerContext) generateChunkJS(chunks []chunkInfo, chunkIndex int, chunkWaitGroup *sync.WaitGroup) {
	chunk := &chunks[chunkIndex]

	timer := c.timer.Fork()
	if timer != nil {
		timeName := fmt.Sprintf("Generate chunk %q", path.Clean(config.TemplateToString(chunk.finalTemplate)))
		timer.Begin(timeName)
		defer c.timer.Join(timer)
		defer timer.End(timeName)
	}

	chunkRepr := chunk.chunkRepr.(*chunkReprJS)
	compileResults := make([]compileResultJS, 0, len(chunk.partsInChunkInOrder))
	runtimeMembers := c.graph.Files[runtime.SourceIndex].InputFile.Repr.(*graph.JSRepr).AST.ModuleScope.Members
	commonJSRef := js_ast.FollowSymbols(c.graph.Symbols, runtimeMembers["__commonJS"].Ref)
	esmRef := js_ast.FollowSymbols(c.graph.Symbols, runtimeMembers["__esm"].Ref)
	toModuleRef := js_ast.FollowSymbols(c.graph.Symbols, runtimeMembers["__toModule"].Ref)
	runtimeRequireRef := js_ast.FollowSymbols(c.graph.Symbols, runtimeMembers["__require"].Ref)
	r := c.renameSymbolsInChunk(chunk, chunk.filesInChunkInOrder, timer)
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
	for _, partRange := range chunk.partsInChunkInOrder {
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
			chunk.entryBits,
			chunkAbsDir,
			commonJSRef,
			esmRef,
			toModuleRef,
			runtimeRequireRef,
			compileResult,
			dataForSourceMaps,
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
		printOptions := js_printer.Options{
			Indent:           indent,
			OutputFormat:     c.options.OutputFormat,
			RemoveWhitespace: c.options.RemoveWhitespace,
			MangleSyntax:     c.options.MangleSyntax,
		}
		crossChunkImportRecords := make([]ast.ImportRecord, len(chunk.crossChunkImports))
		for i, chunkImport := range chunk.crossChunkImports {
			crossChunkImportRecords[i] = ast.ImportRecord{
				Kind: chunkImport.importKind,
				Path: logger.Path{Text: chunks[chunkImport.chunkIndex].uniqueKey},
			}
		}
		crossChunkPrefix = js_printer.Print(js_ast.AST{
			ImportRecords: crossChunkImportRecords,
			Parts:         []js_ast.Part{{Stmts: chunkRepr.crossChunkPrefixStmts}},
		}, c.graph.Symbols, r, printOptions).JS
		crossChunkSuffix = js_printer.Print(js_ast.AST{
			Parts: []js_ast.Part{{Stmts: chunkRepr.crossChunkSuffixStmts}},
		}, c.graph.Symbols, r, printOptions).JS
	}

	// Generate the exports for the entry point, if there are any
	var entryPointTail compileResultJS
	if chunk.isEntryPoint {
		entryPointTail = c.generateEntryPointTailJS(
			r,
			toModuleRef,
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
	if c.options.RemoveWhitespace {
		space = ""
		newline = ""
	}
	newlineBeforeComment := false
	isExecutable := false

	if chunk.isEntryPoint {
		repr := c.graph.Files[chunk.sourceIndex].InputFile.Repr.(*graph.JSRepr)

		// Start with the hashbang if there is one
		if repr.AST.Hashbang != "" {
			hashbang := repr.AST.Hashbang + "\n"
			prevOffset.AdvanceString(hashbang)
			j.AddString(hashbang)
			newlineBeforeComment = true
			isExecutable = true
		}

		// Add the top-level directive if present
		if repr.AST.Directive != "" {
			quoted := string(js_printer.QuoteForJSON(repr.AST.Directive, c.options.ASCIIOnly)) + ";" + newline
			prevOffset.AdvanceString(quoted)
			j.AddString(quoted)
			newlineBeforeComment = true
		}
	}

	if len(c.options.JSBanner) > 0 {
		prevOffset.AdvanceString(c.options.JSBanner)
		prevOffset.AdvanceString("\n")
		j.AddString(c.options.JSBanner)
		j.AddString("\n")
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
		for _, chunkImport := range chunk.crossChunkImports {
			if isFirstMeta {
				isFirstMeta = false
			} else {
				jMeta.AddString(",")
			}
			jMeta.AddString(fmt.Sprintf("\n        {\n          \"path\": %s,\n          \"kind\": %s\n        }",
				js_printer.QuoteForJSON(c.res.PrettyPath(logger.Path{Text: chunks[chunkImport.chunkIndex].uniqueKey, Namespace: "file"}), c.options.ASCIIOnly),
				js_printer.QuoteForJSON(chunkImport.importKind.StringForMetafile(), c.options.ASCIIOnly)))
		}
		if !isFirstMeta {
			jMeta.AddString("\n      ")
		}

		// Print exports
		jMeta.AddString("],\n      \"exports\": [")
		var aliases []string
		if c.options.OutputFormat.KeepES6ImportExportSyntax() {
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
				js_printer.QuoteForJSON(alias, c.options.ASCIIOnly)))
		}
		if !isFirstMeta {
			jMeta.AddString("\n      ")
		}
		if chunk.isEntryPoint {
			entryPoint := c.graph.Files[chunk.sourceIndex].InputFile.Source.PrettyPath
			jMeta.AddString(fmt.Sprintf("],\n      \"entryPoint\": %s,\n      \"inputs\": {", js_printer.QuoteForJSON(entryPoint, c.options.ASCIIOnly)))
		} else {
			jMeta.AddString("],\n      \"inputs\": {")
		}
	}

	// Concatenate the generated JavaScript chunks together
	var compileResultsForSourceMap []compileResultJS
	var legalCommentList []string
	var metaOrder []uint32
	var metaByteCount map[string]int
	legalCommentSet := make(map[string]bool)
	prevFileNameComment := uint32(0)
	if c.options.NeedsMetafile {
		metaOrder = make([]uint32, 0, len(compileResults))
		metaByteCount = make(map[string]int, len(compileResults))
	}
	for _, compileResult := range compileResults {
		isRuntime := compileResult.sourceIndex == runtime.SourceIndex
		for text := range compileResult.ExtractedLegalComments {
			if !legalCommentSet[text] {
				legalCommentSet[text] = true
				legalCommentList = append(legalCommentList, text)
			}
		}

		// Add a comment with the file path before the file contents
		if c.options.Mode == config.ModeBundle && !c.options.RemoveWhitespace &&
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
		if isRuntime {
			prevOffset.AdvanceString(string(compileResult.JS))
			j.AddBytes(compileResult.JS)
		} else {
			// Save the offset to the start of the stored JavaScript
			compileResult.generatedOffset = prevOffset
			j.AddBytes(compileResult.JS)

			// Ignore empty source map chunks
			if compileResult.SourceMapChunk.ShouldIgnore {
				prevOffset.AdvanceBytes(compileResult.JS)
			} else {
				prevOffset = sourcemap.LineColumnOffset{}

				// Include this file in the source map
				if c.options.SourceMap != config.SourceMapNone {
					compileResultsForSourceMap = append(compileResultsForSourceMap, compileResult)
				}
			}

			// Include this file in the metadata
			if c.options.NeedsMetafile {
				// Accumulate file sizes since a given file may be split into multiple parts
				path := c.graph.Files[compileResult.sourceIndex].InputFile.Source.PrettyPath
				if count, ok := metaByteCount[path]; ok {
					metaByteCount[path] = count + len(compileResult.JS)
				} else {
					metaOrder = append(metaOrder, compileResult.sourceIndex)
					metaByteCount[path] = len(compileResult.JS)
				}
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

	// Add all unique license comments to the end of the file. These are
	// deduplicated because some projects have thousands of files with the same
	// comment. The comment must be preserved in the output for legal reasons but
	// at the same time we want to generate a small bundle when minifying.
	if len(legalCommentList) > 0 {
		sort.Strings(legalCommentList)

		switch c.options.LegalComments {
		case config.LegalCommentsEndOfFile:
			for _, text := range legalCommentList {
				j.AddString(text)
				j.AddString("\n")
			}

		case config.LegalCommentsLinkedWithComment,
			config.LegalCommentsExternalWithoutComment:
			jComments := helpers.Joiner{}
			for _, text := range legalCommentList {
				jComments.AddString(text)
				jComments.AddString("\n")
			}
			chunk.externalLegalComments = jComments.Done()
		}
	}

	if len(c.options.JSFooter) > 0 {
		j.AddString(c.options.JSFooter)
		j.AddString("\n")
	}

	// The JavaScript contents are done now that the source map comment is in
	jsContents := j.Done()
	timer.End("Join JavaScript files")

	if c.options.SourceMap != config.SourceMapNone {
		timer.Begin("Generate source map")
		chunk.outputSourceMap = c.generateSourceMapForChunk(compileResultsForSourceMap, chunkAbsDir, dataForSourceMaps)
		timer.End("Generate source map")
	}

	// End the metadata lazily. The final output size is not known until the
	// final import paths are substituted into the output pieces generated below.
	if c.options.NeedsMetafile {
		chunk.jsonMetadataChunkCallback = func(finalOutputSize int) []byte {
			isFirstMeta := true
			for _, sourceIndex := range metaOrder {
				if isFirstMeta {
					isFirstMeta = false
				} else {
					jMeta.AddString(",")
				}
				path := c.graph.Files[sourceIndex].InputFile.Source.PrettyPath
				extra := c.generateExtraDataForFileJS(sourceIndex)
				jMeta.AddString(fmt.Sprintf("\n        %s: {\n          \"bytesInOutput\": %d\n        %s}",
					js_printer.QuoteForJSON(path, c.options.ASCIIOnly), metaByteCount[path], extra))
			}
			if !isFirstMeta {
				jMeta.AddString("\n      ")
			}
			jMeta.AddString(fmt.Sprintf("},\n      \"bytes\": %d\n    }", finalOutputSize))
			return jMeta.Done()
		}
	}

	chunk.outputPieces = c.breakOutputIntoPieces(jsContents, uint32(len(chunks)))
	c.generateIsolatedHashInParallel(chunk)
	chunk.isExecutable = isExecutable
	chunkWaitGroup.Done()
}

func (c *linkerContext) generateGlobalNamePrefix() string {
	var text string
	prefix := c.options.GlobalName[0]
	space := " "
	join := ";\n"

	if c.options.RemoveWhitespace {
		space = ""
		join = ";"
	}

	if js_printer.CanQuoteIdentifier(prefix, c.options.UnsupportedJSFeatures, c.options.ASCIIOnly) {
		if c.options.ASCIIOnly {
			prefix = string(js_printer.QuoteIdentifier(nil, prefix, c.options.UnsupportedJSFeatures))
		}
		text = fmt.Sprintf("var %s%s=%s", prefix, space, space)
	} else {
		prefix = fmt.Sprintf("this[%s]", js_printer.QuoteForJSON(prefix, c.options.ASCIIOnly))
		text = fmt.Sprintf("%s%s=%s", prefix, space, space)
	}

	for _, name := range c.options.GlobalName[1:] {
		oldPrefix := prefix
		if js_printer.CanQuoteIdentifier(name, c.options.UnsupportedJSFeatures, c.options.ASCIIOnly) {
			if c.options.ASCIIOnly {
				name = string(js_printer.QuoteIdentifier(nil, name, c.options.UnsupportedJSFeatures))
			}
			prefix = fmt.Sprintf("%s.%s", prefix, name)
		} else {
			prefix = fmt.Sprintf("%s[%s]", prefix, js_printer.QuoteForJSON(name, c.options.ASCIIOnly))
		}
		text += fmt.Sprintf("%s%s||%s{}%s%s%s=%s", oldPrefix, space, space, join, prefix, space, space)
	}

	return text
}

type compileResultCSS struct {
	printedCSS      string
	sourceIndex     uint32
	hasCharset      bool
	externalImports []externalImportCSS
}

type externalImportCSS struct {
	record     ast.ImportRecord
	conditions []css_ast.Token
}

func (c *linkerContext) generateChunkCSS(chunks []chunkInfo, chunkIndex int, chunkWaitGroup *sync.WaitGroup) {
	chunk := &chunks[chunkIndex]

	timer := c.timer.Fork()
	if timer != nil {
		timeName := fmt.Sprintf("Generate chunk %q", path.Clean(config.TemplateToString(chunk.finalTemplate)))
		timer.Begin(timeName)
		defer c.timer.Join(timer)
		defer timer.End(timeName)
	}

	compileResults := make([]compileResultCSS, 0, len(chunk.filesInChunkInOrder))

	// Generate CSS for each file in parallel
	timer.Begin("Print CSS files")
	waitGroup := sync.WaitGroup{}
	for _, sourceIndex := range chunk.filesInChunkInOrder {
		// Create a goroutine for this file
		compileResults = append(compileResults, compileResultCSS{})
		compileResult := &compileResults[len(compileResults)-1]
		waitGroup.Add(1)
		go func(sourceIndex uint32, compileResult *compileResultCSS) {
			file := &c.graph.Files[sourceIndex]
			ast := file.InputFile.Repr.(*graph.CSSRepr).AST

			// Filter out "@import" rules
			rules := make([]css_ast.R, 0, len(ast.Rules))
			for _, rule := range ast.Rules {
				switch r := rule.(type) {
				case *css_ast.RAtCharset:
					compileResult.hasCharset = true
					continue
				case *css_ast.RAtImport:
					if record := ast.ImportRecords[r.ImportRecordIndex]; !record.SourceIndex.IsValid() {
						compileResult.externalImports = append(compileResult.externalImports, externalImportCSS{
							record:     record,
							conditions: r.ImportConditions,
						})
					}
					continue
				}
				rules = append(rules, rule)
			}
			ast.Rules = rules

			compileResult.printedCSS = css_printer.Print(ast, css_printer.Options{
				RemoveWhitespace: c.options.RemoveWhitespace,
				ASCIIOnly:        c.options.ASCIIOnly,
			})
			compileResult.sourceIndex = sourceIndex
			waitGroup.Done()
		}(sourceIndex, compileResult)
	}

	waitGroup.Wait()
	timer.End("Print CSS files")
	timer.Begin("Join CSS files")
	j := helpers.Joiner{}
	newlineBeforeComment := false

	if len(c.options.CSSBanner) > 0 {
		j.AddString(c.options.CSSBanner)
		j.AddString("\n")
	}

	// Generate any prefix rules now
	{
		ast := css_ast.AST{}

		// "@charset" is the only thing that comes before "@import"
		for _, compileResult := range compileResults {
			if compileResult.hasCharset {
				ast.Rules = append(ast.Rules, &css_ast.RAtCharset{Encoding: "UTF-8"})
				break
			}
		}

		// Insert all external "@import" rules at the front. In CSS, all "@import"
		// rules must come first or the browser will just ignore them.
		for _, compileResult := range compileResults {
			for _, external := range compileResult.externalImports {
				ast.Rules = append(ast.Rules, &css_ast.RAtImport{
					ImportRecordIndex: uint32(len(ast.ImportRecords)),
					ImportConditions:  external.conditions,
				})
				ast.ImportRecords = append(ast.ImportRecords, external.record)
			}
		}

		if len(ast.Rules) > 0 {
			css := css_printer.Print(ast, css_printer.Options{
				RemoveWhitespace: c.options.RemoveWhitespace,
			})
			if len(css) > 0 {
				j.AddString(css)
				newlineBeforeComment = true
			}
		}
	}

	// Start the metadata
	jMeta := helpers.Joiner{}
	if c.options.NeedsMetafile {
		isFirstMeta := true
		jMeta.AddString("{\n      \"imports\": [")
		for _, chunkImport := range chunk.crossChunkImports {
			if isFirstMeta {
				isFirstMeta = false
			} else {
				jMeta.AddString(",")
			}
			jMeta.AddString(fmt.Sprintf("\n        {\n          \"path\": %s,\n          \"kind\": %s\n        }",
				js_printer.QuoteForJSON(c.res.PrettyPath(logger.Path{Text: chunks[chunkImport.chunkIndex].uniqueKey, Namespace: "file"}), c.options.ASCIIOnly),
				js_printer.QuoteForJSON(chunkImport.importKind.StringForMetafile(), c.options.ASCIIOnly)))
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
					js_printer.QuoteForJSON(file.InputFile.Source.PrettyPath, c.options.ASCIIOnly)))
			} else {
				jMeta.AddString("],\n      \"inputs\": {")
			}
		} else {
			jMeta.AddString("],\n      \"inputs\": {")
		}
	}
	isFirstMeta := true

	// Concatenate the generated CSS chunks together
	for _, compileResult := range compileResults {
		if c.options.Mode == config.ModeBundle && !c.options.RemoveWhitespace {
			if newlineBeforeComment {
				j.AddString("\n")
			}
			j.AddString(fmt.Sprintf("/* %s */\n", c.graph.Files[compileResult.sourceIndex].InputFile.Source.PrettyPath))
		}
		if len(compileResult.printedCSS) > 0 {
			newlineBeforeComment = true
		}
		j.AddString(compileResult.printedCSS)

		// Include this file in the metadata
		if c.options.NeedsMetafile {
			if isFirstMeta {
				isFirstMeta = false
			} else {
				jMeta.AddString(",")
			}
			jMeta.AddString(fmt.Sprintf("\n        %s: {\n          \"bytesInOutput\": %d\n        }",
				js_printer.QuoteForJSON(c.graph.Files[compileResult.sourceIndex].InputFile.Source.PrettyPath, c.options.ASCIIOnly),
				len(compileResult.printedCSS)))
		}
	}

	// Make sure the file ends with a newline
	j.EnsureNewlineAtEnd()

	if len(c.options.CSSFooter) > 0 {
		j.AddString(c.options.CSSFooter)
		j.AddString("\n")
	}

	// The CSS contents are done now that the source map comment is in
	cssContents := j.Done()
	timer.End("Join CSS files")

	// End the metadata lazily. The final output size is not known until the
	// final import paths are substituted into the output pieces generated below.
	if c.options.NeedsMetafile {
		chunk.jsonMetadataChunkCallback = func(finalOutputSize int) []byte {
			if !isFirstMeta {
				jMeta.AddString("\n      ")
			}
			jMeta.AddString(fmt.Sprintf("},\n      \"bytes\": %d\n    }", finalOutputSize))
			return jMeta.Done()
		}
	}

	chunk.outputPieces = c.breakOutputIntoPieces(cssContents, uint32(len(chunks)))
	c.generateIsolatedHashInParallel(chunk)
	chunkWaitGroup.Done()
}

func appendIsolatedHashesForImportedChunks(
	hash hash.Hash,
	chunks []chunkInfo,
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
	chunk := &chunks[chunkIndex]

	// Visit the other chunks that this chunk imports before visiting this chunk
	for _, chunkImport := range chunk.crossChunkImports {
		appendIsolatedHashesForImportedChunks(hash, chunks, chunkImport.chunkIndex, visited, visitedKey)
	}

	// Mix in the hash for this chunk
	hash.Write(chunk.waitForIsolatedHash())
}

func (c *linkerContext) breakOutputIntoPieces(output []byte, chunkCount uint32) []outputPiece {
	var pieces []outputPiece
	prefix := c.uniqueKeyPrefixBytes
	for {
		// Scan for the next chunk path
		boundary := bytes.Index(output, prefix)

		// Try to parse the chunk index
		var chunkIndex uint32
		if boundary != -1 {
			if start := boundary + len(prefix); start+8 > len(output) {
				boundary = -1
			} else {
				for j := 0; j < 8; j++ {
					c := output[start+j]
					if c < '0' || c > '9' {
						boundary = -1
						break
					}
					chunkIndex = chunkIndex*10 + uint32(c) - '0'
				}
			}
			if chunkIndex >= chunkCount {
				boundary = -1
			}
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
			data:       output[:boundary],
			chunkIndex: ast.MakeIndex32(chunkIndex),
		})
		output = output[boundary+len(prefix)+8:]
	}
	return pieces
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
	for _, partRange := range chunk.partsInChunkInOrder {
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

	// Hash the output path template as part of the content hash because we want
	// any import to be considered different if the import's output path has changed.
	for _, part := range chunk.finalTemplate {
		hashWriteLengthPrefixed(hash, []byte(part.Data))
	}

	// Include the generated output content in the hash. This excludes the
	// randomly-generated import paths (the unique keys) and only includes the
	// data in the spans between them.
	for _, piece := range chunk.outputPieces {
		hashWriteLengthPrefixed(hash, piece.data)
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

func preventBindingsFromBeingRenamed(binding js_ast.Binding, symbols js_ast.SymbolMap) {
	switch b := binding.Data.(type) {
	case *js_ast.BMissing:

	case *js_ast.BIdentifier:
		symbols.Get(b.Ref).MustNotBeRenamed = true

	case *js_ast.BArray:
		for _, i := range b.Items {
			preventBindingsFromBeingRenamed(i.Binding, symbols)
		}

	case *js_ast.BObject:
		for _, p := range b.Properties {
			preventBindingsFromBeingRenamed(p.Value, symbols)
		}

	default:
		panic(fmt.Sprintf("Unexpected binding of type %T", binding.Data))
	}
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
				// Ignore imports from the internal runtime code. These are generated
				// automatically and aren't part of the original source code. We
				// shouldn't consider the file a module if the only ES6 import or
				// export is the automatically generated one.
				record := &repr.AST.ImportRecords[s.ImportRecordIndex]
				if record.SourceIndex.IsValid() && record.SourceIndex.GetIndex() == runtime.SourceIndex {
					continue
				}

				hasImportOrExport = true

			case *js_ast.SLocal:
				if s.IsExport {
					for _, decl := range s.Decls {
						preventBindingsFromBeingRenamed(decl.Binding, c.graph.Symbols)
					}
					hasImportOrExport = true
				}

			case *js_ast.SFunction:
				if s.IsExport {
					c.graph.Symbols.Get(s.Fn.Name.Ref).Kind = js_ast.SymbolUnbound
					hasImportOrExport = true
				}

			case *js_ast.SClass:
				if s.IsExport {
					c.graph.Symbols.Get(s.Class.Name.Ref).Kind = js_ast.SymbolUnbound
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
			c.graph.Symbols.Get(member.Ref).MustNotBeRenamed = true
		}
	}
}

func (c *linkerContext) generateSourceMapForChunk(
	results []compileResultJS,
	chunkAbsDir string,
	dataForSourceMaps []dataForSourceMap,
) (pieces sourcemap.SourceMapPieces) {
	j := helpers.Joiner{}
	j.AddString("{\n  \"version\": 3")

	// Only write out the sources for a given source index once
	sourceIndexToSourcesIndex := make(map[uint32]int)

	// Generate the "sources" and "sourcesContent" arrays
	type item struct {
		path           logger.Path
		prettyPath     string
		quotedContents []byte
	}
	items := make([]item, 0, len(results))
	nextSourcesIndex := 0
	for _, result := range results {
		if _, ok := sourceIndexToSourcesIndex[result.sourceIndex]; ok {
			continue
		}
		sourceIndexToSourcesIndex[result.sourceIndex] = nextSourcesIndex
		file := &c.graph.Files[result.sourceIndex]

		// Simple case: no nested source map
		if file.InputFile.InputSourceMap == nil {
			var quotedContents []byte
			if !c.options.ExcludeSourcesContent {
				quotedContents = dataForSourceMaps[result.sourceIndex].quotedContents[0]
			}
			items = append(items, item{
				path:           file.InputFile.Source.KeyPath,
				prettyPath:     file.InputFile.Source.PrettyPath,
				quotedContents: quotedContents,
			})
			nextSourcesIndex++
			continue
		}

		// Complex case: nested source map
		sm := file.InputFile.InputSourceMap
		for i, source := range sm.Sources {
			path := logger.Path{
				Namespace: file.InputFile.Source.KeyPath.Namespace,
				Text:      source,
			}

			// If this file is in the "file" namespace, change the relative path in
			// the source map into an absolute path using the directory of this file
			if path.Namespace == "file" {
				path.Text = c.fs.Join(c.fs.Dir(file.InputFile.Source.KeyPath.Text), source)
			}

			var quotedContents []byte
			if !c.options.ExcludeSourcesContent {
				quotedContents = dataForSourceMaps[result.sourceIndex].quotedContents[i]
			}
			items = append(items, item{
				path:           path,
				prettyPath:     source,
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
		if item.path.Namespace == "file" {
			if relPath, ok := c.fs.Rel(chunkAbsDir, item.path.Text); ok {
				// Make sure to always use forward slashes, even on Windows
				item.prettyPath = strings.ReplaceAll(relPath, "\\", "/")
			}
		}

		j.AddBytes(js_printer.QuoteForJSON(item.prettyPath, c.options.ASCIIOnly))
	}
	j.AddString("]")

	if c.options.SourceRoot != "" {
		j.AddString(",\n  \"sourceRoot\": ")
		j.AddBytes(js_printer.QuoteForJSON(c.options.SourceRoot, c.options.ASCIIOnly))
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
	pieces.Prefix = j.Done()

	// Write the mappings
	jMappings := helpers.Joiner{}
	prevEndState := js_printer.SourceMapState{}
	prevColumnOffset := 0
	for _, result := range results {
		chunk := result.SourceMapChunk
		offset := result.generatedOffset
		sourcesIndex := sourceIndexToSourcesIndex[result.sourceIndex]

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
			SourceIndex:     sourcesIndex,
			GeneratedLine:   offset.Lines,
			GeneratedColumn: offset.Columns,
		}
		if offset.Lines == 0 {
			startState.GeneratedColumn += prevColumnOffset
		}

		// Append the precomputed source map chunk
		js_printer.AppendSourceMapChunk(&jMappings, prevEndState, startState, chunk.Buffer)

		// Generate the relative offset to start from next time
		prevEndState = chunk.EndState
		prevEndState.SourceIndex += sourcesIndex
		prevColumnOffset = chunk.FinalGeneratedColumn

		// If this was all one line, include the column offset from the start
		if prevEndState.GeneratedLine == 0 {
			prevEndState.GeneratedColumn += startState.GeneratedColumn
			prevColumnOffset += startState.GeneratedColumn
		}
	}
	pieces.Mappings = jMappings.Done()

	// Finish the source map
	pieces.Suffix = []byte("\",\n  \"names\": []\n}\n")
	return
}
