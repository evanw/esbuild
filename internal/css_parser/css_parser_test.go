package css_parser

import (
	"testing"

	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
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
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug)
		Parse(log, test.SourceForTest(contents), Options{})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logger.OutputOptions{}, logger.TerminalInfo{})
		}
		test.AssertEqual(t, text, expected)
	})
}

func expectPrintedCommon(t *testing.T, name string, contents string, expected string, options config.Options) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug)
		tree := Parse(log, test.SourceForTest(contents), Options{
			MangleSyntax:           options.MangleSyntax,
			RemoveWhitespace:       options.RemoveWhitespace,
			UnsupportedCSSFeatures: options.UnsupportedCSSFeatures,
		})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			if msg.Kind == logger.Error {
				text += msg.String(logger.OutputOptions{}, logger.TerminalInfo{})
			}
		}
		assertEqual(t, text, "")
		css := css_printer.Print(tree, css_printer.Options{
			RemoveWhitespace: options.RemoveWhitespace,
		})
		assertEqual(t, string(css), expected)
	})
}

func expectPrinted(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, contents, expected, config.Options{})
}

func expectPrintedLower(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [mangle]", contents, expected, config.Options{
		UnsupportedCSSFeatures: ^compat.CSSFeature(0),
	})
}

func expectPrintedMangle(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [mangle]", contents, expected, config.Options{
		MangleSyntax: true,
	})
}

func expectPrintedLowerMangle(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [mangle]", contents, expected, config.Options{
		UnsupportedCSSFeatures: ^compat.CSSFeature(0),
		MangleSyntax:           true,
	})
}

func expectPrintedMangleMinify(t *testing.T, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents+" [mangle, minify]", contents, expected, config.Options{
		MangleSyntax:     true,
		RemoveWhitespace: true,
	})
}

func TestEscapes(t *testing.T) {
	// TIdent
	expectPrinted(t, "a { value: id\\65nt }", "a {\n  value: ident;\n}\n")
	expectPrinted(t, "a { value: \\69 dent }", "a {\n  value: ident;\n}\n")
	expectPrinted(t, "a { value: \\69dent }", "a {\n  value: \u69DEnt;\n}\n")
	expectPrinted(t, "a { value: \\2cx }", "a {\n  value: \\,x;\n}\n")
	expectPrinted(t, "a { value: \\,x }", "a {\n  value: \\,x;\n}\n")
	expectPrinted(t, "a { value: x\\2c }", "a {\n  value: x\\,;\n}\n")
	expectPrinted(t, "a { value: x\\, }", "a {\n  value: x\\,;\n}\n")
	expectPrinted(t, "a { value: x\\0 }", "a {\n  value: x\uFFFD;\n}\n")
	expectPrinted(t, "a { value: x\\1 }", "a {\n  value: x\\\x01;\n}\n")
	expectPrinted(t, "a { value: x\x00 }", "a {\n  value: x\uFFFD;\n}\n")
	expectPrinted(t, "a { value: x\x01 }", "a {\n  value: x\x01;\n}\n")

	// THash
	expectPrinted(t, "a { value: #0h\\61sh }", "a {\n  value: #0hash;\n}\n")
	expectPrinted(t, "a { value: #\\30hash }", "a {\n  value: #0hash;\n}\n")
	expectPrinted(t, "a { value: #\\2cx }", "a {\n  value: #\\,x;\n}\n")
	expectPrinted(t, "a { value: #\\,x }", "a {\n  value: #\\,x;\n}\n")

	// THashID
	expectPrinted(t, "a { value: #h\\61sh }", "a {\n  value: #hash;\n}\n")
	expectPrinted(t, "a { value: #\\68 ash }", "a {\n  value: #hash;\n}\n")
	expectPrinted(t, "a { value: #\\68ash }", "a {\n  value: #\u068Ash;\n}\n")
	expectPrinted(t, "a { value: #x\\2c }", "a {\n  value: #x\\,;\n}\n")
	expectPrinted(t, "a { value: #x\\, }", "a {\n  value: #x\\,;\n}\n")

	// TFunction
	expectPrinted(t, "a { value: f\\6e() }", "a {\n  value: fn();\n}\n")
	expectPrinted(t, "a { value: \\66n() }", "a {\n  value: fn();\n}\n")
	expectPrinted(t, "a { value: \\2cx() }", "a {\n  value: \\,x();\n}\n")
	expectPrinted(t, "a { value: \\,x() }", "a {\n  value: \\,x();\n}\n")
	expectPrinted(t, "a { value: x\\2c() }", "a {\n  value: x\\,();\n}\n")
	expectPrinted(t, "a { value: x\\,() }", "a {\n  value: x\\,();\n}\n")

	// TString
	expectPrinted(t, "a { value: 'a\\62 c' }", "a {\n  value: \"abc\";\n}\n")
	expectPrinted(t, "a { value: 'a\\62c' }", "a {\n  value: \"a\u062C\";\n}\n")
	expectPrinted(t, "a { value: '\\61 bc' }", "a {\n  value: \"abc\";\n}\n")
	expectPrinted(t, "a { value: '\\61bc' }", "a {\n  value: \"\u61BC\";\n}\n")
	expectPrinted(t, "a { value: '\\2c' }", "a {\n  value: \",\";\n}\n")
	expectPrinted(t, "a { value: '\\,' }", "a {\n  value: \",\";\n}\n")
	expectPrinted(t, "a { value: '\\0' }", "a {\n  value: \"\uFFFD\";\n}\n")
	expectPrinted(t, "a { value: '\\1' }", "a {\n  value: \"\x01\";\n}\n")
	expectPrinted(t, "a { value: '\x00' }", "a {\n  value: \"\uFFFD\";\n}\n")
	expectPrinted(t, "a { value: '\x01' }", "a {\n  value: \"\x01\";\n}\n")

	// TURL
	expectPrinted(t, "a { value: url(a\\62 c) }", "a {\n  value: url(abc);\n}\n")
	expectPrinted(t, "a { value: url(a\\62c) }", "a {\n  value: url(a\u062C);\n}\n")
	expectPrinted(t, "a { value: url(\\61 bc) }", "a {\n  value: url(abc);\n}\n")
	expectPrinted(t, "a { value: url(\\61bc) }", "a {\n  value: url(\u61BC);\n}\n")
	expectPrinted(t, "a { value: url(\\2c) }", "a {\n  value: url(,);\n}\n")
	expectPrinted(t, "a { value: url(\\,) }", "a {\n  value: url(,);\n}\n")

	// TAtKeyword
	expectPrinted(t, "a { value: @k\\65yword }", "a {\n  value: @keyword;\n}\n")
	expectPrinted(t, "a { value: @\\6b eyword }", "a {\n  value: @keyword;\n}\n")
	expectPrinted(t, "a { value: @\\6beyword }", "a {\n  value: @\u06BEyword;\n}\n")
	expectPrinted(t, "a { value: @\\2cx }", "a {\n  value: @\\,x;\n}\n")
	expectPrinted(t, "a { value: @\\,x }", "a {\n  value: @\\,x;\n}\n")
	expectPrinted(t, "a { value: @x\\2c }", "a {\n  value: @x\\,;\n}\n")
	expectPrinted(t, "a { value: @x\\, }", "a {\n  value: @x\\,;\n}\n")

	// TDimension
	expectPrinted(t, "a { value: 10\\65m }", "a {\n  value: 10em;\n}\n")
	expectPrinted(t, "a { value: 10p\\32x }", "a {\n  value: 10p2x;\n}\n")
	expectPrinted(t, "a { value: 10e\\32x }", "a {\n  value: 10\\65 2x;\n}\n")
	expectPrinted(t, "a { value: 10\\32x }", "a {\n  value: 10\\32x;\n}\n")
	expectPrinted(t, "a { value: 10\\2cx }", "a {\n  value: 10\\,x;\n}\n")
	expectPrinted(t, "a { value: 10\\,x }", "a {\n  value: 10\\,x;\n}\n")
	expectPrinted(t, "a { value: 10x\\2c }", "a {\n  value: 10x\\,;\n}\n")
	expectPrinted(t, "a { value: 10x\\, }", "a {\n  value: 10x\\,;\n}\n")

	// RDeclaration
	expectPrintedMangle(t, "a { c\\6flor: #f00 }", "a {\n  color: red;\n}\n")
	expectPrintedMangle(t, "a { \\63olor: #f00 }", "a {\n  color: red;\n}\n")
	expectPrintedMangle(t, "a { \\2color: #f00 }", "a {\n  \\,olor: #f00;\n}\n")
	expectPrintedMangle(t, "a { \\,olor: #f00 }", "a {\n  \\,olor: #f00;\n}\n")

	// RUnknownAt
	expectPrinted(t, "@unknown;", "@unknown;\n")
	expectPrinted(t, "@u\\6eknown;", "@unknown;\n")
	expectPrinted(t, "@\\75nknown;", "@unknown;\n")
	expectPrinted(t, "@u\\2cnknown;", "@u\\,nknown;\n")
	expectPrinted(t, "@u\\,nknown;", "@u\\,nknown;\n")
	expectPrinted(t, "@\\2cunknown;", "@\\,unknown;\n")
	expectPrinted(t, "@\\,unknown;", "@\\,unknown;\n")

	// RAtKeyframes
	expectPrinted(t, "@k\\65yframes abc { from {} }", "@keyframes abc {\n  from {\n  }\n}\n")
	expectPrinted(t, "@keyframes \\61 bc { from {} }", "@keyframes abc {\n  from {\n  }\n}\n")
	expectPrinted(t, "@keyframes a\\62 c { from {} }", "@keyframes abc {\n  from {\n  }\n}\n")
	expectPrinted(t, "@keyframes abc { \\66rom {} }", "@keyframes abc {\n  from {\n  }\n}\n")
	expectPrinted(t, "@keyframes a\\2c c { \\66rom {} }", "@keyframes a\\,c {\n  from {\n  }\n}\n")
	expectPrinted(t, "@keyframes a\\,c { \\66rom {} }", "@keyframes a\\,c {\n  from {\n  }\n}\n")

	// RAtNamespace
	expectPrinted(t, "@n\\61mespace ns 'path';", "@namespace ns \"path\";\n")
	expectPrinted(t, "@namespace \\6es 'path';", "@namespace ns \"path\";\n")
	expectPrinted(t, "@namespace ns 'p\\61th';", "@namespace ns \"path\";\n")
	expectPrinted(t, "@namespace \\2cs 'p\\61th';", "@namespace \\,s \"path\";\n")
	expectPrinted(t, "@namespace \\,s 'p\\61th';", "@namespace \\,s \"path\";\n")

	// CompoundSelector
	expectPrinted(t, "* {}", "* {\n}\n")
	expectPrinted(t, "*|div {}", "*|div {\n}\n")
	expectPrinted(t, "\\2a {}", "\\* {\n}\n")
	expectPrinted(t, "\\2a|div {}", "\\*|div {\n}\n")
	expectPrinted(t, "\\2d {}", "\\- {\n}\n")
	expectPrinted(t, "\\2d- {}", "-- {\n}\n")
	expectPrinted(t, "-\\2d {}", "-- {\n}\n")
	expectPrinted(t, "\\2d 123 {}", "\\-123 {\n}\n")

	// SSHash
	expectPrinted(t, "#h\\61sh {}", "#hash {\n}\n")
	expectPrinted(t, "#\\2chash {}", "#\\,hash {\n}\n")
	expectPrinted(t, "#\\,hash {}", "#\\,hash {\n}\n")
	expectPrinted(t, "#\\2d {}", "#\\- {\n}\n")
	expectPrinted(t, "#\\2d- {}", "#-- {\n}\n")
	expectPrinted(t, "#-\\2d {}", "#-- {\n}\n")
	expectPrinted(t, "#\\2d 123 {}", "#\\-123 {\n}\n")
	expectPrinted(t, "#\\61hash {}", "#ahash {\n}\n")
	expectPrinted(t, "#\\30hash {}", "#\\30hash {\n}\n")
	expectPrinted(t, "#0\\2chash {}", "#0\\,hash {\n}\n")
	expectPrinted(t, "#0\\,hash {}", "#0\\,hash {\n}\n")

	// SSClass
	expectPrinted(t, ".cl\\61ss {}", ".class {\n}\n")
	expectPrinted(t, ".\\2c class {}", ".\\,class {\n}\n")
	expectPrinted(t, ".\\,class {}", ".\\,class {\n}\n")

	// SSPseudoClass
	expectPrinted(t, ":pseudocl\\61ss {}", ":pseudoclass {\n}\n")
	expectPrinted(t, ":pseudo\\2c class {}", ":pseudo\\,class {\n}\n")
	expectPrinted(t, ":pseudo\\,class {}", ":pseudo\\,class {\n}\n")
	expectPrinted(t, ":pseudo(cl\\61ss) {}", ":pseudo(class) {\n}\n")
	expectPrinted(t, ":pseudo(cl\\2css) {}", ":pseudo(cl\\,ss) {\n}\n")
	expectPrinted(t, ":pseudo(cl\\,ss) {}", ":pseudo(cl\\,ss) {\n}\n")

	// SSAttribute
	expectPrinted(t, "[\\61ttr] {}", "[attr] {\n}\n")
	expectPrinted(t, "[\\2c attr] {}", "[\\,attr] {\n}\n")
	expectPrinted(t, "[\\,attr] {}", "[\\,attr] {\n}\n")
	expectPrinted(t, "[attr\\7e=x] {}", "[attr\\~=x] {\n}\n")
	expectPrinted(t, "[attr\\~=x] {}", "[attr\\~=x] {\n}\n")
	expectPrinted(t, "[attr=\\2c] {}", "[attr=\",\"] {\n}\n")
	expectPrinted(t, "[attr=\\,] {}", "[attr=\",\"] {\n}\n")
	expectPrinted(t, "[attr=\"-\"] {}", "[attr=\"-\"] {\n}\n")
	expectPrinted(t, "[attr=\"--\"] {}", "[attr=--] {\n}\n")
	expectPrinted(t, "[attr=\"-a\"] {}", "[attr=-a] {\n}\n")
	expectPrinted(t, "[\\6es|attr] {}", "[ns|attr] {\n}\n")
	expectPrinted(t, "[ns|\\61ttr] {}", "[ns|attr] {\n}\n")
	expectPrinted(t, "[\\2cns|attr] {}", "[\\,ns|attr] {\n}\n")
	expectPrinted(t, "[ns|\\2c attr] {}", "[ns|\\,attr] {\n}\n")
	expectPrinted(t, "[*|attr] {}", "[*|attr] {\n}\n")
	expectPrinted(t, "[\\2a|attr] {}", "[\\*|attr] {\n}\n")
}

