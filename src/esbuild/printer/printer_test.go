package printer

import (
	"esbuild/logging"
	"esbuild/parser"
	"testing"
)

func assertEqual(t *testing.T, a interface{}, b interface{}) {
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
}

func expectPrintedCommon(t *testing.T, name string, contents string, expected string, options PrintOptions) {
	t.Run(name, func(t *testing.T) {
		log, join := logging.NewDeferLog()
		ast, ok := parser.Parse(log, logging.Source{
			Index:        0,
			AbsolutePath: "<stdin>",
			PrettyPath:   "<stdin>",
			Contents:     contents,
		}, parser.ParseOptions{})
		msgs := join()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
		}
		assertEqual(t, text, "")
		if !ok {
			t.Fatal("Parse error")
		}
		js := Print(ast, options).JS
		assertEqual(t, string(js), expected)
	})
}

func expectPrinted(t *testing.T, contents string, expected string) {
	expectPrintedCommon(t, contents, contents, expected, PrintOptions{})
}

func expectPrintedMinify(t *testing.T, contents string, expected string) {
	expectPrintedCommon(t, contents+" [minified]", contents, expected, PrintOptions{
		RemoveWhitespace: true,
	})
}

func TestHashbang(t *testing.T) {
	expectPrinted(t, "#!/usr/bin/env node\nlet x", "#!/usr/bin/env node\nlet x;\n")
}

func TestNumber(t *testing.T) {
	expectPrinted(t, "0.5", "0.5;\n")
	expectPrinted(t, "-0.5", "-0.5;\n")
	expectPrinted(t, "1e100", "1e+100;\n")
	expectPrinted(t, "1e-100", "1e-100;\n")
	expectPrintedMinify(t, "0.5", ".5;")
	expectPrintedMinify(t, "-0.5", "-.5;")
	expectPrintedMinify(t, "1e100", "1e100;")
	expectPrintedMinify(t, "1e-100", "1e-100;")

	// Check numbers around the ends of various integer ranges. These were
	// crashing in the WebAssembly build due to a bug in the Go runtime.

	// int32
	expectPrinted(t, "0x7fff_ffff", "2147483647;\n")
	expectPrinted(t, "0x8000_0000", "2147483648;\n")
	expectPrinted(t, "0x8000_0001", "2147483649;\n")
	expectPrinted(t, "-0x7fff_ffff", "-2147483647;\n")
	expectPrinted(t, "-0x8000_0000", "-2147483648;\n")
	expectPrinted(t, "-0x8000_0001", "-2147483649;\n")

	// uint32
	expectPrinted(t, "0xffff_ffff", "4294967295;\n")
	expectPrinted(t, "0x1_0000_0000", "4294967296;\n")
	expectPrinted(t, "0x1_0000_0001", "4294967297;\n")
	expectPrinted(t, "-0xffff_ffff", "-4294967295;\n")
	expectPrinted(t, "-0x1_0000_0000", "-4294967296;\n")
	expectPrinted(t, "-0x1_0000_0001", "-4294967297;\n")

	// int64
	expectPrinted(t, "0x7fff_ffff_ffff_fdff", "9223372036854774784;\n")
	expectPrinted(t, "0x8000_0000_0000_0000", "9.223372036854776e+18;\n")
	expectPrinted(t, "0x8000_0000_0000_3000", "9.223372036854788e+18;\n")
	expectPrinted(t, "-0x7fff_ffff_ffff_fdff", "-9223372036854774784;\n")
	expectPrinted(t, "-0x8000_0000_0000_0000", "-9.223372036854776e+18;\n")
	expectPrinted(t, "-0x8000_0000_0000_3000", "-9.223372036854788e+18;\n")

	// uint64
	expectPrinted(t, "0xffff_ffff_ffff_fbff", "1.844674407370955e+19;\n")
	expectPrinted(t, "0x1_0000_0000_0000_0000", "1.8446744073709552e+19;\n")
	expectPrinted(t, "0x1_0000_0000_0000_1000", "1.8446744073709556e+19;\n")
	expectPrinted(t, "-0xffff_ffff_ffff_fbff", "-1.844674407370955e+19;\n")
	expectPrinted(t, "-0x1_0000_0000_0000_0000", "-1.8446744073709552e+19;\n")
	expectPrinted(t, "-0x1_0000_0000_0000_1000", "-1.8446744073709556e+19;\n")
}

