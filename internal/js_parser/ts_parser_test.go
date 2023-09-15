package js_parser

import (
	"testing"

	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
)

func expectParseErrorTS(t *testing.T, contents string, expected string) {
	t.Helper()
	expectParseErrorCommon(t, contents, expected, config.Options{
		TS: config.TSOptions{
			Parse: true,
		},
	})
}

func expectParseErrorExperimentalDecoratorTS(t *testing.T, contents string, expected string) {
	t.Helper()
	expectParseErrorCommon(t, contents, expected, config.Options{
		TS: config.TSOptions{
			Parse: true,
			Config: config.TSConfig{
				ExperimentalDecorators: config.True,
			},
		},
	})
}

func expectParseErrorWithUnsupportedFeaturesTS(t *testing.T, unsupportedJSFeatures compat.JSFeature, contents string, expected string) {
	t.Helper()
	expectParseErrorCommon(t, contents, expected, config.Options{
		TS: config.TSOptions{
			Parse: true,
		},
		UnsupportedJSFeatures: unsupportedJSFeatures,
	})
}

func expectParseErrorTargetTS(t *testing.T, esVersion int, contents string, expected string) {
	t.Helper()
	expectParseErrorCommon(t, contents, expected, config.Options{
		TS: config.TSOptions{
			Parse: true,
		},
		UnsupportedJSFeatures: compat.UnsupportedJSFeatures(map[compat.Engine]compat.Semver{
			compat.ES: {Parts: []int{esVersion}},
		}),
	})
}

func expectPrintedTS(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		TS: config.TSOptions{
			Parse: true,
		},
	})
}

func expectPrintedAssignSemanticsTS(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		TS: config.TSOptions{
			Parse: true,
			Config: config.TSConfig{
				UseDefineForClassFields: config.False,
			},
		},
	})
}

func expectPrintedAssignSemanticsTargetTS(t *testing.T, esVersion int, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		TS: config.TSOptions{
			Parse: true,
			Config: config.TSConfig{
				UseDefineForClassFields: config.False,
			},
		},
		UnsupportedJSFeatures: compat.UnsupportedJSFeatures(map[compat.Engine]compat.Semver{
			compat.ES: {Parts: []int{esVersion}},
		}),
	})
}

func expectPrintedExperimentalDecoratorTS(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		TS: config.TSOptions{
			Parse: true,
			Config: config.TSConfig{
				ExperimentalDecorators: config.True,
			},
		},
	})
}

func expectPrintedMangleTS(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		TS: config.TSOptions{
			Parse: true,
		},
		MinifySyntax: true,
	})
}

func expectPrintedMangleAssignSemanticsTS(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		TS: config.TSOptions{
			Parse: true,
			Config: config.TSConfig{
				UseDefineForClassFields: config.False,
			},
		},
		MinifySyntax: true,
	})
}

func expectPrintedTargetTS(t *testing.T, esVersion int, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		TS: config.TSOptions{
			Parse: true,
		},
		UnsupportedJSFeatures: compat.UnsupportedJSFeatures(map[compat.Engine]compat.Semver{
			compat.ES: {Parts: []int{esVersion}},
		}),
	})
}

func expectPrintedTargetExperimentalDecoratorTS(t *testing.T, esVersion int, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		TS: config.TSOptions{
			Parse: true,
			Config: config.TSConfig{
				ExperimentalDecorators: config.True,
			},
		},
		UnsupportedJSFeatures: compat.UnsupportedJSFeatures(map[compat.Engine]compat.Semver{
			compat.ES: {Parts: []int{esVersion}},
		}),
	})
}

func expectParseErrorTSNoAmbiguousLessThan(t *testing.T, contents string, expected string) {
	t.Helper()
	expectParseErrorCommon(t, contents, expected, config.Options{
		TS: config.TSOptions{
			Parse:               true,
			NoAmbiguousLessThan: true,
		},
	})
}

func expectPrintedTSNoAmbiguousLessThan(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		TS: config.TSOptions{
			Parse:               true,
			NoAmbiguousLessThan: true,
		},
	})
}

func expectParseErrorTSX(t *testing.T, contents string, expected string) {
	t.Helper()
	expectParseErrorCommon(t, contents, expected, config.Options{
		TS: config.TSOptions{
			Parse: true,
		},
		JSX: config.JSXOptions{
			Parse: true,
		},
	})
}

func expectPrintedTSX(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		TS: config.TSOptions{
			Parse: true,
		},
		JSX: config.JSXOptions{
			Parse: true,
		},
	})
}

