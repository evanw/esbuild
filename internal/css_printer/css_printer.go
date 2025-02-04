package css_printer

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/sourcemap"
)

const quoteForURL byte = 0

type printer struct {
	options                Options
	symbols                ast.SymbolMap
	importRecords          []ast.ImportRecord
	css                    []byte
	hasLegalComment        map[string]struct{}
	extractedLegalComments []string
	jsonMetadataImports    []string
	builder                sourcemap.ChunkBuilder
	oldLineStart           int
	oldLineEnd             int
}

type Options struct {
	// This will be present if the input file had a source map. In that case we
	// want to map all the way back to the original input file(s).
	InputSourceMap *sourcemap.SourceMap

	// If we're writing out a source map, this table of line start indices lets
	// us do binary search on to figure out what line a given AST node came from
	LineOffsetTables []sourcemap.LineOffsetTable

	// Local symbol renaming results go here
	LocalNames map[ast.Ref]string

	LineLimit           int
	InputSourceIndex    uint32
	UnsupportedFeatures compat.CSSFeature
	MinifyWhitespace    bool
	ASCIIOnly           bool
	SourceMap           config.SourceMap
	AddSourceMappings   bool
	LegalComments       config.LegalComments
	NeedsMetafile       bool
}

type PrintResult struct {
	CSS                    []byte
	ExtractedLegalComments []string
	JSONMetadataImports    []string

	// This source map chunk just contains the VLQ-encoded offsets for the "CSS"
	// field above. It's not a full source map. The bundler will be joining many
	// source map chunks together to form the final source map.
	SourceMapChunk sourcemap.Chunk
}

func Print(tree css_ast.AST, symbols ast.SymbolMap, options Options) PrintResult {
	p := printer{
		options:       options,
		symbols:       symbols,
		importRecords: tree.ImportRecords,
		builder:       sourcemap.MakeChunkBuilder(options.InputSourceMap, options.LineOffsetTables, options.ASCIIOnly),
	}
	for _, rule := range tree.Rules {
		p.printRule(rule, 0, false)
	}
	result := PrintResult{
		CSS:                    p.css,
		ExtractedLegalComments: p.extractedLegalComments,
		JSONMetadataImports:    p.jsonMetadataImports,
	}
	if options.SourceMap != config.SourceMapNone {
		// This is expensive. Only do this if it's necessary. For example, skipping
		// this if it's not needed sped up end-to-end parsing and printing of a
		// large CSS file from 66ms to 52ms (around 25% faster).
		result.SourceMapChunk = p.builder.GenerateChunk(p.css)
	}
	return result
}

func (p *printer) recordImportPathForMetafile(importRecordIndex uint32) {
	if p.options.NeedsMetafile {
		record := p.importRecords[importRecordIndex]
		external := ""
		if (record.Flags & ast.ShouldNotBeExternalInMetafile) == 0 {
			external = ",\n          \"external\": true"
		}
		p.jsonMetadataImports = append(p.jsonMetadataImports, fmt.Sprintf("\n        {\n          \"path\": %s,\n          \"kind\": %s%s\n        }",
			helpers.QuoteForJSON(record.Path.Text, p.options.ASCIIOnly),
			helpers.QuoteForJSON(record.Kind.StringForMetafile(), p.options.ASCIIOnly),
			external))
	}
}

