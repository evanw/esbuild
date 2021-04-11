package graph

import (
	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/js_ast"
)

type EntryPointKind uint8

const (
	EntryPointNone EntryPointKind = iota
	EntryPointUserSpecified
	EntryPointDynamicImport
)

type LinkerFile struct {
	InputFile InputFile

	// This holds all entry points that can reach this file. It will be used to
	// assign the parts in this file to a chunk.
	EntryBits helpers.BitSet

	// The minimum number of links in the module graph to get from an entry point
	// to this file
	DistanceFromEntryPoint uint32

	// If "entryPointKind" is not "entryPointNone", this is the index of the
	// corresponding entry point chunk.
	EntryPointChunkIndex uint32

	// This file is an entry point if and only if this is not "entryPointNone".
	// Note that dynamically-imported files are allowed to also be specified by
	// the user as top-level entry points, so some dynamically-imported files
	// may be "entryPointUserSpecified" instead of "entryPointDynamicImport".
	EntryPointKind EntryPointKind

	// This is true if this file has been marked as live by the tree shaking
	// algorithm.
	IsLive bool
}

func (f *LinkerFile) IsEntryPoint() bool {
	return f.EntryPointKind != EntryPointNone
}

type LinkerGraph struct {
	Files   []LinkerFile
	Symbols js_ast.SymbolMap

	// We should avoid traversing all files in the bundle, because the linker
	// should be able to run a linking operation on a large bundle where only
	// a few files are needed (e.g. an incremental compilation scenario). This
	// holds all files that could possibly be reached through the entry points.
	// If you need to iterate over all files in the linking operation, iterate
	// over this array. This array is also sorted in a deterministic ordering
	// to help ensure deterministic builds (source indices are random).
	ReachableFiles []uint32

	// This maps from unstable source index to stable reachable file index. This
	// is useful as a deterministic key for sorting if you need to sort something
	// containing a source index (such as "js_ast.Ref" symbol references).
	StableSourceIndices []uint32
}

func MakeLinkerGraph(
	inputFiles []InputFile,
	reachableFiles []uint32,
) LinkerGraph {
	symbols := js_ast.NewSymbolMap(len(inputFiles))
	files := make([]LinkerFile, len(inputFiles))

	// Clone various things since we may mutate them later
	for _, sourceIndex := range reachableFiles {
		file := LinkerFile{
			InputFile: inputFiles[sourceIndex],
		}

		switch repr := file.InputFile.Repr.(type) {
		case *JSRepr:
			// Clone the representation
			{
				clone := *repr
				repr = &clone
				file.InputFile.Repr = repr
			}

			// Clone the symbol map
			fileSymbols := append([]js_ast.Symbol{}, repr.AST.Symbols...)
			symbols.SymbolsForSource[sourceIndex] = fileSymbols
			repr.AST.Symbols = nil

			// Clone the parts
			repr.AST.Parts = append([]js_ast.Part{}, repr.AST.Parts...)
			for i := range repr.AST.Parts {
				part := &repr.AST.Parts[i]
				clone := make(map[js_ast.Ref]js_ast.SymbolUse, len(part.SymbolUses))
				for ref, uses := range part.SymbolUses {
					clone[ref] = uses
				}
				part.SymbolUses = clone
				part.Dependencies = append([]js_ast.Dependency{}, part.Dependencies...)
			}

			// Clone the import records
			repr.AST.ImportRecords = append([]ast.ImportRecord{}, repr.AST.ImportRecords...)

			// Clone the import map
			namedImports := make(map[js_ast.Ref]js_ast.NamedImport, len(repr.AST.NamedImports))
			for k, v := range repr.AST.NamedImports {
				namedImports[k] = v
			}
			repr.AST.NamedImports = namedImports

			// Clone the export map
			resolvedExports := make(map[string]ExportData)
			for alias, name := range repr.AST.NamedExports {
				resolvedExports[alias] = ExportData{
					Ref:         name.Ref,
					SourceIndex: sourceIndex,
					NameLoc:     name.AliasLoc,
				}
			}

			// Clone the top-level symbol-to-parts map
			topLevelSymbolToParts := make(map[js_ast.Ref][]uint32)
			for ref, parts := range repr.AST.TopLevelSymbolToParts {
				topLevelSymbolToParts[ref] = parts
			}
			repr.AST.TopLevelSymbolToParts = topLevelSymbolToParts

			// Clone the top-level scope so we can generate more variables
			{
				new := &js_ast.Scope{}
				*new = *repr.AST.ModuleScope
				new.Generated = append([]js_ast.Ref{}, new.Generated...)
				repr.AST.ModuleScope = new
			}

			// Also associate some default metadata with the file
			repr.Meta.ResolvedExports = resolvedExports
			repr.Meta.IsProbablyTypeScriptType = make(map[js_ast.Ref]bool)
			repr.Meta.ImportsToBind = make(map[js_ast.Ref]ImportData)

		case *CSSRepr:
			// Clone the representation
			{
				clone := *repr
				repr = &clone
				file.InputFile.Repr = repr
			}

			// Clone the import records
			repr.AST.ImportRecords = append([]ast.ImportRecord{}, repr.AST.ImportRecords...)
		}

		// All files start off as far as possible from an entry point
		file.DistanceFromEntryPoint = ^uint32(0)

		// Update the file in our copy of the file array
		files[sourceIndex] = file
	}

	// Create a way to convert source indices to a stable ordering
	stableSourceIndices := make([]uint32, len(inputFiles))
	for stableIndex, sourceIndex := range reachableFiles {
		stableSourceIndices[sourceIndex] = uint32(stableIndex)
	}

	return LinkerGraph{
		Symbols:             symbols,
		Files:               files,
		ReachableFiles:      reachableFiles,
		StableSourceIndices: stableSourceIndices,
	}
}
