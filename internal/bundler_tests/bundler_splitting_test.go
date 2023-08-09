package bundler_tests

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

var splitting_suite = suite{
	name: "splitting",
}

func TestSplittingSharedES6IntoES6(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import {foo} from "./shared.js"
				console.log(foo)
			`,
			"/b.js": `
				import {foo} from "./shared.js"
				console.log(foo)
			`,
			"/shared.js": `export let foo = 123`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
	})
}

func TestSplittingSharedCommonJSIntoES6(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				const {foo} = require("./shared.js")
				console.log(foo)
			`,
			"/b.js": `
				const {foo} = require("./shared.js")
				console.log(foo)
			`,
			"/shared.js": `exports.foo = 123`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
	})
}

func TestSplittingDynamicES6IntoES6(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import("./foo.js").then(({bar}) => console.log(bar))
			`,
			"/foo.js": `
				export let bar = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
	})
}

func TestSplittingDynamicCommonJSIntoES6(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import("./foo.js").then(({default: {bar}}) => console.log(bar))
			`,
			"/foo.js": `
				exports.bar = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
	})
}

func TestSplittingDynamicAndNotDynamicES6IntoES6(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {bar as a} from "./foo.js"
				import("./foo.js").then(({bar: b}) => console.log(a, b))
			`,
			"/foo.js": `
				export let bar = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
	})
}

func TestSplittingDynamicAndNotDynamicCommonJSIntoES6(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {bar as a} from "./foo.js"
				import("./foo.js").then(({default: {bar: b}}) => console.log(a, b))
			`,
			"/foo.js": `
				exports.bar = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
	})
}

func TestSplittingWithMinChunkSize(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import * as ns from './common_mini'
				import * as nsl from './common_large'
				export let a = 'a' + ns.foo
				export let aa = 'a' + nsl.bar
			`,
			"/b.js": `
				import * as ns from './common_mini'
				export let b = 'b' + ns.foo
			`,
			"/c.js": `
				import * as ns from './common_large'
				export let b = 'b' + ns.bar
			`,
			"/common_mini.js":  `export let foo = 123`,
			"/common_large.js": `export let bar = 1234`,
		},
		entryPaths: []string{"/a.js", "/b.js", "/c.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			MinChunkSize:  21,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
	})
}

func TestSplittingAssignToLocal(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import {foo, setFoo} from "./shared.js"
				setFoo(123)
				console.log(foo)
			`,
			"/b.js": `
				import {foo} from "./shared.js"
				console.log(foo)
			`,
			"/shared.js": `
				export let foo
				export function setFoo(value) {
					foo = value
				}
			`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
	})
}

func TestSplittingSideEffectsWithoutDependencies(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import {a} from "./shared.js"
				console.log(a)
			`,
			"/b.js": `
				import {b} from "./shared.js"
				console.log(b)
			`,
			"/shared.js": `
				export let a = 1
				export let b = 2
				console.log('side effect')
			`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
	})
}

func TestSplittingNestedDirectories(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/pages/pageA/page.js": `
				import x from "../shared.js"
				console.log(x)
			`,
			"/Users/user/project/src/pages/pageB/page.js": `
				import x from "../shared.js"
				console.log(-x)
			`,
			"/Users/user/project/src/pages/shared.js": `
				export default 123
			`,
		},
		entryPaths: []string{
			"/Users/user/project/src/pages/pageA/page.js",
			"/Users/user/project/src/pages/pageB/page.js",
		},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/Users/user/project/out",
		},
	})
}

func TestSplittingCircularReferenceIssue251(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				export * from './b.js';
				export var p = 5;
			`,
			"/b.js": `
				export * from './a.js';
				export var q = 6;
			`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
	})
}

func TestSplittingMissingLazyExport(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import {foo} from './common.js'
				console.log(foo())
			`,
			"/b.js": `
				import {bar} from './common.js'
				console.log(bar())
			`,
			"/common.js": `
				import * as ns from './empty.js'
				export function foo() { return [ns, ns.missing] }
				export function bar() { return [ns.missing] }
			`,
			"/empty.js": `
				// This forces the module into ES6 mode without importing or exporting anything
				import.meta
			`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
		expectedCompileLog: `common.js: WARNING: Import "missing" will always be undefined because the file "empty.js" has no exports
`,
	})
}

func TestSplittingReExportIssue273(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				export const a = 1
			`,
			"/b.js": `
				export { a } from './a'
			`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
	})
}

func TestSplittingDynamicImportIssue272(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import('./b')
			`,
			"/b.js": `
				export default 1
			`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
	})
}

func TestSplittingDynamicImportOutsideSourceTreeIssue264(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry1.js": `
				import('package')
			`,
			"/Users/user/project/src/entry2.js": `
				import('package')
			`,
			"/Users/user/project/node_modules/package/index.js": `
				console.log('imported')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry1.js", "/Users/user/project/src/entry2.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
	})
}

