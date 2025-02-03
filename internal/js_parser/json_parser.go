package js_parser

import (
	"fmt"

	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_lexer"
	"github.com/evanw/esbuild/internal/logger"
)

type jsonParser struct {
	log                            logger.Log
	source                         logger.Source
	tracker                        logger.LineColumnTracker
	lexer                          js_lexer.Lexer
	options                        JSONOptions
	suppressWarningsAboutWeirdCode bool
}

func (p *jsonParser) parseMaybeTrailingComma(closeToken js_lexer.T) bool {
	commaRange := p.lexer.Range()
	p.lexer.Expect(js_lexer.TComma)

	if p.lexer.Token == closeToken {
		if p.options.Flavor == js_lexer.JSON {
			p.log.AddError(&p.tracker, commaRange, "JSON does not support trailing commas")
		}
		return false
	}

	return true
}

func (p *jsonParser) parseExpr() js_ast.Expr {
	loc := p.lexer.Loc()

	switch p.lexer.Token {
	case js_lexer.TFalse:
		p.lexer.Next()
		return js_ast.Expr{Loc: loc, Data: &js_ast.EBoolean{Value: false}}

	case js_lexer.TTrue:
		p.lexer.Next()
		return js_ast.Expr{Loc: loc, Data: &js_ast.EBoolean{Value: true}}

	case js_lexer.TNull:
		p.lexer.Next()
		return js_ast.Expr{Loc: loc, Data: js_ast.ENullShared}

	case js_lexer.TStringLiteral:
		value := p.lexer.StringLiteral()
		p.lexer.Next()
		return js_ast.Expr{Loc: loc, Data: &js_ast.EString{Value: value}}

	case js_lexer.TNumericLiteral:
		value := p.lexer.Number
		p.lexer.Next()
		return js_ast.Expr{Loc: loc, Data: &js_ast.ENumber{Value: value}}

	case js_lexer.TMinus:
		p.lexer.Next()
		value := p.lexer.Number
		p.lexer.Expect(js_lexer.TNumericLiteral)
		return js_ast.Expr{Loc: loc, Data: &js_ast.ENumber{Value: -value}}

	case js_lexer.TOpenBracket:
		p.lexer.Next()
		isSingleLine := !p.lexer.HasNewlineBefore
		items := []js_ast.Expr{}

		for p.lexer.Token != js_lexer.TCloseBracket {
			if len(items) > 0 {
				if p.lexer.HasNewlineBefore {
					isSingleLine = false
				}
				if !p.parseMaybeTrailingComma(js_lexer.TCloseBracket) {
					break
				}
				if p.lexer.HasNewlineBefore {
					isSingleLine = false
				}
			}

			item := p.parseExpr()
			items = append(items, item)
		}

		if p.lexer.HasNewlineBefore {
			isSingleLine = false
		}
		closeBracketLoc := p.lexer.Loc()
		p.lexer.Expect(js_lexer.TCloseBracket)
		return js_ast.Expr{Loc: loc, Data: &js_ast.EArray{
			Items:           items,
			IsSingleLine:    isSingleLine,
			CloseBracketLoc: closeBracketLoc,
		}}

	case js_lexer.TOpenBrace:
		p.lexer.Next()
		isSingleLine := !p.lexer.HasNewlineBefore
		properties := []js_ast.Property{}
		duplicates := make(map[string]logger.Range)

		for p.lexer.Token != js_lexer.TCloseBrace {
			if len(properties) > 0 {
				if p.lexer.HasNewlineBefore {
					isSingleLine = false
				}
				if !p.parseMaybeTrailingComma(js_lexer.TCloseBrace) {
					break
				}
				if p.lexer.HasNewlineBefore {
					isSingleLine = false
				}
			}

			keyString := p.lexer.StringLiteral()
			keyRange := p.lexer.Range()
			key := js_ast.Expr{Loc: keyRange.Loc, Data: &js_ast.EString{Value: keyString}}
			p.lexer.Expect(js_lexer.TStringLiteral)

			// Warn about duplicate keys
			if !p.suppressWarningsAboutWeirdCode {
				keyText := helpers.UTF16ToString(keyString)
				if prevRange, ok := duplicates[keyText]; ok {
					p.log.AddIDWithNotes(logger.MsgID_JS_DuplicateObjectKey, logger.Warning, &p.tracker, keyRange,
						fmt.Sprintf("Duplicate key %q in object literal", keyText),
						[]logger.MsgData{p.tracker.MsgData(prevRange, fmt.Sprintf("The original key %q is here:", keyText))})
				} else {
					duplicates[keyText] = keyRange
				}
			}

			p.lexer.Expect(js_lexer.TColon)
			value := p.parseExpr()

			property := js_ast.Property{
				Kind:       js_ast.PropertyField,
				Loc:        keyRange.Loc,
				Key:        key,
				ValueOrNil: value,
			}

			// The key "__proto__" must not be a string literal in JavaScript because
			// that actually modifies the prototype of the object. This can be
			// avoided by using a computed property key instead of a string literal.
			if helpers.UTF16EqualsString(keyString, "__proto__") && !p.options.UnsupportedJSFeatures.Has(compat.ObjectExtensions) {
				property.Flags |= js_ast.PropertyIsComputed
			}

			properties = append(properties, property)
		}

		if p.lexer.HasNewlineBefore {
			isSingleLine = false
		}
		closeBraceLoc := p.lexer.Loc()
		p.lexer.Expect(js_lexer.TCloseBrace)
		return js_ast.Expr{Loc: loc, Data: &js_ast.EObject{
			Properties:    properties,
			IsSingleLine:  isSingleLine,
			CloseBraceLoc: closeBraceLoc,
		}}

	case js_lexer.TBigIntegerLiteral:
		if !p.options.IsForDefine {
			p.lexer.Unexpected()
		}
		value := p.lexer.Identifier
		p.lexer.Next()
		return js_ast.Expr{Loc: loc, Data: &js_ast.EBigInt{Value: value.String}}

	default:
		p.lexer.Unexpected()
		return js_ast.Expr{}
	}
}

