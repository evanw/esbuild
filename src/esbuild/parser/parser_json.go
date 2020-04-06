package parser

import (
	"esbuild/ast"
	"esbuild/lexer"
	"esbuild/logging"
)

type jsonParser struct {
	log    logging.Log
	source logging.Source
	lexer  lexer.Lexer
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

	case lexer.TOpenBracket:
		p.lexer.Next()
		items := []ast.Expr{}

		for p.lexer.Token != lexer.TCloseBracket {
			item := p.parseExpr()
			items = append(items, item)

			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
		}

		p.lexer.Expect(lexer.TCloseBracket)
		return ast.Expr{loc, &ast.EArray{items}}

	case lexer.TOpenBrace:
		p.lexer.Next()
		properties := []ast.Property{}

		for p.lexer.Token != lexer.TCloseBrace {
			key := ast.Expr{p.lexer.Loc(), &ast.EString{p.lexer.StringLiteral}}
			p.lexer.Expect(lexer.TStringLiteral)
			p.lexer.Expect(lexer.TColon)
			value := p.parseExpr()

			property := ast.Property{
				Kind:  ast.PropertyNormal,
				Key:   key,
				Value: &value,
			}
			properties = append(properties, property)

			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
		}

		p.lexer.Expect(lexer.TCloseBrace)
		return ast.Expr{loc, &ast.EObject{properties}}

	default:
		p.lexer.Unexpected()
		return ast.Expr{}
	}
}

func ParseJson(log logging.Log, source logging.Source) (result ast.Expr, ok bool) {
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
		log:    log,
		source: source,
		lexer:  lexer.NewLexer(log, source),
	}

	result = p.parseExpr()
	return
}
