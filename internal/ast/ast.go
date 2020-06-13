package ast

import (
	"strings"
)

// Every module (i.e. file) is parsed into a separate AST data structure. For
// efficiency, the parser also resolves all scopes and binds all symbols in the
// tree.
//
// Identifiers in the tree are referenced by a Ref, which is a pointer into the
// symbol table for the file. The symbol table is stored as a top-level field
// in the AST so it can be accessed without traversing the tree. For example,
// a renaming pass can iterate over the symbol table without touching the tree.
//
// Parse trees are intended to be immutable. That makes it easy to build an
// incremental compiler with a "watch" mode that can avoid re-parsing files
// that have already been parsed. Any passes that operate on an AST after it
// has been parsed should create a copy of the mutated parts of the tree
// instead of mutating the original tree.

type L int

// https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Operators/Operator_Precedence
const (
	LLowest L = iota
	LComma
	LSpread
	LYield
	LAssign
	LConditional
	LNullishCoalescing
	LLogicalOr
	LLogicalAnd
	LBitwiseOr
	LBitwiseXor
	LBitwiseAnd
	LEquals
	LCompare
	LShift
	LAdd
	LMultiply
	LExponentiation
	LPrefix
	LPostfix
	LNew
	LCall
)

type OpCode int

func (op OpCode) IsPrefix() bool {
	return op < UnOpPostDec
}

func (op OpCode) IsUnaryUpdate() bool {
	return op >= UnOpPreDec && op <= UnOpPostInc
}

func (op OpCode) IsLeftAssociative() bool {
	return op >= BinOpAdd && op < BinOpComma && op != BinOpPow
}

func (op OpCode) IsRightAssociative() bool {
	return op >= BinOpAssign || op == BinOpPow
}

func (op OpCode) IsBinaryAssign() bool {
	return op >= BinOpAssign
}

// If you add a new token, remember to add it to "OpTable" too
const (
	// Prefix
	UnOpPos OpCode = iota
	UnOpNeg
	UnOpCpl
	UnOpNot
	UnOpVoid
	UnOpTypeof
	UnOpDelete

	// Prefix update
	UnOpPreDec
	UnOpPreInc

	// Postfix update
	UnOpPostDec
	UnOpPostInc

	// Left-associative
	BinOpAdd
	BinOpSub
	BinOpMul
	BinOpDiv
	BinOpRem
	BinOpPow
	BinOpLt
	BinOpLe
	BinOpGt
	BinOpGe
	BinOpIn
	BinOpInstanceof
	BinOpShl
	BinOpShr
	BinOpUShr
	BinOpLooseEq
	BinOpLooseNe
	BinOpStrictEq
	BinOpStrictNe
	BinOpNullishCoalescing
	BinOpLogicalOr
	BinOpLogicalAnd
	BinOpBitwiseOr
	BinOpBitwiseAnd
	BinOpBitwiseXor

	// Non-associative
	BinOpComma

	// Right-associative
	BinOpAssign
	BinOpAddAssign
	BinOpSubAssign
	BinOpMulAssign
	BinOpDivAssign
	BinOpRemAssign
	BinOpPowAssign
	BinOpShlAssign
	BinOpShrAssign
	BinOpUShrAssign
	BinOpBitwiseOrAssign
	BinOpBitwiseAndAssign
	BinOpBitwiseXorAssign
	BinOpNullishCoalescingAssign
	BinOpLogicalOrAssign
	BinOpLogicalAndAssign
)

type opTableEntry struct {
	Text      string
	Level     L
	IsKeyword bool
}

