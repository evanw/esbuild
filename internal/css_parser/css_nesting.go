package css_parser

import (
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

func lowerNestingInRule(rule css_ast.Rule, results []css_ast.Rule) []css_ast.Rule {
	switch r := rule.Data.(type) {
	case *css_ast.RSelector:
		// Filter out pseudo elements because they are ignored by nested style
		// rules. This is because pseudo-elements are not valid within :is():
		// https://www.w3.org/TR/selectors-4/#matches-pseudo. This restriction
		// may be relaxed in the future, but this restriction hash shipped so
		// we're stuck with it: https://github.com/w3c/csswg-drafts/issues/7433.
		selectors := r.Selectors
		n := 0
		for _, sel := range selectors {
			if !sel.UsesPseudoElement() {
				selectors[n] = sel
				n++
			}
		}
		selectors = selectors[:n]

		// Emit this selector before its nested children
		start := len(results)
		results = append(results, rule)

		// Lower all children and filter out ones that become empty
		context := lowerNestingContext{
			parentSelectors: selectors,
			loweredRules:    results,
		}
		r.Rules = lowerNestingInRulesAndReturnRemaining(r.Rules, &context)

		// Omit this selector entirely if it's now empty
		if len(r.Rules) == 0 {
			copy(context.loweredRules[start:], context.loweredRules[start+1:])
			context.loweredRules = context.loweredRules[:len(context.loweredRules)-1]
		}
		return context.loweredRules

	case *css_ast.RKnownAt:
		var rules []css_ast.Rule
		for _, child := range r.Rules {
			rules = lowerNestingInRule(child, rules)
		}
		r.Rules = rules

	case *css_ast.RAtLayer:
		var rules []css_ast.Rule
		for _, child := range r.Rules {
			rules = lowerNestingInRule(child, rules)
		}
		r.Rules = rules
	}

	return append(results, rule)
}

// Lower all children and filter out ones that become empty
func lowerNestingInRulesAndReturnRemaining(rules []css_ast.Rule, context *lowerNestingContext) []css_ast.Rule {
	n := 0
	for _, child := range rules {
		child = lowerNestingInRuleWithContext(child, context)
		if child.Data != nil {
			rules[n] = child
			n++
		}
	}
	return rules[:n]
}

type lowerNestingContext struct {
	parentSelectors []css_ast.ComplexSelector
	loweredRules    []css_ast.Rule
}

func lowerNestingInRuleWithContext(rule css_ast.Rule, context *lowerNestingContext) css_ast.Rule {
	switch r := rule.Data.(type) {
	case *css_ast.RSelector:
		// "a { & b {} }" => "a b {}"
		// "a { &b {} }" => "a:is(b) {}"
		// "a { &:hover {} }" => "a:hover {}"
		// ".x { &b {} }" => "b.x {}"
		// "a, b { .c, d {} }" => ":is(a, b) :is(.c, d) {}"
		// "a, b { &.c, & d, e & {} }" => ":is(a, b).c, :is(a, b) d, e :is(a, b) {}"

		// Pass 1: Canonicalize and analyze our selectors
		canUseGroupDescendantCombinator := true // Can we do "parent «space» :is(...selectors)"?
		canUseGroupSubSelector := true          // Can we do "parent«nospace»:is(...selectors)"?
		for i := range r.Selectors {
			sel := &r.Selectors[i]

			// Inject the implicit "&" now for simplicity later on
			if sel.IsRelative() {
				sel.Selectors = append([]css_ast.CompoundSelector{{HasNestingSelector: true}}, sel.Selectors...)
			}

			// Are all children of the form "& «something»"?
			if len(sel.Selectors) < 2 || !sel.Selectors[0].IsSingleAmpersand() {
				canUseGroupDescendantCombinator = false
			}

			// Are all children of the form "&«something»"?
			if first := sel.Selectors[0]; !first.HasNestingSelector || first.IsSingleAmpersand() {
				canUseGroupSubSelector = false
			}
		}

		// Try to apply simplifications for shorter output
		if canUseGroupDescendantCombinator {
			// "& a, & b {}" => "& :is(a, b) {}"
			for i := range r.Selectors {
				sel := &r.Selectors[i]
				sel.Selectors = sel.Selectors[1:]
			}
			merged := multipleComplexSelectorsToSingleComplexSelector(r.Selectors)
			merged.Selectors = append([]css_ast.CompoundSelector{{HasNestingSelector: true}}, merged.Selectors...)
			r.Selectors = []css_ast.ComplexSelector{merged}
		} else if canUseGroupSubSelector {
			// "&a, &b {}" => "&:is(a, b) {}"
			for i := range r.Selectors {
				sel := &r.Selectors[i]
				sel.Selectors[0].HasNestingSelector = false
			}
			merged := multipleComplexSelectorsToSingleComplexSelector(r.Selectors)
			merged.Selectors[0].HasNestingSelector = true
			r.Selectors = []css_ast.ComplexSelector{merged}
		}

		// Pass 2: Substitue "&" for the parent selector
		for i := range r.Selectors {
			complex := &r.Selectors[i]
			results := make([]css_ast.CompoundSelector, 0, len(complex.Selectors))
			parent := multipleComplexSelectorsToSingleComplexSelector(context.parentSelectors)
			for _, compound := range complex.Selectors {
				results = substituteAmpersandsInCompoundSelector(compound, parent, results)
			}
			complex.Selectors = results
		}

		// Lower all child rules using our newly substituted selector
		context.loweredRules = lowerNestingInRule(rule, context.loweredRules)
		return css_ast.Rule{}

	case *css_ast.RKnownAt:
		childContext := lowerNestingContext{parentSelectors: context.parentSelectors}
		r.Rules = lowerNestingInRulesAndReturnRemaining(r.Rules, &childContext)

		// "div { @media screen { color: red } }" "@media screen { div { color: red } }"
		if len(r.Rules) > 0 {
			childContext.loweredRules = append([]css_ast.Rule{{Loc: rule.Loc, Data: &css_ast.RSelector{
				Selectors: context.parentSelectors,
				Rules:     r.Rules,
			}}}, childContext.loweredRules...)
		}

		// "div { @media screen { &:hover { color: red } } }" "@media screen { div:hover { color: red } }"
		if len(childContext.loweredRules) > 0 {
			r.Rules = childContext.loweredRules
			context.loweredRules = append(context.loweredRules, rule)
		}

		return css_ast.Rule{}

	case *css_ast.RAtLayer:
		// Lower all children and filter out ones that become empty
		childContext := lowerNestingContext{parentSelectors: context.parentSelectors}
		r.Rules = lowerNestingInRulesAndReturnRemaining(r.Rules, &childContext)

		// "div { @layer foo { color: red } }" "@layer foo { div { color: red } }"
		if len(r.Rules) > 0 {
			childContext.loweredRules = append([]css_ast.Rule{{Loc: rule.Loc, Data: &css_ast.RSelector{
				Selectors: context.parentSelectors,
				Rules:     r.Rules,
			}}}, childContext.loweredRules...)
		}

		// "div { @layer foo { &:hover { color: red } } }" "@layer foo { div:hover { color: red } }"
		// "div { @layer foo {} }" => "@layer foo {}" (layers have side effects, so don't remove empty ones)
		r.Rules = childContext.loweredRules
		context.loweredRules = append(context.loweredRules, rule)
		return css_ast.Rule{}
	}

	return rule
}

func substituteAmpersandsInCompoundSelector(sel css_ast.CompoundSelector, replacement css_ast.ComplexSelector, results []css_ast.CompoundSelector) []css_ast.CompoundSelector {
	if sel.HasNestingSelector {
		sel.HasNestingSelector = false

		// Convert the replacement to a single compound selector
		var single css_ast.CompoundSelector
		if len(replacement.Selectors) == 1 || len(results) == 0 {
			// ".foo { :hover & {} }" => ":hover .foo {}"
			// ".foo .bar { &:hover {} }" => ".foo .bar:hover {}"
			last := len(replacement.Selectors) - 1
			results = append(results, replacement.Selectors[:last]...)
			single = replacement.Selectors[last]
		} else {
			// ".foo .bar { :hover & {} }" => ":hover :is(.foo .bar) {}"
			single = css_ast.CompoundSelector{
				SubclassSelectors: []css_ast.SS{&css_ast.SSPseudoClass{
					Name: "is",
					Args: replacement.AppendToTokens(nil),
				}},
			}
		}

		var subclassSelectorPrefix []css_ast.SS

		// Insert the type selector
		if single.TypeSelector != nil {
			if sel.TypeSelector != nil {
				subclassSelectorPrefix = append(subclassSelectorPrefix, &css_ast.SSPseudoClass{
					Name: "is",
					Args: sel.TypeSelector.AppendToTokens(nil),
				})
			}
			sel.TypeSelector = single.TypeSelector
		}

		// Insert the subclass selectors
		subclassSelectorPrefix = append(subclassSelectorPrefix, single.SubclassSelectors...)

		// Write the changes back
		if len(subclassSelectorPrefix) > 0 {
			sel.SubclassSelectors = append(subclassSelectorPrefix, sel.SubclassSelectors...)
		}
	}

	// "div { :is(&.foo) {} }" => ":is(div.foo) {}"
	for _, ss := range sel.SubclassSelectors {
		if class, ok := ss.(*css_ast.SSPseudoClass); ok {
			class.Args = substituteAmpersandsInTokens(class.Args, replacement)
		}
	}

	return append(results, sel)
}

func substituteAmpersandsInTokens(tokens []css_ast.Token, replacement css_ast.ComplexSelector) []css_ast.Token {
	foundAmpersand := false

	for _, t := range tokens {
		if t.Children != nil {
			*t.Children = substituteAmpersandsInTokens(*t.Children, replacement)
		}
		if t.Kind == css_lexer.TDelimAmpersand {
			foundAmpersand = true
		}
	}

	// We only need to allocate if we find an ampersand
	if !foundAmpersand {
		return tokens
	}

	var results []css_ast.Token
	replacementTokens := replacement.AppendToTokens(nil)
	for _, t := range tokens {
		if t.Kind != css_lexer.TDelimAmpersand {
			results = append(results, t)
			continue
		}

		insert := replacementTokens

		// Try to avoid generating a bad substitution when "&" is in a weird place.
		// This is necessary because we're operating on a list of tokens instead of
		// a fully-parsed AST. Here's an example:
		//
		//   "div { :is(.foo&) {} }" => ":is(.foo:is(div))"
		//   "div { :is(#foo&) {} }" => ":is(#foo:is(div))"
		//   "div { :is([foo]&) {} }" => ":is([foo]:is(div))"
		//   "div { :is(:hover&) {} }" => ":is(:hover:is(div))"
		//   "div { :is(:is(.foo)&) {} }" => ":is(:is(.foo):is(div))"
		//
		// There are likely a lot of edge cases with this that aren't covered. But
		// this is probably fine because a) who would write code like this and b)
		// PostCSS's nesting transform doesn't handle these edge cases at all.
		if len(results) > 0 && len(insert) > 0 {
			if first := insert[0]; first.Kind == css_lexer.TIdent && (first.Whitespace&css_ast.WhitespaceBefore) == 0 {
				if last := results[len(results)-1]; (last.Whitespace & css_ast.WhitespaceAfter) == 0 {
					switch last.Kind {
					case css_lexer.TIdent, css_lexer.TOpenBracket, css_lexer.TFunction:
						insert = []css_ast.Token{
							{Kind: css_lexer.TColon, Text: ":"},
							{Kind: css_lexer.TFunction, Text: "is", Children: &replacementTokens},
						}
					}
				}
			}
		}

		results = append(results, insert...)
	}
	return results
}

// Turn the list of selectors into a single selector by wrapping lists
// without a single element with ":is(...)". Note that this may result
// in an empty ":is()" selector (which matches nothing).
func multipleComplexSelectorsToSingleComplexSelector(selectors []css_ast.ComplexSelector) css_ast.ComplexSelector {
	if len(selectors) == 1 {
		return selectors[0]
	}

	// This must be non-nil so we print ":is()" instead of ":is"
	tokens := []css_ast.Token{}

	for i, sel := range selectors {
		if i > 0 {
			tokens = append(tokens, css_ast.Token{Kind: css_lexer.TComma, Text: ",", Whitespace: css_ast.WhitespaceAfter})
		}
		tokens = sel.AppendToTokens(tokens)
	}

	return css_ast.ComplexSelector{
		Selectors: []css_ast.CompoundSelector{{
			SubclassSelectors: []css_ast.SS{&css_ast.SSPseudoClass{
				Name: "is",
				Args: tokens,
			}},
		}},
	}
}
