package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/parser"
)

func TestTSImportStarES6Unused(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.ts
let foo = 234;
console.log(foo);
`,
		},
	})
}

func TestTSImportStarES6Capture(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.ts
const foo_exports = {};
__export(foo_exports, {
  foo: () => foo2
});
const foo2 = 123;

// /entry.ts
let foo = 234;
console.log(foo_exports, foo2, foo);
`,
		},
	})
}

func TestTSImportStarES6NoCapture(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.ts
const foo2 = 123;

// /entry.ts
let foo = 234;
console.log(foo2, foo2, foo);
`,
		},
	})
}

func TestTSImportStarES6ExportImportStarUnused(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.ts
let foo = 234;
console.log(foo);
`,
		},
	})
}

func TestTSImportStarES6ExportImportStarNoCapture(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.ts
const foo_exports = {};
__export(foo_exports, {
  foo: () => foo2
});
const foo2 = 123;

// /bar.ts

// /entry.ts
let foo = 234;
console.log(foo_exports.foo, foo_exports.foo, foo);
`,
		},
	})
}

func TestTSImportStarES6ExportImportStarCapture(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.ts
const foo_exports = {};
__export(foo_exports, {
  foo: () => foo2
});
const foo2 = 123;

// /bar.ts

// /entry.ts
let foo = 234;
console.log(foo_exports, foo_exports.foo, foo);
`,
		},
	})
}

func TestTSImportStarES6ExportStarAsUnused(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.ts
let foo = 234;
console.log(foo);
`,
		},
	})
}

func TestTSImportStarES6ExportStarAsNoCapture(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.ts
const foo_exports = {};
__export(foo_exports, {
  foo: () => foo2
});
const foo2 = 123;

// /bar.ts

// /entry.ts
let foo = 234;
console.log(foo_exports.foo, foo_exports.foo, foo);
`,
		},
	})
}

func TestTSImportStarES6ExportStarAsCapture(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.ts
const foo_exports = {};
__export(foo_exports, {
  foo: () => foo2
});
const foo2 = 123;

// /bar.ts

// /entry.ts
let foo = 234;
console.log(foo_exports, foo_exports.foo, foo);
`,
		},
	})
}

func TestTSImportStarES6ExportStarUnused(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.ts
let foo = 234;
console.log(foo);
`,
		},
	})
}

func TestTSImportStarES6ExportStarNoCapture(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.ts
const foo2 = 123;

// /bar.ts

// /entry.ts
let foo = 234;
console.log(foo2, foo2, foo);
`,
		},
	})
}

func TestTSImportStarES6ExportStarCapture(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.ts
const foo2 = 123;

// /bar.ts
const bar_exports = {};
__export(bar_exports, {
  foo: () => foo2
});

// /entry.ts
let foo = 234;
console.log(bar_exports, foo2, foo);
`,
		},
	})
}

func TestTSImportStarCommonJSUnused(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.ts
let foo = 234;
console.log(foo);
`,
		},
	})
}

func TestTSImportStarCommonJSCapture(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.ts
var require_foo = __commonJS((exports) => {
  exports.foo = 123;
});

// /entry.ts
const ns = __toModule(require_foo());
let foo = 234;
console.log(ns, ns.foo, foo);
`,
		},
	})
}

func TestTSImportStarCommonJSNoCapture(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.ts
var require_foo = __commonJS((exports) => {
  exports.foo = 123;
});

// /entry.ts
const ns = __toModule(require_foo());
let foo = 234;
console.log(ns.foo, ns.foo, foo);
`,
		},
	})
}

func TestTSImportStarES6AndCommonJS(t *testing.T) {
	expectBundled(t, bundled{
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
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.ts
var require_foo = __commonJS((exports) => {
  __export(exports, {
    foo: () => foo2
  });
  const foo2 = 123;
});

// /entry.js
const ns = __toModule(require_foo());
const ns2 = require_foo();
console.log(ns.foo, ns2.foo);
`,
		},
	})
}

func TestTSImportStarES6NoBundleUnused(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './foo'
				let foo = 234
				console.log(foo)
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `let foo = 234;
console.log(foo);
`,
		},
	})
}

func TestTSImportStarES6NoBundleCapture(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns, ns.foo, foo)
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `import * as ns from "./foo";
let foo = 234;
console.log(ns, ns.foo, foo);
`,
		},
	})
}

func TestTSImportStarES6NoBundleNoCapture(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns.foo, ns.foo, foo)
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `import * as ns from "./foo";
let foo = 234;
console.log(ns.foo, ns.foo, foo);
`,
		},
	})
}

func TestTSImportStarES6MangleNoBundleUnused(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './foo'
				let foo = 234
				console.log(foo)
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling:   false,
			MangleSyntax: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `let foo = 234;
console.log(foo);
`,
		},
	})
}

func TestTSImportStarES6MangleNoBundleCapture(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns, ns.foo, foo)
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling:   false,
			MangleSyntax: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `import * as ns from "./foo";
let foo = 234;
console.log(ns, ns.foo, foo);
`,
		},
	})
}

func TestTSImportStarES6MangleNoBundleNoCapture(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns.foo, ns.foo, foo)
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling:   false,
			MangleSyntax: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `import * as ns from "./foo";
let foo = 234;
console.log(ns.foo, ns.foo, foo);
`,
		},
	})
}
