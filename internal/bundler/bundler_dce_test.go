package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

var dce_suite = suite{
	name: "dce",
}

func TestPackageJsonSideEffectsFalseKeepNamedImportES6(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "demo-pkg"
				console.log(foo)
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				export const foo = 123
				console.log('hello')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": false
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsFalseKeepNamedImportCommonJS(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "demo-pkg"
				console.log(foo)
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				exports.foo = 123
				console.log('hello')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": false
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsFalseKeepStarImportES6(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import * as ns from "demo-pkg"
				console.log(ns)
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				export const foo = 123
				console.log('hello')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": false
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsFalseKeepStarImportCommonJS(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import * as ns from "demo-pkg"
				console.log(ns)
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				exports.foo = 123
				console.log('hello')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": false
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsTrueKeepES6(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import "demo-pkg"
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				export const foo = 123
				console.log('hello')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": true
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsTrueKeepCommonJS(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import "demo-pkg"
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				exports.foo = 123
				console.log('hello')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": true
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsFalseKeepBareImportAndRequireES6(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import "demo-pkg"
				require('demo-pkg')
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				export const foo = 123
				console.log('hello')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": false
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: warning: Ignoring this import because "` +
			`Users/user/project/node_modules/demo-pkg/index.js" was marked as having no side effects
Users/user/project/node_modules/demo-pkg/package.json: note: "sideEffects" is false ` +
			`in the enclosing "package.json" file
`,
	})
}

func TestPackageJsonSideEffectsFalseKeepBareImportAndRequireCommonJS(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import "demo-pkg"
				require('demo-pkg')
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				exports.foo = 123
				console.log('hello')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": false
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: warning: Ignoring this import because "` +
			`Users/user/project/node_modules/demo-pkg/index.js" was marked as having no side effects
Users/user/project/node_modules/demo-pkg/package.json: note: "sideEffects" is false ` +
			`in the enclosing "package.json" file
`,
	})
}

func TestPackageJsonSideEffectsFalseRemoveBareImportES6(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import "demo-pkg"
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				export const foo = 123
				console.log('hello')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": false
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: warning: Ignoring this import because "` +
			`Users/user/project/node_modules/demo-pkg/index.js" was marked as having no side effects
Users/user/project/node_modules/demo-pkg/package.json: note: "sideEffects" is false ` +
			`in the enclosing "package.json" file
`,
	})
}

func TestPackageJsonSideEffectsFalseRemoveBareImportCommonJS(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import "demo-pkg"
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				exports.foo = 123
				console.log('hello')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": false
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: warning: Ignoring this import because "` +
			`Users/user/project/node_modules/demo-pkg/index.js" was marked as having no side effects
Users/user/project/node_modules/demo-pkg/package.json: note: "sideEffects" is false ` +
			`in the enclosing "package.json" file
`,
	})
}

func TestPackageJsonSideEffectsFalseRemoveNamedImportES6(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "demo-pkg"
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				export const foo = 123
				console.log('hello')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": false
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsFalseRemoveNamedImportCommonJS(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "demo-pkg"
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				exports.foo = 123
				console.log('hello')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": false
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsFalseRemoveStarImportES6(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import * as ns from "demo-pkg"
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				export const foo = 123
				console.log('hello')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": false
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsFalseRemoveStarImportCommonJS(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import * as ns from "demo-pkg"
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				exports.foo = 123
				console.log('hello')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": false
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsArrayRemove(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "demo-pkg"
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				export const foo = 123
				console.log('hello')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": []
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsArrayKeep(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "demo-pkg"
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				export const foo = 123
				console.log('hello')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": ["./index.js"]
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsArrayKeepMainUseModule(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "demo-pkg"
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg/index-main.js": `
				export const foo = 123
				console.log('TEST FAILED')
			`,
			"/Users/user/project/node_modules/demo-pkg/index-module.js": `
				export const foo = 123
				console.log('TEST FAILED')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "index-main.js",
					"module": "index-module.js",
					"sideEffects": ["./index-main.js"]
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			MainFields:    []string{"module"},
		},
	})
}

func TestPackageJsonSideEffectsArrayKeepMainUseMain(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "demo-pkg"
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg/index-main.js": `
				export const foo = 123
				console.log('this should be kept')
			`,
			"/Users/user/project/node_modules/demo-pkg/index-module.js": `
				export const foo = 123
				console.log('TEST FAILED')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "index-main.js",
					"module": "index-module.js",
					"sideEffects": ["./index-main.js"]
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			MainFields:    []string{"main"},
		},
	})
}

