package js_ast

import (
	"math"
	"strconv"

	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/logger"
)

// If this returns true, then calling this expression captures the target of
// the property access as "this" when calling the function in the property.
func IsPropertyAccess(expr Expr) bool {
	switch expr.Data.(type) {
	case *EDot, *EIndex:
		return true
	}
	return false
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
//
// This function intentionally avoids mutating the input AST so it can be
// called after the AST has been frozen (i.e. after parsing ends).
func MaybeSimplifyNot(expr Expr) (Expr, bool) {
	switch e := expr.Data.(type) {
	case *EAnnotation:
		return MaybeSimplifyNot(e.Value)

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
		if e.Op == UnOpNot && KnownPrimitiveType(e.Value.Data) == PrimitiveBoolean {
			return e.Value, true
		}

	case *EBinary:
		// Make sure that these transformations are all safe for special values.
		// For example, "!(a < b)" is not the same as "a >= b" if a and/or b are
		// NaN (or undefined, or null, or possibly other problem cases too).
		switch e.Op {
		case BinOpLooseEq:
			// "!(a == b)" => "a != b"
			return Expr{Loc: expr.Loc, Data: &EBinary{Op: BinOpLooseNe, Left: e.Left, Right: e.Right}}, true

		case BinOpLooseNe:
			// "!(a != b)" => "a == b"
			return Expr{Loc: expr.Loc, Data: &EBinary{Op: BinOpLooseEq, Left: e.Left, Right: e.Right}}, true

		case BinOpStrictEq:
			// "!(a === b)" => "a !== b"
			return Expr{Loc: expr.Loc, Data: &EBinary{Op: BinOpStrictNe, Left: e.Left, Right: e.Right}}, true

		case BinOpStrictNe:
			// "!(a !== b)" => "a === b"
			return Expr{Loc: expr.Loc, Data: &EBinary{Op: BinOpStrictEq, Left: e.Left, Right: e.Right}}, true

		case BinOpComma:
			// "!(a, b)" => "a, !b"
			return Expr{Loc: expr.Loc, Data: &EBinary{Op: BinOpComma, Left: e.Left, Right: Not(e.Right)}}, true
		}
	}

	return Expr{}, false
}

// This function intentionally avoids mutating the input AST so it can be
// called after the AST has been frozen (i.e. after parsing ends).
func MaybeSimplifyEqualityComparison(loc logger.Loc, e *EBinary, unsupportedFeatures compat.JSFeature) (Expr, bool) {
	value, primitive := e.Left, e.Right

	// Detect when the primitive comes first and flip the order of our checks
	if IsPrimitiveLiteral(value.Data) {
		value, primitive = primitive, value
	}

	// "!x === true" => "!x"
	// "!x === false" => "!!x"
	// "!x !== true" => "!!x"
	// "!x !== false" => "!x"
	if boolean, ok := primitive.Data.(*EBoolean); ok && KnownPrimitiveType(value.Data) == PrimitiveBoolean {
		if boolean.Value == (e.Op == BinOpLooseNe || e.Op == BinOpStrictNe) {
			return Not(value), true
		} else {
			return value, true
		}
	}

	// "typeof x != 'undefined'" => "typeof x < 'u'"
	// "typeof x == 'undefined'" => "typeof x > 'u'"
	if !unsupportedFeatures.Has(compat.TypeofExoticObjectIsObject) {
		// Only do this optimization if we know that the "typeof" operator won't
		// return something random. The only case of this happening was Internet
		// Explorer returning "unknown" for some objects, which messes with this
		// optimization. So we don't do this when targeting Internet Explorer.
		if typeof, ok := value.Data.(*EUnary); ok && typeof.Op == UnOpTypeof {
			if str, ok := primitive.Data.(*EString); ok && helpers.UTF16EqualsString(str.Value, "undefined") {
				flip := value == e.Right
				op := BinOpLt
				if (e.Op == BinOpLooseEq || e.Op == BinOpStrictEq) != flip {
					op = BinOpGt
				}
				primitive.Data = &EString{Value: []uint16{'u'}}
				if flip {
					value, primitive = primitive, value
				}
				return Expr{Loc: loc, Data: &EBinary{Op: op, Left: value, Right: primitive}}, true
			}
		}
	}

	return Expr{}, false
}

func IsPrimitiveLiteral(data E) bool {
	switch e := data.(type) {
	case *EAnnotation:
		return IsPrimitiveLiteral(e.Value.Data)

	case *EInlinedEnum:
		return IsPrimitiveLiteral(e.Value.Data)

	case *ENull, *EUndefined, *EString, *EBoolean, *ENumber, *EBigInt:
		return true
	}
	return false
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
	x := KnownPrimitiveType(a.Data)
	if x == PrimitiveUnknown {
		return PrimitiveUnknown
	}

	y := KnownPrimitiveType(b.Data)
	if y == PrimitiveUnknown {
		return PrimitiveUnknown
	}

	if x == y {
		return x
	}
	return PrimitiveMixed // Definitely some kind of primitive
}

// Note: This function does not say whether the expression is side-effect free
// or not. For example, the expression "++x" always returns a primitive.
func KnownPrimitiveType(expr E) PrimitiveType {
	switch e := expr.(type) {
	case *EAnnotation:
		return KnownPrimitiveType(e.Value.Data)

	case *EInlinedEnum:
		return KnownPrimitiveType(e.Value.Data)

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
			value := KnownPrimitiveType(e.Value.Data)
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
			left := KnownPrimitiveType(e.Left.Data)
			right := KnownPrimitiveType(e.Right.Data)
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
			left := KnownPrimitiveType(e.Left.Data)
			right := KnownPrimitiveType(e.Right.Data)
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
			right := KnownPrimitiveType(e.Right.Data)
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
			return KnownPrimitiveType(e.Right.Data)
		}
	}

	return PrimitiveUnknown
}

func CanChangeStrictToLoose(a Expr, b Expr) bool {
	x := KnownPrimitiveType(a.Data)
	y := KnownPrimitiveType(b.Data)
	return x == y && x != PrimitiveUnknown && x != PrimitiveMixed
}

// Returns true if the result of the "typeof" operator on this expression is
// statically determined and this expression has no side effects (i.e. can be
// removed without consequence).
func TypeofWithoutSideEffects(data E) (string, bool) {
	switch e := data.(type) {
	case *EAnnotation:
		if e.Flags.Has(CanBeRemovedIfUnusedFlag) {
			return TypeofWithoutSideEffects(e.Value.Data)
		}

	case *EInlinedEnum:
		return TypeofWithoutSideEffects(e.Value.Data)

	case *ENull:
		return "object", true

	case *EUndefined:
		return "undefined", true

	case *EBoolean:
		return "boolean", true

	case *ENumber:
		return "number", true

	case *EBigInt:
		return "bigint", true

	case *EString:
		return "string", true

	case *EFunction, *EArrow:
		return "function", true
	}

	return "", false
}

