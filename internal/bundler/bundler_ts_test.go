package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/parser"
)

func TestTSDeclareConst(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare const require: any
				declare const exports: any;
				declare const module: any

				declare const foo: any
				let foo = bar()
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
let foo = bar();
`,
		},
	})
}

func TestTSDeclareLet(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare let require: any
				declare let exports: any;
				declare let module: any

				declare let foo: any
				let foo = bar()
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
let foo = bar();
`,
		},
	})
}

func TestTSDeclareVar(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare var require: any
				declare var exports: any;
				declare var module: any

				declare var foo: any
				let foo = bar()
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
let foo = bar();
`,
		},
	})
}

func TestTSDeclareClass(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare class require {}
				declare class exports {};
				declare class module {}

				declare class foo {}
				let foo = bar()
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
let foo = bar();
`,
		},
	})
}

func TestTSDeclareFunction(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare function require(): void
				declare function exports(): void;
				declare function module(): void

				declare function foo() {}
				let foo = bar()
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
let foo = bar();
`,
		},
	})
}

func TestTSDeclareNamespace(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare namespace require {}
				declare namespace exports {};
				declare namespace module {}

				declare namespace foo {}
				let foo = bar()
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
let foo = bar();
`,
		},
	})
}

func TestTSDeclareEnum(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare enum require {}
				declare enum exports {};
				declare enum module {}

				declare enum foo {}
				let foo = bar()
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
let foo = bar();
`,
		},
	})
}

func TestTSDeclareConstEnum(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare const enum require {}
				declare const enum exports {};
				declare const enum module {}

				declare const enum foo {}
				let foo = bar()
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
let foo = bar();
`,
		},
	})
}

func TestTSImportEmptyNamespace(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {ns} from './ns.ts'
				function foo(): ns.type {}
				foo();
			`,
			"/ns.ts": `
				export namespace ns {}
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
function foo() {
}
foo();
`,
		},
	})
}

func TestTSImportMissingES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import fn, {x as a, y as b} from './foo'
				console.log(fn(a, b))
			`,
			"/foo.js": `
				export const x = 123
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
		expectedCompileLog: `/entry.ts: error: No matching export for import "default"
/entry.ts: error: No matching export for import "y"
`,
	})
}

func TestTSImportMissingUnusedES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import fn, {x as a, y as b} from './foo'
			`,
			"/foo.js": `
				export const x = 123
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
			"/out.js": ``,
		},
	})
}

func TestTSExportMissingES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './foo'
				console.log(ns)
			`,
			"/foo.ts": `
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
		expected: map[string]string{
			"/out.js": `// /bar.js

// /foo.ts
const foo_exports = {};

// /entry.js
console.log(foo_exports);
`,
		},
	})
}

// It's an error to import from a file that does not exist
func TestTSImportMissingFile(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {Something} from './doesNotExist.ts'
				let foo = new Something
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
		expectedScanLog: "/entry.ts: error: Could not resolve \"./doesNotExist.ts\"\n",
	})
}

// It's not an error to import a type from a file that does not exist
func TestTSImportTypeOnlyFile(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {SomeType1} from './doesNotExist1.ts'
				import {SomeType2} from './doesNotExist2.ts'
				let foo: SomeType1 = bar()
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
let foo = bar();
`,
		},
	})
}

func TestTSExportEquals(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.ts": `
				import b from './b.ts'
				console.log(b)
			`,
			"/b.ts": `
				export = [123, foo]
				function foo() {}
			`,
		},
		entryPaths: []string{"/a.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /b.ts
var require_b = __commonJS((exports, module) => {
  function foo() {
  }
  module.exports = [123, foo];
});

// /a.ts
const b = __toModule(require_b());
console.log(b.default);
`,
		},
	})
}

func TestTSExportNamespace(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.ts": `
				import {Foo} from './b.ts'
				console.log(new Foo)
			`,
			"/b.ts": `
				export class Foo {}
				export namespace Foo {
					export let foo = 1
				}
				export namespace Foo {
					export let bar = 2
				}
			`,
		},
		entryPaths: []string{"/a.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /b.ts
class Foo {
}
(function(Foo2) {
  Foo2.foo = 1;
})(Foo || (Foo = {}));
(function(Foo2) {
  Foo2.bar = 2;
})(Foo || (Foo = {}));

// /a.ts
console.log(new Foo());
`,
		},
	})
}

