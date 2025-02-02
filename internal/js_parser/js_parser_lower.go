// This file contains code for "lowering" syntax, which means converting it to
// older JavaScript. For example, "a ** b" becomes a call to "Math.pow(a, b)"
// when lowered. Which syntax is lowered is determined by the language target.

package js_parser

import (
	"fmt"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/logger"
)

func (p *parser) markSyntaxFeature(feature compat.JSFeature, r logger.Range) (didGenerateError bool) {
	didGenerateError = true

	if !p.options.unsupportedJSFeatures.Has(feature) {
		if feature == compat.TopLevelAwait && !p.options.outputFormat.KeepESMImportExportSyntax() {
			p.log.AddError(&p.tracker, r, fmt.Sprintf(
				"Top-level await is currently not supported with the %q output format", p.options.outputFormat.String()))
			return
		}

		didGenerateError = false
		return
	}

	var name string
	where := config.PrettyPrintTargetEnvironment(p.options.originalTargetEnv, p.options.unsupportedJSFeatureOverridesMask)

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

	case compat.Destructuring:
		name = "destructuring"

	case compat.NewTarget:
		name = "new.target"

	case compat.ConstAndLet:
		name = p.source.TextForRange(r)

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

	case compat.ImportAttributes:
		p.log.AddError(&p.tracker, r, fmt.Sprintf(
			"Using an arbitrary value as the second argument to \"import()\" is not possible in %s", where))
		return

	case compat.TopLevelAwait:
		p.log.AddError(&p.tracker, r, fmt.Sprintf(
			"Top-level await is not available in %s", where))
		return

	case compat.Bigint:
		// This can't be polyfilled
		kind := logger.Warning
		if p.suppressWarningsAboutWeirdCode || p.fnOrArrowDataVisit.tryBodyCount > 0 {
			kind = logger.Debug
		}
		p.log.AddID(logger.MsgID_JS_BigInt, kind, &p.tracker, r, fmt.Sprintf(
			"Big integer literals are not available in %s and may crash at run-time", where))
		return

	case compat.ImportMeta:
		// This can't be polyfilled
		kind := logger.Warning
		if p.suppressWarningsAboutWeirdCode || p.fnOrArrowDataVisit.tryBodyCount > 0 {
			kind = logger.Debug
		}
		p.log.AddID(logger.MsgID_JS_EmptyImportMeta, kind, &p.tracker, r, fmt.Sprintf(
			"\"import.meta\" is not available in %s and will be empty", where))
		return

	default:
		p.log.AddError(&p.tracker, r, fmt.Sprintf(
			"This feature is not available in %s", where))
		return
	}

	p.log.AddError(&p.tracker, r, fmt.Sprintf(
		"Transforming %s to %s is not supported yet", name, where))
	return
}

func (p *parser) isStrictMode() bool {
	return p.currentScope.StrictMode != js_ast.SloppyMode
}

func (p *parser) isStrictModeOutputFormat() bool {
	return p.options.outputFormat == config.FormatESModule
}

type strictModeFeature uint8

const (
	withStatement strictModeFeature = iota
	deleteBareName
	forInVarInit
	evalOrArguments
	reservedWord
	legacyOctalLiteral
	legacyOctalEscape
	ifElseFunctionStmt
	labelFunctionStmt
	duplicateLexicallyDeclaredNames
)

func (p *parser) markStrictModeFeature(feature strictModeFeature, r logger.Range, detail string) {
	var text string
	canBeTransformed := false

	switch feature {
	case withStatement:
		text = "With statements"

	case deleteBareName:
		text = "Delete of a bare identifier"

	case forInVarInit:
		text = "Variable initializers inside for-in loops"
		canBeTransformed = true

	case evalOrArguments:
		text = fmt.Sprintf("Declarations with the name %q", detail)

	case reservedWord:
		text = fmt.Sprintf("%q is a reserved word and", detail)

	case legacyOctalLiteral:
		text = "Legacy octal literals"

	case legacyOctalEscape:
		text = "Legacy octal escape sequences"

	case ifElseFunctionStmt:
		text = "Function declarations inside if statements"

	case labelFunctionStmt:
		text = "Function declarations inside labels"

	case duplicateLexicallyDeclaredNames:
		text = "Duplicate lexically-declared names"

	default:
		text = "This feature"
	}

	if p.isStrictMode() {
		where, notes := p.whyStrictMode(p.currentScope)
		p.log.AddErrorWithNotes(&p.tracker, r,
			fmt.Sprintf("%s cannot be used %s", text, where), notes)
	} else if !canBeTransformed && p.isStrictModeOutputFormat() {
		p.log.AddError(&p.tracker, r,
			fmt.Sprintf("%s cannot be used with the \"esm\" output format due to strict mode", text))
	}
}

func (p *parser) whyStrictMode(scope *js_ast.Scope) (where string, notes []logger.MsgData) {
	where = "in strict mode"

	switch scope.StrictMode {
	case js_ast.ImplicitStrictModeClass:
		notes = []logger.MsgData{p.tracker.MsgData(p.enclosingClassKeyword,
			"All code inside a class is implicitly in strict mode")}

	case js_ast.ImplicitStrictModeTSAlwaysStrict:
		tsAlwaysStrict := p.options.tsAlwaysStrict
		t := logger.MakeLineColumnTracker(&tsAlwaysStrict.Source)
		notes = []logger.MsgData{t.MsgData(tsAlwaysStrict.Range, fmt.Sprintf(
			"TypeScript's %q setting was enabled here:", tsAlwaysStrict.Name))}

	case js_ast.ImplicitStrictModeJSXAutomaticRuntime:
		notes = []logger.MsgData{p.tracker.MsgData(logger.Range{Loc: p.firstJSXElementLoc, Len: 1},
			"This file is implicitly in strict mode due to the JSX element here:"),
			{Text: "When React's \"automatic\" JSX transform is enabled, using a JSX element automatically inserts " +
				"an \"import\" statement at the top of the file for the corresponding the JSX helper function. " +
				"This means the file is considered an ECMAScript module, and all ECMAScript modules use strict mode."}}

	case js_ast.ExplicitStrictMode:
		notes = []logger.MsgData{p.tracker.MsgData(p.source.RangeOfString(scope.UseStrictLoc),
			"Strict mode is triggered by the \"use strict\" directive here:")}

	case js_ast.ImplicitStrictModeESM:
		_, notes = p.whyESModule()
		where = "in an ECMAScript module"
	}

	return
}

func (p *parser) markAsyncFn(asyncRange logger.Range, isGenerator bool) (didGenerateError bool) {
	// Lowered async functions are implemented in terms of generators. So if
	// generators aren't supported, async functions aren't supported either.
	// But if generators are supported, then async functions are unconditionally
	// supported because we can use generators to implement them.
	if !p.options.unsupportedJSFeatures.Has(compat.Generator) {
		return false
	}

	feature := compat.AsyncAwait
	if isGenerator {
		feature = compat.AsyncGenerator
	}
	return p.markSyntaxFeature(feature, asyncRange)
}

func (p *parser) captureThis() ast.Ref {
	if p.fnOnlyDataVisit.thisCaptureRef == nil {
		ref := p.newSymbol(ast.SymbolHoisted, "_this")
		p.fnOnlyDataVisit.thisCaptureRef = &ref
	}

	ref := *p.fnOnlyDataVisit.thisCaptureRef
	p.recordUsage(ref)
	return ref
}

func (p *parser) captureArguments() ast.Ref {
	if p.fnOnlyDataVisit.argumentsCaptureRef == nil {
		ref := p.newSymbol(ast.SymbolHoisted, "_arguments")
		p.fnOnlyDataVisit.argumentsCaptureRef = &ref
	}

	ref := *p.fnOnlyDataVisit.argumentsCaptureRef
	p.recordUsage(ref)
	return ref
}

