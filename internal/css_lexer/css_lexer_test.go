package css_lexer

import (
	"testing"

	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

func lexToken(contents string) (T, string) {
	log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
	result := Tokenize(log, test.SourceForTest(contents))
	if len(result.Tokens) > 0 {
		t := result.Tokens[0]
		return t.Kind, t.DecodedText(contents)
	}
	return TEndOfFile, ""
}

func lexerError(contents string) string {
	log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
	Tokenize(log, test.SourceForTest(contents))
	text := ""
	for _, msg := range log.Done() {
		text += msg.String(logger.OutputOptions{}, logger.TerminalInfo{})
	}
	return text
}

func TestTokens(t *testing.T) {
	expected := []struct {
		contents string
		text     string
		token    T
	}{
		{"", "end of file", TEndOfFile},
		{"@media", "@-keyword", TAtKeyword},
		{"url(x y", "bad URL token", TBadURL},
		{"-->", "\"-->\"", TCDC},
		{"<!--", "\"<!--\"", TCDO},
		{"}", "\"}\"", TCloseBrace},
		{"]", "\"]\"", TCloseBracket},
		{")", "\")\"", TCloseParen},
		{":", "\":\"", TColon},
		{",", "\",\"", TComma},
		{"?", "delimiter", TDelim},
		{"&", "\"&\"", TDelimAmpersand},
		{"*", "\"*\"", TDelimAsterisk},
		{"|", "\"|\"", TDelimBar},
		{"^", "\"^\"", TDelimCaret},
		{"$", "\"$\"", TDelimDollar},
		{".", "\".\"", TDelimDot},
		{"=", "\"=\"", TDelimEquals},
		{"!", "\"!\"", TDelimExclamation},
		{">", "\">\"", TDelimGreaterThan},
		{"+", "\"+\"", TDelimPlus},
		{"/", "\"/\"", TDelimSlash},
		{"~", "\"~\"", TDelimTilde},
		{"1px", "dimension", TDimension},
		{"max(", "function token", TFunction},
		{"#name", "hash token", THash},
		{"name", "identifier", TIdent},
		{"123", "number", TNumber},
		{"{", "\"{\"", TOpenBrace},
		{"[", "\"[\"", TOpenBracket},
		{"(", "\"(\"", TOpenParen},
		{"50%", "percentage", TPercentage},
		{";", "\";\"", TSemicolon},
		{"'abc'", "string token", TString},
		{"url(test)", "URL token", TURL},
		{" ", "whitespace", TWhitespace},
	}

	for _, it := range expected {
		contents := it.contents
		token := it.token
		t.Run(contents, func(t *testing.T) {
			kind, _ := lexToken(contents)
			test.AssertEqual(t, kind, token)
		})
	}
}

func TestStringParsing(t *testing.T) {
	contentsOfStringToken := func(contents string) string {
		t.Helper()
		kind, text := lexToken(contents)
		test.AssertEqual(t, kind, TString)
		return text
	}
	test.AssertEqual(t, contentsOfStringToken("\"foo\""), "foo")
	test.AssertEqual(t, contentsOfStringToken("\"f\\oo\""), "foo")
	test.AssertEqual(t, contentsOfStringToken("\"f\\\"o\""), "f\"o")
	test.AssertEqual(t, contentsOfStringToken("\"f\\\\o\""), "f\\o")
	test.AssertEqual(t, contentsOfStringToken("\"f\\\no\""), "fo")
	test.AssertEqual(t, contentsOfStringToken("\"f\\\ro\""), "fo")
	test.AssertEqual(t, contentsOfStringToken("\"f\\\r\no\""), "fo")
	test.AssertEqual(t, contentsOfStringToken("\"f\\\fo\""), "fo")
	test.AssertEqual(t, contentsOfStringToken("\"f\\6fo\""), "foo")
	test.AssertEqual(t, contentsOfStringToken("\"f\\6f o\""), "foo")
	test.AssertEqual(t, contentsOfStringToken("\"f\\6f  o\""), "fo o")
	test.AssertEqual(t, contentsOfStringToken("\"f\\fffffffo\""), "f\uFFFDfo")
	test.AssertEqual(t, contentsOfStringToken("\"f\\10abcdeo\""), "f\U0010ABCDeo")
}

func TestURLParsing(t *testing.T) {
	contentsOfURLToken := func(expected T, contents string) string {
		t.Helper()
		kind, text := lexToken(contents)
		test.AssertEqual(t, kind, expected)
		return text
	}
	test.AssertEqual(t, contentsOfURLToken(TURL, "url(foo)"), "foo")
	test.AssertEqual(t, contentsOfURLToken(TURL, "url(  foo\t\t)"), "foo")
	test.AssertEqual(t, contentsOfURLToken(TURL, "url(f\\oo)"), "foo")
	test.AssertEqual(t, contentsOfURLToken(TURL, "url(f\\\"o)"), "f\"o")
	test.AssertEqual(t, contentsOfURLToken(TURL, "url(f\\'o)"), "f'o")
	test.AssertEqual(t, contentsOfURLToken(TURL, "url(f\\)o)"), "f)o")
	test.AssertEqual(t, contentsOfURLToken(TURL, "url(f\\6fo)"), "foo")
	test.AssertEqual(t, contentsOfURLToken(TURL, "url(f\\6f o)"), "foo")
	test.AssertEqual(t, contentsOfURLToken(TBadURL, "url(f\\6f  o)"), "url(f\\6f  o)")
}

func TestComment(t *testing.T) {
	test.AssertEqualWithDiff(t, lexerError("/*"), "<stdin>: ERROR: Expected \"*/\" to terminate multi-line comment\n<stdin>: NOTE: The multi-line comment starts here:\n")
	test.AssertEqualWithDiff(t, lexerError("/*/"), "<stdin>: ERROR: Expected \"*/\" to terminate multi-line comment\n<stdin>: NOTE: The multi-line comment starts here:\n")
	test.AssertEqualWithDiff(t, lexerError("/**/"), "")
	test.AssertEqualWithDiff(t, lexerError("//"), "<stdin>: WARNING: Comments in CSS use \"/* ... */\" instead of \"//\"\n")
}

func TestString(t *testing.T) {
	test.AssertEqualWithDiff(t, lexerError("'"), "<stdin>: WARNING: Unterminated string token\n")
	test.AssertEqualWithDiff(t, lexerError("\""), "<stdin>: WARNING: Unterminated string token\n")
	test.AssertEqualWithDiff(t, lexerError("'\\'"), "<stdin>: WARNING: Unterminated string token\n")
	test.AssertEqualWithDiff(t, lexerError("\"\\\""), "<stdin>: WARNING: Unterminated string token\n")
	test.AssertEqualWithDiff(t, lexerError("''"), "")
	test.AssertEqualWithDiff(t, lexerError("\"\""), "")
}

func TestBOM(t *testing.T) {
	// A byte order mark should not be parsed as an identifier
	kind, _ := lexToken("\uFEFF.")
	test.AssertEqual(t, kind, TDelimDot)
}
