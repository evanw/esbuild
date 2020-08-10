package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

var default_suite = suite{
	name: "default",
}

func TestSimpleES6(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {fn} from './foo'
				console.log(fn())
			`,
			"/foo.js": `
				export function fn() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestSimpleCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const fn = require('./foo')
				console.log(fn())
			`,
			"/foo.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

// This test makes sure that require() calls are still recognized in nested
// scopes. It guards against bugs where require() calls are only recognized in
// the top-level module scope.
func TestNestedCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function nestedScope() {
					const fn = require('./foo')
					console.log(fn())
				}
				nestedScope()
			`,
			"/foo.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestCommonJSFromES6(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const {foo} = require('./foo')
				console.log(foo(), bar())
				const {bar} = require('./bar') // This should not be hoisted
			`,
			"/foo.js": `
				export function foo() {
					return 'foo'
				}
			`,
			"/bar.js": `
				export function bar() {
					return 'bar'
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestES6FromCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {foo} from './foo'
				console.log(foo(), bar())
				import {bar} from './bar' // This should be hoisted
			`,
			"/foo.js": `
				exports.foo = function() {
					return 'foo'
				}
			`,
			"/bar.js": `
				exports.bar = function() {
					return 'bar'
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

// This test makes sure that ES6 imports are still recognized in nested
// scopes. It guards against bugs where require() calls are only recognized in
// the top-level module scope.
func TestNestedES6FromCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {fn} from './foo'
				(() => {
					console.log(fn())
				})()
			`,
			"/foo.js": `
				exports.fn = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestExportFormsES6(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export default 123
				export var v = 234
				export let l = 234
				export const c = 234
				export {Class as C}
				export function Fn() {}
				export class Class {}
				export * from './a'
				export * as b from './b'
			`,
			"/a.js": "export const abc = undefined",
			"/b.js": "export const xyz = null",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestExportFormsIIFE(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export default 123
				export var v = 234
				export let l = 234
				export const c = 234
				export {Class as C}
				export function Fn() {}
				export class Class {}
				export * from './a'
				export * as b from './b'
			`,
			"/a.js": "export const abc = undefined",
			"/b.js": "export const xyz = null",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			OutputFormat:  config.FormatIIFE,
			ModuleName:    "moduleName",
			AbsOutputFile: "/out.js",
		},
	})
}

func TestExportFormsWithMinifyIdentifiersAndNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				export default 123
				export var varName = 234
				export let letName = 234
				export const constName = 234
				function Func2() {}
				class Class2 {}
				export {Class as Cls, Func2 as Fn2, Class2 as Cls2}
				export function Func() {}
				export class Class {}
				export * from './a'
				export * as fromB from './b'
			`,
			"/b.js": "export default function() {}",
			"/c.js": "export default function foo() {}",
			"/d.js": "export default class {}",
			"/e.js": "export default class Foo {}",
		},
		entryPaths: []string{
			"/a.js",
			"/b.js",
			"/c.js",
			"/d.js",
			"/e.js",
		},
		options: config.Options{
			IsBundling:        false,
			MinifyIdentifiers: true,
			AbsOutputDir:      "/out",
		},
	})
}

func TestImportFormsWithNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import 'foo'
				import {} from 'foo'
				import * as ns from 'foo'
				import {a, b as c} from 'foo'
				import def from 'foo'
				import def2, * as ns2 from 'foo'
				import def3, {a2, b as c3} from 'foo'
				const imp = [
					import('foo'),
					function nested() { return import('foo') },
				]
				console.log(ns, a, c, def, def2, ns2, def3, a2, c3, imp)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestImportFormsWithMinifyIdentifiersAndNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import 'foo'
				import {} from 'foo'
				import * as ns from 'foo'
				import {a, b as c} from 'foo'
				import def from 'foo'
				import def2, * as ns2 from 'foo'
				import def3, {a2, b as c3} from 'foo'
				const imp = [
					import('foo'),
					function() { return import('foo') },
				]
				console.log(ns, a, c, def, def2, ns2, def3, a2, c3, imp)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:        false,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
	})
}

func TestExportFormsCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				require('./commonjs')
				require('./c')
				require('./d')
				require('./e')
				require('./f')
				require('./g')
				require('./h')
			`,
			"/commonjs.js": `
				export default 123
				export var v = 234
				export let l = 234
				export const c = 234
				export {Class as C}
				export function Fn() {}
				export class Class {}
				export * from './a'
				export * as b from './b'
			`,
			"/a.js": "export const abc = undefined",
			"/b.js": "export const xyz = null",
			"/c.js": "export default class {}",
			"/d.js": "export default class Foo {} Foo.prop = 123",
			"/e.js": "export default function() {}",
			"/f.js": "export default function foo() {} foo.prop = 123",
			"/g.js": "export default async function() {}",
			"/h.js": "export default async function foo() {} foo.prop = 123",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestReExportDefaultCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {foo as entry} from './foo'
				entry()
			`,
			"/foo.js": `
				export {default as foo} from './bar'
			`,
			"/bar.js": `
				export default function foo() {
					return exports // Force this to be a CommonJS module
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestExportChain(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export {b as a} from './foo'
			`,
			"/foo.js": `
				export {c as b} from './bar'
			`,
			"/bar.js": `
				export const c = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestExportInfiniteCycle1(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export {a as b} from './entry'
				export {b as c} from './entry'
				export {c as d} from './entry'
				export {d as a} from './entry'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedCompileLog: `/entry.js: error: Detected cycle while resolving import "a"
/entry.js: error: Detected cycle while resolving import "b"
/entry.js: error: Detected cycle while resolving import "c"
/entry.js: error: Detected cycle while resolving import "d"
`,
	})
}

