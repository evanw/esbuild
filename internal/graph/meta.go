package graph

import (
	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/logger"
)

type WrapKind uint8

const (
	WrapNone WrapKind = iota

	// The module will be bundled CommonJS-style like this:
	//
	//   // foo.ts
	//   let require_foo = __commonJS((exports, module) => {
	//     exports.foo = 123;
	//   });
	//
	//   // bar.ts
	//   let foo = flag ? require_foo() : null;
	//
	WrapCJS

	// The module will be bundled ESM-style like this:
	//
	//   // foo.ts
	//   var foo, foo_exports = {};
	//   __exports(foo_exports, {
	//     foo: () => foo
	//   });
	//   let init_foo = __esm(() => {
	//     foo = 123;
	//   });
	//
	//   // bar.ts
	//   let foo = flag ? (init_foo(), foo_exports) : null;
	//
	WrapESM
)

// This contains linker-specific metadata corresponding to a "file" struct
// from the initial scan phase of the bundler. It's separated out because it's
// conceptually only used for a single linking operation and because multiple
// linking operations may be happening in parallel with different metadata for
// the same file.
type JSReprMeta struct {
	PartMeta []PartMeta

	// This is the index to the automatically-generated part containing code that
	// calls "__export(exports, { ... getters ... })". This is used to generate
	// getters on an exports object for ES6 export statements, and is both for
	// ES6 star imports and CommonJS-style modules.
	NSExportPartIndex uint32

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
	IsProbablyTypeScriptType map[js_ast.Ref]bool

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
	ImportsToBind map[js_ast.Ref]ImportData

	IsAsyncOrHasAsyncDependency bool
	DependsOnRuntimeSymbol      bool
	Wrap                        WrapKind

	// If true, the "__export(exports, { ... })" call will be force-included even
	// if there are no parts that reference "exports". Otherwise this call will
	// be removed due to the tree shaking pass. This is used when for entry point
	// files when code related to the current output format needs to reference
	// the "exports" variable.
	ForceIncludeExportsForEntryPoint bool

	// This is set when we need to pull in the "__export" symbol in to the part
	// at "nsExportPartIndex". This can't be done in "createExportsForFile"
	// because of concurrent map hazards. Instead, it must be done later.
	NeedsExportSymbolFromRuntime       bool
	NeedsMarkAsModuleSymbolFromRuntime bool

	// The index of the automatically-generated part used to represent the
	// CommonJS or ESM wrapper. This part is empty and is only useful for tree
	// shaking and code splitting. The wrapper can't be inserted into the part
	// because the wrapper contains other parts, which can't be represented by
	// the current part system.
	WrapperPartIndex ast.Index32

	// This includes both named exports and re-exports.
	//
	// Named exports come from explicit export statements in the original file,
	// and are copied from the "NamedExports" field in the AST.
	//
	// Re-exports come from other files and are the result of resolving export
	// star statements (i.e. "export * from 'foo'").
	ResolvedExports    map[string]ExportData
	ResolvedExportStar *ExportData

	// Never iterate over "resolvedExports" directly. Instead, iterate over this
	// array. Some exports in that map aren't meant to end up in generated code.
	// This array excludes these exports and is also sorted, which avoids non-
	// determinism due to random map iteration order.
	SortedAndFilteredExportAliases []string

	// If this is an entry point, this array holds a reference to one free
	// temporary symbol for each entry in "sortedAndFilteredExportAliases".
	// These may be needed to store copies of CommonJS re-exports in ESM.
	CJSExportCopies []js_ast.Ref
}

type ImportData struct {
	// This is an array of intermediate statements that re-exported this symbol
	// in a chain before getting to the final symbol. This can be done either with
	// "export * from" or "export {} from". If this is done with "export * from"
	// then this may not be the result of a single chain but may instead form
	// a diamond shape if this same symbol was re-exported multiple times from
	// different files.
	ReExports []js_ast.Dependency

	SourceIndex uint32
	NameLoc     logger.Loc // Optional, goes with sourceIndex, ignore if zero
	Ref         js_ast.Ref
}

type ExportData struct {
	Ref js_ast.Ref

	// Export star resolution happens first before import resolution. That means
	// it cannot yet determine if duplicate names from export star resolution are
	// ambiguous (point to different symbols) or not (point to the same symbol).
	// This issue can happen in the following scenario:
	//
	//   // entry.js
	//   export * from './a'
	//   export * from './b'
	//
	//   // a.js
	//   export * from './c'
	//
	//   // b.js
	//   export {x} from './c'
	//
	//   // c.js
	//   export let x = 1, y = 2
	//
	// In this case "entry.js" should have two exports "x" and "y", neither of
	// which are ambiguous. To handle this case, ambiguity resolution must be
	// deferred until import resolution time. That is done using this array.
	PotentiallyAmbiguousExportStarRefs []ImportData

	// This is the file that the named export above came from. This will be
	// different from the file that contains this object if this is a re-export.
	SourceIndex uint32
	NameLoc     logger.Loc // Optional, goes with sourceIndex, ignore if zero
}

// This contains linker-specific metadata corresponding to a "js_ast.Part" struct
// from the initial scan phase of the bundler. It's separated out because it's
// conceptually only used for a single linking operation and because multiple
// linking operations may be happening in parallel with different metadata for
// the same part in the same file.
type PartMeta struct {
	// This is true if this file has been marked as live by the tree shaking
	// algorithm.
	IsLive bool
}
