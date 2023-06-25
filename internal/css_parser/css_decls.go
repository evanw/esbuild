package css_parser

import (
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/logger"
)

func (p *parser) commaToken() css_ast.Token {
	t := css_ast.Token{
		Kind: css_lexer.TComma,
		Text: ",",
	}
	if !p.options.minifyWhitespace {
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

func (p *parser) processDeclarations(rules []css_ast.Rule) (rewrittenRules []css_ast.Rule) {
	margin := boxTracker{key: css_ast.DMargin, keyText: "margin", allowAuto: true}
	padding := boxTracker{key: css_ast.DPadding, keyText: "padding", allowAuto: false}
	inset := boxTracker{key: css_ast.DInset, keyText: "inset", allowAuto: true}
	borderRadius := borderRadiusTracker{}
	rewrittenRules = make([]css_ast.Rule, 0, len(rules))

	for _, rule := range rules {
		rewrittenRules = append(rewrittenRules, rule)
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

				if p.options.minifySyntax {
					t := decl.Value[0]
					if hex, ok := parseColor(t); ok {
						decl.Value[0] = p.mangleColor(t, hex)
					}
				}
			}

		case css_ast.DFont:
			if p.options.minifySyntax {
				decl.Value = p.mangleFont(decl.Value)
			}

		case css_ast.DFontFamily:
			if p.options.minifySyntax {
				if value, ok := p.mangleFontFamily(decl.Value); ok {
					decl.Value = value
				}
			}

		case css_ast.DFontWeight:
			if len(decl.Value) == 1 && p.options.minifySyntax {
				decl.Value[0] = p.mangleFontWeight(decl.Value[0])
			}

		case css_ast.DTransform:
			if p.options.minifySyntax {
				decl.Value = p.mangleTransforms(decl.Value)
			}

		case css_ast.DBoxShadow:
			if p.options.minifySyntax {
				decl.Value = p.mangleBoxShadows(decl.Value)
			}

		// Margin
		case css_ast.DMargin:
			if p.options.minifySyntax {
				margin.mangleSides(rewrittenRules, decl, p.options.minifyWhitespace)
			}
		case css_ast.DMarginTop:
			if p.options.minifySyntax {
				margin.mangleSide(rewrittenRules, decl, p.options.minifyWhitespace, boxTop)
			}
		case css_ast.DMarginRight:
			if p.options.minifySyntax {
				margin.mangleSide(rewrittenRules, decl, p.options.minifyWhitespace, boxRight)
			}
		case css_ast.DMarginBottom:
			if p.options.minifySyntax {
				margin.mangleSide(rewrittenRules, decl, p.options.minifyWhitespace, boxBottom)
			}
		case css_ast.DMarginLeft:
			if p.options.minifySyntax {
				margin.mangleSide(rewrittenRules, decl, p.options.minifyWhitespace, boxLeft)
			}

		// Padding
		case css_ast.DPadding:
			if p.options.minifySyntax {
				padding.mangleSides(rewrittenRules, decl, p.options.minifyWhitespace)
			}
		case css_ast.DPaddingTop:
			if p.options.minifySyntax {
				padding.mangleSide(rewrittenRules, decl, p.options.minifyWhitespace, boxTop)
			}
		case css_ast.DPaddingRight:
			if p.options.minifySyntax {
				padding.mangleSide(rewrittenRules, decl, p.options.minifyWhitespace, boxRight)
			}
		case css_ast.DPaddingBottom:
			if p.options.minifySyntax {
				padding.mangleSide(rewrittenRules, decl, p.options.minifyWhitespace, boxBottom)
			}
		case css_ast.DPaddingLeft:
			if p.options.minifySyntax {
				padding.mangleSide(rewrittenRules, decl, p.options.minifyWhitespace, boxLeft)
			}

		// Inset
		case css_ast.DInset:
			if !p.options.unsupportedCSSFeatures.Has(compat.InsetProperty) && p.options.minifySyntax {
				inset.mangleSides(rewrittenRules, decl, p.options.minifyWhitespace)
			}
		case css_ast.DTop:
			if !p.options.unsupportedCSSFeatures.Has(compat.InsetProperty) && p.options.minifySyntax {
				inset.mangleSide(rewrittenRules, decl, p.options.minifyWhitespace, boxTop)
			}
		case css_ast.DRight:
			if !p.options.unsupportedCSSFeatures.Has(compat.InsetProperty) && p.options.minifySyntax {
				inset.mangleSide(rewrittenRules, decl, p.options.minifyWhitespace, boxRight)
			}
		case css_ast.DBottom:
			if !p.options.unsupportedCSSFeatures.Has(compat.InsetProperty) && p.options.minifySyntax {
				inset.mangleSide(rewrittenRules, decl, p.options.minifyWhitespace, boxBottom)
			}
		case css_ast.DLeft:
			if !p.options.unsupportedCSSFeatures.Has(compat.InsetProperty) && p.options.minifySyntax {
				inset.mangleSide(rewrittenRules, decl, p.options.minifyWhitespace, boxLeft)
			}

		// Border radius
		case css_ast.DBorderRadius:
			if p.options.minifySyntax {
				borderRadius.mangleCorners(rewrittenRules, decl, p.options.minifyWhitespace)
			}
		case css_ast.DBorderTopLeftRadius:
			if p.options.minifySyntax {
				borderRadius.mangleCorner(rewrittenRules, decl, p.options.minifyWhitespace, borderRadiusTopLeft)
			}
		case css_ast.DBorderTopRightRadius:
			if p.options.minifySyntax {
				borderRadius.mangleCorner(rewrittenRules, decl, p.options.minifyWhitespace, borderRadiusTopRight)
			}
		case css_ast.DBorderBottomRightRadius:
			if p.options.minifySyntax {
				borderRadius.mangleCorner(rewrittenRules, decl, p.options.minifyWhitespace, borderRadiusBottomRight)
			}
		case css_ast.DBorderBottomLeftRadius:
			if p.options.minifySyntax {
				borderRadius.mangleCorner(rewrittenRules, decl, p.options.minifyWhitespace, borderRadiusBottomLeft)
			}
		}

		if prefixes, ok := p.options.cssPrefixData[decl.Key]; ok {
			if (prefixes & compat.WebkitPrefix) != 0 {
				rewrittenRules = p.insertPrefixedDeclaration(rewrittenRules, "-webkit-", rule.Loc, decl)
			}
			if (prefixes & compat.MozPrefix) != 0 {
				rewrittenRules = p.insertPrefixedDeclaration(rewrittenRules, "-moz-", rule.Loc, decl)
			}
			if (prefixes & compat.MsPrefix) != 0 {
				rewrittenRules = p.insertPrefixedDeclaration(rewrittenRules, "-ms-", rule.Loc, decl)
			}
			if (prefixes & compat.OPrefix) != 0 {
				rewrittenRules = p.insertPrefixedDeclaration(rewrittenRules, "-o-", rule.Loc, decl)
			}
		}
	}

	// Compact removed rules
	if p.options.minifySyntax {
		end := 0
		for _, rule := range rewrittenRules {
			if rule.Data != nil {
				rewrittenRules[end] = rule
				end++
			}
		}
		rewrittenRules = rewrittenRules[:end]
	}

	return
}

