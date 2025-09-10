package api_test

import (
	"testing"

	"github.com/evanw/esbuild/internal/test"
	"github.com/evanw/esbuild/pkg/api"
)

func TestFormatMessages(t *testing.T) {
	check := func(name string, opts api.FormatMessagesOptions, msg api.Message, expected string) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			test.AssertEqualWithDiff(t, api.FormatMessages([]api.Message{msg}, opts)[0], expected)
		})
	}

	check("Error", api.FormatMessagesOptions{Kind: api.ErrorMessage}, api.Message{Text: "This is a test"}, "✘ [ERROR] This is a test\n\n")
	check("Warning", api.FormatMessagesOptions{Kind: api.WarningMessage}, api.Message{Text: "This is a test"}, "▲ [WARNING] This is a test\n\n")

	check("Basic location",
		api.FormatMessagesOptions{},
		api.Message{Text: "This is a test", Location: &api.Location{
			File:       "some file.js",
			Line:       100,
			Column:     5, // 0-based
			Length:     3,
			LineText:   "this.foo();",
			Suggestion: "bar",
		}},
		`✘ [ERROR] This is a test

    some file.js:100:5:
      100 │ this.foo();
          │      ~~~
          ╵      bar

`,
	)

	check("Unicode location",
		api.FormatMessagesOptions{
			Kind: api.WarningMessage,
		},
		api.Message{Text: "This is a test", Location: &api.Location{
			File:       "some file.js",
			Line:       100,
			Column:     17, // In UTF-8 bytes
			Length:     10, // In UTF-8 bytes
			LineText:   "𝓉𝒽𝒾𝓈.𝒻ℴℴ();",
			Suggestion: "𝒷𝒶𝓇",
		}},
		`▲ [WARNING] This is a test

    some file.js:100:17:
      100 │ 𝓉𝒽𝒾𝓈.𝒻ℴℴ();
          │      ~~~
          ╵      𝒷𝒶𝓇

`,
	)

	check("Tab stop rendering",
		api.FormatMessagesOptions{
			Kind: api.WarningMessage,
		},
		api.Message{Text: "This is a test", Location: &api.Location{
			File:     "some file.js",
			Line:     100,
			Column:   6,
			Length:   4,
			LineText: "0\t1\t23\t45\t678",
		}},
		`▲ [WARNING] This is a test

    some file.js:100:6:
      100 │ 0 1 23  45  678
          ╵       ~~~~~~

`,
	)

	check("Truncated location tail, zero length",
		api.FormatMessagesOptions{
			TerminalWidth: 32,
		},
		api.Message{Text: "This is a test", Location: &api.Location{
			File:     "some file.js",
			Line:     100,
			Column:   3,
			Length:   0,
			LineText: "012345678 abcdefghi ABCDEFGHI 012345678 abcdefghi ABCDEFGHI",
		}},
		`✘ [ERROR] This is a test

    some file.js:100:3:
      100 │ 012345678 abcdefg...
          ╵    ^

`,
	)

	check("Truncated location tail, nonzero length",
		api.FormatMessagesOptions{
			TerminalWidth: 32,
		},
		api.Message{Text: "This is a test", Location: &api.Location{
			File:     "some file.js",
			Line:     100,
			Column:   3,
			Length:   6,
			LineText: "012345678 abcdefghi ABCDEFGHI 012345678 abcdefghi ABCDEFGHI",
		}},
		`✘ [ERROR] This is a test

    some file.js:100:3:
      100 │ 012345678 abcdefg...
          ╵    ~~~~~~

`,
	)

	check("Truncated location tail, truncated length",
		api.FormatMessagesOptions{
			TerminalWidth: 32,
		},
		api.Message{Text: "This is a test", Location: &api.Location{
			File:     "some file.js",
			Line:     100,
			Column:   3,
			Length:   100,
			LineText: "012345678 abcdefghi ABCDEFGHI 012345678 abcdefghi ABCDEFGHI",
		}},
		`✘ [ERROR] This is a test

    some file.js:100:3:
      100 │ 012345678 abcdefg...
          ╵    ~~~~~~~~~~~~~~

`,
	)

	check("Truncated location head, zero length",
		api.FormatMessagesOptions{
			TerminalWidth: 32,
		},
		api.Message{Text: "This is a test", Location: &api.Location{
			File:     "some file.js",
			Line:     100,
			Column:   200,
			Length:   0,
			LineText: "012345678 abcdefghi ABCDEFGHI 012345678 abcdefghi ABCDEFGHI",
		}},
		`✘ [ERROR] This is a test

    some file.js:100:59:
      100 │ ...defghi ABCDEFGHI
          ╵                    ^

`,
	)

	check("Truncated location head, nonzero length",
		api.FormatMessagesOptions{
			TerminalWidth: 32,
		},
		api.Message{Text: "This is a test", Location: &api.Location{
			File:     "some file.js",
			Line:     100,
			Column:   50,
			Length:   200,
			LineText: "012345678 abcdefghi ABCDEFGHI 012345678 abcdefghi ABCDEFGHI",
		}},
		`✘ [ERROR] This is a test

    some file.js:100:50:
      100 │ ...cdefghi ABCDEFGHI
          ╵            ~~~~~~~~~

`,
	)

	check("Truncated location head and tail, truncated length",
		api.FormatMessagesOptions{
			TerminalWidth: 32,
		},
		api.Message{Text: "This is a test", Location: &api.Location{
			File:     "some file.js",
			Line:     100,
			Column:   30,
			Length:   30,
			LineText: "012345678 abcdefghi ABCDEFGHI 012345678 abcdefghi ABCDEFGHI",
		}},
		`✘ [ERROR] This is a test

    some file.js:100:30:
      100 │ ... 012345678 abc...
          ╵     ~~~~~~~~~~~~~

`,
	)

	check("Truncated location head and tail, non-truncated length",
		api.FormatMessagesOptions{
			TerminalWidth: 32,
		},
		api.Message{Text: "This is a test", Location: &api.Location{
			File:     "some file.js",
			Line:     100,
			Column:   30,
			Length:   9,
			LineText: "012345678 abcdefghi ABCDEFGHI 012345678 abcdefghi ABCDEFGHI",
		}},
		`✘ [ERROR] This is a test

    some file.js:100:30:
      100 │ ...HI 012345678 a...
          ╵       ~~~~~~~~~

`,
	)

	check("Multi-line line text",
		api.FormatMessagesOptions{},
		api.Message{Text: "ReferenceError: Cannot access 'foo' before initialization", Location: &api.Location{
			File:   "some file.js",
			Line:   100,
			Column: 2,
			LineText: `  foo();
    at ModuleJob.run (node:internal/modules/esm/module_job:185:25)
    at async Promise.all (index 0)
    at async ESMLoader.import (node:internal/modules/esm/loader:281:24)
    at async loadESM (node:internal/process/esm_loader:88:5)
    at async handleMainPromise (node:internal/modules/run_main:65:12)`,
		}},
		`✘ [ERROR] ReferenceError: Cannot access 'foo' before initialization

    some file.js:100:2:
      100 │   foo();
          ╵   ^

    at ModuleJob.run (node:internal/modules/esm/module_job:185:25)
    at async Promise.all (index 0)
    at async ESMLoader.import (node:internal/modules/esm/loader:281:24)
    at async loadESM (node:internal/process/esm_loader:88:5)
    at async handleMainPromise (node:internal/modules/run_main:65:12)

`,
	)

	check("Note formatting",
		api.FormatMessagesOptions{
			TerminalWidth: 40,
		},
		api.Message{
			Text: "Why would you do this?",
			Location: &api.Location{
				File:     "some file.js",
				Line:     1,
				Column:   10,
				Length:   16,
				LineText: "let ten = +([+!+[]]+[+[]]);",
			},
			Notes: []api.Note{{
				Text: "This is 1:",
				Location: &api.Location{
					File:       "some file.js",
					Line:       1,
					Column:     12,
					Length:     7,
					LineText:   "let ten = +([+!+[]]+[+[]]);",
					Suggestion: "'1'",
				},
			}, {
				Text: "This is 0:",
				Location: &api.Location{
					File:       "some file.js",
					Line:       1,
					Column:     20,
					Length:     5,
					LineText:   "let ten = +([+!+[]]+[+[]]);",
					Suggestion: "'0'",
				},
			}, {
				Text: "The number 0 is created by +[], where [] is the empty array and + is the unary plus, " +
					"used to convert the right side to a numeric value. The number 1 is formed as +!+[], where " +
					"the boolean value true is converted into the numeric value 1 by the prepended plus sign.",
			}},
		},
		`✘ [ERROR] Why would you do this?

    some file.js:1:10:
      1 │ let ten = +([+!+[]]+[+[]]);
        ╵           ~~~~~~~~~~~~~~~~

  This is 1:

    some file.js:1:12:
      1 │ let ten = +([+!+[]]+[+[]]);
        │             ~~~~~~~
        ╵             '1'

  This is 0:

    some file.js:1:20:
      1 │ let ten = +([+!+[]]+[+[]]);
        │                     ~~~~~
        ╵                     '0'

  The number 0 is created by +[], where
  [] is the empty array and + is the
  unary plus, used to convert the right
  side to a numeric value. The number 1
  is formed as +!+[], where the boolean
  value true is converted into the
  numeric value 1 by the prepended plus
  sign.

`,
	)
}

