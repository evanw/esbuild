package parser

import (
	"fmt"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/lexer"
	"github.com/evanw/esbuild/internal/logging"
)

type jsonParser struct {
	log                 logging.Log
	source              logging.Source
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
		return ast.Expr{loc, &ast.EBoolean{false}}

	case lexer.TTrue:
		p.lexer.Next()
		return ast.Expr{loc, &ast.EBoolean{true}}

	case lexer.TNull:
		p.lexer.Next()
		return ast.Expr{loc, &ast.ENull{}}

	case lexer.TStringLiteral:
		value := p.lexer.StringLiteral
		p.lexer.Next()
		return ast.Expr{loc, &ast.EString{value}}

	case lexer.TNumericLiteral:
		value := p.lexer.Number
		p.lexer.Next()
		return ast.Expr{loc, &ast.ENumber{value}}

	case lexer.TMinus:
		p.lexer.Next()
		value := p.lexer.Number
		p.lexer.Expect(lexer.TNumericLiteral)
		return ast.Expr{loc, &ast.ENumber{-value}}

	case lexer.TOpenBracket:
		lineCountAtStart := p.lexer.ApproximateLineCount
		p.lexer.Next()
		items := []ast.Expr{}

		for p.lexer.Token != lexer.TCloseBracket {
			if len(items) > 0 && !p.parseMaybeTrailingComma(lexer.TCloseBracket) {
				break
			}

			item := p.parseExpr()
			items = append(items, item)
		}

		isSingleLine := p.lexer.ApproximateLineCount == lineCountAtStart
		p.lexer.Expect(lexer.TCloseBracket)
		return ast.Expr{loc, &ast.EArray{
			Items:        items,
			IsSingleLine: isSingleLine,
		}}

	case lexer.TOpenBrace:
		lineCountAtStart := p.lexer.ApproximateLineCount
		p.lexer.Next()
		properties := []ast.Property{}
		duplicates := make(map[string]bool)

		for p.lexer.Token != lexer.TCloseBrace {
			if len(properties) > 0 && !p.parseMaybeTrailingComma(lexer.TCloseBrace) {
				break
			}

			keyString := p.lexer.StringLiteral
			keyRange := p.lexer.Range()
			key := ast.Expr{keyRange.Loc, &ast.EString{keyString}}
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

		isSingleLine := p.lexer.ApproximateLineCount == lineCountAtStart
		p.lexer.Expect(lexer.TCloseBrace)
		return ast.Expr{loc, &ast.EObject{
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

func ParseJSON(log logging.Log, source logging.Source, options ParseJSONOptions) (result ast.Expr, ok bool) {
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
