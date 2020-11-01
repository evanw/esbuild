package js_parser

import (
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/logger"
)

func ParseGlobalName(log logger.Log, source logger.Source) (result []string, ok bool) {
	ok = true
	defer func() {
		r := recover()
		if _, isLexerPanic := r.(js_lexer.LexerPanic); isLexerPanic {
			ok = false
		} else if r != nil {
			panic(r)
		}
	}()

	lexer := js_lexer.NewLexerGlobalName(log, source)

	// Start off with an identifier
	result = append(result, lexer.Identifier)
	lexer.Expect(js_lexer.TIdentifier)

	// Follow with dot or index expressions
	for lexer.Token != js_lexer.TEndOfFile {
		switch lexer.Token {
		case js_lexer.TDot:
			lexer.Next()
			if !lexer.IsIdentifierOrKeyword() {
				lexer.Expect(js_lexer.TIdentifier)
			}
			result = append(result, lexer.Identifier)
			lexer.Next()

		case js_lexer.TOpenBracket:
			lexer.Next()
			result = append(result, js_lexer.UTF16ToString(lexer.StringLiteral))
			lexer.Expect(js_lexer.TStringLiteral)
			lexer.Expect(js_lexer.TCloseBracket)

		default:
			lexer.Expect(js_lexer.TDot)
		}
	}

	return
}
