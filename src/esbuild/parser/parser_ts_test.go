package parser

import (
	"esbuild/logging"
	"esbuild/printer"
	"testing"
)

func expectParseErrorTS(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log, join := logging.NewDeferLog()
		Parse(log, logging.Source{
			Index:        0,
			AbsolutePath: "<stdin>",
			PrettyPath:   "<stdin>",
			Contents:     contents,
		}, ParseOptions{
			TS: TypeScriptOptions{
				Parse: true,
			},
		})
		msgs := join()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
		}
		assertEqual(t, text, expected)
	})
}

func expectPrintedTS(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log, join := logging.NewDeferLog()
		ast, ok := Parse(log, logging.Source{
			Index:        0,
			AbsolutePath: "<stdin>",
			PrettyPath:   "<stdin>",
			Contents:     contents,
		}, ParseOptions{
			TS: TypeScriptOptions{
				Parse: true,
			},
		})
		msgs := join()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
		}
		assertEqual(t, text, "")
		if !ok {
			t.Fatal("Parse error")
		}
		js, _ := printer.Print(ast, printer.Options{})
		assertEqual(t, string(js), expected)
	})
}

func expectParseErrorTSX(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log, join := logging.NewDeferLog()
		Parse(log, logging.Source{
			Index:        0,
			AbsolutePath: "<stdin>",
			PrettyPath:   "<stdin>",
			Contents:     contents,
		}, ParseOptions{
			TS: TypeScriptOptions{
				Parse: true,
			},
			JSX: JSXOptions{
				Parse: true,
			},
		})
		msgs := join()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
		}
		assertEqual(t, text, expected)
	})
}

func expectPrintedTSX(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log, join := logging.NewDeferLog()
		ast, ok := Parse(log, logging.Source{
			Index:        0,
			AbsolutePath: "<stdin>",
			PrettyPath:   "<stdin>",
			Contents:     contents,
		}, ParseOptions{
			TS: TypeScriptOptions{
				Parse: true,
			},
			JSX: JSXOptions{
				Parse: true,
			},
		})
		msgs := join()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
		}
		assertEqual(t, text, "")
		if !ok {
			t.Fatal("Parse error")
		}
		js, _ := printer.Print(ast, printer.Options{})
		assertEqual(t, string(js), expected)
	})
}

func TestTSTypes(t *testing.T) {
	expectPrintedTS(t, "let x: T extends number\n ? T\n : number", "let x;\n")
	expectPrintedTS(t, "let x: (number | string)[]", "let x;\n")
	expectPrintedTS(t, "type x =\n | A\n | B\n C", "C;\n")
	expectPrintedTS(t, "type x = [-1, 0, 1]\n[]", "[];\n")
	expectPrintedTS(t, "type x = {0: number, readonly 1: boolean}\n[]", "[];\n")
	expectPrintedTS(t, "type x = {'a': number, readonly 'b': boolean}\n[]", "[];\n")
	expectPrintedTS(t, "let x: {x: 'a', y: false, z: null}", "let x;\n")
	expectPrintedTS(t, "let x: {foo(): void}", "let x;\n")
	expectPrintedTS(t, "let x: {['x']: number}", "let x;\n")
	expectPrintedTS(t, "let x: {['x'](): void}", "let x;\n")
	expectPrintedTS(t, "let x: {[key: string]: number}", "let x;\n")
	expectPrintedTS(t, "let x: () => void = () => {}", "let x = () => {\n};\n")
	expectPrintedTS(t, "let x: new () => void = Foo", "let x = Foo;\n")
	expectPrintedTS(t, "let x = 'x' as keyof T", "let x = \"x\";\n")
	expectPrintedTS(t, "let x = [1] as readonly [number]", "let x = [1];\n")
	expectPrintedTS(t, "let x = 'x' as keyof typeof Foo", "let x = \"x\";\n")
	expectPrintedTS(t, "let fs: typeof import('fs') = require('fs')", "let fs = require(\"fs\");\n")
	expectPrintedTS(t, "let x: <T>() => Foo<T>", "let x;\n")
	expectPrintedTS(t, "let x: new <T>() => Foo<T>", "let x;\n")
	expectPrintedTS(t, "let x: <T extends object>() => Foo<T>", "let x;\n")
	expectPrintedTS(t, "let x: new <T extends object>() => Foo<T>", "let x;\n")

	expectPrintedTS(t, "let x: A.B<X.Y>", "let x;\n")
	expectPrintedTS(t, "let x: A.B<X.Y>=2", "let x = 2;\n")
	expectPrintedTS(t, "let x: A.B<X.Y<Z>>", "let x;\n")
	expectPrintedTS(t, "let x: A.B<X.Y<Z>>=2", "let x = 2;\n")
	expectPrintedTS(t, "let x: A.B<X.Y<Z<T>>>", "let x;\n")
	expectPrintedTS(t, "let x: A.B<X.Y<Z<T>>>=2", "let x = 2;\n")
}

