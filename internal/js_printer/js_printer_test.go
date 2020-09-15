package js_printer

import (
	"testing"

	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_parser"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/renamer"
	"github.com/evanw/esbuild/internal/test"
)

func assertEqual(t *testing.T, a interface{}, b interface{}) {
	t.Helper()
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
}

func expectPrintedCommon(t *testing.T, name string, contents string, expected string, options PrintOptions) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog()
		tree, ok := js_parser.Parse(log, test.SourceForTest(contents), config.Options{})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logger.StderrOptions{}, logger.TerminalInfo{})
		}
		assertEqual(t, text, "")
		if !ok {
			t.Fatal("Parse error")
		}
		symbols := js_ast.NewSymbolMap(1)
		symbols.Outer[0] = tree.Symbols
		r := renamer.NewNoOpRenamer(symbols)
		js := Print(tree, symbols, r, options).JS
		assertEqual(t, string(js), expected)
	})
}

func expectPrinted(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, contents, expected, PrintOptions{})
}

func expectPrintedMinify(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [minified]", contents, expected, PrintOptions{
		RemoveWhitespace: true,
	})
}

func expectPrintedMangle(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [minified]", contents, expected, PrintOptions{
		MangleSyntax: true,
	})
}

func expectPrintedTarget(t *testing.T, esVersion int, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, contents, expected, PrintOptions{
		UnsupportedFeatures: compat.UnsupportedFeatures(map[compat.Engine][]int{
			compat.ES: {esVersion},
		}),
	})
}

func expectPrintedTargetMinify(t *testing.T, esVersion int, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [minified]", contents, expected, PrintOptions{
		UnsupportedFeatures: compat.UnsupportedFeatures(map[compat.Engine][]int{
			compat.ES: {esVersion},
		}),
		RemoveWhitespace: true,
	})
}

func expectPrintedTargetMangle(t *testing.T, esVersion int, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [minified]", contents, expected, PrintOptions{
		UnsupportedFeatures: compat.UnsupportedFeatures(map[compat.Engine][]int{
			compat.ES: {esVersion},
		}),
		MangleSyntax: true,
	})
}

