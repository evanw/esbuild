package exitcode_test

import (
	"errors"
	"flag"
	"fmt"
	"testing"

	"github.com/evanw/esbuild/internal/exitcode"
)

func TestGet(t *testing.T) {
	base := exitcode.Set(errors.New(""), 4)
	wrapped := fmt.Errorf("wrapping: %w", base)

	testCases := map[string]struct {
		error
		int
	}{
		"nil":     {nil, 0},
		"default": {errors.New(""), 1},
		"help":    {flag.ErrHelp, 2},
		"set":     {exitcode.Set(errors.New(""), 3), 3},
		"wrapped": {wrapped, 4},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			err := tc.error
			want := tc.int
			got := exitcode.Get(err)
			if got != want {
				t.Errorf("%v: %d != %d", err, got, want)
			}
		})
	}
}

func TestSet(t *testing.T) {
	t.Run("same-message", func(t *testing.T) {
		err := errors.New("hello")
		coder := exitcode.Set(err, 2)
		got := err.Error()
		want := coder.Error()
		if got != want {
			t.Errorf("error message %q != %q", got, want)
		}
	})
	t.Run("keep-chain", func(t *testing.T) {
		err := errors.New("hello")
		coder := exitcode.Set(err, 3)

		if !errors.Is(coder, err) {
			t.Errorf("broken chain: %v is not %v", coder, err)
		}
	})
}