func (p *parser) lowerFunction(
	isAsync *bool,
	isGenerator *bool,
	args *[]js_ast.Arg,
	bodyLoc logger.Loc,
	bodyBlock *js_ast.SBlock,
	preferExpr *bool,
	hasRestArg *bool,
	isArrow bool,
) {
	// Lower object rest binding patterns in function arguments
	if p.options.unsupportedJSFeatures.Has(compat.ObjectRestSpread) {
		var prefixStmts []js_ast.Stmt

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
				target := js_ast.ConvertBindingToExpr(arg.Binding, nil)
				init := js_ast.Expr{Loc: arg.Binding.Loc, Data: &js_ast.EIdentifier{Ref: ref}}
				p.recordUsage(ref)

				if decls, ok := p.lowerObjectRestToDecls(target, init, nil); ok {
					// Replace the binding but leave the default value intact
					(*args)[i].Binding.Data = &js_ast.BIdentifier{Ref: ref}

					// Append a variable declaration to the function body
					prefixStmts = append(prefixStmts, js_ast.Stmt{Loc: arg.Binding.Loc,
						Data: &js_ast.SLocal{Kind: js_ast.LocalVar, Decls: decls}})
				}
			}
		}

		if len(prefixStmts) > 0 {
			bodyBlock.Stmts = append(prefixStmts, bodyBlock.Stmts...)
		}
	}

	// Lower async functions and async generator functions
	if *isAsync && (p.options.unsupportedJSFeatures.Has(compat.AsyncAwait) || (isGenerator != nil && *isGenerator && p.options.unsupportedJSFeatures.Has(compat.AsyncGenerator))) {
		// Use the shortened form if we're an arrow function
		if preferExpr != nil {
			*preferExpr = true
		}

		// Determine the value for "this"
		thisValue, hasThisValue := p.valueForThis(
			bodyLoc,
			false, /* shouldWarn */
			js_ast.AssignTargetNone,
			false, /* isCallTarget */
			false, /* isDeleteTarget */
		)
		if !hasThisValue {
			thisValue = js_ast.Expr{Loc: bodyLoc, Data: js_ast.EThisShared}
		}

		// Move the code into a nested generator function
		fn := js_ast.Fn{
			IsGenerator: true,
			Body:        js_ast.FnBody{Loc: bodyLoc, Block: *bodyBlock},
		}
		bodyBlock.Stmts = nil

		// Errors thrown during argument evaluation must reject the
		// resulting promise, which needs more complex code to handle
		couldThrowErrors := false
		for _, arg := range *args {
			if _, ok := arg.Binding.Data.(*js_ast.BIdentifier); !ok ||
				(arg.DefaultOrNil.Data != nil && couldPotentiallyThrow(arg.DefaultOrNil.Data)) {
				couldThrowErrors = true
				break
			}
		}

		// Forward the arguments to the wrapper function
		usesArgumentsRef := !isArrow && p.fnOnlyDataVisit.argumentsRef != nil &&
			p.symbolUses[*p.fnOnlyDataVisit.argumentsRef].CountEstimate > 0
		var forwardedArgs js_ast.Expr
		if !couldThrowErrors && !usesArgumentsRef {
			// Simple case: the arguments can stay on the outer function. It's
			// worth separating out the simple case because it's the common case
			// and it generates smaller code.
			forwardedArgs = js_ast.Expr{Loc: bodyLoc, Data: js_ast.ENullShared}
		} else {
			// If code uses "arguments" then we must move the arguments to the inner
			// function. This is because you can modify arguments by assigning to
			// elements in the "arguments" object:
			//
			//   async function foo(x) {
			//     arguments[0] = 1;
			//     // "x" must be 1 here
			//   }
			//

			// Complex case: the arguments must be moved to the inner function
			fn.Args = *args
			fn.HasRestArg = *hasRestArg
			*args = nil
			*hasRestArg = false

			// Make sure to not change the value of the "length" property. This is
			// done by generating dummy arguments for the outer function equal to
			// the expected length of the function:
			//
			//   async function foo(a, b, c = d, ...e) {
			//   }
			//
			// This turns into:
			//
			//   function foo(_0, _1) {
			//     return __async(this, arguments, function* (a, b, c = d, ...e) {
			//     });
			//   }
			//
			// The "_0" and "_1" are dummy variables to ensure "foo.length" is 2.
			for i, arg := range fn.Args {
				if arg.DefaultOrNil.Data != nil || fn.HasRestArg && i+1 == len(fn.Args) {
					// Arguments from here on don't add to the "length"
					break
				}

				// Generate a dummy variable
				argRef := p.newSymbol(ast.SymbolOther, fmt.Sprintf("_%d", i))
				p.currentScope.Generated = append(p.currentScope.Generated, argRef)
				*args = append(*args, js_ast.Arg{Binding: js_ast.Binding{Loc: arg.Binding.Loc, Data: &js_ast.BIdentifier{Ref: argRef}}})
			}

			// Forward all arguments from the outer function to the inner function
			if !isArrow {
				// Normal functions can just use "arguments" to forward everything
				forwardedArgs = js_ast.Expr{Loc: bodyLoc, Data: &js_ast.EIdentifier{Ref: *p.fnOnlyDataVisit.argumentsRef}}
			} else {
				// Arrow functions can't use "arguments", so we need to forward
				// the arguments manually.
				//
				// Note that if the arrow function references "arguments" in its body
				// (even if it's inside another nested arrow function), that reference
				// to "arguments" will have to be substituted with a captured variable.
				// This is because we're changing the arrow function into a generator
				// function, which introduces a variable named "arguments". This is
				// handled separately during symbol resolution instead of being handled
				// here so we don't need to re-traverse the arrow function body.

				// If we need to forward more than the current number of arguments,
				// add a rest argument to the set of forwarding variables. This is the
				// case if the arrow function has rest or default arguments.
				if len(*args) < len(fn.Args) {
					argRef := p.newSymbol(ast.SymbolOther, fmt.Sprintf("_%d", len(*args)))
					p.currentScope.Generated = append(p.currentScope.Generated, argRef)
					*args = append(*args, js_ast.Arg{Binding: js_ast.Binding{Loc: bodyLoc, Data: &js_ast.BIdentifier{Ref: argRef}}})
					*hasRestArg = true
				}

				// Forward all of the arguments
				items := make([]js_ast.Expr, 0, len(*args))
				for i, arg := range *args {
					id := arg.Binding.Data.(*js_ast.BIdentifier)
					item := js_ast.Expr{Loc: arg.Binding.Loc, Data: &js_ast.EIdentifier{Ref: id.Ref}}
					if *hasRestArg && i+1 == len(*args) {
						item.Data = &js_ast.ESpread{Value: item}
					}
					items = append(items, item)
				}
				forwardedArgs = js_ast.Expr{Loc: bodyLoc, Data: &js_ast.EArray{Items: items, IsSingleLine: true}}
			}
		}

		var name string
		if isGenerator != nil && *isGenerator {
			// "async function* foo(a, b) { stmts }" => "function foo(a, b) { return __asyncGenerator(this, null, function* () { stmts }) }"
			name = "__asyncGenerator"
			*isGenerator = false
		} else {
			// "async function foo(a, b) { stmts }" => "function foo(a, b) { return __async(this, null, function* () { stmts }) }"
			name = "__async"
		}
		*isAsync = false
		callAsync := p.callRuntime(bodyLoc, name, []js_ast.Expr{
			thisValue,
			forwardedArgs,
			{Loc: bodyLoc, Data: &js_ast.EFunction{Fn: fn}},
		})
		bodyBlock.Stmts = []js_ast.Stmt{{Loc: bodyLoc, Data: &js_ast.SReturn{ValueOrNil: callAsync}}}
	}
}