func TestExportInfiniteCycle2(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export {a as b} from './foo'
				export {c as d} from './foo'
			`,
			"/foo.js": `
				export {b as c} from './entry'
				export {d as a} from './entry'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedCompileLog: `/entry.js: error: Detected cycle while resolving import "a"
/entry.js: error: Detected cycle while resolving import "c"
/foo.js: error: Detected cycle while resolving import "b"
/foo.js: error: Detected cycle while resolving import "d"
`,
	})
}

func TestJSXImportsCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.jsx": `
				import {elem, frag} from './custom-react'
				console.log(<div/>, <>fragment</>)
			`,
			"/custom-react.js": `
				module.exports = {}
			`,
		},
		entryPaths: []string{"/entry.jsx"},
		options: config.Options{
			IsBundling: true,
			JSX: config.JSXOptions{
				Factory:  []string{"elem"},
				Fragment: []string{"frag"},
			},
			AbsOutputFile: "/out.js",
		},
	})
}

func TestJSXImportsES6(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.jsx": `
				import {elem, frag} from './custom-react'
				console.log(<div/>, <>fragment</>)
			`,
			"/custom-react.js": `
				export function elem() {}
				export function frag() {}
			`,
		},
		entryPaths: []string{"/entry.jsx"},
		options: config.Options{
			IsBundling: true,
			JSX: config.JSXOptions{
				Factory:  []string{"elem"},
				Fragment: []string{"frag"},
			},
			AbsOutputFile: "/out.js",
		},
	})
}

func TestJSXSyntaxInJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(<div/>)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: error: Unexpected "<"
`,
	})
}

func TestNodeModules(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonMain(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./custom-main.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/custom-main.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonSyntaxErrorComment(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					// Single-line comment
					"a": 1
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: "/Users/user/project/node_modules/demo-pkg/package.json: error: JSON does not support comments\n",
	})
}

func TestPackageJsonSyntaxErrorTrailingComma(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"a": 1,
					"b": 2,
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: "/Users/user/project/node_modules/demo-pkg/package.json: error: JSON does not support trailing commas\n",
	})
}

