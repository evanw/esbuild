package css_parser

import (
	"strconv"
	"strings"

	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

// Specification: https://drafts.csswg.org/css-fonts/#font-prop
// [ <font-style> || <font-variant-css2> || <font-weight> || <font-stretch-css3> ]? <font-size> [ / <line-height> ]? <font-family>
func (p *parser) mangleFont(tokens []css_ast.Token) []css_ast.Token {
	var result []css_ast.Token

	// Scan up to the font size
	pos := 0
	for ; pos < len(tokens); pos++ {
		token := tokens[pos]
		if isFontSize(token) {
			break
		}

		switch token.Kind {
		case css_lexer.TIdent:
			switch strings.ToLower(token.Text) {
			case "normal":
				// "All subproperties of the font property are first reset to their initial values"
				// This implies that "normal" doesn't do anything. Also all of the optional values
				// contain "normal" as an option and they are unordered so it's impossible to say
				// what property "normal" corresponds to. Just drop these tokens to save space.
				continue

			// <font-style>
			case "italic":
			case "oblique":
				if pos+1 < len(tokens) && tokens[pos+1].IsAngle() {
					result = append(result, token, tokens[pos+1])
					pos++
					continue
				}

			// <font-variant-css2>
			case "small-caps":

			// <font-weight>
			case "bold", "bolder", "lighter":
				result = append(result, p.mangleFontWeight(token))
				continue

			// <font-stretch-css3>
			case "ultra-condensed", "extra-condensed", "condensed", "semi-condensed",
				"semi-expanded", "expanded", "extra-expanded", "ultra-expanded":

			default:
				// All other tokens are unrecognized, so we bail if we hit one
				return tokens
			}
			result = append(result, token)

		case css_lexer.TNumber:
			// "Only values greater than or equal to 1, and less than or equal to
			// 1000, are valid, and all other values are invalid."
			if value, err := strconv.ParseFloat(token.Text, 64); err != nil || value < 1 || value > 1000 {
				return tokens
			}
			result = append(result, token)

		default:
			// All other tokens are unrecognized, so we bail if we hit one
			return tokens
		}
	}

	// <font-size>
	if pos == len(tokens) {
		return tokens
	}
	result = append(result, tokens[pos])
	pos++

	// / <line-height>
	if pos < len(tokens) && tokens[pos].Kind == css_lexer.TDelimSlash {
		if pos+1 == len(tokens) {
			return tokens
		}
		result = append(result, tokens[pos], tokens[pos+1])
		pos += 2

		// Remove the whitespace around the "/" character
		if p.options.minifyWhitespace {
			result[len(result)-3].Whitespace &= ^css_ast.WhitespaceAfter
			result[len(result)-2].Whitespace = 0
			result[len(result)-1].Whitespace &= ^css_ast.WhitespaceBefore
		}
	}

	// <font-family>
	if family, ok := p.mangleFontFamily(tokens[pos:]); ok {
		if len(result) > 0 && len(family) > 0 && family[0].Kind != css_lexer.TString {
			family[0].Whitespace |= css_ast.WhitespaceBefore
		}
		return append(result, family...)
	}
	return tokens
}

var fontSizeKeywords = map[string]bool{
	// <absolute-size>: https://drafts.csswg.org/css-fonts/#valdef-font-size-absolute-size
	"xx-small":  true,
	"x-small":   true,
	"small":     true,
	"medium":    true,
	"large":     true,
	"x-large":   true,
	"xx-large":  true,
	"xxx-large": true,

	// <relative-size>: https://drafts.csswg.org/css-fonts/#valdef-font-size-relative-size
	"larger":  true,
	"smaller": true,
}

// Specification: https://drafts.csswg.org/css-fonts/#font-size-prop
func isFontSize(token css_ast.Token) bool {
	// <length-percentage>
	if token.Kind == css_lexer.TDimension || token.Kind == css_lexer.TPercentage {
		return true
	}

	// <absolute-size> or <relative-size>
	if token.Kind == css_lexer.TIdent {
		_, ok := fontSizeKeywords[strings.ToLower(token.Text)]
		return ok
	}

	return false
}
