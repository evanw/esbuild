// This file contains code for "lowering" syntax, which means converting it to
// older JavaScript. For example, "a ** b" becomes a call to "Math.pow(a, b)"
// when lowered. Which syntax is lowered is determined by the language target.

package parser

import (
	"fmt"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/lexer"
)

func (p *parser) markSyntaxFeature(feature compat.Feature, r ast.Range) (didGenerateError bool) {
	didGenerateError = true

	if !p.UnsupportedFeatures.Has(feature) {
		if feature == compat.TopLevelAwait {
			if p.Mode == config.ModeBundle {
				p.log.AddRangeError(&p.source, r, "Top-level await is currently not supported when bundling")
				return
			}
			if p.Mode == config.ModeConvertFormat && !p.OutputFormat.KeepES6ImportExportSyntax() {
				p.log.AddRangeError(&p.source, r, fmt.Sprintf(
					"Top-level await is currently not supported with the %q output format", p.OutputFormat.String()))
				return
			}
		}

		didGenerateError = false
		return
	}

	var name string
	where := "the configured target environment"

	switch feature {
	case compat.DefaultArgument:
		name = "default arguments"

	case compat.RestArgument:
		name = "rest arguments"

	case compat.ArraySpread:
		name = "array spread"

	case compat.ForOf:
		name = "for-of loops"

	case compat.ObjectAccessors:
		name = "object accessors"

	case compat.ObjectExtensions:
		name = "object literal extensions"

	case compat.TemplateLiteral:
		name = "tagged template literals"

	case compat.Destructuring:
		name = "destructuring"

	case compat.NewTarget:
		name = "new.target"

	case compat.Const:
		name = "const"

	case compat.Let:
		name = "let"

	case compat.Arrow:
		name = "arrow functions"

	case compat.Class:
		name = "class syntax"

	case compat.Generator:
		name = "generator functions"

	case compat.AsyncAwait:
		name = "async functions"

	case compat.AsyncGenerator:
		name = "async generator functions"

	case compat.ForAwait:
		name = "for-await loops"

	case compat.NestedRestBinding:
		name = "non-identifier array rest patterns"

	case compat.TopLevelAwait:
		p.log.AddRangeError(&p.source, r,
			fmt.Sprintf("Top-level await is not available in %s", where))
		return

	case compat.BigInt:
		// Transforming these will never be supported
		p.log.AddRangeError(&p.source, r,
			fmt.Sprintf("Big integer literals are not available in %s", where))
		return

	case compat.ImportMeta:
		// This can't be polyfilled
		p.log.AddRangeWarning(&p.source, r,
			fmt.Sprintf("\"import.meta\" is not available in %s and will be empty", where))
		return

	default:
		p.log.AddRangeError(&p.source, r,
			fmt.Sprintf("This feature is not available in %s", where))
		return
	}

	p.log.AddRangeError(&p.source, r,
		fmt.Sprintf("Transforming %s to %s is not supported yet", name, where))
	return
}

// Mark the feature if "loweredFeature" is unsupported. This is used when one
// feature is implemented in terms of another feature.
func (p *parser) markLoweredSyntaxFeature(feature compat.Feature, r ast.Range, loweredFeature compat.Feature) {
	if p.UnsupportedFeatures.Has(loweredFeature) {
		p.markSyntaxFeature(feature, r)
	}
}

func (p *parser) isPrivateUnsupported(private *ast.EPrivateIdentifier) bool {
	return p.UnsupportedFeatures.Has(p.symbols[private.Ref.InnerIndex].Kind.Feature())
}

func (p *parser) lowerFunction(
	isAsync *bool,
	args *[]ast.Arg,
	bodyLoc ast.Loc,
	bodyStmts *[]ast.Stmt,
	preferExpr *bool,
	hasRestArg *bool,
) {
	// Lower object rest binding patterns in function arguments
	if p.UnsupportedFeatures.Has(compat.ObjectRestSpread) {
		var prefixStmts []ast.Stmt

		// Lower each argument individually instead of lowering all arguments
		// together. There is a correctness tradeoff here around default values
		// for function arguments, with no right answer.
		//
		// Lowering all arguments together will preserve the order of side effects
		// for default values, but will mess up their scope:
		//
		//   // Side effect order: a(), b(), c()
		//   function foo([{[a()]: w, ...x}, y = b()], z = c()) {}
		//
		//   // Side effect order is correct but scope is wrong
		//   function foo(_a, _b) {
		//     var [[{[a()]: w, ...x}, y = b()], z = c()] = [_a, _b]
		//   }
		//
		// Lowering each argument individually will preserve the scope for default
		// values that don't contain object rest binding patterns, but will mess up
		// the side effect order:
		//
		//   // Side effect order: a(), b(), c()
		//   function foo([{[a()]: w, ...x}, y = b()], z = c()) {}
		//
		//   // Side effect order is wrong but scope for c() is correct
		//   function foo(_a, z = c()) {
		//     var [{[a()]: w, ...x}, y = b()] = _a
		//   }
		//
		// This transform chooses to lower each argument individually with the
		// thinking that perhaps scope matters more in real-world code than side
		// effect order.
		for i, arg := range *args {
			if bindingHasObjectRest(arg.Binding) {
				ref := p.generateTempRef(tempRefNoDeclare, "")
				target := p.convertBindingToExpr(arg.Binding, nil)
				init := ast.Expr{Loc: arg.Binding.Loc, Data: &ast.EIdentifier{Ref: ref}}
				p.recordUsage(ref)

				if decls, ok := p.lowerObjectRestToDecls(target, init, nil); ok {
					// Replace the binding but leave the default value intact
					(*args)[i].Binding.Data = &ast.BIdentifier{Ref: ref}

					// Append a variable declaration to the function body
					prefixStmts = append(prefixStmts, ast.Stmt{Loc: arg.Binding.Loc,
						Data: &ast.SLocal{Kind: ast.LocalVar, Decls: decls}})
				}
			}
		}

		if len(prefixStmts) > 0 {
			*bodyStmts = append(prefixStmts, *bodyStmts...)
		}
	}

	// Lower async functions
	if p.UnsupportedFeatures.Has(compat.AsyncAwait) && *isAsync {
		// Use the shortened form if we're an arrow function
		if preferExpr != nil {
			*preferExpr = true
		}

		// Determine the value for "this"
		thisValue, hasThisValue := p.valueForThis(bodyLoc)
		if !hasThisValue {
			thisValue = ast.Expr{Loc: bodyLoc, Data: &ast.EThis{}}
		}

		// Move the code into a nested generator function
		fn := ast.Fn{
			IsGenerator: true,
			Body:        ast.FnBody{Loc: bodyLoc, Stmts: *bodyStmts},
		}
		*bodyStmts = nil

		// Forward the arguments to the wrapper function
		usesArguments := p.argumentsRef != nil && p.symbolUses[*p.argumentsRef].CountEstimate > 0
		var forwardedArgs ast.Expr
		if len(*args) == 0 && !usesArguments {
			// Don't allocate anything if arguments aren't needed
			forwardedArgs = ast.Expr{Loc: bodyLoc, Data: &ast.ENull{}}
		} else {
			// Errors thrown during argument evaluation must reject the
			// resulting promise, which needs more complex code to handle
			couldThrowErrors := false
			for _, arg := range *args {
				if _, ok := arg.Binding.Data.(*ast.BIdentifier); !ok || arg.Default != nil {
					couldThrowErrors = true
					break
				}
			}

			// If code uses "arguments" then we must move the arguments to the inner
			// function. This is because you can modify arguments by assigning to
			// elements in the "arguments" object:
			//
			//   async function foo(x) {
			//     arguments[0] = 1;
			//     // "x" must be 1 here
			//   }
			//
			if !couldThrowErrors && !usesArguments {
				// Simple case: the arguments can stay on the outer function. It's
				// worth separating out the simple case because it's the common case
				// and it generates smaller code.
				forwardedArgs = ast.Expr{Loc: bodyLoc, Data: &ast.ENull{}}
			} else {
				// Complex case: the arguments must be moved to the inner function
				fn.Args = *args
				fn.HasRestArg = *hasRestArg
				*args = nil
				*hasRestArg = false

				// Make sure to not change the value of the "length" property
				for i, arg := range fn.Args {
					if arg.Default != nil || fn.HasRestArg && i+1 == len(fn.Args) {
						// Arguments from here on don't add to the "length"
						break
					}

					// Generate a dummy variable
					argRef := p.newSymbol(ast.SymbolOther, fmt.Sprintf("_%d", i))
					p.currentScope.Generated = append(p.currentScope.Generated, argRef)
					*args = append(*args, ast.Arg{Binding: ast.Binding{Loc: arg.Binding.Loc, Data: &ast.BIdentifier{Ref: argRef}}})
				}

				// Forward all arguments from the outer function to the inner function
				if p.argumentsRef != nil {
					// Normal functions can just use "arguments" to forward everything
					forwardedArgs = ast.Expr{Loc: bodyLoc, Data: &ast.EIdentifier{Ref: *p.argumentsRef}}
				} else {
					// Arrow functions can't use "arguments", so we need to forward
					// the arguments manually

					// If we need to forward more than the current number of arguments,
					// add a rest argument to the set of forwarding variables
					if usesArguments || fn.HasRestArg || len(*args) < len(fn.Args) {
						argRef := p.newSymbol(ast.SymbolOther, fmt.Sprintf("_%d", len(*args)))
						p.currentScope.Generated = append(p.currentScope.Generated, argRef)
						*args = append(*args, ast.Arg{Binding: ast.Binding{Loc: bodyLoc, Data: &ast.BIdentifier{Ref: argRef}}})
						*hasRestArg = true
					}

					// Forward all of the arguments
					items := make([]ast.Expr, 0, len(*args))
					for i, arg := range *args {
						id := arg.Binding.Data.(*ast.BIdentifier)
						item := ast.Expr{Loc: arg.Binding.Loc, Data: &ast.EIdentifier{Ref: id.Ref}}
						if *hasRestArg && i+1 == len(*args) {
							item.Data = &ast.ESpread{Value: item}
						}
						items = append(items, item)
					}
					forwardedArgs = ast.Expr{Loc: bodyLoc, Data: &ast.EArray{Items: items, IsSingleLine: true}}
				}
			}
		}

		// "async function foo(a, b) { stmts }" => "function foo(a, b) { return __async(this, null, function* () { stmts }) }"
		*isAsync = false
		callAsync := p.callRuntime(bodyLoc, "__async", []ast.Expr{
			thisValue,
			forwardedArgs,
			{Loc: bodyLoc, Data: &ast.EFunction{Fn: fn}},
		})
		*bodyStmts = []ast.Stmt{{Loc: bodyLoc, Data: &ast.SReturn{Value: &callAsync}}}
	}
}

