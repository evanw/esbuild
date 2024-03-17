package bundler_tests

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

var tsconfig_suite = suite{
	name: "tsconfig",
}

func TestTsconfigPaths(t *testing.T) {
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

func TestTsconfigPathsNoBaseURL(t *testing.T) {
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

func TestTsconfigBadPathsNoBaseURL(t *testing.T) {
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
		expectedScanLog: `Users/user/project/entry.ts: ERROR: Could not resolve "should-not-be-imported"
NOTE: Use the relative path "./should-not-be-imported" to reference the file "Users/user/project/should-not-be-imported.ts". Without the leading "./", the path "should-not-be-imported" is being interpreted as a package path instead.
Users/user/project/tsconfig.json: WARNING: Non-relative path "bad" is not allowed when "baseUrl" is not set (did you forget a leading "./"?)
Users/user/project/tsconfig.json: WARNING: Non-relative path "@bad/core" is not allowed when "baseUrl" is not set (did you forget a leading "./"?)
Users/user/project/tsconfig.json: WARNING: Non-relative path ".*/bad" is not allowed when "baseUrl" is not set (did you forget a leading "./"?)
Users/user/project/tsconfig.json: WARNING: Non-relative path "..*/bad" is not allowed when "baseUrl" is not set (did you forget a leading "./"?)
Users/user/project/tsconfig.json: WARNING: Non-relative path "c*:\\bad" is not allowed when "baseUrl" is not set (did you forget a leading "./"?)
Users/user/project/tsconfig.json: WARNING: Non-relative path "c:*\\bad" is not allowed when "baseUrl" is not set (did you forget a leading "./"?)
Users/user/project/tsconfig.json: WARNING: Non-relative path "http://bad" is not allowed when "baseUrl" is not set (did you forget a leading "./"?)
`,
	})
}

// https://github.com/evanw/esbuild/issues/913
func TestTsconfigPathsOverriddenBaseURL(t *testing.T) {
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

func TestTsconfigPathsOverriddenBaseURLDifferentDir(t *testing.T) {
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

func TestTsconfigPathsMissingBaseURL(t *testing.T) {
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
		expectedScanLog: `Users/user/project/src/entry.ts: ERROR: Could not resolve "#/test"
NOTE: You can mark the path "#/test" as external to exclude it from the bundle, which will remove this error and leave the unresolved path in the bundle.
`,
	})
}

func TestTsconfigPathsTypeOnly(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/entry.ts": `
				import { fib } from "fib";

				console.log(fib(10));
			`,
			"/Users/user/project/node_modules/fib/index.js": `
				export function fib(input) {
					if (input < 2) {
						return input;
					}
					return fib(input - 1) + fib(input - 2);
				}
			`,
			"/Users/user/project/fib-local.d.ts": `
				export function fib(input: number): number;
			`,
			"/Users/user/project/tsconfig.json": `
				{
					"compilerOptions": {
						"baseUrl": ".",
						"paths": {
							"fib": ["fib-local.d.ts"]
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
	})
}

func TestTsconfigJSX(t *testing.T) {
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

func TestTsconfigNestedJSX(t *testing.T) {
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

func TestTsconfigPreserveJSX(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/entry.tsx": `
				console.log(<><div/><div/></>)
			`,
			"/Users/user/project/tsconfig.json": `
				{
					"compilerOptions": {
						"jsx": "preserve" // This should be ignored
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

func TestTsconfigPreserveJSXAutomatic(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/entry.tsx": `
				console.log(<><div/><div/></>)
			`,
			"/Users/user/project/tsconfig.json": `
				{
					"compilerOptions": {
						"jsx": "preserve" // This should be ignored
					}
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/entry.tsx"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
			JSX: config.JSXOptions{
				AutomaticRuntime: true,
			},
			ExternalSettings: config.ExternalSettings{
				PreResolve: config.ExternalMatchers{Exact: map[string]bool{
					"react/jsx-runtime": true,
				}},
			},
		},
	})
}

func TestTsconfigReactJSX(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/entry.tsx": `
				console.log(<><div/><div/></>)
			`,
			"/Users/user/project/tsconfig.json": `
				{
					"compilerOptions": {
						"jsx": "react-jsx",
						"jsxImportSource": "notreact"
					}
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/entry.tsx"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
			ExternalSettings: config.ExternalSettings{
				PreResolve: config.ExternalMatchers{Exact: map[string]bool{
					"notreact/jsx-runtime": true,
				}},
			},
		},
	})
}

func TestTsconfigReactJSXDev(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/entry.tsx": `
				console.log(<><div/><div/></>)
			`,
			"/Users/user/project/tsconfig.json": `
				{
					"compilerOptions": {
						"jsx": "react-jsxdev"
					}
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/entry.tsx"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
			ExternalSettings: config.ExternalSettings{
				PreResolve: config.ExternalMatchers{Exact: map[string]bool{
					"react/jsx-dev-runtime": true,
				}},
			},
		},
	})
}

