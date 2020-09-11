# Changelog

## Unreleased

* Mark output files with a hashbang as executable ([#364](https://github.com/evanw/esbuild/issues/364))

    Output files that start with a hashbang line such as `#!/usr/bin/env node` will now automatically be marked as executable. This lets you run them directly in a Unix-like shell without using the `node` command.

## 0.6.34

* Fix parsing of `type;` statements followed by an identifier in TypeScript ([#377](https://github.com/evanw/esbuild/pull/377))

    The following TypeScript code is now correctly parsed as two separate expression statements instead of one type declaration statement:

    ```ts
    type
    Foo = {}
    ```

    This was contributed by [@rtsao](https://github.com/rtsao).

* Fix `export {Type}` in TypeScript when bundling ([#379](https://github.com/evanw/esbuild/issues/379))

    In TypeScript, `export {Type}` is supposed to be silently removed by the compiler if `Type` does not refer to a value declared locally in the file. Previously this behavior was incompletely implemented. The statement itself was removed but the export record was not, so later stages of the pipeline could sometimes add the export statement back. This release removes the export record as well as the statement so it should stay removed in all cases.

* Forbid exporting non-local symbols in JavaScript

    It is now an error to export an identifier using `export {foo}` if `foo` is not declared locally in the same file. This error matches the error that would happen at run-time if the code were to be evaluated in a JavaScript environment that supports ES6 module syntax. This is only an error in JavaScript. In TypeScript, the missing identifier is silently removed instead since it's assumed to be a type name.

* Handle source maps with out-of-order mappings ([#378](https://github.com/evanw/esbuild/issues/378))

    Almost all tools that generate source maps write out the mappings in increasing order by generated position since the mappings are generated along with the output. However, some tools can apparently generate source maps with out-of-order mappings. It's impossible for generated line numbers to be out of order due to the way the source map format works, but it's possible for generated column numbers to be out of order. This release fixes this issue by sorting the mappings by generated position after parsing if necessary.

## 0.6.33

* Fix precedence of tagged template expressions ([#372](https://github.com/evanw/esbuild/issues/372))

    Previously `` await tag`text` `` and `` new tag`text` `` were incorrectly parsed as `` (await tag)`text` `` and `` (new tag)`text` ``. They are now correctly parsed as `` await (tag`text`) `` and `` new (tag`text`) `` instead.

* Fix invalid syntax when lowering `super` inside `async` to `es2016` or earlier ([#375](https://github.com/evanw/esbuild/issues/375))

    This release fixes a bug where using `super.prop` inside an `async` function with `--target=es2016` or earlier generated code that contained a syntax error. This was because `async` functions are converted to generator functions inside a wrapper function in this case, and `super` is not available inside the wrapper function. The fix is to move the reference to `super` outside of the wrapper function.

* Fix duplicate definition of `module` when targeting CommonJS ([#370](https://github.com/evanw/esbuild/issues/370))

    The bundler didn't properly reserve the identifier `module` when using `--format=cjs`. This meant automatically-generated variables named `module` could potentially not be renamed to avoid collisions with the CommonJS `module` variable. It was possible to get into this situation when importing a module named `module`, such as the [node built-in module by that name](https://nodejs.org/api/module.html). This name is now marked as reserved when bundling to CommonJS, so automatically-generated variables named `module` will now be renamed to `module2` to avoid collisions.

## 0.6.32

* Allow `--format` when bundling is disabled ([#109](https://github.com/evanw/esbuild/issues/109))

    This change means esbuild can be used to convert ES6 import and export syntax to CommonJS syntax. The following code:

    ```js
    import foo from 'foo'
    export const bar = foo
    ```

    will be transformed into the following code with `--format=cjs` (the code for `__export` and `__toModule` was omitted for brevity):

    ```js
    __export(exports, {
      bar: () => bar
    });
    const foo = __toModule(require("foo"));
    const bar = foo.default;
    ```

    This also applies to non-JavaScript loaders too. The following JSON:

    ```json
    {"foo": true, "bar": false}
    ```

    is normally converted to the following code with `--loader=json`:

    ```js
    module.exports = {foo: true, bar: false};
    ```

    but will be transformed into the following code instead with `--loader=json --format=esm`:

    ```js
    var foo = true;
    var bar = false;
    var stdin_default = {foo, bar};
    export {
      bar,
      stdin_default as default,
      foo
    };
    ```

    Note that converting CommonJS `require()` calls to ES6 imports is not currently supported. Code containing a reference to `require` in these situations will generate a warning.

* Change the flag for boolean and string minification ([#371](https://github.com/evanw/esbuild/issues/371))

    Previously setting the `--minify-whitespace` flag shortened `true` and `false` to `!0` and `!1` and shortened string literals containing many newlines by writing them as template literals instead. These shortening operations have been changed to the `--minify-syntax` flag instead. There is no change in behavior for the `--minify` flag because that flag already implies both `--minify-whitespace` and `--minify-syntax`.

* Remove trailing `()` from `new` when minifying

    Now `new Foo()` will be printed as `new Foo` when minifying (as long as it's safe to do so), resulting in slightly shorter minified code.

* Forbid `async` functions when the target is `es5`

    Previously using `async` functions did not cause a compile error when targeting `es5` since if they are unavailable, they are rewritten to use generator functions instead. However, generator functions may also be unsupported. It is now an error to use `async` functions if generator functions are unsupported.

* Fix subtle issue with transforming `async` functions when targeting `es2016` or below

    The TypeScript compiler has a bug where, when the language target is set to `ES2016` or earlier, exceptions thrown during argument evaluation are incorrectly thrown immediately instead of later causing the returned promise to be rejected. Since esbuild replicates TypeScript's `async` function transformation pass, esbuild inherited this same bug. The behavior of esbuild has been changed to match the JavaScript specification.

    Here's an example of code that was affected:

    ```js
    async function test(value = getDefaultValue()) {}
    let promise = test()
    ```

    The call to `test()` here should never throw, even if `getDefaultValue()` throws an exception.

## 0.6.31

* Invalid source maps are no longer an error ([#367](https://github.com/evanw/esbuild/issues/367))

    Previously esbuild would fail the build with an error if it encountered a source map that failed validation according to [the specification](https://sourcemaps.info/spec.html). Now invalid source maps will be validated with an error-tolerant validator that will either silently ignore errors or generate a warning, but will never fail the build.

* Fix various edge cases for conformance tests

    * Hoisted function declarations in nested scopes can now shadow symbols in the enclosing scope without a syntax error:

        ```js
        let foo
        {
          function foo() {}
        }
        ```

    * If statements directly containing function declarations now introduce a nested scope so this code is no longer a syntax error:

        ```js
        let foo
        if (true)
          function foo() {}
        ```

    * Keywords can now be used as export aliases with `export * as` statements:

        ```js
        export * as class from 'path'
        ```

    * It is now a syntax error to use `break` or `continue` in invalid locations:

        ```js
        function foo() { break }
        ```

    * Using `yield` as an identifier outside of a generator function is now allowed:

        ```js
        var yield = null
        ```

    * It is now a syntax error to use `yield` or `await` inside a generator or `async` function if it contains an escape sequence:

        ```js
        async function foo() {
          return \u0061wait;
        }
        ```

    * It is now a syntax error to use an `import()` expression with the `new` operator without parentheses:

        ```js
        new import('path')
        ```

    * Using `let` as an identifier is now allowed:

        ```js
        let = null
        ```

    * It is no longer a compile-time error to assign to an import when not bundling:

        ```js
        import {foo} from 'path'
        foo = null
        ```

        Instead the behavior will be left up to the host environment at run-time, which should cause a run-time error. However, this will still be treated as a compile-time error when bundling because the scope-hoisting optimization that happens during bundling means the host may no longer cause run-time errors.

    * You can now declare a variable named `arguments` inside a function without an error:

        ```js
        function foo() {
          let arguments = null
        }
        ```

    * Comma expressions in the iterable position of for-of loops are now a syntax error:

        ```js
        for (var a of b, c) {
        }
        ```

    * It is now a syntax error to use `||` or `&&` with `??` without parentheses

        ```js
        a ?? b || c   // Syntax error
        a ?? (b || c) // Allowed
        (a ?? b) || c // Allowed
        ```

    * It is now a syntax error to use `arguments` inside a `class` field initializer

        ```js
        class Foo {
          foo = arguments
        }
        ```

    * It is now a syntax error to a strict mode reserved word to name a `class`

        ```js
        class static {}
        ```

## 0.6.30

* Fix optional call of `super` property ([#362](https://github.com/evanw/esbuild/issues/362))

    This fixes a bug where lowering the code `super.foo?.()` was incorrectly transformed to this:

    ```js
    var _a, _b;
    (_b = (_a = super).foo) == null ? void 0 : _b.call(_a);
    ```

    This is invalid code because a bare `super` keyword is not allowed. Now that code is transformed to this instead:

    ```js
    var _a;
    (_a = super.foo) == null ? void 0 : _a.call(this);
    ```

* Add a `--strict:optional-chaining` option

    This affects the transform for the `?.` optional chaining operator. In loose mode (the default), `a?.b` is transformed to `a == null ? void 0 : a.b`. This works fine in all cases except when `a` is the special object `document.all`. In strict mode, `a?.b` is transformed to `a === null || a === void 0 ? void 0 : a.b` which works correctly with `document.all`. Enable `--strict:optional-chaining` if you need to use `document.all` with the `?.` operator.

## 0.6.29

* Add a warning for comparison with `NaN`

    This warning triggers for code such as `x === NaN`. Code that does this is almost certainly a bug because `NaN === NaN` is false in JavaScript.

* Add a warning for duplicate switch case clauses

    This warning detects situations when multiple `case` clauses in the same `switch` statement match on the same expression. This almost certainly indicates a problem with the code. This warning protects against situations like this:

    ```js
    switch (typeof x) {
      case 'object':
        // ...
      case 'function':
        // ...
      case 'boolean':
        // ...
      case 'object':
        // ...
    }
    ```

* Allow getters and setters in ES5 ([#356](https://github.com/evanw/esbuild/issues/356))

    This was an oversight. I incorrectly thought getters and setters were added in ES6, not in ES5. This release allows getter and setter method syntax even when `--target=es5`.

* Fix a Windows-only regression with missing directory errors ([#359](https://github.com/evanw/esbuild/issues/359))

    Various Go file system APIs return `ENOTDIR` for missing file system entries on Windows instead of `ENOENT` like they do on other platforms. This interfered with code added in the previous release that makes unexpected file system errors no longer silent. `ENOTDIR` is usually an unexpected error because it's supposed to happen when the file system entry is present but just unexpectedly a file instead of a directory. This release changes `ENOTDIR` to `ENOENT` in certain cases so that these Windows-only errors are no longer treated as unexpected errors.

* Enforce object accessor argument counts

    According to the JavaScript specification, getter methods must have zero arguments and setter methods must have exactly one argument. This release enforces these rules.

* Validate assignment targets

    Code containing invalid assignments such as `1 = 2` will now be correctly rejected as a syntax error. Previously such code was passed through unmodified and the output file would contain a syntax error (i.e. "garbage in, garbage out").

## 0.6.28

* Avoid running out of file handles when ulimit is low ([#348](https://github.com/evanw/esbuild/issues/348))

    When esbuild uses aggressive concurrency, it can sometimes simultaneously use more file handles than allowed by the system. This can be a problem when the limit is low (e.g. using `ulimit -n 32`). In this release, esbuild now limits itself to using a maximum of 32 file operations simultaneously (in practice this may use up to 64 file handles since some file operations need two handles). This limit was chosen to be low enough to not cause issues with normal ulimit values but high enough to not impact benchmark times.

* Unexpected file system errors are no longer silent ([#348](https://github.com/evanw/esbuild/issues/348))

    All file system errors were previously treated the same; any error meant the file or directory was considered to not exist. This was problematic when the process ran out of available file handles because it meant esbuild could ignore files that do actually exist if file handles are exhausted. Then esbuild could potentially generate a different output instead of failing with an error. Now if esbuild gets into this situation, it should report unexpected file system errors and fail to build instead of continuing to build and potentially producing incorrect output.

* Install script tries `npm install` before a direct download ([#347](https://github.com/evanw/esbuild/issues/347))

    The `esbuild` package has a post-install script that downloads the native binary for the current platform over HTTP. Some people have configured their environments such that HTTP requests to npmjs.org will hang, and configured npm to use a proxy for HTTP requests instead. In this case, esbuild's install script will still work as long as `npm install` works because the HTTP request will eventually time out, at which point the install script will run `npm install` as a fallback. The timeout is of course undesirable.

    This release changes the order of attempted download methods in the install script. Now `npm install` is tried first and directly downloading the file over HTTP will be tried as a fallback. This means installations will be slightly slower since npm is slow, but it should avoid the situation where the install script takes a long time because it's waiting for a HTTP timeout. This should still support the scenarios where there is a HTTP proxy configured, where there is a custom registry configured, and where the `npm` command isn't available.

## 0.6.27

* Add parentheses when calling `require()` inside `new` ([#339](https://github.com/evanw/esbuild/issues/339))

    This release fixes an issue where `new (require('path')).ctor()` became `new require_path().ctor()` after bundling, which caused `require_path()` to be invoked as the constructor instead of `ctor()`. With this fix the code `new (require_path()).ctor()` is generated instead, which correctly invokes `ctor()` as the constructor. This was contributed by [@rtsao](https://github.com/rtsao).

## 0.6.26

* Fix syntax error when minifying and bundling CommonJS to ES5 ([#335](https://github.com/evanw/esbuild/issues/335))

    With the flags `--minify --bundle --target=es5`, esbuild had a bug where the arrow function for the closure used to wrap CommonJS modules was not correctly printed as an ES5 function expression, causing a syntax error. This bug has been fixed.

## 0.6.25

* Avoid the `\v` escape sequence in JSON strings

    Source maps are JSON files, and must obey the [JSON specification](https://www.json.org/). The escape sequence `\v` (for the ASCII control character 11) is valid in JavaScript but not in JSON. Previously esbuild contained a bug where source maps for files containing this ASCII control character were invalid JSON. This release fixes the bug by printing this character as `\u000B` instead.

* Speedup for `esbuild-wasm` when using the command line

    The [esbuild-wasm](https://www.npmjs.com/package/esbuild-wasm) package includes a WebAssembly command-line tool called `esbuild` which functions the same as the native command-line tool called `esbuild` in the [esbuild](https://www.npmjs.com/package/esbuild) package. The difference is that the WebAssembly implementation is around an order of magnitude slower than the native version.

    This release changes the API used to instantiate the WebAssembly module from [WebAssembly.instantiate](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/WebAssembly/instantiate) to [WebAssembly.Module](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/WebAssembly/Module/Module), which reduces end-to-end build time by around 1 second on my development laptop. The WebAssembly version is still much slower than the native version, but now it's a little faster than before.

* Optimize for the [@material-ui/icons](https://www.npmjs.com/package/@material-ui/icons) package

    This package has a directory containing over 11,000 files. Certain optimizations in esbuild that worked fine for common cases severely impacted performance for this edge case. This release changes some aspects of path resolution caching to fix these problems. Build time for a certain benchmark involving this package improved from 1.01s for the previous release to 0.22s for this release. Other benchmark times appear to be unaffected.

## 0.6.24

* Switch from base64 encoding to base32 encoding for file hashes

    Certain output files contain hashes in their name both to prevent collisions and to improve caching. For example, an SVG file named `example.svg` that is loaded using the `file` loader might be copied to a file named `example.T3K5TRK4.svg` in the build directory. The hashes are based on the file's contents so they only change when the file content itself changes.

    The hashes previously used [base64 encoding](https://en.wikipedia.org/wiki/Base64) but I recently realized that since certain file systems (e.g. Windows) are case-insensitive, this could lead to confusing situations where esbuild could theoretically generate two files with different case-sensitive names but with the same case-insensitive name. Hashes now use [base32 encoding](https://en.wikipedia.org/wiki/Base32) which only includes uppercase letters, not lowercase letters, which should avoid this confusing situation.

* Optimize character frequency for better gzip compression

    The character sequence used to generate minified names is now the characters in the input files sorted descending by frequency. Previously it was just the valid identifier characters in alphabetic order. This means minified names are more likely to contain characters found elsewhere in the output file (e.g. in keywords and strings). This is a pretty small win but it was added because it's a consistent win, it's simple to implement, and it's very fast to compute.

* Minor syntax minification improvements

    This release contains these additional rules for syntax minification:

    * `a ? b : b` is minified to `a, b`
    * `a ? a : b` is minified to `a || b`
    * `a ? b : a` is minified to `a && b`
    * `a == void 0` is minified to `a == null`
    * `a && (b && c)` is minified to `a && b && c` (same for `||`)
    * `a ? c : (b, c)` is minified to `(a || b), c`
    * `a ? (b, c) : c` is minified to `(a && b), c`
    * `a ? b || c : c` is minified to `(a && b) || c`
    * `a ? c : b && c` is minified to `(a || b) && c`
    * `a ? b(c) : b(d)` is minified to `b(a ? c : d)`
    * `a ? true : false` is minified to `!!a`
    * `a != null ? a : b` is minified to `a ?? b` if it's supported in the target environment
    * `a ? (b ? c : d) : d` is minified to `(a && b) ? c : d`
    * `a ? b : (c ? b : d)` is minified to `(a || c) ? b : d`
    * `(function foo() {})` is minified to `(function() {})`
    * `typeof a === "string"` is minified to `typeof a == "string"`
    * `if (a) if (b) return c` is minified to `if (a && b) return c`
    * `while (a) if (!b) break;` is minified to `for (; a && b; ) ;`
    * `a === null || a === undefined` is minified to `a == null`

    These improvements cause minified code to be slightly smaller.

## 0.6.23

* Add an error message for a missing `--tsconfig` file ([#330](https://github.com/evanw/esbuild/issues/330))

    The `--tsconfig` flag that was added in version 0.6.1 didn't report an error if the provided file doesn't actually exist. This release makes doing this an error that will fail the build.

* Avoid generating the minified label name `if` ([#332](https://github.com/evanw/esbuild/issues/332))

    The recent minification changes in 0.6.20 introduced a regression where input files containing 333 or more label statements resulted in a label being assigned the minified name `if`, which is a JavaScript keyword. This is the first JavaScript keyword in the minified name sequence that esbuild uses for label names: `a b c ... aa ba ca ...`. The regression has been fixed and there is now test coverage for this case.

## 0.6.22

* The bell character is now escaped

    In most terminals, printing the bell character (ASCII code 7) will trigger a sound. The macOS terminal will also flash the screen if sound is muted. This is annoying, and can happen when dumping the output of esbuild to the terminal if the input contains a bell character. Now esbuild will always escape bell characters in the output to avoid this problem.

* CommonJS modules now export properties of prototype ([#326](https://github.com/evanw/esbuild/issues/326))

    This change is for compatibility with Webpack. You can now assign an object with a custom prototype to `module.exports` and esbuild will consider all enumerable properties on the prototype as exports. This behavior is necessary to correctly bundle the [paper.js](https://github.com/paperjs/paper.js) library, for example.

## 0.6.21

* Upgrade from Go 1.14 to Go 1.15

    This change isn't represented by a commit in the repo, but from now on I will be using Go 1.15 to build the distributed binaries instead of Go 1.14. The [release notes for Go 1.15](https://golang.org/doc/go1.15) mention improvements to binary size:

    > Go 1.15 reduces typical binary sizes by around 5% compared to Go 1.14 by eliminating certain types of GC metadata and more aggressively eliminating unused type metadata.

    Initial testing shows that upgrading Go reduces the esbuild binary size on macOS from 7.4mb to 5.3mb, which is a 30% smaller binary! I assume the binary size savings are similar for other platforms. Run-time performance on the esbuild benchmarks seems consistent with previous releases.

* Lower non-tag template literals to ES5 ([#297](https://github.com/evanw/esbuild/issues/297))

    You can now use non-tag template literals such as `` `abc` `` and `` `a${b}c` `` with `--target=es5` and esbuild will convert them to string addition such as `"abc"` and `"a" + b + "c"` instead of reporting an error.

* Newline normalization in template literals

    This fixes a bug with esbuild that caused carriage-return characters to incorrectly end up in multi-line template literals if the source file used Windows-style line endings (i.e. `\r\n`). The ES6 language specification says that both carriage-return characters and Windows carriage-return line-feed sequences must be converted to line-feed characters instead. With this change, esbuild's parsing of multi-line template literals should no longer be platform-dependent.

* Fix minification bug with variable hoisting

    Hoisted variables that are declared with `var` in a nested scope but hoisted to the top-level scope were incorrectly minified as a nested scope symbol instead of a top-level symbol, which could potentially cause a name collision. This bug has been fixed.

## 0.6.20

* Symbols are now renamed separately per chunk ([#16](https://github.com/evanw/esbuild/issues/16))

    Previously, bundling with code splitting assigned minified names using a single frequency distribution calculated across all chunks. This meant that typical code changes in one chunk would often cause the contents of all chunks to change, which negated some of the benefits of the browser cache.

    Now symbol renaming (both minified and not minified) is done separately per chunk. It was challenging to implement this without making esbuild a lot slower and causing it to use a lot more memory. Symbol renaming has been mostly rewritten to accomplish this and appears to actually usually use a little less memory and run a bit faster than before, even for code splitting builds that generate a lot of chunks. In addition, minified chunks are now slightly smaller because a given minified name can now be reused by multiple chunks.

## 0.6.19

* Reduce memory usage for large builds by 30-40% ([#304](https://github.com/evanw/esbuild/issues/304))

    This release reduces memory usage. These specific percentages are likely only accurate for builds with a large number of files. Memory is reduced by ~30% for all builds by avoiding unnecessary per-file symbol maps, and is reduced by an additional ~10% for builds with source maps by preallocating some large arrays relating to source map output.

* Replace `.js` and `.jsx` with `.ts` or `.tsx` when resolving ([#118](https://github.com/evanw/esbuild/issues/118))

    This adds an import path resolution behavior that's specific to the TypeScript compiler where you can use an import path that ends in `.js` or `.jsx` when the correct import path actually ends in `.ts` or `.tsx` instead. See the discussion here for more historical context: https://github.com/microsoft/TypeScript/issues/4595.

## 0.6.18

* Install script falls back to `npm install` ([#319](https://github.com/evanw/esbuild/issues/319))

    The `esbuild` package has a post-install script that downloads the esbuild binary. However, this will fail if `registry.npmjs.org` (or the configured custom npm registry) is inaccessible.

    This release adds an additional fallback for when the download fails. It tries to use the `npm install` command to download the esbuild binary instead. This handles situations where users have either configured npm with a proxy or have a custom command in their path called `npm`.

## 0.6.17

* Add a download cache to the install script

    This speeds up repeated esbuild installs for the same version by only downloading the binary from npm the first time and then reusing it for subsequent installs. The binary files are cached in these locations, which are the same locations as the Electron install script:

    * Windows: `%USERPROFILE%\AppData\Local\Cache\esbuild\bin`
    * macOS: `~/Library/Caches/esbuild/bin`
    * Other: `~/.cache/esbuild/bin`

    The cache holds a maximum of 5 entries and purges least-recently-used entries above that limit.

* Omit `export default` of local type names ([#316](https://github.com/evanw/esbuild/issues/316))

    Normally the `export default` syntax takes a value expression to export. However, TypeScript has a special case for `export default <identifier>` where the identifier is allowed to be a type expression instead of a value expression. In that case, the type expression should not be emitted in the resulting bundle. This release improves support for this case by omitting the export when the identifier matches a local type name.

## 0.6.16

* Colors for Windows console output

    Console output on Windows now uses color instead of being monochrome. This should make log messages easier to read.

* Parenthesize destructuring assignment in arrow function expressions ([#313](https://github.com/evanw/esbuild/issues/313))

    This fixes a bug where `() => ({} = {})` was incorrectly printed as `() => ({}) = {}`, which is a syntax error. This case is now printed correctly.

## 0.6.15

* Support symlinks with absolute paths in `node_modules` ([#310](https://github.com/evanw/esbuild/issues/310))

    Previously esbuild only supported symlinks with relative paths, not absolute paths. Adding support for absolute paths in symlinks fixes issues with esbuild and [pnpm](https://github.com/pnpm/pnpm) on Windows.

* Preserve leading comments inside `import()` expressions ([#309](https://github.com/evanw/esbuild/issues/309))

    This makes it possible to use esbuild as a faster TypeScript-to-JavaScript frontend for Webpack, which has special [magic comments](https://webpack.js.org/api/module-methods/#magic-comments) inside `import()` expressions that affect Webpack's behavior.

* Fix crash for source files beginning with `\r\n` when using source maps ([#311](https://github.com/evanw/esbuild/issues/311))

    The source map changes in version 0.6.13 introduced a regression that caused source files beginning with `\r\n` to crash esbuild when source map generation was enabled. This was not caught during testing both because not many source files begin with a newline and not many source files have Windows-style line endings in them. This regression has been fixed and Windows-style line endings now have test coverage.

## 0.6.14

* Add support for parsing top-level await ([#253](https://github.com/evanw/esbuild/issues/253))

    It seems appropriate for esbuild to support top-level await syntax now that [node is supporting top-level await syntax by default](https://github.com/nodejs/node/issues/34551) (it's the first widely-used platform to do so). This syntax can now be parsed by esbuild and is always passed through untransformed. It's only allowed when the target is `esnext` because the proposal is still in stage 3. It also cannot be used when bundling. Adding support for top-level await to the bundler is complicated since it causes imports to be asynchronous, which has far-reaching implications. This change is mainly for people using esbuild as a library to transform TypeScript into JavaScript one file at a time.

## 0.6.13

* Exclude non-JavaScript files from source maps ([#304](https://github.com/evanw/esbuild/issues/304))

    Previously all input files were eligible for source map generation, even binary files included using loaders such as `dataurl`. This was not intentional. Doing this doesn't serve a purpose and can massively bloat the resulting source maps. Now all files are excluded except those loaded by the `js`, `jsx`, `ts`, and `tsx` loaders.

* Fix incorrect source maps with code splitting ([#303](https://github.com/evanw/esbuild/issues/303))

    Source maps were completely incorrect when code splitting was enabled for chunk files that imported other chunk files. The source map offsets were not being adjusted past the automatically-generated cross-chunk import statements. This has been fixed.

* Change source map column offsets from bytes to UTF-16 code units

    The [source map specification](https://sourcemaps.info/spec.html) leaves many things unspecified including what column numbers mean. Until now esbuild has been generating byte offsets for column numbers, but Mozilla's popular [source-map](https://github.com/mozilla/source-map) library appears to use UTF-16 code unit counts for column numbers instead. With this release, esbuild now also uses UTF-16 code units for column numbers in source maps. This should help esbuild's compatibility with other tools in the ecosystem.

* Fix a bug with partial source mappings

    The source map specification makes it valid to have mappings that don't actually map to anything. These mappings were never generated by esbuild but they are sometimes present in source maps generated by other tools. There was a bug where the source map line number would be thrown off if one of these mappings was present at the end of a line. This bug has been fixed.

## 0.6.12

* Fix bugs with cross-chunk assignment handling ([#302](https://github.com/evanw/esbuild/issues/302))

    The code splitting process may end up moving the declaration of a file-local variable into a separate chunk from an assignment to that variable. However, it's not possible to assign to a variable in another chunk because assigning to an import is not allowed in ES6. To avoid generating invalid code, esbuild runs an additional pass after code splitting to force all code involved in cross-chunk assignments into the same chunk.

    The logic to do this is quite tricky. For example, moving code between chunks may introduce more cross-chunk assignments that also need to be handled. In this case the bug was caused by not handling complex cases with three or more levels of cross-chunk assignment dependency recursion. These cases now have test coverage and should be handled correctly.

## 0.6.11

* Code splitting chunks now use content hashes ([#16](https://github.com/evanw/esbuild/issues/16))

    Code that is shared between multiple entry points is separated out into "chunk" files when code splitting is enabled. These files are named `chunk.HASH.js` where `HASH` is a string of characters derived from a hash (e.g. `chunk.iJkFSV6U.js`).

    Previously the hash was computed from the paths of all entry points which needed that chunk. This was done because it was a simple way to ensure that each chunk was unique, since each chunk represents shared code from a unique set of entry points. But it meant that changing the contents of the chunk did not cause the chunk name to change.

    Now the hash is computed from the contents of the chunk file instead. This better aligns esbuild with the behavior of other bundlers. If changing the contents of the file always causes the name to change, you can serve these files with a very large `max-age` so the browser knows to never re-request them from your server if they are already cached.

    Note that the names of entry points _do not_ currently contain a hash, so this optimization does not apply to entry points. Do not serve entry point files with a very large `max-age` or the browser may not re-request them even when they are updated. Including a hash in the names of entry point files has not been done in this release because that would be a breaking change. This release is an intermediate step to a state where all output file names contain content hashes.

    The reason why this hasn't been done before now is because this change makes chunk generation more complex. Generating the contents of a chunk involves generating import statements for the other chunks which that chunk depends on. However, if chunk names now include a content hash, chunk generation must wait until the dependency chunks have finished. This more complex behavior has now been implemented.

    Care was taken to still parallelize as much as possible despite parts of the code having to block. Each input file in a chunk is still printed to a string fully in parallel. Waiting was only introduced in the chunk assembly stage where input file strings are joined together. In practice, this change doesn't appear to have slowed down esbuild by a noticeable amount.

* Fix an off-by-one error with source map generation ([#289](https://github.com/evanw/esbuild/issues/289))

    The nested source map support added in version 0.6.5 contained a bug. Input files that were included in the bundle but that didn't themselves contain any generated code caused the source index to shift by one, throwing off the source names of all files after it. This could happen with files consisting only of re-export statements (e.g. `export {name} from 'path'`). This bug has been fixed and this specific scenario now has test coverage.

## 0.6.10

* Revert the binary operator chain change

    It turns out this caused some behavior bugs in the generated code.

## 0.6.9

* Performance optimizations for large file transforms

    There are two main JavaScript APIs: `build()` which operates on the file system and `transform()` which operates on in-memory data. Previously transforming large files using the JavaScript `transform()` API could be significantly slower than just writing the in-memory string to the file system, calling `build()`, and reading the result back from the file system. This is based on performance tests done on macOS 10.15.

    Now esbuild will go through the file system when transforming large files (currently >1mb). This approach is only faster for large files, and can be significantly slower for small files, so small files still keep everything in memory.

* Avoid stack overflow for binary operator chains

    Syntax trees with millions of sequential binary operators nested inside each other can cause the parser to stack overflow because it uses a recursive visitor pattern, so each binary operator added an entry to the call stack. Now code like this no longer triggers a stack overflow because the visitor uses the heap instead of the stack in this case. This is unlikely to matter in real-world code but can show up in certain artificial test cases, especially when `--minify-syntax` is enabled.

* Resolve implicitly-named `tsconfig.json` base files ([#279](https://github.com/evanw/esbuild/issues/279))

    The official TypeScript compiler lets you specify a package path as the `extends` property of a `tsconfig.json` file. The base file is then searched for in the relevant `node_modules` directory. Previously the package path had to end with the name of the base file. Now you can additionally omit the name of the base file if the file name is `tsconfig.json`. This more closely matches the behavior of the official TypeScript compiler.

* Support for 32-bit Windows systems ([#285](https://github.com/evanw/esbuild/issues/285))

    You can now install the esbuild npm package on 32-bit Windows systems.

## 0.6.8

* Attempt to support the taobao.org registry ([#291](https://github.com/evanw/esbuild/issues/291))

    This release attempts to add support for the registry at https://registry.npm.taobao.org, which uses a different URL structure than the official npm registry. Also, the install script will now fall back to the official npm registry if installing with the configured custom registry fails.

## 0.6.7

* Custom registry can now have a path ([#286](https://github.com/evanw/esbuild/issues/286))

    This adds support for custom registries hosted at a path other than `/`. Previously the registry had to be hosted at the domain level, like npm itself.

* Nested source maps use relative paths ([#289](https://github.com/evanw/esbuild/issues/289))

    The original paths in nested source maps are now modified to be relative to the directory containing the source map. This means source maps from packages inside `node_modules` will stay inside `node_modules` in browser developer tools instead of appearing at the root of the virtual file system where they might collide with the original paths of files in other packages.

* Support for 32-bit Linux systems ([#285](https://github.com/evanw/esbuild/issues/285))

    You can now install the esbuild npm package on 32-bit Linux systems.

## 0.6.6

* Fix minification bug with `this` values for function calls ([#282](https://github.com/evanw/esbuild/issues/282))

    Previously `(0, this.fn)()` was incorrectly minified to `this.fn()`, which changes the value of `this` used for the function call. Now syntax like this is preserved during minification.

* Install script now respects the npm registry setting ([#286](https://github.com/evanw/esbuild/issues/286))

    If you have configured npm to use a custom registry using `npm config set registry <url>` or by installing esbuild using `npm install --registry=<url> ...`, this custom registry URL should now be respected by the esbuild install script.

    Specifically, the install script now uses the URL from the `npm_config_registry` environment variable if present instead of the default registry URL `https://registry.npmjs.org/`. Note that the URL must have both a protocol and a host name.

* Fixed ordering between `node_modules` and a force-overridden `tsconfig.json` ([#278](https://github.com/evanw/esbuild/issues/278))

    When the `tsconfig.json` settings have been force-overridden using the new `--tsconfig` flag, the path resolution behavior behaved subtly differently than if esbuild naturally discovers the `tsconfig.json` file without the flag. The difference caused package paths present in a `node_modules` folder to incorrectly take precedence over custom path aliases configured in `tsconfig.json`. The ordering has been corrected such that custom path aliases always take place over `node_modules`.

* Add the `--out-extension` flag for custom output extensions ([#281](https://github.com/evanw/esbuild/issues/281))

    Previously esbuild could only output files ending in `.js`. Now you can override this to another extension by passing something like `--out-extension:.js=.mjs`. This allows generating output files with the node-specific `.cjs` and `.mjs` extensions without having to use a separate command to rename them afterwards.

## 0.6.5

* Fix IIFE wrapper for ES5

    The wrapper for immediately-invoked function expressions is hard-coded to an arrow function and was not updated when the ES5 target was added. This meant that bundling ES5 code would generate a bundle what wasn't ES5-compatible. Doing this now uses a function expression instead.

* Add support for nested source maps ([#211](https://github.com/evanw/esbuild/issues/211))

    Source map comments of the form `//# sourceMappingURL=...` inside input files are now respected. This means you can bundle files with source maps and esbuild will generate a source map that maps all the way back to the original files instead of to the intermediate file with the source map.

## 0.6.4

* Allow extending `tsconfig.json` paths inside packages ([#269](https://github.com/evanw/esbuild/issues/269))

    Previously the `extends` field in `tsconfig.json` only worked with relative paths (paths starting with `./` or `../`). Now this field can also take a package path, which will be resolved by looking for the package in the `node_modules` directory.

* Install script now avoids the `npm` command ([#274](https://github.com/evanw/esbuild/issues/274))

    The install script now downloads the binary directly from npmjs.org instead of using the `npm` command to install the package. This should be more compatible with unusual node environments (e.g. having multiple old copies of npm installed).

* Fix a code splitting bug with re-exported symbols ([#273](https://github.com/evanw/esbuild/issues/273))

    Re-exporting a symbol in an entry point didn't correctly track the cross-chunk dependency, which caused the output file to be missing a required import. This bug has been fixed.

* Fix code splitting if a dynamic entry point is doubled as a normal entry point ([#272](https://github.com/evanw/esbuild/issues/272))

    Using a dynamic `import()` expression automatically adds the imported path as an entry point. However, manually adding the imported path to the bundler entry point list resulted in a build failure. This case is now handled.

* Fix dynamic imports from a parent directory ([#264](https://github.com/evanw/esbuild/issues/264))

    The nested output directory feature interacted badly with the code splitting feature when an entry point contained a dynamic `import()` to a file from a directory that was a parent directory to all entry points. This caused esbuild to generate output paths starting with `../` which stepped outside of the output directory.

    The directory structure of the input files is mirrored in the output directory relative to the [lowest common ancestor](https://en.wikipedia.org/wiki/Lowest_common_ancestor) among all entry point paths. However, code splitting introduces a new entry point for each dynamic import. These additional entry points are not in the original entry point list so they were ignored by the lowest common ancestor algorithm. The fix is to make sure all entry points are included, user-specified and dynamic.

## 0.6.3

* Fix `/* @__PURE__ */` IIFEs at start of statement ([#258](https://github.com/evanw/esbuild/issues/258))

    The introduction of support for `/* @__PURE__ */` comments in an earlier release introduced a bug where parentheses were no longer inserted if a statement started with a function expression that was immediately invoked. This bug has been fixed and parentheses are now inserted correctly.

* Add support for `@jsx` and `@jsxFrag` comments ([#138](https://github.com/evanw/esbuild/issues/138))

    You can now override the JSX factory and fragment values on a per-file basis using comments:

    ```jsx
    // @jsx h
    // @jsxFrag Fragment
    import {h, Fragment} from 'preact'
    console.log(<><a/></>)
    ```

    This now generates the following code:

    ```js
    import {h, Fragment} from "preact";
    console.log(h(Fragment, null, h("a", null)));
    ```

* Add the `Write` option to the Go API

    This brings the Go API to parity with the JavaScript API, and makes certain uses of the `api.Build()` call simpler. You can now specify `Write: true` to have the output files written to the file system during the build instead of having to do that yourself.

## 0.6.2

* Fix code splitting bug with re-export cycles ([#251](https://github.com/evanw/esbuild/issues/251))

    Two files that both re-export each other could cause invalid code to be generated when code splitting is enabled. The specific failure was an export statement without a matching import statement from the shared code chunk. This bug has been fixed.

    Semantically a `export * from 'path'` statement should behave like a `export {name} from 'path'` statement with the export list determined automatically. And likewise `export {name} from 'path'` should behave like `import {name} from 'path'; export {name}`.

    This issue was caused by the re-exported symbols not registering themselves as if they were imported with an import statement. That caused code splitting to fail to generate an import statement when the definition of the symbol ended up in a different chunk than the use of the symbol.

* Fix code splitting bug with missing generated imports

    An ES6 module that doesn't import or export anything but that still uses ES6 module syntax (e.g. `import.meta`) interacted badly with some optimizations and caused invalid code to be generated. This generated an import statement without a matching export statement. The bug has been fixed.

    To improve tree shaking, esbuild automatically converts `import * as ns from 'path'; use(ns.prop)` into `import {prop} from 'path'; use(prop)` at parse time. The parser doesn't yet know anything about `path` because parsing happens in parallel, so this transformation is always performed.

    Later on `path` is determined to be an ES6 module with no exports. This means that there is no symbol to bind `prop` to. Since it was originally a property access on what is now known to be an empty exports object, its value is guaranteed to be undefined. It's no longer a property access so esbuild inlines the undefined value at all uses by replacing `prop` with `void 0`.

    However, code splitting wasn't aware of this and still thought imports needed to be generated for uses of `prop`, even though it doesn't actually exist. That caused invalid and unnecessary import statements to be generated. Now code splitting is aware of this undefined substitution behavior and ignores these symbol uses.

## 0.6.1

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

* Implement `extends` for `tsconfig.json` ([#233](https://github.com/evanw/esbuild/issues/233))

    A `tsconfig.json` file can inherit configurations from another file using the `extends` property. Before this release, esbuild didn't support this property and any inherited settings were missing. Now esbuild should include these inherited settings.

* Allow manually overriding `tsconfig.json` ([#226](https://github.com/evanw/esbuild/issues/226))

    Normally esbuild finds the appropriate `tsconfig.json` file by walking up the directory tree. This release adds the `--tsconfig=...` flag which lets you disable this feature and force esbuild to use the provided configuration file instead. This corresponds to the TypeScript compiler's `--project` flag.

* Remove gaps in source maps within a file ([#249](https://github.com/evanw/esbuild/issues/249))

    The widely-used [source-map](https://github.com/mozilla/source-map) library for parsing source maps [has a bug](https://github.com/mozilla/source-map/issues/261) where it doesn't return mappings from previous lines. This can cause queries within generated code to fail even though there are valid mappings on both sides of the query.

    To work around this issue with the source-map library, esbuild now generates a mapping for every line of code that is generated from an input file. This means that queries with the source-map library should be more robust. For example, you should now be able to query within a multi-line template literal and not have the query fail.

    Note that some lines of code generated during bundling will still not have source mappings. Examples include run-time library code and cross-chunk imports and exports.

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
