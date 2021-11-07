package css_parser

import (
	"strings"

	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

// Specification: https://drafts.csswg.org/css-fonts/#font-prop
func (p *parser) mangleFont(tokens []css_ast.Token) []css_ast.Token {
	// invalid or system fonts
	if len(tokens) <= 1 {
		return tokens
	}

	pos := 0
	for ; pos < len(tokens); pos++ {
		token := tokens[pos]
		if isFontSize(token) {
			break
		}

		tokens[pos] = p.mangleFontWeight(token)
	}

	if !(pos < len(tokens)) || !isFontSize(tokens[pos]) {
		return tokens
	}
	pos++

	// / <line-height>
	if tokens[pos].Kind == css_lexer.TDelimSlash {
		if !(pos+1 < len(tokens)) {
			return tokens
		}

		if p.options.RemoveWhitespace {
			tokens[pos-1].Whitespace ^= css_ast.WhitespaceAfter
			tokens[pos].Whitespace = 0
			tokens[pos+1].Whitespace ^= css_ast.WhitespaceBefore
		}

		pos += 2
	}

	newFamilyTokens := p.mangleFontFamily(tokens[pos:])
	tokens = tokens[:pos]
	tokens = append(tokens, newFamilyTokens...)
	return tokens
}

var fontSizeKeywords = map[string]bool{
	// <absolute-size>: https://drafts.csswg.org/css-fonts/#valdef-font-size-absolute-size
	"xx-small":  true,
	"x-small":   true,
	"small":     true,
	"medium":    true,
	"large":     true,
	"x-large":   true,
	"xx-large":  true,
	"xxx-large": true,
	// <relative-size>: https://drafts.csswg.org/css-fonts/#valdef-font-size-relative-size
	"larger":  true,
	"smaller": true,
}

// Specification: https://drafts.csswg.org/css-fonts/#font-size-prop
func isFontSize(token css_ast.Token) bool {
	// <length-percentage>
	if token.Kind == css_lexer.TDimension || token.Kind == css_lexer.TPercentage {
		return true
	}

	// <absolute-size> or <relative-size>
	if token.Kind == css_lexer.TIdent {
		_, ok := fontSizeKeywords[strings.ToLower(token.Text)]
		return ok
	}

	return false
}
