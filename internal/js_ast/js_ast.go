package js_ast

import (
	"math"
	"sort"

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

type PropertyKind int

const (
	PropertyNormal PropertyKind = iota
	PropertyGet
	PropertySet
	PropertySpread
	PropertyDeclare
	PropertyClassStaticBlock
)

type ClassStaticBlock struct {
	Loc   logger.Loc
	Stmts []Stmt
}

type Property struct {
	TSDecorators     []Expr
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

	Kind            PropertyKind
	IsComputed      bool
	IsMethod        bool
	IsStatic        bool
	WasShorthand    bool
	PreferQuotedKey bool
}

type PropertyBinding struct {
	Key               Expr
	Value             Binding
	DefaultValueOrNil Expr
	IsComputed        bool
	IsSpread          bool
	PreferQuotedKey   bool
}

type Arg struct {
	TSDecorators []Expr
	Binding      Binding
	DefaultOrNil Expr

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

	// This is true if the function is a method
	IsUniqueFormalParameters bool
}

type FnBody struct {
	Loc   logger.Loc
	Stmts []Stmt
}

type Class struct {
	ClassKeyword logger.Range
	TSDecorators []Expr
	Name         *LocRef
	ExtendsOrNil Expr
	BodyLoc      logger.Loc
	Properties   []Property
}

type ArrayBinding struct {
	Binding           Binding
	DefaultValueOrNil Expr
}

type Binding struct {
	Loc  logger.Loc
	Data B
}

// This interface is never called. Its purpose is to encode a variant type in
// Go's type system.
type B interface{ isBinding() }

func (*BMissing) isBinding()    {}
func (*BIdentifier) isBinding() {}
func (*BArray) isBinding()      {}
func (*BObject) isBinding()     {}

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

type Expr struct {
	Loc  logger.Loc
	Data E
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
func (*EJSXElement) isExpr()           {}
func (*EMissing) isExpr()              {}
func (*ENumber) isExpr()               {}
func (*EBigInt) isExpr()               {}
func (*EObject) isExpr()               {}
func (*ESpread) isExpr()               {}
func (*EString) isExpr()               {}
func (*ETemplate) isExpr()             {}
func (*ERegExp) isExpr()               {}
func (*EInlinedEnum) isExpr()          {}
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
	IsSingleLine     bool
	IsParenthesized  bool
}

type EUnary struct {
	Op    OpCode
	Value Expr
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
var ESuperShared = &ESuper{}
var ENullShared = &ENull{}
var EUndefinedShared = &EUndefined{}
var EThisShared = &EThis{}

type ENew struct {
	Target Expr
	Args   []Expr

	// True if there is a comment containing "@__PURE__" or "#__PURE__" preceding
	// this call expression. See the comment inside ECall for more details.
	CanBeUnwrappedIfUnused bool
}

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

	IsAsync    bool
	HasRestArg bool
	PreferExpr bool // Use shorthand if true and "Body" is a single return statement
}

type EFunction struct{ Fn Fn }

type EClass struct{ Class Class }

type EIdentifier struct {
	Ref Ref

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
	Ref             Ref
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
	Ref Ref
}

type EJSXElement struct {
	TagOrNil   Expr
	Properties []Property
	Children   []Expr
	CloseLoc   logger.Loc
}

type ENumber struct{ Value float64 }

type EBigInt struct{ Value string }

type EObject struct {
	Properties       []Property
	CommaAfterSpread logger.Loc
	IsSingleLine     bool
	IsParenthesized  bool
}

type ESpread struct{ Value Expr }

// This is used for both strings and no-substitution template literals to reduce
// the number of cases that need to be checked for string optimization code
type EString struct {
	Value          []uint16
	LegacyOctalLoc logger.Loc
	PreferTemplate bool
}

type TemplatePart struct {
	Value      Expr
	TailLoc    logger.Loc
	TailCooked []uint16 // Only use when "TagOrNil" is nil
	TailRaw    string   // Only use when "TagOrNil" is not nil
}

type ETemplate struct {
	TagOrNil       Expr
	HeadLoc        logger.Loc
	HeadCooked     []uint16 // Only use when "TagOrNil" is nil
	HeadRaw        string   // Only use when "TagOrNil" is not nil
	Parts          []TemplatePart
	LegacyOctalLoc logger.Loc
}

type ERegExp struct{ Value string }

type EInlinedEnum struct {
	Value   Expr
	Comment string
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
}

type ERequireResolveString struct {
	ImportRecordIndex uint32
}

type EImportString struct {
	ImportRecordIndex uint32

	// Comments inside "import()" expressions have special meaning for Webpack.
	// Preserving comments inside these expressions makes it possible to use
	// esbuild as a TypeScript-to-JavaScript frontend for Webpack to improve
	// performance. We intentionally do not interpret these comments in esbuild
	// because esbuild is not Webpack. But we do preserve them since doing so is
	// harmless, easy to maintain, and useful to people. See the Webpack docs for
	// more info: https://webpack.js.org/api/module-methods/#magic-comments.
	LeadingInteriorComments []Comment
}

type EImportCall struct {
	Expr         Expr
	OptionsOrNil Expr

	// See the comment for this same field on "EImportCall" for more information
	LeadingInteriorComments []Comment
}

func IsOptionalChain(value Expr) bool {
	switch e := value.Data.(type) {
	case *EDot:
		return e.OptionalChain != OptionalChainNone
	case *EIndex:
		return e.OptionalChain != OptionalChainNone
	case *ECall:
		return e.OptionalChain != OptionalChainNone
	}
	return false
}

func Assign(a Expr, b Expr) Expr {
	return Expr{Loc: a.Loc, Data: &EBinary{Op: BinOpAssign, Left: a, Right: b}}
}

func AssignStmt(a Expr, b Expr) Stmt {
	return Stmt{Loc: a.Loc, Data: &SExpr{Value: Assign(a, b)}}
}

// Wraps the provided expression in the "!" prefix operator. The expression
// will potentially be simplified to avoid generating unnecessary extra "!"
// operators. For example, calling this with "!!x" will return "!x" instead
// of returning "!!!x".
func Not(expr Expr) Expr {
	if result, ok := MaybeSimplifyNot(expr); ok {
		return result
	}
	return Expr{Loc: expr.Loc, Data: &EUnary{Op: UnOpNot, Value: expr}}
}

// The given "expr" argument should be the operand of a "!" prefix operator
// (i.e. the "x" in "!x"). This returns a simplified expression for the
// whole operator (i.e. the "!x") if it can be simplified, or false if not.
// It's separate from "Not()" above to avoid allocation on failure in case
// that is undesired.
func MaybeSimplifyNot(expr Expr) (Expr, bool) {
	switch e := expr.Data.(type) {
	case *EInlinedEnum:
		if value, ok := MaybeSimplifyNot(e.Value); ok {
			return value, true
		}

	case *ENull, *EUndefined:
		return Expr{Loc: expr.Loc, Data: &EBoolean{Value: true}}, true

	case *EBoolean:
		return Expr{Loc: expr.Loc, Data: &EBoolean{Value: !e.Value}}, true

	case *ENumber:
		return Expr{Loc: expr.Loc, Data: &EBoolean{Value: e.Value == 0 || math.IsNaN(e.Value)}}, true

	case *EBigInt:
		return Expr{Loc: expr.Loc, Data: &EBoolean{Value: e.Value == "0"}}, true

	case *EString:
		return Expr{Loc: expr.Loc, Data: &EBoolean{Value: len(e.Value) == 0}}, true

	case *EFunction, *EArrow, *ERegExp:
		return Expr{Loc: expr.Loc, Data: &EBoolean{Value: false}}, true

	case *EUnary:
		// "!!!a" => "!a"
		if e.Op == UnOpNot && KnownPrimitiveType(e.Value) == PrimitiveBoolean {
			return e.Value, true
		}

	case *EBinary:
		// Make sure that these transformations are all safe for special values.
		// For example, "!(a < b)" is not the same as "a >= b" if a and/or b are
		// NaN (or undefined, or null, or possibly other problem cases too).
		switch e.Op {
		case BinOpLooseEq:
			// "!(a == b)" => "a != b"
			e.Op = BinOpLooseNe
			return expr, true

		case BinOpLooseNe:
			// "!(a != b)" => "a == b"
			e.Op = BinOpLooseEq
			return expr, true

		case BinOpStrictEq:
			// "!(a === b)" => "a !== b"
			e.Op = BinOpStrictNe
			return expr, true

		case BinOpStrictNe:
			// "!(a !== b)" => "a === b"
			e.Op = BinOpStrictEq
			return expr, true

		case BinOpComma:
			// "!(a, b)" => "a, !b"
			e.Right = Not(e.Right)
			return expr, true
		}
	}

	return Expr{}, false
}

type PrimitiveType uint8

const (
	PrimitiveUnknown PrimitiveType = iota
	PrimitiveMixed
	PrimitiveNull
	PrimitiveUndefined
	PrimitiveBoolean
	PrimitiveNumber
	PrimitiveString
	PrimitiveBigInt
)

// This can be used when the returned type is either one or the other
func MergedKnownPrimitiveTypes(a Expr, b Expr) PrimitiveType {
	x := KnownPrimitiveType(a)
	y := KnownPrimitiveType(b)
	if x == PrimitiveUnknown || y == PrimitiveUnknown {
		return PrimitiveUnknown
	}
	if x == y {
		return x
	}
	return PrimitiveMixed // Definitely some kind of primitive
}

func KnownPrimitiveType(a Expr) PrimitiveType {
	switch e := a.Data.(type) {
	case *EInlinedEnum:
		return KnownPrimitiveType(e.Value)

	case *ENull:
		return PrimitiveNull

	case *EUndefined:
		return PrimitiveUndefined

	case *EBoolean:
		return PrimitiveBoolean

	case *ENumber:
		return PrimitiveNumber

	case *EString:
		return PrimitiveString

	case *EBigInt:
		return PrimitiveBigInt

	case *ETemplate:
		if e.TagOrNil.Data == nil {
			return PrimitiveString
		}

	case *EIf:
		return MergedKnownPrimitiveTypes(e.Yes, e.No)

	case *EUnary:
		switch e.Op {
		case UnOpVoid:
			return PrimitiveUndefined

		case UnOpTypeof:
			return PrimitiveString

		case UnOpNot, UnOpDelete:
			return PrimitiveBoolean

		case UnOpPos:
			return PrimitiveNumber // Cannot be bigint because that throws an exception

		case UnOpNeg, UnOpCpl:
			value := KnownPrimitiveType(e.Value)
			if value == PrimitiveBigInt {
				return PrimitiveBigInt
			}
			if value != PrimitiveUnknown && value != PrimitiveMixed {
				return PrimitiveNumber
			}
			return PrimitiveMixed // Can be number or bigint

		case UnOpPreDec, UnOpPreInc, UnOpPostDec, UnOpPostInc:
			return PrimitiveMixed // Can be number or bigint
		}

	case *EBinary:
		switch e.Op {
		case BinOpStrictEq, BinOpStrictNe, BinOpLooseEq, BinOpLooseNe,
			BinOpLt, BinOpGt, BinOpLe, BinOpGe,
			BinOpInstanceof, BinOpIn:
			return PrimitiveBoolean

		case BinOpLogicalOr, BinOpLogicalAnd:
			return MergedKnownPrimitiveTypes(e.Left, e.Right)

		case BinOpNullishCoalescing:
			left := KnownPrimitiveType(e.Left)
			right := KnownPrimitiveType(e.Right)
			if left == PrimitiveNull || left == PrimitiveUndefined {
				return right
			}
			if left != PrimitiveUnknown {
				if left != PrimitiveMixed {
					return left // Definitely not null or undefined
				}
				if right != PrimitiveUnknown {
					return PrimitiveMixed // Definitely some kind of primitive
				}
			}

		case BinOpAdd:
			left := KnownPrimitiveType(e.Left)
			right := KnownPrimitiveType(e.Right)
			if left == PrimitiveString || right == PrimitiveString {
				return PrimitiveString
			}
			if left == PrimitiveBigInt && right == PrimitiveBigInt {
				return PrimitiveBigInt
			}
			if left != PrimitiveUnknown && left != PrimitiveMixed && left != PrimitiveBigInt &&
				right != PrimitiveUnknown && right != PrimitiveMixed && right != PrimitiveBigInt {
				return PrimitiveNumber
			}
			return PrimitiveMixed // Can be number or bigint or string (or an exception)

		case BinOpAddAssign:
			right := KnownPrimitiveType(e.Right)
			if right == PrimitiveString {
				return PrimitiveString
			}
			return PrimitiveMixed // Can be number or bigint or string (or an exception)

		case
			BinOpSub, BinOpSubAssign,
			BinOpMul, BinOpMulAssign,
			BinOpDiv, BinOpDivAssign,
			BinOpRem, BinOpRemAssign,
			BinOpPow, BinOpPowAssign,
			BinOpBitwiseAnd, BinOpBitwiseAndAssign,
			BinOpBitwiseOr, BinOpBitwiseOrAssign,
			BinOpBitwiseXor, BinOpBitwiseXorAssign,
			BinOpShl, BinOpShlAssign,
			BinOpShr, BinOpShrAssign,
			BinOpUShr, BinOpUShrAssign:
			return PrimitiveMixed // Can be number or bigint (or an exception)

		case BinOpAssign, BinOpComma:
			return KnownPrimitiveType(e.Right)
		}
	}

	return PrimitiveUnknown
}

// The goal of this function is to "rotate" the AST if it's possible to use the
// left-associative property of the operator to avoid unnecessary parentheses.
//
// When using this, make absolutely sure that the operator is actually
// associative. For example, the "-" operator is not associative for
// floating-point numbers.
func JoinWithLeftAssociativeOp(op OpCode, a Expr, b Expr) Expr {
	// "(a, b) op c" => "a, b op c"
	if comma, ok := a.Data.(*EBinary); ok && comma.Op == BinOpComma {
		comma.Right = JoinWithLeftAssociativeOp(op, comma.Right, b)
		return a
	}

	// "a op (b op c)" => "(a op b) op c"
	// "a op (b op (c op d))" => "((a op b) op c) op d"
	if binary, ok := b.Data.(*EBinary); ok && binary.Op == op {
		return JoinWithLeftAssociativeOp(
			op,
			JoinWithLeftAssociativeOp(op, a, binary.Left),
			binary.Right,
		)
	}

	// "a op b" => "a op b"
	// "(a op b) op c" => "(a op b) op c"
	return Expr{Loc: a.Loc, Data: &EBinary{Op: op, Left: a, Right: b}}
}

func JoinWithComma(a Expr, b Expr) Expr {
	if a.Data == nil {
		return b
	}
	if b.Data == nil {
		return a
	}
	return Expr{Loc: a.Loc, Data: &EBinary{Op: BinOpComma, Left: a, Right: b}}
}

func JoinAllWithComma(all []Expr) (result Expr) {
	for _, value := range all {
		result = JoinWithComma(result, value)
	}
	return
}

type Stmt struct {
	Loc  logger.Loc
	Data S
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
	Stmts []Stmt
}

type SEmpty struct{}

// This is a stand-in for a TypeScript type declaration
type STypeScript struct{}

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
	NamespaceRef      Ref
	ImportRecordIndex uint32
	IsSingleLine      bool
}