func (p *parser) lowerOptionalChain(expr js_ast.Expr, in exprIn, childOut exprOut) (js_ast.Expr, exprOut) {
	valueWhenUndefined := js_ast.Expr{Loc: expr.Loc, Data: js_ast.EUndefinedShared}
	endsWithPropertyAccess := false
	containsPrivateName := false
	startsWithCall := false
	originalExpr := expr
	chain := []js_ast.Expr{}
	loc := expr.Loc

	// Step 1: Get an array of all expressions in the chain. We're traversing the
	// chain from the outside in, so the array will be filled in "backwards".
flatten:
	for {
		chain = append(chain, expr)

		switch e := expr.Data.(type) {
		case *js_ast.EDot:
			expr = e.Target
			if len(chain) == 1 {
				endsWithPropertyAccess = true
			}
			if e.OptionalChain == js_ast.OptionalChainStart {
				break flatten
			}

		case *js_ast.EIndex:
			expr = e.Target
			if len(chain) == 1 {
				endsWithPropertyAccess = true
			}

			// If this is a private name that needs to be lowered, the entire chain
			// itself will have to be lowered even if the language target supports
			// optional chaining. This is because there's no way to use our shim
			// function for private names with optional chaining syntax.
			if private, ok := e.Index.Data.(*js_ast.EPrivateIdentifier); ok && p.privateSymbolNeedsToBeLowered(private) {
				containsPrivateName = true
			}

			if e.OptionalChain == js_ast.OptionalChainStart {
				break flatten
			}

		case *js_ast.ECall:
			expr = e.Target
			if e.OptionalChain == js_ast.OptionalChainStart {
				startsWithCall = true
				break flatten
			}

		case *js_ast.EUnary: // UnOpDelete
			valueWhenUndefined = js_ast.Expr{Loc: loc, Data: &js_ast.EBoolean{Value: true}}
			expr = e.Value

		default:
			panic("Internal error")
		}
	}

	// Stop now if we can strip the whole chain as dead code. Since the chain is
	// lazily evaluated, it's safe to just drop the code entirely.
	if p.options.minifySyntax {
		if isNullOrUndefined, sideEffects, ok := js_ast.ToNullOrUndefinedWithSideEffects(expr.Data); ok && isNullOrUndefined {
			if sideEffects == js_ast.CouldHaveSideEffects {
				return js_ast.JoinWithComma(p.astHelpers.SimplifyUnusedExpr(expr, p.options.unsupportedJSFeatures), valueWhenUndefined), exprOut{}
			}
			return valueWhenUndefined, exprOut{}
		}
	} else {
		switch expr.Data.(type) {
		case *js_ast.ENull, *js_ast.EUndefined:
			return valueWhenUndefined, exprOut{}
		}
	}

	// We need to lower this if this is an optional call off of a private name
	// such as "foo.#bar?.()" because the value of "this" must be captured.
	if _, _, private := p.extractPrivateIndex(expr); private != nil {
		containsPrivateName = true
	}

	// Don't lower this if we don't need to. This check must be done here instead
	// of earlier so we can do the dead code elimination above when the target is
	// null or undefined.
	if !p.options.unsupportedJSFeatures.Has(compat.OptionalChain) && !containsPrivateName {
		return originalExpr, exprOut{}
	}

	// Step 2: Figure out if we need to capture the value for "this" for the
	// initial ECall. This will be passed to ".call(this, ...args)" later.
	var thisArg js_ast.Expr
	var targetWrapFunc func(js_ast.Expr) js_ast.Expr
	if startsWithCall {
		if childOut.thisArgFunc != nil {
			// The initial value is a nested optional chain that ended in a property
			// access. The nested chain was processed first and has saved the
			// appropriate value for "this". The callback here will return a
			// reference to that saved location.
			thisArg = childOut.thisArgFunc()
		} else {
			// The initial value is a normal expression. If it's a property access,
			// strip the property off and save the target of the property access to
			// be used as the value for "this".
			switch e := expr.Data.(type) {
			case *js_ast.EDot:
				if _, ok := e.Target.Data.(*js_ast.ESuper); ok {
					// Lower "super.prop" if necessary
					if p.shouldLowerSuperPropertyAccess(e.Target) {
						key := js_ast.Expr{Loc: e.NameLoc, Data: &js_ast.EString{Value: helpers.StringToUTF16(e.Name)}}
						expr = p.lowerSuperPropertyGet(expr.Loc, key)
					}

					// Special-case "super.foo?.()" to avoid a syntax error. Without this,
					// we would generate:
					//
					//   (_b = (_a = super).foo) == null ? void 0 : _b.call(_a)
					//
					// which is a syntax error. Now we generate this instead:
					//
					//   (_a = super.foo) == null ? void 0 : _a.call(this)
					//
					thisArg = js_ast.Expr{Loc: loc, Data: js_ast.EThisShared}
				} else {
					targetFunc, wrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, e.Target, valueDefinitelyNotMutated)
					expr = js_ast.Expr{Loc: loc, Data: &js_ast.EDot{
						Target:  targetFunc(),
						Name:    e.Name,
						NameLoc: e.NameLoc,
					}}
					thisArg = targetFunc()
					targetWrapFunc = wrapFunc
				}

			case *js_ast.EIndex:
				if _, ok := e.Target.Data.(*js_ast.ESuper); ok {
					// Lower "super[prop]" if necessary
					if p.shouldLowerSuperPropertyAccess(e.Target) {
						expr = p.lowerSuperPropertyGet(expr.Loc, e.Index)
					}

					// See the comment above about a similar special case for EDot
					thisArg = js_ast.Expr{Loc: loc, Data: js_ast.EThisShared}
				} else {
					targetFunc, wrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, e.Target, valueDefinitelyNotMutated)
					targetWrapFunc = wrapFunc

					// Capture the value of "this" if the target of the starting call
					// expression is a private property access
					if private, ok := e.Index.Data.(*js_ast.EPrivateIdentifier); ok && p.privateSymbolNeedsToBeLowered(private) {
						// "foo().#bar?.()" must capture "foo()" for "this"
						expr = p.lowerPrivateGet(targetFunc(), e.Index.Loc, private)
						thisArg = targetFunc()
						break
					}

					expr = js_ast.Expr{Loc: loc, Data: &js_ast.EIndex{
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
	exprFunc, exprWrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, expr, valueDefinitelyNotMutated)
	expr = exprFunc()
	result := exprFunc()

	// Step 4: Wrap the starting value by each expression in the chain. We
	// traverse the chain in reverse because we want to go from the inside out
	// and the chain was built from the outside in.
	var parentThisArgFunc func() js_ast.Expr
	var parentThisArgWrapFunc func(js_ast.Expr) js_ast.Expr
	var privateThisFunc func() js_ast.Expr
	var privateThisWrapFunc func(js_ast.Expr) js_ast.Expr
	for i := len(chain) - 1; i >= 0; i-- {
		// Save a reference to the value of "this" for our parent ECall
		if i == 0 && in.storeThisArgForParentOptionalChain && endsWithPropertyAccess {
			parentThisArgFunc, parentThisArgWrapFunc = p.captureValueWithPossibleSideEffects(result.Loc, 2, result, valueDefinitelyNotMutated)
			result = parentThisArgFunc()
		}

		switch e := chain[i].Data.(type) {
		case *js_ast.EDot:
			result = js_ast.Expr{Loc: loc, Data: &js_ast.EDot{
				Target:  result,
				Name:    e.Name,
				NameLoc: e.NameLoc,
			}}

		case *js_ast.EIndex:
			if private, ok := e.Index.Data.(*js_ast.EPrivateIdentifier); ok && p.privateSymbolNeedsToBeLowered(private) {
				// If this is private name property access inside a call expression and
				// the call expression is part of this chain, then the call expression
				// is going to need a copy of the property access target as the value
				// for "this" for the call. Example for this case: "foo.#bar?.()"
				if i > 0 {
					if _, ok := chain[i-1].Data.(*js_ast.ECall); ok {
						privateThisFunc, privateThisWrapFunc = p.captureValueWithPossibleSideEffects(loc, 2, result, valueDefinitelyNotMutated)
						result = privateThisFunc()
					}
				}

				result = p.lowerPrivateGet(result, e.Index.Loc, private)
				continue
			}

			result = js_ast.Expr{Loc: loc, Data: &js_ast.EIndex{
				Target: result,
				Index:  e.Index,
			}}

		case *js_ast.ECall:
			// If this is the initial ECall in the chain and it's being called off of
			// a property access, invoke the function using ".call(this, ...args)" to
			// explicitly provide the value for "this".
			if i == len(chain)-1 && thisArg.Data != nil {
				result = js_ast.Expr{Loc: loc, Data: &js_ast.ECall{
					Target: js_ast.Expr{Loc: loc, Data: &js_ast.EDot{
						Target:  result,
						Name:    "call",
						NameLoc: loc,
					}},
					Args:                   append([]js_ast.Expr{thisArg}, e.Args...),
					CanBeUnwrappedIfUnused: e.CanBeUnwrappedIfUnused,
					IsMultiLine:            e.IsMultiLine,
					Kind:                   js_ast.TargetWasOriginallyPropertyAccess,
				}}
				break
			}

			// If the target of this call expression is a private name property
			// access that's also part of this chain, then we must use the copy of
			// the property access target that was stashed away earlier as the value
			// for "this" for the call. Example for this case: "foo.#bar?.()"
			if privateThisFunc != nil {
				result = privateThisWrapFunc(js_ast.Expr{Loc: loc, Data: &js_ast.ECall{
					Target: js_ast.Expr{Loc: loc, Data: &js_ast.EDot{
						Target:  result,
						Name:    "call",
						NameLoc: loc,
					}},
					Args:                   append([]js_ast.Expr{privateThisFunc()}, e.Args...),
					CanBeUnwrappedIfUnused: e.CanBeUnwrappedIfUnused,
					IsMultiLine:            e.IsMultiLine,
					Kind:                   js_ast.TargetWasOriginallyPropertyAccess,
				}})
				privateThisFunc = nil
				break
			}

			result = js_ast.Expr{Loc: loc, Data: &js_ast.ECall{
				Target:                 result,
				Args:                   e.Args,
				CanBeUnwrappedIfUnused: e.CanBeUnwrappedIfUnused,
				IsMultiLine:            e.IsMultiLine,
				Kind:                   e.Kind,
			}}

		case *js_ast.EUnary:
			result = js_ast.Expr{Loc: loc, Data: &js_ast.EUnary{
				Op:    js_ast.UnOpDelete,
				Value: result,

				// If a delete of an optional chain takes place, it behaves as if the
				// optional chain isn't there with regard to the "delete" semantics.
				WasOriginallyDeleteOfIdentifierOrPropertyAccess: e.WasOriginallyDeleteOfIdentifierOrPropertyAccess,
			}}

		default:
			panic("Internal error")
		}
	}

	// Step 5: Wrap it all in a conditional that returns the chain or the default
	// value if the initial value is null/undefined. The default value is usually
	// "undefined" but is "true" if the chain ends in a "delete" operator.
	// "x?.y" => "x == null ? void 0 : x.y"
	// "x()?.y()" => "(_a = x()) == null ? void 0 : _a.y()"
	result = js_ast.Expr{Loc: loc, Data: &js_ast.EIf{
		Test: js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
			Op:    js_ast.BinOpLooseEq,
			Left:  expr,
			Right: js_ast.Expr{Loc: loc, Data: js_ast.ENullShared},
		}},
		Yes: valueWhenUndefined,
		No:  result,
	}}
	if exprWrapFunc != nil {
		result = exprWrapFunc(result)
	}
	if targetWrapFunc != nil {
		result = targetWrapFunc(result)
	}
	if childOut.thisArgWrapFunc != nil {
		result = childOut.thisArgWrapFunc(result)
	}
	return result, exprOut{
		thisArgFunc:     parentThisArgFunc,
		thisArgWrapFunc: parentThisArgWrapFunc,
	}
}

func (p *parser) lowerParenthesizedOptionalChain(loc logger.Loc, e *js_ast.ECall, childOut exprOut) js_ast.Expr {
	return childOut.thisArgWrapFunc(js_ast.Expr{Loc: loc, Data: &js_ast.ECall{
		Target: js_ast.Expr{Loc: loc, Data: &js_ast.EDot{
			Target:  e.Target,
			Name:    "call",
			NameLoc: loc,
		}},
		Args:        append(append(make([]js_ast.Expr, 0, len(e.Args)+1), childOut.thisArgFunc()), e.Args...),
		IsMultiLine: e.IsMultiLine,
		Kind:        js_ast.TargetWasOriginallyPropertyAccess,
	}})
}

func (p *parser) lowerAssignmentOperator(value js_ast.Expr, callback func(js_ast.Expr, js_ast.Expr) js_ast.Expr) js_ast.Expr {
	switch left := value.Data.(type) {
	case *js_ast.EDot:
		if left.OptionalChain == js_ast.OptionalChainNone {
			referenceFunc, wrapFunc := p.captureValueWithPossibleSideEffects(value.Loc, 2, left.Target, valueDefinitelyNotMutated)
			return wrapFunc(callback(
				js_ast.Expr{Loc: value.Loc, Data: &js_ast.EDot{
					Target:  referenceFunc(),
					Name:    left.Name,
					NameLoc: left.NameLoc,
				}},
				js_ast.Expr{Loc: value.Loc, Data: &js_ast.EDot{
					Target:  referenceFunc(),
					Name:    left.Name,
					NameLoc: left.NameLoc,
				}},
			))
		}

	case *js_ast.EIndex:
		if left.OptionalChain == js_ast.OptionalChainNone {
			targetFunc, targetWrapFunc := p.captureValueWithPossibleSideEffects(value.Loc, 2, left.Target, valueDefinitelyNotMutated)
			indexFunc, indexWrapFunc := p.captureValueWithPossibleSideEffects(value.Loc, 2, left.Index, valueDefinitelyNotMutated)
			return targetWrapFunc(indexWrapFunc(callback(
				js_ast.Expr{Loc: value.Loc, Data: &js_ast.EIndex{
					Target: targetFunc(),
					Index:  indexFunc(),
				}},
				js_ast.Expr{Loc: value.Loc, Data: &js_ast.EIndex{
					Target: targetFunc(),
					Index:  indexFunc(),
				}},
			)))
		}

	case *js_ast.EIdentifier:
		return callback(
			js_ast.Expr{Loc: value.Loc, Data: &js_ast.EIdentifier{Ref: left.Ref}},
			value,
		)
	}

	// We shouldn't get here with valid syntax? Just let this through for now
	// since there's currently no assignment target validation. Garbage in,
	// garbage out.
	return value
}

func (p *parser) lowerExponentiationAssignmentOperator(loc logger.Loc, e *js_ast.EBinary) js_ast.Expr {
	if target, privateLoc, private := p.extractPrivateIndex(e.Left); private != nil {
		// "a.#b **= c" => "__privateSet(a, #b, __pow(__privateGet(a, #b), c))"
		targetFunc, targetWrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, target, valueDefinitelyNotMutated)
		return targetWrapFunc(p.lowerPrivateSet(targetFunc(), privateLoc, private,
			p.callRuntime(loc, "__pow", []js_ast.Expr{
				p.lowerPrivateGet(targetFunc(), privateLoc, private),
				e.Right,
			})))
	}

	return p.lowerAssignmentOperator(e.Left, func(a js_ast.Expr, b js_ast.Expr) js_ast.Expr {
		// "a **= b" => "a = __pow(a, b)"
		return js_ast.Assign(a, p.callRuntime(loc, "__pow", []js_ast.Expr{b, e.Right}))
	})
}

