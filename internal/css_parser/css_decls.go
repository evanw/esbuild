package css_parser

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

var commaToken = css_ast.Token{
	Kind:               css_lexer.TComma,
	Text:               ",",
	HasWhitespaceAfter: true,
}

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

// These names are longer than their hex codes
var shortColorHex = map[string]string{
	"aliceblue":            "#f0f8ff",
	"antiquewhite":         "#faebd7",
	"aquamarine":           "#7fffd4",
	"black":                "#000",
	"blanchedalmond":       "#ffebcd",
	"blueviolet":           "#8a2be2",
	"burlywood":            "#deb887",
	"cadetblue":            "#5f9ea0",
	"chartreuse":           "#7fff00",
	"chocolate":            "#d2691e",
	"cornflowerblue":       "#6495ed",
	"cornsilk":             "#fff8dc",
	"darkblue":             "#00008b",
	"darkcyan":             "#008b8b",
	"darkgoldenrod":        "#b8860b",
	"darkgray":             "#a9a9a9",
	"darkgreen":            "#006400",
	"darkgrey":             "#a9a9a9",
	"darkkhaki":            "#bdb76b",
	"darkmagenta":          "#8b008b",
	"darkolivegreen":       "#556b2f",
	"darkorange":           "#ff8c00",
	"darkorchid":           "#9932cc",
	"darksalmon":           "#e9967a",
	"darkseagreen":         "#8fbc8f",
	"darkslateblue":        "#483d8b",
	"darkslategray":        "#2f4f4f",
	"darkslategrey":        "#2f4f4f",
	"darkturquoise":        "#00ced1",
	"darkviolet":           "#9400d3",
	"deeppink":             "#ff1493",
	"deepskyblue":          "#00bfff",
	"dodgerblue":           "#1e90ff",
	"firebrick":            "#b22222",
	"floralwhite":          "#fffaf0",
	"forestgreen":          "#228b22",
	"fuchsia":              "#f0f",
	"gainsboro":            "#dcdcdc",
	"ghostwhite":           "#f8f8ff",
	"goldenrod":            "#daa520",
	"greenyellow":          "#adff2f",
	"honeydew":             "#f0fff0",
	"indianred":            "#cd5c5c",
	"lavender":             "#e6e6fa",
	"lavenderblush":        "#fff0f5",
	"lawngreen":            "#7cfc00",
	"lemonchiffon":         "#fffacd",
	"lightblue":            "#add8e6",
	"lightcoral":           "#f08080",
	"lightcyan":            "#e0ffff",
	"lightgoldenrodyellow": "#fafad2",
	"lightgray":            "#d3d3d3",
	"lightgreen":           "#90ee90",
	"lightgrey":            "#d3d3d3",
	"lightpink":            "#ffb6c1",
	"lightsalmon":          "#ffa07a",
	"lightseagreen":        "#20b2aa",
	"lightskyblue":         "#87cefa",
	"lightslategray":       "#789",
	"lightslategrey":       "#789",
	"lightsteelblue":       "#b0c4de",
	"lightyellow":          "#ffffe0",
	"limegreen":            "#32cd32",
	"magenta":              "#f0f",
	"mediumaquamarine":     "#66cdaa",
	"mediumblue":           "#0000cd",
	"mediumorchid":         "#ba55d3",
	"mediumpurple":         "#9370db",
	"mediumseagreen":       "#3cb371",
	"mediumslateblue":      "#7b68ee",
	"mediumspringgreen":    "#00fa9a",
	"mediumturquoise":      "#48d1cc",
	"mediumvioletred":      "#c71585",
	"midnightblue":         "#191970",
	"mintcream":            "#f5fffa",
	"mistyrose":            "#ffe4e1",
	"moccasin":             "#ffe4b5",
	"navajowhite":          "#ffdead",
	"olivedrab":            "#6b8e23",
	"orangered":            "#ff4500",
	"palegoldenrod":        "#eee8aa",
	"palegreen":            "#98fb98",
	"paleturquoise":        "#afeeee",
	"palevioletred":        "#db7093",
	"papayawhip":           "#ffefd5",
	"peachpuff":            "#ffdab9",
	"powderblue":           "#b0e0e6",
	"rebeccapurple":        "#663399",
	"rosybrown":            "#bc8f8f",
	"royalblue":            "#4169e1",
	"saddlebrown":          "#8b4513",
	"sandybrown":           "#f4a460",
	"seagreen":             "#2e8b57",
	"seashell":             "#fff5ee",
	"slateblue":            "#6a5acd",
	"slategray":            "#708090",
	"slategrey":            "#708090",
	"springgreen":          "#00ff7f",
	"steelblue":            "#4682b4",
	"turquoise":            "#40e0d0",
	"white":                "#fff",
	"whitesmoke":           "#f5f5f5",
	"yellow":               "#ff0",
	"yellowgreen":          "#9acd32",
}