func (p *parser) lowerOptionalChain(expr ast.Expr, in exprIn, out exprOut, thisArgFunc func() ast.Expr) (ast.Expr, exprOut) {
	valueWhenUndefined := ast.Expr{Loc: expr.Loc, Data: &ast.EUndefined{}}
	endsWithPropertyAccess := false
	containsPrivateName := false
	startsWithCall := false
	originalExpr := expr
	chain := []ast.Expr{}
	loc := expr.Loc

	// Step 1: Get an array of all expressions in the chain. We're traversing the
	// chain from the outside in, so the array will be filled in "backwards".
flatten:
	for {
		chain = append(chain, expr)

		switch e := expr.Data.(type) {
		case *ast.EDot:
			expr = e.Target
			if len(chain) == 1 {
				endsWithPropertyAccess = true
			}
			if e.OptionalChain == ast.OptionalChainStart {
				break flatten
			}

		case *ast.EIndex:
			expr = e.Target
			if len(chain) == 1 {
				endsWithPropertyAccess = true
			}

			// If this is a private name that needs to be lowered, the entire chain
			// itself will have to be lowered even if the language target supports
			// optional chaining. This is because there's no way to use our shim
			// function for private names with optional chaining syntax.
			if private, ok := e.Index.Data.(*ast.EPrivateIdentifier); ok && p.isPrivateUnsupported(private) {
				containsPrivateName = true
			}

			if e.OptionalChain == ast.OptionalChainStart {
				break flatten
			}

		case *ast.ECall:
			expr = e.Target
			if e.OptionalChain == ast.OptionalChainStart {
				startsWithCall = true
				break flatten
			}

		case *ast.EUnary: // UnOpDelete
			valueWhenUndefined = ast.Expr{Loc: loc, Data: &ast.EBoolean{Value: true}}
			expr = e.Value

		default:
			panic("Internal error")
		}
	}

	// Stop now if we can strip the whole chain as dead code. Since the chain is
	// lazily evaluated, it's safe to just drop the code entirely.
	switch expr.Data.(type) {
	case *ast.ENull, *ast.EUndefined:
		return valueWhenUndefined, exprOut{}
	}

	// We need to lower this if this is an optional call off of a private name
	// such as "foo.#bar?.()" because the value of "this" must be captured.
	if _, _, private := p.extractPrivateIndex(expr); private != nil {
		containsPrivateName = true
	}

	// Don't lower this if we don't need to. This check must be done here instead
	// of earlier so we can do the dead code elimination above when the target is
	// null or undefined.
	if !p.UnsupportedFeatures.Has(compat.OptionalChain) && !containsPrivateName {
		return originalExpr, exprOut{}
	}

	// Step 2: Figure out if we need to capture the value for "this" for the
	// initial ECall. This will be passed to ".call(this, ...args)" later.
	var thisArg ast.Expr
	var targetWrapFunc func(ast.Expr) ast.Expr
	if startsWithCall {
		if thisArgFunc != nil {
			// The initial value is a nested optional chain that ended in a property
			// access. The nested chain was processed first and has saved the
			// appropriate value for "this". The callback here will return a
			// reference to that saved location.
			thisArg = thisArgFunc()
		} else {
			// The initial value is a normal expression. If it's a property access,
			// strip the property off and save the target of the property access to
			// be used as the value for "this".
			switch e := expr.Data.(type) {
			case *ast.EDot:
				if _, ok := e.Target.Data.(*ast.ESuper); ok {
					// Special-case "super.foo?.()" to avoid a syntax error. Without this,
					// we would generate:
					//
					//   (_b = (_a = super).foo) == null ? void 0 : _b.call(_a)
					//
					// which is a syntax error. Now we generate this instead:
					//
					//   (_a = super.foo) == null ? void 0 : _a.call(this)
					//
					thisArg = ast.Expr{Loc: loc, Data: &ast.EThis{}}
				} else {
					targetFunc, wrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, e.Target)
					expr = ast.Expr{Loc: loc, Data: &ast.EDot{
						Target:  targetFunc(),
						Name:    e.Name,
						NameLoc: e.NameLoc,
					}}
					thisArg = targetFunc()
					targetWrapFunc = wrapFunc
				}

			case *ast.EIndex:
				if _, ok := e.Target.Data.(*ast.ESuper); ok {
					// See the comment above about a similar special case for EDot
					thisArg = ast.Expr{Loc: loc, Data: &ast.EThis{}}
				} else {
					targetFunc, wrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, e.Target)
					targetWrapFunc = wrapFunc

					// Capture the value of "this" if the target of the starting call
					// expression is a private property access
					if private, ok := e.Index.Data.(*ast.EPrivateIdentifier); ok && p.isPrivateUnsupported(private) {
						// "foo().#bar?.()" must capture "foo()" for "this"
						expr = p.lowerPrivateGet(targetFunc(), e.Index.Loc, private)
						thisArg = targetFunc()
						break
					}

					expr = ast.Expr{Loc: loc, Data: &ast.EIndex{
						Target: targetFunc(),
						Index:  e.Index,
					}}
					thisArg = targetFunc()
				}
			}
		}
	}

	// Step 3: Figure out if we need to capture the starting value. We don't need
	// to capture it if it doesn't have any side effects (e.g. it's just a bare
	// identifier). Skipping the capture reduces code size and matches the output
	// of the TypeScript compiler.
	exprFunc, exprWrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, expr)
	expr = exprFunc()
	result := exprFunc()

	// Step 4: Wrap the starting value by each expression in the chain. We
	// traverse the chain in reverse because we want to go from the inside out
	// and the chain was built from the outside in.
	var privateThisFunc func() ast.Expr
	var privateThisWrapFunc func(ast.Expr) ast.Expr
	for i := len(chain) - 1; i >= 0; i-- {
		// Save a reference to the value of "this" for our parent ECall
		if i == 0 && in.storeThisArgForParentOptionalChain != nil && endsWithPropertyAccess {
			result = in.storeThisArgForParentOptionalChain(result)
		}

		switch e := chain[i].Data.(type) {
		case *ast.EDot:
			result = ast.Expr{Loc: loc, Data: &ast.EDot{
				Target:  result,
				Name:    e.Name,
				NameLoc: e.NameLoc,
			}}

		case *ast.EIndex:
			if private, ok := e.Index.Data.(*ast.EPrivateIdentifier); ok && p.isPrivateUnsupported(private) {
				// If this is private name property access inside a call expression and
				// the call expression is part of this chain, then the call expression
				// is going to need a copy of the property access target as the value
				// for "this" for the call. Example for this case: "foo.#bar?.()"
				if i > 0 {
					if _, ok := chain[i-1].Data.(*ast.ECall); ok {
						privateThisFunc, privateThisWrapFunc = p.captureValueWithPossibleSideEffects(loc, 2, result)
						result = privateThisFunc()
					}
				}

				result = p.lowerPrivateGet(result, e.Index.Loc, private)
				continue
			}

			result = ast.Expr{Loc: loc, Data: &ast.EIndex{
				Target: result,
				Index:  e.Index,
			}}

		case *ast.ECall:
			// If this is the initial ECall in the chain and it's being called off of
			// a property access, invoke the function using ".call(this, ...args)" to
			// explicitly provide the value for "this".
			if i == len(chain)-1 && thisArg.Data != nil {
				result = ast.Expr{Loc: loc, Data: &ast.ECall{
					Target: ast.Expr{Loc: loc, Data: &ast.EDot{
						Target:  result,
						Name:    "call",
						NameLoc: loc,
					}},
					Args:                   append([]ast.Expr{thisArg}, e.Args...),
					CanBeUnwrappedIfUnused: e.CanBeUnwrappedIfUnused,
				}}
				break
			}

			// If the target of this call expression is a private name property
			// access that's also part of this chain, then we must use the copy of
			// the property access target that was stashed away earlier as the value
			// for "this" for the call. Example for this case: "foo.#bar?.()"
			if privateThisFunc != nil {
				result = privateThisWrapFunc(ast.Expr{Loc: loc, Data: &ast.ECall{
					Target: ast.Expr{Loc: loc, Data: &ast.EDot{
						Target:  result,
						Name:    "call",
						NameLoc: loc,
					}},
					Args:                   append([]ast.Expr{privateThisFunc()}, e.Args...),
					CanBeUnwrappedIfUnused: e.CanBeUnwrappedIfUnused,
				}})
				privateThisFunc = nil
				break
			}

			result = ast.Expr{Loc: loc, Data: &ast.ECall{
				Target:                 result,
				Args:                   e.Args,
				IsDirectEval:           e.IsDirectEval,
				CanBeUnwrappedIfUnused: e.CanBeUnwrappedIfUnused,
			}}

		case *ast.EUnary:
			result = ast.Expr{Loc: loc, Data: &ast.EUnary{
				Op:    ast.UnOpDelete,
				Value: result,
			}}

		default:
			panic("Internal error")
		}
	}

	// Step 5: Wrap it all in a conditional that returns the chain or the default
	// value if the initial value is null/undefined. The default value is usually
	// "undefined" but is "true" if the chain ends in a "delete" operator.
	if p.Strict.OptionalChaining {
		// "x?.y" => "x === null || x === void 0 ? void 0 : x.y"
		// "x()?.y()" => "(_a = x()) === null || _a === void 0 ? void 0 : _a.y()"
		result = ast.Expr{Loc: loc, Data: &ast.EIf{
			Test: ast.Expr{Loc: loc, Data: &ast.EBinary{
				Op: ast.BinOpLogicalOr,
				Left: ast.Expr{Loc: loc, Data: &ast.EBinary{
					Op:    ast.BinOpStrictEq,
					Left:  expr,
					Right: ast.Expr{Loc: loc, Data: &ast.ENull{}},
				}},
				Right: ast.Expr{Loc: loc, Data: &ast.EBinary{
					Op:    ast.BinOpStrictEq,
					Left:  exprFunc(),
					Right: ast.Expr{Loc: loc, Data: &ast.EUndefined{}},
				}},
			}},
			Yes: valueWhenUndefined,
			No:  result,
		}}
	} else {
		// "x?.y" => "x == null ? void 0 : x.y"
		// "x()?.y()" => "(_a = x()) == null ? void 0 : _a.y()"
		result = ast.Expr{Loc: loc, Data: &ast.EIf{
			Test: ast.Expr{Loc: loc, Data: &ast.EBinary{
				Op:    ast.BinOpLooseEq,
				Left:  expr,
				Right: ast.Expr{Loc: loc, Data: &ast.ENull{}},
			}},
			Yes: valueWhenUndefined,
			No:  result,
		}}
	}
	if exprWrapFunc != nil {
		result = exprWrapFunc(result)
	}
	if targetWrapFunc != nil {
		result = targetWrapFunc(result)
	}
	return result, exprOut{}
}