func (p *parser) lowerNullishCoalescingAssignmentOperator(loc logger.Loc, e *js_ast.EBinary) (js_ast.Expr, bool) {
	if target, privateLoc, private := p.extractPrivateIndex(e.Left); private != nil {
		if p.options.unsupportedJSFeatures.Has(compat.NullishCoalescing) {
			// "a.#b ??= c" => "(_a = __privateGet(a, #b)) != null ? _a : __privateSet(a, #b, c)"
			targetFunc, targetWrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, target, valueDefinitelyNotMutated)
			left := p.lowerPrivateGet(targetFunc(), privateLoc, private)
			right := p.lowerPrivateSet(targetFunc(), privateLoc, private, e.Right)
			return targetWrapFunc(p.lowerNullishCoalescing(loc, left, right)), true
		}

		// "a.#b ??= c" => "__privateGet(a, #b) ?? __privateSet(a, #b, c)"
		targetFunc, targetWrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, target, valueDefinitelyNotMutated)
		return targetWrapFunc(js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
			Op:    js_ast.BinOpNullishCoalescing,
			Left:  p.lowerPrivateGet(targetFunc(), privateLoc, private),
			Right: p.lowerPrivateSet(targetFunc(), privateLoc, private, e.Right),
		}}), true
	}

	if p.options.unsupportedJSFeatures.Has(compat.LogicalAssignment) {
		return p.lowerAssignmentOperator(e.Left, func(a js_ast.Expr, b js_ast.Expr) js_ast.Expr {
			if p.options.unsupportedJSFeatures.Has(compat.NullishCoalescing) {
				// "a ??= b" => "(_a = a) != null ? _a : a = b"
				return p.lowerNullishCoalescing(loc, a, js_ast.Assign(b, e.Right))
			}

			// "a ??= b" => "a ?? (a = b)"
			return js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
				Op:    js_ast.BinOpNullishCoalescing,
				Left:  a,
				Right: js_ast.Assign(b, e.Right),
			}}
		}), true
	}

	return js_ast.Expr{}, false
}

func (p *parser) lowerLogicalAssignmentOperator(loc logger.Loc, e *js_ast.EBinary, op js_ast.OpCode) (js_ast.Expr, bool) {
	if target, privateLoc, private := p.extractPrivateIndex(e.Left); private != nil {
		// "a.#b &&= c" => "__privateGet(a, #b) && __privateSet(a, #b, c)"
		// "a.#b ||= c" => "__privateGet(a, #b) || __privateSet(a, #b, c)"
		targetFunc, targetWrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, target, valueDefinitelyNotMutated)
		return targetWrapFunc(js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
			Op:    op,
			Left:  p.lowerPrivateGet(targetFunc(), privateLoc, private),
			Right: p.lowerPrivateSet(targetFunc(), privateLoc, private, e.Right),
		}}), true
	}

	if p.options.unsupportedJSFeatures.Has(compat.LogicalAssignment) {
		return p.lowerAssignmentOperator(e.Left, func(a js_ast.Expr, b js_ast.Expr) js_ast.Expr {
			// "a &&= b" => "a && (a = b)"
			// "a ||= b" => "a || (a = b)"
			return js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
				Op:    op,
				Left:  a,
				Right: js_ast.Assign(b, e.Right),
			}}
		}), true
	}

	return js_ast.Expr{}, false
}

func (p *parser) lowerNullishCoalescing(loc logger.Loc, left js_ast.Expr, right js_ast.Expr) js_ast.Expr {
	// "x ?? y" => "x != null ? x : y"
	// "x() ?? y()" => "_a = x(), _a != null ? _a : y"
	leftFunc, wrapFunc := p.captureValueWithPossibleSideEffects(loc, 2, left, valueDefinitelyNotMutated)
	return wrapFunc(js_ast.Expr{Loc: loc, Data: &js_ast.EIf{
		Test: js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
			Op:    js_ast.BinOpLooseNe,
			Left:  leftFunc(),
			Right: js_ast.Expr{Loc: loc, Data: js_ast.ENullShared},
		}},
		Yes: leftFunc(),
		No:  right,
	}})
}

// Lower object spread for environments that don't support them. Non-spread
// properties are grouped into object literals and then passed to the
// "__spreadValues" and "__spreadProps" functions like this:
//
//	"{a, b, ...c, d, e}" => "__spreadProps(__spreadValues(__spreadProps({a, b}, c), {d, e})"
//
// If the object literal starts with a spread, then we pass an empty object
// literal to "__spreadValues" to make sure we clone the object:
//
//	"{...a, b}" => "__spreadProps(__spreadValues({}, a), {b})"
//
// It's not immediately obvious why we don't compile everything to a single
// call to a function that takes any number of arguments, since that would be
// shorter. The reason is to preserve the order of side effects. Consider
// this code:
//
//	let a = {
//	  get x() {
//	    b = {y: 2}
//	    return 1
//	  }
//	}
//	let b = {}
//	let c = {...a, ...b}
//
// Converting the above code to "let c = __spreadFn({}, a, null, b)" means "c"
// becomes "{x: 1}" which is incorrect. Converting the above code instead to
// "let c = __spreadProps(__spreadProps({}, a), b)" means "c" becomes
// "{x: 1, y: 2}" which is correct.
func (p *parser) lowerObjectSpread(loc logger.Loc, e *js_ast.EObject) js_ast.Expr {
	needsLowering := false

	if p.options.unsupportedJSFeatures.Has(compat.ObjectRestSpread) {
		for _, property := range e.Properties {
			if property.Kind == js_ast.PropertySpread {
				needsLowering = true
				break
			}
		}
	}

	if !needsLowering {
		return js_ast.Expr{Loc: loc, Data: e}
	}

	var result js_ast.Expr
	properties := []js_ast.Property{}

	for _, property := range e.Properties {
		if property.Kind != js_ast.PropertySpread {
			properties = append(properties, property)
			continue
		}

		if len(properties) > 0 || result.Data == nil {
			if result.Data == nil {
				// "{a, ...b}" => "__spreadValues({a}, b)"
				result = js_ast.Expr{Loc: loc, Data: &js_ast.EObject{
					Properties:   properties,
					IsSingleLine: e.IsSingleLine,
				}}
			} else {
				// "{...a, b, ...c}" => "__spreadValues(__spreadProps(__spreadValues({}, a), {b}), c)"
				result = p.callRuntime(loc, "__spreadProps",
					[]js_ast.Expr{result, {Loc: loc, Data: &js_ast.EObject{
						Properties:   properties,
						IsSingleLine: e.IsSingleLine,
					}}})
			}
			properties = []js_ast.Property{}
		}

		// "{a, ...b}" => "__spreadValues({a}, b)"
		result = p.callRuntime(loc, "__spreadValues", []js_ast.Expr{result, property.ValueOrNil})
	}

	if len(properties) > 0 {
		// "{...a, b}" => "__spreadProps(__spreadValues({}, a), {b})"
		result = p.callRuntime(loc, "__spreadProps", []js_ast.Expr{result, {Loc: loc, Data: &js_ast.EObject{
			Properties:    properties,
			IsSingleLine:  e.IsSingleLine,
			CloseBraceLoc: e.CloseBraceLoc,
		}}})
	}

	return result
}

func (p *parser) maybeLowerAwait(loc logger.Loc, e *js_ast.EAwait) js_ast.Expr {
	// "await x" turns into "yield __await(x)" when lowering async generator functions
	if p.fnOrArrowDataVisit.isGenerator && (p.options.unsupportedJSFeatures.Has(compat.AsyncAwait) || p.options.unsupportedJSFeatures.Has(compat.AsyncGenerator)) {
		return js_ast.Expr{Loc: loc, Data: &js_ast.EYield{
			ValueOrNil: js_ast.Expr{Loc: loc, Data: &js_ast.ENew{
				Target: p.importFromRuntime(loc, "__await"),
				Args:   []js_ast.Expr{e.Value},
			}},
		}}
	}

	// "await x" turns into "yield x" when lowering async functions
	if p.options.unsupportedJSFeatures.Has(compat.AsyncAwait) {
		return js_ast.Expr{Loc: loc, Data: &js_ast.EYield{
			ValueOrNil: e.Value,
		}}
	}

	return js_ast.Expr{Loc: loc, Data: e}
}

