package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/logger"
)

func AssertEqual(t *testing.T, observed interface{}, expected interface{}) {
	t.Helper()
	if observed != expected {
		t.Fatalf("%s != %s", observed, expected)
	}
}

func AssertEqualWithDiff(t *testing.T, observed interface{}, expected interface{}) {
	t.Helper()
	if observed != expected {
		stringA := fmt.Sprintf("%v", observed)
		stringB := fmt.Sprintf("%v", expected)
		if strings.Contains(stringA, "\n") {
			color := !fs.CheckIfWindows()
			t.Fatal(diff(stringB, stringA, color))
		} else {
			t.Fatalf("%s != %s", observed, expected)
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
