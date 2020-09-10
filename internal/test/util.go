package test

import (
	"testing"

	"github.com/evanw/esbuild/internal/logging"
)

func AssertEqual(t *testing.T, a interface{}, b interface{}) {
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
}

func SourceForTest(contents string) logging.Source {
	return logging.Source{
		Index:          0,
		KeyPath:        logging.Path{Text: "<stdin>"},
		PrettyPath:     "<stdin>",
		Contents:       contents,
		IdentifierName: "stdin",
	}
}
