package bundler

import (
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/logging"
	"github.com/evanw/esbuild/internal/parser"
	"github.com/evanw/esbuild/internal/printer"
	"github.com/evanw/esbuild/internal/resolver"
	"github.com/kylelemons/godebug/diff"
)

func assertEqual(t *testing.T, a interface{}, b interface{}) {
	if a != b {
		stringA := fmt.Sprintf("%v", a)
		stringB := fmt.Sprintf("%v", b)
		if strings.Contains(stringA, "\n") {
			t.Fatal(diff.Diff(stringB, stringA))
		} else {
			t.Fatalf("%s != %s", a, b)
		}
	}
}

func assertLog(t *testing.T, msgs []logging.Msg, expected string) {
	text := ""
	for _, msg := range msgs {
		text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
	}
	assertEqual(t, text, expected)
}

func hasErrors(msgs []logging.Msg) bool {
	for _, msg := range msgs {
		if msg.Kind == logging.Error {
			return true
		}
	}
	return false
}

type bundled struct {
	files              map[string]string
	entryPaths         []string
	expected           map[string]string
	expectedScanLog    string
	expectedCompileLog string
	parseOptions       parser.ParseOptions
	bundleOptions      BundleOptions
	resolveOptions     resolver.ResolveOptions
}

func expectBundled(t *testing.T, args bundled) {
	t.Run("", func(t *testing.T) {
		fs := fs.MockFS(args.files)
		args.resolveOptions.ExtensionOrder = []string{".tsx", ".ts", ".jsx", ".js", ".json"}
		log, join := logging.NewDeferLog()
		resolver := resolver.NewResolver(fs, log, args.resolveOptions)
		bundle := ScanBundle(log, fs, resolver, args.entryPaths, args.parseOptions, args.bundleOptions)
		msgs := join()
		assertLog(t, msgs, args.expectedScanLog)

		// Stop now if there were any errors during the scan
		if hasErrors(msgs) {
			return
		}

		log, join = logging.NewDeferLog()
		args.bundleOptions.omitRuntimeForTests = true
		if args.bundleOptions.AbsOutputFile != "" {
			args.bundleOptions.AbsOutputDir = path.Dir(args.bundleOptions.AbsOutputFile)
		}
		results := bundle.Compile(log, args.bundleOptions)
		msgs = join()
		assertLog(t, msgs, args.expectedCompileLog)

		// Stop now if there were any errors during the compile
		if hasErrors(msgs) {
			return
		}

		// Don't include source maps in results since they are just noise. Source
		// map validity is tested separately in a test that uses Mozilla's source
		// map parsing library.
		resultsWithoutSourceMaps := []OutputFile{}
		for _, result := range results {
			if !strings.HasSuffix(result.AbsPath, ".map") {
				resultsWithoutSourceMaps = append(resultsWithoutSourceMaps, result)
			}
		}

		assertEqual(t, len(resultsWithoutSourceMaps), len(args.expected))
		for _, result := range resultsWithoutSourceMaps {
			file := args.expected[result.AbsPath]
			path := "[" + result.AbsPath + "]\n"
			assertEqual(t, path+string(result.Contents), path+file)
		}
	})
}

func TestSimpleES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {fn} from './foo'
				console.log(fn())
			`,
			"/foo.js": `
				export function fn() {
					return 123
				}
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
function fn() {
  return 123;
}

// /entry.js
console.log(fn());
`,
		},
	})
}

func TestSimpleCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const fn = require('./foo')
				console.log(fn())
			`,
			"/foo.js": `
				module.exports = function() {
					return 123
				}
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
var require_foo = __commonJS((exports, module) => {
  module.exports = function() {
    return 123;
  };
});

// /entry.js
const fn = require_foo();
console.log(fn());
`,
		},
	})
}

// This test makes sure that require() calls are still recognized in nested
// scopes. It guards against bugs where require() calls are only recognized in
// the top-level module scope.
func TestNestedCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function nestedScope() {
					const fn = require('./foo')
					console.log(fn())
				}
				nestedScope()
			`,
			"/foo.js": `
				module.exports = function() {
					return 123
				}
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
var require_foo = __commonJS((exports, module) => {
  module.exports = function() {
    return 123;
  };
});

// /entry.js
function nestedScope() {
  const fn = require_foo();
  console.log(fn());
}
nestedScope();
`,
		},
	})
}

func TestCommonJSFromES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const {foo} = require('./foo')
				console.log(foo(), bar())
				const {bar} = require('./bar') // This should not be hoisted
			`,
			"/foo.js": `
				export function foo() {
					return 'foo'
				}
			`,
			"/bar.js": `
				export function bar() {
					return 'bar'
				}
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
  function foo2() {
    return "foo";
  }
});

// /bar.js
var require_bar = __commonJS((exports) => {
  __export(exports, {
    bar: () => bar2
  });
  function bar2() {
    return "bar";
  }
});

// /entry.js
const {foo} = require_foo();
console.log(foo(), bar());
const {bar} = require_bar();
`,
		},
	})
}

func TestES6FromCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {foo} from './foo'
				console.log(foo(), bar())
				import {bar} from './bar' // This should be hoisted
			`,
			"/foo.js": `
				exports.foo = function() {
					return 'foo'
				}
			`,
			"/bar.js": `
				exports.bar = function() {
					return 'bar'
				}
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
  exports.foo = function() {
    return "foo";
  };
});

// /bar.js
var require_bar = __commonJS((exports) => {
  exports.bar = function() {
    return "bar";
  };
});

// /entry.js
const foo = __toModule(require_foo());
const bar = __toModule(require_bar());
console.log(foo.foo(), bar.bar());
`,
		},
	})
}

// This test makes sure that ES6 imports are still recognized in nested
// scopes. It guards against bugs where require() calls are only recognized in
// the top-level module scope.
func TestNestedES6FromCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {fn} from './foo'
				(() => {
					console.log(fn())
				})()
			`,
			"/foo.js": `
				exports.fn = function() {
					return 123
				}
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
  exports.fn = function() {
    return 123;
  };
});

// /entry.js
const foo = __toModule(require_foo());
(() => {
  console.log(foo.fn());
})();
`,
		},
	})
}

func TestExportFormsES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export default 123
				export var v = 234
				export let l = 234
				export const c = 234
				export {Class as C}
				export function Fn() {}
				export class Class {}
				export * from './a'
				export * as b from './b'
			`,
			"/a.js": "export const abc = undefined",
			"/b.js": "export const xyz = null",
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
			"/out.js": `// /a.js
const abc = void 0;

// /b.js
const b_exports = {};
__export(b_exports, {
  xyz: () => xyz
});
const xyz = null;

// /entry.js
const entry_default = 123;
var v = 234;
let l = 234;
const c = 234;
function Fn() {
}
class Class {
}
export {Class as C, Class, Fn, abc, b_exports as b, c, entry_default as default, l, v};
`,
		},
	})
}

func TestExportFormsIIFE(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export default 123
				export var v = 234
				export let l = 234
				export const c = 234
				export {Class as C}
				export function Fn() {}
				export class Class {}
				export * from './a'
				export * as b from './b'
			`,
			"/a.js": "export const abc = undefined",
			"/b.js": "export const xyz = null",
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			OutputFormat:  printer.FormatIIFE,
			ModuleName:    "moduleName",
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `var moduleName = (() => {
  // /entry.js
  var require_entry = __commonJS((exports) => {
    __export(exports, {
      C: () => Class,
      Class: () => Class,
      Fn: () => Fn,
      abc: () => abc,
      b: () => b_exports,
      c: () => c,
      default: () => entry_default,
      l: () => l,
      v: () => v
    });
    const entry_default = 123;
    var v = 234;
    let l = 234;
    const c = 234;
    function Fn() {
    }
    class Class {
    }
  });

  // /a.js
  const abc = void 0;

  // /b.js
  const b_exports = {};
  __export(b_exports, {
    xyz: () => xyz
  });
  const xyz = null;
  return require_entry();
})();
`,
		},
	})
}

func TestExportFormsWithMinifyIdentifiersAndNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				export default 123
				export var varName = 234
				export let letName = 234
				export const constName = 234
				function Func2() {}
				class Class2 {}
				export {Class as Cls, Func2 as Fn2, Class2 as Cls2}
				export function Func() {}
				export class Class {}
				export * from './a'
				export * as fromB from './b'
			`,
			"/b.js": "export default function() {}",
			"/c.js": "export default function foo() {}",
			"/d.js": "export default class {}",
			"/e.js": "export default class Foo {}",
		},
		entryPaths: []string{
			"/a.js",
			"/b.js",
			"/c.js",
			"/d.js",
			"/e.js",
		},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
		},
		bundleOptions: BundleOptions{
			IsBundling:        false,
			MinifyIdentifiers: true,
			AbsOutputDir:      "/out",
		},
		expected: map[string]string{
			"/out/a.js": `export default 123;
export var varName = 234;
export let letName = 234;
export const constName = 234;
function a() {
}
class b {
}
export {Class as Cls, a as Fn2, b as Cls2};
export function Func() {
}
export class Class {
}
export * from "./a";
export * as fromB from "./b";
`,
			"/out/b.js": `export default function() {
}
`,
			"/out/c.js": `export default function a() {
}
`,
			"/out/d.js": `export default class {
}
`,
			"/out/e.js": `export default class a {
}
`,
		},
	})
}

func TestImportFormsWithNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import 'foo'
				import {} from 'foo'
				import * as ns from 'foo'
				import {a, b as c} from 'foo'
				import def from 'foo'
				import def2, * as ns2 from 'foo'
				import def3, {a2, b as c3} from 'foo'
				const imp = [
					import('foo'),
					function nested() { return import('foo') },
				]
				console.log(ns, a, c, def, def2, ns2, def3, a2, c3, imp)
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
			"/out.js": `import "foo";
import {} from "foo";
import * as ns from "foo";
import {a, b as c} from "foo";
import def from "foo";
import def2, * as ns2 from "foo";
import def3, {a2, b as c3} from "foo";
const imp = [
  import("foo"),
  function nested() {
    return import("foo");
  }
];
console.log(ns, a, c, def, def2, ns2, def3, a2, c3, imp);
`,
		},
	})
}

func TestImportFormsWithMinifyIdentifiersAndNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import 'foo'
				import {} from 'foo'
				import * as ns from 'foo'
				import {a, b as c} from 'foo'
				import def from 'foo'
				import def2, * as ns2 from 'foo'
				import def3, {a2, b as c3} from 'foo'
				const imp = [
					import('foo'),
					function() { return import('foo') },
				]
				console.log(ns, a, c, def, def2, ns2, def3, a2, c3, imp)
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
		},
		bundleOptions: BundleOptions{
			IsBundling:        false,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
		expected: map[string]string{
			"/out.js": `import "foo";