var OpTable = []opTableEntry{
	// Prefix
	{"+", LPrefix, false},
	{"-", LPrefix, false},
	{"~", LPrefix, false},
	{"!", LPrefix, false},
	{"void", LPrefix, true},
	{"typeof", LPrefix, true},
	{"delete", LPrefix, true},

	// Prefix update
	{"--", LPrefix, false},
	{"++", LPrefix, false},

	// Postfix update
	{"--", LPostfix, false},
	{"++", LPostfix, false},

	// Left-associative
	{"+", LAdd, false},
	{"-", LAdd, false},
	{"*", LMultiply, false},
	{"/", LMultiply, false},
	{"%", LMultiply, false},
	{"**", LExponentiation, false}, // Right-associative
	{"<", LCompare, false},
	{"<=", LCompare, false},
	{">", LCompare, false},
	{">=", LCompare, false},
	{"in", LCompare, true},
	{"instanceof", LCompare, true},
	{"<<", LShift, false},
	{">>", LShift, false},
	{">>>", LShift, false},
	{"==", LEquals, false},
	{"!=", LEquals, false},
	{"===", LEquals, false},
	{"!==", LEquals, false},
	{"??", LNullishCoalescing, false},
	{"||", LLogicalOr, false},
	{"&&", LLogicalAnd, false},
	{"|", LBitwiseOr, false},
	{"&", LBitwiseAnd, false},
	{"^", LBitwiseXor, false},

	// Non-associative
	{",", LComma, false},

	// Right-associative
	{"=", LAssign, false},
	{"+=", LAssign, false},
	{"-=", LAssign, false},
	{"*=", LAssign, false},
	{"/=", LAssign, false},
	{"%=", LAssign, false},
	{"**=", LAssign, false},
	{"<<=", LAssign, false},
	{">>=", LAssign, false},
	{">>>=", LAssign, false},
	{"|=", LAssign, false},
	{"&=", LAssign, false},
	{"^=", LAssign, false},
	{"??=", LAssign, false},
	{"||=", LAssign, false},
	{"&&=", LAssign, false},
}

type Loc struct {
	// This is the 0-based index of this location from the start of the file
	Start int32
}

type Range struct {
	Loc Loc
	Len int32
}

func (r Range) End() int32 {
	return r.Loc.Start + r.Len
}

type LocRef struct {
	Loc Loc
	Ref Ref
}

type Path struct {
	Loc  Loc
	Text string

	// If "UseSourceIndex" is true, the path is already resolved. This is used
	// for generated paths. The source file it has been resolved to is stored in
	// the "SourceIndex" field.
	UseSourceIndex bool
	SourceIndex    uint32
}

// The runtime source is always at a special index. The index is always zero
// but this constant is always used instead to improve readability and ensure
// all code that references this index can be discovered easily.
const RuntimeSourceIndex = 0

type PropertyKind int

const (
	PropertyNormal PropertyKind = iota
	PropertyGet
	PropertySet
	PropertySpread
)

type Property struct {
	TSDecorators []Expr
	Key          Expr

	// This is omitted for class fields
	Value *Expr

	// This is used when parsing a pattern that uses default values:
	//
	//   [a = 1] = [];
	//   ({a = 1} = {});
	//
	// It's also used for class fields:
	//
	//   class Foo { a = 1 }
	//
	Initializer *Expr

	Kind       PropertyKind
	IsComputed bool
	IsMethod   bool
	IsStatic   bool
}

type PropertyBinding struct {
	IsComputed   bool
	IsSpread     bool
	Key          Expr
	Value        Binding
	DefaultValue *Expr
}

type Arg struct {
	TSDecorators []Expr
	Binding      Binding
	Default      *Expr

	// "constructor(public x: boolean) {}"
	IsTypeScriptCtorField bool
}

type Fn struct {
	Name *LocRef
	Args []Arg
	Body FnBody

	IsAsync     bool
	IsGenerator bool
	HasRestArg  bool
}

type FnBody struct {
	Loc   Loc
	Stmts []Stmt
}

type Class struct {
	TSDecorators []Expr
	Name         *LocRef
	Extends      *Expr
	BodyLoc      Loc
	Properties   []Property
}

type ArrayBinding struct {
	Binding      Binding
	DefaultValue *Expr
}

type Binding struct {
	Loc  Loc
	Data B
}

// This interface is never called. Its purpose is to encode a variant type in
// Go's type system.
type B interface{ isBinding() }

type BMissing struct{}

type BIdentifier struct{ Ref Ref }

type BArray struct {
	Items     []ArrayBinding
	HasSpread bool
}

type BObject struct{ Properties []PropertyBinding }

func (*BMissing) isBinding()    {}
func (*BIdentifier) isBinding() {}
func (*BArray) isBinding()      {}
func (*BObject) isBinding()     {}

type Expr struct {
	Loc  Loc
	Data E
}

// This interface is never called. Its purpose is to encode a variant type in
// Go's type system.
type E interface{ isExpr() }

type EArray struct {
	Items        []Expr
	IsSingleLine bool
}

type EUnary struct {
	Op    OpCode
	Value Expr
}

type EBinary struct {
	Op    OpCode
	Left  Expr
	Right Expr
}

type EBoolean struct{ Value bool }

