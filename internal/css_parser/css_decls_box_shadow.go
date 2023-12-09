package css_parser

import (
	"strings"

	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

func (p *parser) lowerAndMangleBoxShadow(tokens []css_ast.Token, wouldClipColor *bool) []css_ast.Token {
	insetCount := 0
	colorCount := 0
	numbersBegin := 0
	numbersCount := 0
	numbersDone := false
	foundUnexpectedToken := false

	for i, t := range tokens {
		if t.Kind == css_lexer.TNumber || t.Kind == css_lexer.TDimension {
			if numbersDone {
				// Track if we found a non-number in between two numbers
				foundUnexpectedToken = true
			}
			if p.options.minifySyntax && t.TurnLengthIntoNumberIfZero() {
				// "0px" => "0"
				tokens[i] = t
			}
			if numbersCount == 0 {
				// Track the index of the first number
				numbersBegin = i
			}
			numbersCount++
		} else {
			if numbersCount != 0 {
				// Track when we find a non-number after a number
				numbersDone = true
			}

			if looksLikeColor(t) {
				colorCount++
				tokens[i] = p.lowerAndMinifyColor(t, wouldClipColor)
			} else if t.Kind == css_lexer.TIdent && strings.EqualFold(t.Text, "inset") {
				insetCount++
			} else {
				// Track if we found a token other than a number, a color, or "inset"
				foundUnexpectedToken = true
			}
		}
	}

	// If everything looks like a valid rule, trim trailing zeros off the numbers.
	// There are three valid configurations of numbers:
	//
	//   offset-x | offset-y
	//   offset-x | offset-y | blur-radius
	//   offset-x | offset-y | blur-radius | spread-radius
	//
	// If omitted, blur-radius and spread-radius are implied to be zero.
	if p.options.minifySyntax && insetCount <= 1 && colorCount <= 1 && numbersCount > 2 && numbersCount <= 4 && !foundUnexpectedToken {
		numbersEnd := numbersBegin + numbersCount
		for numbersCount > 2 && tokens[numbersBegin+numbersCount-1].IsZero() {
			numbersCount--
		}
		tokens = append(tokens[:numbersBegin+numbersCount], tokens[numbersEnd:]...)
	}

	// Set the whitespace flags
	for i := range tokens {
		var whitespace css_ast.WhitespaceFlags
		if i > 0 || !p.options.minifyWhitespace {
			whitespace |= css_ast.WhitespaceBefore
		}
		if i+1 < len(tokens) {
			whitespace |= css_ast.WhitespaceAfter
		}
		tokens[i].Whitespace = whitespace
	}
	return tokens
}

func (p *parser) lowerAndMangleBoxShadows(tokens []css_ast.Token, wouldClipColor *bool) []css_ast.Token {
	n := len(tokens)
	end := 0
	i := 0

	for i < n {
		// Find the comma or the end of the token list
		comma := i
		for comma < n && tokens[comma].Kind != css_lexer.TComma {
			comma++
		}

		// Mangle this individual shadow
		end += copy(tokens[end:], p.lowerAndMangleBoxShadow(tokens[i:comma], wouldClipColor))

		// Skip over the comma
		if comma < n {
			tokens[end] = tokens[comma]
			end++
			comma++
		}
		i = comma
	}

	return tokens[:end]
}
