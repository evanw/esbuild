package js_printer

import (
	"testing"
)

func expectRemoveConsole(t *testing.T, contents string, expected string, transpile []string) {
	t.Helper()
	expectPrintedCommon(t, contents, contents, expected, Options{
		RemoveConsole: true,
	})
}

func expectRemoveDebugger(t *testing.T, contents string, expected string, transpile []string) {
	t.Helper()
	expectPrintedCommon(t, contents, contents, expected, Options{
		RemoveDebugger: true,
	})
}

func TestRemoveConsole(t *testing.T) {
	expectRemoveConsole(t, "console.log(\"x\");", "", []string{"remove-console"})
	expectPrintedMinify(t, "console.log(\"x\");", "console.log(\"x\");")
}

func TestRemoveDebugger(t *testing.T) {
	expectRemoveDebugger(t, "debugger;", "", []string{"remove-debugger"})
	expectPrintedMinify(t, "debugger;", "debugger;")
}