func TestTsconfigReactJSXWithDevInMainConfig(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/entry.tsx": `
				console.log(<><div/><div/></>)
			`,
			"/Users/user/project/tsconfig.json": `
				{
					"compilerOptions": {
						"jsx": "react-jsx"
					}
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/entry.tsx"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
			JSX: config.JSXOptions{
				Development: true,
			},
			ExternalSettings: config.ExternalSettings{
				PreResolve: config.ExternalMatchers{Exact: map[string]bool{
					"react/jsx-dev-runtime": true,
				}},
			},
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
	tsconfig_suite.expectBundledUnix(t, bundled{
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

	tsconfig_suite.expectBundledWindows(t, bundled{
		files: map[string]string{
			"/Users/user/project/entry.jsx": `
				console.log(<div/>, <></>)
			`,
			"/Users/user/project/tsconfig.json": `
				{
					"extends": "C:\\Users\\user\\project\\base.json",
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
		expectedScanLog: `base.json: WARNING: Base config file "./tsconfig" forms cycle
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
			TSConfigPath:  "/Users/user/project/other/config-for-ts.json",
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
			TSConfigPath:  "/Users/user/project/other/config-for-ts.json",
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
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
			TSConfigPath:  "/this/file/doesn't/exist/tsconfig.json",
		},
		expectedScanLog: `ERROR: Cannot find tsconfig file "this/file/doesn't/exist/tsconfig.json"
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

func TestTsconfigJsonNodeModulesTsconfigPathExact(t *testing.T) {
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
			"/Users/user/project/src/node_modules/foo/package.json": `
				{
					"tsconfig": "over/here.json"
				}
			`,
			"/Users/user/project/src/node_modules/foo/over/here.json": `
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

func TestTsconfigJsonNodeModulesTsconfigPathImplicitJson(t *testing.T) {
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
			"/Users/user/project/src/node_modules/foo/package.json": `
				{
					"tsconfig": "over/here"
				}
			`,
			"/Users/user/project/src/node_modules/foo/over/here.json": `
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

func TestTsconfigJsonNodeModulesTsconfigPathDirectory(t *testing.T) {
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
			"/Users/user/project/src/node_modules/foo/package.json": `
				{
					"tsconfig": "over/here"
				}
			`,
			"/Users/user/project/src/node_modules/foo/over/here/tsconfig.json": `
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

func TestTsconfigJsonNodeModulesTsconfigPathBad(t *testing.T) {
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
			"/Users/user/project/src/node_modules/foo/package.json": `
				{
					"tsconfig": "over/here.json"
				}
			`,
			"/Users/user/project/src/node_modules/foo/tsconfig.json": `
				{
					"compilerOptions": {
						"jsx": "react",
						"jsxFactory": "THIS SHOULD NOT BE LOADED"
					}
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/app/entry.tsx"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/tsconfig.json: WARNING: Cannot find base config file "foo"
`,
	})
}

func TestTsconfigJsonInsideNodeModules(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/app/entry.tsx": `
				import 'foo'
			`,
			"/Users/user/project/src/node_modules/foo/index.tsx": `
				console.log(<div/>)
			`,
			"/Users/user/project/src/node_modules/foo/tsconfig.json": `
				{
					"compilerOptions": {
						"jsxFactory": "TEST_FAILED"
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
		expectedScanLog: `Users/user/project/src/foo/tsconfig.json: WARNING: Cannot find base config file "extends for foo"
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
			ExternalSettings: config.ExternalSettings{
				PostResolve: config.ExternalMatchers{Exact: map[string]bool{
					"/Users/user/project/src/foo": true,
				}},
			},
		},
	})
}

func TestTsconfigImportsNotUsedAsValuesPreserve(t *testing.T) {
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
			ExternalSettings: config.ExternalSettings{
				PostResolve: config.ExternalMatchers{Exact: map[string]bool{
					"/Users/user/project/src/foo": true,
				}},
			},
		},
	})
}

func TestTsconfigPreserveValueImports(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				import {} from "a"
				import {b1} from "b"
				import {c1, type c2} from "c"
				import {d1, d2, type d3} from "d"
				import {type e1, type e2} from "e"
				import f1, {} from "f"
				import g1, {g2} from "g"
				import h1, {type h2} from "h"
				import * as i1 from "i"
				import "j"
			`,
			"/Users/user/project/src/tsconfig.json": `{
				"compilerOptions": {
					"preserveValueImports": true
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:          config.ModeConvertFormat,
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/Users/user/project/out.js",
			ExternalSettings: config.ExternalSettings{
				PostResolve: config.ExternalMatchers{Exact: map[string]bool{
					"/Users/user/project/src/foo": true,
				}},
			},
		},
	})
}

func TestTsconfigPreserveValueImportsAndImportsNotUsedAsValuesPreserve(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				import {} from "a"
				import {b1} from "b"
				import {c1, type c2} from "c"
				import {d1, d2, type d3} from "d"
				import {type e1, type e2} from "e"
				import f1, {} from "f"
				import g1, {g2} from "g"
				import h1, {type h2} from "h"
				import * as i1 from "i"
				import "j"
			`,
			"/Users/user/project/src/tsconfig.json": `{
				"compilerOptions": {
					"importsNotUsedAsValues": "preserve",
					"preserveValueImports": true
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:          config.ModeConvertFormat,
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/Users/user/project/out.js",
			ExternalSettings: config.ExternalSettings{
				PostResolve: config.ExternalMatchers{Exact: map[string]bool{
					"/Users/user/project/src/foo": true,
				}},
			},
		},
	})
}

func TestTsconfigUseDefineForClassFieldsES2020(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				Foo = class {
					useDefine = false
				}
			`,
			"/Users/user/project/src/tsconfig.json": `{
				"compilerOptions": {
					"target": "ES2020"
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:              config.ModeBundle,
			AbsOutputFile:     "/Users/user/project/out.js",
			OriginalTargetEnv: "esnext",
		},
	})
}

func TestTsconfigUseDefineForClassFieldsESNext(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				Foo = class {
					useDefine = true
				}
			`,
			"/Users/user/project/src/tsconfig.json": `{
				"compilerOptions": {
					"target": "ESNext"
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:              config.ModeBundle,
			AbsOutputFile:     "/Users/user/project/out.js",
			OriginalTargetEnv: "esnext",
		},
	})
}

func TestTsconfigUnrecognizedTargetWarning(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				import "./a"
				import "b"
			`,
			"/Users/user/project/src/a/index.ts": ``,
			"/Users/user/project/src/a/tsconfig.json": `{
				"compilerOptions": {
					"target": "es4"
				}
			}`,
			"/Users/user/project/src/node_modules/b/index.ts": ``,
			"/Users/user/project/src/node_modules/b/tsconfig.json": `{
				"compilerOptions": {
					"target": "es4"
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/a/tsconfig.json: WARNING: Unrecognized target environment "es4"
`,
	})
}

func TestTsconfigIgnoredTargetSilent(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				import "./a"
				import "b"
			`,
			"/Users/user/project/src/a/index.ts": ``,
			"/Users/user/project/src/a/tsconfig.json": `{
				"compilerOptions": {
					"target": "es5"
				}
			}`,
			"/Users/user/project/src/node_modules/b/index.ts": ``,
			"/Users/user/project/src/node_modules/b/tsconfig.json": `{
				"compilerOptions": {
					"target": "es5"
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:                  config.ModeBundle,
			AbsOutputFile:         "/Users/user/project/out.js",
			UnsupportedJSFeatures: es(5),
			OriginalTargetEnv:     "ES5",
		},
	})
}

func TestTsconfigNoBaseURLExtendsPaths(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				import { foo } from "foo"
				console.log(foo)
			`,
			"/Users/user/project/lib/foo.ts": `
				export let foo = 123
			`,
			"/Users/user/project/tsconfig.json": `{
				"extends": "./base/defaults"
			}`,
			"/Users/user/project/base/defaults.json": `{
				"compilerOptions": {
					"paths": {
						"*": ["lib/*"]
					}
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/base/defaults.json: WARNING: Non-relative path "lib/*" is not allowed when "baseUrl" is not set (did you forget a leading "./"?)
Users/user/project/src/entry.ts: ERROR: Could not resolve "foo"
NOTE: You can mark the path "foo" as external to exclude it from the bundle, which will remove this error and leave the unresolved path in the bundle.
`,
	})
}

func TestTsconfigBaseURLExtendsPaths(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				import { foo } from "foo"
				console.log(foo)
			`,
			"/Users/user/project/lib/foo.ts": `
				export let foo = 123
			`,
			"/Users/user/project/tsconfig.json": `{
				"extends": "./base/defaults",
				"compilerOptions": {
					"baseUrl": "."
				}
			}`,
			"/Users/user/project/base/defaults.json": `{
				"compilerOptions": {
					"paths": {
						"*": ["lib/*"]
					}
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

func TestTsconfigPathsExtendsBaseURL(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				import { foo } from "foo"
				console.log(foo)
			`,
			"/Users/user/project/base/test/lib/foo.ts": `
				export let foo = 123
			`,
			"/Users/user/project/tsconfig.json": `{
				"extends": "./base/defaults",
				"compilerOptions": {
					"paths": {
						"*": ["lib/*"]
					}
				}
			}`,
			"/Users/user/project/base/defaults.json": `{
				"compilerOptions": {
					"baseUrl": "test"
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

func TestTsconfigPathsInNodeModulesIssue2386(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/main.js": `
				import first from "wow/first";
				import next from "wow/next";
				console.log(first, next);
			`,
			"/Users/user/project/node_modules/wow/package.json": `{
				"name": "wow",
				"type": "module",
				"private": true,
				"exports": {
					"./*": "./dist/*.js"
				},
				"typesVersions": {
					"*": {
						"*": [
							"dist/*"
						]
					}
				}
			}`,
			"/Users/user/project/node_modules/wow/tsconfig.json": `{
				"compilerOptions": {
					"paths": { "wow/*": [ "./*" ] }
				}
			}`,
			"/Users/user/project/node_modules/wow/dist/first.js": `
				export default "dist";
			`,
			"/Users/user/project/node_modules/wow/dist/next.js": `
				import next from "wow/first";
				export default next;
			`,
			"/Users/user/project/node_modules/wow/first.ts": `
				export default "source";
			`,
		},
		entryPaths: []string{"/Users/user/project/main.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestTsconfigWithStatementAlwaysStrictFalse(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				with (x) y
			`,
			"/Users/user/project/tsconfig.json": `{
				"compilerOptions": {
					"alwaysStrict": false
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
			OutputFormat:  config.FormatIIFE,
		},
	})
}

func TestTsconfigWithStatementAlwaysStrictTrue(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				with (x) y
			`,
			"/Users/user/project/tsconfig.json": `{
				"compilerOptions": {
					"alwaysStrict": true
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.ts: ERROR: With statements cannot be used in strict mode
Users/user/project/tsconfig.json: NOTE: TypeScript's "alwaysStrict" setting was enabled here:
`,
	})
}

func TestTsconfigWithStatementStrictFalse(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				with (x) y
			`,
			"/Users/user/project/tsconfig.json": `{
				"compilerOptions": {
					"strict": false
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
			OutputFormat:  config.FormatIIFE,
		},
	})
}

func TestTsconfigWithStatementStrictTrue(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				with (x) y
			`,
			"/Users/user/project/tsconfig.json": `{
				"compilerOptions": {
					"strict": true
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.ts: ERROR: With statements cannot be used in strict mode
Users/user/project/tsconfig.json: NOTE: TypeScript's "strict" setting was enabled here:
`,
	})
}

func TestTsconfigWithStatementStrictFalseAlwaysStrictTrue(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				with (x) y
			`,
			"/Users/user/project/tsconfig.json": `{
				"compilerOptions": {
					"strict": false,
					"alwaysStrict": true
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.ts: ERROR: With statements cannot be used in strict mode
Users/user/project/tsconfig.json: NOTE: TypeScript's "alwaysStrict" setting was enabled here:
`,
	})
}

func TestTsconfigWithStatementStrictTrueAlwaysStrictFalse(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				with (x) y
			`,
			"/Users/user/project/tsconfig.json": `{
				"compilerOptions": {
					"strict": true,
					"alwaysStrict": false
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
			OutputFormat:  config.FormatIIFE,
		},
	})
}

func TestTsconfigAlwaysStrictTrueEmitDirectivePassThrough(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/implicit.ts": `
				console.log('this file should start with "use strict"')
			`,
			"/Users/user/project/src/explicit.ts": `
				'use strict'
				console.log('this file should start with "use strict"')
			`,
			"/Users/user/project/tsconfig.json": `{
				"compilerOptions": {
					"alwaysStrict": true
				}
			}`,
		},
		entryPaths: []string{
			"/Users/user/project/src/implicit.ts",
			"/Users/user/project/src/explicit.ts",
		},
		options: config.Options{
			Mode:         config.ModePassThrough,
			AbsOutputDir: "/Users/user/project/out",
		},
	})
}

func TestTsconfigAlwaysStrictTrueEmitDirectiveFormat(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/implicit.ts": `
				console.log('this file should start with "use strict"')
			`,
			"/Users/user/project/src/explicit.ts": `
				'use strict'
				console.log('this file should start with "use strict"')
			`,
			"/Users/user/project/tsconfig.json": `{
				"compilerOptions": {
					"alwaysStrict": true
				}
			}`,
		},
		entryPaths: []string{
			"/Users/user/project/src/implicit.ts",
			"/Users/user/project/src/explicit.ts",
		},
		options: config.Options{
			Mode:         config.ModeConvertFormat,
			AbsOutputDir: "/Users/user/project/out",
		},
	})
}

func TestTsconfigAlwaysStrictTrueEmitDirectiveBundleIIFE(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/implicit.ts": `
				console.log('this file should start with "use strict"')
			`,
			"/Users/user/project/src/explicit.ts": `
				'use strict'
				console.log('this file should start with "use strict"')
			`,
			"/Users/user/project/tsconfig.json": `{
				"compilerOptions": {
					"alwaysStrict": true
				}
			}`,
		},
		entryPaths: []string{
			"/Users/user/project/src/implicit.ts",
			"/Users/user/project/src/explicit.ts",
		},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/Users/user/project/out",
			OutputFormat: config.FormatIIFE,
		},
	})
}

func TestTsconfigAlwaysStrictTrueEmitDirectiveBundleCJS(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/implicit.ts": `
				console.log('this file should start with "use strict"')
			`,
			"/Users/user/project/src/explicit.ts": `
				'use strict'
				console.log('this file should start with "use strict"')
			`,
			"/Users/user/project/tsconfig.json": `{
				"compilerOptions": {
					"alwaysStrict": true
				}
			}`,
		},
		entryPaths: []string{
			"/Users/user/project/src/implicit.ts",
			"/Users/user/project/src/explicit.ts",
		},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/Users/user/project/out",
			OutputFormat: config.FormatCommonJS,
		},
	})
}

func TestTsconfigAlwaysStrictTrueEmitDirectiveBundleESM(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/implicit.ts": `
				console.log('this file should not start with "use strict"')
			`,
			"/Users/user/project/src/explicit.ts": `
				'use strict'
				console.log('this file should not start with "use strict"')
			`,
			"/Users/user/project/tsconfig.json": `{
				"compilerOptions": {
					"alwaysStrict": true
				}
			}`,
		},
		entryPaths: []string{
			"/Users/user/project/src/implicit.ts",
			"/Users/user/project/src/explicit.ts",
		},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/Users/user/project/out",
			OutputFormat: config.FormatESModule,
		},
	})
}

func TestTsconfigExtendsDotWithoutSlash(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/main.tsx": `
				console.log(<div/>)
			`,
			"/Users/user/project/src/foo.json": `{
				"extends": "."
			}`,
			"/Users/user/project/src/tsconfig.json": `{
				"compilerOptions": {
					"jsxFactory": "success"
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/main.tsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/Users/user/project/out",
			OutputFormat: config.FormatESModule,
			TSConfigPath: "/Users/user/project/src/foo.json",
		},
	})
}

func TestTsconfigExtendsDotDotWithoutSlash(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/main.tsx": `
				console.log(<div/>)
			`,
			"/Users/user/project/src/tsconfig.json": `{
				"extends": ".."
			}`,
			"/Users/user/project/tsconfig.json": `{
				"compilerOptions": {
					"jsxFactory": "success"
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/main.tsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/Users/user/project/out",
			OutputFormat: config.FormatESModule,
		},
	})
}

