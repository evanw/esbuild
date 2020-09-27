package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

var css_suite = suite{
	name: "css",
}

func TestCSSEntryPoint(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
				body {
					background: white;
					color: black }
			`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.css",
		},
	})
}

func TestCSSAtImportMissing(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
				@import "./missing.css";
			`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.css",
		},
		expectedScanLog: `/entry.css: error: Could not resolve "./missing.css"
`,
	})
}

func TestCSSAtImportExternal(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
				@import "./internal.css";
				@import "./external1.css";
				@import "./external2.css";
				@import "./charset1.css";
				@import "./charset2.css";
			`,
			"/internal.css": `
				.before { color: red }
			`,
			"/charset1.css": `
				@charset "UTF-8";
				.middle { color: green }
			`,
			"/charset2.css": `
				@charset "UTF-8";
				.after { color: blue }
			`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.css",
			ExternalModules: config.ExternalModules{
				AbsPaths: map[string]bool{
					"/external1.css": true,
					"/external2.css": true,
				},
			},
		},
	})
}

func TestCSSAtImport(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
				@import "./a.css";
				@import "./b.css";
				.entry { color: red }
			`,
			"/a.css": `
				@import "./shared.css";
				.a { color: green }
			`,
			"/b.css": `
				@import "./shared.css";
				.b { color: blue }
			`,
			"/shared.css": `
				.shared { color: black }
			`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.css",
		},
	})
}

func TestCSSFromJSMissingImport(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {missing} from "./a.css"
				console.log(missing)
			`,
			"/a.css": `
				.a { color: red }
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
		expectedCompileLog: `/entry.js: error: No matching export for import "missing"
`,
	})
}

func TestCSSFromJSMissingStarImport(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from "./a.css"
				console.log(ns.missing)
			`,
			"/a.css": `
				.a { color: red }
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
		expectedCompileLog: `/entry.js: warning: No matching export for import "missing"
`,
	})
}

func TestImportCSSFromJS(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import "./a.js"
				import "./b.js"
			`,
			"/a.js": `
				import "./a.css";
				console.log('a')
			`,
			"/a.css": `
				.a { color: red }
			`,
			"/b.js": `
				import "./b.css";
				console.log('b')
			`,
			"/b.css": `
				.b { color: blue }
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestImportCSSFromJSWriteToStdout(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import "./entry.css"
			`,
			"/entry.css": `
				.entry { color: red }
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			WriteToStdout: true,
		},
		expectedScanLog: `/entry.js: error: Cannot import "/entry.css" into a JavaScript file without an output path configured
`,
	})
}

func TestImportJSFromCSS(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export default 123
			`,
			"/entry.css": `
				@import "./entry.js";
			`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
		expectedScanLog: `/entry.css: error: Cannot import "/entry.js" into a CSS file
`,
	})
}

func TestImportJSONFromCSS(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.json": `
				{}
			`,
			"/entry.css": `
				@import "./entry.json";
			`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
		expectedScanLog: `/entry.css: error: Cannot import "/entry.json" into a CSS file
`,
	})
}
