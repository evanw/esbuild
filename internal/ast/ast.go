package ast

// This file contains data structures that are used with the AST packages for
// both JavaScript and CSS. This helps the bundler treat both AST formats in
// a somewhat format-agnostic manner.

import (
	"sort"

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

	// A CSS "composes" declaration
	ImportComposesFrom

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
	case ImportAt:
		return "import-rule"
	case ImportComposesFrom:
		return "composes-from"
	case ImportURL:
		return "url-token"
	case ImportEntryPoint:
		return "entry-point"
	default:
		panic("Internal error")
	}
}

func (kind ImportKind) IsFromCSS() bool {
	switch kind {
	case ImportAt, ImportComposesFrom, ImportURL:
		return true
	}
	return false
}

func (kind ImportKind) MustResolveToCSS() bool {
	switch kind {
	case ImportAt, ImportComposesFrom:
		return true
	}
	return false
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

	// Unique keys are randomly-generated strings that are used to replace paths
	// in the source code after it's printed. These must not ever be split apart.
	ContainsUniqueKey
)

func (flags ImportRecordFlags) Has(flag ImportRecordFlags) bool {
	return (flags & flag) != 0
}

type ImportRecord struct {
	AssertOrWith *ImportAssertOrWith
	GlobPattern  *GlobPattern
	Path         logger.Path
	Range        logger.Range

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

type AssertOrWithKeyword uint8

const (
	AssertKeyword AssertOrWithKeyword = iota
	WithKeyword
)

func (kw AssertOrWithKeyword) String() string {
	if kw == AssertKeyword {
		return "assert"
	}
	return "with"
}

type ImportAssertOrWith struct {
	Entries            []AssertOrWithEntry
	KeywordLoc         logger.Loc
	InnerOpenBraceLoc  logger.Loc
	InnerCloseBraceLoc logger.Loc
	OuterOpenBraceLoc  logger.Loc
	OuterCloseBraceLoc logger.Loc
	Keyword            AssertOrWithKeyword
}

type AssertOrWithEntry struct {
	Key             []uint16 // An identifier or a string
	Value           []uint16 // Always a string
	KeyLoc          logger.Loc
	ValueLoc        logger.Loc
	PreferQuotedKey bool
}

func FindAssertOrWithEntry(assertions []AssertOrWithEntry, name string) *AssertOrWithEntry {
	for _, assertion := range assertions {
		if helpers.UTF16EqualsString(assertion.Key, name) {
			return &assertion
		}
	}
	return nil
}

type GlobPattern struct {
	Parts       []helpers.GlobPart
	ExportAlias string
	Kind        ImportKind
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

type SymbolKind uint8

const (
	// An unbound symbol is one that isn't declared in the file it's referenced
	// in. For example, using "window" without declaring it will be unbound.
	SymbolUnbound SymbolKind = iota

	// This has special merging behavior. You're allowed to re-declare these
	// symbols more than once in the same scope. These symbols are also hoisted
	// out of the scope they are declared in to the closest containing function
	// or module scope. These are the symbols with this kind:
	//
	// - Function arguments
	// - Function statements
	// - Variables declared using "var"
	//
	SymbolHoisted
	SymbolHoistedFunction

	// There's a weird special case where catch variables declared using a simple
	// identifier (i.e. not a binding pattern) block hoisted variables instead of
	// becoming an error:
	//
	//   var e = 0;
	//   try { throw 1 } catch (e) {
	//     print(e) // 1
	//     var e = 2
	//     print(e) // 2
	//   }
	//   print(e) // 0 (since the hoisting stops at the catch block boundary)
	//
	// However, other forms are still a syntax error:
	//
	//   try {} catch (e) { let e }
	//   try {} catch ({e}) { var e }
	//
	// This symbol is for handling this weird special case.
	SymbolCatchIdentifier

	// Generator and async functions are not hoisted, but still have special
	// properties such as being able to overwrite previous functions with the
	// same name
	SymbolGeneratorOrAsyncFunction

	// This is the special "arguments" variable inside functions
	SymbolArguments

	// Classes can merge with TypeScript namespaces.
	SymbolClass

	// Class names are not allowed to be referenced by computed property keys
	SymbolClassInComputedPropertyKey

	// A class-private identifier (i.e. "#foo").
	SymbolPrivateField
	SymbolPrivateMethod
	SymbolPrivateGet
	SymbolPrivateSet
	SymbolPrivateGetSetPair
	SymbolPrivateStaticField
	SymbolPrivateStaticMethod
	SymbolPrivateStaticGet
	SymbolPrivateStaticSet
	SymbolPrivateStaticGetSetPair

	// Labels are in their own namespace
	SymbolLabel

	// TypeScript enums can merge with TypeScript namespaces and other TypeScript
	// enums.
	SymbolTSEnum

	// TypeScript namespaces can merge with classes, functions, TypeScript enums,
	// and other TypeScript namespaces.
	SymbolTSNamespace

	// In TypeScript, imports are allowed to silently collide with symbols within
	// the module. Presumably this is because the imports may be type-only.
	SymbolImport

	// Assigning to a "const" symbol will throw a TypeError at runtime
	SymbolConst

	// Injected symbols can be overridden by provided defines
	SymbolInjected

	// Properties can optionally be renamed to shorter names
	SymbolMangledProp

	// CSS identifiers that are never renamed
	SymbolGlobalCSS

	// CSS identifiers that are renamed to be unique to the file they are in
	SymbolLocalCSS

	// This annotates all other symbols that don't have special behavior
	SymbolOther
)

func (kind SymbolKind) IsPrivate() bool {
	return kind >= SymbolPrivateField && kind <= SymbolPrivateStaticGetSetPair
}

func (kind SymbolKind) IsHoisted() bool {
	return kind == SymbolHoisted || kind == SymbolHoistedFunction
}

func (kind SymbolKind) IsHoistedOrFunction() bool {
	return kind.IsHoisted() || kind == SymbolGeneratorOrAsyncFunction
}

func (kind SymbolKind) IsFunction() bool {
	return kind == SymbolHoistedFunction || kind == SymbolGeneratorOrAsyncFunction
}

func (kind SymbolKind) IsUnboundOrInjected() bool {
	return kind == SymbolUnbound || kind == SymbolInjected
}

var InvalidRef Ref = Ref{^uint32(0), ^uint32(0)}

// Files are parsed in parallel for speed. We want to allow each parser to
// generate symbol IDs that won't conflict with each other. We also want to be
// able to quickly merge symbol tables from all files into one giant symbol
// table.
//
// We can accomplish both goals by giving each symbol ID two parts: a source
// index that is unique to the parser goroutine, and an inner index that
// increments as the parser generates new symbol IDs. Then a symbol map can
// be an array of arrays indexed first by source index, then by inner index.
// The maps can be merged quickly by creating a single outer array containing
// all inner arrays from all parsed files.
type Ref struct {
	SourceIndex uint32
	InnerIndex  uint32
}

type LocRef struct {
	Loc logger.Loc
	Ref Ref
}

type ImportItemStatus uint8

const (
	ImportItemNone ImportItemStatus = iota

	// The linker doesn't report import/export mismatch errors
	ImportItemGenerated

	// The printer will replace this import with "undefined"
	ImportItemMissing
)

type SymbolFlags uint16

const (
	// Certain symbols must not be renamed or minified. For example, the
	// "arguments" variable is declared by the runtime for every function.
	// Renaming can also break any identifier used inside a "with" statement.
	MustNotBeRenamed SymbolFlags = 1 << iota

	// In React's version of JSX, lower-case names are strings while upper-case
	// names are identifiers. If we are preserving JSX syntax (i.e. not
	// transforming it), then we need to be careful to name the identifiers
	// something with a capital letter so further JSX processing doesn't treat
	// them as strings instead.
	MustStartWithCapitalLetterForJSX

	// If true, this symbol is the target of a "__name" helper function call.
	// This call is special because it deliberately doesn't count as a use
	// of the symbol (otherwise keeping names would disable tree shaking)
	// so "UseCountEstimate" is not incremented. This flag helps us know to
	// avoid optimizing this symbol when "UseCountEstimate" is 1 in this case.
	DidKeepName

	// Sometimes we lower private symbols even if they are supported. For example,
	// consider the following TypeScript code:
	//
	//   class Foo {
	//     #foo = 123
	//     bar = this.#foo
	//   }
	//
	// If "useDefineForClassFields: false" is set in "tsconfig.json", then "bar"
	// must use assignment semantics instead of define semantics. We can compile
	// that to this code:
	//
	//   class Foo {
	//     constructor() {
	//       this.#foo = 123;
	//       this.bar = this.#foo;
	//     }
	//     #foo;
	//   }
	//
	// However, we can't do the same for static fields:
	//
	//   class Foo {
	//     static #foo = 123
	//     static bar = this.#foo
	//   }
	//
	// Compiling these static fields to something like this would be invalid:
	//
	//   class Foo {
	//     static #foo;
	//   }
	//   Foo.#foo = 123;
	//   Foo.bar = Foo.#foo;
	//
	// Thus "#foo" must be lowered even though it's supported. Another case is
	// when we're converting top-level class declarations to class expressions
	// to avoid the TDZ and the class shadowing symbol is referenced within the
	// class body:
	//
	//   class Foo {
	//     static #foo = Foo
	//   }
	//
	// This cannot be converted into something like this:
	//
	//   var Foo = class {
	//     static #foo;
	//   };
	//   Foo.#foo = Foo;
	//
	PrivateSymbolMustBeLowered

	// This is used to remove the all but the last function re-declaration if a
	// function is re-declared multiple times like this:
	//
	//   function foo() { console.log(1) }
	//   function foo() { console.log(2) }
	//
	RemoveOverwrittenFunctionDeclaration

	// This flag is to avoid warning about this symbol more than once. It only
	// applies to the "module" and "exports" unbound symbols.
	DidWarnAboutCommonJSInESM

	// If this is present, the symbol could potentially be overwritten. This means
	// it's not safe to make assumptions about this symbol from the initializer.
	CouldPotentiallyBeMutated

	// This flags all symbols that were exported from the module using the ES6
	// "export" keyword, either directly on the declaration or using "export {}".
	WasExported

	// This means the symbol is a normal function that has no body statements.
	IsEmptyFunction

	// This means the symbol is a normal function that takes a single argument
	// and returns that argument.
	IsIdentityFunction

	// If true, calls to this symbol can be unwrapped (i.e. removed except for
	// argument side effects) if the result is unused.
	CallCanBeUnwrappedIfUnused
)

func (flags SymbolFlags) Has(flag SymbolFlags) bool {
	return (flags & flag) != 0
}

// Note: the order of values in this struct matters to reduce struct size.
type Symbol struct {
	// This is used for symbols that represent items in the import clause of an
	// ES6 import statement. These should always be referenced by EImportIdentifier
	// instead of an EIdentifier. When this is present, the expression should
	// be printed as a property access off the namespace instead of as a bare
	// identifier.
	//
	// For correctness, this must be stored on the symbol instead of indirectly
	// associated with the Ref for the symbol somehow. In ES6 "flat bundling"
	// mode, re-exported symbols are collapsed using MergeSymbols() and renamed
	// symbols from other files that end up at this symbol must be able to tell
	// if it has a namespace alias.
	NamespaceAlias *NamespaceAlias

	// This is the name that came from the parser. Printed names may be renamed
	// during minification or to avoid name collisions. Do not use the original
	// name during printing.
	OriginalName string

	// Used by the parser for single pass parsing. Symbols that have been merged
	// form a linked-list where the last link is the symbol to use. This link is
	// an invalid ref if it's the last link. If this isn't invalid, you need to
	// FollowSymbols to get the real one.
	Link Ref

	// An estimate of the number of uses of this symbol. This is used to detect
	// whether a symbol is used or not. For example, TypeScript imports that are
	// unused must be removed because they are probably type-only imports. This
	// is an estimate and may not be completely accurate due to oversights in the
	// code. But it should always be non-zero when the symbol is used.
	UseCountEstimate uint32

	// This is for generating cross-chunk imports and exports for code splitting.
	ChunkIndex Index32

	// This is used for minification. Symbols that are declared in sibling scopes
	// can share a name. A good heuristic (from Google Closure Compiler) is to
	// assign names to symbols from sibling scopes in declaration order. That way
	// local variable names are reused in each global function like this, which
	// improves gzip compression:
	//
	//   function x(a, b) { ... }
	//   function y(a, b, c) { ... }
	//
	// The parser fills this in for symbols inside nested scopes. There are three
	// slot namespaces: regular symbols, label symbols, and private symbols.
	NestedScopeSlot Index32

	// Boolean values should all be flags instead to save space
	Flags SymbolFlags

	Kind SymbolKind

	// We automatically generate import items for property accesses off of
	// namespace imports. This lets us remove the expensive namespace imports
	// while bundling in many cases, replacing them with a cheap import item
	// instead:
	//
	//   import * as ns from 'path'
	//   ns.foo()
	//
	// That can often be replaced by this, which avoids needing the namespace:
	//
	//   import {foo} from 'path'
	//   foo()
	//
	// However, if the import is actually missing then we don't want to report a
	// compile-time error like we do for real import items. This status lets us
	// avoid this. We also need to be able to replace such import items with
	// undefined, which this status is also used for.
	ImportItemStatus ImportItemStatus
}

// You should call "MergeSymbols" instead of calling this directly
func (newSymbol *Symbol) MergeContentsWith(oldSymbol *Symbol) {
	newSymbol.UseCountEstimate += oldSymbol.UseCountEstimate
	if oldSymbol.Flags.Has(MustNotBeRenamed) && !newSymbol.Flags.Has(MustNotBeRenamed) {
		newSymbol.OriginalName = oldSymbol.OriginalName
		newSymbol.Flags |= MustNotBeRenamed
	}
	if oldSymbol.Flags.Has(MustStartWithCapitalLetterForJSX) {
		newSymbol.Flags |= MustStartWithCapitalLetterForJSX
	}
}

type SlotNamespace uint8

const (
	SlotDefault SlotNamespace = iota
	SlotLabel
	SlotPrivateName
	SlotMangledProp
	SlotMustNotBeRenamed
)

func (s *Symbol) SlotNamespace() SlotNamespace {
	if s.Kind == SymbolUnbound || s.Flags.Has(MustNotBeRenamed) {
		return SlotMustNotBeRenamed
	}
	if s.Kind.IsPrivate() {
		return SlotPrivateName
	}
	if s.Kind == SymbolLabel {
		return SlotLabel
	}
	if s.Kind == SymbolMangledProp {
		return SlotMangledProp
	}
	return SlotDefault
}

type SlotCounts [4]uint32

func (a *SlotCounts) UnionMax(b SlotCounts) {
	for i := range *a {
		ai := &(*a)[i]
		bi := b[i]
		if *ai < bi {
			*ai = bi
		}
	}
}

type NamespaceAlias struct {
	Alias        string
	NamespaceRef Ref
}

type SymbolMap struct {
	// This could be represented as a "map[Ref]Symbol" but a two-level array was
	// more efficient in profiles. This appears to be because it doesn't involve
	// a hash. This representation also makes it trivial to quickly merge symbol
	// maps from multiple files together. Each file only generates symbols in a
	// single inner array, so you can join the maps together by just make a
	// single outer array containing all of the inner arrays. See the comment on
	// "Ref" for more detail.
	SymbolsForSource [][]Symbol
}

func NewSymbolMap(sourceCount int) SymbolMap {
	return SymbolMap{make([][]Symbol, sourceCount)}
}

func (sm SymbolMap) Get(ref Ref) *Symbol {
	return &sm.SymbolsForSource[ref.SourceIndex][ref.InnerIndex]
}

// Returns the canonical ref that represents the ref for the provided symbol.
// This may not be the provided ref if the symbol has been merged with another
// symbol.
func FollowSymbols(symbols SymbolMap, ref Ref) Ref {
	symbol := symbols.Get(ref)
	if symbol.Link == InvalidRef {
		return ref
	}

	link := FollowSymbols(symbols, symbol.Link)

	// Only write if needed to avoid concurrent map update hazards
	if symbol.Link != link {
		symbol.Link = link
	}

	return link
}

// Use this before calling "FollowSymbols" from separate threads to avoid
// concurrent map update hazards. In Go, mutating a map is not threadsafe
// but reading from a map is. Calling "FollowAllSymbols" first ensures that
// all mutation is done up front.
func FollowAllSymbols(symbols SymbolMap) {
	for sourceIndex, inner := range symbols.SymbolsForSource {
		for symbolIndex := range inner {
			FollowSymbols(symbols, Ref{uint32(sourceIndex), uint32(symbolIndex)})
		}
	}
}

// Makes "old" point to "new" by joining the linked lists for the two symbols
// together. That way "FollowSymbols" on both "old" and "new" will result in
// the same ref.
func MergeSymbols(symbols SymbolMap, old Ref, new Ref) Ref {
	if old == new {
		return new
	}

	oldSymbol := symbols.Get(old)
	if oldSymbol.Link != InvalidRef {
		oldSymbol.Link = MergeSymbols(symbols, oldSymbol.Link, new)
		return oldSymbol.Link
	}

	newSymbol := symbols.Get(new)
	if newSymbol.Link != InvalidRef {
		newSymbol.Link = MergeSymbols(symbols, old, newSymbol.Link)
		return newSymbol.Link
	}

	oldSymbol.Link = new
	newSymbol.MergeContentsWith(oldSymbol)
	return new
}

// This is a histogram of character frequencies for minification
type CharFreq [64]int32

func (freq *CharFreq) Scan(text string, delta int32) {
	if delta == 0 {
		return
	}

	// This matches the order in "DefaultNameMinifier"
	for i, n := 0, len(text); i < n; i++ {
		c := text[i]
		switch {
		case c >= 'a' && c <= 'z':
			(*freq)[c-'a'] += delta
		case c >= 'A' && c <= 'Z':
			(*freq)[c-('A'-26)] += delta
		case c >= '0' && c <= '9':
			(*freq)[c+(52-'0')] += delta
		case c == '_':
			(*freq)[62] += delta
		case c == '$':
			(*freq)[63] += delta
		}
	}
}

func (freq *CharFreq) Include(other *CharFreq) {
	for i := 0; i < 64; i++ {
		(*freq)[i] += (*other)[i]
	}
}

type NameMinifier struct {
	head string
	tail string
}

var DefaultNameMinifierJS = NameMinifier{
	head: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_$",
	tail: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_$",
}

var DefaultNameMinifierCSS = NameMinifier{
	head: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_",
	tail: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_",
}

type charAndCount struct {
	char  string
	count int32
	index byte
}

// This type is just so we can use Go's native sort function
type charAndCountArray []charAndCount

func (a charAndCountArray) Len() int          { return len(a) }
func (a charAndCountArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a charAndCountArray) Less(i int, j int) bool {
	ai := a[i]
	aj := a[j]
	return ai.count > aj.count || (ai.count == aj.count && ai.index < aj.index)
}

func (source NameMinifier) ShuffleByCharFreq(freq CharFreq) NameMinifier {
	// Sort the histogram in descending order by count
	array := make(charAndCountArray, 64)
	for i := 0; i < len(source.tail); i++ {
		array[i] = charAndCount{
			char:  source.tail[i : i+1],
			index: byte(i),
			count: freq[i],
		}
	}
	sort.Sort(array)

	// Compute the identifier start and identifier continue sequences
	minifier := NameMinifier{}
	for _, item := range array {
		if item.char < "0" || item.char > "9" {
			minifier.head += item.char
		}
		minifier.tail += item.char
	}
	return minifier
}

func (minifier NameMinifier) NumberToMinifiedName(i int) string {
	n_head := len(minifier.head)
	n_tail := len(minifier.tail)

	j := i % n_head
	name := minifier.head[j : j+1]
	i = i / n_head

	for i > 0 {
		i--
		j := i % n_tail
		name += minifier.tail[j : j+1]
		i = i / n_tail
	}

	return name
}