func TestPackageJsonModule(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main.js",
					"module": "./main.esm.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.esm.js": `
				export default function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserString(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": "./browser"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/browser.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserMapRelativeToRelative(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main",
					"browser": {
						"./main.js": "./main-browser",
						"./lib/util.js": "./lib/util-browser"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				const util = require('./lib/util')
				module.exports = function() {
					return ['main', util]
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main-browser.js": `
				const util = require('./lib/util')
				module.exports = function() {
					return ['main-browser', util]
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/lib/util.js": `
				module.exports = 'util'
			`,
			"/Users/user/project/node_modules/demo-pkg/lib/util-browser.js": `
				module.exports = 'util-browser'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserMapRelativeToModule(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main",
					"browser": {
						"./util.js": "util-browser"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				const util = require('./util')
				module.exports = function() {
					return ['main', util]
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/util.js": `
				module.exports = 'util'
			`,
			"/Users/user/project/node_modules/util-browser/index.js": `
				module.exports = 'util-browser'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserMapRelativeDisabled(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main",
					"browser": {
						"./util-node.js": false
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				const util = require('./util-node')
				module.exports = function(obj) {
					return util.inspect(obj)
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/util-node.js": `
				module.exports = require('util')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserMapModuleToRelative(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": {
						"node-pkg": "./node-pkg-browser"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/node-pkg-browser.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				const fn = require('node-pkg')
				module.exports = function() {
					return fn()
				}
			`,
			"/Users/user/project/node_modules/node-pkg/index.js": `
				module.exports = function() {
					return 234
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserMapModuleToModule(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": {
						"node-pkg": "node-pkg-browser"
					}
				}
			`,
			"/Users/user/project/node_modules/node-pkg-browser/index.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				const fn = require('node-pkg')
				module.exports = function() {
					return fn()
				}
			`,
			"/Users/user/project/node_modules/node-pkg/index.js": `
				module.exports = function() {
					return 234
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserMapModuleDisabled(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": {
						"node-pkg": false
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				const fn = require('node-pkg')
				module.exports = function() {
					return fn()
				}
			`,
			"/Users/user/project/node_modules/node-pkg/index.js": `
				module.exports = function() {
					return 234
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserMapNativeModuleDisabled(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": {
						"fs": false
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				const fs = require('fs')
				module.exports = function() {
					return fs.readFile()
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserMapAvoidMissing(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'component-classes'
			`,
			"/Users/user/project/node_modules/component-classes/package.json": `
				{
					"browser": {
						"indexof": "component-indexof"
					}
				}
			`,
			"/Users/user/project/node_modules/component-classes/index.js": `
				try {
					var index = require('indexof');
				} catch (err) {
					var index = require('component-indexof');
				}
			`,
			"/Users/user/project/node_modules/component-indexof/index.js": `
				module.exports = function() {
					return 234
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserOverModule(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main.js",
					"module": "./main.esm.js",
					"browser": "./main.browser.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.esm.js": `
				export default function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.browser.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserWithModule(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main.js",
					"module": "./main.esm.js",
					"browser": {
						"./main.js": "./main.browser.js",
						"./main.esm.js": "./main.browser.esm.js"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.esm.js": `
				export default function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.browser.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.browser.esm.js": `
				export default function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserFromParent(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/package.json": `
				{
					"browser": {
						"https": "https-browserify"
					}
				}
			`,
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/https-browserify/index.js": `
				module.exports = {
					get: function() { return false; }
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"name": "demo-pkg",
					"module": "./index.js",
					"browser": {
						"fs": false
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				const https = require('https')
				const fs = require('fs');
				module.exports = function(url, cb) {
					return https.get(url, cb);
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestRequireChildDirCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				console.log(require('./dir'))
			`,
			"/Users/user/project/src/dir/index.js": `
				module.exports = 123
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestRequireChildDirES6(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import value from './dir'
				console.log(value)
			`,
			"/Users/user/project/src/dir/index.js": `
				export default 123
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestRequireParentDirCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/dir/entry.js": `
				console.log(require('..'))
			`,
			"/Users/user/project/src/index.js": `
				module.exports = 123
			`,
		},
		entryPaths: []string{"/Users/user/project/src/dir/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestRequireParentDirES6(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/dir/entry.js": `
				import value from '..'
				console.log(value)
			`,
			"/Users/user/project/src/index.js": `
				export default 123
			`,
		},
		entryPaths: []string{"/Users/user/project/src/dir/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestImportMissingES6(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import fn, {x as a, y as b} from './foo'
				console.log(fn(a, b))
			`,
			"/foo.js": `
				export const x = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedCompileLog: `/entry.js: error: No matching export for import "default"
/entry.js: error: No matching export for import "y"
`,
	})
}

func TestImportMissingUnusedES6(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import fn, {x as a, y as b} from './foo'
			`,
			"/foo.js": `
				export const x = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedCompileLog: `/entry.js: error: No matching export for import "default"
/entry.js: error: No matching export for import "y"
`,
	})
}

func TestImportMissingCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import fn, {x as a, y as b} from './foo'
				console.log(fn(a, b))
			`,
			"/foo.js": `
				exports.x = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestImportMissingNeitherES6NorCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import fn, {x as a, y as b} from './foo'
				import * as ns from './foo'
				console.log(fn(a, b))
			`,
			"/foo.js": `
				console.log('no exports here')
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedCompileLog: `/entry.js: warning: Import "default" will always be undefined
/entry.js: warning: Import "x" will always be undefined
/entry.js: warning: Import "y" will always be undefined
`,
	})
}

func TestExportMissingES6(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				console.log(ns)
			`,
			"/foo.js": `
				export {nope} from './bar'
			`,
			"/bar.js": `
				export const yep = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedCompileLog: `/foo.js: error: No matching export for import "nope"
`,
	})
}

func TestDotImport(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {x} from '.'
				console.log(x)
			`,
			"/index.js": `
				exports.x = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestRequireWithTemplate(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				console.log(require('./b'))
				console.log(require(` + "`./b`" + `))
			`,
			"/b.js": `
				exports.x = 123
			`,
		},
		entryPaths: []string{"/a.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestDynamicImportWithTemplateIIFE(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import('./b').then(ns => console.log(ns))
				import(` + "`./b`" + `).then(ns => console.log(ns))
			`,
			"/b.js": `
				exports.x = 123
			`,
		},
		entryPaths: []string{"/a.js"},
		options: config.Options{
			IsBundling:    true,
			OutputFormat:  config.FormatIIFE,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestRequireAndDynamicImportInvalidTemplate(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				require(tag` + "`./b`" + `)
				require(` + "`./${b}`" + `)
				import(tag` + "`./b`" + `)
				import(` + "`./${b}`" + `)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: warning: This call to "require" will not be bundled because the argument is not a string literal
/entry.js: warning: This call to "require" will not be bundled because the argument is not a string literal
/entry.js: warning: This dynamic import will not be bundled because the argument is not a string literal
/entry.js: warning: This dynamic import will not be bundled because the argument is not a string literal
`,
	})
}

func TestRequireBadArgumentCount(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				require()
				require("a", "b")
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: warning: This call to "require" will not be bundled because it has 0 arguments
/entry.js: warning: This call to "require" will not be bundled because it has 2 arguments
`,
	})
}

func TestRequireJson(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.json'))
			`,
			"/test.json": `
				{
					"a": true,
					"b": 123,
					"c": [null]
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestRequireTxt(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.txt'))
			`,
			"/test.txt": `This is a test.`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestRequireBadExtension(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test'))
			`,
			"/test": `This is a test.`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: error: File extension not supported: /test
`,
	})
}

func TestFalseRequire(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				(require => require('/test.txt'))()
			`,
			"/test.txt": `This is a test.`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestRequireWithoutCall(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const req = require
				req('./entry')
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: warning: Indirect calls to "require" will not be bundled
`,
	})
}

func TestNestedRequireWithoutCall(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				(() => {
					const req = require
					req('./entry')
				})()
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: warning: Indirect calls to "require" will not be bundled
`,
	})
}

// Test a workaround for the "debug" library
func TestRequireWithCallInsideTry(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				try {
					const supportsColor = require('supports-color');
					if (supportsColor && (supportsColor.stderr || supportsColor).level >= 2) {
						exports.colors = [];
					}
				} catch (error) {
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

// Test a workaround for the "moment" library
func TestRequireWithoutCallInsideTry(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				try {
					oldLocale = globalLocale._abbr;
					var aliasedRequire = require;
					aliasedRequire('./locale/' + name);
					getSetGlobalLocale(oldLocale);
				} catch (e) {}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestSourceMap(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {bar} from './bar'
				function foo() { bar() }
				foo()
			`,
			"/Users/user/project/src/bar.js": `
				export function bar() { throw new Error('test') }
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			IsBundling:    true,
			SourceMap:     config.SourceMapLinkedWithComment,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

// This test covers a bug where a "var" in a nested scope did not correctly
// bind with references to that symbol in sibling scopes. Instead, the
// references were incorrectly considered to be unbound even though the symbol
// should be hoisted. This caused the renamer to name them different things to
// avoid a collision, which changed the meaning of the code.
func TestNestedScopeBug(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				(() => {
					function a() {
						b()
					}
					{
						var b = () => {}
					}
					a()
				})()
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestHashbangBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `#!/usr/bin/env a
				import {code} from './code'
				process.exit(code)
			`,
			"/code.js": `#!/usr/bin/env b
				export const code = 0
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestHashbangNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `#!/usr/bin/env node
				process.exit(0);
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTypeofRequireBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log([
					typeof require,
					typeof require == 'function',
					typeof require == 'function' && require,
					'function' == typeof require,
					'function' == typeof require && require,
				]);
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTypeofRequireNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log([
					typeof require,
					typeof require == 'function',
					typeof require == 'function' && require,
					'function' == typeof require,
					'function' == typeof require && require,
				]);
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTypeofRequireBadPatterns(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log([
					typeof require != 'function' && require,
					typeof require === 'function' && require,
					typeof require == 'function' || require,
					typeof require == 'function' && notRequire,
					typeof notRequire == 'function' && require,

					'function' != typeof require && require,
					'function' === typeof require && require,
					'function' == typeof require || require,
					'function' == typeof require && notRequire,
					'function' == typeof notRequire && require,
				]);
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: warning: Indirect calls to "require" will not be bundled
/entry.js: warning: Indirect calls to "require" will not be bundled
/entry.js: warning: Indirect calls to "require" will not be bundled
/entry.js: warning: Indirect calls to "require" will not be bundled
/entry.js: warning: Indirect calls to "require" will not be bundled
/entry.js: warning: Indirect calls to "require" will not be bundled
/entry.js: warning: Indirect calls to "require" will not be bundled
/entry.js: warning: Indirect calls to "require" will not be bundled
`,
	})
}

func TestRequireFSBrowser(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('fs'))
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			Platform:      config.PlatformBrowser,
		},
		expectedScanLog: "/entry.js: error: Could not resolve \"fs\"\n",
	})
}

func TestRequireFSNode(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				return require('fs')
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			OutputFormat:  config.FormatCommonJS,
			AbsOutputFile: "/out.js",
			Platform:      config.PlatformNode,
		},
	})
}

func TestRequireFSNodeMinify(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				return require('fs')
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:       true,
			RemoveWhitespace: true,
			OutputFormat:     config.FormatCommonJS,
			AbsOutputFile:    "/out.js",
			Platform:         config.PlatformNode,
		},
	})
}

