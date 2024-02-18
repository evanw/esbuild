package css_parser

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/helpers"
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
func (p *parser) lowerAndMinifyColor(token css_ast.Token, wouldClipColor *bool) css_ast.Token {
	text := token.Text

	switch token.Kind {
	case css_lexer.THash:
		if p.options.unsupportedCSSFeatures.Has(compat.HexRGBA) {
			switch len(text) {
			case 4:
				// "#1234" => "rgba(1, 2, 3, 0.004)"
				if hex, ok := parseHex(text); ok {
					hex = expandHex(hex)
					return p.tryToGenerateColor(token, parsedColor{hex: hex}, nil)
				}

			case 8:
				// "#12345678" => "rgba(18, 52, 86, 0.47)"
				if hex, ok := parseHex(text); ok {
					return p.tryToGenerateColor(token, parsedColor{hex: hex}, nil)
				}
			}
		}

	case css_lexer.TIdent:
		if p.options.unsupportedCSSFeatures.Has(compat.RebeccaPurple) && strings.EqualFold(text, "rebeccapurple") {
			token.Kind = css_lexer.THash
			token.Text = "663399"
		}

	case css_lexer.TFunction:
		switch strings.ToLower(text) {
		case "rgb", "rgba", "hsl", "hsla":
			if p.options.unsupportedCSSFeatures.Has(compat.Modern_RGB_HSL) {
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
						commaToken := p.commaToken(token.Loc)
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
						commaToken := p.commaToken(token.Loc)
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
					if strings.EqualFold(text, "rgba") {
						token.Text = "rgb"
					} else if strings.EqualFold(text, "hsla") {
						token.Text = "hsl"
					}
				} else if addAlpha {
					if strings.EqualFold(text, "rgb") {
						token.Text = "rgba"
					} else if strings.EqualFold(text, "hsl") {
						token.Text = "hsla"
					}
				}
			}

		case "hwb":
			if p.options.unsupportedCSSFeatures.Has(compat.HWB) {
				if color, ok := parseColor(token); ok {
					return p.tryToGenerateColor(token, color, wouldClipColor)
				}
			}

		case "color", "lab", "lch", "oklab", "oklch":
			if p.options.unsupportedCSSFeatures.Has(compat.ColorFunctions) {
				if color, ok := parseColor(token); ok {
					return p.tryToGenerateColor(token, color, wouldClipColor)
				}
			}
		}
	}

	// When minifying, try to parse the color and print it back out. This minifies
	// the color because we always print it out using the shortest encoding.
	if p.options.minifySyntax {
		if hex, ok := parseColor(token); ok {
			token = p.tryToGenerateColor(token, hex, wouldClipColor)
		}
	}

	return token
}

type parsedColor struct {
	x, y, z       F64    // color if hasColorSpace == true
	hex           uint32 // color and alpha if hasColorSpace == false, alpha if hasColorSpace == true
	hasColorSpace bool
}

func looksLikeColor(token css_ast.Token) bool {
	switch token.Kind {
	case css_lexer.TIdent:
		if _, ok := colorNameToHex[strings.ToLower(token.Text)]; ok {
			return true
		}

	case css_lexer.THash:
		switch len(token.Text) {
		case 3, 4, 6, 8:
			if _, ok := parseHex(token.Text); ok {
				return true
			}
		}

	case css_lexer.TFunction:
		switch strings.ToLower(token.Text) {
		case
			"color-mix",
			"color",
			"hsl",
			"hsla",
			"hwb",
			"lab",
			"lch",
			"oklab",
			"oklch",
			"rgb",
			"rgba":
			return true
		}
	}

	return false
}

