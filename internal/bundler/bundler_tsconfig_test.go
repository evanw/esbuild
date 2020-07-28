package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

func TestTsConfigPaths(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/entry.ts": `
				import baseurl_dot from './baseurl_dot'
				import baseurl_nested from './baseurl_nested'
				console.log(baseurl_dot, baseurl_nested)
			`,

			// Tests with "baseUrl": "."
			"/Users/user/project/baseurl_dot/index.ts": `
				import test0 from 'test0'
				import test1 from 'test1/foo'
				import test2 from 'test2/foo'
				import test3 from 'test3/foo'
				import test4 from 'test4/foo'
				import test5 from 'test5/foo'
				export default {
					test0,
					test1,
					test2,
					test3,
					test4,
					test5,
				}
			`,
			"/Users/user/project/baseurl_dot/tsconfig.json": `
				{
					"compilerOptions": {
						"baseUrl": ".",
						"paths": {
							"test0": ["./test0-success.ts"],
							"test1/*": ["./test1-success.ts"],
							"test2/*": ["./test2-success/*"],
							"t*t3/foo": ["./test3-succ*s.ts"],
							"test4/*": ["./test4-first/*", "./test4-second/*"],
							"test5/*": ["./test5-first/*", "./test5-second/*"],
						}
					}
				}
			`,
			"/Users/user/project/baseurl_dot/test0-success.ts": `
				export default 'test0-success'
			`,
			"/Users/user/project/baseurl_dot/test1-success.ts": `
				export default 'test1-success'
			`,
			"/Users/user/project/baseurl_dot/test2-success/foo.ts": `
				export default 'test2-success'
			`,
			"/Users/user/project/baseurl_dot/test3-success.ts": `
				export default 'test3-success'
			`,
			"/Users/user/project/baseurl_dot/test4-first/foo.ts": `
				export default 'test4-success'
			`,
			"/Users/user/project/baseurl_dot/test5-second/foo.ts": `
				export default 'test5-success'
			`,

			// Tests with "baseUrl": "nested"
			"/Users/user/project/baseurl_nested/index.ts": `
				import test0 from 'test0'
				import test1 from 'test1/foo'
				import test2 from 'test2/foo'
				import test3 from 'test3/foo'
				import test4 from 'test4/foo'
				import test5 from 'test5/foo'
				export default {
					test0,
					test1,
					test2,
					test3,
					test4,
					test5,
				}
			`,
			"/Users/user/project/baseurl_nested/tsconfig.json": `
				{
					"compilerOptions": {
						"baseUrl": "nested",
						"paths": {
							"test0": ["./test0-success.ts"],
							"test1/*": ["./test1-success.ts"],
							"test2/*": ["./test2-success/*"],
							"t*t3/foo": ["./test3-succ*s.ts"],
							"test4/*": ["./test4-first/*", "./test4-second/*"],
							"test5/*": ["./test5-first/*", "./test5-second/*"],
						}
					}
				}
			`,
			"/Users/user/project/baseurl_nested/nested/test0-success.ts": `
				export default 'test0-success'
			`,
			"/Users/user/project/baseurl_nested/nested/test1-success.ts": `
				export default 'test1-success'
			`,
			"/Users/user/project/baseurl_nested/nested/test2-success/foo.ts": `
				export default 'test2-success'
			`,
			"/Users/user/project/baseurl_nested/nested/test3-success.ts": `
				export default 'test3-success'
			`,
			"/Users/user/project/baseurl_nested/nested/test4-first/foo.ts": `
				export default 'test4-success'
			`,
			"/Users/user/project/baseurl_nested/nested/test5-second/foo.ts": `
				export default 'test5-success'
			`,
		},
		entryPaths: []string{"/Users/user/project/entry.ts"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/baseurl_dot/test0-success.ts
var test0_success_default = "test0-success";

// /Users/user/project/baseurl_dot/test1-success.ts
var test1_success_default = "test1-success";

// /Users/user/project/baseurl_dot/test2-success/foo.ts
var foo_default = "test2-success";

// /Users/user/project/baseurl_dot/test3-success.ts
var test3_success_default = "test3-success";

// /Users/user/project/baseurl_dot/test4-first/foo.ts
var foo_default2 = "test4-success";

// /Users/user/project/baseurl_dot/test5-second/foo.ts
var foo_default3 = "test5-success";

// /Users/user/project/baseurl_dot/index.ts
var baseurl_dot_default = {
  test0: test0_success_default,
  test1: test1_success_default,
  test2: foo_default,
  test3: test3_success_default,
  test4: foo_default2,
  test5: foo_default3
};

// /Users/user/project/baseurl_nested/nested/test0-success.ts
var test0_success_default2 = "test0-success";

// /Users/user/project/baseurl_nested/nested/test1-success.ts
var test1_success_default2 = "test1-success";

// /Users/user/project/baseurl_nested/nested/test2-success/foo.ts
var foo_default4 = "test2-success";

// /Users/user/project/baseurl_nested/nested/test3-success.ts
var test3_success_default2 = "test3-success";

// /Users/user/project/baseurl_nested/nested/test4-first/foo.ts
var foo_default5 = "test4-success";

// /Users/user/project/baseurl_nested/nested/test5-second/foo.ts
var foo_default6 = "test5-success";

// /Users/user/project/baseurl_nested/index.ts
var baseurl_nested_default = {
  test0: test0_success_default2,
  test1: test1_success_default2,
  test2: foo_default4,
  test3: test3_success_default2,
  test4: foo_default5,
  test5: foo_default6
};

// /Users/user/project/entry.ts
console.log(baseurl_dot_default, baseurl_nested_default);
`,
		},
	})
}

