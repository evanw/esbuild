package bundler

import (
	"regexp"
	"strings"
	"testing"

	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
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
		expectedCompileLog: `entry.js: ERROR: Detected cycle while resolving import "a"
entry.js: ERROR: Detected cycle while resolving import "b"
entry.js: ERROR: Detected cycle while resolving import "c"
entry.js: ERROR: Detected cycle while resolving import "d"
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
		expectedCompileLog: `entry.js: ERROR: Detected cycle while resolving import "a"
entry.js: ERROR: Detected cycle while resolving import "c"
foo.js: ERROR: Detected cycle while resolving import "b"
foo.js: ERROR: Detected cycle while resolving import "d"
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
				Factory:  config.JSXExpr{Parts: []string{"elem"}},
				Fragment: config.JSXExpr{Parts: []string{"frag"}},
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
				Factory:  config.JSXExpr{Parts: []string{"elem"}},
				Fragment: config.JSXExpr{Parts: []string{"frag"}},
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
		expectedScanLog: `entry.js: ERROR: The JSX syntax extension is not currently enabled
NOTE: The esbuild loader for this file is currently set to "js" but it must be set to "jsx" to be able to parse JSX syntax. ` +
			`You can use 'Loader: map[string]api.Loader{".js": api.LoaderJSX}' to do that.
`,
	})
}

func TestJSXConstantFragments(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import './default'
				import './null'
				import './boolean'
				import './number'
				import './string-single-empty'
				import './string-double-empty'
				import './string-single-punctuation'
				import './string-double-punctuation'
				import './string-template'
			`,
			"/default.jsx":                   `console.log(<></>)`,
			"/null.jsx":                      `console.log(<></>) // @jsxFrag null`,
			"/boolean.jsx":                   `console.log(<></>) // @jsxFrag true`,
			"/number.jsx":                    `console.log(<></>) // @jsxFrag 123`,
			"/string-single-empty.jsx":       `console.log(<></>) // @jsxFrag ''`,
			"/string-double-empty.jsx":       `console.log(<></>) // @jsxFrag ""`,
			"/string-single-punctuation.jsx": `console.log(<></>) // @jsxFrag '['`,
			"/string-double-punctuation.jsx": `console.log(<></>) // @jsxFrag "["`,
			"/string-template.jsx":           `console.log(<></>) // @jsxFrag ` + "``",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			JSX: config.JSXOptions{
				Fragment: config.JSXExpr{
					Constant: &js_ast.EString{Value: js_lexer.StringToUTF16("]")},
				},
			},
		},
		expectedScanLog: `string-template.jsx: WARNING: Invalid JSX fragment: ` + "``" + `
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
		expectedCompileLog: `entry.js: ERROR: No matching export in "foo.js" for import "default"
entry.js: ERROR: No matching export in "foo.js" for import "y"
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
		expectedCompileLog: `entry.js: ERROR: No matching export in "foo.js" for import "default"
entry.js: ERROR: No matching export in "foo.js" for import "y"
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
		expectedCompileLog: `named.js: WARNING: Import "x" will always be undefined because the file "foo.js" has no exports
named.js: WARNING: Import "y" will always be undefined because the file "foo.js" has no exports
star.js: WARNING: Import "x" will always be undefined because the file "foo.js" has no exports
star.js: WARNING: Import "y" will always be undefined because the file "foo.js" has no exports
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
		expectedCompileLog: `foo.js: ERROR: No matching export in "bar.js" for import "nope"
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

				try {
					require(tag` + "`./b`" + `)
					require(` + "`./${b}`" + `)
				} catch {
				}

				(async () => {
					import(tag` + "`./b`" + `)
					import(` + "`./${b}`" + `)
					await import(tag` + "`./b`" + `)
					await import(` + "`./${b}`" + `)

					try {
						import(tag` + "`./b`" + `)
						import(` + "`./${b}`" + `)
						await import(tag` + "`./b`" + `)
						await import(` + "`./${b}`" + `)
					} catch {
					}
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
			Platform:      config.PlatformNode,
			OutputFormat:  config.FormatCommonJS,
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
	})
}

func TestConditionalImport(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import(x ? 'a' : y ? './import' : 'c')
			`,
			"/b.js": `
				import(x ? y ? 'a' : './import' : c)
			`,
			"/import.js": `
				exports.foo = 213
			`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"a": true,
					"c": true,
				},
			},
		},
	})
}

func TestRequireBadArgumentCount(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				require()
				require("a", "b")

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
		expectedScanLog: `entry.js: ERROR: Do not know how to load path: test
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

func TestRequirePropertyAccessCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				// These shouldn't warn since the format is CommonJS
				console.log(Object.keys(require.cache))
				console.log(Object.keys(require.extensions))
				delete require.cache['fs']
				delete require.extensions['.json']
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			Platform:      config.PlatformNode,
			OutputFormat:  config.FormatCommonJS,
			AbsOutputFile: "/out.js",
		},
	})
}

// Test a workaround for code using "await import()"
func TestAwaitImportInsideTry(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				async function main(name) {
					try {
						return await import(name)
					} catch {
					}
				}
				main('fs')
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestImportInsideTry(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				let x
				try {
					x = import('nope1')
					x = await import('nope2')
				} catch {
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `entry.js: ERROR: Could not resolve "nope1"
NOTE: You can mark the path "nope1" as external to exclude it from the bundle, which will remove this error. You can also add ".catch()" here to handle this failure at run-time instead of bundle-time.
`,
	})
}

// Test a workaround for code using "import().catch()"
func TestImportThenCatch(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import(name).then(pass, fail)
				import(name).then(pass).catch(fail)
				import(name).catch(fail)
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
		expectedScanLog: `entry.js: ERROR: Could not resolve "fs"
NOTE: The package "fs" wasn't found on the file system but is built into node. Are you trying to bundle for node? You can use "Platform: api.PlatformNode" to do that, which will remove this error.
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
		expectedScanLog: `entry.js: ERROR: Could not resolve "fs"
NOTE: The package "fs" wasn't found on the file system but is built into node. Are you trying to bundle for node? You can use "Platform: api.PlatformNode" to do that, which will remove this error.
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
		expectedScanLog: `entry.js: ERROR: Could not resolve "fs"
NOTE: The package "fs" wasn't found on the file system but is built into node. Are you trying to bundle for node? You can use "Platform: api.PlatformNode" to do that, which will remove this error.
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
				import * as fs from 'fs'
				import {readFileSync} from 'fs'
				exports.fs = fs
				exports.readFileSync = readFileSync
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

func TestTopLevelReturnForbiddenImport(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				return
				import 'foo'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `entry.js: ERROR: Top-level return cannot be used inside an ECMAScript module
entry.js: NOTE: This file is considered an ECMAScript module because of the "import" keyword here:
`,
	})
}

func TestTopLevelReturnForbiddenExport(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				return
				export var foo
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `entry.js: ERROR: Top-level return cannot be used inside an ECMAScript module
entry.js: NOTE: This file is considered an ECMAScript module because of the "export" keyword here:
`,
	})
}

