package snap_printer

import "testing"

// test('simple require')
func TestElinkSimpleRequire(t *testing.T) {
	expectPrinted(t, `
const a = require('a')
const b = require('b')
function main () {
  const c = {a: b, b: a}
  return a + b
}
    `, `
let a;
function __get_a__() {
  return a = a || (require("a"))
}
const b = require("b");
function main() {
  const c = {a: b, b: (__get_a__())};
  return (__get_a__()) + b;
}
`,
		func(mod string) bool { return mod == "a" })
}

// test('conditional requires')
func TestElinkConditionalRequires(t *testing.T) {
	expectPrinted(t, `
let a, b;
if (condition) {
  a = require('a')
  b = require('b')
} else {
  a = require('c')
  b = require('d')
}

function main () {
  return a + b
}
    `, `
let __get_a__;
let a, b;
if (condition) {
  
__get_a__ = function() {
  return a = a || (require("a"))
};
  b = require("b");
} else {
  
__get_a__ = function() {
  return a = a || (require("c"))
};
  b = require("d");
}
function main() {
  return (__get_a__()) + b;
}
`,
		func(mod string) bool { return mod == "a" || mod == "c" })
}

// test('top-level variables assignments that depend on previous requires')
func TestElinkVarAssignmentsDependingOnPreviousRequires(t *testing.T) {
	expectPrinted(t, `
const a = require('a')
const b = require('b')
const c = require('c').foo.bar
const d = c.X | c.Y | c.Z
var e
e = c.e
const f = b.f
function main () {
  c.qux()
  console.log(d)
  e()
} `, `
let __get_e__;

let a;
function __get_a__() {
  return a = a || (require("a"))
}
const b = require("b");

let c;
function __get_c__() {
  return c = c || (require("c").foo.bar)
}

let d;
function __get_d__() {
  return d = d || ((__get_c__()).X | (__get_c__()).Y | (__get_c__()).Z)
}
var e;

__get_e__ = function() {
  return e = e || ((__get_c__()).e)
};
const f = b.f;
function main() {
  (__get_c__()).qux();
  get_console().log((__get_d__()));
  (__get_e__())();
}`, func(mod string) bool { return mod == "a" || mod == "c" })

}

//
// Function Closures
//

// First three following are parts of the related electron-link example which is
// tested in one piece in the forth test
// test('requires that appear in a closure wrapper defined in the top-level scope (e.g. CoffeeScript)')
func TestElinkTopLevelClosureWrapperCall(t *testing.T) {
	expectPrinted(t, `
(function () {
	const a = require('a')
	const b = require('b')
	function main () {
		return a + b
	}
}).call(this)
`, `
(function() {

let a;
function __get_a__() {
  return a = a || (require("a"))
}

let b;
function __get_b__() {
  return b = b || (require("b"))
}
  function main() {
    return (__get_a__()) + (__get_b__());
  }
}).call(this);
`, ReplaceAll)
}

func TestElinkTopLevelClosureWrapperSelfExecuteFiltered(t *testing.T) {
	expectPrinted(t, `
(function () {
  const a = require('a')
  const b = require('b')
  function main () {
    return a + b
  }
})()
`, `
(function() {

let a;
function __get_a__() {
  return a = a || (require("a"))
}
  const b = require("b");
  function main() {
    return (__get_a__()) + b;
  }
})();
`,
		func(mod string) bool { return mod == "a" },
	)
}

// NOTE: electron-link does not rewrite anything here, however this may be a mistake as
// `foo` might invoke the callback synchronously when it runs and thus execute the `require`s
// For now we conform to this (possibly incorrect) behavior
func TestElinkTopLevelFunctionInvokingCallback(t *testing.T) {
	expectPrinted(t, `
foo(function () {
  const b = require('b')
  const c = require('c')
  function main () {
    return b + c
  }
})
`, `
foo(function() {
  const b = require("b");
  const c = require("c");
  function main() {
    return b + c;
  }
});
`, ReplaceAll)
}

// Note that a missing semicolon was added here in order to fix this invalid code as it was
// found inside electron-link tests
func TestElinkTopLevelClosureCompleteFiltered(t *testing.T) {
	expectPrinted(t, `
(function () {
  const a = require('a')
  const b = require('b')
  function main () {
    return a + b
  }
}).call(this)

;(function () {
  const a = require('a')
  const b = require('b')
  function main () {
    return a + b
  }
})()

foo(function () {
  const b = require('b')
  const c = require('c')
  function main () {
    return b + c
  }
})
`, `
(function() {

let a;
function __get_a__() {
  return a = a || (require("a"))
}
  const b = require("b");
  function main() {
    return (__get_a__()) + b;
  }
}).call(this);
(function() {

let a;
function __get_a__() {
  return a = a || (require("a"))
}
  const b = require("b");
  function main() {
    return (__get_a__()) + b;
  }
})();
foo(function() {
  const b = require("b");
  const c = require("c");
  function main() {
    return b + c;
  }
});
`, func(mod string) bool { return mod == "a" || mod == "c" })
}

