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
