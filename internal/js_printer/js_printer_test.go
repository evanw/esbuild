package js_printer

import (
	"strings"
	"testing"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/js_parser"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/renamer"
	"github.com/evanw/esbuild/internal/test"
)

func expectPrintedCommon(t *testing.T, name string, contents string, expected string, options config.Options) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		tree, ok := js_parser.Parse(log, test.SourceForTest(contents), js_parser.OptionsFromConfig(&options))
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			if msg.Kind != logger.Error {
				continue
			}
			text += msg.String(logger.OutputOptions{}, logger.TerminalInfo{})
		}
		test.AssertEqualWithDiff(t, text, "")
		if !ok {
			t.Fatal("Parse error")
		}
		symbols := ast.NewSymbolMap(1)
		symbols.SymbolsForSource[0] = tree.Symbols
		r := renamer.NewNoOpRenamer(symbols)
		js := Print(tree, symbols, r, Options{
			ASCIIOnly:           options.ASCIIOnly,
			MinifySyntax:        options.MinifySyntax,
			MinifyWhitespace:    options.MinifyWhitespace,
			UnsupportedFeatures: options.UnsupportedJSFeatures,
		}).JS
		test.AssertEqualWithDiff(t, string(js), expected)
	})
}

func expectPrinted(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, contents, expected, config.Options{})
}

func expectPrintedMinify(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [minified]", contents, expected, config.Options{
		MinifyWhitespace: true,
	})
}

func expectPrintedMangle(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [mangled]", contents, expected, config.Options{
		MinifySyntax: true,
	})
}

func expectPrintedMangleMinify(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [mangled, minified]", contents, expected, config.Options{
		MinifySyntax:     true,
		MinifyWhitespace: true,
	})
}

func expectPrintedASCII(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [ascii]", contents, expected, config.Options{
		ASCIIOnly: true,
	})
}

func expectPrintedMinifyASCII(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [ascii]", contents, expected, config.Options{
		MinifyWhitespace: true,
		ASCIIOnly:        true,
	})
}

func expectPrintedTarget(t *testing.T, esVersion int, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, contents, expected, config.Options{
		UnsupportedJSFeatures: compat.UnsupportedJSFeatures(map[compat.Engine]compat.Semver{
			compat.ES: {Parts: []int{esVersion}},
		}),
	})
}

func expectPrintedTargetMinify(t *testing.T, esVersion int, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [minified]", contents, expected, config.Options{
		UnsupportedJSFeatures: compat.UnsupportedJSFeatures(map[compat.Engine]compat.Semver{
			compat.ES: {Parts: []int{esVersion}},
		}),
		MinifyWhitespace: true,
	})
}

func expectPrintedTargetMangle(t *testing.T, esVersion int, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [mangled]", contents, expected, config.Options{
		UnsupportedJSFeatures: compat.UnsupportedJSFeatures(map[compat.Engine]compat.Semver{
			compat.ES: {Parts: []int{esVersion}},
		}),
		MinifySyntax: true,
	})
}

func expectPrintedTargetASCII(t *testing.T, esVersion int, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [ascii]", contents, expected, config.Options{
		UnsupportedJSFeatures: compat.UnsupportedJSFeatures(map[compat.Engine]compat.Semver{
			compat.ES: {Parts: []int{esVersion}},
		}),
		ASCIIOnly: true,
	})
}

func expectPrintedJSX(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, contents, expected, config.Options{
		JSX: config.JSXOptions{
			Parse:    true,
			Preserve: true,
		},
	})
}

func expectPrintedJSXASCII(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, contents, expected, config.Options{
		JSX: config.JSXOptions{
			Parse:    true,
			Preserve: true,
		},
		ASCIIOnly: true,
	})
}

func expectPrintedJSXMinify(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [minified]", contents, expected, config.Options{
		JSX: config.JSXOptions{
			Parse:    true,
			Preserve: true,
		},
		MinifyWhitespace: true,
	})
}

