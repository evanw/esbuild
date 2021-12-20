// This file contains code for parsing TypeScript syntax. The parser just skips
// over type expressions as if they are whitespace and doesn't bother generating
// an AST because nothing uses type information.

package js_parser

import (
	"fmt"
	"strings"

	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/logger"
)

func (p *parser) skipTypeScriptBinding() {
	switch p.lexer.Token {
	case js_lexer.TIdentifier, js_lexer.TThis:
		p.lexer.Next()

	case js_lexer.TOpenBracket:
		p.lexer.Next()

		// "[, , a]"
		for p.lexer.Token == js_lexer.TComma {
			p.lexer.Next()
		}

		// "[a, b]"
		for p.lexer.Token != js_lexer.TCloseBracket {
			p.skipTypeScriptBinding()
			if p.lexer.Token != js_lexer.TComma {
				break
			}
			p.lexer.Next()
		}

		p.lexer.Expect(js_lexer.TCloseBracket)

	case js_lexer.TOpenBrace:
		p.lexer.Next()

		for p.lexer.Token != js_lexer.TCloseBrace {
			foundIdentifier := false

			switch p.lexer.Token {
			case js_lexer.TDotDotDot:
				p.lexer.Next()

				if p.lexer.Token != js_lexer.TIdentifier {
					p.lexer.Unexpected()
				}

				// "{...x}"
				foundIdentifier = true
				p.lexer.Next()

			case js_lexer.TIdentifier:
				// "{x}"
				// "{x: y}"
				foundIdentifier = true
				p.lexer.Next()

				// "{1: y}"
				// "{'x': y}"
			case js_lexer.TStringLiteral, js_lexer.TNumericLiteral:
				p.lexer.Next()

			default:
				if p.lexer.IsIdentifierOrKeyword() {
					// "{if: x}"
					p.lexer.Next()
				} else {
					p.lexer.Unexpected()
				}
			}

			if p.lexer.Token == js_lexer.TColon || !foundIdentifier {
				p.lexer.Expect(js_lexer.TColon)
				p.skipTypeScriptBinding()
			}

			if p.lexer.Token != js_lexer.TComma {
				break
			}
			p.lexer.Next()
		}

		p.lexer.Expect(js_lexer.TCloseBrace)

	default:
		p.lexer.Unexpected()
	}
}

func (p *parser) skipTypeScriptFnArgs() {
	p.lexer.Expect(js_lexer.TOpenParen)

	for p.lexer.Token != js_lexer.TCloseParen {
		// "(...a)"
		if p.lexer.Token == js_lexer.TDotDotDot {
			p.lexer.Next()
		}

		p.skipTypeScriptBinding()

		// "(a?)"
		if p.lexer.Token == js_lexer.TQuestion {
			p.lexer.Next()
		}

		// "(a: any)"
		if p.lexer.Token == js_lexer.TColon {
			p.lexer.Next()
			p.skipTypeScriptType(js_ast.LLowest)
		}

		// "(a, b)"
		if p.lexer.Token != js_lexer.TComma {
			break
		}
		p.lexer.Next()
	}

	p.lexer.Expect(js_lexer.TCloseParen)
}

// This is a spot where the TypeScript grammar is highly ambiguous. Here are
// some cases that are valid:
//
//     let x = (y: any): (() => {}) => { };
//     let x = (y: any): () => {} => { };
//     let x = (y: any): (y) => {} => { };
//     let x = (y: any): (y[]) => {};
//     let x = (y: any): (a | b) => {};
//
// Here are some cases that aren't valid:
//
//     let x = (y: any): (y) => {};
//     let x = (y: any): (y) => {return 0};
//     let x = (y: any): asserts y is (y) => {};
//
func (p *parser) skipTypeScriptParenOrFnType() {
	if p.trySkipTypeScriptArrowArgsWithBacktracking() {
		p.skipTypeScriptReturnType()
	} else {
		p.lexer.Expect(js_lexer.TOpenParen)
		p.skipTypeScriptType(js_ast.LLowest)
		p.lexer.Expect(js_lexer.TCloseParen)
	}
}

func (p *parser) skipTypeScriptReturnType() {
	p.skipTypeScriptTypeWithOpts(js_ast.LLowest, skipTypeOpts{isReturnType: true})
}

func (p *parser) skipTypeScriptType(level js_ast.L) {
	p.skipTypeScriptTypeWithOpts(level, skipTypeOpts{})
}

type skipTypeOpts struct {
	isReturnType     bool
	allowTupleLabels bool
}

type tsTypeIdentifierKind uint8

const (
	tsTypeIdentifierNormal tsTypeIdentifierKind = iota
	tsTypeIdentifierUnique
	tsTypeIdentifierAbstract
	tsTypeIdentifierAsserts
	tsTypeIdentifierPrefix
	tsTypeIdentifierPrimitive
)

// Use a map to improve lookup speed
var tsTypeIdentifierMap = map[string]tsTypeIdentifierKind{
	"unique":   tsTypeIdentifierUnique,
	"abstract": tsTypeIdentifierAbstract,
	"asserts":  tsTypeIdentifierAsserts,

	"keyof":    tsTypeIdentifierPrefix,
	"readonly": tsTypeIdentifierPrefix,
	"infer":    tsTypeIdentifierPrefix,

	"any":       tsTypeIdentifierPrimitive,
	"never":     tsTypeIdentifierPrimitive,
	"unknown":   tsTypeIdentifierPrimitive,
	"undefined": tsTypeIdentifierPrimitive,
	"object":    tsTypeIdentifierPrimitive,
	"number":    tsTypeIdentifierPrimitive,
	"string":    tsTypeIdentifierPrimitive,
	"boolean":   tsTypeIdentifierPrimitive,
	"bigint":    tsTypeIdentifierPrimitive,
	"symbol":    tsTypeIdentifierPrimitive,
}