func TestTsconfigExtendsDotWithSlash(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/main.tsx": `
				console.log(<div/>)
			`,
			"/Users/user/project/src/foo.json": `{
				"extends": "./"
			}`,
			"/Users/user/project/src/tsconfig.json": `{
				"compilerOptions": {
					"jsxFactory": "FAILURE"
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/main.tsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/Users/user/project/out",
			OutputFormat: config.FormatESModule,
			TSConfigPath: "/Users/user/project/src/foo.json",
		},
		expectedScanLog: `Users/user/project/src/foo.json: WARNING: Cannot find base config file "./"
`,
	})
}

func TestTsconfigExtendsDotDotWithSlash(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/main.tsx": `
				console.log(<div/>)
			`,
			"/Users/user/project/src/tsconfig.json": `{
				"extends": "../"
			}`,
			"/Users/user/project/tsconfig.json": `{
				"compilerOptions": {
					"jsxFactory": "FAILURE"
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/main.tsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/Users/user/project/out",
			OutputFormat: config.FormatESModule,
		},
		expectedScanLog: `Users/user/project/src/tsconfig.json: WARNING: Cannot find base config file "../"
`,
	})
}

func TestTsconfigExtendsWithExports(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/main.tsx": `
				console.log(<div/>)
			`,
			"/Users/user/project/tsconfig.json": `{
				"extends": "@whatever/tsconfig/a/b/c"
			}`,
			"/Users/user/project/node_modules/@whatever/tsconfig/package.json": `{
				"exports": {
					"./a/b/c": "./foo.json"
				}
			}`,
			"/Users/user/project/node_modules/@whatever/tsconfig/foo.json": `{
				"compilerOptions": {
					"jsxFactory": "success"
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/main.tsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/Users/user/project/out",
			OutputFormat: config.FormatESModule,
		},
	})
}

