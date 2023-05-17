package css_parser

import (
	"fmt"

	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/logger"
)

type parseSelectorOpts struct {
	isDeclarationContext bool
}

func (p *parser) parseSelectorList(opts parseSelectorOpts) (list []css_ast.ComplexSelector, ok bool) {
	// Parse the first selector
	sel, good := p.parseComplexSelector(parseComplexSelectorOpts{
		parseSelectorOpts: opts,
		isFirst:           true,
	})
	if !good {
		return
	}
	list = append(list, sel)

	// Parse the remaining selectors
skip:
	for {
		p.eat(css_lexer.TWhitespace)
		if !p.eat(css_lexer.TComma) {
			break
		}
		p.eat(css_lexer.TWhitespace)
		sel, good := p.parseComplexSelector(parseComplexSelectorOpts{
			parseSelectorOpts: opts,
		})
		if !good {
			return
		}

		// Omit duplicate selectors
		if p.options.MinifySyntax {
			for _, existing := range list {
				if sel.Equal(existing, nil) {
					continue skip
				}
			}
		}

		list = append(list, sel)
	}

	if p.options.MinifySyntax {
		for i := 1; i < len(list); i++ {
			if analyzeLeadingAmpersand(list[i], opts.isDeclarationContext) != cannotRemoveLeadingAmpersand {
				list[i].Selectors = list[i].Selectors[1:]
			}
		}

		switch analyzeLeadingAmpersand(list[0], opts.isDeclarationContext) {
		case canAlwaysRemoveLeadingAmpersand:
			list[0].Selectors = list[0].Selectors[1:]

		case canRemoveLeadingAmpersandIfNotFirst:
			for i := 1; i < len(list); i++ {
				if sel := list[i].Selectors[0]; !sel.HasNestingSelector && (sel.Combinator != 0 || sel.TypeSelector == nil) {
					list[0].Selectors = list[0].Selectors[1:]
					list[0], list[i] = list[i], list[0]
					break
				}
			}
		}
	}

	ok = true
	return
}

type leadingAmpersand uint8

const (
	cannotRemoveLeadingAmpersand leadingAmpersand = iota
	canAlwaysRemoveLeadingAmpersand
	canRemoveLeadingAmpersandIfNotFirst
)

func analyzeLeadingAmpersand(sel css_ast.ComplexSelector, isDeclarationContext bool) leadingAmpersand {
	if len(sel.Selectors) > 1 {
		if first := sel.Selectors[0]; first.IsSingleAmpersand() {
			if second := sel.Selectors[1]; second.Combinator == 0 && second.HasNestingSelector {
				// ".foo { & &.bar {} }" => ".foo { & &.bar {} }"
			} else if second.Combinator != 0 || second.TypeSelector == nil || !isDeclarationContext {
				// "& + div {}" => "+ div {}"
				// "& div {}" => "div {}"
				// ".foo { & + div {} }" => ".foo { + div {} }"
				// ".foo { & + &.bar {} }" => ".foo { + &.bar {} }"
				// ".foo { & :hover {} }" => ".foo { :hover {} }"
				return canAlwaysRemoveLeadingAmpersand
			} else {
				// ".foo { & div {} }"
				// ".foo { .bar, & div {} }" => ".foo { .bar, div {} }"
				return canRemoveLeadingAmpersandIfNotFirst
			}
		}
	} else {
		// "& {}" => "& {}"
	}
	return cannotRemoveLeadingAmpersand
}

type parseComplexSelectorOpts struct {
	parseSelectorOpts
	isFirst bool
}