func (p *parser) skipTypeScriptTypeWithOpts(level js_ast.L, opts skipTypeOpts) {
	for {
		switch p.lexer.Token {
		case js_lexer.TNumericLiteral, js_lexer.TBigIntegerLiteral, js_lexer.TStringLiteral,
			js_lexer.TNoSubstitutionTemplateLiteral, js_lexer.TTrue, js_lexer.TFalse,
			js_lexer.TNull, js_lexer.TVoid:
			p.lexer.Next()

		case js_lexer.TConst:
			r := p.lexer.Range()
			p.lexer.Next()

			// "[const: number]"
			if opts.allowTupleLabels && p.lexer.Token == js_lexer.TColon {
				p.log.Add(logger.Error, &p.tracker, r, "Unexpected \"const\"")
			}

		case js_lexer.TThis:
			p.lexer.Next()

			// "function check(): this is boolean"
			if p.lexer.IsContextualKeyword("is") && !p.lexer.HasNewlineBefore {
				p.lexer.Next()
				p.skipTypeScriptType(js_ast.LLowest)
				return
			}

		case js_lexer.TMinus:
			// "-123"
			// "-123n"
			p.lexer.Next()
			if p.lexer.Token == js_lexer.TBigIntegerLiteral {
				p.lexer.Next()
			} else {
				p.lexer.Expect(js_lexer.TNumericLiteral)
			}

		case js_lexer.TAmpersand:
		case js_lexer.TBar:
			// Support things like "type Foo = | A | B" and "type Foo = & A & B"
			p.lexer.Next()
			continue

		case js_lexer.TImport:
			// "import('fs')"
			p.lexer.Next()

			// "[import: number]"
			if opts.allowTupleLabels && p.lexer.Token == js_lexer.TColon {
				return
			}

			p.lexer.Expect(js_lexer.TOpenParen)
			p.lexer.Expect(js_lexer.TStringLiteral)
			p.lexer.Expect(js_lexer.TCloseParen)

		case js_lexer.TNew:
			// "new () => Foo"
			// "new <T>() => Foo<T>"
			p.lexer.Next()

			// "[new: number]"
			if opts.allowTupleLabels && p.lexer.Token == js_lexer.TColon {
				return
			}

			p.skipTypeScriptTypeParameters()
			p.skipTypeScriptParenOrFnType()

		case js_lexer.TLessThan:
			// "<T>() => Foo<T>"
			p.skipTypeScriptTypeParameters()
			p.skipTypeScriptParenOrFnType()

		case js_lexer.TOpenParen:
			// "(number | string)"
			p.skipTypeScriptParenOrFnType()

		case js_lexer.TIdentifier:
			kind := tsTypeIdentifierMap[p.lexer.Identifier]

			if kind == tsTypeIdentifierPrefix {
				p.lexer.Next()
				// {[keyof: string]: number}
				// {[readonly: string]: number}
				// {[infer: string]: number}
				if p.lexer.Token != js_lexer.TColon {
					p.skipTypeScriptType(js_ast.LPrefix)
				}
				break
			}

			checkTypeParameters := true

			if kind == tsTypeIdentifierUnique {
				p.lexer.Next()

				// "let foo: unique symbol"
				if p.lexer.IsContextualKeyword("symbol") {
					p.lexer.Next()
					break
				}
			} else if kind == tsTypeIdentifierAbstract {
				p.lexer.Next()

				// "let foo: abstract new () => {}" added in TypeScript 4.2
				if p.lexer.Token == js_lexer.TNew {
					continue
				}
			} else if kind == tsTypeIdentifierAsserts {
				p.lexer.Next()

				// "function assert(x: boolean): asserts x"
				// "function assert(x: boolean): asserts x is boolean"
				if opts.isReturnType && !p.lexer.HasNewlineBefore && (p.lexer.Token == js_lexer.TIdentifier || p.lexer.Token == js_lexer.TThis) {
					p.lexer.Next()
				}
			} else if kind == tsTypeIdentifierPrimitive {
				p.lexer.Next()
				checkTypeParameters = false
			} else {
				p.lexer.Next()
			}

			// "function assert(x: any): x is boolean"
			if p.lexer.IsContextualKeyword("is") && !p.lexer.HasNewlineBefore {
				p.lexer.Next()
				p.skipTypeScriptType(js_ast.LLowest)
				return
			}

			// "let foo: any \n <number>foo" must not become a single type
			if checkTypeParameters && !p.lexer.HasNewlineBefore {
				p.skipTypeScriptTypeArguments(false /* isInsideJSXElement */)
			}

		case js_lexer.TTypeof:
			p.lexer.Next()

			// "[typeof: number]"
			if opts.allowTupleLabels && p.lexer.Token == js_lexer.TColon {
				return
			}

			if p.lexer.Token == js_lexer.TImport {
				// "typeof import('fs')"
				continue
			} else {
				// "typeof x"
				// "typeof x.y"
				for {
					if !p.lexer.IsIdentifierOrKeyword() {
						p.lexer.Expected(js_lexer.TIdentifier)
					}
					p.lexer.Next()
					if p.lexer.Token != js_lexer.TDot {
						break
					}
					p.lexer.Next()
				}
			}

		case js_lexer.TOpenBracket:
			// "[number, string]"
			// "[first: number, second: string]"
			p.lexer.Next()
			for p.lexer.Token != js_lexer.TCloseBracket {
				if p.lexer.Token == js_lexer.TDotDotDot {
					p.lexer.Next()
				}
				p.skipTypeScriptTypeWithOpts(js_ast.LLowest, skipTypeOpts{allowTupleLabels: true})
				if p.lexer.Token == js_lexer.TQuestion {
					p.lexer.Next()
				}
				if p.lexer.Token == js_lexer.TColon {
					p.lexer.Next()
					p.skipTypeScriptType(js_ast.LLowest)
				}
				if p.lexer.Token != js_lexer.TComma {
					break
				}
				p.lexer.Next()
			}
			p.lexer.Expect(js_lexer.TCloseBracket)

		case js_lexer.TOpenBrace:
			p.skipTypeScriptObjectType()

		case js_lexer.TTemplateHead:
			// "`${'a' | 'b'}-${'c' | 'd'}`"
			for {
				p.lexer.Next()
				p.skipTypeScriptType(js_ast.LLowest)
				p.lexer.RescanCloseBraceAsTemplateToken()
				if p.lexer.Token == js_lexer.TTemplateTail {
					p.lexer.Next()
					break
				}
			}

		default:
			// "[function: number]"
			if opts.allowTupleLabels && p.lexer.IsIdentifierOrKeyword() {
				if p.lexer.Token != js_lexer.TFunction {
					p.log.Add(logger.Error, &p.tracker, p.lexer.Range(), fmt.Sprintf("Unexpected %q", p.lexer.Raw()))
				}
				p.lexer.Next()
				if p.lexer.Token != js_lexer.TColon {
					p.lexer.Expect(js_lexer.TColon)
				}
				return
			}

			p.lexer.Unexpected()
		}
		break
	}

	for {
		switch p.lexer.Token {
		case js_lexer.TBar:
			if level >= js_ast.LBitwiseOr {
				return
			}
			p.lexer.Next()
			p.skipTypeScriptType(js_ast.LBitwiseOr)

		case js_lexer.TAmpersand:
			if level >= js_ast.LBitwiseAnd {
				return
			}
			p.lexer.Next()
			p.skipTypeScriptType(js_ast.LBitwiseAnd)

		case js_lexer.TExclamation:
			// A postfix "!" is allowed in JSDoc types in TypeScript, which are only
			// present in comments. While it's not valid in a non-comment position,
			// it's still parsed and turned into a soft error by the TypeScript
			// compiler. It turns out parsing this is important for correctness for
			// "as" casts because the "!" token must still be consumed.
			if p.lexer.HasNewlineBefore {
				return
			}
			p.lexer.Next()

		case js_lexer.TDot:
			p.lexer.Next()
			if !p.lexer.IsIdentifierOrKeyword() {
				p.lexer.Expect(js_lexer.TIdentifier)
			}
			p.lexer.Next()

			// "{ <A extends B>(): c.d \n <E extends F>(): g.h }" must not become a single type
			if !p.lexer.HasNewlineBefore {
				p.skipTypeScriptTypeArguments(false /* isInsideJSXElement */)
			}

		case js_lexer.TOpenBracket:
			// "{ ['x']: string \n ['y']: string }" must not become a single type
			if p.lexer.HasNewlineBefore {
				return
			}
			p.lexer.Next()
			if p.lexer.Token != js_lexer.TCloseBracket {
				p.skipTypeScriptType(js_ast.LLowest)
			}
			p.lexer.Expect(js_lexer.TCloseBracket)

		case js_lexer.TExtends:
			// "{ x: number \n extends: boolean }" must not become a single type
			if p.lexer.HasNewlineBefore || level >= js_ast.LConditional {
				return
			}
			p.lexer.Next()

			// The type following "extends" is not permitted to be another conditional type
			p.skipTypeScriptType(js_ast.LConditional)
			p.lexer.Expect(js_lexer.TQuestion)
			p.skipTypeScriptType(js_ast.LLowest)
			p.lexer.Expect(js_lexer.TColon)
			p.skipTypeScriptType(js_ast.LLowest)

		default:
			return
		}
	}
}

