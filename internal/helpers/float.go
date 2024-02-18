package helpers

import "math"

// This wraps float64 math operations. Why does this exist? The Go compiler
// contains some optimizations to take advantage of "fused multiply and add"
// (FMA) instructions on certain processors. These instructions lead to
// different output on those processors, which means esbuild's output is no
// longer deterministic across all platforms. From the Go specification itself
// (https://go.dev/ref/spec#Floating_point_operators):
//
//	An implementation may combine multiple floating-point operations into a
//	single fused operation, possibly across statements, and produce a result
//	that differs from the value obtained by executing and rounding the
//	instructions individually. An explicit floating-point type conversion
//	rounds to the precision of the target type, preventing fusion that would
//	discard that rounding.
//
//	For instance, some architectures provide a "fused multiply and add" (FMA)
//	instruction that computes x*y + z without rounding the intermediate result
//	x*y.
//
// Therefore we need to add explicit type conversions such as "float64(x)" to
// prevent optimizations that break correctness. Rather than adding them on a
// case-by-case basis as real correctness issues are discovered, we instead
// preemptively force them to be added everywhere by using this wrapper type
// for all floating-point math.
type F64 struct {
	value float64
}

func NewF64(a float64) F64 {
	return F64{value: float64(a)}
}

func (a F64) Value() float64 {
	return a.value
}

func (a F64) IsNaN() bool {
	return math.IsNaN(a.value)
}

func (a F64) Neg() F64 {
	return NewF64(-a.value)
}

func (a F64) Abs() F64 {
	return NewF64(math.Abs(a.value))
}

func (a F64) Sin() F64 {
	return NewF64(math.Sin(a.value))
}

func (a F64) Cos() F64 {
	return NewF64(math.Cos(a.value))
}

func (a F64) Log2() F64 {
	return NewF64(math.Log2(a.value))
}

func (a F64) Round() F64 {
	return NewF64(math.Round(a.value))
}

func (a F64) Floor() F64 {
	return NewF64(math.Floor(a.value))
}

func (a F64) Ceil() F64 {
	return NewF64(math.Ceil(a.value))
}

func (a F64) Squared() F64 {
	return a.Mul(a)
}

func (a F64) Cubed() F64 {
	return a.Mul(a).Mul(a)
}

func (a F64) Sqrt() F64 {
	return NewF64(math.Sqrt(a.value))
}

func (a F64) Cbrt() F64 {
	return NewF64(math.Cbrt(a.value))
}

func (a F64) Add(b F64) F64 {
	return NewF64(a.value + b.value)
}

func (a F64) AddConst(b float64) F64 {
	return NewF64(a.value + b)
}

func (a F64) Sub(b F64) F64 {
	return NewF64(a.value - b.value)
}

func (a F64) SubConst(b float64) F64 {
	return NewF64(a.value - b)
}

func (a F64) Mul(b F64) F64 {
	return NewF64(a.value * b.value)
}

func (a F64) MulConst(b float64) F64 {
	return NewF64(a.value * b)
}

func (a F64) Div(b F64) F64 {
	return NewF64(a.value / b.value)
}

func (a F64) DivConst(b float64) F64 {
	return NewF64(a.value / b)
}

func (a F64) Pow(b F64) F64 {
	return NewF64(math.Pow(a.value, b.value))
}

func (a F64) PowConst(b float64) F64 {
	return NewF64(math.Pow(a.value, b))
}

func (a F64) Atan2(b F64) F64 {
	return NewF64(math.Atan2(a.value, b.value))
}

func (a F64) WithSignFrom(b F64) F64 {
	return NewF64(math.Copysign(a.value, b.value))
}

func Min2(a F64, b F64) F64 {
	return NewF64(math.Min(a.value, b.value))
}

func Max2(a F64, b F64) F64 {
	return NewF64(math.Max(a.value, b.value))
}

func Min3(a F64, b F64, c F64) F64 {
	return NewF64(math.Min(math.Min(a.value, b.value), c.value))
}

func Max3(a F64, b F64, c F64) F64 {
	return NewF64(math.Max(math.Max(a.value, b.value), c.value))
}

func Lerp(a F64, b F64, t F64) F64 {
	return b.Sub(a).Mul(t).Add(a)
}