func parseColor(token css_ast.Token) (parsedColor, bool) {
	text := token.Text

	switch token.Kind {
	case css_lexer.TIdent:
		if hex, ok := colorNameToHex[strings.ToLower(text)]; ok {
			return parsedColor{hex: hex}, true
		}

	case css_lexer.THash:
		switch len(text) {
		case 3:
			// "#123"
			if hex, ok := parseHex(text); ok {
				return parsedColor{hex: (expandHex(hex) << 8) | 0xFF}, true
			}

		case 4:
			// "#1234"
			if hex, ok := parseHex(text); ok {
				return parsedColor{hex: expandHex(hex)}, true
			}

		case 6:
			// "#112233"
			if hex, ok := parseHex(text); ok {
				return parsedColor{hex: (hex << 8) | 0xFF}, true
			}

		case 8:
			// "#11223344"
			if hex, ok := parseHex(text); ok {
				return parsedColor{hex: hex}, true
			}
		}

	case css_lexer.TFunction:
		lowerText := strings.ToLower(text)
		switch lowerText {
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
							return parsedColor{hex: (r << 24) | (g << 16) | (b << 8) | a}, true
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

			// HSL => RGB
			if h, ok := degreesForAngle(h); ok {
				if s, ok := s.ClampedFractionForPercentage(); ok {
					if l, ok := l.ClampedFractionForPercentage(); ok {
						if a, ok := parseAlphaByte(a); ok {
							r, g, b := hslToRgb(helpers.NewF64(h), helpers.NewF64(s), helpers.NewF64(l))
							return parsedColor{hex: packRGBA(r, g, b, a)}, true
						}
					}
				}
			}

		case "hwb":
			args := *token.Children
			var h, s, l, a css_ast.Token

			switch len(args) {
			case 3:
				// "hwb(1 2 3)"
				h, s, l = args[0], args[1], args[2]

			case 5:
				// "hwb(1 2 3 / 4%)"
				if args[3].Kind == css_lexer.TDelimSlash {
					h, s, l, a = args[0], args[1], args[2], args[4]
				}
			}

			// HWB => RGB
			if h, ok := degreesForAngle(h); ok {
				if white, ok := s.ClampedFractionForPercentage(); ok {
					if black, ok := l.ClampedFractionForPercentage(); ok {
						if a, ok := parseAlphaByte(a); ok {
							r, g, b := hwbToRgb(helpers.NewF64(h), helpers.NewF64(white), helpers.NewF64(black))
							return parsedColor{hex: packRGBA(r, g, b, a)}, true
						}
					}
				}
			}

		case "color":
			args := *token.Children
			var colorSpace, alpha css_ast.Token

			switch len(args) {
			case 4:
				// "color(xyz 1 2 3)"
				colorSpace = args[0]

			case 6:
				// "color(xyz 1 2 3 / 50%)"
				if args[4].Kind == css_lexer.TDelimSlash {
					colorSpace, alpha = args[0], args[5]
				}
			}

			if colorSpace.Kind == css_lexer.TIdent {
				if v0, ok := args[1].NumberOrFractionForPercentage(1, 0); ok {
					if v1, ok := args[2].NumberOrFractionForPercentage(1, 0); ok {
						if v2, ok := args[3].NumberOrFractionForPercentage(1, 0); ok {
							if a, ok := parseAlphaByte(alpha); ok {
								v0, v1, v2 := helpers.NewF64(v0), helpers.NewF64(v1), helpers.NewF64(v2)
								switch strings.ToLower(colorSpace.Text) {
								case "a98-rgb":
									r, g, b := lin_a98rgb(v0, v1, v2)
									x, y, z := lin_a98rgb_to_xyz(r, g, b)
									return parsedColor{hasColorSpace: true, x: x, y: y, z: z, hex: a}, true

								case "display-p3":
									r, g, b := lin_p3(v0, v1, v2)
									x, y, z := lin_p3_to_xyz(r, g, b)
									return parsedColor{hasColorSpace: true, x: x, y: y, z: z, hex: a}, true

								case "prophoto-rgb":
									r, g, b := lin_prophoto(v0, v1, v2)
									x, y, z := lin_prophoto_to_xyz(r, g, b)
									x, y, z = d50_to_d65(x, y, z)
									return parsedColor{hasColorSpace: true, x: x, y: y, z: z, hex: a}, true

								case "rec2020":
									r, g, b := lin_2020(v0, v1, v2)
									x, y, z := lin_2020_to_xyz(r, g, b)
									return parsedColor{hasColorSpace: true, x: x, y: y, z: z, hex: a}, true

								case "srgb":
									r, g, b := lin_srgb(v0, v1, v2)
									x, y, z := lin_srgb_to_xyz(r, g, b)
									return parsedColor{hasColorSpace: true, x: x, y: y, z: z, hex: a}, true

								case "srgb-linear":
									x, y, z := lin_srgb_to_xyz(v0, v1, v2)
									return parsedColor{hasColorSpace: true, x: x, y: y, z: z, hex: a}, true

								case "xyz", "xyz-d65":
									return parsedColor{hasColorSpace: true, x: v0, y: v1, z: v2, hex: a}, true

								case "xyz-d50":
									x, y, z := d50_to_d65(v0, v1, v2)
									return parsedColor{hasColorSpace: true, x: x, y: y, z: z, hex: a}, true
								}
							}
						}
					}
				}
			}

		case "lab", "lch", "oklab", "oklch":
			args := *token.Children
			var v0, v1, v2, alpha css_ast.Token

			switch len(args) {
			case 3:
				// "lab(1 2 3)"
				v0, v1, v2 = args[0], args[1], args[2]

			case 5:
				// "lab(1 2 3 / 50%)"
				if args[3].Kind == css_lexer.TDelimSlash {
					v0, v1, v2, alpha = args[0], args[1], args[2], args[4]
				}
			}

			if v0.Kind != css_lexer.T(0) {
				if alpha, ok := parseAlphaByte(alpha); ok {
					switch lowerText {
					case "lab":
						if v0, ok := v0.NumberOrFractionForPercentage(100, 0); ok {
							if v1, ok := v1.NumberOrFractionForPercentage(125, css_ast.AllowAnyPercentage); ok {
								if v2, ok := v2.NumberOrFractionForPercentage(125, css_ast.AllowAnyPercentage); ok {
									v0, v1, v2 := helpers.NewF64(v0), helpers.NewF64(v1), helpers.NewF64(v2)
									x, y, z := lab_to_xyz(v0, v1, v2)
									x, y, z = d50_to_d65(x, y, z)
									return parsedColor{hasColorSpace: true, x: x, y: y, z: z, hex: alpha}, true
								}
							}
						}

					case "lch":
						if v0, ok := v0.NumberOrFractionForPercentage(100, 0); ok {
							if v1, ok := v1.NumberOrFractionForPercentage(125, css_ast.AllowPercentageAbove100); ok {
								if v2, ok := degreesForAngle(v2); ok {
									v0, v1, v2 := helpers.NewF64(v0), helpers.NewF64(v1), helpers.NewF64(v2)
									l, a, b := lch_to_lab(v0, v1, v2)
									x, y, z := lab_to_xyz(l, a, b)
									x, y, z = d50_to_d65(x, y, z)
									return parsedColor{hasColorSpace: true, x: x, y: y, z: z, hex: alpha}, true
								}
							}
						}

					case "oklab":
						if v0, ok := v0.NumberOrFractionForPercentage(1, 0); ok {
							if v1, ok := v1.NumberOrFractionForPercentage(0.4, css_ast.AllowAnyPercentage); ok {
								if v2, ok := v2.NumberOrFractionForPercentage(0.4, css_ast.AllowAnyPercentage); ok {
									v0, v1, v2 := helpers.NewF64(v0), helpers.NewF64(v1), helpers.NewF64(v2)
									x, y, z := oklab_to_xyz(v0, v1, v2)
									return parsedColor{hasColorSpace: true, x: x, y: y, z: z, hex: alpha}, true
								}
							}
						}

					case "oklch":
						if v0, ok := v0.NumberOrFractionForPercentage(1, 0); ok {
							if v1, ok := v1.NumberOrFractionForPercentage(0.4, css_ast.AllowPercentageAbove100); ok {
								if v2, ok := degreesForAngle(v2); ok {
									v0, v1, v2 := helpers.NewF64(v0), helpers.NewF64(v1), helpers.NewF64(v2)
									l, a, b := oklch_to_oklab(v0, v1, v2)
									x, y, z := oklab_to_xyz(l, a, b)
									return parsedColor{hasColorSpace: true, x: x, y: y, z: z, hex: alpha}, true
								}
							}
						}
					}
				}
			}
		}
	}

	return parsedColor{}, false
}

