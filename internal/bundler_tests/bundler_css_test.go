package bundler_tests

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
			ExternalSettings: config.ExternalSettings{
				PostResolve: config.ExternalMatchers{Exact: map[string]bool{
					"/external1.css": true,
					"/external2.css": true,
					"/external3.css": true,
					"/external4.css": true,
					"/external5.css": true,
				}},
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
		debugLogs: true,
		expectedCompileLog: `entry.js: DEBUG: Import "missing" will always be undefined because there is no matching export in "a.css"
`,
	})
}

func TestImportGlobalCSSFromJS(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import "./a.js"
				import "./b.js"
			`,
			"/a.js": `
				import * as stylesA from "./a.css"
				console.log('a', stylesA.a, stylesA.default.a)
			`,
			"/a.css": `
				.a { color: red }
			`,
			"/b.js": `
				import * as stylesB from "./b.css"
				console.log('b', stylesB.b, stylesB.default.b)
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

func TestImportLocalCSSFromJS(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import "./a.js"
				import "./b.js"
			`,
			"/a.js": `
				import * as stylesA from "./dir1/style.css"
				console.log('file 1', stylesA.button, stylesA.default.a)
			`,
			"/dir1/style.css": `
				.a { color: red }
				.button { display: none }
			`,
			"/b.js": `
				import * as stylesB from "./dir2/style.css"
				console.log('file 2', stylesB.button, stylesB.default.b)
			`,
			"/dir2/style.css": `
				.b { color: blue }
				.button { display: none }
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".css": config.LoaderLocalCSS,
			},
		},
	})
}

func TestImportLocalCSSFromJSMinifyIdentifiers(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import "./a.js"
				import "./b.js"
			`,
			"/a.js": `
				import * as stylesA from "./dir1/style.css"
				console.log('file 1', stylesA.button, stylesA.default.a)
			`,
			"/dir1/style.css": `
				.a { color: red }
				.button { display: none }
			`,
			"/b.js": `
				import * as stylesB from "./dir2/style.css"
				console.log('file 2', stylesB.button, stylesB.default.b)
			`,
			"/dir2/style.css": `
				.b { color: blue }
				.button { display: none }
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".css": config.LoaderLocalCSS,
			},
			MinifyIdentifiers: true,
		},
	})
}

func TestImportLocalCSSFromJSMinifyIdentifiersAvoidGlobalNames(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import "./global.css"
				import "./local.module.css"
			`,
			"/global.css": `
				:is(.a, .b, .c, .d, .e, .f, .g, .h, .i, .j, .k, .l, .m, .n, .o, .p, .q, .r, .s, .t, .u, .v, .w, .x, .y, .z),
				:is(.A, .B, .C, .D, .E, .F, .G, .H, .I, .J, .K, .L, .M, .N, .O, .P, .Q, .R, .S, .T, .U, .V, .W, .X, .Y, .Z),
				._ { color: red }
			`,
			"/local.module.css": `
				.rename-this { color: blue }
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExtensionToLoader: map[string]config.Loader{
				".js":         config.LoaderJS,
				".css":        config.LoaderCSS,
				".module.css": config.LoaderLocalCSS,
			},
			MinifyIdentifiers: true,
		},
	})
}