func TestPackageJsonSideEffectsArrayKeepMainImplicitModule(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "demo-pkg"
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg/index-main.js": `
				export const foo = 123
				console.log('TEST FAILED')
			`,
			"/Users/user/project/node_modules/demo-pkg/index-module.js": `
				export const foo = 123
				console.log('TEST FAILED')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "index-main.js",
					"module": "index-module.js",
					"sideEffects": ["./index-main.js"]
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsArrayKeepMainImplicitMain(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "demo-pkg"
				import "./require-demo-pkg"
				console.log('unused import')
			`,
			"/Users/user/project/src/require-demo-pkg.js": `
				// This causes "index-main.js" to be selected
				require('demo-pkg')
			`,
			"/Users/user/project/node_modules/demo-pkg/index-main.js": `
				export const foo = 123
				console.log('this should be kept')
			`,
			"/Users/user/project/node_modules/demo-pkg/index-module.js": `
				export const foo = 123
				console.log('TEST FAILED')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "index-main.js",
					"module": "index-module.js",
					"sideEffects": ["./index-main.js"]
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsArrayKeepModuleUseModule(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "demo-pkg"
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg/index-main.js": `
				export const foo = 123
				console.log('TEST FAILED')
			`,
			"/Users/user/project/node_modules/demo-pkg/index-module.js": `
				export const foo = 123
				console.log('this should be kept')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "index-main.js",
					"module": "index-module.js",
					"sideEffects": ["./index-module.js"]
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			MainFields:    []string{"module"},
		},
	})
}

func TestPackageJsonSideEffectsArrayKeepModuleUseMain(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "demo-pkg"
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg/index-main.js": `
				export const foo = 123
				console.log('TEST FAILED')
			`,
			"/Users/user/project/node_modules/demo-pkg/index-module.js": `
				export const foo = 123
				console.log('TEST FAILED')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "index-main.js",
					"module": "index-module.js",
					"sideEffects": ["./index-module.js"]
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			MainFields:    []string{"main"},
		},
	})
}

func TestPackageJsonSideEffectsArrayKeepModuleImplicitModule(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "demo-pkg"
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg/index-main.js": `
				export const foo = 123
				console.log('TEST FAILED')
			`,
			"/Users/user/project/node_modules/demo-pkg/index-module.js": `
				export const foo = 123
				console.log('this should be kept')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "index-main.js",
					"module": "index-module.js",
					"sideEffects": ["./index-module.js"]
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsArrayKeepModuleImplicitMain(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "demo-pkg"
				import "./require-demo-pkg"
				console.log('unused import')
			`,
			"/Users/user/project/src/require-demo-pkg.js": `
				// This causes "index-main.js" to be selected
				require('demo-pkg')
			`,
			"/Users/user/project/node_modules/demo-pkg/index-main.js": `
				export const foo = 123
				console.log('this should be kept')
			`,
			"/Users/user/project/node_modules/demo-pkg/index-module.js": `
				export const foo = 123
				console.log('TEST FAILED')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "index-main.js",
					"module": "index-module.js",
					"sideEffects": ["./index-module.js"]
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsArrayGlob(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import "demo-pkg/keep/this/file"
				import "demo-pkg/remove/this/file"
			`,
			"/Users/user/project/node_modules/demo-pkg/keep/this/file.js": `
				console.log('this should be kept')
			`,
			"/Users/user/project/node_modules/demo-pkg/remove/this/file.js": `
				console.log('TEST FAILED')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": [
						"./ke?p/*/file.js",
						"./remove/this/file.j",
						"./re?ve/this/file.js"
					]
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: warning: Ignoring this import because ` +
			`"Users/user/project/node_modules/demo-pkg/remove/this/file.js" was marked as having no side effects
Users/user/project/node_modules/demo-pkg/package.json: note: It was excluded from the "sideEffects" ` +
			`array in the enclosing "package.json" file
`,
	})
}

func TestPackageJsonSideEffectsNestedDirectoryRemove(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "demo-pkg/a/b/c"
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": false
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/a/b/c/index.js": `
				export const foo = 123
				console.log('hello')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsKeepExportDefaultExpr(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import foo from "demo-pkg"
				console.log(foo)
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				export default exprWithSideEffects()
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": false
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestJSONLoaderRemoveUnused(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import unused from "./example.json"
				console.log('unused import')
			`,
			"/example.json": `{"data": true}`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTextLoaderRemoveUnused(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import unused from "./example.txt"
				console.log('unused import')
			`,
			"/example.txt": `some data`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestBase64LoaderRemoveUnused(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import unused from "./example.data"
				console.log('unused import')
			`,
			"/example.data": `some data`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":   config.LoaderJS,
				".data": config.LoaderBase64,
			},
		},
	})
}

func TestDataURLLoaderRemoveUnused(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import unused from "./example.data"
				console.log('unused import')
			`,
			"/example.data": `some data`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":   config.LoaderJS,
				".data": config.LoaderDataURL,
			},
		},
	})
}

