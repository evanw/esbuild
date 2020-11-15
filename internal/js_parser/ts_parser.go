// This file contains code for parsing TypeScript syntax. The parser just skips
// over type expressions as if they are whitespace and doesn't bother generating
// an AST because nothing uses type information.

package js_parser

import (
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
	// Skip over "function assert(x: boolean): asserts x"
	if p.lexer.IsContextualKeyword("asserts") {
		p.lexer.Next()

		// "function assert(x: boolean): asserts" is also valid
		if p.lexer.Token != js_lexer.TIdentifier && p.lexer.Token != js_lexer.TThis {
			return
		}
		p.lexer.Next()

		// Continue on to the "is" check below to handle something like
		// "function assert(x: any): asserts x is boolean"
	} else {
		p.skipTypeScriptType(js_ast.LLowest)
	}

	if p.lexer.IsContextualKeyword("is") && !p.lexer.HasNewlineBefore {
		p.lexer.Next()
		p.skipTypeScriptType(js_ast.LLowest)
	}
}

func (p *parser) skipTypeScriptType(level js_ast.L) {
	p.skipTypeScriptTypePrefix()
	p.skipTypeScriptTypeSuffix(level)
}

func (p *parser) skipTypeScriptTypePrefix() {
	switch p.lexer.Token {
	case js_lexer.TNumericLiteral, js_lexer.TBigIntegerLiteral, js_lexer.TStringLiteral,
		js_lexer.TNoSubstitutionTemplateLiteral, js_lexer.TThis, js_lexer.TTrue, js_lexer.TFalse,
		js_lexer.TNull, js_lexer.TVoid, js_lexer.TConst:
		p.lexer.Next()

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
		p.skipTypeScriptTypePrefix()

	case js_lexer.TImport:
		// "import('fs')"
		p.lexer.Next()
		p.lexer.Expect(js_lexer.TOpenParen)
		p.lexer.Expect(js_lexer.TStringLiteral)
		p.lexer.Expect(js_lexer.TCloseParen)

	case js_lexer.TNew:
		// "new () => Foo"
		// "new <T>() => Foo<T>"
		p.lexer.Next()
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
		switch p.lexer.Identifier {
		case "keyof", "readonly", "infer":
			p.lexer.Next()
			p.skipTypeScriptType(js_ast.LPrefix)

		case "unique":
			p.lexer.Next()
			if p.lexer.IsContextualKeyword("symbol") {
				p.lexer.Next()
			}

		// This was added in TypeScript 4.2
		case "abstract":
			p.lexer.Next()
			if p.lexer.Token == js_lexer.TNew {
				p.skipTypeScriptTypePrefix()
			}

		default:
			p.lexer.Next()
		}

	case js_lexer.TTypeof:
		p.lexer.Next()
		if p.lexer.Token == js_lexer.TImport {
			// "typeof import('fs')"
			p.skipTypeScriptTypePrefix()
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
			p.skipTypeScriptType(js_ast.LLowest)
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
		p.lexer.Unexpected()
	}
}

func (p *parser) skipTypeScriptTypeSuffix(level js_ast.L) {
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

		case js_lexer.TLessThan, js_lexer.TLessThanEquals,
			js_lexer.TLessThanLessThan, js_lexer.TLessThanLessThanEquals:
			// "let foo: any \n <number>foo" must not become a single type
			if p.lexer.HasNewlineBefore {
				return
			}
			p.lexer.ExpectLessThan(false /* isInsideJSXElement */)
			for {
				p.skipTypeScriptType(js_ast.LLowest)
				if p.lexer.Token != js_lexer.TComma {
					break
				}
				p.lexer.Next()
			}
			p.lexer.ExpectGreaterThan(false /* isInsideJSXElement */)

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
	if p.lexer.Token != js_lexer.TLessThan {
		return false
	}

	p.lexer.Next()

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

func (p *parser) parseTypeScriptEnumStmt(loc logger.Loc, opts parseStmtOpts, isConst bool) js_ast.Stmt {
	p.lexer.Expect(js_lexer.TEnum)
	nameLoc := p.lexer.Loc()
	nameText := p.lexer.Identifier
	p.lexer.Expect(js_lexer.TIdentifier)
	name := js_ast.LocRef{Loc: nameLoc, Ref: js_ast.InvalidRef}
	argRef := js_ast.InvalidRef
	if !opts.isTypeScriptDeclare {
		name.Ref = p.declareSymbol(js_ast.SymbolTSEnum, nameLoc, nameText)
		p.pushScopeForParsePass(js_ast.ScopeEntry, loc)
		argRef = p.declareSymbol(js_ast.SymbolHoisted, nameLoc, nameText)
	}
	p.lexer.Expect(js_lexer.TOpenBrace)

	values := []js_ast.EnumValue{}

	for p.lexer.Token != js_lexer.TCloseBrace {
		value := js_ast.EnumValue{
			Loc: p.lexer.Loc(),
			Ref: js_ast.InvalidRef,
		}

		// Parse the name
		if p.lexer.Token == js_lexer.TStringLiteral {
			value.Name = p.lexer.StringLiteral
		} else if p.lexer.IsIdentifierOrKeyword() {
			value.Name = js_lexer.StringToUTF16(p.lexer.Identifier)
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
			initializer := p.parseExpr(js_ast.LComma)
			value.Value = &initializer
		}

		values = append(values, value)

		if p.lexer.Token != js_lexer.TComma && p.lexer.Token != js_lexer.TSemicolon {
			break
		}
		p.lexer.Next()
	}

	if !opts.isTypeScriptDeclare {
		p.popScope()
	}

	p.lexer.Expect(js_lexer.TCloseBrace)
	return js_ast.Stmt{Loc: loc, Data: &js_ast.SEnum{
		Name:     name,
		Arg:      argRef,
		Values:   values,
		IsExport: opts.isExport,
		IsConst:  isConst,
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
		path := js_ast.Expr{Loc: p.lexer.Loc(), Data: &js_ast.EString{Value: p.lexer.StringLiteral}}
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
	ref := p.declareSymbol(js_ast.SymbolConst, defaultNameLoc, defaultName)
	decls := []js_ast.Decl{{
		Binding: js_ast.Binding{Loc: defaultNameLoc, Data: &js_ast.BIdentifier{Ref: ref}},
		Value:   &value,
	}}

	return js_ast.Stmt{Loc: loc, Data: &js_ast.SLocal{
		Kind:              kind,
		Decls:             decls,
		IsExport:          opts.isExport,
		WasTSImportEquals: true,
	}}
}

func (p *parser) parseTypeScriptNamespaceStmt(loc logger.Loc, opts parseStmtOpts) js_ast.Stmt {
	// "namespace Foo {}"
	nameLoc := p.lexer.Loc()
	nameText := p.lexer.Identifier
	p.lexer.Next()

	name := js_ast.LocRef{Loc: nameLoc, Ref: js_ast.InvalidRef}
	scopeIndex := p.pushScopeForParsePass(js_ast.ScopeEntry, loc)

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
	if len(stmts) == importEqualsCount || opts.isTypeScriptDeclare {
		p.popAndDiscardScope(scopeIndex)
		if opts.isModuleScope {
			p.localTypeNames[nameText] = true
		}
		return js_ast.Stmt{Loc: loc, Data: &js_ast.STypeScript{}}
	}

	argRef := js_ast.InvalidRef
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
			argRef = p.newSymbol(js_ast.SymbolHoisted, "_"+nameText)
			p.currentScope.Generated = append(p.currentScope.Generated, argRef)
		} else {
			argRef = p.declareSymbol(js_ast.SymbolHoisted, nameLoc, nameText)
		}
	}

	p.popScope()
	if !opts.isTypeScriptDeclare {
		name.Ref = p.declareSymbol(js_ast.SymbolTSNamespace, nameLoc, nameText)
	}
	return js_ast.Stmt{Loc: loc, Data: &js_ast.SNamespace{
		Name:     name,
		Arg:      argRef,
		Stmts:    stmts,
		IsExport: opts.isExport,
	}}
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
		p.emittedNamespaceVars[nameRef] = true
		if p.enclosingNamespaceArgRef == nil {
			// Top-level namespace
			stmts = append(stmts, js_ast.Stmt{Loc: stmtLoc, Data: &js_ast.SLocal{
				Kind:     js_ast.LocalVar,
				Decls:    []js_ast.Decl{{Binding: js_ast.Binding{Loc: nameLoc, Data: &js_ast.BIdentifier{Ref: nameRef}}}},
				IsExport: isExport,
			}})
		} else {
			// Nested namespace
			stmts = append(stmts, js_ast.Stmt{Loc: stmtLoc, Data: &js_ast.SLocal{
				Kind:  js_ast.LocalLet,
				Decls: []js_ast.Decl{{Binding: js_ast.Binding{Loc: nameLoc, Data: &js_ast.BIdentifier{Ref: nameRef}}}},
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

	// Call the closure with the name object
	stmts = append(stmts, js_ast.Stmt{Loc: stmtLoc, Data: &js_ast.SExpr{Value: js_ast.Expr{Loc: stmtLoc, Data: &js_ast.ECall{
		Target: js_ast.Expr{Loc: stmtLoc, Data: &js_ast.EFunction{Fn: js_ast.Fn{
			Args: []js_ast.Arg{{Binding: js_ast.Binding{Loc: nameLoc, Data: &js_ast.BIdentifier{Ref: argRef}}}},
			Body: js_ast.FnBody{Loc: stmtLoc, Stmts: stmtsInsideClosure},
		}}},
		Args: []js_ast.Expr{argExpr},
	}}}})

	return stmts
}
