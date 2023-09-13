package bundler_tests

import (
	"testing"

	"github.com/evanw/esbuild/internal/bundler"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
)

var loader_suite = suite{
	name: "loader",
}

func TestLoaderFile(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.svg'))
			`,
			"/test.svg": "<svg></svg>",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out/",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".svg": config.LoaderFile,
			},
		},
	})
}

func TestLoaderFileMultipleNoCollision(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(
					require('./a/test.txt'),
					require('./b/test.txt'),
				)
			`,

			// Two files with the same contents but different paths
			"/a/test.txt": "test",
			"/b/test.txt": "test",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/dist/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".txt": config.LoaderFile,
			},
		},
	})
}

func TestJSXSyntaxInJSWithJSXLoader(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(<div/>)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js": config.LoaderJSX,
			},
		},
	})
}

func TestJSXPreserveCapitalLetter(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.jsx": `
				import { mustStartWithUpperCaseLetter as Test } from './foo'
				console.log(<Test/>)
			`,
			"/foo.js": `
				export class mustStartWithUpperCaseLetter {}
			`,
		},
		entryPaths: []string{"/entry.jsx"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			JSX: config.JSXOptions{
				Parse:    true,
				Preserve: true,
			},
		},
	})
}

func TestJSXPreserveCapitalLetterMinify(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.jsx": `
				import { mustStartWithUpperCaseLetter as XYYYY } from './foo'
				console.log(<XYYYY tag-must-start-with-capital-letter />)
			`,
			"/foo.js": `
				export class mustStartWithUpperCaseLetter {}
			`,
		},
		entryPaths: []string{"/entry.jsx"},
		options: config.Options{
			Mode:              config.ModeBundle,
			AbsOutputFile:     "/out.js",
			MinifyIdentifiers: true,
			JSX: config.JSXOptions{
				Parse:    true,
				Preserve: true,
			},
		},
	})
}

func TestJSXPreserveCapitalLetterMinifyNested(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.jsx": `
				x = () => {
					class XYYYYY {} // This should be named "Y" due to frequency analysis
					return <XYYYYY tag-must-start-with-capital-letter />
				}
			`,
		},
		entryPaths: []string{"/entry.jsx"},
		options: config.Options{
			Mode:              config.ModeBundle,
			AbsOutputFile:     "/out.js",
			MinifyIdentifiers: true,
			JSX: config.JSXOptions{
				Parse:    true,
				Preserve: true,
			},
		},
	})
}

func TestRequireCustomExtensionString(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.custom'))
			`,
			"/test.custom": `#include <stdio.h>`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":     config.LoaderJS,
				".custom": config.LoaderText,
			},
		},
	})
}

func TestRequireCustomExtensionBase64(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.custom'))
			`,
			"/test.custom": "a\x00b\x80c\xFFd",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":     config.LoaderJS,
				".custom": config.LoaderBase64,
			},
		},
	})
}

func TestRequireCustomExtensionDataURL(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.custom'))
			`,
			"/test.custom": "a\x00b\x80c\xFFd",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":     config.LoaderJS,
				".custom": config.LoaderDataURL,
			},
		},
	})
}

func TestRequireCustomExtensionPreferLongest(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.txt'), require('./test.base64.txt'))
			`,
			"/test.txt":        `test.txt`,
			"/test.base64.txt": `test.base64.txt`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":         config.LoaderJS,
				".txt":        config.LoaderText,
				".base64.txt": config.LoaderBase64,
			},
		},
	})
}

func TestAutoDetectMimeTypeFromExtension(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.svg'))
			`,
			"/test.svg": "a\x00b\x80c\xFFd",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".svg": config.LoaderDataURL,
			},
		},
	})
}