func TestTSMinifyEnum(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.ts": `
				enum Foo { A, B, C = Foo }
			`,
			"/b.ts": `
				export enum Foo { X, Y, Z = Foo }
			`,
		},
		entryPaths: []string{"/a.ts", "/b.ts"},
		parseOptions: parser.ParseOptions{
			MangleSyntax: true,
		},
		bundleOptions: BundleOptions{
			RemoveWhitespace:  true,
			MinifyIdentifiers: true,
			AbsOutputDir:      "/",
		},
		expected: map[string]string{
			"/a.js": "var Foo;(function(a){a[a.A=0]=\"A\",a[a.B=1]=\"B\",a[a.C=a]=\"C\"})(Foo||(Foo={}));\n",
			"/b.js": "export var Foo;(function(a){a[a.X=0]=\"X\",a[a.Y=1]=\"Y\",a[a.Z=a]=\"Z\"})(Foo||(Foo={}));\n",
		},
	})
}

func TestTSMinifyNamespace(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.ts": `
				namespace Foo {
					export namespace Bar {
						foo(Foo, Bar)
					}
				}
			`,
			"/b.ts": `
				export namespace Foo {
					export namespace Bar {
						foo(Foo, Bar)
					}
				}
			`,
		},
		entryPaths: []string{"/a.ts", "/b.ts"},
		parseOptions: parser.ParseOptions{
			MangleSyntax: true,
		},
		bundleOptions: BundleOptions{
			RemoveWhitespace:  true,
			MinifyIdentifiers: true,
			AbsOutputDir:      "/",
		},
		expected: map[string]string{
			"/a.js": "var Foo;(function(a){let b;(function(c){foo(a,c)})(b=a.Bar||(a.Bar={}))})(Foo||(Foo={}));\n",
			"/b.js": "export var Foo;(function(a){let b;(function(c){foo(a,c)})(b=a.Bar||(a.Bar={}))})(Foo||(Foo={}));\n",
		},
	})
}

func TestTSMinifyDerivedClass(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class Foo extends Bar {
					foo = 1;
					bar = 2;
					constructor() {
						super();
						foo();
						bar();
					}
				}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			MangleSyntax: true,
			Target:       parser.ES2015,
		},
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `class Foo extends Bar {
  constructor() {
    super();
    this.foo = 1;
    this.bar = 2;
    foo(), bar();
  }
}
`,
		},
	})
}

func TestTSImportVsLocalCollisionAllTypes(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {a, b, c, d, e} from './other.ts'
				let a
				const b = 0
				var c
				function d() {}
				class e {}
				console.log(a, b, c, d, e)
			`,
			"/other.ts": `
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
let a;
const b = 0;
var c;
function d() {
}
class e {
}
console.log(a, b, c, d, e);
`,
		},
	})
}

func TestTSImportVsLocalCollisionMixed(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {a, b, c, d, e, real} from './other.ts'
				let a
				const b = 0
				var c
				function d() {}
				class e {}
				console.log(a, b, c, d, e, real)
			`,
			"/other.ts": `
				export let real = 123
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
			"/out.js": `// /other.ts
let real = 123;

// /entry.ts
let a;
const b = 0;
var c;
function d() {
}
class e {
}
console.log(a, b, c, d, e, real);
`,
		},
	})
}

func TestTSMinifiedBundleES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {foo} from './a'
				console.log(foo())
			`,
			"/a.ts": `
				export function foo() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/entry.ts"},
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
			"/out.js": `function a(){return 123}console.log(a());
`,
		},
	})
}

func TestTSMinifiedBundleCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				const {foo} = require('./a')
				console.log(foo(), require('./j.json'))
			`,
			"/a.ts": `
				exports.foo = function() {
					return 123
				}
			`,
			"/j.json": `
				{"test": true}
			`,
		},
		entryPaths: []string{"/entry.ts"},
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
			"/out.js": `var c=e(b=>{b.foo=function(){return 123}});var d=e((b,a)=>{a.exports={test:!0}});const{foo:f}=c();console.log(f(),d());
`,
		},
	})
}

func TestTypeScriptDecorators(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import all from './all'
				import all_computed from './all_computed'
				import {a} from './a'
				import {b} from './b'
				import {c} from './c'
				import {d} from './d'
				import e from './e'
				import f from './f'
				import g from './g'
				import h from './h'
				import {i} from './i'
				import {j} from './j'
				import k from './k'
				console.log(all, all_computed, a, b, c, d, e, f, g, h, i, j, k)
			`,
			"/all.ts": `
				@x.y()
				@new y.x()
				export default class Foo {
					@x @y mUndef
					@x @y mDef = 1
					@x @y method(@x0 @y0 arg0, @x1 @y1 arg1) { return new Foo }
					@x @y static sUndef
					@x @y static sDef = new Foo
					@x @y static sMethod(@x0 @y0 arg0, @x1 @y1 arg1) { return new Foo }
				}
			`,
			"/all_computed.ts": `
				@x?.[_ + 'y']()
				@new y?.[_ + 'x']()
				export default class Foo {
					@x @y [mUndef()]
					@x @y [mDef()] = 1
					@x @y [method()](@x0 @y0 arg0, @x1 @y1 arg1) { return new Foo }

					// Side effect order must be preserved even for fields without decorators
					[xUndef()]
					[xDef()] = 2
					static [yUndef()]
					static [yDef()] = 3

					@x @y static [sUndef()]
					@x @y static [sDef()] = new Foo
					@x @y static [sMethod()](@x0 @y0 arg0, @x1 @y1 arg1) { return new Foo }
				}
			`,
			"/a.ts": `
				@x(() => 0) @y(() => 1)
				class a_class {
					fn() { return new a_class }
					static z = new a_class
				}
				export let a = a_class
			`,
			"/b.ts": `
				@x(() => 0) @y(() => 1)
				abstract class b_class {
					fn() { return new b_class }
					static z = new b_class
				}
				export let b = b_class
			`,
			"/c.ts": `
				@x(() => 0) @y(() => 1)
				export class c {
					fn() { return new c }
					static z = new c
				}
			`,
			"/d.ts": `
				@x(() => 0) @y(() => 1)
				export abstract class d {
					fn() { return new d }
					static z = new d
				}
			`,
			"/e.ts": `
				@x(() => 0) @y(() => 1)
				export default class {}
			`,
			"/f.ts": `
				@x(() => 0) @y(() => 1)
				export default class f {
					fn() { return new f }
					static z = new f
				}
			`,
			"/g.ts": `
				@x(() => 0) @y(() => 1)
				export default abstract class {}
			`,
			"/h.ts": `
				@x(() => 0) @y(() => 1)
				export default abstract class h {
					fn() { return new h }
					static z = new h
				}
			`,
			"/i.ts": `
				class i_class {
					@x(() => 0) @y(() => 1)
					foo
				}
				export let i = i_class
			`,
			"/j.ts": `
				export class j {
					@x(() => 0) @y(() => 1)
					foo() {}
				}
			`,
			"/k.ts": `
				export default class {
					foo(@x(() => 0) @y(() => 1) x) {}
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
			"/out.js": `// /all.ts
let Foo = class {
  constructor() {
    this.mDef = 1;
  }
  method(arg0, arg1) {
    return new Foo();
  }
  static sMethod(arg0, arg1) {
    return new Foo();
  }
};
Foo.sDef = new Foo();
__decorate([
  x,
  y
], Foo.prototype, "mUndef", 2);
__decorate([
  x,
  y
], Foo.prototype, "mDef", 2);
__decorate([
  x,
  y,
  __param(0, x0),
  __param(0, y0),
  __param(1, x1),
  __param(1, y1)
], Foo.prototype, "method", 1);
__decorate([
  x,
  y
], Foo.prototype, "sUndef", 2);
__decorate([
  x,
  y
], Foo.prototype, "sDef", 2);
__decorate([
  x,
  y,
  __param(0, x0),
  __param(0, y0),
  __param(1, x1),
  __param(1, y1)
], Foo.prototype, "sMethod", 1);
Foo = __decorate([
  x.y(),
  new y.x()
], Foo);
const all_default = Foo;

