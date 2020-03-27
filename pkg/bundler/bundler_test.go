package bundler

import (
	"path"
	"testing"

	"github.com/progrium/esbuild/pkg/fs"
	"github.com/progrium/esbuild/pkg/logging"
	"github.com/progrium/esbuild/pkg/parser"
	"github.com/progrium/esbuild/pkg/resolver"
)

func assertEqual(t *testing.T, a interface{}, b interface{}) {
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
}

func assertLog(t *testing.T, msgs []logging.Msg, expected string) {
	text := ""
	for _, msg := range msgs {
		text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
	}
	assertEqual(t, text, expected)
}

type bundled struct {
	files              map[string]string
	entryPaths         []string
	expected           map[string]string
	expectedScanLog    string
	expectedCompileLog string
	parseOptions       parser.ParseOptions
	bundleOptions      BundleOptions
}

func expectBundled(t *testing.T, args bundled) {
	t.Run("", func(t *testing.T) {
		fs := fs.MockFS(args.files)
		resolver := resolver.NewResolver(fs, []string{".jsx", ".js", ".json"})

		log, join := logging.NewDeferLog()
		bundle := ScanBundle(log, fs, resolver, args.entryPaths, args.parseOptions)
		msgs := join()
		assertLog(t, msgs, args.expectedScanLog)

		// Stop now if there were any errors during the scan
		if len(msgs) > 0 {
			return
		}

		log, join = logging.NewDeferLog()
		args.bundleOptions.omitBootstrapForTests = true
		if args.bundleOptions.AbsOutputFile != "" {
			args.bundleOptions.AbsOutputDir = path.Dir(args.bundleOptions.AbsOutputFile)
		}
		results := bundle.Compile(log, args.bundleOptions)
		assertLog(t, join(), args.expectedCompileLog)

		assertEqual(t, len(results), len(args.expected))
		for _, result := range results {
			file := args.expected[result.JsAbsPath]
			path := "[" + result.JsAbsPath + "]\n"
			assertEqual(t, path+string(result.JsContents), path+file)
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
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /foo.js
    function fn() {
      return 123;
    }

    // /entry.js
    console.log(fn());
  }
}, 0);
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
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(require, exports, module) {
    // /foo.js
    module.exports = function() {
      return 123;
    };
  },

  0(require) {
    // /entry.js
    const fn = require(1 /* ./foo */);
    console.log(fn());
  }
}, 0);
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
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(require, exports, module) {
    // /foo.js
    module.exports = function() {
      return 123;
    };
  },

  0(require) {
    // /entry.js
    function nestedScope() {
      const fn = require(1 /* ./foo */);
      console.log(fn());
    }
    nestedScope();
  }
}, 0);
`,
		},
	})
}

func TestCommonJSFromES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const fn = require('./foo')
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
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(require, exports) {
    // /foo.js
    require(exports, {
      fn: () => fn
    });
    function fn() {
      return 123;
    }
  },

  0(require) {
    // /entry.js
    const fn = require(1 /* ./foo */);
    console.log(fn());
  }
}, 0);
`,
		},
	})
}