func TestTsconfigExtendsWithExportsStar(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/main.tsx": `
				console.log(<div/>)
			`,
			"/Users/user/project/tsconfig.json": `{
				"extends": "@whatever/tsconfig/a/b/c"
			}`,
			"/Users/user/project/node_modules/@whatever/tsconfig/package.json": `{
				"exports": {
					"./*": "./tsconfig.*.json"
				}
			}`,
			"/Users/user/project/node_modules/@whatever/tsconfig/tsconfig.a/b/c.json": `{
				"compilerOptions": {
					"jsxFactory": "success"
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/main.tsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/Users/user/project/out",
			OutputFormat: config.FormatESModule,
		},
	})
}

func TestTsconfigExtendsWithExportsStarTrailing(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/main.tsx": `
				console.log(<div/>)
			`,
			"/Users/user/project/tsconfig.json": `{
				"extends": "@whatever/tsconfig/a/b/c.json"
			}`,
			"/Users/user/project/node_modules/@whatever/tsconfig/package.json": `{
				"exports": {
					"./*": "./tsconfig.*"
				}
			}`,
			"/Users/user/project/node_modules/@whatever/tsconfig/tsconfig.a/b/c.json": `{
				"compilerOptions": {
					"jsxFactory": "success"
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/main.tsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/Users/user/project/out",
			OutputFormat: config.FormatESModule,
		},
	})
}

