package css_lexer

import (
	"strings"
	"unicode/utf8"

	"github.com/evanw/esbuild/internal/logger"
)

// The lexer converts a source file to a stream of tokens. Unlike esbuild's
// JavaScript lexer, this CSS lexer runs to completion before the CSS parser
// begins, resulting in a single array of all tokens in the file.

type T uint8

const eof = -1
const replacementCharacter = 0xFFFD

const (
	TEndOfFile T = iota

	TAtKeyword
	TBadString
	TBadURL
	TCDC // "-->"
	TCDO // "<!--"
	TCloseBrace
	TCloseBracket
	TCloseParen
	TColon
	TComma
	TDelim
	TDelimAmpersand
	TDelimAsterisk
	TDelimBar
	TDelimCaret
	TDelimDollar
	TDelimDot
	TDelimEquals
	TDelimExclamation
	TDelimGreaterThan
	TDelimPlus
	TDelimSlash
	TDelimTilde
	TDimension
	TFunction
	THash
	THashID
	TIdent
	TNumber
	TOpenBrace
	TOpenBracket
	TOpenParen
	TPercentage
	TSemicolon
	TString
	TURL
	TWhitespace
)

var tokenToString = []string{
	"end of file",
	"@-keyword",
	"bad string token",
	"bad URL token",
	"\"-->\"",
	"\"<!--\"",
	"\"}\"",
	"\"]\"",
	"\")\"",
	"\":\"",
	"\",\"",
	"delimiter",
	"\"&\"",
	"\"*\"",
	"\"|\"",
	"\"^\"",
	"\"$\"",
	"\".\"",
	"\"=\"",
	"\"!\"",
	"\">\"",
	"\"+\"",
	"\"/\"",
	"\"~\"",
	"dimension",
	"function token",
	"hash token",
	"hash token",
	"identifier",
	"number",
	"\"{\"",
	"\"[\"",
	"\"(\"",
	"percentage",
	"\";\"",
	"string token",
	"URL token",
	"whitespace",
}

func (t T) String() string {
	return tokenToString[t]
}

// This token struct is designed to be memory-efficient. It just references a
// range in the input file instead of directly containing the substring of text
// since a range takes up less memory than a string.
type Token struct {
	Range      logger.Range // 8 bytes
	UnitOffset uint16       // 2 bytes
	Kind       T            // 1 byte
}

func (token Token) DecodedText(contents string) string {
	raw := contents[token.Range.Loc.Start:token.Range.End()]

	switch token.Kind {
	case TIdent, TDimension:
		return decodeEscapesInToken(raw)

	case TAtKeyword, THash, THashID:
		return decodeEscapesInToken(raw[1:])

	case TFunction:
		return decodeEscapesInToken(raw[:len(raw)-1])

	case TString:
		return decodeEscapesInToken(raw[1 : len(raw)-1])

	case TURL:
		start := 4
		end := len(raw) - 1

		// Trim leading and trailing whitespace
		for start < end && isWhitespace(rune(raw[start])) {
			start++
		}
		for start < end && isWhitespace(rune(raw[end-1])) {
			end--
		}

		return decodeEscapesInToken(raw[start:end])
	}

	return raw
}

type lexer struct {
	log       logger.Log
	source    logger.Source
	current   int
	codePoint rune
	Token     Token
}

func Tokenize(log logger.Log, source logger.Source) (tokens []Token) {
	lexer := lexer{
		log:    log,
		source: source,
	}
	lexer.step()
	lexer.next()
	for lexer.Token.Kind != TEndOfFile {
		tokens = append(tokens, lexer.Token)
		lexer.next()
	}
	return
}

func (lexer *lexer) step() {
	codePoint, width := utf8.DecodeRuneInString(lexer.source.Contents[lexer.current:])

	// Use -1 to indicate the end of the file
	if width == 0 {
		codePoint = eof
	}

	lexer.codePoint = codePoint
	lexer.Token.Range.Len = int32(lexer.current) - lexer.Token.Range.Loc.Start
	lexer.current += width
}