func (p *parser) insertPrefixedDeclaration(rules []css_ast.Rule, prefix string, loc logger.Loc, decl *css_ast.RDeclaration) []css_ast.Rule {
	keyText := prefix + decl.KeyText

	// Don't insert a prefixed declaration if there already is one
	for i := len(rules) - 2; i >= 0; i-- {
		if prev, ok := rules[i].Data.(*css_ast.RDeclaration); ok && prev.Key == css_ast.DUnknown {
			if prev.KeyText == keyText {
				// We found a previous declaration with a matching prefixed property.
				// The value is ignored, which matches the behavior of "autoprefixer".
				return rules
			}
			if p, d := len(prev.KeyText), len(decl.KeyText); p > d && prev.KeyText[p-d-1] == '-' && prev.KeyText[p-d:] == decl.KeyText {
				// Continue through a run of prefixed properties with the same name
				continue
			}
		}
		break
	}

	// Additional special cases for when the prefix applies
	switch decl.Key {
	case css_ast.DBackgroundClip:
		// The prefix is only needed for "background-clip: text"
		if len(decl.Value) != 1 || decl.Value[0].Kind != css_lexer.TIdent || decl.Value[0].Text != "text" {
			return rules
		}

	case css_ast.DPosition:
		// The prefix is only needed for "position: sticky"
		if len(decl.Value) != 1 || decl.Value[0].Kind != css_lexer.TIdent || decl.Value[0].Text != "sticky" {
			return rules
		}
	}

	// Clone the import records so that the duplicate has its own copy
	var value []css_ast.Token
	value, p.importRecords = css_ast.CloneTokensWithImportRecords(decl.Value, p.importRecords, nil, p.importRecords)

	// Additional special cases for how to transform the contents
	switch decl.Key {
	case css_ast.DPosition:
		// The prefix applies to the value, not the property
		keyText = decl.KeyText
		value[0].Text = "-webkit-sticky"

	case css_ast.DUserSelect:
		// The prefix applies to the value as well as the property
		if prefix == "-moz-" && len(value) == 1 && value[0].Kind == css_lexer.TIdent && value[0].Text == "none" {
			value[0].Text = "-moz-none"
		}
	}

	// Overwrite the latest declaration with the prefixed declaration
	rules[len(rules)-1] = css_ast.Rule{Loc: loc, Data: &css_ast.RDeclaration{
		KeyText:   keyText,
		KeyRange:  decl.KeyRange,
		Value:     value,
		Important: decl.Important,
	}}

	// Re-add the latest declaration after the inserted declaration
	rules = append(rules, css_ast.Rule{Loc: loc, Data: decl})
	return rules
}