func TestLoaderJSONCommonJSAndES6(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const x_json = require('./x.json')
				import y_json from './y.json'
				import {small, if as fi} from './z.json'
				console.log(x_json, y_json, small, fi)
			`,
			"/x.json": `{"x": true}`,
			"/y.json": `{"y1": true, "y2": false}`,
			"/z.json": `{
				"big": "this is a big long line of text that should be discarded",
				"small": "some small text",
				"if": "test keyword imports"
			}`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestLoaderJSONInvalidIdentifierES6(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as ns from './test.json'
				import * as ns2 from './test2.json'
				console.log(ns['invalid-identifier'], ns2)
			`,
			"/test.json":  `{"invalid-identifier": true}`,
			"/test2.json": `{"invalid-identifier": true}`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestLoaderJSONMissingES6(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {missing} from './test.json'
			`,
			"/test.json": `{"present": true}`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedCompileLog: `entry.js: ERROR: No matching export in "test.json" for import "missing"
`,
	})
}

func TestLoaderTextCommonJSAndES6(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const x_txt = require('./x.txt')
				import y_txt from './y.txt'
				console.log(x_txt, y_txt)
			`,
			"/x.txt": "x",
			"/y.txt": "y",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestLoaderBase64CommonJSAndES6(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const x_b64 = require('./x.b64')
				import y_b64 from './y.b64'
				console.log(x_b64, y_b64)
			`,
			"/x.b64": "x",
			"/y.b64": "y",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".b64": config.LoaderBase64,
			},
		},
	})
}

func TestLoaderDataURLCommonJSAndES6(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const x_url = require('./x.txt')
				import y_url from './y.txt'
				console.log(x_url, y_url)
			`,
			"/x.txt": "x",
			"/y.txt": "y",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".txt": config.LoaderDataURL,
			},
		},
	})
}

