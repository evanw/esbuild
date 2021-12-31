package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/compat"
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
		expectedScanLog: `Users/user/project/src/entry.js: WARNING: Ignoring this import because "` +
			`Users/user/project/node_modules/demo-pkg/index.js" was marked as having no side effects
Users/user/project/node_modules/demo-pkg/package.json: NOTE: "sideEffects" is false ` +
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
		expectedScanLog: `Users/user/project/src/entry.js: WARNING: Ignoring this import because "` +
			`Users/user/project/node_modules/demo-pkg/index.js" was marked as having no side effects
Users/user/project/node_modules/demo-pkg/package.json: NOTE: "sideEffects" is false ` +
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
		expectedScanLog: `Users/user/project/src/entry.js: WARNING: Ignoring this import because "` +
			`Users/user/project/node_modules/demo-pkg/index.js" was marked as having no side effects
Users/user/project/node_modules/demo-pkg/package.json: NOTE: "sideEffects" is false ` +
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
		expectedScanLog: `Users/user/project/src/entry.js: WARNING: Ignoring this import because "` +
			`Users/user/project/node_modules/demo-pkg/index.js" was marked as having no side effects
Users/user/project/node_modules/demo-pkg/package.json: NOTE: "sideEffects" is false ` +
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
		expectedScanLog: `Users/user/project/src/entry.js: WARNING: Ignoring this import because ` +
			`"Users/user/project/node_modules/demo-pkg/remove/this/file.js" was marked as having no side effects
Users/user/project/node_modules/demo-pkg/package.json: NOTE: It was excluded from the "sideEffects" ` +
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