func TestTsConfigJSX(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/entry.tsx": `
				console.log(<><div/><div/></>)
			`,
			"/Users/user/project/tsconfig.json": `
				{
					"compilerOptions": {
						"jsxFactory": "R.c",
						"jsxFragmentFactory": "R.F"
					}
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/entry.tsx"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/entry.tsx
console.log(/* @__PURE__ */ R.c(R.F, null, /* @__PURE__ */ R.c("div", null), /* @__PURE__ */ R.c("div", null)));
`,
		},
	})
}

func TestTsConfigNestedJSX(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/entry.ts": `
				import factory from './factory'
				import fragment from './fragment'
				import both from './both'
				console.log(factory, fragment, both)
			`,
			"/Users/user/project/factory/index.tsx": `
				export default <><div/><div/></>
			`,
			"/Users/user/project/factory/tsconfig.json": `
				{
					"compilerOptions": {
						"jsxFactory": "h"
					}
				}
			`,
			"/Users/user/project/fragment/index.tsx": `
				export default <><div/><div/></>
			`,
			"/Users/user/project/fragment/tsconfig.json": `
				{
					"compilerOptions": {
						"jsxFragmentFactory": "a.b"
					}
				}
			`,
			"/Users/user/project/both/index.tsx": `
				export default <><div/><div/></>
			`,
			"/Users/user/project/both/tsconfig.json": `
				{
					"compilerOptions": {
						"jsxFactory": "R.c",
						"jsxFragmentFactory": "R.F"
					}
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/entry.ts"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/factory/index.tsx
var factory_default = /* @__PURE__ */ h(React.Fragment, null, /* @__PURE__ */ h("div", null), /* @__PURE__ */ h("div", null));

// /Users/user/project/fragment/index.tsx
var fragment_default = /* @__PURE__ */ React.createElement(a.b, null, /* @__PURE__ */ React.createElement("div", null), /* @__PURE__ */ React.createElement("div", null));

// /Users/user/project/both/index.tsx
var both_default = /* @__PURE__ */ R.c(R.F, null, /* @__PURE__ */ R.c("div", null), /* @__PURE__ */ R.c("div", null));

// /Users/user/project/entry.ts
console.log(factory_default, fragment_default, both_default);
`,
		},
	})
}

func TestTsconfigJsonBaseUrl(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/app/entry.js": `
				import fn from 'lib/util'
				console.log(fn())
			`,
			"/Users/user/project/src/tsconfig.json": `
				{
					"compilerOptions": {
						"baseUrl": "."
					}
				}
			`,
			"/Users/user/project/src/lib/util.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/app/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/src/lib/util.js
var require_util = __commonJS((exports, module) => {
  module.exports = function() {
    return 123;
  };
});

// /Users/user/project/src/app/entry.js
const util = __toModule(require_util());
console.log(util.default());
`,
		},
	})
}

