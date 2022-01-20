package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/compat"
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
		expectedScanLog: `entry.css: ERROR: Could not resolve "./missing.css"
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
				@import "./external5.css" screen;
			`,
			"/internal.css": `
				@import "./external5.css" print;
				.before { color: red }
			`,
			"/charset1.css": `
				@charset "UTF-8";
				@import "./external3.css";
				@import "./external4.css";
				@import "./external5.css";
				@import "https://www.example.com/style1.css";
				@import "https://www.example.com/style2.css";
				@import "https://www.example.com/style3.css" print;
				.middle { color: green }
			`,
			"/charset2.css": `
				@charset "UTF-8";
				@import "./external3.css";
				@import "./external5.css" screen;
				@import "https://www.example.com/style1.css";
				@import "https://www.example.com/style3.css";
				.after { color: blue }
			`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExternalModules: config.ExternalModules{
				AbsPaths: map[string]bool{
					"/external1.css": true,
					"/external2.css": true,
					"/external3.css": true,
					"/external4.css": true,
					"/external5.css": true,
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
		expectedCompileLog: `entry.js: ERROR: No matching export in "a.css" for import "missing"
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
		expectedCompileLog: `entry.js: WARNING: Import "missing" will always be undefined because there is no matching export in "a.css"
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
		expectedScanLog: `entry.js: ERROR: Cannot import "entry.css" into a JavaScript file without an output path configured
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
		expectedScanLog: `entry.css: ERROR: Cannot import "entry.js" into a CSS file
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
		expectedScanLog: `entry.css: ERROR: Cannot import "entry.json" into a CSS file
`,
	})
}

func TestMissingImportURLInCSS(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/entry.css": `
				a { background: url(./one.png); }
				b { background: url("./two.png"); }
			`,
		},
		entryPaths: []string{"/src/entry.css"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
		expectedScanLog: `src/entry.css: ERROR: Could not resolve "./one.png"
src/entry.css: ERROR: Could not resolve "./two.png"
`,
	})
}

func TestExternalImportURLInCSS(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/entry.css": `
				div:after {
					content: 'If this is recognized, the path should become "../src/external.png"';
					background: url(./external.png);
				}

				/* These URLs should be external automatically */
				a { background: url(http://example.com/images/image.png) }
				b { background: url(https://example.com/images/image.png) }
				c { background: url(//example.com/images/image.png) }
				d { background: url(data:image/png;base64,iVBORw0KGgo=) }
				path { fill: url(#filter) }
			`,
		},
		entryPaths: []string{"/src/entry.css"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExternalModules: config.ExternalModules{
				AbsPaths: map[string]bool{
					"/src/external.png": true,
				},
			},
		},
	})
}

func TestInvalidImportURLInCSS(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
				a {
					background: url(./js.js);
					background: url("./jsx.jsx");
					background: url(./ts.ts);
					background: url('./tsx.tsx');
					background: url(./json.json);
					background: url(./css.css);
				}
			`,
			"/js.js":     `export default 123`,
			"/jsx.jsx":   `export default 123`,
			"/ts.ts":     `export default 123`,
			"/tsx.tsx":   `export default 123`,
			"/json.json": `{ "test": true }`,
			"/css.css":   `a { color: red }`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
		expectedScanLog: `entry.css: ERROR: Cannot use "js.js" as a URL
entry.css: ERROR: Cannot use "jsx.jsx" as a URL
entry.css: ERROR: Cannot use "ts.ts" as a URL
entry.css: ERROR: Cannot use "tsx.tsx" as a URL
entry.css: ERROR: Cannot use "json.json" as a URL
entry.css: ERROR: Cannot use "css.css" as a URL
`,
	})
}

func TestTextImportURLInCSSText(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
				a {
					background: url(./example.txt);
				}
			`,
			"/example.txt": `This is some text.`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestDataURLImportURLInCSS(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
				a {
					background: url(./example.png);
				}
			`,
			"/example.png": "\x89\x50\x4E\x47\x0D\x0A\x1A\x0A",
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExtensionToLoader: map[string]config.Loader{
				".css": config.LoaderCSS,
				".png": config.LoaderDataURL,
			},
		},
	})
}

func TestBinaryImportURLInCSS(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
				a {
					background: url(./example.png);
				}
			`,
			"/example.png": "\x89\x50\x4E\x47\x0D\x0A\x1A\x0A",
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExtensionToLoader: map[string]config.Loader{
				".css": config.LoaderCSS,
				".png": config.LoaderBinary,
			},
		},
	})
}

func TestBase64ImportURLInCSS(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
				a {
					background: url(./example.png);
				}
			`,
			"/example.png": "\x89\x50\x4E\x47\x0D\x0A\x1A\x0A",
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExtensionToLoader: map[string]config.Loader{
				".css": config.LoaderCSS,
				".png": config.LoaderBase64,
			},
		},
	})
}

