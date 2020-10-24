package css_printer

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
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
		tree := css_parser.Parse(log, test.SourceForTest(contents), config.Options{})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			if msg.Kind == logger.Error {
				text += msg.String(logger.StderrOptions{}, logger.TerminalInfo{})
			}
		}
		assertEqual(t, text, "")
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

func expectPrintedASCII(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [ascii]", contents, expected, Options{
		ASCIIOnly: true,
	})
}

func expectPrintedString(t *testing.T, stringValue string, expected string) {
	t.Helper()
	t.Run(stringValue, func(t *testing.T) {
		t.Helper()
		p := printer{}
		p.printQuoted(stringValue)
		assertEqual(t, p.sb.String(), expected)
	})
}

func TestStringQuote(t *testing.T) {
	expectPrintedString(t, "", "\"\"")
	expectPrintedString(t, "foo", "\"foo\"")
	expectPrintedString(t, "f\"o", "'f\"o'")
	expectPrintedString(t, "f'\"'o", "\"f'\\\"'o\"")
	expectPrintedString(t, "f\"'\"o", "'f\"\\'\"o'")
	expectPrintedString(t, "f\\o", "\"f\\\\o\"")
	expectPrintedString(t, "f\ro", "\"f\\do\"")
	expectPrintedString(t, "f\no", "\"f\\ao\"")
	expectPrintedString(t, "f\fo", "\"f\\co\"")
	expectPrintedString(t, "f\r\no", "\"f\\d\\ao\"")
	expectPrintedString(t, "f\r0", "\"f\\d 0\"")
	expectPrintedString(t, "f\n0", "\"f\\a 0\"")
	expectPrintedString(t, "f\n ", "\"f\\a  \"")
	expectPrintedString(t, "f\n\t", "\"f\\a \t\"")
	expectPrintedString(t, "f\nf", "\"f\\a f\"")
	expectPrintedString(t, "f\nF", "\"f\\a F\"")
	expectPrintedString(t, "f\ng", "\"f\\ag\"")
	expectPrintedString(t, "f\nG", "\"f\\aG\"")
	expectPrintedString(t, "f\x00o", "\"f\\0o\"")
	expectPrintedString(t, "f\x01o", "\"f\x01o\"")
	expectPrintedString(t, "f\to", "\"f\to\"")
}

func TestURLQuote(t *testing.T) {
	expectPrinted(t, "* { background: url('foo') }", "* {\n  background: url(foo);\n}\n")
	expectPrinted(t, "* { background: url('f o') }", "* {\n  background: url(f\\ o);\n}\n")
	expectPrinted(t, "* { background: url('f  o') }", "* {\n  background: url(\"f  o\");\n}\n")
	expectPrinted(t, "* { background: url('foo)') }", "* {\n  background: url(foo\\));\n}\n")
	expectPrinted(t, "* { background: url('(foo') }", "* {\n  background: url(\\(foo);\n}\n")
	expectPrinted(t, "* { background: url('(foo)') }", "* {\n  background: url(\"(foo)\");\n}\n")
	expectPrinted(t, "* { background: url('\"foo\"') }", "* {\n  background: url('\"foo\"');\n}\n")
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

	expectPrinted(t, ":unknown( x (a+b), 'c' ) {}", ":unknown(x (a+b), \"c\") {\n}\n")
	expectPrinted(t, ":unknown( x (a-b), 'c' ) {}", ":unknown(x (a-b), \"c\") {\n}\n")
	expectPrinted(t, ":unknown( x (a,b), 'c' ) {}", ":unknown(x (a, b), \"c\") {\n}\n")
	expectPrinted(t, ":unknown( x ( a + b ), 'c' ) {}", ":unknown(x (a + b), \"c\") {\n}\n")
	expectPrinted(t, ":unknown( x ( a - b ), 'c' ) {}", ":unknown(x (a - b), \"c\") {\n}\n")
	expectPrinted(t, ":unknown( x ( a , b ), 'c' ) {}", ":unknown(x (a, b), \"c\") {\n}\n")

	expectPrintedMinify(t, ":unknown( x (a+b), 'c' ) {}", ":unknown(x (a+b),\"c\"){}")
	expectPrintedMinify(t, ":unknown( x (a-b), 'c' ) {}", ":unknown(x (a-b),\"c\"){}")
	expectPrintedMinify(t, ":unknown( x (a,b), 'c' ) {}", ":unknown(x (a,b),\"c\"){}")
	expectPrintedMinify(t, ":unknown( x ( a + b ), 'c' ) {}", ":unknown(x (a + b),\"c\"){}")
	expectPrintedMinify(t, ":unknown( x ( a - b ), 'c' ) {}", ":unknown(x (a - b),\"c\"){}")
	expectPrintedMinify(t, ":unknown( x ( a , b ), 'c' ) {}", ":unknown(x (a,b),\"c\"){}")
}