func hex1(c int) int {
	if c >= 'a' {
		return c + (10 - 'a')
	}
	return c - '0'
}

func hex2(hi int, lo int) int {
	return (hex1(hi) << 4) | hex1(lo)
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

func floatToString(a float64) string {
	text := fmt.Sprintf("%.03f", a)
	for text[len(text)-1] == '0' {
		text = text[:len(text)-1]
	}
	if text[len(text)-1] == '.' {
		text = text[:len(text)-1]
	}
	return text
}

func degreesForAngle(token css_ast.Token) (float64, bool) {
	if token.Kind == css_lexer.TDimension {
		if value, err := strconv.ParseFloat(token.DimensionValue(), 64); err == nil {
			switch token.DimensionUnit() {
			case "deg":
				return value, true
			case "grad":
				return value * (360.0 / 400.0), true
			case "rad":
				return value * (180.0 / math.Pi), true
			case "turn":
				return value * 360.0, true
			}
		}
	}
	return 0, false
}

func lowerAlphaPercentageToNumber(token css_ast.Token) css_ast.Token {
	if token.Kind == css_lexer.TPercentage {
		if value, err := strconv.ParseFloat(token.Text[:len(token.Text)-1], 64); err == nil {
			token.Kind = css_lexer.TNumber
			token.Text = floatToString(value / 100.0)
		}
	}
	return token
}

// Convert newer color syntax to older color syntax for older browsers
func (p *parser) lowerColor(token css_ast.Token) css_ast.Token {
	text := token.Text

	switch token.Kind {
	case css_lexer.THash, css_lexer.THashID:
		if p.options.UnsupportedCSSFeatures.Has(compat.HexRGBA) {
			switch len(text) {
			case 5:
				// "#1234" => "rgba(1, 2, 3, 0.004)"
				r, r_ok := toLowerHex(text[1])
				g, g_ok := toLowerHex(text[2])
				b, b_ok := toLowerHex(text[3])
				a, a_ok := toLowerHex(text[4])
				if r_ok && g_ok && b_ok && a_ok {
					token.Kind = css_lexer.TFunction
					token.Text = "rgba("
					token.Children = &[]css_ast.Token{
						{Kind: css_lexer.TNumber, Text: strconv.Itoa(hex2(r, r))},
						commaToken,
						{Kind: css_lexer.TNumber, Text: strconv.Itoa(hex2(g, g))},
						commaToken,
						{Kind: css_lexer.TNumber, Text: strconv.Itoa(hex2(b, b))},
						commaToken,
						{Kind: css_lexer.TNumber, Text: floatToString(float64(hex2(a, a)) / 255.0)},
					}
				}

			case 9:
				// "#12345678" => "rgba(18, 52, 86, 0.47)"
				r1, r1_ok := toLowerHex(text[1])
				r2, r2_ok := toLowerHex(text[2])
				g1, g1_ok := toLowerHex(text[3])
				g2, g2_ok := toLowerHex(text[4])
				b1, b1_ok := toLowerHex(text[5])
				b2, b2_ok := toLowerHex(text[6])
				a1, a1_ok := toLowerHex(text[7])
				a2, a2_ok := toLowerHex(text[8])
				if r1_ok && r2_ok && g1_ok && g2_ok && b1_ok && b2_ok && a1_ok && a2_ok {
					token.Kind = css_lexer.TFunction
					token.Text = "rgba("
					token.Children = &[]css_ast.Token{
						{Kind: css_lexer.TNumber, Text: strconv.Itoa(hex2(r1, r2))},
						commaToken,
						{Kind: css_lexer.TNumber, Text: strconv.Itoa(hex2(g1, g2))},
						commaToken,
						{Kind: css_lexer.TNumber, Text: strconv.Itoa(hex2(b1, b2))},
						commaToken,
						{Kind: css_lexer.TNumber, Text: floatToString(float64(hex2(a1, a2)) / 255.0)},
					}
				}
			}
		}

	case css_lexer.TIdent:
		if text == "rebeccapurple" && p.options.UnsupportedCSSFeatures.Has(compat.RebeccaPurple) {
			token.Kind = css_lexer.THash
			token.Text = "#663399"
		}

	case css_lexer.TFunction:
		switch text {
		case "rgb(", "rgba(", "hsl(", "hsla(":
			if p.options.UnsupportedCSSFeatures.Has(compat.Modern_RGB_HSL) {
				args := *token.Children
				removeAlpha := false
				addAlpha := false

				// "hsl(1deg, 2%, 3%)" => "hsl(1, 2%, 3%)"
				if (text == "hsl(" || text == "hsla(") && len(args) > 0 {
					if degrees, ok := degreesForAngle(args[0]); ok {
						args[0].Kind = css_lexer.TNumber
						args[0].Text = floatToString(degrees)
					}
				}

				switch len(args) {
				case 3:
					// "rgba(1 2 3)" => "rgb(1, 2, 3)"
					// "hsla(1 2% 3%)" => "rgb(1, 2%, 3%)"
					removeAlpha = true
					args[0].HasWhitespaceAfter = false
					args[1].HasWhitespaceAfter = false
					token.Children = &[]css_ast.Token{
						args[0], commaToken,
						args[1], commaToken,
						args[2],
					}

				case 5:
					// "rgba(1, 2, 3)" => "rgb(1, 2, 3)"
					// "hsla(1, 2%, 3%)" => "hsl(1%, 2%, 3%)"
					if args[1].Kind == css_lexer.TComma && args[3].Kind == css_lexer.TComma {
						removeAlpha = true
						break
					}

					// "rgb(1 2 3 / 4%)" => "rgba(1, 2, 3, 0.04)"
					// "hsl(1 2% 3% / 4%)" => "hsla(1, 2%, 3%, 0.04)"
					if args[3].Kind == css_lexer.TDelimSlash {
						addAlpha = true
						args[0].HasWhitespaceAfter = false
						args[1].HasWhitespaceAfter = false
						args[2].HasWhitespaceAfter = false
						token.Children = &[]css_ast.Token{
							args[0], commaToken,
							args[1], commaToken,
							args[2], commaToken,
							lowerAlphaPercentageToNumber(args[4]),
						}
					}

				case 7:
					// "rgb(1%, 2%, 3%, 4%)" => "rgba(1%, 2%, 3%, 0.04)"
					// "hsl(1, 2%, 3%, 4%)" => "hsla(1, 2%, 3%, 0.04)"
					if args[1].Kind == css_lexer.TComma && args[3].Kind == css_lexer.TComma && args[5].Kind == css_lexer.TComma {
						addAlpha = true
						args[6] = lowerAlphaPercentageToNumber(args[6])
					}
				}

				if removeAlpha {
					if text == "rgba(" {
						token.Text = "rgb("
					} else if text == "hsla(" {
						token.Text = "hsl("
					}
				} else if addAlpha {
					if text == "rgb(" {
						token.Text = "rgba("
					} else if text == "hsl(" {
						token.Text = "hsla("
					}
				}
			}
		}
	}

	return token
}

func (p *parser) mangleColor(token css_ast.Token) css_ast.Token {
	// Note: Do NOT remove color information from fully transparent colors.
	// Safari behaves differently than other browsers for color interpolation:
	// https://css-tricks.com/thing-know-gradients-transparent-black/

	switch token.Kind {
	case css_lexer.TIdent:
		if hex, ok := shortColorHex[strings.ToLower(token.Text)]; ok {
			token.Text = hex
		}

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

			if len(decl.Value) == 1 {
				decl.Value[0] = p.lowerColor(decl.Value[0])

				if p.options.MangleSyntax {
					decl.Value[0] = p.mangleColor(decl.Value[0])
				}
			}
		}
	}
}