func (p *parser) lowerAssignmentOperator(value ast.Expr, callback func(ast.Expr, ast.Expr) ast.Expr) ast.Expr {
	switch left := value.Data.(type) {
	case *ast.EDot:
		if left.OptionalChain == ast.OptionalChainNone {
			referenceFunc, wrapFunc := p.captureValueWithPossibleSideEffects(value.Loc, 2, left.Target)
			return wrapFunc(callback(
				ast.Expr{Loc: value.Loc, Data: &ast.EDot{
					Target:  referenceFunc(),
					Name:    left.Name,
					NameLoc: left.NameLoc,
				}},
				ast.Expr{Loc: value.Loc, Data: &ast.EDot{
					Target:  referenceFunc(),
					Name:    left.Name,
					NameLoc: left.NameLoc,
				}},
			))
		}

	case *ast.EIndex:
		if left.OptionalChain == ast.OptionalChainNone {
			targetFunc, targetWrapFunc := p.captureValueWithPossibleSideEffects(value.Loc, 2, left.Target)
			indexFunc, indexWrapFunc := p.captureValueWithPossibleSideEffects(value.Loc, 2, left.Index)
			return targetWrapFunc(indexWrapFunc(callback(
				ast.Expr{Loc: value.Loc, Data: &ast.EIndex{
					Target: targetFunc(),
					Index:  indexFunc(),
				}},
				ast.Expr{Loc: value.Loc, Data: &ast.EIndex{
					Target: targetFunc(),
					Index:  indexFunc(),
				}},
			)))
		}

	case *ast.EIdentifier:
		return callback(
			ast.Expr{Loc: value.Loc, Data: &ast.EIdentifier{Ref: left.Ref}},
			value,
		)
	}

	// We shouldn't get here with valid syntax? Just let this through for now
	// since there's currently no assignment target validation. Garbage in,
	// garbage out.
	return value
}

func (p *parser) lowerExponentiationAssignmentOperator(loc ast.Loc, e *ast.EBinary) ast.Expr {
	if target, privateLoc, private := p.extractPrivateIndex(e.Left); private != nil {
		// "a.#b **= c" => "__privateSet(a, #b, __pow(__privateGet(a, #b), c))"
		targetFunc, targetWrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, target)
		return targetWrapFunc(p.lowerPrivateSet(targetFunc(), privateLoc, private,
			p.callRuntime(loc, "__pow", []ast.Expr{
				p.lowerPrivateGet(targetFunc(), privateLoc, private),
				e.Right,
			})))
	}

	return p.lowerAssignmentOperator(e.Left, func(a ast.Expr, b ast.Expr) ast.Expr {
		// "a **= b" => "a = __pow(a, b)"
		return ast.Assign(a, p.callRuntime(loc, "__pow", []ast.Expr{b, e.Right}))
	})
}

func (p *parser) lowerNullishCoalescingAssignmentOperator(loc ast.Loc, e *ast.EBinary) ast.Expr {
	if target, privateLoc, private := p.extractPrivateIndex(e.Left); private != nil {
		if p.UnsupportedFeatures.Has(compat.NullishCoalescing) {
			// "a.#b ??= c" => "(_a = __privateGet(a, #b)) != null ? _a : __privateSet(a, #b, c)"
			targetFunc, targetWrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, target)
			left := p.lowerPrivateGet(targetFunc(), privateLoc, private)
			right := p.lowerPrivateSet(targetFunc(), privateLoc, private, e.Right)
			return targetWrapFunc(p.lowerNullishCoalescing(loc, left, right))
		}

		// "a.#b ??= c" => "__privateGet(a, #b) ?? __privateSet(a, #b, c)"
		targetFunc, targetWrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, target)
		return targetWrapFunc(ast.Expr{Loc: loc, Data: &ast.EBinary{
			Op:    ast.BinOpNullishCoalescing,
			Left:  p.lowerPrivateGet(targetFunc(), privateLoc, private),
			Right: p.lowerPrivateSet(targetFunc(), privateLoc, private, e.Right),
		}})
	}

	return p.lowerAssignmentOperator(e.Left, func(a ast.Expr, b ast.Expr) ast.Expr {
		if p.UnsupportedFeatures.Has(compat.NullishCoalescing) {
			// "a ??= b" => "(_a = a) != null ? _a : a = b"
			return p.lowerNullishCoalescing(loc, a, ast.Assign(b, e.Right))
		}

		// "a ??= b" => "a ?? (a = b)"
		return ast.Expr{Loc: loc, Data: &ast.EBinary{
			Op:    ast.BinOpNullishCoalescing,
			Left:  a,
			Right: ast.Assign(b, e.Right),
		}}
	})
}

func (p *parser) lowerLogicalAssignmentOperator(loc ast.Loc, e *ast.EBinary, op ast.OpCode) ast.Expr {
	if target, privateLoc, private := p.extractPrivateIndex(e.Left); private != nil {
		// "a.#b &&= c" => "__privateGet(a, #b) && __privateSet(a, #b, c)"
		// "a.#b ||= c" => "__privateGet(a, #b) || __privateSet(a, #b, c)"
		targetFunc, targetWrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, target)
		return targetWrapFunc(ast.Expr{Loc: loc, Data: &ast.EBinary{
			Op:    op,
			Left:  p.lowerPrivateGet(targetFunc(), privateLoc, private),
			Right: p.lowerPrivateSet(targetFunc(), privateLoc, private, e.Right),
		}})
	}

	return p.lowerAssignmentOperator(e.Left, func(a ast.Expr, b ast.Expr) ast.Expr {
		// "a &&= b" => "a && (a = b)"
		// "a ||= b" => "a || (a = b)"
		return ast.Expr{Loc: loc, Data: &ast.EBinary{
			Op:    op,
			Left:  a,
			Right: ast.Assign(b, e.Right),
		}}
	})
}

func (p *parser) lowerNullishCoalescing(loc ast.Loc, left ast.Expr, right ast.Expr) ast.Expr {
	if p.Strict.NullishCoalescing {
		// "x ?? y" => "x !== null && x !== void 0 ? x : y"
		// "x() ?? y()" => "_a = x(), _a !== null && _a !== void 0 ? _a : y"
		leftFunc, wrapFunc := p.captureValueWithPossibleSideEffects(loc, 3, left)
		return wrapFunc(ast.Expr{Loc: loc, Data: &ast.EIf{
			Test: ast.Expr{Loc: loc, Data: &ast.EBinary{
				Op: ast.BinOpLogicalAnd,
				Left: ast.Expr{Loc: loc, Data: &ast.EBinary{
					Op:    ast.BinOpStrictNe,
					Left:  leftFunc(),
					Right: ast.Expr{Loc: loc, Data: &ast.ENull{}},
				}},
				Right: ast.Expr{Loc: loc, Data: &ast.EBinary{
					Op:    ast.BinOpStrictNe,
					Left:  leftFunc(),
					Right: ast.Expr{Loc: loc, Data: &ast.EUndefined{}},
				}},
			}},
			Yes: leftFunc(),
			No:  right,
		}})
	}

	// "x ?? y" => "x != null ? x : y"
	// "x() ?? y()" => "_a = x(), _a != null ? _a : y"
	leftFunc, wrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, left)
	return wrapFunc(ast.Expr{Loc: loc, Data: &ast.EIf{
		Test: ast.Expr{Loc: loc, Data: &ast.EBinary{
			Op:    ast.BinOpLooseNe,
			Left:  leftFunc(),
			Right: ast.Expr{Loc: loc, Data: &ast.ENull{}},
		}},
		Yes: leftFunc(),
		No:  right,
	}})
}