func TestPackageJsonSideEffectsFalseNoWarningInNodeModulesIssue999(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import "demo-pkg"
				console.log('used import')
				`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				import "demo-pkg2"
				console.log('unused import')
			`,
			"/Users/user/project/node_modules/demo-pkg2/index.js": `
				export const foo = 123
				console.log('hello')
			`,
			"/Users/user/project/node_modules/demo-pkg2/package.json": `
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

func TestPackageJsonSideEffectsFalseIntermediateFilesUnused(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "demo-pkg"
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				export {foo} from "./foo.js"
				throw 'REMOVE THIS'
			`,
			"/Users/user/project/node_modules/demo-pkg/foo.js": `
				export const foo = 123
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{ "sideEffects": false }
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsFalseIntermediateFilesUsed(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "demo-pkg"
				console.log(foo)
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				export {foo} from "./foo.js"
				throw 'keep this'
			`,
			"/Users/user/project/node_modules/demo-pkg/foo.js": `
				export const foo = 123
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{ "sideEffects": false }
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsFalseIntermediateFilesChainAll(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "a"
				console.log(foo)
			`,
			"/Users/user/project/node_modules/a/index.js": `
				export {foo} from "b"
			`,
			"/Users/user/project/node_modules/a/package.json": `
				{ "sideEffects": false }
			`,
			"/Users/user/project/node_modules/b/index.js": `
				export {foo} from "c"
				throw 'keep this'
			`,
			"/Users/user/project/node_modules/b/package.json": `
				{ "sideEffects": false }
			`,
			"/Users/user/project/node_modules/c/index.js": `
				export {foo} from "d"
			`,
			"/Users/user/project/node_modules/c/package.json": `
				{ "sideEffects": false }
			`,
			"/Users/user/project/node_modules/d/index.js": `
				export const foo = 123
			`,
			"/Users/user/project/node_modules/d/package.json": `
				{ "sideEffects": false }
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsFalseIntermediateFilesChainOne(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "a"
				console.log(foo)
			`,
			"/Users/user/project/node_modules/a/index.js": `
				export {foo} from "b"
			`,
			"/Users/user/project/node_modules/b/index.js": `
				export {foo} from "c"
				throw 'keep this'
			`,
			"/Users/user/project/node_modules/b/package.json": `
				{ "sideEffects": false }
			`,
			"/Users/user/project/node_modules/c/index.js": `
				export {foo} from "d"
			`,
			"/Users/user/project/node_modules/d/index.js": `
				export const foo = 123
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsFalseIntermediateFilesDiamond(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {foo} from "a"
				console.log(foo)
			`,
			"/Users/user/project/node_modules/a/index.js": `
				export * from "b1"
				export * from "b2"
			`,
			"/Users/user/project/node_modules/b1/index.js": `
				export {foo} from "c"
				throw 'keep this 1'
			`,
			"/Users/user/project/node_modules/b1/package.json": `
				{ "sideEffects": false }
			`,
			"/Users/user/project/node_modules/b2/index.js": `
				export {foo} from "c"
				throw 'keep this 2'
			`,
			"/Users/user/project/node_modules/b2/package.json": `
				{ "sideEffects": false }
			`,
			"/Users/user/project/node_modules/c/index.js": `
				export {foo} from "d"
			`,
			"/Users/user/project/node_modules/d/index.js": `
				export const foo = 123
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsFalseOneFork(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import("a").then(x => assert(x.foo === "foo"))
			`,
			"/Users/user/project/node_modules/a/index.js": `
				export {foo} from "b"
			`,
			"/Users/user/project/node_modules/b/index.js": `
				export {foo, bar} from "c"
				export {baz} from "d"
			`,
			"/Users/user/project/node_modules/b/package.json": `
				{ "sideEffects": false }
			`,
			"/Users/user/project/node_modules/c/index.js": `
				export let foo = "foo"
				export let bar = "bar"
			`,
			"/Users/user/project/node_modules/d/index.js": `
				export let baz = "baz"
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestPackageJsonSideEffectsFalseAllFork(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import("a").then(x => assert(x.foo === "foo"))
			`,
			"/Users/user/project/node_modules/a/index.js": `
				export {foo} from "b"
			`,
			"/Users/user/project/node_modules/b/index.js": `
				export {foo, bar} from "c"
				export {baz} from "d"
			`,
			"/Users/user/project/node_modules/b/package.json": `
				{ "sideEffects": false }
			`,
			"/Users/user/project/node_modules/c/index.js": `
				export let foo = "foo"
				export let bar = "bar"
			`,
			"/Users/user/project/node_modules/c/package.json": `
				{ "sideEffects": false }
			`,
			"/Users/user/project/node_modules/d/index.js": `
				export let baz = "baz"
			`,
			"/Users/user/project/node_modules/d/package.json": `
				{ "sideEffects": false }
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

				let nospace_at_yes = /*@__PURE__*/ foo(bar);
				let nospace_at_no = /*@__PURE__*/ foo(bar());
				let nospace_new_at_yes = /*@__PURE__*/ new foo(bar);
				let nospace_new_at_no = /*@__PURE__*/ new foo(bar());

				let num_yes = /* #__PURE__ */ foo(bar);
				let num_no = /* #__PURE__ */ foo(bar());
				let new_num_yes = /* #__PURE__ */ new foo(bar);
				let new_num_no = /* #__PURE__ */ new foo(bar());

				let nospace_num_yes = /*#__PURE__*/ foo(bar);
				let nospace_num_no = /*#__PURE__*/ foo(bar());
				let nospace_new_num_yes = /*#__PURE__*/ new foo(bar);
				let nospace_new_num_no = /*#__PURE__*/ new foo(bar());

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

func TestTreeShakingUnaryOperators(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				// These operators may have side effects
				let keep;
				+keep;
				-keep;
				~keep;
				delete keep;
				++keep;
				--keep;
				keep++;
				keep--;

				// These operators never have side effects
				let REMOVE;
				!REMOVE;
				void REMOVE;
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTreeShakingBinaryOperators(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				// These operators may have side effects
				let keep, keep2;
				keep + keep2;
				keep - keep2;
				keep * keep2;
				keep / keep2;
				keep % keep2;
				keep ** keep2;
				keep < keep2;
				keep <= keep2;
				keep > keep2;
				keep >= keep2;
				keep in keep2;
				keep instanceof keep2;
				keep << keep2;
				keep >> keep2;
				keep >>> keep2;
				keep == keep2;
				keep != keep2;
				keep | keep2;
				keep & keep2;
				keep ^ keep2;
				keep = keep2;
				keep += keep2;
				keep -= keep2;
				keep *= keep2;
				keep /= keep2;
				keep %= keep2;
				keep **= keep2;
				keep <<= keep2;
				keep >>= keep2;
				keep >>>= keep2;
				keep |= keep2;
				keep &= keep2;
				keep ^= keep2;
				keep ??= keep2;
				keep ||= keep2;
				keep &&= keep2;

				// These operators never have side effects
				let REMOVE, REMOVE2;
				REMOVE === REMOVE2;
				REMOVE !== REMOVE2;
				REMOVE, REMOVE2;
				REMOVE ?? REMOVE2;
				REMOVE || REMOVE2;
				REMOVE && REMOVE2;
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTreeShakingNoBundleESM(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function keep() {}
				function unused() {}
				keep()
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeConvertFormat,
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTreeShakingNoBundleCJS(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function keep() {}
				function unused() {}
				keep()
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeConvertFormat,
			OutputFormat:  config.FormatCommonJS,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTreeShakingNoBundleIIFE(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function keep() {}
				function REMOVE() {}
				keep()
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeConvertFormat,
			OutputFormat:  config.FormatIIFE,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTreeShakingInESMWrapper(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {keep1} from './lib'
				console.log(keep1(), require('./cjs'))
			`,
			"/cjs.js": `
				import {keep2} from './lib'
				export default keep2()
			`,
			"/lib.js": `
				export let keep1 = () => 'keep1'
				export let keep2 = () => 'keep2'
				export let REMOVE = () => 'REMOVE'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestDCETypeOf(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				// These should be removed because they have no side effects
				typeof x_REMOVE
				typeof v_REMOVE
				typeof f_REMOVE
				typeof g_REMOVE
				typeof a_REMOVE
				var v_REMOVE
				function f_REMOVE() {}
				function* g_REMOVE() {}
				async function a_REMOVE() {}

				// These technically have side effects due to TDZ, but this is not currently handled
				typeof c_remove
				typeof l_remove
				typeof s_remove
				const c_remove = 0
				let l_remove
				class s_remove {}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestDCETypeOfEqualsString(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				var hasBar = typeof bar !== 'undefined'
				if (false) console.log(hasBar)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatIIFE,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestDCETypeOfEqualsStringMangle(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				// Everything here should be removed as dead code due to tree shaking
				var hasBar = typeof bar !== 'undefined'
				if (false) console.log(hasBar)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatIIFE,
			MangleSyntax:  true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestDCETypeOfEqualsStringGuardCondition(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				// Everything here should be removed as dead code due to tree shaking
				var REMOVE_1 = typeof x !== 'undefined' ? x : null
				var REMOVE_1 = typeof x != 'undefined' ? x : null
				var REMOVE_1 = typeof x === 'undefined' ? null : x
				var REMOVE_1 = typeof x == 'undefined' ? null : x
				var REMOVE_1 = typeof x !== 'undefined' && x
				var REMOVE_1 = typeof x != 'undefined' && x
				var REMOVE_1 = typeof x === 'undefined' || x
				var REMOVE_1 = typeof x == 'undefined' || x
				var REMOVE_1 = 'undefined' !== typeof x ? x : null
				var REMOVE_1 = 'undefined' != typeof x ? x : null
				var REMOVE_1 = 'undefined' === typeof x ? null : x
				var REMOVE_1 = 'undefined' == typeof x ? null : x
				var REMOVE_1 = 'undefined' !== typeof x && x
				var REMOVE_1 = 'undefined' != typeof x && x
				var REMOVE_1 = 'undefined' === typeof x || x
				var REMOVE_1 = 'undefined' == typeof x || x

				// Everything here should be removed as dead code due to tree shaking
				var REMOVE_2 = typeof x === 'object' ? x : null
				var REMOVE_2 = typeof x == 'object' ? x : null
				var REMOVE_2 = typeof x !== 'object' ? null : x
				var REMOVE_2 = typeof x != 'object' ? null : x
				var REMOVE_2 = typeof x === 'object' && x
				var REMOVE_2 = typeof x == 'object' && x
				var REMOVE_2 = typeof x !== 'object' || x
				var REMOVE_2 = typeof x != 'object' || x
				var REMOVE_2 = 'object' === typeof x ? x : null
				var REMOVE_2 = 'object' == typeof x ? x : null
				var REMOVE_2 = 'object' !== typeof x ? null : x
				var REMOVE_2 = 'object' != typeof x ? null : x
				var REMOVE_2 = 'object' === typeof x && x
				var REMOVE_2 = 'object' == typeof x && x
				var REMOVE_2 = 'object' !== typeof x || x
				var REMOVE_2 = 'object' != typeof x || x

				// Everything here should be kept as live code because it has side effects
				var keep_1 = typeof x !== 'object' ? x : null
				var keep_1 = typeof x != 'object' ? x : null
				var keep_1 = typeof x === 'object' ? null : x
				var keep_1 = typeof x == 'object' ? null : x
				var keep_1 = typeof x !== 'object' && x
				var keep_1 = typeof x != 'object' && x
				var keep_1 = typeof x === 'object' || x
				var keep_1 = typeof x == 'object' || x
				var keep_1 = 'object' !== typeof x ? x : null
				var keep_1 = 'object' != typeof x ? x : null
				var keep_1 = 'object' === typeof x ? null : x
				var keep_1 = 'object' == typeof x ? null : x
				var keep_1 = 'object' !== typeof x && x
				var keep_1 = 'object' != typeof x && x
				var keep_1 = 'object' === typeof x || x
				var keep_1 = 'object' == typeof x || x

				// Everything here should be kept as live code because it has side effects
				var keep_2 = typeof x !== 'undefined' ? y : null
				var keep_2 = typeof x != 'undefined' ? y : null
				var keep_2 = typeof x === 'undefined' ? null : y
				var keep_2 = typeof x == 'undefined' ? null : y
				var keep_2 = typeof x !== 'undefined' && y
				var keep_2 = typeof x != 'undefined' && y
				var keep_2 = typeof x === 'undefined' || y
				var keep_2 = typeof x == 'undefined' || y
				var keep_2 = 'undefined' !== typeof x ? y : null
				var keep_2 = 'undefined' != typeof x ? y : null
				var keep_2 = 'undefined' === typeof x ? null : y
				var keep_2 = 'undefined' == typeof x ? null : y
				var keep_2 = 'undefined' !== typeof x && y
				var keep_2 = 'undefined' != typeof x && y
				var keep_2 = 'undefined' === typeof x || y
				var keep_2 = 'undefined' == typeof x || y

				// Everything here should be kept as live code because it has side effects
				var keep_3 = typeof x !== 'undefined' ? null : x
				var keep_3 = typeof x != 'undefined' ? null : x
				var keep_3 = typeof x === 'undefined' ? x : null
				var keep_3 = typeof x == 'undefined' ? x : null
				var keep_3 = typeof x !== 'undefined' || x
				var keep_3 = typeof x != 'undefined' || x
				var keep_3 = typeof x === 'undefined' && x
				var keep_3 = typeof x == 'undefined' && x
				var keep_3 = 'undefined' !== typeof x ? null : x
				var keep_3 = 'undefined' != typeof x ? null : x
				var keep_3 = 'undefined' === typeof x ? x : null
				var keep_3 = 'undefined' == typeof x ? x : null
				var keep_3 = 'undefined' !== typeof x || x
				var keep_3 = 'undefined' != typeof x || x
				var keep_3 = 'undefined' === typeof x && x
				var keep_3 = 'undefined' == typeof x && x
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatIIFE,
			AbsOutputFile: "/out.js",
		},
	})
}

// These unused imports should be removed since they aren't used, and removing
// them makes the code shorter.
func TestRemoveUnusedImports(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import a from 'a'
				import * as b from 'b'
				import {c} from 'c'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			MangleSyntax:  true,
			AbsOutputFile: "/out.js",
		},
	})
}

// These unused imports should be kept since the direct eval could potentially
// reference them, even though they appear to be unused.
func TestRemoveUnusedImportsEval(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import a from 'a'
				import * as b from 'b'
				import {c} from 'c'
				eval('foo(a, b, c)')
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			MangleSyntax:  true,
			AbsOutputFile: "/out.js",
		},
	})
}

// These unused imports should be removed even though there is a direct eval
// because they may be types, not values, so keeping them will likely cause
// module instantiation failures. It's still true that direct eval could
// access them of course, but that's very unlikely while module instantiation
// failure is very likely so we bias towards the likely case here instead.
func TestRemoveUnusedImportsEvalTS(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import a from 'a'
				import * as b from 'b'
				import {c} from 'c'
				eval('foo(a, b, c)')
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			MangleSyntax:  true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestDCEClassStaticBlocks(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class A_REMOVE {
					static {}
				}
				class B_REMOVE {
					static { 123 }
				}
				class C_REMOVE {
					static { /* @__PURE__*/ foo() }
				}
				class D_REMOVE {
					static { try {} catch {} }
				}
				class E_REMOVE {
					static { try { /* @__PURE__*/ foo() } catch {} }
				}
				class F_REMOVE {
					static { try { 123 } catch { 123 } finally { 123 } }
				}

				class A_keep {
					static { foo }
				}
				class B_keep {
					static { this.foo }
				}
				class C_keep {
					static { try { foo } catch {} }
				}
				class D_keep {
					static { try {} finally { foo } }
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestDCEVarExports(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				var foo = { bar: 123 }
				module.exports = foo
			`,
			"/b.js": `
				var exports = { bar: 123 }
				module.exports = exports
			`,
			"/c.js": `
				var module = { bar: 123 }
				exports.foo = module
			`,
		},
		entryPaths: []string{"/a.js", "/b.js", "/c.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestDCETemplateLiteral(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": "" +
				"var remove;\n" +
				"var alsoKeep;\n" +
				"let a = `${keep}`\n" +
				"let b = `${123}`\n" +
				"let c = `${keep ? 1 : 2n}`\n" +
				"let d = `${remove ? 1 : 2n}`\n" +
				"let e = `${alsoKeep}`\n",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

// Calls to the runtime "__publicField" function are not considered side effects
func TestTreeShakingLoweredClassStaticField(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class REMOVE_ME {
					static x = 'x'
					static y = 'y'
					static z = 'z'
				}
				function REMOVE_ME_TOO() {
					new REMOVE_ME()
				}
				class KeepMe1 {
					static x = 'x'
					static y = sideEffects()
					static z = 'z'
				}
				class KeepMe2 {
					static x = 'x'
					static y = 'y'
					static z = 'z'
				}
				new KeepMe2()
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:                  config.ModeBundle,
			AbsOutputDir:          "/out",
			UnsupportedJSFeatures: compat.ClassField,
		},
	})
}

