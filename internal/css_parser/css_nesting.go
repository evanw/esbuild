package css_parser

import (
	"fmt"

	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/logger"
)

func (p *parser) lowerNestingInRule(rule css_ast.Rule, results []css_ast.Rule) []css_ast.Rule {
	switch r := rule.Data.(type) {
	case *css_ast.RSelector:
		scope := func(loc logger.Loc) css_ast.ComplexSelector {
			return css_ast.ComplexSelector{
				Selectors: []css_ast.CompoundSelector{{
					SubclassSelectors: []css_ast.SubclassSelector{{
						Range: logger.Range{Loc: loc},
						Data:  &css_ast.SSPseudoClass{Name: "scope"},
					}},
				}},
			}
		}

		parentSelectors := make([]css_ast.ComplexSelector, 0, len(r.Selectors))
		for i, sel := range r.Selectors {
			// Top-level "&" should be replaced with ":scope" to avoid recursion.
			// From https://www.w3.org/TR/css-nesting-1/#nest-selector:
			//
			//   "When used in the selector of a nested style rule, the nesting
			//   selector represents the elements matched by the parent rule. When
			//   used in any other context, it represents the same elements as
			//   :scope in that context (unless otherwise defined)."
			//
			substituted := make([]css_ast.CompoundSelector, 0, len(sel.Selectors))
			for _, x := range sel.Selectors {
				substituted = p.substituteAmpersandsInCompoundSelector(x, scope, substituted, keepLeadingCombinator)
			}
			r.Selectors[i] = css_ast.ComplexSelector{Selectors: substituted}

			// Filter out pseudo elements because they are ignored by nested style
			// rules. This is because pseudo-elements are not valid within :is():
			// https://www.w3.org/TR/selectors-4/#matches-pseudo. This restriction
			// may be relaxed in the future, but this restriction hash shipped so
			// we're stuck with it: https://github.com/w3c/csswg-drafts/issues/7433.
			//
			// Note: This is only for the parent selector list that is used to
			// substitute "&" within child rules. Do not filter out the pseudo
			// element from the top-level selector list.
			if !sel.UsesPseudoElement() {
				parentSelectors = append(parentSelectors, css_ast.ComplexSelector{Selectors: substituted})
			}
		}

		// Emit this selector before its nested children
		start := len(results)
		results = append(results, rule)

		// Lower all children and filter out ones that become empty
		context := lowerNestingContext{
			parentSelectors: parentSelectors,
			loweredRules:    results,
		}
		r.Rules = p.lowerNestingInRulesAndReturnRemaining(r.Rules, &context)

		// Omit this selector entirely if it's now empty
		if len(r.Rules) == 0 {
			copy(context.loweredRules[start:], context.loweredRules[start+1:])
			context.loweredRules = context.loweredRules[:len(context.loweredRules)-1]
		}
		return context.loweredRules

	case *css_ast.RKnownAt:
		var rules []css_ast.Rule
		for _, child := range r.Rules {
			rules = p.lowerNestingInRule(child, rules)
		}
		r.Rules = rules

	case *css_ast.RAtLayer:
		var rules []css_ast.Rule
		for _, child := range r.Rules {
			rules = p.lowerNestingInRule(child, rules)
		}
		r.Rules = rules
	}

	return append(results, rule)
}