func (p *parser) parseComplexSelector(opts parseComplexSelectorOpts) (result css_ast.ComplexSelector, ok bool) {
	// This is an extension: https://drafts.csswg.org/css-nesting-1/
	r := p.current().Range
	combinator := p.parseCombinator()
	if combinator != 0 {
		p.reportUseOfNesting(r, opts.isDeclarationContext)
		p.eat(css_lexer.TWhitespace)
	}

	// Parent
	sel, good := p.parseCompoundSelector(parseComplexSelectorOpts{
		parseSelectorOpts: opts.parseSelectorOpts,
		isFirst:           opts.isFirst,
	})
	if !good {
		return
	}
	sel.Combinator = combinator
	result.Selectors = append(result.Selectors, sel)

	for {
		p.eat(css_lexer.TWhitespace)
		if p.peek(css_lexer.TEndOfFile) || p.peek(css_lexer.TComma) || p.peek(css_lexer.TOpenBrace) {
			break
		}

		// Optional combinator
		combinator := p.parseCombinator()
		if combinator != 0 {
			p.eat(css_lexer.TWhitespace)
		}

		// Child
		sel, good := p.parseCompoundSelector(parseComplexSelectorOpts{
			parseSelectorOpts: opts.parseSelectorOpts,
		})
		if !good {
			return
		}
		sel.Combinator = combinator
		result.Selectors = append(result.Selectors, sel)
	}

	ok = true
	return
}

func (p *parser) nameToken() css_ast.NameToken {
	return css_ast.NameToken{
		Kind: p.current().Kind,
		Text: p.decoded(),
	}
}

func (p *parser) parseCompoundSelector(opts parseComplexSelectorOpts) (sel css_ast.CompoundSelector, ok bool) {
	startLoc := p.current().Range.Loc

	// This is an extension: https://drafts.csswg.org/css-nesting-1/
	hasLeadingNestingSelector := p.peek(css_lexer.TDelimAmpersand)
	if hasLeadingNestingSelector {
		p.reportUseOfNesting(p.current().Range, opts.isDeclarationContext)
		sel.HasNestingSelector = true
		p.advance()
	}

	// Parse the type selector
	typeSelectorLoc := p.current().Range.Loc
	switch p.current().Kind {
	case css_lexer.TDelimBar, css_lexer.TIdent, css_lexer.TDelimAsterisk:
		nsName := css_ast.NamespacedName{}
		if !p.peek(css_lexer.TDelimBar) {
			nsName.Name = p.nameToken()
			p.advance()
		} else {
			// Hack: Create an empty "identifier" to represent this
			nsName.Name.Kind = css_lexer.TIdent
		}
		if p.eat(css_lexer.TDelimBar) {
			if !p.peek(css_lexer.TIdent) && !p.peek(css_lexer.TDelimAsterisk) {
				p.expect(css_lexer.TIdent)
				return
			}
			prefix := nsName.Name
			nsName.NamespacePrefix = &prefix
			nsName.Name = p.nameToken()
			p.advance()
		}
		sel.TypeSelector = &nsName
	}

	// Parse the subclass selectors
subclassSelectors:
	for {
		switch p.current().Kind {
		case css_lexer.THash:
			if (p.current().Flags & css_lexer.IsID) == 0 {
				break subclassSelectors
			}
			name := p.decoded()
			sel.SubclassSelectors = append(sel.SubclassSelectors, &css_ast.SSHash{Name: name})
			p.advance()

		case css_lexer.TDelimDot:
			p.advance()
			name := p.decoded()
			sel.SubclassSelectors = append(sel.SubclassSelectors, &css_ast.SSClass{Name: name})
			p.expect(css_lexer.TIdent)

		case css_lexer.TOpenBracket:
			attr, good := p.parseAttributeSelector()
			if !good {
				return
			}
			sel.SubclassSelectors = append(sel.SubclassSelectors, &attr)

		case css_lexer.TColon:
			if p.next().Kind == css_lexer.TColon {
				// Special-case the start of the pseudo-element selector section
				for p.current().Kind == css_lexer.TColon {
					isElement := p.next().Kind == css_lexer.TColon
					if isElement {
						p.advance()
					}
					pseudo := p.parsePseudoClassSelector()

					// https://www.w3.org/TR/selectors-4/#single-colon-pseudos
					// The four Level 2 pseudo-elements (::before, ::after, ::first-line,
					// and ::first-letter) may, for legacy reasons, be represented using
					// the <pseudo-class-selector> grammar, with only a single ":"
					// character at their start.
					if p.options.MinifySyntax && isElement && len(pseudo.Args) == 0 {
						switch pseudo.Name {
						case "before", "after", "first-line", "first-letter":
							isElement = false
						}
					}

					pseudo.IsElement = isElement
					sel.SubclassSelectors = append(sel.SubclassSelectors, &pseudo)
				}
				break subclassSelectors
			}
			pseudo := p.parsePseudoClassSelector()
			sel.SubclassSelectors = append(sel.SubclassSelectors, &pseudo)

		case css_lexer.TDelimAmpersand:
			// This is an extension: https://drafts.csswg.org/css-nesting-1/
			p.reportUseOfNesting(p.current().Range, sel.HasNestingSelector)
			sel.HasNestingSelector = true
			p.advance()

		default:
			break subclassSelectors
		}
	}

	// The compound selector must be non-empty
	if !sel.HasNestingSelector && sel.TypeSelector == nil && len(sel.SubclassSelectors) == 0 {
		p.unexpected()
		return
	}

	// Note: "&div {}" was originally valid, but is now an invalid selector:
	// https://github.com/w3c/csswg-drafts/issues/8662#issuecomment-1514977935.
	// This is because SASS already uses that syntax to mean something very
	// different, so that syntax has been removed to avoid mistakes.
	if hasLeadingNestingSelector && sel.TypeSelector != nil {
		r := logger.Range{Loc: typeSelectorLoc, Len: p.at(p.index-1).Range.End() - typeSelectorLoc.Start}
		text := sel.TypeSelector.Name.Text
		if sel.TypeSelector.NamespacePrefix != nil {
			text = fmt.Sprintf("%s|%s", sel.TypeSelector.NamespacePrefix.Text, text)
		}
		var howToFix string
		suggestion := p.source.TextForRange(r)
		if opts.isFirst {
			suggestion = fmt.Sprintf(":is(%s)", suggestion)
			howToFix = "You can wrap this selector in \":is()\" as a workaround. "
		} else {
			r = logger.Range{Loc: startLoc, Len: r.End() - startLoc.Start}
			suggestion += "&"
			howToFix = "You can move the \"&\" to the end of this selector as a workaround. "
		}
		msg := logger.Msg{
			Kind: logger.Warning,
			Data: p.tracker.MsgData(r, fmt.Sprintf("Cannot use type selector %q directly after nesting selector \"&\"", text)),
			Notes: []logger.MsgData{{Text: "CSS nesting syntax does not allow the \"&\" selector to come before a type selector. " +
				howToFix +
				"This restriction exists to avoid problems with SASS nesting, where the same syntax means something very different " +
				"that has no equivalent in real CSS (appending a suffix to the parent selector)."}},
		}
		msg.Data.Location.Suggestion = suggestion
		p.log.AddMsgID(logger.MsgID_CSS_CSSSyntaxError, msg)
		return
	}

	// The type selector must always come first
	switch p.current().Kind {
	case css_lexer.TDelimBar, css_lexer.TIdent, css_lexer.TDelimAsterisk:
		p.unexpected()
		return
	}

	ok = true
	return
}

