package parser

import (
	"fmt"
	"testing"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/lexer"
	"github.com/evanw/esbuild/internal/logging"
	"github.com/evanw/esbuild/internal/printer"
	"github.com/evanw/esbuild/internal/test"
)

func expectParseError(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log := logging.NewDeferLog()
		Parse(log, test.SourceForTest(contents), config.Options{})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
		}
		test.AssertEqual(t, text, expected)
	})
}

func expectParseErrorTarget(t *testing.T, esVersion int, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log := logging.NewDeferLog()
		Parse(log, test.SourceForTest(contents), config.Options{
			UnsupportedFeatures: compat.UnsupportedFeatures(map[compat.Engine][]int{
				compat.ES: {esVersion},
			}),
		})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
		}
		test.AssertEqual(t, text, expected)
	})
}

func expectPrinted(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log := logging.NewDeferLog()
		ast, ok := Parse(log, test.SourceForTest(contents), config.Options{})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			if msg.Kind != logging.Warning {
				text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
			}
		}
		test.AssertEqual(t, text, "")
		if !ok {
			t.Fatal("Parse error")
		}
		js := printer.Print(ast, printer.PrintOptions{}).JS
		test.AssertEqual(t, string(js), expected)
	})
}

func expectPrintedMangle(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log := logging.NewDeferLog()
		ast, ok := Parse(log, test.SourceForTest(contents), config.Options{
			MangleSyntax: true,
		})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
		}
		test.AssertEqual(t, text, "")
		if !ok {
			t.Fatal("Parse error")
		}
		js := printer.Print(ast, printer.PrintOptions{}).JS
		test.AssertEqual(t, string(js), expected)
	})
}

func expectPrintedTarget(t *testing.T, esVersion int, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log := logging.NewDeferLog()
		unsupportedFeatures := compat.UnsupportedFeatures(map[compat.Engine][]int{
			compat.ES: {esVersion},
		})
		ast, ok := Parse(log, test.SourceForTest(contents), config.Options{
			UnsupportedFeatures: unsupportedFeatures,
		})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			if msg.Kind != logging.Warning {
				text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
			}
		}
		test.AssertEqual(t, text, "")
		if !ok {
			t.Fatal("Parse error")
		}
		js := printer.Print(ast, printer.PrintOptions{
			UnsupportedFeatures: unsupportedFeatures,
		}).JS
		test.AssertEqual(t, string(js), expected)
	})
}

func expectPrintedTargetStrict(t *testing.T, esVersion int, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log := logging.NewDeferLog()
		ast, ok := Parse(log, test.SourceForTest(contents), config.Options{
			UnsupportedFeatures: compat.UnsupportedFeatures(map[compat.Engine][]int{
				compat.ES: {esVersion},
			}),
			Strict: config.StrictOptions{
				NullishCoalescing: true,
				ClassFields:       true,
			},
		})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			if msg.Kind != logging.Warning {
				text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
			}
		}
		test.AssertEqual(t, text, "")
		if !ok {
			t.Fatal("Parse error")
		}
		js := printer.Print(ast, printer.PrintOptions{}).JS
		test.AssertEqual(t, string(js), expected)
	})
}

func expectParseErrorJSX(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log := logging.NewDeferLog()
		Parse(log, test.SourceForTest(contents), config.Options{
			JSX: config.JSXOptions{
				Parse: true,
			},
		})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
		}
		test.AssertEqual(t, text, expected)
	})
}

func expectPrintedJSX(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log := logging.NewDeferLog()
		ast, ok := Parse(log, test.SourceForTest(contents), config.Options{
			JSX: config.JSXOptions{
				Parse: true,
			},
		})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
		}
		test.AssertEqual(t, text, "")
		if !ok {
			t.Fatal("Parse error")
		}
		js := printer.Print(ast, printer.PrintOptions{}).JS
		test.AssertEqual(t, string(js), expected)
	})
}

func TestBinOp(t *testing.T) {
	for code, entry := range ast.OpTable {
		if ast.OpCode(code).IsLeftAssociative() {
			op := entry.Text
			expectPrinted(t, "a "+op+" b "+op+" c", "a "+op+" b "+op+" c;\n")
			expectPrinted(t, "(a "+op+" b) "+op+" c", "a "+op+" b "+op+" c;\n")
			expectPrinted(t, "a "+op+" (b "+op+" c)", "a "+op+" (b "+op+" c);\n")
		}

		if ast.OpCode(code).IsRightAssociative() {
			op := entry.Text
			expectPrinted(t, "a "+op+" b "+op+" c", "a "+op+" b "+op+" c;\n")
			expectPrinted(t, "(a "+op+" b) "+op+" c", "(a "+op+" b) "+op+" c;\n")
			expectPrinted(t, "a "+op+" (b "+op+" c)", "a "+op+" b "+op+" c;\n")
		}
	}
}