func TestArray(t *testing.T) {
	expectPrinted(t, "[]", "[];\n")
	expectPrinted(t, "[,]", "[,];\n")
	expectPrinted(t, "[,,]", "[, ,];\n")
}

func TestNew(t *testing.T) {
	expectPrinted(t, "new x", "new x();\n")
	expectPrinted(t, "new x()", "new x();\n")
	expectPrinted(t, "new (x)", "new x();\n")
	expectPrinted(t, "new (x())", "new (x())();\n")
	expectPrinted(t, "new (new x())", "new new x()();\n")
	expectPrinted(t, "new (x + x)", "new (x + x)();\n")
	expectPrinted(t, "(new x)()", "new x()();\n")

	expectPrinted(t, "new foo().bar", "new foo().bar;\n")
	expectPrinted(t, "new (foo().bar)", "new (foo()).bar();\n")
	expectPrinted(t, "new (foo()).bar", "new (foo()).bar();\n")
	expectPrinted(t, "new foo()[bar]", "new foo()[bar];\n")
	expectPrinted(t, "new (foo()[bar])", "new (foo())[bar]();\n")
	expectPrinted(t, "new (foo())[bar]", "new (foo())[bar]();\n")

	expectPrinted(t, "new (import('foo').bar)", "new (import(\"foo\")).bar();\n")
	expectPrinted(t, "new (import('foo')).bar", "new (import(\"foo\")).bar();\n")
	expectPrinted(t, "new (import('foo')[bar])", "new (import(\"foo\"))[bar]();\n")
	expectPrinted(t, "new (import('foo'))[bar]", "new (import(\"foo\"))[bar]();\n")
}

func TestCall(t *testing.T) {
	expectPrinted(t, "x()()()", "x()()();\n")
	expectPrinted(t, "x().y()[z]()", "x().y()[z]();\n")
	expectPrinted(t, "(--x)();", "(--x)();\n")
	expectPrinted(t, "(x--)();", "(x--)();\n")
}

func TestMember(t *testing.T) {
	expectPrinted(t, "x.y[z]", "x.y[z];\n")
	expectPrinted(t, "((x+1).y+1)[z]", "((x + 1).y + 1)[z];\n")
}

func TestComma(t *testing.T) {
	expectPrinted(t, "1, 2, 3", "1, 2, 3;\n")
	expectPrinted(t, "(1, 2), 3", "1, 2, 3;\n")
	expectPrinted(t, "1, (2, 3)", "1, 2, 3;\n")
	expectPrinted(t, "a ? (b, c) : (d, e)", "a ? (b, c) : (d, e);\n")
	expectPrinted(t, "let x = (a, b)", "let x = (a, b);\n")
	expectPrinted(t, "(x = a), b", "x = a, b;\n")
	expectPrinted(t, "x = (a, b)", "x = (a, b);\n")
	expectPrinted(t, "x((1, 2))", "x((1, 2));\n")
}

func TestUnary(t *testing.T) {
	expectPrinted(t, "+(x--)", "+x--;\n")
	expectPrinted(t, "-(x++)", "-x++;\n")
}

func TestNullish(t *testing.T) {
	// "??" can't directly contain "||" or "&&"
	expectPrinted(t, "(a && b) ?? c", "(a && b) ?? c;\n")
	expectPrinted(t, "(a || b) ?? c", "(a || b) ?? c;\n")
	expectPrinted(t, "a ?? (b && c)", "a ?? (b && c);\n")
	expectPrinted(t, "a ?? (b || c)", "a ?? (b || c);\n")

	// "||" and "&&" can't directly contain "??"
	expectPrinted(t, "a && (b ?? c)", "a && (b ?? c);\n")
	expectPrinted(t, "a || (b ?? c)", "a || (b ?? c);\n")
	expectPrinted(t, "(a ?? b) && c", "(a ?? b) && c;\n")
	expectPrinted(t, "(a ?? b) || c", "(a ?? b) || c;\n")
}

