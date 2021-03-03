package snap_api

import (
	"github.com/evanw/esbuild/internal/js_ast"
	"testing"
)

var snapApiSuite = suite{
	name: "Snap API",
}

func TestEntryRequiringLocalModule(t *testing.T) {
	snapApiSuite.expectBuild(t, built{
		files: map[string]string{
			ProjectBaseDir + "/entry.js": `
				const { oneTwoThree } = require('./foo')
                module.exports = function () {
				  console.log(oneTwoThree)
			    }
			`,
			ProjectBaseDir + "/foo.js": `exports.oneTwoThree = 123`,
		},
		entryPoints: []string{ProjectBaseDir + "/entry.js"},
	},
		buildResult{
			files: map[string]string{
				ProjectBaseDir + "/entry.js": `
__commonJS["./entry.js"] = function(exports, module, __filename, __dirname, require) {
let oneTwoThree;
function __get_oneTwoThree__() {
  return oneTwoThree = oneTwoThree || (require("./foo.js").oneTwoThree)
}
  module.exports = function() {
    get_console().log((__get_oneTwoThree__()));
  };
};`,
				ProjectBaseDir + `/foo.js`: `
__commonJS["./foo.js"] = function(exports2, module2, __filename, __dirname, require) {
  exports2.oneTwoThree = 123;
};`,
			},
		},
	)
}

// TODO: what about __toModule?
//   - @see snap_printer.go:1078 (printRequireOrImportExpr)
func TestEntryImportingLocalModule(t *testing.T) {
	snapApiSuite.expectBuild(t, built{
		files: map[string]string{
			ProjectBaseDir + "/entry.js": `
				import { oneTwoThree } from'./foo'
                module.exports = function () {
				  console.log(oneTwoThree)
			    }
			`,
			ProjectBaseDir + "/foo.js": `exports.oneTwoThree = 123`,
		},
		entryPoints: []string{ProjectBaseDir + "/entry.js"},
	},
		buildResult{
			files: map[string]string{
				ProjectBaseDir + `/foo.js`: `
__commonJS["./foo.js"] = function(exports2, module2, __filename, __dirname, require) {
  exports2.oneTwoThree = 123;
};`,
				ProjectBaseDir + `/entry.js`: `
__commonJS["./entry.js"] = function(exports, module, __filename, __dirname, require) {
let foo;
function __get_foo__() {
  return foo = foo || (__toModule(require("./foo.js")))
}
  module.exports = function() {
    get_console().log((__get_foo__()).oneTwoThree);
  };
};`,
			},
		},
	)
}
func TestCallingResultOfRequiringModule(t *testing.T) {
	snapApiSuite.expectBuild(t, built{
		files: map[string]string{
			ProjectBaseDir + "/entry.js": `
var deprecate = require('./depd')('http-errors')
module.exports = function () { deprecate() }
`,
			ProjectBaseDir + "/depd.js": "module.exports = function (s) {}",
		},
		entryPoints: []string{ProjectBaseDir + "/entry.js"},
	},

		buildResult{
			files: map[string]string{
				ProjectBaseDir + `/entry.js`: `
__commonJS["./entry.js"] = function(exports, module, __filename, __dirname, require) {
let deprecate;
function __get_deprecate__() {
  return deprecate = deprecate || (require("./depd.js")("http-errors"))
}
  module.exports = function() {
    (__get_deprecate__())();
  };
};`,
			},
		},
	)
}

func TestNotWrappingExports(t *testing.T) {
	snapApiSuite.expectBuild(t,
		built{
			files: map[string]string{
				ProjectBaseDir + "/entry.js":       `require('./body-parser')`,
				ProjectBaseDir + "/body-parser.js": `exports = module.exports = foo()`,
			},
			entryPoints: []string{ProjectBaseDir + "/entry.js"},
		},
		buildResult{
			files: map[string]string{
				ProjectBaseDir + "/body-parser.js": `
__commonJS["./body-parser.js"] = function(exports2, module2, __filename, __dirname, require) {
  exports2 = module2.exports = foo();
};`,
				ProjectBaseDir + "/entry.js": `
__commonJS["./entry.js"] = function(exports, module, __filename, __dirname, require) {
  require("./body-parser.js");
};`,
			},
		},
	)
}

func TestDeclarationsInsertedAfterUseStrict(t *testing.T) {
	snapApiSuite.expectBuild(t, built{
		files: map[string]string{
			ProjectBaseDir + "/entry.js": `
"use strict";
var old;
old = Promise;
`,
		},
		entryPoints: []string{ProjectBaseDir + "/entry.js"},
	},
		buildResult{
			files: map[string]string{
				ProjectBaseDir + `/entry.js`: `
__commonJS["./entry.js"] = function(exports, module, __filename, __dirname, require) {
  "use strict";
let __get_old__;
  var old;
  
__get_old__ = function() {
  return old = old || (Promise)
};
};`,
			},
		},
	)
}