// The goal of this function is to "rotate" the AST if it's possible to use the
// left-associative property of the operator to avoid unnecessary parentheses.
//
// When using this, make absolutely sure that the operator is actually
// associative. For example, the "+" operator is not associative for
// floating-point numbers.
//
// This function intentionally avoids mutating the input AST so it can be
// called after the AST has been frozen (i.e. after parsing ends).
func JoinWithLeftAssociativeOp(op OpCode, a Expr, b Expr) Expr {
	// "(a, b) op c" => "a, b op c"
	if comma, ok := a.Data.(*EBinary); ok && comma.Op == BinOpComma {
		// Don't mutate the original AST
		clone := *comma
		clone.Right = JoinWithLeftAssociativeOp(op, clone.Right, b)
		return Expr{Loc: a.Loc, Data: &clone}
	}

	// "a op (b op c)" => "(a op b) op c"
	// "a op (b op (c op d))" => "((a op b) op c) op d"
	for {
		if binary, ok := b.Data.(*EBinary); ok && binary.Op == op {
			a = JoinWithLeftAssociativeOp(op, a, binary.Left)
			b = binary.Right
		} else {
			break
		}
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
			var flags PropertyFlags
			if property.IsComputed {
				flags |= PropertyIsComputed
			}
			properties[i] = Property{
				Kind:             kind,
				Flags:            flags,
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

// This will return a nil expression if the expression can be totally removed.
//
// This function intentionally avoids mutating the input AST so it can be
// called after the AST has been frozen (i.e. after parsing ends).
func SimplifyUnusedExpr(expr Expr, unsupportedFeatures compat.JSFeature, isUnbound func(Ref) bool) Expr {
	switch e := expr.Data.(type) {
	case *EAnnotation:
		if e.Flags.Has(CanBeRemovedIfUnusedFlag) {
			return Expr{}
		}

	case *EInlinedEnum:
		return SimplifyUnusedExpr(e.Value, unsupportedFeatures, isUnbound)

	case *ENull, *EUndefined, *EMissing, *EBoolean, *ENumber, *EBigInt,
		*EString, *EThis, *ERegExp, *EFunction, *EArrow, *EImportMeta:
		return Expr{}

	case *EDot:
		if e.CanBeRemovedIfUnused {
			return Expr{}
		}

	case *EIdentifier:
		if e.MustKeepDueToWithStmt {
			break
		}
		if e.CanBeRemovedIfUnused || !isUnbound(e.Ref) {
			return Expr{}
		}

	case *ETemplate:
		if e.TagOrNil.Data == nil {
			var comma Expr
			var templateLoc logger.Loc
			var template *ETemplate
			for _, part := range e.Parts {
				// If we know this value is some kind of primitive, then we know that
				// "ToString" has no side effects and can be avoided.
				if KnownPrimitiveType(part.Value.Data) != PrimitiveUnknown {
					if template != nil {
						comma = JoinWithComma(comma, Expr{Loc: templateLoc, Data: template})
						template = nil
					}
					comma = JoinWithComma(comma, SimplifyUnusedExpr(part.Value, unsupportedFeatures, isUnbound))
					continue
				}

				// Make sure "ToString" is still evaluated on the value. We can't use
				// string addition here because that may evaluate "ValueOf" instead.
				if template == nil {
					template = &ETemplate{}
					templateLoc = part.Value.Loc
				}
				template.Parts = append(template.Parts, TemplatePart{Value: part.Value})
			}
			if template != nil {
				comma = JoinWithComma(comma, Expr{Loc: templateLoc, Data: template})
			}
			return comma
		}

	case *EArray:
		// Arrays with "..." spread expressions can't be unwrapped because the
		// "..." triggers code evaluation via iterators. In that case, just trim
		// the other items instead and leave the array expression there.
		for _, spread := range e.Items {
			if _, ok := spread.Data.(*ESpread); ok {
				items := make([]Expr, 0, len(e.Items))
				for _, item := range e.Items {
					item = SimplifyUnusedExpr(item, unsupportedFeatures, isUnbound)
					if item.Data != nil {
						items = append(items, item)
					}
				}

				// Don't mutate the original AST
				clone := *e
				clone.Items = items
				return Expr{Loc: expr.Loc, Data: &clone}
			}
		}

		// Otherwise, the array can be completely removed. We only need to keep any
		// array items with side effects. Apply this simplification recursively.
		var result Expr
		for _, item := range e.Items {
			result = JoinWithComma(result, SimplifyUnusedExpr(item, unsupportedFeatures, isUnbound))
		}
		return result

	case *EObject:
		// Objects with "..." spread expressions can't be unwrapped because the
		// "..." triggers code evaluation via getters. In that case, just trim
		// the other items instead and leave the object expression there.
		for _, spread := range e.Properties {
			if spread.Kind == PropertySpread {
				properties := make([]Property, 0, len(e.Properties))
				for _, property := range e.Properties {
					// Spread properties must always be evaluated
					if property.Kind != PropertySpread {
						value := SimplifyUnusedExpr(property.ValueOrNil, unsupportedFeatures, isUnbound)
						if value.Data != nil {
							// Keep the value
							property.ValueOrNil = value
						} else if !property.Flags.Has(PropertyIsComputed) {
							// Skip this property if the key doesn't need to be computed
							continue
						} else {
							// Replace values without side effects with "0" because it's short
							property.ValueOrNil.Data = &ENumber{}
						}
					}
					properties = append(properties, property)
				}

				// Don't mutate the original AST
				clone := *e
				clone.Properties = properties
				return Expr{Loc: expr.Loc, Data: &clone}
			}
		}

		// Otherwise, the object can be completely removed. We only need to keep any
		// object properties with side effects. Apply this simplification recursively.
		var result Expr
		for _, property := range e.Properties {
			if property.Flags.Has(PropertyIsComputed) {
				// Make sure "ToString" is still evaluated on the key
				result = JoinWithComma(result, Expr{Loc: property.Key.Loc, Data: &EBinary{
					Op:    BinOpAdd,
					Left:  property.Key,
					Right: Expr{Loc: property.Key.Loc, Data: &EString{}},
				}})
			}
			result = JoinWithComma(result, SimplifyUnusedExpr(property.ValueOrNil, unsupportedFeatures, isUnbound))
		}
		return result

	case *EIf:
		yes := SimplifyUnusedExpr(e.Yes, unsupportedFeatures, isUnbound)
		no := SimplifyUnusedExpr(e.No, unsupportedFeatures, isUnbound)

		// "foo() ? 1 : 2" => "foo()"
		if yes.Data == nil && no.Data == nil {
			return SimplifyUnusedExpr(e.Test, unsupportedFeatures, isUnbound)
		}

		// "foo() ? 1 : bar()" => "foo() || bar()"
		if yes.Data == nil {
			return JoinWithLeftAssociativeOp(BinOpLogicalOr, e.Test, no)
		}

		// "foo() ? bar() : 2" => "foo() && bar()"
		if no.Data == nil {
			return JoinWithLeftAssociativeOp(BinOpLogicalAnd, e.Test, yes)
		}

		if yes != e.Yes || no != e.No {
			return Expr{Loc: expr.Loc, Data: &EIf{Test: e.Test, Yes: yes, No: no}}
		}

	case *EUnary:
		switch e.Op {
		// These operators must not have any type conversions that can execute code
		// such as "toString" or "valueOf". They must also never throw any exceptions.
		case UnOpVoid, UnOpNot:
			return SimplifyUnusedExpr(e.Value, unsupportedFeatures, isUnbound)

		case UnOpTypeof:
			if _, ok := e.Value.Data.(*EIdentifier); ok && e.WasOriginallyTypeofIdentifier {
				// "typeof x" must not be transformed into if "x" since doing so could
				// cause an exception to be thrown. Instead we can just remove it since
				// "typeof x" is special-cased in the standard to never throw.
				return Expr{}
			}
			return SimplifyUnusedExpr(e.Value, unsupportedFeatures, isUnbound)
		}

	case *EBinary:
		left := e.Left
		right := e.Right

		switch e.Op {
		// These operators must not have any type conversions that can execute code
		// such as "toString" or "valueOf". They must also never throw any exceptions.
		case BinOpStrictEq, BinOpStrictNe, BinOpComma:
			return JoinWithComma(SimplifyUnusedExpr(left, unsupportedFeatures, isUnbound), SimplifyUnusedExpr(right, unsupportedFeatures, isUnbound))

		// We can simplify "==" and "!=" even though they can call "toString" and/or
		// "valueOf" if we can statically determine that the types of both sides are
		// primitives. In that case there won't be any chance for user-defined
		// "toString" and/or "valueOf" to be called.
		case BinOpLooseEq, BinOpLooseNe:
			if MergedKnownPrimitiveTypes(left, right) != PrimitiveUnknown {
				return JoinWithComma(SimplifyUnusedExpr(left, unsupportedFeatures, isUnbound), SimplifyUnusedExpr(right, unsupportedFeatures, isUnbound))
			}

		case BinOpLogicalAnd, BinOpLogicalOr, BinOpNullishCoalescing:
			// If this is a boolean logical operation and the result is unused, then
			// we know the left operand will only be used for its boolean value and
			// can be simplified under that assumption
			if e.Op != BinOpNullishCoalescing {
				left = SimplifyBooleanExpr(left)
			}

			// Preserve short-circuit behavior: the left expression is only unused if
			// the right expression can be completely removed. Otherwise, the left
			// expression is important for the branch.
			right = SimplifyUnusedExpr(right, unsupportedFeatures, isUnbound)
			if right.Data == nil {
				return SimplifyUnusedExpr(left, unsupportedFeatures, isUnbound)
			}

			// Try to take advantage of the optional chain operator to shorten code
			if !unsupportedFeatures.Has(compat.OptionalChain) {
				if binary, ok := left.Data.(*EBinary); ok {
					// "a != null && a.b()" => "a?.b()"
					// "a == null || a.b()" => "a?.b()"
					if (binary.Op == BinOpLooseNe && e.Op == BinOpLogicalAnd) || (binary.Op == BinOpLooseEq && e.Op == BinOpLogicalOr) {
						var test Expr
						if _, ok := binary.Right.Data.(*ENull); ok {
							test = binary.Left
						} else if _, ok := binary.Left.Data.(*ENull); ok {
							test = binary.Right
						}

						// Note: Technically unbound identifiers can refer to a getter on
						// the global object and that getter can have side effects that can
						// be observed if we run that getter once instead of twice. But this
						// seems like terrible coding practice and very unlikely to come up
						// in real software, so we deliberately ignore this possibility and
						// optimize for size instead of for this obscure edge case.
						//
						// If this is ever changed, then we must also pessimize the lowering
						// of "foo?.bar" to save the value of "foo" to ensure that it's only
						// evaluated once. Specifically "foo?.bar" would have to expand to:
						//
						//   var _a;
						//   (_a = foo) == null ? void 0 : _a.bar;
						//
						// instead of:
						//
						//   foo == null ? void 0 : foo.bar;
						//
						// Babel does the first one while TypeScript does the second one.
						// Since TypeScript doesn't handle this extreme edge case and
						// TypeScript is very widely used, I think it's fine for us to not
						// handle this edge case either.
						if id, ok := test.Data.(*EIdentifier); ok && !id.MustKeepDueToWithStmt && TryToInsertOptionalChain(test, right) {
							return right
						}
					}
				}
			}

		case BinOpAdd:
			if result, isStringAddition := simplifyUnusedStringAdditionChain(expr); isStringAddition {
				return result
			}
		}

		if left != e.Left || right != e.Right {
			return Expr{Loc: expr.Loc, Data: &EBinary{Op: e.Op, Left: left, Right: right}}
		}

	case *ECall:
		// A call that has been marked "__PURE__" can be removed if all arguments
		// can be removed. The annotation causes us to ignore the target.
		if e.CanBeUnwrappedIfUnused {
			var result Expr
			for _, arg := range e.Args {
				if _, ok := arg.Data.(*ESpread); ok {
					arg.Data = &EArray{Items: []Expr{arg}, IsSingleLine: true}
				}
				result = JoinWithComma(result, SimplifyUnusedExpr(arg, unsupportedFeatures, isUnbound))
			}
			return result
		}

		// Attempt to shorten IIFEs
		if len(e.Args) == 0 {
			switch target := e.Target.Data.(type) {
			case *EFunction:
				if len(target.Fn.Args) != 0 {
					break
				}

				// Just delete "(function() {})()" completely
				if len(target.Fn.Body.Block.Stmts) == 0 {
					return Expr{}
				}

			case *EArrow:
				if len(target.Args) != 0 {
					break
				}

				// Just delete "(() => {})()" completely
				if len(target.Body.Block.Stmts) == 0 {
					return Expr{}
				}

				if len(target.Body.Block.Stmts) == 1 {
					switch s := target.Body.Block.Stmts[0].Data.(type) {
					case *SExpr:
						if !target.IsAsync {
							// Replace "(() => { foo() })()" with "foo()"
							return s.Value
						} else {
							// Replace "(async () => { foo() })()" with "(async () => foo())()"
							clone := *target
							clone.Body.Block.Stmts[0].Data = &SReturn{ValueOrNil: s.Value}
							clone.PreferExpr = true
							return Expr{Loc: expr.Loc, Data: &ECall{Target: Expr{Loc: e.Target.Loc, Data: &clone}}}
						}

					case *SReturn:
						if !target.IsAsync {
							// Replace "(() => foo())()" with "foo()"
							return s.ValueOrNil
						}
					}
				}
			}
		}

	case *ENew:
		// A constructor call that has been marked "__PURE__" can be removed if all
		// arguments can be removed. The annotation causes us to ignore the target.
		if e.CanBeUnwrappedIfUnused {
			var result Expr
			for _, arg := range e.Args {
				if _, ok := arg.Data.(*ESpread); ok {
					arg.Data = &EArray{Items: []Expr{arg}, IsSingleLine: true}
				}
				result = JoinWithComma(result, SimplifyUnusedExpr(arg, unsupportedFeatures, isUnbound))
			}
			return result
		}
	}

	return expr
}

// This function intentionally avoids mutating the input AST so it can be
// called after the AST has been frozen (i.e. after parsing ends).
func simplifyUnusedStringAdditionChain(expr Expr) (Expr, bool) {
	switch e := expr.Data.(type) {
	case *EString:
		// "'x' + y" => "'' + y"
		return Expr{Loc: expr.Loc, Data: &EString{}}, true

	case *EBinary:
		if e.Op == BinOpAdd {
			left, leftIsStringAddition := simplifyUnusedStringAdditionChain(e.Left)

			if right, rightIsString := e.Right.Data.(*EString); rightIsString {
				// "('' + x) + 'y'" => "'' + x"
				if leftIsStringAddition {
					return left, true
				}

				// "x + 'y'" => "x + ''"
				if !leftIsStringAddition && len(right.Value) > 0 {
					return Expr{Loc: expr.Loc, Data: &EBinary{
						Op:    BinOpAdd,
						Left:  left,
						Right: Expr{Loc: e.Right.Loc, Data: &EString{}},
					}}, true
				}
			}

			// Don't mutate the original AST
			if left != e.Left {
				expr.Data = &EBinary{Op: BinOpAdd, Left: left, Right: e.Right}
			}

			return expr, leftIsStringAddition
		}
	}

	return expr, false
}

func ToInt32(f float64) int32 {
	// The easy way
	i := int32(f)
	if float64(i) == f {
		return i
	}

	// The hard way
	i = int32(uint32(math.Mod(math.Abs(f), 4294967296)))
	if math.Signbit(f) {
		return -i
	}
	return i
}

func ToUint32(f float64) uint32 {
	return uint32(ToInt32(f))
}

func isInt32OrUint32(data E) bool {
	switch e := data.(type) {
	case *EUnary:
		return e.Op == UnOpCpl

	case *EBinary:
		switch e.Op {
		case BinOpBitwiseAnd, BinOpBitwiseOr, BinOpBitwiseXor, BinOpShl, BinOpShr, BinOpUShr:
			return true

		case BinOpLogicalOr, BinOpLogicalAnd:
			return isInt32OrUint32(e.Left.Data) && isInt32OrUint32(e.Right.Data)
		}

	case *EIf:
		return isInt32OrUint32(e.Yes.Data) && isInt32OrUint32(e.No.Data)
	}
	return false
}

func ToNumberWithoutSideEffects(data E) (float64, bool) {
	switch e := data.(type) {
	case *EAnnotation:
		return ToNumberWithoutSideEffects(e.Value.Data)

	case *EInlinedEnum:
		return ToNumberWithoutSideEffects(e.Value.Data)

	case *ENull:
		return 0, true

	case *EUndefined:
		return math.NaN(), true

	case *EBoolean:
		if e.Value {
			return 1, true
		} else {
			return 0, true
		}

	case *ENumber:
		return e.Value, true
	}

	return 0, false
}

func extractNumericValue(data E) (float64, bool) {
	switch e := data.(type) {
	case *EAnnotation:
		return extractNumericValue(e.Value.Data)

	case *EInlinedEnum:
		return extractNumericValue(e.Value.Data)

	case *ENumber:
		return e.Value, true
	}

	return 0, false
}

func extractNumericValues(left Expr, right Expr) (float64, float64, bool) {
	if a, ok := extractNumericValue(left.Data); ok {
		if b, ok := extractNumericValue(right.Data); ok {
			return a, b, true
		}
	}
	return 0, 0, false
}

func approximatePrintedIntCharCount(intValue float64) int {
	count := 1 + (int)(math.Max(0, math.Floor(math.Log10(math.Abs(intValue)))))
	if intValue < 0 {
		count++
	}
	return count
}

func ShouldFoldBinaryArithmeticWhenMinifying(binary *EBinary) bool {
	switch binary.Op {
	case
		// Minification always folds right signed shift operations since they are
		// unlikely to result in larger output. Note: ">>>" could result in
		// bigger output such as "-1 >>> 0" becoming "4294967295".
		BinOpShr,

		// Minification always folds the following bitwise operations since they
		// are unlikely to result in larger output.
		BinOpBitwiseAnd,
		BinOpBitwiseOr,
		BinOpBitwiseXor:
		return true

	case BinOpShl:
		// "1 << 3" => "8"
		// "1 << 24" => "1 << 24" (since "1<<24" is shorter than "16777216")
		if left, right, ok := extractNumericValues(binary.Left, binary.Right); ok {
			leftLen := approximatePrintedIntCharCount(left)
			rightLen := approximatePrintedIntCharCount(right)
			resultLen := approximatePrintedIntCharCount(float64(ToInt32(left) << (ToUint32(right) & 31)))
			return resultLen <= leftLen+2+rightLen
		}

	case BinOpUShr:
		// "10 >>> 1" => "5"
		// "-1 >>> 0" => "-1 >>> 0" (since "-1>>>0" is shorter than "4294967295")
		if left, right, ok := extractNumericValues(binary.Left, binary.Right); ok {
			leftLen := approximatePrintedIntCharCount(left)
			rightLen := approximatePrintedIntCharCount(right)
			resultLen := approximatePrintedIntCharCount(float64(ToUint32(left) >> (ToUint32(right) & 31)))
			return resultLen <= leftLen+3+rightLen
		}
	}
	return false
}

// This function intentionally avoids mutating the input AST so it can be
// called after the AST has been frozen (i.e. after parsing ends).
func FoldBinaryArithmetic(loc logger.Loc, e *EBinary) Expr {
	switch e.Op {
	case BinOpAdd:
		if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
			return Expr{Loc: loc, Data: &ENumber{Value: left + right}}
		}

	case BinOpSub:
		if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
			return Expr{Loc: loc, Data: &ENumber{Value: left - right}}
		}

	case BinOpMul:
		if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
			return Expr{Loc: loc, Data: &ENumber{Value: left * right}}
		}

	case BinOpDiv:
		if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
			return Expr{Loc: loc, Data: &ENumber{Value: left / right}}
		}

	case BinOpRem:
		if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
			return Expr{Loc: loc, Data: &ENumber{Value: math.Mod(left, right)}}
		}

	case BinOpPow:
		if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
			return Expr{Loc: loc, Data: &ENumber{Value: math.Pow(left, right)}}
		}

	case BinOpShl:
		if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
			return Expr{Loc: loc, Data: &ENumber{Value: float64(ToInt32(left) << (ToUint32(right) & 31))}}
		}

	case BinOpShr:
		if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
			return Expr{Loc: loc, Data: &ENumber{Value: float64(ToInt32(left) >> (ToUint32(right) & 31))}}
		}

	case BinOpUShr:
		if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
			return Expr{Loc: loc, Data: &ENumber{Value: float64(ToUint32(left) >> (ToUint32(right) & 31))}}
		}

	case BinOpBitwiseAnd:
		if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
			return Expr{Loc: loc, Data: &ENumber{Value: float64(ToInt32(left) & ToInt32(right))}}
		}

	case BinOpBitwiseOr:
		if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
			return Expr{Loc: loc, Data: &ENumber{Value: float64(ToInt32(left) | ToInt32(right))}}
		}

	case BinOpBitwiseXor:
		if left, right, ok := extractNumericValues(e.Left, e.Right); ok {
			return Expr{Loc: loc, Data: &ENumber{Value: float64(ToInt32(left) ^ ToInt32(right))}}
		}
	}

	return Expr{}
}