func TestImportCSSFromJSLocalVsGlobal(t *testing.T) {
	css := `
		.top_level { color: #000 }

		:global(.GLOBAL) { color: #001 }
		:local(.local) { color: #002 }

		div:global(.GLOBAL) { color: #003 }
		div:local(.local) { color: #004 }

		.top_level:global(div) { color: #005 }
		.top_level:local(div) { color: #006 }

		:global(div.GLOBAL) { color: #007 }
		:local(div.local) { color: #008 }

		div:global(span.GLOBAL) { color: #009 }
		div:local(span.local) { color: #00A }

		div:global(#GLOBAL_A.GLOBAL_B.GLOBAL_C):local(.local_a.local_b#local_c) { color: #00B }
		div:global(#GLOBAL_A .GLOBAL_B .GLOBAL_C):local(.local_a .local_b #local_c) { color: #00C }

		.nested {
			:global(&.GLOBAL) { color: #00D }
			:local(&.local) { color: #00E }

			&:global(.GLOBAL) { color: #00F }
			&:local(.local) { color: #010 }
		}

		:global(.GLOBAL_A .GLOBAL_B) { color: #011 }
		:local(.local_a .local_b) { color: #012 }

		div:global(.GLOBAL_A .GLOBAL_B):hover { color: #013 }
		div:local(.local_a .local_b):hover { color: #014 }

		div :global(.GLOBAL_A .GLOBAL_B) span { color: #015 }
		div :local(.local_a .local_b) span { color: #016 }

		div > :global(.GLOBAL_A ~ .GLOBAL_B) + span { color: #017 }
		div > :local(.local_a ~ .local_b) + span { color: #018 }

		div:global(+ .GLOBAL_A):hover { color: #019 }
		div:local(+ .local_a):hover { color: #01A }
	`

	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import normalStyles from "./normal.css"
				import globalStyles from "./LOCAL.global-css"
				import localStyles from "./LOCAL.local-css"

				console.log('should be empty:', normalStyles)
				console.log('fewer local names:', globalStyles)
				console.log('more local names:', localStyles)
			`,
			"/normal.css":       css,
			"/LOCAL.global-css": css,
			"/LOCAL.local-css":  css,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExtensionToLoader: map[string]config.Loader{
				".js":         config.LoaderJS,
				".css":        config.LoaderCSS,
				".global-css": config.LoaderGlobalCSS,
				".local-css":  config.LoaderLocalCSS,
			},
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
NOTE: An "@import" rule can only be used to import another CSS file, and "entry.js" is not a CSS file (it was loaded with the "js" loader).
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
NOTE: An "@import" rule can only be used to import another CSS file, and "entry.json" is not a CSS file (it was loaded with the "json" loader).
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
			ExternalSettings: config.ExternalSettings{
				PostResolve: config.ExternalMatchers{Exact: map[string]bool{
					"/src/external.png": true,
				}},
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
NOTE: You can't use a "url()" token to reference the file "js.js" because it was loaded with the "js" loader, which doesn't provide a URL to embed in the resulting CSS.
entry.css: ERROR: Cannot use "jsx.jsx" as a URL
NOTE: You can't use a "url()" token to reference the file "jsx.jsx" because it was loaded with the "jsx" loader, which doesn't provide a URL to embed in the resulting CSS.
entry.css: ERROR: Cannot use "ts.ts" as a URL
NOTE: You can't use a "url()" token to reference the file "ts.ts" because it was loaded with the "ts" loader, which doesn't provide a URL to embed in the resulting CSS.
entry.css: ERROR: Cannot use "tsx.tsx" as a URL
NOTE: You can't use a "url()" token to reference the file "tsx.tsx" because it was loaded with the "tsx" loader, which doesn't provide a URL to embed in the resulting CSS.
entry.css: ERROR: Cannot use "json.json" as a URL
NOTE: You can't use a "url()" token to reference the file "json.json" because it was loaded with the "json" loader, which doesn't provide a URL to embed in the resulting CSS.
entry.css: ERROR: Cannot use "css.css" as a URL
NOTE: You can't use a "url()" token to reference a CSS file, and "css.css" is a CSS file (it was loaded with the "css" loader).
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
			ExternalSettings: config.ExternalSettings{
				PreResolve: config.ExternalMatchers{Patterns: []config.WildcardPattern{
					{Suffix: ".png"},
				}},
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
			ExternalSettings: config.ExternalSettings{
				PreResolve: config.ExternalMatchers{Patterns: []config.WildcardPattern{
					{Suffix: ".png?baz"},
					{Suffix: ".png#baz"},
				}},
			},
		},
	})
}

func TestCSSNestingOldBrowser(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/nested-@layer.css":          `a { @layer base { color: red; } }`,
			"/nested-@media.css":          `a { @media screen { color: red; } }`,
			"/nested-ampersand-twice.css": `a { &, & { color: red; } }`,
			"/nested-ampersand-first.css": `a { &, b { color: red; } }`,
			"/nested-attribute.css":       `a { [href] { color: red; } }`,
			"/nested-colon.css":           `a { :hover { color: red; } }`,
			"/nested-dot.css":             `a { .cls { color: red; } }`,
			"/nested-greaterthan.css":     `a { > b { color: red; } }`,
			"/nested-hash.css":            `a { #id { color: red; } }`,
			"/nested-plus.css":            `a { + b { color: red; } }`,
			"/nested-tilde.css":           `a { ~ b { color: red; } }`,

			"/toplevel-ampersand-twice.css":  `&, & { color: red; }`,
			"/toplevel-ampersand-first.css":  `&, a { color: red; }`,
			"/toplevel-ampersand-second.css": `a, & { color: red; }`,
			"/toplevel-attribute.css":        `[href] { color: red; }`,
			"/toplevel-colon.css":            `:hover { color: red; }`,
			"/toplevel-dot.css":              `.cls { color: red; }`,
			"/toplevel-greaterthan.css":      `> b { color: red; }`,
			"/toplevel-hash.css":             `#id { color: red; }`,
			"/toplevel-plus.css":             `+ b { color: red; }`,
			"/toplevel-tilde.css":            `~ b { color: red; }`,

			"/media-ampersand-twice.css":  `@media screen { &, & { color: red; } }`,
			"/media-ampersand-first.css":  `@media screen { &, a { color: red; } }`,
			"/media-ampersand-second.css": `@media screen { a, & { color: red; } }`,
			"/media-attribute.css":        `@media screen { [href] { color: red; } }`,
			"/media-colon.css":            `@media screen { :hover { color: red; } }`,
			"/media-dot.css":              `@media screen { .cls { color: red; } }`,
			"/media-greaterthan.css":      `@media screen { > b { color: red; } }`,
			"/media-hash.css":             `@media screen { #id { color: red; } }`,
			"/media-plus.css":             `@media screen { + b { color: red; } }`,
			"/media-tilde.css":            `@media screen { ~ b { color: red; } }`,

			// See: https://github.com/evanw/esbuild/issues/3197
			"/page-no-warning.css": `@page { @top-left { background: red } }`,
		},
		entryPaths: []string{
			"/nested-@layer.css",
			"/nested-@media.css",
			"/nested-ampersand-twice.css",
			"/nested-ampersand-first.css",
			"/nested-attribute.css",
			"/nested-colon.css",
			"/nested-dot.css",
			"/nested-greaterthan.css",
			"/nested-hash.css",
			"/nested-plus.css",
			"/nested-tilde.css",

			"/toplevel-ampersand-twice.css",
			"/toplevel-ampersand-first.css",
			"/toplevel-ampersand-second.css",
			"/toplevel-attribute.css",
			"/toplevel-colon.css",
			"/toplevel-dot.css",
			"/toplevel-greaterthan.css",
			"/toplevel-hash.css",
			"/toplevel-plus.css",
			"/toplevel-tilde.css",

			"/media-ampersand-twice.css",
			"/media-ampersand-first.css",
			"/media-ampersand-second.css",
			"/media-attribute.css",
			"/media-colon.css",
			"/media-dot.css",
			"/media-greaterthan.css",
			"/media-hash.css",
			"/media-plus.css",
			"/media-tilde.css",

			"/page-no-warning.css",
		},
		options: config.Options{
			Mode:                   config.ModeBundle,
			AbsOutputDir:           "/out",
			UnsupportedCSSFeatures: compat.Nesting | compat.IsPseudoClass,
			OriginalTargetEnv:      "chrome10",
		},
		expectedScanLog: `media-ampersand-first.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
media-ampersand-second.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
media-ampersand-twice.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
media-ampersand-twice.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
media-greaterthan.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
media-plus.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
media-tilde.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
nested-@layer.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
nested-@media.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
nested-ampersand-first.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
nested-ampersand-twice.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
nested-attribute.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
nested-colon.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
nested-dot.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
nested-greaterthan.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
nested-hash.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
nested-plus.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
nested-tilde.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
toplevel-ampersand-first.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
toplevel-ampersand-second.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
toplevel-ampersand-twice.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
toplevel-ampersand-twice.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
toplevel-greaterthan.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
toplevel-plus.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
toplevel-tilde.css: WARNING: CSS nesting syntax is not supported in the configured target environment (chrome10)
`,
	})
}

