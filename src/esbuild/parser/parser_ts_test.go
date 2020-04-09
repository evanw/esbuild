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
  export let x;
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
}

func TestTSNamespaceExports(t *testing.T) {
	expectPrintedTS(t, `
		namespace A {
			export namespace B {
				export function fn() {}
				export class Class {}
			}
			namespace C {
				export function fn() {}
				export class Class {}
			}
		}
	`, `var A;
(function(A) {
  let B;
  (function(B) {
    function fn() {
    }
    B.fn = fn;
    class Class {
    }
    B.Class = Class;
  })(B = A.B || (A.B = {}));
  let C;
  (function(C) {
    function fn() {
    }
    C.fn = fn;
    class Class {
    }
    C.Class = Class;
  })(C || (C = {}));
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
}

func TestTSFunction(t *testing.T) {
	expectPrintedTS(t, "function foo(): void; function foo(): void {}", "function foo() {\n}\n")
}

func TestTSDeclare(t *testing.T) {
	expectPrintedTS(t, "declare var x: number", "")
	expectPrintedTS(t, "declare let x: number", "")
	expectPrintedTS(t, "declare const x: number", "")
	expectPrintedTS(t, "declare class X {}", "")
	expectPrintedTS(t, "declare interface X {}", "")
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
