package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/parser"
)

func TestImportStarES6Unused(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				let foo = 234
				console.log(foo)
			`,
			"/foo.js": `
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
			"/out.js": `// /foo.js
const foo2 = 123;

// /entry.js
let foo = 234;
console.log(foo);
`,
		},
	})
}

func TestImportStarES6Capture(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns, ns.foo, foo)
			`,
			"/foo.js": `
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
			"/out.js": `// /foo.js
const foo_exports = {};
__export(foo_exports, {
  foo: () => foo2
});
const foo2 = 123;

// /entry.js
let foo = 234;
console.log(foo_exports, foo2, foo);
`,
		},
	})
}

func TestImportStarES6NoCapture(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns.foo, ns.foo, foo)
			`,
			"/foo.js": `
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
			"/out.js": `// /foo.js
const foo2 = 123;

// /entry.js
let foo = 234;
console.log(foo2, foo2, foo);
`,
		},
	})
}

func TestImportStarES6ExportImportStarUnused(t *testing.T) {
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
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.js
const foo_exports = {};
__export(foo_exports, {
  foo: () => foo2
});
const foo2 = 123;

// /bar.js

// /entry.js
let foo = 234;
console.log(foo);
`,
		},
	})
}

func TestImportStarES6ExportImportStarNoCapture(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {ns} from './bar'
				let foo = 234
				console.log(ns.foo, ns.foo, foo)
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
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.js
const foo_exports = {};
__export(foo_exports, {
  foo: () => foo2
});
const foo2 = 123;

// /bar.js

// /entry.js
let foo = 234;
console.log(foo_exports.foo, foo_exports.foo, foo);
`,
		},
	})
}

func TestImportStarES6ExportImportStarCapture(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {ns} from './bar'
				let foo = 234
				console.log(ns, ns.foo, foo)
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
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.js
const foo_exports = {};
__export(foo_exports, {
  foo: () => foo2
});
const foo2 = 123;

// /bar.js

// /entry.js
let foo = 234;
console.log(foo_exports, foo_exports.foo, foo);
`,
		},
	})
}

func TestImportStarES6ExportStarAsUnused(t *testing.T) {
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
				export * as ns from './foo'
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
			"/out.js": `// /foo.js
const foo_exports = {};
__export(foo_exports, {
  foo: () => foo2
});
const foo2 = 123;

// /bar.js

// /entry.js
let foo = 234;
console.log(foo);
`,
		},
	})
}

func TestImportStarES6ExportStarAsNoCapture(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {ns} from './bar'
				let foo = 234
				console.log(ns.foo, ns.foo, foo)
			`,
			"/foo.js": `
				export const foo = 123
			`,
			"/bar.js": `
				export * as ns from './foo'
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
			"/out.js": `// /foo.js
const foo_exports = {};
__export(foo_exports, {
  foo: () => foo2
});
const foo2 = 123;

// /bar.js

// /entry.js
let foo = 234;
console.log(foo_exports.foo, foo_exports.foo, foo);
`,
		},
	})
}

func TestImportStarES6ExportStarAsCapture(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {ns} from './bar'
				let foo = 234
				console.log(ns, ns.foo, foo)
			`,
			"/foo.js": `
				export const foo = 123
			`,
			"/bar.js": `
				export * as ns from './foo'
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
			"/out.js": `// /foo.js
const foo_exports = {};
__export(foo_exports, {
  foo: () => foo2
});
const foo2 = 123;

// /bar.js

// /entry.js
let foo = 234;
console.log(foo_exports, foo_exports.foo, foo);
`,
		},
	})
}

func TestImportStarES6ExportStarUnused(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './bar'
				let foo = 234
				console.log(foo)
			`,
			"/foo.js": `
				export const foo = 123
			`,
			"/bar.js": `
				export * from './foo'
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
			"/out.js": `// /foo.js
const foo2 = 123;

// /bar.js

// /entry.js
let foo = 234;
console.log(foo);
`,
		},
	})
}

func TestImportStarES6ExportStarNoCapture(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './bar'
				let foo = 234
				console.log(ns.foo, ns.foo, foo)
			`,
			"/foo.js": `
				export const foo = 123
			`,
			"/bar.js": `
				export * from './foo'
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
			"/out.js": `// /foo.js
const foo2 = 123;

// /bar.js

// /entry.js
let foo = 234;
console.log(foo2, foo2, foo);
`,
		},
	})
}

func TestImportStarES6ExportStarCapture(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './bar'
				let foo = 234
				console.log(ns, ns.foo, foo)
			`,
			"/foo.js": `
				export const foo = 123
			`,
			"/bar.js": `
				export * from './foo'
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
			"/out.js": `// /foo.js
const foo2 = 123;

// /bar.js
const bar_exports = {};
__export(bar_exports, {
  foo: () => foo2
});

// /entry.js
let foo = 234;
console.log(bar_exports, foo2, foo);
`,
		},
	})
}

func TestImportStarCommonJSUnused(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				let foo = 234
				console.log(foo)
			`,
			"/foo.js": `
				exports.foo = 123
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
			"/out.js": `// /foo.js
var require_foo = __commonJS((exports) => {
  exports.foo = 123;
});

// /entry.js
const ns = __toModule(require_foo());
let foo = 234;
console.log(foo);
`,
		},
	})
}

func TestImportStarCommonJSCapture(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns, ns.foo, foo)
			`,
			"/foo.js": `
				exports.foo = 123
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
			"/out.js": `// /foo.js
var require_foo = __commonJS((exports) => {
  exports.foo = 123;
});

// /entry.js
const ns = __toModule(require_foo());
let foo = 234;
console.log(ns, ns.foo, foo);
`,
		},
	})
}

func TestImportStarCommonJSNoCapture(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns.foo, ns.foo, foo)
			`,
			"/foo.js": `
				exports.foo = 123
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
			"/out.js": `// /foo.js
var require_foo = __commonJS((exports) => {
  exports.foo = 123;
});

// /entry.js
const ns = __toModule(require_foo());
let foo = 234;
console.log(ns.foo, ns.foo, foo);
`,
		},
	})
}

func TestImportStarES6AndCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				const ns2 = require('./foo')
				console.log(ns.foo, ns2.foo)
			`,
			"/foo.js": `
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
			"/out.js": `// /foo.js
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

func TestImportStarES6NoBundleUnused(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				let foo = 234
				console.log(foo)
			`,
		},
		entryPaths: []string{"/entry.js"},
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
console.log(foo);
`,
		},
	})
}

func TestImportStarES6NoBundleCapture(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns, ns.foo, foo)
			`,
		},
		entryPaths: []string{"/entry.js"},
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

func TestImportStarES6NoBundleNoCapture(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns.foo, ns.foo, foo)
			`,
		},
		entryPaths: []string{"/entry.js"},
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

func TestImportStarES6MangleNoBundleUnused(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				let foo = 234
				console.log(foo)
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling:   false,
			MangleSyntax: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `import "./foo";
let foo = 234;
console.log(foo);
`,
		},
	})
}

func TestImportStarES6MangleNoBundleCapture(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns, ns.foo, foo)
			`,
		},
		entryPaths: []string{"/entry.js"},
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

func TestImportStarES6MangleNoBundleNoCapture(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				let foo = 234
				console.log(ns.foo, ns.foo, foo)
			`,
		},
		entryPaths: []string{"/entry.js"},
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