func TestLoaderFileCommonJSAndES6(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const x_url = require('./x.txt')
				import y_url from './y.txt'
				console.log(x_url, y_url)
			`,
			"/x.txt": "x",
			"/y.txt": "y",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".txt": config.LoaderFile,
			},
		},
	})
}

func TestLoaderFileRelativePathJS(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/entries/entry.js": `
				import x from '../images/image.png'
				console.log(x)
			`,
			"/src/images/image.png": "x",
		},
		entryPaths: []string{"/src/entries/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputBase: "/src",
			AbsOutputDir:  "/out",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".png": config.LoaderFile,
			},
		},
	})
}

func TestLoaderFileRelativePathCSS(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/entries/entry.css": `
				div {
					background: url(../images/image.png);
				}
			`,
			"/src/images/image.png": "x",
		},
		entryPaths: []string{"/src/entries/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputBase: "/src",
			AbsOutputDir:  "/out",
			ExtensionToLoader: map[string]config.Loader{
				".css": config.LoaderCSS,
				".png": config.LoaderFile,
			},
		},
	})
}

func TestLoaderFileRelativePathAssetNamesJS(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/entries/entry.js": `
				import x from '../images/image.png'
				console.log(x)
			`,
			"/src/images/image.png": "x",
		},
		entryPaths: []string{"/src/entries/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputBase: "/src",
			AbsOutputDir:  "/out",
			AssetPathTemplate: []config.PathTemplate{
				{Data: "", Placeholder: config.DirPlaceholder},
				{Data: "/", Placeholder: config.NamePlaceholder},
				{Data: "-", Placeholder: config.HashPlaceholder},
			},
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".png": config.LoaderFile,
			},
		},
	})
}

func TestLoaderFileExtPathAssetNamesJS(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/entries/entry.js": `
				import x from '../images/image.png'
				import y from '../uploads/file.txt'
				console.log(x, y)
			`,
			"/src/images/image.png": "x",
			"/src/uploads/file.txt": "y",
		},
		entryPaths: []string{"/src/entries/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputBase: "/src",
			AbsOutputDir:  "/out",
			AssetPathTemplate: []config.PathTemplate{
				{Data: "", Placeholder: config.ExtPlaceholder},
				{Data: "/", Placeholder: config.NamePlaceholder},
				{Data: "-", Placeholder: config.HashPlaceholder},
			},
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".png": config.LoaderFile,
				".txt": config.LoaderFile,
			},
		},
	})
}

func TestLoaderFileRelativePathAssetNamesCSS(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/entries/entry.css": `
				div {
					background: url(../images/image.png);
				}
			`,
			"/src/images/image.png": "x",
		},
		entryPaths: []string{"/src/entries/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputBase: "/src",
			AbsOutputDir:  "/out",
			AssetPathTemplate: []config.PathTemplate{
				{Data: "", Placeholder: config.DirPlaceholder},
				{Data: "/", Placeholder: config.NamePlaceholder},
				{Data: "-", Placeholder: config.HashPlaceholder},
			},
			ExtensionToLoader: map[string]config.Loader{
				".css": config.LoaderCSS,
				".png": config.LoaderFile,
			},
		},
	})
}

func TestLoaderFilePublicPathJS(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/entries/entry.js": `
				import x from '../images/image.png'
				console.log(x)
			`,
			"/src/images/image.png": "x",
		},
		entryPaths: []string{"/src/entries/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputBase: "/src",
			AbsOutputDir:  "/out",
			PublicPath:    "https://example.com",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".png": config.LoaderFile,
			},
		},
	})
}

func TestLoaderFilePublicPathCSS(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/entries/entry.css": `
				div {
					background: url(../images/image.png);
				}
			`,
			"/src/images/image.png": "x",
		},
		entryPaths: []string{"/src/entries/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputBase: "/src",
			AbsOutputDir:  "/out",
			PublicPath:    "https://example.com",
			ExtensionToLoader: map[string]config.Loader{
				".css": config.LoaderCSS,
				".png": config.LoaderFile,
			},
		},
	})
}

func TestLoaderFilePublicPathAssetNamesJS(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/entries/entry.js": `
				import x from '../images/image.png'
				console.log(x)
			`,
			"/src/images/image.png": "x",
		},
		entryPaths: []string{"/src/entries/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputBase: "/src",
			AbsOutputDir:  "/out",
			PublicPath:    "https://example.com",
			AssetPathTemplate: []config.PathTemplate{
				{Data: "", Placeholder: config.DirPlaceholder},
				{Data: "/", Placeholder: config.NamePlaceholder},
				{Data: "-", Placeholder: config.HashPlaceholder},
			},
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".png": config.LoaderFile,
			},
		},
	})
}

func TestLoaderFilePublicPathAssetNamesCSS(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/entries/entry.css": `
				div {
					background: url(../images/image.png);
				}
			`,
			"/src/images/image.png": "x",
		},
		entryPaths: []string{"/src/entries/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputBase: "/src",
			AbsOutputDir:  "/out",
			PublicPath:    "https://example.com",
			AssetPathTemplate: []config.PathTemplate{
				{Data: "", Placeholder: config.DirPlaceholder},
				{Data: "/", Placeholder: config.NamePlaceholder},
				{Data: "-", Placeholder: config.HashPlaceholder},
			},
			ExtensionToLoader: map[string]config.Loader{
				".css": config.LoaderCSS,
				".png": config.LoaderFile,
			},
		},
	})
}

func TestLoaderFileOneSourceTwoDifferentOutputPathsJS(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/entries/entry.js": `
				import '../shared/common.js'
			`,
			"/src/entries/other/entry.js": `
				import '../../shared/common.js'
			`,
			"/src/shared/common.js": `
				import x from './common.png'
				console.log(x)
			`,
			"/src/shared/common.png": "x",
		},
		entryPaths: []string{
			"/src/entries/entry.js",
			"/src/entries/other/entry.js",
		},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputBase: "/src",
			AbsOutputDir:  "/out",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".png": config.LoaderFile,
			},
		},
	})
}

func TestLoaderFileOneSourceTwoDifferentOutputPathsCSS(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/entries/entry.css": `
				@import "../shared/common.css";
			`,
			"/src/entries/other/entry.css": `
				@import "../../shared/common.css";
			`,
			"/src/shared/common.css": `
				div {
					background: url(common.png);
				}
			`,
			"/src/shared/common.png": "x",
		},
		entryPaths: []string{
			"/src/entries/entry.css",
			"/src/entries/other/entry.css",
		},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputBase: "/src",
			AbsOutputDir:  "/out",
			ExtensionToLoader: map[string]config.Loader{
				".css": config.LoaderCSS,
				".png": config.LoaderFile,
			},
		},
	})
}

func TestLoaderJSONNoBundle(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/test.json": `{"test": 123, "invalid-identifier": true}`,
		},
		entryPaths: []string{"/test.json"},
		options: config.Options{
			AbsOutputFile: "/out.js",
		},
	})
}

func TestLoaderJSONNoBundleES6(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/test.json": `{"test": 123, "invalid-identifier": true}`,
		},
		entryPaths: []string{"/test.json"},
		options: config.Options{
			Mode:                  config.ModeConvertFormat,
			OutputFormat:          config.FormatESModule,
			UnsupportedJSFeatures: compat.ArbitraryModuleNamespaceNames,
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLoaderJSONNoBundleES6ArbitraryModuleNamespaceNames(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/test.json": `{"test": 123, "invalid-identifier": true}`,
		},
		entryPaths: []string{"/test.json"},
		options: config.Options{
			Mode:          config.ModeConvertFormat,
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestLoaderJSONNoBundleCommonJS(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/test.json": `{"test": 123, "invalid-identifier": true}`,
		},
		entryPaths: []string{"/test.json"},
		options: config.Options{
			Mode:          config.ModeConvertFormat,
			OutputFormat:  config.FormatCommonJS,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestLoaderJSONNoBundleIIFE(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/test.json": `{"test": 123, "invalid-identifier": true}`,
		},
		entryPaths: []string{"/test.json"},
		options: config.Options{
			Mode:          config.ModeConvertFormat,
			OutputFormat:  config.FormatIIFE,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestLoaderJSONSharedWithMultipleEntriesIssue413(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import data from './data.json'
				console.log('a:', data)
			`,
			"/b.js": `
				import data from './data.json'
				console.log('b:', data)
			`,
			"/data.json": `{"test": 123}`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			OutputFormat: config.FormatESModule,
			AbsOutputDir: "/out",
		},
	})
}