func TestString(t *testing.T) {
	expectPrinted(t, "let x = ''", "let x = \"\";\n")
	expectPrinted(t, "let x = '\b'", "let x = \"\\b\";\n")
	expectPrinted(t, "let x = '\f'", "let x = \"\\f\";\n")
	expectPrinted(t, "let x = '\t'", "let x = \"\t\";\n")
	expectPrinted(t, "let x = '\v'", "let x = \"\\v\";\n")
	expectPrinted(t, "let x = '\\n'", "let x = \"\\n\";\n")
	expectPrinted(t, "let x = '\\''", "let x = \"'\";\n")
	expectPrinted(t, "let x = '\\\"'", "let x = '\"';\n")
	expectPrinted(t, "let x = '\\'\"'", "let x = `'\"`;\n")
	expectPrinted(t, "let x = '\\\\'", "let x = \"\\\\\";\n")
	expectPrinted(t, "let x = '\x00'", "let x = \"\\0\";\n")
	expectPrinted(t, "let x = '\\0'", "let x = \"\\0\";\n")
	expectPrinted(t, "let x = '\x00!'", "let x = \"\\0!\";\n")
	expectPrinted(t, "let x = '\\0!'", "let x = \"\\0!\";\n")
	expectPrinted(t, "let x = '\x001'", "let x = \"\\x001\";\n")
	expectPrinted(t, "let x = '\\01'", "let x = \"\x01\";\n")
	expectPrinted(t, "let x = '\x10'", "let x = \"\x10\";\n")
	expectPrinted(t, "let x = '\\x10'", "let x = \"\x10\";\n")
	expectPrinted(t, "let x = '\uABCD'", "let x = \"\uABCD\";\n")
	expectPrinted(t, "let x = '\\uABCD'", "let x = \"\uABCD\";\n")
	expectPrinted(t, "let x = '\U000123AB'", "let x = \"\U000123AB\";\n")
	expectPrinted(t, "let x = '\\u{123AB}'", "let x = \"\U000123AB\";\n")
	expectPrinted(t, "let x = '\\uD808\\uDFAB'", "let x = \"\U000123AB\";\n")
	expectPrinted(t, "let x = '\\uD808'", "let x = \"\\uD808\";\n")
	expectPrinted(t, "let x = '\\uD808X'", "let x = \"\\uD808X\";\n")
	expectPrinted(t, "let x = '\\uDFAB'", "let x = \"\\uDFAB\";\n")
	expectPrinted(t, "let x = '\\uDFABX'", "let x = \"\\uDFABX\";\n")

	expectPrinted(t, "let x = '\\x80'", "let x = \"\U00000080\";\n")
	expectPrinted(t, "let x = '\\xFF'", "let x = \"\U000000FF\";\n")
	expectPrinted(t, "let x = '\\xF0\\x9F\\x8D\\x95'", "let x = \"\U000000F0\U0000009F\U0000008D\U00000095\";\n")
	expectPrinted(t, "let x = '\\uD801\\uDC02\\uDC03\\uD804'", "let x = \"\U00010402\\uDC03\\uD804\";\n")
}

func TestTemplate(t *testing.T) {
	expectPrinted(t, "let x = `\\1`", "let x = `\x01`;\n")
	expectPrinted(t, "let x = `\\x01`", "let x = `\x01`;\n")
	expectPrinted(t, "let x = `\\1${0}`", "let x = `\x01${0}`;\n")
	expectPrinted(t, "let x = `\\x01${0}`", "let x = `\x01${0}`;\n")
	expectPrinted(t, "let x = `${0}\\1`", "let x = `${0}\x01`;\n")
	expectPrinted(t, "let x = `${0}\\x01`", "let x = `${0}\x01`;\n")
	expectPrinted(t, "let x = `${0}\\1${1}`", "let x = `${0}\x01${1}`;\n")
	expectPrinted(t, "let x = `${0}\\x01${1}`", "let x = `${0}\x01${1}`;\n")

	expectPrinted(t, "let x = String.raw`\\1`", "let x = String.raw`\\1`;\n")
	expectPrinted(t, "let x = String.raw`\\x01`", "let x = String.raw`\\x01`;\n")
	expectPrinted(t, "let x = String.raw`\\1${0}`", "let x = String.raw`\\1${0}`;\n")
	expectPrinted(t, "let x = String.raw`\\x01${0}`", "let x = String.raw`\\x01${0}`;\n")
	expectPrinted(t, "let x = String.raw`${0}\\1`", "let x = String.raw`${0}\\1`;\n")
	expectPrinted(t, "let x = String.raw`${0}\\x01`", "let x = String.raw`${0}\\x01`;\n")
	expectPrinted(t, "let x = String.raw`${0}\\1${1}`", "let x = String.raw`${0}\\1${1}`;\n")
	expectPrinted(t, "let x = String.raw`${0}\\x01${1}`", "let x = String.raw`${0}\\x01${1}`;\n")

	expectPrinted(t, "let x = `${y}`", "let x = `${y}`;\n")
	expectPrinted(t, "let x = `$(y)`", "let x = `$(y)`;\n")
	expectPrinted(t, "let x = `{y}$`", "let x = `{y}$`;\n")
	expectPrinted(t, "let x = `$}y{`", "let x = `$}y{`;\n")
	expectPrinted(t, "let x = `\\${y}`", "let x = `\\${y}`;\n")
	expectPrinted(t, "let x = `$\\{y}`", "let x = `\\${y}`;\n")
}

