package js_ast

import (
	"strconv"

	"github.com/evanw/esbuild/internal/ast"
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

type L uint8

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

type OpCode uint8

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

func (op OpCode) IsShortCircuit() bool {
	switch op {
	case BinOpLogicalOr, BinOpLogicalOrAssign,
		BinOpLogicalAnd, BinOpLogicalAndAssign,
		BinOpNullishCoalescing, BinOpNullishCoalescingAssign:
		return true
	}
	return false
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

type OpTableEntry struct {
	Text      string
	Level     L
	IsKeyword bool
}

var OpTable = []OpTableEntry{
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

type Decorator struct {
	Value            Expr
	AtLoc            logger.Loc
	OmitNewlineAfter bool
}

type PropertyKind uint8

const (
	PropertyField PropertyKind = iota
	PropertyMethod
	PropertyGetter
	PropertySetter
	PropertyAutoAccessor
	PropertySpread
	PropertyDeclareOrAbstract
	PropertyClassStaticBlock
)

// This returns true if and only if this property matches the "MethodDefinition"
// grammar from the specification. That means it's one of the following forms:
//
//	foo() {}
//	*foo() {}
//	async foo() {}
//	async *foo() {}
//	get foo() {}
//	set foo(_) {}
//
// If this returns true, the "ValueOrNil" field of the property is always an
// "EFunction" expression and it is always printed as a method.
func (kind PropertyKind) IsMethodDefinition() bool {
	return kind == PropertyMethod || kind == PropertyGetter || kind == PropertySetter
}

type ClassStaticBlock struct {
	Block SBlock
	Loc   logger.Loc
}

type PropertyFlags uint8

const (
	PropertyIsComputed PropertyFlags = 1 << iota
	PropertyIsStatic
	PropertyWasShorthand
	PropertyPreferQuotedKey
)

func (flags PropertyFlags) Has(flag PropertyFlags) bool {
	return (flags & flag) != 0
}

type Property struct {
	ClassStaticBlock *ClassStaticBlock

	Key Expr

	// This is omitted for class fields
	ValueOrNil Expr

	// This is used when parsing a pattern that uses default values:
	//
	//   [a = 1] = [];
	//   ({a = 1} = {});
	//
	// It's also used for class fields:
	//
	//   class Foo { a = 1 }
	//
	InitializerOrNil Expr

	Decorators []Decorator

	Loc             logger.Loc
	CloseBracketLoc logger.Loc
	Kind            PropertyKind
	Flags           PropertyFlags
}

type PropertyBinding struct {
	Key               Expr
	Value             Binding
	DefaultValueOrNil Expr
	Loc               logger.Loc
	CloseBracketLoc   logger.Loc
	IsComputed        bool
	IsSpread          bool
	PreferQuotedKey   bool
}

type Arg struct {
	Binding      Binding
	DefaultOrNil Expr
	Decorators   []Decorator

	// "constructor(public x: boolean) {}"
	IsTypeScriptCtorField bool
}

type Fn struct {
	Name         *ast.LocRef
	Args         []Arg
	Body         FnBody
	ArgumentsRef ast.Ref
	OpenParenLoc logger.Loc

	IsAsync     bool
	IsGenerator bool
	HasRestArg  bool
	HasIfScope  bool

	// See: https://github.com/rollup/rollup/pull/5024
	HasNoSideEffectsComment bool

	// This is true if the function is a method
	IsUniqueFormalParameters bool
}

type FnBody struct {
	Block SBlock
	Loc   logger.Loc
}

type Class struct {
	Decorators    []Decorator
	Name          *ast.LocRef
	ExtendsOrNil  Expr
	Properties    []Property
	ClassKeyword  logger.Range
	BodyLoc       logger.Loc
	CloseBraceLoc logger.Loc

	// If true, JavaScript decorators (i.e. not TypeScript experimental
	// decorators) should be lowered. This is the case either if JavaScript
	// decorators are not supported in the configured target environment, or
	// if "useDefineForClassFields" is set to false and this class has
	// decorators on it. Note that this flag is not necessarily set to true if
	// "useDefineForClassFields" is false and a class has an "accessor" even
	// though the accessor feature comes from the decorator specification.
	ShouldLowerStandardDecorators bool

	// If true, property field initializers cannot be assumed to have no side
	// effects. For example:
	//
	//   class Foo {
	//     static set foo(x) { importantSideEffect(x) }
	//   }
	//   class Bar extends Foo {
	//     foo = 1
	//   }
	//
	// This happens in TypeScript when "useDefineForClassFields" is disabled
	// because TypeScript (and esbuild) transforms the above class into this:
	//
	//   class Foo {
	//     static set foo(x) { importantSideEffect(x); }
	//   }
	//   class Bar extends Foo {
	//   }
	//   Bar.foo = 1;
	//
	UseDefineForClassFields bool
}

type ArrayBinding struct {
	Binding           Binding
	DefaultValueOrNil Expr
	Loc               logger.Loc
}

type Binding struct {
	Data B
	Loc  logger.Loc
}

// This interface is never called. Its purpose is to encode a variant type in
// Go's type system.
type B interface{ isBinding() }

func (*BMissing) isBinding()    {}
func (*BIdentifier) isBinding() {}
func (*BArray) isBinding()      {}
func (*BObject) isBinding()     {}

type BMissing struct{}

type BIdentifier struct{ Ref ast.Ref }

type BArray struct {
	Items           []ArrayBinding
	CloseBracketLoc logger.Loc
	HasSpread       bool
	IsSingleLine    bool
}

type BObject struct {
	Properties    []PropertyBinding
	CloseBraceLoc logger.Loc
	IsSingleLine  bool
}

type Expr struct {
	Data E
	Loc  logger.Loc
}

// This interface is never called. Its purpose is to encode a variant type in
// Go's type system.
type E interface{ isExpr() }

func (*EArray) isExpr()                {}
func (*EUnary) isExpr()                {}
func (*EBinary) isExpr()               {}
func (*EBoolean) isExpr()              {}
func (*ESuper) isExpr()                {}
func (*ENull) isExpr()                 {}
func (*EUndefined) isExpr()            {}
func (*EThis) isExpr()                 {}
func (*ENew) isExpr()                  {}
func (*ENewTarget) isExpr()            {}
func (*EImportMeta) isExpr()           {}
func (*ECall) isExpr()                 {}
func (*EDot) isExpr()                  {}
func (*EIndex) isExpr()                {}
func (*EArrow) isExpr()                {}
func (*EFunction) isExpr()             {}
func (*EClass) isExpr()                {}
func (*EIdentifier) isExpr()           {}
func (*EImportIdentifier) isExpr()     {}
func (*EPrivateIdentifier) isExpr()    {}
func (*ENameOfSymbol) isExpr()         {}
func (*EJSXElement) isExpr()           {}
func (*EJSXText) isExpr()              {}
func (*EMissing) isExpr()              {}
func (*ENumber) isExpr()               {}
func (*EBigInt) isExpr()               {}
func (*EObject) isExpr()               {}
func (*ESpread) isExpr()               {}
func (*EString) isExpr()               {}
func (*ETemplate) isExpr()             {}
func (*ERegExp) isExpr()               {}
func (*EInlinedEnum) isExpr()          {}
func (*EAnnotation) isExpr()           {}
func (*EAwait) isExpr()                {}
func (*EYield) isExpr()                {}
func (*EIf) isExpr()                   {}
func (*ERequireString) isExpr()        {}
func (*ERequireResolveString) isExpr() {}
func (*EImportString) isExpr()         {}
func (*EImportCall) isExpr()           {}

type EArray struct {
	Items            []Expr
	CommaAfterSpread logger.Loc
	CloseBracketLoc  logger.Loc
	IsSingleLine     bool
	IsParenthesized  bool
}

type EUnary struct {
	Value Expr
	Op    OpCode

	// The expression "typeof (0, x)" must not become "typeof x" if "x"
	// is unbound because that could suppress a ReferenceError from "x".
	//
	// Also if we know a typeof operator was originally an identifier, then
	// we know that this typeof operator always has no side effects (even if
	// we consider the identifier by itself to have a side effect).
	//
	// Note that there *is* actually a case where "typeof x" can throw an error:
	// when "x" is being referenced inside of its TDZ (temporal dead zone). TDZ
	// checks are not yet handled correctly by esbuild, so this possibility is
	// currently ignored.
	WasOriginallyTypeofIdentifier bool

	// Similarly the expression "delete (0, x)" must not become "delete x"
	// because that syntax is invalid in strict mode. We also need to make sure
	// we don't accidentally change the return value:
	//
	//   Returns false:
	//     "var a; delete (a)"
	//     "var a = Object.freeze({b: 1}); delete (a.b)"
	//     "var a = Object.freeze({b: 1}); delete (a?.b)"
	//     "var a = Object.freeze({b: 1}); delete (a['b'])"
	//     "var a = Object.freeze({b: 1}); delete (a?.['b'])"
	//
	//   Returns true:
	//     "var a; delete (0, a)"
	//     "var a = Object.freeze({b: 1}); delete (true && a.b)"
	//     "var a = Object.freeze({b: 1}); delete (false || a?.b)"
	//     "var a = Object.freeze({b: 1}); delete (null ?? a?.['b'])"
	//     "var a = Object.freeze({b: 1}); delete (true ? a['b'] : a['b'])"
	//
	WasOriginallyDeleteOfIdentifierOrPropertyAccess bool
}

type EBinary struct {
	Left  Expr
	Right Expr
	Op    OpCode
}

type EBoolean struct{ Value bool }

type EMissing struct{}

type ESuper struct{}

type ENull struct{}

type EUndefined struct{}

type EThis struct{}

type ENewTarget struct {
	Range logger.Range
}

type EImportMeta struct {
	RangeLen int32
}

// These help reduce unnecessary memory allocations
var BMissingShared = &BMissing{}
var EMissingShared = &EMissing{}
var ENullShared = &ENull{}
var ESuperShared = &ESuper{}
var EThisShared = &EThis{}
var EUndefinedShared = &EUndefined{}
var SDebuggerShared = &SDebugger{}
var SEmptyShared = &SEmpty{}
var STypeScriptShared = &STypeScript{}
var STypeScriptSharedWasDeclareClass = &STypeScript{WasDeclareClass: true}

type ENew struct {
	Target Expr
	Args   []Expr

	CloseParenLoc logger.Loc
	IsMultiLine   bool

	// True if there is a comment containing "@__PURE__" or "#__PURE__" preceding
	// this call expression. See the comment inside ECall for more details.
	CanBeUnwrappedIfUnused bool
}

type CallKind uint8

const (
	NormalCall CallKind = iota
	DirectEval
	TargetWasOriginallyPropertyAccess
)

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
	CloseParenLoc logger.Loc
	OptionalChain OptionalChain
	Kind          CallKind
	IsMultiLine   bool

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
		a.Kind == b.Kind &&
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

	// Symbol values are known to not have side effects when used as property
	// names in class declarations and object literals.
	IsSymbolInstance bool
}

func (a *EDot) HasSameFlagsAs(b *EDot) bool {
	return a.OptionalChain == b.OptionalChain &&
		a.CanBeRemovedIfUnused == b.CanBeRemovedIfUnused &&
		a.CallCanBeUnwrappedIfUnused == b.CallCanBeUnwrappedIfUnused &&
		a.IsSymbolInstance == b.IsSymbolInstance
}

type EIndex struct {
	Target          Expr
	Index           Expr
	CloseBracketLoc logger.Loc
	OptionalChain   OptionalChain

	// If true, this property access is known to be free of side-effects. That
	// means it can be removed if the resulting value isn't used.
	CanBeRemovedIfUnused bool

	// If true, this property access is a function that, when called, can be
	// unwrapped if the resulting value is unused. Unwrapping means discarding
	// the call target but keeping any arguments with side effects.
	CallCanBeUnwrappedIfUnused bool

	// Symbol values are known to not have side effects when used as property
	// names in class declarations and object literals.
	IsSymbolInstance bool
}

func (a *EIndex) HasSameFlagsAs(b *EIndex) bool {
	return a.OptionalChain == b.OptionalChain &&
		a.CanBeRemovedIfUnused == b.CanBeRemovedIfUnused &&
		a.CallCanBeUnwrappedIfUnused == b.CallCanBeUnwrappedIfUnused &&
		a.IsSymbolInstance == b.IsSymbolInstance
}

type EArrow struct {
	Args []Arg
	Body FnBody

	IsAsync    bool
	HasRestArg bool
	PreferExpr bool // Use shorthand if true and "Body" is a single return statement

	// See: https://github.com/rollup/rollup/pull/5024
	HasNoSideEffectsComment bool
}

type EFunction struct{ Fn Fn }

type EClass struct{ Class Class }

type EIdentifier struct {
	Ref ast.Ref

	// If we're inside a "with" statement, this identifier may be a property
	// access. In that case it would be incorrect to remove this identifier since
	// the property access may be a getter or setter with side effects.
	MustKeepDueToWithStmt bool

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
	Ref             ast.Ref
	PreferQuotedKey bool

	// If true, this was originally an identifier expression such as "foo". If
	// false, this could potentially have been a member access expression such
	// as "ns.foo" off of an imported namespace object.
	WasOriginallyIdentifier bool
}

// This is similar to EIdentifier but it represents class-private fields and
// methods. It can be used where computed properties can be used, such as
// EIndex and Property.
type EPrivateIdentifier struct {
	Ref ast.Ref
}

// This represents an internal property name that can be mangled. The symbol
// referenced by this expression should be a "SymbolMangledProp" symbol.
type ENameOfSymbol struct {
	Ref                   ast.Ref
	HasPropertyKeyComment bool // If true, a preceding comment contains "@__KEY__"
}

type EJSXElement struct {
	TagOrNil   Expr
	Properties []Property

	// Note: This array may contain nil entries. Be careful about nil entries
	// when iterating over this array.
	//
	// Each nil entry corresponds to the "JSXChildExpression_opt" part of the
	// grammar (https://facebook.github.io/jsx/#prod-JSXChild):
	//
	//   JSXChild :
	//       JSXText
	//       JSXElement
	//       JSXFragment
	//       { JSXChildExpression_opt }
	//
	// This is the "{}" part in "<a>{}</a>". We allow this because some people
	// put comments there and then expect to be able to process them from
	// esbuild's output. These absent AST nodes are completely omitted when
	// JSX is transformed to JS. They are only present when JSX preservation is
	// enabled.
	NullableChildren []Expr

	CloseLoc        logger.Loc
	IsTagSingleLine bool
}

// The JSX specification doesn't say how JSX text is supposed to be interpreted
// so our "preserve" JSX transform should reproduce the original source code
// verbatim. One reason why this matters is because there is no canonical way
// to interpret JSX text (Babel and TypeScript differ in what newlines mean).
// Another reason is that some people want to do custom things such as this:
// https://github.com/evanw/esbuild/issues/3605
type EJSXText struct {
	Raw string
}

type ENumber struct{ Value float64 }

type EBigInt struct{ Value string }

type EObject struct {
	Properties       []Property
	CommaAfterSpread logger.Loc
	CloseBraceLoc    logger.Loc
	IsSingleLine     bool
	IsParenthesized  bool
}

type ESpread struct{ Value Expr }

// This is used for both strings and no-substitution template literals to reduce
// the number of cases that need to be checked for string optimization code
type EString struct {
	Value                 []uint16
	LegacyOctalLoc        logger.Loc
	PreferTemplate        bool
	HasPropertyKeyComment bool // If true, a preceding comment contains "@__KEY__"
	ContainsUniqueKey     bool // If true, this string must not be wrapped
}

type TemplatePart struct {
	Value      Expr
	TailRaw    string   // Only use when "TagOrNil" is not nil
	TailCooked []uint16 // Only use when "TagOrNil" is nil
	TailLoc    logger.Loc
}

type ETemplate struct {
	TagOrNil       Expr
	HeadRaw        string   // Only use when "TagOrNil" is not nil
	HeadCooked     []uint16 // Only use when "TagOrNil" is nil
	Parts          []TemplatePart
	HeadLoc        logger.Loc
	LegacyOctalLoc logger.Loc

	// True if this is a tagged template literal with a comment that indicates
	// this function call can be removed if the result is unused. Note that the
	// arguments are not considered to be part of the call. If the call itself
	// is removed due to this annotation, the arguments must remain if they have
	// side effects (including the string conversions).
	CanBeUnwrappedIfUnused bool

	// If the tag is present, it is expected to be a function and is called. If
	// the tag is a syntactic property access, then the value for "this" in the
	// function call is the object whose property was accessed (e.g. in "a.b``"
	// the value for "this" in "a.b" is "a"). We need to ensure that if "a``"
	// ever becomes "b.c``" later on due to optimizations, it is written as
	// "(0, b.c)``" to avoid a behavior change.
	TagWasOriginallyPropertyAccess bool
}

type ERegExp struct{ Value string }

type EInlinedEnum struct {
	Value   Expr
	Comment string
}

type AnnotationFlags uint8

const (
	// This is sort of like an IIFE with a "/* @__PURE__ */" comment except it's an
	// inline annotation on an expression itself without the nested scope. Sometimes
	// we can't easily introduce a new scope (e.g. if the expression uses "await").
	CanBeRemovedIfUnusedFlag AnnotationFlags = 1 << iota
)

func (flags AnnotationFlags) Has(flag AnnotationFlags) bool {
	return (flags & flag) != 0
}

type EAnnotation struct {
	Value Expr
	Flags AnnotationFlags
}

type EAwait struct {
	Value Expr
}

type EYield struct {
	ValueOrNil Expr
	IsStar     bool
}

type EIf struct {
	Test Expr
	Yes  Expr
	No   Expr
}

type ERequireString struct {
	ImportRecordIndex uint32
	CloseParenLoc     logger.Loc
}

type ERequireResolveString struct {
	ImportRecordIndex uint32
	CloseParenLoc     logger.Loc
}

type EImportString struct {
	ImportRecordIndex uint32
	CloseParenLoc     logger.Loc
}

type EImportCall struct {
	Expr          Expr
	OptionsOrNil  Expr
	CloseParenLoc logger.Loc
}

type Stmt struct {
	Data S
	Loc  logger.Loc
}

// This interface is never called. Its purpose is to encode a variant type in
// Go's type system.
type S interface{ isStmt() }

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

type SBlock struct {
	Stmts         []Stmt
	CloseBraceLoc logger.Loc
}

type SEmpty struct{}

// This is a stand-in for a TypeScript type declaration
type STypeScript struct {
	WasDeclareClass bool
}

type SComment struct {
	Text           string
	IsLegalComment bool
}

type SDebugger struct{}

type SDirective struct {
	Value          []uint16
	LegacyOctalLoc logger.Loc
}

type SExportClause struct {
	Items        []ClauseItem
	IsSingleLine bool
}

type SExportFrom struct {
	Items             []ClauseItem
	NamespaceRef      ast.Ref
	ImportRecordIndex uint32
	IsSingleLine      bool
}

type SExportDefault struct {
	Value       Stmt // May be a SExpr or SFunction or SClass
	DefaultName ast.LocRef
}

type ExportStarAlias struct {
	// Although this alias name starts off as being the same as the statement's
	// namespace symbol, it may diverge if the namespace symbol name is minified.
	// The original alias name is preserved here to avoid this scenario.
	OriginalName string

	Loc logger.Loc
}

type SExportStar struct {
	Alias             *ExportStarAlias
	NamespaceRef      ast.Ref
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

	// This is set to true for automatically-generated expressions that are part
	// of class syntax lowering. A single class declaration may end up with many
	// generated expressions after it (e.g. class field initializations, a call
	// to keep the original value of the "name" property). When this happens we
	// can't tell that the class is side-effect free anymore because all of these
	// methods mutate the class. We use this annotation for that instead.
	IsFromClassOrFnThatCanBeRemovedIfUnused bool
}

type EnumValue struct {
	ValueOrNil Expr
	Name       []uint16
	Ref        ast.Ref
	Loc        logger.Loc
}

type SEnum struct {
	Values   []EnumValue
	Name     ast.LocRef
	Arg      ast.Ref
	IsExport bool
}

type SNamespace struct {
	Stmts    []Stmt
	Name     ast.LocRef
	Arg      ast.Ref
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
	Stmt             Stmt
	Name             ast.LocRef
	IsSingleLineStmt bool
}

type SIf struct {
	Test            Expr
	Yes             Stmt
	NoOrNil         Stmt
	IsSingleLineYes bool
	IsSingleLineNo  bool
}

type SFor struct {
	InitOrNil        Stmt // May be a SConst, SLet, SVar, or SExpr
	TestOrNil        Expr
	UpdateOrNil      Expr
	Body             Stmt
	IsSingleLineBody bool
}

type SForIn struct {
	Init             Stmt // May be a SConst, SLet, SVar, or SExpr
	Value            Expr
	Body             Stmt
	IsSingleLineBody bool
}

type SForOf struct {
	Init             Stmt // May be a SConst, SLet, SVar, or SExpr
	Value            Expr
	Body             Stmt
	Await            logger.Range
	IsSingleLineBody bool
}

type SDoWhile struct {
	Body Stmt
	Test Expr
}

type SWhile struct {
	Test             Expr
	Body             Stmt
	IsSingleLineBody bool
}

type SWith struct {
	Value            Expr
	Body             Stmt
	BodyLoc          logger.Loc
	IsSingleLineBody bool
}

type Catch struct {
	BindingOrNil Binding
	Block        SBlock
	Loc          logger.Loc
	BlockLoc     logger.Loc
}

type Finally struct {
	Block SBlock
	Loc   logger.Loc
}

type STry struct {
	Catch    *Catch
	Finally  *Finally
	Block    SBlock
	BlockLoc logger.Loc
}

type Case struct {
	ValueOrNil Expr // If this is nil, this is "default" instead of "case"
	Body       []Stmt
	Loc        logger.Loc
}

type SSwitch struct {
	Test          Expr
	Cases         []Case
	BodyLoc       logger.Loc
	CloseBraceLoc logger.Loc
}

// This object represents all of these types of import statements:
//
//	import 'path'
//	import {item1, item2} from 'path'
//	import * as ns from 'path'
//	import defaultItem, {item1, item2} from 'path'
//	import defaultItem, * as ns from 'path'
//
// Many parts are optional and can be combined in different ways. The only
// restriction is that you cannot have both a clause and a star namespace.
type SImport struct {
	DefaultName *ast.LocRef
	Items       *[]ClauseItem
	StarNameLoc *logger.Loc

	// If this is a star import: This is a Ref for the namespace symbol. The Loc
	// for the symbol is StarLoc.
	//
	// Otherwise: This is an auto-generated Ref for the namespace representing
	// the imported file. In this case StarLoc is nil. The NamespaceRef is used
	// when converting this module to a CommonJS module.
	NamespaceRef ast.Ref

	ImportRecordIndex uint32
	IsSingleLine      bool
}

type SReturn struct {
	ValueOrNil Expr
}

type SThrow struct {
	Value Expr
}

type LocalKind uint8

const (
	LocalVar LocalKind = iota
	LocalLet
	LocalConst
	LocalUsing
	LocalAwaitUsing
)

func (kind LocalKind) IsUsing() bool {
	return kind >= LocalUsing
}

type SLocal struct {
	Decls    []Decl
	Kind     LocalKind
	IsExport bool

	// The TypeScript compiler doesn't generate code for "import foo = bar"
	// statements where the import is never used.
	WasTSImportEquals bool
}

type SBreak struct {
	Label *ast.LocRef
}

type SContinue struct {
	Label *ast.LocRef
}

type ClauseItem struct {
	Alias string

	// This is the original name of the symbol stored in "Name". It's needed for
	// "SExportClause" statements such as this:
	//
	//   export {foo as bar} from 'path'
	//
	// In this case both "foo" and "bar" are aliases because it's a re-export.
	// We need to preserve both aliases in case the symbol is renamed. In this
	// example, "foo" is "OriginalName" and "bar" is "Alias".
	OriginalName string

	AliasLoc logger.Loc
	Name     ast.LocRef
}

type Decl struct {
	Binding    Binding
	ValueOrNil Expr
}

type ScopeKind uint8

const (
	ScopeBlock ScopeKind = iota
	ScopeWith
	ScopeLabel
	ScopeClassName
	ScopeClassBody
	ScopeCatchBinding

	// The scopes below stop hoisted variables from extending into parent scopes
	ScopeEntry // This is a module, TypeScript enum, or TypeScript namespace
	ScopeFunctionArgs
	ScopeFunctionBody
	ScopeClassStaticInit
)

func (kind ScopeKind) StopsHoisting() bool {
	return kind >= ScopeEntry
}

type ScopeMember struct {
	Ref ast.Ref
	Loc logger.Loc
}

type Scope struct {
	// This will be non-nil if this is a TypeScript "namespace" or "enum"
	TSNamespace *TSNamespaceScope

	Parent    *Scope
	Children  []*Scope
	Members   map[string]ScopeMember
	Replaced  []ScopeMember
	Generated []ast.Ref

	// The location of the "use strict" directive for ExplicitStrictMode
	UseStrictLoc logger.Loc

	// This is used to store the ref of the label symbol for ScopeLabel scopes.
	Label           ast.LocRef
	LabelStmtIsLoop bool

	// If a scope contains a direct eval() expression, then none of the symbols
	// inside that scope can be renamed. We conservatively assume that the
	// evaluated code might reference anything that it has access to.
	ContainsDirectEval bool

	// This is to help forbid "arguments" inside class body scopes
	ForbidArguments bool

	// As a special case, we enable constant propagation for any chain of "const"
	// declarations at the start of a statement list. This special case doesn't
	// have any TDZ considerations because no other statements come before it.
	IsAfterConstLocalPrefix bool

	StrictMode StrictModeKind
	Kind       ScopeKind
}

type StrictModeKind uint8

const (
	SloppyMode StrictModeKind = iota
	ExplicitStrictMode
	ImplicitStrictModeClass
	ImplicitStrictModeESM
	ImplicitStrictModeTSAlwaysStrict
	ImplicitStrictModeJSXAutomaticRuntime
)

func (s *Scope) RecursiveSetStrictMode(kind StrictModeKind) {
	if s.StrictMode == SloppyMode {
		s.StrictMode = kind
		for _, child := range s.Children {
			child.RecursiveSetStrictMode(kind)
		}
	}
}

// This is for TypeScript "enum" and "namespace" blocks. Each block can
// potentially be instantiated multiple times. The exported members of each
// block are merged into a single namespace while the non-exported code is
// still scoped to just within that block:
//
//	let x = 1;
//	namespace Foo {
//	  let x = 2;
//	  export let y = 3;
//	}
//	namespace Foo {
//	  console.log(x); // 1
//	  console.log(y); // 3
//	}
//
// Doing this also works inside an enum:
//
//	enum Foo {
//	  A = 3,
//	  B = A + 1,
//	}
//	enum Foo {
//	  C = A + 2,
//	}
//	console.log(Foo.B) // 4
//	console.log(Foo.C) // 5
//
// This is a form of identifier lookup that works differently than the
// hierarchical scope-based identifier lookup in JavaScript. Lookup now needs
// to search sibling scopes in addition to parent scopes. This is accomplished
// by sharing the map of exported members between all matching sibling scopes.
type TSNamespaceScope struct {
	// This is shared between all sibling namespace blocks
	ExportedMembers TSNamespaceMembers

	// This is a lazily-generated map of identifiers that actually represent
	// property accesses to this namespace's properties. For example:
	//
	//   namespace x {
	//     export let y = 123
	//   }
	//   namespace x {
	//     export let z = y
	//   }
	//
	// This should be compiled into the following code:
	//
	//   var x;
	//   (function(x2) {
	//     x2.y = 123;
	//   })(x || (x = {}));
	//   (function(x3) {
	//     x3.z = x3.y;
	//   })(x || (x = {}));
	//
	// When we try to find the symbol "y", we instead return one of these lazily
	// generated proxy symbols that represent the property access "x3.y". This
	// map is unique per namespace block because "x3" is the argument symbol that
	// is specific to that particular namespace block.
	LazilyGeneratedProperyAccesses map[string]ast.Ref

	// This is specific to this namespace block. It's the argument of the
	// immediately-invoked function expression that the namespace block is
	// compiled into:
	//
	//   var ns;
	//   (function (ns2) {
	//     ns2.x = 123;
	//   })(ns || (ns = {}));
	//
	// This variable is "ns2" in the above example. It's the symbol to use when
	// generating property accesses off of this namespace when it's in scope.
	ArgRef ast.Ref

	// Even though enums are like namespaces and both enums and namespaces allow
	// implicit references to properties of sibling scopes, they behave like
	// separate, er, namespaces. Implicit references only work namespace-to-
	// namespace and enum-to-enum. They do not work enum-to-namespace. And I'm
	// not sure what's supposed to happen for the namespace-to-enum case because
	// the compiler crashes: https://github.com/microsoft/TypeScript/issues/46891.
	// So basically these both work:
	//
	//   enum a { b = 1 }
	//   enum a { c = b }
	//
	//   namespace x { export let y = 1 }
	//   namespace x { export let z = y }
	//
	// This doesn't work:
	//
	//   enum a { b = 1 }
	//   namespace a { export let c = b }
	//
	// And this crashes the TypeScript compiler:
	//
	//   namespace a { export let b = 1 }
	//   enum a { c = b }
	//
	// Therefore we only allow enum/enum and namespace/namespace interactions.
	IsEnumScope bool
}

type TSNamespaceMembers map[string]TSNamespaceMember

type TSNamespaceMember struct {
	Data        TSNamespaceMemberData
	Loc         logger.Loc
	IsEnumValue bool
}

type TSNamespaceMemberData interface {
	isTSNamespaceMember()
}

func (TSNamespaceMemberProperty) isTSNamespaceMember()   {}
func (TSNamespaceMemberNamespace) isTSNamespaceMember()  {}
func (TSNamespaceMemberEnumNumber) isTSNamespaceMember() {}
func (TSNamespaceMemberEnumString) isTSNamespaceMember() {}

// "namespace ns { export let it }"
type TSNamespaceMemberProperty struct{}

// "namespace ns { export namespace it {} }"
type TSNamespaceMemberNamespace struct {
	ExportedMembers TSNamespaceMembers
}

// "enum ns { it }"
type TSNamespaceMemberEnumNumber struct {
	Value float64
}

// "enum ns { it = 'it' }"
type TSNamespaceMemberEnumString struct {
	Value []uint16
}

type ExportsKind uint8

const (
	// This file doesn't have any kind of export, so it's impossible to say what
	// kind of file this is. An empty file is in this category, for example.
	ExportsNone ExportsKind = iota

	// The exports are stored on "module" and/or "exports". Calling "require()"
	// on this module returns "module.exports". All imports to this module are
	// allowed but may return undefined.
	ExportsCommonJS

	// All export names are known explicitly. Calling "require()" on this module
	// generates an exports object (stored in "exports") with getters for the
	// export names. Named imports to this module are only allowed if they are
	// in the set of export names.
	ExportsESM

	// Some export names are known explicitly, but others fall back to a dynamic
	// run-time object. This is necessary when using the "export * from" syntax
	// with either a CommonJS module or an external module (i.e. a module whose
	// export names are not known at compile-time).
	//
	// Calling "require()" on this module generates an exports object (stored in
	// "exports") with getters for the export names. All named imports to this
	// module are allowed. Direct named imports reference the corresponding export
	// directly. Other imports go through property accesses on "exports".
	ExportsESMWithDynamicFallback
)

func (kind ExportsKind) IsDynamic() bool {
	return kind == ExportsCommonJS || kind == ExportsESMWithDynamicFallback
}

type ModuleType uint8

const (
	ModuleUnknown ModuleType = iota

	// ".cjs" or ".cts" or "type: commonjs" in package.json
	ModuleCommonJS_CJS
	ModuleCommonJS_CTS
	ModuleCommonJS_PackageJSON

	// ".mjs" or ".mts" or "type: module" in package.json
	ModuleESM_MJS
	ModuleESM_MTS
	ModuleESM_PackageJSON
)

func (mt ModuleType) IsCommonJS() bool {
	return mt >= ModuleCommonJS_CJS && mt <= ModuleCommonJS_PackageJSON
}

func (mt ModuleType) IsESM() bool {
	return mt >= ModuleESM_MJS && mt <= ModuleESM_PackageJSON
}

type ModuleTypeData struct {
	Source *logger.Source
	Range  logger.Range
	Type   ModuleType
}

// This is the index to the automatically-generated part containing code that
// calls "__export(exports, { ... getters ... })". This is used to generate
// getters on an exports object for ES6 export statements, and is both for
// ES6 star imports and CommonJS-style modules. All files have one of these,
// although it may contain no statements if there is nothing to export.
const NSExportPartIndex = uint32(0)

type AST struct {
	ModuleTypeData ModuleTypeData
	Parts          []Part
	Symbols        []ast.Symbol
	ExprComments   map[logger.Loc][]string
	ModuleScope    *Scope
	CharFreq       *ast.CharFreq

	// This is internal-only data used for the implementation of Yarn PnP
	ManifestForYarnPnP Expr

	Hashbang   string
	Directives []string
	URLForCSS  string

	// Note: If you're in the linker, do not use this map directly. This map is
	// filled in by the parser and is considered immutable. For performance reasons,
	// the linker doesn't mutate this map (cloning a map is slow in Go). Instead the
	// linker super-imposes relevant information on top in a method call. You should
	// call "TopLevelSymbolToParts" instead.
	TopLevelSymbolToPartsFromParser map[ast.Ref][]uint32

	// This contains all top-level exported TypeScript enum constants. It exists
	// to enable cross-module inlining of constant enums.
	TSEnums map[ast.Ref]map[string]TSEnumValue

	// This contains the values of all detected inlinable constants. It exists
	// to enable cross-module inlining of these constants.
	ConstValues map[ast.Ref]ConstValue

	// Properties in here are represented as symbols instead of strings, which
	// allows them to be renamed to smaller names.
	MangledProps map[string]ast.Ref

	// Properties in here are existing non-mangled properties in the source code
	// and must not be used when generating mangled names to avoid a collision.
	ReservedProps map[string]bool

	// These are stored at the AST level instead of on individual AST nodes so
	// they can be manipulated efficiently without a full AST traversal
	ImportRecords []ast.ImportRecord

	// These are used when bundling. They are filled in during the parser pass
	// since we already have to traverse the AST then anyway and the parser pass
	// is conveniently fully parallelized.
	NamedImports            map[ast.Ref]NamedImport
	NamedExports            map[string]NamedExport
	ExportStarImportRecords []uint32

	SourceMapComment logger.Span

	// This is a list of ES6 features. They are ranges instead of booleans so
	// that they can be used in log messages. Check to see if "Len > 0".
	ExportKeyword            logger.Range // Does not include TypeScript-specific syntax
	TopLevelAwaitKeyword     logger.Range
	LiveTopLevelAwaitKeyword logger.Range // Excludes top-level await in dead branches

	ExportsRef ast.Ref
	ModuleRef  ast.Ref
	WrapperRef ast.Ref

	ApproximateLineCount  int32
	NestedScopeSlotCounts ast.SlotCounts
	HasLazyExport         bool

	// This is a list of CommonJS features. When a file uses CommonJS features,
	// it's not a candidate for "flat bundling" and must be wrapped in its own
	// closure. Note that this also includes top-level "return" but these aren't
	// here because only the parser checks those.
	UsesExportsRef bool
	UsesModuleRef  bool
	ExportsKind    ExportsKind
}

type TSEnumValue struct {
	String []uint16 // Use this if it's not nil
	Number float64  // Use this if "String" is nil
}

type ConstValueKind uint8

const (
	ConstValueNone ConstValueKind = iota
	ConstValueNull
	ConstValueUndefined
	ConstValueTrue
	ConstValueFalse
	ConstValueNumber
)

type ConstValue struct {
	Number float64 // Use this for "ConstValueNumber"
	Kind   ConstValueKind
}

func ExprToConstValue(expr Expr) ConstValue {
	switch v := expr.Data.(type) {
	case *ENull:
		return ConstValue{Kind: ConstValueNull}

	case *EUndefined:
		return ConstValue{Kind: ConstValueUndefined}

	case *EBoolean:
		if v.Value {
			return ConstValue{Kind: ConstValueTrue}
		} else {
			return ConstValue{Kind: ConstValueFalse}
		}

	case *ENumber:
		// Inline integers and other small numbers. Don't inline large
		// real numbers because people may not want them to be inlined
		// as it will increase the minified code size by too much.
		if asInt := int64(v.Value); v.Value == float64(asInt) || len(strconv.FormatFloat(v.Value, 'g', -1, 64)) <= 8 {
			return ConstValue{Kind: ConstValueNumber, Number: v.Value}
		}

	case *EString:
		// I'm deliberately not inlining strings here. It seems more likely that
		// people won't want them to be inlined since they can be arbitrarily long.

	case *EBigInt:
		// I'm deliberately not inlining bigints here for the same reason (they can
		// be arbitrarily long).
	}

	return ConstValue{}
}

func ConstValueToExpr(loc logger.Loc, value ConstValue) Expr {
	switch value.Kind {
	case ConstValueNull:
		return Expr{Loc: loc, Data: ENullShared}

	case ConstValueUndefined:
		return Expr{Loc: loc, Data: EUndefinedShared}

	case ConstValueTrue:
		return Expr{Loc: loc, Data: &EBoolean{Value: true}}

	case ConstValueFalse:
		return Expr{Loc: loc, Data: &EBoolean{Value: false}}

	case ConstValueNumber:
		return Expr{Loc: loc, Data: &ENumber{Value: value.Number}}
	}

	panic("Internal error: invalid constant value")
}

type NamedImport struct {
	Alias string

	// Parts within this file that use this import
	LocalPartsWithUses []uint32

	AliasLoc          logger.Loc
	NamespaceRef      ast.Ref
	ImportRecordIndex uint32

	// If true, the alias refers to the entire export namespace object of a
	// module. This is no longer represented as an alias called "*" because of
	// the upcoming "Arbitrary module namespace identifier names" feature:
	// https://github.com/tc39/ecma262/pull/2154
	AliasIsStar bool

	// It's useful to flag exported imports because if they are in a TypeScript
	// file, we can't tell if they are a type or a value.
	IsExported bool
}

type NamedExport struct {
	Ref      ast.Ref
	AliasLoc logger.Loc
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
	SymbolUses map[ast.Ref]SymbolUse

	// An estimate of the number of uses of all symbols used as the target of
	// function calls within this part.
	SymbolCallUses map[ast.Ref]SymbolCallUse

	// This tracks property accesses off of imported symbols. We don't know
	// during parsing if an imported symbol is going to be an inlined enum
	// value or not. This is only known during linking. So we defer adding
	// a dependency on these imported symbols until we know whether the
	// property access is an inlined enum value or not.
	ImportSymbolPropertyUses map[ast.Ref]map[string]SymbolUse

	// The indices of the other parts in this file that are needed if this part
	// is needed.
	Dependencies []Dependency

	// If true, this part can be removed if none of the declared symbols are
	// used. If the file containing this part is imported, then all parts that
	// don't have this flag enabled must be included.
	CanBeRemovedIfUnused bool

	// This is used for generated parts that we don't want to be present if they
	// aren't needed. This enables tree shaking for these parts even if global
	// tree shaking isn't enabled.
	ForceTreeShaking bool

	// This is true if this file has been marked as live by the tree shaking
	// algorithm.
	IsLive bool
}

type Dependency struct {
	SourceIndex uint32
	PartIndex   uint32
}

type DeclaredSymbol struct {
	Ref        ast.Ref
	IsTopLevel bool
}

type SymbolUse struct {
	CountEstimate uint32
}

type SymbolCallUse struct {
	CallCountEstimate                   uint32
	SingleArgNonSpreadCallCountEstimate uint32
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
	dir, base, _ := logger.PlatformIndependentPathDirBaseExt(path)

	// If the name is "index", use the directory name instead. This is because
	// many packages in npm use the file name "index.js" because it triggers
	// node's implicit module resolution rules that allows you to import it by
	// just naming the directory.
	if base == "index" {
		_, dirBase, _ := logger.PlatformIndependentPathDirBaseExt(dir)
		if dirBase != "" {
			base = dirBase
		}
	}

	return EnsureValidIdentifier(base)
}

func EnsureValidIdentifier(base string) string {
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
