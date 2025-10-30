package css_parser

import (
	"strings"

	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/logger"
)

// Reference: https://drafts.csswg.org/mediaqueries-4/
func (p *parser) parseMediaQueryListUntil(stop func(css_lexer.T) bool) []css_ast.MediaQuery {
	var queries []css_ast.MediaQuery
	p.eat(css_lexer.TWhitespace)
	for !p.peek(css_lexer.TEndOfFile) && !stop(p.current().Kind) {
		start := p.index
		query, ok := p.parseMediaQuery()
		if !ok {
			// If parsing failed, parse an arbitrary sequence of tokens instead
			p.index = start
			loc := p.current().Range.Loc
			for !p.peek(css_lexer.TEndOfFile) && !stop(p.current().Kind) && !p.peek(css_lexer.TComma) {
				p.parseComponentValue()
			}
			tokens := p.convertTokens(p.tokens[start:p.index])
			query = css_ast.MediaQuery{Loc: loc, Data: &css_ast.MQArbitraryTokens{Tokens: tokens}}
		}
		queries = append(queries, query)
		p.eat(css_lexer.TWhitespace)
		if !p.eat(css_lexer.TComma) {
			break
		}
		p.eat(css_lexer.TWhitespace)
	}
	return queries
}

func (p *parser) parseMediaQuery() (css_ast.MediaQuery, bool) {
	loc := p.current().Range.Loc

	// Check for a media condition first
	if p.looksLikeMediaCondition() {
		return p.parseMediaCondition(mediaWithOr)
	}

	// Parse the media type and potentially the leading "not" or "only" keyword
	mediaType := p.decoded()
	if !p.peek(css_lexer.TIdent) {
		p.expect(css_lexer.TIdent)
		return css_ast.MediaQuery{}, false
	}
	op := css_ast.MQTypeOpNone
	if strings.EqualFold(mediaType, "not") {
		op = css_ast.MQTypeOpNot
	} else if strings.EqualFold(mediaType, "only") {
		op = css_ast.MQTypeOpOnly
	}
	if op != css_ast.MQTypeOpNone {
		p.advance()
		p.eat(css_lexer.TWhitespace)
		mediaType = p.decoded()
		if !p.peek(css_lexer.TIdent) {
			p.expect(css_lexer.TIdent)
			return css_ast.MediaQuery{}, false
		}
	}

	// The <media-type> production does not include the keywords "only", "not", "and", "or", and "layer".
	if strings.EqualFold(mediaType, "only") ||
		strings.EqualFold(mediaType, "not") ||
		strings.EqualFold(mediaType, "and") ||
		strings.EqualFold(mediaType, "or") ||
		strings.EqualFold(mediaType, "layer") {
		p.unexpected()
		return css_ast.MediaQuery{}, false
	}
	p.advance()
	p.eat(css_lexer.TWhitespace)

	// Potentially parse a chain of "and" operators
	var andOrNull css_ast.MediaQuery
	if p.peek(css_lexer.TIdent) && strings.EqualFold(p.decoded(), "and") {
		p.advance()
		p.eat(css_lexer.TWhitespace)
		var ok bool
		andOrNull, ok = p.parseMediaCondition(mediaWithoutOr)
		if !ok {
			return css_ast.MediaQuery{}, false
		}
	}

	return css_ast.MediaQuery{Loc: loc, Data: &css_ast.MQType{Op: op, Type: mediaType, AndOrNull: andOrNull}}, true
}

func (p *parser) looksLikeMediaCondition() bool {
	kind := p.current().Kind
	return kind == css_lexer.TOpenParen || kind == css_lexer.TFunction ||
		(kind == css_lexer.TIdent && strings.EqualFold(p.decoded(), "not") &&
			p.next().Kind == css_lexer.TWhitespace &&
			p.at(p.index+2).Kind == css_lexer.TOpenParen)
}

type mediaOr uint8

const (
	mediaWithOr mediaOr = iota
	mediaWithoutOr
)

