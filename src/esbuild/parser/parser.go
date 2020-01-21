package parser

import (
	"esbuild/ast"
	"esbuild/lexer"
	"esbuild/logging"
	"fmt"
	"math"
	"reflect"
	"strings"
	"unsafe"
)

func isJumpStatement(data ast.S) bool {
	switch data.(type) {
	case *ast.SBreak, *ast.SContinue, *ast.SReturn, *ast.SThrow:
		return true
	}

	return false
}

func hasNoSideEffects(data ast.E) bool {
	switch data.(type) {
	case *ast.ENull, *ast.EUndefined, *ast.EBoolean, *ast.ENumber, *ast.EBigInt,
		*ast.EString, *ast.EThis, *ast.ERegExp, *ast.EFunction, *ast.EArrow:
		return true
	}

	return false
}

func toBooleanWithoutSideEffects(data ast.E) (bool, bool) {
	switch e := data.(type) {
	case *ast.ENull, *ast.EUndefined:
		return false, true

	case *ast.EBoolean:
		return e.Value, true

	case *ast.ENumber:
		return e.Value != 0 && !math.IsNaN(e.Value), true

	case *ast.EBigInt:
		return e.Value != "0", true

	case *ast.EString:
		return len(e.Value) > 0, true

	case *ast.EFunction, *ast.EArrow:
		return true, true
	}

	return false, false
}

func toNumberWithoutSideEffects(data ast.E) (float64, bool) {
	switch e := data.(type) {
	case *ast.ENull:
		return 0, true

	case *ast.EUndefined:
		return math.NaN(), true

	case *ast.EBoolean:
		if e.Value {
			return 1, true
		} else {
			return 0, true
		}

	case *ast.ENumber:
		return e.Value, true
	}

	return 0, false
}

func typeofWithoutSideEffects(data ast.E) (string, bool) {
	switch data.(type) {
	case *ast.ENull:
		return "object", true

	case *ast.EUndefined:
		return "undefined", true

	case *ast.EBoolean:
		return "boolean", true

	case *ast.ENumber:
		return "number", true

	case *ast.EBigInt:
		return "bigint", true

	case *ast.EString:
		return "string", true

	case *ast.EFunction, *ast.EArrow:
		return "function", true
	}

	return "", false
}

func checkEqualityIfNoSideEffects(left ast.E, right ast.E) (bool, bool) {
	switch l := left.(type) {
	case *ast.ENull:
		if _, ok := right.(*ast.ENull); ok {
			return true, true
		}

	case *ast.EUndefined:
		if _, ok := right.(*ast.EUndefined); ok {
			return true, true
		}

	case *ast.EBoolean:
		if r, ok := right.(*ast.EBoolean); ok {
			return l.Value == r.Value, true
		}

	case *ast.ENumber:
		if r, ok := right.(*ast.ENumber); ok {
			return l.Value == r.Value, true
		}

	case *ast.EBigInt:
		if r, ok := right.(*ast.EBigInt); ok {
			return l.Value == r.Value, true
		}

	case *ast.EString:
		if r, ok := right.(*ast.EString); ok {
			lv := l.Value
			rv := r.Value
			if len(lv) != len(rv) {
				return false, true
			}
			for i := 0; i < len(lv); i++ {
				if lv[i] != rv[i] {
					return false, true
				}
			}
			return true, true
		}
	}

	return false, false
}

type fnOpts struct {
	allowAwait bool
	allowYield bool
}

type parser struct {
	log                      logging.Log
	source                   logging.Source
	lexer                    lexer.Lexer
	importPaths              []ast.ImportPath
	omitWarnings             bool
	allowIn                  bool
	currentFnOpts            fnOpts
	jsx                      JSXOptions
	latestReturnHadSemicolon bool
	allocatedNames           []string
}

func (p *parser) addError(loc ast.Loc, text string) {
	p.log.AddError(p.source, loc, text)
}

func (p *parser) addRangeError(r ast.Range, text string) {
	p.log.AddRangeError(p.source, r, text)
}

// The name is temporarily stored in the ref until the scope traversal pass
// happens, at which point a symbol will be generated and the ref will point
// to the symbol instead.
//
// The scope traversal pass will reconstruct the name using one of two methods.
// In the common case, the name is a slice of the file itself. In that case we
// can just store the slice and not need to allocate any extra memory. In the
// rare case, the name is an externally-allocated string. In that case we store
// an index to the string and use that index during the scope traversal pass.
func (p *parser) storeNameInRef(name string) ast.Ref {
	c := (*reflect.StringHeader)(unsafe.Pointer(&p.source.Contents))
	n := (*reflect.StringHeader)(unsafe.Pointer(&name))

	// Is the data in "name" a subset of the data in "p.source.Contents"?
	if n.Data >= c.Data && n.Data+uintptr(n.Len) < c.Data+uintptr(c.Len) {
		// The name is a slice of the file contents, so we can just reference it by
		// length and don't have to allocate anything. This is the common case.
		//
		// It's stored as a negative value so we'll crash if we try to use it. That
		// way we'll catch cases where we've forgetten to call loadNameFromRef().
		// The length is the negative part because we know it's non-zero.
		return ast.Ref{-uint32(n.Len), uint32(n.Data - c.Data)}
	} else {
		// The name is some memory allocated elsewhere. This is either an inline
		// string constant in the parser or an identifier with escape sequences
		// in the source code, which is very unusual. Stash it away for later.
		// This uses allocations but it should hopefully be very uncommon.
		ref := ast.Ref{0x80000000, uint32(len(p.allocatedNames))}
		p.allocatedNames = append(p.allocatedNames, name)
		return ref
	}
}

// Due to ES6 destructuring patterns, there are many cases where it's
// impossible to distinguish between an array or object literal and a
// destructuring assignment until we hit the "=" operator later on.
// This object defers errors about being in one state or the other
// until we discover which state we're in.
type deferredErrors struct {
	// These are errors for expressions
	invalidExprDefaultValue ast.Range

	// These are errors for destructuring patterns
	invalidBindingCommaAfterSpread ast.Range
}

func (from *deferredErrors) mergeInto(to *deferredErrors) {
	if from.invalidExprDefaultValue.Len > 0 {
		to.invalidExprDefaultValue = from.invalidExprDefaultValue
	}
	if from.invalidBindingCommaAfterSpread.Len > 0 {
		to.invalidBindingCommaAfterSpread = from.invalidBindingCommaAfterSpread
	}
}

type propertyContext int

const (
	propertyContextObject = iota
	propertyContextClass
)

type propertyOpts struct {
	isAsync     bool
	isGenerator bool
	isStatic    bool
}

func (p *parser) parseProperty(context propertyContext, kind ast.PropertyKind, opts propertyOpts, errors *deferredErrors) ast.Property {
	var key ast.Expr
	isComputed := false

	switch p.lexer.Token {
	case lexer.TNumericLiteral:
		key = ast.Expr{p.lexer.Loc(), &ast.ENumber{p.lexer.Number}}
		p.lexer.Next()

	case lexer.TStringLiteral:
		key = ast.Expr{p.lexer.Loc(), &ast.EString{p.lexer.StringLiteral}}
		p.lexer.Next()

	case lexer.TOpenBracket:
		isComputed = true
		p.lexer.Next()
		expr := p.parseExpr(ast.LLowest)
		p.lexer.Expect(lexer.TCloseBracket)
		key = expr

	case lexer.TAsterisk:
		if kind != ast.PropertyNormal || opts.isGenerator {
			p.lexer.Unexpected()
		}
		p.lexer.Next()
		opts.isGenerator = true
		return p.parseProperty(context, ast.PropertyNormal, opts, errors)

	default:
		name := p.lexer.Identifier
		loc := p.lexer.Loc()
		if !p.lexer.IsIdentifierOrKeyword() {
			p.lexer.Expect(lexer.TIdentifier)
		}
		p.lexer.Next()

		// Support contextual keywords
		if kind == ast.PropertyNormal && !opts.isGenerator {
			// Does the following token look like a key?
			couldBeModifierKeyword := p.lexer.IsIdentifierOrKeyword()
			if !couldBeModifierKeyword {
				switch p.lexer.Token {
				case lexer.TOpenBracket, lexer.TNumericLiteral, lexer.TStringLiteral, lexer.TAsterisk:
					couldBeModifierKeyword = true
				}
			}

			// If so, check for a modifier keyword
			if couldBeModifierKeyword {
				switch name {
				case "get":
					if !opts.isAsync {
						return p.parseProperty(context, ast.PropertyGet, opts, nil)
					}

				case "set":
					if !opts.isAsync {
						return p.parseProperty(context, ast.PropertySet, opts, nil)
					}

				case "async":
					if !opts.isAsync {
						opts.isAsync = true
						return p.parseProperty(context, kind, opts, nil)
					}

				case "static":
					if !opts.isStatic && !opts.isAsync && context == propertyContextClass {
						opts.isStatic = true
						return p.parseProperty(context, kind, opts, nil)
					}
				}
			}
		}

		key = ast.Expr{loc, &ast.EString{lexer.StringToUTF16(name)}}

		// Parse a shorthand property
		if context == propertyContextObject && kind == ast.PropertyNormal &&
			p.lexer.Token != lexer.TColon && p.lexer.Token != lexer.TOpenParen && !opts.isGenerator {
			ref := p.storeNameInRef(name)
			value := ast.Expr{key.Loc, &ast.EIdentifier{ref}}

			// Destructuring patterns have an optional default value
			var initializer *ast.Expr = nil
			if errors != nil && p.lexer.Token == lexer.TEquals {
				errors.invalidExprDefaultValue = p.lexer.Range()
				p.lexer.Next()
				value := p.parseExpr(ast.LComma)
				initializer = &value
			}

			return ast.Property{
				Kind:        kind,
				Key:         key,
				Value:       &value,
				Initializer: initializer,
			}
		}
	}

	// Parse a field
	if context == propertyContextClass && kind == ast.PropertyNormal &&
		!opts.isAsync && !opts.isGenerator && p.lexer.Token != lexer.TOpenParen {
		var initializer *ast.Expr
		if p.lexer.Token == lexer.TEquals {
			p.lexer.Next()
			value := p.parseExpr(ast.LComma)
			initializer = &value
		}
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Property{
			Kind:        kind,
			IsComputed:  isComputed,
			IsStatic:    opts.isStatic,
			Key:         key,
			Initializer: initializer,
		}
	}

	// Parse a method expression
	if p.lexer.Token == lexer.TOpenParen || kind != ast.PropertyNormal ||
		context == propertyContextClass || opts.isAsync || opts.isGenerator {
		loc := p.lexer.Loc()
		fn := p.parseFn(nil, fnOpts{
			allowAwait: opts.isAsync,
			allowYield: opts.isGenerator,
		})
		value := ast.Expr{loc, &ast.EFunction{fn}}
		return ast.Property{
			Kind:       kind,
			IsComputed: isComputed,
			IsMethod:   true,
			IsStatic:   opts.isStatic,
			Key:        key,
			Value:      &value,
		}
	}

	p.lexer.Expect(lexer.TColon)
	value := p.parseExprOrBindings(ast.LComma, errors)
	return ast.Property{
		Kind:       kind,
		IsComputed: isComputed,
		Key:        key,
		Value:      &value,
	}
}

func (p *parser) parsePropertyBinding() ast.PropertyBinding {
	var key ast.Expr
	isComputed := false

	switch p.lexer.Token {
	case lexer.TDotDotDot:
		p.lexer.Next()
		value := ast.Binding{p.lexer.Loc(), &ast.BIdentifier{p.storeNameInRef(p.lexer.Identifier)}}
		p.lexer.Expect(lexer.TIdentifier)
		return ast.PropertyBinding{
			IsSpread: true,
			Value:    value,
		}

	case lexer.TNumericLiteral:
		key = ast.Expr{p.lexer.Loc(), &ast.ENumber{p.lexer.Number}}
		p.lexer.Next()

	case lexer.TStringLiteral:
		key = ast.Expr{p.lexer.Loc(), &ast.EString{p.lexer.StringLiteral}}
		p.lexer.Next()

	case lexer.TOpenBracket:
		isComputed = true
		p.lexer.Next()

		// "in" expressions are allowed
		oldAllowIn := p.allowIn
		p.allowIn = true
		expr := p.parseExpr(ast.LLowest)
		p.allowIn = oldAllowIn

		p.lexer.Expect(lexer.TCloseBracket)
		key = expr

	default:
		name := p.lexer.Identifier
		loc := p.lexer.Loc()
		if !p.lexer.IsIdentifierOrKeyword() {
			p.lexer.Expect(lexer.TIdentifier)
		}
		p.lexer.Next()
		key = ast.Expr{loc, &ast.EString{lexer.StringToUTF16(name)}}

		if p.lexer.Token != lexer.TColon && p.lexer.Token != lexer.TOpenParen {
			ref := p.storeNameInRef(name)
			value := ast.Binding{loc, &ast.BIdentifier{ref}}

			var defaultValue *ast.Expr
			if p.lexer.Token == lexer.TEquals {
				p.lexer.Next()

				// "in" expressions are allowed
				oldAllowIn := p.allowIn
				p.allowIn = true
				init := p.parseExpr(ast.LComma)
				defaultValue = &init
				p.allowIn = oldAllowIn
			}

			return ast.PropertyBinding{
				Key:          key,
				Value:        value,
				DefaultValue: defaultValue,
			}
		}
	}

	p.lexer.Expect(lexer.TColon)
	value := p.parseBinding()

	var defaultValue *ast.Expr
	if p.lexer.Token == lexer.TEquals {
		p.lexer.Next()

		// "in" expressions are allowed
		oldAllowIn := p.allowIn
		p.allowIn = true
		init := p.parseExpr(ast.LComma)
		defaultValue = &init
		p.allowIn = oldAllowIn
	}

	return ast.PropertyBinding{
		IsComputed:   isComputed,
		Key:          key,
		Value:        value,
		DefaultValue: defaultValue,
	}
}

// This assumes that the "=>" token has already been parsed by the caller
func (p *parser) parseArrowBody(opts fnOpts) ([]ast.Stmt, *ast.Expr) {
	if p.lexer.Token == lexer.TOpenBrace {
		stmts := p.parseFnBodyStmts(opts)
		return stmts, nil
	}

	oldFnOpts := p.currentFnOpts
	p.currentFnOpts = opts
	expr := p.parseExpr(ast.LComma)
	p.currentFnOpts = oldFnOpts
	return []ast.Stmt{}, &expr
}

func (p *parser) isAsyncExprSuffix() bool {
	switch p.lexer.Token {
	case lexer.TFunction, lexer.TEqualsGreaterThan:
		return true
	}
	return false
}

// This parses an expression. This assumes we've already parsed the "async"
// keyword and are currently looking at the following token.
func (p *parser) parseAsyncExpr(loc ast.Loc, level ast.L) ast.Expr {
	var expr ast.Expr

	// Make sure this matches the switch statement in isAsyncExprSuffix()
	switch p.lexer.Token {
	// "async function() {}"
	case lexer.TFunction:
		return p.parseFnExpr(loc, true /* isAsync */)

		// "async => {}"
	case lexer.TEqualsGreaterThan:
		p.lexer.Next()
		arg := ast.Arg{ast.Binding{loc, &ast.BIdentifier{p.storeNameInRef("async")}}, nil}
		stmts, expr := p.parseArrowBody(fnOpts{})
		return ast.Expr{loc, &ast.EArrow{Args: []ast.Arg{arg}, Stmts: stmts, Expr: expr}}

		// "async x => {}"
	case lexer.TIdentifier:
		ref := p.storeNameInRef(p.lexer.Identifier)
		arg := ast.Arg{ast.Binding{p.lexer.Loc(), &ast.BIdentifier{ref}}, nil}
		p.lexer.Next()
		p.lexer.Expect(lexer.TEqualsGreaterThan)
		stmts, expr := p.parseArrowBody(fnOpts{allowAwait: true})
		return ast.Expr{loc, &ast.EArrow{
			IsAsync: true,
			Args:    []ast.Arg{arg},
			Stmts:   stmts,
			Expr:    expr,
		}}

		// "async()"
		// "async () => {}"
	case lexer.TOpenParen:
		p.lexer.Next()
		return p.parseParenExpr(loc, true /* isAsync */)

		// "async"
		// "async + 1"
	default:
		expr = ast.Expr{loc, &ast.EIdentifier{p.storeNameInRef("async")}}
	}

	return p.parseSuffix(expr, level)
}

func (p *parser) parseFnExpr(loc ast.Loc, isAsync bool) ast.Expr {
	p.lexer.Next()
	isGenerator := p.lexer.Token == lexer.TAsterisk
	if isGenerator {
		p.lexer.Next()
	}
	var name *ast.LocRef

	if p.lexer.Token == lexer.TIdentifier {
		temp := ast.LocRef{p.lexer.Loc(), p.storeNameInRef(p.lexer.Identifier)}
		name = &temp
		p.lexer.Next()
	}

	fn := p.parseFn(name, fnOpts{
		allowAwait: isAsync,
		allowYield: isGenerator,
	})
	return ast.Expr{loc, &ast.EFunction{fn}}
}

func (p *parser) logExprErrors(errors *deferredErrors) {
	if errors.invalidExprDefaultValue.Len > 0 {
		p.addRangeError(errors.invalidExprDefaultValue, "Unexpected \"=\"")
	}
}

func (p *parser) logBindingErrors(errors *deferredErrors) {
	if errors.invalidBindingCommaAfterSpread.Len > 0 {
		p.addRangeError(errors.invalidBindingCommaAfterSpread, "Unexpected \",\" after rest pattern")
	}
}

