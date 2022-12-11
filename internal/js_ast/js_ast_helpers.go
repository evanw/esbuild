package js_ast

import (
	"math"

	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/logger"
)

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

// Returns true if this expression is known to result in a primitive value (i.e.
// null, undefined, boolean, number, bigint, or string), even if the expression
// cannot be removed due to side effects.
func IsPrimitiveWithSideEffects(data E) bool {
	switch e := data.(type) {
	case *EInlinedEnum:
		return IsPrimitiveWithSideEffects(e.Value.Data)

	case *ENull, *EUndefined, *EBoolean, *ENumber, *EBigInt, *EString:
		return true

	case *EUnary:
		switch e.Op {
		case
			// Number or bigint
			UnOpPos, UnOpNeg, UnOpCpl,
			UnOpPreDec, UnOpPreInc, UnOpPostDec, UnOpPostInc,
			// Boolean
			UnOpNot, UnOpDelete,
			// Undefined
			UnOpVoid,
			// String
			UnOpTypeof:
			return true
		}

	case *EBinary:
		switch e.Op {
		case
			// Boolean
			BinOpLt, BinOpLe, BinOpGt, BinOpGe, BinOpIn,
			BinOpInstanceof, BinOpLooseEq, BinOpLooseNe, BinOpStrictEq, BinOpStrictNe,
			// String, number, or bigint
			BinOpAdd, BinOpAddAssign,
			// Number or bigint
			BinOpSub, BinOpMul, BinOpDiv, BinOpRem, BinOpPow,
			BinOpSubAssign, BinOpMulAssign, BinOpDivAssign, BinOpRemAssign, BinOpPowAssign,
			BinOpShl, BinOpShr, BinOpUShr,
			BinOpShlAssign, BinOpShrAssign, BinOpUShrAssign,
			BinOpBitwiseOr, BinOpBitwiseAnd, BinOpBitwiseXor,
			BinOpBitwiseOrAssign, BinOpBitwiseAndAssign, BinOpBitwiseXorAssign:
			return true

		// These always return one of the arguments unmodified
		case BinOpLogicalAnd, BinOpLogicalOr, BinOpNullishCoalescing,
			BinOpLogicalAndAssign, BinOpLogicalOrAssign, BinOpNullishCoalescingAssign:
			return IsPrimitiveWithSideEffects(e.Left.Data) && IsPrimitiveWithSideEffects(e.Right.Data)

		case BinOpComma:
			return IsPrimitiveWithSideEffects(e.Right.Data)
		}

	case *EIf:
		return IsPrimitiveWithSideEffects(e.Yes.Data) && IsPrimitiveWithSideEffects(e.No.Data)
	}

	return false
}

