package js_ast

import (
	"sort"
	"strings"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/logger"
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
	LMember
)

type OpCode int

func (op OpCode) IsPrefix() bool {
	return op < UnOpPostDec
}

func (op OpCode) UnaryAssignTarget() AssignTarget {
	if op >= UnOpPreDec && op <= UnOpPostInc {
		return AssignTargetUpdate
	}
	return AssignTargetNone
}

func (op OpCode) IsLeftAssociative() bool {
	return op >= BinOpAdd && op < BinOpComma && op != BinOpPow
}

func (op OpCode) IsRightAssociative() bool {
	return op >= BinOpAssign || op == BinOpPow
}

func (op OpCode) BinaryAssignTarget() AssignTarget {
	if op == BinOpAssign {
		return AssignTargetReplace
	}
	if op > BinOpAssign {
		return AssignTargetUpdate
	}
	return AssignTargetNone
}

type AssignTarget uint8

const (
	AssignTargetNone    AssignTarget = iota
	AssignTargetReplace              // "a = b"
	AssignTargetUpdate               // "a += b"
)

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

type LocRef struct {
	Loc logger.Loc
	Ref Ref
}

type Comment struct {
	Loc  logger.Loc
	Text string
}

type Span struct {
	Text  string
	Range logger.Range
}

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

	Kind         PropertyKind
	IsComputed   bool
	IsMethod     bool
	IsStatic     bool
	WasShorthand bool
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
	Type         Ref
	// "constructor(public x: boolean) {}"
	IsTypeScriptCtorField bool
}

type Fn struct {
	Name         *LocRef
	OpenParenLoc logger.Loc
	Args         []Arg
	Body         FnBody
	ArgumentsRef Ref

	IsAsync     bool
	IsGenerator bool
	HasRestArg  bool
	HasIfScope  bool
}

type FnBody struct {
	Loc   logger.Loc
	Stmts []Stmt
}

type Class struct {
	TSDecorators []Expr
	Name         *LocRef
	Extends      *Expr
	BodyLoc      logger.Loc
	Properties   []Property
}

type ArrayBinding struct {
	Binding      Binding
	DefaultValue *Expr
}

type Binding struct {
	Loc  logger.Loc
	Data B
}

// This interface is never called. Its purpose is to encode a variant type in
// Go's type system.
type B interface{ isBinding() }

type BMissing struct{}

type BIdentifier struct{ Ref Ref }

type BArray struct {
	Items        []ArrayBinding
	HasSpread    bool
	IsSingleLine bool
}

type BObject struct {
	Properties   []PropertyBinding
	IsSingleLine bool
}

func (*BMissing) isBinding()    {}
func (*BIdentifier) isBinding() {}
func (*BArray) isBinding()      {}
func (*BObject) isBinding()     {}

type Expr struct {
	Loc  logger.Loc
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

	// True if there is a comment containing "@__PURE__" or "#__PURE__" preceding
	// this call expression. See the comment inside ECall for more details.
	CanBeUnwrappedIfUnused bool
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

	// True if there is a comment containing "@__PURE__" or "#__PURE__" preceding
	// this call expression. This is an annotation used for tree shaking, and
	// means that the call can be removed if it's unused. It does not mean the
	// call is pure (e.g. it may still return something different if called twice).
	//
	// Note that the arguments are not considered to be part of the call. If the
	// call itself is removed due to this annotation, the arguments must remain
	// if they have side effects.
	CanBeUnwrappedIfUnused bool
}

func (a *ECall) HasSameFlagsAs(b *ECall) bool {
	return a.OptionalChain == b.OptionalChain &&
		a.IsDirectEval == b.IsDirectEval &&
		a.CanBeUnwrappedIfUnused == b.CanBeUnwrappedIfUnused
}

type EDot struct {
	Target        Expr
	Name          string
	NameLoc       logger.Loc
	OptionalChain OptionalChain

	// If true, this property access is known to be free of side-effects. That
	// means it can be removed if the resulting value isn't used.
	CanBeRemovedIfUnused bool

	// If true, this property access is a function that, when called, can be
	// unwrapped if the resulting value is unused. Unwrapping means discarding
	// the call target but keeping any arguments with side effects.
	CallCanBeUnwrappedIfUnused bool
}

func (a *EDot) HasSameFlagsAs(b *EDot) bool {
	return a.OptionalChain == b.OptionalChain &&
		a.CanBeRemovedIfUnused == b.CanBeRemovedIfUnused &&
		a.CallCanBeUnwrappedIfUnused == b.CallCanBeUnwrappedIfUnused
}