func (p *parser) skipTypeScriptObjectType() {
	p.lexer.Expect(js_lexer.TOpenBrace)

	for p.lexer.Token != js_lexer.TCloseBrace {
		// "{ -readonly [K in keyof T]: T[K] }"
		// "{ +readonly [K in keyof T]: T[K] }"
		if p.lexer.Token == js_lexer.TPlus || p.lexer.Token == js_lexer.TMinus {
			p.lexer.Next()
		}

		// Skip over modifiers and the property identifier
		foundKey := false
		for p.lexer.IsIdentifierOrKeyword() ||
			p.lexer.Token == js_lexer.TStringLiteral ||
			p.lexer.Token == js_lexer.TNumericLiteral {
			p.lexer.Next()
			foundKey = true
		}

		if p.lexer.Token == js_lexer.TOpenBracket {
			// Index signature or computed property
			p.lexer.Next()
			p.skipTypeScriptType(js_ast.LLowest)

			// "{ [key: string]: number }"
			// "{ readonly [K in keyof T]: T[K] }"
			if p.lexer.Token == js_lexer.TColon {
				p.lexer.Next()
				p.skipTypeScriptType(js_ast.LLowest)
			} else if p.lexer.Token == js_lexer.TIn {
				p.lexer.Next()
				p.skipTypeScriptType(js_ast.LLowest)
				if p.lexer.IsContextualKeyword("as") {
					// "{ [K in keyof T as `get-${K}`]: T[K] }"
					p.lexer.Next()
					p.skipTypeScriptType(js_ast.LLowest)
				}
			}

			p.lexer.Expect(js_lexer.TCloseBracket)

			// "{ [K in keyof T]+?: T[K] }"
			// "{ [K in keyof T]-?: T[K] }"
			if p.lexer.Token == js_lexer.TPlus || p.lexer.Token == js_lexer.TMinus {
				p.lexer.Next()
			}

			foundKey = true
		}

		// "?" indicates an optional property
		// "!" indicates an initialization assertion
		if foundKey && (p.lexer.Token == js_lexer.TQuestion || p.lexer.Token == js_lexer.TExclamation) {
			p.lexer.Next()
		}

		// Type parameters come right after the optional mark
		p.skipTypeScriptTypeParameters()

		switch p.lexer.Token {
		case js_lexer.TColon:
			// Regular property
			if !foundKey {
				p.lexer.Expect(js_lexer.TIdentifier)
			}
			p.lexer.Next()
			p.skipTypeScriptType(js_ast.LLowest)

		case js_lexer.TOpenParen:
			// Method signature
			p.skipTypeScriptFnArgs()
			if p.lexer.Token == js_lexer.TColon {
				p.lexer.Next()
				p.skipTypeScriptReturnType()
			}

		default:
			if !foundKey {
				p.lexer.Unexpected()
			}
		}

		switch p.lexer.Token {
		case js_lexer.TCloseBrace:

		case js_lexer.TComma, js_lexer.TSemicolon:
			p.lexer.Next()

		default:
			if !p.lexer.HasNewlineBefore {
				p.lexer.Unexpected()
			}
		}
	}

	p.lexer.Expect(js_lexer.TCloseBrace)
}

// This is the type parameter declarations that go with other symbol
// declarations (class, function, type, etc.)
func (p *parser) skipTypeScriptTypeParameters() {
	if p.lexer.Token == js_lexer.TLessThan {
		p.lexer.Next()

		for {
			p.lexer.Expect(js_lexer.TIdentifier)

			// "class Foo<T extends number> {}"
			if p.lexer.Token == js_lexer.TExtends {
				p.lexer.Next()
				p.skipTypeScriptType(js_ast.LLowest)
			}

			// "class Foo<T = void> {}"
			if p.lexer.Token == js_lexer.TEquals {
				p.lexer.Next()
				p.skipTypeScriptType(js_ast.LLowest)
			}

			if p.lexer.Token != js_lexer.TComma {
				break
			}
			p.lexer.Next()
			if p.lexer.Token == js_lexer.TGreaterThan {
				break
			}
		}

		p.lexer.ExpectGreaterThan(false /* isInsideJSXElement */)
	}
}

func (p *parser) skipTypeScriptTypeArguments(isInsideJSXElement bool) bool {
	switch p.lexer.Token {
	case js_lexer.TLessThan, js_lexer.TLessThanEquals,
		js_lexer.TLessThanLessThan, js_lexer.TLessThanLessThanEquals:
	default:
		return false
	}

	p.lexer.ExpectLessThan(false /* isInsideJSXElement */)

	for {
		p.skipTypeScriptType(js_ast.LLowest)
		if p.lexer.Token != js_lexer.TComma {
			break
		}
		p.lexer.Next()
	}

	// This type argument list must end with a ">"
	p.lexer.ExpectGreaterThan(isInsideJSXElement)
	return true
}

func (p *parser) trySkipTypeScriptTypeArgumentsWithBacktracking() bool {
	oldLexer := p.lexer
	p.lexer.IsLogDisabled = true

	// Implement backtracking by restoring the lexer's memory to its original state
	defer func() {
		r := recover()
		if _, isLexerPanic := r.(js_lexer.LexerPanic); isLexerPanic {
			p.lexer = oldLexer
		} else if r != nil {
			panic(r)
		}
	}()

	p.skipTypeScriptTypeArguments(false /* isInsideJSXElement */)

	// Check the token after this and backtrack if it's the wrong one
	if !p.canFollowTypeArgumentsInExpression() {
		p.lexer.Unexpected()
	}

	// Restore the log disabled flag. Note that we can't just set it back to false
	// because it may have been true to start with.
	p.lexer.IsLogDisabled = oldLexer.IsLogDisabled
	return true
}

func (p *parser) trySkipTypeScriptTypeParametersThenOpenParenWithBacktracking() bool {
	oldLexer := p.lexer
	p.lexer.IsLogDisabled = true

	// Implement backtracking by restoring the lexer's memory to its original state
	defer func() {
		r := recover()
		if _, isLexerPanic := r.(js_lexer.LexerPanic); isLexerPanic {
			p.lexer = oldLexer
		} else if r != nil {
			panic(r)
		}
	}()

	p.skipTypeScriptTypeParameters()
	if p.lexer.Token != js_lexer.TOpenParen {
		p.lexer.Unexpected()
	}

	// Restore the log disabled flag. Note that we can't just set it back to false
	// because it may have been true to start with.
	p.lexer.IsLogDisabled = oldLexer.IsLogDisabled
	return true
}

func (p *parser) trySkipTypeScriptArrowReturnTypeWithBacktracking() bool {
	oldLexer := p.lexer
	p.lexer.IsLogDisabled = true

	// Implement backtracking by restoring the lexer's memory to its original state
	defer func() {
		r := recover()
		if _, isLexerPanic := r.(js_lexer.LexerPanic); isLexerPanic {
			p.lexer = oldLexer
		} else if r != nil {
			panic(r)
		}
	}()

	p.lexer.Expect(js_lexer.TColon)
	p.skipTypeScriptReturnType()

	// Check the token after this and backtrack if it's the wrong one
	if p.lexer.Token != js_lexer.TEqualsGreaterThan {
		p.lexer.Unexpected()
	}

	// Restore the log disabled flag. Note that we can't just set it back to false
	// because it may have been true to start with.
	p.lexer.IsLogDisabled = oldLexer.IsLogDisabled
	return true
}