import {} from "foo";
import * as a from "foo";
import {a as b, b as c} from "foo";
import d from "foo";
import f, * as e from "foo";
import g, {a2 as h, b as i} from "foo";
const j = [
  import("foo"),
  function() {
    return import("foo");
  }
];
console.log(a, b, c, d, f, e, g, h, i, j);
`,
		},
	})
}

func TestExportFormsCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				require('./commonjs')
				require('./c')
				require('./d')
				require('./e')
				require('./f')
				require('./g')
				require('./h')
			`,
			"/commonjs.js": `
				export default 123
				export var v = 234
				export let l = 234
				export const c = 234
				export {Class as C}
				export function Fn() {}
				export class Class {}
				export * from './a'
				export * as b from './b'
			`,
			"/a.js": "export const abc = undefined",
			"/b.js": "export const xyz = null",
			"/c.js": "export default class {}",
			"/d.js": "export default class Foo {} Foo.prop = 123",
			"/e.js": "export default function() {}",
			"/f.js": "export default function foo() {} foo.prop = 123",
			"/g.js": "export default async function() {}",
			"/h.js": "export default async function foo() {} foo.prop = 123",
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
			"/out.js": `// /commonjs.js
var require_commonjs = __commonJS((exports) => {
  __export(exports, {
    C: () => Class,
    Class: () => Class,
    Fn: () => Fn,
    abc: () => abc,
    b: () => b_exports,
    c: () => c,
    default: () => commonjs_default,
    l: () => l,
    v: () => v
  });
  const commonjs_default = 123;
  var v = 234;
  let l = 234;
  const c = 234;
  function Fn() {
  }
  class Class {
  }
});

// /c.js
var require_c = __commonJS((exports) => {
  __export(exports, {
    default: () => c_default
  });
  class c_default {
  }
});

// /d.js
var require_d = __commonJS((exports) => {
  __export(exports, {
    default: () => Foo
  });
  class Foo {
  }
  Foo.prop = 123;
});

// /e.js
var require_e = __commonJS((exports) => {
  __export(exports, {
    default: () => e_default
  });
  function e_default() {
  }
});

// /f.js
var require_f = __commonJS((exports) => {
  __export(exports, {
    default: () => foo
  });
  function foo() {
  }
  foo.prop = 123;
});

// /g.js
var require_g = __commonJS((exports) => {
  __export(exports, {
    default: () => g_default
  });
  async function g_default() {
  }
});

// /h.js
var require_h = __commonJS((exports) => {
  __export(exports, {
    default: () => foo
  });
  async function foo() {
  }
  foo.prop = 123;
});

// /a.js
const abc = void 0;

// /b.js
const b_exports = {};
__export(b_exports, {
  xyz: () => xyz
});
const xyz = null;

// /entry.js
require_commonjs();
require_c();
require_d();
require_e();
require_f();
require_g();
require_h();
`,
		},
	})
}

func TestReExportDefaultCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {foo as entry} from './foo'
				entry()
			`,
			"/foo.js": `
				export {default as foo} from './bar'
			`,
			"/bar.js": `
				export default function foo() {
					return exports // Force this to be a CommonJS module
				}
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
var require_bar = __commonJS((exports) => {
  __export(exports, {
    default: () => foo2
  });
  function foo2() {
    return exports;
  }
});

// /foo.js
const bar = __toModule(require_bar());

// /entry.js
bar.default();
`,
		},
	})
}

func TestExportChain(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export {b as a} from './foo'
			`,
			"/foo.js": `
				export {c as b} from './bar'
			`,
			"/bar.js": `
				export const c = 123
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
const c = 123;

// /foo.js

// /entry.js
export {c as a};
`,
		},
	})
}

func TestExportInfiniteCycle1(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export {a as b} from './entry'
				export {b as c} from './entry'
				export {c as d} from './entry'
				export {d as a} from './entry'
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
		expectedCompileLog: `/entry.js: error: Detected cycle while resolving import "a"
/entry.js: error: Detected cycle while resolving import "b"
/entry.js: error: Detected cycle while resolving import "c"
/entry.js: error: Detected cycle while resolving import "d"
`,
	})
}

func TestExportInfiniteCycle2(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export {a as b} from './foo'
				export {c as d} from './foo'
			`,
			"/foo.js": `
				export {b as c} from './entry'
				export {d as a} from './entry'
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
		expectedCompileLog: `/entry.js: error: Detected cycle while resolving import "a"
/entry.js: error: Detected cycle while resolving import "c"
/foo.js: error: Detected cycle while resolving import "b"
/foo.js: error: Detected cycle while resolving import "d"
`,
	})
}

func TestJSXImportsCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.jsx": `
				import {elem, frag} from './custom-react'
				console.log(<div/>, <>fragment</>)
			`,
			"/custom-react.js": `
				module.exports = {}
			`,
		},
		entryPaths: []string{"/entry.jsx"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
			JSX: parser.JSXOptions{
				Factory:  []string{"elem"},
				Fragment: []string{"frag"},
			},
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /custom-react.js
var require_custom_react = __commonJS((exports, module) => {
  module.exports = {};
});

// /entry.jsx
const custom_react = __toModule(require_custom_react());
console.log(custom_react.elem("div", null), custom_react.elem(custom_react.frag, null, "fragment"));
`,
		},
	})
}

func TestJSXImportsES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.jsx": `
				import {elem, frag} from './custom-react'
				console.log(<div/>, <>fragment</>)
			`,
			"/custom-react.js": `
				export function elem() {}
				export function frag() {}
			`,
		},
		entryPaths: []string{"/entry.jsx"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
			JSX: parser.JSXOptions{
				Factory:  []string{"elem"},
				Fragment: []string{"frag"},
			},
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /custom-react.js
function elem() {
}
function frag() {
}

// /entry.jsx
console.log(elem("div", null), elem(frag, null, "fragment"));
`,
		},
	})
}

func TestJSXSyntaxInJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(<div/>)
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
		expectedScanLog: `/entry.js: error: Unexpected "<"
`,
	})
}

func TestJSXSyntaxInJSWithJSXLoader(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(<div/>)
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]Loader{
				".js": LoaderJSX,
			},
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
console.log(React.createElement("div", null));
`,
		},
	})
}

func TestNodeModules(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/node_modules/demo-pkg/index.js
var require_demo_pkg = __commonJS((exports, module) => {
  module.exports = function() {
    return 123;
  };
});

// /Users/user/project/src/entry.js
const demo_pkg = __toModule(require_demo_pkg());
console.log(demo_pkg.default());
`,
		},
	})
}

func TestPackageJsonMain(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./custom-main.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/custom-main.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/node_modules/demo-pkg/custom-main.js
var require_custom_main = __commonJS((exports, module) => {
  module.exports = function() {
    return 123;
  };
});

// /Users/user/project/src/entry.js
const demo_pkg = __toModule(require_custom_main());
console.log(demo_pkg.default());
`,
		},
	})
}

func TestPackageJsonSyntaxErrorComment(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					// Single-line comment
					"a": 1
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: "/Users/user/project/node_modules/demo-pkg/package.json: error: JSON does not support comments\n",
	})
}

func TestPackageJsonSyntaxErrorTrailingComma(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"a": 1,
					"b": 2,
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: "/Users/user/project/node_modules/demo-pkg/package.json: error: JSON does not support trailing commas\n",
	})
}

func TestTsconfigJsonBaseUrl(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/app/entry.js": `
				import fn from 'lib/util'
				console.log(fn())
			`,
			"/Users/user/project/src/tsconfig.json": `
				{
					"compilerOptions": {
						"baseUrl": "."
					}
				}
			`,
			"/Users/user/project/src/lib/util.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/app/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/src/lib/util.js
var require_util = __commonJS((exports, module) => {
  module.exports = function() {
    return 123;
  };
});

// /Users/user/project/src/app/entry.js
const util = __toModule(require_util());
console.log(util.default());
`,
		},
	})
}

func TestJsconfigJsonBaseUrl(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/app/entry.js": `
				import fn from 'lib/util'
				console.log(fn())
			`,
			"/Users/user/project/src/jsconfig.json": `
				{
					"compilerOptions": {
						"baseUrl": "."
					}
				}
			`,
			"/Users/user/project/src/lib/util.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/app/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/src/lib/util.js
var require_util = __commonJS((exports, module) => {
  module.exports = function() {
    return 123;
  };
});

// /Users/user/project/src/app/entry.js
const util = __toModule(require_util());
console.log(util.default());
`,
		},
	})
}

func TestTsconfigJsonCommentAllowed(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/app/entry.js": `
				import fn from 'lib/util'
				console.log(fn())
			`,
			"/Users/user/project/src/tsconfig.json": `
				{
					// Single-line comment
					"compilerOptions": {
						"baseUrl": "."
					}
				}
			`,
			"/Users/user/project/src/lib/util.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/app/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/src/lib/util.js
var require_util = __commonJS((exports, module) => {
  module.exports = function() {
    return 123;
  };
});

// /Users/user/project/src/app/entry.js
const util = __toModule(require_util());
console.log(util.default());
`,
		},
	})
}

func TestTsconfigJsonTrailingCommaAllowed(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/app/entry.js": `
				import fn from 'lib/util'
				console.log(fn())
			`,
			"/Users/user/project/src/tsconfig.json": `
				{
					"compilerOptions": {
						"baseUrl": ".",
					},
				}
			`,
			"/Users/user/project/src/lib/util.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/app/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/src/lib/util.js
var require_util = __commonJS((exports, module) => {
  module.exports = function() {
    return 123;
  };
});

// /Users/user/project/src/app/entry.js
const util = __toModule(require_util());
console.log(util.default());
`,
		},
	})
}

func TestPackageJsonModule(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main.js",
					"module": "./main.esm.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.esm.js": `
				export default function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/node_modules/demo-pkg/main.esm.js
function main_esm_default() {
  return 123;
}

// /Users/user/project/src/entry.js
console.log(main_esm_default());
`,
		},
	})
}

