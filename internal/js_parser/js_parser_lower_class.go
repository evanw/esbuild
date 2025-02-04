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

	// If something has decorators, just lower everything for now. It's possible
	// that we could avoid lowering in certain cases, but doing so is very tricky
	// due to the complexity of the decorator specification. The specification is
	// also still evolving so trying to optimize it now is also potentially
	// premature.
	if class.ShouldLowerStandardDecorators {
		for _, prop := range class.Properties {
			if len(prop.Decorators) > 0 {
				for _, prop := range class.Properties {
					if private, ok := prop.Key.Data.(*js_ast.EPrivateIdentifier); ok {
						p.symbols[private.Ref.InnerIndex].Flags |= ast.PrivateSymbolMustBeLowered
					}
				}
				result.lowerAllStaticFields = true
				result.lowerAllInstanceFields = true
				break
			}
		}
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
		if prop.Kind.IsMethodDefinition() {
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
			if p.options.ts.Parse && !class.UseDefineForClassFields {
				// Convert instance fields to assignment statements if the TypeScript
				// setting for this is enabled. I don't think this matters for private
				// fields because there's no way for this to call a setter in the base
				// class, so this isn't done for private fields.
				if prop.InitializerOrNil.Data != nil {
					// We can skip lowering all instance fields if all instance fields
					// disappear completely when lowered. This happens when
					// "useDefineForClassFields" is false and there is no initializer.
					result.lowerAllInstanceFields = true
				}
			} else if p.options.unsupportedJSFeatures.Has(compat.ClassField) {
				// Instance fields must be lowered if the target doesn't support them
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

type classKind uint8

const (
	classKindExpr classKind = iota
	classKindStmt
	classKindExportStmt
	classKindExportDefaultStmt
)

type lowerClassContext struct {
	nameToKeep  string
	kind        classKind
	class       *js_ast.Class
	classLoc    logger.Loc
	classExpr   js_ast.Expr // Only for "kind == classKindExpr", may be replaced by "nameFunc()"
	defaultName ast.LocRef

	ctor                   *js_ast.EFunction
	extendsRef             ast.Ref
	parameterFields        []js_ast.Stmt
	instanceMembers        []js_ast.Stmt
	instancePrivateMethods []js_ast.Stmt
	autoAccessorCount      int

	// These expressions are generated after the class body, in this order
	computedPropertyChain js_ast.Expr
	privateMembers        []js_ast.Expr
	staticMembers         []js_ast.Expr
	staticPrivateMethods  []js_ast.Expr

	// These contain calls to "__decorateClass" for TypeScript experimental decorators
	instanceExperimentalDecorators []js_ast.Expr
	staticExperimentalDecorators   []js_ast.Expr

	// These are used for implementing JavaScript decorators
	decoratorContextRef                          ast.Ref
	decoratorClassDecorators                     js_ast.Expr
	decoratorPropertyToInitializerMap            map[int]int
	decoratorCallInstanceMethodExtraInitializers bool
	decoratorCallStaticMethodExtraInitializers   bool
	decoratorStaticNonFieldElements              []js_ast.Expr
	decoratorInstanceNonFieldElements            []js_ast.Expr
	decoratorStaticFieldElements                 []js_ast.Expr
	decoratorInstanceFieldElements               []js_ast.Expr

	// These are used by "lowerMethod"
	privateInstanceMethodRef ast.Ref
	privateStaticMethodRef   ast.Ref

	// These are only for class expressions that need to be captured
	nameFunc            func() js_ast.Expr
	wrapFunc            func(js_ast.Expr) js_ast.Expr
	didCaptureClassExpr bool
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
func (p *parser) lowerClass(stmt js_ast.Stmt, expr js_ast.Expr, result visitClassResult, nameToKeep string) ([]js_ast.Stmt, js_ast.Expr) {
	ctx := lowerClassContext{
		nameToKeep:               nameToKeep,
		extendsRef:               ast.InvalidRef,
		decoratorContextRef:      ast.InvalidRef,
		privateInstanceMethodRef: ast.InvalidRef,
		privateStaticMethodRef:   ast.InvalidRef,
	}

	// Unpack the class from the statement or expression
	if stmt.Data == nil {
		e, _ := expr.Data.(*js_ast.EClass)
		ctx.class = &e.Class
		ctx.classExpr = expr
		ctx.kind = classKindExpr
		if ctx.class.Name != nil {
			symbol := &p.symbols[ctx.class.Name.Ref.InnerIndex]
			ctx.nameToKeep = symbol.OriginalName

			// The inner class name inside the class expression should be the same as
			// the class expression name itself
			if result.innerClassNameRef != ast.InvalidRef {
				p.mergeSymbols(result.innerClassNameRef, ctx.class.Name.Ref)
			}

			// Remove unused class names when minifying. Check this after we merge in
			// the inner class name above since that will adjust the use count.
			if p.options.minifySyntax && symbol.UseCountEstimate == 0 {
				ctx.class.Name = nil
			}
		}
	} else if s, ok := stmt.Data.(*js_ast.SClass); ok {
		ctx.class = &s.Class
		if ctx.class.Name != nil {
			ctx.nameToKeep = p.symbols[ctx.class.Name.Ref.InnerIndex].OriginalName
		}
		if s.IsExport {
			ctx.kind = classKindExportStmt
		} else {
			ctx.kind = classKindStmt
		}
	} else {
		s, _ := stmt.Data.(*js_ast.SExportDefault)
		s2, _ := s.Value.Data.(*js_ast.SClass)
		ctx.class = &s2.Class
		if ctx.class.Name != nil {
			ctx.nameToKeep = p.symbols[ctx.class.Name.Ref.InnerIndex].OriginalName
		}
		ctx.defaultName = s.DefaultName
		ctx.kind = classKindExportDefaultStmt
	}
	if stmt.Data == nil {
		ctx.classLoc = expr.Loc
	} else {
		ctx.classLoc = stmt.Loc
	}

	classLoweringInfo := p.computeClassLoweringInfo(ctx.class)
	ctx.enableNameCapture(p, result)
	ctx.processProperties(p, classLoweringInfo, result)
	ctx.insertInitializersIntoConstructor(p, classLoweringInfo, result)
	return ctx.finishAndGenerateCode(p, result)
}

func (ctx *lowerClassContext) enableNameCapture(p *parser, result visitClassResult) {
	// Class statements can be missing a name if they are in an
	// "export default" statement:
	//
	//   export default class {
	//     static foo = 123
	//   }
	//
	ctx.nameFunc = func() js_ast.Expr {
		if ctx.kind == classKindExpr {
			// If this is a class expression, capture and store it. We have to
			// do this even if it has a name since the name isn't exposed
			// outside the class body.
			classExpr := &js_ast.EClass{Class: *ctx.class}
			ctx.class = &classExpr.Class
			ctx.nameFunc, ctx.wrapFunc = p.captureValueWithPossibleSideEffects(ctx.classLoc, 2, js_ast.Expr{Loc: ctx.classLoc, Data: classExpr}, valueDefinitelyNotMutated)
			ctx.classExpr = ctx.nameFunc()
			ctx.didCaptureClassExpr = true
			name := ctx.nameFunc()

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
			if ctx.class.Name != nil {
				p.mergeSymbols(ctx.class.Name.Ref, name.Data.(*js_ast.EIdentifier).Ref)
				ctx.class.Name = nil
			}

			return name
		} else {
			// If anything referenced the inner class name, then we should use that
			// name for any automatically-generated initialization code, since it
			// will come before the outer class name is initialized.
			if result.innerClassNameRef != ast.InvalidRef {
				p.recordUsage(result.innerClassNameRef)
				return js_ast.Expr{Loc: ctx.class.Name.Loc, Data: &js_ast.EIdentifier{Ref: result.innerClassNameRef}}
			}

			// Otherwise we should just use the outer class name
			if ctx.class.Name == nil {
				if ctx.kind == classKindExportDefaultStmt {
					ctx.class.Name = &ctx.defaultName
				} else {
					ctx.class.Name = &ast.LocRef{Loc: ctx.classLoc, Ref: p.generateTempRef(tempRefNoDeclare, "")}
				}
			}
			p.recordUsage(ctx.class.Name.Ref)
			return js_ast.Expr{Loc: ctx.class.Name.Loc, Data: &js_ast.EIdentifier{Ref: ctx.class.Name.Ref}}
		}
	}
}

// Handle lowering of instance and static fields. Move their initializers
// from the class body to either the constructor (instance fields) or after
// the class (static fields).
//
// If this returns true, the return property should be added to the class
// body. Otherwise the property should be omitted from the class body.
func (ctx *lowerClassContext) lowerField(
	p *parser,
	prop js_ast.Property,
	private *js_ast.EPrivateIdentifier,
	shouldOmitFieldInitializer bool,
	staticFieldToBlockAssign bool,
	initializerIndex int,
) (js_ast.Property, ast.Ref, bool) {
	mustLowerPrivate := private != nil && p.privateSymbolNeedsToBeLowered(private)
	ref := ast.InvalidRef

	// The TypeScript compiler doesn't follow the JavaScript spec for
	// uninitialized fields. They are supposed to be set to undefined but the
	// TypeScript compiler just omits them entirely.
	if !shouldOmitFieldInitializer {
		loc := prop.Loc

		// Determine where to store the field
		var target js_ast.Expr
		if prop.Flags.Has(js_ast.PropertyIsStatic) && !staticFieldToBlockAssign {
			target = ctx.nameFunc()
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

		// Optionally call registered decorator initializers
		if initializerIndex != -1 {
			var value js_ast.Expr
			if prop.Flags.Has(js_ast.PropertyIsStatic) {
				value = ctx.nameFunc()
			} else {
				value = js_ast.Expr{Loc: loc, Data: js_ast.EThisShared}
			}
			args := []js_ast.Expr{
				{Loc: loc, Data: &js_ast.EIdentifier{Ref: ctx.decoratorContextRef}},
				{Loc: loc, Data: &js_ast.ENumber{Value: float64((4 + 2*initializerIndex) << 1)}},
				value,
			}
			if _, ok := init.Data.(*js_ast.EUndefined); !ok {
				args = append(args, init)
			}
			init = p.callRuntime(init.Loc, "__runInitializers", args)
			p.recordUsage(ctx.decoratorContextRef)
		}

		// Generate the assignment target
		var memberExpr js_ast.Expr
		if mustLowerPrivate {
			// Generate a new symbol for this private field
			ref = p.generateTempRef(tempRefNeedsDeclare, "_"+p.symbols[private.Ref.InnerIndex].OriginalName[1:])
			p.symbols[private.Ref.InnerIndex].Link = ref

			// Initialize the private field to a new WeakMap
			if p.weakMapRef == ast.InvalidRef {
				p.weakMapRef = p.newSymbol(ast.SymbolUnbound, "WeakMap")
				p.moduleScope.Generated = append(p.moduleScope.Generated, p.weakMapRef)
			}
			ctx.privateMembers = append(ctx.privateMembers, js_ast.Assign(
				js_ast.Expr{Loc: prop.Key.Loc, Data: &js_ast.EIdentifier{Ref: ref}},
				js_ast.Expr{Loc: prop.Key.Loc, Data: &js_ast.ENew{Target: js_ast.Expr{Loc: prop.Key.Loc, Data: &js_ast.EIdentifier{Ref: p.weakMapRef}}}},
			))
			p.recordUsage(ref)

			// Add every newly-constructed instance into this map
			key := js_ast.Expr{Loc: prop.Key.Loc, Data: &js_ast.EIdentifier{Ref: ref}}
			args := []js_ast.Expr{target, key}
			if _, ok := init.Data.(*js_ast.EUndefined); !ok {
				args = append(args, init)
			}
			memberExpr = p.callRuntime(loc, "__privateAdd", args)
			p.recordUsage(ref)
		} else if private == nil && ctx.class.UseDefineForClassFields {
			if p.shouldAddKeyComment {
				if str, ok := prop.Key.Data.(*js_ast.EString); ok {
					str.HasPropertyKeyComment = true
				}
			}
			args := []js_ast.Expr{target, prop.Key}
			if _, ok := init.Data.(*js_ast.EUndefined); !ok {
				args = append(args, init)
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

		// Run extra initializers
		if initializerIndex != -1 {
			var value js_ast.Expr
			if prop.Flags.Has(js_ast.PropertyIsStatic) {
				value = ctx.nameFunc()
			} else {
				value = js_ast.Expr{Loc: loc, Data: js_ast.EThisShared}
			}
			memberExpr = js_ast.JoinWithComma(memberExpr, p.callRuntime(loc, "__runInitializers", []js_ast.Expr{
				{Loc: loc, Data: &js_ast.EIdentifier{Ref: ctx.decoratorContextRef}},
				{Loc: loc, Data: &js_ast.ENumber{Value: float64(((5 + 2*initializerIndex) << 1) | 1)}},
				value,
			}))
			p.recordUsage(ctx.decoratorContextRef)
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
				}, ref, true
			} else {
				// Move this property to an assignment after the class ends
				ctx.staticMembers = append(ctx.staticMembers, memberExpr)
			}
		} else {
			// Move this property to an assignment inside the class constructor
			ctx.instanceMembers = append(ctx.instanceMembers, js_ast.Stmt{Loc: loc, Data: &js_ast.SExpr{Value: memberExpr}})
		}
	}

	if private == nil || mustLowerPrivate {
		// Remove the field from the class body
		return js_ast.Property{}, ref, false
	}

	// Keep the private field but remove the initializer
	prop.InitializerOrNil = js_ast.Expr{}
	return prop, ref, true
}

func (ctx *lowerClassContext) lowerPrivateMethod(p *parser, prop js_ast.Property, private *js_ast.EPrivateIdentifier) {
	// All private methods can share the same WeakSet
	var ref *ast.Ref
	if prop.Flags.Has(js_ast.PropertyIsStatic) {
		ref = &ctx.privateStaticMethodRef
	} else {
		ref = &ctx.privateInstanceMethodRef
	}
	if *ref == ast.InvalidRef {
		// Generate a new symbol to store the WeakSet
		var name string
		if prop.Flags.Has(js_ast.PropertyIsStatic) {
			name = "_static"
		} else {
			name = "_instances"
		}
		if ctx.nameToKeep != "" {
			name = fmt.Sprintf("_%s%s", ctx.nameToKeep, name)
		}
		*ref = p.generateTempRef(tempRefNeedsDeclare, name)

		// Generate the initializer
		if p.weakSetRef == ast.InvalidRef {
			p.weakSetRef = p.newSymbol(ast.SymbolUnbound, "WeakSet")
			p.moduleScope.Generated = append(p.moduleScope.Generated, p.weakSetRef)
		}
		ctx.privateMembers = append(ctx.privateMembers, js_ast.Assign(
			js_ast.Expr{Loc: ctx.classLoc, Data: &js_ast.EIdentifier{Ref: *ref}},
			js_ast.Expr{Loc: ctx.classLoc, Data: &js_ast.ENew{Target: js_ast.Expr{Loc: ctx.classLoc, Data: &js_ast.EIdentifier{Ref: p.weakSetRef}}}},
		))
		p.recordUsage(*ref)
		p.recordUsage(p.weakSetRef)

		// Determine what to store in the WeakSet
		var target js_ast.Expr
		if prop.Flags.Has(js_ast.PropertyIsStatic) {
			target = ctx.nameFunc()
		} else {
			target = js_ast.Expr{Loc: ctx.classLoc, Data: js_ast.EThisShared}
		}

		// Add every newly-constructed instance into this set
		methodExpr := p.callRuntime(ctx.classLoc, "__privateAdd", []js_ast.Expr{
			target,
			{Loc: ctx.classLoc, Data: &js_ast.EIdentifier{Ref: *ref}},
		})
		p.recordUsage(*ref)

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
			ctx.staticPrivateMethods = append(ctx.staticPrivateMethods, methodExpr)
		} else {
			// Move this property to an assignment inside the class constructor
			ctx.instancePrivateMethods = append(ctx.instancePrivateMethods, js_ast.Stmt{Loc: ctx.classLoc, Data: &js_ast.SExpr{Value: methodExpr}})
		}
	}
	p.symbols[private.Ref.InnerIndex].Link = *ref
}

// If this returns true, the method property should be dropped as it has
// already been accounted for elsewhere (e.g. a lowered private method).
func (ctx *lowerClassContext) lowerMethod(p *parser, prop js_ast.Property, private *js_ast.EPrivateIdentifier) bool {
	if private != nil && p.privateSymbolNeedsToBeLowered(private) {
		ctx.lowerPrivateMethod(p, prop, private)

		// Move the method definition outside the class body
		methodRef := p.generateTempRef(tempRefNeedsDeclare, "_")
		if prop.Kind == js_ast.PropertySetter {
			p.symbols[methodRef.InnerIndex].Link = p.privateSetters[private.Ref]
		} else {
			p.symbols[methodRef.InnerIndex].Link = p.privateGetters[private.Ref]
		}
		p.recordUsage(methodRef)
		ctx.privateMembers = append(ctx.privateMembers, js_ast.Assign(
			js_ast.Expr{Loc: prop.Key.Loc, Data: &js_ast.EIdentifier{Ref: methodRef}},
			prop.ValueOrNil,
		))
		return true
	}

	if key, ok := prop.Key.Data.(*js_ast.EString); ok && helpers.UTF16EqualsString(key.Value, "constructor") {
		if fn, ok := prop.ValueOrNil.Data.(*js_ast.EFunction); ok {
			// Remember where the constructor is for later
			ctx.ctor = fn

			// Initialize TypeScript constructor parameter fields
			if p.options.ts.Parse {
				for _, arg := range ctx.ctor.Fn.Args {
					if arg.IsTypeScriptCtorField {
						if id, ok := arg.Binding.Data.(*js_ast.BIdentifier); ok {
							ctx.parameterFields = append(ctx.parameterFields, js_ast.AssignStmt(
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

type propertyAnalysis struct {
	private                         *js_ast.EPrivateIdentifier
	propExperimentalDecorators      []js_ast.Decorator
	propDecorators                  []js_ast.Decorator
	mustLowerField                  bool
	needsValueOfKey                 bool
	rewriteAutoAccessorToGetSet     bool
	shouldOmitFieldInitializer      bool
	staticFieldToBlockAssign        bool
	isComputedPropertyCopiedOrMoved bool
}

func (ctx *lowerClassContext) analyzeProperty(p *parser, prop js_ast.Property, classLoweringInfo classLoweringInfo) (analysis propertyAnalysis) {
	// The TypeScript class field transform requires removing fields without
	// initializers. If the field is removed, then we only need the key for
	// its side effects and we don't need a temporary reference for the key.
	// However, the TypeScript compiler doesn't remove the field when doing
	// strict class field initialization, so we shouldn't either.
	analysis.private, _ = prop.Key.Data.(*js_ast.EPrivateIdentifier)
	mustLowerPrivate := analysis.private != nil && p.privateSymbolNeedsToBeLowered(analysis.private)
	analysis.shouldOmitFieldInitializer = p.options.ts.Parse && !prop.Kind.IsMethodDefinition() && prop.InitializerOrNil.Data == nil &&
		!ctx.class.UseDefineForClassFields && !mustLowerPrivate && !ctx.class.ShouldLowerStandardDecorators

	// Class fields must be lowered if the environment doesn't support them
	if !prop.Kind.IsMethodDefinition() {
		if prop.Flags.Has(js_ast.PropertyIsStatic) {
			analysis.mustLowerField = classLoweringInfo.lowerAllStaticFields
		} else if prop.Kind == js_ast.PropertyField && p.options.ts.Parse && !ctx.class.UseDefineForClassFields && analysis.private == nil {
			// Lower non-private instance fields (not accessors) if TypeScript's
			// "useDefineForClassFields" setting is disabled. When all such fields
			// have no initializers, we avoid setting the "lowerAllInstanceFields"
			// flag as an optimization because we can just remove all class field
			// declarations in that case without messing with the constructor. But
			// we must set the "mustLowerField" flag here to cause this class field
			// declaration to still be removed.
			analysis.mustLowerField = true
		} else {
			analysis.mustLowerField = classLoweringInfo.lowerAllInstanceFields
		}
	}

	// If the field uses the TypeScript "declare" or "abstract" keyword, just
	// omit it entirely. However, we must still keep any side-effects in the
	// computed value and/or in the decorators.
	if prop.Kind == js_ast.PropertyDeclareOrAbstract && prop.ValueOrNil.Data == nil {
		analysis.mustLowerField = true
		analysis.shouldOmitFieldInitializer = true
	}

	// For convenience, split decorators off into separate fields based on how
	// they will end up being lowered (if they are even being lowered at all)
	if p.options.ts.Parse && p.options.ts.Config.ExperimentalDecorators == config.True {
		analysis.propExperimentalDecorators = prop.Decorators
	} else if ctx.class.ShouldLowerStandardDecorators {
		analysis.propDecorators = prop.Decorators
	}

	// Note: Auto-accessors use a different transform when they are decorated.
	// This transform trades off worse run-time performance for better code size.
	analysis.rewriteAutoAccessorToGetSet = len(analysis.propDecorators) == 0 && prop.Kind == js_ast.PropertyAutoAccessor &&
		(p.options.unsupportedJSFeatures.Has(compat.Decorators) || analysis.mustLowerField)

	// Transform non-lowered static fields that use assign semantics into an
	// assignment in an inline static block instead of lowering them. This lets
	// us avoid having to unnecessarily lower static private fields when
	// "useDefineForClassFields" is disabled.
	analysis.staticFieldToBlockAssign = prop.Kind == js_ast.PropertyField && !analysis.mustLowerField && !ctx.class.UseDefineForClassFields &&
		prop.Flags.Has(js_ast.PropertyIsStatic) && analysis.private == nil

	// Computed properties can't be copied or moved because they have side effects
	// and we don't want to evaluate their side effects twice or change their
	// evaluation order. We'll need to store them in temporary variables to keep
	// their side effects in place when we reference them elsewhere.
	analysis.needsValueOfKey = true
	if prop.Flags.Has(js_ast.PropertyIsComputed) &&
		(len(analysis.propExperimentalDecorators) > 0 ||
			len(analysis.propDecorators) > 0 ||
			analysis.mustLowerField ||
			analysis.staticFieldToBlockAssign ||
			analysis.rewriteAutoAccessorToGetSet) {
		analysis.isComputedPropertyCopiedOrMoved = true

		// Determine if we don't actually need the value of the key (only the side
		// effects). In that case we don't need a temporary variable.
		if len(analysis.propExperimentalDecorators) == 0 &&
			len(analysis.propDecorators) == 0 &&
			!analysis.rewriteAutoAccessorToGetSet &&
			analysis.shouldOmitFieldInitializer {
			analysis.needsValueOfKey = false
		}
	}
	return
}

func (p *parser) propertyNameHint(key js_ast.Expr) string {
	switch k := key.Data.(type) {
	case *js_ast.EString:
		return helpers.UTF16ToString(k.Value)
	case *js_ast.EIdentifier:
		return p.symbols[k.Ref.InnerIndex].OriginalName
	case *js_ast.EPrivateIdentifier:
		return p.symbols[k.Ref.InnerIndex].OriginalName[1:]
	default:
		return ""
	}
}

func (ctx *lowerClassContext) hoistComputedProperties(p *parser, classLoweringInfo classLoweringInfo) (
	propertyKeyTempRefs map[int]ast.Ref, decoratorTempRefs map[int]ast.Ref) {
	var nextComputedPropertyKey *js_ast.Expr

	// Computed property keys must be evaluated in a specific order for their
	// side effects. This order must be preserved even when we have to move a
	// class element around. For example, this can happen when using class fields
	// with computed property keys and targeting environments without class field
	// support. For example:
	//
	//   class Foo {
	//     [a()]() {}
	//     static [b()] = null;
	//     [c()]() {}
	//   }
	//
	// If we need to lower the static field because static fields aren't supported,
	// we still need to ensure that "b()" is called before "a()" and after "c()".
	// That looks something like this:
	//
	//   var _a;
	//   class Foo {
	//     [a()]() {}
	//     [(_a = b(), c())]() {}
	//   }
	//   __publicField(Foo, _a, null);
	//
	// Iterate in reverse so that any initializers are "pushed up" before the
	// class body if there's nowhere else to put them. They can't be "pushed
	// down" into a static block in the class body (the logical place to put
	// them that's next in the evaluation order) because these expressions
	// may contain "await" and static blocks do not allow "await".
	for propIndex := len(ctx.class.Properties) - 1; propIndex >= 0; propIndex-- {
		prop := &ctx.class.Properties[propIndex]
		analysis := ctx.analyzeProperty(p, *prop, classLoweringInfo)

		// Evaluate the decorator expressions inline before computed property keys
		var decorators js_ast.Expr
		if len(analysis.propDecorators) > 0 {
			name := p.propertyNameHint(prop.Key)
			if name != "" {
				name = "_" + name
			}
			name += "_dec"
			ref := p.generateTempRef(tempRefNeedsDeclare, name)
			values := make([]js_ast.Expr, len(analysis.propDecorators))
			for i, decorator := range analysis.propDecorators {
				values[i] = decorator.Value
			}
			atLoc := analysis.propDecorators[0].AtLoc
			decorators = js_ast.Assign(
				js_ast.Expr{Loc: atLoc, Data: &js_ast.EIdentifier{Ref: ref}},
				js_ast.Expr{Loc: atLoc, Data: &js_ast.EArray{Items: values, IsSingleLine: true}})
			p.recordUsage(ref)
			if decoratorTempRefs == nil {
				decoratorTempRefs = make(map[int]ast.Ref)
			}
			decoratorTempRefs[propIndex] = ref
		}

		// Skip property keys that we know are side-effect free
		switch prop.Key.Data.(type) {
		case *js_ast.EString, *js_ast.ENameOfSymbol, *js_ast.ENumber, *js_ast.EPrivateIdentifier:
			// Figure out where to stick the decorator side effects to preserve their order
			if nextComputedPropertyKey != nil {
				// Insert it before everything that comes after it
				*nextComputedPropertyKey = js_ast.JoinWithComma(decorators, *nextComputedPropertyKey)
			} else {
				// Insert it after the first thing that comes before it
				ctx.computedPropertyChain = js_ast.JoinWithComma(decorators, ctx.computedPropertyChain)
			}
			continue

		default:
			// Otherwise, evaluate the decorators right before the property key
			if decorators.Data != nil {
				prop.Key = js_ast.JoinWithComma(decorators, prop.Key)
				prop.Flags |= js_ast.PropertyIsComputed
			}
		}

		// If this key is referenced elsewhere, make sure to still preserve
		// its side effects in the property's original location
		if analysis.isComputedPropertyCopiedOrMoved {
			// If this property is being duplicated instead of moved or removed, then
			// we still need the assignment to the temporary so that we can reference
			// it in multiple places, but we don't have to hoist the assignment to an
			// earlier property (since this property is still there). In that case
			// we can reduce generated code size by avoiding the hoist. One example
			// of this case is a decorator on a class element with a computed
			// property key:
			//
			//   class Foo {
			//     @dec [a()]() {}
			//   }
			//
			// We want to do this:
			//
			//   var _a;
			//   class Foo {
			//     [_a = a()]() {}
			//   }
			//   __decorateClass([dec], Foo.prototype, _a, 1);
			//
			// instead of this:
			//
			//   var _a;
			//   _a = a();
			//   class Foo {
			//     [_a]() {}
			//   }
			//   __decorateClass([dec], Foo.prototype, _a, 1);
			//
			// So only do the hoist if this property is being moved or removed.
			if !analysis.rewriteAutoAccessorToGetSet && (analysis.mustLowerField || analysis.staticFieldToBlockAssign) {
				inlineKey := prop.Key

				if !analysis.needsValueOfKey {
					// In certain cases, we only need to evaluate a property key for its
					// side effects but we don't actually need the value of the key itself.
					// For example, a TypeScript class field without an initializer is
					// omitted when TypeScript's "useDefineForClassFields" setting is false.
				} else {
					// Store the key in a temporary so we can refer to it later
					ref := p.generateTempRef(tempRefNeedsDeclare, "")
					inlineKey = js_ast.Assign(js_ast.Expr{Loc: prop.Key.Loc, Data: &js_ast.EIdentifier{Ref: ref}}, prop.Key)
					p.recordUsage(ref)

					// Replace this property key with a reference to the temporary. We
					// don't need to store the temporary in the "propertyKeyTempRefs"
					// map because all references will refer to the temporary, not just
					// some of them.
					prop.Key = js_ast.Expr{Loc: prop.Key.Loc, Data: &js_ast.EIdentifier{Ref: ref}}
					p.recordUsage(ref)
				}

				// Figure out where to stick this property's side effect to preserve its order
				if nextComputedPropertyKey != nil {
					// Insert it before everything that comes after it
					*nextComputedPropertyKey = js_ast.JoinWithComma(inlineKey, *nextComputedPropertyKey)
				} else {
					// Insert it after the first thing that comes before it
					ctx.computedPropertyChain = js_ast.JoinWithComma(inlineKey, ctx.computedPropertyChain)
				}
				continue
			}

			// Otherwise, we keep the side effects in place (as described above) but
			// just store the key in a temporary so we can refer to it later.
			ref := p.generateTempRef(tempRefNeedsDeclare, "")
			prop.Key = js_ast.Assign(js_ast.Expr{Loc: prop.Key.Loc, Data: &js_ast.EIdentifier{Ref: ref}}, prop.Key)
			p.recordUsage(ref)

			// Use this temporary when creating duplicate references to this key
			if propertyKeyTempRefs == nil {
				propertyKeyTempRefs = make(map[int]ast.Ref)
			}
			propertyKeyTempRefs[propIndex] = ref

			// Deliberately continue to fall through to the "computed" case below:
		}

		// Otherwise, this computed property could be a good location to evaluate
		// something that comes before it. Remember this location for later.
		if prop.Flags.Has(js_ast.PropertyIsComputed) {
			// If any side effects after this were hoisted here, then inline them now.
			// We don't want to reorder any side effects.
			if ctx.computedPropertyChain.Data != nil {
				ref, ok := propertyKeyTempRefs[propIndex]
				if !ok {
					ref = p.generateTempRef(tempRefNeedsDeclare, "")
					prop.Key = js_ast.Assign(js_ast.Expr{Loc: prop.Key.Loc, Data: &js_ast.EIdentifier{Ref: ref}}, prop.Key)
					p.recordUsage(ref)
				}
				prop.Key = js_ast.JoinWithComma(
					js_ast.JoinWithComma(prop.Key, ctx.computedPropertyChain),
					js_ast.Expr{Loc: prop.Key.Loc, Data: &js_ast.EIdentifier{Ref: ref}})
				p.recordUsage(ref)
				ctx.computedPropertyChain = js_ast.Expr{}
			}

			// Remember this location for later
			nextComputedPropertyKey = &prop.Key
		}
	}

	// If any side effects in the class body were hoisted up to the "extends"
	// clause, then inline them before the "extends" clause is evaluated. We
	// don't want to reorder any side effects. For example:
	//
	//   class Foo extends a() {
	//     static [b()]
	//   }
	//
	// We want to do this:
	//
	//   var _a, _b;
	//   class Foo extends (_b = a(), _a = b(), _b) {
	//   }
	//   __publicField(Foo, _a);
	//
	// instead of this:
	//
	//   var _a;
	//   _a = b();
	//   class Foo extends a() {
	//   }
	//   __publicField(Foo, _a);
	//
	if ctx.computedPropertyChain.Data != nil && ctx.class.ExtendsOrNil.Data != nil {
		ctx.extendsRef = p.generateTempRef(tempRefNeedsDeclare, "")
		ctx.class.ExtendsOrNil = js_ast.JoinWithComma(js_ast.JoinWithComma(
			js_ast.Assign(js_ast.Expr{Loc: ctx.class.ExtendsOrNil.Loc, Data: &js_ast.EIdentifier{Ref: ctx.extendsRef}}, ctx.class.ExtendsOrNil),
			ctx.computedPropertyChain),
			js_ast.Expr{Loc: ctx.class.ExtendsOrNil.Loc, Data: &js_ast.EIdentifier{Ref: ctx.extendsRef}})
		p.recordUsage(ctx.extendsRef)
		p.recordUsage(ctx.extendsRef)
		ctx.computedPropertyChain = js_ast.Expr{}
	}
	return
}

// This corresponds to the initialization order in the specification:
//
//  27. For each element e of staticElements, do
//     a. If e is a ClassElementDefinition Record and e.[[Kind]] is not field, then
//
//  28. For each element e of instanceElements, do
//     a. If e.[[Kind]] is not field, then
//
//  29. For each element e of staticElements, do
//     a. If e.[[Kind]] is field, then
//
//  30. For each element e of instanceElements, do
//     a. If e.[[Kind]] is field, then
func fieldOrAccessorOrder(kind js_ast.PropertyKind, flags js_ast.PropertyFlags) (int, bool) {
	if kind == js_ast.PropertyAutoAccessor {
		if flags.Has(js_ast.PropertyIsStatic) {
			return 0, true
		} else {
			return 1, true
		}
	} else if kind == js_ast.PropertyField {
		if flags.Has(js_ast.PropertyIsStatic) {
			return 2, true
		} else {
			return 3, true
		}
	}
	return 0, false
}

func (ctx *lowerClassContext) processProperties(p *parser, classLoweringInfo classLoweringInfo, result visitClassResult) {
	properties := make([]js_ast.Property, 0, len(ctx.class.Properties))
	propertyKeyTempRefs, decoratorTempRefs := ctx.hoistComputedProperties(p, classLoweringInfo)

	// Save the initializer index for each field and accessor element
	if ctx.class.ShouldLowerStandardDecorators {
		var counts [4]int

		// Count how many initializers there are in each section
		for _, prop := range ctx.class.Properties {
			if len(prop.Decorators) > 0 {
				if i, ok := fieldOrAccessorOrder(prop.Kind, prop.Flags); ok {
					counts[i]++
				} else if prop.Flags.Has(js_ast.PropertyIsStatic) {
					ctx.decoratorCallStaticMethodExtraInitializers = true
				} else {
					ctx.decoratorCallInstanceMethodExtraInitializers = true
				}
			}
		}

		// Give each on an index for the order it will be initialized in
		if counts[0] > 0 || counts[1] > 0 || counts[2] > 0 || counts[3] > 0 {
			indices := [4]int{0, counts[0], counts[0] + counts[1], counts[0] + counts[1] + counts[2]}
			ctx.decoratorPropertyToInitializerMap = make(map[int]int)

			for propIndex, prop := range ctx.class.Properties {
				if len(prop.Decorators) > 0 {
					if i, ok := fieldOrAccessorOrder(prop.Kind, prop.Flags); ok {
						ctx.decoratorPropertyToInitializerMap[propIndex] = indices[i]
						indices[i]++
					}
				}
			}
		}
	}

	// Evaluate the decorator expressions inline
	if ctx.class.ShouldLowerStandardDecorators && len(ctx.class.Decorators) > 0 {
		name := ctx.nameToKeep
		if name == "" {
			name = "class"
		}
		decoratorsRef := p.generateTempRef(tempRefNeedsDeclare, fmt.Sprintf("_%s_decorators", name))
		values := make([]js_ast.Expr, len(ctx.class.Decorators))
		for i, decorator := range ctx.class.Decorators {
			values[i] = decorator.Value
		}
		atLoc := ctx.class.Decorators[0].AtLoc
		ctx.computedPropertyChain = js_ast.JoinWithComma(js_ast.Assign(
			js_ast.Expr{Loc: atLoc, Data: &js_ast.EIdentifier{Ref: decoratorsRef}},
			js_ast.Expr{Loc: atLoc, Data: &js_ast.EArray{Items: values, IsSingleLine: true}},
		), ctx.computedPropertyChain)
		p.recordUsage(decoratorsRef)
		ctx.decoratorClassDecorators = js_ast.Expr{Loc: atLoc, Data: &js_ast.EIdentifier{Ref: decoratorsRef}}
		p.recordUsage(decoratorsRef)
		ctx.class.Decorators = nil
	}

	for propIndex, prop := range ctx.class.Properties {
		if prop.Kind == js_ast.PropertyClassStaticBlock {
			// Drop empty class blocks when minifying
			if p.options.minifySyntax && len(prop.ClassStaticBlock.Block.Stmts) == 0 {
				continue
			}

			// Lower this block if needed
			if classLoweringInfo.lowerAllStaticFields {
				ctx.lowerStaticBlock(p, prop.Loc, *prop.ClassStaticBlock)
				continue
			}

			// Otherwise, keep this property
			properties = append(properties, prop)
			continue
		}

		// Merge parameter decorators with method decorators
		if p.options.ts.Parse && prop.Kind.IsMethodDefinition() {
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
							decorators = &ctx.class.Decorators
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

		analysis := ctx.analyzeProperty(p, prop, classLoweringInfo)

		// When the property key needs to be referenced multiple times, subsequent
		// references may need to reference a temporary variable instead of copying
		// the whole property key expression (since we only want to evaluate side
		// effects once).
		keyExprNoSideEffects := prop.Key
		if ref, ok := propertyKeyTempRefs[propIndex]; ok {
			keyExprNoSideEffects.Data = &js_ast.EIdentifier{Ref: ref}
		}

		// Handle TypeScript experimental decorators
		if len(analysis.propExperimentalDecorators) > 0 {
			prop.Decorators = nil

			// Generate a single call to "__decorateClass()" for this property
			loc := prop.Key.Loc

			// This code tells "__decorateClass()" if the descriptor should be undefined
			descriptorKind := float64(1)
			if prop.Kind == js_ast.PropertyField || prop.Kind == js_ast.PropertyDeclareOrAbstract {
				descriptorKind = 2
			}

			// Instance properties use the prototype, static properties use the class
			var target js_ast.Expr
			if prop.Flags.Has(js_ast.PropertyIsStatic) {
				target = ctx.nameFunc()
			} else {
				target = js_ast.Expr{Loc: loc, Data: &js_ast.EDot{Target: ctx.nameFunc(), Name: "prototype", NameLoc: loc}}
			}

			values := make([]js_ast.Expr, len(analysis.propExperimentalDecorators))
			for i, decorator := range analysis.propExperimentalDecorators {
				values[i] = decorator.Value
			}
			decorator := p.callRuntime(loc, "__decorateClass", []js_ast.Expr{
				{Loc: loc, Data: &js_ast.EArray{Items: values}},
				target,
				cloneKeyForLowerClass(keyExprNoSideEffects),
				{Loc: loc, Data: &js_ast.ENumber{Value: descriptorKind}},
			})

			// Static decorators are grouped after instance decorators
			if prop.Flags.Has(js_ast.PropertyIsStatic) {
				ctx.staticExperimentalDecorators = append(ctx.staticExperimentalDecorators, decorator)
			} else {
				ctx.instanceExperimentalDecorators = append(ctx.instanceExperimentalDecorators, decorator)
			}
		}

		// Handle JavaScript decorators
		initializerIndex := -1
		if len(analysis.propDecorators) > 0 {
			prop.Decorators = nil
			loc := prop.Loc
			keyLoc := prop.Key.Loc
			atLoc := analysis.propDecorators[0].AtLoc

			// Encode information about this property using bit flags
			var flags int
			switch prop.Kind {
			case js_ast.PropertyMethod:
				flags = 1
			case js_ast.PropertyGetter:
				flags = 2
			case js_ast.PropertySetter:
				flags = 3
			case js_ast.PropertyAutoAccessor:
				flags = 4
			case js_ast.PropertyField:
				flags = 5
			}
			if flags >= 4 {
				initializerIndex = ctx.decoratorPropertyToInitializerMap[propIndex]
			}
			if prop.Flags.Has(js_ast.PropertyIsStatic) {
				flags |= 8
			}
			if analysis.private != nil {
				flags |= 16
			}

			// Start the arguments for the call to "__decorateElement"
			var key js_ast.Expr
			decoratorsRef := decoratorTempRefs[propIndex]
			if ctx.decoratorContextRef == ast.InvalidRef {
				ctx.decoratorContextRef = p.generateTempRef(tempRefNeedsDeclare, "_init")
			}
			if analysis.private != nil {
				key = js_ast.Expr{Loc: loc, Data: &js_ast.EString{Value: helpers.StringToUTF16(p.symbols[analysis.private.Ref.InnerIndex].OriginalName)}}
			} else {
				key = cloneKeyForLowerClass(keyExprNoSideEffects)
			}
			args := []js_ast.Expr{
				{Loc: loc, Data: &js_ast.EIdentifier{Ref: ctx.decoratorContextRef}},
				{Loc: loc, Data: &js_ast.ENumber{Value: float64(flags)}},
				key,
				{Loc: atLoc, Data: &js_ast.EIdentifier{Ref: decoratorsRef}},
			}
			p.recordUsage(ctx.decoratorContextRef)
			p.recordUsage(decoratorsRef)

			// Append any optional additional arguments
			privateFnRef := ast.InvalidRef
			if analysis.private != nil {
				// Add the "target" argument (the weak set)
				args = append(args, js_ast.Expr{Loc: keyLoc, Data: &js_ast.EIdentifier{Ref: analysis.private.Ref}})
				p.recordUsage(analysis.private.Ref)

				// Add the "extra" argument (the function)
				switch prop.Kind {
				case js_ast.PropertyMethod:
					privateFnRef = p.privateGetters[analysis.private.Ref]
				case js_ast.PropertyGetter:
					privateFnRef = p.privateGetters[analysis.private.Ref]
				case js_ast.PropertySetter:
					privateFnRef = p.privateSetters[analysis.private.Ref]
				}
				if privateFnRef != ast.InvalidRef {
					args = append(args, js_ast.Expr{Loc: keyLoc, Data: &js_ast.EIdentifier{Ref: privateFnRef}})
					p.recordUsage(privateFnRef)
				}
			} else {
				// Add the "target" argument (the class object)
				args = append(args, ctx.nameFunc())
			}

			// Auto-accessors will generate a private field for storage. Lower this
			// field, which will generate a WeakMap instance, and then pass the
			// WeakMap instance into the decorator helper so the lowered getter and
			// setter can use it.
			if prop.Kind == js_ast.PropertyAutoAccessor {
				var kind ast.SymbolKind
				if prop.Flags.Has(js_ast.PropertyIsStatic) {
					kind = ast.SymbolPrivateStaticField
				} else {
					kind = ast.SymbolPrivateField
				}
				ref := p.newSymbol(kind, "#"+p.propertyNameHint(prop.Key))
				p.symbols[ref.InnerIndex].Flags |= ast.PrivateSymbolMustBeLowered
				_, autoAccessorWeakMapRef, _ := ctx.lowerField(p, prop, &js_ast.EPrivateIdentifier{Ref: ref}, false, false, initializerIndex)
				args = append(args, js_ast.Expr{Loc: keyLoc, Data: &js_ast.EIdentifier{Ref: autoAccessorWeakMapRef}})
				p.recordUsage(autoAccessorWeakMapRef)
			}

			// Assign the result
			element := p.callRuntime(loc, "__decorateElement", args)
			if privateFnRef != ast.InvalidRef {
				element = js_ast.Assign(js_ast.Expr{Loc: keyLoc, Data: &js_ast.EIdentifier{Ref: privateFnRef}}, element)
				p.recordUsage(privateFnRef)
			} else if prop.Kind == js_ast.PropertyAutoAccessor && analysis.private != nil {
				ref := p.generateTempRef(tempRefNeedsDeclare, "")
				privateGetFnRef := p.generateTempRef(tempRefNeedsDeclare, "_")
				privateSetFnRef := p.generateTempRef(tempRefNeedsDeclare, "_")
				p.symbols[privateGetFnRef.InnerIndex].Link = p.privateGetters[analysis.private.Ref]
				p.symbols[privateSetFnRef.InnerIndex].Link = p.privateSetters[analysis.private.Ref]

				// Unpack the "get" and "set" properties from the returned property descriptor
				element = js_ast.JoinWithComma(js_ast.JoinWithComma(
					js_ast.Assign(
						js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: ref}},
						element),
					js_ast.Assign(
						js_ast.Expr{Loc: keyLoc, Data: &js_ast.EIdentifier{Ref: privateGetFnRef}},
						js_ast.Expr{Loc: loc, Data: &js_ast.EDot{Target: js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: ref}}, Name: "get", NameLoc: loc}})),
					js_ast.Assign(
						js_ast.Expr{Loc: keyLoc, Data: &js_ast.EIdentifier{Ref: privateSetFnRef}},
						js_ast.Expr{Loc: loc, Data: &js_ast.EDot{Target: js_ast.Expr{Loc: loc, Data: &js_ast.EIdentifier{Ref: ref}}, Name: "set", NameLoc: loc}}))
				p.recordUsage(ref)
				p.recordUsage(privateGetFnRef)
				p.recordUsage(ref)
				p.recordUsage(privateSetFnRef)
				p.recordUsage(ref)
			}

			// Put the call to the decorators in the right place
			if prop.Kind == js_ast.PropertyField {
				// Field
				if prop.Flags.Has(js_ast.PropertyIsStatic) {
					ctx.decoratorStaticFieldElements = append(ctx.decoratorStaticFieldElements, element)
				} else {
					ctx.decoratorInstanceFieldElements = append(ctx.decoratorInstanceFieldElements, element)
				}
			} else {
				// Non-field
				if prop.Flags.Has(js_ast.PropertyIsStatic) {
					ctx.decoratorStaticNonFieldElements = append(ctx.decoratorStaticNonFieldElements, element)
				} else {
					ctx.decoratorInstanceNonFieldElements = append(ctx.decoratorInstanceNonFieldElements, element)
				}
			}

			// Omit decorated auto-accessors as they will be now generated at run-time instead
			if prop.Kind == js_ast.PropertyAutoAccessor {
				if analysis.private != nil {
					ctx.lowerPrivateMethod(p, prop, analysis.private)
				}
				continue
			}
		}

		// Generate get/set methods for auto-accessors
		if analysis.rewriteAutoAccessorToGetSet {
			properties = ctx.rewriteAutoAccessorToGetSet(p, prop, properties, keyExprNoSideEffects, analysis.mustLowerField, analysis.private, result)
			continue
		}

		// Lower fields
		if (!prop.Kind.IsMethodDefinition() && analysis.mustLowerField) || analysis.staticFieldToBlockAssign {
			var keep bool
			prop, _, keep = ctx.lowerField(p, prop, analysis.private, analysis.shouldOmitFieldInitializer, analysis.staticFieldToBlockAssign, initializerIndex)
			if !keep {
				continue
			}
		}

		// Lower methods
		if prop.Kind.IsMethodDefinition() && ctx.lowerMethod(p, prop, analysis.private) {
			continue
		}

		// Keep this property
		properties = append(properties, prop)
	}

	// Finish the filtering operation
	ctx.class.Properties = properties
}

func (ctx *lowerClassContext) lowerStaticBlock(p *parser, loc logger.Loc, block js_ast.ClassStaticBlock) {
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
		ctx.staticMembers = append(ctx.staticMembers, isAllExprs...)
	} else {
		// But if there is a non-expression statement, fall back to using an
		// IIFE since we may be in an expression context and can't use a block.
		ctx.staticMembers = append(ctx.staticMembers, js_ast.Expr{Loc: loc, Data: &js_ast.ECall{
			Target: js_ast.Expr{Loc: loc, Data: &js_ast.EArrow{Body: js_ast.FnBody{
				Loc:   block.Loc,
				Block: block.Block,
			}}},
			CanBeUnwrappedIfUnused: p.astHelpers.StmtsCanBeRemovedIfUnused(block.Block.Stmts, 0),
		}})
	}
}

func (ctx *lowerClassContext) rewriteAutoAccessorToGetSet(
	p *parser,
	prop js_ast.Property,
	properties []js_ast.Property,
	keyExprNoSideEffects js_ast.Expr,
	mustLowerField bool,
	private *js_ast.EPrivateIdentifier,
	result visitClassResult,
) []js_ast.Property {
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
		storageName = "#" + ast.DefaultNameMinifierJS.NumberToMinifiedName(ctx.autoAccessorCount)
		ctx.autoAccessorCount++
	}

	// Generate the symbols we need
	storageRef := p.newSymbol(storageKind, storageName)
	argRef := p.newSymbol(ast.SymbolOther, "_")
	result.bodyScope.Generated = append(result.bodyScope.Generated, storageRef)
	result.bodyScope.Children = append(result.bodyScope.Children, &js_ast.Scope{Kind: js_ast.ScopeFunctionBody, Generated: []ast.Ref{argRef}})

	// Replace this accessor with other properties
	loc := keyExprNoSideEffects.Loc
	storagePrivate := &js_ast.EPrivateIdentifier{Ref: storageRef}
	if mustLowerField {
		// Forward the accessor's lowering status on to the storage field. If we
		// don't do this, then we risk having the underlying private symbol
		// behaving differently than if it were authored manually (e.g. being
		// placed outside of the class body, which is a syntax error).
		p.symbols[storageRef.InnerIndex].Flags |= ast.PrivateSymbolMustBeLowered
	}
	storageNeedsToBeLowered := p.privateSymbolNeedsToBeLowered(storagePrivate)
	storageProp := js_ast.Property{
		Loc:              prop.Loc,
		Kind:             js_ast.PropertyField,
		Flags:            prop.Flags & js_ast.PropertyIsStatic,
		Key:              js_ast.Expr{Loc: loc, Data: storagePrivate},
		InitializerOrNil: prop.InitializerOrNil,
	}
	if !mustLowerField {
		properties = append(properties, storageProp)
	} else if prop, _, ok := ctx.lowerField(p, storageProp, storagePrivate, false, false, -1); ok {
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
		Kind:  js_ast.PropertyGetter,
		Flags: prop.Flags,
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
	if !ctx.lowerMethod(p, getterProp, private) {
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
		Kind:  js_ast.PropertySetter,
		Flags: prop.Flags,
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
	if !ctx.lowerMethod(p, setterProp, private) {
		properties = append(properties, setterProp)
	}
	return properties
}

func (ctx *lowerClassContext) insertInitializersIntoConstructor(p *parser, classLoweringInfo classLoweringInfo, result visitClassResult) {
	if len(ctx.parameterFields) == 0 &&
		!ctx.decoratorCallInstanceMethodExtraInitializers &&
		len(ctx.instancePrivateMethods) == 0 &&
		len(ctx.instanceMembers) == 0 &&
		(ctx.ctor == nil || result.superCtorRef == ast.InvalidRef) {
		// No need to generate a constructor
		return
	}

	// Create a constructor if one doesn't already exist
	if ctx.ctor == nil {
		ctx.ctor = &js_ast.EFunction{Fn: js_ast.Fn{Body: js_ast.FnBody{Loc: ctx.classLoc}}}

		// Append it to the list to reuse existing allocation space
		ctx.class.Properties = append(ctx.class.Properties, js_ast.Property{
			Kind:       js_ast.PropertyMethod,
			Loc:        ctx.classLoc,
			Key:        js_ast.Expr{Loc: ctx.classLoc, Data: &js_ast.EString{Value: helpers.StringToUTF16("constructor")}},
			ValueOrNil: js_ast.Expr{Loc: ctx.classLoc, Data: ctx.ctor},
		})

		// Make sure the constructor has a super() call if needed
		if ctx.class.ExtendsOrNil.Data != nil {
			target := js_ast.Expr{Loc: ctx.classLoc, Data: js_ast.ESuperShared}
			if classLoweringInfo.shimSuperCtorCalls {
				p.recordUsage(result.superCtorRef)
				target.Data = &js_ast.EIdentifier{Ref: result.superCtorRef}
			}
			argumentsRef := p.newSymbol(ast.SymbolUnbound, "arguments")
			p.currentScope.Generated = append(p.currentScope.Generated, argumentsRef)
			ctx.ctor.Fn.Body.Block.Stmts = append(ctx.ctor.Fn.Body.Block.Stmts, js_ast.Stmt{Loc: ctx.classLoc, Data: &js_ast.SExpr{Value: js_ast.Expr{Loc: ctx.classLoc, Data: &js_ast.ECall{
				Target: target,
				Args:   []js_ast.Expr{{Loc: ctx.classLoc, Data: &js_ast.ESpread{Value: js_ast.Expr{Loc: ctx.classLoc, Data: &js_ast.EIdentifier{Ref: argumentsRef}}}}},
			}}}})
		}
	}

	// Run instanceMethodExtraInitializers if needed
	var decoratorInstanceMethodExtraInitializers js_ast.Expr
	if ctx.decoratorCallInstanceMethodExtraInitializers {
		decoratorInstanceMethodExtraInitializers = p.callRuntime(ctx.classLoc, "__runInitializers", []js_ast.Expr{
			{Loc: ctx.classLoc, Data: &js_ast.EIdentifier{Ref: ctx.decoratorContextRef}},
			{Loc: ctx.classLoc, Data: &js_ast.ENumber{Value: (2 << 1) | 1}},
			{Loc: ctx.classLoc, Data: js_ast.EThisShared},
		})
		p.recordUsage(ctx.decoratorContextRef)
	}

	// Make sure the instance field initializers come after "super()" since
	// they need "this" to ba available
	generatedStmts := make([]js_ast.Stmt, 0,
		len(ctx.parameterFields)+
			len(ctx.instancePrivateMethods)+
			len(ctx.instanceMembers))
	generatedStmts = append(generatedStmts, ctx.parameterFields...)
	if decoratorInstanceMethodExtraInitializers.Data != nil {
		generatedStmts = append(generatedStmts, js_ast.Stmt{Loc: decoratorInstanceMethodExtraInitializers.Loc, Data: &js_ast.SExpr{Value: decoratorInstanceMethodExtraInitializers}})
	}
	generatedStmts = append(generatedStmts, ctx.instancePrivateMethods...)
	generatedStmts = append(generatedStmts, ctx.instanceMembers...)
	p.insertStmtsAfterSuperCall(&ctx.ctor.Fn.Body, generatedStmts, result.superCtorRef)

	// Sort the constructor first to match the TypeScript compiler's output
	for i := 0; i < len(ctx.class.Properties); i++ {
		if ctx.class.Properties[i].ValueOrNil.Data == ctx.ctor {
			ctorProp := ctx.class.Properties[i]
			for j := i; j > 0; j-- {
				ctx.class.Properties[j] = ctx.class.Properties[j-1]
			}
			ctx.class.Properties[0] = ctorProp
			break
		}
	}
}

func (ctx *lowerClassContext) finishAndGenerateCode(p *parser, result visitClassResult) ([]js_ast.Stmt, js_ast.Expr) {
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
	mustConvertStmtToExpr := ctx.kind != classKindExpr && p.currentScope.Parent == nil && (p.options.mode == config.ModeBundle || p.willWrapModuleInTryCatchForUsing)

	// Check to see if we have lowered decorators on the class itself
	var classDecorators js_ast.Expr
	var classExperimentalDecorators []js_ast.Decorator
	if p.options.ts.Parse && p.options.ts.Config.ExperimentalDecorators == config.True {
		classExperimentalDecorators = ctx.class.Decorators
		ctx.class.Decorators = nil
	} else if ctx.class.ShouldLowerStandardDecorators {
		classDecorators = ctx.decoratorClassDecorators
	}

	var decorateClassExpr js_ast.Expr
	if classDecorators.Data != nil {
		// Handle JavaScript decorators on the class itself
		if ctx.decoratorContextRef == ast.InvalidRef {
			ctx.decoratorContextRef = p.generateTempRef(tempRefNeedsDeclare, "_init")
		}
		decorateClassExpr = p.callRuntime(ctx.classLoc, "__decorateElement", []js_ast.Expr{
			{Loc: ctx.classLoc, Data: &js_ast.EIdentifier{Ref: ctx.decoratorContextRef}},
			{Loc: ctx.classLoc, Data: &js_ast.ENumber{Value: 0}},
			{Loc: ctx.classLoc, Data: &js_ast.EString{Value: helpers.StringToUTF16(ctx.nameToKeep)}},
			classDecorators,
			ctx.nameFunc(),
		})
		p.recordUsage(ctx.decoratorContextRef)
		decorateClassExpr = js_ast.Assign(ctx.nameFunc(), decorateClassExpr)
	} else if ctx.decoratorContextRef != ast.InvalidRef {
		// Decorator metadata is present if there are any decorators on the class at all
		decorateClassExpr = p.callRuntime(ctx.classLoc, "__decoratorMetadata", []js_ast.Expr{
			{Loc: ctx.classLoc, Data: &js_ast.EIdentifier{Ref: ctx.decoratorContextRef}},
			ctx.nameFunc(),
		})
	}

	// If this is true, we have removed some code from the class body that could
	// potentially contain an expression that captures the inner class name.
	// In this case we must explicitly store the class to a separate inner class
	// name binding to avoid incorrect behavior if the class is later re-assigned,
	// since the removed code will no longer be in the class body scope.
	hasPotentialInnerClassNameEscape := result.innerClassNameRef != ast.InvalidRef &&
		(ctx.computedPropertyChain.Data != nil ||
			len(ctx.privateMembers) > 0 ||
			len(ctx.staticPrivateMethods) > 0 ||
			len(ctx.staticMembers) > 0 ||

			// TypeScript experimental decorators
			len(ctx.instanceExperimentalDecorators) > 0 ||
			len(ctx.staticExperimentalDecorators) > 0 ||
			len(classExperimentalDecorators) > 0 ||

			// JavaScript decorators
			ctx.decoratorContextRef != ast.InvalidRef)

	// If we need to represent the class as an expression (even if it's a
	// statement), then generate another symbol to use as the class name
	nameForClassDecorators := ast.LocRef{Ref: ast.InvalidRef}
	if len(classExperimentalDecorators) > 0 || hasPotentialInnerClassNameEscape || mustConvertStmtToExpr {
		if ctx.kind == classKindExpr {
			// For expressions, the inner and outer class names are the same
			name := ctx.nameFunc()
			nameForClassDecorators = ast.LocRef{Loc: name.Loc, Ref: name.Data.(*js_ast.EIdentifier).Ref}
		} else {
			// For statements we need to use the outer class name, not the inner one
			if ctx.class.Name != nil {
				nameForClassDecorators = *ctx.class.Name
			} else if ctx.kind == classKindExportDefaultStmt {
				nameForClassDecorators = ctx.defaultName
			} else {
				nameForClassDecorators = ast.LocRef{Loc: ctx.classLoc, Ref: p.generateTempRef(tempRefNoDeclare, "")}
			}
			p.recordUsage(nameForClassDecorators.Ref)
		}
	}

	var prefixExprs []js_ast.Expr
	var suffixExprs []js_ast.Expr

	// If there are JavaScript decorators, start by allocating a context object
	if ctx.decoratorContextRef != ast.InvalidRef {
		base := js_ast.Expr{Loc: ctx.classLoc, Data: js_ast.ENullShared}
		if ctx.class.ExtendsOrNil.Data != nil {
			if ctx.extendsRef == ast.InvalidRef {
				ctx.extendsRef = p.generateTempRef(tempRefNeedsDeclare, "")
				ctx.class.ExtendsOrNil = js_ast.Assign(js_ast.Expr{Loc: ctx.class.ExtendsOrNil.Loc, Data: &js_ast.EIdentifier{Ref: ctx.extendsRef}}, ctx.class.ExtendsOrNil)
				p.recordUsage(ctx.extendsRef)
			}
			base.Data = &js_ast.EIdentifier{Ref: ctx.extendsRef}
		}
		suffixExprs = append(suffixExprs, js_ast.Assign(
			js_ast.Expr{Loc: ctx.classLoc, Data: &js_ast.EIdentifier{Ref: ctx.decoratorContextRef}},
			p.callRuntime(ctx.classLoc, "__decoratorStart", []js_ast.Expr{base}),
		))
		p.recordUsage(ctx.decoratorContextRef)
	}

	// Any of the computed property chain that we hoisted out of the class
	// body needs to come before the class expression.
	if ctx.computedPropertyChain.Data != nil {
		prefixExprs = append(prefixExprs, ctx.computedPropertyChain)
	}

	// WeakSets and WeakMaps
	suffixExprs = append(suffixExprs, ctx.privateMembers...)

	// Evaluate JavaScript decorators here
	suffixExprs = append(suffixExprs, ctx.decoratorStaticNonFieldElements...)
	suffixExprs = append(suffixExprs, ctx.decoratorInstanceNonFieldElements...)
	suffixExprs = append(suffixExprs, ctx.decoratorStaticFieldElements...)
	suffixExprs = append(suffixExprs, ctx.decoratorInstanceFieldElements...)

	// Lowered initializers for static methods (including getters and setters)
	suffixExprs = append(suffixExprs, ctx.staticPrivateMethods...)

	// Run JavaScript class decorators at the end of class initialization
	if decorateClassExpr.Data != nil {
		suffixExprs = append(suffixExprs, decorateClassExpr)
	}

	// For each element initializer of staticMethodExtraInitializers
	if ctx.decoratorCallStaticMethodExtraInitializers {
		suffixExprs = append(suffixExprs, p.callRuntime(ctx.classLoc, "__runInitializers", []js_ast.Expr{
			{Loc: ctx.classLoc, Data: &js_ast.EIdentifier{Ref: ctx.decoratorContextRef}},
			{Loc: ctx.classLoc, Data: &js_ast.ENumber{Value: (1 << 1) | 1}},
			ctx.nameFunc(),
		}))
		p.recordUsage(ctx.decoratorContextRef)
	}

	// Lowered initializers for static fields, static accessors, and static blocks
	suffixExprs = append(suffixExprs, ctx.staticMembers...)

	// The official TypeScript compiler adds generated code after the class body
	// in this exact order. Matching this order is important for correctness.
	suffixExprs = append(suffixExprs, ctx.instanceExperimentalDecorators...)
	suffixExprs = append(suffixExprs, ctx.staticExperimentalDecorators...)

	// For each element initializer of classExtraInitializers
	if classDecorators.Data != nil {
		suffixExprs = append(suffixExprs, p.callRuntime(ctx.classLoc, "__runInitializers", []js_ast.Expr{
			{Loc: ctx.classLoc, Data: &js_ast.EIdentifier{Ref: ctx.decoratorContextRef}},
			{Loc: ctx.classLoc, Data: &js_ast.ENumber{Value: (0 << 1) | 1}},
			ctx.nameFunc(),
		}))
		p.recordUsage(ctx.decoratorContextRef)
	}

	// Run TypeScript experimental class decorators at the end of class initialization
	if len(classExperimentalDecorators) > 0 {
		values := make([]js_ast.Expr, len(classExperimentalDecorators))
		for i, decorator := range classExperimentalDecorators {
			values[i] = decorator.Value
		}
		suffixExprs = append(suffixExprs, js_ast.Assign(
			js_ast.Expr{Loc: nameForClassDecorators.Loc, Data: &js_ast.EIdentifier{Ref: nameForClassDecorators.Ref}},
			p.callRuntime(ctx.classLoc, "__decorateClass", []js_ast.Expr{
				{Loc: ctx.classLoc, Data: &js_ast.EArray{Items: values}},
				{Loc: nameForClassDecorators.Loc, Data: &js_ast.EIdentifier{Ref: nameForClassDecorators.Ref}},
			}),
		))
		p.recordUsage(nameForClassDecorators.Ref)
		p.recordUsage(nameForClassDecorators.Ref)
	}

	// Our caller expects us to return the same form that was originally given to
	// us. If the class was originally an expression, then return an expression.
	if ctx.kind == classKindExpr {
		// Calling "nameFunc" will replace "classExpr", so make sure to do that first
		// before joining "classExpr" with any other expressions
		var nameToJoin js_ast.Expr
		if ctx.didCaptureClassExpr || len(suffixExprs) > 0 {
			nameToJoin = ctx.nameFunc()
		}

		// Insert expressions on either side of the class as appropriate
		ctx.classExpr = js_ast.JoinWithComma(js_ast.JoinAllWithComma(prefixExprs), ctx.classExpr)
		ctx.classExpr = js_ast.JoinWithComma(ctx.classExpr, js_ast.JoinAllWithComma(suffixExprs))

		// Finally join "classExpr" with the variable that holds the class object
		ctx.classExpr = js_ast.JoinWithComma(ctx.classExpr, nameToJoin)
		if ctx.wrapFunc != nil {
			ctx.classExpr = ctx.wrapFunc(ctx.classExpr)
		}
		return nil, ctx.classExpr
	}

	// Otherwise, the class was originally a statement. Return an array of
	// statements instead.
	var stmts []js_ast.Stmt
	var outerClassNameDecl js_ast.Stmt

	// Insert expressions before the class as appropriate
	for _, expr := range prefixExprs {
		stmts = append(stmts, js_ast.Stmt{Loc: expr.Loc, Data: &js_ast.SExpr{Value: expr}})
	}

	// Handle converting a class statement to a class expression
	if nameForClassDecorators.Ref != ast.InvalidRef {
		classExpr := js_ast.EClass{Class: *ctx.class}
		ctx.class = &classExpr.Class
		init := js_ast.Expr{Loc: ctx.classLoc, Data: &classExpr}

		// If the inner class name was referenced, then set the name of the class
		// that we will end up printing to the inner class name. Otherwise if the
		// inner class name was unused, we can just leave it blank.
		if result.innerClassNameRef != ast.InvalidRef {
			// "class Foo { x = Foo }" => "const Foo = class _Foo { x = _Foo }"
			ctx.class.Name.Ref = result.innerClassNameRef
		} else {
			// "class Foo {}" => "const Foo = class {}"
			ctx.class.Name = nil
		}

		// Generate the class initialization statement
		if len(classExperimentalDecorators) > 0 {
			// If there are class decorators, then we actually need to mutate the
			// immutable "const" binding that shadows everything in the class body.
			// The official TypeScript compiler does this by rewriting all class name
			// references in the class body to another temporary variable. This is
			// basically what we're doing here.
			p.recordUsage(nameForClassDecorators.Ref)
			stmts = append(stmts, js_ast.Stmt{Loc: ctx.classLoc, Data: &js_ast.SLocal{
				Kind:     p.selectLocalKind(js_ast.LocalLet),
				IsExport: ctx.kind == classKindExportStmt,
				Decls: []js_ast.Decl{{
					Binding:    js_ast.Binding{Loc: nameForClassDecorators.Loc, Data: &js_ast.BIdentifier{Ref: nameForClassDecorators.Ref}},
					ValueOrNil: init,
				}},
			}})
			if ctx.class.Name != nil {
				p.mergeSymbols(ctx.class.Name.Ref, nameForClassDecorators.Ref)
				ctx.class.Name = nil
			}
		} else if hasPotentialInnerClassNameEscape {
			// If the inner class name was used, then we explicitly generate a binding
			// for it. That means the mutable outer class name is separate, and is
			// initialized after all static member initializers have finished.
			captureRef := p.newSymbol(ast.SymbolOther, p.symbols[result.innerClassNameRef.InnerIndex].OriginalName)
			p.currentScope.Generated = append(p.currentScope.Generated, captureRef)
			p.recordDeclaredSymbol(captureRef)
			p.mergeSymbols(result.innerClassNameRef, captureRef)
			kind := js_ast.LocalConst
			if classDecorators.Data != nil {
				// Class decorators need to be able to potentially mutate this binding
				kind = js_ast.LocalLet
			}
			stmts = append(stmts, js_ast.Stmt{Loc: ctx.classLoc, Data: &js_ast.SLocal{
				Kind: p.selectLocalKind(kind),
				Decls: []js_ast.Decl{{
					Binding:    js_ast.Binding{Loc: nameForClassDecorators.Loc, Data: &js_ast.BIdentifier{Ref: captureRef}},
					ValueOrNil: init,
				}},
			}})
			p.recordUsage(nameForClassDecorators.Ref)
			p.recordUsage(captureRef)
			outerClassNameDecl = js_ast.Stmt{Loc: ctx.classLoc, Data: &js_ast.SLocal{
				Kind:     p.selectLocalKind(js_ast.LocalLet),
				IsExport: ctx.kind == classKindExportStmt,
				Decls: []js_ast.Decl{{
					Binding:    js_ast.Binding{Loc: nameForClassDecorators.Loc, Data: &js_ast.BIdentifier{Ref: nameForClassDecorators.Ref}},
					ValueOrNil: js_ast.Expr{Loc: ctx.classLoc, Data: &js_ast.EIdentifier{Ref: captureRef}},
				}},
			}}
		} else {
			// Otherwise, the inner class name isn't needed and we can just
			// use a single variable declaration for the outer class name.
			p.recordUsage(nameForClassDecorators.Ref)
			stmts = append(stmts, js_ast.Stmt{Loc: ctx.classLoc, Data: &js_ast.SLocal{
				Kind:     p.selectLocalKind(js_ast.LocalLet),
				IsExport: ctx.kind == classKindExportStmt,
				Decls: []js_ast.Decl{{
					Binding:    js_ast.Binding{Loc: nameForClassDecorators.Loc, Data: &js_ast.BIdentifier{Ref: nameForClassDecorators.Ref}},
					ValueOrNil: init,
				}},
			}})
		}
	} else {
		// Generate the specific kind of class statement that was passed in to us
		switch ctx.kind {
		case classKindStmt:
			stmts = append(stmts, js_ast.Stmt{Loc: ctx.classLoc, Data: &js_ast.SClass{Class: *ctx.class}})
		case classKindExportStmt:
			stmts = append(stmts, js_ast.Stmt{Loc: ctx.classLoc, Data: &js_ast.SClass{Class: *ctx.class, IsExport: true}})
		case classKindExportDefaultStmt:
			stmts = append(stmts, js_ast.Stmt{Loc: ctx.classLoc, Data: &js_ast.SExportDefault{
				DefaultName: ctx.defaultName,
				Value:       js_ast.Stmt{Loc: ctx.classLoc, Data: &js_ast.SClass{Class: *ctx.class}},
			}})
		}

		// The inner class name inside the class statement should be the same as
		// the class statement name itself
		if ctx.class.Name != nil && result.innerClassNameRef != ast.InvalidRef {
			// If the class body contains a direct eval call, then the inner class
			// name will be marked as "MustNotBeRenamed" (because we have already
			// popped the class body scope) but the outer class name won't be marked
			// as "MustNotBeRenamed" yet (because we haven't yet popped the containing
			// scope). Propagate this flag now before we merge these symbols so we
			// don't end up accidentally renaming the outer class name to the inner
			// class name.
			if p.currentScope.ContainsDirectEval {
				p.symbols[ctx.class.Name.Ref.InnerIndex].Flags |= (p.symbols[result.innerClassNameRef.InnerIndex].Flags & ast.MustNotBeRenamed)
			}
			p.mergeSymbols(result.innerClassNameRef, ctx.class.Name.Ref)
		}
	}

	// Insert expressions after the class as appropriate
	for _, expr := range suffixExprs {
		stmts = append(stmts, js_ast.Stmt{Loc: expr.Loc, Data: &js_ast.SExpr{Value: expr}})
	}

	// This must come after the class body initializers have finished
	if outerClassNameDecl.Data != nil {
		stmts = append(stmts, outerClassNameDecl)
	}

	if nameForClassDecorators.Ref != ast.InvalidRef && ctx.kind == classKindExportDefaultStmt {
		// "export default class x {}" => "class x {} export {x as default}"
		stmts = append(stmts, js_ast.Stmt{Loc: ctx.classLoc, Data: &js_ast.SExportClause{
			Items: []js_ast.ClauseItem{{Alias: "default", Name: ctx.defaultName}},
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
	//     return this;
	//   };
	//
	argsRef := p.newSymbol(ast.SymbolOther, "args")
	p.currentScope.Generated = append(p.currentScope.Generated, argsRef)
	p.recordUsage(argsRef)
	superCall := js_ast.Expr{Loc: body.Loc, Data: &js_ast.ECall{
		Target: js_ast.Expr{Loc: body.Loc, Data: js_ast.ESuperShared},
		Args:   []js_ast.Expr{{Loc: body.Loc, Data: &js_ast.ESpread{Value: js_ast.Expr{Loc: body.Loc, Data: &js_ast.EIdentifier{Ref: argsRef}}}}},
	}}
	stmtsToInsert = append(append(
		[]js_ast.Stmt{{Loc: body.Loc, Data: &js_ast.SExpr{Value: superCall}}},
		stmtsToInsert...),
		js_ast.Stmt{Loc: body.Loc, Data: &js_ast.SReturn{ValueOrNil: js_ast.Expr{Loc: body.Loc, Data: js_ast.EThisShared}}},
	)
	if p.options.minifySyntax {
		stmtsToInsert = p.mangleStmts(stmtsToInsert, stmtsFnBody)
	}
	body.Block.Stmts = append([]js_ast.Stmt{{Loc: body.Loc, Data: &js_ast.SLocal{Decls: []js_ast.Decl{{
		Binding: js_ast.Binding{Loc: body.Loc, Data: &js_ast.BIdentifier{Ref: superCtorRef}}, ValueOrNil: js_ast.Expr{Loc: body.Loc, Data: &js_ast.EArrow{
			HasRestArg: true,
			PreferExpr: true,
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
