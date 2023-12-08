package css_parser

import "math"

// Reference: https://drafts.csswg.org/css-color/#color-conversion-code

func lin_srgb(r float64, g float64, b float64) (float64, float64, float64) {
	f := func(val float64) float64 {
		if abs := math.Abs(val); abs < 0.04045 {
			return val / 12.92
		} else {
			return math.Copysign(math.Pow((abs+0.055)/1.055, 2.4), val)
		}
	}
	return f(r), f(g), f(b)
}

func gam_srgb(r float64, g float64, b float64) (float64, float64, float64) {
	f := func(val float64) float64 {
		if abs := math.Abs(val); abs > 0.0031308 {
			return math.Copysign(1.055*math.Pow(abs, 1/2.4)-0.055, val)
		} else {
			return 12.92 * val
		}
	}
	return f(r), f(g), f(b)
}

func lin_srgb_to_xyz(r float64, g float64, b float64) (float64, float64, float64) {
	M := [9]float64{
		506752.0 / 1228815, 87881.0 / 245763, 12673.0 / 70218,
		87098.0 / 409605, 175762.0 / 245763, 12673.0 / 175545,
		7918.0 / 409605, 87881.0 / 737289, 1001167.0 / 1053270,
	}
	return multiplyMatrices(M, r, g, b)
}

func xyz_to_lin_srgb(x float64, y float64, z float64) (float64, float64, float64) {
	M := [9]float64{
		12831.0 / 3959, -329.0 / 214, -1974.0 / 3959,
		-851781.0 / 878810, 1648619.0 / 878810, 36519.0 / 878810,
		705.0 / 12673, -2585.0 / 12673, 705.0 / 667,
	}
	return multiplyMatrices(M, x, y, z)
}

func lin_p3(r float64, g float64, b float64) (float64, float64, float64) {
	return lin_srgb(r, g, b)
}

func lin_p3_to_xyz(r float64, g float64, b float64) (float64, float64, float64) {
	M := [9]float64{
		608311.0 / 1250200, 189793.0 / 714400, 198249.0 / 1000160,
		35783.0 / 156275, 247089.0 / 357200, 198249.0 / 2500400,
		0.0 / 1, 32229.0 / 714400, 5220557.0 / 5000800,
	}
	return multiplyMatrices(M, r, g, b)
}

func lin_prophoto(r float64, g float64, b float64) (float64, float64, float64) {
	f := func(val float64) float64 {
		const Et2 = 16.0 / 512
		if abs := math.Abs(val); abs <= Et2 {
			return val / 16
		} else {
			return math.Copysign(math.Pow(abs, 1.8), val)
		}
	}
	return f(r), f(g), f(b)
}

func lin_prophoto_to_xyz(r float64, g float64, b float64) (float64, float64, float64) {
	M := [9]float64{
		0.7977604896723027, 0.13518583717574031, 0.0313493495815248,
		0.2880711282292934, 0.7118432178101014, 0.00008565396060525902,
		0.0, 0.0, 0.8251046025104601,
	}
	return multiplyMatrices(M, r, g, b)
}

func lin_a98rgb(r float64, g float64, b float64) (float64, float64, float64) {
	f := func(val float64) float64 {
		return math.Copysign(math.Pow(math.Abs(val), 563.0/256), val)
	}
	return f(r), f(g), f(b)
}

func lin_a98rgb_to_xyz(r float64, g float64, b float64) (float64, float64, float64) {
	M := [9]float64{
		573536.0 / 994567, 263643.0 / 1420810, 187206.0 / 994567,
		591459.0 / 1989134, 6239551.0 / 9945670, 374412.0 / 4972835,
		53769.0 / 1989134, 351524.0 / 4972835, 4929758.0 / 4972835,
	}
	return multiplyMatrices(M, r, g, b)
}

func lin_2020(r float64, g float64, b float64) (float64, float64, float64) {
	f := func(val float64) float64 {
		const α = 1.09929682680944
		const β = 0.018053968510807
		if abs := math.Abs(val); abs < β*4.5 {
			return val / 4.5
		} else {
			return math.Copysign(math.Pow((abs+(α-1))/α, 1/0.45), val)
		}
	}
	return f(r), f(g), f(b)
}

func lin_2020_to_xyz(r float64, g float64, b float64) (float64, float64, float64) {
	var M = [9]float64{
		63426534.0 / 99577255, 20160776.0 / 139408157, 47086771.0 / 278816314,
		26158966.0 / 99577255, 472592308.0 / 697040785, 8267143.0 / 139408157,
		0.0 / 1, 19567812.0 / 697040785, 295819943.0 / 278816314,
	}
	return multiplyMatrices(M, r, g, b)
}

func d65_to_d50(x float64, y float64, z float64) (float64, float64, float64) {
	M := [9]float64{
		1.0479297925449969, 0.022946870601609652, -0.05019226628920524,
		0.02962780877005599, 0.9904344267538799, -0.017073799063418826,
		-0.009243040646204504, 0.015055191490298152, 0.7518742814281371,
	}
	return multiplyMatrices(M, x, y, z)
}

func d50_to_d65(x float64, y float64, z float64) (float64, float64, float64) {
	M := [9]float64{
		0.955473421488075, -0.02309845494876471, 0.06325924320057072,
		-0.0283697093338637, 1.0099953980813041, 0.021041441191917323,
		0.012314014864481998, -0.020507649298898964, 1.330365926242124,
	}
	return multiplyMatrices(M, x, y, z)
}

