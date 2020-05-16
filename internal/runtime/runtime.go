package runtime

const Code = `
	var __defineProperty = Object.defineProperty
	var __hasOwnProperty = Object.hasOwnProperty
	var __getOwnPropertySymbols = Object.getOwnPropertySymbols
	var __propertyIsEnumerable = Object.propertyIsEnumerable

	export var __pow = Math.pow
	export var __assign = Object.assign

	export var __rest = (source, exclude) => {
		var target = {}
		for (var prop in source)
			if (__hasOwnProperty.call(source, prop) && exclude.indexOf(prop) < 0)
				target[prop] = source[prop]
		if (source != null && typeof __getOwnPropertySymbols === 'function')
			for (var prop of __getOwnPropertySymbols(source))
				if (exclude.indexOf(prop) < 0 && __propertyIsEnumerable.call(source, prop))
					target[prop] = source[prop]
		return target
	}

	// Wraps a CommonJS closure and returns a require() function
	export var __commonJS = callback => {
		var module
		return () => {
			if (!module) {
				module = {exports: {}}
				callback(module.exports, module)
			}
			return module.exports
		}
	}

	// Converts the module from CommonJS to ES6 if necessary
	export var __toModule = module => {
		if (module && module.__esModule)
			return module
		var result = {}
		for (var key in module)
			if (__hasOwnProperty.call(module, key))
				result[key] = module[key]
		result.default = module
		return result
	}

	// Used to implement ES6 exports to CommonJS
	export var __export = (target, all) => {
		__defineProperty(target, '__esModule', { value: true })
		for (var name in all)
			__defineProperty(target, name, { get: all[name], enumerable: true })
	}
`
