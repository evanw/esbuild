package test

import (
	"fmt"
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
		color := !fs.CheckIfWindows()
		t.Fatal("\n" + diff(stringB, stringA, color))
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