func TestTSClass(t *testing.T) {
	expectPrintedTS(t, "export default class Foo<T> {}", "export default class Foo {\n}\n")
	expectPrintedTS(t, "export default class Foo<T> extends Bar<T> {}", "export default class Foo extends Bar {\n}\n")
	expectPrintedTS(t, "(class Foo<T> {})", "(class Foo {\n});\n")
	expectPrintedTS(t, "(class Foo<T> extends Bar<T> {})", "(class Foo extends Bar {\n});\n")

	expectPrintedTS(t, "export default class <T> {}", "export default class {\n}\n")
	expectPrintedTS(t, "export default class <T> extends Foo<T> {}", "export default class extends Foo {\n}\n")
	expectPrintedTS(t, "(class <T> {})", "(class {\n});\n")
	expectPrintedTS(t, "(class <T> extends Foo<T> {})", "(class extends Foo {\n});\n")

	expectPrintedTS(t, "export abstract class A { abstract foo(): void; bar(): void {} }", "export class A {\n  bar() {\n  }\n}\n")
	expectPrintedTS(t, "abstract class A { abstract foo(): void; bar(): void {} }", "class A {\n  bar() {\n  }\n}\n")

	expectPrintedTS(t, "class A<T extends number> extends B.C<D, E> {}", "class A extends B.C {\n}\n")
	expectPrintedTS(t, "class A<T extends number> implements B.C<D, E>, F.G<H, I> {}", "class A {\n}\n")
	expectPrintedTS(t, "class A<T extends number> extends X implements B.C<D, E>, F.G<H, I> {}", "class A extends X {\n}\n")

	expectPrintedTS(t, "class Foo { constructor(public x) {} }", "class Foo {\n  constructor(x) {\n    this.x = x;\n  }\n}\n")
	expectPrintedTS(t, "class Foo { constructor(protected x) {} }", "class Foo {\n  constructor(x) {\n    this.x = x;\n  }\n}\n")
	expectPrintedTS(t, "class Foo { constructor(private x) {} }", "class Foo {\n  constructor(x) {\n    this.x = x;\n  }\n}\n")
	expectPrintedTS(t, "class Foo { constructor(readonly x) {} }", "class Foo {\n  constructor(x) {\n    this.x = x;\n  }\n}\n")
	expectPrintedTS(t, "class Foo { constructor(public readonly x) {} }", "class Foo {\n  constructor(x) {\n    this.x = x;\n  }\n}\n")
	expectPrintedTS(t, "class Foo { constructor(protected readonly x) {} }", "class Foo {\n  constructor(x) {\n    this.x = x;\n  }\n}\n")
	expectPrintedTS(t, "class Foo { constructor(private readonly x) {} }", "class Foo {\n  constructor(x) {\n    this.x = x;\n  }\n}\n")

	expectParseErrorTS(t, "class Foo { constructor(public {x}) {} }", "<stdin>: error: Expected identifier but found \"{\"\n")
	expectParseErrorTS(t, "class Foo { constructor(protected {x}) {} }", "<stdin>: error: Expected identifier but found \"{\"\n")
	expectParseErrorTS(t, "class Foo { constructor(private {x}) {} }", "<stdin>: error: Expected identifier but found \"{\"\n")
	expectParseErrorTS(t, "class Foo { constructor(readonly {x}) {} }", "<stdin>: error: Expected identifier but found \"{\"\n")

	expectParseErrorTS(t, "class Foo { constructor(public [x]) {} }", "<stdin>: error: Expected identifier but found \"[\"\n")
	expectParseErrorTS(t, "class Foo { constructor(protected [x]) {} }", "<stdin>: error: Expected identifier but found \"[\"\n")
	expectParseErrorTS(t, "class Foo { constructor(private [x]) {} }", "<stdin>: error: Expected identifier but found \"[\"\n")
	expectParseErrorTS(t, "class Foo { constructor(readonly [x]) {} }", "<stdin>: error: Expected identifier but found \"[\"\n")

	expectPrintedTS(t, "class Foo { foo: number }", "class Foo {\n  foo;\n}\n")
	expectPrintedTS(t, "class Foo { foo: number = 0 }", "class Foo {\n  foo = 0;\n}\n")
	expectPrintedTS(t, "class Foo { foo(): void {} }", "class Foo {\n  foo() {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { foo(): void; foo(): void {} }", "class Foo {\n  foo() {\n  }\n}\n")

	expectPrintedTS(t, "class Foo { foo?: number }", "class Foo {\n  foo;\n}\n")
	expectPrintedTS(t, "class Foo { foo?: number = 0 }", "class Foo {\n  foo = 0;\n}\n")
	expectPrintedTS(t, "class Foo { foo?(): void {} }", "class Foo {\n  foo() {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { foo?(): void; foo(): void {} }", "class Foo {\n  foo() {\n  }\n}\n")

	expectPrintedTS(t, "class Foo { foo!: number }", "class Foo {\n  foo;\n}\n")
	expectPrintedTS(t, "class Foo { foo!: number = 0 }", "class Foo {\n  foo = 0;\n}\n")
	expectPrintedTS(t, "class Foo { foo!(): void {} }", "class Foo {\n  foo() {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { foo!(): void; foo(): void {} }", "class Foo {\n  foo() {\n  }\n}\n")
}