func TestImportFSBrowser(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import 'fs'
				import * as fs from 'fs'
				import defaultValue from 'fs'
				import {readFileSync} from 'fs'
				console.log(fs, readFileSync, defaultValue)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			Platform:      config.PlatformBrowser,
		},
		expectedScanLog: `/entry.js: error: Could not resolve "fs"
`,
	})
}

func TestImportFSNodeCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import 'fs'
				import * as fs from 'fs'
				import defaultValue from 'fs'
				import {readFileSync} from 'fs'
				console.log(fs, readFileSync, defaultValue)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			OutputFormat:  config.FormatCommonJS,
			AbsOutputFile: "/out.js",
			Platform:      config.PlatformNode,
		},
	})
}

func TestImportFSNodeES6(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import 'fs'
				import * as fs from 'fs'
				import defaultValue from 'fs'
				import {readFileSync} from 'fs'
				console.log(fs, readFileSync, defaultValue)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/out.js",
			Platform:      config.PlatformNode,
		},
	})
}

func TestExportFSBrowser(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export * as fs from 'fs'
				export {readFileSync} from 'fs'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			Platform:      config.PlatformBrowser,
		},
		expectedScanLog: `/entry.js: error: Could not resolve "fs"
`,
	})
}

func TestExportFSNode(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export * as fs from 'fs'
				export {readFileSync} from 'fs'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			Platform:      config.PlatformNode,
		},
	})
}

func TestReExportFSNode(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export {fs as f} from './foo'
				export {readFileSync as rfs} from './foo'
			`,
			"/foo.js": `
				export * as fs from 'fs'
				export {readFileSync} from 'fs'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			Platform:      config.PlatformNode,
		},
	})
}

func TestExportFSNodeInCommonJSModule(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export * as fs from 'fs'
				export {readFileSync} from 'fs'

				// Force this to be a CommonJS module
				exports.foo = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			Platform:      config.PlatformNode,
		},
	})
}

func TestExportWildcardFSNodeES6(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export * from 'fs'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/out.js",
			Platform:      config.PlatformNode,
		},
	})
}

func TestExportWildcardFSNodeCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export * from 'fs'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			OutputFormat:  config.FormatCommonJS,
			AbsOutputFile: "/out.js",
			Platform:      config.PlatformNode,
		},
	})
}

func TestMinifiedBundleES6(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {foo} from './a'
				console.log(foo())
			`,
			"/a.js": `
				export function foo() {
					return 123
				}
				foo()
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:        true,
			MangleSyntax:      true,
			RemoveWhitespace:  true,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
	})
}

func TestMinifiedBundleCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const {foo} = require('./a')
				console.log(foo(), require('./j.json'))
			`,
			"/a.js": `
				exports.foo = function() {
					return 123
				}
			`,
			"/j.json": `
				{"test": true}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:        true,
			MangleSyntax:      true,
			RemoveWhitespace:  true,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
	})
}

func TestMinifiedBundleEndingWithImportantSemicolon(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				while(foo()); // This semicolon must not be stripped
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:       true,
			RemoveWhitespace: true,
			OutputFormat:     config.FormatIIFE,
			AbsOutputFile:    "/out.js",
		},
	})
}

func TestRuntimeNameCollisionNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function __require() { return 123 }
				console.log(__require())
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTopLevelReturn(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {foo} from './foo'
				foo()
			`,
			"/foo.js": `
				// Top-level return must force CommonJS mode
				if (Math.random() < 0.5) return

				export function foo() {}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestThisOutsideFunction(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(this)
				console.log((x = this) => this)
				console.log({x: this})
				console.log(class extends this.foo {})
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestThisInsideFunction(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function foo(x = this) { console.log(this) }
				const obj = {
					foo(x = this) { console.log(this) }
				}
				class Foo {
					x = this
					static y = this.z
					foo(x = this) { console.log(this) }
					static bar(x = this) { console.log(this) }
				}
				new Foo(foo(obj))
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

// The value of "this" is "exports" in CommonJS modules and undefined in ES6
// modules. This is determined by the presence of ES6 import/export syntax.
func TestThisWithES6Syntax(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import './cjs'

				import './es6-import-stmt'
				import './es6-import-assign'
				import './es6-import-dynamic'
				import './es6-import-meta'
				import './es6-expr-import-dynamic'
				import './es6-expr-import-meta'

				import './es6-export-variable'
				import './es6-export-function'
				import './es6-export-async-function'
				import './es6-export-enum'
				import './es6-export-const-enum'
				import './es6-export-module'
				import './es6-export-namespace'
				import './es6-export-class'
				import './es6-export-abstract-class'
				import './es6-export-default'
				import './es6-export-clause'
				import './es6-export-clause-from'
				import './es6-export-star'
				import './es6-export-star-as'
				import './es6-export-assign'
				import './es6-export-import-assign'

				import './es6-ns-export-variable'
				import './es6-ns-export-function'
				import './es6-ns-export-async-function'
				import './es6-ns-export-enum'
				import './es6-ns-export-const-enum'
				import './es6-ns-export-module'
				import './es6-ns-export-namespace'
				import './es6-ns-export-class'
				import './es6-ns-export-abstract-class'
				`,
			"/dummy.js": `export const dummy = 123`,
			"/cjs.js":   `console.log(this)`,

			"/es6-import-stmt.js":         `import './dummy'; console.log(this)`,
			"/es6-import-assign.ts":       `import x = require('./dummy'); console.log(this)`,
			"/es6-import-dynamic.js":      `import('./dummy'); console.log(this)`,
			"/es6-import-meta.js":         `import.meta; console.log(this)`,
			"/es6-expr-import-dynamic.js": `(import('./dummy')); console.log(this)`,
			"/es6-expr-import-meta.js":    `(import.meta); console.log(this)`,

			"/es6-export-variable.js":       `export const foo = 123; console.log(this)`,
			"/es6-export-function.js":       `export function foo() {} console.log(this)`,
			"/es6-export-async-function.js": `export async function foo() {} console.log(this)`,
			"/es6-export-enum.ts":           `export enum Foo {} console.log(this)`,
			"/es6-export-const-enum.ts":     `export const enum Foo {} console.log(this)`,
			"/es6-export-module.ts":         `export module Foo {} console.log(this)`,
			"/es6-export-namespace.ts":      `export namespace Foo {} console.log(this)`,
			"/es6-export-class.js":          `export class Foo {} console.log(this)`,
			"/es6-export-abstract-class.ts": `export abstract class Foo {} console.log(this)`,
			"/es6-export-default.js":        `export default 123; console.log(this)`,
			"/es6-export-clause.js":         `export {}; console.log(this)`,
			"/es6-export-clause-from.js":    `export {} from './dummy'; console.log(this)`,
			"/es6-export-star.js":           `export * from './dummy'; console.log(this)`,
			"/es6-export-star-as.js":        `export * as ns from './dummy'; console.log(this)`,
			"/es6-export-assign.ts":         `export = 123; console.log(this)`,
			"/es6-export-import-assign.ts":  `export import x = require('./dummy'); console.log(this)`,

			"/es6-ns-export-variable.ts":       `namespace ns { export const foo = 123; } console.log(this)`,
			"/es6-ns-export-function.ts":       `namespace ns { export function foo() {} } console.log(this)`,
			"/es6-ns-export-async-function.ts": `namespace ns { export async function foo() {} } console.log(this)`,
			"/es6-ns-export-enum.ts":           `namespace ns { export enum Foo {} } console.log(this)`,
			"/es6-ns-export-const-enum.ts":     `namespace ns { export const enum Foo {} } console.log(this)`,
			"/es6-ns-export-module.ts":         `namespace ns { export module Foo {} } console.log(this)`,
			"/es6-ns-export-namespace.ts":      `namespace ns { export namespace Foo {} } console.log(this)`,
			"/es6-ns-export-class.ts":          `namespace ns { export class Foo {} } console.log(this)`,
			"/es6-ns-export-abstract-class.ts": `namespace ns { export abstract class Foo {} } console.log(this)`,
		},
		entryPaths: []string{
			"/entry.js",
		},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestArrowFnScope(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				tests = {
					0: ((x = y => x + y, y) => x + y),
					1: ((y, x = y => x + y) => x + y),
					2: ((x = (y = z => x + y + z, z) => x + y + z, y, z) => x + y + z),
					3: ((y, z, x = (z, y = z => x + y + z) => x + y + z) => x + y + z),
					4: ((x = y => x + y, y), x + y),
					5: ((y, x = y => x + y), x + y),
					6: ((x = (y = z => x + y + z, z) => x + y + z, y, z), x + y + z),
					7: ((y, z, x = (z, y = z => x + y + z) => x + y + z), x + y + z),
				};
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:        true,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
	})
}

func TestSwitchScopeNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				switch (foo) { default: var foo }
				switch (bar) { default: let bar }
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:        false,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
	})
}

func TestArgumentDefaultValueScopeNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export function a(x = foo) { var foo; return x }
				export class b { fn(x = foo) { var foo; return x } }
				export let c = [
					function(x = foo) { var foo; return x },
					(x = foo) => { var foo; return x },
					{ fn(x = foo) { var foo; return x }},
					class { fn(x = foo) { var foo; return x }},
				]
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:        false,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
	})
}

func TestArgumentsSpecialCaseNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				(() => {
					var arguments;

					function foo(x = arguments) { return arguments }
					(function(x = arguments) { return arguments });
					({foo(x = arguments) { return arguments }});
					class Foo { foo(x = arguments) { return arguments } }
					(class { foo(x = arguments) { return arguments } });

					function foo(x = arguments) { var arguments; return arguments }
					(function(x = arguments) { var arguments; return arguments });
					({foo(x = arguments) { var arguments; return arguments }});
					class Foo2 { foo(x = arguments) { var arguments; return arguments } }
					(class { foo(x = arguments) { var arguments; return arguments } });

					(x => arguments);
					(() => arguments);
					(async () => arguments);
					((x = arguments) => arguments);
					(async (x = arguments) => arguments);

					x => arguments;
					() => arguments;
					async () => arguments;
					(x = arguments) => arguments;
					async (x = arguments) => arguments;

					(x => { return arguments });
					(() => { return arguments });
					(async () => { return arguments });
					((x = arguments) => { return arguments });
					(async (x = arguments) => { return arguments });

					x => { return arguments };
					() => { return arguments };
					async () => { return arguments };
					(x = arguments) => { return arguments };
					async (x = arguments) => { return arguments };
				})()
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:        false,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
	})
}

func TestWithStatementTaintingNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				(() => {
					let local = 1
					let outer = 2
					let outerDead = 3
					with ({}) {
						var hoisted = 4
						let local = 5
						hoisted++
						local++
						if (1) outer++
						if (0) outerDead++
					}
					if (1) {
						hoisted++
						local++
						outer++
						outerDead++
					}
				})()
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:        false,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
	})
}

func TestDirectEvalTaintingNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function test1() {
					function add(first, second) {
						return first + second
					}
					eval('add(1, 2)')
				}

				function test2() {
					function add(first, second) {
						return first + second
					}
					(0, eval)('add(1, 2)')
				}

				function test3() {
					function add(first, second) {
						return first + second
					}
				}

				function test4(eval) {
					function add(first, second) {
						return first + second
					}
					eval('add(1, 2)')
				}

				function test5() {
					function containsDirectEval() { eval() }
					if (true) { var shouldNotBeRenamed }
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:        false,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
	})
}

func TestImportReExportES6Issue149(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/app.jsx": `
				import { p as Part, h, render } from './import';
				import { Internal } from './in2';
				const App = () => <Part> <Internal /> T </Part>;
				render(<App />, document.getElementById('app'));
			`,
			"/in2.jsx": `
				import { p as Part, h } from './import';
				export const Internal = () => <Part> Test 2 </Part>;
			`,
			"/import.js": `
				import { h, render } from 'preact';
				export const p = "p";
				export { h, render }
			`,
		},
		entryPaths: []string{"/app.jsx"},
		options: config.Options{
			IsBundling: true,
			JSX: config.JSXOptions{
				Factory: []string{"h"},
			},
			AbsOutputFile: "/out.js",
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"preact": true,
				},
			},
		},
	})
}

func TestExternalModuleExclusionPackage(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/index.js": `
				import { S3 } from 'aws-sdk';
				import { DocumentClient } from 'aws-sdk/clients/dynamodb';
				export const s3 = new S3();
				export const dynamodb = new DocumentClient();
			`,
		},
		entryPaths: []string{"/index.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"aws-sdk": true,
				},
			},
		},
	})
}

func TestExternalModuleExclusionScopedPackage(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/index.js": `
				import '@a1'
				import '@a1/a2'
				import '@a1-a2'

				import '@b1'
				import '@b1/b2'
				import '@b1/b2/b3'
				import '@b1/b2-b3'

				import '@c1'
				import '@c1/c2'
				import '@c1/c2/c3'
				import '@c1/c2/c3/c4'
				import '@c1/c2/c3-c4'
			`,
		},
		entryPaths: []string{"/index.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"@a1":       true,
					"@b1/b2":    true,
					"@c1/c2/c3": true,
				},
			},
		},
		expectedScanLog: `/index.js: error: Could not resolve "@a1-a2"
/index.js: error: Could not resolve "@b1"
/index.js: error: Could not resolve "@b1/b2-b3"
/index.js: error: Could not resolve "@c1"
/index.js: error: Could not resolve "@c1/c2"
/index.js: error: Could not resolve "@c1/c2/c3-c4"
`,
	})
}

func TestScopedExternalModuleExclusion(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/index.js": `
				import { Foo } from '@scope/foo';
				import { Bar } from '@scope/foo/bar';
				export const foo = new Foo();
				export const bar = new Bar();
			`,
		},
		entryPaths: []string{"/index.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"@scope/foo": true,
				},
			},
		},
	})
}

func TestExternalModuleExclusionRelativePath(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/index.js": `
				import './nested/folder/test'
			`,
			"/Users/user/project/src/nested/folder/test.js": `
				import foo from './foo.js'
				import sha256 from '../../sha256.min.js'
				import config from '/api/config?a=1&b=2'
				console.log(foo, sha256, config)
			`,
		},
		entryPaths: []string{"/Users/user/project/src/index.js"},
		options: config.Options{
			IsBundling:   true,
			AbsOutputDir: "/Users/user/project/out",
			ExternalModules: config.ExternalModules{
				AbsPaths: map[string]bool{
					"/Users/user/project/src/nested/folder/foo.js": true,
					"/Users/user/project/src/sha256.min.js":        true,
					"/api/config?a=1&b=2":                          true,
				},
			},
		},
	})
}

// This test case makes sure many entry points don't cause a crash
func TestManyEntryPoints(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/shared.js": `export default 123`,

			"/e00.js": `import x from './shared'; console.log(x)`,
			"/e01.js": `import x from './shared'; console.log(x)`,
			"/e02.js": `import x from './shared'; console.log(x)`,
			"/e03.js": `import x from './shared'; console.log(x)`,
			"/e04.js": `import x from './shared'; console.log(x)`,
			"/e05.js": `import x from './shared'; console.log(x)`,
			"/e06.js": `import x from './shared'; console.log(x)`,
			"/e07.js": `import x from './shared'; console.log(x)`,
			"/e08.js": `import x from './shared'; console.log(x)`,
			"/e09.js": `import x from './shared'; console.log(x)`,

			"/e10.js": `import x from './shared'; console.log(x)`,
			"/e11.js": `import x from './shared'; console.log(x)`,
			"/e12.js": `import x from './shared'; console.log(x)`,
			"/e13.js": `import x from './shared'; console.log(x)`,
			"/e14.js": `import x from './shared'; console.log(x)`,
			"/e15.js": `import x from './shared'; console.log(x)`,
			"/e16.js": `import x from './shared'; console.log(x)`,
			"/e17.js": `import x from './shared'; console.log(x)`,
			"/e18.js": `import x from './shared'; console.log(x)`,
			"/e19.js": `import x from './shared'; console.log(x)`,

			"/e20.js": `import x from './shared'; console.log(x)`,
			"/e21.js": `import x from './shared'; console.log(x)`,
			"/e22.js": `import x from './shared'; console.log(x)`,
			"/e23.js": `import x from './shared'; console.log(x)`,
			"/e24.js": `import x from './shared'; console.log(x)`,
			"/e25.js": `import x from './shared'; console.log(x)`,
			"/e26.js": `import x from './shared'; console.log(x)`,
			"/e27.js": `import x from './shared'; console.log(x)`,
			"/e28.js": `import x from './shared'; console.log(x)`,
			"/e29.js": `import x from './shared'; console.log(x)`,

			"/e30.js": `import x from './shared'; console.log(x)`,
			"/e31.js": `import x from './shared'; console.log(x)`,
			"/e32.js": `import x from './shared'; console.log(x)`,
			"/e33.js": `import x from './shared'; console.log(x)`,
			"/e34.js": `import x from './shared'; console.log(x)`,
			"/e35.js": `import x from './shared'; console.log(x)`,
			"/e36.js": `import x from './shared'; console.log(x)`,
			"/e37.js": `import x from './shared'; console.log(x)`,
			"/e38.js": `import x from './shared'; console.log(x)`,
			"/e39.js": `import x from './shared'; console.log(x)`,
		},
		entryPaths: []string{
			"/e00.js", "/e01.js", "/e02.js", "/e03.js", "/e04.js", "/e05.js", "/e06.js", "/e07.js", "/e08.js", "/e09.js",
			"/e10.js", "/e11.js", "/e12.js", "/e13.js", "/e14.js", "/e15.js", "/e16.js", "/e17.js", "/e18.js", "/e19.js",
			"/e20.js", "/e21.js", "/e22.js", "/e23.js", "/e24.js", "/e25.js", "/e26.js", "/e27.js", "/e28.js", "/e29.js",
			"/e30.js", "/e31.js", "/e32.js", "/e33.js", "/e34.js", "/e35.js", "/e36.js", "/e37.js", "/e38.js", "/e39.js",
		},
		options: config.Options{
			IsBundling:   true,
			AbsOutputDir: "/out",
		},
	})
}

func TestRenamePrivateIdentifiersNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#foo
					foo = class {
						#foo
						#foo2
						#bar
					}
					get #bar() {}
					set #bar() {}
				}
				class Bar {
					#foo
					foo = class {
						#foo2
						#foo
						#bar
					}
					get #bar() {}
					set #bar() {}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestMinifyPrivateIdentifiersNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#foo
					foo = class {
						#foo
						#foo2
						#bar
					}
					get #bar() {}
					set #bar() {}
				}
				class Bar {
					#foo
					foo = class {
						#foo2
						#foo
						#bar
					}
					get #bar() {}
					set #bar() {}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:        false,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
	})
}

func TestExportsAndModuleFormatCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as foo from './foo/test'
				import * as bar from './bar/test'
				console.log(exports, module.exports, foo, bar)
			`,
			"/foo/test.js": `
				export let foo = 123
			`,
			"/bar/test.js": `
				export let bar = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			OutputFormat:  config.FormatCommonJS,
			AbsOutputFile: "/out.js",
			Platform:      config.PlatformNode,
		},

		// The "test_exports" names must be different
	})
}

func TestMinifiedExportsAndModuleFormatCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as foo from './foo/test'
				import * as bar from './bar/test'
				console.log(exports, module.exports, foo, bar)
			`,
			"/foo/test.js": `
				export let foo = 123
			`,
			"/bar/test.js": `
				export let bar = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:        true,
			MinifyIdentifiers: true,
			OutputFormat:      config.FormatCommonJS,
			AbsOutputFile:     "/out.js",
			Platform:          config.PlatformNode,
		},

		// The "test_exports" names must be minified, and the "exports" and
		// "module" names must not be minified
	})
}

// The minifier should not remove "use strict" or join it with other expressions
func TestUseStrictDirectiveMinifyNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				'use strict'
				'use loose'
				a
				b
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:       false,
			MangleSyntax:     true,
			RemoveWhitespace: true,
			AbsOutputFile:    "/out.js",
		},
	})
}

func TestNoOverwriteInputFileError(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(123)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:   true,
			AbsOutputDir: "/",
		},
		expectedCompileLog: "error: Refusing to overwrite input file: /entry.js\n",
	})
}

func TestDuplicateEntryPointError(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(123)
			`,
		},
		entryPaths: []string{"/entry.js", "/entry.js"},
		options: config.Options{
			IsBundling:   true,
			AbsOutputDir: "/out.js",
		},
		expectedScanLog: "error: Duplicate entry point: /entry.js\n",
	})
}