func TestMissingFileRequiredOnlyWarns(t *testing.T) {
	snapApiSuite.expectBuild(t, built{
		files: map[string]string{
			ProjectBaseDir + "/entry.js": `
require('non-existent')
`,
		},
		entryPoints: []string{ProjectBaseDir + "/entry.js"},
	},
		buildResult{
			files: map[string]string{
				ProjectBaseDir + `/entry.js`: `
__commonJS["./entry.js"] = function(exports, module, __filename, __dirname, require) {
  require("non-existent");
};`,
			},
		})
}

// @see https://github.com/evanw/esbuild/commit/918d44e7e2912fa23f9ba409e1d6623275f7b83f
func TestNestedScopeVarsAreNotRelocated(t *testing.T) {
	snapApiSuite.expectBuild(t, built{
		files: map[string]string{
			ProjectBaseDir + "/entry.js": `
{ var obj = Array.from({}) }
`,
		},
		entryPoints: []string{ProjectBaseDir + "/entry.js"},
	},
		buildResult{
			files: map[string]string{
				ProjectBaseDir + `/entry.js`: `
__commonJS["./entry.js"] = function(exports, module, __filename, __dirname, require) {
  {
let obj;
function __get_obj__() {
  return obj = obj || (Array.from({}))
}
  }
};`,
			},
		},
	)
}

func TestShouldRewriteModule(t *testing.T) {
	snapApiSuite.expectBuild(t, built{
		shouldRewriteModule: func(filePath string) bool {
			return filePath != ProjectBaseDir[1:]+"/foo.js"
		},
		files: map[string]string{
			ProjectBaseDir + "/foo.js": `var fs = require('fs')`,
			ProjectBaseDir + "/bar.js": `var path = require('path')`,
			ProjectBaseDir + "/entry.js": `
exports.foo = require('./foo')
exports.bar = require('./bar')
`,
		},
		entryPoints: []string{ProjectBaseDir + "/entry.js"},
	},
		buildResult{
			files: map[string]string{
				`dev/foo.js`: `
__commonJS["./foo.js"] = function(exports2, module2, __filename, __dirname, require) {
  var fs = require("fs");
};`,
				`dev/bar.js`: `
__commonJS["./bar.js"] = function(exports2, module2, __filename, __dirname, require) {
let path;
function __get_path__() {
  return path = path || (require("path"))
}
};`,
				`dev/entry.js`: `
__commonJS["./entry.js"] = function(exports, module, __filename, __dirname, require) {
  Object.defineProperty(exports, "foo", { get: () => require("./foo.js") });
  Object.defineProperty(exports, "bar", { get: () => require("./bar.js") });
};`,
			},
		},
	)
}

func TestPreventResolutionOfNativeModules(t *testing.T) {
	snapApiSuite.expectBuild(t, built{
		shouldRewriteModule: func(filePath string) bool {
			return false
		},
		files: map[string]string{
			ProjectBaseDir + "/node_modules/fsevents/fsevents.js": `
const Native = require('./fsevents.node');
const events = Native.constants;
`,
			ProjectBaseDir + "/entry.js": `
exports.fsevents = require('` + ProjectBaseDir + `/node_modules/fsevents/fsevents.js')
`,
		},
		entryPoints: []string{ProjectBaseDir + "/entry.js"},
	},
		buildResult{
			files: map[string]string{
				`dev/node_modules/fsevents/fsevents.js`: `
__commonJS["./node_modules/fsevents/fsevents.js"] = function(exports2, module2, __filename, __dirname, require) {
  var Native = require("./node_modules/fsevents/fsevents.node");
  var events = Native.constants;
};`,
				`dev/entry.js`: `
__commonJS["./entry.js"] = function(exports, module, __filename, __dirname, require) {
  exports.fsevents = require("./node_modules/fsevents/fsevents.js");
};`,
			},
		},
	)
}

func TestRejectAstWhenDirnameIsUsedExcludesModuleUsingDirname(t *testing.T) {
	// NOTE: also logs 'Forbidden use of __dirname' to stderr
	snapApiSuite.expectBuild(t, built{
		shouldRejectAst: func(tree *js_ast.AST) (string, bool) {
			if tree.UsesDirnameRef {
				return "Forbidden use of __dirname", true
			}
			return "", false
		},
		files: map[string]string{
			ProjectBaseDir + "/node_modules/fsevents/fsevents.js": `
module.exports = __dirname
`,
			ProjectBaseDir + "/entry.js": `
exports.fsevents = require('` + ProjectBaseDir + `/node_modules/fsevents/fsevents.js')
`,
		},
		entryPoints: []string{ProjectBaseDir + "/entry.js"},
	},
		buildResult{
			files: map[string]string{
				`dev/entry.js`: `
__commonJS["./entry.js"] = function(exports, module, __filename, __dirname, require) {
  Object.defineProperty(exports, "fsevents", { get: () => require("./node_modules/fsevents/fsevents.js") });
};`,
			},
		},
	)
}

func TestDebug(t *testing.T) {
	snapApiSuite.debugBuild(t, built{
		files: map[string]string{
			"/entry.js": `
"use strict";
var old;
old = Promise;
`,
		},
		entryPoints: []string{"/entry.js"},
	},
	)
}
