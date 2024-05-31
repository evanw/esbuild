package css_parser

import (
	"strings"

	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

// list-style-image: <image> | none
// <image>: <url> | <gradient>
// <url>: <url()> | <src()>
// <gradient>: <linear-gradient()> | <repeating-linear-gradient()> | <radial-gradient()> | <repeating-radial-gradient()>
//
// list-style-type: <counter-style> | <string> | none (where the string is a literal bullet marker)
// <counter-style>: <counter-style-name> | <symbols()>
// <counter-style-name>: not: decimal | disc | square | circle | disclosure-open | disclosure-closed | <wide keyword>
// when parsing a <custom-ident> with conflicts, only parse one if no other thing can claim it

func (p *parser) processListStyleShorthand(tokens []css_ast.Token) {
	if len(tokens) < 1 || len(tokens) > 3 {
		return
	}

	foundImage := false
	foundPosition := false
	typeIndex := -1
	noneCount := 0

	for i, t := range tokens {
		switch t.Kind {
		case css_lexer.TString:
			// "list-style-type" is definitely not a <custom-ident>
			return

		case css_lexer.TURL:
			if !foundImage {
				foundImage = true
				continue
			}

		case css_lexer.TFunction:
			if !foundImage {
				switch strings.ToLower(t.Text) {
				case "src", "linear-gradient", "repeating-linear-gradient", "radial-gradient", "radial-linear-gradient":
					foundImage = true
					continue
				}
			}

		case css_lexer.TIdent:
			lower := strings.ToLower(t.Text)

			// Note: If "none" is present, it's ambiguous whether it applies to
			// "list-style-image" or "list-style-type". To resolve ambiguity it's
			// applied at the end to whichever property isn't otherwise set.
			if lower == "none" {
				noneCount++
				continue
			}

			if !foundPosition && (lower == "inside" || lower == "outside") {
				foundPosition = true
				continue
			}

			if typeIndex == -1 {
				if cssWideAndReservedKeywords[lower] || predefinedCounterStyles[lower] {
					// "list-style-type" is definitely not a <custom-ident>
					return
				}
				typeIndex = i
				continue
			}
		}

		// Bail if we hit an unexpected token
		return
	}

	if typeIndex != -1 {
		// The first "none" applies to "list-style-image" if it's missing
		if !foundImage && noneCount > 0 {
			noneCount--
		}

		if noneCount > 0 {
			// "list-style-type" is "none", not a <custom-ident>
			return
		}

		if t := &tokens[typeIndex]; t.Kind == css_lexer.TIdent {
			t.Kind = css_lexer.TSymbol
			t.PayloadIndex = p.symbolForName(t.Loc, t.Text).Ref.InnerIndex
		}
	}
}

func (p *parser) processListStyleType(t *css_ast.Token) {
	if t.Kind == css_lexer.TIdent {
		if lower := strings.ToLower(t.Text); lower != "none" && !cssWideAndReservedKeywords[lower] && !predefinedCounterStyles[lower] {
			t.Kind = css_lexer.TSymbol
			t.PayloadIndex = p.symbolForName(t.Loc, t.Text).Ref.InnerIndex
		}
	}
}

// https://drafts.csswg.org/css-counter-styles-3/#predefined-counters
var predefinedCounterStyles = map[string]bool{
	// 6.1. Numeric:
	"arabic-indic":         true,
	"armenian":             true,
	"bengali":              true,
	"cambodian":            true,
	"cjk-decimal":          true,
	"decimal-leading-zero": true,
	"decimal":              true,
	"devanagari":           true,
	"georgian":             true,
	"gujarati":             true,
	"gurmukhi":             true,
	"hebrew":               true,
	"kannada":              true,
	"khmer":                true,
	"lao":                  true,
	"lower-armenian":       true,
	"lower-roman":          true,
	"malayalam":            true,
	"mongolian":            true,
	"myanmar":              true,
	"oriya":                true,
	"persian":              true,
	"tamil":                true,
	"telugu":               true,
	"thai":                 true,
	"tibetan":              true,
	"upper-armenian":       true,
	"upper-roman":          true,

	// 6.2. Alphabetic:
	"hiragana-iroha": true,
	"hiragana":       true,
	"katakana-iroha": true,
	"katakana":       true,
	"lower-alpha":    true,
	"lower-greek":    true,
	"lower-latin":    true,
	"upper-alpha":    true,
	"upper-latin":    true,

	// 6.3. Symbolic:
	"circle":            true,
	"disc":              true,
	"disclosure-closed": true,
	"disclosure-open":   true,
	"square":            true,

	// 6.4. Fixed:
	"cjk-earthly-branch": true,
	"cjk-heavenly-stem":  true,

	// 7.1.1. Japanese:
	"japanese-formal":   true,
	"japanese-informal": true,

	// 7.1.2. Korean:
	"korean-hangul-formal":  true,
	"korean-hanja-formal":   true,
	"korean-hanja-informal": true,

	// 7.1.3. Chinese:
	"simp-chinese-formal":   true,
	"simp-chinese-informal": true,
	"trad-chinese-formal":   true,
	"trad-chinese-informal": true,

	// 7.2. Ethiopic Numeric Counter Style:
	"ethiopic-numeric": true,
}