func (lexer *lexer) next() {
	// Reference: https://www.w3.org/TR/css-syntax-3/

	for {
		lexer.Token.Range = logger.Range{Loc: logger.Loc{Start: lexer.Token.Range.End()}}

		switch lexer.codePoint {
		case eof:
			lexer.Token.Kind = TEndOfFile

		case '/':
			lexer.step()
			switch lexer.codePoint {
			case '*':
				lexer.step()
				lexer.consumeToEndOfMultiLineComment()
				continue
			case '/':
				lexer.step()
				lexer.consumeToEndOfSingleLineComment()
				continue
			}
			lexer.Token.Kind = TDelimSlash

		case ' ', '\t', '\n', '\r', '\f':
			lexer.step()
			for {
				if isWhitespace(lexer.codePoint) {
					lexer.step()
				} else if lexer.codePoint == '/' && lexer.current < len(lexer.source.Contents) && lexer.source.Contents[lexer.current] == '*' {
					lexer.step()
					lexer.step()
					lexer.consumeToEndOfMultiLineComment()
				} else {
					break
				}
			}
			lexer.Token.Kind = TWhitespace

		case '"', '\'':
			lexer.Token.Kind = lexer.consumeString()

		case '#':
			lexer.step()
			if IsNameContinue(lexer.codePoint) || lexer.isValidEscape() {
				if lexer.wouldStartIdentifier() {
					lexer.Token.Kind = THashID
				} else {
					lexer.Token.Kind = THash
				}
				lexer.consumeName()
			} else {
				lexer.Token.Kind = TDelim
			}

		case '(':
			lexer.step()
			lexer.Token.Kind = TOpenParen

		case ')':
			lexer.step()
			lexer.Token.Kind = TCloseParen

		case '[':
			lexer.step()
			lexer.Token.Kind = TOpenBracket

		case ']':
			lexer.step()
			lexer.Token.Kind = TCloseBracket

		case '{':
			lexer.step()
			lexer.Token.Kind = TOpenBrace

		case '}':
			lexer.step()
			lexer.Token.Kind = TCloseBrace

		case ',':
			lexer.step()
			lexer.Token.Kind = TComma

		case ':':
			lexer.step()
			lexer.Token.Kind = TColon

		case ';':
			lexer.step()
			lexer.Token.Kind = TSemicolon

		case '+':
			if lexer.wouldStartNumber() {
				lexer.Token.Kind = lexer.consumeNumeric()
			} else {
				lexer.step()
				lexer.Token.Kind = TDelimPlus
			}

		case '.':
			if lexer.wouldStartNumber() {
				lexer.Token.Kind = lexer.consumeNumeric()
			} else {
				lexer.step()
				lexer.Token.Kind = TDelimDot
			}

		case '-':
			if lexer.wouldStartNumber() {
				lexer.Token.Kind = lexer.consumeNumeric()
			} else if lexer.current+2 <= len(lexer.source.Contents) && lexer.source.Contents[lexer.current:lexer.current+2] == "->" {
				lexer.step()
				lexer.step()
				lexer.step()
				lexer.Token.Kind = TCDC
			} else if lexer.wouldStartIdentifier() {
				lexer.consumeName()
				lexer.Token.Kind = TIdent
			} else {
				lexer.step()
				lexer.Token.Kind = TDelim
			}

		case '<':
			if lexer.current+3 <= len(lexer.source.Contents) && lexer.source.Contents[lexer.current:lexer.current+3] == "!--" {
				lexer.step()
				lexer.step()
				lexer.step()
				lexer.step()
				lexer.Token.Kind = TCDO
			} else {
				lexer.step()
				lexer.Token.Kind = TDelim
			}

		case '@':
			lexer.step()
			if lexer.wouldStartIdentifier() {
				lexer.consumeName()
				lexer.Token.Kind = TAtKeyword
			} else {
				lexer.Token.Kind = TDelim
			}

		case '\\':
			if lexer.isValidEscape() {
				lexer.Token.Kind = lexer.consumeIdentLike()
			} else {
				lexer.step()
				lexer.log.AddRangeError(&lexer.source, lexer.Token.Range, "Invalid escape")
				lexer.Token.Kind = TDelim
			}

		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			lexer.Token.Kind = lexer.consumeNumeric()

		case '>':
			lexer.step()
			lexer.Token.Kind = TDelimGreaterThan

		case '~':
			lexer.step()
			lexer.Token.Kind = TDelimTilde

		case '&':
			lexer.step()
			lexer.Token.Kind = TDelimAmpersand

		case '*':
			lexer.step()
			lexer.Token.Kind = TDelimAsterisk

		case '|':
			lexer.step()
			lexer.Token.Kind = TDelimBar

		case '!':
			lexer.step()
			lexer.Token.Kind = TDelimExclamation

		case '=':
			lexer.step()
			lexer.Token.Kind = TDelimEquals

		case '^':
			lexer.step()
			lexer.Token.Kind = TDelimCaret

		case '$':
			lexer.step()
			lexer.Token.Kind = TDelimDollar

		default:
			if IsNameStart(lexer.codePoint) {
				lexer.Token.Kind = lexer.consumeIdentLike()
			} else {
				lexer.step()
				lexer.Token.Kind = TDelim
			}
		}

		return
	}
}