func TestNestedSelector(t *testing.T) {
	expectPrintedMinify(t, "a { &b {} }", "a{&b{}}")
	expectPrintedMinify(t, "a { & b {} }", "a{& b{}}")
	expectPrintedMinify(t, "a { & :b {} }", "a{& :b{}}")
}

func TestBadQualifiedRules(t *testing.T) {
	expectPrinted(t, "$bad: rule;", "$bad: rule {\n}\n")
	expectPrinted(t, "a { div.major { color: blue } color: red }", "a {\n  div.major { color: blue };\n  color: red;\n}\n")
	expectPrinted(t, "a { div:hover { color: blue } color: red }", "a {\n  div: hover { color: blue };\n  color: red;\n}\n")
	expectPrinted(t, "a { div:hover { color: blue }; color: red }", "a {\n  div: hover { color: blue };\n  color: red;\n}\n")

	expectPrinted(t, "$bad{ color: red }", "$bad {\n  color: red;\n}\n")
	expectPrinted(t, "$bad { color: red }", "$bad {\n  color: red;\n}\n")
	expectPrinted(t, "$bad foo{ color: red }", "$bad foo {\n  color: red;\n}\n")
	expectPrinted(t, "$bad foo { color: red }", "$bad foo {\n  color: red;\n}\n")

	expectPrintedMinify(t, "$bad{ color: red }", "$bad{color:red}")
	expectPrintedMinify(t, "$bad { color: red }", "$bad{color:red}")
	expectPrintedMinify(t, "$bad foo{ color: red }", "$bad foo{color:red}")
	expectPrintedMinify(t, "$bad foo { color: red }", "$bad foo{color:red}")
}

func TestDeclaration(t *testing.T) {
	expectPrinted(t, "* { unknown: x (a+b) }", "* {\n  unknown: x (a+b);\n}\n")
	expectPrinted(t, "* { unknown: x (a-b) }", "* {\n  unknown: x (a-b);\n}\n")
	expectPrinted(t, "* { unknown: x (a,b) }", "* {\n  unknown: x (a, b);\n}\n")
	expectPrinted(t, "* { unknown: x ( a + b ) }", "* {\n  unknown: x (a + b);\n}\n")
	expectPrinted(t, "* { unknown: x ( a - b ) }", "* {\n  unknown: x (a - b);\n}\n")
	expectPrinted(t, "* { unknown: x ( a , b ) }", "* {\n  unknown: x (a, b);\n}\n")

	expectPrintedMinify(t, "* { unknown: x (a+b) }", "*{unknown:x (a+b)}")
	expectPrintedMinify(t, "* { unknown: x (a-b) }", "*{unknown:x (a-b)}")
	expectPrintedMinify(t, "* { unknown: x (a,b) }", "*{unknown:x (a,b)}")
	expectPrintedMinify(t, "* { unknown: x ( a + b ) }", "*{unknown:x (a + b)}")
	expectPrintedMinify(t, "* { unknown: x ( a - b ) }", "*{unknown:x (a - b)}")
	expectPrintedMinify(t, "* { unknown: x ( a , b ) }", "*{unknown:x (a,b)}")
}

