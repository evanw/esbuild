package css_printer

import (
	"testing"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/css_parser"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

func expectPrintedCommon(t *testing.T, name string, contents string, expected string, options Options) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		tree := css_parser.Parse(log, test.SourceForTest(contents), css_parser.OptionsFromConfig(config.LoaderCSS, &config.Options{
			MinifyWhitespace: options.MinifyWhitespace,
		}))
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			if msg.Kind == logger.Error {
				text += msg.String(logger.OutputOptions{}, logger.TerminalInfo{})
			}
		}
		test.AssertEqualWithDiff(t, text, "")
		symbols := ast.NewSymbolMap(1)
		symbols.SymbolsForSource[0] = tree.Symbols
		result := Print(tree, symbols, options)
		test.AssertEqualWithDiff(t, string(result.CSS), expected)
	})
}

func expectPrinted(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, contents, expected, Options{})
}

func expectPrintedMinify(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [minified]", contents, expected, Options{
		MinifyWhitespace: true,
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
		p.printQuoted(stringValue, 0)
		test.AssertEqualWithDiff(t, string(p.css), expected)
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
	expectPrintedString(t, "f\x01o", "\"f\x01o\"")
	expectPrintedString(t, "f\to", "\"f\to\"")

	expectPrintedString(t, "</script>", "\"</script>\"")
	expectPrintedString(t, "</style>", "\"<\\/style>\"")
	expectPrintedString(t, "</style", "\"<\\/style\"")
	expectPrintedString(t, "</STYLE", "\"<\\/STYLE\"")
	expectPrintedString(t, "</StYlE", "\"<\\/StYlE\"")
	expectPrintedString(t, ">/style", "\">/style\"")
	expectPrintedString(t, ">/STYLE", "\">/STYLE\"")
	expectPrintedString(t, ">/StYlE", "\">/StYlE\"")
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

	// ":foo()" is a parse error, but should ideally still be preserved so they don't accidentally become valid
	expectPrinted(t, ":is {}", ":is {\n}\n")
	expectPrinted(t, ":is() {}", ":is() {\n}\n")
	expectPrinted(t, ":hover {}", ":hover {\n}\n")
	expectPrinted(t, ":hover() {}", ":hover() {\n}\n")
	expectPrintedMinify(t, ":is {}", ":is{}")
	expectPrintedMinify(t, ":is() {}", ":is(){}")
	expectPrintedMinify(t, ":hover {}", ":hover{}")
	expectPrintedMinify(t, ":hover() {}", ":hover(){}")
}

func TestNestedSelector(t *testing.T) {
	expectPrintedMinify(t, "a { &b {} }", "a{&b{}}")
	expectPrintedMinify(t, "a { & b {} }", "a{& b{}}")
	expectPrintedMinify(t, "a { & :b {} }", "a{& :b{}}")
	expectPrintedMinify(t, "& a & b & c {}", "& a & b & c{}")
}

func TestBadQualifiedRules(t *testing.T) {
	expectPrinted(t, ";", "; {\n}\n")
	expectPrinted(t, "$bad: rule;", "$bad: rule; {\n}\n")
	expectPrinted(t, "a {}; b {};", "a {\n}\n; b {\n}\n; {\n}\n")
	expectPrinted(t, "a { div.major { color: blue } color: red }", "a {\n  div.major {\n    color: blue;\n  }\n  color: red;\n}\n")
	expectPrinted(t, "a { div:hover { color: blue } color: red }", "a {\n  div:hover {\n    color: blue;\n  }\n  color: red;\n}\n")
	expectPrinted(t, "a { div:hover { color: blue }; color: red }", "a {\n  div:hover {\n    color: blue;\n  }\n  color: red;\n}\n")

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

	// Pretty-print long lists in declarations
	expectPrinted(t, "a { b: c, d }", "a {\n  b: c, d;\n}\n")
	expectPrinted(t, "a { b: c, (d, e) }", "a {\n  b: c, (d, e);\n}\n")
	expectPrinted(t, "a { b: c, d, e }", "a {\n  b:\n    c,\n    d,\n    e;\n}\n")
	expectPrinted(t, "a { b: c, (d, e), f }", "a {\n  b:\n    c,\n    (d, e),\n    f;\n}\n")

	expectPrintedMinify(t, "a { b: c, d }", "a{b:c,d}")
	expectPrintedMinify(t, "a { b: c, (d, e) }", "a{b:c,(d,e)}")
	expectPrintedMinify(t, "a { b: c, d, e }", "a{b:c,d,e}")
	expectPrintedMinify(t, "a { b: c, (d, e), f }", "a{b:c,(d,e),f}")
}

