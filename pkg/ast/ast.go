package ast

import (
	"path"
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
}

type Loc struct {
	// This is the 0-based index of this location from the start of the file
	Start int32
}

type Range struct {
	Loc Loc
	Len int32
}

type LocRef struct {
	Loc Loc
	Ref Ref
}

type Path struct {
	Loc  Loc
	Text string
}

type PropertyKind int

const (
	PropertyNormal PropertyKind = iota
	PropertyGet
	PropertySet
	PropertySpread
)

type Property struct {
	Kind       PropertyKind
	IsComputed bool
	IsMethod   bool
	IsStatic   bool
	Key        Expr

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
}

type PropertyBinding struct {
	IsComputed   bool
	IsSpread     bool
	Key          Expr
	Value        Binding
	DefaultValue *Expr
}

type Arg struct {
	Binding Binding
	Default *Expr
}

type Fn struct {
	Name        *LocRef
	Args        []Arg
	IsAsync     bool
	IsGenerator bool
	HasRestArg  bool
	Stmts       []Stmt
}

type Class struct {
	Name       *LocRef
	Extends    *Expr
	Properties []Property
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

type EArray struct{ Items []Expr }

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

type ECall struct {
	Target          Expr
	Args            []Expr
	IsOptionalChain bool
}

type EDot struct {
	Target          Expr
	Name            string
	NameLoc         Loc
	IsOptionalChain bool
}

type EIndex struct {
	Target          Expr
	Index           Expr
	IsOptionalChain bool
}

type EArrow struct {
	IsAsync    bool
	Args       []Arg
	HasRestArg bool
	Stmts      []Stmt
	Expr       *Expr
}

type EFunction struct{ Fn Fn }

type EClass struct{ Class Class }

type EIdentifier struct{ Ref Ref }

type ENamespaceImport struct {
	NamespaceRef Ref
	ItemRef      Ref
	Alias        string
}

type EJSXElement struct {
	Tag        *Expr
	Properties []Property
	Children   []Expr
}

type EMissing struct{}

type ENumber struct{ Value float64 }

type EBigInt struct{ Value string }

type EObject struct{ Properties []Property }

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

func (*EArray) isExpr()           {}
func (*EUnary) isExpr()           {}
func (*EBinary) isExpr()          {}
func (*EBoolean) isExpr()         {}
func (*ESuper) isExpr()           {}
func (*ENull) isExpr()            {}
func (*EUndefined) isExpr()       {}
func (*EThis) isExpr()            {}
func (*ENew) isExpr()             {}
func (*ENewTarget) isExpr()       {}
func (*EImportMeta) isExpr()      {}
func (*ECall) isExpr()            {}
func (*EDot) isExpr()             {}
func (*EIndex) isExpr()           {}
func (*EArrow) isExpr()           {}
func (*EFunction) isExpr()        {}
func (*EClass) isExpr()           {}
func (*EIdentifier) isExpr()      {}
func (*ENamespaceImport) isExpr() {}
func (*EJSXElement) isExpr()      {}
func (*EMissing) isExpr()         {}
func (*ENumber) isExpr()          {}
func (*EBigInt) isExpr()          {}
func (*EObject) isExpr()          {}
func (*ESpread) isExpr()          {}
func (*EString) isExpr()          {}
func (*ETemplate) isExpr()        {}
func (*ERegExp) isExpr()          {}
func (*EAwait) isExpr()           {}
func (*EYield) isExpr()           {}
func (*EIf) isExpr()              {}
func (*ERequire) isExpr()         {}
func (*EImport) isExpr()          {}

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

type SExportStar struct {
	Item *ClauseItem
	Path Path
}

type SExpr struct {
	Value Expr
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
	Value Expr
	Body  Stmt
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
	Test  Expr
	Cases []Case
}

// This object represents all of these types of import statements:
//
//   import 'path'
//   import {item1, item2} from 'path'
//   import * as ns from 'path'
//   import defaultItem, {item1, item2} from 'path'
//   import defaultItem, * as ns from 'path'
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
	StarLoc     *Loc
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
func (*SExportClause) isStmt()  {}
func (*SExportFrom) isStmt()    {}
func (*SExportDefault) isStmt() {}
func (*SExportStar) isStmt()    {}
func (*SExpr) isStmt()          {}
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

	// This annotates all other symbols that don't have special behavior.
	SymbolOther
)

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

type Symbol struct {
	Kind SymbolKind

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
}

type ScopeKind int

const (
	ScopeBlock ScopeKind = iota
	ScopeLabel
	ScopeFunction
	ScopeFunctionName
	ScopeClassName
	ScopeModule
)

