//go:build go1.18

package css_lexer

import (
	"testing"

	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

func FuzzTokenizeCSS(f *testing.F) {
	f.Add([]byte(`body { color: red }`))
	f.Add([]byte(`U+0025-00FF`))
	f.Add([]byte(`U+4??`))
	f.Add([]byte(`url(https://example.com/foo)`))
	f.Add([]byte(`url("https://example.com/foo")`))
	f.Add([]byte(`url(bad url with spaces)`))
	f.Add([]byte(`"unclosed string`))
	f.Add([]byte(`'unclosed string`))
	f.Add([]byte(`/* unclosed comment`))
	f.Add([]byte(`\61\62\63`))
	f.Add([]byte(`#hash .class ::pseudo :nth-child(2n+1)`))
	f.Add([]byte(`calc(100% - 2px)`))

	f.Fuzz(func(t *testing.T, data []byte) {
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		source := test.SourceForTest(string(data))
		Tokenize(log, source, Options{})
	})
}