func TestTSInterface(t *testing.T) {
	expectPrintedTS(t, "interface A<T extends number> extends B.C<D, E>, F.G<H, I> {}", "")
	expectPrintedTS(t, "export interface A<T extends number> extends B.C<D, E>, F.G<H, I> {}", "")
}

func TestTSNamespace(t *testing.T) {
	expectPrintedTS(t, "namespace Foo { 0 }", `var Foo;
(function(Foo) {
  0;
})(Foo || (Foo = {}));
`)
	expectPrintedTS(t, "export namespace Foo { 0 }", `export var Foo;
(function(Foo) {
  0;
})(Foo || (Foo = {}));
`)

	// Namespaces should introduce a scope that prevents name collisions
	expectPrintedTS(t, "namespace Foo { let x } let x", `var Foo;
(function(Foo) {
  let x;
})(Foo || (Foo = {}));
let x;
`)

	// Exports in namespaces shouldn't collide with module exports
	expectPrintedTS(t, "namespace Foo { export let x } export let x", `var Foo;
(function(Foo) {
})(Foo || (Foo = {}));
export let x;
`)
	expectPrintedTS(t, "declare namespace Foo { export let x } export let x", `export let x;
`)

	// Namespaces with values are not allowed to merge
	expectParseErrorTS(t, "var foo; namespace foo { 0 }", "<stdin>: error: \"foo\" has already been declared\n")
	expectParseErrorTS(t, "let foo; namespace foo { 0 }", "<stdin>: error: \"foo\" has already been declared\n")
	expectParseErrorTS(t, "const foo = 0; namespace foo { 0 }", "<stdin>: error: \"foo\" has already been declared\n")
	expectParseErrorTS(t, "namespace foo { 0 } var foo", "<stdin>: error: \"foo\" has already been declared\n")
	expectParseErrorTS(t, "namespace foo { 0 } let foo", "<stdin>: error: \"foo\" has already been declared\n")
	expectParseErrorTS(t, "namespace foo { 0 } const foo = 0", "<stdin>: error: \"foo\" has already been declared\n")

	// Namespaces without values are allowed to merge
	expectPrintedTS(t, "var foo; namespace foo {}", "var foo;\n")
	expectPrintedTS(t, "let foo; namespace foo {}", "let foo;\n")
	expectPrintedTS(t, "const foo = 0; namespace foo {}", "const foo = 0;\n")
	expectPrintedTS(t, "namespace foo {} var foo", "var foo;\n")
	expectPrintedTS(t, "namespace foo {} let foo", "let foo;\n")
	expectPrintedTS(t, "namespace foo {} const foo = 0", "const foo = 0;\n")

	// Namespaces with types but no values are allowed to merge
	expectPrintedTS(t, "var foo; namespace foo { export type bar = number }", "var foo;\n")
	expectPrintedTS(t, "let foo; namespace foo { export type bar = number }", "let foo;\n")
	expectPrintedTS(t, "const foo = 0; namespace foo { export type bar = number }", "const foo = 0;\n")
	expectPrintedTS(t, "namespace foo { export type bar = number } var foo", "var foo;\n")
	expectPrintedTS(t, "namespace foo { export type bar = number } let foo", "let foo;\n")
	expectPrintedTS(t, "namespace foo { export type bar = number } const foo = 0", "const foo = 0;\n")

	// Namespaces are allowed to merge with certain symbols
	expectPrintedTS(t, "function foo() {} namespace foo { 0 }", `function foo() {
}
(function(foo) {
  0;
})(foo || (foo = {}));
`)
	expectPrintedTS(t, "class foo {} namespace foo { 0 }", `class foo {
}
(function(foo) {
  0;
})(foo || (foo = {}));
`)
	expectPrintedTS(t, "enum foo { a } namespace foo { 0 }", `var foo;
(function(foo) {
  foo[foo["a"] = 0] = "a";
})(foo || (foo = {}));
(function(foo) {
  0;
})(foo || (foo = {}));
`)
	expectPrintedTS(t, "namespace foo {} namespace foo { 0 }", `var foo;
(function(foo) {
  0;
})(foo || (foo = {}));
`)
	expectParseErrorTS(t, "namespace foo { 0 } function foo() {}", "<stdin>: error: \"foo\" has already been declared\n")
	expectParseErrorTS(t, "namespace foo { 0 } class foo {}", "<stdin>: error: \"foo\" has already been declared\n")
	expectPrintedTS(t, "namespace foo { 0 } enum foo { a }", `var foo;
(function(foo) {
  0;
})(foo || (foo = {}));
(function(foo) {
  foo[foo["a"] = 0] = "a";
})(foo || (foo = {}));
`)
	expectPrintedTS(t, "namespace foo { 0 } namespace foo {}", `var foo;
(function(foo) {
  0;
})(foo || (foo = {}));
`)
	expectPrintedTS(t, "namespace foo { 0 } namespace foo { 0 }", `var foo;
(function(foo) {
  0;
})(foo || (foo = {}));
(function(foo) {
  0;
})(foo || (foo = {}));
`)
	expectPrintedTS(t, "function foo() {} namespace foo { 0 } function foo() {}", `function foo() {
}
(function(foo) {
  0;
})(foo || (foo = {}));
function foo() {
}
`)

	// Namespace merging shouldn't allow for other merging
	expectParseErrorTS(t, "class foo {} namespace foo { 0 } class foo {}", "<stdin>: error: \"foo\" has already been declared\n")
	expectParseErrorTS(t, "class foo {} namespace foo { 0 } enum foo {}", "<stdin>: error: \"foo\" has already been declared\n")
	expectParseErrorTS(t, "enum foo {} namespace foo { 0 } class foo {}", "<stdin>: error: \"foo\" has already been declared\n")
	expectParseErrorTS(t, "namespace foo { 0 } namespace foo { 0 } let foo", "<stdin>: error: \"foo\" has already been declared\n")
	expectParseErrorTS(t, "namespace foo { 0 } enum foo {} class foo {}", "<stdin>: error: \"foo\" has already been declared\n")

	// Test dot nested namespace syntax
	expectPrintedTS(t, "namespace foo.bar { foo(bar) }", `var foo;
(function(foo) {
  let bar;
  (function(bar) {
    foo(bar);
  })(bar = foo.bar || (foo.bar = {}));
})(foo || (foo = {}));
`)

	// "module" is a deprecated alias for "namespace"
	expectPrintedTS(t, "module foo { export namespace bar { foo(bar) } }", `var foo;
(function(foo) {
  let bar;
  (function(bar) {
    foo(bar);
  })(bar = foo.bar || (foo.bar = {}));
})(foo || (foo = {}));
`)
	expectPrintedTS(t, "namespace foo { export module bar { foo(bar) } }", `var foo;
(function(foo) {
  let bar;
  (function(bar) {
    foo(bar);
  })(bar = foo.bar || (foo.bar = {}));
})(foo || (foo = {}));
`)
	expectPrintedTS(t, "module foo.bar { foo(bar) }", `var foo;
(function(foo) {
  let bar;
  (function(bar) {
    foo(bar);
  })(bar = foo.bar || (foo.bar = {}));
})(foo || (foo = {}));
`)
}

