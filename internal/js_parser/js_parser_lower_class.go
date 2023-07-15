package js_parser

import (
	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/logger"
)

func (p *parser) privateSymbolNeedsToBeLowered(private *js_ast.EPrivateIdentifier) bool {
	symbol := &p.symbols[private.Ref.InnerIndex]
	return p.options.unsupportedJSFeatures.Has(compat.SymbolFeature(symbol.Kind)) || symbol.Flags.Has(ast.PrivateSymbolMustBeLowered)
}

func (p *parser) lowerPrivateBrandCheck(target js_ast.Expr, loc logger.Loc, private *js_ast.EPrivateIdentifier) js_ast.Expr {
	// "#field in this" => "__privateIn(#field, this)"
	return p.callRuntime(loc, "__privateIn", []js_ast.Expr{
		{Loc: loc, Data: &js_ast.EIdentifier{Ref: private.Ref}},
		target,
	})
}

func (p *parser) lowerPrivateGet(target js_ast.Expr, loc logger.Loc, private *js_ast.EPrivateIdentifier) js_ast.Expr {
	switch p.symbols[private.Ref.InnerIndex].Kind {
	case ast.SymbolPrivateMethod, ast.SymbolPrivateStaticMethod:
		// "this.#method" => "__privateMethod(this, #method, method_fn)"
		fnRef := p.privateGetters[private.Ref]
		p.recordUsage(fnRef)
		return p.callRuntime(target.Loc, "__privateMethod", []js_ast.Expr{
			target,
			{Loc: loc, Data: &js_ast.EIdentifier{Ref: private.Ref}},
			{Loc: loc, Data: &js_ast.EIdentifier{Ref: fnRef}},
		})

	case ast.SymbolPrivateGet, ast.SymbolPrivateStaticGet,
		ast.SymbolPrivateGetSetPair, ast.SymbolPrivateStaticGetSetPair:
		// "this.#getter" => "__privateGet(this, #getter, getter_get)"
		fnRef := p.privateGetters[private.Ref]
		p.recordUsage(fnRef)
		return p.callRuntime(target.Loc, "__privateGet", []js_ast.Expr{
			target,
			{Loc: loc, Data: &js_ast.EIdentifier{Ref: private.Ref}},
			{Loc: loc, Data: &js_ast.EIdentifier{Ref: fnRef}},
		})

	default:
		// "this.#field" => "__privateGet(this, #field)"
		return p.callRuntime(target.Loc, "__privateGet", []js_ast.Expr{
			target,
			{Loc: loc, Data: &js_ast.EIdentifier{Ref: private.Ref}},
		})
	}
}

func (p *parser) lowerPrivateSet(
	target js_ast.Expr,
	loc logger.Loc,
	private *js_ast.EPrivateIdentifier,
	value js_ast.Expr,
) js_ast.Expr {
	switch p.symbols[private.Ref.InnerIndex].Kind {
	case ast.SymbolPrivateSet, ast.SymbolPrivateStaticSet,
		ast.SymbolPrivateGetSetPair, ast.SymbolPrivateStaticGetSetPair:
		// "this.#setter = 123" => "__privateSet(this, #setter, 123, setter_set)"
		fnRef := p.privateSetters[private.Ref]
		p.recordUsage(fnRef)
		return p.callRuntime(target.Loc, "__privateSet", []js_ast.Expr{
			target,
			{Loc: loc, Data: &js_ast.EIdentifier{Ref: private.Ref}},
			value,
			{Loc: loc, Data: &js_ast.EIdentifier{Ref: fnRef}},
		})

	default:
		// "this.#field = 123" => "__privateSet(this, #field, 123)"
		return p.callRuntime(target.Loc, "__privateSet", []js_ast.Expr{
			target,
			{Loc: loc, Data: &js_ast.EIdentifier{Ref: private.Ref}},
			value,
		})
	}
}

func (p *parser) lowerPrivateSetUnOp(target js_ast.Expr, loc logger.Loc, private *js_ast.EPrivateIdentifier, op js_ast.OpCode) js_ast.Expr {
	kind := p.symbols[private.Ref.InnerIndex].Kind

	// Determine the setter, if any
	var setter js_ast.Expr
	switch kind {
	case ast.SymbolPrivateSet, ast.SymbolPrivateStaticSet,
		ast.SymbolPrivateGetSetPair, ast.SymbolPrivateStaticGetSetPair:
		ref := p.privateSetters[private.Ref]
		p.recordUsage(ref)
		setter = js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: ref}}
	}

	// Determine the getter, if any
	var getter js_ast.Expr
	switch kind {
	case ast.SymbolPrivateGet, ast.SymbolPrivateStaticGet,
		ast.SymbolPrivateGetSetPair, ast.SymbolPrivateStaticGetSetPair:
		ref := p.privateGetters[private.Ref]
		p.recordUsage(ref)
		getter = js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: ref}}
	}

	// Only include necessary arguments
	args := []js_ast.Expr{
		target,
		{Loc: loc, Data: &js_ast.EIdentifier{Ref: private.Ref}},
	}
	if setter.Data != nil {
		args = append(args, setter)
	}
	if getter.Data != nil {
		if setter.Data == nil {
			args = append(args, js_ast.Expr{Loc: loc, Data: js_ast.ENullShared})
		}
		args = append(args, getter)
	}

	// "target.#private++" => "__privateWrapper(target, #private, private_set, private_get)._++"
	return js_ast.Expr{Loc: loc, Data: &js_ast.EUnary{
		Op: op,
		Value: js_ast.Expr{Loc: target.Loc, Data: &js_ast.EDot{
			Target:  p.callRuntime(target.Loc, "__privateWrapper", args),
			NameLoc: target.Loc,
			Name:    "_",
		}},
	}}
}

func (p *parser) lowerPrivateSetBinOp(target js_ast.Expr, loc logger.Loc, private *js_ast.EPrivateIdentifier, op js_ast.OpCode, value js_ast.Expr) js_ast.Expr {
	// "target.#private += 123" => "__privateSet(target, #private, __privateGet(target, #private) + 123)"
	targetFunc, targetWrapFunc := p.captureValueWithPossibleSideEffects(target.Loc, 2, target, valueDefinitelyNotMutated)
	return targetWrapFunc(p.lowerPrivateSet(targetFunc(), loc, private, js_ast.Expr{Loc: value.Loc, Data: &js_ast.EBinary{
		Op:    op,
		Left:  p.lowerPrivateGet(targetFunc(), loc, private),
		Right: value,
	}}))
}

// Returns valid data if target is an expression of the form "foo.#bar" and if
// the language target is such that private members must be lowered
func (p *parser) extractPrivateIndex(target js_ast.Expr) (js_ast.Expr, logger.Loc, *js_ast.EPrivateIdentifier) {
	if index, ok := target.Data.(*js_ast.EIndex); ok {
		if private, ok := index.Index.Data.(*js_ast.EPrivateIdentifier); ok && p.privateSymbolNeedsToBeLowered(private) {
			return index.Target, index.Index.Loc, private
		}
	}
	return js_ast.Expr{}, logger.Loc{}, nil
}

// Returns a valid property if target is an expression of the form "super.bar"
// or "super[bar]" and if the situation is such that it must be lowered
func (p *parser) extractSuperProperty(target js_ast.Expr) js_ast.Expr {
	switch e := target.Data.(type) {
	case *js_ast.EDot:
		if p.shouldLowerSuperPropertyAccess(e.Target) {
			return js_ast.Expr{Loc: e.NameLoc, Data: &js_ast.EString{Value: helpers.StringToUTF16(e.Name)}}
		}
	case *js_ast.EIndex:
		if p.shouldLowerSuperPropertyAccess(e.Target) {
			return e.Index
		}
	}
	return js_ast.Expr{}
}

func (p *parser) lowerSuperPropertyOrPrivateInAssign(expr js_ast.Expr) (js_ast.Expr, bool) {
	didLower := false

	switch e := expr.Data.(type) {
	case *js_ast.ESpread:
		if value, ok := p.lowerSuperPropertyOrPrivateInAssign(e.Value); ok {
			e.Value = value
			didLower = true
		}

	case *js_ast.EDot:
		// "[super.foo] = [bar]" => "[__superWrapper(this, 'foo')._] = [bar]"
		if p.shouldLowerSuperPropertyAccess(e.Target) {
			key := js_ast.Expr{Loc: e.NameLoc, Data: &js_ast.EString{Value: helpers.StringToUTF16(e.Name)}}
			expr = p.callSuperPropertyWrapper(expr.Loc, key)
			didLower = true
		}

	case *js_ast.EIndex:
		// "[super[foo]] = [bar]" => "[__superWrapper(this, foo)._] = [bar]"
		if p.shouldLowerSuperPropertyAccess(e.Target) {
			expr = p.callSuperPropertyWrapper(expr.Loc, e.Index)
			didLower = true
			break
		}

		// "[a.#b] = [c]" => "[__privateWrapper(a, #b)._] = [c]"
		if private, ok := e.Index.Data.(*js_ast.EPrivateIdentifier); ok && p.privateSymbolNeedsToBeLowered(private) {
			var target js_ast.Expr

			switch p.symbols[private.Ref.InnerIndex].Kind {
			case ast.SymbolPrivateSet, ast.SymbolPrivateStaticSet,
				ast.SymbolPrivateGetSetPair, ast.SymbolPrivateStaticGetSetPair:
				// "this.#setter" => "__privateWrapper(this, #setter, setter_set)"
				fnRef := p.privateSetters[private.Ref]
				p.recordUsage(fnRef)
				target = p.callRuntime(expr.Loc, "__privateWrapper", []js_ast.Expr{
					e.Target,
					{Loc: expr.Loc, Data: &js_ast.EIdentifier{Ref: private.Ref}},
					{Loc: expr.Loc, Data: &js_ast.EIdentifier{Ref: fnRef}},
				})

			default:
				// "this.#field" => "__privateWrapper(this, #field)"
				target = p.callRuntime(expr.Loc, "__privateWrapper", []js_ast.Expr{
					e.Target,
					{Loc: expr.Loc, Data: &js_ast.EIdentifier{Ref: private.Ref}},
				})
			}

			// "__privateWrapper(this, #field)" => "__privateWrapper(this, #field)._"
			expr.Data = &js_ast.EDot{Target: target, Name: "_", NameLoc: expr.Loc}
			didLower = true
		}

	case *js_ast.EArray:
		for i, item := range e.Items {
			if item, ok := p.lowerSuperPropertyOrPrivateInAssign(item); ok {
				e.Items[i] = item
				didLower = true
			}
		}

	case *js_ast.EObject:
		for i, property := range e.Properties {
			if property.ValueOrNil.Data != nil {
				if value, ok := p.lowerSuperPropertyOrPrivateInAssign(property.ValueOrNil); ok {
					e.Properties[i].ValueOrNil = value
					didLower = true
				}
			}
		}
	}

	return expr, didLower
}