func TestNumber(t *testing.T) {
	// Check "1eN"
	expectPrinted(t, "1e-100", "1e-100;\n")
	expectPrinted(t, "1e-4", "1e-4;\n")
	expectPrinted(t, "1e-3", "1e-3;\n")
	expectPrinted(t, "1e-2", "0.01;\n")
	expectPrinted(t, "1e-1", "0.1;\n")
	expectPrinted(t, "1e0", "1;\n")
	expectPrinted(t, "1e1", "10;\n")
	expectPrinted(t, "1e2", "100;\n")
	expectPrinted(t, "1e3", "1e3;\n")
	expectPrinted(t, "1e4", "1e4;\n")
	expectPrinted(t, "1e100", "1e100;\n")
	expectPrintedMinify(t, "1e-100", "1e-100;")
	expectPrintedMinify(t, "1e-5", "1e-5;")
	expectPrintedMinify(t, "1e-4", "1e-4;")
	expectPrintedMinify(t, "1e-3", ".001;")
	expectPrintedMinify(t, "1e-2", ".01;")
	expectPrintedMinify(t, "1e-1", ".1;")
	expectPrintedMinify(t, "1e0", "1;")
	expectPrintedMinify(t, "1e1", "10;")
	expectPrintedMinify(t, "1e2", "100;")
	expectPrintedMinify(t, "1e3", "1e3;")
	expectPrintedMinify(t, "1e4", "1e4;")
	expectPrintedMinify(t, "1e100", "1e100;")

	// Check "12eN"
	expectPrinted(t, "12e-100", "12e-100;\n")
	expectPrinted(t, "12e-5", "12e-5;\n")
	expectPrinted(t, "12e-4", "12e-4;\n")
	expectPrinted(t, "12e-3", "0.012;\n")
	expectPrinted(t, "12e-2", "0.12;\n")
	expectPrinted(t, "12e-1", "1.2;\n")
	expectPrinted(t, "12e0", "12;\n")
	expectPrinted(t, "12e1", "120;\n")
	expectPrinted(t, "12e2", "1200;\n")
	expectPrinted(t, "12e3", "12e3;\n")
	expectPrinted(t, "12e4", "12e4;\n")
	expectPrinted(t, "12e100", "12e100;\n")
	expectPrintedMinify(t, "12e-100", "12e-100;")
	expectPrintedMinify(t, "12e-6", "12e-6;")
	expectPrintedMinify(t, "12e-5", "12e-5;")
	expectPrintedMinify(t, "12e-4", ".0012;")
	expectPrintedMinify(t, "12e-3", ".012;")
	expectPrintedMinify(t, "12e-2", ".12;")
	expectPrintedMinify(t, "12e-1", "1.2;")
	expectPrintedMinify(t, "12e0", "12;")
	expectPrintedMinify(t, "12e1", "120;")
	expectPrintedMinify(t, "12e2", "1200;")
	expectPrintedMinify(t, "12e3", "12e3;")
	expectPrintedMinify(t, "12e4", "12e4;")
	expectPrintedMinify(t, "12e100", "12e100;")

	// Check cases for "A.BeX" => "ABeY" simplification
	expectPrinted(t, "123456789", "123456789;\n")
	expectPrinted(t, "1123456789", "1123456789;\n")
	expectPrinted(t, "10123456789", "10123456789;\n")
	expectPrinted(t, "100123456789", "100123456789;\n")
	expectPrinted(t, "1000123456789", "1000123456789;\n")
	expectPrinted(t, "10000123456789", "10000123456789;\n")
	expectPrinted(t, "100000123456789", "100000123456789;\n")
	expectPrinted(t, "1000000123456789", "1000000123456789;\n")
	expectPrinted(t, "10000000123456789", "10000000123456788;\n")
	expectPrinted(t, "100000000123456789", "100000000123456780;\n")
	expectPrinted(t, "1000000000123456789", "1000000000123456800;\n")
	expectPrinted(t, "10000000000123456789", "10000000000123458e3;\n")
	expectPrinted(t, "100000000000123456789", "10000000000012345e4;\n")

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
	expectPrinted(t, "0x7fff_ffff_ffff_fdff", "9223372036854775e3;\n")
	expectPrinted(t, "0x8000_0000_0000_0000", "9223372036854776e3;\n")
	expectPrinted(t, "0x8000_0000_0000_3000", "9223372036854788e3;\n")
	expectPrinted(t, "-0x7fff_ffff_ffff_fdff", "-9223372036854775e3;\n")
	expectPrinted(t, "-0x8000_0000_0000_0000", "-9223372036854776e3;\n")
	expectPrinted(t, "-0x8000_0000_0000_3000", "-9223372036854788e3;\n")

	// uint64
	expectPrinted(t, "0xffff_ffff_ffff_fbff", "1844674407370955e4;\n")
	expectPrinted(t, "0x1_0000_0000_0000_0000", "18446744073709552e3;\n")
	expectPrinted(t, "0x1_0000_0000_0000_1000", "18446744073709556e3;\n")
	expectPrinted(t, "-0xffff_ffff_ffff_fbff", "-1844674407370955e4;\n")
	expectPrinted(t, "-0x1_0000_0000_0000_0000", "-18446744073709552e3;\n")
	expectPrinted(t, "-0x1_0000_0000_0000_1000", "-18446744073709556e3;\n")
}

func TestArray(t *testing.T) {
	expectPrinted(t, "[]", "[];\n")
	expectPrinted(t, "[,]", "[,];\n")
	expectPrinted(t, "[,,]", "[, ,];\n")
}

func TestSplat(t *testing.T) {
	expectPrinted(t, "[...(a, b)]", "[...(a, b)];\n")
	expectPrinted(t, "x(...(a, b))", "x(...(a, b));\n")
	expectPrinted(t, "({...(a, b)})", "({...(a, b)});\n")
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

	expectPrintedMinify(t, "new x", "new x;")
	expectPrintedMinify(t, "new x.y", "new x.y;")
	expectPrintedMinify(t, "(new x).y", "new x().y;")
	expectPrintedMinify(t, "new x().y", "new x().y;")
	expectPrintedMinify(t, "new x() + y", "new x+y;")
	expectPrintedMinify(t, "new x() ** 2", "new x**2;")
}

