package css_parser

import (
	"strings"

	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

func (p *parser) mangleFontWeight(tokens []css_ast.Token) []css_ast.Token {
	if len(tokens) != 1 {
		return tokens
	}

	if tokens[0].Kind != css_lexer.TIdent {
		return tokens
	}

	switch strings.ToLower(tokens[0].Text) {
	case "normal":
		tokens[0].Text = "400"
		tokens[0].Kind = css_lexer.TNumber
	case "bold":
		tokens[0].Text = "700"
		tokens[0].Kind = css_lexer.TNumber
	}

	return tokens
}
