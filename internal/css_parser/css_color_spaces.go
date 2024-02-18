package css_parser

import (
	"math"

	"github.com/evanw/esbuild/internal/helpers"
)

// Wrap float64 math to avoid compiler optimizations that break determinism
type F64 = helpers.F64

// Reference: https://drafts.csswg.org/css-color/#color-conversion-code

type colorSpace uint8

const (
	colorSpace_a98_rgb colorSpace = iota
	colorSpace_display_p3
	colorSpace_hsl
	colorSpace_hwb
	colorSpace_lab
	colorSpace_lch
	colorSpace_oklab
	colorSpace_oklch
	colorSpace_prophoto_rgb
	colorSpace_rec2020
	colorSpace_srgb
	colorSpace_srgb_linear
	colorSpace_xyz
	colorSpace_xyz_d50
	colorSpace_xyz_d65
)

func (colorSpace colorSpace) isPolar() bool {
	switch colorSpace {
	case colorSpace_hsl, colorSpace_hwb, colorSpace_lch, colorSpace_oklch:
		return true
	}
	return false
}

type hueMethod uint8

const (
	shorterHue hueMethod = iota
	longerHue
	increasingHue
	decreasingHue
)

func lin_srgb(r F64, g F64, b F64) (F64, F64, F64) {
	f := func(val F64) F64 {
		if abs := val.Abs(); abs.Value() < 0.04045 {
			return val.DivConst(12.92)
		} else {
			return abs.AddConst(0.055).DivConst(1.055).PowConst(2.4).WithSignFrom(val)
		}
	}
	return f(r), f(g), f(b)
}

func gam_srgb(r F64, g F64, b F64) (F64, F64, F64) {
	f := func(val F64) F64 {
		if abs := val.Abs(); abs.Value() > 0.0031308 {
			return abs.PowConst(1 / 2.4).MulConst(1.055).SubConst(0.055).WithSignFrom(val)
		} else {
			return val.MulConst(12.92)
		}
	}
	return f(r), f(g), f(b)
}

func lin_srgb_to_xyz(r F64, g F64, b F64) (F64, F64, F64) {
	M := [9]float64{
		506752.0 / 1228815, 87881.0 / 245763, 12673.0 / 70218,
		87098.0 / 409605, 175762.0 / 245763, 12673.0 / 175545,
		7918.0 / 409605, 87881.0 / 737289, 1001167.0 / 1053270,
	}
	return multiplyMatrices(M, r, g, b)
}

func xyz_to_lin_srgb(x F64, y F64, z F64) (F64, F64, F64) {
	M := [9]float64{
		12831.0 / 3959, -329.0 / 214, -1974.0 / 3959,
		-851781.0 / 878810, 1648619.0 / 878810, 36519.0 / 878810,
		705.0 / 12673, -2585.0 / 12673, 705.0 / 667,
	}
	return multiplyMatrices(M, x, y, z)
}

func lin_p3(r F64, g F64, b F64) (F64, F64, F64) {
	return lin_srgb(r, g, b)
}

func gam_p3(r F64, g F64, b F64) (F64, F64, F64) {
	return gam_srgb(r, g, b)
}

func lin_p3_to_xyz(r F64, g F64, b F64) (F64, F64, F64) {
	M := [9]float64{
		608311.0 / 1250200, 189793.0 / 714400, 198249.0 / 1000160,
		35783.0 / 156275, 247089.0 / 357200, 198249.0 / 2500400,
		0.0 / 1, 32229.0 / 714400, 5220557.0 / 5000800,
	}
	return multiplyMatrices(M, r, g, b)
}

func xyz_to_lin_p3(x F64, y F64, z F64) (F64, F64, F64) {
	M := [9]float64{
		446124.0 / 178915, -333277.0 / 357830, -72051.0 / 178915,
		-14852.0 / 17905, 63121.0 / 35810, 423.0 / 17905,
		11844.0 / 330415, -50337.0 / 660830, 316169.0 / 330415,
	}
	return multiplyMatrices(M, x, y, z)
}