func (lexer *lexer) consumeToEndOfMultiLineComment() {
	for {
		switch lexer.codePoint {
		case '*':
			lexer.step()
			if lexer.codePoint == '/' {
				lexer.step()
				return
			}

		case eof: // This indicates the end of the file
			lexer.log.AddError(&lexer.source, logger.Loc{Start: lexer.Token.Range.End()}, "Expected \"*/\" to terminate multi-line comment")
			return

		default:
			lexer.step()
		}
	}
}

func (lexer *lexer) consumeToEndOfSingleLineComment() {
	for !isNewline(lexer.codePoint) && lexer.codePoint != eof {
		lexer.step()
	}
	lexer.log.AddRangeWarning(&lexer.source, lexer.Token.Range, "Comments in CSS use \"/* ... */\" instead of \"//\"")
}

func (lexer *lexer) isValidEscape() bool {
	if lexer.codePoint != '\\' {
		return false
	}
	c, _ := utf8.DecodeRuneInString(lexer.source.Contents[lexer.current:])
	return !isNewline(c)
}

func (lexer *lexer) wouldStartIdentifier() bool {
	if IsNameStart(lexer.codePoint) {
		return true
	}

	if lexer.codePoint == '-' {
		c, w := utf8.DecodeRuneInString(lexer.source.Contents[lexer.current:])
		if IsNameStart(c) || c == '-' {
			return true
		}
		if c == '\\' {
			c, _ = utf8.DecodeRuneInString(lexer.source.Contents[lexer.current+w:])
			return !isNewline(c)
		}
		return false
	}

	return lexer.isValidEscape()
}

func WouldStartIdentifierWithoutEscapes(text string) bool {
	if len(text) > 0 {
		c, width := utf8.DecodeRuneInString(text)
		if IsNameStart(c) {
			return true
		} else if c == '-' {
			if c, _ := utf8.DecodeRuneInString(text[width:]); IsNameStart(c) || c == '-' {
				return true
			}
		}
	}
	return false
}

func (lexer *lexer) wouldStartNumber() bool {
	if lexer.codePoint >= '0' && lexer.codePoint <= '9' {
		return true
	} else if lexer.codePoint == '.' {
		contents := lexer.source.Contents
		if lexer.current < len(contents) {
			c := contents[lexer.current]
			return c >= '0' && c <= '9'
		}
	} else if lexer.codePoint == '+' || lexer.codePoint == '-' {
		contents := lexer.source.Contents
		n := len(contents)
		if lexer.current < n {
			c := contents[lexer.current]
			if c >= '0' && c <= '9' {
				return true
			}
			if c == '.' && lexer.current+1 < n {
				c = contents[lexer.current+1]
				return c >= '0' && c <= '9'
			}
		}
	}
	return false
}

func (lexer *lexer) consumeName() string {
	// Common case: no escapes, identifier is a substring of the input
	for IsNameContinue(lexer.codePoint) {
		lexer.step()
	}
	raw := lexer.source.Contents[lexer.Token.Range.Loc.Start:lexer.Token.Range.End()]
	if !lexer.isValidEscape() {
		return raw
	}

	// Uncommon case: escapes, identifier is allocated
	sb := strings.Builder{}
	sb.WriteString(raw)
	sb.WriteRune(lexer.consumeEscape())
	for {
		if IsNameContinue(lexer.codePoint) {
			sb.WriteRune(lexer.codePoint)
			lexer.step()
		} else if lexer.isValidEscape() {
			sb.WriteRune(lexer.consumeEscape())
		} else {
			break
		}
	}
	return sb.String()
}