func (p *parser) parseMediaCondition(or mediaOr) (css_ast.MediaQuery, bool) {
	loc := p.current().Range.Loc

	// Handle a leading "not"
	if p.peek(css_lexer.TIdent) && strings.EqualFold(p.decoded(), "not") {
		p.advance()
		p.eat(css_lexer.TWhitespace)
		if inner, ok := p.parseMediaInParens(); !ok {
			return css_ast.MediaQuery{}, false
		} else {
			return p.maybeSimplifyMediaNot(loc, inner), true
		}
	}

	// Parse the first term
	first, ok := p.parseMediaInParens()
	if !ok {
		return css_ast.MediaQuery{}, false
	}
	p.eat(css_lexer.TWhitespace)

	// Potentially parse a chain of "and" or "or" operators
	if p.peek(css_lexer.TIdent) {
		if keyword := p.decoded(); strings.EqualFold(keyword, "and") || (or == mediaWithOr && strings.EqualFold(keyword, "or")) {
			op := css_ast.MQBinaryOpAnd
			if len(keyword) == 2 {
				op = css_ast.MQBinaryOpOr
			}
			inner := p.appendMediaTerm([]css_ast.MediaQuery{}, first, op)
			for {
				p.advance()
				p.eat(css_lexer.TWhitespace)
				next, ok := p.parseMediaInParens()
				if !ok {
					return css_ast.MediaQuery{}, false
				}
				inner = p.appendMediaTerm(inner, next, op)
				p.eat(css_lexer.TWhitespace)
				if !p.peek(css_lexer.TIdent) || !strings.EqualFold(p.decoded(), keyword) {
					break
				}
			}
			return css_ast.MediaQuery{Loc: loc, Data: &css_ast.MQBinary{Op: op, Terms: inner}}, true
		}
	}

	return first, true
}

func (p *parser) appendMediaTerm(inner []css_ast.MediaQuery, term css_ast.MediaQuery, op css_ast.MQBinaryOp) []css_ast.MediaQuery {
	// "(a and b) and c" => "a and b and c"
	// "(a or b) or c" => "a or b or c"
	if binary, ok := term.Data.(*css_ast.MQBinary); ok && binary.Op == op && p.options.minifySyntax {
		return append(inner, binary.Terms...)
	} else {
		return append(inner, term)
	}
}

func (p *parser) parseMediaInParens() (css_ast.MediaQuery, bool) {
	p.eat(css_lexer.TWhitespace)
	start := p.index

	// Consume the opening token
	isFunction := p.eat(css_lexer.TFunction)
	if !isFunction && !p.expect(css_lexer.TOpenParen) {
		return css_ast.MediaQuery{}, false
	}
	p.eat(css_lexer.TWhitespace)

	// Handle a media condition
	if !isFunction && p.looksLikeMediaCondition() {
		if inner, ok := p.parseMediaCondition(mediaWithOr); !ok {
			return css_ast.MediaQuery{}, false
		} else {
			p.eat(css_lexer.TWhitespace)
			if !p.expect(css_lexer.TCloseParen) {
				return css_ast.MediaQuery{}, false
			}
			return inner, ok
		}
	}

	// Scan over the remaining tokens
	for !p.peek(css_lexer.TCloseParen) && !p.peek(css_lexer.TEndOfFile) {
		p.parseComponentValue()
	}
	end := p.index
	if !p.expect(css_lexer.TCloseParen) {
		return css_ast.MediaQuery{}, false
	}
	tokens := p.convertTokens(p.tokens[start:end])
	loc := tokens[0].Loc

	// Potentially pattern-match the tokens inside the parentheses
	if !isFunction && len(tokens) == 1 {
		if children := tokens[0].Children; children != nil {
			if term, ok := parsePlainOrBooleanMediaFeature(*children); ok {
				return css_ast.MediaQuery{Loc: loc, Data: term}, true
			}
			if term, ok := parseRangeMediaFeature(*children); ok {
				if p.options.unsupportedCSSFeatures.Has(compat.MediaRange) {
					var terms []css_ast.MediaQuery
					if term.BeforeCmp != css_ast.MQCmpNone {
						terms = append(terms, lowerMediaRange(term.NameLoc, term.Name, term.BeforeCmp.Reverse(), term.Before))
					}
					if term.AfterCmp != css_ast.MQCmpNone {
						terms = append(terms, lowerMediaRange(term.NameLoc, term.Name, term.AfterCmp, term.After))
					}
					if len(terms) == 1 {
						return terms[0], true
					} else {
						return css_ast.MediaQuery{Loc: loc, Data: &css_ast.MQBinary{Op: css_ast.MQBinaryOpAnd, Terms: terms}}, true
					}
				}
				return css_ast.MediaQuery{Loc: loc, Data: term}, true
			}
		}
	}
	return css_ast.MediaQuery{Loc: loc, Data: &css_ast.MQArbitraryTokens{Tokens: tokens}}, true
}