func (p *parser) lowerForAwaitLoop(loc logger.Loc, loop *js_ast.SForOf, stmts []js_ast.Stmt) []js_ast.Stmt {
	// This code:
	//
	//   for await (let x of y) z()
	//
	// is transformed into the following code:
	//
	//   try {
	//     for (var iter = __forAwait(y), more, temp, error; more = !(temp = await iter.next()).done; more = false) {
	//       let x = temp.value;
	//       z();
	//     }
	//   } catch (temp) {
	//     error = [temp]
	//   } finally {
	//     try {
	//       more && (temp = iter.return) && (await temp.call(iter))
	//     } finally {
	//       if (error) throw error[0]
	//     }
	//   }
	//
	// except that "yield" is used instead of "await" if await is unsupported.
	// This mostly follows TypeScript's implementation of the syntax transform.

	iterRef := p.generateTempRef(tempRefNoDeclare, "iter")
	moreRef := p.generateTempRef(tempRefNoDeclare, "more")
	tempRef := p.generateTempRef(tempRefNoDeclare, "temp")
	errorRef := p.generateTempRef(tempRefNoDeclare, "error")

	switch init := loop.Init.Data.(type) {
	case *js_ast.SLocal:
		if len(init.Decls) == 1 {
			init.Decls[0].ValueOrNil = js_ast.Expr{Loc: loc, Data: &js_ast.EDot{
				Target:  js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: tempRef}},
				NameLoc: loc,
				Name:    "value",
			}}
		}
	case *js_ast.SExpr:
		init.Value.Data = &js_ast.EBinary{
			Op:   js_ast.BinOpAssign,
			Left: init.Value,
			Right: js_ast.Expr{Loc: loc, Data: &js_ast.EDot{
				Target:  js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: tempRef}},
				NameLoc: loc,
				Name:    "value",
			}},
		}
	}

	var body []js_ast.Stmt
	var closeBraceLoc logger.Loc
	body = append(body, loop.Init)

	if block, ok := loop.Body.Data.(*js_ast.SBlock); ok {
		body = append(body, block.Stmts...)
		closeBraceLoc = block.CloseBraceLoc
	} else {
		body = append(body, loop.Body)
	}

	awaitIterNext := js_ast.Expr{Loc: loc, Data: &js_ast.ECall{
		Target: js_ast.Expr{Loc: loc, Data: &js_ast.EDot{
			Target:  js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: iterRef}},
			NameLoc: loc,
			Name:    "next",
		}},
		Kind: js_ast.TargetWasOriginallyPropertyAccess,
	}}
	awaitTempCallIter := js_ast.Expr{Loc: loc, Data: &js_ast.ECall{
		Target: js_ast.Expr{Loc: loc, Data: &js_ast.EDot{
			Target:  js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: tempRef}},
			NameLoc: loc,
			Name:    "call",
		}},
		Args: []js_ast.Expr{{Loc: loc, Data: &js_ast.EIdentifier{Ref: iterRef}}},
		Kind: js_ast.TargetWasOriginallyPropertyAccess,
	}}

	// "await" expressions turn into "yield" expressions when lowering
	awaitIterNext = p.maybeLowerAwait(awaitIterNext.Loc, &js_ast.EAwait{Value: awaitIterNext})
	awaitTempCallIter = p.maybeLowerAwait(awaitTempCallIter.Loc, &js_ast.EAwait{Value: awaitTempCallIter})

	return append(stmts, js_ast.Stmt{Loc: loc, Data: &js_ast.STry{
		BlockLoc: loc,
		Block: js_ast.SBlock{
			Stmts: []js_ast.Stmt{{Loc: loc, Data: &js_ast.SFor{
				InitOrNil: js_ast.Stmt{Loc: loc, Data: &js_ast.SLocal{Kind: js_ast.LocalVar, Decls: []js_ast.Decl{
					{Binding: js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: iterRef}},
						ValueOrNil: p.callRuntime(loc, "__forAwait", []js_ast.Expr{loop.Value})},
					{Binding: js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: moreRef}}},
					{Binding: js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: tempRef}}},
					{Binding: js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: errorRef}}},
				}}},
				TestOrNil: js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
					Op:   js_ast.BinOpAssign,
					Left: js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: moreRef}},
					Right: js_ast.Expr{Loc: loc, Data: &js_ast.EUnary{
						Op: js_ast.UnOpNot,
						Value: js_ast.Expr{Loc: loc, Data: &js_ast.EDot{
							Target: js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
								Op:    js_ast.BinOpAssign,
								Left:  js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: tempRef}},
								Right: awaitIterNext,
							}},
							NameLoc: loc,
							Name:    "done",
						}},
					}},
				}},
				UpdateOrNil: js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
					Op:    js_ast.BinOpAssign,
					Left:  js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: moreRef}},
					Right: js_ast.Expr{Loc: loc, Data: &js_ast.EBoolean{Value: false}},
				}},
				Body: js_ast.Stmt{Loc: loop.Body.Loc, Data: &js_ast.SBlock{
					Stmts:         body,
					CloseBraceLoc: closeBraceLoc,
				}},
			}}},
		},

		Catch: &js_ast.Catch{
			Loc: loc,
			BindingOrNil: js_ast.Binding{
				Loc:  loc,
				Data: &js_ast.BIdentifier{Ref: tempRef},
			},
			Block: js_ast.SBlock{
				Stmts: []js_ast.Stmt{{Loc: loc, Data: &js_ast.SExpr{Value: js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
					Op:   js_ast.BinOpAssign,
					Left: js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: errorRef}},
					Right: js_ast.Expr{Loc: loc, Data: &js_ast.EArray{
						Items:        []js_ast.Expr{{Loc: loc, Data: &js_ast.EIdentifier{Ref: tempRef}}},
						IsSingleLine: true,
					}},
				}}}}},
			},
		},

		Finally: &js_ast.Finally{
			Loc: loc,
			Block: js_ast.SBlock{
				Stmts: []js_ast.Stmt{{Loc: loc, Data: &js_ast.STry{
					BlockLoc: loc,
					Block: js_ast.SBlock{Stmts: []js_ast.Stmt{{Loc: loc, Data: &js_ast.SExpr{
						Value: js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
							Op: js_ast.BinOpLogicalAnd,
							Left: js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
								Op:   js_ast.BinOpLogicalAnd,
								Left: js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: moreRef}},
								Right: js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
									Op:   js_ast.BinOpAssign,
									Left: js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: tempRef}},
									Right: js_ast.Expr{Loc: loc, Data: &js_ast.EDot{
										Target:  js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: iterRef}},
										NameLoc: loc,
										Name:    "return",
									}},
								}},
							}},
							Right: awaitTempCallIter,
						}},
					}}}},
					Finally: &js_ast.Finally{
						Loc: loc,
						Block: js_ast.SBlock{Stmts: []js_ast.Stmt{{Loc: loc, Data: &js_ast.SIf{
							Test: js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: errorRef}},
							Yes: js_ast.Stmt{Loc: loc, Data: &js_ast.SThrow{Value: js_ast.Expr{Loc: loc, Data: &js_ast.EIndex{
								Target: js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: errorRef}},
								Index:  js_ast.Expr{Loc: loc, Data: &js_ast.ENumber{Value: 0}},
							}}}},
						}}}},
					},
				}}},
			},
		},
	}})
}

func bindingHasObjectRest(binding js_ast.Binding) bool {
	switch b := binding.Data.(type) {
	case *js_ast.BArray:
		for _, item := range b.Items {
			if bindingHasObjectRest(item.Binding) {
				return true
			}
		}
	case *js_ast.BObject:
		for _, property := range b.Properties {
			if property.IsSpread || bindingHasObjectRest(property.Value) {
				return true
			}
		}
	}
	return false
}

func exprHasObjectRest(expr js_ast.Expr) bool {
	switch e := expr.Data.(type) {
	case *js_ast.EBinary:
		if e.Op == js_ast.BinOpAssign && exprHasObjectRest(e.Left) {
			return true
		}
	case *js_ast.EArray:
		for _, item := range e.Items {
			if exprHasObjectRest(item) {
				return true
			}
		}
	case *js_ast.EObject:
		for _, property := range e.Properties {
			if property.Kind == js_ast.PropertySpread || exprHasObjectRest(property.ValueOrNil) {
				return true
			}
		}
	}
	return false
}

