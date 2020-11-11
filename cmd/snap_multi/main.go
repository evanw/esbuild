package main

import (
	"fmt"
	"github.com/evanw/esbuild/internal/snap_api"
	"github.com/evanw/esbuild/pkg/api"
)

func main() {
	snap_api.SnapCmd(nodeExpress)
}

/*
# Revisit later

## eslint

"supports-color", // tty
"chalk",          // supports-color
"debug",          // tty
"@eslint/eslintrc", // "./node_modules/@eslint/eslintrc/lib/config-array/index.js"

"./node_modules/@eslint/eslintrc/lib/config-array/config-dependency.js", // util.inspect.custom
"./node_modules/@eslint/eslintrc/lib/config-array/override-tester.js",   // util.inspect.custom
"./node_modules/@eslint/eslintrc/lib/config-array/index.js", 			 // exports config-dependency.js

The last one is imported by `@eslint/eslintrc` itself making it impossible to use until we can defer exports
via getters.
Another option is to not import eslint directly, but all of its dependencies that are importable.

Additionally eslint itself has nested destructuring `require` statements, i.e.:

```
const {
    Legacy: {
        ConfigOps: {
            getRuleSeverity
        }
    }
} = require("@eslint/eslintrc");
```

which we'd have to handle if this occurs sufficiently often.
*/

var external = []string{
	// gulp-complexity dependencies
	"readable-stream", // stream
	"through2",        // readable-stream
	"vinyl",           // util.inspect.custom
	"gulp-util",       // vinyl via immediate export of gulp-util/lib/File.js

	// gulp dependencies
	"stream-exhaust",       // stream
	"safe-buffer",          // buffer
	"ordered-read-streams", // readable-stream
	"unique-stream",        // global double assignment rewrite bug
	"pumpify",              // buffer
	"glob-parent",          // os
	"graceful-fs",          // fs
	"append-buffer",        // buffer
	"lazystream",           // readable-stream
	"flush-write-stream",   // buffer
	"snapdragon",           // tty
	"expand-brackets",      // tty
	"upath",                // path

	// yeoman-generator dependencies
	"locate-path", // re-assignment of function parameter we replace incorrectly (same for following 2)
	"vinyl-file",
	"dir-glob",
	"has-flag",          // rewrite results in recursive call
	"supports-color",    // tty
	"chalk",             // supports-color
	"@babel/highlight",  // chalk
	"@babel/code-frame", // @babel/highlight
	"async",             // rewrite results in called function not being initialized yet
	"edition-es5",       // path
	"istextorbinary",    // resolves edition-es5 during module load
	"debug",             // tty
	"shelljs",           // buffer
	"duplexer3",         // stream

}

var deferModules = []string{
	// gulp-complexity dependencies
	"./node_modules/gulp-complexity/reporter.js", // custom multiline comment template

	// inquirer dependencies
	"./node_modules/inquirer/node_modules/chalk/source/index.js", // tty
	"./node_modules/mute-stream/mute.js",                         // stream
	"./node_modules/iconv-lite/lib/index.js",                     // buffer
	"./node_modules/tmp/lib/tmp.js",                              // process interaction
	// dep: rxjs
	// invokes `Promise.resolve()` during module load causing SIGTRAP during `mksnapshot`
	"./node_modules/rxjs/internal/util/Immediate.js",

	// gulp dependencies
	"./node_modules/glob-stream/readable.js",         // readable-stream
	"./node_modules/vinyl-fs/lib/file-operations.js", // readable-stream

	// yeoman-generator dependencies
	"./node_modules/yeoman-generator/lib/util/conflicter.js", // resolves `assert` via `error` module invocation
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

	return api.Build(api.BuildOptions{
		LogLevel:    api.LogLevelInfo,
		Target:      api.ES2020,
		Bundle:      true,
		Metafile:    args.Metafile,
		Outfile:     args.Outfile,
		EntryPoints: []string{args.EntryPoint},

		Platform: platform,
		Engines: []api.Engine{
			{Name: api.EngineNode, Version: "12.4"},
		},

		Format: api.FormatCommonJS,

		External: external,
		Write:    true,

		Snapshot: &api.SnapshotOptions{
			CreateSnapshot:       true,
			ShouldReplaceRequire: snap_api.CreateShouldReplaceRequire(platform, external, shouldReplaceRequire),
			AbsBasedir:           args.Basedir,
		},
	})
}
