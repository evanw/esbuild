package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/logger"
)

func AssertEqual(t *testing.T, a interface{}, b interface{}) {
	t.Helper()
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
}

func AssertEqualWithDiff(t *testing.T, a interface{}, b interface{}) {
	t.Helper()
	if a != b {
		stringA := fmt.Sprintf("%v", a)
		stringB := fmt.Sprintf("%v", b)
		if strings.Contains(stringA, "\n") {
			color := !fs.CheckIfWindows()
			t.Fatal(diff(stringB, stringA, color))
		} else {
			t.Fatalf("%s != %s", a, b)
		}
	}
}

func SourceForTest(contents string) logger.Source {
	return logger.Source{
		Index:          0,
		KeyPath:        logger.Path{Text: "<stdin>"},
		PrettyPath:     "<stdin>",
		Contents:       contents,
		IdentifierName: "stdin",
	}
}