func TestSplittingCrossChunkAssignmentDependencies(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import {setValue} from './shared'
				setValue(123)
			`,
			"/b.js": `
				import './shared'
			`,
			"/shared.js": `
				var observer;
				var value;
				export function setObserver(cb) {
					observer = cb;
				}
				export function getValue() {
					return value;
				}
				export function setValue(next) {
					value = next;
					if (observer) observer();
				}
				sideEffects(getValue);
			`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
	})
}

func TestSplittingCrossChunkAssignmentDependenciesRecursive(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import { setX } from './x'
				setX()
			`,
			"/b.js": `
				import { setZ } from './z'
				setZ()
			`,
			"/c.js": `
				import { setX2 } from './x'
				import { setY2 } from './y'
				import { setZ2 } from './z'
				setX2();
				setY2();
				setZ2();
			`,
			"/x.js": `
				let _x
				export function setX(v) { _x = v }
				export function setX2(v) { _x = v }
			`,
			"/y.js": `
				import { setX } from './x'
				let _y
				export function setY(v) { _y = v }
				export function setY2(v) { setX(v); _y = v }
			`,
			"/z.js": `
				import { setY } from './y'
				let _z
				export function setZ(v) { _z = v }
				export function setZ2(v) { setY(v); _z = v }
			`,
		},
		entryPaths: []string{"/a.js", "/b.js", "/c.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
	})
}

func TestSplittingDuplicateChunkCollision(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import "./ab"
			`,
			"/b.js": `
				import "./ab"
			`,
			"/c.js": `
				import "./cd"
			`,
			"/d.js": `
				import "./cd"
			`,
			"/ab.js": `
				console.log(123)
			`,
			"/cd.js": `
				console.log(123)
			`,
		},
		entryPaths: []string{"/a.js", "/b.js", "/c.js", "/d.js"},
		options: config.Options{
			Mode:             config.ModeBundle,
			CodeSplitting:    true,
			MinifyWhitespace: true,
			OutputFormat:     config.FormatESModule,
			AbsOutputDir:     "/out",
		},
	})
}

func TestSplittingMinifyIdentifiersCrashIssue437(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import {foo} from "./shared"
				console.log(foo)
			`,
			"/b.js": `
				import {foo} from "./shared"
				console.log(foo)
			`,
			"/c.js": `
				import "./shared"
			`,
			"/shared.js": `
				export function foo(bar) {}
			`,
		},
		entryPaths: []string{"/a.js", "/b.js", "/c.js"},
		options: config.Options{
			Mode:              config.ModeBundle,
			CodeSplitting:     true,
			MinifyIdentifiers: true,
			OutputFormat:      config.FormatESModule,
			AbsOutputDir:      "/out",
		},
	})
}

func TestSplittingHybridESMAndCJSIssue617(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				export let foo
			`,
			"/b.js": `
				export let bar = require('./a')
			`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
	})
}

func TestSplittingPublicPathEntryName(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import("./b")
			`,
			"/b.js": `
				console.log('b')
			`,
		},
		entryPaths: []string{"/a.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			PublicPath:    "/www/",
			AbsOutputDir:  "/out",
		},
	})
}

func TestSplittingChunkPathDirPlaceholderImplicitOutbase(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/project/entry.js": `
				console.log(import('./output-path/should-contain/this-text/file'))
			`,
			"/project/output-path/should-contain/this-text/file.js": `
				console.log('file.js')
			`,
		},
		entryPaths: []string{"/project/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatESModule,
			CodeSplitting: true,
			AbsOutputDir:  "/out",
			ChunkPathTemplate: []config.PathTemplate{
				{Data: "./", Placeholder: config.DirPlaceholder},
				{Data: "/", Placeholder: config.NamePlaceholder},
				{Data: "-", Placeholder: config.HashPlaceholder},
			},
		},
	})
}

func TestEdgeCaseIssue2793WithSplitting(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/a.js": `
				export const A = 42;
			`,
			"/src/b.js": `
				export const B = async () => (await import(".")).A
			`,
			"/src/index.js": `
				export * from "./a"
				export * from "./b"
			`,
		},
		entryPaths: []string{"/src/index.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatESModule,
			CodeSplitting: true,
			AbsOutputDir:  "/out",
		},
	})
}

func TestEdgeCaseIssue2793WithoutSplitting(t *testing.T) {
	splitting_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/a.js": `
				export const A = 42;
			`,
			"/src/b.js": `
				export const B = async () => (await import(".")).A
			`,
			"/src/index.js": `
				export * from "./a"
				export * from "./b"
			`,
		},
		entryPaths: []string{"/src/index.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			OutputFormat: config.FormatESModule,
			AbsOutputDir: "/out",
		},
	})
}