// This assumes that the open parenthesis has already been parsed by the caller
func (p *parser) parseParenExpr(loc ast.Loc, isAsync bool) ast.Expr {
	items := []ast.Expr{}
	errors := deferredErrors{}
	spreadRange := ast.Range{}

	// Scan over the comma-separated arguments or expressions
	for p.lexer.Token != lexer.TCloseParen {
		itemLoc := p.lexer.Loc()
		isSpread := p.lexer.Token == lexer.TDotDotDot

		if isSpread {
			spreadRange = p.lexer.Range()
			p.lexer.Next()
		}

		// We don't know yet whether these are arguments or expressions, so parse
		// a superset of the expression syntax. Errors about things that are valid
		// in one but not in the other are deferred.
		item := p.parseExprOrBindings(ast.LComma, &errors)

		if isSpread {
			item = ast.Expr{itemLoc, &ast.ESpread{item}}
		}

		items = append(items, item)
		if p.lexer.Token != lexer.TComma {
			break
		}

		// Spread arguments must come last. If there's a spread argument followed
		// by a comma, throw an error if we use these expressions as bindings.
		if isSpread {
			errors.invalidBindingCommaAfterSpread = p.lexer.Range()
		}

		// Eat the comma token
		p.lexer.Next()
	}

	// The parenthetical construct must end with a close parenthesis
	p.lexer.Expect(lexer.TCloseParen)

	// Are these arguments to an arrow function?
	if p.lexer.Token == lexer.TEqualsGreaterThan {
		p.logBindingErrors(&errors)
		p.lexer.Next()
		args := []ast.Arg{}
		for _, item := range items {
			if spread, ok := item.Data.(*ast.ESpread); ok {
				item = spread.Value
			}
			binding, initializer := p.convertExprToBindingAndInitializer(item)
			args = append(args, ast.Arg{binding, initializer})
		}
		stmts, expr := p.parseArrowBody(fnOpts{allowAwait: isAsync})
		return ast.Expr{loc, &ast.EArrow{
			IsAsync:    isAsync,
			Args:       args,
			HasRestArg: spreadRange.Len > 0,
			Stmts:      stmts,
			Expr:       expr,
		}}
	}

	// Are these arguments for a call to a function named "async"?
	if isAsync {
		p.logExprErrors(&errors)
		async := ast.Expr{loc, &ast.EIdentifier{p.storeNameInRef("async")}}
		return ast.Expr{loc, &ast.ECall{async, items, false}}
	}

	// Is this a chain of expressions and comma operators?
	if len(items) > 0 {
		p.logExprErrors(&errors)
		if spreadRange.Len > 0 {
			p.addRangeError(spreadRange, "Unexpected \"...\"")
			panic(lexer.LexerPanic{})
		}
		value := items[0]
		for _, item := range items[1:] {
			value.Data = &ast.EBinary{ast.BinOpComma, value, item}
		}
		return value
	}

	// Indicate that we expected an arrow function
	p.lexer.Expected(lexer.TEqualsGreaterThan)
	return ast.Expr{}
}

func (p *parser) convertExprToBindingAndInitializer(expr ast.Expr) (binding ast.Binding, initializer *ast.Expr) {
	if assign, ok := expr.Data.(*ast.EBinary); ok && assign.Op == ast.BinOpAssign {
		initializer = &assign.Right
		expr = assign.Left
	}
	binding = p.convertExprToBinding(expr)
	return
}

func (p *parser) convertExprToBinding(expr ast.Expr) ast.Binding {
	switch e := expr.Data.(type) {
	case *ast.EMissing:
		return ast.Binding{expr.Loc, &ast.BMissing{}}

	case *ast.EIdentifier:
		return ast.Binding{expr.Loc, &ast.BIdentifier{e.Ref}}

	case *ast.EArray:
		items := []ast.ArrayBinding{}
		isSpread := false
		for _, item := range e.Items {
			if i, ok := item.Data.(*ast.ESpread); ok {
				isSpread = true
				item = i.Value
			}
			binding, initializer := p.convertExprToBindingAndInitializer(item)
			items = append(items, ast.ArrayBinding{binding, initializer})
		}
		return ast.Binding{expr.Loc, &ast.BArray{
			Items:     items,
			HasSpread: isSpread,
		}}

	case *ast.EObject:
		items := []ast.PropertyBinding{}
		for _, item := range e.Properties {
			if item.Kind == ast.PropertyGet || item.IsMethod ||
				item.Kind == ast.PropertySet {
				p.addError(item.Key.Loc, "Invalid binding pattern")
				panic(lexer.LexerPanic{})
			}
			binding, initializer := p.convertExprToBindingAndInitializer(*item.Value)
			if initializer == nil {
				initializer = item.Initializer
			}
			items = append(items, ast.PropertyBinding{
				IsSpread:     item.Kind == ast.PropertySpread,
				IsComputed:   item.IsComputed,
				Key:          item.Key,
				Value:        binding,
				DefaultValue: initializer,
			})
		}
		return ast.Binding{expr.Loc, &ast.BObject{items}}

	default:
		p.addError(expr.Loc, "Invalid binding pattern")
		panic(lexer.LexerPanic{})
	}
}

func (p *parser) parsePrefix(level ast.L, errors *deferredErrors) ast.Expr {
	loc := p.lexer.Loc()

	switch p.lexer.Token {
	case lexer.TSuper:
		p.lexer.Next()

		switch p.lexer.Token {
		case lexer.TOpenParen:
			if level < ast.LCall {
				return ast.Expr{loc, &ast.ESuper{}}
			}

		case lexer.TDot, lexer.TOpenBracket:
			return ast.Expr{loc, &ast.ESuper{}}
		}

		p.lexer.Unexpected()
		return ast.Expr{}

	case lexer.TOpenParen:
		p.lexer.Next()

		// Allow "in" inside parentheses
		oldAllowIn := p.allowIn
		p.allowIn = true

		// Arrow functions aren't allowed in the middle of expressions
		if level > ast.LAssign {
			value := p.parseExpr(ast.LLowest)
			p.lexer.Expect(lexer.TCloseParen)
			p.allowIn = oldAllowIn
			return value
		}

		value := p.parseParenExpr(loc, false /* isAsync */)
		p.allowIn = oldAllowIn
		return value

	case lexer.TFalse:
		p.lexer.Next()
		return ast.Expr{loc, &ast.EBoolean{false}}

	case lexer.TTrue:
		p.lexer.Next()
		return ast.Expr{loc, &ast.EBoolean{true}}

	case lexer.TNull:
		p.lexer.Next()
		return ast.Expr{loc, &ast.ENull{}}

	case lexer.TThis:
		p.lexer.Next()
		return ast.Expr{loc, &ast.EThis{}}

	case lexer.TYield:
		if !p.currentFnOpts.allowYield {
			p.addRangeError(p.lexer.Range(), "Cannot use \"yield\" outside a generator function")
			panic(lexer.LexerPanic{})
		}

		if level > ast.LAssign {
			p.addRangeError(p.lexer.Range(), "Cannot use a \"yield\" expression here without parentheses")
			panic(lexer.LexerPanic{})
		}

		p.lexer.Next()

		// Parse a yield-from expression, which yields from an iterator
		isStar := p.lexer.Token == lexer.TAsterisk
		if isStar {
			if p.lexer.HasNewlineBefore {
				p.lexer.Unexpected()
			}
			p.lexer.Next()
		}

		var value *ast.Expr

		// The yield expression only has a value in certain cases
		switch p.lexer.Token {
		case lexer.TCloseBrace, lexer.TCloseBracket, lexer.TCloseParen,
			lexer.TColon, lexer.TComma, lexer.TSemicolon:

		default:
			if isStar || !p.lexer.HasNewlineBefore {
				expr := p.parseExpr(ast.LYield)
				value = &expr
			}
		}

		return ast.Expr{loc, &ast.EYield{value, isStar}}

	case lexer.TIdentifier:
		name := p.lexer.Identifier
		p.lexer.Next()

		// Handle async and await expressions
		if name == "async" {
			return p.parseAsyncExpr(loc, level)
		} else if p.currentFnOpts.allowAwait && name == "await" {
			return ast.Expr{loc, &ast.EAwait{p.parseExpr(ast.LPrefix)}}
		}

		// Handle the start of an arrow expression
		if p.lexer.Token == lexer.TEqualsGreaterThan {
			p.lexer.Next()
			ref := p.storeNameInRef(name)
			arg := ast.Arg{ast.Binding{loc, &ast.BIdentifier{ref}}, nil}
			stmts, expr := p.parseArrowBody(fnOpts{})
			return ast.Expr{loc, &ast.EArrow{Args: []ast.Arg{arg}, Stmts: stmts, Expr: expr}}
		}

		ref := p.storeNameInRef(name)
		return ast.Expr{loc, &ast.EIdentifier{ref}}

	case lexer.TStringLiteral:
		value := p.lexer.StringLiteral
		p.lexer.Next()
		return ast.Expr{loc, &ast.EString{value}}

	case lexer.TNoSubstitutionTemplateLiteral:
		head := p.lexer.StringLiteral
		p.lexer.Next()
		return ast.Expr{loc, &ast.ETemplate{nil, head, "", []ast.TemplatePart{}}}

	case lexer.TTemplateHead:
		head := p.lexer.StringLiteral
		parts := p.parseTemplateParts(false /* includeRaw */)
		return ast.Expr{loc, &ast.ETemplate{nil, head, "", parts}}

	case lexer.TNumericLiteral:
		value := p.lexer.Number
		p.lexer.Next()
		return ast.Expr{loc, &ast.ENumber{value}}

	case lexer.TBigIntegerLiteral:
		value := p.lexer.Identifier
		p.lexer.Next()
		return ast.Expr{p.lexer.Loc(), &ast.EBigInt{value}}

	case lexer.TSlash, lexer.TSlashEquals:
		p.lexer.ScanRegExp()
		value := p.lexer.Raw()
		p.lexer.Next()
		return ast.Expr{loc, &ast.ERegExp{value}}

	case lexer.TVoid:
		p.lexer.Next()
		return ast.Expr{loc, &ast.EUnary{ast.UnOpVoid, p.parseExpr(ast.LPrefix)}}

	case lexer.TTypeOf:
		p.lexer.Next()
		return ast.Expr{loc, &ast.EUnary{ast.UnOpTypeOf, p.parseExpr(ast.LPrefix)}}

	case lexer.TDelete:
		p.lexer.Next()
		return ast.Expr{loc, &ast.EUnary{ast.UnOpDelete, p.parseExpr(ast.LPrefix)}}

	case lexer.TPlus:
		p.lexer.Next()
		return ast.Expr{loc, &ast.EUnary{ast.UnOpPos, p.parseExpr(ast.LPrefix)}}

	case lexer.TMinus:
		p.lexer.Next()
		return ast.Expr{loc, &ast.EUnary{ast.UnOpNeg, p.parseExpr(ast.LPrefix)}}

	case lexer.TTilde:
		p.lexer.Next()
		return ast.Expr{loc, &ast.EUnary{ast.UnOpCpl, p.parseExpr(ast.LPrefix)}}

	case lexer.TExclamation:
		p.lexer.Next()
		return ast.Expr{loc, &ast.EUnary{ast.UnOpNot, p.parseExpr(ast.LPrefix)}}

	case lexer.TMinusMinus:
		p.lexer.Next()
		return ast.Expr{loc, &ast.EUnary{ast.UnOpPreDec, p.parseExpr(ast.LPrefix)}}

	case lexer.TPlusPlus:
		p.lexer.Next()
		return ast.Expr{loc, &ast.EUnary{ast.UnOpPreInc, p.parseExpr(ast.LPrefix)}}

	case lexer.TFunction:
		return p.parseFnExpr(loc, false /* isAsync */)

	case lexer.TClass:
		p.lexer.Next()
		var name *ast.LocRef

		if p.lexer.Token == lexer.TIdentifier {
			temp := ast.LocRef{p.lexer.Loc(), p.storeNameInRef(p.lexer.Identifier)}
			name = &temp
			p.lexer.Next()
		}

		class := p.parseClass(name)

		return ast.Expr{loc, &ast.EClass{class}}

	case lexer.TNew:
		p.lexer.Next()

		// Special-case the weird "new.target" expression here
		if p.lexer.Token == lexer.TDot {
			p.lexer.Next()
			if p.lexer.Token != lexer.TIdentifier || p.lexer.Identifier != "target" {
				p.lexer.Unexpected()
			}
			p.lexer.Next()
			return ast.Expr{loc, &ast.ENewTarget{}}
		}

		target := p.parseExpr(ast.LCall)
		args := []ast.Expr{}

		if p.lexer.Token == lexer.TOpenParen {
			args = p.parseCallArgs()
		}

		return ast.Expr{loc, &ast.ENew{target, args}}

	case lexer.TOpenBracket:
		p.lexer.Next()
		items := []ast.Expr{}
		selfErrors := deferredErrors{}

		// Allow "in" inside arrays
		oldAllowIn := p.allowIn
		p.allowIn = true

		for p.lexer.Token != lexer.TCloseBracket {
			switch p.lexer.Token {
			case lexer.TComma:
				items = append(items, ast.Expr{loc, &ast.EMissing{}})

			case lexer.TDotDotDot:
				dotsLoc := p.lexer.Loc()
				p.lexer.Next()
				item := p.parseExprOrBindings(ast.LComma, &selfErrors)
				items = append(items, ast.Expr{dotsLoc, &ast.ESpread{item}})

				// Commas are not allowed here when destructuring
				if p.lexer.Token == lexer.TComma {
					selfErrors.invalidBindingCommaAfterSpread = p.lexer.Range()
				}

			default:
				item := p.parseExprOrBindings(ast.LComma, &selfErrors)
				items = append(items, item)
			}

			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
		}

		p.lexer.Expect(lexer.TCloseBracket)
		p.allowIn = oldAllowIn

		if p.willNeedBindingPattern() {
			// Is this a binding pattern?
			p.logBindingErrors(&selfErrors)
		} else if errors == nil {
			// Is this an expression?
			p.logExprErrors(&selfErrors)
		} else {
			// In this case, we can't distinguish between the two yet
			selfErrors.mergeInto(errors)
		}

		return ast.Expr{loc, &ast.EArray{items}}

	case lexer.TOpenBrace:
		p.lexer.Next()
		properties := []ast.Property{}
		selfErrors := deferredErrors{}

		// Allow "in" inside object literals
		oldAllowIn := p.allowIn
		p.allowIn = true

		for p.lexer.Token != lexer.TCloseBrace {
			if p.lexer.Token == lexer.TDotDotDot {
				p.lexer.Next()
				value := p.parseExpr(ast.LComma)
				properties = append(properties, ast.Property{
					Kind:  ast.PropertySpread,
					Value: &value,
				})

				// Commas are not allowed here when destructuring
				if p.lexer.Token == lexer.TComma {
					selfErrors.invalidBindingCommaAfterSpread = p.lexer.Range()
				}
			} else {
				property := p.parseProperty(propertyContextObject, ast.PropertyNormal, propertyOpts{}, &selfErrors)
				properties = append(properties, property)
			}

			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
		}

		p.lexer.Expect(lexer.TCloseBrace)
		p.allowIn = oldAllowIn

		if p.willNeedBindingPattern() {
			// Is this a binding pattern?
			p.logBindingErrors(&selfErrors)
		} else if errors == nil {
			// Is this an expression?
			p.logExprErrors(&selfErrors)
		} else {
			// In this case, we can't distinguish between the two yet
			selfErrors.mergeInto(errors)
		}

		return ast.Expr{loc, &ast.EObject{properties}}

	case lexer.TLessThan:
		if p.jsx.Parse {
			// Use NextInsideJSXElement() instead of Next() so we parse "<<" as "<"
			p.lexer.NextInsideJSXElement()
			element := p.parseJSXElement(loc)

			// The call to parseJSXElement() above doesn't consume the last
			// TGreaterThan because the caller knows what Next() function to call.
			// Use Next() instead of NextInsideJSXElement() here since the next
			// token is an expression.
			p.lexer.Next()
			return element
		}
		p.lexer.Unexpected()
		return ast.Expr{}

	case lexer.TImport:
		name := p.lexer.Identifier
		p.lexer.Next()
		return p.parseImportExpr(loc, name)

	default:
		p.lexer.Unexpected()
		return ast.Expr{}
	}
}

func (p *parser) willNeedBindingPattern() bool {
	switch p.lexer.Token {
	case lexer.TEquals:
		// "[a] = b;"
		return true

	case lexer.TIn:
		// "for ([a] in b) {}"
		return !p.allowIn

	case lexer.TIdentifier:
		// "for ([a] of b) {}"
		return !p.allowIn && p.lexer.IsContextualKeyword("of")

	default:
		return false
	}
}

func (p *parser) parseImportExpr(loc ast.Loc, name string) ast.Expr {
	// Parse an "import.meta" expression
	if p.lexer.Token == lexer.TDot {
		p.lexer.Next()
		if p.lexer.IsContextualKeyword("meta") {
			p.lexer.Next()
			return ast.Expr{loc, &ast.EImportMeta{}}
		} else {
			p.lexer.ExpectedString("\"meta\"")
		}
	}

	// Parse a function call for now, then resolve to EImport in the binding pass
	target := ast.Expr{loc, &ast.EIdentifier{p.storeNameInRef(name)}}

	// Allow "in" inside call arguments
	oldAllowIn := p.allowIn
	p.allowIn = true

	p.lexer.Expect(lexer.TOpenParen)
	args := []ast.Expr{p.parseExpr(ast.LComma)}
	p.lexer.Expect(lexer.TCloseParen)

	p.allowIn = oldAllowIn
	return ast.Expr{loc, &ast.ECall{target, args, false}}
}

func (p *parser) parseIndexExpr() ast.Expr {
	value := p.parseExpr(ast.LComma)

	// Warn about commas inside index expressions. In addition to being confusing,
	// these can happen because of mistakes due to automatic semicolon insertion:
	//
	//   console.log('...')
	//   [a, b].forEach(() => ...)
	//
	// This warning will trigger in this case too, which should catch some errors.
	if !p.omitWarnings && p.lexer.Token == lexer.TComma {
		p.log.AddRangeWarning(p.source, p.lexer.Range(),
			"Use of \",\" inside a property access is misleading because JavaScript doesn't have multidimensional arrays")
		p.lexer.Next()
		value = ast.Expr{value.Loc, &ast.EBinary{ast.BinOpComma, value, p.parseExpr(ast.LComma)}}
	}

	return value
}

func (p *parser) parseExprOrBindings(level ast.L, errors *deferredErrors) ast.Expr {
	return p.parseSuffix(p.parsePrefix(level, errors), level)
}

func (p *parser) parseExpr(level ast.L) ast.Expr {
	return p.parseSuffix(p.parsePrefix(level, nil), level)
}