func TestTreeShakingLoweredClassStaticFieldMinified(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class REMOVE_ME {
					static x = 'x'
					static y = 'y'
					static z = 'z'
				}
				function REMOVE_ME_TOO() {
					new REMOVE_ME()
				}
				class KeepMe1 {
					static x = 'x'
					static y = sideEffects()
					static z = 'z'
				}
				class KeepMe2 {
					static x = 'x'
					static y = 'y'
					static z = 'z'
				}
				new KeepMe2()
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:                  config.ModeBundle,
			AbsOutputDir:          "/out",
			UnsupportedJSFeatures: compat.ClassField,
			MangleSyntax:          true,
		},
	})
}

// Assignments are considered side effects
func TestTreeShakingLoweredClassStaticFieldAssignment(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class KeepMe1 {
					static x = 'x'
					static y = 'y'
					static z = 'z'
				}
				class KeepMe2 {
					static x = 'x'
					static y = sideEffects()
					static z = 'z'
				}
				class KeepMe3 {
					static x = 'x'
					static y = 'y'
					static z = 'z'
				}
				new KeepMe3()
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:                    config.ModeBundle,
			AbsOutputDir:            "/out",
			UnsupportedJSFeatures:   compat.ClassField,
			UseDefineForClassFields: config.False,
		},
	})
}

