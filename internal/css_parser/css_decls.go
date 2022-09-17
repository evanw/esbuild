package css_parser

import (
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

func (p *parser) commaToken() css_ast.Token {
	t := css_ast.Token{
		Kind: css_lexer.TComma,
		Text: ",",
	}
	if !p.options.MinifyWhitespace {
		t.Whitespace = css_ast.WhitespaceAfter
	}
	return t
}

func expandTokenQuad(tokens []css_ast.Token, allowedIdent string) (result [4]css_ast.Token, ok bool) {
	n := len(tokens)
	if n < 1 || n > 4 {
		return
	}

	// Don't do this if we encounter any unexpected tokens such as "var()"
	for i := 0; i < n; i++ {
		if t := tokens[i]; !t.Kind.IsNumeric() && (t.Kind != css_lexer.TIdent || allowedIdent == "" || t.Text != allowedIdent) {
			return
		}
	}

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

	ok = true
	return
}

func compactTokenQuad(a css_ast.Token, b css_ast.Token, c css_ast.Token, d css_ast.Token, minifyWhitespace bool) []css_ast.Token {
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
		if !minifyWhitespace || i > 0 {
			whitespace |= css_ast.WhitespaceBefore
		}
		if i+1 < len(tokens) {
			whitespace |= css_ast.WhitespaceAfter
		}
		tokens[i].Whitespace = whitespace
	}
	return tokens
}

