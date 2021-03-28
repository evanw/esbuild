package test

import (
	"fmt"
	"strings"

	"github.com/evanw/esbuild/internal/logger"
)

func Diff(old string, new string, color bool) string {
	return strings.Join(diffRec(nil, strings.Split(old, "\n"), strings.Split(new, "\n"), color), "\n")
}

// This is a simple recursive line-by-line diff implementation
func diffRec(result []string, old []string, new []string, color bool) []string {
	o, n, common := lcSubstr(old, new)

	if common == 0 {
		// Everything changed
		for _, line := range old {
			if color {
				result = append(result, fmt.Sprintf("%s-%s%s", logger.TerminalColors.Red, line, logger.TerminalColors.Reset))
			} else {
				result = append(result, "-"+line)
			}
		}
		for _, line := range new {
			if color {
				result = append(result, fmt.Sprintf("%s+%s%s", logger.TerminalColors.Green, line, logger.TerminalColors.Reset))
			} else {
				result = append(result, "+"+line)
			}
		}
	} else {
		// Something in the middle stayed the same
		result = diffRec(result, old[:o], new[:n], color)
		for _, line := range old[o : o+common] {
			if color {
				result = append(result, fmt.Sprintf("%s %s%s", logger.TerminalColors.Dim, line, logger.TerminalColors.Reset))
			} else {
				result = append(result, " "+line)
			}
		}
		result = diffRec(result, old[o+common:], new[n+common:], color)
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
