package test

import (
	"strings"
)

func Diff(old string, new string) string {
	return strings.Join(diffRec(nil, strings.Split(old, "\n"), strings.Split(new, "\n")), "\n")
}

// This is a simple recursive line-by-line diff implementation
func diffRec(result []string, old []string, new []string) []string {
	o, n, common := lcSubstr(old, new)

	if common == 0 {
		// Everything changed
		for _, line := range old {
			result = append(result, "-"+line)
		}
		for _, line := range new {
			result = append(result, "+"+line)
		}
	} else {
		// Something in the middle stayed the same
		result = diffRec(result, old[:o], new[:n])
		for _, line := range old[o : o+common] {
			result = append(result, " "+line)
		}
		result = diffRec(result, old[o+common:], new[n+common:])
	}

	return result
}

// From: https://en.wikipedia.org/wiki/Longest_common_substring_problem
func lcSubstr(S []string, T []string) (int, int, int) {
	r := len(S)
	n := len(T)
	Lprev := make([]int, n)
	Lnext := make([]int, n)
	z := 0
	retI := 0
	retJ := 0

	for i := 0; i < r; i++ {
		for j := 0; j < n; j++ {
			if S[i] == T[j] {
				if j == 0 {
					Lnext[j] = 1
				} else {
					Lnext[j] = Lprev[j-1] + 1
				}
				if Lnext[j] > z {
					z = Lnext[j]
					retI = i + 1
					retJ = j + 1
				}
			} else {
				Lnext[j] = 0
			}
		}
		Lprev, Lnext = Lnext, Lprev
	}

	return retI - z, retJ - z, z
}