func TestTsConfigPaths(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/entry.ts": `
				import baseurl_dot from './baseurl_dot'
				import baseurl_nested from './baseurl_nested'
				console.log(baseurl_dot, baseurl_nested)
			`,

			// Tests with "baseUrl": "."
			"/Users/user/project/baseurl_dot/index.ts": `
				import test0 from 'test0'
				import test1 from 'test1/foo'
				import test2 from 'test2/foo'
				import test3 from 'test3/foo'
				import test4 from 'test4/foo'
				import test5 from 'test5/foo'
				export default {
					test0,
					test1,
					test2,
					test3,
					test4,
					test5,
				}
			`,
			"/Users/user/project/baseurl_dot/tsconfig.json": `
				{
					"compilerOptions": {
						"baseUrl": ".",
						"paths": {
							"test0": ["./test0-success.ts"],
							"test1/*": ["./test1-success.ts"],
							"test2/*": ["./test2-success/*"],
							"t*t3/foo": ["./test3-succ*s.ts"],
							"test4/*": ["./test4-first/*", "./test4-second/*"],
							"test5/*": ["./test5-first/*", "./test5-second/*"],
						}
					}
				}
			`,
			"/Users/user/project/baseurl_dot/test0-success.ts": `
				export default 'test0-success'
			`,
			"/Users/user/project/baseurl_dot/test1-success.ts": `
				export default 'test1-success'
			`,
			"/Users/user/project/baseurl_dot/test2-success/foo.ts": `
				export default 'test2-success'
			`,
			"/Users/user/project/baseurl_dot/test3-success.ts": `
				export default 'test3-success'
			`,
			"/Users/user/project/baseurl_dot/test4-first/foo.ts": `
				export default 'test4-success'
			`,
			"/Users/user/project/baseurl_dot/test5-second/foo.ts": `
				export default 'test5-success'
			`,

			// Tests with "baseUrl": "nested"
			"/Users/user/project/baseurl_nested/index.ts": `
				import test0 from 'test0'
				import test1 from 'test1/foo'
				import test2 from 'test2/foo'
				import test3 from 'test3/foo'
				import test4 from 'test4/foo'
				import test5 from 'test5/foo'
				export default {
					test0,
					test1,
					test2,
					test3,
					test4,
					test5,
				}
			`,
			"/Users/user/project/baseurl_nested/tsconfig.json": `
				{
					"compilerOptions": {
						"baseUrl": "nested",
						"paths": {
							"test0": ["./test0-success.ts"],
							"test1/*": ["./test1-success.ts"],
							"test2/*": ["./test2-success/*"],
							"t*t3/foo": ["./test3-succ*s.ts"],
							"test4/*": ["./test4-first/*", "./test4-second/*"],
							"test5/*": ["./test5-first/*", "./test5-second/*"],
						}
					}
				}
			`,
			"/Users/user/project/baseurl_nested/nested/test0-success.ts": `
				export default 'test0-success'
			`,
			"/Users/user/project/baseurl_nested/nested/test1-success.ts": `
				export default 'test1-success'
			`,
			"/Users/user/project/baseurl_nested/nested/test2-success/foo.ts": `
				export default 'test2-success'
			`,
			"/Users/user/project/baseurl_nested/nested/test3-success.ts": `
				export default 'test3-success'
			`,
			"/Users/user/project/baseurl_nested/nested/test4-first/foo.ts": `
				export default 'test4-success'
			`,
			"/Users/user/project/baseurl_nested/nested/test5-second/foo.ts": `
				export default 'test5-success'
			`,
		},
		entryPaths: []string{"/Users/user/project/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/baseurl_dot/test0-success.ts
const test0_success_default = "test0-success";

// /Users/user/project/baseurl_dot/test1-success.ts
const test1_success_default = "test1-success";

// /Users/user/project/baseurl_dot/test2-success/foo.ts
const foo_default = "test2-success";

// /Users/user/project/baseurl_dot/test3-success.ts
const test3_success_default = "test3-success";

// /Users/user/project/baseurl_dot/test4-first/foo.ts
const foo_default2 = "test4-success";

// /Users/user/project/baseurl_dot/test5-second/foo.ts
const foo_default3 = "test5-success";

// /Users/user/project/baseurl_dot/index.ts
const baseurl_dot_default = {
  test0: test0_success_default,
  test1: test1_success_default,
  test2: foo_default,
  test3: test3_success_default,
  test4: foo_default2,
  test5: foo_default3
};

// /Users/user/project/baseurl_nested/nested/test0-success.ts
const test0_success_default2 = "test0-success";

// /Users/user/project/baseurl_nested/nested/test1-success.ts
const test1_success_default2 = "test1-success";

// /Users/user/project/baseurl_nested/nested/test2-success/foo.ts
const foo_default4 = "test2-success";

// /Users/user/project/baseurl_nested/nested/test3-success.ts
const test3_success_default2 = "test3-success";

// /Users/user/project/baseurl_nested/nested/test4-first/foo.ts
const foo_default5 = "test4-success";

// /Users/user/project/baseurl_nested/nested/test5-second/foo.ts
const foo_default6 = "test5-success";

// /Users/user/project/baseurl_nested/index.ts
const baseurl_nested_default = {
  test0: test0_success_default2,
  test1: test1_success_default2,
  test2: foo_default4,
  test3: test3_success_default2,
  test4: foo_default5,
  test5: foo_default6
};

// /Users/user/project/entry.ts
console.log(baseurl_dot_default, baseurl_nested_default);
`,
		},
	})
}

func TestPackageJsonBrowserString(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": "./browser"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/browser.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/node_modules/demo-pkg/browser.js
var require_browser = __commonJS((exports, module) => {
  module.exports = function() {
    return 123;
  };
});

// /Users/user/project/src/entry.js
const demo_pkg = __toModule(require_browser());
console.log(demo_pkg.default());
`,
		},
	})
}

func TestPackageJsonBrowserMapRelativeToRelative(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main",
					"browser": {
						"./main.js": "./main-browser",
						"./lib/util.js": "./lib/util-browser"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				const util = require('./lib/util')
				module.exports = function() {
					return ['main', util]
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main-browser.js": `
				const util = require('./lib/util')
				module.exports = function() {
					return ['main-browser', util]
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/lib/util.js": `
				module.exports = 'util'
			`,
			"/Users/user/project/node_modules/demo-pkg/lib/util-browser.js": `
				module.exports = 'util-browser'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/node_modules/demo-pkg/lib/util-browser.js
var require_util_browser = __commonJS((exports, module) => {
  module.exports = "util-browser";
});

// /Users/user/project/node_modules/demo-pkg/main-browser.js
var require_main_browser = __commonJS((exports, module) => {
  const util = require_util_browser();
  module.exports = function() {
    return ["main-browser", util];
  };
});

// /Users/user/project/src/entry.js
const demo_pkg = __toModule(require_main_browser());
console.log(demo_pkg.default());
`,
		},
	})
}

func TestPackageJsonBrowserMapRelativeToModule(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main",
					"browser": {
						"./util.js": "util-browser"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				const util = require('./util')
				module.exports = function() {
					return ['main', util]
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/util.js": `
				module.exports = 'util'
			`,
			"/Users/user/project/node_modules/util-browser/index.js": `
				module.exports = 'util-browser'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/node_modules/util-browser/index.js
var require_util_browser = __commonJS((exports, module) => {
  module.exports = "util-browser";
});

// /Users/user/project/node_modules/demo-pkg/main.js
var require_main = __commonJS((exports, module) => {
  const util = require_util_browser();
  module.exports = function() {
    return ["main", util];
  };
});

// /Users/user/project/src/entry.js
const demo_pkg = __toModule(require_main());
console.log(demo_pkg.default());
`,
		},
	})
}

func TestPackageJsonBrowserMapRelativeDisabled(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main",
					"browser": {
						"./util-node.js": false
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				const util = require('./util-node')
				module.exports = function(obj) {
					return util.inspect(obj)
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/util-node.js": `
				module.exports = require('util')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/node_modules/demo-pkg/util-node.js
var require_util_node = __commonJS(() => {
});

// /Users/user/project/node_modules/demo-pkg/main.js
var require_main = __commonJS((exports, module) => {
  const util = require_util_node();
  module.exports = function(obj) {
    return util.inspect(obj);
  };
});

// /Users/user/project/src/entry.js
const demo_pkg = __toModule(require_main());
console.log(demo_pkg.default());
`,
		},
	})
}

func TestPackageJsonBrowserMapModuleToRelative(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": {
						"node-pkg": "./node-pkg-browser"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/node-pkg-browser.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				const fn = require('node-pkg')
				module.exports = function() {
					return fn()
				}
			`,
			"/Users/user/project/node_modules/node-pkg/index.js": `
				module.exports = function() {
					return 234
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/node_modules/demo-pkg/node-pkg-browser.js
var require_node_pkg_browser = __commonJS((exports, module) => {
  module.exports = function() {
    return 123;
  };
});

// /Users/user/project/node_modules/demo-pkg/index.js
var require_demo_pkg = __commonJS((exports, module) => {
  const fn2 = require_node_pkg_browser();
  module.exports = function() {
    return fn2();
  };
});

// /Users/user/project/src/entry.js
const demo_pkg = __toModule(require_demo_pkg());
console.log(demo_pkg.default());
`,
		},
	})
}

func TestPackageJsonBrowserMapModuleToModule(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": {
						"node-pkg": "node-pkg-browser"
					}
				}
			`,
			"/Users/user/project/node_modules/node-pkg-browser/index.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				const fn = require('node-pkg')
				module.exports = function() {
					return fn()
				}
			`,
			"/Users/user/project/node_modules/node-pkg/index.js": `
				module.exports = function() {
					return 234
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/node_modules/node-pkg-browser/index.js
var require_node_pkg_browser = __commonJS((exports, module) => {
  module.exports = function() {
    return 123;
  };
});

// /Users/user/project/node_modules/demo-pkg/index.js
var require_demo_pkg = __commonJS((exports, module) => {
  const fn2 = require_node_pkg_browser();
  module.exports = function() {
    return fn2();
  };
});

// /Users/user/project/src/entry.js
const demo_pkg = __toModule(require_demo_pkg());
console.log(demo_pkg.default());
`,
		},
	})
}

func TestPackageJsonBrowserMapModuleDisabled(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": {
						"node-pkg": false
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				const fn = require('node-pkg')
				module.exports = function() {
					return fn()
				}
			`,
			"/Users/user/project/node_modules/node-pkg/index.js": `
				module.exports = function() {
					return 234
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/node_modules/node-pkg/index.js
var require_node_pkg = __commonJS(() => {
});

// /Users/user/project/node_modules/demo-pkg/index.js
var require_demo_pkg = __commonJS((exports, module) => {
  const fn2 = require_node_pkg();
  module.exports = function() {
    return fn2();
  };
});

// /Users/user/project/src/entry.js
const demo_pkg = __toModule(require_demo_pkg());
console.log(demo_pkg.default());
`,
		},
	})
}

func TestPackageJsonBrowserMapAvoidMissing(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'component-classes'
			`,
			"/Users/user/project/node_modules/component-classes/package.json": `
				{
					"browser": {
						"indexof": "component-indexof"
					}
				}
			`,
			"/Users/user/project/node_modules/component-classes/index.js": `
				try {
					var index = require('indexof');
				} catch (err) {
					var index = require('component-indexof');
				}
			`,
			"/Users/user/project/node_modules/component-indexof/index.js": `
				module.exports = function() {
					return 234
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/node_modules/component-indexof/index.js
var require_component_indexof = __commonJS((exports, module) => {
  module.exports = function() {
    return 234;
  };
});

// /Users/user/project/node_modules/component-classes/index.js
try {
  var index = require_component_indexof();
} catch (err) {
  var index = require_component_indexof();
}

// /Users/user/project/src/entry.js
`,
		},
	})
}

func TestPackageJsonBrowserOverModule(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main.js",
					"module": "./main.esm.js",
					"browser": "./main.browser.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.esm.js": `
				export default function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.browser.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/node_modules/demo-pkg/main.browser.js
var require_main_browser = __commonJS((exports, module) => {
  module.exports = function() {
    return 123;
  };
});

// /Users/user/project/src/entry.js
const demo_pkg = __toModule(require_main_browser());
console.log(demo_pkg.default());
`,
		},
	})
}

