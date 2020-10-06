package js_parser

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_printer"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/renamer"
	"github.com/evanw/esbuild/internal/test"
)

func expectParseErrorTS(t *testing.T, contents string, expected string) {
	t.Helper()
	t.Run(contents, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog()
		Parse(log, test.SourceForTest(contents), config.Options{
			TS: config.TSOptions{
				Parse: true,
			},
		})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logger.StderrOptions{}, logger.TerminalInfo{})
		}
		test.AssertEqual(t, text, expected)
	})
}

func expectPrintedTS(t *testing.T, contents string, expected string) {
	t.Helper()
	t.Run(contents, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog()
		tree, ok := Parse(log, test.SourceForTest(contents), config.Options{
			TS: config.TSOptions{
				Parse: true,
			},
		})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logger.StderrOptions{}, logger.TerminalInfo{})
		}
		test.AssertEqual(t, text, "")
		if !ok {
			t.Fatal("Parse error")
		}
		symbols := js_ast.NewSymbolMap(1)
		symbols.Outer[0] = tree.Symbols
		r := renamer.NewNoOpRenamer(symbols)
		js := js_printer.Print(tree, symbols, r, js_printer.PrintOptions{}).JS
		test.AssertEqual(t, string(js), expected)
	})
}

func expectParseErrorTSX(t *testing.T, contents string, expected string) {
	t.Helper()
	t.Run(contents, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog()
		Parse(log, test.SourceForTest(contents), config.Options{
			TS: config.TSOptions{
				Parse: true,
			},
			JSX: config.JSXOptions{
				Parse: true,
			},
		})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logger.StderrOptions{}, logger.TerminalInfo{})
		}
		test.AssertEqual(t, text, expected)
	})
}

func expectPrintedTSX(t *testing.T, contents string, expected string) {
	t.Helper()
	t.Run(contents, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog()
		tree, ok := Parse(log, test.SourceForTest(contents), config.Options{
			TS: config.TSOptions{
				Parse: true,
			},
			JSX: config.JSXOptions{
				Parse: true,
			},
		})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logger.StderrOptions{}, logger.TerminalInfo{})
		}
		test.AssertEqual(t, text, "")
		if !ok {
			t.Fatal("Parse error")
		}
		symbols := js_ast.NewSymbolMap(1)
		symbols.Outer[0] = tree.Symbols
		r := renamer.NewNoOpRenamer(symbols)
		js := js_printer.Print(tree, symbols, r, js_printer.PrintOptions{}).JS
		test.AssertEqual(t, string(js), expected)
	})
}

func TestTSTypes(t *testing.T) {
	expectPrintedTS(t, "let x: T extends number\n ? T\n : number", "let x;\n")
	expectPrintedTS(t, "let x: {y: T extends number ? T : number}", "let x;\n")
	expectPrintedTS(t, "let x: {y: T \n extends?: number}", "let x;\n")
	expectPrintedTS(t, "let x: (number | string)[]", "let x;\n")
	expectPrintedTS(t, "let x: [string[]?]", "let x;\n")
	expectPrintedTS(t, "let x: [number?, string?]", "let x;\n")
	expectPrintedTS(t, "let x: [a: number, b?: string, ...c: number[]]", "let x;\n")
	expectPrintedTS(t, "type x =\n A\n | B\n C", "C;\n")
	expectPrintedTS(t, "type x =\n | A\n | B\n C", "C;\n")
	expectPrintedTS(t, "type x =\n A\n & B\n C", "C;\n")
	expectPrintedTS(t, "type x =\n & A\n & B\n C", "C;\n")
	expectPrintedTS(t, "type x = [-1, 0, 1]\n[]", "[];\n")
	expectPrintedTS(t, "type x = [-1n, 0n, 1n]\n[]", "[];\n")
	expectPrintedTS(t, "type x = {0: number, readonly 1: boolean}\n[]", "[];\n")
	expectPrintedTS(t, "type x = {'a': number, readonly 'b': boolean}\n[]", "[];\n")
	expectPrintedTS(t, "type\nFoo = {}", "type;\nFoo = {};\n")
	expectParseErrorTS(t, "export type\nFoo = {}", "<stdin>: error: Unexpected newline after \"type\"\n")
	expectPrintedTS(t, "let x: {x: 'a', y: false, z: null}", "let x;\n")
	expectPrintedTS(t, "let x: {foo(): void}", "let x;\n")
	expectPrintedTS(t, "let x: {['x']: number}", "let x;\n")
	expectPrintedTS(t, "let x: {['x'](): void}", "let x;\n")
	expectPrintedTS(t, "let x: {[key: string]: number}", "let x;\n")
	expectPrintedTS(t, "let x: new () => void = Foo", "let x = Foo;\n")
	expectPrintedTS(t, "let x = 'x' as keyof T", "let x = \"x\";\n")
	expectPrintedTS(t, "let x = [1] as readonly [number]", "let x = [1];\n")
	expectPrintedTS(t, "let x = 'x' as keyof typeof Foo", "let x = \"x\";\n")
	expectPrintedTS(t, "let fs: typeof import('fs') = require('fs')", "let fs = require(\"fs\");\n")
	expectPrintedTS(t, "let fs: typeof import('fs').exists = require('fs').exists", "let fs = require(\"fs\").exists;\n")
	expectPrintedTS(t, "let x: <T>() => Foo<T>", "let x;\n")
	expectPrintedTS(t, "let x: new <T>() => Foo<T>", "let x;\n")
	expectPrintedTS(t, "let x: <T extends object>() => Foo<T>", "let x;\n")
	expectPrintedTS(t, "let x: new <T extends object>() => Foo<T>", "let x;\n")
	expectPrintedTS(t, "type Foo<T> = {[P in keyof T]?: T[P]}", "")
	expectPrintedTS(t, "type Foo<T> = {[P in keyof T]+?: T[P]}", "")
	expectPrintedTS(t, "type Foo<T> = {[P in keyof T]-?: T[P]}", "")
	expectPrintedTS(t, "type Foo<T> = {readonly [P in keyof T]: T[P]}", "")
	expectPrintedTS(t, "type Foo<T> = {-readonly [P in keyof T]: T[P]}", "")
	expectPrintedTS(t, "type Foo<T> = {+readonly [P in keyof T]: T[P]}", "")
	expectPrintedTS(t, "const x: unique symbol = y", "const x = y;\n")
	expectPrintedTS(t, "let x: typeof a = y", "let x = y;\n")
	expectPrintedTS(t, "let x: typeof a.b = y", "let x = y;\n")
	expectPrintedTS(t, "let x: typeof a.if = y", "let x = y;\n")
	expectPrintedTS(t, "let x: typeof if.a = y", "let x = y;\n")
	expectPrintedTS(t, "let x: typeof readonly = y", "let x = y;\n")
	expectParseErrorTS(t, "let x: typeof readonly Array", "<stdin>: error: Expected \";\" but found \"Array\"\n")
	expectPrintedTS(t, "let x: `y`", "let x;\n")
	expectParseErrorTS(t, "let x: tag`y`", "<stdin>: error: Expected \";\" but found \"`y`\"\n")

	expectPrintedTS(t, "let x: A.B<X.Y>", "let x;\n")
	expectPrintedTS(t, "let x: A.B<X.Y>=2", "let x = 2;\n")
	expectPrintedTS(t, "let x: A.B<X.Y<Z>>", "let x;\n")
	expectPrintedTS(t, "let x: A.B<X.Y<Z>>=2", "let x = 2;\n")
	expectPrintedTS(t, "let x: A.B<X.Y<Z<T>>>", "let x;\n")
	expectPrintedTS(t, "let x: A.B<X.Y<Z<T>>>=2", "let x = 2;\n")

	expectPrintedTS(t, "let foo: any\n<x>y", "let foo;\ny;\n")
	expectPrintedTSX(t, "let foo: any\n<x>y</x>", "let foo;\n/* @__PURE__ */ React.createElement(\"x\", null, \"y\");\n")
	expectParseErrorTS(t, "let foo: (any\n<x>y)", "<stdin>: error: Expected \")\" but found \"<\"\n")

	expectPrintedTS(t, "let foo = bar as (null)", "let foo = bar;\n")
	expectPrintedTS(t, "let foo = bar\nas (null)", "let foo = bar;\nas(null);\n")
	expectParseErrorTS(t, "let foo = (bar\nas (null))", "<stdin>: error: Expected \")\" but found \"as\"\n")

	expectPrintedTS(t, "let foo: keyof Object = 'toString'", "let foo = \"toString\";\n")
	expectPrintedTS(t, "let foo: keyof\nObject = 'toString'", "let foo = \"toString\";\n")
	expectPrintedTS(t, "let foo: (keyof\nObject) = 'toString'", "let foo = \"toString\";\n")

	expectPrintedTS(t, "type Foo = Array<<T>(x: T) => T>\n x", "x;\n")
	expectPrintedTSX(t, "<Foo<<T>(x: T) => T>/>", "/* @__PURE__ */ React.createElement(Foo, null);\n")

	// TypeScript 4.1
	expectPrintedTS(t, "let foo: `${'a' | 'b'}-${'c' | 'd'}` = 'a-c'", "let foo = \"a-c\";\n")
}