func TestObject(t *testing.T) {
	expectPrinted(t, "let x = {'(':')'}", "let x = {\n  \"(\": \")\"\n};\n")
}

func TestFor(t *testing.T) {
	// Make sure "in" expressions are forbidden in the right places
	expectPrinted(t, "for ((a in b);;);", "for ((a in b); ; )\n  ;\n")
	expectPrinted(t, "for (a ? b : (c in d);;);", "for (a ? b : (c in d); ; )\n  ;\n")
	expectPrinted(t, "for ((a ? b : c in d).foo;;);", "for ((a ? b : c in d).foo; ; )\n  ;\n")
	expectPrinted(t, "for (var x = (a in b);;);", "for (var x = (a in b); ; )\n  ;\n")
	expectPrinted(t, "for (x = (a in b);;);", "for (x = (a in b); ; )\n  ;\n")
	expectPrinted(t, "for (x == (a in b);;);", "for (x == (a in b); ; )\n  ;\n")
	expectPrinted(t, "for (1 * (x == a in b);;);", "for (1 * (x == a in b); ; )\n  ;\n")
	expectPrinted(t, "for (a ? b : x = (c in d);;);", "for (a ? b : x = (c in d); ; )\n  ;\n")
	expectPrinted(t, "for (var x = y = (a in b);;);", "for (var x = y = (a in b); ; )\n  ;\n")
	expectPrinted(t, "for ([a in b];;);", "for ([a in b]; ; )\n  ;\n")
	expectPrinted(t, "for (x(a in b);;);", "for (x(a in b); ; )\n  ;\n")
	expectPrinted(t, "for (x[a in b];;);", "for (x[a in b]; ; )\n  ;\n")
	expectPrinted(t, "for (x?.[a in b];;);", "for (x?.[a in b]; ; )\n  ;\n")

	// Make sure iterables with commas are wrapped in parentheses
	expectPrinted(t, "for (let a in (b, c));", "for (let a in (b, c))\n  ;\n")
	expectPrinted(t, "for (let a of (b, c));", "for (let a of (b, c))\n  ;\n")
}

func TestFunction(t *testing.T) {
	expectPrinted(t,
		"function foo(a = (b, c), ...d) {}",
		"function foo(a = (b, c), ...d) {\n}\n")
	expectPrinted(t,
		"function foo({[1 + 2]: a = 3} = {[1 + 2]: 3}) {}",
		"function foo({[1 + 2]: a = 3} = {\n  [1 + 2]: 3\n}) {\n}\n")
	expectPrinted(t,
		"function foo([a = (1, 2), ...[b, ...c]] = [1, [2, 3]]) {}",
		"function foo([a = (1, 2), ...[b, ...c]] = [1, [2, 3]]) {\n}\n")
	expectPrinted(t,
		"function foo([] = []) {}",
		"function foo([] = []) {\n}\n")
	expectPrinted(t,
		"function foo([,] = [,]) {}",
		"function foo([,] = [,]) {\n}\n")
	expectPrinted(t,
		"function foo([,,] = [,,]) {}",
		"function foo([, ,] = [, ,]) {\n}\n")
}

