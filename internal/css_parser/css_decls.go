package css_parser

import (
	"strings"

	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/logger"
)

func (p *parser) commaToken(loc logger.Loc) css_ast.Token {
	t := css_ast.Token{
		Loc:  loc,
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

func (p *parser) processDeclarations(rules []css_ast.Rule, composesContext *composesContext) (rewrittenRules []css_ast.Rule) {
	margin := boxTracker{key: css_ast.DMargin, keyText: "margin", allowAuto: true}
	padding := boxTracker{key: css_ast.DPadding, keyText: "padding", allowAuto: false}
	inset := boxTracker{key: css_ast.DInset, keyText: "inset", allowAuto: true}
	borderRadius := borderRadiusTracker{}
	rewrittenRules = make([]css_ast.Rule, 0, len(rules))
	didWarnAboutComposes := false
	wouldClipColorFlag := false
	var declarationKeys map[string]struct{}

	// Don't automatically generate the "inset" property if it's not supported
	if p.options.unsupportedCSSFeatures.Has(compat.InsetProperty) {
		inset.key = css_ast.DUnknown
		inset.keyText = ""
	}

	// If this is a local class selector, track which CSS properties it declares.
	// This is used to warn when CSS "composes" is used incorrectly.
	if composesContext != nil {
		for _, ref := range composesContext.parentRefs {
			composes, ok := p.composes[ref]
			if !ok {
				composes = &css_ast.Composes{}
				p.composes[ref] = composes
			}
			properties := composes.Properties
			if properties == nil {
				properties = make(map[string]logger.Loc)
				composes.Properties = properties
			}
			for _, rule := range rules {
				if decl, ok := rule.Data.(*css_ast.RDeclaration); ok && decl.Key != css_ast.DComposes {
					properties[decl.KeyText] = decl.KeyRange.Loc
				}
			}
		}
	}

	for i := 0; i < len(rules); i++ {
		rule := rules[i]
		rewrittenRules = append(rewrittenRules, rule)
		decl, ok := rule.Data.(*css_ast.RDeclaration)
		if !ok {
			continue
		}

		// If the previous loop iteration would have clipped a color, we will
		// duplicate it and insert the clipped copy before the unclipped copy
		var wouldClipColor *bool
		if wouldClipColorFlag {
			wouldClipColorFlag = false
			clone := *decl
			clone.Value = css_ast.CloneTokensWithoutImportRecords(clone.Value)
			decl = &clone
			rule.Data = decl
			n := len(rewrittenRules) - 2
			rewrittenRules = append(rewrittenRules[:n], rule, rewrittenRules[n])
		} else {
			wouldClipColor = &wouldClipColorFlag
		}

		switch decl.Key {
		case css_ast.DComposes:
			// Only process "composes" directives if we're in "local-css" or
			// "global-css" mode. In these cases, "composes" directives will always
			// be removed (because they are being processed) even if they contain
			// errors. Otherwise we leave "composes" directives there untouched and
			// don't check them for errors.
			if p.options.symbolMode != symbolModeDisabled {
				if composesContext == nil {
					if !didWarnAboutComposes {
						didWarnAboutComposes = true
						p.log.AddID(logger.MsgID_CSS_CSSSyntaxError, logger.Warning, &p.tracker, decl.KeyRange, "\"composes\" is not valid here")
					}
				} else if composesContext.problemRange.Len > 0 {
					if !didWarnAboutComposes {
						didWarnAboutComposes = true
						p.log.AddIDWithNotes(logger.MsgID_CSS_CSSSyntaxError, logger.Warning, &p.tracker, decl.KeyRange, "\"composes\" only works inside single class selectors",
							[]logger.MsgData{p.tracker.MsgData(composesContext.problemRange, "The parent selector is not a single class selector because of the syntax here:")})
					}
				} else {
					p.handleComposesPragma(*composesContext, decl.Value)
				}
				rewrittenRules = rewrittenRules[:len(rewrittenRules)-1]
			}

		case css_ast.DBackground:
			for i, t := range decl.Value {
				t = p.lowerAndMinifyColor(t, wouldClipColor)
				t = p.lowerAndMinifyGradient(t, wouldClipColor)
				decl.Value[i] = t
			}

		case css_ast.DBackgroundImage,
			css_ast.DBorderImage,
			css_ast.DMaskImage:

			for i, t := range decl.Value {
				t = p.lowerAndMinifyGradient(t, wouldClipColor)
				decl.Value[i] = t
			}

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
				decl.Value[0] = p.lowerAndMinifyColor(decl.Value[0], wouldClipColor)
			}

		case css_ast.DTransform:
			if p.options.minifySyntax {
				decl.Value = p.mangleTransforms(decl.Value)
			}

		case css_ast.DBoxShadow:
			decl.Value = p.lowerAndMangleBoxShadows(decl.Value, wouldClipColor)

		// Container name
		case css_ast.DContainer:
			p.processContainerShorthand(decl.Value)
		case css_ast.DContainerName:
			p.processContainerName(decl.Value)

			// Animation name
		case css_ast.DAnimation:
			p.processAnimationShorthand(decl.Value)
		case css_ast.DAnimationName:
			p.processAnimationName(decl.Value)

		// List style
		case css_ast.DListStyle:
			p.processListStyleShorthand(decl.Value)
		case css_ast.DListStyleType:
			if len(decl.Value) == 1 {
				p.processListStyleType(&decl.Value[0])
			}

			// Font
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
			if p.options.unsupportedCSSFeatures.Has(compat.InsetProperty) {
				if decls, ok := p.lowerInset(rule.Loc, decl); ok {
					rewrittenRules = rewrittenRules[:len(rewrittenRules)-1]
					for i := range decls {
						rewrittenRules = append(rewrittenRules, decls[i])
						if p.options.minifySyntax {
							inset.mangleSide(rewrittenRules, decls[i].Data.(*css_ast.RDeclaration), p.options.minifyWhitespace, i)
						}
					}
					break
				}
			}
			if p.options.minifySyntax {
				inset.mangleSides(rewrittenRules, decl, p.options.minifyWhitespace)
			}
		case css_ast.DTop:
			if p.options.minifySyntax {
				inset.mangleSide(rewrittenRules, decl, p.options.minifyWhitespace, boxTop)
			}
		case css_ast.DRight:
			if p.options.minifySyntax {
				inset.mangleSide(rewrittenRules, decl, p.options.minifyWhitespace, boxRight)
			}
		case css_ast.DBottom:
			if p.options.minifySyntax {
				inset.mangleSide(rewrittenRules, decl, p.options.minifyWhitespace, boxBottom)
			}
		case css_ast.DLeft:
			if p.options.minifySyntax {
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
			if declarationKeys == nil {
				// Only generate this map if it's needed
				declarationKeys = make(map[string]struct{})
				for _, rule := range rules {
					if decl, ok := rule.Data.(*css_ast.RDeclaration); ok {
						declarationKeys[decl.KeyText] = struct{}{}
					}
				}
			}
			if (prefixes & compat.WebkitPrefix) != 0 {
				rewrittenRules = p.insertPrefixedDeclaration(rewrittenRules, "-webkit-", rule.Loc, decl, declarationKeys)
			}
			if (prefixes & compat.KhtmlPrefix) != 0 {
				rewrittenRules = p.insertPrefixedDeclaration(rewrittenRules, "-khtml-", rule.Loc, decl, declarationKeys)
			}
			if (prefixes & compat.MozPrefix) != 0 {
				rewrittenRules = p.insertPrefixedDeclaration(rewrittenRules, "-moz-", rule.Loc, decl, declarationKeys)
			}
			if (prefixes & compat.MsPrefix) != 0 {
				rewrittenRules = p.insertPrefixedDeclaration(rewrittenRules, "-ms-", rule.Loc, decl, declarationKeys)
			}
			if (prefixes & compat.OPrefix) != 0 {
				rewrittenRules = p.insertPrefixedDeclaration(rewrittenRules, "-o-", rule.Loc, decl, declarationKeys)
			}
		}

		// If this loop iteration would have clipped a color, the out-of-gamut
		// colors will not be clipped and this flag will be set. We then set up the
		// next iteration of the loop to duplicate this rule and process it again
		// with color clipping enabled.
		if wouldClipColorFlag {
			if p.options.unsupportedCSSFeatures.Has(compat.ColorFunctions) {
				// Only do this if there was no previous instance of that property so
				// we avoid overwriting any manually-specified fallback values
				for j := len(rewrittenRules) - 2; j >= 0; j-- {
					if prev, ok := rewrittenRules[j].Data.(*css_ast.RDeclaration); ok && prev.Key == decl.Key {
						wouldClipColorFlag = false
						break
					}
				}
				if wouldClipColorFlag {
					// If the code above would have clipped a color outside of the sRGB gamut,
					// process this rule again so we can generate the clipped version next time
					i -= 1
					continue
				}
			}
			wouldClipColorFlag = false
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

func (p *parser) insertPrefixedDeclaration(rules []css_ast.Rule, prefix string, loc logger.Loc, decl *css_ast.RDeclaration, declarationKeys map[string]struct{}) []css_ast.Rule {
	keyText := prefix + decl.KeyText

	// Don't insert a prefixed declaration if there already is one
	if _, ok := declarationKeys[keyText]; ok {
		// We found a previous declaration with a matching prefixed property.
		// The value is ignored, which matches the behavior of "autoprefixer".
		return rules
	}

	// Additional special cases for when the prefix applies
	switch decl.Key {
	case css_ast.DBackgroundClip:
		// The prefix is only needed for "background-clip: text"
		if len(decl.Value) != 1 || decl.Value[0].Kind != css_lexer.TIdent || !strings.EqualFold(decl.Value[0].Text, "text") {
			return rules
		}

	case css_ast.DPosition:
		// The prefix is only needed for "position: sticky"
		if len(decl.Value) != 1 || decl.Value[0].Kind != css_lexer.TIdent || !strings.EqualFold(decl.Value[0].Text, "sticky") {
			return rules
		}
	}

	value := css_ast.CloneTokensWithoutImportRecords(decl.Value)

	// Additional special cases for how to transform the contents
	switch decl.Key {
	case css_ast.DPosition:
		// The prefix applies to the value, not the property
		keyText = decl.KeyText
		value[0].Text = "-webkit-sticky"

	case css_ast.DUserSelect:
		// The prefix applies to the value as well as the property
		if prefix == "-moz-" && len(value) == 1 && value[0].Kind == css_lexer.TIdent && strings.EqualFold(value[0].Text, "none") {
			value[0].Text = "-moz-none"
		}

	case css_ast.DMaskComposite:
		// WebKit uses different names for these values
		if prefix == "-webkit-" {
			for i, token := range value {
				if token.Kind == css_lexer.TIdent {
					switch token.Text {
					case "add":
						value[i].Text = "source-over"
					case "subtract":
						value[i].Text = "source-out"
					case "intersect":
						value[i].Text = "source-in"
					case "exclude":
						value[i].Text = "xor"
					}
				}
			}
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

func (p *parser) lowerInset(loc logger.Loc, decl *css_ast.RDeclaration) ([]css_ast.Rule, bool) {
	if tokens, ok := expandTokenQuad(decl.Value, ""); ok {
		mask := ^css_ast.WhitespaceAfter
		if p.options.minifyWhitespace {
			mask = 0
		}
		for i := range tokens {
			tokens[i].Whitespace &= mask
		}
		return []css_ast.Rule{
			{Loc: loc, Data: &css_ast.RDeclaration{
				KeyText:   "top",
				KeyRange:  decl.KeyRange,
				Key:       css_ast.DTop,
				Value:     tokens[0:1],
				Important: decl.Important,
			}},
			{Loc: loc, Data: &css_ast.RDeclaration{
				KeyText:   "right",
				KeyRange:  decl.KeyRange,
				Key:       css_ast.DRight,
				Value:     tokens[1:2],
				Important: decl.Important,
			}},
			{Loc: loc, Data: &css_ast.RDeclaration{
				KeyText:   "bottom",
				KeyRange:  decl.KeyRange,
				Key:       css_ast.DBottom,
				Value:     tokens[2:3],
				Important: decl.Important,
			}},
			{Loc: loc, Data: &css_ast.RDeclaration{
				KeyText:   "left",
				KeyRange:  decl.KeyRange,
				Key:       css_ast.DLeft,
				Value:     tokens[3:4],
				Important: decl.Important,
			}},
		}, true
	}
	return nil, false
}
