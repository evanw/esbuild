package bundler_tests

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

var importphase_suite = suite{
	name: "importphase",
}

func TestImportDeferExternalESM(t *testing.T) {
	t.Parallel()
	importphase_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import defer * as foo0 from './foo.json'
				import defer * as foo1 from './foo.json' with { type: 'json' }

				console.log(
					foo0,
					foo1,
					import.defer('./foo.json'),
					import.defer('./foo.json', { with: { type: 'json' } }),
					import.defer(` + "`./${foo}.json`" + `),
					import.defer(` + "`./${foo}.json`" + `, { with: { type: 'json' } }),
				)
			`,
			"/foo.json": `{}`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/out.js",
			ExternalSettings: config.ExternalSettings{
				PreResolve: config.ExternalMatchers{
					Patterns: []config.WildcardPattern{{Suffix: ".json"}},
				},
			},
		},
	})
}

func TestImportDeferExternalCommonJS(t *testing.T) {
	t.Parallel()
	importphase_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import defer * as foo0 from './foo.json'
				import defer * as foo1 from './foo.json' with { type: 'json' }

				console.log(
					foo0,
					foo1,
					import.defer('./foo.json'),
					import.defer('./foo.json', { with: { type: 'json' } }),
					import.defer(` + "`./${foo}.json`" + `),
					import.defer(` + "`./${foo}.json`" + `, { with: { type: 'json' } }),
				)
			`,
			"/foo.json": `{}`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatCommonJS,
			AbsOutputFile: "/out.js",
			ExternalSettings: config.ExternalSettings{
				PreResolve: config.ExternalMatchers{
					Patterns: []config.WildcardPattern{{Suffix: ".json"}},
				},
			},
		},
		expectedScanLog: `entry.js: ERROR: Bundling deferred imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "cjs" output format is not supported
`,
	})
}

func TestImportDeferExternalIIFE(t *testing.T) {
	t.Parallel()
	importphase_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import defer * as foo0 from './foo.json'
				import defer * as foo1 from './foo.json' with { type: 'json' }

				console.log(
					foo0,
					foo1,
					import.defer('./foo.json'),
					import.defer('./foo.json', { with: { type: 'json' } }),
					import.defer(` + "`./${foo}.json`" + `),
					import.defer(` + "`./${foo}.json`" + `, { with: { type: 'json' } }),
				)
			`,
			"/foo.json": `{}`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatIIFE,
			AbsOutputFile: "/out.js",
			ExternalSettings: config.ExternalSettings{
				PreResolve: config.ExternalMatchers{
					Patterns: []config.WildcardPattern{{Suffix: ".json"}},
				},
			},
		},
		expectedScanLog: `entry.js: ERROR: Bundling deferred imports with the "iife" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "iife" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "iife" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "iife" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "iife" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "iife" output format is not supported
`,
	})
}

func TestImportDeferInternalESM(t *testing.T) {
	t.Parallel()
	importphase_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import defer * as foo0 from './foo.json'
				import defer * as foo1 from './foo.json' with { type: 'json' }

				console.log(
					foo0,
					foo1,
					import.defer('./foo.json'),
					import.defer('./foo.json', { with: { type: 'json' } }),
					import.defer(` + "`./${foo}.json`" + `),
					import.defer(` + "`./${foo}.json`" + `, { with: { type: 'json' } }),
				)
			`,
			"/foo.json": `{}`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `entry.js: ERROR: Bundling with deferred imports is not supported unless they are external
entry.js: ERROR: Bundling with deferred imports is not supported unless they are external
entry.js: ERROR: Bundling with deferred imports is not supported unless they are external
entry.js: ERROR: Bundling with deferred imports is not supported unless they are external
entry.js: ERROR: Bundling with deferred imports is not supported unless they are external
entry.js: ERROR: Bundling with deferred imports is not supported unless they are external
`,
	})
}

func TestImportDeferInternalCommonJS(t *testing.T) {
	t.Parallel()
	importphase_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import defer * as foo0 from './foo.json'
				import defer * as foo1 from './foo.json' with { type: 'json' }

				console.log(
					foo0,
					foo1,
					import.defer('./foo.json'),
					import.defer('./foo.json', { with: { type: 'json' } }),
					import.defer(` + "`./${foo}.json`" + `),
					import.defer(` + "`./${foo}.json`" + `, { with: { type: 'json' } }),
				)
			`,
			"/foo.json": `{}`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatCommonJS,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `entry.js: ERROR: Bundling deferred imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "cjs" output format is not supported
`,
	})
}

func TestImportDeferInternalIIFE(t *testing.T) {
	t.Parallel()
	importphase_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import defer * as foo0 from './foo.json'
				import defer * as foo1 from './foo.json' with { type: 'json' }

				console.log(
					foo0,
					foo1,
					import.defer('./foo.json'),
					import.defer('./foo.json', { with: { type: 'json' } }),
					import.defer(` + "`./${foo}.json`" + `),
					import.defer(` + "`./${foo}.json`" + `, { with: { type: 'json' } }),
				)
			`,
			"/foo.json": `{}`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatIIFE,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `entry.js: ERROR: Bundling deferred imports with the "iife" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "iife" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "iife" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "iife" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "iife" output format is not supported
entry.js: ERROR: Bundling deferred imports with the "iife" output format is not supported
`,
	})
}