func TestMultipleEntryPointsSameNameCollision(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a/entry.js": `import {foo} from '../common.js'; console.log(foo)`,
			"/b/entry.js": `import {foo} from '../common.js'; console.log(foo)`,
			"/common.js":  `export let foo = 123`,
		},
		entryPaths: []string{"/a/entry.js", "/b/entry.js"},
		options: config.Options{
			IsBundling:   true,
			AbsOutputDir: "/out/",
		},
	})
}

func TestReExportCommonJSAsES6(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export {bar} from './foo'
			`,
			"/foo.js": `
				exports.bar = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestReExportDefaultInternal(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export {default as foo} from './foo'
				export {default as bar} from './bar'
			`,
			"/foo.js": `
				export default 'foo'
			`,
			"/bar.js": `
				export default 'bar'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestReExportDefaultExternal(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export {default as foo} from 'foo'
				export {bar} from './bar'
			`,
			"/bar.js": `
				export {default as bar} from 'bar'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"foo": true,
					"bar": true,
				},
			},
		},
	})
}

func TestReExportDefaultNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export {default as foo} from './foo'
				export {default as bar} from './bar'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestImportMetaCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(import.meta.url, import.meta.path)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			OutputFormat:  config.FormatCommonJS,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestImportMetaES6(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(import.meta.url, import.meta.path)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestImportMetaNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(import.meta.url, import.meta.path)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestDeduplicateCommentsInBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import './a'
				import './b'
				import './c'
			`,
			"/a.js": `console.log('in a') //! Copyright notice 1`,
			"/b.js": `console.log('in b') //! Copyright notice 1`,
			"/c.js": `console.log('in c') //! Copyright notice 2`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:       true,
			RemoveWhitespace: true,
			AbsOutputFile:    "/out.js",
		},
	})
}

// The IIFE should not be an arrow function when targeting ES5
func TestIIFE_ES5(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log('test');
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:          true,
			UnsupportedFeatures: es(5),
			OutputFormat:        config.FormatIIFE,
			AbsOutputFile:       "/out.js",
		},
	})
}

func TestOutputExtensionRemappingFile(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log('test');
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:       true,
			OutputExtensions: map[string]string{".js": ".notjs"},
			AbsOutputFile:    "/out.js",
		},
	})
}

func TestOutputExtensionRemappingDir(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log('test');
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:       true,
			OutputExtensions: map[string]string{".js": ".notjs"},
			AbsOutputDir:     "/out",
		},
	})
}

func TestTopLevelAwait(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				await foo;
				for await (foo of bar) ;
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: error: Top-level await is currently not supported when bundling
/entry.js: error: Top-level await is currently not supported when bundling
`,
	})
}