// Lower object spread for environments that don't support them. Non-spread
// properties are grouped into object literals and then passed to __assign()
// like this (__assign() is an alias for Object.assign()):
//
//   "{a, b, ...c, d, e}" => "__assign(__assign(__assign({a, b}, c), {d, e})"
//
// If the object literal starts with a spread, then we pass an empty object
// literal to __assign() to make sure we clone the object:
//
//   "{...a, b}" => "__assign(__assign({}, a), {b})"
//
// It's not immediately obvious why we don't compile everything to a single
// call to __assign(). After all, Object.assign() can take any number of
// arguments. The reason is to preserve the order of side effects. Consider
// this code:
//
//   let a = {get x() { b = {y: 2}; return 1 }}
//   let b = {}
//   let c = {...a, ...b}
//
// Converting the above code to "let c = __assign({}, a, b)" means "c" becomes
// "{x: 1}" which is incorrect. Converting the above code instead to
// "let c = __assign(__assign({}, a), b)" means "c" becomes "{x: 1, y: 2}"
// which is correct.
func (p *parser) lowerObjectSpread(loc ast.Loc, e *ast.EObject) ast.Expr {
	needsLowering := false

	if p.UnsupportedFeatures.Has(compat.ObjectRestSpread) {
		for _, property := range e.Properties {
			if property.Kind == ast.PropertySpread {
				needsLowering = true
				break
			}
		}
	}

	if !needsLowering {
		return ast.Expr{Loc: loc, Data: e}
	}

	var result ast.Expr
	properties := []ast.Property{}

	for _, property := range e.Properties {
		if property.Kind != ast.PropertySpread {
			properties = append(properties, property)
			continue
		}

		if len(properties) > 0 || result.Data == nil {
			if result.Data == nil {
				// "{a, ...b}" => "__assign({a}, b)"
				result = ast.Expr{Loc: loc, Data: &ast.EObject{
					Properties:   properties,
					IsSingleLine: e.IsSingleLine,
				}}
			} else {
				// "{...a, b, ...c}" => "__assign(__assign(__assign({}, a), {b}), c)"
				result = p.callRuntime(loc, "__assign",
					[]ast.Expr{result, {Loc: loc, Data: &ast.EObject{
						Properties:   properties,
						IsSingleLine: e.IsSingleLine,
					}}})
			}
			properties = []ast.Property{}
		}

		// "{a, ...b}" => "__assign({a}, b)"
		result = p.callRuntime(loc, "__assign", []ast.Expr{result, *property.Value})
	}

	if len(properties) > 0 {
		// "{...a, b}" => "__assign(__assign({}, a), {b})"
		result = p.callRuntime(loc, "__assign", []ast.Expr{result, {Loc: loc, Data: &ast.EObject{
			Properties:   properties,
			IsSingleLine: e.IsSingleLine,
		}}})
	}

	return result
}

func (p *parser) lowerPrivateGet(target ast.Expr, loc ast.Loc, private *ast.EPrivateIdentifier) ast.Expr {
	switch p.symbols[private.Ref.InnerIndex].Kind {
	case ast.SymbolPrivateMethod, ast.SymbolPrivateStaticMethod:
		// "this.#method" => "__privateMethod(this, #method, method_fn)"
		fnRef := p.privateGetters[private.Ref]
		p.recordUsage(fnRef)
		return p.callRuntime(target.Loc, "__privateMethod", []ast.Expr{
			target,
			{Loc: loc, Data: &ast.EIdentifier{Ref: private.Ref}},
			{Loc: loc, Data: &ast.EIdentifier{Ref: fnRef}},
		})

	case ast.SymbolPrivateGet, ast.SymbolPrivateStaticGet,
		ast.SymbolPrivateGetSetPair, ast.SymbolPrivateStaticGetSetPair:
		// "this.#getter" => "__privateGet(this, #getter, getter_get)"
		fnRef := p.privateGetters[private.Ref]
		p.recordUsage(fnRef)
		return p.callRuntime(target.Loc, "__privateGet", []ast.Expr{
			target,
			{Loc: loc, Data: &ast.EIdentifier{Ref: private.Ref}},
			{Loc: loc, Data: &ast.EIdentifier{Ref: fnRef}},
		})

	default:
		// "this.#field" => "__privateGet(this, #field)"
		return p.callRuntime(target.Loc, "__privateGet", []ast.Expr{
			target,
			{Loc: loc, Data: &ast.EIdentifier{Ref: private.Ref}},
		})
	}
}

func (p *parser) lowerPrivateSet(
	target ast.Expr,
	loc ast.Loc,
	private *ast.EPrivateIdentifier,
	value ast.Expr,
) ast.Expr {
	switch p.symbols[private.Ref.InnerIndex].Kind {
	case ast.SymbolPrivateSet, ast.SymbolPrivateStaticSet,
		ast.SymbolPrivateGetSetPair, ast.SymbolPrivateStaticGetSetPair:
		// "this.#setter = 123" => "__privateSet(this, #setter, 123, setter_set)"
		fnRef := p.privateSetters[private.Ref]
		p.recordUsage(fnRef)
		return p.callRuntime(target.Loc, "__privateSet", []ast.Expr{
			target,
			{Loc: loc, Data: &ast.EIdentifier{Ref: private.Ref}},
			value,
			{Loc: loc, Data: &ast.EIdentifier{Ref: fnRef}},
		})

	default:
		// "this.#field = 123" => "__privateSet(this, #field, 123)"
		return p.callRuntime(target.Loc, "__privateSet", []ast.Expr{
			target,
			{Loc: loc, Data: &ast.EIdentifier{Ref: private.Ref}},
			value,
		})
	}
}

func (p *parser) lowerPrivateSetUnOp(target ast.Expr, loc ast.Loc, private *ast.EPrivateIdentifier, op ast.OpCode, isSuffix bool) ast.Expr {
	targetFunc, targetWrapFunc := p.captureValueWithPossibleSideEffects(target.Loc, 2, target)
	target = targetFunc()

	// Load the private field and then use the unary "+" operator to force it to
	// be a number. Otherwise the binary "+" operator may cause string
	// concatenation instead of addition if one of the operands is not a number.
	value := ast.Expr{Loc: target.Loc, Data: &ast.EUnary{
		Op:    ast.UnOpPos,
		Value: p.lowerPrivateGet(targetFunc(), loc, private),
	}}

	if isSuffix {
		// "target.#private++" => "__privateSet(target, #private, _a = +__privateGet(target, #private) + 1), _a"
		valueFunc, valueWrapFunc := p.captureValueWithPossibleSideEffects(value.Loc, 2, value)
		assign := valueWrapFunc(targetWrapFunc(p.lowerPrivateSet(target, loc, private, ast.Expr{Loc: target.Loc, Data: &ast.EBinary{
			Op:    op,
			Left:  valueFunc(),
			Right: ast.Expr{Loc: target.Loc, Data: &ast.ENumber{Value: 1}},
		}})))
		return ast.JoinWithComma(assign, valueFunc())
	}

	// "++target.#private" => "__privateSet(target, #private, +__privateGet(target, #private) + 1)"
	return targetWrapFunc(p.lowerPrivateSet(target, loc, private, ast.Expr{Loc: target.Loc, Data: &ast.EBinary{
		Op:    op,
		Left:  value,
		Right: ast.Expr{Loc: target.Loc, Data: &ast.ENumber{Value: 1}},
	}}))
}

func (p *parser) lowerPrivateSetBinOp(target ast.Expr, loc ast.Loc, private *ast.EPrivateIdentifier, op ast.OpCode, value ast.Expr) ast.Expr {
	// "target.#private += 123" => "__privateSet(target, #private, __privateGet(target, #private) + 123)"
	targetFunc, targetWrapFunc := p.captureValueWithPossibleSideEffects(target.Loc, 2, target)
	return targetWrapFunc(p.lowerPrivateSet(targetFunc(), loc, private, ast.Expr{Loc: value.Loc, Data: &ast.EBinary{
		Op:    op,
		Left:  p.lowerPrivateGet(targetFunc(), loc, private),
		Right: value,
	}}))
}

// Returns valid data if target is an expression of the form "foo.#bar" and if
// the language target is such that private members must be lowered
func (p *parser) extractPrivateIndex(target ast.Expr) (ast.Expr, ast.Loc, *ast.EPrivateIdentifier) {
	if index, ok := target.Data.(*ast.EIndex); ok {
		if private, ok := index.Index.Data.(*ast.EPrivateIdentifier); ok && p.isPrivateUnsupported(private) {
			return index.Target, index.Index.Loc, private
		}
	}
	return ast.Expr{}, ast.Loc{}, nil
}

func bindingHasObjectRest(binding ast.Binding) bool {
	switch b := binding.Data.(type) {
	case *ast.BArray:
		for _, item := range b.Items {
			if bindingHasObjectRest(item.Binding) {
				return true
			}
		}
	case *ast.BObject:
		for _, property := range b.Properties {
			if property.IsSpread || bindingHasObjectRest(property.Value) {
				return true
			}
		}
	}
	return false
}

func exprHasObjectRest(expr ast.Expr) bool {
	switch e := expr.Data.(type) {
	case *ast.EBinary:
		if e.Op == ast.BinOpAssign && exprHasObjectRest(e.Left) {
			return true
		}
	case *ast.EArray:
		for _, item := range e.Items {
			if exprHasObjectRest(item) {
				return true
			}
		}
	case *ast.EObject:
		for _, property := range e.Properties {
			if property.Kind == ast.PropertySpread || exprHasObjectRest(*property.Value) {
				return true
			}
		}
	}
	return false
}

