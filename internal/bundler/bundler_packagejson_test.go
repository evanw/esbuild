package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

var packagejson_suite = suite{
	name: "packagejson",
}

func TestPackageJsonMain(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./custom-main.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/custom-main.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBadMain(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./does-not-exist.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonSyntaxErrorComment(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					// Single-line comment
					"a": 1
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/node_modules/demo-pkg/package.json: error: JSON does not support comments
`,
	})
}

func TestPackageJsonSyntaxErrorTrailingComma(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"a": 1,
					"b": 2,
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/node_modules/demo-pkg/package.json: error: JSON does not support trailing commas
`,
	})
}

func TestPackageJsonModule(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main.js",
					"module": "./main.esm.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.esm.js": `
				export default function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserString(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": "./browser"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/browser.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserMapRelativeToRelative(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main",
					"browser": {
						"./main.js": "./main-browser",
						"./lib/util.js": "./lib/util-browser"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				const util = require('./lib/util')
				module.exports = function() {
					return ['main', util]
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main-browser.js": `
				const util = require('./lib/util')
				module.exports = function() {
					return ['main-browser', util]
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/lib/util.js": `
				module.exports = 'util'
			`,
			"/Users/user/project/node_modules/demo-pkg/lib/util-browser.js": `
				module.exports = 'util-browser'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserMapRelativeToModule(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main",
					"browser": {
						"./util.js": "util-browser"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				const util = require('./util')
				module.exports = function() {
					return ['main', util]
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/util.js": `
				module.exports = 'util'
			`,
			"/Users/user/project/node_modules/util-browser/index.js": `
				module.exports = 'util-browser'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserMapRelativeDisabled(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main",
					"browser": {
						"./util-node.js": false
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				const util = require('./util-node')
				module.exports = function(obj) {
					return util.inspect(obj)
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/util-node.js": `
				module.exports = require('util')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserMapModuleToRelative(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": {
						"node-pkg": "./node-pkg-browser"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/node-pkg-browser.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				const fn = require('node-pkg')
				module.exports = function() {
					return fn()
				}
			`,
			"/Users/user/project/node_modules/node-pkg/index.js": `
				module.exports = function() {
					return 234
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserMapModuleToModule(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": {
						"node-pkg": "node-pkg-browser"
					}
				}
			`,
			"/Users/user/project/node_modules/node-pkg-browser/index.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				const fn = require('node-pkg')
				module.exports = function() {
					return fn()
				}
			`,
			"/Users/user/project/node_modules/node-pkg/index.js": `
				module.exports = function() {
					return 234
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserMapModuleDisabled(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": {
						"node-pkg": false
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				const fn = require('node-pkg')
				module.exports = function() {
					return fn()
				}
			`,
			"/Users/user/project/node_modules/node-pkg/index.js": `
				module.exports = function() {
					return 234
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserMapNativeModuleDisabled(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": {
						"fs": false
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				const fs = require('fs')
				module.exports = function() {
					return fs.readFile()
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserMapAvoidMissing(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'component-classes'
			`,
			"/Users/user/project/node_modules/component-classes/package.json": `
				{
					"browser": {
						"indexof": "component-indexof"
					}
				}
			`,
			"/Users/user/project/node_modules/component-classes/index.js": `
				try {
					var index = require('indexof');
				} catch (err) {
					var index = require('component-indexof');
				}
			`,
			"/Users/user/project/node_modules/component-indexof/index.js": `
				module.exports = function() {
					return 234
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserOverModuleBrowser(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main.js",
					"module": "./main.esm.js",
					"browser": "./main.browser.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.esm.js": `
				export default function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.browser.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			Platform:      config.PlatformBrowser,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserOverMainNode(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main.js",
					"module": "./main.esm.js",
					"browser": "./main.browser.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.esm.js": `
				export default function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.browser.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			Platform:      config.PlatformNode,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserWithModuleBrowser(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main.js",
					"module": "./main.esm.js",
					"browser": {
						"./main.js": "./main.browser.js",
						"./main.esm.js": "./main.browser.esm.js"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.esm.js": `
				export default function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.browser.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.browser.esm.js": `
				export default function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			Platform:      config.PlatformBrowser,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserWithMainNode(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main.js",
					"module": "./main.esm.js",
					"browser": {
						"./main.js": "./main.browser.js",
						"./main.esm.js": "./main.browser.esm.js"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.esm.js": `
				export default function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.browser.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.browser.esm.js": `
				export default function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			Platform:      config.PlatformNode,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserNodeModulesNoExt(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {browser as a} from 'demo-pkg/no-ext'
				import {node as b} from 'demo-pkg/no-ext.js'
				import {browser as c} from 'demo-pkg/ext'
				import {browser as d} from 'demo-pkg/ext.js'
				console.log(a)
				console.log(b)
				console.log(c)
				console.log(d)
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": {
						"./no-ext": "./no-ext-browser.js",
						"./ext.js": "./ext-browser.js"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/no-ext.js": `
				export let node = 'node'
			`,
			"/Users/user/project/node_modules/demo-pkg/no-ext-browser.js": `
				export let browser = 'browser'
			`,
			"/Users/user/project/node_modules/demo-pkg/ext.js": `
				export let node = 'node'
			`,
			"/Users/user/project/node_modules/demo-pkg/ext-browser.js": `
				export let browser = 'browser'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserNodeModulesIndexNoExt(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {browser as a} from 'demo-pkg/no-ext'
				import {node as b} from 'demo-pkg/no-ext/index.js'
				import {browser as c} from 'demo-pkg/ext'
				import {browser as d} from 'demo-pkg/ext/index.js'
				console.log(a)
				console.log(b)
				console.log(c)
				console.log(d)
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": {
						"./no-ext": "./no-ext-browser/index.js",
						"./ext/index.js": "./ext-browser/index.js"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/no-ext/index.js": `
				export let node = 'node'
			`,
			"/Users/user/project/node_modules/demo-pkg/no-ext-browser/index.js": `
				export let browser = 'browser'
			`,
			"/Users/user/project/node_modules/demo-pkg/ext/index.js": `
				export let node = 'node'
			`,
			"/Users/user/project/node_modules/demo-pkg/ext-browser/index.js": `
				export let browser = 'browser'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserNoExt(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {browser as a} from './demo-pkg/no-ext'
				import {node as b} from './demo-pkg/no-ext.js'
				import {browser as c} from './demo-pkg/ext'
				import {browser as d} from './demo-pkg/ext.js'
				console.log(a)
				console.log(b)
				console.log(c)
				console.log(d)
			`,
			"/Users/user/project/src/demo-pkg/package.json": `
				{
					"browser": {
						"./no-ext": "./no-ext-browser.js",
						"./ext.js": "./ext-browser.js"
					}
				}
			`,
			"/Users/user/project/src/demo-pkg/no-ext.js": `
				export let node = 'node'
			`,
			"/Users/user/project/src/demo-pkg/no-ext-browser.js": `
				export let browser = 'browser'
			`,
			"/Users/user/project/src/demo-pkg/ext.js": `
				export let node = 'node'
			`,
			"/Users/user/project/src/demo-pkg/ext-browser.js": `
				export let browser = 'browser'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonBrowserIndexNoExt(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {browser as a} from './demo-pkg/no-ext'
				import {node as b} from './demo-pkg/no-ext/index.js'
				import {browser as c} from './demo-pkg/ext'
				import {browser as d} from './demo-pkg/ext/index.js'
				console.log(a)
				console.log(b)
				console.log(c)
				console.log(d)
			`,
			"/Users/user/project/src/demo-pkg/package.json": `
				{
					"browser": {
						"./no-ext": "./no-ext-browser/index.js",
						"./ext/index.js": "./ext-browser/index.js"
					}
				}
			`,
			"/Users/user/project/src/demo-pkg/no-ext/index.js": `
				export let node = 'node'
			`,
			"/Users/user/project/src/demo-pkg/no-ext-browser/index.js": `
				export let browser = 'browser'
			`,
			"/Users/user/project/src/demo-pkg/ext/index.js": `
				export let node = 'node'
			`,
			"/Users/user/project/src/demo-pkg/ext-browser/index.js": `
				export let browser = 'browser'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonDualPackageHazardImportOnly(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import value from 'demo-pkg'
				console.log(value)
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main.js",
					"module": "./module.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				module.exports = 'main'
			`,
			"/Users/user/project/node_modules/demo-pkg/module.js": `
				export default 'module'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonDualPackageHazardRequireOnly(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				console.log(require('demo-pkg'))
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main.js",
					"module": "./module.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				module.exports = 'main'
			`,
			"/Users/user/project/node_modules/demo-pkg/module.js": `
				export default 'module'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonDualPackageHazardImportAndRequireSameFile(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import value from 'demo-pkg'
				console.log(value, require('demo-pkg'))
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main.js",
					"module": "./module.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				module.exports = 'main'
			`,
			"/Users/user/project/node_modules/demo-pkg/module.js": `
				export default 'module'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonDualPackageHazardImportAndRequireSeparateFiles(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import './test-main'
				import './test-module'
			`,
			"/Users/user/project/src/test-main.js": `
				console.log(require('demo-pkg'))
			`,
			"/Users/user/project/src/test-module.js": `
				import value from 'demo-pkg'
				console.log(value)
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main.js",
					"module": "./module.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				module.exports = 'main'
			`,
			"/Users/user/project/node_modules/demo-pkg/module.js": `
				export default 'module'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonDualPackageHazardImportAndRequireForceModuleBeforeMain(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import './test-main'
				import './test-module'
			`,
			"/Users/user/project/src/test-main.js": `
				console.log(require('demo-pkg'))
			`,
			"/Users/user/project/src/test-module.js": `
				import value from 'demo-pkg'
				console.log(value)
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main.js",
					"module": "./module.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				module.exports = 'main'
			`,
			"/Users/user/project/node_modules/demo-pkg/module.js": `
				export default 'module'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			MainFields:    []string{"module", "main"},
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonDualPackageHazardImportAndRequireImplicitMain(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import './test-index'
				import './test-module'
			`,
			"/Users/user/project/src/test-index.js": `
				console.log(require('demo-pkg'))
			`,
			"/Users/user/project/src/test-module.js": `
				import value from 'demo-pkg'
				console.log(value)
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"module": "./module.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				module.exports = 'index'
			`,
			"/Users/user/project/node_modules/demo-pkg/module.js": `
				export default 'module'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonDualPackageHazardImportAndRequireImplicitMainForceModuleBeforeMain(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import './test-index'
				import './test-module'
			`,
			"/Users/user/project/src/test-index.js": `
				console.log(require('demo-pkg'))
			`,
			"/Users/user/project/src/test-module.js": `
				import value from 'demo-pkg'
				console.log(value)
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"module": "./module.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				module.exports = 'index'
			`,
			"/Users/user/project/node_modules/demo-pkg/module.js": `
				export default 'module'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			MainFields:    []string{"module", "main"},
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonDualPackageHazardImportAndRequireBrowser(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import './test-main'
				import './test-module'
			`,
			"/Users/user/project/src/test-main.js": `
				console.log(require('demo-pkg'))
			`,
			"/Users/user/project/src/test-module.js": `
				import value from 'demo-pkg'
				console.log(value)
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main.js",
					"module": "./module.js",
					"browser": {
						"./main.js": "./main.browser.js",
						"./module.js": "./module.browser.js"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				module.exports = 'main'
			`,
			"/Users/user/project/node_modules/demo-pkg/module.js": `
				export default 'module'
			`,
			"/Users/user/project/node_modules/demo-pkg/main.browser.js": `
				module.exports = 'browser main'
			`,
			"/Users/user/project/node_modules/demo-pkg/module.browser.js": `
				export default 'browser module'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonMainFieldsA(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import value from 'demo-pkg'
				console.log(value)
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"a": "./a.js",
					"b": "./b.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/a.js": `
				module.exports = 'a'
			`,
			"/Users/user/project/node_modules/demo-pkg/b.js": `
				export default 'b'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			MainFields:    []string{"a", "b"},
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonMainFieldsB(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import value from 'demo-pkg'
				console.log(value)
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"a": "./a.js",
					"b": "./b.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/a.js": `
				module.exports = 'a'
			`,
			"/Users/user/project/node_modules/demo-pkg/b.js": `
				export default 'b'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			MainFields:    []string{"b", "a"},
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonNeutralNoDefaultMainFields(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main.js",
					"module": "./main.esm.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.esm.js": `
				export default function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			Platform:      config.PlatformNeutral,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "demo-pkg" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/demo-pkg/package.json: note: The "main" field was ignored (main fields must be configured manually when using the "neutral" platform)
`,
	})
}

func TestPackageJsonNeutralExplicitMainFields(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"hello": "./main.js",
					"module": "./main.esm.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			Platform:      config.PlatformNeutral,
			MainFields:    []string{"hello"},
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonExportsErrorInvalidModuleSpecifier(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg1'
				import 'pkg2'
				import 'pkg3'
				import 'pkg4'
				import 'pkg5'
				import 'pkg6'
			`,
			"/Users/user/project/node_modules/pkg1/package.json": `
				{ "exports": { ".": "./%%" } }
			`,
			"/Users/user/project/node_modules/pkg2/package.json": `
				{ "exports": { ".": "./%2f" } }
			`,
			"/Users/user/project/node_modules/pkg3/package.json": `
				{ "exports": { ".": "./%2F" } }
			`,
			"/Users/user/project/node_modules/pkg4/package.json": `
				{ "exports": { ".": "./%5c" } }
			`,
			"/Users/user/project/node_modules/pkg5/package.json": `
				{ "exports": { ".": "./%5C" } }
			`,
			"/Users/user/project/node_modules/pkg6/package.json": `
				{ "exports": { ".": "./%31.js" } }
			`,
			"/Users/user/project/node_modules/pkg6/1.js": `
				console.log(1)
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "pkg1" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg1/package.json: note: The module specifier "./%%" is invalid
Users/user/project/src/entry.js: error: Could not resolve "pkg2" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg2/package.json: note: The module specifier "./%2f" is invalid
Users/user/project/src/entry.js: error: Could not resolve "pkg3" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg3/package.json: note: The module specifier "./%2F" is invalid
Users/user/project/src/entry.js: error: Could not resolve "pkg4" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg4/package.json: note: The module specifier "./%5c" is invalid
Users/user/project/src/entry.js: error: Could not resolve "pkg5" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg5/package.json: note: The module specifier "./%5C" is invalid
`,
	})
}

func TestPackageJsonExportsErrorInvalidPackageConfiguration(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg1'
				import 'pkg2/foo'
			`,
			"/Users/user/project/node_modules/pkg1/package.json": `
				{ "exports": { ".": false } }
			`,
			"/Users/user/project/node_modules/pkg2/package.json": `
				{ "exports": { "./foo": false } }
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/node_modules/pkg1/package.json: warning: This value must be a string, an object, an array, or null
Users/user/project/node_modules/pkg2/package.json: warning: This value must be a string, an object, an array, or null
Users/user/project/src/entry.js: error: Could not resolve "pkg1" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg1/package.json: note: The package configuration has an invalid value here
Users/user/project/src/entry.js: error: Could not resolve "pkg2/foo" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg2/package.json: note: The package configuration has an invalid value here
`,
	})
}

func TestPackageJsonExportsErrorInvalidPackageTarget(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg1'
				import 'pkg2'
				import 'pkg3'
			`,
			"/Users/user/project/node_modules/pkg1/package.json": `
				{ "exports": { ".": "invalid" } }
			`,
			"/Users/user/project/node_modules/pkg2/package.json": `
				{ "exports": { ".": "../pkg3" } }
			`,
			"/Users/user/project/node_modules/pkg3/package.json": `
				{ "exports": { ".": "./node_modules/pkg" } }
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "pkg1" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg1/package.json: note: The package target "invalid" is invalid
Users/user/project/src/entry.js: error: Could not resolve "pkg2" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg2/package.json: note: The package target "../pkg3" is invalid
Users/user/project/src/entry.js: error: Could not resolve "pkg3" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg3/package.json: note: The package target "./node_modules/pkg" is invalid
`,
	})
}

func TestPackageJsonExportsErrorPackagePathNotExported(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg1/foo'
			`,
			"/Users/user/project/node_modules/pkg1/package.json": `
				{ "exports": { ".": {} } }
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "pkg1/foo" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg1/package.json: note: The path "./foo" is not exported by package "pkg1"
`,
	})
}

func TestPackageJsonExportsErrorModuleNotFound(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg1'
			`,
			"/Users/user/project/node_modules/pkg1/package.json": `
				{ "exports": { ".": "./foo.js" } }
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "pkg1" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg1/package.json: note: The module "./foo.js" was not found on the file system
`,
	})
}

func TestPackageJsonExportsErrorUnsupportedDirectoryImport(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg1'
				import 'pkg2'
			`,
			"/Users/user/project/node_modules/pkg1/package.json": `
				{ "exports": { ".": "./foo/" } }
			`,
			"/Users/user/project/node_modules/pkg2/package.json": `
				{ "exports": { ".": "./foo" } }
			`,
			"/Users/user/project/node_modules/pkg2/foo/bar.js": `
				console.log(bar)
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "pkg1" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg1/package.json: note: The module "./foo" was not found on the file system
Users/user/project/src/entry.js: error: Could not resolve "pkg2" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg2/package.json: note: Importing the directory "./foo" is not supported
`,
	})
}

func TestPackageJsonExportsRequireOverImport(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				require('pkg')
			`,
			"/Users/user/project/node_modules/pkg/package.json": `
				{
					"exports": {
						"import": "./import.js",
						"require": "./require.js",
						"default": "./default.js"
					}
				}
			`,
			"/Users/user/project/node_modules/pkg/import.js": `
				console.log('FAILURE')
			`,
			"/Users/user/project/node_modules/pkg/require.js": `
				console.log('SUCCESS')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonExportsImportOverRequire(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg'
			`,
			"/Users/user/project/node_modules/pkg/package.json": `
				{
					"exports": {
						"require": "./require.js",
						"import": "./import.js",
						"default": "./default.js"
					}
				}
			`,
			"/Users/user/project/node_modules/pkg/require.js": `
				console.log('FAILURE')
			`,
			"/Users/user/project/node_modules/pkg/import.js": `
				console.log('SUCCESS')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonExportsDefaultOverImportAndRequire(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg'
			`,
			"/Users/user/project/node_modules/pkg/package.json": `
				{
					"exports": {
						"default": "./default.js",
						"import": "./import.js",
						"require": "./require.js"
					}
				}
			`,
			"/Users/user/project/node_modules/pkg/require.js": `
				console.log('FAILURE')
			`,
			"/Users/user/project/node_modules/pkg/import.js": `
				console.log('FAILURE')
			`,
			"/Users/user/project/node_modules/pkg/default.js": `
				console.log('SUCCESS')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonExportsBrowser(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg'
			`,
			"/Users/user/project/node_modules/pkg/package.json": `
				{
					"exports": {
						"node": "./node.js",
						"browser": "./browser.js",
						"default": "./default.js"
					}
				}
			`,
			"/Users/user/project/node_modules/pkg/node.js": `
				console.log('FAILURE')
			`,
			"/Users/user/project/node_modules/pkg/browser.js": `
				console.log('SUCCESS')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
			Platform:      config.PlatformBrowser,
		},
	})
}

func TestPackageJsonExportsNode(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg'
			`,
			"/Users/user/project/node_modules/pkg/package.json": `
				{
					"exports": {
						"browser": "./browser.js",
						"node": "./node.js",
						"default": "./default.js"
					}
				}
			`,
			"/Users/user/project/node_modules/pkg/browser.js": `
				console.log('FAILURE')
			`,
			"/Users/user/project/node_modules/pkg/node.js": `
				console.log('SUCCESS')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
			Platform:      config.PlatformNode,
		},
	})
}

func TestPackageJsonExportsNeutral(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg'
			`,
			"/Users/user/project/node_modules/pkg/package.json": `
				{
					"exports": {
						"node": "./node.js",
						"browser": "./browser.js",
						"default": "./default.js"
					}
				}
			`,
			"/Users/user/project/node_modules/pkg/node.js": `
				console.log('FAILURE')
			`,
			"/Users/user/project/node_modules/pkg/browser.js": `
				console.log('FAILURE')
			`,
			"/Users/user/project/node_modules/pkg/default.js": `
				console.log('SUCCESS')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
			Platform:      config.PlatformNeutral,
		},
	})
}

func TestPackageJsonExportsOrderIndependent(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg1/foo/bar.js'
				import 'pkg2/foo/bar.js'
			`,
			"/Users/user/project/node_modules/pkg1/package.json": `
				{
					"exports": {
						"./": "./1/",
						"./foo/": "./2/"
					}
				}
			`,
			"/Users/user/project/node_modules/pkg1/1/foo/bar.js": `
				console.log('FAILURE')
			`,
			"/Users/user/project/node_modules/pkg1/2/bar.js": `
				console.log('SUCCESS')
			`,
			"/Users/user/project/node_modules/pkg2/package.json": `
				{
					"exports": {
						"./foo/": "./1/",
						"./": "./2/"
					}
				}
			`,
			"/Users/user/project/node_modules/pkg2/1/bar.js": `
				console.log('SUCCESS')
			`,
			"/Users/user/project/node_modules/pkg2/2/foo/bar.js": `
				console.log('FAILURE')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonExportsWildcard(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg1/foo'
				import 'pkg1/foo2'
			`,
			"/Users/user/project/node_modules/pkg1/package.json": `
				{
					"exports": {
						"./foo*": "./file*.js"
					}
				}
			`,
			"/Users/user/project/node_modules/pkg1/file2.js": `
				console.log('SUCCESS')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "pkg1/foo" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg1/package.json: note: The path "./foo" is not exported by package "pkg1"
`,
	})
}

func TestPackageJsonExportsErrorMissingTrailingSlash(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg1/foo/bar'
			`,
			"/Users/user/project/node_modules/pkg1/package.json": `
				{ "exports": { "./foo/": "./test" } }
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "pkg1/foo/bar" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg1/package.json: note: The module specifier "./test" is invalid
`,
	})
}

func TestPackageJsonExportsCustomConditions(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg1'
			`,
			"/Users/user/project/node_modules/pkg1/package.json": `
				{
					"exports": {
						"custom1": "./custom1.js",
						"custom2": "./custom2.js",
						"default": "./default.js"
					}
				}
			`,
			"/Users/user/project/node_modules/pkg1/custom2.js": `
				console.log('SUCCESS')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
			Conditions:    []string{"custom2"},
		},
	})
}

func TestPackageJsonExportsNotExactMissingExtension(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg1/foo/bar'
			`,
			"/Users/user/project/node_modules/pkg1/package.json": `
				{
					"exports": {
						"./foo/": "./dir/"
					}
				}
			`,
			"/Users/user/project/node_modules/pkg1/dir/bar.js": `
				console.log('SUCCESS')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonExportsNotExactMissingExtensionPattern(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg1/foo/bar'
			`,
			"/Users/user/project/node_modules/pkg1/package.json": `
				{
					"exports": {
						"./foo/*": "./dir/*"
					}
				}
			`,
			"/Users/user/project/node_modules/pkg1/dir/bar.js": `
				console.log('SUCCESS')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "pkg1/foo/bar" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg1/package.json: note: The module "./dir/bar" was not found on the file system
`,
	})
}

func TestPackageJsonExportsExactMissingExtension(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg1/foo/bar'
			`,
			"/Users/user/project/node_modules/pkg1/package.json": `
				{
					"exports": {
						"./foo/bar": "./dir/bar"
					}
				}
			`,
			"/Users/user/project/node_modules/pkg1/dir/bar.js": `
				console.log('SUCCESS')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "pkg1/foo/bar" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg1/package.json: note: The module "./dir/bar" was not found on the file system
`,
	})
}

func TestPackageJsonExportsNoConditionsMatch(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg1'
				import 'pkg1/foo.js'
			`,
			"/Users/user/project/node_modules/pkg1/package.json": `
				{
					"exports": {
						".": {
							"what": "./foo.js"
						},
						"./foo.js": {
							"what": "./foo.js"
						}
					}
				}
			`,
			"/Users/user/project/node_modules/pkg1/foo.js": `
				console.log('FAILURE')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "pkg1" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg1/package.json: note: The path "." is not currently exported by package "pkg1"
Users/user/project/node_modules/pkg1/package.json: note: None of the conditions provided ("what") match any of the currently active conditions ("browser", "default", "import")
Users/user/project/src/entry.js: error: Could not resolve "pkg1/foo.js" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg1/package.json: note: The path "./foo.js" is not currently exported by package "pkg1"
Users/user/project/node_modules/pkg1/package.json: note: None of the conditions provided ("what") match any of the currently active conditions ("browser", "default", "import")
`,
	})
}

func TestPackageJsonExportsMustUseRequire(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'pkg1'
				import 'pkg1/foo.js'
			`,
			"/Users/user/project/node_modules/pkg1/package.json": `
				{
					"exports": {
						".": {
							"require": "./foo.js"
						},
						"./foo.js": {
							"require": "./foo.js"
						}
					}
				}
			`,
			"/Users/user/project/node_modules/pkg1/foo.js": `
				console.log('FAILURE')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "pkg1" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg1/package.json: note: The path "." is not currently exported by package "pkg1"
Users/user/project/node_modules/pkg1/package.json: note: None of the conditions provided ("require") match any of the currently active conditions ("browser", "default", "import")
Users/user/project/src/entry.js: note: Consider using a "require()" call to import this file
Users/user/project/src/entry.js: error: Could not resolve "pkg1/foo.js" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/pkg1/package.json: note: The path "./foo.js" is not currently exported by package "pkg1"
Users/user/project/node_modules/pkg1/package.json: note: None of the conditions provided ("require") match any of the currently active conditions ("browser", "default", "import")
Users/user/project/src/entry.js: note: Consider using a "require()" call to import this file
`,
	})
}

func TestPackageJsonExportsMustUseImport(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				require('pkg1')
				require('pkg1/foo.js')
			`,
			"/Users/user/project/node_modules/pkg1/package.json": `
				{
					"exports": {
						".": {
							"import": "./foo.js"
						},
						"./foo.js": {
							"import": "./foo.js"
						}
					}
				}
			`,
			"/Users/user/project/node_modules/pkg1/foo.js": `
				console.log('FAILURE')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "pkg1" (mark it as external to exclude it from the bundle, or surround it with try/catch to handle the failure at run-time)
Users/user/project/node_modules/pkg1/package.json: note: The path "." is not currently exported by package "pkg1"
Users/user/project/node_modules/pkg1/package.json: note: None of the conditions provided ("import") match any of the currently active conditions ("browser", "default", "require")
Users/user/project/src/entry.js: note: Consider using an "import" statement to import this file
Users/user/project/src/entry.js: error: Could not resolve "pkg1/foo.js" (mark it as external to exclude it from the bundle, or surround it with try/catch to handle the failure at run-time)
Users/user/project/node_modules/pkg1/package.json: note: The path "./foo.js" is not currently exported by package "pkg1"
Users/user/project/node_modules/pkg1/package.json: note: None of the conditions provided ("import") match any of the currently active conditions ("browser", "default", "require")
Users/user/project/src/entry.js: note: Consider using an "import" statement to import this file
`,
	})
}

func TestPackageJsonExportsReverseLookup(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				require('pkg/path/to/real/file')
				require('pkg/path/to/other/file')
			`,
			"/Users/user/project/node_modules/pkg/package.json": `
				{
					"exports": {
						"./lib/te*": {
							"default": "./path/to/re*.js"
						},
						"./extra/": {
							"default": "./path/to/"
						}
					}
				}
			`,
			"/Users/user/project/node_modules/pkg/path/to/real/file.js":  ``,
			"/Users/user/project/node_modules/pkg/path/to/other/file.js": ``,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "pkg/path/to/real/file" (mark it as external to exclude it from the bundle, or surround it with try/catch to handle the failure at run-time)
Users/user/project/node_modules/pkg/package.json: note: The path "./path/to/real/file" is not exported by package "pkg"
Users/user/project/node_modules/pkg/package.json: note: The file "./path/to/real/file.js" is exported at path "./lib/teal/file"
Users/user/project/src/entry.js: note: Import from "pkg/lib/teal/file" to get the file "Users/user/project/node_modules/pkg/path/to/real/file.js"
Users/user/project/src/entry.js: error: Could not resolve "pkg/path/to/other/file" (mark it as external to exclude it from the bundle, or surround it with try/catch to handle the failure at run-time)
Users/user/project/node_modules/pkg/package.json: note: The path "./path/to/other/file" is not exported by package "pkg"
Users/user/project/node_modules/pkg/package.json: note: The file "./path/to/other/file.js" is exported at path "./extra/other/file.js"
Users/user/project/src/entry.js: note: Import from "pkg/extra/other/file.js" to get the file "Users/user/project/node_modules/pkg/path/to/other/file.js"
`,
	})
}

func TestPackageJsonImports(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import '#top-level'
				import '#nested/path.js'
				import '#star/c.js'
				import '#slash/d.js'
			`,
			"/Users/user/project/src/package.json": `
				{
					"imports": {
						"#top-level": "./a.js",
						"#nested/path.js": "./b.js",
						"#star/*": "./some-star/*",
						"#slash/": "./some-slash/"
					}
				}
			`,
			"/Users/user/project/src/a.js":            `console.log('a.js')`,
			"/Users/user/project/src/b.js":            `console.log('b.js')`,
			"/Users/user/project/src/some-star/c.js":  `console.log('c.js')`,
			"/Users/user/project/src/some-slash/d.js": `console.log('d.js')`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonImportsRemapToOtherPackage(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import '#top-level'
				import '#nested/path.js'
				import '#star/c.js'
				import '#slash/d.js'
			`,
			"/Users/user/project/src/package.json": `
				{
					"imports": {
						"#top-level": "pkg/a.js",
						"#nested/path.js": "pkg/b.js",
						"#star/*": "pkg/some-star/*",
						"#slash/": "pkg/some-slash/"
					}
				}
			`,
			"/Users/user/project/src/node_modules/pkg/a.js":            `console.log('a.js')`,
			"/Users/user/project/src/node_modules/pkg/b.js":            `console.log('b.js')`,
			"/Users/user/project/src/node_modules/pkg/some-star/c.js":  `console.log('c.js')`,
			"/Users/user/project/src/node_modules/pkg/some-slash/d.js": `console.log('d.js')`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}

func TestPackageJsonImportsErrorMissingRemappedPackage(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import '#foo'
			`,
			"/Users/user/project/src/package.json": `
				{
					"imports": {
						"#foo": "bar"
					}
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "#foo" (mark it as external to exclude it from the bundle)
Users/user/project/src/package.json: note: The remapped path "bar" could not be resolved
`,
	})
}

func TestPackageJsonImportsInvalidPackageConfiguration(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import '#foo'
			`,
			"/Users/user/project/src/package.json": `
				{
					"imports": "#foo"
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "#foo" (mark it as external to exclude it from the bundle)
Users/user/project/src/package.json: note: The package configuration has an invalid value here
Users/user/project/src/package.json: warning: The value for "imports" must be an object
`,
	})
}

func TestPackageJsonImportsErrorEqualsHash(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import '#'
			`,
			"/Users/user/project/src/package.json": `
				{
					"imports": {}
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "#" (mark it as external to exclude it from the bundle)
Users/user/project/src/package.json: note: This "imports" map was ignored because the module specifier "#" is invalid
`,
	})
}

func TestPackageJsonImportsErrorStartsWithHashSlash(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import '#/foo'
			`,
			"/Users/user/project/src/package.json": `
				{
					"imports": {}
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "#/foo" (mark it as external to exclude it from the bundle)
Users/user/project/src/package.json: note: This "imports" map was ignored because the module specifier "#/foo" is invalid
`,
	})
}

func TestPackageJsonMainFieldsErrorMessageDefault(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'foo'
			`,
			"/Users/user/project/node_modules/foo/package.json": `
				{
					"main": "./foo"
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "foo" (mark it as external to exclude it from the bundle)
`,
	})
}

func TestPackageJsonMainFieldsErrorMessageNotIncluded(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'foo'
			`,
			"/Users/user/project/node_modules/foo/package.json": `
				{
					"main": "./foo"
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
			MainFields:    []string{"some", "fields"},
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "foo" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/foo/package.json: note: The "main" field was ignored because the list of main fields to use is currently set to ["some", "fields"]
`,
	})
}

func TestPackageJsonMainFieldsErrorMessageEmpty(t *testing.T) {
	packagejson_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'foo'
			`,
			"/Users/user/project/node_modules/foo/package.json": `
				{
					"main": "./foo"
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
			MainFields:    []string{},
		},
		expectedScanLog: `Users/user/project/src/entry.js: error: Could not resolve "foo" (mark it as external to exclude it from the bundle)
Users/user/project/node_modules/foo/package.json: note: The "main" field was ignored because the list of main fields to use is currently set to []
`,
	})
}