func (p *parser) trySkipTypeScriptArrowArgsWithBacktracking() bool {
	oldLexer := p.lexer
	p.lexer.IsLogDisabled = true

	// Implement backtracking by restoring the lexer's memory to its original state
	defer func() {
		r := recover()
		if _, isLexerPanic := r.(js_lexer.LexerPanic); isLexerPanic {
			p.lexer = oldLexer
		} else if r != nil {
			panic(r)
		}
	}()

	p.skipTypeScriptFnArgs()
	p.lexer.Expect(js_lexer.TEqualsGreaterThan)

	// Restore the log disabled flag. Note that we can't just set it back to false
	// because it may have been true to start with.
	p.lexer.IsLogDisabled = oldLexer.IsLogDisabled
	return true
}

// Returns true if the current less-than token is considered to be an arrow
// function under TypeScript's rules for files containing JSX syntax
func (p *parser) isTSArrowFnJSX() (isTSArrowFn bool) {
	oldLexer := p.lexer
	p.lexer.Next()

	// Look ahead to see if this should be an arrow function instead
	if p.lexer.Token == js_lexer.TIdentifier {
		p.lexer.Next()
		if p.lexer.Token == js_lexer.TComma {
			isTSArrowFn = true
		} else if p.lexer.Token == js_lexer.TExtends {
			p.lexer.Next()
			isTSArrowFn = p.lexer.Token != js_lexer.TEquals && p.lexer.Token != js_lexer.TGreaterThan
		}
	}

	// Restore the lexer
	p.lexer = oldLexer
	return
}

// This function is taken from the official TypeScript compiler source code:
// https://github.com/microsoft/TypeScript/blob/master/src/compiler/parser.ts
func (p *parser) canFollowTypeArgumentsInExpression() bool {
	switch p.lexer.Token {
	case
		// These are the only tokens can legally follow a type argument list. So we
		// definitely want to treat them as type arg lists.
		js_lexer.TOpenParen,                     // foo<x>(
		js_lexer.TNoSubstitutionTemplateLiteral, // foo<T> `...`
		js_lexer.TTemplateHead:                  // foo<T> `...${100}...`
		return true

	case
		// These cases can't legally follow a type arg list. However, they're not
		// legal expressions either. The user is probably in the middle of a
		// generic type. So treat it as such.
		js_lexer.TDot,                     // foo<x>.
		js_lexer.TCloseParen,              // foo<x>)
		js_lexer.TCloseBracket,            // foo<x>]
		js_lexer.TColon,                   // foo<x>:
		js_lexer.TSemicolon,               // foo<x>;
		js_lexer.TQuestion,                // foo<x>?
		js_lexer.TEqualsEquals,            // foo<x> ==
		js_lexer.TEqualsEqualsEquals,      // foo<x> ===
		js_lexer.TExclamationEquals,       // foo<x> !=
		js_lexer.TExclamationEqualsEquals, // foo<x> !==
		js_lexer.TAmpersandAmpersand,      // foo<x> &&
		js_lexer.TBarBar,                  // foo<x> ||
		js_lexer.TQuestionQuestion,        // foo<x> ??
		js_lexer.TCaret,                   // foo<x> ^
		js_lexer.TAmpersand,               // foo<x> &
		js_lexer.TBar,                     // foo<x> |
		js_lexer.TCloseBrace,              // foo<x> }
		js_lexer.TEndOfFile:               // foo<x>
		return true

	case
		// We don't want to treat these as type arguments. Otherwise we'll parse
		// this as an invocation expression. Instead, we want to parse out the
		// expression in isolation from the type arguments.
		js_lexer.TComma,     // foo<x>,
		js_lexer.TOpenBrace: // foo<x> {
		return false

	default:
		// Anything else treat as an expression.
		return false
	}
}

func (p *parser) skipTypeScriptInterfaceStmt(opts parseStmtOpts) {
	name := p.lexer.Identifier
	p.lexer.Expect(js_lexer.TIdentifier)

	if opts.isModuleScope {
		p.localTypeNames[name] = true
	}

	p.skipTypeScriptTypeParameters()

	if p.lexer.Token == js_lexer.TExtends {
		p.lexer.Next()
		for {
			p.skipTypeScriptType(js_ast.LLowest)
			if p.lexer.Token != js_lexer.TComma {
				break
			}
			p.lexer.Next()
		}
	}

	if p.lexer.IsContextualKeyword("implements") {
		p.lexer.Next()
		for {
			p.skipTypeScriptType(js_ast.LLowest)
			if p.lexer.Token != js_lexer.TComma {
				break
			}
			p.lexer.Next()
		}
	}

	p.skipTypeScriptObjectType()
}

func (p *parser) skipTypeScriptTypeStmt(opts parseStmtOpts) {
	if opts.isExport && p.lexer.Token == js_lexer.TOpenBrace {
		// "export type {foo}"
		// "export type {foo} from 'bar'"
		p.parseExportClause()
		if p.lexer.IsContextualKeyword("from") {
			p.lexer.Next()
			p.parsePath()
		}
		p.lexer.ExpectOrInsertSemicolon()
		return
	}

	name := p.lexer.Identifier
	p.lexer.Expect(js_lexer.TIdentifier)

	if opts.isModuleScope {
		p.localTypeNames[name] = true
	}

	p.skipTypeScriptTypeParameters()
	p.lexer.Expect(js_lexer.TEquals)
	p.skipTypeScriptType(js_ast.LLowest)
	p.lexer.ExpectOrInsertSemicolon()
}

func (p *parser) parseTypeScriptDecorators() []js_ast.Expr {
	var tsDecorators []js_ast.Expr
	if p.options.ts.Parse {
		for p.lexer.Token == js_lexer.TAt {
			p.lexer.Next()

			// Parse a new/call expression with "exprFlagTSDecorator" so we ignore
			// EIndex expressions, since they may be part of a computed property:
			//
			//   class Foo {
			//     @foo ['computed']() {}
			//   }
			//
			// This matches the behavior of the TypeScript compiler.
			tsDecorators = append(tsDecorators, p.parseExprWithFlags(js_ast.LNew, exprFlagTSDecorator))
		}
	}
	return tsDecorators
}

