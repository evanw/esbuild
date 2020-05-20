package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/parser"
)

func TestImportStarUnused(t *testing.T) {
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

// /entry.js
let foo = 234;
console.log(foo);
`,
		},
	})
}

func TestImportStarCapture(t *testing.T) {
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

func TestImportStarNoCapture(t *testing.T) {
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

func TestImportStarExportImportStarUnused(t *testing.T) {
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

// /bar.js

// /entry.js
let foo = 234;
console.log(foo);
`,
		},
	})
}

func TestImportStarExportImportStarNoCapture(t *testing.T) {
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

func TestImportStarExportImportStarCapture(t *testing.T) {
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

func TestImportStarExportStarAsUnused(t *testing.T) {
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

// /bar.js

// /entry.js
let foo = 234;
console.log(foo);
`,
		},
	})
}

func TestImportStarExportStarAsNoCapture(t *testing.T) {
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

func TestImportStarExportStarAsCapture(t *testing.T) {
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

func TestImportStarExportStarUnused(t *testing.T) {
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

// /bar.js

// /entry.js
let foo = 234;
console.log(foo);
`,
		},
	})
}

func TestImportStarExportStarNoCapture(t *testing.T) {
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

func TestImportStarExportStarCapture(t *testing.T) {
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

func TestImportStarAndCommonJS(t *testing.T) {
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

func TestImportStarNoBundleUnused(t *testing.T) {
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

func TestImportStarNoBundleCapture(t *testing.T) {
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

func TestImportStarNoBundleNoCapture(t *testing.T) {
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

func TestImportStarMangleNoBundleUnused(t *testing.T) {
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

func TestImportStarMangleNoBundleCapture(t *testing.T) {
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

func TestImportStarMangleNoBundleNoCapture(t *testing.T) {
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

func TestImportStarExportStarOmitAmbiguous(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './common'
				console.log(ns)
			`,
			"/common.js": `
				export * from './foo'
				export * from './bar'
			`,
			"/foo.js": `
				export const x = 1
				export const y = 2
			`,
			"/bar.js": `
				export const y = 3
				export const z = 4
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
const x = 1;

// /bar.js
const z = 4;

// /common.js
const common_exports = {};
__export(common_exports, {
  x: () => x,
  z: () => z
});

// /entry.js
console.log(common_exports);
`,
		},
	})
}

func TestImportExportStarAmbiguousError(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {x, y, z} from './common'
				console.log(x, y, z)
			`,
			"/common.js": `
				export * from './foo'
				export * from './bar'
			`,
			"/foo.js": `
				export const x = 1
				export const y = 2
			`,
			"/bar.js": `
				export const y = 3
				export const z = 4
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
		expectedCompileLog: "/entry.js: error: Ambiguous import \"y\" has multiple matching exports\n",
	})
}

func TestImportStarOfExportStarAs(t *testing.T) {
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
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
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

func TestImportOfExportStar(t *testing.T) {
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
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
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

func TestImportOfExportStarOfImport(t *testing.T) {
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
				export {value as bar} from './baz'
			`,
			"/baz.js": `
				export const value = 123
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
			"/out.js": `// /baz.js
const value = 123;

// /bar.js

// /foo.js
statement();
statement();
statement();
statement();

// /entry.js
console.log(value);
`,
		},
	})
}