func TestComments(t *testing.T) {
	expectParseError(t, "throw //\n x", "<stdin>: error: Unexpected newline after \"throw\"\n")
	expectParseError(t, "throw /**/\n x", "<stdin>: error: Unexpected newline after \"throw\"\n")
	expectParseError(t, "throw <!--\n x",
		"<stdin>: warning: Treating \"<!--\" as the start of a legacy HTML single-line comment\n"+
			"<stdin>: error: Unexpected newline after \"throw\"\n")
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
	expectParseError(t, "const x", "<stdin>: error: This constant must be initialized\n")
	expectParseError(t, "for (var x;;) ;", "")
	expectParseError(t, "for (let x;;) ;", "")
	expectParseError(t, "for (const x;;) ;", "<stdin>: error: This constant must be initialized\n")

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

func TestFor(t *testing.T) {
	expectParseError(t, "for (; in x) ;", "<stdin>: error: Unexpected \"in\"\n")
	expectParseError(t, "for (; of x) ;", "<stdin>: error: Expected \";\" but found \"x\"\n")
	expectParseError(t, "for (; in; ) ;", "<stdin>: error: Unexpected \"in\"\n")
	expectPrinted(t, "for (; of; ) ;", "for (; of; )\n  ;\n")

	expectPrinted(t, "for (a in b) ;", "for (a in b)\n  ;\n")
	expectPrinted(t, "for (var a in b) ;", "for (var a in b)\n  ;\n")
	expectPrinted(t, "for (let a in b) ;", "for (let a in b)\n  ;\n")
	expectPrinted(t, "for (const a in b) ;", "for (const a in b)\n  ;\n")
	expectParseError(t, "for (var a, b in b) ;", "<stdin>: error: for-in loops must have a single declaration\n")
	expectParseError(t, "for (let a, b in b) ;", "<stdin>: error: for-in loops must have a single declaration\n")
	expectParseError(t, "for (const a, b in b) ;", "<stdin>: error: for-in loops must have a single declaration\n")

	expectPrinted(t, "for (a of b) ;", "for (a of b)\n  ;\n")
	expectPrinted(t, "for (var a of b) ;", "for (var a of b)\n  ;\n")
	expectPrinted(t, "for (let a of b) ;", "for (let a of b)\n  ;\n")
	expectPrinted(t, "for (const a of b) ;", "for (const a of b)\n  ;\n")
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
}

func TestScope(t *testing.T) {
	expectParseError(t, "var x; var y", "")
	expectParseError(t, "var x; let y", "")
	expectParseError(t, "let x; var y", "")
	expectParseError(t, "let x; let y", "")

	expectParseError(t, "var x; var x", "")
	expectParseError(t, "var x; let x", "<stdin>: error: \"x\" has already been declared\n")
	expectParseError(t, "let x; var x", "<stdin>: error: \"x\" has already been declared\n")
	expectParseError(t, "let x; let x", "<stdin>: error: \"x\" has already been declared\n")

	expectParseError(t, "var x; {var x}", "")
	expectParseError(t, "var x; {let x}", "")
	expectParseError(t, "let x; {var x}", "<stdin>: error: \"x\" has already been declared\n")
	expectParseError(t, "let x; {let x}", "")

	expectParseError(t, "{var x} var x", "")
	expectParseError(t, "{var x} let x", "<stdin>: error: \"x\" has already been declared\n")
	expectParseError(t, "{let x} var x", "")
	expectParseError(t, "{let x} let x", "")

	expectParseError(t, "{var x; {var x}}", "")
	expectParseError(t, "{var x; {let x}}", "")
	expectParseError(t, "{let x; {var x}}", "<stdin>: error: \"x\" has already been declared\n")
	expectParseError(t, "{let x; {let x}}", "")

	expectParseError(t, "{{var x} var x}", "")
	expectParseError(t, "{{var x} let x}", "<stdin>: error: \"x\" has already been declared\n")
	expectParseError(t, "{{let x} var x}", "")
	expectParseError(t, "{{let x} let x}", "")

	expectParseError(t, "{var x} {var x}", "")
	expectParseError(t, "{var x} {let x}", "")
	expectParseError(t, "{let x} {var x}", "")
	expectParseError(t, "{let x} {let x}", "")

	expectParseError(t, "var x=1, x=2", "")
	expectParseError(t, "let x=1, x=2", "<stdin>: error: \"x\" has already been declared\n")
	expectParseError(t, "const x=1, x=2", "<stdin>: error: \"x\" has already been declared\n")

	expectParseError(t, "function foo(x) { var x }", "")
	expectParseError(t, "function foo(x) { let x }", "<stdin>: error: \"x\" has already been declared\n")
	expectParseError(t, "function foo(x) { const x = 0 }", "<stdin>: error: \"x\" has already been declared\n")
	expectParseError(t, "function foo() { var foo }", "")
	expectParseError(t, "function foo() { let foo }", "")
	expectParseError(t, "function foo() { const foo = 0 }", "")

	expectParseError(t, "(function foo(x) { var x })", "")
	expectParseError(t, "(function foo(x) { let x })", "<stdin>: error: \"x\" has already been declared\n")
	expectParseError(t, "(function foo(x) { const x = 0 })", "<stdin>: error: \"x\" has already been declared\n")
	expectParseError(t, "(function foo() { var foo })", "")
	expectParseError(t, "(function foo() { let foo })", "")
	expectParseError(t, "(function foo() { const foo = 0 })", "")

	expectParseError(t, "var x; function x() {}", "")
	expectParseError(t, "let x; function x() {}", "<stdin>: error: \"x\" has already been declared\n")
	expectParseError(t, "function x() {} var x", "")
	expectParseError(t, "function x() {} let x", "<stdin>: error: \"x\" has already been declared\n")
	expectParseError(t, "function x() {} function x() {}", "")

	expectParseError(t, "var x; class x {}", "<stdin>: error: \"x\" has already been declared\n")
	expectParseError(t, "let x; class x {}", "<stdin>: error: \"x\" has already been declared\n")
	expectParseError(t, "class x {} var x", "<stdin>: error: \"x\" has already been declared\n")
	expectParseError(t, "class x {} let x", "<stdin>: error: \"x\" has already been declared\n")
	expectParseError(t, "class x {} class x {}", "<stdin>: error: \"x\" has already been declared\n")
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

	// This is a weird corner case where ASI applies without a newline
	expectPrinted(t, "do x;while(y)z", "do\n  x;\nwhile (y);\nz;\n")
	expectPrinted(t, "do x;while(y);z", "do\n  x;\nwhile (y);\nz;\n")
	expectPrinted(t, "{do x;while(y)}", "{\n  do\n    x;\n  while (y);\n}\n")
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
}

func TestObject(t *testing.T) {
	expectPrinted(t, "({foo:0})", "({foo: 0});\n")
	expectPrinted(t, "({foo() {}})", "({foo() {\n}});\n")
	expectPrinted(t, "({*foo() {}})", "({*foo() {\n}});\n")
	expectPrinted(t, "({get foo() {}})", "({get foo() {\n}});\n")
	expectPrinted(t, "({set foo() {}})", "({set foo() {\n}});\n")

	expectPrinted(t, "({if:0})", "({if: 0});\n")
	expectPrinted(t, "({if() {}})", "({if() {\n}});\n")
	expectPrinted(t, "({*if() {}})", "({*if() {\n}});\n")
	expectPrinted(t, "({get if() {}})", "({get if() {\n}});\n")
	expectPrinted(t, "({set if() {}})", "({set if() {\n}});\n")

	expectParseError(t, "({static foo() {}})", "<stdin>: error: Expected \"}\" but found \"foo\"\n")
	expectParseError(t, "({`a`})", "<stdin>: error: Expected identifier but found \"`a`\"\n")
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
	}

	for _, context := range singleStmtContext {
		expectParseError(t, fmt.Sprintf(context, "const x = 0"), "<stdin>: error: Cannot use a declaration in a single-statement context\n")
		expectParseError(t, fmt.Sprintf(context, "let x"), "<stdin>: error: Cannot use a declaration in a single-statement context\n")
		expectParseError(t, fmt.Sprintf(context, "class X {}"), "<stdin>: error: Cannot use a declaration in a single-statement context\n")
		expectParseError(t, fmt.Sprintf(context, "function* x() {}"), "<stdin>: error: Cannot use a declaration in a single-statement context\n")
		expectParseError(t, fmt.Sprintf(context, "async function x() {}"), "<stdin>: error: Cannot use a declaration in a single-statement context\n")
		expectParseError(t, fmt.Sprintf(context, "async function* x() {}"), "<stdin>: error: Cannot use a declaration in a single-statement context\n")
	}
}