type ESuper struct{}

type ENull struct{}

type EUndefined struct{}

type EThis struct{}

type ENew struct {
	Target Expr
	Args   []Expr
}

type ENewTarget struct{}

type EImportMeta struct{}

type OptionalChain uint8

const (
	// "a.b"
	OptionalChainNone OptionalChain = iota

	// "a?.b"
	OptionalChainStart

	// "a?.b.c" => ".c" is OptionalChainContinue
	// "(a?.b).c" => ".c" is OptionalChainNone
	OptionalChainContinue
)

type ECall struct {
	Target        Expr
	Args          []Expr
	OptionalChain OptionalChain
	IsDirectEval  bool
}

type EDot struct {
	Target        Expr
	Name          string
	NameLoc       Loc
	OptionalChain OptionalChain

	// If true, this property access is known to be free of side-effects
	CanBeRemovedIfUnused bool
}

type EIndex struct {
	Target        Expr
	Index         Expr
	OptionalChain OptionalChain
}

type EArrow struct {
	IsAsync         bool
	Args            []Arg
	HasRestArg      bool
	IsParenthesized bool
	PreferExpr      bool // Use shorthand if true and "Body" is a single return statement
	Body            FnBody
}

type EFunction struct{ Fn Fn }

type EClass struct{ Class Class }

type EIdentifier struct{ Ref Ref }

// This is similar to an EIdentifier but it represents a reference to an ES6
// import item.
//
// Depending on how the code is linked, the file containing this EImportIdentifier
// may or may not be in the same module group as the file it was imported from.
//
// If it's the same module group than we can just merge the import item symbol
// with the corresponding symbol that was imported, effectively renaming them
// to be the same thing and statically binding them together.
//
// But if it's a different module group, then the import must be dynamically
// evaluated using a property access off the corresponding namespace symbol,
// which represents the result of a require() call.
//
// It's stored as a separate type so it's not easy to confuse with a plain
// identifier. For example, it'd be bad if code trying to convert "{x: x}" into
// "{x}" shorthand syntax wasn't aware that the "x" in this case is actually
// "{x: importedNamespace.x}". This separate type forces code to opt-in to
// doing this instead of opt-out.
type EImportIdentifier struct {
	Ref Ref
}

// This is similar to EIdentifier but it represents class-private fields and
// methods. It can be used where computed properties can be used, such as
// EIndex and Property.
type EPrivateIdentifier struct {
	Ref Ref
}

type EJSXElement struct {
	Tag        *Expr
	Properties []Property
	Children   []Expr
}

type EMissing struct{}

type ENumber struct{ Value float64 }

type EBigInt struct{ Value string }

type EObject struct {
	Properties   []Property
	IsSingleLine bool
}

type ESpread struct{ Value Expr }

type EString struct{ Value []uint16 }

type TemplatePart struct {
	Value   Expr
	Tail    []uint16
	TailRaw string // This is only filled out for tagged template literals
}

type ETemplate struct {
	Tag     *Expr
	Head    []uint16
	HeadRaw string // This is only filled out for tagged template literals
	Parts   []TemplatePart
}

type ERegExp struct{ Value string }

type EAwait struct {
	Value Expr
}

type EYield struct {
	Value  *Expr
	IsStar bool
}

type EIf struct {
	Test Expr
	Yes  Expr
	No   Expr
}

type ERequire struct {
	Path        Path
	IsES6Import bool
}

type EImport struct {
	Expr Expr
}

func (*EArray) isExpr()             {}
func (*EUnary) isExpr()             {}
func (*EBinary) isExpr()            {}
func (*EBoolean) isExpr()           {}
func (*ESuper) isExpr()             {}
func (*ENull) isExpr()              {}
func (*EUndefined) isExpr()         {}
func (*EThis) isExpr()              {}
func (*ENew) isExpr()               {}
func (*ENewTarget) isExpr()         {}
func (*EImportMeta) isExpr()        {}
func (*ECall) isExpr()              {}
func (*EDot) isExpr()               {}
func (*EIndex) isExpr()             {}
func (*EArrow) isExpr()             {}
func (*EFunction) isExpr()          {}
func (*EClass) isExpr()             {}
func (*EIdentifier) isExpr()        {}
func (*EImportIdentifier) isExpr()  {}
func (*EPrivateIdentifier) isExpr() {}
func (*EJSXElement) isExpr()        {}
func (*EMissing) isExpr()           {}
func (*ENumber) isExpr()            {}
func (*EBigInt) isExpr()            {}
func (*EObject) isExpr()            {}
func (*ESpread) isExpr()            {}
func (*EString) isExpr()            {}
func (*ETemplate) isExpr()          {}
func (*ERegExp) isExpr()            {}
func (*EAwait) isExpr()             {}
func (*EYield) isExpr()             {}
func (*EIf) isExpr()                {}
func (*ERequire) isExpr()           {}
func (*EImport) isExpr()            {}