func (p *parser) shouldLowerSuperPropertyAccess(expr js_ast.Expr) bool {
	if p.fnOrArrowDataVisit.shouldLowerSuperPropertyAccess {
		_, isSuper := expr.Data.(*js_ast.ESuper)
		return isSuper
	}
	return false
}

func (p *parser) callSuperPropertyWrapper(loc logger.Loc, key js_ast.Expr) js_ast.Expr {
	ref := *p.fnOnlyDataVisit.innerClassNameRef
	p.recordUsage(ref)
	class := js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: ref}}
	this := js_ast.Expr{Loc: loc, Data: js_ast.EThisShared}

	// Handle "this" in lowered static class field initializers
	if p.fnOnlyDataVisit.shouldReplaceThisWithInnerClassNameRef {
		p.recordUsage(ref)
		this.Data = &js_ast.EIdentifier{Ref: ref}
	}

	if !p.fnOnlyDataVisit.isInStaticClassContext {
		// "super.foo" => "__superWrapper(Class.prototype, this, 'foo')._"
		// "super[foo]" => "__superWrapper(Class.prototype, this, foo)._"
		class.Data = &js_ast.EDot{Target: class, NameLoc: loc, Name: "prototype"}
	}

	return js_ast.Expr{Loc: loc, Data: &js_ast.EDot{Target: p.callRuntime(loc, "__superWrapper", []js_ast.Expr{
		class,
		this,
		key,
	}), Name: "_", NameLoc: loc}}
}

func (p *parser) lowerSuperPropertyGet(loc logger.Loc, key js_ast.Expr) js_ast.Expr {
	ref := *p.fnOnlyDataVisit.innerClassNameRef
	p.recordUsage(ref)
	class := js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: ref}}
	this := js_ast.Expr{Loc: loc, Data: js_ast.EThisShared}

	// Handle "this" in lowered static class field initializers
	if p.fnOnlyDataVisit.shouldReplaceThisWithInnerClassNameRef {
		p.recordUsage(ref)
		this.Data = &js_ast.EIdentifier{Ref: ref}
	}

	if !p.fnOnlyDataVisit.isInStaticClassContext {
		// "super.foo" => "__superGet(Class.prototype, this, 'foo')"
		// "super[foo]" => "__superGet(Class.prototype, this, foo)"
		class.Data = &js_ast.EDot{Target: class, NameLoc: loc, Name: "prototype"}
	}

	return p.callRuntime(loc, "__superGet", []js_ast.Expr{
		class,
		this,
		key,
	})
}

func (p *parser) lowerSuperPropertySet(loc logger.Loc, key js_ast.Expr, value js_ast.Expr) js_ast.Expr {
	// "super.foo = bar" => "__superSet(Class, this, 'foo', bar)"
	// "super[foo] = bar" => "__superSet(Class, this, foo, bar)"
	ref := *p.fnOnlyDataVisit.innerClassNameRef
	p.recordUsage(ref)
	class := js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: ref}}
	this := js_ast.Expr{Loc: loc, Data: js_ast.EThisShared}

	// Handle "this" in lowered static class field initializers
	if p.fnOnlyDataVisit.shouldReplaceThisWithInnerClassNameRef {
		p.recordUsage(ref)
		this.Data = &js_ast.EIdentifier{Ref: ref}
	}

	if !p.fnOnlyDataVisit.isInStaticClassContext {
		// "super.foo = bar" => "__superSet(Class.prototype, this, 'foo', bar)"
		// "super[foo] = bar" => "__superSet(Class.prototype, this, foo, bar)"
		class.Data = &js_ast.EDot{Target: class, NameLoc: loc, Name: "prototype"}
	}

	return p.callRuntime(loc, "__superSet", []js_ast.Expr{
		class,
		this,
		key,
		value,
	})
}

func (p *parser) lowerSuperPropertySetBinOp(loc logger.Loc, property js_ast.Expr, op js_ast.OpCode, value js_ast.Expr) js_ast.Expr {
	// "super.foo += bar" => "__superSet(Class, this, 'foo', __superGet(Class, this, 'foo') + bar)"
	// "super[foo] += bar" => "__superSet(Class, this, foo, __superGet(Class, this, foo) + bar)"
	// "super[foo()] += bar" => "__superSet(Class, this, _a = foo(), __superGet(Class, this, _a) + bar)"
	targetFunc, targetWrapFunc := p.captureValueWithPossibleSideEffects(property.Loc, 2, property, valueDefinitelyNotMutated)
	return targetWrapFunc(p.lowerSuperPropertySet(loc, targetFunc(), js_ast.Expr{Loc: value.Loc, Data: &js_ast.EBinary{
		Op:    op,
		Left:  p.lowerSuperPropertyGet(loc, targetFunc()),
		Right: value,
	}}))
}

func (p *parser) maybeLowerSuperPropertyGetInsideCall(call *js_ast.ECall) {
	var key js_ast.Expr

	switch e := call.Target.Data.(type) {
	case *js_ast.EDot:
		// Lower "super.prop" if necessary
		if !p.shouldLowerSuperPropertyAccess(e.Target) {
			return
		}
		key = js_ast.Expr{Loc: e.NameLoc, Data: &js_ast.EString{Value: helpers.StringToUTF16(e.Name)}}

	case *js_ast.EIndex:
		// Lower "super[prop]" if necessary
		if !p.shouldLowerSuperPropertyAccess(e.Target) {
			return
		}
		key = e.Index

	default:
		return
	}

	// "super.foo(a, b)" => "__superGet(Class, this, 'foo').call(this, a, b)"
	call.Target.Data = &js_ast.EDot{
		Target:  p.lowerSuperPropertyGet(call.Target.Loc, key),
		NameLoc: key.Loc,
		Name:    "call",
	}
	thisExpr := js_ast.Expr{Loc: call.Target.Loc, Data: js_ast.EThisShared}
	call.Args = append([]js_ast.Expr{thisExpr}, call.Args...)
}

type classLoweringInfo struct {
	lowerAllInstanceFields bool
	lowerAllStaticFields   bool
	shimSuperCtorCalls     bool
}