func TestTsconfigExtendsWithExportsRequire(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/main.tsx": `
				console.log(<div/>)
			`,
			"/Users/user/project/tsconfig.json": `{
				"extends": "@whatever/tsconfig/a/b/c.json"
			}`,
			"/Users/user/project/node_modules/@whatever/tsconfig/package.json": `{
				"exports": {
					"./*": {
						"import": "./import.json",
						"require": "./require.json",
						"default": "./default.json"
					}
				}
			}`,
			"/Users/user/project/node_modules/@whatever/tsconfig/import.json":  `FAILURE`,
			"/Users/user/project/node_modules/@whatever/tsconfig/default.json": `FAILURE`,
			"/Users/user/project/node_modules/@whatever/tsconfig/require.json": `{
				"compilerOptions": {
					"jsxFactory": "success"
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/main.tsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/Users/user/project/out",
			OutputFormat: config.FormatESModule,
		},
	})
}

func TestTsconfigVerbatimModuleSyntaxTrue(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/main.ts": `
				export { Car } from "./car";
				import type * as car from "./car";
				import { type Car } from "./car";
				export { type Car } from "./car";
				import type { A } from "a";
				import { b, type c, type d } from "bcd";
				import { type xyz } from "xyz";
			`,
			"/Users/user/project/tsconfig.json": `{
				"compilerOptions": {
					"verbatimModuleSyntax": true
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/main.ts"},
		options: config.Options{
			Mode:         config.ModePassThrough,
			AbsOutputDir: "/Users/user/project/out",
		},
	})
}