// This will return a nil expression if the expression can be totally removed
func SimplifyUnusedExpr(expr Expr, unsupportedFeatures compat.JSFeature, isUnbound func(Ref) bool) Expr {
	switch e := expr.Data.(type) {
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
				if KnownPrimitiveType(part.Value) != PrimitiveUnknown {
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
				end := 0
				for _, item := range e.Items {
					item = SimplifyUnusedExpr(item, unsupportedFeatures, isUnbound)
					if item.Data != nil {
						e.Items[end] = item
						end++
					}
				}
				e.Items = e.Items[:end]
				return expr
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
				end := 0
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
					e.Properties[end] = property
					end++
				}
				e.Properties = e.Properties[:end]
				return expr
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
		e.Yes = SimplifyUnusedExpr(e.Yes, unsupportedFeatures, isUnbound)
		e.No = SimplifyUnusedExpr(e.No, unsupportedFeatures, isUnbound)

		// "foo() ? 1 : 2" => "foo()"
		if e.Yes.Data == nil && e.No.Data == nil {
			return SimplifyUnusedExpr(e.Test, unsupportedFeatures, isUnbound)
		}

		// "foo() ? 1 : bar()" => "foo() || bar()"
		if e.Yes.Data == nil {
			return JoinWithLeftAssociativeOp(BinOpLogicalOr, e.Test, e.No)
		}

		// "foo() ? bar() : 2" => "foo() && bar()"
		if e.No.Data == nil {
			return JoinWithLeftAssociativeOp(BinOpLogicalAnd, e.Test, e.Yes)
		}

	case *EUnary:
		switch e.Op {
		// These operators must not have any type conversions that can execute code
		// such as "toString" or "valueOf". They must also never throw any exceptions.
		case UnOpVoid, UnOpNot:
			return SimplifyUnusedExpr(e.Value, unsupportedFeatures, isUnbound)

		case UnOpTypeof:
			if _, ok := e.Value.Data.(*EIdentifier); ok && e.ValueWasOriginallyIdentifier {
				// "typeof x" must not be transformed into if "x" since doing so could
				// cause an exception to be thrown. Instead we can just remove it since
				// "typeof x" is special-cased in the standard to never throw.
				return Expr{}
			}
			return SimplifyUnusedExpr(e.Value, unsupportedFeatures, isUnbound)
		}

	case *EBinary:
		switch e.Op {
		// These operators must not have any type conversions that can execute code
		// such as "toString" or "valueOf". They must also never throw any exceptions.
		case BinOpStrictEq, BinOpStrictNe, BinOpComma:
			return JoinWithComma(SimplifyUnusedExpr(e.Left, unsupportedFeatures, isUnbound), SimplifyUnusedExpr(e.Right, unsupportedFeatures, isUnbound))

		// We can simplify "==" and "!=" even though they can call "toString" and/or
		// "valueOf" if we can statically determine that the types of both sides are
		// primitives. In that case there won't be any chance for user-defined
		// "toString" and/or "valueOf" to be called.
		case BinOpLooseEq, BinOpLooseNe:
			if IsPrimitiveWithSideEffects(e.Left.Data) && IsPrimitiveWithSideEffects(e.Right.Data) {
				return JoinWithComma(SimplifyUnusedExpr(e.Left, unsupportedFeatures, isUnbound), SimplifyUnusedExpr(e.Right, unsupportedFeatures, isUnbound))
			}

		case BinOpLogicalAnd, BinOpLogicalOr, BinOpNullishCoalescing:
			// If this is a boolean logical operation and the result is unused, then
			// we know the left operand will only be used for its boolean value and
			// can be simplified under that assumption
			if e.Op != BinOpNullishCoalescing {
				e.Left = SimplifyBooleanExpr(e.Left)
			}

			// Preserve short-circuit behavior: the left expression is only unused if
			// the right expression can be completely removed. Otherwise, the left
			// expression is important for the branch.
			e.Right = SimplifyUnusedExpr(e.Right, unsupportedFeatures, isUnbound)
			if e.Right.Data == nil {
				return SimplifyUnusedExpr(e.Left, unsupportedFeatures, isUnbound)
			}

			// Try to take advantage of the optional chain operator to shorten code
			if !unsupportedFeatures.Has(compat.OptionalChain) {
				if binary, ok := e.Left.Data.(*EBinary); ok {
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
						if id, ok := test.Data.(*EIdentifier); ok && !id.MustKeepDueToWithStmt && TryToInsertOptionalChain(test, e.Right) {
							return e.Right
						}
					}
				}
			}

		case BinOpAdd:
			if result, isStringAddition := simplifyUnusedStringAdditionChain(expr); isStringAddition {
				return result
			}
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
							target.Body.Block.Stmts[0].Data = &SReturn{ValueOrNil: s.Value}
							target.PreferExpr = true
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

func simplifyUnusedStringAdditionChain(expr Expr) (Expr, bool) {
	switch e := expr.Data.(type) {
	case *EString:
		// "'x' + y" => "'' + y"
		return Expr{Loc: expr.Loc, Data: &EString{}}, true

	case *EBinary:
		if e.Op == BinOpAdd {
			left, leftIsStringAddition := simplifyUnusedStringAdditionChain(e.Left)
			e.Left = left

			if _, rightIsString := e.Right.Data.(*EString); rightIsString {
				// "('' + x) + 'y'" => "'' + x"
				if leftIsStringAddition {
					return left, true
				}

				// "x + 'y'" => "x + ''"
				if !leftIsStringAddition {
					e.Right.Data = &EString{}
					return expr, true
				}
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
	case *EInlinedEnum:
		return extractNumericValue(e.Value.Data)

	case *ENumber:
		return e.Value, true
	}

	return 0, false
}

func ExtractNumericValues(left Expr, right Expr) (float64, float64, bool) {
	if a, ok := extractNumericValue(left.Data); ok {
		if b, ok := extractNumericValue(right.Data); ok {
			return a, b, true
		}
	}
	return 0, 0, false
}

// Returns "equal, ok". If "ok" is false, then nothing is known about the two
// values. If "ok" is true, the equality or inequality of the two values is
// stored in "equal".
func CheckEqualityIfNoSideEffects(left E, right E) (bool, bool) {
	if r, ok := right.(*EInlinedEnum); ok {
		return CheckEqualityIfNoSideEffects(left, r.Value.Data)
	}

	switch l := left.(type) {
	case *EInlinedEnum:
		return CheckEqualityIfNoSideEffects(l.Value.Data, right)

	case *ENull:
		_, ok := right.(*ENull)
		return ok, ok

	case *EUndefined:
		_, ok := right.(*EUndefined)
		return ok, ok

	case *EBoolean:
		r, ok := right.(*EBoolean)
		return ok && l.Value == r.Value, ok

	case *ENumber:
		r, ok := right.(*ENumber)
		return ok && l.Value == r.Value, ok

	case *EBigInt:
		r, ok := right.(*EBigInt)
		return ok && l.Value == r.Value, ok

	case *EString:
		r, ok := right.(*EString)
		return ok && helpers.UTF16EqualsUTF16(l.Value, r.Value), ok
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

	equal, ok := CheckEqualityIfNoSideEffects(left, right)
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

type SideEffects uint8

const (
	CouldHaveSideEffects SideEffects = iota
	NoSideEffects
)

func ToNullOrUndefinedWithSideEffects(data E) (isNullOrUndefined bool, sideEffects SideEffects, ok bool) {
	switch e := data.(type) {
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
			if e.ValueWasOriginallyIdentifier {
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

func ToBooleanWithSideEffects(data E) (boolean bool, SideEffects SideEffects, ok bool) {
	switch e := data.(type) {
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
			if e.ValueWasOriginallyIdentifier {
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
func SimplifyBooleanExpr(expr Expr) Expr {
	switch e := expr.Data.(type) {
	case *EUnary:
		if e.Op == UnOpNot {
			// "!!a" => "a"
			if e2, ok2 := e.Value.Data.(*EUnary); ok2 && e2.Op == UnOpNot {
				return SimplifyBooleanExpr(e2.Value)
			}

			// "!!!a" => "!a"
			e.Value = SimplifyBooleanExpr(e.Value)
		}

	case *EBinary:
		switch e.Op {
		case BinOpStrictEq, BinOpStrictNe, BinOpLooseEq, BinOpLooseNe:
			if right, ok := extractNumericValue(e.Right.Data); ok && right == 0 && isInt32OrUint32(e.Left.Data) {
				// If the left is guaranteed to be an integer (e.g. not NaN,
				// Infinity, or a non-numeric value) then a test against zero
				// in a boolean context is unnecessary because the value is
				// only truthy if it's not zero.
				if e.Op == BinOpStrictNe || e.Op == BinOpLooseNe {
					// "if ((a | b) !== 0)" => "if (a | b)"
					return e.Left
				} else {
					// "if ((a | b) === 0)" => "if (!(a | b))"
					return Not(e.Left)
				}
			}

		case BinOpLogicalAnd:
			// "if (!!a && !!b)" => "if (a && b)"
			e.Left = SimplifyBooleanExpr(e.Left)
			e.Right = SimplifyBooleanExpr(e.Right)

			if boolean, SideEffects, ok := ToBooleanWithSideEffects(e.Right.Data); ok && boolean && SideEffects == NoSideEffects {
				// "if (anything && truthyNoSideEffects)" => "if (anything)"
				return e.Left
			}

		case BinOpLogicalOr:
			// "if (!!a || !!b)" => "if (a || b)"
			e.Left = SimplifyBooleanExpr(e.Left)
			e.Right = SimplifyBooleanExpr(e.Right)

			if boolean, SideEffects, ok := ToBooleanWithSideEffects(e.Right.Data); ok && !boolean && SideEffects == NoSideEffects {
				// "if (anything || falsyNoSideEffects)" => "if (anything)"
				return e.Left
			}
		}

	case *EIf:
		// "if (a ? !!b : !!c)" => "if (a ? b : c)"
		e.Yes = SimplifyBooleanExpr(e.Yes)
		e.No = SimplifyBooleanExpr(e.No)

		if boolean, SideEffects, ok := ToBooleanWithSideEffects(e.Yes.Data); ok && SideEffects == NoSideEffects {
			if boolean {
				// "if (anything1 ? truthyNoSideEffects : anything2)" => "if (anything1 || anything2)"
				return JoinWithLeftAssociativeOp(BinOpLogicalOr, e.Test, e.No)
			} else {
				// "if (anything1 ? falsyNoSideEffects : anything2)" => "if (!anything1 || anything2)"
				return JoinWithLeftAssociativeOp(BinOpLogicalAnd, Not(e.Test), e.No)
			}
		}

		if boolean, SideEffects, ok := ToBooleanWithSideEffects(e.No.Data); ok && SideEffects == NoSideEffects {
			if boolean {
				// "if (anything1 ? anything2 : truthyNoSideEffects)" => "if (!anything1 || anything2)"
				return JoinWithLeftAssociativeOp(BinOpLogicalOr, Not(e.Test), e.Yes)
			} else {
				// "if (anything1 ? anything2 : falsyNoSideEffects)" => "if (anything1 && anything2)"
				return JoinWithLeftAssociativeOp(BinOpLogicalAnd, e.Test, e.Yes)
			}
		}
	}

	return expr
}
