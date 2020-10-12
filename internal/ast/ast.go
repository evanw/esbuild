package ast

import "github.com/evanw/esbuild/internal/logger"

// This file contains data structures that are used with the AST packages for
// both JavaScript and CSS. This helps the bundler treat both AST formats in
// a somewhat format-agnostic manner.

type ImportKind uint8

const (
	// An ES6 import or re-export statement
	ImportStmt ImportKind = iota

	// A call to "require()"
	ImportRequire

	// An "import()" expression with a string argument
	ImportDynamic

	// A call to "require.resolve()"
	ImportRequireResolve

	// A CSS "@import" rule
	AtImport

	// A CSS "url(...)" token
	URLToken
)

func (kind ImportKind) IsFromCSS() bool {
	return kind == AtImport || kind == URLToken
}

type ImportRecord struct {
	Range logger.Range
	Path  logger.Path

	// The resolved source index for an internal import (within the bundle) or
	// nil for an external import (not included in the bundle)
	SourceIndex *uint32

	// Sometimes the parser creates an import record and decides it isn't needed.
	// For example, TypeScript code may have import statements that later turn
	// out to be type-only imports after analyzing the whole file.
	IsUnused bool

	// If this is true, the import contains syntax like "* as ns". This is used
	// to determine whether modules that have no exports need to be wrapped in a
	// CommonJS wrapper or not.
	ContainsImportStar bool

	// If true, this "export * from 'path'" statement is evaluated at run-time by
	// calling the "__exportStar()" helper function
	CallsRunTimeExportStarFn bool

	// Tell the printer to wrap this call to "require()" in "__toModule(...)"
	WrapWithToModule bool

	// True for require calls like this: "try { require() } catch {}". In this
	// case we shouldn't generate an error if the path could not be resolved.
	IsInsideTryBody bool

	Kind ImportKind
}
