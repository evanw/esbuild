package snap_api

import (
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
				ProjectBaseDir + "/entry.js":
				`require('./body-parser')`,
				ProjectBaseDir + "/body-parser.js":
				`exports = module.exports = foo()`,
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