func TestLoaderFileWithQueryParameter(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				// Each of these should have a separate identity (i.e. end up in the output file twice)
				import foo from './file.txt?foo'
				import bar from './file.txt?bar'
				console.log(foo, bar)
			`,
			"/file.txt": `This is some text`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".txt": config.LoaderFile,
			},
		},
	})
}

func TestLoaderFromExtensionWithQueryParameter(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import foo from './file.abc?query.xyz'
				console.log(foo)
			`,
			"/file.abc": `This should not be base64 encoded`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".abc": config.LoaderText,
				".xyz": config.LoaderBase64,
			},
		},
	})
}

func TestLoaderDataURLTextCSS(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
				@import "data:text/css,body{color:%72%65%64}";
				@import "data:text/css;base64,Ym9keXtiYWNrZ3JvdW5kOmJsdWV9";
				@import "data:text/css;charset=UTF-8,body{color:%72%65%64}";
				@import "data:text/css;charset=UTF-8;base64,Ym9keXtiYWNrZ3JvdW5kOmJsdWV9";
			`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestLoaderDataURLTextCSSCannotImport(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
				@import "data:text/css,@import './other.css';";
			`,
			"/other.css": `
				div { should-not-be-imported: true }
			`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
		expectedScanLog: `<data:text/css,@import './other.css';>: ERROR: Could not resolve "./other.css"
`,
	})
}

