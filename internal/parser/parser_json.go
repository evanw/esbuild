package parser

import (
	"fmt"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/lexer"
	"github.com/evanw/esbuild/internal/logger"
)

type jsonParser struct {
	log                 logger.Log
	source              logger.Source
	lexer               lexer.Lexer
	allowTrailingCommas bool
}

func (p *jsonParser) parseMaybeTrailingComma(closeToken lexer.T) bool {
	commaRange := p.lexer.Range()
	p.lexer.Expect(lexer.TComma)

	if p.lexer.Token == closeToken {
		if !p.allowTrailingCommas {
			p.log.AddRangeError(&p.source, commaRange, "JSON does not support trailing commas")
		}
		return false
	}

	return true
}

func (p *jsonParser) parseExpr() ast.Expr {
	loc := p.lexer.Loc()

	switch p.lexer.Token {
	case lexer.TFalse:
		p.lexer.Next()
		return ast.Expr{Loc: loc, Data: &ast.EBoolean{Value: false}}

	case lexer.TTrue:
		p.lexer.Next()
		return ast.Expr{Loc: loc, Data: &ast.EBoolean{Value: true}}

	case lexer.TNull:
		p.lexer.Next()
		return ast.Expr{Loc: loc, Data: &ast.ENull{}}

	case lexer.TStringLiteral:
		value := p.lexer.StringLiteral
		p.lexer.Next()
		return ast.Expr{Loc: loc, Data: &ast.EString{Value: value}}

	case lexer.TNumericLiteral:
		value := p.lexer.Number
		p.lexer.Next()
		return ast.Expr{Loc: loc, Data: &ast.ENumber{Value: value}}

	case lexer.TMinus:
		p.lexer.Next()
		value := p.lexer.Number
		p.lexer.Expect(lexer.TNumericLiteral)
		return ast.Expr{Loc: loc, Data: &ast.ENumber{Value: -value}}

	case lexer.TOpenBracket:
		p.lexer.Next()
		isSingleLine := !p.lexer.HasNewlineBefore
		items := []ast.Expr{}

		for p.lexer.Token != lexer.TCloseBracket {
			if len(items) > 0 {
				if p.lexer.HasNewlineBefore {
					isSingleLine = false
				}
				if !p.parseMaybeTrailingComma(lexer.TCloseBracket) {
					break
				}
				if p.lexer.HasNewlineBefore {
					isSingleLine = false
				}
			}

			item := p.parseExpr()
			items = append(items, item)
		}

		if p.lexer.HasNewlineBefore {
			isSingleLine = false
		}
		p.lexer.Expect(lexer.TCloseBracket)
		return ast.Expr{Loc: loc, Data: &ast.EArray{
			Items:        items,
			IsSingleLine: isSingleLine,
		}}

	case lexer.TOpenBrace:
		p.lexer.Next()
		isSingleLine := !p.lexer.HasNewlineBefore
		properties := []ast.Property{}
		duplicates := make(map[string]bool)

		for p.lexer.Token != lexer.TCloseBrace {
			if len(properties) > 0 {
				if p.lexer.HasNewlineBefore {
					isSingleLine = false
				}
				if !p.parseMaybeTrailingComma(lexer.TCloseBrace) {
					break
				}
				if p.lexer.HasNewlineBefore {
					isSingleLine = false
				}
			}

			keyString := p.lexer.StringLiteral
			keyRange := p.lexer.Range()
			key := ast.Expr{Loc: keyRange.Loc, Data: &ast.EString{Value: keyString}}
			p.lexer.Expect(lexer.TStringLiteral)

			// Warn about duplicate keys
			keyText := lexer.UTF16ToString(keyString)
			if duplicates[keyText] {
				p.log.AddRangeWarning(&p.source, keyRange, fmt.Sprintf("Duplicate key: %q", keyText))
			} else {
				duplicates[keyText] = true
			}

			p.lexer.Expect(lexer.TColon)
			value := p.parseExpr()

			property := ast.Property{
				Kind:  ast.PropertyNormal,
				Key:   key,
				Value: &value,
			}
			properties = append(properties, property)
		}

		if p.lexer.HasNewlineBefore {
			isSingleLine = false
		}
		p.lexer.Expect(lexer.TCloseBrace)
		return ast.Expr{Loc: loc, Data: &ast.EObject{
			Properties:   properties,
			IsSingleLine: isSingleLine,
		}}

	default:
		p.lexer.Unexpected()
		return ast.Expr{}
	}
}

type ParseJSONOptions struct {
	AllowComments       bool
	AllowTrailingCommas bool
}

func ParseJSON(log logger.Log, source logger.Source, options ParseJSONOptions) (result ast.Expr, ok bool) {
	ok = true
	defer func() {
		r := recover()
		if _, isLexerPanic := r.(lexer.LexerPanic); isLexerPanic {
			ok = false
		} else if r != nil {
			panic(r)
		}
	}()

	p := &jsonParser{
		log:                 log,
		source:              source,
		lexer:               lexer.NewLexerJSON(log, source, options.AllowComments),
		allowTrailingCommas: options.AllowTrailingCommas,
	}

	result = p.parseExpr()
	p.lexer.Expect(lexer.TEndOfFile)
	return
}
