package runtime

type Sym uint16

const (
	// These flags are designed to be merged together using bitwise-or to figure
	// out what runtime symbols are used. Each flag includes its dependencies so
	// that a bitwise-or will automatically also mark them as used too.
	DefinePropertySym        Sym = (1 << 0)
	HasOwnPropertySym        Sym = (1 << 1)
	GetOwnPropertySymbolsSym Sym = (1 << 2)
	PropertyIsEnumerableSym  Sym = (1 << 3)
	RestSym                  Sym = (1 << 4) | HasOwnPropertySym | GetOwnPropertySymbolsSym | PropertyIsEnumerableSym
	ModulesSym               Sym = (1 << 5)
	CommonJsSym              Sym = (1 << 6)
	RequireSym               Sym = (1 << 7) | ModulesSym | CommonJsSym
	ToModuleSym              Sym = (1 << 8) | HasOwnPropertySym
	ImportSym                Sym = (1 << 9) | ToModuleSym | RequireSym
	ExportSym                Sym = (1 << 10) | DefinePropertySym
)

var SymMap = map[string]Sym{
	"__defineProperty":        DefinePropertySym,
	"__hasOwnProperty":        HasOwnPropertySym,
	"__getOwnPropertySymbols": GetOwnPropertySymbolsSym,
	"__propertyIsEnumerable":  PropertyIsEnumerableSym,
	"__rest":                  RestSym,
	"__modules":               ModulesSym,
	"__commonjs":              CommonJsSym,
	"__require":               RequireSym,
	"__toModule":              ToModuleSym,
	"__import":                ImportSym,
	"__export":                ExportSym,
}

const Code = `
	let __defineProperty = Object.defineProperty
	let __hasOwnProperty = Object.hasOwnProperty
	let __getOwnPropertySymbols = Object.getOwnPropertySymbols
	let __propertyIsEnumerable = Object.propertyIsEnumerable

	let __rest = (source, exclude) => {
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

	// Holds the exports for all modules that have been evaluated
	let __modules = {}

	// Will be filled in with the CommonJS module map
	let __commonjs

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
`
