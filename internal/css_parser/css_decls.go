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

var colorNameToHex = map[string]uint32{
	"black":                0x000000,
	"silver":               0xc0c0c0,
	"gray":                 0x808080,
	"white":                0xffffff,
	"maroon":               0x800000,
	"red":                  0xff0000,
	"purple":               0x800080,
	"fuchsia":              0xff00ff,
	"green":                0x008000,
	"lime":                 0x00ff00,
	"olive":                0x808000,
	"yellow":               0xffff00,
	"navy":                 0x000080,
	"blue":                 0x0000ff,
	"teal":                 0x008080,
	"aqua":                 0x00ffff,
	"orange":               0xffa500,
	"aliceblue":            0xf0f8ff,
	"antiquewhite":         0xfaebd7,
	"aquamarine":           0x7fffd4,
	"azure":                0xf0ffff,
	"beige":                0xf5f5dc,
	"bisque":               0xffe4c4,
	"blanchedalmond":       0xffebcd,
	"blueviolet":           0x8a2be2,
	"brown":                0xa52a2a,
	"burlywood":            0xdeb887,
	"cadetblue":            0x5f9ea0,
	"chartreuse":           0x7fff00,
	"chocolate":            0xd2691e,
	"coral":                0xff7f50,
	"cornflowerblue":       0x6495ed,
	"cornsilk":             0xfff8dc,
	"crimson":              0xdc143c,
	"cyan":                 0x00ffff,
	"darkblue":             0x00008b,
	"darkcyan":             0x008b8b,
	"darkgoldenrod":        0xb8860b,
	"darkgray":             0xa9a9a9,
	"darkgreen":            0x006400,
	"darkgrey":             0xa9a9a9,
	"darkkhaki":            0xbdb76b,
	"darkmagenta":          0x8b008b,
	"darkolivegreen":       0x556b2f,
	"darkorange":           0xff8c00,
	"darkorchid":           0x9932cc,
	"darkred":              0x8b0000,
	"darksalmon":           0xe9967a,
	"darkseagreen":         0x8fbc8f,
	"darkslateblue":        0x483d8b,
	"darkslategray":        0x2f4f4f,
	"darkslategrey":        0x2f4f4f,
	"darkturquoise":        0x00ced1,
	"darkviolet":           0x9400d3,
	"deeppink":             0xff1493,
	"deepskyblue":          0x00bfff,
	"dimgray":              0x696969,
	"dimgrey":              0x696969,
	"dodgerblue":           0x1e90ff,
	"firebrick":            0xb22222,
	"floralwhite":          0xfffaf0,
	"forestgreen":          0x228b22,
	"gainsboro":            0xdcdcdc,
	"ghostwhite":           0xf8f8ff,
	"gold":                 0xffd700,
	"goldenrod":            0xdaa520,
	"greenyellow":          0xadff2f,
	"grey":                 0x808080,
	"honeydew":             0xf0fff0,
	"hotpink":              0xff69b4,
	"indianred":            0xcd5c5c,
	"indigo":               0x4b0082,
	"ivory":                0xfffff0,
	"khaki":                0xf0e68c,
	"lavender":             0xe6e6fa,
	"lavenderblush":        0xfff0f5,
	"lawngreen":            0x7cfc00,
	"lemonchiffon":         0xfffacd,
	"lightblue":            0xadd8e6,
	"lightcoral":           0xf08080,
	"lightcyan":            0xe0ffff,
	"lightgoldenrodyellow": 0xfafad2,
	"lightgray":            0xd3d3d3,
	"lightgreen":           0x90ee90,
	"lightgrey":            0xd3d3d3,
	"lightpink":            0xffb6c1,
	"lightsalmon":          0xffa07a,
	"lightseagreen":        0x20b2aa,
	"lightskyblue":         0x87cefa,
	"lightslategray":       0x778899,
	"lightslategrey":       0x778899,
	"lightsteelblue":       0xb0c4de,
	"lightyellow":          0xffffe0,
	"limegreen":            0x32cd32,
	"linen":                0xfaf0e6,
	"magenta":              0xff00ff,
	"mediumaquamarine":     0x66cdaa,
	"mediumblue":           0x0000cd,
	"mediumorchid":         0xba55d3,
	"mediumpurple":         0x9370db,
	"mediumseagreen":       0x3cb371,
	"mediumslateblue":      0x7b68ee,
	"mediumspringgreen":    0x00fa9a,
	"mediumturquoise":      0x48d1cc,
	"mediumvioletred":      0xc71585,
	"midnightblue":         0x191970,
	"mintcream":            0xf5fffa,
	"mistyrose":            0xffe4e1,
	"moccasin":             0xffe4b5,
	"navajowhite":          0xffdead,
	"oldlace":              0xfdf5e6,
	"olivedrab":            0x6b8e23,
	"orangered":            0xff4500,
	"orchid":               0xda70d6,
	"palegoldenrod":        0xeee8aa,
	"palegreen":            0x98fb98,
	"paleturquoise":        0xafeeee,
	"palevioletred":        0xdb7093,
	"papayawhip":           0xffefd5,
	"peachpuff":            0xffdab9,
	"peru":                 0xcd853f,
	"pink":                 0xffc0cb,
	"plum":                 0xdda0dd,
	"powderblue":           0xb0e0e6,
	"rosybrown":            0xbc8f8f,
	"royalblue":            0x4169e1,
	"saddlebrown":          0x8b4513,
	"salmon":               0xfa8072,
	"sandybrown":           0xf4a460,
	"seagreen":             0x2e8b57,
	"seashell":             0xfff5ee,
	"sienna":               0xa0522d,
	"skyblue":              0x87ceeb,
	"slateblue":            0x6a5acd,
	"slategray":            0x708090,
	"slategrey":            0x708090,
	"snow":                 0xfffafa,
	"springgreen":          0x00ff7f,
	"steelblue":            0x4682b4,
	"tan":                  0xd2b48c,
	"thistle":              0xd8bfd8,
	"tomato":               0xff6347,
	"turquoise":            0x40e0d0,
	"violet":               0xee82ee,
	"wheat":                0xf5deb3,
	"whitesmoke":           0xf5f5f5,
	"yellowgreen":          0x9acd32,
	"rebeccapurple":        0x66339,
}

