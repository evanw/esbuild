package css_parser

import (
	"strings"

	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

func turnPercentIntoNumberIfShorter(t *css_ast.Token) {
	if t.Kind == css_lexer.TPercentage {
		if shifted, ok := shiftDot(t.PercentageValue(), -2); ok && len(shifted) < len(t.Text) {
			t.Kind = css_lexer.TNumber
			t.Text = shifted
		}
	}
}

// https://www.w3.org/TR/css-transforms-1/#two-d-transform-functions
// https://drafts.csswg.org/css-transforms-2/#transform-functions
func (p *parser) mangleTransforms(tokens []css_ast.Token) []css_ast.Token {
	for i := range tokens {
		if token := &tokens[i]; token.Kind == css_lexer.TFunction {
			if args := *token.Children; css_ast.TokensAreCommaSeparated(args) {
				n := len(args)

				switch strings.ToLower(token.Text) {
				////////////////////////////////////////////////////////////////////////////////
				// 2D transforms

				case "matrix":
					// specifies a 2D transformation in the form of a transformation
					// matrix of the six values a, b, c, d, e, f.
					if n == 11 {
						// | a c 0 e |
						// | b d 0 f |
						// | 0 0 1 0 |
						// | 0 0 0 1 |
						a, b, c, d, e, f := args[0], args[2], args[4], args[6], args[8], args[10]
						if b.IsZero() && c.IsZero() && e.IsZero() && f.IsZero() {
							// | a 0 0 0 |
							// | 0 d 0 0 |
							// | 0 0 1 0 |
							// | 0 0 0 1 |
							if a.EqualIgnoringWhitespace(d) {
								// "matrix(a, 0, 0, a, 0, 0)" => "scale(a)"
								token.Text = "scale"
								*token.Children = args[:1]
							} else if d.IsOne() {
								// "matrix(a, 0, 0, 1, 0, 0)" => "scaleX(a)"
								token.Text = "scaleX"
								*token.Children = args[:1]
							} else if a.IsOne() {
								// "matrix(1, 0, 0, d, 0, 0)" => "scaleY(d)"
								token.Text = "scaleY"
								*token.Children = args[6:7]
							} else {
								// "matrix(a, 0, 0, d, 0, 0)" => "scale(a, d)"
								token.Text = "scale"
								*token.Children = append(args[:2], d)
							}

							// Note: A "matrix" cannot be directly converted into a "translate"
							// because "translate" requires units while "matrix" requires no
							// units. I'm not sure exactly what the semantics are so I'm not
							// sure if you can just add "px" or not. Even if that did work,
							// you still couldn't substitute values containing "var()" since
							// units would still not be substituted in that case.
						}
					}

				case "translate":
					// specifies a 2D translation by the vector [tx, ty], where tx is the
					// first translation-value parameter and ty is the optional second
					// translation-value parameter. If <ty> is not provided, ty has zero
					// as a value.
					if n == 1 {
						args[0].TurnLengthOrPercentageIntoNumberIfZero()
					} else if n == 3 {
						tx, ty := &args[0], &args[2]
						tx.TurnLengthOrPercentageIntoNumberIfZero()
						ty.TurnLengthOrPercentageIntoNumberIfZero()
						if ty.IsZero() {
							// "translate(tx, 0)" => "translate(tx)"
							*token.Children = args[:1]
						} else if tx.IsZero() {
							// "translate(0, ty)" => "translateY(ty)"
							token.Text = "translateY"
							*token.Children = args[2:]
						}
					}

				case "translatex":
					// specifies a translation by the given amount in the X direction.
					if n == 1 {
						// "translateX(tx)" => "translate(tx)"
						token.Text = "translate"
						args[0].TurnLengthOrPercentageIntoNumberIfZero()
					}

				case "translatey":
					// specifies a translation by the given amount in the Y direction.
					if n == 1 {
						args[0].TurnLengthOrPercentageIntoNumberIfZero()
					}

				case "scale":
					// specifies a 2D scale operation by the [sx,sy] scaling vector
					// described by the 2 parameters. If the second parameter is not
					// provided, it takes a value equal to the first. For example,
					// scale(1, 1) would leave an element unchanged, while scale(2, 2)
					// would cause it to appear twice as long in both the X and Y axes,
					// or four times its typical geometric size.
					if n == 1 {
						turnPercentIntoNumberIfShorter(&args[0])
					} else if n == 3 {
						sx, sy := &args[0], &args[2]
						turnPercentIntoNumberIfShorter(sx)
						turnPercentIntoNumberIfShorter(sy)
						if sx.EqualIgnoringWhitespace(*sy) {
							// "scale(s, s)" => "scale(s)"
							*token.Children = args[:1]
						} else if sy.IsOne() {
							// "scale(s, 1)" => "scaleX(s)"
							token.Text = "scaleX"
							*token.Children = args[:1]
						} else if sx.IsOne() {
							// "scale(1, s)" => "scaleY(s)"
							token.Text = "scaleY"
							*token.Children = args[2:]
						}
					}

				case "scalex":
					// specifies a 2D scale operation using the [sx,1] scaling vector,
					// where sx is given as the parameter.
					if n == 1 {
						turnPercentIntoNumberIfShorter(&args[0])
					}

				case "scaley":
					// specifies a 2D scale operation using the [1,sy] scaling vector,
					// where sy is given as the parameter.
					if n == 1 {
						turnPercentIntoNumberIfShorter(&args[0])
					}

				case "rotate":
					// specifies a 2D rotation by the angle specified in the parameter
					// about the origin of the element, as defined by the
					// transform-origin property. For example, rotate(90deg) would
					// cause elements to appear rotated one-quarter of a turn in the
					// clockwise direction.
					if n == 1 {
						args[0].TurnLengthIntoNumberIfZero()
					}

				// Note: This is considered a 2D transform even though it's specified
				// in terms of a 3D transform because it doesn't trigger Safari's 3D
				// transform bugs.
				case "rotatez":
					// same as rotate3d(0, 0, 1, <angle>), which is a 3d transform
					// equivalent to the 2d transform rotate(<angle>).
					if n == 1 {
						// "rotateZ(angle)" => "rotate(angle)"
						token.Text = "rotate"
						args[0].TurnLengthIntoNumberIfZero()
					}

				case "skew":
					// specifies a 2D skew by [ax,ay] for X and Y. If the second
					// parameter is not provided, it has a zero value.
					if n == 1 {
						args[0].TurnLengthIntoNumberIfZero()
					} else if n == 3 {
						ax, ay := &args[0], &args[2]
						ax.TurnLengthIntoNumberIfZero()
						ay.TurnLengthIntoNumberIfZero()
						if ay.IsZero() {
							// "skew(ax, 0)" => "skew(ax)"
							*token.Children = args[:1]
						}
					}

				case "skewx":
					// specifies a 2D skew transformation along the X axis by the given
					// angle.
					if n == 1 {
						// "skewX(ax)" => "skew(ax)"
						token.Text = "skew"
						args[0].TurnLengthIntoNumberIfZero()
					}

				case "skewy":
					// specifies a 2D skew transformation along the Y axis by the given
					// angle.
					if n == 1 {
						args[0].TurnLengthIntoNumberIfZero()
					}

					////////////////////////////////////////////////////////////////////////////////
					// 3D transforms

					// Note: Safari has a bug where 3D transforms render differently than
					// other transforms. This means we should not minify a 3D transform
					// into a 2D transform or it will cause a rendering difference in
					// Safari.

				case "matrix3d":
					// specifies a 3D transformation as a 4x4 homogeneous matrix of 16
					// values in column-major order.
					if n == 31 {
						// | m0 m4 m8  m12 |
						// | m1 m5 m9  m13 |
						// | m2 m6 m10 m14 |
						// | m3 m7 m11 m15 |
						mask := uint32(0)
						for i := 0; i < 16; i++ {
							if arg := args[i*2]; arg.IsZero() {
								mask |= 1 << i
							} else if arg.IsOne() {
								mask |= (1 << 16) << i
							}
						}
						const onlyScale = 0b1000_0000_0000_0000_0111_1011_1101_1110
						if (mask & onlyScale) == onlyScale {
							// | m0 0  0   0 |
							// | 0  m5 0   0 |
							// | 0  0  m10 0 |
							// | 0  0  0   1 |
							sx, sy := args[0], args[10]
							if sx.IsOne() && sy.IsOne() {
								token.Text = "scaleZ"
								*token.Children = args[20:21]
							} else {
								token.Text = "scale3d"
								*token.Children = append(append(args[0:2], args[10:12]...), args[20])
							}
						}

						// Note: A "matrix3d" cannot be directly converted into a "translate3d"
						// because "translate3d" requires units while "matrix3d" requires no
						// units. I'm not sure exactly what the semantics are so I'm not
						// sure if you can just add "px" or not. Even if that did work,
						// you still couldn't substitute values containing "var()" since
						// units would still not be substituted in that case.
					}

				case "translate3d":
					// specifies a 3D translation by the vector [tx,ty,tz], with tx,
					// ty and tz being the first, second and third translation-value
					// parameters respectively.
					if n == 5 {
						tx, ty, tz := &args[0], &args[2], &args[4]
						tx.TurnLengthOrPercentageIntoNumberIfZero()
						ty.TurnLengthOrPercentageIntoNumberIfZero()
						tz.TurnLengthIntoNumberIfZero()
						if tx.IsZero() && ty.IsZero() {
							// "translate3d(0, 0, tz)" => "translateZ(tz)"
							token.Text = "translateZ"
							*token.Children = args[4:]
						}
					}

				case "translatez":
					// specifies a 3D translation by the vector [0,0,tz] with the given
					// amount in the Z direction.
					if n == 1 {
						args[0].TurnLengthIntoNumberIfZero()
					}

				case "scale3d":
					// specifies a 3D scale operation by the [sx,sy,sz] scaling vector
					// described by the 3 parameters.
					if n == 5 {
						sx, sy, sz := &args[0], &args[2], &args[4]
						turnPercentIntoNumberIfShorter(sx)
						turnPercentIntoNumberIfShorter(sy)
						turnPercentIntoNumberIfShorter(sz)
						if sx.IsOne() && sy.IsOne() {
							// "scale3d(1, 1, sz)" => "scaleZ(sz)"
							token.Text = "scaleZ"
							*token.Children = args[4:]
						}
					}

				case "scalez":
					// specifies a 3D scale operation using the [1,1,sz] scaling vector,
					// where sz is given as the parameter.
					if n == 1 {
						turnPercentIntoNumberIfShorter(&args[0])
					}

				case "rotate3d":
					// specifies a 3D rotation by the angle specified in last parameter
					// about the [x,y,z] direction vector described by the first three
					// parameters. A direction vector that cannot be normalized, such as
					// [0,0,0], will cause the rotation to not be applied.
					if n == 7 {
						x, y, z, angle := &args[0], &args[2], &args[4], &args[6]
						angle.TurnLengthIntoNumberIfZero()
						if x.IsOne() && y.IsZero() && z.IsZero() {
							// "rotate3d(1, 0, 0, angle)" => "rotateX(angle)"
							token.Text = "rotateX"
							*token.Children = args[6:]
						} else if x.IsZero() && y.IsOne() && z.IsZero() {
							// "rotate3d(0, 1, 0, angle)" => "rotateY(angle)"
							token.Text = "rotateY"
							*token.Children = args[6:]
						}
					}

				case "rotatex":
					// same as rotate3d(1, 0, 0, <angle>).
					if n == 1 {
						args[0].TurnLengthIntoNumberIfZero()
					}

				case "rotatey":
					// same as rotate3d(0, 1, 0, <angle>).
					if n == 1 {
						args[0].TurnLengthIntoNumberIfZero()
					}

				case "perspective":
					// specifies a perspective projection matrix. This matrix scales
					// points in X and Y based on their Z value, scaling points with
					// positive Z values away from the origin, and those with negative Z
					// values towards the origin. Points on the z=0 plane are unchanged.
					// The parameter represents the distance of the z=0 plane from the
					// viewer.
					if n == 1 {
						args[0].TurnLengthIntoNumberIfZero()
					}
				}

				// Trim whitespace at the ends
				if args := *token.Children; len(args) > 0 {
					args[0].Whitespace &= ^css_ast.WhitespaceBefore
					args[len(args)-1].Whitespace &= ^css_ast.WhitespaceAfter
				}
			}
		}
	}

	return tokens
}