func (p *parser) parseTypeScriptEnumStmt(loc logger.Loc, opts parseStmtOpts) js_ast.Stmt {
	p.lexer.Expect(js_lexer.TEnum)
	nameLoc := p.lexer.Loc()
	nameText := p.lexer.Identifier
	p.lexer.Expect(js_lexer.TIdentifier)
	name := js_ast.LocRef{Loc: nameLoc, Ref: js_ast.InvalidRef}

	// Generate the namespace object
	exportedMembers := p.getOrCreateExportedNamespaceMembers(nameText, opts.isExport)
	tsNamespace := &js_ast.TSNamespaceScope{
		ExportedMembers: exportedMembers,
		ArgRef:          js_ast.InvalidRef,
		IsEnumScope:     true,
	}
	enumMemberData := &js_ast.TSNamespaceMemberNamespace{
		ExportedMembers: exportedMembers,
	}

	// Declare the enum and create the scope
	if !opts.isTypeScriptDeclare {
		name.Ref = p.declareSymbol(js_ast.SymbolTSEnum, nameLoc, nameText)
		p.pushScopeForParsePass(js_ast.ScopeEntry, loc)
		p.currentScope.TSNamespace = tsNamespace
		p.refToTSNamespaceMemberData[name.Ref] = enumMemberData
	}

	p.lexer.Expect(js_lexer.TOpenBrace)
	values := []js_ast.EnumValue{}

	oldFnOrArrowData := p.fnOrArrowDataParse
	p.fnOrArrowDataParse = fnOrArrowDataParse{
		isThisDisallowed: true,
		needsAsyncLoc:    logger.Loc{Start: -1},
	}

	// Parse the body
	for p.lexer.Token != js_lexer.TCloseBrace {
		nameRange := p.lexer.Range()
		value := js_ast.EnumValue{
			Loc: nameRange.Loc,
			Ref: js_ast.InvalidRef,
		}

		// Parse the name
		var nameText string
		if p.lexer.Token == js_lexer.TStringLiteral {
			value.Name = p.lexer.StringLiteral()
			nameText = js_lexer.UTF16ToString(value.Name)
		} else if p.lexer.IsIdentifierOrKeyword() {
			nameText = p.lexer.Identifier
			value.Name = js_lexer.StringToUTF16(nameText)
		} else {
			p.lexer.Expect(js_lexer.TIdentifier)
		}
		p.lexer.Next()

		// Identifiers can be referenced by other values
		if !opts.isTypeScriptDeclare && js_lexer.IsIdentifierUTF16(value.Name) {
			value.Ref = p.declareSymbol(js_ast.SymbolOther, value.Loc, js_lexer.UTF16ToString(value.Name))
		}

		// Parse the initializer
		if p.lexer.Token == js_lexer.TEquals {
			p.lexer.Next()
			value.ValueOrNil = p.parseExpr(js_ast.LComma)
		}

		values = append(values, value)

		// Add this enum value as a member of the enum's namespace
		exportedMembers[nameText] = js_ast.TSNamespaceMember{
			Loc:         value.Loc,
			Data:        &js_ast.TSNamespaceMemberProperty{},
			IsEnumValue: true,
		}

		if p.lexer.Token != js_lexer.TComma && p.lexer.Token != js_lexer.TSemicolon {
			if p.lexer.IsIdentifierOrKeyword() || p.lexer.Token == js_lexer.TStringLiteral {
				var errorLoc logger.Loc
				var errorText string

				if value.ValueOrNil.Data == nil {
					errorLoc = logger.Loc{Start: nameRange.End()}
					errorText = fmt.Sprintf("Expected \",\" after %q in enum", nameText)
				} else {
					var nextName string
					if p.lexer.Token == js_lexer.TStringLiteral {
						nextName = js_lexer.UTF16ToString(p.lexer.StringLiteral())
					} else {
						nextName = p.lexer.Identifier
					}
					errorLoc = p.lexer.Loc()
					errorText = fmt.Sprintf("Expected \",\" before %q in enum", nextName)
				}

				data := p.tracker.MsgData(logger.Range{Loc: errorLoc}, errorText)
				data.Location.Suggestion = ","
				p.log.AddMsg(logger.Msg{Kind: logger.Error, Data: data})
				panic(js_lexer.LexerPanic{})
			}
			break
		}
		p.lexer.Next()
	}

	p.fnOrArrowDataParse = oldFnOrArrowData

	if !opts.isTypeScriptDeclare {
		// Avoid a collision with the enum closure argument variable if the
		// enum exports a symbol with the same name as the enum itself:
		//
		//   enum foo {
		//     foo = 123,
		//     bar = foo,
		//   }
		//
		// TypeScript generates the following code in this case:
		//
		//   var foo;
		//   (function (foo) {
		//     foo[foo["foo"] = 123] = "foo";
		//     foo[foo["bar"] = 123] = "bar";
		//   })(foo || (foo = {}));
		//
		// Whereas in this case:
		//
		//   enum foo {
		//     bar = foo as any,
		//   }
		//
		// TypeScript generates the following code:
		//
		//   var foo;
		//   (function (foo) {
		//     foo[foo["bar"] = foo] = "bar";
		//   })(foo || (foo = {}));
		//
		if _, ok := p.currentScope.Members[nameText]; ok {
			// Add a "_" to make tests easier to read, since non-bundler tests don't
			// run the renamer. For external-facing things the renamer will avoid
			// collisions automatically so this isn't important for correctness.
			tsNamespace.ArgRef = p.newSymbol(js_ast.SymbolHoisted, "_"+nameText)
			p.currentScope.Generated = append(p.currentScope.Generated, tsNamespace.ArgRef)
		} else {
			tsNamespace.ArgRef = p.declareSymbol(js_ast.SymbolHoisted, nameLoc, nameText)
		}
		p.refToTSNamespaceMemberData[tsNamespace.ArgRef] = enumMemberData

		p.popScope()
	}

	p.lexer.Expect(js_lexer.TCloseBrace)

	if opts.isTypeScriptDeclare {
		if opts.isNamespaceScope && opts.isExport {
			p.hasNonLocalExportDeclareInsideNamespace = true
		}

		return js_ast.Stmt{Loc: loc, Data: &js_ast.STypeScript{}}
	}

	return js_ast.Stmt{Loc: loc, Data: &js_ast.SEnum{
		Name:     name,
		Arg:      tsNamespace.ArgRef,
		Values:   values,
		IsExport: opts.isExport,
	}}
}

// This assumes the caller has already parsed the "import" token
func (p *parser) parseTypeScriptImportEqualsStmt(loc logger.Loc, opts parseStmtOpts, defaultNameLoc logger.Loc, defaultName string) js_ast.Stmt {
	p.lexer.Expect(js_lexer.TEquals)

	kind := p.selectLocalKind(js_ast.LocalConst)
	name := p.lexer.Identifier
	value := js_ast.Expr{Loc: p.lexer.Loc(), Data: &js_ast.EIdentifier{Ref: p.storeNameInRef(name)}}
	p.lexer.Expect(js_lexer.TIdentifier)

	if name == "require" && p.lexer.Token == js_lexer.TOpenParen {
		// "import ns = require('x')"
		p.lexer.Next()
		path := js_ast.Expr{Loc: p.lexer.Loc(), Data: &js_ast.EString{Value: p.lexer.StringLiteral()}}
		p.lexer.Expect(js_lexer.TStringLiteral)
		p.lexer.Expect(js_lexer.TCloseParen)
		value.Data = &js_ast.ECall{
			Target: value,
			Args:   []js_ast.Expr{path},
		}
	} else {
		// "import Foo = Bar"
		// "import Foo = Bar.Baz"
		for p.lexer.Token == js_lexer.TDot {
			p.lexer.Next()
			value.Data = &js_ast.EDot{
				Target:  value,
				Name:    p.lexer.Identifier,
				NameLoc: p.lexer.Loc(),
			}
			p.lexer.Expect(js_lexer.TIdentifier)
		}
	}

	p.lexer.ExpectOrInsertSemicolon()

	if opts.isTypeScriptDeclare {
		// "import type foo = require('bar');"
		// "import type foo = bar.baz;"
		return js_ast.Stmt{Loc: loc, Data: &js_ast.STypeScript{}}
	}

	ref := p.declareSymbol(js_ast.SymbolConst, defaultNameLoc, defaultName)
	decls := []js_ast.Decl{{
		Binding:    js_ast.Binding{Loc: defaultNameLoc, Data: &js_ast.BIdentifier{Ref: ref}},
		ValueOrNil: value,
	}}

	return js_ast.Stmt{Loc: loc, Data: &js_ast.SLocal{
		Kind:              kind,
		Decls:             decls,
		IsExport:          opts.isExport,
		WasTSImportEquals: true,
	}}
}