func hex(c byte) (int, bool) {
	if c >= '0' && c <= '9' {
		return int(c) - '0', true
	}
	if c >= 'a' && c <= 'f' {
		return int(c) + (10 - 'a'), true
	}
	if c >= 'A' && c <= 'F' {
		return int(c) + (10 - 'A'), true
	}
	return 0, false
}

func hex2(hi int, lo int) int {
	return (hi << 4) | lo
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
				if r, ok := hex(text[1]); ok {
					if g, ok := hex(text[2]); ok {
						if b, ok := hex(text[3]); ok {
							if a, ok := hex(text[4]); ok {
								token.Kind = css_lexer.TFunction
								token.Text = "rgba("
								token.Children = &[]css_ast.Token{
									{Kind: css_lexer.TNumber, Text: strconv.Itoa(hex2(r, r))}, commaToken,
									{Kind: css_lexer.TNumber, Text: strconv.Itoa(hex2(g, g))}, commaToken,
									{Kind: css_lexer.TNumber, Text: strconv.Itoa(hex2(b, b))}, commaToken,
									{Kind: css_lexer.TNumber, Text: floatToString(float64(hex2(a, a)) / 255)},
								}
							}
						}
					}
				}

			case 9:
				// "#12345678" => "rgba(18, 52, 86, 0.47)"
				if r1, ok := hex(text[1]); ok {
					if r2, ok := hex(text[2]); ok {
						if g1, ok := hex(text[3]); ok {
							if g2, ok := hex(text[4]); ok {
								if b1, ok := hex(text[5]); ok {
									if b2, ok := hex(text[6]); ok {
										if a1, ok := hex(text[7]); ok {
											if a2, ok := hex(text[8]); ok {
												token.Kind = css_lexer.TFunction
												token.Text = "rgba("
												token.Children = &[]css_ast.Token{
													{Kind: css_lexer.TNumber, Text: strconv.Itoa(hex2(r1, r2))}, commaToken,
													{Kind: css_lexer.TNumber, Text: strconv.Itoa(hex2(g1, g2))}, commaToken,
													{Kind: css_lexer.TNumber, Text: strconv.Itoa(hex2(b1, b2))}, commaToken,
													{Kind: css_lexer.TNumber, Text: floatToString(float64(hex2(a1, a2)) / 255)},
												}
											}
										}
									}
								}
							}
						}
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

func parseColor(token css_ast.Token) (int, int, int, int, bool) {
	text := token.Text

	switch token.Kind {
	case css_lexer.TIdent:
		if hex, ok := colorNameToHex[strings.ToLower(text)]; ok {
			return int(hex >> 16), int((hex >> 8) & 255), int(hex & 255), 255, true
		}

	case css_lexer.THash, css_lexer.THashID:
		switch len(text) {
		case 4:
			// "#123"
			if r, ok := hex(text[1]); ok {
				if g, ok := hex(text[2]); ok {
					if b, ok := hex(text[3]); ok {
						return hex2(r, r), hex2(g, g), hex2(b, b), 255, true
					}
				}
			}

		case 5:
			// "#1234"
			if r, ok := hex(text[1]); ok {
				if g, ok := hex(text[2]); ok {
					if b, ok := hex(text[3]); ok {
						if a, ok := hex(text[4]); ok {
							return hex2(r, r), hex2(g, g), hex2(b, b), hex2(a, a), true
						}
					}
				}
			}

		case 7:
			// "#112233"
			if r1, ok := hex(text[1]); ok {
				if r2, ok := hex(text[2]); ok {
					if g1, ok := hex(text[3]); ok {
						if g2, ok := hex(text[4]); ok {
							if b1, ok := hex(text[5]); ok {
								if b2, ok := hex(text[6]); ok {
									return hex2(r1, r2), hex2(g1, g2), hex2(b1, b2), 255, true
								}
							}
						}
					}
				}
			}

		case 9:
			// "#11223344"
			if r1, ok := hex(text[1]); ok {
				if r2, ok := hex(text[2]); ok {
					if g1, ok := hex(text[3]); ok {
						if g2, ok := hex(text[4]); ok {
							if b1, ok := hex(text[5]); ok {
								if b2, ok := hex(text[6]); ok {
									if a1, ok := hex(text[7]); ok {
										if a2, ok := hex(text[8]); ok {
											return hex2(r1, r2), hex2(g1, g2), hex2(b1, b2), hex2(a1, a2), true
										}
									}
								}
							}
						}
					}
				}
			}
		}

	case css_lexer.TFunction:
		switch text {
		case "rgb(", "rgba(":
			args := *token.Children
			var r, g, b, a css_ast.Token
			var ok bool

			switch len(args) {
			case 3:
				// "rgb(1 2 3)"
				r = args[0]
				g = args[1]
				b = args[2]
				ok = true

			case 5:
				// "rgba(1, 2, 3)"
				if args[1].Kind == css_lexer.TComma && args[3].Kind == css_lexer.TComma {
					r = args[0]
					g = args[2]
					b = args[4]
					ok = true
					break
				}

				// "rgb(1 2 3 / 4%)"
				if args[3].Kind == css_lexer.TDelimSlash {
					r = args[0]
					g = args[1]
					b = args[2]
					a = args[4]
					ok = true
				}

			case 7:
				// "rgb(1%, 2%, 3%, 4%)"
				if args[1].Kind == css_lexer.TComma && args[3].Kind == css_lexer.TComma && args[5].Kind == css_lexer.TComma {
					r = args[0]
					g = args[2]
					b = args[4]
					a = args[6]
					ok = true
				}
			}

			if ok {
				if r, ok := parseColorByte(r); ok {
					if g, ok := parseColorByte(g); ok {
						if b, ok := parseColorByte(b); ok {
							if a.Kind == css_lexer.T(0) {
								return r, g, b, 255, true
							} else if a, ok := parseColorByte(a); ok {
								return r, g, b, a, true
							}
						}
					}
				}
			}
		}
	}

	return 0, 0, 0, 0, false
}

func parseColorByte(token css_ast.Token) (i int, ok bool) {
	switch token.Kind {
	case css_lexer.TNumber:
		if f, err := strconv.ParseFloat(token.Text, 64); err == nil {
			i = int(math.Round(f * 255))
			ok = true
		}

	case css_lexer.TPercentage:
		if f, err := strconv.ParseFloat(token.PercentValue(), 64); err == nil {
			i = int(math.Round(f * (255.0 / 100.0)))
			ok = true
		}
	}

	if i < 0 {
		i = 0
	} else if i > 255 {
		i = 255
	}
	return i, ok
}

func (p *parser) mangleColor(token css_ast.Token) css_ast.Token {
	// Note: Do NOT remove color information from fully transparent colors.
	// Safari behaves differently than other browsers for color interpolation:
	// https://css-tricks.com/thing-know-gradients-transparent-black/

	if r, g, b, a, ok := parseColor(token); ok {
		rgba := (r << 24) | (g << 16) | (b << 8) | a
		if a == 255 {
			token.Children = nil
			if name, ok := shortColorName[rgba>>8]; ok {
				token.Kind = css_lexer.TIdent
				token.Text = name
			} else {
				token.Kind = css_lexer.THash
				if (r>>4) == (r&0xF) && (g>>4) == (g&0xF) && (b>>4) == (b&0xF) {
					token.Text = fmt.Sprintf("#%03x", ((r>>4)<<8)|((g>>4)<<4)|(b>>4))
				} else {
					token.Text = fmt.Sprintf("#%06x", rgba>>8)
				}
			}
		} else if !p.options.UnsupportedCSSFeatures.Has(compat.HexRGBA) {
			token.Children = nil
			token.Kind = css_lexer.THash
			if (r>>4) == (r&0xF) && (g>>4) == (g&0xF) && (b>>4) == (b&0xF) && (a>>4) == (a&0xF) {
				token.Text = fmt.Sprintf("#%04x", ((r>>4)<<12)|((g>>4)<<8)|((b>>4)<<4)|(a>>4))
			} else {
				token.Text = fmt.Sprintf("#%08x", rgba)
			}
		} else if !p.options.UnsupportedCSSFeatures.Has(compat.Modern_RGB_HSL) {
			token.Kind = css_lexer.TFunction
			token.Text = "rgb("
			token.Children = &[]css_ast.Token{
				{Kind: css_lexer.TNumber, Text: strconv.Itoa(r), HasWhitespaceAfter: true},
				{Kind: css_lexer.TNumber, Text: strconv.Itoa(g), HasWhitespaceAfter: true},
				{Kind: css_lexer.TNumber, Text: strconv.Itoa(b), HasWhitespaceAfter: true},
				{Kind: css_lexer.TDelimSlash, Text: "/", HasWhitespaceAfter: true},
				{Kind: css_lexer.TNumber, Text: floatToString(float64(a) / 255)},
			}
		} else {
			token.Kind = css_lexer.TFunction
			token.Text = "rgba("
			token.Children = &[]css_ast.Token{
				{Kind: css_lexer.TNumber, Text: strconv.Itoa(r)}, commaToken,
				{Kind: css_lexer.TNumber, Text: strconv.Itoa(g)}, commaToken,
				{Kind: css_lexer.TNumber, Text: strconv.Itoa(b)}, commaToken,
				{Kind: css_lexer.TNumber, Text: floatToString(float64(a) / 255)},
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
