package runtime

type Fn uint8

const (
	ExportFn Fn = 1 << 1
)

var FnMap = map[string]Fn{
	"__export": ExportFn,
}

const Code = `
	let __defineProperty = Object.defineProperty
	let __hasOwnProperty = Object.hasOwnProperty

	// Holds the exports for all modules that have been evaluated
	let __modules = {}

	// Used to import a bundled module using require()
	let __require = id => {
		let module = __modules[id]
		if (!module) {
			module = __modules[id] = {exports: {}}
			__commonjs[id](module.exports, module)
		}
		return module.exports
	}

	// Converts the module from CommonJS to ES6 if necessary
	let __toModule = module => {
		if (module && module.__esModule) {
			return module
		}
		let result = {}
		for (let key in module) {
			if (__hasOwnProperty.call(module, key)) {
				result[key] = module[key]
			}
		}
		result.default = module
		return result
	}

	// Used to import a bundled module using an ES6 import statement
	let __import = id => {
		return __toModule(__require(id))
	}

	// Used to implement ES6 exports to CommonJS
	let __export = (target, all) => {
		__defineProperty(target, '__esModule', { value: true })
		for (let name in all) {
			__defineProperty(target, name, { get: all[name], enumerable: true })
		}
	}

	// Will be filled in with the CommonJS module map
	let __commonjs
`