func (p *parser) parseSuffix(left ast.Expr, level ast.L) ast.Expr {
	for {
		switch p.lexer.Token {
		case lexer.TDot:
			p.lexer.Next()
			if !p.lexer.IsIdentifierOrKeyword() {
				p.lexer.Expect(lexer.TIdentifier)
			}
			name := p.lexer.Identifier
			nameLoc := p.lexer.Loc()
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EDot{left, name, nameLoc, false}}

		case lexer.TQuestionDot:
			p.lexer.Next()

			switch p.lexer.Token {
			case lexer.TOpenBracket:
				p.lexer.Next()

				// Allow "in" inside the brackets
				oldAllowIn := p.allowIn
				p.allowIn = true

				index := p.parseIndexExpr()

				p.allowIn = oldAllowIn

				p.lexer.Expect(lexer.TCloseBracket)
				left = ast.Expr{left.Loc, &ast.EIndex{left, index, true}}

			case lexer.TOpenParen:
				if level >= ast.LCall {
					return left
				}
				left = ast.Expr{left.Loc, &ast.ECall{left, p.parseCallArgs(), true}}

			default:
				if !p.lexer.IsIdentifierOrKeyword() {
					p.lexer.Expect(lexer.TIdentifier)
				}
				name := p.lexer.Identifier
				nameLoc := p.lexer.Loc()
				p.lexer.Next()
				left = ast.Expr{left.Loc, &ast.EDot{left, name, nameLoc, true}}
			}

		case lexer.TNoSubstitutionTemplateLiteral:
			if level >= ast.LPrefix {
				return left
			}
			head := p.lexer.StringLiteral
			headRaw := p.lexer.RawTemplateContents()
			p.lexer.Next()
			tag := left
			left = ast.Expr{left.Loc, &ast.ETemplate{&tag, head, headRaw, []ast.TemplatePart{}}}

		case lexer.TTemplateHead:
			if level >= ast.LPrefix {
				return left
			}
			head := p.lexer.StringLiteral
			headRaw := p.lexer.RawTemplateContents()
			parts := p.parseTemplateParts(true /* includeRaw */)
			tag := left
			left = ast.Expr{left.Loc, &ast.ETemplate{&tag, head, headRaw, parts}}

		case lexer.TOpenBracket:
			p.lexer.Next()

			// Allow "in" inside the brackets
			oldAllowIn := p.allowIn
			p.allowIn = true

			index := p.parseIndexExpr()

			p.allowIn = oldAllowIn

			p.lexer.Expect(lexer.TCloseBracket)
			left = ast.Expr{left.Loc, &ast.EIndex{left, index, false}}

		case lexer.TOpenParen:
			if level >= ast.LCall {
				return left
			}
			left = ast.Expr{left.Loc, &ast.ECall{left, p.parseCallArgs(), false}}

		case lexer.TQuestion:
			if level >= ast.LConditional {
				return left
			}
			p.lexer.Next()

			// Allow "in" in between "?" and ":"
			oldAllowIn := p.allowIn
			p.allowIn = true

			yes := p.parseExpr(ast.LComma)

			p.allowIn = oldAllowIn

			p.lexer.Expect(lexer.TColon)
			no := p.parseExpr(ast.LComma)
			left = ast.Expr{left.Loc, &ast.EIf{left, yes, no}}

		case lexer.TMinusMinus:
			if p.lexer.HasNewlineBefore || level >= ast.LPostfix {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EUnary{ast.UnOpPostDec, left}}

		case lexer.TPlusPlus:
			if p.lexer.HasNewlineBefore || level >= ast.LPostfix {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EUnary{ast.UnOpPostInc, left}}

		case lexer.TComma:
			if level >= ast.LComma {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpComma, left, p.parseExpr(ast.LComma)}}

		case lexer.TPlus:
			if level >= ast.LAdd {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpAdd, left, p.parseExpr(ast.LAdd)}}

		case lexer.TPlusEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpAddAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TMinus:
			if level >= ast.LAdd {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpSub, left, p.parseExpr(ast.LAdd)}}

		case lexer.TMinusEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpSubAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TAsterisk:
			if level >= ast.LMultiply {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpMul, left, p.parseExpr(ast.LMultiply)}}

		case lexer.TAsteriskAsterisk:
			if level >= ast.LExponentiation {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpPow, left, p.parseExpr(ast.LExponentiation - 1)}}

		case lexer.TAsteriskAsteriskEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpPowAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TAsteriskEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpMulAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TPercent:
			if level >= ast.LMultiply {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpRem, left, p.parseExpr(ast.LMultiply)}}

		case lexer.TPercentEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpRemAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TSlash:
			if level >= ast.LMultiply {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpDiv, left, p.parseExpr(ast.LMultiply)}}

		case lexer.TSlashEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpDivAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TEqualsEquals:
			if level >= ast.LEquals {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpLooseEq, left, p.parseExpr(ast.LEquals)}}

		case lexer.TExclamationEquals:
			if level >= ast.LEquals {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpLooseNe, left, p.parseExpr(ast.LEquals)}}

		case lexer.TEqualsEqualsEquals:
			if level >= ast.LEquals {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpStrictEq, left, p.parseExpr(ast.LEquals)}}

		case lexer.TExclamationEqualsEquals:
			if level >= ast.LEquals {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpStrictNe, left, p.parseExpr(ast.LEquals)}}

		case lexer.TLessThan:
			if level >= ast.LCompare {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpLt, left, p.parseExpr(ast.LCompare)}}

		case lexer.TLessThanEquals:
			if level >= ast.LCompare {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpLe, left, p.parseExpr(ast.LCompare)}}

		case lexer.TGreaterThan:
			if level >= ast.LCompare {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpGt, left, p.parseExpr(ast.LCompare)}}

		case lexer.TGreaterThanEquals:
			if level >= ast.LCompare {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpGe, left, p.parseExpr(ast.LCompare)}}

		case lexer.TLessThanLessThan:
			if level >= ast.LShift {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpShl, left, p.parseExpr(ast.LShift)}}

		case lexer.TLessThanLessThanEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpShlAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TGreaterThanGreaterThan:
			if level >= ast.LShift {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpShr, left, p.parseExpr(ast.LShift)}}

		case lexer.TGreaterThanGreaterThanEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpShrAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TGreaterThanGreaterThanGreaterThan:
			if level >= ast.LShift {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpUShr, left, p.parseExpr(ast.LShift)}}

		case lexer.TGreaterThanGreaterThanGreaterThanEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpUShrAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TQuestionQuestion:
			if level >= ast.LNullishCoalescing {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpNullishCoalescing, left, p.parseExpr(ast.LNullishCoalescing)}}

		case lexer.TBarBar:
			if level >= ast.LLogicalOr {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpLogicalOr, left, p.parseExpr(ast.LLogicalOr)}}

		case lexer.TAmpersandAmpersand:
			if level >= ast.LLogicalAnd {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpLogicalAnd, left, p.parseExpr(ast.LLogicalAnd)}}

		case lexer.TBar:
			if level >= ast.LBitwiseOr {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpBitwiseOr, left, p.parseExpr(ast.LBitwiseOr)}}

		case lexer.TBarEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpBitwiseOrAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TAmpersand:
			if level >= ast.LBitwiseAnd {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpBitwiseAnd, left, p.parseExpr(ast.LBitwiseAnd)}}

		case lexer.TAmpersandEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpBitwiseAndAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TCaret:
			if level >= ast.LBitwiseXor {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpBitwiseXor, left, p.parseExpr(ast.LBitwiseXor)}}

		case lexer.TCaretEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpBitwiseXorAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TEquals:
			if level >= ast.LAssign {
				return left
			}
			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpAssign, left, p.parseExpr(ast.LAssign - 1)}}

		case lexer.TIn:
			if level >= ast.LCompare || !p.allowIn {
				return left
			}

			// Warn about "!a in b" instead of "!(a in b)"
			if !p.omitWarnings {
				if e, ok := left.Data.(*ast.EUnary); ok && e.Op == ast.UnOpNot {
					p.log.AddWarning(p.source, left.Loc,
						"Suspicious use of the \"!\" operator inside the \"in\" operator")
				}
			}

			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpIn, left, p.parseExpr(ast.LCompare)}}

		case lexer.TInstanceOf:
			if level >= ast.LCompare {
				return left
			}

			// Warn about "!a instanceof b" instead of "!(a instanceof b)"
			if !p.omitWarnings {
				if e, ok := left.Data.(*ast.EUnary); ok && e.Op == ast.UnOpNot {
					p.log.AddWarning(p.source, left.Loc,
						"Suspicious use of the \"!\" operator inside the \"instanceof\" operator")
				}
			}

			p.lexer.Next()
			left = ast.Expr{left.Loc, &ast.EBinary{ast.BinOpInstanceOf, left, p.parseExpr(ast.LCompare)}}

		default:
			return left
		}
	}
}

func (p *parser) parseCallArgs() []ast.Expr {
	// Allow "in" inside call arguments
	oldAllowIn := p.allowIn
	p.allowIn = true

	args := []ast.Expr{}
	p.lexer.Expect(lexer.TOpenParen)

	for p.lexer.Token != lexer.TCloseParen {
		loc := p.lexer.Loc()
		isSpread := p.lexer.Token == lexer.TDotDotDot
		if isSpread {
			p.lexer.Next()
		}
		arg := p.parseExpr(ast.LComma)
		if isSpread {
			arg = ast.Expr{loc, &ast.ESpread{arg}}
		}
		args = append(args, arg)
		if p.lexer.Token != lexer.TComma {
			break
		}
		p.lexer.Next()
	}

	p.lexer.Expect(lexer.TCloseParen)
	p.allowIn = oldAllowIn
	return args
}

func (p *parser) parseJSXTag() (ast.Range, string, *ast.Expr) {
	loc := p.lexer.Loc()

	// A missing tag is a fragment
	if p.lexer.Token == lexer.TGreaterThan {
		return ast.Range{loc, 0}, "", nil
	}

	// The tag is an identifier
	name := p.lexer.Identifier
	tagRange := p.lexer.Range()
	p.lexer.ExpectInsideJSXElement(lexer.TIdentifier)

	// Certain identifiers are strings
	if strings.ContainsRune(name, '-') || (p.lexer.Token != lexer.TDot && name[0] >= 'a' && name[0] <= 'z') {
		return tagRange, name, &ast.Expr{loc, &ast.EString{lexer.StringToUTF16(name)}}
	}

	// Otherwise, this is an identifier
	tag := &ast.Expr{loc, &ast.EIdentifier{p.storeNameInRef(name)}}

	// Parse a member expression chain
	for p.lexer.Token == lexer.TDot {
		p.lexer.NextInsideJSXElement()
		memberRange := p.lexer.Range()
		member := p.lexer.Identifier
		p.lexer.ExpectInsideJSXElement(lexer.TIdentifier)

		// Dashes are not allowed in member expression chains
		index := strings.IndexByte(member, '-')
		if index >= 0 {
			p.addError(ast.Loc{memberRange.Loc.Start + int32(index)}, "Unexpected \"-\"")
			panic(lexer.LexerPanic{})
		}

		name += "." + member
		tag = &ast.Expr{loc, &ast.EDot{*tag, member, memberRange.Loc, false}}
		tagRange.Len = memberRange.Loc.Start + memberRange.Len - tagRange.Loc.Start
	}

	return tagRange, name, tag
}

func (p *parser) parseJSXElement(loc ast.Loc) ast.Expr {
	// Parse the tag
	_, startText, startTag := p.parseJSXTag()

	// Parse attributes
	properties := []ast.Property{}
	if startTag != nil {
	parseAttributes:
		for {
			switch p.lexer.Token {
			case lexer.TIdentifier:
				// Parse the key
				keyRange := p.lexer.Range()
				key := ast.Expr{keyRange.Loc, &ast.EString{lexer.StringToUTF16(p.lexer.Identifier)}}
				p.lexer.NextInsideJSXElement()

				// Parse the value
				var value ast.Expr
				if p.lexer.Token != lexer.TEquals {
					// Implicitly true value
					value = ast.Expr{ast.Loc{keyRange.Loc.Start + keyRange.Len}, &ast.EBoolean{true}}
				} else {
					// Use NextInsideJSXElement() not Next() so we can parse a JSX-style string literal
					p.lexer.NextInsideJSXElement()
					if p.lexer.Token == lexer.TStringLiteral {
						value = ast.Expr{p.lexer.Loc(), &ast.EString{p.lexer.StringLiteral}}
						p.lexer.NextInsideJSXElement()
					} else {
						// Use Expect() not ExpectInsideJSXElement() so we can parse expression tokens
						p.lexer.Expect(lexer.TOpenBrace)
						value = p.parseExpr(ast.LLowest)
						p.lexer.ExpectInsideJSXElement(lexer.TCloseBrace)
					}
				}

				// Add a property
				properties = append(properties, ast.Property{
					Key:   key,
					Value: &value,
				})

			case lexer.TOpenBrace:
				// Use Next() not ExpectInsideJSXElement() so we can parse "..."
				p.lexer.Next()
				p.lexer.Expect(lexer.TDotDotDot)
				value := p.parseExpr(ast.LComma)
				properties = append(properties, ast.Property{
					Kind:  ast.PropertySpread,
					Value: &value,
				})

				// Use NextInsideJSXElement() not Next() so we can parse ">>" as ">"
				p.lexer.NextInsideJSXElement()

			default:
				break parseAttributes
			}
		}
	}

	// A slash here is a self-closing element
	if p.lexer.Token == lexer.TSlash {
		// Use NextInsideJSXElement() not Next() so we can parse ">>" as ">"
		p.lexer.NextInsideJSXElement()
		if p.lexer.Token != lexer.TGreaterThan {
			p.lexer.Expected(lexer.TGreaterThan)
		}
		return ast.Expr{loc, &ast.EJSXElement{startTag, properties, []ast.Expr{}}}
	}

	// Use ExpectJSXElementChild() so we parse child strings
	p.lexer.ExpectJSXElementChild(lexer.TGreaterThan)

	// Parse the children of this element
	children := []ast.Expr{}
	for {
		switch p.lexer.Token {
		case lexer.TStringLiteral:
			children = append(children, ast.Expr{p.lexer.Loc(), &ast.EString{p.lexer.StringLiteral}})
			p.lexer.NextJSXElementChild()

		case lexer.TOpenBrace:
			// Use Next() instead of NextJSXElementChild() here since the next token is an expression
			p.lexer.Next()

			// The expression is optional, and may be absent
			if p.lexer.Token != lexer.TCloseBrace {
				children = append(children, p.parseExpr(ast.LLowest))
			}

			// Use ExpectJSXElementChild() so we parse child strings
			p.lexer.ExpectJSXElementChild(lexer.TCloseBrace)

		case lexer.TLessThan:
			lessThanLoc := p.lexer.Loc()
			p.lexer.NextInsideJSXElement()

			if p.lexer.Token != lexer.TSlash {
				// This is a child element
				children = append(children, p.parseJSXElement(lessThanLoc))

				// The call to parseJSXElement() above doesn't consume the last
				// TGreaterThan because the caller knows what Next() function to call.
				// Use NextJSXElementChild() here since the next token is an element
				// child.
				p.lexer.NextJSXElementChild()
				continue
			}

			// This is the closing element
			p.lexer.NextInsideJSXElement()
			endRange, endText, _ := p.parseJSXTag()
			if startText != endText {
				p.addRangeError(endRange, fmt.Sprintf("Expected closing tag %q to match opening tag %q", endText, startText))
			}
			if p.lexer.Token != lexer.TGreaterThan {
				p.lexer.Expected(lexer.TGreaterThan)
			}

			return ast.Expr{loc, &ast.EJSXElement{startTag, properties, children}}

		default:
			p.lexer.Unexpected()
		}
	}
}

func (p *parser) parseTemplateParts(includeRaw bool) []ast.TemplatePart {
	parts := []ast.TemplatePart{}
	for {
		p.lexer.Next()
		value := p.parseExpr(ast.LLowest)
		p.lexer.RescanCloseBraceAsTemplateToken()
		tail := p.lexer.StringLiteral
		tailRaw := ""
		if includeRaw {
			tailRaw = p.lexer.RawTemplateContents()
		}
		parts = append(parts, ast.TemplatePart{value, tail, tailRaw})
		if p.lexer.Token == lexer.TTemplateTail {
			p.lexer.Next()
			break
		}
	}
	return parts
}

func (p *parser) parseDecls() []ast.Decl {
	decls := []ast.Decl{}

	for {
		var value *ast.Expr
		local := p.parseBinding()

		if p.lexer.Token == lexer.TEquals {
			p.lexer.Next()
			expr := p.parseExpr(ast.LComma)
			value = &expr
		}

		decls = append(decls, ast.Decl{local, value})

		if p.lexer.Token != lexer.TComma {
			break
		}
		p.lexer.Next()
	}

	return decls
}

func (p *parser) requireInitializers(decls []ast.Decl) {
	for _, d := range decls {
		if d.Value == nil {
			if _, ok := d.Binding.Data.(*ast.BIdentifier); ok {
				p.addError(d.Binding.Loc, "This constant must be initialized")
			}
		}
	}
}

func (p *parser) forbidInitializers(decls []ast.Decl, loopType string, isVar bool) {
	if len(decls) > 1 {
		p.addError(decls[0].Binding.Loc, fmt.Sprintf("for-%s loops must have a single declaration", loopType))
	} else if len(decls) == 1 && decls[0].Value != nil {
		if isVar {
			if _, ok := decls[0].Binding.Data.(*ast.BIdentifier); ok {
				// This is a weird special case. Initializers are allowed in "var"
				// statements with identifier bindings.
				return
			}
		}
		p.addError(decls[0].Value.Loc, fmt.Sprintf("for-%s loop variables cannot have an initializer", loopType))
	}
}

func (p *parser) parseImportClause() []ast.ClauseItem {
	items := []ast.ClauseItem{}
	p.lexer.Expect(lexer.TOpenBrace)

	for p.lexer.Token != lexer.TCloseBrace {
		alias := p.lexer.Identifier
		aliasLoc := p.lexer.Loc()
		name := ast.LocRef{aliasLoc, p.storeNameInRef(alias)}

		// The alias may be a keyword
		isIdentifier := p.lexer.Token == lexer.TIdentifier
		if !p.lexer.IsIdentifierOrKeyword() {
			p.lexer.Expect(lexer.TIdentifier)
		}
		p.lexer.Next()

		if p.lexer.IsContextualKeyword("as") {
			p.lexer.Next()
			name = ast.LocRef{p.lexer.Loc(), p.storeNameInRef(p.lexer.Identifier)}
			p.lexer.Expect(lexer.TIdentifier)
		} else if !isIdentifier {
			// An import where the name is a keyword must have an alias
			p.lexer.Unexpected()
		}

		items = append(items, ast.ClauseItem{alias, aliasLoc, name})

		if p.lexer.Token != lexer.TComma {
			break
		}
		p.lexer.Next()
	}

	p.lexer.Expect(lexer.TCloseBrace)
	return items
}

func (p *parser) parseExportClause() []ast.ClauseItem {
	items := []ast.ClauseItem{}
	firstKeywordItemLoc := ast.Loc{}
	p.lexer.Expect(lexer.TOpenBrace)

	for p.lexer.Token != lexer.TCloseBrace {
		alias := p.lexer.Identifier
		aliasLoc := p.lexer.Loc()
		name := ast.LocRef{aliasLoc, p.storeNameInRef(alias)}

		// The name can actually be a keyword if we're really an "export from"
		// statement. However, we won't know until later. Allow keywords as
		// identifiers for now and throw an error later if there's no "from".
		//
		//   // This is fine
		//   export { default } from 'path'
		//
		//   // This is a syntax error
		//   export { default }
		//
		if p.lexer.Token != lexer.TIdentifier {
			if !p.lexer.IsIdentifierOrKeyword() {
				p.lexer.Expect(lexer.TIdentifier)
			}
			if firstKeywordItemLoc.Start == 0 {
				firstKeywordItemLoc = p.lexer.Loc()
			}
		}
		p.lexer.Next()

		if p.lexer.IsContextualKeyword("as") {
			p.lexer.Next()
			alias = p.lexer.Identifier
			aliasLoc = p.lexer.Loc()

			// The alias may be a keyword
			if !p.lexer.IsIdentifierOrKeyword() {
				p.lexer.Expect(lexer.TIdentifier)
			}
			p.lexer.Next()
		}

		items = append(items, ast.ClauseItem{alias, aliasLoc, name})

		if p.lexer.Token != lexer.TComma {
			break
		}
		p.lexer.Next()
	}

	p.lexer.Expect(lexer.TCloseBrace)

	// Throw an error here if we found a keyword earlier and this isn't an
	// "export from" statement after all
	if firstKeywordItemLoc.Start != 0 && !p.lexer.IsContextualKeyword("from") {
		r := lexer.RangeOfIdentifier(p.source, firstKeywordItemLoc)
		p.addRangeError(r, fmt.Sprintf("Expected identifier but found %q", p.source.TextForRange(r)))
		panic(lexer.LexerPanic{})
	}

	return items
}

