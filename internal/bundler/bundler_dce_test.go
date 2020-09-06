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
