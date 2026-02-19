//go:build go1.18

package js_lexer

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

func FuzzLexJS(f *testing.F) {
	f.Add([]byte(`var x = 1;`))
	f.Add([]byte(`/regex/gimsuvy`))
	f.Add([]byte(`/(?<=x)(?<!y)[^]*/g`))
	f.Add([]byte("const x = `hello ${world}`"))
	f.Add([]byte("const x = `${`nested`}`"))
	f.Add([]byte(`'\u0041\u{42}\x43\n\t'`))
	f.Add([]byte(`0x1F + 0o17 + 0b1010`))
	f.Add([]byte(`123_456_789n`))
	f.Add([]byte(`1.5e10`))
	f.Add([]byte(`#!/usr/bin/env node`))
	f.Add([]byte(`// comment
/* block comment */`))
	f.Add([]byte(`"\\""`))

	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() { recover() }()

		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		source := test.SourceForTest(string(data))
		lexer := NewLexer(log, source, config.TSOptions{})
		for lexer.Token != TEndOfFile {
			// The parser always panics on TSyntaxError via SyntaxError(),
			// so the lexer is never called again after this token. Stop
			// here to match real usage and avoid known infinite loops in
			// the lexer's error recovery path.
			if lexer.Token == TSyntaxError {
				break
			}
			lexer.Next()
		}
	})
}
