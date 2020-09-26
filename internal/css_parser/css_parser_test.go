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

	expectPrinted(t, "[b = \"c\" i] {}", "[b=\"c\" i] {\n}\n")
	expectPrinted(t, "[b = \"c\" I] {}", "[b=\"c\" I] {\n}\n")
	expectParseError(t, "[b i] {}", "<stdin>: error: Expected \"]\" but found \"i\"\n<stdin>: error: Unexpected \"]\"\n")
	expectParseError(t, "[b I] {}", "<stdin>: error: Expected \"]\" but found \"I\"\n<stdin>: error: Unexpected \"]\"\n")

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
	expectPrinted(t, "& {}", "& {\n}\n")
	expectPrinted(t, "& b {}", "& b {\n}\n")
	expectPrinted(t, "&:b {}", "&:b {\n}\n")
	expectPrinted(t, "&* {}", "&* {\n}\n")
	expectPrinted(t, "&|b {}", "&|b {\n}\n")
	expectPrinted(t, "&*|b {}", "&*|b {\n}\n")
	expectPrinted(t, "&a|b {}", "&a|b {\n}\n")
	expectPrinted(t, "&[a] {}", "&[a] {\n}\n")

	expectPrinted(t, "a { & {} }", "a {\n  & {\n  }\n}\n")
	expectPrinted(t, "a { & b {} }", "a {\n  & b {\n  }\n}\n")
	expectPrinted(t, "a { &:b {} }", "a {\n  &:b {\n  }\n}\n")
	expectPrinted(t, "a { &* {} }", "a {\n  &* {\n  }\n}\n")
	expectPrinted(t, "a { &|b {} }", "a {\n  &|b {\n  }\n}\n")
	expectPrinted(t, "a { &*|b {} }", "a {\n  &*|b {\n  }\n}\n")
	expectPrinted(t, "a { &a|b {} }", "a {\n  &a|b {\n  }\n}\n")
	expectPrinted(t, "a { &[b] {} }", "a {\n  &[b] {\n  }\n}\n")
}

func TestAtCharset(t *testing.T) {
	expectPrinted(t, "@charset \"UTF-8\";", "@charset \"UTF-8\";\n")
	expectPrinted(t, "@charset 'UTF-8';", "@charset \"UTF-8\";\n")

	expectParseError(t, "@charset;", "<stdin>: error: Expected whitespace but found \";\"\n")
	expectParseError(t, "@charset ;", "<stdin>: error: Expected string token but found \";\"\n")
	expectParseError(t, "@charset\"UTF-8\";", "<stdin>: error: Expected whitespace but found \"\\\"UTF-8\\\"\"\n")
	expectParseError(t, "@charset \"UTF-8\"", "<stdin>: error: Expected \";\" but found end of file\n")
	expectParseError(t, "@charset url(UTF-8);", "<stdin>: error: Expected string token but found \"url(UTF-8)\"\n")
	expectParseError(t, "@charset url(\"UTF-8\");", "<stdin>: error: Expected string token but found \"url(\"\n")
	expectParseError(t, "@charset \"UTF-8\" ", "<stdin>: error: Expected \";\" but found \" \"\n")
	expectParseError(t, "@charset \"UTF-8\"{}", "<stdin>: error: Expected \";\" but found \"{\"\n")
}

