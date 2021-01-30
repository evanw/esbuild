package js_parser

import (
	"fmt"
	"testing"

	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_printer"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

func expectParseErrorJSON(t *testing.T, contents string, expected string) {
	t.Helper()
	t.Run(contents, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog()
		ParseJSON(log, test.SourceForTest(contents), JSONOptions{})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logger.OutputOptions{}, logger.TerminalInfo{})
		}
		test.AssertEqual(t, text, expected)
	})
}

// Note: The input is parsed as JSON but printed as JS. This means the printed
// code may not be valid JSON. That's ok because esbuild always outputs JS
// bundles, not JSON bundles.
func expectPrintedJSON(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedJSONWithWarning(t, contents, "", expected)
}

func expectPrintedJSONWithWarning(t *testing.T, contents string, warning string, expected string) {
	t.Helper()
	t.Run(contents, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog()
		expr, ok := ParseJSON(log, test.SourceForTest(contents), JSONOptions{})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logger.OutputOptions{}, logger.TerminalInfo{})
		}
		test.AssertEqual(t, text, warning)
		if !ok {
			t.Fatal("Parse error")
		}

		// Insert this expression into a statement
		tree := js_ast.AST{
			Parts: []js_ast.Part{{Stmts: []js_ast.Stmt{{Data: &js_ast.SExpr{Value: expr}}}}},
		}

		js := js_printer.Print(tree, js_ast.SymbolMap{}, nil, js_printer.Options{
			RemoveWhitespace: true,
		}).JS

		// Remove the trailing semicolon
		if n := len(js); n > 1 && js[n-1] == ';' {
			js = js[:n-1]
		}

		test.AssertEqual(t, string(js), expected)
	})
}

func TestJSONAtom(t *testing.T) {
	expectPrintedJSON(t, "false", "false")
	expectPrintedJSON(t, "true", "true")
	expectPrintedJSON(t, "null", "null")
	expectParseErrorJSON(t, "undefined", "<stdin>: error: Unexpected \"undefined\"\n")
}

func TestJSONString(t *testing.T) {
	expectPrintedJSON(t, "\"x\"", "\"x\"")
	expectParseErrorJSON(t, "'x'", "<stdin>: error: JSON strings must use double quotes\n")
	expectParseErrorJSON(t, "`x`", "<stdin>: error: Unexpected \"`x`\"\n")

	// Newlines
	expectPrintedJSON(t, "\"\u2028\"", "\"\\u2028\"")
	expectPrintedJSON(t, "\"\u2029\"", "\"\\u2029\"")
	expectParseErrorJSON(t, "\"\r\"", "<stdin>: error: Unterminated string literal\n")
	expectParseErrorJSON(t, "\"\n\"", "<stdin>: error: Unterminated string literal\n")

	// Control characters
	for c := 0; c < 0x20; c++ {
		if c != '\r' && c != '\n' {
			expectParseErrorJSON(t, fmt.Sprintf("\"%c\"", c),
				fmt.Sprintf("<stdin>: error: Syntax error \"\\x%02X\"\n", c))
		}
	}

	// Valid escapes
	expectPrintedJSON(t, "\"\\\"\"", "'\"'")
	expectPrintedJSON(t, "\"\\\\\"", "\"\\\\\"")
	expectPrintedJSON(t, "\"\\/\"", "\"/\"")
	expectPrintedJSON(t, "\"\\b\"", "\"\\b\"")
	expectPrintedJSON(t, "\"\\f\"", "\"\\f\"")
	expectPrintedJSON(t, "\"\\n\"", "\"\\n\"")
	expectPrintedJSON(t, "\"\\r\"", "\"\\r\"")
	expectPrintedJSON(t, "\"\\t\"", "\"\t\"")
	expectPrintedJSON(t, "\"\\u0000\"", "\"\\0\"")
	expectPrintedJSON(t, "\"\\u0078\"", "\"x\"")
	expectPrintedJSON(t, "\"\\u1234\"", "\"\u1234\"")
	expectPrintedJSON(t, "\"\\uD800\"", "\"\\uD800\"")
	expectPrintedJSON(t, "\"\\uDC00\"", "\"\\uDC00\"")

	// Invalid escapes
	expectParseErrorJSON(t, "\"\\", "<stdin>: error: Unterminated string literal\n")
	expectParseErrorJSON(t, "\"\\0\"", "<stdin>: error: Syntax error \"0\"\n")
	expectParseErrorJSON(t, "\"\\1\"", "<stdin>: error: Syntax error \"1\"\n")
	expectParseErrorJSON(t, "\"\\'\"", "<stdin>: error: Syntax error \"'\"\n")
	expectParseErrorJSON(t, "\"\\a\"", "<stdin>: error: Syntax error \"a\"\n")
	expectParseErrorJSON(t, "\"\\v\"", "<stdin>: error: Syntax error \"v\"\n")
	expectParseErrorJSON(t, "\"\\\n\"", "<stdin>: error: Syntax error \"\\x0A\"\n")
	expectParseErrorJSON(t, "\"\\x78\"", "<stdin>: error: Syntax error \"x\"\n")
	expectParseErrorJSON(t, "\"\\u{1234}\"", "<stdin>: error: Syntax error \"{\"\n")
	expectParseErrorJSON(t, "\"\\uG\"", "<stdin>: error: Syntax error \"G\"\n")
	expectParseErrorJSON(t, "\"\\uDG\"", "<stdin>: error: Syntax error \"G\"\n")
	expectParseErrorJSON(t, "\"\\uDEG\"", "<stdin>: error: Syntax error \"G\"\n")
	expectParseErrorJSON(t, "\"\\uDEFG\"", "<stdin>: error: Syntax error \"G\"\n")
	expectParseErrorJSON(t, "\"\\u\"", "<stdin>: error: Syntax error '\"'\n")
	expectParseErrorJSON(t, "\"\\uD\"", "<stdin>: error: Syntax error '\"'\n")
	expectParseErrorJSON(t, "\"\\uDE\"", "<stdin>: error: Syntax error '\"'\n")
	expectParseErrorJSON(t, "\"\\uDEF\"", "<stdin>: error: Syntax error '\"'\n")
}

