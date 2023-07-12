package bundler_tests

import (
	"regexp"
	"testing"

	"github.com/evanw/esbuild/internal/compat"
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

func TestTSDeclareClassFields(t *testing.T) {
	// Note: this test uses arrow functions to validate that
	// scopes inside "declare" fields are correctly discarded
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import './define-false'
				import './define-true'
			`,
			"/define-false/index.ts": `
				class Foo {
					a
					declare b
					[(() => null, c)]
					declare [(() => null, d)]

					static A
					static declare B
					static [(() => null, C)]
					static declare [(() => null, D)]
				}
				(() => new Foo())()
			`,
			"/define-true/index.ts": `
				class Bar {
					a
					declare b
					[(() => null, c)]
					declare [(() => null, d)]

					static A
					static declare B
					static [(() => null, C)]
					static declare [(() => null, D)]
				}
				(() => new Bar())()
			`,
			"/define-false/tsconfig.json": `{
				"compilerOptions": {
					"useDefineForClassFields": false
				}
			}`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:                  config.ModeBundle,
			AbsOutputFile:         "/out.js",
			UnsupportedJSFeatures: compat.ClassField,
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

func TestTSConstEnumComments(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/bar.ts": `
				export const enum Foo {
					"%/*" = 1,
					"*/%" = 2,
				}
			`,
			"/foo.ts": `
				import { Foo } from "./bar";
				const enum Bar {
					"%/*" = 1,
					"*/%" = 2,
				}
				console.log({
					'should have comments': [
						Foo["%/*"],
						Bar["%/*"],
					],
					'should not have comments': [
						Foo["*/%"],
						Bar["*/%"],
					],
				});
			`,
		},
		entryPaths: []string{"/foo.ts"},
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
		expectedCompileLog: `entry.ts: ERROR: No matching export in "foo.js" for import "default"
entry.ts: ERROR: No matching export in "foo.js" for import "y"
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
		expectedScanLog: `entry.ts: ERROR: Could not resolve "./doesNotExist.ts"
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

func TestTSNamespaceKeepNames(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				namespace ns {
					export let foo = () => {}
					export function bar() {}
					export class Baz {}
				}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			KeepNames:     true,
		},
	})
}

func TestTSNamespaceKeepNamesTargetES2015(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				namespace ns {
					export let foo = () => {}
					export function bar() {}
					export class Baz {}
				}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:                  config.ModeBundle,
			AbsOutputFile:         "/out.js",
			KeepNames:             true,
			UnsupportedJSFeatures: es(2015),
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
			MinifySyntax:      true,
			MinifyWhitespace:  true,
			MinifyIdentifiers: true,
			AbsOutputDir:      "/",
		},
	})
}

func TestTSMinifyNestedEnum(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.ts": `
				function foo() { enum Foo { A, B, C = Foo } return Foo }
			`,
			"/b.ts": `
				export function foo() { enum Foo { X, Y, Z = Foo } return Foo }
			`,
		},
		entryPaths: []string{"/a.ts", "/b.ts"},
		options: config.Options{
			MinifySyntax:      true,
			MinifyWhitespace:  true,
			MinifyIdentifiers: true,
			AbsOutputDir:      "/",
		},
	})
}

func TestTSMinifyNestedEnumNoLogicalAssignment(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.ts": `
				function foo() { enum Foo { A, B, C = Foo } return Foo }
			`,
			"/b.ts": `
				export function foo() { enum Foo { X, Y, Z = Foo } return Foo }
			`,
		},
		entryPaths: []string{"/a.ts", "/b.ts"},
		options: config.Options{
			MinifySyntax:          true,
			MinifyWhitespace:      true,
			MinifyIdentifiers:     true,
			AbsOutputDir:          "/",
			UnsupportedJSFeatures: compat.LogicalAssignment,
		},
	})
}

func TestTSMinifyNestedEnumNoArrow(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.ts": `
				function foo() { enum Foo { A, B, C = Foo } return Foo }
			`,
			"/b.ts": `
				export function foo() { enum Foo { X, Y, Z = Foo } return Foo }
			`,
		},
		entryPaths: []string{"/a.ts", "/b.ts"},
		options: config.Options{
			MinifySyntax:          true,
			MinifyWhitespace:      true,
			MinifyIdentifiers:     true,
			AbsOutputDir:          "/",
			UnsupportedJSFeatures: compat.Arrow,
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
			MinifySyntax:      true,
			MinifyWhitespace:  true,
			MinifyIdentifiers: true,
			AbsOutputDir:      "/",
		},
	})
}

func TestTSMinifyNamespaceNoLogicalAssignment(t *testing.T) {
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
			MinifySyntax:          true,
			MinifyWhitespace:      true,
			MinifyIdentifiers:     true,
			AbsOutputDir:          "/",
			UnsupportedJSFeatures: compat.LogicalAssignment,
		},
	})
}

func TestTSMinifyNamespaceNoArrow(t *testing.T) {
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
			MinifySyntax:          true,
			MinifyWhitespace:      true,
			MinifyIdentifiers:     true,
			AbsOutputDir:          "/",
			UnsupportedJSFeatures: compat.Arrow,
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
			"/tsconfig.json": `{
				"compilerOptions": {
					"useDefineForClassFields": false
				}
			}`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			MinifySyntax:          true,
			UnsupportedJSFeatures: es(2015),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestTSMinifyEnumPropertyNames(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import { CrossFileGood, CrossFileBad } from './cross-file'
				const enum SameFileGood {
					STR = 'str 1',
					NUM = 123,
				}
				const enum SameFileBad {
					PROTO = '__proto__',
					CONSTRUCTOR = 'constructor',
					PROTOTYPE = 'prototype',
				}
				class Foo {
					[100] = 100;
					'200' = 200;
					['300'] = 300;
					[SameFileGood.STR] = SameFileGood.STR;
					[SameFileGood.NUM] = SameFileGood.NUM;
					[CrossFileGood.STR] = CrossFileGood.STR;
					[CrossFileGood.NUM] = CrossFileGood.NUM;
				}
				shouldNotBeComputed(
					class {
						[100] = 100;
						'200' = 200;
						['300'] = 300;
						[SameFileGood.STR] = SameFileGood.STR;
						[SameFileGood.NUM] = SameFileGood.NUM;
						[CrossFileGood.STR] = CrossFileGood.STR;
						[CrossFileGood.NUM] = CrossFileGood.NUM;
					},
					{
						[100]: 100,
						'200': 200,
						['300']: 300,
						[SameFileGood.STR]: SameFileGood.STR,
						[SameFileGood.NUM]: SameFileGood.NUM,
						[CrossFileGood.STR]: CrossFileGood.STR,
						[CrossFileGood.NUM]: CrossFileGood.NUM,
					},
				)
				mustBeComputed(
					{ [SameFileBad.PROTO]: null },
					{ [CrossFileBad.PROTO]: null },
					class { [SameFileBad.CONSTRUCTOR]() {} },
					class { [CrossFileBad.CONSTRUCTOR]() {} },
					class { static [SameFileBad.PROTOTYPE]() {} },
					class { static [CrossFileBad.PROTOTYPE]() {} },
				)
			`,
			"/cross-file.ts": `
				export const enum CrossFileGood {
					STR = 'str 2',
					NUM = 321,
				}
				export const enum CrossFileBad {
					PROTO = '__proto__',
					CONSTRUCTOR = 'constructor',
					PROTOTYPE = 'prototype',
				}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			MinifySyntax:  true,
			AbsOutputFile: "/out.js",
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

func TestTSImportEqualsEliminationTest(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import a = foo.a
				import b = a.b
				import c = b.c

				import x = foo.x
				import y = x.y
				import z = y.z

				export let bar = c
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportEqualsTreeShakingFalse(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import { foo } from 'pkg'
				import used = foo.used
				import unused = foo.unused
				export { used }
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			AbsOutputFile: "/out.js",
			TreeShaking:   false,
		},
	})
}

func TestTSImportEqualsTreeShakingTrue(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import { foo } from 'pkg'
				import used = foo.used
				import unused = foo.unused
				export { used }
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			AbsOutputFile: "/out.js",
			TreeShaking:   true,
		},
	})
}

func TestTSImportEqualsBundle(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import { foo } from 'pkg'
				import used = foo.used
				import unused = foo.unused
				export { used }
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExternalSettings: config.ExternalSettings{
				PreResolve: config.ExternalMatchers{
					Exact: map[string]bool{
						"pkg": true,
					},
				},
			},
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
			MinifySyntax:      true,
			MinifyWhitespace:  true,
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
			MinifySyntax:      true,
			MinifyWhitespace:  true,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
	})
}

func TestTSExperimentalDecoratorsNoConfig(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare let x: any, y: any
				@x.y()
				@(new y.x)
				export default class Foo {
					@x @y mUndef: any
					@x @y mDef = 1
					@x @y method() { return new Foo }
					@x @y declare mDecl: any
					@x @y accessor aUndef: any
					@x @y accessor aDef = 1

					@x @y static sUndef: any
					@x @y static sDef = new Foo
					@x @y static sMethod() { return new Foo }
					@x @y static declare sDecl: any
					@x @y static accessor asUndef: any
					@x @y static accessor asDef = 1

					@x @y #mUndef: any
					@x @y #mDef = 1
					@x @y #method() { return new Foo }
					@x @y accessor #aUndef: any
					@x @y accessor #aDef = 1

					@x @y static #sUndef: any
					@x @y static #sDef = 1
					@x @y static #sMethod() { return new Foo }
					@x @y static accessor #asUndef: any
					@x @y static accessor #asDef = 1
				}
			`,
			"/tsconfig.json": `{
				"compilerOptions": {
					"experimentalDecorators": false
				}
			}`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSExperimentalDecorators(t *testing.T) {
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
					@x @y method(@x0 @y0 arg0, @x1 @y1 arg1) { return new Foo }
					@x @y declare mDecl
					constructor(@x0 @y0 arg0, @x1 @y1 arg1) {}

					@x @y static sUndef
					@x @y static sDef = new Foo
					@x @y static sMethod(@x0 @y0 arg0, @x1 @y1 arg1) { return new Foo }
					@x @y static declare mDecl
				}
			`,
			"/all_computed.ts": `
				@x?.[_ + 'y']()
				@new y?.[_ + 'x']()
				export default class Foo {
					@x @y [mUndef()]
					@x @y [mDef()] = 1
					@x @y [method()](@x0 @y0 arg0, @x1 @y1 arg1) { return new Foo }
					@x @y declare [mDecl()]

					// Side effect order must be preserved even for fields without decorators
					[xUndef()]
					[xDef()] = 2
					static [yUndef()]
					static [yDef()] = 3

					@x @y static [sUndef()]
					@x @y static [sDef()] = new Foo
					@x @y static [sMethod()](@x0 @y0 arg0, @x1 @y1 arg1) { return new Foo }
					@x @y static declare [mDecl()]
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
			"/tsconfig.json": `{
				"compilerOptions": {
					"useDefineForClassFields": false,
					"experimentalDecorators": true
				}
			}`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSExperimentalDecoratorsKeepNames(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				@decoratorMustComeAfterName
				export class Foo {}
			`,
			"/tsconfig.json": `{
				"compilerOptions": {
					"experimentalDecorators": true
				}
			}`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			KeepNames:     true,
		},
	})
}

// See: https://github.com/evanw/esbuild/issues/2147
func TestTSExperimentalDecoratorScopeIssue2147(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				let foo = 1
				class Foo {
					method1(@dec(foo) foo = 2) {}
					method2(@dec(() => foo) foo = 3) {}
				}

				class Bar {
					static x = class {
						static y = () => {
							let bar = 1
							@dec(bar)
							@dec(() => bar)
							class Baz {
								@dec(bar) method1() {}
								@dec(() => bar) method2() {}
								method3(@dec(() => bar) bar) {}
								method4(@dec(() => bar) bar) {}
							}
							return Baz
						}
					}
				}
			`,
			"/tsconfig.json": `{
				"compilerOptions": {
					"useDefineForClassFields": false,
					"experimentalDecorators": true
				}
			}`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModePassThrough,
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
					dc_def, dc,
					dl_def, dl,
					im_def, im,
					in_def, _in,
					tn_def, tn,
					vn_def, vn,
					vnm_def, vnm,

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
			"/tsconfig.json": `{
				"compilerOptions": {
					"useDefineForClassFields": false
				}
			}`,
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

				import 'pkg/foo-js.js'
				import 'pkg/foo-jsx.jsx'
				import 'pkg-exports/xyz-js'
				import 'pkg-exports/xyz-jsx'
				import 'pkg-exports/foo-js.js'
				import 'pkg-exports/foo-jsx.jsx'
				import 'pkg-imports'
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

			"/node_modules/pkg/foo-js.ts":   `console.log("correct")`,
			"/node_modules/pkg/foo-jsx.tsx": `console.log("correct")`,

			"/node_modules/pkg-exports/package.json": `{
				"exports": {
					"./xyz-js": "./abc-js.js",
					"./xyz-jsx": "./abc-jsx.jsx",
					"./*": "./lib/*"
				}
			}`,
			"/node_modules/pkg-exports/abc-js.ts":       `console.log("correct")`,
			"/node_modules/pkg-exports/abc-jsx.tsx":     `console.log("correct")`,
			"/node_modules/pkg-exports/lib/foo-js.ts":   `console.log("correct")`,
			"/node_modules/pkg-exports/lib/foo-jsx.tsx": `console.log("correct")`,

			"/node_modules/pkg-imports/package.json": `{
				"imports": {
					"#xyz-js": "./abc-js.js",
					"#xyz-jsx": "./abc-jsx.jsx",
					"#bar/*": "./lib/*"
				}
			}`,
			"/node_modules/pkg-imports/index.js": `
				import "#xyz-js"
				import "#xyz-jsx"
				import "#bar/foo-js.js"
				import "#bar/foo-jsx.jsx"
			`,
			"/node_modules/pkg-imports/abc-js.ts":       `console.log("correct")`,
			"/node_modules/pkg-imports/abc-jsx.tsx":     `console.log("correct")`,
			"/node_modules/pkg-imports/lib/foo-js.ts":   `console.log("correct")`,
			"/node_modules/pkg-imports/lib/foo-jsx.tsx": `console.log("correct")`,
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
		expectedScanLog: `entry.ts: ERROR: Could not resolve "./mjs.mjs"
entry.ts: ERROR: Could not resolve "./cjs.cjs"
entry.ts: ERROR: Could not resolve "./js.js"
entry.ts: ERROR: Could not resolve "./jsx.jsx"
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			TS: config.TSOptions{Config: config.TSConfig{
				UseDefineForClassFields: config.False,
			}},
		},
	})
}

func TestThisInsideFunctionTS(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			TS: config.TSOptions{Config: config.TSConfig{
				UseDefineForClassFields: config.False,
			}},
		},
	})
}

func TestThisInsideFunctionTSUseDefineForClassFields(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			TS: config.TSOptions{Config: config.TSConfig{
				UseDefineForClassFields: config.True,
			}},
		},
	})
}

func TestThisInsideFunctionTSNoBundle(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
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
			"/tsconfig.json": `{
				"compilerOptions": {
					"useDefineForClassFields": false
				}
			}`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestThisInsideFunctionTSNoBundleUseDefineForClassFields(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
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
			TS: config.TSOptions{Config: config.TSConfig{
				UseDefineForClassFields: config.True,
			}},
		},
	})
}

func TestTSComputedClassFieldUseDefineFalse(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class Foo {
					[q];
					[r] = s;
					@dec
					[x];
					@dec
					[y] = z;
				}
				new Foo()
			`,
			"/tsconfig.json": `{
				"compilerOptions": {
					"useDefineForClassFields": false,
					"experimentalDecorators": true
				}
			}`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSComputedClassFieldUseDefineTrue(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class Foo {
					[q];
					[r] = s;
					@dec
					[x];
					@dec
					[y] = z;
				}
				new Foo()
			`,
			"/tsconfig.json": `{
				"compilerOptions": {
					"useDefineForClassFields": true,
					"experimentalDecorators": true
				}
			}`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSComputedClassFieldUseDefineTrueLower(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class Foo {
					[q];
					[r] = s;
					@dec
					[x];
					@dec
					[y] = z;
				}
				new Foo()
			`,
			"/tsconfig.json": `{
				"compilerOptions": {
					"useDefineForClassFields": true,
					"experimentalDecorators": true
				}
			}`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:                  config.ModePassThrough,
			AbsOutputFile:         "/out.js",
			UnsupportedJSFeatures: compat.ClassField,
		},
	})
}

func TestTSAbstractClassFieldUseAssign(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				const keepThis = Symbol('keepThis')
				declare const AND_REMOVE_THIS: unique symbol
				abstract class Foo {
					REMOVE_THIS: any
					[keepThis]: any
					abstract REMOVE_THIS_TOO: any
					abstract [AND_REMOVE_THIS]: any
					abstract [(x => y => x + y)('nested')('scopes')]: any
				}
				(() => new Foo())()
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			AbsOutputFile: "/out.js",
			TS: config.TSOptions{Config: config.TSConfig{
				UseDefineForClassFields: config.False,
			}},
		},
	})
}

