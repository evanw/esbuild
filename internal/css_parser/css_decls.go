package css_parser

import (
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

func (p *parser) commaToken() css_ast.Token {
	t := css_ast.Token{
		Kind: css_lexer.TComma,
		Text: ",",
	}
	if !p.options.RemoveWhitespace {
		t.Whitespace = css_ast.WhitespaceAfter
	}
	return t
}

func (p *parser) processDeclarations(rules []css_ast.R) []css_ast.R {
	margin := marginTracker{}

	for i, rule := range rules {
		decl, ok := rule.(*css_ast.RDeclaration)
		if !ok {
			continue
		}

		switch decl.Key {
		case css_ast.DBackgroundColor,
			css_ast.DBorderBlockEndColor,
			css_ast.DBorderBlockStartColor,
			css_ast.DBorderBottomColor,
			css_ast.DBorderColor,
			css_ast.DBorderInlineEndColor,
			css_ast.DBorderInlineStartColor,
			css_ast.DBorderLeftColor,
			css_ast.DBorderRightColor,
			css_ast.DBorderTopColor,
			css_ast.DCaretColor,
			css_ast.DColor,
			css_ast.DColumnRuleColor,
			css_ast.DFloodColor,
			css_ast.DLightingColor,
			css_ast.DOutlineColor,
			css_ast.DStopColor,
			css_ast.DTextDecorationColor,
			css_ast.DTextEmphasisColor:

			if len(decl.Value) == 1 {
				decl.Value[0] = p.lowerColor(decl.Value[0])

				if p.options.MangleSyntax {
					decl.Value[0] = p.mangleColor(decl.Value[0])
				}
			}

		case css_ast.DMargin:
			if p.options.MangleSyntax {
				margin.mangleSides(rules, decl, i)
			}

		case css_ast.DMarginTop:
			if p.options.MangleSyntax {
				margin.mangleSide(rules, decl, i, marginTop)
			}

		case css_ast.DMarginRight:
			if p.options.MangleSyntax {
				margin.mangleSide(rules, decl, i, marginRight)
			}

		case css_ast.DMarginBottom:
			if p.options.MangleSyntax {
				margin.mangleSide(rules, decl, i, marginBottom)
			}

		case css_ast.DMarginLeft:
			if p.options.MangleSyntax {
				margin.mangleSide(rules, decl, i, marginLeft)
			}
		}
	}

	// Compact removed rules
	if p.options.MangleSyntax {
		end := 0
		for _, rule := range rules {
			if rule != nil {
				rules[end] = rule
				end++
			}
		}
		rules = rules[:end]
	}

	return rules
}