func TestGenerator(t *testing.T) {
	expectPrinted(t,
		"function* foo() {}",
		"function* foo() {\n}\n")
	expectPrinted(t,
		"(function* () {})",
		"(function* () {\n});\n")
	expectPrinted(t,
		"(function* foo() {})",
		"(function* foo() {\n});\n")

	expectPrinted(t,
		"class Foo { *foo() {} }",
		"class Foo {\n  *foo() {\n  }\n}\n")
	expectPrinted(t,
		"class Foo { static *foo() {} }",
		"class Foo {\n  static *foo() {\n  }\n}\n")
	expectPrinted(t,
		"class Foo { *[foo]() {} }",
		"class Foo {\n  *[foo]() {\n  }\n}\n")
	expectPrinted(t,
		"class Foo { static *[foo]() {} }",
		"class Foo {\n  static *[foo]() {\n  }\n}\n")

	expectPrinted(t,
		"(class { *foo() {} })",
		"(class {\n  *foo() {\n  }\n});\n")
	expectPrinted(t,
		"(class { static *foo() {} })",
		"(class {\n  static *foo() {\n  }\n});\n")
	expectPrinted(t,
		"(class { *[foo]() {} })",
		"(class {\n  *[foo]() {\n  }\n});\n")
	expectPrinted(t,
		"(class { static *[foo]() {} })",
		"(class {\n  static *[foo]() {\n  }\n});\n")
}

func TestArrow(t *testing.T) {
	expectPrinted(t, "() => {}", "() => {\n};\n")
	expectPrinted(t, "x => (x, 0)", "(x) => (x, 0);\n")
	expectPrinted(t, "x => {y}", "(x) => {\n  y;\n};\n")
	expectPrinted(t,
		"(a = (b, c), ...d) => {}",
		"(a = (b, c), ...d) => {\n};\n")
	expectPrinted(t,
		"({[1 + 2]: a = 3} = {[1 + 2]: 3}) => {}",
		"({[1 + 2]: a = 3} = {\n  [1 + 2]: 3\n}) => {\n};\n")
	expectPrinted(t,
		"([a = (1, 2), ...[b, ...c]] = [1, [2, 3]]) => {}",
		"([a = (1, 2), ...[b, ...c]] = [1, [2, 3]]) => {\n};\n")
	expectPrinted(t,
		"([] = []) => {}",
		"([] = []) => {\n};\n")
	expectPrinted(t,
		"([,] = [,]) => {}",
		"([,] = [,]) => {\n};\n")
	expectPrinted(t,
		"([,,] = [,,]) => {}",
		"([, ,] = [, ,]) => {\n};\n")
	expectPrinted(t,
		"a = () => {}",
		"a = () => {\n};\n")
	expectPrinted(t,
		"a || (() => {})",
		"a || (() => {\n});\n")
	expectPrinted(t,
		"({a = b, c = d}) => {}",
		"({a = b, c = d}) => {\n};\n")
	expectPrinted(t,
		"([{a = b, c = d} = {}] = []) => {}",
		"([{a = b, c = d} = {}] = []) => {\n};\n")
	expectPrinted(t,
		"({a: [b = c] = []} = {}) => {}",
		"({a: [b = c] = []} = {}) => {\n};\n")

	// These are not arrow functions but initially look like one
	expectPrinted(t, "(a = b, c)", "a = b, c;\n")
	expectPrinted(t, "([...a = b])", "[...a = b];\n")
	expectPrinted(t, "([...a, ...b])", "[...a, ...b];\n")
	expectPrinted(t, "({a: b, c() {}})", "({\n  a: b,\n  c() {\n  }\n});\n")
	expectPrinted(t, "({a: b, get c() {}})", "({\n  a: b,\n  get c() {\n  }\n});\n")
	expectPrinted(t, "({a: b, set c() {}})", "({\n  a: b,\n  set c() {\n  }\n});\n")
}

func TestClass(t *testing.T) {
	expectPrinted(t, "class Foo extends (a, b) {}", "class Foo extends (a, b) {\n}\n")
	expectPrinted(t, "class Foo { get foo() {} }", "class Foo {\n  get foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { set foo() {} }", "class Foo {\n  set foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static foo() {} }", "class Foo {\n  static foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static get foo() {} }", "class Foo {\n  static get foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static set foo() {} }", "class Foo {\n  static set foo() {\n  }\n}\n")
}