func TestTSAbstractClassFieldUseDefine(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				const keepThisToo = Symbol('keepThisToo')
				declare const REMOVE_THIS_TOO: unique symbol
				abstract class Foo {
					keepThis: any
					[keepThisToo]: any
					abstract REMOVE_THIS: any
					abstract [REMOVE_THIS_TOO]: any
					abstract [(x => y => x + y)('nested')('scopes')]: any
				}
				(() => new Foo())()
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			AbsOutputFile: "/out.js",
			TS: config.TSOptions{Config: config.TSConfig{
				UseDefineForClassFields: config.True,
			}},
		},
	})
}

func TestTSImportMTS(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import './imported.mjs'
			`,
			"/imported.mts": `
				console.log('works')
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			OutputFormat:  config.FormatESModule,
		},
	})
}

func TestTSImportCTS(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				require('./required.cjs')
			`,
			"/required.cjs": `
				console.log('works')
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			OutputFormat:  config.FormatCommonJS,
		},
	})
}

func TestTSSideEffectsFalseWarningTypeDeclarations(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import "some-js"
				import "some-ts"
				import "empty-js"
				import "empty-ts"
				import "empty-dts"
			`,

			"/node_modules/some-js/package.json": `{ "main": "./foo.js", "sideEffects": false }`,
			"/node_modules/some-js/foo.js":       `console.log('foo')`,

			"/node_modules/some-ts/package.json": `{ "main": "./foo.ts", "sideEffects": false }`,
			"/node_modules/some-ts/foo.ts":       `console.log('foo' as string)`,

			"/node_modules/empty-js/package.json": `{ "main": "./foo.js", "sideEffects": false }`,
			"/node_modules/empty-js/foo.js":       ``,

			"/node_modules/empty-ts/package.json": `{ "main": "./foo.ts", "sideEffects": false }`,
			"/node_modules/empty-ts/foo.ts":       `export type Foo = number`,

			"/node_modules/empty-dts/package.json": `{ "main": "./foo.d.ts", "sideEffects": false }`,
			"/node_modules/empty-dts/foo.d.ts":     `export type Foo = number`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `entry.ts: WARNING: Ignoring this import because "node_modules/some-js/foo.js" was marked as having no side effects
node_modules/some-js/package.json: NOTE: "sideEffects" is false in the enclosing "package.json" file:
entry.ts: WARNING: Ignoring this import because "node_modules/some-ts/foo.ts" was marked as having no side effects
node_modules/some-ts/package.json: NOTE: "sideEffects" is false in the enclosing "package.json" file:
`,
	})
}

func TestTSSiblingNamespace(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/let.ts": `
				export namespace x { export let y = 123 }
				export namespace x { export let z = y }
			`,
			"/function.ts": `
				export namespace x { export function y() {} }
				export namespace x { export let z = y }
			`,
			"/class.ts": `
				export namespace x { export class y {} }
				export namespace x { export let z = y }
			`,
			"/namespace.ts": `
				export namespace x { export namespace y { 0 } }
				export namespace x { export let z = y }
			`,
			"/enum.ts": `
				export namespace x { export enum y {} }
				export namespace x { export let z = y }
			`,
		},
		entryPaths: []string{
			"/let.ts",
			"/function.ts",
			"/class.ts",
			"/namespace.ts",
			"/enum.ts",
		},
		options: config.Options{
			Mode:         config.ModePassThrough,
			AbsOutputDir: "/out",
		},
	})
}

func TestTSSiblingEnum(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/number.ts": `
				export enum x { y, yy = y }
				export enum x { z = y + 1 }

				declare let y: any, z: any
				export namespace x { console.log(y, z) }
				console.log(x.y, x.z)
			`,
			"/string.ts": `
				export enum x { y = 'a', yy = y }
				export enum x { z = y }

				declare let y: any, z: any
				export namespace x { console.log(y, z) }
				console.log(x.y, x.z)
			`,
			"/propagation.ts": `
				export enum a { b = 100 }
				export enum x {
					c = a.b,
					d = c * 2,
					e = x.d ** 2,
					f = x['e'] / 4,
				}
				export enum x { g = f >> 4 }
				console.log(a.b, a['b'], x.g, x['g'])
			`,
			"/nested-number.ts": `
				export namespace foo { export enum x { y, yy = y } }
				export namespace foo { export enum x { z = y + 1 } }

				declare let y: any, z: any
				export namespace foo.x {
					console.log(y, z)
					console.log(x.y, x.z)
				}
			`,
			"/nested-string.ts": `
				export namespace foo { export enum x { y = 'a', yy = y } }
				export namespace foo { export enum x { z = y } }

				declare let y: any, z: any
				export namespace foo.x {
					console.log(y, z)
					console.log(x.y, x.z)
				}
			`,
			"/nested-propagation.ts": `
				export namespace n { export enum a { b = 100 } }
				export namespace n {
					export enum x {
						c = n.a.b,
						d = c * 2,
						e = x.d ** 2,
						f = x['e'] / 4,
					}
				}
				export namespace n {
					export enum x { g = f >> 4 }
					console.log(a.b, n.a.b, n['a']['b'], x.g, n.x.g, n['x']['g'])
				}
			`,
		},
		entryPaths: []string{
			"/number.ts",
			"/string.ts",
			"/propagation.ts",
			"/nested-number.ts",
			"/nested-string.ts",
			"/nested-propagation.ts",
		},
		options: config.Options{
			Mode:         config.ModePassThrough,
			AbsOutputDir: "/out",
		},
	})
}

func TestTSEnumTreeShaking(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/simple-member.ts": `
				enum x { y = 123 }
				console.log(x.y)
			`,
			"/simple-enum.ts": `
				enum x { y = 123 }
				console.log(x)
			`,
			"/sibling-member.ts": `
				enum x { y = 123 }
				enum x { z = y * 2 }
				console.log(x.y, x.z)
			`,
			"/sibling-enum-before.ts": `
				console.log(x)
				enum x { y = 123 }
				enum x { z = y * 2 }
			`,
			"/sibling-enum-middle.ts": `
				enum x { y = 123 }
				console.log(x)
				enum x { z = y * 2 }
			`,
			"/sibling-enum-after.ts": `
				enum x { y = 123 }
				enum x { z = y * 2 }
				console.log(x)
			`,
			"/namespace-before.ts": `
				namespace x { console.log(x, y) }
				enum x { y = 123 }
			`,
			"/namespace-after.ts": `
				enum x { y = 123 }
				namespace x { console.log(x, y) }
			`,
		},
		entryPaths: []string{
			"/simple-member.ts",
			"/simple-enum.ts",
			"/sibling-member.ts",
			"/sibling-enum-before.ts",
			"/sibling-enum-middle.ts",
			"/sibling-enum-after.ts",
			"/namespace-before.ts",
			"/namespace-after.ts",
		},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			OutputFormat: config.FormatESModule,
		},
	})
}

func TestTSEnumJSX(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/element.tsx": `
				export enum Foo { Div = 'div' }
				console.log(<Foo.Div />)
			`,
			"/fragment.tsx": `
				export enum React { Fragment = 'div' }
				console.log(<>test</>)
			`,
			"/nested-element.tsx": `
				namespace x.y { export enum Foo { Div = 'div' } }
				namespace x.y { console.log(<x.y.Foo.Div />) }
			`,
			"/nested-fragment.tsx": `
				namespace x.y { export enum React { Fragment = 'div' } }
				namespace x.y { console.log(<>test</>) }
			`,
		},
		entryPaths: []string{
			"/element.tsx",
			"/fragment.tsx",
			"/nested-element.tsx",
			"/nested-fragment.tsx",
		},
		options: config.Options{
			Mode:         config.ModePassThrough,
			AbsOutputDir: "/out",
		},
	})
}

func TestTSEnumDefine(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				enum a { b = 123, c = d }
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:         config.ModePassThrough,
			AbsOutputDir: "/out",
			Defines: &config.ProcessedDefines{
				IdentifierDefines: map[string]config.DefineData{
					"d": {
						DefineExpr: &config.DefineExpr{
							Parts: []string{"b"},
						},
					},
				},
			},
		},
	})
}

func TestTSEnumSameModuleInliningAccess(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				enum a { x = 123 }
				enum b { x = 123 }
				enum c { x = 123 }
				enum d { x = 123 }
				enum e { x = 123 }
				console.log([
					a.x,
					b['x'],
					c?.x,
					d?.['x'],
					e,
				])
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestTSEnumCrossModuleInliningAccess(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import { a, b, c, d, e } from './enums'
				console.log([
					a.x,
					b['x'],
					c?.x,
					d?.['x'],
					e,
				])
			`,
			"/enums.ts": `
				export enum a { x = 123 }
				export enum b { x = 123 }
				export enum c { x = 123 }
				export enum d { x = 123 }
				export enum e { x = 123 }
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestTSEnumCrossModuleInliningDefinitions(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import { a } from './enums'
				console.log([
					a.implicit_number,
					a.explicit_number,
					a.explicit_string,
					a.non_constant,
				])
			`,
			"/enums.ts": `
				export enum a {
					implicit_number,
					explicit_number = 123,
					explicit_string = 'xyz',
					non_constant = foo,
				}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestTSEnumCrossModuleInliningReExport(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import { a } from './re-export'
				import { b } from './re-export-star'
				import * as ns from './enums'
				console.log([
					a.x,
					b.x,
					ns.c.x,
				])
			`,
			"/re-export.js": `
				export { a } from './enums'
			`,
			"/re-export-star.js": `
				export * from './enums'
			`,
			"/enums.ts": `
				export enum a { x = 'a' }
				export enum b { x = 'b' }
				export enum c { x = 'c' }
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestTSEnumCrossModuleInliningMinifyIndexIntoDot(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				const enum Foo {
					foo1 = 'abc',
					foo2 = 'a b c',
				}
				import { Bar } from './lib'
				inlined = [
					obj[Foo.foo1],
					obj[Bar.bar1],
					obj?.[Foo.foo1],
					obj?.[Bar.bar1],
					obj?.prop[Foo.foo1],
					obj?.prop[Bar.bar1],
				]
				notInlined = [
					obj[Foo.foo2],
					obj[Bar.bar2],
					obj?.[Foo.foo2],
					obj?.[Bar.bar2],
					obj?.prop[Foo.foo2],
					obj?.prop[Bar.bar2],
				]
			`,
			"/lib.ts": `
				export const enum Bar {
					bar1 = 'xyz',
					bar2 = 'x y z',
				}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			MinifySyntax:  true,
		},
	})
}

func TestTSEnumCrossModuleTreeShaking(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {
					a_DROP,
					b_DROP,
					c_DROP,
				} from './enums'

				console.log([
					a_DROP.x,
					b_DROP['x'],
					c_DROP.x,
				])

				import {
					a_keep,
					b_keep,
					c_keep,
					d_keep,
					e_keep,
				} from './enums'

				console.log([
					a_keep.x,
					b_keep.x,
					c_keep,
					d_keep.y,
					e_keep.x,
				])
			`,
			"/enums.ts": `
				export enum a_DROP { x = 1 }  // test a dot access
				export enum b_DROP { x = 2 }  // test an index access
				export enum c_DROP { x = '' } // test a string enum

				export enum a_keep { x = false } // false is not inlinable
				export enum b_keep { x = foo }   // foo has side effects
				export enum c_keep { x = 3 }     // this enum object is captured
				export enum d_keep { x = 4 }     // we access "y" on this object
				export let e_keep = {}           // non-enum properties should be kept
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestTSEnumExportClause(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {
					A,
					B,
					C as c,
					d as dd,
				} from './enums'

				console.log([
					A.A,
					B.B,
					c.C,
					dd.D,
				])
			`,
			"/enums.ts": `
					export enum A { A = 1 }
					enum B { B = 2 }
					export enum C { C = 3 }
					enum D { D = 4 }
					export { B, D as d }
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

// This checks that we don't generate a warning for code that the TypeScript
// compiler generates that looks like this:
//
//	var __rest = (this && this.__rest) || function (s, e) {
//	  ...
//	};
func TestTSThisIsUndefinedWarning(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/warning1.ts": `export var foo = this`,
			"/warning2.ts": `export var foo = this || this.foo`,
			"/warning3.ts": `export var foo = this ? this.foo : null`,

			"/silent1.ts": `export var foo = this && this.foo`,
			"/silent2.ts": `export var foo = this && (() => this.foo)`,
		},
		entryPaths: []string{
			"/warning1.ts",
			"/warning2.ts",
			"/warning3.ts",

			"/silent1.ts",
			"/silent2.ts",
		},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
		debugLogs: true,
		expectedScanLog: `warning1.ts: DEBUG: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
warning1.ts: NOTE: This file is considered to be an ECMAScript module because of the "export" keyword here:
warning2.ts: DEBUG: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
warning2.ts: NOTE: This file is considered to be an ECMAScript module because of the "export" keyword here:
warning3.ts: DEBUG: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
warning3.ts: NOTE: This file is considered to be an ECMAScript module because of the "export" keyword here:
`,
	})
}

func TestTSCommonJSVariableInESMTypeModule(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts":     `module.exports = null`,
			"/package.json": `{ "type": "module" }`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `entry.ts: WARNING: The CommonJS "module" variable is treated as a global variable in an ECMAScript module and may not work as expected
package.json: NOTE: This file is considered to be an ECMAScript module because the enclosing "package.json" file sets the type of this file to "module":
NOTE: Node's package format requires that CommonJS files in a "type": "module" package use the ".cjs" file extension. If you are using TypeScript, you can use the ".cts" file extension with esbuild instead.
`,
	})
}

func TestEnumRulesFrom_TypeScript_5_0(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/supported.ts": `
				// From https://github.com/microsoft/TypeScript/pull/50528:
				// "An expression is considered a constant expression if it is
				const enum Foo {
					// a number or string literal,
					X0 = 123,
					X1 = 'x',

					// a unary +, -, or ~ applied to a numeric constant expression,
					X2 = +1,
					X3 = -2,
					X4 = ~3,

					// a binary +, -, *, /, %, **, <<, >>, >>>, |, &, ^ applied to two numeric constant expressions,
					X5 = 1 + 2,
					X6 = 1 - 2,
					X7 = 2 * 3,
					X8 = 1 / 2,
					X9 = 3 % 2,
					X10 = 2 ** 3,
					X11 = 1 << 2,
					X12 = -9 >> 1,
					X13 = -9 >>> 1,
					X14 = 5 | 12,
					X15 = 5 & 12,
					X16 = 5 ^ 12,

					// a binary + applied to two constant expressions whereof at least one is a string,
					X17 = 'x' + 0,
					X18 = 0 + 'x',
					X19 = 'x' + 'y',
					X20 = '' + NaN,
					X21 = '' + Infinity,
					X22 = '' + -Infinity,
					X23 = '' + -0,

					// a template expression where each substitution expression is a constant expression,
					X24 = ` + "`A${0}B${'x'}C${1 + 3 - 4 / 2 * 5 ** 6}D`" + `,

					// a parenthesized constant expression,
					X25 = (321),

					// a dotted name (e.g. x.y.z) that references a const variable with a constant expression initializer and no type annotation,
					/* (we don't implement this one) */

					// a dotted name that references an enum member with an enum literal type, or
					X26 = X0,
					X27 = X0 + 'x',
					X28 = 'x' + X0,
					X29 = ` + "`a${X0}b`" + `,
					X30 = Foo.X0,
					X31 = Foo.X0 + 'x',
					X32 = 'x' + Foo.X0,
					X33 = ` + "`a${Foo.X0}b`" + `,

					// a dotted name indexed by a string literal (e.g. x.y["z"]) that references an enum member with an enum literal type."
					X34 = X1,
					X35 = X1 + 'y',
					X36 = 'y' + X1,
					X37 = ` + "`a${X1}b`" + `,
					X38 = Foo['X1'],
					X39 = Foo['X1'] + 'y',
					X40 = 'y' + Foo['X1'],
					X41 = ` + "`a${Foo['X1']}b`" + `,
				}

				console.log(
					// a number or string literal,
					Foo.X0,
					Foo.X1,

					// a unary +, -, or ~ applied to a numeric constant expression,
					Foo.X2,
					Foo.X3,
					Foo.X4,

					// a binary +, -, *, /, %, **, <<, >>, >>>, |, &, ^ applied to two numeric constant expressions,
					Foo.X5,
					Foo.X6,
					Foo.X7,
					Foo.X8,
					Foo.X9,
					Foo.X10,
					Foo.X11,
					Foo.X12,
					Foo.X13,
					Foo.X14,
					Foo.X15,
					Foo.X16,

					// a template expression where each substitution expression is a constant expression,
					Foo.X17,
					Foo.X18,
					Foo.X19,
					Foo.X20,
					Foo.X21,
					Foo.X22,
					Foo.X23,

					// a template expression where each substitution expression is a constant expression,
					Foo.X24,

					// a parenthesized constant expression,
					Foo.X25,

					// a dotted name that references an enum member with an enum literal type, or
					Foo.X26,
					Foo.X27,
					Foo.X28,
					Foo.X29,
					Foo.X30,
					Foo.X31,
					Foo.X32,
					Foo.X33,

					// a dotted name indexed by a string literal (e.g. x.y["z"]) that references an enum member with an enum literal type."
					Foo.X34,
					Foo.X35,
					Foo.X36,
					Foo.X37,
					Foo.X38,
					Foo.X39,
					Foo.X40,
					Foo.X41,
				)
			`,
			"/not-supported.ts": `
				const enum NonIntegerNumberToString {
					SUPPORTED = '' + 1,
					UNSUPPORTED = '' + 1.5,
				}
				console.log(
					NonIntegerNumberToString.SUPPORTED,
					NonIntegerNumberToString.UNSUPPORTED,
				)

				const enum OutOfBoundsNumberToString {
					SUPPORTED = '' + 1_000_000_000,
					UNSUPPORTED = '' + 1_000_000_000_000,
				}
				console.log(
					OutOfBoundsNumberToString.SUPPORTED,
					OutOfBoundsNumberToString.UNSUPPORTED,
				)

				const enum TemplateExpressions {
					// TypeScript enums don't handle any of these
					NULL = '' + null,
					TRUE = '' + true,
					FALSE = '' + false,
					BIGINT = '' + 123n,
				}
				console.log(
					TemplateExpressions.NULL,
					TemplateExpressions.TRUE,
					TemplateExpressions.FALSE,
					TemplateExpressions.BIGINT,
				)
			`,
		},
		entryPaths: []string{
			"/supported.ts",
			"/not-supported.ts",
		},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestTSEnumUseBeforeDeclare(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				export function before() {
					console.log(Foo.FOO)
				}
				enum Foo { FOO }
				export function after() {
					console.log(Foo.FOO)
				}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestTSPreferJSOverTSInsideNodeModules(t *testing.T) {
	// We now prefer ".js" over ".ts" inside "node_modules"
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/main.ts": `
				// Implicit extensions
				import './relative/path'
				import 'package/path'

				// Explicit extensions
				import './relative2/path.js'
				import 'package2/path.js'
			`,

			"/Users/user/project/src/relative/path.ts": `console.log('success')`,
			"/Users/user/project/src/relative/path.js": `console.log('FAILURE')`,

			"/Users/user/project/src/relative2/path.ts": `console.log('FAILURE')`,
			"/Users/user/project/src/relative2/path.js": `console.log('success')`,

			"/Users/user/project/node_modules/package/path.ts": `console.log('FAILURE')`,
			"/Users/user/project/node_modules/package/path.js": `console.log('success')`,

			"/Users/user/project/node_modules/package2/path.ts": `console.log('FAILURE')`,
			"/Users/user/project/node_modules/package2/path.js": `console.log('success')`,
		},
		entryPaths: []string{"/Users/user/project/src/main.ts"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestTSExperimentalDecoratorsManglePropsDefineSemantics(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class Foo {
					@dec(1) prop1 = null
					@dec(2) prop2_ = null
					@dec(3) ['prop3'] = null
					@dec(4) ['prop4_'] = null
					@dec(5) [/* @__KEY__ */ 'prop5'] = null
					@dec(6) [/* @__KEY__ */ 'prop6_'] = null
				}
			`,
			"/tsconfig.json": `{
        "compilerOptions": {
          "experimentalDecorators": true,
          "useDefineForClassFields": true,
        },
			}`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			MangleProps:   regexp.MustCompile("_$"),
		},
	})
}