type EIndex struct {
	Target        Expr
	Index         Expr
	OptionalChain OptionalChain
}

func (a *EIndex) HasSameFlagsAs(b *EIndex) bool {
	return a.OptionalChain == b.OptionalChain
}

type EArrow struct {
	Args []Arg
	Body FnBody

	IsAsync         bool
	HasRestArg      bool
	IsParenthesized bool
	PreferExpr      bool // Use shorthand if true and "Body" is a single return statement
}

type EFunction struct{ Fn Fn }

type EClass struct{ Class Class }

type EIdentifier struct {
	Ref Ref

	// If true, this identifier is known to not have a side effect (i.e. to not
	// throw an exception) when referenced. If false, this identifier may or may
	// not have side effects when referenced. This is used to allow the removal
	// of known globals such as "Object" if they aren't used.
	CanBeRemovedIfUnused bool

	// If true, this identifier represents a function that, when called, can be
	// unwrapped if the resulting value is unused. Unwrapping means discarding
	// the call target but keeping any arguments with side effects.
	CallCanBeUnwrappedIfUnused bool
}

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

type EString struct {
	Value          []uint16
	PreferTemplate bool
}

type TemplatePart struct {
	Value   Expr
	TailLoc logger.Loc
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
	ImportRecordIndex uint32
}

type ERequireResolve struct {
	ImportRecordIndex uint32
}

type EImport struct {
	Expr              Expr
	ImportRecordIndex *uint32

	// Comments inside "import()" expressions have special meaning for Webpack.
	// Preserving comments inside these expressions makes it possible to use
	// esbuild as a TypeScript-to-JavaScript frontend for Webpack to improve
	// performance. We intentionally do not interpret these comments in esbuild
	// because esbuild is not Webpack. But we do preserve them since doing so is
	// harmless, easy to maintain, and useful to people. See the Webpack docs for
	// more info: https://webpack.js.org/api/module-methods/#magic-comments.
	LeadingInteriorComments []Comment
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
func (*ERequireResolve) isExpr()    {}
func (*EImport) isExpr()            {}

func Assign(a Expr, b Expr) Expr {
	return Expr{a.Loc, &EBinary{BinOpAssign, a, b}}
}

func AssignStmt(a Expr, b Expr) Stmt {
	return Stmt{a.Loc, &SExpr{Expr{a.Loc, &EBinary{BinOpAssign, a, b}}}}
}

func Not(a Expr) Expr {
	// "!!!a" => "!a"
	if not, ok := a.Data.(*EUnary); ok && not.Op == UnOpNot && IsBooleanValue(not.Value) {
		return not.Value
	}
	return Expr{a.Loc, &EUnary{UnOpNot, a}}
}

func IsBooleanValue(a Expr) bool {
	switch e := a.Data.(type) {
	case *EBoolean:
		return true
	case *EUnary:
		return e.Op == UnOpNot || e.Op == UnOpDelete
	case *EBinary:
		switch e.Op {
		case BinOpStrictEq, BinOpStrictNe, BinOpLooseEq, BinOpLooseNe,
			BinOpLt, BinOpGt, BinOpLe, BinOpGe,
			BinOpInstanceof, BinOpIn:
			return true
		case BinOpLogicalOr, BinOpLogicalAnd:
			return IsBooleanValue(e.Left) && IsBooleanValue(e.Right)
		case BinOpNullishCoalescing:
			return IsBooleanValue(e.Left)
		}
	}
	return false
}

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
	Loc  logger.Loc
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

type SComment struct {
	Text string
}

type SDebugger struct{}

type SDirective struct {
	Value []uint16
}

type SExportClause struct {
	Items        []ClauseItem
	IsSingleLine bool
}

type SExportFrom struct {
	Items             []ClauseItem
	NamespaceRef      Ref
	ImportRecordIndex uint32
	IsSingleLine      bool
}

type SExportDefault struct {
	DefaultName LocRef
	Value       ExprOrStmt // May be a SFunction or SClass
}

type ExportStarAlias struct {
	Loc  logger.Loc
	Name string
}

type SExportStar struct {
	NamespaceRef      Ref
	Alias             *ExportStarAlias
	ImportRecordIndex uint32
}

// This is an "export = value;" statement in TypeScript
type SExportEquals struct {
	Value Expr
}

// The decision of whether to export an expression using "module.exports" or
// "export default" is deferred until linking using this statement kind
type SLazyExport struct {
	Value Expr
}

type SExpr struct {
	Value Expr
}

type EnumValue struct {
	Loc   logger.Loc
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
	BodyLoc logger.Loc
	Body    Stmt
}

type Catch struct {
	Loc     logger.Loc
	Binding *Binding
	Body    []Stmt
}

type Finally struct {
	Loc   logger.Loc
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
	BodyLoc logger.Loc
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

	DefaultName       *LocRef
	Items             *[]ClauseItem
	StarNameLoc       *logger.Loc
	ImportRecordIndex uint32
	IsSingleLine      bool
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
	Label *LocRef
}

type SContinue struct {
	Label *LocRef
}

func (*SBlock) isStmt()         {}
func (*SComment) isStmt()       {}
func (*SDebugger) isStmt()      {}
func (*SDirective) isStmt()     {}
func (*SEmpty) isStmt()         {}
func (*STypeScript) isStmt()    {}
func (*SExportClause) isStmt()  {}
func (*SExportFrom) isStmt()    {}
func (*SExportDefault) isStmt() {}
func (*SExportStar) isStmt()    {}
func (*SExportEquals) isStmt()  {}
func (*SLazyExport) isStmt()    {}
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
	AliasLoc logger.Loc
	Name     LocRef

	// This is needed for "export {foo as bar} from 'path'" statements. This case
	// is a re-export and "foo" and "bar" are both aliases. We need to preserve
	// both aliases in case the symbol is renamed.
	OriginalName string
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

	// Generator and async functions are not hoisted, but still have special
	// properties such as being able to overwrite previous functions with the
	// same name
	SymbolGeneratorOrAsyncFunction

	// This is the special "arguments" variable inside functions
	SymbolArguments

	// Classes can merge with TypeScript namespaces.
	SymbolClass

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

	// This annotates all other symbols that don't have special behavior.
	SymbolOther

	// This symbol causes a compile error when referenced
	SymbolError
)

