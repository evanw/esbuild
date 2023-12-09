package css_parser

import (
	"strings"

	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

type gradientKind uint8

const (
	linearGradient gradientKind = iota
	radialGradient
	conicGradient
)

type parsedGradient struct {
	initialTokens []css_ast.Token
	colorStops    []colorStop
	kind          gradientKind
	repeating     bool
}

type colorStop struct {
	positions []css_ast.Token
	color     css_ast.Token
	hint      css_ast.Token // Absent if "hint.Kind == css_lexer.T(0)"
}

func parseGradient(token css_ast.Token) (gradient parsedGradient, success bool) {
	if token.Kind != css_lexer.TFunction {
		return
	}

	switch strings.ToLower(token.Text) {
	case "linear-gradient":
		gradient.kind = linearGradient

	case "radial-gradient":
		gradient.kind = radialGradient

	case "conic-gradient":
		gradient.kind = conicGradient

	case "repeating-linear-gradient":
		gradient.kind = linearGradient
		gradient.repeating = true

	case "repeating-radial-gradient":
		gradient.kind = radialGradient
		gradient.repeating = true

	case "repeating-conic-gradient":
		gradient.kind = conicGradient
		gradient.repeating = true

	default:
		return
	}

	// Bail if any token is a "var()" since it may introduce commas
	tokens := *token.Children
	for _, t := range tokens {
		if t.Kind == css_lexer.TFunction && strings.EqualFold(t.Text, "var") {
			return
		}
	}

	// Try to strip the initial tokens
	if len(tokens) > 0 && !looksLikeColor(tokens[0]) {
		i := 0
		for i < len(tokens) && tokens[i].Kind != css_lexer.TComma {
			i++
		}
		gradient.initialTokens = tokens[:i]
		if i < len(tokens) {
			tokens = tokens[i+1:]
		} else {
			tokens = nil
		}
	}

	// Try to parse the color stops
	for len(tokens) > 0 {
		// Parse the color
		color := tokens[0]
		if !looksLikeColor(color) {
			return
		}
		tokens = tokens[1:]

		// Parse up to two positions
		var positions []css_ast.Token
		for len(positions) < 2 && len(tokens) > 0 {
			position := tokens[0]
			if position.Kind.IsNumeric() || (position.Kind == css_lexer.TFunction && strings.EqualFold(position.Text, "calc")) {
				positions = append(positions, position)
			} else {
				break
			}
			tokens = tokens[1:]
		}

		// Parse the comma
		var hint css_ast.Token
		if len(tokens) > 0 {
			if tokens[0].Kind != css_lexer.TComma {
				return
			}
			tokens = tokens[1:]
			if len(tokens) == 0 {
				return
			}

			// Parse the hint, if any
			if len(tokens) > 0 && tokens[0].Kind.IsNumeric() {
				hint = tokens[0]
				tokens = tokens[1:]

				// Followed by a mandatory comma
				if len(tokens) == 0 || tokens[0].Kind != css_lexer.TComma {
					return
				}
				tokens = tokens[1:]
			}
		}

		// Add the color stop
		gradient.colorStops = append(gradient.colorStops, colorStop{
			color:     color,
			positions: positions,
			hint:      hint,
		})
	}

	success = true
	return
}

func (p *parser) generateGradient(token css_ast.Token, gradient parsedGradient) css_ast.Token {
	var children []css_ast.Token
	commaToken := p.commaToken(token.Loc)

	children = append(children, gradient.initialTokens...)
	for _, stop := range gradient.colorStops {
		if len(children) > 0 {
			children = append(children, commaToken)
		}
		children = append(children, stop.color)
		children = append(children, stop.positions...)
		if stop.hint.Kind != css_lexer.T(0) {
			children = append(children, commaToken, stop.hint)
		}
	}

	token.Children = &children
	return token
}

func (p *parser) lowerAndMinifyGradient(token css_ast.Token, wouldClipColor *bool) css_ast.Token {
	gradient, ok := parseGradient(token)
	if !ok {
		return token
	}

	// Lower all colors in the gradient stop
	for i, stop := range gradient.colorStops {
		gradient.colorStops[i].color = p.lowerAndMinifyColor(stop.color, wouldClipColor)
	}

	if p.options.unsupportedCSSFeatures.Has(compat.GradientDoublePosition) {
		// Replace double positions with duplicated single positions
		for _, stop := range gradient.colorStops {
			if len(stop.positions) > 1 {
				gradient.colorStops = switchToSinglePositions(gradient.colorStops)
				break
			}
		}
	} else if p.options.minifySyntax {
		// Replace duplicated single positions with double positions
		for i, stop := range gradient.colorStops {
			if i > 0 && len(stop.positions) == 1 {
				if prev := gradient.colorStops[i-1]; len(prev.positions) == 1 && prev.hint.Kind == css_lexer.T(0) &&
					css_ast.TokensEqual([]css_ast.Token{prev.color}, []css_ast.Token{stop.color}, nil) {
					gradient.colorStops = switchToDoublePositions(gradient.colorStops)
					break
				}
			}
		}
	}

	return p.generateGradient(token, gradient)
}

func switchToSinglePositions(double []colorStop) (single []colorStop) {
	for _, stop := range double {
		for i := range stop.positions {
			stop.positions[i].Whitespace = css_ast.WhitespaceBefore
		}
		for len(stop.positions) > 1 {
			clone := stop
			clone.positions = stop.positions[:1]
			clone.hint = css_ast.Token{}
			single = append(single, clone)
			stop.positions = stop.positions[1:]
		}
		single = append(single, stop)
	}
	return
}

func switchToDoublePositions(single []colorStop) (double []colorStop) {
	for i := 0; i < len(single); i++ {
		stop := single[i]
		if i+1 < len(single) && len(stop.positions) == 1 && stop.hint.Kind == css_lexer.T(0) {
			if next := single[i+1]; len(next.positions) == 1 &&
				css_ast.TokensEqual([]css_ast.Token{stop.color}, []css_ast.Token{next.color}, nil) {
				double = append(double, colorStop{
					color:     stop.color,
					positions: []css_ast.Token{stop.positions[0], next.positions[0]},
					hint:      next.hint,
				})
				i++
				continue
			}
		}
		double = append(double, stop)
	}
	return
}