// Reference: https://drafts.csswg.org/css-color/#hwb-to-rgb
func hwbToRgb(hue F64, white F64, black F64) (r F64, g F64, b F64) {
	if white.Add(black).Value() >= 1 {
		gray := white.Div(white.Add(black))
		return gray, gray, gray
	}
	delta := white.Add(black).Neg().AddConst(1)
	r, g, b = hslToRgb(hue, helpers.NewF64(1), helpers.NewF64(0.5))
	r = delta.Mul(r).Add(white)
	g = delta.Mul(g).Add(white)
	b = delta.Mul(b).Add(white)
	return
}

// Reference https://drafts.csswg.org/css-color/#hsl-to-rgb
func hslToRgb(hue F64, sat F64, light F64) (r F64, g F64, b F64) {
	hue = hue.DivConst(360.0)
	var t2 F64
	if light.Value() <= 0.5 {
		t2 = sat.AddConst(1).Mul(light)
	} else {
		t2 = light.Add(sat).Sub(light.Mul(sat))
	}
	t1 := light.MulConst(2).Sub(t2)
	r = hueToRgb(t1, t2, hue.AddConst(1.0/3.0))
	g = hueToRgb(t1, t2, hue)
	b = hueToRgb(t1, t2, hue.SubConst(1.0/3.0))
	return
}

