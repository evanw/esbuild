package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

var importstar_ts_suite = suite{
	name: "importstar_ts",
}

func TestTSImportStarUnused(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './foo'
				let foo = 234
				console.log(foo)
			`,
			"/foo.ts": `
				export const foo = 123
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarCapture(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns, ns.foo, foo)
			`,
			"/foo.ts": `
				export const foo = 123
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarNoCapture(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns.foo, ns.foo, foo)
			`,
			"/foo.ts": `
				export const foo = 123
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarExportImportStarUnused(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {ns} from './bar'
				let foo = 234
				console.log(foo)
			`,
			"/foo.ts": `
				export const foo = 123
			`,
			"/bar.ts": `
				import * as ns from './foo'
				export {ns}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarExportImportStarNoCapture(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {ns} from './bar'
				let foo = 234
				console.log(ns.foo, ns.foo, foo)
			`,
			"/foo.ts": `
				export const foo = 123
			`,
			"/bar.ts": `
				import * as ns from './foo'
				export {ns}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarExportImportStarCapture(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {ns} from './bar'
				let foo = 234
				console.log(ns, ns.foo, foo)
			`,
			"/foo.ts": `
				export const foo = 123
			`,
			"/bar.ts": `
				import * as ns from './foo'
				export {ns}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarExportStarAsUnused(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {ns} from './bar'
				let foo = 234
				console.log(foo)
			`,
			"/foo.ts": `
				export const foo = 123
			`,
			"/bar.ts": `
				export * as ns from './foo'
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarExportStarAsNoCapture(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {ns} from './bar'
				let foo = 234
				console.log(ns.foo, ns.foo, foo)
			`,
			"/foo.ts": `
				export const foo = 123
			`,
			"/bar.ts": `
				export * as ns from './foo'
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarExportStarAsCapture(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {ns} from './bar'
				let foo = 234
				console.log(ns, ns.foo, foo)
			`,
			"/foo.ts": `
				export const foo = 123
			`,
			"/bar.ts": `
				export * as ns from './foo'
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarExportStarUnused(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './bar'
				let foo = 234
				console.log(foo)
			`,
			"/foo.ts": `
				export const foo = 123
			`,
			"/bar.ts": `
				export * from './foo'
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarExportStarNoCapture(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './bar'
				let foo = 234
				console.log(ns.foo, ns.foo, foo)
			`,
			"/foo.ts": `
				export const foo = 123
			`,
			"/bar.ts": `
				export * from './foo'
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarExportStarCapture(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './bar'
				let foo = 234
				console.log(ns, ns.foo, foo)
			`,
			"/foo.ts": `
				export const foo = 123
			`,
			"/bar.ts": `
				export * from './foo'
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarCommonJSUnused(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './foo'
				let foo = 234
				console.log(foo)
			`,
			"/foo.ts": `
				exports.foo = 123
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarCommonJSCapture(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns, ns.foo, foo)
			`,
			"/foo.ts": `
				exports.foo = 123
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarCommonJSNoCapture(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns.foo, ns.foo, foo)
			`,
			"/foo.ts": `
				exports.foo = 123
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarAndCommonJS(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				const ns2 = require('./foo')
				console.log(ns.foo, ns2.foo)
			`,
			"/foo.ts": `
				export const foo = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarNoBundleUnused(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './foo'
				let foo = 234
				console.log(foo)
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarNoBundleCapture(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns, ns.foo, foo)
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarNoBundleNoCapture(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns.foo, ns.foo, foo)
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarMangleNoBundleUnused(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './foo'
				let foo = 234
				console.log(foo)
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			MangleSyntax:  true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarMangleNoBundleCapture(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns, ns.foo, foo)
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			MangleSyntax:  true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSImportStarMangleNoBundleNoCapture(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns.foo, ns.foo, foo)
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			MangleSyntax:  true,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSReExportTypeOnlyFileES6(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './re-export'
				console.log(ns.foo)
			`,
			"/re-export.ts": `
				export * from './types1'
				export * from './types2'
				export * from './types3'
				export * from './values'
			`,
			"/types1.ts": `
				export interface Foo {}
				export type Bar = number
				console.log('some code')
			`,
			"/types2.ts": `
				import {Foo} from "./type"
				export {Foo}
				console.log('some code')
			`,
			"/types3.ts": `
				export {Foo} from "./type"
				console.log('some code')
			`,
			"/values.ts": `
				export let foo = 123
			`,
			"/type.ts": `
				export type Foo = number
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSDuplicateExportRefs(t *testing.T) {
	importstar_ts_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './re-export'
				console.log(ns.Foo)
			`,
			"/re-export.ts": `
				export * from './types1'
				export { Foo } from './types1'
			`,
			"/types1.ts": `
				export class Foo {}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}