func TestAtRule(t *testing.T) {
	expectPrintedMinify(t, "@unknown;", "@unknown;")
	expectPrintedMinify(t, "@unknown x;", "@unknown x;")
	expectPrintedMinify(t, "@unknown{}", "@unknown{}")
	expectPrintedMinify(t, "@unknown{\na: b;\nc: d;\n}", "@unknown{a: b; c: d;}")

	expectPrinted(t, "@unknown x{}", "@unknown x {}\n")
	expectPrinted(t, "@unknown x {}", "@unknown x {}\n")
	expectPrintedMinify(t, "@unknown x{}", "@unknown x{}")
	expectPrintedMinify(t, "@unknown x {}", "@unknown x{}")

	expectPrinted(t, "@unknown x ( a + b ) ;", "@unknown x (a + b);\n")
	expectPrinted(t, "@unknown x ( a - b ) ;", "@unknown x (a - b);\n")
	expectPrinted(t, "@unknown x ( a , b ) ;", "@unknown x (a, b);\n")
	expectPrintedMinify(t, "@unknown x ( a + b ) ;", "@unknown x (a + b);")
	expectPrintedMinify(t, "@unknown x ( a - b ) ;", "@unknown x (a - b);")
	expectPrintedMinify(t, "@unknown x ( a , b ) ;", "@unknown x (a,b);")
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

func TestAtKeyframes(t *testing.T) {
	expectPrintedMinify(t, "@keyframes name { 0%, 50% { color: red } 25%, 75% { color: blue } }",
		"@keyframes name{0%,50%{color:red}25%,75%{color:blue}}")
	expectPrintedMinify(t, "@keyframes name { from { color: red } to { color: blue } }",
		"@keyframes name{from{color:red}to{color:blue}}")
}

func TestAtMedia(t *testing.T) {
	expectPrinted(t, "@media screen { div { color: red } }", "@media screen {\n  div {\n    color: red;\n  }\n}\n")
	expectPrinted(t, "@media screen{div{color:red}}", "@media screen {\n  div {\n    color: red;\n  }\n}\n")
	expectPrintedMinify(t, "@media screen { div { color: red } }", "@media screen{div{color:red}}")
	expectPrintedMinify(t, "@media screen{div{color:red}}", "@media screen{div{color:red}}")
}

func TestAtFontFace(t *testing.T) {
	expectPrinted(t, "@font-face { font-family: 'Open Sans'; src: url('OpenSans.woff') format('woff') }",
		"@font-face {\n  font-family: \"Open Sans\";\n  src: url(OpenSans.woff) format(\"woff\");\n}\n")
	expectPrintedMinify(t, "@font-face { font-family: 'Open Sans'; src: url('OpenSans.woff') format('woff') }",
		"@font-face{font-family:\"Open Sans\";src:url(OpenSans.woff) format(\"woff\")}")
}

func TestAtPage(t *testing.T) {
	expectPrinted(t, "@page { margin: 1cm }", "@page {\n  margin: 1cm;\n}\n")
	expectPrinted(t, "@page :first { margin: 1cm }", "@page :first {\n  margin: 1cm;\n}\n")
	expectPrintedMinify(t, "@page { margin: 1cm }", "@page{margin:1cm}")
	expectPrintedMinify(t, "@page :first { margin: 1cm }", "@page :first{margin:1cm}")
}

func TestMsGridColumnsWhitespace(t *testing.T) {
	// Must not insert a space between the "]" and the "("
	expectPrinted(t, "div { -ms-grid-columns: (1fr)[3] }", "div {\n  -ms-grid-columns: (1fr)[3];\n}\n")
	expectPrinted(t, "div { -ms-grid-columns: 1fr (20px 1fr)[3] }", "div {\n  -ms-grid-columns: 1fr (20px 1fr)[3];\n}\n")
	expectPrintedMinify(t, "div { -ms-grid-columns: (1fr)[3] }", "div{-ms-grid-columns:(1fr)[3]}")
	expectPrintedMinify(t, "div { -ms-grid-columns: 1fr (20px 1fr)[3] }", "div{-ms-grid-columns:1fr (20px 1fr)[3]}")
}

func TestASCII(t *testing.T) {
	expectPrintedASCII(t, "* { background: url(🐈) }", "* {\n  background: url(\\1f408);\n}\n")
	expectPrintedASCII(t, "* { background: url(🐈6) }", "* {\n  background: url(\\1f408 6);\n}\n")
	expectPrintedASCII(t, "* { background: url('🐈') }", "* {\n  background: url(\\1f408);\n}\n")
	expectPrintedASCII(t, "* { background: url('🐈6') }", "* {\n  background: url(\\1f408 6);\n}\n")
	expectPrintedASCII(t, "* { background: url('(🐈)') }", "* {\n  background: url(\"(\\1f408)\");\n}\n")
	expectPrintedASCII(t, "* { background: url('(🐈6)') }", "* {\n  background: url(\"(\\1f408 6)\");\n}\n")

	expectPrintedASCII(t, "div { 🐈: 🐈('🐈') }", "div {\n  \\1f408: \\1f408(\"\\1f408\");\n}\n")
	expectPrintedASCII(t, "div { 🐈 : 🐈 ('🐈 ') }", "div {\n  \\1f408: \\1f408  (\"\\1f408  \");\n}\n")
	expectPrintedASCII(t, "div { 🐈6: 🐈6('🐈6') }", "div {\n  \\1f408 6: \\1f408 6(\"\\1f408 6\");\n}\n")

	expectPrintedASCII(t, "@🐈;", "@\\1f408;\n")
	expectPrintedASCII(t, "@🐈 {}", "@\\1f408 {}\n")
	expectPrintedASCII(t, "@🐈 x {}", "@\\1f408  x {}\n")

	expectPrintedASCII(t, "#🐈#x {}", "#\\1f408#x {\n}\n")
	expectPrintedASCII(t, "#🐈 #x {}", "#\\1f408  #x {\n}\n")
	expectPrintedASCII(t, "#🐈::x {}", "#\\1f408::x {\n}\n")
	expectPrintedASCII(t, "#🐈 ::x {}", "#\\1f408  ::x {\n}\n")

	expectPrintedASCII(t, ".🐈.x {}", ".\\1f408.x {\n}\n")
	expectPrintedASCII(t, ".🐈 .x {}", ".\\1f408  .x {\n}\n")
	expectPrintedASCII(t, ".🐈::x {}", ".\\1f408::x {\n}\n")
	expectPrintedASCII(t, ".🐈 ::x {}", ".\\1f408  ::x {\n}\n")

	expectPrintedASCII(t, "🐈|🐈.x {}", "\\1f408|\\1f408.x {\n}\n")
	expectPrintedASCII(t, "🐈|🐈 .x {}", "\\1f408|\\1f408  .x {\n}\n")
	expectPrintedASCII(t, "🐈|🐈::x {}", "\\1f408|\\1f408::x {\n}\n")
	expectPrintedASCII(t, "🐈|🐈 ::x {}", "\\1f408|\\1f408  ::x {\n}\n")

	expectPrintedASCII(t, "::🐈:x {}", "::\\1f408:x {\n}\n")
	expectPrintedASCII(t, "::🐈 :x {}", "::\\1f408  :x {\n}\n")

	expectPrintedASCII(t, "[🐈] {}", "[\\1f408] {\n}\n")
	expectPrintedASCII(t, "[🐈=🐈] {}", "[\\1f408=\\1f408] {\n}\n")
	expectPrintedASCII(t, "[🐈|🐈=🐈] {}", "[\\1f408|\\1f408=\\1f408] {\n}\n")
}
