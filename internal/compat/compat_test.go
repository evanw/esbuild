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

func TestCompareSemver(t *testing.T) {
	t.Helper()

	check := func(a Semver, b Semver, expected rune) {
		t.Helper()

		at := a.String()
		bt := b.String()

		t.Run(fmt.Sprintf("%q ? %q", at, bt), func(t *testing.T) {
			observed := '='
			if result := CompareSemver(a, b); result < 0 {
				observed = '<'
			} else if result > 0 {
				observed = '>'
			}
			if observed != expected {
				test.AssertEqual(t, fmt.Sprintf("%c", observed), fmt.Sprintf("%c", expected))
			}
		})

		// Automatically generate reverse comparison cases to cut down on test count
		if expected == '<' {
			t.Run(fmt.Sprintf("%q ? %q", bt, at), func(t *testing.T) {
				observed := '='
				if result := CompareSemver(b, a); result < 0 {
					observed = '<'
				} else if result > 0 {
					observed = '>'
				}
				if observed != '>' {
					test.AssertEqual(t, fmt.Sprintf("%c", observed), ">")
				}
			})
		} else if expected == '>' {
			t.Fail()
		}
	}

	check(Semver{}, Semver{}, '=')
	check(Semver{Parts: []int{0}}, Semver{}, '=')
	check(Semver{}, Semver{Parts: []int{0}}, '=')
	check(Semver{Parts: []int{0}}, Semver{Parts: []int{0}}, '=')
	check(Semver{Parts: []int{0, 0}}, Semver{Parts: []int{0}}, '=')
	check(Semver{Parts: []int{0}}, Semver{Parts: []int{0}}, '=')
	check(Semver{Parts: []int{0, 0}}, Semver{Parts: []int{0, 0}}, '=')
	check(Semver{Parts: []int{1}, PreRelease: "-alpha"}, Semver{Parts: []int{1}, PreRelease: "-alpha"}, '=')
	check(Semver{Parts: []int{1, 0}, PreRelease: "-alpha"}, Semver{Parts: []int{1}, PreRelease: "-alpha"}, '=')
	check(Semver{Parts: []int{1, 0}, PreRelease: "-alpha"}, Semver{Parts: []int{1, 0, 0}, PreRelease: "-alpha"}, '=')

	check(Semver{}, Semver{Parts: []int{1}}, '<')
	check(Semver{}, Semver{Parts: []int{0, 1}}, '<')
	check(Semver{Parts: []int{0}}, Semver{Parts: []int{1}}, '<')
	check(Semver{Parts: []int{0, 1}}, Semver{Parts: []int{1}}, '<')
	check(Semver{Parts: []int{0, 1}}, Semver{Parts: []int{1, 0}}, '<')
	check(Semver{Parts: []int{2, 0, 1}}, Semver{Parts: []int{2, 1, 0}}, '<')
	check(Semver{Parts: []int{2, 0, 1}}, Semver{Parts: []int{2, 0, 2}}, '<')
	check(Semver{Parts: []int{1}, PreRelease: "-alpha"}, Semver{Parts: []int{1}}, '<')
	check(Semver{Parts: []int{1, 0}, PreRelease: "-alpha"}, Semver{Parts: []int{1}, PreRelease: "-alpha.1"}, '<')
	check(Semver{Parts: []int{1, 0}, PreRelease: "-alpha.1"}, Semver{Parts: []int{1}, PreRelease: "-alpha.beta"}, '<')
	check(Semver{Parts: []int{1, 0}, PreRelease: "-alpha.1"}, Semver{Parts: []int{1}, PreRelease: "-alpha.11"}, '<')
	check(Semver{Parts: []int{1, 0}, PreRelease: "-alpha.1"}, Semver{Parts: []int{1}, PreRelease: "-alpha.02"}, '<')
	check(Semver{Parts: []int{1, 0}, PreRelease: "-alpha.1"}, Semver{Parts: []int{1}, PreRelease: "-alpha.01"}, '<')
	check(Semver{Parts: []int{1, 0}, PreRelease: "-alpha.01"}, Semver{Parts: []int{1}, PreRelease: "-alpha.2"}, '<')
	check(Semver{Parts: []int{1, 0}, PreRelease: "-alpha.08"}, Semver{Parts: []int{1}, PreRelease: "-alpha.011"}, '<')
}
