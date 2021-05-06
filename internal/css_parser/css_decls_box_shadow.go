package css_parser

import (
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

func (p *parser) mangleBoxShadow(tokens []css_ast.Token) []css_ast.Token {
	n := len(tokens)
	end := 0
	i := 0

	for i < n {
		t := tokens[i]

		// Parse a run of numbers
		if t.Kind == css_lexer.TNumber || t.Kind == css_lexer.TDimension {
			runStart := i
			for i < n {
				t := tokens[i]
				if t.Kind != css_lexer.TNumber && t.Kind != css_lexer.TDimension {
					break
				}
				if t.TurnLengthIntoNumberIfZero() {
					tokens[i] = t
				}
				i++
			}

			// Trim trailing zeros. There are three valid configurations:
			//
			//   offset-x | offset-y
			//   offset-x | offset-y | blur-radius
			//   offset-x | offset-y | blur-radius | spread-radius
			//
			// If omitted, blur-radius and spread-radius are implied to be zero.
			runEnd := i
			for runEnd > runStart+2 {
				t := tokens[runEnd-1]
				if t.Kind != css_lexer.TNumber || t.Text != "0" {
					break
				}
				runEnd--
			}

			// Copy over the remaining tokens
			end += copy(tokens[end:], tokens[runStart:runEnd])
			continue
		}

		t = p.mangleColor(t)
		tokens[end] = t
		end++
		i++
	}

	// Set the whitespace flags
	tokens = tokens[:end]
	for i := range tokens {
		var whitespace css_ast.WhitespaceFlags
		if i > 0 || !p.options.RemoveWhitespace {
			whitespace |= css_ast.WhitespaceBefore
		}
		if i+1 < end {
			whitespace |= css_ast.WhitespaceAfter
		}
		tokens[i].Whitespace = whitespace
	}
	return tokens
}

func (p *parser) mangleBoxShadows(tokens []css_ast.Token) []css_ast.Token {
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
		end += copy(tokens[end:], p.mangleBoxShadow(tokens[i:comma]))

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