func TestNumber(t *testing.T) {
	// Check "1eN"
	expectPrinted(t, "x = 1e-100", "x = 1e-100;\n")
	expectPrinted(t, "x = 1e-4", "x = 1e-4;\n")
	expectPrinted(t, "x = 1e-3", "x = 1e-3;\n")
	expectPrinted(t, "x = 1e-2", "x = 0.01;\n")
	expectPrinted(t, "x = 1e-1", "x = 0.1;\n")
	expectPrinted(t, "x = 1e0", "x = 1;\n")
	expectPrinted(t, "x = 1e1", "x = 10;\n")
	expectPrinted(t, "x = 1e2", "x = 100;\n")
	expectPrinted(t, "x = 1e3", "x = 1e3;\n")
	expectPrinted(t, "x = 1e4", "x = 1e4;\n")
	expectPrinted(t, "x = 1e100", "x = 1e100;\n")
	expectPrintedMinify(t, "x = 1e-100", "x=1e-100;")
	expectPrintedMinify(t, "x = 1e-5", "x=1e-5;")
	expectPrintedMinify(t, "x = 1e-4", "x=1e-4;")
	expectPrintedMinify(t, "x = 1e-3", "x=.001;")
	expectPrintedMinify(t, "x = 1e-2", "x=.01;")
	expectPrintedMinify(t, "x = 1e-1", "x=.1;")
	expectPrintedMinify(t, "x = 1e0", "x=1;")
	expectPrintedMinify(t, "x = 1e1", "x=10;")
	expectPrintedMinify(t, "x = 1e2", "x=100;")
	expectPrintedMinify(t, "x = 1e3", "x=1e3;")
	expectPrintedMinify(t, "x = 1e4", "x=1e4;")
	expectPrintedMinify(t, "x = 1e100", "x=1e100;")

	// Check "12eN"
	expectPrinted(t, "x = 12e-100", "x = 12e-100;\n")
	expectPrinted(t, "x = 12e-5", "x = 12e-5;\n")
	expectPrinted(t, "x = 12e-4", "x = 12e-4;\n")
	expectPrinted(t, "x = 12e-3", "x = 0.012;\n")
	expectPrinted(t, "x = 12e-2", "x = 0.12;\n")
	expectPrinted(t, "x = 12e-1", "x = 1.2;\n")
	expectPrinted(t, "x = 12e0", "x = 12;\n")
	expectPrinted(t, "x = 12e1", "x = 120;\n")
	expectPrinted(t, "x = 12e2", "x = 1200;\n")
	expectPrinted(t, "x = 12e3", "x = 12e3;\n")
	expectPrinted(t, "x = 12e4", "x = 12e4;\n")
	expectPrinted(t, "x = 12e100", "x = 12e100;\n")
	expectPrintedMinify(t, "x = 12e-100", "x=12e-100;")
	expectPrintedMinify(t, "x = 12e-6", "x=12e-6;")
	expectPrintedMinify(t, "x = 12e-5", "x=12e-5;")
	expectPrintedMinify(t, "x = 12e-4", "x=.0012;")
	expectPrintedMinify(t, "x = 12e-3", "x=.012;")
	expectPrintedMinify(t, "x = 12e-2", "x=.12;")
	expectPrintedMinify(t, "x = 12e-1", "x=1.2;")
	expectPrintedMinify(t, "x = 12e0", "x=12;")
	expectPrintedMinify(t, "x = 12e1", "x=120;")
	expectPrintedMinify(t, "x = 12e2", "x=1200;")
	expectPrintedMinify(t, "x = 12e3", "x=12e3;")
	expectPrintedMinify(t, "x = 12e4", "x=12e4;")
	expectPrintedMinify(t, "x = 12e100", "x=12e100;")

	// Check cases for "A.BeX" => "ABeY" simplification
	expectPrinted(t, "x = 123456789", "x = 123456789;\n")
	expectPrinted(t, "x = 1123456789", "x = 1123456789;\n")
	expectPrinted(t, "x = 10123456789", "x = 10123456789;\n")
	expectPrinted(t, "x = 100123456789", "x = 100123456789;\n")
	expectPrinted(t, "x = 1000123456789", "x = 1000123456789;\n")
	expectPrinted(t, "x = 10000123456789", "x = 10000123456789;\n")
	expectPrinted(t, "x = 100000123456789", "x = 100000123456789;\n")
	expectPrinted(t, "x = 1000000123456789", "x = 1000000123456789;\n")
	expectPrinted(t, "x = 10000000123456789", "x = 10000000123456788;\n")
	expectPrinted(t, "x = 100000000123456789", "x = 100000000123456780;\n")
	expectPrinted(t, "x = 1000000000123456789", "x = 1000000000123456800;\n")
	expectPrinted(t, "x = 10000000000123456789", "x = 10000000000123458e3;\n")
	expectPrinted(t, "x = 100000000000123456789", "x = 10000000000012345e4;\n")

	// Check numbers around the ends of various integer ranges. These were
	// crashing in the WebAssembly build due to a bug in the Go runtime.

	// int32
	expectPrinted(t, "x = 0x7fff_ffff", "x = 2147483647;\n")
	expectPrinted(t, "x = 0x8000_0000", "x = 2147483648;\n")
	expectPrinted(t, "x = 0x8000_0001", "x = 2147483649;\n")
	expectPrinted(t, "x = -0x7fff_ffff", "x = -2147483647;\n")
	expectPrinted(t, "x = -0x8000_0000", "x = -2147483648;\n")
	expectPrinted(t, "x = -0x8000_0001", "x = -2147483649;\n")

	// uint32
	expectPrinted(t, "x = 0xffff_ffff", "x = 4294967295;\n")
	expectPrinted(t, "x = 0x1_0000_0000", "x = 4294967296;\n")
	expectPrinted(t, "x = 0x1_0000_0001", "x = 4294967297;\n")
	expectPrinted(t, "x = -0xffff_ffff", "x = -4294967295;\n")
	expectPrinted(t, "x = -0x1_0000_0000", "x = -4294967296;\n")
	expectPrinted(t, "x = -0x1_0000_0001", "x = -4294967297;\n")

	// int64
	expectPrinted(t, "x = 0x7fff_ffff_ffff_fdff", "x = 9223372036854775e3;\n")
	expectPrinted(t, "x = 0x8000_0000_0000_0000", "x = 9223372036854776e3;\n")
	expectPrinted(t, "x = 0x8000_0000_0000_3000", "x = 9223372036854788e3;\n")
	expectPrinted(t, "x = -0x7fff_ffff_ffff_fdff", "x = -9223372036854775e3;\n")
	expectPrinted(t, "x = -0x8000_0000_0000_0000", "x = -9223372036854776e3;\n")
	expectPrinted(t, "x = -0x8000_0000_0000_3000", "x = -9223372036854788e3;\n")

	// uint64
	expectPrinted(t, "x = 0xffff_ffff_ffff_fbff", "x = 1844674407370955e4;\n")
	expectPrinted(t, "x = 0x1_0000_0000_0000_0000", "x = 18446744073709552e3;\n")
	expectPrinted(t, "x = 0x1_0000_0000_0000_1000", "x = 18446744073709556e3;\n")
	expectPrinted(t, "x = -0xffff_ffff_ffff_fbff", "x = -1844674407370955e4;\n")
	expectPrinted(t, "x = -0x1_0000_0000_0000_0000", "x = -18446744073709552e3;\n")
	expectPrinted(t, "x = -0x1_0000_0000_0000_1000", "x = -18446744073709556e3;\n")

	// Check the hex vs. decimal decision boundary when minifying
	expectPrinted(t, "x = 999999999999", "x = 999999999999;\n")
	expectPrinted(t, "x = 1000000000001", "x = 1000000000001;\n")
	expectPrinted(t, "x = 0x0FFF_FFFF_FFFF_FF80", "x = 1152921504606846800;\n")
	expectPrinted(t, "x = 0x1000_0000_0000_0000", "x = 1152921504606847e3;\n")
	expectPrinted(t, "x = 0xFFFF_FFFF_FFFF_F000", "x = 18446744073709548e3;\n")
	expectPrinted(t, "x = 0xFFFF_FFFF_FFFF_F800", "x = 1844674407370955e4;\n")
	expectPrinted(t, "x = 0xFFFF_FFFF_FFFF_FFFF", "x = 18446744073709552e3;\n")
	expectPrintedMinify(t, "x = 999999999999", "x=999999999999;")
	expectPrintedMinify(t, "x = 1000000000001", "x=0xe8d4a51001;")
	expectPrintedMinify(t, "x = 0x0FFF_FFFF_FFFF_FF80", "x=0xfffffffffffff80;")
	expectPrintedMinify(t, "x = 0x1000_0000_0000_0000", "x=1152921504606847e3;")
	expectPrintedMinify(t, "x = 0xFFFF_FFFF_FFFF_F000", "x=0xfffffffffffff000;")
	expectPrintedMinify(t, "x = 0xFFFF_FFFF_FFFF_F800", "x=1844674407370955e4;")
	expectPrintedMinify(t, "x = 0xFFFF_FFFF_FFFF_FFFF", "x=18446744073709552e3;")

	// Check printing a space in between a number and a subsequent "."
	expectPrintedMinify(t, "x = 0.0001 .y", "x=1e-4.y;")
	expectPrintedMinify(t, "x = 0.001 .y", "x=.001.y;")
	expectPrintedMinify(t, "x = 0.01 .y", "x=.01.y;")
	expectPrintedMinify(t, "x = 0.1 .y", "x=.1.y;")
	expectPrintedMinify(t, "x = 0 .y", "x=0 .y;")
	expectPrintedMinify(t, "x = 10 .y", "x=10 .y;")
	expectPrintedMinify(t, "x = 100 .y", "x=100 .y;")
	expectPrintedMinify(t, "x = 1000 .y", "x=1e3.y;")
	expectPrintedMinify(t, "x = 12345 .y", "x=12345 .y;")
	expectPrintedMinify(t, "x = 0xFFFF_0000_FFFF_0000 .y", "x=0xffff0000ffff0000.y;")
}

func TestArray(t *testing.T) {
	expectPrinted(t, "[]", "[];\n")
	expectPrinted(t, "[,]", "[,];\n")
	expectPrinted(t, "[,,]", "[, ,];\n")
}

