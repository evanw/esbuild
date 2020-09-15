package js_lexer

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
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/logger"
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
	TAmpersandAmpersandEquals
	TAmpersandEquals
	TAsteriskAsteriskEquals
	TAsteriskEquals
	TBarBarEquals
	TBarEquals
	TCaretEquals
	TEquals
	TGreaterThanGreaterThanEquals
	TGreaterThanGreaterThanGreaterThanEquals
	TLessThanLessThanEquals
	TMinusEquals
	TPercentEquals
	TPlusEquals
	TQuestionQuestionEquals
	TSlashEquals

	// Class-private fields and methods
	TPrivateIdentifier

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
)

var Keywords = map[string]T{
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
}

var StrictModeReservedWords = map[string]bool{
	"implements": true,
	"interface":  true,
	"let":        true,
	"package":    true,
	"private":    true,
	"protected":  true,
	"public":     true,
	"static":     true,
	"yield":      true,
}

type json struct {
	parse         bool
	allowComments bool
}

type Lexer struct {
	log                             logger.Log
	source                          logger.Source
	current                         int
	start                           int
	end                             int
	ApproximateNewlineCount         int
	Token                           T
	HasNewlineBefore                bool
	HasPureCommentBefore            bool
	PreserveAllCommentsBefore       bool
	CommentsToPreserveBefore        []js_ast.Comment
	codePoint                       rune
	StringLiteral                   []uint16
	Identifier                      string
	JSXFactoryPragmaComment         js_ast.Span
	JSXFragmentPragmaComment        js_ast.Span
	SourceMappingURL                js_ast.Span
	Number                          float64
	rescanCloseBraceAsTemplateToken bool
	json                            json

	// The log is disabled during speculative scans that may backtrack
	IsLogDisabled bool
}

type LexerPanic struct{}

func NewLexer(log logger.Log, source logger.Source) Lexer {
	lexer := Lexer{
		log:    log,
		source: source,
	}
	lexer.step()
	lexer.Next()
	return lexer
}

func NewLexerJSON(log logger.Log, source logger.Source, allowComments bool) Lexer {
	lexer := Lexer{
		log:    log,
		source: source,
		json: json{
			parse:         true,
			allowComments: allowComments,
		},
	}
	lexer.step()
	lexer.Next()
	return lexer
}

func (lexer *Lexer) Loc() logger.Loc {
	return logger.Loc{Start: int32(lexer.start)}
}

func (lexer *Lexer) Range() logger.Range {
	return logger.Range{Loc: logger.Loc{Start: int32(lexer.start)}, Len: int32(lexer.end - lexer.start)}
}

func (lexer *Lexer) Raw() string {
	return lexer.source.Contents[lexer.start:lexer.end]
}

func (lexer *Lexer) RawTemplateContents() string {
	var text string
	switch lexer.Token {
	case TNoSubstitutionTemplateLiteral, TTemplateTail:
		// "`x`" or "}x`"
		text = lexer.source.Contents[lexer.start+1 : lexer.end-1]

	case TTemplateHead, TTemplateMiddle:
		// "`x${" or "}x${"
		text = lexer.source.Contents[lexer.start+1 : lexer.end-2]
	}

	if strings.IndexByte(text, '\r') == -1 {
		return text
	}

	// From the specification:
	//
	// 11.8.6.1 Static Semantics: TV and TRV
	//
	// TV excludes the code units of LineContinuation while TRV includes
	// them. <CR><LF> and <CR> LineTerminatorSequences are normalized to
	// <LF> for both TV and TRV. An explicit EscapeSequence is needed to
	// include a <CR> or <CR><LF> sequence.

	bytes := []byte(text)
	end := 0
	i := 0

	for i < len(bytes) {
		c := bytes[i]
		i++

		if c == '\r' {
			// Convert '\r\n' into '\n'
			if i < len(bytes) && bytes[i] == '\n' {
				i++
			}

			// Convert '\r' into '\n'
			c = '\n'
		}

		bytes[end] = c
		end++
	}

	return string(bytes[:end])
}

func (lexer *Lexer) IsIdentifierOrKeyword() bool {
	return lexer.Token >= TIdentifier
}

func (lexer *Lexer) IsContextualKeyword(text string) bool {
	return lexer.Token == TIdentifier && lexer.Raw() == text
}