func (p *parser) lowerObjectRestInDecls(decls []js_ast.Decl) []js_ast.Decl {
	if !p.options.unsupportedJSFeatures.Has(compat.ObjectRestSpread) {
		return decls
	}

	// Don't do any allocations if there are no object rest patterns. We want as
	// little overhead as possible in the common case.
	for i, decl := range decls {
		if decl.ValueOrNil.Data != nil && bindingHasObjectRest(decl.Binding) {
			clone := append([]js_ast.Decl{}, decls[:i]...)
			for _, decl := range decls[i:] {
				if decl.ValueOrNil.Data != nil {
					target := js_ast.ConvertBindingToExpr(decl.Binding, nil)
					if result, ok := p.lowerObjectRestToDecls(target, decl.ValueOrNil, clone); ok {
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

func (p *parser) lowerObjectRestInForLoopInit(init js_ast.Stmt, body *js_ast.Stmt) {
	if !p.options.unsupportedJSFeatures.Has(compat.ObjectRestSpread) {
		return
	}

	var bodyPrefixStmt js_ast.Stmt

	switch s := init.Data.(type) {
	case *js_ast.SExpr:
		// "for ({...x} in y) {}"
		// "for ({...x} of y) {}"
		if exprHasObjectRest(s.Value) {
			ref := p.generateTempRef(tempRefNeedsDeclare, "")
			if expr, ok := p.lowerAssign(s.Value, js_ast.Expr{Loc: init.Loc, Data: &js_ast.EIdentifier{Ref: ref}}, objRestReturnValueIsUnused); ok {
				p.recordUsage(ref)
				s.Value.Data = &js_ast.EIdentifier{Ref: ref}
				bodyPrefixStmt = js_ast.Stmt{Loc: expr.Loc, Data: &js_ast.SExpr{Value: expr}}
			}
		}

	case *js_ast.SLocal:
		// "for (let {...x} in y) {}"
		// "for (let {...x} of y) {}"
		if len(s.Decls) == 1 && bindingHasObjectRest(s.Decls[0].Binding) {
			ref := p.generateTempRef(tempRefNoDeclare, "")
			decl := js_ast.Decl{Binding: s.Decls[0].Binding, ValueOrNil: js_ast.Expr{Loc: init.Loc, Data: &js_ast.EIdentifier{Ref: ref}}}
			p.recordUsage(ref)
			decls := p.lowerObjectRestInDecls([]js_ast.Decl{decl})
			s.Decls[0].Binding.Data = &js_ast.BIdentifier{Ref: ref}
			bodyPrefixStmt = js_ast.Stmt{Loc: init.Loc, Data: &js_ast.SLocal{Kind: s.Kind, Decls: decls}}
		}
	}

	if bodyPrefixStmt.Data != nil {
		if block, ok := body.Data.(*js_ast.SBlock); ok {
			// If there's already a block, insert at the front
			stmts := make([]js_ast.Stmt, 0, 1+len(block.Stmts))
			block.Stmts = append(append(stmts, bodyPrefixStmt), block.Stmts...)
		} else {
			// Otherwise, make a block and insert at the front
			body.Data = &js_ast.SBlock{Stmts: []js_ast.Stmt{bodyPrefixStmt, *body}}
		}
	}
}

func (p *parser) lowerObjectRestInCatchBinding(catch *js_ast.Catch) {
	if !p.options.unsupportedJSFeatures.Has(compat.ObjectRestSpread) {
		return
	}

	if catch.BindingOrNil.Data != nil && bindingHasObjectRest(catch.BindingOrNil) {
		ref := p.generateTempRef(tempRefNoDeclare, "")
		decl := js_ast.Decl{Binding: catch.BindingOrNil, ValueOrNil: js_ast.Expr{Loc: catch.BindingOrNil.Loc, Data: &js_ast.EIdentifier{Ref: ref}}}
		p.recordUsage(ref)
		decls := p.lowerObjectRestInDecls([]js_ast.Decl{decl})
		catch.BindingOrNil.Data = &js_ast.BIdentifier{Ref: ref}
		stmts := make([]js_ast.Stmt, 0, 1+len(catch.Block.Stmts))
		stmts = append(stmts, js_ast.Stmt{Loc: catch.BindingOrNil.Loc, Data: &js_ast.SLocal{Kind: js_ast.LocalLet, Decls: decls}})
		catch.Block.Stmts = append(stmts, catch.Block.Stmts...)
	}
}

type objRestMode uint8

const (
	objRestReturnValueIsUnused objRestMode = iota
	objRestMustReturnInitExpr
)

func (p *parser) lowerAssign(rootExpr js_ast.Expr, rootInit js_ast.Expr, mode objRestMode) (js_ast.Expr, bool) {
	rootExpr, didLower := p.lowerSuperPropertyOrPrivateInAssign(rootExpr)

	var expr js_ast.Expr
	assign := func(left js_ast.Expr, right js_ast.Expr) {
		expr = js_ast.JoinWithComma(expr, js_ast.Assign(left, right))
	}

	if initWrapFunc, ok := p.lowerObjectRestHelper(rootExpr, rootInit, assign, tempRefNeedsDeclare, mode); ok {
		if initWrapFunc != nil {
			expr = initWrapFunc(expr)
		}
		return expr, true
	}

	if didLower {
		return js_ast.Assign(rootExpr, rootInit), true
	}

	return js_ast.Expr{}, false
}

func (p *parser) lowerObjectRestToDecls(rootExpr js_ast.Expr, rootInit js_ast.Expr, decls []js_ast.Decl) ([]js_ast.Decl, bool) {
	assign := func(left js_ast.Expr, right js_ast.Expr) {
		binding, invalidLog := p.convertExprToBinding(left, invalidLog{})
		if len(invalidLog.invalidTokens) > 0 {
			panic("Internal error")
		}
		decls = append(decls, js_ast.Decl{Binding: binding, ValueOrNil: right})
	}

	if _, ok := p.lowerObjectRestHelper(rootExpr, rootInit, assign, tempRefNoDeclare, objRestReturnValueIsUnused); ok {
		return decls, true
	}

	return nil, false
}

func (p *parser) lowerObjectRestHelper(
	rootExpr js_ast.Expr,
	rootInit js_ast.Expr,
	assign func(js_ast.Expr, js_ast.Expr),
	declare generateTempRefArg,
	mode objRestMode,
) (wrapFunc func(js_ast.Expr) js_ast.Expr, ok bool) {
	if !p.options.unsupportedJSFeatures.Has(compat.ObjectRestSpread) {
		return nil, false
	}

	// Check if this could possibly contain an object rest binding
	switch rootExpr.Data.(type) {
	case *js_ast.EArray, *js_ast.EObject:
	default:
		return nil, false
	}

	// Scan for object rest bindings and initialize rest binding containment
	containsRestBinding := make(map[js_ast.E]bool)
	var findRestBindings func(js_ast.Expr) bool
	findRestBindings = func(expr js_ast.Expr) bool {
		found := false
		switch e := expr.Data.(type) {
		case *js_ast.EBinary:
			if e.Op == js_ast.BinOpAssign && findRestBindings(e.Left) {
				found = true
			}
		case *js_ast.EArray:
			for _, item := range e.Items {
				if findRestBindings(item) {
					found = true
				}
			}
		case *js_ast.EObject:
			for _, property := range e.Properties {
				if property.Kind == js_ast.PropertySpread || findRestBindings(property.ValueOrNil) {
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
		return nil, false
	}

	// If there is at least one rest binding, lower the whole expression
	var visit func(js_ast.Expr, js_ast.Expr, []func() js_ast.Expr)

	captureIntoRef := func(expr js_ast.Expr) ast.Ref {
		ref := p.generateTempRef(declare, "")
		assign(js_ast.Expr{Loc: expr.Loc, Data: &js_ast.EIdentifier{Ref: ref}}, expr)
		p.recordUsage(ref)
		return ref
	}

	lowerObjectRestPattern := func(
		before []js_ast.Property,
		binding js_ast.Expr,
		init js_ast.Expr,
		capturedKeys []func() js_ast.Expr,
		isSingleLine bool,
	) {
		// If there are properties before this one, store the initializer in a
		// temporary so we can reference it multiple times, then create a new
		// destructuring assignment for these properties
		if len(before) > 0 {
			// "let {a, ...b} = c"
			ref := captureIntoRef(init)
			assign(js_ast.Expr{Loc: before[0].Key.Loc, Data: &js_ast.EObject{Properties: before, IsSingleLine: isSingleLine}},
				js_ast.Expr{Loc: init.Loc, Data: &js_ast.EIdentifier{Ref: ref}})
			init = js_ast.Expr{Loc: init.Loc, Data: &js_ast.EIdentifier{Ref: ref}}
			p.recordUsage(ref)
			p.recordUsage(ref)
		}

		// Call "__objRest" to clone the initializer without the keys for previous
		// properties, then assign the result to the binding for the rest pattern
		keysToExclude := make([]js_ast.Expr, len(capturedKeys))
		for i, capturedKey := range capturedKeys {
			keysToExclude[i] = capturedKey()
		}
		assign(binding, p.callRuntime(binding.Loc, "__objRest", []js_ast.Expr{init,
			{Loc: binding.Loc, Data: &js_ast.EArray{Items: keysToExclude, IsSingleLine: isSingleLine}}}))
	}

	splitArrayPattern := func(
		before []js_ast.Expr,
		split js_ast.Expr,
		after []js_ast.Expr,
		init js_ast.Expr,
		isSingleLine bool,
	) {
		// If this has a default value, skip the value to target the binding
		binding := &split
		if binary, ok := binding.Data.(*js_ast.EBinary); ok && binary.Op == js_ast.BinOpAssign {
			binding = &binary.Left
		}

		// Swap the binding with a temporary
		splitRef := p.generateTempRef(declare, "")
		deferredBinding := *binding
		binding.Data = &js_ast.EIdentifier{Ref: splitRef}
		items := append(before, split)

		// If there are any items left over, defer them until later too
		var tailExpr js_ast.Expr
		var tailInit js_ast.Expr
		if len(after) > 0 {
			tailRef := p.generateTempRef(declare, "")
			loc := after[0].Loc
			tailExpr = js_ast.Expr{Loc: loc, Data: &js_ast.EArray{Items: after, IsSingleLine: isSingleLine}}
			tailInit = js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: tailRef}}
			items = append(items, js_ast.Expr{Loc: loc, Data: &js_ast.ESpread{Value: js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: tailRef}}}})
			p.recordUsage(tailRef)
			p.recordUsage(tailRef)
		}

		// The original destructuring assignment must come first
		assign(js_ast.Expr{Loc: split.Loc, Data: &js_ast.EArray{Items: items, IsSingleLine: isSingleLine}}, init)

		// Then the deferred split is evaluated
		visit(deferredBinding, js_ast.Expr{Loc: split.Loc, Data: &js_ast.EIdentifier{Ref: splitRef}}, nil)
		p.recordUsage(splitRef)

		// Then anything after the split
		if len(after) > 0 {
			visit(tailExpr, tailInit, nil)
		}
	}

	splitObjectPattern := func(
		upToSplit []js_ast.Property,
		afterSplit []js_ast.Property,
		init js_ast.Expr,
		capturedKeys []func() js_ast.Expr,
		isSingleLine bool,
	) {
		// If there are properties after the split, store the initializer in a
		// temporary so we can reference it multiple times
		var afterSplitInit js_ast.Expr
		if len(afterSplit) > 0 {
			ref := captureIntoRef(init)
			init = js_ast.Expr{Loc: init.Loc, Data: &js_ast.EIdentifier{Ref: ref}}
			afterSplitInit = js_ast.Expr{Loc: init.Loc, Data: &js_ast.EIdentifier{Ref: ref}}
		}

		split := &upToSplit[len(upToSplit)-1]
		binding := &split.ValueOrNil

		// Swap the binding with a temporary
		splitRef := p.generateTempRef(declare, "")
		deferredBinding := *binding
		binding.Data = &js_ast.EIdentifier{Ref: splitRef}
		p.recordUsage(splitRef)

		// Use a destructuring assignment to unpack everything up to and including
		// the split point
		assign(js_ast.Expr{Loc: binding.Loc, Data: &js_ast.EObject{Properties: upToSplit, IsSingleLine: isSingleLine}}, init)

		// Handle any nested rest binding patterns inside the split point
		visit(deferredBinding, js_ast.Expr{Loc: binding.Loc, Data: &js_ast.EIdentifier{Ref: splitRef}}, nil)
		p.recordUsage(splitRef)

		// Then continue on to any properties after the split
		if len(afterSplit) > 0 {
			visit(js_ast.Expr{Loc: binding.Loc, Data: &js_ast.EObject{
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
	visit = func(expr js_ast.Expr, init js_ast.Expr, capturedKeys []func() js_ast.Expr) {
		switch e := expr.Data.(type) {
		case *js_ast.EArray:
			// Split on the first binding with a nested rest binding pattern
			for i, item := range e.Items {
				// "let [a, {...b}, c] = d"
				if containsRestBinding[item.Data] {
					splitArrayPattern(e.Items[:i], item, append([]js_ast.Expr{}, e.Items[i+1:]...), init, e.IsSingleLine)
					return
				}
			}

		case *js_ast.EObject:
			last := len(e.Properties) - 1
			endsWithRestBinding := last >= 0 && e.Properties[last].Kind == js_ast.PropertySpread

			// Split on the first binding with a nested rest binding pattern
			for i := range e.Properties {
				property := &e.Properties[i]

				// "let {a, ...b} = c"
				if property.Kind == js_ast.PropertySpread {
					lowerObjectRestPattern(e.Properties[:i], property.ValueOrNil, init, capturedKeys, e.IsSingleLine)
					return
				}

				// Save a copy of this key so the rest binding can exclude it
				if endsWithRestBinding {
					key, capturedKey := p.captureKeyForObjectRest(property.Key)
					property.Key = key
					capturedKeys = append(capturedKeys, capturedKey)
				}

				// "let {a: {...b}, c} = d"
				if containsRestBinding[property.ValueOrNil.Data] {
					splitObjectPattern(e.Properties[:i+1], e.Properties[i+1:], init, capturedKeys, e.IsSingleLine)
					return
				}
			}
		}

		assign(expr, init)
	}

	// Capture and return the value of the initializer if this is an assignment
	// expression and the return value is used:
	//
	//   // Input:
	//   console.log({...x} = x);
	//
	//   // Output:
	//   var _a;
	//   console.log((x = __objRest(_a = x, []), _a));
	//
	// This isn't necessary if the return value is unused:
	//
	//   // Input:
	//   ({...x} = x);
	//
	//   // Output:
	//   x = __objRest(x, []);
	//
	if mode == objRestMustReturnInitExpr {
		initFunc, initWrapFunc := p.captureValueWithPossibleSideEffects(rootInit.Loc, 2, rootInit, valueCouldBeMutated)
		rootInit = initFunc()
		wrapFunc = func(expr js_ast.Expr) js_ast.Expr {
			return initWrapFunc(js_ast.JoinWithComma(expr, initFunc()))
		}
	}

	visit(rootExpr, rootInit, nil)
	return wrapFunc, true
}

// Save a copy of the key for the call to "__objRest" later on. Certain
// expressions can be converted to keys more efficiently than others.
func (p *parser) captureKeyForObjectRest(originalKey js_ast.Expr) (finalKey js_ast.Expr, capturedKey func() js_ast.Expr) {
	loc := originalKey.Loc
	finalKey = originalKey

	switch k := originalKey.Data.(type) {
	case *js_ast.EString:
		capturedKey = func() js_ast.Expr { return js_ast.Expr{Loc: loc, Data: &js_ast.EString{Value: k.Value}} }

	case *js_ast.ENumber:
		// Emit it as the number plus a string (i.e. call toString() on it).
		// It's important to do it this way instead of trying to print the
		// float as a string because Go's floating-point printer doesn't
		// behave exactly the same as JavaScript and if they are different,
		// the generated code will be wrong.
		capturedKey = func() js_ast.Expr {
			return js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
				Op:    js_ast.BinOpAdd,
				Left:  js_ast.Expr{Loc: loc, Data: &js_ast.ENumber{Value: k.Value}},
				Right: js_ast.Expr{Loc: loc, Data: &js_ast.EString{}},
			}}
		}

	case *js_ast.EIdentifier:
		capturedKey = func() js_ast.Expr {
			p.recordUsage(k.Ref)
			return p.callRuntime(loc, "__restKey", []js_ast.Expr{{Loc: loc, Data: &js_ast.EIdentifier{Ref: k.Ref}}})
		}

	default:
		// If it's an arbitrary expression, it probably has a side effect.
		// Stash it in a temporary reference so we don't evaluate it twice.
		tempRef := p.generateTempRef(tempRefNeedsDeclare, "")
		finalKey = js_ast.Assign(js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: tempRef}}, originalKey)
		capturedKey = func() js_ast.Expr {
			p.recordUsage(tempRef)
			return p.callRuntime(loc, "__restKey", []js_ast.Expr{{Loc: loc, Data: &js_ast.EIdentifier{Ref: tempRef}}})
		}
	}

	return
}

func (p *parser) lowerTemplateLiteral(loc logger.Loc, e *js_ast.ETemplate, tagThisFunc func() js_ast.Expr, tagWrapFunc func(js_ast.Expr) js_ast.Expr) js_ast.Expr {
	// If there is no tag, turn this into normal string concatenation
	if e.TagOrNil.Data == nil {
		var value js_ast.Expr

		// Handle the head
		value = js_ast.Expr{Loc: loc, Data: &js_ast.EString{
			Value:          e.HeadCooked,
			LegacyOctalLoc: e.LegacyOctalLoc,
		}}

		// Handle the tail. Each one is handled with a separate call to ".concat()"
		// to handle various corner cases in the specification including:
		//
		//   * For objects, "toString" must be called instead of "valueOf"
		//   * Side effects must happen inline instead of at the end
		//   * Passing a "Symbol" instance should throw
		//
		for _, part := range e.Parts {
			var args []js_ast.Expr
			if len(part.TailCooked) > 0 {
				args = []js_ast.Expr{part.Value, {Loc: part.TailLoc, Data: &js_ast.EString{Value: part.TailCooked}}}
			} else {
				args = []js_ast.Expr{part.Value}
			}
			value = js_ast.Expr{Loc: loc, Data: &js_ast.ECall{
				Target: js_ast.Expr{Loc: loc, Data: &js_ast.EDot{
					Target:  value,
					Name:    "concat",
					NameLoc: part.Value.Loc,
				}},
				Args: args,
				Kind: js_ast.TargetWasOriginallyPropertyAccess,
			}}
		}

		return value
	}

	// Otherwise, call the tag with the template object
	needsRaw := false
	cooked := []js_ast.Expr{}
	raw := []js_ast.Expr{}
	args := make([]js_ast.Expr, 0, 1+len(e.Parts))
	args = append(args, js_ast.Expr{})

	// Handle the head
	if e.HeadCooked == nil {
		cooked = append(cooked, js_ast.Expr{Loc: e.HeadLoc, Data: js_ast.EUndefinedShared})
		needsRaw = true
	} else {
		cooked = append(cooked, js_ast.Expr{Loc: e.HeadLoc, Data: &js_ast.EString{Value: e.HeadCooked}})
		if !helpers.UTF16EqualsString(e.HeadCooked, e.HeadRaw) {
			needsRaw = true
		}
	}
	raw = append(raw, js_ast.Expr{Loc: e.HeadLoc, Data: &js_ast.EString{Value: helpers.StringToUTF16(e.HeadRaw)}})

	// Handle the tail
	for _, part := range e.Parts {
		args = append(args, part.Value)
		if part.TailCooked == nil {
			cooked = append(cooked, js_ast.Expr{Loc: part.TailLoc, Data: js_ast.EUndefinedShared})
			needsRaw = true
		} else {
			cooked = append(cooked, js_ast.Expr{Loc: part.TailLoc, Data: &js_ast.EString{Value: part.TailCooked}})
			if !helpers.UTF16EqualsString(part.TailCooked, part.TailRaw) {
				needsRaw = true
			}
		}
		raw = append(raw, js_ast.Expr{Loc: part.TailLoc, Data: &js_ast.EString{Value: helpers.StringToUTF16(part.TailRaw)}})
	}

	// Construct the template object
	cookedArray := js_ast.Expr{Loc: e.HeadLoc, Data: &js_ast.EArray{Items: cooked, IsSingleLine: true}}
	var arrays []js_ast.Expr
	if needsRaw {
		arrays = []js_ast.Expr{cookedArray, {Loc: e.HeadLoc, Data: &js_ast.EArray{Items: raw, IsSingleLine: true}}}
	} else {
		arrays = []js_ast.Expr{cookedArray}
	}
	templateObj := p.callRuntime(e.HeadLoc, "__template", arrays)

	// Cache it in a temporary object (required by the specification)
	tempRef := p.generateTopLevelTempRef()
	p.recordUsage(tempRef)
	p.recordUsage(tempRef)
	args[0] = js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
		Op:   js_ast.BinOpLogicalOr,
		Left: js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: tempRef}},
		Right: js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
			Op:    js_ast.BinOpAssign,
			Left:  js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: tempRef}},
			Right: templateObj,
		}},
	}}

	// If this optional chain was used as a template tag, then also forward the value for "this"
	if tagThisFunc != nil {
		return tagWrapFunc(js_ast.Expr{Loc: loc, Data: &js_ast.ECall{
			Target: js_ast.Expr{Loc: loc, Data: &js_ast.EDot{
				Target:  e.TagOrNil,
				Name:    "call",
				NameLoc: e.HeadLoc,
			}},
			Args: append([]js_ast.Expr{tagThisFunc()}, args...),
			Kind: js_ast.TargetWasOriginallyPropertyAccess,
		}})
	}

	// Call the tag function
	kind := js_ast.NormalCall
	if e.TagWasOriginallyPropertyAccess {
		kind = js_ast.TargetWasOriginallyPropertyAccess
	}
	return js_ast.Expr{Loc: loc, Data: &js_ast.ECall{
		Target: e.TagOrNil,
		Args:   args,
		Kind:   kind,
	}}
}

