package css_parser

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

func (p *parser) tryToReduceCalcExpression(token css_ast.Token) css_ast.Token {
	if term := tryToParseCalcTerm(*token.Children); term != nil {
		whitespace := css_ast.WhitespaceBefore | css_ast.WhitespaceAfter
		if p.options.RemoveWhitespace {
			whitespace = 0
		}
		term = term.partiallySimplify()
		result := term.convertToToken(whitespace)
		if result.Kind == css_lexer.TOpenParen {
			result.Kind = css_lexer.TFunction
			result.Text = "calc"
		}
		return result
	}
	return token
}

// See: https://www.w3.org/TR/css-values-4/#calc-internal
type calcTerm interface {
	convertToToken(whitespace css_ast.WhitespaceFlags) css_ast.Token
	partiallySimplify() calcTerm
}

type calcSum struct {
	terms []calcTerm
}

type calcProduct struct {
	terms []calcTerm
}

type calcNegate struct {
	term calcTerm
}

type calcInvert struct {
	term calcTerm
}

type calcNumeric struct {
	number float64
	unit   string
}

type calcValue struct {
	token                css_ast.Token
	isInvalidPlusOrMinus bool
}

func floatToStringForCalc(a float64) string {
	if math.IsNaN(a) {
		return "NaN"
	}
	if math.IsInf(a, 1) {
		return "Infinity"
	}
	if math.IsInf(a, -1) {
		return "-Infinity"
	}
	text := fmt.Sprintf("%.05f", a)
	for text[len(text)-1] == '0' {
		text = text[:len(text)-1]
	}
	if text[len(text)-1] == '.' {
		text = text[:len(text)-1]
	}
	return text
}

func (c *calcSum) convertToToken(whitespace css_ast.WhitespaceFlags) css_ast.Token {
	// Specification: https://www.w3.org/TR/css-values-4/#calc-serialize
	tokens := make([]css_ast.Token, 0, len(c.terms)*2)

	// ALGORITHM DEVIATION: Avoid parenthesizing product nodes inside sum nodes
	if product, ok := c.terms[0].(*calcProduct); ok {
		tokens = append(tokens, *product.convertToToken(whitespace).Children...)
	} else {
		tokens = append(tokens, c.terms[0].convertToToken(whitespace))
	}

	for _, term := range c.terms[1:] {
		// If child is a Negate node, append " - " to s, then serialize the Negate’s child and append the result to s.
		if negate, ok := term.(*calcNegate); ok {
			tokens = append(tokens, css_ast.Token{
				Kind:       css_lexer.TDelimMinus,
				Text:       "-",
				Whitespace: css_ast.WhitespaceBefore | css_ast.WhitespaceAfter,
			}, negate.term.convertToToken(whitespace))
			continue
		}

		// If child is a negative numeric value, append " - " to s, then serialize the negation of child as normal and append the result to s.
		if numeric, ok := term.(*calcNumeric); ok && numeric.number < 0 {
			clone := *numeric
			clone.number = -clone.number
			tokens = append(tokens, css_ast.Token{
				Kind:       css_lexer.TDelimMinus,
				Text:       "-",
				Whitespace: css_ast.WhitespaceBefore | css_ast.WhitespaceAfter,
			}, clone.convertToToken(whitespace))
			continue
		}

		// Otherwise, append " + " to s, then serialize child and append the result to s.
		tokens = append(tokens, css_ast.Token{
			Kind:       css_lexer.TDelimPlus,
			Text:       "+",
			Whitespace: css_ast.WhitespaceBefore | css_ast.WhitespaceAfter,
		})

		// ALGORITHM DEVIATION: Avoid parenthesizing product nodes inside sum nodes
		if product, ok := term.(*calcProduct); ok {
			tokens = append(tokens, *product.convertToToken(whitespace).Children...)
		} else {
			tokens = append(tokens, term.convertToToken(whitespace))
		}
	}

	return css_ast.Token{
		Kind:     css_lexer.TOpenParen,
		Text:     "(",
		Children: &tokens,
	}
}

