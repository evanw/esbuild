package bundler

import (
	"esbuild/parser"
	"testing"
)

func TestTSDeclareConst(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare const require: any
				declare const exports: any;
				declare const module: any

				declare const foo: any
				let foo
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
			"/out.js": `bootstrap({
  0() {
    // /entry.ts
    let foo;
  }
}, 0);
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
				let foo
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
			"/out.js": `bootstrap({
  0() {
    // /entry.ts
    let foo;
  }
}, 0);
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
				let foo
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
			"/out.js": `bootstrap({
  0() {
    // /entry.ts
    let foo;
  }
}, 0);
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
				let foo
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
			"/out.js": `bootstrap({
  0() {
    // /entry.ts
    ;
    let foo;
  }
}, 0);
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
				let foo
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
			"/out.js": `bootstrap({
  0() {
    // /entry.ts
    let foo;
  }
}, 0);
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
				let foo
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
			"/out.js": `bootstrap({
  0() {
    // /entry.ts
    ;
    let foo;
  }
}, 0);
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
				let foo
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
			"/out.js": `bootstrap({
  0() {
    // /entry.ts
    ;
    let foo;
  }
}, 0);
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
				let foo
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
			"/out.js": `bootstrap({
  0() {
    // /entry.ts
    ;
    let foo;
  }
}, 0);
`,
		},
	})
}

func TestTSImportEmptyNamespace(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {ns} from './ns.ts'
				console.log(ns)
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
			"/out.js": `bootstrap({
  0() {
    // /ns.ts

    // /entry.ts
    console.log(ns2);
  }
}, 0);
`,
		},
	})
}

func TestPackageImportMissingTS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import fn, {x as a, y as b} from './foo'
				console.log(fn(a, b))
			`,
			"/foo.js": `
				export const x = 132
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
			"/out.js": `bootstrap({
  0() {
    // /foo.js
    const x = 132;

    // /entry.ts
    console.log(fn(x, b));
  }
}, 0);
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
				let foo: SomeType1
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
			"/out.js": `bootstrap({
  0() {
    // /entry.ts
    let foo;
  }
}, 0);
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
			"/out.js": `bootstrap({
  1(exports, module) {
    // /b.ts
    function foo() {
    }
    module.exports = [123, foo];
  },

  0() {
    // /a.ts
    const b = __import(1 /* ./b.ts */);
    console.log(b.default);
  }
}, 0);
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
			"/out.js": `bootstrap({
  0() {
    // /b.ts
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
  }
}, 0);
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
			"/a.min.js": "var b;(function(a){a[a.A=0]=\"A\",a[a.B=1]=\"B\",a[a.C=a]=\"C\"})(b||(b={}));\n",
			"/b.min.js": "export var Foo;(function(a){a[a.X=0]=\"X\",a[a.Y=1]=\"Y\",a[a.Z=a]=\"Z\"})(Foo||(Foo={}));\n",
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
			"/a.min.js": "var b;(function(a){let c;(function(d){foo(a,d)})(c=a.Bar||(a.Bar={}))})(b||(b={}));\n",
			"/b.min.js": "export var Foo;(function(a){let b;(function(c){foo(a,c)})(b=a.Bar||(a.Bar={}))})(Foo||(Foo={}));\n",
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
			"/out.js": `bootstrap({
  0() {
    // /entry.ts
    let a;
    const b = 0;
    var c;
    function d() {
    }
    class e {
    }
    console.log(a, b, c, d, e);
  }
}, 0);
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
			"/out.js": `bootstrap({
  0() {
    // /other.ts
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
  }
}, 0);
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
			"/out.js": `bootstrap({1(){function a(){return 123}console.log(a())}},1);
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
			"/out.js": `bootstrap({0(a){a.foo=function(){return 123}},2(b,a){a.exports={test:!0}},1(){const{foo:b}=f$(0);console.log(b(),f$(2))}},1);
`,
		},
	})
}
