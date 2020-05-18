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
			"/out.js": `// /entry.js
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

func TestDCEImportStarOfExportStar(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as foo from './foo'
				console.log(foo)
			`,
			"/foo.js": `
				export * from './bar'
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
const bar = 123;

// /foo.js
const foo_exports = {};
__export(foo_exports, {
  bar: () => bar
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
			"/out.js": `// /entry.js
let foo = 234;
console.log(foo);
`,
		},
	})
}

func TestDCEImportOfExportStar(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {bar} from './foo'
				console.log(bar)
			`,
			"/foo.js": `
				export * from './bar'
			`,
			"/bar.js": `
				// Add some statements to increase the part index (this reproduced a crash)
				statement()
				statement()
				statement()
				statement()
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
statement();
statement();
statement();
statement();
const bar = 123;

// /foo.js

// /entry.js
console.log(bar);
`,
		},
	})
}

func TestDCEImportOfExportStarOfImport(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {bar} from './foo'
				console.log(bar)
			`,
			"/foo.js": `
				// Add some statements to increase the part index (this reproduced a crash)
				statement()
				statement()
				statement()
				statement()
				export * from './bar'
			`,
			"/bar.js": `
				export {baz as bar} from './baz'
			`,
			"/baz.js": `
				export const baz = 123
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
			"/out.js": `// /baz.js
const baz = 123;

// /bar.js

// /foo.js
statement();
statement();
statement();
statement();

// /entry.js
console.log(baz);
`,
		},
	})
}