func (p *parser) computeClassLoweringInfo(class *js_ast.Class) (result classLoweringInfo) {
	// Name keeping for classes is implemented with a static block. So we need to
	// lower all static fields if static blocks are unsupported so that the name
	// keeping comes first before other static initializers.
	if p.options.keepNames && p.options.unsupportedJSFeatures.Has(compat.ClassStaticBlocks) {
		result.lowerAllStaticFields = true
	}

	// TypeScript's "experimentalDecorators" feature replaces all references of
	// the class name with the decorated class after class decorators have run.
	// This cannot be done by only reassigning to the class symbol in JavaScript
	// because it's shadowed by the class name within the class body. Instead,
	// we need to hoist all code in static contexts out of the class body so
	// that it's no longer shadowed:
	//
	//   const decorate = x => ({ x })
	//   @decorate
	//   class Foo {
	//     static oldFoo = Foo
	//     static newFoo = () => Foo
	//   }
	//   console.log('This must be false:', Foo.x.oldFoo === Foo.x.newFoo())
	//
	if p.options.ts.Parse && p.options.ts.Config.ExperimentalDecorators == config.True && len(class.Decorators) > 0 {
		result.lowerAllStaticFields = true
	}

	// Conservatively lower fields of a given type (instance or static) when any
	// member of that type needs to be lowered. This must be done to preserve
	// evaluation order. For example:
	//
	//   class Foo {
	//     #foo = 123
	//     bar = this.#foo
	//   }
	//
	// It would be bad if we transformed that into something like this:
	//
	//   var _foo;
	//   class Foo {
	//     constructor() {
	//       _foo.set(this, 123);
	//     }
	//     bar = __privateGet(this, _foo);
	//   }
	//   _foo = new WeakMap();
	//
	// That evaluates "bar" then "foo" instead of "foo" then "bar" like the
	// original code. We need to do this instead:
	//
	//   var _foo;
	//   class Foo {
	//     constructor() {
	//       _foo.set(this, 123);
	//       __publicField(this, "bar", __privateGet(this, _foo));
	//     }
	//   }
	//   _foo = new WeakMap();
	//
	for _, prop := range class.Properties {
		if prop.Kind == js_ast.PropertyClassStaticBlock {
			if p.options.unsupportedJSFeatures.Has(compat.ClassStaticBlocks) {
				result.lowerAllStaticFields = true
			}
			continue
		}

		if private, ok := prop.Key.Data.(*js_ast.EPrivateIdentifier); ok {
			if prop.Flags.Has(js_ast.PropertyIsStatic) {
				if p.privateSymbolNeedsToBeLowered(private) {
					result.lowerAllStaticFields = true
				}
			} else {
				if p.privateSymbolNeedsToBeLowered(private) {
					result.lowerAllInstanceFields = true

					// We can't transform this:
					//
					//   class Foo {
					//     #foo = 123
					//     static bar = new Foo().#foo
					//   }
					//
					// into this:
					//
					//   var _foo;
					//   const _Foo = class {
					//     constructor() {
					//       _foo.set(this, 123);
					//     }
					//     static bar = __privateGet(new _Foo(), _foo);
					//   };
					//   let Foo = _Foo;
					//   _foo = new WeakMap();
					//
					// because "_Foo" won't be initialized in the initializer for "bar".
					// So we currently lower all static fields in this case too. This
					// isn't great and it would be good to find a way to avoid this.
					// The inner class name symbol substitution mechanism should probably
					// be rethought.
					result.lowerAllStaticFields = true
				}
			}
			continue
		}

		if prop.Kind == js_ast.PropertyAutoAccessor {
			if prop.Flags.Has(js_ast.PropertyIsStatic) {
				if p.options.unsupportedJSFeatures.Has(compat.ClassPrivateStaticField) {
					result.lowerAllStaticFields = true
				}
			} else {
				if p.options.unsupportedJSFeatures.Has(compat.ClassPrivateField) {
					result.lowerAllInstanceFields = true
					result.lowerAllStaticFields = true
				}
			}
			continue
		}

		// This doesn't come before the private member check above because
		// unsupported private methods must also trigger field lowering:
		//
		//   class Foo {
		//     bar = this.#foo()
		//     #foo() {}
		//   }
		//
		// It would be bad if we transformed that to something like this:
		//
		//   var _foo, foo_fn;
		//   class Foo {
		//     constructor() {
		//       _foo.add(this);
		//     }
		//     bar = __privateMethod(this, _foo, foo_fn).call(this);
		//   }
		//   _foo = new WeakSet();
		//   foo_fn = function() {
		//   };
		//
		// In that case the initializer of "bar" would fail to call "#foo" because
		// it's only added to the instance in the body of the constructor.
		if prop.Flags.Has(js_ast.PropertyIsMethod) {
			// We need to shim "super()" inside the constructor if this is a derived
			// class and the constructor has any parameter properties, since those
			// use "this" and we can only access "this" after "super()" is called
			if class.ExtendsOrNil.Data != nil {
				if key, ok := prop.Key.Data.(*js_ast.EString); ok && helpers.UTF16EqualsString(key.Value, "constructor") {
					if fn, ok := prop.ValueOrNil.Data.(*js_ast.EFunction); ok {
						for _, arg := range fn.Fn.Args {
							if arg.IsTypeScriptCtorField {
								result.shimSuperCtorCalls = true
								break
							}
						}
					}
				}
			}
			continue
		}

		if prop.Flags.Has(js_ast.PropertyIsStatic) {
			// Static fields must be lowered if the target doesn't support them
			if p.options.unsupportedJSFeatures.Has(compat.ClassStaticField) {
				result.lowerAllStaticFields = true
			}

			// Convert static fields to assignment statements if the TypeScript
			// setting for this is enabled. I don't think this matters for private
			// fields because there's no way for this to call a setter in the base
			// class, so this isn't done for private fields.
			//
			// If class static blocks are supported, then we can do this inline
			// without needing to move the initializers outside of the class body.
			// Otherwise, we need to lower all static class fields.
			if p.options.ts.Parse && !class.UseDefineForClassFields && p.options.unsupportedJSFeatures.Has(compat.ClassStaticBlocks) {
				result.lowerAllStaticFields = true
			}
		} else {
			// Instance fields must be lowered if the target doesn't support them
			if p.options.unsupportedJSFeatures.Has(compat.ClassField) {
				result.lowerAllInstanceFields = true
			}

			// Convert instance fields to assignment statements if the TypeScript
			// setting for this is enabled. I don't think this matters for private
			// fields because there's no way for this to call a setter in the base
			// class, so this isn't done for private fields.
			if p.options.ts.Parse && !class.UseDefineForClassFields {
				result.lowerAllInstanceFields = true
			}
		}
	}

	// We need to shim "super()" inside the constructor if this is a derived
	// class and there are any instance fields that need to be lowered, since
	// those use "this" and we can only access "this" after "super()" is called
	if result.lowerAllInstanceFields && class.ExtendsOrNil.Data != nil {
		result.shimSuperCtorCalls = true
	}

	return
}

