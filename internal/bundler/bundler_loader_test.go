package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

func TestLoaderFile(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test3.svg'))
			`,

			// "/test3.svg" generates the file name "test3.0sKdZN/F.svg" if the
			// standard base64 encoding is used instead of the URL base64 encoding
			"/test3.svg": "<svg></svg>",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:   true,
			AbsOutputDir: "/out/",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".svg": config.LoaderFile,
			},
		},
		expected: map[string]string{
			"/out/test3.0sKdZN_F.svg": "<svg></svg>",
			"/out/entry.js": `// /test3.svg
var require_test3 = __commonJS((exports, module) => {
  module.exports = "test3.0sKdZN_F.svg";
});

// /entry.js
console.log(require_test3());
`,
		},
	})
}

func TestLoaderFileMultipleNoCollision(t *testing.T) {
	expectBundled(t, bundled{
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
			IsBundling:    true,
			AbsOutputFile: "/dist/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".txt": config.LoaderFile,
			},
		},
		expected: map[string]string{
			"/dist/test.d-VvEp_S.txt": "test",
			"/dist/test.pL3kpHJC.txt": "test",
			"/dist/out.js": `// /a/test.txt
var require_test = __commonJS((exports, module) => {
  module.exports = "test.d-VvEp_S.txt";
});

// /b/test.txt
var require_test2 = __commonJS((exports, module) => {
  module.exports = "test.pL3kpHJC.txt";
});

// /entry.js
console.log(require_test(), require_test2());
`,
		},
	})
}

func TestJSXSyntaxInJSWithJSXLoader(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(<div/>)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js": config.LoaderJSX,
			},
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
console.log(/* @__PURE__ */ React.createElement("div", null));
`,
		},
	})
}

func TestRequireCustomExtensionString(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.custom'))
			`,
			"/test.custom": `#include <stdio.h>`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":     config.LoaderJS,
				".custom": config.LoaderText,
			},
		},
		expected: map[string]string{
			"/out.js": `// /test.custom
var require_test = __commonJS((exports, module) => {
  module.exports = "#include <stdio.h>";
});

// /entry.js
console.log(require_test());
`,
		},
	})
}

func TestRequireCustomExtensionBase64(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.custom'))
			`,
			"/test.custom": "a\x00b\x80c\xFFd",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":     config.LoaderJS,
				".custom": config.LoaderBase64,
			},
		},
		expected: map[string]string{
			"/out.js": `// /test.custom
var require_test = __commonJS((exports, module) => {
  module.exports = "YQBigGP/ZA==";
});

// /entry.js
console.log(require_test());
`,
		},
	})
}

func TestRequireCustomExtensionDataURL(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.custom'))
			`,
			"/test.custom": "a\x00b\x80c\xFFd",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":     config.LoaderJS,
				".custom": config.LoaderDataURL,
			},
		},
		expected: map[string]string{
			"/out.js": `// /test.custom
var require_test = __commonJS((exports, module) => {
  module.exports = "data:application/octet-stream;base64,YQBigGP/ZA==";
});

// /entry.js
console.log(require_test());
`,
		},
	})
}

func TestRequireCustomExtensionPreferLongest(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.txt'), require('./test.base64.txt'))
			`,
			"/test.txt":        `test.txt`,
			"/test.base64.txt": `test.base64.txt`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":         config.LoaderJS,
				".txt":        config.LoaderText,
				".base64.txt": config.LoaderBase64,
			},
		},
		expected: map[string]string{
			"/out.js": `// /test.txt
var require_test = __commonJS((exports, module) => {
  module.exports = "test.txt";
});

// /test.base64.txt
var require_test_base64 = __commonJS((exports, module) => {
  module.exports = "dGVzdC5iYXNlNjQudHh0";
});

// /entry.js
console.log(require_test(), require_test_base64());
`,
		},
	})
}

func TestAutoDetectMimeTypeFromExtension(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.svg'))
			`,
			"/test.svg": "a\x00b\x80c\xFFd",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".svg": config.LoaderDataURL,
			},
		},
		expected: map[string]string{
			"/out.js": `// /test.svg
var require_test = __commonJS((exports, module) => {
  module.exports = "data:image/svg+xml;base64,YQBigGP/ZA==";
});

// /entry.js
console.log(require_test());
`,
		},
	})
}

func TestLoaderJSONCommonJSAndES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const x_json = require('./x.json')
				import y_json from './y.json'
				console.log(x_json, y_json)
			`,
			"/x.json": "{\"x\": true}",
			"/y.json": "{\"y\": true}",
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /x.json
var require_x = __commonJS((exports, module) => {
  module.exports = {x: true};
});

// /y.json
const y_default = {y: true};

// /entry.js
const x_json = require_x();
console.log(x_json, y_default);
`,
		},
	})
}

func TestLoaderTextCommonJSAndES6(t *testing.T) {
	expectBundled(t, bundled{
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
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /x.txt
var require_x = __commonJS((exports, module) => {
  module.exports = "x";
});

// /y.txt
const y_default = "y";

// /entry.js
const x_txt = require_x();
console.log(x_txt, y_default);
`,
		},
	})
}

func TestLoaderBase64CommonJSAndES6(t *testing.T) {
	expectBundled(t, bundled{
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
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".b64": config.LoaderBase64,
			},
		},
		expected: map[string]string{
			"/out.js": `// /x.b64
var require_x = __commonJS((exports, module) => {
  module.exports = "eA==";
});

// /y.b64
const y_default = "eQ==";

// /entry.js
const x_b64 = require_x();
console.log(x_b64, y_default);
`,
		},
	})
}

func TestLoaderDataURLCommonJSAndES6(t *testing.T) {
	expectBundled(t, bundled{
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
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".txt": config.LoaderDataURL,
			},
		},
		expected: map[string]string{
			"/out.js": `// /x.txt
var require_x = __commonJS((exports, module) => {
  module.exports = "data:text/plain; charset=utf-8;base64,eA==";
});

// /y.txt
const y_default = "data:text/plain; charset=utf-8;base64,eQ==";

// /entry.js
const x_url = require_x();
console.log(x_url, y_default);
`,
		},
	})
}

func TestLoaderFileCommonJSAndES6(t *testing.T) {
	expectBundled(t, bundled{
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
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]config.Loader{
				".js":  config.LoaderJS,
				".txt": config.LoaderFile,
			},
		},
		expected: map[string]string{
			"/x.ZzUefEyG.txt": `x`,
			"/y.KRCjcBKx.txt": `y`,
			"/out.js": `// /x.txt
var require_x = __commonJS((exports, module) => {
  module.exports = "x.ZzUefEyG.txt";
});

// /y.txt
const y_default = "y.KRCjcBKx.txt";

// /entry.js
const x_url = require_x();
console.log(x_url, y_default);
`,
		},
	})
}
