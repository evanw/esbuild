package bundler

import (
	"testing"

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
		expectedCompileLog: `entry.js: error: No matching export for import "missing"
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

func TestLoaderJSONNoBundleUMD(t *testing.T) {
	loader_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/test.json": `{"test": 123, "invalid-identifier": true}`,
		},
		entryPaths: []string{"/test.json"},
		options: config.Options{
			Mode:          config.ModeConvertFormat,
			OutputFormat:  config.FormatUMD,
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