func TestVerbatimWhitespace(t *testing.T) {
	expectPrinted(t, "*{--x:}", "* {\n  --x:;\n}\n")
	expectPrinted(t, "*{--x: }", "* {\n  --x: ;\n}\n")
	expectPrinted(t, "* { --x:; }", "* {\n  --x:;\n}\n")
	expectPrinted(t, "* { --x: ; }", "* {\n  --x: ;\n}\n")

	expectPrintedMinify(t, "*{--x:}", "*{--x:}")
	expectPrintedMinify(t, "*{--x: }", "*{--x: }")
	expectPrintedMinify(t, "* { --x:; }", "*{--x:}")
	expectPrintedMinify(t, "* { --x: ; }", "*{--x: }")

	expectPrinted(t, "*{--x:!important}", "* {\n  --x:!important;\n}\n")
	expectPrinted(t, "*{--x: !important}", "* {\n  --x: !important;\n}\n")
	expectPrinted(t, "*{ --x:!important }", "* {\n  --x:!important;\n}\n")
	expectPrinted(t, "*{ --x: !important }", "* {\n  --x: !important;\n}\n")
	expectPrinted(t, "* { --x:!important; }", "* {\n  --x:!important;\n}\n")
	expectPrinted(t, "* { --x: !important; }", "* {\n  --x: !important;\n}\n")
	expectPrinted(t, "* { --x:! important ; }", "* {\n  --x:!important;\n}\n")
	expectPrinted(t, "* { --x: ! important ; }", "* {\n  --x: !important;\n}\n")

	expectPrintedMinify(t, "*{--x:!important}", "*{--x:!important}")
	expectPrintedMinify(t, "*{--x: !important}", "*{--x: !important}")
	expectPrintedMinify(t, "*{ --x:!important }", "*{--x:!important}")
	expectPrintedMinify(t, "*{ --x: !important }", "*{--x: !important}")
	expectPrintedMinify(t, "* { --x:!important; }", "*{--x:!important}")
	expectPrintedMinify(t, "* { --x: !important; }", "*{--x: !important}")
	expectPrintedMinify(t, "* { --x:! important ; }", "*{--x:!important}")
	expectPrintedMinify(t, "* { --x: ! important ; }", "*{--x: !important}")

	expectPrinted(t, "* { --x:y; }", "* {\n  --x:y;\n}\n")
	expectPrinted(t, "* { --x: y; }", "* {\n  --x: y;\n}\n")
	expectPrinted(t, "* { --x:y ; }", "* {\n  --x:y ;\n}\n")
	expectPrinted(t, "* { --x:y, ; }", "* {\n  --x:y, ;\n}\n")
	expectPrinted(t, "* { --x: var(y,); }", "* {\n  --x: var(y,);\n}\n")
	expectPrinted(t, "* { --x: var(y, ); }", "* {\n  --x: var(y, );\n}\n")

	expectPrintedMinify(t, "* { --x:y; }", "*{--x:y}")
	expectPrintedMinify(t, "* { --x: y; }", "*{--x: y}")
	expectPrintedMinify(t, "* { --x:y ; }", "*{--x:y }")
	expectPrintedMinify(t, "* { --x:y, ; }", "*{--x:y, }")
	expectPrintedMinify(t, "* { --x: var(y,); }", "*{--x: var(y,)}")
	expectPrintedMinify(t, "* { --x: var(y, ); }", "*{--x: var(y, )}")

	expectPrinted(t, "* { --x:(y); }", "* {\n  --x:(y);\n}\n")
	expectPrinted(t, "* { --x:(y) ; }", "* {\n  --x:(y) ;\n}\n")
	expectPrinted(t, "* { --x: (y); }", "* {\n  --x: (y);\n}\n")
	expectPrinted(t, "* { --x:(y ); }", "* {\n  --x:(y );\n}\n")
	expectPrinted(t, "* { --x:( y); }", "* {\n  --x:( y);\n}\n")

	expectPrintedMinify(t, "* { --x:(y); }", "*{--x:(y)}")
	expectPrintedMinify(t, "* { --x:(y) ; }", "*{--x:(y) }")
	expectPrintedMinify(t, "* { --x: (y); }", "*{--x: (y)}")
	expectPrintedMinify(t, "* { --x:(y ); }", "*{--x:(y )}")
	expectPrintedMinify(t, "* { --x:( y); }", "*{--x:( y)}")

	expectPrinted(t, "* { --x:f(y); }", "* {\n  --x:f(y);\n}\n")
	expectPrinted(t, "* { --x:f(y) ; }", "* {\n  --x:f(y) ;\n}\n")
	expectPrinted(t, "* { --x: f(y); }", "* {\n  --x: f(y);\n}\n")
	expectPrinted(t, "* { --x:f(y ); }", "* {\n  --x:f(y );\n}\n")
	expectPrinted(t, "* { --x:f( y); }", "* {\n  --x:f( y);\n}\n")

	expectPrintedMinify(t, "* { --x:f(y); }", "*{--x:f(y)}")
	expectPrintedMinify(t, "* { --x:f(y) ; }", "*{--x:f(y) }")
	expectPrintedMinify(t, "* { --x: f(y); }", "*{--x: f(y)}")
	expectPrintedMinify(t, "* { --x:f(y ); }", "*{--x:f(y )}")
	expectPrintedMinify(t, "* { --x:f( y); }", "*{--x:f( y)}")

	expectPrinted(t, "* { --x:[y]; }", "* {\n  --x:[y];\n}\n")
	expectPrinted(t, "* { --x:[y] ; }", "* {\n  --x:[y] ;\n}\n")
	expectPrinted(t, "* { --x: [y]; }", "* {\n  --x: [y];\n}\n")
	expectPrinted(t, "* { --x:[y ]; }", "* {\n  --x:[y ];\n}\n")
	expectPrinted(t, "* { --x:[ y]; }", "* {\n  --x:[ y];\n}\n")

	expectPrintedMinify(t, "* { --x:[y]; }", "*{--x:[y]}")
	expectPrintedMinify(t, "* { --x:[y] ; }", "*{--x:[y] }")
	expectPrintedMinify(t, "* { --x: [y]; }", "*{--x: [y]}")
	expectPrintedMinify(t, "* { --x:[y ]; }", "*{--x:[y ]}")
	expectPrintedMinify(t, "* { --x:[ y]; }", "*{--x:[ y]}")

	// Note: These cases now behave like qualified rules
	expectPrinted(t, "* { --x:{y}; }", "* {\n  --x: {\n    y;\n  }\n}\n")
	expectPrinted(t, "* { --x:{y} ; }", "* {\n  --x: {\n    y;\n  }\n}\n")
	expectPrinted(t, "* { --x: {y}; }", "* {\n  --x: {\n    y;\n  }\n}\n")
	expectPrinted(t, "* { --x:{y }; }", "* {\n  --x: {\n    y;\n  }\n}\n")
	expectPrinted(t, "* { --x:{ y}; }", "* {\n  --x: {\n    y;\n  }\n}\n")

	// Note: These cases now behave like qualified rules
	expectPrintedMinify(t, "* { --x:{y}; }", "*{--x:{y}}")
	expectPrintedMinify(t, "* { --x:{y} ; }", "*{--x:{y}}")
	expectPrintedMinify(t, "* { --x: {y}; }", "*{--x:{y}}")
	expectPrintedMinify(t, "* { --x:{y }; }", "*{--x:{y}}")
	expectPrintedMinify(t, "* { --x:{ y}; }", "*{--x:{y}}")

	expectPrintedMinify(t, "@supports ( --x : y , z ) { a { color: red; } }", "@supports ( --x : y , z ){a{color:red}}")
	expectPrintedMinify(t, "@supports ( --x : ) { a { color: red; } }", "@supports ( --x : ){a{color:red}}")
	expectPrintedMinify(t, "@supports (--x: ) { a { color: red; } }", "@supports (--x: ){a{color:red}}")
	expectPrintedMinify(t, "@supports ( --x y , z ) { a { color: red; } }", "@supports (--x y,z){a{color:red}}")
	expectPrintedMinify(t, "@supports ( --x ) { a { color: red; } }", "@supports (--x){a{color:red}}")
	expectPrintedMinify(t, "@supports ( ) { a { color: red; } }", "@supports (){a{color:red}}")
	expectPrintedMinify(t, "@supports ( . --x : y , z ) { a { color: red; } }", "@supports (. --x : y,z){a{color:red}}")
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

func TestAtImport(t *testing.T) {
	expectPrinted(t, "@import\"foo.css\";", "@import \"foo.css\";\n")
	expectPrinted(t, "@import \"foo.css\";", "@import \"foo.css\";\n")
	expectPrinted(t, "@import url(foo.css);", "@import \"foo.css\";\n")
	expectPrinted(t, "@import url(\"foo.css\");", "@import \"foo.css\";\n")
	expectPrinted(t, "@import url(\"foo.css\") print;", "@import \"foo.css\" print;\n")

	expectPrintedMinify(t, "@import\"foo.css\";", "@import\"foo.css\";")
	expectPrintedMinify(t, "@import \"foo.css\";", "@import\"foo.css\";")
	expectPrintedMinify(t, "@import url(foo.css);", "@import\"foo.css\";")
	expectPrintedMinify(t, "@import url(\"foo.css\");", "@import\"foo.css\";")
	expectPrintedMinify(t, "@import url(\"foo.css\") print;", "@import\"foo.css\"print;")
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

	// A space must be consumed after an escaped code point even with six digits
	expectPrintedASCII(t, ".\\10FFF abc:after { content: '\\10FFF abc' }", ".\\10fff abc:after {\n  content: \"\\10fff abc\";\n}\n")
	expectPrintedASCII(t, ".\U00010FFFabc:after { content: '\U00010FFFabc' }", ".\\10fff abc:after {\n  content: \"\\10fff abc\";\n}\n")
	expectPrintedASCII(t, ".\\10FFFFabc:after { content: '\\10FFFFabc' }", ".\\10ffffabc:after {\n  content: \"\\10ffffabc\";\n}\n")
	expectPrintedASCII(t, ".\\10FFFF abc:after { content: '\\10FFFF abc' }", ".\\10ffffabc:after {\n  content: \"\\10ffffabc\";\n}\n")
	expectPrintedASCII(t, ".\U0010FFFFabc:after { content: '\U0010FFFFabc' }", ".\\10ffffabc:after {\n  content: \"\\10ffffabc\";\n}\n")

	// This character should always be escaped
	expectPrinted(t, ".\\FEFF:after { content: '\uFEFF' }", ".\\feff:after {\n  content: \"\\feff\";\n}\n")
}