func TestJsconfigJsonBaseUrl(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/app/entry.js": `
				import fn from 'lib/util'
				console.log(fn())
			`,
			"/Users/user/project/src/jsconfig.json": `
				{
					"compilerOptions": {
						"baseUrl": "."
					}
				}
			`,
			"/Users/user/project/src/lib/util.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/app/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/src/lib/util.js
var require_util = __commonJS((exports, module) => {
  module.exports = function() {
    return 123;
  };
});

// /Users/user/project/src/app/entry.js
const util = __toModule(require_util());
console.log(util.default());
`,
		},
	})
}

func TestTsconfigJsonCommentAllowed(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/app/entry.js": `
				import fn from 'lib/util'
				console.log(fn())
			`,
			"/Users/user/project/src/tsconfig.json": `
				{
					// Single-line comment
					"compilerOptions": {
						"baseUrl": "."
					}
				}
			`,
			"/Users/user/project/src/lib/util.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/app/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/src/lib/util.js
var require_util = __commonJS((exports, module) => {
  module.exports = function() {
    return 123;
  };
});

// /Users/user/project/src/app/entry.js
const util = __toModule(require_util());
console.log(util.default());
`,
		},
	})
}

func TestTsconfigJsonTrailingCommaAllowed(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/app/entry.js": `
				import fn from 'lib/util'
				console.log(fn())
			`,
			"/Users/user/project/src/tsconfig.json": `
				{
					"compilerOptions": {
						"baseUrl": ".",
					},
				}
			`,
			"/Users/user/project/src/lib/util.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/app/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/src/lib/util.js
var require_util = __commonJS((exports, module) => {
  module.exports = function() {
    return 123;
  };
});

// /Users/user/project/src/app/entry.js
const util = __toModule(require_util());
console.log(util.default());
`,
		},
	})
}

func TestTsconfigJsonExtends(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.jsx": `
				console.log(<div/>, <></>)
			`,
			"/tsconfig.json": `
				{
					"extends": "./base",
					"compilerOptions": {
						"jsxFragmentFactory": "derivedFragment"
					}
				}
			`,
			"/base.json": `
				{
					"compilerOptions": {
						"jsxFactory": "baseFactory",
						"jsxFragmentFactory": "baseFragment"
					}
				}
			`,
		},
		entryPaths: []string{"/entry.jsx"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.jsx
console.log(/* @__PURE__ */ baseFactory("div", null), /* @__PURE__ */ baseFactory(derivedFragment, null));
`,
		},
	})
}

func TestTsconfigJsonExtendsThreeLevels(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.jsx": `
				console.log(<div/>, <></>)
			`,
			"/tsconfig.json": `
				{
					"extends": "./base",
					"compilerOptions": {
						"jsxFragmentFactory": "derivedFragment"
					}
				}
			`,
			"/base.json": `
				{
					"extends": "./base2"
				}
			`,
			"/base2.json": `
				{
					"compilerOptions": {
						"jsxFactory": "baseFactory",
						"jsxFragmentFactory": "baseFragment"
					}
				}
			`,
		},
		entryPaths: []string{"/entry.jsx"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.jsx
console.log(/* @__PURE__ */ baseFactory("div", null), /* @__PURE__ */ baseFactory(derivedFragment, null));
`,
		},
	})
}

