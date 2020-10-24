package css_printer

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

const quoteForURL rune = -1

type printer struct {
	Options
	importRecords []ast.ImportRecord
	sb            strings.Builder
}

type Options struct {
	RemoveWhitespace bool
	ASCIIOnly        bool
}

func Print(tree css_ast.AST, options Options) string {
	p := printer{
		Options:       options,
		importRecords: tree.ImportRecords,
	}
	for _, rule := range tree.Rules {
		p.printRule(rule, 0, false)
	}
	return p.sb.String()
}

func (p *printer) printRule(rule css_ast.R, indent int, omitTrailingSemicolon bool) {
	if !p.RemoveWhitespace {
		p.printIndent(indent)
	}

	switch r := rule.(type) {
	case *css_ast.RAtCharset:
		// It's not valid to remove the space in between these two tokens
		p.print("@charset ")

		// It's not valid to print the string with single quotes
		p.printQuotedWithQuote(r.Encoding, '"')
		p.print(";")

	case *css_ast.RAtNamespace:
		if r.Prefix != "" {
			p.print("@namespace ")
			p.printIdent(r.Prefix, identNormal, canDiscardWhitespaceAfter)
		} else {
			p.print("@namespace")
		}
		if !p.RemoveWhitespace {
			p.print(" ")
		}
		p.printQuoted(r.Path)
		p.print(";")

	case *css_ast.RAtImport:
		if p.RemoveWhitespace {
			p.print("@import")
		} else {
			p.print("@import ")
		}
		p.printQuoted(p.importRecords[r.ImportRecordIndex].Path.Text)
		p.print(";")

	case *css_ast.RAtKeyframes:
		p.print("@")
		p.printIdent(r.AtToken, identNormal, mayNeedWhitespaceAfter)
		p.print(" ")
		if r.Name == "" {
			p.print("\"\"")
		} else {
			p.printIdent(r.Name, identNormal, canDiscardWhitespaceAfter)
		}
		if !p.RemoveWhitespace {
			p.print(" ")
		}
		if p.RemoveWhitespace {
			p.print("{")
		} else {
			p.print("{\n")
		}
		indent++
		for _, block := range r.Blocks {
			if !p.RemoveWhitespace {
				p.printIndent(indent)
			}
			for i, sel := range block.Selectors {
				if i > 0 {
					if p.RemoveWhitespace {
						p.print(",")
					} else {
						p.print(", ")
					}
				}
				p.print(sel)
			}
			if !p.RemoveWhitespace {
				p.print(" ")
			}
			p.printRuleBlock(block.Rules, indent)
			if !p.RemoveWhitespace {
				p.print("\n")
			}
		}
		indent--
		if !p.RemoveWhitespace {
			p.printIndent(indent)
		}
		p.print("}")

	case *css_ast.RKnownAt:
		p.print("@")
		whitespace := mayNeedWhitespaceAfter
		if len(r.Prelude) == 0 {
			whitespace = canDiscardWhitespaceAfter
		}
		p.printIdent(r.AtToken, identNormal, whitespace)
		if !p.RemoveWhitespace || len(r.Prelude) > 0 {
			p.print(" ")
		}
		p.printTokens(r.Prelude)
		if !p.RemoveWhitespace && len(r.Prelude) > 0 {
			p.print(" ")
		}
		p.printRuleBlock(r.Rules, indent)

	case *css_ast.RUnknownAt:
		p.print("@")
		whitespace := mayNeedWhitespaceAfter
		if len(r.Prelude) == 0 {
			whitespace = canDiscardWhitespaceAfter
		}
		p.printIdent(r.AtToken, identNormal, whitespace)
		if (!p.RemoveWhitespace && r.Block != nil) || len(r.Prelude) > 0 {
			p.print(" ")
		}
		p.printTokens(r.Prelude)
		if !p.RemoveWhitespace && r.Block != nil && len(r.Prelude) > 0 {
			p.print(" ")
		}
		if r.Block == nil {
			p.print(";")
		} else {
			p.printTokens(r.Block)
		}

	case *css_ast.RSelector:
		p.printComplexSelectors(r.Selectors, indent)
		if !p.RemoveWhitespace {
			p.print(" ")
		}
		p.printRuleBlock(r.Rules, indent)

	case *css_ast.RQualified:
		p.printTokens(r.Prelude)
		if !p.RemoveWhitespace {
			p.print(" ")
		}
		p.printRuleBlock(r.Rules, indent)

	case *css_ast.RDeclaration:
		p.printIdent(r.KeyText, identNormal, canDiscardWhitespaceAfter)
		if p.RemoveWhitespace {
			p.print(":")
		} else {
			p.print(": ")
		}
		p.printTokens(r.Value)
		if r.Important {
			if !p.RemoveWhitespace {
				p.print(" ")
			}
			p.print("!important")
		}
		if !omitTrailingSemicolon {
			p.print(";")
		}

	case *css_ast.RBadDeclaration:
		p.printTokens(r.Tokens)
		if !omitTrailingSemicolon {
			p.print(";")
		}

	default:
		panic("Internal error")
	}

	if !p.RemoveWhitespace {
		p.print("\n")
	}
}