// Lower all children and filter out ones that become empty
func (p *parser) lowerNestingInRulesAndReturnRemaining(rules []css_ast.Rule, context *lowerNestingContext) []css_ast.Rule {
	n := 0
	for _, child := range rules {
		child = p.lowerNestingInRuleWithContext(child, context)
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

func (p *parser) lowerNestingInRuleWithContext(rule css_ast.Rule, context *lowerNestingContext) css_ast.Rule {
	switch r := rule.Data.(type) {
	case *css_ast.RSelector:
		// "a { & b {} }" => "a b {}"
		// "a { &b {} }" => "a:is(b) {}"
		// "a { &:hover {} }" => "a:hover {}"
		// ".x { &b {} }" => "b.x {}"
		// "a, b { .c, d {} }" => ":is(a, b) :is(.c, d) {}"
		// "a, b { &.c, & d, e & {} }" => ":is(a, b).c, :is(a, b) d, e :is(a, b) {}"

		// Pass 1: Canonicalize and analyze our selectors
		for i := range r.Selectors {
			sel := &r.Selectors[i]

			// Inject the implicit "&" now for simplicity later on
			if sel.IsRelative() {
				sel.Selectors = append([]css_ast.CompoundSelector{{NestingSelectorLocs: []logger.Loc{rule.Loc}}}, sel.Selectors...)
			}
		}

		// Pass 2: Substitute "&" for the parent selector
		if !p.options.unsupportedCSSFeatures.Has(compat.IsPseudoClass) || len(context.parentSelectors) <= 1 {
			// If we can use ":is", or we don't have to because there's only one
			// parent selector, or we are using ":is()" to match zero parent selectors
			// (even if ":is" is unsupported), then substituting "&" for the parent
			// selector is easy.
			for i := range r.Selectors {
				complex := &r.Selectors[i]
				results := make([]css_ast.CompoundSelector, 0, len(complex.Selectors))
				parent := p.multipleComplexSelectorsToSingleComplexSelector(context.parentSelectors)
				for _, compound := range complex.Selectors {
					results = p.substituteAmpersandsInCompoundSelector(compound, parent, results, keepLeadingCombinator)
				}
				complex.Selectors = results
			}
		} else {
			// Otherwise if we can't use ":is", the transform is more complicated.
			// Avoiding ":is" can lead to a combinatorial explosion of cases so we
			// want to avoid this if possible. For example:
			//
			//   .first, .second, .third {
			//     & > & {
			//       color: red;
			//     }
			//   }
			//
			// If we can use ":is" (the easy case above) then we can do this:
			//
			//   :is(.first, .second, .third) > :is(.first, .second, .third) {
			//     color: red;
			//   }
			//
			// But if we can't use ":is" then we have to do this instead:
			//
			//   .first > .first,
			//   .first > .second,
			//   .first > .third,
			//   .second > .first,
			//   .second > .second,
			//   .second > .third,
			//   .third > .first,
			//   .third > .second,
			//   .third > .third {
			//     color: red;
			//   }
			//
			// That combinatorial explosion is what the loop below implements. Note
			// that PostCSS's implementation of nesting gets this wrong. It generates
			// this instead:
			//
			//   .first > .first,
			//   .second > .second,
			//   .third > .third {
			//     color: red;
			//   }
			//
			// That's not equivalent, so that's an incorrect transformation.
			var selectors []css_ast.ComplexSelector
			var indices []int
			for {
				// Every time we encounter another "&", add another dimension
				offset := 0
				parent := func(loc logger.Loc) css_ast.ComplexSelector {
					if offset == len(indices) {
						indices = append(indices, 0)
					}
					index := indices[offset]
					offset++
					return context.parentSelectors[index]
				}

				// Do the substitution for this particular combination
				for i := range r.Selectors {
					complex := r.Selectors[i]
					results := make([]css_ast.CompoundSelector, 0, len(complex.Selectors))
					for _, compound := range complex.Selectors {
						results = p.substituteAmpersandsInCompoundSelector(compound, parent, results, keepLeadingCombinator)
					}
					complex.Selectors = results
					selectors = append(selectors, complex)
					offset = 0
				}

				// Do addition with carry on the indices across dimensions
				carry := len(indices)
				for carry > 0 {
					index := &indices[carry-1]
					if *index+1 < len(context.parentSelectors) {
						*index++
						break
					}
					*index = 0
					carry--
				}
				if carry == 0 {
					break
				}
			}
			r.Selectors = selectors
		}

		// Lower all child rules using our newly substituted selector
		context.loweredRules = p.lowerNestingInRule(rule, context.loweredRules)
		return css_ast.Rule{}

	case *css_ast.RKnownAt:
		childContext := lowerNestingContext{parentSelectors: context.parentSelectors}
		r.Rules = p.lowerNestingInRulesAndReturnRemaining(r.Rules, &childContext)

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
		r.Rules = p.lowerNestingInRulesAndReturnRemaining(r.Rules, &childContext)

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

type leadingCombinatorStrip uint8

const (
	keepLeadingCombinator leadingCombinatorStrip = iota
	stripLeadingCombinator
)

func (p *parser) substituteAmpersandsInCompoundSelector(
	sel css_ast.CompoundSelector,
	replacementFn func(logger.Loc) css_ast.ComplexSelector,
	results []css_ast.CompoundSelector,
	strip leadingCombinatorStrip,
) []css_ast.CompoundSelector {
	for _, nestingSelectorLoc := range sel.NestingSelectorLocs {
		replacement := replacementFn(nestingSelectorLoc)

		// Convert the replacement to a single compound selector
		var single css_ast.CompoundSelector
		if sel.Combinator.Byte == 0 && (len(replacement.Selectors) == 1 || len(results) == 0) {
			// ".foo { :hover & {} }" => ":hover .foo {}"
			// ".foo .bar { &:hover {} }" => ".foo .bar:hover {}"
			last := len(replacement.Selectors) - 1
			results = append(results, replacement.Selectors[:last]...)
			single = replacement.Selectors[last]
			if strip == stripLeadingCombinator {
				single.Combinator = css_ast.Combinator{}
			}
			sel.Combinator = single.Combinator
		} else if len(replacement.Selectors) == 1 {
			// ".foo { > &:hover {} }" => ".foo > .foo:hover {}"
			single = replacement.Selectors[0]
			if strip == stripLeadingCombinator {
				single.Combinator = css_ast.Combinator{}
			}
		} else {
			// ".foo .bar { :hover & {} }" => ":hover :is(.foo .bar) {}"
			// ".foo .bar { > &:hover {} }" => ".foo .bar > :is(.foo .bar):hover {}"
			p.reportNestingWithGeneratedPseudoClassIs(nestingSelectorLoc)
			single = css_ast.CompoundSelector{
				SubclassSelectors: []css_ast.SubclassSelector{{
					Range: logger.Range{Loc: nestingSelectorLoc},
					Data: &css_ast.SSPseudoClassWithSelectorList{
						Kind:      css_ast.PseudoClassIs,
						Selectors: []css_ast.ComplexSelector{replacement.Clone()},
					},
				}},
			}
		}

		var subclassSelectorPrefix []css_ast.SubclassSelector

		// Insert the type selector
		if single.TypeSelector != nil {
			if sel.TypeSelector != nil {
				p.reportNestingWithGeneratedPseudoClassIs(nestingSelectorLoc)
				subclassSelectorPrefix = append(subclassSelectorPrefix, css_ast.SubclassSelector{
					Range: sel.TypeSelector.Range(),
					Data: &css_ast.SSPseudoClassWithSelectorList{
						Kind:      css_ast.PseudoClassIs,
						Selectors: []css_ast.ComplexSelector{{Selectors: []css_ast.CompoundSelector{{TypeSelector: sel.TypeSelector}}}},
					},
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
	sel.NestingSelectorLocs = nil

	// "div { :is(&.foo) {} }" => ":is(div.foo) {}"
	for _, ss := range sel.SubclassSelectors {
		if class, ok := ss.Data.(*css_ast.SSPseudoClassWithSelectorList); ok {
			outer := make([]css_ast.ComplexSelector, 0, len(class.Selectors))
			for _, complex := range class.Selectors {
				inner := make([]css_ast.CompoundSelector, 0, len(complex.Selectors))
				for _, sel := range complex.Selectors {
					inner = p.substituteAmpersandsInCompoundSelector(sel, replacementFn, inner, stripLeadingCombinator)
				}
				outer = append(outer, css_ast.ComplexSelector{Selectors: inner})
			}
			class.Selectors = outer
		}
	}

	return append(results, sel)
}

// Turn the list of selectors into a single selector by wrapping lists
// without a single element with ":is(...)". Note that this may result
// in an empty ":is()" selector (which matches nothing).
func (p *parser) multipleComplexSelectorsToSingleComplexSelector(selectors []css_ast.ComplexSelector) func(logger.Loc) css_ast.ComplexSelector {
	if len(selectors) == 1 {
		return func(logger.Loc) css_ast.ComplexSelector {
			return selectors[0]
		}
	}

	var leadingCombinator css_ast.Combinator
	clones := make([]css_ast.ComplexSelector, len(selectors))

	for i, sel := range selectors {
		// "> a, > b" => "> :is(a, b)" (the caller should have already checked that all leading combinators are the same)
		leadingCombinator = sel.Selectors[0].Combinator
		clones[i] = sel.Clone()
	}

	return func(loc logger.Loc) css_ast.ComplexSelector {
		return css_ast.ComplexSelector{
			Selectors: []css_ast.CompoundSelector{{
				Combinator: leadingCombinator,
				SubclassSelectors: []css_ast.SubclassSelector{{
					Range: logger.Range{Loc: loc},
					Data: &css_ast.SSPseudoClassWithSelectorList{
						Kind:      css_ast.PseudoClassIs,
						Selectors: clones,
					},
				}},
			}},
		}
	}
}

func (p *parser) reportNestingWithGeneratedPseudoClassIs(nestingSelectorLoc logger.Loc) {
	if p.options.unsupportedCSSFeatures.Has(compat.IsPseudoClass) {
		_, didWarn := p.nestingWarnings[nestingSelectorLoc]
		if didWarn {
			// Only warn at each location once
			return
		}
		if p.nestingWarnings == nil {
			p.nestingWarnings = make(map[logger.Loc]struct{})
		}
		p.nestingWarnings[nestingSelectorLoc] = struct{}{}
		text := "Transforming this CSS nesting syntax is not supported in the configured target environment"
		if p.options.originalTargetEnv != "" {
			text = fmt.Sprintf("%s (%s)", text, p.options.originalTargetEnv)
		}
		r := logger.Range{Loc: nestingSelectorLoc, Len: 1}
		p.log.AddIDWithNotes(logger.MsgID_CSS_UnsupportedCSSNesting, logger.Warning, &p.tracker, r, text, []logger.MsgData{{
			Text: "The nesting transform for this case must generate an \":is(...)\" but the configured target environment does not support the \":is\" pseudo-class."}})
	}
}
