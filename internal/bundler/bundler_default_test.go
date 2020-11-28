package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/logger"
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

// This test makes sure that NewExpressions containing require() calls aren't
// broken.
func TestNewExpressionCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				new (require("./foo.js")).Foo();
			`,
			"/foo.js": `
				class Foo {}
				module.exports = {Foo};
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatIIFE,
			GlobalName:    []string{"globalName"},
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode: config.ModeBundle,
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
			Mode: config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestImportMissingNeitherES6NorCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/named.js": `
				import fn, {x as a, y as b} from './foo'
				console.log(fn(a, b))
			`,
			"/star.js": `
				import * as ns from './foo'
				console.log(ns.default(ns.x, ns.y))
			`,
			"/star-capture.js": `
				import * as ns from './foo'
				console.log(ns)
			`,
			"/bare.js": `
				import './foo'
			`,
			"/require.js": `
				console.log(require('./foo'))
			`,
			"/import.js": `
				console.log(import('./foo'))
			`,
			"/foo.js": `
				console.log('no exports here')
			`,
		},
		entryPaths: []string{
			"/named.js",
			"/star.js",
			"/star-capture.js",
			"/bare.js",
			"/require.js",
			"/import.js",
		},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
		expectedCompileLog: `/named.js: warning: Import "default" will always be undefined
/named.js: warning: Import "x" will always be undefined
/named.js: warning: Import "y" will always be undefined
/star.js: warning: Import "default" will always be undefined
/star.js: warning: Import "x" will always be undefined
/star.js: warning: Import "y" will always be undefined
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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

				// Try/catch should silence this warning for require()
				try {
					require(tag` + "`./b`" + `)
					require(` + "`./${b}`" + `)
					import(tag` + "`./b`" + `)
					import(` + "`./${b}`" + `)
				} catch {
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: warning: This call to "require" will not be bundled because the argument is not a string literal
/entry.js: warning: This call to "require" will not be bundled because the argument is not a string literal
/entry.js: warning: This dynamic import will not be bundled because the argument is not a string literal
/entry.js: warning: This dynamic import will not be bundled because the argument is not a string literal
/entry.js: warning: This dynamic import will not be bundled because the argument is not a string literal
/entry.js: warning: This dynamic import will not be bundled because the argument is not a string literal
`,
	})
}

func TestDynamicImportWithExpressionCJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import('foo')
				import(foo())
			`,
		},
		entryPaths: []string{"/a.js"},
		options: config.Options{
			Mode:          config.ModeConvertFormat,
			OutputFormat:  config.FormatCommonJS,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestDynamicImportWithExpressionCJSAndES5(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import('foo')
				import(foo())
			`,
		},
		entryPaths: []string{"/a.js"},
		options: config.Options{
			Mode:                  config.ModeConvertFormat,
			OutputFormat:          config.FormatCommonJS,
			UnsupportedJSFeatures: es(5),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestMinifiedDynamicImportWithExpressionCJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import('foo')
				import(foo())
			`,
		},
		entryPaths: []string{"/a.js"},
		options: config.Options{
			Mode:             config.ModeConvertFormat,
			OutputFormat:     config.FormatCommonJS,
			AbsOutputFile:    "/out.js",
			RemoveWhitespace: true,
		},
	})
}

func TestMinifiedDynamicImportWithExpressionCJSAndES5(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import('foo')
				import(foo())
			`,
		},
		entryPaths: []string{"/a.js"},
		options: config.Options{
			Mode:                  config.ModeConvertFormat,
			OutputFormat:          config.FormatCommonJS,
			UnsupportedJSFeatures: es(5),
			AbsOutputFile:         "/out.js",
			RemoveWhitespace:      true,
		},
	})
}

func TestConditionalRequireResolve(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				require.resolve(x ? 'a' : y ? 'b' : 'c')
				require.resolve(x ? y ? 'a' : 'b' : c)
			`,
		},
		entryPaths: []string{"/a.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"a": true,
					"b": true,
					"c": true,
				},
			},
		},
	})
}

