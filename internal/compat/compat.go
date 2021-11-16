package compat

type v struct {
	major uint16
	minor uint8
	patch uint8
}

// Returns <0 if "a < b"
// Returns 0 if "a == b"
// Returns >0 if "a > b"
func compareVersions(a v, b []int) int {
	diff := int(a.major)
	if len(b) > 0 {
		diff -= b[0]
	}
	if diff == 0 {
		diff = int(a.minor)
		if len(b) > 1 {
			diff -= b[1]
		}
	}
	if diff == 0 {
		diff = int(a.patch)
		if len(b) > 2 {
			diff -= b[2]
		}
	}
	return diff
}

// The start is inclusive and the end is exclusive
type versionRange struct {
	start v
	end   v // Use 0.0.0 for "no end"
}

func isVersionSupported(ranges []versionRange, version []int) bool {
	for _, r := range ranges {
		if compareVersions(r.start, version) <= 0 && (r.end == (v{}) || compareVersions(r.end, version) > 0) {
			return true
		}
	}
	return false
}