func lowerMediaRange(loc logger.Loc, name string, cmp css_ast.MQCmp, value []css_ast.Token) css_ast.MediaQuery {
	switch cmp {
	case css_ast.MQCmpLe:
		// "foo <= 123" => "max-foo: 123"
		return css_ast.MediaQuery{Loc: loc, Data: &css_ast.MQPlainOrBoolean{Name: "max-" + name, ValueOrNil: value}}

	case css_ast.MQCmpGe:
		// "foo >= 123" => "min-foo: 123"
		return css_ast.MediaQuery{Loc: loc, Data: &css_ast.MQPlainOrBoolean{Name: "min-" + name, ValueOrNil: value}}

	case css_ast.MQCmpLt:
		// "foo < 123" => "not (min-foo: 123)"
		return css_ast.MediaQuery{Loc: loc, Data: &css_ast.MQNot{
			Inner: css_ast.MediaQuery{Loc: loc, Data: &css_ast.MQPlainOrBoolean{Name: "min-" + name, ValueOrNil: value}},
		}}

	case css_ast.MQCmpGt:
		// "foo > 123" => "not (max-foo: 123)"
		return css_ast.MediaQuery{Loc: loc, Data: &css_ast.MQNot{
			Inner: css_ast.MediaQuery{Loc: loc, Data: &css_ast.MQPlainOrBoolean{Name: "max-" + name, ValueOrNil: value}},
		}}

	default:
		// "foo = 123" => "foo: 123"
		return css_ast.MediaQuery{Loc: loc, Data: &css_ast.MQPlainOrBoolean{Name: name, ValueOrNil: value}}
	}
}

func parsePlainOrBooleanMediaFeature(tokens []css_ast.Token) (*css_ast.MQPlainOrBoolean, bool) {
	if len(tokens) == 1 && tokens[0].Kind == css_lexer.TIdent {
		return &css_ast.MQPlainOrBoolean{Name: tokens[0].Text}, true
	}
	if len(tokens) >= 3 && tokens[0].Kind == css_lexer.TIdent && tokens[1].Kind == css_lexer.TColon {
		if value, rest := scanMediaValue(tokens[2:]); len(rest) == 0 {
			return &css_ast.MQPlainOrBoolean{Name: tokens[0].Text, ValueOrNil: value}, true
		}
	}
	return nil, false
}

func parseRangeMediaFeature(tokens []css_ast.Token) (*css_ast.MQRange, bool) {
	if first, tokens := scanMediaValue(tokens); len(first) > 0 {
		if firstCmp, tokens := scanMediaComparison(tokens); firstCmp != css_ast.MQCmpNone {
			if second, tokens := scanMediaValue(tokens); len(second) > 0 {
				if len(tokens) == 0 {
					if name, nameLoc, ok := isSingleIdent(first); ok {
						return &css_ast.MQRange{
							Name:     name,
							NameLoc:  nameLoc,
							AfterCmp: firstCmp,
							After:    second,
						}, true
					} else if name, nameLoc, ok := isSingleIdent(second); ok {
						return &css_ast.MQRange{
							Before:    first,
							BeforeCmp: firstCmp,
							Name:      name,
							NameLoc:   nameLoc,
						}, true
					}
				} else if name, nameLoc, ok := isSingleIdent(second); ok {
					if secondCmp, tokens := scanMediaComparison(tokens); secondCmp != css_ast.MQCmpNone {
						if f, s := firstCmp.Dir(), secondCmp.Dir(); (f < 0 && s < 0) || (f > 0 && s > 0) {
							if third, tokens := scanMediaValue(tokens); len(third) > 0 && len(tokens) == 0 {
								return &css_ast.MQRange{
									Before:    first,
									BeforeCmp: firstCmp,
									Name:      name,
									NameLoc:   nameLoc,
									AfterCmp:  secondCmp,
									After:     third,
								}, true
							}
						}
					}
				}
			}
		}
	}
	return nil, false
}