func TestTopLevelReturnForbiddenTLA(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				return await foo
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `entry.js: ERROR: Top-level return cannot be used inside an ECMAScript module
entry.js: NOTE: This file is considered an ECMAScript module because of the "await" keyword here:
`,
	})
}

func TestThisOutsideFunction(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				if (shouldBeExportsNotThis) {
					console.log(this)
					console.log((x = this) => this)
					console.log({x: this})
					console.log(class extends this.foo {})
					console.log(class { [this.foo] })
					console.log(class { [this.foo]() {} })
					console.log(class { static [this.foo] })
					console.log(class { static [this.foo]() {} })
				}
				if (shouldBeThisNotExports) {
					console.log(class { foo = this })
					console.log(class { foo() { this } })
					console.log(class { static foo = this })
					console.log(class { static foo() { this } })
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

func TestThisInsideFunction(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function foo(x = this) { console.log(this) }
				const objFoo = {
					foo(x = this) { console.log(this) }
				}
				class Foo {
					x = this
					static y = this.z
					foo(x = this) { console.log(this) }
					static bar(x = this) { console.log(this) }
				}
				new Foo(foo(objFoo))
				if (nested) {
					function bar(x = this) { console.log(this) }
					const objBar = {
						foo(x = this) { console.log(this) }
					}
					class Bar {
						x = this
						static y = this.z
						foo(x = this) { console.log(this) }
						static bar(x = this) { console.log(this) }
					}
					new Bar(bar(objBar))
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
		expectedScanLog: `es6-export-abstract-class.ts: WARNING: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
es6-export-abstract-class.ts: NOTE: This file is considered an ECMAScript module because of the "export" keyword here:
es6-export-async-function.js: WARNING: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
es6-export-async-function.js: NOTE: This file is considered an ECMAScript module because of the "export" keyword here:
es6-export-class.js: WARNING: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
es6-export-class.js: NOTE: This file is considered an ECMAScript module because of the "export" keyword here:
es6-export-clause-from.js: WARNING: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
es6-export-clause-from.js: NOTE: This file is considered an ECMAScript module because of the "export" keyword here:
es6-export-clause.js: WARNING: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
es6-export-clause.js: NOTE: This file is considered an ECMAScript module because of the "export" keyword here:
es6-export-const-enum.ts: WARNING: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
es6-export-const-enum.ts: NOTE: This file is considered an ECMAScript module because of the "export" keyword here:
es6-export-default.js: WARNING: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
es6-export-default.js: NOTE: This file is considered an ECMAScript module because of the "export" keyword here:
es6-export-enum.ts: WARNING: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
es6-export-enum.ts: NOTE: This file is considered an ECMAScript module because of the "export" keyword here:
es6-export-function.js: WARNING: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
es6-export-function.js: NOTE: This file is considered an ECMAScript module because of the "export" keyword here:
es6-export-import-assign.ts: WARNING: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
es6-export-import-assign.ts: NOTE: This file is considered an ECMAScript module because of the "export" keyword here:
es6-export-module.ts: WARNING: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
es6-export-module.ts: NOTE: This file is considered an ECMAScript module because of the "export" keyword here:
es6-export-namespace.ts: WARNING: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
es6-export-namespace.ts: NOTE: This file is considered an ECMAScript module because of the "export" keyword here:
es6-export-star-as.js: WARNING: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
es6-export-star-as.js: NOTE: This file is considered an ECMAScript module because of the "export" keyword here:
es6-export-star.js: WARNING: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
es6-export-star.js: NOTE: This file is considered an ECMAScript module because of the "export" keyword here:
es6-export-variable.js: WARNING: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
es6-export-variable.js: NOTE: This file is considered an ECMAScript module because of the "export" keyword here:
es6-expr-import-meta.js: WARNING: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
es6-expr-import-meta.js: NOTE: This file is considered an ECMAScript module because of the "import" keyword here:
es6-import-meta.js: WARNING: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
es6-import-meta.js: NOTE: This file is considered an ECMAScript module because of the "import" keyword here:
es6-import-stmt.js: WARNING: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
es6-import-stmt.js: NOTE: This file is considered an ECMAScript module because of the "import" keyword here:
`,
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
				Factory: config.JSXExpr{Parts: []string{"h"}},
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
		expectedScanLog: `index.js: ERROR: Could not resolve "@a1-a2"
NOTE: You can mark the path "@a1-a2" as external to exclude it from the bundle, which will remove this error.
index.js: ERROR: Could not resolve "@b1"
NOTE: You can mark the path "@b1" as external to exclude it from the bundle, which will remove this error.
index.js: ERROR: Could not resolve "@b1/b2-b3"
NOTE: You can mark the path "@b1/b2-b3" as external to exclude it from the bundle, which will remove this error.
index.js: ERROR: Could not resolve "@c1"
NOTE: You can mark the path "@c1" as external to exclude it from the bundle, which will remove this error.
index.js: ERROR: Could not resolve "@c1/c2"
NOTE: You can mark the path "@c1/c2" as external to exclude it from the bundle, which will remove this error.
index.js: ERROR: Could not resolve "@c1/c2/c3-c4"
NOTE: You can mark the path "@c1/c2/c3-c4" as external to exclude it from the bundle, which will remove this error.
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

// Webpack supports this case, so we do too. Some libraries apparently have
// these paths: https://github.com/webpack/enhanced-resolve/issues/247
func TestImportWithHashInPath(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import foo from './file#foo.txt'
				import bar from './file#bar.txt'
				console.log(foo, bar)
			`,
			"/file#foo.txt": `foo`,
			"/file#bar.txt": `bar`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestImportWithHashParameter(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				// Each of these should have a separate identity (i.e. end up in the output file twice)
				import foo from './file.txt#foo'
				import bar from './file.txt#bar'
				console.log(foo, bar)
			`,
			"/file.txt": `This is some text`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestImportWithQueryParameter(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				// Each of these should have a separate identity (i.e. end up in the output file twice)
				import foo from './file.txt?foo'
				import bar from './file.txt?bar'
				console.log(foo, bar)
			`,
			"/file.txt": `This is some text`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestImportAbsPathWithQueryParameter(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/entry.js": `
				// Each of these should have a separate identity (i.e. end up in the output file twice)
				import foo from '/Users/user/project/file.txt?foo'
				import bar from '/Users/user/project/file.txt#bar'
				console.log(foo, bar)
			`,
			"/Users/user/project/file.txt": `This is some text`,
		},
		entryPaths: []string{"/Users/user/project/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestImportAbsPathAsFile(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/entry.js": `
				import pkg from '/Users/user/project/node_modules/pkg/index'
				console.log(pkg)
			`,
			"/Users/user/project/node_modules/pkg/index.js": `
				export default 123
			`,
		},
		entryPaths: []string{"/Users/user/project/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestImportAbsPathAsDir(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/entry.js": `
				import pkg from '/Users/user/project/node_modules/pkg'
				console.log(pkg)
			`,
			"/Users/user/project/node_modules/pkg/index.js": `
				export default 123
			`,
		},
		entryPaths: []string{"/Users/user/project/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
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

func TestAutoExternalNode(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				// These URLs should be external automatically
				import fs from "node:fs/promises";
				fs.readFile();

				// This should be external and should be tree-shaken because it's side-effect free
				import "node:path";

				// This should be external too, but shouldn't be tree-shaken because it could be a run-time error
				import "node:what-is-this";
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			Platform:     config.PlatformNode,
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
		expectedScanLog: `entry.js: ERROR: Could not resolve "/sassets/images/test.jpg"
entry.js: ERROR: Could not resolve "/dir/file.gif"
entry.js: ERROR: Could not resolve "./file.ping"
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

func TestEmptyExportClauseBundleAsCommonJSIssue910(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./types.mjs'))
			`,
			"/types.mjs": `
				export {}
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

func TestUseStrictDirectiveBundleIssue1837(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./cjs'))
			`,
			"/cjs.js": `
				'use strict'
				exports.foo = process
			`,
			"/shims.js": `
				import process from 'process'
				export { process }
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:           config.ModeBundle,
			AbsOutputFile:  "/out.js",
			InjectAbsPaths: []string{"/shims.js"},
			Platform:       config.PlatformNode,
			OutputFormat:   config.FormatIIFE,
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
		expectedCompileLog: `ERROR: Refusing to overwrite input file "entry.js" (use "AllowOverwrite: true" to allow this)
`,
	})
}

func TestDuplicateEntryPoint(t *testing.T) {
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
	})
}

func TestRelativeEntryPointError(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(123)
			`,
		},
		entryPaths: []string{"entry"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out.js",
		},
		expectedScanLog: `ERROR: Could not resolve "entry"
NOTE: Use the relative path "./entry" to reference the file "entry.js". Without the leading "./", the path "entry" is being interpreted as a package path instead.
`,
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

func TestLegalCommentsNone(t *testing.T) {
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

			"/entry.css": `
				@import "./a.css";
				@import "./b.css";
				@import "./c.css";
			`,
			"/a.css": `a { zoom: 2 } /*! Copyright notice 1 */`,
			"/b.css": `b { zoom: 2 } /*! Copyright notice 1 */`,
			"/c.css": `c { zoom: 2 } /*! Copyright notice 2 */`,
		},
		entryPaths: []string{"/entry.js", "/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputDir:  "/out",
			LegalComments: config.LegalCommentsNone,
		},
	})
}

func TestLegalCommentsInline(t *testing.T) {
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

			"/entry.css": `
				@import "./a.css";
				@import "./b.css";
				@import "./c.css";
			`,
			"/a.css": `a { zoom: 2 } /*! Copyright notice 1 */`,
			"/b.css": `b { zoom: 2 } /*! Copyright notice 1 */`,
			"/c.css": `c { zoom: 2 } /*! Copyright notice 2 */`,
		},
		entryPaths: []string{"/entry.js", "/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputDir:  "/out",
			LegalComments: config.LegalCommentsInline,
		},
	})
}

func TestLegalCommentsEndOfFile(t *testing.T) {
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

			"/entry.css": `
				@import "./a.css";
				@import "./b.css";
				@import "./c.css";
			`,
			"/a.css": `a { zoom: 2 } /*! Copyright notice 1 */`,
			"/b.css": `b { zoom: 2 } /*! Copyright notice 1 */`,
			"/c.css": `c { zoom: 2 } /*! Copyright notice 2 */`,
		},
		entryPaths: []string{"/entry.js", "/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputDir:  "/out",
			LegalComments: config.LegalCommentsEndOfFile,
		},
	})
}

func TestLegalCommentsLinked(t *testing.T) {
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

			"/entry.css": `
				@import "./a.css";
				@import "./b.css";
				@import "./c.css";
			`,
			"/a.css": `a { zoom: 2 } /*! Copyright notice 1 */`,
			"/b.css": `b { zoom: 2 } /*! Copyright notice 1 */`,
			"/c.css": `c { zoom: 2 } /*! Copyright notice 2 */`,
		},
		entryPaths: []string{"/entry.js", "/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputDir:  "/out",
			LegalComments: config.LegalCommentsLinkedWithComment,
		},
	})
}

func TestLegalCommentsExternal(t *testing.T) {
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

			"/entry.css": `
				@import "./a.css";
				@import "./b.css";
				@import "./c.css";
			`,
			"/a.css": `a { zoom: 2 } /*! Copyright notice 1 */`,
			"/b.css": `b { zoom: 2 } /*! Copyright notice 1 */`,
			"/c.css": `c { zoom: 2 } /*! Copyright notice 2 */`,
		},
		entryPaths: []string{"/entry.js", "/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputDir:  "/out",
			LegalComments: config.LegalCommentsExternalWithoutComment,
		},
	})
}

func TestLegalCommentsModifyIndent(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export default () => {
					/**
					 * @preserve
					 */
				}
			`,
			"/entry.css": `
				@media (x: y) {
					/**
					 * @preserve
					 */
					z { zoom: 2 }
				}
			`,
		},
		entryPaths: []string{"/entry.js", "/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputDir:  "/out",
			LegalComments: config.LegalCommentsInline,
		},
	})
}