func TestTSClass(t *testing.T) {
	expectPrintedTS(t, "export default class Foo {}", "export default class Foo {\n}\n")
	expectPrintedTS(t, "export default class Foo extends Bar<T> {}", "export default class Foo extends Bar {\n}\n")
	expectPrintedTS(t, "export default class Foo extends Bar<T>() {}", "export default class Foo extends Bar() {\n}\n")
	expectPrintedTS(t, "export default class Foo implements Bar<T> {}", "export default class Foo {\n}\n")
	expectPrintedTS(t, "export default class Foo<T> {}", "export default class Foo {\n}\n")
	expectPrintedTS(t, "export default class Foo<T> extends Bar<T> {}", "export default class Foo extends Bar {\n}\n")
	expectPrintedTS(t, "export default class Foo<T> extends Bar<T>() {}", "export default class Foo extends Bar() {\n}\n")
	expectPrintedTS(t, "export default class Foo<T> implements Bar<T> {}", "export default class Foo {\n}\n")
	expectPrintedTS(t, "(class Foo<T> {})", "(class Foo {\n});\n")
	expectPrintedTS(t, "(class Foo<T> extends Bar<T> {})", "(class Foo extends Bar {\n});\n")
	expectPrintedTS(t, "(class Foo<T> extends Bar<T>() {})", "(class Foo extends Bar() {\n});\n")
	expectPrintedTS(t, "(class Foo<T> implements Bar<T> {})", "(class Foo {\n});\n")

	expectPrintedTS(t, "export default class {}", "export default class {\n}\n")
	expectPrintedTS(t, "export default class extends Foo<T> {}", "export default class extends Foo {\n}\n")
	expectPrintedTS(t, "export default class implements Foo<T> {}", "export default class {\n}\n")
	expectPrintedTS(t, "export default class <T> {}", "export default class {\n}\n")
	expectPrintedTS(t, "export default class <T> extends Foo<T> {}", "export default class extends Foo {\n}\n")
	expectPrintedTS(t, "export default class <T> implements Foo<T> {}", "export default class {\n}\n")
	expectPrintedTS(t, "(class <T> {})", "(class {\n});\n")
	expectPrintedTS(t, "(class extends Foo<T> {})", "(class extends Foo {\n});\n")
	expectPrintedTS(t, "(class extends Foo<T>() {})", "(class extends Foo() {\n});\n")
	expectPrintedTS(t, "(class implements Foo<T> {})", "(class {\n});\n")
	expectPrintedTS(t, "(class <T> extends Foo<T> {})", "(class extends Foo {\n});\n")
	expectPrintedTS(t, "(class <T> extends Foo<T>() {})", "(class extends Foo() {\n});\n")
	expectPrintedTS(t, "(class <T> implements Foo<T> {})", "(class {\n});\n")

	expectPrintedTS(t, "abstract class A { abstract foo(): void; bar(): void {} }", "class A {\n  bar() {\n  }\n}\n")
	expectPrintedTS(t, "export abstract class A { abstract foo(): void; bar(): void {} }", "export class A {\n  bar() {\n  }\n}\n")
	expectPrintedTS(t, "export default abstract", "export default abstract;\n")
	expectPrintedTS(t, "export default abstract - after", "export default abstract - after;\n")
	expectPrintedTS(t, "export default abstract class { abstract foo(): void; bar(): void {} } - after", "export default class {\n  bar() {\n  }\n}\n-after;\n")
	expectPrintedTS(t, "export default abstract class A { abstract foo(): void; bar(): void {} } - after", "export default class A {\n  bar() {\n  }\n}\n-after;\n")

	expectPrintedTS(t, "class A<T extends number> extends B.C<D, E> {}", "class A extends B.C {\n}\n")
	expectPrintedTS(t, "class A<T extends number> implements B.C<D, E>, F.G<H, I> {}", "class A {\n}\n")
	expectPrintedTS(t, "class A<T extends number> extends X implements B.C<D, E>, F.G<H, I> {}", "class A extends X {\n}\n")

	expectPrintedTS(t, "class Foo { constructor(public) {} }", "class Foo {\n  constructor(public) {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { constructor(protected) {} }", "class Foo {\n  constructor(protected) {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { constructor(private) {} }", "class Foo {\n  constructor(private) {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { constructor(readonly) {} }", "class Foo {\n  constructor(readonly) {\n  }\n}\n")
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

	expectPrintedTS(t, "class Foo { foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { foo: number = 0 }", "class Foo {\n  constructor() {\n    this.foo = 0;\n  }\n}\n")
	expectPrintedTS(t, "class Foo { foo(): void {} }", "class Foo {\n  foo() {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { foo(): void; foo(): void {} }", "class Foo {\n  foo() {\n  }\n}\n")
	expectParseErrorTS(t, "class Foo { foo(): void foo(): void {} }", "<stdin>: error: Expected \";\" but found \"foo\"\n")

	expectPrintedTS(t, "class Foo { foo?: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { foo?: number = 0 }", "class Foo {\n  constructor() {\n    this.foo = 0;\n  }\n}\n")
	expectPrintedTS(t, "class Foo { foo?(): void {} }", "class Foo {\n  foo() {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { foo?(): void; foo(): void {} }", "class Foo {\n  foo() {\n  }\n}\n")
	expectParseErrorTS(t, "class Foo { foo?(): void foo(): void {} }", "<stdin>: error: Expected \";\" but found \"foo\"\n")

	expectPrintedTS(t, "class Foo { foo!: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { foo!: number = 0 }", "class Foo {\n  constructor() {\n    this.foo = 0;\n  }\n}\n")
	expectPrintedTS(t, "class Foo { foo!(): void {} }", "class Foo {\n  foo() {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { foo!(): void; foo(): void {} }", "class Foo {\n  foo() {\n  }\n}\n")
	expectParseErrorTS(t, "class Foo { foo!(): void foo(): void {} }", "<stdin>: error: Expected \";\" but found \"foo\"\n")

	expectPrintedTS(t, "class Foo { public foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { private foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { protected foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { declare foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { declare public foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { public declare foo: number }", "class Foo {\n}\n")

	expectPrintedTS(t, "class Foo { public static foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { private static foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { protected static foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { declare static foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { declare public static foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { public declare static foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { public static declare foo: number }", "class Foo {\n}\n")

	expectPrintedTS(t, "class Foo { [key: string]: any\nfoo = 0 }", "class Foo {\n  constructor() {\n    this.foo = 0;\n  }\n}\n")
	expectPrintedTS(t, "class Foo { [key: string]: any; foo = 0 }", "class Foo {\n  constructor() {\n    this.foo = 0;\n  }\n}\n")

	expectParseErrorTS(t, "class Foo<> {}", "<stdin>: error: Expected identifier but found \">\"\n")
	expectParseErrorTS(t, "class Foo<,> {}", "<stdin>: error: Expected identifier but found \",\"\n")
	expectParseErrorTS(t, "class Foo<T><T> {}", "<stdin>: error: Expected \"{\" but found \"<\"\n")
}

