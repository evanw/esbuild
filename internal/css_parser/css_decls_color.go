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

// These names are shorter than their hex codes
var shortColorName = map[uint32]string{
	0x000080ff: "navy",
	0x008000ff: "green",
	0x008080ff: "teal",
	0x4b0082ff: "indigo",
	0x800000ff: "maroon",
	0x800080ff: "purple",
	0x808000ff: "olive",
	0x808080ff: "gray",
	0xa0522dff: "sienna",
	0xa52a2aff: "brown",
	0xc0c0c0ff: "silver",
	0xcd853fff: "peru",
	0xd2b48cff: "tan",
	0xda70d6ff: "orchid",
	0xdda0ddff: "plum",
	0xee82eeff: "violet",
	0xf0e68cff: "khaki",
	0xf0ffffff: "azure",
	0xf5deb3ff: "wheat",
	0xf5f5dcff: "beige",
	0xfa8072ff: "salmon",
	0xfaf0e6ff: "linen",
	0xff0000ff: "red",
	0xff6347ff: "tomato",
	0xff7f50ff: "coral",
	0xffa500ff: "orange",
	0xffc0cbff: "pink",
	0xffd700ff: "gold",
	0xffe4c4ff: "bisque",
	0xfffafaff: "snow",
	0xfffff0ff: "ivory",
}

