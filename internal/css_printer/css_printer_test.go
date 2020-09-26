package css_printer

import (
	"testing"

	"github.com/evanw/esbuild/internal/css_parser"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

func assertEqual(t *testing.T, a interface{}, b interface{}) {
	t.Helper()
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
}

func expectPrintedCommon(t *testing.T, name string, contents string, expected string, options Options) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog()
		tree := css_parser.Parse(log, test.SourceForTest(contents))
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logger.StderrOptions{}, logger.TerminalInfo{})
		}
		assertEqual(t, text, "")
		options.Contents = contents
		css := Print(tree, options)
		assertEqual(t, string(css), expected)
	})
}

func expectPrinted(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, contents, expected, Options{})
}

func expectPrintedMinify(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [minified]", contents, expected, Options{
		RemoveWhitespace: true,
	})
}

func TestImportant(t *testing.T) {
	expectPrinted(t, "a { b: c!important }", "a {\n  b: c !important;\n}\n")
	expectPrinted(t, "a { b: c!important; }", "a {\n  b: c !important;\n}\n")
	expectPrinted(t, "a { b: c! important }", "a {\n  b: c !important;\n}\n")
	expectPrinted(t, "a { b: c! important; }", "a {\n  b: c !important;\n}\n")
	expectPrinted(t, "a { b: c ! important }", "a {\n  b: c !important;\n}\n")
	expectPrinted(t, "a { b: c ! important; }", "a {\n  b: c !important;\n}\n")
	expectPrinted(t, "a { b: c !IMPORTANT; }", "a {\n  b: c !important;\n}\n")
	expectPrinted(t, "a { b: c !ImPoRtAnT; }", "a {\n  b: c !important;\n}\n")
	expectPrintedMinify(t, "a { b: c !important }", "a{b:c!important}")
}

func TestSelector(t *testing.T) {
	expectPrintedMinify(t, "a + b c > d ~ e{}", "a+b c>d~e{}")
}

func TestNestedSelector(t *testing.T) {
	expectPrintedMinify(t, "a { &b {} }", "a{&b{}}")
	expectPrintedMinify(t, "a { & b {} }", "a{& b{}}")
	expectPrintedMinify(t, "a { & :b {} }", "a{& :b{}}")
}

func TestAtCharset(t *testing.T) {
	expectPrinted(t, "@charset \"UTF-8\";", "@charset \"UTF-8\";\n")
	expectPrintedMinify(t, "@charset \"UTF-8\";", "@charset \"UTF-8\";")
}

func TestAtNamespace(t *testing.T) {
	expectPrinted(t, "@namespace\"http://www.com\";", "@namespace \"http://www.com\";\n")
	expectPrinted(t, "@namespace \"http://www.com\";", "@namespace \"http://www.com\";\n")
	expectPrinted(t, "@namespace url(http://www.com);", "@namespace \"http://www.com\";\n")
	expectPrinted(t, "@namespace url(\"http://www.com\");", "@namespace \"http://www.com\";\n")
	expectPrinted(t, "@namespace ns\"http://www.com\";", "@namespace ns \"http://www.com\";\n")
	expectPrinted(t, "@namespace ns \"http://www.com\";", "@namespace ns \"http://www.com\";\n")
	expectPrinted(t, "@namespace ns url(http://www.com);", "@namespace ns \"http://www.com\";\n")
	expectPrinted(t, "@namespace ns url(\"http://www.com\");", "@namespace ns \"http://www.com\";\n")

	expectPrintedMinify(t, "@namespace\"http://www.com\";", "@namespace\"http://www.com\";")
	expectPrintedMinify(t, "@namespace \"http://www.com\";", "@namespace\"http://www.com\";")
	expectPrintedMinify(t, "@namespace url(http://www.com);", "@namespace\"http://www.com\";")
	expectPrintedMinify(t, "@namespace url(\"http://www.com\");", "@namespace\"http://www.com\";")
	expectPrintedMinify(t, "@namespace ns\"http://www.com\";", "@namespace ns\"http://www.com\";")
	expectPrintedMinify(t, "@namespace ns \"http://www.com\";", "@namespace ns\"http://www.com\";")
	expectPrintedMinify(t, "@namespace ns url(http://www.com);", "@namespace ns\"http://www.com\";")
	expectPrintedMinify(t, "@namespace ns url(\"http://www.com\");", "@namespace ns\"http://www.com\";")
}

func TestAtImport(t *testing.T) {
	expectPrinted(t, "@import\"foo.css\";", "@import \"foo.css\";\n")
	expectPrinted(t, "@import \"foo.css\";", "@import \"foo.css\";\n")
	expectPrinted(t, "@import url(foo.css);", "@import \"foo.css\";\n")
	expectPrinted(t, "@import url(\"foo.css\");", "@import \"foo.css\";\n")

	expectPrintedMinify(t, "@import\"foo.css\";", "@import\"foo.css\";")
	expectPrintedMinify(t, "@import \"foo.css\";", "@import\"foo.css\";")
	expectPrintedMinify(t, "@import url(foo.css);", "@import\"foo.css\";")
	expectPrintedMinify(t, "@import url(\"foo.css\");", "@import\"foo.css\";")
}