func TestLegalCommentsAvoidSlashTagInline(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				//! <script>foo</script>
				export let x
			`,
			"/entry.css": `
				/*! <style>foo</style> */
				x { y: z }
			`,
		},
		entryPaths: []string{"/entry.js", "/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputDir:  "/out",
			LegalComments: config.LegalCommentsInline,
		},
	})
}

func TestLegalCommentsAvoidSlashTagEndOfFile(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				//! <script>foo</script>
				export let x
			`,
			"/entry.css": `
				/*! <style>foo</style> */
				x { y: z }
			`,
		},
		entryPaths: []string{"/entry.js", "/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputDir:  "/out",
			LegalComments: config.LegalCommentsEndOfFile,
		},
	})
}

func TestLegalCommentsAvoidSlashTagExternal(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				//! <script>foo</script>
				export let x
			`,
			"/entry.css": `
				/*! <style>foo</style> */
				x { y: z }
			`,
		},
		entryPaths: []string{"/entry.js", "/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputDir:  "/out",
			LegalComments: config.LegalCommentsExternalWithoutComment,
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

func TestTopLevelAwaitIIFE(t *testing.T) {
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
			OutputFormat:  config.FormatIIFE,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `entry.js: ERROR: Top-level await is currently not supported with the "iife" output format
entry.js: ERROR: Top-level await is currently not supported with the "iife" output format
`,
	})
}

func TestTopLevelAwaitCJS(t *testing.T) {
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
			OutputFormat:  config.FormatCommonJS,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `entry.js: ERROR: Top-level await is currently not supported with the "cjs" output format
entry.js: ERROR: Top-level await is currently not supported with the "cjs" output format
`,
	})
}

func TestTopLevelAwaitESM(t *testing.T) {
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
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/out.js",
		},
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
		expectedScanLog: `entry.js: ERROR: Top-level await is currently not supported with the "cjs" output format
entry.js: ERROR: Top-level await is currently not supported with the "cjs" output format
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
		expectedScanLog: `entry.js: ERROR: Top-level await is currently not supported with the "iife" output format
entry.js: ERROR: Top-level await is currently not supported with the "iife" output format
`,
	})
}