func (c *calcProduct) convertToToken(whitespace css_ast.WhitespaceFlags) css_ast.Token {
	// Specification: https://www.w3.org/TR/css-values-4/#calc-serialize
	tokens := make([]css_ast.Token, 0, len(c.terms)*2)
	tokens = append(tokens, c.terms[0].convertToToken(whitespace))

	for _, term := range c.terms[1:] {
		// If child is an Invert node, append " / " to s, then serialize the Invert’s child and append the result to s.
		if invert, ok := term.(*calcInvert); ok {
			tokens = append(tokens, css_ast.Token{
				Kind:       css_lexer.TDelimSlash,
				Text:       "/",
				Whitespace: whitespace,
			}, invert.term.convertToToken(whitespace))
			continue
		}

		// Otherwise, append " * " to s, then serialize child and append the result to s.
		tokens = append(tokens, css_ast.Token{
			Kind:       css_lexer.TDelimAsterisk,
			Text:       "*",
			Whitespace: whitespace,
		}, term.convertToToken(whitespace))
	}

	return css_ast.Token{
		Kind:     css_lexer.TOpenParen,
		Text:     "(",
		Children: &tokens,
	}
}

func (c *calcNegate) convertToToken(whitespace css_ast.WhitespaceFlags) css_ast.Token {
	// Specification: https://www.w3.org/TR/css-values-4/#calc-serialize
	return css_ast.Token{
		Kind: css_lexer.TOpenParen,
		Text: "(",
		Children: &[]css_ast.Token{
			{Kind: css_lexer.TNumber, Text: "-1"},
			{Kind: css_lexer.TDelimSlash, Text: "*", Whitespace: css_ast.WhitespaceBefore | css_ast.WhitespaceAfter},
			c.term.convertToToken(whitespace),
		},
	}
}

func (c *calcInvert) convertToToken(whitespace css_ast.WhitespaceFlags) css_ast.Token {
	// Specification: https://www.w3.org/TR/css-values-4/#calc-serialize
	return css_ast.Token{
		Kind: css_lexer.TOpenParen,
		Text: "(",
		Children: &[]css_ast.Token{
			{Kind: css_lexer.TNumber, Text: "1"},
			{Kind: css_lexer.TDelimSlash, Text: "/", Whitespace: css_ast.WhitespaceBefore | css_ast.WhitespaceAfter},
			c.term.convertToToken(whitespace),
		},
	}
}

func (c *calcNumeric) convertToToken(whitespace css_ast.WhitespaceFlags) css_ast.Token {
	if c.unit == "" {
		return css_ast.Token{
			Kind: css_lexer.TNumber,
			Text: floatToStringForCalc(c.number),
		}
	} else if c.unit == "%" {
		return css_ast.Token{
			Kind: css_lexer.TPercentage,
			Text: floatToStringForCalc(c.number) + "%",
		}
	} else {
		text := floatToStringForCalc(c.number)
		return css_ast.Token{
			Kind:       css_lexer.TDimension,
			Text:       text + c.unit,
			UnitOffset: uint16(len(text)),
		}
	}
}

func (c *calcValue) convertToToken(whitespace css_ast.WhitespaceFlags) css_ast.Token {
	t := c.token
	t.Whitespace = 0
	return t
}

func (c *calcSum) partiallySimplify() calcTerm {
	// Specification: https://www.w3.org/TR/css-values-4/#calc-simplification

	// For each of root’s children that are Sum nodes, replace them with their children.
	terms := make([]calcTerm, 0, len(c.terms))
	for _, term := range c.terms {
		term = term.partiallySimplify()
		if sum, ok := term.(*calcSum); ok {
			terms = append(terms, sum.terms...)
		} else {
			terms = append(terms, term)
		}
	}

	// For each set of root’s children that are numeric values with identical units, remove
	// those children and replace them with a single numeric value containing the sum of the
	// removed nodes, and with the same unit. (E.g. combine numbers, combine percentages,
	// combine px values, etc.)
	for i := 0; i < len(terms); i++ {
		term := terms[i]
		if numeric, ok := term.(*calcNumeric); ok {
			end := i + 1
			for j := end; j < len(terms); j++ {
				term2 := terms[j]
				if numeric2, ok := term2.(*calcNumeric); ok && numeric2.unit == numeric.unit {
					numeric.number += numeric2.number
				} else {
					terms[end] = term2
					end++
				}
			}
			terms = terms[:end]
		}
	}

	// If root has only a single child at this point, return the child.
	if len(terms) == 1 {
		return terms[0]
	}

	// Otherwise, return root.
	c.terms = terms
	return c
}

