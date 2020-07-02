package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/parser"
)

func TestPackageJsonSideEffectsFalseKeepNamedImportES6(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/node_modules/demo-pkg/index.js
const foo = 123;
console.log("hello");

// /Users/user/project/src/entry.js
console.log(foo);
`,
		},
	})
}

func TestPackageJsonSideEffectsFalseKeepNamedImportCommonJS(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/node_modules/demo-pkg/index.js
var require_demo_pkg = __commonJS((exports) => {
  exports.foo = 123;
  console.log("hello");
});

// /Users/user/project/src/entry.js
const demo_pkg = __toModule(require_demo_pkg());
console.log(demo_pkg.foo);
`,
		},
	})
}

func TestPackageJsonSideEffectsFalseKeepStarImportES6(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/node_modules/demo-pkg/index.js
const demo_pkg_exports = {};
__export(demo_pkg_exports, {
  foo: () => foo
});
const foo = 123;
console.log("hello");

// /Users/user/project/src/entry.js
console.log(demo_pkg_exports);
`,
		},
	})
}

func TestPackageJsonSideEffectsFalseKeepStarImportCommonJS(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/node_modules/demo-pkg/index.js
var require_demo_pkg = __commonJS((exports) => {
  exports.foo = 123;
  console.log("hello");
});

// /Users/user/project/src/entry.js
const ns = __toModule(require_demo_pkg());
console.log(ns);
`,
		},
	})
}

func TestPackageJsonSideEffectsTrueKeepES6(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/node_modules/demo-pkg/index.js
console.log("hello");

// /Users/user/project/src/entry.js
console.log("unused import");
`,
		},
	})
}

func TestPackageJsonSideEffectsTrueKeepCommonJS(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/node_modules/demo-pkg/index.js
var require_demo_pkg = __commonJS((exports) => {
  exports.foo = 123;
  console.log("hello");
});

// /Users/user/project/src/entry.js
const demo_pkg = __toModule(require_demo_pkg());
console.log("unused import");
`,
		},
	})
}

func TestPackageJsonSideEffectsFalseKeepBareImportAndRequireES6(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/node_modules/demo-pkg/index.js
var require_demo_pkg = __commonJS((exports) => {
  __export(exports, {
    foo: () => foo
  });
  const foo = 123;
  console.log("hello");
});

// /Users/user/project/src/entry.js
require_demo_pkg();
console.log("unused import");
`,
		},
	})
}

func TestPackageJsonSideEffectsFalseKeepBareImportAndRequireCommonJS(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/node_modules/demo-pkg/index.js
var require_demo_pkg = __commonJS((exports) => {
  exports.foo = 123;
  console.log("hello");
});

// /Users/user/project/src/entry.js
require_demo_pkg();
console.log("unused import");
`,
		},
	})
}

func TestPackageJsonSideEffectsFalseRemoveBareImportES6(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/src/entry.js
console.log("unused import");
`,
		},
	})
}

func TestPackageJsonSideEffectsFalseRemoveBareImportCommonJS(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/src/entry.js
console.log("unused import");
`,
		},
	})
}

func TestPackageJsonSideEffectsFalseRemoveNamedImportES6(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/src/entry.js
console.log("unused import");
`,
		},
	})
}

func TestPackageJsonSideEffectsFalseRemoveNamedImportCommonJS(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/src/entry.js
console.log("unused import");
`,
		},
	})
}

func TestPackageJsonSideEffectsFalseRemoveStarImportES6(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/src/entry.js
console.log("unused import");
`,
		},
	})
}

func TestPackageJsonSideEffectsFalseRemoveStarImportCommonJS(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/src/entry.js
console.log("unused import");
`,
		},
	})
}

func TestPackageJsonSideEffectsArrayRemove(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/src/entry.js
console.log("unused import");
`,
		},
	})
}

func TestPackageJsonSideEffectsArrayKeep(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/node_modules/demo-pkg/index.js
console.log("hello");

// /Users/user/project/src/entry.js
console.log("unused import");
`,
		},
	})
}

func TestPackageJsonSideEffectsNestedDirectoryRemove(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/src/entry.js
console.log("unused import");
`,
		},
	})
}

func TestPackageJsonSideEffectsKeepExportDefaultExpr(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/node_modules/demo-pkg/index.js
const demo_pkg_default = exprWithSideEffects();

// /Users/user/project/src/entry.js
console.log(demo_pkg_default);
`,
		},
	})
}

func TestJSONLoaderRemoveUnused(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import unused from "./example.json"
				console.log('unused import')
			`,
			"/example.json": `{"data": true}`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
console.log("unused import");
`,
		},
	})
}

func TestTextLoaderRemoveUnused(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import unused from "./example.txt"
				console.log('unused import')
			`,
			"/example.txt": `some data`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
console.log("unused import");
`,
		},
	})
}

func TestBase64LoaderRemoveUnused(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import unused from "./example.data"
				console.log('unused import')
			`,
			"/example.data": `some data`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":   config.LoaderJS,
				".data": config.LoaderBase64,
			},
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
console.log("unused import");
`,
		},
	})
}

func TestDataURLLoaderRemoveUnused(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import unused from "./example.data"
				console.log('unused import')
			`,
			"/example.data": `some data`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":   config.LoaderJS,
				".data": config.LoaderDataURL,
			},
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
console.log("unused import");
`,
		},
	})
}

func TestFileLoaderRemoveUnused(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import unused from "./example.data"
				console.log('unused import')
			`,
			"/example.data": `some data`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":   config.LoaderJS,
				".data": config.LoaderFile,
			},
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
console.log("unused import");
`,
		},
	})
}

func TestRemoveUnusedImportMeta(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function foo() {
					console.log(import.meta.url, import.meta.path)
				}
				console.log('foo is unused')
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
console.log("foo is unused");
`,
		},
	})
}