func TestLoaderDataURLTextJavaScript(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import "data:text/javascript,console.log('%31%32%33')";
				import "data:text/javascript;base64,Y29uc29sZS5sb2coMjM0KQ==";
				import "data:text/javascript;charset=UTF-8,console.log(%31%32%33)";
				import "data:text/javascript;charset=UTF-8;base64,Y29uc29sZS5sb2coMjM0KQ==";
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestLoaderDataURLTextJavaScriptCannotImport(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import "data:text/javascript,import './other.js'"
			`,
			"/other.js": `
				shouldNotBeImported = true
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
		expectedScanLog: `<data:text/javascript,import './other.js'>: ERROR: Could not resolve "./other.js"
`,
	})
}

// The "+" character must not be interpreted as a " " character
func TestLoaderDataURLTextJavaScriptPlusCharacter(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import "data:text/javascript,console.log(1+2)";
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestLoaderDataURLApplicationJSON(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import a from 'data:application/json,"%31%32%33"';
				import b from 'data:application/json;base64,eyJ3b3JrcyI6dHJ1ZX0=';
				import c from 'data:application/json;charset=UTF-8,%31%32%33';
				import d from 'data:application/json;charset=UTF-8;base64,eyJ3b3JrcyI6dHJ1ZX0=';
				console.log([
					a, b, c, d,
				])
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestLoaderDataURLUnknownMIME(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import a from 'data:some/thing;what,someData%31%32%33';
				import b from 'data:other/thing;stuff;base64,c29tZURhdGEyMzQ=';
				console.log(a, b)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestLoaderDataURLExtensionBasedMIME(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.foo": `
				export { default as css }   from "./example.css"
				export { default as eot }   from "./example.eot"
				export { default as gif }   from "./example.gif"
				export { default as htm }   from "./example.htm"
				export { default as html }  from "./example.html"
				export { default as jpeg }  from "./example.jpeg"
				export { default as jpg }   from "./example.jpg"
				export { default as js }    from "./example.js"
				export { default as json }  from "./example.json"
				export { default as mjs }   from "./example.mjs"
				export { default as otf }   from "./example.otf"
				export { default as pdf }   from "./example.pdf"
				export { default as png }   from "./example.png"
				export { default as sfnt }  from "./example.sfnt"
				export { default as svg }   from "./example.svg"
				export { default as ttf }   from "./example.ttf"
				export { default as wasm }  from "./example.wasm"
				export { default as webp }  from "./example.webp"
				export { default as woff }  from "./example.woff"
				export { default as woff2 } from "./example.woff2"
				export { default as xml }   from "./example.xml"
			`,
			"/example.css":   `css`,
			"/example.eot":   `eot`,
			"/example.gif":   `gif`,
			"/example.htm":   `htm`,
			"/example.html":  `html`,
			"/example.jpeg":  `jpeg`,
			"/example.jpg":   `jpg`,
			"/example.js":    `js`,
			"/example.json":  `json`,
			"/example.mjs":   `mjs`,
			"/example.otf":   `otf`,
			"/example.pdf":   `pdf`,
			"/example.png":   `png`,
			"/example.sfnt":  `sfnt`,
			"/example.svg":   `svg`,
			"/example.ttf":   `ttf`,
			"/example.wasm":  `wasm`,
			"/example.webp":  `webp`,
			"/example.woff":  `woff`,
			"/example.woff2": `woff2`,
			"/example.xml":   `xml`,
		},
		entryPaths: []string{"/entry.foo"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExtensionToLoader: map[string]config.Loader{
				".foo":   config.LoaderJS,
				".css":   config.LoaderDataURL,
				".eot":   config.LoaderDataURL,
				".gif":   config.LoaderDataURL,
				".htm":   config.LoaderDataURL,
				".html":  config.LoaderDataURL,
				".jpeg":  config.LoaderDataURL,
				".jpg":   config.LoaderDataURL,
				".js":    config.LoaderDataURL,
				".json":  config.LoaderDataURL,
				".mjs":   config.LoaderDataURL,
				".otf":   config.LoaderDataURL,
				".pdf":   config.LoaderDataURL,
				".png":   config.LoaderDataURL,
				".sfnt":  config.LoaderDataURL,
				".svg":   config.LoaderDataURL,
				".ttf":   config.LoaderDataURL,
				".wasm":  config.LoaderDataURL,
				".webp":  config.LoaderDataURL,
				".woff":  config.LoaderDataURL,
				".woff2": config.LoaderDataURL,
				".xml":   config.LoaderDataURL,
			},
		},
	})
}

// Percent-encoded data URLs should switch over to base64
// data URLs if it would result in a smaller size
func TestLoaderDataURLBase64VsPercentEncoding(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import a from './shouldUsePercent_1.txt'
				import b from './shouldUsePercent_2.txt'
				import c from './shouldUseBase64_1.txt'
				import d from './shouldUseBase64_2.txt'
				console.log(
					a,
					b,
					c,
					d,
				)
			`,
			"/shouldUsePercent_1.txt": "\n\n\n",
			"/shouldUsePercent_2.txt": "\n\n\n\n",
			"/shouldUseBase64_1.txt":  "\n\n\n\n\n",
			"/shouldUseBase64_2.txt":  "\n\n\n\n\n\n",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".txt": config.LoaderDataURL,
			},
		},
	})
}