// The mapping of JS entry point to associated CSS bundle isn't necessarily 1:1.
// Here is a case where it isn't. Two JS entry points share the same associated
// CSS bundle. This must be reflected in the metafile by only having the JS
// entry points point to the associated CSS bundle but not the other way around
// (since there isn't one JS entry point to point to). This test mainly exists
// to document this edge case.
func TestMetafileCSSBundleTwoToOne(t *testing.T) {
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/foo/entry.js": `
				import '../common.css'
				console.log('foo')
			`,
			"/bar/entry.js": `
				import '../common.css'
				console.log('bar')
			`,
			"/common.css": `
				body { color: red }
			`,
		},
		entryPaths: []string{
			"/foo/entry.js",
			"/bar/entry.js",
		},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			EntryPathTemplate: []config.PathTemplate{
				// "[ext]/[hash]"
				{Data: "./", Placeholder: config.ExtPlaceholder},
				{Data: "/", Placeholder: config.HashPlaceholder},
			},
			NeedsMetafile: true,
		},
	})
}

func TestDeduplicateRules(t *testing.T) {
	// These are done as bundler tests instead of parser tests because rule
	// deduplication now happens during linking (so that it has effects across files)
	css_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/yes0.css": "a { color: red; color: green; color: red }",
			"/yes1.css": "a { color: red } a { color: green } a { color: red }",
			"/yes2.css": "@media screen { a { color: red } } @media screen { a { color: red } }",
			"/yes3.css": "@media screen { a { color: red } } @media screen { & a { color: red } }",

			"/no0.css": "@media screen { a { color: red } } @media screen { b a& { color: red } }",
			"/no1.css": "@media screen { a { color: red } } @media screen { a[x] { color: red } }",
			"/no2.css": "@media screen { a { color: red } } @media screen { a.x { color: red } }",
			"/no3.css": "@media screen { a { color: red } } @media screen { a#x { color: red } }",
			"/no4.css": "@media screen { a { color: red } } @media screen { a:x { color: red } }",
			"/no5.css": "@media screen { a:x { color: red } } @media screen { a:x(y) { color: red } }",
			"/no6.css": "@media screen { a b { color: red } } @media screen { a + b { color: red } }",

			"/across-files.css":   "@import 'across-files-0.css'; @import 'across-files-1.css'; @import 'across-files-2.css';",
			"/across-files-0.css": "a { color: red; color: red }",
			"/across-files-1.css": "a { color: green }",
			"/across-files-2.css": "a { color: red }",

			"/across-files-url.css":   "@import 'across-files-url-0.css'; @import 'across-files-url-1.css'; @import 'across-files-url-2.css';",
			"/across-files-url-0.css": "@import 'http://example.com/some.css'; @font-face { src: url(http://example.com/some.font); }",
			"/across-files-url-1.css": "@font-face { src: url(http://example.com/some.other.font); }",
			"/across-files-url-2.css": "@font-face { src: url(http://example.com/some.font); }",
		},
		entryPaths: []string{
			"/yes0.css",
			"/yes1.css",
			"/yes2.css",
			"/yes3.css",

			"/no0.css",
			"/no1.css",
			"/no2.css",
			"/no3.css",
			"/no4.css",
			"/no5.css",
			"/no6.css",

			"/across-files.css",
			"/across-files-url.css",
		},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			MinifySyntax: true,
		},
	})
}