func (lexer *Lexer) ExpectContextualKeyword(text string) {
	if !lexer.IsContextualKeyword(text) {
		lexer.ExpectedString(fmt.Sprintf("%q", text))
	}
	lexer.Next()
}

func (lexer *Lexer) SyntaxError() {
	loc := logger.Loc{Start: int32(lexer.end)}
	message := "Unexpected end of file"
	if lexer.end < len(lexer.source.Contents) {
		c, _ := utf8.DecodeRuneInString(lexer.source.Contents[lexer.end:])
		if c < 0x20 {
			message = fmt.Sprintf("Syntax error \"\\x%02X\"", c)
		} else if c >= 0x80 {
			message = fmt.Sprintf("Syntax error \"\\u{%x}\"", c)
		} else if c != '"' {
			message = fmt.Sprintf("Syntax error \"%c\"", c)
		} else {
			message = "Syntax error '\"'"
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
	found := fmt.Sprintf("%q", lexer.Raw())
	if lexer.start == len(lexer.source.Contents) {
		found = "end of file"
	}
	lexer.addRangeError(lexer.Range(), fmt.Sprintf("Unexpected %s", found))
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

// This parses a single "<" token. If that is the first part of a longer token,
// this function splits off the first "<" and leaves the remainder of the
// current token as another, smaller token. For example, "<<=" becomes "<=".
func (lexer *Lexer) ExpectLessThan(isInsideJSXElement bool) {
	switch lexer.Token {
	case TLessThan:
		if isInsideJSXElement {
			lexer.NextInsideJSXElement()
		} else {
			lexer.Next()
		}

	case TLessThanEquals:
		lexer.Token = TEquals
		lexer.start++

	case TLessThanLessThan:
		lexer.Token = TLessThan
		lexer.start++

	case TLessThanLessThanEquals:
		lexer.Token = TLessThanEquals
		lexer.start++

	default:
		lexer.Expected(TLessThan)
	}
}

// This parses a single ">" token. If that is the first part of a longer token,
// this function splits off the first ">" and leaves the remainder of the
// current token as another, smaller token. For example, ">>=" becomes ">=".
func (lexer *Lexer) ExpectGreaterThan(isInsideJSXElement bool) {
	switch lexer.Token {
	case TGreaterThan:
		if isInsideJSXElement {
			lexer.NextInsideJSXElement()
		} else {
			lexer.Next()
		}

	case TGreaterThanEquals:
		lexer.Token = TEquals
		lexer.start++

	case TGreaterThanGreaterThan:
		lexer.Token = TGreaterThan
		lexer.start++

	case TGreaterThanGreaterThanEquals:
		lexer.Token = TGreaterThanEquals
		lexer.start++

	case TGreaterThanGreaterThanGreaterThan:
		lexer.Token = TGreaterThanGreaterThan
		lexer.start++

	case TGreaterThanGreaterThanGreaterThanEquals:
		lexer.Token = TGreaterThanGreaterThanEquals
		lexer.start++

	default:
		lexer.Expected(TGreaterThan)
	}
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

func ForceValidIdentifier(text string) string {
	if IsIdentifier(text) {
		return text
	}
	sb := strings.Builder{}

	// Identifier start
	c, width := utf8.DecodeRuneInString(text)
	text = text[width:]
	if IsIdentifierStart(c) {
		sb.WriteRune(c)
	} else {
		sb.WriteRune('_')
	}

	// Identifier continue
	for text != "" {
		c, width := utf8.DecodeRuneInString(text)
		text = text[width:]
		if IsIdentifierContinue(c) {
			sb.WriteRune(c)
		} else {
			sb.WriteRune('_')
		}
	}

	return sb.String()
}

// This does "IsIdentifier(UTF16ToString(text))" without any allocations
func IsIdentifierUTF16(text []uint16) bool {
	n := len(text)
	if n == 0 {
		return false
	}
	for i := 0; i < n; i++ {
		r1 := rune(text[i])
		if utf16.IsSurrogate(r1) && i+1 < n {
			r2 := rune(text[i+1])
			r1 = (r1-0xD800)<<10 | (r2 - 0xDC00) + 0x10000
			i++
		}
		if i == 0 {
			if !IsIdentifierStart(r1) {
				return false
			}
		} else {
			if !IsIdentifierContinue(r1) {
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

// See the "White Space Code Points" table in the ECMAScript standard
func IsWhitespace(codePoint rune) bool {
	switch codePoint {
	case
		'\u0009', // character tabulation
		'\u000B', // line tabulation
		'\u000C', // form feed
		'\u0020', // space
		'\u00A0', // no-break space

		// Unicode "Space_Separator" code points
		'\u1680', // ogham space mark
		'\u2000', // en quad
		'\u2001', // em quad
		'\u2002', // en space
		'\u2003', // em space
		'\u2004', // three-per-em space
		'\u2005', // four-per-em space
		'\u2006', // six-per-em space
		'\u2007', // figure space
		'\u2008', // punctuation space
		'\u2009', // thin space
		'\u200A', // hair space
		'\u202F', // narrow no-break space
		'\u205F', // medium mathematical space
		'\u3000', // ideographic space

		'\uFEFF': // zero width non-breaking space
		return true

	default:
		return false
	}
}

func RangeOfIdentifier(source logger.Source, loc logger.Loc) logger.Range {
	text := source.Contents[loc.Start:]
	if len(text) == 0 {
		return logger.Range{Loc: loc, Len: 0}
	}

	i := 0
	c, width := utf8.DecodeRuneInString(text)
	i += width

	if IsIdentifierStart(c) {
		// Search for the end of the identifier
		for i < len(text) {
			c2, width2 := utf8.DecodeRuneInString(text[i:])
			if !IsIdentifierContinue(c2) {
				return logger.Range{Loc: loc, Len: int32(i)}
			}
			i += width2
		}
	}

	// When minifying, this identifier may have originally been a string
	return source.RangeOfString(loc)
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

		case '{':
			lexer.step()
			lexer.Token = TOpenBrace

		case '<':
			lexer.step()
			lexer.Token = TLessThan

		default:
			needsFixing := false

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

				// Skip this token if it turned out to be empty after trimming
				if len(lexer.StringLiteral) == 0 {
					lexer.HasNewlineBefore = true
					continue
				}
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

		case '\t', ' ':
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
						panic(LexerPanic{})

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
			// Check for unusual whitespace characters
			if IsWhitespace(lexer.codePoint) {
				lexer.step()
				continue
			}

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
	lexer.HasNewlineBefore = lexer.end == 0
	lexer.HasPureCommentBefore = false
	lexer.CommentsToPreserveBefore = nil

	for {
		lexer.start = lexer.end
		lexer.Token = 0

		switch lexer.codePoint {
		case -1: // This indicates the end of the file
			lexer.Token = TEndOfFile

		case '#':
			if lexer.start == 0 && strings.HasPrefix(lexer.source.Contents, "#!") {
				// "#!/usr/bin/env node"
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
				// "#foo"
				lexer.step()
				if lexer.codePoint == '\\' {
					lexer.Identifier, _ = lexer.scanIdentifierWithEscapes(privateIdentifier)
				} else {
					if !IsIdentifierStart(lexer.codePoint) {
						lexer.SyntaxError()
					}
					lexer.step()
					for IsIdentifierContinue(lexer.codePoint) {
						lexer.step()
					}
					if lexer.codePoint == '\\' {
						lexer.Identifier, _ = lexer.scanIdentifierWithEscapes(privateIdentifier)
					} else {
						lexer.Identifier = lexer.Raw()
					}
				}
				lexer.Token = TPrivateIdentifier
			}

		case '\r', '\n', '\u2028', '\u2029':
			lexer.step()
			lexer.HasNewlineBefore = true
			continue

		case '\t', ' ':
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
			// '?' or '?.' or '??' or '??='
			lexer.step()
			switch lexer.codePoint {
			case '?':
				lexer.step()
				switch lexer.codePoint {
				case '=':
					lexer.step()
					lexer.Token = TQuestionQuestionEquals
				default:
					lexer.Token = TQuestionQuestion
				}
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
			// '&' or '&=' or '&&' or '&&='
			lexer.step()
			switch lexer.codePoint {
			case '=':
				lexer.step()
				lexer.Token = TAmpersandEquals
			case '&':
				lexer.step()
				switch lexer.codePoint {
				case '=':
					lexer.step()
					lexer.Token = TAmpersandAmpersandEquals
				default:
					lexer.Token = TAmpersandAmpersand
				}
			default:
				lexer.Token = TAmpersand
			}

		case '|':
			// '|' or '|=' or '||' or '||='
			lexer.step()
			switch lexer.codePoint {
			case '=':
				lexer.step()
				lexer.Token = TBarEquals
			case '|':
				lexer.step()
				switch lexer.codePoint {
				case '=':
					lexer.step()
					lexer.Token = TBarBarEquals
				default:
					lexer.Token = TBarBar
				}
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
			// '-' or '-=' or '--' or '-->'
			lexer.step()
			switch lexer.codePoint {
			case '=':
				lexer.step()
				lexer.Token = TMinusEquals
			case '-':
				lexer.step()

				// Handle legacy HTML-style comments
				if lexer.codePoint == '>' && lexer.HasNewlineBefore {
					lexer.step()
					lexer.log.AddRangeWarning(&lexer.source, lexer.Range(),
						"Treating \"-->\" as the start of a legacy HTML single-line comment")
				singleLineHTMLCloseComment:
					for {
						switch lexer.codePoint {
						case '\r', '\n', '\u2028', '\u2029':
							break singleLineHTMLCloseComment

						case -1: // This indicates the end of the file
							break singleLineHTMLCloseComment
						}
						lexer.step()
					}
					continue
				}

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
				if lexer.json.parse && !lexer.json.allowComments {
					lexer.addRangeError(lexer.Range(), "JSON does not support comments")
				}
				lexer.scanCommentText()
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
						panic(LexerPanic{})

					default:
						lexer.step()
					}
				}
				if lexer.json.parse && !lexer.json.allowComments {
					lexer.addRangeError(lexer.Range(), "JSON does not support comments")
				}
				lexer.scanCommentText()
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
			// '<' or '<<' or '<=' or '<<=' or '<!--'
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

				// Handle legacy HTML-style comments
			case '!':
				if strings.HasPrefix(lexer.source.Contents[lexer.start:], "<!--") {
					lexer.step()
					lexer.step()
					lexer.step()
					lexer.log.AddRangeWarning(&lexer.source, lexer.Range(),
						"Treating \"<!--\" as the start of a legacy HTML single-line comment")
				singleLineHTMLOpenComment:
					for {
						switch lexer.codePoint {
						case '\r', '\n', '\u2028', '\u2029':
							break singleLineHTMLOpenComment

						case -1: // This indicates the end of the file
							break singleLineHTMLOpenComment
						}
						lexer.step()
					}
					continue
				}

				lexer.Token = TLessThan

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
			needsSlowPath := false
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
					needsSlowPath = true
					lexer.step()

					// Handle Windows CRLF
					if lexer.codePoint == '\r' && !lexer.json.parse {
						lexer.step()
						if lexer.codePoint == '\n' {
							lexer.step()
						}
						continue
					}

				case -1: // This indicates the end of the file
					lexer.SyntaxError()

				case '\r':
					if quote != '`' {
						lexer.addError(logger.Loc{Start: int32(lexer.end)}, "Unterminated string literal")
						panic(LexerPanic{})
					}

					// Template literals require newline normalization
					needsSlowPath = true

				case '\n':
					if quote != '`' {
						lexer.addError(logger.Loc{Start: int32(lexer.end)}, "Unterminated string literal")
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
						needsSlowPath = true
					} else if lexer.json.parse && lexer.codePoint < 0x20 {
						lexer.SyntaxError()
					}
				}
				lexer.step()
			}

			text := lexer.source.Contents[lexer.start+1 : lexer.end-suffixLen]

			if needsSlowPath {
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

			if quote == '\'' && lexer.json.parse {
				lexer.addRangeError(lexer.Range(), "JSON strings must use double quotes")
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
				lexer.Identifier, lexer.Token = lexer.scanIdentifierWithEscapes(normalIdentifier)
			} else {
				contents := lexer.Raw()
				lexer.Identifier = contents
				lexer.Token = Keywords[contents]
				if lexer.Token == 0 {
					lexer.Token = TIdentifier
				}
			}

		case '\\':
			lexer.Identifier, lexer.Token = lexer.scanIdentifierWithEscapes(normalIdentifier)

		case '.', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			lexer.parseNumericLiteralOrDot()

		default:
			// Check for unusual whitespace characters
			if IsWhitespace(lexer.codePoint) {
				lexer.step()
				continue
			}

			if IsIdentifierStart(lexer.codePoint) {
				lexer.step()
				for IsIdentifierContinue(lexer.codePoint) {
					lexer.step()
				}
				if lexer.codePoint == '\\' {
					lexer.Identifier, lexer.Token = lexer.scanIdentifierWithEscapes(normalIdentifier)
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

type identifierKind uint8

const (
	normalIdentifier identifierKind = iota
	privateIdentifier
)

// This is an edge case that doesn't really exist in the wild, so it doesn't
// need to be as fast as possible.
func (lexer *Lexer) scanIdentifierWithEscapes(kind identifierKind) (string, T) {
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
	identifier := text
	if kind == privateIdentifier {
		identifier = identifier[1:] // Skip over the "#"
	}
	if !IsIdentifier(identifier) {
		lexer.addRangeError(logger.Range{Loc: logger.Loc{Start: int32(lexer.start)}, Len: int32(lexer.end - lexer.start)},
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
	if Keywords[text] != 0 {
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

		case '0', '1', '2', '3', '4', '5', '6', '7', '_':
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
				if isFirst || isLegacyOctalLiteral {
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
		isInvalidLegacyOctalLiteral := first == '0' && (lexer.codePoint == '8' || lexer.codePoint == '9')

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

				// The specification forbids underscores in this case
				if isInvalidLegacyOctalLiteral {
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
			if lexer.codePoint == '_' {
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
	afterLastNonWhitespace := -1
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
			if firstNonWhitespace != -1 && afterLastNonWhitespace != -1 {
				if len(decoded) > 0 {
					decoded = append(decoded, ' ')
				}

				// Trim whitespace off the start and end of lines in the middle
				decoded = decodeJSXEntities(decoded, text[firstNonWhitespace:afterLastNonWhitespace])
			}

			// Reset for the next line
			firstNonWhitespace = -1

		case '\t', ' ':
			// Whitespace

		default:
			// Check for unusual whitespace characters
			if !IsWhitespace(c) {
				afterLastNonWhitespace = i + width
				if firstNonWhitespace == -1 {
					firstNonWhitespace = i
				}
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
		case '\r':
			// From the specification:
			//
			// 11.8.6.1 Static Semantics: TV and TRV
			//
			// TV excludes the code units of LineContinuation while TRV includes
			// them. <CR><LF> and <CR> LineTerminatorSequences are normalized to
			// <LF> for both TV and TRV. An explicit EscapeSequence is needed to
			// include a <CR> or <CR><LF> sequence.

			// Convert '\r\n' into '\n'
			if i < len(text) && text[i] == '\n' {
				i++
			}

			// Convert '\r' into '\n'
			decoded = append(decoded, '\n')
			continue

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
				if lexer.json.parse {
					lexer.end = start + i - width2
					lexer.SyntaxError()
				}

				decoded = append(decoded, '\v')
				continue

			case '0', '1', '2', '3', '4', '5', '6', '7':
				if lexer.json.parse {
					lexer.end = start + i - width2
					lexer.SyntaxError()
				}

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
				if lexer.json.parse {
					lexer.end = start + i - width2
					lexer.SyntaxError()
				}

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
					if lexer.json.parse {
						lexer.end = start + i - width2
						lexer.SyntaxError()
					}

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
						lexer.addRangeError(logger.Range{Loc: logger.Loc{Start: int32(start + hexStart)}, Len: int32(i - hexStart)},
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
				if lexer.json.parse {
					lexer.end = start + i - width2
					lexer.SyntaxError()
				}

				// Ignore line continuations. A line continuation is not an escaped newline.
				if i < len(text) && text[i] == '\n' {
					// Make sure Windows CRLF counts as a single newline
					i++
				}
				continue

			case '\n', '\u2028', '\u2029':
				if lexer.json.parse {
					lexer.end = start + i - width2
					lexer.SyntaxError()
				}

				// Ignore line continuations. A line continuation is not an escaped newline.
				continue

			default:
				if lexer.json.parse {
					switch c2 {
					case '"', '\\', '/':

					default:
						lexer.end = start + i - width2
						lexer.SyntaxError()
					}
				}

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

	// Track the approximate number of newlines in the file so we can preallocate
	// the line offset table in the printer for source maps. The line offset table
	// is the #1 highest allocation in the heap profile, so this is worth doing.
	// This count is approximate because it handles "\n" and "\r\n" (the common
	// cases) but not "\r" or "\u2028" or "\u2029". Getting this wrong is harmless
	// because it's only a preallocation. The array will just grow if it's too small.
	if codePoint == '\n' {
		lexer.ApproximateNewlineCount++
	}

	lexer.codePoint = codePoint
	lexer.end = lexer.current
	lexer.current += width
}

func (lexer *Lexer) addError(loc logger.Loc, text string) {
	if !lexer.IsLogDisabled {
		lexer.log.AddError(&lexer.source, loc, text)
	}
}

func (lexer *Lexer) addRangeError(r logger.Range, text string) {
	if !lexer.IsLogDisabled {
		lexer.log.AddRangeError(&lexer.source, r, text)
	}
}

func hasPrefixWithWordBoundary(text string, prefix string) bool {
	t := len(text)
	p := len(prefix)
	if t >= p && text[0:p] == prefix {
		if t == p {
			return true
		}
		c, _ := utf8.DecodeRuneInString(text[p:])
		if !IsIdentifierContinue(c) {
			return true
		}
	}
	return false
}

type pragmaArg uint8

const (
	pragmaNoSpaceFirst pragmaArg = iota
	pragmaSkipSpaceFirst
)

func scanForPragmaArg(kind pragmaArg, start int, pragma string, text string) (js_ast.Span, bool) {
	text = text[len(pragma):]
	start += len(pragma)

	if text == "" {
		return js_ast.Span{}, false
	}

	// One or more whitespace characters
	c, width := utf8.DecodeRuneInString(text)
	if kind == pragmaSkipSpaceFirst {
		if !IsWhitespace(c) {
			return js_ast.Span{}, false
		}
		for IsWhitespace(c) {
			text = text[width:]
			start += width
			if text == "" {
				return js_ast.Span{}, false
			}
			c, width = utf8.DecodeRuneInString(text)
		}
	}

	// One or more non-whitespace characters
	i := 0
	for !IsWhitespace(c) {
		i += width
		if i >= len(text) {
			break
		}
		c, width = utf8.DecodeRuneInString(text[i:])
		if IsWhitespace(c) {
			break
		}
	}

	return js_ast.Span{
		Text: text[:i],
		Range: logger.Range{
			Loc: logger.Loc{Start: int32(start)},
			Len: int32(i),
		},
	}, true
}

func (lexer *Lexer) scanCommentText() {
	text := lexer.source.Contents[lexer.start:lexer.end]
	hasPreserveAnnotation := len(text) > 2 && text[2] == '!'

	for i, n := 0, len(text); i < n; i++ {
		switch text[i] {
		case '#':
			rest := text[i+1:]
			if hasPrefixWithWordBoundary(rest, "__PURE__") {
				lexer.HasPureCommentBefore = true
			} else if strings.HasPrefix(rest, " sourceMappingURL=") {
				if arg, ok := scanForPragmaArg(pragmaNoSpaceFirst, lexer.start+i+1, " sourceMappingURL=", rest); ok {
					lexer.SourceMappingURL = arg
				}
			}

		case '@':
			rest := text[i+1:]
			if hasPrefixWithWordBoundary(rest, "__PURE__") {
				lexer.HasPureCommentBefore = true
			} else if hasPrefixWithWordBoundary(rest, "preserve") || hasPrefixWithWordBoundary(rest, "license") {
				hasPreserveAnnotation = true
			} else if hasPrefixWithWordBoundary(rest, "jsx") {
				if arg, ok := scanForPragmaArg(pragmaSkipSpaceFirst, lexer.start+i+1, "jsx", rest); ok {
					lexer.JSXFactoryPragmaComment = arg
				}
			} else if hasPrefixWithWordBoundary(rest, "jsxFrag") {
				if arg, ok := scanForPragmaArg(pragmaSkipSpaceFirst, lexer.start+i+1, "jsxFrag", rest); ok {
					lexer.JSXFragmentPragmaComment = arg
				}
			} else if strings.HasPrefix(rest, " sourceMappingURL=") {
				if arg, ok := scanForPragmaArg(pragmaNoSpaceFirst, lexer.start+i+1, " sourceMappingURL=", rest); ok {
					lexer.SourceMappingURL = arg
				}
			}
		}
	}

	if hasPreserveAnnotation || lexer.PreserveAllCommentsBefore {
		if text[1] == '*' {
			text = removeMultiLineCommentIndent(lexer.source.Contents[:lexer.start], text)
		}

		lexer.CommentsToPreserveBefore = append(lexer.CommentsToPreserveBefore, js_ast.Comment{
			Loc:  logger.Loc{Start: int32(lexer.start)},
			Text: text,
		})
	}
}

func removeMultiLineCommentIndent(prefix string, text string) string {
	// Figure out the initial indent
	indent := 0
seekBackwardToNewline:
	for len(prefix) > 0 {
		c, size := utf8.DecodeLastRuneInString(prefix)
		switch c {
		case '\r', '\n', '\u2028', '\u2029':
			break seekBackwardToNewline
		}
		prefix = prefix[:len(prefix)-size]
		indent++
	}

	// Split the comment into lines
	var lines []string
	start := 0
	for i, c := range text {
		switch c {
		case '\r', '\n':
			// Don't double-append for Windows style "\r\n" newlines
			if start <= i {
				lines = append(lines, text[start:i])
			}

			start = i + 1

			// Ignore the second part of Windows style "\r\n" newlines
			if c == '\r' && start < len(text) && text[start] == '\n' {
				start++
			}

		case '\u2028', '\u2029':
			lines = append(lines, text[start:i])
			start = i + 3
		}
	}
	lines = append(lines, text[start:])

	// Find the minimum indent over all lines after the first line
	for _, line := range lines[1:] {
		lineIndent := 0
		for _, c := range line {
			if !IsWhitespace(c) {
				break
			}
			lineIndent++
		}
		if indent > lineIndent {
			indent = lineIndent
		}
	}

	// Trim the indent off of all lines after the first line
	for i, line := range lines {
		if i > 0 {
			lines[i] = line[indent:]
		}
	}
	return strings.Join(lines, "\n")
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

// Does "UTF16ToString(text) == str" without a temporary allocation
func UTF16EqualsString(text []uint16, str string) bool {
	if len(text) > len(str) {
		// Strings can't be equal if UTF-16 encoding is longer than UTF-8 encoding
		return false
	}
	temp := [utf8.UTFMax]byte{}
	n := len(text)
	j := 0
	for i := 0; i < n; i++ {
		r1 := rune(text[i])
		if utf16.IsSurrogate(r1) && i+1 < n {
			r2 := rune(text[i+1])
			r1 = (r1-0xD800)<<10 | (r2 - 0xDC00) + 0x10000
			i++
		}
		width := encodeWTF8Rune(temp[:], r1)
		if j+width > len(str) {
			return false
		}
		for k := 0; k < width; k++ {
			if temp[k] != str[j] {
				return false
			}
			j++
		}
	}
	return j == len(str)
}

func UTF16EqualsUTF16(a []uint16, b []uint16) bool {
	if len(a) == len(b) {
		for i, c := range a {
			if c != b[i] {
				return false
			}
		}
		return true
	}
	return false
}

// Does "append(bytes, UTF16ToString(text))" without a temporary allocation
func AppendUTF16ToBytes(bytes []byte, text []uint16) []byte {
	temp := make([]byte, utf8.UTFMax)
	n := len(text)
	for i := 0; i < n; i++ {
		r1 := rune(text[i])
		if utf16.IsSurrogate(r1) && i+1 < n {
			r2 := rune(text[i+1])
			r1 = (r1-0xD800)<<10 | (r2 - 0xDC00) + 0x10000
			i++
		}
		width := encodeWTF8Rune(temp, r1)
		bytes = append(bytes, temp[:width]...)
	}
	return bytes
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
// decode using WTF-8 instead. See https://simonsapin.github.io/wtf-8/ for
// more info.
func DecodeWTF8Rune(s string) (rune, int) {
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