func (p *printer) printRuleBlock(rules []css_ast.R, indent int) {
	if p.RemoveWhitespace {
		p.print("{")
	} else {
		p.print("{\n")
	}

	for i, decl := range rules {
		omitTrailingSemicolon := p.RemoveWhitespace && i+1 == len(rules)
		p.printRule(decl, indent+1, omitTrailingSemicolon)
	}

	if !p.RemoveWhitespace {
		p.printIndent(indent)
	}
	p.print("}")
}

func (p *printer) printComplexSelectors(selectors []css_ast.ComplexSelector, indent int) {
	for i, complex := range selectors {
		if i > 0 {
			if p.RemoveWhitespace {
				p.print(",")
			} else {
				p.print(",\n")
				p.printIndent(indent)
			}
		}

		for j, compound := range complex.Selectors {
			p.printCompoundSelector(compound, j == 0, j+1 == len(complex.Selectors))
		}
	}
}

func (p *printer) printCompoundSelector(sel css_ast.CompoundSelector, isFirst bool, isLast bool) {
	if sel.HasNestPrefix {
		p.print("&")
	}

	if sel.Combinator != "" {
		if !p.RemoveWhitespace {
			p.print(" ")
		}
		p.print(sel.Combinator)
		if !p.RemoveWhitespace {
			p.print(" ")
		}
	} else if !isFirst {
		p.print(" ")
	}

	if sel.TypeSelector != nil {
		whitespace := mayNeedWhitespaceAfter
		if len(sel.SubclassSelectors) > 0 || len(sel.PseudoClassSelectors) > 0 {
			// There is no chance of whitespace before a subclass selector or pseudo
			// class selector
			whitespace = canDiscardWhitespaceAfter
		}
		p.printNamespacedName(*sel.TypeSelector, whitespace)
	}

	for i, sub := range sel.SubclassSelectors {
		whitespace := mayNeedWhitespaceAfter

		// There is no chance of whitespace between subclass selectors
		if i+1 < len(sel.SubclassSelectors) || len(sel.PseudoClassSelectors) > 0 {
			whitespace = canDiscardWhitespaceAfter
		}

		switch s := sub.(type) {
		case *css_ast.SSHash:
			p.print("#")

			// This deliberately does not use identHash. From the specification:
			// "In <id-selector>, the <hash-token>'s value must be an identifier."
			p.printIdent(s.Name, identNormal, whitespace)

		case *css_ast.SSClass:
			p.print(".")
			p.printIdent(s.Name, identNormal, whitespace)

		case *css_ast.SSAttribute:
			p.print("[")
			p.printNamespacedName(s.NamespacedName, canDiscardWhitespaceAfter)
			if s.MatcherOp != "" {
				p.print(s.MatcherOp)
				printAsIdent := false

				// Print the value as an identifier if it's possible
				if css_lexer.WouldStartIdentifierWithoutEscapes(s.MatcherValue) {
					printAsIdent = true
					for _, c := range s.MatcherValue {
						if !css_lexer.IsNameContinue(c) {
							printAsIdent = false
							break
						}
					}
				}

				if printAsIdent {
					p.printIdent(s.MatcherValue, identNormal, canDiscardWhitespaceAfter)
				} else {
					p.printQuoted(s.MatcherValue)
				}
			}
			if s.MatcherModifier != 0 {
				p.print(" ")
				p.print(string(rune(s.MatcherModifier)))
			}
			p.print("]")

		case *css_ast.SSPseudoClass:
			p.printPseudoClassSelector(*s, whitespace)
		}
	}

	if len(sel.PseudoClassSelectors) > 0 {
		p.print(":")
		for i, pseudo := range sel.PseudoClassSelectors {
			whitespace := mayNeedWhitespaceAfter
			if i+1 < len(sel.PseudoClassSelectors) || isLast {
				whitespace = canDiscardWhitespaceAfter
			}
			p.printPseudoClassSelector(pseudo, whitespace)
		}
	}
}

