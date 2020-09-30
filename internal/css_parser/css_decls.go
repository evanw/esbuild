package css_parser

import (
	"fmt"

	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

// These names are shorter than their hex codes
var shortColorName = map[int]string{
	0x000080: "navy",
	0x008000: "green",
	0x008080: "teal",
	0x4b0082: "indigo",
	0x800000: "maroon",
	0x800080: "purple",
	0x808000: "olive",
	0x808080: "gray",
	0xa0522d: "sienna",
	0xa52a2a: "brown",
	0xc0c0c0: "silver",
	0xcd853f: "peru",
	0xd2b48c: "tan",
	0xda70d6: "orchid",
	0xdda0dd: "plum",
	0xee82ee: "violet",
	0xf0e68c: "khaki",
	0xf0ffff: "azure",
	0xf5deb3: "wheat",
	0xf5f5dc: "beige",
	0xfa8072: "salmon",
	0xfaf0e6: "linen",
	0xff0000: "red",
	0xff6347: "tomato",
	0xff7f50: "coral",
	0xffa500: "orange",
	0xffc0cb: "pink",
	0xffd700: "gold",
	0xffe4c4: "bisque",
	0xfffafa: "snow",
	0xfffff0: "ivory",
}

func hex1(c int) int {
	if c >= 'a' {
		return c + (10 - 'a')
	}
	return c - '0'
}

func hex3(r int, g int, b int) int {
	return hex6(r, r, g, g, b, b)
}

func hex6(r1 int, r2 int, g1 int, g2 int, b1 int, b2 int) int {
	return (hex1(r1) << 20) | (hex1(r2) << 16) | (hex1(g1) << 12) | (hex1(g2) << 8) | (hex1(b1) << 4) | hex1(b2)
}

func toLowerHex(c byte) (int, bool) {
	if c >= '0' && c <= '9' {
		return int(c), true
	}
	if c >= 'a' && c <= 'f' {
		return int(c), true
	}
	if c >= 'A' && c <= 'F' {
		return int(c) + ('a' - 'A'), true
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
		case 4:
			// "#ff0" => "red"
			r, r_ok := toLowerHex(text[1])
			g, g_ok := toLowerHex(text[2])
			b, b_ok := toLowerHex(text[3])
			if r_ok && g_ok && b_ok {
				if name, ok := shortColorName[hex3(r, g, b)]; ok {
					token.Kind = css_lexer.TIdent
					token.Text = name
				}
			}

		case 5:
			// "#123f" => "#123"
			r, r_ok := toLowerHex(text[1])
			g, g_ok := toLowerHex(text[2])
			b, b_ok := toLowerHex(text[3])
			a, a_ok := toLowerHex(text[4])
			if r_ok && g_ok && b_ok && a_ok && a == 'f' {
				if name, ok := shortColorName[hex3(r, g, b)]; ok {
					token.Kind = css_lexer.TIdent
					token.Text = name
				} else {
					token.Text = fmt.Sprintf("#%c%c%c", r, g, b)
				}
			}

		case 7:
			// "#112233" => "#123"
			r1, r1_ok := toLowerHex(text[1])
			r2, r2_ok := toLowerHex(text[2])
			g1, g1_ok := toLowerHex(text[3])
			g2, g2_ok := toLowerHex(text[4])
			b1, b1_ok := toLowerHex(text[5])
			b2, b2_ok := toLowerHex(text[6])
			if r1_ok && r2_ok && g1_ok && g2_ok && b1_ok && b2_ok {
				if name, ok := shortColorName[hex6(r1, r2, g1, g2, b1, b2)]; ok {
					token.Kind = css_lexer.TIdent
					token.Text = name
				} else if r1 == r2 && g1 == g2 && b1 == b2 {
					token.Text = fmt.Sprintf("#%c%c%c", r1, g1, b1)
				}
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
				if a1 == 'f' {
					if name, ok := shortColorName[hex6(r1, r2, g1, g2, b1, b2)]; ok {
						token.Kind = css_lexer.TIdent
						token.Text = name
					} else if r1 == r2 && g1 == g2 && b1 == b2 {
						token.Text = fmt.Sprintf("#%c%c%c", r1, g1, b1)
					} else {
						token.Text = fmt.Sprintf("#%c%c%c%c%c%c", r1, r2, g1, g2, b1, b2)
					}
				} else if r1 == r2 && g1 == g2 && b1 == b2 {
					token.Text = fmt.Sprintf("#%c%c%c%c", r1, g1, b1, a1)
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