func (lexer *lexer) consumeEscape() rune {
	lexer.step() // Skip the backslash
	c := lexer.codePoint

	if hex, ok := isHex(c); ok {
		lexer.step()
		for i := 0; i < 5; i++ {
			if next, ok := isHex(lexer.codePoint); ok {
				lexer.step()
				hex = hex*16 + next
			} else {
				break
			}
		}
		if isWhitespace(lexer.codePoint) {
			lexer.step()
		}
		if hex == 0 || (hex >= 0xD800 && hex <= 0xDFFF) || hex > 0x10FFFF {
			return replacementCharacter
		}
		return rune(hex)
	}

	if c == eof {
		return replacementCharacter
	}

	lexer.step()
	return c
}

func (lexer *lexer) consumeIdentLike() T {
	name := lexer.consumeName()

	if lexer.codePoint == '(' {
		lexer.step()
		if len(name) == 3 {
			u, r, l := name[0], name[1], name[2]
			if (u == 'u' || u == 'U') && (r == 'r' || r == 'R') && (l == 'l' || l == 'L') {
				for isWhitespace(lexer.codePoint) {
					lexer.step()
				}
				if lexer.codePoint != '"' && lexer.codePoint != '\'' {
					return lexer.consumeURL()
				}
			}
		}
		return TFunction
	}

	return TIdent
}

func (lexer *lexer) consumeURL() T {
validURL:
	for {
		switch lexer.codePoint {
		case ')':
			lexer.step()
			return TURL

		case eof:
			loc := logger.Loc{Start: lexer.Token.Range.End()}
			lexer.log.AddError(&lexer.source, loc, "Expected \")\" to end URL token")
			return TBadURL

		case ' ', '\t', '\n', '\r', '\f':
			lexer.step()
			for isWhitespace(lexer.codePoint) {
				lexer.step()
			}
			if lexer.codePoint != ')' {
				loc := logger.Loc{Start: lexer.Token.Range.End()}
				lexer.log.AddError(&lexer.source, loc, "Expected \")\" to end URL token")
				break validURL
			}
			lexer.step()
			return TURL

		case '"', '\'', '(':
			r := logger.Range{Loc: logger.Loc{Start: lexer.Token.Range.End()}, Len: 1}
			lexer.log.AddRangeError(&lexer.source, r, "Expected \")\" to end URL token")
			break validURL

		case '\\':
			if !lexer.isValidEscape() {
				r := logger.Range{Loc: logger.Loc{Start: lexer.Token.Range.End()}, Len: 1}
				lexer.log.AddRangeError(&lexer.source, r, "Invalid escape")
				break validURL
			}
			lexer.consumeEscape()

		default:
			if isNonPrintable(lexer.codePoint) {
				r := logger.Range{Loc: logger.Loc{Start: lexer.Token.Range.End()}, Len: 1}
				lexer.log.AddRangeError(&lexer.source, r, "Unexpected non-printable character in URL token")
			}
			lexer.step()
		}
	}

	// Consume the remnants of a bad url
	for {
		switch lexer.codePoint {
		case ')', eof:
			lexer.step()
			return TBadURL

		case '\\':
			if lexer.isValidEscape() {
				lexer.consumeEscape()
			}
		}
		lexer.step()
	}
}

func (lexer *lexer) consumeString() T {
	quote := lexer.codePoint
	lexer.step()

	for {
		switch lexer.codePoint {
		case '\\':
			lexer.step()

			// Handle Windows CRLF
			if lexer.codePoint == '\r' {
				lexer.step()
				if lexer.codePoint == '\n' {
					lexer.step()
				}
				continue
			}

			// Otherwise, fall through to ignore the character after the backslash

		case eof:
			lexer.log.AddError(&lexer.source, logger.Loc{Start: lexer.Token.Range.End()}, "Unterminated string token")
			return TBadString

		case '\n', '\r', '\f':
			lexer.log.AddError(&lexer.source, logger.Loc{Start: lexer.Token.Range.End()}, "Unterminated string token")
			return TBadString

		case quote:
			lexer.step()
			return TString
		}
		lexer.step()
	}
}

