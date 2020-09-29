package css_parser

import (
	"fmt"
	"strings"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/logger"
)

// This is mostly a normal CSS parser with one exception: the addition of
// support for parsing https://drafts.csswg.org/css-nesting-1/.

type parser struct {
	log           logger.Log
	source        logger.Source
	options       config.Options
	tokens        []css_lexer.Token
	stack         []css_lexer.T
	index         int
	end           int
	prevError     logger.Loc
	importRecords []ast.ImportRecord
}

func Parse(log logger.Log, source logger.Source, options config.Options) css_ast.AST {
	p := parser{
		log:       log,
		source:    source,
		options:   options,
		tokens:    css_lexer.Tokenize(log, source),
		prevError: logger.Loc{Start: -1},
	}
	p.end = len(p.tokens)
	tree := css_ast.AST{}
	tree.Rules = p.parseListOfRules(ruleContext{
		isTopLevel:     true,
		parseSelectors: true,
	})
	tree.ImportRecords = p.importRecords
	p.expect(css_lexer.TEndOfFile)
	return tree
}

func (p *parser) advance() {
	if p.index < p.end {
		p.index++
	}
}

func (p *parser) at(index int) css_lexer.Token {
	if index < p.end {
		return p.tokens[index]
	}
	if p.end < len(p.tokens) {
		return css_lexer.Token{
			Kind:  css_lexer.TEndOfFile,
			Range: logger.Range{Loc: p.tokens[p.end].Range.Loc},
		}
	}
	return css_lexer.Token{
		Kind:  css_lexer.TEndOfFile,
		Range: logger.Range{Loc: logger.Loc{Start: int32(len(p.source.Contents))}},
	}
}

func (p *parser) current() css_lexer.Token {
	return p.at(p.index)
}

func (p *parser) next() css_lexer.Token {
	return p.at(p.index + 1)
}

func (p *parser) text() string {
	return p.current().Raw(p.source.Contents)
}

func (p *parser) peek(kind css_lexer.T) bool {
	return kind == p.current().Kind
}

func (p *parser) eat(kind css_lexer.T) bool {
	if p.peek(kind) {
		p.advance()
		return true
	}
	return false
}

func (p *parser) expect(kind css_lexer.T) bool {
	if p.eat(kind) {
		return true
	}
	t := p.current()
	var text string
	if kind == css_lexer.TSemicolon && p.index > 0 && p.at(p.index-1).Kind == css_lexer.TWhitespace {
		// Have a nice error message for forgetting a trailing semicolon
		text = fmt.Sprintf("Expected \";\"")
		t = p.at(p.index - 1)
	} else {
		switch t.Kind {
		case css_lexer.TEndOfFile, css_lexer.TWhitespace:
			text = fmt.Sprintf("Expected %s but found %s", kind.String(), t.Kind.String())
			t.Range.Len = 0
		case css_lexer.TBadURL, css_lexer.TBadString:
			text = fmt.Sprintf("Expected %s but found %s", kind.String(), t.Kind.String())
		default:
			text = fmt.Sprintf("Expected %s but found %q", kind.String(), p.text())
		}
	}
	if t.Range.Loc.Start > p.prevError.Start {
		p.log.AddRangeWarning(&p.source, t.Range, text)
		p.prevError = t.Range.Loc
	}
	return false
}

func (p *parser) unexpected() {
	if t := p.current(); t.Range.Loc.Start > p.prevError.Start {
		var text string
		switch t.Kind {
		case css_lexer.TEndOfFile, css_lexer.TWhitespace:
			text = fmt.Sprintf("Unexpected %s", t.Kind.String())
			t.Range.Len = 0
		case css_lexer.TBadURL, css_lexer.TBadString:
			text = fmt.Sprintf("Unexpected %s", t.Kind.String())
		default:
			text = fmt.Sprintf("Unexpected %q", p.text())
		}
		p.log.AddRangeWarning(&p.source, t.Range, text)
		p.prevError = t.Range.Loc
	}
}

type ruleContext struct {
	isTopLevel     bool
	parseSelectors bool
}

func (p *parser) parseListOfRules(context ruleContext) []css_ast.R {
	rules := []css_ast.R{}
	for {
		switch p.current().Kind {
		case css_lexer.TEndOfFile, css_lexer.TCloseBrace:
			return rules

		case css_lexer.TWhitespace:
			p.advance()
			continue

		case css_lexer.TAtKeyword:
			rules = append(rules, p.parseAtRule(atRuleContext{}))
			continue

		case css_lexer.TCDO, css_lexer.TCDC:
			if context.isTopLevel {
				p.advance()
				continue
			}
		}

		if context.parseSelectors {
			rules = append(rules, p.parseSelectorRule())
		} else {
			rules = append(rules, p.parseQualifiedRuleFrom(p.index))
		}
	}
}