func TestTopLevelAwaitForbiddenRequire(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				require('./a')
				require('./b')
				require('./c')
				require('./entry')
				await 0
			`,
			"/a.js": `
				import './b'
			`,
			"/b.js": `
				import './c'
			`,
			"/c.js": `
				await 0
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `entry.js: ERROR: This require call is not allowed because the transitive dependency "c.js" contains a top-level await
a.js: NOTE: The file "a.js" imports the file "b.js" here:
b.js: NOTE: The file "b.js" imports the file "c.js" here:
c.js: NOTE: The top-level await in "c.js" is here:
entry.js: ERROR: This require call is not allowed because the transitive dependency "c.js" contains a top-level await
b.js: NOTE: The file "b.js" imports the file "c.js" here:
c.js: NOTE: The top-level await in "c.js" is here:
entry.js: ERROR: This require call is not allowed because the imported file "c.js" contains a top-level await
c.js: NOTE: The top-level await in "c.js" is here:
entry.js: ERROR: This require call is not allowed because the imported file "entry.js" contains a top-level await
entry.js: NOTE: The top-level await in "entry.js" is here:
`,
	})
}

func TestTopLevelAwaitAllowedImportWithoutSplitting(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import('./a')
				import('./b')
				import('./c')
				import('./entry')
				await 0
			`,
			"/a.js": `
				import './b'
			`,
			"/b.js": `
				import './c'
			`,
			"/c.js": `
				await 0
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

func TestTopLevelAwaitAllowedImportWithSplitting(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import('./a')
				import('./b')
				import('./c')
				import('./entry')
				await 0
			`,
			"/a.js": `
				import './b'
			`,
			"/b.js": `
				import './c'
			`,
			"/c.js": `
				await 0
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatESModule,
			CodeSplitting: true,
			AbsOutputDir:  "/out",
		},
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
		expectedScanLog: `bad0.js: ERROR: Cannot assign to import "x"
NOTE: Imports are immutable in JavaScript. To modify the value of this import, you must export a setter function in the imported file (e.g. "setX") and then import and call that function here instead.
bad1.js: ERROR: Cannot assign to import "x"
NOTE: Imports are immutable in JavaScript. To modify the value of this import, you must export a setter function in the imported file (e.g. "setX") and then import and call that function here instead.
bad10.js: ERROR: Cannot assign to import "y z"
NOTE: Imports are immutable in JavaScript. To modify the value of this import, you must export a setter function in the imported file and then import and call that function here instead.
bad11.js: ERROR: Cannot assign to import "x"
NOTE: Imports are immutable in JavaScript. To modify the value of this import, you must export a setter function in the imported file (e.g. "setX") and then import and call that function here instead.
bad11.js: ERROR: Delete of a bare identifier cannot be used in strict mode
bad11.js: NOTE: This file is implicitly in strict mode because of the "import" keyword here:
bad12.js: ERROR: Cannot assign to import "x"
NOTE: Imports are immutable in JavaScript. To modify the value of this import, you must export a setter function in the imported file (e.g. "setX") and then import and call that function here instead.
bad12.js: ERROR: Delete of a bare identifier cannot be used in strict mode
bad12.js: NOTE: This file is implicitly in strict mode because of the "import" keyword here:
bad13.js: ERROR: Cannot assign to import "y"
NOTE: Imports are immutable in JavaScript. To modify the value of this import, you must export a setter function in the imported file (e.g. "setY") and then import and call that function here instead.
bad14.js: ERROR: Cannot assign to import "y"
NOTE: Imports are immutable in JavaScript. To modify the value of this import, you must export a setter function in the imported file (e.g. "setY") and then import and call that function here instead.
bad15.js: ERROR: Cannot assign to property on import "x"
NOTE: Imports are immutable in JavaScript. To modify the value of this import, you must export a setter function in the imported file and then import and call that function here instead.
bad2.js: ERROR: Cannot assign to import "x"
NOTE: Imports are immutable in JavaScript. To modify the value of this import, you must export a setter function in the imported file (e.g. "setX") and then import and call that function here instead.
bad3.js: ERROR: Cannot assign to import "x"
NOTE: Imports are immutable in JavaScript. To modify the value of this import, you must export a setter function in the imported file (e.g. "setX") and then import and call that function here instead.
bad4.js: ERROR: Cannot assign to import "x"
NOTE: Imports are immutable in JavaScript. To modify the value of this import, you must export a setter function in the imported file (e.g. "setX") and then import and call that function here instead.
bad5.js: ERROR: Cannot assign to import "x"
NOTE: Imports are immutable in JavaScript. To modify the value of this import, you must export a setter function in the imported file (e.g. "setX") and then import and call that function here instead.
bad6.js: ERROR: Cannot assign to import "x"
NOTE: Imports are immutable in JavaScript. To modify the value of this import, you must export a setter function in the imported file (e.g. "setX") and then import and call that function here instead.
bad7.js: ERROR: Cannot assign to import "y"
NOTE: Imports are immutable in JavaScript. To modify the value of this import, you must export a setter function in the imported file (e.g. "setY") and then import and call that function here instead.
bad8.js: ERROR: Cannot assign to property on import "x"
NOTE: Imports are immutable in JavaScript. To modify the value of this import, you must export a setter function in the imported file and then import and call that function here instead.
bad9.js: ERROR: Cannot assign to import "y"
NOTE: Imports are immutable in JavaScript. To modify the value of this import, you must export a setter function in the imported file (e.g. "setY") and then import and call that function here instead.
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
				import "./dup-case.js";        import "./node_modules/dup-case.js";        import "@plugin/dup-case.js"
				import "./not-in.js";          import "./node_modules/not-in.js";          import "@plugin/not-in.js"
				import "./not-instanceof.js";  import "./node_modules/not-instanceof.js";  import "@plugin/not-instanceof.js"
				import "./return-asi.js";      import "./node_modules/return-asi.js";      import "@plugin/return-asi.js"
				import "./bad-typeof.js";      import "./node_modules/bad-typeof.js";      import "@plugin/bad-typeof.js"
				import "./equals-neg-zero.js"; import "./node_modules/equals-neg-zero.js"; import "@plugin/equals-neg-zero.js"
				import "./equals-nan.js";      import "./node_modules/equals-nan.js";      import "@plugin/equals-nan.js"
				import "./equals-object.js";   import "./node_modules/equals-object.js";   import "@plugin/equals-object.js"
				import "./write-getter.js";    import "./node_modules/write-getter.js";    import "@plugin/write-getter.js"
				import "./read-setter.js";     import "./node_modules/read-setter.js";     import "@plugin/read-setter.js"
				import "./delete-super.js";    import "./node_modules/delete-super.js";    import "@plugin/delete-super.js"
			`,

			"/dup-case.js":                         "switch (x) { case 0: case 0: }",
			"/node_modules/dup-case.js":            "switch (x) { case 0: case 0: }",
			"/plugin-dir/node_modules/dup-case.js": "switch (x) { case 0: case 0: }",

			"/not-in.js":                         "!a in b",
			"/node_modules/not-in.js":            "!a in b",
			"/plugin-dir/node_modules/not-in.js": "!a in b",

			"/not-instanceof.js":                         "!a instanceof b",
			"/node_modules/not-instanceof.js":            "!a instanceof b",
			"/plugin-dir/node_modules/not-instanceof.js": "!a instanceof b",

			"/return-asi.js":                         "return\n123",
			"/node_modules/return-asi.js":            "return\n123",
			"/plugin-dir/node_modules/return-asi.js": "return\n123",

			"/bad-typeof.js":                         "typeof x == 'null'",
			"/node_modules/bad-typeof.js":            "typeof x == 'null'",
			"/plugin-dir/node_modules/bad-typeof.js": "typeof x == 'null'",

			"/equals-neg-zero.js":                         "x === -0",
			"/node_modules/equals-neg-zero.js":            "x === -0",
			"/plugin-dir/node_modules/equals-neg-zero.js": "x === -0",

			"/equals-nan.js":                         "x === NaN",
			"/node_modules/equals-nan.js":            "x === NaN",
			"/plugin-dir/node_modules/equals-nan.js": "x === NaN",

			"/equals-object.js":                         "x === []",
			"/node_modules/equals-object.js":            "x === []",
			"/plugin-dir/node_modules/equals-object.js": "x === []",

			"/write-getter.js":                         "class Foo { get #foo() {} foo() { this.#foo = 123 } }",
			"/node_modules/write-getter.js":            "class Foo { get #foo() {} foo() { this.#foo = 123 } }",
			"/plugin-dir/node_modules/write-getter.js": "class Foo { get #foo() {} foo() { this.#foo = 123 } }",

			"/read-setter.js":                         "class Foo { set #foo(x) {} foo() { return this.#foo } }",
			"/node_modules/read-setter.js":            "class Foo { set #foo(x) {} foo() { return this.#foo } }",
			"/plugin-dir/node_modules/read-setter.js": "class Foo { set #foo(x) {} foo() { return this.#foo } }",

			"/delete-super.js":                         "class Foo extends Bar { foo() { delete super.foo } }",
			"/node_modules/delete-super.js":            "class Foo extends Bar { foo() { delete super.foo } }",
			"/plugin-dir/node_modules/delete-super.js": "class Foo extends Bar { foo() { delete super.foo } }",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			Plugins: []config.Plugin{{
				OnResolve: []config.OnResolve{
					{
						Filter: regexp.MustCompile("^@plugin/"),
						Callback: func(args config.OnResolveArgs) config.OnResolveResult {
							return config.OnResolveResult{
								Path: logger.Path{
									Text:      strings.Replace(args.Path, "@plugin/", "/plugin-dir/node_modules/", 1),
									Namespace: "file",
								},
							}
						},
					},
				},
			}},
		},
		expectedScanLog: `bad-typeof.js: WARNING: The "typeof" operator will never evaluate to "null"
NOTE: The expression "typeof x" actually evaluates to "object" in JavaScript, not "null". You need to use "x === null" to test for null.
delete-super.js: WARNING: Attempting to delete a property of "super" will throw a ReferenceError
dup-case.js: WARNING: This case clause will never be evaluated because it duplicates an earlier case clause
dup-case.js: NOTE: The earlier case clause is here:
equals-nan.js: WARNING: Comparison with NaN using the "===" operator here is always false
NOTE: Floating-point equality is defined such that NaN is never equal to anything, so "x === NaN" always returns false. You need to use "isNaN(x)" instead to test for NaN.
equals-neg-zero.js: WARNING: Comparison with -0 using the "===" operator will also match 0
NOTE: Floating-point equality is defined such that 0 and -0 are equal, so "x === -0" returns true for both 0 and -0. You need to use "Object.is(x, -0)" instead to test for -0.
equals-object.js: WARNING: Comparison using the "===" operator here is always false
NOTE: Equality with a new object is always false in JavaScript because the equality operator tests object identity. You need to write code to compare the contents of the object instead. For example, use "Array.isArray(x) && x.length === 0" instead of "x === []" to test for an empty array.
not-in.js: WARNING: Suspicious use of the "!" operator inside the "in" operator
NOTE: The code "!x in y" is parsed as "(!x) in y". You need to insert parentheses to get "!(x in y)" instead.
not-instanceof.js: WARNING: Suspicious use of the "!" operator inside the "instanceof" operator
NOTE: The code "!x instanceof y" is parsed as "(!x) instanceof y". You need to insert parentheses to get "!(x instanceof y)" instead.
read-setter.js: WARNING: Reading from setter-only property "#foo" will throw
return-asi.js: WARNING: The following expression is not returned because of an automatically-inserted semicolon
write-getter.js: WARNING: Writing to getter-only property "#foo" will throw
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
			Platform:      config.PlatformNode,
			OutputFormat:  config.FormatCommonJS,
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
		expectedScanLog: `entry.js: WARNING: "./present-file" should be marked as external for use with "require.resolve"
