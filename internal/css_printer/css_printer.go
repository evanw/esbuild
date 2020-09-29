package css_printer

import (
	"fmt"
	"strings"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

const noneQuote rune = -1

type printer struct {
	Options
	importRecords []ast.ImportRecord
	sb            strings.Builder
}

type Options struct {
	RemoveWhitespace bool
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
			p.print(r.Prefix)
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
		p.print(r.AtToken)
		p.print(" ")
		p.print(r.Name)
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
		p.print(r.AtToken)
		if !p.RemoveWhitespace || len(r.Prelude) > 0 {
			p.print(" ")
		}
		p.printTokens(r.Prelude)
		if !p.RemoveWhitespace {
			p.print(" ")
		}
		p.printRuleBlock(r.Rules, indent)

	case *css_ast.RUnknownAt:
		p.print(r.AtToken)
		if len(r.Prelude) > 0 || (r.Block != nil && !p.RemoveWhitespace) {
			p.print(" ")
		}
		p.printTokens(r.Prelude)
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
		p.printRuleBlock(r.Rules, indent)

	case *css_ast.RDeclaration:
		p.print(r.Key)
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
			p.printCompoundSelector(compound, j == 0)
		}
	}
}

func (p *printer) printCompoundSelector(sel css_ast.CompoundSelector, isFirst bool) {
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
		p.printNamespacedName(*sel.TypeSelector)
	}

	for _, sub := range sel.SubclassSelectors {
		switch s := sub.(type) {
		case *css_ast.SSHash:
			p.print("#")
			p.print(s.Name)

		case *css_ast.SSClass:
			p.print(".")
			p.print(s.Name)

		case *css_ast.SSAttribute:
			p.print("[")
			if s.NamespacedName.NamespacePrefix != nil && *s.NamespacedName.NamespacePrefix == "" {
				// "[|attr]" is equivalent to "[attr]"
				p.print(s.NamespacedName.Name)
			} else {
				p.printNamespacedName(s.NamespacedName)
			}
			p.print(s.MatcherOp)
			p.print(s.MatcherValue)
			if s.MatcherModifier != 0 {
				p.print(" ")
				p.print(string(rune(s.MatcherModifier)))
			}
			p.print("]")

		case *css_ast.SSPseudoClass:
			p.printPseudoClassSelector(*s)
		}
	}

	if len(sel.PseudoClassSelectors) > 0 {
		p.print(":")
		for _, pseudo := range sel.PseudoClassSelectors {
			p.printPseudoClassSelector(pseudo)
		}
	}
}

func (p *printer) printNamespacedName(nsName css_ast.NamespacedName) {
	if nsName.NamespacePrefix != nil {
		p.print(*nsName.NamespacePrefix)
		p.print("|")
	}
	p.print(nsName.Name)
}

func (p *printer) printPseudoClassSelector(pseudo css_ast.SSPseudoClass) {
	p.print(":")
	p.print(pseudo.Name)
	if len(pseudo.Args) > 0 {
		p.print("(")
		p.printTokens(pseudo.Args)
		p.print(")")
	}
}

func (p *printer) print(text string) {
	p.sb.WriteString(text)
}

func bestQuoteCharForString(text string, allowNone bool) rune {
	noneCost := 0
	singleCost := 2
	doubleCost := 2

	for _, c := range text {
		switch c {
		case '\'':
			noneCost++
			singleCost++

		case '"':
			noneCost++
			doubleCost++

		case '(', ')', ' ', '\t':
			noneCost++

		case '\\', '\n', '\r', '\f':
			noneCost++
			singleCost++
			doubleCost++
		}
	}

	// Quotes can sometimes be omitted for URL tokens
	if allowNone && noneCost < singleCost && noneCost < doubleCost {
		return noneQuote
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

func (p *printer) printQuotedWithQuote(text string, quote rune) {
	if quote != noneQuote {
		p.sb.WriteRune(quote)
	}

	for i, c := range text {
		switch c {
		case 0, '\r', '\n', '\f':
			p.sb.WriteString(fmt.Sprintf("\\%x", c))

			// Make sure the next character is not interpreted as part of the escape sequence
			if next := i + 1; next < len(text) {
				c = rune(text[next])
				if c == ' ' || c == '\t' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
					p.sb.WriteRune(' ')
				}
			}

			// Don't print out the character being escaped itself
			continue

		case '\\', quote:
			p.sb.WriteRune('\\')

		case '(', ')', ' ', '\t', '"', '\'':
			// These characters must be escaped in URL tokens
			if quote == noneQuote {
				p.sb.WriteRune('\\')
			}
		}

		p.sb.WriteRune(c)
	}

	if quote != noneQuote {
		p.sb.WriteRune(quote)
	}
}

func (p *printer) printIndent(indent int) {
	for i := 0; i < indent; i++ {
		p.sb.WriteString("  ")
	}
}

func (p *printer) printTokens(tokens []css_ast.Token) {
	for i, t := range tokens {
		switch t.Kind {
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

		if t.HasWhitespaceAfter && i+1 != len(tokens) {
			if t.Kind == css_lexer.TComma && p.RemoveWhitespace {
				// Assume that whitespace can always be removed after a comma
			} else {
				p.print(" ")
			}
		}
	}
}