func (c *calcProduct) partiallySimplify() calcTerm {
	// Specification: https://www.w3.org/TR/css-values-4/#calc-simplification

	// For each of root’s children that are Product nodes, replace them with their children.
	terms := make([]calcTerm, 0, len(c.terms))
	for _, term := range c.terms {
		term = term.partiallySimplify()
		if product, ok := term.(*calcProduct); ok {
			terms = append(terms, product.terms...)
		} else {
			terms = append(terms, term)
		}
	}

	// If root has multiple children that are numbers (not percentages or dimensions), remove
	// them and replace them with a single number containing the product of the removed nodes.
	for i, term := range terms {
		if numeric, ok := term.(*calcNumeric); ok && numeric.unit == "" {
			end := i + 1
			for j := end; j < len(terms); j++ {
				term2 := terms[j]
				if numeric2, ok := term2.(*calcNumeric); ok && numeric2.unit == "" {
					numeric.number *= numeric2.number
				} else {
					terms[end] = term2
					end++
				}
			}
			terms = terms[:end]
			break
		}
	}

	// If root contains only numeric values and/or Invert nodes containing numeric values,
	// and multiplying the types of all the children (noting that the type of an Invert
	// node is the inverse of its child’s type) results in a type that matches any of the
	// types that a math function can resolve to, return the result of multiplying all the
	// values of the children (noting that the value of an Invert node is the reciprocal
	// of its child’s value), expressed in the result’s canonical unit.
	if len(terms) == 2 {
		// Right now, only handle the case of two numbers, one of which has no unit
		if first, ok := terms[0].(*calcNumeric); ok {
			if second, ok := terms[1].(*calcNumeric); ok {
				if first.unit == "" {
					second.number *= first.number
					return second
				}
				if second.unit == "" {
					first.number *= second.number
					return first
				}
			}
		}
	}

	// ALGORITHM DEVIATION: Divide instead of multiply if the reciprocal is shorter
	for i := 1; i < len(terms); i++ {
		if numeric, ok := terms[i].(*calcNumeric); ok {
			reciprocal := 1 / numeric.number
			multiply := floatToStringForCalc(numeric.number)
			divide := floatToStringForCalc(reciprocal)
			if len(divide) < len(multiply) {
				numeric.number = reciprocal
				terms[i] = &calcInvert{term: numeric}
			}
		}
	}

	// If root has only a single child at this point, return the child.
	if len(terms) == 1 {
		return terms[0]
	}

	// Otherwise, return root.
	c.terms = terms
	return c
}

func (c *calcNegate) partiallySimplify() calcTerm {
	// Specification: https://www.w3.org/TR/css-values-4/#calc-simplification

	c.term = c.term.partiallySimplify()

	// If root’s child is a numeric value, return an equivalent numeric value, but with the value negated (0 - value).
	if numeric, ok := c.term.(*calcNumeric); ok {
		numeric.number = -numeric.number
		return numeric
	}

	// If root’s child is a Negate node, return the child’s child.
	if negate, ok := c.term.(*calcNegate); ok {
		return negate.term
	}

	return c
}

func (c *calcInvert) partiallySimplify() calcTerm {
	// Specification: https://www.w3.org/TR/css-values-4/#calc-simplification

	c.term = c.term.partiallySimplify()

	// If root’s child is a number (not a percentage or dimension) return the reciprocal of the child’s value.
	if numeric, ok := c.term.(*calcNumeric); ok && numeric.unit == "" {
		numeric.number = 1 / numeric.number
		return numeric
	}

	// If root’s child is an Invert node, return the child’s child.
	if invert, ok := c.term.(*calcInvert); ok {
		return invert.term
	}

	return c
}

func (c *calcNumeric) partiallySimplify() calcTerm {
	return c
}

func (c *calcValue) partiallySimplify() calcTerm {
	return c
}

