package js_parser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_printer"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/renamer"
	"github.com/evanw/esbuild/internal/test"
)

func expectParseErrorCommon(t *testing.T, contents string, expected string, options config.Options) {
	t.Helper()
	t.Run(contents, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		Parse(log, test.SourceForTest(contents), OptionsFromConfig(&options))
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logger.OutputOptions{}, logger.TerminalInfo{})
		}
		test.AssertEqualWithDiff(t, text, expected)
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

func expectPrintedWithUnsupportedFeatures(t *testing.T, unsupportedJSFeatures compat.JSFeature, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		UnsupportedJSFeatures: unsupportedJSFeatures,
	})
}

func expectParseErrorWithUnsupportedFeatures(t *testing.T, unsupportedJSFeatures compat.JSFeature, contents string, expected string) {
	t.Helper()
	expectParseErrorCommon(t, contents, expected, config.Options{
		UnsupportedJSFeatures: unsupportedJSFeatures,
	})
}

func expectPrintedCommon(t *testing.T, contents string, expected string, options config.Options) {
	t.Helper()
	t.Run(contents, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		options.OmitRuntimeForTests = true
		tree, ok := Parse(log, test.SourceForTest(contents), OptionsFromConfig(&options))
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			if msg.Kind != logger.Warning {
				text += msg.String(logger.OutputOptions{}, logger.TerminalInfo{})
			}
		}
		test.AssertEqualWithDiff(t, text, "")
		if !ok {
			t.Fatal("Parse error")
		}
		symbols := js_ast.NewSymbolMap(1)
		symbols.SymbolsForSource[0] = tree.Symbols
		r := renamer.NewNoOpRenamer(symbols)
		js := js_printer.Print(tree, symbols, r, js_printer.Options{
			UnsupportedFeatures: options.UnsupportedJSFeatures,
			ASCIIOnly:           options.ASCIIOnly,
		}).JS
		test.AssertEqualWithDiff(t, string(js), expected)
	})
}

func expectPrinted(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{})
}

func expectPrintedMangle(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		MinifySyntax: true,
	})
}

func expectPrintedNormalAndMangle(t *testing.T, contents string, normal string, mangle string) {
	expectPrinted(t, contents, normal)
	expectPrintedMangle(t, contents, mangle)
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
		MinifySyntax: true,
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

func expectPrintedJSXSideEffects(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		JSX: config.JSXOptions{
			Parse:       true,
			SideEffects: true,
		},
	})
}

func expectPrintedMangleJSX(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		MinifySyntax: true,
		JSX: config.JSXOptions{
			Parse: true,
		},
	})
}

type JSXAutomaticTestOptions struct {
	Development            bool
	ImportSource           string
	OmitJSXRuntimeForTests bool
	SideEffects            bool
}

func expectParseErrorJSXAutomatic(t *testing.T, options JSXAutomaticTestOptions, contents string, expected string) {
	t.Helper()
	expectParseErrorCommon(t, contents, expected, config.Options{
		OmitJSXRuntimeForTests: options.OmitJSXRuntimeForTests,
		JSX: config.JSXOptions{
			AutomaticRuntime: true,
			Parse:            true,
			Development:      options.Development,
			ImportSource:     options.ImportSource,
			SideEffects:      options.SideEffects,
		},
	})
}

func expectPrintedJSXAutomatic(t *testing.T, options JSXAutomaticTestOptions, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, expected, config.Options{
		OmitJSXRuntimeForTests: options.OmitJSXRuntimeForTests,
		JSX: config.JSXOptions{
			AutomaticRuntime: true,
			Parse:            true,
			Development:      options.Development,
			ImportSource:     options.ImportSource,
			SideEffects:      options.SideEffects,
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
	expectParseError(t, "throw //\n x", "<stdin>: ERROR: Unexpected newline after \"throw\"\n")
	expectParseError(t, "throw /**/\n x", "<stdin>: ERROR: Unexpected newline after \"throw\"\n")
	expectParseError(t, "throw <!--\n x",
		`<stdin>: ERROR: Unexpected newline after "throw"
<stdin>: WARNING: Treating "<!--" as the start of a legacy HTML single-line comment
`)
	expectParseError(t, "throw -->\n x", "<stdin>: ERROR: Unexpected \">\"\n")

	expectParseError(t, "export {}\n<!--", `<stdin>: ERROR: Legacy HTML single-line comments are not allowed in ECMAScript modules
<stdin>: NOTE: This file is considered to be an ECMAScript module because of the "export" keyword here:
<stdin>: WARNING: Treating "<!--" as the start of a legacy HTML single-line comment
`)

	expectParseError(t, "export {}\n-->", `<stdin>: ERROR: Legacy HTML single-line comments are not allowed in ECMAScript modules
<stdin>: NOTE: This file is considered to be an ECMAScript module because of the "export" keyword here:
<stdin>: WARNING: Treating "-->" as the start of a legacy HTML single-line comment
`)

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

func TestStrictMode(t *testing.T) {
	useStrict := "<stdin>: NOTE: Strict mode is triggered by the \"use strict\" directive here:\n"

	expectPrinted(t, "'use strict'", "\"use strict\";\n")
	expectPrinted(t, "`use strict`", "`use strict`;\n")
	expectPrinted(t, "//! @legal comment\n 'use strict'", "\"use strict\";\n//! @legal comment\n")
	expectPrinted(t, "/*! @legal comment */ 'use strict'", "\"use strict\";\n/*! @legal comment */\n")
	expectPrinted(t, "function f() { //! @legal comment\n 'use strict' }", "function f() {\n  //! @legal comment\n  \"use strict\";\n}\n")
	expectPrinted(t, "function f() { /*! @legal comment */ 'use strict' }", "function f() {\n  /*! @legal comment */\n  \"use strict\";\n}\n")
	expectParseError(t, "//! @legal comment\n 'use strict'", "")
	expectParseError(t, "/*! @legal comment */ 'use strict'", "")
	expectParseError(t, "function f() { //! @legal comment\n 'use strict' }", "")
	expectParseError(t, "function f() { /*! @legal comment */ 'use strict' }", "")

	nonSimple := "<stdin>: ERROR: Cannot use a \"use strict\" directive in a function with a non-simple parameter list\n"
	expectParseError(t, "function f() { 'use strict' }", "")
	expectParseError(t, "function f(x) { 'use strict' }", "")
	expectParseError(t, "function f([x]) { 'use strict' }", nonSimple)
	expectParseError(t, "function f({x}) { 'use strict' }", nonSimple)
	expectParseError(t, "function f(x = 1) { 'use strict' }", nonSimple)
	expectParseError(t, "function f(x, ...y) { 'use strict' }", nonSimple)
	expectParseError(t, "(function() { 'use strict' })", "")
	expectParseError(t, "(function(x) { 'use strict' })", "")
	expectParseError(t, "(function([x]) { 'use strict' })", nonSimple)
	expectParseError(t, "(function({x}) { 'use strict' })", nonSimple)
	expectParseError(t, "(function(x = 1) { 'use strict' })", nonSimple)
	expectParseError(t, "(function(x, ...y) { 'use strict' })", nonSimple)
	expectParseError(t, "() => { 'use strict' }", "")
	expectParseError(t, "(x) => { 'use strict' }", "")
	expectParseError(t, "([x]) => { 'use strict' }", nonSimple)
	expectParseError(t, "({x}) => { 'use strict' }", nonSimple)
	expectParseError(t, "(x = 1) => { 'use strict' }", nonSimple)
	expectParseError(t, "(x, ...y) => { 'use strict' }", nonSimple)
	expectParseError(t, "(x, ...y) => { //! @license comment\n 'use strict' }", nonSimple)

	why := "<stdin>: NOTE: This file is considered to be an ECMAScript module because of the \"export\" keyword here:\n"

	expectPrinted(t, "let x = '\\0'", "let x = \"\\0\";\n")
	expectPrinted(t, "let x = '\\00'", "let x = \"\\0\";\n")
	expectPrinted(t, "'use strict'; let x = '\\0'", "\"use strict\";\nlet x = \"\\0\";\n")
	expectPrinted(t, "let x = '\\0'; export {}", "let x = \"\\0\";\nexport {};\n")
	expectParseError(t, "'use strict'; let x = '\\00'", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; let x = '\\08'", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; let x = '\\008'", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "let x = '\\00'; export {}", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in an ECMAScript module\n"+why)
	expectParseError(t, "let x = '\\09'; export {}", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in an ECMAScript module\n"+why)
	expectParseError(t, "let x = '\\009'; export {}", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in an ECMAScript module\n"+why)

	expectPrinted(t, "'\\0'", "\"\\0\";\n")
	expectPrinted(t, "'\\00'", "\"\\0\";\n")
	expectPrinted(t, "'use strict'; '\\0'", "\"use strict\";\n\"\\0\";\n")
	expectParseError(t, "'use strict'; '\\00'", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; '\\08'", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; '\\008'", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'\\00'; 'use strict';", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'\\08'; 'use strict';", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'\\008'; 'use strict';", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'\\00'; export {}", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in an ECMAScript module\n"+why)
	expectParseError(t, "'\\09'; export {}", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in an ECMAScript module\n"+why)
	expectParseError(t, "'\\009'; export {}", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in an ECMAScript module\n"+why)

	expectPrinted(t, "with (x) y", "with (x)\n  y;\n")
	expectParseError(t, "'use strict'; with (x) y", "<stdin>: ERROR: With statements cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "with (x) y; export {}", "<stdin>: ERROR: With statements cannot be used in an ECMAScript module\n"+why)

	expectPrinted(t, "delete x", "delete x;\n")
	expectParseError(t, "'use strict'; delete x", "<stdin>: ERROR: Delete of a bare identifier cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "delete x; export {}", "<stdin>: ERROR: Delete of a bare identifier cannot be used in an ECMAScript module\n"+why)

	expectPrinted(t, "for (var x = y in z) ;", "x = y;\nfor (var x in z)\n  ;\n")
	expectParseError(t, "'use strict'; for (var x = y in z) ;",
		"<stdin>: ERROR: Variable initializers inside for-in loops cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "for (var x = y in z) ; export {}",
		"<stdin>: ERROR: Variable initializers inside for-in loops cannot be used in an ECMAScript module\n"+why)

	expectPrinted(t, "function f(a, a) {}", "function f(a, a) {\n}\n")
	expectPrinted(t, "(function(a, a) {})", "(function(a, a) {\n});\n")
	expectPrinted(t, "({ f: function(a, a) {} })", "({ f: function(a, a) {\n} });\n")
	expectPrinted(t, "({ f: function*(a, a) {} })", "({ f: function* (a, a) {\n} });\n")
	expectPrinted(t, "({ f: async function(a, a) {} })", "({ f: async function(a, a) {\n} });\n")

	bindingError := "<stdin>: ERROR: \"a\" cannot be bound multiple times in the same parameter list\n" +
		"<stdin>: NOTE: The name \"a\" was originally bound here:\n"

	expectParseError(t, "function f(a, a) { 'use strict' }", bindingError)
	expectParseError(t, "function *f(a, a) { 'use strict' }", bindingError)
	expectParseError(t, "async function f(a, a) { 'use strict' }", bindingError)
	expectParseError(t, "(function(a, a) { 'use strict' })", bindingError)
	expectParseError(t, "(function*(a, a) { 'use strict' })", bindingError)
	expectParseError(t, "(async function(a, a) { 'use strict' })", bindingError)
	expectParseError(t, "function f(a, [a]) {}", bindingError)
	expectParseError(t, "function f([a], a) {}", bindingError)
	expectParseError(t, "'use strict'; function f(a, a) {}", bindingError)
	expectParseError(t, "'use strict'; (function(a, a) {})", bindingError)
	expectParseError(t, "'use strict'; ((a, a) => {})", bindingError)
	expectParseError(t, "function f(a, a) {}; export {}", bindingError)
	expectParseError(t, "(function(a, a) {}); export {}", bindingError)
	expectParseError(t, "(function(a, [a]) {})", bindingError)
	expectParseError(t, "({ f(a, a) {} })", bindingError)
	expectParseError(t, "({ *f(a, a) {} })", bindingError)
	expectParseError(t, "({ async f(a, a) {} })", bindingError)
	expectParseError(t, "(a, a) => {}", bindingError)

	expectParseError(t, "'use strict'; if (0) function f() {}",
		"<stdin>: ERROR: Function declarations inside if statements cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; if (0) ; else function f() {}",
		"<stdin>: ERROR: Function declarations inside if statements cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; x: function f() {}",
		"<stdin>: ERROR: Function declarations inside labels cannot be used in strict mode\n"+useStrict)

	expectParseError(t, "if (0) function f() {} export {}",
		"<stdin>: ERROR: Function declarations inside if statements cannot be used in an ECMAScript module\n"+why)
	expectParseError(t, "if (0) ; else function f() {} export {}",
		"<stdin>: ERROR: Function declarations inside if statements cannot be used in an ECMAScript module\n"+why)
	expectParseError(t, "x: function f() {} export {}",
		"<stdin>: ERROR: Function declarations inside labels cannot be used in an ECMAScript module\n"+why)

	expectPrinted(t, "eval++", "eval++;\n")
	expectPrinted(t, "eval = 0", "eval = 0;\n")
	expectPrinted(t, "eval += 0", "eval += 0;\n")
	expectPrinted(t, "[eval] = 0", "[eval] = 0;\n")
	expectPrinted(t, "arguments++", "arguments++;\n")
	expectPrinted(t, "arguments = 0", "arguments = 0;\n")
	expectPrinted(t, "arguments += 0", "arguments += 0;\n")
	expectPrinted(t, "[arguments] = 0", "[arguments] = 0;\n")
	expectParseError(t, "'use strict'; eval++", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "'use strict'; eval = 0", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "'use strict'; eval += 0", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "'use strict'; [eval] = 0", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "'use strict'; arguments++", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "'use strict'; arguments = 0", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "'use strict'; arguments += 0", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "'use strict'; [arguments] = 0", "<stdin>: ERROR: Invalid assignment target\n")

	evalDecl := "<stdin>: ERROR: Declarations with the name \"eval\" cannot be used in strict mode\n" + useStrict
	argsDecl := "<stdin>: ERROR: Declarations with the name \"arguments\" cannot be used in strict mode\n" + useStrict
	expectPrinted(t, "function eval() {}", "function eval() {\n}\n")
	expectPrinted(t, "function arguments() {}", "function arguments() {\n}\n")
	expectPrinted(t, "function f(eval) {}", "function f(eval) {\n}\n")
	expectPrinted(t, "function f(arguments) {}", "function f(arguments) {\n}\n")
	expectPrinted(t, "({ f(eval) {} })", "({ f(eval) {\n} });\n")
	expectPrinted(t, "({ f(arguments) {} })", "({ f(arguments) {\n} });\n")
	expectParseError(t, "'use strict'; function eval() {}", evalDecl)
	expectParseError(t, "'use strict'; function arguments() {}", argsDecl)
	expectParseError(t, "'use strict'; function f(eval) {}", evalDecl)
	expectParseError(t, "'use strict'; function f(arguments) {}", argsDecl)
	expectParseError(t, "function eval() { 'use strict' }", evalDecl)
	expectParseError(t, "function arguments() { 'use strict' }", argsDecl)
	expectParseError(t, "function f(eval) { 'use strict' }", evalDecl)
	expectParseError(t, "function f(arguments) { 'use strict' }", argsDecl)
	expectParseError(t, "({ f(eval) { 'use strict' } })", evalDecl)
	expectParseError(t, "({ f(arguments) { 'use strict' } })", argsDecl)
	expectParseError(t, "'use strict'; class eval {}", evalDecl)
	expectParseError(t, "'use strict'; class arguments {}", argsDecl)

	expectPrinted(t, "let protected", "let protected;\n")
	expectPrinted(t, "let protecte\\u0064", "let protected;\n")
	expectPrinted(t, "let x = protected", "let x = protected;\n")
	expectPrinted(t, "let x = protecte\\u0064", "let x = protected;\n")
	expectParseError(t, "'use strict'; let protected",
		"<stdin>: ERROR: \"protected\" is a reserved word and cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; let protecte\\u0064",
		"<stdin>: ERROR: \"protected\" is a reserved word and cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; let x = protected",
		"<stdin>: ERROR: \"protected\" is a reserved word and cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; let x = protecte\\u0064",
		"<stdin>: ERROR: \"protected\" is a reserved word and cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; protected: 0",
		"<stdin>: ERROR: \"protected\" is a reserved word and cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; protecte\\u0064: 0",
		"<stdin>: ERROR: \"protected\" is a reserved word and cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; function protected() {}",
		"<stdin>: ERROR: \"protected\" is a reserved word and cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; function protecte\\u0064() {}",
		"<stdin>: ERROR: \"protected\" is a reserved word and cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; (function protected() {})",
		"<stdin>: ERROR: \"protected\" is a reserved word and cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; (function protecte\\u0064() {})",
		"<stdin>: ERROR: \"protected\" is a reserved word and cannot be used in strict mode\n"+useStrict)

	expectPrinted(t, "0123", "83;\n")
	expectPrinted(t, "({0123: 4})", "({ 83: 4 });\n")
	expectPrinted(t, "let {0123: x} = y", "let { 83: x } = y;\n")
	expectParseError(t, "'use strict'; 0123",
		"<stdin>: ERROR: Legacy octal literals cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; ({0123: 4})",
		"<stdin>: ERROR: Legacy octal literals cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; let {0123: x} = y",
		"<stdin>: ERROR: Legacy octal literals cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; 08",
		"<stdin>: ERROR: Legacy octal literals cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; ({08: 4})",
		"<stdin>: ERROR: Legacy octal literals cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "'use strict'; let {08: x} = y",
		"<stdin>: ERROR: Legacy octal literals cannot be used in strict mode\n"+useStrict)

	classNote := "<stdin>: NOTE: All code inside a class is implicitly in strict mode\n"

	expectPrinted(t, "function f() { 'use strict' } with (x) y", "function f() {\n  \"use strict\";\n}\nwith (x)\n  y;\n")
	expectPrinted(t, "with (x) y; function f() { 'use strict' }", "with (x)\n  y;\nfunction f() {\n  \"use strict\";\n}\n")
	expectPrinted(t, "class f {} with (x) y", "class f {\n}\nwith (x)\n  y;\n")
	expectPrinted(t, "with (x) y; class f {}", "with (x)\n  y;\nclass f {\n}\n")
	expectPrinted(t, "`use strict`; with (x) y", "`use strict`;\nwith (x)\n  y;\n")
	expectPrinted(t, "{ 'use strict'; with (x) y }", "{\n  \"use strict\";\n  with (x)\n    y;\n}\n")
	expectPrinted(t, "if (0) { 'use strict'; with (x) y }", "if (0) {\n  \"use strict\";\n  with (x)\n    y;\n}\n")
	expectPrinted(t, "while (0) { 'use strict'; with (x) y }", "while (0) {\n  \"use strict\";\n  with (x)\n    y;\n}\n")
	expectPrinted(t, "try { 'use strict'; with (x) y } catch {}", "try {\n  \"use strict\";\n  with (x)\n    y;\n} catch {\n}\n")
	expectPrinted(t, "try {} catch { 'use strict'; with (x) y }", "try {\n} catch {\n  \"use strict\";\n  with (x)\n    y;\n}\n")
	expectPrinted(t, "try {} finally { 'use strict'; with (x) y }", "try {\n} finally {\n  \"use strict\";\n  with (x)\n    y;\n}\n")
	expectParseError(t, "\"use strict\"; with (x) y", "<stdin>: ERROR: With statements cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "function f() { 'use strict'; with (x) y }", "<stdin>: ERROR: With statements cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "function f() { 'use strict'; function y() { with (x) y } }", "<stdin>: ERROR: With statements cannot be used in strict mode\n"+useStrict)
	expectParseError(t, "class f { x() { with (x) y } }", "<stdin>: ERROR: With statements cannot be used in strict mode\n"+classNote)
	expectParseError(t, "class f { x() { function y() { with (x) y } } }", "<stdin>: ERROR: With statements cannot be used in strict mode\n"+classNote)
	expectParseError(t, "class f { x() { function protected() {} } }", "<stdin>: ERROR: \"protected\" is a reserved word and cannot be used in strict mode\n"+classNote)

	reservedWordExport := "<stdin>: ERROR: \"protected\" is a reserved word and cannot be used in an ECMAScript module\n" +
		why

	expectParseError(t, "var protected; export {}", reservedWordExport)
	expectParseError(t, "class protected {} export {}", reservedWordExport)
	expectParseError(t, "(class protected {}); export {}", reservedWordExport)
	expectParseError(t, "function protected() {} export {}", reservedWordExport)
	expectParseError(t, "(function protected() {}); export {}", reservedWordExport)

	importMeta := "<stdin>: ERROR: With statements cannot be used in an ECMAScript module\n" +
		"<stdin>: NOTE: This file is considered to be an ECMAScript module because of the use of \"import.meta\" here:\n"
	importStatement := "<stdin>: ERROR: With statements cannot be used in an ECMAScript module\n" +
		"<stdin>: NOTE: This file is considered to be an ECMAScript module because of the \"import\" keyword here:\n"
	exportKeyword := "<stdin>: ERROR: With statements cannot be used in an ECMAScript module\n" +
		"<stdin>: NOTE: This file is considered to be an ECMAScript module because of the \"export\" keyword here:\n"
	tlaKeyword := "<stdin>: ERROR: With statements cannot be used in an ECMAScript module\n" +
		"<stdin>: NOTE: This file is considered to be an ECMAScript module because of the top-level \"await\" keyword here:\n"

	expectPrinted(t, "import(x); with (y) z", "import(x);\nwith (y)\n  z;\n")
	expectPrinted(t, "import('x'); with (y) z", "import(\"x\");\nwith (y)\n  z;\n")
	expectPrinted(t, "with (y) z; import(x)", "with (y)\n  z;\nimport(x);\n")
	expectPrinted(t, "with (y) z; import('x')", "with (y)\n  z;\nimport(\"x\");\n")
	expectPrinted(t, "(import(x)); with (y) z", "import(x);\nwith (y)\n  z;\n")
	expectPrinted(t, "(import('x')); with (y) z", "import(\"x\");\nwith (y)\n  z;\n")
	expectPrinted(t, "with (y) z; (import(x))", "with (y)\n  z;\nimport(x);\n")
	expectPrinted(t, "with (y) z; (import('x'))", "with (y)\n  z;\nimport(\"x\");\n")

	expectParseError(t, "import.meta; with (y) z", importMeta)
	expectParseError(t, "with (y) z; import.meta", importMeta)
	expectParseError(t, "(import.meta); with (y) z", importMeta)
	expectParseError(t, "with (y) z; (import.meta)", importMeta)
	expectParseError(t, "import 'x'; with (y) z", importStatement)
	expectParseError(t, "import * as x from 'x'; with (y) z", importStatement)
	expectParseError(t, "import x from 'x'; with (y) z", importStatement)
	expectParseError(t, "import {x} from 'x'; with (y) z", importStatement)

	expectParseError(t, "export {}; with (y) z", exportKeyword)
	expectParseError(t, "export let x; with (y) z", exportKeyword)
	expectParseError(t, "export function x() {} with (y) z", exportKeyword)
	expectParseError(t, "export class x {} with (y) z", exportKeyword)

	expectParseError(t, "await 0; with (y) z", tlaKeyword)
	expectParseError(t, "with (y) z; await 0", tlaKeyword)
	expectParseError(t, "for await (x of y); with (y) z", tlaKeyword)
	expectParseError(t, "with (y) z; for await (x of y);", tlaKeyword)
	expectParseError(t, "await using x = _; with (y) z", tlaKeyword)
	expectParseError(t, "with (y) z; await using x = _", tlaKeyword)
	expectParseError(t, "for (await using x of _) ; with (y) z", tlaKeyword)
	expectParseError(t, "with (y) z; for (await using x of _) ;", tlaKeyword)

	fAlreadyDeclaredError := "<stdin>: ERROR: The symbol \"f\" has already been declared\n" +
		"<stdin>: NOTE: The symbol \"f\" was originally declared here:\n"
	nestedNote := "<stdin>: NOTE: Duplicate function declarations are not allowed in nested blocks"
	moduleNote := "<stdin>: NOTE: Duplicate top-level function declarations are not allowed in an ECMAScript module. " +
		"This file is considered to be an ECMAScript module because of the \"export\" keyword here:\n"

	cases := []string{
		"function f() {} function f() {}",
		"function f() {} function *f() {}",
		"function *f() {} function f() {}",
		"function f() {} async function f() {}",
		"async function f() {} function f() {}",
		"function f() {} async function *f() {}",
		"async function *f() {} function f() {}",
	}

	for _, c := range cases {
		expectParseError(t, c, "")
		expectParseError(t, "'use strict'; "+c, "")
		expectParseError(t, "function foo() { 'use strict'; "+c+" }", "")
	}

	expectParseError(t, "function f() {} function f() {} export {}", fAlreadyDeclaredError+moduleNote)
	expectParseError(t, "function f() {} function *f() {} export {}", fAlreadyDeclaredError+moduleNote)
	expectParseError(t, "function f() {} async function f() {} export {}", fAlreadyDeclaredError+moduleNote)
	expectParseError(t, "function *f() {} function f() {} export {}", fAlreadyDeclaredError+moduleNote)
	expectParseError(t, "async function f() {} function f() {} export {}", fAlreadyDeclaredError+moduleNote)

	expectParseError(t, "'use strict'; { function f() {} function f() {} }",
		fAlreadyDeclaredError+nestedNote+" in strict mode. Strict mode is triggered by the \"use strict\" directive here:\n")
	expectParseError(t, "'use strict'; switch (0) { case 1: function f() {} default: function f() {} }",
		fAlreadyDeclaredError+nestedNote+" in strict mode. Strict mode is triggered by the \"use strict\" directive here:\n")

	expectParseError(t, "function foo() { 'use strict'; { function f() {} function f() {} } }",
		fAlreadyDeclaredError+nestedNote+" in strict mode. Strict mode is triggered by the \"use strict\" directive here:\n")
	expectParseError(t, "function foo() { 'use strict'; switch (0) { case 1: function f() {} default: function f() {} } }",
		fAlreadyDeclaredError+nestedNote+" in strict mode. Strict mode is triggered by the \"use strict\" directive here:\n")

	expectParseError(t, "{ function f() {} function f() {} } export {}",
		fAlreadyDeclaredError+nestedNote+" in an ECMAScript module. This file is considered to be an ECMAScript module because of the \"export\" keyword here:\n")
	expectParseError(t, "switch (0) { case 1: function f() {} default: function f() {} } export {}",
		fAlreadyDeclaredError+nestedNote+" in an ECMAScript module. This file is considered to be an ECMAScript module because of the \"export\" keyword here:\n")

	expectParseError(t, "var x; var x", "")
	expectParseError(t, "'use strict'; var x; var x", "")
	expectParseError(t, "var x; var x; export {}", "")
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

	expectParseError(t, "-x ** 2", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "+x ** 2", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "~x ** 2", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "!x ** 2", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "void x ** 2", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "delete x ** 2", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "typeof x ** 2", "<stdin>: ERROR: Unexpected \"**\"\n")

	expectParseError(t, "-x.y() ** 2", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "+x.y() ** 2", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "~x.y() ** 2", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "!x.y() ** 2", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "void x.y() ** 2", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "delete x.y() ** 2", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "typeof x.y() ** 2", "<stdin>: ERROR: Unexpected \"**\"\n")

	// https://github.com/tc39/ecma262/issues/2197
	expectParseError(t, "delete x ** 0", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "delete x.prop ** 0", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "delete x[0] ** 0", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "delete x?.prop ** 0", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "void x ** 0", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "typeof x ** 0", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "+x ** 0", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "-x ** 0", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "~x ** 0", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "!x ** 0", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "await x ** 0", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectParseError(t, "await -x ** 0", "<stdin>: ERROR: Unexpected \"**\"\n")
	expectPrinted(t, "(delete x) ** 0", "(delete x) ** 0;\n")
	expectPrinted(t, "(delete x.prop) ** 0", "(delete x.prop) ** 0;\n")
	expectPrinted(t, "(delete x[0]) ** 0", "(delete x[0]) ** 0;\n")
	expectPrinted(t, "(delete x?.prop) ** 0", "(delete x?.prop) ** 0;\n")
	expectPrinted(t, "(void x) ** 0", "(void x) ** 0;\n")
	expectPrinted(t, "(typeof x) ** 0", "(typeof x) ** 0;\n")
	expectPrinted(t, "(+x) ** 0", "(+x) ** 0;\n")
	expectPrinted(t, "(-x) ** 0", "(-x) ** 0;\n")
	expectPrinted(t, "(~x) ** 0", "(~x) ** 0;\n")
	expectPrinted(t, "(!x) ** 0", "(!x) ** 0;\n")
	expectPrinted(t, "(await x) ** 0", "(await x) ** 0;\n")
	expectPrinted(t, "(await -x) ** 0", "(await -x) ** 0;\n")
}

func TestAwait(t *testing.T) {
	expectPrinted(t, "await x", "await x;\n")
	expectPrinted(t, "await +x", "await +x;\n")
	expectPrinted(t, "await -x", "await -x;\n")
	expectPrinted(t, "await ~x", "await ~x;\n")
	expectPrinted(t, "await !x", "await !x;\n")
	expectPrinted(t, "await --x", "await --x;\n")
	expectPrinted(t, "await ++x", "await ++x;\n")
	expectPrinted(t, "await x--", "await x--;\n")
	expectPrinted(t, "await x++", "await x++;\n")
	expectPrinted(t, "await void x", "await void x;\n")
	expectPrinted(t, "await typeof x", "await typeof x;\n")
	expectPrinted(t, "await (x * y)", "await (x * y);\n")
	expectPrinted(t, "await (x ** y)", "await (x ** y);\n")

	expectParseError(t, "var { await } = {}", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")
	expectParseError(t, "async function f() { var { await } = {} }", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")
	expectParseError(t, "async function* f() { var { await } = {} }", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")
	expectParseError(t, "class C { async f() { var { await } = {} } }", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")
	expectParseError(t, "class C { async* f() { var { await } = {} } }", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")
	expectParseError(t, "class C { static { var { await } = {} } }", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")

	expectParseError(t, "var {} = { await }", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")
	expectParseError(t, "async function f() { var {} = { await } }", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")
	expectParseError(t, "async function* f() { var {} = { await } }", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")
	expectParseError(t, "class C { async f() { var {} = { await } } }", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")
	expectParseError(t, "class C { async* f() { var {} = { await } } }", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")
	expectParseError(t, "class C { static { var {} = { await } } }", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")

	expectParseError(t, "await delete x",
		`<stdin>: ERROR: Delete of a bare identifier cannot be used in an ECMAScript module
<stdin>: NOTE: This file is considered to be an ECMAScript module because of the top-level "await" keyword here:
`)
	expectPrinted(t, "async function f() { await delete x }", "async function f() {\n  await delete x;\n}\n")

	// Can't use await at the top-level without top-level await
	err := "<stdin>: ERROR: Top-level await is not available in the configured target environment\n"
	expectParseErrorWithUnsupportedFeatures(t, compat.TopLevelAwait, "await x;", err)
	expectParseErrorWithUnsupportedFeatures(t, compat.TopLevelAwait, "if (true) await x;", err)
	expectPrintedWithUnsupportedFeatures(t, compat.TopLevelAwait, "if (false) await x;", "if (false)\n  x;\n")
	expectParseErrorWithUnsupportedFeatures(t, compat.TopLevelAwait, "with (x) y; if (false) await x;",
		"<stdin>: ERROR: With statements cannot be used in an ECMAScript module\n"+
			"<stdin>: NOTE: This file is considered to be an ECMAScript module because of the top-level \"await\" keyword here:\n")
}

func TestRegExp(t *testing.T) {
	expectPrinted(t, "/x/d", "/x/d;\n")
	expectPrinted(t, "/x/g", "/x/g;\n")
	expectPrinted(t, "/x/i", "/x/i;\n")
	expectPrinted(t, "/x/m", "/x/m;\n")
	expectPrinted(t, "/x/s", "/x/s;\n")
	expectPrinted(t, "/x/u", "/x/u;\n")
	expectPrinted(t, "/x/y", "/x/y;\n")

	expectParseError(t, "/)/", "<stdin>: ERROR: Unexpected \")\" in regular expression\n")
	expectPrinted(t, "/[\\])]/", "/[\\])]/;\n")

	expectParseError(t, "/x/msuygig",
		`<stdin>: ERROR: Duplicate flag "g" in regular expression
<stdin>: NOTE: The first "g" was here:
`)
}

func TestUnicodeIdentifierNames(t *testing.T) {
	// There are two code points that are valid in identifiers in ES5 but not in ES6+:
	//
	//   U+30FB KATAKANA MIDDLE DOT
	//   U+FF65 HALFWIDTH KATAKANA MIDDLE DOT
	//
	expectPrinted(t, "x = {x・: 0}", "x = { \"x・\": 0 };\n")
	expectPrinted(t, "x = {x･: 0}", "x = { \"x･\": 0 };\n")
	expectPrinted(t, "x = {xπ: 0}", "x = { xπ: 0 };\n")
	expectPrinted(t, "x = y.x・", "x = y[\"x・\"];\n")
	expectPrinted(t, "x = y.x･", "x = y[\"x･\"];\n")
	expectPrinted(t, "x = y.xπ", "x = y.xπ;\n")
}

func TestIdentifierEscapes(t *testing.T) {
	expectPrinted(t, "var _\\u0076\\u0061\\u0072", "var _var;\n")
	expectParseError(t, "var \\u0076\\u0061\\u0072", "<stdin>: ERROR: Expected identifier but found \"\\\\u0076\\\\u0061\\\\u0072\"\n")
	expectParseError(t, "\\u0076\\u0061\\u0072 foo", "<stdin>: ERROR: Unexpected \"\\\\u0076\\\\u0061\\\\u0072\"\n")

	expectPrinted(t, "foo._\\u0076\\u0061\\u0072", "foo._var;\n")
	expectPrinted(t, "foo.\\u0076\\u0061\\u0072", "foo.var;\n")

	expectParseError(t, "\u200Ca", "<stdin>: ERROR: Unexpected \"\\u200c\"\n")
	expectParseError(t, "\u200Da", "<stdin>: ERROR: Unexpected \"\\u200d\"\n")
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
	expectParseError(t, "const x", "<stdin>: ERROR: The constant \"x\" must be initialized\n")
	expectParseError(t, "const {}", "<stdin>: ERROR: This constant must be initialized\n")
	expectParseError(t, "const []", "<stdin>: ERROR: This constant must be initialized\n")
	expectParseError(t, "for (var x;;) ;", "")
	expectParseError(t, "for (let x;;) ;", "")
	expectParseError(t, "for (const x;;) ;", "<stdin>: ERROR: The constant \"x\" must be initialized\n")
	expectParseError(t, "for (const {};;) ;", "<stdin>: ERROR: This constant must be initialized\n")
	expectParseError(t, "for (const [];;) ;", "<stdin>: ERROR: This constant must be initialized\n")

	// Make sure bindings are visited during parsing
	expectPrinted(t, "var {[x]: y} = {}", "var { [x]: y } = {};\n")
	expectPrinted(t, "var {...x} = {}", "var { ...x } = {};\n")

	// Test destructuring patterns
	expectPrinted(t, "var [...x] = []", "var [...x] = [];\n")
	expectPrinted(t, "var {...x} = {}", "var { ...x } = {};\n")
	expectPrinted(t, "([...x] = []) => {}", "([...x] = []) => {\n};\n")
	expectPrinted(t, "({...x} = {}) => {}", "({ ...x } = {}) => {\n};\n")

	expectParseError(t, "var [...x,] = []", "<stdin>: ERROR: Unexpected \",\" after rest pattern\n")
	expectParseError(t, "var {...x,} = {}", "<stdin>: ERROR: Unexpected \",\" after rest pattern\n")
	expectParseError(t, "([...x,] = []) => {}", "<stdin>: ERROR: Invalid binding pattern\n")
	expectParseError(t, "({...x,} = {}) => {}", "<stdin>: ERROR: Invalid binding pattern\n")

	expectPrinted(t, "[b, ...c] = d", "[b, ...c] = d;\n")
	expectPrinted(t, "([b, ...c] = d)", "[b, ...c] = d;\n")
	expectPrinted(t, "({b, ...c} = d)", "({ b, ...c } = d);\n")
	expectPrinted(t, "({a = b} = c)", "({ a = b } = c);\n")
	expectPrinted(t, "({a: b = c} = d)", "({ a: b = c } = d);\n")
	expectPrinted(t, "({a: b.c} = d)", "({ a: b.c } = d);\n")
	expectPrinted(t, "[a = {}] = b", "[a = {}] = b;\n")
	expectPrinted(t, "[[...a, b].x] = c", "[[...a, b].x] = c;\n")
	expectPrinted(t, "[{...a, b}.x] = c", "[{ ...a, b }.x] = c;\n")
	expectPrinted(t, "({x: [...a, b].x} = c)", "({ x: [...a, b].x } = c);\n")
	expectPrinted(t, "({x: {...a, b}.x} = c)", "({ x: { ...a, b }.x } = c);\n")
	expectPrinted(t, "[x = [...a, b]] = c", "[x = [...a, b]] = c;\n")
	expectPrinted(t, "[x = {...a, b}] = c", "[x = { ...a, b }] = c;\n")
	expectPrinted(t, "({x = [...a, b]} = c)", "({ x = [...a, b] } = c);\n")
	expectPrinted(t, "({x = {...a, b}} = c)", "({ x = { ...a, b } } = c);\n")

	expectPrinted(t, "(x = y)", "x = y;\n")
	expectPrinted(t, "([] = [])", "[] = [];\n")
	expectPrinted(t, "({} = {})", "({} = {});\n")
	expectPrinted(t, "([[]] = [[]])", "[[]] = [[]];\n")
	expectPrinted(t, "({x: {}} = {x: {}})", "({ x: {} } = { x: {} });\n")
	expectPrinted(t, "(x) = y", "x = y;\n")
	expectParseError(t, "([]) = []", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "({}) = {}", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "[([])] = [[]]", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "({x: ({})} = {x: {}})", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "(([]) = []) => {}", "<stdin>: ERROR: Invalid binding pattern\n")
	expectParseError(t, "(({}) = {}) => {}", "<stdin>: ERROR: Invalid binding pattern\n")
	expectParseError(t, "function f(([]) = []) {}", "<stdin>: ERROR: Expected identifier but found \"(\"\n")
	expectParseError(t, "function f(({}) = {}) {}", "<stdin>: ERROR: Expected identifier but found \"(\"\n")

	expectPrinted(t, "for (x in y) ;", "for (x in y)\n  ;\n")
	expectPrinted(t, "for ([] in y) ;", "for ([] in y)\n  ;\n")
	expectPrinted(t, "for ({} in y) ;", "for ({} in y)\n  ;\n")
	expectPrinted(t, "for ((x) in y) ;", "for (x in y)\n  ;\n")
	expectParseError(t, "for (([]) in y) ;", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "for (({}) in y) ;", "<stdin>: ERROR: Invalid assignment target\n")

	expectPrinted(t, "for (x of y) ;", "for (x of y)\n  ;\n")
	expectPrinted(t, "for ([] of y) ;", "for ([] of y)\n  ;\n")
	expectPrinted(t, "for ({} of y) ;", "for ({} of y)\n  ;\n")
	expectPrinted(t, "for ((x) of y) ;", "for (x of y)\n  ;\n")
	expectParseError(t, "for (([]) of y) ;", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "for (({}) of y) ;", "<stdin>: ERROR: Invalid assignment target\n")

	expectParseError(t, "[[...a, b]] = c", "<stdin>: ERROR: Unexpected \",\" after rest pattern\n")
	expectParseError(t, "[{...a, b}] = c", "<stdin>: ERROR: Unexpected \",\" after rest pattern\n")
	expectParseError(t, "({x: [...a, b]} = c)", "<stdin>: ERROR: Unexpected \",\" after rest pattern\n")
	expectParseError(t, "({x: {...a, b}} = c)", "<stdin>: ERROR: Unexpected \",\" after rest pattern\n")
	expectParseError(t, "[b, ...c,] = d", "<stdin>: ERROR: Unexpected \",\" after rest pattern\n")
	expectParseError(t, "([b, ...c,] = d)", "<stdin>: ERROR: Unexpected \",\" after rest pattern\n")
	expectParseError(t, "({b, ...c,} = d)", "<stdin>: ERROR: Unexpected \",\" after rest pattern\n")
	expectParseError(t, "({a = b})", "<stdin>: ERROR: Unexpected \"=\"\n")
	expectParseError(t, "({x = {a = b}} = c)", "<stdin>: ERROR: Unexpected \"=\"\n")
	expectParseError(t, "[a = {b = c}] = d", "<stdin>: ERROR: Unexpected \"=\"\n")

	expectPrinted(t, "for ([{a = {}}] in b) {}", "for ([{ a = {} }] in b) {\n}\n")
	expectPrinted(t, "for ([{a = {}}] of b) {}", "for ([{ a = {} }] of b) {\n}\n")
	expectPrinted(t, "for ({a = {}} in b) {}", "for ({ a = {} } in b) {\n}\n")
	expectPrinted(t, "for ({a = {}} of b) {}", "for ({ a = {} } of b) {\n}\n")

	expectParseError(t, "({a = {}} in b)", "<stdin>: ERROR: Unexpected \"=\"\n")
	expectParseError(t, "[{a = {}}]\nof()", "<stdin>: ERROR: Unexpected \"=\"\n")
	expectParseError(t, "for ([...a, b] in c) {}", "<stdin>: ERROR: Unexpected \",\" after rest pattern\n")
	expectParseError(t, "for ([...a, b] of c) {}", "<stdin>: ERROR: Unexpected \",\" after rest pattern\n")
}

func TestBreakAndContinue(t *testing.T) {
	expectParseError(t, "break", "<stdin>: ERROR: Cannot use \"break\" here:\n")
	expectParseError(t, "continue", "<stdin>: ERROR: Cannot use \"continue\" here:\n")

	expectParseError(t, "x: { break }", "<stdin>: ERROR: Cannot use \"break\" here:\n")
	expectParseError(t, "x: { break x }", "")
	expectParseError(t, "x: { continue }", "<stdin>: ERROR: Cannot use \"continue\" here:\n")
	expectParseError(t, "x: { continue x }", "<stdin>: ERROR: Cannot continue to label \"x\"\n")

	expectParseError(t, "while (1) break", "")
	expectParseError(t, "while (1) continue", "")
	expectParseError(t, "while (1) { function foo() { break } }", "<stdin>: ERROR: Cannot use \"break\" here:\n")
	expectParseError(t, "while (1) { function foo() { continue } }", "<stdin>: ERROR: Cannot use \"continue\" here:\n")
	expectParseError(t, "x: while (1) break x", "")
	expectParseError(t, "x: while (1) continue x", "")
	expectParseError(t, "x: while (1) y: { break x }", "")
	expectParseError(t, "x: while (1) y: { continue x }", "")
	expectParseError(t, "x: while (1) y: { break y }", "")
	expectParseError(t, "x: while (1) y: { continue y }", "<stdin>: ERROR: Cannot continue to label \"y\"\n")
	expectParseError(t, "x: while (1) { function foo() { break x } }", "<stdin>: ERROR: There is no containing label named \"x\"\n")
	expectParseError(t, "x: while (1) { function foo() { continue x } }", "<stdin>: ERROR: There is no containing label named \"x\"\n")

	expectParseError(t, "switch (1) { case 1: break }", "")
	expectParseError(t, "switch (1) { case 1: continue }", "<stdin>: ERROR: Cannot use \"continue\" here:\n")
	expectParseError(t, "x: switch (1) { case 1: break x }", "")
	expectParseError(t, "x: switch (1) { case 1: continue x }", "<stdin>: ERROR: Cannot continue to label \"x\"\n")
}

func TestFor(t *testing.T) {
	expectParseError(t, "for (; in x) ;", "<stdin>: ERROR: Unexpected \"in\"\n")
	expectParseError(t, "for (; of x) ;", "<stdin>: ERROR: Expected \";\" but found \"x\"\n")
	expectParseError(t, "for (; in; ) ;", "<stdin>: ERROR: Unexpected \"in\"\n")
	expectPrinted(t, "for (; of; ) ;", "for (; of; )\n  ;\n")

	expectPrinted(t, "for (a in b) ;", "for (a in b)\n  ;\n")
	expectPrinted(t, "for (var a in b) ;", "for (var a in b)\n  ;\n")
	expectPrinted(t, "for (let a in b) ;", "for (let a in b)\n  ;\n")
	expectPrinted(t, "for (const a in b) ;", "for (const a in b)\n  ;\n")
	expectPrinted(t, "for (a in b, c) ;", "for (a in b, c)\n  ;\n")
	expectPrinted(t, "for (a in b = c) ;", "for (a in b = c)\n  ;\n")
	expectPrinted(t, "for (var a in b, c) ;", "for (var a in b, c)\n  ;\n")
	expectPrinted(t, "for (var a in b = c) ;", "for (var a in b = c)\n  ;\n")
	expectParseError(t, "for (var a, b in b) ;", "<stdin>: ERROR: for-in loops must have a single declaration\n")
	expectParseError(t, "for (let a, b in b) ;", "<stdin>: ERROR: for-in loops must have a single declaration\n")
	expectParseError(t, "for (const a, b in b) ;", "<stdin>: ERROR: for-in loops must have a single declaration\n")

	expectPrinted(t, "for (a of b) ;", "for (a of b)\n  ;\n")
	expectPrinted(t, "for (var a of b) ;", "for (var a of b)\n  ;\n")
	expectPrinted(t, "for (let a of b) ;", "for (let a of b)\n  ;\n")
	expectPrinted(t, "for (const a of b) ;", "for (const a of b)\n  ;\n")
	expectPrinted(t, "for (a of b = c) ;", "for (a of b = c)\n  ;\n")
	expectPrinted(t, "for (var a of b = c) ;", "for (var a of b = c)\n  ;\n")
	expectParseError(t, "for (a of b, c) ;", "<stdin>: ERROR: Expected \")\" but found \",\"\n")
	expectParseError(t, "for (var a of b, c) ;", "<stdin>: ERROR: Expected \")\" but found \",\"\n")
	expectParseError(t, "for (var a, b of b) ;", "<stdin>: ERROR: for-of loops must have a single declaration\n")
	expectParseError(t, "for (let a, b of b) ;", "<stdin>: ERROR: for-of loops must have a single declaration\n")
	expectParseError(t, "for (const a, b of b) ;", "<stdin>: ERROR: for-of loops must have a single declaration\n")

	// Avoid the initializer starting with "let" token
	expectPrinted(t, "for ((let) of bar);", "for ((let) of bar)\n  ;\n")
	expectPrinted(t, "for ((let).foo of bar);", "for ((let).foo of bar)\n  ;\n")
	expectPrinted(t, "for ((let.foo) of bar);", "for ((let).foo of bar)\n  ;\n")
	expectPrinted(t, "for ((let``.foo) of bar);", "for ((let)``.foo of bar)\n  ;\n")
	expectParseError(t, "for (let.foo of bar);", "<stdin>: ERROR: \"let\" must be wrapped in parentheses to be used as an expression here:\n")
	expectParseError(t, "for (let().foo of bar);", "<stdin>: ERROR: \"let\" must be wrapped in parentheses to be used as an expression here:\n")
	expectParseError(t, "for (let``.foo of bar);", "<stdin>: ERROR: \"let\" must be wrapped in parentheses to be used as an expression here:\n")

	expectPrinted(t, "for (var x = 0 in y) ;", "x = 0;\nfor (var x in y)\n  ;\n") // This is a weird special-case
	expectParseError(t, "for (let x = 0 in y) ;", "<stdin>: ERROR: for-in loop variables cannot have an initializer\n")
	expectParseError(t, "for (const x = 0 in y) ;", "<stdin>: ERROR: for-in loop variables cannot have an initializer\n")
	expectParseError(t, "for (var x = 0 of y) ;", "<stdin>: ERROR: for-of loop variables cannot have an initializer\n")
	expectParseError(t, "for (let x = 0 of y) ;", "<stdin>: ERROR: for-of loop variables cannot have an initializer\n")
	expectParseError(t, "for (const x = 0 of y) ;", "<stdin>: ERROR: for-of loop variables cannot have an initializer\n")

	expectParseError(t, "for (var [x] = y in z) ;", "<stdin>: ERROR: for-in loop variables cannot have an initializer\n")
	expectParseError(t, "for (let [x] = y in z) ;", "<stdin>: ERROR: for-in loop variables cannot have an initializer\n")
	expectParseError(t, "for (const [x] = y in z) ;", "<stdin>: ERROR: for-in loop variables cannot have an initializer\n")
	expectParseError(t, "for (var [x] = y of z) ;", "<stdin>: ERROR: for-of loop variables cannot have an initializer\n")
	expectParseError(t, "for (let [x] = y of z) ;", "<stdin>: ERROR: for-of loop variables cannot have an initializer\n")
	expectParseError(t, "for (const [x] = y of z) ;", "<stdin>: ERROR: for-of loop variables cannot have an initializer\n")

	expectParseError(t, "for (var {x} = y in z) ;", "<stdin>: ERROR: for-in loop variables cannot have an initializer\n")
	expectParseError(t, "for (let {x} = y in z) ;", "<stdin>: ERROR: for-in loop variables cannot have an initializer\n")
	expectParseError(t, "for (const {x} = y in z) ;", "<stdin>: ERROR: for-in loop variables cannot have an initializer\n")
	expectParseError(t, "for (var {x} = y of z) ;", "<stdin>: ERROR: for-of loop variables cannot have an initializer\n")
	expectParseError(t, "for (let {x} = y of z) ;", "<stdin>: ERROR: for-of loop variables cannot have an initializer\n")
	expectParseError(t, "for (const {x} = y of z) ;", "<stdin>: ERROR: for-of loop variables cannot have an initializer\n")

	// Make sure "in" rules are enabled
	expectPrinted(t, "for (var x = () => a in b);", "x = () => a;\nfor (var x in b)\n  ;\n")
	expectPrinted(t, "for (var x = a + b in c);", "x = a + b;\nfor (var x in c)\n  ;\n")

	// Make sure "in" rules are disabled
	expectPrinted(t, "for (var x = `${y in z}`;;);", "for (var x = `${y in z}`; ; )\n  ;\n")
	expectPrinted(t, "for (var {[x in y]: z} = {};;);", "for (var { [x in y]: z } = {}; ; )\n  ;\n")
	expectPrinted(t, "for (var {x = y in z} = {};;);", "for (var { x = y in z } = {}; ; )\n  ;\n")
	expectPrinted(t, "for (var [x = y in z] = {};;);", "for (var [x = y in z] = {}; ; )\n  ;\n")
	expectPrinted(t, "for (var {x: y = z in w} = {};;);", "for (var { x: y = z in w } = {}; ; )\n  ;\n")
	expectPrinted(t, "for (var x = (a in b);;);", "for (var x = (a in b); ; )\n  ;\n")
	expectPrinted(t, "for (var x = [a in b];;);", "for (var x = [a in b]; ; )\n  ;\n")
	expectPrinted(t, "for (var x = y(a in b);;);", "for (var x = y(a in b); ; )\n  ;\n")
	expectPrinted(t, "for (var x = {y: a in b};;);", "for (var x = { y: a in b }; ; )\n  ;\n")
	expectPrinted(t, "for (a ? b in c : d;;);", "for (a ? b in c : d; ; )\n  ;\n")
	expectPrinted(t, "for (var x = () => { a in b };;);", "for (var x = () => {\n  a in b;\n}; ; )\n  ;\n")
	expectPrinted(t, "for (var x = async () => { a in b };;);", "for (var x = async () => {\n  a in b;\n}; ; )\n  ;\n")
	expectPrinted(t, "for (var x = function() { a in b };;);", "for (var x = function() {\n  a in b;\n}; ; )\n  ;\n")
	expectPrinted(t, "for (var x = async function() { a in b };;);", "for (var x = async function() {\n  a in b;\n}; ; )\n  ;\n")
	expectPrinted(t, "for (var x = class { [a in b]() {} };;);", "for (var x = class {\n  [a in b]() {\n  }\n}; ; )\n  ;\n")
	expectParseError(t, "for (var x = class extends a in b {};;);", "<stdin>: ERROR: Expected \"{\" but found \"in\"\n")

	errorText := `<stdin>: WARNING: This assignment will throw because "x" is a constant
<stdin>: NOTE: The symbol "x" was declared a constant here:
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

	expectPrinted(t, "async of => {}", "async (of) => {\n};\n")
	expectPrinted(t, "for ((async) of []) ;", "for ((async) of [])\n  ;\n")
	expectPrinted(t, "for (async.x of []) ;", "for (async.x of [])\n  ;\n")
	expectPrinted(t, "for (async of => {};;) ;", "for (async (of) => {\n}; ; )\n  ;\n")
	expectPrinted(t, "for (\\u0061sync of []) ;", "for ((async) of [])\n  ;\n")
	expectPrinted(t, "for await (async of []) ;", "for await (async of [])\n  ;\n")
	expectParseError(t, "for (async of []) ;", "<stdin>: ERROR: For loop initializers cannot start with \"async of\"\n")
	expectParseError(t, "for (async o\\u0066 []) ;", "<stdin>: ERROR: Expected \";\" but found \"o\\\\u0066\"\n")
	expectParseError(t, "for await (async of => {}) ;", "<stdin>: ERROR: Expected \"of\" but found \")\"\n")
	expectParseError(t, "for await (async of => {} of []) ;", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "for await (async o\\u0066 []) ;", "<stdin>: ERROR: Expected \"of\" but found \"o\\\\u0066\"\n")

	// Can't use await at the top-level without top-level await
	err := "<stdin>: ERROR: Top-level await is not available in the configured target environment\n"
	expectParseErrorWithUnsupportedFeatures(t, compat.TopLevelAwait, "for await (x of y);", err)
	expectParseErrorWithUnsupportedFeatures(t, compat.TopLevelAwait, "if (true) for await (x of y);", err)
	expectPrintedWithUnsupportedFeatures(t, compat.TopLevelAwait, "if (false) for await (x of y);", "if (false)\n  for (x of y)\n    ;\n")
	expectParseErrorWithUnsupportedFeatures(t, compat.TopLevelAwait, "with (x) y; if (false) for await (x of y);",
		"<stdin>: ERROR: With statements cannot be used in an ECMAScript module\n"+
			"<stdin>: NOTE: This file is considered to be an ECMAScript module because of the top-level \"await\" keyword here:\n")
}

func TestScope(t *testing.T) {
	errorText := `<stdin>: ERROR: The symbol "x" has already been declared
<stdin>: NOTE: The symbol "x" was originally declared here:
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

	expectParseError(t, "function x() {} {var x}", "")
	expectParseError(t, "function *x() {} {var x}", "")
	expectParseError(t, "async function x() {} {var x}", "")
	expectParseError(t, "async function *x() {} {var x}", "")

	expectParseError(t, "{var x} function x() {}", "")
	expectParseError(t, "{var x} function *x() {}", "")
	expectParseError(t, "{var x} async function x() {}", "")
	expectParseError(t, "{var x} async function *x() {}", "")

	expectParseError(t, "{ function x() {} {var x} }", errorText)
	expectParseError(t, "{ function *x() {} {var x} }", errorText)
	expectParseError(t, "{ async function x() {} {var x} }", errorText)
	expectParseError(t, "{ async function *x() {} {var x} }", errorText)

	expectParseError(t, "{ {var x} function x() {} }", errorText)
	expectParseError(t, "{ {var x} function *x() {} }", errorText)
	expectParseError(t, "{ {var x} async function x() {} }", errorText)
	expectParseError(t, "{ {var x} async function *x() {} }", errorText)

	expectParseError(t, "function f() { function x() {} {var x} }", "")
	expectParseError(t, "function f() { function *x() {} {var x} }", "")
	expectParseError(t, "function f() { async function x() {} {var x} }", "")
	expectParseError(t, "function f() { async function *x() {} {var x} }", "")

	expectParseError(t, "function f() { {var x} function x() {} }", "")
	expectParseError(t, "function f() { {var x} function *x() {} }", "")
	expectParseError(t, "function f() { {var x} async function x() {} }", "")
	expectParseError(t, "function f() { {var x} async function *x() {} }", "")

	expectParseError(t, "function f() { { function x() {} {var x} } }", errorText)
	expectParseError(t, "function f() { { function *x() {} {var x} } }", errorText)
	expectParseError(t, "function f() { { async function x() {} {var x} } }", errorText)
	expectParseError(t, "function f() { { async function *x() {} {var x} } }", errorText)

	expectParseError(t, "function f() { { {var x} function x() {} } }", errorText)
	expectParseError(t, "function f() { { {var x} function *x() {} } }", errorText)
	expectParseError(t, "function f() { { {var x} async function x() {} } }", errorText)
	expectParseError(t, "function f() { { {var x} async function *x() {} } }", errorText)

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

	expectParseError(t, "function x() {} function x() {}", "")
	expectParseError(t, "function x() {} function *x() {}", "")
	expectParseError(t, "function x() {} async function x() {}", "")
	expectParseError(t, "function *x() {} function x() {}", "")
	expectParseError(t, "function *x() {} function *x() {}", "")
	expectParseError(t, "async function x() {} function x() {}", "")
	expectParseError(t, "async function x() {} async function x() {}", "")

	expectParseError(t, "function f() { function x() {} function x() {} }", "")
	expectParseError(t, "function f() { function x() {} function *x() {} }", "")
	expectParseError(t, "function f() { function x() {} async function x() {} }", "")
	expectParseError(t, "function f() { function *x() {} function x() {} }", "")
	expectParseError(t, "function f() { function *x() {} function *x() {} }", "")
	expectParseError(t, "function f() { async function x() {} function x() {} }", "")
	expectParseError(t, "function f() { async function x() {} async function x() {} }", "")

	text := "<stdin>: ERROR: The symbol \"x\" has already been declared\n<stdin>: NOTE: The symbol \"x\" was originally declared here:\n"
	for _, scope := range []string{"", "with (x)", "while (x)", "if (x)"} {
		expectParseError(t, scope+"{ function x() {} function x() {} }", "")
		expectParseError(t, scope+"{ function x() {} function *x() {} }", text)
		expectParseError(t, scope+"{ function x() {} async function x() {} }", text)
		expectParseError(t, scope+"{ function *x() {} function x() {} }", text)
		expectParseError(t, scope+"{ function *x() {} function *x() {} }", text)
		expectParseError(t, scope+"{ async function x() {} function x() {} }", text)
		expectParseError(t, scope+"{ async function x() {} async function x() {} }", text)
	}
}

func TestASI(t *testing.T) {
	expectParseError(t, "throw\n0", "<stdin>: ERROR: Unexpected newline after \"throw\"\n")
	expectParseError(t, "return\n0", "<stdin>: WARNING: The following expression is not returned because of an automatically-inserted semicolon\n")
	expectPrinted(t, "return\n0", "return;\n0;\n")
	expectPrinted(t, "0\n[1]", "0[1];\n")
	expectPrinted(t, "0\n(1)", "0(1);\n")
	expectPrinted(t, "new x\n(1)", "new x(1);\n")
	expectPrinted(t, "while (true) break\nx", "while (true)\n  break;\nx;\n")
	expectPrinted(t, "x\n!y", "x;\n!y;\n")
	expectPrinted(t, "x\n++y", "x;\n++y;\n")
	expectPrinted(t, "x\n--y", "x;\n--y;\n")

	expectPrinted(t, "function* foo(){yield\na}", "function* foo() {\n  yield;\n  a;\n}\n")
	expectParseError(t, "function* foo(){yield\n*a}", "<stdin>: ERROR: Unexpected \"*\"\n")
	expectPrinted(t, "function* foo(){yield*\na}", "function* foo() {\n  yield* a;\n}\n")

	expectPrinted(t, "async\nx => {}", "async;\n(x) => {\n};\n")
	expectPrinted(t, "async\nfunction foo() {}", "async;\nfunction foo() {\n}\n")
	expectPrinted(t, "export default async\nx => {}", "export default async;\n(x) => {\n};\n")
	expectPrinted(t, "export default async\nfunction foo() {}", "export default async;\nfunction foo() {\n}\n")
	expectParseError(t, "async\n() => {}", "<stdin>: ERROR: Expected \";\" but found \"=>\"\n")
	expectParseError(t, "export async\nfunction foo() {}", "<stdin>: ERROR: Unexpected newline after \"async\"\n")
	expectParseError(t, "export default async\n() => {}", "<stdin>: ERROR: Expected \";\" but found \"=>\"\n")
	expectParseError(t, "(async\nx => {})", "<stdin>: ERROR: Expected \")\" but found \"x\"\n")
	expectParseError(t, "(async\n() => {})", "<stdin>: ERROR: Expected \")\" but found \"=>\"\n")
	expectParseError(t, "(async\nfunction foo() {})", "<stdin>: ERROR: Expected \")\" but found \"function\"\n")

	expectPrinted(t, "if (0) let\nx = 0", "if (0)\n  let;\nx = 0;\n")
	expectPrinted(t, "if (0) let\n{x}", "if (0)\n  let;\n{\n  x;\n}\n")
	expectParseError(t, "if (0) let\n{x} = 0", "<stdin>: ERROR: Unexpected \"=\"\n")
	expectParseError(t, "if (0) let\n[x] = 0", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectPrinted(t, "function *foo() { if (0) let\nyield 0 }", "function* foo() {\n  if (0)\n    let;\n  yield 0;\n}\n")
	expectPrinted(t, "async function foo() { if (0) let\nawait 0 }", "async function foo() {\n  if (0)\n    let;\n  await 0;\n}\n")

	expectPrinted(t, "let\nx = 0", "let x = 0;\n")
	expectPrinted(t, "let\n{x} = 0", "let { x } = 0;\n")
	expectPrinted(t, "let\n[x] = 0", "let [x] = 0;\n")
	expectParseError(t, "function *foo() { let\nyield 0 }",
		"<stdin>: ERROR: Cannot use \"yield\" as an identifier here:\n<stdin>: ERROR: Expected \";\" but found \"0\"\n")
	expectParseError(t, "async function foo() { let\nawait 0 }",
		"<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n<stdin>: ERROR: Expected \";\" but found \"0\"\n")

	// This is a weird corner case where ASI applies without a newline
	expectPrinted(t, "do x;while(y)z", "do\n  x;\nwhile (y);\nz;\n")
	expectPrinted(t, "do x;while(y);z", "do\n  x;\nwhile (y);\nz;\n")
	expectPrinted(t, "{do x;while(y)}", "{\n  do\n    x;\n  while (y);\n}\n")
}

func TestLocal(t *testing.T) {
	expectPrinted(t, "var let = 0", "var let = 0;\n")
	expectParseError(t, "let let = 0", "<stdin>: ERROR: Cannot use \"let\" as an identifier here:\n")
	expectParseError(t, "const let = 0", "<stdin>: ERROR: Cannot use \"let\" as an identifier here:\n")

	expectPrinted(t, "var\nlet = 0", "var let = 0;\n")
	expectParseError(t, "let\nlet = 0", "<stdin>: ERROR: Cannot use \"let\" as an identifier here:\n")
	expectParseError(t, "const\nlet = 0", "<stdin>: ERROR: Cannot use \"let\" as an identifier here:\n")

	expectPrinted(t, "for (var let in x) ;", "for (var let in x)\n  ;\n")
	expectParseError(t, "for (let let in x) ;", "<stdin>: ERROR: Cannot use \"let\" as an identifier here:\n")
	expectParseError(t, "for (const let in x) ;", "<stdin>: ERROR: Cannot use \"let\" as an identifier here:\n")

	expectPrinted(t, "for (var let of x) ;", "for (var let of x)\n  ;\n")
	expectParseError(t, "for (let let of x) ;", "<stdin>: ERROR: Cannot use \"let\" as an identifier here:\n")
	expectParseError(t, "for (const let of x) ;", "<stdin>: ERROR: Cannot use \"let\" as an identifier here:\n")

	errorText := `<stdin>: WARNING: This assignment will throw because "x" is a constant
<stdin>: NOTE: The symbol "x" was declared a constant here:
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
	expectPrinted(t, "let {if: x} = y", "let { if: x } = y;\n")
	expectParseError(t, "let {x: if} = y", "<stdin>: ERROR: Expected identifier but found \"if\"\n")

	expectPrinted(t, "let {1_2_3n: x} = y", "let { 123n: x } = y;\n")
	expectPrinted(t, "let {0x1_2_3n: x} = y", "let { 0x123n: x } = y;\n")

	expectParseError(t, "var [ (x) ] = 0", "<stdin>: ERROR: Expected identifier but found \"(\"\n")
	expectParseError(t, "var [ ...(x) ] = 0", "<stdin>: ERROR: Expected identifier but found \"(\"\n")
	expectParseError(t, "var { (x) } = 0", "<stdin>: ERROR: Expected identifier but found \"(\"\n")
	expectParseError(t, "var { x: (y) } = 0", "<stdin>: ERROR: Expected identifier but found \"(\"\n")
	expectParseError(t, "var { ...(x) } = 0", "<stdin>: ERROR: Expected identifier but found \"(\"\n")
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

	expectParseError(t, "[ (y) ] = 0", "")
	expectParseError(t, "[ ...(y) ] = 0", "")
	expectParseError(t, "({ (y) } = 0)", "<stdin>: ERROR: Expected identifier but found \"(\"\n")
	expectParseError(t, "({ y: (z) } = 0)", "")
	expectParseError(t, "({ ...(y) } = 0)", "")

	expectParseError(t, "[...x = y] = 0", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "x() = 0", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "x?.y = 0", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "x?.[y] = 0", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "({x: 0} = 0)", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "({x() {}} = 0)", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "({x: 0 = y} = 0)", "<stdin>: ERROR: Invalid assignment target\n")
}

func TestObject(t *testing.T) {
	expectPrinted(t, "({foo})", "({ foo });\n")
	expectPrinted(t, "({foo:0})", "({ foo: 0 });\n")
	expectPrinted(t, "({1e9:0})", "({ 1e9: 0 });\n")
	expectPrinted(t, "({1_2_3n:0})", "({ 123n: 0 });\n")
	expectPrinted(t, "({0x1_2_3n:0})", "({ 0x123n: 0 });\n")
	expectPrinted(t, "({foo() {}})", "({ foo() {\n} });\n")
	expectPrinted(t, "({*foo() {}})", "({ *foo() {\n} });\n")
	expectPrinted(t, "({get foo() {}})", "({ get foo() {\n} });\n")
	expectPrinted(t, "({set foo(x) {}})", "({ set foo(x) {\n} });\n")

	expectPrinted(t, "({if:0})", "({ if: 0 });\n")
	expectPrinted(t, "({if() {}})", "({ if() {\n} });\n")
	expectPrinted(t, "({*if() {}})", "({ *if() {\n} });\n")
	expectPrinted(t, "({get if() {}})", "({ get if() {\n} });\n")
	expectPrinted(t, "({set if(x) {}})", "({ set if(x) {\n} });\n")

	expectParseError(t, "({static foo() {}})", "<stdin>: ERROR: Expected \"}\" but found \"foo\"\n")
	expectParseError(t, "({`a`})", "<stdin>: ERROR: Expected identifier but found \"`a`\"\n")
	expectParseError(t, "({if})", "<stdin>: ERROR: Expected \":\" but found \"}\"\n")

	protoError := "<stdin>: ERROR: Cannot specify the \"__proto__\" property more than once per object\n" +
		"<stdin>: NOTE: The earlier \"__proto__\" property is here:\n"

	expectParseError(t, "({__proto__: 1, __proto__: 2})", protoError)
	expectParseError(t, "({__proto__: 1, '__proto__': 2})", protoError)
	expectParseError(t, "({__proto__: 1, __proto__() {}})", "")
	expectParseError(t, "({__proto__: 1, get __proto__() {}})", "")
	expectParseError(t, "({__proto__: 1, set __proto__(x) {}})", "")
	expectParseError(t, "({__proto__: 1, ['__proto__']: 2})", "")
	expectParseError(t, "({__proto__, __proto__: 2})", "")
	expectParseError(t, "({__proto__: x, __proto__: y} = z)", "")

	expectPrintedMangle(t, "x = {['_proto_']: x}", "x = { _proto_: x };\n")
	expectPrintedMangle(t, "x = {['__proto__']: x}", "x = { [\"__proto__\"]: x };\n")

	expectParseError(t, "({set foo() {}})", "<stdin>: ERROR: Setter \"foo\" must have exactly one argument\n")
	expectParseError(t, "({get foo(x) {}})", "<stdin>: ERROR: Getter \"foo\" must have zero arguments\n")
	expectParseError(t, "({set foo(x, y) {}})", "<stdin>: ERROR: Setter \"foo\" must have exactly one argument\n")

	expectParseError(t, "(class {set #foo() {}})", "<stdin>: ERROR: Setter \"#foo\" must have exactly one argument\n")
	expectParseError(t, "(class {get #foo(x) {}})", "<stdin>: ERROR: Getter \"#foo\" must have zero arguments\n")
	expectParseError(t, "(class {set #foo(x, y) {}})", "<stdin>: ERROR: Setter \"#foo\" must have exactly one argument\n")

	expectParseError(t, "({set [foo]() {}})", "<stdin>: ERROR: Setter property must have exactly one argument\n")
	expectParseError(t, "({get [foo](x) {}})", "<stdin>: ERROR: Getter property must have zero arguments\n")
	expectParseError(t, "({set [foo](x, y) {}})", "<stdin>: ERROR: Setter property must have exactly one argument\n")

	duplicateWarning := "<stdin>: WARNING: Duplicate key \"x\" in object literal\n" +
		"<stdin>: NOTE: The original key \"x\" is here:\n"
	expectParseError(t, "({x, x})", duplicateWarning)
	expectParseError(t, "({x() {}, x() {}})", duplicateWarning)
	expectParseError(t, "({get x() {}, get x() {}})", duplicateWarning)
	expectParseError(t, "({get x() {}, set x(y) {}, get x() {}})", duplicateWarning)
	expectParseError(t, "({get x() {}, set x(y) {}, set x(y) {}})", duplicateWarning)
	expectParseError(t, "({get x() {}, set x(y) {}})", "")
	expectParseError(t, "({set x(y) {}, get x() {}})", "")

	// Check the string-to-int optimization
	expectPrintedMangle(t, "x = { '0': y }", "x = { 0: y };\n")
	expectPrintedMangle(t, "x = { '123': y }", "x = { 123: y };\n")
	expectPrintedMangle(t, "x = { '-123': y }", "x = { \"-123\": y };\n")
	expectPrintedMangle(t, "x = { '-0': y }", "x = { \"-0\": y };\n")
	expectPrintedMangle(t, "x = { '01': y }", "x = { \"01\": y };\n")
	expectPrintedMangle(t, "x = { '-01': y }", "x = { \"-01\": y };\n")
	expectPrintedMangle(t, "x = { '0x1': y }", "x = { \"0x1\": y };\n")
	expectPrintedMangle(t, "x = { '-0x1': y }", "x = { \"-0x1\": y };\n")
	expectPrintedMangle(t, "x = { '2147483647': y }", "x = { 2147483647: y };\n")
	expectPrintedMangle(t, "x = { '2147483648': y }", "x = { \"2147483648\": y };\n")
	expectPrintedMangle(t, "x = { '-2147483648': y }", "x = { \"-2147483648\": y };\n")
	expectPrintedMangle(t, "x = { '-2147483649': y }", "x = { \"-2147483649\": y };\n")
}

func TestComputedProperty(t *testing.T) {
	expectPrinted(t, "({[a]: foo})", "({ [a]: foo });\n")
	expectPrinted(t, "({[(a, b)]: foo})", "({ [(a, b)]: foo });\n")
	expectParseError(t, "({[a, b]: foo})", "<stdin>: ERROR: Expected \"]\" but found \",\"\n")

	expectPrinted(t, "({[a]: foo}) => {}", "({ [a]: foo }) => {\n};\n")
	expectPrinted(t, "({[(a, b)]: foo}) => {}", "({ [(a, b)]: foo }) => {\n};\n")
	expectParseError(t, "({[a, b]: foo}) => {}", "<stdin>: ERROR: Expected \"]\" but found \",\"\n")

	expectPrinted(t, "var {[a]: foo} = bar", "var { [a]: foo } = bar;\n")
	expectPrinted(t, "var {[(a, b)]: foo} = bar", "var { [(a, b)]: foo } = bar;\n")
	expectParseError(t, "var {[a, b]: foo} = bar", "<stdin>: ERROR: Expected \"]\" but found \",\"\n")

	expectPrinted(t, "class Foo {[a] = foo}", "class Foo {\n  [a] = foo;\n}\n")
	expectPrinted(t, "class Foo {[(a, b)] = foo}", "class Foo {\n  [(a, b)] = foo;\n}\n")
	expectParseError(t, "class Foo {[a, b] = foo}", "<stdin>: ERROR: Expected \"]\" but found \",\"\n")
}

func TestQuotedProperty(t *testing.T) {
	expectPrinted(t, "x.x; y['y']", "x.x;\ny[\"y\"];\n")
	expectPrinted(t, "({y: y, 'z': z} = x)", "({ y, \"z\": z } = x);\n")
	expectPrinted(t, "var {y: y, 'z': z} = x", "var { y, \"z\": z } = x;\n")
	expectPrinted(t, "x = {y: 1, 'z': 2}", "x = { y: 1, \"z\": 2 };\n")
	expectPrinted(t, "x = {y() {}, 'z'() {}}", "x = { y() {\n}, \"z\"() {\n} };\n")
	expectPrinted(t, "x = {get y() {}, set 'z'(z) {}}", "x = { get y() {\n}, set \"z\"(z) {\n} };\n")
	expectPrinted(t, "x = class {y = 1; 'z' = 2}", "x = class {\n  y = 1;\n  \"z\" = 2;\n};\n")
	expectPrinted(t, "x = class {y() {}; 'z'() {}}", "x = class {\n  y() {\n  }\n  \"z\"() {\n  }\n};\n")
	expectPrinted(t, "x = class {get y() {}; set 'z'(z) {}}", "x = class {\n  get y() {\n  }\n  set \"z\"(z) {\n  }\n};\n")

	expectPrintedMangle(t, "x.x; y['y']", "x.x, y.y;\n")
	expectPrintedMangle(t, "({y: y, 'z': z} = x)", "({ y, z } = x);\n")
	expectPrintedMangle(t, "var {y: y, 'z': z} = x", "var { y, z } = x;\n")
	expectPrintedMangle(t, "x = {y: 1, 'z': 2}", "x = { y: 1, z: 2 };\n")
	expectPrintedMangle(t, "x = {y() {}, 'z'() {}}", "x = { y() {\n}, z() {\n} };\n")
	expectPrintedMangle(t, "x = {get y() {}, set 'z'(z) {}}", "x = { get y() {\n}, set z(z) {\n} };\n")
	expectPrintedMangle(t, "x = class {y = 1; 'z' = 2}", "x = class {\n  y = 1;\n  z = 2;\n};\n")
	expectPrintedMangle(t, "x = class {y() {}; 'z'() {}}", "x = class {\n  y() {\n  }\n  z() {\n  }\n};\n")
	expectPrintedMangle(t, "x = class {get y() {}; set 'z'(z) {}}", "x = class {\n  get y() {\n  }\n  set z(z) {\n  }\n};\n")
}

func TestLexicalDecl(t *testing.T) {
	expectPrinted(t, "if (1) var x", "if (1)\n  var x;\n")
	expectPrinted(t, "if (1) function x() {}", "if (1) {\n  let x = function() {\n  };\n  var x = x;\n}\n")
	expectPrinted(t, "if (1) {} else function x() {}", "if (1) {\n} else {\n  let x = function() {\n  };\n  var x = x;\n}\n")
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
		expectParseError(t, fmt.Sprintf(context, "const x = 0"), "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
		expectParseError(t, fmt.Sprintf(context, "let x"), "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
		expectParseError(t, fmt.Sprintf(context, "class X {}"), "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
		expectParseError(t, fmt.Sprintf(context, "function* x() {}"), "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
		expectParseError(t, fmt.Sprintf(context, "async function x() {}"), "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
		expectParseError(t, fmt.Sprintf(context, "async function* x() {}"), "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	}

	expectPrinted(t, "function f() {}", "function f() {\n}\n")
	expectPrinted(t, "{function f() {}} let f", "{\n  let f = function() {\n  };\n}\nlet f;\n")
	expectPrinted(t, "if (1) function f() {} let f", "if (1) {\n  let f = function() {\n  };\n}\nlet f;\n")
	expectPrinted(t, "if (0) ; else function f() {} let f", "if (0)\n  ;\nelse {\n  let f = function() {\n  };\n}\nlet f;\n")
	expectPrinted(t, "x: function f() {}", "x: {\n  let f = function() {\n  };\n  var f = f;\n}\n")
	expectPrinted(t, "{function* f() {}} let f", "{\n  function* f() {\n  }\n}\nlet f;\n")
	expectPrinted(t, "{async function f() {}} let f", "{\n  async function f() {\n  }\n}\nlet f;\n")

	expectParseError(t, "if (1) label: function f() {} let f", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "if (1) label: label2: function f() {} let f", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "if (0) ; else label: function f() {} let f", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "if (0) ; else label: label2: function f() {} let f", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")

	expectParseError(t, "for (;;) function f() {}", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "for (x in y) function f() {}", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "for (x of y) function f() {}", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "for await (x of y) function f() {}", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "with (1) function f() {}", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "while (1) function f() {}", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "do function f() {} while (0)", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")

	fnLabelAwait := "<stdin>: ERROR: Function declarations inside labels cannot be used in an ECMAScript module\n" +
		"<stdin>: NOTE: This file is considered to be an ECMAScript module because of the top-level \"await\" keyword here:\n"

	expectParseError(t, "for (;;) label: function f() {}", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "for (x in y) label: function f() {}", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "for (x of y) label: function f() {}", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "for await (x of y) label: function f() {}", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n"+fnLabelAwait)
	expectParseError(t, "with (1) label: function f() {}", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "while (1) label: function f() {}", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "do label: function f() {} while (0)", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")

	expectParseError(t, "for (;;) label: label2: function f() {}", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "for (x in y) label: label2: function f() {}", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "for (x of y) label: label2: function f() {}", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "for await (x of y) label: label2: function f() {}", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n"+fnLabelAwait)
	expectParseError(t, "with (1) label: label2: function f() {}", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "while (1) label: label2: function f() {}", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")
	expectParseError(t, "do label: label2: function f() {} while (0)", "<stdin>: ERROR: Cannot use a declaration in a single-statement context\n")

	// Test direct "eval"
	expectPrinted(t, "if (foo) { function x() {} }", "if (foo) {\n  let x = function() {\n  };\n  var x = x;\n}\n")
	expectPrinted(t, "if (foo) { function x() {} eval('') }", "if (foo) {\n  function x() {\n  }\n  eval(\"\");\n}\n")
	expectPrinted(t, "if (foo) { function x() {} if (bar) { eval('') } }", "if (foo) {\n  function x() {\n  }\n  if (bar) {\n    eval(\"\");\n  }\n}\n")
	expectPrinted(t, "if (foo) { eval(''); function x() {} }", "if (foo) {\n  function x() {\n  }\n  eval(\"\");\n}\n")
	expectPrinted(t, "'use strict'; if (foo) { function x() {} }", "\"use strict\";\nif (foo) {\n  let x = function() {\n  };\n}\n")
	expectPrinted(t, "'use strict'; if (foo) { function x() {} eval('') }", "\"use strict\";\nif (foo) {\n  function x() {\n  }\n  eval(\"\");\n}\n")
	expectPrinted(t, "'use strict'; if (foo) { function x() {} if (bar) { eval('') } }", "\"use strict\";\nif (foo) {\n  function x() {\n  }\n  if (bar) {\n    eval(\"\");\n  }\n}\n")
	expectPrinted(t, "'use strict'; if (foo) { eval(''); function x() {} }", "\"use strict\";\nif (foo) {\n  function x() {\n  }\n  eval(\"\");\n}\n")
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

	expectPrinted(t, "(function foo() { var arguments })", "(function foo() {\n  var arguments;\n});\n")
	expectPrinted(t, "(function foo() { { var arguments } })", "(function foo() {\n  {\n    var arguments;\n  }\n});\n")

	expectPrintedMangle(t, "function foo() { return undefined }", "function foo() {\n}\n")
	expectPrintedMangle(t, "function* foo() { return undefined }", "function* foo() {\n}\n")
	expectPrintedMangle(t, "async function foo() { return undefined }", "async function foo() {\n}\n")
	expectPrintedMangle(t, "async function* foo() { return undefined }", "async function* foo() {\n  return void 0;\n}\n")

	// Strip overwritten function declarations
	expectPrintedMangle(t, "function f() { x() } function f() { y() }", "function f() {\n  y();\n}\n")
	expectPrintedMangle(t, "function f() { x() } function *f() { y() }", "function* f() {\n  y();\n}\n")
	expectPrintedMangle(t, "function *f() { x() } function f() { y() }", "function f() {\n  y();\n}\n")
	expectPrintedMangle(t, "function *f() { x() } function *f() { y() }", "function* f() {\n  y();\n}\n")
	expectPrintedMangle(t, "function f() { x() } async function f() { y() }", "async function f() {\n  y();\n}\n")
	expectPrintedMangle(t, "async function f() { x() } function f() { y() }", "function f() {\n  y();\n}\n")
	expectPrintedMangle(t, "async function f() { x() } async function f() { y() }", "async function f() {\n  y();\n}\n")
	expectPrintedMangle(t, "var f; function f() {}", "var f;\nfunction f() {\n}\n")
	expectPrintedMangle(t, "function f() {} var f", "function f() {\n}\nvar f;\n")
	expectPrintedMangle(t, "var f; function f() { x() } function f() { y() }", "var f;\nfunction f() {\n  y();\n}\n")
	expectPrintedMangle(t, "function f() { x() } function f() { y() } var f", "function f() {\n  y();\n}\nvar f;\n")
	expectPrintedMangle(t, "function f() { x() } var f; function f() { y() }", "function f() {\n  x();\n}\nvar f;\nfunction f() {\n  y();\n}\n")

	redeclaredError := "<stdin>: ERROR: The symbol \"f\" has already been declared\n" +
		"<stdin>: NOTE: The symbol \"f\" was originally declared here:\n"

	expectParseError(t, "function *f() {} function *f() {}", "")
	expectParseError(t, "function f() {} let f", redeclaredError)
	expectParseError(t, "function f() {} var f", "")
	expectParseError(t, "function *f() {} var f", "")
	expectParseError(t, "let f; function f() {}", redeclaredError)
	expectParseError(t, "var f; function f() {}", "")
	expectParseError(t, "var f; function *f() {}", "")

	expectParseError(t, "{ function *f() {} function *f() {} }", redeclaredError)
	expectParseError(t, "{ function f() {} let f }", redeclaredError)
	expectParseError(t, "{ function f() {} var f }", redeclaredError)
	expectParseError(t, "{ function *f() {} var f }", redeclaredError)
	expectParseError(t, "{ let f; function f() {} }", redeclaredError)
	expectParseError(t, "{ var f; function f() {} }", redeclaredError)
	expectParseError(t, "{ var f; function *f() {} }", redeclaredError)

	expectParseError(t, "switch (0) { case 1: function *f() {} default: function *f() {} }", redeclaredError)
	expectParseError(t, "switch (0) { case 1: function f() {} default: let f }", redeclaredError)
	expectParseError(t, "switch (0) { case 1: function f() {} default: var f }", redeclaredError)
	expectParseError(t, "switch (0) { case 1: function *f() {} default: var f }", redeclaredError)
	expectParseError(t, "switch (0) { case 1: let f; default: function f() {} }", redeclaredError)
	expectParseError(t, "switch (0) { case 1: var f; default: function f() {} }", redeclaredError)
	expectParseError(t, "switch (0) { case 1: var f; default: function *f() {} }", redeclaredError)
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
	expectParseError(t, "class Foo { async static foo() {} }", "<stdin>: ERROR: Expected \"(\" but found \"foo\"\n")

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
	expectParseError(t, "class Foo { async static if() {} }", "<stdin>: ERROR: Expected \"(\" but found \"if\"\n")

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
	expectParseError(t, "class Foo { a() {} async static b() {} }", "<stdin>: ERROR: Expected \"(\" but found \"b\"\n")

	expectParseError(t, "class Foo { `a`() {} }", "<stdin>: ERROR: Expected identifier but found \"`a`\"\n")

	// Strict mode reserved words cannot be used as class names
	expectParseError(t, "class static {}",
		"<stdin>: ERROR: \"static\" is a reserved word and cannot be used in strict mode\n"+
			"<stdin>: NOTE: All code inside a class is implicitly in strict mode\n")
	expectParseError(t, "(class static {})",
		"<stdin>: ERROR: \"static\" is a reserved word and cannot be used in strict mode\n"+
			"<stdin>: NOTE: All code inside a class is implicitly in strict mode\n")
	expectParseError(t, "class implements {}",
		"<stdin>: ERROR: \"implements\" is a reserved word and cannot be used in strict mode\n"+
			"<stdin>: NOTE: All code inside a class is implicitly in strict mode\n")
	expectParseError(t, "(class implements {})",
		"<stdin>: ERROR: \"implements\" is a reserved word and cannot be used in strict mode\n"+
			"<stdin>: NOTE: All code inside a class is implicitly in strict mode\n")

	// The name "arguments" is forbidden in class bodies outside of computed properties
	expectPrinted(t, "class Foo { [arguments] }", "class Foo {\n  [arguments];\n}\n")
	expectPrinted(t, "class Foo { [arguments] = 1 }", "class Foo {\n  [arguments] = 1;\n}\n")
	expectPrinted(t, "class Foo { arguments = 1 }", "class Foo {\n  arguments = 1;\n}\n")
	expectPrinted(t, "class Foo { x = class { arguments = 1 } }", "class Foo {\n  x = class {\n    arguments = 1;\n  };\n}\n")
	expectPrinted(t, "class Foo { x = function() { arguments } }", "class Foo {\n  x = function() {\n    arguments;\n  };\n}\n")
	expectParseError(t, "class Foo { x = arguments }", "<stdin>: ERROR: Cannot access \"arguments\" here:\n")
	expectParseError(t, "class Foo { x = () => arguments }", "<stdin>: ERROR: Cannot access \"arguments\" here:\n")
	expectParseError(t, "class Foo { x = typeof arguments }", "<stdin>: ERROR: Cannot access \"arguments\" here:\n")
	expectParseError(t, "class Foo { x = 1 ? 2 : arguments }", "<stdin>: ERROR: Cannot access \"arguments\" here:\n")
	expectParseError(t, "class Foo { x = class { [arguments] } }", "<stdin>: ERROR: Cannot access \"arguments\" here:\n")
	expectParseError(t, "class Foo { x = class { [arguments] = 1 } }", "<stdin>: ERROR: Cannot access \"arguments\" here:\n")
	expectParseError(t, "class Foo { static { arguments } }", "<stdin>: ERROR: Cannot access \"arguments\" here:\n")
	expectParseError(t, "class Foo { static { class Bar { [arguments] } } }", "<stdin>: ERROR: Cannot access \"arguments\" here:\n")

	// The name "constructor" is sometimes forbidden
	expectPrinted(t, "class Foo { get ['constructor']() {} }", "class Foo {\n  get [\"constructor\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { set ['constructor'](x) {} }", "class Foo {\n  set [\"constructor\"](x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { *['constructor']() {} }", "class Foo {\n  *[\"constructor\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async ['constructor']() {} }", "class Foo {\n  async [\"constructor\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async *['constructor']() {} }", "class Foo {\n  async *[\"constructor\"]() {\n  }\n}\n")
	expectParseError(t, "class Foo { get constructor() {} }", "<stdin>: ERROR: Class constructor cannot be a getter\n")
	expectParseError(t, "class Foo { get 'constructor'() {} }", "<stdin>: ERROR: Class constructor cannot be a getter\n")
	expectParseError(t, "class Foo { set constructor(x) {} }", "<stdin>: ERROR: Class constructor cannot be a setter\n")
	expectParseError(t, "class Foo { set 'constructor'(x) {} }", "<stdin>: ERROR: Class constructor cannot be a setter\n")
	expectParseError(t, "class Foo { *constructor() {} }", "<stdin>: ERROR: Class constructor cannot be a generator\n")
	expectParseError(t, "class Foo { *'constructor'() {} }", "<stdin>: ERROR: Class constructor cannot be a generator\n")
	expectParseError(t, "class Foo { async constructor() {} }", "<stdin>: ERROR: Class constructor cannot be an async function\n")
	expectParseError(t, "class Foo { async 'constructor'() {} }", "<stdin>: ERROR: Class constructor cannot be an async function\n")
	expectParseError(t, "class Foo { async *constructor() {} }", "<stdin>: ERROR: Class constructor cannot be an async function\n")
	expectParseError(t, "class Foo { async *'constructor'() {} }", "<stdin>: ERROR: Class constructor cannot be an async function\n")
	expectPrinted(t, "class Foo { static get constructor() {} }", "class Foo {\n  static get constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static get 'constructor'() {} }", "class Foo {\n  static get \"constructor\"() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static set constructor(x) {} }", "class Foo {\n  static set constructor(x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { static set 'constructor'(x) {} }", "class Foo {\n  static set \"constructor\"(x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { static *constructor() {} }", "class Foo {\n  static *constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static *'constructor'() {} }", "class Foo {\n  static *\"constructor\"() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async constructor() {} }", "class Foo {\n  static async constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async 'constructor'() {} }", "class Foo {\n  static async \"constructor\"() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async *constructor() {} }", "class Foo {\n  static async *constructor() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async *'constructor'() {} }", "class Foo {\n  static async *\"constructor\"() {\n  }\n}\n")
	expectPrinted(t, "({ constructor: 1 })", "({ constructor: 1 });\n")
	expectPrinted(t, "({ get constructor() {} })", "({ get constructor() {\n} });\n")
	expectPrinted(t, "({ set constructor(x) {} })", "({ set constructor(x) {\n} });\n")
	expectPrinted(t, "({ *constructor() {} })", "({ *constructor() {\n} });\n")
	expectPrinted(t, "({ async constructor() {} })", "({ async constructor() {\n} });\n")
	expectPrinted(t, "({ async* constructor() {} })", "({ async *constructor() {\n} });\n")

	// The name "prototype" is sometimes forbidden
	expectPrinted(t, "class Foo { get prototype() {} }", "class Foo {\n  get prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { get 'prototype'() {} }", "class Foo {\n  get \"prototype\"() {\n  }\n}\n")
	expectPrinted(t, "class Foo { set prototype(x) {} }", "class Foo {\n  set prototype(x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { set 'prototype'(x) {} }", "class Foo {\n  set \"prototype\"(x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { *prototype() {} }", "class Foo {\n  *prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { *'prototype'() {} }", "class Foo {\n  *\"prototype\"() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async prototype() {} }", "class Foo {\n  async prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async 'prototype'() {} }", "class Foo {\n  async \"prototype\"() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async *prototype() {} }", "class Foo {\n  async *prototype() {\n  }\n}\n")
	expectPrinted(t, "class Foo { async *'prototype'() {} }", "class Foo {\n  async *\"prototype\"() {\n  }\n}\n")
	expectParseError(t, "class Foo { static get prototype() {} }", "<stdin>: ERROR: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static get 'prototype'() {} }", "<stdin>: ERROR: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static set prototype(x) {} }", "<stdin>: ERROR: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static set 'prototype'(x) {} }", "<stdin>: ERROR: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static *prototype() {} }", "<stdin>: ERROR: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static *'prototype'() {} }", "<stdin>: ERROR: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static async prototype() {} }", "<stdin>: ERROR: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static async 'prototype'() {} }", "<stdin>: ERROR: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static async *prototype() {} }", "<stdin>: ERROR: Invalid static method name \"prototype\"\n")
	expectParseError(t, "class Foo { static async *'prototype'() {} }", "<stdin>: ERROR: Invalid static method name \"prototype\"\n")
	expectPrinted(t, "class Foo { static get ['prototype']() {} }", "class Foo {\n  static get [\"prototype\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static set ['prototype'](x) {} }", "class Foo {\n  static set [\"prototype\"](x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { static *['prototype']() {} }", "class Foo {\n  static *[\"prototype\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async ['prototype']() {} }", "class Foo {\n  static async [\"prototype\"]() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static async *['prototype']() {} }", "class Foo {\n  static async *[\"prototype\"]() {\n  }\n}\n")
	expectPrinted(t, "({ prototype: 1 })", "({ prototype: 1 });\n")
	expectPrinted(t, "({ get prototype() {} })", "({ get prototype() {\n} });\n")
	expectPrinted(t, "({ set prototype(x) {} })", "({ set prototype(x) {\n} });\n")
	expectPrinted(t, "({ *prototype() {} })", "({ *prototype() {\n} });\n")
	expectPrinted(t, "({ async prototype() {} })", "({ async prototype() {\n} });\n")
	expectPrinted(t, "({ async* prototype() {} })", "({ async *prototype() {\n} });\n")

	expectPrintedMangle(t, "class Foo { ['constructor'] = 0 }", "class Foo {\n  [\"constructor\"] = 0;\n}\n")
	expectPrintedMangle(t, "class Foo { ['constructor']() {} }", "class Foo {\n  [\"constructor\"]() {\n  }\n}\n")
	expectPrintedMangle(t, "class Foo { *['constructor']() {} }", "class Foo {\n  *[\"constructor\"]() {\n  }\n}\n")
	expectPrintedMangle(t, "class Foo { get ['constructor']() {} }", "class Foo {\n  get [\"constructor\"]() {\n  }\n}\n")
	expectPrintedMangle(t, "class Foo { set ['constructor'](x) {} }", "class Foo {\n  set [\"constructor\"](x) {\n  }\n}\n")
	expectPrintedMangle(t, "class Foo { async ['constructor']() {} }", "class Foo {\n  async [\"constructor\"]() {\n  }\n}\n")

	expectPrintedMangle(t, "class Foo { static ['constructor'] = 0 }", "class Foo {\n  static [\"constructor\"] = 0;\n}\n")
	expectPrintedMangle(t, "class Foo { static ['constructor']() {} }", "class Foo {\n  static constructor() {\n  }\n}\n")
	expectPrintedMangle(t, "class Foo { static *['constructor']() {} }", "class Foo {\n  static *constructor() {\n  }\n}\n")
	expectPrintedMangle(t, "class Foo { static get ['constructor']() {} }", "class Foo {\n  static get constructor() {\n  }\n}\n")
	expectPrintedMangle(t, "class Foo { static set ['constructor'](x) {} }", "class Foo {\n  static set constructor(x) {\n  }\n}\n")
	expectPrintedMangle(t, "class Foo { static async ['constructor']() {} }", "class Foo {\n  static async constructor() {\n  }\n}\n")

	expectPrintedMangle(t, "class Foo { ['prototype'] = 0 }", "class Foo {\n  prototype = 0;\n}\n")
	expectPrintedMangle(t, "class Foo { ['prototype']() {} }", "class Foo {\n  prototype() {\n  }\n}\n")
	expectPrintedMangle(t, "class Foo { *['prototype']() {} }", "class Foo {\n  *prototype() {\n  }\n}\n")
	expectPrintedMangle(t, "class Foo { get ['prototype']() {} }", "class Foo {\n  get prototype() {\n  }\n}\n")
	expectPrintedMangle(t, "class Foo { set ['prototype'](x) {} }", "class Foo {\n  set prototype(x) {\n  }\n}\n")
	expectPrintedMangle(t, "class Foo { async ['prototype']() {} }", "class Foo {\n  async prototype() {\n  }\n}\n")

	expectPrintedMangle(t, "class Foo { static ['prototype'] = 0 }", "class Foo {\n  static [\"prototype\"] = 0;\n}\n")
	expectPrintedMangle(t, "class Foo { static ['prototype']() {} }", "class Foo {\n  static [\"prototype\"]() {\n  }\n}\n")
	expectPrintedMangle(t, "class Foo { static *['prototype']() {} }", "class Foo {\n  static *[\"prototype\"]() {\n  }\n}\n")
	expectPrintedMangle(t, "class Foo { static get ['prototype']() {} }", "class Foo {\n  static get [\"prototype\"]() {\n  }\n}\n")
	expectPrintedMangle(t, "class Foo { static set ['prototype'](x) {} }", "class Foo {\n  static set [\"prototype\"](x) {\n  }\n}\n")
	expectPrintedMangle(t, "class Foo { static async ['prototype']() {} }", "class Foo {\n  static async [\"prototype\"]() {\n  }\n}\n")

	dupCtor := "<stdin>: ERROR: Classes cannot contain more than one constructor\n"

	expectParseError(t, "class Foo { constructor() {} constructor() {} }", dupCtor)
	expectParseError(t, "class Foo { constructor() {} 'constructor'() {} }", dupCtor)
	expectParseError(t, "class Foo { constructor() {} ['constructor']() {} }", "")
	expectParseError(t, "class Foo { 'constructor'() {} constructor() {} }", dupCtor)
	expectParseError(t, "class Foo { ['constructor']() {} constructor() {} }", "")
	expectParseError(t, "class Foo { constructor() {} static constructor() {} }", "")
	expectParseError(t, "class Foo { static constructor() {} constructor() {} }", "")
	expectParseError(t, "class Foo { static constructor() {} static constructor() {} }", "")
	expectParseError(t, "class Foo { constructor = () => {}; constructor = () => {} }",
		"<stdin>: ERROR: Invalid field name \"constructor\"\n<stdin>: ERROR: Invalid field name \"constructor\"\n")
	expectParseError(t, "({ constructor() {}, constructor() {} })",
		"<stdin>: WARNING: Duplicate key \"constructor\" in object literal\n<stdin>: NOTE: The original key \"constructor\" is here:\n")
	expectParseError(t, "(class { constructor() {} constructor() {} })", dupCtor)
	expectPrintedMangle(t, "class Foo { constructor() {} ['constructor']() {} }",
		"class Foo {\n  constructor() {\n  }\n  [\"constructor\"]() {\n  }\n}\n")
	expectPrintedMangle(t, "class Foo { static constructor() {} static ['constructor']() {} }",
		"class Foo {\n  static constructor() {\n  }\n  static constructor() {\n  }\n}\n")

	// Check the string-to-int optimization
	expectPrintedMangle(t, "class x { '0' = y }", "class x {\n  0 = y;\n}\n")
	expectPrintedMangle(t, "class x { '123' = y }", "class x {\n  123 = y;\n}\n")
	expectPrintedMangle(t, "class x { ['-123'] = y }", "class x {\n  \"-123\" = y;\n}\n")
	expectPrintedMangle(t, "class x { '-0' = y }", "class x {\n  \"-0\" = y;\n}\n")
	expectPrintedMangle(t, "class x { '01' = y }", "class x {\n  \"01\" = y;\n}\n")
	expectPrintedMangle(t, "class x { '-01' = y }", "class x {\n  \"-01\" = y;\n}\n")
	expectPrintedMangle(t, "class x { '0x1' = y }", "class x {\n  \"0x1\" = y;\n}\n")
	expectPrintedMangle(t, "class x { '-0x1' = y }", "class x {\n  \"-0x1\" = y;\n}\n")
	expectPrintedMangle(t, "class x { '2147483647' = y }", "class x {\n  2147483647 = y;\n}\n")
	expectPrintedMangle(t, "class x { '2147483648' = y }", "class x {\n  \"2147483648\" = y;\n}\n")
	expectPrintedMangle(t, "class x { ['-2147483648'] = y }", "class x {\n  \"-2147483648\" = y;\n}\n")
	expectPrintedMangle(t, "class x { ['-2147483649'] = y }", "class x {\n  \"-2147483649\" = y;\n}\n")
}

func TestSuperCall(t *testing.T) {
	expectParseError(t, "super", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "super()", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "class Foo { foo = super() }", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "class Foo { foo() { super() } }", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "class Foo extends Bar { foo = super() }", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "class Foo extends Bar { foo() { super() } }", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "class Foo extends Bar { static constructor() { super() } }", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "class Foo extends Bar { constructor(x = function() { super() }) {} }", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "class Foo extends Bar { constructor() { function foo() { super() } } }", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "class Foo extends Bar { constructor() { super } }", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectPrinted(t, "class Foo extends Bar { constructor() { super() } }",
		"class Foo extends Bar {\n  constructor() {\n    super();\n  }\n}\n")
	expectPrinted(t, "class Foo extends Bar { constructor() { () => super() } }",
		"class Foo extends Bar {\n  constructor() {\n    () => super();\n  }\n}\n")
	expectPrinted(t, "class Foo extends Bar { constructor() { () => { super() } } }",
		"class Foo extends Bar {\n  constructor() {\n    () => {\n      super();\n    };\n  }\n}\n")
	expectPrinted(t, "class Foo extends Bar { constructor(x = super()) {} }",
		"class Foo extends Bar {\n  constructor(x = super()) {\n  }\n}\n")
	expectPrinted(t, "class Foo extends Bar { constructor(x = () => super()) {} }",
		"class Foo extends Bar {\n  constructor(x = () => super()) {\n  }\n}\n")

	expectPrintedMangleTarget(t, 2015, "class A extends B { x; constructor() { super() } }",
		"class A extends B {\n  constructor() {\n    super();\n    __publicField(this, \"x\");\n  }\n}\n")
	expectPrintedMangleTarget(t, 2015, "class A extends B { x = 1; constructor() { super() } }",
		"class A extends B {\n  constructor() {\n    super();\n    __publicField(this, \"x\", 1);\n  }\n}\n")
	expectPrintedMangleTarget(t, 2015, "class A extends B { x = 1; constructor() { super(); c() } }",
		"class A extends B {\n  constructor() {\n    super();\n    __publicField(this, \"x\", 1);\n    c();\n  }\n}\n")
	expectPrintedMangleTarget(t, 2015, "class A extends B { x = 1; constructor() { c(); super() } }",
		"class A extends B {\n  constructor() {\n    c();\n    super();\n    __publicField(this, \"x\", 1);\n  }\n}\n")
	expectPrintedMangleTarget(t, 2015, "class A extends B { x = 1; constructor() { super(); if (c) throw c } }",
		"class A extends B {\n  constructor() {\n    super();\n    __publicField(this, \"x\", 1);\n    if (c)\n      throw c;\n  }\n}\n")
	expectPrintedMangleTarget(t, 2015, "class A extends B { x = 1; constructor() { super(); switch (c) { case 0: throw c } } }",
		"class A extends B {\n  constructor() {\n    super();\n    __publicField(this, \"x\", 1);\n    switch (c) {\n      case 0:\n        throw c;\n    }\n  }\n}\n")
	expectPrintedMangleTarget(t, 2015, "class A extends B { x = 1; constructor() { super(); while (!c) throw c } }",
		"class A extends B {\n  constructor() {\n    super();\n    __publicField(this, \"x\", 1);\n    for (; !c; )\n      throw c;\n  }\n}\n")
	expectPrintedMangleTarget(t, 2015, "class A extends B { x = 1; constructor() { super(); return c } }",
		"class A extends B {\n  constructor() {\n    super();\n    __publicField(this, \"x\", 1);\n    return c;\n  }\n}\n")
	expectPrintedMangleTarget(t, 2015, "class A extends B { x = 1; constructor() { super(); throw c } }",
		"class A extends B {\n  constructor() {\n    super();\n    __publicField(this, \"x\", 1);\n    throw c;\n  }\n}\n")
	expectPrintedMangleTarget(t, 2015, "class A extends B { x = 1; constructor() { if (true) super(1); else super(2); } }",
		"class A extends B {\n  constructor() {\n    super(1);\n    __publicField(this, \"x\", 1);\n  }\n}\n")
	expectPrintedMangleTarget(t, 2015, "class A extends B { x = 1; constructor() { if (foo) super(1); else super(2); } }",
		"class A extends B {\n  constructor() {\n    var __super = (...args) => {\n      super(...args);\n      __publicField(this, \"x\", 1);\n    };\n    foo ? __super(1) : __super(2);\n  }\n}\n")
}

func TestSuperProp(t *testing.T) {
	expectParseError(t, "super.x", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "super[x]", "<stdin>: ERROR: Unexpected \"super\"\n")

	expectPrinted(t, "class Foo { foo() { super.x } }", "class Foo {\n  foo() {\n    super.x;\n  }\n}\n")
	expectPrinted(t, "class Foo { foo() { super[x] } }", "class Foo {\n  foo() {\n    super[x];\n  }\n}\n")
	expectPrinted(t, "class Foo { foo(x = super.x) {} }", "class Foo {\n  foo(x = super.x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { foo(x = super[x]) {} }", "class Foo {\n  foo(x = super[x]) {\n  }\n}\n")
	expectPrinted(t, "class Foo { static foo() { super.x } }", "class Foo {\n  static foo() {\n    super.x;\n  }\n}\n")
	expectPrinted(t, "class Foo { static foo() { super[x] } }", "class Foo {\n  static foo() {\n    super[x];\n  }\n}\n")
	expectPrinted(t, "class Foo { static foo(x = super.x) {} }", "class Foo {\n  static foo(x = super.x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { static foo(x = super[x]) {} }", "class Foo {\n  static foo(x = super[x]) {\n  }\n}\n")

	expectPrinted(t, "(class { foo() { super.x } })", "(class {\n  foo() {\n    super.x;\n  }\n});\n")
	expectPrinted(t, "(class { foo() { super[x] } })", "(class {\n  foo() {\n    super[x];\n  }\n});\n")
	expectPrinted(t, "(class { foo(x = super.x) {} })", "(class {\n  foo(x = super.x) {\n  }\n});\n")
	expectPrinted(t, "(class { foo(x = super[x]) {} })", "(class {\n  foo(x = super[x]) {\n  }\n});\n")
	expectPrinted(t, "(class { static foo() { super.x } })", "(class {\n  static foo() {\n    super.x;\n  }\n});\n")
	expectPrinted(t, "(class { static foo() { super[x] } })", "(class {\n  static foo() {\n    super[x];\n  }\n});\n")
	expectPrinted(t, "(class { static foo(x = super.x) {} })", "(class {\n  static foo(x = super.x) {\n  }\n});\n")
	expectPrinted(t, "(class { static foo(x = super[x]) {} })", "(class {\n  static foo(x = super[x]) {\n  }\n});\n")

	expectPrinted(t, "class Foo { foo = super.x }", "class Foo {\n  foo = super.x;\n}\n")
	expectPrinted(t, "class Foo { foo = super[x] }", "class Foo {\n  foo = super[x];\n}\n")
	expectPrinted(t, "class Foo { foo = () => super.x }", "class Foo {\n  foo = () => super.x;\n}\n")
	expectPrinted(t, "class Foo { foo = () => super[x] }", "class Foo {\n  foo = () => super[x];\n}\n")
	expectParseError(t, "class Foo { foo = function () { super.x } }", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "class Foo { foo = function () { super[x] } }", "<stdin>: ERROR: Unexpected \"super\"\n")

	expectPrinted(t, "class Foo { static foo = super.x }", "class Foo {\n  static foo = super.x;\n}\n")
	expectPrinted(t, "class Foo { static foo = super[x] }", "class Foo {\n  static foo = super[x];\n}\n")
	expectPrinted(t, "class Foo { static foo = () => super.x }", "class Foo {\n  static foo = () => super.x;\n}\n")
	expectPrinted(t, "class Foo { static foo = () => super[x] }", "class Foo {\n  static foo = () => super[x];\n}\n")
	expectParseError(t, "class Foo { static foo = function () { super.x } }", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "class Foo { static foo = function () { super[x] } }", "<stdin>: ERROR: Unexpected \"super\"\n")

	expectPrinted(t, "(class { foo = super.x })", "(class {\n  foo = super.x;\n});\n")
	expectPrinted(t, "(class { foo = super[x] })", "(class {\n  foo = super[x];\n});\n")
	expectPrinted(t, "(class { foo = () => super.x })", "(class {\n  foo = () => super.x;\n});\n")
	expectPrinted(t, "(class { foo = () => super[x] })", "(class {\n  foo = () => super[x];\n});\n")
	expectParseError(t, "(class { foo = function () { super.x } })", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "(class { foo = function () { super[x] } })", "<stdin>: ERROR: Unexpected \"super\"\n")

	expectPrinted(t, "(class { static foo = super.x })", "(class {\n  static foo = super.x;\n});\n")
	expectPrinted(t, "(class { static foo = super[x] })", "(class {\n  static foo = super[x];\n});\n")
	expectPrinted(t, "(class { static foo = () => super.x })", "(class {\n  static foo = () => super.x;\n});\n")
	expectPrinted(t, "(class { static foo = () => super[x] })", "(class {\n  static foo = () => super[x];\n});\n")
	expectParseError(t, "(class { static foo = function () { super.x } })", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "(class { static foo = function () { super[x] } })", "<stdin>: ERROR: Unexpected \"super\"\n")

	expectParseError(t, "({ foo: super.x })", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "({ foo: super[x] })", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "({ foo: () => super.x })", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "({ foo: () => super[x] })", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "({ foo: function () { super.x } })", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "({ foo: function () { super[x] } })", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectPrinted(t, "({ foo() { super.x } })", "({ foo() {\n  super.x;\n} });\n")
	expectPrinted(t, "({ foo() { super[x] } })", "({ foo() {\n  super[x];\n} });\n")
	expectPrinted(t, "({ foo(x = super.x) {} })", "({ foo(x = super.x) {\n} });\n")

	expectParseError(t, "class Foo { [super.x] }", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "class Foo { [super[x]] }", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "class Foo { static [super.x] }", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "class Foo { static [super[x]] }", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "(class { [super.x] })", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "(class { [super[x]] })", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "(class { static [super.x] })", "<stdin>: ERROR: Unexpected \"super\"\n")
	expectParseError(t, "(class { static [super[x]] })", "<stdin>: ERROR: Unexpected \"super\"\n")
}

func TestClassFields(t *testing.T) {
	expectPrinted(t, "class Foo { a }", "class Foo {\n  a;\n}\n")
	expectPrinted(t, "class Foo { a = 1 }", "class Foo {\n  a = 1;\n}\n")
	expectPrinted(t, "class Foo { a = 1; b }", "class Foo {\n  a = 1;\n  b;\n}\n")
	expectParseError(t, "class Foo { a = 1 b }", "<stdin>: ERROR: Expected \";\" but found \"b\"\n")

	expectPrinted(t, "class Foo { [a] }", "class Foo {\n  [a];\n}\n")
	expectPrinted(t, "class Foo { [a] = 1 }", "class Foo {\n  [a] = 1;\n}\n")
	expectPrinted(t, "class Foo { [a] = 1; [b] }", "class Foo {\n  [a] = 1;\n  [b];\n}\n")
	expectParseError(t, "class Foo { [a] = 1 b }", "<stdin>: ERROR: Expected \";\" but found \"b\"\n")

	expectPrinted(t, "class Foo { static a }", "class Foo {\n  static a;\n}\n")
	expectPrinted(t, "class Foo { static a = 1 }", "class Foo {\n  static a = 1;\n}\n")
	expectPrinted(t, "class Foo { static a = 1; b }", "class Foo {\n  static a = 1;\n  b;\n}\n")
	expectParseError(t, "class Foo { static a = 1 b }", "<stdin>: ERROR: Expected \";\" but found \"b\"\n")

	expectPrinted(t, "class Foo { static [a] }", "class Foo {\n  static [a];\n}\n")
	expectPrinted(t, "class Foo { static [a] = 1 }", "class Foo {\n  static [a] = 1;\n}\n")
	expectPrinted(t, "class Foo { static [a] = 1; [b] }", "class Foo {\n  static [a] = 1;\n  [b];\n}\n")
	expectParseError(t, "class Foo { static [a] = 1 b }", "<stdin>: ERROR: Expected \";\" but found \"b\"\n")

	expectParseError(t, "class Foo { get a }", "<stdin>: ERROR: Expected \"(\" but found \"}\"\n")
	expectParseError(t, "class Foo { set a }", "<stdin>: ERROR: Expected \"(\" but found \"}\"\n")
	expectParseError(t, "class Foo { async a }", "<stdin>: ERROR: Expected \"(\" but found \"}\"\n")

	expectParseError(t, "class Foo { get a = 1 }", "<stdin>: ERROR: Expected \"(\" but found \"=\"\n")
	expectParseError(t, "class Foo { set a = 1 }", "<stdin>: ERROR: Expected \"(\" but found \"=\"\n")
	expectParseError(t, "class Foo { async a = 1 }", "<stdin>: ERROR: Expected \"(\" but found \"=\"\n")

	expectParseError(t, "class Foo { `a` = 0 }", "<stdin>: ERROR: Expected identifier but found \"`a`\"\n")

	// The name "constructor" is forbidden
	expectParseError(t, "class Foo { constructor }", "<stdin>: ERROR: Invalid field name \"constructor\"\n")
	expectParseError(t, "class Foo { 'constructor' }", "<stdin>: ERROR: Invalid field name \"constructor\"\n")
	expectParseError(t, "class Foo { constructor = 1 }", "<stdin>: ERROR: Invalid field name \"constructor\"\n")
	expectParseError(t, "class Foo { 'constructor' = 1 }", "<stdin>: ERROR: Invalid field name \"constructor\"\n")
	expectParseError(t, "class Foo { static constructor }", "<stdin>: ERROR: Invalid field name \"constructor\"\n")
	expectParseError(t, "class Foo { static 'constructor' }", "<stdin>: ERROR: Invalid field name \"constructor\"\n")
	expectParseError(t, "class Foo { static constructor = 1 }", "<stdin>: ERROR: Invalid field name \"constructor\"\n")
	expectParseError(t, "class Foo { static 'constructor' = 1 }", "<stdin>: ERROR: Invalid field name \"constructor\"\n")

	expectPrinted(t, "class Foo { ['constructor'] }", "class Foo {\n  [\"constructor\"];\n}\n")
	expectPrinted(t, "class Foo { ['constructor'] = 1 }", "class Foo {\n  [\"constructor\"] = 1;\n}\n")
	expectPrinted(t, "class Foo { static ['constructor'] }", "class Foo {\n  static [\"constructor\"];\n}\n")
	expectPrinted(t, "class Foo { static ['constructor'] = 1 }", "class Foo {\n  static [\"constructor\"] = 1;\n}\n")

	// The name "prototype" is sometimes forbidden
	expectPrinted(t, "class Foo { prototype }", "class Foo {\n  prototype;\n}\n")
	expectPrinted(t, "class Foo { 'prototype' }", "class Foo {\n  \"prototype\";\n}\n")
	expectPrinted(t, "class Foo { prototype = 1 }", "class Foo {\n  prototype = 1;\n}\n")
	expectPrinted(t, "class Foo { 'prototype' = 1 }", "class Foo {\n  \"prototype\" = 1;\n}\n")
	expectParseError(t, "class Foo { static prototype }", "<stdin>: ERROR: Invalid field name \"prototype\"\n")
	expectParseError(t, "class Foo { static 'prototype' }", "<stdin>: ERROR: Invalid field name \"prototype\"\n")
	expectParseError(t, "class Foo { static prototype = 1 }", "<stdin>: ERROR: Invalid field name \"prototype\"\n")
	expectParseError(t, "class Foo { static 'prototype' = 1 }", "<stdin>: ERROR: Invalid field name \"prototype\"\n")
	expectPrinted(t, "class Foo { static ['prototype'] }", "class Foo {\n  static [\"prototype\"];\n}\n")
	expectPrinted(t, "class Foo { static ['prototype'] = 1 }", "class Foo {\n  static [\"prototype\"] = 1;\n}\n")
}

func TestClassStaticBlocks(t *testing.T) {
	expectPrinted(t, "class Foo { static {} }", "class Foo {\n  static {\n  }\n}\n")
	expectPrinted(t, "class Foo { static {} x = 1 }", "class Foo {\n  static {\n  }\n  x = 1;\n}\n")
	expectPrinted(t, "class Foo { static { this.foo() } }", "class Foo {\n  static {\n    this.foo();\n  }\n}\n")

	expectParseError(t, "class Foo { static { yield } }",
		"<stdin>: ERROR: \"yield\" is a reserved word and cannot be used in strict mode\n"+
			"<stdin>: NOTE: All code inside a class is implicitly in strict mode\n")
	expectParseError(t, "class Foo { static { await } }", "<stdin>: ERROR: The keyword \"await\" cannot be used here:\n")
	expectParseError(t, "class Foo { static { return } }", "<stdin>: ERROR: A return statement cannot be used here:\n")
	expectParseError(t, "class Foo { static { break } }", "<stdin>: ERROR: Cannot use \"break\" here:\n")
	expectParseError(t, "class Foo { static { continue } }", "<stdin>: ERROR: Cannot use \"continue\" here:\n")
	expectParseError(t, "x: { class Foo { static { break x } } }", "<stdin>: ERROR: There is no containing label named \"x\"\n")
	expectParseError(t, "x: { class Foo { static { continue x } } }", "<stdin>: ERROR: There is no containing label named \"x\"\n")

	expectPrintedMangle(t, "class Foo { static {} }", "class Foo {\n}\n")
	expectPrintedMangle(t, "class Foo { static { 123 } }", "class Foo {\n}\n")
	expectPrintedMangle(t, "class Foo { static { /* @__PURE__ */ foo() } }", "class Foo {\n}\n")
	expectPrintedMangle(t, "class Foo { static { foo() } }", "class Foo {\n  static {\n    foo();\n  }\n}\n")
}

func TestAutoAccessors(t *testing.T) {
	expectPrinted(t, "class Foo { accessor }", "class Foo {\n  accessor;\n}\n")
	expectPrinted(t, "class Foo { accessor \n x }", "class Foo {\n  accessor;\n  x;\n}\n")
	expectPrinted(t, "class Foo { static accessor }", "class Foo {\n  static accessor;\n}\n")
	expectPrinted(t, "class Foo { static accessor \n x }", "class Foo {\n  static accessor;\n  x;\n}\n")

	expectPrinted(t, "class Foo { accessor x }", "class Foo {\n  accessor x;\n}\n")
	expectPrinted(t, "class Foo { accessor x = y }", "class Foo {\n  accessor x = y;\n}\n")
	expectPrinted(t, "class Foo { accessor [x] }", "class Foo {\n  accessor [x];\n}\n")
	expectPrinted(t, "class Foo { accessor [x] = y }", "class Foo {\n  accessor [x] = y;\n}\n")
	expectPrinted(t, "class Foo { static accessor x }", "class Foo {\n  static accessor x;\n}\n")
	expectPrinted(t, "class Foo { static accessor [x] }", "class Foo {\n  static accessor [x];\n}\n")
	expectPrinted(t, "class Foo { static accessor x = y }", "class Foo {\n  static accessor x = y;\n}\n")
	expectPrinted(t, "class Foo { static accessor [x] = y }", "class Foo {\n  static accessor [x] = y;\n}\n")

	expectPrinted(t, "Foo = class { accessor x }", "Foo = class {\n  accessor x;\n};\n")
	expectPrinted(t, "Foo = class { accessor [x] }", "Foo = class {\n  accessor [x];\n};\n")
	expectPrinted(t, "Foo = class { accessor x = y }", "Foo = class {\n  accessor x = y;\n};\n")
	expectPrinted(t, "Foo = class { accessor [x] = y }", "Foo = class {\n  accessor [x] = y;\n};\n")
	expectPrinted(t, "Foo = class { static accessor x }", "Foo = class {\n  static accessor x;\n};\n")
	expectPrinted(t, "Foo = class { static accessor [x] }", "Foo = class {\n  static accessor [x];\n};\n")
	expectPrinted(t, "Foo = class { static accessor x = y }", "Foo = class {\n  static accessor x = y;\n};\n")

	expectPrinted(t, "class Foo { accessor get }", "class Foo {\n  accessor get;\n}\n")
	expectPrinted(t, "class Foo { get accessor() {} }", "class Foo {\n  get accessor() {\n  }\n}\n")
	expectParseError(t, "class Foo { accessor x() {} }", "<stdin>: ERROR: Expected \";\" but found \"(\"\n")
	expectParseError(t, "class Foo { accessor get x() {} }", "<stdin>: ERROR: Expected \";\" but found \"x\"\n")
	expectParseError(t, "class Foo { get accessor x() {} }", "<stdin>: ERROR: Expected \"(\" but found \"x\"\n")

	expectPrinted(t, "Foo = { get accessor() {} }", "Foo = { get accessor() {\n} };\n")
	expectParseError(t, "Foo = { accessor x }", "<stdin>: ERROR: Expected \"}\" but found \"x\"\n")
	expectParseError(t, "Foo = { accessor x() {} }", "<stdin>: ERROR: Expected \"}\" but found \"x\"\n")
	expectParseError(t, "Foo = { get accessor x() {} }", "<stdin>: ERROR: Expected \"(\" but found \"x\"\n")

	expectParseError(t, "class Foo { accessor x, y }", "<stdin>: ERROR: Expected \";\" but found \",\"\n")
	expectParseError(t, "class Foo { static accessor x, y }", "<stdin>: ERROR: Expected \";\" but found \",\"\n")
	expectParseError(t, "Foo = class { accessor x, y }", "<stdin>: ERROR: Expected \";\" but found \",\"\n")
	expectParseError(t, "Foo = class { static accessor x, y }", "<stdin>: ERROR: Expected \";\" but found \",\"\n")
}

func TestDecorators(t *testing.T) {
	expectPrinted(t, "@x @y class Foo {}", "@x\n@y\nclass Foo {\n}\n")
	expectPrinted(t, "@x @y export class Foo {}", "@x\n@y\nexport class Foo {\n}\n")
	expectPrinted(t, "@x @y export default class Foo {}", "@x\n@y\nexport default class Foo {\n}\n")
	expectPrinted(t, "_ = @x @y class {}", "_ = @x @y class {\n};\n")

	expectPrinted(t, "class Foo { @x y }", "class Foo {\n  @x\n  y;\n}\n")
	expectPrinted(t, "class Foo { @x y() {} }", "class Foo {\n  @x\n  y() {\n  }\n}\n")
	expectPrinted(t, "class Foo { @x static y }", "class Foo {\n  @x\n  static y;\n}\n")
	expectPrinted(t, "class Foo { @x static y() {} }", "class Foo {\n  @x\n  static y() {\n  }\n}\n")
	expectPrinted(t, "class Foo { @x accessor y }", "class Foo {\n  @x\n  accessor y;\n}\n")

	expectPrinted(t, "class Foo { @x #y }", "class Foo {\n  @x\n  #y;\n}\n")
	expectPrinted(t, "class Foo { @x #y() {} }", "class Foo {\n  @x\n  #y() {\n  }\n}\n")
	expectPrinted(t, "class Foo { @x static #y }", "class Foo {\n  @x\n  static #y;\n}\n")
	expectPrinted(t, "class Foo { @x static #y() {} }", "class Foo {\n  @x\n  static #y() {\n  }\n}\n")
	expectPrinted(t, "class Foo { @x accessor #y }", "class Foo {\n  @x\n  accessor #y;\n}\n")

	expectParseError(t, "class Foo { x(@y z) {} }", "<stdin>: ERROR: Parameter decorators are not allowed in JavaScript\n")
	expectParseError(t, "class Foo { @x static {} }", "<stdin>: ERROR: Expected \";\" but found \"{\"\n")

	expectPrinted(t, "@\na\n(\n)\n@\n(\nb\n)\nclass\nFoo\n{\n}\n", "@a()\n@b\nclass Foo {\n}\n")
	expectPrinted(t, "@(a, b) class Foo {}\n", "@(a, b)\nclass Foo {\n}\n")
	expectPrinted(t, "@x() class Foo {}", "@x()\nclass Foo {\n}\n")
	expectPrinted(t, "@x.y() class Foo {}", "@x.y()\nclass Foo {\n}\n")
	expectPrinted(t, "@(() => {}) class Foo {}", "@(() => {\n})\nclass Foo {\n}\n")
	expectPrinted(t, "class Foo { #x = @y.#x.y.#x class {} }", "class Foo {\n  #x = @y.#x.y.#x class {\n  };\n}\n")
	expectParseError(t, "@123 class Foo {}", "<stdin>: ERROR: Expected identifier but found \"123\"\n")
	expectParseError(t, "@x[y] class Foo {}",
		"<stdin>: ERROR: Expected \"class\" after decorator but found \"[\"\n<stdin>: NOTE: The preceding decorator is here:\n"+
			"NOTE: Decorators can only be used with class declarations.\n<stdin>: ERROR: Expected \";\" but found \"class\"\n")
	expectParseError(t, "@x?.() class Foo {}", "<stdin>: ERROR: Expected \".\" but found \"?.\"\n")
	expectParseError(t, "@x?.y() class Foo {}", "<stdin>: ERROR: Expected \".\" but found \"?.\"\n")
	expectParseError(t, "@x?.[y]() class Foo {}", "<stdin>: ERROR: Expected \".\" but found \"?.\"\n")
	expectParseError(t, "@new Function() class Foo {}", "<stdin>: ERROR: Expected identifier but found \"new\"\n")
	expectParseError(t, "@() => {} class Foo {}", "<stdin>: ERROR: Unexpected \")\"\n")

	errorText := "<stdin>: ERROR: Transforming JavaScript decorators to the configured target environment is not supported yet\n"
	expectParseErrorWithUnsupportedFeatures(t, compat.Decorators, "@dec class Foo {}", errorText)
	expectParseErrorWithUnsupportedFeatures(t, compat.Decorators, "class Foo { @dec x }", errorText)
	expectParseErrorWithUnsupportedFeatures(t, compat.Decorators, "class Foo { @dec x() {} }", errorText)
	expectParseErrorWithUnsupportedFeatures(t, compat.Decorators, "class Foo { @dec accessor x }", errorText)
	expectParseErrorWithUnsupportedFeatures(t, compat.Decorators, "class Foo { @dec static x }", errorText)
	expectParseErrorWithUnsupportedFeatures(t, compat.Decorators, "class Foo { @dec static x() {} }", errorText)
	expectParseErrorWithUnsupportedFeatures(t, compat.Decorators, "class Foo { @dec static accessor x }", errorText)
}

func TestGenerator(t *testing.T) {
	expectParseError(t, "(class { * foo })", "<stdin>: ERROR: Expected \"(\" but found \"}\"\n")
	expectParseError(t, "(class { * *foo() {} })", "<stdin>: ERROR: Unexpected \"*\"\n")
	expectParseError(t, "(class { get*foo() {} })", "<stdin>: ERROR: Unexpected \"*\"\n")
	expectParseError(t, "(class { set*foo() {} })", "<stdin>: ERROR: Unexpected \"*\"\n")
	expectParseError(t, "(class { *get foo() {} })", "<stdin>: ERROR: Expected \"(\" but found \"foo\"\n")
	expectParseError(t, "(class { *set foo() {} })", "<stdin>: ERROR: Expected \"(\" but found \"foo\"\n")
	expectParseError(t, "(class { *static foo() {} })", "<stdin>: ERROR: Expected \"(\" but found \"foo\"\n")

	expectParseError(t, "function* foo() { -yield 100 }", "<stdin>: ERROR: Cannot use a \"yield\" expression here without parentheses:\n")
	expectPrinted(t, "function* foo() { -(yield 100) }", "function* foo() {\n  -(yield 100);\n}\n")
}

func TestYield(t *testing.T) {
	expectParseError(t, "yield 100", "<stdin>: ERROR: Cannot use \"yield\" outside a generator function\n")
	expectParseError(t, "-yield 100", "<stdin>: ERROR: Cannot use \"yield\" outside a generator function\n")
	expectPrinted(t, "yield\n100", "yield;\n100;\n")

	noYield := "<stdin>: ERROR: The keyword \"yield\" cannot be used here:\n"
	expectParseError(t, "function* bar(x = yield y) {}", noYield+"<stdin>: ERROR: Expected \")\" but found \"y\"\n")
	expectParseError(t, "(function*(x = yield y) {})", noYield+"<stdin>: ERROR: Expected \")\" but found \"y\"\n")
	expectParseError(t, "({ *foo(x = yield y) {} })", noYield+"<stdin>: ERROR: Expected \")\" but found \"y\"\n")
	expectParseError(t, "class Foo { *foo(x = yield y) {} }", noYield+"<stdin>: ERROR: Expected \")\" but found \"y\"\n")
	expectParseError(t, "(class { *foo(x = yield y) {} })", noYield+"<stdin>: ERROR: Expected \")\" but found \"y\"\n")

	expectParseError(t, "function *foo() { function bar(x = yield y) {} }", "<stdin>: ERROR: Cannot use \"yield\" outside a generator function\n")
	expectParseError(t, "function *foo() { (function(x = yield y) {}) }", "<stdin>: ERROR: Cannot use \"yield\" outside a generator function\n")
	expectParseError(t, "function *foo() { ({ foo(x = yield y) {} }) }", "<stdin>: ERROR: Cannot use \"yield\" outside a generator function\n")
	expectParseError(t, "function *foo() { class Foo { foo(x = yield y) {} } }", "<stdin>: ERROR: Cannot use \"yield\" outside a generator function\n")
	expectParseError(t, "function *foo() { (class { foo(x = yield y) {} }) }", "<stdin>: ERROR: Cannot use \"yield\" outside a generator function\n")
	expectParseError(t, "function *foo() { (x = yield y) => {} }", "<stdin>: ERROR: Cannot use a \"yield\" expression here:\n")
	expectPrinted(t, "function *foo() { x = yield }", "function* foo() {\n  x = yield;\n}\n")
	expectPrinted(t, "function *foo() { x = yield; }", "function* foo() {\n  x = yield;\n}\n")
	expectPrinted(t, "function *foo() { (x = yield) }", "function* foo() {\n  x = yield;\n}\n")
	expectPrinted(t, "function *foo() { [x = yield] }", "function* foo() {\n  [x = yield];\n}\n")
	expectPrinted(t, "function *foo() { x = (yield, yield) }", "function* foo() {\n  x = (yield, yield);\n}\n")
	expectPrinted(t, "function *foo() { x = y ? yield : yield }", "function* foo() {\n  x = y ? yield : yield;\n}\n")
	expectParseError(t, "function *foo() { x = yield ? y : z }", "<stdin>: ERROR: Unexpected \"?\"\n")
	expectParseError(t, "function *foo() { x = yield * }", "<stdin>: ERROR: Unexpected \"}\"\n")
	expectParseError(t, "function *foo() { (x = yield *) }", "<stdin>: ERROR: Unexpected \")\"\n")
	expectParseError(t, "function *foo() { [x = yield *] }", "<stdin>: ERROR: Unexpected \"]\"\n")
	expectPrinted(t, "function *foo() { x = yield y }", "function* foo() {\n  x = yield y;\n}\n")
	expectPrinted(t, "function *foo() { (x = yield y) }", "function* foo() {\n  x = yield y;\n}\n")
	expectPrinted(t, "function *foo() { x = yield \n y }", "function* foo() {\n  x = yield;\n  y;\n}\n")
	expectPrinted(t, "function *foo() { x = yield * y }", "function* foo() {\n  x = yield* y;\n}\n")
	expectPrinted(t, "function *foo() { (x = yield * y) }", "function* foo() {\n  x = yield* y;\n}\n")
	expectPrinted(t, "function *foo() { x = yield * \n y }", "function* foo() {\n  x = yield* y;\n}\n")
	expectParseError(t, "function *foo() { x = yield \n * y }", "<stdin>: ERROR: Unexpected \"*\"\n")
	expectParseError(t, "function foo() { (x = yield y) }", "<stdin>: ERROR: Cannot use \"yield\" outside a generator function\n")
	expectPrinted(t, "function foo() { x = yield * y }", "function foo() {\n  x = yield * y;\n}\n")
	expectPrinted(t, "function foo() { (x = yield * y) }", "function foo() {\n  x = yield * y;\n}\n")
	expectParseError(t, "function *foo() { (x = \\u0079ield) }", "<stdin>: ERROR: The keyword \"yield\" cannot be escaped\n")
	expectParseError(t, "function *foo() { (x = \\u0079ield* y) }", "<stdin>: ERROR: The keyword \"yield\" cannot be escaped\n")

	// Yield as an identifier
	expectPrinted(t, "({yield} = x)", "({ yield } = x);\n")
	expectPrinted(t, "let x = {yield}", "let x = { yield };\n")
	expectPrinted(t, "function* yield() {}", "function* yield() {\n}\n")
	expectPrinted(t, "function foo() { ({yield} = x) }", "function foo() {\n  ({ yield } = x);\n}\n")
	expectPrinted(t, "function foo() { let x = {yield} }", "function foo() {\n  let x = { yield };\n}\n")
	expectParseError(t, "function *foo() { ({yield} = x) }", "<stdin>: ERROR: Cannot use \"yield\" as an identifier here:\n")
	expectParseError(t, "function *foo() { let x = {yield} }", "<stdin>: ERROR: Cannot use \"yield\" as an identifier here:\n")

	// Yield as a declaration
	expectPrinted(t, "({ *yield() {} })", "({ *yield() {\n} });\n")
	expectPrinted(t, "(class { *yield() {} })", "(class {\n  *yield() {\n  }\n});\n")
	expectPrinted(t, "class Foo { *yield() {} }", "class Foo {\n  *yield() {\n  }\n}\n")
	expectPrinted(t, "function* yield() {}", "function* yield() {\n}\n")
	expectParseError(t, "(function* yield() {})", "<stdin>: ERROR: A generator function expression cannot be named \"yield\"\n")

	// Yield as an async declaration
	expectPrinted(t, "({ async *yield() {} })", "({ async *yield() {\n} });\n")
	expectPrinted(t, "(class { async *yield() {} })", "(class {\n  async *yield() {\n  }\n});\n")
	expectPrinted(t, "class Foo { async *yield() {} }", "class Foo {\n  async *yield() {\n  }\n}\n")
	expectPrinted(t, "async function* yield() {}", "async function* yield() {\n}\n")
	expectParseError(t, "(async function* yield() {})", "<stdin>: ERROR: A generator function expression cannot be named \"yield\"\n")
}

func TestAsync(t *testing.T) {
	expectPrinted(t, "function foo() { await }", "function foo() {\n  await;\n}\n")
	expectPrinted(t, "async function foo() { await 0 }", "async function foo() {\n  await 0;\n}\n")
	expectParseError(t, "async function() {}", "<stdin>: ERROR: Expected identifier but found \"(\"\n")

	expectPrinted(t, "-async function foo() { await 0 }", "-async function foo() {\n  await 0;\n};\n")
	expectPrinted(t, "-async function() { await 0 }", "-async function() {\n  await 0;\n};\n")
	expectPrinted(t, "1 - async function foo() { await 0 }", "1 - async function foo() {\n  await 0;\n};\n")
	expectPrinted(t, "1 - async function() { await 0 }", "1 - async function() {\n  await 0;\n};\n")
	expectPrinted(t, "(async function foo() { await 0 })", "(async function foo() {\n  await 0;\n});\n")
	expectPrinted(t, "(async function() { await 0 })", "(async function() {\n  await 0;\n});\n")
	expectPrinted(t, "(x, async function foo() { await 0 })", "x, async function foo() {\n  await 0;\n};\n")
	expectPrinted(t, "(x, async function() { await 0 })", "x, async function() {\n  await 0;\n};\n")
	expectPrinted(t, "new async function() { await 0 }", "new async function() {\n  await 0;\n}();\n")
	expectPrinted(t, "new async function() { await 0 }.x", "new async function() {\n  await 0;\n}.x();\n")

	friendlyAwaitError := "<stdin>: ERROR: \"await\" can only be used inside an \"async\" function\n"
	friendlyAwaitErrorWithNote := friendlyAwaitError + "<stdin>: NOTE: Consider adding the \"async\" keyword here:\n"

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
	expectPrinted(t, "new async()", "new async();\n")
	expectPrinted(t, "new async().x", "new async().x;\n")
	expectPrinted(t, "new (async())", "new (async())();\n")
	expectPrinted(t, "new (async().x)", "new (async()).x();\n")
	expectParseError(t, "async x;", "<stdin>: ERROR: Expected \"=>\" but found \";\"\n")
	expectParseError(t, "async (...x,) => {}", "<stdin>: ERROR: Unexpected \",\" after rest pattern\n")
	expectParseError(t, "async => await 0", friendlyAwaitErrorWithNote)
	expectParseError(t, "new async => {}", "<stdin>: ERROR: Expected \";\" but found \"=>\"\n")
	expectParseError(t, "new async () => {}", "<stdin>: ERROR: Expected \";\" but found \"=>\"\n")

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
	expectParseError(t, "export default async x => y, z", "<stdin>: ERROR: Expected \";\" but found \",\"\n")
	expectParseError(t, "export default async (x) => y, z", "<stdin>: ERROR: Expected \";\" but found \",\"\n")

	expectPrinted(t, "class Foo { async async() {} }", "class Foo {\n  async async() {\n  }\n}\n")
	expectPrinted(t, "(class { async async() {} })", "(class {\n  async async() {\n  }\n});\n")
	expectPrinted(t, "({ async async() {} })", "({ async async() {\n} });\n")
	expectParseError(t, "class Foo { async async }", "<stdin>: ERROR: Expected \"(\" but found \"}\"\n")
	expectParseError(t, "(class { async async })", "<stdin>: ERROR: Expected \"(\" but found \"}\"\n")
	expectParseError(t, "({ async async })", "<stdin>: ERROR: Expected \"(\" but found \"}\"\n")

	noAwait := "<stdin>: ERROR: The keyword \"await\" cannot be used here:\n"
	expectParseError(t, "async function bar(x = await y) {}", noAwait+"<stdin>: ERROR: Expected \")\" but found \"y\"\n")
	expectParseError(t, "async (function(x = await y) {})", friendlyAwaitError)
	expectParseError(t, "async ({ foo(x = await y) {} })", friendlyAwaitError)
	expectParseError(t, "class Foo { async foo(x = await y) {} }", noAwait+"<stdin>: ERROR: Expected \")\" but found \"y\"\n")
	expectParseError(t, "(class { async foo(x = await y) {} })", noAwait+"<stdin>: ERROR: Expected \")\" but found \"y\"\n")

	expectParseError(t, "async function foo() { function bar(x = await y) {} }", friendlyAwaitError)
	expectParseError(t, "async function foo() { (function(x = await y) {}) }", friendlyAwaitError)
	expectParseError(t, "async function foo() { ({ foo(x = await y) {} }) }", friendlyAwaitError)
	expectParseError(t, "async function foo() { class Foo { foo(x = await y) {} } }", friendlyAwaitError)
	expectParseError(t, "async function foo() { (class { foo(x = await y) {} }) }", friendlyAwaitError)
	expectParseError(t, "async function foo() { (x = await y) => {} }", "<stdin>: ERROR: Cannot use an \"await\" expression here:\n")
	expectParseError(t, "async function foo(x = await y) {}", "<stdin>: ERROR: The keyword \"await\" cannot be used here:\n<stdin>: ERROR: Expected \")\" but found \"y\"\n")
	expectParseError(t, "async function foo({ [await y]: x }) {}", "<stdin>: ERROR: The keyword \"await\" cannot be used here:\n<stdin>: ERROR: Expected \"]\" but found \"y\"\n")
	expectPrinted(t, "async function foo() { (x = await y) }", "async function foo() {\n  x = await y;\n}\n")
	expectParseError(t, "function foo() { (x = await y) }", friendlyAwaitErrorWithNote)

	// Newlines
	expectPrinted(t, "(class { async \n foo() {} })", "(class {\n  async;\n  foo() {\n  }\n});\n")
	expectPrinted(t, "(class { async \n *foo() {} })", "(class {\n  async;\n  *foo() {\n  }\n});\n")
	expectParseError(t, "({ async \n foo() {} })", "<stdin>: ERROR: Expected \"}\" but found \"foo\"\n")
	expectParseError(t, "({ async \n *foo() {} })", "<stdin>: ERROR: Expected \"}\" but found \"*\"\n")

	// Top-level await
	expectPrinted(t, "await foo;", "await foo;\n")
	expectPrinted(t, "for await(foo of bar);", "for await (foo of bar)\n  ;\n")
	expectParseError(t, "function foo() { await foo }", friendlyAwaitErrorWithNote)
	expectParseError(t, "function foo() { for await(foo of bar); }", "<stdin>: ERROR: Cannot use \"await\" outside an async function\n")
	expectPrinted(t, "function foo(x = await) {}", "function foo(x = await) {\n}\n")
	expectParseError(t, "function foo(x = await y) {}", friendlyAwaitError)
	expectPrinted(t, "(function(x = await) {})", "(function(x = await) {\n});\n")
	expectParseError(t, "(function(x = await y) {})", friendlyAwaitError)
	expectPrinted(t, "({ foo(x = await) {} })", "({ foo(x = await) {\n} });\n")
	expectParseError(t, "({ foo(x = await y) {} })", friendlyAwaitError)
	expectPrinted(t, "class Foo { foo(x = await) {} }", "class Foo {\n  foo(x = await) {\n  }\n}\n")
	expectParseError(t, "class Foo { foo(x = await y) {} }", friendlyAwaitError)
	expectPrinted(t, "(class { foo(x = await) {} })", "(class {\n  foo(x = await) {\n  }\n});\n")
	expectParseError(t, "(class { foo(x = await y) {} })", friendlyAwaitError)
	expectParseError(t, "(x = await) => {}", "<stdin>: ERROR: Unexpected \")\"\n")
	expectParseError(t, "(x = await y) => {}", "<stdin>: ERROR: Cannot use an \"await\" expression here:\n")
	expectParseError(t, "(x = await)", "<stdin>: ERROR: Unexpected \")\"\n")
	expectPrinted(t, "(x = await y)", "x = await y;\n")
	expectParseError(t, "async (x = await) => {}", "<stdin>: ERROR: Unexpected \")\"\n")
	expectParseError(t, "async (x = await y) => {}", "<stdin>: ERROR: Cannot use an \"await\" expression here:\n")
	expectPrinted(t, "async(x = await y)", "async(x = await y);\n")

	// Keywords with escapes
	expectPrinted(t, "\\u0061sync", "async;\n")
	expectPrinted(t, "(\\u0061sync)", "async;\n")
	expectPrinted(t, "function foo() { \\u0061wait }", "function foo() {\n  await;\n}\n")
	expectPrinted(t, "function foo() { var \\u0061wait }", "function foo() {\n  var await;\n}\n")
	expectParseError(t, "\\u0061wait", "<stdin>: ERROR: The keyword \"await\" cannot be escaped\n")
	expectParseError(t, "var \\u0061wait", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")
	expectParseError(t, "async function foo() { \\u0061wait }", "<stdin>: ERROR: The keyword \"await\" cannot be escaped\n")
	expectParseError(t, "async function foo() { var \\u0061wait }", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")
	expectParseError(t, "\\u0061sync x => {}", "<stdin>: ERROR: Expected \";\" but found \"x\"\n")
	expectParseError(t, "\\u0061sync () => {}", "<stdin>: ERROR: Expected \";\" but found \"=>\"\n")
	expectParseError(t, "\\u0061sync function foo() {}", "<stdin>: ERROR: Expected \";\" but found \"function\"\n")
	expectParseError(t, "({ \\u0061sync foo() {} })", "<stdin>: ERROR: Expected \"}\" but found \"foo\"\n")
	expectParseError(t, "({ \\u0061sync *foo() {} })", "<stdin>: ERROR: Expected \"}\" but found \"*\"\n")

	// For-await
	expectParseError(t, "for await(;;);", "<stdin>: ERROR: Unexpected \";\"\n")
	expectParseError(t, "for await(x in y);", "<stdin>: ERROR: Expected \"of\" but found \"in\"\n")
	expectParseError(t, "async function foo(){for await(;;);}", "<stdin>: ERROR: Unexpected \";\"\n")
	expectParseError(t, "async function foo(){for await(let x;;);}", "<stdin>: ERROR: Expected \"of\" but found \";\"\n")
	expectPrinted(t, "async function foo(){for await(x of y);}", "async function foo() {\n  for await (x of y)\n    ;\n}\n")
	expectPrinted(t, "async function foo(){for await(let x of y);}", "async function foo() {\n  for await (let x of y)\n    ;\n}\n")

	// Await as an identifier
	expectPrinted(t, "(function await() {})", "(function await() {\n});\n")
	expectPrinted(t, "function foo() { ({await} = x) }", "function foo() {\n  ({ await } = x);\n}\n")
	expectPrinted(t, "function foo() { let x = {await} }", "function foo() {\n  let x = { await };\n}\n")
	expectParseError(t, "({await} = x)", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")
	expectParseError(t, "let x = {await}", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")
	expectParseError(t, "class await {}", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")
	expectParseError(t, "(class await {})", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")
	expectParseError(t, "function await() {}", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")
	expectParseError(t, "async function foo() { ({await} = x) }", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")
	expectParseError(t, "async function foo() { let x = {await} }", "<stdin>: ERROR: Cannot use \"await\" as an identifier here:\n")

	// Await as a declaration
	expectPrinted(t, "({ async await() {} })", "({ async await() {\n} });\n")
	expectPrinted(t, "(class { async await() {} })", "(class {\n  async await() {\n  }\n});\n")
	expectPrinted(t, "class Foo { async await() {} }", "class Foo {\n  async await() {\n  }\n}\n")
	expectParseError(t, "async function await() {}", "<stdin>: ERROR: An async function cannot be named \"await\"\n")
	expectParseError(t, "(async function await() {})", "<stdin>: ERROR: An async function cannot be named \"await\"\n")

	// Await as a generator declaration
	expectPrinted(t, "({ async *await() {} })", "({ async *await() {\n} });\n")
	expectPrinted(t, "(class { async *await() {} })", "(class {\n  async *await() {\n  }\n});\n")
	expectPrinted(t, "class Foo { async *await() {} }", "class Foo {\n  async *await() {\n  }\n}\n")
	expectParseError(t, "async function* await() {}", "<stdin>: ERROR: An async function cannot be named \"await\"\n")
	expectParseError(t, "(async function* await() {})", "<stdin>: ERROR: An async function cannot be named \"await\"\n")
}

func TestLabels(t *testing.T) {
	expectPrinted(t, "{a:b}", "{\n  a:\n    b;\n}\n")
	expectPrinted(t, "({a:b})", "({ a: b });\n")

	expectParseError(t, "while (1) break x", "<stdin>: ERROR: There is no containing label named \"x\"\n")
	expectParseError(t, "while (1) continue x", "<stdin>: ERROR: There is no containing label named \"x\"\n")

	expectPrinted(t, "x: y: z: 1", "x:\n  y:\n    z:\n      1;\n")
	expectPrinted(t, "x: 1; y: 2; x: 3", "x:\n  1;\ny:\n  2;\nx:\n  3;\n")
	expectPrinted(t, "x: (() => { x: 1; })()", "x:\n  (() => {\n    x:\n      1;\n  })();\n")
	expectPrinted(t, "x: ({ f() { x: 1; } }).f()", "x:\n  ({ f() {\n    x:\n      1;\n  } }).f();\n")
	expectPrinted(t, "x: (function() { x: 1; })()", "x:\n  (function() {\n    x:\n      1;\n  })();\n")
	expectParseError(t, "x: y: x: 1", "<stdin>: ERROR: Duplicate label \"x\"\n<stdin>: NOTE: The original label \"x\" is here:\n")

	expectPrinted(t, "x: break x", "x:\n  break x;\n")
	expectPrinted(t, "x: { break x; foo() }", "x: {\n  break x;\n  foo();\n}\n")
	expectPrinted(t, "x: { y: { z: { foo(); break x; } } }", "x: {\n  y: {\n    z: {\n      foo();\n      break x;\n    }\n  }\n}\n")
	expectPrinted(t, "x: { class X { static { new X } } }", "x: {\n  class X {\n    static {\n      new X();\n    }\n  }\n}\n")
	expectPrintedMangle(t, "x: break x", "")
	expectPrintedMangle(t, "x: { break x; foo() }", "")
	expectPrintedMangle(t, "y: while (foo()) x: { break x; foo() }", "y:\n  for (; foo(); )\n    ;\n")
	expectPrintedMangle(t, "y: while (foo()) x: { break y; foo() }", "y:\n  for (; foo(); )\n    x:\n      break y;\n")
	expectPrintedMangle(t, "x: { y: { z: { foo(); break x; } } }", "x:\n  y:\n    z: {\n      foo();\n      break x;\n    }\n")
	expectPrintedMangle(t, "x: { class X { static { new X } } }", "x: {\n  class X {\n    static {\n      new X();\n    }\n  }\n}\n")
}

func TestArrow(t *testing.T) {
	expectParseError(t, "({a: b, c() {}}) => {}", "<stdin>: ERROR: Invalid binding pattern\n")
	expectParseError(t, "({a: b, get c() {}}) => {}", "<stdin>: ERROR: Invalid binding pattern\n")
	expectParseError(t, "({a: b, set c(x) {}}) => {}", "<stdin>: ERROR: Invalid binding pattern\n")

	expectParseError(t, "x = ([ (y) ]) => 0", "<stdin>: ERROR: Invalid binding pattern\n")
	expectParseError(t, "x = ([ ...(y) ]) => 0", "<stdin>: ERROR: Invalid binding pattern\n")
	expectParseError(t, "x = ({ (y) }) => 0", "<stdin>: ERROR: Expected identifier but found \"(\"\n")
	expectParseError(t, "x = ({ y: (z) }) => 0", "<stdin>: ERROR: Invalid binding pattern\n")
	expectParseError(t, "x = ({ ...(y) }) => 0", "<stdin>: ERROR: Invalid binding pattern\n")

	expectPrinted(t, "x = ([ y = [ (z) ] ]) => 0", "x = ([y = [z]]) => 0;\n")
	expectPrinted(t, "x = ([ y = [ ...(z) ] ]) => 0", "x = ([y = [...z]]) => 0;\n")
	expectPrinted(t, "x = ({ y = { y: (z) } }) => 0", "x = ({ y = { y: z } }) => 0;\n")
	expectPrinted(t, "x = ({ y = { ...(y) } }) => 0", "x = ({ y = { ...y } }) => 0;\n")

	expectPrinted(t, "x => function() {}", "(x) => function() {\n};\n")
	expectPrinted(t, "(x) => function() {}", "(x) => function() {\n};\n")
	expectPrinted(t, "(x => function() {})", "(x) => function() {\n};\n")

	expectPrinted(t, "(x = () => {}) => {}", "(x = () => {\n}) => {\n};\n")
	expectPrinted(t, "async (x = () => {}) => {}", "async (x = () => {\n}) => {\n};\n")

	expectParseError(t, "()\n=> {}", "<stdin>: ERROR: Unexpected newline before \"=>\"\n")
	expectParseError(t, "x\n=> {}", "<stdin>: ERROR: Unexpected newline before \"=>\"\n")
	expectParseError(t, "async x\n=> {}", "<stdin>: ERROR: Unexpected newline before \"=>\"\n")
	expectParseError(t, "async ()\n=> {}", "<stdin>: ERROR: Unexpected newline before \"=>\"\n")
	expectParseError(t, "(()\n=> {})", "<stdin>: ERROR: Unexpected newline before \"=>\"\n")
	expectParseError(t, "(x\n=> {})", "<stdin>: ERROR: Unexpected newline before \"=>\"\n")
	expectParseError(t, "(async x\n=> {})", "<stdin>: ERROR: Unexpected newline before \"=>\"\n")
	expectParseError(t, "(async ()\n=> {})", "<stdin>: ERROR: Unexpected newline before \"=>\"\n")

	expectPrinted(t, "(() => {}) ? a : b", "(() => {\n}) ? a : b;\n")
	expectPrintedMangle(t, "(() => {}) ? a : b", "a;\n")
	expectParseError(t, "() => {} ? a : b", "<stdin>: ERROR: Expected \";\" but found \"?\"\n")
	expectPrinted(t, "1 < (() => {})", "1 < (() => {\n});\n")
	expectParseError(t, "1 < () => {}", "<stdin>: ERROR: Unexpected \")\"\n")
	expectParseError(t, "(...x = y) => {}", "<stdin>: ERROR: A rest argument cannot have a default initializer\n")
	expectParseError(t, "([...x = y]) => {}", "<stdin>: ERROR: A rest argument cannot have a default initializer\n")

	// Can assign an arrow function
	expectPrinted(t, "y = x => {}", "y = (x) => {\n};\n")
	expectPrinted(t, "y = () => {}", "y = () => {\n};\n")
	expectPrinted(t, "y = (x) => {}", "y = (x) => {\n};\n")
	expectPrinted(t, "y = async x => {}", "y = async (x) => {\n};\n")
	expectPrinted(t, "y = async () => {}", "y = async () => {\n};\n")
	expectPrinted(t, "y = async (x) => {}", "y = async (x) => {\n};\n")

	// Cannot add an arrow function
	expectPrinted(t, "1 + function () {}", "1 + function() {\n};\n")
	expectPrinted(t, "1 + async function () {}", "1 + async function() {\n};\n")
	expectParseError(t, "1 + x => {}", "<stdin>: ERROR: Expected \";\" but found \"=>\"\n")
	expectParseError(t, "1 + () => {}", "<stdin>: ERROR: Unexpected \")\"\n")
	expectParseError(t, "1 + (x) => {}", "<stdin>: ERROR: Expected \";\" but found \"=>\"\n")
	expectParseError(t, "1 + async x => {}", "<stdin>: ERROR: Expected \";\" but found \"x\"\n")
	expectParseError(t, "1 + async () => {}", "<stdin>: ERROR: Unexpected \"=>\"\n")
	expectParseError(t, "1 + async (x) => {}", "<stdin>: ERROR: Unexpected \"=>\"\n")

	// Cannot extend an arrow function
	expectPrinted(t, "class Foo extends function () {} {}", "class Foo extends function() {\n} {\n}\n")
	expectPrinted(t, "class Foo extends async function () {} {}", "class Foo extends async function() {\n} {\n}\n")
	expectParseError(t, "class Foo extends x => {} {}", "<stdin>: ERROR: Expected \"{\" but found \"=>\"\n")
	expectParseError(t, "class Foo extends () => {} {}", "<stdin>: ERROR: Unexpected \")\"\n")
	expectParseError(t, "class Foo extends (x) => {} {}", "<stdin>: ERROR: Expected \"{\" but found \"=>\"\n")
	expectParseError(t, "class Foo extends async x => {} {}", "<stdin>: ERROR: Expected \"{\" but found \"x\"\n")
	expectParseError(t, "class Foo extends async () => {} {}", "<stdin>: ERROR: Unexpected \"=>\"\n")
	expectParseError(t, "class Foo extends async (x) => {} {}", "<stdin>: ERROR: Unexpected \"=>\"\n")
	expectParseError(t, "(class extends x => {} {})", "<stdin>: ERROR: Expected \"{\" but found \"=>\"\n")
	expectParseError(t, "(class extends () => {} {})", "<stdin>: ERROR: Unexpected \")\"\n")
	expectParseError(t, "(class extends (x) => {} {})", "<stdin>: ERROR: Expected \"{\" but found \"=>\"\n")
	expectParseError(t, "(class extends async x => {} {})", "<stdin>: ERROR: Expected \"{\" but found \"x\"\n")
	expectParseError(t, "(class extends async () => {} {})", "<stdin>: ERROR: Unexpected \"=>\"\n")
	expectParseError(t, "(class extends async (x) => {} {})", "<stdin>: ERROR: Unexpected \"=>\"\n")

	expectParseError(t, "() => {}(0)", "<stdin>: ERROR: Expected \";\" but found \"(\"\n")
	expectParseError(t, "x => {}(0)", "<stdin>: ERROR: Expected \";\" but found \"(\"\n")
	expectParseError(t, "async () => {}(0)", "<stdin>: ERROR: Expected \";\" but found \"(\"\n")
	expectParseError(t, "async x => {}(0)", "<stdin>: ERROR: Expected \";\" but found \"(\"\n")
	expectParseError(t, "async (x) => {}(0)", "<stdin>: ERROR: Expected \";\" but found \"(\"\n")
	expectParseError(t, "0, async () => {}(0)", "<stdin>: ERROR: Expected \";\" but found \"(\"\n")
	expectParseError(t, "0, async x => {}(0)", "<stdin>: ERROR: Expected \";\" but found \"(\"\n")
	expectParseError(t, "0, async (x) => {}(0)", "<stdin>: ERROR: Expected \";\" but found \"(\"\n")

	expectPrinted(t, "() => {}\n(0)", "() => {\n};\n0;\n")
	expectPrinted(t, "x => {}\n(0)", "(x) => {\n};\n0;\n")
	expectPrinted(t, "async () => {}\n(0)", "async () => {\n};\n0;\n")
	expectPrinted(t, "async x => {}\n(0)", "async (x) => {\n};\n0;\n")
	expectPrinted(t, "async (x) => {}\n(0)", "async (x) => {\n};\n0;\n")

	expectPrinted(t, "() => {}\n,0", "() => {\n}, 0;\n")
	expectPrinted(t, "x => {}\n,0", "(x) => {\n}, 0;\n")
	expectPrinted(t, "async () => {}\n,0", "async () => {\n}, 0;\n")
	expectPrinted(t, "async x => {}\n,0", "async (x) => {\n}, 0;\n")
	expectPrinted(t, "async (x) => {}\n,0", "async (x) => {\n}, 0;\n")

	expectPrinted(t, "(() => {})\n(0)", "/* @__PURE__ */ (() => {\n})(0);\n")
	expectPrinted(t, "(x => {})\n(0)", "/* @__PURE__ */ ((x) => {\n})(0);\n")
	expectPrinted(t, "(async () => {})\n(0)", "/* @__PURE__ */ (async () => {\n})(0);\n")
	expectPrinted(t, "(async x => {})\n(0)", "/* @__PURE__ */ (async (x) => {\n})(0);\n")
	expectPrinted(t, "(async (x) => {})\n(0)", "/* @__PURE__ */ (async (x) => {\n})(0);\n")

	expectParseError(t, "y = () => {}(0)", "<stdin>: ERROR: Expected \";\" but found \"(\"\n")
	expectParseError(t, "y = x => {}(0)", "<stdin>: ERROR: Expected \";\" but found \"(\"\n")
	expectParseError(t, "y = async () => {}(0)", "<stdin>: ERROR: Expected \";\" but found \"(\"\n")
	expectParseError(t, "y = async x => {}(0)", "<stdin>: ERROR: Expected \";\" but found \"(\"\n")
	expectParseError(t, "y = async (x) => {}(0)", "<stdin>: ERROR: Expected \";\" but found \"(\"\n")

	expectPrinted(t, "y = () => {}\n(0)", "y = () => {\n};\n0;\n")
	expectPrinted(t, "y = x => {}\n(0)", "y = (x) => {\n};\n0;\n")
	expectPrinted(t, "y = async () => {}\n(0)", "y = async () => {\n};\n0;\n")
	expectPrinted(t, "y = async x => {}\n(0)", "y = async (x) => {\n};\n0;\n")
	expectPrinted(t, "y = async (x) => {}\n(0)", "y = async (x) => {\n};\n0;\n")

	expectPrinted(t, "y = () => {}\n,0", "y = () => {\n}, 0;\n")
	expectPrinted(t, "y = x => {}\n,0", "y = (x) => {\n}, 0;\n")
	expectPrinted(t, "y = async () => {}\n,0", "y = async () => {\n}, 0;\n")
	expectPrinted(t, "y = async x => {}\n,0", "y = async (x) => {\n}, 0;\n")
	expectPrinted(t, "y = async (x) => {}\n,0", "y = async (x) => {\n}, 0;\n")

	expectPrinted(t, "y = (() => {})\n(0)", "y = /* @__PURE__ */ (() => {\n})(0);\n")
	expectPrinted(t, "y = (x => {})\n(0)", "y = /* @__PURE__ */ ((x) => {\n})(0);\n")
	expectPrinted(t, "y = (async () => {})\n(0)", "y = /* @__PURE__ */ (async () => {\n})(0);\n")
	expectPrinted(t, "y = (async x => {})\n(0)", "y = /* @__PURE__ */ (async (x) => {\n})(0);\n")
	expectPrinted(t, "y = (async (x) => {})\n(0)", "y = /* @__PURE__ */ (async (x) => {\n})(0);\n")

	expectParseError(t, "(() => {}(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")
	expectParseError(t, "(x => {}(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")
	expectParseError(t, "(async () => {}(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")
	expectParseError(t, "(async x => {}(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")
	expectParseError(t, "(async (x) => {}(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")

	expectParseError(t, "(() => {}\n(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")
	expectParseError(t, "(x => {}\n(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")
	expectParseError(t, "(async () => {}\n(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")
	expectParseError(t, "(async x => {}\n(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")
	expectParseError(t, "(async (x) => {}\n(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")

	expectPrinted(t, "(() => {}\n,0)", "() => {\n}, 0;\n")
	expectPrinted(t, "(x => {}\n,0)", "(x) => {\n}, 0;\n")
	expectPrinted(t, "(async () => {}\n,0)", "async () => {\n}, 0;\n")
	expectPrinted(t, "(async x => {}\n,0)", "async (x) => {\n}, 0;\n")
	expectPrinted(t, "(async (x) => {}\n,0)", "async (x) => {\n}, 0;\n")

	expectPrinted(t, "((() => {})\n(0))", "/* @__PURE__ */ (() => {\n})(0);\n")
	expectPrinted(t, "((x => {})\n(0))", "/* @__PURE__ */ ((x) => {\n})(0);\n")
	expectPrinted(t, "((async () => {})\n(0))", "/* @__PURE__ */ (async () => {\n})(0);\n")
	expectPrinted(t, "((async x => {})\n(0))", "/* @__PURE__ */ (async (x) => {\n})(0);\n")
	expectPrinted(t, "((async (x) => {})\n(0))", "/* @__PURE__ */ (async (x) => {\n})(0);\n")

	expectParseError(t, "y = (() => {}(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")
	expectParseError(t, "y = (x => {}(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")
	expectParseError(t, "y = (async () => {}(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")
	expectParseError(t, "y = (async x => {}(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")
	expectParseError(t, "y = (async (x) => {}(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")

	expectParseError(t, "y = (() => {}\n(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")
	expectParseError(t, "y = (x => {}\n(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")
	expectParseError(t, "y = (async () => {}\n(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")
	expectParseError(t, "y = (async x => {}\n(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")
	expectParseError(t, "y = (async (x) => {}\n(0))", "<stdin>: ERROR: Expected \")\" but found \"(\"\n")

	expectPrinted(t, "y = (() => {}\n,0)", "y = (() => {\n}, 0);\n")
	expectPrinted(t, "y = (x => {}\n,0)", "y = ((x) => {\n}, 0);\n")
	expectPrinted(t, "y = (async () => {}\n,0)", "y = (async () => {\n}, 0);\n")
	expectPrinted(t, "y = (async x => {}\n,0)", "y = (async (x) => {\n}, 0);\n")
	expectPrinted(t, "y = (async (x) => {}\n,0)", "y = (async (x) => {\n}, 0);\n")

	expectPrinted(t, "y = ((() => {})\n(0))", "y = /* @__PURE__ */ (() => {\n})(0);\n")
	expectPrinted(t, "y = ((x => {})\n(0))", "y = /* @__PURE__ */ ((x) => {\n})(0);\n")
	expectPrinted(t, "y = ((async () => {})\n(0))", "y = /* @__PURE__ */ (async () => {\n})(0);\n")
	expectPrinted(t, "y = ((async x => {})\n(0))", "y = /* @__PURE__ */ (async (x) => {\n})(0);\n")
	expectPrinted(t, "y = ((async (x) => {})\n(0))", "y = /* @__PURE__ */ (async (x) => {\n})(0);\n")
}

func TestTemplate(t *testing.T) {
	expectPrinted(t, "`\\0`", "`\\0`;\n")
	expectPrinted(t, "`${'\\00'}`", "`${\"\\0\"}`;\n")

	expectParseError(t, "`\\7`", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in template literals\n")
	expectParseError(t, "`\\8`", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in template literals\n")
	expectParseError(t, "`\\9`", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in template literals\n")
	expectParseError(t, "`\\00`", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in template literals\n")
	expectParseError(t, "`\\00${x}`", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in template literals\n")
	expectParseError(t, "`${x}\\00`", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in template literals\n")
	expectParseError(t, "`${x}\\00${y}`", "<stdin>: ERROR: Legacy octal escape sequences cannot be used in template literals\n")
	expectParseError(t, "`\\unicode`", "<stdin>: ERROR: Syntax error \"n\"\n")
	expectParseError(t, "`\\unicode${x}`", "<stdin>: ERROR: Syntax error \"n\"\n")
	expectParseError(t, "`${x}\\unicode`", "<stdin>: ERROR: Syntax error \"n\"\n")
	expectParseError(t, "`\\u{10FFFFF}`", "<stdin>: ERROR: Unicode escape sequence is out of range\n")

	expectPrinted(t, "tag`\\7`", "tag`\\7`;\n")
	expectPrinted(t, "tag`\\8`", "tag`\\8`;\n")
	expectPrinted(t, "tag`\\9`", "tag`\\9`;\n")
	expectPrinted(t, "tag`\\00`", "tag`\\00`;\n")
	expectPrinted(t, "tag`\\00${x}`", "tag`\\00${x}`;\n")
	expectPrinted(t, "tag`${x}\\00`", "tag`${x}\\00`;\n")
	expectPrinted(t, "tag`${x}\\00${y}`", "tag`${x}\\00${y}`;\n")
	expectPrinted(t, "tag`\\unicode`", "tag`\\unicode`;\n")
	expectPrinted(t, "tag`\\unicode${x}`", "tag`\\unicode${x}`;\n")
	expectPrinted(t, "tag`${x}\\unicode`", "tag`${x}\\unicode`;\n")
	expectPrinted(t, "tag`\\u{10FFFFF}`", "tag`\\u{10FFFFF}`;\n")

	expectPrinted(t, "tag``", "tag``;\n")
	expectPrinted(t, "(a?.b)``", "(a?.b)``;\n")
	expectPrinted(t, "(a?.(b))``", "(a?.(b))``;\n")
	expectPrinted(t, "(a?.[b])``", "(a?.[b])``;\n")
	expectPrinted(t, "(a?.b.c)``", "(a?.b.c)``;\n")
	expectPrinted(t, "(a?.(b).c)``", "(a?.(b).c)``;\n")
	expectPrinted(t, "(a?.[b].c)``", "(a?.[b].c)``;\n")

	expectParseError(t, "a?.b``", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.(b)``", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.[b]``", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.b.c``", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.(b).c``", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.[b].c``", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")

	expectParseError(t, "a?.b`${d}`", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.(b)`${d}`", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.[b]`${d}`", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.b.c`${d}`", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.(b).c`${d}`", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.[b].c`${d}`", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")

	expectParseError(t, "a?.b\n``", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.(b)\n``", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.[b]\n``", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.b.c\n``", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.(b).c\n``", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.[b].c\n``", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")

	expectParseError(t, "a?.b\n`${d}`", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.(b)\n`${d}`", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.[b]\n`${d}`", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.b.c\n`${d}`", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.(b).c\n`${d}`", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")
	expectParseError(t, "a?.[b].c\n`${d}`", "<stdin>: ERROR: Template literals cannot have an optional chain as a tag\n")

	expectPrinted(t, "`a${1 + `b${2}c` + 3}d`", "`a${`1b${2}c3`}d`;\n")
	expectPrintedMangle(t, "x = `a${1 + `b${2}c` + 3}d`", "x = `a1b2c3d`;\n")

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
	expectParseError(t, "switch (x) { default: default: }", "<stdin>: ERROR: Multiple default clauses are not allowed\n")
}

func TestConstantFolding(t *testing.T) {
	expectPrinted(t, "x = !false", "x = true;\n")
	expectPrinted(t, "x = !true", "x = false;\n")

	expectPrinted(t, "x = !!0", "x = false;\n")
	expectPrinted(t, "x = !!-0", "x = false;\n")
	expectPrinted(t, "x = !!1", "x = true;\n")
	expectPrinted(t, "x = !!NaN", "x = false;\n")
	expectPrinted(t, "x = !!Infinity", "x = true;\n")
	expectPrinted(t, "x = !!-Infinity", "x = true;\n")
	expectPrinted(t, "x = !!\"\"", "x = false;\n")
	expectPrinted(t, "x = !!\"x\"", "x = true;\n")
	expectPrinted(t, "x = !!function() {}", "x = true;\n")
	expectPrinted(t, "x = !!(() => {})", "x = true;\n")
	expectPrinted(t, "x = !!0n", "x = false;\n")
	expectPrinted(t, "x = !!1n", "x = true;\n")

	expectPrinted(t, "x = 1 ? a : b", "x = 1 ? a : b;\n")
	expectPrinted(t, "x = 0 ? a : b", "x = 0 ? a : b;\n")
	expectPrintedMangle(t, "x = 1 ? a : b", "x = a;\n")
	expectPrintedMangle(t, "x = 0 ? a : b", "x = b;\n")

	expectPrinted(t, "x = 1 && 2", "x = 2;\n")
	expectPrinted(t, "x = 1 || 2", "x = 1;\n")
	expectPrinted(t, "x = 0 && 1", "x = 0;\n")
	expectPrinted(t, "x = 0 || 1", "x = 1;\n")

	expectPrinted(t, "x = null ?? 1", "x = 1;\n")
	expectPrinted(t, "x = undefined ?? 1", "x = 1;\n")
	expectPrinted(t, "x = 0 ?? 1", "x = 0;\n")
	expectPrinted(t, "x = false ?? 1", "x = false;\n")
	expectPrinted(t, "x = \"\" ?? 1", "x = \"\";\n")

	expectPrinted(t, "x = typeof undefined", "x = \"undefined\";\n")
	expectPrinted(t, "x = typeof null", "x = \"object\";\n")
	expectPrinted(t, "x = typeof false", "x = \"boolean\";\n")
	expectPrinted(t, "x = typeof true", "x = \"boolean\";\n")
	expectPrinted(t, "x = typeof 123", "x = \"number\";\n")
	expectPrinted(t, "x = typeof 123n", "x = \"bigint\";\n")
	expectPrinted(t, "x = typeof 'abc'", "x = \"string\";\n")
	expectPrinted(t, "x = typeof function() {}", "x = \"function\";\n")
	expectPrinted(t, "x = typeof (() => {})", "x = \"function\";\n")
	expectPrinted(t, "x = typeof {}", "x = typeof {};\n")
	expectPrinted(t, "x = typeof []", "x = typeof [];\n")

	expectPrinted(t, "x = undefined === undefined", "x = true;\n")
	expectPrinted(t, "x = undefined !== undefined", "x = false;\n")
	expectPrinted(t, "x = undefined == undefined", "x = true;\n")
	expectPrinted(t, "x = undefined != undefined", "x = false;\n")

	expectPrinted(t, "x = null === null", "x = true;\n")
	expectPrinted(t, "x = null !== null", "x = false;\n")
	expectPrinted(t, "x = null == null", "x = true;\n")
	expectPrinted(t, "x = null != null", "x = false;\n")

	expectPrinted(t, "x = null === undefined", "x = false;\n")
	expectPrinted(t, "x = null !== undefined", "x = true;\n")
	expectPrinted(t, "x = null == undefined", "x = true;\n")
	expectPrinted(t, "x = null != undefined", "x = false;\n")

	expectPrinted(t, "x = undefined === null", "x = false;\n")
	expectPrinted(t, "x = undefined !== null", "x = true;\n")
	expectPrinted(t, "x = undefined == null", "x = true;\n")
	expectPrinted(t, "x = undefined != null", "x = false;\n")

	expectPrinted(t, "x = true === true", "x = true;\n")
	expectPrinted(t, "x = true === false", "x = false;\n")
	expectPrinted(t, "x = true !== true", "x = false;\n")
	expectPrinted(t, "x = true !== false", "x = true;\n")
	expectPrinted(t, "x = true == true", "x = true;\n")
	expectPrinted(t, "x = true == false", "x = false;\n")
	expectPrinted(t, "x = true != true", "x = false;\n")
	expectPrinted(t, "x = true != false", "x = true;\n")

	expectPrinted(t, "x = 1 === 1", "x = true;\n")
	expectPrinted(t, "x = 1 === 2", "x = false;\n")
	expectPrinted(t, "x = 1 === '1'", "x = 1 === \"1\";\n")
	expectPrinted(t, "x = 1 == 1", "x = true;\n")
	expectPrinted(t, "x = 1 == 2", "x = false;\n")
	expectPrinted(t, "x = 1 == '1'", "x = 1 == \"1\";\n")

	expectPrinted(t, "x = 1 !== 1", "x = false;\n")
	expectPrinted(t, "x = 1 !== 2", "x = true;\n")
	expectPrinted(t, "x = 1 !== '1'", "x = 1 !== \"1\";\n")
	expectPrinted(t, "x = 1 != 1", "x = false;\n")
	expectPrinted(t, "x = 1 != 2", "x = true;\n")
	expectPrinted(t, "x = 1 != '1'", "x = 1 != \"1\";\n")

	expectPrinted(t, "x = 'a' === '\\x61'", "x = true;\n")
	expectPrinted(t, "x = 'a' === '\\x62'", "x = false;\n")
	expectPrinted(t, "x = 'a' === 'abc'", "x = false;\n")
	expectPrinted(t, "x = 'a' !== '\\x61'", "x = false;\n")
	expectPrinted(t, "x = 'a' !== '\\x62'", "x = true;\n")
	expectPrinted(t, "x = 'a' !== 'abc'", "x = true;\n")
	expectPrinted(t, "x = 'a' == '\\x61'", "x = true;\n")
	expectPrinted(t, "x = 'a' == '\\x62'", "x = false;\n")
	expectPrinted(t, "x = 'a' == 'abc'", "x = false;\n")
	expectPrinted(t, "x = 'a' != '\\x61'", "x = false;\n")
	expectPrinted(t, "x = 'a' != '\\x62'", "x = true;\n")
	expectPrinted(t, "x = 'a' != 'abc'", "x = true;\n")

	expectPrinted(t, "x = 'a' + 'b'", "x = \"ab\";\n")
	expectPrinted(t, "x = 'a' + 'bc'", "x = \"abc\";\n")
	expectPrinted(t, "x = 'ab' + 'c'", "x = \"abc\";\n")
	expectPrinted(t, "x = x + 'a' + 'b'", "x = x + \"ab\";\n")
	expectPrinted(t, "x = x + 'a' + 'bc'", "x = x + \"abc\";\n")
	expectPrinted(t, "x = x + 'ab' + 'c'", "x = x + \"abc\";\n")
	expectPrinted(t, "x = 'a' + 1", "x = \"a1\";\n")
	expectPrinted(t, "x = x * 'a' + 'b'", "x = x * \"a\" + \"b\";\n")

	expectPrinted(t, "x = 'string' + `template`", "x = `stringtemplate`;\n")
	expectPrinted(t, "x = 'string' + `a${foo}b`", "x = `stringa${foo}b`;\n")
	expectPrinted(t, "x = 'string' + tag`template`", "x = \"string\" + tag`template`;\n")
	expectPrinted(t, "x = `template` + 'string'", "x = `templatestring`;\n")
	expectPrinted(t, "x = `a${foo}b` + 'string'", "x = `a${foo}bstring`;\n")
	expectPrinted(t, "x = tag`template` + 'string'", "x = tag`template` + \"string\";\n")
	expectPrinted(t, "x = `template` + `a${foo}b`", "x = `templatea${foo}b`;\n")
	expectPrinted(t, "x = `a${foo}b` + `template`", "x = `a${foo}btemplate`;\n")
	expectPrinted(t, "x = `a${foo}b` + `x${bar}y`", "x = `a${foo}bx${bar}y`;\n")
	expectPrinted(t, "x = `a${i}${j}bb` + `xxx${bar}yyyy`", "x = `a${i}${j}bbxxx${bar}yyyy`;\n")
	expectPrinted(t, "x = `a${foo}bb` + `xxx${i}${j}yyyy`", "x = `a${foo}bbxxx${i}${j}yyyy`;\n")
	expectPrinted(t, "x = `template` + tag`template2`", "x = `template` + tag`template2`;\n")
	expectPrinted(t, "x = tag`template` + `template2`", "x = tag`template` + `template2`;\n")

	expectPrinted(t, "x = 123", "x = 123;\n")
	expectPrinted(t, "x = 123 .toString()", "x = 123 .toString();\n")
	expectPrinted(t, "x = -123", "x = -123;\n")
	expectPrinted(t, "x = (-123).toString()", "x = (-123).toString();\n")
	expectPrinted(t, "x = -0", "x = -0;\n")
	expectPrinted(t, "x = (-0).toString()", "x = (-0).toString();\n")
	expectPrinted(t, "x = -0 === 0", "x = true;\n")

	expectPrinted(t, "x = NaN", "x = NaN;\n")
	expectPrinted(t, "x = NaN.toString()", "x = NaN.toString();\n")
	expectPrinted(t, "x = NaN === NaN", "x = false;\n")

	expectPrinted(t, "x = Infinity", "x = Infinity;\n")
	expectPrinted(t, "x = Infinity.toString()", "x = Infinity.toString();\n")
	expectPrinted(t, "x = (-Infinity).toString()", "x = (-Infinity).toString();\n")
	expectPrinted(t, "x = Infinity === Infinity", "x = true;\n")
	expectPrinted(t, "x = Infinity === -Infinity", "x = false;\n")

	expectPrinted(t, "x = 123n === 1_2_3n", "x = true;\n")

	// We support folding strings from sibling AST nodes since that ends up being
	// equivalent with string addition. For example, "(x + 'a') + 'b'" is the
	// same as "x + 'ab'". However, this is not true for numbers. We can't turn
	// "(x + 1) + '2'" into "x + '12'". These tests check for this edge case.
	expectPrinted(t, "x = 'a' + 'b' + y", "x = \"ab\" + y;\n")
	expectPrinted(t, "x = y + 'a' + 'b'", "x = y + \"ab\";\n")
	expectPrinted(t, "x = '3' + 4 + y", "x = \"34\" + y;\n")
	expectPrinted(t, "x = y + 4 + '5'", "x = y + 4 + \"5\";\n")
	expectPrinted(t, "x = '3' + 4 + 5", "x = \"345\";\n")
	expectPrinted(t, "x = 3 + 4 + '5'", "x = 3 + 4 + \"5\";\n")

	expectPrinted(t, "x = null == 0", "x = false;\n")
	expectPrinted(t, "x = 0 == null", "x = false;\n")
	expectPrinted(t, "x = undefined == 0", "x = false;\n")
	expectPrinted(t, "x = 0 == undefined", "x = false;\n")

	expectPrinted(t, "x = null == NaN", "x = false;\n")
	expectPrinted(t, "x = NaN == null", "x = false;\n")
	expectPrinted(t, "x = undefined == NaN", "x = false;\n")
	expectPrinted(t, "x = NaN == undefined", "x = false;\n")

	expectPrinted(t, "x = null == ''", "x = false;\n")
	expectPrinted(t, "x = '' == null", "x = false;\n")
	expectPrinted(t, "x = undefined == ''", "x = false;\n")
	expectPrinted(t, "x = '' == undefined", "x = false;\n")

	expectPrinted(t, "x = null == 'null'", "x = false;\n")
	expectPrinted(t, "x = 'null' == null", "x = false;\n")
	expectPrinted(t, "x = undefined == 'undefined'", "x = false;\n")
	expectPrinted(t, "x = 'undefined' == undefined", "x = false;\n")

	expectPrinted(t, "x = false === 0", "x = false;\n")
	expectPrinted(t, "x = true === 1", "x = false;\n")
	expectPrinted(t, "x = false == 0", "x = true;\n")
	expectPrinted(t, "x = false == -0", "x = true;\n")
	expectPrinted(t, "x = true == 1", "x = true;\n")
	expectPrinted(t, "x = true == 2", "x = false;\n")

	expectPrinted(t, "x = 0 === false", "x = false;\n")
	expectPrinted(t, "x = 1 === true", "x = false;\n")
	expectPrinted(t, "x = 0 == false", "x = true;\n")
	expectPrinted(t, "x = -0 == false", "x = true;\n")
	expectPrinted(t, "x = 1 == true", "x = true;\n")
	expectPrinted(t, "x = 2 == true", "x = false;\n")
}

func TestConstantFoldingScopes(t *testing.T) {
	// Parsing will crash if somehow the scope traversal is misaligned between
	// the parsing and binding passes. This checks for those cases.
	expectPrintedMangle(t, "x; 1 ? 0 : ()=>{}; (()=>{})()", "x;\n")
	expectPrintedMangle(t, "x; 0 ? ()=>{} : 1; (()=>{})()", "x;\n")
	expectPrinted(t, "x; 0 && (()=>{}); (()=>{})()", "x;\n/* @__PURE__ */ (() => {\n})();\n")
	expectPrinted(t, "x; 1 || (()=>{}); (()=>{})()", "x;\n/* @__PURE__ */ (() => {\n})();\n")
	expectPrintedMangle(t, "if (1) 0; else ()=>{}; (()=>{})()", "")
	expectPrintedMangle(t, "if (0) ()=>{}; else 1; (()=>{})()", "")
}

func TestImport(t *testing.T) {
	expectPrinted(t, "import \"foo\"", "import \"foo\";\n")
	expectPrinted(t, "import {} from \"foo\"", "import {} from \"foo\";\n")
	expectPrinted(t, "import {x} from \"foo\";x", "import { x } from \"foo\";\nx;\n")
	expectPrinted(t, "import {x as y} from \"foo\";y", "import { x as y } from \"foo\";\ny;\n")
	expectPrinted(t, "import {x as y, z} from \"foo\";y;z", "import { x as y, z } from \"foo\";\ny;\nz;\n")
	expectPrinted(t, "import {x as y, z,} from \"foo\";y;z", "import { x as y, z } from \"foo\";\ny;\nz;\n")
	expectPrinted(t, "import z, {x as y} from \"foo\";y;z", "import z, { x as y } from \"foo\";\ny;\nz;\n")
	expectPrinted(t, "import z from \"foo\";z", "import z from \"foo\";\nz;\n")
	expectPrinted(t, "import * as ns from \"foo\";ns;ns.x", "import * as ns from \"foo\";\nns;\nns.x;\n")
	expectPrinted(t, "import z, * as ns from \"foo\";z;ns;ns.x", "import z, * as ns from \"foo\";\nz;\nns;\nns.x;\n")

	expectParseError(t, "import * from \"foo\"", "<stdin>: ERROR: Expected \"as\" but found \"from\"\n")

	expectPrinted(t, "import('foo')", "import(\"foo\");\n")
	expectPrinted(t, "(import('foo'))", "import(\"foo\");\n")
	expectPrinted(t, "{import('foo')}", "{\n  import(\"foo\");\n}\n")
	expectPrinted(t, "import('foo').then(() => {})", "import(\"foo\").then(() => {\n});\n")
	expectPrinted(t, "new import.meta", "new import.meta();\n")
	expectPrinted(t, "new (import('foo'))", "new (import(\"foo\"))();\n")
	expectParseError(t, "import()", "<stdin>: ERROR: Unexpected \")\"\n")
	expectParseError(t, "import(...a)", "<stdin>: ERROR: Unexpected \"...\"\n")
	expectParseError(t, "new import('foo')", "<stdin>: ERROR: Cannot use an \"import\" expression here without parentheses:\n")

	expectPrinted(t, "import.meta", "import.meta;\n")
	expectPrinted(t, "(import.meta)", "import.meta;\n")
	expectPrinted(t, "{import.meta}", "{\n  import.meta;\n}\n")

	expectPrinted(t, "import x from \"foo\"; x = 1", "import x from \"foo\";\nx = 1;\n")
	expectPrinted(t, "import x from \"foo\"; x++", "import x from \"foo\";\nx++;\n")
	expectPrinted(t, "import x from \"foo\"; ([x] = 1)", "import x from \"foo\";\n[x] = 1;\n")
	expectPrinted(t, "import x from \"foo\"; ({x} = 1)", "import x from \"foo\";\n({ x } = 1);\n")
	expectPrinted(t, "import x from \"foo\"; ({y: x} = 1)", "import x from \"foo\";\n({ y: x } = 1);\n")
	expectPrinted(t, "import {x} from \"foo\"; x++", "import { x } from \"foo\";\nx++;\n")
	expectPrinted(t, "import * as x from \"foo\"; x++", "import * as x from \"foo\";\nx++;\n")
	expectPrinted(t, "import * as x from \"foo\"; x.y = 1", "import * as x from \"foo\";\nx.y = 1;\n")
	expectPrinted(t, "import * as x from \"foo\"; x[y] = 1", "import * as x from \"foo\";\nx[y] = 1;\n")
	expectPrinted(t, "import * as x from \"foo\"; x['y'] = 1", "import * as x from \"foo\";\nx[\"y\"] = 1;\n")
	expectPrinted(t, "import * as x from \"foo\"; x['y z'] = 1", "import * as x from \"foo\";\nx[\"y z\"] = 1;\n")
	expectPrinted(t, "import x from \"foo\"; ({y = x} = 1)", "import x from \"foo\";\n({ y = x } = 1);\n")
	expectPrinted(t, "import x from \"foo\"; ({[x]: y} = 1)", "import x from \"foo\";\n({ [x]: y } = 1);\n")
	expectPrinted(t, "import x from \"foo\"; x.y = 1", "import x from \"foo\";\nx.y = 1;\n")
	expectPrinted(t, "import x from \"foo\"; x[y] = 1", "import x from \"foo\";\nx[y] = 1;\n")
	expectPrinted(t, "import x from \"foo\"; x['y'] = 1", "import x from \"foo\";\nx[\"y\"] = 1;\n")

	// "eval" and "arguments" are forbidden import names
	expectParseError(t, "import {eval} from 'foo'", "<stdin>: ERROR: Cannot use \"eval\" as an identifier here:\n")
	expectParseError(t, "import {ev\\u0061l} from 'foo'", "<stdin>: ERROR: Cannot use \"eval\" as an identifier here:\n")
	expectParseError(t, "import {x as eval} from 'foo'", "<stdin>: ERROR: Cannot use \"eval\" as an identifier here:\n")
	expectParseError(t, "import {x as ev\\u0061l} from 'foo'", "<stdin>: ERROR: Cannot use \"eval\" as an identifier here:\n")
	expectPrinted(t, "import {eval as x} from 'foo'", "import { eval as x } from \"foo\";\n")
	expectPrinted(t, "import {ev\\u0061l as x} from 'foo'", "import { eval as x } from \"foo\";\n")
	expectParseError(t, "import {arguments} from 'foo'", "<stdin>: ERROR: Cannot use \"arguments\" as an identifier here:\n")
	expectParseError(t, "import {\\u0061rguments} from 'foo'", "<stdin>: ERROR: Cannot use \"arguments\" as an identifier here:\n")
	expectParseError(t, "import {x as arguments} from 'foo'", "<stdin>: ERROR: Cannot use \"arguments\" as an identifier here:\n")
	expectParseError(t, "import {x as \\u0061rguments} from 'foo'", "<stdin>: ERROR: Cannot use \"arguments\" as an identifier here:\n")
	expectPrinted(t, "import {arguments as x} from 'foo'", "import { arguments as x } from \"foo\";\n")
	expectPrinted(t, "import {\\u0061rguments as x} from 'foo'", "import { arguments as x } from \"foo\";\n")

	// String import alias with "import {} from"
	expectPrinted(t, "import {'' as x} from 'foo'", "import { \"\" as x } from \"foo\";\n")
	expectPrinted(t, "import {'🍕' as x} from 'foo'", "import { \"🍕\" as x } from \"foo\";\n")
	expectPrinted(t, "import {'a b' as x} from 'foo'", "import { \"a b\" as x } from \"foo\";\n")
	expectPrinted(t, "import {'\\uD800\\uDC00' as x} from 'foo'", "import { 𐀀 as x } from \"foo\";\n")
	expectParseError(t, "import {'x'} from 'foo'", "<stdin>: ERROR: Expected \"as\" but found \"}\"\n")
	expectParseError(t, "import {'\\uD800' as x} from 'foo'",
		"<stdin>: ERROR: This import alias is invalid because it contains the unpaired Unicode surrogate U+D800\n")
	expectParseError(t, "import {'\\uDC00' as x} from 'foo'",
		"<stdin>: ERROR: This import alias is invalid because it contains the unpaired Unicode surrogate U+DC00\n")
	expectParseErrorTarget(t, 2020, "import {'' as x} from 'foo'",
		"<stdin>: ERROR: Using a string as a module namespace identifier name is not supported in the configured target environment\n")

	// String import alias with "import * as"
	expectParseError(t, "import * as '' from 'foo'", "<stdin>: ERROR: Expected identifier but found \"''\"\n")
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
	expectPrinted(t, "let x; export {x}", "let x;\nexport { x };\n")
	expectPrinted(t, "let x; export {x as y}", "let x;\nexport { x as y };\n")
	expectPrinted(t, "let x, z; export {x as y, z}", "let x, z;\nexport { x as y, z };\n")
	expectPrinted(t, "let x, z; export {x as y, z,}", "let x, z;\nexport { x as y, z };\n")
	expectPrinted(t, "let x; export {x} from \"foo\"", "let x;\nexport { x } from \"foo\";\n")
	expectPrinted(t, "let x; export {x as y} from \"foo\"", "let x;\nexport { x as y } from \"foo\";\n")
	expectPrinted(t, "let x, z; export {x as y, z} from \"foo\"", "let x, z;\nexport { x as y, z } from \"foo\";\n")
	expectPrinted(t, "let x, z; export {x as y, z,} from \"foo\"", "let x, z;\nexport { x as y, z } from \"foo\";\n")

	expectParseError(t, "export x from \"foo\"", "<stdin>: ERROR: Unexpected \"x\"\n")
	expectParseError(t, "export async", "<stdin>: ERROR: Expected \"function\" but found end of file\n")
	expectParseError(t, "export async function", "<stdin>: ERROR: Expected identifier but found end of file\n")
	expectParseError(t, "export async () => {}", "<stdin>: ERROR: Expected \"function\" but found \"(\"\n")
	expectParseError(t, "export var", "<stdin>: ERROR: Expected identifier but found end of file\n")
	expectParseError(t, "export let", "<stdin>: ERROR: Expected identifier but found end of file\n")
	expectParseError(t, "export const", "<stdin>: ERROR: Expected identifier but found end of file\n")

	// String export alias with "export {}"
	expectPrinted(t, "let x; export {x as ''}", "let x;\nexport { x as \"\" };\n")
	expectPrinted(t, "let x; export {x as '🍕'}", "let x;\nexport { x as \"🍕\" };\n")
	expectPrinted(t, "let x; export {x as 'a b'}", "let x;\nexport { x as \"a b\" };\n")
	expectPrinted(t, "let x; export {x as '\\uD800\\uDC00'}", "let x;\nexport { x as 𐀀 };\n")
	expectParseError(t, "let x; export {'x'}", "<stdin>: ERROR: Expected identifier but found \"'x'\"\n")
	expectParseError(t, "let x; export {'x' as 'y'}", "<stdin>: ERROR: Expected identifier but found \"'x'\"\n")
	expectParseError(t, "let x; export {x as '\\uD800'}",
		"<stdin>: ERROR: This export alias is invalid because it contains the unpaired Unicode surrogate U+D800\n")
	expectParseError(t, "let x; export {x as '\\uDC00'}",
		"<stdin>: ERROR: This export alias is invalid because it contains the unpaired Unicode surrogate U+DC00\n")
	expectParseErrorTarget(t, 2020, "let x; export {x as ''}",
		"<stdin>: ERROR: Using a string as a module namespace identifier name is not supported in the configured target environment\n")

	// String import alias with "export {} from"
	expectPrinted(t, "export {'' as x} from 'foo'", "export { \"\" as x } from \"foo\";\n")
	expectPrinted(t, "export {'🍕' as x} from 'foo'", "export { \"🍕\" as x } from \"foo\";\n")
	expectPrinted(t, "export {'a b' as x} from 'foo'", "export { \"a b\" as x } from \"foo\";\n")
	expectPrinted(t, "export {'\\uD800\\uDC00' as x} from 'foo'", "export { 𐀀 as x } from \"foo\";\n")
	expectParseError(t, "export {'\\uD800' as x} from 'foo'",
		"<stdin>: ERROR: This export alias is invalid because it contains the unpaired Unicode surrogate U+D800\n")
	expectParseError(t, "export {'\\uDC00' as x} from 'foo'",
		"<stdin>: ERROR: This export alias is invalid because it contains the unpaired Unicode surrogate U+DC00\n")
	expectParseErrorTarget(t, 2020, "export {'' as x} from 'foo'",
		"<stdin>: ERROR: Using a string as a module namespace identifier name is not supported in the configured target environment\n")

	// String export alias with "export {} from"
	expectPrinted(t, "export {x as ''} from 'foo'", "export { x as \"\" } from \"foo\";\n")
	expectPrinted(t, "export {x as '🍕'} from 'foo'", "export { x as \"🍕\" } from \"foo\";\n")
	expectPrinted(t, "export {x as 'a b'} from 'foo'", "export { x as \"a b\" } from \"foo\";\n")
	expectPrinted(t, "export {x as '\\uD800\\uDC00'} from 'foo'", "export { x as 𐀀 } from \"foo\";\n")
	expectParseError(t, "export {x as '\\uD800'} from 'foo'",
		"<stdin>: ERROR: This export alias is invalid because it contains the unpaired Unicode surrogate U+D800\n")
	expectParseError(t, "export {x as '\\uDC00'} from 'foo'",
		"<stdin>: ERROR: This export alias is invalid because it contains the unpaired Unicode surrogate U+DC00\n")
	expectParseErrorTarget(t, 2020, "export {x as ''} from 'foo'",
		"<stdin>: ERROR: Using a string as a module namespace identifier name is not supported in the configured target environment\n")

	// String import and export alias with "export {} from"
	expectPrinted(t, "export {'x'} from 'foo'", "export { x } from \"foo\";\n")
	expectPrinted(t, "export {'a b'} from 'foo'", "export { \"a b\" } from \"foo\";\n")
	expectPrinted(t, "export {'x' as 'y'} from 'foo'", "export { x as y } from \"foo\";\n")
	expectPrinted(t, "export {'a b' as 'c d'} from 'foo'", "export { \"a b\" as \"c d\" } from \"foo\";\n")

	// String export alias with "export * as"
	expectPrinted(t, "export * as '' from 'foo'", "export * as \"\" from \"foo\";\n")
	expectPrinted(t, "export * as '🍕' from 'foo'", "export * as \"🍕\" from \"foo\";\n")
	expectPrinted(t, "export * as 'a b' from 'foo'", "export * as \"a b\" from \"foo\";\n")
	expectPrinted(t, "export * as '\\uD800\\uDC00' from 'foo'", "export * as 𐀀 from \"foo\";\n")
	expectParseError(t, "export * as '\\uD800' from 'foo'",
		"<stdin>: ERROR: This export alias is invalid because it contains the unpaired Unicode surrogate U+D800\n")
	expectParseError(t, "export * as '\\uDC00' from 'foo'",
		"<stdin>: ERROR: This export alias is invalid because it contains the unpaired Unicode surrogate U+DC00\n")
	expectParseErrorTarget(t, 2020, "export * as '' from 'foo'",
		"<stdin>: ERROR: Using a string as a module namespace identifier name is not supported in the configured target environment\n")
}

func TestExportDuplicates(t *testing.T) {
	expectPrinted(t, "export {x};let x", "export { x };\nlet x;\n")
	expectPrinted(t, "export {x, x as y};let x", "export { x, x as y };\nlet x;\n")
	expectPrinted(t, "export {x};export {x as y} from 'foo';let x", "export { x };\nexport { x as y } from \"foo\";\nlet x;\n")
	expectPrinted(t, "export {x};export default function x() {}", "export { x };\nexport default function x() {\n}\n")
	expectPrinted(t, "export {x};export default class x {}", "export { x };\nexport default class x {\n}\n")

	errorTextX := `<stdin>: ERROR: Multiple exports with the same name "x"
<stdin>: NOTE: The name "x" was originally exported here:
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

	errorTextDefault := `<stdin>: ERROR: Multiple exports with the same name "default"
<stdin>: NOTE: The name "default" was originally exported here:
`

	expectParseError(t, "export {x as default};let x;export default 0", errorTextDefault)
	expectParseError(t, "export {x as default};let x;export default function() {}", errorTextDefault)
	expectParseError(t, "export {x as default};let x;export default class {}", errorTextDefault)
	expectParseError(t, "export {x as default};export default function x() {}", errorTextDefault)
	expectParseError(t, "export {x as default};export default class x {}", errorTextDefault)
}

func TestExportDefault(t *testing.T) {
	expectParseError(t, "export default 1, 2", "<stdin>: ERROR: Expected \";\" but found \",\"\n")
	expectPrinted(t, "export default (1, 2)", "export default (1, 2);\n")

	expectParseError(t, "export default async, 0", "<stdin>: ERROR: Expected \";\" but found \",\"\n")
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
	expectPrinted(t, "export {x, y};let x, y", "export { x, y };\nlet x, y;\n")
	expectPrinted(t, "export {x, y as z,};let x, y", "export { x, y as z };\nlet x, y;\n")
	expectPrinted(t, "export {x, y} from 'path'", "export { x, y } from \"path\";\n")
	expectPrinted(t, "export {default, if} from 'path'", "export { default, if } from \"path\";\n")
	expectPrinted(t, "export {default as foo, if as bar} from 'path'", "export { default as foo, if as bar } from \"path\";\n")
	expectParseError(t, "export {default}", "<stdin>: ERROR: Expected identifier but found \"default\"\n")
	expectParseError(t, "export {default as foo}", "<stdin>: ERROR: Expected identifier but found \"default\"\n")
	expectParseError(t, "export {if}", "<stdin>: ERROR: Expected identifier but found \"if\"\n")
	expectParseError(t, "export {if as foo}", "<stdin>: ERROR: Expected identifier but found \"if\"\n")
}

func TestCatch(t *testing.T) {
	expectPrinted(t, "try {} catch (e) {}", "try {\n} catch (e) {\n}\n")
	expectPrinted(t, "try {} catch (e) { var e }", "try {\n} catch (e) {\n  var e;\n}\n")
	expectPrinted(t, "var e; try {} catch (e) {}", "var e;\ntry {\n} catch (e) {\n}\n")
	expectPrinted(t, "let e; try {} catch (e) {}", "let e;\ntry {\n} catch (e) {\n}\n")
	expectPrinted(t, "try { var e } catch (e) {}", "try {\n  var e;\n} catch (e) {\n}\n")
	expectPrinted(t, "try { function e() {} } catch (e) {}", "try {\n  let e = function() {\n  };\n  var e = e;\n} catch (e) {\n}\n")
	expectPrinted(t, "try {} catch (e) { { function e() {} } }", "try {\n} catch (e) {\n  {\n    let e = function() {\n    };\n    var e = e;\n  }\n}\n")
	expectPrinted(t, "try {} catch (e) { if (1) function e() {} }", "try {\n} catch (e) {\n  if (1) {\n    let e = function() {\n    };\n    var e = e;\n  }\n}\n")
	expectPrinted(t, "try {} catch (e) { if (0) ; else function e() {} }", "try {\n} catch (e) {\n  if (0)\n    ;\n  else {\n    let e = function() {\n    };\n    var e = e;\n  }\n}\n")
	expectPrinted(t, "try {} catch ({ e }) { { function e() {} } }", "try {\n} catch ({ e }) {\n  {\n    let e = function() {\n    };\n    var e = e;\n  }\n}\n")

	errorText := `<stdin>: ERROR: The symbol "e" has already been declared
<stdin>: NOTE: The symbol "e" was originally declared here:
`

	expectParseError(t, "try {} catch (e) { function e() {} }", errorText)
	expectParseError(t, "try {} catch ({ e }) { var e }", errorText)
	expectParseError(t, "try {} catch ({ e }) { { var e } }", errorText)
	expectParseError(t, "try {} catch ({ e }) { function e() {} }", errorText)
	expectParseError(t, "try {} catch (e) { let e }", errorText)
	expectParseError(t, "try {} catch (e) { const e = 0 }", errorText)
}

func TestWarningEqualsNegativeZero(t *testing.T) {
	note := "NOTE: Floating-point equality is defined such that 0 and -0 are equal, so \"x === -0\" returns true for both 0 and -0. " +
		"You need to use \"Object.is(x, -0)\" instead to test for -0.\n"

	expectParseError(t, "x === -0", "<stdin>: WARNING: Comparison with -0 using the \"===\" operator will also match 0\n"+note)
	expectParseError(t, "x == -0", "<stdin>: WARNING: Comparison with -0 using the \"==\" operator will also match 0\n"+note)
	expectParseError(t, "x !== -0", "<stdin>: WARNING: Comparison with -0 using the \"!==\" operator will also match 0\n"+note)
	expectParseError(t, "x != -0", "<stdin>: WARNING: Comparison with -0 using the \"!=\" operator will also match 0\n"+note)
	expectParseError(t, "switch (x) { case -0: }", "<stdin>: WARNING: Comparison with -0 using a case clause will also match 0\n"+note)

	expectParseError(t, "-0 === x", "<stdin>: WARNING: Comparison with -0 using the \"===\" operator will also match 0\n"+note)
	expectParseError(t, "-0 == x", "<stdin>: WARNING: Comparison with -0 using the \"==\" operator will also match 0\n"+note)
	expectParseError(t, "-0 !== x", "<stdin>: WARNING: Comparison with -0 using the \"!==\" operator will also match 0\n"+note)
	expectParseError(t, "-0 != x", "<stdin>: WARNING: Comparison with -0 using the \"!=\" operator will also match 0\n"+note)
	expectParseError(t, "switch (-0) { case x: }", "") // Don't bother to handle this case
}

func TestWarningEqualsNewObject(t *testing.T) {
	note := "NOTE: Equality with a new object is always false in JavaScript because the equality operator tests object identity. " +
		"You need to write code to compare the contents of the object instead. " +
		"For example, use \"Array.isArray(x) && x.length === 0\" instead of \"x === []\" to test for an empty array.\n"

	expectParseError(t, "x === []", "<stdin>: WARNING: Comparison using the \"===\" operator here is always false\n"+note)
	expectParseError(t, "x !== []", "<stdin>: WARNING: Comparison using the \"!==\" operator here is always true\n"+note)
	expectParseError(t, "x == []", "")
	expectParseError(t, "x != []", "")
	expectParseError(t, "switch (x) { case []: }", "<stdin>: WARNING: This case clause will never be evaluated because the comparison is always false\n"+note)

	expectParseError(t, "[] === x", "<stdin>: WARNING: Comparison using the \"===\" operator here is always false\n"+note)
	expectParseError(t, "[] !== x", "<stdin>: WARNING: Comparison using the \"!==\" operator here is always true\n"+note)
	expectParseError(t, "[] == x", "")
	expectParseError(t, "[] != x", "")
	expectParseError(t, "switch ([]) { case x: }", "") // Don't bother to handle this case
}

func TestWarningEqualsNaN(t *testing.T) {
	note := "NOTE: Floating-point equality is defined such that NaN is never equal to anything, so \"x === NaN\" always returns false. " +
		"You need to use \"Number.isNaN(x)\" instead to test for NaN.\n"

	expectParseError(t, "x === NaN", "<stdin>: WARNING: Comparison with NaN using the \"===\" operator here is always false\n"+note)
	expectParseError(t, "x !== NaN", "<stdin>: WARNING: Comparison with NaN using the \"!==\" operator here is always true\n"+note)
	expectParseError(t, "x == NaN", "<stdin>: WARNING: Comparison with NaN using the \"==\" operator here is always false\n"+note)
	expectParseError(t, "x != NaN", "<stdin>: WARNING: Comparison with NaN using the \"!=\" operator here is always true\n"+note)
	expectParseError(t, "switch (x) { case NaN: }", "<stdin>: WARNING: This case clause will never be evaluated because equality with NaN is always false\n"+note)

	expectParseError(t, "NaN === x", "<stdin>: WARNING: Comparison with NaN using the \"===\" operator here is always false\n"+note)
	expectParseError(t, "NaN !== x", "<stdin>: WARNING: Comparison with NaN using the \"!==\" operator here is always true\n"+note)
	expectParseError(t, "NaN == x", "<stdin>: WARNING: Comparison with NaN using the \"==\" operator here is always false\n"+note)
	expectParseError(t, "NaN != x", "<stdin>: WARNING: Comparison with NaN using the \"!=\" operator here is always true\n"+note)
	expectParseError(t, "switch (NaN) { case x: }", "") // Don't bother to handle this case
}

func TestWarningTypeofEquals(t *testing.T) {
	note := "NOTE: The expression \"typeof x\" actually evaluates to \"object\" in JavaScript, not \"null\". " +
		"You need to use \"x === null\" to test for null.\n"

	expectParseError(t, "typeof x === 'null'", "<stdin>: WARNING: The \"typeof\" operator will never evaluate to \"null\"\n"+note)
	expectParseError(t, "typeof x !== 'null'", "<stdin>: WARNING: The \"typeof\" operator will never evaluate to \"null\"\n"+note)
	expectParseError(t, "typeof x == 'null'", "<stdin>: WARNING: The \"typeof\" operator will never evaluate to \"null\"\n"+note)
	expectParseError(t, "typeof x != 'null'", "<stdin>: WARNING: The \"typeof\" operator will never evaluate to \"null\"\n"+note)
	expectParseError(t, "switch (typeof x) { case 'null': }", "<stdin>: WARNING: The \"typeof\" operator will never evaluate to \"null\"\n"+note)

	expectParseError(t, "'null' === typeof x", "<stdin>: WARNING: The \"typeof\" operator will never evaluate to \"null\"\n"+note)
	expectParseError(t, "'null' !== typeof x", "<stdin>: WARNING: The \"typeof\" operator will never evaluate to \"null\"\n"+note)
	expectParseError(t, "'null' == typeof x", "<stdin>: WARNING: The \"typeof\" operator will never evaluate to \"null\"\n"+note)
	expectParseError(t, "'null' != typeof x", "<stdin>: WARNING: The \"typeof\" operator will never evaluate to \"null\"\n"+note)
	expectParseError(t, "switch ('null') { case typeof x: }", "") // Don't bother to handle this case
}

func TestWarningDeleteSuperProperty(t *testing.T) {
	text := "<stdin>: WARNING: Attempting to delete a property of \"super\" will throw a ReferenceError\n"
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

	alwaysWarning := "<stdin>: WARNING: This case clause will never be evaluated because it duplicates an earlier case clause\n" +
		"<stdin>: NOTE: The earlier case clause is here:\n"
	likelyWarning := "<stdin>: WARNING: This case clause may never be evaluated because it likely duplicates an earlier case clause\n" +
		"<stdin>: NOTE: The earlier case clause is here:\n"

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

func TestWarningDuplicateClassMember(t *testing.T) {
	duplicateWarning := "<stdin>: WARNING: Duplicate member \"x\" in class body\n" +
		"<stdin>: NOTE: The original member \"x\" is here:\n"

	expectParseError(t, "class Foo { x; x }", duplicateWarning)
	expectParseError(t, "class Foo { x() {}; x() {} }", duplicateWarning)
	expectParseError(t, "class Foo { get x() {}; get x() {} }", duplicateWarning)
	expectParseError(t, "class Foo { get x() {}; set x(y) {}; get x() {} }", duplicateWarning)
	expectParseError(t, "class Foo { get x() {}; set x(y) {}; set x(y) {} }", duplicateWarning)
	expectParseError(t, "class Foo { get x() {}; set x(y) {} }", "")
	expectParseError(t, "class Foo { set x(y) {}; get x() {} }", "")

	expectParseError(t, "class Foo { static x; static x }", duplicateWarning)
	expectParseError(t, "class Foo { static x() {}; static x() {} }", duplicateWarning)
	expectParseError(t, "class Foo { static get x() {}; static get x() {} }", duplicateWarning)
	expectParseError(t, "class Foo { static get x() {}; static set x(y) {}; static get x() {} }", duplicateWarning)
	expectParseError(t, "class Foo { static get x() {}; static set x(y) {}; static set x(y) {} }", duplicateWarning)
	expectParseError(t, "class Foo { static get x() {}; static set x(y) {} }", "")
	expectParseError(t, "class Foo { static set x(y) {}; static get x() {} }", "")

	expectParseError(t, "class Foo { x; static x }", "")
	expectParseError(t, "class Foo { x; static x() {} }", "")
	expectParseError(t, "class Foo { x() {}; static x }", "")
	expectParseError(t, "class Foo { x() {}; static x() {} }", "")
	expectParseError(t, "class Foo { static x; x }", "")
	expectParseError(t, "class Foo { static x; x() {} }", "")
	expectParseError(t, "class Foo { static x() {}; x }", "")
	expectParseError(t, "class Foo { static x() {}; x() {} }", "")
	expectParseError(t, "class Foo { get x() {}; static get x() {} }", "")
	expectParseError(t, "class Foo { set x(y) {}; static set x(y) {} }", "")
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

func TestMangleLoopJump(t *testing.T) {
	// Trim after jump
	expectPrintedMangle(t, "while (x) { if (1) break; z(); }", "for (; x; )\n  break;\n")
	expectPrintedMangle(t, "while (x) { if (1) continue; z(); }", "for (; x; )\n  ;\n")
	expectPrintedMangle(t, "foo: while (a) while (x) { if (1) continue foo; z(); }", "foo:\n  for (; a; )\n    for (; x; )\n      continue foo;\n")
	expectPrintedMangle(t, "while (x) { y(); if (1) break; z(); }", "for (; x; ) {\n  y();\n  break;\n}\n")
	expectPrintedMangle(t, "while (x) { y(); if (1) continue; z(); }", "for (; x; )\n  y();\n")
	expectPrintedMangle(t, "while (x) { y(); debugger; if (1) continue; z(); }", "for (; x; ) {\n  y();\n  debugger;\n}\n")
	expectPrintedMangle(t, "while (x) { let y = z(); if (1) continue; z(); }", "for (; x; ) {\n  let y = z();\n}\n")
	expectPrintedMangle(t, "while (x) { debugger; if (y) { if (1) break; z() } }", "for (; x; ) {\n  debugger;\n  if (y)\n    break;\n}\n")
	expectPrintedMangle(t, "while (x) { debugger; if (y) { if (1) continue; z() } }", "for (; x; ) {\n  debugger;\n  y;\n}\n")
	expectPrintedMangle(t, "while (x) { debugger; if (1) { if (1) break; z() } }", "for (; x; ) {\n  debugger;\n  break;\n}\n")
	expectPrintedMangle(t, "while (x) { debugger; if (1) { if (1) continue; z() } }", "for (; x; )\n  debugger;\n")

	// Trim trailing continue
	expectPrintedMangle(t, "while (x()) continue", "for (; x(); )\n  ;\n")
	expectPrintedMangle(t, "while (x) { y(); continue }", "for (; x; )\n  y();\n")
	expectPrintedMangle(t, "while (x) { if (y) { z(); continue } }",
		"for (; x; )\n  if (y) {\n    z();\n    continue;\n  }\n")
	expectPrintedMangle(t, "label: while (x) while (y) { z(); continue label }",
		"label:\n  for (; x; )\n    for (; y; ) {\n      z();\n      continue label;\n    }\n")

	// Optimize implicit continue
	expectPrintedMangle(t, "while (x) { if (y) continue; z(); }", "for (; x; )\n  y || z();\n")
	expectPrintedMangle(t, "while (x) { if (y) continue; else z(); w(); }", "for (; x; )\n  y || (z(), w());\n")
	expectPrintedMangle(t, "while (x) { t(); if (y) continue; z(); }", "for (; x; )\n  t(), !y && z();\n")
	expectPrintedMangle(t, "while (x) { t(); if (y) continue; else z(); w(); }", "for (; x; )\n  t(), !y && (z(), w());\n")
	expectPrintedMangle(t, "while (x) { debugger; if (y) continue; z(); }", "for (; x; ) {\n  debugger;\n  y || z();\n}\n")
	expectPrintedMangle(t, "while (x) { debugger; if (y) continue; else z(); w(); }", "for (; x; ) {\n  debugger;\n  y || (z(), w());\n}\n")

	// Do not optimize implicit continue for statements that care about scope
	expectPrintedMangle(t, "while (x) { if (y) continue; function y() {} }", "for (; x; ) {\n  let y = function() {\n  };\n  var y = y;\n}\n")
	expectPrintedMangle(t, "while (x) { if (y) continue; let y }", "for (; x; ) {\n  if (y)\n    continue;\n  let y;\n}\n")
	expectPrintedMangle(t, "while (x) { if (y) continue; var y }", "for (; x; )\n  if (!y)\n    var y;\n")
}

func TestMangleUndefined(t *testing.T) {
	// These should be transformed
	expectPrintedNormalAndMangle(t, "console.log(undefined)", "console.log(void 0);\n", "console.log(void 0);\n")
	expectPrintedNormalAndMangle(t, "console.log(+undefined)", "console.log(NaN);\n", "console.log(NaN);\n")
	expectPrintedNormalAndMangle(t, "console.log(undefined + undefined)", "console.log(void 0 + void 0);\n", "console.log(void 0 + void 0);\n")
	expectPrintedNormalAndMangle(t, "const x = undefined", "const x = void 0;\n", "const x = void 0;\n")
	expectPrintedNormalAndMangle(t, "let x = undefined", "let x = void 0;\n", "let x;\n")
	expectPrintedNormalAndMangle(t, "var x = undefined", "var x = void 0;\n", "var x = void 0;\n")
	expectPrintedNormalAndMangle(t, "function foo(a) { if (!a) return undefined; a() }", "function foo(a) {\n  if (!a)\n    return void 0;\n  a();\n}\n", "function foo(a) {\n  a && a();\n}\n")

	// These should not be transformed
	expectPrintedNormalAndMangle(t, "delete undefined", "delete undefined;\n", "delete undefined;\n")
	expectPrintedNormalAndMangle(t, "undefined--", "undefined--;\n", "undefined--;\n")
	expectPrintedNormalAndMangle(t, "undefined++", "undefined++;\n", "undefined++;\n")
	expectPrintedNormalAndMangle(t, "--undefined", "--undefined;\n", "--undefined;\n")
	expectPrintedNormalAndMangle(t, "++undefined", "++undefined;\n", "++undefined;\n")
	expectPrintedNormalAndMangle(t, "undefined = 1", "undefined = 1;\n", "undefined = 1;\n")
	expectPrintedNormalAndMangle(t, "[undefined] = 1", "[undefined] = 1;\n", "[undefined] = 1;\n")
	expectPrintedNormalAndMangle(t, "({x: undefined} = 1)", "({ x: undefined } = 1);\n", "({ x: undefined } = 1);\n")
	expectPrintedNormalAndMangle(t, "with (x) y(undefined); z(undefined)", "with (x)\n  y(undefined);\nz(void 0);\n", "with (x)\n  y(undefined);\nz(void 0);\n")
	expectPrintedNormalAndMangle(t, "with (x) while (i) y(undefined); z(undefined)", "with (x)\n  while (i)\n    y(undefined);\nz(void 0);\n", "with (x)\n  for (; i; )\n    y(undefined);\nz(void 0);\n")
}

func TestMangleIndex(t *testing.T) {
	expectPrintedNormalAndMangle(t, "x['y']", "x[\"y\"];\n", "x.y;\n")
	expectPrintedNormalAndMangle(t, "x['y z']", "x[\"y z\"];\n", "x[\"y z\"];\n")
	expectPrintedNormalAndMangle(t, "x?.['y']", "x?.[\"y\"];\n", "x?.y;\n")
	expectPrintedNormalAndMangle(t, "x?.['y z']", "x?.[\"y z\"];\n", "x?.[\"y z\"];\n")
	expectPrintedNormalAndMangle(t, "x?.['y']()", "x?.[\"y\"]();\n", "x?.y();\n")
	expectPrintedNormalAndMangle(t, "x?.['y z']()", "x?.[\"y z\"]();\n", "x?.[\"y z\"]();\n")

	// Check the string-to-int optimization
	expectPrintedNormalAndMangle(t, "x['0']", "x[\"0\"];\n", "x[0];\n")
	expectPrintedNormalAndMangle(t, "x['123']", "x[\"123\"];\n", "x[123];\n")
	expectPrintedNormalAndMangle(t, "x['-123']", "x[\"-123\"];\n", "x[-123];\n")
	expectPrintedNormalAndMangle(t, "x['-0']", "x[\"-0\"];\n", "x[\"-0\"];\n")
	expectPrintedNormalAndMangle(t, "x['01']", "x[\"01\"];\n", "x[\"01\"];\n")
	expectPrintedNormalAndMangle(t, "x['-01']", "x[\"-01\"];\n", "x[\"-01\"];\n")
	expectPrintedNormalAndMangle(t, "x['0x1']", "x[\"0x1\"];\n", "x[\"0x1\"];\n")
	expectPrintedNormalAndMangle(t, "x['-0x1']", "x[\"-0x1\"];\n", "x[\"-0x1\"];\n")
	expectPrintedNormalAndMangle(t, "x['2147483647']", "x[\"2147483647\"];\n", "x[2147483647];\n")
	expectPrintedNormalAndMangle(t, "x['2147483648']", "x[\"2147483648\"];\n", "x[\"2147483648\"];\n")
	expectPrintedNormalAndMangle(t, "x['-2147483648']", "x[\"-2147483648\"];\n", "x[-2147483648];\n")
	expectPrintedNormalAndMangle(t, "x['-2147483649']", "x[\"-2147483649\"];\n", "x[\"-2147483649\"];\n")
}

func TestMangleBlock(t *testing.T) {
	expectPrintedMangle(t, "while(1) { while (1) {} }", "for (; ; )\n  for (; ; )\n    ;\n")
	expectPrintedMangle(t, "while(1) { const x = y; }", "for (; ; ) {\n  const x = y;\n}\n")
	expectPrintedMangle(t, "while(1) { let x; }", "for (; ; ) {\n  let x;\n}\n")
	expectPrintedMangle(t, "while(1) { var x; }", "for (; ; )\n  var x;\n")
	expectPrintedMangle(t, "while(1) { class X {} }", "for (; ; ) {\n  class X {\n  }\n}\n")
	expectPrintedMangle(t, "while(1) { function x() {} }", "for (; ; )\n  var x = function() {\n  };\n")
	expectPrintedMangle(t, "while(1) { function* x() {} }", "for (; ; ) {\n  function* x() {\n  }\n}\n")
	expectPrintedMangle(t, "while(1) { async function x() {} }", "for (; ; ) {\n  async function x() {\n  }\n}\n")
	expectPrintedMangle(t, "while(1) { async function* x() {} }", "for (; ; ) {\n  async function* x() {\n  }\n}\n")
}

func TestMangleSwitch(t *testing.T) {
	expectPrintedMangle(t, "x(); switch (y) { case z: return w; }", "switch (x(), y) {\n  case z:\n    return w;\n}\n")
	expectPrintedMangle(t, "if (t) { x(); switch (y) { case z: return w; } }", "if (t)\n  switch (x(), y) {\n    case z:\n      return w;\n  }\n")
}

func TestMangleAddEmptyString(t *testing.T) {
	expectPrintedNormalAndMangle(t, "a = '' + 0", "a = \"0\";\n", "a = \"0\";\n")
	expectPrintedNormalAndMangle(t, "a = 0 + ''", "a = \"0\";\n", "a = \"0\";\n")
	expectPrintedNormalAndMangle(t, "a = '' + b", "a = \"\" + b;\n", "a = \"\" + b;\n")
	expectPrintedNormalAndMangle(t, "a = b + ''", "a = b + \"\";\n", "a = b + \"\";\n")

	expectPrintedNormalAndMangle(t, "a = '' + `${b}`", "a = `${b}`;\n", "a = `${b}`;\n")
	expectPrintedNormalAndMangle(t, "a = `${b}` + ''", "a = `${b}`;\n", "a = `${b}`;\n")
	expectPrintedNormalAndMangle(t, "a = '' + typeof b", "a = typeof b;\n", "a = typeof b;\n")
	expectPrintedNormalAndMangle(t, "a = typeof b + ''", "a = typeof b;\n", "a = typeof b;\n")
}

func TestMangleStringLength(t *testing.T) {
	expectPrinted(t, "a = ''.length", "a = \"\".length;\n")
	expectPrintedMangle(t, "''.length++", "\"\".length++;\n")
	expectPrintedMangle(t, "''.length = a", "\"\".length = a;\n")

	expectPrintedMangle(t, "a = ''.len", "a = \"\".len;\n")
	expectPrintedMangle(t, "a = [].length", "a = [].length;\n")
	expectPrintedMangle(t, "a = ''.length", "a = 0;\n")
	expectPrintedMangle(t, "a = ``.length", "a = 0;\n")
	expectPrintedMangle(t, "a = b``.length", "a = b``.length;\n")
	expectPrintedMangle(t, "a = 'abc'.length", "a = 3;\n")
	expectPrintedMangle(t, "a = 'ȧḃċ'.length", "a = 3;\n")
	expectPrintedMangle(t, "a = '👯‍♂️'.length", "a = 5;\n")
}

func TestMangleNot(t *testing.T) {
	// These can be mangled
	expectPrintedNormalAndMangle(t, "a = !(b == c)", "a = !(b == c);\n", "a = b != c;\n")
	expectPrintedNormalAndMangle(t, "a = !(b != c)", "a = !(b != c);\n", "a = b == c;\n")
	expectPrintedNormalAndMangle(t, "a = !(b === c)", "a = !(b === c);\n", "a = b !== c;\n")
	expectPrintedNormalAndMangle(t, "a = !(b !== c)", "a = !(b !== c);\n", "a = b === c;\n")
	expectPrintedNormalAndMangle(t, "if (!(a, b)) return c", "if (!(a, b))\n  return c;\n", "if (a, !b)\n  return c;\n")

	// These can't be mangled due to NaN and other special cases
	expectPrintedNormalAndMangle(t, "a = !(b < c)", "a = !(b < c);\n", "a = !(b < c);\n")
	expectPrintedNormalAndMangle(t, "a = !(b > c)", "a = !(b > c);\n", "a = !(b > c);\n")
	expectPrintedNormalAndMangle(t, "a = !(b <= c)", "a = !(b <= c);\n", "a = !(b <= c);\n")
	expectPrintedNormalAndMangle(t, "a = !(b >= c)", "a = !(b >= c);\n", "a = !(b >= c);\n")
}

func TestMangleDoubleNot(t *testing.T) {
	expectPrintedNormalAndMangle(t, "a = !!b", "a = !!b;\n", "a = !!b;\n")

	expectPrintedNormalAndMangle(t, "a = !!!b", "a = !!!b;\n", "a = !b;\n")
	expectPrintedNormalAndMangle(t, "a = !!-b", "a = !!-b;\n", "a = !!-b;\n")
	expectPrintedNormalAndMangle(t, "a = !!void b", "a = !!void b;\n", "a = !!void b;\n")
	expectPrintedNormalAndMangle(t, "a = !!delete b", "a = !!delete b;\n", "a = delete b;\n")

	expectPrintedNormalAndMangle(t, "a = !!(b + c)", "a = !!(b + c);\n", "a = !!(b + c);\n")
	expectPrintedNormalAndMangle(t, "a = !!(b == c)", "a = !!(b == c);\n", "a = b == c;\n")
	expectPrintedNormalAndMangle(t, "a = !!(b != c)", "a = !!(b != c);\n", "a = b != c;\n")
	expectPrintedNormalAndMangle(t, "a = !!(b === c)", "a = !!(b === c);\n", "a = b === c;\n")
	expectPrintedNormalAndMangle(t, "a = !!(b !== c)", "a = !!(b !== c);\n", "a = b !== c;\n")
	expectPrintedNormalAndMangle(t, "a = !!(b < c)", "a = !!(b < c);\n", "a = b < c;\n")
	expectPrintedNormalAndMangle(t, "a = !!(b > c)", "a = !!(b > c);\n", "a = b > c;\n")
	expectPrintedNormalAndMangle(t, "a = !!(b <= c)", "a = !!(b <= c);\n", "a = b <= c;\n")
	expectPrintedNormalAndMangle(t, "a = !!(b >= c)", "a = !!(b >= c);\n", "a = b >= c;\n")
	expectPrintedNormalAndMangle(t, "a = !!(b in c)", "a = !!(b in c);\n", "a = b in c;\n")
	expectPrintedNormalAndMangle(t, "a = !!(b instanceof c)", "a = !!(b instanceof c);\n", "a = b instanceof c;\n")

	expectPrintedNormalAndMangle(t, "a = !!(b && c)", "a = !!(b && c);\n", "a = !!(b && c);\n")
	expectPrintedNormalAndMangle(t, "a = !!(b || c)", "a = !!(b || c);\n", "a = !!(b || c);\n")
	expectPrintedNormalAndMangle(t, "a = !!(b ?? c)", "a = !!(b ?? c);\n", "a = !!(b ?? c);\n")

	expectPrintedNormalAndMangle(t, "a = !!(!b && c)", "a = !!(!b && c);\n", "a = !!(!b && c);\n")
	expectPrintedNormalAndMangle(t, "a = !!(!b || c)", "a = !!(!b || c);\n", "a = !!(!b || c);\n")
	expectPrintedNormalAndMangle(t, "a = !!(!b ?? c)", "a = !!!b;\n", "a = !b;\n")

	expectPrintedNormalAndMangle(t, "a = !!(b && !c)", "a = !!(b && !c);\n", "a = !!(b && !c);\n")
	expectPrintedNormalAndMangle(t, "a = !!(b || !c)", "a = !!(b || !c);\n", "a = !!(b || !c);\n")
	expectPrintedNormalAndMangle(t, "a = !!(b ?? !c)", "a = !!(b ?? !c);\n", "a = !!(b ?? !c);\n")

	expectPrintedNormalAndMangle(t, "a = !!(!b && !c)", "a = !!(!b && !c);\n", "a = !b && !c;\n")
	expectPrintedNormalAndMangle(t, "a = !!(!b || !c)", "a = !!(!b || !c);\n", "a = !b || !c;\n")
	expectPrintedNormalAndMangle(t, "a = !!(!b ?? !c)", "a = !!!b;\n", "a = !b;\n")

	expectPrintedNormalAndMangle(t, "a = !!(b, c)", "a = !!(b, c);\n", "a = (b, !!c);\n")
}

func TestMangleBooleanConstructor(t *testing.T) {
	expectPrintedNormalAndMangle(t, "a = Boolean(b); var Boolean", "a = Boolean(b);\nvar Boolean;\n", "a = Boolean(b);\nvar Boolean;\n")

	expectPrintedNormalAndMangle(t, "a = Boolean()", "a = Boolean();\n", "a = false;\n")
	expectPrintedNormalAndMangle(t, "a = Boolean(b)", "a = Boolean(b);\n", "a = !!b;\n")
	expectPrintedNormalAndMangle(t, "a = Boolean(!b)", "a = Boolean(!b);\n", "a = !b;\n")
	expectPrintedNormalAndMangle(t, "a = Boolean(!!b)", "a = Boolean(!!b);\n", "a = !!b;\n")
	expectPrintedNormalAndMangle(t, "a = Boolean(b ? true : false)", "a = Boolean(b ? true : false);\n", "a = !!b;\n")
	expectPrintedNormalAndMangle(t, "a = Boolean(b ? false : true)", "a = Boolean(b ? false : true);\n", "a = !b;\n")
	expectPrintedNormalAndMangle(t, "a = Boolean(b ? c > 0 : c < 0)", "a = Boolean(b ? c > 0 : c < 0);\n", "a = b ? c > 0 : c < 0;\n")

	// Check for calling "SimplifyBooleanExpr" on the argument
	expectPrintedNormalAndMangle(t, "a = Boolean((b | c) !== 0)", "a = Boolean((b | c) !== 0);\n", "a = !!(b | c);\n")
	expectPrintedNormalAndMangle(t, "a = Boolean(b ? (c | d) !== 0 : (d | e) !== 0)", "a = Boolean(b ? (c | d) !== 0 : (d | e) !== 0);\n", "a = !!(b ? c | d : d | e);\n")
}

func TestMangleNumberConstructor(t *testing.T) {
	expectPrintedNormalAndMangle(t, "a = Number(x)", "a = Number(x);\n", "a = Number(x);\n")
	expectPrintedNormalAndMangle(t, "a = Number(0n)", "a = Number(0n);\n", "a = Number(0n);\n")
	expectPrintedNormalAndMangle(t, "a = Number(false); var Number", "a = Number(false);\nvar Number;\n", "a = Number(false);\nvar Number;\n")
	expectPrintedNormalAndMangle(t, "a = Number(0xFFFF_FFFF_FFFF_FFFFn)", "a = Number(0xFFFFFFFFFFFFFFFFn);\n", "a = Number(0xFFFFFFFFFFFFFFFFn);\n")

	expectPrintedNormalAndMangle(t, "a = Number()", "a = Number();\n", "a = 0;\n")
	expectPrintedNormalAndMangle(t, "a = Number(-123)", "a = Number(-123);\n", "a = -123;\n")
	expectPrintedNormalAndMangle(t, "a = Number(false)", "a = Number(false);\n", "a = 0;\n")
	expectPrintedNormalAndMangle(t, "a = Number(true)", "a = Number(true);\n", "a = 1;\n")
	expectPrintedNormalAndMangle(t, "a = Number(undefined)", "a = Number(void 0);\n", "a = NaN;\n")
	expectPrintedNormalAndMangle(t, "a = Number(null)", "a = Number(null);\n", "a = 0;\n")
	expectPrintedNormalAndMangle(t, "a = Number(b ? !c : !d)", "a = Number(b ? !c : !d);\n", "a = +(b ? !c : !d);\n")
}

func TestMangleStringConstructor(t *testing.T) {
	expectPrintedNormalAndMangle(t, "a = String(x)", "a = String(x);\n", "a = String(x);\n")
	expectPrintedNormalAndMangle(t, "a = String('x'); var String", "a = String(\"x\");\nvar String;\n", "a = String(\"x\");\nvar String;\n")

	expectPrintedNormalAndMangle(t, "a = String()", "a = String();\n", "a = \"\";\n")
	expectPrintedNormalAndMangle(t, "a = String('x')", "a = String(\"x\");\n", "a = \"x\";\n")
	expectPrintedNormalAndMangle(t, "a = String(b ? 'x' : 'y')", "a = String(b ? \"x\" : \"y\");\n", "a = b ? \"x\" : \"y\";\n")
}

func TestMangleBigIntConstructor(t *testing.T) {
	expectPrintedNormalAndMangle(t, "a = BigInt(x)", "a = BigInt(x);\n", "a = BigInt(x);\n")
	expectPrintedNormalAndMangle(t, "a = BigInt(0n); var BigInt", "a = BigInt(0n);\nvar BigInt;\n", "a = BigInt(0n);\nvar BigInt;\n")

	// Note: This throws instead of returning "0n"
	expectPrintedNormalAndMangle(t, "a = BigInt()", "a = BigInt();\n", "a = BigInt();\n")

	// Note: Transforming this into "0n" is unsafe because that syntax may not be supported
	expectPrintedNormalAndMangle(t, "a = BigInt('0')", "a = BigInt(\"0\");\n", "a = BigInt(\"0\");\n")

	expectPrintedNormalAndMangle(t, "a = BigInt(0n)", "a = BigInt(0n);\n", "a = 0n;\n")
	expectPrintedNormalAndMangle(t, "a = BigInt(b ? 0n : 1n)", "a = BigInt(b ? 0n : 1n);\n", "a = b ? 0n : 1n;\n")
}

func TestMangleIf(t *testing.T) {
	expectPrintedNormalAndMangle(t, "1 ? a() : b()", "1 ? a() : b();\n", "a();\n")
	expectPrintedNormalAndMangle(t, "0 ? a() : b()", "0 ? a() : b();\n", "b();\n")

	expectPrintedNormalAndMangle(t, "a ? a : b", "a ? a : b;\n", "a || b;\n")
	expectPrintedNormalAndMangle(t, "a ? b : a", "a ? b : a;\n", "a && b;\n")
	expectPrintedNormalAndMangle(t, "a.x ? a.x : b", "a.x ? a.x : b;\n", "a.x ? a.x : b;\n")
	expectPrintedNormalAndMangle(t, "a.x ? b : a.x", "a.x ? b : a.x;\n", "a.x ? b : a.x;\n")

	expectPrintedNormalAndMangle(t, "a ? b() : c()", "a ? b() : c();\n", "a ? b() : c();\n")
	expectPrintedNormalAndMangle(t, "!a ? b() : c()", "!a ? b() : c();\n", "a ? c() : b();\n")
	expectPrintedNormalAndMangle(t, "!!a ? b() : c()", "!!a ? b() : c();\n", "a ? b() : c();\n")
	expectPrintedNormalAndMangle(t, "!!!a ? b() : c()", "!!!a ? b() : c();\n", "a ? c() : b();\n")

	expectPrintedNormalAndMangle(t, "if (1) a(); else b()", "if (1)\n  a();\nelse\n  b();\n", "a();\n")
	expectPrintedNormalAndMangle(t, "if (0) a(); else b()", "if (0)\n  a();\nelse\n  b();\n", "b();\n")
	expectPrintedNormalAndMangle(t, "if (a) b(); else c()", "if (a)\n  b();\nelse\n  c();\n", "a ? b() : c();\n")
	expectPrintedNormalAndMangle(t, "if (!a) b(); else c()", "if (!a)\n  b();\nelse\n  c();\n", "a ? c() : b();\n")
	expectPrintedNormalAndMangle(t, "if (!!a) b(); else c()", "if (!!a)\n  b();\nelse\n  c();\n", "a ? b() : c();\n")
	expectPrintedNormalAndMangle(t, "if (!!!a) b(); else c()", "if (!!!a)\n  b();\nelse\n  c();\n", "a ? c() : b();\n")

	expectPrintedNormalAndMangle(t, "if (1) a()", "if (1)\n  a();\n", "a();\n")
	expectPrintedNormalAndMangle(t, "if (0) a()", "if (0)\n  a();\n", "")
	expectPrintedNormalAndMangle(t, "if (a) b()", "if (a)\n  b();\n", "a && b();\n")
	expectPrintedNormalAndMangle(t, "if (!a) b()", "if (!a)\n  b();\n", "a || b();\n")
	expectPrintedNormalAndMangle(t, "if (!!a) b()", "if (!!a)\n  b();\n", "a && b();\n")
	expectPrintedNormalAndMangle(t, "if (!!!a) b()", "if (!!!a)\n  b();\n", "a || b();\n")

	expectPrintedNormalAndMangle(t, "if (1) {} else a()", "if (1) {\n} else\n  a();\n", "")
	expectPrintedNormalAndMangle(t, "if (0) {} else a()", "if (0) {\n} else\n  a();\n", "a();\n")
	expectPrintedNormalAndMangle(t, "if (a) {} else b()", "if (a) {\n} else\n  b();\n", "a || b();\n")
	expectPrintedNormalAndMangle(t, "if (!a) {} else b()", "if (!a) {\n} else\n  b();\n", "a && b();\n")
	expectPrintedNormalAndMangle(t, "if (!!a) {} else b()", "if (!!a) {\n} else\n  b();\n", "a || b();\n")
	expectPrintedNormalAndMangle(t, "if (!!!a) {} else b()", "if (!!!a) {\n} else\n  b();\n", "a && b();\n")

	expectPrintedNormalAndMangle(t, "if (a) {} else throw b", "if (a) {\n} else\n  throw b;\n", "if (!a)\n  throw b;\n")
	expectPrintedNormalAndMangle(t, "if (!a) {} else throw b", "if (!a) {\n} else\n  throw b;\n", "if (a)\n  throw b;\n")
	expectPrintedNormalAndMangle(t, "a(); if (b) throw c", "a();\nif (b)\n  throw c;\n", "if (a(), b)\n  throw c;\n")
	expectPrintedNormalAndMangle(t, "if (a) if (b) throw c", "if (a) {\n  if (b)\n    throw c;\n}\n", "if (a && b)\n  throw c;\n")

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

	expectPrintedNormalAndMangle(t, "a = b ? true : false", "a = b ? true : false;\n", "a = !!b;\n")
	expectPrintedNormalAndMangle(t, "a = b ? false : true", "a = b ? false : true;\n", "a = !b;\n")
	expectPrintedNormalAndMangle(t, "a = !b ? true : false", "a = !b ? true : false;\n", "a = !b;\n")
	expectPrintedNormalAndMangle(t, "a = !b ? false : true", "a = !b ? false : true;\n", "a = !!b;\n")

	expectPrintedNormalAndMangle(t, "a = b == c ? true : false", "a = b == c ? true : false;\n", "a = b == c;\n")
	expectPrintedNormalAndMangle(t, "a = b != c ? true : false", "a = b != c ? true : false;\n", "a = b != c;\n")
	expectPrintedNormalAndMangle(t, "a = b === c ? true : false", "a = b === c ? true : false;\n", "a = b === c;\n")
	expectPrintedNormalAndMangle(t, "a = b !== c ? true : false", "a = b !== c ? true : false;\n", "a = b !== c;\n")

	expectPrintedNormalAndMangle(t, "a ? b(c) : b(d)", "a ? b(c) : b(d);\n", "a ? b(c) : b(d);\n")
	expectPrintedNormalAndMangle(t, "let a; a ? b(c) : b(d)", "let a;\na ? b(c) : b(d);\n", "let a;\na ? b(c) : b(d);\n")
	expectPrintedNormalAndMangle(t, "let a, b; a ? b(c) : b(d)", "let a, b;\na ? b(c) : b(d);\n", "let a, b;\nb(a ? c : d);\n")
	expectPrintedNormalAndMangle(t, "let a, b; a ? b(c, 0) : b(d)", "let a, b;\na ? b(c, 0) : b(d);\n", "let a, b;\na ? b(c, 0) : b(d);\n")
	expectPrintedNormalAndMangle(t, "let a, b; a ? b(c) : b(d, 0)", "let a, b;\na ? b(c) : b(d, 0);\n", "let a, b;\na ? b(c) : b(d, 0);\n")
	expectPrintedNormalAndMangle(t, "let a, b; a ? b(c, 0) : b(d, 1)", "let a, b;\na ? b(c, 0) : b(d, 1);\n", "let a, b;\na ? b(c, 0) : b(d, 1);\n")
	expectPrintedNormalAndMangle(t, "let a, b; a ? b(c, 0) : b(d, 0)", "let a, b;\na ? b(c, 0) : b(d, 0);\n", "let a, b;\nb(a ? c : d, 0);\n")
	expectPrintedNormalAndMangle(t, "let a, b; a ? b(...c) : b(d)", "let a, b;\na ? b(...c) : b(d);\n", "let a, b;\na ? b(...c) : b(d);\n")
	expectPrintedNormalAndMangle(t, "let a, b; a ? b(c) : b(...d)", "let a, b;\na ? b(c) : b(...d);\n", "let a, b;\na ? b(c) : b(...d);\n")
	expectPrintedNormalAndMangle(t, "let a, b; a ? b(...c) : b(...d)", "let a, b;\na ? b(...c) : b(...d);\n", "let a, b;\nb(...a ? c : d);\n")
	expectPrintedNormalAndMangle(t, "let a, b; a ? b(a) : b(c)", "let a, b;\na ? b(a) : b(c);\n", "let a, b;\nb(a || c);\n")
	expectPrintedNormalAndMangle(t, "let a, b; a ? b(c) : b(a)", "let a, b;\na ? b(c) : b(a);\n", "let a, b;\nb(a && c);\n")
	expectPrintedNormalAndMangle(t, "let a, b; a ? b(...a) : b(...c)", "let a, b;\na ? b(...a) : b(...c);\n", "let a, b;\nb(...a || c);\n")
	expectPrintedNormalAndMangle(t, "let a, b; a ? b(...c) : b(...a)", "let a, b;\na ? b(...c) : b(...a);\n", "let a, b;\nb(...a && c);\n")

	// Note: "a.x" may change "b" and "b.y" may change "a" in the examples
	// below, so the presence of these expressions must prevent reordering
	expectPrintedNormalAndMangle(t, "let a; a.x ? b(c) : b(d)", "let a;\na.x ? b(c) : b(d);\n", "let a;\na.x ? b(c) : b(d);\n")
	expectPrintedNormalAndMangle(t, "let a, b; a.x ? b(c) : b(d)", "let a, b;\na.x ? b(c) : b(d);\n", "let a, b;\na.x ? b(c) : b(d);\n")
	expectPrintedNormalAndMangle(t, "let a, b; a ? b.y(c) : b.y(d)", "let a, b;\na ? b.y(c) : b.y(d);\n", "let a, b;\na ? b.y(c) : b.y(d);\n")
	expectPrintedNormalAndMangle(t, "let a, b; a.x ? b.y(c) : b.y(d)", "let a, b;\na.x ? b.y(c) : b.y(d);\n", "let a, b;\na.x ? b.y(c) : b.y(d);\n")

	expectPrintedNormalAndMangle(t, "a ? b : c ? b : d", "a ? b : c ? b : d;\n", "a || c ? b : d;\n")
	expectPrintedNormalAndMangle(t, "a ? b ? c : d : d", "a ? b ? c : d : d;\n", "a && b ? c : d;\n")

	expectPrintedNormalAndMangle(t, "a ? c : (b, c)", "a ? c : (b, c);\n", "a || b, c;\n")
	expectPrintedNormalAndMangle(t, "a ? (b, c) : c", "a ? (b, c) : c;\n", "a && b, c;\n")
	expectPrintedNormalAndMangle(t, "a ? c : (b, d)", "a ? c : (b, d);\n", "a ? c : (b, d);\n")
	expectPrintedNormalAndMangle(t, "a ? (b, c) : d", "a ? (b, c) : d;\n", "a ? (b, c) : d;\n")

	expectPrintedNormalAndMangle(t, "a ? b || c : c", "a ? b || c : c;\n", "a && b || c;\n")
	expectPrintedNormalAndMangle(t, "a ? b || c : d", "a ? b || c : d;\n", "a ? b || c : d;\n")
	expectPrintedNormalAndMangle(t, "a ? b && c : c", "a ? b && c : c;\n", "a ? b && c : c;\n")

	expectPrintedNormalAndMangle(t, "a ? c : b && c", "a ? c : b && c;\n", "(a || b) && c;\n")
	expectPrintedNormalAndMangle(t, "a ? c : b && d", "a ? c : b && d;\n", "a ? c : b && d;\n")
	expectPrintedNormalAndMangle(t, "a ? c : b || c", "a ? c : b || c;\n", "a ? c : b || c;\n")

	expectPrintedNormalAndMangle(t, "a = b == null ? c : b", "a = b == null ? c : b;\n", "a = b == null ? c : b;\n")
	expectPrintedNormalAndMangle(t, "a = b != null ? b : c", "a = b != null ? b : c;\n", "a = b != null ? b : c;\n")

	expectPrintedNormalAndMangle(t, "let b; a = b == null ? c : b", "let b;\na = b == null ? c : b;\n", "let b;\na = b ?? c;\n")
	expectPrintedNormalAndMangle(t, "let b; a = b != null ? b : c", "let b;\na = b != null ? b : c;\n", "let b;\na = b ?? c;\n")
	expectPrintedNormalAndMangle(t, "let b; a = b == null ? b : c", "let b;\na = b == null ? b : c;\n", "let b;\na = b == null ? b : c;\n")
	expectPrintedNormalAndMangle(t, "let b; a = b != null ? c : b", "let b;\na = b != null ? c : b;\n", "let b;\na = b != null ? c : b;\n")

	expectPrintedNormalAndMangle(t, "let b; a = null == b ? c : b", "let b;\na = null == b ? c : b;\n", "let b;\na = b ?? c;\n")
	expectPrintedNormalAndMangle(t, "let b; a = null != b ? b : c", "let b;\na = null != b ? b : c;\n", "let b;\na = b ?? c;\n")
	expectPrintedNormalAndMangle(t, "let b; a = null == b ? b : c", "let b;\na = null == b ? b : c;\n", "let b;\na = b == null ? b : c;\n")
	expectPrintedNormalAndMangle(t, "let b; a = null != b ? c : b", "let b;\na = null != b ? c : b;\n", "let b;\na = b != null ? c : b;\n")

	// Don't do this if the condition has side effects
	expectPrintedNormalAndMangle(t, "let b; a = b.x == null ? c : b.x", "let b;\na = b.x == null ? c : b.x;\n", "let b;\na = b.x == null ? c : b.x;\n")
	expectPrintedNormalAndMangle(t, "let b; a = b.x != null ? b.x : c", "let b;\na = b.x != null ? b.x : c;\n", "let b;\na = b.x != null ? b.x : c;\n")
	expectPrintedNormalAndMangle(t, "let b; a = null == b.x ? c : b.x", "let b;\na = null == b.x ? c : b.x;\n", "let b;\na = b.x == null ? c : b.x;\n")
	expectPrintedNormalAndMangle(t, "let b; a = null != b.x ? b.x : c", "let b;\na = null != b.x ? b.x : c;\n", "let b;\na = b.x != null ? b.x : c;\n")

	// Don't do this for strict equality comparisons
	expectPrintedNormalAndMangle(t, "let b; a = b === null ? c : b", "let b;\na = b === null ? c : b;\n", "let b;\na = b === null ? c : b;\n")
	expectPrintedNormalAndMangle(t, "let b; a = b !== null ? b : c", "let b;\na = b !== null ? b : c;\n", "let b;\na = b !== null ? b : c;\n")
	expectPrintedNormalAndMangle(t, "let b; a = null === b ? c : b", "let b;\na = null === b ? c : b;\n", "let b;\na = b === null ? c : b;\n")
	expectPrintedNormalAndMangle(t, "let b; a = null !== b ? b : c", "let b;\na = null !== b ? b : c;\n", "let b;\na = b !== null ? b : c;\n")

	expectPrintedNormalAndMangle(t, "let b; a = null === b || b === undefined ? c : b", "let b;\na = null === b || b === void 0 ? c : b;\n", "let b;\na = b ?? c;\n")
	expectPrintedNormalAndMangle(t, "let b; a = b !== undefined && b !== null ? b : c", "let b;\na = b !== void 0 && b !== null ? b : c;\n", "let b;\na = b ?? c;\n")

	// Distinguish between negative an non-negative zero (i.e. Object.is)
	// https://developer.mozilla.org/en-US/docs/Web/JavaScript/Equality_comparisons_and_sameness
	expectPrintedNormalAndMangle(t, "a(b ? 0 : 0)", "a(b ? 0 : 0);\n", "a((b, 0));\n")
	expectPrintedNormalAndMangle(t, "a(b ? +0 : -0)", "a(b ? 0 : -0);\n", "a(b ? 0 : -0);\n")
	expectPrintedNormalAndMangle(t, "a(b ? +0 : 0)", "a(b ? 0 : 0);\n", "a((b, 0));\n")
	expectPrintedNormalAndMangle(t, "a(b ? -0 : 0)", "a(b ? -0 : 0);\n", "a(b ? -0 : 0);\n")

	expectPrintedNormalAndMangle(t, "a ? b : b", "a ? b : b;\n", "a, b;\n")
	expectPrintedNormalAndMangle(t, "let a; a ? b : b", "let a;\na ? b : b;\n", "let a;\nb;\n")

	expectPrintedNormalAndMangle(t, "a ? -b : -b", "a ? -b : -b;\n", "a, -b;\n")
	expectPrintedNormalAndMangle(t, "a ? b.c : b.c", "a ? b.c : b.c;\n", "a, b.c;\n")
	expectPrintedNormalAndMangle(t, "a ? b?.c : b?.c", "a ? b?.c : b?.c;\n", "a, b?.c;\n")
	expectPrintedNormalAndMangle(t, "a ? b[c] : b[c]", "a ? b[c] : b[c];\n", "a, b[c];\n")
	expectPrintedNormalAndMangle(t, "a ? b() : b()", "a ? b() : b();\n", "a, b();\n")
	expectPrintedNormalAndMangle(t, "a ? b?.() : b?.()", "a ? b?.() : b?.();\n", "a, b?.();\n")
	expectPrintedNormalAndMangle(t, "a ? b?.[c] : b?.[c]", "a ? b?.[c] : b?.[c];\n", "a, b?.[c];\n")
	expectPrintedNormalAndMangle(t, "a ? b == c : b == c", "a ? b == c : b == c;\n", "a, b == c;\n")
	expectPrintedNormalAndMangle(t, "a ? b.c(d + e[f]) : b.c(d + e[f])", "a ? b.c(d + e[f]) : b.c(d + e[f]);\n", "a, b.c(d + e[f]);\n")

	expectPrintedNormalAndMangle(t, "a ? -b : !b", "a ? -b : !b;\n", "a ? -b : b;\n")
	expectPrintedNormalAndMangle(t, "a ? b() : b(c)", "a ? b() : b(c);\n", "a ? b() : b(c);\n")
	expectPrintedNormalAndMangle(t, "a ? b(c) : b(d)", "a ? b(c) : b(d);\n", "a ? b(c) : b(d);\n")
	expectPrintedNormalAndMangle(t, "a ? b?.c : b.c", "a ? b?.c : b.c;\n", "a ? b?.c : b.c;\n")
	expectPrintedNormalAndMangle(t, "a ? b?.() : b()", "a ? b?.() : b();\n", "a ? b?.() : b();\n")
	expectPrintedNormalAndMangle(t, "a ? b?.[c] : b[c]", "a ? b?.[c] : b[c];\n", "a ? b?.[c] : b[c];\n")
	expectPrintedNormalAndMangle(t, "a ? b == c : b != c", "a ? b == c : b != c;\n", "a ? b == c : b != c;\n")
	expectPrintedNormalAndMangle(t, "a ? b.c(d + e[f]) : b.c(d + e[g])", "a ? b.c(d + e[f]) : b.c(d + e[g]);\n", "a ? b.c(d + e[f]) : b.c(d + e[g]);\n")

	expectPrintedNormalAndMangle(t, "(a, b) ? c : d", "(a, b) ? c : d;\n", "a, b ? c : d;\n")

	expectPrintedNormalAndMangle(t, "return a && ((b && c) && (d && e))", "return a && (b && c && (d && e));\n", "return a && b && c && d && e;\n")
	expectPrintedNormalAndMangle(t, "return a || ((b || c) || (d || e))", "return a || (b || c || (d || e));\n", "return a || b || c || d || e;\n")
	expectPrintedNormalAndMangle(t, "return a ?? ((b ?? c) ?? (d ?? e))", "return a ?? (b ?? c ?? (d ?? e));\n", "return a ?? b ?? c ?? d ?? e;\n")
	expectPrintedNormalAndMangle(t, "if (a) if (b) if (c) d", "if (a) {\n  if (b) {\n    if (c)\n      d;\n  }\n}\n", "a && b && c && d;\n")
	expectPrintedNormalAndMangle(t, "if (!a) if (!b) if (!c) d", "if (!a) {\n  if (!b) {\n    if (!c)\n      d;\n  }\n}\n", "a || b || c || d;\n")
	expectPrintedNormalAndMangle(t, "let a, b, c; return a != null ? a : b != null ? b : c", "let a, b, c;\nreturn a != null ? a : b != null ? b : c;\n", "let a, b, c;\nreturn a ?? b ?? c;\n")

	expectPrintedMangle(t, "if (a) return c; if (b) return d;", "if (a)\n  return c;\nif (b)\n  return d;\n")
	expectPrintedMangle(t, "if (a) return c; if (b) return c;", "if (a || b)\n  return c;\n")
	expectPrintedMangle(t, "if (a) return c; if (b) return;", "if (a)\n  return c;\nif (b)\n  return;\n")
	expectPrintedMangle(t, "if (a) return; if (b) return c;", "if (a)\n  return;\nif (b)\n  return c;\n")
	expectPrintedMangle(t, "if (a) return; if (b) return;", "if (a || b)\n  return;\n")
	expectPrintedMangle(t, "if (a) throw c; if (b) throw d;", "if (a)\n  throw c;\nif (b)\n  throw d;\n")
	expectPrintedMangle(t, "if (a) throw c; if (b) throw c;", "if (a || b)\n  throw c;\n")
	expectPrintedMangle(t, "while (x) { if (a) break; if (b) break; }", "for (; x && !(a || b); )\n  ;\n")
	expectPrintedMangle(t, "while (x) { if (a) continue; if (b) continue; }", "for (; x; )\n  a || b;\n")
	expectPrintedMangle(t, "while (x) { debugger; if (a) break; if (b) break; }", "for (; x; ) {\n  debugger;\n  if (a || b)\n    break;\n}\n")
	expectPrintedMangle(t, "while (x) { debugger; if (a) continue; if (b) continue; }", "for (; x; ) {\n  debugger;\n  a || b;\n}\n")
	expectPrintedMangle(t, "x: while (x) y: while (y) { if (a) break x; if (b) break y; }",
		"x:\n  for (; x; )\n    y:\n      for (; y; ) {\n        if (a)\n          break x;\n        if (b)\n          break y;\n      }\n")
	expectPrintedMangle(t, "x: while (x) y: while (y) { if (a) continue x; if (b) continue y; }",
		"x:\n  for (; x; )\n    y:\n      for (; y; ) {\n        if (a)\n          continue x;\n        if (b)\n          continue y;\n      }\n")
	expectPrintedMangle(t, "x: while (x) y: while (y) { if (a) break x; if (b) break x; }",
		"x:\n  for (; x; )\n    y:\n      for (; y; )\n        if (a || b)\n          break x;\n")
	expectPrintedMangle(t, "x: while (x) y: while (y) { if (a) continue x; if (b) continue x; }",
		"x:\n  for (; x; )\n    y:\n      for (; y; )\n        if (a || b)\n          continue x;\n")

	expectPrintedNormalAndMangle(t, "if (x ? y : 0) foo()", "if (x ? y : 0)\n  foo();\n", "x && y && foo();\n")
	expectPrintedNormalAndMangle(t, "if (x ? y : 1) foo()", "if (x ? y : 1)\n  foo();\n", "(!x || y) && foo();\n")
	expectPrintedNormalAndMangle(t, "if (x ? 0 : y) foo()", "if (x ? 0 : y)\n  foo();\n", "!x && y && foo();\n")
	expectPrintedNormalAndMangle(t, "if (x ? 1 : y) foo()", "if (x ? 1 : y)\n  foo();\n", "(x || y) && foo();\n")

	expectPrintedNormalAndMangle(t, "if (x ? y : 0) ; else foo()", "if (x ? y : 0)\n  ;\nelse\n  foo();\n", "x && y || foo();\n")
	expectPrintedNormalAndMangle(t, "if (x ? y : 1) ; else foo()", "if (x ? y : 1)\n  ;\nelse\n  foo();\n", "!x || y || foo();\n")
	expectPrintedNormalAndMangle(t, "if (x ? 0 : y) ; else foo()", "if (x ? 0 : y)\n  ;\nelse\n  foo();\n", "!x && y || foo();\n")
	expectPrintedNormalAndMangle(t, "if (x ? 1 : y) ; else foo()", "if (x ? 1 : y)\n  ;\nelse\n  foo();\n", "x || y || foo();\n")

	expectPrintedNormalAndMangle(t, "(x ? y : 0) && foo();", "(x ? y : 0) && foo();\n", "x && y && foo();\n")
	expectPrintedNormalAndMangle(t, "(x ? y : 1) && foo();", "(x ? y : 1) && foo();\n", "(!x || y) && foo();\n")
	expectPrintedNormalAndMangle(t, "(x ? 0 : y) && foo();", "(x ? 0 : y) && foo();\n", "!x && y && foo();\n")
	expectPrintedNormalAndMangle(t, "(x ? 1 : y) && foo();", "(x ? 1 : y) && foo();\n", "(x || y) && foo();\n")

	expectPrintedNormalAndMangle(t, "(x ? y : 0) || foo();", "(x ? y : 0) || foo();\n", "x && y || foo();\n")
	expectPrintedNormalAndMangle(t, "(x ? y : 1) || foo();", "(x ? y : 1) || foo();\n", "!x || y || foo();\n")
	expectPrintedNormalAndMangle(t, "(x ? 0 : y) || foo();", "(x ? 0 : y) || foo();\n", "!x && y || foo();\n")
	expectPrintedNormalAndMangle(t, "(x ? 1 : y) || foo();", "(x ? 1 : y) || foo();\n", "x || y || foo();\n")

	expectPrintedNormalAndMangle(t, "if (!!a || !!b) throw 0", "if (!!a || !!b)\n  throw 0;\n", "if (a || b)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (!!a && !!b) throw 0", "if (!!a && !!b)\n  throw 0;\n", "if (a && b)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (!!a ? !!b : !!c) throw 0", "if (!!a ? !!b : !!c)\n  throw 0;\n", "if (a ? b : c)\n  throw 0;\n")

	expectPrintedNormalAndMangle(t, "if ((a + b) !== 0) throw 0", "if (a + b !== 0)\n  throw 0;\n", "if (a + b !== 0)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if ((a | b) !== 0) throw 0", "if ((a | b) !== 0)\n  throw 0;\n", "if (a | b)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if ((a & b) !== 0) throw 0", "if ((a & b) !== 0)\n  throw 0;\n", "if (a & b)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if ((a ^ b) !== 0) throw 0", "if ((a ^ b) !== 0)\n  throw 0;\n", "if (a ^ b)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if ((a << b) !== 0) throw 0", "if (a << b !== 0)\n  throw 0;\n", "if (a << b)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if ((a >> b) !== 0) throw 0", "if (a >> b !== 0)\n  throw 0;\n", "if (a >> b)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if ((a >>> b) !== 0) throw 0", "if (a >>> b !== 0)\n  throw 0;\n", "if (a >>> b)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (+a !== 0) throw 0", "if (+a !== 0)\n  throw 0;\n", "if (+a != 0)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (~a !== 0) throw 0", "if (~a !== 0)\n  throw 0;\n", "if (~a)\n  throw 0;\n")

	expectPrintedNormalAndMangle(t, "if (0 != (a + b)) throw 0", "if (0 != a + b)\n  throw 0;\n", "if (a + b != 0)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (0 != (a | b)) throw 0", "if (0 != (a | b))\n  throw 0;\n", "if (a | b)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (0 != (a & b)) throw 0", "if (0 != (a & b))\n  throw 0;\n", "if (a & b)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (0 != (a ^ b)) throw 0", "if (0 != (a ^ b))\n  throw 0;\n", "if (a ^ b)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (0 != (a << b)) throw 0", "if (0 != a << b)\n  throw 0;\n", "if (a << b)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (0 != (a >> b)) throw 0", "if (0 != a >> b)\n  throw 0;\n", "if (a >> b)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (0 != (a >>> b)) throw 0", "if (0 != a >>> b)\n  throw 0;\n", "if (a >>> b)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (0 != +a) throw 0", "if (0 != +a)\n  throw 0;\n", "if (+a != 0)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (0 != ~a) throw 0", "if (0 != ~a)\n  throw 0;\n", "if (~a)\n  throw 0;\n")

	expectPrintedNormalAndMangle(t, "if ((a + b) === 0) throw 0", "if (a + b === 0)\n  throw 0;\n", "if (a + b === 0)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if ((a | b) === 0) throw 0", "if ((a | b) === 0)\n  throw 0;\n", "if (!(a | b))\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if ((a & b) === 0) throw 0", "if ((a & b) === 0)\n  throw 0;\n", "if (!(a & b))\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if ((a ^ b) === 0) throw 0", "if ((a ^ b) === 0)\n  throw 0;\n", "if (!(a ^ b))\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if ((a << b) === 0) throw 0", "if (a << b === 0)\n  throw 0;\n", "if (!(a << b))\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if ((a >> b) === 0) throw 0", "if (a >> b === 0)\n  throw 0;\n", "if (!(a >> b))\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if ((a >>> b) === 0) throw 0", "if (a >>> b === 0)\n  throw 0;\n", "if (!(a >>> b))\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (+a === 0) throw 0", "if (+a === 0)\n  throw 0;\n", "if (+a == 0)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (~a === 0) throw 0", "if (~a === 0)\n  throw 0;\n", "if (!~a)\n  throw 0;\n")

	expectPrintedNormalAndMangle(t, "if (0 == (a + b)) throw 0", "if (0 == a + b)\n  throw 0;\n", "if (a + b == 0)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (0 == (a | b)) throw 0", "if (0 == (a | b))\n  throw 0;\n", "if (!(a | b))\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (0 == (a & b)) throw 0", "if (0 == (a & b))\n  throw 0;\n", "if (!(a & b))\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (0 == (a ^ b)) throw 0", "if (0 == (a ^ b))\n  throw 0;\n", "if (!(a ^ b))\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (0 == (a << b)) throw 0", "if (0 == a << b)\n  throw 0;\n", "if (!(a << b))\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (0 == (a >> b)) throw 0", "if (0 == a >> b)\n  throw 0;\n", "if (!(a >> b))\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (0 == (a >>> b)) throw 0", "if (0 == a >>> b)\n  throw 0;\n", "if (!(a >>> b))\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (0 == +a) throw 0", "if (0 == +a)\n  throw 0;\n", "if (+a == 0)\n  throw 0;\n")
	expectPrintedNormalAndMangle(t, "if (0 == ~a) throw 0", "if (0 == ~a)\n  throw 0;\n", "if (!~a)\n  throw 0;\n")
}

func TestMangleWrapToAvoidAmbiguousElse(t *testing.T) {
	expectPrintedMangle(t, "if (a) { if (b) return c } else return d", "if (a) {\n  if (b)\n    return c;\n} else\n  return d;\n")
	expectPrintedMangle(t, "if (a) while (1) { if (b) return c } else return d", "if (a) {\n  for (; ; )\n    if (b)\n      return c;\n} else\n  return d;\n")
	expectPrintedMangle(t, "if (a) for (;;) { if (b) return c } else return d", "if (a) {\n  for (; ; )\n    if (b)\n      return c;\n} else\n  return d;\n")
	expectPrintedMangle(t, "if (a) for (x in y) { if (b) return c } else return d", "if (a) {\n  for (x in y)\n    if (b)\n      return c;\n} else\n  return d;\n")
	expectPrintedMangle(t, "if (a) for (x of y) { if (b) return c } else return d", "if (a) {\n  for (x of y)\n    if (b)\n      return c;\n} else\n  return d;\n")
	expectPrintedMangle(t, "if (a) with (x) { if (b) return c } else return d", "if (a) {\n  with (x)\n    if (b)\n      return c;\n} else\n  return d;\n")
	expectPrintedMangle(t, "if (a) x: { if (b) return c } else return d", "if (a) {\n  x:\n    if (b)\n      return c;\n} else\n  return d;\n")
}

func TestMangleOptionalChain(t *testing.T) {
	expectPrintedMangle(t, "let a; return a != null ? a.b : undefined", "let a;\nreturn a?.b;\n")
	expectPrintedMangle(t, "let a; return a != null ? a[b] : undefined", "let a;\nreturn a?.[b];\n")
	expectPrintedMangle(t, "let a; return a != null ? a(b) : undefined", "let a;\nreturn a?.(b);\n")

	expectPrintedMangle(t, "let a; return a == null ? undefined : a.b", "let a;\nreturn a?.b;\n")
	expectPrintedMangle(t, "let a; return a == null ? undefined : a[b]", "let a;\nreturn a?.[b];\n")
	expectPrintedMangle(t, "let a; return a == null ? undefined : a(b)", "let a;\nreturn a?.(b);\n")

	expectPrintedMangle(t, "let a; return null != a ? a.b : undefined", "let a;\nreturn a?.b;\n")
	expectPrintedMangle(t, "let a; return null != a ? a[b] : undefined", "let a;\nreturn a?.[b];\n")
	expectPrintedMangle(t, "let a; return null != a ? a(b) : undefined", "let a;\nreturn a?.(b);\n")

	expectPrintedMangle(t, "let a; return null == a ? undefined : a.b", "let a;\nreturn a?.b;\n")
	expectPrintedMangle(t, "let a; return null == a ? undefined : a[b]", "let a;\nreturn a?.[b];\n")
	expectPrintedMangle(t, "let a; return null == a ? undefined : a(b)", "let a;\nreturn a?.(b);\n")

	expectPrintedMangle(t, "return a != null ? a.b : undefined", "return a != null ? a.b : void 0;\n")
	expectPrintedMangle(t, "let a; return a != null ? a.b : null", "let a;\nreturn a != null ? a.b : null;\n")
	expectPrintedMangle(t, "let a; return a != null ? b.a : undefined", "let a;\nreturn a != null ? b.a : void 0;\n")
	expectPrintedMangle(t, "let a; return a != 0 ? a.b : undefined", "let a;\nreturn a != 0 ? a.b : void 0;\n")
	expectPrintedMangle(t, "let a; return a !== null ? a.b : undefined", "let a;\nreturn a !== null ? a.b : void 0;\n")
	expectPrintedMangle(t, "let a; return a != undefined ? a.b : undefined", "let a;\nreturn a?.b;\n")
	expectPrintedMangle(t, "let a; return a != null ? a?.b : undefined", "let a;\nreturn a?.b;\n")
	expectPrintedMangle(t, "let a; return a != null ? a.b.c[d](e) : undefined", "let a;\nreturn a?.b.c[d](e);\n")
	expectPrintedMangle(t, "let a; return a != null ? a?.b.c[d](e) : undefined", "let a;\nreturn a?.b.c[d](e);\n")
	expectPrintedMangle(t, "let a; return a != null ? a.b.c?.[d](e) : undefined", "let a;\nreturn a?.b.c?.[d](e);\n")
	expectPrintedMangle(t, "let a; return a != null ? a?.b.c?.[d](e) : undefined", "let a;\nreturn a?.b.c?.[d](e);\n")
	expectPrintedMangleTarget(t, 2019, "let a; return a != null ? a.b : undefined", "let a;\nreturn a != null ? a.b : void 0;\n")
	expectPrintedMangleTarget(t, 2020, "let a; return a != null ? a.b : undefined", "let a;\nreturn a?.b;\n")

	expectPrintedMangle(t, "a != null && a.b()", "a?.b();\n")
	expectPrintedMangle(t, "a == null || a.b()", "a?.b();\n")
	expectPrintedMangle(t, "null != a && a.b()", "a?.b();\n")
	expectPrintedMangle(t, "null == a || a.b()", "a?.b();\n")

	expectPrintedMangle(t, "a == null && a.b()", "a == null && a.b();\n")
	expectPrintedMangle(t, "a != null || a.b()", "a != null || a.b();\n")
	expectPrintedMangle(t, "null == a && a.b()", "a == null && a.b();\n")
	expectPrintedMangle(t, "null != a || a.b()", "a != null || a.b();\n")

	expectPrintedMangle(t, "x = a != null && a.b()", "x = a != null && a.b();\n")
	expectPrintedMangle(t, "x = a == null || a.b()", "x = a == null || a.b();\n")

	expectPrintedMangle(t, "if (a != null) a.b()", "a?.b();\n")
	expectPrintedMangle(t, "if (a == null) ; else a.b()", "a?.b();\n")

	expectPrintedMangle(t, "if (a == null) a.b()", "a == null && a.b();\n")
	expectPrintedMangle(t, "if (a != null) ; else a.b()", "a != null || a.b();\n")
}

func TestMangleNullOrUndefinedWithSideEffects(t *testing.T) {
	expectPrintedNormalAndMangle(t, "x(y ?? 1)", "x(y ?? 1);\n", "x(y ?? 1);\n")
	expectPrintedNormalAndMangle(t, "x(y.z ?? 1)", "x(y.z ?? 1);\n", "x(y.z ?? 1);\n")
	expectPrintedNormalAndMangle(t, "x(y[z] ?? 1)", "x(y[z] ?? 1);\n", "x(y[z] ?? 1);\n")

	expectPrintedNormalAndMangle(t, "x(0 ?? 1)", "x(0);\n", "x(0);\n")
	expectPrintedNormalAndMangle(t, "x(0n ?? 1)", "x(0n);\n", "x(0n);\n")
	expectPrintedNormalAndMangle(t, "x('' ?? 1)", "x(\"\");\n", "x(\"\");\n")
	expectPrintedNormalAndMangle(t, "x(/./ ?? 1)", "x(/./);\n", "x(/./);\n")
	expectPrintedNormalAndMangle(t, "x({} ?? 1)", "x({});\n", "x({});\n")
	expectPrintedNormalAndMangle(t, "x((() => {}) ?? 1)", "x(() => {\n});\n", "x(() => {\n});\n")
	expectPrintedNormalAndMangle(t, "x(class {} ?? 1)", "x(class {\n});\n", "x(class {\n});\n")
	expectPrintedNormalAndMangle(t, "x(function() {} ?? 1)", "x(function() {\n});\n", "x(function() {\n});\n")

	expectPrintedNormalAndMangle(t, "x(null ?? 1)", "x(1);\n", "x(1);\n")
	expectPrintedNormalAndMangle(t, "x(undefined ?? 1)", "x(1);\n", "x(1);\n")

	expectPrintedNormalAndMangle(t, "x(void y ?? 1)", "x(void y ?? 1);\n", "x(void y ?? 1);\n")
	expectPrintedNormalAndMangle(t, "x(-y ?? 1)", "x(-y);\n", "x(-y);\n")
	expectPrintedNormalAndMangle(t, "x(+y ?? 1)", "x(+y);\n", "x(+y);\n")
	expectPrintedNormalAndMangle(t, "x(!y ?? 1)", "x(!y);\n", "x(!y);\n")
	expectPrintedNormalAndMangle(t, "x(~y ?? 1)", "x(~y);\n", "x(~y);\n")
	expectPrintedNormalAndMangle(t, "x(--y ?? 1)", "x(--y);\n", "x(--y);\n")
	expectPrintedNormalAndMangle(t, "x(++y ?? 1)", "x(++y);\n", "x(++y);\n")
	expectPrintedNormalAndMangle(t, "x(y-- ?? 1)", "x(y--);\n", "x(y--);\n")
	expectPrintedNormalAndMangle(t, "x(y++ ?? 1)", "x(y++);\n", "x(y++);\n")
	expectPrintedNormalAndMangle(t, "x(delete y ?? 1)", "x(delete y);\n", "x(delete y);\n")
	expectPrintedNormalAndMangle(t, "x(typeof y ?? 1)", "x(typeof y);\n", "x(typeof y);\n")

	expectPrintedNormalAndMangle(t, "x((y, 0) ?? 1)", "x((y, 0));\n", "x((y, 0));\n")
	expectPrintedNormalAndMangle(t, "x((y, !z) ?? 1)", "x((y, !z));\n", "x((y, !z));\n")
	expectPrintedNormalAndMangle(t, "x((y, null) ?? 1)", "x((y, null) ?? 1);\n", "x((y, null ?? 1));\n")
	expectPrintedNormalAndMangle(t, "x((y, void z) ?? 1)", "x((y, void z) ?? 1);\n", "x((y, void z ?? 1));\n")

	expectPrintedNormalAndMangle(t, "x((y + z) ?? 1)", "x(y + z);\n", "x(y + z);\n")
	expectPrintedNormalAndMangle(t, "x((y - z) ?? 1)", "x(y - z);\n", "x(y - z);\n")
	expectPrintedNormalAndMangle(t, "x((y * z) ?? 1)", "x(y * z);\n", "x(y * z);\n")
	expectPrintedNormalAndMangle(t, "x((y / z) ?? 1)", "x(y / z);\n", "x(y / z);\n")
	expectPrintedNormalAndMangle(t, "x((y % z) ?? 1)", "x(y % z);\n", "x(y % z);\n")
	expectPrintedNormalAndMangle(t, "x((y ** z) ?? 1)", "x(y ** z);\n", "x(y ** z);\n")
	expectPrintedNormalAndMangle(t, "x((y << z) ?? 1)", "x(y << z);\n", "x(y << z);\n")
	expectPrintedNormalAndMangle(t, "x((y >> z) ?? 1)", "x(y >> z);\n", "x(y >> z);\n")
	expectPrintedNormalAndMangle(t, "x((y >>> z) ?? 1)", "x(y >>> z);\n", "x(y >>> z);\n")
	expectPrintedNormalAndMangle(t, "x((y | z) ?? 1)", "x(y | z);\n", "x(y | z);\n")
	expectPrintedNormalAndMangle(t, "x((y & z) ?? 1)", "x(y & z);\n", "x(y & z);\n")
	expectPrintedNormalAndMangle(t, "x((y ^ z) ?? 1)", "x(y ^ z);\n", "x(y ^ z);\n")
	expectPrintedNormalAndMangle(t, "x((y < z) ?? 1)", "x(y < z);\n", "x(y < z);\n")
	expectPrintedNormalAndMangle(t, "x((y > z) ?? 1)", "x(y > z);\n", "x(y > z);\n")
	expectPrintedNormalAndMangle(t, "x((y <= z) ?? 1)", "x(y <= z);\n", "x(y <= z);\n")
	expectPrintedNormalAndMangle(t, "x((y >= z) ?? 1)", "x(y >= z);\n", "x(y >= z);\n")
	expectPrintedNormalAndMangle(t, "x((y == z) ?? 1)", "x(y == z);\n", "x(y == z);\n")
	expectPrintedNormalAndMangle(t, "x((y != z) ?? 1)", "x(y != z);\n", "x(y != z);\n")
	expectPrintedNormalAndMangle(t, "x((y === z) ?? 1)", "x(y === z);\n", "x(y === z);\n")
	expectPrintedNormalAndMangle(t, "x((y !== z) ?? 1)", "x(y !== z);\n", "x(y !== z);\n")

	expectPrintedNormalAndMangle(t, "x((y || z) ?? 1)", "x((y || z) ?? 1);\n", "x((y || z) ?? 1);\n")
	expectPrintedNormalAndMangle(t, "x((y && z) ?? 1)", "x((y && z) ?? 1);\n", "x((y && z) ?? 1);\n")
	expectPrintedNormalAndMangle(t, "x((y ?? z) ?? 1)", "x(y ?? z ?? 1);\n", "x(y ?? z ?? 1);\n")
}

func TestMangleBooleanWithSideEffects(t *testing.T) {
	falsyNoSideEffects := []string{"false", "\"\"", "0", "0n", "null", "void 0"}
	truthyNoSideEffects := []string{"true", "\" \"", "1", "1n", "/./", "(() => {\n})", "function() {\n}"}

	for _, value := range falsyNoSideEffects {
		expectPrintedMangle(t, "y(x && "+value+")", "y(x && "+value+");\n")
		expectPrintedMangle(t, "y(x || "+value+")", "y(x || "+value+");\n")

		expectPrintedMangle(t, "y(!(x && "+value+"))", "y(!(x && "+value+"));\n")
		expectPrintedMangle(t, "y(!(x || "+value+"))", "y(!x);\n")

		expectPrintedMangle(t, "if (x && "+value+") y", "x;\n")
		expectPrintedMangle(t, "if (x || "+value+") y", "x && y;\n")

		expectPrintedMangle(t, "if (x && "+value+") y; else z", "x, z;\n")
		expectPrintedMangle(t, "if (x || "+value+") y; else z", "x ? y : z;\n")

		expectPrintedMangle(t, "y(x && "+value+" ? y : z)", "y((x, z));\n")
		expectPrintedMangle(t, "y(x || "+value+" ? y : z)", "y(x ? y : z);\n")

		expectPrintedMangle(t, "while ("+value+") x()", "for (; "+value+"; )\n  x();\n")
		expectPrintedMangle(t, "for (; "+value+"; ) x()", "for (; "+value+"; )\n  x();\n")
	}

	for _, value := range truthyNoSideEffects {
		expectPrintedMangle(t, "y(x && "+value+")", "y(x && "+value+");\n")
		expectPrintedMangle(t, "y(x || "+value+")", "y(x || "+value+");\n")

		expectPrintedMangle(t, "y(!(x && "+value+"))", "y(!x);\n")
		expectPrintedMangle(t, "y(!(x || "+value+"))", "y(!(x || "+value+"));\n")

		expectPrintedMangle(t, "if (x && "+value+") y", "x && y;\n")
		expectPrintedMangle(t, "if (x || "+value+") y", "x, y;\n")

		expectPrintedMangle(t, "if (x && "+value+") y; else z", "x ? y : z;\n")
		expectPrintedMangle(t, "if (x || "+value+") y; else z", "x, y;\n")

		expectPrintedMangle(t, "y(x && "+value+" ? y : z)", "y(x ? y : z);\n")
		expectPrintedMangle(t, "y(x || "+value+" ? y : z)", "y((x, y));\n")

		expectPrintedMangle(t, "while ("+value+") x()", "for (; ; )\n  x();\n")
		expectPrintedMangle(t, "for (; "+value+"; ) x()", "for (; ; )\n  x();\n")
	}

	falsyHasSideEffects := []string{"void foo()"}
	truthyHasSideEffects := []string{"typeof foo()", "[foo()]", "{ [foo()]: 0 }"}

	for _, value := range falsyHasSideEffects {
		expectPrintedMangle(t, "y(x && "+value+")", "y(x && "+value+");\n")
		expectPrintedMangle(t, "y(x || "+value+")", "y(x || "+value+");\n")

		expectPrintedMangle(t, "y(!(x && "+value+"))", "y(!(x && "+value+"));\n")
		expectPrintedMangle(t, "y(!(x || "+value+"))", "y(!(x || "+value+"));\n")

		expectPrintedMangle(t, "if (x || "+value+") y", "(x || "+value+") && y;\n")
		expectPrintedMangle(t, "if (x || "+value+") y; else z", "x || "+value+" ? y : z;\n")
		expectPrintedMangle(t, "y(x || "+value+" ? y : z)", "y(x || "+value+" ? y : z);\n")

		expectPrintedMangle(t, "while ("+value+") x()", "for (; "+value+"; )\n  x();\n")
		expectPrintedMangle(t, "for (; "+value+"; ) x()", "for (; "+value+"; )\n  x();\n")
	}

	for _, value := range truthyHasSideEffects {
		expectPrintedMangle(t, "y(x && "+value+")", "y(x && "+value+");\n")
		expectPrintedMangle(t, "y(x || "+value+")", "y(x || "+value+");\n")

		expectPrintedMangle(t, "y(!(x || "+value+"))", "y(!(x || "+value+"));\n")
		expectPrintedMangle(t, "y(!(x && "+value+"))", "y(!(x && "+value+"));\n")

		expectPrintedMangle(t, "if (x && "+value+") y", "x && "+value+" && y;\n")
		expectPrintedMangle(t, "if (x && "+value+") y; else z", "x && "+value+" ? y : z;\n")
		expectPrintedMangle(t, "y(x && "+value+" ? y : z)", "y(x && "+value+" ? y : z);\n")

		expectPrintedMangle(t, "while ("+value+") x()", "for (; "+value+"; )\n  x();\n")
		expectPrintedMangle(t, "for (; "+value+"; ) x()", "for (; "+value+"; )\n  x();\n")
	}
}

func TestMangleReturn(t *testing.T) {
	expectPrintedMangle(t, "function foo() { x(); return; }", "function foo() {\n  x();\n}\n")
	expectPrintedMangle(t, "let foo = function() { x(); return; }", "let foo = function() {\n  x();\n};\n")
	expectPrintedMangle(t, "let foo = () => { x(); return; }", "let foo = () => {\n  x();\n};\n")
	expectPrintedMangle(t, "function foo() { x(); return y; }", "function foo() {\n  return x(), y;\n}\n")
	expectPrintedMangle(t, "let foo = function() { x(); return y; }", "let foo = function() {\n  return x(), y;\n};\n")
	expectPrintedMangle(t, "let foo = () => { x(); return y; }", "let foo = () => (x(), y);\n")

	// Don't trim a trailing top-level return because we may be compiling a partial module
	expectPrintedMangle(t, "x(); return;", "x();\nreturn;\n")

	expectPrintedMangle(t, "function foo() { a = b; if (a) return a; if (b) c = b; return c; }",
		"function foo() {\n  return a = b, a || (b && (c = b), c);\n}\n")
	expectPrintedMangle(t, "function foo() { a = b; if (a) return; if (b) c = b; return c; }",
		"function foo() {\n  if (a = b, !a)\n    return b && (c = b), c;\n}\n")
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

	expectPrintedMangle(t, "if (a) return b; else return c; return d;\n", "return a ? b : c;\n")

	// Optimize implicit return
	expectPrintedMangle(t, "function x() { if (y) return; z(); }", "function x() {\n  y || z();\n}\n")
	expectPrintedMangle(t, "function x() { if (y) return; else z(); w(); }", "function x() {\n  y || (z(), w());\n}\n")
	expectPrintedMangle(t, "function x() { t(); if (y) return; z(); }", "function x() {\n  t(), !y && z();\n}\n")
	expectPrintedMangle(t, "function x() { t(); if (y) return; else z(); w(); }", "function x() {\n  t(), !y && (z(), w());\n}\n")
	expectPrintedMangle(t, "function x() { debugger; if (y) return; z(); }", "function x() {\n  debugger;\n  y || z();\n}\n")
	expectPrintedMangle(t, "function x() { debugger; if (y) return; else z(); w(); }", "function x() {\n  debugger;\n  y || (z(), w());\n}\n")
	expectPrintedMangle(t, "function x() { if (y) { if (z) return; } }",
		"function x() {\n  y && z;\n}\n")
	expectPrintedMangle(t, "function x() { if (y) { if (z) return; w(); } }",
		"function x() {\n  if (y) {\n    if (z)\n      return;\n    w();\n  }\n}\n")
	expectPrintedMangle(t, "function foo(x) { if (!x.y) {} else return x }", "function foo(x) {\n  if (x.y)\n    return x;\n}\n")
	expectPrintedMangle(t, "function foo(x) { if (!x.y) return undefined; return x }", "function foo(x) {\n  if (x.y)\n    return x;\n}\n")

	// Do not optimize implicit return for statements that care about scope
	expectPrintedMangle(t, "function x() { if (y) return; function y() {} }", "function x() {\n  if (y)\n    return;\n  function y() {\n  }\n}\n")
	expectPrintedMangle(t, "function x() { if (y) return; let y }", "function x() {\n  if (y)\n    return;\n  let y;\n}\n")
	expectPrintedMangle(t, "function x() { if (y) return; var y }", "function x() {\n  if (!y)\n    var y;\n}\n")
}

func TestMangleThrow(t *testing.T) {
	expectPrintedNormalAndMangle(t,
		"function foo() { a = b; if (a) throw a; if (b) c = b; throw c; }",
		"function foo() {\n  a = b;\n  if (a)\n    throw a;\n  if (b)\n    c = b;\n  throw c;\n}\n",
		"function foo() {\n  throw a = b, a || (b && (c = b), c);\n}\n")
	expectPrintedNormalAndMangle(t,
		"function foo() { if (!a) throw b; throw c; }",
		"function foo() {\n  if (!a)\n    throw b;\n  throw c;\n}\n",
		"function foo() {\n  throw a ? c : b;\n}\n")

	expectPrintedNormalAndMangle(t, "if (1) throw a(); else throw b()", "if (1)\n  throw a();\nelse\n  throw b();\n", "throw a();\n")
	expectPrintedNormalAndMangle(t, "if (0) throw a(); else throw b()", "if (0)\n  throw a();\nelse\n  throw b();\n", "throw b();\n")
	expectPrintedNormalAndMangle(t, "if (a) throw b(); else throw c()", "if (a)\n  throw b();\nelse\n  throw c();\n", "throw a ? b() : c();\n")
	expectPrintedNormalAndMangle(t, "if (!a) throw b(); else throw c()", "if (!a)\n  throw b();\nelse\n  throw c();\n", "throw a ? c() : b();\n")
	expectPrintedNormalAndMangle(t, "if (!!a) throw b(); else throw c()", "if (!!a)\n  throw b();\nelse\n  throw c();\n", "throw a ? b() : c();\n")
	expectPrintedNormalAndMangle(t, "if (!!!a) throw b(); else throw c()", "if (!!!a)\n  throw b();\nelse\n  throw c();\n", "throw a ? c() : b();\n")

	expectPrintedNormalAndMangle(t, "if (1) throw a(); throw b()", "if (1)\n  throw a();\nthrow b();\n", "throw a();\n")
	expectPrintedNormalAndMangle(t, "if (0) throw a(); throw b()", "if (0)\n  throw a();\nthrow b();\n", "throw b();\n")
	expectPrintedNormalAndMangle(t, "if (a) throw b(); throw c()", "if (a)\n  throw b();\nthrow c();\n", "throw a ? b() : c();\n")
	expectPrintedNormalAndMangle(t, "if (!a) throw b(); throw c()", "if (!a)\n  throw b();\nthrow c();\n", "throw a ? c() : b();\n")
	expectPrintedNormalAndMangle(t, "if (!!a) throw b(); throw c()", "if (!!a)\n  throw b();\nthrow c();\n", "throw a ? b() : c();\n")
	expectPrintedNormalAndMangle(t, "if (!!!a) throw b(); throw c()", "if (!!!a)\n  throw b();\nthrow c();\n", "throw a ? c() : b();\n")
}

func TestMangleInitializer(t *testing.T) {
	expectPrintedNormalAndMangle(t, "const a = undefined", "const a = void 0;\n", "const a = void 0;\n")
	expectPrintedNormalAndMangle(t, "let a = undefined", "let a = void 0;\n", "let a;\n")
	expectPrintedNormalAndMangle(t, "let {} = undefined", "let {} = void 0;\n", "let {} = void 0;\n")
	expectPrintedNormalAndMangle(t, "let [] = undefined", "let [] = void 0;\n", "let [] = void 0;\n")
	expectPrintedNormalAndMangle(t, "var a = undefined", "var a = void 0;\n", "var a = void 0;\n")
	expectPrintedNormalAndMangle(t, "var {} = undefined", "var {} = void 0;\n", "var {} = void 0;\n")
	expectPrintedNormalAndMangle(t, "var [] = undefined", "var [] = void 0;\n", "var [] = void 0;\n")
}

func TestMangleCall(t *testing.T) {
	expectPrintedNormalAndMangle(t, "x = foo(1, ...[], 2)", "x = foo(1, ...[], 2);\n", "x = foo(1, 2);\n")
	expectPrintedNormalAndMangle(t, "x = foo(1, ...2, 3)", "x = foo(1, ...2, 3);\n", "x = foo(1, ...2, 3);\n")
	expectPrintedNormalAndMangle(t, "x = foo(1, ...[2], 3)", "x = foo(1, ...[2], 3);\n", "x = foo(1, 2, 3);\n")
	expectPrintedNormalAndMangle(t, "x = foo(1, ...[2, 3], 4)", "x = foo(1, ...[2, 3], 4);\n", "x = foo(1, 2, 3, 4);\n")
	expectPrintedNormalAndMangle(t, "x = foo(1, ...[2, ...y, 3], 4)", "x = foo(1, ...[2, ...y, 3], 4);\n", "x = foo(1, 2, ...y, 3, 4);\n")
	expectPrintedNormalAndMangle(t, "x = foo(1, ...{a, b}, 4)", "x = foo(1, ...{ a, b }, 4);\n", "x = foo(1, ...{ a, b }, 4);\n")

	// Holes must become undefined
	expectPrintedNormalAndMangle(t, "x = foo(1, ...[,2,,], 3)", "x = foo(1, ...[, 2, ,], 3);\n", "x = foo(1, void 0, 2, void 0, 3);\n")
}

func TestMangleNew(t *testing.T) {
	expectPrintedNormalAndMangle(t, "x = new foo(1, ...[], 2)", "x = new foo(1, ...[], 2);\n", "x = new foo(1, 2);\n")
	expectPrintedNormalAndMangle(t, "x = new foo(1, ...2, 3)", "x = new foo(1, ...2, 3);\n", "x = new foo(1, ...2, 3);\n")
	expectPrintedNormalAndMangle(t, "x = new foo(1, ...[2], 3)", "x = new foo(1, ...[2], 3);\n", "x = new foo(1, 2, 3);\n")
	expectPrintedNormalAndMangle(t, "x = new foo(1, ...[2, 3], 4)", "x = new foo(1, ...[2, 3], 4);\n", "x = new foo(1, 2, 3, 4);\n")
	expectPrintedNormalAndMangle(t, "x = new foo(1, ...[2, ...y, 3], 4)", "x = new foo(1, ...[2, ...y, 3], 4);\n", "x = new foo(1, 2, ...y, 3, 4);\n")
	expectPrintedNormalAndMangle(t, "x = new foo(1, ...{a, b}, 4)", "x = new foo(1, ...{ a, b }, 4);\n", "x = new foo(1, ...{ a, b }, 4);\n")

	// Holes must become undefined
	expectPrintedNormalAndMangle(t, "x = new foo(1, ...[,2,,], 3)", "x = new foo(1, ...[, 2, ,], 3);\n", "x = new foo(1, void 0, 2, void 0, 3);\n")
}

func TestMangleArray(t *testing.T) {
	expectPrintedNormalAndMangle(t, "x = [1, ...[], 2]", "x = [1, ...[], 2];\n", "x = [1, 2];\n")
	expectPrintedNormalAndMangle(t, "x = [1, ...2, 3]", "x = [1, ...2, 3];\n", "x = [1, ...2, 3];\n")
	expectPrintedNormalAndMangle(t, "x = [1, ...[2], 3]", "x = [1, ...[2], 3];\n", "x = [1, 2, 3];\n")
	expectPrintedNormalAndMangle(t, "x = [1, ...[2, 3], 4]", "x = [1, ...[2, 3], 4];\n", "x = [1, 2, 3, 4];\n")
	expectPrintedNormalAndMangle(t, "x = [1, ...[2, ...y, 3], 4]", "x = [1, ...[2, ...y, 3], 4];\n", "x = [1, 2, ...y, 3, 4];\n")
	expectPrintedNormalAndMangle(t, "x = [1, ...{a, b}, 4]", "x = [1, ...{ a, b }, 4];\n", "x = [1, ...{ a, b }, 4];\n")

	// Holes must become undefined, which is different than a hole
	expectPrintedNormalAndMangle(t, "x = [1, ...[,2,,], 3]", "x = [1, ...[, 2, ,], 3];\n", "x = [1, void 0, 2, void 0, 3];\n")
}

func TestMangleObject(t *testing.T) {
	expectPrintedNormalAndMangle(t, "x = {['y']: z}", "x = { [\"y\"]: z };\n", "x = { y: z };\n")
	expectPrintedNormalAndMangle(t, "x = {['y']() {}}", "x = { [\"y\"]() {\n} };\n", "x = { y() {\n} };\n")
	expectPrintedNormalAndMangle(t, "x = {get ['y']() {}}", "x = { get [\"y\"]() {\n} };\n", "x = { get y() {\n} };\n")
	expectPrintedNormalAndMangle(t, "x = {set ['y'](z) {}}", "x = { set [\"y\"](z) {\n} };\n", "x = { set y(z) {\n} };\n")
	expectPrintedNormalAndMangle(t, "x = {async ['y']() {}}", "x = { async [\"y\"]() {\n} };\n", "x = { async y() {\n} };\n")
	expectPrintedNormalAndMangle(t, "({['y']: z} = x)", "({ [\"y\"]: z } = x);\n", "({ y: z } = x);\n")

	expectPrintedNormalAndMangle(t, "x = {a, ...{}, b}", "x = { a, ...{}, b };\n", "x = { a, b };\n")
	expectPrintedNormalAndMangle(t, "x = {a, ...b, c}", "x = { a, ...b, c };\n", "x = { a, ...b, c };\n")
	expectPrintedNormalAndMangle(t, "x = {a, ...{b}, c}", "x = { a, ...{ b }, c };\n", "x = { a, b, c };\n")
	expectPrintedNormalAndMangle(t, "x = {a, ...{b() {}}, c}", "x = { a, ...{ b() {\n} }, c };\n", "x = { a, b() {\n}, c };\n")
	expectPrintedNormalAndMangle(t, "x = {a, ...{b, c}, d}", "x = { a, ...{ b, c }, d };\n", "x = { a, b, c, d };\n")
	expectPrintedNormalAndMangle(t, "x = {a, ...{b, ...y, c}, d}", "x = { a, ...{ b, ...y, c }, d };\n", "x = { a, b, ...y, c, d };\n")
	expectPrintedNormalAndMangle(t, "x = {a, ...[b, c], d}", "x = { a, ...[b, c], d };\n", "x = { a, ...[b, c], d };\n")

	// Computed properties should be ok
	expectPrintedNormalAndMangle(t, "x = {a, ...{[b]: c}, d}", "x = { a, ...{ [b]: c }, d };\n", "x = { a, [b]: c, d };\n")
	expectPrintedNormalAndMangle(t, "x = {a, ...{[b]() {}}, c}", "x = { a, ...{ [b]() {\n} }, c };\n", "x = { a, [b]() {\n}, c };\n")

	// Getters and setters are not supported
	expectPrintedNormalAndMangle(t,
		"x = {a, ...{b, get c() { return y++ }, d}, e}",
		"x = { a, ...{ b, get c() {\n  return y++;\n}, d }, e };\n",
		"x = { a, b, ...{ get c() {\n  return y++;\n}, d }, e };\n")
	expectPrintedNormalAndMangle(t,
		"x = {a, ...{b, set c(_) { throw _ }, d}, e}",
		"x = { a, ...{ b, set c(_) {\n  throw _;\n}, d }, e };\n",
		"x = { a, b, ...{ set c(_) {\n  throw _;\n}, d }, e };\n")

	// "__proto__" is not supported
	expectPrintedNormalAndMangle(t,
		"x = {a, ...{b, __proto__: c, d}, e}",
		"x = { a, ...{ b, __proto__: c, d }, e };\n",
		"x = { a, b, ...{ __proto__: c, d }, e };\n")
	expectPrintedNormalAndMangle(t,
		"x = {a, ...{b, ['__proto__']: c, d}, e}",
		"x = { a, ...{ b, [\"__proto__\"]: c, d }, e };\n",
		"x = { a, b, [\"__proto__\"]: c, d, e };\n")
	expectPrintedNormalAndMangle(t,
		"x = {a, ...{b, __proto__() {}, c}, d}",
		"x = { a, ...{ b, __proto__() {\n}, c }, d };\n",
		"x = { a, b, __proto__() {\n}, c, d };\n")

	// Spread is ignored for certain values
	expectPrintedNormalAndMangle(t, "x = {a, ...true, b}", "x = { a, ...true, b };\n", "x = { a, b };\n")
	expectPrintedNormalAndMangle(t, "x = {a, ...null, b}", "x = { a, ...null, b };\n", "x = { a, b };\n")
	expectPrintedNormalAndMangle(t, "x = {a, ...void 0, b}", "x = { a, ...void 0, b };\n", "x = { a, b };\n")
	expectPrintedNormalAndMangle(t, "x = {a, ...123, b}", "x = { a, ...123, b };\n", "x = { a, b };\n")
	expectPrintedNormalAndMangle(t, "x = {a, ...123n, b}", "x = { a, ...123n, b };\n", "x = { a, b };\n")
	expectPrintedNormalAndMangle(t, "x = {a, .../x/, b}", "x = { a, .../x/, b };\n", "x = { a, b };\n")
	expectPrintedNormalAndMangle(t, "x = {a, ...function(){}, b}", "x = { a, ...function() {\n}, b };\n", "x = { a, b };\n")
	expectPrintedNormalAndMangle(t, "x = {a, ...()=>{}, b}", "x = { a, ...() => {\n}, b };\n", "x = { a, b };\n")
	expectPrintedNormalAndMangle(t, "x = {a, ...'123', b}", "x = { a, ...\"123\", b };\n", "x = { a, ...\"123\", b };\n")
	expectPrintedNormalAndMangle(t, "x = {a, ...[1, 2, 3], b}", "x = { a, ...[1, 2, 3], b };\n", "x = { a, ...[1, 2, 3], b };\n")
	expectPrintedNormalAndMangle(t, "x = {a, ...(()=>{})(), b}", "x = { a, .../* @__PURE__ */ (() => {\n})(), b };\n", "x = { a, .../* @__PURE__ */ (() => {\n})(), b };\n")

	// Check simple cases of object simplification (advanced cases are checked in end-to-end tests)
	expectPrintedNormalAndMangle(t, "x = {['y']: z}.y", "x = { [\"y\"]: z }.y;\n", "x = { y: z }.y;\n")
	expectPrintedNormalAndMangle(t, "x = {['y']: z}.y; var z", "x = { [\"y\"]: z }.y;\nvar z;\n", "x = z;\nvar z;\n")
	expectPrintedNormalAndMangle(t, "x = {foo: foo(), y: 1}.y", "x = { foo: foo(), y: 1 }.y;\n", "x = { foo: foo(), y: 1 }.y;\n")
	expectPrintedNormalAndMangle(t, "x = {foo: /* @__PURE__ */ foo(), y: 1}.y", "x = { foo: /* @__PURE__ */ foo(), y: 1 }.y;\n", "x = 1;\n")
	expectPrintedNormalAndMangle(t, "x = {__proto__: null}.y", "x = { __proto__: null }.y;\n", "x = void 0;\n")
	expectPrintedNormalAndMangle(t, "x = {__proto__: null, y: 1}.y", "x = { __proto__: null, y: 1 }.y;\n", "x = 1;\n")
	expectPrintedNormalAndMangle(t, "x = {__proto__: null}.__proto__", "x = { __proto__: null }.__proto__;\n", "x = void 0;\n")
	expectPrintedNormalAndMangle(t, "x = {['__proto__']: null}.y", "x = { [\"__proto__\"]: null }.y;\n", "x = { [\"__proto__\"]: null }.y;\n")
	expectPrintedNormalAndMangle(t, "x = {['__proto__']: null, y: 1}.y", "x = { [\"__proto__\"]: null, y: 1 }.y;\n", "x = { [\"__proto__\"]: null, y: 1 }.y;\n")
	expectPrintedNormalAndMangle(t, "x = {['__proto__']: null}.__proto__", "x = { [\"__proto__\"]: null }.__proto__;\n", "x = { [\"__proto__\"]: null }.__proto__;\n")

	expectPrintedNormalAndMangle(t, "x = {y: 1}?.y", "x = { y: 1 }?.y;\n", "x = 1;\n")
	expectPrintedNormalAndMangle(t, "x = {y: 1}?.['y']", "x = { y: 1 }?.[\"y\"];\n", "x = 1;\n")
	expectPrintedNormalAndMangle(t, "x = {y: {z: 1}}?.y.z", "x = { y: { z: 1 } }?.y.z;\n", "x = 1;\n")
	expectPrintedNormalAndMangle(t, "x = {y: {z: 1}}?.y?.z", "x = { y: { z: 1 } }?.y?.z;\n", "x = { z: 1 }?.z;\n")
	expectPrintedNormalAndMangle(t, "x = {y() {}}?.y()", "x = { y() {\n} }?.y();\n", "x = { y() {\n} }.y();\n")

	// Don't change the value of "this" for tagged template literals if the original syntax had a value for "this"
	expectPrintedNormalAndMangle(t, "function f(x) { return {x}.x`` }", "function f(x) {\n  return { x }.x``;\n}\n", "function f(x) {\n  return { x }.x``;\n}\n")
	expectPrintedNormalAndMangle(t, "function f(x) { return (0, {x}.x)`` }", "function f(x) {\n  return (0, { x }.x)``;\n}\n", "function f(x) {\n  return x``;\n}\n")
}

func TestMangleObjectJSX(t *testing.T) {
	expectPrintedJSX(t, "x = <foo bar {...{}} />", "x = /* @__PURE__ */ React.createElement(\"foo\", { bar: true, ...{} });\n")
	expectPrintedJSX(t, "x = <foo bar {...null} />", "x = /* @__PURE__ */ React.createElement(\"foo\", { bar: true, ...null });\n")
	expectPrintedJSX(t, "x = <foo bar {...{bar}} />", "x = /* @__PURE__ */ React.createElement(\"foo\", { bar: true, ...{ bar } });\n")
	expectPrintedJSX(t, "x = <foo bar {...bar} />", "x = /* @__PURE__ */ React.createElement(\"foo\", { bar: true, ...bar });\n")

	expectPrintedMangleJSX(t, "x = <foo bar {...{}} />", "x = /* @__PURE__ */ React.createElement(\"foo\", { bar: true });\n")
	expectPrintedMangleJSX(t, "x = <foo bar {...null} />", "x = /* @__PURE__ */ React.createElement(\"foo\", { bar: true });\n")
	expectPrintedMangleJSX(t, "x = <foo bar {...{bar}} />", "x = /* @__PURE__ */ React.createElement(\"foo\", { bar: true, bar });\n")
	expectPrintedMangleJSX(t, "x = <foo bar {...bar} />", "x = /* @__PURE__ */ React.createElement(\"foo\", { bar: true, ...bar });\n")
}

func TestMangleArrow(t *testing.T) {
	expectPrintedNormalAndMangle(t, "var a = () => {}", "var a = () => {\n};\n", "var a = () => {\n};\n")
	expectPrintedNormalAndMangle(t, "var a = () => 123", "var a = () => 123;\n", "var a = () => 123;\n")
	expectPrintedNormalAndMangle(t, "var a = () => void 0", "var a = () => void 0;\n", "var a = () => {\n};\n")
	expectPrintedNormalAndMangle(t, "var a = () => undefined", "var a = () => void 0;\n", "var a = () => {\n};\n")
	expectPrintedNormalAndMangle(t, "var a = () => {return}", "var a = () => {\n  return;\n};\n", "var a = () => {\n};\n")
	expectPrintedNormalAndMangle(t, "var a = () => {return 123}", "var a = () => {\n  return 123;\n};\n", "var a = () => 123;\n")
	expectPrintedNormalAndMangle(t, "var a = () => {throw 123}", "var a = () => {\n  throw 123;\n};\n", "var a = () => {\n  throw 123;\n};\n")
}

func TestMangleIIFE(t *testing.T) {
	expectPrintedNormalAndMangle(t, "var a = (() => {})()", "var a = /* @__PURE__ */ (() => {\n})();\n", "var a = /* @__PURE__ */ (() => {\n})();\n")
	expectPrintedNormalAndMangle(t, "(() => {})()", "/* @__PURE__ */ (() => {\n})();\n", "")
	expectPrintedNormalAndMangle(t, "(() => a())()", "(() => a())();\n", "a();\n")
	expectPrintedNormalAndMangle(t, "(() => { a() })()", "(() => {\n  a();\n})();\n", "a();\n")
	expectPrintedNormalAndMangle(t, "(() => { return a() })()", "(() => {\n  return a();\n})();\n", "a();\n")
	expectPrintedNormalAndMangle(t, "(() => { let b = a; b() })()", "(() => {\n  let b = a;\n  b();\n})();\n", "a();\n")
	expectPrintedNormalAndMangle(t, "(() => { let b = a; return b() })()", "(() => {\n  let b = a;\n  return b();\n})();\n", "a();\n")
	expectPrintedNormalAndMangle(t, "(async () => {})()", "/* @__PURE__ */ (async () => {\n})();\n", "")
	expectPrintedNormalAndMangle(t, "(async () => { a() })()", "(async () => {\n  a();\n})();\n", "(async () => a())();\n")
	expectPrintedNormalAndMangle(t, "(async () => { let b = a; b() })()", "(async () => {\n  let b = a;\n  b();\n})();\n", "(async () => a())();\n")

	expectPrintedNormalAndMangle(t, "var a = (function() {})()", "var a = /* @__PURE__ */ function() {\n}();\n", "var a = /* @__PURE__ */ function() {\n}();\n")
	expectPrintedNormalAndMangle(t, "(function() {})()", "/* @__PURE__ */ (function() {\n})();\n", "")
	expectPrintedNormalAndMangle(t, "(function*() {})()", "/* @__PURE__ */ (function* () {\n})();\n", "")
	expectPrintedNormalAndMangle(t, "(async function() {})()", "/* @__PURE__ */ (async function() {\n})();\n", "")
	expectPrintedNormalAndMangle(t, "(function() { a() })()", "(function() {\n  a();\n})();\n", "(function() {\n  a();\n})();\n")
	expectPrintedNormalAndMangle(t, "(function*() { a() })()", "(function* () {\n  a();\n})();\n", "(function* () {\n  a();\n})();\n")
	expectPrintedNormalAndMangle(t, "(async function() { a() })()", "(async function() {\n  a();\n})();\n", "(async function() {\n  a();\n})();\n")

	expectPrintedNormalAndMangle(t, "(() => x)()", "(() => x)();\n", "x;\n")
	expectPrintedNormalAndMangle(t, "/* @__PURE__ */ (() => x)()", "/* @__PURE__ */ (() => x)();\n", "")
	expectPrintedNormalAndMangle(t, "/* @__PURE__ */ (() => x)(y, z)", "/* @__PURE__ */ (() => x)(y, z);\n", "y, z;\n")
}

func TestMangleTemplate(t *testing.T) {
	expectPrintedNormalAndMangle(t, "_ = `a${x}b${y}c`", "_ = `a${x}b${y}c`;\n", "_ = `a${x}b${y}c`;\n")
	expectPrintedNormalAndMangle(t, "_ = `a${x}b${'y'}c`", "_ = `a${x}b${\"y\"}c`;\n", "_ = `a${x}byc`;\n")
	expectPrintedNormalAndMangle(t, "_ = `a${'x'}b${y}c`", "_ = `a${\"x\"}b${y}c`;\n", "_ = `axb${y}c`;\n")
	expectPrintedNormalAndMangle(t, "_ = `a${'x'}b${'y'}c`", "_ = `a${\"x\"}b${\"y\"}c`;\n", "_ = `axbyc`;\n")

	expectPrintedNormalAndMangle(t, "tag`a${x}b${y}c`", "tag`a${x}b${y}c`;\n", "tag`a${x}b${y}c`;\n")
	expectPrintedNormalAndMangle(t, "tag`a${x}b${'y'}c`", "tag`a${x}b${\"y\"}c`;\n", "tag`a${x}b${\"y\"}c`;\n")
	expectPrintedNormalAndMangle(t, "tag`a${'x'}b${y}c`", "tag`a${\"x\"}b${y}c`;\n", "tag`a${\"x\"}b${y}c`;\n")
	expectPrintedNormalAndMangle(t, "tag`a${'x'}b${'y'}c`", "tag`a${\"x\"}b${\"y\"}c`;\n", "tag`a${\"x\"}b${\"y\"}c`;\n")

	expectPrintedNormalAndMangle(t, "(1, x)``", "(1, x)``;\n", "x``;\n")
	expectPrintedNormalAndMangle(t, "(1, x.y)``", "(1, x.y)``;\n", "(0, x.y)``;\n")
	expectPrintedNormalAndMangle(t, "(1, x[y])``", "(1, x[y])``;\n", "(0, x[y])``;\n")
	expectPrintedNormalAndMangle(t, "(true && x)``", "x``;\n", "x``;\n")
	expectPrintedNormalAndMangle(t, "(true && x.y)``", "(0, x.y)``;\n", "(0, x.y)``;\n")
	expectPrintedNormalAndMangle(t, "(true && x[y])``", "(0, x[y])``;\n", "(0, x[y])``;\n")
	expectPrintedNormalAndMangle(t, "(false || x)``", "x``;\n", "x``;\n")
	expectPrintedNormalAndMangle(t, "(false || x.y)``", "(0, x.y)``;\n", "(0, x.y)``;\n")
	expectPrintedNormalAndMangle(t, "(false || x[y])``", "(0, x[y])``;\n", "(0, x[y])``;\n")
	expectPrintedNormalAndMangle(t, "(null ?? x)``", "x``;\n", "x``;\n")
	expectPrintedNormalAndMangle(t, "(null ?? x.y)``", "(0, x.y)``;\n", "(0, x.y)``;\n")
	expectPrintedNormalAndMangle(t, "(null ?? x[y])``", "(0, x[y])``;\n", "(0, x[y])``;\n")

	expectPrintedMangleTarget(t, 2015, "class Foo { #foo() { return this.#foo`` } }", `var _foo, foo_fn;
class Foo {
  constructor() {
    __privateAdd(this, _foo);
  }
}
_foo = new WeakSet(), foo_fn = function() {
  return __privateMethod(this, _foo, foo_fn).bind(this)`+"``"+`;
};
`)

	expectPrintedMangleTarget(t, 2015, "class Foo { #foo() { return (0, this.#foo)`` } }", `var _foo, foo_fn;
class Foo {
  constructor() {
    __privateAdd(this, _foo);
  }
}
_foo = new WeakSet(), foo_fn = function() {
  return __privateMethod(this, _foo, foo_fn)`+"``"+`;
};
`)

	expectPrintedNormalAndMangle(t,
		"function f(a) { let c = a.b; return c`` }",
		"function f(a) {\n  let c = a.b;\n  return c``;\n}\n",
		"function f(a) {\n  return (0, a.b)``;\n}\n")
	expectPrintedNormalAndMangle(t,
		"function f(a) { let c = a.b; return c`${x}` }",
		"function f(a) {\n  let c = a.b;\n  return c`${x}`;\n}\n",
		"function f(a) {\n  return (0, a.b)`${x}`;\n}\n")
}

func TestMangleTypeofIdentifier(t *testing.T) {
	expectPrintedNormalAndMangle(t, "return typeof (123, x)", "return typeof (123, x);\n", "return typeof (0, x);\n")
	expectPrintedNormalAndMangle(t, "return typeof (123, x.y)", "return typeof (123, x.y);\n", "return typeof x.y;\n")
	expectPrintedNormalAndMangle(t, "return typeof (123, x); var x", "return typeof (123, x);\nvar x;\n", "return typeof x;\nvar x;\n")

	expectPrintedNormalAndMangle(t, "return typeof (true && x)", "return typeof (0, x);\n", "return typeof (0, x);\n")
	expectPrintedNormalAndMangle(t, "return typeof (true && x.y)", "return typeof x.y;\n", "return typeof x.y;\n")
	expectPrintedNormalAndMangle(t, "return typeof (true && x); var x", "return typeof x;\nvar x;\n", "return typeof x;\nvar x;\n")

	expectPrintedNormalAndMangle(t, "return typeof (false || x)", "return typeof (0, x);\n", "return typeof (0, x);\n")
	expectPrintedNormalAndMangle(t, "return typeof (false || x.y)", "return typeof x.y;\n", "return typeof x.y;\n")
	expectPrintedNormalAndMangle(t, "return typeof (false || x); var x", "return typeof x;\nvar x;\n", "return typeof x;\nvar x;\n")
}

func TestMangleTypeofEqualsUndefined(t *testing.T) {
	expectPrintedNormalAndMangle(t, "return typeof x !== 'undefined'", "return typeof x !== \"undefined\";\n", "return typeof x < \"u\";\n")
	expectPrintedNormalAndMangle(t, "return typeof x != 'undefined'", "return typeof x != \"undefined\";\n", "return typeof x < \"u\";\n")
	expectPrintedNormalAndMangle(t, "return 'undefined' !== typeof x", "return \"undefined\" !== typeof x;\n", "return typeof x < \"u\";\n")
	expectPrintedNormalAndMangle(t, "return 'undefined' != typeof x", "return \"undefined\" != typeof x;\n", "return typeof x < \"u\";\n")

	expectPrintedNormalAndMangle(t, "return typeof x === 'undefined'", "return typeof x === \"undefined\";\n", "return typeof x > \"u\";\n")
	expectPrintedNormalAndMangle(t, "return typeof x == 'undefined'", "return typeof x == \"undefined\";\n", "return typeof x > \"u\";\n")
	expectPrintedNormalAndMangle(t, "return 'undefined' === typeof x", "return \"undefined\" === typeof x;\n", "return typeof x > \"u\";\n")
	expectPrintedNormalAndMangle(t, "return 'undefined' == typeof x", "return \"undefined\" == typeof x;\n", "return typeof x > \"u\";\n")
}

func TestMangleEquals(t *testing.T) {
	expectPrintedNormalAndMangle(t, "return typeof x === y", "return typeof x === y;\n", "return typeof x === y;\n")
	expectPrintedNormalAndMangle(t, "return typeof x !== y", "return typeof x !== y;\n", "return typeof x !== y;\n")
	expectPrintedNormalAndMangle(t, "return y === typeof x", "return y === typeof x;\n", "return y === typeof x;\n")
	expectPrintedNormalAndMangle(t, "return y !== typeof x", "return y !== typeof x;\n", "return y !== typeof x;\n")

	expectPrintedNormalAndMangle(t, "return typeof x === 'string'", "return typeof x === \"string\";\n", "return typeof x == \"string\";\n")
	expectPrintedNormalAndMangle(t, "return typeof x !== 'string'", "return typeof x !== \"string\";\n", "return typeof x != \"string\";\n")
	expectPrintedNormalAndMangle(t, "return 'string' === typeof x", "return \"string\" === typeof x;\n", "return typeof x == \"string\";\n")
	expectPrintedNormalAndMangle(t, "return 'string' !== typeof x", "return \"string\" !== typeof x;\n", "return typeof x != \"string\";\n")

	expectPrintedNormalAndMangle(t, "return a === 0", "return a === 0;\n", "return a === 0;\n")
	expectPrintedNormalAndMangle(t, "return a !== 0", "return a !== 0;\n", "return a !== 0;\n")
	expectPrintedNormalAndMangle(t, "return +a === 0", "return +a === 0;\n", "return +a == 0;\n") // No BigInt hazard
	expectPrintedNormalAndMangle(t, "return +a !== 0", "return +a !== 0;\n", "return +a != 0;\n")
	expectPrintedNormalAndMangle(t, "return -a === 0", "return -a === 0;\n", "return -a === 0;\n") // BigInt hazard
	expectPrintedNormalAndMangle(t, "return -a !== 0", "return -a !== 0;\n", "return -a !== 0;\n")

	expectPrintedNormalAndMangle(t, "return a === ''", "return a === \"\";\n", "return a === \"\";\n")
	expectPrintedNormalAndMangle(t, "return a !== ''", "return a !== \"\";\n", "return a !== \"\";\n")
	expectPrintedNormalAndMangle(t, "return (a + '!') === 'a!'", "return a + \"!\" === \"a!\";\n", "return a + \"!\" == \"a!\";\n")
	expectPrintedNormalAndMangle(t, "return (a + '!') !== 'a!'", "return a + \"!\" !== \"a!\";\n", "return a + \"!\" != \"a!\";\n")
	expectPrintedNormalAndMangle(t, "return (a += '!') === 'a!'", "return (a += \"!\") === \"a!\";\n", "return (a += \"!\") == \"a!\";\n")
	expectPrintedNormalAndMangle(t, "return (a += '!') !== 'a!'", "return (a += \"!\") !== \"a!\";\n", "return (a += \"!\") != \"a!\";\n")

	expectPrintedNormalAndMangle(t, "return a === false", "return a === false;\n", "return a === false;\n")
	expectPrintedNormalAndMangle(t, "return a === true", "return a === true;\n", "return a === true;\n")
	expectPrintedNormalAndMangle(t, "return a !== false", "return a !== false;\n", "return a !== false;\n")
	expectPrintedNormalAndMangle(t, "return a !== true", "return a !== true;\n", "return a !== true;\n")
	expectPrintedNormalAndMangle(t, "return !a === false", "return !a === false;\n", "return !!a;\n")
	expectPrintedNormalAndMangle(t, "return !a === true", "return !a === true;\n", "return !a;\n")
	expectPrintedNormalAndMangle(t, "return !a !== false", "return !a !== false;\n", "return !a;\n")
	expectPrintedNormalAndMangle(t, "return !a !== true", "return !a !== true;\n", "return !!a;\n")
	expectPrintedNormalAndMangle(t, "return false === !a", "return false === !a;\n", "return !!a;\n")
	expectPrintedNormalAndMangle(t, "return true === !a", "return true === !a;\n", "return !a;\n")
	expectPrintedNormalAndMangle(t, "return false !== !a", "return false !== !a;\n", "return !a;\n")
	expectPrintedNormalAndMangle(t, "return true !== !a", "return true !== !a;\n", "return !!a;\n")

	expectPrintedNormalAndMangle(t, "return a === !b", "return a === !b;\n", "return a === !b;\n")
	expectPrintedNormalAndMangle(t, "return a === !b", "return a === !b;\n", "return a === !b;\n")
	expectPrintedNormalAndMangle(t, "return a !== !b", "return a !== !b;\n", "return a !== !b;\n")
	expectPrintedNormalAndMangle(t, "return a !== !b", "return a !== !b;\n", "return a !== !b;\n")
	expectPrintedNormalAndMangle(t, "return !a === !b", "return !a === !b;\n", "return !a == !b;\n")
	expectPrintedNormalAndMangle(t, "return !a === !b", "return !a === !b;\n", "return !a == !b;\n")
	expectPrintedNormalAndMangle(t, "return !a !== !b", "return !a !== !b;\n", "return !a != !b;\n")
	expectPrintedNormalAndMangle(t, "return !a !== !b", "return !a !== !b;\n", "return !a != !b;\n")

	// These have BigInt hazards and should not be changed
	expectPrintedNormalAndMangle(t, "return (a, -1n) !== -1", "return (a, -1n) !== -1;\n", "return a, -1n !== -1;\n")
	expectPrintedNormalAndMangle(t, "return (a, ~1n) !== -1", "return (a, ~1n) !== -1;\n", "return a, ~1n !== -1;\n")
	expectPrintedNormalAndMangle(t, "return (a -= 1n) !== -1", "return (a -= 1n) !== -1;\n", "return (a -= 1n) !== -1;\n")
	expectPrintedNormalAndMangle(t, "return (a *= 1n) !== -1", "return (a *= 1n) !== -1;\n", "return (a *= 1n) !== -1;\n")
	expectPrintedNormalAndMangle(t, "return (a **= 1n) !== -1", "return (a **= 1n) !== -1;\n", "return (a **= 1n) !== -1;\n")
	expectPrintedNormalAndMangle(t, "return (a /= 1n) !== -1", "return (a /= 1n) !== -1;\n", "return (a /= 1n) !== -1;\n")
	expectPrintedNormalAndMangle(t, "return (a %= 1n) !== -1", "return (a %= 1n) !== -1;\n", "return (a %= 1n) !== -1;\n")
	expectPrintedNormalAndMangle(t, "return (a &= 1n) !== -1", "return (a &= 1n) !== -1;\n", "return (a &= 1n) !== -1;\n")
	expectPrintedNormalAndMangle(t, "return (a |= 1n) !== -1", "return (a |= 1n) !== -1;\n", "return (a |= 1n) !== -1;\n")
	expectPrintedNormalAndMangle(t, "return (a ^= 1n) !== -1", "return (a ^= 1n) !== -1;\n", "return (a ^= 1n) !== -1;\n")
}

func TestMangleUnaryInsideComma(t *testing.T) {
	expectPrintedNormalAndMangle(t, "return -(a, b)", "return -(a, b);\n", "return a, -b;\n")
	expectPrintedNormalAndMangle(t, "return +(a, b)", "return +(a, b);\n", "return a, +b;\n")
	expectPrintedNormalAndMangle(t, "return ~(a, b)", "return ~(a, b);\n", "return a, ~b;\n")
	expectPrintedNormalAndMangle(t, "return !(a, b)", "return !(a, b);\n", "return a, !b;\n")
	expectPrintedNormalAndMangle(t, "return void (a, b)", "return void (a, b);\n", "return a, void b;\n")
	expectPrintedNormalAndMangle(t, "return typeof (a, b)", "return typeof (a, b);\n", "return typeof (a, b);\n")
	expectPrintedNormalAndMangle(t, "return delete (a, b)", "return delete (a, b);\n", "return delete (a, b);\n")
}

func TestMangleBinaryInsideComma(t *testing.T) {
	expectPrintedNormalAndMangle(t, "(a, b) && c", "(a, b) && c;\n", "a, b && c;\n")
	expectPrintedNormalAndMangle(t, "(a, b) == c", "(a, b) == c;\n", "a, b == c;\n")
	expectPrintedNormalAndMangle(t, "(a, b) + c", "(a, b) + c;\n", "a, b + c;\n")
	expectPrintedNormalAndMangle(t, "a && (b, c)", "a && (b, c);\n", "a && (b, c);\n")
	expectPrintedNormalAndMangle(t, "a == (b, c)", "a == (b, c);\n", "a == (b, c);\n")
	expectPrintedNormalAndMangle(t, "a + (b, c)", "a + (b, c);\n", "a + (b, c);\n")
}

func TestMangleUnaryConstantFolding(t *testing.T) {
	expectPrintedNormalAndMangle(t, "x = +5", "x = 5;\n", "x = 5;\n")
	expectPrintedNormalAndMangle(t, "x = -5", "x = -5;\n", "x = -5;\n")
	expectPrintedNormalAndMangle(t, "x = ~5", "x = ~5;\n", "x = -6;\n")
	expectPrintedNormalAndMangle(t, "x = !5", "x = false;\n", "x = false;\n")
	expectPrintedNormalAndMangle(t, "x = typeof 5", "x = \"number\";\n", "x = \"number\";\n")
}

func TestMangleBinaryConstantFolding(t *testing.T) {
	expectPrintedNormalAndMangle(t, "x = 3 + 6", "x = 3 + 6;\n", "x = 3 + 6;\n")
	expectPrintedNormalAndMangle(t, "x = 3 - 6", "x = 3 - 6;\n", "x = 3 - 6;\n")
	expectPrintedNormalAndMangle(t, "x = 3 * 6", "x = 3 * 6;\n", "x = 3 * 6;\n")
	expectPrintedNormalAndMangle(t, "x = 3 / 6", "x = 3 / 6;\n", "x = 3 / 6;\n")
	expectPrintedNormalAndMangle(t, "x = 3 % 6", "x = 3 % 6;\n", "x = 3 % 6;\n")
	expectPrintedNormalAndMangle(t, "x = 3 ** 6", "x = 3 ** 6;\n", "x = 3 ** 6;\n")

	expectPrintedNormalAndMangle(t, "x = 3 < 6", "x = 3 < 6;\n", "x = 3 < 6;\n")
	expectPrintedNormalAndMangle(t, "x = 3 > 6", "x = 3 > 6;\n", "x = 3 > 6;\n")
	expectPrintedNormalAndMangle(t, "x = 3 <= 6", "x = 3 <= 6;\n", "x = 3 <= 6;\n")
	expectPrintedNormalAndMangle(t, "x = 3 >= 6", "x = 3 >= 6;\n", "x = 3 >= 6;\n")
	expectPrintedNormalAndMangle(t, "x = 3 == 6", "x = false;\n", "x = false;\n")
	expectPrintedNormalAndMangle(t, "x = 3 != 6", "x = true;\n", "x = true;\n")
	expectPrintedNormalAndMangle(t, "x = 3 === 6", "x = false;\n", "x = false;\n")
	expectPrintedNormalAndMangle(t, "x = 3 !== 6", "x = true;\n", "x = true;\n")

	expectPrintedNormalAndMangle(t, "x = 3 in 6", "x = 3 in 6;\n", "x = 3 in 6;\n")
	expectPrintedNormalAndMangle(t, "x = 3 instanceof 6", "x = 3 instanceof 6;\n", "x = 3 instanceof 6;\n")
	expectPrintedNormalAndMangle(t, "x = (3, 6)", "x = (3, 6);\n", "x = 6;\n")

	expectPrintedNormalAndMangle(t, "x = 10 << 0", "x = 10 << 0;\n", "x = 10;\n")
	expectPrintedNormalAndMangle(t, "x = 10 << 1", "x = 10 << 1;\n", "x = 20;\n")
	expectPrintedNormalAndMangle(t, "x = 10 << 16", "x = 10 << 16;\n", "x = 655360;\n")
	expectPrintedNormalAndMangle(t, "x = 10 << 17", "x = 10 << 17;\n", "x = 10 << 17;\n")
	expectPrintedNormalAndMangle(t, "x = 10 >> 0", "x = 10 >> 0;\n", "x = 10;\n")
	expectPrintedNormalAndMangle(t, "x = 10 >> 1", "x = 10 >> 1;\n", "x = 5;\n")
	expectPrintedNormalAndMangle(t, "x = 10 >>> 0", "x = 10 >>> 0;\n", "x = 10;\n")
	expectPrintedNormalAndMangle(t, "x = 10 >>> 1", "x = 10 >>> 1;\n", "x = 5;\n")
	expectPrintedNormalAndMangle(t, "x = -10 >>> 1", "x = -10 >>> 1;\n", "x = -10 >>> 1;\n")
	expectPrintedNormalAndMangle(t, "x = -1 >>> 0", "x = -1 >>> 0;\n", "x = -1 >>> 0;\n")
	expectPrintedNormalAndMangle(t, "x = -123 >>> 5", "x = -123 >>> 5;\n", "x = -123 >>> 5;\n")
	expectPrintedNormalAndMangle(t, "x = -123 >>> 6", "x = -123 >>> 6;\n", "x = 67108862;\n")
	expectPrintedNormalAndMangle(t, "x = 3 & 6", "x = 3 & 6;\n", "x = 2;\n")
	expectPrintedNormalAndMangle(t, "x = 3 | 6", "x = 3 | 6;\n", "x = 7;\n")
	expectPrintedNormalAndMangle(t, "x = 3 ^ 6", "x = 3 ^ 6;\n", "x = 5;\n")

	expectPrintedNormalAndMangle(t, "x = 3 && 6", "x = 6;\n", "x = 6;\n")
	expectPrintedNormalAndMangle(t, "x = 3 || 6", "x = 3;\n", "x = 3;\n")
	expectPrintedNormalAndMangle(t, "x = 3 ?? 6", "x = 3;\n", "x = 3;\n")
}

func TestMangleNestedLogical(t *testing.T) {
	expectPrintedNormalAndMangle(t, "(a && b) && c", "a && b && c;\n", "a && b && c;\n")
	expectPrintedNormalAndMangle(t, "a && (b && c)", "a && (b && c);\n", "a && b && c;\n")
	expectPrintedNormalAndMangle(t, "(a || b) && c", "(a || b) && c;\n", "(a || b) && c;\n")
	expectPrintedNormalAndMangle(t, "a && (b || c)", "a && (b || c);\n", "a && (b || c);\n")

	expectPrintedNormalAndMangle(t, "(a || b) || c", "a || b || c;\n", "a || b || c;\n")
	expectPrintedNormalAndMangle(t, "a || (b || c)", "a || (b || c);\n", "a || b || c;\n")
	expectPrintedNormalAndMangle(t, "(a && b) || c", "a && b || c;\n", "a && b || c;\n")
	expectPrintedNormalAndMangle(t, "a || (b && c)", "a || b && c;\n", "a || b && c;\n")
}

func TestMangleEqualsUndefined(t *testing.T) {
	expectPrintedNormalAndMangle(t, "return a === void 0", "return a === void 0;\n", "return a === void 0;\n")
	expectPrintedNormalAndMangle(t, "return a !== void 0", "return a !== void 0;\n", "return a !== void 0;\n")
	expectPrintedNormalAndMangle(t, "return void 0 === a", "return void 0 === a;\n", "return a === void 0;\n")
	expectPrintedNormalAndMangle(t, "return void 0 !== a", "return void 0 !== a;\n", "return a !== void 0;\n")

	expectPrintedNormalAndMangle(t, "return a == void 0", "return a == void 0;\n", "return a == null;\n")
	expectPrintedNormalAndMangle(t, "return a != void 0", "return a != void 0;\n", "return a != null;\n")
	expectPrintedNormalAndMangle(t, "return void 0 == a", "return void 0 == a;\n", "return a == null;\n")
	expectPrintedNormalAndMangle(t, "return void 0 != a", "return void 0 != a;\n", "return a != null;\n")

	expectPrintedNormalAndMangle(t, "return a === null || a === undefined", "return a === null || a === void 0;\n", "return a == null;\n")
	expectPrintedNormalAndMangle(t, "return a === null || a !== undefined", "return a === null || a !== void 0;\n", "return a === null || a !== void 0;\n")
	expectPrintedNormalAndMangle(t, "return a !== null || a === undefined", "return a !== null || a === void 0;\n", "return a !== null || a === void 0;\n")
	expectPrintedNormalAndMangle(t, "return a === null && a === undefined", "return a === null && a === void 0;\n", "return a === null && a === void 0;\n")
	expectPrintedNormalAndMangle(t, "return a.x === null || a.x === undefined", "return a.x === null || a.x === void 0;\n", "return a.x === null || a.x === void 0;\n")

	expectPrintedNormalAndMangle(t, "return a === undefined || a === null", "return a === void 0 || a === null;\n", "return a == null;\n")
	expectPrintedNormalAndMangle(t, "return a === undefined || a !== null", "return a === void 0 || a !== null;\n", "return a === void 0 || a !== null;\n")
	expectPrintedNormalAndMangle(t, "return a !== undefined || a === null", "return a !== void 0 || a === null;\n", "return a !== void 0 || a === null;\n")
	expectPrintedNormalAndMangle(t, "return a === undefined && a === null", "return a === void 0 && a === null;\n", "return a === void 0 && a === null;\n")
	expectPrintedNormalAndMangle(t, "return a.x === undefined || a.x === null", "return a.x === void 0 || a.x === null;\n", "return a.x === void 0 || a.x === null;\n")

	expectPrintedNormalAndMangle(t, "return a !== null && a !== undefined", "return a !== null && a !== void 0;\n", "return a != null;\n")
	expectPrintedNormalAndMangle(t, "return a !== null && a === undefined", "return a !== null && a === void 0;\n", "return a !== null && a === void 0;\n")
	expectPrintedNormalAndMangle(t, "return a === null && a !== undefined", "return a === null && a !== void 0;\n", "return a === null && a !== void 0;\n")
	expectPrintedNormalAndMangle(t, "return a !== null || a !== undefined", "return a !== null || a !== void 0;\n", "return a !== null || a !== void 0;\n")
	expectPrintedNormalAndMangle(t, "return a.x !== null && a.x !== undefined", "return a.x !== null && a.x !== void 0;\n", "return a.x !== null && a.x !== void 0;\n")

	expectPrintedNormalAndMangle(t, "return a !== undefined && a !== null", "return a !== void 0 && a !== null;\n", "return a != null;\n")
	expectPrintedNormalAndMangle(t, "return a !== undefined && a === null", "return a !== void 0 && a === null;\n", "return a !== void 0 && a === null;\n")
	expectPrintedNormalAndMangle(t, "return a === undefined && a !== null", "return a === void 0 && a !== null;\n", "return a === void 0 && a !== null;\n")
	expectPrintedNormalAndMangle(t, "return a !== undefined || a !== null", "return a !== void 0 || a !== null;\n", "return a !== void 0 || a !== null;\n")
	expectPrintedNormalAndMangle(t, "return a.x !== undefined && a.x !== null", "return a.x !== void 0 && a.x !== null;\n", "return a.x !== void 0 && a.x !== null;\n")
}

func TestMangleUnusedFunctionExpressionNames(t *testing.T) {
	expectPrintedNormalAndMangle(t, "x = function y() {}", "x = function y() {\n};\n", "x = function() {\n};\n")
	expectPrintedNormalAndMangle(t, "x = function y() { return y }", "x = function y() {\n  return y;\n};\n", "x = function y() {\n  return y;\n};\n")
	expectPrintedNormalAndMangle(t, "x = function y() { return eval('y') }", "x = function y() {\n  return eval(\"y\");\n};\n", "x = function y() {\n  return eval(\"y\");\n};\n")
	expectPrintedNormalAndMangle(t, "x = function y() { if (0) return y }", "x = function y() {\n  if (0)\n    return y;\n};\n", "x = function() {\n};\n")
}

func TestMangleClass(t *testing.T) {
	expectPrintedNormalAndMangle(t, "class x {['y'] = z}", "class x {\n  [\"y\"] = z;\n}\n", "class x {\n  y = z;\n}\n")
	expectPrintedNormalAndMangle(t, "class x {['y']() {}}", "class x {\n  [\"y\"]() {\n  }\n}\n", "class x {\n  y() {\n  }\n}\n")
	expectPrintedNormalAndMangle(t, "class x {get ['y']() {}}", "class x {\n  get [\"y\"]() {\n  }\n}\n", "class x {\n  get y() {\n  }\n}\n")
	expectPrintedNormalAndMangle(t, "class x {set ['y'](z) {}}", "class x {\n  set [\"y\"](z) {\n  }\n}\n", "class x {\n  set y(z) {\n  }\n}\n")
	expectPrintedNormalAndMangle(t, "class x {async ['y']() {}}", "class x {\n  async [\"y\"]() {\n  }\n}\n", "class x {\n  async y() {\n  }\n}\n")

	expectPrintedNormalAndMangle(t, "x = class {['y'] = z}", "x = class {\n  [\"y\"] = z;\n};\n", "x = class {\n  y = z;\n};\n")
	expectPrintedNormalAndMangle(t, "x = class {['y']() {}}", "x = class {\n  [\"y\"]() {\n  }\n};\n", "x = class {\n  y() {\n  }\n};\n")
	expectPrintedNormalAndMangle(t, "x = class {get ['y']() {}}", "x = class {\n  get [\"y\"]() {\n  }\n};\n", "x = class {\n  get y() {\n  }\n};\n")
	expectPrintedNormalAndMangle(t, "x = class {set ['y'](z) {}}", "x = class {\n  set [\"y\"](z) {\n  }\n};\n", "x = class {\n  set y(z) {\n  }\n};\n")
	expectPrintedNormalAndMangle(t, "x = class {async ['y']() {}}", "x = class {\n  async [\"y\"]() {\n  }\n};\n", "x = class {\n  async y() {\n  }\n};\n")
}

func TestMangleUnusedClassExpressionNames(t *testing.T) {
	expectPrintedNormalAndMangle(t, "x = class y {}", "x = class y {\n};\n", "x = class {\n};\n")
	expectPrintedNormalAndMangle(t, "x = class y { foo() { return y } }", "x = class y {\n  foo() {\n    return y;\n  }\n};\n", "x = class y {\n  foo() {\n    return y;\n  }\n};\n")
	expectPrintedNormalAndMangle(t, "x = class y { foo() { if (0) return y } }", "x = class y {\n  foo() {\n    if (0)\n      return _y;\n  }\n};\n", "x = class {\n  foo() {\n  }\n};\n")
}

func TestMangleUnused(t *testing.T) {
	expectPrintedNormalAndMangle(t, "null", "null;\n", "")
	expectPrintedNormalAndMangle(t, "void 0", "", "")
	expectPrintedNormalAndMangle(t, "void 0", "", "")
	expectPrintedNormalAndMangle(t, "false", "false;\n", "")
	expectPrintedNormalAndMangle(t, "true", "true;\n", "")
	expectPrintedNormalAndMangle(t, "123", "123;\n", "")
	expectPrintedNormalAndMangle(t, "123n", "123n;\n", "")
	expectPrintedNormalAndMangle(t, "'abc'", "\"abc\";\n", "\"abc\";\n") // Technically a directive, not a string expression
	expectPrintedNormalAndMangle(t, "0; 'abc'", "0;\n\"abc\";\n", "")    // Actually a string expression
	expectPrintedNormalAndMangle(t, "'abc'; 'use strict'", "\"abc\";\n\"use strict\";\n", "\"abc\";\n\"use strict\";\n")
	expectPrintedNormalAndMangle(t, "function f() { 'abc'; 'use strict' }", "function f() {\n  \"abc\";\n  \"use strict\";\n}\n", "function f() {\n  \"abc\";\n  \"use strict\";\n}\n")
	expectPrintedNormalAndMangle(t, "this", "this;\n", "")
	expectPrintedNormalAndMangle(t, "/regex/", "/regex/;\n", "")
	expectPrintedNormalAndMangle(t, "(function() {})", "(function() {\n});\n", "")
	expectPrintedNormalAndMangle(t, "(() => {})", "() => {\n};\n", "")
	expectPrintedNormalAndMangle(t, "import.meta", "import.meta;\n", "")

	// Unary operators
	expectPrintedNormalAndMangle(t, "+x", "+x;\n", "+x;\n")
	expectPrintedNormalAndMangle(t, "-x", "-x;\n", "-x;\n")
	expectPrintedNormalAndMangle(t, "!x", "!x;\n", "x;\n")
	expectPrintedNormalAndMangle(t, "~x", "~x;\n", "~x;\n")
	expectPrintedNormalAndMangle(t, "++x", "++x;\n", "++x;\n")
	expectPrintedNormalAndMangle(t, "--x", "--x;\n", "--x;\n")
	expectPrintedNormalAndMangle(t, "x++", "x++;\n", "x++;\n")
	expectPrintedNormalAndMangle(t, "x--", "x--;\n", "x--;\n")
	expectPrintedNormalAndMangle(t, "void x", "void x;\n", "x;\n")
	expectPrintedNormalAndMangle(t, "delete x", "delete x;\n", "delete x;\n")
	expectPrintedNormalAndMangle(t, "typeof x", "typeof x;\n", "")
	expectPrintedNormalAndMangle(t, "typeof x()", "typeof x();\n", "x();\n")
	expectPrintedNormalAndMangle(t, "typeof (0, x)", "typeof (0, x);\n", "x;\n")
	expectPrintedNormalAndMangle(t, "typeof (0 || x)", "typeof (0, x);\n", "x;\n")
	expectPrintedNormalAndMangle(t, "typeof (1 && x)", "typeof (0, x);\n", "x;\n")
	expectPrintedNormalAndMangle(t, "typeof (1 ? x : 0)", "typeof (1 ? x : 0);\n", "x;\n")
	expectPrintedNormalAndMangle(t, "typeof (0 ? 1 : x)", "typeof (0 ? 1 : x);\n", "x;\n")

	// Binary operators
	expectPrintedNormalAndMangle(t, "a + b", "a + b;\n", "a + b;\n")
	expectPrintedNormalAndMangle(t, "a - b", "a - b;\n", "a - b;\n")
	expectPrintedNormalAndMangle(t, "a * b", "a * b;\n", "a * b;\n")
	expectPrintedNormalAndMangle(t, "a / b", "a / b;\n", "a / b;\n")
	expectPrintedNormalAndMangle(t, "a % b", "a % b;\n", "a % b;\n")
	expectPrintedNormalAndMangle(t, "a ** b", "a ** b;\n", "a ** b;\n")
	expectPrintedNormalAndMangle(t, "a & b", "a & b;\n", "a & b;\n")
	expectPrintedNormalAndMangle(t, "a | b", "a | b;\n", "a | b;\n")
	expectPrintedNormalAndMangle(t, "a ^ b", "a ^ b;\n", "a ^ b;\n")
	expectPrintedNormalAndMangle(t, "a << b", "a << b;\n", "a << b;\n")
	expectPrintedNormalAndMangle(t, "a >> b", "a >> b;\n", "a >> b;\n")
	expectPrintedNormalAndMangle(t, "a >>> b", "a >>> b;\n", "a >>> b;\n")
	expectPrintedNormalAndMangle(t, "a === b", "a === b;\n", "a, b;\n")
	expectPrintedNormalAndMangle(t, "a !== b", "a !== b;\n", "a, b;\n")
	expectPrintedNormalAndMangle(t, "a == b", "a == b;\n", "a == b;\n")
	expectPrintedNormalAndMangle(t, "a != b", "a != b;\n", "a != b;\n")
	expectPrintedNormalAndMangle(t, "a, b", "a, b;\n", "a, b;\n")

	expectPrintedNormalAndMangle(t, "a + '' == b", "a + \"\" == b;\n", "a + \"\" == b;\n")
	expectPrintedNormalAndMangle(t, "a + '' != b", "a + \"\" != b;\n", "a + \"\" != b;\n")
	expectPrintedNormalAndMangle(t, "a + '' == b + ''", "a + \"\" == b + \"\";\n", "a + \"\", b + \"\";\n")
	expectPrintedNormalAndMangle(t, "a + '' != b + ''", "a + \"\" != b + \"\";\n", "a + \"\", b + \"\";\n")
	expectPrintedNormalAndMangle(t, "a + '' == (b | c)", "a + \"\" == (b | c);\n", "a + \"\", b | c;\n")
	expectPrintedNormalAndMangle(t, "a + '' != (b | c)", "a + \"\" != (b | c);\n", "a + \"\", b | c;\n")
	expectPrintedNormalAndMangle(t, "typeof a == b + ''", "typeof a == b + \"\";\n", "b + \"\";\n")
	expectPrintedNormalAndMangle(t, "typeof a != b + ''", "typeof a != b + \"\";\n", "b + \"\";\n")
	expectPrintedNormalAndMangle(t, "typeof a == 'b'", "typeof a == \"b\";\n", "")
	expectPrintedNormalAndMangle(t, "typeof a != 'b'", "typeof a != \"b\";\n", "")

	// Known globals can be removed
	expectPrintedNormalAndMangle(t, "Object", "Object;\n", "")
	expectPrintedNormalAndMangle(t, "Object()", "Object();\n", "Object();\n")
	expectPrintedNormalAndMangle(t, "NonObject", "NonObject;\n", "NonObject;\n")

	expectPrintedNormalAndMangle(t, "var bound; unbound", "var bound;\nunbound;\n", "var bound;\nunbound;\n")
	expectPrintedNormalAndMangle(t, "var bound; bound", "var bound;\nbound;\n", "var bound;\n")
	expectPrintedNormalAndMangle(t, "foo, 123, bar", "foo, 123, bar;\n", "foo, bar;\n")

	expectPrintedNormalAndMangle(t, "[[foo,, 123,, bar]]", "[[foo, , 123, , bar]];\n", "foo, bar;\n")
	expectPrintedNormalAndMangle(t, "var bound; [123, unbound, ...unbound, 234]", "var bound;\n[123, unbound, ...unbound, 234];\n", "var bound;\n[unbound, ...unbound];\n")
	expectPrintedNormalAndMangle(t, "var bound; [123, bound, ...bound, 234]", "var bound;\n[123, bound, ...bound, 234];\n", "var bound;\n[...bound];\n")

	expectPrintedNormalAndMangle(t,
		"({foo, x: 123, [y]: 123, z: z, bar})",
		"({ foo, x: 123, [y]: 123, z, bar });\n",
		"foo, y + \"\", z, bar;\n")
	expectPrintedNormalAndMangle(t,
		"var bound; ({x: 123, unbound, ...unbound, [unbound]: null, y: 234})",
		"var bound;\n({ x: 123, unbound, ...unbound, [unbound]: null, y: 234 });\n",
		"var bound;\n({ unbound, ...unbound, [unbound]: 0 });\n")
	expectPrintedNormalAndMangle(t,
		"var bound; ({x: 123, bound, ...bound, [bound]: null, y: 234})",
		"var bound;\n({ x: 123, bound, ...bound, [bound]: null, y: 234 });\n",
		"var bound;\n({ ...bound, [bound]: 0 });\n")
	expectPrintedNormalAndMangle(t,
		"var bound; ({x: 123, bound, ...bound, [bound]: foo(), y: 234})",
		"var bound;\n({ x: 123, bound, ...bound, [bound]: foo(), y: 234 });\n",
		"var bound;\n({ ...bound, [bound]: foo() });\n")

	expectPrintedNormalAndMangle(t, "console.log(1, foo(), bar())", "console.log(1, foo(), bar());\n", "console.log(1, foo(), bar());\n")
	expectPrintedNormalAndMangle(t, "/* @__PURE__ */ console.log(1, foo(), bar())", "/* @__PURE__ */ console.log(1, foo(), bar());\n", "foo(), bar();\n")

	expectPrintedNormalAndMangle(t, "new TestCase(1, foo(), bar())", "new TestCase(1, foo(), bar());\n", "new TestCase(1, foo(), bar());\n")
	expectPrintedNormalAndMangle(t, "/* @__PURE__ */ new TestCase(1, foo(), bar())", "/* @__PURE__ */ new TestCase(1, foo(), bar());\n", "foo(), bar();\n")

	expectPrintedNormalAndMangle(t, "let x = (1, 2)", "let x = (1, 2);\n", "let x = 2;\n")
	expectPrintedNormalAndMangle(t, "let x = (y, 2)", "let x = (y, 2);\n", "let x = (y, 2);\n")
	expectPrintedNormalAndMangle(t, "let x = (/* @__PURE__ */ foo(bar), 2)", "let x = (/* @__PURE__ */ foo(bar), 2);\n", "let x = (bar, 2);\n")

	expectPrintedNormalAndMangle(t, "let x = (2, y)", "let x = (2, y);\n", "let x = y;\n")
	expectPrintedNormalAndMangle(t, "let x = (2, y)()", "let x = (2, y)();\n", "let x = y();\n")
	expectPrintedNormalAndMangle(t, "let x = (true && y)()", "let x = y();\n", "let x = y();\n")
	expectPrintedNormalAndMangle(t, "let x = (false || y)()", "let x = y();\n", "let x = y();\n")
	expectPrintedNormalAndMangle(t, "let x = (null ?? y)()", "let x = y();\n", "let x = y();\n")
	expectPrintedNormalAndMangle(t, "let x = (1 ? y : 2)()", "let x = (1 ? y : 2)();\n", "let x = y();\n")
	expectPrintedNormalAndMangle(t, "let x = (0 ? 1 : y)()", "let x = (0 ? 1 : y)();\n", "let x = y();\n")

	// Make sure call targets with "this" values are preserved
	expectPrintedNormalAndMangle(t, "let x = (2, y.z)", "let x = (2, y.z);\n", "let x = y.z;\n")
	expectPrintedNormalAndMangle(t, "let x = (2, y.z)()", "let x = (2, y.z)();\n", "let x = (0, y.z)();\n")
	expectPrintedNormalAndMangle(t, "let x = (true && y.z)()", "let x = (0, y.z)();\n", "let x = (0, y.z)();\n")
	expectPrintedNormalAndMangle(t, "let x = (false || y.z)()", "let x = (0, y.z)();\n", "let x = (0, y.z)();\n")
	expectPrintedNormalAndMangle(t, "let x = (null ?? y.z)()", "let x = (0, y.z)();\n", "let x = (0, y.z)();\n")
	expectPrintedNormalAndMangle(t, "let x = (1 ? y.z : 2)()", "let x = (1 ? y.z : 2)();\n", "let x = (0, y.z)();\n")
	expectPrintedNormalAndMangle(t, "let x = (0 ? 1 : y.z)()", "let x = (0 ? 1 : y.z)();\n", "let x = (0, y.z)();\n")

	expectPrintedNormalAndMangle(t, "let x = (2, y[z])", "let x = (2, y[z]);\n", "let x = y[z];\n")
	expectPrintedNormalAndMangle(t, "let x = (2, y[z])()", "let x = (2, y[z])();\n", "let x = (0, y[z])();\n")
	expectPrintedNormalAndMangle(t, "let x = (true && y[z])()", "let x = (0, y[z])();\n", "let x = (0, y[z])();\n")
	expectPrintedNormalAndMangle(t, "let x = (false || y[z])()", "let x = (0, y[z])();\n", "let x = (0, y[z])();\n")
	expectPrintedNormalAndMangle(t, "let x = (null ?? y[z])()", "let x = (0, y[z])();\n", "let x = (0, y[z])();\n")
	expectPrintedNormalAndMangle(t, "let x = (1 ? y[z] : 2)()", "let x = (1 ? y[z] : 2)();\n", "let x = (0, y[z])();\n")
	expectPrintedNormalAndMangle(t, "let x = (0 ? 1 : y[z])()", "let x = (0 ? 1 : y[z])();\n", "let x = (0, y[z])();\n")

	// Make sure the return value of "delete" is preserved
	expectPrintedNormalAndMangle(t, "delete (x)", "delete x;\n", "delete x;\n")
	expectPrintedNormalAndMangle(t, "delete (x); var x", "delete x;\nvar x;\n", "delete x;\nvar x;\n")
	expectPrintedNormalAndMangle(t, "delete (x.y)", "delete x.y;\n", "delete x.y;\n")
	expectPrintedNormalAndMangle(t, "delete (x[y])", "delete x[y];\n", "delete x[y];\n")
	expectPrintedNormalAndMangle(t, "delete (x?.y)", "delete x?.y;\n", "delete x?.y;\n")
	expectPrintedNormalAndMangle(t, "delete (x?.[y])", "delete x?.[y];\n", "delete x?.[y];\n")
	expectPrintedNormalAndMangle(t, "delete (2, x)", "delete (2, x);\n", "delete (0, x);\n")
	expectPrintedNormalAndMangle(t, "delete (2, x); var x", "delete (2, x);\nvar x;\n", "delete (0, x);\nvar x;\n")
	expectPrintedNormalAndMangle(t, "delete (2, x.y)", "delete (2, x.y);\n", "delete (0, x.y);\n")
	expectPrintedNormalAndMangle(t, "delete (2, x[y])", "delete (2, x[y]);\n", "delete (0, x[y]);\n")
	expectPrintedNormalAndMangle(t, "delete (2, x?.y)", "delete (2, x?.y);\n", "delete (0, x?.y);\n")
	expectPrintedNormalAndMangle(t, "delete (2, x?.[y])", "delete (2, x?.[y]);\n", "delete (0, x?.[y]);\n")
	expectPrintedNormalAndMangle(t, "delete (true && x)", "delete (0, x);\n", "delete (0, x);\n")
	expectPrintedNormalAndMangle(t, "delete (false || x)", "delete (0, x);\n", "delete (0, x);\n")
	expectPrintedNormalAndMangle(t, "delete (null ?? x)", "delete (0, x);\n", "delete (0, x);\n")
	expectPrintedNormalAndMangle(t, "delete (1 ? x : 2)", "delete (1 ? x : 2);\n", "delete (0, x);\n")
	expectPrintedNormalAndMangle(t, "delete (0 ? 1 : x)", "delete (0 ? 1 : x);\n", "delete (0, x);\n")
	expectPrintedNormalAndMangle(t, "delete (NaN)", "delete NaN;\n", "delete NaN;\n")
	expectPrintedNormalAndMangle(t, "delete (Infinity)", "delete Infinity;\n", "delete Infinity;\n")
	expectPrintedNormalAndMangle(t, "delete (-Infinity)", "delete -Infinity;\n", "delete -Infinity;\n")
	expectPrintedNormalAndMangle(t, "delete (1, NaN)", "delete (1, NaN);\n", "delete (0, NaN);\n")
	expectPrintedNormalAndMangle(t, "delete (1, Infinity)", "delete (1, Infinity);\n", "delete (0, Infinity);\n")
	expectPrintedNormalAndMangle(t, "delete (1, -Infinity)", "delete (1, -Infinity);\n", "delete -Infinity;\n")

	expectPrintedNormalAndMangle(t, "foo ? 1 : 2", "foo ? 1 : 2;\n", "foo;\n")
	expectPrintedNormalAndMangle(t, "foo ? 1 : bar", "foo ? 1 : bar;\n", "foo || bar;\n")
	expectPrintedNormalAndMangle(t, "foo ? bar : 2", "foo ? bar : 2;\n", "foo && bar;\n")
	expectPrintedNormalAndMangle(t, "foo ? bar : baz", "foo ? bar : baz;\n", "foo ? bar : baz;\n")

	for _, op := range []string{"&&", "||", "??"} {
		expectPrintedNormalAndMangle(t, "foo "+op+" bar", "foo "+op+" bar;\n", "foo "+op+" bar;\n")
		expectPrintedNormalAndMangle(t, "var foo; foo "+op+" bar", "var foo;\nfoo "+op+" bar;\n", "var foo;\nfoo "+op+" bar;\n")
		expectPrintedNormalAndMangle(t, "var bar; foo "+op+" bar", "var bar;\nfoo "+op+" bar;\n", "var bar;\nfoo;\n")
		expectPrintedNormalAndMangle(t, "var foo, bar; foo "+op+" bar", "var foo, bar;\nfoo "+op+" bar;\n", "var foo, bar;\n")
	}

	expectPrintedNormalAndMangle(t, "tag`a${b}c${d}e`", "tag`a${b}c${d}e`;\n", "tag`a${b}c${d}e`;\n")
	expectPrintedNormalAndMangle(t, "`a${b}c${d}e`", "`a${b}c${d}e`;\n", "`${b}${d}`;\n")

	// These can't be reduced to string addition due to "valueOf". See:
	// https://github.com/terser/terser/issues/1128#issuecomment-994209801
	expectPrintedNormalAndMangle(t, "`stuff ${x} ${1}`", "`stuff ${x} ${1}`;\n", "`${x}`;\n")
	expectPrintedNormalAndMangle(t, "`stuff ${1} ${y}`", "`stuff ${1} ${y}`;\n", "`${y}`;\n")
	expectPrintedNormalAndMangle(t, "`stuff ${x} ${y}`", "`stuff ${x} ${y}`;\n", "`${x}${y}`;\n")
	expectPrintedNormalAndMangle(t, "`stuff ${x ? 1 : 2} ${y}`", "`stuff ${x ? 1 : 2} ${y}`;\n", "x, `${y}`;\n")
	expectPrintedNormalAndMangle(t, "`stuff ${x} ${y ? 1 : 2}`", "`stuff ${x} ${y ? 1 : 2}`;\n", "`${x}`, y;\n")
	expectPrintedNormalAndMangle(t, "`stuff ${x} ${y ? 1 : 2} ${z}`", "`stuff ${x} ${y ? 1 : 2} ${z}`;\n", "`${x}`, y, `${z}`;\n")

	expectPrintedNormalAndMangle(t, "'a' + b + 'c' + d", "\"a\" + b + \"c\" + d;\n", "\"\" + b + d;\n")
	expectPrintedNormalAndMangle(t, "a + 'b' + c + 'd'", "a + \"b\" + c + \"d\";\n", "a + \"\" + c;\n")
	expectPrintedNormalAndMangle(t, "a + b + 'c' + 'd'", "a + b + \"cd\";\n", "a + b + \"\";\n")
	expectPrintedNormalAndMangle(t, "'a' + 'b' + c + d", "\"ab\" + c + d;\n", "\"\" + c + d;\n")
	expectPrintedNormalAndMangle(t, "(a + '') + (b + '')", "a + (b + \"\");\n", "a + (b + \"\");\n")

	// Make sure identifiers inside "with" statements are kept
	expectPrintedNormalAndMangle(t, "with (a) []", "with (a)\n  [];\n", "with (a)\n  ;\n")
	expectPrintedNormalAndMangle(t, "var a; with (b) a", "var a;\nwith (b)\n  a;\n", "var a;\nwith (b)\n  a;\n")
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

	// Check substituting a side-effect free value into normal binary operators
	check("let x = 1; return x + 2", "return 1 + 2;")
	check("let x = 1; return 2 + x", "return 2 + 1;")
	check("let x = 1; return x + arg0", "return 1 + arg0;")
	check("let x = 1; return arg0 + x", "return arg0 + 1;")
	check("let x = 1; return x + fn()", "return 1 + fn();")
	check("let x = 1; return fn() + x", "let x = 1;\nreturn fn() + x;")
	check("let x = 1; return x + undef", "return 1 + undef;")
	check("let x = 1; return undef + x", "let x = 1;\nreturn undef + x;")

	// Check substituting a value with side-effects into normal binary operators
	check("let x = fn(); return x + 2", "return fn() + 2;")
	check("let x = fn(); return 2 + x", "return 2 + fn();")
	check("let x = fn(); return x + arg0", "return fn() + arg0;")
	check("let x = fn(); return arg0 + x", "let x = fn();\nreturn arg0 + x;")
	check("let x = fn(); return x + fn2()", "return fn() + fn2();")
	check("let x = fn(); return fn2() + x", "let x = fn();\nreturn fn2() + x;")
	check("let x = fn(); return x + undef", "return fn() + undef;")
	check("let x = fn(); return undef + x", "let x = fn();\nreturn undef + x;")

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
	check("let x = fn(); arg0 += x", "let x = fn();\narg0 += x;")
	check("let x = fn(); arg0 ||= x", "let x = fn();\narg0 ||= x;")

	// Cannot substitute past mutating binary operators when the left operand has side effects
	check("let x = 1; y.z = x", "let x = 1;\ny.z = x;")
	check("let x = 1; y.z += x", "let x = 1;\ny.z += x;")
	check("let x = 1; y.z ||= x", "let x = 1;\ny.z ||= x;")
	check("let x = fn(); y.z = x", "let x = fn();\ny.z = x;")
	check("let x = fn(); y.z += x", "let x = fn();\ny.z += x;")
	check("let x = fn(); y.z ||= x", "let x = fn();\ny.z ||= x;")

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
	check("let x = arg0; return (arg1 ? 1 : 2) ? x : 3;", "return arg0;")
	check("let x = arg0; return (arg1 ? 1 : 2) ? 3 : x;", "let x = arg0;\nreturn 3;")
	check("let x = arg0; return (arg1 ? y : 1) ? x : 2;", "let x = arg0;\nreturn !arg1 || y ? x : 2;")
	check("let x = arg0; return (arg1 ? 1 : y) ? x : 2;", "let x = arg0;\nreturn arg1 || y ? x : 2;")
	check("let x = arg0; return (arg1 ? y : 1) ? 2 : x;", "let x = arg0;\nreturn !arg1 || y ? 2 : x;")
	check("let x = arg0; return (arg1 ? 1 : y) ? 2 : x;", "let x = arg0;\nreturn arg1 || y ? 2 : x;")
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
		"let x = 0, y = { valueOf() {\n  x = 1;\n} }, z = x;\nreturn [y == 1, z];")

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
	check("let x = arg0; return {x};", "return { x: arg0 };")
	check("let x = arg0; return {x: y, y: x};", "let x = arg0;\nreturn { x: y, y: x };")
	check("let x = arg0; return {x: arg1, y: x};", "return { x: arg1, y: arg0 };")
	check("let x = arg0; return {[x]: 0};", "return { [arg0]: 0 };")
	check("let x = arg0; return {[y]: x};", "let x = arg0;\nreturn { [y]: x };")
	check("let x = arg0; return {[arg1]: x};", "let x = arg0;\nreturn { [arg1]: x };")
	check("let x = arg0; return {y() {}, x};", "return { y() {\n}, x: arg0 };")
	check("let x = arg0; return {[y]() {}, x};", "let x = arg0;\nreturn { [y]() {\n}, x };")
	check("let x = arg0; return {...x};", "return { ...arg0 };")
	check("let x = arg0; return {...x, y};", "return { ...arg0, y };")
	check("let x = arg0; return {x, ...y};", "return { x: arg0, ...y };")
	check("let x = arg0; return {...y, x};", "let x = arg0;\nreturn { ...y, x };")

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

	// Cannot substitute into call targets when it would change "this"
	check("let x = arg0; x()", "arg0();")
	check("let x = arg0; (0, x)()", "arg0();")
	check("let x = arg0.foo; x.bar()", "arg0.foo.bar();")
	check("let x = arg0.foo; x[bar]()", "arg0.foo[bar]();")
	check("let x = arg0.foo; x()", "let x = arg0.foo;\nx();")
	check("let x = arg0[foo]; x()", "let x = arg0[foo];\nx();")
	check("let x = arg0?.foo; x()", "let x = arg0?.foo;\nx();")
	check("let x = arg0?.[foo]; x()", "let x = arg0?.[foo];\nx();")
	check("let x = arg0.foo; (0, x)()", "let x = arg0.foo;\nx();")
	check("let x = arg0[foo]; (0, x)()", "let x = arg0[foo];\nx();")
	check("let x = arg0?.foo; (0, x)()", "let x = arg0?.foo;\nx();")
	check("let x = arg0?.[foo]; (0, x)()", "let x = arg0?.[foo];\nx();")

	// Explicitly allow reordering calls that are both marked as "/* @__PURE__ */".
	// This happens because only two expressions that are free from side-effects
	// can be freely reordered, and marking something as "/* @__PURE__ */" tells
	// us that it has no side effects.
	check("let x = arg0(); arg1() + x", "let x = arg0();\narg1() + x;")
	check("let x = arg0(); /* @__PURE__ */ arg1() + x", "let x = arg0();\n/* @__PURE__ */ arg1() + x;")
	check("let x = /* @__PURE__ */ arg0(); arg1() + x", "let x = /* @__PURE__ */ arg0();\narg1() + x;")
	check("let x = /* @__PURE__ */ arg0(); /* @__PURE__ */ arg1() + x", "/* @__PURE__ */ arg1() + /* @__PURE__ */ arg0();")
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
	expectPrintedMangle(t, "if (1) a(); else { function a() {} }", "if (1)\n  a();\nelse\n  var a;\n")
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
	expectPrinted(t, "// @license", "// @license\n")
	expectPrinted(t, "// @preserve", "// @preserve\n")

	expectPrinted(t, "/**/", "")
	expectPrinted(t, "/*preserve*/", "")
	expectPrinted(t, "/*@__PURE__*/", "")
	expectPrinted(t, "/*!*/", "/*!*/\n")
	expectPrinted(t, "/*@license*/", "/*@license*/\n")
	expectPrinted(t, "/*@preserve*/", "/*@preserve*/\n")
	expectPrinted(t, "/*\n * @license\n */", "/*\n * @license\n */\n")
	expectPrinted(t, "/*\n * @preserve\n */", "/*\n * @preserve\n */\n")

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
	expectParseError(t, "var\u0008x", "<stdin>: ERROR: Expected identifier but found \"\\b\"\n")
	for _, s := range whitespace {
		expectPrinted(t, "var"+s+"x", "var x;\n")
	}

	// Test "js_lexer.NextInsideJSXElement()"
	expectParseErrorJSX(t, "<x\u0008y/>", "<stdin>: ERROR: Expected \">\" but found \"\\b\"\n")
	for _, s := range whitespace {
		expectPrintedJSX(t, "<x"+s+"y/>", "/* @__PURE__ */ React.createElement(\"x\", { y: true });\n")
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
		r, _ := helpers.DecodeWTF8Rune(s)
		expectParseError(t, "var"+s+"x", fmt.Sprintf("<stdin>: ERROR: Expected identifier but found \"\\u%04x\"\n", r))
	}

	// Test "js_lexer.NextInsideJSXElement()"
	for _, s := range invalidWhitespaceInJS {
		r, _ := helpers.DecodeWTF8Rune(s)
		expectParseErrorJSX(t, "<x"+s+"y/>", fmt.Sprintf("<stdin>: ERROR: Expected \">\" but found \"\\u%04x\"\n", r))
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
	expectPrinted(t, "function f() { new.target }", "function f() {\n  new.target;\n}\n")
	expectPrinted(t, "function f() { (new.target) }", "function f() {\n  new.target;\n}\n")
	expectPrinted(t, "function f() { () => new.target }", "function f() {\n  () => new.target;\n}\n")
	expectPrinted(t, "class Foo { x = new.target }", "class Foo {\n  x = new.target;\n}\n")

	expectParseError(t, "new.t\\u0061rget", "<stdin>: ERROR: Unexpected \"t\\\\u0061rget\"\n")
	expectParseError(t, "new.target", "<stdin>: ERROR: Cannot use \"new.target\" here:\n")
	expectParseError(t, "() => new.target", "<stdin>: ERROR: Cannot use \"new.target\" here:\n")
	expectParseError(t, "class Foo { [new.target] }", "<stdin>: ERROR: Cannot use \"new.target\" here:\n")
}

func TestJSX(t *testing.T) {
	expectParseErrorJSX(t, "<div>></div>",
		"<stdin>: WARNING: The character \">\" is not valid inside a JSX element\n"+
			"NOTE: Did you mean to escape it as \"{'>'}\" instead?\n")
	expectParseErrorJSX(t, "<div>{1}}</div>",
		"<stdin>: WARNING: The character \"}\" is not valid inside a JSX element\n"+
			"NOTE: Did you mean to escape it as \"{'}'}\" instead?\n")
	expectPrintedJSX(t, "<div>></div>", "/* @__PURE__ */ React.createElement(\"div\", null, \">\");\n")
	expectPrintedJSX(t, "<div>{1}}</div>", "/* @__PURE__ */ React.createElement(\"div\", null, 1, \"}\");\n")

	expectParseError(t, "<a/>", "<stdin>: ERROR: The JSX syntax extension is not currently enabled\n"+
		"NOTE: The esbuild loader for this file is currently set to \"js\" but it must be set to \"jsx\" to be able to parse JSX syntax. "+
		"You can use 'Loader: map[string]api.Loader{\".js\": api.LoaderJSX}' to do that.\n")

	expectPrintedJSX(t, "<a/>", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "<a></a>", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "<A/>", "/* @__PURE__ */ React.createElement(A, null);\n")
	expectPrintedJSX(t, "<a.b/>", "/* @__PURE__ */ React.createElement(a.b, null);\n")
	expectPrintedJSX(t, "<_a/>", "/* @__PURE__ */ React.createElement(_a, null);\n")
	expectPrintedJSX(t, "<a-b/>", "/* @__PURE__ */ React.createElement(\"a-b\", null);\n")
	expectPrintedJSX(t, "<a0/>", "/* @__PURE__ */ React.createElement(\"a0\", null);\n")
	expectParseErrorJSX(t, "<0a/>", "<stdin>: ERROR: Expected identifier but found \"0\"\n")

	expectPrintedJSX(t, "<a b/>", "/* @__PURE__ */ React.createElement(\"a\", { b: true });\n")
	expectPrintedJSX(t, "<a b=\"\\\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"\\\\\" });\n")
	expectPrintedJSX(t, "<a b=\"<>\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"<>\" });\n")
	expectPrintedJSX(t, "<a b=\"&lt;&gt;\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"<>\" });\n")
	expectPrintedJSX(t, "<a b=\"&wrong;\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"&wrong;\" });\n")
	expectPrintedJSX(t, "<a b={1, 2}/>", "/* @__PURE__ */ React.createElement(\"a\", { b: (1, 2) });\n")
	expectPrintedJSX(t, "<a b={<c/>}/>", "/* @__PURE__ */ React.createElement(\"a\", { b: /* @__PURE__ */ React.createElement(\"c\", null) });\n")
	expectPrintedJSX(t, "<a {...props}/>", "/* @__PURE__ */ React.createElement(\"a\", { ...props });\n")
	expectPrintedJSX(t, "<a b=\"🙂\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"🙂\" });\n")

	expectPrintedJSX(t, "<a>\n</a>", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "<a>123</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"123\");\n")
	expectPrintedJSX(t, "<a>}</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"}\");\n")
	expectPrintedJSX(t, "<a>=</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"=\");\n")
	expectPrintedJSX(t, "<a>></a>", "/* @__PURE__ */ React.createElement(\"a\", null, \">\");\n")
	expectPrintedJSX(t, "<a>>=</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \">=\");\n")
	expectPrintedJSX(t, "<a>>></a>", "/* @__PURE__ */ React.createElement(\"a\", null, \">>\");\n")
	expectPrintedJSX(t, "<a>{}</a>", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "<a>{/* comment */}</a>", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "<a>b{}</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"b\");\n")
	expectPrintedJSX(t, "<a>b{/* comment */}</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"b\");\n")
	expectPrintedJSX(t, "<a>{}c</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"c\");\n")
	expectPrintedJSX(t, "<a>{/* comment */}c</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"c\");\n")
	expectPrintedJSX(t, "<a>b{}c</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"b\", \"c\");\n")
	expectPrintedJSX(t, "<a>b{/* comment */}c</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"b\", \"c\");\n")
	expectPrintedJSX(t, "<a>{1, 2}</a>", "/* @__PURE__ */ React.createElement(\"a\", null, (1, 2));\n")
	expectPrintedJSX(t, "<a>&lt;&gt;</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"<>\");\n")
	expectPrintedJSX(t, "<a>&wrong;</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"&wrong;\");\n")
	expectPrintedJSX(t, "<a>🙂</a>", "/* @__PURE__ */ React.createElement(\"a\", null, \"🙂\");\n")
	expectPrintedJSX(t, "<a>{...children}</a>", "/* @__PURE__ */ React.createElement(\"a\", null, ...children);\n")

	// Note: The TypeScript compiler and Babel disagree. This matches TypeScript.
	expectPrintedJSX(t, "<a b=\"   c\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"   c\" });\n")
	expectPrintedJSX(t, "<a b=\"   \nc\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"   \\nc\" });\n")
	expectPrintedJSX(t, "<a b=\"\n   c\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"\\n   c\" });\n")
	expectPrintedJSX(t, "<a b=\"c   \"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"c   \" });\n")
	expectPrintedJSX(t, "<a b=\"c   \n\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"c   \\n\" });\n")
	expectPrintedJSX(t, "<a b=\"c\n   \"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"c\\n   \" });\n")
	expectPrintedJSX(t, "<a b=\"c   d\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"c   d\" });\n")
	expectPrintedJSX(t, "<a b=\"c   \nd\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"c   \\nd\" });\n")
	expectPrintedJSX(t, "<a b=\"c\n   d\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"c\\n   d\" });\n")
	expectPrintedJSX(t, "<a b=\"   c\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"   c\" });\n")
	expectPrintedJSX(t, "<a b=\"   \nc\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"   \\nc\" });\n")
	expectPrintedJSX(t, "<a b=\"\n   c\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"\\n   c\" });\n")

	// Same test as above except with multi-byte Unicode characters
	expectPrintedJSX(t, "<a b=\"   🙂\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"   🙂\" });\n")
	expectPrintedJSX(t, "<a b=\"   \n🙂\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"   \\n🙂\" });\n")
	expectPrintedJSX(t, "<a b=\"\n   🙂\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"\\n   🙂\" });\n")
	expectPrintedJSX(t, "<a b=\"🙂   \"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"🙂   \" });\n")
	expectPrintedJSX(t, "<a b=\"🙂   \n\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"🙂   \\n\" });\n")
	expectPrintedJSX(t, "<a b=\"🙂\n   \"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"🙂\\n   \" });\n")
	expectPrintedJSX(t, "<a b=\"🙂   🍕\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"🙂   🍕\" });\n")
	expectPrintedJSX(t, "<a b=\"🙂   \n🍕\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"🙂   \\n🍕\" });\n")
	expectPrintedJSX(t, "<a b=\"🙂\n   🍕\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"🙂\\n   🍕\" });\n")
	expectPrintedJSX(t, "<a b=\"   🙂\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"   🙂\" });\n")
	expectPrintedJSX(t, "<a b=\"   \n🙂\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"   \\n🙂\" });\n")
	expectPrintedJSX(t, "<a b=\"\n   🙂\"/>", "/* @__PURE__ */ React.createElement(\"a\", { b: \"\\n   🙂\" });\n")

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

	expectParseErrorJSX(t, "<a b=true/>", "<stdin>: ERROR: Expected \"{\" but found \"true\"\n")
	expectParseErrorJSX(t, "</a>", "<stdin>: ERROR: Expected identifier but found \"/\"\n")
	expectParseErrorJSX(t, "<></b>", "<stdin>: ERROR: Expected closing \"b\" tag to match opening \"\" tag\n<stdin>: NOTE: The opening \"\" tag is here:\n")
	expectParseErrorJSX(t, "<a></>", "<stdin>: ERROR: Expected closing \"\" tag to match opening \"a\" tag\n<stdin>: NOTE: The opening \"a\" tag is here:\n")
	expectParseErrorJSX(t, "<a></b>", "<stdin>: ERROR: Expected closing \"b\" tag to match opening \"a\" tag\n<stdin>: NOTE: The opening \"a\" tag is here:\n")
	expectParseErrorJSX(t, "<\na\n.\nb\n>\n<\n/\nc\n.\nd\n>",
		"<stdin>: ERROR: Expected closing \"c.d\" tag to match opening \"a.b\" tag\n<stdin>: NOTE: The opening \"a.b\" tag is here:\n")
	expectParseErrorJSX(t, "<a-b.c>", "<stdin>: ERROR: Expected \">\" but found \".\"\n")
	expectParseErrorJSX(t, "<a.b-c>", "<stdin>: ERROR: Unexpected \"-\"\n")

	expectPrintedJSX(t, "< /**/ a/>", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "< //\n a/>", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "<a /**/ />", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "<a //\n />", "/* @__PURE__ */ React.createElement(\n  \"a\",\n  null\n);\n")
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
	expectPrintedJSX(t, "<a \U00020000={0}/>", "/* @__PURE__ */ React.createElement(\"a\", { \"\U00020000\": 0 });\n")

	// Comment tests
	expectParseErrorJSX(t, "<a /* />", "<stdin>: ERROR: Expected \"*/\" to terminate multi-line comment\n<stdin>: NOTE: The multi-line comment starts here:\n")
	expectParseErrorJSX(t, "<a /*/ />", "<stdin>: ERROR: Expected \"*/\" to terminate multi-line comment\n<stdin>: NOTE: The multi-line comment starts here:\n")
	expectParseErrorJSX(t, "<a // />", "<stdin>: ERROR: Expected \">\" but found end of file\n")
	expectParseErrorJSX(t, "<a /**/>", "<stdin>: ERROR: Unexpected end of file before a closing \"a\" tag\n<stdin>: NOTE: The opening \"a\" tag is here:\n")
	expectParseErrorJSX(t, "<a /**/ />", "")
	expectParseErrorJSX(t, "<a // \n />", "")
	expectParseErrorJSX(t, "<a b/* />", "<stdin>: ERROR: Expected \"*/\" to terminate multi-line comment\n<stdin>: NOTE: The multi-line comment starts here:\n")
	expectParseErrorJSX(t, "<a b/*/ />", "<stdin>: ERROR: Expected \"*/\" to terminate multi-line comment\n<stdin>: NOTE: The multi-line comment starts here:\n")
	expectParseErrorJSX(t, "<a b// />", "<stdin>: ERROR: Expected \">\" but found end of file\n")
	expectParseErrorJSX(t, "<a b/**/>", "<stdin>: ERROR: Unexpected end of file before a closing \"a\" tag\n<stdin>: NOTE: The opening \"a\" tag is here:\n")
	expectParseErrorJSX(t, "<a b/**/ />", "")
	expectParseErrorJSX(t, "<a b// \n />", "")

	// JSX namespaced names
	for _, colon := range []string{":", " :", ": ", " : "} {
		expectPrintedJSX(t, "<a"+colon+"b/>", "/* @__PURE__ */ React.createElement(\"a:b\", null);\n")
		expectPrintedJSX(t, "<a-b"+colon+"c-d/>", "/* @__PURE__ */ React.createElement(\"a-b:c-d\", null);\n")
		expectPrintedJSX(t, "<a-"+colon+"b-/>", "/* @__PURE__ */ React.createElement(\"a-:b-\", null);\n")
		expectPrintedJSX(t, "<Te"+colon+"st/>", "/* @__PURE__ */ React.createElement(\"Te:st\", null);\n")
		expectPrintedJSX(t, "<x a"+colon+"b/>", "/* @__PURE__ */ React.createElement(\"x\", { \"a:b\": true });\n")
		expectPrintedJSX(t, "<x a-b"+colon+"c-d/>", "/* @__PURE__ */ React.createElement(\"x\", { \"a-b:c-d\": true });\n")
		expectPrintedJSX(t, "<x a-"+colon+"b-/>", "/* @__PURE__ */ React.createElement(\"x\", { \"a-:b-\": true });\n")
		expectPrintedJSX(t, "<x Te"+colon+"st/>", "/* @__PURE__ */ React.createElement(\"x\", { \"Te:st\": true });\n")
		expectPrintedJSX(t, "<x a"+colon+"b={0}/>", "/* @__PURE__ */ React.createElement(\"x\", { \"a:b\": 0 });\n")
		expectPrintedJSX(t, "<x a-b"+colon+"c-d={0}/>", "/* @__PURE__ */ React.createElement(\"x\", { \"a-b:c-d\": 0 });\n")
		expectPrintedJSX(t, "<x a-"+colon+"b-={0}/>", "/* @__PURE__ */ React.createElement(\"x\", { \"a-:b-\": 0 });\n")
		expectPrintedJSX(t, "<x Te"+colon+"st={0}/>", "/* @__PURE__ */ React.createElement(\"x\", { \"Te:st\": 0 });\n")
		expectPrintedJSX(t, "<a-b a-b={a-b}/>", "/* @__PURE__ */ React.createElement(\"a-b\", { \"a-b\": a - b });\n")
		expectParseErrorJSX(t, "<x"+colon+"/>", "<stdin>: ERROR: Expected identifier after \"x:\" in namespaced JSX name\n")
		expectParseErrorJSX(t, "<x"+colon+"y"+colon+"/>", "<stdin>: ERROR: Expected \">\" but found \":\"\n")
		expectParseErrorJSX(t, "<x"+colon+"0y/>", "<stdin>: ERROR: Expected identifier after \"x:\" in namespaced JSX name\n")
	}
}

func TestJSXSingleLine(t *testing.T) {
	expectPrintedJSX(t, "<x/>", "/* @__PURE__ */ React.createElement(\"x\", null);\n")
	expectPrintedJSX(t, "<x y/>", "/* @__PURE__ */ React.createElement(\"x\", { y: true });\n")
	expectPrintedJSX(t, "<x\n/>", "/* @__PURE__ */ React.createElement(\n  \"x\",\n  null\n);\n")
	expectPrintedJSX(t, "<x\ny/>", "/* @__PURE__ */ React.createElement(\n  \"x\",\n  {\n    y: true\n  }\n);\n")
	expectPrintedJSX(t, "<x y\n/>", "/* @__PURE__ */ React.createElement(\n  \"x\",\n  {\n    y: true\n  }\n);\n")
	expectPrintedJSX(t, "<x\n{...y}/>", "/* @__PURE__ */ React.createElement(\n  \"x\",\n  {\n    ...y\n  }\n);\n")
}

func TestJSXPragmas(t *testing.T) {
	expectPrintedJSX(t, "// @jsx h\n<a/>", "/* @__PURE__ */ h(\"a\", null);\n")
	expectPrintedJSX(t, "/*@jsx h*/\n<a/>", "/* @__PURE__ */ h(\"a\", null);\n")
	expectPrintedJSX(t, "/* @jsx h */\n<a/>", "/* @__PURE__ */ h(\"a\", null);\n")
	expectPrintedJSX(t, "<a/>\n// @jsx h", "/* @__PURE__ */ h(\"a\", null);\n")
	expectPrintedJSX(t, "<a/>\n/*@jsx h*/", "/* @__PURE__ */ h(\"a\", null);\n")
	expectPrintedJSX(t, "<a/>\n/* @jsx h */", "/* @__PURE__ */ h(\"a\", null);\n")
	expectPrintedJSX(t, "// @jsx a.b.c\n<a/>", "/* @__PURE__ */ a.b.c(\"a\", null);\n")
	expectPrintedJSX(t, "/*@jsx a.b.c*/\n<a/>", "/* @__PURE__ */ a.b.c(\"a\", null);\n")
	expectPrintedJSX(t, "/* @jsx a.b.c */\n<a/>", "/* @__PURE__ */ a.b.c(\"a\", null);\n")

	expectPrintedJSX(t, "// @jsxFrag f\n<></>", "/* @__PURE__ */ React.createElement(f, null);\n")
	expectPrintedJSX(t, "/*@jsxFrag f*/\n<></>", "/* @__PURE__ */ React.createElement(f, null);\n")
	expectPrintedJSX(t, "/* @jsxFrag f */\n<></>", "/* @__PURE__ */ React.createElement(f, null);\n")
	expectPrintedJSX(t, "<></>\n// @jsxFrag f", "/* @__PURE__ */ React.createElement(f, null);\n")
	expectPrintedJSX(t, "<></>\n/*@jsxFrag f*/", "/* @__PURE__ */ React.createElement(f, null);\n")
	expectPrintedJSX(t, "<></>\n/* @jsxFrag f */", "/* @__PURE__ */ React.createElement(f, null);\n")
	expectPrintedJSX(t, "// @jsxFrag a.b.c\n<></>", "/* @__PURE__ */ React.createElement(a.b.c, null);\n")
	expectPrintedJSX(t, "/*@jsxFrag a.b.c*/\n<></>", "/* @__PURE__ */ React.createElement(a.b.c, null);\n")
	expectPrintedJSX(t, "/* @jsxFrag a.b.c */\n<></>", "/* @__PURE__ */ React.createElement(a.b.c, null);\n")
}

func TestJSXAutomatic(t *testing.T) {
	// Prod, without runtime imports
	p := JSXAutomaticTestOptions{Development: false, OmitJSXRuntimeForTests: true}
	expectPrintedJSXAutomatic(t, p, "<div>></div>", "/* @__PURE__ */ jsx(\"div\", { children: \">\" });\n")
	expectPrintedJSXAutomatic(t, p, "<div>{1}}</div>", "/* @__PURE__ */ jsxs(\"div\", { children: [\n  1,\n  \"}\"\n] });\n")
	expectPrintedJSXAutomatic(t, p, "<div key={true} />", "/* @__PURE__ */ jsx(\"div\", {}, true);\n")
	expectPrintedJSXAutomatic(t, p, "<div key=\"key\" />", "/* @__PURE__ */ jsx(\"div\", {}, \"key\");\n")
	expectPrintedJSXAutomatic(t, p, "<div key=\"key\" {...props} />", "/* @__PURE__ */ jsx(\"div\", { ...props }, \"key\");\n")
	expectPrintedJSXAutomatic(t, p, "<div {...props} key=\"key\" />", "/* @__PURE__ */ createElement(\"div\", { ...props, key: \"key\" });\n") // Falls back to createElement
	expectPrintedJSXAutomatic(t, p, "<div>{...children}</div>", "/* @__PURE__ */ jsxs(\"div\", { children: [\n  ...children\n] });\n")
	expectPrintedJSXAutomatic(t, p, "<div>{...children}<a/></div>", "/* @__PURE__ */ jsxs(\"div\", { children: [\n  ...children,\n  /* @__PURE__ */ jsx(\"a\", {})\n] });\n")
	expectPrintedJSXAutomatic(t, p, "<>></>", "/* @__PURE__ */ jsx(Fragment, { children: \">\" });\n")

	expectParseErrorJSXAutomatic(t, p, "<a key/>",
		`<stdin>: ERROR: Please provide an explicit value for "key":
NOTE: Using "key" as a shorthand for "key={true}" is not allowed when using React's "automatic" JSX transform.
`)
	expectParseErrorJSXAutomatic(t, p, "<div __self={self} />",
		`<stdin>: ERROR: Duplicate "__self" prop found:
NOTE: Both "__source" and "__self" are set automatically by esbuild when using React's "automatic" JSX transform. This duplicate prop may have come from a plugin.
`)
	expectParseErrorJSXAutomatic(t, p, "<div __source=\"/path/to/source.jsx\" />",
		`<stdin>: ERROR: Duplicate "__source" prop found:
NOTE: Both "__source" and "__self" are set automatically by esbuild when using React's "automatic" JSX transform. This duplicate prop may have come from a plugin.
`)

	// Prod, with runtime imports
	pr := JSXAutomaticTestOptions{Development: false}
	expectPrintedJSXAutomatic(t, pr, "<div/>", "import { jsx } from \"react/jsx-runtime\";\n/* @__PURE__ */ jsx(\"div\", {});\n")
	expectPrintedJSXAutomatic(t, pr, "<><a/><b/></>", "import { Fragment, jsx, jsxs } from \"react/jsx-runtime\";\n/* @__PURE__ */ jsxs(Fragment, { children: [\n  /* @__PURE__ */ jsx(\"a\", {}),\n  /* @__PURE__ */ jsx(\"b\", {})\n] });\n")
	expectPrintedJSXAutomatic(t, pr, "<div {...props} key=\"key\" />", "import { createElement } from \"react\";\n/* @__PURE__ */ createElement(\"div\", { ...props, key: \"key\" });\n")
	expectPrintedJSXAutomatic(t, pr, "<><div {...props} key=\"key\" /></>", "import { Fragment, jsx } from \"react/jsx-runtime\";\nimport { createElement } from \"react\";\n/* @__PURE__ */ jsx(Fragment, { children: /* @__PURE__ */ createElement(\"div\", { ...props, key: \"key\" }) });\n")

	pri := JSXAutomaticTestOptions{Development: false, ImportSource: "my-jsx-lib"}
	expectPrintedJSXAutomatic(t, pri, "<div/>", "import { jsx } from \"my-jsx-lib/jsx-runtime\";\n/* @__PURE__ */ jsx(\"div\", {});\n")
	expectPrintedJSXAutomatic(t, pri, "<div {...props} key=\"key\" />", "import { createElement } from \"my-jsx-lib\";\n/* @__PURE__ */ createElement(\"div\", { ...props, key: \"key\" });\n")

	// Impure JSX call expressions
	pi := JSXAutomaticTestOptions{SideEffects: true, ImportSource: "my-jsx-lib"}
	expectPrintedJSXAutomatic(t, pi, "<a/>", "import { jsx } from \"my-jsx-lib/jsx-runtime\";\njsx(\"a\", {});\n")
	expectPrintedJSXAutomatic(t, pi, "<></>", "import { Fragment, jsx } from \"my-jsx-lib/jsx-runtime\";\njsx(Fragment, {});\n")

	// Dev, without runtime imports
	d := JSXAutomaticTestOptions{Development: true, OmitJSXRuntimeForTests: true}
	expectPrintedJSXAutomatic(t, d, "<div>></div>", "/* @__PURE__ */ jsxDEV(\"div\", { children: \">\" }, void 0, false, {\n  fileName: \"<stdin>\",\n  lineNumber: 1,\n  columnNumber: 1\n}, this);\n")
	expectPrintedJSXAutomatic(t, d, "<div>{1}}</div>", "/* @__PURE__ */ jsxDEV(\"div\", { children: [\n  1,\n  \"}\"\n] }, void 0, true, {\n  fileName: \"<stdin>\",\n  lineNumber: 1,\n  columnNumber: 1\n}, this);\n")
	expectPrintedJSXAutomatic(t, d, "<div key={true} />", "/* @__PURE__ */ jsxDEV(\"div\", {}, true, false, {\n  fileName: \"<stdin>\",\n  lineNumber: 1,\n  columnNumber: 1\n}, this);\n")
	expectPrintedJSXAutomatic(t, d, "<div key=\"key\" />", "/* @__PURE__ */ jsxDEV(\"div\", {}, \"key\", false, {\n  fileName: \"<stdin>\",\n  lineNumber: 1,\n  columnNumber: 1\n}, this);\n")
	expectPrintedJSXAutomatic(t, d, "<div key=\"key\" {...props} />", "/* @__PURE__ */ jsxDEV(\"div\", { ...props }, \"key\", false, {\n  fileName: \"<stdin>\",\n  lineNumber: 1,\n  columnNumber: 1\n}, this);\n")
	expectPrintedJSXAutomatic(t, d, "<div {...props} key=\"key\" />", "/* @__PURE__ */ createElement(\"div\", { ...props, key: \"key\" });\n") // Falls back to createElement
	expectPrintedJSXAutomatic(t, d, "<div>{...children}</div>", "/* @__PURE__ */ jsxDEV(\"div\", { children: [\n  ...children\n] }, void 0, true, {\n  fileName: \"<stdin>\",\n  lineNumber: 1,\n  columnNumber: 1\n}, this);\n")
	expectPrintedJSXAutomatic(t, d, "<div>\n  {...children}\n  <a/></div>", "/* @__PURE__ */ jsxDEV(\"div\", { children: [\n  ...children,\n  /* @__PURE__ */ jsxDEV(\"a\", {}, void 0, false, {\n    fileName: \"<stdin>\",\n    lineNumber: 3,\n    columnNumber: 3\n  }, this)\n] }, void 0, true, {\n  fileName: \"<stdin>\",\n  lineNumber: 1,\n  columnNumber: 1\n}, this);\n")
	expectPrintedJSXAutomatic(t, d, "<>></>", "/* @__PURE__ */ jsxDEV(Fragment, { children: \">\" }, void 0, false, {\n  fileName: \"<stdin>\",\n  lineNumber: 1,\n  columnNumber: 1\n}, this);\n")

	expectParseErrorJSXAutomatic(t, d, "<a key/>",
		`<stdin>: ERROR: Please provide an explicit value for "key":
NOTE: Using "key" as a shorthand for "key={true}" is not allowed when using React's "automatic" JSX transform.
`)
	expectParseErrorJSXAutomatic(t, d, "<div __self={self} />",
		`<stdin>: ERROR: Duplicate "__self" prop found:
NOTE: Both "__source" and "__self" are set automatically by esbuild when using React's "automatic" JSX transform. This duplicate prop may have come from a plugin.
`)
	expectParseErrorJSXAutomatic(t, d, "<div __source=\"/path/to/source.jsx\" />",
		`<stdin>: ERROR: Duplicate "__source" prop found:
NOTE: Both "__source" and "__self" are set automatically by esbuild when using React's "automatic" JSX transform. This duplicate prop may have come from a plugin.
`)

	// Line/column offset tests. Unlike Babel, TypeScript sometimes points to a
	// location other than the start of the element. I'm not sure if that's a bug
	// or not, but it seems weird. So I decided to match Babel instead.
	expectPrintedJSXAutomatic(t, d, "\r\n<x/>", "/* @__PURE__ */ jsxDEV(\"x\", {}, void 0, false, {\n  fileName: \"<stdin>\",\n  lineNumber: 2,\n  columnNumber: 1\n}, this);\n")
	expectPrintedJSXAutomatic(t, d, "\n\r<x/>", "/* @__PURE__ */ jsxDEV(\"x\", {}, void 0, false, {\n  fileName: \"<stdin>\",\n  lineNumber: 3,\n  columnNumber: 1\n}, this);\n")
	expectPrintedJSXAutomatic(t, d, "let 𐀀 = <x>🍕🍕🍕<y/></x>", "let 𐀀 = /* @__PURE__ */ jsxDEV(\"x\", { children: [\n  \"🍕🍕🍕\",\n  /* @__PURE__ */ jsxDEV(\"y\", {}, void 0, false, {\n    fileName: \"<stdin>\",\n    lineNumber: 1,\n    columnNumber: 19\n  }, this)\n] }, void 0, true, {\n  fileName: \"<stdin>\",\n  lineNumber: 1,\n  columnNumber: 10\n}, this);\n")

	// Dev, with runtime imports
	dr := JSXAutomaticTestOptions{Development: true}
	expectPrintedJSXAutomatic(t, dr, "<div/>", "import { jsxDEV } from \"react/jsx-dev-runtime\";\n/* @__PURE__ */ jsxDEV(\"div\", {}, void 0, false, {\n  fileName: \"<stdin>\",\n  lineNumber: 1,\n  columnNumber: 1\n}, this);\n")
	expectPrintedJSXAutomatic(t, dr, "<>\n  <a/>\n  <b/>\n</>", "import { Fragment, jsxDEV } from \"react/jsx-dev-runtime\";\n/* @__PURE__ */ jsxDEV(Fragment, { children: [\n  /* @__PURE__ */ jsxDEV(\"a\", {}, void 0, false, {\n    fileName: \"<stdin>\",\n    lineNumber: 2,\n    columnNumber: 3\n  }, this),\n  /* @__PURE__ */ jsxDEV(\"b\", {}, void 0, false, {\n    fileName: \"<stdin>\",\n    lineNumber: 3,\n    columnNumber: 3\n  }, this)\n] }, void 0, true, {\n  fileName: \"<stdin>\",\n  lineNumber: 1,\n  columnNumber: 1\n}, this);\n")

	dri := JSXAutomaticTestOptions{Development: true, ImportSource: "preact"}
	expectPrintedJSXAutomatic(t, dri, "<div/>", "import { jsxDEV } from \"preact/jsx-dev-runtime\";\n/* @__PURE__ */ jsxDEV(\"div\", {}, void 0, false, {\n  fileName: \"<stdin>\",\n  lineNumber: 1,\n  columnNumber: 1\n}, this);\n")
	expectPrintedJSXAutomatic(t, dri, "<>\n  <a/>\n  <b/>\n</>", "import { Fragment, jsxDEV } from \"preact/jsx-dev-runtime\";\n/* @__PURE__ */ jsxDEV(Fragment, { children: [\n  /* @__PURE__ */ jsxDEV(\"a\", {}, void 0, false, {\n    fileName: \"<stdin>\",\n    lineNumber: 2,\n    columnNumber: 3\n  }, this),\n  /* @__PURE__ */ jsxDEV(\"b\", {}, void 0, false, {\n    fileName: \"<stdin>\",\n    lineNumber: 3,\n    columnNumber: 3\n  }, this)\n] }, void 0, true, {\n  fileName: \"<stdin>\",\n  lineNumber: 1,\n  columnNumber: 1\n}, this);\n")

	// JSX namespaced names
	for _, colon := range []string{":", " :", ": ", " : "} {
		expectPrintedJSXAutomatic(t, p, "<a"+colon+"b/>", "/* @__PURE__ */ jsx(\"a:b\", {});\n")
		expectPrintedJSXAutomatic(t, p, "<a-b"+colon+"c-d/>", "/* @__PURE__ */ jsx(\"a-b:c-d\", {});\n")
		expectPrintedJSXAutomatic(t, p, "<a-"+colon+"b-/>", "/* @__PURE__ */ jsx(\"a-:b-\", {});\n")
		expectPrintedJSXAutomatic(t, p, "<Te"+colon+"st/>", "/* @__PURE__ */ jsx(\"Te:st\", {});\n")
		expectPrintedJSXAutomatic(t, p, "<x a"+colon+"b/>", "/* @__PURE__ */ jsx(\"x\", { \"a:b\": true });\n")
		expectPrintedJSXAutomatic(t, p, "<x a-b"+colon+"c-d/>", "/* @__PURE__ */ jsx(\"x\", { \"a-b:c-d\": true });\n")
		expectPrintedJSXAutomatic(t, p, "<x a-"+colon+"b-/>", "/* @__PURE__ */ jsx(\"x\", { \"a-:b-\": true });\n")
		expectPrintedJSXAutomatic(t, p, "<x Te"+colon+"st/>", "/* @__PURE__ */ jsx(\"x\", { \"Te:st\": true });\n")
		expectPrintedJSXAutomatic(t, p, "<x a"+colon+"b={0}/>", "/* @__PURE__ */ jsx(\"x\", { \"a:b\": 0 });\n")
		expectPrintedJSXAutomatic(t, p, "<x a-b"+colon+"c-d={0}/>", "/* @__PURE__ */ jsx(\"x\", { \"a-b:c-d\": 0 });\n")
		expectPrintedJSXAutomatic(t, p, "<x a-"+colon+"b-={0}/>", "/* @__PURE__ */ jsx(\"x\", { \"a-:b-\": 0 });\n")
		expectPrintedJSXAutomatic(t, p, "<x Te"+colon+"st={0}/>", "/* @__PURE__ */ jsx(\"x\", { \"Te:st\": 0 });\n")
		expectPrintedJSXAutomatic(t, p, "<a-b a-b={a-b}/>", "/* @__PURE__ */ jsx(\"a-b\", { \"a-b\": a - b });\n")
		expectParseErrorJSXAutomatic(t, p, "<x"+colon+"/>", "<stdin>: ERROR: Expected identifier after \"x:\" in namespaced JSX name\n")
		expectParseErrorJSXAutomatic(t, p, "<x"+colon+"y"+colon+"/>", "<stdin>: ERROR: Expected \">\" but found \":\"\n")
		expectParseErrorJSXAutomatic(t, p, "<x"+colon+"0y/>", "<stdin>: ERROR: Expected identifier after \"x:\" in namespaced JSX name\n")
	}

	// Enabling the "automatic" runtime means that any JSX element will cause the
	// file to be implicitly in strict mode due to the automatically-generated
	// import statement. This is the same behavior as the TypeScript compiler.
	strictModeError := "<stdin>: ERROR: With statements cannot be used in strict mode\n" +
		"<stdin>: NOTE: This file is implicitly in strict mode due to the JSX element here:\n" +
		"NOTE: When React's \"automatic\" JSX transform is enabled, using a JSX element automatically inserts an \"import\" statement at the top of the file " +
		"for the corresponding the JSX helper function. This means the file is considered an ECMAScript module, and all ECMAScript modules use strict mode.\n"
	expectPrintedJSX(t, "with (x) y(<z/>)", "with (x)\n  y(/* @__PURE__ */ React.createElement(\"z\", null));\n")
	expectPrintedJSXAutomatic(t, p, "with (x) y", "with (x)\n  y;\n")
	expectParseErrorJSX(t, "with (x) y(<z/>) // @jsxRuntime automatic", strictModeError)
	expectParseErrorJSXAutomatic(t, p, "with (x) y(<z/>)", strictModeError)
}

func TestJSXAutomaticPragmas(t *testing.T) {
	expectPrintedJSX(t, "// @jsxRuntime automatic\n<a/>", "import { jsx } from \"react/jsx-runtime\";\n/* @__PURE__ */ jsx(\"a\", {});\n")
	expectPrintedJSX(t, "/*@jsxRuntime automatic*/\n<a/>", "import { jsx } from \"react/jsx-runtime\";\n/* @__PURE__ */ jsx(\"a\", {});\n")
	expectPrintedJSX(t, "/* @jsxRuntime automatic */\n<a/>", "import { jsx } from \"react/jsx-runtime\";\n/* @__PURE__ */ jsx(\"a\", {});\n")
	expectPrintedJSX(t, "<a/>\n/*@jsxRuntime automatic*/", "import { jsx } from \"react/jsx-runtime\";\n/* @__PURE__ */ jsx(\"a\", {});\n")
	expectPrintedJSX(t, "<a/>\n/* @jsxRuntime automatic */", "import { jsx } from \"react/jsx-runtime\";\n/* @__PURE__ */ jsx(\"a\", {});\n")

	expectPrintedJSX(t, "// @jsxRuntime classic\n<a/>", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "/*@jsxRuntime classic*/\n<a/>", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "/* @jsxRuntime classic */\n<a/>", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "<a/>\n/*@jsxRuntime classic*/\n", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "<a/>\n/* @jsxRuntime classic */\n", "/* @__PURE__ */ React.createElement(\"a\", null);\n")

	expectParseErrorJSX(t, "// @jsxRuntime foo\n<a/>",
		`<stdin>: WARNING: Invalid JSX runtime: "foo"
NOTE: The JSX runtime can only be set to either "classic" or "automatic".
`)

	expectPrintedJSX(t, "// @jsxRuntime automatic @jsxImportSource src\n<a/>", "import { jsx } from \"src/jsx-runtime\";\n/* @__PURE__ */ jsx(\"a\", {});\n")
	expectPrintedJSX(t, "/*@jsxRuntime automatic @jsxImportSource src*/\n<a/>", "import { jsx } from \"src/jsx-runtime\";\n/* @__PURE__ */ jsx(\"a\", {});\n")
	expectPrintedJSX(t, "/*@jsxRuntime automatic*//*@jsxImportSource src*/\n<a/>", "import { jsx } from \"src/jsx-runtime\";\n/* @__PURE__ */ jsx(\"a\", {});\n")
	expectPrintedJSX(t, "/* @jsxRuntime automatic */\n/* @jsxImportSource src */\n<a/>", "import { jsx } from \"src/jsx-runtime\";\n/* @__PURE__ */ jsx(\"a\", {});\n")
	expectPrintedJSX(t, "<a/>\n/*@jsxRuntime automatic @jsxImportSource src*/", "import { jsx } from \"src/jsx-runtime\";\n/* @__PURE__ */ jsx(\"a\", {});\n")
	expectPrintedJSX(t, "<a/>\n/*@jsxRuntime automatic*/\n/*@jsxImportSource src*/", "import { jsx } from \"src/jsx-runtime\";\n/* @__PURE__ */ jsx(\"a\", {});\n")
	expectPrintedJSX(t, "<a/>\n/* @jsxRuntime automatic */\n/* @jsxImportSource src */", "import { jsx } from \"src/jsx-runtime\";\n/* @__PURE__ */ jsx(\"a\", {});\n")

	expectPrintedJSX(t, "// @jsxRuntime classic @jsxImportSource src\n<a/>", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectParseErrorJSX(t, "// @jsxRuntime classic @jsxImportSource src\n<a/>",
		`<stdin>: WARNING: The JSX import source cannot be set without also enabling React's "automatic" JSX transform
NOTE: You can enable React's "automatic" JSX transform for this file by using a "@jsxRuntime automatic" comment.
`)
	expectParseErrorJSX(t, "// @jsxImportSource src\n<a/>",
		`<stdin>: WARNING: The JSX import source cannot be set without also enabling React's "automatic" JSX transform
NOTE: You can enable React's "automatic" JSX transform for this file by using a "@jsxRuntime automatic" comment.
`)

	expectPrintedJSX(t, "// @jsxRuntime automatic @jsx h\n<a/>", "import { jsx } from \"react/jsx-runtime\";\n/* @__PURE__ */ jsx(\"a\", {});\n")
	expectParseErrorJSX(t, "// @jsxRuntime automatic @jsx h\n<a/>", "<stdin>: WARNING: The JSX factory cannot be set when using React's \"automatic\" JSX transform\n")

	expectPrintedJSX(t, "// @jsxRuntime automatic @jsxFrag f\n<></>", "import { Fragment, jsx } from \"react/jsx-runtime\";\n/* @__PURE__ */ jsx(Fragment, {});\n")
	expectParseErrorJSX(t, "// @jsxRuntime automatic @jsxFrag f\n<></>", "<stdin>: WARNING: The JSX fragment cannot be set when using React's \"automatic\" JSX transform\n")
}

func TestJSXSideEffects(t *testing.T) {
	expectPrintedJSX(t, "<a/>", "/* @__PURE__ */ React.createElement(\"a\", null);\n")
	expectPrintedJSX(t, "<></>", "/* @__PURE__ */ React.createElement(React.Fragment, null);\n")

	expectPrintedJSXSideEffects(t, "<a/>", "React.createElement(\"a\", null);\n")
	expectPrintedJSXSideEffects(t, "<></>", "React.createElement(React.Fragment, null);\n")
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
	expectParseError(t, "#foo", "<stdin>: ERROR: Unexpected \"#foo\"\n")
	expectParseError(t, "#foo in this", "<stdin>: ERROR: Unexpected \"#foo\"\n")
	expectParseError(t, "this.#foo", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseError(t, "this?.#foo", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseError(t, "({ #foo: 1 })", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseError(t, "class Foo { x = { #foo: 1 } }", "<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseError(t, "class Foo { x = #foo }", "<stdin>: ERROR: Expected \"in\" but found \"}\"\n")
	expectParseError(t, "class Foo { #foo; foo() { delete this.#foo } }",
		"<stdin>: ERROR: Deleting the private name \"#foo\" is forbidden\n")
	expectParseError(t, "class Foo { #foo; foo() { delete this?.#foo } }",
		"<stdin>: ERROR: Deleting the private name \"#foo\" is forbidden\n")
	expectParseError(t, "class Foo extends Bar { #foo; foo() { super.#foo } }",
		"<stdin>: ERROR: Expected identifier but found \"#foo\"\n")
	expectParseError(t, "class Foo { #foo = () => { for (#foo in this) ; } }",
		"<stdin>: ERROR: Unexpected \"#foo\"\n")
	expectParseError(t, "class Foo { #foo = () => { for (x = #foo in this) ; } }",
		"<stdin>: ERROR: Unexpected \"#foo\"\n")

	expectPrinted(t, "class Foo { #foo }", "class Foo {\n  #foo;\n}\n")
	expectPrinted(t, "class Foo { #foo = 1 }", "class Foo {\n  #foo = 1;\n}\n")
	expectPrinted(t, "class Foo { #foo = #foo in this }", "class Foo {\n  #foo = #foo in this;\n}\n")
	expectPrinted(t, "class Foo { #foo = #foo in (#bar in this); #bar }", "class Foo {\n  #foo = #foo in (#bar in this);\n  #bar;\n}\n")
	expectPrinted(t, "class Foo { #foo() {} }", "class Foo {\n  #foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { get #foo() {} }", "class Foo {\n  get #foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { set #foo(x) {} }", "class Foo {\n  set #foo(x) {\n  }\n}\n")
	expectPrinted(t, "class Foo { static #foo }", "class Foo {\n  static #foo;\n}\n")
	expectPrinted(t, "class Foo { static #foo = 1 }", "class Foo {\n  static #foo = 1;\n}\n")
	expectPrinted(t, "class Foo { static #foo() {} }", "class Foo {\n  static #foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static get #foo() {} }", "class Foo {\n  static get #foo() {\n  }\n}\n")
	expectPrinted(t, "class Foo { static set #foo(x) {} }", "class Foo {\n  static set #foo(x) {\n  }\n}\n")
	expectParseError(t, "class Foo { #foo = #foo in #bar in this; #bar }", "<stdin>: ERROR: Unexpected \"#bar\"\n")

	// The name "#constructor" is forbidden
	expectParseError(t, "class Foo { #constructor }", "<stdin>: ERROR: Invalid field name \"#constructor\"\n")
	expectParseError(t, "class Foo { #constructor() {} }", "<stdin>: ERROR: Invalid method name \"#constructor\"\n")
	expectParseError(t, "class Foo { static #constructor }", "<stdin>: ERROR: Invalid field name \"#constructor\"\n")
	expectParseError(t, "class Foo { static #constructor() {} }", "<stdin>: ERROR: Invalid method name \"#constructor\"\n")
	expectParseError(t, "class Foo { #\\u0063onstructor }", "<stdin>: ERROR: Invalid field name \"#constructor\"\n")
	expectParseError(t, "class Foo { #\\u0063onstructor() {} }", "<stdin>: ERROR: Invalid method name \"#constructor\"\n")
	expectParseError(t, "class Foo { static #\\u0063onstructor }", "<stdin>: ERROR: Invalid field name \"#constructor\"\n")
	expectParseError(t, "class Foo { static #\\u0063onstructor() {} }", "<stdin>: ERROR: Invalid method name \"#constructor\"\n")

	// Test escape sequences
	expectPrinted(t, "class Foo { #\\u0066oo; foo = this.#foo }", "class Foo {\n  #foo;\n  foo = this.#foo;\n}\n")
	expectPrinted(t, "class Foo { #fo\\u006f; foo = this.#foo }", "class Foo {\n  #foo;\n  foo = this.#foo;\n}\n")
	expectParseError(t, "class Foo { #\\u0020oo }", "<stdin>: ERROR: Invalid identifier: \"# oo\"\n")
	expectParseError(t, "class Foo { #fo\\u0020 }", "<stdin>: ERROR: Invalid identifier: \"#fo \"\n")

	errorText := `<stdin>: ERROR: The symbol "#foo" has already been declared
<stdin>: NOTE: The symbol "#foo" was originally declared here:
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
		"<stdin>: ERROR: Private name \"#foo\" must be declared in an enclosing class\n")
	expectParseError(t, "class Foo { #foo } class Bar { foo = this?.#foo }",
		"<stdin>: ERROR: Private name \"#foo\" must be declared in an enclosing class\n")
	expectParseError(t, "class Foo { #foo } class Bar { foo = #foo in this }",
		"<stdin>: ERROR: Private name \"#foo\" must be declared in an enclosing class\n")

	// Getter and setter warnings
	expectParseError(t, "class Foo { get #x() { this.#x = 1 } }",
		"<stdin>: WARNING: Writing to getter-only property \"#x\" will throw\n")
	expectParseError(t, "class Foo { get #x() { this.#x += 1 } }",
		"<stdin>: WARNING: Writing to getter-only property \"#x\" will throw\n")
	expectParseError(t, "class Foo { set #x(x) { this.#x } }",
		"<stdin>: WARNING: Reading from setter-only property \"#x\" will throw\n")
	expectParseError(t, "class Foo { set #x(x) { this.#x += 1 } }",
		"<stdin>: WARNING: Reading from setter-only property \"#x\" will throw\n")

	// Writing to method warnings
	expectParseError(t, "class Foo { #x() { this.#x = 1 } }",
		"<stdin>: WARNING: Writing to read-only method \"#x\" will throw\n")
	expectParseError(t, "class Foo { #x() { this.#x += 1 } }",
		"<stdin>: WARNING: Writing to read-only method \"#x\" will throw\n")

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

func TestImportAssertions(t *testing.T) {
	expectPrinted(t, "import 'x' assert {}", "import \"x\" assert {};\n")
	expectPrinted(t, "import 'x' assert {\n}", "import \"x\" assert {};\n")
	expectPrinted(t, "import 'x' assert\n{}", "import \"x\" assert {};\n")
	expectPrinted(t, "import 'x'\nassert\n{}", "import \"x\";\nassert;\n{\n}\n")
	expectPrinted(t, "import 'x' assert {type: 'json'}", "import \"x\" assert { type: \"json\" };\n")
	expectPrinted(t, "import 'x' assert {type: 'json',}", "import \"x\" assert { type: \"json\" };\n")
	expectPrinted(t, "import 'x' assert {'type': 'json'}", "import \"x\" assert { \"type\": \"json\" };\n")
	expectPrinted(t, "import 'x' assert {a: 'b', c: 'd'}", "import \"x\" assert { a: \"b\", c: \"d\" };\n")
	expectPrinted(t, "import 'x' assert {a: 'b', c: 'd',}", "import \"x\" assert { a: \"b\", c: \"d\" };\n")
	expectPrinted(t, "import 'x' assert {if: 'keyword'}", "import \"x\" assert { if: \"keyword\" };\n")
	expectPrintedMangle(t, "import 'x' assert {'type': 'json'}", "import \"x\" assert { type: \"json\" };\n")
	expectPrintedMangle(t, "import 'x' assert {'ty pe': 'json'}", "import \"x\" assert { \"ty pe\": \"json\" };\n")

	expectParseError(t, "import 'x' assert {,}", "<stdin>: ERROR: Expected identifier but found \",\"\n")
	expectParseError(t, "import 'x' assert {x}", "<stdin>: ERROR: Expected \":\" but found \"}\"\n")
	expectParseError(t, "import 'x' assert {x 'y'}", "<stdin>: ERROR: Expected \":\" but found \"'y'\"\n")
	expectParseError(t, "import 'x' assert {x: y}", "<stdin>: ERROR: Expected string but found \"y\"\n")
	expectParseError(t, "import 'x' assert {x: 'y',,}", "<stdin>: ERROR: Expected identifier but found \",\"\n")
	expectParseError(t, "import 'x' assert {`x`: 'y'}", "<stdin>: ERROR: Expected identifier but found \"`x`\"\n")
	expectParseError(t, "import 'x' assert {x: `y`}", "<stdin>: ERROR: Expected string but found \"`y`\"\n")
	expectParseError(t, "import 'x' assert: {x: 'y'}", "<stdin>: ERROR: Expected \"{\" but found \":\"\n")

	expectParseError(t, "import 'x' assert {x: 'y', x: 'y'}",
		"<stdin>: ERROR: Duplicate import assertion \"x\"\n<stdin>: NOTE: The first \"x\" was here:\n")
	expectParseError(t, "import 'x' assert {x: 'y', \\u0078: 'y'}",
		"<stdin>: ERROR: Duplicate import assertion \"x\"\n<stdin>: NOTE: The first \"x\" was here:\n")

	expectPrinted(t, "import x from 'x' assert {x: 'y'}", "import x from \"x\" assert { x: \"y\" };\n")
	expectPrinted(t, "import * as x from 'x' assert {x: 'y'}", "import * as x from \"x\" assert { x: \"y\" };\n")
	expectPrinted(t, "import {} from 'x' assert {x: 'y'}", "import {} from \"x\" assert { x: \"y\" };\n")
	expectPrinted(t, "export {} from 'x' assert {x: 'y'}", "export {} from \"x\" assert { x: \"y\" };\n")
	expectPrinted(t, "export * from 'x' assert {x: 'y'}", "export * from \"x\" assert { x: \"y\" };\n")

	expectPrinted(t, "import(x ? 'y' : 'z')", "x ? import(\"y\") : import(\"z\");\n")
	expectPrinted(t, "import(x ? 'y' : 'z', {assert: {}})",
		"x ? import(\"y\", { assert: {} }) : import(\"z\", { assert: {} });\n")
	expectPrinted(t, "import(x ? 'y' : 'z', {assert: {a: 'b'}})",
		"x ? import(\"y\", { assert: { a: \"b\" } }) : import(\"z\", { assert: { a: \"b\" } });\n")
	expectPrinted(t, "import(x ? 'y' : 'z', {assert: {'a': 'b'}})",
		"x ? import(\"y\", { assert: { \"a\": \"b\" } }) : import(\"z\", { assert: { \"a\": \"b\" } });\n")
	expectPrintedMangle(t, "import(x ? 'y' : 'z', {assert: {'a': 'b'}})",
		"x ? import(\"y\", { assert: { a: \"b\" } }) : import(\"z\", { assert: { a: \"b\" } });\n")
	expectPrintedMangle(t, "import(x ? 'y' : 'z', {assert: {'a a': 'b'}})",
		"x ? import(\"y\", { assert: { \"a a\": \"b\" } }) : import(\"z\", { assert: { \"a a\": \"b\" } });\n")

	expectPrinted(t, "import(x ? 'y' : 'z', {})", "import(x ? \"y\" : \"z\", {});\n")
	expectPrinted(t, "import(x ? 'y' : 'z', {assert: []})", "import(x ? \"y\" : \"z\", { assert: [] });\n")
	expectPrinted(t, "import(x ? 'y' : 'z', {asserts: {}})", "import(x ? \"y\" : \"z\", { asserts: {} });\n")
	expectPrinted(t, "import(x ? 'y' : 'z', {assert: {x: 1}})", "import(x ? \"y\" : \"z\", { assert: { x: 1 } });\n")

	expectPrintedTarget(t, 2015, "import 'x' assert {x: 'y'}", "import \"x\";\n")
	expectPrintedTarget(t, 2015, "import(x, {assert: {x: 'y'}})", "import(x);\n")
	expectPrintedTarget(t, 2015, "import(x, {assert: {x: 1}})", "import(x);\n")
	expectPrintedTarget(t, 2015, "import(x ? 'y' : 'z', {assert: {x: 'y'}})", "x ? import(\"y\") : import(\"z\");\n")
	expectPrintedTarget(t, 2015, "import(x ? 'y' : 'z', {assert: {x: 1}})", "import(x ? \"y\" : \"z\");\n")
	expectParseErrorTarget(t, 2015, "import(x ? 'y' : 'z', {assert: {x: foo()}})",
		"<stdin>: ERROR: Using an arbitrary value as the second argument to \"import()\" is not possible in the configured target environment\n")

	// Make sure there are no errors when bundling is disabled
	expectParseError(t, "import { foo } from 'x' assert {type: 'json'}", "")
	expectParseError(t, "export { foo } from 'x' assert {type: 'json'}", "")
}

func TestES5(t *testing.T) {
	// Do not generate "let" when emulating block-level function declarations and targeting ES5
	expectPrintedTarget(t, 2015, "if (1) function f() {}", "if (1) {\n  let f = function() {\n  };\n  var f = f;\n}\n")
	expectPrintedTarget(t, 5, "if (1) function f() {}", "if (1) {\n  var f = function() {\n  };\n  var f = f;\n}\n")

	expectParseErrorTarget(t, 5, "function foo(x = 0) {}",
		"<stdin>: ERROR: Transforming default arguments to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(function(x = 0) {})",
		"<stdin>: ERROR: Transforming default arguments to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(x = 0) => {}",
		"<stdin>: ERROR: Transforming default arguments to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "function foo(...x) {}",
		"<stdin>: ERROR: Transforming rest arguments to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(function(...x) {})",
		"<stdin>: ERROR: Transforming rest arguments to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(...x) => {}",
		"<stdin>: ERROR: Transforming rest arguments to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "foo(...x)",
		"<stdin>: ERROR: Transforming rest arguments to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "[...x]",
		"<stdin>: ERROR: Transforming array spread to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "for (var x of y) ;",
		"<stdin>: ERROR: Transforming for-of loops to the configured target environment is not supported yet\n")
	expectPrintedTarget(t, 5, "({ x })", "({ x: x });\n")
	expectParseErrorTarget(t, 5, "({ [x]: y })",
		"<stdin>: ERROR: Transforming object literal extensions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "({ x() {} });",
		"<stdin>: ERROR: Transforming object literal extensions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "({ get x() {} });", "")
	expectParseErrorTarget(t, 5, "({ set x(x) {} });", "")
	expectParseErrorTarget(t, 5, "({ get [x]() {} });",
		"<stdin>: ERROR: Transforming object literal extensions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "({ set [x](x) {} });",
		"<stdin>: ERROR: Transforming object literal extensions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "function foo([]) {}",
		"<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "function foo({}) {}",
		"<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(function([]) {})",
		"<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(function({}) {})",
		"<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "([]) => {}",
		"<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "({}) => {}",
		"<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "var [] = [];",
		"<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "var {} = {};",
		"<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "([] = []);",
		"<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "({} = {});",
		"<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "for ([] in []);",
		"<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "for ({} in []);",
		"<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "function foo([...x]) {}",
		"<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(function([...x]) {})",
		"<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "([...x]) => {}",
		"<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "function foo([...[x]]) {}",
		`<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet
<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet
<stdin>: ERROR: Transforming non-identifier array rest patterns to the configured target environment is not supported yet
`)
	expectParseErrorTarget(t, 5, "(function([...[x]]) {})",
		`<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet
<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet
<stdin>: ERROR: Transforming non-identifier array rest patterns to the configured target environment is not supported yet
`)
	expectParseErrorTarget(t, 5, "([...[x]]) => {}",
		`<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet
<stdin>: ERROR: Transforming destructuring to the configured target environment is not supported yet
<stdin>: ERROR: Transforming non-identifier array rest patterns to the configured target environment is not supported yet
`)
	expectParseErrorTarget(t, 5, "([...[x]])",
		"<stdin>: ERROR: Transforming array spread to the configured target environment is not supported yet\n")
	expectPrintedTarget(t, 5, "`abc`;", "\"abc\";\n")
	expectPrintedTarget(t, 5, "`a${b}`;", "\"a\".concat(b);\n")
	expectPrintedTarget(t, 5, "`${a}b`;", "\"\".concat(a, \"b\");\n")
	expectPrintedTarget(t, 5, "`${a}${b}`;", "\"\".concat(a).concat(b);\n")
	expectPrintedTarget(t, 5, "`a${b}c`;", "\"a\".concat(b, \"c\");\n")
	expectPrintedTarget(t, 5, "`a${b}${c}`;", "\"a\".concat(b).concat(c);\n")
	expectPrintedTarget(t, 5, "`a${b}${c}d`;", "\"a\".concat(b).concat(c, \"d\");\n")
	expectPrintedTarget(t, 5, "`a${b}c${d}`;", "\"a\".concat(b, \"c\").concat(d);\n")
	expectPrintedTarget(t, 5, "`a${b}c${d}e`;", "\"a\".concat(b, \"c\").concat(d, \"e\");\n")
	expectPrintedTarget(t, 5, "tag``;", "var _a;\ntag(_a || (_a = __template([\"\"])));\n")
	expectPrintedTarget(t, 5, "tag`abc`;", "var _a;\ntag(_a || (_a = __template([\"abc\"])));\n")
	expectPrintedTarget(t, 5, "tag`\\utf`;", "var _a;\ntag(_a || (_a = __template([void 0], [\"\\\\utf\"])));\n")
	expectPrintedTarget(t, 5, "tag`${a}b`;", "var _a;\ntag(_a || (_a = __template([\"\", \"b\"])), a);\n")
	expectPrintedTarget(t, 5, "tag`a${b}`;", "var _a;\ntag(_a || (_a = __template([\"a\", \"\"])), b);\n")
	expectPrintedTarget(t, 5, "tag`a${b}c`;", "var _a;\ntag(_a || (_a = __template([\"a\", \"c\"])), b);\n")
	expectPrintedTarget(t, 5, "tag`a${b}\\u`;", "var _a;\ntag(_a || (_a = __template([\"a\", void 0], [\"a\", \"\\\\u\"])), b);\n")
	expectPrintedTarget(t, 5, "tag`\\u${b}c`;", "var _a;\ntag(_a || (_a = __template([void 0, \"c\"], [\"\\\\u\", \"c\"])), b);\n")
	expectParseErrorTarget(t, 5, "class Foo { constructor() { new.target } }",
		"<stdin>: ERROR: Transforming class syntax to the configured target environment is not supported yet\n"+
			"<stdin>: ERROR: Transforming object literal extensions to the configured target environment is not supported yet\n"+
			"<stdin>: ERROR: Transforming new.target to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "const x = 1;",
		"<stdin>: ERROR: Transforming const to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "let x = 2;",
		"<stdin>: ERROR: Transforming let to the configured target environment is not supported yet\n")
	expectPrintedTarget(t, 5, "async => foo;", "(function(async) {\n  return foo;\n});\n")
	expectPrintedTarget(t, 5, "x => x;", "(function(x) {\n  return x;\n});\n")
	expectParseErrorTarget(t, 5, "async () => foo;",
		"<stdin>: ERROR: Transforming async functions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "class Foo {}",
		"<stdin>: ERROR: Transforming class syntax to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(class {});",
		"<stdin>: ERROR: Transforming class syntax to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "function* gen() {}",
		"<stdin>: ERROR: Transforming generator functions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "(function* () {});",
		"<stdin>: ERROR: Transforming generator functions to the configured target environment is not supported yet\n")
	expectParseErrorTarget(t, 5, "({ *foo() {} });",
		"<stdin>: ERROR: Transforming generator functions to the configured target environment is not supported yet\n")
}

func TestASCIIOnly(t *testing.T) {
	es5 := "<stdin>: ERROR: \"𐀀\" cannot be escaped in the configured target environment " +
		"but you can set the charset to \"utf8\" to allow unescaped Unicode characters\n"

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
	expectPrinted(t, "x.𐀀", "x[\"𐀀\"];\n")
	expectPrintedASCII(t, "x.π", "x.\\u03C0;\n")
	expectPrintedASCII(t, "x.𐀀", "x[\"\\u{10000}\"];\n")
	expectPrintedTargetASCII(t, 5, "x.π", "x.\\u03C0;\n")
	expectPrintedTargetASCII(t, 5, "x.𐀀", "x[\"\\uD800\\uDC00\"];\n")

	expectPrinted(t, "x?.π", "x?.π;\n")
	expectPrinted(t, "x?.𐀀", "x?.[\"𐀀\"];\n")
	expectPrintedASCII(t, "x?.π", "x?.\\u03C0;\n")
	expectPrintedASCII(t, "x?.𐀀", "x?.[\"\\u{10000}\"];\n")
	expectPrintedTargetASCII(t, 5, "x?.π", "x == null ? void 0 : x.\\u03C0;\n")
	expectPrintedTargetASCII(t, 5, "x?.𐀀", "x == null ? void 0 : x[\"\\uD800\\uDC00\"];\n")

	expectPrinted(t, "0 .π", "0 .π;\n")
	expectPrinted(t, "0 .𐀀", "0[\"𐀀\"];\n")
	expectPrintedASCII(t, "0 .π", "0 .\\u03C0;\n")
	expectPrintedASCII(t, "0 .𐀀", "0[\"\\u{10000}\"];\n")
	expectPrintedTargetASCII(t, 5, "0 .π", "0 .\\u03C0;\n")
	expectPrintedTargetASCII(t, 5, "0 .𐀀", "0[\"\\uD800\\uDC00\"];\n")

	expectPrinted(t, "0?.π", "0?.π;\n")
	expectPrinted(t, "0?.𐀀", "0?.[\"𐀀\"];\n")
	expectPrintedASCII(t, "0?.π", "0?.\\u03C0;\n")
	expectPrintedASCII(t, "0?.𐀀", "0?.[\"\\u{10000}\"];\n")
	expectPrintedTargetASCII(t, 5, "0?.π", "0 == null ? void 0 : 0 .\\u03C0;\n")
	expectPrintedTargetASCII(t, 5, "0?.𐀀", "0 == null ? void 0 : 0[\"\\uD800\\uDC00\"];\n")

	expectPrinted(t, "import 'π'", "import \"π\";\n")
	expectPrinted(t, "import '𐀀'", "import \"𐀀\";\n")
	expectPrintedASCII(t, "import 'π'", "import \"\\u03C0\";\n")
	expectPrintedASCII(t, "import '𐀀'", "import \"\\u{10000}\";\n")
	expectPrintedTargetASCII(t, 5, "import 'π'", "import \"\\u03C0\";\n")
	expectPrintedTargetASCII(t, 5, "import '𐀀'", "import \"\\uD800\\uDC00\";\n")

	expectPrinted(t, "({π: 0})", "({ π: 0 });\n")
	expectPrinted(t, "({𐀀: 0})", "({ \"𐀀\": 0 });\n")
	expectPrintedASCII(t, "({π: 0})", "({ \\u03C0: 0 });\n")
	expectPrintedASCII(t, "({𐀀: 0})", "({ \"\\u{10000}\": 0 });\n")
	expectPrintedTargetASCII(t, 5, "({π: 0})", "({ \\u03C0: 0 });\n")
	expectPrintedTargetASCII(t, 5, "({𐀀: 0})", "({ \"\\uD800\\uDC00\": 0 });\n")

	expectPrinted(t, "({π})", "({ π });\n")
	expectPrinted(t, "({𐀀})", "({ \"𐀀\": 𐀀 });\n")
	expectPrintedASCII(t, "({π})", "({ \\u03C0 });\n")
	expectPrintedASCII(t, "({𐀀})", "({ \"\\u{10000}\": \\u{10000} });\n")
	expectPrintedTargetASCII(t, 5, "({π})", "({ \\u03C0: \\u03C0 });\n")
	expectParseErrorTargetASCII(t, 5, "({𐀀})", es5)

	expectPrinted(t, "import * as π from 'path'; π", "import * as π from \"path\";\nπ;\n")
	expectPrinted(t, "import * as 𐀀 from 'path'; 𐀀", "import * as 𐀀 from \"path\";\n𐀀;\n")
	expectPrintedASCII(t, "import * as π from 'path'; π", "import * as \\u03C0 from \"path\";\n\\u03C0;\n")
	expectPrintedASCII(t, "import * as 𐀀 from 'path'; 𐀀", "import * as \\u{10000} from \"path\";\n\\u{10000};\n")
	expectPrintedTargetASCII(t, 5, "import * as π from 'path'; π", "import * as \\u03C0 from \"path\";\n\\u03C0;\n")
	expectParseErrorTargetASCII(t, 5, "import * as 𐀀 from 'path'", es5)

	expectPrinted(t, "import {π} from 'path'; π", "import { π } from \"path\";\nπ;\n")
	expectPrinted(t, "import {𐀀} from 'path'; 𐀀", "import { 𐀀 } from \"path\";\n𐀀;\n")
	expectPrintedASCII(t, "import {π} from 'path'; π", "import { \\u03C0 } from \"path\";\n\\u03C0;\n")
	expectPrintedASCII(t, "import {𐀀} from 'path'; 𐀀", "import { \\u{10000} } from \"path\";\n\\u{10000};\n")
	expectPrintedTargetASCII(t, 5, "import {π} from 'path'; π", "import { \\u03C0 } from \"path\";\n\\u03C0;\n")
	expectParseErrorTargetASCII(t, 5, "import {𐀀} from 'path'", es5)

	expectPrinted(t, "import {π as x} from 'path'", "import { π as x } from \"path\";\n")
	expectPrinted(t, "import {𐀀 as x} from 'path'", "import { 𐀀 as x } from \"path\";\n")
	expectPrintedASCII(t, "import {π as x} from 'path'", "import { \\u03C0 as x } from \"path\";\n")
	expectPrintedASCII(t, "import {𐀀 as x} from 'path'", "import { \\u{10000} as x } from \"path\";\n")
	expectPrintedTargetASCII(t, 5, "import {π as x} from 'path'", "import { \\u03C0 as x } from \"path\";\n")
	expectParseErrorTargetASCII(t, 5, "import {𐀀 as x} from 'path'", es5)

	expectPrinted(t, "import {x as π} from 'path'", "import { x as π } from \"path\";\n")
	expectPrinted(t, "import {x as 𐀀} from 'path'", "import { x as 𐀀 } from \"path\";\n")
	expectPrintedASCII(t, "import {x as π} from 'path'", "import { x as \\u03C0 } from \"path\";\n")
	expectPrintedASCII(t, "import {x as 𐀀} from 'path'", "import { x as \\u{10000} } from \"path\";\n")
	expectPrintedTargetASCII(t, 5, "import {x as π} from 'path'", "import { x as \\u03C0 } from \"path\";\n")
	expectParseErrorTargetASCII(t, 5, "import {x as 𐀀} from 'path'", es5)

	expectPrinted(t, "export * as π from 'path'; π", "export * as π from \"path\";\nπ;\n")
	expectPrinted(t, "export * as 𐀀 from 'path'; 𐀀", "export * as 𐀀 from \"path\";\n𐀀;\n")
	expectPrintedASCII(t, "export * as π from 'path'; π", "export * as \\u03C0 from \"path\";\n\\u03C0;\n")
	expectPrintedASCII(t, "export * as 𐀀 from 'path'; 𐀀", "export * as \\u{10000} from \"path\";\n\\u{10000};\n")
	expectPrintedTargetASCII(t, 5, "export * as π from 'path'", "import * as \\u03C0 from \"path\";\nexport { \\u03C0 };\n")
	expectParseErrorTargetASCII(t, 5, "export * as 𐀀 from 'path'", es5)

	expectPrinted(t, "export {π} from 'path'; π", "export { π } from \"path\";\nπ;\n")
	expectPrinted(t, "export {𐀀} from 'path'; 𐀀", "export { 𐀀 } from \"path\";\n𐀀;\n")
	expectPrintedASCII(t, "export {π} from 'path'; π", "export { \\u03C0 } from \"path\";\n\\u03C0;\n")
	expectPrintedASCII(t, "export {𐀀} from 'path'; 𐀀", "export { \\u{10000} } from \"path\";\n\\u{10000};\n")
	expectPrintedTargetASCII(t, 5, "export {π} from 'path'; π", "export { \\u03C0 } from \"path\";\n\\u03C0;\n")
	expectParseErrorTargetASCII(t, 5, "export {𐀀} from 'path'", es5)

	expectPrinted(t, "export {π as x} from 'path'", "export { π as x } from \"path\";\n")
	expectPrinted(t, "export {𐀀 as x} from 'path'", "export { 𐀀 as x } from \"path\";\n")
	expectPrintedASCII(t, "export {π as x} from 'path'", "export { \\u03C0 as x } from \"path\";\n")
	expectPrintedASCII(t, "export {𐀀 as x} from 'path'", "export { \\u{10000} as x } from \"path\";\n")
	expectPrintedTargetASCII(t, 5, "export {π as x} from 'path'", "export { \\u03C0 as x } from \"path\";\n")
	expectParseErrorTargetASCII(t, 5, "export {𐀀 as x} from 'path'", es5)

	expectPrinted(t, "export {x as π} from 'path'", "export { x as π } from \"path\";\n")
	expectPrinted(t, "export {x as 𐀀} from 'path'", "export { x as 𐀀 } from \"path\";\n")
	expectPrintedASCII(t, "export {x as π} from 'path'", "export { x as \\u03C0 } from \"path\";\n")
	expectPrintedASCII(t, "export {x as 𐀀} from 'path'", "export { x as \\u{10000} } from \"path\";\n")
	expectPrintedTargetASCII(t, 5, "export {x as π} from 'path'", "export { x as \\u03C0 } from \"path\";\n")
	expectParseErrorTargetASCII(t, 5, "export {x as 𐀀} from 'path'", es5)

	expectPrinted(t, "export {π}; var π", "export { π };\nvar π;\n")
	expectPrinted(t, "export {𐀀}; var 𐀀", "export { 𐀀 };\nvar 𐀀;\n")
	expectPrintedASCII(t, "export {π}; var π", "export { \\u03C0 };\nvar \\u03C0;\n")
	expectPrintedASCII(t, "export {𐀀}; var 𐀀", "export { \\u{10000} };\nvar \\u{10000};\n")
	expectPrintedTargetASCII(t, 5, "export {π}; var π", "export { \\u03C0 };\nvar \\u03C0;\n")
	expectParseErrorTargetASCII(t, 5, "export {𐀀}; var 𐀀", es5)

	expectPrinted(t, "export var π", "export var π;\n")
	expectPrinted(t, "export var 𐀀", "export var 𐀀;\n")
	expectPrintedASCII(t, "export var π", "export var \\u03C0;\n")
	expectPrintedASCII(t, "export var 𐀀", "export var \\u{10000};\n")
	expectPrintedTargetASCII(t, 5, "export var π", "export var \\u03C0;\n")
	expectParseErrorTargetASCII(t, 5, "export var 𐀀", es5)
}

func TestMangleCatch(t *testing.T) {
	expectPrintedMangle(t, "try { throw 0 } catch (e) { console.log(0) }", "try {\n  throw 0;\n} catch {\n  console.log(0);\n}\n")
	expectPrintedMangle(t, "try { throw 0 } catch (e) { console.log(0, e) }", "try {\n  throw 0;\n} catch (e) {\n  console.log(0, e);\n}\n")
	expectPrintedMangle(t, "try { throw 0 } catch (e) { 0 && console.log(0, e) }", "try {\n  throw 0;\n} catch {\n}\n")
	expectPrintedMangle(t, "try { thrower() } catch ([a]) { console.log(0) }", "try {\n  thrower();\n} catch ([a]) {\n  console.log(0);\n}\n")
	expectPrintedMangle(t, "try { thrower() } catch ({ a }) { console.log(0) }", "try {\n  thrower();\n} catch ({ a }) {\n  console.log(0);\n}\n")
	expectPrintedMangleTarget(t, 2018, "try { throw 0 } catch (e) { console.log(0) }", "try {\n  throw 0;\n} catch (e) {\n  console.log(0);\n}\n")

	expectPrintedMangle(t, "try { throw 1 } catch (x) { y(x); var x = 2; y(x) }", "try {\n  throw 1;\n} catch (x) {\n  y(x);\n  var x = 2;\n  y(x);\n}\n")
	expectPrintedMangle(t, "try { throw 1 } catch (x) { var x = 2; y(x) }", "try {\n  throw 1;\n} catch (x) {\n  var x = 2;\n  y(x);\n}\n")
	expectPrintedMangle(t, "try { throw 1 } catch (x) { var x = 2 }", "try {\n  throw 1;\n} catch (x) {\n  var x = 2;\n}\n")
	expectPrintedMangle(t, "try { throw 1 } catch (x) { eval('x') }", "try {\n  throw 1;\n} catch (x) {\n  eval(\"x\");\n}\n")
	expectPrintedMangle(t, "if (y) try { throw 1 } catch (x) {} else eval('x')", "if (y)\n  try {\n    throw 1;\n  } catch {\n  }\nelse\n  eval(\"x\");\n")
}

func TestAutoPureForObjectCreate(t *testing.T) {
	expectPrinted(t, "Object.create(null)", "/* @__PURE__ */ Object.create(null);\n")
	expectPrinted(t, "Object.create({})", "/* @__PURE__ */ Object.create({});\n")

	expectPrinted(t, "Object.create()", "Object.create();\n")
	expectPrinted(t, "Object.create(x)", "Object.create(x);\n")
	expectPrinted(t, "Object.create(undefined)", "Object.create(void 0);\n")
}

func TestAutoPureForSet(t *testing.T) {
	expectPrinted(t, "new Set", "/* @__PURE__ */ new Set();\n")
	expectPrinted(t, "new Set(null)", "/* @__PURE__ */ new Set(null);\n")
	expectPrinted(t, "new Set(undefined)", "/* @__PURE__ */ new Set(void 0);\n")
	expectPrinted(t, "new Set([])", "/* @__PURE__ */ new Set([]);\n")
	expectPrinted(t, "new Set([x])", "/* @__PURE__ */ new Set([x]);\n")

	expectPrinted(t, "new Set(x)", "new Set(x);\n")
	expectPrinted(t, "new Set(false)", "new Set(false);\n")
	expectPrinted(t, "new Set({})", "new Set({});\n")
	expectPrinted(t, "new Set({ x })", "new Set({ x });\n")
}

func TestAutoPureForMap(t *testing.T) {
	expectPrinted(t, "new Map", "/* @__PURE__ */ new Map();\n")
	expectPrinted(t, "new Map(null)", "/* @__PURE__ */ new Map(null);\n")
	expectPrinted(t, "new Map(undefined)", "/* @__PURE__ */ new Map(void 0);\n")
	expectPrinted(t, "new Map([])", "/* @__PURE__ */ new Map([]);\n")
	expectPrinted(t, "new Map([[]])", "/* @__PURE__ */ new Map([[]]);\n")
	expectPrinted(t, "new Map([[], []])", "/* @__PURE__ */ new Map([[], []]);\n")

	expectPrinted(t, "new Map(x)", "new Map(x);\n")
	expectPrinted(t, "new Map(false)", "new Map(false);\n")
	expectPrinted(t, "new Map([x])", "new Map([x]);\n")
	expectPrinted(t, "new Map([x, []])", "new Map([x, []]);\n")
	expectPrinted(t, "new Map([[], x])", "new Map([[], x]);\n")
}

func TestAutoPureForWeakSet(t *testing.T) {
	expectPrinted(t, "new WeakSet", "/* @__PURE__ */ new WeakSet();\n")
	expectPrinted(t, "new WeakSet(null)", "/* @__PURE__ */ new WeakSet(null);\n")
	expectPrinted(t, "new WeakSet(undefined)", "/* @__PURE__ */ new WeakSet(void 0);\n")
	expectPrinted(t, "new WeakSet([])", "/* @__PURE__ */ new WeakSet([]);\n")

	expectPrinted(t, "new WeakSet([x])", "new WeakSet([x]);\n")
	expectPrinted(t, "new WeakSet(x)", "new WeakSet(x);\n")
	expectPrinted(t, "new WeakSet(false)", "new WeakSet(false);\n")
	expectPrinted(t, "new WeakSet({})", "new WeakSet({});\n")
	expectPrinted(t, "new WeakSet({ x })", "new WeakSet({ x });\n")
}

func TestAutoPureForWeakMap(t *testing.T) {
	expectPrinted(t, "new WeakMap", "/* @__PURE__ */ new WeakMap();\n")
	expectPrinted(t, "new WeakMap(null)", "/* @__PURE__ */ new WeakMap(null);\n")
	expectPrinted(t, "new WeakMap(undefined)", "/* @__PURE__ */ new WeakMap(void 0);\n")
	expectPrinted(t, "new WeakMap([])", "/* @__PURE__ */ new WeakMap([]);\n")

	expectPrinted(t, "new WeakMap([[]])", "new WeakMap([[]]);\n")
	expectPrinted(t, "new WeakMap([[], []])", "new WeakMap([[], []]);\n")
	expectPrinted(t, "new WeakMap(x)", "new WeakMap(x);\n")
	expectPrinted(t, "new WeakMap(false)", "new WeakMap(false);\n")
	expectPrinted(t, "new WeakMap([x])", "new WeakMap([x]);\n")
	expectPrinted(t, "new WeakMap([x, []])", "new WeakMap([x, []]);\n")
	expectPrinted(t, "new WeakMap([[], x])", "new WeakMap([[], x]);\n")
}

func TestAutoPureForDate(t *testing.T) {
	expectPrinted(t, "new Date", "/* @__PURE__ */ new Date();\n")
	expectPrinted(t, "new Date(0)", "/* @__PURE__ */ new Date(0);\n")
	expectPrinted(t, "new Date('')", "/* @__PURE__ */ new Date(\"\");\n")
	expectPrinted(t, "new Date(null)", "/* @__PURE__ */ new Date(null);\n")
	expectPrinted(t, "new Date(true)", "/* @__PURE__ */ new Date(true);\n")
	expectPrinted(t, "new Date(false)", "/* @__PURE__ */ new Date(false);\n")
	expectPrinted(t, "new Date(undefined)", "/* @__PURE__ */ new Date(void 0);\n")
	expectPrinted(t, "new Date(`${foo}`)", "/* @__PURE__ */ new Date(`${foo}`);\n")
	expectPrinted(t, "new Date(foo ? 'x' : 'y')", "/* @__PURE__ */ new Date(foo ? \"x\" : \"y\");\n")

	expectPrinted(t, "new Date(foo)", "new Date(foo);\n")
	expectPrinted(t, "new Date(foo``)", "new Date(foo``);\n")
	expectPrinted(t, "new Date(foo ? x : y)", "new Date(foo ? x : y);\n")
}

// See: https://github.com/tc39/proposal-explicit-resource-management
func TestUsing(t *testing.T) {
	expectPrinted(t, "using x = y", "using x = y;\n")
	expectPrinted(t, "using x = y; z", "using x = y;\nz;\n")
	expectPrinted(t, "using x = y, z = _", "using x = y, z = _;\n")
	expectPrinted(t, "using x = y, \n z = _", "using x = y, z = _;\n")
	expectPrinted(t, "using \n x = y", "using;\nx = y;\n")
	expectPrinted(t, "using [x]", "using[x];\n")
	expectPrinted(t, "using [x] = y", "using[x] = y;\n")
	expectPrinted(t, "using \n [x] = y", "using[x] = y;\n")
	expectParseError(t, "using x", "<stdin>: ERROR: The declaration \"x\" must be initialized\n")
	expectParseError(t, "using {x}", "<stdin>: ERROR: Expected \";\" but found \"{\"\n")
	expectParseError(t, "using x = y, z", "<stdin>: ERROR: The declaration \"z\" must be initialized\n")
	expectParseError(t, "using x = y, [z] = _", "<stdin>: ERROR: Expected identifier but found \"[\"\n")
	expectParseError(t, "using x = y, {z} = _", "<stdin>: ERROR: Expected identifier but found \"{\"\n")
	expectParseError(t, "export using x = y", "<stdin>: ERROR: Unexpected \"using\"\n")

	expectPrinted(t, "for (using x = y;;) ;", "for (using x = y; ; )\n  ;\n")
	expectPrinted(t, "for (using x of y) ;", "for (using x of y)\n  ;\n")
	expectPrinted(t, "for (using of x) ;", "for (using of x)\n  ;\n")
	expectPrinted(t, "for await (using x of y) ;", "for await (using x of y)\n  ;\n")
	expectPrinted(t, "for await (using of x) ;", "for await (using of x)\n  ;\n")
	expectParseError(t, "for (using x in y) ;", "<stdin>: ERROR: \"using\" declarations are not allowed here\n")
	expectParseError(t, "for (using x;;) ;", "<stdin>: ERROR: The declaration \"x\" must be initialized\n")
	expectParseError(t, "for (using x = y of z) ;", "<stdin>: ERROR: for-of loop variables cannot have an initializer\n")
	expectParseError(t, "for (using \n x of y) ;", "<stdin>: ERROR: Expected \";\" but found \"x\"\n")
	expectParseError(t, "for await (using x = y of z) ;", "<stdin>: ERROR: for-of loop variables cannot have an initializer\n")
	expectParseError(t, "for await (using \n x of y) ;", "<stdin>: ERROR: Expected \"of\" but found \"x\"\n")

	expectPrinted(t, "await using \n x = y", "await using;\nx = y;\n")
	expectPrinted(t, "await \n using \n x \n = \n y", "await using;\nx = y;\n")
	expectPrinted(t, "await using [x]", "await using[x];\n")
	expectPrinted(t, "await using ([x] = y)", "await using([x] = y);\n")
	expectPrinted(t, "await (using [x] = y)", "await (using[x] = y);\n")
	expectParseError(t, "await using [x] = y", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "for (await using x in y) ;", "<stdin>: ERROR: \"await using\" declarations are not allowed here\n")
	expectParseError(t, "for (await using x = y;;) ;", "<stdin>: ERROR: \"await using\" declarations are not allowed here\n")
	expectParseError(t, "for (await using of x) ;", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "for (await using x = y of z) ;", "<stdin>: ERROR: for-of loop variables cannot have an initializer\n")
	expectParseError(t, "for (await using \n x of y) ;", "<stdin>: ERROR: Expected \";\" but found \"x\"\n")
	expectParseError(t, "for await (await using of x) ;", "<stdin>: ERROR: Invalid assignment target\n")
	expectParseError(t, "for await (await using x = y of z) ;", "<stdin>: ERROR: for-of loop variables cannot have an initializer\n")
	expectParseError(t, "for await (await using \n x of y) ;", "<stdin>: ERROR: Expected \"of\" but found \"x\"\n")

	expectPrinted(t, "await using x = y", "await using x = y;\n")
	expectPrinted(t, "await using x = y, z = _", "await using x = y, z = _;\n")
	expectPrinted(t, "for (await using x of y) ;", "for (await using x of y)\n  ;\n")
	expectPrinted(t, "for await (await using x of y) ;", "for await (await using x of y)\n  ;\n")

	expectPrinted(t, "function foo() { using x = y }", "function foo() {\n  using x = y;\n}\n")
	expectPrinted(t, "foo = function() { using x = y }", "foo = function() {\n  using x = y;\n};\n")
	expectPrinted(t, "foo = () => { using x = y }", "foo = () => {\n  using x = y;\n};\n")
	expectPrinted(t, "async function foo() { using x = y }", "async function foo() {\n  using x = y;\n}\n")
	expectPrinted(t, "foo = async function() { using x = y }", "foo = async function() {\n  using x = y;\n};\n")
	expectPrinted(t, "foo = async () => { using x = y }", "foo = async () => {\n  using x = y;\n};\n")
	expectPrinted(t, "async function foo() { await using x = y }", "async function foo() {\n  await using x = y;\n}\n")
	expectPrinted(t, "foo = async function() { await using x = y }", "foo = async function() {\n  await using x = y;\n};\n")
	expectPrinted(t, "foo = async () => { await using x = y }", "foo = async () => {\n  await using x = y;\n};\n")

	expectParseError(t, "export using x = y", "<stdin>: ERROR: Unexpected \"using\"\n")
	expectParseError(t, "export await using x = y", "<stdin>: ERROR: Unexpected \"await\"\n")

	needAsync := "<stdin>: ERROR: \"await\" can only be used inside an \"async\" function\n<stdin>: NOTE: Consider adding the \"async\" keyword here:\n"
	expectParseError(t, "function foo() { await using x = y }", needAsync)
	expectParseError(t, "foo = function() { await using x = y }", needAsync)
	expectParseError(t, "foo = () => { await using x = y }", needAsync)

	// Can't use await at the top-level without top-level await
	err := "<stdin>: ERROR: Top-level await is not available in the configured target environment\n"
	expectParseErrorWithUnsupportedFeatures(t, compat.TopLevelAwait, "await using x = y;", err)
	expectParseErrorWithUnsupportedFeatures(t, compat.TopLevelAwait, "for (await using x of y) ;", err)
	expectParseErrorWithUnsupportedFeatures(t, compat.TopLevelAwait, "if (true) { await using x = y }", err)
	expectParseErrorWithUnsupportedFeatures(t, compat.TopLevelAwait, "if (true) for (await using x of y) ;", err)
	expectPrintedWithUnsupportedFeatures(t, compat.TopLevelAwait, "if (false) { await using x = y }", "if (false) {\n  using x = y;\n}\n")
	expectPrintedWithUnsupportedFeatures(t, compat.TopLevelAwait, "if (false) for (await using x of y) ;", "if (false)\n  for (using x of y)\n    ;\n")
	expectParseErrorWithUnsupportedFeatures(t, compat.TopLevelAwait, "with (x) y; if (false) { await using x = y }",
		"<stdin>: ERROR: With statements cannot be used in an ECMAScript module\n"+
			"<stdin>: NOTE: This file is considered to be an ECMAScript module because of the top-level \"await\" keyword here:\n")
	expectParseErrorWithUnsupportedFeatures(t, compat.TopLevelAwait, "with (x) y; if (false) for (await using x of y) ;",
		"<stdin>: ERROR: With statements cannot be used in an ECMAScript module\n"+
			"<stdin>: NOTE: This file is considered to be an ECMAScript module because of the top-level \"await\" keyword here:\n")

	// Optimization: "using" declarations initialized to null or undefined can avoid the "using" machinery
	expectPrinted(t, "using x = {}", "using x = {};\n")
	expectPrinted(t, "using x = null", "using x = null;\n")
	expectPrinted(t, "using x = undefined", "using x = void 0;\n")
	expectPrinted(t, "using x = (foo, y)", "using x = (foo, y);\n")
	expectPrinted(t, "using x = (foo, null)", "using x = (foo, null);\n")
	expectPrinted(t, "using x = (foo, undefined)", "using x = (foo, void 0);\n")
	expectPrintedMangle(t, "using x = {}", "using x = {};\n")
	expectPrintedMangle(t, "using x = null", "const x = null;\n")
	expectPrintedMangle(t, "using x = undefined", "const x = void 0;\n")
	expectPrintedMangle(t, "using x = (foo, y)", "using x = (foo, y);\n")
	expectPrintedMangle(t, "using x = (foo, null)", "const x = (foo, null);\n")
	expectPrintedMangle(t, "using x = (foo, undefined)", "const x = (foo, void 0);\n")
	expectPrintedMangle(t, "using x = null, y = undefined", "const x = null, y = void 0;\n")
	expectPrintedMangle(t, "using x = null, y = z", "using x = null, y = z;\n")
	expectPrintedMangle(t, "using x = z, y = undefined", "using x = z, y = void 0;\n")
}