func lin_prophoto(r F64, g F64, b F64) (F64, F64, F64) {
	f := func(val F64) F64 {
		const Et2 = 16.0 / 512
		if abs := val.Abs(); abs.Value() <= Et2 {
			return val.DivConst(16)
		} else {
			return abs.PowConst(1.8).WithSignFrom(val)
		}
	}
	return f(r), f(g), f(b)
}

func gam_prophoto(r F64, g F64, b F64) (F64, F64, F64) {
	f := func(val F64) F64 {
		const Et = 1.0 / 512
		if abs := val.Abs(); abs.Value() >= Et {
			return abs.PowConst(1 / 1.8).WithSignFrom(val)
		} else {
			return val.MulConst(16)
		}
	}
	return f(r), f(g), f(b)
}

func lin_prophoto_to_xyz(r F64, g F64, b F64) (F64, F64, F64) {
	M := [9]float64{
		0.7977604896723027, 0.13518583717574031, 0.0313493495815248,
		0.2880711282292934, 0.7118432178101014, 0.00008565396060525902,
		0.0, 0.0, 0.8251046025104601,
	}
	return multiplyMatrices(M, r, g, b)
}

func xyz_to_lin_prophoto(x F64, y F64, z F64) (F64, F64, F64) {
	M := [9]float64{
		1.3457989731028281, -0.25558010007997534, -0.05110628506753401,
		-0.5446224939028347, 1.5082327413132781, 0.02053603239147973,
		0.0, 0.0, 1.2119675456389454,
	}
	return multiplyMatrices(M, x, y, z)
}

func lin_a98rgb(r F64, g F64, b F64) (F64, F64, F64) {
	f := func(val F64) F64 {
		return val.Abs().PowConst(563.0 / 256).WithSignFrom(val)
	}
	return f(r), f(g), f(b)
}

func gam_a98rgb(r F64, g F64, b F64) (F64, F64, F64) {
	f := func(val F64) F64 {
		return val.Abs().PowConst(256.0 / 563).WithSignFrom(val)
	}
	return f(r), f(g), f(b)
}

func lin_a98rgb_to_xyz(r F64, g F64, b F64) (F64, F64, F64) {
	M := [9]float64{
		573536.0 / 994567, 263643.0 / 1420810, 187206.0 / 994567,
		591459.0 / 1989134, 6239551.0 / 9945670, 374412.0 / 4972835,
		53769.0 / 1989134, 351524.0 / 4972835, 4929758.0 / 4972835,
	}
	return multiplyMatrices(M, r, g, b)
}

func xyz_to_lin_a98rgb(x F64, y F64, z F64) (F64, F64, F64) {
	M := [9]float64{
		1829569.0 / 896150, -506331.0 / 896150, -308931.0 / 896150,
		-851781.0 / 878810, 1648619.0 / 878810, 36519.0 / 878810,
		16779.0 / 1248040, -147721.0 / 1248040, 1266979.0 / 1248040,
	}
	return multiplyMatrices(M, x, y, z)
}

func lin_2020(r F64, g F64, b F64) (F64, F64, F64) {
	f := func(val F64) F64 {
		const α = 1.09929682680944
		const β = 0.018053968510807
		if abs := val.Abs(); abs.Value() < β*4.5 {
			return val.DivConst(4.5)
		} else {
			return abs.AddConst(α - 1).DivConst(α).PowConst(1 / 0.45).WithSignFrom(val)
		}
	}
	return f(r), f(g), f(b)
}

func gam_2020(r F64, g F64, b F64) (F64, F64, F64) {
	f := func(val F64) F64 {
		const α = 1.09929682680944
		const β = 0.018053968510807
		if abs := val.Abs(); abs.Value() > β {
			return abs.PowConst(0.45).MulConst(α).SubConst(α - 1).WithSignFrom(val)
		} else {
			return val.MulConst(4.5)
		}
	}
	return f(r), f(g), f(b)
}

