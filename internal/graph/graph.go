package graph

import (
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

	// The minimum number of links in the module graph to get from an entry point
	// to this file
	DistanceFromEntryPoint uint32

	// This is true if this file has been marked as live by the tree shaking
	// algorithm.
	IsLive bool

	// This holds all entry points that can reach this file. It will be used to
	// assign the parts in this file to a chunk.
	EntryBits helpers.BitSet

	// If "entryPointKind" is not "entryPointNone", this is the index of the
	// corresponding entry point chunk.
	EntryPointChunkIndex uint32

	// This file is an entry point if and only if this is not "entryPointNone".
	// Note that dynamically-imported files are allowed to also be specified by
	// the user as top-level entry points, so some dynamically-imported files
	// may be "entryPointUserSpecified" instead of "entryPointDynamicImport".
	EntryPointKind EntryPointKind
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
