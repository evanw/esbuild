package runtime

// The runtime source is always at a special index. The index is always zero
// but this constant is always used instead to improve readability and ensure
// all code that references this index can be discovered easily.
const SourceIndex = uint32(0)

const Code = `
	let __defineProperty = Object.defineProperty
	let __hasOwnProperty = Object.prototype.hasOwnProperty
	let __getOwnPropertySymbols = Object.getOwnPropertySymbols
	let __getOwnPropertyDescriptor = Object.getOwnPropertyDescriptor
	let __propertyIsEnumerable = Object.prototype.propertyIsEnumerable

	export let __pow = Math.pow
	export let __assign = Object.assign

	// For object rest patterns
	export let __restKey = key => typeof key === 'symbol' ? key : key + ''
	export let __rest = (source, exclude) => {
		let target = {}
		for (let prop in source)
			if (__hasOwnProperty.call(source, prop) && exclude.indexOf(prop) < 0)
				target[prop] = source[prop]
		if (source != null && __getOwnPropertySymbols)
			for (let prop of __getOwnPropertySymbols(source))
				if (exclude.indexOf(prop) < 0 && __propertyIsEnumerable.call(source, prop))
					target[prop] = source[prop]
		return target
	}

	// Wraps a CommonJS closure and returns a require() function
	export let __commonJS = (callback, module) => () => {
		if (!module) {
			module = {exports: {}}
			callback(module.exports, module)
		}
		return module.exports
	}

	// Used to implement ES6 exports to CommonJS
	let __markAsModule = target => {
		return __defineProperty(target, '__esModule', { value: true })
	}
	export let __export = (target, all) => {
		__markAsModule(target)
		for (let name in all)
			__defineProperty(target, name, { get: all[name], enumerable: true })
	}
	export let __exportStar = (target, module) => {
		__markAsModule(target)
		if (typeof module === 'object' || typeof module === 'function')
			for (let key in module)
				if (__hasOwnProperty.call(module, key) && !__hasOwnProperty.call(target, key) && key !== 'default')
					__defineProperty(target, key, { get: () => module[key], enumerable: true })
		return target
	}

	// Converts the module from CommonJS to ES6 if necessary
	export let __toModule = module => {
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
	export let __decorate = (decorators, target, key, kind) => {
		var result = kind > 1 ? void 0 : kind ? __getOwnPropertyDescriptor(target, key) : target
		for (var i = decorators.length - 1, decorator; i >= 0; i--)
			if (decorator = decorators[i])
				result = (kind ? decorator(target, key, result) : decorator(result)) || result
		if (kind && result)
			__defineProperty(target, key, result)
		return result
	}
	export let __param = (index, decorator) => (target, key) => decorator(target, key, index)

	// For class members
	export let __publicField = (obj, key, value) => {
		if (key in obj) return __defineProperty(obj, key, {enumerable: true, configurable: true, writable: true, value})
		else return obj[key] = value
	}
	let __accessCheck = (obj, member, msg) => {
		if (!member.has(obj)) throw TypeError('Cannot ' + msg)
	}
	export let __privateGet = (obj, member, getter) => {
		__accessCheck(obj, member, 'read from private field')
		return getter ? getter.call(obj) : member.get(obj)
	}
	export let __privateSet = (obj, member, value, setter) => {
		__accessCheck(obj, member, 'write to private field')
		setter ? setter.call(obj, value) : member.set(obj, value)
		return value
	}
	export let __privateMethod = (obj, member, method) => {
		__accessCheck(obj, member, 'access private method')
		return method
	}

	// This helps for lowering async functions
	export let __async = (__this, __arguments, generator) => {
		return new Promise((resolve, reject) => {
			let fulfilled = value => {
				try {
					step(generator.next(value))
				} catch (e) {
					reject(e)
				}
			}
			let rejected = value => {
				try {
					step(generator.throw(value))
				} catch (e) {
					reject(e)
				}
			}
			let step = result => {
				return result.done ? resolve(result.value) : Promise.resolve(result.value).then(fulfilled, rejected)
			}
			step((generator = generator.apply(__this, __arguments)).next())
		})
	}
`

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
