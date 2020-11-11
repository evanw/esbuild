package main

import (
	"fmt"
	"github.com/evanw/esbuild/internal/snap_api"
	"github.com/evanw/esbuild/pkg/api"
)

func main() {
	snap_api.SnapCmd(nodeExpress)
}

/* Manually determined
var deferModules = []string{
	"./node_modules/debug/src/index.js",         // tty
	"./node_modules/safer-buffer/safer.js",      // buffer
	"./node_modules/iconv-lite/lib/index.js",    // stream
	"./node_modules/send/index.js",              // stream
	"./node_modules/depd/index.js",              // process function (cwd)
	"./node_modules/body-parser/index.js",       // depd
	"./node_modules/express/lib/request.js",     // http
	"./node_modules/express/lib/response.js",    // http
	"./node_modules/express/lib/application.js", // depd
}
*/

// Determined via doctor + depd with body-parser force added
var deferModules = []string{
	"./node_modules/depd/index.js",
	"./node_modules/body-parser/index.js",
	"./node_modules/send/index.js",
	"./node_modules/express/lib/request.js",
	"./node_modules/express/lib/router/route.js",
	"./node_modules/express/lib/response.js",
	"./node_modules/express/lib/router/index.js",
	"./node_modules/express/lib/application.js",
}

func shouldReplaceRequire(mdl string) bool {
	for _, m := range deferModules {
		if m == mdl {
			fmt.Println(mdl)
			return true
		}
	}
	return false
}

func nodeExpress(args *snap_api.SnapCmdArgs) api.BuildResult {
	platform := api.PlatformNode
	var external []string

	return api.Build(api.BuildOptions{
		// https://esbuild.github.io/api/#log-level
		LogLevel: api.LogLevelInfo,

		// https://esbuild.github.io/api/#target
		Target: api.ES2020,

		// inline any imported dependencies into the file itself
		// https://esbuild.github.io/api/#bundle
		Bundle: true,

		// write out a JSON file with metadata about the build
		// https://esbuild.github.io/api/#metafile
		Metafile: args.Metafile,

		// Applies when one entry point is used.
		// https://esbuild.github.io/api/#outfile
		Outfile:     args.Outfile,
		EntryPoints: []string{args.EntryPoint},

		// https://esbuild.github.io/getting-started/#bundling-for-node
		// https://esbuild.github.io/api/#platform
		//
		// Setting to Node results in:
		// - the default output format is set to cjs
		// - built-in node modules such as fs are automatically marked as external
		// - disables the interpretation of the browser field in package.json
		Platform: platform,
		Engines: []api.Engine{
			{Name: api.EngineNode, Version: "12.4"},
		},

		// https://esbuild.github.io/api/#format
		// three possible values: iife, cjs, and esm
		Format: api.FormatCommonJS,

		// the import will be preserved and will be evaluated at run time instead
		// https://esbuild.github.io/api/#external
		External: external,

		//
		// Combination of the below two might be a better way to replace globals
		// while taking the snapshot
		// We'd copy the code for each from the electron blueprint and add it to
		// a module which we use to inject.
		//

		// replace a global variable with an import from another file.
		// https://esbuild.github.io/api/#inject
		// i.e. Inject:      []string{"./process-shim.js"},
		Inject: nil,

		// replace global identifiers with constant expressions
		// https://esbuild.github.io/api/#define
		// i.e.: Define: map[string]string{"DEBUG": "true"},
		Define: nil,

		// When `false` a buffer is returned instead
		// https://esbuild.github.io/api/#write
		Write: true,

		Snapshot: &api.SnapshotOptions{
			CreateSnapshot:       true,
			ShouldReplaceRequire: snap_api.CreateShouldReplaceRequire(platform, external, shouldReplaceRequire),
			AbsBasedir:           args.Basedir,
		},

		//
		// Unused
		//

		// only matters when the format setting is iife
		GlobalName: "",

		Sourcemap: 0,

		// Only works with ESM modules
		// https://esbuild.github.io/api/#splitting
		Splitting: false,

		MinifyWhitespace:  false,
		MinifyIdentifiers: false,
		MinifySyntax:      false,

		JSXFactory:  "",
		JSXFragment: "",

		// Temporal Dead Zone related perf tweak (var vs let)
		// https://esbuild.github.io/api/#avoid-tdz
		AvoidTDZ: false,

		// https://esbuild.github.io/api/#charset
		Charset: 0,

		// https://esbuild.github.io/api/#color
		Color: 0,

		// https://esbuild.github.io/api/#error-limit
		ErrorLimit: 0,


		// additional package.json fields to try when resolving a package
		// https://esbuild.github.io/api/#main-fields
		MainFields: nil,

		// https://esbuild.github.io/api/#out-extension
		OutExtensions: nil,

		// useful in combination with the external file loader
		// https://esbuild.github.io/api/#public-path
		PublicPath: "",

		// /* #__PURE__ */ before a new or call expression means that that
		// expression can be removed
		// https://esbuild.github.io/api/#pure
		Pure: nil,

		// Tweak resolution algorithm used by node via implicit file extensions
		// https://esbuild.github.io/api/#resolve-extensions
		ResolveExtensions: nil,
		Loader:            nil,

		// Use stdin as input instead of a file
		// https://esbuild.github.io/api/#stdin
		Stdin: nil,

		Tsconfig: "",
	})
}
