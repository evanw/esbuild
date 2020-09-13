package css_parser

import (
	"fmt"
	"strings"

	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/logger"
)

type parser struct {
	log       logger.Log
	source    logger.Source
	tokens    []css_lexer.Token
	stack     []css_lexer.T
	index     int
	end       int
	prevError logger.Loc
}

func Parse(log logger.Log, source logger.Source) css_ast.AST {
	p := parser{
		log:       log,
		source:    source,
		tokens:    css_lexer.Tokenize(log, source),
		prevError: logger.Loc{Start: -1},
	}
	p.end = len(p.tokens)
	tree := css_ast.AST{}
	tree.Rules = p.parseListOfRules(ruleContext{
		isTopLevel:     true,
		parseSelectors: true,
	})
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
	var text string
	if p.peek(css_lexer.TEndOfFile) {
		text = fmt.Sprintf("Expected %s but found end of file", kind.String())
	} else {
		text = fmt.Sprintf("Expected %s but found %q", kind.String(), p.text())
	}
	r := p.current().Range
	if r.Loc.Start > p.prevError.Start {
		p.log.AddRangeError(&p.source, r, text)
		p.prevError = r.Loc
	}
	return false
}

func (p *parser) unexpected() {
	var text string
	if p.peek(css_lexer.TEndOfFile) {
		text = "Unexpected end of file"
	} else {
		text = fmt.Sprintf("Unexpected %q", p.text())
	}
	r := p.current().Range
	if r.Loc.Start > p.prevError.Start {
		p.log.AddRangeError(&p.source, r, text)
		p.prevError = r.Loc
	}
}

type ruleContext struct {
	isTopLevel     bool
	parseSelectors bool
}

func (p *parser) parseListOfRules(context ruleContext) []css_ast.R {
	rules := []css_ast.R{}
	for {
		kind := p.current().Kind
		switch {
		case kind == css_lexer.TEndOfFile || kind == css_lexer.TCloseBrace:
			return rules

		case kind == css_lexer.TWhitespace:
			p.advance()

		case kind == css_lexer.TAtKeyword:
			rules = append(rules, p.parseAtRule(atRuleContext{}))

		case (kind == css_lexer.TCDO || kind == css_lexer.TCDC) && context.isTopLevel:
			p.advance()

		default:
			if context.parseSelectors {
				rules = append(rules, p.parseSelectorRule())
			} else {
				rules = append(rules, p.parseQualifiedRuleFrom(p.index))
			}
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

type atRuleKind uint8

const (
	atRuleUnknown atRuleKind = iota
	atRuleQualifiedRules
	atRuleInheritContext
	atRuleEmpty
)

var specialAtRules = map[string]atRuleKind{
	"@keyframes": atRuleQualifiedRules,

	"@document": atRuleInheritContext,
	"@media":    atRuleInheritContext,
	"@scope":    atRuleInheritContext,
	"@supports": atRuleInheritContext,

	"@charset":   atRuleEmpty,
	"@import":    atRuleEmpty,
	"@namespace": atRuleEmpty,
}

type atRuleContext struct {
	isDeclarationList bool
}

func (p *parser) parseAtRule(context atRuleContext) css_ast.R {
	// Parse the name
	name := p.current()
	text := p.text()
	kind := specialAtRules[text]
	p.advance()

	// Parse the prelude
	preludeStart := p.index
	for !p.peek(css_lexer.TOpenBrace) {
		if p.peek(css_lexer.TSemicolon) || p.peek(css_lexer.TCloseBrace) {
			prelude := p.tokens[preludeStart:p.index]

			// Report an error for rules that should have blocks
			if kind != atRuleEmpty && kind != atRuleUnknown {
				p.expect(css_lexer.TOpenBrace)
				p.eat(css_lexer.TSemicolon)
				return &css_ast.RUnknownAt{Name: name, Prelude: prelude}
			}

			// Special-case certain rules
			if text == "@import" {
				tokens := trimWhitespace(prelude)
				if len(tokens) == 1 {
					t := tokens[0]
					switch t.Kind {
					case css_lexer.TString:
						path := css_lexer.ContentsOfStringToken(t.Raw(p.source.Contents))
						p.eat(css_lexer.TSemicolon)
						return &css_ast.RAtImport{PathText: path, PathRange: t.Range}

					case css_lexer.TURL:
						path := css_lexer.ContentsOfURLToken(t.Raw(p.source.Contents))
						p.eat(css_lexer.TSemicolon)
						return &css_ast.RAtImport{PathText: path, PathRange: t.Range}
					}
				}
			}

			p.eat(css_lexer.TSemicolon)
			return &css_ast.RKnownAt{Name: name, Prelude: prelude}
		}

		p.parseComponentValue()
	}
	prelude := p.tokens[preludeStart:p.index]
	blockStart := p.index

	// Report an error for rules that shouldn't have blocks
	if kind == atRuleEmpty {
		p.expect(css_lexer.TSemicolon)
		p.parseBlock(css_lexer.TCloseBrace)
		block := p.tokens[blockStart:p.index]
		return &css_ast.RUnknownAt{Name: name, Prelude: prelude, Block: block}
	}

	// Parse known rules whose blocks consist of qualified rules
	if kind == atRuleQualifiedRules {
		p.advance()
		rules := p.parseListOfRules(ruleContext{})
		p.expect(css_lexer.TCloseBrace)
		return &css_ast.RKnownAt{Name: name, Prelude: prelude, Rules: rules}
	}

	// Parse known rules whose blocks consist of whatever the current context is
	if kind == atRuleInheritContext {
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
		return &css_ast.RKnownAt{Name: name, Prelude: prelude, Rules: rules}
	}

	// Otherwise, parse an unknown rule
	p.parseBlock(css_lexer.TCloseBrace)
	block := p.tokens[blockStart:p.index]
	return &css_ast.RUnknownAt{Name: name, Prelude: prelude, Block: block}
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
	for !p.peek(css_lexer.TOpenBrace) {
		p.parseComponentValue()
	}
	rule := css_ast.RQualified{
		Prelude: p.tokens[preludeStart:p.index],
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
			Tokens: p.tokens[keyStart:p.index],
		}
	}

	// Remove leading and trailing whitespace from the value
	value := trimWhitespace(p.tokens[valueStart:p.index])

	// Remove trailing "!important"
	important := false
	if i := len(value) - 2; i >= 0 && value[i].Kind == css_lexer.TDelimExclamation {
		if t := value[i+1]; t.Kind == css_lexer.TIdent && strings.EqualFold(t.Raw(p.source.Contents), "important") {
			value = value[:i]
			important = true
		}
	}

	return &css_ast.RDeclaration{
		Key:       p.tokens[keyStart],
		Value:     value,
		Important: important,
	}
}

func (p *parser) parseComponentValue() {
	switch p.current().Kind {
	case css_lexer.TFunction:
		p.parseBlock(css_lexer.TCloseParen)

	case css_lexer.TOpenParen:
		p.parseBlock(css_lexer.TCloseParen)

	case css_lexer.TOpenBrace:
		p.parseBlock(css_lexer.TCloseBrace)

	case css_lexer.TOpenBracket:
		p.parseBlock(css_lexer.TCloseBracket)

	case css_lexer.TEndOfFile:
		p.unexpected()

	default:
		p.advance()
	}
}

func (p *parser) parseBlock(close css_lexer.T) {
	p.advance()

	for !p.eat(close) {
		if p.peek(css_lexer.TEndOfFile) {
			p.expect(close)
			return
		}

		p.parseComponentValue()
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