func TestString(t *testing.T) {
	expectPrinted(t, "a:after { content: 'a\\\rb' }", "a:after {\n  content: \"ab\";\n}\n")
	expectPrinted(t, "a:after { content: 'a\\\nb' }", "a:after {\n  content: \"ab\";\n}\n")
	expectPrinted(t, "a:after { content: 'a\\\fb' }", "a:after {\n  content: \"ab\";\n}\n")
	expectPrinted(t, "a:after { content: 'a\\\r\nb' }", "a:after {\n  content: \"ab\";\n}\n")
	expectPrinted(t, "a:after { content: 'a\\62 c' }", "a:after {\n  content: \"abc\";\n}\n")

	expectParseError(t, "a:after { content: '\r' }",
		`<stdin>: error: Unterminated string token
<stdin>: error: Unterminated string token
<stdin>: warning: Expected "}" but found end of file
`)
	expectParseError(t, "a:after { content: '\n' }",
		`<stdin>: error: Unterminated string token
<stdin>: error: Unterminated string token
<stdin>: warning: Expected "}" but found end of file
`)
	expectParseError(t, "a:after { content: '\f' }",
		`<stdin>: error: Unterminated string token
<stdin>: error: Unterminated string token
<stdin>: warning: Expected "}" but found end of file
`)
	expectParseError(t, "a:after { content: '\r\n' }",
		`<stdin>: error: Unterminated string token
<stdin>: error: Unterminated string token
<stdin>: warning: Expected "}" but found end of file
`)

	expectPrinted(t, "a:after { content: '\\1010101' }", "a:after {\n  content: \"\U001010101\";\n}\n")
	expectPrinted(t, "a:after { content: '\\invalid' }", "a:after {\n  content: \"invalid\";\n}\n")
}

func TestNumber(t *testing.T) {
	for _, ext := range []string{"", "%", "px+"} {
		expectPrinted(t, "a { width: .0"+ext+"; }", "a {\n  width: .0"+ext+";\n}\n")
		expectPrinted(t, "a { width: .00"+ext+"; }", "a {\n  width: .00"+ext+";\n}\n")
		expectPrinted(t, "a { width: .10"+ext+"; }", "a {\n  width: .10"+ext+";\n}\n")
		expectPrinted(t, "a { width: 0."+ext+"; }", "a {\n  width: 0."+ext+";\n}\n")
		expectPrinted(t, "a { width: 0.0"+ext+"; }", "a {\n  width: 0.0"+ext+";\n}\n")
		expectPrinted(t, "a { width: 0.1"+ext+"; }", "a {\n  width: 0.1"+ext+";\n}\n")

		expectPrinted(t, "a { width: +.0"+ext+"; }", "a {\n  width: +.0"+ext+";\n}\n")
		expectPrinted(t, "a { width: +.00"+ext+"; }", "a {\n  width: +.00"+ext+";\n}\n")
		expectPrinted(t, "a { width: +.10"+ext+"; }", "a {\n  width: +.10"+ext+";\n}\n")
		expectPrinted(t, "a { width: +0."+ext+"; }", "a {\n  width: +0."+ext+";\n}\n")
		expectPrinted(t, "a { width: +0.0"+ext+"; }", "a {\n  width: +0.0"+ext+";\n}\n")
		expectPrinted(t, "a { width: +0.1"+ext+"; }", "a {\n  width: +0.1"+ext+";\n}\n")

		expectPrinted(t, "a { width: -.0"+ext+"; }", "a {\n  width: -.0"+ext+";\n}\n")
		expectPrinted(t, "a { width: -.00"+ext+"; }", "a {\n  width: -.00"+ext+";\n}\n")
		expectPrinted(t, "a { width: -.10"+ext+"; }", "a {\n  width: -.10"+ext+";\n}\n")
		expectPrinted(t, "a { width: -0."+ext+"; }", "a {\n  width: -0."+ext+";\n}\n")
		expectPrinted(t, "a { width: -0.0"+ext+"; }", "a {\n  width: -0.0"+ext+";\n}\n")
		expectPrinted(t, "a { width: -0.1"+ext+"; }", "a {\n  width: -0.1"+ext+";\n}\n")

		expectPrintedMangle(t, "a { width: .0"+ext+"; }", "a {\n  width: 0"+ext+";\n}\n")
		expectPrintedMangle(t, "a { width: .00"+ext+"; }", "a {\n  width: 0"+ext+";\n}\n")
		expectPrintedMangle(t, "a { width: .10"+ext+"; }", "a {\n  width: .1"+ext+";\n}\n")
		expectPrintedMangle(t, "a { width: 0."+ext+"; }", "a {\n  width: 0"+ext+";\n}\n")
		expectPrintedMangle(t, "a { width: 0.0"+ext+"; }", "a {\n  width: 0"+ext+";\n}\n")
		expectPrintedMangle(t, "a { width: 0.1"+ext+"; }", "a {\n  width: .1"+ext+";\n}\n")

		expectPrintedMangle(t, "a { width: +.0"+ext+"; }", "a {\n  width: +0"+ext+";\n}\n")
		expectPrintedMangle(t, "a { width: +.00"+ext+"; }", "a {\n  width: +0"+ext+";\n}\n")
		expectPrintedMangle(t, "a { width: +.10"+ext+"; }", "a {\n  width: +.1"+ext+";\n}\n")
		expectPrintedMangle(t, "a { width: +0."+ext+"; }", "a {\n  width: +0"+ext+";\n}\n")
		expectPrintedMangle(t, "a { width: +0.0"+ext+"; }", "a {\n  width: +0"+ext+";\n}\n")
		expectPrintedMangle(t, "a { width: +0.1"+ext+"; }", "a {\n  width: +.1"+ext+";\n}\n")

		expectPrintedMangle(t, "a { width: -.0"+ext+"; }", "a {\n  width: -0"+ext+";\n}\n")
		expectPrintedMangle(t, "a { width: -.00"+ext+"; }", "a {\n  width: -0"+ext+";\n}\n")
		expectPrintedMangle(t, "a { width: -.10"+ext+"; }", "a {\n  width: -.1"+ext+";\n}\n")
		expectPrintedMangle(t, "a { width: -0."+ext+"; }", "a {\n  width: -0"+ext+";\n}\n")
		expectPrintedMangle(t, "a { width: -0.0"+ext+"; }", "a {\n  width: -0"+ext+";\n}\n")
		expectPrintedMangle(t, "a { width: -0.1"+ext+"; }", "a {\n  width: -.1"+ext+";\n}\n")
	}
}