func TestFileImportURLInCSS(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
				@import "./one.css";
				@import "./two.css";
			`,
			"/one.css": `
				a { background: url(./example.data) }
			`,
			"/two.css": `
				b { background: url(./example.data) }
			`,
			"/example.data": "This is some data.",
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExtensionToLoader: map[string]config.Loader{
				".css":  config.LoaderCSS,
				".data": config.LoaderFile,
			},
		},
	})
}

func TestIgnoreURLsInAtRulePrelude(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
				/* This should not generate a path resolution error */
				@supports (background: url(ignored.png)) {
					a { color: red }
				}
			`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestPackageURLsInCSS(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
			  @import "test.css";

				a { background: url(a/1.png); }
				b { background: url(b/2.png); }
				c { background: url(c/3.png); }
			`,
			"/test.css":             `.css { color: red }`,
			"/a/1.png":              `a-1`,
			"/node_modules/b/2.png": `b-2-node_modules`,
			"/c/3.png":              `c-3`,
			"/node_modules/c/3.png": `c-3-node_modules`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExtensionToLoader: map[string]config.Loader{
				".css": config.LoaderCSS,
				".png": config.LoaderBase64,
			},
		},
	})
}

func TestCSSAtImportExtensionOrderCollision(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			// This should avoid picking ".js" because it's explicitly configured as non-CSS
			"/entry.css": `@import "./test";`,
			"/test.js":   `console.log('js')`,
			"/test.css":  `.css { color: red }`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:           config.ModeBundle,
			AbsOutputFile:  "/out.css",
			ExtensionOrder: []string{".js", ".css"},
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".css": config.LoaderCSS,
			},
		},
	})
}

func TestCSSAtImportExtensionOrderCollisionUnsupported(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			// This still shouldn't pick ".js" even though ".sass" isn't ".css"
			"/entry.css": `@import "./test";`,
			"/test.js":   `console.log('js')`,
			"/test.sass": `// some code`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:           config.ModeBundle,
			AbsOutputFile:  "/out.css",
			ExtensionOrder: []string{".js", ".sass"},
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".css": config.LoaderCSS,
			},
		},
		expectedScanLog: `entry.css: ERROR: No loader is configured for ".sass" files: test.sass
`,
	})
}

func TestCSSAtImportConditionsNoBundle(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `@import "./print.css" print;`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			AbsOutputFile: "/out.css",
		},
	})
}

func TestCSSAtImportConditionsBundleExternal(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `@import "https://example.com/print.css" print;`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.css",
		},
	})
}

func TestCSSAtImportConditionsBundleExternalConditionWithURL(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
				@import "https://example.com/foo.css" (foo: url("foo.png")) and (bar: url("bar.png"));
			`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.css",
		},
	})
}

func TestCSSAtImportConditionsBundle(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `@import "./print.css" print;`,
			"/print.css": `body { color: red }`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.css",
		},
		expectedScanLog: `entry.css: ERROR: Bundling with conditional "@import" rules is not currently supported
`,
	})
}

// This test mainly just makes sure that this scenario doesn't crash
func TestCSSAndJavaScriptCodeSplittingIssue1064(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import shared from './shared.js'
				console.log(shared() + 1)
			`,
			"/b.js": `
				import shared from './shared.js'
				console.log(shared() + 2)
			`,
			"/c.css": `
				@import "./shared.css";
				body { color: red }
			`,
			"/d.css": `
				@import "./shared.css";
				body { color: blue }
			`,
			"/shared.js": `
				export default function() { return 3 }
			`,
			"/shared.css": `
				body { background: black }
			`,
		},
		entryPaths: []string{
			"/a.js",
			"/b.js",
			"/c.css",
			"/d.css",
		},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatESModule,
			CodeSplitting: true,
			AbsOutputDir:  "/out",
		},
	})
}

func TestCSSExternalQueryAndHashNoMatchIssue1822(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
				a { background: url(foo/bar.png?baz) }
				b { background: url(foo/bar.png#baz) }
			`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.css",
			ExternalModules: config.ExternalModules{
				Patterns: []config.WildcardPattern{
					{Suffix: ".png"},
				},
			},
		},
		expectedScanLog: `entry.css: ERROR: Could not resolve "foo/bar.png?baz"
NOTE: You can mark the path "foo/bar.png?baz" as external to exclude it from the bundle, which will remove this error.
entry.css: ERROR: Could not resolve "foo/bar.png#baz"
NOTE: You can mark the path "foo/bar.png#baz" as external to exclude it from the bundle, which will remove this error.
`,
	})
}

func TestCSSExternalQueryAndHashMatchIssue1822(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
				a { background: url(foo/bar.png?baz) }
				b { background: url(foo/bar.png#baz) }
			`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.css",
			ExternalModules: config.ExternalModules{
				Patterns: []config.WildcardPattern{
					{Suffix: ".png?baz"},
					{Suffix: ".png#baz"},
				},
			},
		},
	})
}

func TestCSSNestingOldBrowser(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
				a { &:hover { color: red; } }
			`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:                   config.ModeBundle,
			AbsOutputFile:          "/out.css",
			UnsupportedCSSFeatures: compat.Nesting,
			OriginalTargetEnv:      "chrome10",
		},
		expectedScanLog: `entry.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
`,
	})
}