func (p *parser) parseBinding() ast.Binding {
	loc := p.lexer.Loc()

	switch p.lexer.Token {
	case lexer.TIdentifier:
		ref := p.storeNameInRef(p.lexer.Identifier)
		p.lexer.Next()
		return ast.Binding{loc, &ast.BIdentifier{ref}}

	case lexer.TOpenBracket:
		p.lexer.Next()
		items := []ast.ArrayBinding{}
		hasSpread := false

		for p.lexer.Token != lexer.TCloseBracket {
			if p.lexer.Token == lexer.TComma {
				binding := ast.Binding{p.lexer.Loc(), &ast.BMissing{}}
				items = append(items, ast.ArrayBinding{binding, nil})
			} else {
				if p.lexer.Token == lexer.TDotDotDot {
					p.lexer.Next()
					hasSpread = true
				}

				binding := p.parseBinding()

				var defaultValue *ast.Expr
				if !hasSpread && p.lexer.Token == lexer.TEquals {
					p.lexer.Next()
					value := p.parseExpr(ast.LComma)
					defaultValue = &value
				}

				items = append(items, ast.ArrayBinding{binding, defaultValue})

				// Commas after spread elements are not allowed
				if hasSpread && p.lexer.Token == lexer.TComma {
					p.addRangeError(p.lexer.Range(), "Unexpected \",\" after rest pattern")
					panic(lexer.LexerPanic{})
				}
			}

			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
		}

		p.lexer.Expect(lexer.TCloseBracket)
		return ast.Binding{loc, &ast.BArray{items, hasSpread}}

	case lexer.TOpenBrace:
		p.lexer.Next()
		properties := []ast.PropertyBinding{}

		for p.lexer.Token != lexer.TCloseBrace {
			property := p.parsePropertyBinding()
			properties = append(properties, property)

			// Commas after spread elements are not allowed
			if property.IsSpread && p.lexer.Token == lexer.TComma {
				p.addRangeError(p.lexer.Range(), "Unexpected \",\" after rest pattern")
				panic(lexer.LexerPanic{})
			}

			if p.lexer.Token != lexer.TComma {
				break
			}
			p.lexer.Next()
		}

		p.lexer.Expect(lexer.TCloseBrace)
		return ast.Binding{loc, &ast.BObject{properties}}
	}

	p.lexer.Expect(lexer.TIdentifier)
	return ast.Binding{}
}

func (p *parser) parseFn(name *ast.LocRef, opts fnOpts) ast.Fn {
	args := []ast.Arg{}
	hasRestArg := false
	p.lexer.Expect(lexer.TOpenParen)

	for p.lexer.Token != lexer.TCloseParen {
		if !hasRestArg && p.lexer.Token == lexer.TDotDotDot {
			p.lexer.Next()
			hasRestArg = true
		}

		arg := p.parseBinding()

		var defaultValue *ast.Expr
		if !hasRestArg && p.lexer.Token == lexer.TEquals {
			p.lexer.Next()
			value := p.parseExpr(ast.LComma)
			defaultValue = &value
		}

		args = append(args, ast.Arg{arg, defaultValue})
		if p.lexer.Token != lexer.TComma {
			break
		}
		if hasRestArg {
			p.lexer.Expect(lexer.TCloseParen)
		}
		p.lexer.Next()
	}

	p.lexer.Expect(lexer.TCloseParen)
	stmts := p.parseFnBodyStmts(opts)

	return ast.Fn{
		Name:        name,
		Args:        args,
		HasRestArg:  hasRestArg,
		IsAsync:     opts.allowAwait,
		IsGenerator: opts.allowYield,
		Stmts:       stmts,
	}
}

func (p *parser) parseClass(name *ast.LocRef) ast.Class {
	var extends *ast.Expr

	if p.lexer.Token == lexer.TExtends {
		p.lexer.Next()
		value := p.parseExpr(ast.LNew)
		extends = &value
	}

	p.lexer.Expect(lexer.TOpenBrace)
	properties := []ast.Property{}

	// Allow "in" inside class bodies
	oldAllowIn := p.allowIn
	p.allowIn = true

	for p.lexer.Token != lexer.TCloseBrace {
		if p.lexer.Token == lexer.TSemicolon {
			p.lexer.Next()
			continue
		}

		property := p.parseProperty(propertyContextClass, ast.PropertyNormal, propertyOpts{}, nil)
		properties = append(properties, property)
	}

	p.allowIn = oldAllowIn

	p.lexer.Expect(lexer.TCloseBrace)
	return ast.Class{name, extends, properties}
}

func (p *parser) parseLabelName() *ast.LocRef {
	if p.lexer.Token != lexer.TIdentifier || p.lexer.HasNewlineBefore {
		return nil
	}

	name := ast.LocRef{p.lexer.Loc(), p.storeNameInRef(p.lexer.Identifier)}
	p.lexer.Next()
	return &name
}

func (p *parser) parsePath() ast.Path {
	path := ast.Path{p.lexer.Loc(), lexer.UTF16ToString(p.lexer.StringLiteral)}
	if p.lexer.Token == lexer.TNoSubstitutionTemplateLiteral {
		p.lexer.Next()
	} else {
		p.lexer.Expect(lexer.TStringLiteral)
	}
	return path
}

// This assumes the "function" token has already been parsed
func (p *parser) parseFnStmt(loc ast.Loc, opts parseStmtOpts, isAsync bool) ast.Stmt {
	isGenerator := p.lexer.Token == lexer.TAsterisk
	if isGenerator {
		p.lexer.Next()
	}
	var name *ast.LocRef
	if !opts.isNameOptional || p.lexer.Token == lexer.TIdentifier {
		name = &ast.LocRef{p.lexer.Loc(), p.storeNameInRef(p.lexer.Identifier)}
		p.lexer.Expect(lexer.TIdentifier)
	}
	fn := p.parseFn(name, fnOpts{
		allowAwait: isAsync,
		allowYield: isGenerator,
	})
	return ast.Stmt{loc, &ast.SFunction{fn, opts.isExport}}
}

type parseStmtOpts struct {
	allowImportAndExport bool
	isExport             bool
	isNameOptional       bool // For "export default" pseudo-statements
}

func (p *parser) parseStmt(opts parseStmtOpts) ast.Stmt {
	loc := p.lexer.Loc()

	switch p.lexer.Token {
	case lexer.TSemicolon:
		p.lexer.Next()
		return ast.Stmt{loc, &ast.SEmpty{}}

	case lexer.TExport:
		if !opts.allowImportAndExport {
			p.lexer.Unexpected()
		}
		p.lexer.Next()

		switch p.lexer.Token {
		case lexer.TClass, lexer.TConst, lexer.TFunction, lexer.TLet, lexer.TVar:
			opts.isExport = true
			return p.parseStmt(opts)

		case lexer.TIdentifier:
			if p.lexer.IsContextualKeyword("async") {
				p.lexer.Next()
				p.lexer.Expect(lexer.TFunction)
				opts.isExport = true
				return p.parseFnStmt(loc, opts, true /* isAsync */)
			}
			p.lexer.Unexpected()
			return ast.Stmt{}

		case lexer.TDefault:
			name := ast.LocRef{p.lexer.Loc(), p.storeNameInRef(p.lexer.Raw())}
			p.lexer.Next()

			if p.lexer.Token == lexer.TFunction || p.lexer.Token == lexer.TClass {
				stmt := p.parseStmt(parseStmtOpts{isNameOptional: true})
				return ast.Stmt{loc, &ast.SExportDefault{name, ast.ExprOrStmt{Stmt: &stmt}}}
			}

			expr := p.parseExpr(ast.LComma)
			p.lexer.ExpectOrInsertSemicolon()
			return ast.Stmt{loc, &ast.SExportDefault{name, ast.ExprOrStmt{Expr: &expr}}}

		case lexer.TAsterisk:
			p.lexer.Next()
			var item *ast.ClauseItem
			if p.lexer.IsContextualKeyword("as") {
				p.lexer.Next()
				name := p.lexer.Identifier
				nameLoc := p.lexer.Loc()
				item = &ast.ClauseItem{name, nameLoc, ast.LocRef{nameLoc, p.storeNameInRef(name)}}
				p.lexer.Expect(lexer.TIdentifier)
			}
			p.lexer.ExpectContextualKeyword("from")
			path := p.parsePath()
			p.lexer.ExpectOrInsertSemicolon()
			p.importPaths = append(p.importPaths, ast.ImportPath{Path: path})
			return ast.Stmt{loc, &ast.SExportStar{item, path}}

		case lexer.TOpenBrace:
			items := p.parseExportClause()
			if p.lexer.IsContextualKeyword("from") {
				p.lexer.Next()
				path := p.parsePath()
				p.importPaths = append(p.importPaths, ast.ImportPath{Path: path})
				p.lexer.ExpectOrInsertSemicolon()
				return ast.Stmt{loc, &ast.SExportFrom{items, ast.InvalidRef, path}}
			}
			p.lexer.ExpectOrInsertSemicolon()
			return ast.Stmt{loc, &ast.SExportClause{items}}

		default:
			p.lexer.Unexpected()
			return ast.Stmt{}
		}

	case lexer.TFunction:
		p.lexer.Next()
		return p.parseFnStmt(loc, opts, false /* isAsync */)

	case lexer.TClass:
		p.lexer.Next()
		var name *ast.LocRef
		if !opts.isNameOptional || p.lexer.Token == lexer.TIdentifier {
			name = &ast.LocRef{p.lexer.Loc(), p.storeNameInRef(p.lexer.Identifier)}
			p.lexer.Expect(lexer.TIdentifier)
		}
		class := p.parseClass(name)
		return ast.Stmt{loc, &ast.SClass{class, opts.isExport}}

	case lexer.TVar:
		p.lexer.Next()
		decls := p.parseDecls()
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{loc, &ast.SVar{decls, opts.isExport}}

	case lexer.TLet:
		p.lexer.Next()
		decls := p.parseDecls()
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{loc, &ast.SLet{decls, opts.isExport}}

	case lexer.TConst:
		p.lexer.Next()
		decls := p.parseDecls()
		p.lexer.ExpectOrInsertSemicolon()
		p.requireInitializers(decls)
		return ast.Stmt{loc, &ast.SConst{decls, opts.isExport}}

	case lexer.TIf:
		p.lexer.Next()
		p.lexer.Expect(lexer.TOpenParen)
		test := p.parseExpr(ast.LLowest)
		p.lexer.Expect(lexer.TCloseParen)
		yes := p.parseStmt(parseStmtOpts{})
		var no *ast.Stmt = nil
		if p.lexer.Token == lexer.TElse {
			p.lexer.Next()
			stmt := p.parseStmt(parseStmtOpts{})
			no = &stmt
		}
		return ast.Stmt{loc, &ast.SIf{test, yes, no}}

	case lexer.TDo:
		p.lexer.Next()
		body := p.parseStmt(parseStmtOpts{})
		p.lexer.Expect(lexer.TWhile)
		p.lexer.Expect(lexer.TOpenParen)
		test := p.parseExpr(ast.LLowest)
		p.lexer.Expect(lexer.TCloseParen)

		// This is a weird corner case where automatic semicolon insertion applies
		// even without a newline present
		if p.lexer.Token == lexer.TSemicolon {
			p.lexer.Next()
		}
		return ast.Stmt{loc, &ast.SDoWhile{body, test}}

	case lexer.TWhile:
		p.lexer.Next()
		p.lexer.Expect(lexer.TOpenParen)
		test := p.parseExpr(ast.LLowest)
		p.lexer.Expect(lexer.TCloseParen)
		body := p.parseStmt(parseStmtOpts{})
		return ast.Stmt{loc, &ast.SWhile{test, body}}

	case lexer.TWith:
		p.lexer.Next()
		p.lexer.Expect(lexer.TOpenParen)
		test := p.parseExpr(ast.LLowest)
		p.lexer.Expect(lexer.TCloseParen)
		body := p.parseStmt(parseStmtOpts{})
		return ast.Stmt{loc, &ast.SWith{test, body}}

	case lexer.TSwitch:
		p.lexer.Next()
		p.lexer.Expect(lexer.TOpenParen)
		test := p.parseExpr(ast.LLowest)
		p.lexer.Expect(lexer.TCloseParen)
		p.lexer.Expect(lexer.TOpenBrace)
		cases := []ast.Case{}

		for p.lexer.Token != lexer.TCloseBrace {
			var value *ast.Expr = nil
			body := []ast.Stmt{}

			if p.lexer.Token == lexer.TDefault {
				p.lexer.Next()
				p.lexer.Expect(lexer.TColon)
			} else {
				p.lexer.Expect(lexer.TCase)
				expr := p.parseExpr(ast.LLowest)
				value = &expr
				p.lexer.Expect(lexer.TColon)
			}

		caseBody:
			for {
				switch p.lexer.Token {
				case lexer.TCloseBrace, lexer.TCase, lexer.TDefault:
					break caseBody

				default:
					body = append(body, p.parseStmt(parseStmtOpts{}))
				}
			}

			cases = append(cases, ast.Case{value, body})
		}

		p.lexer.Expect(lexer.TCloseBrace)
		return ast.Stmt{loc, &ast.SSwitch{test, cases}}

	case lexer.TTry:
		p.lexer.Next()
		p.lexer.Expect(lexer.TOpenBrace)
		body := p.parseStmtsUpTo(lexer.TCloseBrace, parseStmtOpts{})
		p.lexer.Next()

		var catch *ast.Catch = nil
		var finally *[]ast.Stmt = nil

		if p.lexer.Token == lexer.TCatch {
			p.lexer.Next()
			var binding *ast.Binding

			// The catch binding is optional, and can be omitted
			if p.lexer.Token != lexer.TOpenBrace {
				p.lexer.Expect(lexer.TOpenParen)
				value := p.parseBinding()
				binding = &value
				p.lexer.Expect(lexer.TCloseParen)
			}

			p.lexer.Expect(lexer.TOpenBrace)
			stmts := p.parseStmtsUpTo(lexer.TCloseBrace, parseStmtOpts{})
			p.lexer.Next()
			catch = &ast.Catch{binding, stmts}
		}

		if p.lexer.Token == lexer.TFinally || catch == nil {
			p.lexer.Expect(lexer.TFinally)
			p.lexer.Expect(lexer.TOpenBrace)
			stmts := p.parseStmtsUpTo(lexer.TCloseBrace, parseStmtOpts{})
			p.lexer.Next()
			finally = &stmts
		}

		return ast.Stmt{loc, &ast.STry{body, catch, finally}}

	case lexer.TFor:
		p.lexer.Next()

		// "for await (let x of y) {}"
		isAwait := p.lexer.IsContextualKeyword("await")
		if isAwait {
			if !p.currentFnOpts.allowAwait {
				p.addRangeError(p.lexer.Range(), "Cannot use \"await\" outside an async function")
				isAwait = false
			}
			p.lexer.Next()
		}

		p.lexer.Expect(lexer.TOpenParen)

		var init *ast.Stmt = nil
		var test *ast.Expr = nil
		var update *ast.Expr = nil

		// "in" expressions aren't allowed here
		p.allowIn = false

		decls := []ast.Decl{}
		initLoc := p.lexer.Loc()
		switch p.lexer.Token {
		case lexer.TVar:
			p.lexer.Next()
			decls = p.parseDecls()
			init = &ast.Stmt{initLoc, &ast.SVar{decls, false}}

		case lexer.TLet:
			p.lexer.Next()
			decls = p.parseDecls()
			init = &ast.Stmt{initLoc, &ast.SLet{decls, false}}

		case lexer.TConst:
			p.lexer.Next()
			decls = p.parseDecls()
			init = &ast.Stmt{initLoc, &ast.SConst{decls, false}}

		case lexer.TSemicolon:

		default:
			init = &ast.Stmt{initLoc, &ast.SExpr{p.parseExpr(ast.LLowest)}}
		}

		// "in" expressions are allowed again
		p.allowIn = true

		// Detect for-of loops
		if p.lexer.IsContextualKeyword("of") || isAwait {
			if isAwait && !p.lexer.IsContextualKeyword("of") {
				if init != nil {
					p.lexer.ExpectedString("\"of\"")
				} else {
					p.lexer.Unexpected()
				}
			}
			p.forbidInitializers(decls, "of", false)
			p.lexer.Next()
			value := p.parseExpr(ast.LLowest)
			p.lexer.Expect(lexer.TCloseParen)
			body := p.parseStmt(parseStmtOpts{})
			return ast.Stmt{loc, &ast.SForOf{isAwait, *init, value, body}}
		}

		// Detect for-in loops
		if p.lexer.Token == lexer.TIn {
			_, isVar := init.Data.(*ast.SVar)
			p.forbidInitializers(decls, "in", isVar)
			p.lexer.Next()
			value := p.parseExpr(ast.LLowest)
			p.lexer.Expect(lexer.TCloseParen)
			body := p.parseStmt(parseStmtOpts{})
			return ast.Stmt{loc, &ast.SForIn{*init, value, body}}
		}

		// Only require "const" statement initializers when we know we're a normal for loop
		if init != nil {
			if _, ok := init.Data.(*ast.SConst); ok {
				p.requireInitializers(decls)
			}
		}

		p.lexer.Expect(lexer.TSemicolon)

		if p.lexer.Token != lexer.TSemicolon {
			expr := p.parseExpr(ast.LLowest)
			test = &expr
		}

		p.lexer.Expect(lexer.TSemicolon)

		if p.lexer.Token != lexer.TCloseParen {
			expr := p.parseExpr(ast.LLowest)
			update = &expr
		}

		p.lexer.Expect(lexer.TCloseParen)
		body := p.parseStmt(parseStmtOpts{})
		return ast.Stmt{loc, &ast.SFor{init, test, update, body}}

	case lexer.TImport:
		name := p.lexer.Identifier
		p.lexer.Next()
		stmt := ast.SImport{NamespaceRef: ast.InvalidRef}

		switch p.lexer.Token {
		case lexer.TOpenParen, lexer.TDot:
			// "import('path')"
			// "import.meta"
			expr := p.parseSuffix(p.parseImportExpr(loc, name), ast.LLowest)
			p.lexer.ExpectOrInsertSemicolon()
			return ast.Stmt{loc, &ast.SExpr{expr}}

		case lexer.TStringLiteral, lexer.TNoSubstitutionTemplateLiteral:
			// "import 'path'"
			if !opts.allowImportAndExport {
				p.lexer.Unexpected()
				return ast.Stmt{}
			}

		case lexer.TAsterisk:
			// "import * as ns from 'path'"
			if !opts.allowImportAndExport {
				p.lexer.Unexpected()
				return ast.Stmt{}
			}

			p.lexer.Next()
			p.lexer.ExpectContextualKeyword("as")
			stmt.NamespaceRef = p.storeNameInRef(p.lexer.Identifier)
			starLoc := p.lexer.Loc()
			stmt.StarLoc = &starLoc
			p.lexer.Expect(lexer.TIdentifier)
			p.lexer.ExpectContextualKeyword("from")

		case lexer.TOpenBrace:
			// "import {item1, item2} from 'path'"
			if !opts.allowImportAndExport {
				p.lexer.Unexpected()
				return ast.Stmt{}
			}

			items := p.parseImportClause()
			stmt.Items = &items
			p.lexer.ExpectContextualKeyword("from")

		case lexer.TIdentifier:
			// "import defaultItem from 'path'"
			if !opts.allowImportAndExport {
				p.lexer.Unexpected()
				return ast.Stmt{}
			}

			stmt.DefaultName = &ast.LocRef{p.lexer.Loc(), p.storeNameInRef(p.lexer.Identifier)}
			p.lexer.Next()
			if p.lexer.Token == lexer.TComma {
				p.lexer.Next()
				switch p.lexer.Token {
				case lexer.TAsterisk:
					// "import defaultItem, * as ns from 'path'"
					p.lexer.Next()
					p.lexer.ExpectContextualKeyword("as")
					stmt.NamespaceRef = p.storeNameInRef(p.lexer.Identifier)
					starLoc := p.lexer.Loc()
					stmt.StarLoc = &starLoc
					p.lexer.Expect(lexer.TIdentifier)

				case lexer.TOpenBrace:
					// "import defaultItem, {item1, item2} from 'path'"
					items := p.parseImportClause()
					stmt.Items = &items

				default:
					p.lexer.Unexpected()
				}
			}
			p.lexer.ExpectContextualKeyword("from")

		default:
			p.lexer.Unexpected()
			return ast.Stmt{}
		}

		stmt.Path = p.parsePath()
		p.lexer.ExpectOrInsertSemicolon()
		p.importPaths = append(p.importPaths, ast.ImportPath{Path: stmt.Path})
		return ast.Stmt{loc, &stmt}

	case lexer.TBreak:
		p.lexer.Next()
		name := p.parseLabelName()
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{loc, &ast.SBreak{name}}

	case lexer.TContinue:
		p.lexer.Next()
		name := p.parseLabelName()
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{loc, &ast.SContinue{name}}

	case lexer.TReturn:
		p.lexer.Next()
		var value *ast.Expr
		if p.lexer.Token != lexer.TSemicolon &&
			!p.lexer.HasNewlineBefore &&
			p.lexer.Token != lexer.TCloseBrace &&
			p.lexer.Token != lexer.TEndOfFile {
			expr := p.parseExpr(ast.LLowest)
			value = &expr
		}
		p.latestReturnHadSemicolon = p.lexer.Token == lexer.TSemicolon
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{loc, &ast.SReturn{value}}

	case lexer.TThrow:
		p.lexer.Next()
		if p.lexer.HasNewlineBefore {
			p.addError(ast.Loc{loc.Start + 5}, "Unexpected newline after \"throw\"")
			panic(lexer.LexerPanic{})
		}
		expr := p.parseExpr(ast.LLowest)
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{loc, &ast.SThrow{expr}}

	case lexer.TDebugger:
		p.lexer.Next()
		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{loc, &ast.SDebugger{}}

	case lexer.TOpenBrace:
		p.lexer.Next()
		stmts := p.parseStmtsUpTo(lexer.TCloseBrace, parseStmtOpts{})
		p.lexer.Next()
		return ast.Stmt{loc, &ast.SBlock{stmts}}

	default:
		isIdentifier := p.lexer.Token == lexer.TIdentifier

		var expr ast.Expr
		if isIdentifier && p.lexer.Identifier == "async" {
			p.lexer.Next()
			if p.lexer.Token == lexer.TFunction {
				p.lexer.Next()
				return p.parseFnStmt(loc, opts, true /* isAsync */)
			}
			expr = p.parseAsyncExpr(loc, ast.LLowest)
		} else {
			expr = p.parseExpr(ast.LLowest)
		}

		// Parse a labeled statement
		if ident, ok := expr.Data.(*ast.EIdentifier); ok && isIdentifier && p.lexer.Token == lexer.TColon {
			p.lexer.Next()
			name := ast.LocRef{expr.Loc, ident.Ref}
			stmt := p.parseStmt(parseStmtOpts{})
			return ast.Stmt{loc, &ast.SLabel{name, stmt}}
		}

		p.lexer.ExpectOrInsertSemicolon()
		return ast.Stmt{loc, &ast.SExpr{expr}}
	}
}

