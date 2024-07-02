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
	"unicode/utf8"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/logger"
)

type T uint8

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

	// Assignments (keep in sync with IsAssign() below)
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

func (t T) IsAssign() bool {
	return t >= TAmpersandAmpersandEquals && t <= TSlashEquals
}

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

// This represents a string that is maybe a substring of the current file's
// "source.Contents" string. The point of doing this is that if it is a
// substring (the common case), then we can represent it more efficiently.
//
// For compactness and performance, the JS AST represents identifiers as a
// symbol reference instead of as a string. However, we need to track the
// string between the first pass and the second pass because the string is only
// resolved to a symbol in the second pass. To avoid allocating extra memory
// to store the string, we instead use an index+length slice of the original JS
// source code. That index is what "Start" represents here. The length is just
// "len(String)".
//
// Set "Start" to invalid (the zero value) if "String" is not a substring of
// "source.Contents". This is the case for escaped identifiers. For example,
// the identifier "fo\u006f" would be "MaybeSubstring{String: "foo"}". It's
// critical that any code changing the "String" also set "Start" to the zero
// value, which is best done by just overwriting the whole "MaybeSubstring".
//
// The substring range used to be recovered automatically from the string but
// that relied on the Go "unsafe" package which can hypothetically break under
// certain Go compiler optimization passes, so it has been removed and replaced
// with this more error-prone approach that doesn't use "unsafe".
type MaybeSubstring struct {
	String string
	Start  ast.Index32
}

type Lexer struct {
	LegalCommentsBeforeToken     []logger.Range
	CommentsBeforeToken          []logger.Range
	AllComments                  []logger.Range
	Identifier                   MaybeSubstring
	log                          logger.Log
	source                       logger.Source
	JSXFactoryPragmaComment      logger.Span
	JSXFragmentPragmaComment     logger.Span
	JSXRuntimePragmaComment      logger.Span
	JSXImportSourcePragmaComment logger.Span
	SourceMappingURL             logger.Span
	BadArrowInTSXSuggestion      string

	// Escape sequences in string literals are decoded lazily because they are
	// not interpreted inside tagged templates, and tagged templates can contain
	// invalid escape sequences. If the decoded array is nil, the encoded value
	// should be passed to "tryToDecodeEscapeSequences" first.
	decodedStringLiteralOrNil []uint16
	encodedStringLiteralText  string

	errorSuffix string
	tracker     logger.LineColumnTracker

	encodedStringLiteralStart int

	Number                          float64
	current                         int
	start                           int
	end                             int
	ApproximateNewlineCount         int
	CouldBeBadArrowInTSX            int
	BadArrowInTSXRange              logger.Range
	LegacyOctalLoc                  logger.Loc
	AwaitKeywordLoc                 logger.Loc
	FnOrArrowStartLoc               logger.Loc
	PreviousBackslashQuoteInJSX     logger.Range
	LegacyHTMLCommentRange          logger.Range
	codePoint                       rune
	prevErrorLoc                    logger.Loc
	json                            JSONFlavor
	Token                           T
	ts                              config.TSOptions
	HasNewlineBefore                bool
	HasCommentBefore                CommentBefore
	IsLegacyOctalLiteral            bool
	PrevTokenWasAwaitKeyword        bool
	rescanCloseBraceAsTemplateToken bool
	forGlobalName                   bool

	// The log is disabled during speculative scans that may backtrack
	IsLogDisabled bool
}

type CommentBefore uint8

const (
	PureCommentBefore CommentBefore = 1 << iota
	KeyCommentBefore
	NoSideEffectsCommentBefore
)

type LexerPanic struct{}

func NewLexer(log logger.Log, source logger.Source, ts config.TSOptions) Lexer {
	lexer := Lexer{
		log:               log,
		source:            source,
		tracker:           logger.MakeLineColumnTracker(&source),
		prevErrorLoc:      logger.Loc{Start: -1},
		FnOrArrowStartLoc: logger.Loc{Start: -1},
		ts:                ts,
		json:              NotJSON,
	}
	lexer.step()
	lexer.Next()
	return lexer
}

func NewLexerGlobalName(log logger.Log, source logger.Source) Lexer {
	lexer := Lexer{
		log:               log,
		source:            source,
		tracker:           logger.MakeLineColumnTracker(&source),
		prevErrorLoc:      logger.Loc{Start: -1},
		FnOrArrowStartLoc: logger.Loc{Start: -1},
		forGlobalName:     true,
		json:              NotJSON,
	}
	lexer.step()
	lexer.Next()
	return lexer
}

type JSONFlavor uint8

