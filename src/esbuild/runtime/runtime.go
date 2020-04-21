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

	// This holds the exports for all modules that have been evaluated
	let __modules = {}

	let __require = (id, module) => {
		module = __modules[id]
		if (!module) {
			module = __modules[id] = {exports: {}}
			__commonjs[id](module.exports, module)
		}
		return module.exports
	}

	let __import = (module, exports) => {
		module = __require(module)
		if (module && module.__esModule) {
			return module
		}
		exports = Object(module)
		if (!('default' in exports)) {
			__defineProperty(exports, 'default', { value: module, enumerable: true })
		}
		return exports
	}

	let __export = (target, all) => {
		__defineProperty(target, '__esModule', { value: true })
		for (let name in all) {
			__defineProperty(target, name, { get: all[name], enumerable: true })
		}
	}

	// This will be filled in with the CommonJS module map
	let __commonjs
`