func (p *parser) parseFnBodyStmts(opts fnOpts) []ast.Stmt {
	oldFnOpts := p.currentFnOpts
	p.currentFnOpts = opts

	p.lexer.Expect(lexer.TOpenBrace)
	stmts := p.parseStmtsUpTo(lexer.TCloseBrace, parseStmtOpts{})
	p.lexer.Next()

	p.currentFnOpts = oldFnOpts
	return stmts
}

func (p *parser) parseStmtsUpTo(end lexer.T, opts parseStmtOpts) []ast.Stmt {
	stmts := []ast.Stmt{}
	returnWithoutSemicolonStart := int32(-1)

	for p.lexer.Token != end {
		stmt := p.parseStmt(opts)
		stmts = append(stmts, stmt)

		// Warn about ASI and return statements
		if !p.omitWarnings {
			if s, ok := stmt.Data.(*ast.SReturn); ok && s.Value == nil && !p.latestReturnHadSemicolon {
				returnWithoutSemicolonStart = stmt.Loc.Start
			} else {
				if returnWithoutSemicolonStart != -1 {
					if _, ok := stmt.Data.(*ast.SExpr); ok {
						p.log.AddWarning(p.source, ast.Loc{returnWithoutSemicolonStart + 6},
							"The following expression is not returned because of an automatically-inserted semicolon")
					}
				}
				returnWithoutSemicolonStart = -1
			}
		}
	}

	return stmts
}

type binder struct {
	log               logging.Log
	source            logging.Source
	moduleScope       *ast.Scope
	scope             *ast.Scope
	symbols           []ast.Symbol
	unbound           []ast.Ref
	importPaths       []ast.ImportPath
	exportsRef        ast.Ref
	requireRef        ast.Ref
	moduleRef         ast.Ref
	importRef         ast.Ref
	callTarget        ast.E
	identifierDefines map[string]ast.E
	dotDefines        map[string]dotDefine
	mangleSyntax      bool
	isControlFlowDead bool
	omitWarnings      bool
	jsx               JSXOptions
	allocatedNames    []string
	tryBodyCount      int
	tempRefs          []ast.Ref // Temporary variables used for lowering
	target            LanguageTarget

	isBundling              bool
	indirectImportItems     map[ast.Ref]bool
	importItemsForNamespace map[ast.Ref]map[string]ast.Ref
	exprForImportItem       map[ast.Ref]*ast.ENamespaceImport
	exportAliases           map[string]bool
}

type dotDefine struct {
	parts []string
	value ast.E
}

func (b *binder) newSymbol(kind ast.SymbolKind, name string) ast.Ref {
	ref := ast.Ref{b.source.Index, uint32(len(b.symbols))}
	b.symbols = append(b.symbols, ast.Symbol{kind, 0, name, ast.InvalidRef})
	return ref
}

func (b *binder) generateTempRef() ast.Ref {
	scope := b.scope
	for scope.Kind != ast.ScopeFunction && scope.Kind != ast.ScopeModule {
		scope = scope.Parent
	}
	ref := b.newSymbol(ast.SymbolHoisted, "_"+lexer.NumberToMinifiedName(len(b.tempRefs)))
	b.tempRefs = append(b.tempRefs, ref)
	scope.Generated = append(scope.Generated, ref)
	return ref
}

func (b *binder) pushScope(kind ast.ScopeKind) {
	parent := b.scope
	scope := &ast.Scope{
		Kind:     kind,
		Parent:   parent,
		Children: []*ast.Scope{},
		Members:  make(map[string]ast.Ref),
		ScopeRef: ast.InvalidRef,
	}
	if parent != nil {
		parent.Children = append(parent.Children, scope)
	}
	b.scope = scope
}

func (b *binder) popScope() {
	b.scope = b.scope.Parent
}

func (b *binder) loadNameFromRef(ref ast.Ref) string {
	if ref.OuterIndex == 0x80000000 {
		return b.allocatedNames[ref.InnerIndex]
	} else {
		return b.source.Contents[ref.InnerIndex : int32(ref.InnerIndex)-int32(ref.OuterIndex)]
	}
}

func (b *binder) findSymbol(name string) ast.Ref {
	s := b.scope
	for {
		ref, ok := s.Members[name]
		if ok {
			// Track how many times we've referenced this symbol
			if !b.isControlFlowDead {
				b.symbols[ref.InnerIndex].UseCountEstimate++
			}
			return ref
		}

		s = s.Parent
		if s == nil {
			// Allocate an "unbound" symbol
			ref = b.newSymbol(ast.SymbolUnbound, name)
			b.moduleScope.Members[name] = ref
			b.unbound = append(b.unbound, ref)

			// Track how many times we've referenced this symbol
			if !b.isControlFlowDead {
				b.symbols[ref.InnerIndex].UseCountEstimate++
			}
			return ref
		}
	}
}

func (b *binder) findLabelSymbol(loc ast.Loc, name string) ast.Ref {
	for s := b.scope; s != nil && s.Kind != ast.ScopeFunction; s = s.Parent {
		if s.Kind == ast.ScopeLabel && name == b.symbols[s.ScopeRef.InnerIndex].Name {
			// Track how many times we've referenced this symbol
			if !b.isControlFlowDead {
				b.symbols[s.ScopeRef.InnerIndex].UseCountEstimate++
			}
			return s.ScopeRef
		}
	}

	r := lexer.RangeOfIdentifier(b.source, loc)
	b.log.AddRangeError(b.source, r, fmt.Sprintf("There is no containing label named %q", name))

	// Allocate an "unbound" symbol
	ref := b.newSymbol(ast.SymbolUnbound, name)
	b.unbound = append(b.unbound, ref)

	// Track how many times we've referenced this symbol
	if !b.isControlFlowDead {
		b.symbols[ref.InnerIndex].UseCountEstimate++
	}
	return ref
}

func (b *binder) declareSymbol(kind ast.SymbolKind, loc ast.Loc, name string) ast.Ref {
	scope := b.scope

	// Check for collisions that would prevent to hoisting "var" symbols up to the enclosing function scope
	if kind == ast.SymbolHoisted {
		for scope.Kind != ast.ScopeFunction && scope.Kind != ast.ScopeModule {
			if existing, ok := scope.Members[name]; ok {
				symbol := b.symbols[existing.InnerIndex]
				switch symbol.Kind {
				case ast.SymbolUnbound, ast.SymbolHoisted:
					// Continue on to the parent scope
				case ast.SymbolCatchIdentifier:
					// This is a weird special case. Silently reuse this symbol.
					return existing
				default:
					r := lexer.RangeOfIdentifier(b.source, loc)
					b.log.AddRangeError(b.source, r, fmt.Sprintf("%q has already been declared", name))
					return existing
				}
			}
			scope = scope.Parent
		}
	}

	// Allocate a new symbol
	ref := b.newSymbol(kind, name)

	// Check for a collision in the declaring scope
	if existing, ok := scope.Members[name]; ok {
		symbol := b.symbols[existing.InnerIndex]
		if symbol.Kind == ast.SymbolUnbound {
			b.symbols[existing.InnerIndex].Link = ref
		} else if kind != ast.SymbolHoisted || symbol.Kind != ast.SymbolHoisted {
			r := lexer.RangeOfIdentifier(b.source, loc)
			b.log.AddRangeError(b.source, r, fmt.Sprintf("%q has already been declared", name))
			return existing
		} else {
			ref = existing
		}
	}

	// Hoist "var" symbols up to the enclosing function scope
	if kind == ast.SymbolHoisted {
		for s := b.scope; s.Kind != ast.ScopeFunction && s.Kind != ast.ScopeModule; s = s.Parent {
			if existing, ok := s.Members[name]; ok {
				symbol := b.symbols[existing.InnerIndex]
				if symbol.Kind == ast.SymbolUnbound {
					b.symbols[existing.InnerIndex].Link = ref
				}
			}
			s.Members[name] = ref
		}
	}

	// Overwrite this name in the declaring scope
	scope.Members[name] = ref
	return ref
}

func (b *binder) recordExport(loc ast.Loc, alias string) {
	if b.exportAliases[alias] {
		// Warn about duplicate exports
		b.log.AddRangeError(b.source, lexer.RangeOfIdentifier(b.source, loc),
			fmt.Sprintf("Multiple exports with the same name %q", alias))
	} else {
		b.exportAliases[alias] = true
	}
}

func findIdentifiers(binding ast.Binding, identifiers []ast.Decl) []ast.Decl {
	switch b := binding.Data.(type) {
	case *ast.BIdentifier:
		identifiers = append(identifiers, ast.Decl{Binding: binding})

	case *ast.BArray:
		for _, i := range b.Items {
			identifiers = findIdentifiers(i.Binding, identifiers)
		}

	case *ast.BObject:
		for _, i := range b.Properties {
			identifiers = findIdentifiers(i.Value, identifiers)
		}
	}

	return identifiers
}

// If this is in a dead branch, then we want to trim as much dead code as we
// can. Everything can be trimmed except for hoisted declarations ("var" and
// "function"), which affect the parent scope. For example:
//
//   function foo() {
//     if (false) { var x; }
//     x = 1;
//   }
//
// We can't trim the entire branch as dead or calling foo() will incorrectly
// assign to a global variable instead.
func shouldKeepStmtInDeadControlFlow(stmt ast.Stmt) bool {
	switch s := stmt.Data.(type) {
	case *ast.SEmpty, *ast.SExpr, *ast.SConst, *ast.SLet, *ast.SThrow,
		*ast.SReturn, *ast.SBreak, *ast.SContinue, *ast.SClass, *ast.SDebugger:
		// Omit these statements entirely
		return false

	case *ast.SVar:
		// Omit everything except the identifiers
		identifiers := []ast.Decl{}
		for _, decl := range s.Decls {
			identifiers = findIdentifiers(decl.Binding, identifiers)
		}
		s.Decls = identifiers
		return true

	default:
		// Everything else must be kept
		return true
	}
}

func (b *binder) declareAndVisitFnOrModuleStmts(stmts []ast.Stmt) []ast.Stmt {
	oldTempRefs := b.tempRefs
	b.tempRefs = []ast.Ref{}

	stmts = b.declareAndVisitStmts(stmts)

	// Append the temporary variable to the end of the function or module
	if len(b.tempRefs) > 0 {
		decls := []ast.Decl{}
		for _, ref := range b.tempRefs {
			decls = append(decls, ast.Decl{ast.Binding{ast.Loc{}, &ast.BIdentifier{ref}}, nil})
		}
		stmts = append([]ast.Stmt{ast.Stmt{ast.Loc{}, &ast.SVar{Decls: decls}}}, stmts...)
	}

	b.tempRefs = oldTempRefs
	return stmts
}