func TestTSExperimentalDecoratorsManglePropsAssignSemantics(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class Foo {
					@dec(1) prop1 = null
					@dec(2) prop2_ = null
					@dec(3) ['prop3'] = null
					@dec(4) ['prop4_'] = null
					@dec(5) [/* @__KEY__ */ 'prop5'] = null
					@dec(6) [/* @__KEY__ */ 'prop6_'] = null
				}
			`,
			"/tsconfig.json": `{
        "compilerOptions": {
          "experimentalDecorators": true,
          "useDefineForClassFields": false,
        },
			}`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			MangleProps:   regexp.MustCompile("_$"),
		},
	})
}

func TestTSExperimentalDecoratorsManglePropsMethods(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class Foo {
					@dec(1) prop1() {}
					@dec(2) prop2_() {}
					@dec(3) ['prop3']() {}
					@dec(4) ['prop4_']() {}
					@dec(5) [/* @__KEY__ */ 'prop5']() {}
					@dec(6) [/* @__KEY__ */ 'prop6_']() {}
				}
			`,
			"/tsconfig.json": `{
        "compilerOptions": {
          "experimentalDecorators": true,
        },
			}`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			MangleProps:   regexp.MustCompile("_$"),
		},
	})
}

func TestTSExperimentalDecoratorsManglePropsStaticDefineSemantics(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class Foo {
					@dec(1) static prop1 = null
					@dec(2) static prop2_ = null
					@dec(3) static ['prop3'] = null
					@dec(4) static ['prop4_'] = null
					@dec(5) static [/* @__KEY__ */ 'prop5'] = null
					@dec(6) static [/* @__KEY__ */ 'prop6_'] = null
				}
			`,
			"/tsconfig.json": `{
        "compilerOptions": {
          "experimentalDecorators": true,
          "useDefineForClassFields": true,
        },
			}`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			MangleProps:   regexp.MustCompile("_$"),
		},
	})
}

func TestTSExperimentalDecoratorsManglePropsStaticAssignSemantics(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class Foo {
					@dec(1) static prop1 = null
					@dec(2) static prop2_ = null
					@dec(3) static ['prop3'] = null
					@dec(4) static ['prop4_'] = null
					@dec(5) static [/* @__KEY__ */ 'prop5'] = null
					@dec(6) static [/* @__KEY__ */ 'prop6_'] = null
				}
			`,
			"/tsconfig.json": `{
        "compilerOptions": {
          "experimentalDecorators": true,
          "useDefineForClassFields": false,
        },
			}`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			MangleProps:   regexp.MustCompile("_$"),
		},
	})
}

func TestTSExperimentalDecoratorsManglePropsStaticMethods(t *testing.T) {
	ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class Foo {
					@dec(1) static prop1() {}
					@dec(2) static prop2_() {}
					@dec(3) static ['prop3']() {}
					@dec(4) static ['prop4_']() {}
					@dec(5) static [/* @__KEY__ */ 'prop5']() {}
					@dec(6) static [/* @__KEY__ */ 'prop6_']() {}
				}
			`,
			"/tsconfig.json": `{
        "compilerOptions": {
          "experimentalDecorators": true,
        },
			}`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			MangleProps:   regexp.MustCompile("_$"),
		},
	})
}