func TestClass(t *testing.T) {
	expectPrinted(t, "class Foo { foo() {} }", "class Foo {\n  foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { *foo() {} }", "class Foo {\n  *foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { get foo() {} }", "class Foo {\n  get foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { set foo() {} }", "class Foo {\n  set foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static foo() {} }", "class Foo {\n  static foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static *foo() {} }", "class Foo {\n  static *foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static get foo() {} }", "class Foo {\n  static get foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static set foo() {} }", "class Foo {\n  static set foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async foo() {} }", "class Foo {\n  async foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async foo() {} }", "class Foo {\n  static async foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async *foo() {} }", "class Foo {\n  static async *foo() {\n  }\n}\n")
	expectParseError(t, "class Foo { async static foo() {} }", "<stdin>: error: Expected \"(\" but found \"foo\"\n")

	expectPrinted(t, "class Foo { if() {} }", "class Foo {\n  if() {\n  }\n}\n")
	expectPrinted(t, "class Foo { *if() {} }", "class Foo {\n  *if() {\n  }\n}\n")
	expectPrinted(t, "class Foo { get if() {} }", "class Foo {\n  get if() {\n  }\n}\n")
	expectPrinted(t, "class Foo { set if() {} }", "class Foo {\n  set if() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static if() {} }", "class Foo {\n  static if() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static *if() {} }", "class Foo {\n  static *if() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static get if() {} }", "class Foo {\n  static get if() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static set if() {} }", "class Foo {\n  static set if() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async if() {} }", "class Foo {\n  async if() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async if() {} }", "class Foo {\n  static async if() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async *if() {} }", "class Foo {\n  static async *if() {\n  }\n}\n")
	expectParseError(t, "class Foo { async static if() {} }", "<stdin>: error: Expected \"(\" but found \"if\"\n")

	expectPrinted(t, "class Foo { a() {} b() {} }", "class Foo {\n  a() {\n  }\n  b() {\n  }\n}\n")
	expectPrinted(t, "class Foo { a() {} get b() {} }", "class Foo {\n  a() {\n  }\n  get b() {\n  }\n}\n")
	expectPrinted(t, "class Foo { a() {} set b() {} }", "class Foo {\n  a() {\n  }\n  set b() {\n  }\n}\n")
	expectPrinted(t, "class Foo { a() {} static b() {} }", "class Foo {\n  a() {\n  }\n  static b() {\n  }\n}\n")
	expectPrinted(t, "class Foo { a() {} static *b() {} }", "class Foo {\n  a() {\n  }\n  static *b() {\n  }\n}\n")
	expectPrinted(t, "class Foo { a() {} static get b() {} }", "class Foo {\n  a() {\n  }\n  static get b() {\n  }\n}\n")
	expectPrinted(t, "class Foo { a() {} static set b() {} }", "class Foo {\n  a() {\n  }\n  static set b() {\n  }\n}\n")
	expectPrinted(t, "class Foo { a() {} async b() {} }", "class Foo {\n  a() {\n  }\n  async b() {\n  }\n}\n")
	expectPrinted(t, "class Foo { a() {} static async b() {} }", "class Foo {\n  a() {\n  }\n  static async b() {\n  }\n}\n")
	expectPrinted(t, "class Foo { a() {} static async *b() {} }", "class Foo {\n  a() {\n  }\n  static async *b() {\n  }\n}\n")
	expectParseError(t, "class Foo { a() {} async static b() {} }", "<stdin>: error: Expected \"(\" but found \"b\"\n")

	expectParseError(t, "class Foo { `a`() {} }", "<stdin>: error: Expected identifier but found \"`a`\"\n")

	// The name "constructor" is sometimes forbidden
	expectPrinted(t, "class Foo { get ['constructor']() {} }", "class Foo {\n  get [\"constructor\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { set ['constructor']() {} }", "class Foo {\n  set [\"constructor\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { *['constructor']() {} }", "class Foo {\n  *[\"constructor\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async ['constructor']() {} }", "class Foo {\n  async [\"constructor\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async *['constructor']() {} }", "class Foo {\n  async *[\"constructor\"]() {\n  }\n}\n")
	expectParseError(t, "class Foo { get constructor() {} }", "<stdin>: error: Class constructor cannot be a getter\n")
	expectParseError(t, "class Foo { get 'constructor'() {} }", "<stdin>: error: Class constructor cannot be a getter\n")
	expectParseError(t, "class Foo { set constructor() {} }", "<stdin>: error: Class constructor cannot be a setter\n")
	expectParseError(t, "class Foo { set 'constructor'() {} }", "<stdin>: error: Class constructor cannot be a setter\n")
	expectParseError(t, "class Foo { *constructor() {} }", "<stdin>: error: Class constructor cannot be a generator\n")
	expectParseError(t, "class Foo { *'constructor'() {} }", "<stdin>: error: Class constructor cannot be a generator\n")
	expectParseError(t, "class Foo { async constructor() {} }", "<stdin>: error: Class constructor cannot be an async function\n")
	expectParseError(t, "class Foo { async 'constructor'() {} }", "<stdin>: error: Class constructor cannot be an async function\n")
	expectParseError(t, "class Foo { async *constructor() {} }", "<stdin>: error: Class constructor cannot be an async function\n")
	expectParseError(t, "class Foo { async *'constructor'() {} }", "<stdin>: error: Class constructor cannot be an async function\n")
	expectPrinted(t, "class Foo { static get constructor() {} }", "class Foo {\n  static get constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static get 'constructor'() {} }", "class Foo {\n  static get constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static set constructor() {} }", "class Foo {\n  static set constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static set 'constructor'() {} }", "class Foo {\n  static set constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static *constructor() {} }", "class Foo {\n  static *constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static *'constructor'() {} }", "class Foo {\n  static *constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async constructor() {} }", "class Foo {\n  static async constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async 'constructor'() {} }", "class Foo {\n  static async constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async *constructor() {} }", "class Foo {\n  static async *constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async *'constructor'() {} }", "class Foo {\n  static async *constructor() {\n  }\n}\n")
	expectPrinted(t, "({ constructor: 1 })", "({constructor: 1});\n")
	expectPrinted(t, "({ get constructor() {} })", "({get constructor() {\n}});\n")
	expectPrinted(t, "({ set constructor() {} })", "({set constructor() {\n}});\n")
	expectPrinted(t, "({ *constructor() {} })", "({*constructor() {\n}});\n")
	expectPrinted(t, "({ async constructor() {} })", "({async constructor() {\n}});\n")
	expectPrinted(t, "({ async* constructor() {} })", "({async *constructor() {\n}});\n")

	// The name "prototype" is sometimes forbidden
	expectPrinted(t, "class Foo { get prototype() {} }", "class Foo {\n  get prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { get 'prototype'() {} }", "class Foo {\n  get prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { set prototype() {} }", "class Foo {\n  set prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { set 'prototype'() {} }", "class Foo {\n  set prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { *prototype() {} }", "class Foo {\n  *prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { *'prototype'() {} }", "class Foo {\n  *prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async prototype() {} }", "class Foo {\n  async prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async 'prototype'() {} }", "class Foo {\n  async prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async *prototype() {} }", "class Foo {\n  async *prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async *'prototype'() {} }", "class Foo {\n  async *prototype() {\n  }\n}\n")
	expectParseError(t, "class Foo { static get prototype() {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static get 'prototype'() {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static set prototype() {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static set 'prototype'() {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static *prototype() {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static *'prototype'() {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static async prototype() {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static async 'prototype'() {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static async *prototype() {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static async *'prototype'() {} }", "<stdin>: error: Invalid static method name \"prototype\"\n")
	expectPrinted(t, "class Foo { static get ['prototype']() {} }", "class Foo {\n  static get [\"prototype\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static set ['prototype']() {} }", "class Foo {\n  static set [\"prototype\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static *['prototype']() {} }", "class Foo {\n  static *[\"prototype\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async ['prototype']() {} }", "class Foo {\n  static async [\"prototype\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async *['prototype']() {} }", "class Foo {\n  static async *[\"prototype\"]() {\n  }\n}\n")
	expectPrinted(t, "({ prototype: 1 })", "({prototype: 1});\n")
	expectPrinted(t, "({ get prototype() {} })", "({get prototype() {\n}});\n")
	expectPrinted(t, "({ set prototype() {} })", "({set prototype() {\n}});\n")
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

	expectParseError(t, "for await(;;);", "<stdin>: error: Cannot use \"await\" outside an async function\n")
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
	expectParseError(t, "({a: b, set c() {}}) => {}", "<stdin>: error: Invalid binding pattern\n")

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
}

func TestTemplate(t *testing.T) {
	expectPrinted(t, "`a${1 + `b${2}c` + 3}d`", "`a${1 + `b${2}c` + 3}d`;\n")
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

	expectParseError(t, "x === -0", "<stdin>: warning: Comparison with -0 using the === operator will also match 0\n")
	expectParseError(t, "x == -0", "<stdin>: warning: Comparison with -0 using the == operator will also match 0\n")
	expectParseError(t, "x !== -0", "<stdin>: warning: Comparison with -0 using the !== operator will also match 0\n")
	expectParseError(t, "x != -0", "<stdin>: warning: Comparison with -0 using the != operator will also match 0\n")
	expectParseError(t, "-0 === x", "<stdin>: warning: Comparison with -0 using the === operator will also match 0\n")
	expectParseError(t, "-0 == x", "<stdin>: warning: Comparison with -0 using the == operator will also match 0\n")
	expectParseError(t, "-0 !== x", "<stdin>: warning: Comparison with -0 using the !== operator will also match 0\n")
	expectParseError(t, "-0 != x", "<stdin>: warning: Comparison with -0 using the != operator will also match 0\n")

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
	expectParseError(t, "import()", "<stdin>: error: Unexpected \")\"\n")
	expectParseError(t, "import(...a)", "<stdin>: error: Unexpected \"...\"\n")
	expectParseError(t, "import(a, b)", "<stdin>: error: Expected \")\" but found \",\"\n")

	expectPrinted(t, "import.meta", "import.meta;\n")
	expectPrinted(t, "(import.meta)", "import.meta;\n")
	expectPrinted(t, "{import.meta}", "{\n  import.meta;\n}\n")

	expectParseError(t, "import x from \"foo\"; x = 1", "<stdin>: error: Cannot assign to import \"x\"\n")
	expectParseError(t, "import x from \"foo\"; x++", "<stdin>: error: Cannot assign to import \"x\"\n")
	expectParseError(t, "import x from \"foo\"; ([x] = 1)", "<stdin>: error: Cannot assign to import \"x\"\n")
	expectParseError(t, "import x from \"foo\"; ({x} = 1)", "<stdin>: error: Cannot assign to import \"x\"\n")
	expectParseError(t, "import x from \"foo\"; ({y: x} = 1)", "<stdin>: error: Cannot assign to import \"x\"\n")
	expectParseError(t, "import {x} from \"foo\"; x++", "<stdin>: error: Cannot assign to import \"x\"\n")
	expectParseError(t, "import * as x from \"foo\"; x++", "<stdin>: error: Cannot assign to import \"x\"\n")
	expectParseError(t, "import * as x from \"foo\"; x.y = 1", "<stdin>: error: Cannot assign to import \"y\"\n")
	expectParseError(t, "import * as x from \"foo\"; x[y] = 1", "<stdin>: error: Cannot assign to property on import \"x\"\n")
	expectParseError(t, "import * as x from \"foo\"; x['y'] = 1", "<stdin>: error: Cannot assign to import \"y\"\n")
	expectPrinted(t, "import x from \"foo\"; ({y = x} = 1)", "import x from \"foo\";\n({y = x} = 1);\n")
	expectPrinted(t, "import x from \"foo\"; ({[x]: y} = 1)", "import x from \"foo\";\n({[x]: y} = 1);\n")
	expectPrinted(t, "import x from \"foo\"; x.y = 1", "import x from \"foo\";\nx.y = 1;\n")
	expectPrinted(t, "import x from \"foo\"; x[y] = 1", "import x from \"foo\";\nx[y] = 1;\n")
	expectPrinted(t, "import x from \"foo\"; x['y'] = 1", "import x from \"foo\";\nx[\"y\"] = 1;\n")
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
	expectPrinted(t, "export {x}", "export {x};\n")
	expectPrinted(t, "export {x as y}", "export {x as y};\n")
	expectPrinted(t, "export {x as y, z}", "export {x as y, z};\n")
	expectPrinted(t, "export {x as y, z,}", "export {x as y, z};\n")
	expectPrinted(t, "export {x} from \"foo\"", "export {x} from \"foo\";\n")
	expectPrinted(t, "export {x as y} from \"foo\"", "export {x as y} from \"foo\";\n")
	expectPrinted(t, "export {x as y, z} from \"foo\"", "export {x as y, z} from \"foo\";\n")
	expectPrinted(t, "export {x as y, z,} from \"foo\"", "export {x as y, z} from \"foo\";\n")

	expectParseError(t, "export x from \"foo\"", "<stdin>: error: Unexpected \"x\"\n")
	expectParseError(t, "export async", "<stdin>: error: Expected \"function\" but found end of file\n")
	expectParseError(t, "export async function", "<stdin>: error: Expected identifier but found end of file\n")
	expectParseError(t, "export async () => {}", "<stdin>: error: Expected \"function\" but found \"(\"\n")
}