func TestTSNamespaceExports(t *testing.T) {
	expectPrintedTS(t, `
		namespace A {
			export namespace B {
				export function fn() {}
			}
			namespace C {
				export function fn() {}
			}
			namespace D {
				function fn() {}
			}
		}
	`, `var A;
(function(A) {
  let B;
  (function(B) {
    function fn() {
    }
    B.fn = fn;
  })(B = A.B || (A.B = {}));
  let C;
  (function(C) {
    function fn() {
    }
    C.fn = fn;
  })(C || (C = {}));
  let D;
  (function(D) {
    function fn() {
    }
  })(D || (D = {}));
})(A || (A = {}));
`)

	expectPrintedTS(t, `
		namespace A {
			export namespace B {
				export class Class {}
			}
			namespace C {
				export class Class {}
			}
			namespace D {
				class Class {}
			}
		}
`, `var A;
(function(A) {
  let B;
  (function(B) {
    class Class {
    }
    B.Class = Class;
  })(B = A.B || (A.B = {}));
  let C;
  (function(C) {
    class Class {
    }
    C.Class = Class;
  })(C || (C = {}));
  let D;
  (function(D) {
    class Class {
    }
  })(D || (D = {}));
})(A || (A = {}));
`)

	expectPrintedTS(t, `
		namespace A {
			export namespace B {
				export enum Enum {}
			}
			namespace C {
				export enum Enum {}
			}
			namespace D {
				enum Enum {}
			}
		}
`, `var A;
(function(A) {
  let B;
  (function(B) {
    let Enum;
    (function(Enum) {
    })(Enum = B.Enum || (B.Enum = {}));
  })(B = A.B || (A.B = {}));
  let C;
  (function(C) {
    let Enum;
    (function(Enum) {
    })(Enum = C.Enum || (C.Enum = {}));
  })(C || (C = {}));
  let D;
  (function(D) {
    let Enum;
    (function(Enum) {
    })(Enum || (Enum = {}));
  })(D || (D = {}));
})(A || (A = {}));
`)

	expectPrintedTS(t, `
		namespace A {
			export namespace B {
				export let foo = 1
				foo += foo
			}
			namespace C {
				export let foo = 1
				foo += foo
			}
			namespace D {
				let foo = 1
				foo += foo
			}
		}
`, `var A;
(function(A) {
  let B;
  (function(B) {
    B.foo = 1;
    B.foo += B.foo;
  })(B = A.B || (A.B = {}));
  let C;
  (function(C) {
    C.foo = 1;
    C.foo += C.foo;
  })(C || (C = {}));
  let D;
  (function(D) {
    let foo = 1;
    foo += foo;
  })(D || (D = {}));
})(A || (A = {}));
`)

	expectPrintedTS(t, `
		namespace A {
			export namespace B {
				export const foo = 1
				foo += foo
			}
			namespace C {
				export const foo = 1
				foo += foo
			}
			namespace D {
				const foo = 1
				foo += foo
			}
		}
`, `var A;
(function(A) {
  let B;
  (function(B) {
    B.foo = 1;
    B.foo += B.foo;
  })(B = A.B || (A.B = {}));
  let C;
  (function(C) {
    C.foo = 1;
    C.foo += C.foo;
  })(C || (C = {}));
  let D;
  (function(D) {
    const foo = 1;
    foo += foo;
  })(D || (D = {}));
})(A || (A = {}));
`)

	expectPrintedTS(t, `
		namespace A {
			export namespace B {
				export var foo = 1
				foo += foo
			}
			namespace C {
				export var foo = 1
				foo += foo
			}
			namespace D {
				var foo = 1
				foo += foo
			}
		}
`, `var A;
(function(A) {
  let B;
  (function(B) {
    B.foo = 1;
    B.foo += B.foo;
  })(B = A.B || (A.B = {}));
  let C;
  (function(C) {
    C.foo = 1;
    C.foo += C.foo;
  })(C || (C = {}));
  let D;
  (function(D) {
    var foo = 1;
    foo += foo;
  })(D || (D = {}));
})(A || (A = {}));
`)

}