entry.js: WARNING: "./missing-file" should be marked as external for use with "require.resolve"
entry.js: WARNING: "missing-pkg" should be marked as external for use with "require.resolve"
entry.js: WARNING: "@scope/missing-pkg" should be marked as external for use with "require.resolve"
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
		expectedScanLog: `ERROR: Could not read from file: /inject.js
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
		expectedScanLog: `ERROR: Duplicate injected file "inject.js"
`,
	})
}

func TestInject(t *testing.T) {
	defines := config.ProcessDefines(map[string]config.DefineData{
		"chain.prop": {
			DefineFunc: func(args config.DefineArgs) js_ast.E {
				return &js_ast.EIdentifier{Ref: args.FindSymbol(args.Loc, "replace")}
			},
		},
		"obj.defined": {
			DefineFunc: func(args config.DefineArgs) js_ast.E {
				return &js_ast.EString{Value: js_lexer.StringToUTF16("defined")}
			},
		},
		"injectedAndDefined": {
			DefineFunc: func(args config.DefineArgs) js_ast.E {
				return &js_ast.EString{Value: js_lexer.StringToUTF16("should be used")}
			},
		},
	})
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				let sideEffects = console.log('this should be renamed')
				let collide = 123
				console.log(obj.prop)
				console.log(obj.defined)
				console.log(injectedAndDefined)
				console.log(chain.prop.test)
				console.log(collide)
				console.log(re_export)
			`,
			"/inject.js": `
				export let obj = {}
				export let sideEffects = console.log('side effects')
				export let noSideEffects = /* @__PURE__ */ console.log('side effects')
				export let injectedAndDefined = 'should not be used'
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
			DefineFunc: func(args config.DefineArgs) js_ast.E {
				return &js_ast.EIdentifier{Ref: args.FindSymbol(args.Loc, "replace")}
			},
		},
		"obj.defined": {
			DefineFunc: func(args config.DefineArgs) js_ast.E {
				return &js_ast.EString{Value: js_lexer.StringToUTF16("defined")}
			},
		},
		"injectedAndDefined": {
			DefineFunc: func(args config.DefineArgs) js_ast.E {
				return &js_ast.EString{Value: js_lexer.StringToUTF16("should be used")}
			},
		},
	})
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				let sideEffects = console.log('this should be renamed')
				let collide = 123
				console.log(obj.prop)
				console.log(obj.defined)
				console.log(injectedAndDefined)
				console.log(chain.prop.test)
				console.log(collide)
				console.log(re_export)
			`,
			"/inject.js": `
				export let obj = {}
				export let sideEffects = console.log('side effects')
				export let noSideEffects = /* @__PURE__ */ console.log('side effects')
				export let injectedAndDefined = 'should not be used'
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
			TreeShaking:   true,
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
			DefineFunc: func(args config.DefineArgs) js_ast.E {
				return &js_ast.EIdentifier{Ref: args.FindSymbol(args.Loc, "el")}
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

func TestInjectAssign(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				test = true
			`,
			"/inject.js": `
				export let test = false
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			InjectAbsPaths: []string{
				"/inject.js",
			},
		},
		expectedScanLog: `entry.js: ERROR: Cannot assign to "test" because it's an import from an injected file