func (b *binder) declareAndVisitStmts(stmts []ast.Stmt) []ast.Stmt {
	for _, stmt := range stmts {
		b.declareStmt(stmt)
	}

	// Visit all statements first
	visited := make([]ast.Stmt, 0, len(stmts))
	for _, stmt := range stmts {
		visited = b.visitAndAppendStmt(visited, stmt)
	}

	// Stop now if we're not mangling
	if !b.mangleSyntax {
		return visited
	}

	// If this is in a dead branch, trim as much dead code as we can
	if b.isControlFlowDead {
		end := 0
		for _, stmt := range visited {
			if !shouldKeepStmtInDeadControlFlow(stmt) {
				continue
			}

			// Merge adjacent var statements
			if s, ok := stmt.Data.(*ast.SVar); ok && end > 0 {
				prevStmt := visited[end-1]
				if prevS, ok := prevStmt.Data.(*ast.SVar); ok {
					prevS.Decls = append(prevS.Decls, s.Decls...)
					continue
				}
			}

			visited[end] = stmt
			end++
		}
		return visited[:end]
	}

	// Merge adjacent statements during mangling
	result := make([]ast.Stmt, 0, len(visited))
	for _, stmt := range visited {
		switch s := stmt.Data.(type) {
		case *ast.SEmpty:
			// Strip empty statements
			continue

		case *ast.SConst:
			// Merge adjacent const statements
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*ast.SConst); ok && s.IsExport == prevS.IsExport {
					prevS.Decls = append(prevS.Decls, s.Decls...)
					continue
				}
			}

		case *ast.SLet:
			// Merge adjacent let statements
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*ast.SLet); ok && s.IsExport == prevS.IsExport {
					prevS.Decls = append(prevS.Decls, s.Decls...)
					continue
				}
			}

		case *ast.SVar:
			// Merge adjacent var statements
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*ast.SVar); ok && s.IsExport == prevS.IsExport {
					prevS.Decls = append(prevS.Decls, s.Decls...)
					continue
				}
			}

		case *ast.SExpr:
			// Merge adjacent expression statements
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*ast.SExpr); ok {
					prevS.Value = ast.Expr{prevStmt.Loc, &ast.EBinary{
						ast.BinOpComma,
						prevS.Value,
						s.Value,
					}}
					continue
				}
			}

		case *ast.SIf:
			if isJumpStatement(s.Yes.Data) && s.No != nil {
				// "if (a) return b; else if (c) return d; else return e;" => "if (a) return b; if (c) return d; return e;"
				for {
					result = append(result, stmt)
					stmt = *s.No
					s.No = nil
					var ok bool
					s, ok = stmt.Data.(*ast.SIf)
					if !ok || !isJumpStatement(s.Yes.Data) || s.No == nil {
						break
					}
				}
				result = append(result, stmt)
				continue
			}

		case *ast.SReturn:
			// Merge return statements with the previous expression statement
			if len(result) > 0 && s.Value != nil {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*ast.SExpr); ok {
					result[len(result)-1] = ast.Stmt{prevStmt.Loc, &ast.SReturn{&ast.Expr{prevStmt.Loc, &ast.EBinary{
						ast.BinOpComma,
						prevS.Value,
						*s.Value,
					}}}}
					continue
				}
			}

		case *ast.SThrow:
			// Merge throw statements with the previous expression statement
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*ast.SExpr); ok {
					result[len(result)-1] = ast.Stmt{prevStmt.Loc, &ast.SThrow{ast.Expr{prevStmt.Loc, &ast.EBinary{
						ast.BinOpComma,
						prevS.Value,
						s.Value,
					}}}}
					continue
				}
			}

		case *ast.SFor:
			if len(result) > 0 {
				prevStmt := result[len(result)-1]
				if prevS, ok := prevStmt.Data.(*ast.SExpr); ok {
					// Insert the previous expression into the for loop initializer
					if s.Init == nil {
						result[len(result)-1] = stmt
						s.Init = &ast.Stmt{prevStmt.Loc, &ast.SExpr{prevS.Value}}
						continue
					} else if s2, ok := s.Init.Data.(*ast.SExpr); ok {
						result[len(result)-1] = stmt
						s.Init = &ast.Stmt{prevStmt.Loc, &ast.SExpr{ast.Expr{prevStmt.Loc, &ast.EBinary{
							ast.BinOpComma,
							prevS.Value,
							s2.Value,
						}}}}
						continue
					}
				} else {
					// Insert the previous variable declaration into the for loop
					// initializer if it's a "var" declaration, since the scope
					// doesn't matter due to scope hoisting
					if s.Init == nil {
						if s2, ok := prevStmt.Data.(*ast.SVar); ok && !s2.IsExport {
							result[len(result)-1] = stmt
							s.Init = &prevStmt
							continue
						}
					} else {
						if s2, ok := prevStmt.Data.(*ast.SVar); ok && !s2.IsExport {
							if s3, ok := s.Init.Data.(*ast.SVar); ok {
								result[len(result)-1] = stmt
								s.Init.Data = &ast.SVar{Decls: append(s2.Decls, s3.Decls...)}
								continue
							}
						}
					}
				}
			}
		}

		result = append(result, stmt)
	}

	// Merge certain statements in reverse order
	if len(result) >= 2 {
		lastStmt := result[len(result)-1]

		if lastReturn, ok := lastStmt.Data.(*ast.SReturn); ok {
			// "if (a) return b; if (c) return d; return e;" => "return a ? b : c ? d : e;"
		returnLoop:
			for len(result) >= 2 {
				prevIndex := len(result) - 2
				prevStmt := result[prevIndex]

				switch prevS := prevStmt.Data.(type) {
				case *ast.SExpr:
					// This return statement must have a value
					if lastReturn.Value == nil {
						break returnLoop
					}

					// "a(); return b;" => "return a(), b;"
					lastReturn = &ast.SReturn{&ast.Expr{prevStmt.Loc, &ast.EBinary{ast.BinOpComma, prevS.Value, *lastReturn.Value}}}

					// Merge the last two statements
					lastStmt = ast.Stmt{prevStmt.Loc, lastReturn}
					result[prevIndex] = lastStmt
					result = result[:len(result)-1]

				case *ast.SIf:
					// The previous statement must be an if statement with no else clause
					if prevS.No != nil {
						break returnLoop
					}

					// The then clause must be a return
					prevReturn, ok := prevS.Yes.Data.(*ast.SReturn)
					if !ok {
						break returnLoop
					}

					// Handle some or all of the values being undefined
					left := prevReturn.Value
					right := lastReturn.Value
					if left == nil {
						// "if (a) return; return b;" => "return a ? void 0 : b;"
						left = &ast.Expr{prevS.Yes.Loc, &ast.EUndefined{}}
					}
					if right == nil {
						// "if (a) return a; return;" => "return a ? b : void 0;"
						right = &ast.Expr{lastStmt.Loc, &ast.EUndefined{}}
					}

					// "if (!a) return b; return c;" => "return a ? c : b;"
					if not, ok := prevS.Test.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
						prevS.Test = not.Value
						left, right = right, left
					}

					// Handle the returned values being the same
					if boolean, ok := checkEqualityIfNoSideEffects(left.Data, right.Data); ok && boolean {
						// "if (a) return b; return b;" => "return a, b;"
						lastReturn = &ast.SReturn{&ast.Expr{prevS.Test.Loc, &ast.EBinary{ast.BinOpComma, prevS.Test, *left}}}
					} else {
						// "if (a) return b; return c;" => "return a ? b : c;"
						lastReturn = &ast.SReturn{&ast.Expr{prevS.Test.Loc, &ast.EIf{prevS.Test, *left, *right}}}
					}

					// Merge the last two statements
					lastStmt = ast.Stmt{prevStmt.Loc, lastReturn}
					result[prevIndex] = lastStmt
					result = result[:len(result)-1]

				default:
					break returnLoop
				}
			}
		} else if lastThrow, ok := lastStmt.Data.(*ast.SThrow); ok {
			// "if (a) throw b; if (c) throw d; throw e;" => "throw a ? b : c ? d : e;"
		throwLoop:
			for len(result) >= 2 {
				prevIndex := len(result) - 2
				prevStmt := result[prevIndex]

				switch prevS := prevStmt.Data.(type) {
				case *ast.SExpr:
					// "a(); throw b;" => "throw a(), b;"
					lastThrow = &ast.SThrow{ast.Expr{prevStmt.Loc, &ast.EBinary{ast.BinOpComma, prevS.Value, lastThrow.Value}}}

					// Merge the last two statements
					lastStmt = ast.Stmt{prevStmt.Loc, lastThrow}
					result[prevIndex] = lastStmt
					result = result[:len(result)-1]

				case *ast.SIf:
					// The previous statement must be an if statement with no else clause
					if prevS.No != nil {
						break throwLoop
					}

					// The then clause must be a throw
					prevThrow, ok := prevS.Yes.Data.(*ast.SThrow)
					if !ok {
						break throwLoop
					}

					left := prevThrow.Value
					right := lastThrow.Value

					// "if (!a) throw b; throw c;" => "throw a ? c : b;"
					if not, ok := prevS.Test.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
						prevS.Test = not.Value
						left, right = right, left
					}

					// Merge the last two statements
					lastThrow = &ast.SThrow{ast.Expr{prevS.Test.Loc, &ast.EIf{prevS.Test, left, right}}}
					lastStmt = ast.Stmt{prevStmt.Loc, lastThrow}
					result[prevIndex] = lastStmt
					result = result[:len(result)-1]

				default:
					break throwLoop
				}
			}
		}
	}

	return result
}

func (b *binder) declareAndVisitStmt(stmt ast.Stmt) ast.Stmt {
	stmts := b.declareAndVisitStmts([]ast.Stmt{stmt})

	// This statement could potentially expand to several statements
	switch len(stmts) {
	case 0:
		return ast.Stmt{stmt.Loc, &ast.SEmpty{}}
	case 1:
		return stmts[0]
	default:
		return ast.Stmt{stmt.Loc, &ast.SBlock{stmts}}
	}
}

// Lower class fields for environments that don't support them
func (b *binder) lowerClass(class *ast.Class, isStmt bool) (extraExprs []ast.Expr, tempRef ast.Ref) {
	tempRef = ast.InvalidRef
	if b.target == ESNext {
		return
	}

	var ref ast.Ref
	if class.Name != nil && isStmt {
		ref = class.Name.Ref
	} else {
		tempRef = b.generateTempRef()
		ref = tempRef
	}

	props := class.Properties
	end := 0

	for _, prop := range props {
		if prop.IsStatic {
			// Generate the assignment target
			var target ast.Expr
			if key, ok := prop.Key.Data.(*ast.EString); ok && !prop.IsComputed {
				target = ast.Expr{prop.Key.Loc, &ast.EDot{
					Target:  ast.Expr{prop.Key.Loc, &ast.EIdentifier{ref}},
					Name:    lexer.UTF16ToString(key.Value),
					NameLoc: prop.Key.Loc,
				}}
			} else {
				target = ast.Expr{prop.Key.Loc, &ast.EIndex{
					Target: ast.Expr{prop.Key.Loc, &ast.EIdentifier{ref}},
					Index:  prop.Key,
				}}
			}

			// Generate the assignment initializer
			var init ast.Expr
			if prop.Initializer != nil {
				init = *prop.Initializer
			} else if prop.Value != nil {
				init = *prop.Value
			} else {
				init = ast.Expr{prop.Key.Loc, &ast.EUndefined{}}
			}

			// Move this property to an assignment after the class ends
			extraExprs = append(extraExprs, ast.Expr{prop.Key.Loc, &ast.EBinary{
				ast.BinOpAssign,
				target,
				init,
			}})
			continue
		}

		// Keep this property
		props[end] = prop
		end++
	}

	class.Properties = props[:end]
	return
}

func (b *binder) declareBinding(kind ast.SymbolKind, binding ast.Binding, isExport bool) {
	switch d := binding.Data.(type) {
	case *ast.BMissing:

	case *ast.BIdentifier:
		name := b.loadNameFromRef(d.Ref)
		d.Ref = b.declareSymbol(kind, binding.Loc, name)
		if isExport {
			b.recordExport(binding.Loc, name)
		}

	case *ast.BArray:
		for _, i := range d.Items {
			b.declareBinding(kind, i.Binding, isExport)
		}

	case *ast.BObject:
		for _, p := range d.Properties {
			b.declareBinding(kind, p.Value, isExport)
		}

	default:
		panic(fmt.Sprintf("Unexpected binding of type %T", binding.Data))
	}
}

func (b *binder) visitBinding(binding ast.Binding) {
	switch d := binding.Data.(type) {
	case *ast.BMissing, *ast.BIdentifier:

	case *ast.BArray:
		for _, i := range d.Items {
			b.visitBinding(i.Binding)
			if i.DefaultValue != nil {
				*i.DefaultValue = b.visitExpr(*i.DefaultValue)
			}
		}

	case *ast.BObject:
		for i, p := range d.Properties {
			if !p.IsSpread {
				p.Key = b.visitExpr(p.Key)
			}
			b.visitBinding(p.Value)
			if p.DefaultValue != nil {
				*p.DefaultValue = b.visitExpr(*p.DefaultValue)
			}
			d.Properties[i] = p
		}

	default:
		panic(fmt.Sprintf("Unexpected binding of type %T", binding.Data))
	}
}

// Declare anything up front that could potentially be bound by sibling statements
func (b *binder) declareStmt(stmt ast.Stmt) {
	switch s := stmt.Data.(type) {
	case *ast.SConst:
		for _, d := range s.Decls {
			b.declareBinding(ast.SymbolOther, d.Binding, s.IsExport)
		}

	case *ast.SLet:
		for _, d := range s.Decls {
			b.declareBinding(ast.SymbolOther, d.Binding, s.IsExport)
		}

	case *ast.SVar:
		for _, d := range s.Decls {
			b.declareBinding(ast.SymbolHoisted, d.Binding, s.IsExport)
		}

	case *ast.SFunction:
		if s.Fn.Name != nil {
			name := b.loadNameFromRef(s.Fn.Name.Ref)
			s.Fn.Name.Ref = b.declareSymbol(ast.SymbolHoisted, s.Fn.Name.Loc, name)
			if s.IsExport {
				b.recordExport(s.Fn.Name.Loc, name)
			}
		}

	case *ast.SClass:
		if s.Class.Name != nil {
			name := b.loadNameFromRef(s.Class.Name.Ref)
			s.Class.Name.Ref = b.declareSymbol(ast.SymbolOther, s.Class.Name.Loc, name)
			if s.IsExport {
				b.recordExport(s.Class.Name.Loc, name)
			}
		}

	case *ast.SImport:
		if s.StarLoc != nil {
			name := b.loadNameFromRef(s.NamespaceRef)
			s.NamespaceRef = b.declareSymbol(ast.SymbolOther, *s.StarLoc, name)
		} else {
			// Generate a symbol for the namespace
			name := ast.GenerateNonUniqueNameFromPath(s.Path.Text)
			s.NamespaceRef = b.newSymbol(ast.SymbolOther, name)
			b.scope.Generated = append(b.scope.Generated, s.NamespaceRef)
		}
		itemRefs := make(map[string]ast.Ref)

		// Link the default item to the namespace
		if s.DefaultName != nil {
			name := b.loadNameFromRef(s.DefaultName.Ref)
			ref := b.declareSymbol(ast.SymbolOther, s.DefaultName.Loc, name)
			b.exprForImportItem[ref] = &ast.ENamespaceImport{
				NamespaceRef: s.NamespaceRef,
				ItemRef:      ref,
				Alias:        "default",
			}
			s.DefaultName.Ref = ref
			itemRefs["default"] = ref
		}

		// Link each import item to the namespace
		if s.Items != nil {
			for i, item := range *s.Items {
				name := b.loadNameFromRef(item.Name.Ref)
				ref := b.declareSymbol(ast.SymbolOther, item.Name.Loc, name)
				b.exprForImportItem[ref] = &ast.ENamespaceImport{
					NamespaceRef: s.NamespaceRef,
					ItemRef:      ref,
					Alias:        item.Alias,
				}
				(*s.Items)[i].Name.Ref = ref
				itemRefs[item.Alias] = ref
			}
		}

		// Track the items for this namespace
		b.importItemsForNamespace[s.NamespaceRef] = itemRefs

	case *ast.SExportDefault:
		if s.Value.Stmt != nil {
			b.declareStmt(*s.Value.Stmt)
		}
		name := b.loadNameFromRef(s.DefaultName.Ref)
		ref := b.newSymbol(ast.SymbolOther, name)
		s.DefaultName.Ref = ref
		b.scope.Generated = append(b.scope.Generated, ref)
		b.recordExport(s.DefaultName.Loc, name)
	}
}

func statementCaresAboutScope(stmt ast.Stmt) bool {
	switch stmt.Data.(type) {
	case *ast.SBlock, *ast.SEmpty, *ast.SDebugger, *ast.SExpr, *ast.SIf,
		*ast.SFor, *ast.SForIn, *ast.SForOf, *ast.SDoWhile, *ast.SWhile,
		*ast.SWith, *ast.STry, *ast.SSwitch, *ast.SReturn, *ast.SThrow,
		*ast.SVar, *ast.SBreak, *ast.SContinue:
		return false

	default:
		return true
	}
}

func mangleIf(loc ast.Loc, s *ast.SIf, isTestBooleanConstant bool, testBooleanValue bool) ast.Stmt {
	// Constant folding using the test expression
	if isTestBooleanConstant {
		if testBooleanValue {
			// The test is true
			if s.No == nil || !shouldKeepStmtInDeadControlFlow(*s.No) {
				// We can drop the "no" branch
				if statementCaresAboutScope(s.Yes) {
					return ast.Stmt{s.Yes.Loc, &ast.SBlock{[]ast.Stmt{s.Yes}}}
				} else {
					return s.Yes
				}
			} else {
				// We have to keep the "no" branch
			}
		} else {
			// The test is false
			if !shouldKeepStmtInDeadControlFlow(s.Yes) {
				// We can drop the "yes" branch
				if s.No == nil {
					return ast.Stmt{loc, &ast.SEmpty{}}
				} else if statementCaresAboutScope(*s.No) {
					return ast.Stmt{s.No.Loc, &ast.SBlock{[]ast.Stmt{*s.No}}}
				} else {
					return *s.No
				}
			} else {
				// We have to keep the "yes" branch
			}
		}
	}

	if yes, ok := s.Yes.Data.(*ast.SExpr); ok {
		// "yes" is an expression
		if s.No == nil {
			if not, ok := s.Test.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
				// "if (!a) b();" => "a || b();"
				return ast.Stmt{loc, &ast.SExpr{ast.Expr{loc, &ast.EBinary{
					ast.BinOpLogicalOr,
					not.Value,
					yes.Value,
				}}}}
			} else {
				// "if (a) b();" => "a && b();"
				return ast.Stmt{loc, &ast.SExpr{ast.Expr{loc, &ast.EBinary{
					ast.BinOpLogicalAnd,
					s.Test,
					yes.Value,
				}}}}
			}
		} else if no, ok := s.No.Data.(*ast.SExpr); ok {
			if not, ok := s.Test.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
				// "if (!a) b(); else c();" => "a ? c() : b();"
				return ast.Stmt{loc, &ast.SExpr{ast.Expr{loc, &ast.EIf{
					not.Value,
					no.Value,
					yes.Value,
				}}}}
			} else {
				// "if (a) b(); else c();" => "a ? b() : c();"
				return ast.Stmt{loc, &ast.SExpr{ast.Expr{loc, &ast.EIf{
					s.Test,
					yes.Value,
					no.Value,
				}}}}
			}
		}
	} else if _, ok := s.Yes.Data.(*ast.SEmpty); ok {
		// "yes" is missing
		if s.No == nil {
			// "yes" and "no" are both missing
			if hasNoSideEffects(s.Test.Data) {
				// "if (1) {}" => ";"
				return ast.Stmt{loc, &ast.SEmpty{}}
			} else {
				// "if (a) {}" => "a;"
				return ast.Stmt{loc, &ast.SExpr{s.Test}}
			}
		} else if no, ok := s.No.Data.(*ast.SExpr); ok {
			if not, ok := s.Test.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
				// "if (!a) {} else b();" => "a && b();"
				return ast.Stmt{loc, &ast.SExpr{ast.Expr{loc, &ast.EBinary{
					ast.BinOpLogicalAnd,
					not.Value,
					no.Value,
				}}}}
			} else {
				// "if (a) {} else b();" => "a || b();"
				return ast.Stmt{loc, &ast.SExpr{ast.Expr{loc, &ast.EBinary{
					ast.BinOpLogicalOr,
					s.Test,
					no.Value,
				}}}}
			}
		} else {
			// "yes" is missing and "no" is not missing (and is not an expression)
			if not, ok := s.Test.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
				// "if (!a) {} else throw b;" => "if (a) throw b;"
				s.Test = not.Value
				s.Yes = *s.No
				s.No = nil
			} else {
				// "if (a) {} else throw b;" => "if (!a) throw b;"
				s.Test = ast.Expr{s.Test.Loc, &ast.EUnary{ast.UnOpNot, s.Test}}
				s.Yes = *s.No
				s.No = nil
			}
		}
	} else {
		// "yes" is not missing (and is not an expression)
		if s.No != nil {
			// "yes" is not missing (and is not an expression) and "no" is not missing
			if not, ok := s.Test.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
				// "if (!a) return b; else return c;" => "if (a) return c; else return b;"
				s.Test = not.Value
				s.Yes, *s.No = *s.No, s.Yes
			}
		}
	}

	return ast.Stmt{loc, s}
}