func TestTSNamespaceDestructuring(t *testing.T) {
	// Identifiers should be referenced directly
	expectPrintedTS(t, "namespace A { export var [a, b] = ref }", `var A;
(function(A) {
  A.a = ref[0], A.b = ref[1];
})(A || (A = {}));
`)

	// Other expressions should be saved (since they may have side effects)
	expectPrintedTS(t, "namespace A { export var [a, b] = ref.prop }", `var A;
(function(A) {
  var _a;
  _a = ref.prop, A.a = _a[0], A.b = _a[1];
})(A || (A = {}));
`)

	// Nested results used once should not be saved
	expectPrintedTS(t, "namespace A { export var [[[x]]] = ref }", `var A;
(function(A) {
  A.x = ref[0][0][0];
})(A || (A = {}));
`)
	expectPrintedTS(t, "namespace A { export var {x: {y: {z}}} = ref }", `var A;
(function(A) {
  A.z = ref.x.y.z;
})(A || (A = {}));
`)

	// Nested results used more than once should be saved
	expectPrintedTS(t, "namespace A { export var [[[x, y]]] = ref }", `var A;
(function(A) {
  var _a;
  _a = ref[0][0], A.x = _a[0], A.y = _a[1];
})(A || (A = {}));
`)
	expectPrintedTS(t, "namespace A { export var {x: {y: {z, w}}} = ref }", `var A;
(function(A) {
  var _a;
  _a = ref.x.y, A.z = _a.z, A.w = _a.w;
})(A || (A = {}));
`)

	// Values with side effects that appear to be used once but are actually used
	// zero times should still take effect
	expectPrintedTS(t, "namespace A { export var [[,]] = ref() }", `var A;
(function(A) {
  ref()[0];
})(A || (A = {}));
`)
	expectPrintedTS(t, "namespace A { export var {x: [,]} = ref() }", `var A;
(function(A) {
  ref().x;
})(A || (A = {}));
`)

	// Handle default values
	expectPrintedTS(t, "namespace A { export var [a = {}] = ref }", `var A;
(function(A) {
  var _a;
  _a = ref[0], A.a = _a === void 0 ? {} : _a;
})(A || (A = {}));
`)
	expectPrintedTS(t, "namespace A { export var {a = []} = ref }", `var A;
(function(A) {
  var _a;
  _a = ref.a, A.a = _a === void 0 ? [] : _a;
})(A || (A = {}));
`)
	expectPrintedTS(t, "namespace A { export var [[a, b] = {}] = ref }", `var A;
(function(A) {
  var _a, _b;
  _a = ref[0], _b = _a === void 0 ? {} : _a, A.a = _b[0], A.b = _b[1];
})(A || (A = {}));
`)
	expectPrintedTS(t, "namespace A { export var {a: {b, c} = []} = ref }", `var A;
(function(A) {
  var _a, _b;
  _a = ref.a, _b = _a === void 0 ? [] : _a, A.b = _b.b, A.c = _b.c;
})(A || (A = {}));
`)
}