type Scope struct {
	Kind      ScopeKind
	Parent    *Scope
	Children  []*Scope
	Members   map[string]Ref
	Generated []Ref

	// This is used to store the ref of the label symbol for ScopeLabel scopes.
	LabelRef Ref
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

func NewSymbolMap(maxSourceIndex int) *SymbolMap {
	return &SymbolMap{make([][]Symbol, maxSourceIndex+1)}
}

func (sm *SymbolMap) Get(ref Ref) Symbol {
	return sm.Outer[ref.OuterIndex][ref.InnerIndex]
}

func (sm *SymbolMap) IncrementUseCountEstimate(ref Ref) {
	sm.Outer[ref.OuterIndex][ref.InnerIndex].UseCountEstimate++
}

// The symbol must already exist to call this
func (sm *SymbolMap) Set(ref Ref, symbol Symbol) {
	sm.Outer[ref.OuterIndex][ref.InnerIndex] = symbol
}

// The symbol may not already exist when you call this
func (sm *SymbolMap) SetNew(ref Ref, symbol Symbol) {
	outer := sm.Outer
	inner := outer[ref.OuterIndex]
	innerLen := uint32(len(inner))
	if ref.InnerIndex >= innerLen {
		inner = append(inner, make([]Symbol, ref.InnerIndex+1-innerLen)...)
		outer[ref.OuterIndex] = inner
	}
	inner[ref.InnerIndex] = symbol
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
}

type AST struct {
	ImportPaths []ImportPath

	// ENamespaceImport items in this map are printed as an indirect access off
	// of the namespace. This is a way for the bundler to pass this information
	// to the printer. This is necessary when using a namespace import or when
	// an import item must be converted to a property access off a require() call.
	IndirectImportItems map[Ref]bool

	// This is true if something used the "exports" or "module" variables, which
	// means they could have exported something. This is used to silence errors
	// about mismatched exports.
	HasCommonJsExports bool

	Hashbang    string
	Stmts       []Stmt
	Symbols     *SymbolMap
	ModuleScope *Scope
	ExportsRef  Ref
	RequireRef  Ref
	ModuleRef   Ref
}

// Returns the canonical ref that represents the ref for the provided symbol.
// This may not be the provided ref if the symbol has been merged with another
// symbol.
func FollowSymbols(symbols *SymbolMap, ref Ref) Ref {
	symbol := symbols.Get(ref)
	if symbol.Link == InvalidRef {
		return ref
	}

	link := FollowSymbols(symbols, symbol.Link)

	// Only write if needed to avoid concurrent map update hazards
	if symbol.Link != link {
		symbol.Link = link
		symbols.Set(ref, symbol)
	}

	return link
}

// Use this before calling "FollowSymbols" from separate threads to avoid
// concurrent map update hazards. In Go, mutating a map is not threadsafe
// but reading from a map is. Calling "FollowAllSymbols" first ensures that
// all mutation is done up front.
func FollowAllSymbols(symbols *SymbolMap) {
	for sourceIndex, inner := range symbols.Outer {
		for symbolIndex, _ := range inner {
			FollowSymbols(symbols, Ref{uint32(sourceIndex), uint32(symbolIndex)})
		}
	}
}

// Makes "old" point to "new" by joining the linked lists for the two symbols
// together. That way "FollowSymbols" on both "old" and "new" will result in
// the same ref.
func MergeSymbols(symbols *SymbolMap, old Ref, new Ref) Ref {
	if old == new {
		return new
	}

	oldSymbol := symbols.Get(old)
	if oldSymbol.Link != InvalidRef {
		oldSymbol.Link = MergeSymbols(symbols, oldSymbol.Link, new)
		symbols.Set(old, oldSymbol)
		return oldSymbol.Link
	}

	newSymbol := symbols.Get(new)
	if newSymbol.Link != InvalidRef {
		newSymbol.Link = MergeSymbols(symbols, old, newSymbol.Link)
		symbols.Set(new, newSymbol)
		return newSymbol.Link
	}

	oldSymbol.Link = new
	newSymbol.UseCountEstimate += oldSymbol.UseCountEstimate
	symbols.Set(old, oldSymbol)
	symbols.Set(new, newSymbol)
	return new
}

func GenerateNonUniqueNameFromPath(text string) string {
	// Get the file name without the extension
	base := path.Base(text)
	lastDot := strings.LastIndexByte(base, '.')
	if lastDot >= 0 {
		base = base[:lastDot]
	}

	// Convert it to an ASCII identifier
	bytes := []byte{}
	for _, c := range base {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (len(bytes) > 0 && c >= '0' && c <= '9') {
			bytes = append(bytes, byte(c))
		} else if len(bytes) > 0 && bytes[len(bytes)-1] != '_' {
			bytes = append(bytes, '_')
		}
	}

	// Make sure the name isn't empty
	if len(bytes) == 0 {
		return "_"
	}
	return string(bytes)
}