func (p *printer) printNamespacedName(nsName css_ast.NamespacedName, whitespace trailingWhitespace) {
	if nsName.NamespacePrefix != nil {
		switch nsName.NamespacePrefix.Kind {
		case css_lexer.TIdent:
			p.printIdent(nsName.NamespacePrefix.Text, identNormal, canDiscardWhitespaceAfter)
		case css_lexer.TDelimAsterisk:
			p.print("*")
		default:
			panic("Internal error")
		}

		p.print("|")
	}

	switch nsName.Name.Kind {
	case css_lexer.TIdent:
		p.printIdent(nsName.Name.Text, identNormal, whitespace)
	case css_lexer.TDelimAsterisk:
		p.print("*")
	case css_lexer.TDelimAmpersand:
		p.print("&")
	default:
		panic("Internal error")
	}
}

func (p *printer) printPseudoClassSelector(pseudo css_ast.SSPseudoClass, whitespace trailingWhitespace) {
	p.print(":")

	if len(pseudo.Args) > 0 {
		p.printIdent(pseudo.Name, identNormal, canDiscardWhitespaceAfter)
		p.print("(")
		p.printTokens(pseudo.Args)
		p.print(")")
	} else {
		p.printIdent(pseudo.Name, identNormal, whitespace)
	}
}

func (p *printer) print(text string) {
	p.sb.WriteString(text)
}

func bestQuoteCharForString(text string, forURL bool) rune {
	forURLCost := 0
	singleCost := 2
	doubleCost := 2

	for _, c := range text {
		switch c {
		case '\'':
			forURLCost++
			singleCost++

		case '"':
			forURLCost++
			doubleCost++

		case '(', ')', ' ', '\t':
			forURLCost++

		case '\\', '\n', '\r', '\f':
			forURLCost++
			singleCost++
			doubleCost++
		}
	}

	// Quotes can sometimes be omitted for URL tokens
	if forURL && forURLCost < singleCost && forURLCost < doubleCost {
		return quoteForURL
	}

	// Prefer double quotes to single quotes if there is no cost difference
	if singleCost < doubleCost {
		return '\''
	}

	return '"'
}

func (p *printer) printQuoted(text string) {
	p.printQuotedWithQuote(text, bestQuoteCharForString(text, false))
}

type escapeKind uint8

const (
	escapeNone escapeKind = iota
	escapeBackslash
	escapeHex
)

func (p *printer) printWithEscape(c rune, escape escapeKind, remainingText string, mayNeedWhitespaceAfter bool) {
	if escape == escapeBackslash && ((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
		// Hexadecimal characters cannot use a plain backslash escape
		escape = escapeHex
	}

	switch escape {
	case escapeNone:
		p.sb.WriteRune(c)

	case escapeBackslash:
		p.sb.WriteRune('\\')
		p.sb.WriteRune(c)

	case escapeHex:
		p.sb.WriteString(fmt.Sprintf("\\%x", c))

		// Make sure the next character is not interpreted as part of the escape sequence
		if next := utf8.RuneLen(c); next < len(remainingText) {
			c = rune(remainingText[next])
			if c == ' ' || c == '\t' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
				p.sb.WriteRune(' ')
			}
		} else if mayNeedWhitespaceAfter {
			// If the last character is a hexadecimal escape, print a space afterwards
			// for the escape sequence to consume. That way we're sure it won't
			// accidentally consume a semantically significant space afterward.
			p.sb.WriteRune(' ')
		}
	}
}

func (p *printer) printQuotedWithQuote(text string, quote rune) {
	if quote != quoteForURL {
		p.sb.WriteRune(quote)
	}

	for i, c := range text {
		escape := escapeNone

		switch c {
		case '\r', '\n', '\f':
			// Use a hexadecimal escape for characters that would be invalid escapes
			escape = escapeHex

		case '\\', quote:
			escape = escapeBackslash

		case '(', ')', ' ', '\t', '"', '\'':
			// These characters must be escaped in URL tokens
			if quote == quoteForURL {
				escape = escapeBackslash
			}

		default:
			if p.ASCIIOnly && c >= 0x80 {
				escape = escapeHex
			}
		}

		p.printWithEscape(c, escape, text[i:], false)
	}

	if quote != quoteForURL {
		p.sb.WriteRune(quote)
	}
}