func IsBinaryNullAndUndefined(left Expr, right Expr, op OpCode) (Expr, Expr, bool) {
	if a, ok := left.Data.(*EBinary); ok && a.Op == op {
		if b, ok := right.Data.(*EBinary); ok && b.Op == op {
			idA, eqA := a.Left, a.Right
			idB, eqB := b.Left, b.Right

			// Detect when the identifier comes second and flip the order of our checks
			if _, ok := eqA.Data.(*EIdentifier); ok {
				idA, eqA = eqA, idA
			}
			if _, ok := eqB.Data.(*EIdentifier); ok {
				idB, eqB = eqB, idB
			}

			if idA, ok := idA.Data.(*EIdentifier); ok {
				if idB, ok := idB.Data.(*EIdentifier); ok && idA.Ref == idB.Ref {
					// "a === null || a === void 0"
					if _, ok := eqA.Data.(*ENull); ok {
						if _, ok := eqB.Data.(*EUndefined); ok {
							return a.Left, a.Right, true
						}
					}

					// "a === void 0 || a === null"
					if _, ok := eqA.Data.(*EUndefined); ok {
						if _, ok := eqB.Data.(*ENull); ok {
							return b.Left, b.Right, true
						}
					}
				}
			}
		}
	}

	return Expr{}, Expr{}, false
}

type EqualityKind uint8

const (
	LooseEquality EqualityKind = iota
	StrictEquality
)