// Generate a TypeScript namespace object for this namespace's scope. If this
// namespace is another block that is to be merged with an existing namespace,
// use that earlier namespace's object instead.
func (p *parser) getOrCreateExportedNamespaceMembers(name string, isExport bool) js_ast.TSNamespaceMembers {
	// Merge with a sibling namespace from the same scope
	if existingMember, ok := p.currentScope.Members[name]; ok {
		if memberData, ok := p.refToTSNamespaceMemberData[existingMember.Ref]; ok {
			if nsMemberData, ok := memberData.(*js_ast.TSNamespaceMemberNamespace); ok {
				return nsMemberData.ExportedMembers
			}
		}
	}

	// Merge with a sibling namespace from a different scope
	if isExport {
		if parentNamespace := p.currentScope.TSNamespace; parentNamespace != nil {
			if existing, ok := parentNamespace.ExportedMembers[name]; ok {
				if existing, ok := existing.Data.(*js_ast.TSNamespaceMemberNamespace); ok {
					return existing.ExportedMembers
				}
			}
		}
	}

	// Otherwise, generate a new namespace object
	return make(js_ast.TSNamespaceMembers)
}

func (p *parser) parseTypeScriptNamespaceStmt(loc logger.Loc, opts parseStmtOpts) js_ast.Stmt {
	// "namespace Foo {}"
	nameLoc := p.lexer.Loc()
	nameText := p.lexer.Identifier
	p.lexer.Next()

	// Generate the namespace object
	exportedMembers := p.getOrCreateExportedNamespaceMembers(nameText, opts.isExport)
	tsNamespace := &js_ast.TSNamespaceScope{
		ExportedMembers: exportedMembers,
		ArgRef:          js_ast.InvalidRef,
	}
	nsMemberData := &js_ast.TSNamespaceMemberNamespace{
		ExportedMembers: exportedMembers,
	}

	// Declare the namespace and create the scope
	name := js_ast.LocRef{Loc: nameLoc, Ref: js_ast.InvalidRef}
	scopeIndex := p.pushScopeForParsePass(js_ast.ScopeEntry, loc)
	p.currentScope.TSNamespace = tsNamespace

	oldHasNonLocalExportDeclareInsideNamespace := p.hasNonLocalExportDeclareInsideNamespace
	oldFnOrArrowData := p.fnOrArrowDataParse
	p.hasNonLocalExportDeclareInsideNamespace = false
	p.fnOrArrowDataParse = fnOrArrowDataParse{
		isThisDisallowed:   true,
		isReturnDisallowed: true,
		needsAsyncLoc:      logger.Loc{Start: -1},
	}

	// Parse the statements inside the namespace
	var stmts []js_ast.Stmt
	if p.lexer.Token == js_lexer.TDot {
		dotLoc := p.lexer.Loc()
		p.lexer.Next()
		stmts = []js_ast.Stmt{p.parseTypeScriptNamespaceStmt(dotLoc, parseStmtOpts{
			isExport:            true,
			isNamespaceScope:    true,
			isTypeScriptDeclare: opts.isTypeScriptDeclare,
		})}
	} else if opts.isTypeScriptDeclare && p.lexer.Token != js_lexer.TOpenBrace {
		p.lexer.ExpectOrInsertSemicolon()
	} else {
		p.lexer.Expect(js_lexer.TOpenBrace)
		stmts = p.parseStmtsUpTo(js_lexer.TCloseBrace, parseStmtOpts{
			isNamespaceScope:    true,
			isTypeScriptDeclare: opts.isTypeScriptDeclare,
		})
		p.lexer.Next()
	}

	hasNonLocalExportDeclareInsideNamespace := p.hasNonLocalExportDeclareInsideNamespace
	p.hasNonLocalExportDeclareInsideNamespace = oldHasNonLocalExportDeclareInsideNamespace
	p.fnOrArrowDataParse = oldFnOrArrowData

	// Add any exported members from this namespace's body as members of the
	// associated namespace object.
	for _, stmt := range stmts {
		switch s := stmt.Data.(type) {
		case *js_ast.SFunction:
			if s.IsExport {
				name := p.symbols[s.Fn.Name.Ref.InnerIndex].OriginalName
				member := js_ast.TSNamespaceMember{
					Loc:  s.Fn.Name.Loc,
					Data: &js_ast.TSNamespaceMemberProperty{},
				}
				exportedMembers[name] = member
				p.refToTSNamespaceMemberData[s.Fn.Name.Ref] = member.Data
			}

		case *js_ast.SClass:
			if s.IsExport {
				name := p.symbols[s.Class.Name.Ref.InnerIndex].OriginalName
				member := js_ast.TSNamespaceMember{
					Loc:  s.Class.Name.Loc,
					Data: &js_ast.TSNamespaceMemberProperty{},
				}
				exportedMembers[name] = member
				p.refToTSNamespaceMemberData[s.Class.Name.Ref] = member.Data
			}

		case *js_ast.SNamespace:
			if s.IsExport {
				if memberData, ok := p.refToTSNamespaceMemberData[s.Name.Ref]; ok {
					if nsMemberData, ok := memberData.(*js_ast.TSNamespaceMemberNamespace); ok {
						member := js_ast.TSNamespaceMember{
							Loc: s.Name.Loc,
							Data: &js_ast.TSNamespaceMemberNamespace{
								ExportedMembers: nsMemberData.ExportedMembers,
							},
						}
						exportedMembers[p.symbols[s.Name.Ref.InnerIndex].OriginalName] = member
						p.refToTSNamespaceMemberData[s.Name.Ref] = member.Data
					}
				}
			}

		case *js_ast.SEnum:
			if s.IsExport {
				if memberData, ok := p.refToTSNamespaceMemberData[s.Name.Ref]; ok {
					if nsMemberData, ok := memberData.(*js_ast.TSNamespaceMemberNamespace); ok {
						member := js_ast.TSNamespaceMember{
							Loc: s.Name.Loc,
							Data: &js_ast.TSNamespaceMemberNamespace{
								ExportedMembers: nsMemberData.ExportedMembers,
							},
						}
						exportedMembers[p.symbols[s.Name.Ref.InnerIndex].OriginalName] = member
						p.refToTSNamespaceMemberData[s.Name.Ref] = member.Data
					}
				}
			}

		case *js_ast.SLocal:
			if s.IsExport {
				p.exportDeclsInsideNamespace(exportedMembers, s.Decls)
			}
		}
	}

	// Import assignments may be only used in type expressions, not value
	// expressions. If this is the case, the TypeScript compiler removes
	// them entirely from the output. That can cause the namespace itself
	// to be considered empty and thus be removed.
	importEqualsCount := 0
	for _, stmt := range stmts {
		if local, ok := stmt.Data.(*js_ast.SLocal); ok && local.WasTSImportEquals && !local.IsExport {
			importEqualsCount++
		}
	}

	// TypeScript omits namespaces without values. These namespaces
	// are only allowed to be used in type expressions. They are
	// allowed to be exported, but can also only be used in type
	// expressions when imported. So we shouldn't count them as a
	// real export either.
	//
	// TypeScript also strangely counts namespaces containing only
	// "export declare" statements as non-empty even though "declare"
	// statements are only type annotations. We cannot omit the namespace
	// in that case. See https://github.com/evanw/esbuild/issues/1158.
	if (len(stmts) == importEqualsCount && !hasNonLocalExportDeclareInsideNamespace) || opts.isTypeScriptDeclare {
		p.popAndDiscardScope(scopeIndex)
		if opts.isModuleScope {
			p.localTypeNames[nameText] = true
		}
		return js_ast.Stmt{Loc: loc, Data: &js_ast.STypeScript{}}
	}

	if !opts.isTypeScriptDeclare {
		// Avoid a collision with the namespace closure argument variable if the
		// namespace exports a symbol with the same name as the namespace itself:
		//
		//   namespace foo {
		//     export let foo = 123
		//     console.log(foo)
		//   }
		//
		// TypeScript generates the following code in this case:
		//
		//   var foo;
		//   (function (foo_1) {
		//     foo_1.foo = 123;
		//     console.log(foo_1.foo);
		//   })(foo || (foo = {}));
		//
		if _, ok := p.currentScope.Members[nameText]; ok {
			// Add a "_" to make tests easier to read, since non-bundler tests don't
			// run the renamer. For external-facing things the renamer will avoid
			// collisions automatically so this isn't important for correctness.
			tsNamespace.ArgRef = p.newSymbol(js_ast.SymbolHoisted, "_"+nameText)
			p.currentScope.Generated = append(p.currentScope.Generated, tsNamespace.ArgRef)
		} else {
			tsNamespace.ArgRef = p.declareSymbol(js_ast.SymbolHoisted, nameLoc, nameText)
		}
		p.refToTSNamespaceMemberData[tsNamespace.ArgRef] = nsMemberData
	}

	p.popScope()
	if !opts.isTypeScriptDeclare {
		name.Ref = p.declareSymbol(js_ast.SymbolTSNamespace, nameLoc, nameText)
		p.refToTSNamespaceMemberData[name.Ref] = nsMemberData
	}
	return js_ast.Stmt{Loc: loc, Data: &js_ast.SNamespace{
		Name:     name,
		Arg:      tsNamespace.ArgRef,
		Stmts:    stmts,
		IsExport: opts.isExport,
	}}
}