func tryToParseCalcTerm(tokens []css_ast.Token) calcTerm {
	// Specification: https://www.w3.org/TR/css-values-4/#calc-internal
	terms := make([]calcTerm, len(tokens))

	for i, token := range tokens {
		var term calcTerm
		if token.Kind == css_lexer.TFunction && token.Text == "var" {
			// Using "var()" should bail because it can expand to any number of tokens
			return nil
		} else if token.Kind == css_lexer.TOpenParen || (token.Kind == css_lexer.TFunction && token.Text == "calc") {
			term = tryToParseCalcTerm(*token.Children)
			if term == nil {
				return nil
			}
		} else if token.Kind == css_lexer.TNumber {
			if number, err := strconv.ParseFloat(token.Text, 64); err == nil {
				term = &calcNumeric{number: number}
			} else {
				term = &calcValue{token: token}
			}
		} else if token.Kind == css_lexer.TPercentage {
			if number, err := strconv.ParseFloat(token.PercentageValue(), 64); err == nil {
				term = &calcNumeric{number: number, unit: "%"}
			} else {
				term = &calcValue{token: token}
			}
		} else if token.Kind == css_lexer.TDimension {
			if number, err := strconv.ParseFloat(token.DimensionValue(), 64); err == nil {
				term = &calcNumeric{number: number, unit: token.DimensionUnit()}
			} else {
				term = &calcValue{token: token}
			}
		} else if token.Kind == css_lexer.TIdent && strings.EqualFold(token.Text, "Infinity") {
			term = &calcNumeric{number: math.Inf(1)}
		} else if token.Kind == css_lexer.TIdent && strings.EqualFold(token.Text, "-Infinity") {
			term = &calcNumeric{number: math.Inf(-1)}
		} else if token.Kind == css_lexer.TIdent && strings.EqualFold(token.Text, "NaN") {
			term = &calcNumeric{number: math.NaN()}
		} else {
			term = &calcValue{
				token: token,

				// From the specification: "In addition, whitespace is required on both sides of the
				// + and - operators. (The * and / operators can be used without white space around them.)"
				isInvalidPlusOrMinus: i > 0 && i+1 < len(tokens) &&
					(token.Kind == css_lexer.TDelimPlus || token.Kind == css_lexer.TDelimMinus) &&
					(((token.Whitespace&css_ast.WhitespaceBefore) == 0 && (tokens[i-1].Whitespace&css_ast.WhitespaceAfter) == 0) ||
						(token.Whitespace&css_ast.WhitespaceAfter) == 0 && (tokens[i+1].Whitespace&css_ast.WhitespaceBefore) == 0),
			}
		}
		terms[i] = term
	}

	// Collect children into Product and Invert nodes
	first := 1
	for first+1 < len(terms) {
		// If this is a "*" or "/" operator
		if value, ok := terms[first].(*calcValue); ok && (value.token.Kind == css_lexer.TDelimAsterisk || value.token.Kind == css_lexer.TDelimSlash) {
			// Scan over the run
			last := first
			for last+3 < len(terms) {
				if value, ok := terms[last+2].(*calcValue); ok && (value.token.Kind == css_lexer.TDelimAsterisk || value.token.Kind == css_lexer.TDelimSlash) {
					last += 2
				} else {
					break
				}
			}

			// Generate a node for the run
			product := calcProduct{terms: make([]calcTerm, (last-first)/2+2)}
			for i := range product.terms {
				term := terms[first+i*2-1]
				if i > 0 && terms[first+i*2-2].(*calcValue).token.Kind == css_lexer.TDelimSlash {
					term = &calcInvert{term: term}
				}
				product.terms[i] = term
			}

			// Replace the run with a single node
			terms[first-1] = &product
			terms = append(terms[:first], terms[last+2:]...)
			continue
		}

		first++
	}

	// Collect children into Sum and Negate nodes
	first = 1
	for first+1 < len(terms) {
		// If this is a "+" or "-" operator
		if value, ok := terms[first].(*calcValue); ok && !value.isInvalidPlusOrMinus &&
			(value.token.Kind == css_lexer.TDelimPlus || value.token.Kind == css_lexer.TDelimMinus) {
			// Scan over the run
			last := first
			for last+3 < len(terms) {
				if value, ok := terms[last+2].(*calcValue); ok && !value.isInvalidPlusOrMinus &&
					(value.token.Kind == css_lexer.TDelimPlus || value.token.Kind == css_lexer.TDelimMinus) {
					last += 2
				} else {
					break
				}
			}

			// Generate a node for the run
			sum := calcSum{terms: make([]calcTerm, (last-first)/2+2)}
			for i := range sum.terms {
				term := terms[first+i*2-1]
				if i > 0 && terms[first+i*2-2].(*calcValue).token.Kind == css_lexer.TDelimMinus {
					term = &calcNegate{term: term}
				}
				sum.terms[i] = term
			}

			// Replace the run with a single node
			terms[first-1] = &sum
			terms = append(terms[:first], terms[last+2:]...)
			continue
		}

		first++
	}

	// This only succeeds if everything reduces to a single term
	if len(terms) == 1 {
		return terms[0]
	}
	return nil
}