func TestPackageJsonBrowserWithModule(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main.js",
					"module": "./main.esm.js",
					"browser": {
						"./main.js": "./main.browser.js",
						"./main.esm.js": "./main.browser.esm.js"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.esm.js": `
				export default function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.browser.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.browser.esm.js": `
				export default function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/node_modules/demo-pkg/main.browser.esm.js
function main_browser_esm_default() {
  return 123;
}

// /Users/user/project/src/entry.js
console.log(main_browser_esm_default());
`,
		},
	})
}

func TestRequireChildDirCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				console.log(require('./dir'))
			`,
			"/Users/user/project/src/dir/index.js": `
				module.exports = 123
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
			"/out.js": `// /Users/user/project/src/dir/index.js
var require_dir = __commonJS((exports, module) => {
  module.exports = 123;
});

// /Users/user/project/src/entry.js
console.log(require_dir());
`,
		},
	})
}

func TestRequireChildDirES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import value from './dir'
				console.log(value)
			`,
			"/Users/user/project/src/dir/index.js": `
				export default 123
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
			"/out.js": `// /Users/user/project/src/dir/index.js
const dir_default = 123;

// /Users/user/project/src/entry.js
console.log(dir_default);
`,
		},
	})
}

func TestRequireParentDirCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/dir/entry.js": `
				console.log(require('..'))
			`,
			"/Users/user/project/src/index.js": `
				module.exports = 123
			`,
		},
		entryPaths: []string{"/Users/user/project/src/dir/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/src/index.js
var require_src = __commonJS((exports, module) => {
  module.exports = 123;
});

// /Users/user/project/src/dir/entry.js
console.log(require_src());
`,
		},
	})
}

func TestRequireParentDirES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/dir/entry.js": `
				import value from '..'
				console.log(value)
			`,
			"/Users/user/project/src/index.js": `
				export default 123
			`,
		},
		entryPaths: []string{"/Users/user/project/src/dir/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /Users/user/project/src/index.js
const src_default = 123;

// /Users/user/project/src/dir/entry.js
console.log(src_default);
`,
		},
	})
}

func TestImportMissingES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import fn, {x as a, y as b} from './foo'
				console.log(fn(a, b))
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
		expectedCompileLog: `/entry.js: error: No matching export for import "default"
/entry.js: error: No matching export for import "y"
`,
	})
}

func TestImportMissingUnusedES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import fn, {x as a, y as b} from './foo'
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
		expectedCompileLog: `/entry.js: error: No matching export for import "default"
/entry.js: error: No matching export for import "y"
`,
	})
}

func TestImportMissingCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import fn, {x as a, y as b} from './foo'
				console.log(fn(a, b))
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
const foo = __toModule(require_foo());
console.log(foo.default(foo.x, foo.y));
`,
		},
	})
}

func TestImportMissingNeitherES6NorCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import fn, {x as a, y as b} from './foo'
				import * as ns from './foo'
				console.log(fn(a, b))
			`,
			"/foo.js": `
				console.log('no exports here')
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
		expectedCompileLog: `/entry.js: warning: Import "default" will always be undefined
/entry.js: warning: Import "x" will always be undefined
/entry.js: warning: Import "y" will always be undefined
`,
		expected: map[string]string{
			"/out.js": `// /foo.js
var require_foo = __commonJS(() => {
  console.log("no exports here");
});

// /entry.js
const foo = __toModule(require_foo());
const ns = __toModule(require_foo());
console.log(foo.default(foo.x, foo.y));
`,
		},
	})
}

func TestExportMissingES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				console.log(ns)
			`,
			"/foo.js": `
				export {nope} from './bar'
			`,
			"/bar.js": `
				export const yep = 123
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
		expectedCompileLog: `/foo.js: error: No matching export for import "nope"
`,
	})
}

func TestDotImport(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {x} from '.'
				console.log(x)
			`,
			"/index.js": `
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
			"/out.js": `// /index.js
var require_index = __commonJS((exports) => {
  exports.x = 123;
});

// /entry.js
const _ = __toModule(require_index());
console.log(_.x);
`,
		},
	})
}

func TestRequireWithTemplate(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				console.log(require('./b'))
				console.log(require(` + "`./b`" + `))
			`,
			"/b.js": `
				exports.x = 123
			`,
		},
		entryPaths: []string{"/a.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /b.js
var require_b = __commonJS((exports) => {
  exports.x = 123;
});

// /a.js
console.log(require_b());
console.log(require_b());
`,
		},
	})
}

func TestDynamicImportWithTemplateIIFE(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import('./b').then(ns => console.log(ns))
				import(` + "`./b`" + `).then(ns => console.log(ns))
			`,
			"/b.js": `
				exports.x = 123
			`,
		},
		entryPaths: []string{"/a.js"},
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
  // /b.js
  var require_b = __commonJS((exports) => {
    exports.x = 123;
  });

  // /a.js
  Promise.resolve().then(() => __toModule(require_b())).then((ns) => console.log(ns));
  Promise.resolve().then(() => __toModule(require_b())).then((ns) => console.log(ns));
})();
`,
		},
	})
}

func TestRequireAndDynamicImportInvalidTemplate(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				require(tag` + "`./b`" + `)
				require(` + "`./${b}`" + `)
				import(tag` + "`./b`" + `)
				import(` + "`./${b}`" + `)
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
		expectedScanLog: `/entry.js: warning: This call to "require" will not be bundled because the argument is not a string literal
/entry.js: warning: This call to "require" will not be bundled because the argument is not a string literal
/entry.js: warning: This dynamic import will not be bundled because the argument is not a string literal
/entry.js: warning: This dynamic import will not be bundled because the argument is not a string literal
`,
		expected: map[string]string{
			"/out.js": `// /entry.js
require(tag` + "`./b`" + `);
require(` + "`./${b}`" + `);
import(tag` + "`./b`" + `);
import(` + "`./${b}`" + `);
`,
		},
	})
}

func TestRequireBadArgumentCount(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				require()
				require("a", "b")
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
		expectedScanLog: `/entry.js: warning: This call to "require" will not be bundled because it has 0 arguments
/entry.js: warning: This call to "require" will not be bundled because it has 2 arguments
`,
		expected: map[string]string{
			"/out.js": `// /entry.js
require();
require("a", "b");
`,
		},
	})
}

func TestRequireJson(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.json'))
			`,
			"/test.json": `
				{
					"a": true,
					"b": 123,
					"c": [null]
				}
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
			"/out.js": `// /test.json
var require_test = __commonJS((exports, module) => {
  module.exports = {
    a: true,
    b: 123,
    c: [null]
  };
});

// /entry.js
console.log(require_test());
`,
		},
	})
}

func TestRequireTxt(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.txt'))
			`,
			"/test.txt": `This is a test.`,
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
			"/out.js": `// /test.txt
var require_test = __commonJS((exports, module) => {
  module.exports = "This is a test.";
});

// /entry.js
console.log(require_test());
`,
		},
	})
}

func TestRequireCustomExtensionString(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.custom'))
			`,
			"/test.custom": `#include <stdio.h>`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]Loader{
				".js":     LoaderJS,
				".custom": LoaderText,
			},
		},
		expected: map[string]string{
			"/out.js": `// /test.custom
var require_test = __commonJS((exports, module) => {
  module.exports = "#include <stdio.h>";
});

// /entry.js
console.log(require_test());
`,
		},
	})
}

func TestRequireCustomExtensionBase64(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.custom'))
			`,
			"/test.custom": "a\x00b\x80c\xFFd",
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]Loader{
				".js":     LoaderJS,
				".custom": LoaderBase64,
			},
		},
		expected: map[string]string{
			"/out.js": `// /test.custom
var require_test = __commonJS((exports, module) => {
  module.exports = "YQBigGP/ZA==";
});

// /entry.js
console.log(require_test());
`,
		},
	})
}

func TestRequireCustomExtensionDataURL(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.custom'))
			`,
			"/test.custom": "a\x00b\x80c\xFFd",
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]Loader{
				".js":     LoaderJS,
				".custom": LoaderDataURL,
			},
		},
		expected: map[string]string{
			"/out.js": `// /test.custom
var require_test = __commonJS((exports, module) => {
  module.exports = "data:application/octet-stream;base64,YQBigGP/ZA==";
});

// /entry.js
console.log(require_test());
`,
		},
	})
}

func testAutoDetectMimeTypeFromExtension(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.svg'))
			`,
			"/test.svg": "a\x00b\x80c\xFFd",
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]Loader{
				".js":  LoaderJS,
				".svg": LoaderDataURL,
			},
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(exports, module) {
    // /test.svg
    module.exports = "data:image/svg+xml;base64,YQBigGP/ZA==";
  },

  0() {
    // /entry.js
    console.log(__require(1 /* ./test.svg */));
  }
}, 0);
`,
		},
	})
}

func TestLoaderFile(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.svg'))
			`,

			// Use an SVG string that has a base64-encoded SHA1 has with a "/" in it
			"/test.svg": "<svg>$</svg>",
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:   true,
			AbsOutputDir: "/out/",
			ExtensionToLoader: map[string]Loader{
				".js":  LoaderJS,
				".svg": LoaderFile,
			},
		},
		expected: map[string]string{
			"/out/test.1HOBn_hi.svg": "<svg>$</svg>",
			"/out/entry.js": `// /test.svg
var require_test = __commonJS((exports, module) => {
  module.exports = "test.1HOBn_hi.svg";
});

// /entry.js
console.log(require_test());
`,
		},
	})
}

func TestRequireBadExtension(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test'))
			`,
			"/test": `This is a test.`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: error: File extension not supported: /test
`,
	})
}

func TestFalseRequire(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				(require => require('/test.txt'))()
			`,
			"/test.txt": `This is a test.`,
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
((require2) => require2("/test.txt"))();
`,
		},
	})
}

func TestRequireWithoutCall(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const req = require
				req('./entry')
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
		expectedScanLog: `/entry.js: warning: Indirect calls to "require" will not be bundled
`,
		expected: map[string]string{
			"/out.js": `// /entry.js
const req = require;
req("./entry");
`,
		},
	})
}

func TestNestedRequireWithoutCall(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				(() => {
					const req = require
					req('./entry')
				})()
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
		expectedScanLog: `/entry.js: warning: Indirect calls to "require" will not be bundled
`,
		expected: map[string]string{
			"/out.js": `// /entry.js
(() => {
  const req = require;
  req("./entry");
})();
`,
		},
	})
}

// Test a workaround for the "moment" library
func TestRequireWithoutCallInsideTry(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				try {
					oldLocale = globalLocale._abbr;
					var aliasedRequire = require;
					aliasedRequire('./locale/' + name);
					getSetGlobalLocale(oldLocale);
				} catch (e) {}
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
try {
  oldLocale = globalLocale._abbr;
  var aliasedRequire = require;
  aliasedRequire("./locale/" + name);
  getSetGlobalLocale(oldLocale);
} catch (e) {
}
`,
		},
	})
}

func TestSourceMap(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {bar} from './bar'
				function foo() { bar() }
				foo()
			`,
			"/Users/user/project/src/bar.js": `
				export function bar() { throw new Error('test') }
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			SourceMap:     SourceMapLinkedWithComment,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/src/bar.js
function bar() {
  throw new Error("test");
}

// /Users/user/project/src/entry.js
function foo() {
  bar();
}
foo();
//# sourceMappingURL=out.js.map
`,
		},
	})
}

// This test covers a bug where a "var" in a nested scope did not correctly
// bind with references to that symbol in sibling scopes. Instead, the
// references were incorrectly considered to be unbound even though the symbol
// should be hoisted. This caused the renamer to name them different things to
// avoid a collision, which changed the meaning of the code.
func TestNestedScopeBug(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				(() => {
					function a() {
						b()
					}
					{
						var b = () => {}
					}
					a()
				})()
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
(() => {
  function a() {
    b();
  }
  {
    var b = () => {
    };
  }
  a();
})();
`,
		},
	})
}

func TestHashbangBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `#!/usr/bin/env a
				import {code} from './code'
				process.exit(code)
			`,
			"/code.js": `#!/usr/bin/env b
				export const code = 0
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
			"/out.js": `#!/usr/bin/env a

// /code.js
const code = 0;

// /entry.js
process.exit(code);
`,
		},
	})
}

func TestHashbangNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `#!/usr/bin/env node
				process.exit(0);
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
			"/out.js": `#!/usr/bin/env node
process.exit(0);
`,
		},
	})
}

func TestTypeofRequireBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log([
					typeof require,
					typeof require == 'function',
					typeof require == 'function' && require,
					'function' == typeof require,
					'function' == typeof require && require,
				]);
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
console.log([
  "function",
  true,
  false,
  true,
  false
]);
`,
		},
	})
}

func TestTypeofRequireNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log([
					typeof require,
					typeof require == 'function',
					typeof require == 'function' && require,
					'function' == typeof require,
					'function' == typeof require && require,
				]);
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
			"/out.js": `console.log([
  typeof require,
  typeof require == "function",
  typeof require == "function" && require,
  "function" == typeof require,
  "function" == typeof require && require
]);
`,
		},
	})
}

func TestTypeofRequireBadPatterns(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log([
					typeof require != 'function' && require,
					typeof require === 'function' && require,
					typeof require == 'function' || require,
					typeof require == 'function' && notRequire,
					typeof notRequire == 'function' && require,

					'function' != typeof require && require,
					'function' === typeof require && require,
					'function' == typeof require || require,
					'function' == typeof require && notRequire,
					'function' == typeof notRequire && require,
				]);
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
		expectedScanLog: `/entry.js: warning: Indirect calls to "require" will not be bundled
/entry.js: warning: Indirect calls to "require" will not be bundled
/entry.js: warning: Indirect calls to "require" will not be bundled
/entry.js: warning: Indirect calls to "require" will not be bundled
/entry.js: warning: Indirect calls to "require" will not be bundled
/entry.js: warning: Indirect calls to "require" will not be bundled
/entry.js: warning: Indirect calls to "require" will not be bundled
/entry.js: warning: Indirect calls to "require" will not be bundled
`,
		expected: map[string]string{
			"/out.js": `// /entry.js
console.log([
  false,
  require,
  true,
  notRequire,
  typeof notRequire == "function" && require,
  false,
  require,
  true,
  notRequire,
  "function" == typeof notRequire && require
]);
`,
		},
	})
}

func TestRequireFSBrowser(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('fs'))
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
		resolveOptions: resolver.ResolveOptions{
			Platform: parser.PlatformBrowser,
		},
		expectedScanLog: "/entry.js: error: Could not resolve \"fs\"\n",
	})
}

func TestRequireFSNode(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				return require('fs')
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
		resolveOptions: resolver.ResolveOptions{
			Platform: parser.PlatformNode,
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
return require("fs");
`,
		},
	})
}

func TestRequireFSNodeMinify(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				return require('fs')
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:       true,
			RemoveWhitespace: true,
			OutputFormat:     printer.FormatCommonJS,
			AbsOutputFile:    "/out.js",
		},
		resolveOptions: resolver.ResolveOptions{
			Platform: parser.PlatformNode,
		},
		expected: map[string]string{
			"/out.js": `return require("fs");
`,
		},
	})
}

func TestImportFSBrowser(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import 'fs'
				import * as fs from 'fs'
				import defaultValue from 'fs'
				import {readFileSync} from 'fs'
				console.log(fs, readFileSync, defaultValue)
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
		resolveOptions: resolver.ResolveOptions{
			Platform: parser.PlatformBrowser,
		},
		expectedScanLog: `/entry.js: error: Could not resolve "fs"
/entry.js: error: Could not resolve "fs"
/entry.js: error: Could not resolve "fs"
/entry.js: error: Could not resolve "fs"
`,
	})
}

func TestImportFSNodeCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import 'fs'
				import * as fs from 'fs'
				import defaultValue from 'fs'
				import {readFileSync} from 'fs'
				console.log(fs, readFileSync, defaultValue)
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
		resolveOptions: resolver.ResolveOptions{
			Platform: parser.PlatformNode,
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
const fs = __toModule(require("fs"));
const fs2 = __toModule(require("fs"));
const fs3 = __toModule(require("fs"));
const fs4 = __toModule(require("fs"));
console.log(fs2, fs4.readFileSync, fs3.default);
`,
		},
	})
}

func TestImportFSNodeES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import 'fs'
				import * as fs from 'fs'
				import defaultValue from 'fs'
				import {readFileSync} from 'fs'
				console.log(fs, readFileSync, defaultValue)
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
		resolveOptions: resolver.ResolveOptions{
			Platform: parser.PlatformNode,
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
import "fs";
import * as fs2 from "fs";
import defaultValue from "fs";
import {readFileSync} from "fs";
console.log(fs2, readFileSync, defaultValue);
`,
		},
	})
}

func TestExportFSBrowser(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export * as fs from 'fs'
				export {readFileSync} from 'fs'
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
		resolveOptions: resolver.ResolveOptions{
			Platform: parser.PlatformBrowser,
		},
		expectedScanLog: `/entry.js: error: Could not resolve "fs"
/entry.js: error: Could not resolve "fs"
`,
	})
}

func TestExportFSNode(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export * as fs from 'fs'
				export {readFileSync} from 'fs'
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
		resolveOptions: resolver.ResolveOptions{
			Platform: parser.PlatformNode,
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
import * as fs from "fs";
import {readFileSync} from "fs";
export {fs, readFileSync};
`,
		},
	})
}

func TestReExportFSNode(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export {fs as f} from './foo'
				export {readFileSync as rfs} from './foo'
			`,
			"/foo.js": `
				export * as fs from 'fs'
				export {readFileSync} from 'fs'
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
		resolveOptions: resolver.ResolveOptions{
			Platform: parser.PlatformNode,
		},
		expected: map[string]string{
			"/out.js": `// /foo.js
import * as fs from "fs";
import {readFileSync} from "fs";

// /entry.js
export {fs as f, readFileSync as rfs};
`,
		},
	})
}

func TestExportFSNodeInCommonJSModule(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export * as fs from 'fs'
				export {readFileSync} from 'fs'

				// Force this to be a CommonJS module
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
		resolveOptions: resolver.ResolveOptions{
			Platform: parser.PlatformNode,
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
import * as fs from "fs";
import {readFileSync} from "fs";
var require_entry = __commonJS((exports) => {
  __export(exports, {
    fs: () => fs,
    readFileSync: () => readFileSync
  });
  exports.foo = 123;
});
export default require_entry();
`,
		},
	})
}

func TestExportWildcardFSNodeES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export * from 'fs'
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
		resolveOptions: resolver.ResolveOptions{
			Platform: parser.PlatformNode,
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
import * as fs_star from "fs";
var require_entry = __commonJS((exports) => {
  __exportStar(exports, fs_star);
});
export default require_entry();
`,
		},
	})
}

func TestExportWildcardFSNodeCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export * from 'fs'
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
		resolveOptions: resolver.ResolveOptions{
			Platform: parser.PlatformNode,
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
__exportStar(exports, __toModule(require("fs")));
`,
		},
	})
}

func TestMinifiedBundleES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {foo} from './a'
				console.log(foo())
			`,
			"/a.js": `
				export function foo() {
					return 123
				}
				foo()
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling:   true,
			MangleSyntax: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:        true,
			RemoveWhitespace:  true,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
		expected: map[string]string{
			"/out.js": `function a(){return 123}a();console.log(a());
`,
		},
	})
}

func TestMinifiedBundleCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const {foo} = require('./a')
				console.log(foo(), require('./j.json'))
			`,
			"/a.js": `
				exports.foo = function() {
					return 123
				}
			`,
			"/j.json": `
				{"test": true}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling:   true,
			MangleSyntax: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:        true,
			RemoveWhitespace:  true,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
		expected: map[string]string{
			"/out.js": `var d=c(b=>{b.foo=function(){return 123}});var f=c((b,a)=>{a.exports={test:!0}});const{foo:e}=d();console.log(e(),f());
`,
		},
	})
}

func TestMinifiedBundleEndingWithImportantSemicolon(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				while(foo()); // This semicolon must not be stripped
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:       true,
			RemoveWhitespace: true,
			OutputFormat:     printer.FormatIIFE,
			AbsOutputFile:    "/out.js",
		},
		expected: map[string]string{
			"/out.js": `(()=>{while(foo());})();
`,
		},
	})
}

func TestOptionalCatchNameCollisionNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				try {}
				catch { var e, e2 }
				var e3
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     parser.ES2018,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `try {
} catch (e4) {
  var e, e2;
}
var e3;
`,
		},
	})
}

func TestRuntimeNameCollisionNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function __require() { return 123 }
				console.log(__require())
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     parser.ES2018,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `function __require() {
  return 123;
}
console.log(__require());
`,
		},
	})
}

func TestTopLevelReturn(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {foo} from './foo'
				foo()
			`,
			"/foo.js": `
				// Top-level return must force CommonJS mode
				if (Math.random() < 0.5) return

				export function foo() {}
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
    foo: () => foo3
  });
  if (Math.random() < 0.5)
    return;
  function foo3() {
  }
});

// /entry.js
const foo = __toModule(require_foo());
foo.foo();
`,
		},
	})
}

func TestThisOutsideFunction(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(this)
				console.log((x = this) => this)
				console.log({x: this})
				console.log(class extends this.foo {})
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
var require_entry = __commonJS((exports) => {
  console.log(exports);
  console.log((x = exports) => exports);
  console.log({x: exports});
  console.log(class extends exports.foo {
  });
});
export default require_entry();
`,
		},
	})
}