func TestConditionalRequire(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				require(x ? 'a' : y ? './b' : 'c')
				require(x ? y ? 'a' : './b' : c)
			`,
			"/b.js": `
				exports.foo = 213
			`,
		},
		entryPaths: []string{"/a.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"a": true,
					"c": true,
				},
			},
		},
		expectedScanLog: `/a.js: warning: This call to "require" will not be bundled because the argument is not a string literal
`,
	})
}

func TestConditionalImport(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import(x ? 'a' : y ? './b' : 'c')
				import(x ? y ? 'a' : './b' : c)
			`,
			"/b.js": `
				exports.foo = 213
			`,
		},
		entryPaths: []string{"/a.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"a": true,
					"c": true,
				},
			},
		},
		expectedScanLog: `/a.js: warning: This dynamic import will not be bundled because the argument is not a string literal
`,
	})
}

func TestRequireBadArgumentCount(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				require()
				require("a", "b")

				// Try/catch should silence this warning
				try {
					require()
					require("a", "b")
				} catch {
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: error: File could not be loaded: /test
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: warning: Indirect calls to "require" will not be bundled (surround with a try/catch to silence this warning)
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: warning: Indirect calls to "require" will not be bundled (surround with a try/catch to silence this warning)
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: warning: Indirect calls to "require" will not be bundled (surround with a try/catch to silence this warning)
/entry.js: warning: Indirect calls to "require" will not be bundled (surround with a try/catch to silence this warning)
/entry.js: warning: Indirect calls to "require" will not be bundled (surround with a try/catch to silence this warning)
/entry.js: warning: Indirect calls to "require" will not be bundled (surround with a try/catch to silence this warning)
/entry.js: warning: Indirect calls to "require" will not be bundled (surround with a try/catch to silence this warning)
/entry.js: warning: Indirect calls to "require" will not be bundled (surround with a try/catch to silence this warning)
/entry.js: warning: Indirect calls to "require" will not be bundled (surround with a try/catch to silence this warning)
/entry.js: warning: Indirect calls to "require" will not be bundled (surround with a try/catch to silence this warning)
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			Platform:      config.PlatformBrowser,
		},
		expectedScanLog: `/entry.js: error: Could not resolve "fs" (set platform to "node" when building for node)
`,
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
			Mode:          config.ModeBundle,
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
			Mode:             config.ModeBundle,
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			Platform:      config.PlatformBrowser,
		},
		expectedScanLog: `/entry.js: error: Could not resolve "fs" (set platform to "node" when building for node)
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			Platform:      config.PlatformBrowser,
		},
		expectedScanLog: `/entry.js: error: Could not resolve "fs" (set platform to "node" when building for node)
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:              config.ModeBundle,
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
			Mode:              config.ModeBundle,
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
			Mode:             config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:              config.ModeBundle,
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
			Mode: config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"@a1":       true,
					"@b1/b2":    true,
					"@c1/c2/c3": true,
				},
			},
		},
		expectedScanLog: `/index.js: error: Could not resolve "@a1-a2" (mark it as external to exclude it from the bundle)
/index.js: error: Could not resolve "@b1" (mark it as external to exclude it from the bundle)
/index.js: error: Could not resolve "@b1/b2-b3" (mark it as external to exclude it from the bundle)
/index.js: error: Could not resolve "@c1" (mark it as external to exclude it from the bundle)
/index.js: error: Could not resolve "@c1/c2" (mark it as external to exclude it from the bundle)
/index.js: error: Could not resolve "@c1/c2/c3-c4" (mark it as external to exclude it from the bundle)
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
			Mode:          config.ModeBundle,
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
				import out from '../../../out/in-out-dir.js'
				import sha256 from '../../sha256.min.js'
				import config from '/api/config?a=1&b=2'
				console.log(foo, out, sha256, config)
			`,
		},
		entryPaths: []string{"/Users/user/project/src/index.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/Users/user/project/out",
			ExternalModules: config.ExternalModules{
				AbsPaths: map[string]bool{
					"/Users/user/project/out/in-out-dir.js":        true,
					"/Users/user/project/src/nested/folder/foo.js": true,
					"/Users/user/project/src/sha256.min.js":        true,
					"/api/config?a=1&b=2":                          true,
				},
			},
		},
	})
}