inject.js: NOTE: The symbol "test" was exported from "inject.js" here:
`,
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

func TestDefineImportMeta(t *testing.T) {
	defines := config.ProcessDefines(map[string]config.DefineData{
		"import.meta": {
			DefineFunc: func(args config.DefineArgs) js_ast.E {
				return &js_ast.ENumber{Value: 1}
			},
		},
		"import.meta.foo": {
			DefineFunc: func(args config.DefineArgs) js_ast.E {
				return &js_ast.ENumber{Value: 2}
			},
		},
		"import.meta.foo.bar": {
			DefineFunc: func(args config.DefineArgs) js_ast.E {
				return &js_ast.ENumber{Value: 3}
			},
		},
	})
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(
					// These should be fully substituted
					import.meta,
					import.meta.foo,
					import.meta.foo.bar,

					// Should just substitute "import.meta.foo"
					import.meta.foo.baz,

					// This should not be substituted
					import.meta.bar,
				)
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

func TestDefineImportMetaES5(t *testing.T) {
	defines := config.ProcessDefines(map[string]config.DefineData{
		"import.meta.x": {
			DefineFunc: func(args config.DefineArgs) js_ast.E {
				return &js_ast.ENumber{Value: 1}
			},
		},
	})
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/replaced.js": `
				console.log(import.meta.x)
			`,
			"/kept.js": `
				console.log(import.meta.y)
			`,
			"/dead-code.js": `
				var x = () => console.log(import.meta.z)
			`,
		},
		entryPaths: []string{
			"/replaced.js",
			"/kept.js",
			"/dead-code.js",
		},
		options: config.Options{
			Mode:                  config.ModeBundle,
			AbsOutputDir:          "/out",
			Defines:               &defines,
			UnsupportedJSFeatures: compat.ImportMeta,
		},
		expectedScanLog: `dead-code.js: WARNING: "import.meta" is not available in the configured target environment and will be empty
kept.js: WARNING: "import.meta" is not available in the configured target environment and will be empty
`,
	})
}

func TestDefineThis(t *testing.T) {
	defines := config.ProcessDefines(map[string]config.DefineData{
		"this": {
			DefineFunc: func(args config.DefineArgs) js_ast.E {
				return &js_ast.ENumber{Value: 1}
			},
		},
		"this.foo": {
			DefineFunc: func(args config.DefineArgs) js_ast.E {
				return &js_ast.ENumber{Value: 2}
			},
		},
		"this.foo.bar": {
			DefineFunc: func(args config.DefineArgs) js_ast.E {
				return &js_ast.ENumber{Value: 3}
			},
		},
	})
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				ok(
					// These should be fully substituted
					this,
					this.foo,
					this.foo.bar,

					// Should just substitute "this.foo"
					this.foo.baz,

					// This should not be substituted
					this.bar,
				);

				// This code should be the same as above
				(() => {
					ok(
						this,
						this.foo,
						this.foo.bar,
						this.foo.baz,
						this.bar,
					);
				})();

				// Nothing should be substituted in this code
				(function() {
					doNotSubstitute(
						this,
						this.foo,
						this.foo.bar,
						this.foo.baz,
						this.bar,
					);
				})();
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
		expectedScanLog: `Users/user/project/src/entry.js: ERROR: Could not resolve "some/other/file"
NOTE: Use the relative path "./some/other/file" to reference the file "Users/user/project/src/some/other/file.js". Without the leading "./", the path "some/other/file" is being interpreted as a package path instead.
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
		expectedScanLog: `entry.js: ERROR: Cannot assign to "x" because it is a constant
entry.js: NOTE: The symbol "x" was declared a constant here:
`,
	})
}

func TestConstWithLet(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const a = 1; console.log(a)
				if (true) { const b = 2; console.log(b) }
				if (true) { const b = 2; unknownFn(b) }
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
				if (true) { const b = 2; unknownFn(b) }
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

func TestRequireMainCacheCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log('is main:', require.main === module)
				console.log(require('./is-main'))
				console.log('cache:', require.cache);
			`,
			"/is-main.js": `
				module.exports = require.main === module
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			Platform:      config.PlatformNode,
			AbsOutputFile: "/out.js",
			OutputFormat:  config.FormatCommonJS,
		},
	})
}

func TestExternalES6ConvertedToCommonJS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				require('./a')
				require('./b')
				require('./c')
				require('./d')
				require('./e')
			`,
			"/a.js": `
				import * as ns from 'x'
				export {ns}
			`,
			"/b.js": `
				import * as ns from 'x' // "ns" must be renamed to avoid collisions with "a.js"
				export {ns}
			`,
			"/c.js": `
				export * as ns from 'x'
			`,
			"/d.js": `
				export {ns} from 'x'
			`,
			"/e.js": `
				export * from 'x'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			OutputFormat:  config.FormatESModule,
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"x": true,
				},
			},
		},
	})
}