func (p *parser) parseAttributeSelector() (attr css_ast.SSAttribute, ok bool) {
	matchingLoc := p.current().Range.Loc
	p.advance()

	// Parse the namespaced name
	switch p.current().Kind {
	case css_lexer.TDelimBar, css_lexer.TDelimAsterisk:
		// "[|x]"
		// "[*|x]"
		if p.peek(css_lexer.TDelimAsterisk) {
			prefix := p.nameToken()
			p.advance()
			attr.NamespacedName.NamespacePrefix = &prefix
		} else {
			// "[|attr]" is equivalent to "[attr]". From the specification:
			// "In keeping with the Namespaces in the XML recommendation, default
			// namespaces do not apply to attributes, therefore attribute selectors
			// without a namespace component apply only to attributes that have no
			// namespace (equivalent to |attr)."
		}
		if !p.expect(css_lexer.TDelimBar) {
			return
		}
		attr.NamespacedName.Name = p.nameToken()
		if !p.expect(css_lexer.TIdent) {
			return
		}

	default:
		// "[x]"
		// "[x|y]"
		attr.NamespacedName.Name = p.nameToken()
		if !p.expect(css_lexer.TIdent) {
			return
		}
		if p.next().Kind != css_lexer.TDelimEquals && p.eat(css_lexer.TDelimBar) {
			prefix := attr.NamespacedName.Name
			attr.NamespacedName.NamespacePrefix = &prefix
			attr.NamespacedName.Name = p.nameToken()
			if !p.expect(css_lexer.TIdent) {
				return
			}
		}
	}

	// Parse the optional matcher operator
	p.eat(css_lexer.TWhitespace)
	if p.eat(css_lexer.TDelimEquals) {
		attr.MatcherOp = "="
	} else {
		switch p.current().Kind {
		case css_lexer.TDelimTilde:
			attr.MatcherOp = "~="
		case css_lexer.TDelimBar:
			attr.MatcherOp = "|="
		case css_lexer.TDelimCaret:
			attr.MatcherOp = "^="
		case css_lexer.TDelimDollar:
			attr.MatcherOp = "$="
		case css_lexer.TDelimAsterisk:
			attr.MatcherOp = "*="
		}
		if attr.MatcherOp != "" {
			p.advance()
			p.expect(css_lexer.TDelimEquals)
		}
	}

	// Parse the optional matcher value
	if attr.MatcherOp != "" {
		p.eat(css_lexer.TWhitespace)
		if !p.peek(css_lexer.TString) && !p.peek(css_lexer.TIdent) {
			p.unexpected()
		}
		attr.MatcherValue = p.decoded()
		p.advance()
		p.eat(css_lexer.TWhitespace)
		if p.peek(css_lexer.TIdent) {
			if modifier := p.decoded(); len(modifier) == 1 {
				if c := modifier[0]; c == 'i' || c == 'I' || c == 's' || c == 'S' {
					attr.MatcherModifier = c
					p.advance()
				}
			}
		}
	}

	p.expectWithMatchingLoc(css_lexer.TCloseBracket, matchingLoc)
	ok = true
	return
}

