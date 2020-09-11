package css_lexer

import (
	"testing"

	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

func lexToken(t *testing.T, contents string) T {
	log := logger.NewDeferLog()
	tokens := Tokenize(log, test.SourceForTest(contents))
	if len(tokens) > 0 {
		return tokens[0].Kind
	}
	return TEndOfFile
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
			test.AssertEqual(t, lexToken(t, contents), token)
		})
	}
}

func TestStringParsing(t *testing.T) {
	test.AssertEqual(t, ContentsOfStringToken("\"foo\""), "foo")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\oo\""), "foo")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\\"o\""), "f\"o")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\\\o\""), "f\\o")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\\no\""), "f\no")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\\ro\""), "f\ro")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\\vo\""), "f\vo")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\6fo\""), "foo")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\6f o\""), "foo")
	test.AssertEqual(t, ContentsOfStringToken("\"f\\6f  o\""), "fo o")
}

func TestURLParsing(t *testing.T) {
	test.AssertEqual(t, ContentsOfURLToken("url(foo)"), "foo")
	test.AssertEqual(t, ContentsOfURLToken("url(  foo\t\t)"), "foo")
	test.AssertEqual(t, ContentsOfURLToken("url(f\\oo)"), "foo")
	test.AssertEqual(t, ContentsOfURLToken("url(f\\\"o)"), "f\"o")
	test.AssertEqual(t, ContentsOfURLToken("url(f\\'o)"), "f'o")
	test.AssertEqual(t, ContentsOfURLToken("url(f\\)o)"), "f)o")
	test.AssertEqual(t, ContentsOfURLToken("url(f\\6fo)"), "foo")
	test.AssertEqual(t, ContentsOfURLToken("url(f\\6f o)"), "foo")
	test.AssertEqual(t, ContentsOfURLToken("url(f\\6f  o)"), "fo o")
}

func TestStringQuoting(t *testing.T) {
	test.AssertEqual(t, QuoteForStringToken("foo"), "\"foo\"")
	test.AssertEqual(t, QuoteForStringToken("f\"o"), "\"f\\\"o\"")
	test.AssertEqual(t, QuoteForStringToken("f\\o"), "\"f\\\\o\"")
	test.AssertEqual(t, QuoteForStringToken("f\no"), "\"f\\\no\"")
	test.AssertEqual(t, QuoteForStringToken("f\ro"), "\"f\\\ro\"")
	test.AssertEqual(t, QuoteForStringToken("f\fo"), "\"f\\\fo\"")
}