type SExportDefault struct {
	DefaultName LocRef
	Value       Stmt // May be a SExpr or SFunction or SClass
}

type ExportStarAlias struct {
	Loc logger.Loc

	// Although this alias name starts off as being the same as the statement's
	// namespace symbol, it may diverge if the namespace symbol name is minified.
	// The original alias name is preserved here to avoid this scenario.
	OriginalName string
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

	// This is set to true for automatically-generated expressions that should
	// not affect tree shaking. For example, calling a function from the runtime
	// that doesn't have externally-visible side effects.
	DoesNotAffectTreeShaking bool
}

type EnumValue struct {
	Name       []uint16
	ValueOrNil Expr
	Ref        Ref
	Loc        logger.Loc
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
	Test    Expr
	Yes     Stmt
	NoOrNil Stmt
}

type SFor struct {
	InitOrNil   Stmt // May be a SConst, SLet, SVar, or SExpr
	TestOrNil   Expr
	UpdateOrNil Expr
	Body        Stmt
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
	BindingOrNil Binding
	Body         []Stmt
	Loc          logger.Loc
	BodyLoc      logger.Loc
}

type Finally struct {
	Loc   logger.Loc
	Stmts []Stmt
}

type STry struct {
	BodyLoc logger.Loc
	Body    []Stmt
	Catch   *Catch
	Finally *Finally
}