func TestTsconfigVerbatimModuleSyntaxFalse(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/main.ts": `
				export { Car } from "./car";
				import type * as car from "./car";
				import { type Car } from "./car";
				export { type Car } from "./car";
				import type { A } from "a";
				import { b, type c, type d } from "bcd";
				import { type xyz } from "xyz";
			`,
			"/Users/user/project/tsconfig.json": `{
				"compilerOptions": {
					"verbatimModuleSyntax": false
				}
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/main.ts"},
		options: config.Options{
			Mode:         config.ModePassThrough,
			AbsOutputDir: "/Users/user/project/out",
		},
	})
}

func TestTsconfigExtendsArray(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/main.tsx": `
				declare let h: any, frag: any
				console.log(<><div /></>)
			`,
			"/Users/user/project/tsconfig.json": `{
				"extends": [
					"./a.json",
					"./b.json",
				],
			}`,
			"/Users/user/project/a.json": `{
				"compilerOptions": {
					"jsxFactory": "h",
					"jsxFragmentFactory": "FAILURE",
				},
			}`,
			"/Users/user/project/b.json": `{
				"compilerOptions": {
					"jsxFragmentFactory": "frag",
				},
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/main.tsx"},
		options: config.Options{
			Mode:         config.ModePassThrough,
			AbsOutputDir: "/Users/user/project/out",
		},
	})
}