func (p *parser) maybeSimplifyMediaNot(loc logger.Loc, inner css_ast.MediaQuery) css_ast.MediaQuery {
	if p.options.minifySyntax {
		switch data := inner.Data.(type) {
		case *css_ast.MQNot:
			// "not (not a)" => "a"
			// "not (not (not a))" => "not a"
			return data.Inner

		case *css_ast.MQBinary:
			// "not ((not a) and (not b))" => "a or b"
			// "not ((not a) or (not b))" => "a and b"
			terms := make([]css_ast.MediaQuery, 0, len(data.Terms))
			for _, term := range data.Terms {
				if not, ok := term.Data.(*css_ast.MQNot); ok {
					terms = append(terms, not.Inner)
				} else {
					break
				}
			}
			if len(terms) == len(data.Terms) {
				data.Op ^= 1
				data.Terms = terms
				return inner
			}

		case *css_ast.MQRange:
			if (data.BeforeCmp == css_ast.MQCmpNone && data.AfterCmp != css_ast.MQCmpEq) ||
				(data.AfterCmp == css_ast.MQCmpNone && data.BeforeCmp != css_ast.MQCmpEq) {
				data.BeforeCmp = data.BeforeCmp.Flip()
				data.AfterCmp = data.AfterCmp.Flip()
				return inner
			}
		}
	}
	return css_ast.MediaQuery{Loc: loc, Data: &css_ast.MQNot{Inner: inner}}
}

func isSingleIdent(tokens []css_ast.Token) (string, logger.Loc, bool) {
	if len(tokens) == 1 && tokens[0].Kind == css_lexer.TIdent {
		return tokens[0].Text, tokens[0].Loc, true
	} else {
		return "", logger.Loc{}, false
	}
}

func scanMediaComparison(tokens []css_ast.Token) (css_ast.MQCmp, []css_ast.Token) {
	if len(tokens) >= 1 {
		switch tokens[0].Kind {
		case css_lexer.TDelimEquals:
			return css_ast.MQCmpEq, tokens[1:]

		case css_lexer.TDelimLessThan:
			// Handle "<=" or "<"
			if len(tokens) >= 2 && tokens[1].Kind == css_lexer.TDelimEquals &&
				((tokens[0].Whitespace&css_ast.WhitespaceAfter)|(tokens[1].Whitespace&css_ast.WhitespaceBefore)) == 0 {
				return css_ast.MQCmpLe, tokens[2:]
			}
			return css_ast.MQCmpLt, tokens[1:]

		case css_lexer.TDelimGreaterThan:
			// Handle ">=" or ">"
			if len(tokens) >= 2 && tokens[1].Kind == css_lexer.TDelimEquals &&
				((tokens[0].Whitespace&css_ast.WhitespaceAfter)|(tokens[1].Whitespace&css_ast.WhitespaceBefore)) == 0 {
				return css_ast.MQCmpGe, tokens[2:]
			}
			return css_ast.MQCmpGt, tokens[1:]
		}
	}

	return css_ast.MQCmpNone, tokens
}

func scanMediaValue(tokens []css_ast.Token) ([]css_ast.Token, []css_ast.Token) {
	n := 0

	if len(tokens) >= 1 {
		switch tokens[0].Kind {
		case css_lexer.TDimension, css_lexer.TIdent:
			n = 1

		case css_lexer.TNumber:
			// Potentially recognize a ratio which is "<number> / <number>"
			if len(tokens) >= 3 && tokens[1].Kind == css_lexer.TDelimSlash && tokens[2].Kind == css_lexer.TNumber {
				n = 3
			} else {
				n = 1
			}
		}
	}

	// Trim whitespace at the endpoints
	if n > 0 {
		tokens[0].Whitespace &= ^css_ast.WhitespaceBefore
		tokens[n-1].Whitespace &= ^css_ast.WhitespaceAfter
	}

	return tokens[:n], tokens[n:]
}
