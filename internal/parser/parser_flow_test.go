package parser

import (
	"testing"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/printer"
	"github.com/evanw/esbuild/internal/renamer"
	"github.com/evanw/esbuild/internal/test"
)

func expectParseErrorFlow(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log := logger.NewDeferLog()
		Parse(log, test.SourceForTest(contents), config.Options{
			Flow: config.FlowOptions{
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

func expectPrintedFlow(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log := logger.NewDeferLog()
		tree, ok := Parse(log, test.SourceForTest(contents), config.Options{
			Flow: config.FlowOptions{
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
		symbols := ast.NewSymbolMap(1)
		symbols.Outer[0] = tree.Symbols
		r := renamer.NewNoOpRenamer(symbols)
		js := printer.Print(tree, symbols, r, printer.PrintOptions{}).JS
		test.AssertEqual(t, string(js), expected)
	})
}

func TestFlowExistentialType(t *testing.T) {
	expectPrintedFlow(t, "type foo = *;", "")
}

func TestFlowExportTypeNamespace(t *testing.T) {
	expectPrintedFlow(t, "export type * from 'path'", "")
}

func TestFlowImportTypeof(t *testing.T) {
	expectPrintedFlow(t, "import typeof foo from 'pkg'", "")
	expectPrintedFlow(t, "import typeof {foo} from 'pkg'", "")
	expectPrintedFlow(t, "import typeof {foo, bar} from 'pkg'", "")
	expectPrintedFlow(t, "import typeof * as ns from 'pkg'", "")
	expectPrintedFlow(t, "import typeof foo, {bar, baz} from 'pkg'", "")
	expectPrintedFlow(t, "import typeof foo, * as ns from 'pkg'", "")

	// Allowed by Flow parser, but not by Babel
	expectPrintedFlow(t, "import typeof from from 'pkg'", "")

	expectParseErrorFlow(t, "import typeof foo, bar from 'pkg'", "<stdin>: error: Unexpected \"bar\"\n")
	expectParseErrorFlow(t, "import typeof * as ns, bar from 'pkg'", "<stdin>: error: Expected \"from\" but found \",\"\n")
	expectParseErrorFlow(t, "import typeof {foo}, bar from 'pkg'", "<stdin>: error: Expected \"from\" but found \",\"\n")
}

func TestFlowTypeCastExpressions(t *testing.T) {
	expectPrintedFlow(t, "(value: number)", "value;\n")
	expectPrintedFlow(t, "((value: any): number)", "value;\n")
	expectPrintedFlow(t, "(value: typeof bar)", "value;\n")

	// expectPrintedFlow(t, "([a: string]) => {}", "([a]) => {};")

	expectPrintedFlow(t, "({xxx: 0, yyy: \"hey\"}: {xxx: number; yyy: string})", "({xxx: 0, yyy: \"hey\"});\n")
	expectPrintedFlow(t, "((xxx) => xxx + 1: (xxx: number) => number)", "(xxx) => xxx + 1;\n")
	expectPrintedFlow(t, "(xxx: number)", "xxx;\n")
	expectPrintedFlow(t, "((xxx: number), (yyy: string))", "xxx, yyy;\n")
	// expectPrintedFlow(t, "(<T>() => {}: any);\n((<T>() => {}): any);\n", "() => {\n};\n() => {\n};\n")
	// expectPrintedFlow(t, "([a: string]) => {};\n([a, [b: string]]) => {};\n([a: string] = []) => {};\n({ x: [a: string] }) => {};\n\nasync ([a: string]) => {};\nasync ([a, [b: string]]) => {};\nasync ([a: string] = []) => {};\nasync ({ x: [a: string] }) => {};\n\nlet [a1: string] = c;\nlet [a2, [b: string]] = c;\nlet [a3: string] = c;\nlet { x: [a4: string] } = c;\n", "([a]) => {\n};\n([a, [b2]]) => {\n};\n([a] = []) => {\n};\n({\n  x: [a]\n}) => {\n};\nasync ([a]) => {\n};\nasync ([a, [b2]]) => {\n};\nasync ([a] = []) => {\n};\nasync ({\n  x: [a]\n}) => {\n};\nlet [a1] = c;\nlet [a2, [b]] = c;\nlet [a3] = c;\nlet {\n  x: [a4]\n} = c;\n")
	// expectPrintedFlow(t, "(<T>() => {}: any);\n((<T>() => {}): any);\n", "() => {\n};\n() => {\n};\n")
	expectPrintedFlow(t, "function* foo(z) {\n  const x = ((yield 3): any)\n}", "function* foo(z) {\n  const x = yield 3;\n}\n")
	expectPrintedFlow(t, "function* foo(z) {\n  const x = (yield 3: any)\n}", "function* foo(z) {\n  const x = yield 3;\n}\n")
}

// Auto-generated ported test cases

func TestPortedBabelFlowTests(t *testing.T) {
	// // anonymous-function-no-parens-types
	// expectPrintedFlow(t, "type A = string => void\n", "")
	// expectPrintedFlow(t, "type A = Array<string> => void\n", "")
	// expectPrintedFlow(t, "var f = (): number => 123;\n", "var f = () => 123;\n")
	// expectPrintedFlow(t, "var f = (): string | number => 123;\n", "var f = () => 123;\n")
	// expectPrintedFlow(t, "var f = (x): (number => 123) => 123;\n", "var f = (x) => 123;\n")
	// expectPrintedFlow(t, "type A = string | number => boolean;\n", "")
	// expectPrintedFlow(t, "type A = ?number => boolean;\n", "")
	// expectPrintedFlow(t, "type A = string & number => boolean;\n", "")
	// expectPrintedFlow(t, "type A = number[] => boolean;\n", "")
	// expectPrintedFlow(t, "type A = string => boolean | number;\n", "")
	// expectPrintedFlow(t, "type A = (string => boolean) => number\n", "")
	// expectPrintedFlow(t, "const x = ():*=>{}\n", "const x = () => {\n};\n")
	// expectPrintedFlow(t, "type T = Array<(string) => number>", "")
	// expectPrintedFlow(t, "type A = string => boolean => number;\n", "")
	// expectPrintedFlow(t, "let x = (): Array<(string) => number> => []", "let x = () => [];\n")
	//
	// // bounded-polymorphism
	// expectPrintedFlow(t, "function bar<T: ?number>() {}", "function bar() {\n}\n")
	// expectPrintedFlow(t, "class A<T: Foo> {}", "class A {\n}\n")
	//
	// // anonymous-function-types
	// expectPrintedFlow(t, "declare function foo(x: number, string): void;\n", "")
	// expectPrintedFlow(t, "type A = (string) => void\n", "")
	// expectPrintedFlow(t, "type A = (Array<string>) => void\n", "")
	// expectPrintedFlow(t, "// TODO: declare export syntax\n// declare export function foo(x: number, string): void;\n", "")
	// expectPrintedFlow(t, "type A = (string,) => void\n", "")
	// expectPrintedFlow(t, "type A = (Array<string>,) => void\n", "")
	// expectPrintedFlow(t, "type A = (x: string, number) => void\n", "")
	// expectPrintedFlow(t, "type A = (Array<string>, ...Array<string>) => void\n", "")
	// expectPrintedFlow(t, "type A = (...Array<string>) => void\n", "")
	// expectPrintedFlow(t, "var f = (x): (x: number) => 123 => 123;\n", "var f = (x) => 123;\n")
	// expectPrintedFlow(t, "var f = (): (number) => 123;\n", "var f = () => 123;\n")
	// expectPrintedFlow(t, "var f = (x): ((number) => 123) => 123;\n", "var f = (x) => 123;\n")
	// expectPrintedFlow(t, "var f = (): string | (number) => 123;\n", "var f = () => 123;\n")
	// expectPrintedFlow(t, "var f = (x): | 1 | 2 => 1;\n", "var f = (x) => 1;\n")
	//
	// // array-types
	// expectPrintedFlow(t, "var a: number[]", "var a;\n")
	// expectPrintedFlow(t, "var a: ?number[]", "var a;\n")
	// expectPrintedFlow(t, "var a: (?number)[]", "var a;\n")
	// expectPrintedFlow(t, "var a: number[][]\n", "var a;\n")
	// expectPrintedFlow(t, "var a: (() => number)[]", "var a;\n")
	// expectPrintedFlow(t, "var a: () => number[]", "var a;\n")
	// expectPrintedFlow(t, "var a: typeof A[]", "var a;\n")
	// expectPrintedFlow(t, "var a: number[][][]\n", "var a;\n")
	// expectPrintedFlow(t, "var a: number\n[]\n", "var a;\n[];\n")
	//
	// // class-private-property
	// expectPrintedFlow(t, "class A {\n  #prop1: string;\n  #prop2: number = value;\n}", "class A {\n  #prop1;\n  #prop2 = value;\n}\n")
	// expectPrintedFlow(t, "class A {\n  #prop1: string;\n  #prop2: number;\n}", "class A {\n  #prop1;\n  #prop2;\n}\n")
	// expectPrintedFlow(t, "class A {\n  declare #foo\n}\n", "class A {\n  #foo;\n}\n")
	//
	// // comment
	// expectPrintedFlow(t, "class MyClass {\n  /*   :: prop: string; */\n    /*   :: prop2: number; */\n}\n", "class MyClass {\n  prop;\n  prop2;\n}\n")
	// expectPrintedFlow(t, "/*::\ntype Foo = {\n  foo: number,\n  bar: boolean,\n  baz: string\n};\n*/\n", "")
	// expectPrintedFlow(t, "/* flow-include\ntype Foo = {\n  foo: number,\n  bar: boolean,\n  baz: string\n};\n*/\n", "")
	// expectPrintedFlow(t, "function method(param /*: string */) /*: number */ {\n}\n", "function method(param) {\n}\n")
	// expectPrintedFlow(t, "class MyClass {\n  /*      flow-include prop: string; */\n      /*   flow-include prop2: number; */\n}\n", "class MyClass {\n  prop;\n  prop2;\n}\n")
	// expectPrintedFlow(t, "/* hi */\nfunction commentsAttachedToIdentifier() {\n  var x = (...args: any) => {};\n}\n", "function commentsAttachedToIdentifier() {\n  var x = (...args) => {\n  };\n}\n")
	//
	// // call-properties
	// expectPrintedFlow(t, "var a : { (): number; }", "var a;\n")
	// expectPrintedFlow(t, "var a : { (): number }", "var a;\n")
	// expectPrintedFlow(t, "var a : { <T>(x: T): number; }", "var a;\n")
	// expectPrintedFlow(t, "interface A { (): number; }", "")
	// expectPrintedFlow(t, "var a : { (): number; y: string; (x: string): string }", "var a;\n")
	//
	// // classes
	// expectPrintedFlow(t, "class C { field:*=null }\n", "class C {\n  field = null;\n}\n")
	// expectPrintedFlow(t, "class A implements B {}\n", "class A {\n}\n")
	// expectPrintedFlow(t, "class A implements B, C {}\n", "class A {\n}\n")
	// expectPrintedFlow(t, "class A {\n  constructor(): Object {\n    return {};\n  }\n}\n", "class A {\n  constructor() {\n    return {};\n  }\n}\n")
	//
	// // comment-disabled
	// expectPrintedFlow(t, "class MyClass {\n  /*:: prop: string; */\n  /*    :: foo: number; */\n}\n", "class MyClass {\n}\n")
	// expectPrintedFlow(t, "/*::\ntype Foo = {\n  foo: number,\n  bar: boolean,\n  baz: string\n};\n*/\n", "")
	// expectPrintedFlow(t, "function method(param /*: string */) /*: number */ {\n}\n", "function method(param) {\n}\n")
	// expectPrintedFlow(t, "class MyClass {\n  /*flow-include prop: string; */\n  /*      flow-include foo: number; */\n}\n", "class MyClass {\n}\n")
	// expectPrintedFlow(t, "/* flow-include\ntype Foo = {\n  foo: number,\n  bar: boolean,\n  baz: string\n};\n*/\n", "")
	//
	// // class-properties
	// expectPrintedFlow(t, "class A {\n  @dec declare foo\n}\n", "class A {\n}\n")
	// expectPrintedFlow(t, "class A {\n  declare foo\n}\n", "class A {\n}\n")
	// expectPrintedFlow(t, "class A {\n  declare [foo]\n}\n", "class A {\n}\n")
	// expectPrintedFlow(t, "class A {\n  declare static\n}\n", "class A {\n}\n")
	// expectPrintedFlow(t, "class A {\n  declare foo: string\n}\n", "class A {\n}\n")
	// expectPrintedFlow(t, "class A {\n  declare static foo\n}\n", "class A {\n}\n")
	// expectPrintedFlow(t, "class A {\n  declare\n}\n", "class A {\n  declare;\n}\n")
	// expectPrintedFlow(t, "class A {\n  declare: string\n}\n", "class A {\n  declare;\n}\n")
	// expectPrintedFlow(t, "declare class B {\n  get a(): number;\n  set b(a: number): void;\n  get \"c\"(): number;\n  set \"d\"(a: number): void;\n  get 1(): number;\n  set 2(a: number): void;\n}\n", "")
	// expectPrintedFlow(t, "declare class A {\n  static: T;\n}\n", "")
	// expectPrintedFlow(t, "class A {\n  declare() {}\n}\n", "class A {\n  declare() {\n  }\n}\n")
	// expectPrintedFlow(t, "class A {\n  static declare\n}\n", "class A {\n  static declare;\n}\n")
	//
	// // declare-class
	// expectPrintedFlow(t, "declare class A implements B, C {}\n", "")
	// expectPrintedFlow(t, "declare class A implements B {}\n", "")
	// expectPrintedFlow(t, "declare class A mixins B implements C {}\n", "")
	// expectPrintedFlow(t, "declare class A mixins B, C {}\n", "")
	// expectPrintedFlow(t, "declare class A mixins B {}\n", "")
	//
	// // declare-export
	// expectPrintedFlow(t, "declare module \"foo\" { declare export class Foo { meth(p1: number): void; } }\n", "")
	// expectPrintedFlow(t, "declare export default class A {};\n", ";\n")
	// expectPrintedFlow(t, "declare export default function bar(p1: number): string;\n", "")
	// expectPrintedFlow(t, "declare export default (a:number) => number\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export {a,} from \"bar\"; }\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export default number|string; }", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export function bar(p1: number): string; }\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export interface bar {} }\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export interface bar {} declare export var baz: number; }\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export {a,}; }\nvar a;\n", "var a;\n")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export interface bar {} declare module.exports: number; }\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export * from \"bar\"; }\n", "")
	// expectPrintedFlow(t, "declare export * as test from ''\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export type bar = number; }\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export type bar = number; declare module.exports: number; }\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export type * from \"bar\"; }", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export var a: number; }\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export type bar = number; declare export var baz: number; }\n", "")
	//
	// // declare-module
	// expectPrintedFlow(t, "declare module A {}", "")
	// expectPrintedFlow(t, "declare module \"./a/b.js\" {}", "")
	// expectPrintedFlow(t, "declare module.exports: { foo(): number; }\n", "")
	// expectPrintedFlow(t, "declare module A { declare var x: number; }", "")
	// expectPrintedFlow(t, "declare module A { declare function foo(): number; }", "")
	// expectPrintedFlow(t, "declare module A { declare class B { foo(): number; } }", "")
	// expectPrintedFlow(t, "declare module A { declare module.exports: number; }\n", "")
	// expectPrintedFlow(t, "declare module A { declare module.exports: { foo(): number; } }\n", "")
	// expectPrintedFlow(t, "declare module \"M\" {\n  import type T from \"TM\";\n  import typeof U from \"UM\";\n}\n", "")
	//
	// // def-site-variance
	// expectPrintedFlow(t, "class C<+T,-U> {}\nfunction f<+T,-U>() {}\ntype T<+T,-U> = {}\n", "class C {\n}\nfunction f() {\n}\n")
	//
	// // declare-statements
	// expectPrintedFlow(t, "declare class A { static foo(): number; static x : string }", "")
	// expectPrintedFlow(t, "declare class A { static () : number }", "")
	// expectPrintedFlow(t, "declare class A { static [ indexer: number]: string }", "")
	// expectPrintedFlow(t, "declare var foo", "")
	// expectPrintedFlow(t, "declare type A = string;\ndeclare type T<U> = { [k:string]: U };\n", "")
	// expectPrintedFlow(t, "declare interface I { foo: string }\ndeclare interface I2<T> { foo: T }\n", "")
	// expectPrintedFlow(t, "declare class A { static () : number }\ndeclare class B { () : number }\n", "")
	// expectPrintedFlow(t, "declare class A mixins B<T>, C {}\n", "")
	// expectPrintedFlow(t, "declare class X {\n\ta: number;\n\tstatic b: number;\n\tc: number;\n}\n", "")
	// expectPrintedFlow(t, "declare class A extends C.B.D { }\n", "")
	// expectPrintedFlow(t, "declare var string: any;\n", "")
	// expectPrintedFlow(t, "declare function foo(): void", "")
	// expectPrintedFlow(t, "declare var foo;", "")
	// expectPrintedFlow(t, "declare function foo<T>(): void;", "")
	// expectPrintedFlow(t, "declare function foo(): void;", "")
	// expectPrintedFlow(t, "declare class A {}", "")
	// expectPrintedFlow(t, "declare function foo(x: number, y: string): void;", "")
	// expectPrintedFlow(t, "declare class IViewFactory { didAnimate(view:Object, prop:string) :void; }", "")
	// expectPrintedFlow(t, "declare class A<T> extends B<T> { x: number }", "")
	// expectPrintedFlow(t, "declare var x: symbol;\n", "")
	//
	// // explicit-inexact-object
	// expectPrintedFlow(t, "//@flow\ntype T = {...};\ntype U = {x: number, ...};\ntype V = {x: number, ...V, ...U};\n", "")
	// expectPrintedFlow(t, "//@flow\ntype T = { ..., };\ntype U = { ...; };\ntype V = {\n  x: number,\n  ...,\n};\ntype W = {\n  x: number;\n  ...;\n};\n", "")
	//
	// // interface-types
	// expectPrintedFlow(t, "type T = interface extends X { p: string }\n", "")
	// expectPrintedFlow(t, "type T = interface { p: string }\n", "")
	// expectPrintedFlow(t, "type T = interface extends X, Y { p: string }\n", "")
	// expectPrintedFlow(t, "type T = interface { static(): number }\n", "")
	//
	// // interfaces-module-and-script
	// expectPrintedFlow(t, "interface A {}", "")
	// expectPrintedFlow(t, "interface IFoo {\n  x: boolean;\n  static (): void;\n}\n", "")
	// expectPrintedFlow(t, "interface A extends B {}", "")
	// expectPrintedFlow(t, "interface A<T> extends B<T>, C<T> {}", "")
	// expectPrintedFlow(t, "interface Dictionary { [index: string]: string; length: number; }", "")
	// expectPrintedFlow(t, "class Foo implements Bar {}", "class Foo {\n}\n")
	// expectPrintedFlow(t, "interface A { foo: () => number; }", "")
	// expectPrintedFlow(t, "class Foo extends Bar implements Bat, Man<number> {}", "class Foo extends Bar {\n}\n")
	// expectPrintedFlow(t, "class Foo extends class Bar implements Bat {} {}", "class Foo extends class Bar {\n} {\n}\n")
	// expectPrintedFlow(t, "class Foo extends class Bar implements Bat {} implements Man {}", "class Foo extends class Bar {\n} {\n}\n")
	// expectPrintedFlow(t, "interface A { static(): number }\n", "")
	// expectPrintedFlow(t, "interface switch {}\n", "")
	// expectPrintedFlow(t, "interface B { static?: number }\n", "")
	// expectPrintedFlow(t, "interface Foo {}\n\nexport type { Foo }", "")
	// expectPrintedFlow(t, "class Foo implements switch {}\n", "class Foo {\n}\n")
	//
	// // internal-slot
	// expectPrintedFlow(t, "declare class C { [[foo]]: T }\n", "")
	// expectPrintedFlow(t, "declare class C { static [[foo]]: T }\n", "")
	// expectPrintedFlow(t, "interface T { [[foo]](): X }\n", "")
	// expectPrintedFlow(t, "type T = { [[foo]]: X }\n", "")
	// expectPrintedFlow(t, "interface T { [[foo]]: X }\n", "")
	// expectPrintedFlow(t, "type T = { [[foo]](): X }\n", "")
	// expectPrintedFlow(t, "type T = { [[foo]]?: X }\n", "")
	//
	// // literal-types
	// expectPrintedFlow(t, "var foo: true\n", "var foo;\n")
	// expectPrintedFlow(t, "var foo: false\n", "var foo;\n")
	// expectPrintedFlow(t, "var a: 123.0", "var a;\n")
	// expectPrintedFlow(t, "var foo: null\n", "var foo;\n")
	// expectPrintedFlow(t, "var a: 123", "var a;\n")
	// expectPrintedFlow(t, "var a: 0b1111011", "var a;\n")
	// expectPrintedFlow(t, "var a: -0b1111011\n", "var a;\n")
	// expectPrintedFlow(t, "var a: -0x7B\n", "var a;\n")
	// expectPrintedFlow(t, "var a: -123.0\n", "var a;\n")
	// expectPrintedFlow(t, "var a: 0x7B", "var a;\n")
	// expectPrintedFlow(t, "var a: 0o173", "var a;\n")
	// expectPrintedFlow(t, "var a: -0o173\n", "var a;\n")
	// expectPrintedFlow(t, "function createElement(tagName: \"div\"): HTMLDivElement {}", "function createElement(tagName) {\n}\n")
	// expectPrintedFlow(t, "function createElement(tagName: 'div'): HTMLDivElement {}", "function createElement(tagName) {\n}\n")
	//
	// // iterator
	// expectPrintedFlow(t, "declare class A {\n  @@iterator(): Iterator<File>;\n}\n", "")
	// expectPrintedFlow(t, "declare class A {\n  @@asyncIterator(): Iterator<File>;\n}\n", "")
	// expectPrintedFlow(t, "function foo(): { @@asyncIterator: () => string } {\n  return (0: any);\n}\n", "function foo() {\n  return 0;\n}\n")
	// expectPrintedFlow(t, "function foo(): { @@iterator: () => string } {\n  return (0: any);\n}\n", "function foo() {\n  return 0;\n}\n")
	// expectPrintedFlow(t, "interface A {\n  @@iterator(): Iterator<File>;\n}\n", "")
	// expectPrintedFlow(t, "interface A {\n  @@asyncIterator(): Iterator<File>;\n}\n", "")
	//
	// // object-types
	// expectPrintedFlow(t, "type o = { m(|int|bool): void }\n", "")
	//
	// // opaque-type-alias
	// expectPrintedFlow(t, "opaque type opaque = number;\nopaque type not_transparent = opaque;\n", "")
	// expectPrintedFlow(t, "declare export opaque type Foo\n", "")
	// expectPrintedFlow(t, "declare export opaque type Foo: Bar\n", "")
	// expectPrintedFlow(t, "declare opaque type Foo\n", "")
	// expectPrintedFlow(t, "declare opaque type Test: Foo\n", "")
	// expectPrintedFlow(t, "var opaque = 0;\nopaque += 4;\n", "var opaque = 0;\nopaque += 4;\n")
	// expectPrintedFlow(t, "opaque(0);\n", "opaque(0);\n")
	// expectPrintedFlow(t, "export opaque type Counter: Box<T> = Container<S>;\n", "")
	// expectPrintedFlow(t, "opaque type Counter: Box<T> = Container<T>;\n", "")
	// expectPrintedFlow(t, "opaque type ID = number;\n", "")
	// expectPrintedFlow(t, "export opaque type ID = number;\n", "")
	// expectPrintedFlow(t, "opaque type switch = number;\n", "")
	//
	// // pragma
	// expectPrintedFlow(t, "foo<x>(y);\n// @flow\nfoo<x>(y);", "foo < x > y;\nfoo < x > y;\n")
	// expectPrintedFlow(t, "'use strict';\n// @flow\nfoo<x>(y);", "\"use strict\";\nfoo(y);\n")
	// expectPrintedFlow(t, "// arbitrary comment\n// @flow\nfoo<x>(y);", "foo(y);\n")
	// expectPrintedFlow(t, "'use strict';\n// arbitrary comment\n// @flow\nfoo<x>(y);", "\"use strict\";\nfoo(y);\n")
	// expectPrintedFlow(t, "#!/usr/bin/env node\n'use strict';\n// arbitrary comment\n// @flow\nfoo<x>(y);", "#!/usr/bin/env node\n\"use strict\";\nfoo(y);\n")
	//
	// // optional-type
	// expectPrintedFlow(t, "const f = (x?) => {}\n", "const f = (x) => {\n};\n")
	// expectPrintedFlow(t, "const f = (x?, y?:Object = {}) => {}\n", "const f = (x, y = {}) => {\n};\n")
	// expectPrintedFlow(t, "const f = (...x?) => {}\n", "const f = (...x) => {\n};\n")
	//
	// // predicates
	// expectPrintedFlow(t, "declare function foo(x: mixed): boolean %checks(x !== null);\n", "")
	// expectPrintedFlow(t, "var f = (x: mixed): %checks => typeof x === \"string\";\n", "var f = (x) => typeof x === \"string\";\n")
	// expectPrintedFlow(t, "function foo(x: mixed): %checks { return typeof x === \"string\"; };\n", "function foo(x) {\n  return typeof x === \"string\";\n}\n;\n")
	// expectPrintedFlow(t, "declare function my_filter<T, P: $Pred<1>>(v: Array<T>, cb: P): Array<$Refine<T,P,1>>;\n", "")
	//
	// // multiple-declarations
	// expectPrintedFlow(t, "declare class C1{}\ndeclare class C1{}\n\ndeclare module M1 {\n  declare class C1 {}\n}\n", "")
	// expectPrintedFlow(t, "declare function F1(): void\ndeclare function F1(): void\nfunction F1() {}\n", "function F1() {\n}\n")
	// expectPrintedFlow(t, "declare var V1;\ndeclare var V1;\nvar V1;\n", "var V1;\n")
	//
	// // sourcetype-script
	// expectPrintedFlow(t, "export type Foo = number;\nexport opaque type Foo2 = number;\n", "")
	// expectPrintedFlow(t, "export type * from \"foo\";\n", "")
	// expectPrintedFlow(t, "import type { Foo } from \"\";\nimport typeof Foo2 from \"\";\n", "")
	//
	// // proto-props
	// expectPrintedFlow(t, "declare class A {\n  proto: T;\n}\n\ndeclare class B {\n  proto x: T;\n}\n\ndeclare class C {\n  proto +x: T;\n}\n", "")
	//
	// // trailing-function-commas-type
	// expectPrintedFlow(t, "( props: SomeType, ) : ReturnType => ( 3 );\n", "(props) => 3;\n")
	//
	// // qualified-generic-type
	// expectPrintedFlow(t, "var a : A.B", "var a;\n")
	// expectPrintedFlow(t, "var a : A.B<T>", "var a;\n")
	// expectPrintedFlow(t, "var a : A.B.C", "var a;\n")
	// expectPrintedFlow(t, "var a : typeof A.B<T>", "var a;\n")
	//
	// // tuples
	// expectPrintedFlow(t, "var a : [] = [];", "var a = [];\n")
	// expectPrintedFlow(t, "var a : [Foo<T>] = [foo];", "var a = [foo];\n")
	// expectPrintedFlow(t, "var a : [number,] = [123,];", "var a = [123];\n")
	// expectPrintedFlow(t, "var a : [number, string] = [123, \"duck\"];", "var a = [123, \"duck\"];\n")
	//
	// // scope
	// expectPrintedFlow(t, "function f() {\n  const g = true ? foo => {}: null;\n\n  const foo = 'foo'\n}\n", "function f() {\n  const g = (foo2) => {\n  };\n  const foo = \"foo\";\n}\n")
	// expectPrintedFlow(t, "declare class A {}\ndeclare class A {}\n", "")
	// expectPrintedFlow(t, "declare function A(): void;\ndeclare var A: number;\n", "")
	// expectPrintedFlow(t, "declare function A(): void;\ndeclare function A(): void;\n", "")
	// expectPrintedFlow(t, "declare function A(): void;\nfunction A() {}\n", "function A() {\n}\n")
	// expectPrintedFlow(t, "declare var A: number;\ndeclare var A: number;\n", "")
	//
	// // regression
	// expectPrintedFlow(t, "async (f)\n: t => { }\n", "async (f) => {\n};\n")
	// expectPrintedFlow(t, "test\n  ? (x: T): U => y\n  : z", "test ? (x) => y : z;\n")
	// expectPrintedFlow(t, "declare class C1 {}\ndeclare interface I1 {}\ndeclare type T1 = number;\n\ninterface I2 {}\ntype T2 = number;\n\nexport type { C1, I1, I2, T1, T2 }\n", "")
	// expectPrintedFlow(t, "test\n  ? (x: T): U => y\n", "test ? x : (U) => y;\n")
	// expectPrintedFlow(t, "class of<T> {}\n\n", "class of {\n}\n")
	// expectPrintedFlow(t, "// @flow\n\ntrue ? async.waterfall() : null;\n", "async.waterfall();\n")
	// expectPrintedFlow(t, "function of<T>() {}\n\n", "function of() {\n}\n")
	// expectPrintedFlow(t, "function *foo() {\n  const x = (yield 5: any);\n  x ? yield 1 : x;\n}\n", "function* foo() {\n  const x = yield 5;\n  x ? yield 1 : x;\n}\n")
	// expectPrintedFlow(t, "interface of<T> {}\n\n", "")
	// expectPrintedFlow(t, "class Foo {\n  foo() {\n    switch (1) {\n      case (MatrixType.IsScaling | MatrixType.IsTranslation):\n    }\n  }\n\n  bar() {\n    switch ((typeA << 4) | typeB) {}\n  }\n}\n", "class Foo {\n  foo() {\n    switch (1) {\n      case MatrixType.IsScaling | MatrixType.IsTranslation:\n    }\n  }\n  bar() {\n    switch (typeA << 4 | typeB) {\n    }\n  }\n}\n")
	// expectPrintedFlow(t, "let hello = (greeting:string = ' world') : string => {\n  console.log('hello' + greeting);\n};\n\nhello();\n", "let hello = (greeting = \" world\") => {\n  console.log(\"hello\" + greeting);\n};\nhello();\n")
	// expectPrintedFlow(t, "const fn: ( Object, ?Object ) => void = ( o1, o2 ) => o1;\nconst fn2: ( Object, ?Object, ) => void = ( o1, o2, ) => o1;\n", "const fn = (o1, o2) => o1;\nconst fn2 = (o1, o2) => o1;\n")
	// expectPrintedFlow(t, "const map = {\n  [age <= 17] : 'Too young'\n};\n", "const map = {\n  [age <= 17]: \"Too young\"\n};\n")
	// expectPrintedFlow(t, "const fn = async (a?: any): Promise<void> => {};\n", "const fn = async (a) => {\n};\n")
	// expectPrintedFlow(t, "// Valid lhs value inside parentheses\na ? (b) : c => d; // a ? b : (c => d)\na ? (b) : c => d : e; // a ? ((b): c => d) : e\na ? (b) : (c) : d => e; // a ? b : ((c): d => e)\n\n// Nested arrow function inside parentheses\na ? (b = (c) => d) : e => f; // a ? (b = (c) => d) : (e => f)\na ? (b = (c) => d) : e => f : g; // a ? ((b = (c) => d): e => f) : g\n\n// Nested conditional expressions\n    b ? c ? (d) : e => (f) : g : h; // b ? (c ? ((d): e => f) : g) : h\na ? b ? c ? (d) : e => (f) : g : h; // a ? (b ? (c ? d : (e => f)) : g) : h\n\na ? b ? (c) : (d) : (e) => f : g; // a ? (b ? c : ((d): e => f)) : g\n\n// Multiple arrow functions\na ? (b) : c => d : (e) : f => g; // a ? ((b): c => d) : ((e): f => g)\n\n// Multiple nested arrow functions (<T> is needed to avoid ambiguities)\na ? (b) : c => (d) : e => f : g; // a ? ((b): c => ((d): e => f)) : g\na ? (b) : c => <T>(d) : e => f; // a ? b : (c => (<T>(d): e => f))\na ? <T>(b) : c => (d) : e => f; // a ? (<T>(b): c => d) : (e => f)\n\n// Invalid lhs value inside parentheses\na ? (b => c) : d => e; // a ? (b => c) : (d => e)\na ? b ? (c => d) : e => f : g; // a ? (b ? (c => d) : (e => f)) : g\n\n// Invalid lhs value inside parentheses inside arrow function\na ? (b) : c => (d => e) : f => g; // a ? ((b): c => (d => e)) : (f => g)\na ? b ? (c => d) : e => (f => g) : h => i; // a ? (b ? (c => d) : (e => (f => g))) : (h => i)\n\n// Function as type annotation\na ? (b) : (c => d) => e : f; // a ? ((b): (c => d) => e) : f\n\n// Async functions or calls\na ? async (b) : c => d; // a ? (async(b)) : (c => d)\na ? async (b) : c => d : e; // a ? (async (b): c => d) : e\na ? async (b => c) : d => e; // a ? (async(b => c)) : (d => e)\na ? async (b) => (c => d) : e => f; // a ? (async (b) => c => d) : (e => f)\n\n// https://github.com/prettier/prettier/issues/2194\nlet icecream = what == \"cone\"\n  ? p => (!!p ? `here's your ${p} cone` : `just the empty cone for you`)\n  : p => `here's your ${p} ${what}`;\n", "a ? b : (c2) => d;\na ? (b2) => d : e;\na ? b : (c2) => e;\na ? b = (c2) => d : (e2) => f;\na ? (b2 = (c2) => d) => f : g;\nb ? c ? (d2) => f : g : h;\na ? b ? c ? d : (e2) => f : g : h;\na ? b ? c : (d2) => f : g;\na ? (b2) => d : (e2) => g;\na ? (b2) => (d2) => f : g;\na ? b : (c2) => (d2) => f;\na ? (b2) => d : (e2) => f;\na ? (b2) => c : (d2) => e;\na ? b ? (c2) => d : (e2) => f : g;\na ? (b2) => (d2) => e : (f2) => g;\na ? b ? (c2) => d : (e2) => (f2) => g : (h2) => i;\na ? (b2) => e : f;\na ? async(b) : (c2) => d;\na ? async (b2) => d : e;\na ? async((b2) => c) : (d2) => e;\na ? async (b2) => (c2) => d : (e2) => f;\nlet icecream = what == \"cone\" ? (p) => !!p ? `here's your ${p} cone` : `just the empty cone for you` : (p) => `here's your ${p} ${what}`;\n")
	// expectPrintedFlow(t, "const fail = (): X => <x />;", "const fail = () => /* @__PURE__ */ React.createElement(\"x\", null);\n")
	// expectPrintedFlow(t, "const a = async (foo: string = \"\") => {}\n", "const a = async (foo = \"\") => {\n};\n")
	//
	// // type-exports
	// expectPrintedFlow(t, "export interface foo { p: number };\nexport interface bar<T> { p: T };\n", ";\n;\n")
	// expectPrintedFlow(t, "let foo;\nexport type { foo };\n", "let foo;\n")
	// expectPrintedFlow(t, "export type a = number;\n", "")
	// expectPrintedFlow(t, "export type * from \"foo\";\n", "")
	// expectPrintedFlow(t, "export type { foo } from \"foobar\";\n", "")
	//
	// // type-alias
	// expectPrintedFlow(t, "type FBID = number;", "")
	// expectPrintedFlow(t, "type Foo<T> = Bar<T>", "")
	// expectPrintedFlow(t, "export type Foo = number;", "")
	// expectPrintedFlow(t, "type a = ??string;", "")
	// expectPrintedFlow(t, "type A = Foo<\n  | {type: \"A\"}\n  | {type: \"B\"}\n>;\n\ntype B = Foo<\n  & {type: \"A\"}\n  & {type: \"B\"}\n>;\n", "")
	// expectPrintedFlow(t, "type union =\n | {type: \"A\"}\n | {type: \"B\"}\n;\n\ntype overloads =\n  & ((x: string) => number)\n  & ((x: number) => string)\n;\n\ntype union2 = {\n  x:\n    | {type: \"A\"}\n    | {type: \"B\"}\n};\n\ntype overloads2 = {\n  x:\n    & {type: \"A\"}\n    & {type: \"B\"}\n};\n", "")
	//
	// // type-generics
	// expectPrintedFlow(t, "const functionReturningIdentityAsAField = () => ({ id: <T>(value: T): T => value });\n", "const functionReturningIdentityAsAField = () => ({\n  id: (value) => value\n});\n")
	// expectPrintedFlow(t, "const identity = <T>(t: T): T => t;\nconst a = 1;\n", "const identity = (t) => t;\nconst a = 1;\n")
	// expectPrintedFlow(t, "async <T>(fn: () => T) => fn;", "async (fn) => fn;\n")
	// expectPrintedFlow(t, "const f = async <T, R, S>(\n  x: T,\n  y: R,\n  z: S,\n) => {\n  return null;\n};\n", "const f = async (x, y, z) => {\n  return null;\n};\n")
	// expectPrintedFlow(t, "async <T>(fn: () => T);\n\n// This looks A LOT like an async arrow function, but it isn't because\n// T + U isn't a valid type parameter.\n(async <T + U>(fn: T): T => fn);\n", "async < T > fn;\nasync < T + U > fn;\n")
	// expectPrintedFlow(t, "async (...args?: any) => {};", "async (...args) => {\n};\n")
	// expectPrintedFlow(t, "async (...args: any) => {};", "async (...args) => {\n};\n")
	// expectPrintedFlow(t, "let child: Element<any> = <img src={url} key=\"img\" />;\n", "let child = /* @__PURE__ */ React.createElement(\"img\", {\n  src: url,\n  key: \"img\"\n});\n")
	//
	// // type-grouping
	// expectPrintedFlow(t, "var a: (() => number) | () => string", "var a;\n")
	// expectPrintedFlow(t, "var a: (number)", "var a;\n")
	// expectPrintedFlow(t, "var a: number & (string | bool)", "var a;\n")
	// expectPrintedFlow(t, "var a: (typeof A)", "var a;\n")
	//
	// // type-imports
	// expectPrintedFlow(t, "import type Def1 from \"foo\";\nimport type {named1} from \"foo\";\nimport type Def2, {named2} from \"foo\";\nimport type switch1 from \"foo\";\nimport type { switch2 } from \"foo\";\nimport type { foo1, bar1 } from \"baz\";\nimport type from \"foo\";\nimport typeof foo3 from \"bar\";\nimport typeof switch4 from \"foo\";\nimport typeof { switch5 } from \"foo\";\nimport typeof { foo as bar6 } from \"baz\";\nimport typeof * as ns7 from \"foo\";\nimport typeof * as switch8 from \"foo\";\n", "import type from \"foo\";\n")
	// expectPrintedFlow(t, "import type, { foo } from \"bar\";\n", "import type, {foo} from \"bar\";\n")
	// expectPrintedFlow(t, "import {type} from \"foo\";\nimport {type t} from \"foo\";\nimport {type as} from \"foo\";\nimport {type as as foo} from \"foo\";\nimport {type t as u} from \"foo\";\nimport {type switch} from \"foo\";\n\nimport {typeof t2} from \"foo\";\nimport {typeof as2} from \"foo\";\nimport {typeof t as u2} from \"foo\";\nimport {typeof switch2} from \"foo\";\n", "import {type} from \"foo\";\n")
	//
	// // type-parameter-declaration
	// expectPrintedFlow(t, "<T>() => 123;\n<T>(x) => 123;\n<T>(x: number) => 123;\n<T>(x: number) => { 123 };\n\n", "() => 123;\n(x) => 123;\n(x) => 123;\n(x) => {\n  123;\n};\n")
	// expectPrintedFlow(t, "class X {\n  foobar<T>() {}\n  delete<T>() {}\n  yield<T>() {}\n  do<T>() {}\n  static foobar<T>() {}\n  static delete<T>() {}\n  static yield<T>() {}\n  static do<T>() {}\n};\n", "class X {\n  foobar() {\n  }\n  delete() {\n  }\n  yield() {\n  }\n  do() {\n  }\n  static foobar() {\n  }\n  static delete() {\n  }\n  static yield() {\n  }\n  static do() {\n  }\n}\n;\n")
	// expectPrintedFlow(t, "<T>() => 123;\n<T>(x) => 123;\n<T>(x: number) => 123;\n<T>(x: number) => { 123 };\n\n", "() => 123;\n(x) => 123;\n(x) => 123;\n(x) => {\n  123;\n};\n")
	// expectPrintedFlow(t, "declare class X {\n  foobar<T>(): void;\n  delete<T>(): void;\n  yield<T>(): void;\n  do<T>(): void;\n  static foobar<T>(): void;\n  static delete<T>(): void;\n  static yield<T>(): void;\n  static do<T>(): void;\n};\n", ";\n")
	// expectPrintedFlow(t, "declare interface X {\n  foobar<T>(): void;\n  delete<T>(): void;\n  yield<T>(): void;\n  do<T>(): void;\n};\n", ";\n")
	// expectPrintedFlow(t, "type A1<T = string> = T\ntype A2<T = *> = T\ntype A3<T: ?string = string> = T\ntype A4<S, T: ?string = string> = T\ntype A5<S = number, T: ?string = string> = T\nclass A6<T = string> {}\nclass A7<T: ?string = string> {}\nclass A8<S, T: ?string = string> {}\nclass A9<S = number, T: ?string = string> {}\n;(class A10<T = string> {})\n;(class A11<T: ?string = string> {})\n;(class A12<S, T: ?string = string> {})\n;(class A13<S = number, T: ?string = string> {})\ndeclare class A14<T = string> {}\ndeclare class A15<T: ?string = string> {}\ndeclare class A16<S, T: ?string = string> {}\ndeclare class A17<S = number, T: ?string = string> {}\ninterface A18<T = string> {}\ninterface A19<T: ?string = string> {}\ninterface A20<S, T: ?string = string> {}\ninterface A21<S = number, T: ?string = string> {}\ntype A22<T = void> = T\nfunction A26<T = string>() {}\n;({ A28<T = string>() {} });\nclass A29 {\n  foo<T = string>() {}\n}\n;(class A30 {\n  foo<T = string>() {}\n});\ndeclare class A31 { foo<T = string>(): void }\n<T = string>() => 123;", "class A6 {\n}\nclass A7 {\n}\nclass A8 {\n}\nclass A9 {\n}\n;\n(class A10 {\n});\n(class A11 {\n});\n(class A12 {\n});\n(class A13 {\n});\nfunction A26() {\n}\n;\n({\n  A28() {\n  }\n});\nclass A29 {\n  foo() {\n  }\n}\n;\n(class A30 {\n  foo() {\n  }\n});\n() => 123;\n")
	// expectPrintedFlow(t, "interface X {\n  foobar<T>(): void;\n  delete<T>(): void;\n  yield<T>(): void;\n  do<T>(): void;\n};\n", ";\n")
	// expectPrintedFlow(t, "const x = {\n  foobar<T>() {},\n  delete<T>() {},\n  yield<T>() {},\n  do<T>() {},\n};\n", "const x = {\n  foobar() {\n  },\n  delete() {\n  },\n  yield() {\n  },\n  do() {\n  }\n};\n")
	// expectPrintedFlow(t, "const s = {\n  delete<T>(d = <Foo />) {},\n}\n", "const s = {\n  delete(d = /* @__PURE__ */ React.createElement(Foo, null)) {\n  }\n};\n")
	// expectPrintedFlow(t, "type X = {\n  foobar<T>(): void;\n  delete<T>(): void;\n  yield<T>(): void;\n  do<T>(): void;\n};\n", "")
	//
	// // typeapp-call
	// expectPrintedFlow(t, "// @flow\nasync<T>();\n", "async();\n")
	// expectPrintedFlow(t, "f<T>(e);\n", "f < T > e;\n")
	// expectPrintedFlow(t, "// @flow\nasync <T>() => {};\nasync <T>(): T => {}\n", "async () => {\n};\nasync () => {\n};\n")
	// expectPrintedFlow(t, "new C<T>(e);\n", "new C() < T > e;\n")
	// expectPrintedFlow(t, "// @flow\nf?.<T>(e);\n", "f?.(e);\n")
	// expectPrintedFlow(t, "// @flow\nf<T>(x)<U>(y);\n", "f(x)(y);\n")
	// expectPrintedFlow(t, "// @flow\no.m<T>();\n", "o.m();\n")
	// expectPrintedFlow(t, "// @flow\nf<T>();\n", "f();\n")
	// expectPrintedFlow(t, "// @flow\no[e]<T>();\n", "o[e]();\n")
	// expectPrintedFlow(t, "// @flow\no.m?.<T>(e);\n", "o.m?.(e);\n")
	// expectPrintedFlow(t, "// @flow\no?.m<T>(e);\n", "o?.m(e);\n")
	// expectPrintedFlow(t, "// @flow\nnew C<T>();\n", "new C();\n")
	// expectPrintedFlow(t, "// @flow\nnew C<T>;\n", "new C();\n")
	// expectPrintedFlow(t, "// @flow\nf<T>[e];\n", "f < T > [e];\n")
	// expectPrintedFlow(t, "// @flow\nf<T>.0;\n", "f < T > 0;\n")
	// expectPrintedFlow(t, "//@flow\ntest<\n  _,\n  _,\n  number,\n  _,\n  _,\n>();\n", "test();\n")
	// expectPrintedFlow(t, "// @flow\nf<T><U></U>;\n", "f < T > /* @__PURE__ */ React.createElement(U, null);\n")
	// expectPrintedFlow(t, "//@flow\ntest<_>();\n", "test();\n")
	// expectPrintedFlow(t, "//@flow\ntest<number, _, string, _, _, _, Foo, Bar, Baz>();\n", "test();\n")
	// expectPrintedFlow(t, "//@flow\ninstance.method()<_>();\n", "instance.method()();\n")
	// expectPrintedFlow(t, "//@flow\nnew test<_>();\n", "new test();\n")
	//
	// // typecasts
	// expectPrintedFlow(t, "({xxx: 0, yyy: \"hey\"}: {xxx: number; yyy: string})", "({\n  xxx: 0,\n  yyy: \"hey\"\n});\n")
	// expectPrintedFlow(t, "((xxx) => xxx + 1: (xxx: number) => number)", "(xxx) => xxx + 1;\n")
	// expectPrintedFlow(t, "(xxx: number)", "xxx;\n")
	// expectPrintedFlow(t, "((xxx: number), (yyy: string))", "xxx, yyy;\n")
	// expectPrintedFlow(t, "(<T>() => {}: any);\n((<T>() => {}): any);\n", "() => {\n};\n() => {\n};\n")
	// expectPrintedFlow(t, "([a: string]) => {};\n([a, [b: string]]) => {};\n([a: string] = []) => {};\n({ x: [a: string] }) => {};\n\nasync ([a: string]) => {};\nasync ([a, [b: string]]) => {};\nasync ([a: string] = []) => {};\nasync ({ x: [a: string] }) => {};\n\nlet [a1: string] = c;\nlet [a2, [b: string]] = c;\nlet [a3: string] = c;\nlet { x: [a4: string] } = c;\n", "([a]) => {\n};\n([a, [b2]]) => {\n};\n([a] = []) => {\n};\n({\n  x: [a]\n}) => {\n};\nasync ([a]) => {\n};\nasync ([a, [b2]]) => {\n};\nasync ([a] = []) => {\n};\nasync ({\n  x: [a]\n}) => {\n};\nlet [a1] = c;\nlet [a2, [b]] = c;\nlet [a3] = c;\nlet {\n  x: [a4]\n} = c;\n")
	// expectPrintedFlow(t, "(<T>() => {}: any);\n((<T>() => {}): any);\n", "() => {\n};\n() => {\n};\n")
	// expectPrintedFlow(t, "function* foo(z) {\n  const x = ((yield 3): any)\n}", "function* foo(z) {\n  const x = yield 3;\n}\n")
	// expectPrintedFlow(t, "function* foo(z) {\n  const x = (yield 3: any)\n}", "function* foo(z) {\n  const x = yield 3;\n}\n")
	//
	// // type-annotations
	// expectPrintedFlow(t, "function foo(numVal: any, otherVal: mixed){}", "function foo(numVal, otherVal) {\n}\n")
	// expectPrintedFlow(t, "function foo(callback: (_1:bool, _2:string) => number){}", "function foo(callback) {\n}\n")
	// expectPrintedFlow(t, "class Foo { bar():this { return this; }}\n", "class Foo {\n  bar() {\n    return this;\n  }\n}\n")
	// expectPrintedFlow(t, "( ...props: SomeType ) : ?ReturnType => ( 3 );\n", "(...props) => 3;\n")
	// expectPrintedFlow(t, "export default (...modifiers): Array<string> => {};\n", "export default (...modifiers) => {\n};\n")
	// expectPrintedFlow(t, "const parser = (rootPath: string, ...filesToParse: Array<string>):a => {}\n", "const parser = (rootPath, ...filesToParse) => {\n};\n")
	// expectPrintedFlow(t, "class Foo {\n  get<T>() {}\n}\n\nclass Bar {\n  set<T>() {}\n}\n", "class Foo {\n  get() {\n  }\n}\nclass Bar {\n  set() {\n  }\n}\n")
	// expectPrintedFlow(t, "function g(a:number=1, e:number=1) {}\n", "function g(a = 1, e = 1) {\n}\n")
	// expectPrintedFlow(t, "var x = ({ a } : any = 'foo') => {}\n", "var x = ({\n  a\n} = \"foo\") => {\n};\n")
	// expectPrintedFlow(t, "var a : {| x: number, y: string |} = { x: 0, y: 'foo' };\nvar b : {| x: number, y: string, |} = { x: 0, y: 'foo' };\nvar c : {| |} = {};\nvar d : { a: {| x: number, y: string |}, b: boolean } = { a: { x: 0, y: 'foo' }, b: false };\nvar e : {| a: { x: number, y: string }, b: boolean |} = { a: { x: 0, y: 'foo' }, b: false };\n", "var a = {\n  x: 0,\n  y: \"foo\"\n};\nvar b = {\n  x: 0,\n  y: \"foo\"\n};\nvar c = {};\nvar d = {\n  a: {\n    x: 0,\n    y: \"foo\"\n  },\n  b: false\n};\nvar e = {\n  a: {\n    x: 0,\n    y: \"foo\"\n  },\n  b: false\n};\n")
	// expectPrintedFlow(t, "type X = {+p:T}\n", "")
	// expectPrintedFlow(t, "type X = {-p:T}\n", "")
	// expectPrintedFlow(t, "function foo(callback: (_1:bool, ...foo:Array<number>) => number){}", "function foo(callback) {\n}\n")
	// expectPrintedFlow(t, "type X = {+[k:K]:V}\n", "")
	// expectPrintedFlow(t, "type X = {-[k:K]:V}\n", "")
	// expectPrintedFlow(t, "class A {+p:T}\n", "class A {\n  p;\n}\n")
	// expectPrintedFlow(t, "class A {-p:T}\n", "class A {\n  p;\n}\n")
	// expectPrintedFlow(t, "function foo():number{}", "function foo() {\n}\n")
	// expectPrintedFlow(t, "type A = { [string]: number };\n", "")
	// expectPrintedFlow(t, "type A = { [string | boolean]: number };\n", "")
	// expectPrintedFlow(t, "var x:\n | 1\n | 2\n= 2;\n", "var x = 2;\n")
	// expectPrintedFlow(t, "function foo():() => void{}", "function foo() {\n}\n")
	// expectPrintedFlow(t, "function x(a: | 1 | 2, b: & 3 & 4): number {}\n", "function x(a, b) {\n}\n")
	// expectPrintedFlow(t, "type A = {\n\t...any,\n};\n", "")
	// expectPrintedFlow(t, "type A = {\n\tp: {},\n\t...{},\n};\n", "")
	// expectPrintedFlow(t, "class A {}\nclass B {}\ntype C = {\n\t...A&B\n};\n", "class A {\n}\nclass B {\n}\n")
	// expectPrintedFlow(t, "function foo(): {} {}", "function foo() {\n}\n")
	// expectPrintedFlow(t, "function foo():(_?:bool) => number{}", "function foo() {\n}\n")
	// expectPrintedFlow(t, "function foo():(_:bool) => number{}", "function foo() {\n}\n")
	// expectPrintedFlow(t, "function foo<T>() {}", "function foo() {\n}\n")
	// expectPrintedFlow(t, "function foo<T,S>() {}", "function foo() {\n}\n")
	// expectPrintedFlow(t, "a=function<T,S>() {}", "a = function() {\n};\n")
	// expectPrintedFlow(t, "function foo(numVal: number){}", "function foo(numVal) {\n}\n")
	// expectPrintedFlow(t, "a={set fooProp(value:number){}}", "a = {\n  set fooProp(value) {\n  }\n};\n")
	// expectPrintedFlow(t, "a={get fooProp():number{}}", "a = {\n  get fooProp() {\n  }\n};\n")
	// expectPrintedFlow(t, "a={set fooProp(value:number):void{}}", "a = {\n  set fooProp(value) {\n  }\n};\n")
	// expectPrintedFlow(t, "a={id<T>(x: T): T {}}", "a = {\n  id(x) {\n  }\n};\n")
	// expectPrintedFlow(t, "a={*id<T>(x: T): T {}}", "a = {\n  *id(x) {\n  }\n};\n")
	// expectPrintedFlow(t, "a={async id<T>(x: T): T {}}", "a = {\n  async id(x) {\n  }\n};\n")
	// expectPrintedFlow(t, "a={123<T>(x: T): T {}}", "a = {\n  123(x) {\n  }\n};\n")
	// expectPrintedFlow(t, "class Foo {set fooProp(value:number){}}", "class Foo {\n  set fooProp(value) {\n  }\n}\n")
	// expectPrintedFlow(t, "class Foo {set fooProp(value:number):void{}}", "class Foo {\n  set fooProp(value) {\n  }\n}\n")
	// expectPrintedFlow(t, "class Foo {get fooProp():number{}}", "class Foo {\n  get fooProp() {\n  }\n}\n")
	// expectPrintedFlow(t, "function foo(numVal: number, strVal: string){}", "function foo(numVal, strVal) {\n}\n")
	// expectPrintedFlow(t, "var numVal:number;", "var numVal;\n")
	// expectPrintedFlow(t, "var numVal:number = otherNumVal;", "var numVal = otherNumVal;\n")
	// expectPrintedFlow(t, "var a: {numVal: number};", "var a;\n")
	// expectPrintedFlow(t, "var a: {numVal: number; [indexer: string]: number};", "var a;\n")
	// expectPrintedFlow(t, "var a: {numVal: number;};", "var a;\n")
	// expectPrintedFlow(t, "var a: {numVal: number; strVal: string}", "var a;\n")
	// expectPrintedFlow(t, "var a: ?{numVal: number};", "var a;\n")
	// expectPrintedFlow(t, "var a: {subObj: {strVal: string}}", "var a;\n")
	// expectPrintedFlow(t, "var a: {subObj: ?{strVal: string}}", "var a;\n")
	// expectPrintedFlow(t, "var a: {param1: number; param2: string}", "var a;\n")
	// expectPrintedFlow(t, "function foo(numVal: number, untypedVal){}", "function foo(numVal, untypedVal) {\n}\n")
	// expectPrintedFlow(t, "var a: {param1: number; param2?: string}", "var a;\n")
	// expectPrintedFlow(t, "var a: { [a: number]: string; [b: number]: string; };", "var a;\n")
	// expectPrintedFlow(t, "var a: { id<T>(x: T): T; }", "var a;\n")
	// expectPrintedFlow(t, "var a:Array<number> = [1, 2, 3]", "var a = [1, 2, 3];\n")
	// expectPrintedFlow(t, "var a: {add(x:number, ...y:Array<string>): void}", "var a;\n")
	// expectPrintedFlow(t, "a = class Foo<T> { }", "a = class Foo {\n};\n")
	// expectPrintedFlow(t, "class Foo<T> {}", "class Foo {\n}\n")
	// expectPrintedFlow(t, "a = class Foo<T> extends Bar<T> { }", "a = class Foo extends Bar {\n};\n")
	// expectPrintedFlow(t, "class Foo<T> extends Bar<T> { }", "class Foo extends Bar {\n}\n")
	// expectPrintedFlow(t, "class Foo<T> extends mixin(Bar) { }", "class Foo extends mixin(Bar) {\n}\n")
	// expectPrintedFlow(t, "class Foo { \"bar\"<T>() { } }", "class Foo {\n  bar() {\n  }\n}\n")
	// expectPrintedFlow(t, "class Foo<T> { bar<U>():number { return 42; }}", "class Foo {\n  bar() {\n    return 42;\n  }\n}\n")
	// expectPrintedFlow(t, "function foo(untypedVal, numVal: number){}", "function foo(untypedVal, numVal) {\n}\n")
	// expectPrintedFlow(t, "function foo(requiredParam, optParam?) {}", "function foo(requiredParam, optParam) {\n}\n")
	// expectPrintedFlow(t, "class Foo { static prop1:string; prop2:number; }", "class Foo {\n  static prop1;\n  prop2;\n}\n")
	// expectPrintedFlow(t, "class Foo { prop1:string; prop2:number; }", "class Foo {\n  prop1;\n  prop2;\n}\n")
	// expectPrintedFlow(t, "var x : number | string = 4;", "var x = 4;\n")
	// expectPrintedFlow(t, "var x : () => number | () => string = fn;", "var x = fn;\n")
	// expectPrintedFlow(t, "var x: typeof Y = Y;", "var x = Y;\n")
	// expectPrintedFlow(t, "var x: typeof Y | number = Y;", "var x = Y;\n")
	// expectPrintedFlow(t, "class Array { concat(items:number | string) {}; }", "class Array {\n  concat(items) {\n  }\n}\n")
	// expectPrintedFlow(t, "function foo(nullableNum: ?number){}", "function foo(nullableNum) {\n}\n")
	// expectPrintedFlow(t, "var {x}: {x: string } = { x: \"hello\" };", "var {x} = {x: \"hello\"\n};\n")
	// expectPrintedFlow(t, "var [x]: Array<string> = [ \"hello\" ];", "var [x] = [\"hello\"];\n")
	// expectPrintedFlow(t, "var {x}: {x: string; } = { x: \"hello\" };", "var {x} = {x: \"hello\"};\n")
	// expectPrintedFlow(t, "function foo({x}: { x: string; }) {}", "function foo({x}) {\n}\n")
	// expectPrintedFlow(t, "function foo([x]: Array<string>) {}", "function foo([x]) {\n}\n")
	// expectPrintedFlow(t, "function foo(...rest: Array<number>) {}", "function foo(...rest) {\n}\n")
	// expectPrintedFlow(t, "(function (...rest: Array<number>) {})", "(function(...rest) {\n});\n")
	// expectPrintedFlow(t, "var foo = (bar): number => bar;", "var foo = (bar) => bar;\n")
	// expectPrintedFlow(t, "var foo = (): number => bar;", "var foo = () => bar;\n")
	// expectPrintedFlow(t, "function foo(callback: () => void){}", "function foo(callback) {\n}\n")
	// expectPrintedFlow(t, "var foo = async (foo: bar, bar: foo) => {}", "var foo = async (foo2, bar) => {\n};\n")
	// expectPrintedFlow(t, "var foo = async (): number => bar;", "var foo = async () => bar;\n")
	// expectPrintedFlow(t, "var foo = async (bar): number => bar;", "var foo = async (bar) => bar;\n")
	// expectPrintedFlow(t, "var foo = ((): number => bar);", "var foo = () => bar;\n")
	// expectPrintedFlow(t, "var foo = ((bar): number => bar);", "var foo = (bar) => bar;\n")
	// expectPrintedFlow(t, "var foo = (((bar): number => bar): number);", "var foo = (bar) => bar;\n")
	// expectPrintedFlow(t, "var foo = (async (): number => bar);", "var foo = async () => bar;\n")
	// expectPrintedFlow(t, "var foo = ((async (bar): number => bar): number);", "var foo = async (bar) => bar;\n")
	// expectPrintedFlow(t, "var foo = (async (bar): number => bar);", "var foo = async (bar) => bar;\n")
	// expectPrintedFlow(t, "var foo = bar ? (foo) : number;", "var foo = bar ? foo : number;\n")
	// expectPrintedFlow(t, "var foo = bar ? (foo) : number => {} : baz;\n", "var foo = bar ? (foo2) => {\n} : baz;\n")
	// expectPrintedFlow(t, "function foo(callback: () => number){}", "function foo(callback) {\n}\n")
	// expectPrintedFlow(t, "((...rest: Array<number>) => rest)", "(...rest) => rest;\n")
	// expectPrintedFlow(t, "var a: Map<string, Array<string>>", "var a;\n")
	// expectPrintedFlow(t, "var a: Map<string, Array<string> >", "var a;\n")
	// expectPrintedFlow(t, "var a: ?string[]", "var a;\n")
	// expectPrintedFlow(t, "var a: Promise<bool>[]", "var a;\n")
	// expectPrintedFlow(t, "var a: number[]", "var a;\n")
	// expectPrintedFlow(t, "var a:(...rest:Array<number>) => number", "var a;\n")
	// expectPrintedFlow(t, "var identity: <T>(x: T) => T", "var identity;\n")
	// expectPrintedFlow(t, "({f: function <T>() {}})\n", "({f: function() {\n}});\n")
	// expectPrintedFlow(t, "var identity: <T>(x: T, ...y:T[]) => T", "var identity;\n")
	// expectPrintedFlow(t, "function foo(callback: (_:bool) => number){}", "function foo(callback) {\n}\n")
	// expectPrintedFlow(t, "var a: {param1?: number; param2: string; param3: string;}\n", "var a;\n")
	// expectPrintedFlow(t, "const x = (foo: string)\n: number => {}\n", "const x = (foo) => {\n};\n")
	// expectPrintedFlow(t, "(foo, bar): z => null\n", "(foo, bar) => null;\n")
	// expectPrintedFlow(t, "// bounds\ntype T1 = any;\ntype T2 = mixed;\ntype T3 = empty;\n// builtins\ntype T4 = void;\ntype T5 = number;\ntype T6 = string;\ntype T7 = bool;\ntype T8 = boolean;\n// literal type annotations\ntype T9 = null;\ntype T10 = \"\";\ntype T11 = 0;\ntype T12 = true;\ntype T13 = false;\n", "")
	// expectPrintedFlow(t, "let f : * = (x : null | *) : (*) => {}\n", "let f = (x) => {\n};\n")
	// expectPrintedFlow(t, "type Maybe<T> = _Maybe<T, *>;\n", "")
	// expectPrintedFlow(t, "<bar x={function (x): Array<string> {}} />", "/* @__PURE__ */ React.createElement(\"bar\", {\n  x: function(x) {\n  }\n});\n")
	// expectPrintedFlow(t, "function foo(a:function) {}\n", "function foo(a) {\n}\n")
	// expectPrintedFlow(t, "/**\n * @flow\n */\n\ntype DirectionVector =\n  | -1\n  | 0\n  | 1;\n\n\nvar x:DirectionVector = -1;\nconsole.log('foo');\n", "var x = -1;\nconsole.log(\"foo\");\n")
	// expectPrintedFlow(t, "type T = { a: () => void };\ntype T1 = { a: <T>() => void };\ntype T2 = { a(): void };\ntype T3 = { a<T>(): void };\n\ntype T4 = { (): number };\ntype T5 = { <T>(x: T): number; }\n\ndeclare class T6 { foo(): number; }\ndeclare class T7 { static foo(): number; }\ndeclare class T8 { (): number }\ndeclare class T9 { static (): number }\n", "")
	// expectPrintedFlow(t, "const x: symbol = Symbol();\n", "const x = Symbol();\n")
	// expectPrintedFlow(t, "// @flow\nconst a: typeof default = \"hi\";\nconst b: typeof stuff.default = \"hi\";\n\nconst c: typeof any = \"hi\";\nconst d: typeof bool = \"hi\";\nconst e: typeof boolean = \"hi\";\nconst f: typeof empty = \"hi\";\nconst g: typeof false = \"hi\";\nconst h: typeof mixed = \"hi\";\nconst i: typeof null = \"hi\";\nconst j: typeof number = \"hi\";\nconst k: typeof string = \"hi\";\nconst l: typeof true = \"hi\";\nconst m: typeof void = \"hi\";\n", "const a = \"hi\";\nconst b = \"hi\";\nconst c = \"hi\";\nconst d = \"hi\";\nconst e = \"hi\";\nconst f = \"hi\";\nconst g = \"hi\";\nconst h = \"hi\";\nconst i = \"hi\";\nconst j = \"hi\";\nconst k = \"hi\";\nconst l = \"hi\";\nconst m = \"hi\";\n")
	// expectPrintedFlow(t, "function x(foo: string = \"1\") {}\n", "function x(foo = \"1\") {\n}\n")
}

func TestPortedFlowTests(t *testing.T) {
	// // aliases
	// expectPrintedFlow(t, "type x = (string);\n", "")
	// expectPrintedFlow(t, "type x = (string)\n", "")
	// expectPrintedFlow(t, "type\nFoo = {};\n", "type;\nFoo = {};\n")
	// expectPrintedFlow(t, "type FBID = number;\n", "")
	// expectPrintedFlow(t, "type FBID = number\n", "")
	// expectPrintedFlow(t, "type Arr<T> = Array<T>;\n", "")
	// expectPrintedFlow(t, "type union = | A | B | C\n", "")
	// expectPrintedFlow(t, "type overloads = & ((x: string) => number) & ((x: number) => string);\n", "")
	//
	// // annotations_in_comments
	// expectPrintedFlow(t, "function foo(numVal/*: number*/, x/* : number*/){}\n", "function foo(numVal, x) {\n}\n")
	// expectPrintedFlow(t, "function foo(a/* :function*/, b/*  : switch*/){}\n", "function foo(a, b) {\n}\n")
	// expectPrintedFlow(t, "function foo(numVal/*::: number*/, strVal/*:: :string*/){}\n", "function foo(numVal, strVal) {\n}\n")
	// expectPrintedFlow(t, "function foo(numVal/*  :: : number*/, untypedVal){}\n", "function foo(numVal, untypedVal) {\n}\n")
	// expectPrintedFlow(t, "function foo(untypedVal, numVal/*flow-include: number*/){}\n", "function foo(untypedVal, numVal) {\n}\n")
	// expectPrintedFlow(t, "function foo(nullableNum/*flow-include : ?number*/){}\n", "function foo(nullableNum) {\n}\n")
	// expectPrintedFlow(t, "function foo(callback/* flow-include : () => void*/){}\n", "function foo(callback) {\n}\n")
	// expectPrintedFlow(t, "function foo(callback/*: () => number*/){}\n", "function foo(callback) {\n}\n")
	// expectPrintedFlow(t, "function foo(callback/*: (_:bool) => number*/){}\n", "function foo(callback) {\n}\n")
	// expectPrintedFlow(t, "function foo(callback/*: (_1:bool, _2:string) => number*/){}\n", "function foo(callback) {\n}\n")
	// expectPrintedFlow(t, "function foo()/*:() => void*/{}\n", "function foo() {\n}\n")
	// expectPrintedFlow(t, "function foo()/*:number*/{}\n", "function foo() {\n}\n")
	// expectPrintedFlow(t, "/*::\ntype duck = {\n  quack(): string;\n};\n*/\n", "")
	// expectPrintedFlow(t, "/*flow-include\ntype duck = {\n  quack(): string;\n};\n*/\n", "")
	// expectPrintedFlow(t, "function foo/*:: <T> */(x /*: T */)/*: T */ { return x; }\n", "function foo(x) {\n  return x;\n}\n")
	// expectPrintedFlow(t, "/*::type F = /* inner escaped comment *-/ number;*/\n", "")
	// expectPrintedFlow(t, "/*flow-include type F = /* inner escaped comment *-/ number;*/\n", "")
	// expectPrintedFlow(t, "var a/*: /* inner escaped comment *-/ number*/;\n", "var a;\n")
	//
	// // annotations_in_comments_types_disabled
	// expectPrintedFlow(t, "function foo(numVal/*  : number */){}\n", "function foo(numVal) {\n}\n")
	//
	// // boolean_literal
	// expectPrintedFlow(t, "var a: true\n", "var a;\n")
	// expectPrintedFlow(t, "var a: false\n", "var a;\n")
	//
	// // bigint_literal
	// expectPrintedFlow(t, "var a: 123n\n", "var a;\n")
	// expectPrintedFlow(t, "var a: 0x7Bn\n", "var a;\n")
	// expectPrintedFlow(t, "var a: 0b1111011n\n", "var a;\n")
	// expectPrintedFlow(t, "var a: 0o173n\n", "var a;\n")
	// expectPrintedFlow(t, "var a: -123n\n", "var a;\n")
	// expectPrintedFlow(t, "var a: - 123n\n", "var a;\n")
	// expectPrintedFlow(t, "var a: 25257156155n\n", "var a;\n")
	// expectPrintedFlow(t, "var a: 0x5E1719E3Bn\n", "var a;\n")
	// expectPrintedFlow(t, "var a: 0o274134317073n\n", "var a;\n")
	//
	// // class_property_variance
	// expectPrintedFlow(t, "class C {p:T}\n", "class C {\n  p;\n}\n")
	// expectPrintedFlow(t, "class C {+p:T}\n", "class C {\n  p;\n}\n")
	// expectPrintedFlow(t, "class C {-p:T}\n", "class C {\n  p;\n}\n")
	//
	// // declare_class_properties
	// expectPrintedFlow(t, "class C {\n  declare x: string;\n  declare static y: string;\n}\n", "class C {\n}\n")
	// expectPrintedFlow(t, "class C {\n  declare x;\n}\n", "class C {\n}\n")
	//
	// // declare_class
	// expectPrintedFlow(t, "declare class A {}\n", "")
	// expectPrintedFlow(t, "declare class A { static : number }\n", "")
	// expectPrintedFlow(t, "declare class A implements B {}\n", "")
	// expectPrintedFlow(t, "declare class A mixins B implements C {}\n", "")
	// expectPrintedFlow(t, "declare class A implements B, C {}\n", "")
	// expectPrintedFlow(t, "declare class Foo {\n  m(): this;\n}\n", "")
	// expectPrintedFlow(t, "declare class A mixins B {}\n", "")
	// expectPrintedFlow(t, "declare class A mixins B, C {}\n", "")
	// expectPrintedFlow(t, "declare class A {\n  proto: T;\n}\n\ndeclare class B {\n  proto x: T;\n}\n\ndeclare class C {\n  proto +x: T;\n}\n", "")
	// expectPrintedFlow(t, "declare class A { static [ indexer: number]: string }\n", "")
	// expectPrintedFlow(t, "declare class A { static () : number }\n", "")
	//
	// // declare_module
	// expectPrintedFlow(t, "declare module A {}\n", "")
	// expectPrintedFlow(t, "declare module \"./a/b.js\" {}\n", "")
	// expectPrintedFlow(t, "declare module \"M\" { import type T from \"TM\"; }\n", "")
	// expectPrintedFlow(t, "declare module A {}\ndeclare module B {}\n", "")
	//
	// // declare_interface
	// expectPrintedFlow(t, "declare interface A {}\n", "")
	// expectPrintedFlow(t, "declare interface A<T, S> {}\n", "")
	// expectPrintedFlow(t, "declare interface A { foo: number; }\n", "")
	// expectPrintedFlow(t, "declare interface A extends B {}\n", "")
	// expectPrintedFlow(t, "declare interface A extends B, C {}\n", "")
	// expectPrintedFlow(t, "declare interface A<T> extends B<T> {}\n", "")
	// expectPrintedFlow(t, "declare interface A {numVal: number; [index: number]: string};\n", ";\n")
	// expectPrintedFlow(t, "declare interface A {[index: number]: string; [index2: string]: number};\n", ";\n")
	//
	// // declare_module_exports
	// expectPrintedFlow(t, "declare module.exports: number\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare module.exports: number; }\n", "")
	//
	// // annotations
	// expectPrintedFlow(t, "type T = {...};\ntype U = {x: number, ...};\ntype V = {x: number, ...V, ...U, ...};\n", "")
	// expectPrintedFlow(t, "type T = { ..., }\ntype U = { ...; }\ntype V = {\n  x: number,\n  ...,\n}\ntype W = {\n  x: number;\n  ...;\n}\n", "")
	// expectPrintedFlow(t, "function foo(numVal: number, x: number){}\n", "function foo(numVal, x) {\n}\n")
	// expectPrintedFlow(t, "function foo(a: function, b: switch){}\n", "function foo(a, b) {\n}\n")
	// expectPrintedFlow(t, "function foo(numVal: number, strVal: string){}\n", "function foo(numVal, strVal) {\n}\n")
	// expectPrintedFlow(t, "function foo(numVal: number, untypedVal){}\n", "function foo(numVal, untypedVal) {\n}\n")
	// expectPrintedFlow(t, "function foo(untypedVal, numVal: number){}\n", "function foo(untypedVal, numVal) {\n}\n")
	// expectPrintedFlow(t, "function foo(nullableNum: ?number){}\n", "function foo(nullableNum) {\n}\n")
	// expectPrintedFlow(t, "function foo(callback: () => void){}\n", "function foo(callback) {\n}\n")
	// expectPrintedFlow(t, "function foo(callback: () => number){}\n", "function foo(callback) {\n}\n")
	// expectPrintedFlow(t, "function foo(callback: (_:bool) => number){}\n", "function foo(callback) {\n}\n")
	// expectPrintedFlow(t, "function foo(callback: (_1:bool, _2:string) => number){}\n", "function foo(callback) {\n}\n")
	// expectPrintedFlow(t, "function foo():number{}\n", "function foo() {\n}\n")
	// expectPrintedFlow(t, "function foo():() => void{}\n", "function foo() {\n}\n")
	// expectPrintedFlow(t, "function foo():(_:bool) => number{}\n", "function foo() {\n}\n")
	// expectPrintedFlow(t, "function foo(): {} {}\n", "function foo() {\n}\n")
	// expectPrintedFlow(t, "function foo<T>() {}\n", "function foo() {\n}\n")
	// expectPrintedFlow(t, "function foo<T,S>() {}\n", "function foo() {\n}\n")
	// expectPrintedFlow(t, "function foo(...typedRest: Array<number>){}\n", "function foo(...typedRest) {\n}\n")
	// expectPrintedFlow(t, "a=function<T,S>() {}\n", "a = function() {\n};\n")
	// expectPrintedFlow(t, "a={set fooProp(value:number){}}\n", "a = {\n  fooProp(value) {\n  }\n};\n")
	// expectPrintedFlow(t, "var numVal:number;\n", "var numVal;\n")
	// expectPrintedFlow(t, "var numVal:number = otherNumVal;\n", "var numVal = otherNumVal;\n")
	// expectPrintedFlow(t, "var a: ?{numVal: number};\n", "var a;\n")
	// expectPrintedFlow(t, "var a: {numVal: number};\n", "var a;\n")
	// expectPrintedFlow(t, "var a: {numVal: number; [index: number]: string};\n", "var a;\n")
	// expectPrintedFlow(t, "var a: {[index: number]: string; [index2: string]: number};\n", "var a;\n")
	// expectPrintedFlow(t, "var a: {subObj: {strVal: string}}\n", "var a;\n")
	// expectPrintedFlow(t, "var a: {param1: number; param2: string}\n", "var a;\n")
	// expectPrintedFlow(t, "var a: {param1: number; param2?: string}\n", "var a;\n")
	// expectPrintedFlow(t, "var a: { add(...rest:Array<number>): number; }\n", "var a;\n")
	// expectPrintedFlow(t, "var a: { get foo(): number; }\n", "var a;\n")
	// expectPrintedFlow(t, "var a: { set foo(x: number): void; }\n", "var a;\n")
	// expectPrintedFlow(t, "var a: { get foo(): number; set foo(x: number): void; }\n", "var a;\n")
	// expectPrintedFlow(t, "var a: {get?: number; set?: string}\n", "var a;\n")
	// expectPrintedFlow(t, "var a:(...rest:Array<number>) => number\n", "var a;\n")
	// expectPrintedFlow(t, "var bar: (str:number, i:number)=> string = foo;\n", "var bar = foo;\n")
	// expectPrintedFlow(t, "var a:Array<number> = [1, 2, 3]\n", "var a = [1, 2, 3];\n")
	// expectPrintedFlow(t, "function foo(requiredParam, optParam?) {}\n", "function foo(requiredParam, optParam) {\n}\n")
	// expectPrintedFlow(t, "function foo(requiredParam, optParam?=123) {}\n", "function foo(requiredParam, optParam = 123) {\n}\n")
	// expectPrintedFlow(t, "class Foo {set fooProp(value:number){}}\n", "class Foo {\n  set fooProp(value) {\n  }\n}\n")
	// expectPrintedFlow(t, "a = class Foo<T> { }\n", "a = class Foo {\n};\n")
	// expectPrintedFlow(t, "class Foo<T> {}\n", "class Foo {\n}\n")
	// expectPrintedFlow(t, "class Foo<T> { bar<U>():number { return 42; }}\n", "class Foo {\n  bar() {\n    return 42;\n  }\n}\n")
	// expectPrintedFlow(t, "class Foo { \"bar\"<T>() { } }\n", "class Foo {\n  bar() {\n  }\n}\n")
	// expectPrintedFlow(t, "class Foo { prop1:string; prop2:number; }\n", "class Foo {\n  prop1;\n  prop2;\n}\n")
	// expectPrintedFlow(t, "class Foo { static prop: number; }\n", "class Foo {\n  static prop;\n}\n")
	// expectPrintedFlow(t, "class Foo { \"prop1\":string; }\n", "class Foo {\n  prop1;\n}\n")
	// expectPrintedFlow(t, "class Foo { 123:string; }\n", "class Foo {\n  123;\n}\n")
	// expectPrintedFlow(t, "class Foo { [prop1]: string; }\n", "class Foo {\n  [prop1];\n}\n")
	// expectPrintedFlow(t, "class Foo { [1 + 1]: string; }\n", "class Foo {\n  [1 + 1];\n}\n")
	// expectPrintedFlow(t, "class Foo<T> extends Bar<T> {}\n", "class Foo extends Bar {\n}\n")
	// expectPrintedFlow(t, "a = class Foo<T> extends Bar<T> {}\n", "a = class Foo extends Bar {\n};\n")
	// expectPrintedFlow(t, "class Foo<+T1,-T2> {}\n", "class Foo {\n}\n")
	// expectPrintedFlow(t, "var {x}: {x: string; } = { x: \"hello\" };\n", "var {x} = {x: \"hello\"};\n")
	// expectPrintedFlow(t, "var [x]: Array<string> = [ \"hello\" ];\n", "var [x] = [\"hello\"];\n")
	// expectPrintedFlow(t, "function foo({x}: { x: string; }) {}\n", "function foo({x}) {\n}\n")
	// expectPrintedFlow(t, "function foo([x]: Array<string>) {}\n", "function foo([x]) {\n}\n")
	// expectPrintedFlow(t, "var x : number | string = 4;\n", "var x = 4;\n")
	// expectPrintedFlow(t, "interface Array<T> { concat(...items: Array<Array<T> | T>): Array<T>; }\n", "")
	// expectPrintedFlow(t, "var x : () => number | () => string = fn;\n", "var x = fn;\n")
	// expectPrintedFlow(t, "var x: typeof Y = Y;\n", "var x = Y;\n")
	// expectPrintedFlow(t, "var x: typeof Y | number = Y;\n", "var x = Y;\n")
	// expectPrintedFlow(t, "var x : number & string = 4;\n", "var x = 4;\n")
	// expectPrintedFlow(t, "interface Array<T> { concat(...items: Array<Array<T> & T>): Array<T>; }\n", "")
	// expectPrintedFlow(t, "var x : () => number & () => string = fn;\n", "var x = fn;\n")
	// expectPrintedFlow(t, "var x: typeof Y & number = Y;\n", "var x = Y;\n")
	// expectPrintedFlow(t, "var identity: <T>(x: T) => T\n", "var identity;\n")
	// expectPrintedFlow(t, "var a: (number: any, string: any, any: any, type: any) => any;\n", "var a;\n")
	// expectPrintedFlow(t, "var a: {[type: any]: any}\n", "var a;\n")
	// expectPrintedFlow(t, "type foo<A,B,> = bar;\n", "")
	// expectPrintedFlow(t, "class Foo<A,B,> extends Bar<C,D,> {}\n", "class Foo extends Bar {\n}\n")
	// expectPrintedFlow(t, "interface Foo<A,B,> {}\n", "")
	// expectPrintedFlow(t, "function f<A,B,>() {}\n", "function f() {\n}\n")
	// expectPrintedFlow(t, "type Foo = Array<*>\n", "")
	// expectPrintedFlow(t, "type T = { a: | number | string }\n", "")
	// expectPrintedFlow(t, "declare var x: symbol;\n", "")
	// expectPrintedFlow(t, "test<\n  _,\n  _,\n  number,\n  _,\n  _,\n>();\n", "test();\n")
	// expectPrintedFlow(t, "test<number, _, string, _, _, _, Foo, Bar, Baz>();\n", "test();\n")
	// expectPrintedFlow(t, "test<_>();\n", "test();\n")
	// expectPrintedFlow(t, "new test<_>();\n", "new test();\n")
	// expectPrintedFlow(t, "instance.method()<_>();\n", "instance.method()();\n")
	//
	// // declare_type_alias
	// expectPrintedFlow(t, "declare type x = number;\n", "")
	//
	// // declare_statements_invalid
	// expectPrintedFlow(t, "declare class A { static implements: number; implements: number }\n", "")
	//
	// // declare_module_with_exports
	// expectPrintedFlow(t, "declare module Foo {\n  declare export type foo = (string);\n}\n", "")
	// expectPrintedFlow(t, "declare module Foo {\n  declare export type foo = (string)\n}\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export * from \"bar\"; }\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export {a,} from \"bar\"; }\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export {a,}; }\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export var a: number; }\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export function bar(p1: number): string; }\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export class Foo { meth(p1: number): void; } }\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export type bar = number; }\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export type bar = number; declare export var baz: number; }\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export type bar = number; declare module.exports: number; }\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export interface bar {} }\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export interface bar {} declare export var baz: number; }\n", "")
	// expectPrintedFlow(t, "declare module \"foo\" { declare export interface bar {} declare module.exports: number; }\n", "")
	//
	// // exact_objects
	// expectPrintedFlow(t, "var obj: {| x: number, y: string |} // no trailing comma\n", "var obj;\n")
	// expectPrintedFlow(t, "var obj: {| x: number, y: string, |} // trailing comma\n", "var obj;\n")
	//
	// // declare_statements
	// expectPrintedFlow(t, "declare class A { static : number }\n", "")
	// expectPrintedFlow(t, "declare var foo\n", "")
	// expectPrintedFlow(t, "declare var foo;\n", "")
	// expectPrintedFlow(t, "declare function foo(): void\n", "")
	// expectPrintedFlow(t, "declare function foo(): void;\n", "")
	// expectPrintedFlow(t, "declare function foo(x: number, y: string): void;\n", "")
	// // Function argument types are now optional
	// expectPrintedFlow(t, "declare function foo(x: number, string): void\n", "")
	// expectPrintedFlow(t, "declare class A {}\n", "")
	// expectPrintedFlow(t, "declare class A<T> extends B<T> { x: number }\n", "")
	// expectPrintedFlow(t, "declare class A { static foo(): number; static x : string; }\n", "")
	// expectPrintedFlow(t, "declare class A { static () : number }\n", "")
	// expectPrintedFlow(t, "declare class A { static [ indexer: number]: string }\n", "")
	// expectPrintedFlow(t, "declare class A { get foo(): number; }\n", "")
	// expectPrintedFlow(t, "declare class A { set foo(x: number): void; }\n", "")
	// expectPrintedFlow(t, "declare class A { get foo(): number; set foo(x: string): void; }\n", "")
	//
	// // function_predicates
	// expectPrintedFlow(t, "declare function f(x: mixed): boolean %checks(x !== null);\n", "")
	// expectPrintedFlow(t, "declare function f(x: mixed): boolean %checks(x !== null)\n", "")
	// expectPrintedFlow(t, "function foo(x: mixed): %checks { return x !== null }\n", "function foo(x) {\n  return x !== null;\n}\n")
	// expectPrintedFlow(t, "var a1 = (x: mixed): %checks => x !== null;\n", "var a1 = (x) => x !== null;\n")
	// expectPrintedFlow(t, "(x): %checks => x !== null;\n", "(x) => x !== null;\n")
	//
	// // grouping
	// expectPrintedFlow(t, "type A = (B) => (C)\n", "")
	// expectPrintedFlow(t, "type A = (b: (B)) => C\n", "")
	// expectPrintedFlow(t, "type A = B & (C)\n", "")
	// expectPrintedFlow(t, "var a: number & (string | bool)\n", "var a;\n")
	// expectPrintedFlow(t, "type A = {\n  b(): (B)\n}\n", "")
	// expectPrintedFlow(t, "var a: (number)\n", "var a;\n")
	// expectPrintedFlow(t, "var a: (() => number) | () => string\n", "var a;\n")
	// expectPrintedFlow(t, "var a: (typeof A)\n", "var a;\n")
	// expectPrintedFlow(t, "var a: Array<(number)>\n", "var a;\n")
	// expectPrintedFlow(t, "var a: ([]) = []\n", "var a = [];\n")
	// expectPrintedFlow(t, "var a: (number: number) => number = (number) => { return 123; }\n", "var a = (number) => {\n  return 123;\n};\n")
	// expectPrintedFlow(t, "type A = ?(?B)\n", "")
	// expectPrintedFlow(t, "type A = {\n  (): (B)\n}\n", "")
	// expectPrintedFlow(t, "type A = {\n  [B]: (C)\n}\n", "")
	// expectPrintedFlow(t, "type A = {\n  b: (B)\n}\n", "")
	// expectPrintedFlow(t, "type A = {\n  ...(B)\n}\n", "")
	// expectPrintedFlow(t, "type A = typeof (B)\n", "")
	// expectPrintedFlow(t, "type A = B | (C)\n", "")
	//
	// // import_types
	// // ok, because `switch` is not a reserved type name
	// expectPrintedFlow(t, "import type switch from 'foo';\n", "")
	// // ok, reserved words are not reserved types
	// expectPrintedFlow(t, "import type { switch } from 'foo';\n", "")
	// // ok, `string` is a reserved type but it's renamed
	// expectPrintedFlow(t, "import typeof { string as StringT } from 'foo';\n", "")
	//
	// // import_type_shorthand
	// expectPrintedFlow(t, "import {type} from \"foo\";\n", "import {type} from \"foo\";\n")
	// expectPrintedFlow(t, "import {type t} from \"foo\";\n", "")
	// expectPrintedFlow(t, "import {type as} from \"foo\";\n", "")
	// expectPrintedFlow(t, "import {type t as u} from \"foo\";\n", "")
	// expectPrintedFlow(t, "import {typeof t} from \"foo\";\n", "")
	// expectPrintedFlow(t, "import {typeof as} from \"foo\";\n", "")
	// expectPrintedFlow(t, "import {typeof t as u} from \"foo\";\n", "")
	// // ok, a type named `as`, renamed to `x`
	// expectPrintedFlow(t, "import { type as as x } from \"ModuleName\";\n", "")
	// // ok, `switch` is a reserved value but not reserved type name
	// expectPrintedFlow(t, "import { typeof switch } from \"foo\";\n", "")
	//
	// // function_types_with_anonymous_parameters
	// expectPrintedFlow(t, "type T = Array<(string) => number>\n", "")
	// expectPrintedFlow(t, "let x = (): Array<(string) => number> => []\n", "let x = () => [];\n")
	// expectPrintedFlow(t, "type A = (string) => void\n", "")
	// expectPrintedFlow(t, "type A = (string,) => void\n", "")
	// expectPrintedFlow(t, "type A = (Array<string>) => void\n", "")
	// expectPrintedFlow(t, "type A = (Array<string>,) => void\n", "")
	// expectPrintedFlow(t, "type A = (x: string, number) => void\n", "")
	// expectPrintedFlow(t, "type A = (...Array<string>) => void\n", "")
	// expectPrintedFlow(t, "type A = (Array<string>, ...Array<string>) => void\n", "")
	// // Non-anonymous function types are allowed as arrow function return types
	// expectPrintedFlow(t, "var f = (x): (x: number) => 123 => 123;\n", "var f = (x) => 123;\n")
	// // Anonymous function types are disallowed as arrow function return types
	// // So the `=>` clearly belongs to the arrow function
	// expectPrintedFlow(t, "var f = (): (number) => 123;\n", "var f = () => 123;\n")
	// expectPrintedFlow(t, "var f = (): string | (number) => 123;\n", "var f = () => 123;\n")
	// // You can write anonymous function types as arrow function return types
	// // if you wrap them in parens
	// expectPrintedFlow(t, "var f = (x): ((number) => 123) => 123;\n", "var f = (x) => 123;\n")
	// expectPrintedFlow(t, "type A = string => void\n", "")
	// expectPrintedFlow(t, "type A = Array<string> => void\n", "")
	// // Anonymous function types are disallowed as arrow function return types
	// // So the `=>` clearly belongs to the arrow function
	// expectPrintedFlow(t, "var f = (): number => 123;\n", "var f = () => 123;\n")
	// // Anonymous function types are disallowed as arrow function return types
	// // So the `=>` clearly belongs to the arrow function
	// expectPrintedFlow(t, "var f = (): string | number => 123;\n", "var f = () => 123;\n")
	// // You can write anonymous function types as arrow function return types
	// // if you wrap them in parens
	// expectPrintedFlow(t, "var f = (x): (number => 123) => 123;\n", "var f = (x) => 123;\n")
	// // string | (number => boolean)
	// expectPrintedFlow(t, "type A = string | number => boolean;\n", "")
	// // string & (number => boolean)
	// expectPrintedFlow(t, "type A = string & number => boolean;\n", "")
	// // (?number) => boolean
	// expectPrintedFlow(t, "type A = ?number => boolean;\n", "")
	// // (number[]) => boolean
	// expectPrintedFlow(t, "type A = number[] => boolean;\n", "")
	// expectPrintedFlow(t, "type A = (string => boolean) => number\n", "")
	// // string => (boolean | number)
	// expectPrintedFlow(t, "type A = string => boolean | number;\n", "")
	// // Becomes string => (boolean => number)
	// expectPrintedFlow(t, "type A = string => boolean => number;\n", "")
	// expectPrintedFlow(t, "type T = (arg: string, number) => void\n", "")
	//
	// // member
	// expectPrintedFlow(t, "var a : A.B\n", "var a;\n")
	// expectPrintedFlow(t, "var a : A.B.C\n", "var a;\n")
	// expectPrintedFlow(t, "var a : A.B<T>\n", "var a;\n")
	// expectPrintedFlow(t, "var a : typeof A.B<T>\n", "var a;\n")
	// expectPrintedFlow(t, "var a: function.switch\n", "var a;\n")
	//
	// // keyword_variable_collision
	// expectPrintedFlow(t, "type arguments = string\n", "")
	// expectPrintedFlow(t, "\"use strict\";\ntype arguments = string\n", "\"use strict\";\n")
	// expectPrintedFlow(t, "type eval = string\n", "")
	// expectPrintedFlow(t, "\"use strict\";\ntype eval = string\n", "\"use strict\";\n")
	// expectPrintedFlow(t, "opaque type opaque = number;\nopaque type not_transparent = opaque;\n", "")
	// expectPrintedFlow(t, "var opaque = 0;\nopaque += 4;\n", "var opaque = 0;\nopaque += 4;\n")
	// expectPrintedFlow(t, "opaque(0);\n", "opaque(0);\n")
	//
	// // interfaces
	// // ok, `implements` refers to types so reserved values are fine
	// expectPrintedFlow(t, "class Foo implements switch {}\n", "class Foo {\n}\n")
	// expectPrintedFlow(t, "interface A {}\n", "")
	// expectPrintedFlow(t, "interface A<T, S> {}\n", "")
	// expectPrintedFlow(t, "interface A { foo: number; }\n", "")
	// expectPrintedFlow(t, "interface A extends B {}\n", "")
	// expectPrintedFlow(t, "interface A extends B, C {}\n", "")
	// expectPrintedFlow(t, "interface A<T> extends B<T> {}\n", "")
	// expectPrintedFlow(t, "class Foo implements Bar {}\n", "class Foo {\n}\n")
	// expectPrintedFlow(t, "class Foo extends Bar implements Bat, Man<number> {}\n", "class Foo extends Bar {\n}\n")
	// expectPrintedFlow(t, "class Foo extends class Bar implements Bat {} {}\n", "class Foo extends class Bar {\n} {\n}\n")
	// expectPrintedFlow(t, "class Foo extends class Bar implements Bat {} implements Man {}\n", "class Foo extends class Bar {\n} {\n}\n")
	// expectPrintedFlow(t, "interface A {numVal: number; [index: number]: string};\n", ";\n")
	// expectPrintedFlow(t, "interface A {[index: number]: string; [index2: string]: number};\n", ";\n")
	// // OK: static is a valid identifier name
	// expectPrintedFlow(t, "interface A { static: number }\ninterface B { static?: number }\ninterface C { static(): void } // method named static\ninterface D { static<X>(x: X): X } // poly method named static\n", "")
	// // ok, reserved values can be valid types
	// expectPrintedFlow(t, "interface switch {}\n", "")
	//
	// // number_literal
	// expectPrintedFlow(t, "var a: 123\n", "var a;\n")
	// expectPrintedFlow(t, "var a: 123.0\n", "var a;\n")
	// expectPrintedFlow(t, "var a: 0x7B\n", "var a;\n")
	// expectPrintedFlow(t, "var a: 0b1111011\n", "var a;\n")
	// expectPrintedFlow(t, "var a: 0o173\n", "var a;\n")
	// expectPrintedFlow(t, "var a: -123\n", "var a;\n")
	// expectPrintedFlow(t, "var a: - 123\n", "var a;\n")
	// expectPrintedFlow(t, "var a: 25257156155\n", "var a;\n")
	// expectPrintedFlow(t, "var a: 0x5E1719E3B\n", "var a;\n")
	// expectPrintedFlow(t, "var a: 0o274134317073\n", "var a;\n")
	// expectPrintedFlow(t, "var a: 1e5\n", "var a;\n")
	//
	// // object_type_spread
	// expectPrintedFlow(t, "type T = {...O}\n", "")
	// expectPrintedFlow(t, "type T = {...O,}\n", "")
	// expectPrintedFlow(t, "type T = {p:T, ...O}\n", "")
	// expectPrintedFlow(t, "type T = {...O, p:T}\n", "")
	// expectPrintedFlow(t, "type T = {...O1, ...O2}\n", "")
	//
	// // optional_indexer_name
	// expectPrintedFlow(t, "type A = { [string]: number };\n", "")
	// expectPrintedFlow(t, "type A = { [string | boolean]: number };\n", "")
	//
	// // object_type_property_variance
	// expectPrintedFlow(t, "type X = {p:T}\n", "")
	// expectPrintedFlow(t, "type X = {+p:T}\n", "")
	// expectPrintedFlow(t, "type X = {-p:T}\n", "")
	// expectPrintedFlow(t, "type X = {m():T}\n", "")
	// expectPrintedFlow(t, "type X = {[k:K]:V}\n", "")
	// expectPrintedFlow(t, "type X = {+[k:K]:V}\n", "")
	// expectPrintedFlow(t, "type X = {-[k:K]:V}\n", "")
	// expectPrintedFlow(t, "type X = {():T}\n", "")
	//
	// // string_literal
	// expectPrintedFlow(t, "var a: \"duck\"\n", "var a;\n")
	// expectPrintedFlow(t, "var a: 'duck'\n", "var a;\n")
	// expectPrintedFlow(t, "var a: \"foo bar\"\n", "var a;\n")
	//
	// // tuples
	// expectPrintedFlow(t, "var a : [] = [];\n", "var a = [];\n")
	// expectPrintedFlow(t, "var a : [Foo<T>] = [foo];\n", "var a = [foo];\n")
	// expectPrintedFlow(t, "var a : [number,] = [123,];\n", "var a = [123];\n")
	// expectPrintedFlow(t, "var a : [number, string] = [123, \"duck\"];\n", "var a = [123, \"duck\"];\n")
	//
	// // typecasts
	// expectPrintedFlow(t, "(xxx: number)\n", "xxx;\n")
	// expectPrintedFlow(t, "({xxx: 0, yyy: \"hey\"}: {xxx: number; yyy: string})\n", "({\n  xxx: 0,\n  yyy: \"hey\"\n});\n")
	// // distinguish between function type params and typecasts
	// expectPrintedFlow(t, "((xxx) => xxx + 1: (xxx: number) => number)\n", "(xxx) => xxx + 1;\n")
	// // parens disambiguate groups from casts
	// expectPrintedFlow(t, "((xxx: number), (yyy: string))\n", "xxx, yyy;\n")
	//
	// // parameter_defaults
	// expectPrintedFlow(t, "type A<T = string> = T\n", "")
	// expectPrintedFlow(t, "type A<T: ?string = string> = T\n", "")
	// expectPrintedFlow(t, "type A<S, T: ?string = string> = T\n", "")
	// expectPrintedFlow(t, "type A<S = number, T: ?string = string> = T\n", "")
	// expectPrintedFlow(t, "class A<T = string> {}\n", "class A {\n}\n")
	// expectPrintedFlow(t, "class A<T: ?string = string> {}\n", "class A {\n}\n")
	// expectPrintedFlow(t, "class A<S, T: ?string = string> {}\n", "class A {\n}\n")
	// expectPrintedFlow(t, "class A<S = number, T: ?string = string> {}\n", "class A {\n}\n")
	// expectPrintedFlow(t, "(class A<T = string> {})\n", "(class A {\n});\n")
	// expectPrintedFlow(t, "(class A<T: ?string = string> {})\n", "(class A {\n});\n")
	// expectPrintedFlow(t, "(class A<S, T: ?string = string> {})\n", "(class A {\n});\n")
	// expectPrintedFlow(t, "(class A<S = number, T: ?string = string> {})\n", "(class A {\n});\n")
	// expectPrintedFlow(t, "declare class A<T = string> {}\n", "")
	// expectPrintedFlow(t, "declare class A<T: ?string = string> {}\n", "")
	// expectPrintedFlow(t, "declare class A<S, T: ?string = string> {}\n", "")
	// expectPrintedFlow(t, "declare class A<S = number, T: ?string = string> {}\n", "")
	// expectPrintedFlow(t, "interface A<T = string> {}\n", "")
	// expectPrintedFlow(t, "interface A<T: ?string = string> {}\n", "")
	// expectPrintedFlow(t, "interface A<S, T: ?string = string> {}\n", "")
	// expectPrintedFlow(t, "interface A<S = number, T: ?string = string> {}\n", "")
	// expectPrintedFlow(t, "var x: Foo<>\n", "var x;\n")
	// expectPrintedFlow(t, "class A extends B<> {}\n", "class A extends B {\n}\n")
	// expectPrintedFlow(t, "function foo<T = string>() {}\n", "function foo() {\n}\n")
	// expectPrintedFlow(t, "({ foo<T = string>() {} })\n", "({foo() {\n}});\n")
	// expectPrintedFlow(t, "class A { foo<T = string>() {} }\n", "class A {\n  foo() {\n  }\n}\n")
	// expectPrintedFlow(t, "(class A { foo<T = string>() {} })\n", "(class A {\n  foo() {\n  }\n});\n")
	// expectPrintedFlow(t, "declare class A { foo<T = string>(): void }\n", "")
	// expectPrintedFlow(t, "<T = string>() => 123\n", "() => 123;\n")
	//
	// // declare_export
	// expectPrintedFlow(t, "declare export class A { static : number }\n", "")
	//
	// // declare_export/batch
	// expectPrintedFlow(t, "declare export * from \"foo\";\n", "")
	// expectPrintedFlow(t, "declare export * from \"foo\"\n", "")
	//
	// // declare_export/class
	// expectPrintedFlow(t, "declare export class A {}\n", "")
	// expectPrintedFlow(t, "declare export class A<T> extends B<T> { x: number }\n", "")
	// expectPrintedFlow(t, "declare export class A { static foo(): number; static x : string }\n", "")
	// expectPrintedFlow(t, "declare export class A { static [ indexer: number]: string }\n", "")
	// expectPrintedFlow(t, "declare export class A { static () : number }\n", "")
	//
	// // declare_export/function
	// expectPrintedFlow(t, "declare export function foo(): void\n", "")
	// expectPrintedFlow(t, "declare export function foo(): void;\n", "")
	// expectPrintedFlow(t, "declare export function foo<T>(): void;\n", "")
	// expectPrintedFlow(t, "declare export function foo(x: number, y: string): void;\n", "")
	// expectPrintedFlow(t, "declare export function foo(x: number, string): void\n", "")
	//
	// // declare_export/default
	// expectPrintedFlow(t, "declare export default (string);\n", "")
	// expectPrintedFlow(t, "declare export default (string)\n", "")
	// expectPrintedFlow(t, "declare export default number;\n", "")
	// expectPrintedFlow(t, "declare export default number\n", "")
	// expectPrintedFlow(t, "declare export default function foo(): void\n", "")
	// expectPrintedFlow(t, "declare export default function foo(): void;\n", "")
	// expectPrintedFlow(t, "declare export default function foo<T>(): void;\n", "")
	// expectPrintedFlow(t, "declare export default function foo(x: number, y: string): void;\n", "")
	// expectPrintedFlow(t, "declare export default class A {}\n", "")
	// expectPrintedFlow(t, "declare export default class A<T> extends B<T> { x: number }\n", "")
	// expectPrintedFlow(t, "declare export default class A { static foo(): number; static x : string }\n", "")
	// expectPrintedFlow(t, "declare export default class A { static [ indexer: number]: string }\n", "")
	// expectPrintedFlow(t, "declare export default class A { static () : number }\n", "")
	//
	// // declare_export/var
	// expectPrintedFlow(t, "declare export var x\n", "")
	// expectPrintedFlow(t, "declare export var x;\n", "")
	// expectPrintedFlow(t, "declare export var x: number;\n", "")
	//
	// // declare_export/named
	// expectPrintedFlow(t, "declare export {} from \"foo\";\n", "")
	// expectPrintedFlow(t, "declare export { bar } from \"foo\";\n", "")
	// expectPrintedFlow(t, "declare export { bar } from \"foo\"\n", "")
	// expectPrintedFlow(t, "declare export { bar, baz } from \"foo\";\n", "")
	// expectPrintedFlow(t, "declare export { bar };\n", "")
	// expectPrintedFlow(t, "declare export { bar, }\n", "")
	// expectPrintedFlow(t, "declare export { bar, baz };\n", "")
	//
	// // declare_export_invalid
	// expectPrintedFlow(t, "declare export class A { static implements: number; implements: number }\n", "")
	//
	// // object/indexers
	// expectPrintedFlow(t, "type X = { [string: string]: string }\n", "")
	// expectPrintedFlow(t, "type X = { [switch: string]: string }\n", "")
	//
	// // object/methods
	// expectPrintedFlow(t, "type T = { foo<U>(x: U): number; }\n", "")
	// expectPrintedFlow(t, "type T = { foo(): number };\n", "")
	//
	// // opaque_aliases/declare
	// expectPrintedFlow(t, "declare export opaque type Foo\n", "")
	// expectPrintedFlow(t, "declare export opaque type Foo: Bar\n", "")
	// expectPrintedFlow(t, "declare opaque type Foo\n", "")
	// expectPrintedFlow(t, "declare opaque type Test: Foo\n", "")
	//
	// // opaque_aliases/valid
	// expectPrintedFlow(t, "opaque type Counter: Box<T> = Container<T>;\n", "")
	// expectPrintedFlow(t, "export opaque type Counter: Box<T> = Container<S>;\n", "")
	// expectPrintedFlow(t, "opaque type FBID = number;\n", "")
	// expectPrintedFlow(t, "export opaque type FBID = number;\n", "")
	// expectPrintedFlow(t, "opaque type switch = number;\n", "")
}