func TestThisInsideFunction(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function foo(x = this) { console.log(this) }
				const obj = {
					foo(x = this) { console.log(this) }
				}
				class Foo {
					x = this
					static y = this.z
					foo(x = this) { console.log(this) }
					static bar(x = this) { console.log(this) }
				}
				new Foo(foo(obj))
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
function foo(x = this) {
  console.log(this);
}
const obj = {
  foo(x = this) {
    console.log(this);
  }
};
class Foo {
  x = this;
  static y = this.z;
  foo(x = this) {
    console.log(this);
  }
  static bar(x = this) {
    console.log(this);
  }
}
new Foo(foo(obj));
`,
		},
	})
}

// The value of "this" is "exports" in CommonJS modules and undefined in ES6
// modules. This is determined by the presence of ES6 import/export syntax.
func TestThisWithES6Syntax(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import './cjs'

				import './es6-import-stmt'
				import './es6-import-assign'
				import './es6-import-dynamic'
				import './es6-import-meta'
				import './es6-expr-import-dynamic'
				import './es6-expr-import-meta'

				import './es6-export-variable'
				import './es6-export-function'
				import './es6-export-async-function'
				import './es6-export-enum'
				import './es6-export-const-enum'
				import './es6-export-module'
				import './es6-export-namespace'
				import './es6-export-class'
				import './es6-export-abstract-class'
				import './es6-export-default'
				import './es6-export-clause'
				import './es6-export-clause-from'
				import './es6-export-star'
				import './es6-export-star-as'
				import './es6-export-assign'
				import './es6-export-import-assign'

				import './es6-ns-export-variable'
				import './es6-ns-export-function'
				import './es6-ns-export-async-function'
				import './es6-ns-export-enum'
				import './es6-ns-export-const-enum'
				import './es6-ns-export-module'
				import './es6-ns-export-namespace'
				import './es6-ns-export-class'
				import './es6-ns-export-abstract-class'
				`,
			"/dummy.js": `export const dummy = 123`,
			"/cjs.js":   `console.log(this)`,

			"/es6-import-stmt.js":         `import './dummy'; console.log(this)`,
			"/es6-import-assign.ts":       `import x = require('./dummy'); console.log(this)`,
			"/es6-import-dynamic.js":      `import('./dummy'); console.log(this)`,
			"/es6-import-meta.js":         `import.meta; console.log(this)`,
			"/es6-expr-import-dynamic.js": `(import('./dummy')); console.log(this)`,
			"/es6-expr-import-meta.js":    `(import.meta); console.log(this)`,

			"/es6-export-variable.js":       `export const foo = 123; console.log(this)`,
			"/es6-export-function.js":       `export function foo() {} console.log(this)`,
			"/es6-export-async-function.js": `export async function foo() {} console.log(this)`,
			"/es6-export-enum.ts":           `export enum Foo {} console.log(this)`,
			"/es6-export-const-enum.ts":     `export const enum Foo {} console.log(this)`,
			"/es6-export-module.ts":         `export module Foo {} console.log(this)`,
			"/es6-export-namespace.ts":      `export namespace Foo {} console.log(this)`,
			"/es6-export-class.js":          `export class Foo {} console.log(this)`,
			"/es6-export-abstract-class.ts": `export abstract class Foo {} console.log(this)`,
			"/es6-export-default.js":        `export default 123; console.log(this)`,
			"/es6-export-clause.js":         `export {}; console.log(this)`,
			"/es6-export-clause-from.js":    `export {} from './dummy'; console.log(this)`,
			"/es6-export-star.js":           `export * from './dummy'; console.log(this)`,
			"/es6-export-star-as.js":        `export * as ns from './dummy'; console.log(this)`,
			"/es6-export-assign.ts":         `export = 123; console.log(this)`,
			"/es6-export-import-assign.ts":  `export import x = require('./dummy'); console.log(this)`,

			"/es6-ns-export-variable.ts":       `namespace ns { export const foo = 123; } console.log(this)`,
			"/es6-ns-export-function.ts":       `namespace ns { export function foo() {} } console.log(this)`,
			"/es6-ns-export-async-function.ts": `namespace ns { export async function foo() {} } console.log(this)`,
			"/es6-ns-export-enum.ts":           `namespace ns { export enum Foo {} } console.log(this)`,
			"/es6-ns-export-const-enum.ts":     `namespace ns { export const enum Foo {} } console.log(this)`,
			"/es6-ns-export-module.ts":         `namespace ns { export module Foo {} } console.log(this)`,
			"/es6-ns-export-namespace.ts":      `namespace ns { export namespace Foo {} } console.log(this)`,
			"/es6-ns-export-class.ts":          `namespace ns { export class Foo {} } console.log(this)`,
			"/es6-ns-export-abstract-class.ts": `namespace ns { export abstract class Foo {} } console.log(this)`,
		},
		entryPaths: []string{
			"/entry.js",
		},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /cjs.js
var require_cjs = __commonJS((exports) => {
  console.log(exports);
});

// /dummy.js
var require_dummy = __commonJS((exports) => {
  __export(exports, {
    dummy: () => dummy3
  });
  const dummy3 = 123;
});

// /es6-export-star.js
var require_es6_export_star = __commonJS((exports) => {
  __exportStar(exports, __toModule(require_dummy()));
  console.log(void 0);
});

// /es6-export-assign.ts
var require_es6_export_assign = __commonJS((exports, module) => {
  console.log(void 0);
  module.exports = 123;
});

// /es6-ns-export-variable.ts
var require_es6_ns_export_variable = __commonJS((exports) => {
  var ns2;
  (function(ns3) {
    ns3.foo = 123;
  })(ns2 || (ns2 = {}));
  console.log(exports);
});

// /es6-ns-export-function.ts
var require_es6_ns_export_function = __commonJS((exports) => {
  var ns2;
  (function(ns3) {
    function foo4() {
    }
    ns3.foo = foo4;
  })(ns2 || (ns2 = {}));
  console.log(exports);
});

// /es6-ns-export-async-function.ts
var require_es6_ns_export_async_function = __commonJS((exports) => {
  var ns2;
  (function(ns3) {
    async function foo4() {
    }
    ns3.foo = foo4;
  })(ns2 || (ns2 = {}));
  console.log(exports);
});

// /es6-ns-export-enum.ts
var require_es6_ns_export_enum = __commonJS((exports) => {
  var ns2;
  (function(ns3) {
    let Foo5;
    (function(Foo6) {
    })(Foo5 = ns3.Foo || (ns3.Foo = {}));
  })(ns2 || (ns2 = {}));
  console.log(exports);
});

// /es6-ns-export-const-enum.ts
var require_es6_ns_export_const_enum = __commonJS((exports) => {
  var ns2;
  (function(ns3) {
    let Foo5;
    (function(Foo6) {
    })(Foo5 = ns3.Foo || (ns3.Foo = {}));
  })(ns2 || (ns2 = {}));
  console.log(exports);
});

// /es6-ns-export-module.ts
var require_es6_ns_export_module = __commonJS((exports) => {
  console.log(exports);
});

// /es6-ns-export-namespace.ts
var require_es6_ns_export_namespace = __commonJS((exports) => {
  console.log(exports);
});

// /es6-ns-export-class.ts
var require_es6_ns_export_class = __commonJS((exports) => {
  var ns2;
  (function(ns3) {
    class Foo5 {
    }
    ns3.Foo = Foo5;
  })(ns2 || (ns2 = {}));
  console.log(exports);
});

// /es6-ns-export-abstract-class.ts
var require_es6_ns_export_abstract_class = __commonJS((exports) => {
  var ns2;
  (function(ns3) {
    class Foo5 {
    }
    ns3.Foo = Foo5;
  })(ns2 || (ns2 = {}));
  console.log(exports);
});

// /es6-import-stmt.js
const dummy2 = __toModule(require_dummy());
console.log(void 0);

// /es6-import-assign.ts
const x2 = require_dummy();
console.log(void 0);

// /es6-import-dynamic.js
Promise.resolve().then(() => __toModule(require_dummy()));
console.log(void 0);

// /es6-import-meta.js
console.log(void 0);

// /es6-expr-import-dynamic.js
Promise.resolve().then(() => __toModule(require_dummy()));
console.log(void 0);

// /es6-expr-import-meta.js
console.log(void 0);

// /es6-export-variable.js
console.log(void 0);

// /es6-export-function.js
console.log(void 0);

// /es6-export-async-function.js
console.log(void 0);

// /es6-export-enum.ts
var Foo4;
(function(Foo5) {
})(Foo4 || (Foo4 = {}));
console.log(void 0);

// /es6-export-const-enum.ts
var Foo3;
(function(Foo5) {
})(Foo3 || (Foo3 = {}));
console.log(void 0);

// /es6-export-module.ts
console.log(void 0);

// /es6-export-namespace.ts
console.log(void 0);

// /es6-export-class.js
console.log(void 0);

// /es6-export-abstract-class.ts
console.log(void 0);

// /es6-export-default.js
console.log(void 0);

// /es6-export-clause.js
console.log(void 0);

// /es6-export-clause-from.js
const dummy = __toModule(require_dummy());
console.log(void 0);

// /es6-export-star-as.js
const ns = __toModule(require_dummy());
console.log(void 0);

// /es6-export-import-assign.ts
const x = require_dummy();
console.log(void 0);

// /entry.js
const cjs = __toModule(require_cjs());
const es6_export_star = __toModule(require_es6_export_star());
const es6_export_assign = __toModule(require_es6_export_assign());
const es6_ns_export_variable = __toModule(require_es6_ns_export_variable());
const es6_ns_export_function = __toModule(require_es6_ns_export_function());
const es6_ns_export_async_function = __toModule(require_es6_ns_export_async_function());
const es6_ns_export_enum = __toModule(require_es6_ns_export_enum());
const es6_ns_export_const_enum = __toModule(require_es6_ns_export_const_enum());
const es6_ns_export_module = __toModule(require_es6_ns_export_module());
const es6_ns_export_namespace = __toModule(require_es6_ns_export_namespace());
const es6_ns_export_class = __toModule(require_es6_ns_export_class());
const es6_ns_export_abstract_class = __toModule(require_es6_ns_export_abstract_class());
`,
		},
	})
}

func TestArrowFnScope(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				tests = {
					0: ((x = y => x + y, y) => x + y),
					1: ((y, x = y => x + y) => x + y),
					2: ((x = (y = z => x + y + z, z) => x + y + z, y, z) => x + y + z),
					3: ((y, z, x = (z, y = z => x + y + z) => x + y + z) => x + y + z),
					4: ((x = y => x + y, y), x + y),
					5: ((y, x = y => x + y), x + y),
					6: ((x = (y = z => x + y + z, z) => x + y + z, y, z), x + y + z),
					7: ((y, z, x = (z, y = z => x + y + z) => x + y + z), x + y + z),
				};
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:        true,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
tests = {
  0: (a = (c) => a + c, b) => a + b,
  1: (a, b = (c) => b + c) => b + a,
  2: (a = (d = (f) => a + d + f, e) => a + d + e, b, c) => a + b + c,
  3: (a, b, c = (d, e = (f) => c + e + f) => c + e + d) => c + a + b,
  4: (x = (a) => x + a, y, x + y),
  5: (y, x = (a) => x + a, x + y),
  6: (x = (a = (c) => x + a + c, b) => x + a + b, y, z, x + y + z),
  7: (y, z, x = (a, b = (c) => x + b + c) => x + b + a, x + y + z)
};
`,
		},
	})
}

func TestLowerObjectSpreadNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.jsx": `
				let tests = [
					{...a, ...b},
					{a, b, ...c},
					{...a, b, c},
					{a, ...b, c},
					{a, b, ...c, ...d, e, f, ...g, ...h, i, j},
				]
				let jsx = [
					<div {...a} {...b}/>,
					<div a b {...c}/>,
					<div {...a} b c/>,
					<div a {...b} c/>,
					<div a b {...c} {...d} e f {...g} {...h} i j/>,
				]
			`,
		},
		entryPaths: []string{"/entry.jsx"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     parser.ES2017,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `let tests = [
  __assign(__assign({}, a), b),
  __assign({a, b}, c),
  __assign(__assign({}, a), {b, c}),
  __assign(__assign({a}, b), {c}),
  __assign(__assign(__assign(__assign(__assign(__assign({a, b}, c), d), {e, f}), g), h), {i, j})
];
let jsx = [
  React.createElement("div", __assign(__assign({}, a), b)),
  React.createElement("div", __assign({
    a: true,
    b: true
  }, c)),
  React.createElement("div", __assign(__assign({}, a), {
    b: true,
    c: true
  })),
  React.createElement("div", __assign(__assign({
    a: true
  }, b), {
    c: true
  })),
  React.createElement("div", __assign(__assign(__assign(__assign(__assign(__assign({
    a: true,
    b: true
  }, c), d), {
    e: true,
    f: true
  }), g), h), {
    i: true,
    j: true
  }))
];
`,
		},
	})
}

func TestLowerExponentiationOperatorNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				let tests = {
					// Exponentiation operator
					0: a ** b ** c,
					1: (a ** b) ** c,

					// Exponentiation assignment operator
					2: a **= b,
					3: a.b **= c,
					4: a[b] **= c,
					5: a().b **= c,
					6: a()[b] **= c,
					7: a[b()] **= c,
					8: a()[b()] **= c,

					// These all should not need capturing (no object identity)
					9: a[0] **= b,
					10: a[false] **= b,
					11: a[null] **= b,
					12: a[void 0] **= b,
					13: a[123n] **= b,
					14: a[this] **= b,

					// These should need capturing (have object identitiy)
					15: a[/x/] **= b,
					16: a[{}] **= b,
					17: a[[]] **= b,
					18: a[() => {}] **= b,
					19: a[function() {}] **= b,
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     parser.ES2015,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: "/entry.js: error: Big integer literals are from ES2020 and transforming them to ES2015 is not supported\n",
		expected: map[string]string{
			"/out.js": `var _a, _b, _c, _d, _e, _f, _g, _h, _i, _j;
let tests = {
  0: __pow(a, __pow(b, c)),
  1: __pow(__pow(a, b), c),
  2: a = __pow(a, b),
  3: a.b = __pow(a.b, c),
  4: a[b] = __pow(a[b], c),
  5: (_a = a()).b = __pow(_a.b, c),
  6: (_b = a())[b] = __pow(_b[b], c),
  7: a[_c = b()] = __pow(a[_c], c),
  8: (_d = a())[_e = b()] = __pow(_d[_e], c),
  9: a[0] = __pow(a[0], b),
  10: a[false] = __pow(a[false], b),
  11: a[null] = __pow(a[null], b),
  12: a[void 0] = __pow(a[void 0], b),
  13: a[123n] = __pow(a[123n], b),
  14: a[this] = __pow(a[this], b),
  15: a[_f = /x/] = __pow(a[_f], b),
  16: a[_g = {}] = __pow(a[_g], b),
  17: a[_h = []] = __pow(a[_h], b),
  18: a[_i = () => {
  }] = __pow(a[_i], b),
  19: a[_j = function() {
  }] = __pow(a[_j], b)
};
`,
		},
	})
}

func TestSwitchScopeNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				switch (foo) { default: var foo }
				switch (bar) { default: let bar }
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
		},
		bundleOptions: BundleOptions{
			IsBundling:        false,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
		expected: map[string]string{
			"/out.js": `switch (foo) {
  default:
    var foo;
}
switch (bar) {
  default:
    let a;
}
`,
		},
	})
}

func TestArgumentDefaultValueScopeNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export function a(x = foo) { var foo; return x }
				export class b { fn(x = foo) { var foo; return x } }
				export let c = [
					function(x = foo) { var foo; return x },
					(x = foo) => { var foo; return x },
					{ fn(x = foo) { var foo; return x }},
					class { fn(x = foo) { var foo; return x }},
				]
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
		},
		bundleOptions: BundleOptions{
			IsBundling:        false,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
		expected: map[string]string{
			"/out.js": `export function a(d = foo) {
  var e;
  return d;
}
export class b {
  fn(d = foo) {
    var e;
    return d;
  }
}
export let c = [
  function(d = foo) {
    var e;
    return d;
  },
  (d = foo) => {
    var e;
    return d;
  },
  {fn(d = foo) {
    var e;
    return d;
  }},
  class {
    fn(d = foo) {
      var e;
      return d;
    }
  }
];
`,
		},
	})
}

func TestArgumentsSpecialCaseNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				(() => {
					var arguments;

					function foo(x = arguments) { return arguments }
					(function(x = arguments) { return arguments });
					({foo(x = arguments) { return arguments }});
					class Foo { foo(x = arguments) { return arguments } }
					(class { foo(x = arguments) { return arguments } });

					function foo(x = arguments) { var arguments; return arguments }
					(function(x = arguments) { var arguments; return arguments });
					({foo(x = arguments) { var arguments; return arguments }});
					class Foo2 { foo(x = arguments) { var arguments; return arguments } }
					(class { foo(x = arguments) { var arguments; return arguments } });

					(x => arguments);
					(() => arguments);
					(async () => arguments);
					((x = arguments) => arguments);
					(async (x = arguments) => arguments);

					x => arguments;
					() => arguments;
					async () => arguments;
					(x = arguments) => arguments;
					async (x = arguments) => arguments;

					(x => { return arguments });
					(() => { return arguments });
					(async () => { return arguments });
					((x = arguments) => { return arguments });
					(async (x = arguments) => { return arguments });

					x => { return arguments };
					() => { return arguments };
					async () => { return arguments };
					(x = arguments) => { return arguments };
					async (x = arguments) => { return arguments };
				})()
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
		},
		bundleOptions: BundleOptions{
			IsBundling:        false,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
		expected: map[string]string{
			"/out.js": `(() => {
  var a;
  function c(b = arguments) {
    return arguments;
  }
  (function(b = arguments) {
    return arguments;
  });
  ({foo(b = arguments) {
    return arguments;
  }});
  class d {
    foo(b = arguments) {
      return arguments;
    }
  }
  (class {
    foo(b = arguments) {
      return arguments;
    }
  });
  function c(b = arguments) {
    var arguments;
    return arguments;
  }
  (function(b = arguments) {
    var arguments;
    return arguments;
  });
  ({foo(b = arguments) {
    var arguments;
    return arguments;
  }});
  class e {
    foo(b = arguments) {
      var arguments;
      return arguments;
    }
  }
  (class {
    foo(b = arguments) {
      var arguments;
      return arguments;
    }
  });
  (b) => a;
  () => a;
  async () => a;
  (b = a) => a;
  async (b = a) => a;
  (b) => a;
  () => a;
  async () => a;
  (b = a) => a;
  async (b = a) => a;
  (b) => {
    return a;
  };
  () => {
    return a;
  };
  async () => {
    return a;
  };
  (b = a) => {
    return a;
  };
  async (b = a) => {
    return a;
  };
  (b) => {
    return a;
  };
  () => {
    return a;
  };
  async () => {
    return a;
  };
  (b = a) => {
    return a;
  };
  async (b = a) => {
    return a;
  };
})();
`,
		},
	})
}

func TestWithStatementTaintingNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				(() => {
					let local = 1
					let outer = 2
					let outerDead = 3
					with ({}) {
						var hoisted = 4
						let local = 5
						hoisted++
						local++
						if (1) outer++
						if (0) outerDead++
					}
					if (1) {
						hoisted++
						local++
						outer++
						outerDead++
					}
				})()
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
		},
		bundleOptions: BundleOptions{
			IsBundling:        false,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
		expected: map[string]string{
			"/out.js": `(() => {
  let a = 1;
  let outer = 2;
  let outerDead = 3;
  with ({}) {
    var hoisted = 4;
    let b = 5;
    hoisted++;
    b++;
    if (1)
      outer++;
    if (0)
      outerDead++;
  }
  if (1) {
    hoisted++;
    a++;
    outer++;
    outerDead++;
  }
})();
`,
		},
	})
}

func TestDirectEvalTaintingNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function test1() {
					function add(first, second) {
						return first + second
					}
					eval('add(1, 2)')
				}

				function test2() {
					function add(first, second) {
						return first + second
					}
					(0, eval)('add(1, 2)')
				}

				function test3() {
					function add(first, second) {
						return first + second
					}
				}

				function test4(eval) {
					function add(first, second) {
						return first + second
					}
					eval('add(1, 2)')
				}

				function test5() {
					function containsDirectEval() { eval() }
					if (true) { var shouldNotBeRenamed }
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
		},
		bundleOptions: BundleOptions{
			IsBundling:        false,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
		expected: map[string]string{
			"/out.js": `function test1() {
  function add(b, a) {
    return b + a;
  }
  eval("add(1, 2)");
}
function test2() {
  function b(a, c) {
    return a + c;
  }
  (0, eval)("add(1, 2)");
}
function test3() {
  function b(a, c) {
    return a + c;
  }
}
function test4(eval) {
  function add(b, a) {
    return b + a;
  }
  eval("add(1, 2)");
}
function test5() {
  function containsDirectEval() {
    eval();
  }
  if (true) {
    var shouldNotBeRenamed;
  }
}
`,
		},
	})
}

func TestImportReExportES6Issue149(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/app.jsx": `
				import { p as Part, h, render } from './import';
				import { Internal } from './in2';
				const App = () => <Part> <Internal /> T </Part>;
				render(<App />, document.getElementById('app'));
			`,
			"/in2.jsx": `
				import { p as Part, h } from './import';
				export const Internal = () => <Part> Test 2 </Part>;
			`,
			"/import.js": `
				import { h, render } from 'preact';
				export const p = "p";
				export { h, render }
			`,
		},
		entryPaths: []string{"/app.jsx"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
			JSX: parser.JSXOptions{
				Factory: []string{"h"},
			},
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		resolveOptions: resolver.ResolveOptions{
			ExternalModules: map[string]bool{
				"preact": true,
			},
		},
		expected: map[string]string{
			"/out.js": `// /import.js
import {h, render} from "preact";
const p = "p";

// /in2.jsx
const Internal = () => h(p, null, " Test 2 ");

