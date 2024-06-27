package runtime

// This is esbuild's runtime code. It contains helper functions that are
// automatically injected into output files to implement certain features. For
// example, the "**" operator is replaced with a call to "__pow" when targeting
// ES2015. Tree shaking automatically removes unused code from the runtime.

import (
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/logger"
)

// The runtime source is always at a special index. The index is always zero
// but this constant is always used instead to improve readability and ensure
// all code that references this index can be discovered easily.
const SourceIndex = uint32(0)

func Source(unsupportedJSFeatures compat.JSFeature) logger.Source {
	// Note: These helper functions used to be named similar things to the helper
	// functions from the TypeScript compiler. However, people sometimes use these
	// two projects in combination and TypeScript's implementation of these helpers
	// causes name collisions. Some examples:
	//
	// * The "tslib" library will overwrite esbuild's helper functions if the bundled
	//   code is run in the global scope: https://github.com/evanw/esbuild/issues/1102
	//
	// * Running the TypeScript compiler on esbuild's output to convert ES6 to ES5
	//   will also overwrite esbuild's helper functions because TypeScript doesn't
	//   change the names of its helper functions to avoid name collisions:
	//   https://github.com/microsoft/TypeScript/issues/43296
	//
	// These can both be considered bugs in TypeScript. However, they are unlikely
	// to be fixed and it's simplest to just avoid using the same names to avoid
	// these bugs. Forbidden names (from "tslib"):
	//
	//   __assign
	//   __asyncDelegator
	//   __asyncGenerator
	//   __asyncValues
	//   __await
	//   __awaiter
	//   __classPrivateFieldGet
	//   __classPrivateFieldSet
	//   __createBinding
	//   __decorate
	//   __exportStar
	//   __extends
	//   __generator
	//   __importDefault
	//   __importStar
	//   __makeTemplateObject
	//   __metadata
	//   __param
	//   __read
	//   __rest
	//   __spread
	//   __spreadArray
	//   __spreadArrays
	//   __values
	//
	// Note: The "__objRest" function has a for-of loop which requires ES6, but
	// transforming destructuring to ES5 isn't even supported so it's ok.
	text := `
		var __create = Object.create
		var __freeze = Object.freeze
		var __defProp = Object.defineProperty
		var __defProps = Object.defineProperties
		var __getOwnPropDesc = Object.getOwnPropertyDescriptor // Note: can return "undefined" due to a Safari bug
		var __getOwnPropDescs = Object.getOwnPropertyDescriptors
		var __getOwnPropNames = Object.getOwnPropertyNames
		var __getOwnPropSymbols = Object.getOwnPropertySymbols
		var __getProtoOf = Object.getPrototypeOf
		var __hasOwnProp = Object.prototype.hasOwnProperty
		var __propIsEnum = Object.prototype.propertyIsEnumerable
		var __reflectGet = Reflect.get
		var __reflectSet = Reflect.set

		var __knownSymbol = (name, symbol) => (symbol = Symbol[name]) ? symbol : Symbol.for('Symbol.' + name)
		var __typeError = msg => { throw TypeError(msg) }

		export var __pow = Math.pow

		var __defNormalProp = (obj, key, value) => key in obj
			? __defProp(obj, key, {enumerable: true, configurable: true, writable: true, value})
			: obj[key] = value

		export var __spreadValues = (a, b) => {
			for (var prop in b ||= {})
				if (__hasOwnProp.call(b, prop))
					__defNormalProp(a, prop, b[prop])
			if (__getOwnPropSymbols)
		`

	// Avoid "of" when not using ES6
	if !unsupportedJSFeatures.Has(compat.ForOf) {
		text += `
				for (var prop of __getOwnPropSymbols(b)) {
		`
	} else {
		text += `
				for (var props = __getOwnPropSymbols(b), i = 0, n = props.length, prop; i < n; i++) {
					prop = props[i]
		`
	}

	text += `
					if (__propIsEnum.call(b, prop))
						__defNormalProp(a, prop, b[prop])
				}
			return a
		}
		export var __spreadProps = (a, b) => __defProps(a, __getOwnPropDescs(b))

		// Update the "name" property on the function or class for "--keep-names"
		export var __name = (target, value) => __defProp(target, 'name', { value, configurable: true })

		// This fallback "require" function exists so that "typeof require" can
		// naturally be "function" even in non-CommonJS environments since esbuild
		// emulates a CommonJS environment (issue #1202). However, people want this
		// shim to fall back to "globalThis.require" even if it's defined later
		// (including property accesses such as "require.resolve") so we need to
		// use a proxy (issue #1614).
		export var __require =
			/* @__PURE__ */ (x =>
				typeof require !== 'undefined' ? require :
				typeof Proxy !== 'undefined' ? new Proxy(x, {
					get: (a, b) => (typeof require !== 'undefined' ? require : a)[b]
				}) : x
			)(function(x) {
				if (typeof require !== 'undefined') return require.apply(this, arguments)
				throw Error('Dynamic require of "' + x + '" is not supported')
			})

		// This is used for glob imports
		export var __glob = map => path => {
			var fn = map[path]
			if (fn) return fn()
			throw new Error('Module not found in bundle: ' + path)
		}

		// For object rest patterns
		export var __restKey = key => typeof key === 'symbol' ? key : key + ''
		export var __objRest = (source, exclude) => {
			var target = {}
			for (var prop in source)
				if (__hasOwnProp.call(source, prop) && exclude.indexOf(prop) < 0)
					target[prop] = source[prop]
			if (source != null && __getOwnPropSymbols)
	`

	// Avoid "of" when not using ES6
	if !unsupportedJSFeatures.Has(compat.ForOf) {
		text += `
				for (var prop of __getOwnPropSymbols(source)) {
		`
	} else {
		text += `
				for (var props = __getOwnPropSymbols(source), i = 0, n = props.length, prop; i < n; i++) {
					prop = props[i]
		`
	}

	text += `
					if (exclude.indexOf(prop) < 0 && __propIsEnum.call(source, prop))
						target[prop] = source[prop]
				}
			return target
		}

		// This is for lazily-initialized ESM code. This has two implementations, a
		// compact one for minified code and a verbose one that generates friendly
		// names in V8's profiler and in stack traces.
		export var __esm = (fn, res) => function __init() {
			return fn && (res = (0, fn[__getOwnPropNames(fn)[0]])(fn = 0)), res
		}
		export var __esmMin = (fn, res) => () => (fn && (res = fn(fn = 0)), res)

		// Wraps a CommonJS closure and returns a require() function. This has two
		// implementations, a compact one for minified code and a verbose one that
		// generates friendly names in V8's profiler and in stack traces.
		export var __commonJS = (cb, mod) => function __require() {
			return mod || (0, cb[__getOwnPropNames(cb)[0]])((mod = {exports: {}}).exports, mod), mod.exports
		}
		export var __commonJSMin = (cb, mod) => () => (mod || cb((mod = {exports: {}}).exports, mod), mod.exports)

		// Used to implement ESM exports both for "require()" and "import * as"
		export var __export = (target, all) => {
			for (var name in all)
				__defProp(target, name, { get: all[name], enumerable: true })
		}

		var __copyProps = (to, from, except, desc) => {
			if (from && typeof from === 'object' || typeof from === 'function')
	`

	// Avoid "let" when not using ES6
	if !unsupportedJSFeatures.Has(compat.ForOf) && !unsupportedJSFeatures.Has(compat.ConstAndLet) {
		text += `
				for (let key of __getOwnPropNames(from))
					if (!__hasOwnProp.call(to, key) && key !== except)
						__defProp(to, key, { get: () => from[key], enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable })
		`
	} else {
		text += `
				for (var keys = __getOwnPropNames(from), i = 0, n = keys.length, key; i < n; i++) {
					key = keys[i]
					if (!__hasOwnProp.call(to, key) && key !== except)
						__defProp(to, key, { get: (k => from[k]).bind(null, key), enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable })
				}
		`
	}

	text += `
			return to
		}

		// This is used to implement "export * from" statements. It copies properties
		// from the imported module to the current module's ESM export object. If the
		// current module is an entry point and the target format is CommonJS, we
		// also copy the properties to "module.exports" in addition to our module's
		// internal ESM export object.
		export var __reExport = (target, mod, secondTarget) => (
			__copyProps(target, mod, 'default'),
			secondTarget && __copyProps(secondTarget, mod, 'default')
		)

		// Converts the module from CommonJS to ESM. When in node mode (i.e. in an
		// ".mjs" file, package.json has "type: module", or the "__esModule" export
		// in the CommonJS file is falsy or missing), the "default" property is
		// overridden to point to the original CommonJS exports object instead.
		export var __toESM = (mod, isNodeMode, target) => (
			target = mod != null ? __create(__getProtoOf(mod)) : {},
			__copyProps(
				// If the importer is in node compatibility mode or this is not an ESM
				// file that has been converted to a CommonJS file using a Babel-
				// compatible transform (i.e. "__esModule" has not been set), then set
				// "default" to the CommonJS "module.exports" for node compatibility.
				isNodeMode || !mod || !mod.__esModule
					? __defProp(target, 'default', { value: mod, enumerable: true })
					: target,
				mod)
		)

		// Converts the module from ESM to CommonJS. This clones the input module
		// object with the addition of a non-enumerable "__esModule" property set
		// to "true", which overwrites any existing export named "__esModule".
		export var __toCommonJS = mod => __copyProps(__defProp({}, '__esModule', { value: true }), mod)

		// For TypeScript experimental decorators
		// - kind === undefined: class
		// - kind === 1: method, parameter
		// - kind === 2: field
		export var __decorateClass = (decorators, target, key, kind) => {
			var result = kind > 1 ? void 0 : kind ? __getOwnPropDesc(target, key) : target
			for (var i = decorators.length - 1, decorator; i >= 0; i--)
				if (decorator = decorators[i])
					result = (kind ? decorator(target, key, result) : decorator(result)) || result
			if (kind && result) __defProp(target, key, result)
			return result
		}
		export var __decorateParam = (index, decorator) => (target, key) => decorator(target, key, index)

		// For JavaScript decorators
		export var __decoratorStart = base => [, , , __create(base?.[__knownSymbol('metadata')] ?? null)]
		var __decoratorStrings = ['class', 'method', 'getter', 'setter', 'accessor', 'field', 'value', 'get', 'set']
		var __expectFn = fn => fn !== void 0 && typeof fn !== 'function' ? __typeError('Function expected') : fn
		var __decoratorContext = (kind, name, done, metadata, fns) => ({ kind: __decoratorStrings[kind], name, metadata, addInitializer: fn =>
			done._ ? __typeError('Already initialized') : fns.push(__expectFn(fn || null)) })
		export var __decoratorMetadata = (array, target) => __defNormalProp(target, __knownSymbol('metadata'), array[3])
		export var __runInitializers = (array, flags, self, value) => {
			for (var i = 0, fns = array[flags >> 1], n = fns && fns.length; i < n; i++) flags & 1 ? fns[i].call(self) : value = fns[i].call(self, value)
			return value
		}
		export var __decorateElement = (array, flags, name, decorators, target, extra) => {
			var fn, it, done, ctx, access, k = flags & 7, s = !!(flags & 8), p = !!(flags & 16)
			var j = k > 3 ? array.length + 1 : k ? s ? 1 : 2 : 0, key = __decoratorStrings[k + 5]
			var initializers = k > 3 && (array[j - 1] = []), extraInitializers = array[j] || (array[j] = [])
			var desc = k && (
				!p && !s && (target = target.prototype),
				k < 5 && (k > 3 || !p) &&
			`

	// Avoid object extensions when not using ES6
	if !unsupportedJSFeatures.Has(compat.ObjectExtensions) && !unsupportedJSFeatures.Has(compat.ObjectAccessors) {
		text += `__getOwnPropDesc(k < 4 ? target : { get [name]() { return __privateGet(this, extra) }, set [name](x) { return __privateSet(this, extra, x) } }, name)`
	} else {
		text += `(k < 4 ? __getOwnPropDesc(target, name) : { get: () => __privateGet(this, extra), set: x => __privateSet(this, extra, x) })`
	}

	text += `
			)
			k ? p && k < 4 && __name(extra, (k > 2 ? 'set ' : k > 1 ? 'get ' : '') + name) : __name(target, name)

			for (var i = decorators.length - 1; i >= 0; i--) {
				ctx = __decoratorContext(k, name, done = {}, array[3], extraInitializers)

				if (k) {
					ctx.static = s, ctx.private = p, access = ctx.access = { has: p ? x => __privateIn(target, x) : x => name in x }
					if (k ^ 3) access.get = p ? x => (k ^ 1 ? __privateGet : __privateMethod)(x, target, k ^ 4 ? extra : desc.get) : x => x[name]
					if (k > 2) access.set = p ? (x, y) => __privateSet(x, target, y, k ^ 4 ? extra : desc.set) : (x, y) => x[name] = y
				}

				it = (0, decorators[i])(k ? k < 4 ? p ? extra : desc[key] : k > 4 ? void 0 : { get: desc.get, set: desc.set } : target, ctx), done._ = 1

				if (k ^ 4 || it === void 0) __expectFn(it) && (k > 4 ? initializers.unshift(it) : k ? p ? extra = it : desc[key] = it : target = it)
				else if (typeof it !== 'object' || it === null) __typeError('Object expected')
				else __expectFn(fn = it.get) && (desc.get = fn), __expectFn(fn = it.set) && (desc.set = fn), __expectFn(fn = it.init) && initializers.unshift(fn)
			}

			return k || __decoratorMetadata(array, target),
				desc && __defProp(target, name, desc),
				p ? k ^ 4 ? extra : desc : target
		}

		// For class members
		export var __publicField = (obj, key, value) => (
			__defNormalProp(obj, typeof key !== 'symbol' ? key + '' : key, value)
		)
		var __accessCheck = (obj, member, msg) => (
			member.has(obj) || __typeError('Cannot ' + msg)
		)
		export var __privateIn = (member, obj) => (
			Object(obj) !== obj ? __typeError('Cannot use the "in" operator on this value') :
			member.has(obj)
		)
		export var __privateGet = (obj, member, getter) => (
			__accessCheck(obj, member, 'read from private field'),
			getter ? getter.call(obj) : member.get(obj)
		)
		export var __privateAdd = (obj, member, value) => (
			member.has(obj) ? __typeError('Cannot add the same private member more than once') :
			member instanceof WeakSet ? member.add(obj) : member.set(obj, value)
		)
		export var __privateSet = (obj, member, value, setter) => (
			__accessCheck(obj, member, 'write to private field'),
			setter ? setter.call(obj, value) : member.set(obj, value),
			value
		)
		export var __privateMethod = (obj, member, method) => (
			__accessCheck(obj, member, 'access private method'),
			method
		)
		export var __earlyAccess = (name) => {
			throw ReferenceError('Cannot access "' + name + '" before initialization')
		}
	`

	if !unsupportedJSFeatures.Has(compat.ObjectAccessors) {
		text += `
			export var __privateWrapper = (obj, member, setter, getter) => ({
				set _(value) { __privateSet(obj, member, value, setter) },
				get _() { return __privateGet(obj, member, getter) },
			})
		`
	} else {
		text += `
		export var __privateWrapper = (obj, member, setter, getter) => __defProp({}, '_', {
			set: value => __privateSet(obj, member, value, setter),
			get: () => __privateGet(obj, member, getter),
		})
		`
	}

	text += `
		// For "super" property accesses
		export var __superGet = (cls, obj, key) => __reflectGet(__getProtoOf(cls), key, obj)
		export var __superSet = (cls, obj, key, val) => (__reflectSet(__getProtoOf(cls), key, val, obj), val)
	`

	if !unsupportedJSFeatures.Has(compat.ObjectAccessors) {
		text += `
			export var __superWrapper = (cls, obj, key) => ({
				get _() { return __superGet(cls, obj, key) },
				set _(val) { __superSet(cls, obj, key, val) },
			})
		`
	} else {
		text += `
			export var __superWrapper = (cls, obj, key) => __defProp({}, '_', {
				get: () => __superGet(cls, obj, key),
				set: val => __superSet(cls, obj, key, val),
			})
		`
	}

	text += `
		// For lowering tagged template literals
		export var __template = (cooked, raw) => __freeze(__defProp(cooked, 'raw', { value: __freeze(raw || cooked.slice()) }))

		// This helps for lowering async functions
		export var __async = (__this, __arguments, generator) => {
			return new Promise((resolve, reject) => {
				var fulfilled = value => {
					try {
						step(generator.next(value))
					} catch (e) {
						reject(e)
					}
				}
				var rejected = value => {
					try {
						step(generator.throw(value))
					} catch (e) {
						reject(e)
					}
				}
				var step = x => x.done ? resolve(x.value) : Promise.resolve(x.value).then(fulfilled, rejected)
				step((generator = generator.apply(__this, __arguments)).next())
			})
		}

		// These help for lowering async generator functions
		export var __await = function (promise, isYieldStar) {
			this[0] = promise
			this[1] = isYieldStar
		}
		export var __asyncGenerator = (__this, __arguments, generator) => {
			var resume = (k, v, yes, no) => {
				try {
					var x = generator[k](v), isAwait = (v = x.value) instanceof __await, done = x.done
					Promise.resolve(isAwait ? v[0] : v)
						.then(y => isAwait
							? resume(k === 'return' ? k : 'next', v[1] ? { done: y.done, value: y.value } : y, yes, no)
							: yes({ value: y, done }))
						.catch(e => resume('throw', e, yes, no))
				} catch (e) {
					no(e)
				}
			}, method = k => it[k] = x => new Promise((yes, no) => resume(k, x, yes, no)), it = {}
			return generator = generator.apply(__this, __arguments),
				it[__knownSymbol('asyncIterator')] = () => it,
				method('next'),
				method('throw'),
				method('return'),
				it
		}
		export var __yieldStar = value => {
			var obj = value[__knownSymbol('asyncIterator')], isAwait = false, method, it = {}
			if (obj == null) {
				obj = value[__knownSymbol('iterator')]()
				method = k => it[k] = x => obj[k](x)
			} else {
				obj = obj.call(value)
				method = k => it[k] = v => {
					if (isAwait) {
						isAwait = false
						if (k === 'throw') throw v
						return v
					}
					isAwait = true
					return {
						done: false,
						value: new __await(new Promise(resolve => {
							var x = obj[k](v)
							if (!(x instanceof Object)) __typeError('Object expected')
							resolve(x)
						}), 1),
					}
				}
			}
			return it[__knownSymbol('iterator')] = () => it,
				method('next'),
				'throw' in obj ? method('throw') : it.throw = x => { throw x },
				'return' in obj && method('return'),
				it
		}

		// This helps for lowering for-await loops
		export var __forAwait = (obj, it, method) =>
			(it = obj[__knownSymbol('asyncIterator')])
				? it.call(obj)
				: (obj = obj[__knownSymbol('iterator')](),
					it = {},
					method = (key, fn) =>
						(fn = obj[key]) && (it[key] = arg =>
							new Promise((yes, no, done) => (
								arg = fn.call(obj, arg),
								done = arg.done,
								Promise.resolve(arg.value)
									.then(value => yes({ value, done }), no)
							))),
					method('next'),
					method('return'),
					it)

		// This is for the "binary" loader (custom code is ~2x faster than "atob")
		export var __toBinaryNode = base64 => new Uint8Array(Buffer.from(base64, 'base64'))
		export var __toBinary = /* @__PURE__ */ (() => {
			var table = new Uint8Array(128)
			for (var i = 0; i < 64; i++) table[i < 26 ? i + 65 : i < 52 ? i + 71 : i < 62 ? i - 4 : i * 4 - 205] = i
			return base64 => {
				var n = base64.length, bytes = new Uint8Array((n - (base64[n - 1] == '=') - (base64[n - 2] == '=')) * 3 / 4 | 0)
				for (var i = 0, j = 0; i < n;) {
					var c0 = table[base64.charCodeAt(i++)], c1 = table[base64.charCodeAt(i++)]
					var c2 = table[base64.charCodeAt(i++)], c3 = table[base64.charCodeAt(i++)]
					bytes[j++] = (c0 << 2) | (c1 >> 4)
					bytes[j++] = (c1 << 4) | (c2 >> 2)
					bytes[j++] = (c2 << 6) | c3
				}
				return bytes
			}
		})()

		// These are for the "using" statement in TypeScript 5.2+
		export var __using = (stack, value, async) => {
			if (value != null) {
				if (typeof value !== 'object' && typeof value !== 'function') __typeError('Object expected')
				var dispose, inner
				if (async) dispose = value[__knownSymbol('asyncDispose')]
				if (dispose === void 0) {
					dispose = value[__knownSymbol('dispose')]
					if (async) inner = dispose
				}
				if (typeof dispose !== 'function') __typeError('Object not disposable')
				if (inner) dispose = function() { try { inner.call(this) } catch (e) { return Promise.reject(e) } }
				stack.push([async, dispose, value])
			} else if (async) {
				stack.push([async])
			}
			return value
		}
		export var __callDispose = (stack, error, hasError) => {
			var E = typeof SuppressedError === 'function' ? SuppressedError :
				function (e, s, m, _) { return _ = Error(m), _.name = 'SuppressedError', _.error = e, _.suppressed = s, _ }
			var fail = e => error = hasError ? new E(e, error, 'An error was suppressed during disposal') : (hasError = true, e)
			var next = (it) => {
				while (it = stack.pop()) {
					try {
						var result = it[1] && it[1].call(it[2])
						if (it[0]) return Promise.resolve(result).then(next, (e) => (fail(e), next()))
					} catch (e) {
						fail(e)
					}
				}
				if (hasError) throw error
			}
			return next()
		}
	`

	return logger.Source{
		Index:          SourceIndex,
		KeyPath:        logger.Path{Text: "<runtime>"},
		PrettyPath:     "<runtime>",
		IdentifierName: "runtime",
		Contents:       text,
	}
}