func (kind SymbolKind) IsPrivate() bool {
	return kind >= SymbolPrivateField && kind <= SymbolPrivateStaticGetSetPair
}

func (kind SymbolKind) Feature() compat.JSFeature {
	switch kind {
	case SymbolPrivateField:
		return compat.ClassPrivateField
	case SymbolPrivateMethod:
		return compat.ClassPrivateMethod
	case SymbolPrivateGet, SymbolPrivateSet, SymbolPrivateGetSetPair:
		return compat.ClassPrivateAccessor
	case SymbolPrivateStaticField:
		return compat.ClassPrivateStaticField
	case SymbolPrivateStaticMethod:
		return compat.ClassPrivateStaticMethod
	case SymbolPrivateStaticGet, SymbolPrivateStaticSet, SymbolPrivateStaticGetSetPair:
		return compat.ClassPrivateStaticAccessor
	default:
		return 0
	}
}

func (kind SymbolKind) IsHoisted() bool {
	return kind == SymbolHoisted || kind == SymbolHoistedFunction
}

func (kind SymbolKind) IsHoistedOrFunction() bool {
	return kind.IsHoisted() || kind == SymbolGeneratorOrAsyncFunction
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

// Note: the order of values in this struct matters to reduce struct size.
type Symbol struct {
	// This is the name that came from the parser. Printed names may be renamed
	// during minification or to avoid name collisions. Do not use the original
	// name during printing.
	OriginalName string

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
	// It's stored as one's complement so the zero value is invalid.
	ChunkIndex uint32

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
	// slot namespaces: regular symbols, label symbols, and private symbols. This
	// is stored as one's complement so the zero value is invalid.
	NestedScopeSlot uint32

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
}

type SlotNamespace uint8

const (
	SlotDefault SlotNamespace = iota
	SlotLabel
	SlotPrivateName
	SlotMustNotBeRenamed
)

func (s *Symbol) SlotNamespace() SlotNamespace {
	if s.Kind == SymbolUnbound || s.MustNotBeRenamed {
		return SlotMustNotBeRenamed
	}
	if s.Kind.IsPrivate() {
		return SlotPrivateName
	}
	if s.Kind == SymbolLabel {
		return SlotLabel
	}
	return SlotDefault
}

type SlotCounts [3]uint32

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

type ScopeMember struct {
	Ref Ref
	Loc logger.Loc
}

type Scope struct {
	Kind        ScopeKind
	Parent      *Scope
	Children    []*Scope
	Members     map[string]ScopeMember
	Identifiers map[string]string
	Generated   []Ref
	// ArgsIdentifier map[string]string
	// This is used to store the ref of the label symbol for ScopeLabel scopes.
	LabelRef        Ref
	LabelStmtIsLoop bool

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

type AST struct {
	ApproximateLineCount  int32
	NestedScopeSlotCounts SlotCounts
	HasLazyExport         bool

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
	URLForCSS   string
	Parts       []Part
	Symbols     []Symbol
	ModuleScope *Scope
	CharFreq    *CharFreq
	ExportsRef  Ref
	ModuleRef   Ref
	WrapperRef  Ref

	// These are stored at the AST level instead of on individual AST nodes so
	// they can be manipulated efficiently without a full AST traversal
	ImportRecords []ast.ImportRecord

	// These are used when bundling. They are filled in during the parser pass
	// since we already have to traverse the AST then anyway and the parser pass
	// is conveniently fully parallelized.
	NamedImports            map[Ref]NamedImport
	NamedExports            map[string]Ref
	TopLevelSymbolToParts   map[Ref][]uint32
	ExportStarImportRecords []uint32

	SourceMapComment Span
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

var DefaultNameMinifier = NameMinifier{
	head: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_$",
	tail: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_$",
}

type charAndCount struct {
	index byte
	count int32
	char  string
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

func (freq *CharFreq) Compile() NameMinifier {
	// Sort the histogram in descending order by count
	array := make(charAndCountArray, 64)
	for i := 0; i < len(DefaultNameMinifier.tail); i++ {
		array[i] = charAndCount{
			char:  DefaultNameMinifier.tail[i : i+1],
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

func (minifier *NameMinifier) NumberToMinifiedName(i int) string {
	j := i % 54
	name := minifier.head[j : j+1]
	i = i / 54

	for i > 0 {
		i--
		j := i % 64
		name += minifier.tail[j : j+1]
		i = i / 64
	}

	return name
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
	// Parts within this file that use this import
	LocalPartsWithUses []uint32

	Alias             string
	AliasLoc          logger.Loc
	NamespaceRef      Ref
	ImportRecordIndex uint32

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
	Stmts  []Stmt
	Scopes []*Scope

	// Each is an index into the file-level import record list
	ImportRecordIndices []uint32

	// All symbols that are declared in this part. Note that a given symbol may
	// have multiple declarations, and so may end up being declared in multiple
	// parts (e.g. multiple "var" declarations with the same name). Also note
	// that this list isn't deduplicated and may contain duplicates.
	DeclaredSymbols []DeclaredSymbol

	// An estimate of the number of uses of all symbols used within this part.
	SymbolUses map[Ref]SymbolUse

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

type SymbolUse struct {
	CountEstimate uint32
	IsAssigned    bool
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
	newSymbol.UseCountEstimate += oldSymbol.UseCountEstimate
	if oldSymbol.MustNotBeRenamed {
		newSymbol.MustNotBeRenamed = true
	}
	return new
}

// This has a custom implementation instead of using "filepath.Dir/Base/Ext"
// because it should work the same on Unix and Windows. These names end up in
// the generated output and the generated output should not depend on the OS.
func PlatformIndependentPathDirBaseExt(path string) (dir string, base string, ext string) {
	for {
		i := strings.LastIndexAny(path, "/\\")

		// Stop if there are no more slashes
		if i < 0 {
			base = path
			break
		}

		// Stop if we found a non-trailing slash
		if i+1 != len(path) {
			dir, base = path[:i], path[i+1:]
			break
		}

		// Ignore trailing slashes
		path = path[:i]
	}

	// Strip off the extension
	if dot := strings.LastIndexByte(base, '.'); dot >= 0 {
		base, ext = base[:dot], base[dot:]
	}

	return
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
	dir, base, _ := PlatformIndependentPathDirBaseExt(path)

	// If the name is "index", use the directory name instead. This is because
	// many packages in npm use the file name "index.js" because it triggers
	// node's implicit module resolution rules that allows you to import it by
	// just naming the directory.
	if base == "index" {
		_, dirBase, _ := PlatformIndependentPathDirBaseExt(dir)
		if dirBase != "" {
			base = dirBase
		}
	}

	// Convert it to an ASCII identifier. Note: If you change this to a non-ASCII
	// identifier, you're going to potentially cause trouble with non-BMP code
	// points in target environments that don't support bracketed Unicode escapes.
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