var colorNameToHex = map[string]uint32{
	"black":                0x000000ff,
	"silver":               0xc0c0c0ff,
	"gray":                 0x808080ff,
	"white":                0xffffffff,
	"maroon":               0x800000ff,
	"red":                  0xff0000ff,
	"purple":               0x800080ff,
	"fuchsia":              0xff00ffff,
	"green":                0x008000ff,
	"lime":                 0x00ff00ff,
	"olive":                0x808000ff,
	"yellow":               0xffff00ff,
	"navy":                 0x000080ff,
	"blue":                 0x0000ffff,
	"teal":                 0x008080ff,
	"aqua":                 0x00ffffff,
	"orange":               0xffa500ff,
	"aliceblue":            0xf0f8ffff,
	"antiquewhite":         0xfaebd7ff,
	"aquamarine":           0x7fffd4ff,
	"azure":                0xf0ffffff,
	"beige":                0xf5f5dcff,
	"bisque":               0xffe4c4ff,
	"blanchedalmond":       0xffebcdff,
	"blueviolet":           0x8a2be2ff,
	"brown":                0xa52a2aff,
	"burlywood":            0xdeb887ff,
	"cadetblue":            0x5f9ea0ff,
	"chartreuse":           0x7fff00ff,
	"chocolate":            0xd2691eff,
	"coral":                0xff7f50ff,
	"cornflowerblue":       0x6495edff,
	"cornsilk":             0xfff8dcff,
	"crimson":              0xdc143cff,
	"cyan":                 0x00ffffff,
	"darkblue":             0x00008bff,
	"darkcyan":             0x008b8bff,
	"darkgoldenrod":        0xb8860bff,
	"darkgray":             0xa9a9a9ff,
	"darkgreen":            0x006400ff,
	"darkgrey":             0xa9a9a9ff,
	"darkkhaki":            0xbdb76bff,
	"darkmagenta":          0x8b008bff,
	"darkolivegreen":       0x556b2fff,
	"darkorange":           0xff8c00ff,
	"darkorchid":           0x9932ccff,
	"darkred":              0x8b0000ff,
	"darksalmon":           0xe9967aff,
	"darkseagreen":         0x8fbc8fff,
	"darkslateblue":        0x483d8bff,
	"darkslategray":        0x2f4f4fff,
	"darkslategrey":        0x2f4f4fff,
	"darkturquoise":        0x00ced1ff,
	"darkviolet":           0x9400d3ff,
	"deeppink":             0xff1493ff,
	"deepskyblue":          0x00bfffff,
	"dimgray":              0x696969ff,
	"dimgrey":              0x696969ff,
	"dodgerblue":           0x1e90ffff,
	"firebrick":            0xb22222ff,
	"floralwhite":          0xfffaf0ff,
	"forestgreen":          0x228b22ff,
	"gainsboro":            0xdcdcdcff,
	"ghostwhite":           0xf8f8ffff,
	"gold":                 0xffd700ff,
	"goldenrod":            0xdaa520ff,
	"greenyellow":          0xadff2fff,
	"grey":                 0x808080ff,
	"honeydew":             0xf0fff0ff,
	"hotpink":              0xff69b4ff,
	"indianred":            0xcd5c5cff,
	"indigo":               0x4b0082ff,
	"ivory":                0xfffff0ff,
	"khaki":                0xf0e68cff,
	"lavender":             0xe6e6faff,
	"lavenderblush":        0xfff0f5ff,
	"lawngreen":            0x7cfc00ff,
	"lemonchiffon":         0xfffacdff,
	"lightblue":            0xadd8e6ff,
	"lightcoral":           0xf08080ff,
	"lightcyan":            0xe0ffffff,
	"lightgoldenrodyellow": 0xfafad2ff,
	"lightgray":            0xd3d3d3ff,
	"lightgreen":           0x90ee90ff,
	"lightgrey":            0xd3d3d3ff,
	"lightpink":            0xffb6c1ff,
	"lightsalmon":          0xffa07aff,
	"lightseagreen":        0x20b2aaff,
	"lightskyblue":         0x87cefaff,
	"lightslategray":       0x778899ff,
	"lightslategrey":       0x778899ff,
	"lightsteelblue":       0xb0c4deff,
	"lightyellow":          0xffffe0ff,
	"limegreen":            0x32cd32ff,
	"linen":                0xfaf0e6ff,
	"magenta":              0xff00ffff,
	"mediumaquamarine":     0x66cdaaff,
	"mediumblue":           0x0000cdff,
	"mediumorchid":         0xba55d3ff,
	"mediumpurple":         0x9370dbff,
	"mediumseagreen":       0x3cb371ff,
	"mediumslateblue":      0x7b68eeff,
	"mediumspringgreen":    0x00fa9aff,
	"mediumturquoise":      0x48d1ccff,
	"mediumvioletred":      0xc71585ff,
	"midnightblue":         0x191970ff,
	"mintcream":            0xf5fffaff,
	"mistyrose":            0xffe4e1ff,
	"moccasin":             0xffe4b5ff,
	"navajowhite":          0xffdeadff,
	"oldlace":              0xfdf5e6ff,
	"olivedrab":            0x6b8e23ff,
	"orangered":            0xff4500ff,
	"orchid":               0xda70d6ff,
	"palegoldenrod":        0xeee8aaff,
	"palegreen":            0x98fb98ff,
	"paleturquoise":        0xafeeeeff,
	"palevioletred":        0xdb7093ff,
	"papayawhip":           0xffefd5ff,
	"peachpuff":            0xffdab9ff,
	"peru":                 0xcd853fff,
	"pink":                 0xffc0cbff,
	"plum":                 0xdda0ddff,
	"powderblue":           0xb0e0e6ff,
	"rosybrown":            0xbc8f8fff,
	"royalblue":            0x4169e1ff,
	"saddlebrown":          0x8b4513ff,
	"salmon":               0xfa8072ff,
	"sandybrown":           0xf4a460ff,
	"seagreen":             0x2e8b57ff,
	"seashell":             0xfff5eeff,
	"sienna":               0xa0522dff,
	"skyblue":              0x87ceebff,
	"slateblue":            0x6a5acdff,
	"slategray":            0x708090ff,
	"slategrey":            0x708090ff,
	"snow":                 0xfffafaff,
	"springgreen":          0x00ff7fff,
	"steelblue":            0x4682b4ff,
	"tan":                  0xd2b48cff,
	"thistle":              0xd8bfd8ff,
	"tomato":               0xff6347ff,
	"turquoise":            0x40e0d0ff,
	"violet":               0xee82eeff,
	"wheat":                0xf5deb3ff,
	"whitesmoke":           0xf5f5f5ff,
	"yellowgreen":          0x9acd32ff,
	"rebeccapurple":        0x663399ff,
}