func TestImportSourceExternalESM(t *testing.T) {
	t.Parallel()
	importphase_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import source foo0 from './foo.json'
				import source foo1 from './foo.json' with { type: 'json' }

				console.log(
					foo0,
					foo1,
					import.source('./foo.json'),
					import.source('./foo.json', { with: { type: 'json' } }),
					import.source(` + "`./${foo}.json`" + `),
					import.source(` + "`./${foo}.json`" + `, { with: { type: 'json' } }),
				)
			`,
			"/foo.json": `{}`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/out.js",
			ExternalSettings: config.ExternalSettings{
				PreResolve: config.ExternalMatchers{
					Patterns: []config.WildcardPattern{{Suffix: ".json"}},
				},
			},
		},
	})
}

func TestImportSourceExternalCommonJS(t *testing.T) {
	t.Parallel()
	importphase_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import source foo0 from './foo.json'
				import source foo1 from './foo.json' with { type: 'json' }

				console.log(
					foo0,
					foo1,
					import.source('./foo.json'),
					import.source('./foo.json', { with: { type: 'json' } }),
					import.source(` + "`./${foo}.json`" + `),
					import.source(` + "`./${foo}.json`" + `, { with: { type: 'json' } }),
				)
			`,
			"/foo.json": `{}`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatCommonJS,
			AbsOutputFile: "/out.js",
			ExternalSettings: config.ExternalSettings{
				PreResolve: config.ExternalMatchers{
					Patterns: []config.WildcardPattern{{Suffix: ".json"}},
				},
			},
		},
		expectedScanLog: `entry.js: ERROR: Bundling source phase imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "cjs" output format is not supported
`,
	})
}

func TestImportSourceExternalIIFE(t *testing.T) {
	t.Parallel()
	importphase_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import source foo0 from './foo.json'
				import source foo1 from './foo.json' with { type: 'json' }

				console.log(
					foo0,
					foo1,
					import.source('./foo.json'),
					import.source('./foo.json', { with: { type: 'json' } }),
					import.source(` + "`./${foo}.json`" + `),
					import.source(` + "`./${foo}.json`" + `, { with: { type: 'json' } }),
				)
			`,
			"/foo.json": `{}`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatIIFE,
			AbsOutputFile: "/out.js",
			ExternalSettings: config.ExternalSettings{
				PreResolve: config.ExternalMatchers{
					Patterns: []config.WildcardPattern{{Suffix: ".json"}},
				},
			},
		},
		expectedScanLog: `entry.js: ERROR: Bundling source phase imports with the "iife" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "iife" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "iife" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "iife" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "iife" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "iife" output format is not supported
`,
	})
}

func TestImportSourceInternalESM(t *testing.T) {
	t.Parallel()
	importphase_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import source foo0 from './foo.json'
				import source foo1 from './foo.json' with { type: 'json' }

				console.log(
					foo0,
					foo1,
					import.source('./foo.json'),
					import.source('./foo.json', { with: { type: 'json' } }),
					import.source(` + "`./${foo}.json`" + `),
					import.source(` + "`./${foo}.json`" + `, { with: { type: 'json' } }),
				)
			`,
			"/foo.json": `{}`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `entry.js: ERROR: Bundling with source phase imports is not supported unless they are external
entry.js: ERROR: Bundling with source phase imports is not supported unless they are external
entry.js: ERROR: Bundling with source phase imports is not supported unless they are external
entry.js: ERROR: Bundling with source phase imports is not supported unless they are external
entry.js: ERROR: Bundling with source phase imports is not supported unless they are external
entry.js: ERROR: Bundling with source phase imports is not supported unless they are external
`,
	})
}

func TestImportSourceInternalCommonJS(t *testing.T) {
	t.Parallel()
	importphase_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import source foo0 from './foo.json'
				import source foo1 from './foo.json' with { type: 'json' }

				console.log(
					foo0,
					foo1,
					import.source('./foo.json'),
					import.source('./foo.json', { with: { type: 'json' } }),
					import.source(` + "`./${foo}.json`" + `),
					import.source(` + "`./${foo}.json`" + `, { with: { type: 'json' } }),
				)
			`,
			"/foo.json": `{}`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatCommonJS,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `entry.js: ERROR: Bundling source phase imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "cjs" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "cjs" output format is not supported
`,
	})
}

func TestImportSourceInternalIIFE(t *testing.T) {
	t.Parallel()
	importphase_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import source foo0 from './foo.json'
				import source foo1 from './foo.json' with { type: 'json' }

				console.log(
					foo0,
					foo1,
					import.source('./foo.json'),
					import.source('./foo.json', { with: { type: 'json' } }),
					import.source(` + "`./${foo}.json`" + `),
					import.source(` + "`./${foo}.json`" + `, { with: { type: 'json' } }),
				)
			`,
			"/foo.json": `{}`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatIIFE,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `entry.js: ERROR: Bundling source phase imports with the "iife" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "iife" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "iife" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "iife" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "iife" output format is not supported
entry.js: ERROR: Bundling source phase imports with the "iife" output format is not supported
`,
	})
}