func TestTsconfigExtendsArrayNested(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/main.tsx": `
				import { foo } from 'foo'
				declare let b: any, bBase: any
				export class Foo {
					render = () => <><div /></>
				}
			`,
			"/Users/user/project/tsconfig.json": `{
				"extends": [
					"./a.json",
					"./b.json",
				],
			}`,
			"/Users/user/project/a.json": `{
				"extends": "./a-base.json",
				"compilerOptions": {
					"jsxFactory": "a",
					"jsxFragmentFactory": "a",
					"target": "ES2015",
				},
			}`,
			"/Users/user/project/a-base.json": `{
				"compilerOptions": {
					"jsxFactory": "aBase",
					"jsxFragmentFactory": "aBase",
					"target": "ES2022",
					"verbatimModuleSyntax": true,
				},
			}`,
			"/Users/user/project/b.json": `{
				"extends": "./b-base.json",
				"compilerOptions": {
					"jsxFactory": "b",
				},
			}`,
			"/Users/user/project/b-base.json": `{
				"compilerOptions": {
					"jsxFactory": "bBase",
					"jsxFragmentFactory": "bBase",
				},
			}`,
		},
		entryPaths: []string{"/Users/user/project/src/main.tsx"},
		options: config.Options{
			Mode:              config.ModePassThrough,
			AbsOutputDir:      "/Users/user/project/out",
			OriginalTargetEnv: "esnext",
		},
	})
}

func TestTsconfigIgnoreInsideNodeModules(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/main.ts": `
				import { foo } from 'js-pkg'
				import { bar } from 'ts-pkg'
				import { foo as shimFoo, bar as shimBar } from 'pkg'
				if (foo !== 'foo') throw 'fail: foo'
				if (bar !== 'bar') throw 'fail: bar'
				if (shimFoo !== 'shimFoo') throw 'fail: shimFoo'
				if (shimBar !== 'shimBar') throw 'fail: shimBar'
			`,
			"/Users/user/project/shim.ts": `
				export let foo = 'shimFoo'
				export let bar = 'shimBar'
			`,
			"/Users/user/project/tsconfig.json": `{
				"compilerOptions": {
					"paths": {
						"pkg": ["./shim"],
					},
				},
			}`,
			"/Users/user/project/node_modules/js-pkg/index.js": `
				import { foo as pkgFoo } from 'pkg'
				export let foo = pkgFoo
			`,
			"/Users/user/project/node_modules/ts-pkg/index.ts": `
				import { bar as pkgBar } from 'pkg'
				export let bar = pkgBar
			`,
			"/Users/user/project/node_modules/pkg/index.js": `
				export let foo = 'foo'
				export let bar = 'bar'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/main.ts"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/Users/user/project/out",
		},
	})
}