type JSONOptions struct {
	UnsupportedJSFeatures compat.JSFeature
	Flavor                js_lexer.JSONFlavor
	ErrorSuffix           string
	IsForDefine           bool
}

func ParseJSON(log logger.Log, source logger.Source, options JSONOptions) (result js_ast.Expr, ok bool) {
	ok = true
	defer func() {
		r := recover()
		if _, isLexerPanic := r.(js_lexer.LexerPanic); isLexerPanic {
			ok = false
		} else if r != nil {
			panic(r)
		}
	}()

	if options.ErrorSuffix == "" {
		options.ErrorSuffix = " in JSON"
	}

	p := &jsonParser{
		log:                            log,
		source:                         source,
		tracker:                        logger.MakeLineColumnTracker(&source),
		options:                        options,
		lexer:                          js_lexer.NewLexerJSON(log, source, options.Flavor, options.ErrorSuffix),
		suppressWarningsAboutWeirdCode: helpers.IsInsideNodeModules(source.KeyPath.Text),
	}

	result = p.parseExpr()
	p.lexer.Expect(js_lexer.TEndOfFile)
	return
}

func isValidJSON(value js_ast.Expr) bool {
	switch e := value.Data.(type) {
	case *js_ast.ENull, *js_ast.EBoolean, *js_ast.EString, *js_ast.ENumber:
		return true

	case *js_ast.EArray:
		for _, item := range e.Items {
			if !isValidJSON(item) {
				return false
			}
		}
		return true

	case *js_ast.EObject:
		for _, property := range e.Properties {
			if property.Kind != js_ast.PropertyField || property.Flags.Has(js_ast.PropertyIsComputed) {
				return false
			}
			if _, ok := property.Key.Data.(*js_ast.EString); !ok {
				return false
			}
			if !isValidJSON(property.ValueOrNil) {
				return false
			}
		}
		return true
	}

	return false
}