func TestTSEnum(t *testing.T) {
	expectPrintedTS(t, "enum Foo { A, B }", `var Foo;
(function(Foo) {
  Foo[Foo["A"] = 0] = "A";
  Foo[Foo["B"] = 1] = "B";
})(Foo || (Foo = {}));
`)
	expectPrintedTS(t, "export enum Foo { A; B }", `export var Foo;
(function(Foo) {
  Foo[Foo["A"] = 0] = "A";
  Foo[Foo["B"] = 1] = "B";
})(Foo || (Foo = {}));
`)
	expectPrintedTS(t, "enum Foo { A, B, C = 3.3, D, E }", `var Foo;
(function(Foo) {
  Foo[Foo["A"] = 0] = "A";
  Foo[Foo["B"] = 1] = "B";
  Foo[Foo["C"] = 3.3] = "C";
  Foo[Foo["D"] = 4.3] = "D";
  Foo[Foo["E"] = 5.3] = "E";
})(Foo || (Foo = {}));
`)
	expectPrintedTS(t, "enum Foo { A, B, C = 'x', D, E }", `var Foo;
(function(Foo) {
  Foo[Foo["A"] = 0] = "A";
  Foo[Foo["B"] = 1] = "B";
  Foo[Foo["C"] = "x"] = "C";
  Foo[Foo["D"] = void 0] = "D";
  Foo[Foo["E"] = void 0] = "E";
})(Foo || (Foo = {}));
`)

	// TypeScript allows splitting an enum into multiple blocks
	expectPrintedTS(t, "enum Foo { A = 1 } enum Foo { B = 2 }", `var Foo;
(function(Foo) {
  Foo[Foo["A"] = 1] = "A";
})(Foo || (Foo = {}));
(function(Foo) {
  Foo[Foo["B"] = 2] = "B";
})(Foo || (Foo = {}));
`)

	expectPrintedTS(t, `
		enum Foo {
			'a' = 10.01,
			'a b' = 100,
			c = a + Foo.a + Foo['a b'],
			d,
			e = a + Foo.a + Foo['a b'] + Math.random(),
			f,
		}
		enum Bar {
			a = Foo.a
		}
	`, `var Foo;
(function(Foo) {
  Foo[Foo["a"] = 10.01] = "a";
  Foo[Foo["a b"] = 100] = "a b";
  Foo[Foo["c"] = 120.02] = "c";
  Foo[Foo["d"] = 121.02] = "d";
  Foo[Foo["e"] = 120.02 + Math.random()] = "e";
  Foo[Foo["f"] = void 0] = "f";
})(Foo || (Foo = {}));
var Bar;
(function(Bar) {
  Bar[Bar["a"] = 10.01] = "a";
})(Bar || (Bar = {}));
`)

}

