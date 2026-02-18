//go:build js && wasm
// +build js,wasm

package js_parser

import "strconv"

// bigIntToDecimal converts a BigInt literal (which may be hex/octal/binary)
// to its decimal string form. For values that exceed uint64, the original
// string is returned unchanged since we can't convert without math/big.
func bigIntToDecimal(value string) string {
	if len(value) < 2 || value[0] != '0' {
		return value
	}

	var base int
	digits := value[2:]
	switch value[1] {
	case 'x', 'X':
		base = 16
	case 'o', 'O':
		base = 8
	case 'b', 'B':
		base = 2
	default:
		return value
	}

	if len(digits) == 0 {
		return value
	}

	// Remove underscores (numeric separators)
	clean := digits
	for i := 0; i < len(clean); i++ {
		if clean[i] == '_' {
			buf := make([]byte, 0, len(clean))
			for j := 0; j < len(clean); j++ {
				if clean[j] != '_' {
					buf = append(buf, clean[j])
				}
			}
			clean = string(buf)
			break
		}
	}

	n, err := strconv.ParseUint(clean, base, 64)
	if err != nil {
		// Value exceeds uint64; return original
		return value
	}
	return strconv.FormatUint(n, 10)
}
