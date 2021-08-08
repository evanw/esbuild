package css_printer

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/sourcemap"
)

const quoteForURL byte = 0

type printer struct {
	options       Options
	importRecords []ast.ImportRecord
	css           []byte
	builder       sourcemap.ChunkBuilder
}

type Options struct {
	RemoveWhitespace  bool
	ASCIIOnly         bool
	AddSourceMappings bool

	// If we're writing out a source map, this table of line start indices lets
	// us do binary search on to figure out what line a given AST node came from
	LineOffsetTables []sourcemap.LineOffsetTable

	// This will be present if the input file had a source map. In that case we
	// want to map all the way back to the original input file(s).
	InputSourceMap *sourcemap.SourceMap
}

type PrintResult struct {
	CSS            []byte
	SourceMapChunk sourcemap.Chunk
}

func Print(tree css_ast.AST, options Options) PrintResult {
	p := printer{
		options:       options,
		importRecords: tree.ImportRecords,
		builder:       sourcemap.MakeChunkBuilder(options.InputSourceMap, options.LineOffsetTables),
	}
	for _, rule := range tree.Rules {
		p.printRule(rule, 0, false)
	}
	return PrintResult{
		CSS:            p.css,
		SourceMapChunk: p.builder.GenerateChunk(p.css),
	}
}