func TestTsconfigJsonExtendsLoop(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(123)
			`,
			"/tsconfig.json": `
				{
					"extends": "./base.json"
				}
			`,
			"/base.json": `
				{
					"extends": "./tsconfig"
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: "/base.json: warning: Base config file \"./tsconfig\" forms cycle\n",
		expected: map[string]string{
			"/out.js": `// /entry.js
console.log(123);
`,
		},
	})
}

func TestTsconfigJsonExtendsPackage(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/app/entry.jsx": `
				console.log(<div/>)
			`,
			"/Users/user/project/src/tsconfig.json": `
				{
					"extends": "@package/foo/tsconfig.json"
				}
			`,
			"/Users/user/project/node_modules/@package/foo/tsconfig.json": `
				{
					"compilerOptions": {
						"jsxFactory": "worked"
					}
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/app/entry.jsx"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/src/app/entry.jsx
console.log(/* @__PURE__ */ worked("div", null));
`,
		},
	})
}

func TestTsconfigJsonOverrideMissing(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/app/entry.ts": `
				import 'foo'
			`,
			"/Users/user/project/src/foo-bad.ts": `
				console.log('bad')
			`,
			"/Users/user/project/src/tsconfig.json": `
				{
					"compilerOptions": {
						"baseUrl": ".",
						"paths": {
							"foo": ["./foo-bad.ts"]
						}
					}
				}
			`,
			"/Users/user/project/other/foo-good.ts": `
				console.log('good')
			`,
			"/Users/user/project/other/config-for-ts.json": `
				{
					"compilerOptions": {
						"baseUrl": ".",
						"paths": {
							"foo": ["./foo-good.ts"]
						}
					}
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/app/entry.ts"},
		options: config.Options{
			IsBundling:       true,
			AbsOutputFile:    "/Users/user/project/out.js",
			TsConfigOverride: "/Users/user/project/other/config-for-ts.json",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/other/foo-good.ts
console.log("good");

// /Users/user/project/src/app/entry.ts
`,
		},
	})
}

func TestTsconfigJsonOverrideNodeModules(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/app/entry.ts": `
				import 'foo'
			`,
			"/Users/user/project/src/node_modules/foo/index.js": `
				console.log('default')
			`,
			"/Users/user/project/src/foo-bad.ts": `
				console.log('bad')
			`,
			"/Users/user/project/src/tsconfig.json": `
				{
					"compilerOptions": {
						"baseUrl": ".",
						"paths": {
							"foo": ["./foo-bad.ts"]
						}
					}
				}
			`,
			"/Users/user/project/other/foo-good.ts": `
				console.log('good')
			`,
			"/Users/user/project/other/config-for-ts.json": `
				{
					"compilerOptions": {
						"baseUrl": ".",
						"paths": {
							"foo": ["./foo-good.ts"]
						}
					}
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/app/entry.ts"},
		options: config.Options{
			IsBundling:       true,
			AbsOutputFile:    "/Users/user/project/out.js",
			TsConfigOverride: "/Users/user/project/other/config-for-ts.json",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/other/foo-good.ts
console.log("good");

// /Users/user/project/src/app/entry.ts
`,
		},
	})
}

func TestTsconfigJsonNodeModulesImplicitFile(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/app/entry.tsx": `
				console.log(<div/>)
			`,
			"/Users/user/project/src/tsconfig.json": `
				{
					"extends": "foo"
				}
			`,
			"/Users/user/project/src/node_modules/foo/tsconfig.json": `
				{
					"compilerOptions": {
						"jsx": "react",
						"jsxFactory": "worked"
					}
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/app/entry.tsx"},
		options: config.Options{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `// /Users/user/project/src/app/entry.tsx
console.log(/* @__PURE__ */ worked("div", null));
`,
		},
	})
}
