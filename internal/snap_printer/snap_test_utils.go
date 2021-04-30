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
	shouldRewrite        bool
	snapFilePath         string
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
			text += msg.String(logger.OutputOptions{}, logger.TerminalInfo{})
		}
		assertEqual(t, text, "")
		if !ok {
			t.Fatal("Parse error")
		}
		symbols := js_ast.NewSymbolMap(1)
		symbols.SymbolsForSource[0] = tree.Symbols
		r := snap_renamer.NewSnapRenamer(
			symbols,
			name,
			tree.DirnameRef,
			tree.FilenameRef,
			testOpts.shouldRewrite)

		js := Print(
			tree,
			symbols,
			&r,
			options,
			testOpts.isWrapped,
			testOpts.shouldReplaceRequire,
		).JS

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
				if len(testOpts.snapFilePath) != 0 {
					err := ioutil.WriteFile(testOpts.snapFilePath, []byte(actualTrimmed), 0644)
					if err != nil {
						panic(err)
					}
				}
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
		testOpts{shouldReplaceRequire: shouldReplaceRequire, shouldRewrite: true},
	)
}

func expectPrintedNorewrite(t *testing.T, contents string, expected string, shouldReplaceRequire func(string) bool) {
	t.Helper()
	expectPrintedCommon(
		t,
		contents,
		contents,
		expected,
		PrintOptions{},
		testOpts{shouldReplaceRequire: shouldReplaceRequire, shouldRewrite: false},
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
		testOpts{shouldReplaceRequire: shouldReplaceRequire, compareByLine: true, shouldRewrite: true},
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
		testOpts{shouldReplaceRequire: shouldReplaceRequire, compareByLine: true, debug: true, shouldRewrite: true},
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
		testOpts{shouldReplaceRequire: shouldReplaceRequire, debug: true, shouldRewrite: true},
	)
}

func debugPrintedNorewrite(t *testing.T, contents string, shouldReplaceRequire func(string) bool) {
	t.Helper()
	expectPrintedCommon(
		t,
		contents,
		contents,
		"",
		PrintOptions{},
		testOpts{shouldReplaceRequire: shouldReplaceRequire, debug: true, shouldRewrite: false},
	)
}

func debugFixture(t *testing.T, fixtureName string, shouldReplaceRequire func(string) bool) {
	t.Helper()
	contents, err := ioutil.ReadFile("./fixtures/" + fixtureName)
	if err != nil {
		panic(err)
	}

	expectPrintedCommon(
		t,
		fixtureName,
		string(contents),
		"",
		PrintOptions{},
		testOpts{
			shouldReplaceRequire: shouldReplaceRequire,
			debug:                true,
			snapFilePath:         "./fixtures/snap-" + fixtureName,
			shouldRewrite:        true,
		},
	)
}

func expectFixture(t *testing.T, fixtureName string, shouldReplaceRequire func(string) bool) {
	t.Helper()
	contents, err1 := ioutil.ReadFile("./fixtures/" + fixtureName)
	if err1 != nil {
		panic(err1)
	}
	expected, err2 := ioutil.ReadFile("./fixtures/snap-" + fixtureName)
	if err2 != nil {
		panic(err1)
	}

	expectPrintedCommon(
		t,
		fixtureName,
		string(contents),
		string(expected),
		PrintOptions{},
		testOpts{
			shouldReplaceRequire: shouldReplaceRequire,
			debug:                true,
			snapFilePath:         "./fixtures/snap-" + fixtureName,
			shouldRewrite:        true,
		},
	)
}

func expectValidationErrors(
	t *testing.T,
	contents string,
	expectedMessages []string,
) {
	name := contents
	options := PrintOptions{}
	testOpts := testOpts{shouldReplaceRequire: ReplaceAll, shouldRewrite: true}

	t.Helper()
	t.Run(name, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog()
		tree, ok := js_parser.Parse(log, test.SourceForTest(contents), js_parser.Options{})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logger.OutputOptions{}, logger.TerminalInfo{})
		}
		assertEqual(t, text, "")
		if !ok {
			t.Fatal("Parse error")
		}
		symbols := js_ast.NewSymbolMap(1)
		symbols.SymbolsForSource[0] = tree.Symbols
		r := snap_renamer.NewSnapRenamer(
			symbols,
			name,
			tree.DirnameRef,
			tree.FilenameRef,
			testOpts.shouldRewrite)

		errors := Print(
			tree,
			symbols,
			&r,
			options,
			testOpts.isWrapped,
			testOpts.shouldReplaceRequire,
		).ValidationErrors

		if expectedMessages == nil {
			fmt.Println("[]string{")
			for _, err := range errors {
				fmt.Printf("    \"%s\",\n", err.Msg)
			}
			fmt.Println("}")
		} else {
			assertEqual(t, len(errors), len(expectedMessages))
			for idx, err := range errors {
				assertEqual(t, expectedMessages[idx], err.Msg)
			}
		}
	})
}
func ReplaceAll(string) bool  { return true }
func ReplaceNone(string) bool { return false }