func (p *parser) processDeclarations(rules []css_ast.Rule) []css_ast.Rule {
	margin := boxTracker{key: css_ast.DMargin, keyText: "margin", allowAuto: true}
	padding := boxTracker{key: css_ast.DPadding, keyText: "padding", allowAuto: false}
	inset := boxTracker{key: css_ast.DInset, keyText: "inset", allowAuto: true}
	borderRadius := borderRadiusTracker{}

	for i, rule := range rules {
		decl, ok := rule.Data.(*css_ast.RDeclaration)
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

				if p.options.MinifySyntax {
					t := decl.Value[0]
					if hex, ok := parseColor(t); ok {
						decl.Value[0] = p.mangleColor(t, hex)
					}
				}
			}

		case css_ast.DFont:
			if p.options.MinifySyntax {
				decl.Value = p.mangleFont(decl.Value)
			}

		case css_ast.DFontFamily:
			if p.options.MinifySyntax {
				if value, ok := p.mangleFontFamily(decl.Value); ok {
					decl.Value = value
				}
			}

		case css_ast.DFontWeight:
			if len(decl.Value) == 1 && p.options.MinifySyntax {
				decl.Value[0] = p.mangleFontWeight(decl.Value[0])
			}

		case css_ast.DTransform:
			if p.options.MinifySyntax {
				decl.Value = p.mangleTransforms(decl.Value)
			}

		case css_ast.DBoxShadow:
			if p.options.MinifySyntax {
				decl.Value = p.mangleBoxShadows(decl.Value)
			}

		// Margin
		case css_ast.DMargin:
			if p.options.MinifySyntax {
				margin.mangleSides(rules, decl, i, p.options.MinifyWhitespace)
			}
		case css_ast.DMarginTop:
			if p.options.MinifySyntax {
				margin.mangleSide(rules, decl, i, p.options.MinifyWhitespace, boxTop)
			}
		case css_ast.DMarginRight:
			if p.options.MinifySyntax {
				margin.mangleSide(rules, decl, i, p.options.MinifyWhitespace, boxRight)
			}
		case css_ast.DMarginBottom:
			if p.options.MinifySyntax {
				margin.mangleSide(rules, decl, i, p.options.MinifyWhitespace, boxBottom)
			}
		case css_ast.DMarginLeft:
			if p.options.MinifySyntax {
				margin.mangleSide(rules, decl, i, p.options.MinifyWhitespace, boxLeft)
			}

		// Padding
		case css_ast.DPadding:
			if p.options.MinifySyntax {
				padding.mangleSides(rules, decl, i, p.options.MinifyWhitespace)
			}
		case css_ast.DPaddingTop:
			if p.options.MinifySyntax {
				padding.mangleSide(rules, decl, i, p.options.MinifyWhitespace, boxTop)
			}
		case css_ast.DPaddingRight:
			if p.options.MinifySyntax {
				padding.mangleSide(rules, decl, i, p.options.MinifyWhitespace, boxRight)
			}
		case css_ast.DPaddingBottom:
			if p.options.MinifySyntax {
				padding.mangleSide(rules, decl, i, p.options.MinifyWhitespace, boxBottom)
			}
		case css_ast.DPaddingLeft:
			if p.options.MinifySyntax {
				padding.mangleSide(rules, decl, i, p.options.MinifyWhitespace, boxLeft)
			}

		// Inset
		case css_ast.DInset:
			if !p.options.UnsupportedCSSFeatures.Has(compat.InsetProperty) && p.options.MinifySyntax {
				inset.mangleSides(rules, decl, i, p.options.MinifyWhitespace)
			}
		case css_ast.DTop:
			if !p.options.UnsupportedCSSFeatures.Has(compat.InsetProperty) && p.options.MinifySyntax {
				inset.mangleSide(rules, decl, i, p.options.MinifyWhitespace, boxTop)
			}
		case css_ast.DRight:
			if !p.options.UnsupportedCSSFeatures.Has(compat.InsetProperty) && p.options.MinifySyntax {
				inset.mangleSide(rules, decl, i, p.options.MinifyWhitespace, boxRight)
			}
		case css_ast.DBottom:
			if !p.options.UnsupportedCSSFeatures.Has(compat.InsetProperty) && p.options.MinifySyntax {
				inset.mangleSide(rules, decl, i, p.options.MinifyWhitespace, boxBottom)
			}
		case css_ast.DLeft:
			if !p.options.UnsupportedCSSFeatures.Has(compat.InsetProperty) && p.options.MinifySyntax {
				inset.mangleSide(rules, decl, i, p.options.MinifyWhitespace, boxLeft)
			}

		// Border radius
		case css_ast.DBorderRadius:
			if p.options.MinifySyntax {
				borderRadius.mangleCorners(rules, decl, i, p.options.MinifyWhitespace)
			}
		case css_ast.DBorderTopLeftRadius:
			if p.options.MinifySyntax {
				borderRadius.mangleCorner(rules, decl, i, p.options.MinifyWhitespace, borderRadiusTopLeft)
			}
		case css_ast.DBorderTopRightRadius:
			if p.options.MinifySyntax {
				borderRadius.mangleCorner(rules, decl, i, p.options.MinifyWhitespace, borderRadiusTopRight)
			}
		case css_ast.DBorderBottomRightRadius:
			if p.options.MinifySyntax {
				borderRadius.mangleCorner(rules, decl, i, p.options.MinifyWhitespace, borderRadiusBottomRight)
			}
		case css_ast.DBorderBottomLeftRadius:
			if p.options.MinifySyntax {
				borderRadius.mangleCorner(rules, decl, i, p.options.MinifyWhitespace, borderRadiusBottomLeft)
			}

		case css_ast.DAnimation,
			css_ast.DAnimationName:
			mustBeString := cssWideAndReservedKeywords[decl.Value[0].Text] || decl.Value[0].Text == "none"
			if p.options.MinifySyntax {
				if decl.Value[0].Kind == css_lexer.TString && !mustBeString {
					decl.Value[0].Kind = css_lexer.TIdent
				}
			}
		}
	}

	// Compact removed rules
	if p.options.MinifySyntax {
		end := 0
		for _, rule := range rules {
			if rule.Data != nil {
				rules[end] = rule
				end++
			}
		}
		rules = rules[:end]
	}

	return rules
}