func TestTsconfigJsonPackagesExternal(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import truePkg from 'pkg1'
				import falsePkg from 'internal/pkg2'
				truePkg()
				falsePkg()
			`,
			"/Users/user/project/src/tsconfig.json": `
				{
					"compilerOptions": {
						"paths": {
							"internal/*": ["./stuff/*"]
						}
					}
				}
			`,
			"/Users/user/project/src/stuff/pkg2.js": `
				export default success
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:             config.ModeBundle,
			AbsOutputFile:    "/Users/user/project/out.js",
			ExternalPackages: true,
		},
	})
}

func TestTsconfigJsonTopLevelMistakeWarning(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				@foo
				class Foo {}
			`,
			"/Users/user/project/src/tsconfig.json": `
				{
					"experimentalDecorators": true
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.ts"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/tsconfig.json: WARNING: Expected the "experimentalDecorators" option to be nested inside a "compilerOptions" object
`,
	})
}

// https://github.com/evanw/esbuild/issues/3307
func TestTsconfigJsonBaseUrlIssue3307(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/tsconfig.json": `
				{
					"compilerOptions": {
						"baseUrl": "./subdir"
					}
				}
			`,
			"/Users/user/project/src/test.ts": `
				export const foo = "well, this is correct...";
			`,
			"/Users/user/project/src/subdir/test.ts": `
				export const foo = "WRONG";
			`,
		},
		entryPaths:    []string{"test.ts"},
		absWorkingDir: "/Users/user/project/src",
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

// https://github.com/evanw/esbuild/issues/3354
func TestTsconfigJsonAsteriskNameCollisionIssue3354(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.ts": `
				import {foo} from "foo";
				foo();
			`,
			"/Users/user/project/src/tsconfig.json": `
				{
					"compilerOptions": {
						"baseUrl": ".",
						"paths": {
							"*": ["web/*"]
						}
					}
				}
			`,
			"/Users/user/project/src/web/foo.ts": `
				import {foo as barFoo} from 'bar/foo';
				export function foo() {
					console.log('web/foo');
					barFoo();
				}
			`,
			"/Users/user/project/src/web/bar/foo/foo.ts": `
				export function foo() {
					console.log('bar/foo');
				}
			`,
			"/Users/user/project/src/web/bar/foo/index.ts": `
				export {foo} from './foo'
			`,
		},
		entryPaths:    []string{"entry.ts"},
		absWorkingDir: "/Users/user/project/src",
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

// https://github.com/evanw/esbuild/issues/3698
func TestTsconfigPackageJsonExportsYarnPnP(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/packages/app/index.tsx": `
				console.log(<div/>)
			`,
			"/Users/user/project/packages/app/tsconfig.json": `
				{
					"extends": "tsconfigs/config"
				}
			`,
			"/Users/user/project/packages/tsconfigs/package.json": `
				{
					"exports": {
						"./config": "./configs/tsconfig.json"
					}
				}
			`,
			"/Users/user/project/packages/tsconfigs/configs/tsconfig.json": `
				{
					"compilerOptions": {
						"jsxFactory": "success"
					}
				}
			`,
			"/Users/user/project/.pnp.data.json": `
				{
					"packageRegistryData": [
						[
							"app",
							[
								[
									"workspace:packages/app",
									{
										"packageLocation": "./packages/app/",
										"packageDependencies": [
											[
												"tsconfigs",
												"workspace:packages/tsconfigs"
											]
										],
										"linkType": "SOFT"
									}
								]
							]
						],
						[
							"tsconfigs",
							[
								[
									"workspace:packages/tsconfigs",
									{
										"packageLocation": "./packages/tsconfigs/",
										"packageDependencies": [],
										"linkType": "SOFT"
									}
								]
							]
						]
					]
				}
			`,
		},
		entryPaths:    []string{"/Users/user/project/packages/app/index.tsx"},
		absWorkingDir: "/Users/user/project",
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}