// Apply all relevant transforms to a class object (either a statement or an
// expression) including:
//
//   - Transforming class fields for older environments
//   - Transforming static blocks for older environments
//   - Transforming TypeScript experimental decorators into JavaScript
//   - Transforming TypeScript class fields into assignments for "useDefineForClassFields"
//
// Note that this doesn't transform any nested AST subtrees inside the class
// body (e.g. the contents of initializers, methods, and static blocks). Those
// have already been transformed by "visitClass" by this point. It's done that
// way for performance so that we don't need to do another AST pass.
func (p *parser) lowerClass(stmt js_ast.Stmt, expr js_ast.Expr, result visitClassResult) ([]js_ast.Stmt, js_ast.Expr) {
	type classKind uint8
	const (
		classKindExpr classKind = iota
		classKindStmt
		classKindExportStmt
		classKindExportDefaultStmt
	)

	// Unpack the class from the statement or expression
	var kind classKind
	var class *js_ast.Class
	var classLoc logger.Loc
	var defaultName ast.LocRef
	if stmt.Data == nil {
		e, _ := expr.Data.(*js_ast.EClass)
		class = &e.Class
		kind = classKindExpr
		if class.Name != nil {
			symbol := &p.symbols[class.Name.Ref.InnerIndex]

			// The inner class name inside the class expression should be the same as
			// the class expression name itself
			if result.innerClassNameRef != ast.InvalidRef {
				p.mergeSymbols(result.innerClassNameRef, class.Name.Ref)
			}

			// Remove unused class names when minifying. Check this after we merge in
			// the inner class name above since that will adjust the use count.
			if p.options.minifySyntax && symbol.UseCountEstimate == 0 {
				class.Name = nil
			}
		}
	} else if s, ok := stmt.Data.(*js_ast.SClass); ok {
		class = &s.Class
		if s.IsExport {
			kind = classKindExportStmt
		} else {
			kind = classKindStmt
		}
	} else {
		s, _ := stmt.Data.(*js_ast.SExportDefault)
		s2, _ := s.Value.Data.(*js_ast.SClass)
		class = &s2.Class
		defaultName = s.DefaultName
		kind = classKindExportDefaultStmt
	}
	if stmt.Data == nil {
		classLoc = expr.Loc
	} else {
		classLoc = stmt.Loc
	}

	var ctor *js_ast.EFunction
	var parameterFields []js_ast.Stmt
	var instanceMembers []js_ast.Stmt
	var instancePrivateMethods []js_ast.Stmt

	// These expressions are generated after the class body, in this order
	var computedPropertyCache js_ast.Expr
	var privateMembers []js_ast.Expr
	var staticMembers []js_ast.Expr
	var staticPrivateMethods []js_ast.Expr
	var instanceDecorators []js_ast.Expr
	var staticDecorators []js_ast.Expr

	// These are only for class expressions that need to be captured
	var nameFunc func() js_ast.Expr
	var wrapFunc func(js_ast.Expr) js_ast.Expr
	didCaptureClassExpr := false

	// Class statements can be missing a name if they are in an
	// "export default" statement:
	//
	//   export default class {
	//     static foo = 123
	//   }
	//
	nameFunc = func() js_ast.Expr {
		if kind == classKindExpr {
			// If this is a class expression, capture and store it. We have to
			// do this even if it has a name since the name isn't exposed
			// outside the class body.
			classExpr := &js_ast.EClass{Class: *class}
			class = &classExpr.Class
			nameFunc, wrapFunc = p.captureValueWithPossibleSideEffects(classLoc, 2, js_ast.Expr{Loc: classLoc, Data: classExpr}, valueDefinitelyNotMutated)
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
				p.mergeSymbols(class.Name.Ref, name.Data.(*js_ast.EIdentifier).Ref)
				class.Name = nil
			}

			return name
		} else {
			// If anything referenced the inner class name, then we should use that
			// name for any automatically-generated initialization code, since it
			// will come before the outer class name is initialized.
			if result.innerClassNameRef != ast.InvalidRef {
				p.recordUsage(result.innerClassNameRef)
				return js_ast.Expr{Loc: class.Name.Loc, Data: &js_ast.EIdentifier{Ref: result.innerClassNameRef}}
			}

			// Otherwise we should just use the outer class name
			if class.Name == nil {
				if kind == classKindExportDefaultStmt {
					class.Name = &defaultName
				} else {
					class.Name = &ast.LocRef{Loc: classLoc, Ref: p.generateTempRef(tempRefNoDeclare, "")}
				}
			}
			p.recordUsage(class.Name.Ref)
			return js_ast.Expr{Loc: class.Name.Loc, Data: &js_ast.EIdentifier{Ref: class.Name.Ref}}
		}
	}

	// Handle lowering of instance and static fields. Move their initializers
	// from the class body to either the constructor (instance fields) or after
	// the class (static fields).
	//
	// If this returns true, the return property should be added to the class
	// body. Otherwise the property should be omitted from the class body.
	lowerField := func(prop js_ast.Property, private *js_ast.EPrivateIdentifier, shouldOmitFieldInitializer bool, staticFieldToBlockAssign bool) (js_ast.Property, bool) {
		mustLowerPrivate := private != nil && p.privateSymbolNeedsToBeLowered(private)

		// The TypeScript compiler doesn't follow the JavaScript spec for
		// uninitialized fields. They are supposed to be set to undefined but the
		// TypeScript compiler just omits them entirely.
		if !shouldOmitFieldInitializer {
			loc := prop.Loc

			// Determine where to store the field
			var target js_ast.Expr
			if prop.Flags.Has(js_ast.PropertyIsStatic) && !staticFieldToBlockAssign {
				target = nameFunc()
			} else {
				target = js_ast.Expr{Loc: loc, Data: js_ast.EThisShared}
			}

			// Generate the assignment initializer
			var init js_ast.Expr
			if prop.InitializerOrNil.Data != nil {
				init = prop.InitializerOrNil
			} else {
				init = js_ast.Expr{Loc: loc, Data: js_ast.EUndefinedShared}
			}

			// Generate the assignment target
			var memberExpr js_ast.Expr
			if mustLowerPrivate {
				// Generate a new symbol for this private field
				ref := p.generateTempRef(tempRefNeedsDeclare, "_"+p.symbols[private.Ref.InnerIndex].OriginalName[1:])
				p.symbols[private.Ref.InnerIndex].Link = ref

				// Initialize the private field to a new WeakMap
				if p.weakMapRef == ast.InvalidRef {
					p.weakMapRef = p.newSymbol(ast.SymbolUnbound, "WeakMap")
					p.moduleScope.Generated = append(p.moduleScope.Generated, p.weakMapRef)
				}
				privateMembers = append(privateMembers, js_ast.Assign(
					js_ast.Expr{Loc: prop.Key.Loc, Data: &js_ast.EIdentifier{Ref: ref}},
					js_ast.Expr{Loc: prop.Key.Loc, Data: &js_ast.ENew{Target: js_ast.Expr{Loc: prop.Key.Loc, Data: &js_ast.EIdentifier{Ref: p.weakMapRef}}}},
				))
				p.recordUsage(ref)

				// Add every newly-constructed instance into this map
				memberExpr = p.callRuntime(loc, "__privateAdd", []js_ast.Expr{
					target,
					{Loc: prop.Key.Loc, Data: &js_ast.EIdentifier{Ref: ref}},
					init,
				})
				p.recordUsage(ref)
			} else if private == nil && class.UseDefineForClassFields {
				var args []js_ast.Expr
				if _, ok := init.Data.(*js_ast.EUndefined); ok {
					args = []js_ast.Expr{target, prop.Key}
				} else {
					args = []js_ast.Expr{target, prop.Key, init}
				}
				memberExpr = js_ast.Expr{Loc: loc, Data: &js_ast.ECall{
					Target: p.importFromRuntime(loc, "__publicField"),
					Args:   args,
				}}
			} else {
				if key, ok := prop.Key.Data.(*js_ast.EString); ok && !prop.Flags.Has(js_ast.PropertyIsComputed) && !prop.Flags.Has(js_ast.PropertyPreferQuotedKey) {
					target = js_ast.Expr{Loc: loc, Data: &js_ast.EDot{
						Target:  target,
						Name:    helpers.UTF16ToString(key.Value),
						NameLoc: prop.Key.Loc,
					}}
				} else {
					target = js_ast.Expr{Loc: loc, Data: &js_ast.EIndex{
						Target: target,
						Index:  prop.Key,
					}}
				}

				memberExpr = js_ast.Assign(target, init)
			}

			if prop.Flags.Has(js_ast.PropertyIsStatic) {
				// Move this property to an assignment after the class ends
				if staticFieldToBlockAssign {
					// Use inline assignment in a static block instead of lowering
					return js_ast.Property{
						Loc:  loc,
						Kind: js_ast.PropertyClassStaticBlock,
						ClassStaticBlock: &js_ast.ClassStaticBlock{
							Loc: loc,
							Block: js_ast.SBlock{Stmts: []js_ast.Stmt{
								{Loc: loc, Data: &js_ast.SExpr{Value: memberExpr}}},
							},
						},
					}, true
				} else {
					// Move this property to an assignment after the class ends
					staticMembers = append(staticMembers, memberExpr)
				}
			} else {
				// Move this property to an assignment inside the class constructor
				instanceMembers = append(instanceMembers, js_ast.Stmt{Loc: loc, Data: &js_ast.SExpr{Value: memberExpr}})
			}
		}

		if private == nil || mustLowerPrivate {
			// Remove the field from the class body
			return js_ast.Property{}, false
		}

		// Keep the private field but remove the initializer
		prop.InitializerOrNil = js_ast.Expr{}
		return prop, true
	}

	// If this returns true, the method property should be dropped as it has
	// already been accounted for elsewhere (e.g. a lowered private method).
	lowerMethod := func(prop js_ast.Property, private *js_ast.EPrivateIdentifier) bool {
		if private != nil && p.privateSymbolNeedsToBeLowered(private) {
			loc := prop.Loc

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
				privateMembers = append(privateMembers, js_ast.Assign(
					js_ast.Expr{Loc: prop.Key.Loc, Data: &js_ast.EIdentifier{Ref: ref}},
					js_ast.Expr{Loc: prop.Key.Loc, Data: &js_ast.ENew{Target: js_ast.Expr{Loc: prop.Key.Loc, Data: &js_ast.EIdentifier{Ref: p.weakSetRef}}}},
				))
				p.recordUsage(ref)

				// Determine where to store the private method
				var target js_ast.Expr
				if prop.Flags.Has(js_ast.PropertyIsStatic) {
					target = nameFunc()
				} else {
					target = js_ast.Expr{Loc: loc, Data: js_ast.EThisShared}
				}

				// Add every newly-constructed instance into this map
				methodExpr := p.callRuntime(loc, "__privateAdd", []js_ast.Expr{
					target,
					{Loc: prop.Key.Loc, Data: &js_ast.EIdentifier{Ref: ref}},
				})
				p.recordUsage(ref)

				// Make sure that adding to the map happens before any field
				// initializers to handle cases like this:
				//
				//   class A {
				//     pub = this.#priv;
				//     #priv() {}
				//   }
				//
				if prop.Flags.Has(js_ast.PropertyIsStatic) {
					// Move this property to an assignment after the class ends
					staticPrivateMethods = append(staticPrivateMethods, methodExpr)
				} else {
					// Move this property to an assignment inside the class constructor
					instancePrivateMethods = append(instancePrivateMethods, js_ast.Stmt{Loc: loc, Data: &js_ast.SExpr{Value: methodExpr}})
				}
			}

			// Move the method definition outside the class body
			methodRef := p.generateTempRef(tempRefNeedsDeclare, "_")
			if prop.Kind == js_ast.PropertySet {
				p.symbols[methodRef.InnerIndex].Link = p.privateSetters[private.Ref]
			} else {
				p.symbols[methodRef.InnerIndex].Link = p.privateGetters[private.Ref]
			}
			p.recordUsage(methodRef)
			privateMembers = append(privateMembers, js_ast.Assign(
				js_ast.Expr{Loc: prop.Key.Loc, Data: &js_ast.EIdentifier{Ref: methodRef}},
				prop.ValueOrNil,
			))
			return true
		}

		if key, ok := prop.Key.Data.(*js_ast.EString); ok && helpers.UTF16EqualsString(key.Value, "constructor") {
			if fn, ok := prop.ValueOrNil.Data.(*js_ast.EFunction); ok {
				// Remember where the constructor is for later
				ctor = fn

				// Initialize TypeScript constructor parameter fields
				if p.options.ts.Parse {
					for _, arg := range ctor.Fn.Args {
						if arg.IsTypeScriptCtorField {
							if id, ok := arg.Binding.Data.(*js_ast.BIdentifier); ok {
								parameterFields = append(parameterFields, js_ast.AssignStmt(
									js_ast.Expr{Loc: arg.Binding.Loc, Data: p.dotOrMangledPropVisit(
										js_ast.Expr{Loc: arg.Binding.Loc, Data: js_ast.EThisShared},
										p.symbols[id.Ref.InnerIndex].OriginalName,
										arg.Binding.Loc,
									)},
									js_ast.Expr{Loc: arg.Binding.Loc, Data: &js_ast.EIdentifier{Ref: id.Ref}},
								))
							}
						}
					}
				}
			}
		}

		return false
	}

	classLoweringInfo := p.computeClassLoweringInfo(class)
	properties := make([]js_ast.Property, 0, len(class.Properties))
	autoAccessorCount := 0

	for _, prop := range class.Properties {
		if prop.Kind == js_ast.PropertyClassStaticBlock {
			// Drop empty class blocks when minifying
			if p.options.minifySyntax && len(prop.ClassStaticBlock.Block.Stmts) == 0 {
				continue
			}

			if classLoweringInfo.lowerAllStaticFields {
				block := *prop.ClassStaticBlock
				isAllExprs := []js_ast.Expr{}

				// Are all statements in the block expression statements?
			loop:
				for _, stmt := range block.Block.Stmts {
					switch s := stmt.Data.(type) {
					case *js_ast.SEmpty:
						// Omit stray semicolons completely
					case *js_ast.SExpr:
						isAllExprs = append(isAllExprs, s.Value)
					default:
						isAllExprs = nil
						break loop
					}
				}

				if isAllExprs != nil {
					// I think it should be safe to inline the static block IIFE here
					// since all uses of "this" should have already been replaced by now.
					staticMembers = append(staticMembers, isAllExprs...)
				} else {
					// But if there is a non-expression statement, fall back to using an
					// IIFE since we may be in an expression context and can't use a block.
					staticMembers = append(staticMembers, js_ast.Expr{Loc: prop.Loc, Data: &js_ast.ECall{
						Target: js_ast.Expr{Loc: prop.Loc, Data: &js_ast.EArrow{Body: js_ast.FnBody{
							Loc:   block.Loc,
							Block: block.Block,
						}}},
						CanBeUnwrappedIfUnused: js_ast.StmtsCanBeRemovedIfUnused(block.Block.Stmts, 0, p.isUnbound),
					}})
				}
				continue
			}

			// Keep this property
			properties = append(properties, prop)
			continue
		}

		// Merge parameter decorators with method decorators
		if p.options.ts.Parse && prop.Flags.Has(js_ast.PropertyIsMethod) {
			if fn, ok := prop.ValueOrNil.Data.(*js_ast.EFunction); ok {
				isConstructor := false
				if key, ok := prop.Key.Data.(*js_ast.EString); ok {
					isConstructor = helpers.UTF16EqualsString(key.Value, "constructor")
				}
				args := fn.Fn.Args
				for i, arg := range args {
					for _, decorator := range arg.Decorators {
						// Generate a call to "__decorateParam()" for this parameter decorator
						var decorators *[]js_ast.Decorator = &prop.Decorators
						if isConstructor {
							decorators = &class.Decorators
						}
						*decorators = append(*decorators, js_ast.Decorator{
							Value: p.callRuntime(decorator.Value.Loc, "__decorateParam", []js_ast.Expr{
								{Loc: decorator.Value.Loc, Data: &js_ast.ENumber{Value: float64(i)}},
								decorator.Value,
							}),
							AtLoc: decorator.AtLoc,
						})
						args[i].Decorators = nil
					}
				}
			}
		}

		// The TypeScript class field transform requires removing fields without
		// initializers. If the field is removed, then we only need the key for
		// its side effects and we don't need a temporary reference for the key.
		// However, the TypeScript compiler doesn't remove the field when doing
		// strict class field initialization, so we shouldn't either.
		private, _ := prop.Key.Data.(*js_ast.EPrivateIdentifier)
		mustLowerPrivate := private != nil && p.privateSymbolNeedsToBeLowered(private)
		shouldOmitFieldInitializer := p.options.ts.Parse && !prop.Flags.Has(js_ast.PropertyIsMethod) && prop.InitializerOrNil.Data == nil &&
			!class.UseDefineForClassFields && !mustLowerPrivate

		// Class fields must be lowered if the environment doesn't support them
		mustLowerField := false
		if !prop.Flags.Has(js_ast.PropertyIsMethod) {
			if prop.Flags.Has(js_ast.PropertyIsStatic) {
				mustLowerField = classLoweringInfo.lowerAllStaticFields
			} else {
				mustLowerField = classLoweringInfo.lowerAllInstanceFields
			}
		}

		// If the field uses the TypeScript "declare" keyword, just omit it entirely.
		// However, we must still keep any side-effects in the computed value and/or
		// in the decorators.
		if prop.Kind == js_ast.PropertyDeclare && prop.ValueOrNil.Data == nil {
			mustLowerField = true
			shouldOmitFieldInitializer = true
		}

		var propExperimentalDecorators []js_ast.Decorator
		if p.options.ts.Parse && p.options.ts.Config.ExperimentalDecorators == config.True {
			propExperimentalDecorators = prop.Decorators
		}
		rewriteAutoAccessorToGetSet := prop.Kind == js_ast.PropertyAutoAccessor && (p.options.unsupportedJSFeatures.Has(compat.Decorators) || mustLowerField)

		// Transform non-lowered static fields that use assign semantics into an
		// assignment in an inline static block instead of lowering them. This lets
		// us avoid having to unnecessarily lower static private fields when
		// "useDefineForClassFields" is disabled.
		staticFieldToBlockAssign := prop.Kind == js_ast.PropertyNormal && !mustLowerField && !class.UseDefineForClassFields &&
			!prop.Flags.Has(js_ast.PropertyIsMethod) && prop.Flags.Has(js_ast.PropertyIsStatic) && private == nil

		// Make sure the order of computed property keys doesn't change. These
		// expressions have side effects and must be evaluated in order.
		keyExprNoSideEffects := prop.Key
		if prop.Flags.Has(js_ast.PropertyIsComputed) && (len(propExperimentalDecorators) > 0 || mustLowerField || staticFieldToBlockAssign || computedPropertyCache.Data != nil || rewriteAutoAccessorToGetSet) {
			needsKey := true
			if len(propExperimentalDecorators) == 0 && !rewriteAutoAccessorToGetSet && (prop.Flags.Has(js_ast.PropertyIsMethod) || shouldOmitFieldInitializer || (!mustLowerField && !staticFieldToBlockAssign)) {
				needsKey = false
			}

			// Assume all non-literal computed keys have important side effects
			switch prop.Key.Data.(type) {
			case *js_ast.EString, *js_ast.ENameOfSymbol, *js_ast.ENumber:
				// These have no side effects
			default:
				if !needsKey {
					// Just evaluate the key for its side effects
					computedPropertyCache = js_ast.JoinWithComma(computedPropertyCache, prop.Key)
				} else {
					// Store the key in a temporary so we can assign to it later
					ref := p.generateTempRef(tempRefNeedsDeclare, "")
					p.recordUsage(ref)
					computedPropertyCache = js_ast.JoinWithComma(computedPropertyCache,
						js_ast.Assign(js_ast.Expr{Loc: prop.Key.Loc, Data: &js_ast.EIdentifier{Ref: ref}}, prop.Key))
					prop.Key = js_ast.Expr{Loc: prop.Key.Loc, Data: &js_ast.EIdentifier{Ref: ref}}
					keyExprNoSideEffects = prop.Key
				}

				// If this is a computed method, the property value will be used
				// immediately. In this case we inline all computed properties so far to
				// make sure all computed properties before this one are evaluated first.
				if rewriteAutoAccessorToGetSet || (!mustLowerField && !staticFieldToBlockAssign) {
					prop.Key = computedPropertyCache
					computedPropertyCache = js_ast.Expr{}
				}
			}
		}

		// Handle decorators
		if p.options.ts.Parse {
			// Generate a single call to "__decorateClass()" for this property
			if len(propExperimentalDecorators) > 0 {
				loc := prop.Key.Loc

				// This code tells "__decorateClass()" if the descriptor should be undefined
				descriptorKind := float64(1)
				if !prop.Flags.Has(js_ast.PropertyIsMethod) && prop.Kind != js_ast.PropertyAutoAccessor {
					descriptorKind = 2
				}

				// Instance properties use the prototype, static properties use the class
				var target js_ast.Expr
				if prop.Flags.Has(js_ast.PropertyIsStatic) {
					target = nameFunc()
				} else {
					target = js_ast.Expr{Loc: loc, Data: &js_ast.EDot{Target: nameFunc(), Name: "prototype", NameLoc: loc}}
				}

				values := make([]js_ast.Expr, len(propExperimentalDecorators))
				for i, decorator := range propExperimentalDecorators {
					values[i] = decorator.Value
				}
				prop.Decorators = nil
				decorator := p.callRuntime(loc, "__decorateClass", []js_ast.Expr{
					{Loc: loc, Data: &js_ast.EArray{Items: values}},
					target,
					cloneKeyForLowerClass(keyExprNoSideEffects),
					{Loc: loc, Data: &js_ast.ENumber{Value: descriptorKind}},
				})

				// Static decorators are grouped after instance decorators
				if prop.Flags.Has(js_ast.PropertyIsStatic) {
					staticDecorators = append(staticDecorators, decorator)
				} else {
					instanceDecorators = append(instanceDecorators, decorator)
				}
			}
		}

		// Generate get/set methods for auto-accessors
		if rewriteAutoAccessorToGetSet {
			var storageKind ast.SymbolKind
			if prop.Flags.Has(js_ast.PropertyIsStatic) {
				storageKind = ast.SymbolPrivateStaticField
			} else {
				storageKind = ast.SymbolPrivateField
			}

			// Generate the name of the private field to use for storage
			var storageName string
			switch k := keyExprNoSideEffects.Data.(type) {
			case *js_ast.EString:
				storageName = "#" + helpers.UTF16ToString(k.Value)
			case *js_ast.EPrivateIdentifier:
				storageName = "#_" + p.symbols[k.Ref.InnerIndex].OriginalName[1:]
			default:
				storageName = "#" + ast.DefaultNameMinifierJS.NumberToMinifiedName(autoAccessorCount)
				autoAccessorCount++
			}

			// Generate the symbols we need
			storageRef := p.newSymbol(storageKind, storageName)
			argRef := p.newSymbol(ast.SymbolOther, "_")
			result.bodyScope.Generated = append(result.bodyScope.Generated, storageRef)
			result.bodyScope.Children = append(result.bodyScope.Children, &js_ast.Scope{Kind: js_ast.ScopeFunctionBody, Generated: []ast.Ref{argRef}})

			// Replace this accessor with other properties
			loc := keyExprNoSideEffects.Loc
			storagePrivate := &js_ast.EPrivateIdentifier{Ref: storageRef}
			storageNeedsToBeLowered := p.privateSymbolNeedsToBeLowered(storagePrivate)
			storageProp := js_ast.Property{
				Loc:              prop.Loc,
				Kind:             js_ast.PropertyNormal,
				Flags:            prop.Flags & js_ast.PropertyIsStatic,
				Key:              js_ast.Expr{Loc: loc, Data: storagePrivate},
				InitializerOrNil: prop.InitializerOrNil,
			}
			if !mustLowerField {
				properties = append(properties, storageProp)
			} else if prop, ok := lowerField(storageProp, storagePrivate, false, false); ok {
				properties = append(properties, prop)
			}

			// Getter
			var getExpr js_ast.Expr
			if storageNeedsToBeLowered {
				getExpr = p.lowerPrivateGet(js_ast.Expr{Loc: loc, Data: js_ast.EThisShared}, loc, storagePrivate)
			} else {
				p.recordUsage(storageRef)
				getExpr = js_ast.Expr{Loc: loc, Data: &js_ast.EIndex{
					Target: js_ast.Expr{Loc: loc, Data: js_ast.EThisShared},
					Index:  js_ast.Expr{Loc: loc, Data: &js_ast.EPrivateIdentifier{Ref: storageRef}},
				}}
			}
			getterProp := js_ast.Property{
				Loc:   prop.Loc,
				Kind:  js_ast.PropertyGet,
				Flags: prop.Flags | js_ast.PropertyIsMethod,
				Key:   prop.Key,
				ValueOrNil: js_ast.Expr{Loc: loc, Data: &js_ast.EFunction{
					Fn: js_ast.Fn{
						Body: js_ast.FnBody{
							Loc: loc,
							Block: js_ast.SBlock{
								Stmts: []js_ast.Stmt{
									{Loc: loc, Data: &js_ast.SReturn{ValueOrNil: getExpr}},
								},
							},
						},
					},
				}},
			}
			if !lowerMethod(getterProp, private) {
				properties = append(properties, getterProp)
			}

			// Setter
			var setExpr js_ast.Expr
			if storageNeedsToBeLowered {
				setExpr = p.lowerPrivateSet(js_ast.Expr{Loc: loc, Data: js_ast.EThisShared}, loc, storagePrivate,
					js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: argRef}})
			} else {
				p.recordUsage(storageRef)
				p.recordUsage(argRef)
				setExpr = js_ast.Assign(
					js_ast.Expr{Loc: loc, Data: &js_ast.EIndex{
						Target: js_ast.Expr{Loc: loc, Data: js_ast.EThisShared},
						Index:  js_ast.Expr{Loc: loc, Data: &js_ast.EPrivateIdentifier{Ref: storageRef}},
					}},
					js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: argRef}},
				)
			}
			setterProp := js_ast.Property{
				Loc:   prop.Loc,
				Kind:  js_ast.PropertySet,
				Flags: prop.Flags | js_ast.PropertyIsMethod,
				Key:   cloneKeyForLowerClass(keyExprNoSideEffects),
				ValueOrNil: js_ast.Expr{Loc: loc, Data: &js_ast.EFunction{
					Fn: js_ast.Fn{
						Args: []js_ast.Arg{
							{Binding: js_ast.Binding{Loc: loc, Data: &js_ast.BIdentifier{Ref: argRef}}},
						},
						Body: js_ast.FnBody{
							Loc: loc,
							Block: js_ast.SBlock{
								Stmts: []js_ast.Stmt{
									{Loc: loc, Data: &js_ast.SExpr{Value: setExpr}},
								},
							},
						},
					},
				}},
			}
			if !lowerMethod(setterProp, private) {
				properties = append(properties, setterProp)
			}
			continue
		}

		// Lower fields
		if (!prop.Flags.Has(js_ast.PropertyIsMethod) && mustLowerField) || staticFieldToBlockAssign {
			var keep bool
			prop, keep = lowerField(prop, private, shouldOmitFieldInitializer, staticFieldToBlockAssign)
			if !keep {
				continue
			}
		}

		// Lower methods
		if prop.Flags.Has(js_ast.PropertyIsMethod) && lowerMethod(prop, private) {
			continue
		}

		// Keep this property
		properties = append(properties, prop)
	}

	// Finish the filtering operation
	class.Properties = properties

	// If there are expressions with side effects left over and static blocks are
	// supported, insert a static block at the start of the class body. This is
	// necessary because computed static fields need to reference variables that
	// are initialized in this expression:
	//
	//   class Foo {
	//     static [x()] = 1
	//   }
	//
	// The TypeScript compiler transforms that to this:
	//
	//   var _a;
	//   class Foo {
	//     static { _a = x(); }
	//     static { this[_a] = 1; }
	//   }
	//
	if computedPropertyCache.Data != nil && !p.options.unsupportedJSFeatures.Has(compat.ClassStaticBlocks) {
		loc := computedPropertyCache.Loc
		class.Properties = append(append(
			make([]js_ast.Property, 0, 1+len(class.Properties)),
			js_ast.Property{
				Loc:  loc,
				Kind: js_ast.PropertyClassStaticBlock,
				ClassStaticBlock: &js_ast.ClassStaticBlock{
					Loc: loc,
					Block: js_ast.SBlock{
						Stmts: []js_ast.Stmt{
							{Loc: loc, Data: &js_ast.SExpr{Value: computedPropertyCache}},
						},
					},
				},
			}),
			class.Properties...,
		)
		computedPropertyCache = js_ast.Expr{}
	}

	// Insert instance field initializers into the constructor
	if len(parameterFields) > 0 || len(instancePrivateMethods) > 0 || len(instanceMembers) > 0 || (ctor != nil && result.superCtorRef != ast.InvalidRef) {
		// Create a constructor if one doesn't already exist
		if ctor == nil {
			ctor = &js_ast.EFunction{Fn: js_ast.Fn{Body: js_ast.FnBody{Loc: classLoc}}}

			// Append it to the list to reuse existing allocation space
			class.Properties = append(class.Properties, js_ast.Property{
				Flags:      js_ast.PropertyIsMethod,
				Loc:        classLoc,
				Key:        js_ast.Expr{Loc: classLoc, Data: &js_ast.EString{Value: helpers.StringToUTF16("constructor")}},
				ValueOrNil: js_ast.Expr{Loc: classLoc, Data: ctor},
			})

			// Make sure the constructor has a super() call if needed
			if class.ExtendsOrNil.Data != nil {
				target := js_ast.Expr{Loc: classLoc, Data: js_ast.ESuperShared}
				if classLoweringInfo.shimSuperCtorCalls {
					p.recordUsage(result.superCtorRef)
					target.Data = &js_ast.EIdentifier{Ref: result.superCtorRef}
				}
				argumentsRef := p.newSymbol(ast.SymbolUnbound, "arguments")
				p.currentScope.Generated = append(p.currentScope.Generated, argumentsRef)
				ctor.Fn.Body.Block.Stmts = append(ctor.Fn.Body.Block.Stmts, js_ast.Stmt{Loc: classLoc, Data: &js_ast.SExpr{Value: js_ast.Expr{Loc: classLoc, Data: &js_ast.ECall{
					Target: target,
					Args:   []js_ast.Expr{{Loc: classLoc, Data: &js_ast.ESpread{Value: js_ast.Expr{Loc: classLoc, Data: &js_ast.EIdentifier{Ref: argumentsRef}}}}},
				}}}})
			}
		}

		// Make sure the instance field initializers come after "super()" since
		// they need "this" to ba available
		generatedStmts := make([]js_ast.Stmt, 0, len(parameterFields)+len(instancePrivateMethods)+len(instanceMembers))
		generatedStmts = append(generatedStmts, parameterFields...)
		generatedStmts = append(generatedStmts, instancePrivateMethods...)
		generatedStmts = append(generatedStmts, instanceMembers...)
		p.insertStmtsAfterSuperCall(&ctor.Fn.Body, generatedStmts, result.superCtorRef)

		// Sort the constructor first to match the TypeScript compiler's output
		for i := 0; i < len(class.Properties); i++ {
			if class.Properties[i].ValueOrNil.Data == ctor {
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
		var nameToJoin js_ast.Expr
		if didCaptureClassExpr || computedPropertyCache.Data != nil ||
			len(privateMembers) > 0 || len(staticPrivateMethods) > 0 || len(staticMembers) > 0 {
			nameToJoin = nameFunc()
		}

		// Then join "expr" with any other expressions that apply
		if computedPropertyCache.Data != nil {
			expr = js_ast.JoinWithComma(expr, computedPropertyCache)
		}
		for _, value := range privateMembers {
			expr = js_ast.JoinWithComma(expr, value)
		}
		for _, value := range staticPrivateMethods {
			expr = js_ast.JoinWithComma(expr, value)
		}
		for _, value := range staticMembers {
			expr = js_ast.JoinWithComma(expr, value)
		}

		// Finally join "expr" with the variable that holds the class object
		if nameToJoin.Data != nil {
			expr = js_ast.JoinWithComma(expr, nameToJoin)
		}
		if wrapFunc != nil {
			expr = wrapFunc(expr)
		}
		return nil, expr
	}

	// When bundling is enabled, we convert top-level class statements to
	// expressions:
	//
	//   // Before
	//   class Foo {
	//     static foo = () => Foo
	//   }
	//   Foo = wrap(Foo)
	//
	//   // After
	//   var _Foo = class _Foo {
	//     static foo = () => _Foo;
	//   };
	//   var Foo = _Foo;
	//   Foo = wrap(Foo);
	//
	// One reason to do this is that esbuild's bundler sometimes needs to lazily-
	// evaluate a module. For example, a module may end up being both the target
	// of a dynamic "import()" call and a static "import" statement. Lazy module
	// evaluation is done by wrapping the top-level module code in a closure. To
	// avoid a performance hit for static "import" statements, esbuild stores
	// top-level exported symbols outside of the closure and references them
	// directly instead of indirectly.
	//
	// Another reason to do this is that multiple JavaScript VMs have had and
	// continue to have performance issues with TDZ (i.e. "temporal dead zone")
	// checks. These checks validate that a let, or const, or class symbol isn't
	// used before it's initialized. Here are two issues with well-known VMs:
	//
	//   * V8: https://bugs.chromium.org/p/v8/issues/detail?id=13723 (10% slowdown)
	//   * JavaScriptCore: https://bugs.webkit.org/show_bug.cgi?id=199866 (1,000% slowdown!)
	//
	// JavaScriptCore had a severe performance issue as their TDZ implementation
	// had time complexity that was quadratic in the number of variables needing
	// TDZ checks in the same scope (with the top-level scope typically being the
	// worst offender). V8 has ongoing issues with TDZ checks being present
	// throughout the code their JIT generates even when they have already been
	// checked earlier in the same function or when the function in question has
	// already been run (so the checks have already happened).
	//
	// Due to esbuild's parallel architecture, we both a) need to transform class
	// statements to variables during parsing and b) don't yet know whether this
	// module will need to be lazily-evaluated or not in the parser. So we always
	// do this just in case it's needed.
	mustConvertStmtToExpr := p.currentScope.Parent == nil && (p.options.mode == config.ModeBundle || p.willWrapModuleInTryCatchForUsing)

	var classExperimentalDecorators []js_ast.Decorator
	if p.options.ts.Parse && p.options.ts.Config.ExperimentalDecorators == config.True {
		classExperimentalDecorators = class.Decorators
	}

	// If this is true, we have removed some code from the class body that could
	// potentially contain an expression that captures the inner class name.
	// In this case we must explicitly store the class to a separate inner class
	// name binding to avoid incorrect behavior if the class is later re-assigned,
	// since the removed code will no longer be in the class body scope.
	hasPotentialInnerClassNameEscape := result.innerClassNameRef != ast.InvalidRef &&
		(computedPropertyCache.Data != nil ||
			len(privateMembers) > 0 ||
			len(staticPrivateMethods) > 0 ||
			len(staticMembers) > 0 ||
			len(instanceDecorators) > 0 ||
			len(staticDecorators) > 0 ||
			len(classExperimentalDecorators) > 0)

	// Pack the class back into a statement, with potentially some extra
	// statements afterwards
	var stmts []js_ast.Stmt
	var outerClassNameDecl js_ast.Stmt
	var nameForClassDecorators ast.LocRef
	didGenerateLocalStmt := false
	if len(classExperimentalDecorators) > 0 || hasPotentialInnerClassNameEscape || mustConvertStmtToExpr {
		didGenerateLocalStmt = true

		// Determine the name to use for decorators
		if kind == classKindExpr {
			// For expressions, the inner and outer class names are the same
			name := nameFunc()
			nameForClassDecorators = ast.LocRef{Loc: name.Loc, Ref: name.Data.(*js_ast.EIdentifier).Ref}
		} else {
			// For statements we need to use the outer class name, not the inner one
			if class.Name != nil {
				nameForClassDecorators = *class.Name
			} else if kind == classKindExportDefaultStmt {
				nameForClassDecorators = defaultName
			} else {
				nameForClassDecorators = ast.LocRef{Loc: classLoc, Ref: p.generateTempRef(tempRefNoDeclare, "")}
			}
			p.recordUsage(nameForClassDecorators.Ref)
		}

		classExpr := js_ast.EClass{Class: *class}
		class = &classExpr.Class
		init := js_ast.Expr{Loc: classLoc, Data: &classExpr}

		// If the inner class name was referenced, then set the name of the class
		// that we will end up printing to the inner class name. Otherwise if the
		// inner class name was unused, we can just leave it blank.
		if result.innerClassNameRef != ast.InvalidRef {
			// "class Foo { x = Foo }" => "const Foo = class _Foo { x = _Foo }"
			class.Name.Ref = result.innerClassNameRef
		} else {
			// "class Foo {}" => "const Foo = class {}"
			class.Name = nil
		}

		// Generate the class initialization statement
		if len(classExperimentalDecorators) > 0 {
			// If there are class decorators, then we actually need to mutate the
			// immutable "const" binding that shadows everything in the class body.
			// The official TypeScript compiler does this by rewriting all class name
			// references in the class body to another temporary variable. This is
			// basically what we're doing here.
			p.recordUsage(nameForClassDecorators.Ref)
			stmts = append(stmts, js_ast.Stmt{Loc: classLoc, Data: &js_ast.SLocal{
				Kind:     p.selectLocalKind(js_ast.LocalLet),
				IsExport: kind == classKindExportStmt,
				Decls: []js_ast.Decl{{
					Binding:    js_ast.Binding{Loc: nameForClassDecorators.Loc, Data: &js_ast.BIdentifier{Ref: nameForClassDecorators.Ref}},
					ValueOrNil: init,
				}},
			}})
			if class.Name != nil {
				p.mergeSymbols(class.Name.Ref, nameForClassDecorators.Ref)
				class.Name = nil
			}
		} else if hasPotentialInnerClassNameEscape {
			// If the inner class name was used, then we explicitly generate a binding
			// for it. That means the mutable outer class name is separate, and is
			// initialized after all static member initializers have finished.
			captureRef := p.newSymbol(ast.SymbolOther, p.symbols[result.innerClassNameRef.InnerIndex].OriginalName)
			p.currentScope.Generated = append(p.currentScope.Generated, captureRef)
			p.recordDeclaredSymbol(captureRef)
			p.mergeSymbols(result.innerClassNameRef, captureRef)
			stmts = append(stmts, js_ast.Stmt{Loc: classLoc, Data: &js_ast.SLocal{
				Kind: p.selectLocalKind(js_ast.LocalConst),
				Decls: []js_ast.Decl{{
					Binding:    js_ast.Binding{Loc: nameForClassDecorators.Loc, Data: &js_ast.BIdentifier{Ref: captureRef}},
					ValueOrNil: init,
				}},
			}})
			p.recordUsage(nameForClassDecorators.Ref)
			p.recordUsage(captureRef)
			outerClassNameDecl = js_ast.Stmt{Loc: classLoc, Data: &js_ast.SLocal{
				Kind:     p.selectLocalKind(js_ast.LocalLet),
				IsExport: kind == classKindExportStmt,
				Decls: []js_ast.Decl{{
					Binding:    js_ast.Binding{Loc: nameForClassDecorators.Loc, Data: &js_ast.BIdentifier{Ref: nameForClassDecorators.Ref}},
					ValueOrNil: js_ast.Expr{Loc: classLoc, Data: &js_ast.EIdentifier{Ref: captureRef}},
				}},
			}}
		} else {
			// Otherwise, the inner class name isn't needed and we can just
			// use a single variable declaration for the outer class name.
			p.recordUsage(nameForClassDecorators.Ref)
			stmts = append(stmts, js_ast.Stmt{Loc: classLoc, Data: &js_ast.SLocal{
				Kind:     p.selectLocalKind(js_ast.LocalLet),
				IsExport: kind == classKindExportStmt,
				Decls: []js_ast.Decl{{
					Binding:    js_ast.Binding{Loc: nameForClassDecorators.Loc, Data: &js_ast.BIdentifier{Ref: nameForClassDecorators.Ref}},
					ValueOrNil: init,
				}},
			}})
		}
	} else {
		switch kind {
		case classKindStmt:
			stmts = append(stmts, js_ast.Stmt{Loc: classLoc, Data: &js_ast.SClass{Class: *class}})
		case classKindExportStmt:
			stmts = append(stmts, js_ast.Stmt{Loc: classLoc, Data: &js_ast.SClass{Class: *class, IsExport: true}})
		case classKindExportDefaultStmt:
			stmts = append(stmts, js_ast.Stmt{Loc: classLoc, Data: &js_ast.SExportDefault{
				DefaultName: defaultName,
				Value:       js_ast.Stmt{Loc: classLoc, Data: &js_ast.SClass{Class: *class}},
			}})
		}

		// The inner class name inside the class statement should be the same as
		// the class statement name itself
		if class.Name != nil && result.innerClassNameRef != ast.InvalidRef {
			// If the class body contains a direct eval call, then the inner class
			// name will be marked as "MustNotBeRenamed" (because we have already
			// popped the class body scope) but the outer class name won't be marked
			// as "MustNotBeRenamed" yet (because we haven't yet popped the containing
			// scope). Propagate this flag now before we merge these symbols so we
			// don't end up accidentally renaming the outer class name to the inner
			// class name.
			if p.currentScope.ContainsDirectEval {
				p.symbols[class.Name.Ref.InnerIndex].Flags |= (p.symbols[result.innerClassNameRef.InnerIndex].Flags & ast.MustNotBeRenamed)
			}
			p.mergeSymbols(result.innerClassNameRef, class.Name.Ref)
		}
	}

	// The official TypeScript compiler adds generated code after the class body
	// in this exact order. Matching this order is important for correctness.
	if computedPropertyCache.Data != nil {
		stmts = append(stmts, js_ast.Stmt{Loc: expr.Loc, Data: &js_ast.SExpr{Value: computedPropertyCache}})
	}
	for _, expr := range privateMembers {
		stmts = append(stmts, js_ast.Stmt{Loc: expr.Loc, Data: &js_ast.SExpr{Value: expr}})
	}
	for _, expr := range staticPrivateMethods {
		stmts = append(stmts, js_ast.Stmt{Loc: expr.Loc, Data: &js_ast.SExpr{Value: expr}})
	}
	for _, expr := range staticMembers {
		stmts = append(stmts, js_ast.Stmt{Loc: expr.Loc, Data: &js_ast.SExpr{Value: expr}})
	}
	for _, expr := range instanceDecorators {
		stmts = append(stmts, js_ast.Stmt{Loc: expr.Loc, Data: &js_ast.SExpr{Value: expr}})
	}
	for _, expr := range staticDecorators {
		stmts = append(stmts, js_ast.Stmt{Loc: expr.Loc, Data: &js_ast.SExpr{Value: expr}})
	}
	if outerClassNameDecl.Data != nil {
		// This must come after the class body initializers have finished
		stmts = append(stmts, outerClassNameDecl)
	}
	if len(classExperimentalDecorators) > 0 {
		values := make([]js_ast.Expr, len(classExperimentalDecorators))
		for i, decorator := range classExperimentalDecorators {
			values[i] = decorator.Value
		}
		class.Decorators = nil
		stmts = append(stmts, js_ast.AssignStmt(
			js_ast.Expr{Loc: nameForClassDecorators.Loc, Data: &js_ast.EIdentifier{Ref: nameForClassDecorators.Ref}},
			p.callRuntime(classLoc, "__decorateClass", []js_ast.Expr{
				{Loc: classLoc, Data: &js_ast.EArray{Items: values}},
				{Loc: nameForClassDecorators.Loc, Data: &js_ast.EIdentifier{Ref: nameForClassDecorators.Ref}},
			}),
		))
		p.recordUsage(nameForClassDecorators.Ref)
		p.recordUsage(nameForClassDecorators.Ref)
	}
	if didGenerateLocalStmt && kind == classKindExportDefaultStmt {
		// "export default class x {}" => "class x {} export {x as default}"
		stmts = append(stmts, js_ast.Stmt{Loc: classLoc, Data: &js_ast.SExportClause{
			Items: []js_ast.ClauseItem{{Alias: "default", Name: defaultName}},
		}})
	}
	return stmts, js_ast.Expr{}
}

func cloneKeyForLowerClass(key js_ast.Expr) js_ast.Expr {
	switch k := key.Data.(type) {
	case *js_ast.ENumber:
		clone := *k
		key.Data = &clone
	case *js_ast.EString:
		clone := *k
		key.Data = &clone
	case *js_ast.EIdentifier:
		clone := *k
		key.Data = &clone
	case *js_ast.ENameOfSymbol:
		clone := *k
		key.Data = &clone
	case *js_ast.EPrivateIdentifier:
		clone := *k
		key.Data = &clone
	default:
		panic("Internal error")
	}
	return key
}

// Replace "super()" calls with our shim so that we can guarantee
// that instance field initialization doesn't happen before "super()"
// is called, since at that point "this" isn't available.
func (p *parser) insertStmtsAfterSuperCall(body *js_ast.FnBody, stmtsToInsert []js_ast.Stmt, superCtorRef ast.Ref) {
	// If this class has no base class, then there's no "super()" call to handle
	if superCtorRef == ast.InvalidRef || p.symbols[superCtorRef.InnerIndex].UseCountEstimate == 0 {
		body.Block.Stmts = append(stmtsToInsert, body.Block.Stmts...)
		return
	}

	// It's likely that there's only one "super()" call, and that it's a
	// top-level expression in the constructor function body. If so, we
	// can generate tighter code for this common case.
	if p.symbols[superCtorRef.InnerIndex].UseCountEstimate == 1 {
		for i, stmt := range body.Block.Stmts {
			var before js_ast.Expr
			var callLoc logger.Loc
			var callData *js_ast.ECall
			var after js_ast.Stmt

			switch s := stmt.Data.(type) {
			case *js_ast.SExpr:
				if b, loc, c, a := findFirstTopLevelSuperCall(s.Value, superCtorRef); c != nil {
					before, callLoc, callData = b, loc, c
					if a.Data != nil {
						s.Value = a
						after = js_ast.Stmt{Loc: a.Loc, Data: s}
					}
				}

			case *js_ast.SReturn:
				if s.ValueOrNil.Data != nil {
					if b, loc, c, a := findFirstTopLevelSuperCall(s.ValueOrNil, superCtorRef); c != nil && a.Data != nil {
						before, callLoc, callData = b, loc, c
						s.ValueOrNil = a
						after = js_ast.Stmt{Loc: a.Loc, Data: s}
					}
				}

			case *js_ast.SThrow:
				if b, loc, c, a := findFirstTopLevelSuperCall(s.Value, superCtorRef); c != nil && a.Data != nil {
					before, callLoc, callData = b, loc, c
					s.Value = a
					after = js_ast.Stmt{Loc: a.Loc, Data: s}
				}

			case *js_ast.SIf:
				if b, loc, c, a := findFirstTopLevelSuperCall(s.Test, superCtorRef); c != nil && a.Data != nil {
					before, callLoc, callData = b, loc, c
					s.Test = a
					after = js_ast.Stmt{Loc: a.Loc, Data: s}
				}

			case *js_ast.SSwitch:
				if b, loc, c, a := findFirstTopLevelSuperCall(s.Test, superCtorRef); c != nil && a.Data != nil {
					before, callLoc, callData = b, loc, c
					s.Test = a
					after = js_ast.Stmt{Loc: a.Loc, Data: s}
				}

			case *js_ast.SFor:
				if expr, ok := s.InitOrNil.Data.(*js_ast.SExpr); ok {
					if b, loc, c, a := findFirstTopLevelSuperCall(expr.Value, superCtorRef); c != nil {
						before, callLoc, callData = b, loc, c
						if a.Data != nil {
							expr.Value = a
						} else {
							s.InitOrNil.Data = nil
						}
						after = js_ast.Stmt{Loc: a.Loc, Data: s}
					}
				}
			}

			if callData != nil {
				// Revert "__super()" back to "super()"
				callData.Target.Data = js_ast.ESuperShared
				p.ignoreUsage(superCtorRef)

				// Inject "stmtsToInsert" after "super()"
				stmtsBefore := body.Block.Stmts[:i]
				stmtsAfter := body.Block.Stmts[i+1:]
				stmts := append([]js_ast.Stmt{}, stmtsBefore...)
				if before.Data != nil {
					stmts = append(stmts, js_ast.Stmt{Loc: before.Loc, Data: &js_ast.SExpr{Value: before}})
				}
				stmts = append(stmts, js_ast.Stmt{Loc: callLoc, Data: &js_ast.SExpr{Value: js_ast.Expr{Loc: callLoc, Data: callData}}})
				stmts = append(stmts, stmtsToInsert...)
				if after.Data != nil {
					stmts = append(stmts, after)
				}
				stmts = append(stmts, stmtsAfter...)
				body.Block.Stmts = stmts
				return
			}
		}
	}

	// Otherwise, inject a generated "__super" helper function at the top of the
	// constructor that looks like this:
	//
	//   var __super = (...args) => {
	//     super(...args);
	//     ...stmtsToInsert...
	//   };
	//
	argsRef := p.newSymbol(ast.SymbolOther, "args")
	p.currentScope.Generated = append(p.currentScope.Generated, argsRef)
	stmtsToInsert = append([]js_ast.Stmt{{Loc: body.Loc, Data: &js_ast.SExpr{Value: js_ast.Expr{Loc: body.Loc, Data: &js_ast.ECall{
		Target: js_ast.Expr{Loc: body.Loc, Data: js_ast.ESuperShared},
		Args:   []js_ast.Expr{{Loc: body.Loc, Data: &js_ast.ESpread{Value: js_ast.Expr{Loc: body.Loc, Data: &js_ast.EIdentifier{Ref: argsRef}}}}},
	}}}}}, stmtsToInsert...)
	body.Block.Stmts = append([]js_ast.Stmt{{Loc: body.Loc, Data: &js_ast.SLocal{Decls: []js_ast.Decl{{
		Binding: js_ast.Binding{Loc: body.Loc, Data: &js_ast.BIdentifier{Ref: superCtorRef}}, ValueOrNil: js_ast.Expr{Loc: body.Loc, Data: &js_ast.EArrow{
			HasRestArg: true,
			Args:       []js_ast.Arg{{Binding: js_ast.Binding{Loc: body.Loc, Data: &js_ast.BIdentifier{Ref: argsRef}}}},
			Body:       js_ast.FnBody{Loc: body.Loc, Block: js_ast.SBlock{Stmts: stmtsToInsert}},
		}},
	}}}}}, body.Block.Stmts...)
}

func findFirstTopLevelSuperCall(expr js_ast.Expr, superCtorRef ast.Ref) (js_ast.Expr, logger.Loc, *js_ast.ECall, js_ast.Expr) {
	if call, ok := expr.Data.(*js_ast.ECall); ok {
		if target, ok := call.Target.Data.(*js_ast.EIdentifier); ok && target.Ref == superCtorRef {
			call.Target.Data = js_ast.ESuperShared
			return js_ast.Expr{}, expr.Loc, call, js_ast.Expr{}
		}
	}

	// Also search down comma operator chains for a super call
	if comma, ok := expr.Data.(*js_ast.EBinary); ok && comma.Op == js_ast.BinOpComma {
		if before, loc, call, after := findFirstTopLevelSuperCall(comma.Left, superCtorRef); call != nil {
			return before, loc, call, js_ast.JoinWithComma(after, comma.Right)
		}

		if before, loc, call, after := findFirstTopLevelSuperCall(comma.Right, superCtorRef); call != nil {
			return js_ast.JoinWithComma(comma.Left, before), loc, call, after
		}
	}

	return js_ast.Expr{}, logger.Loc{}, nil, js_ast.Expr{}
}
