package bundler_tests

import (
	"regexp"
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
		expectedScanLog: `Users/user/project/src/entry.js: WARNING: Ignoring this import because "Users/user/project/node_modules/demo-pkg/index.js" was marked as having no side effects
Users/user/project/node_modules/demo-pkg/package.json: NOTE: "sideEffects" is false in the enclosing "package.json" file:
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
		expectedScanLog: `Users/user/project/src/entry.js: WARNING: Ignoring this import because "Users/user/project/node_modules/demo-pkg/index.js" was marked as having no side effects
Users/user/project/node_modules/demo-pkg/package.json: NOTE: "sideEffects" is false in the enclosing "package.json" file:
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
		expectedScanLog: `Users/user/project/src/entry.js: WARNING: Ignoring this import because "Users/user/project/node_modules/demo-pkg/index.js" was marked as having no side effects
Users/user/project/node_modules/demo-pkg/package.json: NOTE: "sideEffects" is false in the enclosing "package.json" file:
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
		expectedScanLog: `Users/user/project/src/entry.js: WARNING: Ignoring this import because "Users/user/project/node_modules/demo-pkg/index.js" was marked as having no side effects
Users/user/project/node_modules/demo-pkg/package.json: NOTE: "sideEffects" is false in the enclosing "package.json" file:
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
		expectedScanLog: `Users/user/project/src/entry.js: WARNING: Ignoring this import because "Users/user/project/node_modules/demo-pkg/remove/this/file.js" was marked as having no side effects
Users/user/project/node_modules/demo-pkg/package.json: NOTE: It was excluded from the "sideEffects" array in the enclosing "package.json" file:
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
			MinifySyntax:  true,
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
			MinifySyntax:  true,
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

func TestTreeShakingObjectProperty(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				let remove1 = { x: 'x' }
				let remove2 = { x() {} }
				let remove3 = { get x() {} }
				let remove4 = { set x(_) {} }
				let remove5 = { async x() {} }
				let remove6 = { ['x']: 'x' }
				let remove7 = { ['x']() {} }
				let remove8 = { get ['x']() {} }
				let remove9 = { set ['x'](_) {} }
				let remove10 = { async ['x']() {} }
				let remove11 = { [0]: 'x' }
				let remove12 = { [null]: 'x' }
				let remove13 = { [undefined]: 'x' }
				let remove14 = { [false]: 'x' }
				let remove15 = { [0n]: 'x' }
				let remove16 = { toString() {} }

				let keep1 = { x }
				let keep2 = { x: x }
				let keep3 = { ...x }
				let keep4 = { [x]: 'x' }
				let keep5 = { [x]() {} }
				let keep6 = { get [x]() {} }
				let keep7 = { set [x](_) {} }
				let keep8 = { async [x]() {} }
				let keep9 = { [{ toString() {} }]: 'x' }
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			TreeShaking:   true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTreeShakingClassProperty(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				let remove1 = class { x }
				let remove2 = class { x = x }
				let remove3 = class { x() {} }
				let remove4 = class { get x() {} }
				let remove5 = class { set x(_) {} }
				let remove6 = class { async x() {} }
				let remove7 = class { ['x'] = x }
				let remove8 = class { ['x']() {} }
				let remove9 = class { get ['x']() {} }
				let remove10 = class { set ['x'](_) {} }
				let remove11 = class { async ['x']() {} }
				let remove12 = class { [0] = 'x' }
				let remove13 = class { [null] = 'x' }
				let remove14 = class { [undefined] = 'x' }
				let remove15 = class { [false] = 'x' }
				let remove16 = class { [0n] = 'x' }
				let remove17 = class { toString() {} }

				let keep1 = class { [x] = 'x' }
				let keep2 = class { [x]() {} }
				let keep3 = class { get [x]() {} }
				let keep4 = class { set [x](_) {} }
				let keep5 = class { async [x]() {} }
				let keep6 = class { [{ toString() {} }] = 'x' }
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			TreeShaking:   true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTreeShakingClassStaticProperty(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				let remove1 = class { static x }
				let remove3 = class { static x() {} }
				let remove4 = class { static get x() {} }
				let remove5 = class { static set x(_) {} }
				let remove6 = class { static async x() {} }
				let remove8 = class { static ['x']() {} }
				let remove9 = class { static get ['x']() {} }
				let remove10 = class { static set ['x'](_) {} }
				let remove11 = class { static async ['x']() {} }
				let remove12 = class { static [0] = 'x' }
				let remove13 = class { static [null] = 'x' }
				let remove14 = class { static [undefined] = 'x' }
				let remove15 = class { static [false] = 'x' }
				let remove16 = class { static [0n] = 'x' }
				let remove17 = class { static toString() {} }

				let keep1 = class { static x = x }
				let keep2 = class { static ['x'] = x }
				let keep3 = class { static [x] = 'x' }
				let keep4 = class { static [x]() {} }
				let keep5 = class { static get [x]() {} }
				let keep6 = class { static set [x](_) {} }
				let keep7 = class { static async [x]() {} }
				let keep8 = class { static [{ toString() {} }] = 'x' }
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			TreeShaking:   true,
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
			OutputFormat:  config.FormatIIFE,
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
			MinifySyntax:  true,
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

func TestDCETypeOfCompareStringGuardCondition(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				// Everything here should be removed as dead code due to tree shaking
				var REMOVE_1 = typeof x <= 'u' ? x : null
				var REMOVE_1 = typeof x < 'u' ? x : null
				var REMOVE_1 = typeof x >= 'u' ? null : x
				var REMOVE_1 = typeof x > 'u' ? null : x
				var REMOVE_1 = typeof x <= 'u' && x
				var REMOVE_1 = typeof x < 'u' && x
				var REMOVE_1 = typeof x >= 'u' || x
				var REMOVE_1 = typeof x > 'u' || x
				var REMOVE_1 = 'u' >= typeof x ? x : null
				var REMOVE_1 = 'u' > typeof x ? x : null
				var REMOVE_1 = 'u' <= typeof x ? null : x
				var REMOVE_1 = 'u' < typeof x ? null : x
				var REMOVE_1 = 'u' >= typeof x && x
				var REMOVE_1 = 'u' > typeof x && x
				var REMOVE_1 = 'u' <= typeof x || x
				var REMOVE_1 = 'u' < typeof x || x

				// Everything here should be kept as live code because it has side effects
				var keep_1 = typeof x <= 'u' ? y : null
				var keep_1 = typeof x < 'u' ? y : null
				var keep_1 = typeof x >= 'u' ? null : y
				var keep_1 = typeof x > 'u' ? null : y
				var keep_1 = typeof x <= 'u' && y
				var keep_1 = typeof x < 'u' && y
				var keep_1 = typeof x >= 'u' || y
				var keep_1 = typeof x > 'u' || y
				var keep_1 = 'u' >= typeof x ? y : null
				var keep_1 = 'u' > typeof x ? y : null
				var keep_1 = 'u' <= typeof x ? null : y
				var keep_1 = 'u' < typeof x ? null : y
				var keep_1 = 'u' >= typeof x && y
				var keep_1 = 'u' > typeof x && y
				var keep_1 = 'u' <= typeof x || y
				var keep_1 = 'u' < typeof x || y

				// Everything here should be kept as live code because it has side effects
				var keep_2 = typeof x <= 'u' ? null : x
				var keep_2 = typeof x < 'u' ? null : x
				var keep_2 = typeof x >= 'u' ? x : null
				var keep_2 = typeof x > 'u' ? x : null
				var keep_2 = typeof x <= 'u' || x
				var keep_2 = typeof x < 'u' || x
				var keep_2 = typeof x >= 'u' && x
				var keep_2 = typeof x > 'u' && x
				var keep_2 = 'u' >= typeof x ? null : x
				var keep_2 = 'u' > typeof x ? null : x
				var keep_2 = 'u' <= typeof x ? x : null
				var keep_2 = 'u' < typeof x ? x : null
				var keep_2 = 'u' >= typeof x || x
				var keep_2 = 'u' > typeof x || x
				var keep_2 = 'u' <= typeof x && x
				var keep_2 = 'u' < typeof x && x
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
			MinifySyntax:  true,
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
			MinifySyntax:  true,
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
			MinifySyntax:  true,
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
			MinifySyntax:          true,
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
			Mode:                  config.ModeBundle,
			AbsOutputDir:          "/out",
			UnsupportedJSFeatures: compat.ClassField,
			TS: config.TSOptions{Config: config.TSConfig{
				UseDefineForClassFields: config.False,
			}},
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
			MinifySyntax: true,
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

			"/empty-if-else.js": `
				function DROP() {}
				if (foo) { let bar = baz(); bar(); bar() } else DROP();
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
			"/empty-if-else.js",
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
			MinifySyntax: true,
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
			MinifySyntax: true,
		},
	})
}

