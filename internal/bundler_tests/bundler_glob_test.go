package bundler_tests

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
					},
					template: {
						require: require(` + "`./src/${ab}`" + `),
						import: import(` + "`./src/${ab}`" + `),
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
					},
					template: {
						require: require(` + "`./src/${ab}`" + `),
						import: import(` + "`./src/${ab}`" + `),
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
					},
					template: {
						require: require(` + "`./src/${ab}`" + `),
						import: import(` + "`./src/${ab}`" + `),
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
					},
					template: {
						require: require(` + "`./src/${ab}.json`" + `),
						import: import(` + "`./src/${ab}.json`" + `),
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

func TestGlobWildcardSlash(t *testing.T) {
	glob_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const ab = Math.random() < 0.5 ? 'a.js' : 'b.js'
				console.log({
					concat: {
						require: require('./src/' + ab + '.js'),
						import: import('./src/' + ab + '.js'),
					},
					template: {
						require: require(` + "`./src/${ab}.js`" + `),
						import: import(` + "`./src/${ab}.js`" + `),
					},
				})
			`,

			"/src/file-a.js":     `module.exports = 'a'`,
			"/src/file-b.js":     `module.exports = 'b'`,
			"/src/file-a.js.map": `DO NOT BUNDLE`,
			"/src/file-b.js.map": `DO NOT BUNDLE`,

			"/src/nested/dir/file-a.js":     `module.exports = 'a'`,
			"/src/nested/dir/file-b.js":     `module.exports = 'b'`,
			"/src/nested/dir/file-a.js.map": `DO NOT BUNDLE`,
			"/src/nested/dir/file-b.js.map": `DO NOT BUNDLE`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestGlobWildcardNoSlash(t *testing.T) {
	glob_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const ab = Math.random() < 0.5 ? 'a.js' : 'b.js'
				console.log({
					concat: {
						require: require('./src/file-' + ab + '.js'),
						import: import('./src/file-' + ab + '.js'),
					},
					template: {
						require: require(` + "`./src/file-${ab}.js`" + `),
						import: import(` + "`./src/file-${ab}.js`" + `),
					},
				})
			`,

			"/src/file-a.js":     `module.exports = 'a'`,
			"/src/file-b.js":     `module.exports = 'b'`,
			"/src/file-a.js.map": `DO NOT BUNDLE`,
			"/src/file-b.js.map": `DO NOT BUNDLE`,

			"/src/nested/dir/file-a.js":     `DO NOT BUNDLE`,
			"/src/nested/dir/file-b.js":     `DO NOT BUNDLE`,
			"/src/nested/dir/file-a.js.map": `DO NOT BUNDLE`,
			"/src/nested/dir/file-b.js.map": `DO NOT BUNDLE`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}