func (p *parser) exportDeclsInsideNamespace(exportedMembers js_ast.TSNamespaceMembers, decls []js_ast.Decl) {
	for _, decl := range decls {
		p.exportBindingInsideNamespace(exportedMembers, decl.Binding)
	}
}

func (p *parser) exportBindingInsideNamespace(exportedMembers js_ast.TSNamespaceMembers, binding js_ast.Binding) {
	switch b := binding.Data.(type) {
	case *js_ast.BMissing:

	case *js_ast.BIdentifier:
		name := p.symbols[b.Ref.InnerIndex].OriginalName
		member := js_ast.TSNamespaceMember{
			Loc:  binding.Loc,
			Data: &js_ast.TSNamespaceMemberProperty{},
		}
		exportedMembers[name] = member
		p.refToTSNamespaceMemberData[b.Ref] = member.Data

	case *js_ast.BArray:
		for _, item := range b.Items {
			p.exportBindingInsideNamespace(exportedMembers, item.Binding)
		}

	case *js_ast.BObject:
		for _, property := range b.Properties {
			p.exportBindingInsideNamespace(exportedMembers, property.Value)
		}

	default:
		panic("Internal error")
	}
}

func (p *parser) generateClosureForTypeScriptNamespaceOrEnum(
	stmts []js_ast.Stmt, stmtLoc logger.Loc, isExport bool, nameLoc logger.Loc,
	nameRef js_ast.Ref, argRef js_ast.Ref, stmtsInsideClosure []js_ast.Stmt,
) []js_ast.Stmt {
	// Follow the link chain in case symbols were merged
	symbol := p.symbols[nameRef.InnerIndex]
	for symbol.Link != js_ast.InvalidRef {
		nameRef = symbol.Link
		symbol = p.symbols[nameRef.InnerIndex]
	}

	// Make sure to only emit a variable once for a given namespace, since there
	// can be multiple namespace blocks for the same namespace
	if (symbol.Kind == js_ast.SymbolTSNamespace || symbol.Kind == js_ast.SymbolTSEnum) && !p.emittedNamespaceVars[nameRef] {
		decls := []js_ast.Decl{{Binding: js_ast.Binding{Loc: nameLoc, Data: &js_ast.BIdentifier{Ref: nameRef}}}}
		p.emittedNamespaceVars[nameRef] = true
		if p.currentScope == p.moduleScope {
			// Top-level namespace: "var"
			stmts = append(stmts, js_ast.Stmt{Loc: stmtLoc, Data: &js_ast.SLocal{
				Kind:     js_ast.LocalVar,
				Decls:    decls,
				IsExport: isExport,
			}})
		} else {
			// Nested namespace: "let"
			stmts = append(stmts, js_ast.Stmt{Loc: stmtLoc, Data: &js_ast.SLocal{
				Kind:  js_ast.LocalLet,
				Decls: decls,
			}})
		}
	}

	var argExpr js_ast.Expr
	if isExport && p.enclosingNamespaceArgRef != nil {
		// "name = enclosing.name || (enclosing.name = {})"
		name := p.symbols[nameRef.InnerIndex].OriginalName
		argExpr = js_ast.Assign(
			js_ast.Expr{Loc: nameLoc, Data: &js_ast.EIdentifier{Ref: nameRef}},
			js_ast.Expr{Loc: nameLoc, Data: &js_ast.EBinary{
				Op: js_ast.BinOpLogicalOr,
				Left: js_ast.Expr{Loc: nameLoc, Data: &js_ast.EDot{
					Target:  js_ast.Expr{Loc: nameLoc, Data: &js_ast.EIdentifier{Ref: *p.enclosingNamespaceArgRef}},
					Name:    name,
					NameLoc: nameLoc,
				}},
				Right: js_ast.Assign(
					js_ast.Expr{Loc: nameLoc, Data: &js_ast.EDot{
						Target:  js_ast.Expr{Loc: nameLoc, Data: &js_ast.EIdentifier{Ref: *p.enclosingNamespaceArgRef}},
						Name:    name,
						NameLoc: nameLoc,
					}},
					js_ast.Expr{Loc: nameLoc, Data: &js_ast.EObject{}},
				),
			}},
		)
		p.recordUsage(*p.enclosingNamespaceArgRef)
		p.recordUsage(*p.enclosingNamespaceArgRef)
		p.recordUsage(nameRef)
	} else {
		// "name || (name = {})"
		argExpr = js_ast.Expr{Loc: nameLoc, Data: &js_ast.EBinary{
			Op:   js_ast.BinOpLogicalOr,
			Left: js_ast.Expr{Loc: nameLoc, Data: &js_ast.EIdentifier{Ref: nameRef}},
			Right: js_ast.Assign(
				js_ast.Expr{Loc: nameLoc, Data: &js_ast.EIdentifier{Ref: nameRef}},
				js_ast.Expr{Loc: nameLoc, Data: &js_ast.EObject{}},
			),
		}}
		p.recordUsage(nameRef)
		p.recordUsage(nameRef)
	}

	// Try to use an arrow function if possible for compactness
	var targetExpr js_ast.Expr
	args := []js_ast.Arg{{Binding: js_ast.Binding{Loc: nameLoc, Data: &js_ast.BIdentifier{Ref: argRef}}}}
	if p.options.unsupportedJSFeatures.Has(compat.Arrow) {
		targetExpr = js_ast.Expr{Loc: stmtLoc, Data: &js_ast.EFunction{Fn: js_ast.Fn{
			Args: args,
			Body: js_ast.FnBody{Loc: stmtLoc, Stmts: stmtsInsideClosure},
		}}}
	} else {
		targetExpr = js_ast.Expr{Loc: stmtLoc, Data: &js_ast.EArrow{
			Args: args,
			Body: js_ast.FnBody{Loc: stmtLoc, Stmts: stmtsInsideClosure},
		}}
	}

	// Call the closure with the name object
	stmts = append(stmts, js_ast.Stmt{Loc: stmtLoc, Data: &js_ast.SExpr{Value: js_ast.Expr{Loc: stmtLoc, Data: &js_ast.ECall{
		Target: targetExpr,
		Args:   []js_ast.Expr{argExpr},
	}}}})

	return stmts
}

