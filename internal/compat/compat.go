package compat

import (
	"strconv"
	"strings"

	"github.com/evanw/esbuild/internal/ast"
)

type v struct {
	major uint16
	minor uint8
	patch uint8
}

type Semver struct {
	// "1.2.3-alpha" => { Parts: {1, 2, 3}, PreRelease: "-alpha" }
	Parts      []int
	PreRelease string
}

func (v Semver) String() string {
	b := strings.Builder{}
	for _, part := range v.Parts {
		if b.Len() > 0 {
			b.WriteRune('.')
		}
		b.WriteString(strconv.Itoa(part))
	}
	b.WriteString(v.PreRelease)
	return b.String()
}

// Returns <0 if "a < b"
// Returns 0 if "a == b"
// Returns >0 if "a > b"
func compareVersions(a v, b Semver) int {
	diff := int(a.major)
	if len(b.Parts) > 0 {
		diff -= b.Parts[0]
	}
	if diff == 0 {
		diff = int(a.minor)
		if len(b.Parts) > 1 {
			diff -= b.Parts[1]
		}
	}
	if diff == 0 {
		diff = int(a.patch)
		if len(b.Parts) > 2 {
			diff -= b.Parts[2]
		}
	}
	if diff == 0 && len(b.PreRelease) != 0 {
		return 1 // "1.0.0" > "1.0.0-alpha"
	}
	return diff
}

func CompareSemver(a Semver, b Semver) int {
	an := len(a.Parts)
	bn := len(b.Parts)

	n := an
	if bn > n {
		n = bn
	}

	for i := 0; i < n; i++ {
		ai := 0
		bi := 0
		if i < an {
			ai = a.Parts[i]
		}
		if i < bn {
			bi = b.Parts[i]
		}
		if ai != bi {
			return ai - bi
		}
	}

	if (a.PreRelease != "") != (b.PreRelease != "") {
		return len(b.PreRelease) - len(a.PreRelease)
	}

	a_tail := strings.TrimPrefix(a.PreRelease, "-")
	b_tail := strings.TrimPrefix(b.PreRelease, "-")

	for a_tail != "" && b_tail != "" {
		var a_head, b_head string
		a_head, a_tail = splitOffNextPreReleasePart(a_tail)
		b_head, b_tail = splitOffNextPreReleasePart(b_tail)
		if a_head == b_head {
			continue
		}

		// Check for numbers
		a_num, a_isnum := preReleasePartToNumber(a_head)
		b_num, b_isnum := preReleasePartToNumber(b_head)
		if !a_isnum && !b_isnum {
			// Lexicographic comparison for non-numbers
			if a_head < b_head {
				return -1
			} else {
				return 1
			}
		} else if a_isnum && b_isnum {
			// Numeric comparison for numbers
			if a_num != b_num {
				return a_num - b_num
			} else {
				// Compare lengths for different text but equal numbers (e.g. "0" vs. "00")
				return len(a_head) - len(b_head)
			}
		} else {
			// One is a number and the other isn't
			if a_isnum {
				return -1
			} else {
				return 1
			}
		}
	}

	return len(a_tail) - len(b_tail)
}

func splitOffNextPreReleasePart(text string) (head string, tail string) {
	if i := strings.IndexByte(text, '.'); i != -1 {
		return text[:i], text[i+1:]
	}
	return text, ""
}

func preReleasePartToNumber(text string) (int, bool) {
	for _, c := range text {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(text)
	return n, err == nil
}

// The start is inclusive and the end is exclusive
type versionRange struct {
	start v
	end   v // Use 0.0.0 for "no end"
}

func isVersionSupported(ranges []versionRange, version Semver) bool {
	for _, r := range ranges {
		if compareVersions(r.start, version) <= 0 && (r.end == (v{}) || compareVersions(r.end, version) > 0) {
			return true
		}
	}
	return false
}

func SymbolFeature(kind ast.SymbolKind) JSFeature {
	switch kind {
	case ast.SymbolPrivateField:
		return ClassPrivateField
	case ast.SymbolPrivateMethod:
		return ClassPrivateMethod
	case ast.SymbolPrivateGet, ast.SymbolPrivateSet, ast.SymbolPrivateGetSetPair:
		return ClassPrivateAccessor
	case ast.SymbolPrivateStaticField:
		return ClassPrivateStaticField
	case ast.SymbolPrivateStaticMethod:
		return ClassPrivateStaticMethod
	case ast.SymbolPrivateStaticGet, ast.SymbolPrivateStaticSet, ast.SymbolPrivateStaticGetSetPair:
		return ClassPrivateStaticAccessor
	default:
		return 0
	}
}
