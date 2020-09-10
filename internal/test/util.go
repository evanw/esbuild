package test

import (
	"testing"

	"github.com/evanw/esbuild/internal/logger"
)

func AssertEqual(t *testing.T, a interface{}, b interface{}) {
	if a != b {
		t.Fatalf("%s != %s", a, b)
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
