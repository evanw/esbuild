package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/parser"
)

func TestDCEImportStar(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				console.log(ns)
			`,
			"/foo.js": `
				export const foo = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling:  true,
			TreeShaking: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			TreeShaking:   true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.js
const foo_exports = {};
__export(foo_exports, {
  foo: () => foo
});
const foo = 123;

// /entry.js
console.log(foo_exports);
`,
		},
	})
}

func TestDCEImportStarUnused(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				console.log()
			`,
			"/foo.js": `
				export const foo = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling:  true,
			TreeShaking: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			TreeShaking:   true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.js

// /entry.js
console.log();
`,
		},
	})
}

func TestDCEImportStarNested(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as foo_ns from './foo'
				console.log(foo_ns)
			`,
			"/foo.js": `
				export * as bar_ns from './bar'
			`,
			"/bar.js": `
				export const bar = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling:  true,
			TreeShaking: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			TreeShaking:   true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /bar.js
const bar_exports = {};
__export(bar_exports, {
  bar: () => bar
});
const bar = 123;

// /foo.js
const foo_exports = {};
__export(foo_exports, {
  bar_ns: () => bar_exports
});

// /entry.js
console.log(foo_exports);
`,
		},
	})
}

func TestDCEImportStarES6ExportImportStarUnused(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {ns} from './bar'
				let foo = 234
				console.log(foo)
			`,
			"/foo.js": `
				export const foo = 123
			`,
			"/bar.js": `
				import * as ns from './foo'
				export {ns}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling:  true,
			TreeShaking: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			TreeShaking:   true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.js

// /bar.js

// /entry.js
let foo = 234;
console.log(foo);
`,
		},
	})
}
