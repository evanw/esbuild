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