func TestCall(t *testing.T) {
	expectPrinted(t, "x()()()", "x()()();\n")
	expectPrinted(t, "x().y()[z]()", "x().y()[z]();\n")
	expectPrinted(t, "(--x)();", "(--x)();\n")
	expectPrinted(t, "(x--)();", "(x--)();\n")

	expectPrinted(t, "eval(x)", "eval(x);\n")
	expectPrinted(t, "eval?.(x)", "eval?.(x);\n")
	expectPrinted(t, "(eval)(x)", "eval(x);\n")
	expectPrinted(t, "(eval)?.(x)", "eval?.(x);\n")

	expectPrinted(t, "eval(x, y)", "eval(x, y);\n")
	expectPrinted(t, "eval?.(x, y)", "eval?.(x, y);\n")
	expectPrinted(t, "(1, eval)(x)", "(1, eval)(x);\n")
	expectPrinted(t, "(1, eval)?.(x)", "(1, eval)?.(x);\n")
	expectPrinted(t, "(1 ? eval : 2)(x)", "(0, eval)(x);\n")
	expectPrinted(t, "(1 ? eval : 2)?.(x)", "(0, eval)?.(x);\n")

	expectPrintedMinify(t, "eval?.(x)", "eval?.(x);")
	expectPrintedMinify(t, "eval(x,y)", "eval(x,y);")
	expectPrintedMinify(t, "eval?.(x,y)", "eval?.(x,y);")
	expectPrintedMinify(t, "(1, eval)(x)", "(1,eval)(x);")
	expectPrintedMinify(t, "(1, eval)?.(x)", "(1,eval)?.(x);")
	expectPrintedMinify(t, "(1 ? eval : 2)(x)", "(0,eval)(x);")
	expectPrintedMinify(t, "(1 ? eval : 2)?.(x)", "(0,eval)?.(x);")
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
	expectPrinted(t, "let x = '\x00!'", "let x = \"\\0!\";\n")
	expectPrinted(t, "let x = '\x001'", "let x = \"\\x001\";\n")
	expectPrinted(t, "let x = '\\0'", "let x = \"\\0\";\n")
	expectPrinted(t, "let x = '\\0!'", "let x = \"\\0!\";\n")
	expectPrinted(t, "let x = '\x07'", "let x = \"\\x07\";\n")
	expectPrinted(t, "let x = '\x07!'", "let x = \"\\x07!\";\n")
	expectPrinted(t, "let x = '\x071'", "let x = \"\\x071\";\n")
	expectPrinted(t, "let x = '\\7'", "let x = \"\\x07\";\n")
	expectPrinted(t, "let x = '\\7!'", "let x = \"\\x07!\";\n")
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

	expectPrinted(t, "await tag`x`", "await tag`x`;\n")
	expectPrinted(t, "await (tag`x`)", "await tag`x`;\n")
	expectPrinted(t, "(await tag)`x`", "(await tag)`x`;\n")

	expectPrinted(t, "await tag`${x}`", "await tag`${x}`;\n")
	expectPrinted(t, "await (tag`${x}`)", "await tag`${x}`;\n")
	expectPrinted(t, "(await tag)`${x}`", "(await tag)`${x}`;\n")

	expectPrinted(t, "new tag`x`", "new tag`x`();\n")
	expectPrinted(t, "new (tag`x`)", "new tag`x`();\n")
	expectPrinted(t, "new tag()`x`", "new tag()`x`;\n")
	expectPrinted(t, "(new tag)`x`", "new tag()`x`;\n")
	expectPrintedMinify(t, "new tag`x`", "new tag`x`;")
	expectPrintedMinify(t, "new (tag`x`)", "new tag`x`;")
	expectPrintedMinify(t, "new tag()`x`", "new tag()`x`;")
	expectPrintedMinify(t, "(new tag)`x`", "new tag()`x`;")

	expectPrinted(t, "new tag`${x}`", "new tag`${x}`();\n")
	expectPrinted(t, "new (tag`${x}`)", "new tag`${x}`();\n")
	expectPrinted(t, "new tag()`${x}`", "new tag()`${x}`;\n")
	expectPrinted(t, "(new tag)`${x}`", "new tag()`${x}`;\n")
	expectPrintedMinify(t, "new tag`${x}`", "new tag`${x}`;")
	expectPrintedMinify(t, "new (tag`${x}`)", "new tag`${x}`;")
	expectPrintedMinify(t, "new tag()`${x}`", "new tag()`${x}`;")
	expectPrintedMinify(t, "(new tag)`${x}`", "new tag()`${x}`;")
}

