package css_parser

import (
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/logger"
)

func (p *parser) parseSelectorList(opts parseSelectorOpts) (list []css_ast.ComplexSelector, ok bool) {
	// Parse the first selector
	sel, good := p.parseComplexSelector(opts)
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
		sel, good := p.parseComplexSelector(opts)
		if !good {
			return
		}
		list = append(list, sel)
	}

	ok = true
	return
}

type parseSelectorOpts struct {
	isTopLevel bool
}

func (p *parser) parseComplexSelector(opts parseSelectorOpts) (result css_ast.ComplexSelector, ok bool) {
	// This is an extension: https://drafts.csswg.org/css-nesting-1/
	r := p.current().Range
	combinator := p.parseCombinator()
	if combinator != "" {
		if opts.isTopLevel {
			p.maybeWarnAboutNesting(r)
		}
		p.eat(css_lexer.TWhitespace)
	}

	// Parent
	sel, good := p.parseCompoundSelector(opts)
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

func (p *parser) parseCompoundSelector(opts parseSelectorOpts) (sel css_ast.CompoundSelector, ok bool) {
	// This is an extension: https://drafts.csswg.org/css-nesting-1/
	if p.peek(css_lexer.TDelimAmpersand) {
		if opts.isTopLevel {
			p.maybeWarnAboutNesting(p.current().Range)
		}
		sel.HasNestingSelector = true
		p.advance()
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
			if !sel.HasNestingSelector {
				p.maybeWarnAboutNesting(p.current().Range)
				sel.HasNestingSelector = true
			}
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