func TestHexColor(t *testing.T) {
	// "#RGBA"

	expectPrinted(t, "a { color: #1234 }", "a {\n  color: #1234;\n}\n")
	expectPrinted(t, "a { color: #123f }", "a {\n  color: #123f;\n}\n")
	expectPrinted(t, "a { color: #abcd }", "a {\n  color: #abcd;\n}\n")
	expectPrinted(t, "a { color: #abcf }", "a {\n  color: #abcf;\n}\n")
	expectPrinted(t, "a { color: #ABCD }", "a {\n  color: #ABCD;\n}\n")
	expectPrinted(t, "a { color: #ABCF }", "a {\n  color: #ABCF;\n}\n")

	expectPrintedMangle(t, "a { color: #1234 }", "a {\n  color: #1234;\n}\n")
	expectPrintedMangle(t, "a { color: #123f }", "a {\n  color: #123;\n}\n")
	expectPrintedMangle(t, "a { color: #abcd }", "a {\n  color: #abcd;\n}\n")
	expectPrintedMangle(t, "a { color: #abcf }", "a {\n  color: #abc;\n}\n")
	expectPrintedMangle(t, "a { color: #ABCD }", "a {\n  color: #abcd;\n}\n")
	expectPrintedMangle(t, "a { color: #ABCF }", "a {\n  color: #abc;\n}\n")

	// "#RRGGBB"

	expectPrinted(t, "a { color: #112233 }", "a {\n  color: #112233;\n}\n")
	expectPrinted(t, "a { color: #122233 }", "a {\n  color: #122233;\n}\n")
	expectPrinted(t, "a { color: #112333 }", "a {\n  color: #112333;\n}\n")
	expectPrinted(t, "a { color: #112234 }", "a {\n  color: #112234;\n}\n")

	expectPrintedMangle(t, "a { color: #112233 }", "a {\n  color: #123;\n}\n")
	expectPrintedMangle(t, "a { color: #122233 }", "a {\n  color: #122233;\n}\n")
	expectPrintedMangle(t, "a { color: #112333 }", "a {\n  color: #112333;\n}\n")
	expectPrintedMangle(t, "a { color: #112234 }", "a {\n  color: #112234;\n}\n")

	expectPrinted(t, "a { color: #aabbcc }", "a {\n  color: #aabbcc;\n}\n")
	expectPrinted(t, "a { color: #abbbcc }", "a {\n  color: #abbbcc;\n}\n")
	expectPrinted(t, "a { color: #aabccc }", "a {\n  color: #aabccc;\n}\n")
	expectPrinted(t, "a { color: #aabbcd }", "a {\n  color: #aabbcd;\n}\n")

	expectPrintedMangle(t, "a { color: #aabbcc }", "a {\n  color: #abc;\n}\n")
	expectPrintedMangle(t, "a { color: #abbbcc }", "a {\n  color: #abbbcc;\n}\n")
	expectPrintedMangle(t, "a { color: #aabccc }", "a {\n  color: #aabccc;\n}\n")
	expectPrintedMangle(t, "a { color: #aabbcd }", "a {\n  color: #aabbcd;\n}\n")

	expectPrinted(t, "a { color: #AABBCC }", "a {\n  color: #AABBCC;\n}\n")
	expectPrinted(t, "a { color: #ABBBCC }", "a {\n  color: #ABBBCC;\n}\n")
	expectPrinted(t, "a { color: #AABCCC }", "a {\n  color: #AABCCC;\n}\n")
	expectPrinted(t, "a { color: #AABBCD }", "a {\n  color: #AABBCD;\n}\n")

	expectPrintedMangle(t, "a { color: #AABBCC }", "a {\n  color: #abc;\n}\n")
	expectPrintedMangle(t, "a { color: #ABBBCC }", "a {\n  color: #abbbcc;\n}\n")
	expectPrintedMangle(t, "a { color: #AABCCC }", "a {\n  color: #aabccc;\n}\n")
	expectPrintedMangle(t, "a { color: #AABBCD }", "a {\n  color: #aabbcd;\n}\n")

	// "#RRGGBBAA"

	expectPrinted(t, "a { color: #11223344 }", "a {\n  color: #11223344;\n}\n")
	expectPrinted(t, "a { color: #12223344 }", "a {\n  color: #12223344;\n}\n")
	expectPrinted(t, "a { color: #11233344 }", "a {\n  color: #11233344;\n}\n")
	expectPrinted(t, "a { color: #11223444 }", "a {\n  color: #11223444;\n}\n")
	expectPrinted(t, "a { color: #11223345 }", "a {\n  color: #11223345;\n}\n")

	expectPrintedMangle(t, "a { color: #11223344 }", "a {\n  color: #1234;\n}\n")
	expectPrintedMangle(t, "a { color: #12223344 }", "a {\n  color: #12223344;\n}\n")
	expectPrintedMangle(t, "a { color: #11233344 }", "a {\n  color: #11233344;\n}\n")
	expectPrintedMangle(t, "a { color: #11223444 }", "a {\n  color: #11223444;\n}\n")
	expectPrintedMangle(t, "a { color: #11223345 }", "a {\n  color: #11223345;\n}\n")

	expectPrinted(t, "a { color: #aabbccdd }", "a {\n  color: #aabbccdd;\n}\n")
	expectPrinted(t, "a { color: #abbbccdd }", "a {\n  color: #abbbccdd;\n}\n")
	expectPrinted(t, "a { color: #aabcccdd }", "a {\n  color: #aabcccdd;\n}\n")
	expectPrinted(t, "a { color: #aabbcddd }", "a {\n  color: #aabbcddd;\n}\n")
	expectPrinted(t, "a { color: #aabbccde }", "a {\n  color: #aabbccde;\n}\n")

	expectPrintedMangle(t, "a { color: #aabbccdd }", "a {\n  color: #abcd;\n}\n")
	expectPrintedMangle(t, "a { color: #abbbccdd }", "a {\n  color: #abbbccdd;\n}\n")
	expectPrintedMangle(t, "a { color: #aabcccdd }", "a {\n  color: #aabcccdd;\n}\n")
	expectPrintedMangle(t, "a { color: #aabbcddd }", "a {\n  color: #aabbcddd;\n}\n")
	expectPrintedMangle(t, "a { color: #aabbccde }", "a {\n  color: #aabbccde;\n}\n")

	expectPrinted(t, "a { color: #AABBCCDD }", "a {\n  color: #AABBCCDD;\n}\n")
	expectPrinted(t, "a { color: #ABBBCCDD }", "a {\n  color: #ABBBCCDD;\n}\n")
	expectPrinted(t, "a { color: #AABCCCDD }", "a {\n  color: #AABCCCDD;\n}\n")
	expectPrinted(t, "a { color: #AABBCDDD }", "a {\n  color: #AABBCDDD;\n}\n")
	expectPrinted(t, "a { color: #AABBCCDE }", "a {\n  color: #AABBCCDE;\n}\n")

	expectPrintedMangle(t, "a { color: #AABBCCDD }", "a {\n  color: #abcd;\n}\n")
	expectPrintedMangle(t, "a { color: #ABBBCCDD }", "a {\n  color: #abbbccdd;\n}\n")
	expectPrintedMangle(t, "a { color: #AABCCCDD }", "a {\n  color: #aabcccdd;\n}\n")
	expectPrintedMangle(t, "a { color: #AABBCDDD }", "a {\n  color: #aabbcddd;\n}\n")
	expectPrintedMangle(t, "a { color: #AABBCCDE }", "a {\n  color: #aabbccde;\n}\n")

	// "#RRGGBBFF"

	expectPrinted(t, "a { color: #112233ff }", "a {\n  color: #112233ff;\n}\n")
	expectPrinted(t, "a { color: #122233ff }", "a {\n  color: #122233ff;\n}\n")
	expectPrinted(t, "a { color: #112333ff }", "a {\n  color: #112333ff;\n}\n")
	expectPrinted(t, "a { color: #112234ff }", "a {\n  color: #112234ff;\n}\n")
	expectPrinted(t, "a { color: #112233ef }", "a {\n  color: #112233ef;\n}\n")

	expectPrintedMangle(t, "a { color: #112233ff }", "a {\n  color: #123;\n}\n")
	expectPrintedMangle(t, "a { color: #122233ff }", "a {\n  color: #122233;\n}\n")
	expectPrintedMangle(t, "a { color: #112333ff }", "a {\n  color: #112333;\n}\n")
	expectPrintedMangle(t, "a { color: #112234ff }", "a {\n  color: #112234;\n}\n")
	expectPrintedMangle(t, "a { color: #112233ef }", "a {\n  color: #112233ef;\n}\n")

	expectPrinted(t, "a { color: #aabbccff }", "a {\n  color: #aabbccff;\n}\n")
	expectPrinted(t, "a { color: #abbbccff }", "a {\n  color: #abbbccff;\n}\n")
	expectPrinted(t, "a { color: #aabcccff }", "a {\n  color: #aabcccff;\n}\n")
	expectPrinted(t, "a { color: #aabbcdff }", "a {\n  color: #aabbcdff;\n}\n")
	expectPrinted(t, "a { color: #aabbccef }", "a {\n  color: #aabbccef;\n}\n")

	expectPrintedMangle(t, "a { color: #aabbccff }", "a {\n  color: #abc;\n}\n")
	expectPrintedMangle(t, "a { color: #abbbccff }", "a {\n  color: #abbbcc;\n}\n")
	expectPrintedMangle(t, "a { color: #aabcccff }", "a {\n  color: #aabccc;\n}\n")
	expectPrintedMangle(t, "a { color: #aabbcdff }", "a {\n  color: #aabbcd;\n}\n")
	expectPrintedMangle(t, "a { color: #aabbccef }", "a {\n  color: #aabbccef;\n}\n")

	expectPrinted(t, "a { color: #AABBCCFF }", "a {\n  color: #AABBCCFF;\n}\n")
	expectPrinted(t, "a { color: #ABBBCCFF }", "a {\n  color: #ABBBCCFF;\n}\n")
	expectPrinted(t, "a { color: #AABCCCFF }", "a {\n  color: #AABCCCFF;\n}\n")
	expectPrinted(t, "a { color: #AABBCDFF }", "a {\n  color: #AABBCDFF;\n}\n")
	expectPrinted(t, "a { color: #AABBCCEF }", "a {\n  color: #AABBCCEF;\n}\n")

	expectPrintedMangle(t, "a { color: #AABBCCFF }", "a {\n  color: #abc;\n}\n")
	expectPrintedMangle(t, "a { color: #ABBBCCFF }", "a {\n  color: #abbbcc;\n}\n")
	expectPrintedMangle(t, "a { color: #AABCCCFF }", "a {\n  color: #aabccc;\n}\n")
	expectPrintedMangle(t, "a { color: #AABBCDFF }", "a {\n  color: #aabbcd;\n}\n")
	expectPrintedMangle(t, "a { color: #AABBCCEF }", "a {\n  color: #aabbccef;\n}\n")
}

func TestColorNames(t *testing.T) {
	expectPrinted(t, "a { color: #f00 }", "a {\n  color: #f00;\n}\n")
	expectPrinted(t, "a { color: #f00f }", "a {\n  color: #f00f;\n}\n")
	expectPrinted(t, "a { color: #ff0000 }", "a {\n  color: #ff0000;\n}\n")
	expectPrinted(t, "a { color: #ff0000ff }", "a {\n  color: #ff0000ff;\n}\n")

	expectPrintedMangle(t, "a { color: #f00 }", "a {\n  color: red;\n}\n")
	expectPrintedMangle(t, "a { color: #f00e }", "a {\n  color: #f00e;\n}\n")
	expectPrintedMangle(t, "a { color: #f00f }", "a {\n  color: red;\n}\n")
	expectPrintedMangle(t, "a { color: #ff0000 }", "a {\n  color: red;\n}\n")
	expectPrintedMangle(t, "a { color: #ff0000ef }", "a {\n  color: #ff0000ef;\n}\n")
	expectPrintedMangle(t, "a { color: #ff0000ff }", "a {\n  color: red;\n}\n")
	expectPrintedMangle(t, "a { color: #ffc0cb }", "a {\n  color: pink;\n}\n")
	expectPrintedMangle(t, "a { color: #ffc0cbef }", "a {\n  color: #ffc0cbef;\n}\n")
	expectPrintedMangle(t, "a { color: #ffc0cbff }", "a {\n  color: pink;\n}\n")

	expectPrinted(t, "a { color: white }", "a {\n  color: white;\n}\n")
	expectPrinted(t, "a { color: tUrQuOiSe }", "a {\n  color: tUrQuOiSe;\n}\n")

	expectPrintedMangle(t, "a { color: white }", "a {\n  color: #fff;\n}\n")
	expectPrintedMangle(t, "a { color: tUrQuOiSe }", "a {\n  color: #40e0d0;\n}\n")
}

func TestColorRGBA(t *testing.T) {
	expectPrintedMangle(t, "a { color: rgba(1 2 3 / 0.5) }", "a {\n  color: #01020380;\n}\n")
	expectPrintedMangle(t, "a { color: rgba(1 2 3 / 50%) }", "a {\n  color: #0102037f;\n}\n")
	expectPrintedMangle(t, "a { color: rgba(1, 2, 3, 0.5) }", "a {\n  color: #01020380;\n}\n")
	expectPrintedMangle(t, "a { color: rgba(1, 2, 3, 50%) }", "a {\n  color: #0102037f;\n}\n")
	expectPrintedMangle(t, "a { color: rgba(1% 2% 3% / 0.5) }", "a {\n  color: #03050880;\n}\n")
	expectPrintedMangle(t, "a { color: rgba(1% 2% 3% / 50%) }", "a {\n  color: #0305087f;\n}\n")
	expectPrintedMangle(t, "a { color: rgba(1%, 2%, 3%, 0.5) }", "a {\n  color: #03050880;\n}\n")
	expectPrintedMangle(t, "a { color: rgba(1%, 2%, 3%, 50%) }", "a {\n  color: #0305087f;\n}\n")

	expectPrintedLowerMangle(t, "a { color: rgb(1, 2, 3, 0.4) }", "a {\n  color: rgba(1, 2, 3, .4);\n}\n")
	expectPrintedLowerMangle(t, "a { color: rgba(1, 2, 3, 40%) }", "a {\n  color: rgba(1, 2, 3, .4);\n}\n")

	expectPrintedLowerMangle(t, "a { color: rgb(var(--x) var(--y) var(--z)) }", "a {\n  color: rgb(var(--x) var(--y) var(--z));\n}\n")
}

func TestColorHSLA(t *testing.T) {
	expectPrintedMangle(t, ".red { color: hsl(0, 100%, 50%) }", ".red {\n  color: red;\n}\n")
	expectPrintedMangle(t, ".orange { color: hsl(30deg, 100%, 50%) }", ".orange {\n  color: #ff8000;\n}\n")
	expectPrintedMangle(t, ".yellow { color: hsl(60 100% 50%) }", ".yellow {\n  color: #ff0;\n}\n")
	expectPrintedMangle(t, ".green { color: hsl(120, 100%, 50%) }", ".green {\n  color: #0f0;\n}\n")
	expectPrintedMangle(t, ".cyan { color: hsl(200grad, 100%, 50%) }", ".cyan {\n  color: #0ff;\n}\n")
	expectPrintedMangle(t, ".blue { color: hsl(240, 100%, 50%) }", ".blue {\n  color: #00f;\n}\n")
	expectPrintedMangle(t, ".purple { color: hsl(0.75turn 100% 50%) }", ".purple {\n  color: #7f00ff;\n}\n")
	expectPrintedMangle(t, ".magenta { color: hsl(300, 100%, 50%) }", ".magenta {\n  color: #f0f;\n}\n")

	expectPrintedMangle(t, "a { color: hsl(30 25% 50% / 50%) }", "a {\n  color: #9f80607f;\n}\n")
	expectPrintedMangle(t, "a { color: hsla(30 25% 50% / 50%) }", "a {\n  color: #9f80607f;\n}\n")

	expectPrintedLowerMangle(t, "a { color: hsl(1, 2%, 3%, 0.4) }", "a {\n  color: rgba(8, 8, 7, .4);\n}\n")
	expectPrintedLowerMangle(t, "a { color: hsla(1, 2%, 3%, 40%) }", "a {\n  color: rgba(8, 8, 7, .4);\n}\n")

	expectPrintedLowerMangle(t, "a { color: hsl(var(--x) var(--y) var(--z)) }", "a {\n  color: hsl(var(--x) var(--y) var(--z));\n}\n")
}