func TestSplat(t *testing.T) {
	expectPrinted(t, "[...(a, b)]", "[...(a, b)];\n")
	expectPrinted(t, "x(...(a, b))", "x(...(a, b));\n")
	expectPrinted(t, "({...(a, b)})", "({ ...(a, b) });\n")
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

	// Test preservation of Webpack-specific comments
	expectPrinted(t, "new Worker(// webpackFoo: 1\n // webpackBar: 2\n 'path');", "new Worker(\n  // webpackFoo: 1\n  // webpackBar: 2\n  \"path\"\n);\n")
	expectPrinted(t, "new Worker(/* webpackFoo: 1 */ /* webpackBar: 2 */ 'path');", "new Worker(\n  /* webpackFoo: 1 */\n  /* webpackBar: 2 */\n  \"path\"\n);\n")
	expectPrinted(t, "new Worker(\n    /* multi\n     * line\n     * webpackBar: */ 'path');", "new Worker(\n  /* multi\n   * line\n   * webpackBar: */\n  \"path\"\n);\n")
	expectPrinted(t, "new Worker(/* webpackFoo: 1 */ 'path' /* webpackBar:2 */);", "new Worker(\n  /* webpackFoo: 1 */\n  \"path\"\n  /* webpackBar:2 */\n);\n")
	expectPrinted(t, "new Worker(/* webpackFoo: 1 */ 'path' /* webpackBar:2 */ ,);", "new Worker(\n  /* webpackFoo: 1 */\n  \"path\"\n);\n") // Not currently handled
	expectPrinted(t, "new Worker(/* webpackFoo: 1 */ 'path', /* webpackBar:2 */ );", "new Worker(\n  /* webpackFoo: 1 */\n  \"path\"\n  /* webpackBar:2 */\n);\n")
	expectPrinted(t, "new Worker(new URL('path', /* webpackFoo: these can go anywhere */ import.meta.url))",
		"new Worker(new URL(\n  \"path\",\n  /* webpackFoo: these can go anywhere */\n  import.meta.url\n));\n")
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
	expectPrintedMangle(t, "(1 ? eval : 2)(x)", "(0, eval)(x);\n")
	expectPrintedMangle(t, "(1 ? eval : 2)?.(x)", "eval?.(x);\n")

	expectPrintedMinify(t, "eval?.(x)", "eval?.(x);")
	expectPrintedMinify(t, "eval(x,y)", "eval(x,y);")
	expectPrintedMinify(t, "eval?.(x,y)", "eval?.(x,y);")
	expectPrintedMinify(t, "(1, eval)(x)", "(1,eval)(x);")
	expectPrintedMinify(t, "(1, eval)?.(x)", "(1,eval)?.(x);")
	expectPrintedMangleMinify(t, "(1 ? eval : 2)(x)", "(0,eval)(x);")
	expectPrintedMangleMinify(t, "(1 ? eval : 2)?.(x)", "eval?.(x);")
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
	expectPrinted(t, "let x = '\x1B'", "let x = \"\\x1B\";\n")
	expectPrinted(t, "let x = '\\x1B'", "let x = \"\\x1B\";\n")
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
	expectPrinted(t, "let x = `\\0`", "let x = `\\0`;\n")
	expectPrinted(t, "let x = `\\x01`", "let x = `\x01`;\n")
	expectPrinted(t, "let x = `\\0${0}`", "let x = `\\0${0}`;\n")
	expectPrinted(t, "let x = `\\x01${0}`", "let x = `\x01${0}`;\n")
	expectPrinted(t, "let x = `${0}\\0`", "let x = `${0}\\0`;\n")
	expectPrinted(t, "let x = `${0}\\x01`", "let x = `${0}\x01`;\n")
	expectPrinted(t, "let x = `${0}\\0${1}`", "let x = `${0}\\0${1}`;\n")
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
	expectPrinted(t, "let x = {'(':')'}", "let x = { \"(\": \")\" };\n")
	expectPrinted(t, "({})", "({});\n")
	expectPrinted(t, "({}.x)", "({}).x;\n")
	expectPrinted(t, "({} = {})", "({} = {});\n")
	expectPrinted(t, "(x, {} = {})", "x, {} = {};\n")
	expectPrinted(t, "let x = () => ({})", "let x = () => ({});\n")
	expectPrinted(t, "let x = () => ({}.x)", "let x = () => ({}).x;\n")
	expectPrinted(t, "let x = () => ({} = {})", "let x = () => ({} = {});\n")
	expectPrinted(t, "let x = () => (x, {} = {})", "let x = () => (x, {} = {});\n")

	// "{ __proto__: __proto__ }" must not become "{ __proto__ }"
	expectPrinted(t, "function foo(__proto__) { return { __proto__: __proto__ } }", "function foo(__proto__) {\n  return { __proto__: __proto__ };\n}\n")
	expectPrinted(t, "function foo(__proto__) { return { '__proto__': __proto__ } }", "function foo(__proto__) {\n  return { \"__proto__\": __proto__ };\n}\n")
	expectPrinted(t, "function foo(__proto__) { return { ['__proto__']: __proto__ } }", "function foo(__proto__) {\n  return { [\"__proto__\"]: __proto__ };\n}\n")
	expectPrinted(t, "import { __proto__ } from 'foo'; let foo = () => ({ __proto__: __proto__ })", "import { __proto__ } from \"foo\";\nlet foo = () => ({ __proto__: __proto__ });\n")
	expectPrinted(t, "import { __proto__ } from 'foo'; let foo = () => ({ '__proto__': __proto__ })", "import { __proto__ } from \"foo\";\nlet foo = () => ({ \"__proto__\": __proto__ });\n")
	expectPrinted(t, "import { __proto__ } from 'foo'; let foo = () => ({ ['__proto__']: __proto__ })", "import { __proto__ } from \"foo\";\nlet foo = () => ({ [\"__proto__\"]: __proto__ });\n")

	// Don't use ES6+ features (such as a shorthand or computed property name) in ES5
	expectPrintedTarget(t, 5, "function foo(__proto__) { return { __proto__ } }", "function foo(__proto__) {\n  return { __proto__: __proto__ };\n}\n")
}

func TestSwitch(t *testing.T) {
	// Ideally comments on case clauses would be preserved
	expectPrinted(t, "switch (x) { /* 1 */ case 1: /* 2 */ case 2: /* default */ default: break }",
		"switch (x) {\n  /* 1 */\n  case 1:\n  /* 2 */\n  case 2:\n  /* default */\n  default:\n    break;\n}\n")
}