// /all_computed.ts
var _a, _b, _c, _d, _e, _f, _g, _h;
let Foo2 = class {
  constructor() {
    this[_b] = 1;
    this[_d] = 2;
  }
  [(_a = mUndef(), _b = mDef(), _c = method())](arg0, arg1) {
    return new Foo2();
  }
  static [(xUndef(), _d = xDef(), yUndef(), _e = yDef(), _f = sUndef(), _g = sDef(), _h = sMethod())](arg0, arg1) {
    return new Foo2();
  }
};
Foo2[_e] = 3;
Foo2[_g] = new Foo2();
__decorate([
  x,
  y
], Foo2.prototype, _a, 2);
__decorate([
  x,
  y
], Foo2.prototype, _b, 2);
__decorate([
  x,
  y,
  __param(0, x0),
  __param(0, y0),
  __param(1, x1),
  __param(1, y1)
], Foo2.prototype, _c, 1);
__decorate([
  x,
  y
], Foo2.prototype, _f, 2);
__decorate([
  x,
  y
], Foo2.prototype, _g, 2);
__decorate([
  x,
  y,
  __param(0, x0),
  __param(0, y0),
  __param(1, x1),
  __param(1, y1)
], Foo2.prototype, _h, 1);
Foo2 = __decorate([
  x?.[_ + "y"](),
  new y?.[_ + "x"]()
], Foo2);
const all_computed_default = Foo2;

// /a.ts
let a_class = class {
  fn() {
    return new a_class();
  }
};
a_class.z = new a_class();
a_class = __decorate([
  x(() => 0),
  y(() => 1)
], a_class);
let a = a_class;

// /b.ts
let b_class = class {
  fn() {
    return new b_class();
  }
};
b_class.z = new b_class();
b_class = __decorate([
  x(() => 0),
  y(() => 1)
], b_class);
let b = b_class;

// /c.ts
let c = class {
  fn() {
    return new c();
  }
};
c.z = new c();
c = __decorate([
  x(() => 0),
  y(() => 1)
], c);

// /d.ts
let d = class {
  fn() {
    return new d();
  }
};
d.z = new d();
d = __decorate([
  x(() => 0),
  y(() => 1)
], d);

// /e.ts
let e_default = class {
};
e_default = __decorate([
  x(() => 0),
  y(() => 1)
], e_default);
const e_default2 = e_default;

// /f.ts
let f2 = class {
  fn() {
    return new f2();
  }
};
f2.z = new f2();
f2 = __decorate([
  x(() => 0),
  y(() => 1)
], f2);
const f_default = f2;

// /g.ts
let g_default2 = class {
};
g_default2 = __decorate([
  x(() => 0),
  y(() => 1)
], g_default2);
const g_default = g_default2;

// /h.ts
let h2 = class {
  fn() {
    return new h2();
  }
};
h2.z = new h2();
h2 = __decorate([
  x(() => 0),
  y(() => 1)
], h2);
const h_default = h2;

// /i.ts
class i_class {
}
__decorate([
  x(() => 0),
  y(() => 1)
], i_class.prototype, "foo", 2);
let i2 = i_class;

// /j.ts
class j2 {
  foo() {
  }
}
__decorate([
  x(() => 0),
  y(() => 1)
], j2.prototype, "foo", 1);

// /k.ts
class k_default {
  foo(x2) {
  }
}
__decorate([
  __param(0, x2(() => 0)),
  __param(0, y(() => 1))
], k_default.prototype, "foo", 1);

// /entry.js
console.log(all_default, all_computed_default, a, b, c, d, e_default2, f_default, g_default, h_default, i2, j2, k_default);
`,
		},
	})
}