func TestLowerColor(t *testing.T) {
	expectPrintedLower(t, "a { color: rebeccapurple }", "a {\n  color: #663399;\n}\n")

	expectPrintedLower(t, "a { color: #0123 }", "a {\n  color: rgba(0, 17, 34, 0.2);\n}\n")
	expectPrintedLower(t, "a { color: #1230 }", "a {\n  color: rgba(17, 34, 51, 0);\n}\n")
	expectPrintedLower(t, "a { color: #1234 }", "a {\n  color: rgba(17, 34, 51, 0.267);\n}\n")
	expectPrintedLower(t, "a { color: #123f }", "a {\n  color: rgba(17, 34, 51, 1);\n}\n")
	expectPrintedLower(t, "a { color: #12345678 }", "a {\n  color: rgba(18, 52, 86, 0.471);\n}\n")
	expectPrintedLower(t, "a { color: #ff00007f }", "a {\n  color: rgba(255, 0, 0, 0.498);\n}\n")

	expectPrintedLower(t, "a { color: rgb(1 2 3) }", "a {\n  color: rgb(1, 2, 3);\n}\n")
	expectPrintedLower(t, "a { color: hsl(1 2% 3%) }", "a {\n  color: hsl(1, 2%, 3%);\n}\n")
	expectPrintedLower(t, "a { color: rgba(1% 2% 3%) }", "a {\n  color: rgb(1%, 2%, 3%);\n}\n")
	expectPrintedLower(t, "a { color: hsla(1deg 2% 3%) }", "a {\n  color: hsl(1, 2%, 3%);\n}\n")

	expectPrintedLower(t, "a { color: hsla(200grad 2% 3%) }", "a {\n  color: hsl(180, 2%, 3%);\n}\n")
	expectPrintedLower(t, "a { color: hsla(6.28319rad 2% 3%) }", "a {\n  color: hsl(360, 2%, 3%);\n}\n")
	expectPrintedLower(t, "a { color: hsla(0.5turn 2% 3%) }", "a {\n  color: hsl(180, 2%, 3%);\n}\n")
	expectPrintedLower(t, "a { color: hsla(+200grad 2% 3%) }", "a {\n  color: hsl(180, 2%, 3%);\n}\n")
	expectPrintedLower(t, "a { color: hsla(-200grad 2% 3%) }", "a {\n  color: hsl(-180, 2%, 3%);\n}\n")

	expectPrintedLower(t, "a { color: rgb(1 2 3 / 4) }", "a {\n  color: rgba(1, 2, 3, 4);\n}\n")
	expectPrintedLower(t, "a { color: rgba(1% 2% 3% / 4%) }", "a {\n  color: rgba(1%, 2%, 3%, 0.04);\n}\n")
	expectPrintedLower(t, "a { color: hsl(1 2% 3% / 4) }", "a {\n  color: hsla(1, 2%, 3%, 4);\n}\n")
	expectPrintedLower(t, "a { color: hsla(1 2% 3% / 4%) }", "a {\n  color: hsla(1, 2%, 3%, 0.04);\n}\n")

	expectPrintedLower(t, "a { color: rgb(1, 2, 3, 4) }", "a {\n  color: rgba(1, 2, 3, 4);\n}\n")
	expectPrintedLower(t, "a { color: rgba(1%, 2%, 3%, 4%) }", "a {\n  color: rgba(1%, 2%, 3%, 0.04);\n}\n")
	expectPrintedLower(t, "a { color: rgb(1%, 2%, 3%, 0.4%) }", "a {\n  color: rgba(1%, 2%, 3%, 0.004);\n}\n")

	expectPrintedLower(t, "a { color: hsl(1, 2%, 3%, 4) }", "a {\n  color: hsla(1, 2%, 3%, 4);\n}\n")
	expectPrintedLower(t, "a { color: hsla(1deg, 2%, 3%, 4%) }", "a {\n  color: hsla(1, 2%, 3%, 0.04);\n}\n")
	expectPrintedLower(t, "a { color: hsl(1deg, 2%, 3%, 0.4%) }", "a {\n  color: hsla(1, 2%, 3%, 0.004);\n}\n")
}

func TestDeclaration(t *testing.T) {
	expectPrinted(t, ".decl {}", ".decl {\n}\n")
	expectPrinted(t, ".decl { a: b }", ".decl {\n  a: b;\n}\n")
	expectPrinted(t, ".decl { a: b; }", ".decl {\n  a: b;\n}\n")
	expectPrinted(t, ".decl { a: b; c: d }", ".decl {\n  a: b;\n  c: d;\n}\n")
	expectPrinted(t, ".decl { a: b; c: d; }", ".decl {\n  a: b;\n  c: d;\n}\n")
	expectParseError(t, ".decl { a { b: c; } }", "<stdin>: warning: Expected \":\" but found \"{\"\n")
	expectPrinted(t, ".decl { & a { b: c; } }", ".decl {\n  & a {\n    b: c;\n  }\n}\n")

	// See http://browserhacks.com/
	expectPrinted(t, ".selector { (;property: value;); }", ".selector {\n  (;property: value;);\n}\n")
	expectPrinted(t, ".selector { [;property: value;]; }", ".selector {\n  [;property: value;];\n}\n")
	expectPrinted(t, ".selector, {}", ".selector, {\n}\n")
	expectPrinted(t, ".selector\\ {}", ".selector\\  {\n}\n")
	expectPrinted(t, ".selector { property: value\\9; }", ".selector {\n  property: value\\\t;\n}\n")
	expectPrinted(t, "@media \\0screen\\,screen\\9 {}", "@media \uFFFDscreen\\,screen\\\t {\n}\n")
}