type Case struct {
	ValueOrNil Expr // If this is nil, this is "default" instead of "case"
	Body       []Stmt
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
)

type SLocal struct {
	Decls    []Decl
	Kind     LocalKind
	IsExport bool

	// The TypeScript compiler doesn't generate code for "import foo = bar"
	// statements where the import is never used.
	WasTSImportEquals bool
}

type SBreak struct {
	Label *LocRef
}

type SContinue struct {
	Label *LocRef
}

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

	// This is the original name of the symbol stored in "Name". It's needed for
	// "SExportClause" statements such as this:
	//
	//   export {foo as bar} from 'path'
	//
	// In this case both "foo" and "bar" are aliases because it's a re-export.
	// We need to preserve both aliases in case the symbol is renamed. In this
	// example, "foo" is "OriginalName" and "bar" is "Alias".
	OriginalName string
}

type Decl struct {
	Binding    Binding
	ValueOrNil Expr
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

	// Injected symbols can be overridden by provided defines
	SymbolInjected

	// This annotates all other symbols that don't have special behavior.
	SymbolOther
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
	ChunkIndex ast.Index32

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
	NestedScopeSlot ast.Index32

	Kind SymbolKind

	// Certain symbols must not be renamed or minified. For example, the
	// "arguments" variable is declared by the runtime for every function.
	// Renaming can also break any identifier used inside a "with" statement.
	MustNotBeRenamed bool

	// In React's version of JSX, lower-case names are strings while upper-case
	// names are identifiers. If we are preserving JSX syntax (i.e. not
	// transforming it), then we need to be careful to name the identifiers
	// something with a capital letter so further JSX processing doesn't treat
	// them as strings instead.
	MustStartWithCapitalLetterForJSX bool

	// If true, this symbol is the target of a "__name" helper function call.
	// This call is special because it deliberately doesn't count as a use
	// of the symbol (otherwise keeping names would disable tree shaking)
	// so "UseCountEstimate" is not incremented. This flag helps us know to
	// avoid optimizing this symbol when "UseCountEstimate" is 1 in this case.
	DidKeepName bool

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
	PrivateSymbolMustBeLowered bool
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
	Ref Ref
	Loc logger.Loc
}