func TestFor(t *testing.T) {
	// Make sure "in" expressions are forbidden in the right places
	expectPrinted(t, "for ((a in b);;);", "for ((a in b); ; ) ;\n")
	expectPrinted(t, "for (a ? b : (c in d);;);", "for (a ? b : (c in d); ; ) ;\n")
	expectPrinted(t, "for ((a ? b : c in d).foo;;);", "for ((a ? b : c in d).foo; ; ) ;\n")
	expectPrinted(t, "for (var x = (a in b);;);", "for (var x = (a in b); ; ) ;\n")
	expectPrinted(t, "for (x = (a in b);;);", "for (x = (a in b); ; ) ;\n")
	expectPrinted(t, "for (x == (a in b);;);", "for (x == (a in b); ; ) ;\n")
	expectPrinted(t, "for (1 * (x == a in b);;);", "for (1 * (x == a in b); ; ) ;\n")
	expectPrinted(t, "for (a ? b : x = (c in d);;);", "for (a ? b : x = (c in d); ; ) ;\n")
	expectPrinted(t, "for (var x = y = (a in b);;);", "for (var x = y = (a in b); ; ) ;\n")
	expectPrinted(t, "for ([a in b];;);", "for ([a in b]; ; ) ;\n")
	expectPrinted(t, "for (x(a in b);;);", "for (x(a in b); ; ) ;\n")
	expectPrinted(t, "for (x[a in b];;);", "for (x[a in b]; ; ) ;\n")
	expectPrinted(t, "for (x?.[a in b];;);", "for (x?.[a in b]; ; ) ;\n")
	expectPrinted(t, "for ((x => a in b);;);", "for ((x) => (a in b); ; ) ;\n")

	// Make sure for-of loops with commas are wrapped in parentheses
	expectPrinted(t, "for (let a in b, c);", "for (let a in b, c) ;\n")
	expectPrinted(t, "for (let a of (b, c));", "for (let a of (b, c)) ;\n")
}

func TestFunction(t *testing.T) {
	expectPrinted(t,
		"function foo(a = (b, c), ...d) {}",
		"function foo(a = (b, c), ...d) {\n}\n")
	expectPrinted(t,
		"function foo({[1 + 2]: a = 3} = {[1 + 2]: 3}) {}",
		"function foo({ [1 + 2]: a = 3 } = { [1 + 2]: 3 }) {\n}\n")
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

func TestCommentsAndParentheses(t *testing.T) {
	expectPrinted(t, "(/* foo */ { x() { foo() } }.x());", "/* foo */\n({ x() {\n  foo();\n} }).x();\n")
	expectPrinted(t, "(/* foo */ function f() { foo(f) }());", "/* foo */\n(function f() {\n  foo(f);\n})();\n")
	expectPrinted(t, "(/* foo */ class x { static y() { foo(x) } }.y());", "/* foo */\n(class x {\n  static y() {\n    foo(x);\n  }\n}).y();\n")
	expectPrinted(t, "(/* @__PURE__ */ (() => foo())());", "/* @__PURE__ */ (() => foo())();\n")
	expectPrinted(t, "export default (/* foo */ function f() {});", "export default (\n  /* foo */\n  function f() {\n  }\n);\n")
	expectPrinted(t, "export default (/* foo */ class x {});", "export default (\n  /* foo */\n  class x {\n  }\n);\n")
	expectPrinted(t, "x = () => (/* foo */ {});", "x = () => (\n  /* foo */\n  {}\n);\n")
	expectPrinted(t, "for ((/* foo */ let).x of y) ;", "for (\n  /* foo */\n  (let).x of y\n) ;\n")
	expectPrinted(t, "for (/* foo */ (let).x of y) ;", "for (\n  /* foo */\n  (let).x of y\n) ;\n")
	expectPrinted(t, "function *x() { yield (/* foo */ y) }", "function* x() {\n  yield (\n    /* foo */\n    y\n  );\n}\n")
}

func TestPureComment(t *testing.T) {
	expectPrinted(t,
		"(function() { foo() })",
		"(function() {\n  foo();\n});\n")
	expectPrinted(t,
		"(function() { foo() })()",
		"(function() {\n  foo();\n})();\n")
	expectPrinted(t,
		"/*@__PURE__*/(function() { foo() })()",
		"/* @__PURE__ */ (function() {\n  foo();\n})();\n")

	expectPrinted(t,
		"new (function() {})",
		"new function() {\n}();\n")
	expectPrinted(t,
		"new (function() {})()",
		"new function() {\n}();\n")
	expectPrinted(t,
		"/*@__PURE__*/new (function() {})()",
		"/* @__PURE__ */ new function() {\n}();\n")

	expectPrinted(t,
		"export default (function() { foo() })",
		"export default (function() {\n  foo();\n});\n")
	expectPrinted(t,
		"export default (function() { foo() })()",
		"export default (function() {\n  foo();\n})();\n")
	expectPrinted(t,
		"export default /*@__PURE__*/(function() { foo() })()",
		"export default /* @__PURE__ */ (function() {\n  foo();\n})();\n")
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
		"({ [1 + 2]: a = 3 } = { [1 + 2]: 3 }) => {\n};\n")
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
		"({ a = b, c = d }) => {\n};\n")
	expectPrinted(t,
		"([{a = b, c = d} = {}] = []) => {}",
		"([{ a = b, c = d } = {}] = []) => {\n};\n")
	expectPrinted(t,
		"({a: [b = c] = []} = {}) => {}",
		"({ a: [b = c] = [] } = {}) => {\n};\n")

	// These are not arrow functions but initially look like one
	expectPrinted(t, "(a = b, c)", "a = b, c;\n")
	expectPrinted(t, "([...a = b])", "[...a = b];\n")
	expectPrinted(t, "([...a, ...b])", "[...a, ...b];\n")
	expectPrinted(t, "({a: b, c() {}})", "({ a: b, c() {\n} });\n")
	expectPrinted(t, "({a: b, get c() {}})", "({ a: b, get c() {\n} });\n")
	expectPrinted(t, "({a: b, set c(x) {}})", "({ a: b, set c(x) {\n} });\n")
}

func TestClass(t *testing.T) {
	expectPrinted(t, "class Foo extends (a, b) {}", "class Foo extends (a, b) {\n}\n")
	expectPrinted(t, "class Foo { get foo() {} }", "class Foo {\n  get foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { set foo(x) {} }", "class Foo {\n  set foo(x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { static foo() {} }", "class Foo {\n  static foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static get foo() {} }", "class Foo {\n  static get foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static set foo(x) {} }", "class Foo {\n  static set foo(x) {\n  }\n}\n")
}

func TestAutoAccessors(t *testing.T) {
	expectPrinted(t, "class Foo { accessor x; static accessor y }", "class Foo {\n  accessor x;\n  static accessor y;\n}\n")
	expectPrinted(t, "class Foo { accessor [x]; static accessor [y] }", "class Foo {\n  accessor [x];\n  static accessor [y];\n}\n")
	expectPrintedMinify(t, "class Foo { accessor x; static accessor y }", "class Foo{accessor x;static accessor y}")
	expectPrintedMinify(t, "class Foo { accessor [x]; static accessor [y] }", "class Foo{accessor[x];static accessor[y]}")
}

func TestPrivateIdentifiers(t *testing.T) {
	expectPrinted(t, "class Foo { #foo; foo() { return #foo in this } }", "class Foo {\n  #foo;\n  foo() {\n    return #foo in this;\n  }\n}\n")
	expectPrintedMinify(t, "class Foo { #foo; foo() { return #foo in this } }", "class Foo{#foo;foo(){return#foo in this}}")
}

