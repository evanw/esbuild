package bundler

import (
	"fmt"
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

var default_suite11 = suite{
	name: "default_test",
}

func TestSubImportModuleWithPkgBrowser1(t *testing.T) {
	default_suite11.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
			import { v } from "pkg/sub";
			console.log(v);
		`,
			"/node_modules/pkg/package.json": `{ "browser": { "./sub": "./sub/foo.js" } }`,
			"/node_modules/pkg/sub/foo.js": `
				export { version as v } from "sub";
			`,
			"/node_modules/sub/index.js": `export const version = 123`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
	fmt.Print("================================\n\n\n\n")
}

func TestSubImportModuleWithPkgBrowser1Ok(t *testing.T) {
	default_suite11.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
			import { v } from "pkg/sub";
			console.log(v);
		`,
			"/node_modules/pkg/package.json": `{ "browser": { "./sub": "./sub/foo.js" } }`,
			"/node_modules/pkg/sub/foo.js": `
				export { version as v } from "sub1111";
			`,
			"/node_modules/sub1111/index.js": `export const version = 123`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}
