package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

func TestSplittingSharedES6IntoSystemJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/test.js": `
        import './side-effect.js';
        import * as dep from './dep.js';
        export var p = dep.q * 10;
        import('external');
      `,
			"/side-effect.js": `
        global.sideeffect = 'side effect';
      `,
			"/dep.js": `export var q = 10`,
		},
		entryPaths: []string{"/test.js", "/dep.js"},
		options: config.Options{
			IsBundling:    true,
			CodeSplitting: true,
			OutputFormat:  config.FormatSystemJS,
			AbsOutputDir:  "/out",
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"external": true,
				},
			},
		},
		expected: map[string]string{
			"/out/test.js": `System.register(["./chunk.Jhw0X7kF.js"], function (__exports, __context) {
  var q;
  return {
    setters: [function (__m) {
      q = __m.q;
    }],
    execute: function () {
      // /side-effect.js
      global.sideeffect = "side effect";

      // /test.js
      var p = q * 10;
      __context.import("external");
      __exports({
        p
      });
    }
  };
});
`,
			"/out/dep.js": `System.register(["./chunk.Jhw0X7kF.js"], function (__exports, __context) {
  var q;
  return {
    setters: [function (__m) {
      q = __m.q;
    }],
    execute: function () {
      // /dep.js
      __exports({
        q
      });
    }
  };
});
`,
			"/out/chunk.Jhw0X7kF.js": `System.register([], function (__exports, __context) {
  return {
    setters: [],
    execute: function () {
      // /dep.js
      var q = 10;
      __exports({
        q
      });
    }
  };
});
`,
		},
	})
}
