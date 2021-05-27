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

func expandTokenQuad(tokens []css_ast.Token) (result [4]css_ast.Token) {
	n := len(tokens)
	result[0] = tokens[0]
	if n > 1 {
		result[1] = tokens[1]
	} else {
		result[1] = result[0]
	}
	if n > 2 {
		result[2] = tokens[2]
	} else {
		result[2] = result[0]
	}
	if n > 3 {
		result[3] = tokens[3]
	} else {
		result[3] = result[1]
	}
	return
}

func compactTokenQuad(a css_ast.Token, b css_ast.Token, c css_ast.Token, d css_ast.Token, removeWhitespace bool) []css_ast.Token {
	tokens := []css_ast.Token{a, b, c, d}
	if tokens[3].EqualIgnoringWhitespace(tokens[1]) {
		if tokens[2].EqualIgnoringWhitespace(tokens[0]) {
			if tokens[1].EqualIgnoringWhitespace(tokens[0]) {
				tokens = tokens[:1]
			} else {
				tokens = tokens[:2]
			}
		} else {
			tokens = tokens[:3]
		}
	}
	for i := range tokens {
		var whitespace css_ast.WhitespaceFlags
		if !removeWhitespace || i > 0 {
			whitespace |= css_ast.WhitespaceBefore
		}
		if i+1 < len(tokens) {
			whitespace |= css_ast.WhitespaceAfter
		}
		tokens[i].Whitespace = whitespace
	}
	return tokens
}

func (p *parser) processDeclarations(rules []css_ast.R) []css_ast.R {
	margin := boxTracker{}
	padding := boxTracker{}
	borderRadius := borderRadiusTracker{}

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
			css_ast.DFill,
			css_ast.DFloodColor,
			css_ast.DLightingColor,
			css_ast.DOutlineColor,
			css_ast.DStopColor,
			css_ast.DStroke,
			css_ast.DTextDecorationColor,
			css_ast.DTextEmphasisColor:

			if len(decl.Value) == 1 {
				decl.Value[0] = p.lowerColor(decl.Value[0])

				if p.options.MangleSyntax {
					decl.Value[0] = p.mangleColor(decl.Value[0])
				}
			}

		case css_ast.DBoxShadow:
			if p.options.MangleSyntax {
				decl.Value = p.mangleBoxShadows(decl.Value)
			}

		case css_ast.DPadding:
			if p.options.MangleSyntax {
				padding.mangleSides(rules, decl, i, p.options.RemoveWhitespace)
			}
		case css_ast.DPaddingTop:
			if p.options.MangleSyntax {
				padding.mangleSide(rules, decl, i, p.options.RemoveWhitespace, boxTop)
			}
		case css_ast.DPaddingRight:
			if p.options.MangleSyntax {
				padding.mangleSide(rules, decl, i, p.options.RemoveWhitespace, boxRight)
			}
		case css_ast.DPaddingBottom:
			if p.options.MangleSyntax {
				padding.mangleSide(rules, decl, i, p.options.RemoveWhitespace, boxBottom)
			}
		case css_ast.DPaddingLeft:
			if p.options.MangleSyntax {
				padding.mangleSide(rules, decl, i, p.options.RemoveWhitespace, boxLeft)
			}

		case css_ast.DMargin:
			if p.options.MangleSyntax {
				margin.mangleSides(rules, decl, i, p.options.RemoveWhitespace)
			}
		case css_ast.DMarginTop:
			if p.options.MangleSyntax {
				margin.mangleSide(rules, decl, i, p.options.RemoveWhitespace, boxTop)
			}
		case css_ast.DMarginRight:
			if p.options.MangleSyntax {
				margin.mangleSide(rules, decl, i, p.options.RemoveWhitespace, boxRight)
			}
		case css_ast.DMarginBottom:
			if p.options.MangleSyntax {
				margin.mangleSide(rules, decl, i, p.options.RemoveWhitespace, boxBottom)
			}
		case css_ast.DMarginLeft:
			if p.options.MangleSyntax {
				margin.mangleSide(rules, decl, i, p.options.RemoveWhitespace, boxLeft)
			}

		case css_ast.DBorderRadius:
			if p.options.MangleSyntax {
				borderRadius.mangleCorners(rules, decl, i, p.options.RemoveWhitespace)
			}
		case css_ast.DBorderTopLeftRadius:
			if p.options.MangleSyntax {
				borderRadius.mangleCorner(rules, decl, i, p.options.RemoveWhitespace, borderRadiusTopLeft)
			}
		case css_ast.DBorderTopRightRadius:
			if p.options.MangleSyntax {
				borderRadius.mangleCorner(rules, decl, i, p.options.RemoveWhitespace, borderRadiusTopRight)
			}
		case css_ast.DBorderBottomRightRadius:
			if p.options.MangleSyntax {
				borderRadius.mangleCorner(rules, decl, i, p.options.RemoveWhitespace, borderRadiusBottomRight)
			}
		case css_ast.DBorderBottomLeftRadius:
			if p.options.MangleSyntax {
				borderRadius.mangleCorner(rules, decl, i, p.options.RemoveWhitespace, borderRadiusBottomLeft)
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