func TestSelector(t *testing.T) {
	expectPrinted(t, "a{}", "a {\n}\n")
	expectPrinted(t, "a {}", "a {\n}\n")
	expectPrinted(t, "a b {}", "a b {\n}\n")

	expectPrinted(t, "a/**/b {}", "a b {\n}\n")
	expectPrinted(t, "a/**/.b {}", "a.b {\n}\n")
	expectPrinted(t, "a/**/:b {}", "a:b {\n}\n")
	expectPrinted(t, "a/**/[b] {}", "a[b] {\n}\n")
	expectPrinted(t, "a>/**/b {}", "a > b {\n}\n")
	expectPrinted(t, "a+/**/b {}", "a + b {\n}\n")
	expectPrinted(t, "a~/**/b {}", "a ~ b {\n}\n")

	expectPrinted(t, "[b]{}", "[b] {\n}\n")
	expectPrinted(t, "[b] {}", "[b] {\n}\n")
	expectPrinted(t, "a[b] {}", "a[b] {\n}\n")
	expectPrinted(t, "a [b] {}", "a [b] {\n}\n")
	expectParseError(t, "[] {}", "<stdin>: warning: Expected identifier but found \"]\"\n")
	expectParseError(t, "[b {}", "<stdin>: warning: Expected \"]\" but found \"{\"\n")
	expectParseError(t, "[b]] {}", "<stdin>: warning: Unexpected \"]\"\n")
	expectParseError(t, "a[b {}", "<stdin>: warning: Expected \"]\" but found \"{\"\n")
	expectParseError(t, "a[b]] {}", "<stdin>: warning: Unexpected \"]\"\n")

	expectPrinted(t, "[|b]{}", "[b] {\n}\n") // "[|b]" is equivalent to "[b]"
	expectPrinted(t, "[*|b]{}", "[*|b] {\n}\n")
	expectPrinted(t, "[a|b]{}", "[a|b] {\n}\n")
	expectPrinted(t, "[a|b|=\"c\"]{}", "[a|b|=c] {\n}\n")
	expectPrinted(t, "[a|b |= \"c\"]{}", "[a|b|=c] {\n}\n")
	expectParseError(t, "[a||b] {}", "<stdin>: warning: Expected identifier but found \"|\"\n")
	expectParseError(t, "[* | b] {}", "<stdin>: warning: Expected \"|\" but found whitespace\n")
	expectParseError(t, "[a | b] {}", "<stdin>: warning: Expected \"=\" but found whitespace\n")

	expectPrinted(t, "[b=\"c\"] {}", "[b=c] {\n}\n")
	expectPrinted(t, "[b=\"c d\"] {}", "[b=\"c d\"] {\n}\n")
	expectPrinted(t, "[b=\"0c\"] {}", "[b=\"0c\"] {\n}\n")
	expectPrinted(t, "[b~=\"c\"] {}", "[b~=c] {\n}\n")
	expectPrinted(t, "[b^=\"c\"] {}", "[b^=c] {\n}\n")
	expectPrinted(t, "[b$=\"c\"] {}", "[b$=c] {\n}\n")
	expectPrinted(t, "[b*=\"c\"] {}", "[b*=c] {\n}\n")
	expectPrinted(t, "[b|=\"c\"] {}", "[b|=c] {\n}\n")
	expectParseError(t, "[b?=\"c\"] {}", "<stdin>: warning: Expected \"]\" but found \"?\"\n")

	expectPrinted(t, "[b = \"c\"] {}", "[b=c] {\n}\n")
	expectPrinted(t, "[b ~= \"c\"] {}", "[b~=c] {\n}\n")
	expectPrinted(t, "[b ^= \"c\"] {}", "[b^=c] {\n}\n")
	expectPrinted(t, "[b $= \"c\"] {}", "[b$=c] {\n}\n")
	expectPrinted(t, "[b *= \"c\"] {}", "[b*=c] {\n}\n")
	expectPrinted(t, "[b |= \"c\"] {}", "[b|=c] {\n}\n")
	expectParseError(t, "[b ?= \"c\"] {}", "<stdin>: warning: Expected \"]\" but found \"?\"\n")

	expectPrinted(t, "[b = \"c\" i] {}", "[b=c i] {\n}\n")
	expectPrinted(t, "[b = \"c\" I] {}", "[b=c I] {\n}\n")
	expectPrinted(t, "[b = \"c\" s] {}", "[b=c s] {\n}\n")
	expectPrinted(t, "[b = \"c\" S] {}", "[b=c S] {\n}\n")
	expectParseError(t, "[b i] {}", "<stdin>: warning: Expected \"]\" but found \"i\"\n<stdin>: warning: Unexpected \"]\"\n")
	expectParseError(t, "[b I] {}", "<stdin>: warning: Expected \"]\" but found \"I\"\n<stdin>: warning: Unexpected \"]\"\n")
	expectParseError(t, "[b s] {}", "<stdin>: warning: Expected \"]\" but found \"s\"\n<stdin>: warning: Unexpected \"]\"\n")
	expectParseError(t, "[b S] {}", "<stdin>: warning: Expected \"]\" but found \"S\"\n<stdin>: warning: Unexpected \"]\"\n")

	expectPrinted(t, "|b {}", "|b {\n}\n")
	expectPrinted(t, "|* {}", "|* {\n}\n")
	expectPrinted(t, "a|b {}", "a|b {\n}\n")
	expectPrinted(t, "a|* {}", "a|* {\n}\n")
	expectPrinted(t, "*|b {}", "*|b {\n}\n")
	expectPrinted(t, "*|* {}", "*|* {\n}\n")
	expectParseError(t, "a||b {}", "<stdin>: warning: Expected identifier but found \"|\"\n")

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
	expectPrinted(t, "a: b {}", "a: b {\n}\n")

	expectPrinted(t, "#id {}", "#id {\n}\n")
	expectPrinted(t, "#--0 {}", "#--0 {\n}\n")
	expectPrinted(t, "#\\-0 {}", "#\\-0 {\n}\n")
	expectPrinted(t, "#\\30 {}", "#\\30  {\n}\n")
	expectPrinted(t, "div#id {}", "div#id {\n}\n")
	expectPrinted(t, "div#--0 {}", "div#--0 {\n}\n")
	expectPrinted(t, "div#\\-0 {}", "div#\\-0 {\n}\n")
	expectPrinted(t, "div#\\30 {}", "div#\\30  {\n}\n")
	expectParseError(t, "#0 {}", "<stdin>: warning: Unexpected \"#0\"\n")
	expectParseError(t, "#-0 {}", "<stdin>: warning: Unexpected \"#-0\"\n")
	expectParseError(t, "div#0 {}", "<stdin>: warning: Unexpected \"#0\"\n")
	expectParseError(t, "div#-0 {}", "<stdin>: warning: Unexpected \"#-0\"\n")

	expectPrinted(t, "div::before::after::selection::first-line::first-letter {color:red}",
		"div::before::after::selection::first-line::first-letter {\n  color: red;\n}\n")
	expectPrintedMangle(t, "div::before::after::selection::first-line::first-letter {color:red}",
		"div:before:after::selection:first-line:first-letter {\n  color: red;\n}\n")

	// Make sure '-' and '\\' consume an ident-like token instead of a name
	expectParseError(t, "_:-ms-lang(x) {}", "")
	expectParseError(t, "_:\\ms-lang(x) {}", "")
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

func TestBadQualifiedRules(t *testing.T) {
	expectParseError(t, "$bad: rule;", "<stdin>: warning: Unexpected \"$\"\n")
	expectParseError(t, "$bad { color: red }", "<stdin>: warning: Unexpected \"$\"\n")
	expectParseError(t, "a { div.major { color: blue } color: red }", "<stdin>: warning: Expected \":\" but found \".\"\n")
	expectParseError(t, "a { div:hover { color: blue } color: red }", "<stdin>: warning: Expected \";\"\n")
	expectParseError(t, "a { div:hover { color: blue }; color: red }", "")
	expectParseError(t, "a { div:hover { color: blue } ; color: red }", "")
}

func TestAtRule(t *testing.T) {
	expectPrinted(t, "@unknown", "@unknown;\n")
	expectPrinted(t, "@unknown;", "@unknown;\n")
	expectPrinted(t, "@unknown{}", "@unknown {}\n")
	expectPrinted(t, "@unknown x;", "@unknown x;\n")
	expectPrinted(t, "@unknown{\na: b;\nc: d;\n}", "@unknown { a: b; c: d; }\n")

	expectParseError(t, "@unknown", "<stdin>: warning: Expected \"{\" but found end of file\n")
	expectParseError(t, "@", "<stdin>: warning: Unexpected \"@\"\n")
	expectParseError(t, "@;", "<stdin>: warning: Unexpected \"@\"\n")
	expectParseError(t, "@{}", "<stdin>: warning: Unexpected \"@\"\n")

	expectPrinted(t, "@viewport { width: 100vw }", "@viewport {\n  width: 100vw;\n}\n")
	expectPrinted(t, "@-ms-viewport { width: 100vw }", "@-ms-viewport {\n  width: 100vw;\n}\n")

	expectPrinted(t, "@document url(\"https://www.example.com/\") { h1 { color: green } }",
		"@document url(https://www.example.com/) {\n  h1 {\n    color: green;\n  }\n}\n")
	expectPrinted(t, "@-moz-document url-prefix() { h1 { color: green } }",
		"@-moz-document url-prefix() {\n  h1 {\n    color: green;\n  }\n}\n")

	// https://www.w3.org/TR/css-page-3/#syntax-page-selector
	expectPrinted(t, `
		@page :first { margin: 0 }
		@page {
			@top-left-corner { content: 'tlc' }
			@top-left { content: 'tl' }
			@top-center { content: 'tc' }
			@top-right { content: 'tr' }
			@top-right-corner { content: 'trc' }
			@bottom-left-corner { content: 'blc' }
			@bottom-left { content: 'bl' }
			@bottom-center { content: 'bc' }
			@bottom-right { content: 'br' }
			@bottom-right-corner { content: 'brc' }
			@left-top { content: 'lt' }
			@left-middle { content: 'lm' }
			@left-bottom { content: 'lb' }
			@right-top { content: 'rt' }
			@right-middle { content: 'rm' }
			@right-bottom { content: 'rb' }
		}
	`, `@page :first {
  margin: 0;
}
@page {
  @top-left-corner {
    content: "tlc";
  }
  @top-left {
    content: "tl";
  }
  @top-center {
    content: "tc";
  }
  @top-right {
    content: "tr";
  }
  @top-right-corner {
    content: "trc";
  }
  @bottom-left-corner {
    content: "blc";
  }
  @bottom-left {
    content: "bl";
  }
  @bottom-center {
    content: "bc";
  }
  @bottom-right {
    content: "br";
  }
  @bottom-right-corner {
    content: "brc";
  }
  @left-top {
    content: "lt";
  }
  @left-middle {
    content: "lm";
  }
  @left-bottom {
    content: "lb";
  }
  @right-top {
    content: "rt";
  }
  @right-middle {
    content: "rm";
  }
  @right-bottom {
    content: "rb";
  }
}
`)
}

func TestAtCharset(t *testing.T) {
	expectPrinted(t, "@charset \"UTF-8\";", "@charset \"UTF-8\";\n")
	expectPrinted(t, "@charset 'UTF-8';", "@charset \"UTF-8\";\n")

	expectParseError(t, "@charset \"utf-8\";", "")
	expectParseError(t, "@charset \"Utf-8\";", "")
	expectParseError(t, "@charset \"UTF-8\";", "")
	expectParseError(t, "@charset \"US-ASCII\";", "<stdin>: warning: \"UTF-8\" will be used instead of unsupported charset \"US-ASCII\"\n")
	expectParseError(t, "@charset;", "<stdin>: warning: Expected whitespace but found \";\"\n")
	expectParseError(t, "@charset ;", "<stdin>: warning: Expected string token but found \";\"\n")
	expectParseError(t, "@charset\"UTF-8\";", "<stdin>: warning: Expected whitespace but found \"\\\"UTF-8\\\"\"\n")
	expectParseError(t, "@charset \"UTF-8\"", "<stdin>: warning: Expected \";\" but found end of file\n")
	expectParseError(t, "@charset url(UTF-8);", "<stdin>: warning: Expected string token but found \"url(UTF-8)\"\n")
	expectParseError(t, "@charset url(\"UTF-8\");", "<stdin>: warning: Expected string token but found \"url(\"\n")
	expectParseError(t, "@charset \"UTF-8\" ", "<stdin>: warning: Expected \";\" but found whitespace\n")
	expectParseError(t, "@charset \"UTF-8\"{}", "<stdin>: warning: Expected \";\" but found \"{\"\n")
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
	expectPrinted(t, "@import url(\"foo.css\") print;", "@import \"foo.css\" print;\n")
	expectPrinted(t, "@import url(\"foo.css\") screen and (orientation:landscape);", "@import \"foo.css\" screen and (orientation:landscape);\n")

	expectParseError(t, "@import;", "<stdin>: warning: Expected URL token but found \";\"\n")
	expectParseError(t, "@import ;", "<stdin>: warning: Expected URL token but found \";\"\n")
	expectParseError(t, "@import \"foo.css\"", "<stdin>: warning: Expected \";\" but found end of file\n")
	expectParseError(t, "@import url(\"foo.css\";", "<stdin>: warning: Expected \")\" but found \";\"\n")
	expectParseError(t, "@import noturl(\"foo.css\");", "<stdin>: warning: Expected URL token but found \"noturl(\"\n")
	expectParseError(t, "@import url(", `<stdin>: warning: Expected URL token but found bad URL token
<stdin>: error: Expected ")" to end URL token
<stdin>: warning: Expected ";" but found end of file
`)

	expectParseError(t, "@import \"foo.css\" {}", "<stdin>: warning: Expected \";\" but found end of file\n")
}

func TestAtKeyframes(t *testing.T) {
	expectPrinted(t, "@keyframes {}", "@keyframes \"\" {\n}\n")
	expectPrinted(t, "@keyframes name{}", "@keyframes name {\n}\n")
	expectPrinted(t, "@keyframes name {}", "@keyframes name {\n}\n")
	expectPrinted(t, "@keyframes name{0%,50%{color:red}25%,75%{color:blue}}",
		"@keyframes name {\n  0%, 50% {\n    color: red;\n  }\n  25%, 75% {\n    color: blue;\n  }\n}\n")
	expectPrinted(t, "@keyframes name { 0%, 50% { color: red } 25%, 75% { color: blue } }",
		"@keyframes name {\n  0%, 50% {\n    color: red;\n  }\n  25%, 75% {\n    color: blue;\n  }\n}\n")
	expectPrinted(t, "@keyframes name{from{color:red}to{color:blue}}",
		"@keyframes name {\n  from {\n    color: red;\n  }\n  to {\n    color: blue;\n  }\n}\n")
	expectPrinted(t, "@keyframes name { from { color: red } to { color: blue } }",
		"@keyframes name {\n  from {\n    color: red;\n  }\n  to {\n    color: blue;\n  }\n}\n")

	expectPrinted(t, "@keyframes name { from { color: red } }", "@keyframes name {\n  from {\n    color: red;\n  }\n}\n")
	expectPrinted(t, "@keyframes name { 100% { color: red } }", "@keyframes name {\n  100% {\n    color: red;\n  }\n}\n")
	expectPrintedMangle(t, "@keyframes name { from { color: red } }", "@keyframes name {\n  0% {\n    color: red;\n  }\n}\n")
	expectPrintedMangle(t, "@keyframes name { 100% { color: red } }", "@keyframes name {\n  to {\n    color: red;\n  }\n}\n")

	expectPrinted(t, "@-webkit-keyframes name {}", "@-webkit-keyframes name {\n}\n")
	expectPrinted(t, "@-moz-keyframes name {}", "@-moz-keyframes name {\n}\n")
	expectPrinted(t, "@-ms-keyframes name {}", "@-ms-keyframes name {\n}\n")
	expectPrinted(t, "@-o-keyframes name {}", "@-o-keyframes name {\n}\n")

	expectParseError(t, "@keyframes {}", "<stdin>: warning: Expected identifier but found \"{\"\n")
	expectParseError(t, "@keyframes 'name' {}", "<stdin>: warning: Expected identifier but found \"'name'\"\n")
	expectParseError(t, "@keyframes name { 0% 100% {} }", "<stdin>: warning: Expected \",\" but found \"100%\"\n")
	expectParseError(t, "@keyframes name { {} 0% {} }", "<stdin>: warning: Expected percentage but found \"{\"\n")
	expectParseError(t, "@keyframes name { 100 {} }", "<stdin>: warning: Expected percentage but found \"100\"\n")
	expectParseError(t, "@keyframes name { into {} }", "<stdin>: warning: Expected percentage but found \"into\"\n")
	expectParseError(t, "@keyframes name { 1,2 {} }", "<stdin>: warning: Expected percentage but found \"1\"\n<stdin>: warning: Expected percentage but found \"2\"\n")
	expectParseError(t, "@keyframes name { 1, 2 {} }", "<stdin>: warning: Expected percentage but found \"1\"\n<stdin>: warning: Expected percentage but found \"2\"\n")
	expectParseError(t, "@keyframes name { 1 ,2 {} }", "<stdin>: warning: Expected percentage but found \"1\"\n<stdin>: warning: Expected percentage but found \"2\"\n")
	expectParseError(t, "@keyframes name { 1%,,2% {} }", "<stdin>: warning: Expected percentage but found \",\"\n")
}

func TestAtRuleValidation(t *testing.T) {
	expectParseError(t, "a {} @charset \"UTF-8\";",
		"<stdin>: warning: \"@charset\" must be the first rule in the file\n"+
			"<stdin>: note: This rule cannot come before a \"@charset\" rule\n")

	expectParseError(t, "a {} @import \"foo\";",
		"<stdin>: warning: All \"@import\" rules must come first\n"+
			"<stdin>: note: This rule cannot come before an \"@import\" rule\n")
}

func TestEmptyRule(t *testing.T) {
	expectPrinted(t, "div {}", "div {\n}\n")
	expectPrinted(t, "@media screen {}", "@media screen {\n}\n")
	expectPrinted(t, "@page { @top-left {} }", "@page {\n  @top-left {\n  }\n}\n")
	expectPrinted(t, "@keyframes test { from {} to {} }", "@keyframes test {\n  from {\n  }\n  to {\n  }\n}\n")

	expectPrintedMangle(t, "div {}", "")
	expectPrintedMangle(t, "@media screen {}", "")
	expectPrintedMangle(t, "@page { @top-left {} }", "")
	expectPrintedMangle(t, "@keyframes test { from {} to {} }", "")

	expectPrinted(t, "$invalid {}", "$invalid {\n}\n")
	expectPrinted(t, "@page { color: red; @top-left {} }", "@page {\n  color: red;\n  @top-left {\n  }\n}\n")
	expectPrinted(t, "@keyframes test { from {} to { color: red } }", "@keyframes test {\n  from {\n  }\n  to {\n    color: red;\n  }\n}\n")
	expectPrinted(t, "@keyframes test { from { color: red } to {} }", "@keyframes test {\n  from {\n    color: red;\n  }\n  to {\n  }\n}\n")

	expectPrintedMangle(t, "$invalid {}", "$invalid {\n}\n")
	expectPrintedMangle(t, "@page { color: red; @top-left {} }", "@page {\n  color: red;\n}\n")
	expectPrintedMangle(t, "@keyframes test { from {} to { color: red } }", "@keyframes test {\n  to {\n    color: red;\n  }\n}\n")
	expectPrintedMangle(t, "@keyframes test { from { color: red } to {} }", "@keyframes test {\n  0% {\n    color: red;\n  }\n}\n")

	expectPrintedMangleMinify(t, "$invalid {}", "$invalid{}")
	expectPrintedMangleMinify(t, "@page { color: red; @top-left {} }", "@page{color:red}")
	expectPrintedMangleMinify(t, "@keyframes test { from {} to { color: red } }", "@keyframes test{to{color:red}}")
	expectPrintedMangleMinify(t, "@keyframes test { from { color: red } to {} }", "@keyframes test{0%{color:red}}")
}

func TestMarginAndPadding(t *testing.T) {
	for _, x := range []string{"margin", "padding"} {
		expectPrinted(t, "a { "+x+": 0 1 0 1 }", "a {\n  "+x+": 0 1 0 1;\n}\n")
		expectPrinted(t, "a { "+x+": 0 1 0px 1px }", "a {\n  "+x+": 0 1 0px 1px;\n}\n")

		expectPrintedMangle(t, "a { "+x+"-top: 0px }", "a {\n  "+x+"-top: 0;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-right: 0px }", "a {\n  "+x+"-right: 0;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-bottom: 0px }", "a {\n  "+x+"-bottom: 0;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-left: 0px }", "a {\n  "+x+"-left: 0;\n}\n")

		expectPrintedMangle(t, "a { "+x+"-top: 1px }", "a {\n  "+x+"-top: 1px;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-right: 1px }", "a {\n  "+x+"-right: 1px;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-bottom: 1px }", "a {\n  "+x+"-bottom: 1px;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-left: 1px }", "a {\n  "+x+"-left: 1px;\n}\n")

		expectPrintedMangle(t, "a { "+x+": 0 1 0 0 }", "a {\n  "+x+": 0 1 0 0;\n}\n")
		expectPrintedMangle(t, "a { "+x+": 0 1 2 1 }", "a {\n  "+x+": 0 1 2;\n}\n")
		expectPrintedMangle(t, "a { "+x+": 0 1 0 1 }", "a {\n  "+x+": 0 1;\n}\n")
		expectPrintedMangle(t, "a { "+x+": 0 0 0 0 }", "a {\n  "+x+": 0;\n}\n")
		expectPrintedMangle(t, "a { "+x+": 0 0 0 0 !important }", "a {\n  "+x+": 0 !important;\n}\n")
		expectPrintedMangle(t, "a { "+x+": 0 1px 0px 1px }", "a {\n  "+x+": 0 1px;\n}\n")
		expectPrintedMangle(t, "a { "+x+": 0 1 0px 1px }", "a {\n  "+x+": 0 1 0 1px;\n}\n")

		expectPrintedMangle(t, "a { "+x+": 1 2 3 4; "+x+"-top: 5 }", "a {\n  "+x+": 5 2 3 4;\n}\n")
		expectPrintedMangle(t, "a { "+x+": 1 2 3 4; "+x+"-right: 5 }", "a {\n  "+x+": 1 5 3 4;\n}\n")
		expectPrintedMangle(t, "a { "+x+": 1 2 3 4; "+x+"-bottom: 5 }", "a {\n  "+x+": 1 2 5 4;\n}\n")
		expectPrintedMangle(t, "a { "+x+": 1 2 3 4; "+x+"-left: 5 }", "a {\n  "+x+": 1 2 3 5;\n}\n")

		expectPrintedMangle(t, "a { "+x+"-top: 5; "+x+": 1 2 3 4 }", "a {\n  "+x+": 1 2 3 4;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-right: 5; "+x+": 1 2 3 4 }", "a {\n  "+x+": 1 2 3 4;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-bottom: 5; "+x+": 1 2 3 4 }", "a {\n  "+x+": 1 2 3 4;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-left: 5; "+x+": 1 2 3 4 }", "a {\n  "+x+": 1 2 3 4;\n}\n")

		expectPrintedMangle(t, "a { "+x+"-top: 1; "+x+"-top: 2 }", "a {\n  "+x+"-top: 2;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-right: 1; "+x+"-right: 2 }", "a {\n  "+x+"-right: 2;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-bottom: 1; "+x+"-bottom: 2 }", "a {\n  "+x+"-bottom: 2;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-left: 1; "+x+"-left: 2 }", "a {\n  "+x+"-left: 2;\n}\n")

		expectPrintedMangle(t, "a { "+x+": 1; "+x+": 2 !important }",
			"a {\n  "+x+": 1;\n  "+x+": 2 !important;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-top: 1; "+x+"-top: 2 !important }",
			"a {\n  "+x+"-top: 1;\n  "+x+"-top: 2 !important;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-right: 1; "+x+"-right: 2 !important }",
			"a {\n  "+x+"-right: 1;\n  "+x+"-right: 2 !important;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-bottom: 1; "+x+"-bottom: 2 !important }",
			"a {\n  "+x+"-bottom: 1;\n  "+x+"-bottom: 2 !important;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-left: 1; "+x+"-left: 2 !important }",
			"a {\n  "+x+"-left: 1;\n  "+x+"-left: 2 !important;\n}\n")

		expectPrintedMangle(t, "a { "+x+": 1 !important; "+x+": 2 }",
			"a {\n  "+x+": 1 !important;\n  "+x+": 2;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-top: 1 !important; "+x+"-top: 2 }",
			"a {\n  "+x+"-top: 1 !important;\n  "+x+"-top: 2;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-right: 1 !important; "+x+"-right: 2 }",
			"a {\n  "+x+"-right: 1 !important;\n  "+x+"-right: 2;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-bottom: 1 !important; "+x+"-bottom: 2 }",
			"a {\n  "+x+"-bottom: 1 !important;\n  "+x+"-bottom: 2;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-left: 1 !important; "+x+"-left: 2 }",
			"a {\n  "+x+"-left: 1 !important;\n  "+x+"-left: 2;\n}\n")

		expectPrintedMangle(t, "a { "+x+"-top: 1; "+x+"-top: }", "a {\n  "+x+"-top: 1;\n  "+x+"-top:;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-top: 1; "+x+"-top: 2 3 }", "a {\n  "+x+"-top: 1;\n  "+x+"-top: 2 3;\n}\n")
		expectPrintedMangle(t, "a { "+x+": 1 2 3 4; "+x+"-left: -4; "+x+"-right: -2 }", "a {\n  "+x+": 1 -2 3 -4;\n}\n")
		expectPrintedMangle(t, "a { "+x+": 1 auto 3 4; "+x+"-left: auto }", "a {\n  "+x+": 1 auto 3;\n}\n")
		expectPrintedMangle(t, "a { "+x+": 1 2; "+x+"-top: 5 }", "a {\n  "+x+": 5 2 1;\n}\n")
		expectPrintedMangle(t, "a { "+x+": 1; "+x+"-top: 5 }", "a {\n  "+x+": 5 1 1;\n}\n")

		// This doesn't collapse because if the "calc" has an error it
		// will be ignored and the original rule will show through
		expectPrintedMangle(t, "a { "+x+": 1 2 3 4; "+x+"-right: calc(1 + var(--x)) }", "a {\n  "+x+": 1 2 3 4;\n  "+x+"-right: calc(1 + var(--x));\n}\n")

		expectPrintedMangle(t, "a { "+x+"-left: 1; "+x+"-right: 2; "+x+"-top: 3; "+x+"-bottom: 4 }", "a {\n  "+x+": 3 2 4 1;\n}\n")
		expectPrintedMangle(t, "a { "+x+": 1 2 3 4; "+x+"-right: 5 !important }",
			"a {\n  "+x+": 1 2 3 4;\n  "+x+"-right: 5 !important;\n}\n")
		expectPrintedMangle(t, "a { "+x+": 1 2 3 4 !important; "+x+"-right: 5 }",
			"a {\n  "+x+": 1 2 3 4 !important;\n  "+x+"-right: 5;\n}\n")
		expectPrintedMangle(t, "a { "+x+"-left: 1 !important; "+x+"-right: 2; "+x+"-top: 3 !important; "+x+"-bottom: 4 }",
			"a {\n  "+x+"-left: 1 !important;\n  "+x+"-right: 2;\n  "+x+"-top: 3 !important;\n  "+x+"-bottom: 4;\n}\n")

		// This should not be changed because "--x" and "--z" could be empty
		expectPrintedMangle(t, "a { "+x+": var(--x) var(--y) var(--z) var(--y) }", "a {\n  "+x+": var(--x) var(--y) var(--z) var(--y);\n}\n")
	}
}

func TestBorderRadius(t *testing.T) {
	expectPrinted(t, "a { border-top-left-radius: 0 0 }", "a {\n  border-top-left-radius: 0 0;\n}\n")
	expectPrintedMangle(t, "a { border-top-left-radius: 0 0 }", "a {\n  border-top-left-radius: 0;\n}\n")
	expectPrintedMangle(t, "a { border-top-left-radius: 0 0px }", "a {\n  border-top-left-radius: 0;\n}\n")
	expectPrintedMangle(t, "a { border-top-left-radius: 0 1 }", "a {\n  border-top-left-radius: 0 1;\n}\n")

	expectPrintedMangle(t, "a { border-top-left-radius: 0; border-radius: 1 }", "a {\n  border-radius: 1;\n}\n")
	expectPrintedMangle(t, "a { border-top-left-radius: 0; border-radius: inherit }", "a {\n  border-radius: inherit;\n}\n")

	expectPrintedMangle(t, "a { border-radius: 1 2 3 4 }", "a {\n  border-radius: 1 2 3 4;\n}\n")
	expectPrintedMangle(t, "a { border-radius: 1 2 1 3 }", "a {\n  border-radius: 1 2 1 3;\n}\n")
	expectPrintedMangle(t, "a { border-radius: 1 2 3 2 }", "a {\n  border-radius: 1 2 3;\n}\n")
	expectPrintedMangle(t, "a { border-radius: 1 2 1 2 }", "a {\n  border-radius: 1 2;\n}\n")
	expectPrintedMangle(t, "a { border-radius: 1 1 1 1 }", "a {\n  border-radius: 1;\n}\n")

	expectPrintedMangle(t, "a { border-radius: 0/1 2 3 4 }", "a {\n  border-radius: 0 / 1 2 3 4;\n}\n")
	expectPrintedMangle(t, "a { border-radius: 0/1 2 1 3 }", "a {\n  border-radius: 0 / 1 2 1 3;\n}\n")
	expectPrintedMangle(t, "a { border-radius: 0/1 2 3 2 }", "a {\n  border-radius: 0 / 1 2 3;\n}\n")
	expectPrintedMangle(t, "a { border-radius: 0/1 2 1 2 }", "a {\n  border-radius: 0 / 1 2;\n}\n")
	expectPrintedMangle(t, "a { border-radius: 0/1 1 1 1 }", "a {\n  border-radius: 0 / 1;\n}\n")

	expectPrintedMangle(t, "a { border-radius: 1 2; border-top-left-radius: 3; }", "a {\n  border-radius: 3 2 1;\n}\n")
	expectPrintedMangle(t, "a { border-radius: 1; border-top-left-radius: 3; }", "a {\n  border-radius: 3 1 1;\n}\n")
	expectPrintedMangle(t, "a { border-radius: 0/1; border-top-left-radius: 3; }", "a {\n  border-radius: 3 0 0 / 3 1 1;\n}\n")
	expectPrintedMangle(t, "a { border-radius: 0/1 2; border-top-left-radius: 3; }", "a {\n  border-radius: 3 0 0 / 3 2 1;\n}\n")

	for _, x := range []string{"", "-top-left", "-top-right", "-bottom-left", "-bottom-right"} {
		y := "border" + x + "-radius"
		expectPrintedMangle(t, "a { "+y+": 1; "+y+": 2 }",
			"a {\n  "+y+": 2;\n}\n")
		expectPrintedMangle(t, "a { "+y+": 1 !important; "+y+": 2 }",
			"a {\n  "+y+": 1 !important;\n  "+y+": 2;\n}\n")
		expectPrintedMangle(t, "a { "+y+": 1; "+y+": 2 !important }",
			"a {\n  "+y+": 1;\n  "+y+": 2 !important;\n}\n")
		expectPrintedMangle(t, "a { "+y+": 1 !important; "+y+": 2 !important }",
			"a {\n  "+y+": 2 !important;\n}\n")

		expectPrintedMangle(t, "a { border-radius: 1; "+y+": 2 !important; }",
			"a {\n  border-radius: 1;\n  "+y+": 2 !important;\n}\n")
		expectPrintedMangle(t, "a { border-radius: 1 !important; "+y+": 2; }",
			"a {\n  border-radius: 1 !important;\n  "+y+": 2;\n}\n")
	}

	expectPrintedMangle(t, "a { border-top-left-radius: ; border-radius: 1 }",
		"a {\n  border-top-left-radius:;\n  border-radius: 1;\n}\n")
	expectPrintedMangle(t, "a { border-top-left-radius: 1; border-radius: / }",
		"a {\n  border-top-left-radius: 1;\n  border-radius: /;\n}\n")

	expectPrintedMangleMinify(t, "a { border-radius: 1 2 3 4; border-top-right-radius: 5; }", "a{border-radius:1 5 3 4}")
	expectPrintedMangleMinify(t, "a { border-radius: 1 2 3 4; border-top-right-radius: 5 6; }", "a{border-radius:1 5 3 4/1 6 3 4}")

	// These should not be changed because "--x" and "--z" could be empty
	expectPrintedMangle(t, "a { border-radius: var(--x) var(--y) var(--z) var(--y) }", "a {\n  border-radius: var(--x) var(--y) var(--z) var(--y);\n}\n")
	expectPrintedMangle(t, "a { border-radius: 0 / var(--x) var(--y) var(--z) var(--y) }", "a {\n  border-radius: 0 / var(--x) var(--y) var(--z) var(--y);\n}\n")
}

func TestBoxShadow(t *testing.T) {
	expectPrinted(t, "a { box-shadow: inset 0px 0px 0px 0px black }", "a {\n  box-shadow: inset 0px 0px 0px 0px black;\n}\n")
	expectPrintedMangle(t, "a { box-shadow: 0px 0px 0px 0px inset black }", "a {\n  box-shadow: 0 0 inset #000;\n}\n")
	expectPrintedMangle(t, "a { box-shadow: 0px 0px 0px 0px black inset }", "a {\n  box-shadow: 0 0 #000 inset;\n}\n")
	expectPrintedMangle(t, "a { box-shadow: black 0px 0px 0px 0px inset }", "a {\n  box-shadow: #000 0 0 inset;\n}\n")
	expectPrintedMangle(t, "a { box-shadow: inset 0px 0px 0px 0px black }", "a {\n  box-shadow: inset 0 0 #000;\n}\n")
	expectPrintedMangle(t, "a { box-shadow: inset black 0px 0px 0px 0px }", "a {\n  box-shadow: inset #000 0 0;\n}\n")
	expectPrintedMangle(t, "a { box-shadow: black inset 0px 0px 0px 0px }", "a {\n  box-shadow: #000 inset 0 0;\n}\n")
	expectPrintedMangle(t, "a { box-shadow: yellow 1px 0px 0px 1px inset }", "a {\n  box-shadow: #ff0 1px 0 0 1px inset;\n}\n")
	expectPrintedMangle(t, "a { box-shadow: yellow 1px 0px 1px 0px inset }", "a {\n  box-shadow: #ff0 1px 0 1px inset;\n}\n")
	expectPrintedMangle(t, "a { box-shadow: rebeccapurple, yellow, black }", "a {\n  box-shadow:\n    #639,\n    #ff0,\n    #000;\n}\n")
	expectPrintedMangleMinify(t, "a { box-shadow: rebeccapurple , yellow , black }", "a{box-shadow:#639,#ff0,#000}")
	expectPrintedMangleMinify(t, "a { box-shadow: rgb(255, 0, 17) 0 0 1 inset }", "a{box-shadow:#f01 0 0 1 inset}")
}

func TestDeduplicateRules(t *testing.T) {
	expectPrinted(t, "a { color: red; color: green; color: red }",
		"a {\n  color: red;\n  color: green;\n  color: red;\n}\n")
	expectPrintedMangle(t, "a { color: red; color: green; color: red }",
		"a {\n  color: green;\n  color: red;\n}\n")

	expectPrinted(t, "a { color: red } a { color: green } a { color: red }",
		"a {\n  color: red;\n}\na {\n  color: green;\n}\na {\n  color: red;\n}\n")
	expectPrintedMangle(t, "a { color: red } a { color: green } a { color: red }",
		"a {\n  color: green;\n}\na {\n  color: red;\n}\n")

	expectPrintedMangle(t, "@media screen { a { color: red } } @media screen { a { color: red } }",
		"@media screen {\n  a {\n    color: red;\n  }\n}\n")
	expectPrintedMangle(t, "@media screen { a { color: red } } @media screen { & a { color: red } }",
		"@media screen {\n  a {\n    color: red;\n  }\n}\n@media screen {\n  & a {\n    color: red;\n  }\n}\n")
	expectPrintedMangle(t, "@media screen { a { color: red } } @media screen { a[x] { color: red } }",
		"@media screen {\n  a {\n    color: red;\n  }\n}\n@media screen {\n  a[x] {\n    color: red;\n  }\n}\n")
	expectPrintedMangle(t, "@media screen { a { color: red } } @media screen { a.x { color: red } }",
		"@media screen {\n  a {\n    color: red;\n  }\n}\n@media screen {\n  a.x {\n    color: red;\n  }\n}\n")
	expectPrintedMangle(t, "@media screen { a { color: red } } @media screen { a#x { color: red } }",
		"@media screen {\n  a {\n    color: red;\n  }\n}\n@media screen {\n  a#x {\n    color: red;\n  }\n}\n")
	expectPrintedMangle(t, "@media screen { a { color: red } } @media screen { a:x { color: red } }",
		"@media screen {\n  a {\n    color: red;\n  }\n}\n@media screen {\n  a:x {\n    color: red;\n  }\n}\n")
	expectPrintedMangle(t, "@media screen { a:x { color: red } } @media screen { a:x(y) { color: red } }",
		"@media screen {\n  a:x {\n    color: red;\n  }\n}\n@media screen {\n  a:x(y) {\n    color: red;\n  }\n}\n")
	expectPrintedMangle(t, "@media screen { a b { color: red } } @media screen { a + b { color: red } }",
		"@media screen {\n  a b {\n    color: red;\n  }\n}\n@media screen {\n  a + b {\n    color: red;\n  }\n}\n")
}

func TestMangleTime(t *testing.T) {
	expectPrintedMangle(t, "a { animation: b 1s }", "a {\n  animation: b 1s;\n}\n")
	expectPrintedMangle(t, "a { animation: b 1.s }", "a {\n  animation: b 1s;\n}\n")
	expectPrintedMangle(t, "a { animation: b 1.0s }", "a {\n  animation: b 1s;\n}\n")
	expectPrintedMangle(t, "a { animation: b 1.02s }", "a {\n  animation: b 1.02s;\n}\n")
	expectPrintedMangle(t, "a { animation: b .1s }", "a {\n  animation: b .1s;\n}\n")
	expectPrintedMangle(t, "a { animation: b .01s }", "a {\n  animation: b .01s;\n}\n")
	expectPrintedMangle(t, "a { animation: b .001s }", "a {\n  animation: b 1ms;\n}\n")
	expectPrintedMangle(t, "a { animation: b .0012s }", "a {\n  animation: b 1.2ms;\n}\n")
	expectPrintedMangle(t, "a { animation: b -.001s }", "a {\n  animation: b -1ms;\n}\n")
	expectPrintedMangle(t, "a { animation: b -.0012s }", "a {\n  animation: b -1.2ms;\n}\n")
	expectPrintedMangle(t, "a { animation: b .0001s }", "a {\n  animation: b .1ms;\n}\n")
	expectPrintedMangle(t, "a { animation: b .00012s }", "a {\n  animation: b .12ms;\n}\n")
	expectPrintedMangle(t, "a { animation: b .000123s }", "a {\n  animation: b .123ms;\n}\n")
	expectPrintedMangle(t, "a { animation: b .01S }", "a {\n  animation: b .01S;\n}\n")
	expectPrintedMangle(t, "a { animation: b .001S }", "a {\n  animation: b 1ms;\n}\n")

	expectPrintedMangle(t, "a { animation: b 1ms }", "a {\n  animation: b 1ms;\n}\n")
	expectPrintedMangle(t, "a { animation: b 10ms }", "a {\n  animation: b 10ms;\n}\n")
	expectPrintedMangle(t, "a { animation: b 100ms }", "a {\n  animation: b .1s;\n}\n")
	expectPrintedMangle(t, "a { animation: b 120ms }", "a {\n  animation: b .12s;\n}\n")
	expectPrintedMangle(t, "a { animation: b 123ms }", "a {\n  animation: b 123ms;\n}\n")
	expectPrintedMangle(t, "a { animation: b 1000ms }", "a {\n  animation: b 1s;\n}\n")
	expectPrintedMangle(t, "a { animation: b 1200ms }", "a {\n  animation: b 1.2s;\n}\n")
	expectPrintedMangle(t, "a { animation: b 1230ms }", "a {\n  animation: b 1.23s;\n}\n")
	expectPrintedMangle(t, "a { animation: b 1234ms }", "a {\n  animation: b 1234ms;\n}\n")
	expectPrintedMangle(t, "a { animation: b -100ms }", "a {\n  animation: b -.1s;\n}\n")
	expectPrintedMangle(t, "a { animation: b -120ms }", "a {\n  animation: b -.12s;\n}\n")
	expectPrintedMangle(t, "a { animation: b 120mS }", "a {\n  animation: b .12s;\n}\n")
	expectPrintedMangle(t, "a { animation: b 120Ms }", "a {\n  animation: b .12s;\n}\n")
	expectPrintedMangle(t, "a { animation: b 123mS }", "a {\n  animation: b 123mS;\n}\n")
	expectPrintedMangle(t, "a { animation: b 123Ms }", "a {\n  animation: b 123Ms;\n}\n")

	// Mangling times with exponents is not currently supported
	expectPrintedMangle(t, "a { animation: b 1e3ms }", "a {\n  animation: b 1e3ms;\n}\n")
	expectPrintedMangle(t, "a { animation: b 1E3ms }", "a {\n  animation: b 1E3ms;\n}\n")
}

func TestTransform(t *testing.T) {
	expectPrintedMangle(t, "a { transform: matrix(1, 0, 0, 1, 0, 0) }", "a {\n  transform: scale(1);\n}\n")
	expectPrintedMangle(t, "a { transform: matrix(2, 0, 0, 1, 0, 0) }", "a {\n  transform: scaleX(2);\n}\n")
	expectPrintedMangle(t, "a { transform: matrix(1, 0, 0, 2, 0, 0) }", "a {\n  transform: scaleY(2);\n}\n")
	expectPrintedMangle(t, "a { transform: matrix(2, 0, 0, 3, 0, 0) }", "a {\n  transform: scale(2, 3);\n}\n")
	expectPrintedMangle(t, "a { transform: matrix(2, 0, 0, 2, 0, 0) }", "a {\n  transform: scale(2);\n}\n")
	expectPrintedMangle(t, "a { transform: matrix(1, 0, 0, 1, 1, 2) }", "a {\n  transform: matrix(1, 0, 0, 1, 1, 2);\n}\n")

	expectPrintedMangle(t, "a { transform: translate(0, 0) }", "a {\n  transform: translate(0);\n}\n")
	expectPrintedMangle(t, "a { transform: translate(0px, 0px) }", "a {\n  transform: translate(0);\n}\n")
	expectPrintedMangle(t, "a { transform: translate(0%, 0%) }", "a {\n  transform: translate(0);\n}\n")
	expectPrintedMangle(t, "a { transform: translate(1px, 0) }", "a {\n  transform: translate(1px);\n}\n")
	expectPrintedMangle(t, "a { transform: translate(1px, 0px) }", "a {\n  transform: translate(1px);\n}\n")
	expectPrintedMangle(t, "a { transform: translate(1px, 0%) }", "a {\n  transform: translate(1px);\n}\n")
	expectPrintedMangle(t, "a { transform: translate(0, 1px) }", "a {\n  transform: translateY(1px);\n}\n")
	expectPrintedMangle(t, "a { transform: translate(0px, 1px) }", "a {\n  transform: translateY(1px);\n}\n")
	expectPrintedMangle(t, "a { transform: translate(0%, 1px) }", "a {\n  transform: translateY(1px);\n}\n")
	expectPrintedMangle(t, "a { transform: translate(1px, 2px) }", "a {\n  transform: translate(1px, 2px);\n}\n")
	expectPrintedMangle(t, "a { transform: translate(40%, 60%) }", "a {\n  transform: translate(40%, 60%);\n}\n")

	expectPrintedMangle(t, "a { transform: translateX(0) }", "a {\n  transform: translate(0);\n}\n")
	expectPrintedMangle(t, "a { transform: translateX(0px) }", "a {\n  transform: translate(0);\n}\n")
	expectPrintedMangle(t, "a { transform: translateX(0%) }", "a {\n  transform: translate(0);\n}\n")
	expectPrintedMangle(t, "a { transform: translateX(1px) }", "a {\n  transform: translate(1px);\n}\n")
	expectPrintedMangle(t, "a { transform: translateX(50%) }", "a {\n  transform: translate(50%);\n}\n")

	expectPrintedMangle(t, "a { transform: translateY(0) }", "a {\n  transform: translateY(0);\n}\n")
	expectPrintedMangle(t, "a { transform: translateY(0px) }", "a {\n  transform: translateY(0);\n}\n")
	expectPrintedMangle(t, "a { transform: translateY(0%) }", "a {\n  transform: translateY(0);\n}\n")
	expectPrintedMangle(t, "a { transform: translateY(1px) }", "a {\n  transform: translateY(1px);\n}\n")
	expectPrintedMangle(t, "a { transform: translateY(50%) }", "a {\n  transform: translateY(50%);\n}\n")

	expectPrintedMangle(t, "a { transform: scale(1) }", "a {\n  transform: scale(1);\n}\n")
	expectPrintedMangle(t, "a { transform: scale(100%) }", "a {\n  transform: scale(1);\n}\n")
	expectPrintedMangle(t, "a { transform: scale(10%) }", "a {\n  transform: scale(.1);\n}\n")
	expectPrintedMangle(t, "a { transform: scale(99%) }", "a {\n  transform: scale(99%);\n}\n")
	expectPrintedMangle(t, "a { transform: scale(1, 1) }", "a {\n  transform: scale(1);\n}\n")
	expectPrintedMangle(t, "a { transform: scale(100%, 1) }", "a {\n  transform: scale(1);\n}\n")
	expectPrintedMangle(t, "a { transform: scale(10%, 0.1) }", "a {\n  transform: scale(.1);\n}\n")
	expectPrintedMangle(t, "a { transform: scale(99%, 0.99) }", "a {\n  transform: scale(99%, .99);\n}\n")
	expectPrintedMangle(t, "a { transform: scale(60%, 40%) }", "a {\n  transform: scale(.6, .4);\n}\n")
	expectPrintedMangle(t, "a { transform: scale(3, 1) }", "a {\n  transform: scaleX(3);\n}\n")
	expectPrintedMangle(t, "a { transform: scale(300%, 1) }", "a {\n  transform: scaleX(3);\n}\n")
	expectPrintedMangle(t, "a { transform: scale(1, 3) }", "a {\n  transform: scaleY(3);\n}\n")
	expectPrintedMangle(t, "a { transform: scale(1, 300%) }", "a {\n  transform: scaleY(3);\n}\n")

	expectPrintedMangle(t, "a { transform: scaleX(1) }", "a {\n  transform: scaleX(1);\n}\n")
	expectPrintedMangle(t, "a { transform: scaleX(2) }", "a {\n  transform: scaleX(2);\n}\n")
	expectPrintedMangle(t, "a { transform: scaleX(300%) }", "a {\n  transform: scaleX(3);\n}\n")
	expectPrintedMangle(t, "a { transform: scaleX(99%) }", "a {\n  transform: scaleX(99%);\n}\n")

	expectPrintedMangle(t, "a { transform: scaleY(1) }", "a {\n  transform: scaleY(1);\n}\n")
	expectPrintedMangle(t, "a { transform: scaleY(2) }", "a {\n  transform: scaleY(2);\n}\n")
	expectPrintedMangle(t, "a { transform: scaleY(300%) }", "a {\n  transform: scaleY(3);\n}\n")
	expectPrintedMangle(t, "a { transform: scaleY(99%) }", "a {\n  transform: scaleY(99%);\n}\n")

	expectPrintedMangle(t, "a { transform: rotate(0) }", "a {\n  transform: rotate(0);\n}\n")
	expectPrintedMangle(t, "a { transform: rotate(0deg) }", "a {\n  transform: rotate(0);\n}\n")
	expectPrintedMangle(t, "a { transform: rotate(1deg) }", "a {\n  transform: rotate(1deg);\n}\n")

	expectPrintedMangle(t, "a { transform: skew(0) }", "a {\n  transform: skew(0);\n}\n")
	expectPrintedMangle(t, "a { transform: skew(0deg) }", "a {\n  transform: skew(0);\n}\n")
	expectPrintedMangle(t, "a { transform: skew(1deg) }", "a {\n  transform: skew(1deg);\n}\n")
	expectPrintedMangle(t, "a { transform: skew(1deg, 0) }", "a {\n  transform: skew(1deg);\n}\n")
	expectPrintedMangle(t, "a { transform: skew(1deg, 0deg) }", "a {\n  transform: skew(1deg);\n}\n")
	expectPrintedMangle(t, "a { transform: skew(0, 1deg) }", "a {\n  transform: skew(0, 1deg);\n}\n")
	expectPrintedMangle(t, "a { transform: skew(0deg, 1deg) }", "a {\n  transform: skew(0, 1deg);\n}\n")
	expectPrintedMangle(t, "a { transform: skew(1deg, 2deg) }", "a {\n  transform: skew(1deg, 2deg);\n}\n")

	expectPrintedMangle(t, "a { transform: skewX(0) }", "a {\n  transform: skew(0);\n}\n")
	expectPrintedMangle(t, "a { transform: skewX(0deg) }", "a {\n  transform: skew(0);\n}\n")
	expectPrintedMangle(t, "a { transform: skewX(1deg) }", "a {\n  transform: skew(1deg);\n}\n")

	expectPrintedMangle(t, "a { transform: skewY(0) }", "a {\n  transform: skewY(0);\n}\n")
	expectPrintedMangle(t, "a { transform: skewY(0deg) }", "a {\n  transform: skewY(0);\n}\n")
	expectPrintedMangle(t, "a { transform: skewY(1deg) }", "a {\n  transform: skewY(1deg);\n}\n")

	expectPrintedMangle(t,
		"a { transform: matrix3d(1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 2) }",
		"a {\n  transform: matrix3d(1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 2);\n}\n")
	expectPrintedMangle(t,
		"a { transform: matrix3d(1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 2, 3, 4, 1) }",
		"a {\n  transform: matrix3d(1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 2, 3, 4, 1);\n}\n")
	expectPrintedMangle(t,
		"a { transform: matrix3d(1, 0, 1, 0, 0, 1, 0, 0, 1, 0, 1, 0, 0, 0, 0, 1) }",
		"a {\n  transform: matrix3d(1, 0, 1, 0, 0, 1, 0, 0, 1, 0, 1, 0, 0, 0, 0, 1);\n}\n")
	expectPrintedMangle(t,
		"a { transform: matrix3d(1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1) }",
		"a {\n  transform: scale(1);\n}\n")
	expectPrintedMangle(t,
		"a { transform: matrix3d(2, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1) }",
		"a {\n  transform: scaleX(2);\n}\n")
	expectPrintedMangle(t,
		"a { transform: matrix3d(1, 0, 0, 0, 0, 2, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1) }",
		"a {\n  transform: scaleY(2);\n}\n")
	expectPrintedMangle(t,
		"a { transform: matrix3d(2, 0, 0, 0, 0, 2, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1) }",
		"a {\n  transform: scale(2);\n}\n")
	expectPrintedMangle(t,
		"a { transform: matrix3d(2, 0, 0, 0, 0, 3, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1) }",
		"a {\n  transform: scale(2, 3);\n}\n")
	expectPrintedMangle(t,
		"a { transform: matrix3d(1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 2, 0, 0, 0, 0, 1) }",
		"a {\n  transform: scaleZ(2);\n}\n")
	expectPrintedMangle(t,
		"a { transform: matrix3d(1, 0, 0, 0, 0, 2, 0, 0, 0, 0, 3, 0, 0, 0, 0, 1) }",
		"a {\n  transform: scale3d(1, 2, 3);\n}\n")
	expectPrintedMangle(t,
		"a { transform: matrix3d(2, 3, 0, 0, 4, 5, 0, 0, 0, 0, 1, 0, 6, 7, 0, 1) }",
		"a {\n  transform: matrix(2, 3, 4, 5, 6, 7);\n}\n")

	expectPrintedMangle(t, "a { transform: translate3d(0, 0, 0) }", "a {\n  transform: translate(0);\n}\n")
	expectPrintedMangle(t, "a { transform: translate3d(0%, 0%, 0) }", "a {\n  transform: translate(0);\n}\n")
	expectPrintedMangle(t, "a { transform: translate3d(0px, 0px, 0px) }", "a {\n  transform: translate(0);\n}\n")
	expectPrintedMangle(t, "a { transform: translate3d(1px, 0px, 0px) }", "a {\n  transform: translate(1px);\n}\n")
	expectPrintedMangle(t, "a { transform: translate3d(0px, 1px, 0px) }", "a {\n  transform: translateY(1px);\n}\n")
	expectPrintedMangle(t, "a { transform: translate3d(0px, 0px, 1px) }", "a {\n  transform: translateZ(1px);\n}\n")
	expectPrintedMangle(t, "a { transform: translate3d(1px, 2px, 3px) }", "a {\n  transform: translate3d(1px, 2px, 3px);\n}\n")
	expectPrintedMangle(t, "a { transform: translate3d(1px, 0, 3px) }", "a {\n  transform: translate3d(1px, 0, 3px);\n}\n")
	expectPrintedMangle(t, "a { transform: translate3d(0, 2px, 3px) }", "a {\n  transform: translate3d(0, 2px, 3px);\n}\n")
	expectPrintedMangle(t, "a { transform: translate3d(1px, 2px, 0px) }", "a {\n  transform: translate(1px, 2px);\n}\n")
	expectPrintedMangle(t, "a { transform: translate3d(40%, 60%, 0px) }", "a {\n  transform: translate(40%, 60%);\n}\n")

	expectPrintedMangle(t, "a { transform: translateZ(0) }", "a {\n  transform: translateZ(0);\n}\n")
	expectPrintedMangle(t, "a { transform: translateZ(0px) }", "a {\n  transform: translateZ(0);\n}\n")
	expectPrintedMangle(t, "a { transform: translateZ(1px) }", "a {\n  transform: translateZ(1px);\n}\n")

	expectPrintedMangle(t, "a { transform: scale3d(1, 1, 1) }", "a {\n  transform: scale(1);\n}\n")
	expectPrintedMangle(t, "a { transform: scale3d(2, 1, 1) }", "a {\n  transform: scaleX(2);\n}\n")
	expectPrintedMangle(t, "a { transform: scale3d(1, 2, 1) }", "a {\n  transform: scaleY(2);\n}\n")
	expectPrintedMangle(t, "a { transform: scale3d(1, 1, 2) }", "a {\n  transform: scaleZ(2);\n}\n")
	expectPrintedMangle(t, "a { transform: scale3d(1, 2, 3) }", "a {\n  transform: scale3d(1, 2, 3);\n}\n")
	expectPrintedMangle(t, "a { transform: scale3d(2, 3, 1) }", "a {\n  transform: scale(2, 3);\n}\n")
	expectPrintedMangle(t, "a { transform: scale3d(2, 2, 1) }", "a {\n  transform: scale(2);\n}\n")
	expectPrintedMangle(t, "a { transform: scale3d(3, 300%, 100.00%) }", "a {\n  transform: scale(3);\n}\n")
	expectPrintedMangle(t, "a { transform: scale3d(1%, 2%, 3%) }", "a {\n  transform: scale3d(1%, 2%, 3%);\n}\n")

	expectPrintedMangle(t, "a { transform: scaleZ(1) }", "a {\n  transform: scaleZ(1);\n}\n")
	expectPrintedMangle(t, "a { transform: scaleZ(100%) }", "a {\n  transform: scaleZ(1);\n}\n")
	expectPrintedMangle(t, "a { transform: scaleZ(2) }", "a {\n  transform: scaleZ(2);\n}\n")
	expectPrintedMangle(t, "a { transform: scaleZ(200%) }", "a {\n  transform: scaleZ(2);\n}\n")
	expectPrintedMangle(t, "a { transform: scaleZ(99%) }", "a {\n  transform: scaleZ(99%);\n}\n")

	expectPrintedMangle(t, "a { transform: rotate3d(0, 0, 0, 0) }", "a {\n  transform: rotate3d(0, 0, 0, 0);\n}\n")
	expectPrintedMangle(t, "a { transform: rotate3d(0, 0, 0, 0deg) }", "a {\n  transform: rotate3d(0, 0, 0, 0);\n}\n")
	expectPrintedMangle(t, "a { transform: rotate3d(0, 0, 0, 45deg) }", "a {\n  transform: rotate3d(0, 0, 0, 45deg);\n}\n")
	expectPrintedMangle(t, "a { transform: rotate3d(1, 0, 0, 45deg) }", "a {\n  transform: rotateX(45deg);\n}\n")
	expectPrintedMangle(t, "a { transform: rotate3d(0, 1, 0, 45deg) }", "a {\n  transform: rotateY(45deg);\n}\n")
	expectPrintedMangle(t, "a { transform: rotate3d(0, 0, 1, 45deg) }", "a {\n  transform: rotate(45deg);\n}\n")

	expectPrintedMangle(t, "a { transform: rotateX(0) }", "a {\n  transform: rotateX(0);\n}\n")
	expectPrintedMangle(t, "a { transform: rotateX(0deg) }", "a {\n  transform: rotateX(0);\n}\n")
	expectPrintedMangle(t, "a { transform: rotateX(1deg) }", "a {\n  transform: rotateX(1deg);\n}\n")

	expectPrintedMangle(t, "a { transform: rotateY(0) }", "a {\n  transform: rotateY(0);\n}\n")
	expectPrintedMangle(t, "a { transform: rotateY(0deg) }", "a {\n  transform: rotateY(0);\n}\n")
	expectPrintedMangle(t, "a { transform: rotateY(1deg) }", "a {\n  transform: rotateY(1deg);\n}\n")

	expectPrintedMangle(t, "a { transform: rotateZ(0) }", "a {\n  transform: rotate(0);\n}\n")
	expectPrintedMangle(t, "a { transform: rotateZ(0deg) }", "a {\n  transform: rotate(0);\n}\n")
	expectPrintedMangle(t, "a { transform: rotateZ(1deg) }", "a {\n  transform: rotate(1deg);\n}\n")

	expectPrintedMangle(t, "a { transform: perspective(0) }", "a {\n  transform: perspective(0);\n}\n")
	expectPrintedMangle(t, "a { transform: perspective(0px) }", "a {\n  transform: perspective(0);\n}\n")
	expectPrintedMangle(t, "a { transform: perspective(1px) }", "a {\n  transform: perspective(1px);\n}\n")
}