const (
	// Specification: https://json.org/
	JSON JSONFlavor = iota

	// TypeScript's JSON superset is not documented but appears to allow:
	// - Comments: https://github.com/microsoft/TypeScript/issues/4987
	// - Trailing commas
	// - Full JS number syntax
	TSConfigJSON

	// This is used by the JavaScript lexer
	NotJSON
)

func NewLexerJSON(log logger.Log, source logger.Source, json JSONFlavor, errorSuffix string) Lexer {
	lexer := Lexer{
		log:               log,
		source:            source,
		tracker:           logger.MakeLineColumnTracker(&source),
		prevErrorLoc:      logger.Loc{Start: -1},
		FnOrArrowStartLoc: logger.Loc{Start: -1},
		errorSuffix:       errorSuffix,
		json:              json,
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

func (lexer *Lexer) rawIdentifier() MaybeSubstring {
	return MaybeSubstring{lexer.Raw(), ast.MakeIndex32(uint32(lexer.start))}
}

func (lexer *Lexer) StringLiteral() []uint16 {
	if lexer.decodedStringLiteralOrNil == nil {
		// Lazily decode escape sequences if needed
		if decoded, ok, end := lexer.tryToDecodeEscapeSequences(lexer.encodedStringLiteralStart, lexer.encodedStringLiteralText, true /* reportErrors */); !ok {
			lexer.end = end
			lexer.SyntaxError()
		} else {
			lexer.decodedStringLiteralOrNil = decoded
		}
	}
	return lexer.decodedStringLiteralOrNil
}

func (lexer *Lexer) CookedAndRawTemplateContents() ([]uint16, string) {
	var raw string

	switch lexer.Token {
	case TNoSubstitutionTemplateLiteral, TTemplateTail:
		// "`x`" or "}x`"
		raw = lexer.source.Contents[lexer.start+1 : lexer.end-1]

	case TTemplateHead, TTemplateMiddle:
		// "`x${" or "}x${"
		raw = lexer.source.Contents[lexer.start+1 : lexer.end-2]
	}

	if strings.IndexByte(raw, '\r') != -1 {
		// From the specification:
		//
		// 11.8.6.1 Static Semantics: TV and TRV
		//
		// TV excludes the code units of LineContinuation while TRV includes
		// them. <CR><LF> and <CR> LineTerminatorSequences are normalized to
		// <LF> for both TV and TRV. An explicit EscapeSequence is needed to
		// include a <CR> or <CR><LF> sequence.

		bytes := []byte(raw)
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

		raw = string(bytes[:end])
	}

	// This will return nil on failure, which will become "undefined" for the tag
	cooked, _, _ := lexer.tryToDecodeEscapeSequences(lexer.start+1, raw, false /* reportErrors */)
	return cooked, raw
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
	lexer.addRangeError(logger.Range{Loc: loc}, message)
	panic(LexerPanic{})
}

func (lexer *Lexer) ExpectedString(text string) {
	// Provide a friendly error message about "await" without "async"
	if lexer.PrevTokenWasAwaitKeyword {
		var notes []logger.MsgData
		if lexer.FnOrArrowStartLoc.Start != -1 {
			note := lexer.tracker.MsgData(logger.Range{Loc: lexer.FnOrArrowStartLoc},
				"Consider adding the \"async\" keyword here:")
			note.Location.Suggestion = "async"
			notes = []logger.MsgData{note}
		}
		lexer.AddRangeErrorWithNotes(RangeOfIdentifier(lexer.source, lexer.AwaitKeywordLoc),
			"\"await\" can only be used inside an \"async\" function",
			notes)
		panic(LexerPanic{})
	}

	found := fmt.Sprintf("%q", lexer.Raw())
	if lexer.start == len(lexer.source.Contents) {
		found = "end of file"
	}

	suggestion := ""
	if strings.HasPrefix(text, "\"") && strings.HasSuffix(text, "\"") {
		suggestion = text[1 : len(text)-1]
	}

	lexer.addRangeErrorWithSuggestion(lexer.Range(), fmt.Sprintf("Expected %s%s but found %s", text, lexer.errorSuffix, found), suggestion)
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
	lexer.addRangeError(lexer.Range(), fmt.Sprintf("Unexpected %s%s", found, lexer.errorSuffix))
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
		lexer.maybeExpandEquals()

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
		lexer.maybeExpandEquals()

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

func (lexer *Lexer) maybeExpandEquals() {
	switch lexer.codePoint {
	case '>':
		// "=" + ">" = "=>"
		lexer.Token = TEqualsGreaterThan
		lexer.step()

	case '=':
		// "=" + "=" = "=="
		lexer.Token = TEqualsEquals
		lexer.step()

		if lexer.Token == '=' {
			// "=" + "==" = "==="
			lexer.Token = TEqualsEqualsEquals
			lexer.step()
		}
	}
}

func RangeOfIdentifier(source logger.Source, loc logger.Loc) logger.Range {
	text := source.Contents[loc.Start:]
	if len(text) == 0 {
		return logger.Range{Loc: loc, Len: 0}
	}

	i := 0
	c, _ := utf8.DecodeRuneInString(text[i:])

	// Handle private names
	if c == '#' {
		i++
		c, _ = utf8.DecodeRuneInString(text[i:])
	}

	if js_ast.IsIdentifierStart(c) || c == '\\' {
		// Search for the end of the identifier
		for i < len(text) {
			c2, width2 := utf8.DecodeRuneInString(text[i:])
			if c2 == '\\' {
				i += width2

				// Skip over bracketed unicode escapes such as "\u{10000}"
				if i+2 < len(text) && text[i] == 'u' && text[i+1] == '{' {
					i += 2
					for i < len(text) {
						if text[i] == '}' {
							i++
							break
						}
						i++
					}
				}
			} else if !js_ast.IsIdentifierContinue(c2) {
				return logger.Range{Loc: loc, Len: int32(i)}
			} else {
				i += width2
			}
		}
	}

	// When minifying, this identifier may have originally been a string
	return source.RangeOfString(loc)
}

type KeyOrValue uint8

const (
	KeyRange KeyOrValue = iota
	ValueRange
	KeyAndValueRange
)

func RangeOfImportAssertOrWith(source logger.Source, assertOrWith ast.AssertOrWithEntry, which KeyOrValue) logger.Range {
	if which == KeyRange {
		return RangeOfIdentifier(source, assertOrWith.KeyLoc)
	}
	if which == ValueRange {
		return source.RangeOfString(assertOrWith.ValueLoc)
	}
	loc := RangeOfIdentifier(source, assertOrWith.KeyLoc).Loc
	return logger.Range{Loc: loc, Len: source.RangeOfString(assertOrWith.ValueLoc).End() - loc.Start}
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
				case -1, '{', '<':
					// Stop when the string ends
					break stringLiteral

				case '&', '\r', '\n', '\u2028', '\u2029':
					// This needs fixing if it has an entity or if it's a multi-line string
					needsFixing = true
					lexer.step()

				case '}', '>':
					// These technically aren't valid JSX: https://facebook.github.io/jsx/
					//
					//   JSXTextCharacter :
					//     * SourceCharacter but not one of {, <, > or }
					//
					var replacement string
					if lexer.codePoint == '}' {
						replacement = "{'}'}"
					} else {
						replacement = "{'>'}"
					}
					msg := logger.Msg{
						Kind: logger.Error,
						Data: lexer.tracker.MsgData(logger.Range{Loc: logger.Loc{Start: int32(lexer.end)}, Len: 1},
							fmt.Sprintf("The character \"%c\" is not valid inside a JSX element", lexer.codePoint)),
					}

					// Attempt to provide a better error message if this looks like an arrow function
					if lexer.CouldBeBadArrowInTSX > 0 && lexer.codePoint == '>' && lexer.source.Contents[lexer.end-1] == '=' {
						msg.Notes = []logger.MsgData{lexer.tracker.MsgData(lexer.BadArrowInTSXRange,
							"TypeScript's TSX syntax interprets arrow functions with a single generic type parameter as an opening JSX element. "+
								"If you want it to be interpreted as an arrow function instead, you need to add a trailing comma after the type parameter to disambiguate:")}
						msg.Notes[0].Location.Suggestion = lexer.BadArrowInTSXSuggestion
					} else {
						msg.Notes = []logger.MsgData{{Text: fmt.Sprintf("Did you mean to escape it as %q instead?", replacement)}}
						msg.Data.Location.Suggestion = replacement
						if !lexer.ts.Parse {
							// TypeScript treats this as an error but Babel doesn't treat this
							// as an error yet, so allow this in JS for now. Babel version 8
							// was supposed to be released in 2021 but was never released. If
							// it's released in the future, this can be changed to an error too.
							//
							// More context:
							// * TypeScript change: https://github.com/microsoft/TypeScript/issues/36341
							// * Babel 8 change: https://github.com/babel/babel/issues/11042
							// * Babel 8 release: https://github.com/babel/babel/issues/10746
							//
							msg.Kind = logger.Warning
						}
					}

					lexer.log.AddMsg(msg)
					lexer.step()

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
				lexer.decodedStringLiteralOrNil = fixWhitespaceAndDecodeJSXEntities(text)
			} else {
				// Fast path
				n := len(text)
				copy := make([]uint16, n)
				for i := 0; i < n; i++ {
					copy[i] = uint16(text[i])
				}
				lexer.decodedStringLiteralOrNil = copy
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

		case ':':
			lexer.step()
			lexer.Token = TColon

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
				startRange := lexer.Range()
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
						lexer.AddRangeErrorWithNotes(logger.Range{Loc: lexer.Loc()}, "Expected \"*/\" to terminate multi-line comment",
							[]logger.MsgData{lexer.tracker.MsgData(startRange, "The multi-line comment starts here:")})
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
			var backslash logger.Range
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

				case '\\':
					backslash = logger.Range{Loc: logger.Loc{Start: int32(lexer.end)}, Len: 1}
					lexer.step()
					continue

				case quote:
					if backslash.Len > 0 {
						backslash.Len++
						lexer.PreviousBackslashQuoteInJSX = backslash
					}
					lexer.step()
					break stringLiteral

				default:
					// Non-ASCII strings need the slow path
					if lexer.codePoint >= 0x80 {
						needsDecode = true
					}
					lexer.step()
				}
				backslash = logger.Range{}
			}

			lexer.Token = TStringLiteral
			text := lexer.source.Contents[lexer.start+1 : lexer.end-1]

			if needsDecode {
				// Slow path
				lexer.decodedStringLiteralOrNil = decodeJSXEntities([]uint16{}, text)
			} else {
				// Fast path
				n := len(text)
				copy := make([]uint16, n)
				for i := 0; i < n; i++ {
					copy[i] = uint16(text[i])
				}
				lexer.decodedStringLiteralOrNil = copy
			}

		default:
			// Check for unusual whitespace characters
			if js_ast.IsWhitespace(lexer.codePoint) {
				lexer.step()
				continue
			}

			if js_ast.IsIdentifierStart(lexer.codePoint) {
				lexer.step()
				for js_ast.IsIdentifierContinue(lexer.codePoint) || lexer.codePoint == '-' {
					lexer.step()
				}

				lexer.Identifier = lexer.rawIdentifier()
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
	lexer.HasCommentBefore = 0
	lexer.PrevTokenWasAwaitKeyword = false
	lexer.LegalCommentsBeforeToken = lexer.LegalCommentsBeforeToken[:0]
	lexer.CommentsBeforeToken = lexer.CommentsBeforeToken[:0]

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
				lexer.Identifier = lexer.rawIdentifier()
			} else {
				// "#foo"
				lexer.step()
				if lexer.codePoint == '\\' {
					lexer.Identifier, _ = lexer.scanIdentifierWithEscapes(privateIdentifier)
				} else {
					if !js_ast.IsIdentifierStart(lexer.codePoint) {
						lexer.SyntaxError()
					}
					lexer.step()
					for js_ast.IsIdentifierContinue(lexer.codePoint) {
						lexer.step()
					}
					if lexer.codePoint == '\\' {
						lexer.Identifier, _ = lexer.scanIdentifierWithEscapes(privateIdentifier)
					} else {
						lexer.Identifier = lexer.rawIdentifier()
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
					lexer.LegacyHTMLCommentRange = lexer.Range()
					lexer.log.AddID(logger.MsgID_JS_HTMLCommentInJS, logger.Warning, &lexer.tracker, lexer.Range(),
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
				if lexer.json == JSON && lexer.codePoint != '.' && (lexer.codePoint < '0' || lexer.codePoint > '9') {
					lexer.Unexpected()
				}
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
			if lexer.forGlobalName {
				lexer.Token = TSlash
				break
			}
			switch lexer.codePoint {
			case '=':
				lexer.step()
				lexer.Token = TSlashEquals

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
				if lexer.json == JSON {
					lexer.addRangeError(lexer.Range(), "JSON does not support comments")
				}
				lexer.scanCommentText()
				continue

			case '*':
				lexer.step()
				startRange := lexer.Range()
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
						lexer.AddRangeErrorWithNotes(logger.Range{Loc: lexer.Loc()}, "Expected \"*/\" to terminate multi-line comment",
							[]logger.MsgData{lexer.tracker.MsgData(startRange, "The multi-line comment starts here:")})
						panic(LexerPanic{})

					default:
						lexer.step()
					}
				}
				if lexer.json == JSON {
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
					lexer.LegacyHTMLCommentRange = lexer.Range()
					lexer.log.AddID(logger.MsgID_JS_HTMLCommentInJS, logger.Warning, &lexer.tracker, lexer.Range(),
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
					if lexer.codePoint == '\r' && lexer.json != JSON {
						lexer.step()
						if lexer.codePoint == '\n' {
							lexer.step()
						}
						continue
					}

				case -1: // This indicates the end of the file
					lexer.addRangeError(logger.Range{Loc: logger.Loc{Start: int32(lexer.end)}}, "Unterminated string literal")
					panic(LexerPanic{})

				case '\r':
					if quote != '`' {
						lexer.addRangeError(logger.Range{Loc: logger.Loc{Start: int32(lexer.end)}}, "Unterminated string literal")
						panic(LexerPanic{})
					}

					// Template literals require newline normalization
					needsSlowPath = true

				case '\n':
					if quote != '`' {
						lexer.addRangeError(logger.Range{Loc: logger.Loc{Start: int32(lexer.end)}}, "Unterminated string literal")
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
					} else if lexer.json == JSON && lexer.codePoint < 0x20 {
						lexer.SyntaxError()
					}
				}
				lexer.step()
			}

			text := lexer.source.Contents[lexer.start+1 : lexer.end-suffixLen]

			if needsSlowPath {
				// Slow path
				lexer.decodedStringLiteralOrNil = nil
				lexer.encodedStringLiteralStart = lexer.start + 1
				lexer.encodedStringLiteralText = text
			} else {
				// Fast path
				n := len(text)
				copy := make([]uint16, n)
				for i := 0; i < n; i++ {
					copy[i] = uint16(text[i])
				}
				lexer.decodedStringLiteralOrNil = copy
			}

			if quote == '\'' && (lexer.json == JSON || lexer.json == TSConfigJSON) {
				lexer.addRangeError(lexer.Range(), "JSON strings must use double quotes")
			}

		// Note: This case is hot in profiles
		case '_', '$',
			'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm',
			'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
			'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M',
			'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z':
			// This is a fast path for long ASCII identifiers. Doing this in a loop
			// first instead of doing "step()" and "js_ast.IsIdentifierContinue()" like we
			// do after this is noticeably faster in the common case of ASCII-only
			// text. For example, doing this sped up end-to-end consuming of a large
			// TypeScript type declaration file from 97ms to 79ms (around 20% faster).
			contents := lexer.source.Contents
			n := len(contents)
			i := lexer.current
			for i < n {
				c := contents[i]
				if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' && c != '$' {
					break
				}
				i++
			}
			lexer.current = i

			// Now do the slow path for any remaining non-ASCII identifier characters
			lexer.step()
			if lexer.codePoint >= 0x80 {
				for js_ast.IsIdentifierContinue(lexer.codePoint) {
					lexer.step()
				}
			}

			// If there's a slash, then we're in the extra-slow (and extra-rare) case
			// where the identifier has embedded escapes
			if lexer.codePoint == '\\' {
				lexer.Identifier, lexer.Token = lexer.scanIdentifierWithEscapes(normalIdentifier)
				break
			}

			// Otherwise (if there was no escape) we can slice the code verbatim
			lexer.Identifier = lexer.rawIdentifier()
			lexer.Token = Keywords[lexer.Raw()]
			if lexer.Token == 0 {
				lexer.Token = TIdentifier
			}

		case '\\':
			lexer.Identifier, lexer.Token = lexer.scanIdentifierWithEscapes(normalIdentifier)

		case '.', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			lexer.parseNumericLiteralOrDot()

		default:
			// Check for unusual whitespace characters
			if js_ast.IsWhitespace(lexer.codePoint) {
				lexer.step()
				continue
			}

			if js_ast.IsIdentifierStart(lexer.codePoint) {
				lexer.step()
				for js_ast.IsIdentifierContinue(lexer.codePoint) {
					lexer.step()
				}
				if lexer.codePoint == '\\' {
					lexer.Identifier, lexer.Token = lexer.scanIdentifierWithEscapes(normalIdentifier)
				} else {
					lexer.Token = TIdentifier
					lexer.Identifier = lexer.rawIdentifier()
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
func (lexer *Lexer) scanIdentifierWithEscapes(kind identifierKind) (MaybeSubstring, T) {
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
		if !js_ast.IsIdentifierContinue(lexer.codePoint) {
			break
		}
		lexer.step()
	}

	// Second pass: re-use our existing escape sequence parser
	decoded, ok, end := lexer.tryToDecodeEscapeSequences(lexer.start, lexer.Raw(), true /* reportErrors */)
	if !ok {
		lexer.end = end
		lexer.SyntaxError()
	}
	text := string(helpers.UTF16ToString(decoded))

	// Even though it was escaped, it must still be a valid identifier
	identifier := text
	if kind == privateIdentifier {
		identifier = identifier[1:] // Skip over the "#"
	}
	if !js_ast.IsIdentifier(identifier) {
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
		return MaybeSubstring{String: text}, TEscapedKeyword
	} else {
		return MaybeSubstring{String: text}, TIdentifier
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
	isMissingDigitAfterDot := false
	base := 0.0
	lexer.IsLegacyOctalLiteral = false

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
			lexer.IsLegacyOctalLiteral = true

		case '8', '9':
			lexer.IsLegacyOctalLiteral = true
		}
	}

	if base != 0 {
		// Integer literal
		isFirst := true
		isInvalidLegacyOctalLiteral := false
		lexer.Number = 0
		if !lexer.IsLegacyOctalLiteral {
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
				if isFirst || lexer.IsLegacyOctalLiteral {
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
				if lexer.IsLegacyOctalLiteral {
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
			text := lexer.rawIdentifier()

			// Can't use a leading zero for bigint literals
			if isBigIntegerLiteral && lexer.IsLegacyOctalLiteral {
				lexer.SyntaxError()
			}

			// Filter out underscores
			if underscoreCount > 0 {
				bytes := make([]byte, 0, len(text.String)-underscoreCount)
				for i := 0; i < len(text.String); i++ {
					c := text.String[i]
					if c != '_' {
						bytes = append(bytes, c)
					}
				}
				text = MaybeSubstring{String: string(bytes)}
			}

			// Store bigints as text to avoid precision loss
			if isBigIntegerLiteral {
				lexer.Identifier = text
			} else if isInvalidLegacyOctalLiteral {
				// Legacy octal literals may turn out to be a base 10 literal after all
				value, _ := strconv.ParseFloat(text.String, 64)
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
			isMissingDigitAfterDot = true
			for {
				if lexer.codePoint >= '0' && lexer.codePoint <= '9' {
					isMissingDigitAfterDot = false
				} else {
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
		text := lexer.rawIdentifier()

		// Filter out underscores
		if underscoreCount > 0 {
			bytes := make([]byte, 0, len(text.String)-underscoreCount)
			for i := 0; i < len(text.String); i++ {
				c := text.String[i]
				if c != '_' {
					bytes = append(bytes, c)
				}
			}
			text = MaybeSubstring{String: string(bytes)}
		}

		if lexer.codePoint == 'n' && !hasDotOrExponent {
			// The only bigint literal that can start with 0 is "0n"
			if len(text.String) > 1 && first == '0' {
				lexer.SyntaxError()
			}

			// Store bigints as text to avoid precision loss
			lexer.Identifier = text
		} else if !hasDotOrExponent && lexer.end-lexer.start < 10 {
			// Parse a 32-bit integer (very fast path)
			var number uint32 = 0
			for _, c := range text.String {
				number = number*10 + uint32(c-'0')
			}
			lexer.Number = float64(number)
		} else {
			// Parse a double-precision floating-point number
			value, _ := strconv.ParseFloat(text.String, 64)
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
	if js_ast.IsIdentifierStart(lexer.codePoint) {
		lexer.SyntaxError()
	}

	// None of these are allowed in JSON
	if lexer.json == JSON && (first == '.' || base != 0 || underscoreCount > 0 || isMissingDigitAfterDot) {
		lexer.Unexpected()
	}
}

func (lexer *Lexer) ScanRegExp() {
	validateAndStep := func() {
		if lexer.codePoint == '\\' {
			lexer.step()
		}

		switch lexer.codePoint {
		case -1, // This indicates the end of the file
			'\r', '\n', 0x2028, 0x2029: // Newlines aren't allowed in regular expressions
			lexer.addRangeError(logger.Range{Loc: logger.Loc{Start: int32(lexer.end)}}, "Unterminated regular expression")
			panic(LexerPanic{})

		default:
			lexer.step()
		}
	}

	for {
		switch lexer.codePoint {
		case '/':
			lexer.step()
			bits := uint32(0)
			for js_ast.IsIdentifierContinue(lexer.codePoint) {
				switch lexer.codePoint {
				case 'd', 'g', 'i', 'm', 's', 'u', 'v', 'y':
					bit := uint32(1) << uint32(lexer.codePoint-'a')
					if (bit & bits) != 0 {
						// Reject duplicate flags
						r1 := logger.Range{Loc: logger.Loc{Start: int32(lexer.start)}, Len: 1}
						r2 := logger.Range{Loc: logger.Loc{Start: int32(lexer.end)}, Len: 1}
						for r1.Loc.Start < r2.Loc.Start && lexer.source.Contents[r1.Loc.Start] != byte(lexer.codePoint) {
							r1.Loc.Start++
						}
						lexer.log.AddErrorWithNotes(&lexer.tracker, r2,
							fmt.Sprintf("Duplicate flag \"%c\" in regular expression", lexer.codePoint),
							[]logger.MsgData{lexer.tracker.MsgData(r1,
								fmt.Sprintf("The first \"%c\" was here:", lexer.codePoint))})
					} else {
						bits |= bit
					}
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
			if !js_ast.IsWhitespace(c) {
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

// If this fails, this returns "nil, false, end" where "end" is the value to
// store to "lexer.end" before calling "lexer.SyntaxError()" if relevant
func (lexer *Lexer) tryToDecodeEscapeSequences(start int, text string, reportErrors bool) ([]uint16, bool, int) {
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
				if lexer.json == JSON {
					return nil, false, start + i - width2
				}

				decoded = append(decoded, '\v')
				continue

			case '0', '1', '2', '3', '4', '5', '6', '7':
				octalStart := i - 2
				if lexer.json == JSON {
					return nil, false, start + i - width2
				}

				// 1-3 digit octal
				isBad := false
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
					case '8', '9':
						isBad = true
					}
				case '8', '9':
					isBad = true
				}
				c = value

				// Forbid the use of octal literals other than "\0"
				if isBad || text[octalStart:i] != "\\0" {
					lexer.LegacyOctalLoc = logger.Loc{Start: int32(start + octalStart)}
				}

			case '8', '9':
				c = c2

				// Forbid the invalid octal literals "\8" and "\9"
				lexer.LegacyOctalLoc = logger.Loc{Start: int32(start + i - 2)}

			case 'x':
				if lexer.json == JSON {
					return nil, false, start + i - width2
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
						return nil, false, start + i - width3
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
					if lexer.json == JSON {
						return nil, false, start + i - width2
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
								return nil, false, start + i - width3
							}
							break variableLength
						default:
							return nil, false, start + i - width3
						}

						if value > utf8.MaxRune {
							isOutOfRange = true
						}

						isFirst = false
					}

					if isOutOfRange && reportErrors {
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
							return nil, false, start + i - width3
						}

						if j < 3 {
							c3, width3 = utf8.DecodeRuneInString(text[i:])
							i += width3
						}
					}
				}
				c = value

			case '\r':
				if lexer.json == JSON {
					return nil, false, start + i - width2
				}

				// Ignore line continuations. A line continuation is not an escaped newline.
				if i < len(text) && text[i] == '\n' {
					// Make sure Windows CRLF counts as a single newline
					i++
				}
				continue

			case '\n', '\u2028', '\u2029':
				if lexer.json == JSON {
					return nil, false, start + i - width2
				}

				// Ignore line continuations. A line continuation is not an escaped newline.
				continue

			default:
				if lexer.json == JSON {
					switch c2 {
					case '"', '\\', '/':

					default:
						return nil, false, start + i - width2
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

	return decoded, true, 0
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

func (lexer *Lexer) addRangeError(r logger.Range, text string) {
	// Don't report multiple errors in the same spot
	if r.Loc == lexer.prevErrorLoc {
		return
	}
	lexer.prevErrorLoc = r.Loc

	if !lexer.IsLogDisabled {
		lexer.log.AddError(&lexer.tracker, r, text)
	}
}

func (lexer *Lexer) addRangeErrorWithSuggestion(r logger.Range, text string, suggestion string) {
	// Don't report multiple errors in the same spot
	if r.Loc == lexer.prevErrorLoc {
		return
	}
	lexer.prevErrorLoc = r.Loc

	if !lexer.IsLogDisabled {
		data := lexer.tracker.MsgData(r, text)
		data.Location.Suggestion = suggestion
		lexer.log.AddMsg(logger.Msg{Kind: logger.Error, Data: data})
	}
}

func (lexer *Lexer) AddRangeErrorWithNotes(r logger.Range, text string, notes []logger.MsgData) {
	// Don't report multiple errors in the same spot
	if r.Loc == lexer.prevErrorLoc {
		return
	}
	lexer.prevErrorLoc = r.Loc

	if !lexer.IsLogDisabled {
		lexer.log.AddErrorWithNotes(&lexer.tracker, r, text, notes)
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
		if !js_ast.IsIdentifierContinue(c) {
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

func scanForPragmaArg(kind pragmaArg, start int, pragma string, text string) (logger.Span, bool) {
	text = text[len(pragma):]
	start += len(pragma)

	if text == "" {
		return logger.Span{}, false
	}

	// One or more whitespace characters
	c, width := utf8.DecodeRuneInString(text)
	if kind == pragmaSkipSpaceFirst {
		if !js_ast.IsWhitespace(c) {
			return logger.Span{}, false
		}
		for js_ast.IsWhitespace(c) {
			text = text[width:]
			start += width
			if text == "" {
				return logger.Span{}, false
			}
			c, width = utf8.DecodeRuneInString(text)
		}
	}

	// One or more non-whitespace characters
	i := 0
	for !js_ast.IsWhitespace(c) {
		i += width
		if i >= len(text) {
			break
		}
		c, width = utf8.DecodeRuneInString(text[i:])
		if js_ast.IsWhitespace(c) {
			break
		}
	}

	return logger.Span{
		Text: text[:i],
		Range: logger.Range{
			Loc: logger.Loc{Start: int32(start)},
			Len: int32(i),
		},
	}, true
}

func isUpperASCII(c byte) bool {
	return c >= 'A' && c <= 'Z'
}

func isLetterASCII(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func (lexer *Lexer) scanCommentText() {
	text := lexer.source.Contents[lexer.start:lexer.end]
	hasLegalAnnotation := len(text) > 2 && text[2] == '!'
	isMultiLineComment := text[1] == '*'
	omitFromGeneralCommentPreservation := false

	// Save the original comment text so we can subtract comments from the
	// character frequency analysis used by symbol minification
	lexer.AllComments = append(lexer.AllComments, lexer.Range())

	// Omit the trailing "*/" from the checks below
	endOfCommentText := len(text)
	if isMultiLineComment {
		endOfCommentText -= 2
	}

	for i, n := 0, len(text); i < n; i++ {
		switch text[i] {
		case '#':
			rest := text[i+1 : endOfCommentText]
			if hasPrefixWithWordBoundary(rest, "__PURE__") {
				omitFromGeneralCommentPreservation = true
				lexer.HasCommentBefore |= PureCommentBefore
			} else if hasPrefixWithWordBoundary(rest, "__KEY__") {
				omitFromGeneralCommentPreservation = true
				lexer.HasCommentBefore |= KeyCommentBefore
			} else if hasPrefixWithWordBoundary(rest, "__NO_SIDE_EFFECTS__") {
				omitFromGeneralCommentPreservation = true
				lexer.HasCommentBefore |= NoSideEffectsCommentBefore
			} else if i == 2 && strings.HasPrefix(rest, " sourceMappingURL=") {
				if arg, ok := scanForPragmaArg(pragmaNoSpaceFirst, lexer.start+i+1, " sourceMappingURL=", rest); ok {
					omitFromGeneralCommentPreservation = true
					lexer.SourceMappingURL = arg
				}
			}

		case '@':
			rest := text[i+1 : endOfCommentText]
			if hasPrefixWithWordBoundary(rest, "__PURE__") {
				omitFromGeneralCommentPreservation = true
				lexer.HasCommentBefore |= PureCommentBefore
			} else if hasPrefixWithWordBoundary(rest, "__KEY__") {
				omitFromGeneralCommentPreservation = true
				lexer.HasCommentBefore |= KeyCommentBefore
			} else if hasPrefixWithWordBoundary(rest, "__NO_SIDE_EFFECTS__") {
				omitFromGeneralCommentPreservation = true
				lexer.HasCommentBefore |= NoSideEffectsCommentBefore
			} else if hasPrefixWithWordBoundary(rest, "preserve") || hasPrefixWithWordBoundary(rest, "license") {
				hasLegalAnnotation = true
			} else if hasPrefixWithWordBoundary(rest, "jsx") {
				if arg, ok := scanForPragmaArg(pragmaSkipSpaceFirst, lexer.start+i+1, "jsx", rest); ok {
					lexer.JSXFactoryPragmaComment = arg
				}
			} else if hasPrefixWithWordBoundary(rest, "jsxFrag") {
				if arg, ok := scanForPragmaArg(pragmaSkipSpaceFirst, lexer.start+i+1, "jsxFrag", rest); ok {
					lexer.JSXFragmentPragmaComment = arg
				}
			} else if hasPrefixWithWordBoundary(rest, "jsxRuntime") {
				if arg, ok := scanForPragmaArg(pragmaSkipSpaceFirst, lexer.start+i+1, "jsxRuntime", rest); ok {
					lexer.JSXRuntimePragmaComment = arg
				}
			} else if hasPrefixWithWordBoundary(rest, "jsxImportSource") {
				if arg, ok := scanForPragmaArg(pragmaSkipSpaceFirst, lexer.start+i+1, "jsxImportSource", rest); ok {
					lexer.JSXImportSourcePragmaComment = arg
				}
			} else if i == 2 && strings.HasPrefix(rest, " sourceMappingURL=") {
				if arg, ok := scanForPragmaArg(pragmaNoSpaceFirst, lexer.start+i+1, " sourceMappingURL=", rest); ok {
					omitFromGeneralCommentPreservation = true
					lexer.SourceMappingURL = arg
				}
			}
		}
	}

	if hasLegalAnnotation {
		lexer.LegalCommentsBeforeToken = append(lexer.LegalCommentsBeforeToken, lexer.Range())
	}

	if !omitFromGeneralCommentPreservation {
		lexer.CommentsBeforeToken = append(lexer.CommentsBeforeToken, lexer.Range())
	}
}
