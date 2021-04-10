package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

var ts_suite = suite{
	name: "ts",
}

func TestTSDeclareConst(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare const require: any
				declare const exports: any;
				declare const module: any

				declare const foo: any
				let foo = bar()
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSDeclareLet(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare let require: any
				declare let exports: any;
				declare let module: any

				declare let foo: any
				let foo = bar()
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSDeclareVar(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare var require: any
				declare var exports: any;
				declare var module: any

				declare var foo: any
				let foo = bar()
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSDeclareClass(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare class require {}
				declare class exports {};
				declare class module {}

				declare class foo {}
				let foo = bar()
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSDeclareFunction(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare function require(): void
				declare function exports(): void;
				declare function module(): void

				declare function foo() {}
				let foo = bar()
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSDeclareNamespace(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare namespace require {}
				declare namespace exports {};
				declare namespace module {}

				declare namespace foo {}
				let foo = bar()
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSDeclareEnum(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare enum require {}
				declare enum exports {};
				declare enum module {}

				declare enum foo {}
				let foo = bar()
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSDeclareConstEnum(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare const enum require {}
				declare const enum exports {};
				declare const enum module {}

				declare const enum foo {}
				let foo = bar()
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportEmptyNamespace(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {ns} from './ns.ts'
				function foo(): ns.type {}
				foo();
			`,
			"/ns.ts": `
				export namespace ns {}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportMissingES6(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import fn, {x as a, y as b} from './foo'
				console.log(fn(a, b))
			`,
			"/foo.js": `
				export const x = 123
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedCompileLog: `entry.ts: error: No matching export in "foo.js" for import "default"
entry.ts: error: No matching export in "foo.js" for import "y"
`,
	})
}

func TestTSImportMissingUnusedES6(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import fn, {x as a, y as b} from './foo'
			`,
			"/foo.js": `
				export const x = 123
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSExportMissingES6(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				console.log(ns)
			`,
			"/foo.ts": `
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
	})
}

// It's an error to import from a file that does not exist
func TestTSImportMissingFile(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {Something} from './doesNotExist.ts'
				let foo = new Something
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `entry.ts: error: Could not resolve "./doesNotExist.ts"
`,
	})
}

// It's not an error to import a type from a file that does not exist
func TestTSImportTypeOnlyFile(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {SomeType1} from './doesNotExist1.ts'
				import {SomeType2} from './doesNotExist2.ts'
				let foo: SomeType1 = bar()
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSExportEquals(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.ts": `
				import b from './b.ts'
				console.log(b)
			`,
			"/b.ts": `
				export = [123, foo]
				function foo() {}
			`,
		},
		entryPaths: []string{"/a.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSExportNamespace(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.ts": `
				import {Foo} from './b.ts'
				console.log(new Foo)
			`,
			"/b.ts": `
				export class Foo {}
				export namespace Foo {
					export let foo = 1
				}
				export namespace Foo {
					export let bar = 2
				}
			`,
		},
		entryPaths: []string{"/a.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSMinifyEnum(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.ts": `
				enum Foo { A, B, C = Foo }
			`,
			"/b.ts": `
				export enum Foo { X, Y, Z = Foo }
			`,
		},
		entryPaths: []string{"/a.ts", "/b.ts"},
		options: config.Options{
			MangleSyntax:      true,
			RemoveWhitespace:  true,
			MinifyIdentifiers: true,
			AbsOutputDir:      "/",
		},
	})
}

func TestTSMinifyNamespace(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.ts": `
				namespace Foo {
					export namespace Bar {
						foo(Foo, Bar)
					}
				}
			`,
			"/b.ts": `
				export namespace Foo {
					export namespace Bar {
						foo(Foo, Bar)
					}
				}
			`,
		},
		entryPaths: []string{"/a.ts", "/b.ts"},
		options: config.Options{
			MangleSyntax:      true,
			RemoveWhitespace:  true,
			MinifyIdentifiers: true,
			AbsOutputDir:      "/",
		},
	})
}

func TestTSMinifyDerivedClass(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class Foo extends Bar {
					foo = 1;
					bar = 2;
					constructor() {
						super();
						foo();
						bar();
					}
				}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			MangleSyntax:          true,
			UnsupportedJSFeatures: es(2015),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestTSImportVsLocalCollisionAllTypes(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {a, b, c, d, e} from './other.ts'
				let a
				const b = 0
				var c
				function d() {}
				class e {}
				console.log(a, b, c, d, e)
			`,
			"/other.ts": `
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportVsLocalCollisionMixed(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {a, b, c, d, e, real} from './other.ts'
				let a
				const b = 0
				var c
				function d() {}
				class e {}
				console.log(a, b, c, d, e, real)
			`,
			"/other.ts": `
				export let real = 123
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSMinifiedBundleES6(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {foo} from './a'
				console.log(foo())
			`,
			"/a.ts": `
				export function foo() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:              config.ModeBundle,
			MangleSyntax:      true,
			RemoveWhitespace:  true,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
	})
}

func TestTSMinifiedBundleCommonJS(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				const {foo} = require('./a')
				console.log(foo(), require('./j.json'))
			`,
			"/a.ts": `
				exports.foo = function() {
					return 123
				}
			`,
			"/j.json": `
				{"test": true}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:              config.ModeBundle,
			MangleSyntax:      true,
			RemoveWhitespace:  true,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
	})
}

func TestTypeScriptDecorators(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import all from './all'
				import all_computed from './all_computed'
				import {a} from './a'
				import {b} from './b'
				import {c} from './c'
				import {d} from './d'
				import e from './e'
				import f from './f'
				import g from './g'
				import h from './h'
				import {i} from './i'
				import {j} from './j'
				import k from './k'
				import {fn} from './arguments'
				console.log(all, all_computed, a, b, c, d, e, f, g, h, i, j, k, fn)
			`,
			"/all.ts": `
				@x.y()
				@new y.x()
				export default class Foo {
					@x @y mUndef
					@x @y mDef = 1
					constructor(@x0 @y0 arg0, @x1 @y1 arg1) {}
					@x @y method(@x0 @y0 arg0, @x1 @y1 arg1) { return new Foo }
					@x @y static sUndef
					@x @y static sDef = new Foo
					@x @y static sMethod(@x0 @y0 arg0, @x1 @y1 arg1) { return new Foo }
				}
			`,
			"/all_computed.ts": `
				@x?.[_ + 'y']()
				@new y?.[_ + 'x']()
				export default class Foo {
					@x @y [mUndef()]
					@x @y [mDef()] = 1
					@x @y [method()](@x0 @y0 arg0, @x1 @y1 arg1) { return new Foo }

					// Side effect order must be preserved even for fields without decorators
					[xUndef()]
					[xDef()] = 2
					static [yUndef()]
					static [yDef()] = 3

					@x @y static [sUndef()]
					@x @y static [sDef()] = new Foo
					@x @y static [sMethod()](@x0 @y0 arg0, @x1 @y1 arg1) { return new Foo }
				}
			`,
			"/a.ts": `
				@x(() => 0) @y(() => 1)
				class a_class {
					fn() { return new a_class }
					static z = new a_class
				}
				export let a = a_class
			`,
			"/b.ts": `
				@x(() => 0) @y(() => 1)
				abstract class b_class {
					fn() { return new b_class }
					static z = new b_class
				}
				export let b = b_class
			`,
			"/c.ts": `
				@x(() => 0) @y(() => 1)
				export class c {
					fn() { return new c }
					static z = new c
				}
			`,
			"/d.ts": `
				@x(() => 0) @y(() => 1)
				export abstract class d {
					fn() { return new d }
					static z = new d
				}
			`,
			"/e.ts": `
				@x(() => 0) @y(() => 1)
				export default class {}
			`,
			"/f.ts": `
				@x(() => 0) @y(() => 1)
				export default class f {
					fn() { return new f }
					static z = new f
				}
			`,
			"/g.ts": `
				@x(() => 0) @y(() => 1)
				export default abstract class {}
			`,
			"/h.ts": `
				@x(() => 0) @y(() => 1)
				export default abstract class h {
					fn() { return new h }
					static z = new h
				}
			`,
			"/i.ts": `
				class i_class {
					@x(() => 0) @y(() => 1)
					foo
				}
				export let i = i_class
			`,
			"/j.ts": `
				export class j {
					@x(() => 0) @y(() => 1)
					foo() {}
				}
			`,
			"/k.ts": `
				export default class {
					foo(@x(() => 0) @y(() => 1) x) {}
				}
			`,
			"/arguments.ts": `
				function dec(x: any): any {}
				export function fn(x: string): any {
					class Foo {
						@dec(arguments[0])
						[arguments[0]]() {}
					}
					return Foo;
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

func TestTSExportDefaultTypeIssue316(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import dc_def, { bar as dc } from './keep/declare-class'
				import dl_def, { bar as dl } from './keep/declare-let'
				import im_def, { bar as im } from './keep/interface-merged'
				import in_def, { bar as _in } from './keep/interface-nested'
				import tn_def, { bar as tn } from './keep/type-nested'
				import vn_def, { bar as vn } from './keep/value-namespace'
				import vnm_def, { bar as vnm } from './keep/value-namespace-merged'

				import i_def, { bar as i } from './remove/interface'
				import ie_def, { bar as ie } from './remove/interface-exported'
				import t_def, { bar as t } from './remove/type'
				import te_def, { bar as te } from './remove/type-exported'
				import ton_def, { bar as ton } from './remove/type-only-namespace'
				import tone_def, { bar as tone } from './remove/type-only-namespace-exported'

				export default [
					dc,
					dl,
					im,
					_in,
					tn,
					vn,
					vnm,

					i,
					ie,
					t,
					te,
					ton,
					tone,
				]
			`,
			"/keep/declare-class.ts": `
				declare class foo {}
				export default foo
				export let bar = 123
			`,
			"/keep/declare-let.ts": `
				declare let foo: number
				export default foo
				export let bar = 123
			`,
			"/keep/interface-merged.ts": `
				class foo {
					static x = new foo
				}
				interface foo {}
				export default foo
				export let bar = 123
			`,
			"/keep/interface-nested.ts": `
				if (true) {
					interface foo {}
				}
				export default foo
				export let bar = 123
			`,
			"/keep/type-nested.ts": `
				if (true) {
					type foo = number
				}
				export default foo
				export let bar = 123
			`,
			"/keep/value-namespace.ts": `
				namespace foo {
					export let num = 0
				}
				export default foo
				export let bar = 123
			`,
			"/keep/value-namespace-merged.ts": `
				namespace foo {
					export type num = number
				}
				namespace foo {
					export let num = 0
				}
				export default foo
				export let bar = 123
			`,
			"/remove/interface.ts": `
				interface foo { }
				export default foo
				export let bar = 123
			`,
			"/remove/interface-exported.ts": `
				export interface foo { }
				export default foo
				export let bar = 123
			`,
			"/remove/type.ts": `
				type foo = number
				export default foo
				export let bar = 123
			`,
			"/remove/type-exported.ts": `
				export type foo = number
				export default foo
				export let bar = 123
			`,
			"/remove/type-only-namespace.ts": `
				namespace foo {
					export type num = number
				}
				export default foo
				export let bar = 123
			`,
			"/remove/type-only-namespace-exported.ts": `
				export namespace foo {
					export type num = number
				}
				export default foo
				export let bar = 123
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImplicitExtensions(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import './pick-js.js'
				import './pick-ts.js'
				import './pick-jsx.jsx'
				import './pick-tsx.jsx'
				import './order-js.js'
				import './order-jsx.jsx'
			`,

			"/pick-js.js": `console.log("correct")`,
			"/pick-js.ts": `console.log("wrong")`,

			"/pick-ts.jsx": `console.log("wrong")`,
			"/pick-ts.ts":  `console.log("correct")`,

			"/pick-jsx.jsx": `console.log("correct")`,
			"/pick-jsx.tsx": `console.log("wrong")`,

			"/pick-tsx.js":  `console.log("wrong")`,
			"/pick-tsx.tsx": `console.log("correct")`,

			"/order-js.ts":  `console.log("correct")`,
			"/order-js.tsx": `console.log("wrong")`,

			"/order-jsx.ts":  `console.log("correct")`,
			"/order-jsx.tsx": `console.log("wrong")`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImplicitExtensionsMissing(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import './mjs.mjs'
				import './cjs.cjs'
				import './js.js'
				import './jsx.jsx'
			`,
			"/mjs.ts":      ``,
			"/mjs.tsx":     ``,
			"/cjs.ts":      ``,
			"/cjs.tsx":     ``,
			"/js.ts.js":    ``,
			"/jsx.tsx.jsx": ``,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `entry.ts: error: Could not resolve "./mjs.mjs"
entry.ts: error: Could not resolve "./cjs.cjs"
entry.ts: error: Could not resolve "./js.js"
entry.ts: error: Could not resolve "./jsx.jsx"
`,
	})
}

func TestExportTypeIssue379(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as A from './a'
				import * as B from './b'
				import * as C from './c'
				import * as D from './d'
				console.log(A, B, C, D)
			`,
			"/a.ts": `
				type Test = Element
				let foo = 123
				export { Test, foo }
			`,
			"/b.ts": `
				export type Test = Element
				export let foo = 123
			`,
			"/c.ts": `
				import { Test } from './test'
				let foo = 123
				export { Test }
				export { foo }
			`,
			"/d.ts": `
				export { Test }
				export { foo }
				import { Test } from './test'
				let foo = 123
			`,
			"/test.ts": `
				export type Test = Element
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:                    config.ModeBundle,
			AbsOutputFile:           "/out.js",
			UseDefineForClassFields: config.False,
		},
	})
}

func TestThisInsideFunctionTS(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
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
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:                    config.ModeBundle,
			AbsOutputFile:           "/out.js",
			UseDefineForClassFields: config.False,
		},
	})
}

func TestThisInsideFunctionTSUseDefineForClassFields(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
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
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:                    config.ModeBundle,
			AbsOutputFile:           "/out.js",
			UseDefineForClassFields: config.True,
		},
	})
}

func TestThisInsideFunctionTSNoBundle(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
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
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestThisInsideFunctionTSNoBundleUseDefineForClassFields(t *testing.T) {
	default_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
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
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:                    config.ModePassThrough,
			AbsOutputFile:           "/out.js",
			UseDefineForClassFields: config.True,
		},
	})
}