func TestAutoExternal(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				// These URLs should be external automatically
				import "http://example.com/code.js";
				import "https://example.com/code.js";
				import "//example.com/code.js";
				import "data:application/javascript;base64,ZXhwb3J0IGRlZmF1bHQgMTIz";
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestExternalWithWildcard(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				// Should match
				import "/assets/images/test.jpg";
				import "/dir/x/file.gif";
				import "/dir//file.gif";
				import "./file.png";

				// Should not match
				import "/sassets/images/test.jpg";
				import "/dir/file.gif";
				import "./file.ping";
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExternalModules: config.ExternalModules{
				Patterns: []config.WildcardPattern{
					{Prefix: "/assets/"},
					{Suffix: ".png"},
					{Prefix: "/dir/", Suffix: "/file.gif"},
				},
			},
		},
		expectedScanLog: `/entry.js: error: Could not read from file: /sassets/images/test.jpg
/entry.js: error: Could not read from file: /dir/file.gif
/entry.js: error: Could not resolve "./file.ping"
`,
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
			Mode:         config.ModeBundle,
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
					set #bar(x) {}
				}
				class Bar {
					#foo
					foo = class {
						#foo2
						#foo
						#bar
					}
					get #bar() {}
					set #bar(x) {}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
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
					set #bar(x) {}
				}
				class Bar {
					#foo
					foo = class {
						#foo2
						#foo
						#bar
					}
					get #bar() {}
					set #bar(x) {}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
	})
}

func TestRenameLabelsNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				foo: {
					bar: {
						if (x) break bar
						break foo
					}
				}
				foo2: {
					bar2: {
						if (x) break bar2
						break foo2
					}
				}
				foo: {
					bar: {
						if (x) break bar
						break foo
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			AbsOutputFile: "/out.js",
		},
	})
}

// These labels should all share the same minified names
func TestMinifySiblingLabelsNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				foo: {
					bar: {
						if (x) break bar
						break foo
					}
				}
				foo2: {
					bar2: {
						if (x) break bar2
						break foo2
					}
				}
				foo: {
					bar: {
						if (x) break bar
						break foo
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
	})
}

