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

func expectDebugTool(t *testing.T, DebugTool string, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, contents, expected, Options{
		DebugTool: DebugTool,
	})
}

func expectRemoveDebugTool(t *testing.T, DebugTool string, contents string, expected string) {
	t.Helper()
	expectPrintedCommon(t, contents, contents, expected, Options{
		DebugTool:       DebugTool,
		RemoveDebugTool: true,
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

func TestDebugToolLogVar(t *testing.T) {
	const source = `import DEBUGTOOL from "./debug";
const a = Math.random() * 10;
const b = Math.random() * 10;
DEBUGTOOL.LOG(a, b);
`
	expectPrinted(t, source, source)

	const expected1 = `import DEBUGTOOL from "./debug";
const a = Math.random() * 10;
const b = Math.random() * 10;
DEBUGTOOL.LOG(null, ` + "[`a`, a], [`b`, b]);" + `
`
	expectDebugTool(t, "DEBUGTOOL", source, expected1)

	const expected2 = `const a = Math.random() * 10;
const b = Math.random() * 10;
`
	expectRemoveDebugTool(t, "DEBUGTOOL", source, expected2)
}

func TestDebugToolAssertVar(t *testing.T) {
	const source = `import DEBUGTOOL from "./debug";
const a = Math.random() * 10;
const b = Math.random() * 10;
DEBUGTOOL.ASSERT(a > 5, b > 5);
`
	expectPrinted(t, source, source)

	const expected1 = `import DEBUGTOOL from "./debug";
const a = Math.random() * 10;
const b = Math.random() * 10;
DEBUGTOOL.ASSERT(null, ` + "[`a > 5`, a > 5], [`b > 5`, b > 5]);" + `
`
	expectDebugTool(t, "DEBUGTOOL", source, expected1)

	const expected2 = `const a = Math.random() * 10;
const b = Math.random() * 10;
`
	expectRemoveDebugTool(t, "DEBUGTOOL", source, expected2)
}

func TestDebugToolHistoryVar(t *testing.T) {
	const source = `import DEBUGTOOL from "./debug";
const a = 1;
const b = 2;
DEBUGTOOL.RESET();
DEBUGTOOL.TRACE(a, b);
DEBUGTOOL.ASSERT("a === 1");
DEBUGTOOL.ASSERT(/b === 2/);
const history = DEBUGTOOL.HISTORY();
DEBUGTOOL.ASSERT(history === "a === 1, b === 2");
`
	expectPrinted(t, source, source)

	const expected = `import DEBUGTOOL from "./debug";
const a = 1;
const b = 2;
DEBUGTOOL.RESET(null);
DEBUGTOOL.TRACE(null, ` + "[`a`, a], [`b`, b]);" + `
DEBUGTOOL.ASSERT(null, "a === 1");
DEBUGTOOL.ASSERT(null, /b === 2/);
const history = DEBUGTOOL.HISTORY();
DEBUGTOOL.ASSERT(null, ` + "[`history === \"a === 1, b === 2\"`, history === \"a === 1, b === 2\"]);" + `
`
	expectDebugTool(t, "DEBUGTOOL", source, expected)
}