type identMode uint8

const (
	identNormal identMode = iota
	identHash
	identDimensionUnit
)

type trailingWhitespace uint8

const (
	mayNeedWhitespaceAfter trailingWhitespace = iota
	canDiscardWhitespaceAfter
)

func (p *printer) printIdent(text string, mode identMode, whitespace trailingWhitespace) {
	for i, c := range text {
		escape := escapeNone

		if p.ASCIIOnly && c >= 0x80 {
			escape = escapeHex
		} else if c == '\r' || c == '\n' || c == '\f' {
			// Use a hexadecimal escape for characters that would be invalid escapes
			escape = escapeHex
		} else {
			// Escape non-identifier characters
			if !css_lexer.IsNameContinue(c) {
				escape = escapeBackslash
			}

			// Special escape behavior for the first character
			if i == 0 {
				switch mode {
				case identNormal:
					if !css_lexer.WouldStartIdentifierWithoutEscapes(text) {
						escape = escapeBackslash
					}

				case identDimensionUnit:
					if !css_lexer.WouldStartIdentifierWithoutEscapes(text) {
						escape = escapeBackslash
					} else if c >= '0' && c <= '9' {
						// Unit: "2x"
						escape = escapeHex
					} else if c == 'e' || c == 'E' {
						if len(text) >= 2 && text[1] >= '0' && text[1] <= '9' {
							// Unit: "e2x"
							escape = escapeBackslash
						} else if len(text) >= 3 && text[1] == '-' && text[2] >= '0' && text[2] <= '9' {
							// Unit: "e-2x"
							escape = escapeBackslash
						}
					}
				}
			}
		}

		// If the last character is a hexadecimal escape, print a space afterwards
		// for the escape sequence to consume. That way we're sure it won't
		// accidentally consume a semantically significant space afterward.
		mayNeedWhitespaceAfter := whitespace == mayNeedWhitespaceAfter && escape != escapeNone && i+utf8.RuneLen(c) == len(text)
		p.printWithEscape(c, escape, text[i:], mayNeedWhitespaceAfter)
	}
}

func (p *printer) printIndent(indent int) {
	for i := 0; i < indent; i++ {
		p.sb.WriteString("  ")
	}
}

func (p *printer) printTokens(tokens []css_ast.Token) {
	for i, t := range tokens {
		hasWhitespaceAfter := t.HasWhitespaceAfter && i+1 != len(tokens)
		whitespace := mayNeedWhitespaceAfter
		if !hasWhitespaceAfter {
			whitespace = canDiscardWhitespaceAfter
		}

		switch t.Kind {
		case css_lexer.TIdent:
			p.printIdent(t.Text, identNormal, whitespace)

		case css_lexer.TFunction:
			p.printIdent(t.Text, identNormal, whitespace)
			p.print("(")

		case css_lexer.TDimension:
			p.print(t.DimensionValue())
			p.printIdent(t.DimensionUnit(), identDimensionUnit, whitespace)

		case css_lexer.TAtKeyword:
			p.print("@")
			p.printIdent(t.Text, identNormal, whitespace)

		case css_lexer.THash, css_lexer.THashID:
			p.print("#")
			p.printIdent(t.Text, identHash, whitespace)

		case css_lexer.TString:
			p.printQuoted(t.Text)

		case css_lexer.TURL:
			text := p.importRecords[t.ImportRecordIndex].Path.Text
			p.print("url(")
			p.printQuotedWithQuote(text, bestQuoteCharForString(text, true))
			p.print(")")

		default:
			p.print(t.Text)
		}

		if t.Children != nil {
			children := *t.Children

			if t.Kind == css_lexer.TOpenBrace && !p.RemoveWhitespace && len(children) > 0 {
				p.print(" ")
			}

			p.printTokens(children)

			switch t.Kind {
			case css_lexer.TFunction:
				p.print(")")

			case css_lexer.TOpenParen:
				p.print(")")

			case css_lexer.TOpenBrace:
				if !p.RemoveWhitespace && len(children) > 0 {
					p.print(" ")
				}
				p.print("}")

			case css_lexer.TOpenBracket:
				p.print("]")
			}
		}

		if hasWhitespaceAfter {
			if t.Kind == css_lexer.TComma && p.RemoveWhitespace {
				// Assume that whitespace can always be removed after a comma
			} else {
				p.print(" ")
			}
		}
	}
}