func TestLoaderDataURLBase64InvalidUTF8(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import a from './binary.txt'
				console.log(a)
			`,
			"/binary.txt": "\xFF",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".txt": config.LoaderDataURL,
			},
		},
	})
}

func TestLoaderDataURLEscapePercents(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import a from './percents.txt'
				console.log(a)
			`,
			"/percents.txt": `
%, %3, %33, %333
%, %e, %ee, %eee
%, %E, %EE, %EEE
`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".txt": config.LoaderDataURL,
			},
		},
	})
}

func TestLoaderCopyWithBundleFromJS(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import x from "../assets/some.file"
				console.log(x)
			`,
			"/Users/user/project/assets/some.file": `stuff`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputBase: "/Users/user/project",
			AbsOutputDir:  "/out",
			ExtensionToLoader: map[string]config.Loader{
				".js":   config.LoaderJS,
				".file": config.LoaderCopy,
			},
		},
	})
}

func TestLoaderCopyWithBundleFromCSS(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.css": `
				body {
					background: url(../assets/some.file);
				}
			`,
			"/Users/user/project/assets/some.file": `stuff`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputBase: "/Users/user/project",
			AbsOutputDir:  "/out",
			ExtensionToLoader: map[string]config.Loader{
				".css":  config.LoaderCSS,
				".file": config.LoaderCopy,
			},
		},
	})
}

func TestLoaderCopyWithBundleEntryPoint(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import x from "../assets/some.file"
				console.log(x)
			`,
			"/Users/user/project/src/entry.css": `
				body {
					background: url(../assets/some.file);
				}
			`,
			"/Users/user/project/assets/some.file": `stuff`,
		},
		entryPaths: []string{
			"/Users/user/project/src/entry.js",
			"/Users/user/project/src/entry.css",
			"/Users/user/project/assets/some.file",
		},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputBase: "/Users/user/project",
			AbsOutputDir:  "/out",
			ExtensionToLoader: map[string]config.Loader{
				".js":   config.LoaderJS,
				".css":  config.LoaderCSS,
				".file": config.LoaderCopy,
			},
		},
	})
}

func TestLoaderCopyWithTransform(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js":     `console.log('entry')`,
			"/Users/user/project/assets/some.file": `stuff`,
		},
		entryPaths: []string{
			"/Users/user/project/src/entry.js",
			"/Users/user/project/assets/some.file",
		},
		options: config.Options{
			Mode:          config.ModePassThrough,
			AbsOutputBase: "/Users/user/project",
			AbsOutputDir:  "/out",
			ExtensionToLoader: map[string]config.Loader{
				".js":   config.LoaderJS,
				".file": config.LoaderCopy,
			},
		},
	})
}