func (p *parser) parseListOfDeclarations() (list []css_ast.R) {
	for {
		switch p.current().Kind {
		case css_lexer.TWhitespace, css_lexer.TSemicolon:
			p.advance()

		case css_lexer.TEndOfFile, css_lexer.TCloseBrace:
			return

		case css_lexer.TAtKeyword:
			list = append(list, p.parseAtRule(atRuleContext{
				isDeclarationList: true,
			}))

		case css_lexer.TDelimAmpersand:
			// Reference: https://drafts.csswg.org/css-nesting-1/
			list = append(list, p.parseSelectorRule())

		default:
			list = append(list, p.parseDeclaration())
		}
	}
}

func (p *parser) parseURLOrString() (string, logger.Range, bool) {
	t := p.current()
	switch t.Kind {
	case css_lexer.TString:
		p.advance()
		return css_lexer.ContentsOfStringToken(t.Raw(p.source.Contents)), t.Range, true

	case css_lexer.TURL:
		p.advance()
		text, r := css_lexer.ContentsOfURLToken(t.Raw(p.source.Contents))
		r.Loc.Start += t.Range.Loc.Start
		return text, r, true

	case css_lexer.TFunction:
		if t.Raw(p.source.Contents) == "url(" {
			p.advance()
			t = p.current()
			if p.expect(css_lexer.TString) && p.expect(css_lexer.TCloseParen) {
				return css_lexer.ContentsOfStringToken(t.Raw(p.source.Contents)), t.Range, true
			}
		}
	}

	return "", logger.Range{}, false
}

func (p *parser) expectURLOrString() (url string, r logger.Range, ok bool) {
	url, r, ok = p.parseURLOrString()
	if !ok {
		p.expect(css_lexer.TURL)
	}
	return
}

type atRuleKind uint8

const (
	atRuleUnknown atRuleKind = iota
	atRuleDeclarations
	atRuleInheritContext
	atRuleEmpty
)

var specialAtRules = map[string]atRuleKind{
	"@font-face": atRuleDeclarations,
	"@page":      atRuleDeclarations,

	"@document": atRuleInheritContext,
	"@media":    atRuleInheritContext,
	"@scope":    atRuleInheritContext,
	"@supports": atRuleInheritContext,
}

type atRuleContext struct {
	isDeclarationList bool
}

