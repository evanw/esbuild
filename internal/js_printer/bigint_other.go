//go:build !js || !wasm
// +build !js !wasm

package js_printer

import (
	"fmt"
	"math/big"
)

func bigIntToDecimal(value string) string {
	var i big.Int
	fmt.Sscan(value, &i)
	return i.String()
}