// Returns "equal, ok". If "ok" is false, then nothing is known about the two
// values. If "ok" is true, the equality or inequality of the two values is
// stored in "equal".
func CheckEqualityIfNoSideEffects(left E, right E, kind EqualityKind) (equal bool, ok bool) {
	if r, ok := right.(*EInlinedEnum); ok {
		return CheckEqualityIfNoSideEffects(left, r.Value.Data, kind)
	}

	switch l := left.(type) {
	case *EInlinedEnum:
		return CheckEqualityIfNoSideEffects(l.Value.Data, right, kind)

	case *ENull:
		switch right.(type) {
		case *ENull:
			// "null === null" is true
			return true, true

		case *EUndefined:
			// "null == undefined" is true
			// "null === undefined" is false
			return kind == LooseEquality, true

		default:
			if IsPrimitiveLiteral(right) {
				// "null == (not null or undefined)" is false
				return false, true
			}
		}

	case *EUndefined:
		switch right.(type) {
		case *EUndefined:
			// "undefined === undefined" is true
			return true, true

		case *ENull:
			// "undefined == null" is true
			// "undefined === null" is false
			return kind == LooseEquality, true

		default:
			if IsPrimitiveLiteral(right) {
				// "undefined == (not null or undefined)" is false
				return false, true
			}
		}

	case *EBoolean:
		switch r := right.(type) {
		case *EBoolean:
			// "false === false" is true
			// "false === true" is false
			return l.Value == r.Value, true

		case *ENumber:
			if kind == LooseEquality {
				if l.Value {
					// "true == 1" is true
					return r.Value == 1, true
				} else {
					// "false == 0" is true
					return r.Value == 0, true
				}
			} else {
				// "true === 1" is false
				// "false === 0" is false
				return false, true
			}

		case *ENull, *EUndefined:
			// "(not null or undefined) == undefined" is false
			return false, true
		}

	case *ENumber:
		switch r := right.(type) {
		case *ENumber:
			// "0 === 0" is true
			// "0 === 1" is false
			return l.Value == r.Value, true

		case *EBoolean:
			if kind == LooseEquality {
				if r.Value {
					// "1 == true" is true
					return l.Value == 1, true
				} else {
					// "0 == false" is true
					return l.Value == 0, true
				}
			} else {
				// "1 === true" is false
				// "0 === false" is false
				return false, true
			}

		case *ENull, *EUndefined:
			// "(not null or undefined) == undefined" is false
			return false, true
		}

	case *EBigInt:
		switch r := right.(type) {
		case *EBigInt:
			// "0n === 0n" is true
			// "0n === 1n" is false
			return l.Value == r.Value, true

		case *ENull, *EUndefined:
			// "(not null or undefined) == undefined" is false
			return false, true
		}

	case *EString:
		switch r := right.(type) {
		case *EString:
			// "'a' === 'a'" is true
			// "'a' === 'b'" is false
			return helpers.UTF16EqualsUTF16(l.Value, r.Value), true

		case *ENull, *EUndefined:
			// "(not null or undefined) == undefined" is false
			return false, true
		}
	}

	return false, false
}

func ValuesLookTheSame(left E, right E) bool {
	if b, ok := right.(*EInlinedEnum); ok {
		return ValuesLookTheSame(left, b.Value.Data)
	}

	switch a := left.(type) {
	case *EInlinedEnum:
		return ValuesLookTheSame(a.Value.Data, right)

	case *EIdentifier:
		if b, ok := right.(*EIdentifier); ok && a.Ref == b.Ref {
			return true
		}

	case *EDot:
		if b, ok := right.(*EDot); ok && a.HasSameFlagsAs(b) &&
			a.Name == b.Name && ValuesLookTheSame(a.Target.Data, b.Target.Data) {
			return true
		}

	case *EIndex:
		if b, ok := right.(*EIndex); ok && a.HasSameFlagsAs(b) &&
			ValuesLookTheSame(a.Target.Data, b.Target.Data) && ValuesLookTheSame(a.Index.Data, b.Index.Data) {
			return true
		}

	case *EIf:
		if b, ok := right.(*EIf); ok && ValuesLookTheSame(a.Test.Data, b.Test.Data) &&
			ValuesLookTheSame(a.Yes.Data, b.Yes.Data) && ValuesLookTheSame(a.No.Data, b.No.Data) {
			return true
		}

	case *EUnary:
		if b, ok := right.(*EUnary); ok && a.Op == b.Op && ValuesLookTheSame(a.Value.Data, b.Value.Data) {
			return true
		}

	case *EBinary:
		if b, ok := right.(*EBinary); ok && a.Op == b.Op && ValuesLookTheSame(a.Left.Data, b.Left.Data) &&
			ValuesLookTheSame(a.Right.Data, b.Right.Data) {
			return true
		}

	case *ECall:
		if b, ok := right.(*ECall); ok && a.HasSameFlagsAs(b) &&
			len(a.Args) == len(b.Args) && ValuesLookTheSame(a.Target.Data, b.Target.Data) {
			for i := range a.Args {
				if !ValuesLookTheSame(a.Args[i].Data, b.Args[i].Data) {
					return false
				}
			}
			return true
		}

	// Special-case to distinguish between negative an non-negative zero when mangling
	// "a ? -0 : 0" => "a ? -0 : 0"
	// https://developer.mozilla.org/en-US/docs/Web/JavaScript/Equality_comparisons_and_sameness
	case *ENumber:
		b, ok := right.(*ENumber)
		if ok && a.Value == 0 && b.Value == 0 && math.Signbit(a.Value) != math.Signbit(b.Value) {
			return false
		}
	}

	equal, ok := CheckEqualityIfNoSideEffects(left, right, StrictEquality)
	return ok && equal
}

func TryToInsertOptionalChain(test Expr, expr Expr) bool {
	switch e := expr.Data.(type) {
	case *EDot:
		if ValuesLookTheSame(test.Data, e.Target.Data) {
			e.OptionalChain = OptionalChainStart
			return true
		}
		if TryToInsertOptionalChain(test, e.Target) {
			if e.OptionalChain == OptionalChainNone {
				e.OptionalChain = OptionalChainContinue
			}
			return true
		}

	case *EIndex:
		if ValuesLookTheSame(test.Data, e.Target.Data) {
			e.OptionalChain = OptionalChainStart
			return true
		}
		if TryToInsertOptionalChain(test, e.Target) {
			if e.OptionalChain == OptionalChainNone {
				e.OptionalChain = OptionalChainContinue
			}
			return true
		}

	case *ECall:
		if ValuesLookTheSame(test.Data, e.Target.Data) {
			e.OptionalChain = OptionalChainStart
			return true
		}
		if TryToInsertOptionalChain(test, e.Target) {
			if e.OptionalChain == OptionalChainNone {
				e.OptionalChain = OptionalChainContinue
			}
			return true
		}
	}

	return false
}

func joinStrings(a []uint16, b []uint16) []uint16 {
	data := make([]uint16, len(a)+len(b))
	copy(data[:len(a)], a)
	copy(data[len(a):], b)
	return data
}

// String concatenation with numbers is required by the TypeScript compiler for
// "constant expression" handling in enums. However, we don't want to introduce
// correctness bugs by accidentally stringifying a number differently than how
// a real JavaScript VM would do it. So we are conservative and we only do this
// when we know it'll be the same result.
func tryToStringOnNumberSafely(n float64) (string, bool) {
	if i := int32(n); float64(i) == n {
		return strconv.Itoa(int(i)), true
	}
	if math.IsNaN(n) {
		return "NaN", true
	}
	if math.IsInf(n, 1) {
		return "Infinity", true
	}
	if math.IsInf(n, -1) {
		return "-Infinity", true
	}
	return "", false
}

type StringAdditionKind uint8

const (
	StringAdditionNormal StringAdditionKind = iota
	StringAdditionWithNestedLeft
)

// This function intentionally avoids mutating the input AST so it can be
// called after the AST has been frozen (i.e. after parsing ends).
func FoldStringAddition(left Expr, right Expr, kind StringAdditionKind) Expr {
	// "See through" inline enum constants
	if l, ok := left.Data.(*EInlinedEnum); ok {
		left = l.Value
	}
	if r, ok := right.Data.(*EInlinedEnum); ok {
		right = r.Value
	}

	// Transforming the left operand into a string is not safe if it comes from
	// a nested AST node. The following transforms are invalid:
	//
	//   "0 + 1 + 'x'" => "0 + '1x'"
	//   "0 + 1 + `${x}`" => "0 + `1${x}`"
	//
	if kind != StringAdditionWithNestedLeft {
		if l, ok := left.Data.(*ENumber); ok {
			switch right.Data.(type) {
			case *EString, *ETemplate:
				// "0 + 'x'" => "0 + 'x'"
				// "0 + `${x}`" => "0 + `${x}`"
				if str, ok := tryToStringOnNumberSafely(l.Value); ok {
					left.Data = &EString{Value: helpers.StringToUTF16(str)}
				}
			}
		}
	}

	switch l := left.Data.(type) {
	case *EString:
		// "'x' + 0" => "'x' + '0'"
		if r, ok := right.Data.(*ENumber); ok {
			if str, ok := tryToStringOnNumberSafely(r.Value); ok {
				right.Data = &EString{Value: helpers.StringToUTF16(str)}
			}
		}

		switch r := right.Data.(type) {
		case *EString:
			// "'x' + 'y'" => "'xy'"
			return Expr{Loc: left.Loc, Data: &EString{
				Value:          joinStrings(l.Value, r.Value),
				PreferTemplate: l.PreferTemplate || r.PreferTemplate,
			}}

		case *ETemplate:
			if r.TagOrNil.Data == nil {
				// "'x' + `y${z}`" => "`xy${z}`"
				return Expr{Loc: left.Loc, Data: &ETemplate{
					HeadLoc:    left.Loc,
					HeadCooked: joinStrings(l.Value, r.HeadCooked),
					Parts:      r.Parts,
				}}
			}
		}

		// "'' + typeof x" => "typeof x"
		if len(l.Value) == 0 && KnownPrimitiveType(right.Data) == PrimitiveString {
			return right
		}

	case *ETemplate:
		if l.TagOrNil.Data == nil {
			// "`${x}` + 0" => "`${x}` + '0'"
			if r, ok := right.Data.(*ENumber); ok {
				if str, ok := tryToStringOnNumberSafely(r.Value); ok {
					right.Data = &EString{Value: helpers.StringToUTF16(str)}
				}
			}

			switch r := right.Data.(type) {
			case *EString:
				// "`${x}y` + 'z'" => "`${x}yz`"
				n := len(l.Parts)
				head := l.HeadCooked
				parts := make([]TemplatePart, n)
				if n == 0 {
					head = joinStrings(head, r.Value)
				} else {
					copy(parts, l.Parts)
					parts[n-1].TailCooked = joinStrings(parts[n-1].TailCooked, r.Value)
				}
				return Expr{Loc: left.Loc, Data: &ETemplate{
					HeadLoc:    l.HeadLoc,
					HeadCooked: head,
					Parts:      parts,
				}}

			case *ETemplate:
				if r.TagOrNil.Data == nil {
					// "`${a}b` + `x${y}`" => "`${a}bx${y}`"
					n := len(l.Parts)
					head := l.HeadCooked
					parts := make([]TemplatePart, n+len(r.Parts))
					copy(parts[n:], r.Parts)
					if n == 0 {
						head = joinStrings(head, r.HeadCooked)
					} else {
						copy(parts[:n], l.Parts)
						parts[n-1].TailCooked = joinStrings(parts[n-1].TailCooked, r.HeadCooked)
					}
					return Expr{Loc: left.Loc, Data: &ETemplate{
						HeadLoc:    l.HeadLoc,
						HeadCooked: head,
						Parts:      parts,
					}}
				}
			}
		}
	}

	// "typeof x + ''" => "typeof x"
	if r, ok := right.Data.(*EString); ok && len(r.Value) == 0 && KnownPrimitiveType(left.Data) == PrimitiveString {
		return left
	}

	return Expr{}
}

