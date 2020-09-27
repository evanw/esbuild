package css_printer

import (
	"strings"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

type printer struct {
	Options
	importRecords []ast.ImportRecord
	sb            strings.Builder
}

type Options struct {
	Contents         string
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
		// Note: It's not valid to remove the space in between these two tokens
		p.print("@charset ")
		p.print(css_lexer.QuoteForStringToken(r.Encoding))
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
		p.print(css_lexer.QuoteForStringToken(r.Path))
		p.print(";")

	case *css_ast.RAtImport:
		if p.RemoveWhitespace {
			p.print("@import")
		} else {
			p.print("@import ")
		}
		p.print(css_lexer.QuoteForStringToken(p.importRecords[r.ImportRecordIndex].Path.Text))
		p.print(";")

	case *css_ast.RKnownAt:
		p.printToken(r.Name)
		p.printTokens(r.Prelude)
		p.printRuleBlock(r.Rules, indent)

	case *css_ast.RUnknownAt:
		p.printToken(r.Name)
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
		p.printToken(r.Key)
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
		for _, arg := range pseudo.Args {
			p.printToken(arg)
		}
		p.print(")")
	}
}

func (p *printer) print(text string) {
	p.sb.WriteString(text)
}

func (p *printer) printIndent(indent int) {
	for i := 0; i < indent; i++ {
		p.sb.WriteString("  ")
	}
}

func (p *printer) printToken(token css_lexer.Token) {
	if token.Kind == css_lexer.TWhitespace {
		p.print(" ")
	} else {
		p.print(token.Raw(p.Contents))
	}
}

func (p *printer) printTokens(tokens []css_lexer.Token) {
	for _, t := range tokens {
		p.printToken(t)
	}
}