func TestTSFunction(t *testing.T) {
	expectPrintedTS(t, "function foo(): void; function foo(): void {}", "function foo() {\n}\n")
}

func TestTSDecl(t *testing.T) {
	expectPrintedTS(t, "var a!: string, b!: boolean", "var a, b;\n")
	expectPrintedTS(t, "let a!: string, b!: boolean", "let a, b;\n")
	expectPrintedTS(t, "const a!: string = '', b!: boolean = false", "const a = \"\", b = false;\n")
	expectParseErrorTS(t, "var a!", "<stdin>: error: Expected \":\" but found end of file\n")
	expectParseErrorTS(t, "var a! = ", "<stdin>: error: Expected \":\" but found \"=\"\n")
	expectParseErrorTS(t, "var a!, b", "<stdin>: error: Expected \":\" but found \",\"\n")
}

func TestTSDeclare(t *testing.T) {
	expectPrintedTS(t, "declare var x: number", "")
	expectPrintedTS(t, "declare let x: number", "")
	expectPrintedTS(t, "declare const x: number", "")
	expectPrintedTS(t, "declare class X {}", "")
	expectPrintedTS(t, "declare interface X {}", "")
	expectPrintedTS(t, "declare namespace X {}", "")
	expectPrintedTS(t, "declare module X {}", "")
	expectPrintedTS(t, "declare module 'X' {}", "")
}

func TestTSDecorator(t *testing.T) {
	expectParseErrorTS(t, "class Foo { @Dec foo() {} @Dec bar() {} }",
		"<stdin>: error: Decorators are not supported yet\n"+
			"<stdin>: error: Decorators are not supported yet\n")
	expectParseErrorTS(t, "class Foo { foo(@Dec x, @Dec y) {} }",
		"<stdin>: error: Decorators are not supported yet\n"+
			"<stdin>: error: Decorators are not supported yet\n")

	expectParseErrorTS(t, "class Foo { @Dec(a(), b()) foo() {} @Dec bar() {} }",
		"<stdin>: error: Decorators are not supported yet\n"+
			"<stdin>: error: Decorators are not supported yet\n")
	expectParseErrorTS(t, "class Foo { foo(@Dec(a(), b()) x, @Dec y) {} }",
		"<stdin>: error: Decorators are not supported yet\n"+
			"<stdin>: error: Decorators are not supported yet\n")
}

func TestTSArrow(t *testing.T) {
	expectPrintedTS(t, "(a?) => {}", "(a) => {\n};\n")
	expectPrintedTS(t, "(a?: number) => {}", "(a) => {\n};\n")
	expectPrintedTS(t, "(a?: number = 0) => {}", "(a = 0) => {\n};\n")
	expectParseErrorTS(t, "(a? = 0) => {}", "<stdin>: error: Unexpected \"=\"\n")

	expectPrintedTS(t, "(a?, b) => {}", "(a, b) => {\n};\n")
	expectPrintedTS(t, "(a?: number, b) => {}", "(a, b) => {\n};\n")
	expectPrintedTS(t, "(a?: number = 0, b) => {}", "(a = 0, b) => {\n};\n")
	expectParseErrorTS(t, "(a? = 0, b) => {}", "<stdin>: error: Unexpected \"=\"\n")

	expectPrintedTS(t, "(a: number) => {}", "(a) => {\n};\n")
	expectPrintedTS(t, "(a: number = 0) => {}", "(a = 0) => {\n};\n")
	expectPrintedTS(t, "(a: number, b) => {}", "(a, b) => {\n};\n")

	expectPrintedTS(t, "(): void => {}", "() => {\n};\n")
	expectPrintedTS(t, "(a): void => {}", "(a) => {\n};\n")
	expectParseErrorTS(t, "x: void => {}", "<stdin>: error: Unexpected \"=>\"\n")
	expectPrintedTS(t, "a ? (1 + 2) : (3 + 4)", "a ? 1 + 2 : 3 + 4;\n")

	expectPrintedTS(t, "async (): void => {}", "async () => {\n};\n")
	expectPrintedTS(t, "async (a): void => {}", "async (a) => {\n};\n")
	expectParseErrorTS(t, "async x: void => {}", "<stdin>: error: Expected \"=>\" but found \":\"\n")

	expectPrintedTS(t, "(x: boolean): asserts x => {}", "(x) => {\n};\n")
	expectPrintedTS(t, "(x: any): x is number => {}", "(x) => {\n};\n")
}