func JoinWithComma(a Expr, b Expr) Expr {
	return Expr{a.Loc, &EBinary{BinOpComma, a, b}}
}

func JoinAllWithComma(all []Expr) Expr {
	result := all[0]
	for _, value := range all[1:] {
		result = JoinWithComma(result, value)
	}
	return result
}

type ExprOrStmt struct {
	Expr *Expr
	Stmt *Stmt
}

type Stmt struct {
	Loc  Loc
	Data S
}

// This interface is never called. Its purpose is to encode a variant type in
// Go's type system.
type S interface{ isStmt() }

type SBlock struct {
	Stmts []Stmt
}

type SEmpty struct{}

// This is a stand-in for a TypeScript type declaration
type STypeScript struct{}

type SDebugger struct{}

type SDirective struct {
	Value []uint16
}

type SExportClause struct {
	Items []ClauseItem
}

type SExportFrom struct {
	Items        []ClauseItem
	NamespaceRef Ref
	Path         Path
}

type SExportDefault struct {
	DefaultName LocRef
	Value       ExprOrStmt // May be a SFunction or SClass
}

type ExportStarAlias struct {
	Loc  Loc
	Name string
}

type SExportStar struct {
	NamespaceRef Ref
	Alias        *ExportStarAlias
	Path         Path
}

// This is an "export = value;" statement in TypeScript
type SExportEquals struct {
	Value Expr
}

type SExpr struct {
	Value Expr
}

type EnumValue struct {
	Loc   Loc
	Ref   Ref
	Name  []uint16
	Value *Expr
}

type SEnum struct {
	Name     LocRef
	Arg      Ref
	Values   []EnumValue
	IsExport bool
}

type SNamespace struct {
	Name     LocRef
	Arg      Ref
	Stmts    []Stmt
	IsExport bool
}

type SFunction struct {
	Fn       Fn
	IsExport bool
}

type SClass struct {
	Class    Class
	IsExport bool
}

type SLabel struct {
	Name LocRef
	Stmt Stmt
}

type SIf struct {
	Test Expr
	Yes  Stmt
	No   *Stmt
}

type SFor struct {
	Init   *Stmt // May be a SConst, SLet, SVar, or SExpr
	Test   *Expr
	Update *Expr
	Body   Stmt
}

type SForIn struct {
	Init  Stmt // May be a SConst, SLet, SVar, or SExpr
	Value Expr
	Body  Stmt
}

type SForOf struct {
	IsAwait bool
	Init    Stmt // May be a SConst, SLet, SVar, or SExpr
	Value   Expr
	Body    Stmt
}

type SDoWhile struct {
	Body Stmt
	Test Expr
}

type SWhile struct {
	Test Expr
	Body Stmt
}

type SWith struct {
	Value   Expr
	BodyLoc Loc
	Body    Stmt
}

type Catch struct {
	Loc     Loc
	Binding *Binding
	Body    []Stmt
}

type Finally struct {
	Loc   Loc
	Stmts []Stmt
}

type STry struct {
	Body    []Stmt
	Catch   *Catch
	Finally *Finally
}

type Case struct {
	Value *Expr
	Body  []Stmt
}

type SSwitch struct {
	Test    Expr
	BodyLoc Loc
	Cases   []Case
}

// This object represents all of these types of import statements:
//
//    import 'path'
//    import {item1, item2} from 'path'
//    import * as ns from 'path'
//    import defaultItem, {item1, item2} from 'path'
//    import defaultItem, * as ns from 'path'
//
// Many parts are optional and can be combined in different ways. The only
// restriction is that you cannot have both a clause and a star namespace.
type SImport struct {
	// If this is a star import: This is a Ref for the namespace symbol. The Loc
	// for the symbol is StarLoc.
	//
	// Otherwise: This is an auto-generated Ref for the namespace representing
	// the imported file. In this case StarLoc is nil. The NamespaceRef is used
	// when converting this module to a CommonJS module.
	NamespaceRef Ref

	DefaultName *LocRef
	Items       *[]ClauseItem
	StarNameLoc *Loc
	Path        Path
}