func TestCallImportNamespaceWarning(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/js.js": `
				import * as a from "a"
				import {b} from "b"
				import c from "c"
				a()
				b()
				c()
				new a()
				new b()
				new c()
			`,
			"/ts.ts": `
				import * as a from "a"
				import {b} from "b"
				import c from "c"
				a()
				b()
				c()
				new a()
				new b()
				new c()
			`,
			"/jsx-components.jsx": `
				import * as A from "a"
				import {B} from "b"
				import C from "c"
				<A/>;
				<B/>;
				<C/>;
			`,
			"/jsx-a.jsx": `
				// @jsx a
				import * as a from "a"
				<div/>
			`,
			"/jsx-b.jsx": `
				// @jsx b
				import {b} from "b"
				<div/>
			`,
			"/jsx-c.jsx": `
				// @jsx c
				import c from "c"
				<div/>
			`,
		},
		entryPaths: []string{
			"/js.js",
			"/ts.ts",
			"/jsx-components.jsx",
			"/jsx-a.jsx",
			"/jsx-b.jsx",
			"/jsx-c.jsx",
		},
		options: config.Options{
			Mode:         config.ModeConvertFormat,
			AbsOutputDir: "/out",
			OutputFormat: config.FormatESModule,
		},
		expectedScanLog: `js.js: WARNING: Calling "a" will crash at run-time because it's an import namespace object, not a function
js.js: NOTE: Consider changing "a" to a default import instead:
js.js: WARNING: Constructing "a" will crash at run-time because it's an import namespace object, not a constructor
js.js: NOTE: Consider changing "a" to a default import instead:
jsx-a.jsx: WARNING: Calling "a" will crash at run-time because it's an import namespace object, not a function
jsx-a.jsx: NOTE: Consider changing "a" to a default import instead:
jsx-components.jsx: WARNING: Using "A" in a JSX expression will crash at run-time because it's an import namespace object, not a component
jsx-components.jsx: NOTE: Consider changing "A" to a default import instead:
ts.ts: WARNING: Calling "a" will crash at run-time because it's an import namespace object, not a function
ts.ts: NOTE: Consider changing "a" to a default import instead:
NOTE: Make sure to enable TypeScript's "esModuleInterop" setting so that TypeScript's type checker generates an error when you try to do this. You can read more about this setting here: https://www.typescriptlang.org/tsconfig#esModuleInterop
ts.ts: WARNING: Constructing "a" will crash at run-time because it's an import namespace object, not a constructor
ts.ts: NOTE: Consider changing "a" to a default import instead:
NOTE: Make sure to enable TypeScript's "esModuleInterop" setting so that TypeScript's type checker generates an error when you try to do this. You can read more about this setting here: https://www.typescriptlang.org/tsconfig#esModuleInterop
`,
	})
}

func TestBundlingFilesOutsideOfOutbase(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/entry.js": `
				console.log('test')
			`,
		},
		entryPaths: []string{"/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputBase: "/some/nested/directory",
			AbsOutputDir:  "/out",
		},
	})
}

var relocateFiles = map[string]string{
	"/top-level.js": `
		var a;
		for (var b; 0;);
		for (var { c, x: [d] } = {}; 0;);
		for (var e of []);
		for (var { f, x: [g] } of []);
		for (var h in {});
		for (var i = 1 in {});
		for (var { j, x: [k] } in {});
		function l() {}
	`,
	"/nested.js": `
		if (true) {
			var a;
			for (var b; 0;);
			for (var { c, x: [d] } = {}; 0;);
			for (var e of []);
			for (var { f, x: [g] } of []);
			for (var h in {});
			for (var i = 1 in {});
			for (var { j, x: [k] } in {});
			function l() {}
		}
	`,
	"/let.js": `
		if (true) {
			let a;
			for (let b; 0;);
			for (let { c, x: [d] } = {}; 0;);
			for (let e of []);
			for (let { f, x: [g] } of []);
			for (let h in {});
			// for (let i = 1 in {});
			for (let { j, x: [k] } in {});
		}
	`,
	"/function.js": `
		function x() {
			var a;
			for (var b; 0;);
			for (var { c, x: [d] } = {}; 0;);
			for (var e of []);
			for (var { f, x: [g] } of []);
			for (var h in {});
			for (var i = 1 in {});
			for (var { j, x: [k] } in {});
			function l() {}
		}
		x()
	`,
	"/function-nested.js": `
		function x() {
			if (true) {
				var a;
				for (var b; 0;);
				for (var { c, x: [d] } = {}; 0;);
				for (var e of []);
				for (var { f, x: [g] } of []);
				for (var h in {});
				for (var i = 1 in {});
				for (var { j, x: [k] } in {});
				function l() {}
			}
		}
		x()
	`,
}

var relocateEntries = []string{
	"/top-level.js",
	"/nested.js",
	"/let.js",
	"/function.js",
	"/function-nested.js",
}

func TestVarRelocatingBundle(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files:      relocateFiles,
		entryPaths: relocateEntries,
		options: config.Options{
			Mode:         config.ModeBundle,
			OutputFormat: config.FormatESModule,
			AbsOutputDir: "/out",
		},
	})
}

func TestVarRelocatingNoBundle(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files:      relocateFiles,
		entryPaths: relocateEntries,
		options: config.Options{
			Mode:         config.ModeConvertFormat,
			OutputFormat: config.FormatESModule,
			AbsOutputDir: "/out",
		},
	})
}

func TestImportNamespaceThisValue(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import def, * as ns from 'external'
				console.log(ns[foo](), new ns[foo]())
			`,
			"/b.js": `
				import def, * as ns from 'external'
				console.log(ns.foo(), new ns.foo())
			`,
			"/c.js": `
				import def, {foo} from 'external'
				console.log(def(), foo())
				console.log(new def(), new foo())
			`,
		},
		entryPaths: []string{"/a.js", "/b.js", "/c.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			OutputFormat: config.FormatCommonJS,
			AbsOutputDir: "/out",
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"external": true,
				},
			},
		},
	})
}

func TestThisUndefinedWarningESM(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import x from './file1.js'
				import y from 'pkg/file2.js'
				console.log(x, y)
			`,
			"/file1.js": `
				export default [this, this]
			`,
			"/node_modules/pkg/file2.js": `
				export default [this, this]
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
		expectedScanLog: `file1.js: WARNING: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
file1.js: NOTE: This file is considered an ECMAScript module because of the "export" keyword here:
`,
	})
}

func TestQuotedProperty(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from 'ext'
				console.log(ns.mustBeUnquoted, ns['mustBeQuoted'])
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			OutputFormat: config.FormatCommonJS,
			AbsOutputDir: "/out",
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"ext": true,
				},
			},
		},
	})
}

func TestQuotedPropertyMangle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from 'ext'
				console.log(ns.mustBeUnquoted, ns['mustBeUnquoted2'])
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			OutputFormat: config.FormatCommonJS,
			AbsOutputDir: "/out",
			MangleSyntax: true,
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"ext": true,
				},
			},
		},
	})
}

func TestDuplicatePropertyWarning(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import './outside-node-modules'
				import 'inside-node-modules'
			`,
			"/outside-node-modules/index.js": `
				console.log({ a: 1, a: 2 })
			`,
			"/outside-node-modules/package.json": `
				{ "b": 1, "b": 2 }
			`,
			"/node_modules/inside-node-modules/index.js": `
				console.log({ c: 1, c: 2 })
			`,
			"/node_modules/inside-node-modules/package.json": `
				{ "d": 1, "d": 2 }
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
		expectedScanLog: `outside-node-modules/index.js: WARNING: Duplicate key "a" in object literal
outside-node-modules/index.js: NOTE: The original key "a" is here:
outside-node-modules/package.json: WARNING: Duplicate key "b" in object literal
outside-node-modules/package.json: NOTE: The original key "b" is here:
`,
	})
}