func TestObject(t *testing.T) {
	expectPrinted(t, "let x = {'(':')'}", "let x = {\"(\": \")\"};\n")
	expectPrinted(t, "({})", "({});\n")
	expectPrinted(t, "({}.x)", "({}).x;\n")
	expectPrinted(t, "({} = {})", "({} = {});\n")
	expectPrinted(t, "(x, {} = {})", "x, {} = {};\n")
	expectPrinted(t, "let x = () => ({})", "let x = () => ({});\n")
	expectPrinted(t, "let x = () => ({}.x)", "let x = () => ({}).x;\n")
	expectPrinted(t, "let x = () => ({} = {})", "let x = () => ({} = {});\n")
	expectPrinted(t, "let x = () => (x, {} = {})", "let x = () => (x, {} = {});\n")
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

	// Make sure for-of loops with commas are wrapped in parentheses
	expectPrinted(t, "for (let a in b, c);", "for (let a in b, c)\n  ;\n")
	expectPrinted(t, "for (let a of (b, c));", "for (let a of (b, c))\n  ;\n")
}

func TestFunction(t *testing.T) {
	expectPrinted(t,
		"function foo(a = (b, c), ...d) {}",
		"function foo(a = (b, c), ...d) {\n}\n")
	expectPrinted(t,
		"function foo({[1 + 2]: a = 3} = {[1 + 2]: 3}) {}",
		"function foo({[1 + 2]: a = 3} = {[1 + 2]: 3}) {\n}\n")
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

func TestPureComment(t *testing.T) {
	expectPrinted(t,
		"(function() {})",
		"(function() {\n});\n")
	expectPrinted(t,
		"(function() {})()",
		"(function() {\n})();\n")
	expectPrinted(t,
		"/*@__PURE__*/(function() {})()",
		"/* @__PURE__ */ (function() {\n})();\n")

	expectPrinted(t,
		"new (function() {})",
		"new function() {\n}();\n")
	expectPrinted(t,
		"new (function() {})()",
		"new function() {\n}();\n")
	expectPrinted(t,
		"/*@__PURE__*/new (function() {})()",
		"/* @__PURE__ */ new function() {\n}();\n")
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
		"({[1 + 2]: a = 3} = {[1 + 2]: 3}) => {\n};\n")
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
	expectPrinted(t, "({a: b, c() {}})", "({a: b, c() {\n}});\n")
	expectPrinted(t, "({a: b, get c() {}})", "({a: b, get c() {\n}});\n")
	expectPrinted(t, "({a: b, set c(x) {}})", "({a: b, set c(x) {\n}});\n")
}

func TestClass(t *testing.T) {
	expectPrinted(t, "class Foo extends (a, b) {}", "class Foo extends (a, b) {\n}\n")
	expectPrinted(t, "class Foo { get foo() {} }", "class Foo {\n  get foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { set foo(x) {} }", "class Foo {\n  set foo(x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { static foo() {} }", "class Foo {\n  static foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static get foo() {} }", "class Foo {\n  static get foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static set foo(x) {} }", "class Foo {\n  static set foo(x) {\n  }\n}\n")
}

func TestImport(t *testing.T) {
	expectPrinted(t, "import('path');", "import(\"path\");\n") // The semicolon must not be a separate statement

	// Test preservation of leading interior comments
	expectPrinted(t, "import(// comment 1\n // comment 2\n 'path');", "import(\n  // comment 1\n  // comment 2\n  \"path\"\n);\n")
	expectPrinted(t, "import(/* comment 1 */ /* comment 2 */ 'path');", "import(\n  /* comment 1 */\n  /* comment 2 */\n  \"path\"\n);\n")
	expectPrinted(t, "import(\n    /* multi\n     * line\n     * comment */ 'path');", "import(\n  /* multi\n   * line\n   * comment */\n  \"path\"\n);\n")
	expectPrinted(t, "import(/* comment 1 */ 'path' /* comment 2 */);", "import(\n  /* comment 1 */\n  \"path\"\n);\n")
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
	expectPrintedMinify(t, "()=>({}+0)", "()=>({}+0);")
	expectPrintedMinify(t, "()=>function(){}", "()=>function(){};")

	expectPrintedMinify(t, "(function(){})", "(function(){});")
	expectPrintedMinify(t, "(class{})", "(class{});")
	expectPrintedMinify(t, "({})", "({});")
}