type SReturn struct {
	Value *Expr
}

type SThrow struct {
	Value Expr
}

type LocalKind uint8

const (
	LocalVar LocalKind = iota
	LocalLet
	LocalConst
)

type SLocal struct {
	Decls    []Decl
	Kind     LocalKind
	IsExport bool

	// The TypeScript compiler doesn't generate code for "import foo = bar"
	// statements inside namespaces where the import is never used.
	WasTSImportEqualsInNamespace bool
}

type SBreak struct {
	Name *LocRef
}

type SContinue struct {
	Name *LocRef
}

func (*SBlock) isStmt()         {}
func (*SDebugger) isStmt()      {}
func (*SDirective) isStmt()     {}
func (*SEmpty) isStmt()         {}
func (*STypeScript) isStmt()    {}
func (*SExportClause) isStmt()  {}
func (*SExportFrom) isStmt()    {}
func (*SExportDefault) isStmt() {}
func (*SExportStar) isStmt()    {}
func (*SExportEquals) isStmt()  {}
func (*SExpr) isStmt()          {}
func (*SEnum) isStmt()          {}
func (*SNamespace) isStmt()     {}
func (*SFunction) isStmt()      {}
func (*SClass) isStmt()         {}
func (*SLabel) isStmt()         {}
func (*SIf) isStmt()            {}
func (*SFor) isStmt()           {}
func (*SForIn) isStmt()         {}
func (*SForOf) isStmt()         {}
func (*SDoWhile) isStmt()       {}
func (*SWhile) isStmt()         {}
func (*SWith) isStmt()          {}
func (*STry) isStmt()           {}
func (*SSwitch) isStmt()        {}
func (*SImport) isStmt()        {}
func (*SReturn) isStmt()        {}
func (*SThrow) isStmt()         {}
func (*SLocal) isStmt()         {}
func (*SBreak) isStmt()         {}
func (*SContinue) isStmt()      {}

func IsSuperCall(stmt Stmt) bool {
	if expr, ok := stmt.Data.(*SExpr); ok {
		if call, ok := expr.Value.Data.(*ECall); ok {
			if _, ok := call.Target.Data.(*ESuper); ok {
				return true
			}
		}
	}
	return false
}

type ClauseItem struct {
	Alias    string
	AliasLoc Loc
	Name     LocRef
}

type Decl struct {
	Binding Binding
	Value   *Expr
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

	// Classes can merge with TypeScript namespaces.
	SymbolClass

	// A class-private identifier (i.e. "#foo").
	SymbolPrivate
	SymbolPrivateGet
	SymbolPrivateSet
	SymbolPrivateGetSetPair

	// TypeScript enums can merge with TypeScript namespaces and other TypeScript
	// enums.
	SymbolTSEnum

	// TypeScript namespaces can merge with classes, functions, TypeScript enums,
	// and other TypeScript namespaces.
	SymbolTSNamespace

	// In TypeScript, imports are allowed to silently collide with symbols within
	// the module. Presumably this is because the imports may be type-only.
	SymbolTSImport

	// This annotates all other symbols that don't have special behavior.
	SymbolOther
)

func (kind SymbolKind) IsPrivate() bool {
	return kind >= SymbolPrivate && kind <= SymbolPrivateGetSetPair
}

func (kind SymbolKind) IsHoisted() bool {
	return kind == SymbolHoisted || kind == SymbolHoistedFunction
}

var InvalidRef Ref = Ref{^uint32(0), ^uint32(0)}

// Files are parsed in parallel for speed. We want to allow each parser to
// generate symbol IDs that won't conflict with each other. We also want to be
// able to quickly merge symbol tables from all files into one giant symbol
// table.
//
// We can accomplish both goals by giving each symbol ID two parts: an outer
// index that is unique to the parser goroutine, and an inner index that
// increments as the parser generates new symbol IDs. Then a symbol map can
// be an array of arrays indexed first by outer index, then by inner index.
// The maps can be merged quickly by creating a single outer array containing
// all inner arrays from all parsed files.
type Ref struct {
	OuterIndex uint32
	InnerIndex uint32
}

type ImportItemStatus uint8