func (p *printer) printRule(rule css_ast.Rule, indent int32, omitTrailingSemicolon bool) {
	if r, ok := rule.Data.(*css_ast.RComment); ok {
		switch p.options.LegalComments {
		case config.LegalCommentsNone:
			return

		case config.LegalCommentsEndOfFile,
			config.LegalCommentsLinkedWithComment,
			config.LegalCommentsExternalWithoutComment:

			// Don't record the same legal comment more than once per file
			if p.hasLegalComment == nil {
				p.hasLegalComment = make(map[string]struct{})
			} else if _, ok := p.hasLegalComment[r.Text]; ok {
				return
			}
			p.hasLegalComment[r.Text] = struct{}{}
			p.extractedLegalComments = append(p.extractedLegalComments, r.Text)
			return
		}
	}

	if p.options.LineLimit > 0 {
		p.printNewlinePastLineLimit(indent)
	}

	if p.options.AddSourceMappings {
		shouldPrintMapping := true
		if indent == 0 || p.options.MinifyWhitespace {
			switch rule.Data.(type) {
			case *css_ast.RSelector, *css_ast.RQualified, *css_ast.RBadDeclaration:
				// These rules will begin with a potentially more accurate mapping. We
				// shouldn't print a mapping here if there's no indent in between this
				// mapping and the rule.
				shouldPrintMapping = false
			}
		}
		if shouldPrintMapping {
			p.builder.AddSourceMapping(rule.Loc, "", p.css)
		}
	}

	if !p.options.MinifyWhitespace {
		p.printIndent(indent)
	}

	switch r := rule.Data.(type) {
	case *css_ast.RAtCharset:
		// It's not valid to remove the space in between these two tokens
		p.print("@charset ")

		// It's not valid to print the string with single quotes
		p.printQuotedWithQuote(r.Encoding, '"', 0)
		p.print(";")

	case *css_ast.RAtImport:
		if p.options.MinifyWhitespace {
			p.print("@import")
		} else {
			p.print("@import ")
		}
		record := p.importRecords[r.ImportRecordIndex]
		var flags printQuotedFlags
		if record.Flags.Has(ast.ContainsUniqueKey) {
			flags |= printQuotedNoWrap
		}
		p.printQuoted(record.Path.Text, flags)
		p.recordImportPathForMetafile(r.ImportRecordIndex)
		if conditions := r.ImportConditions; conditions != nil {
			space := !p.options.MinifyWhitespace
			if len(conditions.Layers) > 0 {
				if space {
					p.print(" ")
				}
				p.printTokens(conditions.Layers, printTokensOpts{})
				space = true
			}
			if len(conditions.Supports) > 0 {
				if space {
					p.print(" ")
				}
				p.printTokens(conditions.Supports, printTokensOpts{})
				space = true
			}
			if len(conditions.Media) > 0 {
				if space {
					p.print(" ")
				}
				p.printTokens(conditions.Media, printTokensOpts{})
			}
		}
		p.print(";")

	case *css_ast.RAtKeyframes:
		p.print("@")
		p.printIdent(r.AtToken, identNormal, mayNeedWhitespaceAfter)
		p.print(" ")
		p.printSymbol(r.Name.Loc, r.Name.Ref, identNormal, canDiscardWhitespaceAfter)
		if !p.options.MinifyWhitespace {
			p.print(" ")
		}
		if p.options.MinifyWhitespace {
			p.print("{")
		} else {
			p.print("{\n")
		}
		indent++
		for _, block := range r.Blocks {
			if p.options.AddSourceMappings {
				p.builder.AddSourceMapping(block.Loc, "", p.css)
			}
			if !p.options.MinifyWhitespace {
				p.printIndent(indent)
			}
			for i, sel := range block.Selectors {
				if i > 0 {
					if p.options.MinifyWhitespace {
						p.print(",")
					} else {
						p.print(", ")
					}
				}
				p.print(sel)
			}
			if !p.options.MinifyWhitespace {
				p.print(" ")
			}
			p.printRuleBlock(block.Rules, indent, block.CloseBraceLoc)
			if !p.options.MinifyWhitespace {
				p.print("\n")
			}
		}
		indent--
		if p.options.AddSourceMappings && r.CloseBraceLoc.Start != 0 {
			p.builder.AddSourceMapping(r.CloseBraceLoc, "", p.css)
		}
		if !p.options.MinifyWhitespace {
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
		if (!p.options.MinifyWhitespace && r.Rules != nil) || len(r.Prelude) > 0 {
			p.print(" ")
		}
		p.printTokens(r.Prelude, printTokensOpts{})
		if r.Rules == nil {
			p.print(";")
		} else {
			if !p.options.MinifyWhitespace && len(r.Prelude) > 0 {
				p.print(" ")
			}
			p.printRuleBlock(r.Rules, indent, r.CloseBraceLoc)
		}

	case *css_ast.RUnknownAt:
		p.print("@")
		whitespace := mayNeedWhitespaceAfter
		if len(r.Prelude) == 0 {
			whitespace = canDiscardWhitespaceAfter
		}
		p.printIdent(r.AtToken, identNormal, whitespace)
		if (!p.options.MinifyWhitespace && len(r.Block) != 0) || len(r.Prelude) > 0 {
			p.print(" ")
		}
		p.printTokens(r.Prelude, printTokensOpts{})
		if !p.options.MinifyWhitespace && len(r.Block) != 0 && len(r.Prelude) > 0 {
			p.print(" ")
		}
		if len(r.Block) == 0 {
			p.print(";")
		} else {
			p.printTokens(r.Block, printTokensOpts{})
		}

	case *css_ast.RSelector:
		p.printComplexSelectors(r.Selectors, indent, layoutMultiLine)
		if !p.options.MinifyWhitespace {
			p.print(" ")
		}
		p.printRuleBlock(r.Rules, indent, r.CloseBraceLoc)

	case *css_ast.RQualified:
		hasWhitespaceAfter := p.printTokens(r.Prelude, printTokensOpts{})
		if !hasWhitespaceAfter && !p.options.MinifyWhitespace {
			p.print(" ")
		}
		p.printRuleBlock(r.Rules, indent, r.CloseBraceLoc)

	case *css_ast.RDeclaration:
		p.printIdent(r.KeyText, identNormal, canDiscardWhitespaceAfter)
		p.print(":")
		hasWhitespaceAfter := p.printTokens(r.Value, printTokensOpts{
			indent:        indent,
			isDeclaration: true,
		})
		if r.Important {
			if !hasWhitespaceAfter && !p.options.MinifyWhitespace && len(r.Value) > 0 {
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

	case *css_ast.RComment:
		p.printIndentedComment(indent, r.Text)

	case *css_ast.RAtLayer:
		p.print("@layer")
		for i, parts := range r.Names {
			if i == 0 {
				p.print(" ")
			} else if !p.options.MinifyWhitespace {
				p.print(", ")
			} else {
				p.print(",")
			}
			p.print(strings.Join(parts, "."))
		}
		if r.Rules == nil {
			p.print(";")
		} else {
			if !p.options.MinifyWhitespace {
				p.print(" ")
			}
			p.printRuleBlock(r.Rules, indent, r.CloseBraceLoc)
		}

	default:
		panic("Internal error")
	}

	if !p.options.MinifyWhitespace {
		p.print("\n")
	}
}

func (p *printer) printIndentedComment(indent int32, text string) {
	// Avoid generating a comment containing the character sequence "</style"
	if !p.options.UnsupportedFeatures.Has(compat.InlineStyle) {
		text = helpers.EscapeClosingTag(text, "/style")
	}

	// Re-indent multi-line comments
	for {
		newline := strings.IndexByte(text, '\n')
		if newline == -1 {
			break
		}
		p.print(text[:newline+1])
		if !p.options.MinifyWhitespace {
			p.printIndent(indent)
		}
		text = text[newline+1:]
	}
	p.print(text)
}

func (p *printer) printRuleBlock(rules []css_ast.Rule, indent int32, closeBraceLoc logger.Loc) {
	if p.options.MinifyWhitespace {
		p.print("{")
	} else {
		p.print("{\n")
	}

	for i, decl := range rules {
		omitTrailingSemicolon := p.options.MinifyWhitespace && i+1 == len(rules)
		p.printRule(decl, indent+1, omitTrailingSemicolon)
	}

	if p.options.AddSourceMappings && closeBraceLoc.Start != 0 {
		p.builder.AddSourceMapping(closeBraceLoc, "", p.css)
	}
	if !p.options.MinifyWhitespace {
		p.printIndent(indent)
	}
	p.print("}")
}

type selectorLayout uint8

const (
	layoutMultiLine selectorLayout = iota
	layoutSingleLine
)

func (p *printer) printComplexSelectors(selectors []css_ast.ComplexSelector, indent int32, layout selectorLayout) {
	for i, complex := range selectors {
		if i > 0 {
			if p.options.MinifyWhitespace {
				p.print(",")
				if p.options.LineLimit > 0 {
					p.printNewlinePastLineLimit(indent)
				}
			} else if layout == layoutMultiLine {
				p.print(",\n")
				p.printIndent(indent)
			} else {
				p.print(", ")
			}
		}

		for j, compound := range complex.Selectors {
			p.printCompoundSelector(compound, j == 0, indent)
		}
	}
}

func (p *printer) printCompoundSelector(sel css_ast.CompoundSelector, isFirst bool, indent int32) {
	if !isFirst && sel.Combinator.Byte == 0 {
		// A space is required in between compound selectors if there is no
		// combinator in the middle. It's fine to convert "a + b" into "a+b"
		// but not to convert "a b" into "ab".
		if p.options.LineLimit <= 0 || !p.printNewlinePastLineLimit(indent) {
			p.print(" ")
		}
	}

	if sel.Combinator.Byte != 0 {
		if !isFirst && !p.options.MinifyWhitespace {
			p.print(" ")
		}

		if p.options.AddSourceMappings {
			p.builder.AddSourceMapping(sel.Combinator.Loc, "", p.css)
		}
		p.css = append(p.css, sel.Combinator.Byte)

		if (p.options.LineLimit <= 0 || !p.printNewlinePastLineLimit(indent)) && !p.options.MinifyWhitespace {
			p.print(" ")
		}
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

	for _, loc := range sel.NestingSelectorLocs {
		if p.options.AddSourceMappings {
			p.builder.AddSourceMapping(loc, "", p.css)
		}

		p.print("&")
	}

	for i, ss := range sel.SubclassSelectors {
		whitespace := mayNeedWhitespaceAfter

		// There is no chance of whitespace between subclass selectors
		if i+1 < len(sel.SubclassSelectors) {
			whitespace = canDiscardWhitespaceAfter
		}

		if p.options.AddSourceMappings {
			p.builder.AddSourceMapping(ss.Range.Loc, "", p.css)
		}

		switch s := ss.Data.(type) {
		case *css_ast.SSHash:
			p.print("#")

			// This deliberately does not use identHash. From the specification:
			// "In <id-selector>, the <hash-token>'s value must be an identifier."
			p.printSymbol(s.Name.Loc, s.Name.Ref, identNormal, whitespace)

		case *css_ast.SSClass:
			p.print(".")
			p.printSymbol(s.Name.Loc, s.Name.Ref, identNormal, whitespace)

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
					p.printQuoted(s.MatcherValue, 0)
				}
			}
			if s.MatcherModifier != 0 {
				p.print(" ")
				p.print(string(rune(s.MatcherModifier)))
			}
			p.print("]")

		case *css_ast.SSPseudoClass:
			p.printPseudoClassSelector(*s, whitespace)

		case *css_ast.SSPseudoClassWithSelectorList:
			p.print(":")
			p.print(s.Kind.String())
			p.print("(")
			if s.Index.A != "" || s.Index.B != "" {
				p.printNthIndex(s.Index)
				if len(s.Selectors) > 0 {
					if p.options.MinifyWhitespace && s.Selectors[0].Selectors[0].TypeSelector == nil {
						p.print(" of")
					} else {
						p.print(" of ")
					}
				}
			}
			p.printComplexSelectors(s.Selectors, indent, layoutSingleLine)
			p.print(")")

		default:
			panic("Internal error")
		}
	}
}

func (p *printer) printNthIndex(index css_ast.NthIndex) {
	if index.A != "" {
		if index.A == "-1" {
			p.print("-")
		} else if index.A != "1" {
			p.print(index.A)
		}
		p.print("n")
		if index.B != "" {
			if !strings.HasPrefix(index.B, "-") {
				p.print("+")
			}
			p.print(index.B)
		}
	} else if index.B != "" {
		p.print(index.B)
	}
}

func (p *printer) printNamespacedName(nsName css_ast.NamespacedName, whitespace trailingWhitespace) {
	if prefix := nsName.NamespacePrefix; prefix != nil {
		if p.options.AddSourceMappings {
			p.builder.AddSourceMapping(prefix.Range.Loc, "", p.css)
		}

		switch prefix.Kind {
		case css_lexer.TIdent:
			p.printIdent(prefix.Text, identNormal, canDiscardWhitespaceAfter)
		case css_lexer.TDelimAsterisk:
			p.print("*")
		default:
			panic("Internal error")
		}

		p.print("|")
	}

	if p.options.AddSourceMappings {
		p.builder.AddSourceMapping(nsName.Name.Range.Loc, "", p.css)
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

	// This checks for "nil" so we can distinguish ":is()" from ":is"
	if pseudo.Args != nil {
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

type printQuotedFlags uint8

const (
	printQuotedNoWrap printQuotedFlags = 1 << iota
)

func (p *printer) printQuoted(text string, flags printQuotedFlags) {
	p.printQuotedWithQuote(text, bestQuoteCharForString(text, false), flags)
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

// Note: This function is hot in profiles
func (p *printer) printQuotedWithQuote(text string, quote byte, flags printQuotedFlags) {
	if quote != quoteForURL {
		p.css = append(p.css, quote)
	}

	n := len(text)
	i := 0
	runStart := 0

	// Only compute the line length if necessary
	var startLineLength int
	wrapLongLines := false
	if p.options.LineLimit > 0 && quote != quoteForURL && (flags&printQuotedNoWrap) == 0 {
		startLineLength = p.currentLineLength()
		if startLineLength > p.options.LineLimit {
			startLineLength = p.options.LineLimit
		}
		wrapLongLines = true
	}

	for i < n {
		// Wrap long lines that are over the limit using escaped newlines
		if wrapLongLines && startLineLength+i >= p.options.LineLimit {
			if runStart < i {
				p.css = append(p.css, text[runStart:i]...)
				runStart = i
			}
			p.css = append(p.css, "\\\n"...)
			startLineLength -= p.options.LineLimit
		}

		c, width := utf8.DecodeRuneInString(text[i:])
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
			if !p.options.UnsupportedFeatures.Has(compat.InlineStyle) && i >= 1 && text[i-1] == '<' && i+6 <= len(text) && strings.EqualFold(text[i+1:i+6], "style") {
				escape = escapeBackslash
			}

		default:
			if (p.options.ASCIIOnly && c >= 0x80) || c == '\uFEFF' {
				escape = escapeHex
			}
		}

		if escape != escapeNone {
			if runStart < i {
				p.css = append(p.css, text[runStart:i]...)
			}
			p.printWithEscape(c, escape, text[i:], false)
			runStart = i + width
		}
		i += width
	}

	if runStart < n {
		p.css = append(p.css, text[runStart:]...)
	}

	if quote != quoteForURL {
		p.css = append(p.css, quote)
	}
}

func (p *printer) currentLineLength() int {
	css := p.css
	n := len(css)
	stop := p.oldLineEnd

	// Update "oldLineStart" to the start of the current line
	for i := n; i > stop; i-- {
		if c := css[i-1]; c == '\r' || c == '\n' {
			p.oldLineStart = i
			break
		}
	}

	p.oldLineEnd = n
	return n - p.oldLineStart
}

func (p *printer) printNewlinePastLineLimit(indent int32) bool {
	if p.currentLineLength() < p.options.LineLimit {
		return false
	}
	p.print("\n")
	if !p.options.MinifyWhitespace {
		p.printIndent(indent)
	}
	return true
}

type identMode uint8

const (
	identNormal identMode = iota
	identHash
	identDimensionUnit
	identDimensionUnitAfterExponent
)

type trailingWhitespace uint8

const (
	mayNeedWhitespaceAfter trailingWhitespace = iota
	canDiscardWhitespaceAfter
)

// Note: This function is hot in profiles
func (p *printer) printIdent(text string, mode identMode, whitespace trailingWhitespace) {
	n := len(text)

	// Special escape behavior for the first character
	initialEscape := escapeNone
	switch mode {
	case identNormal:
		if !css_lexer.WouldStartIdentifierWithoutEscapes(text) {
			initialEscape = escapeBackslash
		}
	case identDimensionUnit, identDimensionUnitAfterExponent:
		if !css_lexer.WouldStartIdentifierWithoutEscapes(text) {
			initialEscape = escapeBackslash
		} else if n > 0 {
			if c := text[0]; c >= '0' && c <= '9' {
				// Unit: "2x"
				initialEscape = escapeHex
			} else if (c == 'e' || c == 'E') && mode != identDimensionUnitAfterExponent {
				if n >= 2 && text[1] >= '0' && text[1] <= '9' {
					// Unit: "e2x"
					initialEscape = escapeHex
				} else if n >= 3 && text[1] == '-' && text[2] >= '0' && text[2] <= '9' {
					// Unit: "e-2x"
					initialEscape = escapeHex
				}
			}
		}
	}

	// Fast path: the identifier does not need to be escaped. This fast path is
	// important for performance. For example, doing this sped up end-to-end
	// parsing and printing of a large CSS file from 84ms to 66ms (around 25%
	// faster).
	if initialEscape == escapeNone {
		for i := 0; i < n; i++ {
			if c := text[i]; c >= 0x80 || !css_lexer.IsNameContinue(rune(c)) {
				goto slowPath
			}
		}
		p.css = append(p.css, text...)
		return
	slowPath:
	}

	// Slow path: the identifier needs to be escaped
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
			if i == 0 && initialEscape != escapeNone {
				escape = initialEscape
			}
		}

		// If the last character is a hexadecimal escape, print a space afterwards
		// for the escape sequence to consume. That way we're sure it won't
		// accidentally consume a semantically significant space afterward.
		mayNeedWhitespaceAfter := whitespace == mayNeedWhitespaceAfter && escape != escapeNone && i+utf8.RuneLen(c) == n
		p.printWithEscape(c, escape, text[i:], mayNeedWhitespaceAfter)
	}
}

func (p *printer) printSymbol(loc logger.Loc, ref ast.Ref, mode identMode, whitespace trailingWhitespace) {
	ref = ast.FollowSymbols(p.symbols, ref)
	originalName := p.symbols.Get(ref).OriginalName
	name, ok := p.options.LocalNames[ref]
	if !ok {
		name = originalName
	}
	if p.options.AddSourceMappings {
		if originalName == name {
			originalName = ""
		}
		p.builder.AddSourceMapping(loc, originalName, p.css)
	}
	p.printIdent(name, mode, whitespace)
}

func (p *printer) printIndent(indent int32) {
	n := int(indent)
	if p.options.LineLimit > 0 && n*2 >= p.options.LineLimit {
		n = p.options.LineLimit / 2
	}
	for i := 0; i < n; i++ {
		p.css = append(p.css, "  "...)
	}
}

type printTokensOpts struct {
	indent               int32
	multiLineCommaPeriod uint8
	isDeclaration        bool
}

func functionMultiLineCommaPeriod(token css_ast.Token) uint8 {
	if token.Kind == css_lexer.TFunction {
		commaCount := 0
		for _, t := range *token.Children {
			if t.Kind == css_lexer.TComma {
				commaCount++
			}
		}

		switch strings.ToLower(token.Text) {
		case "linear-gradient", "radial-gradient", "conic-gradient",
			"repeating-linear-gradient", "repeating-radial-gradient", "repeating-conic-gradient":
			if commaCount >= 2 {
				return 1
			}

		case "matrix":
			if commaCount == 5 {
				return 2
			}

		case "matrix3d":
			if commaCount == 15 {
				return 4
			}
		}
	}
	return 0
}

func (p *printer) printTokens(tokens []css_ast.Token, opts printTokensOpts) bool {
	hasWhitespaceAfter := len(tokens) > 0 && (tokens[0].Whitespace&css_ast.WhitespaceBefore) != 0

	// Pretty-print long comma-separated declarations of 3 or more items
	commaPeriod := int(opts.multiLineCommaPeriod)
	if !p.options.MinifyWhitespace && opts.isDeclaration {
		commaCount := 0
		for _, t := range tokens {
			if t.Kind == css_lexer.TComma {
				commaCount++
				if commaCount >= 2 {
					commaPeriod = 1
					break
				}
			}
			if t.Kind == css_lexer.TFunction && functionMultiLineCommaPeriod(t) > 0 {
				commaPeriod = 1
				break
			}
		}
	}

	commaCount := 0
	for i, t := range tokens {
		if t.Kind == css_lexer.TComma {
			commaCount++
		}
		if t.Kind == css_lexer.TWhitespace {
			hasWhitespaceAfter = true
			continue
		}
		if hasWhitespaceAfter {
			if commaPeriod > 0 && (i == 0 || (tokens[i-1].Kind == css_lexer.TComma && commaCount%commaPeriod == 0)) {
				p.print("\n")
				p.printIndent(opts.indent + 1)
			} else if p.options.LineLimit <= 0 || !p.printNewlinePastLineLimit(opts.indent+1) {
				p.print(" ")
			}
		}
		hasWhitespaceAfter = (t.Whitespace&css_ast.WhitespaceAfter) != 0 ||
			(i+1 < len(tokens) && (tokens[i+1].Whitespace&css_ast.WhitespaceBefore) != 0)

		whitespace := mayNeedWhitespaceAfter
		if !hasWhitespaceAfter {
			whitespace = canDiscardWhitespaceAfter
		}

		if p.options.AddSourceMappings {
			p.builder.AddSourceMapping(t.Loc, "", p.css)
		}

		switch t.Kind {
		case css_lexer.TIdent:
			p.printIdent(t.Text, identNormal, whitespace)

		case css_lexer.TSymbol:
			ref := ast.Ref{SourceIndex: p.options.InputSourceIndex, InnerIndex: t.PayloadIndex}
			p.printSymbol(t.Loc, ref, identNormal, whitespace)

		case css_lexer.TFunction:
			p.printIdent(t.Text, identNormal, whitespace)
			p.print("(")

		case css_lexer.TDimension:
			value := t.DimensionValue()
			p.print(value)
			mode := identDimensionUnit
			if strings.ContainsAny(value, "eE") {
				mode = identDimensionUnitAfterExponent
			}
			p.printIdent(t.DimensionUnit(), mode, whitespace)

		case css_lexer.TAtKeyword:
			p.print("@")
			p.printIdent(t.Text, identNormal, whitespace)

		case css_lexer.THash:
			p.print("#")
			p.printIdent(t.Text, identHash, whitespace)

		case css_lexer.TString:
			p.printQuoted(t.Text, 0)

		case css_lexer.TURL:
			record := p.importRecords[t.PayloadIndex]
			text := record.Path.Text
			tryToAvoidQuote := true
			var flags printQuotedFlags
			if record.Flags.Has(ast.ContainsUniqueKey) {
				flags |= printQuotedNoWrap

				// If the caller will be substituting a path here later using string
				// substitution, then we can't be sure that it will form a valid URL
				// token when unquoted (e.g. it may contain spaces). So we need to
				// quote the unique key here just in case. For more info see this
				// issue: https://github.com/evanw/esbuild/issues/3410
				tryToAvoidQuote = false
			} else if p.options.LineLimit > 0 && p.currentLineLength()+len(text) >= p.options.LineLimit {
				tryToAvoidQuote = false
			}
			p.print("url(")
			p.printQuotedWithQuote(text, bestQuoteCharForString(text, tryToAvoidQuote), flags)
			p.print(")")
			p.recordImportPathForMetafile(t.PayloadIndex)

		case css_lexer.TUnterminatedString:
			// We must end this with a newline so that this string stays unterminated
			p.print(t.Text)
			p.print("\n")
			if !p.options.MinifyWhitespace {
				p.printIndent(opts.indent)
			}
			hasWhitespaceAfter = false

		default:
			p.print(t.Text)
		}

		if t.Children != nil {
			childCommaPeriod := uint8(0)

			if commaPeriod > 0 && opts.isDeclaration {
				childCommaPeriod = functionMultiLineCommaPeriod(t)
			}

			if childCommaPeriod > 0 {
				opts.indent++
				if !p.options.MinifyWhitespace {
					p.print("\n")
					p.printIndent(opts.indent + 1)
				}
			}

			p.printTokens(*t.Children, printTokensOpts{
				indent:               opts.indent,
				multiLineCommaPeriod: childCommaPeriod,
			})

			if childCommaPeriod > 0 {
				opts.indent--
			}

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