func TestInlineFunctionCallForInitDecl(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function empty() {}
				function id(x) { return x }

				for (var y = empty(); false; ) ;
				for (var z = id(123); false; ) ;
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			MinifySyntax: true,
		},
	})
}

func TestConstValueInliningNoBundle(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/top-level.js": `
				// These should be kept because they are top-level and tree shaking is not enabled
				const n_keep = null
				const u_keep = undefined
				const i_keep = 1234567
				const f_keep = 123.456
				const s_keep = ''

				// Values should still be inlined
				console.log(
					// These are doubled to avoid the "inline const/let into next statement if used once" optimization
					n_keep, n_keep,
					u_keep, u_keep,
					i_keep, i_keep,
					f_keep, f_keep,
					s_keep, s_keep,
				)
			`,
			"/nested-block.js": `
				{
					const REMOVE_n = null
					const REMOVE_u = undefined
					const REMOVE_i = 1234567
					const REMOVE_f = 123.456
					const s_keep = '' // String inlining is intentionally not supported right now
					console.log(
						// These are doubled to avoid the "inline const/let into next statement if used once" optimization
						REMOVE_n, REMOVE_n,
						REMOVE_u, REMOVE_u,
						REMOVE_i, REMOVE_i,
						REMOVE_f, REMOVE_f,
						s_keep, s_keep,
					)
				}
			`,
			"/nested-function.js": `
				function nested() {
					const REMOVE_n = null
					const REMOVE_u = undefined
					const REMOVE_i = 1234567
					const REMOVE_f = 123.456
					const s_keep = '' // String inlining is intentionally not supported right now
					console.log(
						// These are doubled to avoid the "inline const/let into next statement if used once" optimization
						REMOVE_n, REMOVE_n,
						REMOVE_u, REMOVE_u,
						REMOVE_i, REMOVE_i,
						REMOVE_f, REMOVE_f,
						s_keep, s_keep,
					)
				}
			`,
			"/namespace-export.ts": `
				namespace ns {
					const x_REMOVE = 1
					export const y_keep = 2
					console.log(
						x_REMOVE, x_REMOVE,
						y_keep, y_keep,
					)
				}
			`,

			"/comment-before.js": `
				{
					//! comment
					const REMOVE = 1
					x = [REMOVE, REMOVE]
				}
			`,
			"/directive-before.js": `
				function nested() {
					'directive'
					const REMOVE = 1
					x = [REMOVE, REMOVE]
				}
			`,
			"/semicolon-before.js": `
				{
					;
					const REMOVE = 1
					x = [REMOVE, REMOVE]
				}
			`,
			"/debugger-before.js": `
				{
					debugger
					const REMOVE = 1
					x = [REMOVE, REMOVE]
				}
			`,
			"/type-before.ts": `
				{
					declare let x
					const REMOVE = 1
					x = [REMOVE, REMOVE]
				}
			`,
			"/exprs-before.js": `
				function nested() {
					const x = [, '', {}, 0n, /./, function() {}, () => {}]
					const y_REMOVE = 1
					function foo() {
						return y_REMOVE
					}
				}
			`,

			"/disabled-tdz.js": `
				foo()
				const x_keep = 1
				function foo() {
					return x_keep
				}
			`,
			"/backwards-reference-top-level.js": `
				const x = y
				const y = 1
				console.log(
					x, x,
					y, y,
				)
			`,
			"/backwards-reference-nested-function.js": `
				function foo() {
					const x = y
					const y = 1
					console.log(
						x, x,
						y, y,
					)
				}
			`,
		},
		entryPaths: []string{
			"/top-level.js",
			"/nested-block.js",
			"/nested-function.js",
			"/namespace-export.ts",

			"/comment-before.js",
			"/directive-before.js",
			"/semicolon-before.js",
			"/debugger-before.js",
			"/type-before.ts",
			"/exprs-before.js",

			"/disabled-tdz.js",
			"/backwards-reference-top-level.js",
			"/backwards-reference-nested-function.js",
		},
		options: config.Options{
			Mode:         config.ModePassThrough,
			AbsOutputDir: "/out",
			MinifySyntax: true,
		},
	})
}

func TestConstValueInliningBundle(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/exported-entry.js": `
				const x_REMOVE = 1
				export const y_keep = 2
				console.log(
					x_REMOVE,
					y_keep,
				)
			`,

			"/re-exported-entry.js": `
				import { x_REMOVE, y_keep } from './re-exported-constants'
				console.log(x_REMOVE, y_keep)
				export { y_keep }
			`,
			"/re-exported-constants.js": `
				export const x_REMOVE = 1
				export const y_keep = 2
			`,

			"/re-exported-2-entry.js": `
				export { y_keep } from './re-exported-2-constants'
			`,
			"/re-exported-2-constants.js": `
				export const x_REMOVE = 1
				export const y_keep = 2
			`,

			"/re-exported-star-entry.js": `
				export * from './re-exported-star-constants'
			`,
			"/re-exported-star-constants.js": `
				export const x_keep = 1
				export const y_keep = 2
			`,

			"/cross-module-entry.js": `
				import { x_REMOVE, y_keep } from './cross-module-constants'
				console.log(x_REMOVE, y_keep)
			`,
			"/cross-module-constants.js": `
				export const x_REMOVE = 1
				foo()
				export const y_keep = 1
				export function foo() {
					return [x_REMOVE, y_keep]
				}
			`,

			"/print-shorthand-entry.js": `
				import { foo, _bar } from './print-shorthand-constants'
				// The inlined constants must still be present in the output! We don't
				// want the printer to use the shorthand syntax here to refer to the
				// name of the constant itself because the constant declaration is omitted.
				console.log({ foo, _bar })
			`,
			"/print-shorthand-constants.js": `
				export const foo = 123
				export const _bar = -321
			`,

			"/circular-import-entry.js": `
				import './circular-import-constants'
			`,
			"/circular-import-constants.js": `
				export const foo = 123 // Inlining should be prevented by the cycle
				export function bar() {
					return foo
				}
				import './circular-import-cycle'
			`,
			"/circular-import-cycle.js": `
				import { bar } from './circular-import-constants'
				console.log(bar()) // This accesses "foo" before it's initialized
			`,

			"/circular-re-export-entry.js": `
				import { baz } from './circular-re-export-constants'
				console.log(baz)
			`,
			"/circular-re-export-constants.js": `
				export const foo = 123 // Inlining should be prevented by the cycle
				export function bar() {
					return foo
				}
				export { baz } from './circular-re-export-cycle'
			`,
			"/circular-re-export-cycle.js": `
				export const baz = 0
				import { bar } from './circular-re-export-constants'
				console.log(bar()) // This accesses "foo" before it's initialized
			`,

			"/circular-re-export-star-entry.js": `
				import './circular-re-export-star-constants'
			`,
			"/circular-re-export-star-constants.js": `
				export const foo = 123 // Inlining should be prevented by the cycle
				export function bar() {
					return foo
				}
				export * from './circular-re-export-star-cycle'
			`,
			"/circular-re-export-star-cycle.js": `
				import { bar } from './circular-re-export-star-constants'
				console.log(bar()) // This accesses "foo" before it's initialized
			`,

			"/non-circular-export-entry.js": `
				import { foo, bar } from './non-circular-export-constants'
				console.log(foo, bar())
			`,
			"/non-circular-export-constants.js": `
				const foo = 123 // Inlining should be prevented by the cycle
				function bar() {
					return foo
				}
				export { foo, bar }
			`,
		},
		entryPaths: []string{
			"/exported-entry.js",
			"/re-exported-entry.js",
			"/re-exported-2-entry.js",
			"/re-exported-star-entry.js",
			"/cross-module-entry.js",
			"/print-shorthand-entry.js",
			"/circular-import-entry.js",
			"/circular-re-export-entry.js",
			"/circular-re-export-star-entry.js",
			"/non-circular-export-entry.js",
		},
		options: config.Options{
			Mode:         config.ModeBundle,
			OutputFormat: config.FormatESModule,
			AbsOutputDir: "/out",
			MinifySyntax: true,
			MangleProps:  regexp.MustCompile("^_"),
		},
	})
}

// Assignment to an inlined constant is not allowed since that would cause a
// syntax error in the output. We don't just keep the reference there because
// the declaration may actually have been completely removed already by the
// time we discover the assignment. I think making these cases an error is
// fine because it's bad code anyway.
func TestConstValueInliningAssign(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/const-assign.js": `
				const x = 1
				x = 2
			`,
			"/const-update.js": `
				const x = 1
				x += 2
			`,
		},
		entryPaths: []string{
			"/const-assign.js",
			"/const-update.js",
		},
		options: config.Options{
			Mode:         config.ModePassThrough,
			AbsOutputDir: "/out",
			MinifySyntax: true,
		},
		expectedScanLog: `const-assign.js: ERROR: Cannot assign to "x" because it is a constant
const-assign.js: NOTE: The symbol "x" was declared a constant here:
const-update.js: ERROR: Cannot assign to "x" because it is a constant
const-update.js: NOTE: The symbol "x" was declared a constant here:
`,
	})
}

func TestConstValueInliningDirectEval(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/top-level-no-eval.js": `
				const x = 1
				console.log(x, evil('x'))
			`,
			"/top-level-eval.js": `
				const x = 1
				console.log(x, eval('x'))
			`,
			"/nested-no-eval.js": `
				(() => {
					const x = 1
					console.log(x, evil('x'))
				})()
			`,
			"/nested-eval.js": `
				(() => {
					const x = 1
					console.log(x, eval('x'))
				})()
			`,
			"/ts-namespace-no-eval.ts": `
				namespace y {
					export const x = 1
					console.log(x, evil('x'))
				}
			`,
			"/ts-namespace-eval.ts": `
				namespace z {
					export const x = 1
					console.log(x, eval('x'))
				}
			`,
		},
		entryPaths: []string{
			"/top-level-no-eval.js",
			"/top-level-eval.js",
			"/nested-no-eval.js",
			"/nested-eval.js",
			"/ts-namespace-no-eval.ts",
			"/ts-namespace-eval.ts",
		},
		options: config.Options{
			Mode:         config.ModePassThrough,
			AbsOutputDir: "/out",
			MinifySyntax: true,
		},
	})
}

func TestCrossModuleConstantFolding(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/enum-constants.ts": `
				export enum x {
					a = 3,
					b = 6,
				}
			`,
			"/enum-entry.ts": `
				import { x } from './enum-constants'
				console.log([
					+x.b,
					-x.b,
					~x.b,
					!x.b,
					typeof x.b,
				], [
					x.a + x.b,
					x.a - x.b,
					x.a * x.b,
					x.a / x.b,
					x.a % x.b,
					x.a ** x.b,
				], [
					x.a < x.b,
					x.a > x.b,
					x.a <= x.b,
					x.a >= x.b,
					x.a == x.b,
					x.a != x.b,
					x.a === x.b,
					x.a !== x.b,
				], [
					x.b << 1,
					x.b >> 1,
					x.b >>> 1,
				], [
					x.a & x.b,
					x.a | x.b,
					x.a ^ x.b,
				], [
					x.a && x.b,
					x.a || x.b,
					x.a ?? x.b,
				])
			`,

			"/const-constants.js": `
				export const a = 3
				export const b = 6
			`,
			"/const-entry.js": `
				import { a, b } from './const-constants'
				console.log([
					+b,
					-b,
					~b,
					!b,
					typeof b,
				], [
					a + b,
					a - b,
					a * b,
					a / b,
					a % b,
					a ** b,
				], [
					a < b,
					a > b,
					a <= b,
					a >= b,
					a == b,
					a != b,
					a === b,
					a !== b,
				], [
					b << 1,
					b >> 1,
					b >>> 1,
				], [
					a & b,
					a | b,
					a ^ b,
				], [
					a && b,
					a || b,
					a ?? b,
				])
			`,

			"/nested-constants.ts": `
				export const a = 2
				export const b = 4
				export const c = 8
				export enum x {
					a = 16,
					b = 32,
					c = 64,
				}
			`,
			"/nested-entry.ts": `
				import { a, b, c, x } from './nested-constants'
				console.log({
					'should be 4': ~(~a & ~b) & (b | c),
					'should be 32': ~(~x.a & ~x.b) & (x.b | x.c),
				})
			`,
		},
		entryPaths: []string{
			"/enum-entry.ts",
			"/const-entry.js",
			"/nested-entry.ts",
		},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			MinifySyntax: true,
		},
	})
}

func TestMultipleDeclarationTreeShaking(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/var2.js": `
				var x = 1
				console.log(x)
				var x = 2
			`,
			"/var3.js": `
				var x = 1
				console.log(x)
				var x = 2
				console.log(x)
				var x = 3
			`,
			"/function2.js": `
				function x() { return 1 }
				console.log(x())
				function x() { return 2 }
			`,
			"/function3.js": `
				function x() { return 1 }
				console.log(x())
				function x() { return 2 }
				console.log(x())
				function x() { return 3 }
			`,
		},
		entryPaths: []string{
			"/var2.js",
			"/var3.js",
			"/function2.js",
			"/function3.js",
		},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			MinifySyntax: false,
		},
	})
}

func TestMultipleDeclarationTreeShakingMinifySyntax(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/var2.js": `
				var x = 1
				console.log(x)
				var x = 2
			`,
			"/var3.js": `
				var x = 1
				console.log(x)
				var x = 2
				console.log(x)
				var x = 3
			`,
			"/function2.js": `
				function x() { return 1 }
				console.log(x())
				function x() { return 2 }
			`,
			"/function3.js": `
				function x() { return 1 }
				console.log(x())
				function x() { return 2 }
				console.log(x())
				function x() { return 3 }
			`,
		},
		entryPaths: []string{
			"/var2.js",
			"/var3.js",
			"/function2.js",
			"/function3.js",
		},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			MinifySyntax: true,
		},
	})
}

// Pure call removal should still run iterators, which can have side effects
func TestPureCallsWithSpread(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				/* @__PURE__ */ foo(...args);
				/* @__PURE__ */ new foo(...args);
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			MinifySyntax:  true,
		},
	})
}

func TestTopLevelFunctionInliningWithSpread(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function empty1() {}
				function empty2() {}
				function empty3() {}

				function identity1(x) { return x }
				function identity2(x) { return x }
				function identity3(x) { return x }

				empty1()
				empty2(args)
				empty3(...args)

				identity1()
				identity2(args)
				identity3(...args)
			`,

			"/inner.js": `
				export function empty1() {}
				export function empty2() {}
				export function empty3() {}

				export function identity1(x) { return x }
				export function identity2(x) { return x }
				export function identity3(x) { return x }
			`,

			"/entry-outer.js": `
				import {
					empty1,
					empty2,
					empty3,

					identity1,
					identity2,
					identity3,
				} from './inner.js'

				empty1()
				empty2(args)
				empty3(...args)

				identity1()
				identity2(args)
				identity3(...args)
			`,
		},
		entryPaths: []string{"/entry.js", "/entry-outer.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			MinifySyntax: true,
		},
	})
}

