package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

var tsconfig_suite = suite{
	name: "tsconfig",
}

func TestTsConfigPaths(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestTsConfigJSX(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestTsConfigNestedJSX(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestTsconfigJsonBaseUrl(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestJsconfigJsonBaseUrl(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestTsconfigJsonCommentAllowed(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestTsconfigJsonTrailingCommaAllowed(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestTsconfigJsonExtends(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTsconfigJsonExtendsThreeLevels(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTsconfigJsonExtendsLoop(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: "/base.json: warning: Base config file \"./tsconfig\" forms cycle\n",
	})
}

func TestTsconfigJsonExtendsPackage(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestTsconfigJsonOverrideMissing(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
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
			Mode:             config.ModeBundle,
			AbsOutputFile:    "/Users/user/project/out.js",
			TsConfigOverride: "/Users/user/project/other/config-for-ts.json",
		},
	})
}

func TestTsconfigJsonOverrideNodeModules(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
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
			Mode:             config.ModeBundle,
			AbsOutputFile:    "/Users/user/project/out.js",
			TsConfigOverride: "/Users/user/project/other/config-for-ts.json",
		},
	})
}

func TestTsconfigJsonOverrideInvalid(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": ``,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			Mode:             config.ModeBundle,
			AbsOutputFile:    "/out.js",
			TsConfigOverride: "/this/file/doesn't/exist/tsconfig.json",
		},
		expectedScanLog: "error: Cannot find tsconfig file \"/this/file/doesn't/exist/tsconfig.json\"\n",
	})
}

func TestTsconfigJsonNodeModulesImplicitFile(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestTsconfigWarningsInsideNodeModules(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.tsx": `
				import "./foo"
				import "bar"
			`,

			"/Users/user/project/src/foo/tsconfig.json": `{ "extends": "extends for foo" }`,
			"/Users/user/project/src/foo/index.js":      ``,

			"/Users/user/project/src/node_modules/bar/tsconfig.json": `{ "extends": "extends for bar" }`,
			"/Users/user/project/src/node_modules/bar/index.js":      ``,
		},
		entryPaths: []string{"/Users/user/project/src/entry.tsx"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `/Users/user/project/src/foo/tsconfig.json: warning: Cannot find base config file "extends for foo"
`,
	})
}

func TestTsconfigRemoveTypeImports(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				import {x, y} from "./foo"
				console.log(1 as x)
			`,
			"/Users/user/project/src/tsconfig.json": `{
				"compilerOptions": {
					"importsNotUsedAsValues": "remove"
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestTsconfigPreserveTypeImports(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				import {x, y} from "./foo"
				console.log(1 as x)
			`,
			"/Users/user/project/src/tsconfig.json": `{
				"compilerOptions": {
					"importsNotUsedAsValues": "preserve"
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
			ExternalModules: config.ExternalModules{
				AbsPaths: map[string]bool{
					"/Users/user/project/src/foo": true,
				},
			},
		},
	})
}