func lin_2020_to_xyz(r F64, g F64, b F64) (F64, F64, F64) {
	var M = [9]float64{
		63426534.0 / 99577255, 20160776.0 / 139408157, 47086771.0 / 278816314,
		26158966.0 / 99577255, 472592308.0 / 697040785, 8267143.0 / 139408157,
		0.0 / 1, 19567812.0 / 697040785, 295819943.0 / 278816314,
	}
	return multiplyMatrices(M, r, g, b)
}

func xyz_to_lin_2020(x F64, y F64, z F64) (F64, F64, F64) {
	M := [9]float64{
		30757411.0 / 17917100, -6372589.0 / 17917100, -4539589.0 / 17917100,
		-19765991.0 / 29648200, 47925759.0 / 29648200, 467509.0 / 29648200,
		792561.0 / 44930125, -1921689.0 / 44930125, 42328811.0 / 44930125,
	}
	return multiplyMatrices(M, x, y, z)
}

func d65_to_d50(x F64, y F64, z F64) (F64, F64, F64) {
	M := [9]float64{
		1.0479297925449969, 0.022946870601609652, -0.05019226628920524,
		0.02962780877005599, 0.9904344267538799, -0.017073799063418826,
		-0.009243040646204504, 0.015055191490298152, 0.7518742814281371,
	}
	return multiplyMatrices(M, x, y, z)
}

func d50_to_d65(x F64, y F64, z F64) (F64, F64, F64) {
	M := [9]float64{
		0.955473421488075, -0.02309845494876471, 0.06325924320057072,
		-0.0283697093338637, 1.0099953980813041, 0.021041441191917323,
		0.012314014864481998, -0.020507649298898964, 1.330365926242124,
	}
	return multiplyMatrices(M, x, y, z)
}

const d50_x = 0.3457 / 0.3585
const d50_z = (1.0 - 0.3457 - 0.3585) / 0.3585

func xyz_to_lab(x F64, y F64, z F64) (F64, F64, F64) {
	const ε = 216.0 / 24389
	const κ = 24389.0 / 27

	x = x.DivConst(d50_x)
	z = z.DivConst(d50_z)

	var f0, f1, f2 F64
	if x.Value() > ε {
		f0 = x.Cbrt()
	} else {
		f0 = x.MulConst(κ).AddConst(16).DivConst(116)
	}
	if y.Value() > ε {
		f1 = y.Cbrt()
	} else {
		f1 = y.MulConst(κ).AddConst(16).DivConst(116)
	}
	if z.Value() > ε {
		f2 = z.Cbrt()
	} else {
		f2 = z.MulConst(κ).AddConst(16).DivConst(116)
	}

	return f1.MulConst(116).SubConst(16),
		f0.Sub(f1).MulConst(500),
		f1.Sub(f2).MulConst(200)
}

func lab_to_xyz(l F64, a F64, b F64) (x F64, y F64, z F64) {
	const κ = 24389.0 / 27
	const ε = 216.0 / 24389

	f1 := l.AddConst(16).DivConst(116)
	f0 := a.DivConst(500).Add(f1)
	f2 := f1.Sub(b.DivConst(200))

	f0_3 := f0.Cubed()
	f2_3 := f2.Cubed()

	if f0_3.Value() > ε {
		x = f0_3
	} else {
		x = f0.MulConst(116).SubConst(16).DivConst(κ)
	}
	if l.Value() > κ*ε {
		y = l.AddConst(16).DivConst(116)
		y = y.Cubed()
	} else {
		y = l.DivConst(κ)
	}
	if f2_3.Value() > ε {
		z = f2_3
	} else {
		z = f2.MulConst(116).SubConst(16).DivConst(κ)
	}

	return x.MulConst(d50_x), y, z.MulConst(d50_z)
}

func lab_to_lch(l F64, a F64, b F64) (F64, F64, F64) {
	hue := b.Atan2(a).MulConst(180 / math.Pi)
	if hue.Value() < 0 {
		hue = hue.AddConst(360)
	}
	return l,
		a.Squared().Add(b.Squared()).Sqrt(),
		hue
}