func (p *parser) parseAtRule(context atRuleContext) css_ast.R {
	// Parse the name
	atToken := p.text()
	atRange := p.current().Range
	kind := specialAtRules[atToken]
	p.advance()

	// Parse the prelude
	preludeStart := p.index
	switch atToken {
	case "@charset":
		kind = atRuleEmpty
		p.expect(css_lexer.TWhitespace)
		if p.peek(css_lexer.TString) {
			encoding := css_lexer.ContentsOfStringToken(p.text())
			if encoding != "UTF-8" {
				p.log.AddRangeWarning(&p.source, p.current().Range,
					fmt.Sprintf("\"UTF-8\" will be used instead of unsupported charset %q", encoding))
			}
			p.advance()
			p.expect(css_lexer.TSemicolon)
			return &css_ast.RAtCharset{Encoding: encoding}
		}
		p.expect(css_lexer.TString)

	case "@namespace":
		kind = atRuleEmpty
		p.eat(css_lexer.TWhitespace)
		prefix := ""
		if p.peek(css_lexer.TIdent) {
			prefix = p.text()
			p.advance()
			p.eat(css_lexer.TWhitespace)
		}
		if path, _, ok := p.expectURLOrString(); ok {
			p.eat(css_lexer.TWhitespace)
			p.expect(css_lexer.TSemicolon)
			return &css_ast.RAtNamespace{Prefix: prefix, Path: path}
		}

	case "@import":
		kind = atRuleEmpty
		p.eat(css_lexer.TWhitespace)
		if path, r, ok := p.expectURLOrString(); ok {
			p.eat(css_lexer.TWhitespace)
			p.expect(css_lexer.TSemicolon)
			importRecordIndex := uint32(len(p.importRecords))
			p.importRecords = append(p.importRecords, ast.ImportRecord{
				Kind:  ast.AtImport,
				Path:  logger.Path{Text: path},
				Range: r,
			})
			return &css_ast.RAtImport{ImportRecordIndex: importRecordIndex}
		}

	case "@keyframes", "@-webkit-keyframes", "@-moz-keyframes", "@-ms-keyframes", "@-o-keyframes":
		p.eat(css_lexer.TWhitespace)
		name := p.current()

		// Consider string names a syntax error even though they are allowed by
		// the specification and they work in Firefox because they do not work in
		// Chrome or Safari.
		if p.expect(css_lexer.TIdent) || p.eat(css_lexer.TString) || p.peek(css_lexer.TOpenBrace) {
			p.eat(css_lexer.TWhitespace)
			if p.expect(css_lexer.TOpenBrace) {
				var blocks []css_ast.KeyframeBlock

			blocks:
				for {
					switch p.current().Kind {
					case css_lexer.TWhitespace:
						p.advance()
						continue

					case css_lexer.TCloseBrace, css_lexer.TEndOfFile:
						break blocks

					case css_lexer.TOpenBrace:
						p.expect(css_lexer.TPercentage)
						p.parseComponentValue()

					default:
						var selectors []string

					selectors:
						for {
							t := p.current()
							switch t.Kind {
							case css_lexer.TWhitespace:
								p.advance()
								continue

							case css_lexer.TOpenBrace, css_lexer.TEndOfFile:
								break selectors

							case css_lexer.TIdent, css_lexer.TPercentage:
								text := t.Raw(p.source.Contents)
								if t.Kind == css_lexer.TIdent {
									if text == "from" {
										if p.options.MangleSyntax {
											text = "0%" // "0%" is equivalent to but shorter than "from"
										}
									} else if text != "to" {
										p.expect(css_lexer.TPercentage)
									}
								} else if p.options.MangleSyntax && text == "100%" {
									text = "to" // "to" is equivalent to but shorter than "100%"
								}
								selectors = append(selectors, text)
								p.advance()

							default:
								p.expect(css_lexer.TPercentage)
								p.parseComponentValue()
							}

							p.eat(css_lexer.TWhitespace)
							if t.Kind != css_lexer.TComma && !p.peek(css_lexer.TOpenBrace) {
								p.expect(css_lexer.TComma)
							}
						}

						if p.expect(css_lexer.TOpenBrace) {
							rules := p.parseListOfDeclarations()
							p.expect(css_lexer.TCloseBrace)
							blocks = append(blocks, css_ast.KeyframeBlock{
								Selectors: selectors,
								Rules:     rules,
							})
						}
					}
				}

				p.expect(css_lexer.TCloseBrace)
				return &css_ast.RAtKeyframes{
					AtToken: atToken,
					Name:    name.Raw(p.source.Contents),
					Blocks:  blocks,
				}
			}
		}

	default:
		// Warn about unsupported at-rules since they will be passed through
		// unmodified and may be part of a CSS preprocessor syntax that should
		// have been compiled away but wasn't.
		//
		// The list of supported at-rules that esbuild draws from is here:
		// https://developer.mozilla.org/en-US/docs/Web/CSS/At-rule. Deprecated
		// and Firefox-only at-rules have been removed.
		if kind == atRuleUnknown {
			p.log.AddRangeWarning(&p.source, atRange, fmt.Sprintf("%q is not a known rule name", atToken))
		}
	}

	// Parse an unknown prelude
prelude:
	for {
		switch p.current().Kind {
		case css_lexer.TOpenBrace, css_lexer.TEndOfFile:
			break prelude

		case css_lexer.TSemicolon, css_lexer.TCloseBrace:
			prelude := p.convertTokens(p.tokens[preludeStart:p.index])

			// Report an error for rules that should have blocks
			if kind != atRuleEmpty && kind != atRuleUnknown {
				p.expect(css_lexer.TOpenBrace)
				p.eat(css_lexer.TSemicolon)
				return &css_ast.RUnknownAt{AtToken: atToken, Prelude: prelude}
			}

			// Otherwise, parse an unknown at rule
			p.expect(css_lexer.TSemicolon)
			return &css_ast.RUnknownAt{AtToken: atToken, Prelude: prelude}

		default:
			p.parseComponentValue()
		}
	}
	prelude := p.convertTokens(p.tokens[preludeStart:p.index])
	blockStart := p.index

	switch kind {
	case atRuleEmpty:
		// Report an error for rules that shouldn't have blocks
		p.expect(css_lexer.TSemicolon)
		p.parseBlock(css_lexer.TOpenBrace, css_lexer.TCloseBrace)
		block := p.convertTokens(p.tokens[blockStart:p.index])
		return &css_ast.RUnknownAt{AtToken: atToken, Prelude: prelude, Block: block}

	case atRuleDeclarations:
		// Parse known rules whose blocks consist of whatever the current context is
		p.advance()
		rules := p.parseListOfDeclarations()
		p.expect(css_lexer.TCloseBrace)
		return &css_ast.RKnownAt{AtToken: atToken, Prelude: prelude, Rules: rules}

	case atRuleInheritContext:
		// Parse known rules whose blocks consist of whatever the current context is
		p.advance()
		var rules []css_ast.R
		if context.isDeclarationList {
			rules = p.parseListOfDeclarations()
		} else {
			rules = p.parseListOfRules(ruleContext{
				parseSelectors: true,
			})
		}
		p.expect(css_lexer.TCloseBrace)
		return &css_ast.RKnownAt{AtToken: atToken, Prelude: prelude, Rules: rules}

	default:
		// Otherwise, parse an unknown rule
		p.parseBlock(css_lexer.TOpenBrace, css_lexer.TCloseBrace)
		block := p.convertTokensWithImports(p.tokens[blockStart:p.index])
		return &css_ast.RUnknownAt{AtToken: atToken, Prelude: prelude, Block: block}
	}
}

