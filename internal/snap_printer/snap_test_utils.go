package snap_printer

import (
	"fmt"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_parser"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/snap_renamer"
	"github.com/evanw/esbuild/internal/test"
	"io/ioutil"
	"strings"
	"testing"
)

func assertEqual(t *testing.T, a interface{}, b interface{}) {
	t.Helper()
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
}

type testOpts struct {
	shouldReplaceRequire func(string) bool
	compareByLine        bool
	debug                bool
	isWrapped            bool
}

func showSpaces(s string) string {
	return strings.ReplaceAll(s, " ", "^")
}

func expectPrintedCommon(
	t *testing.T,
	name string,
	contents string,
	expected string,
	options PrintOptions,
	testOpts testOpts,
) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog()
		tree, ok := js_parser.Parse(log, test.SourceForTest(contents), js_parser.Options{})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logger.StderrOptions{}, logger.TerminalInfo{})
		}
		assertEqual(t, text, "")
		if !ok {
			t.Fatal("Parse error")
		}
		symbols := js_ast.NewSymbolMap(1)
		symbols.Outer[0] = tree.Symbols
		r := snap_renamer.NewSnapRenamer(symbols)
		js := Print(tree, symbols, &r, options, testOpts.isWrapped, testOpts.shouldReplaceRequire).JS
		actualTrimmed := strings.TrimSpace(string(js))
		expectedTrimmed := strings.TrimSpace(expected)
		if testOpts.compareByLine {
			actualLines := strings.Split(actualTrimmed, "\n")
			expectedLines := strings.Split(expectedTrimmed, "\n")
			for i, act := range actualLines {
				exp := expectedLines[i]
				if testOpts.debug {
					fmt.Printf("\nact: %s\nexp: %s", showSpaces(act), showSpaces(exp))
				} else {
					assertEqual(t, act, exp)
				}
			}

		} else {
			if testOpts.debug {
				fmt.Println(actualTrimmed)
			} else {
				assertEqual(t, actualTrimmed, expectedTrimmed)
			}
		}
	})
}

func expectPrinted(t *testing.T, contents string, expected string, shouldReplaceRequire func(string) bool) {
	t.Helper()
	expectPrintedCommon(
		t,
		contents,
		contents,
		expected,
		PrintOptions{},
		testOpts{shouldReplaceRequire, false, false, false},
	)
}

func expectByLine(t *testing.T, contents string, expected string, shouldReplaceRequire func(string) bool) {
	t.Helper()
	expectPrintedCommon(
		t,
		contents,
		contents,
		expected,
		PrintOptions{},
		testOpts{shouldReplaceRequire, true, false, false},
	)
}

func debugByLine(t *testing.T, contents string, expected string, shouldReplaceRequire func(string) bool) {
	t.Helper()
	expectPrintedCommon(
		t,
		contents,
		contents,
		expected,
		PrintOptions{},
		testOpts{shouldReplaceRequire, true, true, false},
	)
}

func debugPrinted(t *testing.T, contents string, shouldReplaceRequire func(string) bool) {
	t.Helper()
	expectPrintedCommon(
		t,
		contents,
		contents,
		"",
		PrintOptions{},
		testOpts{shouldReplaceRequire, false, true, false},
	)
}

func debugFile(t *testing.T, path string, shouldReplaceRequire func(string) bool) {
	t.Helper()
	contents, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	debugPrinted(
		t,
		string(contents),
		shouldReplaceRequire)
}
func ReplaceAll(string) bool { return true }
func ReplaceNone(string) bool { return false }