func lch_to_lab(l F64, c F64, h F64) (F64, F64, F64) {
	return l,
		h.MulConst(math.Pi / 180).Cos().Mul(c),
		h.MulConst(math.Pi / 180).Sin().Mul(c)
}

func xyz_to_oklab(x F64, y F64, z F64) (F64, F64, F64) {
	XYZtoLMS := [9]float64{
		0.8190224432164319, 0.3619062562801221, -0.12887378261216414,
		0.0329836671980271, 0.9292868468965546, 0.03614466816999844,
		0.048177199566046255, 0.26423952494422764, 0.6335478258136937,
	}
	LMStoOKLab := [9]float64{
		0.2104542553, 0.7936177850, -0.0040720468,
		1.9779984951, -2.4285922050, 0.4505937099,
		0.0259040371, 0.7827717662, -0.8086757660,
	}
	l, m, s := multiplyMatrices(XYZtoLMS, x, y, z)
	return multiplyMatrices(LMStoOKLab, l.Cbrt(), m.Cbrt(), s.Cbrt())
}

func oklab_to_xyz(l F64, a F64, b F64) (F64, F64, F64) {
	LMStoXYZ := [9]float64{
		1.2268798733741557, -0.5578149965554813, 0.28139105017721583,
		-0.04057576262431372, 1.1122868293970594, -0.07171106666151701,
		-0.07637294974672142, -0.4214933239627914, 1.5869240244272418,
	}
	OKLabtoLMS := [9]float64{
		0.99999999845051981432, 0.39633779217376785678, 0.21580375806075880339,
		1.0000000088817607767, -0.1055613423236563494, -0.063854174771705903402,
		1.0000000546724109177, -0.089484182094965759684, -1.2914855378640917399,
	}
	l, m, s := multiplyMatrices(OKLabtoLMS, l, a, b)
	return multiplyMatrices(LMStoXYZ, l.Cubed(), m.Cubed(), s.Cubed())
}

func oklab_to_oklch(l F64, a F64, b F64) (F64, F64, F64) {
	return lab_to_lch(l, a, b)
}

func oklch_to_oklab(l F64, c F64, h F64) (F64, F64, F64) {
	return lch_to_lab(l, c, h)
}

func multiplyMatrices(A [9]float64, b0 F64, b1 F64, b2 F64) (F64, F64, F64) {
	return b0.MulConst(A[0]).Add(b1.MulConst(A[1])).Add(b2.MulConst(A[2])),
		b0.MulConst(A[3]).Add(b1.MulConst(A[4])).Add(b2.MulConst(A[5])),
		b0.MulConst(A[6]).Add(b1.MulConst(A[7])).Add(b2.MulConst(A[8]))
}

func delta_eok(L1 F64, a1 F64, b1 F64, L2 F64, a2 F64, b2 F64) F64 {
	ΔL_sq := L1.Sub(L2).Squared()
	Δa_sq := a1.Sub(a2).Squared()
	Δb_sq := b1.Sub(b2).Squared()
	return ΔL_sq.Add(Δa_sq).Add(Δb_sq).Sqrt()
}