func (p *parser) parsePseudoClassSelector() css_ast.SSPseudoClass {
	p.advance()

	if p.peek(css_lexer.TFunction) {
		text := p.decoded()
		matchingLoc := logger.Loc{Start: p.current().Range.End() - 1}
		p.advance()
		args := p.convertTokens(p.parseAnyValue())
		p.expectWithMatchingLoc(css_lexer.TCloseParen, matchingLoc)
		return css_ast.SSPseudoClass{Name: text, Args: args}
	}

	name := p.decoded()
	sel := css_ast.SSPseudoClass{}
	if p.expect(css_lexer.TIdent) {
		sel.Name = name
	}
	return sel
}

func (p *parser) parseAnyValue() []css_lexer.Token {
	// Reference: https://drafts.csswg.org/css-syntax-3/#typedef-declaration-value

	p.stack = p.stack[:0] // Reuse allocated memory
	start := p.index

loop:
	for {
		switch p.current().Kind {
		case css_lexer.TCloseParen, css_lexer.TCloseBracket, css_lexer.TCloseBrace:
			last := len(p.stack) - 1
			if last < 0 || !p.peek(p.stack[last]) {
				break loop
			}
			p.stack = p.stack[:last]

		case css_lexer.TSemicolon, css_lexer.TDelimExclamation:
			if len(p.stack) == 0 {
				break loop
			}

		case css_lexer.TOpenParen, css_lexer.TFunction:
			p.stack = append(p.stack, css_lexer.TCloseParen)

		case css_lexer.TOpenBracket:
			p.stack = append(p.stack, css_lexer.TCloseBracket)

		case css_lexer.TOpenBrace:
			p.stack = append(p.stack, css_lexer.TCloseBrace)

		case css_lexer.TEndOfFile:
			break loop
		}

		p.advance()
	}

	tokens := p.tokens[start:p.index]
	if len(tokens) == 0 {
		p.unexpected()
	}
	return tokens
}

func (p *parser) parseCombinator() uint8 {
	switch p.current().Kind {
	case css_lexer.TDelimGreaterThan:
		p.advance()
		return '>'

	case css_lexer.TDelimPlus:
		p.advance()
		return '+'

	case css_lexer.TDelimTilde:
		p.advance()
		return '~'

	default:
		return 0
	}
}