func (b *binder) visitAndAppendStmt(stmts []ast.Stmt, stmt ast.Stmt) []ast.Stmt {
	switch s := stmt.Data.(type) {
	case *ast.SDebugger, *ast.SEmpty:
		// These don't contain anything to traverse

	case *ast.SImport:
		// This was already handled in "declareStmt"

	case *ast.SExportClause:
		for i, item := range s.Items {
			name := b.loadNameFromRef(item.Name.Ref)
			s.Items[i].Name.Ref = b.findSymbol(name)
			b.recordExport(item.AliasLoc, item.Alias)
		}

	case *ast.SExportFrom:
		// Generate a symbol for the namespace
		name := ast.GenerateNonUniqueNameFromPath(s.Path.Text)
		namespaceRef := b.newSymbol(ast.SymbolOther, name)
		b.scope.Generated = append(b.scope.Generated, namespaceRef)
		s.NamespaceRef = namespaceRef

		// Path: this is a re-export and the names are symbols in another file
		for i, item := range s.Items {
			name := b.loadNameFromRef(item.Name.Ref)
			s.Items[i].Name.Ref = b.newSymbol(ast.SymbolUnbound, name)
			b.unbound = append(b.unbound, item.Name.Ref)
			b.recordExport(item.AliasLoc, item.Alias)
		}

	case *ast.SExportStar:
		if s.Item != nil {
			// "export * as ns from 'path'"
			name := b.loadNameFromRef(s.Item.Name.Ref)
			ref := b.newSymbol(ast.SymbolOther, name)

			// This name isn't ever declared in this scope, so code in this module
			// must not be able to get to it. Still, we need to associate it with
			// the scope somehow so it will be minified.
			b.scope.Generated = append(b.scope.Generated, ref)
			s.Item.Name.Ref = ref
			b.recordExport(s.Item.AliasLoc, s.Item.Alias)
		}

	case *ast.SExportDefault:
		switch {
		case s.Value.Expr != nil:
			*s.Value.Expr = b.visitExpr(*s.Value.Expr)

		case s.Value.Stmt != nil:
			switch s2 := s.Value.Stmt.Data.(type) {
			case *ast.SFunction:
				b.visitFn(&s2.Fn)

			case *ast.SClass:
				b.visitClass(&s2.Class)
				stmts = append(stmts, stmt)

				// Lower class field syntax for browsers that don't support it
				extraExprs, _ := b.lowerClass(&s2.Class, true /* isStmt */)
				for _, expr := range extraExprs {
					stmts = append(stmts, ast.Stmt{expr.Loc, &ast.SExpr{expr}})
				}
				return stmts

			default:
				panic("Internal error")
			}
		}

	case *ast.SBreak:
		if s.Name != nil {
			name := b.loadNameFromRef(s.Name.Ref)
			s.Name.Ref = b.findLabelSymbol(s.Name.Loc, name)
		}

	case *ast.SContinue:
		if s.Name != nil {
			name := b.loadNameFromRef(s.Name.Ref)
			s.Name.Ref = b.findLabelSymbol(s.Name.Loc, name)
		}

	case *ast.SLabel:
		b.pushScope(ast.ScopeLabel)
		name := b.loadNameFromRef(s.Name.Ref)
		ref := b.newSymbol(ast.SymbolOther, name)
		s.Name.Ref = ref
		b.scope.ScopeRef = ref
		s.Stmt = b.declareAndVisitStmt(s.Stmt)
		b.popScope()

	case *ast.SConst:
		for _, d := range s.Decls {
			b.visitBinding(d.Binding)
			if d.Value != nil {
				*d.Value = b.visitExpr(*d.Value)
			}
		}

	case *ast.SLet:
		for i, d := range s.Decls {
			b.visitBinding(d.Binding)
			if d.Value != nil {
				*d.Value = b.visitExpr(*d.Value)

				// Initializing to undefined is implicit
				if b.mangleSyntax {
					if _, ok := d.Value.Data.(*ast.EUndefined); ok {
						s.Decls[i].Value = nil
					}
				}
			}
		}

	case *ast.SVar:
		for i, d := range s.Decls {
			b.visitBinding(d.Binding)
			if d.Value != nil {
				*d.Value = b.visitExpr(*d.Value)

				// Initializing to undefined is implicit
				if b.mangleSyntax {
					if _, ok := d.Value.Data.(*ast.EUndefined); ok {
						s.Decls[i].Value = nil
					}
				}
			}
		}

	case *ast.SExpr:
		s.Value = b.visitExpr(s.Value)

		// Trim expressions without side effects
		if b.mangleSyntax && hasNoSideEffects(s.Value.Data) {
			stmt = ast.Stmt{stmt.Loc, &ast.SEmpty{}}
		}

	case *ast.SThrow:
		s.Value = b.visitExpr(s.Value)

	case *ast.SReturn:
		if s.Value != nil {
			*s.Value = b.visitExpr(*s.Value)

			// Returning undefined is implicit
			if b.mangleSyntax {
				if _, ok := s.Value.Data.(*ast.EUndefined); ok {
					s.Value = nil
				}
			}
		}

	case *ast.SBlock:
		b.pushScope(ast.ScopeBlock)
		s.Stmts = b.declareAndVisitStmts(s.Stmts)
		b.popScope()

		if b.mangleSyntax {
			if len(s.Stmts) == 1 {
				// Unwrap blocks containing a single statement
				stmt = s.Stmts[0]
			} else if len(s.Stmts) == 0 {
				// Trim empty blocks
				stmt = ast.Stmt{stmt.Loc, &ast.SEmpty{}}
			}
		}

	case *ast.SWith:
		s.Value = b.visitExpr(s.Value)
		s.Body = b.declareAndVisitStmt(s.Body)

	case *ast.SWhile:
		s.Test = b.visitBooleanExpr(s.Test)
		s.Body = b.declareAndVisitStmt(s.Body)

		if b.mangleSyntax {
			// "while (a) {}" => "for (;a;) {}"
			test := &s.Test
			if boolean, ok := toBooleanWithoutSideEffects(s.Test.Data); ok && boolean {
				test = nil
			}
			stmt = ast.Stmt{stmt.Loc, &ast.SFor{Test: test, Body: s.Body}}
		}

	case *ast.SDoWhile:
		s.Body = b.declareAndVisitStmt(s.Body)
		s.Test = b.visitBooleanExpr(s.Test)

	case *ast.SIf:
		s.Test = b.visitBooleanExpr(s.Test)

		// Fold constants
		boolean, ok := toBooleanWithoutSideEffects(s.Test.Data)

		// Mark the control flow as dead if the branch is never taken
		if ok && !boolean {
			old := b.isControlFlowDead
			b.isControlFlowDead = true
			s.Yes = b.declareAndVisitStmt(s.Yes)
			b.isControlFlowDead = old
		} else {
			s.Yes = b.declareAndVisitStmt(s.Yes)
		}

		// The "else" clause is optional
		if s.No != nil {
			// Mark the control flow as dead if the branch is never taken
			if ok && boolean {
				old := b.isControlFlowDead
				b.isControlFlowDead = true
				*s.No = b.declareAndVisitStmt(*s.No)
				b.isControlFlowDead = old
			} else {
				*s.No = b.declareAndVisitStmt(*s.No)
			}

			// Trim unnecessary "else" clauses
			if b.mangleSyntax {
				if _, ok := s.No.Data.(*ast.SEmpty); ok {
					s.No = nil
				}
			}
		}

		if b.mangleSyntax {
			stmt = mangleIf(stmt.Loc, s, ok, boolean)
		}

	case *ast.SFor:
		b.pushScope(ast.ScopeBlock)
		if s.Init != nil {
			b.declareAndVisitStmt(*s.Init)
		}

		if s.Test != nil {
			*s.Test = b.visitBooleanExpr(*s.Test)

			// A true value is implied
			if b.mangleSyntax {
				if boolean, ok := toBooleanWithoutSideEffects(s.Test.Data); ok && boolean {
					s.Test = nil
				}
			}
		}

		if s.Update != nil {
			*s.Update = b.visitExpr(*s.Update)
		}
		s.Body = b.declareAndVisitStmt(s.Body)
		b.popScope()

	case *ast.SForIn:
		b.pushScope(ast.ScopeBlock)
		b.declareAndVisitStmt(s.Init)
		s.Value = b.visitExpr(s.Value)
		s.Body = b.declareAndVisitStmt(s.Body)
		b.popScope()

	case *ast.SForOf:
		b.pushScope(ast.ScopeBlock)
		b.declareAndVisitStmt(s.Init)
		s.Value = b.visitExpr(s.Value)
		s.Body = b.declareAndVisitStmt(s.Body)
		b.popScope()

	case *ast.STry:
		b.pushScope(ast.ScopeBlock)
		b.tryBodyCount++
		s.Body = b.declareAndVisitStmts(s.Body)
		b.tryBodyCount--
		b.popScope()

		if s.Catch != nil {
			b.pushScope(ast.ScopeBlock)
			if s.Catch.Binding != nil {
				kind := ast.SymbolOther
				if _, ok := s.Catch.Binding.Data.(*ast.BIdentifier); ok {
					kind = ast.SymbolCatchIdentifier
				}
				b.declareBinding(kind, *s.Catch.Binding, false /* isExport */)
				b.visitBinding(*s.Catch.Binding)
			}
			s.Catch.Body = b.declareAndVisitStmts(s.Catch.Body)
			b.popScope()
		}

		if s.Finally != nil {
			b.pushScope(ast.ScopeBlock)
			*s.Finally = b.declareAndVisitStmts(*s.Finally)
			b.popScope()
		}

	case *ast.SSwitch:
		s.Test = b.visitExpr(s.Test)
		b.pushScope(ast.ScopeBlock)
		for i, c := range s.Cases {
			if c.Value != nil {
				*c.Value = b.visitExpr(*c.Value)
			}
			c.Body = b.declareAndVisitStmts(c.Body)

			// Make sure the assignment to the body above is preserved
			s.Cases[i] = c
		}
		b.popScope()

	case *ast.SFunction:
		b.visitFn(&s.Fn)

	case *ast.SClass:
		b.visitClass(&s.Class)
		stmts = append(stmts, stmt)

		// Lower class field syntax for browsers that don't support it
		extraExprs, _ := b.lowerClass(&s.Class, true /* isStmt */)
		for _, expr := range extraExprs {
			stmts = append(stmts, ast.Stmt{expr.Loc, &ast.SExpr{expr}})
		}
		return stmts

	default:
		panic(fmt.Sprintf("Unexpected statement of type %T", stmt.Data))
	}

	stmts = append(stmts, stmt)
	return stmts
}

func (b *binder) visitClass(class *ast.Class) {
	if class.Extends != nil {
		*class.Extends = b.visitExpr(*class.Extends)
	}
	for i, property := range class.Properties {
		class.Properties[i].Key = b.visitExpr(property.Key)
		if property.Value != nil {
			*property.Value = b.visitExpr(*property.Value)
		}
		if property.Initializer != nil {
			*property.Initializer = b.visitExpr(*property.Initializer)
		}
	}
}

func (b *binder) visitArgs(args []ast.Arg) {
	for _, arg := range args {
		b.declareBinding(ast.SymbolHoisted, arg.Binding, false /* isExport */)
		b.visitBinding(arg.Binding)
		if arg.Default != nil {
			*arg.Default = b.visitExpr(*arg.Default)
		}
	}
}

func (b *binder) isDotDefineMatch(expr ast.Expr, parts []string) bool {
	if len(parts) > 1 {
		// Intermediates must be dot expressions
		e, ok := expr.Data.(*ast.EDot)
		last := len(parts) - 1
		return ok && parts[last] == e.Name && b.isDotDefineMatch(e.Target, parts[:last])
	}

	// The last expression must be an identifier
	e, ok := expr.Data.(*ast.EIdentifier)
	if !ok {
		return false
	}

	// The name must match
	name := b.loadNameFromRef(e.Ref)
	if name != parts[0] {
		return false
	}

	// The last symbol must be unbound
	ref := b.findSymbol(name)
	return b.symbols[ref.InnerIndex].Kind == ast.SymbolUnbound
}

func (b *binder) stringsToMemberExpression(loc ast.Loc, parts []string) ast.Expr {
	ref := b.findSymbol(parts[0])
	value := ast.Expr{loc, &ast.EIdentifier{ref}}

	// Substitute an EImportNamespace now if this is an import item
	if importData, ok := b.exprForImportItem[ref]; ok {
		value.Data = importData
	}

	for i := 1; i < len(parts); i++ {
		value = b.maybeRewriteDot(loc, &ast.EDot{value, parts[i], loc, false})
	}
	return value
}

func (b *binder) warnAboutEqualityCheck(op string, value ast.Expr, afterOpLoc ast.Loc) bool {
	switch e := value.Data.(type) {
	case *ast.ENumber:
		if e.Value == 0 && math.Signbit(e.Value) {
			b.log.AddWarning(b.source, value.Loc,
				fmt.Sprintf("Comparison with -0 using the %s operator will also match 0", op))
			return true
		}

	case *ast.EArray, *ast.EArrow, *ast.EClass,
		*ast.EFunction, *ast.EObject, *ast.ERegExp:
		index := strings.LastIndex(b.source.Contents[:afterOpLoc.Start], op)
		b.log.AddRangeWarning(b.source, ast.Range{ast.Loc{int32(index)}, int32(len(op))},
			fmt.Sprintf("Comparison using the %s operator here is always %v", op, op[0] == '!'))
		return true
	}

	return false
}

// EDot nodes represent a property access. This function may return an
// expression to replace the property access with. It assumes that the
// target of the EDot expression has already been visited.
func (b *binder) maybeRewriteDot(loc ast.Loc, data *ast.EDot) ast.Expr {
	// Rewrite property accesses on explicit namespace imports as an identifier.
	// This lets us replace them easily in the printer to rebind them to
	// something else without paying the cost of a whole-tree traversal during
	// module linking just to rewrite these EDot expressions.
	if id, ok := data.Target.Data.(*ast.EIdentifier); ok {
		if importItems, ok := b.importItemsForNamespace[id.Ref]; ok {
			var itemRef ast.Ref
			var importData *ast.ENamespaceImport

			// Cache translation so each property access resolves to the same import
			itemRef, ok := importItems[data.Name]
			if ok {
				importData = b.exprForImportItem[itemRef]
			} else {
				// Generate a new import item symbol in the module scope
				itemRef = b.newSymbol(ast.SymbolOther, data.Name)
				b.moduleScope.Generated = append(b.moduleScope.Generated, itemRef)

				// Link the namespace import and the import item together
				importItems[data.Name] = itemRef
				importData = &ast.ENamespaceImport{
					NamespaceRef: id.Ref,
					ItemRef:      itemRef,
					Alias:        data.Name,
				}
				b.exprForImportItem[itemRef] = importData

				// Make sure the printer prints this as a property access
				b.indirectImportItems[itemRef] = true
			}

			// Move the use count from the namespace import over to the generated
			// import item. This lets us easily tell if a namespace import is ever
			// captured and used directly or not. If it's not, then we can omit it
			// from the generated code entirely. This is worth doing because the
			// generated code for a namespace import is pretty big (it creates an
			// object with all exports as properties).
			if !b.isControlFlowDead {
				b.symbols[id.Ref.InnerIndex].UseCountEstimate--
				b.symbols[itemRef.InnerIndex].UseCountEstimate++
			}

			return ast.Expr{loc, importData}
		}
	}

	return ast.Expr{loc, data}
}

func joinStrings(a []uint16, b []uint16) []uint16 {
	data := make([]uint16, len(a)+len(b))
	copy(data[:len(a)], a)
	copy(data[len(a):], b)
	return data
}

func foldStringAddition(left ast.Expr, right ast.Expr) *ast.Expr {
	switch l := left.Data.(type) {
	case *ast.EString:
		switch r := right.Data.(type) {
		case *ast.EString:
			return &ast.Expr{left.Loc, &ast.EString{joinStrings(l.Value, r.Value)}}

		case *ast.ETemplate:
			if r.Tag == nil {
				return &ast.Expr{left.Loc, &ast.ETemplate{nil, joinStrings(l.Value, r.Head), "", r.Parts}}
			}
		}

	case *ast.ETemplate:
		if l.Tag == nil {
			switch r := right.Data.(type) {
			case *ast.EString:
				n := len(l.Parts)
				head := l.Head
				parts := make([]ast.TemplatePart, n)
				if n == 0 {
					head = joinStrings(head, r.Value)
				} else {
					copy(parts, l.Parts)
					parts[n-1].Tail = joinStrings(parts[n-1].Tail, r.Value)
				}
				return &ast.Expr{left.Loc, &ast.ETemplate{nil, head, "", parts}}

			case *ast.ETemplate:
				if r.Tag == nil {
					n := len(l.Parts)
					head := l.Head
					parts := make([]ast.TemplatePart, n+len(r.Parts))
					copy(parts[n:], r.Parts)
					if n == 0 {
						head = joinStrings(head, r.Head)
					} else {
						copy(parts[:n], l.Parts)
						parts[n-1].Tail = joinStrings(parts[n-1].Tail, r.Head)
					}
					return &ast.Expr{left.Loc, &ast.ETemplate{nil, head, "", parts}}
				}
			}
		}
	}

	return nil
}

func (b *binder) visitBooleanExpr(expr ast.Expr) ast.Expr {
	expr = b.visitExpr(expr)

	// Simplify syntax when we know it's used inside a boolean context
	if b.mangleSyntax {
		for {
			// "!!a" => "a"
			if not, ok := expr.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
				if not2, ok2 := not.Value.Data.(*ast.EUnary); ok2 && not2.Op == ast.UnOpNot {
					expr = not2.Value
					continue
				}
			}

			break
		}
	}

	return expr
}

