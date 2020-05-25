package runtime

const Code = `
	let __defineProperty = Object.defineProperty
	let __hasOwnProperty = Object.prototype.hasOwnProperty
	let __getOwnPropertySymbols = Object.getOwnPropertySymbols
	let __propertyIsEnumerable = Object.propertyIsEnumerable

	export let __pow = Math.pow
	export let __assign = Object.assign

	export let __rest = (source, exclude) => {
		let target = {}
		for (let prop in source)
			if (__hasOwnProperty.call(source, prop) && exclude.indexOf(prop) < 0)
				target[prop] = source[prop]
		if (source != null && typeof __getOwnPropertySymbols === 'function')
			for (let prop of __getOwnPropertySymbols(source))
				if (exclude.indexOf(prop) < 0 && __propertyIsEnumerable.call(source, prop))
					target[prop] = source[prop]
		return target
	}

	// Wraps a CommonJS closure and returns a require() function
	export let __commonJS = callback => {
		let module
		return () => {
			if (!module) {
				module = {exports: {}}
				callback(module.exports, module)
			}
			return module.exports
		}
	}

	// Converts the module from CommonJS to ES6 if necessary
	export let __toModule = module => {
		if (module && module.__esModule)
			return module
		let result = {}
		__defineProperty(result, 'default', { value: module, enumerable: true })
		for (let key in module)
			if (__hasOwnProperty.call(module, key) && key !== 'default')
				__defineProperty(result, key, { get: () => module[key], enumerable: true })
		return result
	}

	// Used to implement ES6 exports to CommonJS
	export let __export = (target, all) => {
		__defineProperty(target, '__esModule', { value: true })
		for (let name in all)
			__defineProperty(target, name, { get: all[name], enumerable: true })
	}
`