func TestTSCall(t *testing.T) {
	expectPrintedTS(t, "foo()", "foo();\n")
	expectPrintedTS(t, "foo<number>()", "foo();\n")
	expectPrintedTS(t, "foo<number, boolean>()", "foo();\n")
}

func TestTSNew(t *testing.T) {
	expectPrintedTS(t, "new Foo()", "new Foo();\n")
	expectPrintedTS(t, "new Foo<number>()", "new Foo();\n")
	expectPrintedTS(t, "new Foo<number, boolean>()", "new Foo();\n")
}

func TestTSImport(t *testing.T) {
	expectPrintedTS(t, "import {x} from 'foo'", "")
	expectPrintedTS(t, "import {x} from 'foo'; log(x)", "import {x} from \"foo\";\nlog(x);\n")
	expectPrintedTS(t, "import {x, y as z} from 'foo'; log(z)", "import {y as z} from \"foo\";\nlog(z);\n")

	expectPrintedTS(t, "import x from 'foo'", "")
	expectPrintedTS(t, "import x from 'foo'; log(x)", "import x from \"foo\";\nlog(x);\n")

	expectPrintedTS(t, "import * as ns from 'foo'", "")
	expectPrintedTS(t, "import * as ns from 'foo'; log(ns)", "import * as ns from \"foo\";\nlog(ns);\n")

	// Dead control flow must not affect usage tracking
	expectPrintedTS(t, "import {x} from 'foo'; if (false) log(x)", "import \"foo\";\nif (false)\n  log(x);\n")
	expectPrintedTS(t, "import x from 'foo'; if (false) log(x)", "import \"foo\";\nif (false)\n  log(x);\n")
	expectPrintedTS(t, "import * as ns from 'foo'; if (false) log(ns)", "import \"foo\";\nif (false)\n  log(ns);\n")

	// This is TypeScript-specific syntax
	expectPrintedTS(t, "import x = require('foo'); x()", "const x = require(\"foo\");\nx();\n")
}

func TestTSJSX(t *testing.T) {
	expectPrintedTS(t, "const x = <number>1", "const x = 1;\n")
	expectPrintedTSX(t, "const x = <number>1</number>", "const x = React.createElement(\"number\", null, \"1\");\n")
	expectParseErrorTSX(t, "const x = <number>1", "<stdin>: error: Unexpected end of file\n")

	expectPrintedTSX(t, "const x = <Foo<T>></Foo>", "const x = React.createElement(Foo, null);\n")
	expectPrintedTSX(t, "const x = <Foo<T> data-foo></Foo>", "const x = React.createElement(Foo, {\n  \"data-foo\": true\n});\n")
	expectParseErrorTSX(t, "const x = <Foo<T>=>", "<stdin>: error: Expected \">\" but found \"=\"\n")

	expectPrintedTS(t, "const x = <T>() => {}", "const x = () => {\n};\n")
	expectPrintedTS(t, "const x = <T>(y)", "const x = y;\n")
	expectPrintedTS(t, "const x = <T>(y, z)", "const x = (y, z);\n")
	expectPrintedTS(t, "const x = <T>(y, z) => {}", "const x = (y, z) => {\n};\n")

	expectPrintedTS(t, "const x = <{}>() => {}", "const x = () => {\n};\n")
	expectPrintedTS(t, "const x = <{}>(y)", "const x = y;\n")
	expectPrintedTS(t, "const x = <{}>(y, z)", "const x = (y, z);\n")
	expectPrintedTS(t, "const x = <{}>(y, z) => {}", "const x = (y, z) => {\n};\n")

	expectPrintedTS(t, "const x = <[]>() => {}", "const x = () => {\n};\n")
	expectPrintedTS(t, "const x = <[]>(y)", "const x = y;\n")
	expectPrintedTS(t, "const x = <[]>(y, z)", "const x = (y, z);\n")
	expectPrintedTS(t, "const x = <[]>(y, z) => {}", "const x = (y, z) => {\n};\n")
}