func TestFileLoaderRemoveUnused(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import unused from "./example.data"
				console.log('unused import')
			`,
			"/example.data": `some data`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":   config.LoaderJS,
				".data": config.LoaderFile,
			},
		},
	})
}

func TestRemoveUnusedImportMeta(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function foo() {
					console.log(import.meta.url, import.meta.path)
				}
				console.log('foo is unused')
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestRemoveUnusedPureCommentCalls(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function bar() {}
				let bare = foo(bar);

				let at_yes = /* @__PURE__ */ foo(bar);
				let at_no = /* @__PURE__ */ foo(bar());
				let new_at_yes = /* @__PURE__ */ new foo(bar);
				let new_at_no = /* @__PURE__ */ new foo(bar());

				let num_yes = /* #__PURE__ */ foo(bar);
				let num_no = /* #__PURE__ */ foo(bar());
				let new_num_yes = /* #__PURE__ */ new foo(bar);
				let new_num_no = /* #__PURE__ */ new foo(bar());

				let dot_yes = /* @__PURE__ */ foo(sideEffect()).dot(bar);
				let dot_no = /* @__PURE__ */ foo(sideEffect()).dot(bar());
				let new_dot_yes = /* @__PURE__ */ new foo(sideEffect()).dot(bar);
				let new_dot_no = /* @__PURE__ */ new foo(sideEffect()).dot(bar());

				let nested_yes = [1, /* @__PURE__ */ foo(bar), 2];
				let nested_no = [1, /* @__PURE__ */ foo(bar()), 2];
				let new_nested_yes = [1, /* @__PURE__ */ new foo(bar), 2];
				let new_nested_no = [1, /* @__PURE__ */ new foo(bar()), 2];

				let single_at_yes = // @__PURE__
					foo(bar);
				let single_at_no = // @__PURE__
					foo(bar());
				let new_single_at_yes = // @__PURE__
					new foo(bar);
				let new_single_at_no = // @__PURE__
					new foo(bar());

				let single_num_yes = // #__PURE__
					foo(bar);
				let single_num_no = // #__PURE__
					foo(bar());
				let new_single_num_yes = // #__PURE__
					new foo(bar);
				let new_single_num_no = // #__PURE__
					new foo(bar());

				let bad_no = /* __PURE__ */ foo(bar);
				let new_bad_no = /* __PURE__ */ new foo(bar);

				let parens_no = (/* @__PURE__ */ foo)(bar);
				let new_parens_no = new (/* @__PURE__ */ foo)(bar);

				let exp_no = /* @__PURE__ */ foo() ** foo();
				let new_exp_no = /* @__PURE__ */ new foo() ** foo();
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTreeShakingReactElements(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.jsx": `
				function Foo() {}

				let a = <div/>
				let b = <Foo>{a}</Foo>
				let c = <>{b}</>

				let d = <div/>
				let e = <Foo>{d}</Foo>
				let f = <>{e}</>
				console.log(f)
			`,
		},
		entryPaths: []string{"/entry.jsx"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestDisableTreeShaking(t *testing.T) {
	defines := config.ProcessDefines(map[string]config.DefineData{
		"pure":    {CallCanBeUnwrappedIfUnused: true},
		"some.fn": {CallCanBeUnwrappedIfUnused: true},
	})
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.jsx": `
				import './remove-me'
				function RemoveMe1() {}
				let removeMe2 = 0
				class RemoveMe3 {}

				import './keep-me'
				function KeepMe1() {}
				let keepMe2 = <KeepMe1/>
				function keepMe3() { console.log('side effects') }
				let keepMe4 = /* @__PURE__ */ keepMe3()
				let keepMe5 = pure()
				let keepMe6 = some.fn()
			`,
			"/remove-me.js": `
				export default 'unused'
			`,
			"/keep-me/index.js": `
				console.log('side effects')
			`,
			"/keep-me/package.json": `
				{ "sideEffects": false }
			`,
		},
		entryPaths: []string{"/entry.jsx"},
		options: config.Options{
			Mode:                 config.ModeBundle,
			AbsOutputFile:        "/out.js",
			IgnoreDCEAnnotations: true,
			Defines:              &defines,
		},
	})
}

func TestDeadCodeFollowingJump(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function testReturn() {
					if (true) return y + z()
					if (FAIL) return FAIL
					if (x) { var y }
					function z() { KEEP_ME() }
					return FAIL
				}

				function testThrow() {
					if (true) throw y + z()
					if (FAIL) return FAIL
					if (x) { var y }
					function z() { KEEP_ME() }
					return FAIL
				}

				function testBreak() {
					while (true) {
						if (true) {
							y + z()
							break
						}
						if (FAIL) return FAIL
						if (x) { var y }
						function z() { KEEP_ME() }
						return FAIL
					}
				}

				function testContinue() {
					while (true) {
						if (true) {
							y + z()
							continue
						}
						if (FAIL) return FAIL
						if (x) { var y }
						function z() { KEEP_ME() }
						return FAIL
					}
				}

				function testStmts() {
					return [a, b, c, d, e, f, g, h, i]

					while (x) { var a }
					while (FAIL) { let FAIL }

					do { var b } while (x)
					do { let FAIL } while (FAIL)

					for (var c; ;) ;
					for (let FAIL; ;) ;

					for (var d in x) ;
					for (let FAIL in FAIL) ;

					for (var e of x) ;
					for (let FAIL of FAIL) ;

					if (x) { var f }
					if (FAIL) { let FAIL }

					if (x) ; else { var g }
					if (FAIL) ; else { let FAIL }

					{ var h }
					{ let FAIL }

					x: { var i }
					x: { let FAIL }
				}

				testReturn()
				testThrow()
				testBreak()
				testContinue()
				testStmts()
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			MangleSyntax:  true,
		},
	})
}

func TestRemoveTrailingReturn(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function foo() {
					if (a) b()
					return
				}
				function bar() {
					if (a) b()
					return KEEP_ME
				}
				export default [
					foo,
					bar,
					function () {
						if (a) b()
						return
					},
					function () {
						if (a) b()
						return KEEP_ME
					},
					() => {
						if (a) b()
						return
					},
					() => {
						if (a) b()
						return KEEP_ME
					},
				]
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			MangleSyntax:  true,
			OutputFormat:  config.FormatESModule,
		},
	})
}

func TestImportReExportOfNamespaceImport(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/entry.js": `
				import * as ns from 'pkg'
				console.log(ns.foo)
			`,
			"/Users/user/project/node_modules/pkg/index.js": `
				export { default as foo } from './foo'
				export { default as bar } from './bar'
			`,
			"/Users/user/project/node_modules/pkg/package.json": `
				{ "sideEffects": false }
			`,
			"/Users/user/project/node_modules/pkg/foo.js": `
				module.exports = 123
			`,
			"/Users/user/project/node_modules/pkg/bar.js": `
				module.exports = 'abc'
			`,
		},
		entryPaths: []string{"/Users/user/project/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTreeShakingImportIdentifier(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as a from './a'
				new a.Keep()
			`,
			"/a.js": `
				import * as b from './b'
				export class Keep extends b.Base {}
				export class REMOVE extends b.Base {}
			`,
			"/b.js": `
				export class Base {}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}