// "`a${'b'}c`" => "`abc`"
//
// This function intentionally avoids mutating the input AST so it can be
// called after the AST has been frozen (i.e. after parsing ends).
func InlineStringsAndNumbersIntoTemplate(loc logger.Loc, e *ETemplate) Expr {
	// Can't inline strings if there's a custom template tag
	if e.TagOrNil.Data != nil {
		return Expr{Loc: loc, Data: e}
	}

	headCooked := e.HeadCooked
	parts := make([]TemplatePart, 0, len(e.Parts))

	for _, part := range e.Parts {
		if value, ok := part.Value.Data.(*EInlinedEnum); ok {
			part.Value = value.Value
		}
		if value, ok := part.Value.Data.(*ENumber); ok {
			if str, ok := tryToStringOnNumberSafely(value.Value); ok {
				part.Value.Data = &EString{Value: helpers.StringToUTF16(str)}
			}
		}
		if str, ok := part.Value.Data.(*EString); ok {
			if len(parts) == 0 {
				headCooked = append(append(headCooked, str.Value...), part.TailCooked...)
			} else {
				prevPart := &parts[len(parts)-1]
				prevPart.TailCooked = append(append(prevPart.TailCooked, str.Value...), part.TailCooked...)
			}
		} else {
			parts = append(parts, part)
		}
	}

	// Become a plain string if there are no substitutions
	if len(parts) == 0 {
		return Expr{Loc: loc, Data: &EString{
			Value:          headCooked,
			PreferTemplate: true,
		}}
	}

	return Expr{Loc: loc, Data: &ETemplate{
		HeadLoc:    e.HeadLoc,
		HeadCooked: headCooked,
		Parts:      parts,
	}}
}

type SideEffects uint8

const (
	CouldHaveSideEffects SideEffects = iota
	NoSideEffects
)

func ToNullOrUndefinedWithSideEffects(data E) (isNullOrUndefined bool, sideEffects SideEffects, ok bool) {
	switch e := data.(type) {
	case *EAnnotation:
		isNullOrUndefined, sideEffects, ok = ToNullOrUndefinedWithSideEffects(e.Value.Data)
		if e.Flags.Has(CanBeRemovedIfUnusedFlag) {
			sideEffects = NoSideEffects
		}
		return

	case *EInlinedEnum:
		return ToNullOrUndefinedWithSideEffects(e.Value.Data)

		// Never null or undefined
	case *EBoolean, *ENumber, *EString, *ERegExp,
		*EFunction, *EArrow, *EBigInt:
		return false, NoSideEffects, true

	// Never null or undefined
	case *EObject, *EArray, *EClass:
		return false, CouldHaveSideEffects, true

	// Always null or undefined
	case *ENull, *EUndefined:
		return true, NoSideEffects, true

	case *EUnary:
		switch e.Op {
		case
			// Always number or bigint
			UnOpPos, UnOpNeg, UnOpCpl,
			UnOpPreDec, UnOpPreInc, UnOpPostDec, UnOpPostInc,
			// Always boolean
			UnOpNot, UnOpDelete:
			return false, CouldHaveSideEffects, true

		// Always boolean
		case UnOpTypeof:
			if e.WasOriginallyTypeofIdentifier {
				// Expressions such as "typeof x" never have any side effects
				return false, NoSideEffects, true
			}
			return false, CouldHaveSideEffects, true

		// Always undefined
		case UnOpVoid:
			return true, CouldHaveSideEffects, true
		}

	case *EBinary:
		switch e.Op {
		case
			// Always string or number or bigint
			BinOpAdd, BinOpAddAssign,
			// Always number or bigint
			BinOpSub, BinOpMul, BinOpDiv, BinOpRem, BinOpPow,
			BinOpSubAssign, BinOpMulAssign, BinOpDivAssign, BinOpRemAssign, BinOpPowAssign,
			BinOpShl, BinOpShr, BinOpUShr,
			BinOpShlAssign, BinOpShrAssign, BinOpUShrAssign,
			BinOpBitwiseOr, BinOpBitwiseAnd, BinOpBitwiseXor,
			BinOpBitwiseOrAssign, BinOpBitwiseAndAssign, BinOpBitwiseXorAssign,
			// Always boolean
			BinOpLt, BinOpLe, BinOpGt, BinOpGe, BinOpIn, BinOpInstanceof,
			BinOpLooseEq, BinOpLooseNe, BinOpStrictEq, BinOpStrictNe:
			return false, CouldHaveSideEffects, true

		case BinOpComma:
			if isNullOrUndefined, _, ok := ToNullOrUndefinedWithSideEffects(e.Right.Data); ok {
				return isNullOrUndefined, CouldHaveSideEffects, true
			}
		}
	}

	return false, NoSideEffects, false
}

func ToBooleanWithSideEffects(data E) (boolean bool, sideEffects SideEffects, ok bool) {
	switch e := data.(type) {
	case *EAnnotation:
		boolean, sideEffects, ok = ToBooleanWithSideEffects(e.Value.Data)
		if e.Flags.Has(CanBeRemovedIfUnusedFlag) {
			sideEffects = NoSideEffects
		}
		return

	case *EInlinedEnum:
		return ToBooleanWithSideEffects(e.Value.Data)

	case *ENull, *EUndefined:
		return false, NoSideEffects, true

	case *EBoolean:
		return e.Value, NoSideEffects, true

	case *ENumber:
		return e.Value != 0 && !math.IsNaN(e.Value), NoSideEffects, true

	case *EBigInt:
		return e.Value != "0", NoSideEffects, true

	case *EString:
		return len(e.Value) > 0, NoSideEffects, true

	case *EFunction, *EArrow, *ERegExp:
		return true, NoSideEffects, true

	case *EObject, *EArray, *EClass:
		return true, CouldHaveSideEffects, true

	case *EUnary:
		switch e.Op {
		case UnOpVoid:
			return false, CouldHaveSideEffects, true

		case UnOpTypeof:
			// Never an empty string
			if e.WasOriginallyTypeofIdentifier {
				// Expressions such as "typeof x" never have any side effects
				return true, NoSideEffects, true
			}
			return true, CouldHaveSideEffects, true

		case UnOpNot:
			if boolean, SideEffects, ok := ToBooleanWithSideEffects(e.Value.Data); ok {
				return !boolean, SideEffects, true
			}
		}

	case *EBinary:
		switch e.Op {
		case BinOpLogicalOr:
			// "anything || truthy" is truthy
			if boolean, _, ok := ToBooleanWithSideEffects(e.Right.Data); ok && boolean {
				return true, CouldHaveSideEffects, true
			}

		case BinOpLogicalAnd:
			// "anything && falsy" is falsy
			if boolean, _, ok := ToBooleanWithSideEffects(e.Right.Data); ok && !boolean {
				return false, CouldHaveSideEffects, true
			}

		case BinOpComma:
			// "anything, truthy/falsy" is truthy/falsy
			if boolean, _, ok := ToBooleanWithSideEffects(e.Right.Data); ok {
				return boolean, CouldHaveSideEffects, true
			}
		}
	}

	return false, CouldHaveSideEffects, false
}

