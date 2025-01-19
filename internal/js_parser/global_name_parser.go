package js_parser

import (
	"github.com/evanw/esbuild/internal/helpers"
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

	// Start off with an identifier or a keyword that results in an object
	result = append(result, lexer.Identifier.String)
	switch lexer.Token {
	case js_lexer.TThis:
		lexer.Next()

	case js_lexer.TImport:
		// Handle "import.meta"
		lexer.Next()
		lexer.Expect(js_lexer.TDot)
		result = append(result, lexer.Identifier.String)
		lexer.ExpectContextualKeyword("meta")

	default:
		lexer.Expect(js_lexer.TIdentifier)
	}

	// Follow with dot or index expressions
	for lexer.Token != js_lexer.TEndOfFile {
		switch lexer.Token {
		case js_lexer.TDot:
			lexer.Next()
			if !lexer.IsIdentifierOrKeyword() {
				lexer.Expect(js_lexer.TIdentifier)
			}
			result = append(result, lexer.Identifier.String)
			lexer.Next()

		case js_lexer.TOpenBracket:
			lexer.Next()
			result = append(result, helpers.UTF16ToString(lexer.StringLiteral()))
			lexer.Expect(js_lexer.TStringLiteral)
			lexer.Expect(js_lexer.TCloseBracket)

		default:
			lexer.Expect(js_lexer.TDot)
		}
	}

	return
}