func (lexer *lexer) consumeNumeric() T {
	// Skip over leading sign
	if lexer.codePoint == '+' || lexer.codePoint == '-' {
		lexer.step()
	}

	// Skip over leading digits
	for lexer.codePoint >= '0' && lexer.codePoint <= '9' {
		lexer.step()
	}

	// Skip over digits after dot
	if lexer.codePoint == '.' {
		lexer.step()
		for lexer.codePoint >= '0' && lexer.codePoint <= '9' {
			lexer.step()
		}
	}

	// Skip over exponent
	if lexer.codePoint == 'e' || lexer.codePoint == 'E' {
		contents := lexer.source.Contents

		// Look ahead before advancing to make sure this is an exponent, not a unit
		if lexer.current < len(contents) {
			c := contents[lexer.current]
			if (c == '+' || c == '-') && lexer.current+1 < len(contents) {
				c = contents[lexer.current+1]
			}

			// Only consume this if it's an exponent
			if c >= '0' && c <= '9' {
				lexer.step()
				if lexer.codePoint == '+' || lexer.codePoint == '-' {
					lexer.step()
				}
				for lexer.codePoint >= '0' && lexer.codePoint <= '9' {
					lexer.step()
				}
			}
		}
	}

	// Determine the numeric type
	if lexer.wouldStartIdentifier() {
		lexer.Token.UnitOffset = uint16(lexer.Token.Range.Len)
		lexer.consumeName()
		return TDimension
	}
	if lexer.codePoint == '%' {
		lexer.step()
		return TPercentage
	}
	return TNumber
}

func IsNameStart(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c >= 0x80
}

func IsNameContinue(c rune) bool {
	return IsNameStart(c) || (c >= '0' && c <= '9') || c == '-'
}

func isNewline(c rune) bool {
	switch c {
	case '\n', '\r', '\f':
		return true
	}
	return false
}

func isWhitespace(c rune) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	}
	return false
}

func isHex(c rune) (int, bool) {
	if c >= '0' && c <= '9' {
		return int(c - '0'), true
	}
	if c >= 'a' && c <= 'f' {
		return int(c + (10 - 'a')), true
	}
	if c >= 'A' && c <= 'F' {
		return int(c + (10 - 'A')), true
	}
	return 0, false
}

func isNonPrintable(c rune) bool {
	return c <= 0x08 || c == 0x0B || (c >= 0x0E && c <= 0x1F) || c == 0x7F
}

func decodeEscapesInToken(inner string) string {
	i := 0

	for i < len(inner) {
		if inner[i] == '\\' {
			break
		}
		i++
	}

	if i == len(inner) {
		return inner
	}

	sb := strings.Builder{}
	sb.WriteString(inner[:i])
	inner = inner[i:]

	for len(inner) > 0 {
		c, width := utf8.DecodeRuneInString(inner)
		inner = inner[width:]

		if c != '\\' {
			sb.WriteRune(c)
			continue
		}

		if len(inner) == 0 {
			sb.WriteRune(replacementCharacter)
			continue
		}

		c, width = utf8.DecodeRuneInString(inner)
		inner = inner[width:]
		hex, ok := isHex(c)

		if !ok {
			if c == '\n' || c == '\f' {
				continue
			}

			// Handle Windows CRLF
			if c == '\r' {
				c, width = utf8.DecodeRuneInString(inner)
				if c == '\n' {
					inner = inner[width:]
				}
				continue
			}

			// If we get here, this is not a valid escape. However, this is still
			// allowed. In this case the backslash is just ignored.
			sb.WriteRune(c)
			continue
		}

		// Parse up to five additional hex characters (so six in total)
		for i := 0; i < 5 && len(inner) > 0; i++ {
			c, width = utf8.DecodeRuneInString(inner)
			if next, ok := isHex(c); ok {
				inner = inner[width:]
				hex = hex*16 + next
			} else {
				break
			}
		}

		if len(inner) > 0 {
			c, width = utf8.DecodeRuneInString(inner)
			if isWhitespace(c) {
				inner = inner[width:]
			}
		}

		if hex == 0 || (hex >= 0xD800 && hex <= 0xDFFF) || hex > 0x10FFFF {
			sb.WriteRune(replacementCharacter)
			continue
		}

		sb.WriteRune(rune(hex))
	}

	return sb.String()
}