func hueToRgb(t1 F64, t2 F64, hue F64) F64 {
	hue = hue.Sub(hue.Floor())
	hue = hue.MulConst(6)
	var f F64
	if hue.Value() < 1 {
		f = helpers.Lerp(t1, t2, hue)
	} else if hue.Value() < 3 {
		f = t2
	} else if hue.Value() < 4 {
		f = helpers.Lerp(t1, t2, hue.Neg().AddConst(4))
	} else {
		f = t1
	}
	return f
}

func packRGBA(rf F64, gf F64, bf F64, a uint32) uint32 {
	r := floatToByte(rf.Value())
	g := floatToByte(gf.Value())
	b := floatToByte(bf.Value())
	return (r << 24) | (g << 16) | (b << 8) | a
}

func floatToByte(f float64) uint32 {
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

func tryToConvertToHexWithoutClipping(x F64, y F64, z F64, a uint32) (uint32, bool) {
	r, g, b := gam_srgb(xyz_to_lin_srgb(x, y, z))
	if r.Value() < -0.5/255 || r.Value() > 255.5/255 ||
		g.Value() < -0.5/255 || g.Value() > 255.5/255 ||
		b.Value() < -0.5/255 || b.Value() > 255.5/255 {
		return 0, false
	}
	return packRGBA(r, g, b, a), true
}

func (p *parser) tryToGenerateColor(token css_ast.Token, color parsedColor, wouldClipColor *bool) css_ast.Token {
	// Note: Do NOT remove color information from fully transparent colors.
	// Safari behaves differently than other browsers for color interpolation:
	// https://css-tricks.com/thing-know-gradients-transparent-black/

	// Attempt to convert other color spaces to sRGB, and only continue if the
	// result (rounded to the nearest byte) will be in the 0-to-1 sRGB range
	var hex uint32
	if !color.hasColorSpace {
		hex = color.hex
	} else if result, ok := tryToConvertToHexWithoutClipping(color.x, color.y, color.z, color.hex); ok {
		hex = result
	} else if wouldClipColor != nil {
		*wouldClipColor = true
		return token
	} else {
		r, g, b := gamut_mapping_xyz_to_srgb(color.x, color.y, color.z)
		hex = packRGBA(r, g, b, color.hex)
	}

	if hexA(hex) == 255 {
		token.Children = nil
		if name, ok := shortColorName[hex]; ok && p.options.minifySyntax {
			token.Kind = css_lexer.TIdent
			token.Text = name
		} else {
			token.Kind = css_lexer.THash
			hex >>= 8
			compact := compactHex(hex)
			if p.options.minifySyntax && hex == expandHex(compact) {
				token.Text = fmt.Sprintf("%03x", compact)
			} else {
				token.Text = fmt.Sprintf("%06x", hex)
			}
		}
	} else if !p.options.unsupportedCSSFeatures.Has(compat.HexRGBA) {
		token.Children = nil
		token.Kind = css_lexer.THash
		compact := compactHex(hex)
		if p.options.minifySyntax && hex == expandHex(compact) {
			token.Text = fmt.Sprintf("%04x", compact)
		} else {
			token.Text = fmt.Sprintf("%08x", hex)
		}
	} else {
		token.Kind = css_lexer.TFunction
		token.Text = "rgba"
		commaToken := p.commaToken(token.Loc)
		index := hexA(hex) * 4
		alpha := alphaFractionTable[index : index+4]
		if space := strings.IndexByte(alpha, ' '); space != -1 {
			alpha = alpha[:space]
		}
		token.Children = &[]css_ast.Token{
			{Loc: token.Loc, Kind: css_lexer.TNumber, Text: strconv.Itoa(hexR(hex))}, commaToken,
			{Loc: token.Loc, Kind: css_lexer.TNumber, Text: strconv.Itoa(hexG(hex))}, commaToken,
			{Loc: token.Loc, Kind: css_lexer.TNumber, Text: strconv.Itoa(hexB(hex))}, commaToken,
			{Loc: token.Loc, Kind: css_lexer.TNumber, Text: alpha},
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