func (p *printer) printRule(rule css_ast.Rule, indent int32, omitTrailingSemicolon bool) {
	if p.options.AddSourceMappings {
		p.builder.AddSourceMapping(rule.Loc, p.css)
	}

	if !p.options.RemoveWhitespace {
		p.printIndent(indent)
	}

	switch r := rule.Data.(type) {
	case *css_ast.RAtCharset:
		// It's not valid to remove the space in between these two tokens
		p.print("@charset ")

		// It's not valid to print the string with single quotes
		p.printQuotedWithQuote(r.Encoding, '"')
		p.print(";")

	case *css_ast.RAtImport:
		if p.options.RemoveWhitespace {
			p.print("@import")
		} else {
			p.print("@import ")
		}
		p.printQuoted(p.importRecords[r.ImportRecordIndex].Path.Text)
		p.printTokens(r.ImportConditions, printTokensOpts{})
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
		if !p.options.RemoveWhitespace {
			p.print(" ")
		}
		if p.options.RemoveWhitespace {
			p.print("{")
		} else {
			p.print("{\n")
		}
		indent++
		for _, block := range r.Blocks {
			if !p.options.RemoveWhitespace {
				p.printIndent(indent)
			}
			for i, sel := range block.Selectors {
				if i > 0 {
					if p.options.RemoveWhitespace {
						p.print(",")
					} else {
						p.print(", ")
					}
				}
				p.print(sel)
			}
			if !p.options.RemoveWhitespace {
				p.print(" ")
			}
			p.printRuleBlock(block.Rules, indent)
			if !p.options.RemoveWhitespace {
				p.print("\n")
			}
		}
		indent--
		if !p.options.RemoveWhitespace {
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
		if !p.options.RemoveWhitespace || len(r.Prelude) > 0 {
			p.print(" ")
		}
		p.printTokens(r.Prelude, printTokensOpts{})
		if !p.options.RemoveWhitespace && len(r.Prelude) > 0 {
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
		if (!p.options.RemoveWhitespace && r.Block != nil) || len(r.Prelude) > 0 {
			p.print(" ")
		}
		p.printTokens(r.Prelude, printTokensOpts{})
		if !p.options.RemoveWhitespace && r.Block != nil && len(r.Prelude) > 0 {
			p.print(" ")
		}
		if r.Block == nil {
			p.print(";")
		} else {
			p.printTokens(r.Block, printTokensOpts{})
		}

	case *css_ast.RSelector:
		p.printComplexSelectors(r.Selectors, indent)
		if !p.options.RemoveWhitespace {
			p.print(" ")
		}
		p.printRuleBlock(r.Rules, indent)

	case *css_ast.RQualified:
		hasWhitespaceAfter := p.printTokens(r.Prelude, printTokensOpts{})
		if !hasWhitespaceAfter && !p.options.RemoveWhitespace {
			p.print(" ")
		}
		p.printRuleBlock(r.Rules, indent)

	case *css_ast.RDeclaration:
		p.printIdent(r.KeyText, identNormal, canDiscardWhitespaceAfter)
		p.print(":")
		hasWhitespaceAfter := p.printTokens(r.Value, printTokensOpts{
			indent:        indent,
			isDeclaration: true,
		})
		if r.Important {
			if !hasWhitespaceAfter && !p.options.RemoveWhitespace && len(r.Value) > 0 {
				p.print(" ")
			}
			p.print("!important")
		}
		if !omitTrailingSemicolon {
			p.print(";")
		}

	case *css_ast.RBadDeclaration:
		p.printTokens(r.Tokens, printTokensOpts{})
		if !omitTrailingSemicolon {
			p.print(";")
		}

	default:
		panic("Internal error")
	}

	if !p.options.RemoveWhitespace {
		p.print("\n")
	}
}

func (p *printer) printRuleBlock(rules []css_ast.Rule, indent int32) {
	if p.options.RemoveWhitespace {
		p.print("{")
	} else {
		p.print("{\n")
	}

	for i, decl := range rules {
		omitTrailingSemicolon := p.options.RemoveWhitespace && i+1 == len(rules)
		p.printRule(decl, indent+1, omitTrailingSemicolon)
	}

	if !p.options.RemoveWhitespace {
		p.printIndent(indent)
	}
	p.print("}")
}

func (p *printer) printComplexSelectors(selectors []css_ast.ComplexSelector, indent int32) {
	for i, complex := range selectors {
		if i > 0 {
			if p.options.RemoveWhitespace {
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
		if !p.options.RemoveWhitespace {
			p.print(" ")
		}
		p.print(sel.Combinator)
		if !p.options.RemoveWhitespace {
			p.print(" ")
		}
	} else if !isFirst {
		p.print(" ")
	}

	if sel.TypeSelector != nil {
		whitespace := mayNeedWhitespaceAfter
		if len(sel.SubclassSelectors) > 0 {
			// There is no chance of whitespace before a subclass selector or pseudo
			// class selector
			whitespace = canDiscardWhitespaceAfter
		}
		p.printNamespacedName(*sel.TypeSelector, whitespace)
	}

	for i, sub := range sel.SubclassSelectors {
		whitespace := mayNeedWhitespaceAfter

		// There is no chance of whitespace between subclass selectors
		if i+1 < len(sel.SubclassSelectors) {
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
	if pseudo.IsElement {
		p.print("::")
	} else {
		p.print(":")
	}

	if len(pseudo.Args) > 0 {
		p.printIdent(pseudo.Name, identNormal, canDiscardWhitespaceAfter)
		p.print("(")
		p.printTokens(pseudo.Args, printTokensOpts{})
		p.print(")")
	} else {
		p.printIdent(pseudo.Name, identNormal, whitespace)
	}
}

func (p *printer) print(text string) {
	p.css = append(p.css, text...)
}

func bestQuoteCharForString(text string, forURL bool) byte {
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
	var temp [utf8.UTFMax]byte

	if escape == escapeBackslash && ((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
		// Hexadecimal characters cannot use a plain backslash escape
		escape = escapeHex
	}

	switch escape {
	case escapeNone:
		width := utf8.EncodeRune(temp[:], c)
		p.css = append(p.css, temp[:width]...)

	case escapeBackslash:
		p.css = append(p.css, '\\')
		width := utf8.EncodeRune(temp[:], c)
		p.css = append(p.css, temp[:width]...)

	case escapeHex:
		text := fmt.Sprintf("\\%x", c)
		p.css = append(p.css, text...)

		// Make sure the next character is not interpreted as part of the escape sequence
		if len(text) < 1+6 {
			if next := utf8.RuneLen(c); next < len(remainingText) {
				c = rune(remainingText[next])
				if c == ' ' || c == '\t' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
					p.css = append(p.css, ' ')
				}
			} else if mayNeedWhitespaceAfter {
				// If the last character is a hexadecimal escape, print a space afterwards
				// for the escape sequence to consume. That way we're sure it won't
				// accidentally consume a semantically significant space afterward.
				p.css = append(p.css, ' ')
			}
		}
	}
}

func (p *printer) printQuotedWithQuote(text string, quote byte) {
	if quote != quoteForURL {
		p.css = append(p.css, quote)
	}

	for i, c := range text {
		escape := escapeNone

		switch c {
		case '\x00', '\r', '\n', '\f':
			// Use a hexadecimal escape for characters that would be invalid escapes
			escape = escapeHex

		case '\\', rune(quote):
			escape = escapeBackslash

		case '(', ')', ' ', '\t', '"', '\'':
			// These characters must be escaped in URL tokens
			if quote == quoteForURL {
				escape = escapeBackslash
			}

		case '/':
			// Avoid generating the sequence "</style" in CSS code
			if i >= 1 && text[i-1] == '<' && strings.HasPrefix(text[i+1:], "style") {
				escape = escapeBackslash
			}

		default:
			if (p.options.ASCIIOnly && c >= 0x80) || c == '\uFEFF' {
				escape = escapeHex
			}
		}

		p.printWithEscape(c, escape, text[i:], false)
	}

	if quote != quoteForURL {
		p.css = append(p.css, quote)
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

		if p.options.ASCIIOnly && c >= 0x80 {
			escape = escapeHex
		} else if c == '\r' || c == '\n' || c == '\f' || c == '\uFEFF' {
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

func (p *printer) printIndent(indent int32) {
	for i, n := 0, int(indent); i < n; i++ {
		p.css = append(p.css, "  "...)
	}
}

type printTokensOpts struct {
	indent        int32
	isDeclaration bool
}

func (p *printer) printTokens(tokens []css_ast.Token, opts printTokensOpts) bool {
	hasWhitespaceAfter := len(tokens) > 0 && (tokens[0].Whitespace&css_ast.WhitespaceBefore) != 0

	// Pretty-print long comma-separated declarations of 3 or more items
	isMultiLineValue := false
	if !p.options.RemoveWhitespace && opts.isDeclaration {
		commaCount := 0
		for _, t := range tokens {
			if t.Kind == css_lexer.TComma {
				commaCount++
			}
		}
		isMultiLineValue = commaCount >= 2
	}

	for i, t := range tokens {
		if t.Kind == css_lexer.TWhitespace {
			hasWhitespaceAfter = true
			continue
		}
		if hasWhitespaceAfter {
			if isMultiLineValue && (i == 0 || tokens[i-1].Kind == css_lexer.TComma) {
				p.print("\n")
				p.printIndent(opts.indent + 1)
			} else {
				p.print(" ")
			}
		}
		hasWhitespaceAfter = (t.Whitespace&css_ast.WhitespaceAfter) != 0 ||
			(i+1 < len(tokens) && (tokens[i+1].Whitespace&css_ast.WhitespaceBefore) != 0)

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

		case css_lexer.THash:
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
			p.printTokens(*t.Children, printTokensOpts{})

			switch t.Kind {
			case css_lexer.TFunction:
				p.print(")")

			case css_lexer.TOpenParen:
				p.print(")")

			case css_lexer.TOpenBrace:
				p.print("}")

			case css_lexer.TOpenBracket:
				p.print("]")
			}
		}
	}
	if hasWhitespaceAfter {
		p.print(" ")
	}
	return hasWhitespaceAfter
}
