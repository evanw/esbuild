package css_parser

import (
	"strings"

	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

func (p *parser) mangleTransforms(tokens []css_ast.Token) []css_ast.Token {
	for i, token := range tokens {
		transformFuncName := strings.ToLower(token.Text)
		switch transformFuncName {
		case "translate3d":
			token = p.mangleTranslate3d(token)
		}
		tokens[i] = token
	}

	return tokens
}

func (p *parser) mangleTranslate3d(token css_ast.Token) css_ast.Token {
	transformArg := *token.Children
	if len(transformArg) != 5 {
		return token
	}
	// translate3d(0, 0, tz) => translateZ(tz)
	var noWhitespace css_ast.WhitespaceFlags
	argX, argY, argZ := transformArg[0], transformArg[2], transformArg[4]
	if argX.Kind == css_lexer.TNumber && argX.Text == "0" && argY.Kind == css_lexer.TNumber && argY.Text == "0" {
		token.Text = "translateZ"
		argZ.Whitespace = noWhitespace
		token.Children = &[]css_ast.Token{
			argZ,
		}
	}

	return token
}
