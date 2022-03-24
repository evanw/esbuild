package css_parser

import (
	"fmt"

	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/logger"
)

func (p *parser) parseSelectorList(opts parseSelectorOpts) (list []css_ast.ComplexSelector, ok bool) {
	// Parse the first selector
	firstRange := p.current().Range
	sel, good, firstHasNestPrefix := p.parseComplexSelector(opts)
	if !good {
		return
	}
	list = append(list, sel)

	// Parse the remaining selectors
	for {
		p.eat(css_lexer.TWhitespace)
		if !p.eat(css_lexer.TComma) {
			break
		}
		p.eat(css_lexer.TWhitespace)
		loc := p.current().Range.Loc
		sel, good, hasNestPrefix := p.parseComplexSelector(opts)
		if !good {
			return
		}
		list = append(list, sel)

		// Validate nest prefix consistency
		if firstHasNestPrefix && !hasNestPrefix && opts.atNestRange.Len == 0 {
			data := p.tracker.MsgData(logger.Range{Loc: loc}, "Every selector in a nested style rule must start with \"&\"")
			data.Location.Suggestion = "&"
			p.log.AddMsg(logger.Msg{
				Kind:  logger.Warning,
				Data:  data,
				Notes: []logger.MsgData{p.tracker.MsgData(firstRange, "This is a nested style rule because of the \"&\" here:")},
			})
		}
	}

	ok = true
	return
}

type parseSelectorOpts struct {
	atNestRange  logger.Range
	allowNesting bool
	isTopLevel   bool
}

func (p *parser) parseComplexSelector(opts parseSelectorOpts) (result css_ast.ComplexSelector, ok bool, hasNestPrefix bool) {
	// Parent
	loc := p.current().Range.Loc
	sel, good := p.parseCompoundSelector(opts)
	if !good {
		return
	}
	hasNestPrefix = sel.NestingSelector == css_ast.NestingSelectorPrefix
	isNestContaining := sel.NestingSelector != css_ast.NestingSelectorNone
	result.Selectors = append(result.Selectors, sel)

	for {
		p.eat(css_lexer.TWhitespace)
		if p.peek(css_lexer.TEndOfFile) || p.peek(css_lexer.TComma) || p.peek(css_lexer.TOpenBrace) {
			break
		}

		// Optional combinator
		combinator := p.parseCombinator()
		if combinator != "" {
			p.eat(css_lexer.TWhitespace)
		}

		// Child
		sel, good := p.parseCompoundSelector(opts)
		if !good {
			return
		}
		sel.Combinator = combinator
		result.Selectors = append(result.Selectors, sel)
		if sel.NestingSelector != css_ast.NestingSelectorNone {
			isNestContaining = true
		}
	}

	// Validate nest selector consistency
	if opts.atNestRange.Len != 0 && !isNestContaining {
		p.log.AddWithNotes(logger.Warning, &p.tracker, logger.Range{Loc: loc}, "Every selector in a nested style rule must contain \"&\"",
			[]logger.MsgData{p.tracker.MsgData(opts.atNestRange, "This is a nested style rule because of the \"@nest\" here:")})
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

func (p *parser) maybeWarnAboutNesting(r logger.Range, opts parseSelectorOpts) {
	if !opts.allowNesting {
		p.log.Add(logger.Warning, &p.tracker, r, "CSS nesting syntax cannot be used outside of a style rule")
	} else if p.options.UnsupportedCSSFeatures.Has(compat.Nesting) {
		text := "CSS nesting syntax is not supported in the configured target environment"
		if p.options.OriginalTargetEnv != "" {
			text = fmt.Sprintf("%s (%s)", text, p.options.OriginalTargetEnv)
		}
		p.log.Add(logger.Warning, &p.tracker, r, text)
	}
}

func (p *parser) parseCompoundSelector(opts parseSelectorOpts) (sel css_ast.CompoundSelector, ok bool) {
	// This is an extension: https://drafts.csswg.org/css-nesting-1/
	r := p.current().Range
	if p.eat(css_lexer.TDelimAmpersand) {
		sel.NestingSelector = css_ast.NestingSelectorPrefix
		p.maybeWarnAboutNesting(r, opts)
	}

	// Parse the type selector
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
			p.advance()
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
			r := p.current().Range
			p.advance()
			if sel.NestingSelector == css_ast.NestingSelectorNone {
				sel.NestingSelector = css_ast.NestingSelectorPresentButNotPrefix
				p.maybeWarnAboutNesting(r, opts)
			}

		default:
			break subclassSelectors
		}
	}

	// The compound selector must be non-empty
	if sel.NestingSelector == css_ast.NestingSelectorNone && sel.TypeSelector == nil && len(sel.SubclassSelectors) == 0 {
		p.unexpected()
		return
	}

	ok = true
	return
}

func (p *parser) parseAttributeSelector() (attr css_ast.SSAttribute, ok bool) {
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

	p.expect(css_lexer.TCloseBracket)
	ok = true
	return
}

func (p *parser) parsePseudoClassSelector() css_ast.SSPseudoClass {
	p.advance()

	if p.peek(css_lexer.TFunction) {
		text := p.decoded()
		p.advance()
		args := p.convertTokens(p.parseAnyValue())
		p.expect(css_lexer.TCloseParen)
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
		}

		p.advance()
	}

	tokens := p.tokens[start:p.index]
	if len(tokens) == 0 {
		p.unexpected()
	}
	return tokens
}

func (p *parser) parseCombinator() string {
	switch p.current().Kind {
	case css_lexer.TDelimGreaterThan:
		p.advance()
		return ">"

	case css_lexer.TDelimPlus:
		p.advance()
		return "+"

	case css_lexer.TDelimTilde:
		p.advance()
		return "~"

	default:
		return ""
	}
}