func (p *parser) lowerObjectRestInDecls(decls []ast.Decl) []ast.Decl {
	if !p.UnsupportedFeatures.Has(compat.ObjectRestSpread) {
		return decls
	}

	// Don't do any allocations if there are no object rest patterns. We want as
	// little overhead as possible in the common case.
	for i, decl := range decls {
		if decl.Value != nil && bindingHasObjectRest(decl.Binding) {
			clone := append([]ast.Decl{}, decls[:i]...)
			for _, decl := range decls[i:] {
				if decl.Value != nil {
					target := p.convertBindingToExpr(decl.Binding, nil)
					if result, ok := p.lowerObjectRestToDecls(target, *decl.Value, clone); ok {
						clone = result
						continue
					}
				}
				clone = append(clone, decl)
			}

			return clone
		}
	}

	return decls
}

func (p *parser) lowerObjectRestInForLoopInit(init ast.Stmt, body *ast.Stmt) {
	if !p.UnsupportedFeatures.Has(compat.ObjectRestSpread) {
		return
	}

	var bodyPrefixStmt ast.Stmt

	switch s := init.Data.(type) {
	case *ast.SExpr:
		// "for ({...x} in y) {}"
		// "for ({...x} of y) {}"
		if exprHasObjectRest(s.Value) {
			ref := p.generateTempRef(tempRefNeedsDeclare, "")
			if expr, ok := p.lowerObjectRestInAssign(s.Value, ast.Expr{Loc: init.Loc, Data: &ast.EIdentifier{Ref: ref}}); ok {
				s.Value.Data = &ast.EIdentifier{Ref: ref}
				bodyPrefixStmt = ast.Stmt{Loc: expr.Loc, Data: &ast.SExpr{Value: expr}}
			}
		}

	case *ast.SLocal:
		// "for (let {...x} in y) {}"
		// "for (let {...x} of y) {}"
		if len(s.Decls) == 1 && bindingHasObjectRest(s.Decls[0].Binding) {
			ref := p.generateTempRef(tempRefNoDeclare, "")
			decl := ast.Decl{Binding: s.Decls[0].Binding, Value: &ast.Expr{Loc: init.Loc, Data: &ast.EIdentifier{Ref: ref}}}
			p.recordUsage(ref)
			decls := p.lowerObjectRestInDecls([]ast.Decl{decl})
			s.Decls[0].Binding.Data = &ast.BIdentifier{Ref: ref}
			bodyPrefixStmt = ast.Stmt{Loc: init.Loc, Data: &ast.SLocal{Kind: s.Kind, Decls: decls}}
		}
	}

	if bodyPrefixStmt.Data != nil {
		if block, ok := body.Data.(*ast.SBlock); ok {
			// If there's already a block, insert at the front
			stmts := make([]ast.Stmt, 0, 1+len(block.Stmts))
			block.Stmts = append(append(stmts, bodyPrefixStmt), block.Stmts...)
		} else {
			// Otherwise, make a block and insert at the front
			body.Data = &ast.SBlock{Stmts: []ast.Stmt{bodyPrefixStmt, *body}}
		}
	}
}

func (p *parser) lowerObjectRestInCatchBinding(catch *ast.Catch) {
	if !p.UnsupportedFeatures.Has(compat.ObjectRestSpread) {
		return
	}

	if catch.Binding != nil && bindingHasObjectRest(*catch.Binding) {
		ref := p.generateTempRef(tempRefNoDeclare, "")
		decl := ast.Decl{Binding: *catch.Binding, Value: &ast.Expr{Loc: catch.Binding.Loc, Data: &ast.EIdentifier{Ref: ref}}}
		p.recordUsage(ref)
		decls := p.lowerObjectRestInDecls([]ast.Decl{decl})
		catch.Binding.Data = &ast.BIdentifier{Ref: ref}
		stmts := make([]ast.Stmt, 0, 1+len(catch.Body))
		stmts = append(stmts, ast.Stmt{Loc: catch.Binding.Loc, Data: &ast.SLocal{Kind: ast.LocalLet, Decls: decls}})
		catch.Body = append(stmts, catch.Body...)
	}
}

func (p *parser) lowerObjectRestInAssign(rootExpr ast.Expr, rootInit ast.Expr) (ast.Expr, bool) {
	var expr ast.Expr

	assign := func(left ast.Expr, right ast.Expr) {
		expr = maybeJoinWithComma(expr, ast.Assign(left, right))
	}

	if p.lowerObjectRestHelper(rootExpr, rootInit, assign, tempRefNeedsDeclare) {
		return expr, true
	}

	return ast.Expr{}, false
}

func (p *parser) lowerObjectRestToDecls(rootExpr ast.Expr, rootInit ast.Expr, decls []ast.Decl) ([]ast.Decl, bool) {
	assign := func(left ast.Expr, right ast.Expr) {
		binding, log := p.convertExprToBinding(left, nil)
		if len(log) > 0 {
			panic("Internal error")
		}
		decls = append(decls, ast.Decl{Binding: binding, Value: &right})
	}

	if p.lowerObjectRestHelper(rootExpr, rootInit, assign, tempRefNoDeclare) {
		return decls, true
	}

	return nil, false
}

func (p *parser) lowerObjectRestHelper(
	rootExpr ast.Expr,
	rootInit ast.Expr,
	assign func(ast.Expr, ast.Expr),
	declare generateTempRefArg,
) bool {
	if !p.UnsupportedFeatures.Has(compat.ObjectRestSpread) {
		return false
	}

	// Check if this could possibly contain an object rest binding
	switch rootExpr.Data.(type) {
	case *ast.EArray, *ast.EObject:
	default:
		return false
	}

	// Scan for object rest bindings and initalize rest binding containment
	containsRestBinding := make(map[ast.E]bool)
	var findRestBindings func(ast.Expr) bool
	findRestBindings = func(expr ast.Expr) bool {
		found := false
		switch e := expr.Data.(type) {
		case *ast.EBinary:
			if e.Op == ast.BinOpAssign && findRestBindings(e.Left) {
				found = true
			}
		case *ast.EArray:
			for _, item := range e.Items {
				if findRestBindings(item) {
					found = true
				}
			}
		case *ast.EObject:
			for _, property := range e.Properties {
				if property.Kind == ast.PropertySpread || findRestBindings(*property.Value) {
					found = true
				}
			}
		}
		if found {
			containsRestBinding[expr.Data] = true
		}
		return found
	}
	findRestBindings(rootExpr)
	if len(containsRestBinding) == 0 {
		return false
	}

	// If there is at least one rest binding, lower the whole expression
	var visit func(ast.Expr, ast.Expr, []func() ast.Expr)

	captureIntoRef := func(expr ast.Expr) ast.Ref {
		if id, ok := expr.Data.(*ast.EIdentifier); ok {
			return id.Ref
		}

		// If the initializer isn't already a bare identifier that we can
		// reference, store the initializer first so we can reference it later.
		// The initializer may have side effects so we must evaluate it once.
		ref := p.generateTempRef(declare, "")
		assign(ast.Expr{Loc: expr.Loc, Data: &ast.EIdentifier{Ref: ref}}, expr)
		p.recordUsage(ref)
		return ref
	}

	lowerObjectRestPattern := func(
		before []ast.Property,
		binding ast.Expr,
		init ast.Expr,
		capturedKeys []func() ast.Expr,
		isSingleLine bool,
	) {
		// If there are properties before this one, store the initializer in a
		// temporary so we can reference it multiple times, then create a new
		// destructuring assignment for these properties
		if len(before) > 0 {
			// "let {a, ...b} = c"
			ref := captureIntoRef(init)
			assign(ast.Expr{Loc: before[0].Key.Loc, Data: &ast.EObject{Properties: before, IsSingleLine: isSingleLine}},
				ast.Expr{Loc: init.Loc, Data: &ast.EIdentifier{Ref: ref}})
			init = ast.Expr{Loc: init.Loc, Data: &ast.EIdentifier{Ref: ref}}
			p.recordUsage(ref)
			p.recordUsage(ref)
		}

		// Call "__rest" to clone the initializer without the keys for previous
		// properties, then assign the result to the binding for the rest pattern
		keysToExclude := make([]ast.Expr, len(capturedKeys))
		for i, capturedKey := range capturedKeys {
			keysToExclude[i] = capturedKey()
		}
		assign(binding, p.callRuntime(binding.Loc, "__rest", []ast.Expr{init,
			{Loc: binding.Loc, Data: &ast.EArray{Items: keysToExclude, IsSingleLine: isSingleLine}}}))
	}

	splitArrayPattern := func(
		before []ast.Expr,
		split ast.Expr,
		after []ast.Expr,
		init ast.Expr,
		isSingleLine bool,
	) {
		// If this has a default value, skip the value to target the binding
		binding := &split
		if binary, ok := binding.Data.(*ast.EBinary); ok && binary.Op == ast.BinOpAssign {
			binding = &binary.Left
		}

		// Swap the binding with a temporary
		splitRef := p.generateTempRef(declare, "")
		deferredBinding := *binding
		binding.Data = &ast.EIdentifier{Ref: splitRef}
		items := append(before, split)

		// If there are any items left over, defer them until later too
		var tailExpr ast.Expr
		var tailInit ast.Expr
		if len(after) > 0 {
			tailRef := p.generateTempRef(declare, "")
			loc := after[0].Loc
			tailExpr = ast.Expr{Loc: loc, Data: &ast.EArray{Items: after, IsSingleLine: isSingleLine}}
			tailInit = ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: tailRef}}
			items = append(items, ast.Expr{Loc: loc, Data: &ast.ESpread{Value: ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: tailRef}}}})
			p.recordUsage(tailRef)
			p.recordUsage(tailRef)
		}

		// The original destructuring assignment must come first
		assign(ast.Expr{Loc: split.Loc, Data: &ast.EArray{Items: items, IsSingleLine: isSingleLine}}, init)

		// Then the deferred split is evaluated
		visit(deferredBinding, ast.Expr{Loc: split.Loc, Data: &ast.EIdentifier{Ref: splitRef}}, nil)
		p.recordUsage(splitRef)

		// Then anything after the split
		if len(after) > 0 {
			visit(tailExpr, tailInit, nil)
		}
	}

	splitObjectPattern := func(
		upToSplit []ast.Property,
		afterSplit []ast.Property,
		init ast.Expr,
		capturedKeys []func() ast.Expr,
		isSingleLine bool,
	) {
		// If there are properties after the split, store the initializer in a
		// temporary so we can reference it multiple times
		var afterSplitInit ast.Expr
		if len(afterSplit) > 0 {
			ref := captureIntoRef(init)
			init = ast.Expr{Loc: init.Loc, Data: &ast.EIdentifier{Ref: ref}}
			afterSplitInit = ast.Expr{Loc: init.Loc, Data: &ast.EIdentifier{Ref: ref}}
		}

		split := &upToSplit[len(upToSplit)-1]
		binding := split.Value

		// Swap the binding with a temporary
		splitRef := p.generateTempRef(declare, "")
		deferredBinding := *binding
		binding.Data = &ast.EIdentifier{Ref: splitRef}
		p.recordUsage(splitRef)

		// Use a destructuring assignment to unpack everything up to and including
		// the split point
		assign(ast.Expr{Loc: binding.Loc, Data: &ast.EObject{Properties: upToSplit, IsSingleLine: isSingleLine}}, init)

		// Handle any nested rest binding patterns inside the split point
		visit(deferredBinding, ast.Expr{Loc: binding.Loc, Data: &ast.EIdentifier{Ref: splitRef}}, nil)
		p.recordUsage(splitRef)

		// Then continue on to any properties after the split
		if len(afterSplit) > 0 {
			visit(ast.Expr{Loc: binding.Loc, Data: &ast.EObject{
				Properties:   afterSplit,
				IsSingleLine: isSingleLine,
			}}, afterSplitInit, capturedKeys)
		}
	}

	// This takes an expression representing a binding pattern as input and
	// returns that binding pattern with any object rest patterns stripped out.
	// The object rest patterns are lowered and appended to "exprChain" along
	// with any child binding patterns that came after the binding pattern
	// containing the object rest pattern.
	//
	// This transform must be very careful to preserve the exact evaluation
	// order of all assignments, default values, and computed property keys.
	//
	// Unlike the Babel and TypeScript compilers, this transform does not
	// lower binding patterns other than object rest patterns. For example,
	// array spread patterns are preserved.
	//
	// Certain patterns such as "{a: {...a}, b: {...b}, ...c}" may need to be
	// split multiple times. In this case the "capturedKeys" argument allows
	// the visitor to pass on captured keys to the tail-recursive call that
	// handles the properties after the split.
	visit = func(expr ast.Expr, init ast.Expr, capturedKeys []func() ast.Expr) {
		switch e := expr.Data.(type) {
		case *ast.EArray:
			// Split on the first binding with a nested rest binding pattern
			for i, item := range e.Items {
				// "let [a, {...b}, c] = d"
				if containsRestBinding[item.Data] {
					splitArrayPattern(e.Items[:i], item, append([]ast.Expr{}, e.Items[i+1:]...), init, e.IsSingleLine)
					return
				}
			}

		case *ast.EObject:
			last := len(e.Properties) - 1
			endsWithRestBinding := last >= 0 && e.Properties[last].Kind == ast.PropertySpread

			// Split on the first binding with a nested rest binding pattern
			for i := range e.Properties {
				property := &e.Properties[i]

				// "let {a, ...b} = c"
				if property.Kind == ast.PropertySpread {
					lowerObjectRestPattern(e.Properties[:i], *property.Value, init, capturedKeys, e.IsSingleLine)
					return
				}

				// Save a copy of this key so the rest binding can exclude it
				if endsWithRestBinding {
					key, capturedKey := p.captureKeyForObjectRest(property.Key)
					property.Key = key
					capturedKeys = append(capturedKeys, capturedKey)
				}

				// "let {a: {...b}, c} = d"
				if containsRestBinding[property.Value.Data] {
					splitObjectPattern(e.Properties[:i+1], e.Properties[i+1:], init, capturedKeys, e.IsSingleLine)
					return
				}
			}
		}

		assign(expr, init)
	}

	visit(rootExpr, rootInit, nil)
	return true
}