func TestTSPrivateIdentifiers(t *testing.T) {
	// The TypeScript compiler still moves private field initializers into the
	// constructor, but it has to leave the private field declaration in place so
	// the private field is still declared.
	expectPrintedTS(t, "class Foo { #foo }", "class Foo {\n  #foo;\n}\n")
	expectPrintedTS(t, "class Foo { #foo = 1 }", "class Foo {\n  constructor() {\n    this.#foo = 1;\n  }\n  #foo;\n}\n")
	expectPrintedTS(t, "class Foo { #foo() {} }", "class Foo {\n  #foo() {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { get #foo() {} }", "class Foo {\n  get #foo() {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { set #foo(x) {} }", "class Foo {\n  set #foo(x) {\n  }\n}\n")

	// The TypeScript compiler doesn't currently support static private fields
	// because it moves static field initializers to after the class body and
	// private fields can't be used outside the class body. It remains to be seen
	// how the TypeScript compiler will transform private static fields once it
	// finally does support them. For now just leave the initializer in place.
	expectPrintedTS(t, "class Foo { static #foo }", "class Foo {\n  static #foo;\n}\n")
	expectPrintedTS(t, "class Foo { static #foo = 1 }", "class Foo {\n  static #foo = 1;\n}\n")
	expectPrintedTS(t, "class Foo { static #foo() {} }", "class Foo {\n  static #foo() {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { static get #foo() {} }", "class Foo {\n  static get #foo() {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { static set #foo(x) {} }", "class Foo {\n  static set #foo(x) {\n  }\n}\n")
}