func gamut_mapping_xyz_to_srgb(x F64, y F64, z F64) (F64, F64, F64) {
	origin_l, origin_c, origin_h := oklab_to_oklch(xyz_to_oklab(x, y, z))

	if origin_l.Value() >= 1 || origin_l.Value() <= 0 {
		return origin_l, origin_l, origin_l
	}

	oklch_to_srgb := func(l F64, c F64, h F64) (F64, F64, F64) {
		l, a, b := oklch_to_oklab(l, c, h)
		x, y, z := oklab_to_xyz(l, a, b)
		r, g, b := xyz_to_lin_srgb(x, y, z)
		return gam_srgb(r, g, b)
	}

	srgb_to_oklab := func(r F64, g F64, b F64) (F64, F64, F64) {
		r, g, b = lin_srgb(r, g, b)
		x, y, z := lin_srgb_to_xyz(r, g, b)
		return xyz_to_oklab(x, y, z)
	}

	inGamut := func(r F64, g F64, b F64) bool {
		return r.Value() >= 0 && r.Value() <= 1 &&
			g.Value() >= 0 && g.Value() <= 1 &&
			b.Value() >= 0 && b.Value() <= 1
	}

	r, g, b := oklch_to_srgb(origin_l, origin_c, origin_h)
	if inGamut(r, g, b) {
		return r, g, b
	}

	const JND = 0.02
	const epsilon = 0.0001
	min := helpers.NewF64(0.0)
	max := origin_c

	clip := func(x F64) F64 {
		if x.Value() < 0 {
			return helpers.NewF64(0)
		}
		if x.Value() > 1 {
			return helpers.NewF64(1)
		}
		return x
	}

	for max.Sub(min).Value() > epsilon {
		chroma := min.Add(max).DivConst(2)
		origin_c = chroma

		r, g, b = oklch_to_srgb(origin_l, origin_c, origin_h)
		if inGamut(r, g, b) {
			min = chroma
			continue
		}

		clipped_r, clipped_g, clipped_b := clip(r), clip(g), clip(b)
		L1, a1, b1 := srgb_to_oklab(clipped_r, clipped_b, clipped_g)
		L2, a2, b2 := srgb_to_oklab(r, g, b)
		E := delta_eok(L1, a1, b1, L2, a2, b2)
		if E.Value() < JND {
			return clipped_r, clipped_g, clipped_b
		}

		max = chroma
	}

	return r, g, b
}

func hsl_to_rgb(hue F64, sat F64, light F64) (F64, F64, F64) {
	hue = hue.DivConst(360)
	hue = hue.Sub(hue.Floor())
	hue = hue.MulConst(360)

	sat = sat.DivConst(100)
	light = light.DivConst(100)

	f := func(n float64) F64 {
		k := hue.DivConst(30).AddConst(n)
		k = k.DivConst(12)
		k = k.Sub(k.Floor())
		k = k.MulConst(12)
		a := helpers.Min2(light, light.Neg().AddConst(1)).Mul(sat)
		return light.Sub(helpers.Max2(helpers.NewF64(-1), helpers.Min3(k.SubConst(3), k.Neg().AddConst(9), helpers.NewF64(1))).Mul(a))
	}

	return f(0), f(8), f(4)
}

func rgb_to_hsl(red F64, green F64, blue F64) (F64, F64, F64) {
	max := helpers.Max3(red, green, blue)
	min := helpers.Min3(red, green, blue)
	hue, sat, light := helpers.NewF64(math.NaN()), helpers.NewF64(0.0), min.Add(max).DivConst(2)
	d := max.Sub(min)

	if d.Value() != 0 {
		if div := helpers.Min2(light, light.Neg().AddConst(1)); div.Value() != 0 {
			sat = max.Sub(light).Div(div)
		}

		switch max {
		case red:
			hue = green.Sub(blue).Div(d)
			if green.Value() < blue.Value() {
				hue = hue.AddConst(6)
			}
		case green:
			hue = blue.Sub(red).Div(d).AddConst(2)
		case blue:
			hue = red.Sub(green).Div(d).AddConst(4)
		}

		hue = hue.MulConst(60)
	}

	return hue, sat.MulConst(100), light.MulConst(100)
}

func hwb_to_rgb(hue F64, white F64, black F64) (F64, F64, F64) {
	white = white.DivConst(100)
	black = black.DivConst(100)
	if white.Add(black).Value() >= 1 {
		gray := white.Div(white.Add(black))
		return gray, gray, gray
	}
	delta := white.Add(black).Neg().AddConst(1)
	r, g, b := hsl_to_rgb(hue, helpers.NewF64(100), helpers.NewF64(50))
	r = delta.Mul(r).Add(white)
	g = delta.Mul(g).Add(white)
	b = delta.Mul(b).Add(white)
	return r, g, b
}

