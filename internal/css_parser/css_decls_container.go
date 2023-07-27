package css_parser

import (
	"strings"

	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

// Scan for container names in the "container" shorthand property
func (p *parser) processContainerShorthand(tokens []css_ast.Token) {
	// Validate the syntax
	for i, t := range tokens {
		if t.Kind == css_lexer.TIdent {
			continue
		}
		if t.Kind == css_lexer.TDelimSlash && i+2 == len(tokens) && tokens[i+1].Kind == css_lexer.TIdent {
			break
		}
		return
	}

	// Convert any local names
	for i, t := range tokens {
		if t.Kind != css_lexer.TIdent {
			break
		}
		p.handleSingleContainerName(&tokens[i])
	}
}

func (p *parser) processContainerName(tokens []css_ast.Token) {
	// Validate the syntax
	for _, t := range tokens {
		if t.Kind != css_lexer.TIdent {
			return
		}
	}

	// Convert any local names
	for i := range tokens {
		p.handleSingleContainerName(&tokens[i])
	}
}

func (p *parser) handleSingleContainerName(token *css_ast.Token) {
	if lower := strings.ToLower(token.Text); lower == "none" || cssWideAndReservedKeywords[lower] {
		return
	}

	token.Kind = css_lexer.TSymbol
	token.PayloadIndex = p.symbolForName(token.Loc, token.Text).Ref.InnerIndex
}
