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
