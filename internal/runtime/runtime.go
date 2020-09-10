// This is esbuild's runtime code. It contains helper functions that are
// automatically injected into output files to implement certain features. For
// example, the "**" operator is replaced with a call to "__pow" when targeting
// ES2015. Tree shaking automatically removes unused code from the runtime.

package runtime

import (
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/logger"
)

// The runtime source is always at a special index. The index is always zero
// but this constant is always used instead to improve readability and ensure
// all code that references this index can be discovered easily.
const SourceIndex = uint32(0)

func CanUseES6(unsupportedFeatures compat.Feature) bool {
	return !unsupportedFeatures.Has(compat.Let)
}

func code(isES6 bool) string {
	// Note: The "__rest" function has a for-of loop which requires ES6, but
	// transforming destructuring to ES5 isn't even supported so it's ok.
	text := `
		var __defineProperty = Object.defineProperty
		var __hasOwnProperty = Object.prototype.hasOwnProperty
		var __getOwnPropertySymbols = Object.getOwnPropertySymbols
		var __getOwnPropertyDescriptor = Object.getOwnPropertyDescriptor
		var __propertyIsEnumerable = Object.prototype.propertyIsEnumerable

		export var __pow = Math.pow
		export var __assign = Object.assign

		// For object rest patterns
		export var __restKey = key => typeof key === 'symbol' ? key : key + ''
		export var __rest = (source, exclude) => {
			var target = {}
			for (var prop in source)
				if (__hasOwnProperty.call(source, prop) && exclude.indexOf(prop) < 0)
					target[prop] = source[prop]
			if (source != null && __getOwnPropertySymbols)
				for (var prop of __getOwnPropertySymbols(source))
					if (exclude.indexOf(prop) < 0 && __propertyIsEnumerable.call(source, prop))
						target[prop] = source[prop]
			return target
		}

		// Wraps a CommonJS closure and returns a require() function
		export var __commonJS = (callback, module) => () => {
			if (!module) {
				module = {exports: {}}
				callback(module.exports, module)
			}
			return module.exports
		}

		// Used to implement ES6 exports to CommonJS
		var __markAsModule = target => {
			return __defineProperty(target, '__esModule', { value: true })
		}
		export var __export = (target, all) => {
			__markAsModule(target)
			for (var name in all)
				__defineProperty(target, name, { get: all[name], enumerable: true })
		}
		export var __exportStar = (target, module) => {
			__markAsModule(target)
			if (typeof module === 'object' || typeof module === 'function')
	`

	// Avoid "let" when not using ES6
	if isES6 {
		text += `
			for (let key in module)
				if (!__hasOwnProperty.call(target, key) && key !== 'default')
					__defineProperty(target, key, { get: () => module[key], enumerable: true })
		`
	} else {
		text += `
			for (var key in module)
				if (!__hasOwnProperty.call(target, key) && key !== 'default')
					(k => {
						__defineProperty(target, k, { get: () => module[k], enumerable: true })
					})(key)
		`
	}

	text += `
			return target
		}

		// Converts the module from CommonJS to ES6 if necessary
		export var __toModule = module => {
			if (module && module.__esModule)
				return module
			return __exportStar(
				__defineProperty({}, 'default', { value: module, enumerable: true }),
				module)
		}

		// For TypeScript decorators
		// - kind === undefined: class
		// - kind === 1: method, parameter
		// - kind === 2: field
		export var __decorate = (decorators, target, key, kind) => {
			var result = kind > 1 ? void 0 : kind ? __getOwnPropertyDescriptor(target, key) : target
			for (var i = decorators.length - 1, decorator; i >= 0; i--)
				if (decorator = decorators[i])
					result = (kind ? decorator(target, key, result) : decorator(result)) || result
			if (kind && result)
				__defineProperty(target, key, result)
			return result
		}
		export var __param = (index, decorator) => (target, key) => decorator(target, key, index)

		// For class members
		export var __publicField = (obj, key, value) => {
			if (key in obj) return __defineProperty(obj, key, {enumerable: true, configurable: true, writable: true, value})
			else return obj[key] = value
		}
		var __accessCheck = (obj, member, msg) => {
			if (!member.has(obj)) throw TypeError('Cannot ' + msg)
		}
		export var __privateGet = (obj, member, getter) => {
			__accessCheck(obj, member, 'read from private field')
			return getter ? getter.call(obj) : member.get(obj)
		}
		export var __privateSet = (obj, member, value, setter) => {
			__accessCheck(obj, member, 'write to private field')
			setter ? setter.call(obj, value) : member.set(obj, value)
			return value
		}
		export var __privateMethod = (obj, member, method) => {
			__accessCheck(obj, member, 'access private method')
			return method
		}

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
				var step = result => {
					return result.done ? resolve(result.value) : Promise.resolve(result.value).then(fulfilled, rejected)
				}
				step((generator = generator.apply(__this, __arguments)).next())
			})
		}

		// This is for the "binary" loader (custom code is ~2x faster than "atob")
		export var __toBinary = __platform === 'node'
			? base64 => new Uint8Array(Buffer.from(base64, 'base64'))
			: /* @__PURE__ */ (() => {
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
	`

	return text
}

var ES6Source = logger.Source{
	Index:          SourceIndex,
	KeyPath:        logger.Path{Text: "<runtime>"},
	PrettyPath:     "<runtime>",
	IdentifierName: "runtime",
	Contents:       code(true /* isES6 */),
}

var ES5Source = logger.Source{
	Index:          SourceIndex,
	KeyPath:        logger.Path{Text: "<runtime>"},
	PrettyPath:     "<runtime>",
	IdentifierName: "runtime",
	Contents:       code(false /* isES6 */),
}

// The TypeScript decorator transform behaves similar to the official
// TypeScript compiler.
//
// One difference is that the "__decorate" function doesn't contain a reference
// to the non-existent "Reflect.decorate" function. This function was never
// standardized and checking for it is wasted code (as well as a potentially
// dangerous cause of unintentional behavior changes in the future).
//
// Another difference is that the "__decorate" function doesn't take in an
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
//   }                                  C = __decorate([
//                                        dec
//                                      ], C);
//
// ============================ Method decorator ==============================
//
//   // TypeScript                      // JavaScript
//   class C {                          class C {
//     @dec                               foo() {}
//     foo() {}                         }
//   }                                  __decorate([
//                                        dec
//                                      ], C.prototype, 'foo', 1);
//
// =========================== Parameter decorator ============================
//
//   // TypeScript                      // JavaScript
//   class C {                          class C {
//     foo(@dec bar) {}                   foo(bar) {}
//   }                                  }
//                                      __decorate([
//                                        __param(0, dec)
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
//                                      __decorate([
//                                        dec
//                                      ], C.prototype, 'foo', 2);
