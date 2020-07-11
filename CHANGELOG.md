# Changelog

## Unreleased

* Allow bundling with stdin as input ([#212](https://github.com/evanw/esbuild/issues/212))

    You can now use `--bundle` without providing any input files and the input will come from stdin instead. Use `--sourcefile=...` to set the name of the input file for error messages and source maps. Dependencies of the input file will be resolved relative to the current working directory.

    ```
    # These two commands are now basically equivalent
    esbuild --bundle example.js
    esbuild --bundle < example.js --sourcefile=example.js
    ```

    This option has also been added to the JavaScript and Go APIs. If needed, you can customize the resolve directory with the `resolveDir` option:

    ```js
    const {outputFiles: [stdout]} = await build({
      stdin: {
        contents: `
          import {version} from './package.json'
          console.log(version as string)
        `,
        sourcefile: 'example.ts',
        resolveDir: __dirname,
        loader: 'ts',
      },
      bundle: true,
      write: false,
    })
    console.log(stdout)
    ```

## 0.6.0

* Output directory may now contain nested directories ([#224](https://github.com/evanw/esbuild/issues/224))

    Note: This is a breaking change if you use multiple entry points from different directories. Output paths may change with this upgrade.

    Previously esbuild would fail to bundle multiple entry points with the same name because all output files were written to the same directory. This can happen if your entry points are in different nested directories like this:

    ```
    src/
     ├─ a/
     │  └─ page.js
     └─ b/
        └─ page.js
    ```

    With this release, esbuild will now generate nested directories in the output directory that mirror the directory structure of the original entry points. This avoids collisions because the output files will now be in separate directories. The directory structure is mirrored relative to the [lowest common ancestor](https://en.wikipedia.org/wiki/Lowest_common_ancestor) among all entry point paths. This is the same behavior as [Parcel](https://github.com/parcel-bundler/parcel) and the TypeScript compiler.

* Silence errors about missing dependencies inside try/catch blocks ([#247](https://github.com/evanw/esbuild/issues/247))

    This release makes it easier to use esbuild with libraries such as [debug](npmjs.com/package/debug) which contain a use of `require()` inside a `try`/`catch` statement for a module that isn't listed in its dependencies. Normally you need to mark the library as `--external` to silence this error. However, calling `require()` and catching errors is a common pattern for conditionally importing an unknown module, so now esbuild automatically treats the missing module as external in these cases.

* TypeScript type definitions for the browser API

    The node-based JavaScript API already ships with TypeScript type checking for the `esbuild` and `esbuild-wasm` packages. However, up until now the browser-based JavaScript API located in `esbuild-wasm/lib/browser` did not have type definitions. This release adds type definitions so you can now import `esbuild-wasm/lib/browser` in TypeScript and get type checking.

* Add chunk imports to metadata file ([#225](https://github.com/evanw/esbuild/issues/225))

    With code splitting, it's sometimes useful to list out the chunks that will be needed by a given entry point. For example, you may want to use that list to insert one `<link rel="modulepreload">` tag for each chunk in your page header. This information is now present in the JSON metadata file that's generated with the `--metafile` flag. Each object in the `outputs` map now has an `imports` array, and each import has a `path`.

## 0.5.26

* Allow disabling non-existent modules with the `browser` package.json field ([#238](https://github.com/evanw/esbuild/issues/238))

    The [browser field](https://github.com/defunctzombie/package-browser-field-spec) in package.json allows you to disable a module (i.e. force it to become empty) by adding an override that points to `false`. Previously esbuild still required it to have an existing absolute path on the file system so that the disabled module could have a consistent identity. Now this is no longer required, so you can disable modules that don't exist on the file system. For example, you can now use this feature to disable the `fs` module.

* Fix a bug with syntax transformation and `super()` calls ([#242](https://github.com/evanw/esbuild/issues/242))

    In certain situations, esbuild accidentally transformed a class constructor such that a call to `super()` that came first in the original code no longer came first in the generated code. This code generation bug has now been fixed. Calls to `super()` that come first are should now stay that way.

## 0.5.25

* Performance improvment for repeated API calls

    Previously every build or transform API call required parsing a new copy of the [esbuild JavaScript runtime code](internal/runtime/runtime.go). This added a constant overhead for every operation. Now the parsing of the runtime code is cached across API calls. The effect on performance depends on the size of the files you're transforming. Transform API calls appear to be >2x faster for small files, around ~10% faster for normal-sized files, and insignificant for large files.

* Add a binary loader

    You can now assign the `binary` loader to a file extension to load all files of that type into a Uint8Array. The data is encoded as a base64 string and decoded into a Uint8Array at run time. The decoder defaults to a custom platform-independent implementation (faster than `atob`) but it switches to using the `Buffer` API with `--platform=node`.

* Add fine-grained `--target` environments ([#231](https://github.com/evanw/esbuild/issues/231))

    You can now configure individual JavaScript environments as targets. The `--target` flag now takes a comma-separated list of values like this: `--target=chrome58,firefox57,safari11,edge16`. Compatibility data was mainly sourced from [this widely-used compatibility table](https://kangax.github.io/compat-table/es2016plus/).

    There is also now an additional `es5` target. Since no transforms to ES5 are implemented yet, its purpose is mainly to prevent ES6 syntax from accidentally being compiled. This target also prevents esbuild from doing some ES6-specific optimizations that would unintentionally change ES5 code into ES6 code.

## 0.5.24

* Smaller code for loaders that generate expressions

    Loaders that generate expressions (`json`, `text`, `base64`, `file`, and `dataurl`) export them using an assignment to `module.exports`. However, that forces the creation of a CommonJS module which adds unnecessary extra code. Now if the file for that loader is only imported using ES6 import statements instead of `require()`, the expression is exported using an `export default` statement instead. This generates smaller code. The bundler still falls back to the old `module.exports` behavior if the file is imported using `require()` instead of an ES6 import statement.

    Example input file:

    ```js
    import txt from './example.txt'
    console.log(txt)
    ```

    Old bundling behavior:

    ```js
    // ...code for __commonJS() and __toModule() omitted...

    // example.txt
    var require_example = __commonJS((exports, module) => {
      module.exports = "This is a text file.";
    });

    // example.ts
    const example = __toModule(require_example());
    console.log(example.default);
    ```

    New bundling behavior:

    ```js
    // example.txt
    var example_default = "This is a text file.";

    // example.ts
    console.log(example_default);
    ```

    In addition, top-level properties of imported JSON files are now converted into individual ES6 exports for better tree shaking. For example, that means you can now import the `version` property from your `package.json` file and the entire JSON file will be removed from the bundle:

    ```js
    import {version} from './package.json'
    console.log(version)
    ```

    The example above will now generate code that looks like this:

    ```js
    // package.json
    var version = "1.0.0";

    // example.ts
    console.log(version);
    ```

## 0.5.23

* Fix `export declare` inside `namespace` in TypeScript ([#227](https://github.com/evanw/esbuild/issues/227))

    The TypeScript parser assumed that ambient declarations (the `declare` keyword) just declared types and did not affect the output. This was an incorrect assumption for exported declarations of local variables inside namespaces. The assignment to `foo` in the example below must be rewritten to an assignment to `ns.foo`:

    ```ts
    namespace ns {
      export declare let foo: number
      foo = 123
    }
    ```

    This should now work correctly.

* Preserve certain statement-level comments ([#221](https://github.com/evanw/esbuild/issues/221))

    Statement-level comments starting with `//!` or `/*!` or containing `@preserve` or `@license` are now preserved in the output. This matches the behavior of other JavaScript tools such as [Terser](https://github.com/terser/terser).

* Higher memory limit for synchronous JavaScript APIs ([#228](https://github.com/evanw/esbuild/issues/228))

    Apparently the synchronous APIs in node's child process module that esbuild relies on will fail with `ENOBUFS` if the output is larger than a certain size. This caused issues with the `write: false` feature from the previous release. The memory limit has been raised to 16mb which should hopefully avoid these crashes. If that limit is still too low, it can be overridden with the `ESBUILD_MAX_BUFFER` environment variable.

## 0.5.22

* JavaScript build API can now avoid writing to the file system ([#139](https://github.com/evanw/esbuild/issues/139) and [#220](https://github.com/evanw/esbuild/issues/220))

    You can now pass `write: false` to the JavaScript build API to avoid writing to the file system. Instead, the returned object will have the `outputFiles` property with an array of output files, each of which has a string `path` property and a Uint8Array `contents` property. This brings the JavaScript API to parity with the Go API, which already had this feature.

* Support `/* @__PURE__ */` annotations for tree shaking

    You can now annotate call expressions and new expressions with a `/* @__PURE__ */` comment, which tells esbuild that the function call is allowed to be removed if the result is not used. This is a convention from other tools (e.g. UglifyJS and Rollup).

    For example, the code below will now be completely removed during bundling if the `fib` variable is never used. The initializer is a function call and esbuild cannot determine that it has no side effects, but the annotation forces esbuild to consider it removable anyway:

    ```js
    let fib = /* @__PURE__ */ (() => {
      let cache = {}
      return function f(n) {
        return cache[n] || (cache[n] =
          n <= 2 ? 1 : f(n - 1) + f(n - 2));
      }
    })()
    ```

* Add `--pure:name` to annotate calls to globals ([#28](https://github.com/evanw/esbuild/issues/28))

    This flag makes calls to the named function behave as if that call was prefixed by `/* @__PURE__ */`. For example, `--pure:console.log` means calls to `console.log()` will behave as if they were calls to `/* @__PURE__ */ console.log()` instead. This means when `--minify` is active, the calls will be removed as long as the return value is unused (any function arguments with side effects will be kept, however).

* Add basic tree shaking of JSX elements

    Automatically-generated calls to the JSX factory function (usually `React.createElement`) are now marked as `/* @__PURE__ */`. This means the construction of a JSX element is now not considered to have side effects. For example, the code below will be completely removed during bundling if the `element` variable is never used:

    ```jsx
    let element = <div>an unused element</div>
    ```

* Fixed a concurrency issue with the JavaScript API

    Before this release, multiple concurrent JavaScript API calls that used different values for the `define` option could end up using the value from another API call. This bug was due to inverted boolean logic in code that was intended to cache the define map only when there were no user-specified defines. The issue has been fixed.

## 0.5.21

* Binaries for FreeBSD ([#217](https://github.com/evanw/esbuild/pull/217))

    There are now esbuild binaries for FreeBSD, both for AMD64 and ARM64. This was contributed by [@kikuchan](https://github.com/kikuchan).

* Remove nested `node_modules` directory

    The install script for the `esbuild` npm package invokes `npm` recursively to install the binary for the current platform. However, the left over nested `node_modules` directory could potentially cause problems with tools that scan for nested `node_modules` directories. Now the install script no longer leaves a nested `node_modules` directory around after finishing.

## 0.5.20

* Allow multiple `.` characters in loader extensions ([#215](https://github.com/evanw/esbuild/issues/215))

    You are now able to configure two loaders such that one is the suffix of the other. For example, you can now configure both `--loader:.txt=text` and `--loader:.base64.txt=base64`. The loader with the longer matching suffix will be used.

* Add support for scoped external packages ([#214](https://github.com/evanw/esbuild/issues/214))

    You can now mark scoped packages as external. For example, `--external:@babel/core` marks the package `@babel/core` as external. This was contributed by [@floydspace](https://github.com/floydspace).

* Add support for external paths ([#127](https://github.com/evanw/esbuild/issues/127) and [#191](https://github.com/evanw/esbuild/issues/191))

    Previously the `--external:M` flag only worked if `M` was a package name. For example, you can mark the `fs` package as external with `--external:fs`.

    With this release, you can now also mark file paths as external using the same syntax. For example, `--external:./index.js` marks the file `index.js` in the current working directory as external. The path to the external module used in the output file will be relative to the output directory.

## 0.5.19

* Fix bug with TypeScript `typeof` operator ([#213](https://github.com/evanw/esbuild/issues/213))

    The TypeScript parser in esbuild incorrectly treated `readonly` in `typeof readonly` as a type operator instead of an identifier, which meant that it expected a type expression to follow the `readonly` identifier. Type expressions containing `typeof readonly` are now parsed correctly.

## 0.5.18

* Fix bug with code splitting and side effects

    This release fixes a bug with experimental code splitting. Chunks with side effects but without any exports were not imported by the entry points that depended on them, which meant that their side effects accidentally did not occur. The fix ensures that all entry points import all relevant chunks regardless of whether or not the chunks have exports, so their side effects should never be omitted.

## 0.5.17

* Pass through `import.meta` syntax ([#208](https://github.com/evanw/esbuild/issues/208))

    The `import.meta` syntax is a way for code in an ES6 module to access metadata about itself. For example, `import.meta.url` in the browser is the URL of the current module.

    It's a new feature that doesn't work in older browsers, so esbuild converts it to a module-local variable to avoid generating code with a syntax error. However, this is only necessary when targeting older browsers or if the output format doesn't support `import.meta`.

    The `import.meta` syntax is now passed through unmodified when the target is `es2020` or newer and the output format is `esm`. This lets you use features such as `import.meta.url` in those situations.

## 0.5.16

* Experimental code splitting with `--splitting` ([#16](https://github.com/evanw/esbuild/issues/16))

    This release includes experimental support for code splitting. Enable it with the `--splitting` flag. This currently only works with the `esm` output format. Support for the `cjs` and `iife` formats will come later. It's being released early so people can try it out and provide feedback.

    When enabled, code splitting does two things:

    * An asynchronous `import('path')` expression will create another chunk that will only be loaded when that expression is evaluated. This is intended to be used for lazily loading additional code. All additional chunks will be written to the directory configured with `outdir`.

        Note that when code splitting is disabled (i.e. the default behavior), an `import('path')` expression behaves similar to `Promise.resolve(require('path'))` and still bundles the imported file into the entry point bundle. No additional chunks are generated in this case.

    * Multiple entry points will cause additional chunks to be created for code that is shared between entry points. Chunks are generated automatically based on simple principles: code should only ever be in one chunk (i.e. no duplication) and no unnecessary code should be loaded (i.e. chunk boundaries are minimal).

        The way this works is by traversing through the module dependency graph and marking which top-level statements are reachable from which entry points. The set of entry points for a given top-level statement determines which chunk that statement is in.

        This is an advanced form of code splitting where even a single file may end up being split into different chunks. This is not something most other bundlers can do at the moment.

    Note that using code splitting with many entry points may generate many chunks for shared code reachable from different combinations of entry points. This should work fine and should still be efficient with HTTP/2. If you want to only let certain entry points share code, you can run esbuild multiple times for different groups of entry points.

    Please try it out and report any issues on [#16](https://github.com/evanw/esbuild/issues/16).

## 0.5.15

* Remove some unnecessary helper functions ([#206](https://github.com/evanw/esbuild/issues/206))

    Certain unnecessary helper functions were sometimes generated when the output format was `esm`. These helper functions should now only be generated when necessary.

* Optimize CommonJS-to-ES6 module conversion

    CommonJS modules that exported raw strings were unnecessarily slow when imported using an ES6 import statement. This scenario should now be much faster.

    The CommonJS-to-ES6 module conversion in esbuild copies properties off the object one-by-one onto a new object. This is the same approach that the TypeScript compiler uses. However, strings have numeric properties 0 to N-1 where N is the length of the string. Copying all of these numeric properties can take a significantly long time for long strings and is almost certainly unhelpful. Now esbuild's CommonJS-to-ES6 module conversion only copies properties if the export is an object.

* Support JSX fields in `tsconfig.json`

    This release adds support for the `jsxFactory` and `jsxFragmentFactory` fields in `tsconfig.json`. Now you do not have to configure JSX again for esbuild if you have already configured it for TypeScript. The `jsxFragmentFactory` field is a [new feature in the upcoming TypeScript 4.0 release](https://devblogs.microsoft.com/typescript/announcing-typescript-4-0-beta/#custom-jsx-factories).

## 0.5.14

* Prevent assignment to ES6 imports ([#202](https://github.com/evanw/esbuild/issues/202))

    ES6 imports are live bindings to other values, sort of like a getter-only property on an object. An assignment to an import identifier should cause a `TypeError` at run time according to the specification. However, when bundling esbuild performs the "scope hoisting" optimization and merges all modules into a single scope. Imports inside the bundle refer to the imported identifiers without any indirection and an assignment will not throw a `TypeError` at run time.

    This release turns assignments to imports into compile-time errors to reject invalid code instead of allowing it to cause this non-conforming behavior. Handling this at compile-time is consistent with other tools such as TypeScript and Rollup.

* Exclude external child paths from the bundle ([#186](https://github.com/evanw/esbuild/pull/186))

    Marking a module as external via `--external:foo` means any imports for the module `foo` will be preserved in the output instead of being traversed by the bundler. This is helpful if the module contains problematic code such as a native node module that can't be bundled.

    However, code often uses child paths to import a file within a module directly such as `import "foo/bar"`. These paths accidentally bypassed the external module check. The fix means all paths under an external module are now also considered external. This was contributed by [@floydspace](https://github.com/floydspace).

## 0.5.13

* Add support for TypeScript labelled tuples

    This is a new TypeScript feature to be released in TypeScript 4. Tuple types can now have labels:

    ```ts
    let foo: [number, number]           // Without labels
    let bar: [min: number, max: number] // With labels
    ```

    These labels are ignored by the TypeScript compiler and are only there to improve readability. You can read more here: https://devblogs.microsoft.com/typescript/announcing-typescript-4-0-beta/.

## 0.5.12

* Fix a JSX whitespace bug ([#195](https://github.com/evanw/esbuild/issues/195))

    Whitespace behavior in JSX has unfortunately been [left out of the JSX specification](https://github.com/facebook/jsx/issues/6), so it's up to each implementation to determine how to handle whitespace characters. Most of the JSX parsers in the ecosystem have converged on similar behavior. When they differ, esbuild follows the behavior of the TypeScript JSX parser.

    This release fixes a bug where esbuild's JSX parser behaved differently than TypeScript. Certain whitespace characters between JSX elements were incorrectly removed. For example, the space in `<a><b/> <c/></a>` must be preserved to match the TypeScript JSX parser. These cases now have test coverage.

## 0.5.11

* Fix a JavaScript API crash on node 10.x

    The current LTS version of node is 12.x, but some people are still running 10.x and want to use esbuild. Before this fix, attempting to use the esbuild JavaScript API with node 10.x would crash with `ReferenceError: TextEncoder is not defined`. The JavaScript API has been changed to not require `TextEncoder` and now works fine with node 10.x.

## 0.5.10

* Transform object rest properties

    This release transforms object rest property bindings such as `let {...x} = y` when the language target is set to `--target=es2017` or earlier.

    If you're using Babel to transform your source code to ES6 for older browsers, this probably means esbuild's JavaScript API could now be a suitable replacement for Babel in your case. The only remaining features that esbuild can't yet transform to ES6 are a few very rarely used features that don't matter for the vast majority of real-world code (`for async` loops and `async` generators).

## 0.5.9

* Add the `--strict:nullish-coalescing` option

    This affects the transform for the `??` nullish coalescing operator. In loose mode (the default), `a ?? b` is transformed to `a != null ? a : b`. This works fine in all cases except when `a` is the special object `document.all`. In strict mode, `a ?? b` is transformed to `a !== null && a !== void 0 ? a : b` which works correctly with `document.all`. Enable `--strict:nullish-coalescing` if you need to use `document.all` with the `??` operator.

* Add the `--strict:class-fields` option

    This affects the transform for instance and static class fields. In loose mode (the default), class field initialization is transformed to a normal assignment. This is what the TypeScript compiler does by default. However, it doesn't follow the JavaScript specification exactly (e.g. it may call setter methods). Either enable `--strict:class-fields` or add `useDefineForClassFields` to your `tsconfig.json` file if you need accurate class field initialization.

Note that you can also just use `--strict` to enable strictness for all transforms instead of using `--strict:...` for each transform.

## 0.5.8

* Transform async functions ([#137](https://github.com/evanw/esbuild/issues/137))

    This release transforms async functions into generator functions for older browsers when the language target is set to `--target=es2016` or below. The transform esbuild uses is similar to the one used by the TypeScript compiler.

## 0.5.7

* Transform private fields and private methods ([#47](https://github.com/evanw/esbuild/issues/47))

    Private names are an access control mechanism for classes. They begin with a `#` and are not accessible outside of the class they are declared in. Support for parsing this syntax was added in esbuild version 0.4.9 but the syntax was passed through unmodified, meaning it didn't work in older browsers.

    This release adds support for transforming private fields and private methods for older browsers that don't support this syntax. This transform uses `WeakMap` and `WeakSet` to preserve the privacy properties of this feature, similar to the corresponding transforms in the Babel and TypeScript compilers.

    This code:

    ```js
    class Counter {
      #count = 1
      get value() { return this.#count }
      increment() { ++this.#count }
    }
    ```

    is transformed into this code when using `--target=es2020`:

    ```js
    var _count;
    class Counter {
      constructor() { _count.set(this, 1); }
      get value() { return __privateGet(this, _count); }
      increment() { __privateSet(this, _count, +__privateGet(this, _count) + 1); }
    }
    _count = new WeakMap();
    ```

    Note that most modern JavaScript engines (V8, JavaScriptCore, and SpiderMonkey but not ChakraCore) may not have good performance characteristics for large `WeakMap` and `WeakSet` objects. Creating many instances of classes with private fields or private methods with this syntax transform active may cause a lot of overhead for the garbage collector. This is because modern engines (other than ChakraCore) store weak values in an actual map object instead of as hidden properties on the keys themselves, and large map objects can cause performance issues with garbage collection. See [this reference](https://github.com/tc39/ecma262/issues/1657#issuecomment-518916579) for more information.

* Fix re-exports when bundling

    This is similar to the fix for re-exports in version 0.5.6 except that it applies when bundling, instead of just when transforming. It needed to be fixed differently because of how cross-file linking works when bundling.

## 0.5.6

* Fix re-export statements ([#190](https://github.com/evanw/esbuild/issues/190))

    The previous release caused a regression due to some behind-the-scenes work for the upcoming code splitting feature. The re-export alias in statements of the form `export { foo as bar } from 'path'` could sometimes incorrectly be renamed to something else, such as `foo` becoming `foo2`. This release fixes the bug.

## 0.5.5

* Implement logical assignment operator transforms

    There are three new logical assignment operators: `??=`, `&&=`, and `||=`. With this release, you can now use them in older browsers by setting `--target` to a language version other than `esnext`. See [the V8 blog post](https://v8.dev/features/logical-assignment) for more information about how they work.

* Fix re-exports of a CommonJS module in `esm` format

    Previously re-exports of an individual identifier from a CommonJS module generated JavaScript that crashed at run-time when using the `esm` output format. This was because esbuild always tries to generate "live" exports for CommonJS modules that always return the current value of the export instead of "dead" bindings that only return the initial value of the export. The bug happened because the ES6 module format doesn't have a way to forward a live binding to a CommonJS module as an ES6 export. The fix is to generate "dead" exports instead, which is the only available option in this edge case.

    These input files:

    ```js
    // entry_point.js
    export {foo} from './cjs-format.js'
    ```

    ```js
    // cjs-format.js
    Object.defineProperty(exports, 'foo', {
      enumerable: true,
      get: () => Math.random(),
    })
    ```

    Now become this output file:

    ```js
    // cjs-format.js
    var require_cjs_format = __commonJS((exports) => {
      Object.defineProperty(exports, "foo", {
        enumerable: true,
        get: () => Math.random()
      });
    });

    // entry_point.js
    const cjs_format = __toModule(require_cjs_format());
    const export_foo = cjs_format.foo; // This is a "dead" re-export
    export {
      export_foo as foo
    };
    ```

## 0.5.4

* Source maps use `/` on Windows ([#188](https://github.com/evanw/esbuild/issues/188))

    Before generated source maps used `\` on Windows, which meant that tools consuming these source maps (e.g. Chrome) didn't recognize these characters as path separators. Now all platforms consistently use `/` as a path separator.

* Prevent input files from being overwritten

    There are now checks in place to avoid input files being accidentally overwritten. This could easily happen with `--bundle --outdir=.` when bundling JavaScript files since the output file name ends up being the same as the entry point name, and is written to the same directory.

## 0.5.3

* Special-case `require` in browserify bundles ([#80](https://github.com/evanw/esbuild/issues/80) and [#90](https://github.com/evanw/esbuild/issues/90))

    [Browserify](http://browserify.org/) generates code containing the expression `typeof require == "function" && require` which then ends up in a lot of npm packages. This expression is problematic because bundling involves statically determining all source files and their dependencies. Using `require` dynamically like this defeats the static analysis. It's also problematic because esbuild replaces `typeof require == "function"` with `true` since `require` is a function at compile-time when bundling. Then `true && require` becomes `require` in the generated code, which crashes at run time.

    Previously esbuild would generate an error for these expressions. Now esbuild replaces `typeof require == "function" && require` with `false` when targeting the browser and `require` when targeting node. This matches the intent of the browserify prelude snippet and allows esbuild to build libraries containing this code without errors or warnings.

* Allow dynamic dependencies ([#113](https://github.com/evanw/esbuild/issues/113))

    Bundling `require()` or `import()` when the argument isn't a string literal is a dynamic dependency. The dependency path relies on dynamic run-time behavior and cannot be statically determined by esbuild at bundle time.

    Dynamic dependencies used to be an error but are now just a warning. Builds containing them now succeed and the generated code contains the `require()` or `import()` expression. This is useful either when the dynamic dependency is intentional or when you know the dynamic dependency won't ever be triggered. Doing this still generates a warning to alert you that some code was excluded from the bundle and because these expressions may still crash at run time if the imported path isn't valid.

## 0.5.2

* Fix a regression with `--define` and identifiers

    The API refactor introduced a regression where using a `--define` flag to replace something with an identifier followed by another `--define` flag unintentionally caused the first `--define` to use the value from the second `--define` for replacement. This regression was caused by a loop that was added around a Go closure, which caused all closures in that loop to close over the same variable. The bug has been fixed.

* Fix interpretation of legacy `-->` single-line HTML comments

    The `-->` sequence starts a single-line comment similar to `//`. This is legacy behavior from [annex B](http://www.ecma-international.org/ecma-262/6.0/#sec-html-like-comments) under the name `SingleLineHTMLCloseComment`. However, `-->` was incorrectly treated as the start of a comment even when it didn't come at the beginning of the line. Now `-->` only starts a comment if there are no tokens before it on that line.

* Allow shadowing of CommonJS variables ([#165](https://github.com/evanw/esbuild/issues/165))

    It's now no longer an error to re-declare `exports`, `module`, or `require` in a module scope. The re-declared symbol will just silently shadow the CommonJS variable with that name. This allows to use a variable called `exports` in an ES6 module, for example.

## 0.5.1

* Go documentation was moved to godoc ([#177](https://github.com/evanw/esbuild/pull/177))

    The Go documentation is now in the source files itself instead of in an external Markdown file. View it online at https://godoc.org/github.com/evanw/esbuild/pkg/api and https://godoc.org/github.com/evanw/esbuild/pkg/cli.

* The browser API now works in a script tag

    The initial release of the browser API required a bundler to use correctly since it was in CommonJS format. This release adds the ability to use the browser API directly in HTML.

    Here's an example using https://unpkg.com/ for simplicity, although you should consider hosting the files yourself:

    ```html
    <script src="https://unpkg.com/esbuild-wasm@0.5.1/lib/browser.js"></script>
    <script>
      (async () => {
        const service = await esbuild.startService({
          wasmURL: 'https://unpkg.com/esbuild-wasm@0.5.1/esbuild.wasm'
        })
        try {
          const ts = 'enum Foo { A, B, C }'
          const { js } = await service.transform(ts, { loader: 'ts' })
          console.log(js)
        } finally {
          service.stop()
        }
      })()
    </script>
    ```

## 0.5.0

* Overhaul public-facing API code

    This is a rewrite of all externally facing API code. It fixes some bugs and inconsistencies, adds some new features, and makes it easier to support various use cases going forward.

    At a high-level, esbuild's API supports two separate operations: "build" and "transform". Building means reading from the file system and writing back to the file system. Transforming takes an input string and generates an output string. You should use the build API if you want to take advantage of esbuild's bundling capability, and you should use the transform API if you want to integrate esbuild as a library inside another tool (e.g. a "minify" plugin). This rewrite ensures the APIs for these two operations are exposed consistently for all ways of interacting with esbuild (both through the CLI and as a library).

    Here are some of the highlights:

    * There is now a public Go API ([#152](https://github.com/evanw/esbuild/issues/152))

        The main API can be found in the [`github.com/evanw/esbuild/pkg/api`](pkg/api/api.go) module. It exposes the exact same features as the JavaScript API. This means you can use esbuild as a JavaScript transformation and bundling library from Go code without having to run esbuild as a child process. There is also the [`github.com/evanw/esbuild/pkg/cli`](pkg/cli/cli.go) module which can be used to wrap the esbuild CLI itself.

    * There are now synchronous JavaScript APIs ([#136](https://github.com/evanw/esbuild/issues/136))

        Sometimes JavaScript source transformations must be synchronous. For example, using esbuild's API to shim `require()` for `.ts` files was previously not possible because esbuild only had an asynchronous transform API.

        This release adds the new `transformSync()` and `buildSync()` synchronous functions to mirror the existing `transform()` and `build()` asynchronous functions. Note that these synchronous calls incur the cost of starting up a new child process each time, so you should only use these instead of `startService()` if you have to (or if you don't care about optimal performance).

    * There is now an experimental browser-based API ([#172](https://github.com/evanw/esbuild/issues/172))

        The `esbuild-wasm` package now has a file called `browser.js` that exposes a `createService()` API which is similar to the esbuild API available in node. You can either import the `esbuild-wasm` package using a bundler that respects the `browser` field in `package.json` or import the `esbuild-wasm/lib/browser.js` file directly.

        This is what esbuild's browser API looks like:

        ```ts
        interface BrowserOptions {
          wasmURL: string
          worker?: boolean
        }

        interface BrowserService {
          transform(input: string, options: TransformOptions): Promise<TransformResult>
          stop(): void
        }

        declare function createService(options: BrowserOptions): Promise<BrowserService>
        ```

        You must provide the URL to the `esbuild-wasm/esbuild.wasm` file in `wasmURL`. The optional `worker` parameter can be set to `false` to load the WebAssembly module in the same thread instead of creating a worker thread. Using a worker thread is recommended because it means transforming will not block the main thread.

        This API is experimental and may be changed in the future depending on the feedback it gets.

    * Error messages now use `sourcefile` ([#131](https://github.com/evanw/esbuild/issues/131))

        Errors from transform API calls now use `sourcefile` as the the original file name if present. Previously the file name in error messages was always `/input.js`.

## 0.4.14

* Do not reorder `"use strict"` after support code ([#173](https://github.com/evanw/esbuild/issues/173))

    Even when not in bundling mode, esbuild sometimes adds automatically-generated support code at the start of the output file. For example, using the `**` operator with `--target=es2015` causes `let __pow = Math.pow` to be inserted at the start of the file. This interfered with `"use strict"` directives, which must come first. Now `"use strict"` directives are written out first before any automatically-generated support code.

* Fix bug with export star pointing to a re-export ([#176](https://github.com/evanw/esbuild/issues/176))

    This fixes a tree shaking bug that involves an `export * from ...` statement pointing to a file with a `export {name} from ...` statement. Now `name` will no longer be incorrectly removed from the bundle.

## 0.4.13

* Fix possible name collision with CommonJS the target ([#174](https://github.com/evanw/esbuild/issues/174))

    A bug meant that the export objects for individual modules with the same filename could in some cases end up reusing the same name in the output file, which then caused a syntax error. This only happened with the `cjs` target. The bug has been fixed.

## 0.4.12

* Support `export * from ...` for CommonJS modules ([#159](https://github.com/evanw/esbuild/issues/159))

    Wildcard re-exports are now supported when the exports come from a CommonJS or external module. Since CommonJS modules are not statically analyzable, this means in these cases the re-exports are evaluated at run time instead of at bundle time. Modules that re-export symbols this way will also be considered CommonJS modules during bundling because their exports are now also not statically analyzable.

* Add 3rd-party library test coverage

    From the esbuild repo, you can now run `make test-extra` to build some 3rd-party libraries (Rollup, Sucrase, and Esprima) with esbuild and run their test suites. This ensures that these libraries will continue to work as esbuild releases new features.

## 0.4.11

* Fix top-level name minification with runtime

    When not bundling, esbuild only minifies top-level names if the file is an ES6 module (as determined by the presence of an ES6 import or export statement). This determination had a bug where a non-module file was considered a module if esbuild automatically generated an import to some internal support code called the "runtime". For example, using the `**` operator with `--target=es2015` generates an import for the `__pow` runtime function. Runtime imports are now ignored for module determination, so an automatically-generated runtime import no longer causes top-level names to be minified.

* Fix class name generation for default exports

    Some changes to name generation for TypeScript decorators caused the generated class name for `export default class` statements to sometimes not match the name used for other references to that class in the same file. This bug has been fixed.

## 0.4.10

* Initial implementation of TypeScript decorators ([#104](https://github.com/evanw/esbuild/issues/104))

    This release contains an initial implementation of the non-standard TypeScript-specific decorator syntax. This syntax transformation is enabled by default in esbuild, so no extra configuration is needed. The TypeScript compiler will need `"experimentalDecorators": true` configured in `tsconfig.json` for type checking to work with TypeScript decorators.

    Here's an example of a method decorator:

    ```ts
    function logged(target, key, descriptor) {
      let method = descriptor.value
      descriptor.value = function(...args) {
        let result = method.apply(this, args)
        let joined = args.map(x => JSON.stringify(x)).join(', ')
        console.log(`${key}(${joined}) => ${JSON.stringify(result)}`)
        return result
      }
    }

    class Example {
      @logged
      method(text: string) {
        return text + '!'
      }
    }

    const x = new Example
    x.method('text')
    ```

    There are four kinds of TypeScript decorators: class, method, parameter, and field decorators. See [the TypeScript decorator documentation](https://www.typescriptlang.org/docs/handbook/decorators.html) for more information. Note that esbuild only implements TypeScript's `experimentalDecorators` setting. It does not implement the `emitDecoratorMetadata` setting because that requires type information.

* Fix order of side effects for computed fields

    When transforming computed class fields, esbuild had a bug where the side effects of the field property names were not evaluated in source code order. The order of side effects now matches the order in the source code.

* Fix private fields in TypeScript

    This fixes a bug with private instance fields in TypeScript where the private field declaration was incorrectly removed during the TypeScript class field transform, which inlines the initializers into the constructor. Now the initializers are still moved to the constructor but the private field declaration is preserved without the initializer.

    Note that since static private fields are not currently supported by the official TypeScript compiler, they are also not supported by esbuild in TypeScript files. They are supported by esbuild in JavaScript files, however.

## 0.4.9

* Initial support for private names ([#47](https://github.com/evanw/esbuild/issues/47))

    Private names are an access control mechanism for classes. They begin with a `#` and are not accessible outside of the class they are declared in. The private name syntax can now be parsed, printed, and minified correctly. Transforming this syntax for older browsers is not supported yet. This is what the syntax looks like:

    ```js
    class Counter {
      #count = 1
      get value() { return this.#count }
      increment() { this.#count++ }
    }
    ```

    You can read more about these features here:

    * https://github.com/tc39/proposal-private-methods
    * https://github.com/tc39/proposal-class-fields
    * https://github.com/tc39/proposal-static-class-features

* Initial support for logical assignment operators

    This adds support for the three new logical assignment operators `||=`, `&&=`, and `??=`, which can now be parsed and passed through to the output. Transforming this syntax for older browsers is not supported yet. You can read more about these operators here: https://github.com/tc39/proposal-logical-assignment.

* Data loaders now set "no side effects"

    Files loaded using the `json`, `text`, `base64`, `dataurl`, and `file` loaders are now removed from the bundle if the files that import them never use the imports. This is the same behavior as the `"sideEffects": false` setting in `package.json`.

## 0.4.8

* Add the `--metafile` flag ([#140](https://github.com/evanw/esbuild/issues/140))

    Pass `--metafile=meta.json` to write metadata about the build to the file `meta.json`. This includes information such as which files are in the bundle, what other files a given file depends on, and how much of each file ended up in the bundle. This is similar to the [stats option in Webpack](https://webpack.js.org/api/stats/).

    The format looks like this:

    ```ts
    interface Metadata {
      inputs: {
        [path: string]: {
          bytes: number
          imports: {
            path: string
          }[]
        }
      }
      outputs: {
        [path: string]: {
          bytes: number
          inputs: {
            [path: string]: {
              bytesInOutput: number
            }
          }
        }
      }
    }
    ```

* Shorten numeric literals ([#122](https://github.com/evanw/esbuild/issues/122))

    Certain numeric literals now use shorter representations in the generated JavaScript code. For example, `123400000` is now written out as `1234e5`.

## 0.4.7

* Fixed `sideEffects` and nested directories

    This fixes a bug where `package.json` files with `"sideEffects": false` were not respected for files in nested directories. When this bug occurred, bundles could be bigger than necessary. The `sideEffects` hint is now respected if any parent directory contains the hint instead of just the immediate enclosing directory.

* Fixed `sideEffects` and default exports with side effects

    This fixes a bug with default exports with side effects inside a `"sideEffects": false` context that were imported and used. These exports were incorrectly discarded instead of being retained, which could cause the resulting bundle to crash.

## 0.4.6

* Respect the `sideEffects` field when tree shaking ([#50](https://github.com/evanw/esbuild/issues/50))

    Tree shaking now respects `"sideEffects": false` in `package.json`, which means esbuild now generates smaller bundles with certain libraries such as [lodash-es](https://www.npmjs.com/package/lodash-es). This setting is a [convention from Webpack](https://webpack.js.org/guides/tree-shaking/#mark-the-file-as-side-effect-free). Any files in a package with this setting will not be included in the bundle if they are imported using an ES6 import and then never used.

## 0.4.5

* Fix a crash with more than 8 entry points ([#162](https://github.com/evanw/esbuild/pull/162))

    This bug was due to the wrong index being used for an internal bit set. That caused a crash due to an out-of-bounds array read when esbuild is run with more than 8 entry points. I now have test coverage for large numbers of entry points, so this should not happen again.

* Fix slash characters in file loader ([#164](https://github.com/evanw/esbuild/pull/164))

    This fixes a bug where the base64-encoded hash included in the file name could sometimes contain a `/` character. The fix is to use the base64 character set for URL-encoding, which replaces the `/` character with a `_` character.

## 0.4.4

* Fix optional chaining with TypeScript operators ([#168](https://github.com/evanw/esbuild/issues/168))

    The work on optional chaining in the previous release introduced a regression where the TypeScript infix operators `!` and `<>` incorrectly stopped the propagation of optional chaining. That meant `a?.b!()` and `a?.b<T>()` incorrectly behaved like `(a?.b)()` instead of `a?.b()`. This now has test coverage.

* Add support for the `"paths"` field in `tsconfig.json` ([#60](https://github.com/evanw/esbuild/issues/60) and [#144](https://github.com/evanw/esbuild/issues/144))

    This provides a way of remapping module paths to local file paths. It's relatively powerful because it supports wildcard patterns and multiple fallback locations. See [the documentation in the TypeScript handbook](https://www.typescriptlang.org/docs/handbook/module-resolution.html#path-mapping) for more information about how this feature works. This was contributed by [@viankakrisna](https://github.com/viankakrisna).

* Add the `file` loader ([#14](https://github.com/evanw/esbuild/issues/14) and [#135](https://github.com/evanw/esbuild/pull/135))

    The `file` loader copies the input file to the output folder and exports the path of the file as a string to any modules that import the file. For example, `--loader:.png=file` enables this loader for all imported `.png` files. This was contributed by [@viankakrisna](https://github.com/viankakrisna).

* Add the `--resolve-extensions` flag ([#142](https://github.com/evanw/esbuild/pull/142))

    This lets you override the implicit extensions that are tested when importing a file. It must be a comma-separated list of extensions. For example, setting `--resolve-extensions=.jsx,.js` means `import "./foo"` will check for `./foo` then `./foo.jsx` then `./foo.js` in that order. The behavior corresponds to [the similarly-named feature in Webpack](https://webpack.js.org/configuration/resolve/#resolveextensions). This was contributed by [@viankakrisna](https://github.com/viankakrisna).

## 0.4.3

* Fix bug with optional chaining parentheses ([#156](https://github.com/evanw/esbuild/issues/156))

    One edge case with JavaScript optional chaining syntax is that parentheses stop the chain. So `a?.b.c` will be `undefined` if `a` is nullish but `(a?.b).c` will crash if `a` is nullish.

    This was handled correctly when lowering is enabled (i.e. when the language target is `es2019` or below) but was not handled correctly when lowering is disabled (i.e. when the language target is `es2020` or above). The output for `(a?.b).c` was incorrectly `a?.b.c` instead of `(a?.b).c`, which would no longer crash if `a` is nullish. The fix is to preserve the parentheses in the output.

* Support for the PowerPC 64-bit Little Endian architecture on Linux ([#146](https://github.com/evanw/esbuild/pull/146))

    This was contributed by [@runlevel5](https://github.com/runlevel5).

## 0.4.2

* Bind imports to re-exports ([#149](https://github.com/evanw/esbuild/issues/149))

    This fixes a bug where imports of re-exported symbols were not correctly merged in some cases. This resulted in the generated code referencing symbols that were not declared, resulting in a crash.

## 0.4.1

* Add a log level setting ([#117](https://github.com/evanw/esbuild/issues/117))

    You can now silence esbuild except for errors with `--log-level=error`, or except for errors and warnings with `--log-level=warning`.

* Now `jsconfig.json` is an alternative to `tsconfig.json` ([#132](https://github.com/evanw/esbuild/pull/132))

    The `"baseUrl"` setting in `tsconfig.json`, which lets you avoid `../../` relative import paths, is respected by esbuild. With this change, esbuild will also check for this setting in `jsconfig.json` if no `tsconfig.json` file is found. This is relevant to some projects that use the TypeScript compiler with JavaScript files instead of TypeScript files. You can read more about this feature [here](https://code.visualstudio.com/docs/languages/jsconfig). This was contributed by [@viankakrisna](https://github.com/viankakrisna).

* Chinese translation of documentation ([#129](https://github.com/evanw/esbuild/pull/129))

    Both the readme and the architecture documentation have been translated into Chinese, which is available here: http://docs.breword.com/evanw-esbuild. This was contributed by [@92hackers](https://github.com/92hackers).

* Async generator functions require `--target=es2018`

    This fixes a bug where async generator functions were incorrectly allowed with `--target=es2017`, which is incorrect because the [asynchronous iteration spec](https://github.com/tc39/proposal-async-iteration) is part of ES2018.

## 0.4.0

* Add the `esm` output format ([#48](https://github.com/evanw/esbuild/issues/48))

    It is now possible to generate a bundle in ES6 module format using `--format=esm`. The generated code uses ES6 import and export statements. This is useful for bundling code to be used as a library, for using in a `<script type="module>` tag in the browser, or for using with node's `--experimental-modules` flag. Note that CommonJS entry points bundled with this format will become a single default export, which is the same way node works.

* Preliminary tree shaking support ([#50](https://github.com/evanw/esbuild/issues/50))

    Bundling now performs tree shaking, which is also known as dead code elimination. Every top-level statement is considered to be a separate part of the file, and unused parts without any side effects are not included in the bundle. This only really affects code using ES6 modules, so make sure you use ES6 modules to take advantage of tree shaking.

    This is the initial release of tree shaking which lands the fundamental mechanism behind it. This release does not include the [various annotations used by the community](https://webpack.js.org/guides/tree-shaking/) to indicate side-effect free code (e.g. `"sideEffects": false` and `/*#__PURE__*/`), so esbuild will likely generate somewhat bigger bundles than other bundlers. Support for these annotations will come in future releases.

* Benchmarks have been re-run

    This updates all of the bundlers used in the benchmark to their latest versions. Due to recent performance work, esbuild is now at least 100x faster than all other bundlers. I have also included a single-threaded version of esbuild for comparison since some people were wondering how much of esbuild's performance was due to multithreading.

* Warnings about future syntax are now errors

    This happens when an input file contains newer JavaScript syntax and `--target` is set to an earlier version of JavaScript than the syntax can be transformed to. These most of transforms will be implemented eventually, but for now some are still unimplemented. This was changed from a warning to an error because ignoring these warnings could result in broken code in older browsers, so these messages are more serious than warnings.

* Using bundle-related flags without `--bundle` is now an error

    This leaves the possibility open of using these flags for non-bundle mode in the future. For example, in the future `--format` may also work when not bundling.

## 0.3.9

* Add the `dataurl` loader ([#107](https://github.com/evanw/esbuild/pull/107))

    This loader turns the file into a base64-encoded data URL. The mime type is automatically derived from the file extension, with the file contents used as a fallback. This was contributed by [@viankakrisna](https://github.com/viankakrisna).

* Fix minification bug with external modules ([#134](https://github.com/evanw/esbuild/issues/134))

    When loading a module marked `--external` with `require()`, the resulting code was sometimes incorrectly minified when bundling. This now has test coverage.

## 0.3.8

* Fix an issue that prevented non-inline source maps with the `build()` API ([#130](https://github.com/evanw/esbuild/issues/130))

    The issue happend when invoking `esbuild.build({ sourcemap: true })` and was a regression due to the addition of inline source map support. This now has test coverage.

## 0.3.7

* Add an unsupported build for ARM64 ([#123](https://github.com/evanw/esbuild/issues/123))

    Now you can `npm install esbuild` on a Linux ARM64 machine and it should work. This lets you run esbuild on a Raspberry Pi. Note that this target isn't officially supported because it's not covered by any automated tests. This was contributed by [@violentmagician](https://github.com/violentmagician).

## 0.3.6

* Fix a bug with JSX element contents that end in a multi-byte unicode character ([#124](https://github.com/evanw/esbuild/issues/124))

    Such characters are now preserved instead of being truncated.

## 0.3.5

* Performance improvements

    The parsing phase was failing to saturate all CPUs in many cases because input files were being read on a single goroutine in a blocking fashion. Each file is now read on its own goroutine and the parsing phase now saturates all CPUs.

    With the performance improvements in this release and the previous release, the time to run the JavaScript benchmark has been reduced from 0.54s to 0.4s, which is approximately a 25% performance improvement.

## 0.3.4

* Performance improvements

    The GC is now disabled when running in build-and-exit mode, which is a noticeable speedup. This release also fixes some accidental O(n^2) behavior in the code that renames variables to avoid collisions in non-minify mode. This didn't affect any of esbuild's benchmarks but it did cause issues on certain other artificial test cases.

## 0.3.3

* Support all unicode whitespace ([#116](https://github.com/evanw/esbuild/issues/116))

    The lexer now accepts all unicode characters in the `WS` category as valid whitespace to match the JavaScript standard.

## 0.3.2

* Add some options related to source maps

    There is now a `sourcefile` option to set the input file path for input files without a path. This happens in two cases: either using the `service.transform()` API or passing an input file using stdin.

    This release also adds the `inline` value for the `sourcemap` option which inlines the source map as a base64-encoded data URL in the output file instead of writing the source map to a separate file.

## 0.3.1

* Remove type-only exports from TypeScript ([#110](https://github.com/evanw/esbuild/issues/110))

    This fixes a bug where type-only exports in TypeScript files could in some cases generate an invalid export statement.

## 0.3.0

* Support for stdin/stdout ([#76](https://github.com/evanw/esbuild/issues/76))

    You can now pass esbuild an input file over stdin instead of using a file path. Use the `--loader=jsx` syntax to set the loader instead of using the `--loader:.js=jsx` syntax.

    Now if there is no output file, esbuild will write the output to stdout. Before this, esbuild would try to infer an output file based on the input file name. This is a breaking change so it was released with a minor version bump.