func (p *parser) generateClosureForTypeScriptEnum(
	stmts []js_ast.Stmt, stmtLoc logger.Loc, isExport bool, nameLoc logger.Loc,
	nameRef js_ast.Ref, argRef js_ast.Ref, exprsInsideClosure []js_ast.Expr,
	allValuesArePure bool,
) []js_ast.Stmt {
	// Bail back to the namespace code for enums that aren't at the top level.
	// Doing this for nested enums is problematic for two reasons. First of all
	// enums inside of namespaces must be property accesses off the namespace
	// object instead of variable declarations. Also we'd need to use "let"
	// instead of "var" which doesn't allow sibling declarations to be merged.
	if p.currentScope != p.moduleScope {
		stmtsInsideClosure := []js_ast.Stmt{}
		if len(exprsInsideClosure) > 0 {
			if p.options.mangleSyntax {
				// "a; b; c;" => "a, b, c;"
				joined := js_ast.JoinAllWithComma(exprsInsideClosure)
				stmtsInsideClosure = append(stmtsInsideClosure, js_ast.Stmt{Loc: joined.Loc, Data: &js_ast.SExpr{Value: joined}})
			} else {
				for _, expr := range exprsInsideClosure {
					stmtsInsideClosure = append(stmtsInsideClosure, js_ast.Stmt{Loc: expr.Loc, Data: &js_ast.SExpr{Value: expr}})
				}
			}
		}
		return p.generateClosureForTypeScriptNamespaceOrEnum(
			stmts, stmtLoc, isExport, nameLoc, nameRef, argRef, stmtsInsideClosure)
	}

	// This uses an output format for enums that's different but equivalent to
	// what TypeScript uses. Here is TypeScript's output:
	//
	//   var x;
	//   (function (x) {
	//     x[x["y"] = 1] = "y";
	//   })(x || (x = {}));
	//
	// And here's our output:
	//
	//   var x = /* @__PURE__ */ ((x) => {
	//     x[x["y"] = 1] = "y";
	//     return x;
	//   })(x || {});
	//
	// One benefit is that the minified output is smaller:
	//
	//   // Old output minified
	//   var x;(function(n){n[n.y=1]="y"})(x||(x={}));
	//
	//   // New output minified
	//   var x=(r=>(r[r.y=1]="y",r))(x||{});
	//
	// Another benefit is that the @__PURE__ annotation means it automatically
	// works with tree-shaking, even with more advanced features such as sibling
	// enum declarations and enum/namespace merges. Ideally all uses of the enum
	// are just direct references to enum members (and are therefore inlined as
	// long as the enum value is a constant) and the enum definition itself is
	// unused and can be removed as dead code.

	// Follow the link chain in case symbols were merged
	symbol := p.symbols[nameRef.InnerIndex]
	for symbol.Link != js_ast.InvalidRef {
		nameRef = symbol.Link
		symbol = p.symbols[nameRef.InnerIndex]
	}

	// Generate the body of the closure, including a return statement at the end
	stmtsInsideClosure := []js_ast.Stmt{}
	if len(exprsInsideClosure) > 0 {
		argExpr := js_ast.Expr{Loc: nameLoc, Data: &js_ast.EIdentifier{Ref: argRef}}
		if p.options.mangleSyntax {
			// "a; b; return c;" => "return a, b, c;"
			joined := js_ast.JoinAllWithComma(exprsInsideClosure)
			joined = js_ast.JoinWithComma(joined, argExpr)
			stmtsInsideClosure = append(stmtsInsideClosure, js_ast.Stmt{Loc: joined.Loc, Data: &js_ast.SReturn{ValueOrNil: joined}})
		} else {
			for _, expr := range exprsInsideClosure {
				stmtsInsideClosure = append(stmtsInsideClosure, js_ast.Stmt{Loc: expr.Loc, Data: &js_ast.SExpr{Value: expr}})
			}
			stmtsInsideClosure = append(stmtsInsideClosure, js_ast.Stmt{Loc: argExpr.Loc, Data: &js_ast.SReturn{ValueOrNil: argExpr}})
		}
	}

	// Try to use an arrow function if possible for compactness
	var targetExpr js_ast.Expr
	args := []js_ast.Arg{{Binding: js_ast.Binding{Loc: nameLoc, Data: &js_ast.BIdentifier{Ref: argRef}}}}
	if p.options.unsupportedJSFeatures.Has(compat.Arrow) {
		targetExpr = js_ast.Expr{Loc: stmtLoc, Data: &js_ast.EFunction{Fn: js_ast.Fn{
			Args: args,
			Body: js_ast.FnBody{Loc: stmtLoc, Stmts: stmtsInsideClosure},
		}}}
	} else {
		targetExpr = js_ast.Expr{Loc: stmtLoc, Data: &js_ast.EArrow{
			Args:       args,
			Body:       js_ast.FnBody{Loc: stmtLoc, Stmts: stmtsInsideClosure},
			PreferExpr: p.options.mangleSyntax,
		}}
	}

	// Call the closure with the name object and store it to the variable
	decls := []js_ast.Decl{{
		Binding: js_ast.Binding{Loc: nameLoc, Data: &js_ast.BIdentifier{Ref: nameRef}},
		ValueOrNil: js_ast.Expr{Loc: stmtLoc, Data: &js_ast.ECall{
			Target: targetExpr,
			Args: []js_ast.Expr{{Loc: nameLoc, Data: &js_ast.EBinary{
				Op:    js_ast.BinOpLogicalOr,
				Left:  js_ast.Expr{Loc: nameLoc, Data: &js_ast.EIdentifier{Ref: nameRef}},
				Right: js_ast.Expr{Loc: nameLoc, Data: &js_ast.EObject{}},
			}}},
			CanBeUnwrappedIfUnused: allValuesArePure,
		}},
	}}
	p.recordUsage(nameRef)

	// Use a "var" statement since this is a top-level enum, but only use "export" once
	stmts = append(stmts, js_ast.Stmt{Loc: stmtLoc, Data: &js_ast.SLocal{
		Kind:     js_ast.LocalVar,
		Decls:    decls,
		IsExport: isExport && !p.emittedNamespaceVars[nameRef],
	}})
	p.emittedNamespaceVars[nameRef] = true

	return stmts
}

func (p *parser) wrapInlinedEnum(value js_ast.Expr, comment string) js_ast.Expr {
	if p.shouldFoldNumericConstants || p.options.mangleSyntax || strings.Contains(comment, "*/") {
		// Don't wrap with a comment
		return value
	}

	// Wrap with a comment
	return js_ast.Expr{Loc: value.Loc, Data: &js_ast.EInlinedEnum{
		Value:   value,
		Comment: comment,
	}}
}
