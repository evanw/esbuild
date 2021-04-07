package snap_api

import (
	"github.com/evanw/esbuild/pkg/api"
	"testing"
)

var entryPoints = []string{"input.js"}

func _TestRunJS(t *testing.T) {
	result := api.Build(api.BuildOptions{
		EntryPoints: entryPoints,
		Outfile:     "output_js.js",

		Platform: api.PlatformNode,
		Bundle:   true,
		Write:    true,
		LogLevel: api.LogLevelInfo,
	})

	if len(result.Errors) > 0 {
		t.FailNow()
	}
}

func _TestRunSnap(t *testing.T) {
	platform := api.PlatformNode
	external := []string{"inherits"}

	result := api.Build(api.BuildOptions{
		// https://esbuild.github.io/api/#log-level
		LogLevel: api.LogLevelInfo,

		// https://esbuild.github.io/api/#target
		Target: api.ES2020,

		// inline any imported dependencies into the file itself
		// https://esbuild.github.io/api/#bundle
		Bundle: true,

		// write out a JSON file with metadata about the build
		// https://esbuild.github.io/api/#metafile
		// TODO(rebase): what option do we use now?
		// Metafile: "meta_snap.json",

		// Applies when multiple entry points are used.
		// https://esbuild.github.io/api/#outdir
		Outdir: "",
		// Applies when one entry point is used.
		// https://esbuild.github.io/api/#outfile
		Outfile:     "output_snap.js",
		EntryPoints: entryPoints,

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
			ShouldReplaceRequire: IsExternalModule(platform, external),
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

		// https://esbuild.github.io/api/#charset
		Charset: 0,

		// https://esbuild.github.io/api/#color
		Color: 0,

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

	if len(result.Errors) > 0 {
		t.FailNow()
	}
}

/*

--- Encountered Issues ---

# Problematic Modules that need to be excluded

## debug

// ../../examples/express-app/node_modules/debug/src/node.js

- examines process keys on module load

## safer-buffer, safe-buffer

// ../../examples/express-app/node_modules/safer-buffer/safer.js
// ../../examples/express-app/node_modules/safe-buffer/index.js

- accesses Buffer on module load to populate it's buffer wrapper

## inherits: wrapper

// ../../examples/express-app/node_modules/inherits/inherits.js

- Resolves `util.inherits` at module level to know what to export.

### http-errors

// ../../examples/express-app/node_modules/send/node_modules/http-errors/index.js

- calls `createHttpErrorConstructor` during module load which accesses `inherits` module

## iconv-lite

// ../../examples/express-app/node_modules/iconv-lite/lib/index.js

- accesses Node.js process at module level to resolve node version `nodeVer`
- might not be a problem if we just return `undefined` when snapshotting
- alternative is to return a fake process object which has a valid Node.js version in
  order to include `iconv-lite/lib/streams.js` in the snapshot

## body-parser, express

// ../../examples/express-app/node_modules/body-parser/index.js
// ../../examples/express-app/node_modules/express/lib/utils.js

- resolve 'depd' to deprecate its exported function,
  which is only a problem if we wanted to exclude it

## express/lib/response

// ../../examples/express-app/node_modules/express/lib/response.js

- accesses core 'http' module via `__get_res__` during module load
## methods

// ../../examples/express-app/node_modules/methods/index.js

- accesses core 'http' module to export methods during module load

## mine

// ../../examples/express-app/node_modules/mime/mime.js

- accesses process and possibly console via `new Mime().define(...)` during module load

## send

// ../../examples/express-app/node_modules/send/index.js

- accesses core 'util' `inherits` as well as 'mime' during module load

# serve-static

// ../../examples/express-app/node_modules/serve-static/index.js

- accesses 'send' during module load

## request

// ../../examples/express-app/node_modules/express/lib/request.js

- accesses core 'http' module via `__get_req__` during module load

# Require Rewrites

Making 'inherits' an external via `External:    []string{"inherits"},`
caused it to not be included in the bundle which possibly is the only solution here.

# Global Rewrites

- some places wrap prototypes, i.e. `Array.prototype.slice` or `Object.prototype.toString`, not
  sure if that is needed, examples:
  // ../../examples/express-app/node_modules/express/lib/router/route.js
  // ../../examples/express-app/node_modules/express/lib/router/index.js

- some places wrap `Object.create`, not sure if necessary, examples:
  // ../../examples/express-app/node_modules/negotiator/index.js

*/