func TestAtNamespace(t *testing.T) {
	expectPrinted(t, "@namespace\"http://www.com\";", "@namespace \"http://www.com\";\n")
	expectPrinted(t, "@namespace \"http://www.com\";", "@namespace \"http://www.com\";\n")
	expectPrinted(t, "@namespace \"http://www.com\" ;", "@namespace \"http://www.com\";\n")
	expectPrinted(t, "@namespace url();", "@namespace \"\";\n")
	expectPrinted(t, "@namespace url(http://www.com);", "@namespace \"http://www.com\";\n")
	expectPrinted(t, "@namespace url(http://www.com) ;", "@namespace \"http://www.com\";\n")
	expectPrinted(t, "@namespace url(\"http://www.com\");", "@namespace \"http://www.com\";\n")
	expectPrinted(t, "@namespace url(\"http://www.com\") ;", "@namespace \"http://www.com\";\n")

	expectPrinted(t, "@namespace ns\"http://www.com\";", "@namespace ns \"http://www.com\";\n")
	expectPrinted(t, "@namespace ns \"http://www.com\";", "@namespace ns \"http://www.com\";\n")
	expectPrinted(t, "@namespace ns \"http://www.com\" ;", "@namespace ns \"http://www.com\";\n")
	expectPrinted(t, "@namespace ns url();", "@namespace ns \"\";\n")
	expectPrinted(t, "@namespace ns url(http://www.com);", "@namespace ns \"http://www.com\";\n")
	expectPrinted(t, "@namespace ns url(http://www.com) ;", "@namespace ns \"http://www.com\";\n")
	expectPrinted(t, "@namespace ns url(\"http://www.com\");", "@namespace ns \"http://www.com\";\n")
	expectPrinted(t, "@namespace ns url(\"http://www.com\") ;", "@namespace ns \"http://www.com\";\n")

	expectParseError(t, "@namespace;", "<stdin>: error: Expected URL token but found \";\"\n")
	expectParseError(t, "@namespace \"http://www.com\"", "<stdin>: error: Expected \";\" but found end of file\n")
	expectParseError(t, "@namespace url(\"http://www.com\";", "<stdin>: error: Expected \")\" but found \";\"\n")
	expectParseError(t, "@namespace noturl(\"http://www.com\");", "<stdin>: error: Expected URL token but found \"noturl(\"\n")

	expectParseError(t, "@namespace ns;", "<stdin>: error: Expected URL token but found \";\"\n")
	expectParseError(t, "@namespace ns \"http://www.com\"", "<stdin>: error: Expected \";\" but found end of file\n")
	expectParseError(t, "@namespace ns url(\"http://www.com\";", "<stdin>: error: Expected \")\" but found \";\"\n")
	expectParseError(t, "@namespace ns noturl(\"http://www.com\");", "<stdin>: error: Expected URL token but found \"noturl(\"\n")

	expectParseError(t, "@namespace \"http://www.com\" {}", "<stdin>: error: Expected \";\" but found \"{\"\n")
}

func TestAtImport(t *testing.T) {
	expectPrinted(t, "@import\"foo.css\";", "@import \"foo.css\";\n")
	expectPrinted(t, "@import \"foo.css\";", "@import \"foo.css\";\n")
	expectPrinted(t, "@import \"foo.css\" ;", "@import \"foo.css\";\n")
	expectPrinted(t, "@import url();", "@import \"\";\n")
	expectPrinted(t, "@import url(foo.css);", "@import \"foo.css\";\n")
	expectPrinted(t, "@import url(foo.css) ;", "@import \"foo.css\";\n")
	expectPrinted(t, "@import url(\"foo.css\");", "@import \"foo.css\";\n")
	expectPrinted(t, "@import url(\"foo.css\") ;", "@import \"foo.css\";\n")

	expectParseError(t, "@import;", "<stdin>: error: Expected URL token but found \";\"\n")
	expectParseError(t, "@import ;", "<stdin>: error: Expected URL token but found \";\"\n")
	expectParseError(t, "@import \"foo.css\"", "<stdin>: error: Expected \";\" but found end of file\n")
	expectParseError(t, "@import url(\"foo.css\";", "<stdin>: error: Expected \")\" but found \";\"\n")
	expectParseError(t, "@import noturl(\"foo.css\");", "<stdin>: error: Expected URL token but found \"noturl(\"\n")

	expectParseError(t, "@import \"foo.css\" {}", "<stdin>: error: Expected \";\" but found \"{\"\n")
}
