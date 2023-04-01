package api

import (
	"fmt"
	"testing"

	"github.com/evanw/esbuild/internal/test"
)

func TestStripDirPrefix(t *testing.T) {
	expectSuccess := func(path string, prefix string, allowedSlashes string, expected string) {
		t.Helper()
		t.Run(fmt.Sprintf("path=%s prefix=%s slashes=%s", path, prefix, allowedSlashes), func(t *testing.T) {
			t.Helper()
			observed, ok := stripDirPrefix(path, prefix, allowedSlashes)
			if !ok {
				t.Fatalf("Unexpected failure")
			}
			test.AssertEqualWithDiff(t, observed, expected)
		})
	}

	expectFailure := func(path string, prefix string, allowedSlashes string) {
		t.Helper()
		t.Run(fmt.Sprintf("path=%s prefix=%s slashes=%s", path, prefix, allowedSlashes), func(t *testing.T) {
			t.Helper()
			_, ok := stripDirPrefix(path, prefix, allowedSlashes)
			if ok {
				t.Fatalf("Unexpected success")
			}
		})
	}

	// Note: People sometimes set "outdir" to "/" and expect that to work:
	// https://github.com/evanw/esbuild/issues/3027

	expectSuccess(`/foo/bar/baz`, ``, `/`, `/foo/bar/baz`)
	expectSuccess(`/foo/bar/baz`, `/`, `/`, `foo/bar/baz`)
	expectSuccess(`/foo/bar/baz`, `/foo`, `/`, `bar/baz`)
	expectSuccess(`/foo/bar/baz`, `/foo/bar`, `/`, `baz`)
	expectSuccess(`/foo/bar/baz`, `/foo/bar/baz`, `/`, ``)
	expectSuccess(`/foo/bar//baz`, `/foo/bar`, `/`, `/baz`)
	expectSuccess(`C:\foo\bar\baz`, ``, `\/`, `C:\foo\bar\baz`)
	expectSuccess(`C:\foo\bar\baz`, `C:`, `\/`, `foo\bar\baz`)
	expectSuccess(`C:\foo\bar\baz`, `C:\`, `\/`, `foo\bar\baz`)
	expectSuccess(`C:\foo\bar\baz`, `C:\foo`, `\/`, `bar\baz`)
	expectSuccess(`C:\foo\bar\baz`, `C:\foo\bar`, `\/`, `baz`)
	expectSuccess(`C:\foo\bar\baz`, `C:\foo\bar\baz`, `\/`, ``)
	expectSuccess(`C:\foo\bar\\baz`, `C:\foo\bar`, `\/`, `\baz`)
	expectSuccess(`C:\foo\bar/baz`, `C:\foo\bar`, `\/`, `baz`)

	expectFailure(`/foo/bar`, `/foo/ba`, `/`)
	expectFailure(`/foo/bar`, `/fo`, `/`)
	expectFailure(`C:\foo\bar`, `C:\foo\ba`, `\/`)
	expectFailure(`C:\foo\bar`, `C:\fo`, `\/`)
	expectFailure(`C:/foo/bar`, `C:\foo`, `\/`)
}
