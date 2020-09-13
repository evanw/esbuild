package css_parser

import (
	"strings"

	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

func (p *parser) parseSelectorList() (list []css_ast.ComplexSelector, ok bool) {
	// Parse the first selector
	p.eat(css_lexer.TWhitespace)
	sel, good := p.parseComplexSelector()
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
		sel, good := p.parseComplexSelector()
		if !good {
			return
		}
		list = append(list, sel)
	}

	ok = true
	return
}

func (p *parser) parseComplexSelector() (result css_ast.ComplexSelector, ok bool) {
	// Parent
	sel, good := p.parseCompoundSelector()
	if !good {
		return
	}
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
		sel, good := p.parseCompoundSelector()
		if !good {
			return
		}
		sel.Combinator = combinator
		result.Selectors = append(result.Selectors, sel)
	}

	ok = true
	return
}

func (p *parser) parseCompoundSelector() (sel css_ast.CompoundSelector, ok bool) {
	// Parse the type selector
	switch p.current().Kind {
	case css_lexer.TDelimAmpersand:
		// This is an extension: https://drafts.csswg.org/css-nesting-1/
		sel.TypeSelector = &css_ast.NamespacedName{Name: "&"}
		p.advance()

	case css_lexer.TDelimBar, css_lexer.TIdent, css_lexer.TDelimAsterisk:
		nsName := css_ast.NamespacedName{}
		if !p.peek(css_lexer.TDelimBar) {
			nsName.Name = p.text()
			p.advance()
		}
		if p.eat(css_lexer.TDelimBar) {
			if !p.peek(css_lexer.TIdent) && !p.peek(css_lexer.TDelimAsterisk) {
				p.expect(css_lexer.TIdent)
				return
			}
			prefix := nsName.Name
			nsName.NamespacePrefix = &prefix
			nsName.Name = p.text()
			p.advance()
		}
		sel.TypeSelector = &nsName
	}

	// Parse the subclass selectors
subclassSelectors:
	for {
		switch p.current().Kind {
		case css_lexer.THashID:
			name := p.text()[1:]
			sel.SubclassSelectors = append(sel.SubclassSelectors, &css_ast.SSHash{Name: name})
			p.advance()

		case css_lexer.TDelimDot:
			p.advance()
			name := p.text()
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
				// Stop if this is the start of the pseudo-element selector section
				break subclassSelectors
			}
			pseudo := p.parsePseudoElementSelector()
			sel.SubclassSelectors = append(sel.SubclassSelectors, &pseudo)

		default:
			break subclassSelectors
		}
	}

	// Parse the pseudo-element selectors
	if p.eat(css_lexer.TColon) {
		pseudo := p.parsePseudoElementSelector()
		sel.PseudoClassSelectors = append(sel.PseudoClassSelectors, pseudo)
		for p.peek(css_lexer.TColon) {
			pseudo := p.parsePseudoElementSelector()
			sel.PseudoClassSelectors = append(sel.PseudoClassSelectors, pseudo)
		}
	}

	// The compound selector must be non-empty
	if sel.TypeSelector == nil && len(sel.SubclassSelectors) == 0 && len(sel.PseudoClassSelectors) == 0 {
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
		prefix := ""
		if p.peek(css_lexer.TDelimAsterisk) {
			prefix = "*"
			p.advance()
		}
		attr.NamespacedName.NamespacePrefix = &prefix
		if !p.expect(css_lexer.TDelimBar) {
			return
		}
		if !p.peek(css_lexer.TIdent) {
			p.expect(css_lexer.TIdent)
			return
		}
		attr.NamespacedName.Name = p.text()
		p.advance()

	case css_lexer.TIdent:
		// "[x]"
		// "[x|y]"
		attr.NamespacedName.Name = p.text()
		p.advance()
		if p.eat(css_lexer.TDelimBar) {
			if !p.peek(css_lexer.TIdent) {
				p.expect(css_lexer.TIdent)
				return
			}
			prefix := attr.NamespacedName.Name
			attr.NamespacedName.NamespacePrefix = &prefix
			attr.NamespacedName.Name = p.text()
			p.advance()
		}

	default:
		p.expect(css_lexer.TIdent)
		return
	}

	// Parse the optional matcher operator
	if p.eat(css_lexer.TDelimEquals) {
		attr.MatcherOp = "="
	} else if p.next().Kind == css_lexer.TDelimEquals {
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
			p.advance()
		}
	}

	// Parse the optional matcher value
	if attr.MatcherOp != "" {
		if !p.peek(css_lexer.TString) && !p.peek(css_lexer.TIdent) {
			p.unexpected()
		}
		attr.MatcherValue = p.text()
		p.advance()
		p.eat(css_lexer.TWhitespace)
		if p.peek(css_lexer.TIdent) {
			if modifier := p.text(); len(modifier) == 1 {
				if c := modifier[0]; strings.ContainsRune("iIsS", rune(c)) {
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

func (p *parser) parsePseudoElementSelector() css_ast.SSPseudoClass {
	p.advance()

	if p.peek(css_lexer.TFunction) {
		text := p.text()
		p.advance()
		args := p.parseAnyValue()
		p.expect(css_lexer.TCloseParen)
		return css_ast.SSPseudoClass{Name: text[:len(text)-1], Args: args}
	}

	sel := css_ast.SSPseudoClass{Name: p.text()}
	p.expect(css_lexer.TIdent)
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

	case css_lexer.TDelimBar:
		if p.next().Kind == css_lexer.TDelimBar {
			p.advance()
			p.advance()
		}
		return "||"

	default:
		return ""
	}
}