// /app.jsx
const App = () => h(p, null, h(Internal, null), " T ");
render(h(App, null), document.getElementById("app"));
`,
		},
	})
}

// This test case makes sure many entry points don't cause a crash
func TestManyEntryPoints(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/shared.js": `export default 123`,

			"/e00.js": `import x from './shared'; console.log(x)`,
			"/e01.js": `import x from './shared'; console.log(x)`,
			"/e02.js": `import x from './shared'; console.log(x)`,
			"/e03.js": `import x from './shared'; console.log(x)`,
			"/e04.js": `import x from './shared'; console.log(x)`,
			"/e05.js": `import x from './shared'; console.log(x)`,
			"/e06.js": `import x from './shared'; console.log(x)`,
			"/e07.js": `import x from './shared'; console.log(x)`,
			"/e08.js": `import x from './shared'; console.log(x)`,
			"/e09.js": `import x from './shared'; console.log(x)`,

			"/e10.js": `import x from './shared'; console.log(x)`,
			"/e11.js": `import x from './shared'; console.log(x)`,
			"/e12.js": `import x from './shared'; console.log(x)`,
			"/e13.js": `import x from './shared'; console.log(x)`,
			"/e14.js": `import x from './shared'; console.log(x)`,
			"/e15.js": `import x from './shared'; console.log(x)`,
			"/e16.js": `import x from './shared'; console.log(x)`,
			"/e17.js": `import x from './shared'; console.log(x)`,
			"/e18.js": `import x from './shared'; console.log(x)`,
			"/e19.js": `import x from './shared'; console.log(x)`,

			"/e20.js": `import x from './shared'; console.log(x)`,
			"/e21.js": `import x from './shared'; console.log(x)`,
			"/e22.js": `import x from './shared'; console.log(x)`,
			"/e23.js": `import x from './shared'; console.log(x)`,
			"/e24.js": `import x from './shared'; console.log(x)`,
			"/e25.js": `import x from './shared'; console.log(x)`,
			"/e26.js": `import x from './shared'; console.log(x)`,
			"/e27.js": `import x from './shared'; console.log(x)`,
			"/e28.js": `import x from './shared'; console.log(x)`,
			"/e29.js": `import x from './shared'; console.log(x)`,

			"/e30.js": `import x from './shared'; console.log(x)`,
			"/e31.js": `import x from './shared'; console.log(x)`,
			"/e32.js": `import x from './shared'; console.log(x)`,
			"/e33.js": `import x from './shared'; console.log(x)`,
			"/e34.js": `import x from './shared'; console.log(x)`,
			"/e35.js": `import x from './shared'; console.log(x)`,
			"/e36.js": `import x from './shared'; console.log(x)`,
			"/e37.js": `import x from './shared'; console.log(x)`,
			"/e38.js": `import x from './shared'; console.log(x)`,
			"/e39.js": `import x from './shared'; console.log(x)`,
		},
		entryPaths: []string{
			"/e00.js", "/e01.js", "/e02.js", "/e03.js", "/e04.js", "/e05.js", "/e06.js", "/e07.js", "/e08.js", "/e09.js",
			"/e10.js", "/e11.js", "/e12.js", "/e13.js", "/e14.js", "/e15.js", "/e16.js", "/e17.js", "/e18.js", "/e19.js",
			"/e20.js", "/e21.js", "/e22.js", "/e23.js", "/e24.js", "/e25.js", "/e26.js", "/e27.js", "/e28.js", "/e29.js",
			"/e30.js", "/e31.js", "/e32.js", "/e33.js", "/e34.js", "/e35.js", "/e36.js", "/e37.js", "/e38.js", "/e39.js",
		},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:   true,
			AbsOutputDir: "/out",
		},
		expected: map[string]string{
			"/out/e00.js": "// /shared.js\nconst shared_default = 123;\n\n// /e00.js\nconsole.log(shared_default);\n",
			"/out/e01.js": "// /shared.js\nconst shared_default = 123;\n\n// /e01.js\nconsole.log(shared_default);\n",
			"/out/e02.js": "// /shared.js\nconst shared_default = 123;\n\n// /e02.js\nconsole.log(shared_default);\n",
			"/out/e03.js": "// /shared.js\nconst shared_default = 123;\n\n// /e03.js\nconsole.log(shared_default);\n",
			"/out/e04.js": "// /shared.js\nconst shared_default = 123;\n\n// /e04.js\nconsole.log(shared_default);\n",
			"/out/e05.js": "// /shared.js\nconst shared_default = 123;\n\n// /e05.js\nconsole.log(shared_default);\n",
			"/out/e06.js": "// /shared.js\nconst shared_default = 123;\n\n// /e06.js\nconsole.log(shared_default);\n",
			"/out/e07.js": "// /shared.js\nconst shared_default = 123;\n\n// /e07.js\nconsole.log(shared_default);\n",
			"/out/e08.js": "// /shared.js\nconst shared_default = 123;\n\n// /e08.js\nconsole.log(shared_default);\n",
			"/out/e09.js": "// /shared.js\nconst shared_default = 123;\n\n// /e09.js\nconsole.log(shared_default);\n",

			"/out/e10.js": "// /shared.js\nconst shared_default = 123;\n\n// /e10.js\nconsole.log(shared_default);\n",
			"/out/e11.js": "// /shared.js\nconst shared_default = 123;\n\n// /e11.js\nconsole.log(shared_default);\n",
			"/out/e12.js": "// /shared.js\nconst shared_default = 123;\n\n// /e12.js\nconsole.log(shared_default);\n",
			"/out/e13.js": "// /shared.js\nconst shared_default = 123;\n\n// /e13.js\nconsole.log(shared_default);\n",
			"/out/e14.js": "// /shared.js\nconst shared_default = 123;\n\n// /e14.js\nconsole.log(shared_default);\n",
			"/out/e15.js": "// /shared.js\nconst shared_default = 123;\n\n// /e15.js\nconsole.log(shared_default);\n",
			"/out/e16.js": "// /shared.js\nconst shared_default = 123;\n\n// /e16.js\nconsole.log(shared_default);\n",
			"/out/e17.js": "// /shared.js\nconst shared_default = 123;\n\n// /e17.js\nconsole.log(shared_default);\n",
			"/out/e18.js": "// /shared.js\nconst shared_default = 123;\n\n// /e18.js\nconsole.log(shared_default);\n",
			"/out/e19.js": "// /shared.js\nconst shared_default = 123;\n\n// /e19.js\nconsole.log(shared_default);\n",

			"/out/e20.js": "// /shared.js\nconst shared_default = 123;\n\n// /e20.js\nconsole.log(shared_default);\n",
			"/out/e21.js": "// /shared.js\nconst shared_default = 123;\n\n// /e21.js\nconsole.log(shared_default);\n",
			"/out/e22.js": "// /shared.js\nconst shared_default = 123;\n\n// /e22.js\nconsole.log(shared_default);\n",
			"/out/e23.js": "// /shared.js\nconst shared_default = 123;\n\n// /e23.js\nconsole.log(shared_default);\n",
			"/out/e24.js": "// /shared.js\nconst shared_default = 123;\n\n// /e24.js\nconsole.log(shared_default);\n",
			"/out/e25.js": "// /shared.js\nconst shared_default = 123;\n\n// /e25.js\nconsole.log(shared_default);\n",
			"/out/e26.js": "// /shared.js\nconst shared_default = 123;\n\n// /e26.js\nconsole.log(shared_default);\n",
			"/out/e27.js": "// /shared.js\nconst shared_default = 123;\n\n// /e27.js\nconsole.log(shared_default);\n",
			"/out/e28.js": "// /shared.js\nconst shared_default = 123;\n\n// /e28.js\nconsole.log(shared_default);\n",
			"/out/e29.js": "// /shared.js\nconst shared_default = 123;\n\n// /e29.js\nconsole.log(shared_default);\n",

			"/out/e30.js": "// /shared.js\nconst shared_default = 123;\n\n// /e30.js\nconsole.log(shared_default);\n",
			"/out/e31.js": "// /shared.js\nconst shared_default = 123;\n\n// /e31.js\nconsole.log(shared_default);\n",
			"/out/e32.js": "// /shared.js\nconst shared_default = 123;\n\n// /e32.js\nconsole.log(shared_default);\n",
			"/out/e33.js": "// /shared.js\nconst shared_default = 123;\n\n// /e33.js\nconsole.log(shared_default);\n",
			"/out/e34.js": "// /shared.js\nconst shared_default = 123;\n\n// /e34.js\nconsole.log(shared_default);\n",
			"/out/e35.js": "// /shared.js\nconst shared_default = 123;\n\n// /e35.js\nconsole.log(shared_default);\n",
			"/out/e36.js": "// /shared.js\nconst shared_default = 123;\n\n// /e36.js\nconsole.log(shared_default);\n",
			"/out/e37.js": "// /shared.js\nconst shared_default = 123;\n\n// /e37.js\nconsole.log(shared_default);\n",
			"/out/e38.js": "// /shared.js\nconst shared_default = 123;\n\n// /e38.js\nconsole.log(shared_default);\n",
			"/out/e39.js": "// /shared.js\nconst shared_default = 123;\n\n// /e39.js\nconsole.log(shared_default);\n",
		},
	})
}

func TestRenamePrivateIdentifiersNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#foo
					foo = class {
						#foo
						#foo2
						#bar
					}
					get #bar() {}
					set #bar() {}
				}
				class Bar {
					#foo
					foo = class {
						#foo2
						#foo
						#bar
					}
					get #bar() {}
					set #bar() {}
				}
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
			"/out.js": `class Foo {
  #foo;
  foo = class {
    #foo2;
    #foo22;
    #bar2;
  };
  get #bar() {
  }
  set #bar() {
  }
}
class Bar {
  #foo;
  foo = class {
    #foo2;
    #foo3;
    #bar2;
  };
  get #bar() {
  }
  set #bar() {
  }
}
`,
		},
	})
}

func TestMinifyPrivateIdentifiersNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#foo
					foo = class {
						#foo
						#foo2
						#bar
					}
					get #bar() {}
					set #bar() {}
				}
				class Bar {
					#foo
					foo = class {
						#foo2
						#foo
						#bar
					}
					get #bar() {}
					set #bar() {}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
		},
		bundleOptions: BundleOptions{
			IsBundling:        false,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
		expected: map[string]string{
			"/out.js": `class Foo {
  #a;
  foo = class {
    #c;
    #d;
    #e;
  };
  get #b() {
  }
  set #b() {
  }
}
class Bar {
  #a;
  foo = class {
    #c;
    #d;
    #e;
  };
  get #b() {
  }
  set #b() {
  }
}
`,
		},
	})
}

func TestExportsAndModuleFormatCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as foo from './foo/test'
				import * as bar from './bar/test'
				console.log(exports, module.exports, foo, bar)
			`,
			"/foo/test.js": `
				export let foo = 123
			`,
			"/bar/test.js": `
				export let bar = 123
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
		resolveOptions: resolver.ResolveOptions{
			Platform: parser.PlatformNode,
		},

		// The "test_exports" names must be different
		expected: map[string]string{
			"/out.js": `// /foo/test.js
const test_exports2 = {};
__export(test_exports2, {
  foo: () => foo
});
let foo = 123;

// /bar/test.js
const test_exports = {};
__export(test_exports, {
  bar: () => bar
});
let bar = 123;

// /entry.js
console.log(exports, module.exports, test_exports2, test_exports);
`,
		},
	})
}

func TestMinifiedExportsAndModuleFormatCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as foo from './foo/test'
				import * as bar from './bar/test'
				console.log(exports, module.exports, foo, bar)
			`,
			"/foo/test.js": `
				export let foo = 123
			`,
			"/bar/test.js": `
				export let bar = 123
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
		resolveOptions: resolver.ResolveOptions{
			Platform: parser.PlatformNode,
		},

		// The "test_exports" names must be minified, and the "exports" and
		// "module" names must not be minified
		expected: map[string]string{
			"/out.js": `// /foo/test.js
const c = {};
e(c, {
  foo: () => i
});
let i = 123;

// /bar/test.js
const b = {};
e(b, {
  bar: () => h
});
let h = 123;

// /entry.js
console.log(exports, module.exports, c, b);
`,
		},
	})
}

// The minifier should not remove "use strict" or join it with other expressions
func TestUseStrictDirectiveMinifyNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				'use strict'
				'use loose'
				a
				b
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling:   false,
			MangleSyntax: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:       false,
			MangleSyntax:     true,
			RemoveWhitespace: true,
			AbsOutputFile:    "/out.js",
		},
		expected: map[string]string{
			"/out.js": `"use strict";a,b;
`,
		},
	})
}