func TestRequireShimSubstitution(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log([
					require,
					typeof require,
					require('./example.json'),
					require('./example.json', { type: 'json' }),
					require(window.SOME_PATH),
					module.require('./example.json'),
					module.require('./example.json', { type: 'json' }),
					module.require(window.SOME_PATH),
					require.resolve('some-path'),
					require.resolve(window.SOME_PATH),
					import('some-path'),
					import(window.SOME_PATH),
				])
			`,
			"/example.json": `{ "works": true }`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"some-path": true,
				},
			},
			UnsupportedJSFeatures: compat.DynamicImport,
		},
	})
}

// This guards against a bad interaction between the strict mode nested function
// declarations, name keeping, and initialized variable inlining. See this issue
// for full context: https://github.com/evanw/esbuild/issues/1552.
func TestStrictModeNestedFnDeclKeepNamesVariableInliningIssue1552(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export function outer() {
					{
						function inner() {
							return Math.random();
						}
						const x = inner();
						console.log(x);
					}
				}
				outer();
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModePassThrough,
			AbsOutputDir: "/out",
			KeepNames:    true,
			MangleSyntax: true,
		},
	})
}

func TestBuiltInNodeModulePrecedence(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log([
					// These are node core modules
					require('fs'),
					require('fs/promises'),
					require('node:foo'),

					// These are not node core modules
					require('fs/abc'),
					require('fs/'),
				])
			`,
			"/node_modules/fs/abc.js": `
				console.log('include this')
			`,
			"/node_modules/fs/index.js": `
				console.log('include this too')
			`,
			"/node_modules/fs/promises.js": `
				throw 'DO NOT INCLUDE THIS'
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			Platform:     config.PlatformNode,
			OutputFormat: config.FormatCommonJS,
			AbsOutputDir: "/out",
		},
	})
}

func TestEntryNamesNoSlashAfterDir(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/app1/main.ts": `console.log(1)`,
			"/src/app2/main.ts": `console.log(2)`,
			"/src/app3/main.ts": `console.log(3)`,
		},
		entryPathsAdvanced: []EntryPoint{
			{InputPath: "/src/app1/main.ts"},
			{InputPath: "/src/app2/main.ts"},
			{InputPath: "/src/app3/main.ts", OutputPath: "customPath"},
		},
		options: config.Options{
			Mode: config.ModePassThrough,
			EntryPathTemplate: []config.PathTemplate{
				// "[dir]-[name]"
				{Data: "./", Placeholder: config.DirPlaceholder},
				{Data: "-", Placeholder: config.NamePlaceholder},
			},
			AbsOutputDir: "/out",
		},
	})
}

func TestEntryNamesNonPortableCharacter(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry1-*.ts": `console.log(1)`,
			"/entry2-*.ts": `console.log(2)`,
		},
		entryPathsAdvanced: []EntryPoint{
			// The "*" should turn into "_" for cross-platform Windows portability
			{InputPath: "/entry1-*.ts"},

			// The "*" should be preserved since the user _really_ wants it
			{InputPath: "/entry2-*.ts", OutputPath: "entry2-*"},
		},
		options: config.Options{
			Mode:         config.ModePassThrough,
			AbsOutputDir: "/out",
		},
	})
}

func TestEntryNamesChunkNamesExtPlaceholder(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/entries/entry1.js":  `import "../lib/shared.js"; import "./entry1.css"; console.log('entry1')`,
			"/src/entries/entry2.js":  `import "../lib/shared.js"; import "./entry2.css"; console.log('entry2')`,
			"/src/entries/entry1.css": `a:after { content: "entry1" }`,
			"/src/entries/entry2.css": `a:after { content: "entry2" }`,
			"/src/lib/shared.js":      `console.log('shared')`,
		},
		entryPaths: []string{
			"/src/entries/entry1.js",
			"/src/entries/entry2.js",
		},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputBase: "/src",
			AbsOutputDir:  "/out",
			CodeSplitting: true,
			EntryPathTemplate: []config.PathTemplate{
				{Data: "main/", Placeholder: config.ExtPlaceholder},
				{Data: "/", Placeholder: config.NamePlaceholder},
				{Data: "-", Placeholder: config.HashPlaceholder},
			},
			ChunkPathTemplate: []config.PathTemplate{
				{Data: "common/", Placeholder: config.ExtPlaceholder},
				{Data: "/", Placeholder: config.NamePlaceholder},
				{Data: "-", Placeholder: config.HashPlaceholder},
			},
		},
	})
}

func TestMinifyIdentifiersImportPathFrequencyAnalysis(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/import.js": `
				import foo from "./WWWWWWWWWWXXXXXXXXXXYYYYYYYYYYZZZZZZZZZZ"
				console.log(foo, 'no identifier in this file should be named W, X, Y, or Z')
			`,
			"/WWWWWWWWWWXXXXXXXXXXYYYYYYYYYYZZZZZZZZZZ.js": `export default 123`,

			"/require.js": `
				const foo = require("./AAAAAAAAAABBBBBBBBBBCCCCCCCCCCDDDDDDDDDD")
				console.log(foo, 'no identifier in this file should be named A, B, C, or D')
			`,
			"/AAAAAAAAAABBBBBBBBBBCCCCCCCCCCDDDDDDDDDD.js": `module.exports = 123`,
		},
		entryPaths: []string{
			"/import.js",
			"/require.js",
		},
		options: config.Options{
			Mode:              config.ModeBundle,
			AbsOutputDir:      "/out",
			RemoveWhitespace:  true,
			MinifyIdentifiers: true,
		},
	})
}

func TestToESMWrapperOmission(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import 'a_nowrap'

				import { b } from 'b_nowrap'
				b()

				export * from 'c_nowrap'

				import * as d from 'd_WRAP'
				x = d.x

				import e from 'e_WRAP'
				e()

				import { default as f } from 'f_WRAP'
				f()

				import { __esModule as g } from 'g_WRAP'
				g()

				import * as h from 'h_WRAP'
				x = h

				import * as i from 'i_WRAP'
				i.x()

				import * as j from 'j_WRAP'
				j.x` + "``" + `

				x = import("k_WRAP")
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:                  config.ModeConvertFormat,
			OutputFormat:          config.FormatCommonJS,
			AbsOutputDir:          "/out",
			UnsupportedJSFeatures: compat.DynamicImport,
		},
	})
}

// This is coverage for a past bug in esbuild. We used to generate this, which is wrong:
//
//   let x = function(foo) {
//     var foo2;
//     return foo2;
//   };
//
func TestNamedFunctionExpressionArgumentCollision(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				let x = function foo(foo) {
					var foo;
					return foo;
				}
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