// Simplify syntax when we know it's used inside a boolean context
//
// This function intentionally avoids mutating the input AST so it can be
// called after the AST has been frozen (i.e. after parsing ends).
func SimplifyBooleanExpr(expr Expr) Expr {
	switch e := expr.Data.(type) {
	case *EUnary:
		if e.Op == UnOpNot {
			// "!!a" => "a"
			if e2, ok2 := e.Value.Data.(*EUnary); ok2 && e2.Op == UnOpNot {
				return SimplifyBooleanExpr(e2.Value)
			}

			// "!!!a" => "!a"
			return Expr{Loc: expr.Loc, Data: &EUnary{Op: UnOpNot, Value: SimplifyBooleanExpr(e.Value)}}
		}

	case *EBinary:
		left := e.Left
		right := e.Right

		switch e.Op {
		case BinOpStrictEq, BinOpStrictNe, BinOpLooseEq, BinOpLooseNe:
			if r, ok := extractNumericValue(right.Data); ok && r == 0 && isInt32OrUint32(left.Data) {
				// If the left is guaranteed to be an integer (e.g. not NaN,
				// Infinity, or a non-numeric value) then a test against zero
				// in a boolean context is unnecessary because the value is
				// only truthy if it's not zero.
				if e.Op == BinOpStrictNe || e.Op == BinOpLooseNe {
					// "if ((a | b) !== 0)" => "if (a | b)"
					return left
				} else {
					// "if ((a | b) === 0)" => "if (!(a | b))"
					return Not(left)
				}
			}

		case BinOpLogicalAnd:
			// "if (!!a && !!b)" => "if (a && b)"
			left = SimplifyBooleanExpr(left)
			right = SimplifyBooleanExpr(right)

			if boolean, SideEffects, ok := ToBooleanWithSideEffects(right.Data); ok && boolean && SideEffects == NoSideEffects {
				// "if (anything && truthyNoSideEffects)" => "if (anything)"
				return left
			}

		case BinOpLogicalOr:
			// "if (!!a || !!b)" => "if (a || b)"
			left = SimplifyBooleanExpr(left)
			right = SimplifyBooleanExpr(right)

			if boolean, SideEffects, ok := ToBooleanWithSideEffects(right.Data); ok && !boolean && SideEffects == NoSideEffects {
				// "if (anything || falsyNoSideEffects)" => "if (anything)"
				return left
			}
		}

		if left != e.Left || right != e.Right {
			return Expr{Loc: expr.Loc, Data: &EBinary{Op: e.Op, Left: left, Right: right}}
		}

	case *EIf:
		// "if (a ? !!b : !!c)" => "if (a ? b : c)"
		yes := SimplifyBooleanExpr(e.Yes)
		no := SimplifyBooleanExpr(e.No)

		if boolean, SideEffects, ok := ToBooleanWithSideEffects(yes.Data); ok && SideEffects == NoSideEffects {
			if boolean {
				// "if (anything1 ? truthyNoSideEffects : anything2)" => "if (anything1 || anything2)"
				return JoinWithLeftAssociativeOp(BinOpLogicalOr, e.Test, no)
			} else {
				// "if (anything1 ? falsyNoSideEffects : anything2)" => "if (!anything1 || anything2)"
				return JoinWithLeftAssociativeOp(BinOpLogicalAnd, Not(e.Test), no)
			}
		}

		if boolean, SideEffects, ok := ToBooleanWithSideEffects(no.Data); ok && SideEffects == NoSideEffects {
			if boolean {
				// "if (anything1 ? anything2 : truthyNoSideEffects)" => "if (!anything1 || anything2)"
				return JoinWithLeftAssociativeOp(BinOpLogicalOr, Not(e.Test), yes)
			} else {
				// "if (anything1 ? anything2 : falsyNoSideEffects)" => "if (anything1 && anything2)"
				return JoinWithLeftAssociativeOp(BinOpLogicalAnd, e.Test, yes)
			}
		}

		if yes != e.Yes || no != e.No {
			return Expr{Loc: expr.Loc, Data: &EIf{Test: e.Test, Yes: yes, No: no}}
		}
	}

	return expr
}

type StmtsCanBeRemovedIfUnusedFlags uint8

const (
	KeepExportClauses StmtsCanBeRemovedIfUnusedFlags = 1 << iota
)

func StmtsCanBeRemovedIfUnused(stmts []Stmt, flags StmtsCanBeRemovedIfUnusedFlags, isUnbound func(Ref) bool) bool {
	for _, stmt := range stmts {
		switch s := stmt.Data.(type) {
		case *SFunction, *SEmpty:
			// These never have side effects

		case *SImport:
			// Let these be removed if they are unused. Note that we also need to
			// check if the imported file is marked as "sideEffects: false" before we
			// can remove a SImport statement. Otherwise the import must be kept for
			// its side effects.

		case *SClass:
			if !ClassCanBeRemovedIfUnused(s.Class, isUnbound) {
				return false
			}

		case *SExpr:
			if s.IsFromClassOrFnThatCanBeRemovedIfUnused {
				// This statement was automatically generated when lowering a class
				// or function that we were able to analyze as having no side effects
				// before lowering. So we consider it to be removable. The assumption
				// here is that we are seeing at least all of the statements from the
				// class lowering operation all at once (although we may possibly be
				// seeing even more statements than that). Since we're making a binary
				// all-or-nothing decision about the side effects of these statements,
				// we can safely consider these to be side-effect free because we
				// aren't in danger of partially dropping some of the class setup code.
				return true
			}

			if !ExprCanBeRemovedIfUnused(s.Value, isUnbound) {
				return false
			}

		case *SLocal:
			// "await" is a side effect because it affects code timing
			if s.Kind == LocalAwaitUsing {
				return false
			}

			for _, decl := range s.Decls {
				if _, ok := decl.Binding.Data.(*BIdentifier); !ok {
					return false
				}
				if decl.ValueOrNil.Data != nil {
					if !ExprCanBeRemovedIfUnused(decl.ValueOrNil, isUnbound) {
						return false
					} else if s.Kind.IsUsing() {
						// "using" declarations are only side-effect free if they are initialized to null or undefined
						if t := KnownPrimitiveType(decl.ValueOrNil.Data); t != PrimitiveNull && t != PrimitiveUndefined {
							return false
						}
					}
				}
			}

		case *STry:
			if !StmtsCanBeRemovedIfUnused(s.Block.Stmts, 0, isUnbound) || (s.Finally != nil && !StmtsCanBeRemovedIfUnused(s.Finally.Block.Stmts, 0, isUnbound)) {
				return false
			}

		case *SExportFrom:
			// Exports are tracked separately, so this isn't necessary

		case *SExportClause:
			if (flags & KeepExportClauses) != 0 {
				return false
			}

		case *SExportDefault:
			switch s2 := s.Value.Data.(type) {
			case *SExpr:
				if !ExprCanBeRemovedIfUnused(s2.Value, isUnbound) {
					return false
				}

			case *SFunction:
				// These never have side effects

			case *SClass:
				if !ClassCanBeRemovedIfUnused(s2.Class, isUnbound) {
					return false
				}

			default:
				panic("Internal error")
			}

		default:
			// Assume that all statements not explicitly special-cased here have side
			// effects, and cannot be removed even if unused
			return false
		}
	}

	return true
}

func ClassCanBeRemovedIfUnused(class Class, isUnbound func(Ref) bool) bool {
	if len(class.Decorators) > 0 {
		return false
	}

	// Note: This check is incorrect. Extending a non-constructible object can
	// throw an error, which is a side effect:
	//
	//   async function x() {}
	//   class y extends x {}
	//
	// But refusing to tree-shake every class with a base class is not a useful
	// thing for a bundler to do. So we pretend that this edge case doesn't
	// exist. At the time of writing, both Rollup and Terser don't consider this
	// to be a side effect either.
	if class.ExtendsOrNil.Data != nil && !ExprCanBeRemovedIfUnused(class.ExtendsOrNil, isUnbound) {
		return false
	}

	for _, property := range class.Properties {
		if property.Kind == PropertyClassStaticBlock {
			if !StmtsCanBeRemovedIfUnused(property.ClassStaticBlock.Block.Stmts, 0, isUnbound) {
				return false
			}
			continue
		}

		if len(property.Decorators) > 0 {
			return false
		}

		if property.Flags.Has(PropertyIsComputed) && !IsPrimitiveLiteral(property.Key.Data) {
			return false
		}

		if property.Flags.Has(PropertyIsMethod) {
			if fn, ok := property.ValueOrNil.Data.(*EFunction); ok {
				for _, arg := range fn.Fn.Args {
					if len(arg.Decorators) > 0 {
						return false
					}
				}
			}
		}

		if property.Flags.Has(PropertyIsStatic) {
			if property.ValueOrNil.Data != nil && !ExprCanBeRemovedIfUnused(property.ValueOrNil, isUnbound) {
				return false
			}

			if property.InitializerOrNil.Data != nil && !ExprCanBeRemovedIfUnused(property.InitializerOrNil, isUnbound) {
				return false
			}

			// Legacy TypeScript static class fields are considered to have side
			// effects because they use assign semantics, not define semantics, and
			// that can trigger getters. For example:
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
			// Note that it's not possible to analyze the base class to determine that
			// these assignments are side-effect free. For example:
			//
			//   // Some code that already ran before your code
			//   Object.defineProperty(Object.prototype, 'foo', {
			//     set(x) { imporantSideEffect(x) }
			//   })
			//
			//   // Your code
			//   class Foo {
			//     static foo = 1
			//   }
			//
			if !class.UseDefineForClassFields && !property.Flags.Has(PropertyIsMethod) {
				return false
			}
		}
	}

	return true
}

