//go:build go1.18

package js_parser

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

func FuzzParseJS(f *testing.F) {
	f.Add([]byte(`var x = 1;`))
	f.Add([]byte(`export default function() {}`))
	f.Add([]byte(`import { foo } from 'bar'`))
	f.Add([]byte(`class Foo { #x = 1 }`))
	f.Add([]byte(`async function* gen() { yield await 1 }`))
	f.Add([]byte(`const x = a?.b ?? c`))

	options := config.Options{
		OmitRuntimeForTests: true,
	}
	opts := OptionsFromConfig(&options)

	f.Fuzz(func(t *testing.T, data []byte) {
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		source := test.SourceForTest(string(data))
		Parse(log, source, opts)
	})
}