func TestJSONNumber(t *testing.T) {
	expectPrintedJSON(t, "0", "0")
	expectPrintedJSON(t, "-0", "-0")
	expectPrintedJSON(t, "123", "123")
	expectPrintedJSON(t, "123.456", "123.456")
	expectPrintedJSON(t, ".123", ".123")
	expectPrintedJSON(t, "-.123", "-.123")
	expectPrintedJSON(t, "123e20", "123e20")
	expectPrintedJSON(t, "123e-20", "123e-20")
	expectParseErrorJSON(t, "NaN", "<stdin>: error: Unexpected \"NaN\"\n")
	expectParseErrorJSON(t, "Infinity", "<stdin>: error: Unexpected \"Infinity\"\n")
	expectParseErrorJSON(t, "-Infinity", "<stdin>: error: Expected number but found \"Infinity\"\n")
}

func TestJSONObject(t *testing.T) {
	expectPrintedJSON(t, "{\"x\":0}", "({x:0})")
	expectPrintedJSON(t, "{\"x\":0,\"y\":1}", "({x:0,y:1})")
	expectPrintedJSONWithWarning(t, "{\"x\":0,\"x\":1}", "<stdin>: warning: Duplicate key \"x\" in object literal\n", "({x:0,x:1})")
	expectParseErrorJSON(t, "{\"x\":0,}", "<stdin>: error: JSON does not support trailing commas\n")
	expectParseErrorJSON(t, "{x:0}", "<stdin>: error: Expected string but found \"x\"\n")
	expectParseErrorJSON(t, "{1:0}", "<stdin>: error: Expected string but found \"1\"\n")
	expectParseErrorJSON(t, "{[\"x\"]:0}", "<stdin>: error: Expected string but found \"[\"\n")
}

func TestJSONArray(t *testing.T) {
	expectPrintedJSON(t, "[]", "[]")
	expectPrintedJSON(t, "[1]", "[1]")
	expectPrintedJSON(t, "[1,2]", "[1,2]")
	expectParseErrorJSON(t, "[,]", "<stdin>: error: Unexpected \",\"\n")
	expectParseErrorJSON(t, "[,1]", "<stdin>: error: Unexpected \",\"\n")
	expectParseErrorJSON(t, "[1,]", "<stdin>: error: JSON does not support trailing commas\n")
	expectParseErrorJSON(t, "[1,,2]", "<stdin>: error: Unexpected \",\"\n")
}

func TestJSONInvalid(t *testing.T) {
	expectParseErrorJSON(t, "({\"x\":0})", "<stdin>: error: Unexpected \"(\"\n")
	expectParseErrorJSON(t, "{\"x\":(0)}", "<stdin>: error: Unexpected \"(\"\n")
	expectParseErrorJSON(t, "#!/usr/bin/env node\n{}", "<stdin>: error: Unexpected \"#!/usr/bin/env node\"\n")
	expectParseErrorJSON(t, "{\"x\":0}{\"y\":1}", "<stdin>: error: Expected end of file but found \"{\"\n")
}

func TestJSONComments(t *testing.T) {
	expectParseErrorJSON(t, "/*comment*/{}", "<stdin>: error: JSON does not support comments\n")
	expectParseErrorJSON(t, "//comment\n{}", "<stdin>: error: JSON does not support comments\n")
	expectParseErrorJSON(t, "{/*comment*/}", "<stdin>: error: JSON does not support comments\n")
	expectParseErrorJSON(t, "{//comment\n}", "<stdin>: error: JSON does not support comments\n")
	expectParseErrorJSON(t, "{}/*comment*/", "<stdin>: error: JSON does not support comments\n")
	expectParseErrorJSON(t, "{}//comment\n", "<stdin>: error: JSON does not support comments\n")
}