const (
	ImportItemNone ImportItemStatus = iota

	// The linker doesn't report import/export mismatch errors
	ImportItemGenerated

	// The printer will replace this import with "undefined"
	ImportItemMissing
)

type Symbol struct {
	Kind SymbolKind

	// Certain symbols must not be renamed or minified. For example, the
	// "arguments" variable is declared by the runtime for every function.
	// Renaming can also break any identifier used inside a "with" statement.
	MustNotBeRenamed bool

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

	// An estimate of the number of uses of this symbol. This is used for
	// minification (to prefer shorter names for more frequently used symbols).
	// The reason why this is an estimate instead of an accurate count is that
	// it's not updated during dead code elimination for speed. I figure that
	// even without updating after parsing it's still a pretty good heuristic.
	UseCountEstimate uint32

	Name string

	// Used by the parser for single pass parsing. Symbols that have been merged
	// form a linked-list where the last link is the symbol to use. This link is
	// an invalid ref if it's the last link. If this isn't invalid, you need to
	// FollowSymbols to get the real one.
	Link Ref

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
}

type NamespaceAlias struct {
	NamespaceRef Ref
	Alias        string
}

type ScopeKind int

const (
	ScopeBlock ScopeKind = iota
	ScopeWith
	ScopeLabel
	ScopeClassName
	ScopeClassBody

	// The scopes below stop hoisted variables from extending into parent scopes
	ScopeEntry // This is a module, TypeScript enum, or TypeScript namespace
	ScopeFunctionArgs
	ScopeFunctionBody
)

func (kind ScopeKind) StopsHoisting() bool {
	return kind >= ScopeEntry
}

type Scope struct {
	Kind      ScopeKind
	Parent    *Scope
	Children  []*Scope
	Members   map[string]Ref
	Generated []Ref

	// This is used to store the ref of the label symbol for ScopeLabel scopes.
	LabelRef Ref

	// If a scope contains a direct eval() expression, then none of the symbols
	// inside that scope can be renamed. We conservatively assume that the
	// evaluated code might reference anything that it has access to.
	ContainsDirectEval bool
}

type SymbolMap struct {
	// This could be represented as a "map[Ref]Symbol" but a two-level array was
	// more efficient in profiles. This appears to be because it doesn't involve
	// a hash. This representation also makes it trivial to quickly merge symbol
	// maps from multiple files together. Each file only generates symbols in a
	// single inner array, so you can join the maps together by just make a
	// single outer array containing all of the inner arrays. See the comment on
	// "Ref" for more detail.
	Outer [][]Symbol
}

func NewSymbolMap(sourceCount int) SymbolMap {
	return SymbolMap{make([][]Symbol, sourceCount)}
}

func (sm SymbolMap) Get(ref Ref) *Symbol {
	return &sm.Outer[ref.OuterIndex][ref.InnerIndex]
}

type ImportKind uint8

const (
	ImportStmt ImportKind = iota
	ImportRequire
	ImportDynamic
)

type ImportPath struct {
	Path Path
	Kind ImportKind

	// If this is true, the import doesn't actually use any imported values. The
	// import is only used for its side effects.
	DoesNotUseExports bool
}

type AST struct {
	WasTypeScript bool

	// This is a list of CommonJS features. When a file uses CommonJS features,
	// it's not a candidate for "flat bundling" and must be wrapped in its own
	// closure.
	HasTopLevelReturn bool
	UsesExportsRef    bool
	UsesModuleRef     bool

	// This is a list of ES6 features
	HasES6Imports bool
	HasES6Exports bool

	Hashbang    string
	Directive   string
	Parts       []Part
	Symbols     SymbolMap
	ModuleScope *Scope
	ExportsRef  Ref
	ModuleRef   Ref
	WrapperRef  Ref

	// These are used when bundling. They are filled in during the parser pass
	// since we already have to traverse the AST then anyway and the parser pass
	// is conveniently fully parallelized.
	NamedImports          map[Ref]NamedImport
	NamedExports          map[string]Ref
	TopLevelSymbolToParts map[Ref][]uint32
	ExportStars           []Path
}

func (ast *AST) HasCommonJSFeatures() bool {
	return ast.HasTopLevelReturn || ast.UsesExportsRef || ast.UsesModuleRef
}

func (ast *AST) UsesCommonJSExports() bool {
	return ast.UsesExportsRef || ast.UsesModuleRef
}

func (ast *AST) HasES6Syntax() bool {
	return ast.HasES6Imports || ast.HasES6Exports
}