func ExprCanBeRemovedIfUnused(expr Expr, isUnbound func(Ref) bool) bool {
	switch e := expr.Data.(type) {
	case *EAnnotation:
		return e.Flags.Has(CanBeRemovedIfUnusedFlag)

	case *EInlinedEnum:
		return ExprCanBeRemovedIfUnused(e.Value, isUnbound)

	case *ENull, *EUndefined, *EMissing, *EBoolean, *ENumber, *EBigInt,
		*EString, *EThis, *ERegExp, *EFunction, *EArrow, *EImportMeta:
		return true

	case *EDot:
		return e.CanBeRemovedIfUnused

	case *EClass:
		return ClassCanBeRemovedIfUnused(e.Class, isUnbound)

	case *EIdentifier:
		if e.MustKeepDueToWithStmt {
			return false
		}

		// Unbound identifiers cannot be removed because they can have side effects.
		// One possible side effect is throwing a ReferenceError if they don't exist.
		// Another one is a getter with side effects on the global object:
		//
		//   Object.defineProperty(globalThis, 'x', {
		//     get() {
		//       sideEffect();
		//     },
		//   });
		//
		// Be very careful about this possibility. It's tempting to treat all
		// identifier expressions as not having side effects but that's wrong. We
		// must make sure they have been declared by the code we are currently
		// compiling before we can tell that they have no side effects.
		//
		// Note that we currently ignore ReferenceErrors due to TDZ access. This is
		// incorrect but proper TDZ analysis is very complicated and would have to
		// be very conservative, which would inhibit a lot of optimizations of code
		// inside closures. This may need to be revisited if it proves problematic.
		if e.CanBeRemovedIfUnused || !isUnbound(e.Ref) {
			return true
		}

	case *EImportIdentifier:
		// References to an ES6 import item are always side-effect free in an
		// ECMAScript environment.
		//
		// They could technically have side effects if the imported module is a
		// CommonJS module and the import item was translated to a property access
		// (which esbuild's bundler does) and the property has a getter with side
		// effects.
		//
		// But this is very unlikely and respecting this edge case would mean
		// disabling tree shaking of all code that references an export from a
		// CommonJS module. It would also likely violate the expectations of some
		// developers because the code *looks* like it should be able to be tree
		// shaken.
		//
		// So we deliberately ignore this edge case and always treat import item
		// references as being side-effect free.
		return true

	case *EIf:
		return ExprCanBeRemovedIfUnused(e.Test, isUnbound) &&
			((isSideEffectFreeUnboundIdentifierRef(e.Yes, e.Test, true, isUnbound) || ExprCanBeRemovedIfUnused(e.Yes, isUnbound)) &&
				(isSideEffectFreeUnboundIdentifierRef(e.No, e.Test, false, isUnbound) || ExprCanBeRemovedIfUnused(e.No, isUnbound)))

	case *EArray:
		for _, item := range e.Items {
			if !ExprCanBeRemovedIfUnused(item, isUnbound) {
				return false
			}
		}
		return true

	case *EObject:
		for _, property := range e.Properties {
			// The key must still be evaluated if it's computed or a spread
			if property.Kind == PropertySpread || (property.Flags.Has(PropertyIsComputed) && !IsPrimitiveLiteral(property.Key.Data)) {
				return false
			}
			if property.ValueOrNil.Data != nil && !ExprCanBeRemovedIfUnused(property.ValueOrNil, isUnbound) {
				return false
			}
		}
		return true

	case *ECall:
		canCallBeRemoved := e.CanBeUnwrappedIfUnused

		// A call that has been marked "__PURE__" can be removed if all arguments
		// can be removed. The annotation causes us to ignore the target.
		if canCallBeRemoved {
			for _, arg := range e.Args {
				if !ExprCanBeRemovedIfUnused(arg, isUnbound) {
					return false
				}
			}
			return true
		}

	case *ENew:
		// A constructor call that has been marked "__PURE__" can be removed if all
		// arguments can be removed. The annotation causes us to ignore the target.
		if e.CanBeUnwrappedIfUnused {
			for _, arg := range e.Args {
				if !ExprCanBeRemovedIfUnused(arg, isUnbound) {
					return false
				}
			}
			return true
		}

	case *EUnary:
		switch e.Op {
		// These operators must not have any type conversions that can execute code
		// such as "toString" or "valueOf". They must also never throw any exceptions.
		case UnOpVoid, UnOpNot:
			return ExprCanBeRemovedIfUnused(e.Value, isUnbound)

		// The "typeof" operator doesn't do any type conversions so it can be removed
		// if the result is unused and the operand has no side effects. However, it
		// has a special case where if the operand is an identifier expression such
		// as "typeof x" and "x" doesn't exist, no reference error is thrown so the
		// operation has no side effects.
		case UnOpTypeof:
			if _, ok := e.Value.Data.(*EIdentifier); ok && e.WasOriginallyTypeofIdentifier {
				// Expressions such as "typeof x" never have any side effects
				return true
			}
			return ExprCanBeRemovedIfUnused(e.Value, isUnbound)
		}

	case *EBinary:
		switch e.Op {
		// These operators must not have any type conversions that can execute code
		// such as "toString" or "valueOf". They must also never throw any exceptions.
		case BinOpStrictEq, BinOpStrictNe, BinOpComma, BinOpNullishCoalescing:
			return ExprCanBeRemovedIfUnused(e.Left, isUnbound) && ExprCanBeRemovedIfUnused(e.Right, isUnbound)

		// Special-case "||" to make sure "typeof x === 'undefined' || x" can be removed
		case BinOpLogicalOr:
			return ExprCanBeRemovedIfUnused(e.Left, isUnbound) &&
				(isSideEffectFreeUnboundIdentifierRef(e.Right, e.Left, false, isUnbound) || ExprCanBeRemovedIfUnused(e.Right, isUnbound))

		// Special-case "&&" to make sure "typeof x !== 'undefined' && x" can be removed
		case BinOpLogicalAnd:
			return ExprCanBeRemovedIfUnused(e.Left, isUnbound) &&
				(isSideEffectFreeUnboundIdentifierRef(e.Right, e.Left, true, isUnbound) || ExprCanBeRemovedIfUnused(e.Right, isUnbound))

		// For "==" and "!=", pretend the operator was actually "===" or "!==". If
		// we know that we can convert it to "==" or "!=", then we can consider the
		// operator itself to have no side effects. This matters because our mangle
		// logic will convert "typeof x === 'object'" into "typeof x == 'object'"
		// and since "typeof x === 'object'" is considered to be side-effect free,
		// we must also consider "typeof x == 'object'" to be side-effect free.
		case BinOpLooseEq, BinOpLooseNe:
			return CanChangeStrictToLoose(e.Left, e.Right) && ExprCanBeRemovedIfUnused(e.Left, isUnbound) && ExprCanBeRemovedIfUnused(e.Right, isUnbound)

		// Special-case "<" and ">" with string, number, or bigint arguments
		case BinOpLt, BinOpGt, BinOpLe, BinOpGe:
			left := KnownPrimitiveType(e.Left.Data)
			switch left {
			case PrimitiveString, PrimitiveNumber, PrimitiveBigInt:
				return KnownPrimitiveType(e.Right.Data) == left && ExprCanBeRemovedIfUnused(e.Left, isUnbound) && ExprCanBeRemovedIfUnused(e.Right, isUnbound)
			}
		}

	case *ETemplate:
		// A template can be removed if it has no tag and every value has no side
		// effects and results in some kind of primitive, since all primitives
		// have a "ToString" operation with no side effects.
		if e.TagOrNil.Data == nil {
			for _, part := range e.Parts {
				if !ExprCanBeRemovedIfUnused(part.Value, isUnbound) || KnownPrimitiveType(part.Value.Data) == PrimitiveUnknown {
					return false
				}
			}
			return true
		}
	}

	// Assume all other expression types have side effects and cannot be removed
	return false
}