// We shouldn't ever generate a label with the name "if"
func TestMinifyNestedLabelsNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				L001:{L002:{L003:{L004:{L005:{L006:{L007:{L008:{L009:{L010:{L011:{L012:{L013:{L014:{L015:{L016:{nl('\n')
				L017:{L018:{L019:{L020:{L021:{L022:{L023:{L024:{L025:{L026:{L027:{L028:{L029:{L030:{L031:{L032:{nl('\n')
				L033:{L034:{L035:{L036:{L037:{L038:{L039:{L040:{L041:{L042:{L043:{L044:{L045:{L046:{L047:{L048:{nl('\n')
				L049:{L050:{L051:{L052:{L053:{L054:{L055:{L056:{L057:{L058:{L059:{L060:{L061:{L062:{L063:{L064:{nl('\n')
				L065:{L066:{L067:{L068:{L069:{L070:{L071:{L072:{L073:{L074:{L075:{L076:{L077:{L078:{L079:{L080:{nl('\n')
				L081:{L082:{L083:{L084:{L085:{L086:{L087:{L088:{L089:{L090:{L091:{L092:{L093:{L094:{L095:{L096:{nl('\n')
				L097:{L098:{L099:{L100:{L101:{L102:{L103:{L104:{L105:{L106:{L107:{L108:{L109:{L110:{L111:{L112:{nl('\n')
				L113:{L114:{L115:{L116:{L117:{L118:{L119:{L120:{L121:{L122:{L123:{L124:{L125:{L126:{L127:{L128:{nl('\n')
				L129:{L130:{L131:{L132:{L133:{L134:{L135:{L136:{L137:{L138:{L139:{L140:{L141:{L142:{L143:{L144:{nl('\n')
				L145:{L146:{L147:{L148:{L149:{L150:{L151:{L152:{L153:{L154:{L155:{L156:{L157:{L158:{L159:{L160:{nl('\n')
				L161:{L162:{L163:{L164:{L165:{L166:{L167:{L168:{L169:{L170:{L171:{L172:{L173:{L174:{L175:{L176:{nl('\n')
				L177:{L178:{L179:{L180:{L181:{L182:{L183:{L184:{L185:{L186:{L187:{L188:{L189:{L190:{L191:{L192:{nl('\n')
				L193:{L194:{L195:{L196:{L197:{L198:{L199:{L200:{L201:{L202:{L203:{L204:{L205:{L206:{L207:{L208:{nl('\n')
				L209:{L210:{L211:{L212:{L213:{L214:{L215:{L216:{L217:{L218:{L219:{L220:{L221:{L222:{L223:{L224:{nl('\n')
				L225:{L226:{L227:{L228:{L229:{L230:{L231:{L232:{L233:{L234:{L235:{L236:{L237:{L238:{L239:{L240:{nl('\n')
				L241:{L242:{L243:{L244:{L245:{L246:{L247:{L248:{L249:{L250:{L251:{L252:{L253:{L254:{L255:{L256:{nl('\n')
				L257:{L258:{L259:{L260:{L261:{L262:{L263:{L264:{L265:{L266:{L267:{L268:{L269:{L270:{L271:{L272:{nl('\n')
				L273:{L274:{L275:{L276:{L277:{L278:{L279:{L280:{L281:{L282:{L283:{L284:{L285:{L286:{L287:{L288:{nl('\n')
				L289:{L290:{L291:{L292:{L293:{L294:{L295:{L296:{L297:{L298:{L299:{L300:{L301:{L302:{L303:{L304:{nl('\n')
				L305:{L306:{L307:{L308:{L309:{L310:{L311:{L312:{L313:{L314:{L315:{L316:{L317:{L318:{L319:{L320:{nl('\n')
				L321:{L322:{L323:{L324:{L325:{L326:{L327:{L328:{L329:{L330:{L331:{L332:{L333:{}}}}}}}}}}}}}}}}}}nl('\n')
				}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}nl('\n')
				}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}nl('\n')
				}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}nl('\n')
				}}}}}}}}}}}}}}}}}}}}}}}}}}}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			RemoveWhitespace:  true,
			MinifyIdentifiers: true,
			MangleSyntax:      true,
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
			Mode:          config.ModeBundle,
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
			Mode:              config.ModeBundle,
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
			Mode:         config.ModeBundle,
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
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out.js",
		},
		expectedScanLog: "error: Duplicate entry point \"/entry.js\"\n",
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
			Mode:         config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestReExportDefaultExternalES6(t *testing.T) {
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			OutputFormat:  config.FormatESModule,
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"foo": true,
					"bar": true,
				},
			},
		},
	})
}

func TestReExportDefaultExternalCommonJS(t *testing.T) {
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			OutputFormat:  config.FormatCommonJS,
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
			AbsOutputFile: "/out.js",
		},
	})
}

func TestReExportDefaultNoBundleES6(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export {default as foo} from './foo'
				export {default as bar} from './bar'
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

func TestReExportDefaultNoBundleCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export {default as foo} from './foo'
				export {default as bar} from './bar'
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

func TestImportMetaCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(import.meta.url, import.meta.path)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
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
			Mode:          config.ModeBundle,
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
			Mode:             config.ModeBundle,
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
			Mode:                  config.ModeBundle,
			UnsupportedJSFeatures: es(5),
			OutputFormat:          config.FormatIIFE,
			AbsOutputFile:         "/out.js",
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
			Mode:              config.ModeBundle,
			OutputExtensionJS: ".notjs",
			AbsOutputFile:     "/out.js",
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
			Mode:              config.ModeBundle,
			OutputExtensionJS: ".notjs",
			AbsOutputDir:      "/out",
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: error: Top-level await is currently not supported when bundling
/entry.js: error: Top-level await is currently not supported when bundling
`,
	})
}

func TestTopLevelAwaitNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				await foo;
				for await (foo of bar) ;
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTopLevelAwaitNoBundleES6(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				await foo;
				for await (foo of bar) ;
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			OutputFormat:  config.FormatESModule,
			Mode:          config.ModeConvertFormat,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTopLevelAwaitNoBundleCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				await foo;
				for await (foo of bar) ;
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			OutputFormat:  config.FormatCommonJS,
			Mode:          config.ModeConvertFormat,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: error: Top-level await is currently not supported with the "cjs" output format
/entry.js: error: Top-level await is currently not supported with the "cjs" output format
`,
	})
}

func TestTopLevelAwaitNoBundleIIFE(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				await foo;
				for await (foo of bar) ;
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			OutputFormat:  config.FormatIIFE,
			Mode:          config.ModeConvertFormat,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: error: Top-level await is currently not supported with the "iife" output format
/entry.js: error: Top-level await is currently not supported with the "iife" output format
`,
	})
}

func TestAssignToImport(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import "./bad0.js"
				import "./bad1.js"
				import "./bad2.js"
				import "./bad3.js"
				import "./bad4.js"
				import "./bad5.js"
				import "./bad6.js"
				import "./bad7.js"
				import "./bad8.js"
				import "./bad9.js"
				import "./bad10.js"
				import "./bad11.js"
				import "./bad12.js"
				import "./bad13.js"
				import "./bad14.js"
				import "./bad15.js"

				import "./good0.js"
				import "./good1.js"
				import "./good2.js"
				import "./good3.js"
				import "./good4.js"
			`,
			"/node_modules/foo/index.js": ``,

			"/bad0.js":  `import x from "foo"; x = 1`,
			"/bad1.js":  `import x from "foo"; x++`,
			"/bad2.js":  `import x from "foo"; ([x] = 1)`,
			"/bad3.js":  `import x from "foo"; ({x} = 1)`,
			"/bad4.js":  `import x from "foo"; ({y: x} = 1)`,
			"/bad5.js":  `import {x} from "foo"; x++`,
			"/bad6.js":  `import * as x from "foo"; x++`,
			"/bad7.js":  `import * as x from "foo"; x.y = 1`,
			"/bad8.js":  `import * as x from "foo"; x[y] = 1`,
			"/bad9.js":  `import * as x from "foo"; x['y'] = 1`,
			"/bad10.js": `import * as x from "foo"; x['y z'] = 1`,
			"/bad11.js": `import x from "foo"; delete x`,
			"/bad12.js": `import {x} from "foo"; delete x`,
			"/bad13.js": `import * as x from "foo"; delete x.y`,
			"/bad14.js": `import * as x from "foo"; delete x['y']`,
			"/bad15.js": `import * as x from "foo"; delete x[y]`,

			"/good0.js": `import x from "foo"; ({y = x} = 1)`,
			"/good1.js": `import x from "foo"; ({[x]: y} = 1)`,
			"/good2.js": `import x from "foo"; x.y = 1`,
			"/good3.js": `import x from "foo"; x[y] = 1`,
			"/good4.js": `import x from "foo"; x['y'] = 1`,
			"/good5.js": `import x from "foo"; x['y z'] = 1`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/bad0.js: error: Cannot assign to import "x"
/bad1.js: error: Cannot assign to import "x"
/bad10.js: error: Cannot assign to import "y z"
/bad11.js: error: Cannot assign to import "x"
/bad12.js: error: Cannot assign to import "x"
/bad13.js: error: Cannot assign to import "y"
/bad14.js: error: Cannot assign to import "y"
/bad15.js: error: Cannot assign to property on import "x"
/bad2.js: error: Cannot assign to import "x"
/bad3.js: error: Cannot assign to import "x"
/bad4.js: error: Cannot assign to import "x"
/bad5.js: error: Cannot assign to import "x"
/bad6.js: error: Cannot assign to import "x"
/bad7.js: error: Cannot assign to import "y"
/bad8.js: error: Cannot assign to property on import "x"
/bad9.js: error: Cannot assign to import "y"
`,
	})
}

func TestMinifyArguments(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function a(x = arguments) {
					let arguments
				}
				function b(x = arguments) {
					let arguments
				}
				function c(x = arguments) {
					let arguments
				}
				a()
				b()
				c()
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:              config.ModeBundle,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
	})
}

func TestWarningsInsideNodeModules(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import "./dup-case.js";        import "./node_modules/dup-case.js"
				import "./not-in.js";          import "./node_modules/not-in.js"
				import "./not-instanceof.js";  import "./node_modules/not-instanceof.js"
				import "./return-asi.js";      import "./node_modules/return-asi.js"
				import "./bad-typeof.js";      import "./node_modules/bad-typeof.js"
				import "./equals-neg-zero.js"; import "./node_modules/equals-neg-zero.js"
				import "./equals-nan.js";      import "./node_modules/equals-nan.js"
				import "./equals-object.js";   import "./node_modules/equals-object.js"
				import "./write-getter.js";    import "./node_modules/write-getter.js"
				import "./read-setter.js";     import "./node_modules/read-setter.js"
				import "./delete-super.js";    import "./node_modules/delete-super.js"
			`,

			"/dup-case.js":              "switch (x) { case 0: case 0: }",
			"/node_modules/dup-case.js": "switch (x) { case 0: case 0: }",

			"/not-in.js":              "!a in b",
			"/node_modules/not-in.js": "!a in b",

			"/not-instanceof.js":              "!a instanceof b",
			"/node_modules/not-instanceof.js": "!a instanceof b",

			"/return-asi.js":              "return\n123",
			"/node_modules/return-asi.js": "return\n123",

			"/bad-typeof.js":              "typeof x == 'null'",
			"/node_modules/bad-typeof.js": "typeof x == 'null'",

			"/equals-neg-zero.js":              "x === -0",
			"/node_modules/equals-neg-zero.js": "x === -0",

			"/equals-nan.js":              "x === NaN",
			"/node_modules/equals-nan.js": "x === NaN",

			"/equals-object.js":              "x === []",
			"/node_modules/equals-object.js": "x === []",

			"/write-getter.js":              "class Foo { get #foo() {} foo() { this.#foo = 123 } }",
			"/node_modules/write-getter.js": "class Foo { get #foo() {} foo() { this.#foo = 123 } }",

			"/read-setter.js":              "class Foo { set #foo(x) {} foo() { return this.#foo } }",
			"/node_modules/read-setter.js": "class Foo { set #foo(x) {} foo() { return this.#foo } }",

			"/delete-super.js":              "class Foo extends Bar { foo() { delete super.foo } }",
			"/node_modules/delete-super.js": "class Foo extends Bar { foo() { delete super.foo } }",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/bad-typeof.js: warning: The "typeof" operator will never evaluate to "null"
/delete-super.js: warning: Attempting to delete a property of "super" will throw a ReferenceError
/dup-case.js: warning: This case clause will never be evaluated because it duplicates an earlier case clause
/equals-nan.js: warning: Comparison with NaN using the "===" operator here is always false
/equals-neg-zero.js: warning: Comparison with -0 using the "===" operator will also match 0
/equals-object.js: warning: Comparison using the "===" operator here is always false
/not-in.js: warning: Suspicious use of the "!" operator inside the "in" operator
/not-instanceof.js: warning: Suspicious use of the "!" operator inside the "instanceof" operator
/read-setter.js: warning: Reading from setter-only property "#foo" will throw
/return-asi.js: warning: The following expression is not returned because of an automatically-inserted semicolon
/write-getter.js: warning: Writing to getter-only property "#foo" will throw
`,
	})
}

func TestRequireResolve(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require.resolve)
				console.log(require.resolve())
				console.log(require.resolve(foo))
				console.log(require.resolve('a', 'b'))
				console.log(require.resolve('./present-file'))
				console.log(require.resolve('./missing-file'))
				console.log(require.resolve('./external-file'))
				console.log(require.resolve('missing-pkg'))
				console.log(require.resolve('external-pkg'))
				console.log(require.resolve('@scope/missing-pkg'))
				console.log(require.resolve('@scope/external-pkg'))
				try {
					console.log(require.resolve('inside-try'))
				} catch (e) {
				}
				if (false) {
					console.log(require.resolve('dead-code'))
				}
				console.log(false ? require.resolve('dead-if') : 0)
				console.log(true ? 0 : require.resolve('dead-if'))
				console.log(false && require.resolve('dead-and'))
				console.log(true || require.resolve('dead-or'))
				console.log(true ?? require.resolve('dead-nullish'))
			`,
			"/present-file.js": ``,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExternalModules: config.ExternalModules{
				AbsPaths: map[string]bool{
					"/external-file": true,
				},
				NodeModules: map[string]bool{
					"external-pkg":        true,
					"@scope/external-pkg": true,
				},
			},
		},
		expectedScanLog: `/entry.js: warning: Indirect calls to "require" will not be bundled (surround with a try/catch to silence this warning)
/entry.js: warning: Indirect calls to "require" will not be bundled (surround with a try/catch to silence this warning)
/entry.js: warning: Indirect calls to "require" will not be bundled (surround with a try/catch to silence this warning)
/entry.js: warning: "./present-file" should be marked as external for use with "require.resolve"
/entry.js: warning: "./missing-file" should be marked as external for use with "require.resolve"
/entry.js: warning: "missing-pkg" should be marked as external for use with "require.resolve"
/entry.js: warning: "@scope/missing-pkg" should be marked as external for use with "require.resolve"
`,
	})
}

func TestInjectMissing(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": ``,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			InjectAbsPaths: []string{
				"/inject.js",
			},
		},
		expectedScanLog: `error: Could not read from file: /inject.js
`,
	})
}

func TestInjectDuplicate(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js":  ``,
			"/inject.js": ``,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			InjectAbsPaths: []string{
				"/inject.js",
				"/inject.js",
			},
		},
		expectedScanLog: `error: Duplicate injected file "/inject.js"
`,
	})
}

func TestInject(t *testing.T) {
	defines := config.ProcessDefines(map[string]config.DefineData{
		"chain.prop": {
			DefineFunc: func(loc logger.Loc, findSymbol config.FindSymbol) js_ast.E {
				return &js_ast.EIdentifier{Ref: findSymbol(loc, "replace")}
			},
		},
	})
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				let sideEffects = console.log('this should be renamed')
				let collide = 123
				console.log(obj.prop)
				console.log(chain.prop.test)
				console.log(collide)
				console.log(re_export)
			`,
			"/inject.js": `
				export let obj = {}
				export let sideEffects = console.log('side effects')
				export let noSideEffects = /* @__PURE__ */ console.log('side effects')
			`,
			"/node_modules/unused/index.js": `
				console.log('This is unused but still has side effects')
			`,
			"/node_modules/sideEffects-false/index.js": `
				console.log('This is unused and has no side effects')
			`,
			"/node_modules/sideEffects-false/package.json": `{
				"sideEffects": false
			}`,
			"/replacement.js": `
				export let replace = {
					test() {}
				}
			`,
			"/collision.js": `
				export let collide = 123
			`,
			"/re-export.js": `
				export {re_export} from 'external-pkg'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			Defines:       &defines,
			OutputFormat:  config.FormatCommonJS,
			InjectAbsPaths: []string{
				"/inject.js",
				"/node_modules/unused/index.js",
				"/node_modules/sideEffects-false/index.js",
				"/replacement.js",
				"/collision.js",
				"/re-export.js",
			},
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"external-pkg": true,
				},
			},
		},
	})
}

func TestInjectNoBundle(t *testing.T) {
	defines := config.ProcessDefines(map[string]config.DefineData{
		"chain.prop": {
			DefineFunc: func(loc logger.Loc, findSymbol config.FindSymbol) js_ast.E {
				return &js_ast.EIdentifier{Ref: findSymbol(loc, "replace")}
			},
		},
	})
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				let sideEffects = console.log('this should be renamed')
				let collide = 123
				console.log(obj.prop)
				console.log(chain.prop.test)
				console.log(collide)
				console.log(re_export)
			`,
			"/inject.js": `
				export let obj = {}
				export let sideEffects = console.log('side effects')
				export let noSideEffects = /* @__PURE__ */ console.log('side effects')
			`,
			"/node_modules/unused/index.js": `
				console.log('This is unused but still has side effects')
			`,
			"/node_modules/sideEffects-false/index.js": `
				console.log('This is unused and has no side effects')
			`,
			"/node_modules/sideEffects-false/package.json": `{
				"sideEffects": false
			}`,
			"/replacement.js": `
				export let replace = {
					test() {}
				}
			`,
			"/collision.js": `
				export let collide = 123
			`,
			"/re-export.js": `
				export {re_export} from 'external-pkg'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			AbsOutputFile: "/out.js",
			Defines:       &defines,
			InjectAbsPaths: []string{
				"/inject.js",
				"/node_modules/unused/index.js",
				"/node_modules/sideEffects-false/index.js",
				"/replacement.js",
				"/collision.js",
				"/re-export.js",
			},
		},
	})
}

func TestInjectJSX(t *testing.T) {
	defines := config.ProcessDefines(map[string]config.DefineData{
		"React.createElement": {
			DefineFunc: func(loc logger.Loc, findSymbol config.FindSymbol) js_ast.E {
				return &js_ast.EIdentifier{Ref: findSymbol(loc, "el")}
			},
		},
	})
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.jsx": `
				console.log(<div/>)
			`,
			"/inject.js": `
				export function el() {}
			`,
		},
		entryPaths: []string{"/entry.jsx"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			Defines:       &defines,
			InjectAbsPaths: []string{
				"/inject.js",
			},
		},
	})
}

func TestInjectImportTS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				console.log('here')
			`,
			"/inject.js": `
				// Unused imports are automatically removed in TypeScript files (this
				// is a mis-feature of the TypeScript language). However, injected
				// imports are an esbuild feature so we get to decide what the
				// semantics are. We do not want injected imports to disappear unless
				// they have been explicitly marked as having no side effects.
				console.log('must be present')
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeConvertFormat,
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/out.js",
			InjectAbsPaths: []string{
				"/inject.js",
			},
		},
	})
}

func TestInjectImportOrder(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import 'third'
				console.log('third')
			`,
			"/inject-1.js": `
				import 'first'
				console.log('first')
			`,
			"/inject-2.js": `
				import 'second'
				console.log('second')
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			InjectAbsPaths: []string{
				"/inject-1.js",
				"/inject-2.js",
			},
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"first":  true,
					"second": true,
					"third":  true,
				},
			},
		},
	})
}

func TestOutbase(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a/b/c.js": `
				console.log('c')
			`,
			"/a/b/d.js": `
				console.log('d')
			`,
		},
		entryPaths: []string{"/a/b/c.js", "/a/b/d.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputDir:  "/out",
			AbsOutputBase: "/",
		},
	})
}

func TestAvoidTDZ(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					static foo = new Foo
				}
				let foo = Foo.foo
				console.log(foo)
				export class Bar {}
				export let bar = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestAvoidTDZNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					static foo = new Foo
				}
				let foo = Foo.foo
				console.log(foo)
				export class Bar {}
				export let bar = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestProcessEnvNodeEnvWarning(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(
					process.env.NODE_ENV,
					process.env.NODE_ENV,
				)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: warning: Define "process.env.NODE_ENV" when bundling for the browser
`,
	})
}

func TestProcessEnvNodeEnvWarningNode(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(process.env.NODE_ENV)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			Platform:      config.PlatformNode,
		},
	})
}

func TestProcessEnvNodeEnvWarningDefine(t *testing.T) {
	defines := config.ProcessDefines(map[string]config.DefineData{
		"process.env.NODE_ENV": {
			DefineFunc: func(loc logger.Loc, findSymbol config.FindSymbol) js_ast.E {
				return &js_ast.ENull{}
			},
		},
	})
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(process.env.NODE_ENV)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			Defines:       &defines,
		},
	})
}

func TestProcessEnvNodeEnvWarningNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(process.env.NODE_ENV)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestKeepNamesTreeShaking(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function fnStmtRemove() {}
				function fnStmtKeep() {}
				fnStmtKeep()

				let fnExprRemove = function remove() {}
				let fnExprKeep = function keep() {}
				fnExprKeep()

				class clsStmtRemove {}
				class clsStmtKeep {}
				new clsStmtKeep()

				let clsExprRemove = class remove {}
				let clsExprKeep = class keep {}
				new clsExprKeep()
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			KeepNames:     true,
			MangleSyntax:  true,
		},
	})
}

func TestCharFreqIgnoreComments(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				export default function(one, two, three, four) {
					return 'the argument names must be the same'
				}
			`,
			"/b.js": `
				export default function(one, two, three, four) {
					return 'the argument names must be the same'
				}

				// Some comment text to change the character frequency histogram:
				// ________________________________________________________________________________
				// FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF
				// AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
				// IIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIII
				// LLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLL
			`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			Mode:              config.ModeBundle,
			AbsOutputDir:      "/out",
			MinifyIdentifiers: true,
		},
	})
}

func TestImportRelativeAsPackage(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'some/other/file'
			`,
			"/Users/user/project/src/some/other/file.js": `
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `/Users/user/project/src/entry.js: error: Could not resolve "some/other/file" ` +
			`(use "./some/other/file" to import "/Users/user/project/src/some/other/file.js")
`,
	})
}

func TestForbidConstAssignWhenBundling(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const x = 1
				x = 2
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: error: Cannot assign to "x" because it is a constant
/entry.js: note: "x" was declared a constant here
`,
	})
}

func TestConstWithLet(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const a = 1; console.log(a)
				if (true) { const b = 2; console.log(b) }
				for (const c = x;;) console.log(c)
				for (const d in x) console.log(d)
				for (const e of x) console.log(e)
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

func TestConstWithLetNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const a = 1; console.log(a)
				if (true) { const b = 2; console.log(b) }
				for (const c = x;;) console.log(c)
				for (const d in x) console.log(d)
				for (const e of x) console.log(e)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			AbsOutputFile: "/out.js",
			MangleSyntax:  true,
		},
	})
}

func TestConstWithLetNoMangle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const a = 1; console.log(a)
				if (true) { const b = 2; console.log(b) }
				for (const c = x;;) console.log(c)
				for (const d in x) console.log(d)
				for (const e of x) console.log(e)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestRequireMainCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log('is main:', require.main === module)
				console.log(require('./is-main'))
			`,
			"/is-main.js": `
				module.exports = require.main === module
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			OutputFormat:  config.FormatCommonJS,
		},
	})
}

func TestRequireMainIIFE(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log('is main:', require.main === module)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			OutputFormat:  config.FormatIIFE,
		},
		expectedScanLog: `/entry.js: warning: Indirect calls to "require" will not be bundled (surround with a try/catch to silence this warning)
`,
	})
}
