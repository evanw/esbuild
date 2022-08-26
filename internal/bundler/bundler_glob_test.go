package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

var glob_suite = suite{
	name: "glob",
}

func TestGlobBasicNoSplitting(t *testing.T) {
	glob_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const ab = Math.random() < 0.5 ? 'a.js' : 'b.js'
				console.log({
					concat: {
						require: require('./src/' + ab),
						import: import('./src/' + ab),
						newURL: new URL('./src/' + ab, import.meta.url),
					},
					template: {
						require: require(` + "`./src/${ab}`" + `),
						import: import(` + "`./src/${ab}`" + `),
						newURL: new URL(` + "`./src/${ab}`" + `, import.meta.url),
					},
				})
			`,
			"/src/a.js": `module.exports = 'a'`,
			"/src/b.js": `module.exports = 'b'`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `entry.js: WARNING: The "new URL(..., import.meta.url)" syntax won't be bundled without code splitting enabled
entry.js: WARNING: The "new URL(..., import.meta.url)" syntax won't be bundled without code splitting enabled
`,
	})
}

func TestGlobBasicSplitting(t *testing.T) {
	glob_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const ab = Math.random() < 0.5 ? 'a.js' : 'b.js'
				console.log({
					concat: {
						require: require('./src/' + ab),
						import: import('./src/' + ab),
						newURL: new URL('./src/' + ab, import.meta.url),
					},
					template: {
						require: require(` + "`./src/${ab}`" + `),
						import: import(` + "`./src/${ab}`" + `),
						newURL: new URL(` + "`./src/${ab}`" + `, import.meta.url),
					},
				})
			`,
			"/src/a.js": `module.exports = 'a'`,
			"/src/b.js": `module.exports = 'b'`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputDir:  "/out",
			CodeSplitting: true,
		},
	})
}

func TestGlobDirDoesNotExist(t *testing.T) {
	glob_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const ab = Math.random() < 0.5 ? 'a.js' : 'b.js'
				console.log({
					concat: {
						require: require('./src/' + ab),
						import: import('./src/' + ab),
						newURL: new URL('./src/' + ab, import.meta.url),
					},
					template: {
						require: require(` + "`./src/${ab}`" + `),
						import: import(` + "`./src/${ab}`" + `),
						newURL: new URL(` + "`./src/${ab}`" + `, import.meta.url),
					},
				})
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputDir:  "/out",
			CodeSplitting: true,
		},
		expectedScanLog: `entry.js: ERROR: Could not resolve require("./src/**/*")
entry.js: ERROR: Could not resolve import("./src/**/*")
entry.js: ERROR: Could not resolve new URL("./src/**/*", import.meta.url)
`,
	})
}

func TestGlobNoMatches(t *testing.T) {
	glob_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const ab = Math.random() < 0.5 ? 'a.js' : 'b.js'
				console.log({
					concat: {
						require: require('./src/' + ab + '.json'),
						import: import('./src/' + ab + '.json'),
						newURL: new URL('./src/' + ab + '.json', import.meta.url),
					},
					template: {
						require: require(` + "`./src/${ab}.json`" + `),
						import: import(` + "`./src/${ab}.json`" + `),
						newURL: new URL(` + "`./src/${ab}.json`" + `, import.meta.url),
					},
				})
			`,
			"/src/dummy.js": ``,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputDir:  "/out",
			CodeSplitting: true,
		},
		expectedScanLog: `entry.js: WARNING: The glob pattern require("./src/**/*.json") did not match any files
entry.js: WARNING: The glob pattern import("./src/**/*.json") did not match any files
entry.js: WARNING: The glob pattern new URL("./src/**/*.json", import.meta.url) did not match any files
`,
	})
}

func TestGlobEntryPointAbsPath(t *testing.T) {
	glob_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				works = true
			`,
		},
		entryPaths: []string{"/Users/user/project/**/*.js"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}
