// This file contains code for parsing TypeScript syntax. The parser just skips
// over type expressions as if they are whitespace and doesn't bother generating
// an AST because nothing uses type information.

package parser

import (
	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/lexer"
	"github.com/evanw/esbuild/internal/logger"
)

func (p *parser) skipFlowBinding() {
	switch p.lexer.Token {
	case lexer.TIdentifier, lexer.TThis:
		p.lexer.Next()

	case lexer.TOpenBracket:
		p.lexer.Next()

		// "[, , a]"
		for p.lexer.Token == lexer.TComma {
			p.lexer.Next()
		}

		// "[a, b]"
		for p.lexer.Token != lexer.TCloseBracket {
			p.skipFlowBinding()
			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
		}

		p.lexer.Expect(lexer.TCloseBracket)

	case lexer.TOpenBrace:
		p.lexer.Next()

		for p.lexer.Token != lexer.TCloseBrace {
			foundIdentifier := false

			switch p.lexer.Token {
			case lexer.TIdentifier:
				// "{x}"
				// "{x: y}"
				foundIdentifier = true
				p.lexer.Next()

				// "{1: y}"
				// "{'x': y}"
			case lexer.TStringLiteral, lexer.TNumericLiteral:
				p.lexer.Next()

			default:
				if p.lexer.IsIdentifierOrKeyword() {
					// "{if: x}"
					p.lexer.Next()
				} else {
					p.lexer.Unexpected()
				}
			}

			if p.lexer.Token == lexer.TColon || !foundIdentifier {
				p.lexer.Expect(lexer.TColon)
				p.skipFlowBinding()
			}

			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
		}

		p.lexer.Expect(lexer.TCloseBrace)

	default:
		p.lexer.Unexpected()
	}
}