func TestTSInterface(t *testing.T) {
	expectPrintedTS(t, "interface A { a } x", "x;\n")
	expectPrintedTS(t, "interface A { a; b } x", "x;\n")
	expectPrintedTS(t, "interface A { a() } x", "x;\n")
	expectPrintedTS(t, "interface A { a(); b } x", "x;\n")
	expectPrintedTS(t, "interface Foo { foo(): Foo \n is: Bar } x", "x;\n")
	expectPrintedTS(t, "interface A<T extends number> extends B.C<D, E>, F.G<H, I> {} x", "x;\n")
	expectPrintedTS(t, "export interface A<T extends number> extends B.C<D, E>, F.G<H, I> {} x", "x;\n")
	expectPrintedTS(t, "export default interface Foo {} x", "x;\n")
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
	expectPrintedTS(t, "declare namespace Foo { export let x } namespace x { 0 }", `var x;
(function(x) {
  0;
})(x || (x = {}));
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
	expectPrintedTS(t, "function* foo() {} namespace foo { 0 }", `function* foo() {
}
(function(foo) {
  0;
})(foo || (foo = {}));
`)
	expectPrintedTS(t, "async function foo() {} namespace foo { 0 }", `async function foo() {
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
	expectParseErrorTS(t, "namespace foo { 0 } function* foo() {}", "<stdin>: error: \"foo\" has already been declared\n")
	expectParseErrorTS(t, "namespace foo { 0 } async function foo() {}", "<stdin>: error: \"foo\" has already been declared\n")
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
	expectPrintedTS(t, "function* foo() {} namespace foo { 0 } function* foo() {}", `function* foo() {
}
(function(foo) {
  0;
})(foo || (foo = {}));
function* foo() {
}
`)
	expectPrintedTS(t, "async function foo() {} namespace foo { 0 } async function foo() {}", `async function foo() {
}
(function(foo) {
  0;
})(foo || (foo = {}));
async function foo() {
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

	expectPrintedTS(t, `
		namespace ns {
			export declare const L1
			console.log(L1)

			export declare let [[L2 = x, { [y]: L3 }]]
			console.log(L2, L3)

			export declare function F()
			console.log(F)

			export declare function F2() { }
			console.log(F2)

			export declare class C { }
			console.log(C)

			export declare enum E { }
			console.log(E)

			export declare namespace N { }
			console.log(N)
		}
	`, `var ns;
(function(ns) {
  console.log(ns.L1);
  console.log(ns.L2, ns.L3);
  console.log(F);
  console.log(F2);
  console.log(C);
  console.log(E);
  console.log(N);
})(ns || (ns = {}));
`)
}

func TestTSNamespaceDestructuring(t *testing.T) {
	expectPrintedTS(t, `
		namespace A {
			export var [
				a,
				[, b = c, ...d],
				{[x]: [[y]] = z, ...o},
			] = ref
		}
	`, `var A;
(function(A) {
  [
    A.a,
    [, A.b = c, ...A.d],
    {[x]: [[A.y]] = z, ...A.o}
  ] = ref;
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
	expectPrintedTS(t, "enum Foo { A, B, C = 'x', D, E, F = `y`, G = `${z}`, H = tag`` }", `var Foo;
(function(Foo) {
  Foo[Foo["A"] = 0] = "A";
  Foo[Foo["B"] = 1] = "B";
  Foo["C"] = "x";
  Foo[Foo["D"] = void 0] = "D";
  Foo[Foo["E"] = void 0] = "E";
  Foo["F"] = `+"`y`"+`;
  Foo[Foo["G"] = `+"`${z}`"+`] = "G";
  Foo[Foo["H"] = tag`+"``"+`] = "H";
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

	expectPrintedTS(t, `
		enum Foo { A }
		x = [Foo.A, Foo?.A, Foo?.A()]
		y = [Foo['A'], Foo?.['A'], Foo?.['A']()]
	`, `var Foo;
(function(Foo) {
  Foo[Foo["A"] = 0] = "A";
})(Foo || (Foo = {}));
x = [0, Foo?.A, Foo?.A()];
y = [0, Foo?.["A"], Foo?.["A"]()];
`)
}

func TestTSEnumConstantFolding(t *testing.T) {
	expectPrintedTS(t, `
		enum Foo {
			add = 1 + 2,
			sub = -1 - 2,
			mul = 10 * 20,

			div_pos_inf = 1 / 0,
			div_neg_inf = 1 / -0,
			div_nan = 0 / 0,
			div_neg_zero = 1 / (1 / -0),

			div0 = 10 / 20,
			div1 = 10 / -20,
			div2 = -10 / 20,
			div3 = -10 / -20,

			mod0 = 123 % 100,
			mod1 = 123 % -100,
			mod2 = -123 % 100,
			mod3 = -123 % -100,

			fmod0 = 1.375 % 0.75,
			fmod1 = 1.375 % -0.75,
			fmod2 = -1.375 % 0.75,
			fmod3 = -1.375 % -0.75,

			pow0 = 2.25 ** 3,
			pow1 = 2.25 ** -3,
			pow2 = (-2.25) ** 3,
			pow3 = (-2.25) ** -3,
		}
	`, `var Foo;
(function(Foo) {
  Foo[Foo["add"] = 3] = "add";
  Foo[Foo["sub"] = -3] = "sub";
  Foo[Foo["mul"] = 200] = "mul";
  Foo[Foo["div_pos_inf"] = Infinity] = "div_pos_inf";
  Foo[Foo["div_neg_inf"] = -Infinity] = "div_neg_inf";
  Foo[Foo["div_nan"] = NaN] = "div_nan";
  Foo[Foo["div_neg_zero"] = -0] = "div_neg_zero";
  Foo[Foo["div0"] = 0.5] = "div0";
  Foo[Foo["div1"] = -0.5] = "div1";
  Foo[Foo["div2"] = -0.5] = "div2";
  Foo[Foo["div3"] = 0.5] = "div3";
  Foo[Foo["mod0"] = 23] = "mod0";
  Foo[Foo["mod1"] = 23] = "mod1";
  Foo[Foo["mod2"] = -23] = "mod2";
  Foo[Foo["mod3"] = -23] = "mod3";
  Foo[Foo["fmod0"] = 0.625] = "fmod0";
  Foo[Foo["fmod1"] = 0.625] = "fmod1";
  Foo[Foo["fmod2"] = -0.625] = "fmod2";
  Foo[Foo["fmod3"] = -0.625] = "fmod3";
  Foo[Foo["pow0"] = 11.390625] = "pow0";
  Foo[Foo["pow1"] = 0.0877914951989026] = "pow1";
  Foo[Foo["pow2"] = -11.390625] = "pow2";
  Foo[Foo["pow3"] = -0.0877914951989026] = "pow3";
})(Foo || (Foo = {}));
`)

	expectPrintedTS(t, `
		enum Foo {
			shl0 = 987654321 << 2,
			shl1 = 987654321 << 31,
			shl2 = 987654321 << 34,

			shr0 = -987654321 >> 2,
			shr1 = -987654321 >> 31,
			shr2 = -987654321 >> 34,

			ushr0 = -987654321 >>> 2,
			ushr1 = -987654321 >>> 31,
			ushr2 = -987654321 >>> 34,

			bitand = 0xDEADF00D & 0xBADCAFE,
			bitor = 0xDEADF00D | 0xBADCAFE,
			bitxor = 0xDEADF00D ^ 0xBADCAFE,
		}
	`, `var Foo;
(function(Foo) {
  Foo[Foo["shl0"] = -344350012] = "shl0";
  Foo[Foo["shl1"] = -2147483648] = "shl1";
  Foo[Foo["shl2"] = -344350012] = "shl2";
  Foo[Foo["shr0"] = -246913581] = "shr0";
  Foo[Foo["shr1"] = -1] = "shr1";
  Foo[Foo["shr2"] = -246913581] = "shr2";
  Foo[Foo["ushr0"] = 826828243] = "ushr0";
  Foo[Foo["ushr1"] = 1] = "ushr1";
  Foo[Foo["ushr2"] = 826828243] = "ushr2";
  Foo[Foo["bitand"] = 179159052] = "bitand";
  Foo[Foo["bitor"] = -542246145] = "bitor";
  Foo[Foo["bitxor"] = -721405197] = "bitxor";
})(Foo || (Foo = {}));
`)
}

func TestTSFunction(t *testing.T) {
	expectPrintedTS(t, "function foo(): void; function foo(): void {}", "function foo() {\n}\n")

	expectPrintedTS(t, "function foo<A>() {}", "function foo() {\n}\n")
	expectPrintedTS(t, "function foo<A extends B<A>>() {}", "function foo() {\n}\n")
	expectPrintedTS(t, "function foo<A extends B<C<A>>>() {}", "function foo() {\n}\n")
	expectPrintedTS(t, "function foo<A,B,C,>() {}", "function foo() {\n}\n")
	expectPrintedTS(t, "function foo<A extends B<C>= B<C>>() {}", "function foo() {\n}\n")
	expectPrintedTS(t, "function foo<A extends B<C<D>>= B<C<D>>>() {}", "function foo() {\n}\n")
	expectPrintedTS(t, "function foo<A extends B<C<D<E>>>= B<C<D<E>>>>() {}", "function foo() {\n}\n")

	expectParseErrorTS(t, "function foo<>() {}", "<stdin>: error: Expected identifier but found \">\"\n")
	expectParseErrorTS(t, "function foo<,>() {}", "<stdin>: error: Expected identifier but found \",\"\n")
	expectParseErrorTS(t, "function foo<T><T>() {}", "<stdin>: error: Expected \"(\" but found \"<\"\n")

	expectPrintedTS(t, "export default function <T>() {}", "export default function() {\n}\n")
	expectParseErrorTS(t, "export default function <>() {}", "<stdin>: error: Expected identifier but found \">\"\n")
	expectParseErrorTS(t, "export default function <,>() {}", "<stdin>: error: Expected identifier but found \",\"\n")
	expectParseErrorTS(t, "export default function <T><T>() {}", "<stdin>: error: Expected \"(\" but found \"<\"\n")

	expectPrintedTS(t, `
		export default function foo();
		export default function foo(x);
		export default function foo(x?, y?) {}
	`, "export default function foo(x, y) {\n}\n")
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
	expectPrintedTS(t, "declare function fn(); function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare function fn()\n function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare function fn() {} function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare enum X {} function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare class X {} function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare interface X {} function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare namespace X {} function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare module X {} function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare module 'X' {} function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare module 'X'; let foo", "let foo;\n")
	expectPrintedTS(t, "declare module 'X'\nlet foo", "let foo;\n")
	expectPrintedTS(t, "declare module 'X' { let foo }", "")
	expectPrintedTS(t, "declare module 'X'\n{ let foo }", "")
	expectPrintedTS(t, "declare global { interface Foo {} let foo: any } let bar", "let bar;\n")
	expectPrintedTS(t, "declare module M { const x }", "")
	expectParseErrorTS(t, "module M { const x }", "<stdin>: error: This constant must be initialized\n")
}

func TestTSDecorator(t *testing.T) {
	// Tests of "declare class"
	expectPrintedTS(t, "@dec(() => 0) declare class Foo {} {let foo}", "{\n  let foo;\n}\n")
	expectPrintedTS(t, "@dec(() => 0) declare abstract class Foo {} {let foo}", "{\n  let foo;\n}\n")
	expectPrintedTS(t, "@dec(() => 0) export declare class Foo {} {let foo}", "{\n  let foo;\n}\n")
	expectPrintedTS(t, "@dec(() => 0) export declare abstract class Foo {} {let foo}", "{\n  let foo;\n}\n")
	expectPrintedTS(t, "declare class Foo { @dec(() => 0) foo } {let foo}", "{\n  let foo;\n}\n")
	expectPrintedTS(t, "declare class Foo { @dec(() => 0) foo() } {let foo}", "{\n  let foo;\n}\n")
	expectPrintedTS(t, "declare class Foo { foo(@dec(() => 0) x) } {let foo}", "{\n  let foo;\n}\n")

	// Decorators must only work on class statements
	expectParseErrorTS(t, "@dec enum foo {}", "<stdin>: error: Expected \"class\" but found \"enum\"\n")
	expectParseErrorTS(t, "@dec namespace foo {}", "<stdin>: error: Expected \"class\" but found \"namespace\"\n")
	expectParseErrorTS(t, "@dec function foo() {}", "<stdin>: error: Expected \"class\" but found \"function\"\n")
	expectParseErrorTS(t, "@dec abstract", "<stdin>: error: Expected \"class\" but found end of file\n")
	expectParseErrorTS(t, "@dec declare: x", "<stdin>: error: Expected \"class\" but found \":\"\n")
	expectParseErrorTS(t, "@dec declare enum foo {}", "<stdin>: error: Expected \"class\" but found \"enum\"\n")
	expectParseErrorTS(t, "@dec declare namespace foo {}", "<stdin>: error: Expected \"class\" but found \"namespace\"\n")
	expectParseErrorTS(t, "@dec declare function foo()", "<stdin>: error: Expected \"class\" but found \"function\"\n")
	expectParseErrorTS(t, "@dec export {}", "<stdin>: error: Expected \"class\" but found \"{\"\n")
	expectParseErrorTS(t, "@dec export enum foo {}", "<stdin>: error: Expected \"class\" but found \"enum\"\n")
	expectParseErrorTS(t, "@dec export namespace foo {}", "<stdin>: error: Expected \"class\" but found \"namespace\"\n")
	expectParseErrorTS(t, "@dec export function foo() {}", "<stdin>: error: Expected \"class\" but found \"function\"\n")
	expectParseErrorTS(t, "@dec export default abstract", "<stdin>: error: Expected \"class\" but found end of file\n")
	expectParseErrorTS(t, "@dec export declare enum foo {}", "<stdin>: error: Expected \"class\" but found \"enum\"\n")
	expectParseErrorTS(t, "@dec export declare namespace foo {}", "<stdin>: error: Expected \"class\" but found \"namespace\"\n")
	expectParseErrorTS(t, "@dec export declare function foo()", "<stdin>: error: Expected \"class\" but found \"function\"\n")

	// Decorators must be forbidden outside class statements
	expectParseErrorTS(t, "(class { @dec foo })", "<stdin>: error: Expected identifier but found \"@\"\n")
	expectParseErrorTS(t, "(class { @dec foo() {} })", "<stdin>: error: Expected identifier but found \"@\"\n")
	expectParseErrorTS(t, "(class { foo(@dec x) {} })", "<stdin>: error: Expected identifier but found \"@\"\n")
	expectParseErrorTS(t, "({ @dec foo })", "<stdin>: error: Expected identifier but found \"@\"\n")
	expectParseErrorTS(t, "({ @dec foo() {} })", "<stdin>: error: Expected identifier but found \"@\"\n")
	expectParseErrorTS(t, "({ foo(@dec x) {} })", "<stdin>: error: Expected identifier but found \"@\"\n")

	// Decorators aren't allowed with private names
	expectParseErrorTS(t, "class Foo { @dec #foo }", "<stdin>: error: Expected identifier but found \"#foo\"\n")
	expectParseErrorTS(t, "class Foo { @dec #foo = 1 }", "<stdin>: error: Expected identifier but found \"#foo\"\n")
	expectParseErrorTS(t, "class Foo { @dec #foo() {} }", "<stdin>: error: Expected identifier but found \"#foo\"\n")
	expectParseErrorTS(t, "class Foo { @dec *#foo() {} }", "<stdin>: error: Expected identifier but found \"#foo\"\n")
	expectParseErrorTS(t, "class Foo { @dec async #foo() {} }", "<stdin>: error: Expected identifier but found \"#foo\"\n")
	expectParseErrorTS(t, "class Foo { @dec async* #foo() {} }", "<stdin>: error: Expected identifier but found \"#foo\"\n")
	expectParseErrorTS(t, "class Foo { @dec static #foo }", "<stdin>: error: Expected identifier but found \"#foo\"\n")
	expectParseErrorTS(t, "class Foo { @dec static #foo = 1 }", "<stdin>: error: Expected identifier but found \"#foo\"\n")
	expectParseErrorTS(t, "class Foo { @dec static #foo() {} }", "<stdin>: error: Expected identifier but found \"#foo\"\n")
	expectParseErrorTS(t, "class Foo { @dec static *#foo() {} }", "<stdin>: error: Expected identifier but found \"#foo\"\n")
	expectParseErrorTS(t, "class Foo { @dec static async #foo() {} }", "<stdin>: error: Expected identifier but found \"#foo\"\n")
	expectParseErrorTS(t, "class Foo { @dec static async* #foo() {} }", "<stdin>: error: Expected identifier but found \"#foo\"\n")
}

func TestTSTry(t *testing.T) {
	expectPrintedTS(t, "try {} catch (x: any) {}", "try {\n} catch (x) {\n}\n")
	expectPrintedTS(t, "try {} catch (x: unknown) {}", "try {\n} catch (x) {\n}\n")
	expectPrintedTS(t, "try {} catch (x: number) {}", "try {\n} catch (x) {\n}\n")

	expectPrintedTS(t, "try {} catch ({x}: any) {}", "try {\n} catch ({x}) {\n}\n")
	expectPrintedTS(t, "try {} catch ({x}: unknown) {}", "try {\n} catch ({x}) {\n}\n")
	expectPrintedTS(t, "try {} catch ({x}: number) {}", "try {\n} catch ({x}) {\n}\n")

	expectPrintedTS(t, "try {} catch ([x]: any) {}", "try {\n} catch ([x]) {\n}\n")
	expectPrintedTS(t, "try {} catch ([x]: unknown) {}", "try {\n} catch ([x]) {\n}\n")
	expectPrintedTS(t, "try {} catch ([x]: number) {}", "try {\n} catch ([x]) {\n}\n")

	expectParseErrorTS(t, "try {} catch (x!) {}", "<stdin>: error: Expected \")\" but found \"!\"\n")
	expectParseErrorTS(t, "try {} catch (x!: any) {}", "<stdin>: error: Expected \")\" but found \"!\"\n")
	expectParseErrorTS(t, "try {} catch (x!: unknown) {}", "<stdin>: error: Expected \")\" but found \"!\"\n")
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
	expectPrintedTS(t, "(foo) ? (foo as Bar) : null;", "foo ? foo : null;\n")
	expectPrintedTS(t, "((foo) ? (foo as Bar) : null)", "foo ? foo : null;\n")
	expectPrintedTS(t, "let x = a ? (b, c) : (d, e)", "let x = a ? (b, c) : (d, e);\n")

	expectPrintedTS(t, "let x: () => void = () => {}", "let x = () => {\n};\n")
	expectPrintedTS(t, "let x: (y) => void = () => {}", "let x = () => {\n};\n")
	expectPrintedTS(t, "let x: (this) => void = () => {}", "let x = () => {\n};\n")
	expectPrintedTS(t, "let x: (this: any) => void = () => {}", "let x = () => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): (() => {}) => { };", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): () => {} => { };", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): (y) => {} => { };", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ([,[b]]) => {} => { };", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ([a,[b]]) => {} => { };", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ([a,[b],]) => {} => { };", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ({a}) => {} => { };", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ({a,}) => {} => { };", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ({a:{b}}) => {} => { };", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ({0:{b}}) => {} => { };", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ({'a':{b}}) => {} => { };", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ({if:{b}}) => {} => { };", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): (y[]) => {};", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): (a | b) => {};", "let x = (y) => {\n};\n")
	expectParseErrorTS(t, "let x = (y: any): (y) => {};", "<stdin>: error: Unexpected \":\"\n")
	expectParseErrorTS(t, "let x = (y: any): (y) => {return 0};", "<stdin>: error: Unexpected \":\"\n")
	expectParseErrorTS(t, "let x = (y: any): asserts y is (y) => {};", "<stdin>: error: Unexpected \":\"\n")

	expectPrintedTS(t, "async (): void => {}", "async () => {\n};\n")
	expectPrintedTS(t, "async (a): void => {}", "async (a) => {\n};\n")
	expectParseErrorTS(t, "async x: void => {}", "<stdin>: error: Expected \"=>\" but found \":\"\n")

	expectPrintedTS(t, "(x: boolean): asserts x => {}", "(x) => {\n};\n")
	expectPrintedTS(t, "(x: boolean): asserts this is object => {}", "(x) => {\n};\n")
	expectPrintedTS(t, "(x: T): asserts x is NonNullable<T> => {}", "(x) => {\n};\n")
	expectPrintedTS(t, "(x: any): x is number => {}", "(x) => {\n};\n")
	expectPrintedTS(t, "(x: any): this is object => {}", "(x) => {\n};\n")
	expectPrintedTS(t, "(x: any): (() => void) => {}", "(x) => {\n};\n")
	expectPrintedTS(t, "(x: any): ((y: any) => void) => {}", "(x) => {\n};\n")

	expectPrintedTS(t, "let x: () => {} | ({y: z});", "let x;\n")
	expectPrintedTS(t, "function x(): ({y: z}) {}", "function x() {\n}\n")
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

	expectPrintedTS(t, "new Foo!()", "new Foo();\n")
	expectPrintedTS(t, "new Foo!<number>()", "new Foo();\n")
	expectPrintedTS(t, "new Foo\n!(x)", "new Foo();\n!x;\n")
	expectPrintedTS(t, "new Foo<number>!(x)", "new Foo() < number > !x;\n")
	expectParseErrorTS(t, "new Foo<number>!()", "<stdin>: error: Unexpected \")\"\n")
	expectParseError(t, "new Foo!()", "<stdin>: error: Unexpected \"!\"\n")
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

// This is TypeScript-specific import syntax
func TestTSImportEquals(t *testing.T) {
	expectPrintedTS(t, "import x = require('foo'); x()", "const x = require(\"foo\");\nx();\n")
	expectPrintedTS(t, "import x = require('foo')\nx()", "const x = require(\"foo\");\nx();\n")
	expectPrintedTS(t, "import x = foo.bar; x()", "var x = foo.bar;\nx();\n")
	expectPrintedTS(t, "import x = foo.bar\nx()", "var x = foo.bar;\nx();\n")
	expectParseErrorTS(t, "{ import x = foo.bar }", "<stdin>: error: Unexpected \"x\"\n")

	expectPrintedTS(t, "export import x = require('foo'); x()", "export const x = require(\"foo\");\nx();\n")
	expectPrintedTS(t, "export import x = require('foo')\nx()", "export const x = require(\"foo\");\nx();\n")
	expectPrintedTS(t, "export import x = foo.bar; x()", "export var x = foo.bar;\nx();\n")
	expectPrintedTS(t, "export import x = foo.bar\nx()", "export var x = foo.bar;\nx();\n")

	expectParseError(t, "export import foo = bar", "<stdin>: error: Unexpected \"import\"\n")
	expectParseErrorTS(t, "export import {foo} from 'bar'", "<stdin>: error: Expected identifier but found \"{\"\n")
	expectParseErrorTS(t, "export import foo from 'bar'", "<stdin>: error: Expected \"=\" but found \"from\"\n")
	expectParseErrorTS(t, "export import foo = bar; var x; export {x as foo}",
		"<stdin>: error: Multiple exports with the same name \"foo\"\n")
	expectParseErrorTS(t, "{ export import foo = bar }", "<stdin>: error: Unexpected \"export\"\n")
}

func TestTSImportEqualsInNamespace(t *testing.T) {
	expectPrintedTS(t, "namespace ns { import foo = bar }", "")
	expectPrintedTS(t, "namespace ns { import foo = bar; type x = foo.x }", "")
	expectPrintedTS(t, "namespace ns { import foo = bar.x; foo }", `var ns;
(function(ns) {
  var foo = bar.x;
  foo;
})(ns || (ns = {}));
`)
	expectPrintedTS(t, "namespace ns { export import foo = bar }", `var ns;
(function(ns) {
  ns.foo = bar;
})(ns || (ns = {}));
`)
	expectPrintedTS(t, "namespace ns { export import foo = bar.x; foo }", `var ns;
(function(ns) {
  ns.foo = bar.x;
  ns.foo;
})(ns || (ns = {}));
`)
	expectParseErrorTS(t, "namespace ns { import {foo} from 'bar' }", "<stdin>: error: Expected identifier but found \"{\"\n")
	expectParseErrorTS(t, "namespace ns { import foo from 'bar' }", "<stdin>: error: Expected \"=\" but found \"from\"\n")
	expectParseErrorTS(t, "namespace ns { export import {foo} from 'bar' }", "<stdin>: error: Expected identifier but found \"{\"\n")
	expectParseErrorTS(t, "namespace ns { export import foo from 'bar' }", "<stdin>: error: Expected \"=\" but found \"from\"\n")
	expectParseErrorTS(t, "namespace ns { { import foo = bar } }", "<stdin>: error: Unexpected \"foo\"\n")
	expectParseErrorTS(t, "namespace ns { { export import foo = bar } }", "<stdin>: error: Unexpected \"export\"\n")
}

func TestTSTypeOnlyImport(t *testing.T) {
	expectPrintedTS(t, "import type foo from 'bar'; x", "x;\n")
	expectPrintedTS(t, "import type foo from 'bar'\nx", "x;\n")
	expectPrintedTS(t, "import type * as foo from 'bar'; x", "x;\n")
	expectPrintedTS(t, "import type * as foo from 'bar'\nx", "x;\n")
	expectPrintedTS(t, "import type {foo, bar as baz} from 'bar'; x", "x;\n")
	expectPrintedTS(t, "import type {foo, bar as baz} from 'bar'\nx", "x;\n")

	expectPrintedTS(t, "import type = bar", "var type = bar;\n")
	expectPrintedTS(t, "import type from 'bar'; type", "import type from \"bar\";\ntype;\n")

	expectParseErrorTS(t, "import type", "<stdin>: error: Expected \"from\" but found end of file\n")
	expectParseErrorTS(t, "import type * foo", "<stdin>: error: Expected \"as\" but found \"foo\"\n")
	expectParseErrorTS(t, "import type * as 'bar'", "<stdin>: error: Expected identifier but found \"'bar'\"\n")
	expectParseErrorTS(t, "import type { 'bar'", "<stdin>: error: Expected identifier but found \"'bar'\"\n")

	expectParseErrorTS(t, "import type foo, * as foo from 'bar'", "<stdin>: error: Expected \"from\" but found \",\"\n")
	expectParseErrorTS(t, "import type foo, {foo} from 'bar'", "<stdin>: error: Expected \"from\" but found \",\"\n")
}

func TestTSTypeOnlyExport(t *testing.T) {
	expectPrintedTS(t, "export type {foo, bar as baz} from 'bar'", "")
	expectPrintedTS(t, "export type {foo, bar as baz}", "")
	expectPrintedTS(t, "export type {foo} from 'bar'; x", "x;\n")
	expectPrintedTS(t, "export type {foo} from 'bar'\nx", "x;\n")
	expectPrintedTS(t, "export type {default} from 'bar'", "")
	expectParseErrorTS(t, "export type {default}", "<stdin>: error: Expected identifier but found \"default\"\n")

	// Named exports should be removed if they don't refer to a local symbol
	expectPrintedTS(t, "const Foo = {}; export {Foo}", "const Foo = {};\nexport {Foo};\n")
	expectPrintedTS(t, "type Foo = {}; export {Foo}", "")
	expectPrintedTS(t, "const Foo = {}; export {Foo as Bar}", "const Foo = {};\nexport {Foo as Bar};\n")
	expectPrintedTS(t, "type Foo = {}; export {Foo as Bar}", "")
	expectPrintedTS(t, "import Foo from 'foo'; export {Foo}", "import Foo from \"foo\";\nexport {Foo};\n")
	expectPrintedTS(t, "import {Foo} from 'foo'; export {Foo}", "import {Foo} from \"foo\";\nexport {Foo};\n")
	expectPrintedTS(t, "import * as Foo from 'foo'; export {Foo}", "import * as Foo from \"foo\";\nexport {Foo};\n")
	expectPrintedTS(t, "{ var Foo; } export {Foo}", "{\n  var Foo;\n}\nexport {Foo};\n")
	expectPrintedTS(t, "{ let Foo; } export {Foo}", "{\n  let Foo;\n}\n")
	expectPrintedTS(t, "export {Foo}", "")
	expectParseError(t, "export {Foo}", "<stdin>: error: \"Foo\" is not declared in this file\n")
}

func TestTSOptionalChain(t *testing.T) {
	expectParseError(t, "a?.<T>()", "<stdin>: error: Expected identifier but found \"<\"\n")
	expectPrintedTS(t, "a?.<T>()", "a?.();\n")
	expectParseErrorTS(t, "a?.<T>b", "<stdin>: error: Expected \"(\" but found \"b\"\n")
	expectParseErrorTS(t, "a?.<T>[b]", "<stdin>: error: Expected \"(\" but found \"[\"\n")

	expectPrintedTS(t, "a?.b.c", "a?.b.c;\n")
	expectPrintedTS(t, "(a?.b).c", "(a?.b).c;\n")
	expectPrintedTS(t, "a?.b!.c", "a?.b.c;\n")

	expectPrintedTS(t, "a?.b[c]", "a?.b[c];\n")
	expectPrintedTS(t, "(a?.b)[c]", "(a?.b)[c];\n")
	expectPrintedTS(t, "a?.b![c]", "a?.b[c];\n")

	expectPrintedTS(t, "a?.b(c)", "a?.b(c);\n")
	expectPrintedTS(t, "(a?.b)(c)", "(a?.b)(c);\n")
	expectPrintedTS(t, "a?.b!(c)", "a?.b(c);\n")
	expectPrintedTS(t, "a?.b<T>(c)", "a?.b(c);\n")
}

func TestTSJSX(t *testing.T) {
	expectPrintedTS(t, "const x = <number>1", "const x = 1;\n")
	expectPrintedTSX(t, "const x = <number>1</number>", "const x = /* @__PURE__ */ React.createElement(\"number\", null, \"1\");\n")
	expectParseErrorTSX(t, "const x = <number>1", "<stdin>: error: Unexpected end of file\n")

	expectPrintedTSX(t, "<x>a{}c</x>", "/* @__PURE__ */ React.createElement(\"x\", null, \"a\", \"c\");\n")
	expectPrintedTSX(t, "<x>a{b}c</x>", "/* @__PURE__ */ React.createElement(\"x\", null, \"a\", b, \"c\");\n")
	expectPrintedTSX(t, "<x>a{...b}c</x>", "/* @__PURE__ */ React.createElement(\"x\", null, \"a\", b, \"c\");\n")

	expectPrintedTSX(t, "const x = <Foo<T>></Foo>", "const x = /* @__PURE__ */ React.createElement(Foo, null);\n")
	expectPrintedTSX(t, "const x = <Foo<T> data-foo></Foo>", "const x = /* @__PURE__ */ React.createElement(Foo, {\n  \"data-foo\": true\n});\n")
	expectParseErrorTSX(t, "const x = <Foo<T>=>", "<stdin>: error: Expected \">\" but found \"=\"\n")

	expectPrintedTS(t, "const x = <T>() => {}", "const x = () => {\n};\n")
	expectPrintedTS(t, "const x = <T>(y)", "const x = y;\n")
	expectPrintedTS(t, "const x = <T>(y, z)", "const x = (y, z);\n")
	expectPrintedTS(t, "const x = <T>(y: T) => {}", "const x = (y) => {\n};\n")
	expectPrintedTS(t, "const x = <T>(y, z) => {}", "const x = (y, z) => {\n};\n")
	expectPrintedTS(t, "const x = <T = X>(y: T) => {}", "const x = (y) => {\n};\n")
	expectPrintedTS(t, "const x = <T = X>(y, z) => {}", "const x = (y, z) => {\n};\n")
	expectPrintedTS(t, "const x = <T extends X>(y: T) => {}", "const x = (y) => {\n};\n")
	expectPrintedTS(t, "const x = <T extends X>(y, z) => {}", "const x = (y, z) => {\n};\n")
	expectPrintedTS(t, "const x = <T extends X = Y>(y: T) => {}", "const x = (y) => {\n};\n")
	expectPrintedTS(t, "const x = <T extends X = Y>(y, z) => {}", "const x = (y, z) => {\n};\n")

	expectPrintedTS(t, "const x = async <T>() => {}", "const x = async () => {\n};\n")
	expectPrintedTS(t, "const x = async <T>(y)", "const x = async(y);\n")
	expectPrintedTS(t, "const x = async <T>(y, z)", "const x = async(y, z);\n")
	expectPrintedTS(t, "const x = async <T>(y: T) => {}", "const x = async (y) => {\n};\n")
	expectPrintedTS(t, "const x = async <T>(y, z) => {}", "const x = async (y, z) => {\n};\n")
	expectPrintedTS(t, "const x = async <T = X>(y: T) => {}", "const x = async (y) => {\n};\n")
	expectPrintedTS(t, "const x = async <T = X>(y, z) => {}", "const x = async (y, z) => {\n};\n")
	expectPrintedTS(t, "const x = async <T extends X>(y: T) => {}", "const x = async (y) => {\n};\n")
	expectPrintedTS(t, "const x = async <T extends X>(y, z) => {}", "const x = async (y, z) => {\n};\n")
	expectPrintedTS(t, "const x = async <T extends X = Y>(y: T) => {}", "const x = async (y) => {\n};\n")
	expectPrintedTS(t, "const x = async <T extends X = Y>(y, z) => {}", "const x = async (y, z) => {\n};\n")
	expectParseErrorTS(t, "const x = async <T>(y: T)", "<stdin>: error: Unexpected \":\"\n")

	expectPrintedTS(t, "const x = <{}>() => {}", "const x = () => {\n};\n")
	expectPrintedTS(t, "const x = <{}>(y)", "const x = y;\n")
	expectPrintedTS(t, "const x = <{}>(y, z)", "const x = (y, z);\n")
	expectPrintedTS(t, "const x = <{}>(y, z) => {}", "const x = (y, z) => {\n};\n")

	expectPrintedTS(t, "const x = <[]>() => {}", "const x = () => {\n};\n")
	expectPrintedTS(t, "const x = <[]>(y)", "const x = y;\n")
	expectPrintedTS(t, "const x = <[]>(y, z)", "const x = (y, z);\n")
	expectPrintedTS(t, "const x = <[]>(y, z) => {}", "const x = (y, z) => {\n};\n")

	expectPrintedTSX(t, "(<T>(y) => {}</T>)", "/* @__PURE__ */ React.createElement(T, null, \"(y) => \");\n")
	expectPrintedTSX(t, "(<T extends>(y) => {}</T>)", "/* @__PURE__ */ React.createElement(T, {\n  extends: true\n}, \"(y) => \");\n")
	expectPrintedTSX(t, "(<T extends={false}>(y) => {}</T>)", "/* @__PURE__ */ React.createElement(T, {\n  extends: false\n}, \"(y) => \");\n")
	expectPrintedTSX(t, "(<T extends X>(y) => {})", "(y) => {\n};\n")
	expectPrintedTSX(t, "(<T extends X = Y>(y) => {})", "(y) => {\n};\n")
	expectPrintedTSX(t, "(<T, X>(y) => {})", "(y) => {\n};\n")
	expectPrintedTSX(t, "(<T, X>(y): (() => {}) => {})", "(y) => {\n};\n")
	expectParseErrorTSX(t, "(<[]>(y))", "<stdin>: error: Expected identifier but found \"[\"\n")
	expectParseErrorTSX(t, "(<T[]>(y))", "<stdin>: error: Expected \">\" but found \"[\"\n")
	expectParseErrorTSX(t, "(<T = X>(y))", "<stdin>: error: Expected \">\" but found \"=\"\n")
	expectParseErrorTSX(t, "(<T, X>(y))", "<stdin>: error: Expected \"=>\" but found \")\"\n")
	expectParseErrorTSX(t, "(<T, X>y => {})", "<stdin>: error: Expected \"(\" but found \"y\"\n")
}

func TestClassSideEffectOrder(t *testing.T) {
	// The order of computed property side effects must not change
	expectPrintedTS(t, `class Foo {
	[a()]() {}
	[b()];
	[c()] = 1;
	[d()]() {}
	static [e()];
	static [f()] = 1;
	static [g()]() {}
	[h()];
}
`, `var _a, _b;
class Foo {
  constructor() {
    this[_a] = 1;
  }
  [a()]() {
  }
  [(b(), _a = c(), d())]() {
  }
  static [(e(), _b = f(), g())]() {
  }
}
h();
Foo[_b] = 1;
`)
}