// Save a copy of the key for the call to "__rest" later on. Certain
// expressions can be converted to keys more efficiently than others.
func (p *parser) captureKeyForObjectRest(originalKey ast.Expr) (finalKey ast.Expr, capturedKey func() ast.Expr) {
	loc := originalKey.Loc
	finalKey = originalKey

	switch k := originalKey.Data.(type) {
	case *ast.EString:
		capturedKey = func() ast.Expr { return ast.Expr{Loc: loc, Data: &ast.EString{Value: k.Value}} }

	case *ast.ENumber:
		// Emit it as the number plus a string (i.e. call toString() on it).
		// It's important to do it this way instead of trying to print the
		// float as a string because Go's floating-point printer doesn't
		// behave exactly the same as JavaScript and if they are different,
		// the generated code will be wrong.
		capturedKey = func() ast.Expr {
			return ast.Expr{Loc: loc, Data: &ast.EBinary{
				Op:    ast.BinOpAdd,
				Left:  ast.Expr{Loc: loc, Data: &ast.ENumber{Value: k.Value}},
				Right: ast.Expr{Loc: loc, Data: &ast.EString{}},
			}}
		}

	case *ast.EIdentifier:
		capturedKey = func() ast.Expr {
			p.recordUsage(k.Ref)
			return p.callRuntime(loc, "__restKey", []ast.Expr{{Loc: loc, Data: &ast.EIdentifier{Ref: k.Ref}}})
		}

	default:
		// If it's an arbitrary expression, it probably has a side effect.
		// Stash it in a temporary reference so we don't evaluate it twice.
		tempRef := p.generateTempRef(tempRefNeedsDeclare, "")
		finalKey = ast.Assign(ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: tempRef}}, originalKey)
		capturedKey = func() ast.Expr {
			p.recordUsage(tempRef)
			return p.callRuntime(loc, "__restKey", []ast.Expr{{Loc: loc, Data: &ast.EIdentifier{Ref: tempRef}}})
		}
	}

	return
}