// test('references to shadowed variables')
func TestElinkReferencesToShadowedVars(t *testing.T) {
	expectPrinted(t, `
const a = require('a')
function outer () {
  console.log(a)
  function inner () {
    console.log(a)
  }
  let a = []
}

function other () {
  console.log(a)
  function inner () {
    let a = []
    console.log(a)
  }
}
`, `
let a;
function __get_a__() {
  return a = a || (require("a"))
}
function outer() {
  get_console().log(a);
  function inner() {
    get_console().log(a);
  }
  let a = [];
}
function other() {
  get_console().log((__get_a__()));
  function inner() {
    let a = [];
    get_console().log(a);
  }
}
`,
		func(mod string) bool { return mod == "a" })
}

// test('references to globals')
func TestElinkReferencesToGlobals(t *testing.T) {
	expectPrinted(t, `
global.a = 1
process.b = 2
window.c = 3
document.d = 4

function inner () {
  const window = {}
  global.e = 4
  process.f = 5
  window.g = 6
  document.h = 7
}
`, `
get_global().a = 1;
get_process().b = 2;
get_window().c = 3;
get_document().d = 4;
function inner() {
  const window = {};
  get_global().e = 4;
  get_process().f = 5;
  window.g = 6;
  get_document().h = 7;
}
`, ReplaceAll)
}

// test('multiple assignments separated by commas referencing deferred modules')
func TestElinkMultipleAssignmentsByCommaReferencingDeferredModules(t *testing.T) {
	expectPrinted(t, `
let a, b, c, d, e, f;
a = 1, b = 2, c = 3;
d = require("d"), e = d.e, f = e.f;
`, `
let __get_d__, __get_e__, __get_f__;
let a, b, c, d, e, f;
a = 1, b = 2, c = 3;

__get_d__ = function() {
  return d = d || (require("d"))
}, 
__get_e__ = function() {
  return e = e || ((__get_d__()).e)
}, 
__get_f__ = function() {
  return f = f || ((__get_e__()).f)
};
`, ReplaceAll)
}

// test('require with destructuring assignment')
func TestElinkRequireWithDestructuringAssignment(t *testing.T) {
	expectPrinted(t, `
const {a, b, c} = require('module').foo

function main() {
  a.bar()
}
`, `
let a;
function __get_a__() {
  return a = a || (require("module").foo.a)
}

let b;
function __get_b__() {
  return b = b || (require("module").foo.b)
}

let c;
function __get_c__() {
  return c = c || (require("module").foo.c)
}
function main() {
  (__get_a__()).bar();
}
`, ReplaceAll)
}

// This is covered by the bundler which rewrites JSON files appropriately
// test('JSON source') line 322

// test('Object spread properties')
// - merely assuring that we handle it, no rewrite
func TestElinkObjectSpreadProperties(t *testing.T) {
	expectPrinted(t, `
let {a, b, ...rest} = {a: 1, b: 2, c: 3}
`, `
let {a, b, ...rest} = {a: 1, b: 2, c: 3};
`, ReplaceAll)
}

// TODO: this is about rewriting require strings depending on a basedir
//   which is handled at the bundler level.
// test('path resolution') line 353

// test('use reference directly')
func TestElinkUseReferenceDirectly(t *testing.T) {
	expectPrinted(t, `
var pack = require('pack')

const x = console.log(pack);
if (condition) {
  pack
} else {
Object.keys(pack).forEach(function (prop) {
  exports[prop] = pack[prop]
})
}
`, `
let pack;
function __get_pack__() {
  return pack = pack || (require("pack"))
}

let x;
function __get_x__() {
  return x = x || (get_console().log((__get_pack__())))
}
if (condition) {
  (__get_pack__());
} else {
  Object.keys((__get_pack__())).forEach(function(prop) {
    exports[prop] = (__get_pack__())[prop];
  });
}
`, ReplaceAll)
}

// test('assign to `module` or `exports`')
func TestElinkAssignToModuleOrExports(t *testing.T) {
	expectPrinted(t, `
var pack = require('pack')      
if (condition) {
  module.exports.pack = pack
  module.exports = pack
  exports.pack = pack
  exports = pack
}
`, `
let pack;
function __get_pack__() {
  return pack = pack || (require("pack"))
}
if (condition) {
  Object.defineProperty(module.exports, "pack", { get: () => (__get_pack__()) });
  module.exports = (__get_pack__());
  Object.defineProperty(exports, "pack", { get: () => (__get_pack__()) });
  exports = (__get_pack__());
}
`, ReplaceAll)
}