func parseHex(text string) (uint32, bool) {
	hex := uint32(0)
	for _, c := range text {
		hex <<= 4
		switch {
		case c >= '0' && c <= '9':
			hex |= uint32(c) - '0'
		case c >= 'a' && c <= 'f':
			hex |= uint32(c) - ('a' - 10)
		case c >= 'A' && c <= 'F':
			hex |= uint32(c) - ('A' - 10)
		default:
			return 0, false
		}
	}
	return hex, true
}

// 0xAABBCCDD => 0xABCD
func compactHex(v uint32) uint32 {
	return ((v & 0x0FF00000) >> 12) | ((v & 0x00000FF0) >> 4)
}

// 0xABCD => 0xAABBCCDD
func expandHex(v uint32) uint32 {
	return ((v & 0xF000) << 16) | ((v & 0xFF00) << 12) | ((v & 0x0FF0) << 8) | ((v & 0x00FF) << 4) | (v & 0x000F)
}

func hexR(v uint32) int { return int(v >> 24) }
func hexG(v uint32) int { return int((v >> 16) & 255) }
func hexB(v uint32) int { return int((v >> 8) & 255) }
func hexA(v uint32) int { return int(v & 255) }

func floatToStringForColor(a float64) string {
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
	switch token.Kind {
	case css_lexer.TNumber:
		if value, err := strconv.ParseFloat(token.Text, 64); err == nil {
			return value, true
		}

	case css_lexer.TDimension:
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
			token.Text = floatToStringForColor(value / 100.0)
		}
	}
	return token
}

