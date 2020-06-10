package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/parser"
	"github.com/evanw/esbuild/internal/printer"
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

func TestExportSelfIIFE(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export const foo = 123
				export * from './entry'
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			OutputFormat:  printer.FormatIIFE,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `(() => {
  // /entry.js
  var require_entry = __commonJS((exports) => {
    __export(exports, {
      foo: () => foo
    });
    const foo = 123;
  });
  require_entry();
})();
`,
		},
	})
}

func TestExportSelfES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export const foo = 123
				export * from './entry'
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			OutputFormat:  printer.FormatESModule,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
const foo = 123;
export {foo};
`,
		},
	})
}

func TestExportSelfCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export const foo = 123
				export * from './entry'
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			OutputFormat:  printer.FormatCommonJS,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
var require_entry = __commonJS((exports) => {
  __export(exports, {
    foo: () => foo
  });
  const foo = 123;
});
module.exports = require_entry();
`,
		},
	})
}

func TestExportSelfCommonJSMinified(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				module.exports = {foo: 123}
				console.log(require('./entry'))
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:        true,
			MinifyIdentifiers: true,
			OutputFormat:      printer.FormatCommonJS,
			AbsOutputFile:     "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
var b = c((d, e) => {
  e.exports = {foo: 123};
  console.log(b());
});
module.exports = b();
`,
		},
	})
}

func TestImportSelfCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				exports.foo = 123
				import {foo} from './entry'
				console.log(foo)
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			OutputFormat:  printer.FormatCommonJS,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
var require_entry = __commonJS((exports) => {
  const entry = __toModule(require_entry());
  exports.foo = 123;
  console.log(entry.foo);
});
module.exports = require_entry();
`,
		},
	})
}

func TestExportSelfAsNamespaceES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export const foo = 123
				export * as ns from './entry'
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			OutputFormat:  printer.FormatESModule,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
const entry_exports = {};
__export(entry_exports, {
  foo: () => foo,
  ns: () => entry_exports
});
const foo = 123;
export {foo, entry_exports as ns};
`,
		},
	})
}

func TestImportExportSelfAsNamespaceES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export const foo = 123
				import * as ns from './entry'
				export {ns}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			OutputFormat:  printer.FormatESModule,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
const entry_exports = {};
__export(entry_exports, {
  foo: () => foo,
  ns: () => entry_exports
});
const foo = 123;
export {foo, entry_exports as ns};
`,
		},
	})
}

func TestReExportOtherFileExportSelfAsNamespaceES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export * from './foo'
			`,
			"/foo.js": `
				export const foo = 123
				export * as ns from './foo'
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			OutputFormat:  printer.FormatESModule,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.js
const foo_exports = {};
__export(foo_exports, {
  foo: () => foo,
  ns: () => foo_exports
});
const foo = 123;

// /entry.js
export {foo, foo_exports as ns};
`,
		},
	})
}

func TestReExportOtherFileImportExportSelfAsNamespaceES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export * from './foo'
			`,
			"/foo.js": `
				export const foo = 123
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
			OutputFormat:  printer.FormatESModule,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.js
const foo_exports = {};
__export(foo_exports, {
  foo: () => foo,
  ns: () => foo_exports
});
const foo = 123;

// /entry.js
export {foo, foo_exports as ns};
`,
		},
	})
}

func TestOtherFileExportSelfAsNamespaceUnusedES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export {foo} from './foo'
			`,
			"/foo.js": `
				export const foo = 123
				export * as ns from './foo'
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			OutputFormat:  printer.FormatESModule,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.js
const foo2 = 123;

// /entry.js
export {foo2 as foo};
`,
		},
	})
}

func TestOtherFileImportExportSelfAsNamespaceUnusedES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export {foo} from './foo'
			`,
			"/foo.js": `
				export const foo = 123
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
			OutputFormat:  printer.FormatESModule,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.js
const foo2 = 123;

// /entry.js
export {foo2 as foo};
`,
		},
	})
}

func TestExportSelfAsNamespaceCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export const foo = 123
				export * as ns from './entry'
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			OutputFormat:  printer.FormatCommonJS,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
var require_entry = __commonJS((exports) => {
  __export(exports, {
    foo: () => foo,
    ns: () => ns
  });
  const ns = __toModule(require_entry());
  const foo = 123;
});
module.exports = require_entry();
`,
		},
	})
}

func TestExportSelfAndRequireSelfCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export const foo = 123
				console.log(require('./entry'))
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			OutputFormat:  printer.FormatCommonJS,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
var require_entry = __commonJS((exports) => {
  __export(exports, {
    foo: () => foo
  });
  const foo = 123;
  console.log(require_entry());
});
module.exports = require_entry();
`,
		},
	})
}

func TestExportSelfAndImportSelfCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as x from './entry'
				export const foo = 123
				console.log(x)
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			OutputFormat:  printer.FormatCommonJS,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
var require_entry = __commonJS((exports) => {
  __export(exports, {
    foo: () => foo
  });
  const x = __toModule(require_entry());
  const foo = 123;
  console.log(x);
});
module.exports = require_entry();
`,
		},
	})
}

func TestExportOtherAsNamespaceCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export * as ns from './foo'
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
			OutputFormat:  printer.FormatCommonJS,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.js
var require_foo = __commonJS((exports2) => {
  exports2.foo = 123;
});

// /entry.js
__export(exports, {
  ns: () => ns
});
const ns = __toModule(require_foo());
`,
		},
	})
}

func TestImportExportOtherAsNamespaceCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				export {ns}
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
			OutputFormat:  printer.FormatCommonJS,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.js
var require_foo = __commonJS((exports2) => {
  exports2.foo = 123;
});

// /entry.js
__export(exports, {
  ns: () => ns
});
const ns = __toModule(require_foo());
`,
		},
	})
}

func TestNamespaceImportMissingES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				console.log(ns, ns.foo)
			`,
			"/foo.js": `
				export const x = 123
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
  x: () => x
});
const x = 123;

// /entry.js
console.log(foo_exports, void 0);
`,
		},
	})
}

func TestExportOtherCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export {bar} from './foo'
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
			OutputFormat:  printer.FormatCommonJS,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.js
var require_foo = __commonJS((exports2) => {
  exports2.foo = 123;
});

// /entry.js
__export(exports, {
  bar: () => foo.bar
});
const foo = __toModule(require_foo());
`,
		},
	})
}

func TestExportOtherNestedCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export {y} from './bar'
			`,
			"/bar.js": `
				export {x as y} from './foo'
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
			OutputFormat:  printer.FormatCommonJS,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /foo.js
var require_foo = __commonJS((exports2) => {
  exports2.foo = 123;
});

// /bar.js
const foo = __toModule(require_foo());

// /entry.js
__export(exports, {
  y: () => foo.x
});
`,
		},
	})
}

func TestNamespaceImportUnusedMissingES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				console.log(ns.foo)
			`,
			"/foo.js": `
				export const x = 123
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
console.log(void 0);
`,
		},
	})
}

func TestNamespaceImportMissingCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				console.log(ns, ns.foo)
			`,
			"/foo.js": `
				exports.x = 123
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
  exports.x = 123;
});

// /entry.js
const ns = __toModule(require_foo());
console.log(ns, ns.foo);
`,
		},
	})
}

func TestNamespaceImportUnusedMissingCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				console.log(ns.foo)
			`,
			"/foo.js": `
				exports.x = 123
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
  exports.x = 123;
});

// /entry.js
const ns = __toModule(require_foo());
console.log(ns.foo);
`,
		},
	})
}

func TestReExportNamespaceImportMissingES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {ns} from './foo'
				console.log(ns, ns.foo)
			`,
			"/foo.js": `
				export * as ns from './bar'
			`,
			"/bar.js": `
				export const x = 123
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
  x: () => x
});
const x = 123;

// /foo.js

// /entry.js
console.log(bar_exports, bar_exports.foo);
`,
		},
	})
}

func TestReExportNamespaceImportUnusedMissingES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {ns} from './foo'
				console.log(ns.foo)
			`,
			"/foo.js": `
				export * as ns from './bar'
			`,
			"/bar.js": `
				export const x = 123
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
  x: () => x
});
const x = 123;

// /foo.js

// /entry.js
console.log(bar_exports.foo);
`,
		},
	})
}

func TestNamespaceImportReExportMissingES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				console.log(ns, ns.foo)
			`,
			"/foo.js": `
				export {foo} from './bar'
			`,
			"/bar.js": `
				export const x = 123
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
		expectedCompileLog: `/foo.js: error: No matching export for import "foo"
/foo.js: error: No matching export for import "foo"
`,
	})
}

func TestNamespaceImportReExportUnusedMissingES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				console.log(ns.foo)
			`,
			"/foo.js": `
				export {foo} from './bar'
			`,
			"/bar.js": `
				export const x = 123
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
		expectedCompileLog: `/foo.js: error: No matching export for import "foo"
/foo.js: error: No matching export for import "foo"
`,
	})
}

func TestNamespaceImportReExportStarMissingES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				console.log(ns, ns.foo)
			`,
			"/foo.js": `
				export * from './bar'
			`,
			"/bar.js": `
				export const x = 123
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
const x = 123;

// /foo.js
const foo_exports = {};
__export(foo_exports, {
  x: () => x
});

// /entry.js
console.log(foo_exports, void 0);
`,
		},
	})
}

func TestNamespaceImportReExportStarUnusedMissingES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				console.log(ns.foo)
			`,
			"/foo.js": `
				export * from './bar'
			`,
			"/bar.js": `
				export const x = 123
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

// /foo.js

// /entry.js
console.log(void 0);
`,
		},
	})
}

func TestIssue176(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as things from './folders'
				console.log(JSON.stringify(things))
			`,
			"/folders/index.js": `
				export * from "./child"
			`,
			"/folders/child/index.js": `
				export { foo } from './foo'
			`,
			"/folders/child/foo.js": `
				export const foo = () => 'hi there'
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
			"/out.js": `// /folders/child/foo.js
const foo = () => "hi there";

// /folders/child/index.js

// /folders/index.js
const index_exports = {};
__export(index_exports, {
  foo: () => foo
});

// /entry.js
console.log(JSON.stringify(index_exports));
`,
		},
	})
}