func TestDecorators(t *testing.T) {
	example := "class Foo {\n@w\nw; @x x; @a1\n@b1@b2\n@c1@c2@c3\ny = @y1 @y2 class {}; @a1\n@b1@b2\n@c1@c2@c3 z =\n@z1\n@z2\nclass {}}"
	expectPrinted(t, example, "class Foo {\n  @w\n  w;\n  @x x;\n  @a1\n  @b1 @b2\n  @c1 @c2 @c3\n  "+
		"y = @y1 @y2 class {\n  };\n  @a1\n  @b1 @b2\n  @c1 @c2 @c3 z = @z1 @z2 class {\n  };\n}\n")
	expectPrintedMinify(t, example, "class Foo{@w w;@x x;@a1@b1@b2@c1@c2@c3 y=@y1@y2 class{};@a1@b1@b2@c1@c2@c3 z=@z1@z2 class{}}")
}

func TestImport(t *testing.T) {
	expectPrinted(t, "import('path');", "import(\"path\");\n") // The semicolon must not be a separate statement

	// Test preservation of Webpack-specific comments
	expectPrinted(t, "import(// webpackFoo: 1\n // webpackBar: 2\n 'path');", "import(\n  // webpackFoo: 1\n  // webpackBar: 2\n  \"path\"\n);\n")
	expectPrinted(t, "import(// webpackFoo: 1\n // webpackBar: 2\n 'path', {type: 'module'});", "import(\n  // webpackFoo: 1\n  // webpackBar: 2\n  \"path\",\n  { type: \"module\" }\n);\n")
	expectPrinted(t, "import(/* webpackFoo: 1 */ /* webpackBar: 2 */ 'path');", "import(\n  /* webpackFoo: 1 */\n  /* webpackBar: 2 */\n  \"path\"\n);\n")
	expectPrinted(t, "import(/* webpackFoo: 1 */ /* webpackBar: 2 */ 'path', {type: 'module'});", "import(\n  /* webpackFoo: 1 */\n  /* webpackBar: 2 */\n  \"path\",\n  { type: \"module\" }\n);\n")
	expectPrinted(t, "import(\n    /* multi\n     * line\n     * webpackBar: */ 'path');", "import(\n  /* multi\n   * line\n   * webpackBar: */\n  \"path\"\n);\n")
	expectPrinted(t, "import(/* webpackFoo: 1 */ 'path' /* webpackBar:2 */);", "import(\n  /* webpackFoo: 1 */\n  \"path\"\n  /* webpackBar:2 */\n);\n")
	expectPrinted(t, "import(/* webpackFoo: 1 */ 'path' /* webpackBar:2 */ ,);", "import(\n  /* webpackFoo: 1 */\n  \"path\"\n);\n") // Not currently handled
	expectPrinted(t, "import(/* webpackFoo: 1 */ 'path', /* webpackBar:2 */ );", "import(\n  /* webpackFoo: 1 */\n  \"path\"\n  /* webpackBar:2 */\n);\n")
	expectPrinted(t, "import(/* webpackFoo: 1 */ 'path', { type: 'module' } /* webpackBar:2 */ );", "import(\n  /* webpackFoo: 1 */\n  \"path\",\n  { type: \"module\" }\n  /* webpackBar:2 */\n);\n")
	expectPrinted(t, "import(new URL('path', /* webpackFoo: these can go anywhere */ import.meta.url))",
		"import(new URL(\n  \"path\",\n  /* webpackFoo: these can go anywhere */\n  import.meta.url\n));\n")

	// See: https://github.com/tc39/proposal-defer-import-eval
	expectPrintedMinify(t, "import defer * as foo from 'bar'", "import defer*as foo from\"bar\";")

	// See: https://github.com/tc39/proposal-source-phase-imports
	expectPrintedMinify(t, "import source foo from 'bar'", "import source foo from\"bar\";")
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
	expectPrintedMinify(t, "π in function(){}", "π in function(){};")
	expectPrintedMinify(t, "π instanceof function(){}", "π instanceof function(){};")

	expectPrintedMinify(t, "()=>({})", "()=>({});")
	expectPrintedMinify(t, "()=>({}[1])", "()=>({})[1];")
	expectPrintedMinify(t, "()=>({}+0)", "()=>\"[object Object]0\";")
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
	expectPrintedMinify(t, "import {a, ' ' as c} from 'path'", "import{a,\" \"as c}from\"path\";")

	expectPrintedMinify(t, "export * as ns from 'path'", "export*as ns from\"path\";")
	expectPrintedMinify(t, "export * as ' ' from 'path'", "export*as\" \"from\"path\";")
	expectPrintedMinify(t, "export {a, b as c} from 'path'", "export{a,b as c}from\"path\";")
	expectPrintedMinify(t, "export {' ', '-' as ';'} from 'path'", "export{\" \",\"-\"as\";\"}from\"path\";")
	expectPrintedMinify(t, "let a, b; export {a, b as c}", "let a,b;export{a,b as c};")
	expectPrintedMinify(t, "let a, b; export {a, b as ' '}", "let a,b;export{a,b as\" \"};")

	// Print some strings using template literals when minifying
	expectPrinted(t, "x = '\\n'", "x = \"\\n\";\n")
	expectPrintedMangle(t, "x = '\\n'", "x = `\n`;\n")
	expectPrintedMangle(t, "x = {'\\n': 0}", "x = { \"\\n\": 0 };\n")
	expectPrintedMangle(t, "x = class{'\\n' = 0}", "x = class {\n  \"\\n\" = 0;\n};\n")
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

	expectPrintedTarget(t, 5, "foo({a, b})", "foo({ a: a, b: b });\n")
	expectPrintedTarget(t, 2015, "foo({a, b})", "foo({ a, b });\n")

	expectPrintedTarget(t, 5, "x => x", "(function(x) {\n  return x;\n});\n")
	expectPrintedTarget(t, 2015, "x => x", "(x) => x;\n")

	expectPrintedTarget(t, 5, "() => {}", "(function() {\n});\n")
	expectPrintedTarget(t, 2015, "() => {}", "() => {\n};\n")

	expectPrintedTargetMinify(t, 5, "x => x", "(function(x){return x});")
	expectPrintedTargetMinify(t, 2015, "x => x", "x=>x;")

	expectPrintedTargetMinify(t, 5, "() => {}", "(function(){});")
	expectPrintedTargetMinify(t, 2015, "() => {}", "()=>{};")
}