func TestES6FromCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {fn} from './foo'
				console.log(fn())
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
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(require, exports) {
    // /foo.js
    exports.fn = function() {
      return 123;
    };
  },

  0(require) {
    // /entry.js
    const foo = require(1 /* ./foo */, true /* ES6 import */);
    console.log(foo.fn());
  }
}, 0);
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
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(require, exports) {
    // /foo.js
    exports.fn = function() {
      return 123;
    };
  },

  0(require) {
    // /entry.js
    const foo = require(1 /* ./foo */, true /* ES6 import */);
    (() => {
      console.log(foo.fn());
    })();
  }
}, 0);
`,
		},
	})
}

func TestExportForms(t *testing.T) {
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
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  2(require, exports) {
    // /a.js
    const abc = void 0;

    // /b.js
    var b = {};
    require(b, {
      xyz: () => xyz
    });
    const xyz = null;

    // /entry.js
    require(exports, {
      C: () => Class,
      Class: () => Class,
      Fn: () => Fn,
      abc: () => abc,
      b: () => b,
      c: () => c,
      default: () => default2,
      l: () => l,
      v: () => v
    });
    const default2 = 123;
    var v = 234;
    let l = 234;
    const c = 234;
    function Fn() {
    }
    class Class {
    }
  }
}, 2);
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
		bundleOptions: BundleOptions{
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
			"/d.js": "export default class Foo {}",
			"/e.js": "export default function() {}",
			"/f.js": "export default function foo() {}",
			"/g.js": "export default async function() {}",
			"/h.js": "export default async function foo() {}",
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0(require, exports) {
    // /a.js
    require(exports, {
      abc: () => abc
    });
    const abc = void 0;
  },

  1(require, exports) {
    // /b.js
    require(exports, {
      xyz: () => xyz
    });
    const xyz = null;
  },

  3(require, exports) {
    // /commonjs.js
    require(exports, {
      C: () => Class,
      Class: () => Class,
      Fn: () => Fn,
      b: () => b,
      c: () => c,
      default: () => default2,
      l: () => l,
      v: () => v
    });
    const b = require(1 /* ./b */, true /* ES6 import */);
    const default2 = 123;
    var v = 234;
    let l = 234;
    const c = 234;
    function Fn() {
    }
    class Class {
    }
  },

  2(require, exports) {
    // /c.js
    require(exports, {
      default: () => default2
    });
    class default2 {
    }
  },

  4(require, exports) {
    // /d.js
    require(exports, {
      default: () => Foo
    });
    class Foo {
    }
  },

  5(require, exports) {
    // /e.js
    require(exports, {
      default: () => default2
    });
    function default2() {
    }
  },

  7(require, exports) {
    // /f.js
    require(exports, {
      default: () => foo
    });
    function foo() {
    }
  },

  8(require, exports) {
    // /g.js
    require(exports, {
      default: () => default2
    });
    async function default2() {
    }
  },

  9(require, exports) {
    // /h.js
    require(exports, {
      default: () => foo
    });
    async function foo() {
    }
  },

  6(require) {
    // /entry.js
    require(3 /* ./commonjs */);
    require(2 /* ./c */);
    require(4 /* ./d */);
    require(5 /* ./e */);
    require(7 /* ./f */);
    require(8 /* ./g */);
    require(9 /* ./h */);
  }
}, 6);
`,
		},
	})
}

func TestExportSelf(t *testing.T) {
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
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0(require, exports) {
    // /entry.js
    require(exports, {
      foo: () => foo
    });
    const foo = 123;
  }
}, 0);
`,
		},
	})
}

func TestExportSelfAsNamespace(t *testing.T) {
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
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0(require, exports) {
    // /entry.js
    require(exports, {
      foo: () => foo,
      ns: () => exports
    });
    const foo = 123;
  }
}, 0);
`,
		},
	})
}

func TestJSXImportsCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {elem, frag} from './custom-react'
				console.log(<div/>, <>fragment</>)
			`,
			"/custom-react.js": `
				module.exports = {}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
			JSX: parser.JSXOptions{
				Parse:    true,
				Factory:  []string{"elem"},
				Fragment: []string{"frag"},
			},
		},
		bundleOptions: BundleOptions{
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0(require, exports, module) {
    // /custom-react.js
    module.exports = {};
  },

  1(require) {
    // /entry.js
    const custom_react = require(0 /* ./custom-react */, true /* ES6 import */);
    console.log(custom_react.elem("div", null), custom_react.elem(custom_react.frag, null, "fragment"));
  }
}, 1);
`,
		},
	})
}

func TestJSXImportsES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {elem, frag} from './custom-react'
				console.log(<div/>, <>fragment</>)
			`,
			"/custom-react.js": `
				export function elem() {}
				export function frag() {}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
			JSX: parser.JSXOptions{
				Parse:    true,
				Factory:  []string{"elem"},
				Fragment: []string{"frag"},
			},
		},
		bundleOptions: BundleOptions{
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1() {
    // /custom-react.js
    function elem() {
    }
    function frag() {
    }

    // /entry.js
    console.log(elem("div", null), elem(frag, null, "fragment"));
  }
}, 1);
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
			Bundle:        true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  0(require, exports, module) {
    // /Users/user/project/node_modules/demo-pkg/index.js
    module.exports = function() {
      return 123;
    };
  },

  1(require) {
    // /Users/user/project/src/entry.js
    const demo_pkg = require(0 /* demo-pkg */, true /* ES6 import */);
    console.log(demo_pkg.default());
  }
}, 1);
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
			Bundle:        true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  0(require, exports, module) {
    // /Users/user/project/node_modules/demo-pkg/custom-main.js
    module.exports = function() {
      return 123;
    };
  },

  1(require) {
    // /Users/user/project/src/entry.js
    const demo_pkg = require(0 /* demo-pkg */, true /* ES6 import */);
    console.log(demo_pkg.default());
  }
}, 1);
`,
		},
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
			Bundle:        true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  1(require, exports, module) {
    // /Users/user/project/src/lib/util.js
    module.exports = function() {
      return 123;
    };
  },

  0(require) {
    // /Users/user/project/src/app/entry.js
    const util = require(1 /* lib/util */, true /* ES6 import */);
    console.log(util.default());
  }
}, 0);
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
			Bundle:        true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  0(require, exports, module) {
    // /Users/user/project/node_modules/demo-pkg/browser.js
    module.exports = function() {
      return 123;
    };
  },

  1(require) {
    // /Users/user/project/src/entry.js
    const demo_pkg = require(0 /* demo-pkg */, true /* ES6 import */);
    console.log(demo_pkg.default());
  }
}, 1);
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
			Bundle:        true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  0(require, exports, module) {
    // /Users/user/project/node_modules/demo-pkg/lib/util-browser.js
    module.exports = "util-browser";
  },

  1(require, exports, module) {
    // /Users/user/project/node_modules/demo-pkg/main-browser.js
    const util = require(0 /* ./lib/util */);
    module.exports = function() {
      return ["main-browser", util];
    };
  },

  2(require) {
    // /Users/user/project/src/entry.js
    const demo_pkg = require(1 /* demo-pkg */, true /* ES6 import */);
    console.log(demo_pkg.default());
  }
}, 2);
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
			Bundle:        true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  1(require, exports, module) {
    // /Users/user/project/node_modules/util-browser/index.js
    module.exports = "util-browser";
  },

  0(require, exports, module) {
    // /Users/user/project/node_modules/demo-pkg/main.js
    const util = require(1 /* ./util */);
    module.exports = function() {
      return ["main", util];
    };
  },

  2(require) {
    // /Users/user/project/src/entry.js
    const demo_pkg = require(0 /* demo-pkg */, true /* ES6 import */);
    console.log(demo_pkg.default());
  }
}, 2);
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
			Bundle:        true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  1() {
    // /Users/user/project/node_modules/demo-pkg/util-node.js
  },

  0(require, exports, module) {
    // /Users/user/project/node_modules/demo-pkg/main.js
    const util = require(1 /* ./util-node */);
    module.exports = function(obj) {
      return util.inspect(obj);
    };
  },

  2(require) {
    // /Users/user/project/src/entry.js
    const demo_pkg = require(0 /* demo-pkg */, true /* ES6 import */);
    console.log(demo_pkg.default());
  }
}, 2);
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
			Bundle:        true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  1(require, exports, module) {
    // /Users/user/project/node_modules/demo-pkg/node-pkg-browser.js
    module.exports = function() {
      return 123;
    };
  },

  0(require, exports, module) {
    // /Users/user/project/node_modules/demo-pkg/index.js
    const fn = require(1 /* node-pkg */);
    module.exports = function() {
      return fn();
    };
  },

  2(require) {
    // /Users/user/project/src/entry.js
    const demo_pkg = require(0 /* demo-pkg */, true /* ES6 import */);
    console.log(demo_pkg.default());
  }
}, 2);
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
			Bundle:        true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  1(require, exports, module) {
    // /Users/user/project/node_modules/node-pkg-browser/index.js
    module.exports = function() {
      return 123;
    };
  },

  0(require, exports, module) {
    // /Users/user/project/node_modules/demo-pkg/index.js
    const fn = require(1 /* node-pkg */);
    module.exports = function() {
      return fn();
    };
  },

  2(require) {
    // /Users/user/project/src/entry.js
    const demo_pkg = require(0 /* demo-pkg */, true /* ES6 import */);
    console.log(demo_pkg.default());
  }
}, 2);
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
			Bundle:        true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  1() {
    // /Users/user/project/node_modules/node-pkg/index.js
  },

  0(require, exports, module) {
    // /Users/user/project/node_modules/demo-pkg/index.js
    const fn = require(1 /* node-pkg */);
    module.exports = function() {
      return fn();
    };
  },

  2(require) {
    // /Users/user/project/src/entry.js
    const demo_pkg = require(0 /* demo-pkg */, true /* ES6 import */);
    console.log(demo_pkg.default());
  }
}, 2);
`,
		},
	})
}

func TestPackageImportMissingES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import fn, {x as a, y as b} from './foo'
				console.log(fn(a, b))
			`,
			"/foo.js": `
				export const x = 132
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expectedCompileLog: `/entry.js: error: No matching export for import "default"
/entry.js: error: No matching export for import "y"
`,
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /foo.js
    const x = 132;

    // /entry.js
    console.log(fn(x, b));
  }
}, 0);
`,
		},
	})
}

func TestPackageImportMissingCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import fn, {x as a, y as b} from './foo'
				console.log(fn(a, b))
			`,
			"/foo.js": `
				exports.x = 132
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(require, exports) {
    // /foo.js
    exports.x = 132;
  },

  0(require) {
    // /entry.js
    const foo = require(1 /* ./foo */, true /* ES6 import */);
    console.log(foo.default(foo.x, foo.y));
  }
}, 0);
`,
		},
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
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(require, exports) {
    // /index.js
    exports.x = 123;
  },

  0(require) {
    // /entry.js
    const _ = require(1 /* . */, true /* ES6 import */);
    console.log(_.x);
  }
}, 0);
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
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(require, exports, module) {
    // /test.json
    module.exports = {
      a: true,
      b: 123,
      c: [null]
    };
  },

  0(require) {
    // /entry.js
    console.log(require(1 /* ./test.json */));
  }
}, 0);
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
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(require, exports, module) {
    // /test.txt
    module.exports = "This is a test.";
  },

  0(require) {
    // /entry.js
    console.log(require(1 /* ./test.txt */));
  }
}, 0);
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
			Bundle:        true,
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
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /entry.js
    ((require2) => require2("/test.txt"))();
  }
}, 0);
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
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: error: "require" must not be called indirectly
`,
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
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: error: "require" must not be called indirectly
`,
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
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0(require) {
    // /entry.js
    try {
      oldLocale = globalLocale._abbr;
      var aliasedRequire = null;
      aliasedRequire("./locale/" + name);
      getSetGlobalLocale(oldLocale);
    } catch (e) {
    }
  }
}, 0);
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
			Bundle:        true,
			SourceMap:     true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  1() {
    // /Users/user/project/src/bar.js
    function bar2() {
      throw new Error("test");
    }

    // /Users/user/project/src/entry.js
    function foo() {
      bar2();
    }
    foo();
  }
}, 1);
//# sourceMappingURL=out.js.map
`,
		},
	})
}