func TestInlineIdentityFunctionCalls(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/identity.js": `
				function DROP(x) { return x }
				console.log(DROP(1))
				DROP(foo())
				DROP(1)
			`,

			"/identity-last.js": `
				function DROP(x) { return [x] }
				function DROP(x) { return x }
				console.log(DROP(1))
				DROP(foo())
				DROP(1)
			`,

			"/identity-cross-module.js": `
				import { DROP } from './identity-cross-module-def'
				console.log(DROP(1))
				DROP(foo())
				DROP(1)
			`,

			"/identity-cross-module-def.js": `
				export function DROP(x) { return x }
			`,

			"/identity-no-args.js": `
				function keep(x) { return x }
				console.log(keep())
				keep()
			`,

			"/identity-two-args.js": `
				function keep(x) { return x }
				console.log(keep(1, 2))
				keep(1, 2)
			`,

			"/identity-first.js": `
				function keep(x) { return x }
				function keep(x) { return [x] }
				console.log(keep(1))
				keep(foo())
				keep(1)
			`,

			"/identity-generator.js": `
				function* keep(x) { return x }
				console.log(keep(1))
				keep(foo())
				keep(1)
			`,

			"/identity-async.js": `
				async function keep(x) { return x }
				console.log(keep(1))
				keep(foo())
				keep(1)
			`,

			"/reassign.js": `
				function keep(x) { return x }
				keep = reassigned
				console.log(keep(1))
				keep(foo())
				keep(1)
			`,

			"/reassign-inc.js": `
				function keep(x) { return x }
				keep++
				console.log(keep(1))
				keep(foo())
				keep(1)
			`,

			"/reassign-div.js": `
				function keep(x) { return x }
				keep /= reassigned
				console.log(keep(1))
				keep(foo())
				keep(1)
			`,

			"/reassign-array.js": `
				function keep(x) { return x }
				[keep] = reassigned
				console.log(keep(1))
				keep(foo())
				keep(1)
			`,

			"/reassign-object.js": `
				function keep(x) { return x }
				({keep} = reassigned)
				console.log(keep(1))
				keep(foo())
				keep(1)
			`,

			"/not-identity-two-args.js": `
				function keep(x, y) { return x }
				console.log(keep(1))
				keep(foo())
				keep(1)
			`,

			"/not-identity-default.js": `
				function keep(x = foo()) { return x }
				console.log(keep(1))
				keep(foo())
				keep(1)
			`,

			"/not-identity-array.js": `
				function keep([x]) { return x }
				console.log(keep(1))
				keep(foo())
				keep(1)
			`,

			"/not-identity-object.js": `
				function keep({x}) { return x }
				console.log(keep(1))
				keep(foo())
				keep(1)
			`,

			"/not-identity-rest.js": `
				function keep(...x) { return x }
				console.log(keep(1))
				keep(foo())
				keep(1)
			`,

			"/not-identity-return.js": `
				function keep(x) { return [x] }
				console.log(keep(1))
				keep(foo())
				keep(1)
			`,
		},
		entryPaths: []string{
			"/identity.js",
			"/identity-last.js",
			"/identity-first.js",
			"/identity-generator.js",
			"/identity-async.js",
			"/identity-cross-module.js",
			"/identity-no-args.js",
			"/identity-two-args.js",
			"/reassign.js",
			"/reassign-inc.js",
			"/reassign-div.js",
			"/reassign-array.js",
			"/reassign-object.js",
			"/not-identity-two-args.js",
			"/not-identity-default.js",
			"/not-identity-array.js",
			"/not-identity-object.js",
			"/not-identity-rest.js",
			"/not-identity-return.js",
		},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			MangleSyntax: true,
		},
	})
}

func TestInlineEmptyFunctionCalls(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/empty.js": `
				function DROP() {}
				console.log(DROP(foo(), bar()))
				console.log(DROP(foo(), 1))
				console.log(DROP(1, foo()))
				console.log(DROP(1))
				console.log(DROP())
				DROP(foo(), bar())
				DROP(foo(), 1)
				DROP(1, foo())
				DROP(1)
				DROP()
			`,

			"/empty-comma.js": `
				function DROP() {}
				console.log((DROP(), DROP(), foo()))
				console.log((DROP(), foo(), DROP()))
				console.log((foo(), DROP(), DROP()))
				for (DROP(); DROP(); DROP()) DROP();
				DROP(), DROP(), foo();
				DROP(), foo(), DROP();
				foo(), DROP(), DROP();
			`,

			"/empty-last.js": `
				function DROP() { return x }
				function DROP() { return }
				console.log(DROP())
				DROP()
			`,

			"/empty-cross-module.js": `
				import { DROP } from './empty-cross-module-def'
				console.log(DROP())
				DROP()
			`,

			"/empty-cross-module-def.js": `
				export function DROP() {}
			`,

			"/empty-first.js": `
				function keep() { return }
				function keep() { return x }
				console.log(keep())
				keep(foo())
				keep(1)
			`,

			"/empty-generator.js": `
				function* keep() {}
				console.log(keep())
				keep(foo())
				keep(1)
			`,

			"/empty-async.js": `
				async function keep() {}
				console.log(keep())
				keep(foo())
				keep(1)
			`,

			"/reassign.js": `
				function keep() {}
				keep = reassigned
				console.log(keep())
				keep(foo())
				keep(1)
			`,

			"/reassign-inc.js": `
				function keep() {}
				keep++
				console.log(keep(1))
				keep(foo())
				keep(1)
			`,

			"/reassign-div.js": `
				function keep() {}
				keep /= reassigned
				console.log(keep(1))
				keep(foo())
				keep(1)
			`,

			"/reassign-array.js": `
				function keep() {}
				[keep] = reassigned
				console.log(keep(1))
				keep(foo())
				keep(1)
			`,

			"/reassign-object.js": `
				function keep() {}
				({keep} = reassigned)
				console.log(keep(1))
				keep(foo())
				keep(1)
			`,
		},
		entryPaths: []string{
			"/empty.js",
			"/empty-comma.js",
			"/empty-last.js",
			"/empty-cross-module.js",
			"/empty-first.js",
			"/empty-generator.js",
			"/empty-async.js",
			"/reassign.js",
			"/reassign-inc.js",
			"/reassign-div.js",
			"/reassign-array.js",
			"/reassign-object.js",
		},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			MangleSyntax: true,
		},
	})
}

func TestInlineFunctionCallBehaviorChanges(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function empty() {}
				function id(x) { return x }

				export let shouldBeWrapped = [
					id(foo.bar)(),
					id(foo[bar])(),
					id(foo?.bar)(),
					id(foo?.[bar])(),

					(empty(), foo.bar)(),
					(empty(), foo[bar])(),
					(empty(), foo?.bar)(),
					(empty(), foo?.[bar])(),

					id(eval)(),
					id(eval)?.(),
					(empty(), eval)(),
					(empty(), eval)?.(),

					id(foo.bar)` + "``" + `,
					id(foo[bar])` + "``" + `,
					id(foo?.bar)` + "``" + `,
					id(foo?.[bar])` + "``" + `,

					(empty(), foo.bar)` + "``" + `,
					(empty(), foo[bar])` + "``" + `,
					(empty(), foo?.bar)` + "``" + `,
					(empty(), foo?.[bar])` + "``" + `,

					delete id(foo),
					delete id(foo.bar),
					delete id(foo[bar]),
					delete id(foo?.bar),
					delete id(foo?.[bar]),

					delete (empty(), foo),
					delete (empty(), foo.bar),
					delete (empty(), foo[bar]),
					delete (empty(), foo?.bar),
					delete (empty(), foo?.[bar]),

					delete empty(),
				]

				export let shouldNotBeWrapped = [
					id(foo)(),
					(empty(), foo)(),

					id(foo)` + "``" + `,
					(empty(), foo)` + "``" + `,
				]

				export let shouldNotBeDoubleWrapped = [
					delete (empty(), foo(), bar()),
					delete id((foo(), bar())),
				]
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModePassThrough,
			AbsOutputDir: "/out",
			MangleSyntax: true,
		},
	})
}
