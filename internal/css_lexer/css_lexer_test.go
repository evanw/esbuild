package css_lexer

import (
	"testing"

	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

func lexToken(contents string) (T, string) {
	log := logger.NewDeferLog()
	tokens := Tokenize(log, test.SourceForTest(contents))
	if len(tokens) > 0 {
		t := tokens[0]
		return t.Kind, t.DecodedText(contents)
	}
	return TEndOfFile, ""
}

func lexerError(contents string) string {
	log := logger.NewDeferLog()
	Tokenize(log, test.SourceForTest(contents))
	text := ""
	for _, msg := range log.Done() {
		text += msg.String(logger.StderrOptions{}, logger.TerminalInfo{})
	}
	return text
}

func TestTokens(t *testing.T) {
	expected := []struct {
		contents string
		token    T
		text     string
	}{
		{"", TEndOfFile, "end of file"},
		{"@media", TAtKeyword, "@-keyword"},
		{"url(x y", TBadURL, "bad URL token"},
		{"-->", TCDC, "\"-->\""},
		{"<!--", TCDO, "\"<!--\""},
		{"}", TCloseBrace, "\"}\""},
		{"]", TCloseBracket, "\"]\""},
		{")", TCloseParen, "\")\""},
		{":", TColon, "\":\""},
		{",", TComma, "\",\""},
		{"?", TDelim, "delimiter"},
		{"&", TDelimAmpersand, "\"&\""},
		{"*", TDelimAsterisk, "\"*\""},
		{"|", TDelimBar, "\"|\""},
		{"^", TDelimCaret, "\"^\""},
		{"$", TDelimDollar, "\"$\""},
		{".", TDelimDot, "\".\""},
		{"=", TDelimEquals, "\"=\""},
		{"!", TDelimExclamation, "\"!\""},
		{">", TDelimGreaterThan, "\">\""},
		{"+", TDelimPlus, "\"+\""},
		{"/", TDelimSlash, "\"/\""},
		{"~", TDelimTilde, "\"~\""},
		{"1px", TDimension, "dimension"},
		{"max(", TFunction, "function token"},
		{"#0", THash, "hash token"},
		{"#id", THashID, "hash token"},
		{"name", TIdent, "identifier"},
		{"123", TNumber, "number"},
		{"{", TOpenBrace, "\"{\""},
		{"[", TOpenBracket, "\"[\""},
		{"(", TOpenParen, "\"(\""},
		{"50%", TPercentage, "percentage"},
		{";", TSemicolon, "\";\""},
		{"'abc'", TString, "string token"},
		{"url(test)", TURL, "URL token"},
		{" ", TWhitespace, "whitespace"},
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
	test.AssertEqual(t, lexerError("/*"), "<stdin>: error: Expected \"*/\" to terminate multi-line comment\n")
	test.AssertEqual(t, lexerError("/*/"), "<stdin>: error: Expected \"*/\" to terminate multi-line comment\n")
	test.AssertEqual(t, lexerError("/**/"), "")
	test.AssertEqual(t, lexerError("//"), "<stdin>: warning: Comments in CSS use \"/* ... */\" instead of \"//\"\n")
}

func TestString(t *testing.T) {
	test.AssertEqual(t, lexerError("'"), "<stdin>: error: Unterminated string token\n")
	test.AssertEqual(t, lexerError("\""), "<stdin>: error: Unterminated string token\n")
	test.AssertEqual(t, lexerError("'\\'"), "<stdin>: error: Unterminated string token\n")
	test.AssertEqual(t, lexerError("\"\\\""), "<stdin>: error: Unterminated string token\n")
	test.AssertEqual(t, lexerError("''"), "")
	test.AssertEqual(t, lexerError("\"\""), "")
}
