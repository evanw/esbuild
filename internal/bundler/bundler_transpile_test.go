package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

var transpile_suite = suite{
	name: "transpile",
}

func TestNoRemoveConsole(t *testing.T) {
	transpile_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				const a=1;
				console.log(a)
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestRemoveConsole(t *testing.T) {
	transpile_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				const a=1;
				console.log(a)
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			RemoveConsole: true,
			AbsOutputFile: "/out.js",
		},
	})
}
func TestNoRemoveDebbuger(t *testing.T) {
	transpile_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				const a=1;
				debugger;
				console.log(a)
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestRemoveDebbuger(t *testing.T) {
	transpile_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				const a=1;
				debugger;
				console.log(a);
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:           config.ModeBundle,
			RemoveDebugger: true,
			AbsOutputFile:  "/out.js",
		},
	})
}