func TestASCIIOnly(t *testing.T) {
	expectPrinted(t, "let π = 'π'", "let π = \"π\";\n")
	expectPrinted(t, "let π_ = 'π'", "let π_ = \"π\";\n")
	expectPrinted(t, "let _π = 'π'", "let _π = \"π\";\n")
	expectPrintedASCII(t, "let π = 'π'", "let \\u03C0 = \"\\u03C0\";\n")
	expectPrintedASCII(t, "let π_ = 'π'", "let \\u03C0_ = \"\\u03C0\";\n")
	expectPrintedASCII(t, "let _π = 'π'", "let _\\u03C0 = \"\\u03C0\";\n")

	expectPrinted(t, "let 貓 = '🐈'", "let 貓 = \"🐈\";\n")
	expectPrinted(t, "let 貓abc = '🐈'", "let 貓abc = \"🐈\";\n")
	expectPrinted(t, "let abc貓 = '🐈'", "let abc貓 = \"🐈\";\n")
	expectPrintedASCII(t, "let 貓 = '🐈'", "let \\u8C93 = \"\\u{1F408}\";\n")
	expectPrintedASCII(t, "let 貓abc = '🐈'", "let \\u8C93abc = \"\\u{1F408}\";\n")
	expectPrintedASCII(t, "let abc貓 = '🐈'", "let abc\\u8C93 = \"\\u{1F408}\";\n")

	// Test a character outside the BMP
	expectPrinted(t, "var 𐀀", "var 𐀀;\n")
	expectPrinted(t, "var \\u{10000}", "var 𐀀;\n")
	expectPrintedASCII(t, "var 𐀀", "var \\u{10000};\n")
	expectPrintedASCII(t, "var \\u{10000}", "var \\u{10000};\n")
	expectPrintedTargetASCII(t, 2015, "'𐀀'", "\"\\u{10000}\";\n")
	expectPrintedTargetASCII(t, 5, "'𐀀'", "\"\\uD800\\uDC00\";\n")
	expectPrintedTargetASCII(t, 2015, "x.𐀀", "x[\"\\u{10000}\"];\n")
	expectPrintedTargetASCII(t, 5, "x.𐀀", "x[\"\\uD800\\uDC00\"];\n")

	// Escapes should use consistent case
	expectPrintedASCII(t, "var \\u{100a} = {\\u100A: '\\u100A'}", "var \\u100A = { \\u100A: \"\\u100A\" };\n")
	expectPrintedASCII(t, "var \\u{1000a} = {\\u{1000A}: '\\u{1000A}'}", "var \\u{1000A} = { \"\\u{1000A}\": \"\\u{1000A}\" };\n")

	// These characters should always be escaped
	expectPrinted(t, "let x = '\u2028'", "let x = \"\\u2028\";\n")
	expectPrinted(t, "let x = '\u2029'", "let x = \"\\u2029\";\n")
	expectPrinted(t, "let x = '\uFEFF'", "let x = \"\\uFEFF\";\n")

	// There should still be a space before "extends"
	expectPrintedASCII(t, "class 𐀀 extends π {}", "class \\u{10000} extends \\u03C0 {\n}\n")
	expectPrintedASCII(t, "(class 𐀀 extends π {})", "(class \\u{10000} extends \\u03C0 {\n});\n")
	expectPrintedMinifyASCII(t, "class 𐀀 extends π {}", "class \\u{10000} extends \\u03C0{}")
	expectPrintedMinifyASCII(t, "(class 𐀀 extends π {})", "(class \\u{10000} extends \\u03C0{});")
}

func TestJSX(t *testing.T) {
	expectPrintedJSX(t, "<a/>", "<a />;\n")
	expectPrintedJSX(t, "<A/>", "<A />;\n")
	expectPrintedJSX(t, "<a.b/>", "<a.b />;\n")
	expectPrintedJSX(t, "<A.B/>", "<A.B />;\n")
	expectPrintedJSX(t, "<a-b/>", "<a-b />;\n")
	expectPrintedJSX(t, "<a:b/>", "<a:b />;\n")
	expectPrintedJSX(t, "<a></a>", "<a />;\n")
	expectPrintedJSX(t, "<a b></a>", "<a b />;\n")

	expectPrintedJSX(t, "<a b={true}></a>", "<a b={true} />;\n")
	expectPrintedJSX(t, "<a b='x'></a>", "<a b='x' />;\n")
	expectPrintedJSX(t, "<a b=\"x\"></a>", "<a b=\"x\" />;\n")
	expectPrintedJSX(t, "<a b={'x'}></a>", "<a b={\"x\"} />;\n")
	expectPrintedJSX(t, "<a b={`'`}></a>", "<a b={`'`} />;\n")
	expectPrintedJSX(t, "<a b={`\"`}></a>", "<a b={`\"`} />;\n")
	expectPrintedJSX(t, "<a b={`'\"`}></a>", "<a b={`'\"`} />;\n")
	expectPrintedJSX(t, "<a b=\"&quot;\"></a>", "<a b=\"&quot;\" />;\n")
	expectPrintedJSX(t, "<a b=\"&amp;\"></a>", "<a b=\"&amp;\" />;\n")

	expectPrintedJSX(t, "<a>x</a>", "<a>x</a>;\n")
	expectPrintedJSX(t, "<a>x\ny</a>", "<a>x\ny</a>;\n")
	expectPrintedJSX(t, "<a>{'x'}{'y'}</a>", "<a>{\"x\"}{\"y\"}</a>;\n")
	expectPrintedJSX(t, "<a> x</a>", "<a> x</a>;\n")
	expectPrintedJSX(t, "<a>x </a>", "<a>x </a>;\n")
	expectPrintedJSX(t, "<a>&#10;</a>", "<a>&#10;</a>;\n")
	expectPrintedJSX(t, "<a>&amp;</a>", "<a>&amp;</a>;\n")
	expectPrintedJSX(t, "<a>&lt;</a>", "<a>&lt;</a>;\n")
	expectPrintedJSX(t, "<a>&gt;</a>", "<a>&gt;</a>;\n")
	expectPrintedJSX(t, "<a>&#123;</a>", "<a>&#123;</a>;\n")
	expectPrintedJSX(t, "<a>&#125;</a>", "<a>&#125;</a>;\n")

	expectPrintedJSX(t, "<a><x/></a>", "<a><x /></a>;\n")
	expectPrintedJSX(t, "<a><x/><y/></a>", "<a><x /><y /></a>;\n")
	expectPrintedJSX(t, "<a>b<c/>d</a>", "<a>b<c />d</a>;\n")

	expectPrintedJSX(t, "<></>", "<></>;\n")
	expectPrintedJSX(t, "<>x<y/>z</>", "<>x<y />z</>;\n")

	// JSX elements as JSX attribute values
	expectPrintedJSX(t, "<a b=<c/>/>", "<a b=<c /> />;\n")
	expectPrintedJSX(t, "<a b=<>c</>/>", "<a b=<>c</> />;\n")
	expectPrintedJSX(t, "<a b=<>{c}</>/>", "<a b=<>{c}</> />;\n")
	expectPrintedJSX(t, "<a b={<c/>}/>", "<a b={<c />} />;\n")
	expectPrintedJSX(t, "<a b={<>c</>}/>", "<a b={<>c</>} />;\n")
	expectPrintedJSX(t, "<a b={<>{c}</>}/>", "<a b={<>{c}</>} />;\n")

	// These can't be escaped because JSX lacks a syntax for escapes
	expectPrintedJSXASCII(t, "<π/>", "<π />;\n")
	expectPrintedJSXASCII(t, "<π.𐀀/>", "<π.𐀀 />;\n")
	expectPrintedJSXASCII(t, "<𐀀.π/>", "<𐀀.π />;\n")
	expectPrintedJSXASCII(t, "<π>x</π>", "<π>x</π>;\n")
	expectPrintedJSXASCII(t, "<𐀀>x</𐀀>", "<𐀀>x</𐀀>;\n")
	expectPrintedJSXASCII(t, "<a π/>", "<a π />;\n")
	expectPrintedJSXASCII(t, "<a 𐀀/>", "<a 𐀀 />;\n")

	// JSX text is deliberately not printed as ASCII when JSX preservation is
	// enabled. This is because:
	//
	// a) The JSX specification doesn't say how JSX text is supposed to be interpreted
	// b) Enabling JSX preservation means that JSX will be transformed again anyway
	// c) People do very weird/custom things with JSX that "preserve" shouldn't break
	//
	// See also: https://github.com/evanw/esbuild/issues/3605
	expectPrintedJSXASCII(t, "<a b='π'/>", "<a b='π' />;\n")
	expectPrintedJSXASCII(t, "<a b='𐀀'/>", "<a b='𐀀' />;\n")
	expectPrintedJSXASCII(t, "<a>π</a>", "<a>π</a>;\n")
	expectPrintedJSXASCII(t, "<a>𐀀</a>", "<a>𐀀</a>;\n")

	expectPrintedJSXMinify(t, "<a b c={x,y} d='true'/>", "<a b c={(x,y)}d='true'/>;")
	expectPrintedJSXMinify(t, "<a><b/><c/></a>", "<a><b/><c/></a>;")
	expectPrintedJSXMinify(t, "<a> x <b/> y </a>", "<a> x <b/> y </a>;")
	expectPrintedJSXMinify(t, "<a>{' x '}{'<b/>'}{' y '}</a>", "<a>{\" x \"}{\"<b/>\"}{\" y \"}</a>;")
}

