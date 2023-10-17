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

const (
	TEndOfFile T = iota

	TAtKeyword
	TUnterminatedString
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
	TDelimMinus
	TDelimPlus
	TDelimSlash
	TDelimTilde
	TDimension
	TFunction
	THash
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

	// This is never something that the lexer generates directly. Instead this is
	// an esbuild-specific token for global/local names that "TIdent" tokens may
	// be changed into.
	TSymbol
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
	"\"-\"",
	"\"+\"",
	"\"/\"",
	"\"~\"",
	"dimension",
	"function token",
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

	"identifier",
}

func (t T) String() string {
	return tokenToString[t]
}

func (t T) IsNumeric() bool {
	return t == TNumber || t == TPercentage || t == TDimension
}

type TokenFlags uint8

const (
	IsID TokenFlags = 1 << iota
	DidWarnAboutSingleLineComment
)

// This token struct is designed to be memory-efficient. It just references a
// range in the input file instead of directly containing the substring of text
// since a range takes up less memory than a string.
type Token struct {
	Range      logger.Range // 8 bytes
	UnitOffset uint16       // 2 bytes
	Kind       T            // 1 byte
	Flags      TokenFlags   // 1 byte
}

func (token Token) DecodedText(contents string) string {
	raw := contents[token.Range.Loc.Start:token.Range.End()]

	switch token.Kind {
	case TIdent, TDimension:
		return decodeEscapesInToken(raw)

	case TAtKeyword, THash:
		return decodeEscapesInToken(raw[1:])

	case TFunction:
		return decodeEscapesInToken(raw[:len(raw)-1])

	case TString:
		return decodeEscapesInToken(raw[1 : len(raw)-1])

	case TURL:
		start := 4
		end := len(raw)

		// Note: URL tokens with syntax errors may not have a trailing ")"
		if raw[end-1] == ')' {
			end--
		}

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
	Options
	log                     logger.Log
	source                  logger.Source
	allComments             []logger.Range
	legalCommentsBefore     []Comment
	sourceMappingURL        logger.Span
	tracker                 logger.LineColumnTracker
	approximateNewlineCount int
	current                 int
	oldSingleLineCommentEnd logger.Loc
	codePoint               rune
	Token                   Token
}

type Comment struct {
	Text            string
	Loc             logger.Loc
	TokenIndexAfter uint32
}

type TokenizeResult struct {
	Tokens               []Token
	AllComments          []logger.Range
	LegalComments        []Comment
	SourceMapComment     logger.Span
	ApproximateLineCount int32
}

type Options struct {
	RecordAllComments bool
}

func Tokenize(log logger.Log, source logger.Source, options Options) TokenizeResult {
	lexer := lexer{
		Options: options,
		log:     log,
		source:  source,
		tracker: logger.MakeLineColumnTracker(&source),
	}
	lexer.step()

	// The U+FEFF character is usually a zero-width non-breaking space. However,
	// when it's used at the start of a text stream it is called a BOM (byte order
	// mark) instead and indicates that the text stream is UTF-8 encoded. This is
	// problematic for us because CSS does not treat U+FEFF as whitespace. Only
	// " \t\r\n\f" characters are treated as whitespace. Skip over the BOM if it
	// is present so it doesn't cause us trouble when we try to parse it.
	if lexer.codePoint == '\uFEFF' {
		lexer.step()
	}

	lexer.next()
	var tokens []Token
	var legalComments []Comment
	for lexer.Token.Kind != TEndOfFile {
		if lexer.legalCommentsBefore != nil {
			for _, comment := range lexer.legalCommentsBefore {
				comment.TokenIndexAfter = uint32(len(tokens))
				legalComments = append(legalComments, comment)
			}
			lexer.legalCommentsBefore = nil
		}
		tokens = append(tokens, lexer.Token)
		lexer.next()
	}
	if lexer.legalCommentsBefore != nil {
		for _, comment := range lexer.legalCommentsBefore {
			comment.TokenIndexAfter = uint32(len(tokens))
			legalComments = append(legalComments, comment)
		}
		lexer.legalCommentsBefore = nil
	}
	return TokenizeResult{
		Tokens:               tokens,
		AllComments:          lexer.allComments,
		LegalComments:        legalComments,
		ApproximateLineCount: int32(lexer.approximateNewlineCount) + 1,
		SourceMapComment:     lexer.sourceMappingURL,
	}
}

func (lexer *lexer) step() {
	codePoint, width := utf8.DecodeRuneInString(lexer.source.Contents[lexer.current:])

	// Use -1 to indicate the end of the file
	if width == 0 {
		codePoint = eof
	}

	// Track the approximate number of newlines in the file so we can preallocate
	// the line offset table in the printer for source maps. The line offset table
	// is the #1 highest allocation in the heap profile, so this is worth doing.
	// This count is approximate because it handles "\n" and "\r\n" (the common
	// cases) but not "\r" or "\u2028" or "\u2029". Getting this wrong is harmless
	// because it's only a preallocation. The array will just grow if it's too small.
	if codePoint == '\n' {
		lexer.approximateNewlineCount++
	}

	lexer.codePoint = codePoint
	lexer.Token.Range.Len = int32(lexer.current) - lexer.Token.Range.Loc.Start
	lexer.current += width
}

func (lexer *lexer) next() {
	// Reference: https://www.w3.org/TR/css-syntax-3/

	for {
		lexer.Token = Token{Range: logger.Range{Loc: logger.Loc{Start: lexer.Token.Range.End()}}}

		switch lexer.codePoint {
		case eof:
			lexer.Token.Kind = TEndOfFile

		case '/':
			lexer.step()
			switch lexer.codePoint {
			case '*':
				lexer.step()
				lexer.consumeToEndOfMultiLineComment(lexer.Token.Range)
				continue
			case '/':
				// Warn when people use "//" comments, which are invalid in CSS
				loc := lexer.Token.Range.Loc
				if loc.Start >= lexer.oldSingleLineCommentEnd.Start {
					contents := lexer.source.Contents
					end := lexer.current
					for end < len(contents) && !isNewline(rune(contents[end])) {
						end++
					}
					lexer.log.AddID(logger.MsgID_CSS_JSCommentInCSS, logger.Warning, &lexer.tracker, logger.Range{Loc: loc, Len: 2},
						"Comments in CSS use \"/* ... */\" instead of \"//\"")
					lexer.oldSingleLineCommentEnd.Start = int32(end)
					lexer.Token.Flags |= DidWarnAboutSingleLineComment
				}
			}
			lexer.Token.Kind = TDelimSlash

		case ' ', '\t', '\n', '\r', '\f':
			lexer.step()
			for {
				if isWhitespace(lexer.codePoint) {
					lexer.step()
				} else if lexer.codePoint == '/' && lexer.current < len(lexer.source.Contents) && lexer.source.Contents[lexer.current] == '*' {
					startRange := logger.Range{Loc: logger.Loc{Start: lexer.Token.Range.End()}, Len: 2}
					lexer.step()
					lexer.step()
					lexer.consumeToEndOfMultiLineComment(startRange)
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
				lexer.Token.Kind = THash
				if lexer.wouldStartIdentifier() {
					lexer.Token.Flags |= IsID
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
				lexer.Token.Kind = lexer.consumeIdentLike()
			} else {
				lexer.step()
				lexer.Token.Kind = TDelimMinus
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
				lexer.log.AddError(&lexer.tracker, lexer.Token.Range, "Invalid escape")
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

func (lexer *lexer) consumeToEndOfMultiLineComment(startRange logger.Range) {
	startOfSourceMappingURL := 0
	isLegalComment := false

	switch lexer.codePoint {
	case '#', '@':
		// Keep track of the contents of the "sourceMappingURL=" comment
		if strings.HasPrefix(lexer.source.Contents[lexer.current:], " sourceMappingURL=") {
			startOfSourceMappingURL = lexer.current + len(" sourceMappingURL=")
		}

	case '!':
		// Remember if this is a legal comment
		isLegalComment = true
	}

	for {
		switch lexer.codePoint {
		case '*':
			endOfSourceMappingURL := lexer.current - 1
			lexer.step()
			if lexer.codePoint == '/' {
				commentEnd := lexer.current
				lexer.step()

				// Record the source mapping URL
				if startOfSourceMappingURL != 0 {
					r := logger.Range{Loc: logger.Loc{Start: int32(startOfSourceMappingURL)}}
					text := lexer.source.Contents[startOfSourceMappingURL:endOfSourceMappingURL]
					for int(r.Len) < len(text) && !isWhitespace(rune(text[r.Len])) {
						r.Len++
					}
					lexer.sourceMappingURL = logger.Span{Text: text[:r.Len], Range: r}
				}

				// Record all comments
				commentRange := logger.Range{Loc: startRange.Loc, Len: int32(commentEnd) - startRange.Loc.Start}
				if lexer.RecordAllComments {
					lexer.allComments = append(lexer.allComments, commentRange)
				}

				// Record legal comments
				if text := lexer.source.Contents[startRange.Loc.Start:commentEnd]; isLegalComment || containsAtPreserveOrAtLicense(text) {
					text = lexer.source.CommentTextWithoutIndent(commentRange)
					lexer.legalCommentsBefore = append(lexer.legalCommentsBefore, Comment{Loc: startRange.Loc, Text: text})
				}
				return
			}

		case eof: // This indicates the end of the file
			lexer.log.AddErrorWithNotes(&lexer.tracker, logger.Range{Loc: logger.Loc{Start: lexer.Token.Range.End()}},
				"Expected \"*/\" to terminate multi-line comment",
				[]logger.MsgData{lexer.tracker.MsgData(startRange, "The multi-line comment starts here:")})
			return

		default:
			lexer.step()
		}
	}
}

func containsAtPreserveOrAtLicense(text string) bool {
	for i, c := range text {
		if c == '@' && (strings.HasPrefix(text[i+1:], "preserve") || strings.HasPrefix(text[i+1:], "license")) {
			return true
		}
	}
	return false
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
		c, width := utf8.DecodeRuneInString(lexer.source.Contents[lexer.current:])
		if c == utf8.RuneError && width <= 1 {
			return false // Decoding error
		}
		if IsNameStart(c) || c == '-' {
			return true
		}
		if c == '\\' {
			c2, _ := utf8.DecodeRuneInString(lexer.source.Contents[lexer.current+width:])
			return !isNewline(c2)
		}
		return false
	}

	return lexer.isValidEscape()
}

func WouldStartIdentifierWithoutEscapes(text string) bool {
	c, width := utf8.DecodeRuneInString(text)
	if c == utf8.RuneError && width <= 1 {
		return false // Decoding error
	}
	if IsNameStart(c) {
		return true
	}

	if c == '-' {
		c2, width2 := utf8.DecodeRuneInString(text[width:])
		if c2 == utf8.RuneError && width2 <= 1 {
			return false // Decoding error
		}
		if IsNameStart(c2) || c2 == '-' {
			return true
		}
	}
	return false
}

func RangeOfIdentifier(source logger.Source, loc logger.Loc) logger.Range {
	text := source.Contents[loc.Start:]
	if len(text) == 0 {
		return logger.Range{Loc: loc, Len: 0}
	}

	i := 0
	n := len(text)

	for {
		c, width := utf8.DecodeRuneInString(text[i:])
		if IsNameContinue(c) {
			i += width
			continue
		}

		// Handle an escape
		if c == '\\' && i+1 < n && !isNewline(rune(text[i+1])) {
			i += width // Skip the backslash
			c, width = utf8.DecodeRuneInString(text[i:])
			if _, ok := isHex(c); ok {
				i += width
				c, width = utf8.DecodeRuneInString(text[i:])
				for j := 0; j < 5; j++ {
					if _, ok := isHex(c); !ok {
						break
					}
					i += width
					c, width = utf8.DecodeRuneInString(text[i:])
				}
				if isWhitespace(c) {
					i += width
				}
			}
			continue
		}

		break
	}

	// Don't end with a whitespace
	if i > 0 && isWhitespace(rune(text[i-1])) {
		i--
	}

	return logger.Range{Loc: loc, Len: int32(i)}
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

// Note: This function is hot in profiles
func (lexer *lexer) consumeName() string {
	// Common case: no escapes, identifier is a substring of the input. Doing this
	// in a tight loop that avoids UTF-8 decoding and that increments a single
	// number instead of doing "step()" is noticeably faster. For example, doing
	// this sped up end-to-end parsing and printing of a large CSS file from 97ms
	// to 84ms (around 15% faster).
	contents := lexer.source.Contents
	if IsNameContinue(lexer.codePoint) {
		n := len(contents)
		i := lexer.current
		for i < n && IsNameContinue(rune(contents[i])) {
			i++
		}
		lexer.current = i
		lexer.step()
	}
	raw := contents[lexer.Token.Range.Loc.Start:lexer.Token.Range.End()]
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
			return utf8.RuneError
		}
		return rune(hex)
	}

	if c == eof {
		return utf8.RuneError
	}

	lexer.step()
	return c
}

func (lexer *lexer) consumeIdentLike() T {
	name := lexer.consumeName()

	if lexer.codePoint == '(' {
		matchingLoc := logger.Loc{Start: lexer.Token.Range.End()}
		lexer.step()
		if len(name) == 3 {
			u, r, l := name[0], name[1], name[2]
			if (u == 'u' || u == 'U') && (r == 'r' || r == 'R') && (l == 'l' || l == 'L') {
				// Save state
				approximateNewlineCount := lexer.approximateNewlineCount
				codePoint := lexer.codePoint
				tokenRangeLen := lexer.Token.Range.Len
				current := lexer.current

				// Check to see if this is a URL token instead of a function
				for isWhitespace(lexer.codePoint) {
					lexer.step()
				}
				if lexer.codePoint != '"' && lexer.codePoint != '\'' {
					return lexer.consumeURL(matchingLoc)
				}

				// Restore state (i.e. backtrack)
				lexer.approximateNewlineCount = approximateNewlineCount
				lexer.codePoint = codePoint
				lexer.Token.Range.Len = tokenRangeLen
				lexer.current = current
			}
		}
		return TFunction
	}

	return TIdent
}

func (lexer *lexer) consumeURL(matchingLoc logger.Loc) T {
validURL:
	for {
		switch lexer.codePoint {
		case ')':
			lexer.step()
			return TURL

		case eof:
			loc := logger.Loc{Start: lexer.Token.Range.End()}
			lexer.log.AddIDWithNotes(logger.MsgID_CSS_CSSSyntaxError, logger.Warning, &lexer.tracker, logger.Range{Loc: loc}, "Expected \")\" to end URL token",
				[]logger.MsgData{lexer.tracker.MsgData(logger.Range{Loc: matchingLoc, Len: 1}, "The unbalanced \"(\" is here:")})
			return TURL

		case ' ', '\t', '\n', '\r', '\f':
			lexer.step()
			for isWhitespace(lexer.codePoint) {
				lexer.step()
			}
			if lexer.codePoint != ')' {
				loc := logger.Loc{Start: lexer.Token.Range.End()}
				lexer.log.AddIDWithNotes(logger.MsgID_CSS_CSSSyntaxError, logger.Warning, &lexer.tracker, logger.Range{Loc: loc}, "Expected \")\" to end URL token",
					[]logger.MsgData{lexer.tracker.MsgData(logger.Range{Loc: matchingLoc, Len: 1}, "The unbalanced \"(\" is here:")})
				if lexer.codePoint == eof {
					return TURL
				}
				break validURL
			}
			lexer.step()
			return TURL

		case '"', '\'', '(':
			r := logger.Range{Loc: logger.Loc{Start: lexer.Token.Range.End()}, Len: 1}
			lexer.log.AddIDWithNotes(logger.MsgID_CSS_CSSSyntaxError, logger.Warning, &lexer.tracker, r, "Expected \")\" to end URL token",
				[]logger.MsgData{lexer.tracker.MsgData(logger.Range{Loc: matchingLoc, Len: 1}, "The unbalanced \"(\" is here:")})
			break validURL

		case '\\':
			if !lexer.isValidEscape() {
				r := logger.Range{Loc: logger.Loc{Start: lexer.Token.Range.End()}, Len: 1}
				lexer.log.AddID(logger.MsgID_CSS_CSSSyntaxError, logger.Warning, &lexer.tracker, r, "Invalid escape")
				break validURL
			}
			lexer.consumeEscape()

		default:
			if isNonPrintable(lexer.codePoint) {
				r := logger.Range{Loc: logger.Loc{Start: lexer.Token.Range.End()}, Len: 1}
				lexer.log.AddID(logger.MsgID_CSS_CSSSyntaxError, logger.Warning, &lexer.tracker, r, "Unexpected non-printable character in URL token")
				break validURL
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

		case eof, '\n', '\r', '\f':
			lexer.log.AddID(logger.MsgID_CSS_CSSSyntaxError, logger.Warning, &lexer.tracker,
				logger.Range{Loc: logger.Loc{Start: lexer.Token.Range.End()}},
				"Unterminated string token")
			return TUnterminatedString

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
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c >= 0x80 || c == '\x00'
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
		if c := inner[i]; c == '\\' || c == '\x00' {
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
			if c == '\x00' {
				c = utf8.RuneError
			}
			sb.WriteRune(c)
			continue
		}

		if len(inner) == 0 {
			sb.WriteRune(utf8.RuneError)
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
			sb.WriteRune(utf8.RuneError)
			continue
		}

		sb.WriteRune(rune(hex))
	}

	return sb.String()
}