func (p *parser) convertTokens(tokens []css_lexer.Token) []css_ast.Token {
	result, _ := p.convertTokensHelper(tokens, css_lexer.TEndOfFile, false)
	return result
}

func (p *parser) convertTokensWithImports(tokens []css_lexer.Token) []css_ast.Token {
	result, _ := p.convertTokensHelper(tokens, css_lexer.TEndOfFile, true)
	return result
}

func (p *parser) convertTokensHelper(tokens []css_lexer.Token, close css_lexer.T, allowImports bool) ([]css_ast.Token, []css_lexer.Token) {
	var result []css_ast.Token
loop:
	for len(tokens) > 0 {
		t := tokens[0]
		tokens = tokens[1:]
		token := css_ast.Token{
			Kind: t.Kind,
			Text: t.Raw(p.source.Contents),
		}

		switch t.Kind {
		case css_lexer.TWhitespace:
			if last := len(result) - 1; last >= 0 {
				result[last].HasWhitespaceAfter = true
			}
			continue

		case css_lexer.TComma:
			// Assume that whitespace can always be removed before a comma
			if last := len(result) - 1; last >= 0 {
				result[last].HasWhitespaceAfter = false
			}

			// Assume whitespace can always be added after a comma (it will be
			// automatically omitted by the printer if we're minifying)
			token.HasWhitespaceAfter = true

		case css_lexer.TString:
			token.Text = css_lexer.ContentsOfStringToken(token.Text)

		case css_lexer.TURL:
			path, r := css_lexer.ContentsOfURLToken(token.Text)
			r.Loc.Start += t.Range.Loc.Start
			token.Text = ""
			token.ImportRecordIndex = uint32(len(p.importRecords))
			p.importRecords = append(p.importRecords, ast.ImportRecord{
				Kind:     ast.URLToken,
				Path:     logger.Path{Text: path},
				Range:    r,
				IsUnused: !allowImports,
			})

		case css_lexer.TFunction:
			var nested []css_ast.Token
			original := tokens
			nested, tokens = p.convertTokensHelper(tokens, css_lexer.TCloseParen, allowImports)
			token.Children = &nested

			switch token.Text {
			case "url(":
				// Treat a URL function call with a string just like a URL token
				if len(nested) == 1 {
					if arg := nested[0]; arg.Kind == css_lexer.TString {
						token.Kind = css_lexer.TURL
						token.Text = ""
						token.Children = nil
						token.ImportRecordIndex = uint32(len(p.importRecords))
						p.importRecords = append(p.importRecords, ast.ImportRecord{
							Kind:     ast.URLToken,
							Path:     logger.Path{Text: arg.Text},
							Range:    original[0].Range,
							IsUnused: !allowImports,
						})
					}
				}
			}

		case css_lexer.TOpenParen:
			var nested []css_ast.Token
			nested, tokens = p.convertTokensHelper(tokens, css_lexer.TCloseParen, allowImports)
			token.Children = &nested

		case css_lexer.TOpenBrace:
			var nested []css_ast.Token
			nested, tokens = p.convertTokensHelper(tokens, css_lexer.TCloseBrace, allowImports)
			token.Children = &nested

		case css_lexer.TOpenBracket:
			var nested []css_ast.Token
			nested, tokens = p.convertTokensHelper(tokens, css_lexer.TCloseBracket, allowImports)
			token.Children = &nested

		default:
			if t.Kind == close {
				break loop
			}
		}

		result = append(result, token)
	}
	return result, tokens
}