func TestNestedFunctionInliningWithSpread(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function empty1() {}
				function empty2() {}
				function empty3() {}

				function identity1(x) { return x }
				function identity2(x) { return x }
				function identity3(x) { return x }

				check(
					empty1(),
					empty2(args),
					empty3(...args),

					identity1(),
					identity2(args),
					identity3(...args),
				)
			`,

			"/inner.js": `
				export function empty1() {}
				export function empty2() {}
				export function empty3() {}

				export function identity1(x) { return x }
				export function identity2(x) { return x }
				export function identity3(x) { return x }
			`,

			"/entry-outer.js": `
				import {
					empty1,
					empty2,
					empty3,

					identity1,
					identity2,
					identity3,
				} from './inner.js'

				check(
					empty1(),
					empty2(args),
					empty3(...args),

					identity1(),
					identity2(args),
					identity3(...args),
				)
			`,
		},
		entryPaths: []string{"/entry.js", "/entry-outer.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			MinifySyntax: true,
		},
	})
}

func TestPackageJsonSideEffectsFalseCrossPlatformSlash(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import "demo-pkg/foo"
				import "demo-pkg/bar"
			`,
			"/Users/user/project/node_modules/demo-pkg/foo.js": `
				console.log('foo')
			`,
			"/Users/user/project/node_modules/demo-pkg/bar/index.js": `
				console.log('bar')
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"sideEffects": [
						"**/foo.js",
						"bar/index.js"
					]
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

func TestTreeShakingJSWithAssociatedCSS(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/project/test.jsx": `
				import { Button } from 'pkg/button'
				import { Menu } from 'pkg/menu'
				render(<Button/>)
			`,
			"/project/node_modules/pkg/button.js": `
				import './button.css'
				export let Button
			`,
			"/project/node_modules/pkg/button.css": `
				button { color: red }
			`,
			"/project/node_modules/pkg/menu.js": `
				import './menu.css'
				export let Menu
			`,
			"/project/node_modules/pkg/menu.css": `
				menu { color: red }
			`,
		},
		entryPaths: []string{"/project/test.jsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestTreeShakingJSWithAssociatedCSSReExportSideEffectsFalse(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/project/test.jsx": `
				import { Button } from 'pkg'
				render(<Button/>)
			`,
			"/project/node_modules/pkg/entry.js": `
				export { Button } from './components'
			`,
			"/project/node_modules/pkg/package.json": `{
				"main": "./entry.js",
				"sideEffects": false
			}`,
			"/project/node_modules/pkg/components.jsx": `
				require('./button.css')
				export const Button = () => <button/>
			`,
			"/project/node_modules/pkg/button.css": `
				button { color: red }
			`,
		},
		entryPaths: []string{"/project/test.jsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestTreeShakingJSWithAssociatedCSSReExportSideEffectsFalseOnlyJS(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/project/test.jsx": `
				import { Button } from 'pkg'
				render(<Button/>)
			`,
			"/project/node_modules/pkg/entry.js": `
				export { Button } from './components'
			`,
			"/project/node_modules/pkg/package.json": `{
				"main": "./entry.js",
				"sideEffects": ["*.css"]
			}`,
			"/project/node_modules/pkg/components.jsx": `
				require('./button.css')
				export const Button = () => <button/>
			`,
			"/project/node_modules/pkg/button.css": `
				button { color: red }
			`,
		},
		entryPaths: []string{"/project/test.jsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestTreeShakingJSWithAssociatedCSSExportStarSideEffectsFalse(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/project/test.jsx": `
				import { Button } from 'pkg'
				render(<Button/>)
			`,
			"/project/node_modules/pkg/entry.js": `
				export * from './components'
			`,
			"/project/node_modules/pkg/package.json": `{
				"main": "./entry.js",
				"sideEffects": false
			}`,
			"/project/node_modules/pkg/components.jsx": `
				require('./button.css')
				export const Button = () => <button/>
			`,
			"/project/node_modules/pkg/button.css": `
				button { color: red }
			`,
		},
		entryPaths: []string{"/project/test.jsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestTreeShakingJSWithAssociatedCSSExportStarSideEffectsFalseOnlyJS(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/project/test.jsx": `
				import { Button } from 'pkg'
				render(<Button/>)
			`,
			"/project/node_modules/pkg/entry.js": `
				export * from './components'
			`,
			"/project/node_modules/pkg/package.json": `{
				"main": "./entry.js",
				"sideEffects": ["*.css"]
			}`,
			"/project/node_modules/pkg/components.jsx": `
				require('./button.css')
				export const Button = () => <button/>
			`,
			"/project/node_modules/pkg/button.css": `
				button { color: red }
			`,
		},
		entryPaths: []string{"/project/test.jsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestTreeShakingJSWithAssociatedCSSUnusedNestedImportSideEffectsFalse(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/project/test.jsx": `
				import { Button } from 'pkg/button'
				render(<Button/>)
			`,
			"/project/node_modules/pkg/package.json": `{
				"sideEffects": false
			}`,
			"/project/node_modules/pkg/button.jsx": `
				import styles from './styles'
				export const Button = () => <button/>
			`,
			"/project/node_modules/pkg/styles.js": `
				import './styles.css'
				export default {}
			`,
			"/project/node_modules/pkg/styles.css": `
				button { color: red }
			`,
		},
		entryPaths: []string{"/project/test.jsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestTreeShakingJSWithAssociatedCSSUnusedNestedImportSideEffectsFalseOnlyJS(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/project/test.jsx": `
				import { Button } from 'pkg/button'
				render(<Button/>)
			`,
			"/project/node_modules/pkg/package.json": `{
				"sideEffects": ["*.css"]
			}`,
			"/project/node_modules/pkg/button.jsx": `
				import styles from './styles'
				export const Button = () => <button/>
			`,
			"/project/node_modules/pkg/styles.js": `
				import './styles.css'
				export default {}
			`,
			"/project/node_modules/pkg/styles.css": `
				button { color: red }
			`,
		},
		entryPaths: []string{"/project/test.jsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestPreserveDirectivesMinifyPassThrough(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				//! 1
				'use 1'
				//! 2
				'use 2'
				//! 3
				'use 3'
				entry()
				//! 4
				'use 4'
				//! 5
				'use 5'
				//! 6
				'use 6'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			AbsOutputFile: "/out.js",
			MinifySyntax:  true,
		},
	})
}

func TestPreserveDirectivesMinifyIIFE(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				//! 1
				'use 1'
				//! 2
				'use 2'
				//! 3
				'use 3'
				entry()
				//! 4
				'use 4'
				//! 5
				'use 5'
				//! 6
				'use 6'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeConvertFormat,
			OutputFormat:  config.FormatIIFE,
			AbsOutputFile: "/out.js",
			MinifySyntax:  true,
		},
	})
}

func TestPreserveDirectivesMinifyBundle(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				//! 1
				'use 1'
				//! 2
				'use 2'
				//! 3
				'use 3'
				entry()
				//! 4
				'use 4'
				//! 5
				'use 5'
				//! 6
				'use 6'
				import "./nested.js"
			`,
			"/nested.js": `
				//! A
				'use A'
				//! B
				'use B'
				//! C
				'use C'
				nested()
				//! D
				'use D'
				//! E
				'use E'
				//! F
				'use F'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatIIFE,
			AbsOutputFile: "/out.js",
			MinifySyntax:  true,
		},
	})
}

// See: https://github.com/rollup/rollup/pull/5024
func TestNoSideEffectsComment(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/expr-fn.js": `
				//! These should all have "no side effects"
				x([
					/* #__NO_SIDE_EFFECTS__ */ function() {},
					/* #__NO_SIDE_EFFECTS__ */ function y() {},
					/* #__NO_SIDE_EFFECTS__ */ function*() {},
					/* #__NO_SIDE_EFFECTS__ */ function* y() {},
					/* #__NO_SIDE_EFFECTS__ */ async function() {},
					/* #__NO_SIDE_EFFECTS__ */ async function y() {},
					/* #__NO_SIDE_EFFECTS__ */ async function*() {},
					/* #__NO_SIDE_EFFECTS__ */ async function* y() {},
				])
			`,
			"/expr-arrow.js": `
				//! These should all have "no side effects"
				x([
					/* #__NO_SIDE_EFFECTS__ */ y => y,
					/* #__NO_SIDE_EFFECTS__ */ () => {},
					/* #__NO_SIDE_EFFECTS__ */ (y) => (y),
					/* #__NO_SIDE_EFFECTS__ */ async y => y,
					/* #__NO_SIDE_EFFECTS__ */ async () => {},
					/* #__NO_SIDE_EFFECTS__ */ async (y) => (y),
				])
			`,

			"/stmt-fn.js": `
				//! These should all have "no side effects"
				// #__NO_SIDE_EFFECTS__
				function a() {}
				// #__NO_SIDE_EFFECTS__
				function* b() {}
				// #__NO_SIDE_EFFECTS__
				async function c() {}
				// #__NO_SIDE_EFFECTS__
				async function* d() {}
			`,
			"/stmt-export-fn.js": `
				//! These should all have "no side effects"
				/* @__NO_SIDE_EFFECTS__ */ export function a() {}
				/* @__NO_SIDE_EFFECTS__ */ export function* b() {}
				/* @__NO_SIDE_EFFECTS__ */ export async function c() {}
				/* @__NO_SIDE_EFFECTS__ */ export async function* d() {}
			`,
			"/stmt-local.js": `
				//! Only "c0" and "c2" should have "no side effects" (Rollup only respects "const" and only for the first one)
				/* #__NO_SIDE_EFFECTS__ */ var v0 = function() {}, v1 = function() {}
				/* #__NO_SIDE_EFFECTS__ */ let l0 = function() {}, l1 = function() {}
				/* #__NO_SIDE_EFFECTS__ */ const c0 = function() {}, c1 = function() {}
				/* #__NO_SIDE_EFFECTS__ */ var v2 = () => {}, v3 = () => {}
				/* #__NO_SIDE_EFFECTS__ */ let l2 = () => {}, l3 = () => {}
				/* #__NO_SIDE_EFFECTS__ */ const c2 = () => {}, c3 = () => {}
			`,
			"/stmt-export-local.js": `
				//! Only "c0" and "c2" should have "no side effects" (Rollup only respects "const" and only for the first one)
				/* #__NO_SIDE_EFFECTS__ */ export var v0 = function() {}, v1 = function() {}
				/* #__NO_SIDE_EFFECTS__ */ export let l0 = function() {}, l1 = function() {}
				/* #__NO_SIDE_EFFECTS__ */ export const c0 = function() {}, c1 = function() {}
				/* #__NO_SIDE_EFFECTS__ */ export var v2 = () => {}, v3 = () => {}
				/* #__NO_SIDE_EFFECTS__ */ export let l2 = () => {}, l3 = () => {}
				/* #__NO_SIDE_EFFECTS__ */ export const c2 = () => {}, c3 = () => {}
			`,

			"/ns-export-fn.ts": `
				namespace ns {
					//! These should all have "no side effects"
					/* @__NO_SIDE_EFFECTS__ */ export function a() {}
					/* @__NO_SIDE_EFFECTS__ */ export function* b() {}
					/* @__NO_SIDE_EFFECTS__ */ export async function c() {}
					/* @__NO_SIDE_EFFECTS__ */ export async function* d() {}
				}
			`,
			"/ns-export-local.ts": `
				namespace ns {
					//! Only "c0" and "c2" should have "no side effects" (Rollup only respects "const" and only for the first one)
					/* #__NO_SIDE_EFFECTS__ */ export var v0 = function() {}, v1 = function() {}
					/* #__NO_SIDE_EFFECTS__ */ export let l0 = function() {}, l1 = function() {}
					/* #__NO_SIDE_EFFECTS__ */ export const c0 = function() {}, c1 = function() {}
					/* #__NO_SIDE_EFFECTS__ */ export var v2 = () => {}, v3 = () => {}
					/* #__NO_SIDE_EFFECTS__ */ export let l2 = () => {}, l3 = () => {}
					/* #__NO_SIDE_EFFECTS__ */ export const c2 = () => {}, c3 = () => {}
				}
			`,

			"/stmt-export-default-before-fn-anon.js":           `/*! This should have "no side effects" */ /* #__NO_SIDE_EFFECTS__ */ export default function() {}`,
			"/stmt-export-default-before-fn-name.js":           `/*! This should have "no side effects" */ /* #__NO_SIDE_EFFECTS__ */ export default function f() {}`,
			"/stmt-export-default-before-gen-fn-anon.js":       `/*! This should have "no side effects" */ /* #__NO_SIDE_EFFECTS__ */ export default function*() {}`,
			"/stmt-export-default-before-gen-fn-name.js":       `/*! This should have "no side effects" */ /* #__NO_SIDE_EFFECTS__ */ export default function* f() {}`,
			"/stmt-export-default-before-async-fn-anon.js":     `/*! This should have "no side effects" */ /* #__NO_SIDE_EFFECTS__ */ export default async function() {}`,
			"/stmt-export-default-before-async-fn-name.js":     `/*! This should have "no side effects" */ /* #__NO_SIDE_EFFECTS__ */ export default async function f() {}`,
			"/stmt-export-default-before-async-gen-fn-anon.js": `/*! This should have "no side effects" */ /* #__NO_SIDE_EFFECTS__ */ export default async function*() {}`,
			"/stmt-export-default-before-async-gen-fn-name.js": `/*! This should have "no side effects" */ /* #__NO_SIDE_EFFECTS__ */ export default async function* f() {}`,

			"/stmt-export-default-after-fn-anon.js":           `/*! This should have "no side effects" */ export default /* @__NO_SIDE_EFFECTS__ */ function() {}`,
			"/stmt-export-default-after-fn-name.js":           `/*! This should have "no side effects" */ export default /* @__NO_SIDE_EFFECTS__ */ function f() {}`,
			"/stmt-export-default-after-gen-fn-anon.js":       `/*! This should have "no side effects" */ export default /* @__NO_SIDE_EFFECTS__ */ function*() {}`,
			"/stmt-export-default-after-gen-fn-name.js":       `/*! This should have "no side effects" */ export default /* @__NO_SIDE_EFFECTS__ */ function* f() {}`,
			"/stmt-export-default-after-async-fn-anon.js":     `/*! This should have "no side effects" */ export default /* @__NO_SIDE_EFFECTS__ */ async function() {}`,
			"/stmt-export-default-after-async-fn-name.js":     `/*! This should have "no side effects" */ export default /* @__NO_SIDE_EFFECTS__ */ async function f() {}`,
			"/stmt-export-default-after-async-gen-fn-anon.js": `/*! This should have "no side effects" */ export default /* @__NO_SIDE_EFFECTS__ */ async function*() {}`,
			"/stmt-export-default-after-async-gen-fn-name.js": `/*! This should have "no side effects" */ export default /* @__NO_SIDE_EFFECTS__ */ async function* f() {}`,
		},
		entryPaths: []string{
			"/expr-fn.js",
			"/expr-arrow.js",

			"/stmt-fn.js",
			"/stmt-export-fn.js",
			"/stmt-local.js",
			"/stmt-export-local.js",

			"/ns-export-fn.ts",
			"/ns-export-local.ts",

			"/stmt-export-default-before-fn-anon.js",
			"/stmt-export-default-before-fn-name.js",
			"/stmt-export-default-before-gen-fn-anon.js",
			"/stmt-export-default-before-gen-fn-name.js",
			"/stmt-export-default-before-async-fn-anon.js",
			"/stmt-export-default-before-async-fn-name.js",
			"/stmt-export-default-before-async-gen-fn-anon.js",
			"/stmt-export-default-before-async-gen-fn-name.js",

			"/stmt-export-default-after-fn-anon.js",
			"/stmt-export-default-after-fn-name.js",
			"/stmt-export-default-after-gen-fn-anon.js",
			"/stmt-export-default-after-gen-fn-name.js",
			"/stmt-export-default-after-async-fn-anon.js",
			"/stmt-export-default-after-async-fn-name.js",
			"/stmt-export-default-after-async-gen-fn-anon.js",
			"/stmt-export-default-after-async-gen-fn-name.js",
		},
		options: config.Options{
			AbsOutputDir: "/out",
		},
	})
}

func TestNoSideEffectsCommentIgnoreAnnotations(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/expr-fn.js": `
				x([
					/* #__NO_SIDE_EFFECTS__ */ function() {},
					/* #__NO_SIDE_EFFECTS__ */ function y() {},
					/* #__NO_SIDE_EFFECTS__ */ function*() {},
					/* #__NO_SIDE_EFFECTS__ */ function* y() {},
					/* #__NO_SIDE_EFFECTS__ */ async function() {},
					/* #__NO_SIDE_EFFECTS__ */ async function y() {},
					/* #__NO_SIDE_EFFECTS__ */ async function*() {},
					/* #__NO_SIDE_EFFECTS__ */ async function* y() {},
				])
			`,
			"/expr-arrow.js": `
				x([
					/* #__NO_SIDE_EFFECTS__ */ y => y,
					/* #__NO_SIDE_EFFECTS__ */ () => {},
					/* #__NO_SIDE_EFFECTS__ */ (y) => (y),
					/* #__NO_SIDE_EFFECTS__ */ async y => y,
					/* #__NO_SIDE_EFFECTS__ */ async () => {},
					/* #__NO_SIDE_EFFECTS__ */ async (y) => (y),
				])
			`,

			"/stmt-fn.js": `
				// #__NO_SIDE_EFFECTS__
				function a() {}
				// #__NO_SIDE_EFFECTS__
				function* b() {}
				// #__NO_SIDE_EFFECTS__
				async function c() {}
				// #__NO_SIDE_EFFECTS__
				async function* d() {}
			`,
			"/stmt-export-fn.js": `
				/* @__NO_SIDE_EFFECTS__ */ export function a() {}
				/* @__NO_SIDE_EFFECTS__ */ export function* b() {}
				/* @__NO_SIDE_EFFECTS__ */ export async function c() {}
				/* @__NO_SIDE_EFFECTS__ */ export async function* d() {}
			`,
			"/stmt-local.js": `
				/* #__NO_SIDE_EFFECTS__ */ var v0 = function() {}, v1 = function() {}
				/* #__NO_SIDE_EFFECTS__ */ let l0 = function() {}, l1 = function() {}
				/* #__NO_SIDE_EFFECTS__ */ const c0 = function() {}, c1 = function() {}
				/* #__NO_SIDE_EFFECTS__ */ var v2 = () => {}, v3 = () => {}
				/* #__NO_SIDE_EFFECTS__ */ let l2 = () => {}, l3 = () => {}
				/* #__NO_SIDE_EFFECTS__ */ const c2 = () => {}, c3 = () => {}
			`,
			"/stmt-export-local.js": `
				/* #__NO_SIDE_EFFECTS__ */ export var v0 = function() {}, v1 = function() {}
				/* #__NO_SIDE_EFFECTS__ */ export let l0 = function() {}, l1 = function() {}
				/* #__NO_SIDE_EFFECTS__ */ export const c0 = function() {}, c1 = function() {}
				/* #__NO_SIDE_EFFECTS__ */ export var v2 = () => {}, v3 = () => {}
				/* #__NO_SIDE_EFFECTS__ */ export let l2 = () => {}, l3 = () => {}
				/* #__NO_SIDE_EFFECTS__ */ export const c2 = () => {}, c3 = () => {}
			`,

			"/ns-export-fn.ts": `
				namespace ns {
					/* @__NO_SIDE_EFFECTS__ */ export function a() {}
					/* @__NO_SIDE_EFFECTS__ */ export function* b() {}
					/* @__NO_SIDE_EFFECTS__ */ export async function c() {}
					/* @__NO_SIDE_EFFECTS__ */ export async function* d() {}
				}
			`,
			"/ns-export-local.ts": `
				namespace ns {
					/* #__NO_SIDE_EFFECTS__ */ export var v0 = function() {}, v1 = function() {}
					/* #__NO_SIDE_EFFECTS__ */ export let l0 = function() {}, l1 = function() {}
					/* #__NO_SIDE_EFFECTS__ */ export const c0 = function() {}, c1 = function() {}
					/* #__NO_SIDE_EFFECTS__ */ export var v2 = () => {}, v3 = () => {}
					/* #__NO_SIDE_EFFECTS__ */ export let l2 = () => {}, l3 = () => {}
					/* #__NO_SIDE_EFFECTS__ */ export const c2 = () => {}, c3 = () => {}
				}
			`,

			"/stmt-export-default-before-fn-anon.js":           `/* #__NO_SIDE_EFFECTS__ */ export default function() {}`,
			"/stmt-export-default-before-fn-name.js":           `/* #__NO_SIDE_EFFECTS__ */ export default function f() {}`,
			"/stmt-export-default-before-gen-fn-anon.js":       `/* #__NO_SIDE_EFFECTS__ */ export default function*() {}`,
			"/stmt-export-default-before-gen-fn-name.js":       `/* #__NO_SIDE_EFFECTS__ */ export default function* f() {}`,
			"/stmt-export-default-before-async-fn-anon.js":     `/* #__NO_SIDE_EFFECTS__ */ export default async function() {}`,
			"/stmt-export-default-before-async-fn-name.js":     `/* #__NO_SIDE_EFFECTS__ */ export default async function f() {}`,
			"/stmt-export-default-before-async-gen-fn-anon.js": `/* #__NO_SIDE_EFFECTS__ */ export default async function*() {}`,
			"/stmt-export-default-before-async-gen-fn-name.js": `/* #__NO_SIDE_EFFECTS__ */ export default async function* f() {}`,

			"/stmt-export-default-after-fn-anon.js":           `export default /* @__NO_SIDE_EFFECTS__ */ function() {}`,
			"/stmt-export-default-after-fn-name.js":           `export default /* @__NO_SIDE_EFFECTS__ */ function f() {}`,
			"/stmt-export-default-after-gen-fn-anon.js":       `export default /* @__NO_SIDE_EFFECTS__ */ function*() {}`,
			"/stmt-export-default-after-gen-fn-name.js":       `export default /* @__NO_SIDE_EFFECTS__ */ function* f() {}`,
			"/stmt-export-default-after-async-fn-anon.js":     `export default /* @__NO_SIDE_EFFECTS__ */ async function() {}`,
			"/stmt-export-default-after-async-fn-name.js":     `export default /* @__NO_SIDE_EFFECTS__ */ async function f() {}`,
			"/stmt-export-default-after-async-gen-fn-anon.js": `export default /* @__NO_SIDE_EFFECTS__ */ async function*() {}`,
			"/stmt-export-default-after-async-gen-fn-name.js": `export default /* @__NO_SIDE_EFFECTS__ */ async function* f() {}`,
		},
		entryPaths: []string{
			"/expr-fn.js",
			"/expr-arrow.js",

			"/stmt-fn.js",
			"/stmt-export-fn.js",
			"/stmt-local.js",
			"/stmt-export-local.js",

			"/ns-export-fn.ts",
			"/ns-export-local.ts",

			"/stmt-export-default-before-fn-anon.js",
			"/stmt-export-default-before-fn-name.js",
			"/stmt-export-default-before-gen-fn-anon.js",
			"/stmt-export-default-before-gen-fn-name.js",
			"/stmt-export-default-before-async-fn-anon.js",
			"/stmt-export-default-before-async-fn-name.js",
			"/stmt-export-default-before-async-gen-fn-anon.js",
			"/stmt-export-default-before-async-gen-fn-name.js",

			"/stmt-export-default-after-fn-anon.js",
			"/stmt-export-default-after-fn-name.js",
			"/stmt-export-default-after-gen-fn-anon.js",
			"/stmt-export-default-after-gen-fn-name.js",
			"/stmt-export-default-after-async-fn-anon.js",
			"/stmt-export-default-after-async-fn-name.js",
			"/stmt-export-default-after-async-gen-fn-anon.js",
			"/stmt-export-default-after-async-gen-fn-name.js",
		},
		options: config.Options{
			AbsOutputDir:         "/out",
			IgnoreDCEAnnotations: true,
		},
	})
}

func TestNoSideEffectsCommentMinifyWhitespace(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/expr-fn.js": `
				x([
					/* #__NO_SIDE_EFFECTS__ */ function() {},
					/* #__NO_SIDE_EFFECTS__ */ function y() {},
					/* #__NO_SIDE_EFFECTS__ */ function*() {},
					/* #__NO_SIDE_EFFECTS__ */ function* y() {},
					/* #__NO_SIDE_EFFECTS__ */ async function() {},
					/* #__NO_SIDE_EFFECTS__ */ async function y() {},
					/* #__NO_SIDE_EFFECTS__ */ async function*() {},
					/* #__NO_SIDE_EFFECTS__ */ async function* y() {},
				])
			`,
			"/expr-arrow.js": `
				x([
					/* #__NO_SIDE_EFFECTS__ */ y => y,
					/* #__NO_SIDE_EFFECTS__ */ () => {},
					/* #__NO_SIDE_EFFECTS__ */ (y) => (y),
					/* #__NO_SIDE_EFFECTS__ */ async y => y,
					/* #__NO_SIDE_EFFECTS__ */ async () => {},
					/* #__NO_SIDE_EFFECTS__ */ async (y) => (y),
				])
			`,

			"/stmt-fn.js": `
				// #__NO_SIDE_EFFECTS__
				function a() {}
				// #__NO_SIDE_EFFECTS__
				function* b() {}
				// #__NO_SIDE_EFFECTS__
				async function c() {}
				// #__NO_SIDE_EFFECTS__
				async function* d() {}
			`,
			"/stmt-export-fn.js": `
				/* @__NO_SIDE_EFFECTS__ */ export function a() {}
				/* @__NO_SIDE_EFFECTS__ */ export function* b() {}
				/* @__NO_SIDE_EFFECTS__ */ export async function c() {}
				/* @__NO_SIDE_EFFECTS__ */ export async function* d() {}
			`,
			"/stmt-local.js": `
				/* #__NO_SIDE_EFFECTS__ */ var v0 = function() {}, v1 = function() {}
				/* #__NO_SIDE_EFFECTS__ */ let l0 = function() {}, l1 = function() {}
				/* #__NO_SIDE_EFFECTS__ */ const c0 = function() {}, c1 = function() {}
				/* #__NO_SIDE_EFFECTS__ */ var v2 = () => {}, v3 = () => {}
				/* #__NO_SIDE_EFFECTS__ */ let l2 = () => {}, l3 = () => {}
				/* #__NO_SIDE_EFFECTS__ */ const c2 = () => {}, c3 = () => {}
			`,
			"/stmt-export-local.js": `
				/* #__NO_SIDE_EFFECTS__ */ export var v0 = function() {}, v1 = function() {}
				/* #__NO_SIDE_EFFECTS__ */ export let l0 = function() {}, l1 = function() {}
				/* #__NO_SIDE_EFFECTS__ */ export const c0 = function() {}, c1 = function() {}
				/* #__NO_SIDE_EFFECTS__ */ export var v2 = () => {}, v3 = () => {}
				/* #__NO_SIDE_EFFECTS__ */ export let l2 = () => {}, l3 = () => {}
				/* #__NO_SIDE_EFFECTS__ */ export const c2 = () => {}, c3 = () => {}
			`,

			"/ns-export-fn.ts": `
				namespace ns {
					/* @__NO_SIDE_EFFECTS__ */ export function a() {}
					/* @__NO_SIDE_EFFECTS__ */ export function* b() {}
					/* @__NO_SIDE_EFFECTS__ */ export async function c() {}
					/* @__NO_SIDE_EFFECTS__ */ export async function* d() {}
				}
			`,
			"/ns-export-local.ts": `
				namespace ns {
					/* #__NO_SIDE_EFFECTS__ */ export var v0 = function() {}, v1 = function() {}
					/* #__NO_SIDE_EFFECTS__ */ export let l0 = function() {}, l1 = function() {}
					/* #__NO_SIDE_EFFECTS__ */ export const c0 = function() {}, c1 = function() {}
					/* #__NO_SIDE_EFFECTS__ */ export var v2 = () => {}, v3 = () => {}
					/* #__NO_SIDE_EFFECTS__ */ export let l2 = () => {}, l3 = () => {}
					/* #__NO_SIDE_EFFECTS__ */ export const c2 = () => {}, c3 = () => {}
				}
			`,

			"/stmt-export-default-before-fn-anon.js":           `/* #__NO_SIDE_EFFECTS__ */ export default function() {}`,
			"/stmt-export-default-before-fn-name.js":           `/* #__NO_SIDE_EFFECTS__ */ export default function f() {}`,
			"/stmt-export-default-before-gen-fn-anon.js":       `/* #__NO_SIDE_EFFECTS__ */ export default function*() {}`,
			"/stmt-export-default-before-gen-fn-name.js":       `/* #__NO_SIDE_EFFECTS__ */ export default function* f() {}`,
			"/stmt-export-default-before-async-fn-anon.js":     `/* #__NO_SIDE_EFFECTS__ */ export default async function() {}`,
			"/stmt-export-default-before-async-fn-name.js":     `/* #__NO_SIDE_EFFECTS__ */ export default async function f() {}`,
			"/stmt-export-default-before-async-gen-fn-anon.js": `/* #__NO_SIDE_EFFECTS__ */ export default async function*() {}`,
			"/stmt-export-default-before-async-gen-fn-name.js": `/* #__NO_SIDE_EFFECTS__ */ export default async function* f() {}`,

			"/stmt-export-default-after-fn-anon.js":           `export default /* @__NO_SIDE_EFFECTS__ */ function() {}`,
			"/stmt-export-default-after-fn-name.js":           `export default /* @__NO_SIDE_EFFECTS__ */ function f() {}`,
			"/stmt-export-default-after-gen-fn-anon.js":       `export default /* @__NO_SIDE_EFFECTS__ */ function*() {}`,
			"/stmt-export-default-after-gen-fn-name.js":       `export default /* @__NO_SIDE_EFFECTS__ */ function* f() {}`,
			"/stmt-export-default-after-async-fn-anon.js":     `export default /* @__NO_SIDE_EFFECTS__ */ async function() {}`,
			"/stmt-export-default-after-async-fn-name.js":     `export default /* @__NO_SIDE_EFFECTS__ */ async function f() {}`,
			"/stmt-export-default-after-async-gen-fn-anon.js": `export default /* @__NO_SIDE_EFFECTS__ */ async function*() {}`,
			"/stmt-export-default-after-async-gen-fn-name.js": `export default /* @__NO_SIDE_EFFECTS__ */ async function* f() {}`,
		},
		entryPaths: []string{
			"/expr-fn.js",
			"/expr-arrow.js",

			"/stmt-fn.js",
			"/stmt-export-fn.js",
			"/stmt-local.js",
			"/stmt-export-local.js",

			"/ns-export-fn.ts",
			"/ns-export-local.ts",

			"/stmt-export-default-before-fn-anon.js",
			"/stmt-export-default-before-fn-name.js",
			"/stmt-export-default-before-gen-fn-anon.js",
			"/stmt-export-default-before-gen-fn-name.js",
			"/stmt-export-default-before-async-fn-anon.js",
			"/stmt-export-default-before-async-fn-name.js",
			"/stmt-export-default-before-async-gen-fn-anon.js",
			"/stmt-export-default-before-async-gen-fn-name.js",

			"/stmt-export-default-after-fn-anon.js",
			"/stmt-export-default-after-fn-name.js",
			"/stmt-export-default-after-gen-fn-anon.js",
			"/stmt-export-default-after-gen-fn-name.js",
			"/stmt-export-default-after-async-fn-anon.js",
			"/stmt-export-default-after-async-fn-name.js",
			"/stmt-export-default-after-async-gen-fn-anon.js",
			"/stmt-export-default-after-async-gen-fn-name.js",
		},
		options: config.Options{
			AbsOutputDir:     "/out",
			MinifyWhitespace: true,
		},
	})
}

func TestNoSideEffectsCommentUnusedCalls(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/stmt-fn.js": `
				/* @__NO_SIDE_EFFECTS__ */ function f(y) { sideEffect(y) }
				/* @__NO_SIDE_EFFECTS__ */ function* g(y) { sideEffect(y) }
				f('removeThisCall')
				g('removeThisCall')
				f(onlyKeepThisIdentifier)
				g(onlyKeepThisIdentifier)
				x(f('keepThisCall'))
				x(g('keepThisCall'))
			`,
			"/stmt-local.js": `
				/* @__NO_SIDE_EFFECTS__ */ const f = function (y) { sideEffect(y) }
				/* @__NO_SIDE_EFFECTS__ */ const g = function* (y) { sideEffect(y) }
				f('removeThisCall')
				g('removeThisCall')
				f(onlyKeepThisIdentifier)
				g(onlyKeepThisIdentifier)
				x(f('keepThisCall'))
				x(g('keepThisCall'))
			`,
			"/expr-fn.js": `
				const f = /* @__NO_SIDE_EFFECTS__ */ function (y) { sideEffect(y) }
				const g = /* @__NO_SIDE_EFFECTS__ */ function* (y) { sideEffect(y) }
				f('removeThisCall')
				g('removeThisCall')
				f(onlyKeepThisIdentifier)
				g(onlyKeepThisIdentifier)
				x(f('keepThisCall'))
				x(g('keepThisCall'))
			`,
			"/stmt-export-default-fn.js": `
				/* @__NO_SIDE_EFFECTS__ */ export default function f(y) { sideEffect(y) }
				f('removeThisCall')
				f(onlyKeepThisIdentifier)
				x(f('keepThisCall'))
			`,
		},
		entryPaths: []string{
			"/stmt-fn.js",
			"/stmt-local.js",
			"/expr-fn.js",
			"/stmt-export-default-fn.js",
		},
		options: config.Options{
			AbsOutputDir: "/out",
			TreeShaking:  true,
			MinifySyntax: true,
		},
	})
}

func TestNoSideEffectsCommentTypeScriptDeclare(t *testing.T) {
	dce_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				// These should not cause us to crash
				/* @__NO_SIDE_EFFECTS__ */ declare function f1(y) { sideEffect(y) }
				/* @__NO_SIDE_EFFECTS__ */ declare const f2 = function (y) { sideEffect(y) }
				/* @__NO_SIDE_EFFECTS__ */ declare const f3 = (y) => { sideEffect(y) }
				declare const f4 = /* @__NO_SIDE_EFFECTS__ */ function (y) { sideEffect(y) }
				declare const f5 = /* @__NO_SIDE_EFFECTS__ */ (y) => { sideEffect(y) }
				namespace ns {
					/* @__NO_SIDE_EFFECTS__ */ export declare function f1(y) { sideEffect(y) }
					/* @__NO_SIDE_EFFECTS__ */ export declare const f2 = function (y) { sideEffect(y) }
					/* @__NO_SIDE_EFFECTS__ */ export declare const f3 = (y) => { sideEffect(y) }
					export declare const f4 = /* @__NO_SIDE_EFFECTS__ */ function (y) { sideEffect(y) }
					export declare const f5 = /* @__NO_SIDE_EFFECTS__ */ (y) => { sideEffect(y) }
				}
			`,
		},
		entryPaths: []string{
			"/entry.ts",
		},
		options: config.Options{
			AbsOutputDir: "/out",
		},
	})
}