// The TypeScript decorator transform behaves similar to the official
// TypeScript compiler.
//
// One difference is that the "__decorateClass" function doesn't contain a reference
// to the non-existent "Reflect.decorate" function. This function was never
// standardized and checking for it is wasted code (as well as a potentially
// dangerous cause of unintentional behavior changes in the future).
//
// Another difference is that the "__decorateClass" function doesn't take in an
// optional property descriptor like it does in the official TypeScript
// compiler's support code. This appears to be a dead code path in the official
// support code that is only there for legacy reasons.
//
// Here are some examples of how esbuild's decorator transform works:
//
// ============================= Class decorator ==============================
//
//   // TypeScript                      // JavaScript
//   @dec                               let C = class {
//   class C {                          };
//   }                                  C = __decorateClass([
//                                        dec
//                                      ], C);
//
// ============================ Method decorator ==============================
//
//   // TypeScript                      // JavaScript
//   class C {                          class C {
//     @dec                               foo() {}
//     foo() {}                         }
//   }                                  __decorateClass([
//                                        dec
//                                      ], C.prototype, 'foo', 1);
//
// =========================== Parameter decorator ============================
//
//   // TypeScript                      // JavaScript
//   class C {                          class C {
//     foo(@dec bar) {}                   foo(bar) {}
//   }                                  }
//                                      __decorateClass([
//                                        __decorateParam(0, dec)
//                                      ], C.prototype, 'foo', 1);
//
// ============================= Field decorator ==============================
//
//   // TypeScript                      // JavaScript
//   class C {                          class C {
//     @dec                               constructor() {
//     foo = 123                            this.foo = 123
//   }                                    }
//                                      }
//                                      __decorateClass([
//                                        dec
//                                      ], C.prototype, 'foo', 2);
