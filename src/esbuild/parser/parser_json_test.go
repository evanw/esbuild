package parser

import (
	"esbuild/ast"
	"esbuild/logging"
	"esbuild/printer"
	"fmt"
	"testing"
)

func expectParseErrorJSON(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log, join := logging.NewDeferLog()
		ParseJSON(log, logging.Source{
			Index:        0,
			AbsolutePath: "<stdin>",
			PrettyPath:   "<stdin>",
			Contents:     contents,
		})
		msgs := join()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
		}
		assertEqual(t, text, expected)
	})
}

// Note: The input is parsed as JSON but printed as JS. This means the printed
// code may not be valid JSON. That's ok because esbuild always outputs JS
// bundles, not JSON bundles.
func expectPrintedJSON(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log, join := logging.NewDeferLog()
		expr, ok := ParseJSON(log, logging.Source{
			Index:        0,
			AbsolutePath: "<stdin>",
			PrettyPath:   "<stdin>",
			Contents:     contents,
		})
		msgs := join()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
		}
		assertEqual(t, text, "")
		if !ok {
			t.Fatal("Parse error")
		}
		js := printer.PrintExpr(expr, nil, printer.PrintOptions{
			RemoveWhitespace: true,
			RequireRef:       ast.InvalidRef,
			ImportRef:        ast.InvalidRef,
		}).JS
		assertEqual(t, string(js), expected)
	})
}

func expectPrintedJSONWithWarning(t *testing.T, contents string, warning string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log, join := logging.NewDeferLog()
		expr, ok := ParseJSON(log, logging.Source{
			Index:        0,
			AbsolutePath: "<stdin>",
			PrettyPath:   "<stdin>",
			Contents:     contents,
		})
		msgs := join()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
		}
		assertEqual(t, text, warning)
		if !ok {
			t.Fatal("Parse error")
		}
		js := printer.PrintExpr(expr, nil, printer.PrintOptions{
			RemoveWhitespace: true,
			RequireRef:       ast.InvalidRef,
			ImportRef:        ast.InvalidRef,
		}).JS
		assertEqual(t, string(js), expected)
	})
}

func TestJSONAtom(t *testing.T) {
	expectPrintedJSON(t, "false", "!1")
	expectPrintedJSON(t, "true", "!0")
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
	expectPrintedJSON(t, "\"\\n\"", "`\n`")
	expectPrintedJSON(t, "\"\\r\"", "\"\\r\"")
	expectPrintedJSON(t, "\"\\t\"", "\"\t\"")
	expectPrintedJSON(t, "\"\\u0000\"", "\"\\0\"")
	expectPrintedJSON(t, "\"\\u0078\"", "\"x\"")
	expectPrintedJSON(t, "\"\\u1234\"", "\"\u1234\"")
	expectPrintedJSON(t, "\"\\uD800\"", "\"\\uD800\"")
	expectPrintedJSON(t, "\"\\uDC00\"", "\"\\uDC00\"")

	// Invalid escapes
	expectParseErrorJSON(t, "\"\\", "<stdin>: error: Unexpected end of file\n")
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
	expectPrintedJSON(t, "-.123", "-0.123")
	expectPrintedJSON(t, "123e20", "1.23e+22")
	expectPrintedJSON(t, "123e-20", "1.23e-18")
	expectParseErrorJSON(t, "NaN", "<stdin>: error: Unexpected \"NaN\"\n")
	expectParseErrorJSON(t, "Infinity", "<stdin>: error: Unexpected \"Infinity\"\n")
	expectParseErrorJSON(t, "-Infinity", "<stdin>: error: Expected number but found \"Infinity\"\n")
}

func TestJSONObject(t *testing.T) {
	expectPrintedJSON(t, "{\"x\":0}", "{x:0}")
	expectPrintedJSON(t, "{\"x\":0,\"y\":1}", "{x:0,y:1}")
	expectPrintedJSONWithWarning(t, "{\"x\":0,\"x\":1}", "<stdin>: warning: Duplicate key: \"x\"\n", "{x:0,x:1}")
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