func TestJSXSingleLine(t *testing.T) {
	expectPrintedJSX(t, "<x/>", "<x />;\n")
	expectPrintedJSX(t, "<x y/>", "<x y />;\n")
	expectPrintedJSX(t, "<x\n/>", "<x />;\n")
	expectPrintedJSX(t, "<x\ny/>", "<x\n  y\n/>;\n")
	expectPrintedJSX(t, "<x y\n/>", "<x\n  y\n/>;\n")
	expectPrintedJSX(t, "<x\n{...y}/>", "<x\n  {...y}\n/>;\n")

	expectPrintedJSXMinify(t, "<x/>", "<x/>;")
	expectPrintedJSXMinify(t, "<x y/>", "<x y/>;")
	expectPrintedJSXMinify(t, "<x\n/>", "<x/>;")
	expectPrintedJSXMinify(t, "<x\ny/>", "<x y/>;")
	expectPrintedJSXMinify(t, "<x y\n/>", "<x y/>;")
	expectPrintedJSXMinify(t, "<x\n{...y}/>", "<x{...y}/>;")
}

func TestAvoidSlashScript(t *testing.T) {
	// Positive cases
	expectPrinted(t, "x = '</script'", "x = \"<\\/script\";\n")
	expectPrinted(t, "x = `</script`", "x = `<\\/script`;\n")
	expectPrinted(t, "x = `</SCRIPT`", "x = `<\\/SCRIPT`;\n")
	expectPrinted(t, "x = `</ScRiPt`", "x = `<\\/ScRiPt`;\n")
	expectPrinted(t, "x = `</script${y}`", "x = `<\\/script${y}`;\n")
	expectPrinted(t, "x = `${y}</script`", "x = `${y}<\\/script`;\n")
	expectPrintedMinify(t, "x = 1 < /script/.exec(y).length", "x=1< /script/.exec(y).length;")
	expectPrintedMinify(t, "x = 1 < /SCRIPT/.exec(y).length", "x=1< /SCRIPT/.exec(y).length;")
	expectPrintedMinify(t, "x = 1 < /ScRiPt/.exec(y).length", "x=1< /ScRiPt/.exec(y).length;")
	expectPrintedMinify(t, "x = 1 << /script/.exec(y).length", "x=1<< /script/.exec(y).length;")
	expectPrinted(t, "//! </script\n//! >/script\n//! /script", "//! <\\/script\n//! >/script\n//! /script\n")
	expectPrinted(t, "//! </SCRIPT\n//! >/SCRIPT\n//! /SCRIPT", "//! <\\/SCRIPT\n//! >/SCRIPT\n//! /SCRIPT\n")
	expectPrinted(t, "//! </ScRiPt\n//! >/ScRiPt\n//! /ScRiPt", "//! <\\/ScRiPt\n//! >/ScRiPt\n//! /ScRiPt\n")
	expectPrinted(t, "/*! </script \n </script */", "/*! <\\/script \n <\\/script */\n")
	expectPrinted(t, "/*! </SCRIPT \n </SCRIPT */", "/*! <\\/SCRIPT \n <\\/SCRIPT */\n")
	expectPrinted(t, "/*! </ScRiPt \n </ScRiPt */", "/*! <\\/ScRiPt \n <\\/ScRiPt */\n")
	expectPrinted(t, "String.raw`</script`",
		"import { __template } from \"<runtime>\";\nvar _a;\nString.raw(_a || (_a = __template([\"<\\/script\"])));\n")
	expectPrinted(t, "String.raw`</script${a}`",
		"import { __template } from \"<runtime>\";\nvar _a;\nString.raw(_a || (_a = __template([\"<\\/script\", \"\"])), a);\n")
	expectPrinted(t, "String.raw`${a}</script`",
		"import { __template } from \"<runtime>\";\nvar _a;\nString.raw(_a || (_a = __template([\"\", \"<\\/script\"])), a);\n")
	expectPrinted(t, "String.raw`</SCRIPT`",
		"import { __template } from \"<runtime>\";\nvar _a;\nString.raw(_a || (_a = __template([\"<\\/SCRIPT\"])));\n")
	expectPrinted(t, "String.raw`</ScRiPt`",
		"import { __template } from \"<runtime>\";\nvar _a;\nString.raw(_a || (_a = __template([\"<\\/ScRiPt\"])));\n")

	// Negative cases
	expectPrinted(t, "x = '</'", "x = \"</\";\n")
	expectPrinted(t, "x = '</ script'", "x = \"</ script\";\n")
	expectPrinted(t, "x = '< /script'", "x = \"< /script\";\n")
	expectPrinted(t, "x = '/script>'", "x = \"/script>\";\n")
	expectPrinted(t, "x = '<script>'", "x = \"<script>\";\n")
	expectPrintedMinify(t, "x = 1 < / script/.exec(y).length", "x=1</ script/.exec(y).length;")
	expectPrintedMinify(t, "x = 1 << / script/.exec(y).length", "x=1<</ script/.exec(y).length;")
}

