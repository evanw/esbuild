package bundler_tests

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

func TestTsconfigExtendsWithImports(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/project/src/main.tsx": `console.log(<><div/></>)`,
			"/project/src/tsconfig.json": `{
				"extends": "#base"
			}`,
			"/project/package.json": `{
				"imports": {"#base": "./configs/base.json"}
			}`,
			"/project/configs/base.json": `{
				"extends": "./shared.json",
				"compilerOptions": {"jsxFactory": "success"}
			}`,
			"/project/configs/shared.json": `{
				"compilerOptions": {"jsxFragmentFactory": "fragment"}
			}`,
		},
		entryPaths: []string{"/project/src/main.tsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestTsconfigExtendsWithImportsConditions(t *testing.T) {
	for _, condition := range []string{"require", "node", "types", "default"} {
		t.Run(condition, func(t *testing.T) {
			tsconfig_suite.expectBundled(t, bundled{
				files: map[string]string{
					"/project/main.tsx": `console.log(<div/>)`,
					"/project/tsconfig.json": `{
						"extends": "#base"
					}`,
					"/project/package.json": `{
						"type": "module",
						"imports": {"#base": {
							"browser": "./bad.json",
							"import": "./bad.json",
							"custom": "./bad.json",
							"` + condition + `": "./base.json"
						}}
					}`,
					"/project/base.json": `{
						"compilerOptions": {"jsxFactory": "success"}
					}`,
					"/project/bad.json": `FAILURE`,
				},
				entryPaths: []string{"/project/main.tsx"},
				options: config.Options{
					Mode:         config.ModeBundle,
					AbsOutputDir: "/out",
					Conditions:   []string{"custom"},
				},
			})
		})
	}
}

func TestTsconfigExtendsWithImportsPatternAndArray(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/project/main.tsx": `console.log(<div/>)`,
			"/project/tsconfig.json": `{
				"extends": "#configs/web/base"
			}`,
			"/project/package.json": `{
				"imports": {
					"#configs/*": "./bad.json",
					"#configs/web/*": ["../invalid.json", "./configs/*.json"]
				}
			}`,
			"/project/configs/base.json": `{
				"compilerOptions": {"jsxFactory": "success"}
			}`,
			"/project/bad.json": `FAILURE`,
		},
		entryPaths: []string{"/project/main.tsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestTsconfigExtendsWithImportsNearestPackage(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/project/package.json":              `{"imports": {"#base": "./bad.json"}}`,
			"/project/bad.json":                  `FAILURE`,
			"/project/packages/app/src/main.tsx": `console.log(<div/>)`,
			"/project/packages/app/config/custom.json": `{
				"extends": "#base"
			}`,
			"/project/packages/app/package.json": `{
				"imports": {"#base": "./base.json"}
			}`,
			"/project/packages/app/base.json": `{
				"compilerOptions": {"jsxFactory": "success"}
			}`,
		},
		entryPaths: []string{"/project/packages/app/src/main.tsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			TSConfigPath: "/project/packages/app/config/custom.json",
		},
	})
}

func TestTsconfigExtendsWithImportsPackageTarget(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/project/src/main.tsx": `console.log(<div/>)`,
			"/project/src/tsconfig.json": `{
				"extends": "#base"
			}`,
			"/project/package.json": `{
				"imports": {"#base": "@configs/base/base.json"}
			}`,
			"/project/src/node_modules/@configs/base/base.json": `FAILURE`,
			"/project/node_modules/@configs/base/package.json": `{
				"exports": {"./base.json": {
					"browser": "./bad.json",
					"import": "./bad.json",
					"custom": "./bad.json",
					"require": "./config.json"
				}},
				"imports": {"#shared": "./shared.json"}
			}`,
			"/project/node_modules/@configs/base/bad.json": `FAILURE`,
			"/project/node_modules/@configs/base/config.json": `{
				"extends": "#shared"
			}`,
			"/project/node_modules/@configs/base/shared.json": `{
				"compilerOptions": {"jsxFactory": "success"}
			}`,
		},
		entryPaths: []string{"/project/src/main.tsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
			Conditions:   []string{"custom"},
		},
	})
}

