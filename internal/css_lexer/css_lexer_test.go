package css_lexer

import (
	"testing"

	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

func lexToken(contents string) T {
	log := logger.NewDeferLog()
	tokens := Tokenize(log, test.SourceForTest(contents))
	if len(tokens) > 0 {
		return tokens[0].Kind
	}
	return TEndOfFile
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
			test.AssertEqual(t, lexToken(contents), token)
		})
	}
}

func TestStringParsing(t *testing.T) {
	test.AssertEqual(t, ContentsOfStringToken("\"foo\""), "foo")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\oo\""), "foo")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\\"o\""), "f\"o")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\\\o\""), "f\\o")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\\no\""), "fo")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\\ro\""), "fo")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\\r\no\""), "fo")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\\fo\""), "fo")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\6fo\""), "foo")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\6f o\""), "foo")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\6f  o\""), "fo o")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\fffffffo\""), "f\uFFFDfo")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\10abcdeo\""), "f\U0010ABCDeo")
}

func TestURLParsing(t *testing.T) {
	contentsOfURLToken := func(raw string) string {
		text, _ := ContentsOfURLToken(raw)
		return text
	}
	test.AssertEqual(t, contentsOfURLToken("url(foo)"), "foo")
	test.AssertEqual(t, contentsOfURLToken("url(  foo\t\t)"), "foo")
	test.AssertEqual(t, contentsOfURLToken("url(f\\oo)"), "foo")
	test.AssertEqual(t, contentsOfURLToken("url(f\\\"o)"), "f\"o")
	test.AssertEqual(t, contentsOfURLToken("url(f\\'o)"), "f'o")
	test.AssertEqual(t, contentsOfURLToken("url(f\\)o)"), "f)o")
	test.AssertEqual(t, contentsOfURLToken("url(f\\6fo)"), "foo")
	test.AssertEqual(t, contentsOfURLToken("url(f\\6f o)"), "foo")
	test.AssertEqual(t, contentsOfURLToken("url(f\\6f  o)"), "fo o")
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