func TestExportDuplicates(t *testing.T) {
	expectPrinted(t, "export {x}", "export {x};\n")
	expectPrinted(t, "export {x, x as y}", "export {x, x as y};\n")
	expectPrinted(t, "export {x};export {x as y} from 'foo'", "export {x};\nexport {x as y} from \"foo\";\n")
	expectPrinted(t, "export {x};export default function x() {}", "export {x};\nexport default function x() {\n}\n")
	expectPrinted(t, "export {x};export default class x {}", "export {x};\nexport default class x {\n}\n")

	expectParseError(t, "export {x, x}", "<stdin>: error: Multiple exports with the same name \"x\"\n")
	expectParseError(t, "export {x, y as x}", "<stdin>: error: Multiple exports with the same name \"x\"\n")
	expectParseError(t, "export {x};export function x() {}", "<stdin>: error: Multiple exports with the same name \"x\"\n")
	expectParseError(t, "export {x};export class x {}", "<stdin>: error: Multiple exports with the same name \"x\"\n")
	expectParseError(t, "export {x};export const x = 0", "<stdin>: error: Multiple exports with the same name \"x\"\n")
	expectParseError(t, "export {x};export let x", "<stdin>: error: Multiple exports with the same name \"x\"\n")
	expectParseError(t, "export {x};export var x", "<stdin>: error: Multiple exports with the same name \"x\"\n")
	expectParseError(t, "export {x};export {x} from 'foo'", "<stdin>: error: Multiple exports with the same name \"x\"\n")
	expectParseError(t, "export {x};export {y as x} from 'foo'", "<stdin>: error: Multiple exports with the same name \"x\"\n")
	expectParseError(t, "export {x};export * as x from 'foo'", "<stdin>: error: Multiple exports with the same name \"x\"\n")
	expectParseError(t, "export {x as default};export default 0", "<stdin>: error: Multiple exports with the same name \"default\"\n")
	expectParseError(t, "export {x as default};export default function() {}", "<stdin>: error: Multiple exports with the same name \"default\"\n")
	expectParseError(t, "export {x as default};export default class {}", "<stdin>: error: Multiple exports with the same name \"default\"\n")
	expectParseError(t, "export {x as default};export default function x() {}", "<stdin>: error: Multiple exports with the same name \"default\"\n")
	expectParseError(t, "export {x as default};export default class x {}", "<stdin>: error: Multiple exports with the same name \"default\"\n")
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
	expectPrinted(t, "export {x, y}", "export {x, y};\n")
	expectPrinted(t, "export {x, y as z,}", "export {x, y as z};\n")
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
	expectPrinted(t, "try {} catch (e) { function e() {} }", "try {\n} catch (e) {\n  function e() {\n  }\n}\n")
	expectPrinted(t, "var e; try {} catch (e) {}", "var e;\ntry {\n} catch (e) {\n}\n")
	expectPrinted(t, "let e; try {} catch (e) {}", "let e;\ntry {\n} catch (e) {\n}\n")
	expectPrinted(t, "try { var e } catch (e) {}", "try {\n  var e;\n} catch (e) {\n}\n")
	expectPrinted(t, "try { function e() {} } catch (e) {}", "try {\n  function e() {\n  }\n} catch (e) {\n}\n")

	expectParseError(t, "try {} catch ({e}) { var e }", "<stdin>: error: \"e\" has already been declared\n")
	expectParseError(t, "try {} catch ({e}) { function e() {} }", "<stdin>: error: \"e\" has already been declared\n")
	expectParseError(t, "try {} catch (e) { let e }", "<stdin>: error: \"e\" has already been declared\n")
	expectParseError(t, "try {} catch (e) { const e = 0 }", "<stdin>: error: \"e\" has already been declared\n")
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

func TestMangleIf(t *testing.T) {
	expectPrintedMangle(t, "1 ? a() : b()", "a();\n")
	expectPrintedMangle(t, "0 ? a() : b()", "b();\n")
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
}

func TestMangleReturn(t *testing.T) {
	expectPrintedMangle(t, "function foo() { a = b; if (a) return; if (b) c = b; return c; }",
		"function foo() {\n  return a = b, a ? void 0 : (b && (c = b), c);\n}\n")
	expectPrintedMangle(t, "function foo() { if (!a) return b; return c; }", "function foo() {\n  return a ? c : b;\n}\n")

	expectPrintedMangle(t, "if (1) return a(); else return b()", "return a();\n")
	expectPrintedMangle(t, "if (0) return a(); else return b()", "return b();\n")
	expectPrintedMangle(t, "if (a) return b(); else return c()", "return a ? b() : c();\n")
	expectPrintedMangle(t, "if (!a) return b(); else return c()", "return a ? c() : b();\n")
	expectPrintedMangle(t, "if (!!a) return b(); else return c()", "return a ? b() : c();\n")
	expectPrintedMangle(t, "if (!!!a) return b(); else return c()", "return a ? c() : b();\n")
}

func TestMangleThrow(t *testing.T) {
	expectPrintedMangle(t, "function foo() { a = b; if (a) throw a; if (b) c = b; throw c; }",
		"function foo() {\n  throw a = b, a ? a : (b && (c = b), c);\n}\n")
	expectPrintedMangle(t, "function foo() { if (!a) throw b; throw c; }", "function foo() {\n  throw a ? c : b;\n}\n")

	expectPrintedMangle(t, "if (1) throw a(); else throw b()", "throw a();\n")
	expectPrintedMangle(t, "if (0) throw a(); else throw b()", "throw b();\n")
	expectPrintedMangle(t, "if (a) throw b(); else throw c()", "throw a ? b() : c();\n")
	expectPrintedMangle(t, "if (!a) throw b(); else throw c()", "throw a ? c() : b();\n")
	expectPrintedMangle(t, "if (!!a) throw b(); else throw c()", "throw a ? b() : c();\n")
	expectPrintedMangle(t, "if (!!!a) throw b(); else throw c()", "throw a ? c() : b();\n")
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

func TestTrimCodeInDeadControlFlow(t *testing.T) {
	expectPrintedMangle(t, "if (1) a(); else { ; }", "a();\n")
	expectPrintedMangle(t, "if (1) a(); else { b() }", "a();\n")
	expectPrintedMangle(t, "if (1) a(); else { const b = c }", "a();\n")
	expectPrintedMangle(t, "if (1) a(); else { let b }", "a();\n")
	expectPrintedMangle(t, "if (1) a(); else { throw b }", "a();\n")
	expectPrintedMangle(t, "if (1) a(); else { return b }", "a();\n")
	expectPrintedMangle(t, "b: { if (1) a(); else { break b } }", "b:\n  a();\n")
	expectPrintedMangle(t, "b: { if (1) a(); else { continue b } }", "b:\n  a();\n")
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

	// Test "lexer.Next()"
	expectParseError(t, "var\u0008x", "<stdin>: error: Expected identifier but found \"\\b\"\n")
	for _, s := range whitespace {
		expectPrinted(t, "var"+s+"x", "var x;\n")
	}

	// Test "lexer.NextInsideJSXElement()"
	expectParseErrorJSX(t, "<x\u0008y/>", "<stdin>: error: Expected \">\" but found \"\\b\"\n")
	for _, s := range whitespace {
		expectPrintedJSX(t, "<x"+s+"y/>", "/* @__PURE__ */ React.createElement(\"x\", {\n  y: true\n});\n")
	}

	// Test "lexer.NextJSXElementChild()"
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

	// Test "lexer.Next()"
	for _, s := range invalidWhitespaceInJS {
		r, _ := lexer.DecodeWTF8Rune(s)
		expectParseError(t, "var"+s+"x", fmt.Sprintf("<stdin>: error: Expected identifier but found \"\\u%04x\"\n", r))
	}

	// Test "lexer.NextInsideJSXElement()"
	for _, s := range invalidWhitespaceInJS {
		r, _ := lexer.DecodeWTF8Rune(s)
		expectParseErrorJSX(t, "<x"+s+"y/>", fmt.Sprintf("<stdin>: error: Expected \">\" but found \"\\u%04x\"\n", r))
	}

	// Test "lexer.NextJSXElementChild()"
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
	expectParseErrorJSX(t, "<a></b>", "<stdin>: error: Expected closing tag \"b\" to match opening tag \"a\"\n")
	expectParseErrorJSX(t, "<\na\n.\nb\n>\n<\n/\nc\n.\nd\n>", "<stdin>: error: Expected closing tag \"c.d\" to match opening tag \"a.b\"\n")
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
}

func TestLowerFunctionArgumentScope(t *testing.T) {
	templates := []string{
		"(x = %s) => {\n};\n",
		"(function(x = %s) {\n});\n",
		"function foo(x = %s) {\n}\n",

		"({[%s]: x}) => {\n};\n",
		"(function({[%s]: x}) {\n});\n",
		"function foo({[%s]: x}) {\n}\n",

		"({x = %s}) => {\n};\n",
		"(function({x = %s}) {\n});\n",
		"function foo({x = %s}) {\n}\n",
	}

	for _, template := range templates {
		test := func(before string, after string) {
			expectPrintedTarget(t, 2015, fmt.Sprintf(template, before), fmt.Sprintf(template, after))
		}

		test("a() ?? b", "((_a) => (_a = a()) != null ? _a : b)()")
		test("a()?.b", "((_a) => (_a = a()) == null ? void 0 : _a.b)()")
		test("a?.b?.()", "((_a) => (_a = a == null ? void 0 : a.b) == null ? void 0 : _a.call(a))()")
		test("a.b.c?.()", "((_a) => ((_b) => (_b = (_a = a.b).c) == null ? void 0 : _b.call(_a))())()")
		test("class { static a }", "((_a) => (_a = class {\n}, _a.a = void 0, _a))()")
	}
}

func TestLowerNullishCoalescing(t *testing.T) {
	expectPrintedTarget(t, 2020, "a ?? b", "a ?? b;\n")
	expectPrintedTargetStrict(t, 2020, "a ?? b", "a ?? b;\n")

	expectPrintedTarget(t, 2019, "a ?? b", "a != null ? a : b;\n")
	expectPrintedTarget(t, 2019, "a() ?? b()", "var _a;\n(_a = a()) != null ? _a : b();\n")
	expectPrintedTarget(t, 2019, "function foo() { if (x) { a() ?? b() ?? c() } }",
		"function foo() {\n  var _a, _b;\n  if (x) {\n    (_b = (_a = a()) != null ? _a : b()) != null ? _b : c();\n  }\n}\n")
	expectPrintedTarget(t, 2019, "() => a ?? b", "() => a != null ? a : b;\n")
	expectPrintedTarget(t, 2019, "() => a() ?? b()", "() => {\n  var _a;\n  return (_a = a()) != null ? _a : b();\n};\n")

	expectPrintedTargetStrict(t, 2019, "a ?? b", "a !== null && a !== void 0 ? a : b;\n")
	expectPrintedTargetStrict(t, 2019, "a() ?? b()", "var _a;\n(_a = a()) !== null && _a !== void 0 ? _a : b();\n")
}

func TestLowerNullishCoalescingAssign(t *testing.T) {
	expectPrinted(t, "a ??= b", "a ??= b;\n")

	expectPrintedTarget(t, 2019, "a ??= b", "a != null ? a : a = b;\n")
	expectPrintedTarget(t, 2019, "a.b ??= c", "var _a;\n(_a = a.b) != null ? _a : a.b = c;\n")
	expectPrintedTarget(t, 2019, "a().b ??= c", "var _a, _b;\n(_b = (_a = a()).b) != null ? _b : _a.b = c;\n")
	expectPrintedTarget(t, 2019, "a[b] ??= c", "var _a;\n(_a = a[b]) != null ? _a : a[b] = c;\n")
	expectPrintedTarget(t, 2019, "a()[b()] ??= c", "var _a, _b, _c;\n(_c = (_a = a())[_b = b()]) != null ? _c : _a[_b] = c;\n")

	expectPrintedTargetStrict(t, 2019, "a ??= b", "a !== null && a !== void 0 ? a : a = b;\n")
	expectPrintedTargetStrict(t, 2019, "a.b ??= c", "var _a;\n(_a = a.b) !== null && _a !== void 0 ? _a : a.b = c;\n")

	expectPrintedTarget(t, 2020, "a ??= b", "a ?? (a = b);\n")
	expectPrintedTarget(t, 2020, "a.b ??= c", "a.b ?? (a.b = c);\n")
	expectPrintedTarget(t, 2020, "a().b ??= c", "var _a;\n(_a = a()).b ?? (_a.b = c);\n")
	expectPrintedTarget(t, 2020, "a[b] ??= c", "a[b] ?? (a[b] = c);\n")
	expectPrintedTarget(t, 2020, "a()[b()] ??= c", "var _a, _b;\n(_a = a())[_b = b()] ?? (_a[_b] = c);\n")

	expectPrintedTargetStrict(t, 2020, "a ??= b", "a ?? (a = b);\n")
	expectPrintedTargetStrict(t, 2020, "a.b ??= c", "a.b ?? (a.b = c);\n")
}

func TestLowerLogicalAssign(t *testing.T) {
	expectPrinted(t, "a &&= b", "a &&= b;\n")
	expectPrinted(t, "a ||= b", "a ||= b;\n")

	expectPrintedTarget(t, 2020, "a &&= b", "a && (a = b);\n")
	expectPrintedTarget(t, 2020, "a.b &&= c", "a.b && (a.b = c);\n")
	expectPrintedTarget(t, 2020, "a().b &&= c", "var _a;\n(_a = a()).b && (_a.b = c);\n")
	expectPrintedTarget(t, 2020, "a[b] &&= c", "a[b] && (a[b] = c);\n")
	expectPrintedTarget(t, 2020, "a()[b()] &&= c", "var _a, _b;\n(_a = a())[_b = b()] && (_a[_b] = c);\n")

	expectPrintedTarget(t, 2020, "a ||= b", "a || (a = b);\n")
	expectPrintedTarget(t, 2020, "a.b ||= c", "a.b || (a.b = c);\n")
	expectPrintedTarget(t, 2020, "a().b ||= c", "var _a;\n(_a = a()).b || (_a.b = c);\n")
	expectPrintedTarget(t, 2020, "a[b] ||= c", "a[b] || (a[b] = c);\n")
	expectPrintedTarget(t, 2020, "a()[b()] ||= c", "var _a, _b;\n(_a = a())[_b = b()] || (_a[_b] = c);\n")
}

func TestLowerClassSideEffectOrder(t *testing.T) {
	// The order of computed property side effects must not change
	expectPrintedTarget(t, 2015, `class Foo {
	[a()]() {}
	[b()];
	[c()] = 1;
	[d()]() {}
	static [e()];
	static [f()] = 1;
	static [g()]() {}
	[h()];
}
`, `var _a, _b, _c, _d, _e;
class Foo {
  constructor() {
    this[_a] = void 0;
    this[_b] = 1;
    this[_e] = void 0;
  }
  [a()]() {
  }
  [(_a = b(), _b = c(), d())]() {
  }
  static [(_c = e(), _d = f(), g())]() {
  }
}
_e = h();
Foo[_c] = void 0;
Foo[_d] = 1;
`)
}

func TestLowerClassInstance(t *testing.T) {
	expectPrintedTarget(t, 2015, "class Foo {}", "class Foo {\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { foo }", "class Foo {\n  constructor() {\n    this.foo = void 0;\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { foo = null }", "class Foo {\n  constructor() {\n    this.foo = null;\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { 123 }", "class Foo {\n  constructor() {\n    this[123] = void 0;\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { 123 = null }", "class Foo {\n  constructor() {\n    this[123] = null;\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { [foo] }", "var _a;\nclass Foo {\n  constructor() {\n    this[_a] = void 0;\n  }\n}\n_a = foo;\n")
	expectPrintedTarget(t, 2015, "class Foo { [foo] = null }", "var _a;\nclass Foo {\n  constructor() {\n    this[_a] = null;\n  }\n}\n_a = foo;\n")

	expectPrintedTarget(t, 2015, "(class {})", "(class {\n});\n")
	expectPrintedTarget(t, 2015, "(class { foo })", "(class {\n  constructor() {\n    this.foo = void 0;\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { foo = null })", "(class {\n  constructor() {\n    this.foo = null;\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { 123 })", "(class {\n  constructor() {\n    this[123] = void 0;\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { 123 = null })", "(class {\n  constructor() {\n    this[123] = null;\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { [foo] })", "var _a, _b;\n_b = class {\n  constructor() {\n    this[_a] = void 0;\n  }\n}, _a = foo, _b;\n")
	expectPrintedTarget(t, 2015, "(class { [foo] = null })", "var _a, _b;\n_b = class {\n  constructor() {\n    this[_a] = null;\n  }\n}, _a = foo, _b;\n")

	expectPrintedTarget(t, 2015, "class Foo extends Bar {}", `class Foo extends Bar {
}
`)
	expectPrintedTarget(t, 2015, "class Foo extends Bar { bar() {} constructor() { super() } }", `class Foo extends Bar {
  bar() {
  }
  constructor() {
    super();
  }
}
`)
	expectPrintedTarget(t, 2015, "class Foo extends Bar { bar() {} foo }", `class Foo extends Bar {
  constructor() {
    super(...arguments);
    this.foo = void 0;
  }
  bar() {
  }
}
`)
	expectPrintedTarget(t, 2015, "class Foo extends Bar { bar() {} foo; constructor() { super() } }", `class Foo extends Bar {
  constructor() {
    super();
    this.foo = void 0;
  }
  bar() {
  }
}
`)
}

func TestLowerClassStatic(t *testing.T) {
	expectPrintedTarget(t, 2015, "class Foo { static foo }", "class Foo {\n}\nFoo.foo = void 0;\n")
	expectPrintedTarget(t, 2015, "class Foo { static foo = null }", "class Foo {\n}\nFoo.foo = null;\n")
	expectPrintedTarget(t, 2015, "class Foo { static foo(a, b) {} }", "class Foo {\n  static foo(a, b) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { static get foo() {} }", "class Foo {\n  static get foo() {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { static set foo(a) {} }", "class Foo {\n  static set foo(a) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { static 123 }", "class Foo {\n}\nFoo[123] = void 0;\n")
	expectPrintedTarget(t, 2015, "class Foo { static 123 = null }", "class Foo {\n}\nFoo[123] = null;\n")
	expectPrintedTarget(t, 2015, "class Foo { static 123(a, b) {} }", "class Foo {\n  static 123(a, b) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { static get 123() {} }", "class Foo {\n  static get 123() {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { static set 123(a) {} }", "class Foo {\n  static set 123(a) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { static [foo] }", "var _a;\nclass Foo {\n}\n_a = foo;\nFoo[_a] = void 0;\n")
	expectPrintedTarget(t, 2015, "class Foo { static [foo] = null }", "var _a;\nclass Foo {\n}\n_a = foo;\nFoo[_a] = null;\n")
	expectPrintedTarget(t, 2015, "class Foo { static [foo](a, b) {} }", "class Foo {\n  static [foo](a, b) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { static get [foo]() {} }", "class Foo {\n  static get [foo]() {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "class Foo { static set [foo](a) {} }", "class Foo {\n  static set [foo](a) {\n  }\n}\n")

	expectPrintedTarget(t, 2015, "export default class Foo { static foo }", "export default class Foo {\n}\nFoo.foo = void 0;\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static foo = null }", "export default class Foo {\n}\nFoo.foo = null;\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static foo(a, b) {} }", "export default class Foo {\n  static foo(a, b) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static get foo() {} }", "export default class Foo {\n  static get foo() {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static set foo(a) {} }", "export default class Foo {\n  static set foo(a) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static 123 }", "export default class Foo {\n}\nFoo[123] = void 0;\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static 123 = null }", "export default class Foo {\n}\nFoo[123] = null;\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static 123(a, b) {} }", "export default class Foo {\n  static 123(a, b) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static get 123() {} }", "export default class Foo {\n  static get 123() {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static set 123(a) {} }", "export default class Foo {\n  static set 123(a) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static [foo] }", "var _a;\nexport default class Foo {\n}\n_a = foo;\nFoo[_a] = void 0;\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static [foo] = null }", "var _a;\nexport default class Foo {\n}\n_a = foo;\nFoo[_a] = null;\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static [foo](a, b) {} }", "export default class Foo {\n  static [foo](a, b) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static get [foo]() {} }", "export default class Foo {\n  static get [foo]() {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class Foo { static set [foo](a) {} }", "export default class Foo {\n  static set [foo](a) {\n  }\n}\n")

	expectPrintedTarget(t, 2015, "export default class { static foo }",
		"export default class stdin_default {\n}\nstdin_default.foo = void 0;\n")
	expectPrintedTarget(t, 2015, "export default class { static foo = null }",
		"export default class stdin_default {\n}\nstdin_default.foo = null;\n")
	expectPrintedTarget(t, 2015, "export default class { static foo(a, b) {} }", "export default class {\n  static foo(a, b) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class { static get foo() {} }", "export default class {\n  static get foo() {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class { static set foo(a) {} }", "export default class {\n  static set foo(a) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class { static 123 }",
		"export default class stdin_default {\n}\nstdin_default[123] = void 0;\n")
	expectPrintedTarget(t, 2015, "export default class { static 123 = null }",
		"export default class stdin_default {\n}\nstdin_default[123] = null;\n")
	expectPrintedTarget(t, 2015, "export default class { static 123(a, b) {} }", "export default class {\n  static 123(a, b) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class { static get 123() {} }", "export default class {\n  static get 123() {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class { static set 123(a) {} }", "export default class {\n  static set 123(a) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class { static [foo] }",
		"var _a;\nexport default class stdin_default {\n}\n_a = foo;\nstdin_default[_a] = void 0;\n")
	expectPrintedTarget(t, 2015, "export default class { static [foo] = null }",
		"var _a;\nexport default class stdin_default {\n}\n_a = foo;\nstdin_default[_a] = null;\n")
	expectPrintedTarget(t, 2015, "export default class { static [foo](a, b) {} }", "export default class {\n  static [foo](a, b) {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class { static get [foo]() {} }", "export default class {\n  static get [foo]() {\n  }\n}\n")
	expectPrintedTarget(t, 2015, "export default class { static set [foo](a) {} }", "export default class {\n  static set [foo](a) {\n  }\n}\n")

	expectPrintedTarget(t, 2015, "(class Foo { static foo })", "var _a;\n_a = class {\n}, _a.foo = void 0, _a;\n")
	expectPrintedTarget(t, 2015, "(class Foo { static foo = null })", "var _a;\n_a = class {\n}, _a.foo = null, _a;\n")
	expectPrintedTarget(t, 2015, "(class Foo { static foo(a, b) {} })", "(class Foo {\n  static foo(a, b) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class Foo { static get foo() {} })", "(class Foo {\n  static get foo() {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class Foo { static set foo(a) {} })", "(class Foo {\n  static set foo(a) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class Foo { static 123 })", "var _a;\n_a = class {\n}, _a[123] = void 0, _a;\n")
	expectPrintedTarget(t, 2015, "(class Foo { static 123 = null })", "var _a;\n_a = class {\n}, _a[123] = null, _a;\n")
	expectPrintedTarget(t, 2015, "(class Foo { static 123(a, b) {} })", "(class Foo {\n  static 123(a, b) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class Foo { static get 123() {} })", "(class Foo {\n  static get 123() {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class Foo { static set 123(a) {} })", "(class Foo {\n  static set 123(a) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class Foo { static [foo] })", "var _a, _b;\n_b = class {\n}, _a = foo, _b[_a] = void 0, _b;\n")
	expectPrintedTarget(t, 2015, "(class Foo { static [foo] = null })", "var _a, _b;\n_b = class {\n}, _a = foo, _b[_a] = null, _b;\n")
	expectPrintedTarget(t, 2015, "(class Foo { static [foo](a, b) {} })", "(class Foo {\n  static [foo](a, b) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class Foo { static get [foo]() {} })", "(class Foo {\n  static get [foo]() {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class Foo { static set [foo](a) {} })", "(class Foo {\n  static set [foo](a) {\n  }\n});\n")

	expectPrintedTarget(t, 2015, "(class { static foo })", "var _a;\n_a = class {\n}, _a.foo = void 0, _a;\n")
	expectPrintedTarget(t, 2015, "(class { static foo = null })", "var _a;\n_a = class {\n}, _a.foo = null, _a;\n")
	expectPrintedTarget(t, 2015, "(class { static foo(a, b) {} })", "(class {\n  static foo(a, b) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { static get foo() {} })", "(class {\n  static get foo() {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { static set foo(a) {} })", "(class {\n  static set foo(a) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { static 123 })", "var _a;\n_a = class {\n}, _a[123] = void 0, _a;\n")
	expectPrintedTarget(t, 2015, "(class { static 123 = null })", "var _a;\n_a = class {\n}, _a[123] = null, _a;\n")
	expectPrintedTarget(t, 2015, "(class { static 123(a, b) {} })", "(class {\n  static 123(a, b) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { static get 123() {} })", "(class {\n  static get 123() {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { static set 123(a) {} })", "(class {\n  static set 123(a) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { static [foo] })", "var _a, _b;\n_b = class {\n}, _a = foo, _b[_a] = void 0, _b;\n")
	expectPrintedTarget(t, 2015, "(class { static [foo] = null })", "var _a, _b;\n_b = class {\n}, _a = foo, _b[_a] = null, _b;\n")
	expectPrintedTarget(t, 2015, "(class { static [foo](a, b) {} })", "(class {\n  static [foo](a, b) {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { static get [foo]() {} })", "(class {\n  static get [foo]() {\n  }\n});\n")
	expectPrintedTarget(t, 2015, "(class { static set [foo](a) {} })", "(class {\n  static set [foo](a) {\n  }\n});\n")

	expectPrintedTarget(t, 2015, "(class {})", "(class {\n});\n")
	expectPrintedTarget(t, 2015, "class Foo {}", "class Foo {\n}\n")
	expectPrintedTarget(t, 2015, "(class Foo {})", "(class Foo {\n});\n")

	// Static field with initializers that access the class expression name must
	// still work when they are pulled outside of the class body
	expectPrintedTarget(t, 2015, `
		let Bar = class Foo {
			static foo = 123
			static bar = Foo.foo
		}
	`, `var _a;
let Bar = (_a = class {
}, _a.foo = 123, _a.bar = _a.foo, _a);
`)
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

func TestLowerOptionalChain(t *testing.T) {
	expectPrintedTarget(t, 2019, "a?.b.c", "a == null ? void 0 : a.b.c;\n")
	expectPrintedTarget(t, 2019, "(a?.b).c", "(a == null ? void 0 : a.b).c;\n")
	expectPrintedTarget(t, 2019, "a.b?.c", "var _a;\n(_a = a.b) == null ? void 0 : _a.c;\n")
	expectPrintedTarget(t, 2019, "this?.x", "this == null ? void 0 : this.x;\n")

	expectPrintedTarget(t, 2019, "a?.[b][c]", "a == null ? void 0 : a[b][c];\n")
	expectPrintedTarget(t, 2019, "(a?.[b])[c]", "(a == null ? void 0 : a[b])[c];\n")
	expectPrintedTarget(t, 2019, "a[b]?.[c]", "var _a;\n(_a = a[b]) == null ? void 0 : _a[c];\n")
	expectPrintedTarget(t, 2019, "this?.[x]", "this == null ? void 0 : this[x];\n")

	expectPrintedTarget(t, 2019, "a?.(b)(c)", "a == null ? void 0 : a(b)(c);\n")
	expectPrintedTarget(t, 2019, "(a?.(b))(c)", "(a == null ? void 0 : a(b))(c);\n")
	expectPrintedTarget(t, 2019, "a(b)?.(c)", "var _a;\n(_a = a(b)) == null ? void 0 : _a(c);\n")
	expectPrintedTarget(t, 2019, "this?.(x)", "this == null ? void 0 : this(x);\n")

	expectPrintedTarget(t, 2019, "delete a?.b.c", "a == null ? true : delete a.b.c;\n")
	expectPrintedTarget(t, 2019, "delete a?.[b][c]", "a == null ? true : delete a[b][c];\n")
	expectPrintedTarget(t, 2019, "delete a?.(b)(c)", "a == null ? true : delete a(b)(c);\n")

	expectPrintedTarget(t, 2019, "delete (a?.b).c", "delete (a == null ? void 0 : a.b).c;\n")
	expectPrintedTarget(t, 2019, "delete (a?.[b])[c]", "delete (a == null ? void 0 : a[b])[c];\n")
	expectPrintedTarget(t, 2019, "delete (a?.(b))(c)", "delete (a == null ? void 0 : a(b))(c);\n")

	expectPrintedTarget(t, 2019, "(delete a?.b).c", "(a == null ? true : delete a.b).c;\n")
	expectPrintedTarget(t, 2019, "(delete a?.[b])[c]", "(a == null ? true : delete a[b])[c];\n")
	expectPrintedTarget(t, 2019, "(delete a?.(b))(c)", "(a == null ? true : delete a(b))(c);\n")

	expectPrintedTarget(t, 2019, "null?.x", "void 0;\n")
	expectPrintedTarget(t, 2019, "null?.[x]", "void 0;\n")
	expectPrintedTarget(t, 2019, "null?.(x)", "void 0;\n")

	expectPrintedTarget(t, 2019, "delete null?.x", "true;\n")
	expectPrintedTarget(t, 2019, "delete null?.[x]", "true;\n")
	expectPrintedTarget(t, 2019, "delete null?.(x)", "true;\n")

	expectPrintedTarget(t, 2019, "undefined?.x", "void 0;\n")
	expectPrintedTarget(t, 2019, "undefined?.[x]", "void 0;\n")
	expectPrintedTarget(t, 2019, "undefined?.(x)", "void 0;\n")

	expectPrintedTarget(t, 2019, "delete undefined?.x", "true;\n")
	expectPrintedTarget(t, 2019, "delete undefined?.[x]", "true;\n")
	expectPrintedTarget(t, 2019, "delete undefined?.(x)", "true;\n")

	expectPrintedTarget(t, 2020, "x?.y", "x?.y;\n")
	expectPrintedTarget(t, 2020, "x?.[y]", "x?.[y];\n")
	expectPrintedTarget(t, 2020, "x?.(y)", "x?.(y);\n")

	expectPrintedTarget(t, 2020, "null?.x", "void 0;\n")
	expectPrintedTarget(t, 2020, "null?.[x]", "void 0;\n")
	expectPrintedTarget(t, 2020, "null?.(x)", "void 0;\n")

	expectPrintedTarget(t, 2020, "undefined?.x", "void 0;\n")
	expectPrintedTarget(t, 2020, "undefined?.[x]", "void 0;\n")
	expectPrintedTarget(t, 2020, "undefined?.(x)", "void 0;\n")

	// Check multiple levels of nesting
	expectPrintedTarget(t, 2019, "a?.b?.c?.d", `var _a, _b;
(_b = (_a = a == null ? void 0 : a.b) == null ? void 0 : _a.c) == null ? void 0 : _b.d;
`)
	expectPrintedTarget(t, 2019, "a?.[b]?.[c]?.[d]", `var _a, _b;
(_b = (_a = a == null ? void 0 : a[b]) == null ? void 0 : _a[c]) == null ? void 0 : _b[d];
`)
	expectPrintedTarget(t, 2019, "a?.(b)?.(c)?.(d)", `var _a, _b;
(_b = (_a = a == null ? void 0 : a(b)) == null ? void 0 : _a(c)) == null ? void 0 : _b(d);
`)

	// Check the need to use ".call()"
	expectPrintedTarget(t, 2019, "a.b?.(c)", `var _a;
(_a = a.b) == null ? void 0 : _a.call(a, c);
`)
	expectPrintedTarget(t, 2019, "a[b]?.(c)", `var _a;
(_a = a[b]) == null ? void 0 : _a.call(a, c);
`)
	expectPrintedTarget(t, 2019, "a?.[b]?.(c)", `var _a;
(_a = a == null ? void 0 : a[b]) == null ? void 0 : _a.call(a, c);
`)
	expectPrintedTarget(t, 2019, "123?.[b]?.(c)", `var _a;
(_a = 123 == null ? void 0 : 123[b]) == null ? void 0 : _a.call(123, c);
`)
	expectPrintedTarget(t, 2019, "a?.[b][c]?.(d)", `var _a, _b;
(_b = a == null ? void 0 : (_a = a[b])[c]) == null ? void 0 : _b.call(_a, d);
`)
	expectPrintedTarget(t, 2019, "a[b][c]?.(d)", `var _a, _b;
(_b = (_a = a[b])[c]) == null ? void 0 : _b.call(_a, d);
`)

	// Check that direct eval status is propagated through optional chaining
	expectPrintedTarget(t, 2019, "eval?.(x)", "eval == null ? void 0 : eval(x);\n")
	expectPrintedTarget(t, 2019, "(1 ? eval : 0)?.(x)", "eval == null ? void 0 : (0, eval)(x);\n")
}

func TestLowerOptionalCatchBinding(t *testing.T) {
	expectPrintedTarget(t, 2019, "try {} catch {}", "try {\n} catch {\n}\n")
	expectPrintedTarget(t, 2018, "try {} catch {}", "try {\n} catch (e) {\n}\n")
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
	expectPrinted(t, "class Foo { set #foo() {} }", "class Foo {\n  set #foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static #foo }", "class Foo {\n  static #foo;\n}\n")
	expectPrinted(t, "class Foo { static #foo = 1 }", "class Foo {\n  static #foo = 1;\n}\n")
	expectPrinted(t, "class Foo { static #foo() {} }", "class Foo {\n  static #foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static get #foo() {} }", "class Foo {\n  static get #foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static set #foo() {} }", "class Foo {\n  static set #foo() {\n  }\n}\n")

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

	// Scope tests
	expectParseError(t, "class Foo { #foo; #foo }", "<stdin>: error: \"#foo\" has already been declared\n")
	expectParseError(t, "class Foo { #foo; static #foo }", "<stdin>: error: \"#foo\" has already been declared\n")
	expectParseError(t, "class Foo { static #foo; #foo }", "<stdin>: error: \"#foo\" has already been declared\n")
	expectParseError(t, "class Foo { #foo; #foo() {} }", "<stdin>: error: \"#foo\" has already been declared\n")
	expectParseError(t, "class Foo { #foo; get #foo() {} }", "<stdin>: error: \"#foo\" has already been declared\n")
	expectParseError(t, "class Foo { #foo; set #foo() {} }", "<stdin>: error: \"#foo\" has already been declared\n")
	expectParseError(t, "class Foo { #foo() {} #foo }", "<stdin>: error: \"#foo\" has already been declared\n")
	expectParseError(t, "class Foo { get #foo() {} #foo }", "<stdin>: error: \"#foo\" has already been declared\n")
	expectParseError(t, "class Foo { set #foo() {} #foo }", "<stdin>: error: \"#foo\" has already been declared\n")
	expectParseError(t, "class Foo { get #foo() {} get #foo() {} }", "<stdin>: error: \"#foo\" has already been declared\n")
	expectParseError(t, "class Foo { set #foo() {} set #foo() {} }", "<stdin>: error: \"#foo\" has already been declared\n")
	expectParseError(t, "class Foo { get #foo() {} set #foo() {} #foo }", "<stdin>: error: \"#foo\" has already been declared\n")
	expectParseError(t, "class Foo { set #foo() {} get #foo() {} #foo }", "<stdin>: error: \"#foo\" has already been declared\n")
	expectPrinted(t, "class Foo { get #foo() {} set #foo() { this.#foo } }",
		"class Foo {\n  get #foo() {\n  }\n  set #foo() {\n    this.#foo;\n  }\n}\n")
	expectPrinted(t, "class Foo { set #foo() { this.#foo } get #foo() {} }",
		"class Foo {\n  set #foo() {\n    this.#foo;\n  }\n  get #foo() {\n  }\n}\n")
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
	expectParseError(t, "class Foo { set #x() { this.#x } }",
		"<stdin>: warning: Reading from setter-only property \"#x\" will throw\n")
	expectParseError(t, "class Foo { set #x() { this.#x += 1 } }",
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
	expectParseErrorTarget(t, 5, "function foo(...x) {}",
		"<stdin>: error: Transforming rest arguments to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(function(...x) {})",
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
	expectParseErrorTarget(t, 5, "({ get x() {} });",
		"<stdin>: error: Transforming object literal extensions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "({ set x() {} });",
		"<stdin>: error: Transforming object literal extensions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "function foo([]) {}",
		"<stdin>: error: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "function foo({}) {}",
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
	expectParseErrorTarget(t, 5, "`abc`;",
		"<stdin>: error: Transforming template literals to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "`a${b}c`;",
		"<stdin>: error: Transforming template literals to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "tag`a${b}c`;",
		"<stdin>: error: Transforming template literals to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "class Foo { constructor() { new.target } }",
		"<stdin>: error: Transforming class syntax to the configured target environment is not supported yet\n"+
			"<stdin>: error: Transforming object literal extensions to the configured target environment is not supported yet\n"+
			"<stdin>: error: Transforming new.target to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "const x = 1;",
		"<stdin>: error: Transforming const to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "let x = 2;",
		"<stdin>: error: Transforming let to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "async => foo;",
		"<stdin>: error: Transforming arrow functions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "async () => foo;",
		"<stdin>: error: Transforming arrow functions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "() => foo;",
		"<stdin>: error: Transforming arrow functions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "x => x;",
		"<stdin>: error: Transforming arrow functions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "class Foo {}",
		"<stdin>: error: Transforming class syntax to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(class {});",
		"<stdin>: error: Transforming class syntax to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "function* gen() {}",
		"<stdin>: error: Transforming generator functions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(function* () {});",
		"<stdin>: error: Transforming generator functions to the configured target environment is not supported yet\n")
}