func TestImport(t *testing.T) {
	expectPrinted(t, "import('path');", "import(\"path\");\n") // The semicolon must not be a separate statement
}

func TestExportDefault(t *testing.T) {
	expectPrinted(t, "export default function() {}", "export default function() {\n}\n")
	expectPrinted(t, "export default function foo() {}", "export default function foo() {\n}\n")
	expectPrinted(t, "export default async function() {}", "export default async function() {\n}\n")
	expectPrinted(t, "export default async function foo() {}", "export default async function foo() {\n}\n")
	expectPrinted(t, "export default class {}", "export default class {\n}\n")
	expectPrinted(t, "export default class foo {}", "export default class foo {\n}\n")

	expectPrinted(t, "export default (function() {})", "export default (function() {\n});\n")
	expectPrinted(t, "export default (function foo() {})", "export default (function foo() {\n});\n")
	expectPrinted(t, "export default (async function() {})", "export default (async function() {\n});\n")
	expectPrinted(t, "export default (async function foo() {})", "export default (async function foo() {\n});\n")
	expectPrinted(t, "export default (class {})", "export default (class {\n});\n")
	expectPrinted(t, "export default (class foo {})", "export default (class foo {\n});\n")

	expectPrinted(t, "export default (function() {}.toString())", "export default (function() {\n}).toString();\n")
	expectPrinted(t, "export default (function foo() {}.toString())", "export default (function foo() {\n}).toString();\n")
	expectPrinted(t, "export default (async function() {}.toString())", "export default (async function() {\n}).toString();\n")
	expectPrinted(t, "export default (async function foo() {}.toString())", "export default (async function foo() {\n}).toString();\n")
	expectPrinted(t, "export default (class {}.toString())", "export default (class {\n}).toString();\n")
	expectPrinted(t, "export default (class foo {}.toString())", "export default (class foo {\n}).toString();\n")

	expectPrintedMinify(t, "export default function() {}", "export default function(){}")
	expectPrintedMinify(t, "export default function foo() {}", "export default function foo(){}")
	expectPrintedMinify(t, "export default async function() {}", "export default async function(){}")
	expectPrintedMinify(t, "export default async function foo() {}", "export default async function foo(){}")
	expectPrintedMinify(t, "export default class {}", "export default class{}")
	expectPrintedMinify(t, "export default class foo {}", "export default class foo{}")
}