func TestTsconfigExtendsWithImportsYarnPnP(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/project/packages/app/src/main.tsx":      `console.log(<div/>)`,
			"/project/packages/app/src/tsconfig.json": `{"extends": "#base"}`,
			"/project/packages/app/package.json": `{
				"imports": {"#base": "tsconfigs/base.json"}
			}`,
			"/project/packages/tsconfigs/package.json": `{
				"exports": {"./base.json": {
					"browser": "./bad.json",
					"import": "./bad.json",
					"require": "./base.json"
				}}
			}`,
			"/project/packages/tsconfigs/bad.json": `FAILURE`,
			"/project/packages/tsconfigs/base.json": `{
				"compilerOptions": {"jsxFactory": "success"}
			}`,
			"/project/.pnp.data.json": `{
				"packageRegistryData": [
					["app", [
						["workspace:packages/app", {
							"packageLocation": "./packages/app/",
							"packageDependencies": [["tsconfigs", "workspace:packages/tsconfigs"]],
							"linkType": "SOFT"
						}]
					]],
					["tsconfigs", [
						["workspace:packages/tsconfigs", {
							"packageLocation": "./packages/tsconfigs/",
							"packageDependencies": [],
							"linkType": "SOFT"
						}]
					]]
				]
			}`,
		},
		entryPaths:    []string{"/project/packages/app/src/main.tsx"},
		absWorkingDir: "/project",
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestTsconfigExtendsWithImportsAcrossNodeModules(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/project/main.tsx": `console.log(<div/>)`,
			"/project/tsconfig.json": `{
				"extends": "./node_modules/configs/tsconfig.json"
			}`,
			"/project/package.json": `{
				"imports": {"#base": "./base.json"}
			}`,
			// TypeScript also searches ancestors of node_modules for a package scope.
			"/project/node_modules/configs/tsconfig.json": `{"extends": "#base"}`,
			"/project/base.json": `{
				"compilerOptions": {"jsxFactory": "success"}
			}`,
		},
		entryPaths: []string{"/project/main.tsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
	})
}

func TestTsconfigExtendsWithImportsFailure(t *testing.T) {
	for name, packageJSON := range map[string]string{
		"missing-map":     `{}`,
		"missing-entry":   `{"imports": {"#other": "./base.json"}}`,
		"null":            `{"imports": {"#base": null}}`,
		"missing-file":    `{"imports": {"#base": "./missing.json"}}`,
		"invalid":         `{"imports": {"#base": "../base.json"}}`,
		"invalid-segment": `{"imports": {"#base": "./node_modules/base.json"}}`,
		"absolute":        `{"imports": {"#base": "/base.json"}}`,
		"empty":           `{"imports": {"#base": ""}}`,
		"extensionless":   `{"imports": {"#base": "./base"}}`,
		"directory":       `{"imports": {"#base": "./directory.json"}}`,
		// Match normal package imports, which do not recursively remap aliases.
		"alias":       `{"imports": {"#base": "#other", "#other": "./base.json"}}`,
		"alias-cycle": `{"imports": {"#base": "#other", "#other": "#base"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			tsconfig_suite.expectBundled(t, bundled{
				files: map[string]string{
					"/package.json":     `{"imports": {"#base": "./base.json"}}`,
					"/base.json":        `FAILURE`,
					"/project/main.tsx": `console.log(<div/>)`,
					"/project/tsconfig.json": `{
						"extends": "#base",
						"compilerOptions": {"jsxFactory": "fallback"}
					}`,
					"/project/package.json":                 packageJSON,
					"/project/base.json":                    `FAILURE`,
					"/project/node_modules/base.json":       `FAILURE`,
					"/project/directory.json/tsconfig.json": `FAILURE`,
				},
				entryPaths: []string{"/project/main.tsx"},
				options: config.Options{
					Mode:         config.ModeBundle,
					AbsOutputDir: "/out",
				},
				expectedScanLog: `project/tsconfig.json: WARNING: Cannot find base config file "#base"
`,
			})
		})
	}
}

func TestTsconfigExtendsWithImportsCycle(t *testing.T) {
	tsconfig_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/project/main.tsx":      `console.log(<div/>)`,
			"/project/tsconfig.json": `{"extends": "#base"}`,
			"/project/package.json":  `{"imports": {"#base": "./base.json"}}`,
			"/project/base.json": `{
				"extends": "#base",
				"compilerOptions": {"jsxFactory": "success"}
			}`,
		},
		entryPaths: []string{"/project/main.tsx"},
		options: config.Options{
			Mode:         config.ModeBundle,
			AbsOutputDir: "/out",
		},
		expectedScanLog: `project/base.json: WARNING: Base config file "#base" forms cycle
`,
	})
}
