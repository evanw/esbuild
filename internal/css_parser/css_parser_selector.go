package css_parser

import (
	"fmt"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/logger"
)

type parseSelectorOpts struct {
	pseudoClassKind        css_ast.PseudoClassKind
	isDeclarationContext   bool
	stopOnCloseParen       bool
	onlyOneComplexSelector bool
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
	list = p.flattenLocalAndGlobalSelectors(list, sel)

	// Parse the remaining selectors
	if opts.onlyOneComplexSelector {
		if t := p.current(); t.Kind == css_lexer.TComma {
			p.prevError = t.Range.Loc
			kind := fmt.Sprintf(":%s(...)", opts.pseudoClassKind.String())
			p.log.AddIDWithNotes(logger.MsgID_CSS_CSSSyntaxError, logger.Warning, &p.tracker, t.Range,
				fmt.Sprintf("Unexpected \",\" inside %q", kind),
				[]logger.MsgData{{Text: fmt.Sprintf("Different CSS tools behave differently in this case, so esbuild doesn't allow it. "+
					"Either remove this comma or split this selector up into multiple comma-separated %q selectors instead.", kind)}})
		}
	} else {
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
			if p.options.minifySyntax {
				for _, existing := range list {
					if sel.Equal(existing, nil) {
						continue skip
					}
				}
			}

			list = p.flattenLocalAndGlobalSelectors(list, sel)
		}
	}

	if p.options.minifySyntax {
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
				if sel := list[i].Selectors[0]; !sel.HasNestingSelector() && (sel.Combinator.Byte != 0 || sel.TypeSelector == nil) {
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

func mergeCompoundSelectors(target *css_ast.CompoundSelector, source css_ast.CompoundSelector) {
	// ".foo:local(&)" => "&.foo"
	if source.HasNestingSelector() && !target.HasNestingSelector() {
		target.NestingSelectorLoc = source.NestingSelectorLoc
	}

	if source.TypeSelector != nil {
		if target.TypeSelector == nil {
			// ".foo:local(div)" => "div.foo"
			target.TypeSelector = source.TypeSelector
		} else {
			// "div:local(span)" => "div:is(span)"
			//
			// Note: All other implementations of this (Lightning CSS, PostCSS, and
			// Webpack) do something really weird here. They do this instead:
			//
			// "div:local(span)" => "divspan"
			//
			// But that just seems so obviously wrong that I'm not going to do that.
			target.SubclassSelectors = append(target.SubclassSelectors, css_ast.SubclassSelector{
				Loc: source.TypeSelector.FirstLoc(),
				Data: &css_ast.SSPseudoClassWithSelectorList{
					Kind:      css_ast.PseudoClassIs,
					Selectors: []css_ast.ComplexSelector{{Selectors: []css_ast.CompoundSelector{{TypeSelector: source.TypeSelector}}}},
				},
			})
		}
	}

	// ".foo:local(.bar)" => ".foo.bar"
	target.SubclassSelectors = append(target.SubclassSelectors, source.SubclassSelectors...)
}

func containsLocalOrGlobalSelector(sel css_ast.ComplexSelector) bool {
	for _, s := range sel.Selectors {
		for _, ss := range s.SubclassSelectors {
			switch pseudo := ss.Data.(type) {
			case *css_ast.SSPseudoClass:
				if pseudo.Name == "global" || pseudo.Name == "local" {
					return true
				}

			case *css_ast.SSPseudoClassWithSelectorList:
				if pseudo.Kind == css_ast.PseudoClassGlobal || pseudo.Kind == css_ast.PseudoClassLocal {
					return true
				}
			}
		}
	}
	return false
}

// This handles the ":local()" and ":global()" annotations from CSS modules
func (p *parser) flattenLocalAndGlobalSelectors(list []css_ast.ComplexSelector, sel css_ast.ComplexSelector) []css_ast.ComplexSelector {
	// Only do the work to flatten the whole list if there's a ":local" or a ":global"
	if p.options.symbolMode != symbolModeDisabled && containsLocalOrGlobalSelector(sel) {
		var selectors []css_ast.CompoundSelector

		for _, s := range sel.Selectors {
			oldSubclassSelectors := s.SubclassSelectors
			s.SubclassSelectors = make([]css_ast.SubclassSelector, 0, len(oldSubclassSelectors))

			for _, ss := range oldSubclassSelectors {
				switch pseudo := ss.Data.(type) {
				case *css_ast.SSPseudoClass:
					if pseudo.Name == "global" || pseudo.Name == "local" {
						// Remove bare ":global" and ":local" pseudo-classes
						continue
					}

				case *css_ast.SSPseudoClassWithSelectorList:
					if pseudo.Kind == css_ast.PseudoClassGlobal || pseudo.Kind == css_ast.PseudoClassLocal {
						inner := pseudo.Selectors[0].Selectors

						// Replace this pseudo-class with all inner compound selectors.
						// The first inner compound selector is merged with the compound
						// selector before it and the last inner compound selector is
						// merged with the compound selector after it:
						//
						// "div:local(.a .b):hover" => "div.a b:hover"
						//
						// This behavior is really strange since this is not how anything
						// involving pseudo-classes in real CSS works at all. However, all
						// other implementations (Lightning CSS, PostCSS, and Webpack) are
						// consistent with this strange behavior, so we do it too.
						if inner[0].Combinator.Byte == 0 {
							mergeCompoundSelectors(&s, inner[0])
							inner = inner[1:]
						} else {
							// "div:local(+ .foo):hover" => "div + .foo:hover"
						}
						if n := len(inner); n > 0 {
							if !s.IsInvalidBecauseEmpty() {
								// Don't add this selector if it consisted only of a bare ":global" or ":local"
								selectors = append(selectors, s)
							}
							selectors = append(selectors, inner[:n-1]...)
							s = inner[n-1]
						}
						continue
					}
				}

				s.SubclassSelectors = append(s.SubclassSelectors, ss)
			}

			if !s.IsInvalidBecauseEmpty() {
				// Don't add this selector if it consisted only of a bare ":global" or ":local"
				selectors = append(selectors, s)
			}
		}

		if len(selectors) == 0 {
			// Treat a bare ":global" or ":local" as a bare "&" nesting selector
			selectors = append(selectors, css_ast.CompoundSelector{
				NestingSelectorLoc: ast.MakeIndex32(uint32(sel.Selectors[0].FirstLoc().Start)),
			})

			// Make sure we report that nesting is present so that it can be lowered
			p.shouldLowerNesting = true
		}

		sel.Selectors = selectors
	}

	return append(list, sel)
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
			if second := sel.Selectors[1]; second.Combinator.Byte == 0 && second.HasNestingSelector() {
				// ".foo { & &.bar {} }" => ".foo { & &.bar {} }"
			} else if second.Combinator.Byte != 0 || second.TypeSelector == nil || !isDeclarationContext {
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
	if combinator.Byte != 0 {
		p.shouldLowerNesting = true
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

	stop := css_lexer.TOpenBrace
	if opts.stopOnCloseParen {
		stop = css_lexer.TCloseParen
	}
	for {
		p.eat(css_lexer.TWhitespace)
		if p.peek(css_lexer.TEndOfFile) || p.peek(css_lexer.TComma) || p.peek(stop) {
			break
		}

		// Optional combinator
		combinator := p.parseCombinator()
		if combinator.Byte != 0 {
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
	t := p.current()
	return css_ast.NameToken{
		Kind: t.Kind,
		Loc:  t.Range.Loc,
		Text: p.decoded(),
	}
}

func (p *parser) parseCompoundSelector(opts parseComplexSelectorOpts) (sel css_ast.CompoundSelector, ok bool) {
	startLoc := p.current().Range.Loc

	// This is an extension: https://drafts.csswg.org/css-nesting-1/
	hasLeadingNestingSelector := p.peek(css_lexer.TDelimAmpersand)
	if hasLeadingNestingSelector {
		p.shouldLowerNesting = true
		p.reportUseOfNesting(p.current().Range, opts.isDeclarationContext)
		sel.NestingSelectorLoc = ast.MakeIndex32(uint32(startLoc.Start))
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
		subclassToken := p.current()

		switch subclassToken.Kind {
		case css_lexer.THash:
			if (subclassToken.Flags & css_lexer.IsID) == 0 {
				break subclassSelectors
			}
			nameLoc := logger.Loc{Start: subclassToken.Range.Loc.Start + 1}
			name := p.decoded()
			sel.SubclassSelectors = append(sel.SubclassSelectors, css_ast.SubclassSelector{
				Loc: subclassToken.Range.Loc,
				Data: &css_ast.SSHash{
					Name: ast.LocRef{Loc: nameLoc, Ref: p.symbolForName(name)},
				},
			})
			p.advance()

		case css_lexer.TDelimDot:
			p.advance()
			nameLoc := p.current().Range.Loc
			name := p.decoded()
			sel.SubclassSelectors = append(sel.SubclassSelectors, css_ast.SubclassSelector{
				Loc: subclassToken.Range.Loc,
				Data: &css_ast.SSClass{
					Name: ast.LocRef{Loc: nameLoc, Ref: p.symbolForName(name)},
				},
			})
			if !p.expect(css_lexer.TIdent) {
				return
			}

		case css_lexer.TOpenBracket:
			attr, good := p.parseAttributeSelector()
			if !good {
				return
			}
			sel.SubclassSelectors = append(sel.SubclassSelectors, css_ast.SubclassSelector{
				Loc:  subclassToken.Range.Loc,
				Data: &attr,
			})

		case css_lexer.TColon:
			if p.next().Kind == css_lexer.TColon {
				// Special-case the start of the pseudo-element selector section
				for p.current().Kind == css_lexer.TColon {
					firstColonLoc := p.current().Range.Loc
					isElement := p.next().Kind == css_lexer.TColon
					if isElement {
						p.advance()
					}
					pseudo := p.parsePseudoClassSelector(isElement)

					// https://www.w3.org/TR/selectors-4/#single-colon-pseudos
					// The four Level 2 pseudo-elements (::before, ::after, ::first-line,
					// and ::first-letter) may, for legacy reasons, be represented using
					// the <pseudo-class-selector> grammar, with only a single ":"
					// character at their start.
					if p.options.minifySyntax && isElement {
						if pseudo, ok := pseudo.(*css_ast.SSPseudoClass); ok && len(pseudo.Args) == 0 {
							switch pseudo.Name {
							case "before", "after", "first-line", "first-letter":
								pseudo.IsElement = false
							}
						}
					}

					sel.SubclassSelectors = append(sel.SubclassSelectors, css_ast.SubclassSelector{
						Loc:  firstColonLoc,
						Data: pseudo,
					})
				}
				break subclassSelectors
			}

			sel.SubclassSelectors = append(sel.SubclassSelectors, css_ast.SubclassSelector{
				Loc:  subclassToken.Range.Loc,
				Data: p.parsePseudoClassSelector(false),
			})

		case css_lexer.TDelimAmpersand:
			// This is an extension: https://drafts.csswg.org/css-nesting-1/
			p.shouldLowerNesting = true
			p.reportUseOfNesting(subclassToken.Range, sel.HasNestingSelector())
			sel.NestingSelectorLoc = ast.MakeIndex32(uint32(subclassToken.Range.Loc.Start))
			p.advance()

		default:
			break subclassSelectors
		}
	}

	// The compound selector must be non-empty
	if sel.IsInvalidBecauseEmpty() {
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
			if !p.expect(css_lexer.TDelimEquals) {
				return
			}
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

func (p *parser) parsePseudoClassSelector(isElement bool) css_ast.SS {
	p.advance()

	if p.peek(css_lexer.TFunction) {
		text := p.decoded()
		matchingLoc := logger.Loc{Start: p.current().Range.End() - 1}
		p.advance()

		// Potentially parse a pseudo-class with a selector list
		if !isElement {
			var kind css_ast.PseudoClassKind
			local := p.makeLocalSymbols
			ok := true
			switch text {
			case "global":
				kind = css_ast.PseudoClassGlobal
				if p.options.symbolMode != symbolModeDisabled {
					local = false
				}
			case "has":
				kind = css_ast.PseudoClassHas
			case "is":
				kind = css_ast.PseudoClassIs
			case "local":
				kind = css_ast.PseudoClassLocal
				if p.options.symbolMode != symbolModeDisabled {
					local = true
				}
			case "not":
				kind = css_ast.PseudoClassNot
			case "where":
				kind = css_ast.PseudoClassWhere
			default:
				ok = false
			}
			if ok {
				old := p.index

				// ":local" forces local names and ":global" forces global names
				oldLocal := p.makeLocalSymbols
				p.makeLocalSymbols = local
				selectors, ok := p.parseSelectorList(parseSelectorOpts{
					pseudoClassKind:        kind,
					stopOnCloseParen:       true,
					onlyOneComplexSelector: kind == css_ast.PseudoClassGlobal || kind == css_ast.PseudoClassLocal,
				})
				p.makeLocalSymbols = oldLocal

				if ok && p.expectWithMatchingLoc(css_lexer.TCloseParen, matchingLoc) {
					return &css_ast.SSPseudoClassWithSelectorList{Kind: kind, Selectors: selectors}
				}

				p.index = old
			}
		}
		args := p.convertTokens(p.parseAnyValue())
		p.expectWithMatchingLoc(css_lexer.TCloseParen, matchingLoc)
		return &css_ast.SSPseudoClass{IsElement: isElement, Name: text, Args: args}
	}

	name := p.decoded()
	sel := css_ast.SSPseudoClass{IsElement: isElement}
	if p.expect(css_lexer.TIdent) {
		sel.Name = name

		// ":local .local_name :global .global_name {}"
		// ":local { .local_name { :global { .global_name {} } }"
		if p.options.symbolMode != symbolModeDisabled {
			switch name {
			case "local":
				p.makeLocalSymbols = true
			case "global":
				p.makeLocalSymbols = false
			}
		}
	}
	return &sel
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

func (p *parser) parseCombinator() css_ast.Combinator {
	t := p.current()

	switch t.Kind {
	case css_lexer.TDelimGreaterThan:
		p.advance()
		return css_ast.Combinator{Loc: t.Range.Loc, Byte: '>'}

	case css_lexer.TDelimPlus:
		p.advance()
		return css_ast.Combinator{Loc: t.Range.Loc, Byte: '+'}

	case css_lexer.TDelimTilde:
		p.advance()
		return css_ast.Combinator{Loc: t.Range.Loc, Byte: '~'}

	default:
		return css_ast.Combinator{}
	}
}
