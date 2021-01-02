package js_parser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/js_printer"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/renamer"
	"github.com/evanw/esbuild/internal/test"
)

func expectParseErrorCommon(t *testing.T, contents string, expected string, options config.Options) {
	t.Helper()
	t.Run(contents, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog()
		Parse(log, test.SourceForTest(contents), OptionsFromConfig(&options))
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logger.OutputOptions{}, logger.TerminalInfo{})
		}
		test.AssertEqual(t, text, expected)
	})
}

func expectParseError(t *testing.T, contents string, expected string) {
	t.Helper()
	expectParseErrorCommon(t, contents, expected, config.Options{})
}

func expectParseErrorTarget(t *testing.T, esVersion int, contents string, expected string) {
	t.Helper()
	expectParseErrorCommon(t, contents, expected, config.Options{
		UnsupportedJSFeatures: compat.UnsupportedJSFeatures(map[compat.Engine][]int{
			compat.ES: {esVersion},
		}),
	})
}

func expectPrintedCommon(t *testing.T, contents string, expected string, options config.Options) {
	t.Helper()
	t.Run(contents, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog()
		options.OmitRuntimeForTests = true
		tree, ok := Parse(log, test.SourceForTest(contents), OptionsFromConfig(&options))
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			if msg.Kind != logger.Warning {
				text += msg.String(logger.OutputOptions{}, logger.TerminalInfo{})
			}
		}
		test.AssertEqual(t, text, "")
		if !ok {
			t.Fatal("Parse error")
		}
		symbols := js_ast.NewSymbolMap(1)
		symbols.Outer[0] = tree.Symbols
		r := renamer.NewNoOpRenamer(symbols)
		js := js_printer.Print(tree, symbols, r, js_printer.Options{
			UnsupportedFeatures: options.UnsupportedJSFeatures,
			ASCIIOnly:           options.ASCIIOnly,
		}).JS
		test.AssertEqual(t, string(js), expected)
	})
}

func expectPrinted(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{})
}

func expectPrintedMangle(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		MangleSyntax: true,
	})
}

func expectPrintedTarget(t *testing.T, esVersion int, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		UnsupportedJSFeatures: compat.UnsupportedJSFeatures(map[compat.Engine][]int{
			compat.ES: {esVersion},
		}),
	})
}

func expectPrintedMangleTarget(t *testing.T, esVersion int, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		UnsupportedJSFeatures: compat.UnsupportedJSFeatures(map[compat.Engine][]int{
			compat.ES: {esVersion},
		}),
		MangleSyntax: true,
	})
}

func expectPrintedASCII(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		ASCIIOnly: true,
	})
}

func expectPrintedTargetASCII(t *testing.T, esVersion int, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		UnsupportedJSFeatures: compat.UnsupportedJSFeatures(map[compat.Engine][]int{
			compat.ES: {esVersion},
		}),
		ASCIIOnly: true,
	})
}

func expectParseErrorTargetASCII(t *testing.T, esVersion int, contents string, expected string) {
	t.Helper()
	expectParseErrorCommon(t, contents, expected, config.Options{
		UnsupportedJSFeatures: compat.UnsupportedJSFeatures(map[compat.Engine][]int{
			compat.ES: {esVersion},
		}),
		ASCIIOnly: true,
	})
}

func expectParseErrorJSX(t *testing.T, contents string, expected string) {
	t.Helper()
	expectParseErrorCommon(t, contents, expected, config.Options{
		JSX: config.JSXOptions{
			Parse: true,
		},
	})
}

func expectPrintedJSX(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		JSX: config.JSXOptions{
			Parse: true,
		},
	})
}

func TestBinOp(t *testing.T) {
	for code, entry := range js_ast.OpTable {
		opCode := js_ast.OpCode(code)

		if opCode.IsLeftAssociative() {
			op := entry.Text
			expectPrinted(t, "a "+op+" b "+op+" c", "a "+op+" b "+op+" c;\n")
			expectPrinted(t, "(a "+op+" b) "+op+" c", "a "+op+" b "+op+" c;\n")
			expectPrinted(t, "a "+op+" (b "+op+" c)", "a "+op+" (b "+op+" c);\n")
		}

		if opCode.IsRightAssociative() {
			op := entry.Text
			expectPrinted(t, "a "+op+" b "+op+" c", "a "+op+" b "+op+" c;\n")

			// Avoid errors about invalid assignment targets
			if opCode.BinaryAssignTarget() == js_ast.AssignTargetNone {
				expectPrinted(t, "(a "+op+" b) "+op+" c", "(a "+op+" b) "+op+" c;\n")
			}

			expectPrinted(t, "a "+op+" (b "+op+" c)", "a "+op+" b "+op+" c;\n")
		}
	}
}

func TestComments(t *testing.T) {
	expectParseError(t, "throw //\n x", "<stdin>: error: Unexpected newline after \"throw\"\n")
	expectParseError(t, "throw /**/\n x", "<stdin>: error: Unexpected newline after \"throw\"\n")
	expectParseError(t, "throw <!--\n x",
		`<stdin>: error: Unexpected newline after "throw"
<stdin>: warning: Treating "<!--" as the start of a legacy HTML single-line comment
`)
	expectParseError(t, "throw -->\n x", "<stdin>: error: Unexpected \">\"\n")

	expectPrinted(t, "return //\n x", "return;\nx;\n")
	expectPrinted(t, "return /**/\n x", "return;\nx;\n")
	expectPrinted(t, "return <!--\n x", "return;\nx;\n")
	expectPrinted(t, "-->\nx", "x;\n")
	expectPrinted(t, "x\n-->\ny", "x;\ny;\n")
	expectPrinted(t, "x\n -->\ny", "x;\ny;\n")
	expectPrinted(t, "x\n/**/-->\ny", "x;\ny;\n")
	expectPrinted(t, "x/*\n*/-->\ny", "x;\ny;\n")
	expectPrinted(t, "x\n/**/ /**/-->\ny", "x;\ny;\n")
	expectPrinted(t, "if(x-->y)z", "if (x-- > y)\n  z;\n")
}

func TestExponentiation(t *testing.T) {
	expectPrinted(t, "--x ** 2", "--x ** 2;\n")
	expectPrinted(t, "++x ** 2", "++x ** 2;\n")
	expectPrinted(t, "x-- ** 2", "x-- ** 2;\n")
	expectPrinted(t, "x++ ** 2", "x++ ** 2;\n")

	expectPrinted(t, "(-x) ** 2", "(-x) ** 2;\n")
	expectPrinted(t, "(+x) ** 2", "(+x) ** 2;\n")
	expectPrinted(t, "(~x) ** 2", "(~x) ** 2;\n")
	expectPrinted(t, "(!x) ** 2", "(!x) ** 2;\n")
	expectPrinted(t, "(-1) ** 2", "(-1) ** 2;\n")
	expectPrinted(t, "(+1) ** 2", "1 ** 2;\n")
	expectPrinted(t, "(~1) ** 2", "(~1) ** 2;\n")
	expectPrinted(t, "(!1) ** 2", "false ** 2;\n")
	expectPrinted(t, "(void x) ** 2", "(void x) ** 2;\n")
	expectPrinted(t, "(delete x) ** 2", "(delete x) ** 2;\n")
	expectPrinted(t, "(typeof x) ** 2", "(typeof x) ** 2;\n")
	expectPrinted(t, "undefined ** 2", "(void 0) ** 2;\n")

	expectParseError(t, "-x ** 2", "<stdin>: error: Unexpected \"**\"\n")
	expectParseError(t, "+x ** 2", "<stdin>: error: Unexpected \"**\"\n")
	expectParseError(t, "~x ** 2", "<stdin>: error: Unexpected \"**\"\n")
	expectParseError(t, "!x ** 2", "<stdin>: error: Unexpected \"**\"\n")
	expectParseError(t, "void x ** 2", "<stdin>: error: Unexpected \"**\"\n")
	expectParseError(t, "delete x ** 2", "<stdin>: error: Unexpected \"**\"\n")
	expectParseError(t, "typeof x ** 2", "<stdin>: error: Unexpected \"**\"\n")

	expectParseError(t, "-x.y() ** 2", "<stdin>: error: Unexpected \"**\"\n")
	expectParseError(t, "+x.y() ** 2", "<stdin>: error: Unexpected \"**\"\n")
	expectParseError(t, "~x.y() ** 2", "<stdin>: error: Unexpected \"**\"\n")
	expectParseError(t, "!x.y() ** 2", "<stdin>: error: Unexpected \"**\"\n")
	expectParseError(t, "void x.y() ** 2", "<stdin>: error: Unexpected \"**\"\n")
	expectParseError(t, "delete x.y() ** 2", "<stdin>: error: Unexpected \"**\"\n")
	expectParseError(t, "typeof x.y() ** 2", "<stdin>: error: Unexpected \"**\"\n")
}

func TestRegExp(t *testing.T) {
	expectPrinted(t, "/x/g", "/x/g;\n")
	expectPrinted(t, "/x/i", "/x/i;\n")
	expectPrinted(t, "/x/m", "/x/m;\n")
	expectPrinted(t, "/x/s", "/x/s;\n")
	expectPrinted(t, "/x/u", "/x/u;\n")
	expectPrinted(t, "/x/y", "/x/y;\n")
}

func TestIdentifierEscapes(t *testing.T) {
	expectPrinted(t, "var _\\u0076\\u0061\\u0072", "var _var;\n")
	expectParseError(t, "var \\u0076\\u0061\\u0072", "<stdin>: error: Expected identifier but found \"\\\\u0076\\\\u0061\\\\u0072\"\n")
	expectParseError(t, "\\u0076\\u0061\\u0072 foo", "<stdin>: error: Unexpected \"\\\\u0076\\\\u0061\\\\u0072\"\n")

	expectPrinted(t, "foo._\\u0076\\u0061\\u0072", "foo._var;\n")
	expectPrinted(t, "foo.\\u0076\\u0061\\u0072", "foo.var;\n")

	expectParseError(t, "\u200Ca", "<stdin>: error: Unexpected \"\\u200c\"\n")
	expectParseError(t, "\u200Da", "<stdin>: error: Unexpected \"\\u200d\"\n")
}

func TestSpecialIdentifiers(t *testing.T) {
	expectPrinted(t, "exports", "exports;\n")
	expectPrinted(t, "require", "require;\n")
	expectPrinted(t, "module", "module;\n")
}

func TestDecls(t *testing.T) {
	expectParseError(t, "var x = 0", "")
	expectParseError(t, "let x = 0", "")
	expectParseError(t, "const x = 0", "")
	expectParseError(t, "for (var x = 0;;) ;", "")
	expectParseError(t, "for (let x = 0;;) ;", "")
	expectParseError(t, "for (const x = 0;;) ;", "")

	expectParseError(t, "for (var x in y) ;", "")
	expectParseError(t, "for (let x in y) ;", "")
	expectParseError(t, "for (const x in y) ;", "")
	expectParseError(t, "for (var x of y) ;", "")
	expectParseError(t, "for (let x of y) ;", "")
	expectParseError(t, "for (const x of y) ;", "")

	expectParseError(t, "var x", "")
	expectParseError(t, "let x", "")
	expectParseError(t, "const x", "<stdin>: error: The constant \"x\" must be initialized\n")
	expectParseError(t, "const {}", "<stdin>: error: This constant must be initialized\n")
	expectParseError(t, "const []", "<stdin>: error: This constant must be initialized\n")
	expectParseError(t, "for (var x;;) ;", "")
	expectParseError(t, "for (let x;;) ;", "")
	expectParseError(t, "for (const x;;) ;", "<stdin>: error: The constant \"x\" must be initialized\n")
	expectParseError(t, "for (const {};;) ;", "<stdin>: error: This constant must be initialized\n")
	expectParseError(t, "for (const [];;) ;", "<stdin>: error: This constant must be initialized\n")

	// Make sure bindings are visited during parsing
	expectPrinted(t, "var {[x]: y} = {}", "var {[x]: y} = {};\n")
	expectPrinted(t, "var {...x} = {}", "var {...x} = {};\n")

	// Test destructuring patterns
	expectPrinted(t, "var [...x] = []", "var [...x] = [];\n")
	expectPrinted(t, "var {...x} = {}", "var {...x} = {};\n")
	expectPrinted(t, "([...x] = []) => {}", "([...x] = []) => {\n};\n")
	expectPrinted(t, "({...x} = {}) => {}", "({...x} = {}) => {\n};\n")

	expectParseError(t, "var [...x,] = []", "<stdin>: error: Unexpected \",\" after rest pattern\n")
	expectParseError(t, "var {...x,} = {}", "<stdin>: error: Unexpected \",\" after rest pattern\n")
	expectParseError(t, "([...x,] = []) => {}", "<stdin>: error: Unexpected \",\" after rest pattern\n")
	expectParseError(t, "({...x,} = {}) => {}", "<stdin>: error: Unexpected \",\" after rest pattern\n")

	expectPrinted(t, "[b, ...c] = d", "[b, ...c] = d;\n")
	expectPrinted(t, "([b, ...c] = d)", "[b, ...c] = d;\n")
	expectPrinted(t, "({b, ...c} = d)", "({b, ...c} = d);\n")
	expectPrinted(t, "({a = b} = c)", "({a = b} = c);\n")
	expectPrinted(t, "({a: b = c} = d)", "({a: b = c} = d);\n")
	expectPrinted(t, "({a: b.c} = d)", "({a: b.c} = d);\n")
	expectPrinted(t, "[a = {}] = b", "[a = {}] = b;\n")

	expectParseError(t, "[b, ...c,] = d", "<stdin>: error: Unexpected \",\" after rest pattern\n")
	expectParseError(t, "([b, ...c,] = d)", "<stdin>: error: Unexpected \",\" after rest pattern\n")
	expectParseError(t, "({b, ...c,} = d)", "<stdin>: error: Unexpected \",\" after rest pattern\n")
	expectParseError(t, "({a = b})", "<stdin>: error: Unexpected \"=\"\n")
	expectParseError(t, "({a = b}) = c", "<stdin>: error: Unexpected \"=\"\n")
	expectParseError(t, "[a = {b = c}] = d", "<stdin>: error: Unexpected \"=\"\n")

	expectPrinted(t, "for ([{a = {}}] in b) {}", "for ([{a = {}}] in b) {\n}\n")
	expectPrinted(t, "for ([{a = {}}] of b) {}", "for ([{a = {}}] of b) {\n}\n")
	expectPrinted(t, "for ({a = {}} in b) {}", "for ({a = {}} in b) {\n}\n")
	expectPrinted(t, "for ({a = {}} of b) {}", "for ({a = {}} of b) {\n}\n")

	expectParseError(t, "({a = {}} in b)", "<stdin>: error: Unexpected \"=\"\n")
	expectParseError(t, "[{a = {}}]\nof()", "<stdin>: error: Unexpected \"=\"\n")
	expectParseError(t, "for ([...a, b] in c) {}", "<stdin>: error: Unexpected \",\" after rest pattern\n")
	expectParseError(t, "for ([...a, b] of c) {}", "<stdin>: error: Unexpected \",\" after rest pattern\n")
}

func TestBreakAndContinue(t *testing.T) {
	expectParseError(t, "break", "<stdin>: error: Cannot use \"break\" here\n")
	expectParseError(t, "continue", "<stdin>: error: Cannot use \"continue\" here\n")

	expectParseError(t, "x: { break }", "<stdin>: error: Cannot use \"break\" here\n")
	expectParseError(t, "x: { break x }", "")
	expectParseError(t, "x: { continue }", "<stdin>: error: Cannot use \"continue\" here\n")
	expectParseError(t, "x: { continue x }", "<stdin>: error: Cannot continue to label \"x\"\n")

	expectParseError(t, "while (1) break", "")
	expectParseError(t, "while (1) continue", "")
	expectParseError(t, "while (1) { function foo() { break } }", "<stdin>: error: Cannot use \"break\" here\n")
	expectParseError(t, "while (1) { function foo() { continue } }", "<stdin>: error: Cannot use \"continue\" here\n")
	expectParseError(t, "x: while (1) break x", "")
	expectParseError(t, "x: while (1) continue x", "")
	expectParseError(t, "x: while (1) y: { break x }", "")
	expectParseError(t, "x: while (1) y: { continue x }", "")
	expectParseError(t, "x: while (1) y: { break y }", "")
	expectParseError(t, "x: while (1) y: { continue y }", "<stdin>: error: Cannot continue to label \"y\"\n")
	expectParseError(t, "x: while (1) { function foo() { break x } }", "<stdin>: error: There is no containing label named \"x\"\n")
	expectParseError(t, "x: while (1) { function foo() { continue x } }", "<stdin>: error: There is no containing label named \"x\"\n")

	expectParseError(t, "switch (1) { case 1: break }", "")
	expectParseError(t, "switch (1) { case 1: continue }", "<stdin>: error: Cannot use \"continue\" here\n")
	expectParseError(t, "x: switch (1) { case 1: break x }", "")
	expectParseError(t, "x: switch (1) { case 1: continue x }", "<stdin>: error: Cannot continue to label \"x\"\n")
}

func TestFor(t *testing.T) {
	expectParseError(t, "for (; in x) ;", "<stdin>: error: Unexpected \"in\"\n")
	expectParseError(t, "for (; of x) ;", "<stdin>: error: Expected \";\" but found \"x\"\n")
	expectParseError(t, "for (; in; ) ;", "<stdin>: error: Unexpected \"in\"\n")
	expectPrinted(t, "for (; of; ) ;", "for (; of; )\n  ;\n")

	expectPrinted(t, "for (a in b) ;", "for (a in b)\n  ;\n")
	expectPrinted(t, "for (var a in b) ;", "for (var a in b)\n  ;\n")
	expectPrinted(t, "for (let a in b) ;", "for (let a in b)\n  ;\n")
	expectPrinted(t, "for (const a in b) ;", "for (const a in b)\n  ;\n")
	expectPrinted(t, "for (a in b, c) ;", "for (a in b, c)\n  ;\n")
	expectPrinted(t, "for (a in b = c) ;", "for (a in b = c)\n  ;\n")
	expectPrinted(t, "for (var a in b, c) ;", "for (var a in b, c)\n  ;\n")
	expectPrinted(t, "for (var a in b = c) ;", "for (var a in b = c)\n  ;\n")
	expectParseError(t, "for (var a, b in b) ;", "<stdin>: error: for-in loops must have a single declaration\n")
	expectParseError(t, "for (let a, b in b) ;", "<stdin>: error: for-in loops must have a single declaration\n")
	expectParseError(t, "for (const a, b in b) ;", "<stdin>: error: for-in loops must have a single declaration\n")

	expectPrinted(t, "for (a of b) ;", "for (a of b)\n  ;\n")
	expectPrinted(t, "for (var a of b) ;", "for (var a of b)\n  ;\n")
	expectPrinted(t, "for (let a of b) ;", "for (let a of b)\n  ;\n")
	expectPrinted(t, "for (const a of b) ;", "for (const a of b)\n  ;\n")
	expectPrinted(t, "for (a of b = c) ;", "for (a of b = c)\n  ;\n")
	expectPrinted(t, "for (var a of b = c) ;", "for (var a of b = c)\n  ;\n")
	expectParseError(t, "for (a of b, c) ;", "<stdin>: error: Expected \")\" but found \",\"\n")
	expectParseError(t, "for (var a of b, c) ;", "<stdin>: error: Expected \")\" but found \",\"\n")
	expectParseError(t, "for (var a, b of b) ;", "<stdin>: error: for-of loops must have a single declaration\n")
	expectParseError(t, "for (let a, b of b) ;", "<stdin>: error: for-of loops must have a single declaration\n")
	expectParseError(t, "for (const a, b of b) ;", "<stdin>: error: for-of loops must have a single declaration\n")

	expectPrinted(t, "for (var x = 0 in y) ;", "for (var x = 0 in y)\n  ;\n") // This is a weird special-case
	expectParseError(t, "for (let x = 0 in y) ;", "<stdin>: error: for-in loop variables cannot have an initializer\n")
	expectParseError(t, "for (const x = 0 in y) ;", "<stdin>: error: for-in loop variables cannot have an initializer\n")
	expectParseError(t, "for (var x = 0 of y) ;", "<stdin>: error: for-of loop variables cannot have an initializer\n")
	expectParseError(t, "for (let x = 0 of y) ;", "<stdin>: error: for-of loop variables cannot have an initializer\n")
	expectParseError(t, "for (const x = 0 of y) ;", "<stdin>: error: for-of loop variables cannot have an initializer\n")

	expectParseError(t, "for (var [x] = y in z) ;", "<stdin>: error: for-in loop variables cannot have an initializer\n")
	expectParseError(t, "for (let [x] = y in z) ;", "<stdin>: error: for-in loop variables cannot have an initializer\n")
	expectParseError(t, "for (const [x] = y in z) ;", "<stdin>: error: for-in loop variables cannot have an initializer\n")
	expectParseError(t, "for (var [x] = y of z) ;", "<stdin>: error: for-of loop variables cannot have an initializer\n")
	expectParseError(t, "for (let [x] = y of z) ;", "<stdin>: error: for-of loop variables cannot have an initializer\n")
	expectParseError(t, "for (const [x] = y of z) ;", "<stdin>: error: for-of loop variables cannot have an initializer\n")

	expectParseError(t, "for (var {x} = y in z) ;", "<stdin>: error: for-in loop variables cannot have an initializer\n")
	expectParseError(t, "for (let {x} = y in z) ;", "<stdin>: error: for-in loop variables cannot have an initializer\n")
	expectParseError(t, "for (const {x} = y in z) ;", "<stdin>: error: for-in loop variables cannot have an initializer\n")
	expectParseError(t, "for (var {x} = y of z) ;", "<stdin>: error: for-of loop variables cannot have an initializer\n")
	expectParseError(t, "for (let {x} = y of z) ;", "<stdin>: error: for-of loop variables cannot have an initializer\n")
	expectParseError(t, "for (const {x} = y of z) ;", "<stdin>: error: for-of loop variables cannot have an initializer\n")

	// Make sure "in" rules are enabled
	expectPrinted(t, "for (var x = () => a in b);", "for (var x = () => a in b)\n  ;\n")
	expectPrinted(t, "for (var x = a + b in c);", "for (var x = a + b in c)\n  ;\n")

	// Make sure "in" rules are disabled
	expectPrinted(t, "for (var {[x in y]: z} = {};;);", "for (var {[x in y]: z} = {}; ; )\n  ;\n")
	expectPrinted(t, "for (var {x = y in z} = {};;);", "for (var {x = y in z} = {}; ; )\n  ;\n")
	expectPrinted(t, "for (var [x = y in z] = {};;);", "for (var [x = y in z] = {}; ; )\n  ;\n")
	expectPrinted(t, "for (var {x: y = z in w} = {};;);", "for (var {x: y = z in w} = {}; ; )\n  ;\n")
	expectPrinted(t, "for (var x = (a in b);;);", "for (var x = (a in b); ; )\n  ;\n")
	expectPrinted(t, "for (var x = [a in b];;);", "for (var x = [a in b]; ; )\n  ;\n")
	expectPrinted(t, "for (var x = y(a in b);;);", "for (var x = y(a in b); ; )\n  ;\n")
	expectPrinted(t, "for (var x = {y: a in b};;);", "for (var x = {y: a in b}; ; )\n  ;\n")
	expectPrinted(t, "for (a ? b in c : d;;);", "for (a ? b in c : d; ; )\n  ;\n")
	expectPrinted(t, "for (var x = () => { a in b };;);", "for (var x = () => {\n  a in b;\n}; ; )\n  ;\n")
	expectPrinted(t, "for (var x = async () => { a in b };;);", "for (var x = async () => {\n  a in b;\n}; ; )\n  ;\n")
	expectPrinted(t, "for (var x = function() { a in b };;);", "for (var x = function() {\n  a in b;\n}; ; )\n  ;\n")
	expectPrinted(t, "for (var x = async function() { a in b };;);", "for (var x = async function() {\n  a in b;\n}; ; )\n  ;\n")
	expectPrinted(t, "for (var x = class { [a in b]() {} };;);", "for (var x = class {\n  [a in b]() {\n  }\n}; ; )\n  ;\n")
	expectParseError(t, "for (var x = class extends a in b {};;);", "<stdin>: error: Expected \"{\" but found \"in\"\n")

	errorText := `<stdin>: warning: This assignment will throw because "x" is a constant
<stdin>: note: "x" was declared a constant here
`
	expectParseError(t, "for (var x = 0; ; x = 1) ;", "")
	expectParseError(t, "for (let x = 0; ; x = 1) ;", "")
	expectParseError(t, "for (const x = 0; ; x = 1) ;", errorText)
	expectParseError(t, "for (var x = 0; ; x++) ;", "")
	expectParseError(t, "for (let x = 0; ; x++) ;", "")
	expectParseError(t, "for (const x = 0; ; x++) ;", errorText)

	expectParseError(t, "for (var x in y) x = 1", "")
	expectParseError(t, "for (let x in y) x = 1", "")
	expectParseError(t, "for (const x in y) x = 1", errorText)
	expectParseError(t, "for (var x in y) x++", "")
	expectParseError(t, "for (let x in y) x++", "")
	expectParseError(t, "for (const x in y) x++", errorText)

	expectParseError(t, "for (var x of y) x = 1", "")
	expectParseError(t, "for (let x of y) x = 1", "")
	expectParseError(t, "for (const x of y) x = 1", errorText)
	expectParseError(t, "for (var x of y) x++", "")
	expectParseError(t, "for (let x of y) x++", "")
	expectParseError(t, "for (const x of y) x++", errorText)
}

func TestScope(t *testing.T) {
	errorText := `<stdin>: error: "x" has already been declared
<stdin>: note: "x" was originally declared here
`

	expectParseError(t, "var x; var y", "")
	expectParseError(t, "var x; let y", "")
	expectParseError(t, "let x; var y", "")
	expectParseError(t, "let x; let y", "")

	expectParseError(t, "var x; var x", "")
	expectParseError(t, "var x; let x", errorText)
	expectParseError(t, "let x; var x", errorText)
	expectParseError(t, "let x; let x", errorText)
	expectParseError(t, "function x() {} let x", errorText)
	expectParseError(t, "let x; function x() {}", errorText)

	expectParseError(t, "var x; {var x}", "")
	expectParseError(t, "var x; {let x}", "")
	expectParseError(t, "let x; {var x}", errorText)
	expectParseError(t, "let x; {let x}", "")
	expectParseError(t, "let x; {function x() {}}", "")

	expectParseError(t, "{var x} var x", "")
	expectParseError(t, "{var x} let x", errorText)
	expectParseError(t, "{let x} var x", "")
	expectParseError(t, "{let x} let x", "")
	expectParseError(t, "{function x() {}} let x", "")

	expectParseError(t, "{var x; {var x}}", "")
	expectParseError(t, "{var x; {let x}}", "")
	expectParseError(t, "{let x; {var x}}", errorText)
	expectParseError(t, "{let x; {let x}}", "")
	expectParseError(t, "{let x; {function x() {}}}", "")

	expectParseError(t, "{{var x} var x}", "")
	expectParseError(t, "{{var x} let x}", errorText)
	expectParseError(t, "{{let x} var x}", "")
	expectParseError(t, "{{let x} let x}", "")
	expectParseError(t, "{{function x() {}} let x}", "")

	expectParseError(t, "{var x} {var x}", "")
	expectParseError(t, "{var x} {let x}", "")
	expectParseError(t, "{let x} {var x}", "")
	expectParseError(t, "{let x} {let x}", "")
	expectParseError(t, "{let x} {function x() {}}", "")
	expectParseError(t, "{function x() {}} {let x}", "")

	expectParseError(t, "var x=1, x=2", "")
	expectParseError(t, "let x=1, x=2", errorText)
	expectParseError(t, "const x=1, x=2", errorText)

	expectParseError(t, "function foo(x) { var x }", "")
	expectParseError(t, "function foo(x) { let x }", errorText)
	expectParseError(t, "function foo(x) { const x = 0 }", errorText)
	expectParseError(t, "function foo() { var foo }", "")
	expectParseError(t, "function foo() { let foo }", "")
	expectParseError(t, "function foo() { const foo = 0 }", "")

	expectParseError(t, "(function foo(x) { var x })", "")
	expectParseError(t, "(function foo(x) { let x })", errorText)
	expectParseError(t, "(function foo(x) { const x = 0 })", errorText)
	expectParseError(t, "(function foo() { var foo })", "")
	expectParseError(t, "(function foo() { let foo })", "")
	expectParseError(t, "(function foo() { const foo = 0 })", "")

	expectParseError(t, "var x; function x() {}", "")
	expectParseError(t, "var x; function *x() {}", "")
	expectParseError(t, "var x; async function x() {}", "")
	expectParseError(t, "let x; function x() {}", errorText)
	expectParseError(t, "function x() {} var x", "")
	expectParseError(t, "function* x() {} var x", "")
	expectParseError(t, "async function x() {} var x", "")
	expectParseError(t, "function x() {} let x", errorText)
	expectParseError(t, "function x() {} function x() {}", "")

	expectParseError(t, "var x; class x {}", errorText)
	expectParseError(t, "let x; class x {}", errorText)
	expectParseError(t, "class x {} var x", errorText)
	expectParseError(t, "class x {} let x", errorText)
	expectParseError(t, "class x {} class x {}", errorText)
}