// Lower class fields for environments that don't support them. This either
// takes a statement or an expression.
func (p *parser) lowerClass(stmt ast.Stmt, expr ast.Expr) ([]ast.Stmt, ast.Expr) {
	type classKind uint8
	const (
		classKindExpr classKind = iota
		classKindStmt
		classKindExportStmt
		classKindExportDefaultStmt
	)

	// Unpack the class from the statement or expression
	var kind classKind
	var class *ast.Class
	var classLoc ast.Loc
	var defaultName ast.LocRef
	if stmt.Data == nil {
		e, _ := expr.Data.(*ast.EClass)
		class = &e.Class
		kind = classKindExpr
	} else if s, ok := stmt.Data.(*ast.SClass); ok {
		class = &s.Class
		if s.IsExport {
			kind = classKindExportStmt
		} else {
			kind = classKindStmt
		}
	} else {
		s, _ := stmt.Data.(*ast.SExportDefault)
		s2, _ := s.Value.Stmt.Data.(*ast.SClass)
		class = &s2.Class
		defaultName = s.DefaultName
		kind = classKindExportDefaultStmt
	}

	// We always lower class fields when parsing TypeScript since class fields in
	// TypeScript don't follow the JavaScript spec. We also need to always lower
	// TypeScript-style decorators since they don't have a JavaScript equivalent.
	classFeatures := compat.ClassField | compat.ClassStaticField |
		compat.ClassPrivateField | compat.ClassPrivateStaticField |
		compat.ClassPrivateMethod | compat.ClassPrivateStaticMethod |
		compat.ClassPrivateAccessor | compat.ClassPrivateStaticAccessor
	if !p.TS.Parse && !p.UnsupportedFeatures.Has(classFeatures) {
		if kind == classKindExpr {
			return nil, expr
		} else {
			return []ast.Stmt{stmt}, ast.Expr{}
		}
	}

	var ctor *ast.EFunction
	var parameterFields []ast.Stmt
	var instanceMembers []ast.Stmt
	end := 0

	// These expressions are generated after the class body, in this order
	var computedPropertyCache ast.Expr
	var privateMembers []ast.Expr
	var staticMembers []ast.Expr
	var instanceDecorators []ast.Expr
	var staticDecorators []ast.Expr

	// These are only for class expressions that need to be captured
	var nameFunc func() ast.Expr
	var wrapFunc func(ast.Expr) ast.Expr
	didCaptureClassExpr := false

	// Class statements can be missing a name if they are in an
	// "export default" statement:
	//
	//   export default class {
	//     static foo = 123
	//   }
	//
	nameFunc = func() ast.Expr {
		if kind == classKindExpr {
			// If this is a class expression, capture and store it. We have to
			// do this even if it has a name since the name isn't exposed
			// outside the class body.
			classExpr := &ast.EClass{Class: *class}
			class = &classExpr.Class
			nameFunc, wrapFunc = p.captureValueWithPossibleSideEffects(classLoc, 2, ast.Expr{Loc: classLoc, Data: classExpr})
			expr = nameFunc()
			didCaptureClassExpr = true
			name := nameFunc()

			// If we're storing the class expression in a variable, remove the class
			// name and rewrite all references to the class name with references to
			// the temporary variable holding the class expression. This ensures that
			// references to the class expression by name in any expressions that end
			// up being pulled outside of the class body still work. For example:
			//
			//   let Bar = class Foo {
			//     static foo = 123
			//     static bar = Foo.foo
			//   }
			//
			// This might be converted into the following:
			//
			//   var _a;
			//   let Bar = (_a = class {
			//   }, _a.foo = 123, _a.bar = _a.foo, _a);
			//
			if class.Name != nil {
				p.symbols[class.Name.Ref.InnerIndex].Link = name.Data.(*ast.EIdentifier).Ref
				class.Name = nil
			}

			return name
		} else {
			if class.Name == nil {
				if kind == classKindExportDefaultStmt {
					class.Name = &defaultName
				} else {
					class.Name = &ast.LocRef{Loc: classLoc, Ref: p.generateTempRef(tempRefNoDeclare, "")}
				}
			}
			p.recordUsage(class.Name.Ref)
			return ast.Expr{Loc: classLoc, Data: &ast.EIdentifier{Ref: class.Name.Ref}}
		}
	}

	for _, prop := range class.Properties {
		// Merge parameter decorators with method decorators
		if p.TS.Parse && prop.IsMethod {
			if fn, ok := prop.Value.Data.(*ast.EFunction); ok {
				for i, arg := range fn.Fn.Args {
					for _, decorator := range arg.TSDecorators {
						// Generate a call to "__param()" for this parameter decorator
						prop.TSDecorators = append(prop.TSDecorators,
							p.callRuntime(decorator.Loc, "__param", []ast.Expr{
								{Loc: decorator.Loc, Data: &ast.ENumber{Value: float64(i)}},
								decorator,
							}),
						)
					}
				}
			}
		}

		// The TypeScript class field transform requires removing fields without
		// initializers. If the field is removed, then we only need the key for
		// its side effects and we don't need a temporary reference for the key.
		// However, the TypeScript compiler doesn't remove the field when doing
		// strict class field initialization, so we shouldn't either.
		private, _ := prop.Key.Data.(*ast.EPrivateIdentifier)
		mustLowerPrivate := private != nil && p.isPrivateUnsupported(private)
		shouldOmitFieldInitializer := p.TS.Parse && !prop.IsMethod && prop.Initializer == nil &&
			!p.Strict.ClassFields && !mustLowerPrivate

		// Class fields must be lowered if the environment doesn't support them
		mustLowerField := !prop.IsMethod && (!prop.IsStatic && p.UnsupportedFeatures.Has(compat.ClassField) ||
			(prop.IsStatic && p.UnsupportedFeatures.Has(compat.ClassStaticField)))

		// Make sure the order of computed property keys doesn't change. These
		// expressions have side effects and must be evaluated in order.
		keyExprNoSideEffects := prop.Key
		if prop.IsComputed && (p.TS.Parse || len(prop.TSDecorators) > 0 ||
			mustLowerField || computedPropertyCache.Data != nil) {
			needsKey := true
			if len(prop.TSDecorators) == 0 && (prop.IsMethod || shouldOmitFieldInitializer) {
				needsKey = false
			}

			if !needsKey {
				// Just evaluate the key for its side effects
				computedPropertyCache = maybeJoinWithComma(computedPropertyCache, prop.Key)
			} else {
				// Store the key in a temporary so we can assign to it later
				ref := p.generateTempRef(tempRefNeedsDeclare, "")
				computedPropertyCache = maybeJoinWithComma(computedPropertyCache,
					ast.Assign(ast.Expr{Loc: prop.Key.Loc, Data: &ast.EIdentifier{Ref: ref}}, prop.Key))
				prop.Key = ast.Expr{Loc: prop.Key.Loc, Data: &ast.EIdentifier{Ref: ref}}
				keyExprNoSideEffects = prop.Key
			}

			// If this is a computed method, the property value will be used
			// immediately. In this case we inline all computed properties so far to
			// make sure all computed properties before this one are evaluated first.
			if prop.IsMethod {
				prop.Key = computedPropertyCache
				computedPropertyCache = ast.Expr{}
			}
		}

		// Handle decorators
		if p.TS.Parse {
			// Generate a single call to "__decorate()" for this property
			if len(prop.TSDecorators) > 0 {
				loc := prop.Key.Loc

				// Clone the key for the property descriptor
				var descriptorKey ast.Expr
				switch k := keyExprNoSideEffects.Data.(type) {
				case *ast.ENumber:
					descriptorKey = ast.Expr{Loc: loc, Data: &ast.ENumber{Value: k.Value}}
				case *ast.EString:
					descriptorKey = ast.Expr{Loc: loc, Data: &ast.EString{Value: k.Value}}
				case *ast.EIdentifier:
					descriptorKey = ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: k.Ref}}
				default:
					panic("Internal error")
				}

				// This code tells "__decorate()" if the descriptor should be undefined
				descriptorKind := float64(1)
				if !prop.IsMethod {
					descriptorKind = 2
				}

				decorator := p.callRuntime(loc, "__decorate", []ast.Expr{
					{Loc: loc, Data: &ast.EArray{Items: prop.TSDecorators}},
					{Loc: loc, Data: &ast.EDot{
						Target:  nameFunc(),
						Name:    "prototype",
						NameLoc: loc,
					}},
					descriptorKey,
					{Loc: loc, Data: &ast.ENumber{Value: descriptorKind}},
				})

				// Static decorators are grouped after instance decorators
				if prop.IsStatic {
					staticDecorators = append(staticDecorators, decorator)
				} else {
					instanceDecorators = append(instanceDecorators, decorator)
				}
			}
		}

		// Instance and static fields are a JavaScript feature, not just a
		// TypeScript feature. Move their initializers from the class body to
		// either the constructor (instance fields) or after the class (static
		// fields).
		if !prop.IsMethod && (mustLowerField || (p.TS.Parse && !p.Strict.ClassFields && (!prop.IsStatic || private == nil))) {
			// The TypeScript compiler doesn't follow the JavaScript spec for
			// uninitialized fields. They are supposed to be set to undefined but the
			// TypeScript compiler just omits them entirely.
			if !shouldOmitFieldInitializer {
				loc := prop.Key.Loc

				// Determine where to store the field
				var target ast.Expr
				if prop.IsStatic {
					target = nameFunc()
				} else {
					target = ast.Expr{Loc: loc, Data: &ast.EThis{}}
				}

				// Generate the assignment initializer
				var init ast.Expr
				if prop.Initializer != nil {
					init = *prop.Initializer
				} else {
					init = ast.Expr{Loc: loc, Data: &ast.EUndefined{}}
				}

				// Generate the assignment target
				var expr ast.Expr
				if mustLowerPrivate {
					// Generate a new symbol for this private field
					ref := p.generateTempRef(tempRefNeedsDeclare, "_"+p.symbols[private.Ref.InnerIndex].OriginalName[1:])
					p.symbols[private.Ref.InnerIndex].Link = ref

					// Initialize the private field to a new WeakMap
					if p.weakMapRef == ast.InvalidRef {
						p.weakMapRef = p.newSymbol(ast.SymbolUnbound, "WeakMap")
						p.moduleScope.Generated = append(p.moduleScope.Generated, p.weakMapRef)
					}
					privateMembers = append(privateMembers, ast.Assign(
						ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: ref}},
						ast.Expr{Loc: loc, Data: &ast.ENew{Target: ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: p.weakMapRef}}}},
					))
					p.recordUsage(ref)

					// Add every newly-constructed instance into this map
					expr = ast.Expr{Loc: loc, Data: &ast.ECall{
						Target: ast.Expr{Loc: loc, Data: &ast.EDot{
							Target:  ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: ref}},
							Name:    "set",
							NameLoc: loc,
						}},
						Args: []ast.Expr{
							target,
							init,
						},
					}}
					p.recordUsage(ref)
				} else if private == nil && p.Strict.ClassFields {
					expr = p.callRuntime(loc, "__publicField", []ast.Expr{
						target,
						prop.Key,
						init,
					})
				} else {
					if key, ok := prop.Key.Data.(*ast.EString); ok && !prop.IsComputed {
						target = ast.Expr{Loc: loc, Data: &ast.EDot{
							Target:  target,
							Name:    lexer.UTF16ToString(key.Value),
							NameLoc: loc,
						}}
					} else {
						target = ast.Expr{Loc: loc, Data: &ast.EIndex{
							Target: target,
							Index:  prop.Key,
						}}
					}

					expr = ast.Assign(target, init)
				}

				if prop.IsStatic {
					// Move this property to an assignment after the class ends
					staticMembers = append(staticMembers, expr)
				} else {
					// Move this property to an assignment inside the class constructor
					instanceMembers = append(instanceMembers, ast.Stmt{Loc: loc, Data: &ast.SExpr{Value: expr}})
				}
			}

			if private == nil || mustLowerPrivate {
				// Remove the field from the class body
				continue
			}

			// Keep the private field but remove the initializer
			prop.Initializer = nil
		}

		// Remember where the constructor is for later
		if prop.IsMethod {
			if mustLowerPrivate {
				loc := prop.Key.Loc

				// Don't generate a symbol for a getter/setter pair twice
				if p.symbols[private.Ref.InnerIndex].Link == ast.InvalidRef {
					// Generate a new symbol for this private method
					ref := p.generateTempRef(tempRefNeedsDeclare, "_"+p.symbols[private.Ref.InnerIndex].OriginalName[1:])
					p.symbols[private.Ref.InnerIndex].Link = ref

					// Initialize the private method to a new WeakSet
					if p.weakSetRef == ast.InvalidRef {
						p.weakSetRef = p.newSymbol(ast.SymbolUnbound, "WeakSet")
						p.moduleScope.Generated = append(p.moduleScope.Generated, p.weakSetRef)
					}
					privateMembers = append(privateMembers, ast.Assign(
						ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: ref}},
						ast.Expr{Loc: loc, Data: &ast.ENew{Target: ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: p.weakSetRef}}}},
					))
					p.recordUsage(ref)

					// Determine where to store the private method
					var target ast.Expr
					if prop.IsStatic {
						target = nameFunc()
					} else {
						target = ast.Expr{Loc: loc, Data: &ast.EThis{}}
					}

					// Add every newly-constructed instance into this map
					expr := ast.Expr{Loc: loc, Data: &ast.ECall{
						Target: ast.Expr{Loc: loc, Data: &ast.EDot{
							Target:  ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: ref}},
							Name:    "add",
							NameLoc: loc,
						}},
						Args: []ast.Expr{
							target,
						},
					}}
					p.recordUsage(ref)

					if prop.IsStatic {
						// Move this property to an assignment after the class ends
						staticMembers = append(staticMembers, expr)
					} else {
						// Move this property to an assignment inside the class constructor
						instanceMembers = append(instanceMembers, ast.Stmt{Loc: loc, Data: &ast.SExpr{Value: expr}})
					}
				}

				// Move the method definition outside the class body
				methodRef := p.generateTempRef(tempRefNeedsDeclare, "_")
				if prop.Kind == ast.PropertySet {
					p.symbols[methodRef.InnerIndex].Link = p.privateSetters[private.Ref]
				} else {
					p.symbols[methodRef.InnerIndex].Link = p.privateGetters[private.Ref]
				}
				privateMembers = append(privateMembers, ast.Assign(
					ast.Expr{Loc: loc, Data: &ast.EIdentifier{Ref: methodRef}},
					*prop.Value,
				))
				continue
			} else if key, ok := prop.Key.Data.(*ast.EString); ok && lexer.UTF16EqualsString(key.Value, "constructor") {
				if fn, ok := prop.Value.Data.(*ast.EFunction); ok {
					ctor = fn

					// Initialize TypeScript constructor parameter fields
					if p.TS.Parse {
						for _, arg := range ctor.Fn.Args {
							if arg.IsTypeScriptCtorField {
								if id, ok := arg.Binding.Data.(*ast.BIdentifier); ok {
									parameterFields = append(parameterFields, ast.AssignStmt(
										ast.Expr{Loc: arg.Binding.Loc, Data: &ast.EDot{
											Target:  ast.Expr{Loc: arg.Binding.Loc, Data: &ast.EThis{}},
											Name:    p.symbols[id.Ref.InnerIndex].OriginalName,
											NameLoc: arg.Binding.Loc,
										}},
										ast.Expr{Loc: arg.Binding.Loc, Data: &ast.EIdentifier{Ref: id.Ref}},
									))
								}
							}
						}
					}
				}
			}
		}

		// Keep this property
		class.Properties[end] = prop
		end++
	}

	// Finish the filtering operation
	class.Properties = class.Properties[:end]

	// Insert instance field initializers into the constructor
	if len(instanceMembers) > 0 || len(parameterFields) > 0 {
		// Create a constructor if one doesn't already exist
		if ctor == nil {
			ctor = &ast.EFunction{}

			// Append it to the list to reuse existing allocation space
			class.Properties = append(class.Properties, ast.Property{
				IsMethod: true,
				Key:      ast.Expr{Loc: classLoc, Data: &ast.EString{Value: lexer.StringToUTF16("constructor")}},
				Value:    &ast.Expr{Loc: classLoc, Data: ctor},
			})

			// Make sure the constructor has a super() call if needed
			if class.Extends != nil {
				argumentsRef := p.newSymbol(ast.SymbolUnbound, "arguments")
				p.currentScope.Generated = append(p.currentScope.Generated, argumentsRef)
				ctor.Fn.Body.Stmts = append(ctor.Fn.Body.Stmts, ast.Stmt{Loc: classLoc, Data: &ast.SExpr{Value: ast.Expr{Loc: classLoc, Data: &ast.ECall{
					Target: ast.Expr{Loc: classLoc, Data: &ast.ESuper{}},
					Args:   []ast.Expr{{Loc: classLoc, Data: &ast.ESpread{Value: ast.Expr{Loc: classLoc, Data: &ast.EIdentifier{Ref: argumentsRef}}}}},
				}}}})
			}
		}

		// Insert the instance field initializers after the super call if there is one
		stmtsFrom := ctor.Fn.Body.Stmts
		stmtsTo := []ast.Stmt{}
		if len(stmtsFrom) > 0 && ast.IsSuperCall(stmtsFrom[0]) {
			stmtsTo = append(stmtsTo, stmtsFrom[0])
			stmtsFrom = stmtsFrom[1:]
		}
		stmtsTo = append(stmtsTo, parameterFields...)
		stmtsTo = append(stmtsTo, instanceMembers...)
		ctor.Fn.Body.Stmts = append(stmtsTo, stmtsFrom...)

		// Sort the constructor first to match the TypeScript compiler's output
		for i := 0; i < len(class.Properties); i++ {
			if class.Properties[i].Value != nil && class.Properties[i].Value.Data == ctor {
				ctorProp := class.Properties[i]
				for j := i; j > 0; j-- {
					class.Properties[j] = class.Properties[j-1]
				}
				class.Properties[0] = ctorProp
				break
			}
		}
	}

	// Pack the class back into an expression. We don't need to handle TypeScript
	// decorators for class expressions because TypeScript doesn't support them.
	if kind == classKindExpr {
		// Calling "nameFunc" will replace "expr", so make sure to do that first
		// before joining "expr" with any other expressions
		var nameToJoin ast.Expr
		if didCaptureClassExpr || computedPropertyCache.Data != nil ||
			len(privateMembers) > 0 || len(staticMembers) > 0 {
			nameToJoin = nameFunc()
		}

		// Then join "expr" with any other expressions that apply
		if computedPropertyCache.Data != nil {
			expr = ast.JoinWithComma(expr, computedPropertyCache)
		}
		for _, value := range privateMembers {
			expr = ast.JoinWithComma(expr, value)
		}
		for _, value := range staticMembers {
			expr = ast.JoinWithComma(expr, value)
		}

		// Finally join "expr" with the variable that holds the class object
		if nameToJoin.Data != nil {
			expr = ast.JoinWithComma(expr, nameToJoin)
		}
		if wrapFunc != nil {
			expr = wrapFunc(expr)
		}
		return nil, expr
	}

	// Pack the class back into a statement, with potentially some extra
	// statements afterwards
	var stmts []ast.Stmt
	if len(class.TSDecorators) > 0 {
		name := nameFunc()
		id, _ := name.Data.(*ast.EIdentifier)
		classExpr := ast.EClass{Class: *class}
		class = &classExpr.Class
		stmts = append(stmts, ast.Stmt{Loc: classLoc, Data: &ast.SLocal{
			Kind:     ast.LocalLet,
			IsExport: kind == classKindExportStmt,
			Decls: []ast.Decl{{
				Binding: ast.Binding{Loc: name.Loc, Data: &ast.BIdentifier{Ref: id.Ref}},
				Value:   &ast.Expr{Loc: classLoc, Data: &classExpr},
			}},
		}})
	} else {
		switch kind {
		case classKindStmt:
			stmts = append(stmts, ast.Stmt{Loc: classLoc, Data: &ast.SClass{Class: *class}})
		case classKindExportStmt:
			stmts = append(stmts, ast.Stmt{Loc: classLoc, Data: &ast.SClass{Class: *class, IsExport: true}})
		case classKindExportDefaultStmt:
			stmts = append(stmts, ast.Stmt{Loc: classLoc, Data: &ast.SExportDefault{
				DefaultName: defaultName,
				Value:       ast.ExprOrStmt{Stmt: &ast.Stmt{Loc: classLoc, Data: &ast.SClass{Class: *class}}},
			}})
		}
	}

	// The official TypeScript compiler adds generated code after the class body
	// in this exact order. Matching this order is important for correctness.
	if computedPropertyCache.Data != nil {
		stmts = append(stmts, ast.Stmt{Loc: expr.Loc, Data: &ast.SExpr{Value: computedPropertyCache}})
	}
	for _, expr := range privateMembers {
		stmts = append(stmts, ast.Stmt{Loc: expr.Loc, Data: &ast.SExpr{Value: expr}})
	}
	for _, expr := range staticMembers {
		stmts = append(stmts, ast.Stmt{Loc: expr.Loc, Data: &ast.SExpr{Value: expr}})
	}
	for _, expr := range instanceDecorators {
		stmts = append(stmts, ast.Stmt{Loc: expr.Loc, Data: &ast.SExpr{Value: expr}})
	}
	for _, expr := range staticDecorators {
		stmts = append(stmts, ast.Stmt{Loc: expr.Loc, Data: &ast.SExpr{Value: expr}})
	}
	if len(class.TSDecorators) > 0 {
		stmts = append(stmts, ast.AssignStmt(
			nameFunc(),
			p.callRuntime(classLoc, "__decorate", []ast.Expr{
				{Loc: classLoc, Data: &ast.EArray{Items: class.TSDecorators}},
				nameFunc(),
			}),
		))
		if kind == classKindExportDefaultStmt {
			// Generate a new default name symbol since the current one is being used
			// by the class. If this SExportDefault turns into a variable declaration,
			// we don't want it to accidentally use the same variable as the class and
			// cause a name collision.
			defaultRef := p.generateTempRef(tempRefNoDeclare, p.source.IdentifierName+"_default")
			p.namedExports["default"] = defaultRef
			p.recordDeclaredSymbol(defaultRef)

			name := nameFunc()
			stmts = append(stmts, ast.Stmt{Loc: classLoc, Data: &ast.SExportDefault{
				DefaultName: ast.LocRef{Loc: defaultName.Loc, Ref: defaultRef},
				Value:       ast.ExprOrStmt{Expr: &name},
			}})
		}
		class.Name = nil
	}
	return stmts, ast.Expr{}
}
