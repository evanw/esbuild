package bundler_tests

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

var yarnpnp_suite = suite{
	name: "yarnpnp",
}

// https://github.com/evanw/esbuild/issues/3698
func TestTsconfigPackageJsonExportsYarnPnP(t *testing.T) {
	t.Parallel()
	yarnpnp_suite.expectBundled(t, bundled{
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

// https://github.com/evanw/esbuild/issues/3915
func TestTsconfigStackOverflowYarnPnP(t *testing.T) {
	t.Parallel()
	yarnpnp_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/entry.jsx": `
				console.log(<div />)
			`,
			"/Users/user/project/tsconfig.json": `
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
						[null, [
							[null, {
								"packageLocation": "./",
								"packageDependencies": [
									["tsconfigs", "virtual:some-path"]
								],
								"linkType": "SOFT"
							}]
						]],
						["tsconfigs", [
							["virtual:some-path", {
								"packageLocation": "./packages/tsconfigs/",
								"packageDependencies": [
									["tsconfigs", "virtual:some-path"]
								],
								"packagePeers": [],
								"linkType": "SOFT"
							}],
							["workspace:packages/tsconfigs", {
								"packageLocation": "./packages/tsconfigs/",
								"packageDependencies": [
									["tsconfigs", "workspace:packages/tsconfigs"]
								],
								"linkType": "SOFT"
							}]
						]]
					]
				}
			`,
		},
		entryPaths:    []string{"/Users/user/project/entry.jsx"},
		absWorkingDir: "/Users/user/project",
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/Users/user/project/out.js",
		},
	})
}
func TestWindowsCrossVolumeReferenceYarnPnP(t *testing.T) {
	t.Parallel()
	yarnpnp_suite.expectBundledWindows(t, bundled{
		files: map[string]string{
			"D:\\project\\entry.jsx": `
				import * as React from 'react'
				console.log(<div />)
			`,
			"C:\\Users\\user\\AppData\\Local\\Yarn\\Berry\\cache\\react.zip\\node_modules\\react\\index.js": `
				export function createElement() {}
			`,
			"D:\\project\\.pnp.data.json": `
				{
					"packageRegistryData": [
						[null, [
							[null, {
								"packageLocation": "./",
								"packageDependencies": [
									["react", "npm:19.1.1"],
									["project", "workspace:."]
								],
								"linkType": "SOFT"
							}]
						]],
						["react", [
							["npm:19.1.1", {
								"packageLocation": "../../C:/Users/user/AppData/Local/Yarn/Berry/cache/react.zip/node_modules/react/",
								"packageDependencies": [
									["react", "npm:19.1.1"]
								],
								"linkType": "HARD"
							}]
						]],
						["project", [
							["workspace:.", {
								"packageLocation": "./",
								"packageDependencies": [
									["react", "npm:19.1.1"],
									["project", "workspace:."]
								],
								"linkType": "SOFT"
							}]
						]]
					]
				}
			`,
		},
		entryPaths:    []string{"D:\\project\\entry.jsx"},
		absWorkingDir: "D:\\project",
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "D:\\project\\out.js",
		},
	})
}
