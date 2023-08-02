package css_parser

import (
	"strings"

	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

// Scan for animation names in the "animation" shorthand property
func (p *parser) processAnimationShorthand(tokens []css_ast.Token) {
	type foundFlags struct {
		timingFunction bool
		iterationCount bool
		direction      bool
		fillMode       bool
		playState      bool
		name           bool
	}

	found := foundFlags{}

	for i, t := range tokens {
		switch t.Kind {
		case css_lexer.TComma:
			// Reset the flags when we encounter a comma
			found = foundFlags{}

		case css_lexer.TNumber:
			if !found.iterationCount {
				found.iterationCount = true
				continue
			}

		case css_lexer.TIdent:
			if !found.timingFunction {
				switch strings.ToLower(t.Text) {
				case "linear", "ease", "ease-in", "ease-out", "ease-in-out", "step-start", "step-end":
					found.timingFunction = true
					continue
				}
			}

			if !found.iterationCount && strings.ToLower(t.Text) == "infinite" {
				found.iterationCount = true
				continue
			}

			if !found.direction {
				switch strings.ToLower(t.Text) {
				case "normal", "reverse", "alternate", "alternate-reverse":
					found.direction = true
					continue
				}
			}

			if !found.fillMode {
				switch strings.ToLower(t.Text) {
				case "none", "forwards", "backwards", "both":
					found.fillMode = true
					continue
				}
			}

			if !found.playState {
				switch strings.ToLower(t.Text) {
				case "running", "paused":
					found.playState = true
					continue
				}
			}

			if !found.name {
				p.handleSingleAnimationName(&tokens[i])
				found.name = true
				continue
			}

		case css_lexer.TString:
			if !found.name {
				p.handleSingleAnimationName(&tokens[i])
				found.name = true
				continue
			}
		}
	}
}

func (p *parser) processAnimationName(tokens []css_ast.Token) {
	// Convert any local names
	for i, t := range tokens {
		if t.Kind == css_lexer.TIdent || t.Kind == css_lexer.TString {
			p.handleSingleAnimationName(&tokens[i])
		}
	}
}

func (p *parser) handleSingleAnimationName(token *css_ast.Token) {
	if lower := strings.ToLower(token.Text); lower == "none" || cssWideAndReservedKeywords[lower] {
		return
	}

	token.Kind = css_lexer.TSymbol
	token.PayloadIndex = p.symbolForName(token.Loc, token.Text).Ref.InnerIndex
}
