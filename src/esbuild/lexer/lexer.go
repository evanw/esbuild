package lexer

// The lexer converts a source file to a stream of tokens. Unlike many
// compilers, esbuild does not run the lexer to completion before the parser is
// started. Instead, the lexer is called repeatedly by the parser as the parser
// parses the file. This is because many tokens are context-sensitive and need
// high-level information from the parser. Examples are regular expression
// literals and JSX elements.
//
// For efficiency, the text associated with textual tokens is stored in two
// separate ways depending on the token. Identifiers use UTF-8 encoding which
// allows them to be slices of the input file without allocating extra memory.
// Strings use UTF-16 encoding so they can represent unicode surrogates
// accurately.

import (
	"esbuild/ast"
	"esbuild/logging"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

type T uint

// If you add a new token, remember to add it to "tokenToString" too
const (
	TEndOfFile T = iota
	TSyntaxError

	// "#!/usr/bin/env node"
	THashbang

	// Literals
	TNoSubstitutionTemplateLiteral // Contents are in lexer.StringLiteral ([]uint16)
	TNumericLiteral                // Contents are in lexer.Number (float64)
	TStringLiteral                 // Contents are in lexer.StringLiteral ([]uint16)
	TBigIntegerLiteral             // Contents are in lexer.Identifier (string)

	// Pseudo-literals
	TTemplateHead   // Contents are in lexer.StringLiteral ([]uint16)
	TTemplateMiddle // Contents are in lexer.StringLiteral ([]uint16)
	TTemplateTail   // Contents are in lexer.StringLiteral ([]uint16)

	// Punctuation
	TAmpersand
	TAmpersandAmpersand
	TAsterisk
	TAsteriskAsterisk
	TAt
	TBar
	TBarBar
	TCaret
	TCloseBrace
	TCloseBracket
	TCloseParen
	TColon
	TComma
	TDot
	TDotDotDot
	TEqualsEquals
	TEqualsEqualsEquals
	TEqualsGreaterThan
	TExclamation
	TExclamationEquals
	TExclamationEqualsEquals
	TGreaterThan
	TGreaterThanEquals
	TGreaterThanGreaterThan
	TGreaterThanGreaterThanGreaterThan
	TLessThan
	TLessThanEquals
	TLessThanLessThan
	TMinus
	TMinusMinus
	TOpenBrace
	TOpenBracket
	TOpenParen
	TPercent
	TPlus
	TPlusPlus
	TQuestion
	TQuestionDot
	TQuestionQuestion
	TSemicolon
	TSlash
	TTilde

	// Assignments
	TAmpersandEquals
	TAsteriskAsteriskEquals
	TAsteriskEquals
	TBarEquals
	TCaretEquals
	TEquals
	TGreaterThanGreaterThanEquals
	TGreaterThanGreaterThanGreaterThanEquals
	TLessThanLessThanEquals
	TMinusEquals
	TPercentEquals
	TPlusEquals
	TSlashEquals

	// Identifiers
	TIdentifier     // Contents are in lexer.Identifier (string)
	TEscapedKeyword // A keyword that has been escaped as an identifer

	// Reserved words
	TBreak
	TCase
	TCatch
	TClass
	TConst
	TContinue
	TDebugger
	TDefault
	TDelete
	TDo
	TElse
	TEnum
	TExport
	TExtends
	TFalse
	TFinally
	TFor
	TFunction
	TIf
	TImport
	TIn
	TInstanceof
	TNew
	TNull
	TReturn
	TSuper
	TSwitch
	TThis
	TThrow
	TTrue
	TTry
	TTypeof
	TVar
	TVoid
	TWhile
	TWith

	// Strict mode reserved words
	TImplements
	TInterface
	TLet
	TPackage
	TPrivate
	TProtected
	TPublic
	TStatic
	TYield
)

var keywords = map[string]T{
	// Reserved words
	"break":      TBreak,
	"case":       TCase,
	"catch":      TCatch,
	"class":      TClass,
	"const":      TConst,
	"continue":   TContinue,
	"debugger":   TDebugger,
	"default":    TDefault,
	"delete":     TDelete,
	"do":         TDo,
	"else":       TElse,
	"enum":       TEnum,
	"export":     TExport,
	"extends":    TExtends,
	"false":      TFalse,
	"finally":    TFinally,
	"for":        TFor,
	"function":   TFunction,
	"if":         TIf,
	"import":     TImport,
	"in":         TIn,
	"instanceof": TInstanceof,
	"new":        TNew,
	"null":       TNull,
	"return":     TReturn,
	"super":      TSuper,
	"switch":     TSwitch,
	"this":       TThis,
	"throw":      TThrow,
	"true":       TTrue,
	"try":        TTry,
	"typeof":     TTypeof,
	"var":        TVar,
	"void":       TVoid,
	"while":      TWhile,
	"with":       TWith,

	// Strict mode reserved words
	"implements": TImplements,
	"interface":  TInterface,
	"let":        TLet,
	"package":    TPackage,
	"private":    TPrivate,
	"protected":  TProtected,
	"public":     TPublic,
	"static":     TStatic,
	"yield":      TYield,
}

func Keywords() map[string]T {
	result := make(map[string]T)
	for k, v := range keywords {
		result[k] = v
	}
	return result
}

type Lexer struct {
	log                             logging.Log
	source                          logging.Source
	current                         int
	start                           int
	end                             int
	Token                           T
	HasNewlineBefore                bool
	codePoint                       rune
	StringLiteral                   []uint16
	Identifier                      string
	Number                          float64
	rescanCloseBraceAsTemplateToken bool
}

type LexerPanic struct{}

func NewLexer(log logging.Log, source logging.Source) Lexer {
	lexer := Lexer{
		log:    log,
		source: source,
	}
	lexer.step()
	lexer.Next()
	return lexer
}

func (lexer *Lexer) Loc() ast.Loc {
	return ast.Loc{int32(lexer.start)}
}

func (lexer *Lexer) Range() ast.Range {
	return ast.Range{ast.Loc{int32(lexer.start)}, int32(lexer.end - lexer.start)}
}

func (lexer *Lexer) Raw() string {
	return lexer.source.Contents[lexer.start:lexer.end]
}

func (lexer *Lexer) RawTemplateContents() string {
	switch lexer.Token {
	case TNoSubstitutionTemplateLiteral, TTemplateTail:
		// "`x`" or "}x`"
		return lexer.source.Contents[lexer.start+1 : lexer.end-1]

	case TTemplateHead, TTemplateMiddle:
		// "`x${" or "}x${"
		return lexer.source.Contents[lexer.start+1 : lexer.end-2]

	default:
		return ""
	}
}

func (lexer *Lexer) IsIdentifierOrKeyword() bool {
	return lexer.Token >= TIdentifier
}

func (lexer *Lexer) IsContextualKeyword(text string) bool {
	return lexer.Token == TIdentifier && lexer.Raw() == text
}

func (lexer *Lexer) ExpectContextualKeyword(text string) {
	if !lexer.IsContextualKeyword(text) {
		lexer.addRangeError(lexer.Range(), fmt.Sprintf("Expected %q but found %q", text, lexer.Raw()))
		panic(LexerPanic{})
	}
	lexer.Next()
}

func (lexer *Lexer) SyntaxError() {
	loc := ast.Loc{int32(lexer.end)}
	message := "Unexpected end of file"
	if lexer.end < len(lexer.source.Contents) {
		c, _ := utf8.DecodeRuneInString(lexer.source.Contents[lexer.end:])
		if c < 0x20 {
			message = fmt.Sprintf("Syntax error \"\\x%02X\"", c)
		} else if c >= 0x80 {
			message = fmt.Sprintf("Syntax error \"\\u{%x}\"", c)
		} else {
			message = fmt.Sprintf("Syntax error \"%c\"", c)
		}
	}
	lexer.addError(loc, message)
	panic(LexerPanic{})
}

func (lexer *Lexer) ExpectedString(text string) {
	found := fmt.Sprintf("%q", lexer.Raw())
	if lexer.start == len(lexer.source.Contents) {
		found = "end of file"
	}
	lexer.addRangeError(lexer.Range(), fmt.Sprintf("Expected %s but found %s", text, found))
	panic(LexerPanic{})
}

func (lexer *Lexer) Expected(token T) {
	if text, ok := tokenToString[token]; ok {
		lexer.ExpectedString(text)
	} else {
		lexer.Unexpected()
	}
}

func (lexer *Lexer) Unexpected() {
	lexer.addRangeError(lexer.Range(), fmt.Sprintf("Unexpected %q", lexer.Raw()))
	panic(LexerPanic{})
}

func (lexer *Lexer) Expect(token T) {
	if lexer.Token != token {
		lexer.Expected(token)
	}
	lexer.Next()
}

func (lexer *Lexer) ExpectOrInsertSemicolon() {
	if lexer.Token == TSemicolon || (!lexer.HasNewlineBefore &&
		lexer.Token != TCloseBrace && lexer.Token != TEndOfFile) {
		lexer.Expect(TSemicolon)
	}
}

func (lexer *Lexer) ExpectGreaterThan() {
	switch lexer.Token {
	case TGreaterThan:
		lexer.Next()

	default:
		lexer.Expected(TGreaterThan)
	}
}

func NumberToMinifiedName(i int) string {
	j := i % 54
	name := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_$"[j : j+1]
	i = i / 54

	for i > 0 {
		i--
		j := i % 64
		name += "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_$0123456789"[j : j+1]
		i = i / 64
	}

	return name
}

func IsIdentifier(text string) bool {
	if len(text) == 0 {
		return false
	}
	for i, codePoint := range text {
		if i == 0 {
			if !IsIdentifierStart(codePoint) {
				return false
			}
		} else {
			if !IsIdentifierContinue(codePoint) {
				return false
			}
		}
	}
	return true
}

func IsIdentifierStart(codePoint rune) bool {
	switch codePoint {
	case '_', '$',
		'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm',
		'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
		'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M',
		'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z':
		return true
	}

	// All ASCII identifier start code points are listed above
	if codePoint < 0x7F {
		return false
	}

	return unicode.Is(idStart, codePoint)
}

func IsIdentifierContinue(codePoint rune) bool {
	switch codePoint {
	case '_', '$', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
		'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm',
		'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
		'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M',
		'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z':
		return true
	}

	// All ASCII identifier start code points are listed above
	if codePoint < 0x7F {
		return false
	}

	// ZWNJ and ZWJ are allowed in identifiers
	if codePoint == 0x200C || codePoint == 0x200D {
		return true
	}

	return unicode.Is(idContinue, codePoint)
}

func RangeOfIdentifier(source logging.Source, loc ast.Loc) ast.Range {
	text := source.Contents[loc.Start:]
	if len(text) == 0 {
		return ast.Range{loc, 0}
	}

	i := 0
	c, width := utf8.DecodeRuneInString(text)
	i += width

	if IsIdentifierStart(c) {
		// Search for the end of the identifier
		for i < len(text) {
			c2, width2 := utf8.DecodeRuneInString(text[i:])
			if !IsIdentifierContinue(c2) {
				return ast.Range{loc, int32(i)}
			}
			i += width2
		}
	}

	return ast.Range{loc, 0}
}

func (lexer *Lexer) ExpectJSXElementChild(token T) {
	if lexer.Token != token {
		lexer.Expected(token)
	}
	lexer.NextJSXElementChild()
}

func (lexer *Lexer) NextJSXElementChild() {
	lexer.HasNewlineBefore = false
	originalStart := lexer.end

	for {
		lexer.start = lexer.end
		lexer.Token = 0

		switch lexer.codePoint {
		case -1: // This indicates the end of the file
			lexer.Token = TEndOfFile

		case '\r', '\n', '\u2028', '\u2029':
			lexer.step()
			lexer.HasNewlineBefore = true
			continue

		case '\t', '\f', '\v', ' ', '\xA0', '\uFEFF':
			lexer.step()
			continue

		case '{':
			lexer.step()
			lexer.Token = TOpenBrace

		case '<':
			lexer.step()
			lexer.Token = TLessThan

		default:
			// This needs fixing if we skipped over whitespace characters earlier
			needsFixing := lexer.start != originalStart

		stringLiteral:
			for {
				switch lexer.codePoint {
				case -1:
					// Reaching the end of the file without a closing element is an error
					lexer.SyntaxError()

				case '&', '\r', '\n', '\u2028', '\u2029':
					// This needs fixing if it has an entity or if it's a multi-line string
					needsFixing = true
					lexer.step()

				case '{', '<':
					// Stop when the string ends
					break stringLiteral

				default:
					// Non-ASCII strings need the slow path
					if lexer.codePoint >= 0x80 {
						needsFixing = true
					}
					lexer.step()
				}
			}

			lexer.Token = TStringLiteral
			text := lexer.source.Contents[originalStart:lexer.end]

			if needsFixing {
				// Slow path
				lexer.StringLiteral = fixWhitespaceAndDecodeJSXEntities(text)
			} else {
				// Fast path
				n := len(text)
				copy := make([]uint16, n)
				for i := 0; i < n; i++ {
					copy[i] = uint16(text[i])
				}
				lexer.StringLiteral = copy
			}
		}

		break
	}
}

func (lexer *Lexer) ExpectInsideJSXElement(token T) {
	if lexer.Token != token {
		lexer.Expected(token)
	}
	lexer.NextInsideJSXElement()
}

func (lexer *Lexer) NextInsideJSXElement() {
	lexer.HasNewlineBefore = false

	for {
		lexer.start = lexer.end
		lexer.Token = 0

		switch lexer.codePoint {
		case -1: // This indicates the end of the file
			lexer.Token = TEndOfFile

		case '\r', '\n', '\u2028', '\u2029':
			lexer.step()
			lexer.HasNewlineBefore = true
			continue

		case '\t', '\f', '\v', ' ', '\xA0', '\uFEFF':
			lexer.step()
			continue

		case '.':
			lexer.step()
			lexer.Token = TDot

		case '=':
			lexer.step()
			lexer.Token = TEquals

		case '{':
			lexer.step()
			lexer.Token = TOpenBrace

		case '}':
			lexer.step()
			lexer.Token = TCloseBrace

		case '<':
			lexer.step()
			lexer.Token = TLessThan

		case '>':
			lexer.step()
			lexer.Token = TGreaterThan

		case '/':
			// '/' or '//' or '/* ... */'
			lexer.step()
			switch lexer.codePoint {
			case '/':
			singleLineComment:
				for {
					lexer.step()
					switch lexer.codePoint {
					case '\r', '\n', '\u2028', '\u2029':
						break singleLineComment

					case -1: // This indicates the end of the file
						break singleLineComment
					}
				}
				continue

			case '*':
				lexer.step()
			multiLineComment:
				for {
					switch lexer.codePoint {
					case '*':
						lexer.step()
						if lexer.codePoint == '/' {
							lexer.step()
							break multiLineComment
						}

					case '\r', '\n', '\u2028', '\u2029':
						lexer.step()
						lexer.HasNewlineBefore = true

					case -1: // This indicates the end of the file
						lexer.start = lexer.end
						lexer.addError(lexer.Loc(), "Expected \"*/\" to terminate multi-line comment")
						lexer.Token = TSyntaxError
						break multiLineComment

					default:
						lexer.step()
					}
				}
				continue

			default:
				lexer.Token = TSlash
			}

		case '\'', '"':
			quote := lexer.codePoint
			needsDecode := false
			lexer.step()

		stringLiteral:
			for {
				switch lexer.codePoint {
				case -1: // This indicates the end of the file
					lexer.SyntaxError()

				case '&':
					needsDecode = true
					lexer.step()

				case quote:
					lexer.step()
					break stringLiteral

				default:
					// Non-ASCII strings need the slow path
					if lexer.codePoint >= 0x80 {
						needsDecode = true
					}
					lexer.step()
				}
			}

			lexer.Token = TStringLiteral
			text := lexer.source.Contents[lexer.start+1 : lexer.end-1]

			if needsDecode {
				// Slow path
				lexer.StringLiteral = decodeJSXEntities([]uint16{}, text)
			} else {
				// Fast path
				n := len(text)
				copy := make([]uint16, n)
				for i := 0; i < n; i++ {
					copy[i] = uint16(text[i])
				}
				lexer.StringLiteral = copy
			}

		default:
			if IsIdentifierStart(lexer.codePoint) {
				lexer.step()
				for IsIdentifierContinue(lexer.codePoint) || lexer.codePoint == '-' {
					lexer.step()
				}
				lexer.Identifier = lexer.Raw()
				lexer.Token = TIdentifier
				break
			}

			lexer.end = lexer.current
			lexer.Token = TSyntaxError
		}

		return
	}
}

func (lexer *Lexer) Next() {
	lexer.HasNewlineBefore = false

	for {
		lexer.start = lexer.end
		lexer.Token = 0

		switch lexer.codePoint {
		case -1: // This indicates the end of the file
			lexer.Token = TEndOfFile

		case '#':
			if lexer.start == 0 && strings.HasPrefix(lexer.source.Contents, "#!") {
				lexer.Token = THashbang
			hashbang:
				for {
					lexer.step()
					switch lexer.codePoint {
					case '\r', '\n', '\u2028', '\u2029':
						break hashbang

					case -1: // This indicates the end of the file
						break hashbang
					}
				}
				lexer.Identifier = lexer.Raw()
			} else {
				lexer.SyntaxError()
			}

		case '\r', '\n', '\u2028', '\u2029':
			lexer.step()
			lexer.HasNewlineBefore = true
			continue

		case '\t', '\f', '\v', ' ', '\xA0', '\uFEFF':
			lexer.step()
			continue

		case '(':
			lexer.step()
			lexer.Token = TOpenParen

		case ')':
			lexer.step()
			lexer.Token = TCloseParen

		case '[':
			lexer.step()
			lexer.Token = TOpenBracket

		case ']':
			lexer.step()
			lexer.Token = TCloseBracket

		case '{':
			lexer.step()
			lexer.Token = TOpenBrace

		case '}':
			lexer.step()
			lexer.Token = TCloseBrace

		case ',':
			lexer.step()
			lexer.Token = TComma

		case ':':
			lexer.step()
			lexer.Token = TColon

		case ';':
			lexer.step()
			lexer.Token = TSemicolon

		case '@':
			lexer.step()
			lexer.Token = TAt

		case '~':
			lexer.step()
			lexer.Token = TTilde

		case '?':
			// '?' or '??' or '?.'
			lexer.step()
			switch lexer.codePoint {
			case '?':
				lexer.step()
				lexer.Token = TQuestionQuestion
			case '.':
				lexer.Token = TQuestion
				current := lexer.current
				contents := lexer.source.Contents

				// Lookahead to disambiguate with 'a?.1:b'
				if current < len(contents) {
					c := contents[current]
					if c < '0' || c > '9' {
						lexer.step()
						lexer.Token = TQuestionDot
					}
				}
			default:
				lexer.Token = TQuestion
			}

		case '%':
			// '%' or '%='
			lexer.step()
			switch lexer.codePoint {
			case '=':
				lexer.step()
				lexer.Token = TPercentEquals
			default:
				lexer.Token = TPercent
			}

		case '&':
			// '&' or '&=' or '&&'
			lexer.step()
			switch lexer.codePoint {
			case '=':
				lexer.step()
				lexer.Token = TAmpersandEquals
			case '&':
				lexer.step()
				lexer.Token = TAmpersandAmpersand
			default:
				lexer.Token = TAmpersand
			}

		case '|':
			// '|' or '|=' or '||'
			lexer.step()
			switch lexer.codePoint {
			case '=':
				lexer.step()
				lexer.Token = TBarEquals
			case '|':
				lexer.step()
				lexer.Token = TBarBar
			default:
				lexer.Token = TBar
			}

		case '^':
			// '^' or '^='
			lexer.step()
			switch lexer.codePoint {
			case '=':
				lexer.step()
				lexer.Token = TCaretEquals
			default:
				lexer.Token = TCaret
			}

		case '+':
			// '+' or '+=' or '++'
			lexer.step()
			switch lexer.codePoint {
			case '=':
				lexer.step()
				lexer.Token = TPlusEquals
			case '+':
				lexer.step()
				lexer.Token = TPlusPlus
			default:
				lexer.Token = TPlus
			}

		case '-':
			// '-' or '-=' or '--'
			lexer.step()
			switch lexer.codePoint {
			case '=':
				lexer.step()
				lexer.Token = TMinusEquals
			case '-':
				lexer.step()
				lexer.Token = TMinusMinus
			default:
				lexer.Token = TMinus
			}

		case '*':
			// '*' or '*=' or '**' or '**='
			lexer.step()
			switch lexer.codePoint {
			case '=':
				lexer.step()
				lexer.Token = TAsteriskEquals

			case '*':
				lexer.step()
				switch lexer.codePoint {
				case '=':
					lexer.step()
					lexer.Token = TAsteriskAsteriskEquals

				default:
					lexer.Token = TAsteriskAsterisk
				}

			default:
				lexer.Token = TAsterisk
			}

		case '/':
			// '/' or '/=' or '//' or '/* ... */'
			lexer.step()
			switch lexer.codePoint {
			case '=':
				lexer.step()
				lexer.Token = TSlashEquals
				break

			case '/':
			singleLineComment:
				for {
					lexer.step()
					switch lexer.codePoint {
					case '\r', '\n', '\u2028', '\u2029':
						break singleLineComment

					case -1: // This indicates the end of the file
						break singleLineComment
					}
				}
				continue

			case '*':
				lexer.step()
			multiLineComment:
				for {
					switch lexer.codePoint {
					case '*':
						lexer.step()
						if lexer.codePoint == '/' {
							lexer.step()
							break multiLineComment
						}

					case '\r', '\n', '\u2028', '\u2029':
						lexer.step()
						lexer.HasNewlineBefore = true

					case -1: // This indicates the end of the file
						lexer.start = lexer.end
						lexer.addError(lexer.Loc(), "Expected \"*/\" to terminate multi-line comment")
						lexer.Token = TSyntaxError
						break multiLineComment

					default:
						lexer.step()
					}
				}
				continue

			default:
				lexer.Token = TSlash
			}

		case '=':
			// '=' or '=>' or '==' or '==='
			lexer.step()
			switch lexer.codePoint {
			case '>':
				lexer.step()
				lexer.Token = TEqualsGreaterThan
			case '=':
				lexer.step()
				switch lexer.codePoint {
				case '=':
					lexer.step()
					lexer.Token = TEqualsEqualsEquals
				default:
					lexer.Token = TEqualsEquals
				}
			default:
				lexer.Token = TEquals
			}

		case '<':
			// '<' or '<<' or '<=' or '<<='
			lexer.step()
			switch lexer.codePoint {
			case '=':
				lexer.step()
				lexer.Token = TLessThanEquals
			case '<':
				lexer.step()
				switch lexer.codePoint {
				case '=':
					lexer.step()
					lexer.Token = TLessThanLessThanEquals
				default:
					lexer.Token = TLessThanLessThan
				}
			default:
				lexer.Token = TLessThan
			}

		case '>':
			// '>' or '>>' or '>>>' or '>=' or '>>=' or '>>>='
			lexer.step()
			switch lexer.codePoint {
			case '=':
				lexer.step()
				lexer.Token = TGreaterThanEquals
			case '>':
				lexer.step()
				switch lexer.codePoint {
				case '=':
					lexer.step()
					lexer.Token = TGreaterThanGreaterThanEquals
				case '>':
					lexer.step()
					switch lexer.codePoint {
					case '=':
						lexer.step()
						lexer.Token = TGreaterThanGreaterThanGreaterThanEquals
					default:
						lexer.Token = TGreaterThanGreaterThanGreaterThan
					}
				default:
					lexer.Token = TGreaterThanGreaterThan
				}
			default:
				lexer.Token = TGreaterThan
			}

		case '!':
			// '!' or '!=' or '!=='
			lexer.step()
			switch lexer.codePoint {
			case '=':
				lexer.step()
				switch lexer.codePoint {
				case '=':
					lexer.step()
					lexer.Token = TExclamationEqualsEquals
				default:
					lexer.Token = TExclamationEquals
				}
			default:
				lexer.Token = TExclamation
			}

		case '\'', '"', '`':
			quote := lexer.codePoint
			hasEscape := false
			isASCII := true
			suffixLen := 1

			if quote != '`' {
				lexer.Token = TStringLiteral
			} else if lexer.rescanCloseBraceAsTemplateToken {
				lexer.Token = TTemplateTail
			} else {
				lexer.Token = TNoSubstitutionTemplateLiteral
			}
			lexer.step()

		stringLiteral:
			for {
				switch lexer.codePoint {
				case '\\':
					hasEscape = true
					lexer.step()

					// Handle Windows CRLF
					if lexer.codePoint == '\r' {
						lexer.step()
						if lexer.codePoint == '\n' {
							lexer.step()
						}
						continue
					}

				case -1: // This indicates the end of the file
					lexer.SyntaxError()

				case '\r', '\n':
					if quote != '`' {
						lexer.addError(ast.Loc{int32(lexer.end)}, "Unterminated string literal")
						panic(LexerPanic{})
					}

				case '$':
					if quote == '`' {
						lexer.step()
						if lexer.codePoint == '{' {
							suffixLen = 2
							lexer.step()
							if lexer.rescanCloseBraceAsTemplateToken {
								lexer.Token = TTemplateMiddle
							} else {
								lexer.Token = TTemplateHead
							}
							break stringLiteral
						}
						continue stringLiteral
					}

				case quote:
					lexer.step()
					break stringLiteral

				default:
					// Non-ASCII strings need the slow path
					if lexer.codePoint >= 0x80 {
						isASCII = false
					}
				}
				lexer.step()
			}

			text := lexer.source.Contents[lexer.start+1 : lexer.end-suffixLen]

			if hasEscape || !isASCII {
				// Slow path
				lexer.StringLiteral = lexer.decodeEscapeSequences(lexer.start+1, text)
			} else {
				// Fast path
				n := len(text)
				copy := make([]uint16, n)
				for i := 0; i < n; i++ {
					copy[i] = uint16(text[i])
				}
				lexer.StringLiteral = copy
			}

		case '_', '$',
			'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm',
			'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
			'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M',
			'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z':
			lexer.step()
			for IsIdentifierContinue(lexer.codePoint) {
				lexer.step()
			}
			if lexer.codePoint == '\\' {
				lexer.Identifier, lexer.Token = lexer.scanIdentifierWithEscapes()
			} else {
				contents := lexer.Raw()
				lexer.Identifier = contents
				lexer.Token = keywords[contents]
				if lexer.Token == 0 {
					lexer.Token = TIdentifier
				}
			}

		case '\\':
			lexer.Identifier, lexer.Token = lexer.scanIdentifierWithEscapes()

		case '.', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			lexer.parseNumericLiteralOrDot()

		default:
			if IsIdentifierStart(lexer.codePoint) {
				lexer.step()
				for IsIdentifierContinue(lexer.codePoint) {
					lexer.step()
				}
				if lexer.codePoint == '\\' {
					lexer.Identifier, lexer.Token = lexer.scanIdentifierWithEscapes()
				} else {
					lexer.Token = TIdentifier
					lexer.Identifier = lexer.Raw()
				}
				break
			}

			lexer.end = lexer.current
			lexer.Token = TSyntaxError
		}

		return
	}
}

// This is an edge case that doesn't really exist in the wild, so it doesn't
// need to be as fast as possible.
func (lexer *Lexer) scanIdentifierWithEscapes() (string, T) {
	// First pass: scan over the identifier to see how long it is
	for {
		// Scan a unicode escape sequence. There is at least one because that's
		// what caused us to get on this slow path in the first place.
		if lexer.codePoint == '\\' {
			lexer.step()
			if lexer.codePoint != 'u' {
				lexer.SyntaxError()
			}
			lexer.step()
			if lexer.codePoint == '{' {
				// Variable-length
				lexer.step()
				for lexer.codePoint != '}' {
					switch lexer.codePoint {
					case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
						'a', 'b', 'c', 'd', 'e', 'f',
						'A', 'B', 'C', 'D', 'E', 'F':
						lexer.step()
					default:
						lexer.SyntaxError()
					}
				}
				lexer.step()
			} else {
				// Fixed-length
				for j := 0; j < 4; j++ {
					switch lexer.codePoint {
					case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
						'a', 'b', 'c', 'd', 'e', 'f',
						'A', 'B', 'C', 'D', 'E', 'F':
						lexer.step()
					default:
						lexer.SyntaxError()
					}
				}
			}
			continue
		}

		// Stop when we reach the end of the identifier
		if !IsIdentifierContinue(lexer.codePoint) {
			break
		}
		lexer.step()
	}

	// Second pass: re-use our existing escape sequence parser
	text := string(utf16.Decode(lexer.decodeEscapeSequences(lexer.start, lexer.Raw())))

	// Even though it was escaped, it must still be a valid identifier
	if !IsIdentifier(text) {
		lexer.log.AddRangeError(lexer.source, ast.Range{ast.Loc{int32(lexer.start)}, int32(lexer.end - lexer.start)},
			fmt.Sprintf("Invalid identifier: %q", text))
	}

	// Escaped keywords are not allowed to work as actual keywords, but they are
	// allowed wherever we allow identifiers or keywords. For example:
	//
	//   // This is an error (equivalent to "var var;")
	//   var \u0076\u0061\u0072;
	//
	//   // This is an error (equivalent to "var foo;" except for this rule)
	//   \u0076\u0061\u0072 foo;
	//
	//   // This is an fine (equivalent to "foo.var;")
	//   foo.\u0076\u0061\u0072;
	//
	if keywords[text] != 0 {
		return text, TEscapedKeyword
	} else {
		return text, TIdentifier
	}
}

func (lexer *Lexer) parseNumericLiteralOrDot() {
	// Number or dot
	first := lexer.codePoint
	lexer.step()

	// Dot without a digit after it
	if first == '.' && (lexer.codePoint < '0' || lexer.codePoint > '9') {
		// "..."
		if lexer.codePoint == '.' &&
			lexer.current < len(lexer.source.Contents) &&
			lexer.source.Contents[lexer.current] == '.' {
			lexer.step()
			lexer.step()
			lexer.Token = TDotDotDot
			return
		}

		// "."
		lexer.Token = TDot
		return
	}

	underscoreCount := 0
	lastUnderscoreEnd := 0
	hasDotOrExponent := first == '.'
	isLegacyOctalLiteral := false
	base := 0.0

	// Assume this is a number, but potentially change to a bigint later
	lexer.Token = TNumericLiteral

	// Check for binary, octal, or hexadecimal literal
	if first == '0' {
		switch lexer.codePoint {
		case 'b', 'B':
			base = 2

		case 'o', 'O':
			base = 8

		case 'x', 'X':
			base = 16

		case '0', '1', '2', '3', '4', '5', '6', '7':
			base = 8
			isLegacyOctalLiteral = true
		}
	}

	if base != 0 {
		// Integer literal
		isFirst := true
		isInvalidLegacyOctalLiteral := false
		lexer.Number = 0
		if !isLegacyOctalLiteral {
			lexer.step()
		}

	integerLiteral:
		for {
			switch lexer.codePoint {
			case '_':
				// Cannot have multiple underscores in a row
				if lastUnderscoreEnd > 0 && lexer.end == lastUnderscoreEnd+1 {
					lexer.SyntaxError()
				}

				// The first digit must exist
				if isFirst {
					lexer.SyntaxError()
				}

				lastUnderscoreEnd = lexer.end
				underscoreCount++

			case '0', '1':
				lexer.Number = lexer.Number*base + float64(lexer.codePoint-'0')

			case '2', '3', '4', '5', '6', '7':
				if base == 2 {
					lexer.SyntaxError()
				}
				lexer.Number = lexer.Number*base + float64(lexer.codePoint-'0')

			case '8', '9':
				if isLegacyOctalLiteral {
					isInvalidLegacyOctalLiteral = true
				} else if base < 10 {
					lexer.SyntaxError()
				}
				lexer.Number = lexer.Number*base + float64(lexer.codePoint-'0')

			case 'A', 'B', 'C', 'D', 'E', 'F':
				if base != 16 {
					lexer.SyntaxError()
				}
				lexer.Number = lexer.Number*base + float64(lexer.codePoint+10-'A')

			case 'a', 'b', 'c', 'd', 'e', 'f':
				if base != 16 {
					lexer.SyntaxError()
				}
				lexer.Number = lexer.Number*base + float64(lexer.codePoint+10-'a')

			default:
				// The first digit must exist
				if isFirst {
					lexer.SyntaxError()
				}

				break integerLiteral
			}

			lexer.step()
			isFirst = false
		}

		isBigIntegerLiteral := lexer.codePoint == 'n' && !hasDotOrExponent

		// Slow path: do we need to re-scan the input as text?
		if isBigIntegerLiteral || isInvalidLegacyOctalLiteral {
			text := lexer.Raw()

			// Can't use a leading zero for bigint literals
			if isBigIntegerLiteral && isLegacyOctalLiteral {
				lexer.SyntaxError()
			}

			// Filter out underscores
			if underscoreCount > 0 {
				bytes := make([]byte, 0, len(text)-underscoreCount)
				for i := 0; i < len(text); i++ {
					c := text[i]
					if c != '_' {
						bytes = append(bytes, c)
					}
				}
				text = string(bytes)
			}

			// Store bigints as text to avoid precision loss
			if isBigIntegerLiteral {
				lexer.Identifier = text
			} else if isInvalidLegacyOctalLiteral {
				// Legacy octal literals may turn out to be a base 10 literal after all
				value, _ := strconv.ParseFloat(text, 64)
				lexer.Number = value
			}
		}
	} else {
		// Floating-point literal

		// Initial digits
		for {
			if lexer.codePoint < '0' || lexer.codePoint > '9' {
				if lexer.codePoint != '_' {
					break
				}

				// Cannot have multiple underscores in a row
				if lastUnderscoreEnd > 0 && lexer.end == lastUnderscoreEnd+1 {
					lexer.SyntaxError()
				}

				lastUnderscoreEnd = lexer.end
				underscoreCount++
			}
			lexer.step()
		}

		// Fractional digits
		if first != '.' && lexer.codePoint == '.' {
			// An underscore must not come last
			if lastUnderscoreEnd > 0 && lexer.end == lastUnderscoreEnd+1 {
				lexer.end--
				lexer.SyntaxError()
			}

			hasDotOrExponent = true
			lexer.step()
			for {
				if lexer.codePoint < '0' || lexer.codePoint > '9' {
					if lexer.codePoint != '_' {
						break
					}

					// Cannot have multiple underscores in a row
					if lastUnderscoreEnd > 0 && lexer.end == lastUnderscoreEnd+1 {
						lexer.SyntaxError()
					}

					lastUnderscoreEnd = lexer.end
					underscoreCount++
				}
				lexer.step()
			}
		}

		// Exponent
		if lexer.codePoint == 'e' || lexer.codePoint == 'E' {
			// An underscore must not come last
			if lastUnderscoreEnd > 0 && lexer.end == lastUnderscoreEnd+1 {
				lexer.end--
				lexer.SyntaxError()
			}

			hasDotOrExponent = true
			lexer.step()
			if lexer.codePoint == '+' || lexer.codePoint == '-' {
				lexer.step()
			}
			if lexer.codePoint < '0' || lexer.codePoint > '9' {
				lexer.SyntaxError()
			}
			for {
				if lexer.codePoint < '0' || lexer.codePoint > '9' {
					if lexer.codePoint != '_' {
						break
					}

					// Cannot have multiple underscores in a row
					if lastUnderscoreEnd > 0 && lexer.end == lastUnderscoreEnd+1 {
						lexer.SyntaxError()
					}

					lastUnderscoreEnd = lexer.end
					underscoreCount++
				}
				lexer.step()
			}
		}

		// Take a slice of the text to parse
		text := lexer.Raw()

		// Filter out underscores
		if underscoreCount > 0 {
			bytes := make([]byte, 0, len(text)-underscoreCount)
			for i := 0; i < len(text); i++ {
				c := text[i]
				if c != '_' {
					bytes = append(bytes, c)
				}
			}
			text = string(bytes)
		}

		if lexer.codePoint == 'n' && !hasDotOrExponent {
			// The only bigint literal that can start with 0 is "0n"
			if len(text) > 1 && first == '0' {
				lexer.SyntaxError()
			}

			// Store bigints as text to avoid precision loss
			lexer.Identifier = text
		} else if !hasDotOrExponent && lexer.end-lexer.start < 10 {
			// Parse a 32-bit integer (very fast path)
			var number uint32 = 0
			for _, c := range text {
				number = number*10 + uint32(c-'0')
			}
			lexer.Number = float64(number)
		} else {
			// Parse a double-precision floating-point number
			value, _ := strconv.ParseFloat(text, 64)
			lexer.Number = value
		}
	}

	// An underscore must not come last
	if lastUnderscoreEnd > 0 && lexer.end == lastUnderscoreEnd+1 {
		lexer.end--
		lexer.SyntaxError()
	}

	// Handle bigint literals after the underscore-at-end check above
	if lexer.codePoint == 'n' && !hasDotOrExponent {
		lexer.Token = TBigIntegerLiteral
		lexer.step()
	}

	// Identifiers can't occur immediately after numbers
	if IsIdentifierStart(lexer.codePoint) {
		lexer.SyntaxError()
	}
}

func (lexer *Lexer) ScanRegExp() {
	validateAndStep := func() {
		if lexer.codePoint == '\\' {
			lexer.step()
		}

		switch lexer.codePoint {
		case '\r', '\n', 0x2028, 0x2029:
			// Newlines aren't allowed in regular expressions
			lexer.SyntaxError()

		case -1: // This indicates the end of the file
			lexer.SyntaxError()

		default:
			lexer.step()
		}
	}

	for {
		switch lexer.codePoint {
		case '/':
			lexer.step()
			for IsIdentifierContinue(lexer.codePoint) {
				switch lexer.codePoint {
				case 'g', 'i', 'm', 's', 'u', 'y':
					lexer.step()

				default:
					lexer.SyntaxError()
				}
			}
			return

		case '[':
			lexer.step()
			for lexer.codePoint != ']' {
				validateAndStep()
			}
			lexer.step()

		default:
			validateAndStep()
		}
	}
}

func decodeJSXEntities(decoded []uint16, text string) []uint16 {
	i := 0

	for i < len(text) {
		c, width := utf8.DecodeRuneInString(text[i:])
		i += width

		if c == '&' {
			length := strings.IndexByte(text[i:], ';')
			if length > 0 {
				entity := text[i : i+length]
				if entity[0] == '#' {
					number := entity[1:]
					base := 10
					if len(number) > 1 && number[0] == 'x' {
						number = number[1:]
						base = 16
					}
					if value, err := strconv.ParseInt(number, base, 32); err == nil {
						c = rune(value)
						i += length + 1
					}
				} else if value, ok := jsxEntity[entity]; ok {
					c = value
					i += length + 1
				}
			}
		}

		if c <= 0xFFFF {
			decoded = append(decoded, uint16(c))
		} else {
			c -= 0x10000
			decoded = append(decoded, uint16(0xD800+((c>>10)&0x3FF)), uint16(0xDC00+(c&0x3FF)))
		}
	}

	return decoded
}

func fixWhitespaceAndDecodeJSXEntities(text string) []uint16 {
	lastNonWhitespace := -1
	decoded := []uint16{}
	i := 0

	// Trim whitespace off the end of the first line
	firstNonWhitespace := 0

	// Split into lines
	for i < len(text) {
		c, width := utf8.DecodeRuneInString(text[i:])

		switch c {
		case '\r', '\n', '\u2028', '\u2029':
			// Newline
			if firstNonWhitespace != -1 && lastNonWhitespace != -1 {
				if len(decoded) > 0 {
					decoded = append(decoded, ' ')
				}

				// Trim whitespace off the start and end of lines in the middle
				decoded = decodeJSXEntities(decoded, text[firstNonWhitespace:lastNonWhitespace+1])
			}

			// Reset for the next line
			firstNonWhitespace = -1

		case '\t', '\f', '\v', ' ', '\xA0', '\uFEFF':
			// Whitespace

		default:
			lastNonWhitespace = i
			if firstNonWhitespace == -1 {
				firstNonWhitespace = i
			}
		}

		i += width
	}

	if firstNonWhitespace != -1 {
		if len(decoded) > 0 {
			decoded = append(decoded, ' ')
		}

		// Trim whitespace off the start of the last line
		decoded = decodeJSXEntities(decoded, text[firstNonWhitespace:])
	}

	return decoded
}

func (lexer *Lexer) decodeEscapeSequences(start int, text string) []uint16 {
	decoded := []uint16{}
	i := 0

	for i < len(text) {
		c, width := utf8.DecodeRuneInString(text[i:])
		i += width

		switch c {
		case '\\':
			c2, width2 := utf8.DecodeRuneInString(text[i:])
			i += width2

			switch c2 {
			case 'b':
				decoded = append(decoded, '\b')
				continue

			case 'f':
				decoded = append(decoded, '\f')
				continue

			case 'n':
				decoded = append(decoded, '\n')
				continue

			case 'r':
				decoded = append(decoded, '\r')
				continue

			case 't':
				decoded = append(decoded, '\t')
				continue

			case 'v':
				decoded = append(decoded, '\v')
				continue

			case '0', '1', '2', '3', '4', '5', '6', '7':
				// 1-3 digit octal
				value := c2 - '0'
				c3, width3 := utf8.DecodeRuneInString(text[i:])
				switch c3 {
				case '0', '1', '2', '3', '4', '5', '6', '7':
					value = value*8 + c3 - '0'
					i += width3
					c4, width4 := utf8.DecodeRuneInString(text[i:])
					switch c4 {
					case '0', '1', '2', '3', '4', '5', '6', '7':
						temp := value*8 + c4 - '0'
						if temp < 256 {
							value = temp
							i += width4
						}
					}
				}
				c = value

			case 'x':
				// 2-digit hexadecimal
				value := '\000'
				for j := 0; j < 2; j++ {
					c3, width3 := utf8.DecodeRuneInString(text[i:])
					i += width3
					switch c3 {
					case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
						value = value*16 | (c3 - '0')
					case 'a', 'b', 'c', 'd', 'e', 'f':
						value = value*16 | (c3 + 10 - 'a')
					case 'A', 'B', 'C', 'D', 'E', 'F':
						value = value*16 | (c3 + 10 - 'A')
					default:
						lexer.end = start + i - width3
						lexer.SyntaxError()
					}
				}
				c = value

			case 'u':
				// Unicode
				value := '\000'

				// Check the first character
				c3, width3 := utf8.DecodeRuneInString(text[i:])
				i += width3

				if c3 == '{' {
					// Variable-length
					hexStart := i - width - width2 - width3
					isFirst := true
					isOutOfRange := false
				variableLength:
					for {
						c3, width3 = utf8.DecodeRuneInString(text[i:])
						i += width3

						switch c3 {
						case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
							value = value*16 | (c3 - '0')
						case 'a', 'b', 'c', 'd', 'e', 'f':
							value = value*16 | (c3 + 10 - 'a')
						case 'A', 'B', 'C', 'D', 'E', 'F':
							value = value*16 | (c3 + 10 - 'A')
						case '}':
							if isFirst {
								lexer.end = start + i - width3
								lexer.SyntaxError()
							}
							break variableLength
						default:
							lexer.end = start + i - width3
							lexer.SyntaxError()
						}

						if value > utf8.MaxRune {
							isOutOfRange = true
						}

						isFirst = false
					}

					if isOutOfRange {
						lexer.addRangeError(ast.Range{ast.Loc{int32(start + hexStart)}, int32(i - hexStart)},
							"Unicode escape sequence is out of range")
						panic(LexerPanic{})
					}
				} else {
					// Fixed-length
					for j := 0; j < 4; j++ {
						switch c3 {
						case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
							value = value*16 | (c3 - '0')
						case 'a', 'b', 'c', 'd', 'e', 'f':
							value = value*16 | (c3 + 10 - 'a')
						case 'A', 'B', 'C', 'D', 'E', 'F':
							value = value*16 | (c3 + 10 - 'A')
						default:
							lexer.end = start + i - width3
							lexer.SyntaxError()
						}

						if j < 3 {
							c3, width3 = utf8.DecodeRuneInString(text[i:])
							i += width3
						}
					}
				}
				c = value

			case '\r':
				// Ignore line continuations. A line continuation is not an escaped newline.
				if i < len(text) && text[i] == '\n' {
					// Make sure Windows CRLF counts as a single newline
					i++
				}
				continue

			case '\n', '\u2028', '\u2029':
				// Ignore line continuations. A line continuation is not an escaped newline.
				continue

			default:
				c = c2
			}
		}

		if c <= 0xFFFF {
			decoded = append(decoded, uint16(c))
		} else {
			c -= 0x10000
			decoded = append(decoded, uint16(0xD800+((c>>10)&0x3FF)), uint16(0xDC00+(c&0x3FF)))
		}
	}

	return decoded
}

func (lexer *Lexer) RescanCloseBraceAsTemplateToken() {
	if lexer.Token != TCloseBrace {
		lexer.Expected(TCloseBrace)
	}

	lexer.rescanCloseBraceAsTemplateToken = true
	lexer.codePoint = '`'
	lexer.current = lexer.end
	lexer.end -= 1
	lexer.Next()
	lexer.rescanCloseBraceAsTemplateToken = false
}

func (lexer *Lexer) step() {
	codePoint, width := utf8.DecodeRuneInString(lexer.source.Contents[lexer.current:])

	// Use -1 to indicate the end of the file
	if width == 0 {
		codePoint = -1
	}

	lexer.codePoint = codePoint
	lexer.end = lexer.current
	lexer.current += width
}

func (lexer *Lexer) addError(loc ast.Loc, text string) {
	lexer.log.AddError(lexer.source, loc, text)
}

func (lexer *Lexer) addRangeError(r ast.Range, text string) {
	lexer.log.AddRangeError(lexer.source, r, text)
}

func StringToUTF16(text string) []uint16 {
	decoded := []uint16{}
	for _, c := range text {
		if c <= 0xFFFF {
			decoded = append(decoded, uint16(c))
		} else {
			c -= 0x10000
			decoded = append(decoded, uint16(0xD800+((c>>10)&0x3FF)), uint16(0xDC00+(c&0x3FF)))
		}
	}
	return decoded
}

func UTF16ToString(text []uint16) string {
	temp := make([]byte, utf8.UTFMax)
	b := strings.Builder{}
	n := len(text)
	for i := 0; i < n; i++ {
		r1 := rune(text[i])
		if utf16.IsSurrogate(r1) && i+1 < n {
			r2 := rune(text[i+1])
			r1 = (r1-0xD800)<<10 | (r2 - 0xDC00) + 0x10000
			i++
		}
		width := encodeWTF8Rune(temp, r1)
		b.Write(temp[:width])
	}
	return b.String()
}

// This is a clone of "utf8.EncodeRune" that has been modified to encode using
// WTF-8 instead. See https://simonsapin.github.io/wtf-8/ for more info.
func encodeWTF8Rune(p []byte, r rune) int {
	// Negative values are erroneous. Making it unsigned addresses the problem.
	switch i := uint32(r); {
	case i <= 0x7F:
		p[0] = byte(r)
		return 1
	case i <= 0x7FF:
		_ = p[1] // eliminate bounds checks
		p[0] = 0xC0 | byte(r>>6)
		p[1] = 0x80 | byte(r)&0x3F
		return 2
	case i > utf8.MaxRune:
		r = utf8.RuneError
		fallthrough
	case i <= 0xFFFF:
		_ = p[2] // eliminate bounds checks
		p[0] = 0xE0 | byte(r>>12)
		p[1] = 0x80 | byte(r>>6)&0x3F
		p[2] = 0x80 | byte(r)&0x3F
		return 3
	default:
		_ = p[3] // eliminate bounds checks
		p[0] = 0xF0 | byte(r>>18)
		p[1] = 0x80 | byte(r>>12)&0x3F
		p[2] = 0x80 | byte(r>>6)&0x3F
		p[3] = 0x80 | byte(r)&0x3F
		return 4
	}
}

// This is a clone of "utf8.DecodeRuneInString" that has been modified to
// encode using WTF-8 instead. See https://simonsapin.github.io/wtf-8/ for
// more info.
func DecodeUTF8Rune(s string) (rune, int) {
	n := len(s)
	if n < 1 {
		return utf8.RuneError, 0
	}

	s0 := s[0]
	if s0 < 0x80 {
		return rune(s0), 1
	}

	var sz int
	if (s0 & 0xE0) == 0xC0 {
		sz = 2
	} else if (s0 & 0xF0) == 0xE0 {
		sz = 3
	} else if (s0 & 0xF8) == 0xF0 {
		sz = 4
	} else {
		return utf8.RuneError, 1
	}

	if n < sz {
		return utf8.RuneError, 0
	}

	s1 := s[1]
	if (s1 & 0xC0) != 0x80 {
		return utf8.RuneError, 1
	}

	if sz == 2 {
		cp := rune(s0&0x1F)<<6 | rune(s1&0x3F)
		if cp < 0x80 {
			return utf8.RuneError, 1
		}
		return cp, 2
	}
	s2 := s[2]

	if (s2 & 0xC0) != 0x80 {
		return utf8.RuneError, 1
	}

	if sz == 3 {
		cp := rune(s0&0x0F)<<12 | rune(s1&0x3F)<<6 | rune(s2&0x3F)
		if cp < 0x0800 {
			return utf8.RuneError, 1
		}
		return cp, 3
	}
	s3 := s[3]

	if (s3 & 0xC0) != 0x80 {
		return utf8.RuneError, 1
	}

	cp := rune(s0&0x07)<<18 | rune(s1&0x3F)<<12 | rune(s2&0x3F)<<6 | rune(s3&0x3F)
	if cp < 0x010000 || cp > 0x10FFFF {
		return utf8.RuneError, 1
	}
	return cp, 4
}