func (p *parser) skipFlowFnArgs() {
	p.lexer.Expect(lexer.TOpenParen)

	for p.lexer.Token != lexer.TCloseParen {
		// "(...a)"
		if p.lexer.Token == lexer.TDotDotDot {
			p.lexer.Next()
		}

		p.skipFlowBinding()

		// "(a?)"
		if p.lexer.Token == lexer.TQuestion {
			p.lexer.Next()
		}

		// "(a: any)"
		if p.lexer.Token == lexer.TColon {
			p.lexer.Next()
			p.skipFlowType(ast.LLowest)
		}

		// "(a, b)"
		if p.lexer.Token != lexer.TComma {
			break
		}
		p.lexer.Next()
	}

	p.lexer.Expect(lexer.TCloseParen)
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
func (p *parser) skipFlowParenOrFnType() {
	if p.trySkipFlowArrowArgsWithBacktracking() {
		p.skipFlowReturnType()
	} else {
		p.lexer.Expect(lexer.TOpenParen)
		p.skipFlowType(ast.LLowest)
		p.lexer.Expect(lexer.TCloseParen)
	}
}

func (p *parser) skipFlowReturnType() {
	// Skip over "function assert(x: boolean): asserts x"
	if p.lexer.IsContextualKeyword("asserts") {
		p.lexer.Next()

		// "function assert(x: boolean): asserts" is also valid
		if p.lexer.Token != lexer.TIdentifier && p.lexer.Token != lexer.TThis {
			return
		}
		p.lexer.Next()

		// Continue on to the "is" check below to handle something like
		// "function assert(x: any): asserts x is boolean"
	} else {
		p.skipFlowType(ast.LLowest)
	}

	if p.lexer.IsContextualKeyword("is") && !p.lexer.HasNewlineBefore {
		p.lexer.Next()
		p.skipFlowType(ast.LLowest)
	}
}

func (p *parser) skipFlowType(level ast.L) {
	p.skipFlowTypePrefix()
	p.skipFlowTypeSuffix(level)
}

// Modified from TypeScript
func (p *parser) skipFlowTypePrefix() {
	switch p.lexer.Token {
	case lexer.TNumericLiteral, lexer.TBigIntegerLiteral, lexer.TStringLiteral,
		lexer.TNoSubstitutionTemplateLiteral, lexer.TThis, lexer.TTrue, lexer.TFalse,
		lexer.TNull, lexer.TVoid, lexer.TConst, lexer.TAsterisk: // Flow vs TS (Asterisk)
		p.lexer.Next()

	case lexer.TMinus:
		// "-123"
		// "-123n"
		p.lexer.Next()
		if p.lexer.Token == lexer.TBigIntegerLiteral {
			p.lexer.Next()
		} else {
			p.lexer.Expect(lexer.TNumericLiteral)
		}

	case lexer.TAmpersand:
	case lexer.TBar:
		// Support things like "type Foo = | A | B" and "type Foo = & A & B"
		p.lexer.Next()
		p.skipFlowTypePrefix()

	case lexer.TImport:
		// "import('fs')"
		p.lexer.Next()
		p.lexer.Expect(lexer.TOpenParen)
		p.lexer.Expect(lexer.TStringLiteral)
		p.lexer.Expect(lexer.TCloseParen)

	case lexer.TNew:
		// "new () => Foo"
		// "new <T>() => Foo<T>"
		p.lexer.Next()
		p.skipFlowTypeParameters()
		p.skipFlowParenOrFnType()

	case lexer.TLessThan:
		// "<T>() => Foo<T>"
		p.skipFlowTypeParameters()
		p.skipFlowParenOrFnType()

	case lexer.TOpenParen:
		// "(number | string)"
		p.skipFlowParenOrFnType()

	case lexer.TIdentifier:
		switch p.lexer.Identifier {
		case "keyof", "readonly", "infer":
			p.lexer.Next()
			p.skipFlowType(ast.LPrefix)

		case "unique":
			p.lexer.Next()
			if p.lexer.IsContextualKeyword("symbol") {
				p.lexer.Next()
			}

		default:
			p.lexer.Next()
		}

	case lexer.TTypeof:
		p.lexer.Next()
		if p.lexer.Token == lexer.TImport {
			// "typeof import('fs')"
			p.skipFlowTypePrefix()
		} else {
			// "typeof x"
			// "typeof x.y"
			for {
				if !p.lexer.IsIdentifierOrKeyword() {
					p.lexer.Expected(lexer.TIdentifier)
				}
				p.lexer.Next()
				if p.lexer.Token != lexer.TDot {
					break
				}
				p.lexer.Next()
			}
		}

	case lexer.TOpenBracket:
		// "[number, string]"
		// "[first: number, second: string]"
		p.lexer.Next()
		for p.lexer.Token != lexer.TCloseBracket {
			if p.lexer.Token == lexer.TDotDotDot {
				p.lexer.Next()
			}
			p.skipFlowType(ast.LLowest)
			if p.lexer.Token == lexer.TColon {
				p.lexer.Next()
				p.skipFlowType(ast.LLowest)
			}
			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
		}
		p.lexer.Expect(lexer.TCloseBracket)

	case lexer.TOpenBrace:
		p.skipFlowObjectType()

	default:
		p.lexer.Unexpected()
	}
}

func (p *parser) skipFlowTypeSuffix(level ast.L) {
	for {
		switch p.lexer.Token {
		case lexer.TBar:
			if level >= ast.LBitwiseOr {
				return
			}
			p.lexer.Next()
			p.skipFlowType(ast.LBitwiseOr)

		case lexer.TAmpersand:
			if level >= ast.LBitwiseAnd {
				return
			}
			p.lexer.Next()
			p.skipFlowType(ast.LBitwiseAnd)

		case lexer.TDot:
			p.lexer.Next()
			if !p.lexer.IsIdentifierOrKeyword() {
				p.lexer.Expect(lexer.TIdentifier)
			}
			p.lexer.Next()

		case lexer.TOpenBracket:
			// "{ ['x']: string \n ['y']: string }" must not become a single type
			if p.lexer.HasNewlineBefore {
				return
			}
			p.lexer.Next()
			if p.lexer.Token != lexer.TCloseBracket {
				p.skipFlowType(ast.LLowest)
			}
			p.lexer.Expect(lexer.TCloseBracket)

		case lexer.TLessThan, lexer.TLessThanEquals,
			lexer.TLessThanLessThan, lexer.TLessThanLessThanEquals:
			// "let foo: any \n <number>foo" must not become a single type
			if p.lexer.HasNewlineBefore {
				return
			}
			p.lexer.ExpectLessThan(false /* isInsideJSXElement */)
			for {
				p.skipFlowType(ast.LLowest)
				if p.lexer.Token != lexer.TComma {
					break
				}
				p.lexer.Next()
			}
			p.lexer.ExpectGreaterThan(false /* isInsideJSXElement */)

		case lexer.TExtends:
			// "{ x: number \n extends: boolean }" must not become a single type
			if p.lexer.HasNewlineBefore {
				return
			}
			p.lexer.Next()
			p.skipFlowType(ast.LCompare)

		case lexer.TQuestion:
			if level >= ast.LConditional {
				return
			}
			p.lexer.Next()

			switch p.lexer.Token {
			// Stop now if we're parsing one of these:
			// "(a?: b) => void"
			// "(a?, b?) => void"
			// "(a?) => void"
			// "[string?]"
			case lexer.TColon, lexer.TComma, lexer.TCloseParen, lexer.TCloseBracket:
				return
			}

			p.skipFlowType(ast.LLowest)
			p.lexer.Expect(lexer.TColon)
			p.skipFlowType(ast.LLowest)

		default:
			return
		}
	}
}

func (p *parser) skipFlowObjectType() {
	p.lexer.Expect(lexer.TOpenBrace)

	for p.lexer.Token != lexer.TCloseBrace {
		// "{ -readonly [K in keyof T]: T[K] }"
		// "{ +readonly [K in keyof T]: T[K] }"
		if p.lexer.Token == lexer.TPlus || p.lexer.Token == lexer.TMinus {
			p.lexer.Next()
		}

		// Skip over modifiers and the property identifier
		foundKey := false
		for p.lexer.IsIdentifierOrKeyword() ||
			p.lexer.Token == lexer.TStringLiteral ||
			p.lexer.Token == lexer.TNumericLiteral {
			p.lexer.Next()
			foundKey = true
		}

		if p.lexer.Token == lexer.TOpenBracket {
			// Index signature or computed property
			p.lexer.Next()
			p.skipFlowType(ast.LLowest)

			// "{ [key: string]: number }"
			// "{ readonly [K in keyof T]: T[K] }"
			if p.lexer.Token == lexer.TColon || p.lexer.Token == lexer.TIn {
				p.lexer.Next()
				p.skipFlowType(ast.LLowest)
			}

			p.lexer.Expect(lexer.TCloseBracket)

			// "{ [K in keyof T]+?: T[K] }"
			// "{ [K in keyof T]-?: T[K] }"
			if p.lexer.Token == lexer.TPlus || p.lexer.Token == lexer.TMinus {
				p.lexer.Next()
			}

			foundKey = true
		}

		// "?" indicates an optional property
		// "!" indicates an initialization assertion
		if foundKey && (p.lexer.Token == lexer.TQuestion || p.lexer.Token == lexer.TExclamation) {
			p.lexer.Next()
		}

		// Type parameters come right after the optional mark
		p.skipFlowTypeParameters()

		switch p.lexer.Token {
		case lexer.TColon:
			// Regular property
			if !foundKey {
				p.lexer.Expect(lexer.TIdentifier)
			}
			p.lexer.Next()
			p.skipFlowType(ast.LLowest)

		case lexer.TOpenParen:
			// Method signature
			p.skipFlowFnArgs()
			if p.lexer.Token == lexer.TColon {
				p.lexer.Next()
				p.skipFlowReturnType()
			}

		default:
			if !foundKey {
				p.lexer.Unexpected()
			}
		}

		switch p.lexer.Token {
		case lexer.TCloseBrace:

		case lexer.TComma, lexer.TSemicolon:
			p.lexer.Next()

		default:
			if !p.lexer.HasNewlineBefore {
				p.lexer.Unexpected()
			}
		}
	}

	p.lexer.Expect(lexer.TCloseBrace)
}

// This is the type parameter declarations that go with other symbol
// declarations (class, function, type, etc.)
func (p *parser) skipFlowTypeParameters() {
	if p.lexer.Token == lexer.TLessThan {
		p.lexer.Next()

		for {
			p.lexer.Expect(lexer.TIdentifier)

			// "class Foo<T extends number> {}"
			if p.lexer.Token == lexer.TExtends {
				p.lexer.Next()
				p.skipFlowType(ast.LLowest)
			}

			// "class Foo<T = void> {}"
			if p.lexer.Token == lexer.TEquals {
				p.lexer.Next()
				p.skipFlowType(ast.LLowest)
			}

			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
			if p.lexer.Token == lexer.TGreaterThan {
				break
			}
		}

		p.lexer.ExpectGreaterThan(false /* isInsideJSXElement */)
	}
}

func (p *parser) skipFlowTypeArguments(isInsideJSXElement bool) bool {
	if p.lexer.Token != lexer.TLessThan {
		return false
	}

	p.lexer.Next()

	for {
		p.skipFlowType(ast.LLowest)
		if p.lexer.Token != lexer.TComma {
			break
		}
		p.lexer.Next()
	}

	// This type argument list must end with a ">"
	p.lexer.ExpectGreaterThan(isInsideJSXElement)
	return true
}

func (p *parser) trySkipFlowTypeArgumentsWithBacktracking() bool {
	oldLexer := p.lexer
	p.lexer.IsLogDisabled = true

	// Implement backtracking by restoring the lexer's memory to its original state
	defer func() {
		r := recover()
		if _, isLexerPanic := r.(lexer.LexerPanic); isLexerPanic {
			p.lexer = oldLexer
		} else if r != nil {
			panic(r)
		}
	}()

	p.skipFlowTypeArguments(false /* isInsideJSXElement */)

	// Check the token after this and backtrack if it's the wrong one
	if !p.canFollowTypeArgumentsInExpression() {
		p.lexer.Unexpected()
	}

	// Restore the log disabled flag. Note that we can't just set it back to false
	// because it may have been true to start with.
	p.lexer.IsLogDisabled = oldLexer.IsLogDisabled
	return true
}

func (p *parser) trySkipFlowTypeParametersThenOpenParenWithBacktracking() bool {
	oldLexer := p.lexer
	p.lexer.IsLogDisabled = true

	// Implement backtracking by restoring the lexer's memory to its original state
	defer func() {
		r := recover()
		if _, isLexerPanic := r.(lexer.LexerPanic); isLexerPanic {
			p.lexer = oldLexer
		} else if r != nil {
			panic(r)
		}
	}()

	p.skipFlowTypeParameters()
	if p.lexer.Token != lexer.TOpenParen {
		p.lexer.Unexpected()
	}

	// Restore the log disabled flag. Note that we can't just set it back to false
	// because it may have been true to start with.
	p.lexer.IsLogDisabled = oldLexer.IsLogDisabled
	return true
}

func (p *parser) trySkipFlowArrowReturnTypeWithBacktracking() bool {
	oldLexer := p.lexer
	p.lexer.IsLogDisabled = true

	// Implement backtracking by restoring the lexer's memory to its original state
	defer func() {
		r := recover()
		if _, isLexerPanic := r.(lexer.LexerPanic); isLexerPanic {
			p.lexer = oldLexer
		} else if r != nil {
			panic(r)
		}
	}()

	p.lexer.Expect(lexer.TColon)
	p.skipFlowReturnType()

	// Check the token after this and backtrack if it's the wrong one
	if p.lexer.Token != lexer.TEqualsGreaterThan {
		p.lexer.Unexpected()
	}

	// Restore the log disabled flag. Note that we can't just set it back to false
	// because it may have been true to start with.
	p.lexer.IsLogDisabled = oldLexer.IsLogDisabled
	return true
}

func (p *parser) trySkipFlowArrowArgsWithBacktracking() bool {
	oldLexer := p.lexer
	p.lexer.IsLogDisabled = true

	// Implement backtracking by restoring the lexer's memory to its original state
	defer func() {
		r := recover()
		if _, isLexerPanic := r.(lexer.LexerPanic); isLexerPanic {
			p.lexer = oldLexer
		} else if r != nil {
			panic(r)
		}
	}()

	p.skipFlowFnArgs()
	p.lexer.Expect(lexer.TEqualsGreaterThan)

	// Restore the log disabled flag. Note that we can't just set it back to false
	// because it may have been true to start with.
	p.lexer.IsLogDisabled = oldLexer.IsLogDisabled
	return true
}

// // This function is taken from the official TypeScript compiler source code:
// // https://github.com/microsoft/TypeScript/blob/master/src/compiler/parser.ts
// func (p *parser) canFollowTypeArgumentsInExpression() bool {
// 	switch p.lexer.Token {
// 	case
// 		// These are the only tokens can legally follow a type argument list. So we
// 		// definitely want to treat them as type arg lists.
// 		lexer.TOpenParen,                     // foo<x>(
// 		lexer.TNoSubstitutionTemplateLiteral, // foo<T> `...`
// 		lexer.TTemplateHead:                  // foo<T> `...${100}...`
// 		return true
//
// 	case
// 		// These cases can't legally follow a type arg list. However, they're not
// 		// legal expressions either. The user is probably in the middle of a
// 		// generic type. So treat it as such.
// 		lexer.TDot,                     // foo<x>.
// 		lexer.TCloseParen,              // foo<x>)
// 		lexer.TCloseBracket,            // foo<x>]
// 		lexer.TColon,                   // foo<x>:
// 		lexer.TSemicolon,               // foo<x>;
// 		lexer.TQuestion,                // foo<x>?
// 		lexer.TEqualsEquals,            // foo<x> ==
// 		lexer.TEqualsEqualsEquals,      // foo<x> ===
// 		lexer.TExclamationEquals,       // foo<x> !=
// 		lexer.TExclamationEqualsEquals, // foo<x> !==
// 		lexer.TAmpersandAmpersand,      // foo<x> &&
// 		lexer.TBarBar,                  // foo<x> ||
// 		lexer.TQuestionQuestion,        // foo<x> ??
// 		lexer.TCaret,                   // foo<x> ^
// 		lexer.TAmpersand,               // foo<x> &
// 		lexer.TBar,                     // foo<x> |
// 		lexer.TCloseBrace,              // foo<x> }
// 		lexer.TEndOfFile:               // foo<x>
// 		return true
//
// 	case
// 		// We don't want to treat these as type arguments. Otherwise we'll parse
// 		// this as an invocation expression. Instead, we want to parse out the
// 		// expression in isolation from the type arguments.
// 		lexer.TComma,     // foo<x>,
// 		lexer.TOpenBrace: // foo<x> {
// 		return false
//
// 	default:
// 		// Anything else treat as an expression.
// 		return false
// 	}
// }

func (p *parser) skipFlowInterfaceStmt(opts parseStmtOpts) {
	name := p.lexer.Identifier
	p.lexer.Expect(lexer.TIdentifier)

	if opts.isModuleScope {
		p.localTypeNames[name] = true
	}

	p.skipFlowTypeParameters()

	if p.lexer.Token == lexer.TExtends {
		p.lexer.Next()
		for {
			p.skipFlowType(ast.LLowest)
			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
		}
	}

	if p.lexer.IsContextualKeyword("implements") {
		p.lexer.Next()
		for {
			p.skipFlowType(ast.LLowest)
			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
		}
	}

	p.skipFlowObjectType()
}

// Modified from TypeScript
func (p *parser) skipFlowTypeStmt(opts parseStmtOpts) {
	if opts.isExport && p.lexer.Token == lexer.TOpenBrace {
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

	// Addition for Flow (Flow vs TS)
	if opts.isExport && p.lexer.Token == lexer.TAsterisk {
		// "export type * from 'bar'"
		p.lexer.Next()
		if !p.lexer.IsContextualKeyword("from") {
			p.lexer.ExpectedString("\"from\"")
		}
		p.lexer.Next()
		p.parsePath()
		p.lexer.ExpectOrInsertSemicolon()
		return
	}

	name := p.lexer.Identifier
	p.lexer.Expect(lexer.TIdentifier)

	if opts.isModuleScope {
		p.localTypeNames[name] = true
	}

	p.skipFlowTypeParameters()
	p.lexer.Expect(lexer.TEquals)
	p.skipFlowType(ast.LLowest)
	p.lexer.ExpectOrInsertSemicolon()
}

func (p *parser) parseFlowDecorators() []ast.Expr {
	var tsDecorators []ast.Expr
	if p.TS.Parse {
		for p.lexer.Token == lexer.TAt {
			p.lexer.Next()

			// Parse a new/call expression with "exprFlagTSDecorator" so we ignore
			// EIndex expressions, since they may be part of a computed property:
			//
			//   class Foo {
			//     @foo ['computed']() {}
			//   }
			//
			// This matches the behavior of the TypeScript compiler.
			tsDecorators = append(tsDecorators, p.parseExprWithFlags(ast.LNew, exprFlagTSDecorator))
		}
	}
	return tsDecorators
}

func (p *parser) parseFlowEnumStmt(loc logger.Loc, opts parseStmtOpts) ast.Stmt {
	p.lexer.Expect(lexer.TEnum)
	nameLoc := p.lexer.Loc()
	nameText := p.lexer.Identifier
	p.lexer.Expect(lexer.TIdentifier)
	name := ast.LocRef{Loc: nameLoc, Ref: ast.InvalidRef}
	argRef := ast.InvalidRef
	if !opts.isTypeScriptDeclare {
		name.Ref = p.declareSymbol(ast.SymbolTSEnum, nameLoc, nameText)
		p.pushScopeForParsePass(ast.ScopeEntry, loc)
		argRef = p.declareSymbol(ast.SymbolHoisted, nameLoc, nameText)
	}
	p.lexer.Expect(lexer.TOpenBrace)

	values := []ast.EnumValue{}

	for p.lexer.Token != lexer.TCloseBrace {
		value := ast.EnumValue{
			Loc: p.lexer.Loc(),
			Ref: ast.InvalidRef,
		}

		// Parse the name
		if p.lexer.Token == lexer.TStringLiteral {
			value.Name = p.lexer.StringLiteral
		} else if p.lexer.IsIdentifierOrKeyword() {
			value.Name = lexer.StringToUTF16(p.lexer.Identifier)
		} else {
			p.lexer.Expect(lexer.TIdentifier)
		}
		p.lexer.Next()

		// Identifiers can be referenced by other values
		if !opts.isTypeScriptDeclare && lexer.IsIdentifierUTF16(value.Name) {
			value.Ref = p.declareSymbol(ast.SymbolOther, value.Loc, lexer.UTF16ToString(value.Name))
		}

		// Parse the initializer
		if p.lexer.Token == lexer.TEquals {
			p.lexer.Next()
			initializer := p.parseExpr(ast.LComma)
			value.Value = &initializer
		}

		values = append(values, value)

		if p.lexer.Token != lexer.TComma && p.lexer.Token != lexer.TSemicolon {
			break
		}
		p.lexer.Next()
	}

	if !opts.isTypeScriptDeclare {
		p.popScope()
		if opts.isExport {
			p.recordExport(nameLoc, nameText, name.Ref)
		}
	}

	p.lexer.Expect(lexer.TCloseBrace)
	return ast.Stmt{Loc: loc, Data: &ast.SEnum{
		Name:     name,
		Arg:      argRef,
		Values:   values,
		IsExport: opts.isExport,
	}}
}

func (p *parser) parseFlowNamespaceStmt(loc logger.Loc, opts parseStmtOpts) ast.Stmt {
	// "namespace Foo {}"
	nameLoc := p.lexer.Loc()
	nameText := p.lexer.Identifier
	p.lexer.Next()

	name := ast.LocRef{Loc: nameLoc, Ref: ast.InvalidRef}
	argRef := ast.InvalidRef

	scopeIndex := p.pushScopeForParsePass(ast.ScopeEntry, loc)
	oldEnclosingNamespaceRef := p.enclosingNamespaceRef
	p.enclosingNamespaceRef = &name.Ref

	if !opts.isTypeScriptDeclare {
		argRef = p.declareSymbol(ast.SymbolHoistedFunction, nameLoc, nameText)
	}

	var stmts []ast.Stmt
	if p.lexer.Token == lexer.TDot {
		dotLoc := p.lexer.Loc()
		p.lexer.Next()
		stmts = []ast.Stmt{p.parseFlowNamespaceStmt(dotLoc, parseStmtOpts{
			isExport:            true,
			isNamespaceScope:    true,
			isTypeScriptDeclare: opts.isTypeScriptDeclare,
		})}
	} else if opts.isTypeScriptDeclare && p.lexer.Token != lexer.TOpenBrace {
		p.lexer.ExpectOrInsertSemicolon()
	} else {
		p.lexer.Expect(lexer.TOpenBrace)
		stmts = p.parseStmtsUpTo(lexer.TCloseBrace, parseStmtOpts{
			isNamespaceScope:    true,
			isTypeScriptDeclare: opts.isTypeScriptDeclare,
		})
		p.lexer.Next()
	}

	p.enclosingNamespaceRef = oldEnclosingNamespaceRef

	// Import assignments may be only used in type expressions, not value
	// expressions. If this is the case, the TypeScript compiler removes
	// them entirely from the output. That can cause the namespace itself
	// to be considered empty and thus be removed.
	importEqualsCount := 0
	for _, stmt := range stmts {
		if local, ok := stmt.Data.(*ast.SLocal); ok && local.WasTSImportEqualsInNamespace && !local.IsExport {
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
		return ast.Stmt{Loc: loc, Data: &ast.STypeScript{}}
	}

	p.popScope()
	if !opts.isTypeScriptDeclare {
		_, alreadyExists := p.currentScope.Members[nameText]
		name.Ref = p.declareSymbol(ast.SymbolTSNamespace, nameLoc, nameText)

		// It's valid to have multiple exported namespace statements as long as
		// each one has the "export" keyword. Make sure we don't record the same
		// export more than once, because then we will incorrectly detect duplicate
		// exports.
		if opts.isExport && !alreadyExists {
			p.recordExport(nameLoc, nameText, name.Ref)
		}
	}
	return ast.Stmt{Loc: loc, Data: &ast.SNamespace{
		Name:     name,
		Arg:      argRef,
		Stmts:    stmts,
		IsExport: opts.isExport,
	}}
}

func (p *parser) generateClosureForFlowNamespaceOrEnum(
	stmts []ast.Stmt, stmtLoc logger.Loc, isExport bool, nameLoc logger.Loc,
	nameRef ast.Ref, argRef ast.Ref, stmtsInsideClosure []ast.Stmt,
) []ast.Stmt {
	// Follow the link chain in case symbols were merged
	symbol := p.symbols[nameRef.InnerIndex]
	for symbol.Link != ast.InvalidRef {
		nameRef = symbol.Link
		symbol = p.symbols[nameRef.InnerIndex]
	}

	// Make sure to only emit a variable once for a given namespace, since there
	// can be multiple namespace blocks for the same namespace
	if (symbol.Kind == ast.SymbolTSNamespace || symbol.Kind == ast.SymbolTSEnum) && !p.emittedNamespaceVars[nameRef] {
		p.emittedNamespaceVars[nameRef] = true
		if p.enclosingNamespaceRef == nil {
			// Top-level namespace
			stmts = append(stmts, ast.Stmt{Loc: stmtLoc, Data: &ast.SLocal{
				Kind:     ast.LocalVar,
				Decls:    []ast.Decl{{Binding: ast.Binding{Loc: nameLoc, Data: &ast.BIdentifier{Ref: nameRef}}}},
				IsExport: isExport,
			}})
		} else {
			// Nested namespace
			stmts = append(stmts, ast.Stmt{Loc: stmtLoc, Data: &ast.SLocal{
				Kind:  ast.LocalLet,
				Decls: []ast.Decl{{Binding: ast.Binding{Loc: nameLoc, Data: &ast.BIdentifier{Ref: nameRef}}}},
			}})
		}
	}

	var argExpr ast.Expr
	if isExport && p.enclosingNamespaceRef != nil {
		// "name = enclosing.name || (enclosing.name = {})"
		name := p.symbols[nameRef.InnerIndex].OriginalName
		argExpr = ast.Assign(
			ast.Expr{Loc: nameLoc, Data: &ast.EIdentifier{Ref: nameRef}},
			ast.Expr{Loc: nameLoc, Data: &ast.EBinary{
				Op: ast.BinOpLogicalOr,
				Left: ast.Expr{Loc: nameLoc, Data: &ast.EDot{
					Target:  ast.Expr{Loc: nameLoc, Data: &ast.EIdentifier{Ref: *p.enclosingNamespaceRef}},
					Name:    name,
					NameLoc: nameLoc,
				}},
				Right: ast.Assign(
					ast.Expr{Loc: nameLoc, Data: &ast.EDot{
						Target:  ast.Expr{Loc: nameLoc, Data: &ast.EIdentifier{Ref: *p.enclosingNamespaceRef}},
						Name:    name,
						NameLoc: nameLoc,
					}},
					ast.Expr{Loc: nameLoc, Data: &ast.EObject{}},
				),
			}},
		)
		p.recordUsage(*p.enclosingNamespaceRef)
		p.recordUsage(*p.enclosingNamespaceRef)
		p.recordUsage(nameRef)
	} else {
		// "name || (name = {})"
		argExpr = ast.Expr{Loc: nameLoc, Data: &ast.EBinary{
			Op:   ast.BinOpLogicalOr,
			Left: ast.Expr{Loc: nameLoc, Data: &ast.EIdentifier{Ref: nameRef}},
			Right: ast.Assign(
				ast.Expr{Loc: nameLoc, Data: &ast.EIdentifier{Ref: nameRef}},
				ast.Expr{Loc: nameLoc, Data: &ast.EObject{}},
			),
		}}
		p.recordUsage(nameRef)
		p.recordUsage(nameRef)
	}

	// Call the closure with the name object
	stmts = append(stmts, ast.Stmt{Loc: stmtLoc, Data: &ast.SExpr{Value: ast.Expr{Loc: stmtLoc, Data: &ast.ECall{
		Target: ast.Expr{Loc: stmtLoc, Data: &ast.EFunction{Fn: ast.Fn{
			Args: []ast.Arg{{Binding: ast.Binding{Loc: nameLoc, Data: &ast.BIdentifier{Ref: argRef}}}},
			Body: ast.FnBody{Loc: stmtLoc, Stmts: stmtsInsideClosure},
		}}},
		Args: []ast.Expr{argExpr},
	}}}})

	return stmts
}
