package ast

// This file contains data structures that are used with the AST packages for
// both JavaScript and CSS. This helps the bundler treat both AST formats in
// a somewhat format-agnostic manner.

import (
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/logger"
)

type ImportKind uint8

const (
	// An entry point provided by the user
	ImportEntryPoint ImportKind = iota

	// An ES6 import or re-export statement
	ImportStmt

	// A call to "require()"
	ImportRequire

	// An "import()" expression with a string argument
	ImportDynamic

	// A call to "require.resolve()"
	ImportRequireResolve

	// A CSS "@import" rule
	ImportAt

	// A CSS "@import" rule with import conditions
	ImportAtConditional

	// A CSS "url(...)" token
	ImportURL
)

func (kind ImportKind) StringForMetafile() string {
	switch kind {
	case ImportStmt:
		return "import-statement"
	case ImportRequire:
		return "require-call"
	case ImportDynamic:
		return "dynamic-import"
	case ImportRequireResolve:
		return "require-resolve"
	case ImportAt, ImportAtConditional:
		return "import-rule"
	case ImportURL:
		return "url-token"
	case ImportEntryPoint:
		return "entry-point"
	default:
		panic("Internal error")
	}
}

func (kind ImportKind) IsFromCSS() bool {
	return kind == ImportAt || kind == ImportURL
}

type ImportRecordFlags uint16

const (
	// Sometimes the parser creates an import record and decides it isn't needed.
	// For example, TypeScript code may have import statements that later turn
	// out to be type-only imports after analyzing the whole file.
	IsUnused ImportRecordFlags = 1 << iota

	// If this is true, the import contains syntax like "* as ns". This is used
	// to determine whether modules that have no exports need to be wrapped in a
	// CommonJS wrapper or not.
	ContainsImportStar

	// If this is true, the import contains an import for the alias "default",
	// either via the "import x from" or "import {default as x} from" syntax.
	ContainsDefaultAlias

	// If this is true, the import contains an import for the alias "__esModule",
	// via the "import {__esModule} from" syntax.
	ContainsESModuleAlias

	// If true, this "export * from 'path'" statement is evaluated at run-time by
	// calling the "__reExport()" helper function
	CallsRunTimeReExportFn

	// Tell the printer to wrap this call to "require()" in "__toESM(...)"
	WrapWithToESM

	// Tell the printer to wrap this ESM exports object in "__toCJS(...)"
	WrapWithToCJS

	// Tell the printer to use the runtime "__require()" instead of "require()"
	CallRuntimeRequire

	// True for the following cases:
	//
	//   try { require('x') } catch { handle }
	//   try { await import('x') } catch { handle }
	//   try { require.resolve('x') } catch { handle }
	//   import('x').catch(handle)
	//   import('x').then(_, handle)
	//
	// In these cases we shouldn't generate an error if the path could not be
	// resolved.
	HandlesImportErrors

	// If true, this was originally written as a bare "import 'file'" statement
	WasOriginallyBareImport

	// If true, this import can be removed if it's unused
	IsExternalWithoutSideEffects

	// If true, "assert { type: 'json' }" was present
	AssertTypeJSON

	// If true, do not generate "external": true in the metafile
	ShouldNotBeExternalInMetafile

	// CSS "@import" of an empty file should be removed
	WasLoadedWithEmptyLoader
)

func (flags ImportRecordFlags) Has(flag ImportRecordFlags) bool {
	return (flags & flag) != 0
}

type ImportRecord struct {
	Assertions *ImportAssertions
	Path       logger.Path
	Range      logger.Range

	// If the "HandlesImportErrors" flag is present, then this is the location
	// of the error handler. This is used for error reporting.
	ErrorHandlerLoc logger.Loc

	// The resolved source index for an internal import (within the bundle) or
	// invalid for an external import (not included in the bundle)
	SourceIndex Index32

	// Files imported via the "copy" loader use this instead of "SourceIndex"
	// because they are sort of like external imports, and are not bundled.
	CopySourceIndex Index32

	Flags ImportRecordFlags
	Kind  ImportKind
}

type ImportAssertions struct {
	Entries            []AssertEntry
	AssertLoc          logger.Loc
	InnerOpenBraceLoc  logger.Loc
	InnerCloseBraceLoc logger.Loc
	OuterOpenBraceLoc  logger.Loc
	OuterCloseBraceLoc logger.Loc
}

type AssertEntry struct {
	Key             []uint16 // An identifier or a string
	Value           []uint16 // Always a string
	KeyLoc          logger.Loc
	ValueLoc        logger.Loc
	PreferQuotedKey bool
}

func FindAssertion(assertions []AssertEntry, name string) *AssertEntry {
	for _, assertion := range assertions {
		if helpers.UTF16EqualsString(assertion.Key, name) {
			return &assertion
		}
	}
	return nil
}

// This stores a 32-bit index where the zero value is an invalid index. This is
// a better alternative to storing the index as a pointer since that has the
// same properties but takes up more space and costs an extra pointer traversal.
type Index32 struct {
	flippedBits uint32
}

func MakeIndex32(index uint32) Index32 {
	return Index32{flippedBits: ^index}
}

func (i Index32) IsValid() bool {
	return i.flippedBits != 0
}

func (i Index32) GetIndex() uint32 {
	return ^i.flippedBits
}
