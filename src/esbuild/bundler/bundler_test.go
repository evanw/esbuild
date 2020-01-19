package bundler

import (
	"esbuild/fs"
	"esbuild/logging"
	"esbuild/parser"
	"esbuild/resolver"
	"path"
	"testing"
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
	files         map[string]string
	entryPaths    []string
	expected      map[string]string
	expectedLog   string
	parseOptions  parser.ParseOptions
	bundleOptions BundleOptions
}

func expectBundled(t *testing.T, args bundled) {
	t.Run("", func(t *testing.T) {
		resolver := resolver.NewResolver(fs.MockFS(args.files), []string{".jsx", ".js", ".json"})

		log, join := logging.NewDeferLog()
		args.parseOptions.IsBundling = true
		bundle := ScanBundle(log, resolver, args.entryPaths, args.parseOptions)
		assertLog(t, join(), "")

		log, join = logging.NewDeferLog()
		args.bundleOptions.Bundle = true
		args.bundleOptions.omitLoaderForTests = true
		if args.bundleOptions.AbsOutputFile != "" {
			args.bundleOptions.AbsOutputDir = path.Dir(args.bundleOptions.AbsOutputFile)
		}
		results := bundle.Compile(log, args.bundleOptions)
		assertLog(t, join(), args.expectedLog)

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
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `loader({
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
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `loader({
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
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `loader({
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
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `loader({
  1(require, exports) {
    // /foo.js
    exports.fn = function() {
      return 123;
    };
  },

  0(require) {
    // /entry.js
    const foo = require(1 /* ./foo */);
    console.log(foo.fn());
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
			"/a.js": `
				export const abc = undefined
			`,
			"/b.js": `
				export const xyz = null
			`,
		},
		entryPaths: []string{"/entry.js"},
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `loader({
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

func TestExportSelf(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export const foo = 123
				export * from './entry'
			`,
		},
		entryPaths: []string{"/entry.js"},
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `loader({
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
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `loader({
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
			JSX: parser.JSXOptions{
				Parse:    true,
				Factory:  []string{"elem"},
				Fragment: []string{"frag"},
			},
		},
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `loader({
  0(require, exports, module) {
    // /custom-react.js
    module.exports = {};
  },

  1(require) {
    // /entry.js
    const custom_react = require(0 /* ./custom-react */);
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
			JSX: parser.JSXOptions{
				Parse:    true,
				Factory:  []string{"elem"},
				Fragment: []string{"frag"},
			},
		},
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `loader({
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
		bundleOptions: BundleOptions{
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `loader({
  0(require, exports, module) {
    // /Users/user/project/node_modules/demo-pkg/index.js
    module.exports = function() {
      return 123;
    };
  },

  1(require) {
    // /Users/user/project/src/entry.js
    const demo_pkg = require(0 /* demo-pkg */);
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
		bundleOptions: BundleOptions{
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `loader({
  0(require, exports, module) {
    // /Users/user/project/node_modules/demo-pkg/custom-main.js
    module.exports = function() {
      return 123;
    };
  },

  1(require) {
    // /Users/user/project/src/entry.js
    const demo_pkg = require(0 /* demo-pkg */);
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
		bundleOptions: BundleOptions{
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `loader({
  1(require, exports, module) {
    // /Users/user/project/src/lib/util.js
    module.exports = function() {
      return 123;
    };
  },

  0(require) {
    // /Users/user/project/src/app/entry.js
    const util = require(1 /* lib/util */);
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
		bundleOptions: BundleOptions{
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `loader({
  0(require, exports, module) {
    // /Users/user/project/node_modules/demo-pkg/browser.js
    module.exports = function() {
      return 123;
    };
  },

  1(require) {
    // /Users/user/project/src/entry.js
    const demo_pkg = require(0 /* demo-pkg */);
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
						"./util.js": "./util-browser"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				const util = require('./util')
				module.exports = function() {
					return ['main', util]
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main-browser.js": `
				const util = require('./util')
				module.exports = function() {
					return ['main-browser', util]
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/util.js": `
				module.exports = 'util'
			`,
			"/Users/user/project/node_modules/demo-pkg/util-browser.js": `
				module.exports = 'util-browser'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		bundleOptions: BundleOptions{
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `loader({
  1(require, exports, module) {
    // /Users/user/project/node_modules/demo-pkg/util-browser.js
    module.exports = "util-browser";
  },

  0(require, exports, module) {
    // /Users/user/project/node_modules/demo-pkg/main-browser.js
    const util = require(1 /* ./util */);
    module.exports = function() {
      return ["main-browser", util];
    };
  },

  2(require) {
    // /Users/user/project/src/entry.js
    const demo_pkg = require(0 /* demo-pkg */);
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
		bundleOptions: BundleOptions{
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `loader({
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
    const demo_pkg = require(0 /* demo-pkg */);
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
		bundleOptions: BundleOptions{
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `loader({
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
    const demo_pkg = require(0 /* demo-pkg */);
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
		bundleOptions: BundleOptions{
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `loader({
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
    const demo_pkg = require(0 /* demo-pkg */);
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
		bundleOptions: BundleOptions{
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `loader({
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
    const demo_pkg = require(0 /* demo-pkg */);
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
		bundleOptions: BundleOptions{
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `loader({
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
    const demo_pkg = require(0 /* demo-pkg */);
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
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expectedLog: `/entry.js: error: No matching export for import "default"
/entry.js: error: No matching export for import "y"
`,
		expected: map[string]string{
			"/out.js": `loader({
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
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `loader({
  1(require, exports) {
    // /foo.js
    exports.x = 132;
  },

  0(require) {
    // /entry.js
    const foo = require(1 /* ./foo */);
    console.log(foo.default(foo.x, foo.y));
  }
}, 0);
`,
		},
	})
}