type Scope struct {
	Kind      ScopeKind
	Parent    *Scope
	Children  []*Scope
	Members   map[string]ScopeMember
	Generated []Ref

	// This will be non-nil if this is a TypeScript "namespace" or "enum"
	TSNamespace *TSNamespaceScope

	// The location of the "use strict" directive for ExplicitStrictMode
	UseStrictLoc logger.Loc

	// This is used to store the ref of the label symbol for ScopeLabel scopes.
	Label           LocRef
	LabelStmtIsLoop bool

	// If a scope contains a direct eval() expression, then none of the symbols
	// inside that scope can be renamed. We conservatively assume that the
	// evaluated code might reference anything that it has access to.
	ContainsDirectEval bool

	// This is to help forbid "arguments" inside class body scopes
	ForbidArguments bool

	StrictMode StrictModeKind
}

type StrictModeKind uint8

const (
	SloppyMode StrictModeKind = iota
	ExplicitStrictMode
	ImplicitStrictModeImport
	ImplicitStrictModeExport
	ImplicitStrictModeTopLevelAwait
	ImplicitStrictModeClass
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
//   let x = 1;
//   namespace Foo {
//     let x = 2;
//     export let y = 3;
//   }
//   namespace Foo {
//     console.log(x); // 1
//     console.log(y); // 3
//   }
//
// Doing this also works inside an enum:
//
//   enum Foo {
//     A = 3,
//     B = A + 1,
//   }
//   enum Foo {
//     C = A + 2,
//   }
//   console.log(Foo.B) // 4
//   console.log(Foo.C) // 5
//
// This is a form of identifier lookup that works differently than the
// hierarchical scope-based identifier lookup in JavaScript. Lookup now needs
// to search sibling scopes in addition to parent scopes. This is accomplished
// by sharing the map of exported members between all matching sibling scopes.
type TSNamespaceScope struct {
	// This is shared between all sibling namespace blocks
	ExportedMembers TSNamespaceMembers

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
	ArgRef Ref

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
	LazilyGeneratedProperyAccesses map[string]Ref

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
	ModuleCommonJS

	// ".mjs" or ".mts" or "type: module" in package.json
	ModuleESM
)

// This is the index to the automatically-generated part containing code that
// calls "__export(exports, { ... getters ... })". This is used to generate
// getters on an exports object for ES6 export statements, and is both for
// ES6 star imports and CommonJS-style modules. All files have one of these,
// although it may contain no statements if there is nothing to export.
const NSExportPartIndex = uint32(0)

type AST struct {
	ApproximateLineCount  int32
	NestedScopeSlotCounts SlotCounts
	HasLazyExport         bool
	ModuleType            ModuleType

	// This is a list of CommonJS features. When a file uses CommonJS features,
	// it's not a candidate for "flat bundling" and must be wrapped in its own
	// closure. Note that this also includes top-level "return" but these aren't
	// here because only the parser checks those.
	UsesExportsRef bool
	UsesModuleRef  bool
	ExportsKind    ExportsKind

	// This is a list of ES6 features. They are ranges instead of booleans so
	// that they can be used in log messages. Check to see if "Len > 0".
	ImportKeyword        logger.Range // Does not include TypeScript-specific syntax or "import()"
	ExportKeyword        logger.Range // Does not include TypeScript-specific syntax
	TopLevelAwaitKeyword logger.Range

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
	NamedExports            map[string]NamedExport
	ExportStarImportRecords []uint32

	// Note: If you're in the linker, do not use this map directly. This map is
	// filled in by the parser and is considered immutable. For performance reasons,
	// the linker doesn't mutate this map (cloning a map is slow in Go). Instead the
	// linker super-imposes relevant information on top in a method call. You should
	// call "TopLevelSymbolToParts" instead.
	TopLevelSymbolToPartsFromParser map[Ref][]uint32

	SourceMapComment logger.Span
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

type NamedImport struct {
	// Parts within this file that use this import
	LocalPartsWithUses []uint32

	Alias             string
	AliasLoc          logger.Loc
	NamespaceRef      Ref
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
	Ref      Ref
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
	SymbolUses map[Ref]SymbolUse

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
	Ref        Ref
	IsTopLevel bool
}

type SymbolUse struct {
	CountEstimate uint32
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
	newSymbol.UseCountEstimate += oldSymbol.UseCountEstimate
	if oldSymbol.MustNotBeRenamed {
		newSymbol.OriginalName = oldSymbol.OriginalName
		newSymbol.MustNotBeRenamed = true
	}
	if oldSymbol.MustStartWithCapitalLetterForJSX {
		newSymbol.MustStartWithCapitalLetterForJSX = true
	}
	return new
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

func ConvertBindingToExpr(binding Binding, wrapIdentifier func(logger.Loc, Ref) Expr) Expr {
	loc := binding.Loc

	switch b := binding.Data.(type) {
	case *BMissing:
		return Expr{Loc: loc, Data: &EMissing{}}

	case *BIdentifier:
		if wrapIdentifier != nil {
			return wrapIdentifier(loc, b.Ref)
		}
		return Expr{Loc: loc, Data: &EIdentifier{Ref: b.Ref}}

	case *BArray:
		exprs := make([]Expr, len(b.Items))
		for i, item := range b.Items {
			expr := ConvertBindingToExpr(item.Binding, wrapIdentifier)
			if b.HasSpread && i+1 == len(b.Items) {
				expr = Expr{Loc: expr.Loc, Data: &ESpread{Value: expr}}
			} else if item.DefaultValueOrNil.Data != nil {
				expr = Assign(expr, item.DefaultValueOrNil)
			}
			exprs[i] = expr
		}
		return Expr{Loc: loc, Data: &EArray{
			Items:        exprs,
			IsSingleLine: b.IsSingleLine,
		}}

	case *BObject:
		properties := make([]Property, len(b.Properties))
		for i, property := range b.Properties {
			value := ConvertBindingToExpr(property.Value, wrapIdentifier)
			kind := PropertyNormal
			if property.IsSpread {
				kind = PropertySpread
			}
			properties[i] = Property{
				Kind:             kind,
				IsComputed:       property.IsComputed,
				Key:              property.Key,
				ValueOrNil:       value,
				InitializerOrNil: property.DefaultValueOrNil,
			}
		}
		return Expr{Loc: loc, Data: &EObject{
			Properties:   properties,
			IsSingleLine: b.IsSingleLine,
		}}

	default:
		panic("Internal error")
	}
}
