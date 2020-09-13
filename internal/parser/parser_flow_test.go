package parser

import (
	"testing"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/printer"
	"github.com/evanw/esbuild/internal/renamer"
	"github.com/evanw/esbuild/internal/test"
)

func expectParseErrorFlow(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log := logger.NewDeferLog()
		Parse(log, test.SourceForTest(contents), config.Options{
			Flow: config.FlowOptions{
				Parse: true,
			},
		})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logger.StderrOptions{}, logger.TerminalInfo{})
		}
		test.AssertEqual(t, text, expected)
	})
}

func expectPrintedFlow(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log := logger.NewDeferLog()
		tree, ok := Parse(log, test.SourceForTest(contents), config.Options{
			Flow: config.FlowOptions{
				Parse: true,
			},
		})
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logger.StderrOptions{}, logger.TerminalInfo{})
		}
		test.AssertEqual(t, text, "")
		if !ok {
			t.Fatal("Parse error")
		}
		symbols := ast.NewSymbolMap(1)
		symbols.Outer[0] = tree.Symbols
		r := renamer.NewNoOpRenamer(symbols)
		js := printer.Print(tree, symbols, r, printer.PrintOptions{}).JS
		test.AssertEqual(t, string(js), expected)
	})
}

func TestFlowImportTypeof(t *testing.T) {
	expectPrintedFlow(t, "import typeof foo from 'pkg'", "")
	expectPrintedFlow(t, "import typeof {foo} from 'pkg'", "")
	expectPrintedFlow(t, "import typeof {foo, bar} from 'pkg'", "")
	expectPrintedFlow(t, "import typeof * as ns from 'pkg'", "")
	expectPrintedFlow(t, "import typeof foo, {bar, baz} from 'pkg'", "")
	expectPrintedFlow(t, "import typeof foo, * as ns from 'pkg'", "")

	// Allowed by Flow parser, but not by Babel
	expectPrintedFlow(t, "import typeof from from 'pkg'", "")

	expectParseErrorFlow(t, "import typeof foo, bar from 'pkg'", "<stdin>: error: Unexpected \"bar\"\n")
	expectParseErrorFlow(t, "import typeof * as ns, bar from 'pkg'", "<stdin>: error: Expected \"from\" but found \",\"\n")
	expectParseErrorFlow(t, "import typeof {foo}, bar from 'pkg'", "<stdin>: error: Expected \"from\" but found \",\"\n")
}

func TestFlowTypeCastExpressions(t *testing.T) {
	expectPrintedFlow(t, "(value: number)", "value;\n")
	expectPrintedFlow(t, "((value: any): number)", "value;\n")
	expectPrintedFlow(t, "(value: typeof bar)", "value;\n")

	// expectPrintedFlow(t, "([a: string]) => {}", "([a]) => {};")

	expectPrintedFlow(t, "({xxx: 0, yyy: \"hey\"}: {xxx: number; yyy: string})", "({xxx: 0, yyy: \"hey\"});\n")
	expectPrintedFlow(t, "((xxx) => xxx + 1: (xxx: number) => number)", "(xxx) => xxx + 1;\n")
	expectPrintedFlow(t, "(xxx: number)", "xxx;\n")
	expectPrintedFlow(t, "((xxx: number), (yyy: string))", "xxx, yyy;\n")
	// expectPrintedFlow(t, "(<T>() => {}: any);\n((<T>() => {}): any);\n", "() => {\n};\n() => {\n};\n")
	// expectPrintedFlow(t, "([a: string]) => {};\n([a, [b: string]]) => {};\n([a: string] = []) => {};\n({ x: [a: string] }) => {};\n\nasync ([a: string]) => {};\nasync ([a, [b: string]]) => {};\nasync ([a: string] = []) => {};\nasync ({ x: [a: string] }) => {};\n\nlet [a1: string] = c;\nlet [a2, [b: string]] = c;\nlet [a3: string] = c;\nlet { x: [a4: string] } = c;\n", "([a]) => {\n};\n([a, [b2]]) => {\n};\n([a] = []) => {\n};\n({\n  x: [a]\n}) => {\n};\nasync ([a]) => {\n};\nasync ([a, [b2]]) => {\n};\nasync ([a] = []) => {\n};\nasync ({\n  x: [a]\n}) => {\n};\nlet [a1] = c;\nlet [a2, [b]] = c;\nlet [a3] = c;\nlet {\n  x: [a4]\n} = c;\n")
	// expectPrintedFlow(t, "(<T>() => {}: any);\n((<T>() => {}): any);\n", "() => {\n};\n() => {\n};\n")
	expectPrintedFlow(t, "function* foo(z) {\n  const x = ((yield 3): any)\n}", "function* foo(z) {\n  const x = yield 3;\n}\n")
	expectPrintedFlow(t, "function* foo(z) {\n  const x = (yield 3: any)\n}", "function* foo(z) {\n  const x = yield 3;\n}\n")
}