func (p *parser) parseSelectorRule() css_ast.R {
	preludeStart := p.index

	// Try parsing the prelude as a selector list
	if list, ok := p.parseSelectorList(); ok {
		rule := css_ast.RSelector{Selectors: list}
		if p.expect(css_lexer.TOpenBrace) {
			rule.Rules = p.parseListOfDeclarations()
			p.expect(css_lexer.TCloseBrace)
			return &rule
		}
	}

	// Otherwise, parse a generic qualified rule
	return p.parseQualifiedRuleFrom(preludeStart)
}

func (p *parser) parseQualifiedRuleFrom(preludeStart int) *css_ast.RQualified {
	for !p.peek(css_lexer.TOpenBrace) && !p.peek(css_lexer.TEndOfFile) {
		p.parseComponentValue()
	}
	rule := css_ast.RQualified{
		Prelude: p.convertTokens(p.tokens[preludeStart:p.index]),
	}
	if p.expect(css_lexer.TOpenBrace) {
		rule.Rules = p.parseListOfDeclarations()
		p.expect(css_lexer.TCloseBrace)
	}
	return &rule
}

func (p *parser) parseDeclaration() css_ast.R {
	// Parse the key
	keyStart := p.index
	ok := false
	if p.expect(css_lexer.TIdent) {
		p.eat(css_lexer.TWhitespace)
		if p.expect(css_lexer.TColon) {
			ok = true
		}
	} else {
		p.advance()
	}

	// Parse the value
	valueStart := p.index
stop:
	for {
		switch p.current().Kind {
		case css_lexer.TEndOfFile, css_lexer.TSemicolon, css_lexer.TCloseBrace:
			break stop

		default:
			p.parseComponentValue()
		}
	}

	// Stop now if this is not a valid declaration
	if !ok {
		return &css_ast.RBadDeclaration{
			Tokens: p.convertTokens(p.tokens[keyStart:p.index]),
		}
	}

	// Remove leading and trailing whitespace from the value
	value := trimWhitespace(p.tokens[valueStart:p.index])

	// Remove trailing "!important"
	important := false
	if last := len(value) - 1; last >= 0 {
		if t := value[last]; t.Kind == css_lexer.TIdent && strings.EqualFold(t.Raw(p.source.Contents), "important") {
			i := len(value) - 2
			if i >= 0 && value[i].Kind == css_lexer.TWhitespace {
				i--
			}
			if i >= 0 && value[i].Kind == css_lexer.TDelimExclamation {
				if i >= 1 && value[i-1].Kind == css_lexer.TWhitespace {
					i--
				}
				value = value[:i]
				important = true
			}
		}
	}

	return &css_ast.RDeclaration{
		Key:       p.tokens[keyStart].Raw(p.source.Contents),
		Value:     p.convertTokensWithImports(value),
		Important: important,
	}
}

func (p *parser) parseComponentValue() {
	switch p.current().Kind {
	case css_lexer.TFunction:
		p.parseBlock(css_lexer.TFunction, css_lexer.TCloseParen)

	case css_lexer.TOpenParen:
		p.parseBlock(css_lexer.TOpenParen, css_lexer.TCloseParen)

	case css_lexer.TOpenBrace:
		p.parseBlock(css_lexer.TOpenBrace, css_lexer.TCloseBrace)

	case css_lexer.TOpenBracket:
		p.parseBlock(css_lexer.TOpenBracket, css_lexer.TCloseBracket)

	case css_lexer.TEndOfFile:
		p.unexpected()

	default:
		p.advance()
	}
}

func (p *parser) parseBlock(open css_lexer.T, close css_lexer.T) {
	if p.expect(open) {
		for !p.eat(close) {
			if p.peek(css_lexer.TEndOfFile) {
				p.expect(close)
				return
			}

			p.parseComponentValue()
		}
	}
}

func trimWhitespace(tokens []css_lexer.Token) []css_lexer.Token {
	if len(tokens) > 0 && tokens[0].Kind == css_lexer.TWhitespace {
		tokens = tokens[1:]
	}
	if i := len(tokens) - 1; i >= 0 && tokens[i].Kind == css_lexer.TWhitespace {
		tokens = tokens[:i]
	}
	return tokens
}