func isSideEffectFreeUnboundIdentifierRef(value Expr, guardCondition Expr, isYesBranch bool, isUnbound func(Ref) bool) bool {
	if id, ok := value.Data.(*EIdentifier); ok && isUnbound(id.Ref) {
		if binary, ok := guardCondition.Data.(*EBinary); ok {
			switch binary.Op {
			case BinOpStrictEq, BinOpStrictNe, BinOpLooseEq, BinOpLooseNe:
				// Pattern match for "typeof x !== <string>"
				typeof, string := binary.Left, binary.Right
				if _, ok := typeof.Data.(*EString); ok {
					typeof, string = string, typeof
				}
				if typeof, ok := typeof.Data.(*EUnary); ok && typeof.Op == UnOpTypeof && typeof.WasOriginallyTypeofIdentifier {
					if text, ok := string.Data.(*EString); ok {
						// In "typeof x !== 'undefined' ? x : null", the reference to "x" is side-effect free
						// In "typeof x === 'object' ? x : null", the reference to "x" is side-effect free
						if (helpers.UTF16EqualsString(text.Value, "undefined") == isYesBranch) ==
							(binary.Op == BinOpStrictNe || binary.Op == BinOpLooseNe) {
							if id2, ok := typeof.Value.Data.(*EIdentifier); ok && id2.Ref == id.Ref {
								return true
							}
						}
					}
				}

			case BinOpLt, BinOpGt, BinOpLe, BinOpGe:
				// Pattern match for "typeof x < <string>"
				typeof, string := binary.Left, binary.Right
				if _, ok := typeof.Data.(*EString); ok {
					typeof, string = string, typeof
					isYesBranch = !isYesBranch
				}
				if typeof, ok := typeof.Data.(*EUnary); ok && typeof.Op == UnOpTypeof && typeof.WasOriginallyTypeofIdentifier {
					if text, ok := string.Data.(*EString); ok && helpers.UTF16EqualsString(text.Value, "u") {
						// In "typeof x < 'u' ? x : null", the reference to "x" is side-effect free
						// In "typeof x > 'u' ? x : null", the reference to "x" is side-effect free
						if isYesBranch == (binary.Op == BinOpLt || binary.Op == BinOpLe) {
							if id2, ok := typeof.Value.Data.(*EIdentifier); ok && id2.Ref == id.Ref {
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}

func StringToEquivalentNumberValue(value []uint16) (float64, bool) {
	if len(value) > 0 {
		var intValue int32
		isNegative := false
		start := 0

		if value[0] == '-' && len(value) > 1 {
			isNegative = true
			start++
		}

		for _, c := range value[start:] {
			if c < '0' || c > '9' {
				return 0, false
			}
			intValue = intValue*10 + int32(c) - '0'
		}

		if isNegative {
			intValue = -intValue
		}

		if helpers.UTF16EqualsString(value, strconv.FormatInt(int64(intValue), 10)) {
			return float64(intValue), true
		}
	}

	return 0, false
}

// This function intentionally avoids mutating the input AST so it can be
// called after the AST has been frozen (i.e. after parsing ends).
func InlineSpreadsOfArrayLiterals(values []Expr) (results []Expr) {
	for _, value := range values {
		if spread, ok := value.Data.(*ESpread); ok {
			if array, ok := spread.Value.Data.(*EArray); ok {
				for _, item := range array.Items {
					if _, ok := item.Data.(*EMissing); ok {
						results = append(results, Expr{Loc: item.Loc, Data: EUndefinedShared})
					} else {
						results = append(results, item)
					}
				}
				continue
			}
		}
		results = append(results, value)
	}
	return
}

// This function intentionally avoids mutating the input AST so it can be
// called after the AST has been frozen (i.e. after parsing ends).
func MangleObjectSpread(properties []Property) []Property {
	var result []Property
	for _, property := range properties {
		if property.Kind == PropertySpread {
			switch v := property.ValueOrNil.Data.(type) {
			case *EBoolean, *ENull, *EUndefined, *ENumber,
				*EBigInt, *ERegExp, *EFunction, *EArrow:
				// This value is ignored because it doesn't have any of its own properties
				continue

			case *EObject:
				for i, p := range v.Properties {
					// Getters are evaluated at iteration time. The property
					// descriptor is not inlined into the caller. Since we are not
					// evaluating code at compile time, just bail if we hit one
					// and preserve the spread with the remaining properties.
					if p.Kind == PropertyGet || p.Kind == PropertySet {
						// Don't mutate the original AST
						clone := *v
						clone.Properties = v.Properties[i:]
						property.ValueOrNil.Data = &clone
						result = append(result, property)
						break
					}

					// Also bail if we hit a verbatim "__proto__" key. This will
					// actually set the prototype of the object being spread so
					// inlining it is not correct.
					if p.Kind == PropertyNormal && !p.Flags.Has(PropertyIsComputed) && !p.Flags.Has(PropertyIsMethod) {
						if str, ok := p.Key.Data.(*EString); ok && helpers.UTF16EqualsString(str.Value, "__proto__") {
							// Don't mutate the original AST
							clone := *v
							clone.Properties = v.Properties[i:]
							property.ValueOrNil.Data = &clone
							result = append(result, property)
							break
						}
					}

					result = append(result, p)
				}
				continue
			}
		}
		result = append(result, property)
	}
	return result
}

// This function intentionally avoids mutating the input AST so it can be
// called after the AST has been frozen (i.e. after parsing ends).
func MangleIfExpr(loc logger.Loc, e *EIf, unsupportedFeatures compat.JSFeature, isUnbound func(Ref) bool) Expr {
	test := e.Test
	yes := e.Yes
	no := e.No

	// "(a, b) ? c : d" => "a, b ? c : d"
	if comma, ok := test.Data.(*EBinary); ok && comma.Op == BinOpComma {
		return JoinWithComma(comma.Left, MangleIfExpr(comma.Right.Loc, &EIf{
			Test: comma.Right,
			Yes:  yes,
			No:   no,
		}, unsupportedFeatures, isUnbound))
	}

	// "!a ? b : c" => "a ? c : b"
	if not, ok := test.Data.(*EUnary); ok && not.Op == UnOpNot {
		test = not.Value
		yes, no = no, yes
	}

	if ValuesLookTheSame(yes.Data, no.Data) {
		// "/* @__PURE__ */ a() ? b : b" => "b"
		if ExprCanBeRemovedIfUnused(test, isUnbound) {
			return yes
		}

		// "a ? b : b" => "a, b"
		return JoinWithComma(test, yes)
	}

	// "a ? true : false" => "!!a"
	// "a ? false : true" => "!a"
	if y, ok := yes.Data.(*EBoolean); ok {
		if n, ok := no.Data.(*EBoolean); ok {
			if y.Value && !n.Value {
				return Not(Not(test))
			}
			if !y.Value && n.Value {
				return Not(test)
			}
		}
	}

	if id, ok := test.Data.(*EIdentifier); ok {
		// "a ? a : b" => "a || b"
		if id2, ok := yes.Data.(*EIdentifier); ok && id.Ref == id2.Ref {
			return JoinWithLeftAssociativeOp(BinOpLogicalOr, test, no)
		}

		// "a ? b : a" => "a && b"
		if id2, ok := no.Data.(*EIdentifier); ok && id.Ref == id2.Ref {
			return JoinWithLeftAssociativeOp(BinOpLogicalAnd, test, yes)
		}
	}

	// "a ? b ? c : d : d" => "a && b ? c : d"
	if yesIf, ok := yes.Data.(*EIf); ok && ValuesLookTheSame(yesIf.No.Data, no.Data) {
		return Expr{Loc: loc, Data: &EIf{Test: JoinWithLeftAssociativeOp(BinOpLogicalAnd, test, yesIf.Test), Yes: yesIf.Yes, No: no}}
	}

	// "a ? b : c ? b : d" => "a || c ? b : d"
	if noIf, ok := no.Data.(*EIf); ok && ValuesLookTheSame(yes.Data, noIf.Yes.Data) {
		return Expr{Loc: loc, Data: &EIf{Test: JoinWithLeftAssociativeOp(BinOpLogicalOr, test, noIf.Test), Yes: yes, No: noIf.No}}
	}

	// "a ? c : (b, c)" => "(a || b), c"
	if comma, ok := no.Data.(*EBinary); ok && comma.Op == BinOpComma && ValuesLookTheSame(yes.Data, comma.Right.Data) {
		return JoinWithComma(
			JoinWithLeftAssociativeOp(BinOpLogicalOr, test, comma.Left),
			comma.Right,
		)
	}

	// "a ? (b, c) : c" => "(a && b), c"
	if comma, ok := yes.Data.(*EBinary); ok && comma.Op == BinOpComma && ValuesLookTheSame(comma.Right.Data, no.Data) {
		return JoinWithComma(
			JoinWithLeftAssociativeOp(BinOpLogicalAnd, test, comma.Left),
			comma.Right,
		)
	}

	// "a ? b || c : c" => "(a && b) || c"
	if binary, ok := yes.Data.(*EBinary); ok && binary.Op == BinOpLogicalOr &&
		ValuesLookTheSame(binary.Right.Data, no.Data) {
		return Expr{Loc: loc, Data: &EBinary{
			Op:    BinOpLogicalOr,
			Left:  JoinWithLeftAssociativeOp(BinOpLogicalAnd, test, binary.Left),
			Right: binary.Right,
		}}
	}

	// "a ? c : b && c" => "(a || b) && c"
	if binary, ok := no.Data.(*EBinary); ok && binary.Op == BinOpLogicalAnd &&
		ValuesLookTheSame(yes.Data, binary.Right.Data) {
		return Expr{Loc: loc, Data: &EBinary{
			Op:    BinOpLogicalAnd,
			Left:  JoinWithLeftAssociativeOp(BinOpLogicalOr, test, binary.Left),
			Right: binary.Right,
		}}
	}

	// "a ? b(c, d) : b(e, d)" => "b(a ? c : e, d)"
	if y, ok := yes.Data.(*ECall); ok && len(y.Args) > 0 {
		if n, ok := no.Data.(*ECall); ok && len(n.Args) == len(y.Args) &&
			y.HasSameFlagsAs(n) && ValuesLookTheSame(y.Target.Data, n.Target.Data) {
			// Only do this if the condition can be reordered past the call target
			// without side effects. For example, if the test or the call target is
			// an unbound identifier, reordering could potentially mean evaluating
			// the code could throw a different ReferenceError.
			if ExprCanBeRemovedIfUnused(test, isUnbound) && ExprCanBeRemovedIfUnused(y.Target, isUnbound) {
				sameTailArgs := true
				for i, count := 1, len(y.Args); i < count; i++ {
					if !ValuesLookTheSame(y.Args[i].Data, n.Args[i].Data) {
						sameTailArgs = false
						break
					}
				}
				if sameTailArgs {
					yesSpread, yesIsSpread := y.Args[0].Data.(*ESpread)
					noSpread, noIsSpread := n.Args[0].Data.(*ESpread)

					// "a ? b(...c) : b(...e)" => "b(...a ? c : e)"
					if yesIsSpread && noIsSpread {
						// Don't mutate the original AST
						temp := EIf{Test: test, Yes: yesSpread.Value, No: noSpread.Value}
						clone := *y
						clone.Args = append([]Expr{}, clone.Args...)
						clone.Args[0] = Expr{Loc: loc, Data: &ESpread{Value: MangleIfExpr(loc, &temp, unsupportedFeatures, isUnbound)}}
						return Expr{Loc: loc, Data: &clone}
					}

					// "a ? b(c) : b(e)" => "b(a ? c : e)"
					if !yesIsSpread && !noIsSpread {
						// Don't mutate the original AST
						temp := EIf{Test: test, Yes: y.Args[0], No: n.Args[0]}
						clone := *y
						clone.Args = append([]Expr{}, clone.Args...)
						clone.Args[0] = MangleIfExpr(loc, &temp, unsupportedFeatures, isUnbound)
						return Expr{Loc: loc, Data: &clone}
					}
				}
			}
		}
	}

	// Try using the "??" or "?." operators
	if binary, ok := test.Data.(*EBinary); ok {
		var check Expr
		var whenNull Expr
		var whenNonNull Expr

		switch binary.Op {
		case BinOpLooseEq:
			if _, ok := binary.Right.Data.(*ENull); ok {
				// "a == null ? _ : _"
				check = binary.Left
				whenNull = yes
				whenNonNull = no
			} else if _, ok := binary.Left.Data.(*ENull); ok {
				// "null == a ? _ : _"
				check = binary.Right
				whenNull = yes
				whenNonNull = no
			}

		case BinOpLooseNe:
			if _, ok := binary.Right.Data.(*ENull); ok {
				// "a != null ? _ : _"
				check = binary.Left
				whenNonNull = yes
				whenNull = no
			} else if _, ok := binary.Left.Data.(*ENull); ok {
				// "null != a ? _ : _"
				check = binary.Right
				whenNonNull = yes
				whenNull = no
			}
		}

		if ExprCanBeRemovedIfUnused(check, isUnbound) {
			// "a != null ? a : b" => "a ?? b"
			if !unsupportedFeatures.Has(compat.NullishCoalescing) && ValuesLookTheSame(check.Data, whenNonNull.Data) {
				return JoinWithLeftAssociativeOp(BinOpNullishCoalescing, check, whenNull)
			}

			// "a != null ? a.b.c[d](e) : undefined" => "a?.b.c[d](e)"
			if !unsupportedFeatures.Has(compat.OptionalChain) {
				if _, ok := whenNull.Data.(*EUndefined); ok && TryToInsertOptionalChain(check, whenNonNull) {
					return whenNonNull
				}
			}
		}
	}

	// Don't mutate the original AST
	if test != e.Test || yes != e.Yes || no != e.No {
		return Expr{Loc: loc, Data: &EIf{Test: test, Yes: yes, No: no}}
	}

	return Expr{Loc: loc, Data: e}
}

func ForEachIdentifierBindingInDecls(decls []Decl, callback func(loc logger.Loc, b *BIdentifier)) {
	for _, decl := range decls {
		ForEachIdentifierBinding(decl.Binding, callback)
	}
}

func ForEachIdentifierBinding(binding Binding, callback func(loc logger.Loc, b *BIdentifier)) {
	switch b := binding.Data.(type) {
	case *BMissing:

	case *BIdentifier:
		callback(binding.Loc, b)

	case *BArray:
		for _, item := range b.Items {
			ForEachIdentifierBinding(item.Binding, callback)
		}

	case *BObject:
		for _, property := range b.Properties {
			ForEachIdentifierBinding(property.Value, callback)
		}

	default:
		panic("Internal error")
	}
}
