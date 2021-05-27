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
				import absoluteIn from './absolute-in'
				import absoluteInStar from './absolute-in-star'
				import absoluteOut from './absolute-out'
				import absoluteOutStar from './absolute-out-star'
				export default {
					test0,
					test1,
					test2,
					test3,
					test4,
					test5,
					absoluteIn,
					absoluteInStar,
					absoluteOut,
					absoluteOutStar,
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
							"/virtual-in/test": ["./actual/test"],
							"/virtual-in-star/*": ["./actual/*"],
							"/virtual-out/test": ["/Users/user/project/baseurl_dot/actual/test"],
							"/virtual-out-star/*": ["/Users/user/project/baseurl_dot/actual/*"],
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
			"/Users/user/project/baseurl_dot/absolute-in.ts": `
				export {default} from '/virtual-in/test'
			`,
			"/Users/user/project/baseurl_dot/absolute-in-star.ts": `
				export {default} from '/virtual-in-star/test'
			`,
			"/Users/user/project/baseurl_dot/absolute-out.ts": `
				export {default} from '/virtual-out/test'
			`,
			"/Users/user/project/baseurl_dot/absolute-out-star.ts": `
				export {default} from '/virtual-out-star/test'
			`,
			"/Users/user/project/baseurl_dot/actual/test.ts": `
				export default 'absolute-success'
			`,

			// Tests with "baseUrl": "nested"
			"/Users/user/project/baseurl_nested/index.ts": `
				import test0 from 'test0'
				import test1 from 'test1/foo'
				import test2 from 'test2/foo'
				import test3 from 'test3/foo'
				import test4 from 'test4/foo'
				import test5 from 'test5/foo'
				import absoluteIn from './absolute-in'
				import absoluteInStar from './absolute-in-star'
				import absoluteOut from './absolute-out'
				import absoluteOutStar from './absolute-out-star'
				export default {
					test0,
					test1,
					test2,
					test3,
					test4,
					test5,
					absoluteIn,
					absoluteInStar,
					absoluteOut,
					absoluteOutStar,
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
							"/virtual-in/test": ["./actual/test"],
							"/virtual-in-star/*": ["./actual/*"],
							"/virtual-out/test": ["/Users/user/project/baseurl_nested/nested/actual/test"],
							"/virtual-out-star/*": ["/Users/user/project/baseurl_nested/nested/actual/*"],
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
			"/Users/user/project/baseurl_nested/absolute-in.ts": `
				export {default} from '/virtual-in/test'
			`,
			"/Users/user/project/baseurl_nested/absolute-in-star.ts": `
				export {default} from '/virtual-in/test'
			`,
			"/Users/user/project/baseurl_nested/absolute-out.ts": `
				export {default} from '/virtual-out/test'
			`,
			"/Users/user/project/baseurl_nested/absolute-out-star.ts": `
				export {default} from '/virtual-out-star/test'
			`,
			"/Users/user/project/baseurl_nested/nested/actual/test.ts": `
				export default 'absolute-success'
			`,
		},
		entryPaths: []string{"/Users/user/project/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestTsConfigPathsNoBaseURL(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/entry.ts": `
				import simple from './simple'
				import extended from './extended'
				console.log(simple, extended)
			`,

			// Tests with "baseUrl": "."
			"/Users/user/project/simple/index.ts": `
				import test0 from 'test0'
				import test1 from 'test1/foo'
				import test2 from 'test2/foo'
				import test3 from 'test3/foo'
				import test4 from 'test4/foo'
				import test5 from 'test5/foo'
				import absolute from './absolute'
				export default {
					test0,
					test1,
					test2,
					test3,
					test4,
					test5,
					absolute,
				}
			`,
			"/Users/user/project/simple/tsconfig.json": `
				{
					"compilerOptions": {
						"paths": {
							"test0": ["./test0-success.ts"],
							"test1/*": ["./test1-success.ts"],
							"test2/*": ["./test2-success/*"],
							"t*t3/foo": ["./test3-succ*s.ts"],
							"test4/*": ["./test4-first/*", "./test4-second/*"],
							"test5/*": ["./test5-first/*", "./test5-second/*"],
							"/virtual/*": ["./actual/*"],
						}
					}
				}
			`,
			"/Users/user/project/simple/test0-success.ts": `
				export default 'test0-success'
			`,
			"/Users/user/project/simple/test1-success.ts": `
				export default 'test1-success'
			`,
			"/Users/user/project/simple/test2-success/foo.ts": `
				export default 'test2-success'
			`,
			"/Users/user/project/simple/test3-success.ts": `
				export default 'test3-success'
			`,
			"/Users/user/project/simple/test4-first/foo.ts": `
				export default 'test4-success'
			`,
			"/Users/user/project/simple/test5-second/foo.ts": `
				export default 'test5-success'
			`,
			"/Users/user/project/simple/absolute.ts": `
				export {default} from '/virtual/test'
			`,
			"/Users/user/project/simple/actual/test.ts": `
				export default 'absolute-success'
			`,

			// Tests with "baseUrl": "nested"
			"/Users/user/project/extended/index.ts": `
				import test0 from 'test0'
				import test1 from 'test1/foo'
				import test2 from 'test2/foo'
				import test3 from 'test3/foo'
				import test4 from 'test4/foo'
				import test5 from 'test5/foo'
				import absolute from './absolute'
				export default {
					test0,
					test1,
					test2,
					test3,
					test4,
					test5,
					absolute,
				}
			`,
			"/Users/user/project/extended/tsconfig.json": `
				{
					"extends": "./nested/tsconfig.json"
				}
			`,
			"/Users/user/project/extended/nested/tsconfig.json": `
				{
					"compilerOptions": {
						"paths": {
							"test0": ["./test0-success.ts"],
							"test1/*": ["./test1-success.ts"],
							"test2/*": ["./test2-success/*"],
							"t*t3/foo": ["./test3-succ*s.ts"],
							"test4/*": ["./test4-first/*", "./test4-second/*"],
							"test5/*": ["./test5-first/*", "./test5-second/*"],
							"/virtual/*": ["./actual/*"],
						}
					}
				}
			`,
			"/Users/user/project/extended/nested/test0-success.ts": `
				export default 'test0-success'
			`,
			"/Users/user/project/extended/nested/test1-success.ts": `
				export default 'test1-success'
			`,
			"/Users/user/project/extended/nested/test2-success/foo.ts": `
				export default 'test2-success'
			`,
			"/Users/user/project/extended/nested/test3-success.ts": `
				export default 'test3-success'
			`,
			"/Users/user/project/extended/nested/test4-first/foo.ts": `
				export default 'test4-success'
			`,
			"/Users/user/project/extended/nested/test5-second/foo.ts": `
				export default 'test5-success'
			`,
			"/Users/user/project/extended/absolute.ts": `
				export {default} from '/virtual/test'
			`,
			"/Users/user/project/extended/nested/actual/test.ts": `
				export default 'absolute-success'
			`,
		},
		entryPaths: []string{"/Users/user/project/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestTsConfigBadPathsNoBaseURL(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/entry.ts": `
				import "should-not-be-imported"
			`,
			"/Users/user/project/should-not-be-imported.ts": `
			`,
			"/Users/user/project/tsconfig.json": `
				{
					"compilerOptions": {
						"paths": {
							"test": [
								".",
								"..",
								"./good",
								".\\good",
								"../good",
								"..\\good",
								"/good",
								"\\good",
								"c:/good",
								"c:\\good",
								"C:/good",
								"C:\\good",

								"bad",
								"@bad/core",
								".*/bad",
								"..*/bad",
								"c*:\\bad",
								"c:*\\bad",
								"http://bad"
							]
						}
					}
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/entry.ts: error: Could not resolve "should-not-be-imported" ` +
			`(use "./should-not-be-imported" to reference the file "Users/user/project/should-not-be-imported.ts")
Users/user/project/tsconfig.json: warning: Non-relative path "bad" is not allowed when "baseUrl" is not set (did you forget a leading "./"?)
Users/user/project/tsconfig.json: warning: Non-relative path "@bad/core" is not allowed when "baseUrl" is not set (did you forget a leading "./"?)
Users/user/project/tsconfig.json: warning: Non-relative path ".*/bad" is not allowed when "baseUrl" is not set (did you forget a leading "./"?)
Users/user/project/tsconfig.json: warning: Non-relative path "..*/bad" is not allowed when "baseUrl" is not set (did you forget a leading "./"?)
Users/user/project/tsconfig.json: warning: Non-relative path "c*:\\bad" is not allowed when "baseUrl" is not set (did you forget a leading "./"?)
Users/user/project/tsconfig.json: warning: Non-relative path "c:*\\bad" is not allowed when "baseUrl" is not set (did you forget a leading "./"?)
Users/user/project/tsconfig.json: warning: Non-relative path "http://bad" is not allowed when "baseUrl" is not set (did you forget a leading "./"?)
`,
	})
}

// https://github.com/evanw/esbuild/issues/913
func TestTsConfigPathsOverriddenBaseURL(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				import test from '#/test'
				console.log(test)
			`,
			"/Users/user/project/src/test.ts": `
				export default 123
			`,
			"/Users/user/project/tsconfig.json": `
				{
					"extends": "./tsconfig.paths.json",
					"compilerOptions": {
						"baseUrl": "./src"
					}
				}
			`,
			"/Users/user/project/tsconfig.paths.json": `
				{
					"compilerOptions": {
						"paths": {
							"#/*": ["./*"]
						}
					}
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestTsConfigPathsOverriddenBaseURLDifferentDir(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				import test from '#/test'
				console.log(test)
			`,
			"/Users/user/project/src/test.ts": `
				export default 123
			`,
			"/Users/user/project/src/tsconfig.json": `
				{
					"extends": "../tsconfig.paths.json",
					"compilerOptions": {
						"baseUrl": "./"
					}
				}
			`,
			"/Users/user/project/tsconfig.paths.json": `
				{
					"compilerOptions": {
						"paths": {
							"#/*": ["./*"]
						}
					}
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestTsConfigPathsMissingBaseURL(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				import test from '#/test'
				console.log(test)
			`,
			"/Users/user/project/src/test.ts": `
				export default 123
			`,
			"/Users/user/project/src/tsconfig.json": `
				{
					"extends": "../tsconfig.paths.json",
					"compilerOptions": {
					}
				}
			`,
			"/Users/user/project/tsconfig.paths.json": `
				{
					"compilerOptions": {
						"paths": {
							"#/*": ["./*"]
						}
					}
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.ts: error: Could not resolve "#/test" (mark it as external to exclude it from the bundle)
`,
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

func TestTsconfigJsonAbsoluteBaseUrl(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/app/entry.js": `
				import fn from 'lib/util'
				console.log(fn())
			`,
			"/Users/user/project/src/tsconfig.json": `
				{
					"compilerOptions": {
						"baseUrl": "/Users/user/project/src"
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

func TestTsconfigJsonExtendsAbsolute(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/entry.jsx": `
				console.log(<div/>, <></>)
			`,
			"/Users/user/project/tsconfig.json": `
				{
					"extends": "/Users/user/project/base.json",
					"compilerOptions": {
						"jsxFragmentFactory": "derivedFragment"
					}
				}
			`,
			"/Users/user/project/base.json": `
				{
					"compilerOptions": {
						"jsxFactory": "baseFactory",
						"jsxFragmentFactory": "baseFragment"
					}
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/entry.jsx"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTsconfigJsonExtendsThreeLevels(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.jsx": `
				import "test/import.js"
				console.log(<div/>, <></>)
			`,
			"/Users/user/project/src/tsconfig.json": `
				{
					"extends": "./path1/base",
					"compilerOptions": {
						"jsxFragmentFactory": "derivedFragment"
					}
				}
			`,
			"/Users/user/project/src/path1/base.json": `
				{
					"extends": "../path2/base2"
				}
			`,
			"/Users/user/project/src/path2/base2.json": `
				{
					"compilerOptions": {
						"baseUrl": ".",
						"paths": {
							"test/*": ["./works/*"]
						},
						"jsxFactory": "baseFactory",
						"jsxFragmentFactory": "baseFragment"
					}
				}
			`,
			"/Users/user/project/src/path2/works/import.js": `
				console.log('works')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.jsx"},
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
		expectedScanLog: `base.json: warning: Base config file "./tsconfig" forms cycle
`,
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
		expectedScanLog: `error: Cannot find tsconfig file "this/file/doesn't/exist/tsconfig.json"
`,
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
		expectedScanLog: `Users/user/project/src/foo/tsconfig.json: warning: Cannot find base config file "extends for foo"
`,
	})
}

func TestTsconfigRemoveUnusedImports(t *testing.T) {
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

func TestTsconfigPreserveUnusedImports(t *testing.T) {
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

// This must preserve the import clause even though all imports are not used as
// values. THIS BEHAVIOR IS A DEVIATION FROM THE TYPESCRIPT COMPILER! It exists
// to support the use case of compiling partial modules for compile-to-JavaScript
// languages such as Svelte.
func TestTsconfigPreserveUnusedImportClause(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				import {x, y} from "./foo"
				import z from "./foo"
				import * as ns from "./foo"
				console.log(1 as x, 2 as z, 3 as ns.y)
			`,
			"/Users/user/project/src/tsconfig.json": `{
				"compilerOptions": {
					"importsNotUsedAsValues": "preserve"
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:          config.ModeConvertFormat,
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/Users/user/project/out.js",
			ExternalModules: config.ExternalModules{
				AbsPaths: map[string]bool{
					"/Users/user/project/src/foo": true,
				},
			},
		},
	})
}

func TestTsconfigTarget(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				import "./es2018"
				import "./es2019"
				import "./es2020"
				import "./es4"
			`,
			"/Users/user/project/src/es2018/index.ts": `
				let x = { ...y }   // es2018 syntax
				try { y } catch {} // es2019 syntax
				x?.y()             // es2020 syntax
			`,
			"/Users/user/project/src/es2019/index.ts": `
				let x = { ...y }   // es2018 syntax
				try { y } catch {} // es2019 syntax
				x?.y()             // es2020 syntax
			`,
			"/Users/user/project/src/es2020/index.ts": `
				let x = { ...y }   // es2018 syntax
				try { y } catch {} // es2019 syntax
				x?.y()             // es2020 syntax
			`,
			"/Users/user/project/src/es4/index.ts": `
			`,
			"/Users/user/project/src/es2018/tsconfig.json": `{
				"compilerOptions": {
					"target": "ES2018"
				}
			}`,
			"/Users/user/project/src/es2019/tsconfig.json": `{
				"compilerOptions": {
					"target": "es2019"
				}
			}`,
			"/Users/user/project/src/es2020/tsconfig.json": `{
				"compilerOptions": {
					"target": "ESNext"
				}
			}`,
			"/Users/user/project/src/es4/tsconfig.json": `{
				"compilerOptions": {
					"target": "ES4"
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/es4/tsconfig.json: warning: Unrecognized target environment "ES4"
`,
	})
}

func TestTsconfigTargetError(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				x = 123n
			`,
			"/Users/user/project/src/tsconfig.json": `{
				"compilerOptions": {
					"target": "ES2019"
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:              config.ModeBundle,
			AbsOutputFile:     "/Users/user/project/out.js",
			OriginalTargetEnv: "\"esnext\"", // This should not be reported as the cause of the error
		},
		expectedScanLog: `Users/user/project/src/entry.ts: error: Big integer literals are not available in the configured target environment ("ES2019")
Users/user/project/src/tsconfig.json: note: The target environment was set to "ES2019" here
`,
	})
}
