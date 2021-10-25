package css_parser

import (
	"strings"

	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

// Specification: https://drafts.csswg.org/css-values-4/#common-keywords
var wideKeywords = map[string]bool{
	"initial": true,
	"inherit": true,
	"unset":   true,
}

// Specification: https://drafts.csswg.org/css-fonts/#generic-font-families
var genericFamilyNames = map[string]bool{
	"serif":         true,
	"sans-serif":    true,
	"cursive":       true,
	"fantasy":       true,
	"monospace":     true,
	"system-ui":     true,
	"emoji":         true,
	"math":          true,
	"fangsong":      true,
	"ui-serif":      true,
	"ui-sans-serif": true,
	"ui-monospace":  true,
	"ui-rounded":    true,
}

// Specification: https://drafts.csswg.org/css-fonts/#font-family-prop
func (p *parser) mangleFontFamily(tokens []css_ast.Token) []css_ast.Token {
	splittedTokens, ok := getTokensSplittedByComma(tokens)
	if !ok {
		return tokens
	}

	newTokens := make([]css_ast.Token, 0, len(tokens))

	for i, sToken := range splittedTokens {
		if i > 0 {
			newTokens = append(newTokens, p.commaToken())
		}

		ts, ok := turnIntoCustomIdents(sToken)
		if !ok {
			newTokens = append(newTokens, sToken...)
			continue
		}

		if !p.options.RemoveWhitespace {
			ts[0].Whitespace |= css_ast.WhitespaceBefore
		}
		newTokens = append(newTokens, ts...)
	}

	return newTokens
}

func getTokensSplittedByComma(tokens []css_ast.Token) ([][]css_ast.Token, bool) {
	result := make([][]css_ast.Token, 0)

	start := 0
	for i := range tokens {
		if tokens[i].Kind == css_lexer.TComma {
			result = append(result, tokens[start:i])
			start = i + 1
			continue
		}

		// var() and env() may include comma
		if tokens[i].Kind == css_lexer.TFunction {
			switch strings.ToLower(tokens[i].Text) {
			case "var", "env":
				return [][]css_ast.Token{}, false
			}
		}
	}
	result = append(result, tokens[start:])
	return result, true
}

func turnIntoCustomIdents(tokens []css_ast.Token) ([]css_ast.Token, bool) {
	if len(tokens) != 1 || tokens[0].Kind != css_lexer.TString {
		return []css_ast.Token{}, false
	}

	names := strings.Split(tokens[0].Text, " ")
	newTokens := make([]css_ast.Token, 0, len(names))

	for i, name := range names {
		if !isValidCustomIdent(name, genericFamilyNames) {
			return []css_ast.Token{}, false
		}

		var whitespace css_ast.WhitespaceFlags
		if i != 0 {
			whitespace = css_ast.WhitespaceBefore
		}

		newTokens = append(newTokens, css_ast.Token{
			Kind:       css_lexer.TIdent,
			Text:       name,
			Whitespace: whitespace,
		})
	}

	return newTokens, true
}

// Specification: https://drafts.csswg.org/css-values-4/#custom-idents
func isValidCustomIdent(text string, predefinedKeywords map[string]bool) bool {
	loweredText := strings.ToLower(text)

	if _, ok := predefinedKeywords[loweredText]; ok {
		return false
	}
	if _, ok := wideKeywords[loweredText]; ok {
		return false
	}
	if loweredText == "default" {
		return false
	}
	if loweredText == "" {
		return false
	}

	// validate if it contains characters which needs to be escaped
	if !css_lexer.WouldStartIdentifierWithoutEscapes(text) {
		return false
	}
	for _, c := range text {
		if !css_lexer.IsNameContinue(c) {
			return false
		}
	}

	return true
}