func TestInfinity(t *testing.T) {
	expectPrinted(t, "x = Infinity", "x = Infinity;\n")
	expectPrinted(t, "x = -Infinity", "x = -Infinity;\n")
	expectPrinted(t, "x = (Infinity).toString", "x = Infinity.toString;\n")
	expectPrinted(t, "x = (-Infinity).toString", "x = (-Infinity).toString;\n")
	expectPrinted(t, "x = (Infinity) ** 2", "x = Infinity ** 2;\n")
	expectPrinted(t, "x = (-Infinity) ** 2", "x = (-Infinity) ** 2;\n")
	expectPrinted(t, "x = ~Infinity", "x = ~Infinity;\n")
	expectPrinted(t, "x = ~-Infinity", "x = ~-Infinity;\n")
	expectPrinted(t, "x = Infinity * y", "x = Infinity * y;\n")
	expectPrinted(t, "x = Infinity / y", "x = Infinity / y;\n")
	expectPrinted(t, "x = y * Infinity", "x = y * Infinity;\n")
	expectPrinted(t, "x = y / Infinity", "x = y / Infinity;\n")
	expectPrinted(t, "throw Infinity", "throw Infinity;\n")

	expectPrintedMinify(t, "x = Infinity", "x=Infinity;")
	expectPrintedMinify(t, "x = -Infinity", "x=-Infinity;")
	expectPrintedMinify(t, "x = (Infinity).toString", "x=Infinity.toString;")
	expectPrintedMinify(t, "x = (-Infinity).toString", "x=(-Infinity).toString;")
	expectPrintedMinify(t, "x = (Infinity) ** 2", "x=Infinity**2;")
	expectPrintedMinify(t, "x = (-Infinity) ** 2", "x=(-Infinity)**2;")
	expectPrintedMinify(t, "x = ~Infinity", "x=~Infinity;")
	expectPrintedMinify(t, "x = ~-Infinity", "x=~-Infinity;")
	expectPrintedMinify(t, "x = Infinity * y", "x=Infinity*y;")
	expectPrintedMinify(t, "x = Infinity / y", "x=Infinity/y;")
	expectPrintedMinify(t, "x = y * Infinity", "x=y*Infinity;")
	expectPrintedMinify(t, "x = y / Infinity", "x=y/Infinity;")
	expectPrintedMinify(t, "throw Infinity", "throw Infinity;")

	expectPrintedMangle(t, "x = Infinity", "x = 1 / 0;\n")
	expectPrintedMangle(t, "x = -Infinity", "x = -1 / 0;\n")
	expectPrintedMangle(t, "x = (Infinity).toString", "x = (1 / 0).toString;\n")
	expectPrintedMangle(t, "x = (-Infinity).toString", "x = (-1 / 0).toString;\n")
	expectPrintedMangle(t, "x = Infinity ** 2", "x = (1 / 0) ** 2;\n")
	expectPrintedMangle(t, "x = (-Infinity) ** 2", "x = (-1 / 0) ** 2;\n")
	expectPrintedMangle(t, "x = Infinity * y", "x = 1 / 0 * y;\n")
	expectPrintedMangle(t, "x = Infinity / y", "x = 1 / 0 / y;\n")
	expectPrintedMangle(t, "x = y * Infinity", "x = y * (1 / 0);\n")
	expectPrintedMangle(t, "x = y / Infinity", "x = y / (1 / 0);\n")
	expectPrintedMangle(t, "throw Infinity", "throw 1 / 0;\n")

	expectPrintedMangleMinify(t, "x = Infinity", "x=1/0;")
	expectPrintedMangleMinify(t, "x = -Infinity", "x=-1/0;")
	expectPrintedMangleMinify(t, "x = (Infinity).toString", "x=(1/0).toString;")
	expectPrintedMangleMinify(t, "x = (-Infinity).toString", "x=(-1/0).toString;")
	expectPrintedMangleMinify(t, "x = Infinity ** 2", "x=(1/0)**2;")
	expectPrintedMangleMinify(t, "x = (-Infinity) ** 2", "x=(-1/0)**2;")
	expectPrintedMangleMinify(t, "x = Infinity * y", "x=1/0*y;")
	expectPrintedMangleMinify(t, "x = Infinity / y", "x=1/0/y;")
	expectPrintedMangleMinify(t, "x = y * Infinity", "x=y*(1/0);")
	expectPrintedMangleMinify(t, "x = y / Infinity", "x=y/(1/0);")
	expectPrintedMangleMinify(t, "throw Infinity", "throw 1/0;")
}

func TestBinaryOperatorVisitor(t *testing.T) {
	// Make sure the inner "/*b*/" comment doesn't disappear due to weird binary visitor stuff
	expectPrintedMangle(t, "x = (0, /*a*/ (0, /*b*/ (0, /*c*/ 1 == 2) + 3) * 4)", "x = /*a*/\n/*b*/\n(/*c*/\n!1 + 3) * 4;\n")

	// Make sure deeply-nested ASTs don't cause a stack overflow
	x := "x = f()" + strings.Repeat(" || f()", 10_000) + ";\n"
	expectPrinted(t, x, x)
}

// See: https://github.com/tc39/proposal-explicit-resource-management
func TestUsing(t *testing.T) {
	expectPrinted(t, "using x = y", "using x = y;\n")
	expectPrinted(t, "using x = y, z = _", "using x = y, z = _;\n")
	expectPrintedMinify(t, "using x = y", "using x=y;")
	expectPrintedMinify(t, "using x = y, z = _", "using x=y,z=_;")

	expectPrinted(t, "await using x = y", "await using x = y;\n")
	expectPrinted(t, "await using x = y, z = _", "await using x = y, z = _;\n")
	expectPrintedMinify(t, "await using x = y", "await using x=y;")
	expectPrintedMinify(t, "await using x = y, z = _", "await using x=y,z=_;")
}

func TestMinifyBigInt(t *testing.T) {
	expectPrintedTargetMangle(t, 2019, "x = 0b100101n", "x = /* @__PURE__ */ BigInt(37);\n")
	expectPrintedTargetMangle(t, 2019, "x = 0B100101n", "x = /* @__PURE__ */ BigInt(37);\n")
	expectPrintedTargetMangle(t, 2019, "x = 0o76543210n", "x = /* @__PURE__ */ BigInt(16434824);\n")
	expectPrintedTargetMangle(t, 2019, "x = 0O76543210n", "x = /* @__PURE__ */ BigInt(16434824);\n")
	expectPrintedTargetMangle(t, 2019, "x = 0xFEDCBA9876543210n", "x = /* @__PURE__ */ BigInt(\"0xFEDCBA9876543210\");\n")
	expectPrintedTargetMangle(t, 2019, "x = 0XFEDCBA9876543210n", "x = /* @__PURE__ */ BigInt(\"0XFEDCBA9876543210\");\n")
	expectPrintedTargetMangle(t, 2019, "x = 0xb0ba_cafe_f00dn", "x = /* @__PURE__ */ BigInt(0xb0bacafef00d);\n")
	expectPrintedTargetMangle(t, 2019, "x = 0xB0BA_CAFE_F00Dn", "x = /* @__PURE__ */ BigInt(0xB0BACAFEF00D);\n")
	expectPrintedTargetMangle(t, 2019, "x = 102030405060708090807060504030201n", "x = /* @__PURE__ */ BigInt(\"102030405060708090807060504030201\");\n")
}