func TestLoaderCopyWithFormat(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js":     `console.log('entry')`,
			"/Users/user/project/assets/some.file": `stuff`,
		},
		entryPaths: []string{
			"/Users/user/project/src/entry.js",
			"/Users/user/project/assets/some.file",
		},
		options: config.Options{
			Mode:          config.ModeConvertFormat,
			OutputFormat:  config.FormatIIFE,
			AbsOutputBase: "/Users/user/project",
			AbsOutputDir:  "/out",
			ExtensionToLoader: map[string]config.Loader{
				".js":   config.LoaderJS,
				".file": config.LoaderCopy,
			},
		},
	})
}

func TestJSXAutomaticNoNameCollision(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.jsx": `
				import { Link } from "@remix-run/react"
				const x = <Link {...y} key={z} />
			`,
		},
		entryPaths: []string{"/entry.jsx"},
		options: config.Options{
			Mode:          config.ModeConvertFormat,
			OutputFormat:  config.FormatCommonJS,
			AbsOutputFile: "/out.js",
			JSX: config.JSXOptions{
				AutomaticRuntime: true,
			},
		},
	})
}

func TestAssertTypeJSONWrongLoader(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import foo from './foo.json' assert { type: 'json' }
			`,
			"/foo.json": `{}`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode: config.ModeBundle,
			ExtensionToLoader: map[string]config.Loader{
				".js":   config.LoaderJS,
				".json": config.LoaderJS,
			},
		},
		expectedScanLog: `entry.js: ERROR: The file "foo.json" was loaded with the "js" loader
entry.js: NOTE: This import assertion requires the loader to be "json" instead:
NOTE: You need to either reconfigure esbuild to ensure that the loader for this file is "json" or you need to remove this import assertion.
`,
	})
}

func TestEmptyLoaderJS(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import './a.empty'
				import * as ns from './b.empty'
				import def from './c.empty'
				import { named } from './d.empty'
				console.log(ns, def, named)
			`,
			"/a.empty": `throw 'FAIL'`,
			"/b.empty": `throw 'FAIL'`,
			"/c.empty": `throw 'FAIL'`,
			"/d.empty": `throw 'FAIL'`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			SourceMap:     config.SourceMapExternalWithoutComment,
			NeedsMetafile: true,
			ExtensionToLoader: map[string]config.Loader{
				".js":    config.LoaderJS,
				".empty": config.LoaderEmpty,
			},
		},
		expectedCompileLog: `entry.js: WARNING: Import "named" will always be undefined because the file "d.empty" has no exports
`,
	})
}

func TestEmptyLoaderCSS(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
				@import 'a.empty';
				a { background: url(b.empty) }
			`,
			"/a.empty": `body { color: fail }`,
			"/b.empty": `fail`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode:          config.ModeBundle,
			SourceMap:     config.SourceMapExternalWithoutComment,
			NeedsMetafile: true,
			ExtensionToLoader: map[string]config.Loader{
				".css":   config.LoaderCSS,
				".empty": config.LoaderEmpty,
			},
		},
	})
}