func TestParseAST(t *testing.T) {
	// Test basic JavaScript parsing
	result := api.ParseAST("const x = 42; function foo() { return x; }", api.ParseASTOptions{
		Sourcefile: "test.js",
		Loader:     api.LoaderJS,
		Target:     api.ES2020,
	})

	// Should have no errors
	if len(result.Errors) > 0 {
		t.Errorf("Expected no errors, got %d errors", len(result.Errors))
		for _, err := range result.Errors {
			t.Errorf("Error: %s", err.Text)
		}
	}

	// Should have AST
	if result.AST == nil {
		t.Errorf("Expected AST to be non-nil")
	}

	// Should have symbols
	if len(result.Symbols) == 0 {
		t.Errorf("Expected symbols to be present")
	}

	// Should have scope
	if result.Scope == nil {
		t.Errorf("Expected scope to be non-nil")
	}

	// Test TypeScript parsing
	tsResult := api.ParseAST("interface User { name: string; } const user: User = { name: 'test' };", api.ParseASTOptions{
		Sourcefile: "test.ts",
		Loader:     api.LoaderTS,
		Target:     api.ES2020,
	})

	if len(tsResult.Errors) > 0 {
		t.Errorf("Expected no errors for TypeScript, got %d errors", len(tsResult.Errors))
	}

	if tsResult.AST == nil {
		t.Errorf("Expected TypeScript AST to be non-nil")
	}

	// Test JSX parsing
	jsxResult := api.ParseAST("const element = <div>Hello World</div>;", api.ParseASTOptions{
		Sourcefile: "test.jsx",
		Loader:     api.LoaderJSX,
		Target:     api.ES2020,
		JSX:        api.JSXTransform,
	})

	if len(jsxResult.Errors) > 0 {
		t.Errorf("Expected no errors for JSX, got %d errors", len(jsxResult.Errors))
	}

	if jsxResult.AST == nil {
		t.Errorf("Expected JSX AST to be non-nil")
	}

	// Test syntax error handling
	errorResult := api.ParseAST("const x = ;", api.ParseASTOptions{
		Sourcefile: "error.js",
		Loader:     api.LoaderJS,
	})

	if len(errorResult.Errors) == 0 {
		t.Errorf("Expected errors for invalid syntax, got none")
	}

	// AST should be nil on parse error
	if errorResult.AST != nil {
		t.Errorf("Expected AST to be nil on parse error")
	}
}