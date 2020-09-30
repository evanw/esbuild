package css_parser

import (
	"fmt"

	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

func toLowerHex(c byte) (byte, bool) {
	if c >= '0' && c <= '9' {
		return c, true
	}
	if c >= 'a' && c <= 'f' {
		return c, true
	}
	if c >= 'A' && c <= 'F' {
		return c + ('a' - 'A'), true
	}
	return 0, false
}

func (p *parser) mangleColor(token css_ast.Token) css_ast.Token {
	// Note: Do NOT remove color information from fully transparent colors.
	// Safari behaves differently than other browsers for color interpolation:
	// https://css-tricks.com/thing-know-gradients-transparent-black/

	switch token.Kind {
	case css_lexer.THash, css_lexer.THashID:
		text := token.Text
		switch len(text) {
		case 5:
			// "#123f" => "#123"
			r, r_ok := toLowerHex(text[1])
			g, g_ok := toLowerHex(text[2])
			b, b_ok := toLowerHex(text[3])
			a, a_ok := toLowerHex(text[4])
			if r_ok && g_ok && b_ok && a_ok && a == 'f' {
				token.Text = fmt.Sprintf("#%c%c%c", r, g, b)
			}

		case 7:
			// "#112233" => "#123"
			r1, r1_ok := toLowerHex(text[1])
			r2, r2_ok := toLowerHex(text[2])
			g1, g1_ok := toLowerHex(text[3])
			g2, g2_ok := toLowerHex(text[4])
			b1, b1_ok := toLowerHex(text[5])
			b2, b2_ok := toLowerHex(text[6])
			if r1_ok && r2_ok && g1_ok && g2_ok && b1_ok && b2_ok && r1 == r2 && g1 == g2 && b1 == b2 {
				token.Text = fmt.Sprintf("#%c%c%c", r1, g1, b1)
			}

		case 9:
			// "#11223344" => "#1234"
			r1, r1_ok := toLowerHex(text[1])
			r2, r2_ok := toLowerHex(text[2])
			g1, g1_ok := toLowerHex(text[3])
			g2, g2_ok := toLowerHex(text[4])
			b1, b1_ok := toLowerHex(text[5])
			b2, b2_ok := toLowerHex(text[6])
			a1, a1_ok := toLowerHex(text[7])
			a2, a2_ok := toLowerHex(text[8])
			if r1_ok && r2_ok && g1_ok && g2_ok && b1_ok && b2_ok && a1_ok && a2_ok && a1 == a2 {
				if r1 == r2 && g1 == g2 && b1 == b2 {
					if a1 == 'f' {
						token.Text = fmt.Sprintf("#%c%c%c", r1, g1, b1)
					} else {
						token.Text = fmt.Sprintf("#%c%c%c%c", r1, g1, b1, a1)
					}
				} else if a1 == 'f' {
					token.Text = fmt.Sprintf("#%c%c%c%c%c%c", r1, r2, g1, g2, b1, b2)
				}
			}
		}
	}

	return token
}

func (p *parser) processDeclarations(rules []css_ast.R) {
	for _, rule := range rules {
		decl, ok := rule.(*css_ast.RDeclaration)
		if !ok {
			continue
		}

		switch decl.Key {
		case css_ast.DBackgroundColor,
			css_ast.DBorderBlockEndColor,
			css_ast.DBorderBlockStartColor,
			css_ast.DBorderBottomColor,
			css_ast.DBorderColor,
			css_ast.DBorderInlineEndColor,
			css_ast.DBorderInlineStartColor,
			css_ast.DBorderLeftColor,
			css_ast.DBorderRightColor,
			css_ast.DBorderTopColor,
			css_ast.DCaretColor,
			css_ast.DColor,
			css_ast.DColumnRuleColor,
			css_ast.DFloodColor,
			css_ast.DLightingColor,
			css_ast.DOutlineColor,
			css_ast.DStopColor,
			css_ast.DTextDecorationColor,
			css_ast.DTextEmphasisColor:

			if p.options.MangleSyntax && len(decl.Value) == 1 {
				decl.Value[0] = p.mangleColor(decl.Value[0])
			}
		}
	}
}