func TestExtensionlessLoaderJS(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import './what'
			`,
			"/what": `foo()`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode: config.ModeBundle,
			ExtensionToLoader: map[string]config.Loader{
				".js": config.LoaderJS,
				"":    config.LoaderJS,
			},
		},
	})
}

func TestExtensionlessLoaderCSS(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.css": `
				@import './what';
			`,
			"/what": `.foo { color: red }`,
		},
		entryPaths: []string{"/entry.css"},
		options: config.Options{
			Mode: config.ModeBundle,
			ExtensionToLoader: map[string]config.Loader{
				".css": config.LoaderCSS,
				"":     config.LoaderCSS,
			},
		},
	})
}

// Make sure custom entry point output names are respected for the copy loader
func TestLoaderCopyEntryPointAdvanced(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/project/entry.js": `
				import xyz from './xyz.copy'
				console.log(xyz)
			`,
			"/project/TEST FAILED.copy": `some stuff`,
			"/project/xyz.copy":         `more stuff`,
		},
		entryPathsAdvanced: []bundler.EntryPoint{
			{
				InputPath:                "/project/entry.js",
				OutputPath:               "js/input/path",
				InputPathInFileNamespace: true,
			},
			{
				InputPath:                "/project/TEST FAILED.copy",
				OutputPath:               "copy/input/path",
				InputPathInFileNamespace: true,
			},
		},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExtensionToLoader: map[string]config.Loader{
				".js":   config.LoaderJS,
				".copy": config.LoaderCopy,
			},
		},
	})
}

// Make sure we don't turn "src/index.copy" into "src.copy" for files copied
// via the file loader. This is sometimes done for JS files to try to generate
// more useful names because lots of developers name their code "index.js" due
// to node's implicit "index.js" path resolution logic.
func TestLoaderCopyUseIndex(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/index.copy": `some stuff`,
		},
		entryPaths: []string{"/Users/user/project/src/index.copy"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExtensionToLoader: map[string]config.Loader{
				".copy": config.LoaderCopy,
			},
		},
	})
}

// Make sure that if "outfile" is used, a file copied with the copy loader is
// written out to that path. We don't want the file name to come from the
// original source name instead of the "outfile" name, for example.
func TestLoaderCopyExplicitOutputFile(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/project/TEST FAILED.copy": `some stuff`,
		},
		entryPaths: []string{"/project/TEST FAILED.copy"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out/this.worked",
			ExtensionToLoader: map[string]config.Loader{
				".copy": config.LoaderCopy,
			},
		},
	})
}

func TestLoaderCopyStartsWithDotAbsPath(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/project/src/.htaccess": `some stuff`,
			"/project/src/entry.js":  `some.stuff()`,
			"/project/src/.ts":       `foo as number`,
		},
		entryPaths: []string{
			"/project/src/.htaccess",
			"/project/src/entry.js",
			"/project/src/.ts",
		},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExtensionToLoader: map[string]config.Loader{
				".js":       config.LoaderJS,
				".ts":       config.LoaderTS,
				".htaccess": config.LoaderCopy,
			},
		},
	})
}

func TestLoaderCopyStartsWithDotRelPath(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/project/src/.htaccess": `some stuff`,
			"/project/src/entry.js":  `some.stuff()`,
			"/project/src/.ts":       `foo as number`,
		},
		entryPaths: []string{
			"./.htaccess",
			"./entry.js",
			"./.ts",
		},
		absWorkingDir: "/project/src",
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			ExtensionToLoader: map[string]config.Loader{
				".js":       config.LoaderJS,
				".ts":       config.LoaderTS,
				".htaccess": config.LoaderCopy,
			},
		},
	})
}

func TestLoaderCopyWithInjectedFileNoBundle(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/entry.ts":  `console.log('in entry.ts')`,
			"/src/inject.js": `console.log('in inject.js')`,
		},
		entryPaths: []string{"/src/entry.ts"},
		options: config.Options{
			AbsOutputDir: "/out",
			InjectPaths:  []string{"/src/inject.js"},
			ExtensionToLoader: map[string]config.Loader{
				".ts": config.LoaderTS,
				".js": config.LoaderCopy,
			},
		},
		expectedScanLog: `ERROR: Cannot inject "src/inject.js" with the "copy" loader without bundling enabled
`,
	})
}

func TestLoaderCopyWithInjectedFileBundle(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/src/entry.ts":  `console.log('in entry.ts')`,
			"/src/inject.js": `console.log('in inject.js')`,
		},
		entryPaths: []string{"/src/entry.ts"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			InjectPaths:  []string{"/src/inject.js"},
			ExtensionToLoader: map[string]config.Loader{
				".ts": config.LoaderTS,
				".js": config.LoaderCopy,
			},
		},
	})
}

func TestLoaderBundleWithImportAttributes(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import x from "./import.js"
				import y from "./import.js" with { type: 'json' }
				console.log(x === y)
			`,
			"/import.js": `{}`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `entry.js: ERROR: Bundling with import attributes is not currently supported
`,
	})
}