func TestMangle(t *testing.T) {
	expectPrintedMangle(t, "let x = '\\n'", "let x = `\n`;\n")
	expectPrintedMangle(t, "let x = `\n`", "let x = `\n`;\n")
	expectPrintedMangle(t, "let x = '\\n${}'", "let x = \"\\n${}\";\n")
	expectPrintedMangle(t, "let x = `\n\\${}`", "let x = \"\\n${}\";\n")
	expectPrintedMangle(t, "let x = `\n\\${}${y}\\${}`", "let x = `\n\\${}${y}\\${}`;\n")
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
	expectPrintedMinify(t, "true ** 2", "true**2;")
	expectPrintedMinify(t, "false ** 2", "false**2;")
	expectPrintedMangle(t, "true ** 2", "(!0) ** 2;\n")
	expectPrintedMangle(t, "false ** 2", "(!1) ** 2;\n")

	expectPrintedMinify(t, "import a from 'path'", "import a from\"path\";")
	expectPrintedMinify(t, "import * as ns from 'path'", "import*as ns from\"path\";")
	expectPrintedMinify(t, "import {a, b as c} from 'path'", "import{a,b as c}from\"path\";")

	expectPrintedMinify(t, "export * as ns from 'path'", "export*as ns from\"path\";")
	expectPrintedMinify(t, "export {a, b as c} from 'path'", "export{a,b as c}from\"path\";")

	// Print some strings using template literals when minifying
	expectPrinted(t, "'\\n'", "\"\\n\";\n")
	expectPrintedMangle(t, "'\\n'", "`\n`;\n")
	expectPrintedMangle(t, "({'\\n': 0})", "({\"\\n\": 0});\n")
	expectPrintedMangle(t, "(class{'\\n' = 0})", "(class {\n  \"\\n\" = 0;\n});\n")
	expectPrintedMangle(t, "class Foo{'\\n' = 0}", "class Foo {\n  \"\\n\" = 0;\n}\n")

	// Special identifiers must not be minified
	expectPrintedMinify(t, "exports", "exports;")
	expectPrintedMinify(t, "require", "require;")
	expectPrintedMinify(t, "module", "module;")

	// Comment statements must not affect their surroundings when minified
	expectPrintedMinify(t, "//!single\nthrow 1 + 2", "//!single\nthrow 1+2;")
	expectPrintedMinify(t, "/*!multi-\nline*/\nthrow 1 + 2", "/*!multi-\nline*/throw 1+2;")
}

func TestES5(t *testing.T) {
	expectPrintedTargetMangle(t, 5, "foo('a\\n\\n\\nb')", "foo(\"a\\n\\n\\nb\");\n")
	expectPrintedTargetMangle(t, 2015, "foo('a\\n\\n\\nb')", "foo(`a\n\n\nb`);\n")

	expectPrintedTarget(t, 5, "foo({a, b})", "foo({a: a, b: b});\n")
	expectPrintedTarget(t, 2015, "foo({a, b})", "foo({a, b});\n")

	expectPrintedTarget(t, 5, "x => x", "(function(x) {\n  return x;\n});\n")
	expectPrintedTarget(t, 2015, "x => x", "(x) => x;\n")

	expectPrintedTarget(t, 5, "() => {}", "(function() {\n});\n")
	expectPrintedTarget(t, 2015, "() => {}", "() => {\n};\n")

	expectPrintedTargetMinify(t, 5, "x => x", "(function(x){return x});")
	expectPrintedTargetMinify(t, 2015, "x => x", "x=>x;")

	expectPrintedTargetMinify(t, 5, "() => {}", "(function(){});")
	expectPrintedTargetMinify(t, 2015, "() => {}", "()=>{};")
}