const d50_x = 0.3457 / 0.3585
const d50_z = (1.0 - 0.3457 - 0.3585) / 0.3585

func xyz_to_lab(x float64, y float64, z float64) (float64, float64, float64) {
	const ε = 216.0 / 24389
	const κ = 24389.0 / 27

	x /= d50_x
	z /= d50_z

	var f0, f1, f2 float64
	if x > ε {
		f0 = math.Cbrt(x)
	} else {
		f0 = (κ*x + 16) / 116
	}
	if y > ε {
		f1 = math.Cbrt(y)
	} else {
		f1 = (κ*y + 16) / 116
	}
	if z > ε {
		f2 = math.Cbrt(z)
	} else {
		f2 = (κ*z + 16) / 116
	}

	return (116 * f1) - 16,
		500 * (f0 - f1),
		200 * (f1 - f2)
}

func lab_to_xyz(l float64, a float64, b float64) (x float64, y float64, z float64) {
	const κ = 24389.0 / 27
	const ε = 216.0 / 24389

	f1 := (l + 16) / 116
	f0 := a/500 + f1
	f2 := f1 - b/200

	f0_3 := f0 * f0 * f0
	f2_3 := f2 * f2 * f2

	if f0_3 > ε {
		x = f0_3
	} else {
		x = (116*f0 - 16) / κ
	}
	if l > κ*ε {
		y = (l + 16) / 116
		y = y * y * y
	} else {
		y = l / κ
	}
	if f2_3 > ε {
		z = f2_3
	} else {
		z = (116*f2 - 16) / κ
	}

	return x * d50_x, y, z * d50_z
}

func lab_to_lch(l float64, a float64, b float64) (float64, float64, float64) {
	hue := math.Atan2(b, a) * (180 / math.Pi)
	if hue < 0 {
		hue += 360
	}
	return l,
		math.Sqrt(a*a + b*b),
		hue
}

func lch_to_lab(l float64, c float64, h float64) (float64, float64, float64) {
	return l,
		c * math.Cos(h*math.Pi/180),
		c * math.Sin(h*math.Pi/180)
}

func xyz_to_oklab(x float64, y float64, z float64) (float64, float64, float64) {
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
	return multiplyMatrices(LMStoOKLab, math.Cbrt(l), math.Cbrt(m), math.Cbrt(s))
}

func oklab_to_xyz(l float64, a float64, b float64) (float64, float64, float64) {
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
	return multiplyMatrices(LMStoXYZ, l*l*l, m*m*m, s*s*s)
}

func oklab_to_oklch(l float64, a float64, b float64) (float64, float64, float64) {
	return lab_to_lch(l, a, b)
}

func oklch_to_oklab(l float64, c float64, h float64) (float64, float64, float64) {
	return lch_to_lab(l, c, h)
}

func multiplyMatrices(A [9]float64, b0 float64, b1 float64, b2 float64) (float64, float64, float64) {
	return A[0]*b0 + A[1]*b1 + A[2]*b2,
		A[3]*b0 + A[4]*b1 + A[5]*b2,
		A[6]*b0 + A[7]*b1 + A[8]*b2
}

func delta_eok(L1 float64, a1 float64, b1 float64, L2 float64, a2 float64, b2 float64) float64 {
	ΔL := L1 - L2
	Δa := a1 - a2
	Δb := b1 - b2
	return math.Sqrt(ΔL*ΔL + Δa*Δa + Δb*Δb)
}

func gamut_mapping_xyz_to_srgb(x float64, y float64, z float64) (float64, float64, float64) {
	origin_l, origin_c, origin_h := oklab_to_oklch(xyz_to_oklab(x, y, z))

	if origin_l >= 1 || origin_l <= 0 {
		return origin_l, origin_l, origin_l
	}

	oklch_to_srgb := func(l float64, c float64, h float64) (float64, float64, float64) {
		l, a, b := oklch_to_oklab(l, c, h)
		x, y, z := oklab_to_xyz(l, a, b)
		r, g, b := xyz_to_lin_srgb(x, y, z)
		return gam_srgb(r, g, b)
	}

	srgb_to_oklab := func(r float64, g float64, b float64) (float64, float64, float64) {
		r, g, b = lin_srgb(r, g, b)
		x, y, z := lin_srgb_to_xyz(r, g, b)
		return xyz_to_oklab(x, y, z)
	}

	inGamut := func(r float64, g float64, b float64) bool {
		return r >= 0 && r <= 1 && g >= 0 && g <= 1 && b >= 0 && b <= 1
	}

	r, g, b := oklch_to_srgb(origin_l, origin_c, origin_h)
	if inGamut(r, g, b) {
		return r, g, b
	}

	const JND = 0.02
	const epsilon = 0.0001
	min := 0.0
	max := origin_c

	clip := func(x float64) float64 {
		if x < 0 {
			return 0
		}
		if x > 1 {
			return 1
		}
		return x
	}

	for max-min > epsilon {
		chroma := (min + max) / 2
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
		if E < JND {
			return clipped_r, clipped_g, clipped_b
		}

		max = chroma
	}

	return r, g, b
}