// Convert newer color syntax to older color syntax for older browsers
func (p *parser) lowerColor(token css_ast.Token) css_ast.Token {
	text := token.Text

	switch token.Kind {
	case css_lexer.THash:
		if p.options.UnsupportedCSSFeatures.Has(compat.HexRGBA) {
			switch len(text) {
			case 4:
				// "#1234" => "rgba(1, 2, 3, 0.004)"
				if hex, ok := parseHex(text); ok {
					hex = expandHex(hex)
					token.Kind = css_lexer.TFunction
					token.Text = "rgba"
					commaToken := p.commaToken()
					token.Children = &[]css_ast.Token{
						{Kind: css_lexer.TNumber, Text: strconv.Itoa(hexR(hex))}, commaToken,
						{Kind: css_lexer.TNumber, Text: strconv.Itoa(hexG(hex))}, commaToken,
						{Kind: css_lexer.TNumber, Text: strconv.Itoa(hexB(hex))}, commaToken,
						{Kind: css_lexer.TNumber, Text: floatToStringForColor(float64(hexA(hex)) / 255)},
					}
				}

			case 8:
				// "#12345678" => "rgba(18, 52, 86, 0.47)"
				if hex, ok := parseHex(text); ok {
					token.Kind = css_lexer.TFunction
					token.Text = "rgba"
					commaToken := p.commaToken()
					token.Children = &[]css_ast.Token{
						{Kind: css_lexer.TNumber, Text: strconv.Itoa(hexR(hex))}, commaToken,
						{Kind: css_lexer.TNumber, Text: strconv.Itoa(hexG(hex))}, commaToken,
						{Kind: css_lexer.TNumber, Text: strconv.Itoa(hexB(hex))}, commaToken,
						{Kind: css_lexer.TNumber, Text: floatToStringForColor(float64(hexA(hex)) / 255)},
					}
				}
			}
		}

	case css_lexer.TIdent:
		if text == "rebeccapurple" && p.options.UnsupportedCSSFeatures.Has(compat.RebeccaPurple) {
			token.Kind = css_lexer.THash
			token.Text = "663399"
		}

	case css_lexer.TFunction:
		switch text {
		case "rgb", "rgba", "hsl", "hsla":
			if p.options.UnsupportedCSSFeatures.Has(compat.Modern_RGB_HSL) {
				args := *token.Children
				removeAlpha := false
				addAlpha := false

				// "hsl(1deg, 2%, 3%)" => "hsl(1, 2%, 3%)"
				if (text == "hsl" || text == "hsla") && len(args) > 0 {
					if degrees, ok := degreesForAngle(args[0]); ok {
						args[0].Kind = css_lexer.TNumber
						args[0].Text = floatToStringForColor(degrees)
					}
				}

				// These check for "IsNumeric" to reject "var()" since a single "var()"
				// can substitute for multiple tokens and that messes up pattern matching
				switch len(args) {
				case 3:
					// "rgba(1 2 3)" => "rgb(1, 2, 3)"
					// "hsla(1 2% 3%)" => "hsl(1, 2%, 3%)"
					if args[0].Kind.IsNumeric() && args[1].Kind.IsNumeric() && args[2].Kind.IsNumeric() {
						removeAlpha = true
						args[0].Whitespace = 0
						args[1].Whitespace = 0
						commaToken := p.commaToken()
						token.Children = &[]css_ast.Token{
							args[0], commaToken,
							args[1], commaToken,
							args[2],
						}
					}

				case 5:
					// "rgba(1, 2, 3)" => "rgb(1, 2, 3)"
					// "hsla(1, 2%, 3%)" => "hsl(1%, 2%, 3%)"
					if args[0].Kind.IsNumeric() && args[1].Kind == css_lexer.TComma &&
						args[2].Kind.IsNumeric() && args[3].Kind == css_lexer.TComma &&
						args[4].Kind.IsNumeric() {
						removeAlpha = true
						break
					}

					// "rgb(1 2 3 / 4%)" => "rgba(1, 2, 3, 0.04)"
					// "hsl(1 2% 3% / 4%)" => "hsla(1, 2%, 3%, 0.04)"
					if args[0].Kind.IsNumeric() && args[1].Kind.IsNumeric() && args[2].Kind.IsNumeric() &&
						args[3].Kind == css_lexer.TDelimSlash && args[4].Kind.IsNumeric() {
						addAlpha = true
						args[0].Whitespace = 0
						args[1].Whitespace = 0
						args[2].Whitespace = 0
						commaToken := p.commaToken()
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
					if args[0].Kind.IsNumeric() && args[1].Kind == css_lexer.TComma &&
						args[2].Kind.IsNumeric() && args[3].Kind == css_lexer.TComma &&
						args[4].Kind.IsNumeric() && args[5].Kind == css_lexer.TComma &&
						args[6].Kind.IsNumeric() {
						addAlpha = true
						args[6] = lowerAlphaPercentageToNumber(args[6])
					}
				}

				if removeAlpha {
					if text == "rgba" {
						token.Text = "rgb"
					} else if text == "hsla" {
						token.Text = "hsl"
					}
				} else if addAlpha {
					if text == "rgb" {
						token.Text = "rgba"
					} else if text == "hsl" {
						token.Text = "hsla"
					}
				}
			}
		}
	}

	return token
}

func parseColor(token css_ast.Token) (uint32, bool) {
	text := token.Text

	switch token.Kind {
	case css_lexer.TIdent:
		if hex, ok := colorNameToHex[strings.ToLower(text)]; ok {
			return hex, true
		}

	case css_lexer.THash:
		switch len(text) {
		case 3:
			// "#123"
			if hex, ok := parseHex(text); ok {
				return (expandHex(hex) << 8) | 0xFF, true
			}

		case 4:
			// "#1234"
			if hex, ok := parseHex(text); ok {
				return expandHex(hex), true
			}

		case 6:
			// "#112233"
			if hex, ok := parseHex(text); ok {
				return (hex << 8) | 0xFF, true
			}

		case 8:
			// "#11223344"
			if hex, ok := parseHex(text); ok {
				return hex, true
			}
		}

	case css_lexer.TFunction:
		switch text {
		case "rgb", "rgba":
			args := *token.Children
			var r, g, b, a css_ast.Token

			switch len(args) {
			case 3:
				// "rgb(1 2 3)"
				r, g, b = args[0], args[1], args[2]

			case 5:
				// "rgba(1, 2, 3)"
				if args[1].Kind == css_lexer.TComma && args[3].Kind == css_lexer.TComma {
					r, g, b = args[0], args[2], args[4]
					break
				}

				// "rgb(1 2 3 / 4%)"
				if args[3].Kind == css_lexer.TDelimSlash {
					r, g, b, a = args[0], args[1], args[2], args[4]
				}

			case 7:
				// "rgb(1%, 2%, 3%, 4%)"
				if args[1].Kind == css_lexer.TComma && args[3].Kind == css_lexer.TComma && args[5].Kind == css_lexer.TComma {
					r, g, b, a = args[0], args[2], args[4], args[6]
				}
			}

			if r, ok := parseColorByte(r, 1); ok {
				if g, ok := parseColorByte(g, 1); ok {
					if b, ok := parseColorByte(b, 1); ok {
						if a, ok := parseAlphaByte(a); ok {
							return uint32((r << 24) | (g << 16) | (b << 8) | a), true
						}
					}
				}
			}

		case "hsl", "hsla":
			args := *token.Children
			var h, s, l, a css_ast.Token

			switch len(args) {
			case 3:
				// "hsl(1 2 3)"
				h, s, l = args[0], args[1], args[2]

			case 5:
				// "hsla(1, 2, 3)"
				if args[1].Kind == css_lexer.TComma && args[3].Kind == css_lexer.TComma {
					h, s, l = args[0], args[2], args[4]
					break
				}

				// "hsl(1 2 3 / 4%)"
				if args[3].Kind == css_lexer.TDelimSlash {
					h, s, l, a = args[0], args[1], args[2], args[4]
				}

			case 7:
				// "hsl(1%, 2%, 3%, 4%)"
				if args[1].Kind == css_lexer.TComma && args[3].Kind == css_lexer.TComma && args[5].Kind == css_lexer.TComma {
					h, s, l, a = args[0], args[2], args[4], args[6]
				}
			}

			// Convert from HSL to RGB. The algorithm is from the section
			// "Converting HSL colors to sRGB colors" in the specification.
			if h, ok := degreesForAngle(h); ok {
				if s, ok := s.FractionForPercentage(); ok {
					if l, ok := l.FractionForPercentage(); ok {
						if a, ok := parseAlphaByte(a); ok {
							h /= 360.0
							var t2 float64
							if l <= 0.5 {
								t2 = l * (s + 1)
							} else {
								t2 = l + s - (l * s)
							}
							t1 := l*2 - t2
							r := hueToRgb(t1, t2, h+1.0/3.0)
							g := hueToRgb(t1, t2, h)
							b := hueToRgb(t1, t2, h-1.0/3.0)
							return uint32((r << 24) | (g << 16) | (b << 8) | a), true
						}
					}
				}
			}
		}
	}

	return 0, false
}

func hueToRgb(t1 float64, t2 float64, hue float64) uint32 {
	hue -= math.Floor(hue)
	hue *= 6.0
	var f float64
	if hue < 1 {
		f = (t2-t1)*hue + t1
	} else if hue < 3 {
		f = t2
	} else if hue < 4 {
		f = (t2-t1)*(4-hue) + t1
	} else {
		f = t1
	}
	i := int(math.Round(f * 255))
	if i < 0 {
		i = 0
	} else if i > 255 {
		i = 255
	}
	return uint32(i)
}

func parseAlphaByte(token css_ast.Token) (uint32, bool) {
	if token.Kind == css_lexer.T(0) {
		return 255, true
	}
	return parseColorByte(token, 255)
}

func parseColorByte(token css_ast.Token, scale float64) (uint32, bool) {
	var i int
	var ok bool

	switch token.Kind {
	case css_lexer.TNumber:
		if f, err := strconv.ParseFloat(token.Text, 64); err == nil {
			i = int(math.Round(f * scale))
			ok = true
		}

	case css_lexer.TPercentage:
		if f, err := strconv.ParseFloat(token.PercentageValue(), 64); err == nil {
			i = int(math.Round(f * (255.0 / 100.0)))
			ok = true
		}
	}

	if i < 0 {
		i = 0
	} else if i > 255 {
		i = 255
	}
	return uint32(i), ok
}

func (p *parser) mangleColor(token css_ast.Token, hex uint32) css_ast.Token {
	// Note: Do NOT remove color information from fully transparent colors.
	// Safari behaves differently than other browsers for color interpolation:
	// https://css-tricks.com/thing-know-gradients-transparent-black/

	if hexA(hex) == 255 {
		token.Children = nil
		if name, ok := shortColorName[hex]; ok {
			token.Kind = css_lexer.TIdent
			token.Text = name
		} else {
			token.Kind = css_lexer.THash
			hex >>= 8
			compact := compactHex(hex)
			if hex == expandHex(compact) {
				token.Text = fmt.Sprintf("%03x", compact)
			} else {
				token.Text = fmt.Sprintf("%06x", hex)
			}
		}
	} else if !p.options.UnsupportedCSSFeatures.Has(compat.HexRGBA) {
		token.Children = nil
		token.Kind = css_lexer.THash
		compact := compactHex(hex)
		if hex == expandHex(compact) {
			token.Text = fmt.Sprintf("%04x", compact)
		} else {
			token.Text = fmt.Sprintf("%08x", hex)
		}
	} else {
		token.Kind = css_lexer.TFunction
		token.Text = "rgba"
		commaToken := p.commaToken()
		index := hexA(hex) * 4
		alpha := alphaFractionTable[index : index+4]
		if space := strings.IndexByte(alpha, ' '); space != -1 {
			alpha = alpha[:space]
		}
		token.Children = &[]css_ast.Token{
			{Kind: css_lexer.TNumber, Text: strconv.Itoa(hexR(hex))}, commaToken,
			{Kind: css_lexer.TNumber, Text: strconv.Itoa(hexG(hex))}, commaToken,
			{Kind: css_lexer.TNumber, Text: strconv.Itoa(hexB(hex))}, commaToken,
			{Kind: css_lexer.TNumber, Text: alpha},
		}
	}

	return token
}

// Every four characters in this table is the fraction for that index
const alphaFractionTable string = "" +
	"0   .004.008.01 .016.02 .024.027.03 .035.04 .043.047.05 .055.06 " +
	".063.067.07 .075.08 .082.086.09 .094.098.1  .106.11 .114.118.12 " +
	".125.13 .133.137.14 .145.15 .153.157.16 .165.17 .173.176.18 .184" +
	".19 .192.196.2  .204.208.21 .216.22 .224.227.23 .235.24 .243.247" +
	".25 .255.26 .263.267.27 .275.28 .282.286.29 .294.298.3  .306.31 " +
	".314.318.32 .325.33 .333.337.34 .345.35 .353.357.36 .365.37 .373" +
	".376.38 .384.39 .392.396.4  .404.408.41 .416.42 .424.427.43 .435" +
	".44 .443.447.45 .455.46 .463.467.47 .475.48 .482.486.49 .494.498" +
	".5  .506.51 .514.518.52 .525.53 .533.537.54 .545.55 .553.557.56 " +
	".565.57 .573.576.58 .584.59 .592.596.6  .604.608.61 .616.62 .624" +
	".627.63 .635.64 .643.647.65 .655.66 .663.667.67 .675.68 .682.686" +
	".69 .694.698.7  .706.71 .714.718.72 .725.73 .733.737.74 .745.75 " +
	".753.757.76 .765.77 .773.776.78 .784.79 .792.796.8  .804.808.81 " +
	".816.82 .824.827.83 .835.84 .843.847.85 .855.86 .863.867.87 .875" +
	".88 .882.886.89 .894.898.9  .906.91 .914.918.92 .925.93 .933.937" +
	".94 .945.95 .953.957.96 .965.97 .973.976.98 .984.99 .992.9961   "