func couldPotentiallyThrow(data js_ast.E) bool {
	switch data.(type) {
	case *js_ast.ENull, *js_ast.EUndefined, *js_ast.EBoolean, *js_ast.ENumber,
		*js_ast.EBigInt, *js_ast.EString, *js_ast.EFunction, *js_ast.EArrow:
		return false
	}
	return true
}

func (p *parser) maybeLowerSetBinOp(left js_ast.Expr, op js_ast.OpCode, right js_ast.Expr) js_ast.Expr {
	if target, loc, private := p.extractPrivateIndex(left); private != nil {
		return p.lowerPrivateSetBinOp(target, loc, private, op, right)
	}
	if property := p.extractSuperProperty(left); property.Data != nil {
		return p.lowerSuperPropertySetBinOp(left.Loc, property, op, right)
	}
	return js_ast.Expr{}
}

func (p *parser) shouldLowerUsingDeclarations(stmts []js_ast.Stmt) bool {
	for _, stmt := range stmts {
		if local, ok := stmt.Data.(*js_ast.SLocal); ok &&
			((local.Kind == js_ast.LocalUsing && p.options.unsupportedJSFeatures.Has(compat.Using)) ||
				(local.Kind == js_ast.LocalAwaitUsing && (p.options.unsupportedJSFeatures.Has(compat.Using) ||
					p.options.unsupportedJSFeatures.Has(compat.AsyncAwait) ||
					(p.options.unsupportedJSFeatures.Has(compat.AsyncGenerator) && p.fnOrArrowDataVisit.isGenerator)))) {
			return true
		}
	}
	return false
}

type lowerUsingDeclarationContext struct {
	firstUsingLoc logger.Loc
	stackRef      ast.Ref
	hasAwaitUsing bool
}

func (p *parser) lowerUsingDeclarationContext() lowerUsingDeclarationContext {
	return lowerUsingDeclarationContext{
		stackRef: p.newSymbol(ast.SymbolOther, "_stack"),
	}
}