type NamedImport struct {
	Alias        string
	AliasLoc     Loc
	ImportPath   Path
	NamespaceRef Ref

	// Parts within this file that use this import
	LocalPartsWithUses []uint32

	// It's useful to flag exported imports because if they are in a TypeScript
	// file, we can't tell if they are a type or a value.
	IsExported bool
}

// Each file is made up of multiple parts, and each part consists of one or
// more top-level statements. Parts are used for tree shaking and code
// splitting analysis. Individual parts of a file can be discarded by tree
// shaking and can be assigned to separate chunks (i.e. output files) by code
// splitting.
type Part struct {
	ImportPaths []ImportPath
	Stmts       []Stmt

	// All symbols that are declared in this part. Note that a given symbol may
	// have multiple declarations, and so may end up being declared in multiple
	// parts (e.g. multiple "var" declarations with the same name). Also note
	// that this list isn't deduplicated and may contain duplicates.
	DeclaredSymbols []DeclaredSymbol

	// An estimate of the number of uses of all symbols used within this part.
	UseCountEstimates map[Ref]uint32

	// The indices of the other parts in this file that are needed if this part
	// is needed.
	LocalDependencies map[uint32]bool

	// If true, this part can be removed if none of the declared symbols are
	// used. If the file containing this part is imported, then all parts that
	// don't have this flag enabled must be included.
	CanBeRemovedIfUnused bool

	// If true, this is the automatically-generated part for this file's ES6
	// exports. It may hold the "const exports = {};" statement and also the
	// "__export(exports, { ... })" call to initialize the getters.
	IsNamespaceExport bool

	// This is used for generated parts that we don't want to be present if they
	// aren't needed. This enables tree shaking for these parts even if global
	// tree shaking isn't enabled.
	ForceTreeShaking bool
}

type DeclaredSymbol struct {
	Ref        Ref
	IsTopLevel bool
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
	for sourceIndex, inner := range symbols.Outer {
		for symbolIndex, _ := range inner {
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
	newSymbol.UseCountEstimate += oldSymbol.UseCountEstimate
	if oldSymbol.MustNotBeRenamed {
		newSymbol.MustNotBeRenamed = true
	}
	return new
}

// This has a custom implementation instead of using "filepath.Base" because it
// should work the same on Unix and Windows. These names end up in the generated
// output and the generated output should not depend on the OS.
func platformIndependentPathDirAndBaseWithoutExt(path string) (string, string) {
	dir := ""
	for {
		i := strings.LastIndexAny(path, "/\\")
		if i < 0 {
			break
		}
		if i+1 != len(path) {
			dir = path[:i]
			path = path[i+1:]
			break
		}
		path = path[:i]
	}
	if dot := strings.LastIndexByte(path, '.'); dot >= 0 {
		path = path[:dot]
	}
	return dir, path
}

// For readability, the names of certain automatically-generated symbols are
// derived from the file name. For example, instead of the CommonJS wrapper for
// a file being called something like "require273" it can be called something
// like "require_react" instead. This function generates the part of these
// identifiers that's specific to the file path. It can take both an absolute
// path (OS-specific) and a path in the source code (OS-independent).
//
// Note that these generated names do not at all relate to the correctness of
// the code as far as avoiding symbol name collisions. These names still go
// through the renaming logic that all other symbols go through to avoid name
// collisions.
func GenerateNonUniqueNameFromPath(path string) string {
	// Get the file name without the extension
	dir, base := platformIndependentPathDirAndBaseWithoutExt(path)

	// If the name is "index", use the directory name instead. This is because
	// many packages in npm use the file name "index.js" because it triggers
	// node's implicit module resolution rules that allows you to import it by
	// just naming the directory.
	if base == "index" {
		_, dirBase := platformIndependentPathDirAndBaseWithoutExt(dir)
		if dirBase != "" {
			base = dirBase
		}
	}

	// Convert it to an ASCII identifier
	bytes := []byte{}
	needsGap := false
	for _, c := range base {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (len(bytes) > 0 && c >= '0' && c <= '9') {
			if needsGap {
				bytes = append(bytes, '_')
				needsGap = false
			}
			bytes = append(bytes, byte(c))
		} else if len(bytes) > 0 {
			needsGap = true
		}
	}

	// Make sure the name isn't empty
	if len(bytes) == 0 {
		return "_"
	}
	return string(bytes)
}
