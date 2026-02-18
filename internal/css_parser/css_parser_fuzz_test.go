//go:build go1.18

package css_parser

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

func FuzzParseCSS(f *testing.F) {
	f.Add([]byte(`body { color: red }`))
	f.Add([]byte(`@media (max-width: 768px) { .x { margin: 0 } }`))
	f.Add([]byte(`:root { --x: calc(1px + 2em) }`))
	f.Add([]byte(`@keyframes spin { from { transform: rotate(0) } to { transform: rotate(360deg) } }`))
	f.Add([]byte(`.a { & .b { color: red } }`))

	f.Fuzz(func(t *testing.T, data []byte) {
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		source := test.SourceForTest(string(data))
		Parse(log, source, OptionsFromConfig(config.LoaderCSS, &config.Options{}))
	})
}