func TestWhitespace(t *testing.T) {
	expectPrinted(t, "- -x", "- -x;\n")
	expectPrinted(t, "+ -x", "+-x;\n")
	expectPrinted(t, "- +x", "-+x;\n")
	expectPrinted(t, "+ +x", "+ +x;\n")
	expectPrinted(t, "- --x", "- --x;\n")
	expectPrinted(t, "+ --x", "+--x;\n")
	expectPrinted(t, "- ++x", "-++x;\n")
	expectPrinted(t, "+ ++x", "+ ++x;\n")

	expectPrintedMinify(t, "- -x", "- -x;")
	expectPrintedMinify(t, "+ -x", "+-x;")
	expectPrintedMinify(t, "- +x", "-+x;")
	expectPrintedMinify(t, "+ +x", "+ +x;")
	expectPrintedMinify(t, "- --x", "- --x;")
	expectPrintedMinify(t, "+ --x", "+--x;")
	expectPrintedMinify(t, "- ++x", "-++x;")
	expectPrintedMinify(t, "+ ++x", "+ ++x;")

	expectPrintedMinify(t, "x - --y", "x- --y;")
	expectPrintedMinify(t, "x + --y", "x+--y;")
	expectPrintedMinify(t, "x - ++y", "x-++y;")
	expectPrintedMinify(t, "x + ++y", "x+ ++y;")

	expectPrintedMinify(t, "x-- > y", "x-- >y;")
	expectPrintedMinify(t, "x < !--y", "x<! --y;")
	expectPrintedMinify(t, "x > !--y", "x>!--y;")
	expectPrintedMinify(t, "!--y", "!--y;")

	expectPrintedMinify(t, "1 + -0", "1+-0;")
	expectPrintedMinify(t, "1 - -0", "1- -0;")
	expectPrintedMinify(t, "1 + -Infinity", "1+-Infinity;")
	expectPrintedMinify(t, "1 - -Infinity", "1- -Infinity;")

	expectPrintedMinify(t, "/x/ / /y/", "/x// /y/;")
	expectPrintedMinify(t, "/x/ + Foo", "/x/+Foo;")
	expectPrintedMinify(t, "/x/ instanceof Foo", "/x/ instanceof Foo;")
	expectPrintedMinify(t, "[x] instanceof Foo", "[x]instanceof Foo;")

	expectPrintedMinify(t, "throw x", "throw x;")
	expectPrintedMinify(t, "throw typeof x", "throw typeof x;")
	expectPrintedMinify(t, "throw delete x", "throw delete x;")
	expectPrintedMinify(t, "throw function(){}", "throw function(){};")

	expectPrintedMinify(t, "x in function(){}", "x in function(){};")
	expectPrintedMinify(t, "x instanceof function(){}", "x instanceof function(){};")

	expectPrintedMinify(t, "()=>({})", "()=>({});")
	expectPrintedMinify(t, "()=>({}[1])", "()=>({})[1];")
	expectPrintedMinify(t, "()=>({}+0)", "()=>({})+0;")
	expectPrintedMinify(t, "()=>function(){}", "()=>function(){};")

	expectPrintedMinify(t, "(function(){})", "(function(){});")
	expectPrintedMinify(t, "(class{})", "(class{});")
	expectPrintedMinify(t, "({})", "({});")

	expectPrintedMinify(t, "let x = '\\n'", "let x=`\n`;")
	expectPrintedMinify(t, "let x = `\n`", "let x=`\n`;")
	expectPrintedMinify(t, "let x = '\\n${}'", "let x=\"\\n${}\";")
	expectPrintedMinify(t, "let x = `\n\\${}`", "let x=\"\\n${}\";")
	expectPrintedMinify(t, "let x = `\n\\${}${y}\\${}`", "let x=`\n\\${}${y}\\${}`;")
}

func TestMinify(t *testing.T) {
	expectPrintedMinify(t, "0.1", ".1;")
	expectPrintedMinify(t, "1.2", "1.2;")

	expectPrintedMinify(t, "() => {}", "()=>{};")
	expectPrintedMinify(t, "(a) => {}", "a=>{};")
	expectPrintedMinify(t, "(...a) => {}", "(...a)=>{};")
	expectPrintedMinify(t, "(a = 0) => {}", "(a=0)=>{};")
	expectPrintedMinify(t, "(a, b) => {}", "(a,b)=>{};")

	expectPrinted(t, "true ** 2", "true ** 2;\n")
	expectPrinted(t, "false ** 2", "false ** 2;\n")
	expectPrintedMinify(t, "true ** 2", "(!0)**2;")
	expectPrintedMinify(t, "false ** 2", "(!1)**2;")

	expectPrintedMinify(t, "import a from 'path'", "import a from\"path\";")
	expectPrintedMinify(t, "import * as ns from 'path'", "import*as ns from\"path\";")
	expectPrintedMinify(t, "import {a, b as c} from 'path'", "import{a,b as c}from\"path\";")

	expectPrintedMinify(t, "export * as ns from 'path'", "export*as ns from\"path\";")
	expectPrintedMinify(t, "export {a, b as c} from 'path'", "export{a,b as c}from\"path\";")

	// Print some strings using template literals when minifying
	expectPrinted(t, "'\\n'", "\"\\n\";\n")
	expectPrintedMinify(t, "'\\n'", "`\n`;")
	expectPrintedMinify(t, "({'\\n': 0})", "({\"\\n\":0});")
	expectPrintedMinify(t, "(class{'\\n' = 0})", "(class{\"\\n\"=0});")
	expectPrintedMinify(t, "class Foo{'\\n' = 0}", "class Foo{\"\\n\"=0}")

	// Special identifiers must not be minified
	expectPrintedMinify(t, "exports", "exports;")
	expectPrintedMinify(t, "require", "require;")
	expectPrintedMinify(t, "module", "module;")
}
