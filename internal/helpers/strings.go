package helpers

import (
	"fmt"
	"strings"
)

func StringArraysEqual(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, x := range a {
		if x != b[i] {
			return false
		}
	}
	return true
}

func StringArrayArraysEqual(a [][]string, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, x := range a {
		if !StringArraysEqual(x, b[i]) {
			return false
		}
	}
	return true
}

func StringArrayToQuotedCommaSeparatedString(a []string) string {
	sb := strings.Builder{}
	for i, str := range a {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("%q", str))
	}
	return sb.String()
}
