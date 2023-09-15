package compat

import (
	"fmt"
	"testing"

	"github.com/evanw/esbuild/internal/test"
)

func TestCompareVersions(t *testing.T) {
	t.Helper()

	check := func(a v, b Semver, expected rune) {
		t.Helper()

		at := fmt.Sprintf("%d.%d.%d", a.major, a.minor, a.patch)
		bt := b.String()

		t.Run(fmt.Sprintf("%q ? %q", at, bt), func(t *testing.T) {
			observed := '='
			if result := compareVersions(a, b); result < 0 {
				observed = '<'
			} else if result > 0 {
				observed = '>'
			}
			if observed != expected {
				test.AssertEqual(t, fmt.Sprintf("%c", observed), fmt.Sprintf("%c", expected))
			}
		})
	}

	check(v{0, 0, 0}, Semver{}, '=')

	check(v{1, 0, 0}, Semver{}, '>')
	check(v{0, 1, 0}, Semver{}, '>')
	check(v{0, 0, 1}, Semver{}, '>')

	check(v{0, 0, 0}, Semver{Parts: []int{1}}, '<')
	check(v{0, 0, 0}, Semver{Parts: []int{0, 1}}, '<')
	check(v{0, 0, 0}, Semver{Parts: []int{0, 0, 1}}, '<')

	check(v{0, 4, 0}, Semver{Parts: []int{0, 5, 0}}, '<')
	check(v{0, 5, 0}, Semver{Parts: []int{0, 5, 0}}, '=')
	check(v{0, 6, 0}, Semver{Parts: []int{0, 5, 0}}, '>')

	check(v{0, 5, 0}, Semver{Parts: []int{0, 5, 1}}, '<')
	check(v{0, 5, 0}, Semver{Parts: []int{0, 5, 0}}, '=')
	check(v{0, 5, 1}, Semver{Parts: []int{0, 5, 0}}, '>')

	check(v{0, 5, 0}, Semver{Parts: []int{0, 5}}, '=')
	check(v{0, 5, 1}, Semver{Parts: []int{0, 5}}, '>')

	check(v{1, 0, 0}, Semver{Parts: []int{1}}, '=')
	check(v{1, 1, 0}, Semver{Parts: []int{1}}, '>')
	check(v{1, 0, 1}, Semver{Parts: []int{1}}, '>')

	check(v{1, 2, 0}, Semver{Parts: []int{1, 2}, PreRelease: "-pre"}, '>')
	check(v{1, 2, 1}, Semver{Parts: []int{1, 2}, PreRelease: "-pre"}, '>')
	check(v{1, 1, 0}, Semver{Parts: []int{1, 2}, PreRelease: "-pre"}, '<')

	check(v{1, 2, 3}, Semver{Parts: []int{1, 2, 3}, PreRelease: "-pre"}, '>')
	check(v{1, 2, 2}, Semver{Parts: []int{1, 2, 3}, PreRelease: "-pre"}, '<')
}
