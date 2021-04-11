package graph

import "github.com/evanw/esbuild/internal/helpers"

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