// If this returns "nil", then no lowering needed to be done
func (ctx *lowerUsingDeclarationContext) scanStmts(p *parser, stmts []js_ast.Stmt) {
	for _, stmt := range stmts {
		if local, ok := stmt.Data.(*js_ast.SLocal); ok && local.Kind.IsUsing() {
			// Wrap each "using" initializer in a call to the "__using" helper function
			if ctx.firstUsingLoc.Start == 0 {
				ctx.firstUsingLoc = stmt.Loc
			}
			if local.Kind == js_ast.LocalAwaitUsing {
				ctx.hasAwaitUsing = true
			}
			for i, decl := range local.Decls {
				if decl.ValueOrNil.Data != nil {
					valueLoc := decl.ValueOrNil.Loc
					p.recordUsage(ctx.stackRef)
					args := []js_ast.Expr{
						{Loc: valueLoc, Data: &js_ast.EIdentifier{Ref: ctx.stackRef}},
						decl.ValueOrNil,
					}
					if local.Kind == js_ast.LocalAwaitUsing {
						args = append(args, js_ast.Expr{Loc: valueLoc, Data: &js_ast.EBoolean{Value: true}})
					}
					local.Decls[i].ValueOrNil = p.callRuntime(valueLoc, "__using", args)
				}
			}
			if p.willWrapModuleInTryCatchForUsing && p.currentScope.Parent == nil {
				local.Kind = js_ast.LocalVar
			} else {
				local.Kind = p.selectLocalKind(js_ast.LocalConst)
			}
		}
	}
}

func (ctx *lowerUsingDeclarationContext) finalize(p *parser, stmts []js_ast.Stmt, shouldHoistFunctions bool) []js_ast.Stmt {
	var result []js_ast.Stmt
	var exports []js_ast.ClauseItem
	end := 0

	// Filter out statements that can't go in a try/catch block
	for _, stmt := range stmts {
		switch s := stmt.Data.(type) {
		// Note: We don't need to handle class declarations here because they
		// should have been already converted into local "var" declarations
		// before this point. It's done in "lowerClass" instead of here because
		// "lowerClass" already does this sometimes for other reasons, and it's
		// more straightforward to do it in one place because it's complicated.

		case *js_ast.SDirective, *js_ast.SImport, *js_ast.SExportFrom, *js_ast.SExportStar:
			// These can't go in a try/catch block
			result = append(result, stmt)
			continue

		case *js_ast.SExportClause:
			// Merge export clauses together
			exports = append(exports, s.Items...)
			continue

		case *js_ast.SFunction:
			if shouldHoistFunctions {
				// Hoist function declarations for cross-file ESM references
				result = append(result, stmt)
				continue
			}

		case *js_ast.SExportDefault:
			if _, ok := s.Value.Data.(*js_ast.SFunction); ok && shouldHoistFunctions {
				// Hoist function declarations for cross-file ESM references
				result = append(result, stmt)
				continue
			}

		case *js_ast.SLocal:
			// If any of these are exported, turn it into a "var" and add export clauses
			if s.IsExport {
				js_ast.ForEachIdentifierBindingInDecls(s.Decls, func(loc logger.Loc, b *js_ast.BIdentifier) {
					exports = append(exports, js_ast.ClauseItem{
						Alias:    p.symbols[b.Ref.InnerIndex].OriginalName,
						AliasLoc: loc,
						Name:     ast.LocRef{Loc: loc, Ref: b.Ref},
					})
					s.Kind = js_ast.LocalVar
				})
				s.IsExport = false
			}
		}

		stmts[end] = stmt
		end++
	}
	stmts = stmts[:end]

	// Generate the variables we'll need
	caughtRef := p.newSymbol(ast.SymbolOther, "_")
	errorRef := p.newSymbol(ast.SymbolOther, "_error")
	hasErrorRef := p.newSymbol(ast.SymbolOther, "_hasError")

	// Generated variables are declared with "var", so hoist them up
	scope := p.currentScope
	for !scope.Kind.StopsHoisting() {
		scope = scope.Parent
	}
	isTopLevel := scope == p.moduleScope
	scope.Generated = append(scope.Generated, ctx.stackRef, caughtRef, errorRef, hasErrorRef)
	p.declaredSymbols = append(p.declaredSymbols,
		js_ast.DeclaredSymbol{IsTopLevel: isTopLevel, Ref: ctx.stackRef},
		js_ast.DeclaredSymbol{IsTopLevel: isTopLevel, Ref: caughtRef},
		js_ast.DeclaredSymbol{IsTopLevel: isTopLevel, Ref: errorRef},
		js_ast.DeclaredSymbol{IsTopLevel: isTopLevel, Ref: hasErrorRef},
	)

	// Call the "__callDispose" helper function at the end of the scope
	loc := ctx.firstUsingLoc
	p.recordUsage(ctx.stackRef)
	p.recordUsage(errorRef)
	p.recordUsage(hasErrorRef)
	callDispose := p.callRuntime(loc, "__callDispose", []js_ast.Expr{
		{Loc: loc, Data: &js_ast.EIdentifier{Ref: ctx.stackRef}},
		{Loc: loc, Data: &js_ast.EIdentifier{Ref: errorRef}},
		{Loc: loc, Data: &js_ast.EIdentifier{Ref: hasErrorRef}},
	})

	// If there was an "await using", optionally await the returned promise
	var finallyStmts []js_ast.Stmt
	if ctx.hasAwaitUsing {
		promiseRef := p.generateTempRef(tempRefNoDeclare, "_promise")
		scope.Generated = append(scope.Generated, promiseRef)
		p.declaredSymbols = append(p.declaredSymbols, js_ast.DeclaredSymbol{IsTopLevel: isTopLevel, Ref: promiseRef})

		// "await" expressions turn into "yield" expressions when lowering
		p.recordUsage(promiseRef)
		awaitExpr := p.maybeLowerAwait(loc, &js_ast.EAwait{Value: js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: promiseRef}}})

		p.recordUsage(promiseRef)
		finallyStmts = []js_ast.Stmt{
			{Loc: loc, Data: &js_ast.SLocal{Decls: []js_ast.Decl{{
				Binding:    js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: promiseRef}},
				ValueOrNil: callDispose,
			}}}},

			// The "await" must not happen if an error was thrown before the
			// "await using", so we conditionally await here:
			//
			//   var promise = __callDispose(stack, error, hasError);
			//   promise && await promise;
			//
			{Loc: loc, Data: &js_ast.SExpr{Value: js_ast.Expr{Loc: loc, Data: &js_ast.EBinary{
				Op:    js_ast.BinOpLogicalAnd,
				Left:  js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: promiseRef}},
				Right: awaitExpr,
			}}}},
		}
	} else {
		finallyStmts = []js_ast.Stmt{{Loc: loc, Data: &js_ast.SExpr{Value: callDispose}}}
	}

	// Wrap everything in a try/catch/finally block
	p.recordUsage(caughtRef)
	result = append(result,
		js_ast.Stmt{Loc: loc, Data: &js_ast.SLocal{
			Decls: []js_ast.Decl{{
				Binding:    js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: ctx.stackRef}},
				ValueOrNil: js_ast.Expr{Loc: loc, Data: &js_ast.EArray{}},
			}},
		}},
		js_ast.Stmt{Loc: loc, Data: &js_ast.STry{
			Block: js_ast.SBlock{
				Stmts: stmts,
			},
			BlockLoc: loc,
			Catch: &js_ast.Catch{
				Loc:          loc,
				BindingOrNil: js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: caughtRef}},
				Block: js_ast.SBlock{Stmts: []js_ast.Stmt{{Loc: loc, Data: &js_ast.SLocal{
					Decls: []js_ast.Decl{{
						Binding:    js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: errorRef}},
						ValueOrNil: js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: caughtRef}},
					}, {
						Binding:    js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: hasErrorRef}},
						ValueOrNil: js_ast.Expr{Loc: loc, Data: &js_ast.EBoolean{Value: true}},
					}},
				}}}},
				BlockLoc: loc,
			},
			Finally: &js_ast.Finally{
				Loc:   loc,
				Block: js_ast.SBlock{Stmts: finallyStmts},
			},
		}},
	)
	if len(exports) > 0 {
		result = append(result, js_ast.Stmt{Loc: loc, Data: &js_ast.SExportClause{Items: exports}})
	}
	return result
}

func (p *parser) lowerUsingDeclarationInForOf(loc logger.Loc, init *js_ast.SLocal, body *js_ast.Stmt) {
	binding := init.Decls[0].Binding
	id := binding.Data.(*js_ast.BIdentifier)
	tempRef := p.generateTempRef(tempRefNoDeclare, "_"+p.symbols[id.Ref.InnerIndex].OriginalName)
	block, ok := body.Data.(*js_ast.SBlock)
	if !ok {
		block = &js_ast.SBlock{}
		if _, ok := body.Data.(*js_ast.SEmpty); !ok {
			block.Stmts = append(block.Stmts, *body)
		}
		body.Data = block
	}
	blockStmts := make([]js_ast.Stmt, 0, 1+len(block.Stmts))
	blockStmts = append(blockStmts, js_ast.Stmt{Loc: loc, Data: &js_ast.SLocal{
		Kind: init.Kind,
		Decls: []js_ast.Decl{{
			Binding:    js_ast.Binding{Loc: binding.Loc, Data: &js_ast.BIdentifier{Ref: id.Ref}},
			ValueOrNil: js_ast.Expr{Loc: binding.Loc, Data: &js_ast.EIdentifier{Ref: tempRef}},
		}},
	}})
	blockStmts = append(blockStmts, block.Stmts...)
	ctx := p.lowerUsingDeclarationContext()
	ctx.scanStmts(p, blockStmts)
	block.Stmts = ctx.finalize(p, blockStmts, p.willWrapModuleInTryCatchForUsing && p.currentScope.Parent == nil)
	init.Kind = js_ast.LocalVar
	id.Ref = tempRef
}

func (p *parser) maybeLowerUsingDeclarationsInSwitch(loc logger.Loc, s *js_ast.SSwitch) []js_ast.Stmt {
	// Check for a "using" declaration in any case
	shouldLower := false
	for _, c := range s.Cases {
		if p.shouldLowerUsingDeclarations(c.Body) {
			shouldLower = true
			break
		}
	}
	if !shouldLower {
		return nil
	}

	// If we find one, lower all cases together
	ctx := p.lowerUsingDeclarationContext()
	for _, c := range s.Cases {
		ctx.scanStmts(p, c.Body)
	}
	return ctx.finalize(p, []js_ast.Stmt{{Loc: loc, Data: s}}, false)
}