func TestASI(t *testing.T) {
	expectParseError(t, "throw\n0", "<stdin>: error: Unexpected newline after \"throw\"\n")
	expectParseError(t, "return\n0", "<stdin>: warning: The following expression is not returned because of an automatically-inserted semicolon\n")
	expectPrinted(t, "return\n0", "return;\n0;\n")
	expectPrinted(t, "0\n[1]", "0[1];\n")
	expectPrinted(t, "0\n(1)", "0(1);\n")
	expectPrinted(t, "new x\n(1)", "new x(1);\n")
	expectPrinted(t, "while (true) break\nx", "while (true)\n  break;\nx;\n")
	expectPrinted(t, "x\n!y", "x;\n!y;\n")
	expectPrinted(t, "x\n++y", "x;\n++y;\n")
	expectPrinted(t, "x\n--y", "x;\n--y;\n")

	expectPrinted(t, "function* foo(){yield\na}", "function* foo() {\n  yield;\n  a;\n}\n")
	expectParseError(t, "function* foo(){yield\n*a}", "<stdin>: error: Unexpected \"*\"\n")
	expectPrinted(t, "function* foo(){yield*\na}", "function* foo() {\n  yield* a;\n}\n")

	expectPrinted(t, "async\nx => {}", "async;\n(x) => {\n};\n")
	expectPrinted(t, "async\nfunction foo() {}", "async;\nfunction foo() {\n}\n")
	expectPrinted(t, "export default async\nx => {}", "export default async;\n(x) => {\n};\n")
	expectPrinted(t, "export default async\nfunction foo() {}", "export default async;\nfunction foo() {\n}\n")
	expectParseError(t, "async\n() => {}", "<stdin>: error: Expected \";\" but found \"=>\"\n")
	expectParseError(t, "export async\nfunction foo() {}", "<stdin>: error: Unexpected newline after \"async\"\n")
	expectParseError(t, "export default async\n() => {}", "<stdin>: error: Expected \";\" but found \"=>\"\n")
	expectParseError(t, "(async\nx => {})", "<stdin>: error: Expected \")\" but found \"x\"\n")
	expectParseError(t, "(async\n() => {})", "<stdin>: error: Expected \")\" but found \"=>\"\n")
	expectParseError(t, "(async\nfunction foo() {})", "<stdin>: error: Expected \")\" but found \"function\"\n")

	expectPrinted(t, "if (0) let\nx = 0", "if (0)\n  let;\nx = 0;\n")
	expectPrinted(t, "if (0) let\n{x}", "if (0)\n  let;\n{\n  x;\n}\n")
	expectParseError(t, "if (0) let\n{x} = 0", "<stdin>: error: Unexpected \"=\"\n")
	expectParseError(t, "if (0) let\n[x] = 0", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectPrinted(t, "function *foo() { if (0) let\nyield 0 }", "function* foo() {\n  if (0)\n    let;\n  yield 0;\n}\n")
	expectPrinted(t, "async function foo() { if (0) let\nawait 0 }", "async function foo() {\n  if (0)\n    let;\n  await 0;\n}\n")

	expectPrinted(t, "let\nx = 0", "let x = 0;\n")
	expectPrinted(t, "let\n{x} = 0", "let {x} = 0;\n")
	expectPrinted(t, "let\n[x] = 0", "let [x] = 0;\n")
	expectParseError(t, "function *foo() { let\nyield 0 }",
		"<stdin>: error: Cannot use \"yield\" as an identifier here\n<stdin>: error: Expected \";\" but found \"0\"\n")
	expectParseError(t, "async function foo() { let\nawait 0 }",
		"<stdin>: error: Cannot use \"await\" as an identifier here\n<stdin>: error: Expected \";\" but found \"0\"\n")

	// This is a weird corner case where ASI applies without a newline
	expectPrinted(t, "do x;while(y)z", "do\n  x;\nwhile (y);\nz;\n")
	expectPrinted(t, "do x;while(y);z", "do\n  x;\nwhile (y);\nz;\n")
	expectPrinted(t, "{do x;while(y)}", "{\n  do\n    x;\n  while (y);\n}\n")
}

func TestLocal(t *testing.T) {
	expectPrinted(t, "var let = 0", "var let = 0;\n")
	expectParseError(t, "let let = 0", "<stdin>: error: Cannot use \"let\" as an identifier here\n")
	expectParseError(t, "const let = 0", "<stdin>: error: Cannot use \"let\" as an identifier here\n")

	expectPrinted(t, "var\nlet = 0", "var let = 0;\n")
	expectParseError(t, "let\nlet = 0", "<stdin>: error: Cannot use \"let\" as an identifier here\n")
	expectParseError(t, "const\nlet = 0", "<stdin>: error: Cannot use \"let\" as an identifier here\n")

	expectPrinted(t, "for (var let in x) ;", "for (var let in x)\n  ;\n")
	expectParseError(t, "for (let let in x) ;", "<stdin>: error: Cannot use \"let\" as an identifier here\n")
	expectParseError(t, "for (const let in x) ;", "<stdin>: error: Cannot use \"let\" as an identifier here\n")

	expectPrinted(t, "for (var let of x) ;", "for (var let of x)\n  ;\n")
	expectParseError(t, "for (let let of x) ;", "<stdin>: error: Cannot use \"let\" as an identifier here\n")
	expectParseError(t, "for (const let of x) ;", "<stdin>: error: Cannot use \"let\" as an identifier here\n")

	errorText := `<stdin>: warning: This assignment will throw because "x" is a constant
<stdin>: note: "x" was declared a constant here
`
	expectParseError(t, "var x = 0; x = 1", "")
	expectParseError(t, "let x = 0; x = 1", "")
	expectParseError(t, "const x = 0; x = 1", errorText)
	expectParseError(t, "var x = 0; x++", "")
	expectParseError(t, "let x = 0; x++", "")
	expectParseError(t, "const x = 0; x++", errorText)
}

func TestArrays(t *testing.T) {
	expectPrinted(t, "[]", "[];\n")
	expectPrinted(t, "[,]", "[,];\n")
	expectPrinted(t, "[1]", "[1];\n")
	expectPrinted(t, "[1,]", "[1];\n")
	expectPrinted(t, "[,1]", "[, 1];\n")
	expectPrinted(t, "[1,2]", "[1, 2];\n")
	expectPrinted(t, "[,1,2]", "[, 1, 2];\n")
	expectPrinted(t, "[1,,2]", "[1, , 2];\n")
	expectPrinted(t, "[1,2,]", "[1, 2];\n")
	expectPrinted(t, "[1,2,,]", "[1, 2, ,];\n")
}

func TestPattern(t *testing.T) {
	expectPrinted(t, "let {if: x} = y", "let {if: x} = y;\n")
	expectParseError(t, "let {x: if} = y", "<stdin>: error: Expected identifier but found \"if\"\n")

	expectPrinted(t, "let {1_2_3n: x} = y", "let {123n: x} = y;\n")
	expectPrinted(t, "let {0x1_2_3n: x} = y", "let {0x123n: x} = y;\n")
}

func TestAssignTarget(t *testing.T) {
	expectParseError(t, "x = 0", "")
	expectParseError(t, "x.y = 0", "")
	expectParseError(t, "x[y] = 0", "")
	expectParseError(t, "[,] = 0", "")
	expectParseError(t, "[x] = 0", "")
	expectParseError(t, "[x = y] = 0", "")
	expectParseError(t, "[...x] = 0", "")
	expectParseError(t, "({...x} = 0)", "")
	expectParseError(t, "({x = 0} = 0)", "")
	expectParseError(t, "({x: y = 0} = 0)", "")

	expectParseError(t, "[...x = y] = 0", "<stdin>: error: Invalid assignment target\n")
	expectParseError(t, "x() = 0", "<stdin>: error: Invalid assignment target\n")
	expectParseError(t, "x?.y = 0", "<stdin>: error: Invalid assignment target\n")
	expectParseError(t, "x?.[y] = 0", "<stdin>: error: Invalid assignment target\n")
	expectParseError(t, "({x: 0} = 0)", "<stdin>: error: Invalid assignment target\n")
	expectParseError(t, "({x() {}} = 0)", "<stdin>: error: Invalid assignment target\n")
	expectParseError(t, "({x: 0 = y} = 0)", "<stdin>: error: Invalid assignment target\n")
}

func TestObject(t *testing.T) {
	expectPrinted(t, "({foo})", "({foo});\n")
	expectPrinted(t, "({foo:0})", "({foo: 0});\n")
	expectPrinted(t, "({1e9:0})", "({1e9: 0});\n")
	expectPrinted(t, "({1_2_3n:0})", "({123n: 0});\n")
	expectPrinted(t, "({0x1_2_3n:0})", "({0x123n: 0});\n")
	expectPrinted(t, "({foo() {}})", "({foo() {\n}});\n")
	expectPrinted(t, "({*foo() {}})", "({*foo() {\n}});\n")
	expectPrinted(t, "({get foo() {}})", "({get foo() {\n}});\n")
	expectPrinted(t, "({set foo(x) {}})", "({set foo(x) {\n}});\n")

	expectPrinted(t, "({if:0})", "({if: 0});\n")
	expectPrinted(t, "({if() {}})", "({if() {\n}});\n")
	expectPrinted(t, "({*if() {}})", "({*if() {\n}});\n")
	expectPrinted(t, "({get if() {}})", "({get if() {\n}});\n")
	expectPrinted(t, "({set if(x) {}})", "({set if(x) {\n}});\n")

	expectParseError(t, "({static foo() {}})", "<stdin>: error: Expected \"}\" but found \"foo\"\n")
	expectParseError(t, "({`a`})", "<stdin>: error: Expected identifier but found \"`a`\"\n")
	expectParseError(t, "({if})", "<stdin>: error: Expected \":\" but found \"}\"\n")

	expectParseError(t, "({__proto__: 1, __proto__: 2})", "<stdin>: error: Cannot specify the \"__proto__\" property more than once per object\n")
	expectParseError(t, "({__proto__: 1, '__proto__': 2})", "<stdin>: error: Cannot specify the \"__proto__\" property more than once per object\n")
	expectParseError(t, "({__proto__: 1, __proto__() {}})", "")
	expectParseError(t, "({__proto__: 1, get __proto__() {}})", "")
	expectParseError(t, "({__proto__: 1, set __proto__(x) {}})", "")
	expectParseError(t, "({__proto__: 1, ['__proto__']: 2})", "")
	expectParseError(t, "({__proto__, __proto__: 2})", "")
	expectParseError(t, "({__proto__: x, __proto__: y} = z)", "")

	expectParseError(t, "({set foo() {}})", "<stdin>: error: Setter \"foo\" must have exactly one argument\n")
	expectParseError(t, "({get foo(x) {}})", "<stdin>: error: Getter \"foo\" must have zero arguments\n")
	expectParseError(t, "({set foo(x, y) {}})", "<stdin>: error: Setter \"foo\" must have exactly one argument\n")

	expectParseError(t, "(class {set #foo() {}})", "<stdin>: error: Setter \"#foo\" must have exactly one argument\n")
	expectParseError(t, "(class {get #foo(x) {}})", "<stdin>: error: Getter \"#foo\" must have zero arguments\n")
	expectParseError(t, "(class {set #foo(x, y) {}})", "<stdin>: error: Setter \"#foo\" must have exactly one argument\n")

	expectParseError(t, "({set [foo]() {}})", "<stdin>: error: Setter property must have exactly one argument\n")
	expectParseError(t, "({get [foo](x) {}})", "<stdin>: error: Getter property must have zero arguments\n")
	expectParseError(t, "({set [foo](x, y) {}})", "<stdin>: error: Setter property must have exactly one argument\n")
}

func TestComputedProperty(t *testing.T) {
	expectPrinted(t, "({[a]: foo})", "({[a]: foo});\n")
	expectPrinted(t, "({[(a, b)]: foo})", "({[(a, b)]: foo});\n")
	expectParseError(t, "({[a, b]: foo})", "<stdin>: error: Expected \"]\" but found \",\"\n")

	expectPrinted(t, "({[a]: foo}) => {}", "({[a]: foo}) => {\n};\n")
	expectPrinted(t, "({[(a, b)]: foo}) => {}", "({[(a, b)]: foo}) => {\n};\n")
	expectParseError(t, "({[a, b]: foo}) => {}", "<stdin>: error: Expected \"]\" but found \",\"\n")

	expectPrinted(t, "var {[a]: foo} = bar", "var {[a]: foo} = bar;\n")
	expectPrinted(t, "var {[(a, b)]: foo} = bar", "var {[(a, b)]: foo} = bar;\n")
	expectParseError(t, "var {[a, b]: foo} = bar", "<stdin>: error: Expected \"]\" but found \",\"\n")

	expectPrinted(t, "class Foo {[a] = foo}", "class Foo {\n  [a] = foo;\n}\n")
	expectPrinted(t, "class Foo {[(a, b)] = foo}", "class Foo {\n  [(a, b)] = foo;\n}\n")
	expectParseError(t, "class Foo {[a, b] = foo}", "<stdin>: error: Expected \"]\" but found \",\"\n")
}

func TestLexicalDecl(t *testing.T) {
	expectPrinted(t, "if (1) var x", "if (1)\n  var x;\n")
	expectPrinted(t, "if (1) function x() {}", "if (1)\n  function x() {\n  }\n")
	expectPrinted(t, "if (1) {} else function x() {}", "if (1) {\n} else\n  function x() {\n  }\n")
	expectPrinted(t, "switch (1) { case 1: const x = 1 }", "switch (1) {\n  case 1:\n    const x = 1;\n}\n")
	expectPrinted(t, "switch (1) { default: const x = 1 }", "switch (1) {\n  default:\n    const x = 1;\n}\n")

	singleStmtContext := []string{
		"label: %s",
		"for (;;) %s",
		"if (1) %s",
		"while (1) %s",
		"with ({}) %s",
		"if (1) {} else %s",
		"do %s \n while(0)",

		"for (;;) label: %s",
		"if (1) label: %s",
		"while (1) label: %s",
		"with ({}) label: %s",
		"if (1) {} else label: %s",
		"do label: %s \n while(0)",

		"for (;;) label: label2: %s",
		"if (1) label: label2: %s",
		"while (1) label: label2: %s",
		"with ({}) label: label2: %s",
		"if (1) {} else label: label2: %s",
		"do label: label2: %s \n while(0)",
	}

	for _, context := range singleStmtContext {
		expectParseError(t, fmt.Sprintf(context, "const x = 0"), "<stdin>: error: Cannot use a declaration in a single-statement context\n")
		expectParseError(t, fmt.Sprintf(context, "let x"), "<stdin>: error: Cannot use a declaration in a single-statement context\n")
		expectParseError(t, fmt.Sprintf(context, "class X {}"), "<stdin>: error: Cannot use a declaration in a single-statement context\n")
		expectParseError(t, fmt.Sprintf(context, "function* x() {}"), "<stdin>: error: Cannot use a declaration in a single-statement context\n")
		expectParseError(t, fmt.Sprintf(context, "async function x() {}"), "<stdin>: error: Cannot use a declaration in a single-statement context\n")
		expectParseError(t, fmt.Sprintf(context, "async function* x() {}"), "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	}

	expectPrinted(t, "function f() {}", "function f() {\n}\n")
	expectPrinted(t, "{function f() {}} let f", "{\n  function f() {\n  }\n}\nlet f;\n")
	expectPrinted(t, "if (1) function f() {} let f", "if (1)\n  function f() {\n  }\nlet f;\n")
	expectPrinted(t, "if (0) ; else function f() {} let f", "if (0)\n  ;\nelse\n  function f() {\n  }\nlet f;\n")
	expectPrinted(t, "x: function f() {}", "x:\n  function f() {\n  }\n")
	expectPrinted(t, "{function* f() {}} let f", "{\n  function* f() {\n  }\n}\nlet f;\n")
	expectPrinted(t, "{async function f() {}} let f", "{\n  async function f() {\n  }\n}\nlet f;\n")

	expectParseError(t, "if (1) label: function f() {} let f", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "if (1) label: label2: function f() {} let f", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "if (0) ; else label: function f() {} let f", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "if (0) ; else label: label2: function f() {} let f", "<stdin>: error: Cannot use a declaration in a single-statement context\n")

	expectParseError(t, "for (;;) function f() {}", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "for (x in y) function f() {}", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "for (x of y) function f() {}", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "for await (x of y) function f() {}", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "with (1) function f() {}", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "while (1) function f() {}", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "do function f() {} while (0)", "<stdin>: error: Cannot use a declaration in a single-statement context\n")

	expectParseError(t, "for (;;) label: function f() {}", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "for (x in y) label: function f() {}", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "for (x of y) label: function f() {}", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "for await (x of y) label: function f() {}", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "with (1) label: function f() {}", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "while (1) label: function f() {}", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "do label: function f() {} while (0)", "<stdin>: error: Cannot use a declaration in a single-statement context\n")

	expectParseError(t, "for (;;) label: label2: function f() {}", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "for (x in y) label: label2: function f() {}", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "for (x of y) label: label2: function f() {}", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "for await (x of y) label: label2: function f() {}", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "with (1) label: label2: function f() {}", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "while (1) label: label2: function f() {}", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "do label: label2: function f() {} while (0)", "<stdin>: error: Cannot use a declaration in a single-statement context\n")
}

func TestFunction(t *testing.T) {
	expectPrinted(t, "function f() {} function f() {}", "function f() {\n}\nfunction f() {\n}\n")
	expectPrinted(t, "function f() {} function* f() {}", "function f() {\n}\nfunction* f() {\n}\n")
	expectPrinted(t, "function* f() {} function* f() {}", "function* f() {\n}\nfunction* f() {\n}\n")
	expectPrinted(t, "function f() {} async function f() {}", "function f() {\n}\nasync function f() {\n}\n")
	expectPrinted(t, "async function f() {} async function f() {}", "async function f() {\n}\nasync function f() {\n}\n")

	expectPrinted(t, "function arguments() {}", "function arguments() {\n}\n")
	expectPrinted(t, "(function arguments() {})", "(function arguments() {\n});\n")
	expectPrinted(t, "function foo(arguments) {}", "function foo(arguments) {\n}\n")
	expectPrinted(t, "(function foo(arguments) {})", "(function foo(arguments) {\n});\n")
}

func TestClass(t *testing.T) {
	expectPrinted(t, "class Foo { foo() {} }", "class Foo {\n  foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { *foo() {} }", "class Foo {\n  *foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { get foo() {} }", "class Foo {\n  get foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { set foo(x) {} }", "class Foo {\n  set foo(x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { static foo() {} }", "class Foo {\n  static foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static *foo() {} }", "class Foo {\n  static *foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static get foo() {} }", "class Foo {\n  static get foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static set foo(x) {} }", "class Foo {\n  static set foo(x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { async foo() {} }", "class Foo {\n  async foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async foo() {} }", "class Foo {\n  static async foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async *foo() {} }", "class Foo {\n  static async *foo() {\n  }\n}\n")
	expectParseError(t, "class Foo { async static foo() {} }", "<stdin>: error: Expected \"(\" but found \"foo\"\n")

	expectPrinted(t, "class Foo { if() {} }", "class Foo {\n  if() {\n  }\n}\n")
	expectPrinted(t, "class Foo { *if() {} }", "class Foo {\n  *if() {\n  }\n}\n")
	expectPrinted(t, "class Foo { get if() {} }", "class Foo {\n  get if() {\n  }\n}\n")
	expectPrinted(t, "class Foo { set if(x) {} }", "class Foo {\n  set if(x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { static if() {} }", "class Foo {\n  static if() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static *if() {} }", "class Foo {\n  static *if() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static get if() {} }", "class Foo {\n  static get if() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static set if(x) {} }", "class Foo {\n  static set if(x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { async if() {} }", "class Foo {\n  async if() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async if() {} }", "class Foo {\n  static async if() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async *if() {} }", "class Foo {\n  static async *if() {\n  }\n}\n")
	expectParseError(t, "class Foo { async static if() {} }", "<stdin>: error: Expected \"(\" but found \"if\"\n")

	expectPrinted(t, "class Foo { a() {} b() {} }", "class Foo {\n  a() {\n  }\n  b() {\n  }\n}\n")
	expectPrinted(t, "class Foo { a() {} get b() {} }", "class Foo {\n  a() {\n  }\n  get b() {\n  }\n}\n")
	expectPrinted(t, "class Foo { a() {} set b(x) {} }", "class Foo {\n  a() {\n  }\n  set b(x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { a() {} static b() {} }", "class Foo {\n  a() {\n  }\n  static b() {\n  }\n}\n")
	expectPrinted(t, "class Foo { a() {} static *b() {} }", "class Foo {\n  a() {\n  }\n  static *b() {\n  }\n}\n")
	expectPrinted(t, "class Foo { a() {} static get b() {} }", "class Foo {\n  a() {\n  }\n  static get b() {\n  }\n}\n")
	expectPrinted(t, "class Foo { a() {} static set b(x) {} }", "class Foo {\n  a() {\n  }\n  static set b(x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { a() {} async b() {} }", "class Foo {\n  a() {\n  }\n  async b() {\n  }\n}\n")
	expectPrinted(t, "class Foo { a() {} static async b() {} }", "class Foo {\n  a() {\n  }\n  static async b() {\n  }\n}\n")
	expectPrinted(t, "class Foo { a() {} static async *b() {} }", "class Foo {\n  a() {\n  }\n  static async *b() {\n  }\n}\n")
	expectParseError(t, "class Foo { a() {} async static b() {} }", "<stdin>: error: Expected \"(\" but found \"b\"\n")

	expectParseError(t, "class Foo { `a`() {} }", "<stdin>: error: Expected identifier but found \"`a`\"\n")

	// Strict mode reserved words cannot be used as class names
	expectParseError(t, "class static {}", "<stdin>: error: Unexpected \"static\"\n")
	expectParseError(t, "class implements {}", "<stdin>: error: Unexpected \"implements\"\n")
	expectParseError(t, "(class static {})", "<stdin>: error: Expected \"{\" but found \"static\"\n")
	expectParseError(t, "(class implements {})", "<stdin>: error: Expected \"{\" but found \"implements\"\n")

	// The name "arguments" is forbidden
	expectParseError(t, "class Foo { arguments = 1 }", "")
	expectParseError(t, "class Foo { x = function() { arguments } }", "")
	expectParseError(t, "class Foo { [arguments] }", "<stdin>: error: Cannot access \"arguments\" here\n")
	expectParseError(t, "class Foo { [arguments = 1] }", "<stdin>: error: Cannot access \"arguments\" here\n")
	expectParseError(t, "class Foo { x = arguments }", "<stdin>: error: Cannot access \"arguments\" here\n")
	expectParseError(t, "class Foo { x = () => arguments }", "<stdin>: error: Cannot access \"arguments\" here\n")
	expectParseError(t, "class Foo { x = typeof arguments }", "<stdin>: error: Cannot access \"arguments\" here\n")
	expectParseError(t, "class Foo { x = 1 ? 2 : arguments }", "<stdin>: error: Cannot access \"arguments\" here\n")

	// The name "constructor" is sometimes forbidden
	expectPrinted(t, "class Foo { get ['constructor']() {} }", "class Foo {\n  get [\"constructor\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { set ['constructor'](x) {} }", "class Foo {\n  set [\"constructor\"](x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { *['constructor']() {} }", "class Foo {\n  *[\"constructor\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async ['constructor']() {} }", "class Foo {\n  async [\"constructor\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async *['constructor']() {} }", "class Foo {\n  async *[\"constructor\"]() {\n  }\n}\n")
	expectParseError(t, "class Foo { get constructor() {} }", "<stdin>: error: Class constructor cannot be a getter\n")
	expectParseError(t, "class Foo { get 'constructor'() {} }", "<stdin>: error: Class constructor cannot be a getter\n")
	expectParseError(t, "class Foo { set constructor(x) {} }", "<stdin>: error: Class constructor cannot be a setter\n")
	expectParseError(t, "class Foo { set 'constructor'(x) {} }", "<stdin>: error: Class constructor cannot be a setter\n")
	expectParseError(t, "class Foo { *constructor() {} }", "<stdin>: error: Class constructor cannot be a generator\n")
	expectParseError(t, "class Foo { *'constructor'() {} }", "<stdin>: error: Class constructor cannot be a generator\n")
	expectParseError(t, "class Foo { async constructor() {} }", "<stdin>: error: Class constructor cannot be an async function\n")
	expectParseError(t, "class Foo { async 'constructor'() {} }", "<stdin>: error: Class constructor cannot be an async function\n")
	expectParseError(t, "class Foo { async *constructor() {} }", "<stdin>: error: Class constructor cannot be an async function\n")
	expectParseError(t, "class Foo { async *'constructor'() {} }", "<stdin>: error: Class constructor cannot be an async function\n")
	expectPrinted(t, "class Foo { static get constructor() {} }", "class Foo {\n  static get constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static get 'constructor'() {} }", "class Foo {\n  static get constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static set constructor(x) {} }", "class Foo {\n  static set constructor(x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { static set 'constructor'(x) {} }", "class Foo {\n  static set constructor(x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { static *constructor() {} }", "class Foo {\n  static *constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static *'constructor'() {} }", "class Foo {\n  static *constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async constructor() {} }", "class Foo {\n  static async constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async 'constructor'() {} }", "class Foo {\n  static async constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async *constructor() {} }", "class Foo {\n  static async *constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async *'constructor'() {} }", "class Foo {\n  static async *constructor() {\n  }\n}\n")
	expectPrinted(t, "({ constructor: 1 })", "({constructor: 1});\n")
	expectPrinted(t, "({ get constructor() {} })", "({get constructor() {\n}});\n")
	expectPrinted(t, "({ set constructor(x) {} })", "({set constructor(x) {\n}});\n")
	expectPrinted(t, "({ *constructor() {} })", "({*constructor() {\n}});\n")
	expectPrinted(t, "({ async constructor() {} })", "({async constructor() {\n}});\n")
	expectPrinted(t, "({ async* constructor() {} })", "({async *constructor() {\n}});\n")

	// The name "prototype" is sometimes forbidden
	expectPrinted(t, "class Foo { get prototype() {} }", "class Foo {\n  get prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { get 'prototype'() {} }", "class Foo {\n  get prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { set prototype(x) {} }", "class Foo {\n  set prototype(x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { set 'prototype'(x) {} }", "class Foo {\n  set prototype(x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { *prototype() {} }", "class Foo {\n  *prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { *'prototype'() {} }", "class Foo {\n  *prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async prototype() {} }", "class Foo {\n  async prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async 'prototype'() {} }", "class Foo {\n  async prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async *prototype() {} }", "class Foo {\n  async *prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async *'prototype'() {} }", "class Foo {\n  async *prototype() {\n  }\n}\n")
	expectParseError(t, "class Foo { static get prototype() {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static get 'prototype'() {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static set prototype(x) {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static set 'prototype'(x) {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static *prototype() {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static *'prototype'() {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static async prototype() {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static async 'prototype'() {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static async *prototype() {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static async *'prototype'() {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectPrinted(t, "class Foo { static get ['prototype']() {} }", "class Foo {\n  static get [\"prototype\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static set ['prototype'](x) {} }", "class Foo {\n  static set [\"prototype\"](x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { static *['prototype']() {} }", "class Foo {\n  static *[\"prototype\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async ['prototype']() {} }", "class Foo {\n  static async [\"prototype\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async *['prototype']() {} }", "class Foo {\n  static async *[\"prototype\"]() {\n  }\n}\n")
	expectPrinted(t, "({ prototype: 1 })", "({prototype: 1});\n")
	expectPrinted(t, "({ get prototype() {} })", "({get prototype() {\n}});\n")
	expectPrinted(t, "({ set prototype(x) {} })", "({set prototype(x) {\n}});\n")
	expectPrinted(t, "({ *prototype() {} })", "({*prototype() {\n}});\n")
	expectPrinted(t, "({ async prototype() {} })", "({async prototype() {\n}});\n")
	expectPrinted(t, "({ async* prototype() {} })", "({async *prototype() {\n}});\n")
}

func TestSuperCall(t *testing.T) {
	expectParseError(t, "super()", "<stdin>: error: Unexpected \"(\"\n")
	expectParseError(t, "class Foo { foo = super() }", "<stdin>: error: Unexpected \"(\"\n")
	expectParseError(t, "class Foo { foo() { super() } }", "<stdin>: error: Unexpected \"(\"\n")
	expectParseError(t, "class Foo extends Bar { foo = super() }", "<stdin>: error: Unexpected \"(\"\n")
	expectParseError(t, "class Foo extends Bar { foo() { super() } }", "<stdin>: error: Unexpected \"(\"\n")
	expectParseError(t, "class Foo extends Bar { static constructor() { super() } }", "<stdin>: error: Unexpected \"(\"\n")
	expectParseError(t, "class Foo extends Bar { constructor() { function foo() { super() } } }", "<stdin>: error: Unexpected \"(\"\n")
	expectPrinted(t, "class Foo extends Bar { constructor() { super() } }",
		"class Foo extends Bar {\n  constructor() {\n    super();\n  }\n}\n")
	expectPrinted(t, "class Foo extends Bar { constructor() { () => super() } }",
		"class Foo extends Bar {\n  constructor() {\n    () => super();\n  }\n}\n")
	expectPrinted(t, "class Foo extends Bar { constructor() { () => { super() } } }",
		"class Foo extends Bar {\n  constructor() {\n    () => {\n      super();\n    };\n  }\n}\n")
}

func TestClassFields(t *testing.T) {
	expectPrinted(t, "class Foo { a }", "class Foo {\n  a;\n}\n")
	expectPrinted(t, "class Foo { a = 1 }", "class Foo {\n  a = 1;\n}\n")
	expectPrinted(t, "class Foo { a = 1; b }", "class Foo {\n  a = 1;\n  b;\n}\n")
	expectParseError(t, "class Foo { a = 1 b }", "<stdin>: error: Expected \";\" but found \"b\"\n")

	expectPrinted(t, "class Foo { [a] }", "class Foo {\n  [a];\n}\n")
	expectPrinted(t, "class Foo { [a] = 1 }", "class Foo {\n  [a] = 1;\n}\n")
	expectPrinted(t, "class Foo { [a] = 1; [b] }", "class Foo {\n  [a] = 1;\n  [b];\n}\n")
	expectParseError(t, "class Foo { [a] = 1 b }", "<stdin>: error: Expected \";\" but found \"b\"\n")

	expectPrinted(t, "class Foo { static a }", "class Foo {\n  static a;\n}\n")
	expectPrinted(t, "class Foo { static a = 1 }", "class Foo {\n  static a = 1;\n}\n")
	expectPrinted(t, "class Foo { static a = 1; b }", "class Foo {\n  static a = 1;\n  b;\n}\n")
	expectParseError(t, "class Foo { static a = 1 b }", "<stdin>: error: Expected \";\" but found \"b\"\n")

	expectPrinted(t, "class Foo { static [a] }", "class Foo {\n  static [a];\n}\n")
	expectPrinted(t, "class Foo { static [a] = 1 }", "class Foo {\n  static [a] = 1;\n}\n")
	expectPrinted(t, "class Foo { static [a] = 1; [b] }", "class Foo {\n  static [a] = 1;\n  [b];\n}\n")
	expectParseError(t, "class Foo { static [a] = 1 b }", "<stdin>: error: Expected \";\" but found \"b\"\n")

	expectParseError(t, "class Foo { get a }", "<stdin>: error: Expected \"(\" but found \"}\"\n")
	expectParseError(t, "class Foo { set a }", "<stdin>: error: Expected \"(\" but found \"}\"\n")
	expectParseError(t, "class Foo { async a }", "<stdin>: error: Expected \"(\" but found \"}\"\n")

	expectParseError(t, "class Foo { get a = 1 }", "<stdin>: error: Expected \"(\" but found \"=\"\n")
	expectParseError(t, "class Foo { set a = 1 }", "<stdin>: error: Expected \"(\" but found \"=\"\n")
	expectParseError(t, "class Foo { async a = 1 }", "<stdin>: error: Expected \"(\" but found \"=\"\n")

	expectParseError(t, "class Foo { `a` = 0 }", "<stdin>: error: Expected identifier but found \"`a`\"\n")

	// The name "constructor" is forbidden
	expectParseError(t, "class Foo { constructor }", "<stdin>: error: Invalid field name \"constructor\"\n")
	expectParseError(t, "class Foo { 'constructor' }", "<stdin>: error: Invalid field name \"constructor\"\n")
	expectParseError(t, "class Foo { constructor = 1 }", "<stdin>: error: Invalid field name \"constructor\"\n")
	expectParseError(t, "class Foo { 'constructor' = 1 }", "<stdin>: error: Invalid field name \"constructor\"\n")
	expectParseError(t, "class Foo { static constructor }", "<stdin>: error: Invalid field name \"constructor\"\n")
	expectParseError(t, "class Foo { static 'constructor' }", "<stdin>: error: Invalid field name \"constructor\"\n")
	expectParseError(t, "class Foo { static constructor = 1 }", "<stdin>: error: Invalid field name \"constructor\"\n")
	expectParseError(t, "class Foo { static 'constructor' = 1 }", "<stdin>: error: Invalid field name \"constructor\"\n")

	expectPrinted(t, "class Foo { ['constructor'] }", "class Foo {\n  [\"constructor\"];\n}\n")
	expectPrinted(t, "class Foo { ['constructor'] = 1 }", "class Foo {\n  [\"constructor\"] = 1;\n}\n")
	expectPrinted(t, "class Foo { static ['constructor'] }", "class Foo {\n  static [\"constructor\"];\n}\n")
	expectPrinted(t, "class Foo { static ['constructor'] = 1 }", "class Foo {\n  static [\"constructor\"] = 1;\n}\n")

	// The name "prototype" is sometimes forbidden
	expectPrinted(t, "class Foo { prototype }", "class Foo {\n  prototype;\n}\n")
	expectPrinted(t, "class Foo { 'prototype' }", "class Foo {\n  prototype;\n}\n")
	expectPrinted(t, "class Foo { prototype = 1 }", "class Foo {\n  prototype = 1;\n}\n")
	expectPrinted(t, "class Foo { 'prototype' = 1 }", "class Foo {\n  prototype = 1;\n}\n")
	expectParseError(t, "class Foo { static prototype }", "<stdin>: error: Invalid field name \"prototype\"\n")
	expectParseError(t, "class Foo { static 'prototype' }", "<stdin>: error: Invalid field name \"prototype\"\n")
	expectParseError(t, "class Foo { static prototype = 1 }", "<stdin>: error: Invalid field name \"prototype\"\n")
	expectParseError(t, "class Foo { static 'prototype' = 1 }", "<stdin>: error: Invalid field name \"prototype\"\n")
	expectPrinted(t, "class Foo { static ['prototype'] }", "class Foo {\n  static [\"prototype\"];\n}\n")
	expectPrinted(t, "class Foo { static ['prototype'] = 1 }", "class Foo {\n  static [\"prototype\"] = 1;\n}\n")
}

func TestGenerator(t *testing.T) {
	expectParseError(t, "(class { * foo })", "<stdin>: error: Expected \"(\" but found \"}\"\n")
	expectParseError(t, "(class { * *foo() {} })", "<stdin>: error: Unexpected \"*\"\n")
	expectParseError(t, "(class { get*foo() {} })", "<stdin>: error: Unexpected \"*\"\n")
	expectParseError(t, "(class { set*foo() {} })", "<stdin>: error: Unexpected \"*\"\n")
	expectParseError(t, "(class { *get foo() {} })", "<stdin>: error: Expected \"(\" but found \"foo\"\n")
	expectParseError(t, "(class { *set foo() {} })", "<stdin>: error: Expected \"(\" but found \"foo\"\n")
	expectParseError(t, "(class { *static foo() {} })", "<stdin>: error: Expected \"(\" but found \"foo\"\n")

	expectParseError(t, "function* foo() { -yield 100 }", "<stdin>: error: Cannot use a \"yield\" expression here without parentheses\n")
	expectPrinted(t, "function* foo() { -(yield 100) }", "function* foo() {\n  -(yield 100);\n}\n")
}

func TestYield(t *testing.T) {
	expectParseError(t, "yield 100", "<stdin>: error: Cannot use \"yield\" outside a generator function\n")
	expectParseError(t, "-yield 100", "<stdin>: error: Cannot use \"yield\" outside a generator function\n")
	expectPrinted(t, "yield\n100", "yield;\n100;\n")

	expectParseError(t, "function* bar(x = yield y) {}", "<stdin>: error: Cannot use \"yield\" outside a generator function\n")
	expectParseError(t, "(function*(x = yield y) {})", "<stdin>: error: Cannot use \"yield\" outside a generator function\n")
	expectParseError(t, "({ *foo(x = yield y) {} })", "<stdin>: error: Cannot use \"yield\" outside a generator function\n")
	expectParseError(t, "class Foo { *foo(x = yield y) {} }", "<stdin>: error: Cannot use \"yield\" outside a generator function\n")
	expectParseError(t, "(class { *foo(x = yield y) {} })", "<stdin>: error: Cannot use \"yield\" outside a generator function\n")

	expectParseError(t, "function *foo() { function bar(x = yield y) {} }", "<stdin>: error: Cannot use \"yield\" outside a generator function\n")
	expectParseError(t, "function *foo() { (function(x = yield y) {}) }", "<stdin>: error: Cannot use \"yield\" outside a generator function\n")
	expectParseError(t, "function *foo() { ({ foo(x = yield y) {} }) }", "<stdin>: error: Cannot use \"yield\" outside a generator function\n")
	expectParseError(t, "function *foo() { class Foo { foo(x = yield y) {} } }", "<stdin>: error: Cannot use \"yield\" outside a generator function\n")
	expectParseError(t, "function *foo() { (class { foo(x = yield y) {} }) }", "<stdin>: error: Cannot use \"yield\" outside a generator function\n")
	expectParseError(t, "function *foo() { (x = yield y) => {} }", "<stdin>: error: Cannot use a \"yield\" expression here\n")
	expectPrinted(t, "function *foo() { (x = yield y) }", "function* foo() {\n  x = yield y;\n}\n")
	expectParseError(t, "function foo() { (x = yield y) }", "<stdin>: error: Cannot use \"yield\" outside a generator function\n")
	expectParseError(t, "function *foo() { (x = \\u0079ield) }", "<stdin>: error: The keyword \"yield\" cannot be escaped\n")
}

func TestAsync(t *testing.T) {
	expectPrinted(t, "function foo() { await }", "function foo() {\n  await;\n}\n")
	expectPrinted(t, "async function foo() { await 0 }", "async function foo() {\n  await 0;\n}\n")
	expectParseError(t, "async function() {}", "<stdin>: error: Expected identifier but found \"(\"\n")

	expectPrinted(t, "-async function foo() { await 0 }", "-async function foo() {\n  await 0;\n};\n")
	expectPrinted(t, "-async function() { await 0 }", "-async function() {\n  await 0;\n};\n")
	expectPrinted(t, "1 - async function foo() { await 0 }", "1 - async function foo() {\n  await 0;\n};\n")
	expectPrinted(t, "1 - async function() { await 0 }", "1 - async function() {\n  await 0;\n};\n")
	expectPrinted(t, "(async function foo() { await 0 })", "(async function foo() {\n  await 0;\n});\n")
	expectPrinted(t, "(async function() { await 0 })", "(async function() {\n  await 0;\n});\n")
	expectPrinted(t, "(x, async function foo() { await 0 })", "x, async function foo() {\n  await 0;\n};\n")
	expectPrinted(t, "(x, async function() { await 0 })", "x, async function() {\n  await 0;\n};\n")

	expectPrinted(t, "async", "async;\n")
	expectPrinted(t, "async + 1", "async + 1;\n")
	expectPrinted(t, "async => {}", "(async) => {\n};\n")
	expectPrinted(t, "(async, 1)", "async, 1;\n")
	expectPrinted(t, "(async, x) => {}", "(async, x) => {\n};\n")
	expectPrinted(t, "async ()", "async();\n")
	expectPrinted(t, "async (x)", "async(x);\n")
	expectPrinted(t, "async (...x)", "async(...x);\n")
	expectPrinted(t, "async (...x, ...y)", "async(...x, ...y);\n")
	expectPrinted(t, "async () => {}", "async () => {\n};\n")
	expectPrinted(t, "async x => {}", "async (x) => {\n};\n")
	expectPrinted(t, "async (x) => {}", "async (x) => {\n};\n")
	expectPrinted(t, "async (...x) => {}", "async (...x) => {\n};\n")
	expectPrinted(t, "async x => await 0", "async (x) => await 0;\n")
	expectPrinted(t, "async () => await 0", "async () => await 0;\n")
	expectParseError(t, "async x;", "<stdin>: error: Expected \"=>\" but found \";\"\n")
	expectParseError(t, "async (...x,) => {}", "<stdin>: error: Unexpected \",\" after rest pattern\n")
	expectParseError(t, "async => await 0", "<stdin>: error: Expected \";\" but found \"0\"\n")

	expectPrinted(t, "(async x => y), z", "async (x) => y, z;\n")
	expectPrinted(t, "(async x => y, z)", "async (x) => y, z;\n")
	expectPrinted(t, "(async x => (y, z))", "async (x) => (y, z);\n")
	expectPrinted(t, "(async (x) => y), z", "async (x) => y, z;\n")
	expectPrinted(t, "(async (x) => y, z)", "async (x) => y, z;\n")
	expectPrinted(t, "(async (x) => (y, z))", "async (x) => (y, z);\n")
	expectPrinted(t, "async x => y, z", "async (x) => y, z;\n")
	expectPrinted(t, "async x => (y, z)", "async (x) => (y, z);\n")
	expectPrinted(t, "async (x) => y, z", "async (x) => y, z;\n")
	expectPrinted(t, "async (x) => (y, z)", "async (x) => (y, z);\n")
	expectPrinted(t, "export default async x => (y, z)", "export default async (x) => (y, z);\n")
	expectPrinted(t, "export default async (x) => (y, z)", "export default async (x) => (y, z);\n")
	expectParseError(t, "export default async x => y, z", "<stdin>: error: Expected \";\" but found \",\"\n")
	expectParseError(t, "export default async (x) => y, z", "<stdin>: error: Expected \";\" but found \",\"\n")

	expectParseError(t, "async function bar(x = await y) {}", "<stdin>: error: Expected \")\" but found \"y\"\n")
	expectParseError(t, "async (function(x = await y) {})", "<stdin>: error: Expected \")\" but found \"y\"\n")
	expectParseError(t, "async ({ foo(x = await y) {} })", "<stdin>: error: Expected \")\" but found \"y\"\n")
	expectParseError(t, "class Foo { async foo(x = await y) {} }", "<stdin>: error: Expected \")\" but found \"y\"\n")
	expectParseError(t, "(class { async foo(x = await y) {} })", "<stdin>: error: Expected \")\" but found \"y\"\n")

	expectParseError(t, "async function foo() { function bar(x = await y) {} }", "<stdin>: error: Expected \")\" but found \"y\"\n")
	expectParseError(t, "async function foo() { (function(x = await y) {}) }", "<stdin>: error: Expected \")\" but found \"y\"\n")
	expectParseError(t, "async function foo() { ({ foo(x = await y) {} }) }", "<stdin>: error: Expected \")\" but found \"y\"\n")
	expectParseError(t, "async function foo() { class Foo { foo(x = await y) {} } }", "<stdin>: error: Expected \")\" but found \"y\"\n")
	expectParseError(t, "async function foo() { (class { foo(x = await y) {} }) }", "<stdin>: error: Expected \")\" but found \"y\"\n")
	expectParseError(t, "async function foo() { (x = await y) => {} }", "<stdin>: error: Cannot use an \"await\" expression here\n")
	expectPrinted(t, "async function foo() { (x = await y) }", "async function foo() {\n  x = await y;\n}\n")
	expectParseError(t, "function foo() { (x = await y) }", "<stdin>: error: Expected \")\" but found \"y\"\n")

	// Top-level await
	expectPrinted(t, "await foo;", "await foo;\n")
	expectPrinted(t, "for await(foo of bar);", "for await (foo of bar)\n  ;\n")
	expectParseError(t, "function foo() { await foo }", "<stdin>: error: Expected \";\" but found \"foo\"\n")
	expectParseError(t, "function foo() { for await(foo of bar); }", "<stdin>: error: Cannot use \"await\" outside an async function\n")
	expectPrinted(t, "function foo(x = await) {}", "function foo(x = await) {\n}\n")
	expectParseError(t, "function foo(x = await y) {}", "<stdin>: error: Expected \")\" but found \"y\"\n")
	expectPrinted(t, "(function(x = await) {})", "(function(x = await) {\n});\n")
	expectParseError(t, "(function(x = await y) {})", "<stdin>: error: Expected \")\" but found \"y\"\n")
	expectPrinted(t, "({ foo(x = await) {} })", "({foo(x = await) {\n}});\n")
	expectParseError(t, "({ foo(x = await y) {} })", "<stdin>: error: Expected \")\" but found \"y\"\n")
	expectPrinted(t, "class Foo { foo(x = await) {} }", "class Foo {\n  foo(x = await) {\n  }\n}\n")
	expectParseError(t, "class Foo { foo(x = await y) {} }", "<stdin>: error: Expected \")\" but found \"y\"\n")
	expectPrinted(t, "(class { foo(x = await) {} })", "(class {\n  foo(x = await) {\n  }\n});\n")
	expectParseError(t, "(class { foo(x = await y) {} })", "<stdin>: error: Expected \")\" but found \"y\"\n")
	expectParseError(t, "(x = await) => {}", "<stdin>: error: Unexpected \")\"\n")
	expectParseError(t, "(x = await y) => {}", "<stdin>: error: Cannot use an \"await\" expression here\n")
	expectParseError(t, "(x = await)", "<stdin>: error: Unexpected \")\"\n")
	expectPrinted(t, "(x = await y)", "x = await y;\n")
	expectParseError(t, "async (x = await) => {}", "<stdin>: error: Unexpected \")\"\n")
	expectParseError(t, "async (x = await y) => {}", "<stdin>: error: Cannot use an \"await\" expression here\n")
	expectPrinted(t, "async(x = await y)", "async(x = await y);\n")

	// Keywords with escapes
	expectPrinted(t, "\\u0061sync", "async;\n")
	expectPrinted(t, "(\\u0061sync)", "async;\n")
	expectPrinted(t, "function foo() { \\u0061wait }", "function foo() {\n  await;\n}\n")
	expectPrinted(t, "function foo() { var \\u0061wait }", "function foo() {\n  var await;\n}\n")
	expectParseError(t, "\\u0061wait", "<stdin>: error: The keyword \"await\" cannot be escaped\n")
	expectParseError(t, "var \\u0061wait", "<stdin>: error: Cannot use \"await\" as an identifier here\n")
	expectParseError(t, "async function foo() { \\u0061wait }", "<stdin>: error: The keyword \"await\" cannot be escaped\n")
	expectParseError(t, "async function foo() { var \\u0061wait }", "<stdin>: error: Cannot use \"await\" as an identifier here\n")
	expectParseError(t, "\\u0061sync x => {}", "<stdin>: error: Expected \";\" but found \"x\"\n")
	expectParseError(t, "\\u0061sync () => {}", "<stdin>: error: Expected \";\" but found \"=>\"\n")
	expectParseError(t, "\\u0061sync function foo() {}", "<stdin>: error: Expected \";\" but found \"function\"\n")
	expectParseError(t, "({ \\u0061sync foo() {} })", "<stdin>: error: Expected \"}\" but found \"foo\"\n")
	expectParseError(t, "({ \\u0061sync *foo() {} })", "<stdin>: error: Expected \"}\" but found \"*\"\n")

	// For-await
	expectParseError(t, "for await(;;);", "<stdin>: error: Unexpected \";\"\n")
	expectParseError(t, "for await(x in y);", "<stdin>: error: Expected \"of\" but found \"in\"\n")
	expectParseError(t, "async function foo(){for await(;;);}", "<stdin>: error: Unexpected \";\"\n")
	expectParseError(t, "async function foo(){for await(let x;;);}", "<stdin>: error: Expected \"of\" but found \";\"\n")
	expectPrinted(t, "async function foo(){for await(x of y);}", "async function foo() {\n  for await (x of y)\n    ;\n}\n")
	expectPrinted(t, "async function foo(){for await(let x of y);}", "async function foo() {\n  for await (let x of y)\n    ;\n}\n")
}

func TestLabels(t *testing.T) {
	expectPrinted(t, "{a:b}", "{\n  a:\n    b;\n}\n")
	expectPrinted(t, "({a:b})", "({a: b});\n")

	expectParseError(t, "while (1) break x", "<stdin>: error: There is no containing label named \"x\"\n")
	expectParseError(t, "while (1) continue x", "<stdin>: error: There is no containing label named \"x\"\n")
}

func TestArrow(t *testing.T) {
	expectParseError(t, "({a: b, c() {}}) => {}", "<stdin>: error: Invalid binding pattern\n")
	expectParseError(t, "({a: b, get c() {}}) => {}", "<stdin>: error: Invalid binding pattern\n")
	expectParseError(t, "({a: b, set c(x) {}}) => {}", "<stdin>: error: Invalid binding pattern\n")

	expectPrinted(t, "x => function() {}", "(x) => function() {\n};\n")
	expectPrinted(t, "(x) => function() {}", "(x) => function() {\n};\n")
	expectPrinted(t, "(x => function() {})", "(x) => function() {\n};\n")

	expectPrinted(t, "(x = () => {}) => {}", "(x = () => {\n}) => {\n};\n")
	expectPrinted(t, "async (x = () => {}) => {}", "async (x = () => {\n}) => {\n};\n")

	expectParseError(t, "()\n=> {}", "<stdin>: error: Unexpected newline before \"=>\"\n")
	expectParseError(t, "x\n=> {}", "<stdin>: error: Unexpected newline before \"=>\"\n")
	expectParseError(t, "async x\n=> {}", "<stdin>: error: Unexpected newline before \"=>\"\n")
	expectParseError(t, "async ()\n=> {}", "<stdin>: error: Unexpected newline before \"=>\"\n")
	expectParseError(t, "(()\n=> {})", "<stdin>: error: Unexpected newline before \"=>\"\n")
	expectParseError(t, "(x\n=> {})", "<stdin>: error: Unexpected newline before \"=>\"\n")
	expectParseError(t, "(async x\n=> {})", "<stdin>: error: Unexpected newline before \"=>\"\n")
	expectParseError(t, "(async ()\n=> {})", "<stdin>: error: Unexpected newline before \"=>\"\n")

	expectPrinted(t, "(() => {}) ? 1 : 2", "1;\n")
	expectParseError(t, "() => {} ? 1 : 2", "<stdin>: error: Expected \";\" but found \"?\"\n")
	expectPrinted(t, "1 < (() => {})", "1 < (() => {\n});\n")
	expectParseError(t, "1 < () => {}", "<stdin>: error: Unexpected \")\"\n")
	expectParseError(t, "(...x = y) => {}", "<stdin>: error: A rest argument cannot have a default initializer\n")
}

func TestTemplate(t *testing.T) {
	expectPrinted(t, "`a${1 + `b${2}c` + 3}d`", "`a${1 + `b${2}c` + 3}d`;\n")

	expectPrinted(t, "`a\nb`", "`a\nb`;\n")
	expectPrinted(t, "`a\rb`", "`a\nb`;\n")
	expectPrinted(t, "`a\r\nb`", "`a\nb`;\n")
	expectPrinted(t, "`a\\nb`", "`a\nb`;\n")
	expectPrinted(t, "`a\\rb`", "`a\\rb`;\n")
	expectPrinted(t, "`a\\r\\nb`", "`a\\r\nb`;\n")
	expectPrinted(t, "`a\u2028b`", "`a\\u2028b`;\n")
	expectPrinted(t, "`a\u2029b`", "`a\\u2029b`;\n")

	expectPrinted(t, "`a\n${b}`", "`a\n${b}`;\n")
	expectPrinted(t, "`a\r${b}`", "`a\n${b}`;\n")
	expectPrinted(t, "`a\r\n${b}`", "`a\n${b}`;\n")
	expectPrinted(t, "`a\\n${b}`", "`a\n${b}`;\n")
	expectPrinted(t, "`a\\r${b}`", "`a\\r${b}`;\n")
	expectPrinted(t, "`a\\r\\n${b}`", "`a\\r\n${b}`;\n")
	expectPrinted(t, "`a\u2028${b}`", "`a\\u2028${b}`;\n")
	expectPrinted(t, "`a\u2029${b}`", "`a\\u2029${b}`;\n")

	expectPrinted(t, "`${a}\nb`", "`${a}\nb`;\n")
	expectPrinted(t, "`${a}\rb`", "`${a}\nb`;\n")
	expectPrinted(t, "`${a}\r\nb`", "`${a}\nb`;\n")
	expectPrinted(t, "`${a}\\nb`", "`${a}\nb`;\n")
	expectPrinted(t, "`${a}\\rb`", "`${a}\\rb`;\n")
	expectPrinted(t, "`${a}\\r\\nb`", "`${a}\\r\nb`;\n")
	expectPrinted(t, "`${a}\u2028b`", "`${a}\\u2028b`;\n")
	expectPrinted(t, "`${a}\u2029b`", "`${a}\\u2029b`;\n")

	expectPrinted(t, "tag`a\nb`", "tag`a\nb`;\n")
	expectPrinted(t, "tag`a\rb`", "tag`a\nb`;\n")
	expectPrinted(t, "tag`a\r\nb`", "tag`a\nb`;\n")
	expectPrinted(t, "tag`a\\nb`", "tag`a\\nb`;\n")
	expectPrinted(t, "tag`a\\rb`", "tag`a\\rb`;\n")
	expectPrinted(t, "tag`a\\r\\nb`", "tag`a\\r\\nb`;\n")
	expectPrinted(t, "tag`a\u2028b`", "tag`a\u2028b`;\n")
	expectPrinted(t, "tag`a\u2029b`", "tag`a\u2029b`;\n")

	expectPrinted(t, "tag`a\n${b}`", "tag`a\n${b}`;\n")
	expectPrinted(t, "tag`a\r${b}`", "tag`a\n${b}`;\n")
	expectPrinted(t, "tag`a\r\n${b}`", "tag`a\n${b}`;\n")
	expectPrinted(t, "tag`a\\n${b}`", "tag`a\\n${b}`;\n")
	expectPrinted(t, "tag`a\\r${b}`", "tag`a\\r${b}`;\n")
	expectPrinted(t, "tag`a\\r\\n${b}`", "tag`a\\r\\n${b}`;\n")
	expectPrinted(t, "tag`a\u2028${b}`", "tag`a\u2028${b}`;\n")
	expectPrinted(t, "tag`a\u2029${b}`", "tag`a\u2029${b}`;\n")

	expectPrinted(t, "tag`${a}\nb`", "tag`${a}\nb`;\n")
	expectPrinted(t, "tag`${a}\rb`", "tag`${a}\nb`;\n")
	expectPrinted(t, "tag`${a}\r\nb`", "tag`${a}\nb`;\n")
	expectPrinted(t, "tag`${a}\\nb`", "tag`${a}\\nb`;\n")
	expectPrinted(t, "tag`${a}\\rb`", "tag`${a}\\rb`;\n")
	expectPrinted(t, "tag`${a}\\r\\nb`", "tag`${a}\\r\\nb`;\n")
	expectPrinted(t, "tag`${a}\u2028b`", "tag`${a}\u2028b`;\n")
	expectPrinted(t, "tag`${a}\u2029b`", "tag`${a}\u2029b`;\n")
}

func TestSwitch(t *testing.T) {
	expectPrinted(t, "switch (x) { default: }", "switch (x) {\n  default:\n}\n")
	expectPrinted(t, "switch ((x => x + 1)(0)) { case 1: var y } y = 2", "switch (((x) => x + 1)(0)) {\n  case 1:\n    var y;\n}\ny = 2;\n")
	expectParseError(t, "switch (x) { default: default: }", "<stdin>: error: Multiple default clauses are not allowed\n")
}

func TestConstantFolding(t *testing.T) {
	expectPrinted(t, "!false", "true;\n")
	expectPrinted(t, "!true", "false;\n")

	expectPrinted(t, "!!0", "false;\n")
	expectPrinted(t, "!!-0", "false;\n")
	expectPrinted(t, "!!1", "true;\n")
	expectPrinted(t, "!!NaN", "false;\n")
	expectPrinted(t, "!!Infinity", "true;\n")
	expectPrinted(t, "!!-Infinity", "true;\n")
	expectPrinted(t, "!!\"\"", "false;\n")
	expectPrinted(t, "!!\"x\"", "true;\n")
	expectPrinted(t, "!!function() {}", "true;\n")
	expectPrinted(t, "!!(() => {})", "true;\n")
	expectPrinted(t, "!!0n", "false;\n")
	expectPrinted(t, "!!1n", "true;\n")

	expectPrinted(t, "1 ? 2 : 3", "2;\n")
	expectPrinted(t, "0 ? 1 : 2", "2;\n")
	expectPrinted(t, "1 && 2", "2;\n")
	expectPrinted(t, "1 || 2", "1;\n")
	expectPrinted(t, "0 && 1", "0;\n")
	expectPrinted(t, "0 || 1", "1;\n")

	expectPrinted(t, "null ?? 1", "1;\n")
	expectPrinted(t, "undefined ?? 1", "1;\n")
	expectPrinted(t, "0 ?? 1", "0;\n")
	expectPrinted(t, "false ?? 1", "false;\n")
	expectPrinted(t, "\"\" ?? 1", "\"\";\n")

	expectPrinted(t, "typeof undefined", "\"undefined\";\n")
	expectPrinted(t, "typeof null", "\"object\";\n")
	expectPrinted(t, "typeof false", "\"boolean\";\n")
	expectPrinted(t, "typeof true", "\"boolean\";\n")
	expectPrinted(t, "typeof 123", "\"number\";\n")
	expectPrinted(t, "typeof 123n", "\"bigint\";\n")
	expectPrinted(t, "typeof 'abc'", "\"string\";\n")
	expectPrinted(t, "typeof function() {}", "\"function\";\n")
	expectPrinted(t, "typeof (() => {})", "\"function\";\n")
	expectPrinted(t, "typeof {}", "typeof {};\n")
	expectPrinted(t, "typeof []", "typeof [];\n")

	expectPrinted(t, "undefined === undefined", "true;\n")
	expectPrinted(t, "undefined !== undefined", "false;\n")
	expectPrinted(t, "undefined == undefined", "true;\n")
	expectPrinted(t, "undefined != undefined", "false;\n")

	expectPrinted(t, "null === null", "true;\n")
	expectPrinted(t, "null !== null", "false;\n")
	expectPrinted(t, "null == null", "true;\n")
	expectPrinted(t, "null != null", "false;\n")

	expectPrinted(t, "undefined === null", "void 0 === null;\n")
	expectPrinted(t, "undefined !== null", "void 0 !== null;\n")
	expectPrinted(t, "undefined == null", "void 0 == null;\n")
	expectPrinted(t, "undefined != null", "void 0 != null;\n")

	expectPrinted(t, "true === true", "true;\n")
	expectPrinted(t, "true === false", "false;\n")
	expectPrinted(t, "true !== true", "false;\n")
	expectPrinted(t, "true !== false", "true;\n")
	expectPrinted(t, "true == true", "true;\n")
	expectPrinted(t, "true == false", "false;\n")
	expectPrinted(t, "true != true", "false;\n")
	expectPrinted(t, "true != false", "true;\n")

	expectPrinted(t, "1 === 1", "true;\n")
	expectPrinted(t, "1 === 2", "false;\n")
	expectPrinted(t, "1 === '1'", "1 === \"1\";\n")
	expectPrinted(t, "1 == 1", "true;\n")
	expectPrinted(t, "1 == 2", "false;\n")
	expectPrinted(t, "1 == '1'", "1 == \"1\";\n")

	expectPrinted(t, "1 !== 1", "false;\n")
	expectPrinted(t, "1 !== 2", "true;\n")
	expectPrinted(t, "1 !== '1'", "1 !== \"1\";\n")
	expectPrinted(t, "1 != 1", "false;\n")
	expectPrinted(t, "1 != 2", "true;\n")
	expectPrinted(t, "1 != '1'", "1 != \"1\";\n")

	expectPrinted(t, "'a' === '\\x61'", "true;\n")
	expectPrinted(t, "'a' === '\\x62'", "false;\n")
	expectPrinted(t, "'a' === 'abc'", "false;\n")
	expectPrinted(t, "'a' !== '\\x61'", "false;\n")
	expectPrinted(t, "'a' !== '\\x62'", "true;\n")
	expectPrinted(t, "'a' !== 'abc'", "true;\n")
	expectPrinted(t, "'a' == '\\x61'", "true;\n")
	expectPrinted(t, "'a' == '\\x62'", "false;\n")
	expectPrinted(t, "'a' == 'abc'", "false;\n")
	expectPrinted(t, "'a' != '\\x61'", "false;\n")
	expectPrinted(t, "'a' != '\\x62'", "true;\n")
	expectPrinted(t, "'a' != 'abc'", "true;\n")

	expectPrinted(t, "'a' + 'b'", "\"ab\";\n")
	expectPrinted(t, "'a' + 'bc'", "\"abc\";\n")
	expectPrinted(t, "'ab' + 'c'", "\"abc\";\n")
	expectPrinted(t, "x + 'a' + 'b'", "x + \"ab\";\n")
	expectPrinted(t, "x + 'a' + 'bc'", "x + \"abc\";\n")
	expectPrinted(t, "x + 'ab' + 'c'", "x + \"abc\";\n")
	expectPrinted(t, "'a' + 1", "\"a\" + 1;\n")
	expectPrinted(t, "x * 'a' + 'b'", "x * \"a\" + \"b\";\n")

	expectPrinted(t, "'string' + `template`", "`stringtemplate`;\n")
	expectPrinted(t, "'string' + `a${foo}b`", "`stringa${foo}b`;\n")
	expectPrinted(t, "'string' + tag`template`", "\"string\" + tag`template`;\n")
	expectPrinted(t, "`template` + 'string'", "`templatestring`;\n")
	expectPrinted(t, "`a${foo}b` + 'string'", "`a${foo}bstring`;\n")
	expectPrinted(t, "tag`template` + 'string'", "tag`template` + \"string\";\n")
	expectPrinted(t, "`template` + `a${foo}b`", "`templatea${foo}b`;\n")
	expectPrinted(t, "`a${foo}b` + `template`", "`a${foo}btemplate`;\n")
	expectPrinted(t, "`a${foo}b` + `x${bar}y`", "`a${foo}bx${bar}y`;\n")
	expectPrinted(t, "`a${i}${j}bb` + `xxx${bar}yyyy`", "`a${i}${j}bbxxx${bar}yyyy`;\n")
	expectPrinted(t, "`a${foo}bb` + `xxx${i}${j}yyyy`", "`a${foo}bbxxx${i}${j}yyyy`;\n")
	expectPrinted(t, "`template` + tag`template2`", "`template` + tag`template2`;\n")
	expectPrinted(t, "tag`template` + `template2`", "tag`template` + `template2`;\n")

	expectPrinted(t, "123", "123;\n")
	expectPrinted(t, "123 .toString()", "123 .toString();\n")
	expectPrinted(t, "-123", "-123;\n")
	expectPrinted(t, "(-123).toString()", "(-123).toString();\n")
	expectPrinted(t, "-0", "-0;\n")
	expectPrinted(t, "(-0).toString()", "(-0).toString();\n")
	expectPrinted(t, "-0 === 0", "true;\n")

	expectPrinted(t, "NaN", "NaN;\n")
	expectPrinted(t, "NaN.toString()", "NaN.toString();\n")
	expectPrinted(t, "NaN === NaN", "false;\n")

	expectPrinted(t, "Infinity", "Infinity;\n")
	expectPrinted(t, "Infinity.toString()", "Infinity.toString();\n")
	expectPrinted(t, "(-Infinity).toString()", "(-Infinity).toString();\n")
	expectPrinted(t, "Infinity === Infinity", "true;\n")
	expectPrinted(t, "Infinity === -Infinity", "false;\n")

	expectPrinted(t, "123n === 1_2_3n", "true;\n")
}

func TestConstantFoldingScopes(t *testing.T) {
	// Parsing will crash if somehow the scope traversal is misaligned between
	// the parsing and binding passes. This checks for those cases.
	expectPrinted(t, "1 ? 0 : ()=>{}; (()=>{})()", "0;\n(() => {\n})();\n")
	expectPrinted(t, "0 ? ()=>{} : 1; (()=>{})()", "1;\n(() => {\n})();\n")
	expectPrinted(t, "0 && (()=>{}); (()=>{})()", "0;\n(() => {\n})();\n")
	expectPrinted(t, "1 || (()=>{}); (()=>{})()", "1;\n(() => {\n})();\n")
	expectPrintedMangle(t, "if (1) 0; else ()=>{}; (()=>{})()", "(() => {\n})();\n")
	expectPrintedMangle(t, "if (0) ()=>{}; else 1; (()=>{})()", "(() => {\n})();\n")
}

func TestImport(t *testing.T) {
	expectPrinted(t, "import \"foo\"", "import \"foo\";\n")
	expectPrinted(t, "import {} from \"foo\"", "import {} from \"foo\";\n")
	expectPrinted(t, "import {x} from \"foo\";x", "import {x} from \"foo\";\nx;\n")
	expectPrinted(t, "import {x as y} from \"foo\";y", "import {x as y} from \"foo\";\ny;\n")
	expectPrinted(t, "import {x as y, z} from \"foo\";y;z", "import {x as y, z} from \"foo\";\ny;\nz;\n")
	expectPrinted(t, "import {x as y, z,} from \"foo\";y;z", "import {x as y, z} from \"foo\";\ny;\nz;\n")
	expectPrinted(t, "import z, {x as y} from \"foo\";y;z", "import z, {x as y} from \"foo\";\ny;\nz;\n")
	expectPrinted(t, "import z from \"foo\";z", "import z from \"foo\";\nz;\n")
	expectPrinted(t, "import * as ns from \"foo\";ns;ns.x", "import * as ns from \"foo\";\nns;\nns.x;\n")
	expectPrinted(t, "import z, * as ns from \"foo\";z;ns;ns.x", "import z, * as ns from \"foo\";\nz;\nns;\nns.x;\n")

	expectParseError(t, "import * from \"foo\"", "<stdin>: error: Expected \"as\" but found \"from\"\n")

	expectPrinted(t, "import('foo')", "import(\"foo\");\n")
	expectPrinted(t, "(import('foo'))", "import(\"foo\");\n")
	expectPrinted(t, "{import('foo')}", "{\n  import(\"foo\");\n}\n")
	expectPrinted(t, "import('foo').then(() => {})", "import(\"foo\").then(() => {\n});\n")
	expectPrinted(t, "new import.meta", "new import.meta();\n")
	expectPrinted(t, "new (import('foo'))", "new (import(\"foo\"))();\n")
	expectParseError(t, "import()", "<stdin>: error: Unexpected \")\"\n")
	expectParseError(t, "import(...a)", "<stdin>: error: Unexpected \"...\"\n")
	expectParseError(t, "import(a, b)", "<stdin>: error: Expected \")\" but found \",\"\n")
	expectParseError(t, "new import('foo')", "<stdin>: error: Cannot use an \"import\" expression here without parentheses\n")

	expectPrinted(t, "import.meta", "import.meta;\n")
	expectPrinted(t, "(import.meta)", "import.meta;\n")
	expectPrinted(t, "{import.meta}", "{\n  import.meta;\n}\n")

	expectPrinted(t, "import x from \"foo\"; x = 1", "import x from \"foo\";\nx = 1;\n")
	expectPrinted(t, "import x from \"foo\"; x++", "import x from \"foo\";\nx++;\n")
	expectPrinted(t, "import x from \"foo\"; ([x] = 1)", "import x from \"foo\";\n[x] = 1;\n")
	expectPrinted(t, "import x from \"foo\"; ({x} = 1)", "import x from \"foo\";\n({x} = 1);\n")
	expectPrinted(t, "import x from \"foo\"; ({y: x} = 1)", "import x from \"foo\";\n({y: x} = 1);\n")
	expectPrinted(t, "import {x} from \"foo\"; x++", "import {x} from \"foo\";\nx++;\n")
	expectPrinted(t, "import * as x from \"foo\"; x++", "import * as x from \"foo\";\nx++;\n")
	expectPrinted(t, "import * as x from \"foo\"; x.y = 1", "import * as x from \"foo\";\nx.y = 1;\n")
	expectPrinted(t, "import * as x from \"foo\"; x[y] = 1", "import * as x from \"foo\";\nx[y] = 1;\n")
	expectPrinted(t, "import * as x from \"foo\"; x['y'] = 1", "import * as x from \"foo\";\nx[\"y\"] = 1;\n")
	expectPrinted(t, "import * as x from \"foo\"; x['y z'] = 1", "import * as x from \"foo\";\nx[\"y z\"] = 1;\n")
	expectPrinted(t, "import x from \"foo\"; ({y = x} = 1)", "import x from \"foo\";\n({y = x} = 1);\n")
	expectPrinted(t, "import x from \"foo\"; ({[x]: y} = 1)", "import x from \"foo\";\n({[x]: y} = 1);\n")
	expectPrinted(t, "import x from \"foo\"; x.y = 1", "import x from \"foo\";\nx.y = 1;\n")
	expectPrinted(t, "import x from \"foo\"; x[y] = 1", "import x from \"foo\";\nx[y] = 1;\n")
	expectPrinted(t, "import x from \"foo\"; x['y'] = 1", "import x from \"foo\";\nx[\"y\"] = 1;\n")

	// "eval" and "arguments" are forbidden import names
	expectParseError(t, "import {eval} from 'foo'", "<stdin>: error: Cannot use \"eval\" as an identifier here\n")
	expectParseError(t, "import {ev\\u0061l} from 'foo'", "<stdin>: error: Cannot use \"eval\" as an identifier here\n")
	expectParseError(t, "import {x as eval} from 'foo'", "<stdin>: error: Cannot use \"eval\" as an identifier here\n")
	expectParseError(t, "import {x as ev\\u0061l} from 'foo'", "<stdin>: error: Cannot use \"eval\" as an identifier here\n")
	expectPrinted(t, "import {eval as x} from 'foo'", "import {eval as x} from \"foo\";\n")
	expectPrinted(t, "import {ev\\u0061l as x} from 'foo'", "import {eval as x} from \"foo\";\n")
	expectParseError(t, "import {arguments} from 'foo'", "<stdin>: error: Cannot use \"arguments\" as an identifier here\n")
	expectParseError(t, "import {\\u0061rguments} from 'foo'", "<stdin>: error: Cannot use \"arguments\" as an identifier here\n")
	expectParseError(t, "import {x as arguments} from 'foo'", "<stdin>: error: Cannot use \"arguments\" as an identifier here\n")
	expectParseError(t, "import {x as \\u0061rguments} from 'foo'", "<stdin>: error: Cannot use \"arguments\" as an identifier here\n")
	expectPrinted(t, "import {arguments as x} from 'foo'", "import {arguments as x} from \"foo\";\n")
	expectPrinted(t, "import {\\u0061rguments as x} from 'foo'", "import {arguments as x} from \"foo\";\n")
}

func TestExport(t *testing.T) {
	expectPrinted(t, "export default x", "export default x;\n")
	expectPrinted(t, "export class x {}", "export class x {\n}\n")
	expectPrinted(t, "export function x() {}", "export function x() {\n}\n")
	expectPrinted(t, "export async function x() {}", "export async function x() {\n}\n")
	expectPrinted(t, "export var x, y", "export var x, y;\n")
	expectPrinted(t, "export let x, y", "export let x, y;\n")
	expectPrinted(t, "export const x = 0, y = 1", "export const x = 0, y = 1;\n")
	expectPrinted(t, "export * from \"foo\"", "export * from \"foo\";\n")
	expectPrinted(t, "export * as ns from \"foo\"", "export * as ns from \"foo\";\n")
	expectPrinted(t, "export * as if from \"foo\"", "export * as if from \"foo\";\n")
	expectPrinted(t, "let x; export {x}", "let x;\nexport {x};\n")
	expectPrinted(t, "let x; export {x as y}", "let x;\nexport {x as y};\n")
	expectPrinted(t, "let x, z; export {x as y, z}", "let x, z;\nexport {x as y, z};\n")
	expectPrinted(t, "let x, z; export {x as y, z,}", "let x, z;\nexport {x as y, z};\n")
	expectPrinted(t, "let x; export {x} from \"foo\"", "let x;\nexport {x} from \"foo\";\n")
	expectPrinted(t, "let x; export {x as y} from \"foo\"", "let x;\nexport {x as y} from \"foo\";\n")
	expectPrinted(t, "let x, z; export {x as y, z} from \"foo\"", "let x, z;\nexport {x as y, z} from \"foo\";\n")
	expectPrinted(t, "let x, z; export {x as y, z,} from \"foo\"", "let x, z;\nexport {x as y, z} from \"foo\";\n")

	expectParseError(t, "export x from \"foo\"", "<stdin>: error: Unexpected \"x\"\n")
	expectParseError(t, "export async", "<stdin>: error: Expected \"function\" but found end of file\n")
	expectParseError(t, "export async function", "<stdin>: error: Expected identifier but found end of file\n")
	expectParseError(t, "export async () => {}", "<stdin>: error: Expected \"function\" but found \"(\"\n")
}

func TestExportDuplicates(t *testing.T) {
	expectPrinted(t, "export {x};let x", "export {x};\nlet x;\n")
	expectPrinted(t, "export {x, x as y};let x", "export {x, x as y};\nlet x;\n")
	expectPrinted(t, "export {x};export {x as y} from 'foo';let x", "export {x};\nexport {x as y} from \"foo\";\nlet x;\n")
	expectPrinted(t, "export {x};export default function x() {}", "export {x};\nexport default function x() {\n}\n")
	expectPrinted(t, "export {x};export default class x {}", "export {x};\nexport default class x {\n}\n")

	errorTextX := `<stdin>: error: Multiple exports with the same name "x"
<stdin>: note: "x" was originally exported here
`

	expectParseError(t, "export {x, x};let x", errorTextX)
	expectParseError(t, "export {x, y as x};let x, y", errorTextX)
	expectParseError(t, "export {x};export function x() {}", errorTextX)
	expectParseError(t, "export {x};export class x {}", errorTextX)
	expectParseError(t, "export {x};export const x = 0", errorTextX)
	expectParseError(t, "export {x};export let x", errorTextX)
	expectParseError(t, "export {x};export var x", errorTextX)
	expectParseError(t, "export {x};let x;export {x} from 'foo'", errorTextX)
	expectParseError(t, "export {x};let x;export {y as x} from 'foo'", errorTextX)
	expectParseError(t, "export {x};let x;export * as x from 'foo'", errorTextX)

	errorTextDefault := `<stdin>: error: Multiple exports with the same name "default"
<stdin>: note: "default" was originally exported here
`

	expectParseError(t, "export {x as default};let x;export default 0", errorTextDefault)
	expectParseError(t, "export {x as default};let x;export default function() {}", errorTextDefault)
	expectParseError(t, "export {x as default};let x;export default class {}", errorTextDefault)
	expectParseError(t, "export {x as default};export default function x() {}", errorTextDefault)
	expectParseError(t, "export {x as default};export default class x {}", errorTextDefault)
}

func TestExportDefault(t *testing.T) {
	expectParseError(t, "export default 1, 2", "<stdin>: error: Expected \";\" but found \",\"\n")
	expectPrinted(t, "export default (1, 2)", "export default (1, 2);\n")

	expectParseError(t, "export default async, 0", "<stdin>: error: Expected \";\" but found \",\"\n")
	expectPrinted(t, "export default async", "export default async;\n")
	expectPrinted(t, "export default async()", "export default async();\n")
	expectPrinted(t, "export default async + 1", "export default async + 1;\n")
	expectPrinted(t, "export default async => {}", "export default (async) => {\n};\n")
	expectPrinted(t, "export default async x => {}", "export default async (x) => {\n};\n")
	expectPrinted(t, "export default async () => {}", "export default async () => {\n};\n")

	// This is a corner case in the ES6 grammar. The "export default" statement
	// normally takes an expression except for the function and class keywords
	// which behave sort of like their respective declarations instead.
	expectPrinted(t, "export default function() {} - after", "export default function() {\n}\n-after;\n")
	expectPrinted(t, "export default function*() {} - after", "export default function* () {\n}\n-after;\n")
	expectPrinted(t, "export default function foo() {} - after", "export default function foo() {\n}\n-after;\n")
	expectPrinted(t, "export default function* foo() {} - after", "export default function* foo() {\n}\n-after;\n")
	expectPrinted(t, "export default async function() {} - after", "export default async function() {\n}\n-after;\n")
	expectPrinted(t, "export default async function*() {} - after", "export default async function* () {\n}\n-after;\n")
	expectPrinted(t, "export default async function foo() {} - after", "export default async function foo() {\n}\n-after;\n")
	expectPrinted(t, "export default async function* foo() {} - after", "export default async function* foo() {\n}\n-after;\n")
	expectPrinted(t, "export default class {} - after", "export default class {\n}\n-after;\n")
	expectPrinted(t, "export default class Foo {} - after", "export default class Foo {\n}\n-after;\n")
}

func TestExportClause(t *testing.T) {
	expectPrinted(t, "export {x, y};let x, y", "export {x, y};\nlet x, y;\n")
	expectPrinted(t, "export {x, y as z,};let x, y", "export {x, y as z};\nlet x, y;\n")
	expectPrinted(t, "export {x, y} from 'path'", "export {x, y} from \"path\";\n")
	expectPrinted(t, "export {default, if} from 'path'", "export {default, if} from \"path\";\n")
	expectPrinted(t, "export {default as foo, if as bar} from 'path'", "export {default as foo, if as bar} from \"path\";\n")
	expectParseError(t, "export {default}", "<stdin>: error: Expected identifier but found \"default\"\n")
	expectParseError(t, "export {default as foo}", "<stdin>: error: Expected identifier but found \"default\"\n")
	expectParseError(t, "export {if}", "<stdin>: error: Expected identifier but found \"if\"\n")
	expectParseError(t, "export {if as foo}", "<stdin>: error: Expected identifier but found \"if\"\n")
}

func TestCatch(t *testing.T) {
	expectPrinted(t, "try {} catch (e) {}", "try {\n} catch (e) {\n}\n")
	expectPrinted(t, "try {} catch (e) { var e }", "try {\n} catch (e) {\n  var e;\n}\n")
	expectPrinted(t, "var e; try {} catch (e) {}", "var e;\ntry {\n} catch (e) {\n}\n")
	expectPrinted(t, "let e; try {} catch (e) {}", "let e;\ntry {\n} catch (e) {\n}\n")
	expectPrinted(t, "try { var e } catch (e) {}", "try {\n  var e;\n} catch (e) {\n}\n")
	expectPrinted(t, "try { function e() {} } catch (e) {}", "try {\n  function e() {\n  }\n} catch (e) {\n}\n")
	expectPrinted(t, "try {} catch (e) { { function e() {} } }", "try {\n} catch (e) {\n  {\n    function e() {\n    }\n  }\n}\n")
	expectPrinted(t, "try {} catch (e) { if (1) function e() {} }", "try {\n} catch (e) {\n  if (1)\n    function e() {\n    }\n}\n")
	expectPrinted(t, "try {} catch (e) { if (0) ; else function e() {} }", "try {\n} catch (e) {\n  if (0)\n    ;\n  else\n    function e() {\n    }\n}\n")

	errorText := `<stdin>: error: "e" has already been declared
<stdin>: note: "e" was originally declared here
`

	expectParseError(t, "try {} catch (e) { function e() {} }", errorText)
	expectParseError(t, "try {} catch ({e}) { var e }", errorText)
	expectParseError(t, "try {} catch ({e}) { function e() {} }", errorText)
	expectParseError(t, "try {} catch (e) { let e }", errorText)
	expectParseError(t, "try {} catch (e) { const e = 0 }", errorText)
}

func TestWarningEqualsNegativeZero(t *testing.T) {
	expectParseError(t, "x === -0", "<stdin>: warning: Comparison with -0 using the \"===\" operator will also match 0\n")
	expectParseError(t, "x == -0", "<stdin>: warning: Comparison with -0 using the \"==\" operator will also match 0\n")
	expectParseError(t, "x !== -0", "<stdin>: warning: Comparison with -0 using the \"!==\" operator will also match 0\n")
	expectParseError(t, "x != -0", "<stdin>: warning: Comparison with -0 using the \"!=\" operator will also match 0\n")
	expectParseError(t, "switch (x) { case -0: }", "<stdin>: warning: Comparison with -0 using a case clause will also match 0\n")

	expectParseError(t, "-0 === x", "<stdin>: warning: Comparison with -0 using the \"===\" operator will also match 0\n")
	expectParseError(t, "-0 == x", "<stdin>: warning: Comparison with -0 using the \"==\" operator will also match 0\n")
	expectParseError(t, "-0 !== x", "<stdin>: warning: Comparison with -0 using the \"!==\" operator will also match 0\n")
	expectParseError(t, "-0 != x", "<stdin>: warning: Comparison with -0 using the \"!=\" operator will also match 0\n")
	expectParseError(t, "switch (-0) { case x: }", "") // Don't bother to handle this case
}

func TestWarningEqualsNewObject(t *testing.T) {
	expectParseError(t, "x === []", "<stdin>: warning: Comparison using the \"===\" operator here is always false\n")
	expectParseError(t, "x !== []", "<stdin>: warning: Comparison using the \"!==\" operator here is always true\n")
	expectParseError(t, "x == []", "")
	expectParseError(t, "x != []", "")
	expectParseError(t, "switch (x) { case []: }", "<stdin>: warning: This case clause will never be evaluated because the comparison is always false\n")

	expectParseError(t, "[] === x", "<stdin>: warning: Comparison using the \"===\" operator here is always false\n")
	expectParseError(t, "[] !== x", "<stdin>: warning: Comparison using the \"!==\" operator here is always true\n")
	expectParseError(t, "[] == x", "")
	expectParseError(t, "[] != x", "")
	expectParseError(t, "switch ([]) { case x: }", "") // Don't bother to handle this case
}

func TestWarningEqualsNaN(t *testing.T) {
	expectParseError(t, "x === NaN", "<stdin>: warning: Comparison with NaN using the \"===\" operator here is always false\n")
	expectParseError(t, "x !== NaN", "<stdin>: warning: Comparison with NaN using the \"!==\" operator here is always true\n")
	expectParseError(t, "x == NaN", "<stdin>: warning: Comparison with NaN using the \"==\" operator here is always false\n")
	expectParseError(t, "x != NaN", "<stdin>: warning: Comparison with NaN using the \"!=\" operator here is always true\n")
	expectParseError(t, "switch (x) { case NaN: }", "<stdin>: warning: This case clause will never be evaluated because equality with NaN is always false\n")

	expectParseError(t, "NaN === x", "<stdin>: warning: Comparison with NaN using the \"===\" operator here is always false\n")
	expectParseError(t, "NaN !== x", "<stdin>: warning: Comparison with NaN using the \"!==\" operator here is always true\n")
	expectParseError(t, "NaN == x", "<stdin>: warning: Comparison with NaN using the \"==\" operator here is always false\n")
	expectParseError(t, "NaN != x", "<stdin>: warning: Comparison with NaN using the \"!=\" operator here is always true\n")
	expectParseError(t, "switch (NaN) { case x: }", "") // Don't bother to handle this case
}

func TestWarningTypeofEquals(t *testing.T) {
	expectParseError(t, "typeof x === 'null'", "<stdin>: warning: The \"typeof\" operator will never evaluate to \"null\"\n")
	expectParseError(t, "typeof x !== 'null'", "<stdin>: warning: The \"typeof\" operator will never evaluate to \"null\"\n")
	expectParseError(t, "typeof x == 'null'", "<stdin>: warning: The \"typeof\" operator will never evaluate to \"null\"\n")
	expectParseError(t, "typeof x != 'null'", "<stdin>: warning: The \"typeof\" operator will never evaluate to \"null\"\n")
	expectParseError(t, "switch (typeof x) { case 'null': }", "<stdin>: warning: The \"typeof\" operator will never evaluate to \"null\"\n")

	expectParseError(t, "'null' === typeof x", "<stdin>: warning: The \"typeof\" operator will never evaluate to \"null\"\n")
	expectParseError(t, "'null' !== typeof x", "<stdin>: warning: The \"typeof\" operator will never evaluate to \"null\"\n")
	expectParseError(t, "'null' == typeof x", "<stdin>: warning: The \"typeof\" operator will never evaluate to \"null\"\n")
	expectParseError(t, "'null' != typeof x", "<stdin>: warning: The \"typeof\" operator will never evaluate to \"null\"\n")
	expectParseError(t, "switch ('null') { case typeof x: }", "") // Don't bother to handle this case
}

func TestWarningDeleteSuperProperty(t *testing.T) {
	text := "<stdin>: warning: Attempting to delete a property of \"super\" will throw a ReferenceError\n"
	expectParseError(t, "class Foo extends Bar { constructor() { delete super.foo } }", text)
	expectParseError(t, "class Foo extends Bar { constructor() { delete super['foo'] } }", text)
	expectParseError(t, "class Foo extends Bar { constructor() { delete (super.foo) } }", text)
	expectParseError(t, "class Foo extends Bar { constructor() { delete (super['foo']) } }", text)

	expectParseError(t, "class Foo extends Bar { constructor() { delete super.foo.bar } }", "")
	expectParseError(t, "class Foo extends Bar { constructor() { delete super['foo']['bar'] } }", "")
}

func TestWarningDuplicateCase(t *testing.T) {
	expectParseError(t, "switch (x) { case null: case undefined: }", "")
	expectParseError(t, "switch (x) { case false: case true: }", "")
	expectParseError(t, "switch (x) { case 0: case 1: }", "")
	expectParseError(t, "switch (x) { case 1: case 1n: }", "")
	expectParseError(t, "switch (x) { case 'a': case 'b': }", "")
	expectParseError(t, "switch (x) { case y: case z: }", "")
	expectParseError(t, "switch (x) { case y.a: case y.b: }", "")
	expectParseError(t, "switch (x) { case y.a: case z.a: }", "")
	expectParseError(t, "switch (x) { case y.a: case y?.a: }", "")
	expectParseError(t, "switch (x) { case y[a]: case y[b]: }", "")
	expectParseError(t, "switch (x) { case y[a]: case z[a]: }", "")
	expectParseError(t, "switch (x) { case y[a]: case y?.[a]: }", "")

	alwaysWarning := "<stdin>: warning: This case clause will never be evaluated because it duplicates an earlier case clause\n"
	likelyWarning := "<stdin>: warning: This case clause may never be evaluated because it likely duplicates an earlier case clause\n"

	expectParseError(t, "switch (x) { case null: case null: }", alwaysWarning)
	expectParseError(t, "switch (x) { case undefined: case undefined: }", alwaysWarning)
	expectParseError(t, "switch (x) { case true: case true: }", alwaysWarning)
	expectParseError(t, "switch (x) { case false: case false: }", alwaysWarning)
	expectParseError(t, "switch (x) { case 0xF: case 15: }", alwaysWarning)
	expectParseError(t, "switch (x) { case 'a': case `a`: }", alwaysWarning)
	expectParseError(t, "switch (x) { case 123n: case 1_2_3n: }", alwaysWarning)
	expectParseError(t, "switch (x) { case y: case y: }", alwaysWarning)
	expectParseError(t, "switch (x) { case y.a: case y.a: }", likelyWarning)
	expectParseError(t, "switch (x) { case y?.a: case y?.a: }", likelyWarning)
	expectParseError(t, "switch (x) { case y[a]: case y[a]: }", likelyWarning)
	expectParseError(t, "switch (x) { case y?.[a]: case y?.[a]: }", likelyWarning)
}

func TestMangleFor(t *testing.T) {
	expectPrintedMangle(t, "var a; while (1) ;", "for (var a; ; )\n  ;\n")
	expectPrintedMangle(t, "let a; while (1) ;", "let a;\nfor (; ; )\n  ;\n")
	expectPrintedMangle(t, "const a=0; while (1) ;", "const a = 0;\nfor (; ; )\n  ;\n")

	expectPrintedMangle(t, "var a; for (var b;;) ;", "for (var a, b; ; )\n  ;\n")
	expectPrintedMangle(t, "let a; for (let b;;) ;", "let a;\nfor (let b; ; )\n  ;\n")
	expectPrintedMangle(t, "const a=0; for (const b = 1;;) ;", "const a = 0;\nfor (const b = 1; ; )\n  ;\n")

	expectPrintedMangle(t, "export var a; while (1) ;", "export var a;\nfor (; ; )\n  ;\n")
	expectPrintedMangle(t, "export let a; while (1) ;", "export let a;\nfor (; ; )\n  ;\n")
	expectPrintedMangle(t, "export const a=0; while (1) ;", "export const a = 0;\nfor (; ; )\n  ;\n")

	expectPrintedMangle(t, "export var a; for (var b;;) ;", "export var a;\nfor (var b; ; )\n  ;\n")
	expectPrintedMangle(t, "export let a; for (let b;;) ;", "export let a;\nfor (let b; ; )\n  ;\n")
	expectPrintedMangle(t, "export const a=0; for (const b = 1;;) ;", "export const a = 0;\nfor (const b = 1; ; )\n  ;\n")

	expectPrintedMangle(t, "var a; for (let b;;) ;", "var a;\nfor (let b; ; )\n  ;\n")
	expectPrintedMangle(t, "let a; for (const b=0;;) ;", "let a;\nfor (const b = 0; ; )\n  ;\n")
	expectPrintedMangle(t, "const a=0; for (var b;;) ;", "const a = 0;\nfor (var b; ; )\n  ;\n")

	expectPrintedMangle(t, "a(); while (1) ;", "for (a(); ; )\n  ;\n")
	expectPrintedMangle(t, "a(); for (b();;) ;", "for (a(), b(); ; )\n  ;\n")

	expectPrintedMangle(t, "for (; ;) if (x) break;", "for (; !x; )\n  ;\n")
	expectPrintedMangle(t, "for (; ;) if (!x) break;", "for (; x; )\n  ;\n")
	expectPrintedMangle(t, "for (; a;) if (x) break;", "for (; a && !x; )\n  ;\n")
	expectPrintedMangle(t, "for (; a;) if (!x) break;", "for (; a && x; )\n  ;\n")
	expectPrintedMangle(t, "for (; ;) { if (x) break; y(); }", "for (; !x; )\n  y();\n")
	expectPrintedMangle(t, "for (; a;) { if (x) break; y(); }", "for (; a && !x; )\n  y();\n")
	expectPrintedMangle(t, "for (; ;) if (x) break; else y();", "for (; !x; )\n  y();\n")
	expectPrintedMangle(t, "for (; a;) if (x) break; else y();", "for (; a && !x; )\n  y();\n")
	expectPrintedMangle(t, "for (; ;) { if (x) break; else y(); z(); }", "for (; !x; )\n  y(), z();\n")
	expectPrintedMangle(t, "for (; a;) { if (x) break; else y(); z(); }", "for (; a && !x; )\n  y(), z();\n")
	expectPrintedMangle(t, "for (; ;) if (x) y(); else break;", "for (; x; )\n  y();\n")
	expectPrintedMangle(t, "for (; ;) if (!x) y(); else break;", "for (; !x; )\n  y();\n")
	expectPrintedMangle(t, "for (; a;) if (x) y(); else break;", "for (; a && x; )\n  y();\n")
	expectPrintedMangle(t, "for (; a;) if (!x) y(); else break;", "for (; a && !x; )\n  y();\n")
	expectPrintedMangle(t, "for (; ;) { if (x) y(); else break; z(); }", "for (; x; ) {\n  y();\n  z();\n}\n")
	expectPrintedMangle(t, "for (; a;) { if (x) y(); else break; z(); }", "for (; a && x; ) {\n  y();\n  z();\n}\n")
}

func TestMangleUndefined(t *testing.T) {
	// These should be transformed
	expectPrintedMangle(t, "console.log(undefined)", "console.log(void 0);\n")
	expectPrintedMangle(t, "console.log(+undefined)", "console.log(NaN);\n")
	expectPrintedMangle(t, "console.log(undefined + undefined)", "console.log(void 0 + void 0);\n")
	expectPrintedMangle(t, "const x = undefined", "const x = void 0;\n")
	expectPrintedMangle(t, "let x = undefined", "let x;\n")
	expectPrintedMangle(t, "var x = undefined", "var x = void 0;\n")
	expectPrintedMangle(t, "function foo(a) { if (!a) return undefined; a() }", "function foo(a) {\n  if (!a)\n    return;\n  a();\n}\n")

	// These should not be transformed
	expectPrintedMangle(t, "delete undefined", "delete undefined;\n")
	expectPrintedMangle(t, "undefined--", "undefined--;\n")
	expectPrintedMangle(t, "undefined++", "undefined++;\n")
	expectPrintedMangle(t, "--undefined", "--undefined;\n")
	expectPrintedMangle(t, "++undefined", "++undefined;\n")
	expectPrintedMangle(t, "undefined = 1", "undefined = 1;\n")
	expectPrintedMangle(t, "[undefined] = 1", "[undefined] = 1;\n")
	expectPrintedMangle(t, "({x: undefined} = 1)", "({x: undefined} = 1);\n")
	expectPrintedMangle(t, "with (x) y(undefined); z(undefined)", "with (x)\n  y(undefined);\nz(void 0);\n")
	expectPrintedMangle(t, "with (x) while (i) y(undefined); z(undefined)", "with (x)\n  for (; i; )\n    y(undefined);\nz(void 0);\n")
}

func TestMangleIndex(t *testing.T) {
	expectPrintedMangle(t, "x['y']", "x.y;\n")
	expectPrintedMangle(t, "x['y z']", "x[\"y z\"];\n")
	expectPrintedMangle(t, "x?.['y']", "x?.y;\n")
	expectPrintedMangle(t, "x?.['y z']", "x?.[\"y z\"];\n")
	expectPrintedMangle(t, "x?.['y']()", "x?.y();\n")
	expectPrintedMangle(t, "x?.['y z']()", "x?.[\"y z\"]();\n")
}

func TestMangleBlock(t *testing.T) {
	expectPrintedMangle(t, "while(1) { while (1) {} }", "for (; ; )\n  for (; ; )\n    ;\n")
	expectPrintedMangle(t, "while(1) { const x = 0; }", "for (; ; ) {\n  const x = 0;\n}\n")
	expectPrintedMangle(t, "while(1) { let x; }", "for (; ; ) {\n  let x;\n}\n")
	expectPrintedMangle(t, "while(1) { var x; }", "for (; ; )\n  var x;\n")
	expectPrintedMangle(t, "while(1) { class X {} }", "for (; ; ) {\n  class X {\n  }\n}\n")
	expectPrintedMangle(t, "while(1) { function x() {} }", "for (; ; ) {\n  function x() {\n  }\n}\n")
	expectPrintedMangle(t, "while(1) { function* x() {} }", "for (; ; ) {\n  function* x() {\n  }\n}\n")
	expectPrintedMangle(t, "while(1) { async function x() {} }", "for (; ; ) {\n  async function x() {\n  }\n}\n")
	expectPrintedMangle(t, "while(1) { async function* x() {} }", "for (; ; ) {\n  async function* x() {\n  }\n}\n")
}

func TestMangleNot(t *testing.T) {
	// These can be mangled
	expectPrintedMangle(t, "a = !(b == c)", "a = b != c;\n")
	expectPrintedMangle(t, "a = !(b != c)", "a = b == c;\n")
	expectPrintedMangle(t, "a = !(b === c)", "a = b !== c;\n")
	expectPrintedMangle(t, "a = !(b !== c)", "a = b === c;\n")
	expectPrintedMangle(t, "if (!(a, b)) return c", "if (a, !b)\n  return c;\n")

	// These can't be mangled due to NaN and other special cases
	expectPrintedMangle(t, "a = !(b < c)", "a = !(b < c);\n")
	expectPrintedMangle(t, "a = !(b > c)", "a = !(b > c);\n")
	expectPrintedMangle(t, "a = !(b <= c)", "a = !(b <= c);\n")
	expectPrintedMangle(t, "a = !(b >= c)", "a = !(b >= c);\n")
}

func TestMangleDoubleNot(t *testing.T) {
	expectPrintedMangle(t, "a = !!b", "a = !!b;\n")

	expectPrintedMangle(t, "a = !!!b", "a = !b;\n")
	expectPrintedMangle(t, "a = !!-b", "a = !!-b;\n")
	expectPrintedMangle(t, "a = !!void b", "a = !!void b;\n")
	expectPrintedMangle(t, "a = !!delete b", "a = delete b;\n")

	expectPrintedMangle(t, "a = !!(b + c)", "a = !!(b + c);\n")
	expectPrintedMangle(t, "a = !!(b == c)", "a = b == c;\n")
	expectPrintedMangle(t, "a = !!(b != c)", "a = b != c;\n")
	expectPrintedMangle(t, "a = !!(b === c)", "a = b === c;\n")
	expectPrintedMangle(t, "a = !!(b !== c)", "a = b !== c;\n")
	expectPrintedMangle(t, "a = !!(b < c)", "a = b < c;\n")
	expectPrintedMangle(t, "a = !!(b > c)", "a = b > c;\n")
	expectPrintedMangle(t, "a = !!(b <= c)", "a = b <= c;\n")
	expectPrintedMangle(t, "a = !!(b >= c)", "a = b >= c;\n")
	expectPrintedMangle(t, "a = !!(b in c)", "a = b in c;\n")
	expectPrintedMangle(t, "a = !!(b instanceof c)", "a = b instanceof c;\n")

	expectPrintedMangle(t, "a = !!(b && c)", "a = !!(b && c);\n")
	expectPrintedMangle(t, "a = !!(b || c)", "a = !!(b || c);\n")
	expectPrintedMangle(t, "a = !!(b ?? c)", "a = !!(b ?? c);\n")

	expectPrintedMangle(t, "a = !!(!b && c)", "a = !!(!b && c);\n")
	expectPrintedMangle(t, "a = !!(!b || c)", "a = !!(!b || c);\n")
	expectPrintedMangle(t, "a = !!(!b ?? c)", "a = !b ?? c;\n")

	expectPrintedMangle(t, "a = !!(b && !c)", "a = !!(b && !c);\n")
	expectPrintedMangle(t, "a = !!(b || !c)", "a = !!(b || !c);\n")
	expectPrintedMangle(t, "a = !!(b ?? !c)", "a = !!(b ?? !c);\n")

	expectPrintedMangle(t, "a = !!(!b && !c)", "a = !b && !c;\n")
	expectPrintedMangle(t, "a = !!(!b || !c)", "a = !b || !c;\n")
	expectPrintedMangle(t, "a = !!(!b ?? !c)", "a = !b ?? !c;\n")
}

func TestMangleIf(t *testing.T) {
	expectPrintedMangle(t, "1 ? a() : b()", "a();\n")
	expectPrintedMangle(t, "0 ? a() : b()", "b();\n")

	expectPrintedMangle(t, "a ? a : b", "a || b;\n")
	expectPrintedMangle(t, "a ? b : a", "a && b;\n")
	expectPrintedMangle(t, "a.x ? a.x : b", "a.x ? a.x : b;\n")
	expectPrintedMangle(t, "a.x ? b : a.x", "a.x ? b : a.x;\n")

	expectPrintedMangle(t, "a ? b() : c()", "a ? b() : c();\n")
	expectPrintedMangle(t, "!a ? b() : c()", "a ? c() : b();\n")
	expectPrintedMangle(t, "!!a ? b() : c()", "a ? b() : c();\n")
	expectPrintedMangle(t, "!!!a ? b() : c()", "a ? c() : b();\n")

	expectPrintedMangle(t, "if (1) a(); else b()", "a();\n")
	expectPrintedMangle(t, "if (0) a(); else b()", "b();\n")
	expectPrintedMangle(t, "if (a) b(); else c()", "a ? b() : c();\n")
	expectPrintedMangle(t, "if (!a) b(); else c()", "a ? c() : b();\n")
	expectPrintedMangle(t, "if (!!a) b(); else c()", "a ? b() : c();\n")
	expectPrintedMangle(t, "if (!!!a) b(); else c()", "a ? c() : b();\n")

	expectPrintedMangle(t, "if (1) a()", "a();\n")
	expectPrintedMangle(t, "if (0) a()", "")
	expectPrintedMangle(t, "if (a) b()", "a && b();\n")
	expectPrintedMangle(t, "if (!a) b()", "a || b();\n")
	expectPrintedMangle(t, "if (!!a) b()", "a && b();\n")
	expectPrintedMangle(t, "if (!!!a) b()", "a || b();\n")

	expectPrintedMangle(t, "if (1) {} else a()", "")
	expectPrintedMangle(t, "if (0) {} else a()", "a();\n")
	expectPrintedMangle(t, "if (a) {} else b()", "a || b();\n")
	expectPrintedMangle(t, "if (!a) {} else b()", "a && b();\n")
	expectPrintedMangle(t, "if (!!a) {} else b()", "a || b();\n")
	expectPrintedMangle(t, "if (!!!a) {} else b()", "a && b();\n")

	expectPrintedMangle(t, "if (a) {} else throw b", "if (!a)\n  throw b;\n")
	expectPrintedMangle(t, "if (!a) {} else throw b", "if (a)\n  throw b;\n")
	expectPrintedMangle(t, "a(); if (b) throw c", "if (a(), b)\n  throw c;\n")
	expectPrintedMangle(t, "if (a) if (b) throw c", "if (a && b)\n  throw c;\n")

	expectPrintedMangle(t, "if (true) { let a = b; if (c) throw d }",
		"{\n  let a = b;\n  if (c)\n    throw d;\n}\n")
	expectPrintedMangle(t, "if (true) { if (a) throw b; if (c) throw d }",
		"if (a)\n  throw b;\nif (c)\n  throw d;\n")

	expectPrintedMangle(t, "if (false) throw a; else { let b = c; if (d) throw e }",
		"{\n  let b = c;\n  if (d)\n    throw e;\n}\n")
	expectPrintedMangle(t, "if (false) throw a; else { if (b) throw c; if (d) throw e }",
		"if (b)\n  throw c;\nif (d)\n  throw e;\n")

	expectPrintedMangle(t, "if (a) { if (b) throw c; else { let d = e; if (f) throw g } }",
		"if (a) {\n  if (b)\n    throw c;\n  {\n    let d = e;\n    if (f)\n      throw g;\n  }\n}\n")
	expectPrintedMangle(t, "if (a) { if (b) throw c; else if (d) throw e; else if (f) throw g }",
		"if (a) {\n  if (b)\n    throw c;\n  if (d)\n    throw e;\n  if (f)\n    throw g;\n}\n")

	expectPrintedMangle(t, "a = b ? true : false", "a = !!b;\n")
	expectPrintedMangle(t, "a = b ? false : true", "a = !b;\n")
	expectPrintedMangle(t, "a = !b ? true : false", "a = !b;\n")
	expectPrintedMangle(t, "a = !b ? false : true", "a = !!b;\n")

	expectPrintedMangle(t, "a = b == c ? true : false", "a = b == c;\n")
	expectPrintedMangle(t, "a = b != c ? true : false", "a = b != c;\n")
	expectPrintedMangle(t, "a = b === c ? true : false", "a = b === c;\n")
	expectPrintedMangle(t, "a = b !== c ? true : false", "a = b !== c;\n")

	expectPrintedMangle(t, "a ? b(c) : b(d)", "a ? b(c) : b(d);\n")
	expectPrintedMangle(t, "let a; a ? b(c) : b(d)", "let a;\na ? b(c) : b(d);\n")
	expectPrintedMangle(t, "let a, b; a ? b(c) : b(d)", "let a, b;\nb(a ? c : d);\n")
	expectPrintedMangle(t, "let a, b; a ? b(c, 0) : b(d)", "let a, b;\na ? b(c, 0) : b(d);\n")
	expectPrintedMangle(t, "let a, b; a ? b(c) : b(d, 0)", "let a, b;\na ? b(c) : b(d, 0);\n")
	expectPrintedMangle(t, "let a, b; a ? b(c, 0) : b(d, 1)", "let a, b;\na ? b(c, 0) : b(d, 1);\n")
	expectPrintedMangle(t, "let a, b; a ? b(c, 0) : b(d, 0)", "let a, b;\nb(a ? c : d, 0);\n")
	expectPrintedMangle(t, "let a, b; a ? b(...c) : b(d)", "let a, b;\na ? b(...c) : b(d);\n")
	expectPrintedMangle(t, "let a, b; a ? b(c) : b(...d)", "let a, b;\na ? b(c) : b(...d);\n")
	expectPrintedMangle(t, "let a, b; a ? b(...c) : b(...d)", "let a, b;\nb(...a ? c : d);\n")
	expectPrintedMangle(t, "let a, b; a ? b(a) : b(c)", "let a, b;\nb(a || c);\n")
	expectPrintedMangle(t, "let a, b; a ? b(c) : b(a)", "let a, b;\nb(a && c);\n")
	expectPrintedMangle(t, "let a, b; a ? b(...a) : b(...c)", "let a, b;\nb(...a || c);\n")
	expectPrintedMangle(t, "let a, b; a ? b(...c) : b(...a)", "let a, b;\nb(...a && c);\n")

	// Note: "a.x" may change "b" and "b.y" may change "a" in the examples
	// below, so the presence of these expressions must prevent reordering
	expectPrintedMangle(t, "let a; a.x ? b(c) : b(d)", "let a;\na.x ? b(c) : b(d);\n")
	expectPrintedMangle(t, "let a, b; a.x ? b(c) : b(d)", "let a, b;\na.x ? b(c) : b(d);\n")
	expectPrintedMangle(t, "let a, b; a ? b.y(c) : b.y(d)", "let a, b;\na ? b.y(c) : b.y(d);\n")
	expectPrintedMangle(t, "let a, b; a.x ? b.y(c) : b.y(d)", "let a, b;\na.x ? b.y(c) : b.y(d);\n")

	expectPrintedMangle(t, "a ? b : c ? b : d", "a || c ? b : d;\n")
	expectPrintedMangle(t, "a ? b ? c : d : d", "a && b ? c : d;\n")

	expectPrintedMangle(t, "a ? c : (b, c)", "a || b, c;\n")
	expectPrintedMangle(t, "a ? (b, c) : c", "a && b, c;\n")
	expectPrintedMangle(t, "a ? c : (b, d)", "a ? c : (b, d);\n")
	expectPrintedMangle(t, "a ? (b, c) : d", "a ? (b, c) : d;\n")

	expectPrintedMangle(t, "a ? b || c : c", "a && b || c;\n")
	expectPrintedMangle(t, "a ? b || c : d", "a ? b || c : d;\n")
	expectPrintedMangle(t, "a ? b && c : c", "a ? b && c : c;\n")

	expectPrintedMangle(t, "a ? c : b && c", "(a || b) && c;\n")
	expectPrintedMangle(t, "a ? c : b && d", "a ? c : b && d;\n")
	expectPrintedMangle(t, "a ? c : b || c", "a ? c : b || c;\n")

	expectPrintedMangle(t, "a = b == null ? c : b", "a = b == null ? c : b;\n")
	expectPrintedMangle(t, "a = b != null ? b : c", "a = b != null ? b : c;\n")

	expectPrintedMangle(t, "let b; a = b == null ? c : b", "let b;\na = b ?? c;\n")
	expectPrintedMangle(t, "let b; a = b != null ? b : c", "let b;\na = b ?? c;\n")
	expectPrintedMangle(t, "let b; a = b == null ? b : c", "let b;\na = b == null ? b : c;\n")
	expectPrintedMangle(t, "let b; a = b != null ? c : b", "let b;\na = b != null ? c : b;\n")

	expectPrintedMangle(t, "let b; a = null == b ? c : b", "let b;\na = b ?? c;\n")
	expectPrintedMangle(t, "let b; a = null != b ? b : c", "let b;\na = b ?? c;\n")
	expectPrintedMangle(t, "let b; a = null == b ? b : c", "let b;\na = b == null ? b : c;\n")
	expectPrintedMangle(t, "let b; a = null != b ? c : b", "let b;\na = b != null ? c : b;\n")

	// Don't do this if the condition has side effects
	expectPrintedMangle(t, "let b; a = b.x == null ? c : b.x", "let b;\na = b.x == null ? c : b.x;\n")
	expectPrintedMangle(t, "let b; a = b.x != null ? b.x : c", "let b;\na = b.x != null ? b.x : c;\n")
	expectPrintedMangle(t, "let b; a = null == b.x ? c : b.x", "let b;\na = b.x == null ? c : b.x;\n")
	expectPrintedMangle(t, "let b; a = null != b.x ? b.x : c", "let b;\na = b.x != null ? b.x : c;\n")

	// Don't do this for strict equality comparisons
	expectPrintedMangle(t, "let b; a = b === null ? c : b", "let b;\na = b === null ? c : b;\n")
	expectPrintedMangle(t, "let b; a = b !== null ? b : c", "let b;\na = b !== null ? b : c;\n")
	expectPrintedMangle(t, "let b; a = null === b ? c : b", "let b;\na = b === null ? c : b;\n")
	expectPrintedMangle(t, "let b; a = null !== b ? b : c", "let b;\na = b !== null ? b : c;\n")

	expectPrintedMangle(t, "let b; a = null === b || b === undefined ? c : b", "let b;\na = b ?? c;\n")
	expectPrintedMangle(t, "let b; a = b !== undefined && b !== null ? b : c", "let b;\na = b ?? c;\n")

	expectPrintedMangle(t, "a ? b : b", "a, b;\n")
	expectPrintedMangle(t, "let a; a ? b : b", "let a;\nb;\n")

	expectPrintedMangle(t, "a ? -b : -b", "a, -b;\n")
	expectPrintedMangle(t, "a ? b.c : b.c", "a, b.c;\n")
	expectPrintedMangle(t, "a ? b?.c : b?.c", "a, b?.c;\n")
	expectPrintedMangle(t, "a ? b[c] : b[c]", "a, b[c];\n")
	expectPrintedMangle(t, "a ? b() : b()", "a, b();\n")
	expectPrintedMangle(t, "a ? b?.() : b?.()", "a, b?.();\n")
	expectPrintedMangle(t, "a ? b?.[c] : b?.[c]", "a, b?.[c];\n")
	expectPrintedMangle(t, "a ? b == c : b == c", "a, b == c;\n")
	expectPrintedMangle(t, "a ? b.c(d + e[f]) : b.c(d + e[f])", "a, b.c(d + e[f]);\n")

	expectPrintedMangle(t, "a ? -b : !b", "a ? -b : !b;\n")
	expectPrintedMangle(t, "a ? b() : b(c)", "a ? b() : b(c);\n")
	expectPrintedMangle(t, "a ? b(c) : b(d)", "a ? b(c) : b(d);\n")
	expectPrintedMangle(t, "a ? b?.c : b.c", "a ? b?.c : b.c;\n")
	expectPrintedMangle(t, "a ? b?.() : b()", "a ? b?.() : b();\n")
	expectPrintedMangle(t, "a ? b?.[c] : b[c]", "a ? b?.[c] : b[c];\n")
	expectPrintedMangle(t, "a ? b == c : b != c", "a ? b == c : b != c;\n")
	expectPrintedMangle(t, "a ? b.c(d + e[f]) : b.c(d + e[g])", "a ? b.c(d + e[f]) : b.c(d + e[g]);\n")
}

func TestMangleReturn(t *testing.T) {
	expectPrintedMangle(t, "function foo() { a = b; if (a) return a; if (b) c = b; return c; }",
		"function foo() {\n  return a = b, a || (b && (c = b), c);\n}\n")
	expectPrintedMangle(t, "function foo() { a = b; if (a) return; if (b) c = b; return c; }",
		"function foo() {\n  return a = b, a ? void 0 : (b && (c = b), c);\n}\n")
	expectPrintedMangle(t, "function foo() { if (!a) return b; return c; }", "function foo() {\n  return a ? c : b;\n}\n")

	expectPrintedMangle(t, "if (1) return a(); else return b()", "return a();\n")
	expectPrintedMangle(t, "if (0) return a(); else return b()", "return b();\n")
	expectPrintedMangle(t, "if (a) return b(); else return c()", "return a ? b() : c();\n")
	expectPrintedMangle(t, "if (!a) return b(); else return c()", "return a ? c() : b();\n")
	expectPrintedMangle(t, "if (!!a) return b(); else return c()", "return a ? b() : c();\n")
	expectPrintedMangle(t, "if (!!!a) return b(); else return c()", "return a ? c() : b();\n")

	expectPrintedMangle(t, "if (1) return a(); return b()", "return a();\n")
	expectPrintedMangle(t, "if (0) return a(); return b()", "return b();\n")
	expectPrintedMangle(t, "if (a) return b(); return c()", "return a ? b() : c();\n")
	expectPrintedMangle(t, "if (!a) return b(); return c()", "return a ? c() : b();\n")
	expectPrintedMangle(t, "if (!!a) return b(); return c()", "return a ? b() : c();\n")
	expectPrintedMangle(t, "if (!!!a) return b(); return c()", "return a ? c() : b();\n")
}

func TestMangleThrow(t *testing.T) {
	expectPrintedMangle(t, "function foo() { a = b; if (a) throw a; if (b) c = b; throw c; }",
		"function foo() {\n  throw a = b, a || (b && (c = b), c);\n}\n")
	expectPrintedMangle(t, "function foo() { if (!a) throw b; throw c; }", "function foo() {\n  throw a ? c : b;\n}\n")

	expectPrintedMangle(t, "if (1) throw a(); else throw b()", "throw a();\n")
	expectPrintedMangle(t, "if (0) throw a(); else throw b()", "throw b();\n")
	expectPrintedMangle(t, "if (a) throw b(); else throw c()", "throw a ? b() : c();\n")
	expectPrintedMangle(t, "if (!a) throw b(); else throw c()", "throw a ? c() : b();\n")
	expectPrintedMangle(t, "if (!!a) throw b(); else throw c()", "throw a ? b() : c();\n")
	expectPrintedMangle(t, "if (!!!a) throw b(); else throw c()", "throw a ? c() : b();\n")

	expectPrintedMangle(t, "if (1) throw a(); throw b()", "throw a();\n")
	expectPrintedMangle(t, "if (0) throw a(); throw b()", "throw b();\n")
	expectPrintedMangle(t, "if (a) throw b(); throw c()", "throw a ? b() : c();\n")
	expectPrintedMangle(t, "if (!a) throw b(); throw c()", "throw a ? c() : b();\n")
	expectPrintedMangle(t, "if (!!a) throw b(); throw c()", "throw a ? b() : c();\n")
	expectPrintedMangle(t, "if (!!!a) throw b(); throw c()", "throw a ? c() : b();\n")
}

func TestMangleInitializer(t *testing.T) {
	expectPrintedMangle(t, "const a = undefined", "const a = void 0;\n")
	expectPrintedMangle(t, "let a = undefined", "let a;\n")
	expectPrintedMangle(t, "let {} = undefined", "let {} = void 0;\n")
	expectPrintedMangle(t, "let [] = undefined", "let [] = void 0;\n")
	expectPrintedMangle(t, "var a = undefined", "var a = void 0;\n")
	expectPrintedMangle(t, "var {} = undefined", "var {} = void 0;\n")
	expectPrintedMangle(t, "var [] = undefined", "var [] = void 0;\n")
}

func TestMangleCall(t *testing.T) {
	expectPrintedMangle(t, "x = foo(1, ...[], 2)", "x = foo(1, 2);\n")
	expectPrintedMangle(t, "x = foo(1, ...2, 3)", "x = foo(1, ...2, 3);\n")
	expectPrintedMangle(t, "x = foo(1, ...[2], 3)", "x = foo(1, 2, 3);\n")
	expectPrintedMangle(t, "x = foo(1, ...[2, 3], 4)", "x = foo(1, 2, 3, 4);\n")
	expectPrintedMangle(t, "x = foo(1, ...[2, ...y, 3], 4)", "x = foo(1, 2, ...y, 3, 4);\n")
	expectPrintedMangle(t, "x = foo(1, ...{a, b}, 4)", "x = foo(1, ...{a, b}, 4);\n")

	// Holes must become undefined
	expectPrintedMangle(t, "x = foo(1, ...[,2,,], 3)", "x = foo(1, void 0, 2, void 0, 3);\n")
}

func TestMangleArray(t *testing.T) {
	expectPrintedMangle(t, "x = [1, ...[], 2]", "x = [1, 2];\n")
	expectPrintedMangle(t, "x = [1, ...2, 3]", "x = [1, ...2, 3];\n")
	expectPrintedMangle(t, "x = [1, ...[2], 3]", "x = [1, 2, 3];\n")
	expectPrintedMangle(t, "x = [1, ...[2, 3], 4]", "x = [1, 2, 3, 4];\n")
	expectPrintedMangle(t, "x = [1, ...[2, ...y, 3], 4]", "x = [1, 2, ...y, 3, 4];\n")
	expectPrintedMangle(t, "x = [1, ...{a, b}, 4]", "x = [1, ...{a, b}, 4];\n")

	// Holes must become undefined, which is different than a hole
	expectPrintedMangle(t, "x = [1, ...[,2,,], 3]", "x = [1, void 0, 2, void 0, 3];\n")
}

func TestMangleObject(t *testing.T) {
	expectPrintedMangle(t, "x = {a, ...{}, b}", "x = {a, b};\n")
	expectPrintedMangle(t, "x = {a, ...b, c}", "x = {a, ...b, c};\n")
	expectPrintedMangle(t, "x = {a, ...{b}, c}", "x = {a, b, c};\n")
	expectPrintedMangle(t, "x = {a, ...{b() {}}, c}", "x = {a, b() {\n}, c};\n")
	expectPrintedMangle(t, "x = {a, ...{b, c}, d}", "x = {a, b, c, d};\n")
	expectPrintedMangle(t, "x = {a, ...{b, ...y, c}, d}", "x = {a, b, ...y, c, d};\n")
	expectPrintedMangle(t, "x = {a, ...[b, c], d}", "x = {a, ...[b, c], d};\n")

	// Computed properties should be ok
	expectPrintedMangle(t, "x = {a, ...{[b]: c}, d}", "x = {a, [b]: c, d};\n")
	expectPrintedMangle(t, "x = {a, ...{[b]() {}}, c}", "x = {a, [b]() {\n}, c};\n")

	// Getters and setters are not supported
	expectPrintedMangle(t, "x = {a, ...{b, get c() { return y++ }, d}, e}",
		"x = {a, b, ...{get c() {\n  return y++;\n}, d}, e};\n")
	expectPrintedMangle(t, "x = {a, ...{b, set c(_) { throw _ }, d}, e}",
		"x = {a, b, ...{set c(_) {\n  throw _;\n}, d}, e};\n")

	// Spread is ignored for certain values
	expectPrintedMangle(t, "x = {a, ...true, b}", "x = {a, b};\n")
	expectPrintedMangle(t, "x = {a, ...null, b}", "x = {a, b};\n")
	expectPrintedMangle(t, "x = {a, ...void 0, b}", "x = {a, b};\n")
	expectPrintedMangle(t, "x = {a, ...123, b}", "x = {a, b};\n")
	expectPrintedMangle(t, "x = {a, ...123n, b}", "x = {a, b};\n")
	expectPrintedMangle(t, "x = {a, .../x/, b}", "x = {a, b};\n")
	expectPrintedMangle(t, "x = {a, ...function(){}, b}", "x = {a, b};\n")
	expectPrintedMangle(t, "x = {a, ...()=>{}, b}", "x = {a, b};\n")
	expectPrintedMangle(t, "x = {a, ...'123', b}", "x = {a, ...\"123\", b};\n")
	expectPrintedMangle(t, "x = {a, ...[1, 2, 3], b}", "x = {a, ...[1, 2, 3], b};\n")
	expectPrintedMangle(t, "x = {a, ...(()=>{})(), b}", "x = {a, ...(() => {\n})(), b};\n")
}

func TestMangleArrow(t *testing.T) {
	expectPrintedMangle(t, "var a = () => {}", "var a = () => {\n};\n")
	expectPrintedMangle(t, "var a = () => 123", "var a = () => 123;\n")
	expectPrintedMangle(t, "var a = () => void 0", "var a = () => {\n};\n")
	expectPrintedMangle(t, "var a = () => undefined", "var a = () => {\n};\n")
	expectPrintedMangle(t, "var a = () => {return}", "var a = () => {\n};\n")
	expectPrintedMangle(t, "var a = () => {return 123}", "var a = () => 123;\n")
	expectPrintedMangle(t, "var a = () => {throw 123}", "var a = () => {\n  throw 123;\n};\n")
}

func TestMangleTemplate(t *testing.T) {
	expectPrintedMangle(t, "_ = `a${x}b${y}c`", "_ = `a${x}b${y}c`;\n")
	expectPrintedMangle(t, "_ = `a${x}b${'y'}c`", "_ = `a${x}byc`;\n")
	expectPrintedMangle(t, "_ = `a${'x'}b${y}c`", "_ = `axb${y}c`;\n")
	expectPrintedMangle(t, "_ = `a${'x'}b${'y'}c`", "_ = `axbyc`;\n")

	expectPrintedMangle(t, "tag`a${x}b${y}c`", "tag`a${x}b${y}c`;\n")
	expectPrintedMangle(t, "tag`a${x}b${'y'}c`", "tag`a${x}b${\"y\"}c`;\n")
	expectPrintedMangle(t, "tag`a${'x'}b${y}c`", "tag`a${\"x\"}b${y}c`;\n")
	expectPrintedMangle(t, "tag`a${'x'}b${'y'}c`", "tag`a${\"x\"}b${\"y\"}c`;\n")
}

func TestMangleTypeofIdentifier(t *testing.T) {
	expectPrintedMangle(t, "typeof (123, x)", "typeof (0, x);\n")
	expectPrintedMangle(t, "typeof (123, x.y)", "typeof x.y;\n")
	expectPrintedMangle(t, "typeof (123, x); var x", "typeof x;\nvar x;\n")

	expectPrintedMangle(t, "typeof (true && x)", "typeof (0, x);\n")
	expectPrintedMangle(t, "typeof (true && x.y)", "typeof x.y;\n")
	expectPrintedMangle(t, "typeof (true && x); var x", "typeof x;\nvar x;\n")

	expectPrintedMangle(t, "typeof (false || x)", "typeof (0, x);\n")
	expectPrintedMangle(t, "typeof (false || x.y)", "typeof x.y;\n")
	expectPrintedMangle(t, "typeof (false || x); var x", "typeof x;\nvar x;\n")
}

func TestMangleTypeofEquals(t *testing.T) {
	expectPrintedMangle(t, "typeof x === y", "typeof x === y;\n")
	expectPrintedMangle(t, "typeof x !== y", "typeof x !== y;\n")
	expectPrintedMangle(t, "typeof x === 'string'", "typeof x == \"string\";\n")
	expectPrintedMangle(t, "typeof x !== 'string'", "typeof x != \"string\";\n")

	expectPrintedMangle(t, "y === typeof x", "y === typeof x;\n")
	expectPrintedMangle(t, "y !== typeof x", "y !== typeof x;\n")
	expectPrintedMangle(t, "'string' === typeof x", "typeof x == \"string\";\n")
	expectPrintedMangle(t, "'string' !== typeof x", "typeof x != \"string\";\n")
}

func TestMangleNestedLogical(t *testing.T) {
	expectPrintedMangle(t, "(a && b) && c", "a && b && c;\n")
	expectPrintedMangle(t, "a && (b && c)", "a && b && c;\n")
	expectPrintedMangle(t, "(a || b) && c", "(a || b) && c;\n")
	expectPrintedMangle(t, "a && (b || c)", "a && (b || c);\n")

	expectPrintedMangle(t, "(a || b) || c", "a || b || c;\n")
	expectPrintedMangle(t, "a || (b || c)", "a || b || c;\n")
	expectPrintedMangle(t, "(a && b) || c", "a && b || c;\n")
	expectPrintedMangle(t, "a || (b && c)", "a || b && c;\n")
}

func TestMangleEqualsUndefined(t *testing.T) {
	expectPrintedMangle(t, "a === void 0", "a === void 0;\n")
	expectPrintedMangle(t, "a !== void 0", "a !== void 0;\n")
	expectPrintedMangle(t, "void 0 === a", "a === void 0;\n")
	expectPrintedMangle(t, "void 0 !== a", "a !== void 0;\n")

	expectPrintedMangle(t, "a == void 0", "a == null;\n")
	expectPrintedMangle(t, "a != void 0", "a != null;\n")
	expectPrintedMangle(t, "void 0 == a", "a == null;\n")
	expectPrintedMangle(t, "void 0 != a", "a != null;\n")

	expectPrintedMangle(t, "a === null || a === undefined", "a == null;\n")
	expectPrintedMangle(t, "a === null || a !== undefined", "a === null || a !== void 0;\n")
	expectPrintedMangle(t, "a !== null || a === undefined", "a !== null || a === void 0;\n")
	expectPrintedMangle(t, "a === null && a === undefined", "a === null && a === void 0;\n")
	expectPrintedMangle(t, "a.x === null || a.x === undefined", "a.x === null || a.x === void 0;\n")

	expectPrintedMangle(t, "a === undefined || a === null", "a == null;\n")
	expectPrintedMangle(t, "a === undefined || a !== null", "a === void 0 || a !== null;\n")
	expectPrintedMangle(t, "a !== undefined || a === null", "a !== void 0 || a === null;\n")
	expectPrintedMangle(t, "a === undefined && a === null", "a === void 0 && a === null;\n")
	expectPrintedMangle(t, "a.x === undefined || a.x === null", "a.x === void 0 || a.x === null;\n")

	expectPrintedMangle(t, "a !== null && a !== undefined", "a != null;\n")
	expectPrintedMangle(t, "a !== null && a === undefined", "a !== null && a === void 0;\n")
	expectPrintedMangle(t, "a === null && a !== undefined", "a === null && a !== void 0;\n")
	expectPrintedMangle(t, "a !== null || a !== undefined", "a !== null || a !== void 0;\n")
	expectPrintedMangle(t, "a.x !== null && a.x !== undefined", "a.x !== null && a.x !== void 0;\n")

	expectPrintedMangle(t, "a !== undefined && a !== null", "a != null;\n")
	expectPrintedMangle(t, "a !== undefined && a === null", "a !== void 0 && a === null;\n")
	expectPrintedMangle(t, "a === undefined && a !== null", "a === void 0 && a !== null;\n")
	expectPrintedMangle(t, "a !== undefined || a !== null", "a !== void 0 || a !== null;\n")
	expectPrintedMangle(t, "a.x !== undefined && a.x !== null", "a.x !== void 0 && a.x !== null;\n")
}

func TestMangleUnusedFunctionExpressionNames(t *testing.T) {
	expectPrintedMangle(t, "x = function y() {}", "x = function() {\n};\n")
	expectPrintedMangle(t, "x = function y() { return y }", "x = function y() {\n  return y;\n};\n")
	expectPrintedMangle(t, "x = function y() { if (0) return y }", "x = function() {\n};\n")
}

func TestMangleUnusedClassExpressionNames(t *testing.T) {
	expectPrintedMangle(t, "x = class y {}", "x = class {\n};\n")
	expectPrintedMangle(t, "x = class y { foo() { return y } }", "x = class y {\n  foo() {\n    return y;\n  }\n};\n")
	expectPrintedMangle(t, "x = class y { foo() { if (0) return y } }", "x = class {\n  foo() {\n  }\n};\n")
}

func TestMangleUnused(t *testing.T) {
	expectPrintedMangle(t, "null", "")
	expectPrintedMangle(t, "void 0", "")
	expectPrintedMangle(t, "void 0", "")
	expectPrintedMangle(t, "false", "")
	expectPrintedMangle(t, "true", "")
	expectPrintedMangle(t, "123", "")
	expectPrintedMangle(t, "123n", "")
	expectPrintedMangle(t, "'abc'", "")
	expectPrintedMangle(t, "this", "")
	expectPrintedMangle(t, "/regex/", "")
	expectPrintedMangle(t, "(function() {})", "")
	expectPrintedMangle(t, "(() => {})", "")
	expectPrintedMangle(t, "import.meta", "")

	// Known globals can be removed
	expectPrintedMangle(t, "Object", "")
	expectPrintedMangle(t, "Object()", "Object();\n")
	expectPrintedMangle(t, "NonObject", "NonObject;\n")

	expectPrintedMangle(t, "var bound; unbound", "var bound;\nunbound;\n")
	expectPrintedMangle(t, "var bound; bound", "var bound;\n")
	expectPrintedMangle(t, "foo, 123, bar", "foo, bar;\n")

	expectPrintedMangle(t, "[[foo,, 123,, bar]]", "foo, bar;\n")
	expectPrintedMangle(t, "var bound; [123, unbound, ...unbound, 234]", "var bound;\n[unbound, ...unbound];\n")
	expectPrintedMangle(t, "var bound; [123, bound, ...bound, 234]", "var bound;\n[...bound];\n")

	expectPrintedMangle(t, "({foo, x: 123, [y]: 123, z: z, bar})", "foo, y + \"\", z, bar;\n")
	expectPrintedMangle(t, "var bound; ({x: 123, unbound, ...unbound, [unbound]: null, y: 234})", "var bound;\n({unbound, ...unbound, [unbound]: 0});\n")
	expectPrintedMangle(t, "var bound; ({x: 123, bound, ...bound, [bound]: null, y: 234})", "var bound;\n({...bound, [bound]: 0});\n")
	expectPrintedMangle(t, "var bound; ({x: 123, bound, ...bound, [bound]: foo(), y: 234})", "var bound;\n({...bound, [bound]: foo()});\n")

	expectPrintedMangle(t, "console.log(1, foo(), bar())", "console.log(1, foo(), bar());\n")
	expectPrintedMangle(t, "/* @__PURE__ */ console.log(1, foo(), bar())", "foo(), bar();\n")

	expectPrintedMangle(t, "new TestCase(1, foo(), bar())", "new TestCase(1, foo(), bar());\n")
	expectPrintedMangle(t, "/* @__PURE__ */ new TestCase(1, foo(), bar())", "foo(), bar();\n")

	expectPrintedMangle(t, "let x = (1, 2)", "let x = 2;\n")
	expectPrintedMangle(t, "let x = (y, 2)", "let x = (y, 2);\n")
	expectPrintedMangle(t, "let x = (/* @__PURE__ */ foo(bar), 2)", "let x = (bar, 2);\n")

	expectPrintedMangle(t, "let x = (2, y)", "let x = y;\n")
	expectPrintedMangle(t, "let x = (2, y)()", "let x = y();\n")
	expectPrintedMangle(t, "let x = (true && y)()", "let x = y();\n")
	expectPrintedMangle(t, "let x = (false || y)()", "let x = y();\n")
	expectPrintedMangle(t, "let x = (null ?? y)()", "let x = y();\n")
	expectPrintedMangle(t, "let x = (1 ? y : 2)()", "let x = y();\n")
	expectPrintedMangle(t, "let x = (0 ? 1 : y)()", "let x = y();\n")

	// Make sure call targets with "this" values are preserved
	expectPrintedMangle(t, "let x = (2, y.z)", "let x = y.z;\n")
	expectPrintedMangle(t, "let x = (2, y.z)()", "let x = (0, y.z)();\n")
	expectPrintedMangle(t, "let x = (true && y.z)()", "let x = (0, y.z)();\n")
	expectPrintedMangle(t, "let x = (false || y.z)()", "let x = (0, y.z)();\n")
	expectPrintedMangle(t, "let x = (null ?? y.z)()", "let x = (0, y.z)();\n")
	expectPrintedMangle(t, "let x = (1 ? y.z : 2)()", "let x = (0, y.z)();\n")
	expectPrintedMangle(t, "let x = (0 ? 1 : y.z)()", "let x = (0, y.z)();\n")

	expectPrintedMangle(t, "let x = (2, y[z])", "let x = y[z];\n")
	expectPrintedMangle(t, "let x = (2, y[z])()", "let x = (0, y[z])();\n")
	expectPrintedMangle(t, "let x = (true && y[z])()", "let x = (0, y[z])();\n")
	expectPrintedMangle(t, "let x = (false || y[z])()", "let x = (0, y[z])();\n")
	expectPrintedMangle(t, "let x = (null ?? y[z])()", "let x = (0, y[z])();\n")
	expectPrintedMangle(t, "let x = (1 ? y[z] : 2)()", "let x = (0, y[z])();\n")
	expectPrintedMangle(t, "let x = (0 ? 1 : y[z])()", "let x = (0, y[z])();\n")

	// Make sure the return value of "delete" is preserved
	expectPrintedMangle(t, "delete (x)", "delete x;\n")
	expectPrintedMangle(t, "delete (2, x)", "delete (0, x);\n")
	expectPrintedMangle(t, "delete (true && x)", "delete (0, x);\n")
	expectPrintedMangle(t, "delete (false || x)", "delete (0, x);\n")
	expectPrintedMangle(t, "delete (null ?? x)", "delete (0, x);\n")
	expectPrintedMangle(t, "delete (1 ? x : 2)", "delete (0, x);\n")
	expectPrintedMangle(t, "delete (0 ? 1 : x)", "delete (0, x);\n")
	expectPrintedMangle(t, "delete (1, NaN)", "delete (0, NaN);\n")
	expectPrintedMangle(t, "delete (1, Infinity)", "delete (0, Infinity);\n")
	expectPrintedMangle(t, "delete (1, -Infinity)", "delete -Infinity;\n")

	expectPrintedMangle(t, "foo ? 1 : 2", "foo;\n")
	expectPrintedMangle(t, "foo ? 1 : bar", "foo || bar;\n")
	expectPrintedMangle(t, "foo ? bar : 2", "foo && bar;\n")
	expectPrintedMangle(t, "foo ? bar : baz", "foo ? bar : baz;\n")

	for _, op := range []string{"&&", "||", "??"} {
		expectPrintedMangle(t, "foo "+op+" bar", "foo "+op+" bar;\n")
		expectPrintedMangle(t, "var foo; foo "+op+" bar", "var foo;\nfoo "+op+" bar;\n")
		expectPrintedMangle(t, "var bar; foo "+op+" bar", "var bar;\nfoo;\n")
		expectPrintedMangle(t, "var foo, bar; foo "+op+" bar", "var foo, bar;\n")
	}

	expectPrintedMangle(t, "tag`a${b}c${d}e`", "tag`a${b}c${d}e`;\n")
	expectPrintedMangle(t, "`a${b}c${d}e`", "\"\" + b + d;\n")

	expectPrintedMangle(t, "'a' + b + 'c' + d", "\"\" + b + d;\n")
	expectPrintedMangle(t, "a + 'b' + c + 'd'", "a + \"\" + c;\n")
	expectPrintedMangle(t, "a + b + 'c' + 'd'", "a + b + \"\";\n")
	expectPrintedMangle(t, "'a' + 'b' + c + d", "\"\" + c + d;\n")
	expectPrintedMangle(t, "(a + '') + (b + '')", "a + \"\" + (b + \"\");\n")
}

func TestMangleInlineLocals(t *testing.T) {
	check := func(a string, b string) {
		t.Helper()
		expectPrintedMangle(t, "function wrapper(arg0, arg1) {"+a+"}",
			"function wrapper(arg0, arg1) {"+strings.ReplaceAll("\n"+b, "\n", "\n  ")+"\n}\n")
	}

	check("var x = 1; return x", "var x = 1;\nreturn x;")
	check("let x = 1; return x", "return 1;")
	check("const x = 1; return x", "return 1;")

	check("let x = 1; if (false) x++; return x", "return 1;")
	check("let x = 1; if (true) x++; return x", "let x = 1;\nreturn x++, x;")
	check("let x = 1; return x + x", "let x = 1;\nreturn x + x;")

	// Can substitute into normal unary operators
	check("let x = 1; return +x", "return +1;")
	check("let x = 1; return -x", "return -1;")
	check("let x = 1; return !x", "return !1;")
	check("let x = 1; return ~x", "return ~1;")
	check("let x = 1; return void x", "let x = 1;")
	check("let x = 1; return typeof x", "return typeof 1;")

	// Cannot substitute into mutating unary operators
	check("let x = 1; ++x", "let x = 1;\n++x;")
	check("let x = 1; --x", "let x = 1;\n--x;")
	check("let x = 1; x++", "let x = 1;\nx++;")
	check("let x = 1; x--", "let x = 1;\nx--;")
	check("let x = 1; delete x", "let x = 1;\ndelete x;")

	// Cannot substitute into mutating binary operators
	check("let x = 1; x = 2", "let x = 1;\nx = 2;")
	check("let x = 1; x += 2", "let x = 1;\nx += 2;")
	check("let x = 1; x ||= 2", "let x = 1;\nx ||= 2;")

	// Can substitute past mutating binary operators when the left operand has no side effects
	check("let x = 1; arg0 = x", "arg0 = 1;")
	check("let x = 1; arg0 += x", "arg0 += 1;")
	check("let x = 1; arg0 ||= x", "arg0 ||= 1;")
	check("let x = fn(); arg0 = x", "arg0 = fn();")
	check("let x = fn(); arg0 += x", "arg0 += fn();")
	check("let x = fn(); arg0 ||= x", "let x = fn();\narg0 ||= x;")

	// Cannot substitute past mutating binary operators when the left operand has side effects
	check("let x = 1; y.z = x", "let x = 1;\ny.z = x;")
	check("let x = 1; y.z += x", "let x = 1;\ny.z += x;")
	check("let x = 1; y.z ||= x", "let x = 1;\ny.z ||= x;")
	check("let x = fn(); y.z = x", "let x = fn();\ny.z = x;")
	check("let x = fn(); y.z += x", "let x = fn();\ny.z += x;")
	check("let x = fn(); y.z ||= x", "let x = fn();\ny.z ||= x;")

	// Cannot substitute code without side effects past non-mutating binary operators when the left operand has side effects
	check("let x = 1; fn() + x", "let x = 1;\nfn() + x;")

	// Cannot substitute code with side effects past non-mutating binary operators
	check("let x = y(); arg0 + x", "let x = y();\narg0 + x;")

	// Can substitute code without side effects into branches
	check("let x = arg0; return x ? y : z;", "return arg0 ? y : z;")
	check("let x = arg0; return arg1 ? x : y;", "return arg1 ? arg0 : y;")
	check("let x = arg0; return arg1 ? y : x;", "return arg1 ? y : arg0;")
	check("let x = arg0; return x || y;", "return arg0 || y;")
	check("let x = arg0; return x && y;", "return arg0 && y;")
	check("let x = arg0; return x ?? y;", "return arg0 ?? y;")
	check("let x = arg0; return arg1 || x;", "return arg1 || arg0;")
	check("let x = arg0; return arg1 && x;", "return arg1 && arg0;")
	check("let x = arg0; return arg1 ?? x;", "return arg1 ?? arg0;")

	// Can substitute code without side effects into branches past an expression with side effects
	check("let x = arg0; return y ? x : z;", "let x = arg0;\nreturn y ? x : z;")
	check("let x = arg0; return y ? z : x;", "let x = arg0;\nreturn y ? z : x;")
	check("let x = arg0; return (arg1 ? 1 : 2) ? x : 3;", "return (arg1 ? 1 : 2) ? arg0 : 3;")
	check("let x = arg0; return (arg1 ? 1 : 2) ? 3 : x;", "return (arg1 ? 1 : 2) ? 3 : arg0;")
	check("let x = arg0; return (arg1 ? y : 1) ? x : 2;", "let x = arg0;\nreturn (arg1 ? y : 1) ? x : 2;")
	check("let x = arg0; return (arg1 ? 1 : y) ? x : 2;", "let x = arg0;\nreturn (arg1 ? 1 : y) ? x : 2;")
	check("let x = arg0; return (arg1 ? y : 1) ? 2 : x;", "let x = arg0;\nreturn (arg1 ? y : 1) ? 2 : x;")
	check("let x = arg0; return (arg1 ? 1 : y) ? 2 : x;", "let x = arg0;\nreturn (arg1 ? 1 : y) ? 2 : x;")
	check("let x = arg0; return y || x;", "let x = arg0;\nreturn y || x;")
	check("let x = arg0; return y && x;", "let x = arg0;\nreturn y && x;")
	check("let x = arg0; return y ?? x;", "let x = arg0;\nreturn y ?? x;")

	// Cannot substitute code with side effects into branches
	check("let x = fn(); return x ? arg0 : y;", "return fn() ? arg0 : y;")
	check("let x = fn(); return arg0 ? x : y;", "let x = fn();\nreturn arg0 ? x : y;")
	check("let x = fn(); return arg0 ? y : x;", "let x = fn();\nreturn arg0 ? y : x;")
	check("let x = fn(); return x || arg0;", "return fn() || arg0;")
	check("let x = fn(); return x && arg0;", "return fn() && arg0;")
	check("let x = fn(); return x ?? arg0;", "return fn() ?? arg0;")
	check("let x = fn(); return arg0 || x;", "let x = fn();\nreturn arg0 || x;")
	check("let x = fn(); return arg0 && x;", "let x = fn();\nreturn arg0 && x;")
	check("let x = fn(); return arg0 ?? x;", "let x = fn();\nreturn arg0 ?? x;")

	// Test chaining
	check("let x = fn(); let y = x[prop]; let z = y.val; throw z", "throw fn()[prop].val;")
	check("let x = fn(), y = x[prop], z = y.val; throw z", "throw fn()[prop].val;")

	// Can substitute an initializer with side effects
	check("let x = 0; let y = ++x; return y",
		"let x = 0;\nreturn ++x;")

	// Can substitute an initializer without side effects past an expression without side effects
	check("let x = 0; let y = x; return [x, y]",
		"let x = 0;\nreturn [x, x];")

	// Cannot substitute an initializer with side effects past an expression without side effects
	check("let x = 0; let y = ++x; return [x, y]",
		"let x = 0, y = ++x;\nreturn [x, y];")

	// Cannot substitute an initializer without side effects past an expression with side effects
	check("let x = 0; let y = {valueOf() { x = 1 }}; let z = x; return [y == 1, z]",
		"let x = 0, y = {valueOf() {\n  x = 1;\n}}, z = x;\nreturn [y == 1, z];")

	// Cannot inline past a spread operator, since that evaluates code
	check("let x = arg0; return [...x];", "return [...arg0];")
	check("let x = arg0; return [x, ...arg1];", "return [arg0, ...arg1];")
	check("let x = arg0; return [...arg1, x];", "let x = arg0;\nreturn [...arg1, x];")
	check("let x = arg0; return arg1(...x);", "return arg1(...arg0);")
	check("let x = arg0; return arg1(x, ...arg1);", "return arg1(arg0, ...arg1);")
	check("let x = arg0; return arg1(...arg1, x);", "let x = arg0;\nreturn arg1(...arg1, x);")

	// Test various statement kinds
	check("let x = arg0; arg1(x);", "arg1(arg0);")
	check("let x = arg0; throw x;", "throw arg0;")
	check("let x = arg0; return x;", "return arg0;")
	check("let x = arg0; if (x) return 1;", "if (arg0)\n  return 1;")
	check("let x = arg0; switch (x) { case 0: return 1; }", "switch (arg0) {\n  case 0:\n    return 1;\n}")
	check("let x = arg0; let y = x; return y + y;", "let y = arg0;\nreturn y + y;")

	// Loops must not be substituted into because they evaluate multiple times
	check("let x = arg0; do {} while (x);", "let x = arg0;\ndo\n  ;\nwhile (x);")
	check("let x = arg0; while (x) return 1;", "let x = arg0;\nfor (; x; )\n  return 1;")
	check("let x = arg0; for (; x; ) return 1;", "let x = arg0;\nfor (; x; )\n  return 1;")

	// Can substitute an expression without side effects into a branch due to optional chaining
	check("let x = arg0; return arg1?.[x];", "return arg1?.[arg0];")
	check("let x = arg0; return arg1?.(x);", "return arg1?.(arg0);")

	// Cannot substitute an expression with side effects into a branch due to optional chaining,
	// since that would change the expression with side effects from being unconditionally
	// evaluated to being conditionally evaluated, which is a behavior change
	check("let x = fn(); return arg1?.[x];", "let x = fn();\nreturn arg1?.[x];")
	check("let x = fn(); return arg1?.(x);", "let x = fn();\nreturn arg1?.(x);")

	// Can substitute an expression past an optional chaining operation, since it has side effects
	check("let x = arg0; return arg1?.a === x;", "let x = arg0;\nreturn arg1?.a === x;")
	check("let x = arg0; return arg1?.[0] === x;", "let x = arg0;\nreturn arg1?.[0] === x;")
	check("let x = arg0; return arg1?.(0) === x;", "let x = arg0;\nreturn arg1?.(0) === x;")
	check("let x = arg0; return arg1?.a[x];", "let x = arg0;\nreturn arg1?.a[x];")
	check("let x = arg0; return arg1?.a(x);", "let x = arg0;\nreturn arg1?.a(x);")
	check("let x = arg0; return arg1?.[a][x];", "let x = arg0;\nreturn arg1?.[a][x];")
	check("let x = arg0; return arg1?.[a](x);", "let x = arg0;\nreturn arg1?.[a](x);")
	check("let x = arg0; return arg1?.(a)[x];", "let x = arg0;\nreturn arg1?.(a)[x];")
	check("let x = arg0; return arg1?.(a)(x);", "let x = arg0;\nreturn arg1?.(a)(x);")

	// Can substitute into an object as long as there are no side effects
	// beforehand. Note that computed properties must call "toString()" which
	// can have side effects.
	check("let x = arg0; return {x};", "return {x: arg0};")
	check("let x = arg0; return {x: y, y: x};", "let x = arg0;\nreturn {x: y, y: x};")
	check("let x = arg0; return {x: arg1, y: x};", "return {x: arg1, y: arg0};")
	check("let x = arg0; return {[x]: 0};", "return {[arg0]: 0};")
	check("let x = arg0; return {[y]: x};", "let x = arg0;\nreturn {[y]: x};")
	check("let x = arg0; return {[arg1]: x};", "let x = arg0;\nreturn {[arg1]: x};")
	check("let x = arg0; return {y() {}, x};", "return {y() {\n}, x: arg0};")
	check("let x = arg0; return {[y]() {}, x};", "let x = arg0;\nreturn {[y]() {\n}, x};")
	check("let x = arg0; return {...x};", "return {...arg0};")
	check("let x = arg0; return {...x, y};", "return {...arg0, y};")
	check("let x = arg0; return {x, ...y};", "return {x: arg0, ...y};")
	check("let x = arg0; return {...y, x};", "let x = arg0;\nreturn {...y, x};")

	// Check substitutions into template literals
	check("let x = arg0; return `a${x}b${y}c`;", "return `a${arg0}b${y}c`;")
	check("let x = arg0; return `a${y}b${x}c`;", "let x = arg0;\nreturn `a${y}b${x}c`;")
	check("let x = arg0; return `a${arg1}b${x}c`;", "return `a${arg1}b${arg0}c`;")
	check("let x = arg0; return x`y`;", "return arg0`y`;")
	check("let x = arg0; return y`a${x}b`;", "let x = arg0;\nreturn y`a${x}b`;")
	check("let x = arg0; return arg1`a${x}b`;", "return arg1`a${arg0}b`;")
	check("let x = 'x'; return `a${x}b`;", "return `axb`;")

	// Check substitutions into import expressions
	check("let x = arg0; return import(x);", "return import(arg0);")
	check("let x = arg0; return [import(y), x];", "let x = arg0;\nreturn [import(y), x];")
	check("let x = arg0; return [import(arg1), x];", "return [import(arg1), arg0];")

	// Check substitutions into await expressions
	check("return async () => { let x = arg0; await x; };", "return async () => {\n  await arg0;\n};")
	check("return async () => { let x = arg0; await y; return x; };", "return async () => {\n  let x = arg0;\n  return await y, x;\n};")
	check("return async () => { let x = arg0; await arg1; return x; };", "return async () => {\n  let x = arg0;\n  return await arg1, x;\n};")

	// Check substitutions into yield expressions
	check("return function* () { let x = arg0; yield x; };", "return function* () {\n  yield arg0;\n};")
	check("return function* () { let x = arg0; yield; return x; };", "return function* () {\n  let x = arg0;\n  return yield, x;\n};")
	check("return function* () { let x = arg0; yield y; return x; };", "return function* () {\n  let x = arg0;\n  return yield y, x;\n};")
	check("return function* () { let x = arg0; yield arg1; return x; };", "return function* () {\n  let x = arg0;\n  return yield arg1, x;\n};")

	// Make sure that transforms which duplicate identifiers cause
	// them to no longer be considered single-use identifiers
	expectPrintedMangleTarget(t, 2015, "(x => { let y = x; throw y ?? z })()", "((x) => {\n  let y = x;\n  throw y != null ? y : z;\n})();\n")
	expectPrintedMangleTarget(t, 2015, "(x => { let y = x; y.z ??= z })()", "((x) => {\n  var _a;\n  let y = x;\n  (_a = y.z) != null || (y.z = z);\n})();\n")
	expectPrintedMangleTarget(t, 2015, "(x => { let y = x; y?.z })()", "((x) => {\n  let y = x;\n  y == null || y.z;\n})();\n")
}

func TestTrimCodeInDeadControlFlow(t *testing.T) {
	expectPrintedMangle(t, "if (1) a(); else { ; }", "a();\n")
	expectPrintedMangle(t, "if (1) a(); else { b() }", "a();\n")
	expectPrintedMangle(t, "if (1) a(); else { const b = c }", "a();\n")
	expectPrintedMangle(t, "if (1) a(); else { let b }", "a();\n")
	expectPrintedMangle(t, "if (1) a(); else { throw b }", "a();\n")
	expectPrintedMangle(t, "if (1) a(); else { return b }", "a();\n")
	expectPrintedMangle(t, "b: { if (1) a(); else { break b } }", "b:\n  a();\n")
	expectPrintedMangle(t, "b: while (1) if (1) a(); else { continue b }", "b:\n  for (; ; )\n    a();\n")
	expectPrintedMangle(t, "if (1) a(); else { class b {} }", "a();\n")
	expectPrintedMangle(t, "if (1) a(); else { debugger }", "a();\n")

	expectPrintedMangle(t, "if (0) {let a = 1} else a()", "a();\n")
	expectPrintedMangle(t, "if (1) {let a = 1} else a()", "{\n  let a = 1;\n}\n")
	expectPrintedMangle(t, "if (0) a(); else {let a = 1}", "{\n  let a = 1;\n}\n")
	expectPrintedMangle(t, "if (1) a(); else {let a = 1}", "a();\n")

	expectPrintedMangle(t, "if (1) a(); else { var a = b }", "if (1)\n  a();\nelse\n  var a;\n")
	expectPrintedMangle(t, "if (1) a(); else { var [a] = b }", "if (1)\n  a();\nelse\n  var a;\n")
	expectPrintedMangle(t, "if (1) a(); else { var {x: a} = b }", "if (1)\n  a();\nelse\n  var a;\n")
	expectPrintedMangle(t, "if (1) a(); else { function a() {} }", "if (1)\n  a();\nelse {\n  function a() {\n  }\n}\n")
	expectPrintedMangle(t, "if (1) a(); else { for(;;){var a} }", "if (1)\n  a();\nelse\n  for (; ; )\n    var a;\n")
	expectPrintedMangle(t, "if (1) { a(); b() } else { var a; var b; }", "if (1)\n  a(), b();\nelse\n  var a, b;\n")
}

func TestPreservedComments(t *testing.T) {
	expectPrinted(t, "//", "")
	expectPrinted(t, "//preserve", "")
	expectPrinted(t, "//@__PURE__", "")
	expectPrinted(t, "//!", "//!\n")
	expectPrinted(t, "//@license", "//@license\n")
	expectPrinted(t, "//@preserve", "//@preserve\n")

	expectPrinted(t, "/**/", "")
	expectPrinted(t, "/*preserve*/", "")
	expectPrinted(t, "/*@__PURE__*/", "")
	expectPrinted(t, "/*!*/", "/*!*/\n")
	expectPrinted(t, "/*@license*/", "/*@license*/\n")
	expectPrinted(t, "/*@preserve*/", "/*@preserve*/\n")

	expectPrinted(t, "foo() //! test", "foo();\n//! test\n")
	expectPrinted(t, "//! test\nfoo()", "//! test\nfoo();\n")
	expectPrinted(t, "if (1) //! test\nfoo()", "if (1)\n  foo();\n")
	expectPrinted(t, "if (1) {//! test\nfoo()}", "if (1) {\n  //! test\n  foo();\n}\n")
	expectPrinted(t, "if (1) {foo() //! test\n}", "if (1) {\n  foo();\n  //! test\n}\n")

	expectPrinted(t, "    /*!\r     * Re-indent test\r     */", "/*!\n * Re-indent test\n */\n")
	expectPrinted(t, "    /*!\n     * Re-indent test\n     */", "/*!\n * Re-indent test\n */\n")
	expectPrinted(t, "    /*!\r\n     * Re-indent test\r\n     */", "/*!\n * Re-indent test\n */\n")
	expectPrinted(t, "    /*!\u2028     * Re-indent test\u2028     */", "/*!\n * Re-indent test\n */\n")
	expectPrinted(t, "    /*!\u2029     * Re-indent test\u2029     */", "/*!\n * Re-indent test\n */\n")

	expectPrinted(t, "\t\t/*!\r\t\t * Re-indent test\r\t\t */", "/*!\n * Re-indent test\n */\n")
	expectPrinted(t, "\t\t/*!\n\t\t * Re-indent test\n\t\t */", "/*!\n * Re-indent test\n */\n")
	expectPrinted(t, "\t\t/*!\r\n\t\t * Re-indent test\r\n\t\t */", "/*!\n * Re-indent test\n */\n")
	expectPrinted(t, "\t\t/*!\u2028\t\t * Re-indent test\u2028\t\t */", "/*!\n * Re-indent test\n */\n")
	expectPrinted(t, "\t\t/*!\u2029\t\t * Re-indent test\u2029\t\t */", "/*!\n * Re-indent test\n */\n")

	expectPrinted(t, "x\r    /*!\r     * Re-indent test\r     */", "x;\n/*!\n * Re-indent test\n */\n")
	expectPrinted(t, "x\n    /*!\n     * Re-indent test\n     */", "x;\n/*!\n * Re-indent test\n */\n")
	expectPrinted(t, "x\r\n    /*!\r\n     * Re-indent test\r\n     */", "x;\n/*!\n * Re-indent test\n */\n")
	expectPrinted(t, "x\u2028    /*!\u2028     * Re-indent test\u2028     */", "x;\n/*!\n * Re-indent test\n */\n")
	expectPrinted(t, "x\u2029    /*!\u2029     * Re-indent test\u2029     */", "x;\n/*!\n * Re-indent test\n */\n")
}

func TestUnicodeWhitespace(t *testing.T) {
	whitespace := []string{
		"\u0009", // character tabulation
		"\u000B", // line tabulation
		"\u000C", // form feed
		"\u0020", // space
		"\u00A0", // no-break space
		"\u1680", // ogham space mark
		"\u2000", // en quad
		"\u2001", // em quad
		"\u2002", // en space
		"\u2003", // em space
		"\u2004", // three-per-em space
		"\u2005", // four-per-em space
		"\u2006", // six-per-em space
		"\u2007", // figure space
		"\u2008", // punctuation space
		"\u2009", // thin space
		"\u200A", // hair space
		"\u202F", // narrow no-break space
		"\u205F", // medium mathematical space
		"\u3000", // ideographic space
		"\uFEFF", // zero width non-breaking space
	}

	// Test "js_lexer.Next()"
	expectParseError(t, "var\u0008x", "<stdin>: error: Expected identifier but found \"\\b\"\n")
	for _, s := range whitespace {
		expectPrinted(t, "var"+s+"x", "var x;\n")
	}

	// Test "js_lexer.NextInsideJSXElement()"
	expectParseErrorJSX(t, "<x\u0008y/>", "<stdin>: error: Expected \">\" but found \"\\b\"\n")
	for _, s := range whitespace {
		expectPrintedJSX(t, "<x"+s+"y/>", "/* @__PURE__ */ React.createElement(\"x\", {\n  y: true\n});\n")
	}

	// Test "js_lexer.NextJSXElementChild()"
	expectPrintedJSX(t, "<x>\n\u0008\n</x>", "/* @__PURE__ */ React.createElement(\"x\", null, \"\\b\");\n")
	for _, s := range whitespace {
		expectPrintedJSX(t, "<x>\n"+s+"\n</x>", "/* @__PURE__ */ React.createElement(\"x\", null);\n")
	}

	// Test "fixWhitespaceAndDecodeJSXEntities()"
	expectPrintedJSX(t, "<x>\n\u0008&quot;\n</x>", "/* @__PURE__ */ React.createElement(\"x\", null, '\\b\"');\n")
	for _, s := range whitespace {
		expectPrintedJSX(t, "<x>\n"+s+"&quot;\n</x>", "/* @__PURE__ */ React.createElement(\"x\", null, '\"');\n")
	}

	invalidWhitespaceInJS := []string{
		"\u0085", // next line (nel)
	}

	// Test "js_lexer.Next()"
	for _, s := range invalidWhitespaceInJS {
		r, _ := js_lexer.DecodeWTF8Rune(s)
		expectParseError(t, "var"+s+"x", fmt.Sprintf("<stdin>: error: Expected identifier but found \"\\u%04x\"\n", r))
	}

	// Test "js_lexer.NextInsideJSXElement()"
	for _, s := range invalidWhitespaceInJS {
		r, _ := js_lexer.DecodeWTF8Rune(s)
		expectParseErrorJSX(t, "<x"+s+"y/>", fmt.Sprintf("<stdin>: error: Expected \">\" but found \"\\u%04x\"\n", r))
	}

	// Test "js_lexer.NextJSXElementChild()"
	for _, s := range invalidWhitespaceInJS {
		expectPrintedJSX(t, "<x>\n"+s+"\n</x>", "/* @__PURE__ */ React.createElement(\"x\", null, \""+s+"\");\n")
	}

	// Test "fixWhitespaceAndDecodeJSXEntities()"
	for _, s := range invalidWhitespaceInJS {
		expectPrintedJSX(t, "<x>\n"+s+"&quot;\n</x>", "/* @__PURE__ */ React.createElement(\"x\", null, '"+s+"\"');\n")
	}
}

// Make sure we can handle the unicode replacement character "�" in various places
func TestReplacementCharacter(t *testing.T) {
	expectPrinted(t, "//\uFFFD\n123", "123;\n")
	expectPrinted(t, "/*\uFFFD*/123", "123;\n")

	expectPrinted(t, "'\uFFFD'", "\"\uFFFD\";\n")
	expectPrinted(t, "\"\uFFFD\"", "\"\uFFFD\";\n")
	expectPrinted(t, "`\uFFFD`", "`\uFFFD`;\n")
	expectPrinted(t, "/\uFFFD/", "/\uFFFD/;\n")

	expectPrintedJSX(t, "<a>\uFFFD</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"\uFFFD\");\n")
}

func TestNewTarget(t *testing.T) {
	expectPrinted(t, "new.target", "new.target;\n")
	expectPrinted(t, "(new.target)", "new.target;\n")
}

func TestJSX(t *testing.T) {
	expectParseError(t, "<a/>", "<stdin>: error: Unexpected \"<\"\n")

	expectPrintedJSX(t, "<a/>", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "<a></a>", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "<A/>", "/* @__PURE__ */ React.createElement(A, null);\n")
	expectPrintedJSX(t, "<a.b/>", "/* @__PURE__ */ React.createElement(a.b, null);\n")
	expectPrintedJSX(t, "<_a/>", "/* @__PURE__ */ React.createElement(_a, null);\n")
	expectPrintedJSX(t, "<a-b/>", "/* @__PURE__ */ React.createElement(\"a-b\", null);\n")

	expectPrintedJSX(t, "<a b/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: true\n});\n")
	expectPrintedJSX(t, "<a b=\"\\\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"\\\\\"\n});\n")
	expectPrintedJSX(t, "<a b=\"<>\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"<>\"\n});\n")
	expectPrintedJSX(t, "<a b=\"&lt;&gt;\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"<>\"\n});\n")
	expectPrintedJSX(t, "<a b=\"&wrong;\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"&wrong;\"\n});\n")
	expectPrintedJSX(t, "<a b={1, 2}/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: (1, 2)\n});\n")
	expectPrintedJSX(t, "<a b={<c/>}/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: /* @__PURE__ */ React.createElement(\"c\", null)\n});\n")
	expectPrintedJSX(t, "<a {...props}/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  ...props\n});\n")
	expectPrintedJSX(t, "<a b=\"🙂\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"🙂\"\n});\n")

	expectPrintedJSX(t, "<a>\n</a>", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "<a>123</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"123\");\n")
	expectPrintedJSX(t, "<a>}</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"}\");\n")
	expectPrintedJSX(t, "<a>=</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"=\");\n")
	expectPrintedJSX(t, "<a>></a>", "/* @__PURE__ */ React.createElement(\"a\", null, \">\");\n")
	expectPrintedJSX(t, "<a>>=</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \">=\");\n")
	expectPrintedJSX(t, "<a>>></a>", "/* @__PURE__ */ React.createElement(\"a\", null, \">>\");\n")
	expectPrintedJSX(t, "<a>{}</a>", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "<a>{/* comment */}</a>", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "<a>{1, 2}</a>", "/* @__PURE__ */ React.createElement(\"a\", null, (1, 2));\n")
	expectPrintedJSX(t, "<a>&lt;&gt;</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"<>\");\n")
	expectPrintedJSX(t, "<a>&wrong;</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"&wrong;\");\n")
	expectPrintedJSX(t, "<a>🙂</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"🙂\");\n")

	// Note: The TypeScript compiler and Babel disagree. This matches TypeScript.
	expectPrintedJSX(t, "<a b=\"   c\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"   c\"\n});\n")
	expectPrintedJSX(t, "<a b=\"   \nc\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"   \\nc\"\n});\n")
	expectPrintedJSX(t, "<a b=\"\n   c\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"\\n   c\"\n});\n")
	expectPrintedJSX(t, "<a b=\"c   \"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"c   \"\n});\n")
	expectPrintedJSX(t, "<a b=\"c   \n\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"c   \\n\"\n});\n")
	expectPrintedJSX(t, "<a b=\"c\n   \"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"c\\n   \"\n});\n")
	expectPrintedJSX(t, "<a b=\"c   d\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"c   d\"\n});\n")
	expectPrintedJSX(t, "<a b=\"c   \nd\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"c   \\nd\"\n});\n")
	expectPrintedJSX(t, "<a b=\"c\n   d\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"c\\n   d\"\n});\n")
	expectPrintedJSX(t, "<a b=\"   c\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"   c\"\n});\n")
	expectPrintedJSX(t, "<a b=\"   \nc\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"   \\nc\"\n});\n")
	expectPrintedJSX(t, "<a b=\"\n   c\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"\\n   c\"\n});\n")

	// Same test as above except with multi-byte Unicode characters
	expectPrintedJSX(t, "<a b=\"   🙂\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"   🙂\"\n});\n")
	expectPrintedJSX(t, "<a b=\"   \n🙂\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"   \\n🙂\"\n});\n")
	expectPrintedJSX(t, "<a b=\"\n   🙂\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"\\n   🙂\"\n});\n")
	expectPrintedJSX(t, "<a b=\"🙂   \"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"🙂   \"\n});\n")
	expectPrintedJSX(t, "<a b=\"🙂   \n\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"🙂   \\n\"\n});\n")
	expectPrintedJSX(t, "<a b=\"🙂\n   \"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"🙂\\n   \"\n});\n")
	expectPrintedJSX(t, "<a b=\"🙂   🍕\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"🙂   🍕\"\n});\n")
	expectPrintedJSX(t, "<a b=\"🙂   \n🍕\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"🙂   \\n🍕\"\n});\n")
	expectPrintedJSX(t, "<a b=\"🙂\n   🍕\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"🙂\\n   🍕\"\n});\n")
	expectPrintedJSX(t, "<a b=\"   🙂\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"   🙂\"\n});\n")
	expectPrintedJSX(t, "<a b=\"   \n🙂\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"   \\n🙂\"\n});\n")
	expectPrintedJSX(t, "<a b=\"\n   🙂\"/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  b: \"\\n   🙂\"\n});\n")

	expectPrintedJSX(t, "<a>   b</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"   b\");\n")
	expectPrintedJSX(t, "<a>   \nb</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"b\");\n")
	expectPrintedJSX(t, "<a>\n   b</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"b\");\n")
	expectPrintedJSX(t, "<a>b   </a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"b   \");\n")
	expectPrintedJSX(t, "<a>b   \n</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"b\");\n")
	expectPrintedJSX(t, "<a>b\n   </a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"b\");\n")
	expectPrintedJSX(t, "<a>b   c</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"b   c\");\n")
	expectPrintedJSX(t, "<a>b   \nc</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"b c\");\n")
	expectPrintedJSX(t, "<a>b\n   c</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"b c\");\n")
	expectPrintedJSX(t, "<a>   b</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"   b\");\n")
	expectPrintedJSX(t, "<a>   \nb</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"b\");\n")
	expectPrintedJSX(t, "<a>\n   b</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"b\");\n")

	// Same test as above except with multi-byte Unicode characters
	expectPrintedJSX(t, "<a>   🙂</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"   🙂\");\n")
	expectPrintedJSX(t, "<a>   \n🙂</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"🙂\");\n")
	expectPrintedJSX(t, "<a>\n   🙂</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"🙂\");\n")
	expectPrintedJSX(t, "<a>🙂   </a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"🙂   \");\n")
	expectPrintedJSX(t, "<a>🙂   \n</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"🙂\");\n")
	expectPrintedJSX(t, "<a>🙂\n   </a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"🙂\");\n")
	expectPrintedJSX(t, "<a>🙂   🍕</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"🙂   🍕\");\n")
	expectPrintedJSX(t, "<a>🙂   \n🍕</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"🙂 🍕\");\n")
	expectPrintedJSX(t, "<a>🙂\n   🍕</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"🙂 🍕\");\n")
	expectPrintedJSX(t, "<a>   🙂</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"   🙂\");\n")
	expectPrintedJSX(t, "<a>   \n🙂</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"🙂\");\n")
	expectPrintedJSX(t, "<a>\n   🙂</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"🙂\");\n")

	// "<a>{x}</b></a>" with all combinations of "", " ", and "\n" inserted in between
	expectPrintedJSX(t, "<a>{x}<b/></a>;", "/* @__PURE__ */ React.createElement(\"a\", null, x, /* @__PURE__ */ React.createElement(\"b\", null));\n")
	expectPrintedJSX(t, "<a>\n{x}<b/></a>;", "/* @__PURE__ */ React.createElement(\"a\", null, x, /* @__PURE__ */ React.createElement(\"b\", null));\n")
	expectPrintedJSX(t, "<a>{x}\n<b/></a>;", "/* @__PURE__ */ React.createElement(\"a\", null, x, /* @__PURE__ */ React.createElement(\"b\", null));\n")
	expectPrintedJSX(t, "<a>\n{x}\n<b/></a>;", "/* @__PURE__ */ React.createElement(\"a\", null, x, /* @__PURE__ */ React.createElement(\"b\", null));\n")
	expectPrintedJSX(t, "<a>{x}<b/>\n</a>;", "/* @__PURE__ */ React.createElement(\"a\", null, x, /* @__PURE__ */ React.createElement(\"b\", null));\n")
	expectPrintedJSX(t, "<a>\n{x}<b/>\n</a>;", "/* @__PURE__ */ React.createElement(\"a\", null, x, /* @__PURE__ */ React.createElement(\"b\", null));\n")
	expectPrintedJSX(t, "<a>{x}\n<b/>\n</a>;", "/* @__PURE__ */ React.createElement(\"a\", null, x, /* @__PURE__ */ React.createElement(\"b\", null));\n")
	expectPrintedJSX(t, "<a>\n{x}\n<b/>\n</a>;", "/* @__PURE__ */ React.createElement(\"a\", null, x, /* @__PURE__ */ React.createElement(\"b\", null));\n")
	expectPrintedJSX(t, "<a> {x}<b/></a>;", "/* @__PURE__ */ React.createElement(\"a\", null, \" \", x, /* @__PURE__ */ React.createElement(\"b\", null));\n")
	expectPrintedJSX(t, "<a> {x}\n<b/></a>;", "/* @__PURE__ */ React.createElement(\"a\", null, \" \", x, /* @__PURE__ */ React.createElement(\"b\", null));\n")
	expectPrintedJSX(t, "<a> {x}<b/>\n</a>;", "/* @__PURE__ */ React.createElement(\"a\", null, \" \", x, /* @__PURE__ */ React.createElement(\"b\", null));\n")
	expectPrintedJSX(t, "<a> {x}\n<b/>\n</a>;", "/* @__PURE__ */ React.createElement(\"a\", null, \" \", x, /* @__PURE__ */ React.createElement(\"b\", null));\n")
	expectPrintedJSX(t, "<a>{x} <b/></a>;", "/* @__PURE__ */ React.createElement(\"a\", null, x, \" \", /* @__PURE__ */ React.createElement(\"b\", null));\n")
	expectPrintedJSX(t, "<a>\n{x} <b/></a>;", "/* @__PURE__ */ React.createElement(\"a\", null, x, \" \", /* @__PURE__ */ React.createElement(\"b\", null));\n")
	expectPrintedJSX(t, "<a>{x} <b/>\n</a>;", "/* @__PURE__ */ React.createElement(\"a\", null, x, \" \", /* @__PURE__ */ React.createElement(\"b\", null));\n")
	expectPrintedJSX(t, "<a>\n{x} <b/>\n</a>;", "/* @__PURE__ */ React.createElement(\"a\", null, x, \" \", /* @__PURE__ */ React.createElement(\"b\", null));\n")
	expectPrintedJSX(t, "<a> {x} <b/></a>;", "/* @__PURE__ */ React.createElement(\"a\", null, \" \", x, \" \", /* @__PURE__ */ React.createElement(\"b\", null));\n")
	expectPrintedJSX(t, "<a> {x} <b/>\n</a>;", "/* @__PURE__ */ React.createElement(\"a\", null, \" \", x, \" \", /* @__PURE__ */ React.createElement(\"b\", null));\n")
	expectPrintedJSX(t, "<a>{x}<b/> </a>;", "/* @__PURE__ */ React.createElement(\"a\", null, x, /* @__PURE__ */ React.createElement(\"b\", null), \" \");\n")
	expectPrintedJSX(t, "<a>\n{x}<b/> </a>;", "/* @__PURE__ */ React.createElement(\"a\", null, x, /* @__PURE__ */ React.createElement(\"b\", null), \" \");\n")
	expectPrintedJSX(t, "<a>{x}\n<b/> </a>;", "/* @__PURE__ */ React.createElement(\"a\", null, x, /* @__PURE__ */ React.createElement(\"b\", null), \" \");\n")
	expectPrintedJSX(t, "<a>\n{x}\n<b/> </a>;", "/* @__PURE__ */ React.createElement(\"a\", null, x, /* @__PURE__ */ React.createElement(\"b\", null), \" \");\n")
	expectPrintedJSX(t, "<a> {x}<b/> </a>;", "/* @__PURE__ */ React.createElement(\"a\", null, \" \", x, /* @__PURE__ */ React.createElement(\"b\", null), \" \");\n")
	expectPrintedJSX(t, "<a> {x}\n<b/> </a>;", "/* @__PURE__ */ React.createElement(\"a\", null, \" \", x, /* @__PURE__ */ React.createElement(\"b\", null), \" \");\n")
	expectPrintedJSX(t, "<a>{x} <b/> </a>;", "/* @__PURE__ */ React.createElement(\"a\", null, x, \" \", /* @__PURE__ */ React.createElement(\"b\", null), \" \");\n")
	expectPrintedJSX(t, "<a>\n{x} <b/> </a>;", "/* @__PURE__ */ React.createElement(\"a\", null, x, \" \", /* @__PURE__ */ React.createElement(\"b\", null), \" \");\n")
	expectPrintedJSX(t, "<a> {x} <b/> </a>;", "/* @__PURE__ */ React.createElement(\"a\", null, \" \", x, \" \", /* @__PURE__ */ React.createElement(\"b\", null), \" \");\n")

	expectParseErrorJSX(t, "<a b=true/>", "<stdin>: error: Expected \"{\" but found \"true\"\n")
	expectParseErrorJSX(t, "</a>", "<stdin>: error: Expected identifier but found \"/\"\n")
	expectParseErrorJSX(t, "<></b>", "<stdin>: error: Expected closing tag \"b\" to match opening tag \"\"\n<stdin>: note: The opening tag \"\" is here\n")
	expectParseErrorJSX(t, "<a></>", "<stdin>: error: Expected closing tag \"\" to match opening tag \"a\"\n<stdin>: note: The opening tag \"a\" is here\n")
	expectParseErrorJSX(t, "<a></b>", "<stdin>: error: Expected closing tag \"b\" to match opening tag \"a\"\n<stdin>: note: The opening tag \"a\" is here\n")
	expectParseErrorJSX(t, "<\na\n.\nb\n>\n<\n/\nc\n.\nd\n>",
		"<stdin>: error: Expected closing tag \"c.d\" to match opening tag \"a.b\"\n<stdin>: note: The opening tag \"a.b\" is here\n")
	expectParseErrorJSX(t, "<a-b.c>", "<stdin>: error: Expected \">\" but found \".\"\n")
	expectParseErrorJSX(t, "<a.b-c>", "<stdin>: error: Unexpected \"-\"\n")
	expectParseErrorJSX(t, "<a:b>", "<stdin>: error: Expected \">\" but found \":\"\n")
	expectParseErrorJSX(t, "<a>{...children}</a>", "<stdin>: error: Unexpected \"...\"\n")

	expectPrintedJSX(t, "< /**/ a/>", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "< //\n a/>", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "<a /**/ />", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "<a //\n />", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "<a/ /**/ >", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "<a/ //\n >", "/* @__PURE__ */ React.createElement(\"a\", null);\n")

	expectPrintedJSX(t, "<a>b< /**/ /a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"b\");\n")
	expectPrintedJSX(t, "<a>b< //\n /a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"b\");\n")
	expectPrintedJSX(t, "<a>b</ /**/ a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"b\");\n")
	expectPrintedJSX(t, "<a>b</ //\n a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"b\");\n")
	expectPrintedJSX(t, "<a>b</a /**/ >", "/* @__PURE__ */ React.createElement(\"a\", null, \"b\");\n")
	expectPrintedJSX(t, "<a>b</a //\n >", "/* @__PURE__ */ React.createElement(\"a\", null, \"b\");\n")

	expectPrintedJSX(t, "<a> /**/ </a>", "/* @__PURE__ */ React.createElement(\"a\", null, \" /**/ \");\n")
	expectPrintedJSX(t, "<a> //\n </a>", "/* @__PURE__ */ React.createElement(\"a\", null, \" //\");\n")

	// Unicode tests
	expectPrintedJSX(t, "<\U00020000/>", "/* @__PURE__ */ React.createElement(\U00020000, null);\n")
	expectPrintedJSX(t, "<a>\U00020000</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"\U00020000\");\n")
	expectPrintedJSX(t, "<a \U00020000={0}/>", "/* @__PURE__ */ React.createElement(\"a\", {\n  \U00020000: 0\n});\n")

	// Comment tests
	expectParseErrorJSX(t, "<a /* />", "<stdin>: error: Expected \"*/\" to terminate multi-line comment\n<stdin>: note: The multi-line comment starts here\n")
	expectParseErrorJSX(t, "<a /*/ />", "<stdin>: error: Expected \"*/\" to terminate multi-line comment\n<stdin>: note: The multi-line comment starts here\n")
	expectParseErrorJSX(t, "<a // />", "<stdin>: error: Expected \">\" but found end of file\n")
	expectParseErrorJSX(t, "<a /**/>", "<stdin>: error: Unexpected end of file\n")
	expectParseErrorJSX(t, "<a /**/ />", "")
	expectParseErrorJSX(t, "<a // \n />", "")
	expectParseErrorJSX(t, "<a b/* />", "<stdin>: error: Expected \"*/\" to terminate multi-line comment\n<stdin>: note: The multi-line comment starts here\n")
	expectParseErrorJSX(t, "<a b/*/ />", "<stdin>: error: Expected \"*/\" to terminate multi-line comment\n<stdin>: note: The multi-line comment starts here\n")
	expectParseErrorJSX(t, "<a b// />", "<stdin>: error: Expected \">\" but found end of file\n")
	expectParseErrorJSX(t, "<a b/**/>", "<stdin>: error: Unexpected end of file\n")
	expectParseErrorJSX(t, "<a b/**/ />", "")
	expectParseErrorJSX(t, "<a b// \n />", "")
}

func TestJSXPragmas(t *testing.T) {
	expectPrintedJSX(t, "// @jsx h\n<a/>", "/* @__PURE__ */ h(\"a\", null);\n")
	expectPrintedJSX(t, "/* @jsx h */\n<a/>", "/* @__PURE__ */ h(\"a\", null);\n")
	expectPrintedJSX(t, "<a/>\n// @jsx h", "/* @__PURE__ */ h(\"a\", null);\n")
	expectPrintedJSX(t, "<a/>\n/* @jsx h */", "/* @__PURE__ */ h(\"a\", null);\n")
	expectPrintedJSX(t, "// @jsx a.b.c\n<a/>", "/* @__PURE__ */ a.b.c(\"a\", null);\n")
	expectPrintedJSX(t, "/* @jsx a.b.c */\n<a/>", "/* @__PURE__ */ a.b.c(\"a\", null);\n")

	expectPrintedJSX(t, "// @jsxFrag f\n<></>", "/* @__PURE__ */ React.createElement(f, null);\n")
	expectPrintedJSX(t, "/* @jsxFrag f */\n<></>", "/* @__PURE__ */ React.createElement(f, null);\n")
	expectPrintedJSX(t, "<></>\n// @jsxFrag f", "/* @__PURE__ */ React.createElement(f, null);\n")
	expectPrintedJSX(t, "<></>\n/* @jsxFrag f */", "/* @__PURE__ */ React.createElement(f, null);\n")
	expectPrintedJSX(t, "// @jsxFrag a.b.c\n<></>", "/* @__PURE__ */ React.createElement(a.b.c, null);\n")
	expectPrintedJSX(t, "/* @jsxFrag a.b.c */\n<></>", "/* @__PURE__ */ React.createElement(a.b.c, null);\n")
}

func TestPreserveOptionalChainParentheses(t *testing.T) {
	expectPrinted(t, "a?.b.c", "a?.b.c;\n")
	expectPrinted(t, "(a?.b).c", "(a?.b).c;\n")
	expectPrinted(t, "a?.b.c.d", "a?.b.c.d;\n")
	expectPrinted(t, "(a?.b.c).d", "(a?.b.c).d;\n")
	expectPrinted(t, "a?.b[c]", "a?.b[c];\n")
	expectPrinted(t, "(a?.b)[c]", "(a?.b)[c];\n")
	expectPrinted(t, "a?.b(c)", "a?.b(c);\n")
	expectPrinted(t, "(a?.b)(c)", "(a?.b)(c);\n")

	expectPrinted(t, "a?.[b][c]", "a?.[b][c];\n")
	expectPrinted(t, "(a?.[b])[c]", "(a?.[b])[c];\n")
	expectPrinted(t, "a?.[b][c][d]", "a?.[b][c][d];\n")
	expectPrinted(t, "(a?.[b][c])[d]", "(a?.[b][c])[d];\n")
	expectPrinted(t, "a?.[b].c", "a?.[b].c;\n")
	expectPrinted(t, "(a?.[b]).c", "(a?.[b]).c;\n")
	expectPrinted(t, "a?.[b](c)", "a?.[b](c);\n")
	expectPrinted(t, "(a?.[b])(c)", "(a?.[b])(c);\n")

	expectPrinted(t, "a?.(b)(c)", "a?.(b)(c);\n")
	expectPrinted(t, "(a?.(b))(c)", "(a?.(b))(c);\n")
	expectPrinted(t, "a?.(b)(c)(d)", "a?.(b)(c)(d);\n")
	expectPrinted(t, "(a?.(b)(c))(d)", "(a?.(b)(c))(d);\n")
	expectPrinted(t, "a?.(b).c", "a?.(b).c;\n")
	expectPrinted(t, "(a?.(b)).c", "(a?.(b)).c;\n")
	expectPrinted(t, "a?.(b)[c]", "a?.(b)[c];\n")
	expectPrinted(t, "(a?.(b))[c]", "(a?.(b))[c];\n")
}

func TestPrivateIdentifiers(t *testing.T) {
	expectParseError(t, "#foo", "<stdin>: error: Unexpected \"#foo\"\n")
	expectParseError(t, "this.#foo", "<stdin>: error: Expected identifier but found \"#foo\"\n")
	expectParseError(t, "this?.#foo", "<stdin>: error: Expected identifier but found \"#foo\"\n")
	expectParseError(t, "({ #foo: 1 })", "<stdin>: error: Expected identifier but found \"#foo\"\n")
	expectParseError(t, "class Foo { x = { #foo: 1 } }", "<stdin>: error: Expected identifier but found \"#foo\"\n")
	expectParseError(t, "class Foo { #foo; foo() { delete this.#foo } }",
		"<stdin>: error: Deleting the private name \"#foo\" is forbidden\n")
	expectParseError(t, "class Foo { #foo; foo() { delete this?.#foo } }",
		"<stdin>: error: Deleting the private name \"#foo\" is forbidden\n")
	expectParseError(t, "class Foo extends Bar { #foo; foo() { super.#foo } }",
		"<stdin>: error: Expected identifier but found \"#foo\"\n")

	expectPrinted(t, "class Foo { #foo }", "class Foo {\n  #foo;\n}\n")
	expectPrinted(t, "class Foo { #foo = 1 }", "class Foo {\n  #foo = 1;\n}\n")
	expectPrinted(t, "class Foo { #foo() {} }", "class Foo {\n  #foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { get #foo() {} }", "class Foo {\n  get #foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { set #foo(x) {} }", "class Foo {\n  set #foo(x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { static #foo }", "class Foo {\n  static #foo;\n}\n")
	expectPrinted(t, "class Foo { static #foo = 1 }", "class Foo {\n  static #foo = 1;\n}\n")
	expectPrinted(t, "class Foo { static #foo() {} }", "class Foo {\n  static #foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static get #foo() {} }", "class Foo {\n  static get #foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static set #foo(x) {} }", "class Foo {\n  static set #foo(x) {\n  }\n}\n")

	// The name "#constructor" is forbidden
	expectParseError(t, "class Foo { #constructor }", "<stdin>: error: Invalid field name \"#constructor\"\n")
	expectParseError(t, "class Foo { #constructor() {} }", "<stdin>: error: Invalid method name \"#constructor\"\n")
	expectParseError(t, "class Foo { static #constructor }", "<stdin>: error: Invalid field name \"#constructor\"\n")
	expectParseError(t, "class Foo { static #constructor() {} }", "<stdin>: error: Invalid method name \"#constructor\"\n")
	expectParseError(t, "class Foo { #\\u0063onstructor }", "<stdin>: error: Invalid field name \"#constructor\"\n")
	expectParseError(t, "class Foo { #\\u0063onstructor() {} }", "<stdin>: error: Invalid method name \"#constructor\"\n")
	expectParseError(t, "class Foo { static #\\u0063onstructor }", "<stdin>: error: Invalid field name \"#constructor\"\n")
	expectParseError(t, "class Foo { static #\\u0063onstructor() {} }", "<stdin>: error: Invalid method name \"#constructor\"\n")

	// Test escape sequences
	expectPrinted(t, "class Foo { #\\u0066oo; foo = this.#foo }", "class Foo {\n  #foo;\n  foo = this.#foo;\n}\n")
	expectPrinted(t, "class Foo { #fo\\u006f; foo = this.#foo }", "class Foo {\n  #foo;\n  foo = this.#foo;\n}\n")
	expectParseError(t, "class Foo { #\\u0020oo }", "<stdin>: error: Invalid identifier: \"# oo\"\n")
	expectParseError(t, "class Foo { #fo\\u0020 }", "<stdin>: error: Invalid identifier: \"#fo \"\n")

	errorText := `<stdin>: error: "#foo" has already been declared
<stdin>: note: "#foo" was originally declared here
`

	// Scope tests
	expectParseError(t, "class Foo { #foo; #foo }", errorText)
	expectParseError(t, "class Foo { #foo; static #foo }", errorText)
	expectParseError(t, "class Foo { static #foo; #foo }", errorText)
	expectParseError(t, "class Foo { #foo; #foo() {} }", errorText)
	expectParseError(t, "class Foo { #foo; get #foo() {} }", errorText)
	expectParseError(t, "class Foo { #foo; set #foo(x) {} }", errorText)
	expectParseError(t, "class Foo { #foo() {} #foo }", errorText)
	expectParseError(t, "class Foo { get #foo() {} #foo }", errorText)
	expectParseError(t, "class Foo { set #foo(x) {} #foo }", errorText)
	expectParseError(t, "class Foo { get #foo() {} get #foo() {} }", errorText)
	expectParseError(t, "class Foo { set #foo(x) {} set #foo(x) {} }", errorText)
	expectParseError(t, "class Foo { get #foo() {} set #foo(x) {} #foo }", errorText)
	expectParseError(t, "class Foo { set #foo(x) {} get #foo() {} #foo }", errorText)
	expectPrinted(t, "class Foo { get #foo() {} set #foo(x) { this.#foo } }",
		"class Foo {\n  get #foo() {\n  }\n  set #foo(x) {\n    this.#foo;\n  }\n}\n")
	expectPrinted(t, "class Foo { set #foo(x) { this.#foo } get #foo() {} }",
		"class Foo {\n  set #foo(x) {\n    this.#foo;\n  }\n  get #foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { #foo } class Bar { #foo }", "class Foo {\n  #foo;\n}\nclass Bar {\n  #foo;\n}\n")
	expectPrinted(t, "class Foo { foo = this.#foo; #foo }", "class Foo {\n  foo = this.#foo;\n  #foo;\n}\n")
	expectPrinted(t, "class Foo { foo = this?.#foo; #foo }", "class Foo {\n  foo = this?.#foo;\n  #foo;\n}\n")
	expectParseError(t, "class Foo { #foo } class Bar { foo = this.#foo }",
		"<stdin>: error: Private name \"#foo\" must be declared in an enclosing class\n")
	expectParseError(t, "class Foo { #foo } class Bar { foo = this?.#foo }",
		"<stdin>: error: Private name \"#foo\" must be declared in an enclosing class\n")

	// Getter and setter warnings
	expectParseError(t, "class Foo { get #x() { this.#x = 1 } }",
		"<stdin>: warning: Writing to getter-only property \"#x\" will throw\n")
	expectParseError(t, "class Foo { get #x() { this.#x += 1 } }",
		"<stdin>: warning: Writing to getter-only property \"#x\" will throw\n")
	expectParseError(t, "class Foo { set #x(x) { this.#x } }",
		"<stdin>: warning: Reading from setter-only property \"#x\" will throw\n")
	expectParseError(t, "class Foo { set #x(x) { this.#x += 1 } }",
		"<stdin>: warning: Reading from setter-only property \"#x\" will throw\n")

	expectPrinted(t, `class Foo {
	#if
	#im() { return this.#im(this.#if) }
	static #sf
	static #sm() { return this.#sm(this.#sf) }
	foo() {
		return class {
			#inner() {
				return [this.#im, this?.#inner, this?.x.#if]
			}
		}
	}
}
`, `class Foo {
  #if;
  #im() {
    return this.#im(this.#if);
  }
  static #sf;
  static #sm() {
    return this.#sm(this.#sf);
  }
  foo() {
    return class {
      #inner() {
        return [this.#im, this?.#inner, this?.x.#if];
      }
    };
  }
}
`)
}

func TestES5(t *testing.T) {
	expectParseErrorTarget(t, 5, "function foo(x = 0) {}",
		"<stdin>: error: Transforming default arguments to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(function(x = 0) {})",
		"<stdin>: error: Transforming default arguments to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(x = 0) => {}",
		"<stdin>: error: Transforming default arguments to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "function foo(...x) {}",
		"<stdin>: error: Transforming rest arguments to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(function(...x) {})",
		"<stdin>: error: Transforming rest arguments to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(...x) => {}",
		"<stdin>: error: Transforming rest arguments to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "foo(...x)",
		"<stdin>: error: Transforming rest arguments to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "[...x]",
		"<stdin>: error: Transforming array spread to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "for (var x of y) ;",
		"<stdin>: error: Transforming for-of loops to the configured target environment is not supported yet\n")
	expectPrintedTarget(t, 5, "({ x })", "({x: x});\n")
	expectParseErrorTarget(t, 5, "({ [x]: y })",
		"<stdin>: error: Transforming object literal extensions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "({ x() {} });",
		"<stdin>: error: Transforming object literal extensions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "({ get x() {} });", "")
	expectParseErrorTarget(t, 5, "({ set x(x) {} });", "")
	expectParseErrorTarget(t, 5, "({ get [x]() {} });",
		"<stdin>: error: Transforming object literal extensions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "({ set [x](x) {} });",
		"<stdin>: error: Transforming object literal extensions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "function foo([]) {}",
		"<stdin>: error: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "function foo({}) {}",
		"<stdin>: error: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(function([]) {})",
		"<stdin>: error: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(function({}) {})",
		"<stdin>: error: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "([]) => {}",
		"<stdin>: error: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "({}) => {}",
		"<stdin>: error: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "var [] = [];",
		"<stdin>: error: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "var {} = {};",
		"<stdin>: error: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "([] = []);",
		"<stdin>: error: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "({} = {});",
		"<stdin>: error: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "for ([] in []);",
		"<stdin>: error: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "for ({} in []);",
		"<stdin>: error: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "function foo([...x]) {}",
		"<stdin>: error: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(function([...x]) {})",
		"<stdin>: error: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "([...x]) => {}",
		"<stdin>: error: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "function foo([...[x]]) {}",
		`<stdin>: error: Transforming destructuring to the configured target environment is not supported yet
<stdin>: error: Transforming destructuring to the configured target environment is not supported yet
<stdin>: error: Transforming non-identifier array rest patterns to the configured target environment is not supported yet
`)
	expectParseErrorTarget(t, 5, "(function([...[x]]) {})",
		`<stdin>: error: Transforming destructuring to the configured target environment is not supported yet
<stdin>: error: Transforming destructuring to the configured target environment is not supported yet
<stdin>: error: Transforming non-identifier array rest patterns to the configured target environment is not supported yet
`)
	expectParseErrorTarget(t, 5, "([...[x]]) => {}",
		`<stdin>: error: Transforming destructuring to the configured target environment is not supported yet
<stdin>: error: Transforming destructuring to the configured target environment is not supported yet
<stdin>: error: Transforming non-identifier array rest patterns to the configured target environment is not supported yet
`)
	expectParseErrorTarget(t, 5, "([...[x]])",
		"<stdin>: error: Transforming array spread to the configured target environment is not supported yet\n")
	expectPrintedTarget(t, 5, "`abc`;", "\"abc\";\n")
	expectPrintedTarget(t, 5, "`a${b}`;", "\"a\" + b;\n")
	expectPrintedTarget(t, 5, "`${a}b`;", "a + \"b\";\n")
	expectPrintedTarget(t, 5, "`${a}${b}`;", "a + \"\" + b;\n")
	expectPrintedTarget(t, 5, "`a${b}c`;", "\"a\" + b + \"c\";\n")
	expectPrintedTarget(t, 5, "`a${b}${c}`;", "\"a\" + b + c;\n")
	expectPrintedTarget(t, 5, "`a${b}${c}d`;", "\"a\" + b + c + \"d\";\n")
	expectPrintedTarget(t, 5, "`a${b}c${d}`;", "\"a\" + b + \"c\" + d;\n")
	expectPrintedTarget(t, 5, "`a${b}c${d}e`;", "\"a\" + b + \"c\" + d + \"e\";\n")
	expectParseErrorTarget(t, 5, "tag`abc`;",
		"<stdin>: error: Transforming tagged template literals to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "tag`a${b}c`;",
		"<stdin>: error: Transforming tagged template literals to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "class Foo { constructor() { new.target } }",
		"<stdin>: error: Transforming class syntax to the configured target environment is not supported yet\n"+
			"<stdin>: error: Transforming object literal extensions to the configured target environment is not supported yet\n"+
			"<stdin>: error: Transforming new.target to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "const x = 1;",
		"<stdin>: error: Transforming const to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "let x = 2;",
		"<stdin>: error: Transforming let to the configured target environment is not supported yet\n")
	expectPrintedTarget(t, 5, "async => foo;", "(function(async) {\n  return foo;\n});\n")
	expectPrintedTarget(t, 5, "x => x;", "(function(x) {\n  return x;\n});\n")
	expectParseErrorTarget(t, 5, "async () => foo;",
		"<stdin>: error: Transforming async functions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "class Foo {}",
		"<stdin>: error: Transforming class syntax to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(class {});",
		"<stdin>: error: Transforming class syntax to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "function* gen() {}",
		"<stdin>: error: Transforming generator functions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(function* () {});",
		"<stdin>: error: Transforming generator functions to the configured target environment is not supported yet\n")
}

func TestASCIIOnly(t *testing.T) {
	es5 := "<stdin>: error: \"𐀀\" cannot be escaped in the target environment " +
		"(consider setting the charset to \"utf8\" or changing the target)\n"

	// Some context: "π" is in the BMP (i.e. has a code point ≤0xFFFF) and "𐀀" is
	// not in the BMP (i.e. has a code point >0xFFFF). This distinction matters
	// because it's impossible to escape non-BMP characters before ES6.

	expectPrinted(t, "π", "π;\n")
	expectPrinted(t, "𐀀", "𐀀;\n")
	expectPrintedASCII(t, "π", "\\u03C0;\n")
	expectPrintedASCII(t, "𐀀", "\\u{10000};\n")
	expectPrintedTargetASCII(t, 5, "π", "\\u03C0;\n")
	expectParseErrorTargetASCII(t, 5, "𐀀", es5)

	expectPrinted(t, "var π", "var π;\n")
	expectPrinted(t, "var 𐀀", "var 𐀀;\n")
	expectPrintedASCII(t, "var π", "var \\u03C0;\n")
	expectPrintedASCII(t, "var 𐀀", "var \\u{10000};\n")
	expectPrintedTargetASCII(t, 5, "var π", "var \\u03C0;\n")
	expectParseErrorTargetASCII(t, 5, "var 𐀀", es5)

	expectPrinted(t, "'π'", "\"π\";\n")
	expectPrinted(t, "'𐀀'", "\"𐀀\";\n")
	expectPrintedASCII(t, "'π'", "\"\\u03C0\";\n")
	expectPrintedASCII(t, "'𐀀'", "\"\\u{10000}\";\n")
	expectPrintedTargetASCII(t, 5, "'π'", "\"\\u03C0\";\n")
	expectPrintedTargetASCII(t, 5, "'𐀀'", "\"\\uD800\\uDC00\";\n")

	expectPrinted(t, "x.π", "x.π;\n")
	expectPrinted(t, "x.𐀀", "x.𐀀;\n")
	expectPrintedASCII(t, "x.π", "x.\\u03C0;\n")
	expectPrintedASCII(t, "x.𐀀", "x.\\u{10000};\n")
	expectPrintedTargetASCII(t, 5, "x.π", "x.\\u03C0;\n")
	expectPrintedTargetASCII(t, 5, "x.𐀀", "x[\"\\uD800\\uDC00\"];\n")

	expectPrinted(t, "x?.π", "x?.π;\n")
	expectPrinted(t, "x?.𐀀", "x?.𐀀;\n")
	expectPrintedASCII(t, "x?.π", "x?.\\u03C0;\n")
	expectPrintedASCII(t, "x?.𐀀", "x?.\\u{10000};\n")
	expectPrintedTargetASCII(t, 5, "x?.π", "x == null ? void 0 : x.\\u03C0;\n")
	expectPrintedTargetASCII(t, 5, "x?.𐀀", "x == null ? void 0 : x[\"\\uD800\\uDC00\"];\n")

	expectPrinted(t, "0 .π", "0 .π;\n")
	expectPrinted(t, "0 .𐀀", "0 .𐀀;\n")
	expectPrintedASCII(t, "0 .π", "0 .\\u03C0;\n")
	expectPrintedASCII(t, "0 .𐀀", "0 .\\u{10000};\n")
	expectPrintedTargetASCII(t, 5, "0 .π", "0 .\\u03C0;\n")
	expectPrintedTargetASCII(t, 5, "0 .𐀀", "0[\"\\uD800\\uDC00\"];\n")

	expectPrinted(t, "0?.π", "0?.π;\n")
	expectPrinted(t, "0?.𐀀", "0?.𐀀;\n")
	expectPrintedASCII(t, "0?.π", "0?.\\u03C0;\n")
	expectPrintedASCII(t, "0?.𐀀", "0?.\\u{10000};\n")
	expectPrintedTargetASCII(t, 5, "0?.π", "0 == null ? void 0 : 0 .\\u03C0;\n")
	expectPrintedTargetASCII(t, 5, "0?.𐀀", "0 == null ? void 0 : 0[\"\\uD800\\uDC00\"];\n")

	expectPrinted(t, "import 'π'", "import \"π\";\n")
	expectPrinted(t, "import '𐀀'", "import \"𐀀\";\n")
	expectPrintedASCII(t, "import 'π'", "import \"\\u03C0\";\n")
	expectPrintedASCII(t, "import '𐀀'", "import \"\\u{10000}\";\n")
	expectPrintedTargetASCII(t, 5, "import 'π'", "import \"\\u03C0\";\n")
	expectPrintedTargetASCII(t, 5, "import '𐀀'", "import \"\\uD800\\uDC00\";\n")

	expectPrinted(t, "({π: 0})", "({π: 0});\n")
	expectPrinted(t, "({𐀀: 0})", "({𐀀: 0});\n")
	expectPrintedASCII(t, "({π: 0})", "({\\u03C0: 0});\n")
	expectPrintedASCII(t, "({𐀀: 0})", "({\\u{10000}: 0});\n")
	expectPrintedTargetASCII(t, 5, "({π: 0})", "({\\u03C0: 0});\n")
	expectPrintedTargetASCII(t, 5, "({𐀀: 0})", "({\"\\uD800\\uDC00\": 0});\n")

	expectPrinted(t, "({π})", "({π});\n")
	expectPrinted(t, "({𐀀})", "({𐀀});\n")
	expectPrintedASCII(t, "({π})", "({\\u03C0});\n")
	expectPrintedASCII(t, "({𐀀})", "({\\u{10000}});\n")
	expectPrintedTargetASCII(t, 5, "({π})", "({\\u03C0: \\u03C0});\n")
	expectParseErrorTargetASCII(t, 5, "({𐀀})", es5)

	expectPrinted(t, "import * as π from 'path'; π", "import * as π from \"path\";\nπ;\n")
	expectPrinted(t, "import * as 𐀀 from 'path'; 𐀀", "import * as 𐀀 from \"path\";\n𐀀;\n")
	expectPrintedASCII(t, "import * as π from 'path'; π", "import * as \\u03C0 from \"path\";\n\\u03C0;\n")
	expectPrintedASCII(t, "import * as 𐀀 from 'path'; 𐀀", "import * as \\u{10000} from \"path\";\n\\u{10000};\n")
	expectPrintedTargetASCII(t, 5, "import * as π from 'path'; π", "import * as \\u03C0 from \"path\";\n\\u03C0;\n")
	expectParseErrorTargetASCII(t, 5, "import * as 𐀀 from 'path'", es5)

	expectPrinted(t, "import {π} from 'path'; π", "import {π} from \"path\";\nπ;\n")
	expectPrinted(t, "import {𐀀} from 'path'; 𐀀", "import {𐀀} from \"path\";\n𐀀;\n")
	expectPrintedASCII(t, "import {π} from 'path'; π", "import {\\u03C0} from \"path\";\n\\u03C0;\n")
	expectPrintedASCII(t, "import {𐀀} from 'path'; 𐀀", "import {\\u{10000}} from \"path\";\n\\u{10000};\n")
	expectPrintedTargetASCII(t, 5, "import {π} from 'path'; π", "import {\\u03C0} from \"path\";\n\\u03C0;\n")
	expectParseErrorTargetASCII(t, 5, "import {𐀀} from 'path'", es5)

	expectPrinted(t, "import {π as x} from 'path'", "import {π as x} from \"path\";\n")
	expectPrinted(t, "import {𐀀 as x} from 'path'", "import {𐀀 as x} from \"path\";\n")
	expectPrintedASCII(t, "import {π as x} from 'path'", "import {\\u03C0 as x} from \"path\";\n")
	expectPrintedASCII(t, "import {𐀀 as x} from 'path'", "import {\\u{10000} as x} from \"path\";\n")
	expectPrintedTargetASCII(t, 5, "import {π as x} from 'path'", "import {\\u03C0 as x} from \"path\";\n")
	expectParseErrorTargetASCII(t, 5, "import {𐀀 as x} from 'path'", es5)

	expectPrinted(t, "import {x as π} from 'path'", "import {x as π} from \"path\";\n")
	expectPrinted(t, "import {x as 𐀀} from 'path'", "import {x as 𐀀} from \"path\";\n")
	expectPrintedASCII(t, "import {x as π} from 'path'", "import {x as \\u03C0} from \"path\";\n")
	expectPrintedASCII(t, "import {x as 𐀀} from 'path'", "import {x as \\u{10000}} from \"path\";\n")
	expectPrintedTargetASCII(t, 5, "import {x as π} from 'path'", "import {x as \\u03C0} from \"path\";\n")
	expectParseErrorTargetASCII(t, 5, "import {x as 𐀀} from 'path'", es5)

	expectPrinted(t, "export * as π from 'path'; π", "export * as π from \"path\";\nπ;\n")
	expectPrinted(t, "export * as 𐀀 from 'path'; 𐀀", "export * as 𐀀 from \"path\";\n𐀀;\n")
	expectPrintedASCII(t, "export * as π from 'path'; π", "export * as \\u03C0 from \"path\";\n\\u03C0;\n")
	expectPrintedASCII(t, "export * as 𐀀 from 'path'; 𐀀", "export * as \\u{10000} from \"path\";\n\\u{10000};\n")
	expectPrintedTargetASCII(t, 5, "export * as π from 'path'", "import * as \\u03C0 from \"path\";\nexport {\\u03C0};\n")
	expectParseErrorTargetASCII(t, 5, "export * as 𐀀 from 'path'", es5)

	expectPrinted(t, "export {π} from 'path'; π", "export {π} from \"path\";\nπ;\n")
	expectPrinted(t, "export {𐀀} from 'path'; 𐀀", "export {𐀀} from \"path\";\n𐀀;\n")
	expectPrintedASCII(t, "export {π} from 'path'; π", "export {\\u03C0} from \"path\";\n\\u03C0;\n")
	expectPrintedASCII(t, "export {𐀀} from 'path'; 𐀀", "export {\\u{10000}} from \"path\";\n\\u{10000};\n")
	expectPrintedTargetASCII(t, 5, "export {π} from 'path'; π", "export {\\u03C0} from \"path\";\n\\u03C0;\n")
	expectParseErrorTargetASCII(t, 5, "export {𐀀} from 'path'", es5)

	expectPrinted(t, "export {π as x} from 'path'", "export {π as x} from \"path\";\n")
	expectPrinted(t, "export {𐀀 as x} from 'path'", "export {𐀀 as x} from \"path\";\n")
	expectPrintedASCII(t, "export {π as x} from 'path'", "export {\\u03C0 as x} from \"path\";\n")
	expectPrintedASCII(t, "export {𐀀 as x} from 'path'", "export {\\u{10000} as x} from \"path\";\n")
	expectPrintedTargetASCII(t, 5, "export {π as x} from 'path'", "export {\\u03C0 as x} from \"path\";\n")
	expectParseErrorTargetASCII(t, 5, "export {𐀀 as x} from 'path'", es5)

	expectPrinted(t, "export {x as π} from 'path'", "export {x as π} from \"path\";\n")
	expectPrinted(t, "export {x as 𐀀} from 'path'", "export {x as 𐀀} from \"path\";\n")
	expectPrintedASCII(t, "export {x as π} from 'path'", "export {x as \\u03C0} from \"path\";\n")
	expectPrintedASCII(t, "export {x as 𐀀} from 'path'", "export {x as \\u{10000}} from \"path\";\n")
	expectPrintedTargetASCII(t, 5, "export {x as π} from 'path'", "export {x as \\u03C0} from \"path\";\n")
	expectParseErrorTargetASCII(t, 5, "export {x as 𐀀} from 'path'", es5)

	expectPrinted(t, "export {π}; var π", "export {π};\nvar π;\n")
	expectPrinted(t, "export {𐀀}; var 𐀀", "export {𐀀};\nvar 𐀀;\n")
	expectPrintedASCII(t, "export {π}; var π", "export {\\u03C0};\nvar \\u03C0;\n")
	expectPrintedASCII(t, "export {𐀀}; var 𐀀", "export {\\u{10000}};\nvar \\u{10000};\n")
	expectPrintedTargetASCII(t, 5, "export {π}; var π", "export {\\u03C0};\nvar \\u03C0;\n")
	expectParseErrorTargetASCII(t, 5, "export {𐀀}; var 𐀀", es5)

	expectPrinted(t, "export var π", "export var π;\n")
	expectPrinted(t, "export var 𐀀", "export var 𐀀;\n")
	expectPrintedASCII(t, "export var π", "export var \\u03C0;\n")
	expectPrintedASCII(t, "export var 𐀀", "export var \\u{10000};\n")
	expectPrintedTargetASCII(t, 5, "export var π", "export var \\u03C0;\n")
	expectParseErrorTargetASCII(t, 5, "export var 𐀀", es5)
}
