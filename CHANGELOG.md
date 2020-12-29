# Changelog

## Unreleased

* Mark `import.meta` as supported in node 10.4+ ([#626](https://github.com/evanw/esbuild/issues/626))

    It was previously marked as unsupported due to a typo in esbuild's compatibility table, which meant esbuild generated a shim for `import.meta` even when it's not necessary. It should now be marked as supported in node 10.4 and above so the shim will no longer be included when using a sufficiently new target environment such as `--target=node10.4`.

* Fix for when the working directory ends with `/` ([#627](https://github.com/evanw/esbuild/issues/627))

    If the working directory ended in `/`, the last path component would be incorrectly duplicated. This was the case when running esbuild with Yarn 2 (but not Yarn 1) and is problematic because some externally-facing directories reference the current working directory in plugins and in output files. The problem has now been fixed and the last path component is no longer duplicated in this case. This fix was contributed by [@remorses](https://github.com/remorses).

## 0.8.26

* Ensure the current working directory remains unique per `startService()` call

    The change in version 0.8.24 to share service instances caused problems for code that calls `process.chdir()` before calling `startService()` to be able to get a service with a different working directory. With this release, calls to `startService()` no longer share the service instance if the working directory was different at the time of creation.

* Consider import references to be side-effect free ([#613](https://github.com/evanw/esbuild/issues/613))

    This change improves tree shaking for code containing top-level references to imported symbols such as the following code:

    ```js
    import {Base} from './base'
    export class Derived extends Base {}
    ```

    Identifier references are considered side-effect free if they are locally-defined, but esbuild special-cases identifier references to imported symbols in its AST (the identifier `Base` in this example). This meant they did not trigger this check and so were not considered locally-defined and therefore side-effect free. That meant that `Derived` in this example would never be tree-shaken.

    The reason for this is that the side-effect determination is made during parsing and during parsing it's not yet known if `./base` is a CommonJS module or not. If it is, then `Base` would be a dynamic run-time property access on `exports.Base` which could hypothetically be a property with a getter that has side effects. Therefore it could be considered incorrect to remove this code due to tree-shaking because there is technically a side effect.

    However, this is a very unlikely edge case and not tree-shaking this code violates developer expectations. So with this release, esbuild will always consider references to imported symbols as being side-effect free. This also aligns with ECMAScript module semantics because with ECMAScript modules, it's impossible to have a user-defined getter for an imported symbol. This means esbuild will now tree-shake unused code in cases like this.

* Warn about calling an import namespace object

    The following code is an invalid use of an import statement:

    ```js
    import * as express from "express"
    express()
    ```

    The `express` symbol here is an import namespace object, not a function, so calling it will fail at run-time. This code should have been written like this instead:

    ```js
    import express from "express"
    express()
    ```

    This comes up because for legacy reasons, the TypeScript compiler defaults to a compilation mode where the `import * as` statement is converted to `const express = require("express")` which means you can actually call `express()` successfully. Doing this is incompatible with standard ECMAScript module environments such as the browser, node, and esbuild because an import namespace object is never a function. The TypeScript compiler has a setting to disable this behavior called `esModuleInterop` and they highly recommend applying it both to new and existing projects to avoid these compatibility problems. See [the TypeScript documentation](https://www.typescriptlang.org/docs/handbook/release-notes/typescript-2-7.html#support-for-import-d-from-cjs-from-commonjs-modules-with---esmoduleinterop) for more information.

    With this release, esbuild will now issue a warning when you do this. The warning indicates that your code will crash when run and that your code should be fixed.

## 0.8.25

* Fix a performance regression from version 0.8.4 specific to Yarn 2

    Code using esbuild's `transformSync` function via Yarn 2 experienced a dramatic slowdown in esbuild version 0.8.4 and above. This version added a wrapper script to fix Yarn 2's incompatibility with binary packages. Some code that tries to avoid unnecessarily calling into the wrapper script contained a bug that caused it to fail, which meant that using `transformSync` with Yarn 2 called into the wrapper script unnecessarily. This launched an extra node process every time the esbuild executable was invoked which can be over 6x slower than just invoking the esbuild executable directly. This release should now invoke the esbuild executable directly without going through the wrapper script, which fixes the performance regression.

* Fix a size regression from version 0.7.9 with certain source maps ([#611](https://github.com/evanw/esbuild/issues/611))

    Version 0.7.9 added a new behavior to esbuild where in certain cases a JavaScript file may be split into multiple pieces during bundling. Pieces of the same input file may potentially end up in multiple discontiguous regions in the output file. This was necessary to fix an import ordering bug with CommonJS modules. However, it had the side effect of duplicating that file's information in the resulting source map. This didn't affect source map correctness but it made source maps unnecessarily large. This release corrects the problem by ensuring that a given file's information is only ever represented once in the corresponding source map.

## 0.8.24

* Share reference-counted service instances internally ([#600](https://github.com/evanw/esbuild/issues/600))

    Now calling `startService()` multiple times will share the underlying esbuild child process as long as the lifetimes of the service objects overlap (i.e. the time from `startService()` to `service.stop()`). This is just an internal change; there is no change to the public API. It should result in a faster implementation that uses less memory if your code calls `startService()` multiple times. Previously each call to `startService()` generated a separate esbuild child process.

* Fix re-exports of a side-effect free CommonJS module ([#605](https://github.com/evanw/esbuild/issues/605))

    This release fixes a regression introduced in version 0.8.19 in which an `import` of an `export {...} from` re-export of a CommonJS module does not include the CommonJS module if it has been marked as `"sideEffect": false` in its `package.json` file. This was the case with the [Ramda](https://ramdajs.com/) library, and was due to an unhandled case in the linker.

* Optionally take binary executable path from environment variable ([#592](https://github.com/evanw/esbuild/issues/592))

    You can now set the `ESBUILD_BINARY_PATH` environment variable to cause the JavaScript API to use a different binary executable path. This is useful if you want to substitute a modified version of the `esbuild` binary that contains some extra debugging information. This feature was contributed by [@remorses](https://github.com/remorses).

## 0.8.23

* Fix non-string objects being passed to `transformSync` ([#596](https://github.com/evanw/esbuild/issues/596))

    The transform function is only supposed to take a string. The type definitions also specify that the input must be a string. However, it happened to convert non-string inputs to a string and some code relied on that behavior. A change in 0.8.22 broke that behavior for `transformSync` specifically for `Uint8Array` objects, which became an array of numbers instead of a string. This release ensures that the conversion to a string is done up front to avoid something unexpected happening in the implementation. Future releases will likely enforce that the input is a string and throw an error otherwise.

* Revert the speedup to `transformSync` and `buildSync` ([#595](https://github.com/evanw/esbuild/issues/595))

    This speedup relies on the `worker_threads` module in node. However, when esbuild is used via `node -r` as in `node -r esbuild-register file.ts`, the worker thread created by esbuild somehow ends up being completely detached from the main thread. This may be a bug in node itself. Regardless, the approach esbuild was using to improve speed doesn't work in all cases so it has been reverted. It's unclear if it's possible to work around this issue. This approach for improving the speed of synchronous APIs may be a dead end.

## 0.8.22

* Escape fewer characters in virtual module paths ([#588](https://github.com/evanw/esbuild/issues/588))

    If a module's path is not in the `file` namespace (i.e. it was created by a plugin), esbuild doesn't assume it's a file system path. The meaning of these paths is entirely up to the plugin. It could be anything including a HTTP URL, a string of code, or randomly-generated characters.

    Currently esbuild generates a file name for these virtual modules using an internal "human-friendly identifier" that can also be used as a valid JavaScript identifier, which is sometimes used to for example derive the name of the default export of a bundled module. But that means virtual module paths which _do_ happen to represent file system paths could cause more characters to be escaped than necessary. For example, esbuild escapes `-` to `_` because `-` is not valid in a JavaScript identifier.

    This release separates the file names derived from virtual module paths from the internal "human-friendly identifier" concept. Characters in the virtual module path that are valid in file paths are no longer escaped.

    In the future the output file name of a virtual module will likely be completely customizable with a plugin, so it will be possible to have different behavior for this if desired. But that isn't possible quite yet.

* Speed up the JavaScript `buildSync` and `transformSync` APIs ([#590](https://github.com/evanw/esbuild/issues/590))

    Previously the `buildSync` and `transformSync` API calls created a new child esbuild process on every call because communicating with a long-lived child process is asynchronous in node. However, there's a trick that can work around this limitation: esbuild can communicate with the long-lived child process from a child thread using node's [`worker_threads`](https://nodejs.org/api/worker_threads.html) module and block the main thread using JavaScript's new [Atomics API](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Atomics/wait). This was a tip from [@cspotcode](https://github.com/cspotcode).

    This approach has now been implemented. A quick benchmark shows that `transformSync` is now **1.5x to 15x faster** than it used to be. The speedup depends on the size of the input (smaller inputs get a bigger speedup). The worker thread and child process should automatically be terminated when there are no more event handlers registered on the main thread, so there is no explicit `stop()` call like there is with a service object.

* Distribute a 32-bit Linux ARM binary executable via npm ([#528](https://github.com/evanw/esbuild/issues/528))

    You should now be able to use npm to install esbuild on a 32-bit Linux ARM device. This lets you run esbuild on a Raspberry Pi. Note that this target isn't officially supported because it's not covered by any automated tests.

## 0.8.21

* On-resolve plugins now apply to entry points ([#546](https://github.com/evanw/esbuild/issues/546))

    Previously entry points were required to already be resolved to valid file system paths. This meant that on-resolve plugins didn't run, which breaks certain workflows. Now entry point paths are resolved using normal import resolution rules.

    To avoid making this a breaking change, there is now special behavior for entry point path resolution. If the entry point path exists relative to the current working directory and the path does not start with `./` or `../`, esbuild will now automatically insert a leading `./` at the start of the path to prevent the path from being interpreted as a `node_modules` package path. This is only done if the file actually exists to avoid introducing `./` for paths with special plugin-specific syntax.

* Enable the build API in the browser ([#527](https://github.com/evanw/esbuild/issues/527))

    Previously you could only use the transform API in the browser, not the build API. You can now use the build API in the browser too. There is currently no in-browser file system so the build API will not do anything by default. Using this API requires you to use plugins to provide your own file system. Instructions for running esbuild in the browser can be found here: https://esbuild.github.io/api/#running-in-the-browser.

* Set the importer to `sourcefile` in on-resolve plugins for stdin

    When the stdin feature is used with on-resolve plugins, the importer for any import paths in stdin is currently always set to `<stdin>`. The `sourcefile` option provides a way to set the file name of stdin but it wasn't carried through to on-resolve plugins due to an oversight. This release changes this behavior so now `sourcefile` is used instead of `<stdin>` if present. In addition, if the stdin resolve directory is also specified the importer will be placed in the `file` namespace similar to a normal file.

## 0.8.20

* Fix an edge case with class body initialization

    When bundling, top-level class statements are rewritten to variable declarations initialized to a class expression. This avoids a severe performance pitfall in Safari when there are a large number of class statements. However, this transformation was done incorrectly if a class contained a static field that references the class name in its own initializer:

    ```js
    class Foo {
      static foo = new Foo
    }
    ```

    In that specific case, the transformed code could crash when run because the class name is not yet initialized when the static field initializer is run. Only JavaScript code was affected. TypeScript code was not affected. This release fixes this bug.

* Remove more types of statements as dead code ([#580](https://github.com/evanw/esbuild/issues/580))

    This change improves dead-code elimination in the case where unused statements follow an unconditional jump, such as a `return`:

    ```js
    if (true) return
    if (something) thisIsDeadCode()
    ```

    These unused statements are removed in more cases than in the previous release. Some statements may still be kept that contain hoisted symbols (`var` and `function` statements) because they could potentially impact the code before the conditional jump.

## 0.8.19

* Handle non-ambiguous multi-path re-exports ([#568](https://github.com/evanw/esbuild/pull/568))

    Wildcard re-exports using the `export * from 'path'` syntax can potentially result in name collisions that cause an export name to be ambiguous. For example, the following code would result in an ambiguous export if both `a.js` and `b.js` export a symbol with the same name:

    ```js
    export * from './a.js'
    export * from './b.js'
    ```

    Ambiguous exports have two consequences. First, any ambiguous names are silently excluded from the set of exported names. If you use an `import * as` wildcard import, the excluded names will not be present. Second, attempting to explicitly import an ambiguous name using an `import {} from` import clause will result in a module instantiation error.

    This release fixes a bug where esbuild could in certain cases consider a name ambiguous when it actually isn't. Specifically this happens with longer chains of mixed wildcard and named re-exports. Here is one such case:

    ```js
    // entry.js
    import {x, y} from './not-ambiguous.js'
    console.log(x, y)
    ```

    ```js
    // /not-ambiguous.js
    export * from './a.js'
    export * from './b.js'
    ```

    ```js
    // /a.js
    export * from './c.js'
    ```

    ```js
    // /b.js
    export {x} from './c.js'
    ```

    ```js
    // /c.js
    export let x = 1, y = 2
    ```

    Previously bundling `entry.js` with esbuild would incorrectly generate an error about an ambiguous `x` export. Now this case builds successfully without an error.

* Omit warnings about non-string paths in `await import()` inside a `try` block ([#574](https://github.com/evanw/esbuild/issues/574))

    Bundling code that uses `require()` or `import()` with a non-string path currently generates a warning, because the target of that import will not be included in the bundle. This is helpful to warn about because other bundlers handle this case differently (e.g. Webpack bundles the entire directory tree and emulates a file system lookup) so existing code may expect the target of the import to be bundled.

    You can avoid the warning with esbuild by surrounding the call to `require()` with a `try` block. The thinking is that if there is a surrounding `try` block, presumably the code is expecting the `require()` call to possibly fail and is prepared to handle the error. However, there is currently no way to avoid the warning for `import()` expressions. This release introduces an analogous behavior for `import()` expressions. You can now avoid the warning with esbuild if you use `await import()` and surround it with a `try` block.

## 0.8.18

* Fix a bug with certain complex optional chains ([#573](https://github.com/evanw/esbuild/issues/573))

    The `?.` optional chaining operator only runs the right side of the operator if the left side is undefined, otherwise it returns undefined. This operator can be applied to both property accesses and function calls, and these can be combined into long chains of operators. These expressions must be transformed to a chain of `?:` operators if the `?.` operator isn't supported in the configured target environment. However, esbuild had a bug where an optional call of an optional property with a further property access afterward didn't preserve the value of `this` for the call. This bug has been fixed.

* Fix a renaming bug with external imports

    There was a possibility of a cross-module name collision while bundling in a certain edge case. Specifically, when multiple files both contained an `import` statement to an external module and then both of those files were imported using `require`. For example:

    ```js
    // index.js
    console.log(require('./a.js'), require('./b.js'))
    ```

    ```js
    // a.js
    export {exists} from 'fs'
    ```

    ```js
    // b.js
    export {exists} from 'fs'
    ```

    In this case the files `a.js` and `b.js` are converted to CommonJS format so they can be imported using `require`:

    ```js
    // a.js
    import {exists} from "fs";
    var require_a = __commonJS((exports) => {
      __export(exports, {
        exists: () => exists
      });
    });

    // b.js
    import {exists} from "fs";
    var require_b = __commonJS((exports) => {
      __export(exports, {
        exists: () => exists
      });
    });

    // index.js
    console.log(require_a(), require_b());
    ```

    However, the `exists` symbol has been duplicated without being renamed. This is will result in a syntax error at run-time. The reason this happens is that the statements in the files `a.js` and `b.js` are placed in a nested scope because they are inside the CommonJS closure. The `import` statements were extracted outside the closure but the symbols they declared were incorrectly not added to the outer scope. This problem has been fixed, and this edge case should no longer result in name collisions.

## 0.8.17

* Get esbuild working on the Apple M1 chip via Rosetta 2 ([#564](https://github.com/evanw/esbuild/pull/564))

    The Go compiler toolchain does not yet support the new Apple M1 chip. Go version 1.15 is currently in a feature freeze period so support will be added in the next version, Go 1.16, which will be [released in February](https://blog.golang.org/11years#TOC_3.).

    This release changes the install script to install the executable for macOS `x64` on macOS `arm64` too. Doing this should still work because of the executable translation layer built into macOS. This change was contributed by [@sod](https://github.com/sod).

## 0.8.16

* Improve TypeScript type definitions ([#559](https://github.com/evanw/esbuild/issues/559))

    The return value of the `build` API has some optional fields that are undefined unless certain arguments are present. That meant you had to use the `!` null assertion operator to avoid a type error if you have the TypeScript `strictNullChecks` setting enabled in your project. This release adds additional type information so that if the relevant arguments are present, the TypeScript compiler can tell that these optional fields on the return value will never be undefined. This change was contributed by [@lukeed](https://github.com/lukeed).

* Omit a warning about `require.main` when targeting CommonJS ([#560](https://github.com/evanw/esbuild/issues/560))

    A common pattern in code that's intended to be run in node is to check if `require.main === module`. That will be true if the current file is being run from the command line but false if the current file is being run because some other code called `require()` on it. Previously esbuild generated a warning about an unexpected use of `require`. Now this warning is no longer generated for `require.main` when the output format is `cjs`.

* Warn about defining `process.env.NODE_ENV` as an identifier ([#466](https://github.com/evanw/esbuild/issues/466))

    The define feature can be used to replace an expression with either a JSON literal or an identifier. Forgetting to put quotes around a string turns it into an identifier, which is a common mistake. This release introduces a warning when you define `process.env.NODE_ENV` as an identifier instead of a string. It's very common to use define to replace `process.env.NODE_ENV` with either `"production"` or `"development"` and sometimes people accidentally replace it with `production` or `development` instead. This is worth warning about because otherwise there would be no indication that something is wrong until the code crashes when run.

* Allow starting a local server at a specific host address ([#563](https://github.com/evanw/esbuild/pull/563))

    By default, esbuild's local HTTP server is only available on the internal loopback address. This is deliberate behavior for security reasons, since the local network environment may not be trusted. However, it can be useful to run the server on a different address when developing with esbuild inside of a virtual machine/docker container or to request development assets from a remote testing device on the same network at a different IP address. With this release, you can now optionally specify the host in addition to the port:

    ```
    esbuild --serve=192.168.0.1:8000
    ```

    ```js
    esbuild.serve({
      host: '192.168.0.1',
      port: 8000,
    }, {
      ...
    })
    ```

    ```go
    server, err := api.Serve(api.ServeOptions{
      Host: "192.168.0.1",
      Port: 8000,
    }, api.BuildOptions{
      ...
    })
    ```

    This change was contributed by [@jamalc](https://github.com/jamalc).

## 0.8.15

* Allow `paths` without `baseUrl` in `tsconfig.json`

    This feature was [recently released in TypeScript 4.1](https://devblogs.microsoft.com/typescript/announcing-typescript-4-1/#paths-without-baseurl). The `paths` feature in `tsconfig.json` allows you to do custom import path rewriting. For example, you can map paths matching `@namespace/*` to the path `./namespace/src/*` relative to the `tsconfig.json` file. Previously using the `paths` feature required you to additionally specify `baseUrl` so that the compiler could know which directory the path aliases were supposed to be relative to.

    However, specifying `baseUrl` has the potentially-problematic side effect of causing all import paths to be looked up relative to the `baseUrl` directory, which could potentially cause package paths to accidentally be redirected to non-package files. Specifying `baseUrl` also causes Visual Studio Code's auto-import feature to generate paths relative to the `baseUrl` directory instead of relative to the directory containing the current file. There is more information about the problems this causes here: https://github.com/microsoft/TypeScript/issues/31869.

    With TypeScript 4.1, you can now omit `baseUrl` when using `paths`. When you do this, it as if you had written `"baseUrl": "."` instead for the purpose of the `paths` feature, but the `baseUrl` value is not actually set and does not affect path resolution. These `tsconfig.json` files are now supported by esbuild.

* Fix evaluation order issue with import cycles and CommonJS-style output formats ([#542](https://github.com/evanw/esbuild/issues/542))

    Previously entry points involved in an import cycle could cause evaluation order issues if the output format was `iife` or `cjs` instead of `esm`. This happened because this edge case was handled by treating the entry point file as a CommonJS file, which extracted the code into a CommonJS wrapper. Here's an example:

    Input files:

    ```js
    // index.js
    import { test } from './lib'
    export function fn() { return 42 }
    if (test() !== 42) throw 'failure'
    ```

    ```js
    // lib.js
    import { fn } from './index'
    export let test = fn
    ```

    Previous output (problematic):

    ```js
    // index.js
    var require_esbuild = __commonJS((exports) => {
      __export(exports, {
        fn: () => fn2
      });
      function fn2() {
        return 42;
      }
      if (test() !== 42)
        throw "failure";
    });

    // lib.js
    var index = __toModule(require_esbuild());
    var test = index.fn;
    module.exports = require_esbuild();
    ```

    This approach changed the evaluation order because the CommonJS wrapper conflates both binding and evaluation. Binding and evaluation need to be separated to correctly handle this edge case. This edge case is now handled by inlining what would have been the contents of the CommonJS wrapper into the entry point location itself.

    Current output (fixed):

    ```js
    // index.js
    __export(exports, {
      fn: () => fn
    });

    // lib.js
    var test = fn;

    // index.js
    function fn() {
      return 42;
    }
    if (test() !== 42)
      throw "failure";
    ```

## 0.8.14

* Fix a concurrency bug caused by an error message change ([#556](https://github.com/evanw/esbuild/issues/556))

    An improvement to the error message for path resolution was introduced in version 0.8.12. It detects when a relative path is being interpreted as a package path because you forgot to start the path with `./`:

    ```
     > src/posts/index.js: error: Could not resolve "PostCreate" (use "./PostCreate" to import "src/posts/PostCreate.js")
        2 │ import PostCreate from 'PostCreate';
          ╵                        ~~~~~~~~~~~~
    ```

    This is implemented by re-running path resolution for package path resolution failures as a relative path instead. Unfortunately, this second path resolution operation wasn't guarded by a mutex and could result in concurrency bugs. This issue only occurs when path resolution fails. It is fixed in this release.

## 0.8.13

* Assigning to a `const` symbol is now an error when bundling

    This change was made because esbuild may need to change a `const` symbol into a non-constant symbol in certain situations. One situation is when the "avoid TDZ" option is enabled. Another situation is some potential upcoming changes to lazily-evaluate certain modules for code splitting purposes. Making this an error gives esbuild the freedom to do these code transformations without potentially causing problems where constants are mutated. This has already been a warning for a while so code that does this should already have been obvious. This warning was made an error in a patch release because the expectation is that no real code relies on this behavior outside of conformance tests.

* Fix for the `--keep-names` option and anonymous lowered classes

    This release fixes an issue where names were not preserved for anonymous classes that contained newer JavaScript syntax when targeting an older version of JavaScript. This was because that causes the class expression to be transformed into a sequence expression, which was then not recognized as a class expression. For example, the class did not have the name `foo` in the code below when the target was set to `es6`:

    ```js
    let foo = class {
      #privateMethod() {}
    }
    ```

    The `name` property of this class object is now `foo`.

* Fix captured class names when class name is re-assigned

    This fixes a corner case with class lowering to better match the JavaScript specification. In JavaScript, the body of a class statement contains an implicit constant symbol with the same name as the symbol of the class statement itself. Lowering certain class features such as private methods means moving them outside the class body, in which case the contents of the private method are no longer within the scope of the constant symbol. This can lead to a behavior change if the class is later re-assigned:

    ```js
    class Foo {
      static test() { return this.#method() }
      static #method() { return Foo }
    }
    let old = Foo
    Foo = class Bar {}
    console.log(old.test() === old) // This should be true
    ```

    Previously this would print `false` when transformed to ES6 by esbuild. This now prints `true`. The current transformed output looks like this:

    ```js
    var _method, method_fn;
    const Foo2 = class {
      static test() {
        return __privateMethod(this, _method, method_fn).call(this);
      }
    };
    let Foo = Foo2;
    _method = new WeakSet();
    method_fn = function() {
      return Foo2;
    };
    _method.add(Foo);
    let old = Foo;
    Foo = class Bar {
    };
    console.log(old.test() === old);
    ```

* The `--allow-tdz` option is now always applied during bundling

    This option turns top-level `let`, `const`, and `class` statements into `var` statements to work around some severe performance issues in the JavaScript run-time environment in Safari. Previously you had to explicitly enable this option. Now this behavior will always happen, and there is no way to turn it off. This means the `--allow-tdz` option is now meaningless and no longer does anything. It will be removed in a future release.

* When bundling and minifying, `const` is now converted into `let`

    This was done because it's semantically equivalent but shorter. It's a valid transformation because assignment to a `const` symbol is now a compile-time error when bundling, so changing `const` to `let` should now not affect run-time behavior.

## 0.8.12

* Added an API for incremental builds ([#21](https://github.com/evanw/esbuild/issues/21))

    There is now an API for incremental builds. This is what using the API looks like from JavaScript:

    ```js
    require('esbuild').build({
      entryPoints: ['app.js'],
      bundle: true,
      outfile: 'out.js',
      incremental: true,
    }).then(result => {
      // The "rebuild" method is present if "incremental" is true. It returns a
      // promise that resolves to the same kind of object that "build" returns.
      // You can call "rebuild" as many times as you like.
      result.rebuild().then(result2 => {
        // Call "dispose" when you're done to free up resources.
        result.rebuild.dispose()
      })
    })
    ```

    Using the API from Go is similar, except there is no need to manually dispose of the rebuild callback:

    ```go
    result := api.Build(api.BuildOptions{
      EntryPoints: []string{"app.js"},
      Bundle: true,
      Outfile: "out.js",
      Incremental: true,
    })
    result2 := result.Rebuild()
    ```

    Incremental builds are more efficient than regular builds because some data is cached and can be reused if the original files haven't changed since the last build. There are currently two forms of caching used by the incremental build API:

    * Files are stored in memory and are not re-read from the file system if the file metadata hasn't changed since the last build. This optimization only applies to file system paths. It does not apply to virtual modules created by plugins.

    * Parsed ASTs are stored in memory and re-parsing the AST is avoided if the file contents haven't changed since the last build. This optimization applies to virtual modules created by plugins in addition to file system modules, as long as the virtual module path remains the same.

    This is just the initial release of the incremental build API. Incremental build times still have room for improvement. Right now esbuild still re-resolves, re-loads, and re-links everything even if none of the input files have changed. Improvements to the incremental build mechanism will be coming in later releases.

* Support for a local file server ([#537](https://github.com/evanw/esbuild/issues/537))

    You can now run esbuild with the `--serve` flag to start a local server that serves the output files over HTTP. This is intended to be used during development. You can point your `<script>` tag to a local server URL and your JavaScript and CSS files will be automatically built by esbuild whenever that URL is accessed. The server defaults to port 8000 but you can customize the port with `--serve=...`.

    There is also an equivalent API for JavaScript:

    ```js
    require('esbuild').serve({
      port: 8000,
    },{
      entryPoints: ['app.js'],
      bundle: true,
      outfile: 'out.js',
    }).then(server => {
      // Call "stop" on the server when you're done
      server.stop()
    })
    ```

    and for Go:

    ```go
    server, err := api.Serve(api.ServeOptions{
      Port: 8000,
    }, api.BuildOptions{
      EntryPoints: []string{"app.js"},
      Bundle:      true,
      Outfile:     "out.js",
    })

    // Call "stop" on the server when you're done
    server.Stop()
    ```

    This is a similar use case to "watch mode" in other tools where something automatically rebuilds your code when a file has changed on disk. The difference is that you don't encounter the problem where you make an edit, switch to your browser, and reload only to load the old files because the rebuild hasn't finished yet. Using a HTTP request instead of a file system access gives the rebuild tool the ability to delay the load until the rebuild operation has finished so your build is always up to date.

* Install to a temporary directory for Windows ([#547](https://github.com/evanw/esbuild/issues/547))

    The install script runs `npm` in a temporary directory to download the correct binary executable for the current architecture. It then removes the temporary directory after the installation. However, removing a directory is sometimes impossible on Windows. To work around this problem, the install script now installs to the system's temporary directory instead of a directory inside the project itself. That way it's not problematic if a directory is left behind by the install script. This change was contributed by [@Djaler](https://github.com/Djaler).

* Fix the public path ending up in the metafile ([#549](https://github.com/evanw/esbuild/issues/549))

    The change in version 0.8.7 to include the public path in import paths of code splitting chunks caused a regression where the public path was also included in the list of chunk imports in the metafile. This was unintentional. Now the public path setting should not affect the metafile contents.

## 0.8.11

* Fix parsing of casts in TypeScript followed by certain tokens

    This aligns esbuild's TypeScript parser with the official TypeScript parser as far as parsing of `as` casts. It's not valid to form an expression after an `as` cast if the next token is a `(`, `[`, `++`, `--`, `?.`, assignment operator, or template literal. Previously esbuild wouldn't generate an error for these expressions. This is normally not a problem because the TypeScript compiler itself would reject the code as invalid. However, if the next token starts on a new line, that new token may be the start of another statement. In that case the code generated by esbuild was different than the code generated by the TypeScript compiler. This difference has been fixed.

* Implement wildcards for external paths ([#406](https://github.com/evanw/esbuild/issues/406))

    You can now use a `*` wildcard character with the `--external` option to mark all files matching a certain pattern as external, which will remove them from the bundle. For example, you can now do `--external:*.png` to remove all `.png` files. When a `*` wildcard character is present in an external path, that pattern will be applied to the original path in the source code instead of to the path after it has been resolved to a real file system path. This lets you match on paths that aren't real file system paths.

* Add a warning about self-assignment

    This release adds a warning for code that assigns an identifier to itself (e.g. `x = x`). This code is likely a mistake since doing this has no effect. This warning is not generated for assignments to global variables, since that can have side effects, and self-assignments with TypeScript casts, since those can be useful for changing the type of a variable in TypeScript. The warning is also not generated for code inside a `node_modules` folder.

## 0.8.10

* Fix parsing of conditional types in TypeScript ([#541](https://github.com/evanw/esbuild/issues/541))

    Conditional types in TypeScript take the form `A extends B ? C : D`. Parsing of conditional types in esbuild was incorrect. The `?` can only follow an `extends` clause but esbuild didn't require the `extends` clause, which potentially led to build failures or miscompilation. The parsing for this syntax has been fixed and should now match the behavior of the TypeScript compiler. This fix was contributed by [@rtsao](https://github.com/rtsao).

* Ignore comments for character frequency analysis ([#543](https://github.com/evanw/esbuild/issues/543))

    Character frequency analysis is used to derive the order of minified names for better gzip compression. The idea is to prefer using the most-used characters in the non-symbol parts of the document (keywords, strings, etc.) over characters that are less-used or absent. This is a very slight win, and is only approximate based on the input text instead of the output text because otherwise it would require minifying twice.

    Right now comments are included in this character frequency histogram. This is not a correctness issue but it does mean that documents with the same code but different comments may be minified to different output files. This release fixes this difference by removing comments from the character frequency histogram.

* Add an option to ignore tree-shaking annotations ([#458](https://github.com/evanw/esbuild/issues/458))

    Tree shaking is the term the JavaScript community uses for dead code elimination, a common compiler optimization that automatically removes unreachable code. Since JavaScript is a dynamic language, identifying unused code is sometimes very difficult for a compiler, so the community has developed certain annotations to help tell compilers what code should be considered unused. Currently there two forms of tree-shaking annotations that esbuild supports: inline `/* @__PURE__ */` comments before function calls and the `sideEffects` field in `package.json`.

    These annotations can be problematic because the compiler depends completely on developers for accuracy and the annotations are occasionally incorrect. The `sideEffects` field is particularly error-prone because by default it causes all files in your package to be considered dead code if no imports are used. If you add a new file containing side effects and forget to update that field, your package will break when people try to bundle it.

    This release adds a new flag `--tree-shaking=ignore-annotations` to allow you to bundle code that contains incorrect tree-shaking annotations with esbuild. An example of such code is [@tensorflow/tfjs](https://github.com/tensorflow/tfjs). Ideally the `--tree-shaking=ignore-annotations` flag is only a temporary workaround. You should report these issues to the maintainer of the package to get them fixed since they will trip up other people too.

* Add support for absolute `baseUrl` paths in `tsconfig.json` files

    Previously esbuild always joined the `baseUrl` path to the end of the current directory path. However, if the `baseUrl` was an absolute path, that would end up including the current directory path twice. This situation could arise internally in certain cases involving multiple `tsconfig.json` files and `extends` fields even if the `tsconfig.json` files themselves didn't have absolute paths. Absolute paths are now not modified and should work correctly.

* Fix crash for modules that do `module.exports = null` ([#532](https://github.com/evanw/esbuild/issues/532))

    The code generated by esbuild would crash at run-time if a module overwrote `module.exports` with null or undefined. This has been fixed and no longer crashes.

## 0.8.9

* Add support for the `mips64le` architecture ([#523](https://github.com/evanw/esbuild/issues/523))

    You should now be able to install esbuild on the `mips64le` architecture. This build target is second-tier as it's not covered by CI, but I tested it in an emulator and it appears to work at the moment.

* Fix for packages with inconsistent side effect markings

    Packages can have multiple entry points in their `package.json` file. Two commonly-used ones are specified using the fields `main` and `module`. Packages can also mark files in the package as not having side effects using the `sideEffects` field. Some packages have one entry point marked as having side effects and the other entry point as not having side effects. This is arguably a problem with the package itself. However, this caused an issue with esbuild's automatic entry point field selection method where it would incorrectly consider both `main` and `module` to not have side effects if one of them was marked as not having side effects. Now `main` and `module` will only be considered to not have side effects if the individual file was marked as not having side effects.

* Warn about `import './file'` when `./file` was marked as having no side effects

    Files in packages containing `"sideEffects": false` in the enclosing `package.json` file are intended to be automatically removed from the bundle if they aren't used. However, code containing `import './file'` is likely trying to import that file for a side effect. This is a conflict of intentions so it seems like a good idea to warn about this. It's likely a configuration error by the author of the package. The warning points to the location in `package.json` that caused this situation.

* Add support for glob-style tests in `sideEffects` arrays

    The `sideEffects` field in `package.json` can optionally contain an array of files that are considered to have side effects. Any file not in that list will be removed if the import isn't used. Webpack supports the `*` and `?` wildcard characters in these file strings. With this release, esbuild supports these wildcard characters too.

## 0.8.8

* Add the `--banner` and `--footer` options ([#482](https://github.com/evanw/esbuild/issues/482))

    You can now use the `--banner` and `--footer` options to insert code before and/or after the code that esbuild generates. This is usually used to insert a banner comment at the top of your bundle. However, you can also use this for other purposes such as wrapping your whole bundle in `--banner='try {'` and `--footer='} catch (e) { reportError(e) }'`. Note that since these strings can contain partial JavaScript syntax, esbuild will not do anything to ensure the result is valid JavaScript syntax. This feature was contributed by [@Gelio](https://github.com/Gelio).

* Be more permissive inside TypeScript `declare` contexts

    These cases are now allowed by esbuild:

    * TypeScript supports a special `global { ... }` block inside `declare module`
    * TypeScript allows arbitrary import and export statements inside `declare module`
    * The TypeScript-specific `export as namespace name;` syntax is now ignored inside `declare module`.
    * A trailing comma after a rest argument is disallowed in JavaScript but is allowed in TypeScript if you use `declare function`

* Log output to stderr has been overhauled

    The formatting is now slightly different. Line numbers are now displayed to the left of the source text and source text is now dimmed to make the log messages themselves stand out more. And log messages now support "notes" which are additional messages with different attached locations.

    Before:

    ```
    example.ts:13:6: error: "test" has already been declared
    class test extends BaseTest {
          ~~~~
    ```

    After:

    ```
     > example.ts: error: "test" has already been declared
        13 │ class test extends BaseTest {
           ╵       ~~~~
          example.ts: note: "test" was originally declared here
         4 │ function test(name: string, callback: () => void) {
           ╵          ~~~~
    ```

## 0.8.7

* `--public-path` now affects code splitting chunk imports ([#524](https://github.com/evanw/esbuild/issues/524))

    The public path setting is a path prefix that bakes in the path where your code is hosted. It can currently be used with the `file` loader to turn the exported URLs into absolute URLs. Previously this path prefix didn't apply to the cross-chunk imports generated by code splitting. This was an oversight. The public path setting now also works for cross-chunk imports in this release.

* Add `exports` for output files in metafile ([#487](https://github.com/evanw/esbuild/issues/487))

    The metafile JSON data now contains a list of export names for all generated output files. This only affects builds that use the `esm` output format. It includes the names of all exports declared using the `export` keyword, including transitive exports that use the `export * from` syntax. If the entry point is in CommonJS format, there will be a single export called `default`.

* Fix values in metafile `inputs` object

    This fixes a regression in the `inputs` object in generated metafile JSON data. Version 0.7.9 introduced the ability for a module to be split into multiple parts to correctly emulate ECMAScript module instantiation order. However, that caused split files to be present in the `inputs` object multiple times, once for each split part. That looked something like this:

    ```json
    "outputs": {
      "out/a.js": {
        "imports": [
          {
            "path": "out/chunk.QXHH4FDI.js"
          }
        ],
        "inputs": {
          "a.js": {
            "bytesInOutput": 21
          },
          "a.js": {
            "bytesInOutput": 0
          }
        },
        "bytes": 120
      }
    }
    ```

    This is problematic because duplicate keys are allowed in JSON and overwrite the previous key. The fix in this release is to accumulate the `bytesInOutput` values for all parts of a file and then only write out the accumulated values at the end.

* Avoid arrow functions when `import()` is converted to `require()` for `es5`

    Setting the target to `es5` is supposed to remove arrow functions, since they are only supported in `es6` and above. However, arrow functions would still be generated if an `import()` expression pointed to an external module and the output format was `iife` or `cjs`. Now these arrow functions are replaced by function expressions instead.

* Convert `import()` to `require()` even if the argument isn't a string literal

    The `import()` syntax is supposed to be converted to `require()` if the target is `cjs` instead of `esm`. However, this was previously only done if the argument was a string literal. This is now done for all `import()` expressions regardless of what the argument looks like.

* Transpose `require(a ? 'b' : 'c')` into `a ? require('b') : require('c')`

    The reverse transformation is sometimes done by JavaScript minifiers such as [Terser](https://github.com/terser/terser) even if the original source code used the form `a ? require('b') : require('c')`. This messes up esbuild's import resolution which needs `require()` to take a single string as an argument. The transformation done here is a simple way to make sure esbuild still works on minified code. This transformation is also performed on `import()` and `require.resolve()`.

## 0.8.6

* Changes to TypeScript's `import name =` syntax

    The parsing of TypeScript's `import name =` syntax should now match the official TypeScript parser. Previously esbuild incorrectly allowed any kind of expression after the equals sign. Now you can only use either a sequence of identifiers separated by periods or a call to the `require` function with a string literal.

* Do not report warnings about `require()` inside `try` ([#512](https://github.com/evanw/esbuild/issues/512))

    This release no longer reports warnings about un-bundled calls to `require()` if they are within a `try` block statement. Presumably the try/catch statement is there to handle the potential run-time error from the unbundled `require()` call failing, so the potential failure is expected and not worth warning about.

* Add the `--keep-names` option ([#510](https://github.com/evanw/esbuild/issues/510))

    In JavaScript the `name` property on functions and classes defaults to a nearby identifier in the source code. These syntax forms all set the `name` property of the function to `'fn'`:

    ```js
    function fn() {}
    let fn = function() {};
    obj.fn = function() {};
    fn = function() {};
    let [fn = function() {}] = [];
    let {fn = function() {}} = {};
    [fn = function() {}] = [];
    ({fn = function() {}} = {});
    ```

    However, minification renames symbols to reduce code size. That changes value of the `name` property for many of these cases. This is usually fine because the `name` property is normally only used for debugging. However, some frameworks rely on the `name` property for registration and binding purposes. If this is the case, you can now enable `--keep-names` to preserve the original `name` values even in minified code.

* Omit unused TypeScript import assignment aliases ([#474](https://github.com/evanw/esbuild/issues/474))

    In TypeScript, `import x = y` is an alias statement that works for both values and types and can reach across files. Because esbuild doesn't replicate TypeScript's type system and because esbuild converts each file from TypeScript to JavaScript independently, it's not clear to esbuild if the alias refers to a value and should be kept as JavaScript or if the alias refers to a type and should be removed.

    Previously all import aliases were kept in the generated JavaScript. This could lead to problems if the alias actually referred to a type. Now import aliases are only kept if they are used as values. This way import aliases that are only used as types will be automatically removed. This doesn't exactly match what the TypeScript compiler does in complex scenarios but it should work for many real-world cases.

* Validate that on-resolve plugins return absolute paths in the `file` namespace

    The default path namespace for on-resolve plugins is the `file` namespace. Paths in this namespace are expected to be absolute paths. This is now enforced. If the returned path is not supposed to be a file system path, you should set a namespace other than `file` so esbuild doesn't treat it as a file system path.

* External paths returned by a plugin do not default to the `file` namespace

    The `file` namespace is normally implied if it's not specified. However, that probably does not match the intent of the plugin for paths that have been marked as external. Such paths will now have an empty namespace instead of the namespace `file`. You now have to explicitly specify the `file` namespace in your plugin if you want it for external paths.

## 0.8.5

* Direct `eval()` now causes the module to be considered CommonJS ([#175](https://github.com/evanw/esbuild/pull/175))

    Code containing a direct call to `eval()` can potentially access any name in the current scope or in any parent scope. Therefore all symbols in all of these scopes must not be renamed or minified. This was already the case for all non-top-level symbols, but it accidentally wasn't the case for top-level symbols.

    Preventing top-level symbols from being renamed is problematic because they may be merged in with symbols from other files due to the scope hoisting optimization that applies to files in the ECMAScript module format. That could potentially cause the names to collide and cause a syntax error if they aren't renamed. This problem is now avoided by treating files containing direct `eval()` as CommonJS modules instead, which causes these files to each be wrapped in their own closure with a separate scope.

    Note that this change means that tree shaking is disabled for these files. There is rarely a reason to use direct `eval()` and it is almost always a mistake. You likely want to use a form of indirect eval such as `(0, eval)('code')` instead. That also has the benefit of not disabling symbol minification for that file.

* Add a `text` property to output files in build results ([#496](https://github.com/evanw/esbuild/issues/496))

    If you pass `write: false` to the JavaScript `build` API, the output files that would have been written to the file system are instead returned as an array of objects. Each object has a `Uint8Array` property called `contents` with the bytes of the file. It does not contain a string because the bytes of the file may not be valid UTF-8 (e.g. a PNG image) and it's not safe to decode output files as UTF-8 text in all cases.

    This release adds a convenience property called `text` that lazily evaluates and returns `new TextDecoder().decode(contents)` the first time it's accessed. You should only use this in cases where you are sure the contents of the file are encoded using UTF-8 encoding. Invalid code point sequences will be replaced by the U+FFFD replacement character.

## 0.8.4

* Using `delete` on an import namespace object is now an error

    This release makes the following code forbidden when bundling is active:

    ```js
    import * as ns from './some-file';
    delete ns.prop;
    ```

    Doing this does not delete the property because properties on ECMAScript module objects are not mutable. Assigning to a property of an import namespace object is already an error and not including the `delete` operator as an assignment was an oversight. This release just makes `delete` assignment consistent with other forms of assignment.

* Mark dead code inside branching expressions

    Code inside branching expressions where the branch is statically determined to never be taken is now marked as dead code. Previously this was only the case for statements, not expressions. This change means `false && require('pkg')` will no longer generate an error about `pkg` being missing even if it is indeed missing. This change affects the `||`, `&&`, `??`, and `?:` operators.

* Fix metafile when importing CSS from JS ([#504](https://github.com/evanw/esbuild/pull/504))

    This release fixes a bug where importing a CSS file from JavaScript caused esbuild to generate invalid JSON in the resulting metafile. It was only a problem if you were importing CSS from JS and enabled metafile output. This fix was contributed by [@nitsky](https://github.com/nitsky).

* Fix downloads for Yarn 2 ([#505](https://github.com/evanw/esbuild/pull/505))

    The change related to Yarn 2 in the previous release had a bug that prevented downloads from succeeding when installing esbuild with Yarn 2. This fix was contributed by [@mathieudutour](https://github.com/mathieudutour).

## 0.8.3

* Fix name collision with TypeScript namespaces containing their own name

    This fixes a bug where TypeScript namespaces containing a declaration that re-uses the name of the enclosing namespace incorrectly failed the build with a duplicate declaration error. Here is an example:

    ```ts
    namespace foo {
      export let foo
    }
    ```

    This happened because esbuild compiles that code into something like this:

    ```ts
    var foo;
    (function (foo) {
      foo.foo = 123;
      console.log(foo.foo);
    })(foo || (foo = {}));
    ```

    The exported name `foo` was colliding with the automatically-declared function argument also named `foo`, which normally must be declared in that scope to shadow the outer namespace variable. This release fixes the problem by not declaring the function argument in the scope if there is already a declaration with that name in that scope.

* Prefer `.css` files for `@import` in CSS

    People sometimes create a `.js`-related file and an adjacent `.css` file with the same name when creating a component (e.g. `button.tsx` and `button.css`). They also sometimes use `@import "./button"` in CSS and omit the file extension. Currently esbuild uses a single global order of extensions to try when an extension is omitted. This is configured with `--resolve-extensions` and defaults to `.tsx, .ts, .jsx, .mjs, .cjs, .js, .css, .json`. This means the `.tsx` file will be matched because `.tsx` comes before `.css` in the order.

    This release changes the behavior to use a different order of extensions for `@import` statements in CSS files. The order is the list given by `--resolve-extensions` with all extensions removed that have `.js`-related loaders configured. In this case the filtered list would just be `.css` since all other default resolve extensions have JavaScript loaders, but if you also configure another resolve extension to use the `css` loader that will also qualify for implicit extension support with `@import` statements in CSS.

* Add support for `paths` in `tsconfig.json` for absolute paths

    Previously it wasn't possible to use `paths` in `tsconfig.json` to remap paths starting with `/` on systems that considered that an absolute path (so not Windows). This is because absolute paths are handled before normal path resolution logic. Now this should work correctly.

* Hack around lack of support for binary packages in Yarn 2 ([#467](https://github.com/evanw/esbuild/issues/467))

    The Yarn 2 package manager is deliberately incompatible with binary modules because the Yarn 2 developers don't think they should be used. See [yarnpkg/berry#882](https://github.com/yarnpkg/berry/issues/882) for details. This means running esbuild with Yarn 2 currently doesn't work (Yarn 2 tries to load the esbuild binary as a JavaScript file).

    The suggested workaround from the Yarn 2 team is to replace the binary with a JavaScript file wrapper that invokes the esbuild binary using node's `child_process` module. However, doing that would slow down esbuild for everyone. The `esbuild` command that is exported from the main package is intentionally a native executable instead of a JavaScript wrapper script because starting up a new node process just to invoke a native binary is unnecessary additional overhead.

    The hack added in this release is to detect whether esbuild is being installed with Yarn 2 during the install script and only install a JavaScript file wrapper for Yarn 2 users. Doing this should make it possible to run the esbuild command from Yarn 2 without slowing down esbuild for everyone. This change was contributed by [@rtsao](https://github.com/rtsao).

## 0.8.2

* Fix the omission of `outbase` in the JavaScript API ([#471](https://github.com/evanw/esbuild/pull/471))

    The original PR for the `outbase` setting added it to the CLI and Go APIs but not the JavaScript API. This release adds it to the JavaScript API too.

* Fix the TypeScript type definitions ([#499](https://github.com/evanw/esbuild/pull/499))

    The newly-released `plugins` option in the TypeScript type definitions was incorrectly marked as non-optional. It is now optional. This fix was contributed by [@remorses](https://github.com/remorses).

## 0.8.1

* The initial version of the plugin API ([#111](https://github.com/evanw/esbuild/pull/111))

    The plugin API lets you inject custom code inside esbuild's build process. You can write plugins in either JavaScript or Go. Right now you can add an "on resolve" callback to determine where import paths go and an "on load" callback to determine what the imported file contains. These two primitives are very powerful, especially in combination with each other.

    Here's a simple example plugin to show off the API in action. Let's say you wanted to enable a workflow where you can import environment variables like this:

    ```js
    // app.js
    import { NODE_ENV } from 'env'
    console.log(`NODE_ENV is ${NODE_ENV}`)
    ```

    This is how you might do that from JavaScript:

    ```js
    let envPlugin = {
      name: 'env-plugin',
      setup(build) {
        build.onResolve({ filter: /^env$/ }, args => ({
          path: args.path,
          namespace: 'env',
        }))

        build.onLoad({ filter: /.*/, namespace: 'env' }, () => ({
          contents: JSON.stringify(process.env),
          loader: 'json',
        }))
      },
    }

    require('esbuild').build({
      entryPoints: ['app.js'],
      bundle: true,
      outfile: 'out.js',
      plugins: [envPlugin],
      logLevel: 'info',
    }).catch(() => process.exit(1))
    ```

    This is how you might do that from Go:

    ```go
    package main

    import (
      "encoding/json"
      "os"
      "strings"

      "github.com/evanw/esbuild/pkg/api"
    )

    var envPlugin = api.Plugin{
      Name: "env-plugin",
      Setup: func(build api.PluginBuild) {
        build.OnResolve(api.OnResolveOptions{Filter: `^env$`},
          func(args api.OnResolveArgs) (api.OnResolveResult, error) {
            return api.OnResolveResult{
              Path: args.Path,
              Namespace: "env",
            }, nil
          })

        build.OnLoad(api.OnLoadOptions{Filter: `.*`, Namespace: "env"},
          func(args api.OnLoadArgs) (api.OnLoadResult, error) {
            mappings := make(map[string]string)
            for _, item := range os.Environ() {
              if equals := strings.IndexByte(item, '='); equals != -1 {
                mappings[item[:equals]] = item[equals+1:]
              }
            }
            bytes, _ := json.Marshal(mappings)
            contents := string(bytes)
            return api.OnLoadResult{
              Contents: &contents,
              Loader: api.LoaderJSON,
            }, nil
          })
      },
    }

    func main() {
      result := api.Build(api.BuildOptions{
        EntryPoints: []string{"app.js"},
        Bundle:      true,
        Outfile:     "out.js",
        Plugins:     []api.Plugin{envPlugin},
        Write:       true,
        LogLevel:    api.LogLevelInfo,
      })

      if len(result.Errors) > 0 {
        os.Exit(1)
      }
    }
    ```

    Comprehensive documentation for the plugin API is not yet available but is coming soon.

* Add the `outbase` option ([#471](https://github.com/evanw/esbuild/pull/471))

    Currently, esbuild uses the lowest common ancestor of the entrypoints to determine where to place each entrypoint's output file. This is an excellent default, but is not ideal in some situations. Take for example an app with a folder structure similar to Next.js, with js files at `pages/a/b/c.js` and `pages/a/b/d.js`. These two files correspond to the paths `/a/b/c` and `/a/b/d`. Ideally, esbuild would emit `out/a/b/c.js` and `out/a/b/d.js`. However, esbuild identifies `pages/a/b` as the lowest common ancestor and emits `out/c.js` and `out/d.js`. This release introduces an `--outbase` argument to the cli that allows the user to choose which path to base entrypoint output paths on. With this change, running esbuild with `--outbase=pages` results in the desired behavior. This change was contributed by [@nitsky](https://github.com/nitsky).

## 0.8.0

**This release contains backwards-incompatible changes.** Since esbuild is before version 1.0.0, these changes have been released as a new minor version to reflect this (as [recommended by npm](https://docs.npmjs.com/misc/semver)). You should either be pinning the exact version of `esbuild` in your `package.json` file or be using a version range syntax that only accepts patch upgrades such as `^0.7.0`. See the documentation about [semver](https://docs.npmjs.com/misc/semver) for more information.

The breaking changes are as follows:

* Changed the transform API result object

    For the transform API, the return values `js` and `jsSourceMap` have been renamed to `code` and `map` respectively. This is because esbuild now supports CSS as a first-class content type, and returning CSS code in a variable called `js` made no sense.

* The class field transform is now more accurate

    Class fields look like this:

    ```js
    class Foo {
      foo = 123
    }
    ```

    Previously the transform for class fields used a normal assignment for initialization:

    ```js
    class Foo {
      constructor() {
        this.foo = 123;
      }
    }
    ```

    However, this doesn't exactly follow the initialization behavior in the JavaScript specification. For example, it can cause a setter to be called if one exists with that property name, which isn't supposed to happen. A more accurate transform that used `Object.defineProperty()` instead was available under the `--strict:class-fields` option.

    This release removes the `--strict:class-fields` option and makes that the default behavior. There is no longer a way to compile class fields to normal assignments instead, since that doesn't follow JavaScript semantics. Note that for legacy reasons, TypeScript code will still compile class fields to normal assignments unless `useDefineForClassFields` is enabled in `tsconfig.json` just like the official TypeScript compiler.

* When bundling stdin using the API, `resolveDir` is now required to resolve imports

    The `resolveDir` option specifies the directory to resolve relative imports against. Previously it defaulted to the current working directory. Now it no longer does, so you must explicitly specify it if you need it:

    ```js
    const result = await esbuild.build({
      stdin: {
        contents,
        resolveDir,
      },
      bundle: true,
      outdir,
    })
    ```

    This was changed because the original behavior was unintentional, and because being explicit seems better in this case. Note that this only affects the JavaScript and Go APIs. The resolution directory for stdin passed using the command-line API still defaults to the current working directory.

    In addition, it is now possible for esbuild to discover input source maps linked via `//# sourceMappingURL=` comments relative to the `resolveDir` for stdin. This previously only worked for files with a real path on the file system.

* Made names in the Go API consistent

    Previously some of the names in the Go API were unnecessarily different than the corresponding names in the CLI and JavaScript APIs. This made it harder to write documentation and examples for these APIs that work consistently across all three API surfaces. These different names in the Go API have been fixed:

    * `Defines` → `Define`
    * `Externals` → `External`
    * `Loaders` → `Loader`
    * `PureFunctions` → `Pure`

* The global name parameter now takes a JavaScript expression ([#293](https://github.com/evanw/esbuild/issues/293))

    The global name parameter determines the name of the global variable created for exports with the IIFE output format. For example, a global name of `abc` would generate the following IIFE:

    ```js
    var abc = (() => {
      ...
    })();
    ```

    Previously this name was injected into the source code verbatim without any validation. This meant a global name of `abc.def` would generate this code, which is a syntax error:

    ```js
    var abc.def = (() => {
      ...
    })();
    ```

    With this release, a global name of `abc.def` will now generate the following code instead:

    ```js
    var abc = abc || {};
    abc.def = (() => {
      ...
    })();
    ```

    The full syntax is an identifier followed by one or more property accesses. If you need to include a `.` character in your property name, you can use an index expression instead. For example, the global name `versions['1.0']` will generate the following code:

    ```js
    var versions = versions || {};
    versions["1.0"] = (() => {
      ...
    })();
    ```

* Removed the workaround for `document.all` with nullish coalescing and optional chaining

    The `--strict:nullish-coalescing` and `--strict:optional-chaining` options have been removed. They only existed to address a theoretical problem where modern code that uses the new `??` and `?.` operators interacted with the legacy [`document.all` object](https://developer.mozilla.org/en-US/docs/Web/API/Document/all) that has been deprecated for a long time. Realistically this case is extremely unlikely to come up in practice, so these obscure options were removed to simplify the API and reduce code complexity. For what it's worth this behavior also matches [Terser](https://github.com/terser/terser), a commonly-used JavaScript minifier.

## 0.7.22

* Add `tsconfigRaw` to the transform API ([#483](https://github.com/evanw/esbuild/issues/483))

    The `build` API uses access to the file system and doesn't run in the browser, but the `transform` API doesn't access the file system and can run in the browser. Previously you could only use the build API for certain scenarios involving TypeScript code and `tsconfig.json` files, such as configuring the `importsNotUsedAsValues` setting.

    You can now use `tsconfig.json` with the transform API by passing in the raw contents of that file:

    ```js
    let result = esbuild.transformSync(ts, {
      loader: 'ts',
      tsconfigRaw: {
        compilerOptions: {
          importsNotUsedAsValues: 'preserve',
        },
      },
    })
    ```

    Right now four values are supported with the transform API: `jsxFactory`, `jsxFragmentFactory`, `useDefineForClassFields`, and `importsNotUsedAsValues`. The values `extends`, `baseUrl`, and `paths` are not supported because they require access to the file system and the transform API deliberately does not access the file system.

    You can also pass the `tsconfig.json` file as a string instead of a JSON object if you prefer. This can be useful because `tsconfig.json` files actually use a weird pseudo-JSON syntax that allows comments and trailing commas, which means it can't be parsed with `JSON.parse()`.

* Warn about `process.env.NODE_ENV`

    Some popular browser-oriented libraries such as React use `process.env.NODE_ENV` even though this is not an API provided by the browser. While esbuild makes it easy to replace this at compile time using the `--define` feature, you must still do this manually and it's easy to forget. Now esbuild will warn you if you're bundling code containing `process.env.NODE_ENV` for the browser and you haven't configured it to be replaced by something.

* Work around a bug in Safari for the run-time code ([#489](https://github.com/evanw/esbuild/issues/489))

    The `Object.getOwnPropertyDescriptor` function in Safari is broken for numeric properties. It incorrectly returns `undefined`, which crashes the run-time code esbuild uses to bind modules together. This release contains code to avoid a crash in this case.

## 0.7.21

* Use bracketed escape codes for non-BMP characters

    The previous release introduced code that escapes non-ASCII characters using ASCII escape sequences. Since JavaScript uses UCS-2/UTF-16 internally, a non-[BMP](https://en.wikipedia.org/wiki/Plane_(Unicode)#Basic_Multilingual_Plane) character such as `𐀀` ended up being encoded using a [surrogate pair](https://en.wikipedia.org/wiki/Universal_Character_Set_characters#Surrogates): `\uD800\uDC00`. This is fine when the character is contained in a string, but it causes a syntax error when that character is used as an identifier.

    This release fixes this issue by using the newer bracketed escape code instead: `\u{10000}`. One complication with doing this is that this escape code won't work in older environments without ES6 support. Because of this, using identifiers containing non-BMP characters is now an error if the configured target environment doesn't support bracketed escape codes.

* Escape non-ASCII characters in properties

    The previous release overlooked the need to escape non-ASCII characters in properties in various places in the grammar (e.g. object literals, property accesses, import and export aliases). This resulted in output containing non-ASCII characters even with `--charset=ascii`. These characters should now always be escaped, even in properties.

## 0.7.20

* Default to ASCII-only output ([#70](https://github.com/evanw/esbuild/issues/70), [#485](https://github.com/evanw/esbuild/issues/485))

    While esbuild's output is encoded using UTF-8 encoding, there are many other character encodings in the wild (e.g. [Windows-1250](https://en.wikipedia.org/wiki/Windows-1250)). You can explicitly mark the output files as UTF-8 by adding `<meta charset="utf-8">` to your HTML page or by including `charset=utf-8` in the `Content-Type` header sent by your server. This is probably a good idea regardless of the contents of esbuild's output since information being displayed to users is probably also encoded using UTF-8.

    However, sometimes it's not possible to guarantee that your users will be running your code as UTF-8. For example, you may not control the server response or the contents of the HTML page that loads your script. Also, if your code needs to run in IE, there are [certain cases](https://docs.microsoft.com/en-us/troubleshoot/browsers/wrong-character-set-for-html-page) where IE may ignore the `<meta charset="utf-8">` tag and make up another encoding instead.

    Also content encoded using UTF-8 may be parsed up to 1.7x slower by the browser than ASCII-only content, at least according to this blog post from the V8 team: https://v8.dev/blog/scanner. The official recommendation is to "avoid non-ASCII identifiers where possible" to improve parsing performance.

    For these reasons, esbuild's default output has been changed to ASCII-only. All Unicode code points in identifiers and strings that are outside of the printable ASCII range (`\x20-\x7E` inclusive) are escaped using backslash escape sequences. If you would like to use raw UTF-8 encoding instead, you can pass the `--charset=utf8` flag to esbuild.

    Further details:

    * This does not yet escape non-ASCII characters embedded in regular expressions. This is because esbuild does not currently parse the contents of regular expressions at all. The flag was added despite this limitation because it's still useful for code that doesn't contain cases like this.

    * This flag does not apply to comments. I believe preserving non-ASCII data in comments should be fine because even if the encoding is wrong, the run time environment should completely ignore the contents of all comments. For example, the [V8 blog post](https://v8.dev/blog/scanner) mentions an optimization that avoids decoding comment contents completely. And all comments other than license-related comments are stripped out by esbuild anyway.

    * This new `--charset` flag simultaneously applies to all output file types (JavaScript, CSS, and JSON). So if you configure your server to send the correct `Content-Type` header and want to use `--charset=utf8`, make sure your server is configured to treat both `.js` and `.css` files as UTF-8.

* Interpret escape sequences in CSS tokens

    Escape sequences in CSS tokens are now interpreted. This was already the case for string and URL tokens before, but this is now the case for all identifier-like tokens as well. For example, `c\6flor: #\66 00` is now correctly recognized as `color: #f00`.

* Support `.css` with the `--out-extension` option

    The `--out-extension` option was added so you could generate `.mjs` and `.cjs` files for node like this: `--out-extension:.js=.mjs`. However, now that CSS is a first-class content type in esbuild, this should also be available for `.css` files. I'm not sure why you would want to do this, but you can now do `--out-extension:.css=.something` too.

## 0.7.19

* Add the `--avoid-tdz` option for large bundles in Safari ([#478](https://github.com/evanw/esbuild/issues/478))

    This is a workaround for a performance issue with certain large JavaScript files in Safari.

    First, some background. In JavaScript the `var` statement is "hoisted" meaning the variable is declared immediately in the closest surrounding function, module, or global scope. Accessing one of these variables before its declaration has been evaluated results in the value `undefined`. In ES6 the `const`, `let`, and `class` statements introduce what's called a "temporal dead zone" or TDZ. This means that, unlike `var` statements, accessing one of these variable before its declaration has been evaluated results in a `ReferenceError` being thrown. It's called a "temporal dead zone" because it's a zone of time in which the variable is inaccessible.

    According to [this WebKit bug](https://bugs.webkit.org/show_bug.cgi?id=199866), there's a severe performance issue with the tracking of TDZ checks in JavaScriptCore, the JavaScript JIT compiler used by WebKit. In a large private code base I have access to, the initialization phase of the bundle produced by esbuild runs 10x faster in Safari if top-level `const`, `let`, and `class` are replaced with `var`. It's a difference between a loading time of about 2sec vs. about 200ms. This transformation is not enabled by default because it changes the semantics of the code (it removes the TDZ and `const` assignment checks). However, this change in semantics may be acceptable for you given the performance trade-off. You can enable it with the `--avoid-tdz` flag.

* Warn about assignment to `const` symbols

    Now that some `const` symbols may be converted to `var` due to `--avoid-tdz`, it seems like a good idea to at least warn when an assignment to a `const` symbol is detected during bundling. Otherwise accidental assignments to `const` symbols could go unnoticed if there isn't other tooling in place such as TypeScript or a linter.

## 0.7.18

* Treat paths in CSS without a `./` or `../` prefix as relative ([#469](https://github.com/evanw/esbuild/issues/469))

    JavaScript paths starting with `./` or `../` are considered relative paths, while other JavaScript paths are considered package paths and are looked up in that package's `node_modules` directory. Currently `url()` paths in CSS files use that same logic, so `url(images/image.png)` checks for a file named `image.png` in the `image` package.

    This release changes this behavior. Now `url(images/image.png)` first checks for `./images/image.png`, then checks for a file named `image.png` in the `image` package. This behavior should match the behavior of Webpack's standard `css-loader` package.

* Import non-enumerable properties from CommonJS modules ([#472](https://github.com/evanw/esbuild/issues/472))

    You can now import non-enumerable properties from CommonJS modules using an ES6 `import` statement. Here's an example of a situation where that might matter:

    ```js
    // example.js
    module.exports = class {
      static method() {}
    }
    ```

    ```js
    import { method } from './example.js'
    method()
    ```

    Previously that didn't work because the `method` property is non-enumerable. This should now work correctly.

    A minor consequence of this change is that re-exporting from a file using `export * from` will no longer re-export properties inherited from the prototype of the object assigned to `module.exports`. This is because run-time property copying has been changed from a for-in loop to `Object.getOwnPropertyNames`. This change should be inconsequential because as far as I can tell this isn't something any other bundler supports either.

* Remove arrow functions in runtime with `--target=es5`

    The `--target=es5` flag is intended to prevent esbuild from introducing any ES6+ syntax into the generated output file. For example, esbuild usually shortens `{x: x}` into `{x}` since it's shorter, except that requires ES6 support. This release fixes a bug where `=>` arrow expressions in esbuild's runtime of helper functions were not converted to `function` expressions when `--target=es5` was present.

* Merge local variable declarations across files when minifying

    Currently files are minified in parallel and then concatenated together for maximum performance. However, that means certain constructs are not optimally minified if they span multiple files. For example, a bundle containing two files `var a = 1` and `var b = 2` should ideally become `var a=1,b=2;` after minification but it currently becomes `var a=0;var b=2;` instead due to parallelism.

    With this release, esbuild will generate `var a=1,b=2;` in this scenario. This is achieved by splicing the two files together to remove the trailing `;` and the leading `var `, which is more complicated than it sounds when you consider rewriting the source maps.

## 0.7.17

* Add `--public-path=` for the `file` loader ([#459](https://github.com/evanw/esbuild/issues/459))

    The `file` loader causes importing a file to cause that file to be copied into the output directory. The name of the file is exported as the default export:

    ```js
    // Assume ".png" is set to the "file" loader
    import name from 'images/image.png'

    // This prints something like "image.L3XDQOAT.png"
    console.log(name)
    ```

    The new public path setting configures the path prefix. So for example setting it to `https://www.example.com/v1` would change the output text for this example to `https://www.example.com/v1/image.L3XDQOAT.png`.

* Add `--inject:` for polyfills ([#451](https://github.com/evanw/esbuild/issues/451))

    It's now possible to replace global variables with imports from a file with `--inject:file.js`. Note that `file.js` must export symbols using the `export` keyword for this to work. This can be used to polyfill a global variable in code you don't control. For example:

    ```js
    // process.js
    export let process = {cwd() {}}
    ```

    ```js
    // entry.js
    console.log(process.cwd())
    ```

    Building this with `esbuild entry.js --inject:process.js` gives this:

    ```js
    let process = {cwd() {
    }};
    console.log(process.cwd());
    ```

    You can also combine this with the existing `--define` feature to be more selective about what you import. For example:

    ```js
    // process.js
    export function dummy_process_cwd() {}
    ```

    ```js
    // entry.js
    console.log(process.cwd())
    ```

    Building this with `esbuild entry.js --inject:process.js --define:process.cwd=dummy_process_cwd` gives this:

    ```js
    function dummy_process_cwd() {
    }
    console.log(dummy_process_cwd());
    ```

    Note that this means you can use `--inject` to provide the implementation for JSX expressions (e.g. auto-import the `react` package):

    ```js
    // shim.js
    export * as React from 'react'
    ```

    ```jsx
    // entry.jsx
    console.log(<div/>)
    ```

    Building this with `esbuild entry.js --inject:shim.js --format=esm` gives this:

    ```js
    import * as React from "react";
    console.log(/* @__PURE__ */ React.createElement("div", null));
    ```

    You can also use `--inject:file.js` with files that have no exports. In that case the injected file just comes first before the rest of the output as if every input file contained `import "./file.js"`. Because of the way ECMAScript modules work, this injection is still "hygienic" in that symbols with the same name in different files are renamed so they don't collide with each other.

    If you want to _conditionally_ import a file only if the export is actually used, you should mark the injected file as not having side effects by putting it in a package and adding `"sideEffects": false` in that package's `package.json` file. This setting is a [convention from Webpack](https://webpack.js.org/guides/tree-shaking/#mark-the-file-as-side-effect-free) that esbuild respects for any imported file, not just files used with `--inject`.

* Add an ECMAScript module build for the browser ([#342](https://github.com/evanw/esbuild/pull/342))

    The [current browser API](https://github.com/evanw/esbuild/blob/cfaedaeeb35ae6e8b42921ab98ad98f75375d39f/docs/js-api.md#browser-api) lets you use esbuild in the browser via the `esbuild-wasm` package and a script tag:

    ```html
    <script src="node_modules/esbuild-wasm/lib/browser.js"></script>
    <script>
      esbuild.startService({
        wasmURL: 'node_modules/esbuild-wasm/esbuild.wasm',
      }).then(service => {
        // Use service
      })
    </script>
    ```

    In addition to this approach, you can now also use esbuild in the browser from a module-type script (note the use of `esm/browser.js` instead of `lib/browser.js`):

    ```html
    <script type="module">
      import * as esbuild from 'node_modules/esbuild-wasm/esm/browser.js'
      esbuild.startService({
        wasmURL: 'node_modules/esbuild-wasm/esbuild.wasm',
      }).then(service => {
        // Use service
      })
    </script>
    ```

    Part of this fix was contributed by [@calebeby](https://github.com/calebeby).

## 0.7.16

* Fix backward slashes in source maps on Windows ([#463](https://github.com/evanw/esbuild/issues/463))

    The relative path fix in the previous release caused a regression where paths in source maps contained `\` instead of `/` on Windows. That is incorrect because source map paths are URLs, not file system paths. This release replaces `\` with `/` for consistency on Windows.

* `module.require()` is now an alias for `require()` ([#455](https://github.com/evanw/esbuild/issues/455))

    Some packages such as [apollo-server](https://github.com/apollographql/apollo-server) use `module.require()` instead of `require()` with the intent of bypassing the bundler's `require` and calling the underlying function from `node` instead. Unfortunately that doesn't actually work because CommonJS module semantics means `module` is a variable local to that file's CommonJS closure instead of the host's `module` object.

    This wasn't an issue when using `apollo-server` with Webpack because the literal expression `module.require()` is automatically rewritten to `require()` by Webpack: [webpack/webpack#7750](https://github.com/webpack/webpack/pull/7750). To get this package to work, esbuild now matches Webpack's behavior here. Calls to `module.require()` will become external calls to `require()` as long as the required path has been marked as external.

## 0.7.15

* Lower `export * as` syntax for ES2019 and below

    The `export * from 'path'` syntax was added in ES2015 but the `export * as name from 'path'` syntax was added more recently in ES2020. This is a shorthand for an import followed by an export:

    ```js
    // ES2020
    export * as name from 'path'

    // ES2019
    import * as name from 'path'
    export {name}
    ```

    With this release, esbuild will now undo this shorthand syntax when using `--target=es2019` or below.

* Better code generation for TypeScript files with type-only exports ([#447](https://github.com/evanw/esbuild/issues/447))

    Previously TypeScript files could have an unnecessary CommonJS wrapper in certain situations. The specific situation is bundling a file that re-exports something from another file without any exports. This happens because esbuild automatically considers a module to be a CommonJS module if there is no ES6 `import`/`export` syntax.

    This behavior is undesirable because the CommonJS wrapper is usually unnecessary. It's especially undesirable for cases where the re-export uses `export * from` because then the re-exporting module is also converted to a CommonJS wrapper (since re-exporting everything from a CommonJS module must be done at run-time). That can also impact the bundle's exports itself if the entry point does this and the format is `esm`.

    It is generally equivalent to avoid the CommonJS wrapper and just rewrite the imports to an `undefined` literal instead:

    ```js
    import {name} from './empty-file'
    console.log(name)
    ```

    This can be rewritten to this instead (with a warning generated about `name` being missing):

    ```js
    console.log(void 0)
    ```

    With this release, this is now how cases like these are handled. The only case where this can't be done is when the import uses the `import * as` syntax. In that case a CommonJS wrapper is still necessary because the namespace cannot be rewritten to `undefined`.

* Add support for `importsNotUsedAsValues` in TypeScript ([#448](https://github.com/evanw/esbuild/issues/448))

    The `importsNotUsedAsValues` field in `tsconfig.json` is now respected. Setting it to `"preserve"` means esbuild will no longer remove unused imports in TypeScript files. This field was added in TypeScript 3.8.

* Fix relative paths in generated source maps ([#444](https://github.com/evanw/esbuild/issues/444))

    Currently paths in generated source map files don't necessarily correspond to real file system paths. They are really only meant to be human-readable when debugging in the browser.

    However, the Visual Studio Code debugger expects these paths to point back to the original files on the file system. With this release, it should now always be possible to get back to the original source file by joining the directory containing the source map file with the relative path in the source map.

    This fix was contributed by [@yoyo930021](https://github.com/yoyo930021).

## 0.7.14

* Fix a bug with compound import statements ([#446](https://github.com/evanw/esbuild/issues/446))

    Import statements can simultaneously contain both a default import and a namespace import like this:

    ```js
    import defVal, * as nsVal from 'path'
    ```

    These statements were previously miscompiled when bundling if the import path was marked as external, or when converting to a specific output format, and the namespace variable itself was used for something other than a property access. The generated code contained a syntax error because it generated a `{...}` import clause containing the default import.

    This particular problem was caused by code that converts namespace imports into import clauses for more efficient bundling. This transformation should not be done if the namespace import cannot be completely removed:

    ```js
    // Can convert namespace to clause
    import defVal, * as nsVal from 'path'
    console.log(defVal, nsVal.prop)
    ```

    ```js
    // Cannot convert namespace to clause
    import defVal, * as nsVal from 'path'
    console.log(defVal, nsVal)
    ```

## 0.7.13

* Fix `mainFields` in the JavaScript API ([#440](https://github.com/evanw/esbuild/issues/440) and [#441](https://github.com/evanw/esbuild/pull/441))

    It turns out the JavaScript bindings for the `mainFields` API option didn't work due to a copy/paste error. The fix for this was contributed by [@yoyo930021](https://github.com/yoyo930021).

* The benchmarks have been updated

    The benchmarks now include Parcel 2 and Webpack 5 (in addition to Parcel 1 and Webpack 4, which were already included). It looks like Parcel 2 is slightly faster than Parcel 1 and Webpack 5 is significantly slower than Webpack 4.

## 0.7.12

* Fix another subtle ordering issue with `import` statements

    When importing a file while bundling, the import statement was ordered before the imported code. This could affect import execution order in complex scenarios involving nested hybrid ES6/CommonJS modules. The fix was to move the import statement to after the imported code instead. This issue affected the `@sentry/browser` package.

## 0.7.11

* Fix regression in 0.7.9 when minifying with code splitting ([#437](https://github.com/evanw/esbuild/issues/437))

    In certain specific cases, bundling and minifying with code splitting active can cause a crash. This is a regression that was introduced in version 0.7.9 due to the fix for issue [#421](https://github.com/evanw/esbuild/issues/421). The crash has been fixed and this case now has test coverage.

## 0.7.10

* Recover from bad `main` field in `package.json` ([#423](https://github.com/evanw/esbuild/issues/423))

    Some packages are published with invalid information in the `main` field of `package.json`. In that case, path resolution should fall back to searching for a file named `index.js` before giving up. This matters for the `simple-exiftool` package, for example.

* Ignore TypeScript types on `catch` clause bindings ([435](https://github.com/evanw/esbuild/issues/435))

    This fixes an issue where using a type annotation in a `catch` clause like this was a syntax error:

    ```ts
    try {
    } catch (x: unknown) {
    }
    ```

## 0.7.9

* Fixed panic when using a `url()` import in CSS with the `--metafile` option

    This release fixes a crash that happens when `metafile` output is enabled and the `url()` syntax is used in a CSS file to import a successfully-resolved file.

* Minify some CSS colors

    The minifier can now reduce the size of some CSS colors. This is the initial work to start CSS minification in general beyond whitespace removal. There is currently support for minifying hex, `rgb()/rgba()`, and `hsl()/hsla()` into hex or shorthand hex. The minification process respects the configured target browser and doesn't use any syntax that wouldn't be supported.

* Lower newer CSS syntax for older browsers

    Newer color syntax such as `rgba(255 0 0 / 50%)` will be converted to older syntax (in this case `rgba(255, 0, 0, 0.5)`) when the target browser doesn't support the newer syntax. For example, this happens when using `--target=chrome60`.

* Fix an ordering issue with `import` statements ([#421](https://github.com/evanw/esbuild/issues/421))

    Previously `import` statements that resolved to a CommonJS module turned into a call to `require()` inline. This was subtly incorrect when combined with tree shaking because it could sometimes cause imported modules to be reordered:

    ```js
    import {foo} from './cjs-file'
    import {bar} from './esm-file'
    console.log(foo, bar)
    ```

    That code was previously compiled into something like this, which is incorrect because the evaluation of `bar` may depend on side effects from importing `cjs-file.js`:

    ```js
    // ./cjs-file.js
    var require_cjs_file = __commonJS(() => {
      ...
    })

    // ./esm-file.js
    let bar = ...;

    // ./example.js
    const cjs_file = __toModule(require_cjs_file())
    console.log(cjs_file.foo, bar)
    ```

    That code is now compiled into something like this:

    ```js
    // ./cjs-file.js
    var require_cjs_file = __commonJS(() => {
      ...
    })

    // ./example.js
    const cjs_file = __toModule(require_cjs_file())

    // ./esm-file.js
    let bar = ...;

    // ./example.js
    console.log(cjs_file.foo, bar)
    ```

    This now means that a single input file can end up in multiple discontiguous regions in the output file as is the case with `example.js` here, which wasn't the case before this bug fix.

## 0.7.8

* Move external `@import` rules to the top

    Bundling could cause `@import` rules for paths that have been marked as external to be inserted in the middle of the CSS file. This would cause them to become invalid and be ignored by the browser since all `@import` rules must come first at the top of the file. These `@import` rules are now always moved to the top of the file so they stay valid.

* Better support for `@keyframes` rules

    The parser now directly understands `@keyframes` rules, which means it can now format them more accurately and report more specific syntax errors.

* Minify whitespace around commas in CSS

    Whitespace around commas in CSS will now be pretty-printed when not minifying and removed when minifying. So `a , b` becomes `a, b` when pretty-printed and `a,b` when minified.

* Warn about unknown at-rules in CSS

    Using an `@rule` in a CSS file that isn't known by esbuild now generates a warning and these rules will be passed through unmodified. If they aren't known to esbuild, they are probably part of a CSS preprocessor syntax that should have been compiled away before giving the file to esbuild to parse.

* Recoverable CSS syntax errors are now warnings

    The base CSS syntax can preserve nonsensical rules as long as they contain valid tokens and have matching opening and closing brackets. These rule with incorrect syntax now generate a warning instead of an error and esbuild preserves the syntax in the output file. This makes it possible to use esbuild to process CSS that was generated by another tool that contains bugs.

    For example, the following code is invalid CSS, and was presumably generated by a bug in an automatic prefix generator:

    ```css
    div {
      -webkit-undefined;
      -moz-undefined;
      -undefined;
    }
    ```

    This code will no longer prevent esbuild from processing the CSS file.

* Treat `url(...)` in CSS files as an import ([#415](https://github.com/evanw/esbuild/issues/415))

    When bundling, the `url(...)` syntax in CSS now tries to resolve the URL as a path using the bundler's built in path resolution logic. The following loaders can be used with this syntax: `text`, `base64`, `file`, `dataurl`, and `binary`.

* Automatically treat certain paths as external

    The following path forms are now automatically considered external:

    * `http://example.com/image.png`
    * `https://example.com/image.png`
    * `//example.com/image.png`
    * `data:image/png;base64,iVBORw0KGgo=`

    In addition, paths starting with `#` are considered external in CSS files, which allows the following syntax to continue to work:

    ```css
    path {
      /* This can be useful with SVG DOM content */
      fill: url(#filter);
    }
    ```

## 0.7.7

* Fix TypeScript decorators on static members

    This release fixes a bug with the TypeScript transform for the `experimentalDecorators` setting. Previously the target object for all decorators was the class prototype, which was incorrect for static members. Static members now correctly use the class object itself as a target object.

* Experimental support for CSS syntax ([#20](https://github.com/evanw/esbuild/issues/20))

    This release introduces the new `css` loader, enabled by default for `.css` files. It has the following features:

    * You can now use esbuild to process CSS files by passing a CSS file as an entry point. This means CSS is a new first-class file type and you can use it without involving any JavaScript code at all.

    * When bundling is enabled, esbuild will bundle multiple CSS files together if they are referenced using the `@import "./file.css";` syntax. CSS files can be excluded from the bundle by marking them as external similar to JavaScript files.

    * There is basic support for pretty-printing CSS, and for whitespace removal when the `--minify` flag is present. There isn't any support for CSS syntax compression yet. Note that pretty-printing and whitespace removal both rely on the CSS syntax being recognized. Currently esbuild only recognizes certain CSS syntax and passes through unrecognized syntax unchanged.

    Some things to keep in mind:

    * CSS support is a significant undertaking and this is the very first release. There are almost certainly going to be issues. This is an experimental release to land the code and get feedback.

    * There is no support for CSS modules yet. Right now all class names are in the global namespace. Importing a CSS file into a JavaScript file will not result in any import names.

    * There is currently no support for code splitting of CSS. I haven't tested multiple entry-point scenarios yet and code splitting will require additional changes to the AST format.

## 0.7.6

* Fix JSON files with multiple entry points ([#413](https://github.com/evanw/esbuild/issues/413))

    This release fixes an issue where a single build operation containing multiple entry points and a shared JSON file which is used by more than one of those entry points can generate incorrect code for the JSON file when code splitting is disabled. The problem was not cloning the AST representing the JSON file before mutating it.

* Silence warnings about `require.resolve()` for external paths ([#410](https://github.com/evanw/esbuild/issues/410))

    Bundling code containing a call to node's [`require.resolve()`](https://nodejs.org/api/modules.html#modules_require_resolve_request_options) function causes a warning because it's an unsupported use of `require` that does not end up being bundled. For example, the following code will likely have unexpected behavior if `foo` ends up being bundled because the `require()` call is evaluated at bundle time but the `require.resolve()` call is evaluated at run time:

    ```js
    let foo = {
      path: require.resolve('foo'),
      module: require('foo'),
    };
    ```

    These warnings can already be disabled by surrounding the code with a `try`/`catch` statement. With this release, these warnings can now also be disabled by marking the path as external.

* Ensure external relative paths start with `./` or `../`

    Individual file paths can be marked as external in addition to package paths. In that case, the path to the file is rewritten to be relative to the output directory. However, previously the relative path for files in the output directory itself did not start with `./`, meaning they could potentially be interpreted as a package path instead of a relative path. These paths are now prefixed with `./` to avoid this edge case.

## 0.7.5

* Fix an issue with automatic semicolon insertion after `let` ([#409](https://github.com/evanw/esbuild/issues/409))

    The character sequence `let` can be considered either a keyword or an identifier depending on the context. A fix was previously landed in version 0.6.31 to consider `let` as an identifier in code like this:

    ```js
    if (0) let
    x = 0
    ```

    Handling this edge case is useless but the behavior is required by the specification. However, that fix also unintentionally caused `let` to be considered an identifier in code like this:

    ```js
    let
    x = 0
    ```

    In this case, `let` should be considered a keyword instead. This has been fixed.

* Fix some additional conformance tests

    Some additional syntax edge cases are now forbidden including `let let`, `import {eval} from 'path'`, and `if (1) x: function f() {}`.

## 0.7.4

* Undo an earlier change to try to improve yarn compatibility ([#91](https://github.com/evanw/esbuild/pull/91) and [#407](https://github.com/evanw/esbuild/issues/407))

    The [yarn package manager](https://github.com/yarnpkg/yarn) behaves differently from npm and is not compatible in many ways. While npm is the only officially supported package manager for esbuild, people have contributed fixes for other package managers including yarn. One such fix is PR [#91](https://github.com/evanw/esbuild/pull/91) which makes sure the install script only runs once for a given installation directory.

    I suspect this fix is actually incorrect, and is the cause of issue [#407](https://github.com/evanw/esbuild/issues/407). The problem seems to be that if you change the version of a package using `yarn add esbuild@version`, yarn doesn't clear out the installation directory before reinstalling the package so the package ends up with a mix of files from both package versions. This is not how npm behaves and seems like a pretty severe bug in yarn. I am reverting PR [#91](https://github.com/evanw/esbuild/pull/91) in an attempt to fix this issue.

* Disable some warnings for code inside `node_modules` directories ([#395](https://github.com/evanw/esbuild/issues/395) and [#402](https://github.com/evanw/esbuild/issues/402))

    Using esbuild to build code with certain suspicious-looking syntax may generate a warning. These warnings don't fail the build (the build still succeeds) but they point out code that is very likely to not behave as intended. This has caught real bugs in the past:

    * [rollup/rollup#3729](https://github.com/rollup/rollup/issues/3729): Invalid dead code removal for return statement due to ASI
    * [aws/aws-sdk-js#3325](https://github.com/aws/aws-sdk-js/issues/3325): Array equality bug in the Node.js XML parser
    * [olifolkerd/tabulator#2962](https://github.com/olifolkerd/tabulator/issues/2962): Nonsensical comparisons with typeof and "null"
    * [mrdoob/three.js#11183](https://github.com/mrdoob/three.js/pull/11183): Comparison with -0 in Math.js
    * [mrdoob/three.js#11182](https://github.com/mrdoob/three.js/pull/11182): Cperator precedence bug in WWOBJLoader2.js

	However, it's not esbuild's job to find bugs in other libraries, and these warnings are problematic for people using these libraries with esbuild. The only fix is to either disable all esbuild warnings and not get warnings about your own code, or to try to get the warning fixed in the affected library. This is especially annoying if the warning is a false positive as was the case in https://github.com/firebase/firebase-js-sdk/issues/3814. So these warnings are now disabled for code inside `node_modules` directories.

## 0.7.3

* Fix compile error due to missing `unix.SYS_IOCTL` in the latest `golang.org/x/sys` ([#396](https://github.com/evanw/esbuild/pull/396))

    The `unix.SYS_IOCTL` export was apparently removed from `golang.org/x/sys` recently, which affected code in esbuild that gets the width of the terminal. This code now uses another method of getting the terminal width. The fix was contributed by [@akayj](https://github.com/akayj).

* Validate that the versions of the host code and the binary executable match ([#407](https://github.com/evanw/esbuild/issues/407))

    After the install script runs, the version of the downloaded binary should always match the version of the package being installed. I have added some additional checks to verify this in case this invariant is ever broken. Breaking this invariant is very bad because it means the code being run is a mix of code from different package versions.

## 0.7.2

* Transform arrow functions to function expressions with `--target=es5` ([#182](https://github.com/evanw/esbuild/issues/182) and [#297](https://github.com/evanw/esbuild/issues/297))

    Arrow functions are now transformed into function expressions when targeting `es5`. For example, this code:

    ```js
    function foo() {
      var x = () => [this, arguments]
      return x()
    }
    ```

    is transformed into this code:

    ```js
    function foo() {
      var _this = this, _arguments = arguments;
      var x = function() {
        return [_this, _arguments];
      };
      return x();
    }
    ```

* Parse template literal types from TypeScript 4.1

    TypeScript 4.1 includes a new feature called template literal types. You can read [the announcement](https://devblogs.microsoft.com/typescript/announcing-typescript-4-1-beta/#template-literal-types) for more details. The following syntax can now be parsed correctly by esbuild:

    ```ts
    let foo: `${'a' | 'b'}-${'c' | 'd'}` = 'a-c'
    ```

* Parse key remapping in mapped types from TypeScript 4.1

    TypeScript 4.1 includes a new feature called key remapping in mapped types. You can read [the announcement](https://devblogs.microsoft.com/typescript/announcing-typescript-4-1-beta/#key-remapping-mapped-types) for more details. The following syntax can now be parsed correctly by esbuild:

    ```ts
    type RemoveField<T, F> = { [K in keyof T as Exclude<K, F>]: T[K] }
    ```

* Allow automatic semicolon insertion before the TypeScript `as` operator

    The following code now correctly parses as two separate statements instead of one statement with a newline in the middle:

    ```ts
    let foo = bar
    as (null);
    ```

* Fix a bug where `module` was incorrectly minified for non-JavaScript loaders

    If you pass a non-JavaScript file such as a `.json` file to esbuild, it will by default generate `module.exports = {...}`. However, the `module` variable would incorrectly be minified when `--minify` is present. This issue has been fixed. This bug did not appear if `--format=cjs` was also present, only if no `--format` flag was specified.

* Fix bugs with `async` functions ([#388](https://github.com/evanw/esbuild/issues/388))

    This release contains correctness fixes for `async` arrow functions with regard to the `arguments` variable. This affected `async` arrow functions nested inside `function` expressions or statements. Part of this fix was contributed by [@rtsao](https://github.com/rtsao).

* Fix `export` clause when converting to CommonJS in transform API calls ([#393](https://github.com/evanw/esbuild/issues/393))

    This release fixes some bugs with the recently-released feature in version 0.6.32 where you can specify an output format even when bundling is disabled. This is the case when using the transform API call, for example. Previously esbuild could generate code that crashed at run time while trying to export something incorrectly. This only affected code with top-level `export` statements. This has been fixed and these cases now have test coverage.

## 0.7.1

* Fix bug that forbids `undefined` values in the JavaScript API

    The validation added in the previous release was accidentally overly restrictive and forbids `undefined` values for optional properties. This release allows `undefined` values again (which are simply ignored).

## 0.7.0

* Mark output files with a hashbang as executable ([#364](https://github.com/evanw/esbuild/issues/364))

    Output files that start with a hashbang line such as `#!/usr/bin/env node` will now automatically be marked as executable. This lets you run them directly in a Unix-like shell without using the `node` command.

* Use `"main"` for `require()` and `"module"` for `import` ([#363](https://github.com/evanw/esbuild/issues/363))

    The [node module resolution algorithm](https://nodejs.org/api/modules.html#modules_all_together) uses the `"main"` field in `package.json` to determine which file to load when a package is loaded with `require()`. Independent of node, most bundlers have converged on a convention where the `"module"` field takes precedence over the `"main"` field when present. Package authors can then use the `"module"` field to publish the same code in a different format for bundlers than for node.

    This is commonly used to publish "dual packages" that appear to use ECMAScript modules to bundlers but that appear to use CommonJS modules to node. This is useful because ECMAScript modules improve bundler output by taking advantage of "tree shaking" (basically dead-code elimination) and because ECMAScript modules cause lots of problems in node (for example, node doesn't support importing ECMAScript modules using `require()`).

    The problem is that if code using `require()` resolves to the `"module"` field in esbuild, the resulting value is currently always an object. ECMAScript modules export a namespace containing all exported properties. There is no direct equivalent of `module.exports = value` in CommonJS. The closest is `export default value` but the CommonJS equivalent of that is `exports.default = value`. This is problematic for code containing `module.exports = function() {}` which is a frequently-used CommonJS library pattern. An example of such an issue is Webpack issue [#6584](https://github.com/webpack/webpack/issues/6584).

    An often-proposed way to fix this is to map `require()` to `"main"` and map `import` to `"module"`. The problem with this is that it means the same package would be loaded into memory more than once if it is loaded both with `require()` and with `import` (perhaps from separate packages). An example of such an issue is GraphQL issue [#1479](https://github.com/graphql/graphql-js/issues/1479#issuecomment-416718578).

    The workaround for these problems in this release is that esbuild will now exclusively use `"main"` for a package that is loaded using `require()` at least once. Otherwise, if a package is only loaded using `import`, esbuild will exclusively use the `"module"` field. This still takes advantage of tree shaking for ECMAScript modules but gracefully falls back to CommonJS for compatibility.

    Keep in mind that the [`"browser"` field](https://github.com/defunctzombie/package-browser-field-spec) still takes precedence over both `"module"` and `"main"` when building for the browser platform.

* Add the `--main-fields=` flag ([#363](https://github.com/evanw/esbuild/issues/363))

    This adopts a configuration option from Webpack that lets you specify the order of "main fields" from `package.json` to use when determining the main module file for a package. Node only uses `main` but bundlers often respect other ones too such as `module` or `browser`. You can read more about this feature in the Webpack documentation [here](https://webpack.js.org/configuration/resolve/#resolvemainfields).

    The default order when targeting the browser is essentially `browser,module,main` with the caveat that `main` may be chosen over `module` for CommonJS compatibility as described above. If choosing `module` over `main` at the expense of CommonJS compatibility is important to you, this behavior can be disabled by explicitly specifying `--main-fields=browser,module,main`.

    The default order when targeting node is `main,module`. Note that this is different than Webpack, which defaults to `module,main`. This is also for compatibility because some packages incorrectly treat `module` as meaning "code for the browser" instead of what it actually means, which is "code for ES6 environments". Unfortunately this disables most tree shaking that would otherwise be possible because it means CommonJS modules will be chosen over ECMAScript modules. If choosing `module` over `main` is important to you (e.g. to potentially take advantage of improved tree shaking), this behavior can be disabled by explicitly specifying `--main-fields=module,main`.

* Additional validation of arguments to JavaScript API calls ([#381](https://github.com/evanw/esbuild/issues/381))

    JavaScript API calls each take an object with many optional properties as an argument. Previously there was only minimal validation of the contents of that object. If you aren't using TypeScript, this can lead to confusing situations when the data on the object is invalid. Now there is some additional validation done to the shape of the object and the types of the properties.

    It is now an error to pass an object with a property that esbuild won't use. This should help to catch typos. It is also now an error if a property on the object has an unexpected type.

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

    When the `tsconfig.json` settings have been force-overridden using the new `--tsconfig` flag, the path resolution behavior behaved subtly differently than if esbuild naturally discovers the `tsconfig.json` file without the flag. The difference caused package paths present in a `node_modules` directory to incorrectly take precedence over custom path aliases configured in `tsconfig.json`. The ordering has been corrected such that custom path aliases always take place over `node_modules`.

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

        The `esbuild-wasm` package now has a file called `browser.js` that exposes a `startService()` API which is similar to the esbuild API available in node. You can either import the `esbuild-wasm` package using a bundler that respects the `browser` field in `package.json` or import the `esbuild-wasm/lib/browser.js` file directly.

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

        declare function startService(options: BrowserOptions): Promise<BrowserService>
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

    The `file` loader copies the input file to the output directory and exports the path of the file as a string to any modules that import the file. For example, `--loader:.png=file` enables this loader for all imported `.png` files. This was contributed by [@viankakrisna](https://github.com/viankakrisna).

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