func TestTSTypes(t *testing.T) {
	expectPrintedTS(t, "let x: T extends number\n ? T\n : number", "let x;\n")
	expectPrintedTS(t, "let x: {y: T extends number ? T : number}", "let x;\n")
	expectPrintedTS(t, "let x: {y: T \n extends: number}", "let x;\n")
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
	expectPrintedTS(t, "export type\n{ Foo } \n x", "x;\n")
	expectPrintedTS(t, "export type\n* from 'foo' \n x", "x;\n")
	expectPrintedTS(t, "export type\n* as ns from 'foo' \n x", "x;\n")
	expectParseErrorTS(t, "export type\nFoo = {}", "<stdin>: ERROR: Unexpected newline after \"type\"\n")
	expectPrintedTS(t, "let x: {x: 'a', y: false, z: null}", "let x;\n")
	expectPrintedTS(t, "let x: {foo(): void}", "let x;\n")
	expectPrintedTS(t, "let x: {['x']: number}", "let x;\n")
	expectPrintedTS(t, "let x: {['x'](): void}", "let x;\n")
	expectPrintedTS(t, "let x: {[key: string]: number}", "let x;\n")
	expectPrintedTS(t, "let x: {[keyof: string]: number}", "let x;\n")
	expectPrintedTS(t, "let x: {[readonly: string]: number}", "let x;\n")
	expectPrintedTS(t, "let x: {[infer: string]: number}", "let x;\n")
	expectPrintedTS(t, "let x: [keyof: string]", "let x;\n")
	expectPrintedTS(t, "let x: [readonly: string]", "let x;\n")
	expectPrintedTS(t, "let x: [infer: string]", "let x;\n")
	expectParseErrorTS(t, "let x: A extends B ? keyof : string", "<stdin>: ERROR: Unexpected \":\"\n")
	expectParseErrorTS(t, "let x: A extends B ? readonly : string", "<stdin>: ERROR: Unexpected \":\"\n")
	expectParseErrorTS(t, "let x: A extends B ? infer : string", "<stdin>: ERROR: Expected identifier but found \":\"\n")
	expectParseErrorTS(t, "let x: {[new: string]: number}", "<stdin>: ERROR: Expected \"(\" but found \":\"\n")
	expectParseErrorTS(t, "let x: {[import: string]: number}", "<stdin>: ERROR: Expected \"(\" but found \":\"\n")
	expectParseErrorTS(t, "let x: {[typeof: string]: number}", "<stdin>: ERROR: Expected identifier but found \":\"\n")
	expectPrintedTS(t, "let x: () => void = Foo", "let x = Foo;\n")
	expectPrintedTS(t, "let x: new () => void = Foo", "let x = Foo;\n")
	expectPrintedTS(t, "let x = 'x' as keyof T", "let x = \"x\";\n")
	expectPrintedTS(t, "let x = [1] as readonly [number]", "let x = [1];\n")
	expectPrintedTS(t, "let x = 'x' as keyof typeof Foo", "let x = \"x\";\n")
	expectPrintedTS(t, "let fs: typeof import('fs') = require('fs')", "let fs = require(\"fs\");\n")
	expectPrintedTS(t, "let fs: typeof import('fs').exists = require('fs').exists", "let fs = require(\"fs\").exists;\n")
	expectPrintedTS(t, "let fs: typeof import('fs', { assert: { type: 'json' } }) = require('fs')", "let fs = require(\"fs\");\n")
	expectPrintedTS(t, "let fs: typeof import('fs', { assert: { 'resolution-mode': 'import' } }) = require('fs')", "let fs = require(\"fs\");\n")
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
	expectPrintedTS(t, "type Foo<T> = {[infer in T]?: Foo}", "")
	expectPrintedTS(t, "type Foo<T> = {[keyof in T]?: Foo}", "")
	expectPrintedTS(t, "type Foo<T> = {[asserts in T]?: Foo}", "")
	expectPrintedTS(t, "type Foo<T> = {[abstract in T]?: Foo}", "")
	expectPrintedTS(t, "type Foo<T> = {[readonly in T]?: Foo}", "")
	expectPrintedTS(t, "type Foo<T> = {[satisfies in T]?: Foo}", "")
	expectPrintedTS(t, "let x: number! = y", "let x = y;\n")
	expectPrintedTS(t, "let x: number \n !y", "let x;\n!y;\n")
	expectPrintedTS(t, "const x: unique = y", "const x = y;\n")
	expectPrintedTS(t, "const x: unique<T> = y", "const x = y;\n")
	expectPrintedTS(t, "const x: unique\nsymbol = y", "const x = y;\n")
	expectPrintedTS(t, "let x: typeof a = y", "let x = y;\n")
	expectPrintedTS(t, "let x: typeof a.b = y", "let x = y;\n")
	expectPrintedTS(t, "let x: typeof a.if = y", "let x = y;\n")
	expectPrintedTS(t, "let x: typeof if.a = y", "let x = y;\n")
	expectPrintedTS(t, "let x: typeof readonly = y", "let x = y;\n")
	expectParseErrorTS(t, "let x: typeof readonly Array", "<stdin>: ERROR: Expected \";\" but found \"Array\"\n")
	expectPrintedTS(t, "let x: `y`", "let x;\n")
	expectParseErrorTS(t, "let x: tag`y`", "<stdin>: ERROR: Expected \";\" but found \"`y`\"\n")
	expectPrintedTS(t, "let x: { <A extends B>(): c.d \n <E extends F>(): g.h }", "let x;\n")
	expectPrintedTSX(t, "type x = a.b \n <c></c>", "/* @__PURE__ */ React.createElement(\"c\", null);\n")
	expectPrintedTS(t, "type Foo = a.b \n | c.d", "")
	expectPrintedTS(t, "type Foo = a.b \n & c.d", "")
	expectPrintedTS(t, "type Foo = \n | a.b \n | c.d", "")
	expectPrintedTS(t, "type Foo = \n & a.b \n & c.d", "")
	expectPrintedTS(t, "type Foo = Bar extends [infer T] ? T : null", "")
	expectPrintedTS(t, "type Foo = Bar extends [infer T extends string] ? T : null", "")
	expectPrintedTS(t, "type Foo = {} extends infer T extends {} ? A<T> : never", "")
	expectPrintedTS(t, "type Foo = {} extends (infer T extends {}) ? A<T> : never", "")
	expectPrintedTS(t, "let x: A extends B<infer C extends D> ? D : never", "let x;\n")
	expectPrintedTS(t, "let x: A extends B<infer C extends D ? infer C : never> ? D : never", "let x;\n")
	expectPrintedTS(t, "let x: ([e1, e2, ...es]: any) => any", "let x;\n")
	expectPrintedTS(t, "let x: (...[e1, e2, es]: any) => any", "let x;\n")
	expectPrintedTS(t, "let x: (...[e1, e2, ...es]: any) => any", "let x;\n")
	expectPrintedTS(t, "let x: (y, [e1, e2, ...es]: any) => any", "let x;\n")
	expectPrintedTS(t, "let x: (y, ...[e1, e2, es]: any) => any", "let x;\n")
	expectPrintedTS(t, "let x: (y, ...[e1, e2, ...es]: any) => any", "let x;\n")

	expectPrintedTS(t, "let x: A.B<X.Y>", "let x;\n")
	expectPrintedTS(t, "let x: A.B<X.Y>=2", "let x = 2;\n")
	expectPrintedTS(t, "let x: A.B<X.Y<Z>>", "let x;\n")
	expectPrintedTS(t, "let x: A.B<X.Y<Z>>=2", "let x = 2;\n")
	expectPrintedTS(t, "let x: A.B<X.Y<Z<T>>>", "let x;\n")
	expectPrintedTS(t, "let x: A.B<X.Y<Z<T>>>=2", "let x = 2;\n")

	expectPrintedTS(t, "(): A<T>=> 0", "() => 0;\n")
	expectPrintedTS(t, "(): A<B<T>>=> 0", "() => 0;\n")
	expectPrintedTS(t, "(): A<B<C<T>>>=> 0", "() => 0;\n")

	expectPrintedTS(t, "let foo: any\n<x>y", "let foo;\ny;\n")
	expectPrintedTSX(t, "let foo: any\n<x>y</x>", "let foo;\n/* @__PURE__ */ React.createElement(\"x\", null, \"y\");\n")
	expectParseErrorTS(t, "let foo: (any\n<x>y)", "<stdin>: ERROR: Expected \")\" but found \"<\"\n")

	expectPrintedTS(t, "let foo = bar as (null)", "let foo = bar;\n")
	expectPrintedTS(t, "let foo = bar\nas (null)", "let foo = bar;\nas(null);\n")
	expectParseErrorTS(t, "let foo = (bar\nas (null))", "<stdin>: ERROR: Expected \")\" but found \"as\"\n")

	expectPrintedTS(t, "a as any ? b : c;", "a ? b : c;\n")
	expectPrintedTS(t, "a as any ? async () => b : c;", "a ? async () => b : c;\n")
	expectPrintedTS(t, "foo as number extends Object ? any : any;", "foo;\n")
	expectPrintedTS(t, "foo as number extends Object ? () => void : any;", "foo;\n")
	expectPrintedTS(t, "let a = b ? c : d as T extends T ? T extends T ? T : never : never ? e : f;", "let a = b ? c : d ? e : f;\n")
	expectParseErrorTS(t, "type a = b extends c", "<stdin>: ERROR: Expected \"?\" but found end of file\n")
	expectParseErrorTS(t, "type a = b extends c extends d", "<stdin>: ERROR: Expected \"?\" but found \"extends\"\n")
	expectParseErrorTS(t, "type a = b ? c : d", "<stdin>: ERROR: Expected \";\" but found \"?\"\n")

	expectPrintedTS(t, "let foo: keyof Object = 'toString'", "let foo = \"toString\";\n")
	expectPrintedTS(t, "let foo: keyof\nObject = 'toString'", "let foo = \"toString\";\n")
	expectPrintedTS(t, "let foo: (keyof\nObject) = 'toString'", "let foo = \"toString\";\n")

	expectPrintedTS(t, "type Foo = Array<<T>(x: T) => T>\n x", "x;\n")
	expectPrintedTSX(t, "<Foo<<T>(x: T) => T>/>", "/* @__PURE__ */ React.createElement(Foo, null);\n")

	// Certain built-in types do not accept type parameters
	expectPrintedTS(t, "x as 1 < 1", "x < 1;\n")
	expectPrintedTS(t, "x as 1n < 1", "x < 1;\n")
	expectPrintedTS(t, "x as -1 < 1", "x < 1;\n")
	expectPrintedTS(t, "x as -1n < 1", "x < 1;\n")
	expectPrintedTS(t, "x as '' < 1", "x < 1;\n")
	expectPrintedTS(t, "x as `` < 1", "x < 1;\n")
	expectPrintedTS(t, "x as any < 1", "x < 1;\n")
	expectPrintedTS(t, "x as bigint < 1", "x < 1;\n")
	expectPrintedTS(t, "x as false < 1", "x < 1;\n")
	expectPrintedTS(t, "x as never < 1", "x < 1;\n")
	expectPrintedTS(t, "x as null < 1", "x < 1;\n")
	expectPrintedTS(t, "x as number < 1", "x < 1;\n")
	expectPrintedTS(t, "x as object < 1", "x < 1;\n")
	expectPrintedTS(t, "x as string < 1", "x < 1;\n")
	expectPrintedTS(t, "x as symbol < 1", "x < 1;\n")
	expectPrintedTS(t, "x as this < 1", "x < 1;\n")
	expectPrintedTS(t, "x as true < 1", "x < 1;\n")
	expectPrintedTS(t, "x as undefined < 1", "x < 1;\n")
	expectPrintedTS(t, "x as unique symbol < 1", "x < 1;\n")
	expectPrintedTS(t, "x as unknown < 1", "x < 1;\n")
	expectPrintedTS(t, "x as void < 1", "x < 1;\n")
	expectParseErrorTS(t, "x as Foo < 1", "<stdin>: ERROR: Expected \">\" but found end of file\n")

	// These keywords are valid tuple labels
	expectPrintedTS(t, "type _false = [false: string]", "")
	expectPrintedTS(t, "type _function = [function: string]", "")
	expectPrintedTS(t, "type _import = [import: string]", "")
	expectPrintedTS(t, "type _new = [new: string]", "")
	expectPrintedTS(t, "type _null = [null: string]", "")
	expectPrintedTS(t, "type _this = [this: string]", "")
	expectPrintedTS(t, "type _true = [true: string]", "")
	expectPrintedTS(t, "type _typeof = [typeof: string]", "")
	expectPrintedTS(t, "type _void = [void: string]", "")

	// These keywords are invalid tuple labels
	expectParseErrorTS(t, "type _break = [break: string]", "<stdin>: ERROR: Unexpected \"break\"\n")
	expectParseErrorTS(t, "type _case = [case: string]", "<stdin>: ERROR: Unexpected \"case\"\n")
	expectParseErrorTS(t, "type _catch = [catch: string]", "<stdin>: ERROR: Unexpected \"catch\"\n")
	expectParseErrorTS(t, "type _class = [class: string]", "<stdin>: ERROR: Unexpected \"class\"\n")
	expectParseErrorTS(t, "type _const = [const: string]", "<stdin>: ERROR: Unexpected \"const\"\n")
	expectParseErrorTS(t, "type _continue = [continue: string]", "<stdin>: ERROR: Unexpected \"continue\"\n")
	expectParseErrorTS(t, "type _debugger = [debugger: string]", "<stdin>: ERROR: Unexpected \"debugger\"\n")
	expectParseErrorTS(t, "type _default = [default: string]", "<stdin>: ERROR: Unexpected \"default\"\n")
	expectParseErrorTS(t, "type _delete = [delete: string]", "<stdin>: ERROR: Unexpected \"delete\"\n")
	expectParseErrorTS(t, "type _do = [do: string]", "<stdin>: ERROR: Unexpected \"do\"\n")
	expectParseErrorTS(t, "type _else = [else: string]", "<stdin>: ERROR: Unexpected \"else\"\n")
	expectParseErrorTS(t, "type _enum = [enum: string]", "<stdin>: ERROR: Unexpected \"enum\"\n")
	expectParseErrorTS(t, "type _export = [export: string]", "<stdin>: ERROR: Unexpected \"export\"\n")
	expectParseErrorTS(t, "type _extends = [extends: string]", "<stdin>: ERROR: Unexpected \"extends\"\n")
	expectParseErrorTS(t, "type _finally = [finally: string]", "<stdin>: ERROR: Unexpected \"finally\"\n")
	expectParseErrorTS(t, "type _for = [for: string]", "<stdin>: ERROR: Unexpected \"for\"\n")
	expectParseErrorTS(t, "type _if = [if: string]", "<stdin>: ERROR: Unexpected \"if\"\n")
	expectParseErrorTS(t, "type _in = [in: string]", "<stdin>: ERROR: Unexpected \"in\"\n")
	expectParseErrorTS(t, "type _instanceof = [instanceof: string]", "<stdin>: ERROR: Unexpected \"instanceof\"\n")
	expectParseErrorTS(t, "type _return = [return: string]", "<stdin>: ERROR: Unexpected \"return\"\n")
	expectParseErrorTS(t, "type _super = [super: string]", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseErrorTS(t, "type _switch = [switch: string]", "<stdin>: ERROR: Unexpected \"switch\"\n")
	expectParseErrorTS(t, "type _throw = [throw: string]", "<stdin>: ERROR: Unexpected \"throw\"\n")
	expectParseErrorTS(t, "type _try = [try: string]", "<stdin>: ERROR: Unexpected \"try\"\n")
	expectParseErrorTS(t, "type _var = [var: string]", "<stdin>: ERROR: Unexpected \"var\"\n")
	expectParseErrorTS(t, "type _while = [while: string]", "<stdin>: ERROR: Unexpected \"while\"\n")
	expectParseErrorTS(t, "type _with = [with: string]", "<stdin>: ERROR: Unexpected \"with\"\n")

	// TypeScript 4.1
	expectPrintedTS(t, "let foo: `${'a' | 'b'}-${'c' | 'd'}` = 'a-c'", "let foo = \"a-c\";\n")

	// TypeScript 4.2
	expectPrintedTS(t, "let x: abstract new () => void = Foo", "let x = Foo;\n")
	expectPrintedTS(t, "let x: abstract new <T>() => Foo<T>", "let x;\n")
	expectPrintedTS(t, "let x: abstract new <T extends object>() => Foo<T>", "let x;\n")
	expectParseErrorTS(t, "let x: abstract () => void = Foo", "<stdin>: ERROR: Expected \";\" but found \"(\"\n")
	expectParseErrorTS(t, "let x: abstract <T>() => Foo<T>", "<stdin>: ERROR: Expected \";\" but found \"(\"\n")
	expectParseErrorTS(t, "let x: abstract <T extends object>() => Foo<T>", "<stdin>: ERROR: Expected \"?\" but found \">\"\n")

	// TypeScript 4.7
	jsxErrorArrow := "<stdin>: ERROR: The character \">\" is not valid inside a JSX element\n" +
		"NOTE: Did you mean to escape it as \"{'>'}\" instead?\n"
	expectPrintedTS(t, "type Foo<in T> = T", "")
	expectPrintedTS(t, "type Foo<out T> = T", "")
	expectPrintedTS(t, "type Foo<in out> = T", "")
	expectPrintedTS(t, "type Foo<out out> = T", "")
	expectPrintedTS(t, "type Foo<in out out> = T", "")
	expectPrintedTS(t, "type Foo<in X, out Y> = [X, Y]", "")
	expectPrintedTS(t, "type Foo<out X, in Y> = [X, Y]", "")
	expectPrintedTS(t, "type Foo<out X, out Y extends keyof X> = [X, Y]", "")
	expectParseErrorTS(t, "type Foo<i\\u006E T> = T", "<stdin>: ERROR: Expected identifier but found \"i\\\\u006E\"\n")
	expectParseErrorTS(t, "type Foo<ou\\u0074 T> = T", "<stdin>: ERROR: Expected \">\" but found \"T\"\n")
	expectParseErrorTS(t, "type Foo<in in> = T", "<stdin>: ERROR: The modifier \"in\" is not valid here:\n<stdin>: ERROR: Expected identifier but found \">\"\n")
	expectParseErrorTS(t, "type Foo<out in> = T", "<stdin>: ERROR: The modifier \"in\" is not valid here:\n<stdin>: ERROR: Expected identifier but found \">\"\n")
	expectParseErrorTS(t, "type Foo<out in T> = T", "<stdin>: ERROR: The modifier \"in\" is not valid here:\n")
	expectParseErrorTS(t, "type Foo<public T> = T", "<stdin>: ERROR: Expected \">\" but found \"T\"\n")
	expectParseErrorTS(t, "type Foo<in out in T> = T", "<stdin>: ERROR: The modifier \"in\" is not valid here:\n")
	expectParseErrorTS(t, "type Foo<in out out T> = T", "<stdin>: ERROR: The modifier \"out\" is not valid here:\n")
	expectPrintedTS(t, "class Foo<in T> {}", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo<out T> {}", "class Foo {\n}\n")
	expectPrintedTS(t, "export default class Foo<in T> {}", "export default class Foo {\n}\n")
	expectPrintedTS(t, "export default class Foo<out T> {}", "export default class Foo {\n}\n")
	expectPrintedTS(t, "export default class <in T> {}", "export default class {\n}\n")
	expectPrintedTS(t, "export default class <out T> {}", "export default class {\n}\n")
	expectPrintedTS(t, "interface Foo<in T> {}", "")
	expectPrintedTS(t, "interface Foo<out T> {}", "")
	expectPrintedTS(t, "declare class Foo<in T> {}", "")
	expectPrintedTS(t, "declare class Foo<out T> {}", "")
	expectPrintedTS(t, "declare interface Foo<in T> {}", "")
	expectPrintedTS(t, "declare interface Foo<out T> {}", "")
	expectParseErrorTS(t, "function foo<in T>() {}", "<stdin>: ERROR: The modifier \"in\" is not valid here:\n")
	expectParseErrorTS(t, "function foo<out T>() {}", "<stdin>: ERROR: The modifier \"out\" is not valid here:\n")
	expectParseErrorTS(t, "export default function foo<in T>() {}", "<stdin>: ERROR: The modifier \"in\" is not valid here:\n")
	expectParseErrorTS(t, "export default function foo<out T>() {}", "<stdin>: ERROR: The modifier \"out\" is not valid here:\n")
	expectParseErrorTS(t, "export default function <in T>() {}", "<stdin>: ERROR: The modifier \"in\" is not valid here:\n")
	expectParseErrorTS(t, "export default function <out T>() {}", "<stdin>: ERROR: The modifier \"out\" is not valid here:\n")
	expectParseErrorTS(t, "let foo: Foo<in T>", "<stdin>: ERROR: Unexpected \"in\"\n")
	expectParseErrorTS(t, "let foo: Foo<out T>", "<stdin>: ERROR: Expected \">\" but found \"T\"\n")
	expectParseErrorTS(t, "declare function foo<in T>()", "<stdin>: ERROR: The modifier \"in\" is not valid here:\n")
	expectParseErrorTS(t, "declare function foo<out T>()", "<stdin>: ERROR: The modifier \"out\" is not valid here:\n")
	expectParseErrorTS(t, "declare let foo: Foo<in T>", "<stdin>: ERROR: Unexpected \"in\"\n")
	expectParseErrorTS(t, "declare let foo: Foo<out T>", "<stdin>: ERROR: Expected \">\" but found \"T\"\n")
	expectPrintedTS(t, "Foo = class <in T> {}", "Foo = class {\n};\n")
	expectPrintedTS(t, "Foo = class <out T> {}", "Foo = class {\n};\n")
	expectPrintedTS(t, "Foo = class Bar<in T> {}", "Foo = class Bar {\n};\n")
	expectPrintedTS(t, "Foo = class Bar<out T> {}", "Foo = class Bar {\n};\n")
	expectParseErrorTS(t, "foo = function <in T>() {}", "<stdin>: ERROR: The modifier \"in\" is not valid here:\n")
	expectParseErrorTS(t, "foo = function <out T>() {}", "<stdin>: ERROR: The modifier \"out\" is not valid here:\n")
	expectParseErrorTS(t, "class Foo { foo<in T>(): T {} }", "<stdin>: ERROR: The modifier \"in\" is not valid here:\n")
	expectParseErrorTS(t, "class Foo { foo<out T>(): T {} }", "<stdin>: ERROR: The modifier \"out\" is not valid here:\n")
	expectParseErrorTS(t, "foo = { foo<in T>(): T {} }", "<stdin>: ERROR: The modifier \"in\" is not valid here:\n")
	expectParseErrorTS(t, "foo = { foo<out T>(): T {} }", "<stdin>: ERROR: The modifier \"out\" is not valid here:\n")
	expectParseErrorTS(t, "<in T>() => {}", "<stdin>: ERROR: The modifier \"in\" is not valid here:\n")
	expectParseErrorTS(t, "<out T>() => {}", "<stdin>: ERROR: The modifier \"out\" is not valid here:\n")
	expectParseErrorTS(t, "<in T, out T>() => {}", "<stdin>: ERROR: The modifier \"in\" is not valid here:\n<stdin>: ERROR: The modifier \"out\" is not valid here:\n")
	expectParseErrorTS(t, "let x: <in T>() => {}", "<stdin>: ERROR: The modifier \"in\" is not valid here:\n")
	expectParseErrorTS(t, "let x: <out T>() => {}", "<stdin>: ERROR: The modifier \"out\" is not valid here:\n")
	expectParseErrorTS(t, "let x: <in T, out T>() => {}", "<stdin>: ERROR: The modifier \"in\" is not valid here:\n<stdin>: ERROR: The modifier \"out\" is not valid here:\n")
	expectParseErrorTS(t, "let x: new <in T>() => {}", "<stdin>: ERROR: The modifier \"in\" is not valid here:\n")
	expectParseErrorTS(t, "let x: new <out T>() => {}", "<stdin>: ERROR: The modifier \"out\" is not valid here:\n")
	expectParseErrorTS(t, "let x: new <in T, out T>() => {}", "<stdin>: ERROR: The modifier \"in\" is not valid here:\n<stdin>: ERROR: The modifier \"out\" is not valid here:\n")
	expectParseErrorTS(t, "let x: { y<in T>(): any }", "<stdin>: ERROR: The modifier \"in\" is not valid here:\n")
	expectParseErrorTS(t, "let x: { y<out T>(): any }", "<stdin>: ERROR: The modifier \"out\" is not valid here:\n")
	expectParseErrorTS(t, "let x: { y<in T, out T>(): any }", "<stdin>: ERROR: The modifier \"in\" is not valid here:\n<stdin>: ERROR: The modifier \"out\" is not valid here:\n")
	expectPrintedTSX(t, "<in T></in>", "/* @__PURE__ */ React.createElement(\"in\", { T: true });\n")
	expectPrintedTSX(t, "<out T></out>", "/* @__PURE__ */ React.createElement(\"out\", { T: true });\n")
	expectPrintedTSX(t, "<in out T></in>", "/* @__PURE__ */ React.createElement(\"in\", { out: true, T: true });\n")
	expectPrintedTSX(t, "<out in T></out>", "/* @__PURE__ */ React.createElement(\"out\", { in: true, T: true });\n")
	expectPrintedTSX(t, "<in T extends={true}></in>", "/* @__PURE__ */ React.createElement(\"in\", { T: true, extends: true });\n")
	expectPrintedTSX(t, "<out T extends={true}></out>", "/* @__PURE__ */ React.createElement(\"out\", { T: true, extends: true });\n")
	expectPrintedTSX(t, "<in out T extends={true}></in>", "/* @__PURE__ */ React.createElement(\"in\", { out: true, T: true, extends: true });\n")
	expectParseErrorTSX(t, "<in T,>() => {}", "<stdin>: ERROR: Expected \">\" but found \",\"\n")
	expectParseErrorTSX(t, "<out T,>() => {}", "<stdin>: ERROR: Expected \">\" but found \",\"\n")
	expectParseErrorTSX(t, "<in out T,>() => {}", "<stdin>: ERROR: Expected \">\" but found \",\"\n")
	expectParseErrorTSX(t, "<in T extends any>() => {}", jsxErrorArrow+"<stdin>: ERROR: Unexpected end of file before a closing \"in\" tag\n<stdin>: NOTE: The opening \"in\" tag is here:\n")
	expectParseErrorTSX(t, "<out T extends any>() => {}", jsxErrorArrow+"<stdin>: ERROR: Unexpected end of file before a closing \"out\" tag\n<stdin>: NOTE: The opening \"out\" tag is here:\n")
	expectParseErrorTSX(t, "<in out T extends any>() => {}", jsxErrorArrow+"<stdin>: ERROR: Unexpected end of file before a closing \"in\" tag\n<stdin>: NOTE: The opening \"in\" tag is here:\n")
	expectPrintedTS(t, "class Container { get data(): typeof this.#data {} }", "class Container {\n  get data() {\n  }\n}\n")
	expectPrintedTS(t, "const a: typeof this.#a = 1;", "const a = 1;\n")
	expectParseErrorTS(t, "const a: typeof #a = 1;", "<stdin>: ERROR: Expected identifier but found \"#a\"\n")

	// TypeScript 5.0
	expectPrintedTS(t, "class Foo<const T> {}", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo<const T extends X> {}", "class Foo {\n}\n")
	expectPrintedTS(t, "Foo = class <const T> {}", "Foo = class {\n};\n")
	expectPrintedTS(t, "Foo = class Bar<const T> {}", "Foo = class Bar {\n};\n")
	expectPrintedTS(t, "function foo<const T>() {}", "function foo() {\n}\n")
	expectPrintedTS(t, "foo = function <const T>() {}", "foo = function() {\n};\n")
	expectPrintedTS(t, "foo = function bar<const T>() {}", "foo = function bar() {\n};\n")
	expectPrintedTS(t, "class Foo { bar<const T>() {} }", "class Foo {\n  bar() {\n  }\n}\n")
	expectPrintedTS(t, "interface Foo { bar<const T>(): T }", "")
	expectPrintedTS(t, "interface Foo { new bar<const T>(): T }", "")
	expectPrintedTS(t, "let x: { bar<const T>(): T }", "let x;\n")
	expectPrintedTS(t, "let x: { new bar<const T>(): T }", "let x;\n")
	expectPrintedTS(t, "foo = { bar<const T>() {} }", "foo = { bar() {\n} };\n")
	expectPrintedTS(t, "x = <const>(y)", "x = y;\n")
	expectPrintedTS(t, "<const T>() => {}", "() => {\n};\n")
	expectPrintedTS(t, "<const const T>() => {}", "() => {\n};\n")
	expectPrintedTS(t, "async <const T>() => {}", "async () => {\n};\n")
	expectPrintedTS(t, "async <const const T>() => {}", "async () => {\n};\n")
	expectPrintedTS(t, "let x: <const T>() => T = y", "let x = y;\n")
	expectPrintedTS(t, "let x: <const const T>() => T = y", "let x = y;\n")
	expectPrintedTS(t, "let x: new <const T>() => T = y", "let x = y;\n")
	expectPrintedTS(t, "let x: new <const const T>() => T = y", "let x = y;\n")
	expectParseErrorTS(t, "type Foo<const T> = T", "<stdin>: ERROR: The modifier \"const\" is not valid here:\n")
	expectParseErrorTS(t, "interface Foo<const T> {}", "<stdin>: ERROR: The modifier \"const\" is not valid here:\n")
	expectParseErrorTS(t, "let x: <const>() => {}", "<stdin>: ERROR: Expected identifier but found \">\"\n")
	expectParseErrorTS(t, "let x: new <const>() => {}", "<stdin>: ERROR: Expected identifier but found \">\"\n")
	expectParseErrorTS(t, "let x: Foo<const T>", "<stdin>: ERROR: Expected \">\" but found \"T\"\n")
	expectParseErrorTS(t, "x = <T,>(y)", "<stdin>: ERROR: Expected \"=>\" but found end of file\n")
	expectParseErrorTS(t, "x = <const T>(y)", "<stdin>: ERROR: Expected \"=>\" but found end of file\n")
	expectParseErrorTS(t, "x = <T extends X>(y)", "<stdin>: ERROR: Expected \"=>\" but found end of file\n")
	expectParseErrorTS(t, "x = async <T,>(y)", "<stdin>: ERROR: Expected \"=>\" but found end of file\n")
	expectParseErrorTS(t, "x = async <const T>(y)", "<stdin>: ERROR: Expected \"=>\" but found end of file\n")
	expectParseErrorTS(t, "x = async <T extends X>(y)", "<stdin>: ERROR: Expected \"=>\" but found end of file\n")
	expectParseErrorTS(t, "x = <const const>() => {}", "<stdin>: ERROR: Expected \">\" but found \"const\"\n")
	expectPrintedTS(t, "class Foo<const const const T> {}", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo<const in out T> {}", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo<in const out T> {}", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo<in out const T> {}", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo<const in const out const T> {}", "class Foo {\n}\n")
	expectParseErrorTS(t, "class Foo<in const> {}", "<stdin>: ERROR: Expected identifier but found \">\"\n")
	expectParseErrorTS(t, "class Foo<out const> {}", "<stdin>: ERROR: Expected identifier but found \">\"\n")
	expectParseErrorTS(t, "class Foo<in out const> {}", "<stdin>: ERROR: Expected identifier but found \">\"\n")
	expectPrintedTSX(t, "<const>(x)</const>", "/* @__PURE__ */ React.createElement(\"const\", null, \"(x)\");\n")
	expectPrintedTSX(t, "<const const/>", "/* @__PURE__ */ React.createElement(\"const\", { const: true });\n")
	expectPrintedTSX(t, "<const const></const>", "/* @__PURE__ */ React.createElement(\"const\", { const: true });\n")
	expectPrintedTSX(t, "<const T/>", "/* @__PURE__ */ React.createElement(\"const\", { T: true });\n")
	expectPrintedTSX(t, "<const T></const>", "/* @__PURE__ */ React.createElement(\"const\", { T: true });\n")
	expectPrintedTSX(t, "<const T>(y) = {}</const>", "/* @__PURE__ */ React.createElement(\"const\", { T: true }, \"(y) = \");\n")
	expectPrintedTSX(t, "<const T extends/>", "/* @__PURE__ */ React.createElement(\"const\", { T: true, extends: true });\n")
	expectPrintedTSX(t, "<const T extends></const>", "/* @__PURE__ */ React.createElement(\"const\", { T: true, extends: true });\n")
	expectPrintedTSX(t, "<const T extends>(y) = {}</const>", "/* @__PURE__ */ React.createElement(\"const\", { T: true, extends: true }, \"(y) = \");\n")
	expectPrintedTSX(t, "<const T,>() => {}", "() => {\n};\n")
	expectPrintedTSX(t, "<const T, X>() => {}", "() => {\n};\n")
	expectPrintedTSX(t, "<const T, const X>() => {}", "() => {\n};\n")
	expectPrintedTSX(t, "<const T, const const X>() => {}", "() => {\n};\n")
	expectPrintedTSX(t, "<const T extends X>() => {}", "() => {\n};\n")
	expectPrintedTSX(t, "async <const T,>() => {}", "async () => {\n};\n")
	expectPrintedTSX(t, "async <const T, X>() => {}", "async () => {\n};\n")
	expectPrintedTSX(t, "async <const T, const X>() => {}", "async () => {\n};\n")
	expectPrintedTSX(t, "async <const T, const const X>() => {}", "async () => {\n};\n")
	expectPrintedTSX(t, "async <const T extends X>() => {}", "async () => {\n};\n")
	expectParseErrorTSX(t, "<const T>() => {}", jsxErrorArrow+"<stdin>: ERROR: Unexpected end of file before a closing \"const\" tag\n<stdin>: NOTE: The opening \"const\" tag is here:\n")
	expectParseErrorTSX(t, "<const const>() => {}", jsxErrorArrow+"<stdin>: ERROR: Unexpected end of file before a closing \"const\" tag\n<stdin>: NOTE: The opening \"const\" tag is here:\n")
	expectParseErrorTSX(t, "<const const T,>() => {}", "<stdin>: ERROR: Expected \">\" but found \",\"\n")
	expectParseErrorTSX(t, "<const const T extends X>() => {}", jsxErrorArrow+"<stdin>: ERROR: Unexpected end of file before a closing \"const\" tag\n<stdin>: NOTE: The opening \"const\" tag is here:\n")
	expectParseErrorTSX(t, "async <const T>() => {}", "<stdin>: ERROR: Unexpected \"const\"\n")
	expectParseErrorTSX(t, "async <const const>() => {}", "<stdin>: ERROR: Unexpected \"const\"\n")
	expectParseErrorTSX(t, "async <const const T,>() => {}", "<stdin>: ERROR: Unexpected \"const\"\n")
	expectParseErrorTSX(t, "async <const const T extends X>() => {}", "<stdin>: ERROR: Unexpected \"const\"\n")
}

func TestTSAsCast(t *testing.T) {
	expectPrintedTS(t, "x as any\n(y);", "x;\ny;\n")
	expectPrintedTS(t, "x as any\n`y`;", "x;\n`y`;\n")
	expectPrintedTS(t, "x as any\n`${y}`;", "x;\n`${y}`;\n")
	expectPrintedTS(t, "x as any\n--y;", "x;\n--y;\n")
	expectPrintedTS(t, "x as any\n++y;", "x;\n++y;\n")
	expectPrintedTS(t, "x + y as any\n(z as any) + 1;", "x + y;\nz + 1;\n")
	expectPrintedTS(t, "x + y as any\n(z as any) = 1;", "x + y;\nz = 1;\n")
	expectPrintedTS(t, "x = y as any\n(z as any) + 1;", "x = y;\nz + 1;\n")
	expectPrintedTS(t, "x = y as any\n(z as any) = 1;", "x = y;\nz = 1;\n")
	expectPrintedTS(t, "x * y as any\n['z'];", "x * y;\n[\"z\"];\n")
	expectPrintedTS(t, "x * y as any\n.z;", "x * y;\n")
	expectPrintedTS(t, "x as y['x'];", "x;\n")
	expectPrintedTS(t, "x as y!['x'];", "x;\n")
	expectPrintedTS(t, "x as y\n['x'];", "x;\n[\"x\"];\n")
	expectPrintedTS(t, "x as y\n!['x'];", "x;\n![\"x\"];\n")
	expectParseErrorTS(t, "x = y as any `z`;", "<stdin>: ERROR: Expected \";\" but found \"`z`\"\n")
	expectParseErrorTS(t, "x = y as any `${z}`;", "<stdin>: ERROR: Expected \";\" but found \"`${\"\n")
	expectParseErrorTS(t, "x = y as any?.z;", "<stdin>: ERROR: Expected \";\" but found \"?.\"\n")
	expectParseErrorTS(t, "x = y as any--;", "<stdin>: ERROR: Expected \";\" but found \"--\"\n")
	expectParseErrorTS(t, "x = y as any++;", "<stdin>: ERROR: Expected \";\" but found \"++\"\n")
	expectParseErrorTS(t, "x = y as any(z);", "<stdin>: ERROR: Expected \";\" but found \"(\"\n")
	expectParseErrorTS(t, "x = y as any\n= z;", "<stdin>: ERROR: Unexpected \"=\"\n")
	expectParseErrorTS(t, "a, x as y `z`;", "<stdin>: ERROR: Expected \";\" but found \"`z`\"\n")
	expectParseErrorTS(t, "a ? b : x as y `z`;", "<stdin>: ERROR: Expected \";\" but found \"`z`\"\n")
	expectParseErrorTS(t, "x as any = y;", "<stdin>: ERROR: Expected \";\" but found \"=\"\n")
	expectParseErrorTS(t, "(x as any = y);", "<stdin>: ERROR: Expected \")\" but found \"=\"\n")
	expectParseErrorTS(t, "(x = y as any(z));", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")
}

func TestTSSatisfies(t *testing.T) {
	expectPrintedTS(t, "const t1 = { a: 1 } satisfies I1;", "const t1 = { a: 1 };\n")
	expectPrintedTS(t, "const t2 = { a: 1, b: 1 } satisfies I1;", "const t2 = { a: 1, b: 1 };\n")
	expectPrintedTS(t, "const t3 = { } satisfies I1;", "const t3 = {};\n")
	expectPrintedTS(t, "const t4: T1 = { a: 'a' } satisfies T1;", "const t4 = { a: \"a\" };\n")
	expectPrintedTS(t, "const t5 = (m => m.substring(0)) satisfies T2;", "const t5 = (m) => m.substring(0);\n")
	expectPrintedTS(t, "const t6 = [1, 2] satisfies [number, number];", "const t6 = [1, 2];\n")
	expectPrintedTS(t, "let t7 = { a: 'test' } satisfies A;", "let t7 = { a: \"test\" };\n")
	expectPrintedTS(t, "let t8 = { a: 'test', b: 'test' } satisfies A;", "let t8 = { a: \"test\", b: \"test\" };\n")
	expectPrintedTS(t, "export default {} satisfies Foo;", "export default {};\n")
	expectPrintedTS(t, "export default { a: 1 } satisfies Foo;", "export default { a: 1 };\n")
	expectPrintedTS(t,
		"const p = { isEven: n => n % 2 === 0, isOdd: n => n % 2 === 1 } satisfies Predicates;",
		"const p = { isEven: (n) => n % 2 === 0, isOdd: (n) => n % 2 === 1 };\n")
	expectPrintedTS(t,
		"let obj: { f(s: string): void } & Record<string, unknown> = { f(s) { }, g(s) { } } satisfies { g(s: string): void } & Record<string, unknown>;",
		"let obj = { f(s) {\n}, g(s) {\n} };\n")
	expectPrintedTS(t,
		"const car = { start() { }, move(d) { }, stop() { } } satisfies Movable & Record<string, unknown>;",
		"const car = { start() {\n}, move(d) {\n}, stop() {\n} };\n",
	)
	expectPrintedTS(t, "var v = undefined satisfies 1;", "var v = void 0;\n")
	expectPrintedTS(t, "const a = { x: 10 } satisfies Partial<Point2d>;", "const a = { x: 10 };\n")
	expectPrintedTS(t,
		"const p = { a: 0, b: \"hello\", x: 8 } satisfies Partial<Record<Keys, unknown>>;",
		"const p = { a: 0, b: \"hello\", x: 8 };\n",
	)
	expectPrintedTS(t,
		"const p = { a: 0, b: \"hello\", x: 8 } satisfies Record<Keys, unknown>;",
		"const p = { a: 0, b: \"hello\", x: 8 };\n",
	)
	expectPrintedTS(t,
		"const x2 = { m: true, s: \"false\" } satisfies Facts;",
		"const x2 = { m: true, s: \"false\" };\n",
	)
	expectPrintedTS(t,
		"export const Palette = { white: { r: 255, g: 255, b: 255 }, black: { r: 0, g: 0, d: 0 }, blue: { r: 0, g: 0, b: 255 }, } satisfies Record<string, Color>;",
		"export const Palette = { white: { r: 255, g: 255, b: 255 }, black: { r: 0, g: 0, d: 0 }, blue: { r: 0, g: 0, b: 255 } };\n",
	)
	expectPrintedTS(t,
		"const a: \"baz\" = \"foo\" satisfies \"foo\" | \"bar\";",
		"const a = \"foo\";\n",
	)
	expectPrintedTS(t,
		"const b: { xyz: \"baz\" } = { xyz: \"foo\" } satisfies { xyz: \"foo\" | \"bar\" };",
		"const b = { xyz: \"foo\" };\n",
	)
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

	expectPrintedTS(t, "abstract \n class A {}", "abstract;\nclass A {\n}\n")
	expectPrintedTS(t, "abstract class A { abstract \n foo(): void {} }", "class A {\n  abstract;\n  foo() {\n  }\n}\n")

	expectPrintedTS(t, "abstract class A { abstract foo(): void; bar(): void {} }", "class A {\n  bar() {\n  }\n}\n")
	expectPrintedTS(t, "export abstract class A { abstract foo(): void; bar(): void {} }", "export class A {\n  bar() {\n  }\n}\n")
	expectPrintedTS(t, "export default abstract", "export default abstract;\n")
	expectPrintedTS(t, "export default abstract - after", "export default abstract - after;\n")
	expectPrintedTS(t, "export default abstract class { abstract foo(): void; bar(): void {} } - after", "export default class {\n  bar() {\n  }\n}\n-after;\n")
	expectPrintedTS(t, "export default abstract class A { abstract foo(): void; bar(): void {} } - after", "export default class A {\n  bar() {\n  }\n}\n-after;\n")

	expectPrintedTS(t, "class A<T extends number> extends B.C<D, E> {}", "class A extends B.C {\n}\n")
	expectPrintedTS(t, "class A<T extends number> implements B.C<D, E>, F.G<H, I> {}", "class A {\n}\n")
	expectPrintedTS(t, "class A<T extends number> extends X implements B.C<D, E>, F.G<H, I> {}", "class A extends X {\n}\n")

	reservedWordError :=
		" is a reserved word and cannot be used in strict mode\n" +
			"<stdin>: NOTE: All code inside a class is implicitly in strict mode\n"

	expectParseErrorTS(t, "class Foo { constructor(public) {} }", "<stdin>: ERROR: \"public\""+reservedWordError)
	expectParseErrorTS(t, "class Foo { constructor(protected) {} }", "<stdin>: ERROR: \"protected\""+reservedWordError)
	expectParseErrorTS(t, "class Foo { constructor(private) {} }", "<stdin>: ERROR: \"private\""+reservedWordError)
	expectPrintedTS(t, "class Foo { constructor(readonly) {} }", "class Foo {\n  constructor(readonly) {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { constructor(override) {} }", "class Foo {\n  constructor(override) {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { constructor(public x) {} }", "class Foo {\n  constructor(x) {\n    this.x = x;\n  }\n}\n")
	expectPrintedTS(t, "class Foo { constructor(protected x) {} }", "class Foo {\n  constructor(x) {\n    this.x = x;\n  }\n}\n")
	expectPrintedTS(t, "class Foo { constructor(private x) {} }", "class Foo {\n  constructor(x) {\n    this.x = x;\n  }\n}\n")
	expectPrintedTS(t, "class Foo { constructor(readonly x) {} }", "class Foo {\n  constructor(x) {\n    this.x = x;\n  }\n}\n")
	expectPrintedTS(t, "class Foo { constructor(override x) {} }", "class Foo {\n  constructor(x) {\n    this.x = x;\n  }\n}\n")
	expectPrintedTS(t, "class Foo { constructor(public readonly x) {} }", "class Foo {\n  constructor(x) {\n    this.x = x;\n  }\n}\n")
	expectPrintedTS(t, "class Foo { constructor(protected readonly x) {} }", "class Foo {\n  constructor(x) {\n    this.x = x;\n  }\n}\n")
	expectPrintedTS(t, "class Foo { constructor(private readonly x) {} }", "class Foo {\n  constructor(x) {\n    this.x = x;\n  }\n}\n")
	expectPrintedTS(t, "class Foo { constructor(override readonly x) {} }", "class Foo {\n  constructor(x) {\n    this.x = x;\n  }\n}\n")

	expectParseErrorTS(t, "class Foo { constructor(public {x}) {} }", "<stdin>: ERROR: Expected identifier but found \"{\"\n")
	expectParseErrorTS(t, "class Foo { constructor(protected {x}) {} }", "<stdin>: ERROR: Expected identifier but found \"{\"\n")
	expectParseErrorTS(t, "class Foo { constructor(private {x}) {} }", "<stdin>: ERROR: Expected identifier but found \"{\"\n")
	expectParseErrorTS(t, "class Foo { constructor(readonly {x}) {} }", "<stdin>: ERROR: Expected identifier but found \"{\"\n")
	expectParseErrorTS(t, "class Foo { constructor(override {x}) {} }", "<stdin>: ERROR: Expected identifier but found \"{\"\n")

	expectParseErrorTS(t, "class Foo { constructor(public [x]) {} }", "<stdin>: ERROR: Expected identifier but found \"[\"\n")
	expectParseErrorTS(t, "class Foo { constructor(protected [x]) {} }", "<stdin>: ERROR: Expected identifier but found \"[\"\n")
	expectParseErrorTS(t, "class Foo { constructor(private [x]) {} }", "<stdin>: ERROR: Expected identifier but found \"[\"\n")
	expectParseErrorTS(t, "class Foo { constructor(readonly [x]) {} }", "<stdin>: ERROR: Expected identifier but found \"[\"\n")
	expectParseErrorTS(t, "class Foo { constructor(override [x]) {} }", "<stdin>: ERROR: Expected identifier but found \"[\"\n")

	expectPrintedAssignSemanticsTS(t, "class Foo { foo: number }", "class Foo {\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { foo: number = 0 }", "class Foo {\n  constructor() {\n    this.foo = 0;\n  }\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { ['foo']: number }", "class Foo {\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { ['foo']: number = 0 }", "class Foo {\n  constructor() {\n    this[\"foo\"] = 0;\n  }\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { foo(): void {} }", "class Foo {\n  foo() {\n  }\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { foo(): void; foo(): void {} }", "class Foo {\n  foo() {\n  }\n}\n")
	expectParseErrorTS(t, "class Foo { foo(): void foo(): void {} }", "<stdin>: ERROR: Expected \";\" but found \"foo\"\n")

	expectPrintedAssignSemanticsTS(t, "class Foo { foo?: number }", "class Foo {\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { foo?: number = 0 }", "class Foo {\n  constructor() {\n    this.foo = 0;\n  }\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { foo?(): void {} }", "class Foo {\n  foo() {\n  }\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { foo?(): void; foo(): void {} }", "class Foo {\n  foo() {\n  }\n}\n")
	expectParseErrorTS(t, "class Foo { foo?(): void foo(): void {} }", "<stdin>: ERROR: Expected \";\" but found \"foo\"\n")

	expectPrintedAssignSemanticsTS(t, "class Foo { foo!: number }", "class Foo {\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { foo!: number = 0 }", "class Foo {\n  constructor() {\n    this.foo = 0;\n  }\n}\n")
	expectParseErrorTS(t, "class Foo { foo!() {} }", "<stdin>: ERROR: Expected \";\" but found \"(\"\n")
	expectParseErrorTS(t, "class Foo { *foo!() {} }", "<stdin>: ERROR: Expected \"(\" but found \"!\"\n")
	expectParseErrorTS(t, "class Foo { get foo!() {} }", "<stdin>: ERROR: Expected \"(\" but found \"!\"\n")
	expectParseErrorTS(t, "class Foo { set foo!(x) {} }", "<stdin>: ERROR: Expected \"(\" but found \"!\"\n")
	expectParseErrorTS(t, "class Foo { async foo!() {} }", "<stdin>: ERROR: Expected \"(\" but found \"!\"\n")

	expectPrintedAssignSemanticsTS(t, "class Foo { 'foo' = 0 }", "class Foo {\n  constructor() {\n    this[\"foo\"] = 0;\n  }\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { ['foo'] = 0 }", "class Foo {\n  constructor() {\n    this[\"foo\"] = 0;\n  }\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { [foo] = 0 }", "var _a;\nclass Foo {\n  constructor() {\n    this[_a] = 0;\n  }\n  static {\n    _a = foo;\n  }\n}\n")
	expectPrintedMangleAssignSemanticsTS(t, "class Foo { 'foo' = 0 }", "class Foo {\n  constructor() {\n    this.foo = 0;\n  }\n}\n")
	expectPrintedMangleAssignSemanticsTS(t, "class Foo { ['foo'] = 0 }", "class Foo {\n  constructor() {\n    this.foo = 0;\n  }\n}\n")

	expectPrintedAssignSemanticsTS(t, "class Foo { foo \n ?: number }", "class Foo {\n}\n")
	expectParseErrorTS(t, "class Foo { foo \n !: number }", "<stdin>: ERROR: Expected identifier but found \"!\"\n")

	expectPrintedAssignSemanticsTS(t, "class Foo { public foo: number }", "class Foo {\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { private foo: number }", "class Foo {\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { protected foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { declare foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { declare public foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { public declare foo: number }", "class Foo {\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { override foo: number }", "class Foo {\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { override public foo: number }", "class Foo {\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { public override foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { declare override public foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { declare foo = 123 }", "class Foo {\n}\n")

	expectPrintedAssignSemanticsTS(t, "class Foo { public static foo: number }", "class Foo {\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { private static foo: number }", "class Foo {\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { protected static foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { declare static foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { declare public static foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { public declare static foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { public static declare foo: number }", "class Foo {\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { override static foo: number }", "class Foo {\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { override public static foo: number }", "class Foo {\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { public override static foo: number }", "class Foo {\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { public static override foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { declare override public static foo: number }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { declare static foo = 123 }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { static declare foo = 123 }", "class Foo {\n}\n")

	expectParseErrorTS(t, "class Foo { declare #foo }", "<stdin>: ERROR: \"declare\" cannot be used with a private identifier\n")
	expectParseErrorTS(t, "class Foo { declare [foo: string]: number }", "<stdin>: ERROR: \"declare\" cannot be used with an index signature\n")
	expectParseErrorTS(t, "class Foo { declare foo() }", "<stdin>: ERROR: \"declare\" cannot be used with a method\n")
	expectParseErrorTS(t, "class Foo { declare get foo() }", "<stdin>: ERROR: \"declare\" cannot be used with a getter\n")
	expectParseErrorTS(t, "class Foo { declare set foo(x) }", "<stdin>: ERROR: \"declare\" cannot be used with a setter\n")

	expectParseErrorTS(t, "class Foo { declare static #foo }", "<stdin>: ERROR: \"declare\" cannot be used with a private identifier\n")
	expectParseErrorTS(t, "class Foo { declare static [foo: string]: number }", "<stdin>: ERROR: \"declare\" cannot be used with an index signature\n")
	expectParseErrorTS(t, "class Foo { declare static foo() }", "<stdin>: ERROR: \"declare\" cannot be used with a method\n")
	expectParseErrorTS(t, "class Foo { declare static get foo() }", "<stdin>: ERROR: \"declare\" cannot be used with a getter\n")
	expectParseErrorTS(t, "class Foo { declare static set foo(x) }", "<stdin>: ERROR: \"declare\" cannot be used with a setter\n")

	expectParseErrorTS(t, "class Foo { static declare #foo }", "<stdin>: ERROR: \"declare\" cannot be used with a private identifier\n")
	expectParseErrorTS(t, "class Foo { static declare [foo: string]: number }", "<stdin>: ERROR: \"declare\" cannot be used with an index signature\n")
	expectParseErrorTS(t, "class Foo { static declare foo() }", "<stdin>: ERROR: \"declare\" cannot be used with a method\n")
	expectParseErrorTS(t, "class Foo { static declare get foo() }", "<stdin>: ERROR: \"declare\" cannot be used with a getter\n")
	expectParseErrorTS(t, "class Foo { static declare set foo(x) }", "<stdin>: ERROR: \"declare\" cannot be used with a setter\n")

	expectPrintedAssignSemanticsTS(t, "class Foo { [key: string]: any\nfoo = 0 }", "class Foo {\n  constructor() {\n    this.foo = 0;\n  }\n}\n")
	expectPrintedAssignSemanticsTS(t, "class Foo { [key: string]: any; foo = 0 }", "class Foo {\n  constructor() {\n    this.foo = 0;\n  }\n}\n")

	expectParseErrorTS(t, "class Foo<> {}", "<stdin>: ERROR: Expected identifier but found \">\"\n")
	expectParseErrorTS(t, "class Foo<,> {}", "<stdin>: ERROR: Expected identifier but found \",\"\n")
	expectParseErrorTS(t, "class Foo<T><T> {}", "<stdin>: ERROR: Expected \"{\" but found \"<\"\n")

	expectPrintedTS(t, "class Foo { foo<T>() {} }", "class Foo {\n  foo() {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { foo?<T>() {} }", "class Foo {\n  foo() {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { [foo]<T>() {} }", "class Foo {\n  [foo]() {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { [foo]?<T>() {} }", "class Foo {\n  [foo]() {\n  }\n}\n")
	expectParseErrorTS(t, "class Foo { foo<T> }", "<stdin>: ERROR: Expected \"(\" but found \"}\"\n")
	expectParseErrorTS(t, "class Foo { foo?<T> }", "<stdin>: ERROR: Expected \"(\" but found \"}\"\n")
	expectParseErrorTS(t, "class Foo { foo!<T>() {} }", "<stdin>: ERROR: Expected \";\" but found \"<\"\n")
	expectParseErrorTS(t, "class Foo { [foo]<T> }", "<stdin>: ERROR: Expected \"(\" but found \"}\"\n")
	expectParseErrorTS(t, "class Foo { [foo]?<T> }", "<stdin>: ERROR: Expected \"(\" but found \"}\"\n")
	expectParseErrorTS(t, "class Foo { [foo]!<T>() {} }", "<stdin>: ERROR: Expected \";\" but found \"<\"\n")
}

func TestTSAutoAccessors(t *testing.T) {
	expectPrintedTS(t, "class Foo { accessor }", "class Foo {\n  accessor;\n}\n")
	expectPrintedTS(t, "class Foo { accessor x }", "class Foo {\n  accessor x;\n}\n")
	expectPrintedTS(t, "class Foo { accessor x? }", "class Foo {\n  accessor x;\n}\n")
	expectPrintedTS(t, "class Foo { accessor x! }", "class Foo {\n  accessor x;\n}\n")
	expectPrintedTS(t, "class Foo { accessor x = y }", "class Foo {\n  accessor x = y;\n}\n")
	expectPrintedTS(t, "class Foo { accessor x? = y }", "class Foo {\n  accessor x = y;\n}\n")
	expectPrintedTS(t, "class Foo { accessor x! = y }", "class Foo {\n  accessor x = y;\n}\n")
	expectPrintedTS(t, "class Foo { accessor x: any }", "class Foo {\n  accessor x;\n}\n")
	expectPrintedTS(t, "class Foo { accessor x?: any }", "class Foo {\n  accessor x;\n}\n")
	expectPrintedTS(t, "class Foo { accessor x!: any }", "class Foo {\n  accessor x;\n}\n")
	expectPrintedTS(t, "class Foo { accessor x: any = y }", "class Foo {\n  accessor x = y;\n}\n")
	expectPrintedTS(t, "class Foo { accessor x?: any = y }", "class Foo {\n  accessor x = y;\n}\n")
	expectPrintedTS(t, "class Foo { accessor x!: any = y }", "class Foo {\n  accessor x = y;\n}\n")
	expectPrintedTS(t, "class Foo { accessor [x] }", "class Foo {\n  accessor [x];\n}\n")
	expectPrintedTS(t, "class Foo { accessor [x]? }", "class Foo {\n  accessor [x];\n}\n")
	expectPrintedTS(t, "class Foo { accessor [x]! }", "class Foo {\n  accessor [x];\n}\n")
	expectPrintedTS(t, "class Foo { accessor [x] = y }", "class Foo {\n  accessor [x] = y;\n}\n")
	expectPrintedTS(t, "class Foo { accessor [x]? = y }", "class Foo {\n  accessor [x] = y;\n}\n")
	expectPrintedTS(t, "class Foo { accessor [x]! = y }", "class Foo {\n  accessor [x] = y;\n}\n")
	expectPrintedTS(t, "class Foo { accessor [x]: any }", "class Foo {\n  accessor [x];\n}\n")
	expectPrintedTS(t, "class Foo { accessor [x]?: any }", "class Foo {\n  accessor [x];\n}\n")
	expectPrintedTS(t, "class Foo { accessor [x]!: any }", "class Foo {\n  accessor [x];\n}\n")
	expectPrintedTS(t, "class Foo { accessor [x]: any = y }", "class Foo {\n  accessor [x] = y;\n}\n")
	expectPrintedTS(t, "class Foo { accessor [x]?: any = y }", "class Foo {\n  accessor [x] = y;\n}\n")
	expectPrintedTS(t, "class Foo { accessor [x]!: any = y }", "class Foo {\n  accessor [x] = y;\n}\n")

	expectParseErrorTS(t, "class Foo { accessor x<T> }", "<stdin>: ERROR: Expected \";\" but found \"<\"\n")
	expectParseErrorTS(t, "class Foo { accessor x<T>() {} }", "<stdin>: ERROR: Expected \";\" but found \"<\"\n")

	expectPrintedTS(t, "declare class Foo { accessor x }", "")
	expectPrintedTS(t, "declare class Foo { accessor #x }", "")
	expectPrintedTS(t, "declare class Foo { static accessor x }", "")
	expectPrintedTS(t, "declare class Foo { static accessor #x }", "")

	// TypeScript doesn't allow these combinations, but we shouldn't crash
	expectPrintedTS(t, "class Foo { declare accessor x }", "class Foo {\n}\n")
	expectPrintedTS(t, "class Foo { readonly accessor x }", "class Foo {\n  accessor x;\n}\n")
	expectPrintedTS(t, "interface Foo { accessor x }", "")
	expectPrintedTS(t, "interface Foo { static accessor x }", "")
	expectPrintedTS(t, "let x: { accessor x }", "let x;\n")
	expectPrintedTS(t, "let x: { static accessor x }", "let x;\n")
	expectParseErrorTS(t, "class Foo { accessor declare x }", "<stdin>: ERROR: Expected \";\" but found \"x\"\n")
	expectParseErrorTS(t, "class Foo { accessor readonly x }", "<stdin>: ERROR: Expected \";\" but found \"x\"\n")
}

func TestTSPrivateIdentifiers(t *testing.T) {
	// The TypeScript compiler still moves private field initializers into the
	// constructor, but it has to leave the private field declaration in place so
	// the private field is still declared.
	expectPrintedTS(t, "class Foo { #foo }", "class Foo {\n  #foo;\n}\n")
	expectPrintedTS(t, "class Foo { #foo = 1 }", "class Foo {\n  #foo = 1;\n}\n")
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

	// Decorators are not valid on private members
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec #foo }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec #foo = 1 }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec #foo() {} }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec get #foo() {} }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec set #foo() {x} }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec static #foo }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec static #foo = 1 }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec static #foo() {} }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec static get #foo() {} }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec static set #foo() {x} }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")

	// Decorators are not able to access private names, since they use the scope
	// that encloses the class declaration. Note that the TypeScript compiler has
	// a bug where it doesn't handle this case and generates invalid code as a
	// result: https://github.com/microsoft/TypeScript/issues/48515.
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { static #foo; @dec(Foo.#foo) bar }", "<stdin>: ERROR: Private name \"#foo\" must be declared in an enclosing class\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { static #foo; @dec(Foo.#foo) bar() {} }", "<stdin>: ERROR: Private name \"#foo\" must be declared in an enclosing class\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { static #foo; bar(@dec(Foo.#foo) x) {} }", "<stdin>: ERROR: Private name \"#foo\" must be declared in an enclosing class\n")
}

func TestTSInterface(t *testing.T) {
	expectPrintedTS(t, "interface\nA\n{ a }", "interface;\nA;\n{\n  a;\n}\n")

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
	expectPrintedTS(t, "namespace\nx\n{ var y }", "namespace;\nx;\n{\n  var y;\n}\n")

	// Check ES5 emit
	expectPrintedTargetTS(t, 5, "namespace x { export var y = 1 }", "var x;\n(function(x) {\n  x.y = 1;\n})(x || (x = {}));\n")
	expectPrintedTargetTS(t, 2015, "namespace x { export var y = 1 }", "var x;\n((x) => {\n  x.y = 1;\n})(x || (x = {}));\n")

	// Certain syntax isn't allowed inside a namespace block
	expectParseErrorTS(t, "namespace x { return }", "<stdin>: ERROR: A return statement cannot be used here:\n")
	expectParseErrorTS(t, "namespace x { await 1 }", "<stdin>: ERROR: \"await\" can only be used inside an \"async\" function\n")
	expectParseErrorTS(t, "namespace x { if (y) return }", "<stdin>: ERROR: A return statement cannot be used here:\n")
	expectParseErrorTS(t, "namespace x { if (y) await 1 }", "<stdin>: ERROR: \"await\" can only be used inside an \"async\" function\n")
	expectParseErrorTS(t, "namespace x { this }", "<stdin>: ERROR: Cannot use \"this\" here:\n")
	expectParseErrorTS(t, "namespace x { () => this }", "<stdin>: ERROR: Cannot use \"this\" here:\n")
	expectParseErrorTS(t, "namespace x { class y { [this] } }", "<stdin>: ERROR: Cannot use \"this\" here:\n")
	expectParseErrorTS(t, "namespace x { (function() { this }) }", "")
	expectParseErrorTS(t, "namespace x { function y() { this } }", "")
	expectParseErrorTS(t, "namespace x { class y { x = this } }", "")
	expectParseErrorTS(t, "export namespace x { export let yield = 1 }",
		"<stdin>: ERROR: \"yield\" is a reserved word and cannot be used in an ECMAScript module\n"+
			"<stdin>: NOTE: This file is considered to be an ECMAScript module because of the \"export\" keyword here:\n")
	expectPrintedTS(t, "namespace x { export let await = 1, y = await }", `var x;
((x) => {
  x.await = 1;
  x.y = x.await;
})(x || (x = {}));
`)
	expectPrintedTS(t, "namespace x { export let yield = 1, y = yield }", `var x;
((x) => {
  x.yield = 1;
  x.y = x.yield;
})(x || (x = {}));
`)

	expectPrintedTS(t, "namespace Foo { 0 }", `var Foo;
((Foo) => {
  0;
})(Foo || (Foo = {}));
`)
	expectPrintedTS(t, "export namespace Foo { 0 }", `export var Foo;
((Foo) => {
  0;
})(Foo || (Foo = {}));
`)

	// Namespaces should introduce a scope that prevents name collisions
	expectPrintedTS(t, "namespace Foo { let x } let x", `var Foo;
((Foo) => {
  let x;
})(Foo || (Foo = {}));
let x;
`)

	// Exports in namespaces shouldn't collide with module exports
	expectPrintedTS(t, "namespace Foo { export let x } export let x", `var Foo;
((Foo) => {
})(Foo || (Foo = {}));
export let x;
`)
	expectPrintedTS(t, "declare namespace Foo { export let x } namespace x { 0 }", `var x;
((x) => {
  0;
})(x || (x = {}));
`)

	errorText := `<stdin>: ERROR: The symbol "foo" has already been declared
<stdin>: NOTE: The symbol "foo" was originally declared here:
`

	// Namespaces with values are not allowed to merge
	expectParseErrorTS(t, "var foo; namespace foo { 0 }", errorText)
	expectParseErrorTS(t, "let foo; namespace foo { 0 }", errorText)
	expectParseErrorTS(t, "const foo = 0; namespace foo { 0 }", errorText)
	expectParseErrorTS(t, "namespace foo { 0 } var foo", errorText)
	expectParseErrorTS(t, "namespace foo { 0 } let foo", errorText)
	expectParseErrorTS(t, "namespace foo { 0 } const foo = 0", errorText)

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
((foo) => {
  0;
})(foo || (foo = {}));
`)
	expectPrintedTS(t, "function* foo() {} namespace foo { 0 }", `function* foo() {
}
((foo) => {
  0;
})(foo || (foo = {}));
`)
	expectPrintedTS(t, "async function foo() {} namespace foo { 0 }", `async function foo() {
}
((foo) => {
  0;
})(foo || (foo = {}));
`)
	expectPrintedTS(t, "class foo {} namespace foo { 0 }", `class foo {
}
((foo) => {
  0;
})(foo || (foo = {}));
`)
	expectPrintedTS(t, "enum foo { a } namespace foo { 0 }", `var foo = /* @__PURE__ */ ((foo) => {
  foo[foo["a"] = 0] = "a";
  return foo;
})(foo || {});
((foo) => {
  0;
})(foo || (foo = {}));
`)
	expectPrintedTS(t, "namespace foo {} namespace foo { 0 }", `var foo;
((foo) => {
  0;
})(foo || (foo = {}));
`)
	expectParseErrorTS(t, "namespace foo { 0 } function foo() {}", errorText)
	expectParseErrorTS(t, "namespace foo { 0 } function* foo() {}", errorText)
	expectParseErrorTS(t, "namespace foo { 0 } async function foo() {}", errorText)
	expectParseErrorTS(t, "namespace foo { 0 } class foo {}", errorText)
	expectPrintedTS(t, "namespace foo { 0 } enum foo { a }", `((foo) => {
  0;
})(foo || (foo = {}));
var foo = /* @__PURE__ */ ((foo) => {
  foo[foo["a"] = 0] = "a";
  return foo;
})(foo || {});
`)
	expectPrintedTS(t, "namespace foo { 0 } namespace foo {}", `var foo;
((foo) => {
  0;
})(foo || (foo = {}));
`)
	expectPrintedTS(t, "namespace foo { 0 } namespace foo { 0 }", `var foo;
((foo) => {
  0;
})(foo || (foo = {}));
((foo) => {
  0;
})(foo || (foo = {}));
`)
	expectPrintedTS(t, "function foo() {} namespace foo { 0 } function foo() {}", `function foo() {
}
((foo) => {
  0;
})(foo || (foo = {}));
function foo() {
}
`)
	expectPrintedTS(t, "function* foo() {} namespace foo { 0 } function* foo() {}", `function* foo() {
}
((foo) => {
  0;
})(foo || (foo = {}));
function* foo() {
}
`)
	expectPrintedTS(t, "async function foo() {} namespace foo { 0 } async function foo() {}", `async function foo() {
}
((foo) => {
  0;
})(foo || (foo = {}));
async function foo() {
}
`)

	// Namespace merging shouldn't allow for other merging
	expectParseErrorTS(t, "class foo {} namespace foo { 0 } class foo {}", errorText)
	expectParseErrorTS(t, "class foo {} namespace foo { 0 } enum foo {}", errorText)
	expectParseErrorTS(t, "enum foo {} namespace foo { 0 } class foo {}", errorText)
	expectParseErrorTS(t, "namespace foo { 0 } namespace foo { 0 } let foo", errorText)
	expectParseErrorTS(t, "namespace foo { 0 } enum foo {} class foo {}", errorText)

	// Test dot nested namespace syntax
	expectPrintedTS(t, "namespace foo.bar { foo(bar) }", `var foo;
((foo) => {
  let bar;
  ((bar) => {
    foo(bar);
  })(bar = foo.bar || (foo.bar = {}));
})(foo || (foo = {}));
`)

	// "module" is a deprecated alias for "namespace"
	expectPrintedTS(t, "module foo { export namespace bar { foo(bar) } }", `var foo;
((foo) => {
  let bar;
  ((bar) => {
    foo(bar);
  })(bar = foo.bar || (foo.bar = {}));
})(foo || (foo = {}));
`)
	expectPrintedTS(t, "namespace foo { export module bar { foo(bar) } }", `var foo;
((foo) => {
  let bar;
  ((bar) => {
    foo(bar);
  })(bar = foo.bar || (foo.bar = {}));
})(foo || (foo = {}));
`)
	expectPrintedTS(t, "module foo.bar { foo(bar) }", `var foo;
((foo) => {
  let bar;
  ((bar) => {
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
((A) => {
  let B;
  ((B) => {
    function fn() {
    }
    B.fn = fn;
  })(B = A.B || (A.B = {}));
  let C;
  ((C) => {
    function fn() {
    }
    C.fn = fn;
  })(C || (C = {}));
  let D;
  ((D) => {
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
((A) => {
  let B;
  ((B) => {
    class Class {
    }
    B.Class = Class;
  })(B = A.B || (A.B = {}));
  let C;
  ((C) => {
    class Class {
    }
    C.Class = Class;
  })(C || (C = {}));
  let D;
  ((D) => {
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
((A) => {
  let B;
  ((B) => {
    let Enum;
    ((Enum) => {
    })(Enum = B.Enum || (B.Enum = {}));
  })(B = A.B || (A.B = {}));
  let C;
  ((C) => {
    let Enum;
    ((Enum) => {
    })(Enum = C.Enum || (C.Enum = {}));
  })(C || (C = {}));
  let D;
  ((D) => {
    let Enum;
    ((Enum) => {
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
((A) => {
  let B;
  ((B) => {
    B.foo = 1;
    B.foo += B.foo;
  })(B = A.B || (A.B = {}));
  let C;
  ((C) => {
    C.foo = 1;
    C.foo += C.foo;
  })(C || (C = {}));
  let D;
  ((D) => {
    let foo = 1;
    foo += foo;
  })(D || (D = {}));
})(A || (A = {}));
`)

	expectPrintedTS(t, `
		namespace A {
			export namespace B {
				export const foo = 1
			}
			namespace C {
				export const foo = 1
			}
			namespace D {
				const foo = 1
			}
		}
	`, `var A;
((A) => {
  let B;
  ((B) => {
    B.foo = 1;
  })(B = A.B || (A.B = {}));
  let C;
  ((C) => {
    C.foo = 1;
  })(C || (C = {}));
  let D;
  ((D) => {
    const foo = 1;
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
((A) => {
  let B;
  ((B) => {
    B.foo = 1;
    B.foo += B.foo;
  })(B = A.B || (A.B = {}));
  let C;
  ((C) => {
    C.foo = 1;
    C.foo += C.foo;
  })(C || (C = {}));
  let D;
  ((D) => {
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
((ns) => {
  console.log(ns.L1);
  console.log(ns.L2, ns.L3);
  console.log(F);
  console.log(F2);
  console.log(C);
  console.log(E);
  console.log(N);
})(ns || (ns = {}));
`)

	expectPrintedTS(t, `
		namespace a { export var a = 123; log(a) }
		namespace b { export let b = 123; log(b) }
		namespace c { export enum c {} log(c) }
		namespace d { export class d {} log(d) }
		namespace e { export namespace e {} log(e) }
		namespace f { export function f() {} log(f) }
	`, `var a;
((_a) => {
  _a.a = 123;
  log(_a.a);
})(a || (a = {}));
var b;
((_b) => {
  _b.b = 123;
  log(_b.b);
})(b || (b = {}));
var c;
((_c) => {
  let c;
  ((c) => {
  })(c = _c.c || (_c.c = {}));
  log(c);
})(c || (c = {}));
var d;
((_d) => {
  class d {
  }
  _d.d = d;
  log(d);
})(d || (d = {}));
var e;
((e) => {
  log(e);
})(e || (e = {}));
var f;
((_f) => {
  function f() {
  }
  _f.f = f;
  log(f);
})(f || (f = {}));
`)

	expectPrintedTS(t, `
		namespace a { export declare var a }
		namespace b { export declare let b }
		namespace c { export declare enum c {} }
		namespace d { export declare class d {} }
		namespace e { export declare namespace e {} }
		namespace f { export declare function f() {} }
	`, `var a;
((_a) => {
})(a || (a = {}));
var b;
((_b) => {
})(b || (b = {}));
var c;
((c) => {
})(c || (c = {}));
var d;
((d) => {
})(d || (d = {}));
var f;
((f) => {
})(f || (f = {}));
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
((A) => {
  [
    A.a,
    [, A.b = c, ...A.d],
    { [x]: [[A.y]] = z, ...A.o }
  ] = ref;
})(A || (A = {}));
`)
}

func TestTSEnum(t *testing.T) {
	expectParseErrorTS(t, "enum x { y z }", "<stdin>: ERROR: Expected \",\" after \"y\" in enum\n")
	expectParseErrorTS(t, "enum x { 'y' 'z' }", "<stdin>: ERROR: Expected \",\" after \"y\" in enum\n")
	expectParseErrorTS(t, "enum x { y = 0 z }", "<stdin>: ERROR: Expected \",\" before \"z\" in enum\n")
	expectParseErrorTS(t, "enum x { 'y' = 0 'z' }", "<stdin>: ERROR: Expected \",\" before \"z\" in enum\n")

	// Check ES5 emit
	expectPrintedTargetTS(t, 5, "enum x { y = 1 }", "var x = /* @__PURE__ */ function(x) {\n  x[x[\"y\"] = 1] = \"y\";\n  return x;\n}(x || {});\n")
	expectPrintedTargetTS(t, 2015, "enum x { y = 1 }", "var x = /* @__PURE__ */ ((x) => {\n  x[x[\"y\"] = 1] = \"y\";\n  return x;\n})(x || {});\n")

	// Certain syntax isn't allowed inside an enum block
	expectParseErrorTS(t, "enum x { y = this }", "<stdin>: ERROR: Cannot use \"this\" here:\n")
	expectParseErrorTS(t, "enum x { y = () => this }", "<stdin>: ERROR: Cannot use \"this\" here:\n")
	expectParseErrorTS(t, "enum x { y = function() { this } }", "")

	expectPrintedTS(t, "enum Foo { A, B }", `var Foo = /* @__PURE__ */ ((Foo) => {
  Foo[Foo["A"] = 0] = "A";
  Foo[Foo["B"] = 1] = "B";
  return Foo;
})(Foo || {});
`)
	expectPrintedTS(t, "export enum Foo { A; B }", `export var Foo = /* @__PURE__ */ ((Foo) => {
  Foo[Foo["A"] = 0] = "A";
  Foo[Foo["B"] = 1] = "B";
  return Foo;
})(Foo || {});
`)
	expectPrintedTS(t, "enum Foo { A, B, C = 3.3, D, E }", `var Foo = /* @__PURE__ */ ((Foo) => {
  Foo[Foo["A"] = 0] = "A";
  Foo[Foo["B"] = 1] = "B";
  Foo[Foo["C"] = 3.3] = "C";
  Foo[Foo["D"] = 4.3] = "D";
  Foo[Foo["E"] = 5.3] = "E";
  return Foo;
})(Foo || {});
`)
	expectPrintedTS(t, "enum Foo { A, B, C = 'x', D, E, F = `y`, G = `${z}`, H = tag`` }", `var Foo = ((Foo) => {
  Foo[Foo["A"] = 0] = "A";
  Foo[Foo["B"] = 1] = "B";
  Foo["C"] = "x";
  Foo[Foo["D"] = void 0] = "D";
  Foo[Foo["E"] = void 0] = "E";
  Foo["F"] = `+"`y`"+`;
  Foo["G"] = `+"`${z}`"+`;
  Foo[Foo["H"] = tag`+"``"+`] = "H";
  return Foo;
})(Foo || {});
`)

	// TypeScript allows splitting an enum into multiple blocks
	expectPrintedTS(t, "enum Foo { A = 1 } enum Foo { B = 2 }", `var Foo = /* @__PURE__ */ ((Foo) => {
  Foo[Foo["A"] = 1] = "A";
  return Foo;
})(Foo || {});
var Foo = /* @__PURE__ */ ((Foo) => {
  Foo[Foo["B"] = 2] = "B";
  return Foo;
})(Foo || {});
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
	`, `var Foo = ((Foo) => {
  Foo[Foo["a"] = 10.01] = "a";
  Foo[Foo["a b"] = 100] = "a b";
  Foo[Foo["c"] = 120.02] = "c";
  Foo[Foo["d"] = 121.02] = "d";
  Foo[Foo["e"] = 120.02 + Math.random()] = "e";
  Foo[Foo["f"] = void 0] = "f";
  return Foo;
})(Foo || {});
var Bar = /* @__PURE__ */ ((Bar) => {
  Bar[Bar["a"] = 10.01 /* a */] = "a";
  return Bar;
})(Bar || {});
`)

	expectPrintedTS(t, `
		enum Foo { A }
		x = [Foo.A, Foo?.A, Foo?.A()]
		y = [Foo['A'], Foo?.['A'], Foo?.['A']()]
	`, `var Foo = /* @__PURE__ */ ((Foo) => {
  Foo[Foo["A"] = 0] = "A";
  return Foo;
})(Foo || {});
x = [0 /* A */, Foo?.A, Foo?.A()];
y = [0 /* A */, Foo?.["A"], Foo?.["A"]()];
`)

	// Check shadowing
	expectPrintedTS(t, "enum Foo { Foo }", `var Foo = /* @__PURE__ */ ((_Foo) => {
  _Foo[_Foo["Foo"] = 0] = "Foo";
  return _Foo;
})(Foo || {});
`)
	expectPrintedTS(t, "enum Foo { Bar = Foo }", `var Foo = /* @__PURE__ */ ((Foo) => {
  Foo[Foo["Bar"] = Foo] = "Bar";
  return Foo;
})(Foo || {});
`)
	expectPrintedTS(t, "enum Foo { Foo = 1, Bar = Foo }", `var Foo = /* @__PURE__ */ ((_Foo) => {
  _Foo[_Foo["Foo"] = 1] = "Foo";
  _Foo[_Foo["Bar"] = 1 /* Foo */] = "Bar";
  return _Foo;
})(Foo || {});
`)

	// Check top-level "var" and nested "let"
	expectPrintedTS(t, "enum a { b = 1 }", "var a = /* @__PURE__ */ ((a) => {\n  a[a[\"b\"] = 1] = \"b\";\n  return a;\n})(a || {});\n")
	expectPrintedTS(t, "{ enum a { b = 1 } }", "{\n  let a;\n  ((a) => {\n    a[a[\"b\"] = 1] = \"b\";\n  })(a || (a = {}));\n}\n")

	// Check "await" and "yield"
	expectPrintedTS(t, "enum x { await = 1, y = await }", `var x = /* @__PURE__ */ ((x) => {
  x[x["await"] = 1] = "await";
  x[x["y"] = 1 /* await */] = "y";
  return x;
})(x || {});
`)
	expectPrintedTS(t, "enum x { yield = 1, y = yield }", `var x = /* @__PURE__ */ ((x) => {
  x[x["yield"] = 1] = "yield";
  x[x["y"] = 1 /* yield */] = "y";
  return x;
})(x || {});
`)
	expectParseErrorTS(t, "enum x { y = await 1 }", "<stdin>: ERROR: \"await\" can only be used inside an \"async\" function\n")
	expectParseErrorTS(t, "function *f() { enum x { y = yield 1 } }", "<stdin>: ERROR: Cannot use \"yield\" outside a generator function\n")
	expectParseErrorTS(t, "async function f() { enum x { y = await 1 } }", "<stdin>: ERROR: \"await\" can only be used inside an \"async\" function\n")
	expectParseErrorTS(t, "export enum x { yield = 1, y = yield }",
		"<stdin>: ERROR: \"yield\" is a reserved word and cannot be used in an ECMAScript module\n"+
			"<stdin>: NOTE: This file is considered to be an ECMAScript module because of the \"export\" keyword here:\n")

	// Check enum use before declaration
	expectPrintedTS(t, "foo = Foo.FOO; enum Foo { FOO } bar = Foo.FOO", `foo = 0 /* FOO */;
var Foo = /* @__PURE__ */ ((Foo) => {
  Foo[Foo["FOO"] = 0] = "FOO";
  return Foo;
})(Foo || {});
bar = 0 /* FOO */;
`)

	// https://github.com/evanw/esbuild/issues/3205
	expectPrintedTS(t, "(() => { const enum Foo { A } () => Foo.A })", `() => {
  let Foo;
  ((Foo) => {
    Foo[Foo["A"] = 0] = "A";
  })(Foo || (Foo = {}));
  () => 0 /* A */;
};
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
	`, `var Foo = /* @__PURE__ */ ((Foo) => {
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
  return Foo;
})(Foo || {});
`)

	expectPrintedTS(t, `
		enum Foo {
			pos = +54321012345,
			neg = -54321012345,
			cpl = ~54321012345,

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
	`, `var Foo = /* @__PURE__ */ ((Foo) => {
  Foo[Foo["pos"] = 54321012345] = "pos";
  Foo[Foo["neg"] = -54321012345] = "neg";
  Foo[Foo["cpl"] = 1513562502] = "cpl";
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
  return Foo;
})(Foo || {});
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

	expectParseErrorTS(t, "function foo<>() {}", "<stdin>: ERROR: Expected identifier but found \">\"\n")
	expectParseErrorTS(t, "function foo<,>() {}", "<stdin>: ERROR: Expected identifier but found \",\"\n")
	expectParseErrorTS(t, "function foo<T><T>() {}", "<stdin>: ERROR: Expected \"(\" but found \"<\"\n")

	expectPrintedTS(t, "export default function <T>() {}", "export default function() {\n}\n")
	expectParseErrorTS(t, "export default function <>() {}", "<stdin>: ERROR: Expected identifier but found \">\"\n")
	expectParseErrorTS(t, "export default function <,>() {}", "<stdin>: ERROR: Expected identifier but found \",\"\n")
	expectParseErrorTS(t, "export default function <T><T>() {}", "<stdin>: ERROR: Expected \"(\" but found \"<\"\n")

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
	expectPrintedTS(t, "var a\n!b", "var a;\n!b;\n")
	expectPrintedTS(t, "let a\n!b", "let a;\n!b;\n")
	expectParseErrorTS(t, "var a!", "<stdin>: ERROR: Expected \":\" but found end of file\n")
	expectParseErrorTS(t, "var a! = ", "<stdin>: ERROR: Expected \":\" but found \"=\"\n")
	expectParseErrorTS(t, "var a!, b", "<stdin>: ERROR: Expected \":\" but found \",\"\n")

	expectPrinted(t, "a ? ({b}) => {} : c", "a ? ({ b }) => {\n} : c;\n")
	expectPrinted(t, "a ? (({b}) => {}) : c", "a ? ({ b }) => {\n} : c;\n")
	expectPrinted(t, "a ? (({b})) : c", "a ? { b } : c;\n")
	expectParseError(t, "a ? (({b})) => {} : c", "<stdin>: ERROR: Invalid binding pattern\n")
	expectPrintedTS(t, "a ? ({b}) => {} : c", "a ? ({ b }) => {\n} : c;\n")
	expectPrintedTS(t, "a ? (({b}) => {}) : c", "a ? ({ b }) => {\n} : c;\n")
	expectPrintedTS(t, "a ? (({b})) : c", "a ? { b } : c;\n")
	expectParseErrorTS(t, "a ? (({b})) => {} : c", "<stdin>: ERROR: Invalid binding pattern\n")
}

func TestTSDeclare(t *testing.T) {
	expectPrintedTS(t, "declare\nfoo", "declare;\nfoo;\n")
	expectPrintedTS(t, "declare\nvar foo", "declare;\nvar foo;\n")
	expectPrintedTS(t, "declare\nlet foo", "declare;\nlet foo;\n")
	expectPrintedTS(t, "declare\nconst foo = 0", "declare;\nconst foo = 0;\n")
	expectPrintedTS(t, "declare\nfunction foo() {}", "declare;\nfunction foo() {\n}\n")
	expectPrintedTS(t, "declare\nclass Foo {}", "declare;\nclass Foo {\n}\n")
	expectPrintedTS(t, "declare\nenum Foo {}", "declare;\nvar Foo = /* @__PURE__ */ ((Foo) => {\n})(Foo || {});\n")
	expectPrintedTS(t, "class Foo { declare \n foo }", "class Foo {\n  declare;\n  foo;\n}\n")

	expectPrintedTS(t, "declare;", "declare;\n")
	expectPrintedTS(t, "declare();", "declare();\n")
	expectPrintedTS(t, "declare[x];", "declare[x];\n")

	expectPrintedTS(t, "declare var x: number", "")
	expectPrintedTS(t, "declare let x: number", "")
	expectPrintedTS(t, "declare const x: number", "")
	expectPrintedTS(t, "declare var x = function() {}; function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare let x = function() {}; function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare const x = function() {}; function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare function fn(); function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare function fn()\n function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare function fn() {} function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare enum X {} function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare enum X { x = function() {} } function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare class X {} function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare class X { x = function() {} } function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare interface X {} function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare namespace X {} function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare namespace X { export var x = function() {} } function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare namespace X { export let x = function() {} } function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare namespace X { export const x = function() {} } function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare namespace X { export function fn() {} } function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare module X {} function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare module 'X' {} function scope() {}", "function scope() {\n}\n")
	expectPrintedTS(t, "declare module 'X'; let foo", "let foo;\n")
	expectPrintedTS(t, "declare module 'X'\nlet foo", "let foo;\n")
	expectPrintedTS(t, "declare module 'X' { let foo }", "")
	expectPrintedTS(t, "declare module 'X'\n{ let foo }", "")
	expectPrintedTS(t, "declare global { interface Foo {} let foo: any } let bar", "let bar;\n")
	expectPrintedTS(t, "declare module M { const x }", "")
	expectPrintedTS(t, "declare module M { global { const x } }", "")
	expectPrintedTS(t, "declare module M { global { const x } function foo() {} }", "")
	expectPrintedTS(t, "declare module M { global \n { const x } }", "")
	expectPrintedTS(t, "declare module M { import 'path' }", "")
	expectPrintedTS(t, "declare module M { import x from 'path' }", "")
	expectPrintedTS(t, "declare module M { import {x} from 'path' }", "")
	expectPrintedTS(t, "declare module M { import * as ns from 'path' }", "")
	expectPrintedTS(t, "declare module M { import foo = bar }", "")
	expectPrintedTS(t, "declare module M { export import foo = bar }", "")
	expectPrintedTS(t, "declare module M { export {x} from 'path' }", "")
	expectPrintedTS(t, "declare module M { export default 123 }", "")
	expectPrintedTS(t, "declare module M { export default function x() {} }", "")
	expectPrintedTS(t, "declare module M { export default class X {} }", "")
	expectPrintedTS(t, "declare module M { export * as ns from 'path' }", "")
	expectPrintedTS(t, "declare module M { export * from 'path' }", "")
	expectPrintedTS(t, "declare module M { export = foo }", "")
	expectPrintedTS(t, "declare module M { export as namespace ns }", "")
	expectPrintedTS(t, "declare module M { export as namespace ns; }", "")
	expectParseErrorTS(t, "declare module M { export as namespace ns.foo }", "<stdin>: ERROR: Expected \";\" but found \".\"\n")
	expectParseErrorTS(t, "declare module M { export as namespace ns function foo() {} }", "<stdin>: ERROR: Expected \";\" but found \"function\"\n")
	expectParseErrorTS(t, "module M { const x }", "<stdin>: ERROR: The constant \"x\" must be initialized\n")
	expectParseErrorTS(t, "module M { const [] }", "<stdin>: ERROR: This constant must be initialized\n")
	expectParseErrorTS(t, "module M { const {} }", "<stdin>: ERROR: This constant must be initialized\n")

	// This is a weird case where "," after a rest parameter is allowed
	expectPrintedTS(t, "declare function fn(x: any, ...y, )", "")
	expectPrintedTS(t, "declare function fn(x: any, ...y: any, )", "")
	expectParseErrorTS(t, "function fn(x: any, ...y, )", "<stdin>: ERROR: Expected \")\" but found \",\"\n")
	expectParseErrorTS(t, "function fn(x: any, ...y, ) {}", "<stdin>: ERROR: Expected \")\" but found \",\"\n")
	expectParseErrorTS(t, "function fn(x: any, ...y: any, )", "<stdin>: ERROR: Expected \")\" but found \",\"\n")
	expectParseErrorTS(t, "function fn(x: any, ...y: any, ) {}", "<stdin>: ERROR: Expected \")\" but found \",\"\n")

	// This declares a global module
	expectPrintedTS(t, "export as namespace ns", "")
	expectParseErrorTS(t, "export as namespace ns.foo", "<stdin>: ERROR: Expected \";\" but found \".\"\n")

	// TypeScript 4.4+ technically treats these as valid syntax, but I assume
	// this is a bug: https://github.com/microsoft/TypeScript/issues/54602
	expectParseErrorTS(t, "declare foo", "<stdin>: ERROR: Unexpected \"foo\"\n")
	expectParseErrorTS(t, "declare foo()", "<stdin>: ERROR: Unexpected \"foo\"\n")
	expectParseErrorTS(t, "declare {foo}", "<stdin>: ERROR: Unexpected \"{\"\n")
}

func TestTSExperimentalDecorator(t *testing.T) {
	// Tests of "declare class"
	expectPrintedExperimentalDecoratorTS(t, "@dec(() => 0) declare class Foo {} {let foo}", "{\n  let foo;\n}\n")
	expectPrintedExperimentalDecoratorTS(t, "@dec(() => 0) declare abstract class Foo {} {let foo}", "{\n  let foo;\n}\n")
	expectPrintedExperimentalDecoratorTS(t, "@dec(() => 0) export declare class Foo {} {let foo}", "{\n  let foo;\n}\n")
	expectPrintedExperimentalDecoratorTS(t, "@dec(() => 0) export declare abstract class Foo {} {let foo}", "{\n  let foo;\n}\n")
	expectPrintedExperimentalDecoratorTS(t, "declare class Foo { @dec(() => 0) foo } {let foo}", "{\n  let foo;\n}\n")
	expectPrintedExperimentalDecoratorTS(t, "declare class Foo { @dec(() => 0) foo() } {let foo}", "{\n  let foo;\n}\n")
	expectPrintedExperimentalDecoratorTS(t, "declare class Foo { foo(@dec(() => 0) x) } {let foo}", "{\n  let foo;\n}\n")

	// Decorators must only work on class statements
	notes := "<stdin>: NOTE: The preceding decorator is here:\n" +
		"NOTE: Decorators can only be used with class declarations.\n"
	expectParseErrorExperimentalDecoratorTS(t, "@dec enum foo {}", "<stdin>: ERROR: Expected \"class\" after decorator but found \"enum\"\n"+notes)
	expectParseErrorExperimentalDecoratorTS(t, "@dec namespace foo {}", "<stdin>: ERROR: Expected \"class\" after decorator but found \"namespace\"\n"+notes)
	expectParseErrorExperimentalDecoratorTS(t, "@dec function foo() {}", "<stdin>: ERROR: Expected \"class\" after decorator but found \"function\"\n"+notes)
	expectParseErrorExperimentalDecoratorTS(t, "@dec abstract", "<stdin>: ERROR: Expected \"class\" but found end of file\n")
	expectParseErrorExperimentalDecoratorTS(t, "@dec declare: x", "<stdin>: ERROR: Expected \"class\" after decorator but found \":\"\n"+notes)
	expectParseErrorExperimentalDecoratorTS(t, "@dec declare enum foo {}", "<stdin>: ERROR: Expected \"class\" after decorator but found \"enum\"\n"+notes)
	expectParseErrorExperimentalDecoratorTS(t, "@dec declare namespace foo {}", "<stdin>: ERROR: Expected \"class\" after decorator but found \"namespace\"\n"+notes)
	expectParseErrorExperimentalDecoratorTS(t, "@dec declare function foo()", "<stdin>: ERROR: Expected \"class\" after decorator but found \"function\"\n"+notes)
	expectParseErrorExperimentalDecoratorTS(t, "@dec export {}", "<stdin>: ERROR: Expected \"class\" after decorator but found \"{\"\n"+notes)
	expectParseErrorExperimentalDecoratorTS(t, "@dec export enum foo {}", "<stdin>: ERROR: Expected \"class\" after decorator but found \"enum\"\n"+notes)
	expectParseErrorExperimentalDecoratorTS(t, "@dec export namespace foo {}", "<stdin>: ERROR: Expected \"class\" after decorator but found \"namespace\"\n"+notes)
	expectParseErrorExperimentalDecoratorTS(t, "@dec export function foo() {}", "<stdin>: ERROR: Expected \"class\" after decorator but found \"function\"\n"+notes)
	expectParseErrorExperimentalDecoratorTS(t, "@dec export default abstract", "<stdin>: ERROR: Expected \"class\" but found end of file\n")
	expectParseErrorExperimentalDecoratorTS(t, "@dec export declare enum foo {}", "<stdin>: ERROR: Expected \"class\" after decorator but found \"enum\"\n"+notes)
	expectParseErrorExperimentalDecoratorTS(t, "@dec export declare namespace foo {}", "<stdin>: ERROR: Expected \"class\" after decorator but found \"namespace\"\n"+notes)
	expectParseErrorExperimentalDecoratorTS(t, "@dec export declare function foo()", "<stdin>: ERROR: Expected \"class\" after decorator but found \"function\"\n"+notes)

	// Decorators must be forbidden outside class statements
	note := "<stdin>: NOTE: This is a class expression, not a class declaration:\n"
	expectParseErrorExperimentalDecoratorTS(t, "(class { @dec foo })", "<stdin>: ERROR: Experimental decorators can only be used with class declarations in TypeScript\n"+note)
	expectParseErrorExperimentalDecoratorTS(t, "(class { @dec foo() {} })", "<stdin>: ERROR: Experimental decorators can only be used with class declarations in TypeScript\n"+note)
	expectParseErrorExperimentalDecoratorTS(t, "(class { foo(@dec x) {} })", "<stdin>: ERROR: Experimental decorators can only be used with class declarations in TypeScript\n"+note)
	expectParseErrorExperimentalDecoratorTS(t, "({ @dec foo })", "<stdin>: ERROR: Expected identifier but found \"@\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "({ @dec foo() {} })", "<stdin>: ERROR: Expected identifier but found \"@\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "({ foo(@dec x) {} })", "<stdin>: ERROR: Expected identifier but found \"@\"\n")

	// Decorators aren't allowed with private names
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec #foo }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec #foo = 1 }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec #foo() {} }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec *#foo() {} }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec async #foo() {} }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec async* #foo() {} }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec static #foo }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec static #foo = 1 }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec static #foo() {} }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec static *#foo() {} }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec static async #foo() {} }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec static async* #foo() {} }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")

	// Decorators aren't allowed on class constructors
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec constructor() {} }", "<stdin>: ERROR: Decorators are not allowed on class constructors\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec public constructor() {} }", "<stdin>: ERROR: Decorators are not allowed on class constructors\n")

	// Check use of "await"
	friendlyAwaitErrorWithNote := "<stdin>: ERROR: \"await\" can only be used inside an \"async\" function\n" +
		"<stdin>: NOTE: Consider adding the \"async\" keyword here:\n"
	expectPrintedExperimentalDecoratorTS(t, "async function foo() { @dec(await x) class Foo {} }",
		"async function foo() {\n  let Foo = class {\n  };\n  Foo = __decorateClass([\n    dec(await x)\n  ], Foo);\n}\n")
	expectPrintedExperimentalDecoratorTS(t, "async function foo() { class Foo { @dec(await x) foo() {} } }",
		"async function foo() {\n  class Foo {\n    foo() {\n    }\n  }\n  __decorateClass([\n    dec(await x)\n  ], Foo.prototype, \"foo\", 1);\n}\n")
	expectPrintedExperimentalDecoratorTS(t, "async function foo() { class Foo { foo(@dec(await x) y) {} } }",
		"async function foo() {\n  class Foo {\n    foo(y) {\n    }\n  }\n  __decorateClass([\n    __decorateParam(0, dec(await x))\n  ], Foo.prototype, \"foo\", 1);\n}\n")
	expectParseErrorExperimentalDecoratorTS(t, "function foo() { @dec(await x) class Foo {} }", friendlyAwaitErrorWithNote)
	expectParseErrorExperimentalDecoratorTS(t, "function foo() { class Foo { @dec(await x) foo() {} } }", friendlyAwaitErrorWithNote)
	expectParseErrorExperimentalDecoratorTS(t, "function foo() { class Foo { foo(@dec(await x) y) {} } }", friendlyAwaitErrorWithNote)
	expectParseErrorExperimentalDecoratorTS(t, "function foo() { class Foo { @dec(await x) async foo() {} } }", friendlyAwaitErrorWithNote)
	expectParseErrorExperimentalDecoratorTS(t, "function foo() { class Foo { async foo(@dec(await x) y) {} } }",
		"<stdin>: ERROR: The keyword \"await\" cannot be used here:\n<stdin>: ERROR: Expected \")\" but found \"x\"\n")

	// Check lowered use of "await"
	expectPrintedTargetExperimentalDecoratorTS(t, 2015, "async function foo() { @dec(await x) class Foo {} }",
		`function foo() {
  return __async(this, null, function* () {
    let Foo = class {
    };
    Foo = __decorateClass([
      dec(yield x)
    ], Foo);
  });
}
`)
	expectPrintedTargetExperimentalDecoratorTS(t, 2015, "async function foo() { class Foo { @dec(await x) foo() {} } }",
		`function foo() {
  return __async(this, null, function* () {
    class Foo {
      foo() {
      }
    }
    __decorateClass([
      dec(yield x)
    ], Foo.prototype, "foo", 1);
  });
}
`)
	expectPrintedTargetExperimentalDecoratorTS(t, 2015, "async function foo() { class Foo { foo(@dec(await x) y) {} } }",
		`function foo() {
  return __async(this, null, function* () {
    class Foo {
      foo(y) {
      }
    }
    __decorateClass([
      __decorateParam(0, dec(yield x))
    ], Foo.prototype, "foo", 1);
  });
}
`)

	// Check use of "yield"
	expectPrintedExperimentalDecoratorTS(t, "function *foo() { @dec(yield x) class Foo {} }", // We currently allow this but TypeScript doesn't
		"function* foo() {\n  let Foo = class {\n  };\n  Foo = __decorateClass([\n    dec(yield x)\n  ], Foo);\n}\n")
	expectPrintedExperimentalDecoratorTS(t, "function *foo() { class Foo { @dec(yield x) foo() {} } }", // We currently allow this but TypeScript doesn't
		"function* foo() {\n  class Foo {\n    foo() {\n    }\n  }\n  __decorateClass([\n    dec(yield x)\n  ], Foo.prototype, \"foo\", 1);\n}\n")
	expectParseErrorExperimentalDecoratorTS(t, "function *foo() { class Foo { foo(@dec(yield x) y) {} } }", // TypeScript doesn't allow this (although it could because it would work fine)
		"<stdin>: ERROR: Cannot use \"yield\" outside a generator function\n")
	expectParseErrorExperimentalDecoratorTS(t, "function foo() { @dec(yield x) class Foo {} }", "<stdin>: ERROR: Cannot use \"yield\" outside a generator function\n")
	expectParseErrorExperimentalDecoratorTS(t, "function foo() { class Foo { @dec(yield x) foo() {} } }", "<stdin>: ERROR: Cannot use \"yield\" outside a generator function\n")

	// Check inline function expressions
	expectPrintedExperimentalDecoratorTS(t, "@((x, y) => x + y) class Foo {}",
		"let Foo = class {\n};\nFoo = __decorateClass([\n  (x, y) => x + y\n], Foo);\n")
	expectPrintedExperimentalDecoratorTS(t, "@((x, y) => x + y) export class Foo {}",
		"export let Foo = class {\n};\nFoo = __decorateClass([\n  (x, y) => x + y\n], Foo);\n")
	expectPrintedExperimentalDecoratorTS(t, "@(function(x, y) { return x + y }) class Foo {}",
		"let Foo = class {\n};\nFoo = __decorateClass([\n  function(x, y) {\n    return x + y;\n  }\n], Foo);\n")
	expectPrintedExperimentalDecoratorTS(t, "@(function(x, y) { return x + y }) export class Foo {}",
		"export let Foo = class {\n};\nFoo = __decorateClass([\n  function(x, y) {\n    return x + y;\n  }\n], Foo);\n")

	// Don't allow decorators on static blocks
	expectPrintedTS(t, "class Foo { static }", "class Foo {\n  static;\n}\n")
	expectPrintedExperimentalDecoratorTS(t, "class Foo { @dec static }", "class Foo {\n  static;\n}\n__decorateClass([\n  dec\n], Foo.prototype, \"static\", 2);\n")
	expectParseErrorExperimentalDecoratorTS(t, "class Foo { @dec static {} }", "<stdin>: ERROR: Expected \";\" but found \"{\"\n")

	// TypeScript experimental decorators allow more expressions than JavaScript decorators
	expectPrintedExperimentalDecoratorTS(t, "@x() class Foo {}", "let Foo = class {\n};\nFoo = __decorateClass([\n  x()\n], Foo);\n")
	expectPrintedExperimentalDecoratorTS(t, "@x.y() class Foo {}", "let Foo = class {\n};\nFoo = __decorateClass([\n  x.y()\n], Foo);\n")
	expectPrintedExperimentalDecoratorTS(t, "@(() => {}) class Foo {}", "let Foo = class {\n};\nFoo = __decorateClass([\n  () => {\n  }\n], Foo);\n")
	expectPrintedExperimentalDecoratorTS(t, "@123 class Foo {}", "let Foo = class {\n};\nFoo = __decorateClass([\n  123\n], Foo);\n")
	expectPrintedExperimentalDecoratorTS(t, "@x?.() class Foo {}", "let Foo = class {\n};\nFoo = __decorateClass([\n  x?.()\n], Foo);\n")
	expectPrintedExperimentalDecoratorTS(t, "@x?.y() class Foo {}", "let Foo = class {\n};\nFoo = __decorateClass([\n  x?.y()\n], Foo);\n")
	expectPrintedExperimentalDecoratorTS(t, "@x?.[y]() class Foo {}", "let Foo = class {\n};\nFoo = __decorateClass([\n  x?.[y]()\n], Foo);\n")
	expectPrintedExperimentalDecoratorTS(t, "@new Function() class Foo {}", "let Foo = class {\n};\nFoo = __decorateClass([\n  new Function()\n], Foo);\n")
	expectParseErrorExperimentalDecoratorTS(t, "@x[y] class Foo {}",
		"<stdin>: ERROR: Expected \"class\" after decorator but found \"[\"\n<stdin>: NOTE: The preceding decorator is here:\n"+
			"NOTE: Decorators can only be used with class declarations.\n<stdin>: ERROR: Expected \";\" but found \"class\"\n")
	expectParseErrorExperimentalDecoratorTS(t, "@() => {} class Foo {}", "<stdin>: ERROR: Unexpected \")\"\n")
}

func TestTSDecorators(t *testing.T) {
	expectPrintedTS(t, "@x @y class Foo {}", "@x\n@y\nclass Foo {\n}\n")
	expectPrintedTS(t, "@x @y export class Foo {}", "@x\n@y\nexport class Foo {\n}\n")
	expectPrintedTS(t, "@x @y export default class Foo {}", "@x\n@y\nexport default class Foo {\n}\n")
	expectPrintedTS(t, "_ = @x @y class {}", "_ = @x @y class {\n};\n")

	expectPrintedTS(t, "class Foo { @x y: any }", "class Foo {\n  @x\n  y;\n}\n")
	expectPrintedTS(t, "class Foo { @x y(): any {} }", "class Foo {\n  @x\n  y() {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { @x static y: any }", "class Foo {\n  @x\n  static y;\n}\n")
	expectPrintedTS(t, "class Foo { @x static y(): any {} }", "class Foo {\n  @x\n  static y() {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { @x accessor y: any }", "class Foo {\n  @x\n  accessor y;\n}\n")

	expectPrintedTS(t, "class Foo { @x #y: any }", "class Foo {\n  @x\n  #y;\n}\n")
	expectPrintedTS(t, "class Foo { @x #y(): any {} }", "class Foo {\n  @x\n  #y() {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { @x static #y: any }", "class Foo {\n  @x\n  static #y;\n}\n")
	expectPrintedTS(t, "class Foo { @x static #y(): any {} }", "class Foo {\n  @x\n  static #y() {\n  }\n}\n")
	expectPrintedTS(t, "class Foo { @x accessor #y: any }", "class Foo {\n  @x\n  accessor #y;\n}\n")

	expectParseErrorTS(t, "class Foo { x(@y z) {} }", "<stdin>: ERROR: Parameter decorators only work when experimental decorators are enabled\n"+
		"NOTE: You can enable experimental decorators by adding \"experimentalDecorators\": true to your \"tsconfig.json\" file.\n")
	expectParseErrorTS(t, "class Foo { @x static {} }", "<stdin>: ERROR: Expected \";\" but found \"{\"\n")

	expectPrintedTS(t, "@\na\n(\n)\n@\n(\nb\n)\nclass\nFoo\n{\n}\n", "@a()\n@b\nclass Foo {\n}\n")
	expectPrintedTS(t, "@(a, b) class Foo {}\n", "@(a, b)\nclass Foo {\n}\n")
	expectPrintedTS(t, "@x() class Foo {}", "@x()\nclass Foo {\n}\n")
	expectPrintedTS(t, "@x.y() class Foo {}", "@x.y()\nclass Foo {\n}\n")
	expectPrintedTS(t, "@(() => {}) class Foo {}", "@(() => {\n})\nclass Foo {\n}\n")
	expectPrintedTS(t, "class Foo { #x = @y.#x.y.#x class {} }", "class Foo {\n  #x = @y.#x.y.#x class {\n  };\n}\n")
	expectParseErrorTS(t, "@123 class Foo {}", "<stdin>: ERROR: Expected identifier but found \"123\"\n")
	expectParseErrorTS(t, "@x[y] class Foo {}",
		"<stdin>: ERROR: Expected \"class\" after decorator but found \"[\"\n<stdin>: NOTE: The preceding decorator is here:\n"+
			"NOTE: Decorators can only be used with class declarations.\n<stdin>: ERROR: Expected \";\" but found \"class\"\n")
	expectParseErrorTS(t, "@x?.() class Foo {}", "<stdin>: ERROR: Expected \".\" but found \"?.\"\n")
	expectParseErrorTS(t, "@x?.y() class Foo {}", "<stdin>: ERROR: Expected \".\" but found \"?.\"\n")
	expectParseErrorTS(t, "@x?.[y]() class Foo {}", "<stdin>: ERROR: Expected \".\" but found \"?.\"\n")
	expectParseErrorTS(t, "@new Function() class Foo {}", "<stdin>: ERROR: Expected identifier but found \"new\"\n")
	expectParseErrorTS(t, "@() => {} class Foo {}", "<stdin>: ERROR: Unexpected \")\"\n")

	expectPrintedTS(t, "class Foo { @x<{}> y: any }", "class Foo {\n  @x\n  y;\n}\n")
	expectPrintedTS(t, "class Foo { @x<{}>() y: any }", "class Foo {\n  @x()\n  y;\n}\n")
	expectPrintedTS(t, "class Foo { @x<{}> @y<[], () => {}> z: any }", "class Foo {\n  @x\n  @y\n  z;\n}\n")
	expectPrintedTS(t, "class Foo { @x<{}>() @y<[], () => {}>() z: any }", "class Foo {\n  @x()\n  @y()\n  z;\n}\n")
	expectPrintedTS(t, "class Foo { @x<{}>.y<[], () => {}> z: any }", "class Foo {\n  @x.y\n  z;\n}\n")

	// TypeScript 5.0+ allows this but Babel doesn't. I believe this is a bug
	// with TypeScript: https://github.com/microsoft/TypeScript/issues/55336
	expectParseErrorTS(t, "class Foo { @x<{}>().y<[], () => {}>() z: any }", "<stdin>: ERROR: Expected identifier but found \".\"\n")

	errorText := "<stdin>: ERROR: Transforming JavaScript decorators to the configured target environment is not supported yet\n"
	expectParseErrorWithUnsupportedFeaturesTS(t, compat.Decorators, "@dec class Foo {}", errorText)
	expectParseErrorWithUnsupportedFeaturesTS(t, compat.Decorators, "class Foo { @dec x }", errorText)
	expectParseErrorWithUnsupportedFeaturesTS(t, compat.Decorators, "class Foo { @dec x() {} }", errorText)
	expectParseErrorWithUnsupportedFeaturesTS(t, compat.Decorators, "class Foo { @dec accessor x }", errorText)
	expectParseErrorWithUnsupportedFeaturesTS(t, compat.Decorators, "class Foo { @dec static x }", errorText)
	expectParseErrorWithUnsupportedFeaturesTS(t, compat.Decorators, "class Foo { @dec static x() {} }", errorText)
	expectParseErrorWithUnsupportedFeaturesTS(t, compat.Decorators, "class Foo { @dec static accessor x }", errorText)
}

func TestTSTry(t *testing.T) {
	expectPrintedTS(t, "try {} catch (x: any) {}", "try {\n} catch (x) {\n}\n")
	expectPrintedTS(t, "try {} catch (x: unknown) {}", "try {\n} catch (x) {\n}\n")
	expectPrintedTS(t, "try {} catch (x: number) {}", "try {\n} catch (x) {\n}\n")

	expectPrintedTS(t, "try {} catch ({x}: any) {}", "try {\n} catch ({ x }) {\n}\n")
	expectPrintedTS(t, "try {} catch ({x}: unknown) {}", "try {\n} catch ({ x }) {\n}\n")
	expectPrintedTS(t, "try {} catch ({x}: number) {}", "try {\n} catch ({ x }) {\n}\n")

	expectPrintedTS(t, "try {} catch ([x]: any) {}", "try {\n} catch ([x]) {\n}\n")
	expectPrintedTS(t, "try {} catch ([x]: unknown) {}", "try {\n} catch ([x]) {\n}\n")
	expectPrintedTS(t, "try {} catch ([x]: number) {}", "try {\n} catch ([x]) {\n}\n")

	expectParseErrorTS(t, "try {} catch (x!) {}", "<stdin>: ERROR: Expected \")\" but found \"!\"\n")
	expectParseErrorTS(t, "try {} catch (x!: any) {}", "<stdin>: ERROR: Expected \")\" but found \"!\"\n")
	expectParseErrorTS(t, "try {} catch (x!: unknown) {}", "<stdin>: ERROR: Expected \")\" but found \"!\"\n")
}

func TestTSArrow(t *testing.T) {
	expectPrintedTS(t, "(a?) => {}", "(a) => {\n};\n")
	expectPrintedTS(t, "(a?: number) => {}", "(a) => {\n};\n")
	expectPrintedTS(t, "(a?: number = 0) => {}", "(a = 0) => {\n};\n")
	expectParseErrorTS(t, "(a? = 0) => {}", "<stdin>: ERROR: Unexpected \"=\"\n")

	expectPrintedTS(t, "(a?, b) => {}", "(a, b) => {\n};\n")
	expectPrintedTS(t, "(a?: number, b) => {}", "(a, b) => {\n};\n")
	expectPrintedTS(t, "(a?: number = 0, b) => {}", "(a = 0, b) => {\n};\n")
	expectParseErrorTS(t, "(a? = 0, b) => {}", "<stdin>: ERROR: Unexpected \"=\"\n")

	expectPrintedTS(t, "(a: number) => {}", "(a) => {\n};\n")
	expectPrintedTS(t, "(a: number = 0) => {}", "(a = 0) => {\n};\n")
	expectPrintedTS(t, "(a: number, b) => {}", "(a, b) => {\n};\n")

	expectPrintedTS(t, "(): void => {}", "() => {\n};\n")
	expectPrintedTS(t, "(a): void => {}", "(a) => {\n};\n")
	expectParseErrorTS(t, "x: void => {}", "<stdin>: ERROR: Unexpected \"=>\"\n")
	expectPrintedTS(t, "a ? (1 + 2) : (3 + 4)", "a ? 1 + 2 : 3 + 4;\n")
	expectPrintedTS(t, "(foo) ? (foo as Bar) : null;", "foo ? foo : null;\n")
	expectPrintedTS(t, "((foo) ? (foo as Bar) : null)", "foo ? foo : null;\n")
	expectPrintedTS(t, "let x = a ? (b, c) : (d, e)", "let x = a ? (b, c) : (d, e);\n")

	expectPrintedTS(t, "let x: () => void = () => {}", "let x = () => {\n};\n")
	expectPrintedTS(t, "let x: (y) => void = () => {}", "let x = () => {\n};\n")
	expectPrintedTS(t, "let x: (this) => void = () => {}", "let x = () => {\n};\n")
	expectPrintedTS(t, "let x: (this: any) => void = () => {}", "let x = () => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): (() => {}) => {};", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): () => {} => {};", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): (y) => {} => {};", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ([,[b]]) => {} => {};", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ([a,[b]]) => {} => {};", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ([a,[b],]) => {} => {};", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ({a}) => {} => {};", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ({a,}) => {} => {};", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ({a:{b}}) => {} => {};", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ({0:{b}}) => {} => {};", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ({'a':{b}}) => {} => {};", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ({if:{b}}) => {} => {};", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ({...a}) => {} => {};", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): ({a,...b}) => {} => {};", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): (y[]) => {};", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "let x = (y: any): (a | b) => {};", "let x = (y) => {\n};\n")
	expectPrintedTS(t, "type x = ({...fi}) => {};", "")
	expectParseErrorTS(t, "let x = (y: any): (y) => {};", "<stdin>: ERROR: Unexpected \":\"\n")
	expectParseErrorTS(t, "let x = (y: any): (y) => {return 0};", "<stdin>: ERROR: Unexpected \":\"\n")
	expectParseErrorTS(t, "let x = (y: any): asserts y is (y) => {};", "<stdin>: ERROR: Unexpected \":\"\n")
	expectParseErrorTS(t, "type x = ({...if}) => {};", "<stdin>: ERROR: Unexpected \"...\"\n")

	expectPrintedTS(t, "async (): void => {}", "async () => {\n};\n")
	expectPrintedTS(t, "async (a): void => {}", "async (a) => {\n};\n")
	expectParseErrorTS(t, "async x: void => {}", "<stdin>: ERROR: Expected \"=>\" but found \":\"\n")

	expectPrintedTS(t, "function foo(x: boolean): asserts x", "")
	expectPrintedTS(t, "function foo(x: boolean): asserts<T>", "")
	expectPrintedTS(t, "function foo(x: boolean): asserts\nx", "x;\n")
	expectPrintedTS(t, "function foo(x: boolean): asserts<T>\nx", "x;\n")
	expectParseErrorTS(t, "function foo(x: boolean): asserts<T> x", "<stdin>: ERROR: Expected \";\" but found \"x\"\n")
	expectPrintedTS(t, "(x: boolean): asserts x => {}", "(x) => {\n};\n")
	expectPrintedTS(t, "(x: boolean): asserts this is object => {}", "(x) => {\n};\n")
	expectPrintedTS(t, "(x: T): asserts x is NonNullable<T> => {}", "(x) => {\n};\n")
	expectPrintedTS(t, "(x: any): x is number => {}", "(x) => {\n};\n")
	expectPrintedTS(t, "(x: any): this is object => {}", "(x) => {\n};\n")
	expectPrintedTS(t, "(x: any): (() => void) => {}", "(x) => {\n};\n")
	expectPrintedTS(t, "(x: any): ((y: any) => void) => {}", "(x) => {\n};\n")
	expectPrintedTS(t, "function foo(this: any): this is number {}", "function foo() {\n}\n")
	expectPrintedTS(t, "function foo(this: any): asserts this is number {}", "function foo() {\n}\n")
	expectPrintedTS(t, "(symbol: any): symbol is number => {}", "(symbol) => {\n};\n")

	expectPrintedTS(t, "let x: () => {} | ({y: z});", "let x;\n")
	expectPrintedTS(t, "function x(): ({y: z}) {}", "function x() {\n}\n")

	expectParseErrorTargetTS(t, 5, "return check ? (hover = 2, bar) : baz()", "")
	expectParseErrorTargetTS(t, 5, "return check ? (hover = 2, bar) => 0 : baz()",
		"<stdin>: ERROR: Transforming default arguments to the configured target environment is not supported yet\n")
}

func TestTSSuperCall(t *testing.T) {
	expectPrintedAssignSemanticsTS(t, "class A extends B { x = 1 }",
		`class A extends B {
  constructor() {
    super(...arguments);
    this.x = 1;
  }
}
`)

	expectPrintedAssignSemanticsTS(t, "class A extends B { x }",
		`class A extends B {
}
`)

	expectPrintedAssignSemanticsTS(t, "class A extends B { x = 1; constructor() { foo() } }",
		`class A extends B {
  constructor() {
    this.x = 1;
    foo();
  }
}
`)

	expectPrintedAssignSemanticsTS(t, "class A extends B { x; constructor() { foo() } }",
		`class A extends B {
  constructor() {
    foo();
  }
}
`)

	expectPrintedAssignSemanticsTS(t, "class A extends B { x = 1; constructor() { foo(); super(1); } }",
		`class A extends B {
  constructor() {
    foo();
    super(1);
    this.x = 1;
  }
}
`)

	expectPrintedAssignSemanticsTS(t, "class A extends B { x; constructor() { foo(); super(1); } }",
		`class A extends B {
  constructor() {
    foo();
    super(1);
  }
}
`)

	expectPrintedAssignSemanticsTS(t, "class A extends B { constructor(public x = 1) { foo(); super(1); } }",
		`class A extends B {
  constructor(x = 1) {
    foo();
    super(1);
    this.x = x;
  }
}
`)

	expectPrintedAssignSemanticsTS(t, "class A extends B { constructor(public x = 1) { foo(); super(1); super(2); } }",
		`class A extends B {
  constructor(x = 1) {
    var __super = (...args) => {
      super(...args);
      this.x = x;
    };
    foo();
    __super(1);
    __super(2);
  }
}
`)

	expectPrintedAssignSemanticsTS(t, "class A extends B { constructor(public x = 1) { if (false) super(1); super(2); } }", `class A extends B {
  constructor(x = 1) {
    if (false)
      __super(1);
    super(2);
    this.x = x;
  }
}
`)

	expectPrintedAssignSemanticsTS(t, "class A extends B { constructor(public x = 1) { if (foo) super(1); super(2); } }", `class A extends B {
  constructor(x = 1) {
    var __super = (...args) => {
      super(...args);
      this.x = x;
    };
    if (foo)
      __super(1);
    __super(2);
  }
}
`)

	expectPrintedAssignSemanticsTS(t, "class A extends B { constructor(public x = 1) { if (foo) super(1); else super(2); } }", `class A extends B {
  constructor(x = 1) {
    var __super = (...args) => {
      super(...args);
      this.x = x;
    };
    if (foo)
      __super(1);
    else
      __super(2);
  }
}
`)
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
	expectPrintedTS(t, "new Foo<number>", "new Foo();\n")
	expectPrintedTS(t, "new Foo<number, boolean>", "new Foo();\n")

	expectPrintedTS(t, "new Foo!()", "new Foo();\n")
	expectPrintedTS(t, "new Foo!<number>()", "new Foo();\n")
	expectPrintedTS(t, "new Foo!.Bar()", "new Foo.Bar();\n")
	expectPrintedTS(t, "new Foo!.Bar<number>()", "new Foo.Bar();\n")
	expectPrintedTS(t, "new Foo!['Bar']()", "new Foo[\"Bar\"]();\n")
	expectPrintedTS(t, "new Foo\n!(x)", "new Foo();\n!x;\n")
	expectPrintedTS(t, "new Foo<number>!(x)", "new Foo() < number > !x;\n")
	expectParseErrorTS(t, "new Foo<number>!()", "<stdin>: ERROR: Unexpected \")\"\n")
	expectParseErrorTS(t, "new Foo\n!.Bar()", "<stdin>: ERROR: Unexpected \".\"\n")
	expectParseError(t, "new Foo!()", "<stdin>: ERROR: Unexpected \"!\"\n")
}

func TestTSInstantiationExpression(t *testing.T) {
	expectPrintedTS(t, "f<number>", "f;\n")
	expectPrintedTS(t, "f<number, boolean>", "f;\n")
	expectPrintedTS(t, "f.g<number>", "f.g;\n")
	expectPrintedTS(t, "f<number>.g", "f.g;\n")
	expectPrintedTS(t, "f<number>.g<number>", "f.g;\n")
	expectPrintedTS(t, "f['g']<number>", "f[\"g\"];\n")
	expectPrintedTS(t, "(f<number>)<number>", "f;\n")

	// Function call
	expectPrintedTS(t, "const x1 = f<true>\n(true);", "const x1 = f(true);\n")
	// Relational expression
	expectPrintedTS(t, "const x1 = f<true>\ntrue;", "const x1 = f;\ntrue;\n")
	// Instantiation expression
	expectPrintedTS(t, "const x1 = f<true>;\n(true);", "const x1 = f;\ntrue;\n")

	// Trailing commas are not allowed
	expectPrintedTS(t, "const x = Array<number>\n(0);", "const x = Array(0);\n")
	expectPrintedTS(t, "const x = Array<number>;\n(0);", "const x = Array;\n0;\n")
	expectParseErrorTS(t, "const x = Array<number,>\n(0);", "<stdin>: ERROR: Expected identifier but found \">\"\n")
	expectParseErrorTS(t, "const x = Array<number,>;\n(0);", "<stdin>: ERROR: Expected identifier but found \">\"\n")

	expectPrintedTS(t, "f<number>?.();", "f?.();\n")
	expectPrintedTS(t, "f?.<number>();", "f?.();\n")
	expectPrintedTS(t, "f<<T>() => T>?.();", "f?.();\n")
	expectPrintedTS(t, "f?.<<T>() => T>();", "f?.();\n")

	expectPrintedTS(t, "f<number>['g'];", "f < number > [\"g\"];\n")

	expectPrintedTS(t, "type T21 = typeof Array<string>; f();", "f();\n")
	expectPrintedTS(t, "type T22 = typeof Array<string, number>; f();", "f();\n")

	expectPrintedTS(t, "f<x>, g<y>;", "f, g;\n")
	expectPrintedTS(t, "f<<T>() => T>;", "f;\n")
	expectPrintedTS(t, "f.x<<T>() => T>;", "f.x;\n")
	expectPrintedTS(t, "f['x']<<T>() => T>;", "f[\"x\"];\n")
	expectPrintedTS(t, "f<x>g<y>;", "f < x > g;\n")
	expectPrintedTS(t, "f<x>=g<y>;", "f < x >= g;\n")
	expectPrintedTS(t, "f<x>>g<y>;", "f < x >> g;\n")
	expectPrintedTS(t, "f<x>>>g<y>;", "f < x >>> g;\n")
	expectParseErrorTS(t, "f<x>>=g<y>;", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseErrorTS(t, "f<x>>>=g<y>;", "<stdin>: ERROR: Invalid assignment target\n")
	expectPrintedTS(t, "f<x,y>g<y>;", "f < x, y > g;\n")
	expectPrintedTS(t, "f<x,y>=g<y>;", "f < x, y >= g;\n")
	expectPrintedTS(t, "f<x,y>>g<y>;", "f < x, y >> g;\n")
	expectPrintedTS(t, "f<x,y>>>g<y>;", "f < x, y >>> g;\n")
	expectPrintedTS(t, "f<x,y>>=g<y>;", "f < x, y >>= g;\n")
	expectPrintedTS(t, "f<x,y>>>=g<y>;", "f < x, y >>>= g;\n")
	expectPrintedTS(t, "f<x> = g<y>;", "f = g;\n")
	expectParseErrorTS(t, "f<x> > g<y>;", "<stdin>: ERROR: Unexpected \">\"\n")
	expectParseErrorTS(t, "f<x> >> g<y>;", "<stdin>: ERROR: Unexpected \">>\"\n")
	expectParseErrorTS(t, "f<x> >>> g<y>;", "<stdin>: ERROR: Unexpected \">>>\"\n")
	expectParseErrorTS(t, "f<x> >= g<y>;", "<stdin>: ERROR: Unexpected \">=\"\n")
	expectParseErrorTS(t, "f<x> >>= g<y>;", "<stdin>: ERROR: Unexpected \">>=\"\n")
	expectParseErrorTS(t, "f<x> >>>= g<y>;", "<stdin>: ERROR: Unexpected \">>>=\"\n")
	expectPrintedTS(t, "f<x,y> = g<y>;", "f = g;\n")
	expectParseErrorTS(t, "f<x,y> > g<y>;", "<stdin>: ERROR: Unexpected \">\"\n")
	expectParseErrorTS(t, "f<x,y> >> g<y>;", "<stdin>: ERROR: Unexpected \">>\"\n")
	expectParseErrorTS(t, "f<x,y> >>> g<y>;", "<stdin>: ERROR: Unexpected \">>>\"\n")
	expectParseErrorTS(t, "f<x,y> >= g<y>;", "<stdin>: ERROR: Unexpected \">=\"\n")
	expectParseErrorTS(t, "f<x,y> >>= g<y>;", "<stdin>: ERROR: Unexpected \">>=\"\n")
	expectParseErrorTS(t, "f<x,y> >>>= g<y>;", "<stdin>: ERROR: Unexpected \">>>=\"\n")
	expectPrintedTS(t, "[f<x>];", "[f];\n")
	expectPrintedTS(t, "f<x> ? g<y> : h<z>;", "f ? g : h;\n")
	expectPrintedTS(t, "{ f<x> }", "{\n  f;\n}\n")
	expectPrintedTS(t, "f<x> + g<y>;", "f < x > +g;\n")
	expectPrintedTS(t, "f<x> - g<y>;", "f < x > -g;\n")
	expectPrintedTS(t, "f<x> * g<y>;", "f * g;\n")
	expectPrintedTS(t, "f<x> *= g<y>;", "f *= g;\n")
	expectPrintedTS(t, "f<x> == g<y>;", "f == g;\n")
	expectPrintedTS(t, "f<x> ?? g<y>;", "f ?? g;\n")
	expectPrintedTS(t, "f<x> in g<y>;", "f in g;\n")
	expectPrintedTS(t, "f<x> instanceof g<y>;", "f instanceof g;\n")
	expectPrintedTS(t, "f<x> as g<y>;", "f;\n")
	expectPrintedTS(t, "f<x> satisfies g<y>;", "f;\n")
	expectPrintedTS(t, "class A extends B { f() { super.f<x>=y } }", "class A extends B {\n  f() {\n    super.f < x >= y;\n  }\n}\n")
	expectPrintedTS(t, "class A extends B { f() { super.f<x,y>=z } }", "class A extends B {\n  f() {\n    super.f < x, y >= z;\n  }\n}\n")

	expectParseErrorTS(t, "const a8 = f<number><number>;", "<stdin>: ERROR: Unexpected \";\"\n")
	expectParseErrorTS(t, "const b1 = f?.<number>;", "<stdin>: ERROR: Expected \"(\" but found \";\"\n")

	// See: https://github.com/microsoft/TypeScript/issues/48711
	expectPrintedTS(t, "type x = y\n<number>\nz", "z;\n")
	expectPrintedTSX(t, "type x = y\n<number>\nz\n</number>", "/* @__PURE__ */ React.createElement(\"number\", null, \"z\");\n")
	expectPrintedTS(t, "type x = typeof y\n<number>\nz", "z;\n")
	expectPrintedTSX(t, "type x = typeof y\n<number>\nz\n</number>", "/* @__PURE__ */ React.createElement(\"number\", null, \"z\");\n")
	expectPrintedTS(t, "interface Foo { \n (a: number): a \n <T>(): void \n }", "")
	expectPrintedTSX(t, "interface Foo { \n (a: number): a \n <T>(): void \n }", "")
	expectPrintedTS(t, "interface Foo { \n (a: number): typeof a \n <T>(): void \n }", "")
	expectPrintedTSX(t, "interface Foo { \n (a: number): typeof a \n <T>(): void \n }", "")
	expectParseErrorTS(t, "type x = y\n<number>\nz\n</number>", "<stdin>: ERROR: Unterminated regular expression\n")
	expectParseErrorTSX(t, "type x = y\n<number>\nz", "<stdin>: ERROR: Unexpected end of file before a closing \"number\" tag\n<stdin>: NOTE: The opening \"number\" tag is here:\n")
	expectParseErrorTS(t, "type x = typeof y\n<number>\nz\n</number>", "<stdin>: ERROR: Unterminated regular expression\n")
	expectParseErrorTSX(t, "type x = typeof y\n<number>\nz", "<stdin>: ERROR: Unexpected end of file before a closing \"number\" tag\n<stdin>: NOTE: The opening \"number\" tag is here:\n")

	// See: https://github.com/microsoft/TypeScript/issues/48654
	expectPrintedTS(t, "x<true> y", "x < true > y;\n")
	expectPrintedTS(t, "x<true>\ny", "x;\ny;\n")
	expectPrintedTS(t, "x<true>\nif (y) {}", "x;\nif (y) {\n}\n")
	expectPrintedTS(t, "x<true>\nimport 'y'", "x;\nimport \"y\";\n")
	expectPrintedTS(t, "x<true>\nimport('y')", "x;\nimport(\"y\");\n")
	expectPrintedTS(t, "x<true>\nimport.meta", "x;\nimport.meta;\n")
	expectPrintedTS(t, "x<true> import('y')", "x < true > import(\"y\");\n")
	expectPrintedTS(t, "x<true> import.meta", "x < true > import.meta;\n")
	expectPrintedTS(t, "new x<number> y", "new x() < number > y;\n")
	expectPrintedTS(t, "new x<number>\ny", "new x();\ny;\n")
	expectPrintedTS(t, "new x<number>\nif (y) {}", "new x();\nif (y) {\n}\n")
	expectPrintedTS(t, "new x<true>\nimport 'y'", "new x();\nimport \"y\";\n")
	expectPrintedTS(t, "new x<true>\nimport('y')", "new x();\nimport(\"y\");\n")
	expectPrintedTS(t, "new x<true>\nimport.meta", "new x();\nimport.meta;\n")
	expectPrintedTS(t, "new x<true> import('y')", "new x() < true > import(\"y\");\n")
	expectPrintedTS(t, "new x<true> import.meta", "new x() < true > import.meta;\n")

	// See: https://github.com/microsoft/TypeScript/issues/48759
	expectParseErrorTS(t, "x<true>\nimport<T>('y')", "<stdin>: ERROR: Unexpected \"<\"\n")
	expectParseErrorTS(t, "new x<true>\nimport<T>('y')", "<stdin>: ERROR: Unexpected \"<\"\n")

	// See: https://github.com/evanw/esbuild/issues/2201
	expectParseErrorTS(t, "return Array < ;", "<stdin>: ERROR: Unexpected \";\"\n")
	expectParseErrorTS(t, "return Array < > ;", "<stdin>: ERROR: Unexpected \">\"\n")
	expectParseErrorTS(t, "return Array < , > ;", "<stdin>: ERROR: Unexpected \",\"\n")
	expectPrintedTS(t, "return Array < number > ;", "return Array;\n")
	expectPrintedTS(t, "return Array < number > 1;", "return Array < number > 1;\n")
	expectPrintedTS(t, "return Array < number > +1;", "return Array < number > 1;\n")
	expectPrintedTS(t, "return Array < number > (1);", "return Array(1);\n")
	expectPrintedTS(t, "return Array < number >> 1;", "return Array < number >> 1;\n")
	expectPrintedTS(t, "return Array < number >>> 1;", "return Array < number >>> 1;\n")
	expectPrintedTS(t, "return Array < Array < number >> ;", "return Array;\n")
	expectPrintedTS(t, "return Array < Array < number > > ;", "return Array;\n")
	expectParseErrorTS(t, "return Array < Array < number > > 1;", "<stdin>: ERROR: Unexpected \">\"\n")
	expectPrintedTS(t, "return Array < Array < number >> 1;", "return Array < Array < number >> 1;\n")
	expectParseErrorTS(t, "return Array < Array < number > > +1;", "<stdin>: ERROR: Unexpected \">\"\n")
	expectPrintedTS(t, "return Array < Array < number >> +1;", "return Array < Array < number >> 1;\n")
	expectPrintedTS(t, "return Array < Array < number >> (1);", "return Array(1);\n")
	expectPrintedTS(t, "return Array < Array < number > > (1);", "return Array(1);\n")
	expectPrintedTS(t, "return Array < number > in x;", "return Array in x;\n")
	expectPrintedTS(t, "return Array < Array < number >> in x;", "return Array in x;\n")
	expectPrintedTS(t, "return Array < Array < number > > in x;", "return Array in x;\n")
	expectPrintedTS(t, "for (var x = Array < number > in y) ;", "x = Array;\nfor (var x in y)\n  ;\n")
	expectPrintedTS(t, "for (var x = Array < Array < number >> in y) ;", "x = Array;\nfor (var x in y)\n  ;\n")
	expectPrintedTS(t, "for (var x = Array < Array < number > > in y) ;", "x = Array;\nfor (var x in y)\n  ;\n")

	// See: https://github.com/microsoft/TypeScript/pull/49353
	expectPrintedTS(t, "F<{}> 0", "F < {} > 0;\n")
	expectPrintedTS(t, "F<{}> class F<T> {}", "F < {} > class F {\n};\n")
	expectPrintedTS(t, "f<{}> function f<T>() {}", "f < {} > function f() {\n};\n")
	expectPrintedTS(t, "F<{}>\n0", "F;\n0;\n")
	expectPrintedTS(t, "F<{}>\nclass F<T> {}", "F;\nclass F {\n}\n")
	expectPrintedTS(t, "f<{}>\nfunction f<T>() {}", "f;\nfunction f() {\n}\n")
}

func TestTSExponentiation(t *testing.T) {
	// More info: https://github.com/microsoft/TypeScript/issues/41755
	expectParseErrorTS(t, "await x! ** 2", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectPrintedTS(t, "await x as any ** 2", "(await x) ** 2;\n")
}

func TestTSImport(t *testing.T) {
	expectPrintedTS(t, "import {x} from 'foo'", "")
	expectPrintedTS(t, "import {x} from 'foo'; log(x)", "import { x } from \"foo\";\nlog(x);\n")
	expectPrintedTS(t, "import {x, y as z} from 'foo'; log(z)", "import { y as z } from \"foo\";\nlog(z);\n")

	expectPrintedTS(t, "import x from 'foo'", "")
	expectPrintedTS(t, "import x from 'foo'; log(x)", "import x from \"foo\";\nlog(x);\n")

	expectPrintedTS(t, "import * as ns from 'foo'", "")
	expectPrintedTS(t, "import * as ns from 'foo'; log(ns)", "import * as ns from \"foo\";\nlog(ns);\n")

	// Dead control flow must not affect usage tracking
	expectPrintedTS(t, "import {x} from 'foo'; if (false) log(x)", "import \"foo\";\nif (false)\n  log(x);\n")
	expectPrintedTS(t, "import x from 'foo'; if (false) log(x)", "import \"foo\";\nif (false)\n  log(x);\n")
	expectPrintedTS(t, "import * as ns from 'foo'; if (false) log(ns)", "import \"foo\";\nif (false)\n  log(ns);\n")
}

// This is TypeScript-specific export syntax
func TestTSExportEquals(t *testing.T) {
	// This use of the "export" keyword should not trigger strict mode because
	// this syntax works in CommonJS modules, not in ECMAScript modules
	expectPrintedTS(t, "export = []", "module.exports = [];\n")
	expectPrintedTS(t, "export = []; with ({}) ;", "with ({})\n  ;\nmodule.exports = [];\n")
}

// This is TypeScript-specific import syntax
func TestTSImportEquals(t *testing.T) {
	// This use of the "export" keyword should not trigger strict mode because
	// this syntax works in CommonJS modules, not in ECMAScript modules
	expectPrintedTS(t, "import x = require('y')", "const x = require(\"y\");\n")
	expectPrintedTS(t, "import x = require('y'); with ({}) ;", "const x = require(\"y\");\nwith ({})\n  ;\n")

	expectPrintedTS(t, "import x = require('foo'); x()", "const x = require(\"foo\");\nx();\n")
	expectPrintedTS(t, "import x = require('foo')\nx()", "const x = require(\"foo\");\nx();\n")
	expectPrintedTS(t, "import x = require\nx()", "const x = require;\nx();\n")
	expectPrintedTS(t, "import x = foo.bar; x()", "const x = foo.bar;\nx();\n")
	expectPrintedTS(t, "import x = foo.bar\nx()", "const x = foo.bar;\nx();\n")
	expectParseErrorTS(t, "import x = foo()", "<stdin>: ERROR: Expected \";\" but found \"(\"\n")
	expectParseErrorTS(t, "import x = foo<T>.bar", "<stdin>: ERROR: Expected \";\" but found \"<\"\n")
	expectParseErrorTS(t, "{ import x = foo.bar }", "<stdin>: ERROR: Unexpected \"x\"\n")

	expectPrintedTS(t, "export import x = require('foo'); x()", "export const x = require(\"foo\");\nx();\n")
	expectPrintedTS(t, "export import x = require('foo')\nx()", "export const x = require(\"foo\");\nx();\n")
	expectPrintedTS(t, "export import x = foo.bar; x()", "export const x = foo.bar;\nx();\n")
	expectPrintedTS(t, "export import x = foo.bar\nx()", "export const x = foo.bar;\nx();\n")

	expectParseError(t, "export import foo = bar", "<stdin>: ERROR: Unexpected \"import\"\n")
	expectParseErrorTS(t, "export import {foo} from 'bar'", "<stdin>: ERROR: Expected identifier but found \"{\"\n")
	expectParseErrorTS(t, "export import foo from 'bar'", "<stdin>: ERROR: Expected \"=\" but found \"from\"\n")
	expectParseErrorTS(t, "export import foo = bar; var x; export {x as foo}",
		`<stdin>: ERROR: Multiple exports with the same name "foo"
<stdin>: NOTE: The name "foo" was originally exported here:
`)
	expectParseErrorTS(t, "{ export import foo = bar }", "<stdin>: ERROR: Unexpected \"export\"\n")

	errorText := `<stdin>: WARNING: This assignment will throw because "x" is a constant
<stdin>: NOTE: The symbol "x" was declared a constant here:
`
	expectParseErrorTS(t, "import x = require('y'); x = require('z')", errorText)
	expectParseErrorTS(t, "import x = y.z; x = z.y", errorText)
}

func TestTSImportEqualsInNamespace(t *testing.T) {
	expectPrintedTS(t, "namespace ns { import foo = bar }", "")
	expectPrintedTS(t, "namespace ns { import foo = bar; type x = foo.x }", "")
	expectPrintedTS(t, "namespace ns { import foo = bar.x; foo }", `var ns;
((ns) => {
  const foo = bar.x;
  foo;
})(ns || (ns = {}));
`)
	expectPrintedTS(t, "namespace ns { export import foo = bar }", `var ns;
((ns) => {
  ns.foo = bar;
})(ns || (ns = {}));
`)
	expectPrintedTS(t, "namespace ns { export import foo = bar.x; foo }", `var ns;
((ns) => {
  ns.foo = bar.x;
  ns.foo;
})(ns || (ns = {}));
`)
	expectParseErrorTS(t, "namespace ns { import {foo} from 'bar' }", "<stdin>: ERROR: Expected identifier but found \"{\"\n")
	expectParseErrorTS(t, "namespace ns { import foo from 'bar' }", "<stdin>: ERROR: Expected \"=\" but found \"from\"\n")
	expectParseErrorTS(t, "namespace ns { export import {foo} from 'bar' }", "<stdin>: ERROR: Expected identifier but found \"{\"\n")
	expectParseErrorTS(t, "namespace ns { export import foo from 'bar' }", "<stdin>: ERROR: Expected \"=\" but found \"from\"\n")
	expectParseErrorTS(t, "namespace ns { { import foo = bar } }", "<stdin>: ERROR: Unexpected \"foo\"\n")
	expectParseErrorTS(t, "namespace ns { { export import foo = bar } }", "<stdin>: ERROR: Unexpected \"export\"\n")
}

func TestTSTypeOnlyImport(t *testing.T) {
	expectPrintedTS(t, "import type foo from 'bar'; x", "x;\n")
	expectPrintedTS(t, "import type foo from 'bar'\nx", "x;\n")
	expectPrintedTS(t, "import type * as foo from 'bar'; x", "x;\n")
	expectPrintedTS(t, "import type * as foo from 'bar'\nx", "x;\n")
	expectPrintedTS(t, "import type {foo, bar as baz} from 'bar'; x", "x;\n")
	expectPrintedTS(t, "import type {'foo' as bar} from 'bar'\nx", "x;\n")
	expectPrintedTS(t, "import type foo = require('bar'); x", "x;\n")
	expectPrintedTS(t, "import type foo = bar.baz; x", "x;\n")

	expectPrintedTS(t, "import type = bar; type", "const type = bar;\ntype;\n")
	expectPrintedTS(t, "import type = foo.bar; type", "const type = foo.bar;\ntype;\n")
	expectPrintedTS(t, "import type = require('type'); type", "const type = require(\"type\");\ntype;\n")
	expectPrintedTS(t, "import type from 'bar'; type", "import type from \"bar\";\ntype;\n")

	expectPrintedTS(t, "import { type } from 'mod'; type", "import { type } from \"mod\";\ntype;\n")
	expectPrintedTS(t, "import { x, type foo } from 'mod'; x", "import { x } from \"mod\";\nx;\n")
	expectPrintedTS(t, "import { x, type as } from 'mod'; x", "import { x } from \"mod\";\nx;\n")
	expectPrintedTS(t, "import { x, type foo as bar } from 'mod'; x", "import { x } from \"mod\";\nx;\n")
	expectPrintedTS(t, "import { x, type foo as as } from 'mod'; x", "import { x } from \"mod\";\nx;\n")
	expectPrintedTS(t, "import { type as as } from 'mod'; as", "import { type as as } from \"mod\";\nas;\n")
	expectPrintedTS(t, "import { type as foo } from 'mod'; foo", "import { type as foo } from \"mod\";\nfoo;\n")
	expectPrintedTS(t, "import { type as type } from 'mod'; type", "import { type } from \"mod\";\ntype;\n")
	expectPrintedTS(t, "import { x, type as as foo } from 'mod'; x", "import { x } from \"mod\";\nx;\n")
	expectPrintedTS(t, "import { x, type as as as } from 'mod'; x", "import { x } from \"mod\";\nx;\n")
	expectPrintedTS(t, "import { x, type type as as } from 'mod'; x", "import { x } from \"mod\";\nx;\n")
	expectPrintedTS(t, "import { x, \\u0074ype y } from 'mod'; x, y", "import { x } from \"mod\";\nx, y;\n")
	expectPrintedTS(t, "import { x, type if as y } from 'mod'; x, y", "import { x } from \"mod\";\nx, y;\n")

	expectPrintedTS(t, "import a = b; import c = a.c", "")
	expectPrintedTS(t, "import c = a.c; import a = b", "")
	expectPrintedTS(t, "import a = b; import c = a.c; c()", "const a = b;\nconst c = a.c;\nc();\n")
	expectPrintedTS(t, "import c = a.c; import a = b; c()", "const c = a.c;\nconst a = b;\nc();\n")

	expectParseErrorTS(t, "import type", "<stdin>: ERROR: Expected \"from\" but found end of file\n")
	expectParseErrorTS(t, "import type * foo", "<stdin>: ERROR: Expected \"as\" but found \"foo\"\n")
	expectParseErrorTS(t, "import type * as 'bar'", "<stdin>: ERROR: Expected identifier but found \"'bar'\"\n")
	expectParseErrorTS(t, "import type { 'bar' }", "<stdin>: ERROR: Expected \"as\" but found \"}\"\n")

	expectParseErrorTS(t, "import type foo, * as foo from 'bar'", "<stdin>: ERROR: Expected \"from\" but found \",\"\n")
	expectParseErrorTS(t, "import type foo, {foo} from 'bar'", "<stdin>: ERROR: Expected \"from\" but found \",\"\n")
	expectParseErrorTS(t, "import type * as foo = require('bar')", "<stdin>: ERROR: Expected \"from\" but found \"=\"\n")
	expectParseErrorTS(t, "import type {foo} = require('bar')", "<stdin>: ERROR: Expected \"from\" but found \"=\"\n")

	expectParseErrorTS(t, "import { type as export } from 'mod'", "<stdin>: ERROR: Expected \"}\" but found \"export\"\n")
	expectParseErrorTS(t, "import { type as as export } from 'mod'", "<stdin>: ERROR: Expected \"}\" but found \"export\"\n")
	expectParseErrorTS(t, "import { type import } from 'mod'", "<stdin>: ERROR: Expected \"as\" but found \"}\"\n")
	expectParseErrorTS(t, "import { type foo bar } from 'mod'", "<stdin>: ERROR: Expected \"}\" but found \"bar\"\n")
	expectParseErrorTS(t, "import { type foo as } from 'mod'", "<stdin>: ERROR: Expected identifier but found \"}\"\n")
	expectParseErrorTS(t, "import { type foo as bar baz } from 'mod'", "<stdin>: ERROR: Expected \"}\" but found \"baz\"\n")
	expectParseErrorTS(t, "import { type as as as as } from 'mod'", "<stdin>: ERROR: Expected \"}\" but found \"as\"\n")
	expectParseErrorTS(t, "import { type \\u0061s x } from 'mod'", "<stdin>: ERROR: Expected \"}\" but found \"x\"\n")
	expectParseErrorTS(t, "import { type x \\u0061s y } from 'mod'", "<stdin>: ERROR: Expected \"}\" but found \"\\\\u0061s\"\n")
	expectParseErrorTS(t, "import { type x as if } from 'mod'", "<stdin>: ERROR: Expected identifier but found \"if\"\n")
	expectParseErrorTS(t, "import { type as if } from 'mod'", "<stdin>: ERROR: Expected \"}\" but found \"if\"\n")

	// Forbidden names
	expectParseErrorTS(t, "import { type as eval } from 'mod'", "<stdin>: ERROR: Cannot use \"eval\" as an identifier here:\n")
	expectParseErrorTS(t, "import { type as arguments } from 'mod'", "<stdin>: ERROR: Cannot use \"arguments\" as an identifier here:\n")

	// Arbitrary module namespace identifier names
	expectPrintedTS(t, "import { x, type 'y' as z } from 'mod'; x, z", "import { x } from \"mod\";\nx, z;\n")
	expectParseErrorTS(t, "import { x, type 'y' } from 'mod'", "<stdin>: ERROR: Expected \"as\" but found \"}\"\n")
	expectParseErrorTS(t, "import { x, type 'y' as } from 'mod'", "<stdin>: ERROR: Expected identifier but found \"}\"\n")
	expectParseErrorTS(t, "import { x, type 'y' as 'z' } from 'mod'", "<stdin>: ERROR: Expected identifier but found \"'z'\"\n")
	expectParseErrorTS(t, "import { x, type as 'y' } from 'mod'", "<stdin>: ERROR: Expected \"}\" but found \"'y'\"\n")
	expectParseErrorTS(t, "import { x, type y as 'z' } from 'mod'", "<stdin>: ERROR: Expected identifier but found \"'z'\"\n")
}

func TestTSTypeOnlyExport(t *testing.T) {
	expectPrintedTS(t, "export type {foo, bar as baz} from 'bar'", "")
	expectPrintedTS(t, "export type {foo, bar as baz}", "")
	expectPrintedTS(t, "export type {foo} from 'bar'; x", "x;\n")
	expectPrintedTS(t, "export type {foo} from 'bar'\nx", "x;\n")
	expectPrintedTS(t, "export type {default} from 'bar'", "")
	expectParseErrorTS(t, "export type {default}", "<stdin>: ERROR: Expected identifier but found \"default\"\n")

	expectPrintedTS(t, "export { type } from 'mod'; type", "export { type } from \"mod\";\ntype;\n")
	expectPrintedTS(t, "export { type, as } from 'mod'", "export { type, as } from \"mod\";\n")
	expectPrintedTS(t, "export { x, type foo } from 'mod'; x", "export { x } from \"mod\";\nx;\n")
	expectPrintedTS(t, "export { x, type as } from 'mod'; x", "export { x } from \"mod\";\nx;\n")
	expectPrintedTS(t, "export { x, type foo as bar } from 'mod'; x", "export { x } from \"mod\";\nx;\n")
	expectPrintedTS(t, "export { x, type foo as as } from 'mod'; x", "export { x } from \"mod\";\nx;\n")
	expectPrintedTS(t, "export { type as as } from 'mod'; as", "export { type as as } from \"mod\";\nas;\n")
	expectPrintedTS(t, "export { type as foo } from 'mod'; foo", "export { type as foo } from \"mod\";\nfoo;\n")
	expectPrintedTS(t, "export { type as type } from 'mod'; type", "export { type } from \"mod\";\ntype;\n")
	expectPrintedTS(t, "export { x, type as as foo } from 'mod'; x", "export { x } from \"mod\";\nx;\n")
	expectPrintedTS(t, "export { x, type as as as } from 'mod'; x", "export { x } from \"mod\";\nx;\n")
	expectPrintedTS(t, "export { x, type type as as } from 'mod'; x", "export { x } from \"mod\";\nx;\n")
	expectPrintedTS(t, "export { x, \\u0074ype y }; let x, y", "export { x };\nlet x, y;\n")
	expectPrintedTS(t, "export { x, \\u0074ype y } from 'mod'", "export { x } from \"mod\";\n")
	expectPrintedTS(t, "export { x, type if } from 'mod'", "export { x } from \"mod\";\n")
	expectPrintedTS(t, "export { x, type y as if }; let x", "export { x };\nlet x;\n")

	expectParseErrorTS(t, "export { type foo bar } from 'mod'", "<stdin>: ERROR: Expected \"}\" but found \"bar\"\n")
	expectParseErrorTS(t, "export { type foo as } from 'mod'", "<stdin>: ERROR: Expected identifier but found \"}\"\n")
	expectParseErrorTS(t, "export { type foo as bar baz } from 'mod'", "<stdin>: ERROR: Expected \"}\" but found \"baz\"\n")
	expectParseErrorTS(t, "export { type as as as as } from 'mod'", "<stdin>: ERROR: Expected \"}\" but found \"as\"\n")
	expectParseErrorTS(t, "export { type \\u0061s x } from 'mod'", "<stdin>: ERROR: Expected \"}\" but found \"x\"\n")
	expectParseErrorTS(t, "export { type x \\u0061s y } from 'mod'", "<stdin>: ERROR: Expected \"}\" but found \"\\\\u0061s\"\n")
	expectParseErrorTS(t, "export { x, type if }", "<stdin>: ERROR: Expected identifier but found \"if\"\n")

	// Arbitrary module namespace identifier names
	expectPrintedTS(t, "export { type as \"\" } from 'mod'", "export { type as \"\" } from \"mod\";\n")
	expectPrintedTS(t, "export { x, type as as \"\" } from 'mod'", "export { x } from \"mod\";\n")
	expectPrintedTS(t, "export { x, type x as \"\" } from 'mod'", "export { x } from \"mod\";\n")
	expectPrintedTS(t, "export { x, type \"\" as x } from 'mod'", "export { x } from \"mod\";\n")
	expectPrintedTS(t, "export { x, type \"\" as \" \" } from 'mod'", "export { x } from \"mod\";\n")
	expectPrintedTS(t, "export { x, type \"\" } from 'mod'", "export { x } from \"mod\";\n")
	expectParseErrorTS(t, "export { type \"\" }", "<stdin>: ERROR: Expected identifier but found \"\\\"\\\"\"\n")
	expectParseErrorTS(t, "export { type \"\" as x }", "<stdin>: ERROR: Expected identifier but found \"\\\"\\\"\"\n")
	expectParseErrorTS(t, "export { type \"\" as \" \" }", "<stdin>: ERROR: Expected identifier but found \"\\\"\\\"\"\n")

	// Named exports should be removed if they don't refer to a local symbol
	expectPrintedTS(t, "const Foo = {}; export {Foo}", "const Foo = {};\nexport { Foo };\n")
	expectPrintedTS(t, "type Foo = {}; export {Foo}", "export {};\n")
	expectPrintedTS(t, "const Foo = {}; export {Foo as Bar}", "const Foo = {};\nexport { Foo as Bar };\n")
	expectPrintedTS(t, "type Foo = {}; export {Foo as Bar}", "export {};\n")
	expectPrintedTS(t, "import Foo from 'foo'; export {Foo}", "import Foo from \"foo\";\nexport { Foo };\n")
	expectPrintedTS(t, "import {Foo} from 'foo'; export {Foo}", "import { Foo } from \"foo\";\nexport { Foo };\n")
	expectPrintedTS(t, "import * as Foo from 'foo'; export {Foo}", "import * as Foo from \"foo\";\nexport { Foo };\n")
	expectPrintedTS(t, "{ var Foo; } export {Foo}", "{\n  var Foo;\n}\nexport { Foo };\n")
	expectPrintedTS(t, "{ let Foo; } export {Foo}", "{\n  let Foo;\n}\nexport {};\n")
	expectPrintedTS(t, "export {Foo}", "export {};\n")
	expectParseError(t, "export {Foo}", "<stdin>: ERROR: \"Foo\" is not declared in this file\n")

	// This is a syntax error in TypeScript, but we parse it anyway because
	// people blame esbuild when it doesn't parse. It's silently discarded
	// because we always discard all type annotations (even invalid ones).
	expectPrintedTS(t, "export type * from 'foo'\nbar", "bar;\n")
	expectPrintedTS(t, "export type * as foo from 'bar'; foo", "foo;\n")
	expectPrintedTS(t, "export type * as 'f o' from 'bar'; foo", "foo;\n")
}

func TestTSOptionalChain(t *testing.T) {
	expectParseError(t, "a?.<T>()", "<stdin>: ERROR: Expected identifier but found \"<\"\n")
	expectParseError(t, "a?.<<T>() => T>()", "<stdin>: ERROR: Expected identifier but found \"<<\"\n")
	expectPrintedTS(t, "a?.<T>()", "a?.();\n")
	expectPrintedTS(t, "a?.<<T>() => T>()", "a?.();\n")
	expectParseErrorTS(t, "a?.<T>b", "<stdin>: ERROR: Expected \"(\" but found \"b\"\n")
	expectParseErrorTS(t, "a?.<T>[b]", "<stdin>: ERROR: Expected \"(\" but found \"[\"\n")
	expectParseErrorTS(t, "a?.<<T>() => T>b", "<stdin>: ERROR: Expected \"(\" but found \"b\"\n")
	expectParseErrorTS(t, "a?.<<T>() => T>[b]", "<stdin>: ERROR: Expected \"(\" but found \"[\"\n")

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
	expectPrintedTS(t, "a?.b<+T>(c)", "a?.b < +T > c;\n")
	expectPrintedTS(t, "a?.b<<T>() => T>(c)", "a?.b(c);\n")
}

func TestTSJSX(t *testing.T) {
	expectParseErrorTSX(t, "<div>></div>",
		"<stdin>: ERROR: The character \">\" is not valid inside a JSX element\n"+
			"NOTE: Did you mean to escape it as \"{'>'}\" instead?\n")
	expectParseErrorTSX(t, "<div>{1}}</div>",
		"<stdin>: ERROR: The character \"}\" is not valid inside a JSX element\n"+
			"NOTE: Did you mean to escape it as \"{'}'}\" instead?\n")

	expectPrintedTS(t, "const x = <number>1", "const x = 1;\n")
	expectPrintedTSX(t, "const x = <number>1</number>", "const x = /* @__PURE__ */ React.createElement(\"number\", null, \"1\");\n")
	expectParseErrorTSX(t, "const x = <number>1", "<stdin>: ERROR: Unexpected end of file before a closing \"number\" tag\n<stdin>: NOTE: The opening \"number\" tag is here:\n")

	expectPrintedTSX(t, "<x>a{}c</x>", "/* @__PURE__ */ React.createElement(\"x\", null, \"a\", \"c\");\n")
	expectPrintedTSX(t, "<x>a{/* comment */}c</x>", "/* @__PURE__ */ React.createElement(\"x\", null, \"a\", \"c\");\n")
	expectPrintedTSX(t, "<x>a{b}c</x>", "/* @__PURE__ */ React.createElement(\"x\", null, \"a\", b, \"c\");\n")
	expectPrintedTSX(t, "<x>a{...b}c</x>", "/* @__PURE__ */ React.createElement(\"x\", null, \"a\", ...b, \"c\");\n")

	expectPrintedTSX(t, "const x = <Foo<T>></Foo>", "const x = /* @__PURE__ */ React.createElement(Foo, null);\n")
	expectPrintedTSX(t, "const x = <Foo<T> data-foo></Foo>", "const x = /* @__PURE__ */ React.createElement(Foo, { \"data-foo\": true });\n")
	expectParseErrorTSX(t, "const x = <Foo<T>=>", "<stdin>: ERROR: Expected \">\" but found \"=>\"\n")

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
	expectPrintedTS(t, "const x = async\n<T>(y)", "const x = async(y);\n")
	expectPrintedTS(t, "const x = async <T>(y, z)", "const x = async(y, z);\n")
	expectPrintedTS(t, "const x = async <T>(y: T) => {}", "const x = async (y) => {\n};\n")
	expectPrintedTS(t, "const x = async <T>(y, z) => {}", "const x = async (y, z) => {\n};\n")
	expectPrintedTS(t, "const x = async <T = X>(y: T) => {}", "const x = async (y) => {\n};\n")
	expectPrintedTS(t, "const x = async <T = X>(y, z) => {}", "const x = async (y, z) => {\n};\n")
	expectPrintedTS(t, "const x = async <T extends X>(y: T) => {}", "const x = async (y) => {\n};\n")
	expectPrintedTS(t, "const x = async <T extends X>(y, z) => {}", "const x = async (y, z) => {\n};\n")
	expectPrintedTS(t, "const x = async <T extends X = Y>(y: T) => {}", "const x = async (y) => {\n};\n")
	expectPrintedTS(t, "const x = async <T extends X = Y>(y, z) => {}", "const x = async (y, z) => {\n};\n")
	expectPrintedTS(t, "const x = (async <T, X> y)", "const x = (async < T, X > y);\n")
	expectPrintedTS(t, "const x = (async <T, X>(y))", "const x = async(y);\n")
	expectParseErrorTS(t, "const x = async <T,>(y)", "<stdin>: ERROR: Expected \"=>\" but found end of file\n")
	expectParseErrorTS(t, "const x = async <T>(y: T)", "<stdin>: ERROR: Unexpected \":\"\n")
	expectParseErrorTS(t, "const x = async\n<T>() => {}", "<stdin>: ERROR: Expected \";\" but found \"=>\"\n")
	expectParseErrorTS(t, "const x = async\n<T>(x) => {}", "<stdin>: ERROR: Expected \";\" but found \"=>\"\n")

	expectPrintedTS(t, "const x = <{}>() => {}", "const x = () => {\n};\n")
	expectPrintedTS(t, "const x = <{}>(y)", "const x = y;\n")
	expectPrintedTS(t, "const x = <{}>(y, z)", "const x = (y, z);\n")
	expectPrintedTS(t, "const x = <{}>(y, z) => {}", "const x = (y, z) => {\n};\n")

	expectPrintedTS(t, "const x = <[]>() => {}", "const x = () => {\n};\n")
	expectPrintedTS(t, "const x = <[]>(y)", "const x = y;\n")
	expectPrintedTS(t, "const x = <[]>(y, z)", "const x = (y, z);\n")
	expectPrintedTS(t, "const x = <[]>(y, z) => {}", "const x = (y, z) => {\n};\n")

	invalid := "<stdin>: ERROR: The character \">\" is not valid inside a JSX element\nNOTE: Did you mean to escape it as \"{'>'}\" instead?\n"
	invalidWithHint := "<stdin>: ERROR: The character \">\" is not valid inside a JSX element\n<stdin>: NOTE: TypeScript's TSX syntax interprets " +
		"arrow functions with a single generic type parameter as an opening JSX element. If you want it to be interpreted as an arrow function instead, " +
		"you need to add a trailing comma after the type parameter to disambiguate:\n"
	expectPrintedTSX(t, "<T extends/>", "/* @__PURE__ */ React.createElement(T, { extends: true });\n")
	expectPrintedTSX(t, "<T extends>(y) = {}</T>", "/* @__PURE__ */ React.createElement(T, { extends: true }, \"(y) = \");\n")
	expectParseErrorTSX(t, "<T extends X/>", "<stdin>: ERROR: Expected \">\" but found \"/\"\n")
	expectParseErrorTSX(t, "<T extends X>(y) = {}</T>", "<stdin>: ERROR: Expected \"=>\" but found \"=\"\n")
	expectParseErrorTSX(t, "(<T>(y) => {}</T>)", invalidWithHint)
	expectParseErrorTSX(t, "(<T>(x: X<Y>) => {}</Y></T>)", invalidWithHint)
	expectParseErrorTSX(t, "(<T extends>(y) => {}</T>)", invalid)
	expectParseErrorTSX(t, "(<T extends={false}>(y) => {}</T>)", invalid)
	expectPrintedTSX(t, "(<T = X>(y) => {})", "(y) => {\n};\n")
	expectPrintedTSX(t, "(<T extends X>(y) => {})", "(y) => {\n};\n")
	expectPrintedTSX(t, "(<T extends X = Y>(y) => {})", "(y) => {\n};\n")
	expectPrintedTSX(t, "(<T,>() => {})", "() => {\n};\n")
	expectPrintedTSX(t, "(<T, X>(y) => {})", "(y) => {\n};\n")
	expectPrintedTSX(t, "(<T, X>(y): (() => {}) => {})", "(y) => {\n};\n")
	expectParseErrorTSX(t, "(<T>() => {})", invalidWithHint+"<stdin>: ERROR: Unexpected end of file before a closing \"T\" tag\n<stdin>: NOTE: The opening \"T\" tag is here:\n")
	expectParseErrorTSX(t, "(<T>(x: X<Y>) => {})", invalidWithHint+"<stdin>: ERROR: Unexpected end of file before a closing \"Y\" tag\n<stdin>: NOTE: The opening \"Y\" tag is here:\n")
	expectParseErrorTSX(t, "(<T>(x: X<Y>) => {})</Y>", invalidWithHint+"<stdin>: ERROR: Unexpected end of file before a closing \"T\" tag\n<stdin>: NOTE: The opening \"T\" tag is here:\n")
	expectParseErrorTSX(t, "(<[]>(y))", "<stdin>: ERROR: Expected identifier but found \"[\"\n")
	expectParseErrorTSX(t, "(<T[]>(y))", "<stdin>: ERROR: Expected \">\" but found \"[\"\n")
	expectParseErrorTSX(t, "(<T = X>(y))", "<stdin>: ERROR: Expected \"=>\" but found \")\"\n")
	expectParseErrorTSX(t, "(<T, X>(y))", "<stdin>: ERROR: Expected \"=>\" but found \")\"\n")
	expectParseErrorTSX(t, "(<T, X>y => {})", "<stdin>: ERROR: Expected \"(\" but found \"y\"\n")

	// TypeScript doesn't currently parse these even though it seems unambiguous
	expectPrintedTSX(t, "async <T,>() => {}", "async () => {\n};\n")
	expectPrintedTSX(t, "async <T extends X>() => {}", "async () => {\n};\n")
	expectPrintedTSX(t, "async <T>()", "async();\n")
	expectParseErrorTSX(t, "async <T>() => {}", "<stdin>: ERROR: Expected \";\" but found \"=>\"\n")
	expectParseErrorTSX(t, "async <T extends>() => {}", "<stdin>: ERROR: Expected \";\" but found \"extends\"\n")
}

func TestTSNoAmbiguousLessThan(t *testing.T) {
	expectPrintedTSNoAmbiguousLessThan(t, "(<T,>() => {})", "() => {\n};\n")
	expectPrintedTSNoAmbiguousLessThan(t, "(<T, X>() => {})", "() => {\n};\n")
	expectPrintedTSNoAmbiguousLessThan(t, "(<T extends X>() => {})", "() => {\n};\n")
	expectParseErrorTSNoAmbiguousLessThan(t, "(<T>x)",
		"<stdin>: ERROR: This syntax is not allowed in files with the \".mts\" or \".cts\" extension\n")
	expectParseErrorTSNoAmbiguousLessThan(t, "(<T>() => {})",
		"<stdin>: ERROR: This syntax is not allowed in files with the \".mts\" or \".cts\" extension\n")
	expectParseErrorTSNoAmbiguousLessThan(t, "(<T>(x) => {})",
		"<stdin>: ERROR: This syntax is not allowed in files with the \".mts\" or \".cts\" extension\n")
	expectParseErrorTSNoAmbiguousLessThan(t, "<x>y</x>",
		"<stdin>: ERROR: This syntax is not allowed in files with the \".mts\" or \".cts\" extension\n"+
			"<stdin>: ERROR: Unterminated regular expression\n")
	expectParseErrorTSNoAmbiguousLessThan(t, "<x extends></x>",
		"<stdin>: ERROR: This syntax is not allowed in files with the \".mts\" or \".cts\" extension\n"+
			"<stdin>: ERROR: Unexpected \">\"\n")
	expectParseErrorTSNoAmbiguousLessThan(t, "<x extends={y}></x>",
		"<stdin>: ERROR: This syntax is not allowed in files with the \".mts\" or \".cts\" extension\n"+
			"<stdin>: ERROR: Unexpected \"=\"\n")
}

func TestTSClassSideEffectOrder(t *testing.T) {
	// The order of computed property side effects must not change
	expectPrintedAssignSemanticsTS(t, `class Foo {
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
  static {
    h();
  }
  [a()]() {
  }
  [(b(), _a = c(), d())]() {
  }
  static {
    this[_b] = 1;
  }
  static [(e(), _b = f(), g())]() {
  }
}
`)
	expectPrintedAssignSemanticsTS(t, `class Foo {
	static [x()] = 1;
}
`, `var _a;
class Foo {
  static {
    _a = x();
  }
  static {
    this[_a] = 1;
  }
}
`)
	expectPrintedAssignSemanticsTargetTS(t, 2021, `class Foo {
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

func TestTSMangleStringEnumLength(t *testing.T) {
	expectPrintedTS(t, "enum x { y = '' } z = x.y.length",
		"var x = /* @__PURE__ */ ((x) => {\n  x[\"y\"] = \"\";\n  return x;\n})(x || {});\nz = \"\" /* y */.length;\n")

	expectPrintedMangleTS(t, "enum x { y = '' } x.y.length++",
		"var x = /* @__PURE__ */ ((x) => (x.y = \"\", x))(x || {});\n\"\" /* y */.length++;\n")

	expectPrintedMangleTS(t, "enum x { y = '' } x.y.length = z",
		"var x = /* @__PURE__ */ ((x) => (x.y = \"\", x))(x || {});\n\"\" /* y */.length = z;\n")

	expectPrintedMangleTS(t, "enum x { y = '' } z = x.y.length",
		"var x = /* @__PURE__ */ ((x) => (x.y = \"\", x))(x || {});\nz = 0;\n")

	expectPrintedMangleTS(t, "enum x { y = 'abc' } z = x.y.length",
		"var x = /* @__PURE__ */ ((x) => (x.y = \"abc\", x))(x || {});\nz = 3;\n")

	expectPrintedMangleTS(t, "enum x { y = 'ȧḃċ' } z = x.y.length",
		"var x = /* @__PURE__ */ ((x) => (x.y = \"ȧḃċ\", x))(x || {});\nz = 3;\n")

	expectPrintedMangleTS(t, "enum x { y = '👯‍♂️' } z = x.y.length",
		"var x = /* @__PURE__ */ ((x) => (x.y = \"👯‍♂️\", x))(x || {});\nz = 5;\n")
}

func TestTSES5(t *testing.T) {
	// Errors from lowering hypothetical arrow function arguments to ES5 should
	// not leak out when backtracking. This comes up when parentheses are followed
	// by a colon in TypeScript because the colon could deliminate an arrow
	// function return type. See: https://github.com/evanw/esbuild/issues/2375.
	expectPrintedTargetTS(t, 2015, "0 ? ([]) : 0", "0 ? [] : 0;\n")
	expectPrintedTargetTS(t, 2015, "0 ? ({}) : 0", "0 ? {} : 0;\n")
	expectPrintedTargetTS(t, 5, "0 ? ([]) : 0", "0 ? [] : 0;\n")
	expectPrintedTargetTS(t, 5, "0 ? ({}) : 0", "0 ? {} : 0;\n")
	expectPrintedTargetTS(t, 2015, "0 ? ([]): 0 => 0 : 0", "0 ? ([]) => 0 : 0;\n")
	expectPrintedTargetTS(t, 2015, "0 ? ({}): 0 => 0 : 0", "0 ? ({}) => 0 : 0;\n")
	expectParseErrorTargetTS(t, 5, "0 ? ([]): 0 => 0 : 0", "<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTargetTS(t, 5, "0 ? ({}): 0 => 0 : 0", "<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet\n")
}

func TestTSUsing(t *testing.T) {
	expectPrintedTS(t, "using x = y", "using x = y;\n")
	expectPrintedTS(t, "using x: any = y", "using x = y;\n")
	expectPrintedTS(t, "using x: any = y, z: any = _", "using x = y, z = _;\n")
	expectParseErrorTS(t, "export using x: any = y", "<stdin>: ERROR: Unexpected \"using\"\n")
	expectParseErrorTS(t, "namespace ns { export using x: any = y }", "<stdin>: ERROR: Unexpected \"using\"\n")
}
