package css_parser

import (
	"testing"

	"github.com/evanw/esbuild/internal/css_printer"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

func assertEqual(t *testing.T, a interface{}, b interface{}) {
	t.Helper()
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
}

func expectParseError(t *testing.T, contents string, expected string) {
	t.Helper()
	t.Run(contents, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog()
		Parse(log, test.SourceForTest(contents))
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logger.StderrOptions{}, logger.TerminalInfo{})
		}
		test.AssertEqual(t, text, expected)
	})
}

func expectPrinted(t *testing.T, contents string, expected string) {
	t.Helper()
	t.Run(contents, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog()
		tree := Parse(log, test.SourceForTest(contents))
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logger.StderrOptions{}, logger.TerminalInfo{})
		}
		assertEqual(t, text, "")
		css := css_printer.Print(tree, css_printer.Options{
			Contents: contents,
		})
		assertEqual(t, string(css), expected)
	})
}

func TestDeclaration(t *testing.T) {
	expectPrinted(t, ".decl {}", ".decl {\n}\n")
	expectPrinted(t, ".decl { a: b }", ".decl {\n  a: b;\n}\n")
	expectPrinted(t, ".decl { a: b; }", ".decl {\n  a: b;\n}\n")
	expectPrinted(t, ".decl { a: b; c: d }", ".decl {\n  a: b;\n  c: d;\n}\n")
	expectPrinted(t, ".decl { a: b; c: d; }", ".decl {\n  a: b;\n  c: d;\n}\n")
	expectParseError(t, ".decl { a { b: c; } }", "<stdin>: error: Expected \":\" but found \"{\"\n")
	expectPrinted(t, ".decl { & a { b: c; } }", ".decl {\n  & a {\n    b: c;\n  }\n}\n")
}

func TestSelector(t *testing.T) {
	expectPrinted(t, "a{}", "a {\n}\n")
	expectPrinted(t, "a {}", "a {\n}\n")
	expectPrinted(t, "a b {}", "a b {\n}\n")

	expectPrinted(t, "[b]{}", "[b] {\n}\n")
	expectPrinted(t, "[b] {}", "[b] {\n}\n")
	expectPrinted(t, "a[b] {}", "a[b] {\n}\n")
	expectPrinted(t, "a [b] {}", "a [b] {\n}\n")
	expectParseError(t, "[] {}", "<stdin>: error: Expected identifier but found \"]\"\n")
	expectParseError(t, "[b {}", "<stdin>: error: Expected \"]\" but found \"{\"\n")
	expectParseError(t, "[b]] {}", "<stdin>: error: Unexpected \"]\"\n")
	expectParseError(t, "a[b {}", "<stdin>: error: Expected \"]\" but found \"{\"\n")
	expectParseError(t, "a[b]] {}", "<stdin>: error: Unexpected \"]\"\n")

	expectPrinted(t, "[|b]{}", "[b] {\n}\n") // "[|b]" is equivalent to "[b]"
	expectPrinted(t, "[*|b]{}", "[*|b] {\n}\n")
	expectPrinted(t, "[a|b]{}", "[a|b] {\n}\n")
	expectPrinted(t, "[a|b|=\"c\"]{}", "[a|b|=\"c\"] {\n}\n")
	expectPrinted(t, "[a|b |= \"c\"]{}", "[a|b|=\"c\"] {\n}\n")
	expectParseError(t, "[a||b] {}", "<stdin>: error: Expected identifier but found \"|\"\n")
	expectParseError(t, "[* | b] {}", "<stdin>: error: Expected \"|\" but found \" \"\n")
	expectParseError(t, "[a | b] {}", "<stdin>: error: Expected \"=\" but found \" \"\n")

	expectPrinted(t, "[b=\"c\"] {}", "[b=\"c\"] {\n}\n")
	expectPrinted(t, "[b~=\"c\"] {}", "[b~=\"c\"] {\n}\n")
	expectPrinted(t, "[b^=\"c\"] {}", "[b^=\"c\"] {\n}\n")
	expectPrinted(t, "[b$=\"c\"] {}", "[b$=\"c\"] {\n}\n")
	expectPrinted(t, "[b*=\"c\"] {}", "[b*=\"c\"] {\n}\n")
	expectPrinted(t, "[b|=\"c\"] {}", "[b|=\"c\"] {\n}\n")
	expectParseError(t, "[b?=\"c\"] {}", "<stdin>: error: Expected \"]\" but found \"?\"\n")

	expectPrinted(t, "[b = \"c\"] {}", "[b=\"c\"] {\n}\n")
	expectPrinted(t, "[b ~= \"c\"] {}", "[b~=\"c\"] {\n}\n")
	expectPrinted(t, "[b ^= \"c\"] {}", "[b^=\"c\"] {\n}\n")
	expectPrinted(t, "[b $= \"c\"] {}", "[b$=\"c\"] {\n}\n")
	expectPrinted(t, "[b *= \"c\"] {}", "[b*=\"c\"] {\n}\n")
	expectPrinted(t, "[b |= \"c\"] {}", "[b|=\"c\"] {\n}\n")
	expectParseError(t, "[b ?= \"c\"] {}", "<stdin>: error: Expected \"]\" but found \"?\"\n")

	expectPrinted(t, "|b {}", "|b {\n}\n")
	expectPrinted(t, "|* {}", "|* {\n}\n")
	expectPrinted(t, "a|b {}", "a|b {\n}\n")
	expectPrinted(t, "a|* {}", "a|* {\n}\n")
	expectPrinted(t, "*|b {}", "*|b {\n}\n")
	expectPrinted(t, "*|* {}", "*|* {\n}\n")
	expectParseError(t, "a||b {}", "<stdin>: error: Expected identifier but found \"|\"\n")

	expectPrinted(t, "a+b {}", "a + b {\n}\n")
	expectPrinted(t, "a>b {}", "a > b {\n}\n")
	expectPrinted(t, "a+b {}", "a + b {\n}\n")
	expectPrinted(t, "a~b {}", "a ~ b {\n}\n")

	expectPrinted(t, "a + b {}", "a + b {\n}\n")
	expectPrinted(t, "a > b {}", "a > b {\n}\n")
	expectPrinted(t, "a + b {}", "a + b {\n}\n")
	expectPrinted(t, "a ~ b {}", "a ~ b {\n}\n")

	expectPrinted(t, "::b {}", "::b {\n}\n")
	expectPrinted(t, "*::b {}", "*::b {\n}\n")
	expectPrinted(t, "a::b {}", "a::b {\n}\n")
	expectPrinted(t, "::b(c) {}", "::b(c) {\n}\n")
	expectPrinted(t, "*::b(c) {}", "*::b(c) {\n}\n")
	expectPrinted(t, "a::b(c) {}", "a::b(c) {\n}\n")
	expectPrinted(t, "a:b:c {}", "a:b:c {\n}\n")
	expectPrinted(t, "a:b(:c) {}", "a:b(:c) {\n}\n")
}

func TestNestedSelector(t *testing.T) {
	expectPrinted(t, "a { & b {} }", "a {\n  & b {\n  }\n}\n")
}