func rgb_to_hwb(red F64, green F64, blue F64) (F64, F64, F64) {
	h, _, _ := rgb_to_hsl(red, green, blue)
	white := helpers.Min3(red, green, blue)
	black := helpers.Max3(red, green, blue).Neg().AddConst(1)
	return h, white.MulConst(100), black.MulConst(100)
}

func xyz_to_colorSpace(x F64, y F64, z F64, colorSpace colorSpace) (F64, F64, F64) {
	switch colorSpace {
	case colorSpace_a98_rgb:
		return gam_a98rgb(xyz_to_lin_a98rgb(x, y, z))

	case colorSpace_display_p3:
		return gam_p3(xyz_to_lin_p3(x, y, z))

	case colorSpace_hsl:
		return rgb_to_hsl(gam_srgb(xyz_to_lin_srgb(x, y, z)))

	case colorSpace_hwb:
		return rgb_to_hwb(gam_srgb(xyz_to_lin_srgb(x, y, z)))

	case colorSpace_lab:
		return xyz_to_lab(d65_to_d50(x, y, z))

	case colorSpace_lch:
		return lab_to_lch(xyz_to_lab(d65_to_d50(x, y, z)))

	case colorSpace_oklab:
		return xyz_to_oklab(x, y, z)

	case colorSpace_oklch:
		return oklab_to_oklch(xyz_to_oklab(x, y, z))

	case colorSpace_prophoto_rgb:
		return gam_prophoto(xyz_to_lin_prophoto(d65_to_d50(x, y, z)))

	case colorSpace_rec2020:
		return gam_2020(xyz_to_lin_2020(x, y, z))

	case colorSpace_srgb:
		return gam_srgb(xyz_to_lin_srgb(x, y, z))

	case colorSpace_srgb_linear:
		return xyz_to_lin_srgb(x, y, z)

	case colorSpace_xyz, colorSpace_xyz_d65:
		return x, y, z

	case colorSpace_xyz_d50:
		return d65_to_d50(x, y, z)

	default:
		panic("Internal error")
	}
}

func colorSpace_to_xyz(v0 F64, v1 F64, v2 F64, colorSpace colorSpace) (F64, F64, F64) {
	switch colorSpace {
	case colorSpace_a98_rgb:
		return lin_a98rgb_to_xyz(lin_a98rgb(v0, v1, v2))

	case colorSpace_display_p3:
		return lin_p3_to_xyz(lin_p3(v0, v1, v2))

	case colorSpace_hsl:
		return lin_srgb_to_xyz(lin_srgb(hsl_to_rgb(v0, v1, v2)))

	case colorSpace_hwb:
		return lin_srgb_to_xyz(lin_srgb(hwb_to_rgb(v0, v1, v2)))

	case colorSpace_lab:
		return d50_to_d65(lab_to_xyz(v0, v1, v2))

	case colorSpace_lch:
		return d50_to_d65(lab_to_xyz(lch_to_lab(v0, v1, v2)))

	case colorSpace_oklab:
		return oklab_to_xyz(v0, v1, v2)

	case colorSpace_oklch:
		return oklab_to_xyz(oklch_to_oklab(v0, v1, v2))

	case colorSpace_prophoto_rgb:
		return d50_to_d65(lin_prophoto_to_xyz(lin_prophoto(v0, v1, v2)))

	case colorSpace_rec2020:
		return lin_2020_to_xyz(lin_2020(v0, v1, v2))

	case colorSpace_srgb:
		return lin_srgb_to_xyz(lin_srgb(v0, v1, v2))

	case colorSpace_srgb_linear:
		return lin_srgb_to_xyz(v0, v1, v2)

	case colorSpace_xyz, colorSpace_xyz_d65:
		return v0, v1, v2

	case colorSpace_xyz_d50:
		return d50_to_d65(v0, v1, v2)

	default:
		panic("Internal error")
	}
}
