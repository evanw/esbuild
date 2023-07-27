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
// <counter-style>: <counter-style-name> | <symnols()>
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
				switch lower {
				case "decimal", "disc", "square", "circle", "disclosure-open", "disclosure-closed":
					// "list-style-type" is definitely not a <custom-ident>
					return
				}
				if cssWideAndReservedKeywords[lower] {
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

		p.processListStyleType(&tokens[typeIndex])
	}
}

func (p *parser) processListStyleType(token *css_ast.Token) {
	if token.Kind == css_lexer.TIdent {
		token.Kind = css_lexer.TSymbol
		token.PayloadIndex = p.symbolForName(token.Loc, token.Text).Ref.InnerIndex
	}
}