func (b *binder) visitExpr(expr ast.Expr) ast.Expr {
	switch e := expr.Data.(type) {
	case *ast.EMissing, *ast.ENull, *ast.ESuper, *ast.EString,
		*ast.EBoolean, *ast.ENumber, *ast.EBigInt, *ast.EThis,
		*ast.ERegExp, *ast.ENewTarget, *ast.EUndefined:

	case *ast.EImportMeta:
		if b.isBundling {
			// Replace "import.meta" with a dummy object when bundling
			return ast.Expr{expr.Loc, &ast.EObject{}}
		}

	case *ast.ESpread:
		e.Value = b.visitExpr(e.Value)

	case *ast.EIdentifier:
		name := b.loadNameFromRef(e.Ref)
		e.Ref = b.findSymbol(name)

		// Substitute an EImportNamespace now if this is an import item
		if importData, ok := b.exprForImportItem[e.Ref]; ok {
			return ast.Expr{expr.Loc, importData}
		}

		// Substitute user-specified defines for unbound symbols
		if b.symbols[e.Ref.InnerIndex].Kind == ast.SymbolUnbound {
			if value, ok := b.identifierDefines[name]; ok {
				return ast.Expr{expr.Loc, value}
			}
		}

		// Disallow capturing the "require" variable without calling it
		if e.Ref == b.requireRef && e != b.callTarget {
			if b.tryBodyCount == 0 {
				r := lexer.RangeOfIdentifier(b.source, expr.Loc)
				b.log.AddRangeError(b.source, r, "\"require\" must not be called indirectly")
			} else {
				// The "moment" library contains code that looks like this:
				//
				//   try {
				//     oldLocale = globalLocale._abbr;
				//     var aliasedRequire = require;
				//     aliasedRequire('./locale/' + name);
				//     getSetGlobalLocale(oldLocale);
				//   } catch (e) {}
				//
				// This is unfortunate because it prevents the module graph from being
				// statically determined. However, the dependencies are optional and
				// the library will work fine without them.
				//
				// Handle this case by deliberately ignoring code that uses require
				// incorrectly inside a try statement like this. We replace it with
				// null so it's guaranteed to crash at runtime.
				return ast.Expr{expr.Loc, &ast.ENull{}}
			}
		}

	case *ast.EJSXElement:
		// A missing tag is a fragment
		tag := e.Tag
		if tag == nil {
			value := b.stringsToMemberExpression(expr.Loc, b.jsx.Fragment)
			tag = &value
		} else {
			*tag = b.visitExpr(*tag)
		}

		// Visit properties
		for i, p := range e.Properties {
			if p.Kind != ast.PropertySpread {
				p.Key = b.visitExpr(p.Key)
			}
			if p.Value != nil {
				*p.Value = b.visitExpr(*p.Value)
			}
			if p.Initializer != nil {
				*p.Initializer = b.visitExpr(*p.Initializer)
			}
			e.Properties[i] = p
		}

		// Arguments to createElement()
		args := []ast.Expr{*tag}
		if len(e.Properties) > 0 {
			args = append(args, ast.Expr{expr.Loc, &ast.EObject{e.Properties}})
		} else {
			args = append(args, ast.Expr{expr.Loc, &ast.ENull{}})
		}
		if len(e.Children) > 0 {
			for _, child := range e.Children {
				args = append(args, b.visitExpr(child))
			}
		}

		// Call createElement()
		return ast.Expr{expr.Loc, &ast.ECall{b.stringsToMemberExpression(expr.Loc, b.jsx.Factory), args, false}}

	case *ast.ETemplate:
		if e.Tag != nil {
			*e.Tag = b.visitExpr(*e.Tag)
		}
		for i, part := range e.Parts {
			e.Parts[i].Value = b.visitExpr(part.Value)
		}

	case *ast.EBinary:
		e.Left = b.visitExpr(e.Left)
		e.Right = b.visitExpr(e.Right)

		// Fold constants
		switch e.Op {
		case ast.BinOpLooseEq:
			if result, ok := checkEqualityIfNoSideEffects(e.Left.Data, e.Right.Data); ok {
				return ast.Expr{expr.Loc, &ast.EBoolean{result}}
			} else if !b.omitWarnings {
				if !b.warnAboutEqualityCheck("==", e.Left, e.Right.Loc) {
					b.warnAboutEqualityCheck("==", e.Right, e.Right.Loc)
				}
			}

		case ast.BinOpStrictEq:
			if result, ok := checkEqualityIfNoSideEffects(e.Left.Data, e.Right.Data); ok {
				return ast.Expr{expr.Loc, &ast.EBoolean{result}}
			} else if !b.omitWarnings {
				if !b.warnAboutEqualityCheck("===", e.Left, e.Right.Loc) {
					b.warnAboutEqualityCheck("===", e.Right, e.Right.Loc)
				}
			}

		case ast.BinOpLooseNe:
			if result, ok := checkEqualityIfNoSideEffects(e.Left.Data, e.Right.Data); ok {
				return ast.Expr{expr.Loc, &ast.EBoolean{!result}}
			} else if !b.omitWarnings {
				if !b.warnAboutEqualityCheck("!=", e.Left, e.Right.Loc) {
					b.warnAboutEqualityCheck("!=", e.Right, e.Right.Loc)
				}
			}

		case ast.BinOpStrictNe:
			if result, ok := checkEqualityIfNoSideEffects(e.Left.Data, e.Right.Data); ok {
				return ast.Expr{expr.Loc, &ast.EBoolean{!result}}
			} else if !b.omitWarnings {
				if !b.warnAboutEqualityCheck("!==", e.Left, e.Right.Loc) {
					b.warnAboutEqualityCheck("!==", e.Right, e.Right.Loc)
				}
			}

		case ast.BinOpNullishCoalescing:
			switch e2 := e.Left.Data.(type) {
			case *ast.EBoolean, *ast.ENumber, *ast.EString, *ast.ERegExp,
				*ast.EObject, *ast.EArray, *ast.EFunction, *ast.EArrow, *ast.EClass:
				return e.Left

			case *ast.ENull, *ast.EUndefined:
				return e.Right

			case *ast.EIdentifier:
				if b.target != ESNext {
					// "a ?? b" => "a != null ? a : b"
					return ast.Expr{expr.Loc, &ast.EIf{
						ast.Expr{expr.Loc, &ast.EBinary{
							ast.BinOpLooseNe,
							ast.Expr{expr.Loc, &ast.EIdentifier{e2.Ref}},
							ast.Expr{expr.Loc, &ast.ENull{}},
						}},
						e.Left,
						e.Right,
					}}
				}

			default:
				if b.target != ESNext {
					// "a() ?? b()" => "_ = a(), _ != null ? _ : b"
					ref := b.generateTempRef()
					return ast.Expr{expr.Loc, &ast.EBinary{
						ast.BinOpComma,
						ast.Expr{expr.Loc, &ast.EBinary{
							ast.BinOpAssign,
							ast.Expr{expr.Loc, &ast.EIdentifier{ref}},
							e.Left,
						}},
						ast.Expr{e.Right.Loc, &ast.EIf{
							ast.Expr{expr.Loc, &ast.EBinary{
								ast.BinOpLooseNe,
								ast.Expr{expr.Loc, &ast.EIdentifier{ref}},
								ast.Expr{expr.Loc, &ast.ENull{}},
							}},
							ast.Expr{expr.Loc, &ast.EIdentifier{ref}},
							e.Right,
						}},
					}}
				}
			}

		case ast.BinOpLogicalOr:
			if boolean, ok := toBooleanWithoutSideEffects(e.Left.Data); ok {
				if boolean {
					return e.Left
				} else {
					return e.Right
				}
			}

		case ast.BinOpLogicalAnd:
			if boolean, ok := toBooleanWithoutSideEffects(e.Left.Data); ok {
				if boolean {
					return e.Right
				} else {
					return e.Left
				}
			}

		case ast.BinOpAdd:
			// "'abc' + 'xyz'" => "'abcxyz'"
			if result := foldStringAddition(e.Left, e.Right); result != nil {
				return *result
			}

			if left, ok := e.Left.Data.(*ast.EBinary); ok {
				// "x + 'abc' + 'xyz'" => "x + 'abcxyz'"
				if result := foldStringAddition(left.Right, e.Right); result != nil {
					return ast.Expr{expr.Loc, &ast.EBinary{left.Op, left.Left, *result}}
				}
			}
		}

	case *ast.EIndex:
		e.Target = b.visitExpr(e.Target)
		e.Index = b.visitExpr(e.Index)

		if b.mangleSyntax {
			// "a['b']" => "a.b"
			if id, ok := e.Index.Data.(*ast.EString); ok {
				text := lexer.UTF16ToString(id.Value)
				if lexer.IsIdentifier(text) {
					return b.maybeRewriteDot(expr.Loc, &ast.EDot{e.Target, text, e.Index.Loc, false})
				}
			}
		}

	case *ast.EUnary:
		e.Value = b.visitExpr(e.Value)

		// Fold constants
		switch e.Op {
		case ast.UnOpNot:
			if boolean, ok := toBooleanWithoutSideEffects(e.Value.Data); ok {
				return ast.Expr{expr.Loc, &ast.EBoolean{!boolean}}
			}

		case ast.UnOpVoid:
			if hasNoSideEffects(e.Value.Data) {
				return ast.Expr{expr.Loc, &ast.EUndefined{}}
			}

		case ast.UnOpTypeOf:
			if typeof, ok := typeofWithoutSideEffects(e.Value.Data); ok {
				return ast.Expr{expr.Loc, &ast.EString{lexer.StringToUTF16(typeof)}}
			}

		case ast.UnOpPos:
			if number, ok := toNumberWithoutSideEffects(e.Value.Data); ok {
				return ast.Expr{expr.Loc, &ast.ENumber{number}}
			}

		case ast.UnOpNeg:
			if number, ok := toNumberWithoutSideEffects(e.Value.Data); ok {
				return ast.Expr{expr.Loc, &ast.ENumber{-number}}
			}
		}

	case *ast.EDot:
		// Substitute user-specified defines
		if define, ok := b.dotDefines[e.Name]; ok && b.isDotDefineMatch(expr, define.parts) {
			return ast.Expr{expr.Loc, define.value}
		}

		e.Target = b.visitExpr(e.Target)
		return b.maybeRewriteDot(expr.Loc, e)

	case *ast.EIf:
		e.Test = b.visitBooleanExpr(e.Test)

		// Fold constants
		if boolean, ok := toBooleanWithoutSideEffects(e.Test.Data); ok {
			if boolean {
				return b.visitExpr(e.Yes)
			} else {
				return b.visitExpr(e.No)
			}
		}

		e.Yes = b.visitExpr(e.Yes)
		e.No = b.visitExpr(e.No)

		if b.mangleSyntax {
			// "!a ? b : c" => "a ? c : b"
			if not, ok := e.Test.Data.(*ast.EUnary); ok && not.Op == ast.UnOpNot {
				e.Test = not.Value
				e.Yes, e.No = e.No, e.Yes
			}
		}

	case *ast.EAwait:
		e.Value = b.visitExpr(e.Value)

	case *ast.EYield:
		if e.Value != nil {
			*e.Value = b.visitExpr(*e.Value)
		}

	case *ast.EArray:
		for i, item := range e.Items {
			e.Items[i] = b.visitExpr(item)
		}

	case *ast.EObject:
		for i, property := range e.Properties {
			if property.Kind != ast.PropertySpread {
				e.Properties[i].Key = b.visitExpr(property.Key)
			}
			if property.Value != nil {
				*property.Value = b.visitExpr(*property.Value)
			}
			if property.Initializer != nil {
				*property.Initializer = b.visitExpr(*property.Initializer)
			}
		}

	case *ast.ECall:
		b.callTarget = e.Target.Data
		e.Target = b.visitExpr(e.Target)
		for i, arg := range e.Args {
			e.Args[i] = b.visitExpr(arg)
		}

		// Track calls to require() and import() so we can use them while bundling
		if id, ok := e.Target.Data.(*ast.EIdentifier); ok && (id.Ref == b.requireRef || id.Ref == b.importRef) {
			// There must be one argument
			if len(e.Args) != 1 {
				b.log.AddError(b.source, expr.Loc,
					fmt.Sprintf("Calls to %s() must take a single argument", b.symbols[id.Ref.InnerIndex].Name))
				return expr
			}
			arg := e.Args[0]

			// The argument must be a string
			str, ok := arg.Data.(*ast.EString)
			if !ok {
				b.log.AddError(b.source, arg.Loc,
					fmt.Sprintf("The argument to %s() must be a string literal", b.symbols[id.Ref.InnerIndex].Name))
				return expr
			}

			// Ignore calls to require() and import() if the control flow is provably
			// dead here. We don't want to spend time scanning the required files
			// if they will never be used.
			if b.isControlFlowDead {
				return ast.Expr{expr.Loc, &ast.ENull{}}
			}

			path := ast.Path{arg.Loc, lexer.UTF16ToString(str.Value)}

			// Create a new expression to represent the operation
			if id.Ref == b.requireRef {
				b.importPaths = append(b.importPaths, ast.ImportPath{Path: path, Kind: ast.ImportRequire})
				return ast.Expr{expr.Loc, &ast.ERequire{path}}
			} else {
				b.importPaths = append(b.importPaths, ast.ImportPath{Path: path, Kind: ast.ImportDynamic})
				return ast.Expr{expr.Loc, &ast.EImport{path}}
			}
		}

	case *ast.ENew:
		e.Target = b.visitExpr(e.Target)
		for i, arg := range e.Args {
			e.Args[i] = b.visitExpr(arg)
		}

	case *ast.EArrow:
		oldTryBodyCount := b.tryBodyCount
		b.tryBodyCount = 0

		b.pushScope(ast.ScopeFunction)
		b.visitArgs(e.Args)
		if e.Expr != nil {
			*e.Expr = b.visitExpr(*e.Expr)
		} else {
			e.Stmts = b.declareAndVisitStmts(e.Stmts)
		}
		b.popScope()

		b.tryBodyCount = oldTryBodyCount

	case *ast.EFunction:
		if e.Fn.Name != nil {
			b.pushScope(ast.ScopeFunctionName)
			name := b.loadNameFromRef(e.Fn.Name.Ref)
			e.Fn.Name.Ref = b.declareSymbol(ast.SymbolOther, e.Fn.Name.Loc, name)
		}
		b.visitFn(&e.Fn)
		if e.Fn.Name != nil {
			b.popScope()
		}

	case *ast.EClass:
		if e.Class.Name != nil {
			b.pushScope(ast.ScopeClassName)
			name := b.loadNameFromRef(e.Class.Name.Ref)
			e.Class.Name.Ref = b.declareSymbol(ast.SymbolOther, e.Class.Name.Loc, name)
		}
		b.visitClass(&e.Class)
		if e.Class.Name != nil {
			b.popScope()
		}

		// Lower class field syntax for browsers that don't support it
		extraExprs, tempRef := b.lowerClass(&e.Class, false /* isStmt */)
		if len(extraExprs) > 0 {
			expr = ast.Expr{expr.Loc, &ast.EBinary{
				ast.BinOpAssign,
				ast.Expr{expr.Loc, &ast.EIdentifier{tempRef}},
				expr,
			}}
			for _, extra := range extraExprs {
				expr = ast.Expr{expr.Loc, &ast.EBinary{ast.BinOpComma, expr, extra}}
			}
			expr = ast.Expr{expr.Loc, &ast.EBinary{
				ast.BinOpComma,
				expr,
				ast.Expr{expr.Loc, &ast.EIdentifier{tempRef}},
			}}
		}

	default:
		panic(fmt.Sprintf("Unexpected expression of type %T", expr.Data))
	}

	return expr
}

func (b *binder) visitFn(fn *ast.Fn) {
	oldTryBodyCount := b.tryBodyCount
	b.tryBodyCount = 0

	b.pushScope(ast.ScopeFunction)
	b.visitArgs(fn.Args)
	fn.Stmts = b.declareAndVisitFnOrModuleStmts(fn.Stmts)
	b.popScope()

	b.tryBodyCount = oldTryBodyCount
}

type LanguageTarget uint8

const (
	ESNext LanguageTarget = iota
	ES2015
	ES2016
	ES2017
	ES2018
	ES2019
	ES2020
)

type JSXOptions struct {
	Parse    bool
	Factory  []string
	Fragment []string
}

type ParseOptions struct {
	IsBundling           bool
	Defines              map[string]ast.E
	MangleSyntax         bool
	KeepSingleExpression bool
	OmitWarnings         bool
	JSX                  JSXOptions
	Target               LanguageTarget
}

func Parse(log logging.Log, source logging.Source, options ParseOptions) (result ast.AST, ok bool) {
	ok = true
	defer func() {
		r := recover()
		if _, isLexerPanic := r.(lexer.LexerPanic); isLexerPanic {
			ok = false
		} else if r != nil {
			panic(r)
		}
	}()

	// Default options for JSX elements
	if len(options.JSX.Factory) == 0 {
		options.JSX.Factory = []string{"React", "createElement"}
	}
	if len(options.JSX.Fragment) == 0 {
		options.JSX.Fragment = []string{"React", "Fragment"}
	}

	p := &parser{
		log:          log,
		source:       source,
		lexer:        lexer.NewLexer(log, source),
		allowIn:      true,
		jsx:          options.JSX,
		omitWarnings: options.OmitWarnings,
	}

	// Parse the file in the first pass, but do not declare and bind symbols.
	stmts := p.parseStmtsUpTo(lexer.TEndOfFile, parseStmtOpts{allowImportAndExport: true})

	// Load user-specified defines
	b := newBinder(source, options)
	b.log = log
	b.allocatedNames = p.allocatedNames

	if options.Defines != nil {
		for k, v := range options.Defines {
			parts := strings.Split(k, ".")
			if len(parts) == 1 {
				b.identifierDefines[k] = v
			} else {
				b.dotDefines[parts[len(parts)-1]] = dotDefine{parts, v}
			}
		}
	}

	// Declare and bind symbols in a second pass over the AST. I started off
	// doing this in a single pass, but it turns out it's pretty much impossible
	// to do this correctly while handling arrow functions because of the grammar
	// ambiguities. This pass traverses the parse tree, builds scopes, and declares
	// all symbols.
	if options.KeepSingleExpression {
		// Sometimes it's helpful to parse a top-level function expression without
		// trimming it as dead code. This is used to parse the bundle loader.
		if len(stmts) != 1 {
			panic("Internal error")
		}
		b.declareStmt(stmts[0])
		expr, ok := stmts[0].Data.(*ast.SExpr)
		if !ok {
			panic("Internal error")
		}
		b.visitExpr(expr.Value)
	} else {
		stmts = b.declareAndVisitFnOrModuleStmts(stmts)
	}

	// Clear the import paths if we don't want any dependencies
	if !options.IsBundling {
		p.importPaths = []ast.ImportPath{}
	}

	result = b.toAST(source, stmts, p.importPaths)
	return
}

func newBinder(source logging.Source, options ParseOptions) *binder {
	b := &binder{
		identifierDefines: make(map[string]ast.E),
		dotDefines:        make(map[string]dotDefine),

		indirectImportItems:     make(map[ast.Ref]bool),
		importItemsForNamespace: make(map[ast.Ref]map[string]ast.Ref),
		exprForImportItem:       make(map[ast.Ref]*ast.ENamespaceImport),
		exportAliases:           make(map[string]bool),

		jsx:          options.JSX,
		omitWarnings: options.OmitWarnings,
		mangleSyntax: options.MangleSyntax,
		isBundling:   options.IsBundling,
		target:       options.Target,
	}

	b.pushScope(ast.ScopeModule)
	b.moduleScope = b.scope

	// This must be set before we call newSymbol() below or the generated refs
	// will be the same for all files
	b.source = source

	// Swap in certain literal values because those can be constant folded
	b.identifierDefines["undefined"] = &ast.EUndefined{}
	b.identifierDefines["NaN"] = &ast.ENumber{math.NaN()}
	b.identifierDefines["Infinity"] = &ast.ENumber{math.Inf(1)}

	// The bundler pre-declares these symbols
	b.exportsRef = b.newSymbol(ast.SymbolHoisted, "exports")
	b.requireRef = b.newSymbol(ast.SymbolHoisted, "require")
	b.moduleRef = b.newSymbol(ast.SymbolHoisted, "module")
	b.importRef = b.newSymbol(ast.SymbolHoisted, "import")

	// Only declare these symbols if we're bundling
	if b.isBundling {
		b.scope.Members["exports"] = b.exportsRef
		b.scope.Members["require"] = b.requireRef
		b.scope.Members["module"] = b.moduleRef
	}

	// This symbol must always be declared because import() calls are special syntax
	b.scope.Members["import"] = b.importRef

	return b
}

func (b *binder) toAST(source logging.Source, stmts []ast.Stmt, importPaths []ast.ImportPath) ast.AST {
	// Make a symbol map that contains our file's symbols
	symbols := ast.SymbolMap{make([][]ast.Symbol, source.Index+1)}
	symbols.Outer[source.Index] = b.symbols

	// Consider this module to have CommonJS exports if the "exports" or "module"
	// variables were referenced somewhere in the module
	hasCommonJsExports := symbols.Get(b.exportsRef).UseCountEstimate > 0 ||
		symbols.Get(b.moduleRef).UseCountEstimate > 0

	return ast.AST{
		ImportPaths:         append(importPaths, b.importPaths...),
		IndirectImportItems: b.indirectImportItems,
		HasCommonJsExports:  hasCommonJsExports,
		Stmts:               stmts,
		ModuleScope:         b.moduleScope,
		Symbols:             &symbols,
		ExportsRef:          b.exportsRef,
		RequireRef:          b.requireRef,
		ModuleRef:           b.moduleRef,
	}
}
