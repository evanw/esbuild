# Changelog

## 0.11.20

* Omit warning about duplicate JSON keys from inside `node_modules` ([#1254](https://github.com/evanw/esbuild/issues/1254))

    This release no longer warns about duplicate keys inside `package.json` files inside `node_modules`. There are packages like this that are published to npm, and this warning is unactionable. Now esbuild will only issue this warning outside of `node_modules` directories.

* Add CSS minification for `box-shadow` values

    The CSS `box-shadow` property is now minified when `--mangle-syntax` is enabled. This includes trimming length values and minifying color representations.

* Fix object spread transform for non-spread getters ([#1259](https://github.com/evanw/esbuild/issues/1259))

    When transforming an object literal containing object spread (the `...` syntax), properties inside the spread should be evaluated but properties outside the spread should not be evaluated. Previously esbuild's object spread transform incorrectly evaluated properties in both cases. Consider this example:

    ```js
    var obj = {
      ...{ get x() { console.log(1) } },
      get y() { console.log(3) },
    }
    console.log(2)
    obj.y
    ```

    This should print out `1 2 3` because the non-spread getter should not be evaluated. Instead, esbuild was incorrectly transforming this into code that printed `1 3 2`. This issue should now be fixed with this release.

* Prevent private class members from being added more than once

    This fixes a corner case with the private class member implementation. Constructors in JavaScript can return an object other than `this`, so private class members can actually be added to objects other than `this`. This can be abused to attach completely private metadata to other objects:

    ```js
    class Base {
      constructor(x) {
        return x
      }
    }
    class Derived extends Base {
      #y
      static is(z) {
        return #y in z
      }
    }
    const foo = {}
    new Derived(foo)
    console.log(Derived.is(foo)) // true
    ```

    This already worked in code transformed by esbuild for older browsers. However, calling `new Derived(foo)` multiple times in the above code was incorrectly allowed. This should not be allowed because it would mean that the private field `#y` would be re-declared. This is no longer allowed starting from this release.

## 0.11.19

* Allow esbuild to be restarted in Deno ([#1238](https://github.com/evanw/esbuild/pull/1238))

    The esbuild API for [Deno](https://deno.land) has an extra function called `stop()` that doesn't exist in esbuild's API for node. This is because Deno doesn't provide a way to stop esbuild automatically, so calling `stop()` is required to allow Deno to exit. However, once stopped the esbuild API could not be restarted.

    With this release, you can now continue to use esbuild after calling `stop()`. This will restart esbuild's API and means that you will need to call `stop()` again for Deno to be able to exit. This feature was contributed by [@lucacasonato](https://github.com/lucacasonato).

* Fix code splitting edge case ([#1252](https://github.com/evanw/esbuild/issues/1252))

    This release fixes an edge case where bundling with code splitting enabled generated incorrect code if multiple ESM entry points re-exported the same re-exported symbol from a CommonJS file. In this case the cross-chunk symbol dependency should be the variable that holds the return value from the `require()` call instead of the original ESM named `import` clause item. When this bug occurred, the generated ESM code contained an export and import for a symbol that didn't exist, which caused a module initialization error. This case should now work correctly.

* Fix code generation with `declare` class fields ([#1242](https://github.com/evanw/esbuild/issues/1242))

    This fixes a bug with TypeScript code that uses `declare` on a class field and your `tsconfig.json` file has `"useDefineForClassFields": true`. Fields marked as `declare` should not be defined in the generated code, but they were incorrectly being declared as `undefined`. These fields are now correctly omitted from the generated code.

* Annotate module wrapper functions in debug builds ([#1236](https://github.com/evanw/esbuild/pull/1236))

    Sometimes esbuild needs to wrap certain modules in a function when bundling. This is done both for lazy evaluation and for CommonJS modules that use a top-level `return` statement. Previously these functions were all anonymous, so stack traces for errors thrown during initialization looked like this:

    ```
    Error: Electron failed to install correctly, please delete node_modules/electron and try installing again
        at getElectronPath (out.js:16:13)
        at out.js:19:21
        at out.js:1:45
        at out.js:24:3
        at out.js:1:45
        at out.js:29:3
        at out.js:1:45
        at Object.<anonymous> (out.js:33:1)
    ```

    This release adds names to these anonymous functions when minification is disabled. The above stack trace now looks like this:

    ```
    Error: Electron failed to install correctly, please delete node_modules/electron and try installing again
        at getElectronPath (out.js:19:15)
        at node_modules/electron/index.js (out.js:22:23)
        at __require (out.js:2:44)
        at src/base/window.js (out.js:29:5)
        at __require (out.js:2:44)
        at src/base/kiosk.js (out.js:36:5)
        at __require (out.js:2:44)
        at Object.<anonymous> (out.js:41:1)
    ```

    This is similar to Webpack's development-mode behavior:

    ```
    Error: Electron failed to install correctly, please delete node_modules/electron and try installing again
        at getElectronPath (out.js:23:11)
        at Object../node_modules/electron/index.js (out.js:27:18)
        at __webpack_require__ (out.js:96:41)
        at Object../src/base/window.js (out.js:49:1)
        at __webpack_require__ (out.js:96:41)
        at Object../src/base/kiosk.js (out.js:38:1)
        at __webpack_require__ (out.js:96:41)
        at out.js:109:1
        at out.js:111:3
        at Object.<anonymous> (out.js:113:12)
    ```

    These descriptive function names will additionally be available when using a profiler such as the one included in the "Performance" tab in Chrome Developer Tools. Previously all functions were named `(anonymous)` which made it difficult to investigate performance issues during bundle initialization.

* Add CSS minification for more cases

    The following CSS minification cases are now supported:

    * The CSS `margin` property family is now minified including combining the `margin-top`, `margin-right`, `margin-bottom`, and `margin-left` properties into a single `margin` property.

    * The CSS `padding` property family is now minified including combining the `padding-top`, `padding-right`, `padding-bottom`, and `padding-left` properties into a single `padding` property.

    * The CSS `border-radius` property family is now minified including combining the `border-top-left-radius`, `border-top-right-radius`, `border-bottom-right-radius`, and `border-bottom-left-radius` properties into a single `border-radius` property.

    * The four special pseudo-elements `::before`, `::after`, `::first-line`, and `::first-letter` are allowed to be parsed with one `:` for legacy reasons, so the `::` is now converted to `:` for these pseudo-elements.

    * Duplicate CSS rules are now deduplicated. Only the last rule is kept, since that's the only one that has any effect. This applies for both top-level rules and nested rules.

* Preserve quotes around properties when minification is disabled ([#1251](https://github.com/evanw/esbuild/issues/1251))

    Previously the parser did not distinguish between unquoted and quoted properties, since there is no semantic difference. However, some tools such as [Google Closure Compiler](https://developers.google.com/closure/compiler) with "advanced mode" enabled attach their own semantic meaning to quoted properties, and processing code intended for Google Closure Compiler's advanced mode with esbuild was changing those semantics. The distinction between unquoted and quoted properties is now made in the following cases:

    ```js
    import * as ns from 'external-pkg'
    console.log([
      { x: 1, 'y': 2 },
      { x() {}, 'y'() {} },
      class { x = 1; 'y' = 2 },
      class { x() {}; 'y'() {} },
      { x: x, 'y': y } = z,
      [x.x, y['y']],
      [ns.x, ns['y']],
    ])
    ```

    The parser will now preserve the quoted properties in these cases as long as `--minify-syntax` is not enabled. This does not mean that esbuild is officially supporting Google Closure Compiler's advanced mode, just that quoted properties are now preserved when the AST is pretty-printed. Google Closure Compiler's advanced mode accepts a language that shares syntax with JavaScript but that deviates from JavaScript semantics and there could potentially be other situations where preprocessing code intended for Google Closure Compiler's advanced mode with esbuild first causes it to break. If that happens, that is not a bug with esbuild.

## 0.11.18

* Add support for OpenBSD on x86-64 ([#1235](https://github.com/evanw/esbuild/issues/1235))

    Someone has asked for OpenBSD to be supported on x86-64. It should now be supported starting with this release.

* Fix an incorrect warning about top-level `this`

    This was introduced in the previous release, and happens when using a top-level `async` arrow function with a compilation target that doesn't support it. The reason is that doing this generates a shim that preserves the value of `this`. However, this warning message is confusing because there is not necessarily any `this` present in the source code. The warning message has been removed in this case. Now it should only show up if `this` is actually present in the source code.

## 0.11.17

* Fix building with a large `stdin` string with Deno ([#1219](https://github.com/evanw/esbuild/issues/1219))

    When I did the initial port of esbuild's node-based API to Deno, I didn't realize that Deno's `write(bytes)` function doesn't actually write the provided bytes. Instead it may only write some of those bytes and needs to be repeatedly called again until it writes everything. This meant that calling esbuild's Deno-based API could hang if the API request was large enough, which can happen in practice when using the `stdin` string feature. The `write` API is now called in a loop so these hangs in Deno should now be fixed.

* Add a warning about replacing `this` with `undefined` in ESM code ([#1225](https://github.com/evanw/esbuild/issues/1225))

    There is existing JavaScript code that sometimes references top-level `this` as a way to access the global scope. However, top-level `this` is actually specified to be `undefined` inside of ECMAScript module code, which makes referencing top-level `this` inside ESM code useless. This issue can come up when the existing JavaScript code is adapted for ESM by adding `import` and/or `export`. All top-level references to `this` are replaced with `undefined` when bundling to make sure ECMAScript module behavior is emulated correctly regardless of the environment in which the resulting code is run.

    With this release, esbuild will now warn about this when bundling:

    ```
     > example.mjs:1:61: warning: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
        1 │ export let Array = (typeof window !== 'undefined' ? window : this).Array
          ╵                                                              ~~~~
       example.mjs:1:0: note: This file is considered an ECMAScript module because of the "export" keyword here
        1 │ export let Array = (typeof window !== 'undefined' ? window : this).Array
          ╵ ~~~~~~
    ```

    This warning is not unique to esbuild. Rollup also already has a similar warning:

    ```
    (!) `this` has been rewritten to `undefined`
    https://rollupjs.org/guide/en/#error-this-is-undefined
    example.mjs
    1: export let Array = (typeof window !== 'undefined' ? window : this).Array
                                                                    ^
    ```

* Allow a string literal as a JSX fragment ([#1217](https://github.com/evanw/esbuild/issues/1217))

    TypeScript's JSX implementation allows you to configure a custom JSX factory and a custom JSX fragment, but requires that they are both valid JavaScript identifier member expression chains. Since esbuild's JSX implementation is based on TypeScript, esbuild has the same requirement. So `React.createElement` is a valid JSX factory value but `['React', 'createElement']` is not.

    However, the [Mithril](https://mithril.js.org/jsx.html) framework has decided to use `"["` as a JSX fragment, which is not a valid JavaScript identifier member expression chain. This meant that using Mithril with esbuild required a workaround. In this release, esbuild now lets you use a string literal as a custom JSX fragment. It should now be easier to use esbuild's JSX implementation with libraries such as Mithril.

* Fix `metafile` in `onEnd` with `watch` mode enabled ([#1186](https://github.com/evanw/esbuild/issues/1186))

    This release fixes a bug where the `metafile` property was incorrectly undefined inside plugin `onEnd` callbacks if `watch` mode is enabled for all builds after the first build. The `metafile` property was accidentally being set after calling `onEnd` instead of before.

## 0.11.16

* Fix TypeScript `enum` edge case ([#1198](https://github.com/evanw/esbuild/issues/1198))

    In TypeScript, you can reference the inner closure variable in an `enum` within the inner closure by name:

    ```ts
    enum A { B = A }
    ```

    The TypeScript compiler generates the following code for this case:

    ```ts
    var A;
    (function (A) {
      A[A["B"] = A] = "B";
    })(A || (A = {}));
    ```

    However, TypeScript also lets you declare an `enum` value with the same name as the inner closure variable. In that case, the value "shadows" the declaration of the inner closure variable:

    ```ts
    enum A { A = 1, B = A }
    ```

    The TypeScript compiler generates the following code for this case:

    ```ts
    var A;
    (function (A) {
      A[A["A"] = 1] = "A";
      A[A["B"] = 1] = "B";
    })(A || (A = {}));
    ```

    Previously esbuild reported a duplicate variable declaration error in the second case due to the collision between the `enum` value and the inner closure variable with the same name. With this release, the shadowing is now handled correctly.

* Parse the `@-moz-document` CSS rule ([#1203](https://github.com/evanw/esbuild/issues/1203))

    This feature has been removed from the web because it's actively harmful, at least according to [this discussion](https://bugzilla.mozilla.org/show_bug.cgi?id=1035091). However, there is one exception where `@-moz-document url-prefix() {` is accepted by Firefox to basically be an "if Firefox" conditional rule. Because of this, esbuild now parses the `@-moz-document` CSS rule. This should result in better pretty-printing and minification and no more warning when this rule is used.

* Fix syntax error in TypeScript-specific speculative arrow function parsing ([#1211](https://github.com/evanw/esbuild/issues/1211))

    Because of grammar ambiguities, expressions that start with a parenthesis are parsed using what's called a "cover grammar" that is a super-position of both a parenthesized expression and an arrow function parameter list. In JavaScript, the cover grammar is unambiguously an arrow function if and only if the following token is a `=>` token.

    But in TypeScript, the expression is still ambiguously a parenthesized expression or an arrow function if the following token is a `:` since it may be the second half of the `?:` operator or a return type annotation. This requires speculatively attempting to reduce the cover grammar to an arrow function parameter list.

    However, when doing this esbuild eagerly reported an error if a default argument was encountered and the target is `es5` (esbuild doesn't support lowering default arguments to ES5). This is problematic in the following TypeScript code since the parenthesized code turns out to not be an arrow function parameter list:

    ```ts
    function foo(check, hover) {
      return check ? (hover = 2, bar) : baz();
    }
    ```

    Previously this code incorrectly generated an error since `hover = 2` was incorrectly eagerly validated as a default argument. With this release, the reporting of the default argument error when targeting `es5` is now done lazily and only when it's determined that the parenthesized code should actually be interpreted as an arrow function parameter list.

* Further changes to the behavior of the `browser` field ([#1209](https://github.com/evanw/esbuild/issues/1209))

    This release includes some changes to how the `browser` field in `package.json` is interpreted to better match how Browserify, Webpack, Parcel, and Rollup behave. The interpretation of this map in esbuild is intended to be applied if and only if it's applied by any one of these bundlers. However, there were some cases where esbuild applied the mapping and none of the other bundlers did, which could lead to build failures. These cases have been added to my [growing list of `browser` field test cases](https://github.com/evanw/package-json-browser-tests) and esbuild's behavior should now be consistent with other bundlers again.

* Avoid placing a `super()` call inside a `return` statement ([#1208](https://github.com/evanw/esbuild/issues/1208))

    When minification is enabled, an expression followed by a return statement (e.g. `a(); return b`) is merged into a single statement (e.g. `return a(), b`). This is done because it sometimes results in smaller code. If the return statement is the only statement in a block and the block is in a single-statement context, the block can be removed which saves a few characters.

    Previously esbuild applied this rule to calls to `super()` inside of constructors. Doing that broke esbuild's class lowering transform that tries to insert class field initializers after the `super()` call. This transform isn't robust and only scans the top-level statement list inside the constructor, so inserting the `super()` call inside of the `return` statement means class field initializers were inserted before the `super()` call instead of after. This could lead to run-time crashes due to initialization failure.

    With this release, top-level calls to `super()` will no longer be placed inside `return` statements (in addition to various other kinds of statements such as `throw`, which are now also handled). This should avoid class field initializers being inserted before the `super()` call.

* Fix a bug with `onEnd` and watch mode ([#1186](https://github.com/evanw/esbuild/issues/1186))

    This release fixes a bug where `onEnd` plugin callbacks only worked with watch mode when an `onRebuild` watch mode callback was present. Now `onEnd` callbacks should fire even if there is no `onRebuild` callback.

* Fix an edge case with minified export names and code splitting ([#1201](https://github.com/evanw/esbuild/issues/1201))

    The names of symbols imported from other chunks were previously not considered for renaming during minified name assignment. This could cause a syntax error due to a name collision when two symbols have the same original name. This was just an oversight and has been fixed, so symbols imported from other chunks should now be renamed when minification is enabled.

* Provide a friendly error message when you forget `async` ([#1216](https://github.com/evanw/esbuild/issues/1216))

    If the parser hits a parse error inside a non-asynchronous function or arrow expression and the previous token is `await`, esbuild will now report a friendly error about a missing `async` keyword instead of reporting the parse error. This behavior matches other JavaScript parsers including TypeScript, Babel, and V8.

    The previous error looked like this:

    ```
     > test.ts:2:8: error: Expected ";" but found "f"
        2 │   await f();
          ╵         ^
    ```

    The error now looks like this:

    ```
     > example.js:2:2: error: "await" can only be used inside an "async" function
        2 │   await f();
          ╵   ~~~~~
       example.js:1:0: note: Consider adding the "async" keyword here
        1 │ function f() {
          │ ^
          ╵ async
    ```

## 0.11.15

* Provide options for how to handle legal comments ([#919](https://github.com/evanw/esbuild/issues/919))

    A "legal comment" is considered to be any comment that contains `@license` or `@preserve` or that starts with `//!` or `/*!`. These comments are preserved in output files by esbuild since that follows the intent of the original authors of the code.

    However, some people want to remove the automatically-generated license information before they distribute their code. To facilitate this, esbuild now provides several options for how to handle legal comments (via `--legal-comments=` in the CLI and `legalComments` in the JS API):

    * `none`: Do not preserve any legal comments
    * `inline`: Preserve all statement-level legal comments
    * `eof`: Move all statement-level legal comments to the end of the file
    * `linked`: Move all statement-level legal comments to a `.LEGAL.txt` file and link to them with a comment
    * `external`: Move all statement-level legal comments to a `.LEGAL.txt` file but to not link to them

    The default behavior is `eof` when bundling and `inline` otherwise.

* Add `onStart` and `onEnd` callbacks to the plugin API

    Plugins can now register callbacks to run when a build is started and ended:

    ```js
    const result = await esbuild.build({
      ...
      incremental: true,
      plugins: [{
        name: 'example',
        setup(build) {
          build.onStart(() => console.log('build started'))
          build.onEnd(result => console.log('build ended', result))
        },
      }],
    })
    await result.rebuild()
    ```

    One benefit of `onStart` and `onEnd` is that they are run for all builds including rebuilds (relevant for incremental mode, watch mode, or serve mode), so they should be a good place to do work related to the build lifecycle.

    More details:

    * `build.onStart()`

        You should not use an `onStart` callback for initialization since it can be run multiple times. If you want to initialize something, just put your plugin initialization code directly inside the `setup` function instead.

        The `onStart` callback can be `async` and can return a promise. However, the build does not wait for the promise to be resolved before starting, so a slow `onStart` callback will not necessarily slow down the build. All `onStart` callbacks are also run concurrently, not consecutively. The returned promise is purely for error reporting, and matters when the `onStart` callback needs to do an asynchronous operation that may fail. If your plugin needs to wait for an asynchronous task in `onStart` to complete before any `onResolve` or `onLoad` callbacks are run, you will need to have your `onResolve` or `onLoad` callbacks block on that task from `onStart`.

        Note that `onStart` callbacks do not have the ability to mutate `build.initialOptions`. The initial options can only be modified within the `setup` function and are consumed once the `setup` function returns. All rebuilds use the same initial options so the initial options are never re-consumed, and modifications to `build.initialOptions` that are done within `onStart` are ignored.

    * `build.onEnd()`

        All `onEnd` callbacks are run in serial and each callback is given access to the final build result. It can modify the build result before returning and can delay the end of the build by returning a promise. If you want to be able to inspect the build graph, you should set `build.initialOptions.metafile = true` and the build graph will be returned as the `metafile` property on the build result object.

## 0.11.14

* Implement arbitrary module namespace identifiers

    This introduces new JavaScript syntax:

    ```js
    import {'🍕' as food} from 'file'
    export {food as '🧀'}
    ```

    [The proposal for this feature](https://github.com/bmeck/proposal-arbitrary-module-namespace-identifiers) appears to not be going through the regular TC39 process. It is being done as a subtle [direct pull request](https://github.com/tc39/ecma262/pull/2154) instead. It seems appropriate for esbuild to support this feature since it has been implemented in V8 and has now shipped in Chrome 90 and node 16.

    According to the proposal, this feature is intended to improve interop with non-JavaScript languages which use exports that aren't valid JavaScript identifiers such as `Foo::~Foo`. In particular, WebAssembly allows any valid UTF-8 string as to be used as an export alias.

    This feature was actually already partially possible in previous versions of JavaScript via the computed property syntax:

    ```js
    import * as ns from './file.json'
    console.log(ns['🍕'])
    ```

    However, doing this is very un-ergonomic and exporting something as an arbitrary name is impossible outside of `export * from`. So this proposal is designed to fully fill out the possibility matrix and make arbitrary alias names a proper first-class feature.

* Implement more accurate `sideEffects` behavior from Webpack ([#1184](https://github.com/evanw/esbuild/issues/1184))

    This release adds support for the implicit `**/` prefix that must be added to paths in the `sideEffects` array in `package.json` if the path does not contain `/`. Another way of saying this is if `package.json` contains a `sideEffects` array with a string that doesn't contain a `/` then it should be treated as a file name instead of a path. Previously esbuild treated all strings in this array as paths, which does not match how Webpack behaves. The result of this meant that esbuild could consider files to have no side effects while Webpack would consider the same files to have side effects. This bug should now be fixed.

## 0.11.13

* Implement ergonomic brand checks for private fields

    This introduces new JavaScript syntax:

    ```js
    class Foo {
      #field
      static isFoo(x) {
        return #foo in x // This is an "ergonomic brand check"
      }
    }
    assert(Foo.isFoo(new Foo))
    ```

    [The TC39 proposal for this feature](https://github.com/tc39/proposal-private-fields-in-in) is currently at stage 3 but has already been shipped in Chrome 91 and has also landed in Firefox. It seems reasonably inevitable given that it's already shipping and that it's a very simple feature, so it seems appropriate to add this feature to esbuild.

* Add the `--allow-overwrite` flag ([#1152](https://github.com/evanw/esbuild/issues/1152))

    This is a new flag that allows output files to overwrite input files. It's not enabled by default because doing so means overwriting your source code, which can lead to data loss if your code is not checked in. But supporting this makes certain workflows easier by avoiding the need for a temporary directory so doing this is now supported.

* Minify property accesses on object literals ([#1166](https://github.com/evanw/esbuild/issues/1166))

    The code `{a: {b: 1}}.a.b` will now be minified to `1`. This optimization is relatively complex and hard to do safely. Here are some tricky cases that are correctly handled:

    ```js
    var obj = {a: 1}
    assert({a: 1, a: 2}.a === 2)
    assert({a: 1, [String.fromCharCode(97)]: 2}.a === 2)
    assert({__proto__: obj}.a === 1)
    assert({__proto__: null}.a === undefined)
    assert({__proto__: null}.__proto__ === undefined)
    assert({a: function() { return this.b }, b: 1}.a() === 1)
    assert(({a: 1}.a = 2) === 2)
    assert(++{a: 1}.a === 2)
    assert.throws(() => { new ({ a() {} }.a) })
    ```

* Improve arrow function parsing edge cases

    There are now more situations where arrow expressions are not allowed. This improves esbuild's alignment with the JavaScript specification. Some examples of cases that were previously allowed but that are now no longer allowed:

    ```js
    1 + x => {}
    console.log(x || async y => {})
    class Foo extends async () => {} {}
    ```

## 0.11.12

* Fix a bug where `-0` and `0` were collapsed to the same value ([#1159](https://github.com/evanw/esbuild/issues/1159))

    Previously esbuild would collapse `Object.is(x ? 0 : -0, -0)` into `Object.is((x, 0), -0)` during minification, which is incorrect. The IEEE floating-point value `-0` is a different bit pattern than `0` and while they both compare equal, the difference is detectable in a few scenarios such as when using `Object.is()`. The minification transformation now checks for `-0` vs. `0` and no longer has this bug. This fix was contributed by [@rtsao](https://github.com/rtsao).

* Match the TypeScript compiler's output in a strange edge case ([#1158](https://github.com/evanw/esbuild/issues/1158))

    With this release, esbuild's TypeScript-to-JavaScript transform will no longer omit the namespace in this case:

    ```ts
    namespace Something {
      export declare function Print(a: string): void
    }
    Something.Print = function(a) {}
    ```

    This was previously omitted because TypeScript omits empty namespaces, and the namespace was considered empty because the `export declare function` statement isn't "real":

    ```ts
    namespace Something {
      export declare function Print(a: string): void
      setTimeout(() => Print('test'))
    }
    Something.Print = function(a) {}
    ```

    The TypeScript compiler compiles the above code into the following:

    ```js
    var Something;
    (function (Something) {
      setTimeout(() => Print('test'));
    })(Something || (Something = {}));
    Something.Print = function (a) { };
    ```

    Notice how `Something.Print` is never called, and what appears to be a reference to the `Print` symbol on the namespace `Something` is actually a reference to the global variable `Print`. I can only assume this is a bug in TypeScript, but it's important to replicate this behavior inside esbuild for TypeScript compatibility.

    The TypeScript-to-JavaScript transform in esbuild has been updated to match the TypeScript compiler's output in both of these cases.

* Separate the `debug` log level into `debug` and `verbose`

    You can now use `--log-level=debug` to get some additional information that might indicate some problems with your build, but that has a high-enough false-positive rate that it isn't appropriate for warnings, which are on by default. Enabling the `debug` log level no longer generates a torrent of debug information like it did in the past; that behavior is now reserved for the `verbose` log level instead.

## 0.11.11

* Initial support for Deno ([#936](https://github.com/evanw/esbuild/issues/936))

    You can now use esbuild in the [Deno](https://deno.land/) JavaScript environment via esbuild's official Deno package. Using it looks something like this:

    ```js
    import * as esbuild from 'https://deno.land/x/esbuild@v0.11.11/mod.js'
    const ts = 'let hasProcess: boolean = typeof process != "null"'
    const result = await esbuild.transform(ts, { loader: 'ts', logLevel: 'warning' })
    console.log('result:', result)
    esbuild.stop()
    ```

    It has basically the same API as esbuild's npm package with one addition: you need to call `stop()` when you're done because unlike node, Deno doesn't provide the necessary APIs to allow Deno to exit while esbuild's internal child process is still running.

* Remove warnings about non-bundled use of `require` and `import` ([#1153](https://github.com/evanw/esbuild/issues/1153), [#1142](https://github.com/evanw/esbuild/issues/1142), [#1132](https://github.com/evanw/esbuild/issues/1132), [#1045](https://github.com/evanw/esbuild/issues/1045), [#812](https://github.com/evanw/esbuild/issues/812), [#661](https://github.com/evanw/esbuild/issues/661), [#574](https://github.com/evanw/esbuild/issues/574), [#512](https://github.com/evanw/esbuild/issues/512), [#495](https://github.com/evanw/esbuild/issues/495), [#480](https://github.com/evanw/esbuild/issues/480), [#453](https://github.com/evanw/esbuild/issues/453), [#410](https://github.com/evanw/esbuild/issues/410), [#80](https://github.com/evanw/esbuild/issues/80))

    Previously esbuild had warnings when bundling about uses of `require` and `import` that are not of the form `require(<string literal>)` or `import(<string literal>)`. These warnings existed because the bundling process must be able to statically-analyze all dynamic imports to determine which files must be included. Here are some real-world examples of cases that esbuild doesn't statically analyze:

    * From [`mongoose`](https://www.npmjs.com/package/mongoose):

        ```js
        require('./driver').set(require(global.MONGOOSE_DRIVER_PATH));
        ```

    * From [`moment`](https://www.npmjs.com/package/moment):

        ```js
        aliasedRequire = require;
        aliasedRequire('./locale/' + name);
        ```

    * From [`logform`](https://www.npmjs.com/package/logform):

        ```js
        function exposeFormat(name) {
          Object.defineProperty(format, name, {
            get() { return require(`./${name}.js`); }
          });
        }
        exposeFormat('align');
        ```

    All of these dynamic imports will not be bundled (i.e. they will be left as-is) and will crash at run-time if they are evaluated. Some of these crashes are ok since the code paths may have error handling or the code paths may never be used. Other crashes are not ok because the crash will actually be hit.

    The warning from esbuild existed to let you know that esbuild is aware that it's generating a potentially broken bundle. If you discover that your bundle is broken, it's nice to have a warning from esbuild to point out where the problem is. And it was just a warning so the build process still finishes and successfully generates output files. If you didn't want to see the warning, it was easy to turn it off via `--log-level=error`.

    However, there have been quite a few complaints about this warning. Some people seem to not understand the difference between a warning and an error, and think the build has failed even though output files were generated. Other people do not want to see the warning but also do not want to enable `--log-level=error`.

    This release removes this warning for both `require` and `import`. Now when you try to bundle code with esbuild that contains dynamic imports not of the form `require(<string literal>)` or `import(<string literal>)`, esbuild will just silently generate a potentially broken bundle. This may affect people coming from other bundlers that support certain forms of dynamic imports that are not compatible with esbuild such as the [Webpack-specific dynamic `import()` with pattern matching](https://webpack.js.org/api/module-methods/#dynamic-expressions-in-import).

## 0.11.10

* Provide more information about `exports` map import failures if possible ([#1143](https://github.com/evanw/esbuild/issues/1143))

    Node has a new feature where you can [add an `exports` map to your `package.json` file](https://nodejs.org/api/packages.html#packages_package_entry_points) to control how external import paths map to the files in your package. You can change which paths map to which files as well as make it impossible to import certain files (i.e. the files are private).

    If path resolution fails due to an `exports` map and the failure is not related to import conditions, esbuild's current error message for this just says that the import isn't possible:

    ```
     > example.js:1:15: error: Could not resolve "vanillajs-datepicker/js/i18n/locales/ca" (mark it as external to exclude it from the bundle)
        1 │ import ca from 'vanillajs-datepicker/js/i18n/locales/ca'
          ╵                ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
       node_modules/vanillajs-datepicker/package.json:6:13: note: The path "./js/i18n/locales/ca" is not exported by package "vanillajs-datepicker"
        6 │   "exports": {
          ╵              ^
    ```

    This error message matches the error that node itself throws. However, the message could be improved in the case where someone is trying to import a file using its file system path and that path is actually exported by the package, just under a different export path. This case comes up a lot when using TypeScript because the TypeScript compiler (and therefore the Visual Studio Code IDE) [still doesn't support package `exports`](https://github.com/microsoft/TypeScript/issues/33079).

    With this release, esbuild will now do a reverse lookup of the file system path using the `exports` map to determine what the correct import path should be:

    ```
     > example.js:1:15: error: Could not resolve "vanillajs-datepicker/js/i18n/locales/ca" (mark it as external to exclude it from the bundle)
         1 │ import ca from 'vanillajs-datepicker/js/i18n/locales/ca'
           ╵                ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
       node_modules/vanillajs-datepicker/package.json:6:13: note: The path "./js/i18n/locales/ca" is not exported by package "vanillajs-datepicker"
         6 │   "exports": {
           ╵              ^
       node_modules/vanillajs-datepicker/package.json:12:19: note: The file "./js/i18n/locales/ca.js" is exported at path "./locales/ca"
        12 │     "./locales/*": "./js/i18n/locales/*.js",
           ╵                    ~~~~~~~~~~~~~~~~~~~~~~~~
       example.js:1:15: note: Import from "vanillajs-datepicker/locales/ca" to get the file "node_modules/vanillajs-datepicker/js/i18n/locales/ca.js"
         1 │ import ca from 'vanillajs-datepicker/js/i18n/locales/ca'
           │                ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
           ╵                "vanillajs-datepicker/locales/ca"
    ```

    Hopefully this should enable people encountering this issue to fix the problem themselves.

## 0.11.9

* Fix escaping of non-BMP characters in property names ([#977](https://github.com/evanw/esbuild/issues/977))

    Property names in object literals do not have to be quoted if the property is a valid JavaScript identifier. This is defined as starting with a character in the `ID_Start` Unicode category and ending with zero or more characters in the `ID_Continue` Unicode category. However, esbuild had a bug where non-BMP characters (i.e. characters encoded using two UTF-16 code units instead of one) were always checked against `ID_Continue` instead of `ID_Start` because they included a code unit that wasn't at the start. This could result in invalid JavaScript being generated when using `--charset=utf8` because `ID_Continue` is a superset of `ID_Start` and contains some characters that are not valid at the start of an identifier. This bug has been fixed.

* Be maximally liberal in the interpretation of the `browser` field ([#740](https://github.com/evanw/esbuild/issues/740))

    The `browser` field in `package.json` is an informal convention followed by browser-specific bundlers that allows package authors to substitute certain node-specific import paths with alternative browser-specific import paths. It doesn't have a rigorous specification and the [canonical description](https://github.com/defunctzombie/package-browser-field-spec) of the feature doesn't include any tests. As a result, each bundler implements this feature differently. I have tried to create a [survey of how different bundlers interpret the `browser` field](https://github.com/evanw/package-json-browser-tests) and the results are very inconsistent.

    This release attempts to change esbuild to support the union of the behavior of all other bundlers. That way if people have the `browser` field working with some other bundler and they switch to esbuild, the `browser` field shouldn't ever suddenly stop working. This seemed like the most principled approach to take in this situation.

    The drawback of this approach is that it means the `browser` field may start working when switching to esbuild when it was previously not working. This could cause bugs, but I consider this to be a problem with the package (i.e. not using a more well-supported form of the `browser` field), not a problem with esbuild itself.

## 0.11.8

* Fix hash calculation for code splitting and dynamic imports ([#1076](https://github.com/evanw/esbuild/issues/1076))

    The hash included in the file name of each output file is intended to change if and only if anything relevant to the content of that output file changes. It includes:

    * The contents of the file with the paths of other output files omitted
    * The output path of the file the final hash omitted
    * Some information about the input files involved in that output file
    * The contents of the associated source map, if there is one
    * All of the information above for all transitive dependencies found by following `import` statements

    However, this didn't include dynamic `import()` expressions due to an oversight. With this release, dynamic `import()` expressions are now also counted as transitive dependencies. This fixes an issue where the content of an output file could change without its hash also changing. As a side effect of this change, dynamic imports inside output files of other output files are now listed in the metadata file if the `metafile` setting is enabled.

* Refactor the internal module graph representation

    This release changes a large amount of code relating to esbuild's internal module graph. The changes are mostly organizational and help consolidate most of the logic around maintaining various module graph invariants into a separate file where it's easier to audit. The Go language doesn't have great abstraction capabilities (e.g. no zero-cost iterators) so the enforcement of this new abstraction is unfortunately done by convention instead of by the compiler, and there is currently still some code that bypasses the abstraction. But it's better than it was before.

    Another relevant change was moving a number of special cases that happened during the tree shaking traversal into the graph itself instead. Previously there were quite a few implicit dependency rules that were checked in specific places, which was hard to follow. Encoding these special case constraints into the graph itself makes the problem easier to reason about and should hopefully make the code more regular and robust.

    Finally, this set of changes brings back full support for the `sideEffects` annotation in `package.json`. It was previously disabled when code splitting was active as a temporary measure due to the discovery of some bugs in that scenario. But I believe these bugs have been resolved now that tree shaking and code splitting are done in separate passes (see the previous release for more information).

## 0.11.7

* Fix incorrect chunk reference with code splitting, css, and dynamic imports ([#1125](https://github.com/evanw/esbuild/issues/1125))

    This release fixes a bug where when you use code splitting, CSS imports in JS, and dynamic imports all combined, the dynamic import incorrectly references the sibling CSS chunk for the dynamic import instead of the primary JS chunk. In this scenario the entry point file corresponds to two different output chunks (one for CSS and one for JS) and the wrong chunk was being picked. This bug has been fixed.

* Split apart tree shaking and code splitting ([#1123](https://github.com/evanw/esbuild/issues/1123))

    The original code splitting algorithm allowed for files to be split apart and for different parts of the same file to end up in different chunks based on which entry points needed which parts. This was done at the same time as tree shaking by essentially performing tree shaking multiple times, once per entry point, and tracking which entry points each file part is live in. Each file part that is live in at least one entry point was then assigned to a code splitting chunk with all of the other code that is live in the same set of entry points. This ensures that entry points only import code that they will use (i.e. no code will be downloaded by an entry point that is guaranteed to not be used).

    This file-splitting feature has been removed because it doesn't work well with the recently-added top-level await JavaScript syntax, which has complex evaluation order rules that operate at file boundaries. File parts now have a single boolean flag for whether they are live or not instead of a set of flags that track which entry points that part is reachable from (reachability is still tracked at the file level).

    However, this change appears to have introduced some subtly incorrect behavior with code splitting because there is now an implicit dependency in the import graph between adjacent parts within the same file even if the two parts are unrelated and don't reference each other. This is due to the fact each entry point that references one part pulls in the file (but not the whole file, only the parts that are live in at least one entry point). So liveness must be fully computed first before code splitting is computed.

    This release splits apart tree shaking and code splitting into two separate passes, which fixes certain cases where two generated code splitting chunks ended up each importing symbols from the other and causing a cycle. There should hopefully no longer be cycles in generated code splitting chunks.

* Make `this` work in static class fields in TypeScript files

    Currently `this` is mis-compiled in static fields in TypeScript files if the `useDefineForClassFields` setting in `tsconfig.json` is `false` (the default value):

    ```js
    class Foo {
      static foo = 123
      static bar = this.foo
    }
    console.log(Foo.bar)
    ```

    This is currently compiled into the code below, which is incorrect because it changes the value of `this` (it's supposed to refer to `Foo`):

    ```js
    class Foo {
    }
    Foo.foo = 123;
    Foo.bar = this.foo;
    console.log(Foo.bar);
    ```

    This was an intentionally unhandled case because the TypeScript compiler doesn't handle this either (esbuild's currently incorrect output matches the output from the TypeScript compiler, which is also currently incorrect). However, the TypeScript compiler might fix their output at some point in which case esbuild's behavior would become problematic.

    So this release now generates the correct output:

    ```js
    const _Foo = class {
    };
    let Foo = _Foo;
    Foo.foo = 123;
    Foo.bar = _Foo.foo;
    console.log(Foo.bar);
    ```

    Presumably the TypeScript compiler will be fixed to also generate something like this in the future. If you're wondering why esbuild generates the extra `_Foo` variable, it's defensive code to handle the possibility of the class being reassigned, since class declarations are not constants:

    ```js
    class Foo {
      static foo = 123
      static bar = () => Foo.foo
    }
    let bar = Foo.bar
    Foo = { foo: 321 }
    console.log(bar())
    ```

    We can't just move the initializer containing `Foo.foo` outside of the class body because in JavaScript, the class name is shadowed inside the class body by a special hidden constant that is equal to the class object. Even if the class is reassigned later, references to that shadowing symbol within the class body should still refer to the original class object.

* Various fixes for private class members ([#1131](https://github.com/evanw/esbuild/issues/1131))

    This release fixes multiple issues with esbuild's handling of the `#private` syntax. Previously there could be scenarios where references to `this.#private` could be moved outside of the class body, which would cause them to become invalid (since the `#private` name is only available within the class body). One such case is when TypeScript's `useDefineForClassFields` setting has the value `false` (which is the default value), which causes class field initializers to be replaced with assignment expressions to avoid using "define" semantics:

    ```js
    class Foo {
      static #foo = 123
      static bar = Foo.#foo
    }
    ```

    Previously this was turned into the following code, which is incorrect because `Foo.#foo` was moved outside of the class body:

    ```js
    class Foo {
      static #foo = 123;
    }
    Foo.bar = Foo.#foo;
    ```

    This is now handled by converting the private field syntax into normal JavaScript that emulates it with a `WeakMap` instead.

    This conversion is fairly conservative to make sure certain edge cases are covered, so this release may unfortunately convert more private fields than previous releases, even when the target is `esnext`. It should be possible to improve this transformation in future releases so that this happens less often while still preserving correctness.

## 0.11.6

* Fix an incorrect minification transformation ([#1121](https://github.com/evanw/esbuild/issues/1121))

    This release removes an incorrect substitution rule in esbuild's peephole optimizer, which is run when minification is enabled. The incorrect rule transformed `if(a && falsy)` into `if(a, falsy)` which is equivalent if `falsy` has no side effects (such as the literal `false`). However, the rule didn't check that the expression is side-effect free first which could result in miscompiled code. I have removed the rule instead of modifying it to check for the lack of side effects first because while the code is slightly smaller, it may also be more expensive at run-time which is undesirable. The size savings are also very insignificant.

* Change how `NODE_PATH` works to match node ([#1117](https://github.com/evanw/esbuild/issues/1117))

    Node searches for packages in nearby `node_modules` directories, but it also allows you to inject extra directories to search for packages in using the `NODE_PATH` environment variable. This is supported when using esbuild's CLI as well as via the `nodePaths` option when using esbuild's API.

    Node's module resolution algorithm is well-documented, and esbuild's path resolution is designed to follow it. The full algorithm is here: https://nodejs.org/api/modules.html#modules_all_together. However, it appears that the documented algorithm is incorrect with regard to `NODE_PATH`. The documentation says `NODE_PATH` directories should take precedence over `node_modules` directories, and so that's how esbuild worked. However, in practice node actually does it the other way around.

    Starting with this release, esbuild will now allow `node_modules` directories to take precedence over `NODE_PATH` directories. This is a deviation from the published algorithm.

* Provide a better error message for incorrectly-quoted JSX attributes ([#959](https://github.com/evanw/esbuild/issues/959), [#1115](https://github.com/evanw/esbuild/issues/1115))

    People sometimes try to use the output of `JSON.stringify()` as a JSX attribute when automatically-generating JSX code. Doing so is incorrect because JSX strings work like XML instead of like JS (since JSX is XML-in-JS). Specifically, using a backslash before a quote does not cause it to be escaped:

    ```jsx
    //     JSX ends the "content" attribute here and sets "content" to 'some so-called \\'
    //                                            v
    let button = <Button content="some so-called \"button text\"" />
    //                                                        ^
    //         There is no "=" after the JSX attribute "text", so we expect a ">"
    ```

    It's not just esbuild; Babel and TypeScript also treat this as a syntax error. All of these JSX parsers are just following [the JSX specification](https://facebook.github.io/jsx/). This has come up twice now so it could be worth having a dedicated error message. Previously esbuild had a generic syntax error like this:

    ```
     > example.jsx:1:58: error: Expected ">" but found "\\"
        1 │ let button = <Button content="some so-called \"button text\"" />
          ╵                                                           ^
    ```

    Now esbuild will provide more information if it detects this case:

    ```
     > example.jsx:1:58: error: Unexpected backslash in JSX element
        1 │ let button = <Button content="some so-called \"button text\"" />
          ╵                                                           ^
       example.jsx:1:45: note: Quoted JSX attributes use XML-style escapes instead of JavaScript-style escapes
        1 │ let button = <Button content="some so-called \"button text\"" />
          │                                              ~~
          ╵                                              &quot;
       example.jsx:1:29: note: Consider using a JavaScript string inside {...} instead of a quoted JSX attribute
        1 │ let button = <Button content="some so-called \"button text\"" />
          │                              ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
          ╵                              {"some so-called \"button text\""}
    ```

## 0.11.5

* Add support for the `override` keyword in TypeScript 4.3 ([#1105](https://github.com/evanw/esbuild/pull/1105))

    The latest version of TypeScript (now in beta) adds a new keyword called `override` that can be used on class members. You can read more about this feature in [Microsoft's blog post about TypeScript 4.3](https://devblogs.microsoft.com/typescript/announcing-typescript-4-3-beta/#override-and-the-noimplicitoverride-flag). It looks like this:

    ```ts
    class SpecializedComponent extends SomeComponent {
      override show() {
        // ...
      }
    }
    ```

    With this release, esbuild will now ignore the `override` keyword when parsing TypeScript code instead of treating this keyword as a syntax error, which means esbuild can now support TypeScript 4.3 syntax. This change was contributed by [@g-plane](https://github.com/g-plane).

* Allow `async` plugin `setup` functions

    With this release, you can now return a promise from your plugin's `setup` function to delay the start of the build:

    ```js
    let slowInitPlugin = {
      name: 'slow-init',
      async setup(build) {
        // Delay the start of the build
        await new Promise(r => setTimeout(r, 1000))
      },
    }
    ```

    This is useful if your plugin needs to do something asynchronous before the build starts. For example, you may need some asynchronous information before modifying the `initialOptions` object, which must be done before the build starts for the modifications to take effect.

* Add some optimizations around hashing

    This release contains two optimizations to the hashes used in output file names:

    1. Hash generation now happens in parallel with other work, and other work only blocks on the hash computation if the hash ends up being needed (which is only if `[hash]` is included in `--entry-names=`, and potentially `--chunk-names=` if it's relevant). This is a performance improvement because `--entry-names=` does not include `[hash]` in the default case, so bundling time no longer always includes hashing time.

    2. The hashing algorithm has been changed from SHA1 to [xxHash](https://github.com/Cyan4973/xxHash) (specifically [this Go implementation](https://github.com/cespare/xxhash)) which means the hashing step is around 6x faster than before. Thanks to [@Jarred-Sumner](https://github.com/Jarred-Sumner) for the suggestion.

* Disable tree shaking annotations when code splitting is active ([#1070](https://github.com/evanw/esbuild/issues/1070), [#1081](https://github.com/evanw/esbuild/issues/1081))

    Support for [Webpack's `"sideEffects": false` annotation](https://webpack.js.org/guides/tree-shaking/#mark-the-file-as-side-effect-free) in `package.json` is now disabled when code splitting is enabled and there is more than one entry point. This avoids a bug that could cause generated chunks to reference each other in some cases. Now all chunks generated by code splitting should be acyclic.

## 0.11.4

* Avoid name collisions with TypeScript helper functions ([#1102](https://github.com/evanw/esbuild/issues/1102))

    Helper functions are sometimes used when transforming newer JavaScript syntax for older browsers. For example, `let {x, ...y} = {z}` is transformed into `let _a = {z}, {x} = _a, y = __rest(_a, ["x"])` which uses the `__rest` helper function. Many of esbuild's transforms were modeled after the transforms in the TypeScript compiler, so many of the helper functions use the same names as TypeScript's helper functions.

    However, the TypeScript compiler doesn't avoid name collisions with existing identifiers in the transformed code. This means that post-processing esbuild's output with the TypeScript compiler (e.g. for lowering ES6 to ES5) will cause issues since TypeScript will fail to call its own helper functions: [microsoft/TypeScript#43296](https://github.com/microsoft/TypeScript/issues/43296). There is also a problem where TypeScript's `tslib` library overwrites globals with these names, which can overwrite esbuild's helper functions if code bundled with esbuild is run in the global scope.

    To avoid these problems, esbuild will now use different names for its helper functions.

* Fix a chunk hashing issue ([#1099](https://github.com/evanw/esbuild/issues/1099))

    Previously the chunk hashing algorithm skipped hashing entry point chunks when the `--entry-names=` setting doesn't contain `[hash]`, since the hash wasn't used in the file name. However, this is no longer correct with the change in version 0.11.0 that made dynamic entry point chunks use `--chunk-names=` instead of `--entry-names=` since `--chunk-names=` can still contain `[hash]`.

    With this release, chunk contents will now always be hashed regardless of the chunk type. This makes esbuild somewhat slower than before in the common case, but it fixes this correctness issue.

## 0.11.3

* Auto-define `process.env.NODE_ENV` when platform is set to `browser`

    All code in the React world has the requirement that the specific expression `process.env.NODE_ENV` must be replaced with a string at compile-time or your code will immediately crash at run-time. This is a common stumbling point for people when they start using esbuild with React. Previously bundling code with esbuild containing `process.env.NODE_ENV` without defining a string replacement first was a warning that warned you about the lack of a define.

    With this release esbuild will now attempt to define `process.env.NODE_ENV` automatically instead of warning about it. This will be implicitly defined to `"production"` if minification is enabled and `"development"` otherwise. This automatic behavior only happens when the platform is `browser`, since `process` is not a valid browser API and will never exist in the browser. This is also only done if there are no existing defines for `process`, `process.env`, or `process.env.NODE_ENV` so you can override the automatic value if necessary. If you need to disable this behavior, you can use the `neutral` platform instead of the `browser` platform.

* Retain side-effect free intermediate re-exporting files ([#1088](https://github.com/evanw/esbuild/issues/1088))

    This fixes a subtle bug with esbuild's support for [Webpack's `"sideEffects": false` annotation](https://webpack.js.org/guides/tree-shaking/#mark-the-file-as-side-effect-free) in `package.json` when combined with re-export statements. A re-export is when you import something from one file and then export it again. You can re-export something with `export * from` or `export {foo} from` or `import {foo} from` followed by `export {foo}`.

    The bug was that files which only contain re-exports and that are marked as being side-effect free were not being included in the bundle if you import one of the re-exported symbols. This is because esbuild's implementation of re-export linking caused the original importing file to "short circuit" the re-export and just import straight from the file containing the final symbol, skipping the file containing the re-export entirely.

    This was normally not observable since the intermediate file consisted entirely of re-exports, which have no side effects. However, a recent change to allow ESM files to be lazily-initialized relies on all intermediate files being included in the bundle to trigger the initialization of the lazy evaluation wrappers. So the behavior of skipping over re-export files is now causing the imported symbols to not be initialized if the re-exported file is marked as lazily-evaluated.

    The fix is to track all re-exports in the import chain from the original file to the file containing the final symbol and then retain all of those statements if the import ends up being used.

* Add a very verbose `debug` log level

    This log level is an experiment. Enabling it logs a lot of information (currently only about path resolution). The idea is that if you are having an obscure issue, the debug log level might contain some useful information. Unlike normal logs which are meant to mainly provide actionable information, these debug logs are intentionally mostly noise and are designed to be searched through instead.

    Here is an example of debug-level log output:

    ```
     > debug: Resolving import "react" in directory "src" of type "import-statement"
       note: Read 26 entries for directory "src"
       note: Searching for "react" in "node_modules" directories starting from "src"
       note: Attempting to load "src/react" as a file
       note: Failed to find file "src/react"
       note: Failed to find file "src/react.tsx"
       note: Failed to find file "src/react.ts"
       note: Failed to find file "src/react.js"
       note: Failed to find file "src/react.css"
       note: Failed to find file "src/react.svg"
       note: Attempting to load "src/react" as a directory
       note: Failed to read directory "src/react"
       note: Parsed package name "react" and package subpath "."
       note: Checking for a package in the directory "node_modules/react"
       note: Read 7 entries for directory "node_modules/react"
       note: Read 393 entries for directory "node_modules"
       note: Attempting to load "node_modules/react" as a file
       note: Failed to find file "node_modules/react"
       note: Failed to find file "node_modules/react.tsx"
       note: Failed to find file "node_modules/react.ts"
       note: Failed to find file "node_modules/react.js"
       note: Failed to find file "node_modules/react.css"
       note: Failed to find file "node_modules/react.svg"
       note: Attempting to load "node_modules/react" as a directory
       note: Read 7 entries for directory "node_modules/react"
       note: Resolved to "node_modules/react/index.js" using the "main" field in "node_modules/react/package.json"
       note: Read 7 entries for directory "node_modules/react"
       note: Read 7 entries for directory "node_modules/react"
       note: Primary path is "node_modules/react/index.js" in namespace "file"
    ```

## 0.11.2

* Fix missing symbol dependency for wrapped ESM files ([#1086](https://github.com/evanw/esbuild/issues/1086))

    An internal graph node was missing an edge, which could result in generating code that crashes at run-time when code splitting is enabled. Specifically a part containing an import statement must depend on the imported file's wrapper symbol if the imported file is wrapped, regardless of whether it's a wrapped CommonJS or ESM file. Previously this was only the case for CommonJS files but not for ESM files, which is incorrect. This bug has been fixed.

* Fix an edge case with entry points and top-level await

    If an entry point uses `import()` on itself, it currently has to be wrapped since `import()` expressions call the wrapper for the imported file. This means the another call to the wrapper must be inserted at the bottom of the entry point file to start the lazy evaluation of the entry point code (otherwise nothing will be evaluated, since the entry point is wrapped). However, if this entry point then contains a top-level await that means the wrapper is `async` and must be passed to `await` to catch and forward any exceptions thrown during the evaluation of the entry point code. This `await` was previously missing in this specific case due to a bug, but the `await` should now be added in this release.

## 0.11.1

* Fix a missing space before internal `import()` when minifying ([#1082](https://github.com/evanw/esbuild/issues/1082))

    Internal `import()` of a CommonJS module inside the bundle turns into a call to `Promise.resolve().then(() => require())`. However, a space was not inserted before the `Promise` token when minifying, which could lead to a syntax error. This bug has been fixed.

* Fix code generation for unused imported files without side effects ([#1080](https://github.com/evanw/esbuild/issues/1080))

    When esbuild adds a wrapping closure around a file to turn it from a statically-initialized file to a dynamically-initialized file, it also needs to turn import statements in other files that import the wrapped file into calls to the wrapper so that the wrapped file is initialized in the correct ordering. However, although tree-shaking is disabled for wrapped CommonJS files because CommonJS exports are dynamic, tree-shaking is still enabled for wrapped ESM files because ESM exports are static.

    This caused a bug when files that have been marked with [`"sideEffects": false`](https://webpack.js.org/guides/tree-shaking/#mark-the-file-as-side-effect-free) end up being completely unused in the resulting bundle. In that case the file is removed entirely, but esbuild was still turning `import` statements to that file into calls to the ESM wrapper. These wrapper calls should instead be omitted if the file was completely removed from the bundle as dead code. This bug has been fixed.

* Allow top-level await in supported environments

    Top-level await (i.e. using the `await` keyword outside of an `async` function) is not yet part of the JavaScript language standard. The [feature proposal](https://github.com/tc39/proposal-top-level-await) is still at stage 3 and has not yet advanced to stage 4. However, V8 has already implemented it and it has shipped in Chrome 89 and node 14.8. This release allows top-level await to be used when the `--target=` flag is set to those compilation targets.

* Convert `import()` to `require()` if `import()` is not supported ([#1084](https://github.com/evanw/esbuild/issues/1084))

    This release now converts dynamic `import()` expressions into `Promise.resolve().then(() => require())` expressions if the compilation target doesn't support them. This is the case for node before version 13.2, for example.

## 0.11.0

**This release contains backwards-incompatible changes.** Since esbuild is before version 1.0.0, these changes have been released as a new minor version to reflect this (as [recommended by npm](https://docs.npmjs.com/cli/v6/using-npm/semver/)). You should either be pinning the exact version of `esbuild` in your `package.json` file or be using a version range syntax that only accepts patch upgrades such as `~0.10.0`. See the documentation about [semver](https://docs.npmjs.com/cli/v6/using-npm/semver/) for more information.

The changes in this release mostly relate to how entry points are handled. The way output paths are generated has changed in some cases, so you may need to update how you refer to the output path for a given entry point when you update to this release (see below for details). These breaking changes are as follows:

* Change how `require()` and `import()` of ESM works ([#667](https://github.com/evanw/esbuild/issues/667), [#706](https://github.com/evanw/esbuild/issues/706))

    Previously if you call `require()` on an ESM file, or call `import()` on an ESM file with code splitting disabled, esbuild would convert the ESM file to CommonJS. For example, if you had the following input files:

    ```js
    // cjs-file.js
    console.log(require('./esm-file.js').foo)

    // esm-file.js
    export let foo = bar()
    ```

    The previous bundling behavior would generate something like this:

    ```js
    var require_esm_file = __commonJS((exports) => {
      __markAsModule(exports);
      __export(exports, {
        foo: () => foo
      });
      var foo = bar();
    });
    console.log(require_esm_file().foo);
    ```

    This behavior has been changed and esbuild now generates something like this instead:

    ```js
    var esm_file_exports = {};
    __export(esm_file_exports, {
      foo: () => foo
    });
    var foo;
    var init_esm_file = __esm(() => {
      foo = bar();
    });
    console.log((init_esm_file(), esm_file_exports).foo);
    ```

    The variables have been pulled out of the lazily-initialized closure and are accessible to the rest of the module's scope. Some benefits of this approach:

    * If another file does `import {foo} from "./esm-file.js"`, it will just reference `foo` directly and will not pay the performance penalty or code size overhead of the dynamic property accesses that come with CommonJS-style exports. So this improves performance and reduces code size in some cases.

    * This fixes a long-standing bug ([#706](https://github.com/evanw/esbuild/issues/706)) where entry point exports could be broken if the entry point is a target of a `require()` call and the output format was ESM. This happened because previously calling `require()` on an entry point converted it to CommonJS, which then meant it only had a single `default` export, and the exported variables were inside the CommonJS closure and inaccessible to an ESM-style `export {}` clause. Now calling `require()` on an entry point only causes it to be lazily-initialized but all exports are still in the module scope and can still be exported using a normal `export {}` clause.

    * Now that this has been changed, `import()` of a module with top-level await ([#253](https://github.com/evanw/esbuild/issues/253)) is now allowed when code splitting is disabled. Previously this didn't work because `import()` with code splitting disabled was implemented by converting the module to CommonJS and using `Promise.resolve().then(() => require())`, but converting a module with top-level await to CommonJS is impossible because the CommonJS call signature must be synchronous. Now that this implemented using lazy initialization instead of CommonJS conversion, the closure wrapping the ESM file can now be `async` and the `import()` expression can be replaced by a call to the lazy initializer.

    * Adding the ability for ESM files to be lazily-initialized is an important step toward additional future code splitting improvements including: manual chunk names ([#207](https://github.com/evanw/esbuild/issues/207)), correct import evaluation order ([#399](https://github.com/evanw/esbuild/issues/399)), and correct top-level await evaluation order ([#253](https://github.com/evanw/esbuild/issues/253)). These features all need to make use of deferred evaluation of ESM code.

    In addition, calling `require()` on an ESM file now recursively wraps all transitive dependencies of that file instead of just wrapping that ESM file itself. This is an increase in the size of the generated code, but it is important for correctness ([#667](https://github.com/evanw/esbuild/issues/667)). Calling `require()` on a module means its evaluation order is determined at run-time, which means the evaluation order of all dependencies must also be determined at run-time. If you don't want the increase in code size, you should use an `import` statement instead of a `require()` call.

* Dynamic imports now use chunk names instead of entry names ([#1056](https://github.com/evanw/esbuild/issues/1056))

    Previously the output paths of dynamic imports (files imported using the `import()` syntax) were determined by the `--entry-names=` setting. However, this can cause problems if you configure the `--entry-names=` setting to omit both `[dir]` and `[hash]` because then two dynamic imports with the same name will cause an output file name collision.

    Now dynamic imports use the `--chunk-names=` setting instead, which is used for automatically-generated chunks. This setting is effectively required to include `[hash]` so dynamic import name collisions should now be avoided.

    In addition, dynamic imports no longer affect the automatically-computed default value of `outbase`. By default `outbase` is computed to be the [lowest common ancestor](https://en.wikipedia.org/wiki/Lowest_common_ancestor) directory of all entry points. Previously dynamic imports were considered entry points in this calculation so adding a dynamic entry point could unexpectedly affect entry point output file paths. This issue has now been fixed.

* Allow custom output paths for individual entry points

    By default, esbuild will automatically generate an output path for each entry point by computing the relative path from the `outbase` directory to the entry point path, and then joining that relative path to the `outdir` directory. The output path can be customized using `outpath`, but that only works for a single file. Sometimes you may need custom output paths while using multiple entry points. You can now do this by passing the entry points as a map instead of an array:

    * CLI
        ```
        esbuild out1=in1.js out2=in2.js --outdir=out
        ```

    * JS
        ```js
        esbuild.build({
          entryPoints: {
            out1: 'in1.js',
            out2: 'in2.js',
          },
          outdir: 'out',
        })
        ```

    * Go

        ```go
        api.Build(api.BuildOptions{
          EntryPointsAdvanced: []api.EntryPoint{{
            OutputPath: "out1",
            InputPath: "in1.js",
          }, {
            OutputPath: "out2",
            InputPath: "in2.js",
          }},
          Outdir: "out",
        })
        ```

    This will cause esbuild to generate the files `out/out1.js` and `out/out2.js` inside the output directory. These custom output paths are used as input for the `--entry-names=` path template setting, so you can use something like `--entry-names=[dir]/[name]-[hash]` to add an automatically-computed hash to each entry point while still using the custom output path.

* Derive entry point output paths from the original input path ([#945](https://github.com/evanw/esbuild/issues/945))

    Previously esbuild would determine the output path for an entry point by looking at the post-resolved path. For example, running `esbuild --bundle react --outdir=out` would generate the output path `out/index.js` because the input path `react` was resolved to `node_modules/react/index.js`. With this release, the output path is now determined by looking at the pre-resolved path. For example, running `esbuild --bundle react --outdir=out` now generates the output path `out/react.js`. If you need to keep using the output path that esbuild previously generated with the old behavior, you can use the custom output path feature (described above).

* Use the `file` namespace for file entry points ([#791](https://github.com/evanw/esbuild/issues/791))

    Plugins that contain an `onResolve` callback with the `file` filter don't apply to entry point paths because it's not clear that entry point paths are files. For example, you could potentially bundle an entry point of `https://www.example.com/file.js` with a HTTP plugin that automatically downloads data from the server at that URL. But this behavior can be unexpected for people writing plugins.

    With this release, esbuild will do a quick check first to see if the entry point path exists on the file system before running plugins. If it exists as a file, the namespace will now be `file` for that entry point path. This only checks the exact entry point name and doesn't attempt to search for the file, so for example it won't handle cases where you pass a package path as an entry point or where you pass an entry point without an extension. Hopefully this should help improve this situation in the common case where the entry point is an exact path.

In addition to the breaking changes above, the following features are also included in this release:

* Warn about mutation of private methods ([#1067](https://github.com/evanw/esbuild/pull/1067))

    Mutating a private method in JavaScript is not allowed, and will throw at run-time:

    ```js
    class Foo {
      #method() {}
      mutate() {
        this.#method = () => {}
      }
    }
    ```

    This is the case both when esbuild passes the syntax through untransformed and when esbuild transforms the syntax into the equivalent code that uses a `WeakSet` to emulate private methods in older browsers. However, it's clear from this code that doing this will always throw, so this code is almost surely a mistake. With this release, esbuild will now warn when you do this. This change was contributed by [@jridgewell](https://github.com/jridgewell).

* Fix some obscure TypeScript type parsing edge cases

    In TypeScript, type parameters come after a type and are placed in angle brackets like `Foo<T>`. However, certain built-in types do not accept type parameters including primitive types such as `number`. This means `if (x as number < 1) {}` is not a syntax error while `if (x as Foo < 1) {}` is a syntax error. This release changes TypeScript type parsing to allow type parameters in a more restricted set of situations, which should hopefully better resolve these type parsing ambiguities.

## 0.10.2

* Fix a crash that was introduced in the previous release ([#1064](https://github.com/evanw/esbuild/issues/1064))

    This crash happens when code splitting is active and there is a CSS entry point as well as two or more JavaScript entry points. There is a known issue where CSS bundling does not work when code splitting is active (code splitting is still a work in progress, see [#608](https://github.com/evanw/esbuild/issues/608)) so doing this will likely not work as expected. But esbuild obviously shouldn't crash. This release fixes the crash, although esbuild still does not yet generate the correct CSS output in this case.

* Fix private fields inside destructuring assignment ([#1066](https://github.com/evanw/esbuild/issues/1066))

    Private field syntax (i.e. `this.#field`) is supported for older language targets by converting the code into accesses into a `WeakMap`. However, although regular assignment (i.e. `this.#field = 1`) was handled destructuring assignment (i.e. `[this.#field] = [1]`) was not handled due to an oversight. Support for private fields inside destructuring assignment is now included with this release.

* Fix an issue with direct `eval` and top-level symbols

    It was previously the case that using direct `eval` caused the file containing it to be considered a CommonJS file, even if the file used ESM syntax. This was because the evaluated code could potentially attempt to interact with top-level symbols by name and the CommonJS closure was used to isolate those symbols from other modules so their names could be preserved (otherwise their names may need to be renamed to avoid collisions). However, ESM files are no longer convertable to CommonJS files due to the need to support top-level await.

    This caused a bug where scope hoisting could potentially merge two modules containing direct `eval` and containing the same top-level symbol name into the same scope. These symbols were prevented from being renamed due to the direct `eval`, which caused a syntax error at run-time due to the name collision.

    Because of this, esbuild is dropping the guarantee that using direct `eval` in an ESM file will be able to access top-level symbols. These symbols are now free to be renamed to avoid name collisions, and will now be minified when identifier minification is enabled. This is unlikely to affect real-world code because most real-world uses of direct `eval` only attempt to access local variables, not top-level symbols.

    Using direct `eval` in an ESM file when bundling with esbuild will generate a warning. The warning is not new and is present in previous releases of esbuild as well. The way to avoid the warning is to avoid direct `eval`, since direct `eval` is somewhat of an anti-pattern and there are better alternatives.

## 0.10.1

* Expose `metafile` to `onRebuild` in watch mode ([#1057](https://github.com/evanw/esbuild/issues/1057))

    Previously the build results returned to the watch mode `onRebuild` callback was missing the `metafile` property when the `metafile: true` option was present. This bug has been fixed.

* Add a `formatMessages` API ([#1058](https://github.com/evanw/esbuild/issues/1058))

    This API lets you print log messages to the terminal using the same log format that esbuild itself uses. This can be used to filter esbuild's warnings while still making the output look the same. Here's an example of calling this API:

    ```js
    import esbuild from 'esbuild'

    const formatted = await esbuild.formatMessages([{
      text: '"test" has already been declared',
      location: { file: 'file.js', line: 2, column: 4, length: 4, lineText: 'let test = "second"' },
      notes: [{
        text: '"test" was originally declared here',
        location: { file: 'file.js', line: 1, column: 4, length: 4, lineText: 'let test = "first"' },
      }],
    }], {
      kind: 'error',
      color: true,
      terminalWidth: 100,
    })

    process.stdout.write(formatted.join(''))
    ```

* Remove the file splitting optimization ([#998](https://github.com/evanw/esbuild/issues/998))

    This release removes the "file splitting optimization" that has up to this point been a part of esbuild's code splitting algorithm. This optimization allowed code within a single file to end up in separate chunks as long as that code had no side effects. For example, bundling two entry points that both use a disjoint set of code from a shared file consisting only of code without side effects would previously not generate any shared code chunks at all.

    This optimization is being removed because the top-level await feature was added to JavaScript after this optimization was added, and performing this optimization in the presence of top-level await is more difficult than before. The correct evaulation order of a module graph containing top-level await is extremely complicated and is specified at the module boundary. Moving code that is marked as having no side effects across module boundaries under these additional constraints is even more complexity and is getting in the way of implementing top-level await. So the optimization has been removed to unblock work on top-level await, which esbuild must support.

## 0.10.0

**This release contains backwards-incompatible changes.** Since esbuild is before version 1.0.0, these changes have been released as a new minor version to reflect this (as [recommended by npm](https://docs.npmjs.com/cli/v6/using-npm/semver/)). You should either be pinning the exact version of `esbuild` in your `package.json` file or be using a version range syntax that only accepts patch upgrades such as `~0.9.0`. See the documentation about [semver](https://docs.npmjs.com/cli/v6/using-npm/semver/) for more information.

That said, there are no breaking API changes in this release. The breaking changes are instead about how input files are interpreted and/or how output files are generated in some cases. So upgrading should be relatively straightforward as your API calls should still work the same way, but please make sure to test your code when you upgrade because the output may be different. These breaking changes are as follows:

* No longer support `module` or `exports` in an ESM file ([#769](https://github.com/evanw/esbuild/issues/769))

    This removes support for using CommonJS exports in a file with ESM exports. Previously this worked by converting the ESM file to CommonJS and then mixing the CommonJS and ESM exports into the same `exports` object. But it turns out that supporting this is additional complexity for the bundler, so it has been removed. It's also not something that works in real JavaScript environments since modules will never support both export syntaxes at once.

    Note that this doesn't remove support for using `require` in ESM files. Doing this still works (and can be made to work in a real ESM environment by assigning to `globalThis.require`). This also doesn't remove support for using `import` in CommonJS files. Doing this also still works.

* No longer change `import()` to `require()` ([#1029](https://github.com/evanw/esbuild/issues/1029))

    Previously esbuild's transform for `import()` matched TypeScript's behavior, which is to transform it into `Promise.resolve().then(() => require())` when the current output format is something other than ESM. This was done when an import is external (i.e. not bundled), either due to the expression not being a string or due to the string matching an external import path.

    With this release, esbuild will no longer do this. Now `import()` expressions will be preserved in the output instead. These expressions can be handled in non-ESM code by arranging for the `import` identifier to be a function that imports ESM code. This is how node works, so it will now be possible to use `import()` with node when the output format is something other than ESM.

* Run-time `export * as` statements no longer convert the file to CommonJS

    Certain `export * as` statements require a bundler to evaluate them at run-time instead of at compile-time like the JavaScript specification. This is the case when re-exporting symbols from an external file and a file in CommonJS format.

    Previously esbuild would handle this by converting the module containing the `export * as` statement to CommonJS too, since CommonJS exports are evaluated at run-time while ESM exports are evaluated at bundle-time. However, this is undesirable because tree shaking only works for ESM, not for CommonJS, and the CommonJS wrapper causes additional code bloat. Another upcoming problem is that top-level await cannot work within a CommonJS module because CommonJS `require()` is synchronous.

    With this release, esbuild will now convert modules containing a run-time `export * as` statement to a special ESM-plus-dynamic-fallback mode. In this mode, named exports present at bundle time can still be imported directly by name, but any imports that don't match one of the explicit named imports present at bundle time will be converted to a property access on the fallback object instead of being a bundle error. These property accesses are then resolved at run-time and will be undefined if the export is missing.

* Change whether certain files are interpreted as ESM or CommonJS ([#1043](https://github.com/evanw/esbuild/issues/1043))

    The bundling algorithm currently doesn't contain any logic that requires flagging modules as CommonJS vs. ESM beforehand. Instead it handles a superset and then sort of decides later if the module should be treated as CommonJS vs. ESM based on whether the module uses the `module` or `exports` variables and/or the `exports` keyword.

    With this release, files that follow [node's rules for module types](https://nodejs.org/api/packages.html#packages_type) will be flagged as explicitly ESM. This includes files that end in `.mjs` and files within a package containing `"type": "module"` in the enclosing `package.json` file. The CommonJS `module` and `exports` features will be unavailable in these files. This matters most for files without any exports, since then it's otherwise ambiguous what the module type is.

    In addition, files without exports should now accurately fall back to being considered CommonJS. They should now generate a `default` export of an empty object when imported using an `import` statement, since that's what happens in node when you import a CommonJS file into an ESM file in node. Previously the default export could be undefined because these export-less files were sort of treated as ESM but with missing import errors turned into warnings instead.

    This is an edge case that rarely comes up in practice, since you usually never import things from a module that has no exports.

In addition to the breaking changes above, the following features are also included in this release:

* Initial support for bundling with top-level await ([#253](https://github.com/evanw/esbuild/issues/253))

    Top-level await is a feature that lets you use an `await` expression at the top level (outside of an `async` function). Here is an example:

    ```js
    let promise = fetch('https://www.example.com/data')
    export let data = await promise.then(x => x.json())
    ```

    Top-level await only works in ECMAScript modules, and does not work in CommonJS modules. This means that you must use an `import` statement or an `import()` expression to import a module containing top-level await. You cannot use `require()` because it's synchronous while top-level await is asynchronous. There should be a descriptive error message when you try to do this.

    This initial release only has limited support for top-level await. It is only supported with the `esm` output format, but not with the `iife` or `cjs` output formats. In addition, the compilation is not correct in that two modules that both contain top-level await and that are siblings in the import graph will be evaluated in serial instead of in parallel. Full support for top-level await will come in a future release.

* Add the ability to set `sourceRoot` in source maps ([#1028](https://github.com/evanw/esbuild/pull/1028))

    You can now use the `--source-root=` flag to set the `sourceRoot` field in source maps generated by esbuild. When a `sourceRoot` is present in a source map, all source paths are resolved relative to it. This is particularly useful when you are hosting compiled code on a server and you want to point the source files to a GitHub repo, such as [what AMP does](https://cdn.ampproject.org/v0.mjs.map).

    Here is the description of `sourceRoot` from [the source map specification](https://sourcemaps.info/spec.html):

    > An optional source root, useful for relocating source files on a server or removing repeated values in the "sources" entry. This value is prepended to the individual entries in the "source" field. If the sources are not absolute URLs after prepending of the "sourceRoot", the sources are resolved relative to the SourceMap (like resolving script src in a html document).

    This feature was contributed by [@jridgewell](https://github.com/jridgewell).

* Allow plugins to return custom file watcher paths

    Currently esbuild's watch mode automatically watches all file system paths that are handled by esbuild itself, and also automatically watches the paths of files loaded by plugins when the paths are in the `file` namespace. The paths of files that plugins load in namespaces other than the `file` namespace are not automatically watched.

    Also, esbuild never automatically watches any file system paths that are consulted by the plugin during its processing, since esbuild is not aware of those paths. For example, this means that if a plugin calls `require.resolve()`, all of the various "does this file exist" checks that it does will not be watched automatically. So if one of those files is created in the future, esbuild's watch mode will not rebuild automatically even though the build is now outdated.

    To fix this problem, this release introduces the `watchFiles` and `watchDirs` properties on plugin return values. Plugins can specify these to add additional custom file system paths to esbuild's internal watch list. Paths in the `watchFiles` array cause esbuild to rebuild if the file contents change, and paths in the `watchDirs` array cause esbuild to rebuild if the set of directory entry names changes for that directory path.

    Note that `watchDirs` does not cause esbuild to rebuild if any of the contents of files inside that directory are changed. It also does not recursively traverse through subdirectories. It only watches the set of directory entry names (i.e. the output of the Unix `ls` command).

## 0.9.7

* Add support for Android on ARM 64-bit ([#803](https://github.com/evanw/esbuild/issues/803))

    This release includes support for Android in the official `esbuild` package. It should now be possible to install and run esbuild on Android devices through npm.

* Fix incorrect MIME types on Windows ([#1030](https://github.com/evanw/esbuild/issues/1030))

    The web server built into esbuild uses the file extension to determine the value of the `Content-Type` header. This was previously done using the `mime.TypeByExtension()` function from Go's standard library. However, this function is apparently broken on Windows because installed programs can change MIME types in the Windows registry: [golang/go#32350](https://github.com/golang/go/issues/32350). This release fixes the problem by using a copy of Go's `mime.TypeByExtension()` function without the part that reads from the Windows registry.

* Using a top-level return inside an ECMAScript module is now forbidden

    The CommonJS module format is implemented as an anonymous function wrapper, so technically you can use a top-level `return` statement and it will actually work. Some packages in the wild use this to exit early from module initialization, so esbuild supports this. However, the ECMAScript module format doesn't allow top-level returns. With this release, esbuild no longer allows top-level returns in ECMAScript modules.

## 0.9.6

* Expose build options to plugins ([#373](https://github.com/evanw/esbuild/issues/373))

    Plugins can now access build options from within the plugin using the `initialOptions` property. For example:

    ```js
    let nodeEnvPlugin = {
      name: 'node-env',
      setup(build) {
        const options = build.initialOptions
        options.define = options.define || {}
        options.define['process.env.NODE_ENV'] =
          options.minify ? '"production"' : '"development"'
      },
    }
    ```

* Fix an edge case with the object spread transform ([#1017](https://github.com/evanw/esbuild/issues/1017))

    This release fixes esbuild's object spread transform in cases where property assignment could be different than property definition. For example:

    ```js
    console.log({
      get x() {},
      ...{x: 1},
    })
    ```

    This should print `{x: 1}` but transforming this through esbuild with `--target=es6` causes the resulting code to throw an error. The problem is that esbuild currently transforms this code to a call to `Object.assign` and that uses property assignment semantics, which causes the assignment to throw (since you can't assign to a getter-only property).

    With this release, esbuild will now transform this into code that manually loops over the properties and copies them over one-by-one using `Object.defineProperty` instead. This uses property definition semantics which better matches the specification.

* Fix a TypeScript parsing edge case with arrow function return types ([#1016](https://github.com/evanw/esbuild/issues/1016))

    This release fixes the following TypeScript parsing edge case:

    ```ts
    ():Array<number>=>{return [1]}
    ```

    This was tripping up esbuild's TypeScript parser because the `>=` token was split into a `>` token and a `=` token because the `>` token is needed to close the type parameter list, but the `=` token was not being combined with the following `>` token to form a `=>` token. This is normally not an issue because there is normally a space in between the `>` and the `=>` tokens here. The issue only happened when the spaces were removed. This bug has been fixed. Now after the `>=` token is split, esbuild will expand the `=` token into the following characters if possible, which can result in a `=>`, `==`, or `===` token.

* Enable faster synchronous transforms under a flag ([#1000](https://github.com/evanw/esbuild/issues/1000))

    Currently the synchronous JavaScript API calls `transformSync` and `buildSync` spawn a new child process on every call. This is due to limitations with node's `child_process` API. Doing this means `transformSync` and `buildSync` are much slower than `transform` and `build`, which share the same child process across calls.

    There was previously a workaround for this limitation that uses node's `worker_threads` API and atomics to block the main thread while asynchronous communication happens in a worker, but that was reverted due to a bug in node's `worker_threads` implementation. Now that this bug has been fixed by node, I am re-enabling this workaround. This should result in `transformSync` and `buildSync` being much faster.

    This approach is experimental and is currently only enabled if the `ESBUILD_WORKER_THREADS` environment variable is present. If this use case matters to you, please try it out and let me know if you find any problems with it.

* Update how optional chains are compiled to match new V8 versions ([#1019](https://github.com/evanw/esbuild/issues/1019))

    An optional chain is an expression that uses the `?.` operator, which roughly avoids evaluation of the right-hand side if the left-hand side is `null` or `undefined`. So `a?.b` is basically equivalent to `a == null ? void 0 : a.b`. When the language target is set to `es2019` or below, esbuild will transform optional chain expressions into equivalent expressions that do not use the `?.` operator.

    This transform is designed to match the behavior of V8 exactly, and is designed to do something similar to the equivalent transform done by the TypeScript compiler. However, V8 has recently changed its behavior in two cases:

    * Forced call of an optional member expression should propagate the object to the method:

        ```js
        const o = { m() { return this; } };
        assert((o?.m)() === o);
        ```

        V8 bug: https://bugs.chromium.org/p/v8/issues/detail?id=10024

    * Optional call of `eval` must be an indirect eval:

        ```js
        globalThis.a = 'global';
        var b = (a => eval?.('a'))('local');
        assert(b === 'global');
        ```

        V8 bug: https://bugs.chromium.org/p/v8/issues/detail?id=10630

    This release changes esbuild's transform to match V8's new behavior. The transform in the TypeScript compiler is still emulating the old behavior as of version 4.2.3, so these syntax forms should be avoided in TypeScript code for portability.

## 0.9.5

* Fix parsing of the `[dir]` placeholder ([#1013](https://github.com/evanw/esbuild/issues/1013))

    The entry names feature in the previous release accidentally didn't include parsing for the `[dir]` placeholder, so the `[dir]` placeholder was passed through verbatim into the resulting output paths. This release fixes the bug, which means you can now use the `[dir]` placeholder. Sorry about the oversight.

## 0.9.4

* Enable hashes in entry point file paths ([#518](https://github.com/evanw/esbuild/issues/518))

    This release adds the new `--entry-names=` flag. It's similar to the `--chunk-names=` and `--asset-names=` flags except it sets the output paths for entry point files. The pattern defaults to `[dir]/[name]` which should be equivalent to the previous entry point output path behavior, so this should be a backward-compatible change.

    This change has the following consequences:

    * It is now possible for entry point output paths to contain a hash. For example, this now happens if you pass `--entry-names=[dir]/[name]-[hash]`. This means you can now use esbuild to generate output files such that all output paths have a hash in them, which means it should now be possible to serve the output files with an infinite cache lifetime so they are only downloaded once and then cached by the browser forever.

    * It is now possible to prevent the generation of subdirectories inside the output directory. Previously esbuild replicated the directory structure of the input entry points relative to the `outbase` directory (which defaults to the [lowest common ancestor](https://en.wikipedia.org/wiki/Lowest_common_ancestor) directory across all entry points). This value is substituted into the newly-added `[dir]` placeholder. But you can now omit it by omitting that placeholder, like this: `--entry-names=[name]`.

    * Source map names should now be equal to the corresponding output file name plus an additional `.map` extension. Previously the hashes were content hashes, so the source map had a different hash than the corresponding output file because they had different contents. Now they have the same hash so finding the source map should now be easier (just add `.map`).

    * Due to the way the new hashing algorithm works, all chunks can now be generated fully in parallel instead of some chunks having to wait until their dependency chunks have been generated first. The import paths for dependency chunks are now swapped in after chunk generation in a second pass (detailed below). This could theoretically result in a speedup although I haven't done any benchmarks around this.

    Implementing this feature required overhauling how hashes are calculated to prevent the chicken-and-egg hashing problem due to dynamic imports, which can cause cycles in the import graph of the resulting output files when code splitting is enabled. Since generating a hash involved first hashing all of your dependencies, you could end up in a situation where you needed to know the hash to calculate the hash (if a file was a dependency of itself).

    The hashing algorithm now works in three steps (potentially subject to change in the future):

    1. The initial versions of all output files are generated in parallel, with temporary paths used for any imports of other output files. Each temporary path is a randomly-generated string that is unique for each output file. An initial source map is also generated at this step if source maps are enabled.

        The hash for the first step includes: the raw content of the output file excluding the temporary paths, the relative file paths of all input files present in that output file, the relative output path for the resulting output file (with `[hash]` for the hash that hasn't been computed yet), and contents of the initial source map.

    2. After the initial versions of all output files have been generated, calculate the final hash and final output path for each output file. Calculating the final output path involves substituting the final hash for the `[hash]` placeholder in the entry name template.

        The hash for the second step includes: the hash from the first step for this file and all of its transitive dependencies.

    3. After all output files have a final output path, the import paths in each output file for importing other output files are substituted. Source map offsets also have to be adjusted because the final output path is likely a different length than the temporary path used in the first step. This is also done in parallel for each output file.

        This whole algorithm roughly means the hash of a given output file should change if an only if any input file in that output file or any output file it depends on is changed. So the output path and therefore the browser's cache key should not change for a given output file in between builds if none of the relevant input files were changed.

* Fix importing a path containing a `?` character on Windows ([#989](https://github.com/evanw/esbuild/issues/989))

    On Windows, the `?` character is not allowed in path names. This causes esbuild to fail to import paths containing this character. This is usually fine because people don't put `?` in their file names for this reason. However, the import paths for some ancient CSS code contains the `?` character as a hack to work around a bug in Internet Explorer:

    ```css
    @font-face {
      src:
        url("./icons.eot?#iefix") format('embedded-opentype'),
        url("./icons.woff2") format('woff2'),
        url("./icons.woff") format('woff'),
        url("./icons.ttf") format('truetype'),
        url("./icons.svg#icons") format('svg');
    }
    ```

    The intent is for the bundler to ignore the `?#iefix` part. However, there may actually be a file called `icons.eot?#iefix` on the file system so esbuild checks the file system for both `icons.eot?#iefix` and `icons.eot`. This check was triggering this issue. With this release, an invalid path is considered the same as a missing file so bundling code like this should now work on Windows.

* Parse and ignore the deprecated `@-ms-viewport` CSS rule ([#997](https://github.com/evanw/esbuild/issues/997))

    The [`@viewport`](https://www.w3.org/TR/css-device-adapt-1/#atviewport-rule) rule has been deprecated and removed from the web. Modern browsers now completely ignore this rule. However, in theory it sounds like would still work for mobile versions of Internet Explorer, if those still exist. The https://ant.design/ library contains an instance of the `@-ms-viewport` rule and it currently causes a warning with esbuild, so this release adds support for parsing this rule to disable the warning.

* Avoid mutating the binary executable file in place ([#963](https://github.com/evanw/esbuild/issues/963))

    This release changes the install script for the `esbuild` npm package to use the "rename a temporary file" approach instead of the "write the file directly" approach to replace the `esbuild` command stub file with the real binary executable. This should hopefully work around a problem with the [pnpm](https://pnpm.js.org/) package manager and its use of hard links.

* Avoid warning about potential issues with `sideEffects` in packages ([#999](https://github.com/evanw/esbuild/issues/999))

    Bare imports such as `import "foo"` mean the package is only imported for its side effects. Doing this when the package contains `"sideEffects": false` in `package.json` causes a warning because it means esbuild will not import the file since it has been marked as having no side effects, even though the import statement clearly expects it to have side effects. This is usually caused by an incorrect `sideEffects` annotation in the package.

    However, this warning is not immediately actionable if the file containing the import statement is itself in a package. So with this release, esbuild will no longer issue this warning if the file containing the import is inside a `node_modules` folder. Note that even though the warning is no longer there, this situation can still result in a broken bundle if the `sideEffects` annotation is incorrect.

## 0.9.3

* Fix path resolution with the `exports` field for scoped packages

    This release fixes a bug where the `exports` field in `package.json` files was not being detected for scoped packages (i.e. packages of the form `@scope/pkg-name` instead of just `pkg-name`). The `exports` field should now be respected for these kinds of packages.

* Improved error message in `exports` failure case

    Node's new [conditional exports feature](https://nodejs.org/docs/latest/api/packages.html#packages_conditional_exports) can be non-intuitive and hard to use. Now that esbuild supports this feature (as of version 0.9.0), you can get into a situation where it's impossible to import a package if the package's `exports` field in its `package.json` file isn't configured correctly.

    Previously the error message for this looked like this:

    ```
     > entry.js:1:7: error: Could not resolve "jotai" (mark it as external to exclude it from the bundle)
         1 │ import 'jotai'
           ╵        ~~~~~~~
       node_modules/jotai/package.json:16:13: note: The path "." is not exported by "jotai"
        16 │   "exports": {
           ╵              ^
    ```

    With this release, the error message will now provide additional information about why the package cannot be imported:

    ```
     > entry.js:1:7: error: Could not resolve "jotai" (mark it as external to exclude it from the bundle)
         1 │ import 'jotai'
           ╵        ~~~~~~~
       node_modules/jotai/package.json:16:13: note: The path "." is not currently exported by package "jotai"
        16 │   "exports": {
           ╵              ^
       node_modules/jotai/package.json:18:9: note: None of the conditions provided ("module", "require", "types") match any of the currently active conditions ("browser", "default", "import")
        18 │     ".": {
           ╵          ^
       entry.js:1:7: note: Consider using a "require()" call to import this package
         1 │ import 'jotai'
           ╵        ~~~~~~~
    ```

    In this case, one solution could be import this module using `require()` since this package provides an export for the `require` condition. Another solution could be to pass `--conditions=module` to esbuild since this package provides an export for the `module` condition (the `types` condition is likely not valid JavaScript code).

    This problem occurs because this package doesn't provide an import path for ESM code using the `import` condition and also doesn't provide a fallback import path using the `default` condition.

* Mention glob syntax in entry point error messages ([#976](https://github.com/evanw/esbuild/issues/976))

    In this release, including a `*` in the entry point path now causes the failure message to tell you that glob syntax must be expanded first before passing the paths to esbuild. People that hit this are usually converting an existing CLI command to a JavaScript API call and don't know that glob expansion is done by their shell instead of by esbuild. An appropriate fix is to use a library such as [`glob`](https://www.npmjs.com/package/glob) to expand the glob pattern first before passing the paths to esbuild.

* Raise certain VM versions in the JavaScript feature compatibility table

    JavaScript VM feature compatibility data is derived from this dataset: https://kangax.github.io/compat-table/. The scripts that process the dataset expand the data to include all VM versions that support a given feature (e.g. `chrome44`, `chrome45`, `chrome46`, ...) so esbuild takes the minimum observed version as the first version for which the feature is supported.

    However, some features can have subtests that each check a different aspect of the feature. In this case the desired version is the minimum version within each individual subtest, but the maximum of those versions across all subtests (since esbuild should only use the feature if it works in all cases). Previously esbuild computed the minimum version across all subtests, but now esbuild computes the maximum version across all subtests. This means esbuild will now lower JavaScript syntax in more cases.

* Mention the configured target environment in error messages ([#975](https://github.com/evanw/esbuild/issues/975))

    Using newer JavaScript syntax with an older target environment (e.g. `chrome10`) can cause a build error if esbuild doesn't support transforming that syntax such that it is compatible with that target environment. Previously the error message was generic but with this release, the target environment is called outp explicitly in the error message. This is helpful if esbuild is being wrapped by some other tool since the other tool can obscure what target environment is actually being passed to esbuild.

* Fix an issue with Unicode and source maps

    This release fixes a bug where non-ASCII content that ended up in an output file but that was not part of an input file could throw off source mappings. An example of this would be passing a string containing non-ASCII characters to the `globalName` setting with the `minify` setting active and the `charset` setting set to `utf8`. The conditions for this bug are fairly specific and unlikely to be hit, so it's unsurprising that this issue hasn't been discovered earlier. It's also unlikely that this issue affected real-world code.

    The underlying cause is that while the meaning of column numbers in source maps is undefined in the specification, in practice most tools treat it as the number of UTF-16 code units from the start of the line. The bug happened because column increments for outside-of-file characters were incorrectly counted using byte offsets instead of UTF-16 code unit counts.

## 0.9.2

* Fix export name annotations in CommonJS output for node ([#960](https://github.com/evanw/esbuild/issues/960))

    The previous release introduced a regression that caused a syntax error when building ESM files that have a default export with `--platform=node`. This is because the generated export contained the `default` keyword like this: `0 && (module.exports = {default});`. This regression has been fixed.

## 0.9.1

* Fix bundling when parent directory is inaccessible ([#938](https://github.com/evanw/esbuild/issues/938))

    Previously bundling with esbuild when a parent directory is inaccessible did not work because esbuild would try to read the directory to search for a `node_modules` folder and would then fail the build when that failed. In practice this caused issues in certain Linux environments where a directory close to the root directory was inaccessible (e.g. on Android). With this release, esbuild will treat inaccessible directories as empty to allow for the `node_modules` search to continue past the inaccessible directory and into its parent directory. This means it should now be possible to bundle with esbuild in these situations.

* Avoid allocations in JavaScript API stdout processing ([#941](https://github.com/evanw/esbuild/pull/941))

    This release improves the efficiency of the JavaScript API. The API runs the binary esbuild executable in a child process and then communicates with it over stdin/stdout. Previously the stdout buffer containing the remaining partial message was copied after each batch of messages due to a bug. This was unintentional and unnecessary, and has been removed. Now this part of the code no longer involves any allocations. This fix was contributed by [@jridgewell](https://github.com/jridgewell).

* Support conditional `@import` syntax when not bundling ([#953](https://github.com/evanw/esbuild/issues/953))

    Previously conditional CSS imports such as `@import "print.css" print;` was not supported at all and was considered a syntax error. With this release, it is now supported in all cases except when bundling an internal import. Support for bundling internal CSS imports is planned but will happen in a later release.

* Always lower object spread and rest when targeting V8 ([#951](https://github.com/evanw/esbuild/issues/951))

    This release causes object spread (e.g. `a = {...b}`) and object rest (e.g. `{...a} = b`) to always be lowered to a manual implementation instead of using native syntax when the `--target=` parameter includes a V8-based JavaScript runtime such as `chrome`, `edge`, or `node`. It turns out this feature is implemented inefficiently in V8 and copying properties over to a new object is around a 2x performance improvement. In addition, doing this manually instead of using the native implementation generates a lot less work for the garbage collector. You can see [V8 bug 11536](https://bugs.chromium.org/p/v8/issues/detail?id=11536) for details. If the V8 performance bug is eventually fixed, the translation of this syntax will be disabled again for V8-based targets containing the bug fix.

* Fix object rest return value ([#956](https://github.com/evanw/esbuild/issues/956))

    This release fixes a bug where the value of an object rest assignment was incorrect if the object rest assignment was lowered:

    ```js
    // This code was affected
    let x, y
    console.log({x, ...y} = {x: 1, y: 2})
    ```

    Previously this code would incorrectly print `{y: 2}` (the value assigned to `y`) when the object rest expression was lowered (i.e. with `--target=es2017` or below). Now this code will correctly print `{x: 1, y: 2}` instead. This bug did not affect code that did not rely on the return value of the assignment expression, such as this code:

    ```js
    // This code was not affected
    let x, y
    ({x, ...y} = {x: 1, y: 2})
    ```

* Basic support for CSS page margin rules ([#955](https://github.com/evanw/esbuild/issues/955))

    There are 16 different special at-rules that can be nested inside the `@page` rule. They are defined in [this specification](https://www.w3.org/TR/css-page-3/#syntax-page-selector). Previously esbuild treated these as unknown rules, but with this release esbuild will now treat these as known rules. The only real difference in behavior is that esbuild will no longer warn about these rules being unknown.

* Add export name annotations to CommonJS output for node

    When you import a CommonJS file using an ESM `import` statement in node, the `default` import is the value of `module.exports` in the CommonJS file. In addition, node attempts to generate named exports for properties of the `module.exports` object.

    Except that node doesn't actually ever look at the properties of that object to determine the export names. Instead it parses the CommonJS file and scans the AST for certain syntax patterns. A full list of supported patterns can be found in the [documentation for the `cjs-module-lexer` package](https://github.com/guybedford/cjs-module-lexer#grammar). This library doesn't currently support the syntax patterns used by esbuild.

    While esbuild could adapt its syntax to these patterns, the patterns are less compact than the ones used by esbuild and doing this would lead to code bloat. Supporting two separate ways of generating export getters would also complicate esbuild's internal implementation, which is undesirable.

    Another alternative could be to update the implementation of `cjs-module-lexer` to support the specific patterns used by esbuild. This is also undesirable because this pattern detection would break when minification is enabled, this would tightly couple esbuild's output format with node and prevent esbuild from changing it, and it wouldn't work for existing and previous versions of node that still have the old version of this library.

    Instead, esbuild will now add additional code to "annotate" ESM files that have been converted to CommonJS when esbuild's platform has been set to `node`. The annotation is dead code but is still detected by the `cjs-module-lexer` library. If the original ESM file has the exports `foo` and `bar`, the additional annotation code will look like this:

    ```js
    0 && (module.exports = {foo, bar});
    ```

    This allows you to use named imports with an ESM `import` statement in node (previously you could only use the `default` import):

    ```js
    import { foo, bar } from './file-built-by-esbuild.cjs'
    ```

## 0.9.0

**This release contains backwards-incompatible changes.** Since esbuild is before version 1.0.0, these changes have been released as a new minor version to reflect this (as [recommended by npm](https://docs.npmjs.com/cli/v6/using-npm/semver/)). You should either be pinning the exact version of `esbuild` in your `package.json` file or be using a version range syntax that only accepts patch upgrades such as `^0.8.0`. See the documentation about [semver](https://docs.npmjs.com/cli/v6/using-npm/semver/) for more information.

* Add support for node's `exports` field in `package.json` files ([#187](https://github.com/evanw/esbuild/issues/187))

    This feature was recently added to node. It allows you to rewrite what import paths inside your package map to as well as to prevent people from importing certain files in your package. Adding support for this to esbuild is a breaking change (i.e. code that was working fine before can easily stop working) so adding support for it has been delayed until this breaking change release.

    One way to use this feature is to remap import paths for your package. For example, this would remap an import of `your-pkg/esm/lib.js` (the "public" import path) to `your-pkg/dist/esm/lib.js` (the "private" file system path):

    ```json
    {
      "name": "your-pkg",
      "exports": {
        "./esm/*": "./dist/esm/*",
        "./cjs/*": "./dist/cjs/*"
      }
    }
    ```

    Another way to use this feature is to have conditional imports where the same import path can mean different things in different situations. For example, this would remap `require('your-pkg')` to `your-pkg/required.cjs` and `import 'your-pkg'` to `your-pkg/imported.mjs`:

    ```json
    {
      "name": "your-pkg",
      "exports": {
        "import": "./imported.mjs",
        "require": "./required.cjs"
      }
    }
    ```

    There is built-in support for the `import` and `require` conditions depending on the kind of import and the `browser` and `node` conditions depending on the current platform. In addition, the `default` condition always applies regardless of the current configuration settings and can be used as a catch-all fallback condition.

    Note that when you use conditions, _your package may end up in the bundle multiple times!_ This is a subtle issue that can cause bugs due to duplicate copies of your code's state in addition to bloating the resulting bundle. This is commonly known as the [dual package hazard](https://nodejs.org/docs/latest/api/packages.html#packages_dual_package_hazard). The primary way of avoiding this is to put all of your code in the `require` condition and have the `import` condition just be a light wrapper that calls `require` on your package and re-exports the package using ESM syntax.

    There is also support for custom conditions with the `--conditions=` flag. The meaning of these is entirely up to package authors. For example, you could imagine a package that requires you to configure `--conditions=test,en-US`. Node has currently only endorsed the `development` and `production` custom conditions for recommended use.

* Remove the `esbuild.startService()` API

    Due to [#656](https://github.com/evanw/esbuild/issues/656), Calling `service.stop()` no longer does anything, so there is no longer a strong reason for keeping the `esbuild.startService()` API around. The primary thing it currently does is just make the API more complicated and harder to use. You can now just call `esbuild.build()` and `esbuild.transform()` directly instead of calling `esbuild.startService().then(service => service.build())` or `esbuild.startService().then(service => service.transform())`.

    If you are using esbuild in the browser, you now need to call `esbuild.initialize({ wasmURL })` and wait for the returned promise before calling `esbuild.transform()`. It takes the same options that `esbuild.startService()` used to take. Note that the `esbuild.buildSync()` and `esbuild.transformSync()` APIs still exist when using esbuild in node. Nothing has changed about the synchronous esbuild APIs.

* Remove the `metafile` from `outputFiles` ([#633](https://github.com/evanw/esbuild/issues/633))

    Previously using `metafile` with the API is unnecessarily cumbersome because you have to extract the JSON metadata from the output file yourself instead of it just being provided to you as a return value. This is especially a bummer if you are using `write: false` because then you need to use a for loop over the output files and do string comparisons with the file paths to try to find the one corresponding to the `metafile`. Returning the metadata directly is an important UX improvement for the API. It means you can now do this:

    ```js
    const result = await esbuild.build({
      entryPoints: ['entry.js'],
      bundle: true,
      metafile: true,
    })
    console.log(result.metafile.outputs)
    ```

* The banner and footer options are now language-specific ([#712](https://github.com/evanw/esbuild/issues/712))

    The `--banner=` and `--footer=` options now require you to pass the file type:

    * CLI:

        ```
        esbuild --banner:js=//banner --footer:js=//footer
        esbuild --banner:css=/*banner*/ --footer:css=/*footer*/
        ```

    * JavaScript

        ```js
        esbuild.build({
          banner: { js: '//banner', css: '/*banner*/' },
          footer: { js: '//footer', css: '/*footer*/' },
        })
        ```

    * Go

        ```go
        api.Build(api.BuildOptions{
          Banner: map[string]string{"js": "//banner"},
          Footer: map[string]string{"js": "//footer"},
        })
        api.Build(api.BuildOptions{
          Banner: map[string]string{"css": "/*banner*/"},
          Footer: map[string]string{"css": "/*footer*/"},
        })
        ```

    This was changed because the feature was originally added in a JavaScript-specific manner, which was an oversight. CSS banners and footers must be separate from JavaScript banners and footers to avoid injecting JavaScript syntax into your CSS files.

* The extensions `.mjs` and `.cjs` are no longer implicit

    Previously the "resolve extensions" setting included `.mjs` and `.cjs` but this is no longer the case. This wasn't a good default because it doesn't match node's behavior and could break some packages. You now have to either explicitly specify these extensions or configure the "resolve extensions" setting yourself.

* Remove the `--summary` flag and instead just always print a summary ([#704](https://github.com/evanw/esbuild/issues/704))

    The summary can be disabled if you don't want it by passing `--log-level=warning` instead. And it can be enabled in the API by setting `logLevel: 'info'`. I'm going to try this because I believe it will improve the UX. People have this problem with esbuild when they first try it where it runs so quickly that they think it must be broken, only to later discover that it actually worked fine. While this is funny, it seems like a good indication that the UX could be improved. So I'm going to try automatically printing a summary to see how that goes. Note that the summary is not printed if incremental builds are active (this includes the watch and serve modes).

* Rename `--error-limit=` to `--log-limit=`

    This parameter has been renamed because it now applies to both warnings and errors, not just to errors. Previously setting the error limit did not apply any limits to the number of warnings printed, which could sometimes result in a deluge of warnings that are problematic for Windows Command Prompt, which is very slow to print to and has very limited scrollback. Now the log limit applies to the total number of log messages including both errors and warnings, so no more than that number of messages will be printed. The log usually prints log messages immediately but it will now intentionally hold back warnings when approaching the limit to make room for possible future errors during a build. So if a build fails you should be guaranteed to see an error message (i.e. warnings can't use up the entire log limit and then prevent errors from being printed).

* Remove the deprecated `--avoid-tdz` option

    This option is now always enabled and cannot be disabled, so it is being removed from the API. The existing API parameter no longer does anything so this removal has no effect the generated output.

* Remove `SpinnerBusy` and `SpinnerIdle` from the Go API

    These options were part of an experiment with the CLI that didn't work out. Watch mode no longer uses a spinner because it turns out people want to be able to interleave esbuild's stderr pipe with other tools and were getting tripped up by the spinner animation. These options no longer do anything and have been removed.

## 0.8.57

* Fix overlapping chunk names when code splitting is active ([#928](https://github.com/evanw/esbuild/issues/928))

    Code splitting chunks use a content hash in their file name. This is good for caching because it means the file name is guaranteed to change if the chunk contents change, and the file name is guaranteed to stay the same if the chunk contents don't change (e.g. someone only modifies a comment). However, using a pure content hash can cause bugs if two separate chunks end up with the same contents.

    A high-level example would be two identical copies of a library being accidentally collapsed into a single copy. While this results in a smaller bundle, this is incorrect because each copy might need to have its own state and so must be represented independently in the bundle.

    This release fixes this issue by mixing additional information into the file name hash, which is no longer a content hash. The information includes the paths of the input files as well as the ranges of code within the file that are included in the chunk. File paths are used because they are a stable file identifier, but the relative path is used with `/` as the path separator to hopefully eliminate cross-platform differences between Unix and Windows.

* Fix `--keep-names` for lowered class fields

    Anonymous function expressions used in class field initializers are automatically assigned a `.name` property in JavaScript:

    ```js
    class Example {
      field1 = () => {}
      static field2 = () => {}
    }
    assert(new Example().field1.name === 'field1')
    assert(Example.field2.name === 'field2')
    ```

    This usually doesn't need special handling from esbuild's `--keep-names` option because esbuild doesn't modify field names, so the `.name` property will not change. However, esbuild will relocate the field initializer if the configured language target doesn't support class fields (e.g. `--target=es6`). In that case the `.name` property wasn't preserved even when `--keep-names` was specified. This bug has been fixed. Now the `.name` property should be preserved in this case as long as you enable `--keep-names`.

* Enable importing certain data URLs in CSS and JavaScript

    You can now import data URLs of type `text/css` using a CSS `@import` rule and import data URLs of type `text/javascript` and `application/json` using a JavaScript `import` statement. For example, doing this is now possible:

    ```js
    import 'data:text/javascript,console.log("hello!");';
    import _ from 'data:application/json,"world!"';
    ```

    This is for compatibility with node which [supports this feature natively](https://nodejs.org/docs/latest/api/esm.html#esm_data_imports). Importing from a data URL is sometimes useful for injecting code to be evaluated before an external import without needing to generate a separate imported file.

## 0.8.56

* Fix a discrepancy with esbuild's `tsconfig.json` implementation ([#913](https://github.com/evanw/esbuild/issues/913))

    If a `tsconfig.json` file contains a `"baseUrl"` value and `"extends"` another `tsconfig.json` file that contains a `"paths"` value, the base URL used for interpreting the paths should be the overridden value. Previously esbuild incorrectly used the inherited value, but with this release esbuild will now use the overridden value instead.

* Work around the Jest testing framework breaking node's `Buffer` API ([#914](https://github.com/evanw/esbuild/issues/914))

    Running esbuild within a Jest test fails because Jest causes `Buffer` instances to not be considered `Uint8Array` instances, which then breaks the code esbuild uses to communicate with its child process. More info is here: https://github.com/facebook/jest/issues/4422. This release contains a workaround that copies each `Buffer` object into a `Uint8Array` object when this invariant is broken. That should prevent esbuild from crashing when it's run from within a Jest test.

* Better handling of implicit `main` fields in `package.json`

    If esbuild's automatic `main` vs. `module` detection is enabled for `package.json` files, esbuild will now use `index.js` as an implicit `main` field if the `main` field is missing but `index.js` is present. This means if a `package.json` file only contains a `module` field but not a `main` field and the package is imported using both an ESM `import` statement and a CommonJS `require` call, the `index.js` file will now be picked instead of the file in the `module` field.

## 0.8.55

* Align more closely with node's `default` import behavior for CommonJS ([#532](https://github.com/evanw/esbuild/issues/532))

    _Note: This could be considered a breaking change or a bug fix depending on your point of view._

    Importing a CommonJS file into an ESM file does not behave the same everywhere. Historically people compiled their ESM code into CommonJS using Babel before ESM was supported natively. More recently, node has made it possible to use ESM syntax natively but to still import CommonJS files into ESM. These behave differently in many ways but one of the most unfortunate differences is how the `default` export is handled.

    When you import a normal CommonJS file, both Babel and node agree that the value of `module.exports` should be stored in the ESM import named `default`. However, if the CommonJS file used to be an ESM file but was compiled into a CommonJS file, Babel will set the ESM import named `default` to the value of the original ESM export named `default` while node will continue to set the ESM import named `default` to the value of `module.exports`. Babel detects if a CommonJS file used to be an ESM file by the presence of the `exports.__esModule = true` marker.

    This is unfortunate because it means there is no general way to make code work with both ecosystems. With Babel the code `import * as someFile from './some-file'` can access the original `default` export with `someFile.default` but with node you need to use `someFile.default.default` instead. Previously esbuild followed Babel's approach but starting with this release, esbuild will now try to use a blend between the Babel and node approaches.

    This is the new behavior: importing a CommonJS file will set the `default` import to `module.exports` in all cases except when `module.exports.__esModule && "default" in module.exports`, in which case it will fall through to `module.exports.default`. In other words: in cases where the default import was previously `undefined` for CommonJS files when `exports.__esModule === true`, the default import will now be `module.exports`. This should hopefully keep Babel cross-compiled ESM code mostly working but at the same time now enable some node-oriented code to start working.

    If you are authoring a library using ESM but shipping it as CommonJS, the best way to avoid this mess is to just never use `default` exports in ESM. Only use named exports with names other than `default`.

* Fix bug when ESM file has empty exports and is converted to CommonJS ([#910](https://github.com/evanw/esbuild/issues/910))

    A file containing the contents `export {}` is still considered to be an ESM file even though it has no exports. However, if a file containing this edge case is converted to CommonJS internally during bundling (e.g. when it is the target of `require()`), esbuild failed to mark the `exports` symbol from the CommonJS wrapping closure as used even though it is actually needed. This resulted in an output file that crashed when run. The `exports` symbol is now considered used in this case, so the bug has been fixed.

* Avoid introducing `this` for imported function calls

    It is possible to import a function exported by a CommonJS file into an ESM file like this:

    ```js
    import {fn} from './cjs-file.js'
    console.log(fn())
    ```

    When you do this, esbuild currently transforms your code into something like this:

    ```js
    var cjs_file = __toModule(require("./cjs-file.js"));
    console.log(cjs_file.fn());
    ```

    However, doing that changes the value of `this` observed by the export `fn`. The property access `cjs_file.fn` is in the syntactic "call target" position so the value of `this` becomes the value of `cjs_file`. With this release, esbuild will now use a different syntax in this case to avoid passing `cjs_file` as `this`:

    ```js
    var cjs_file = __toModule(require("./cjs-file.js"));
    console.log((0, cjs_file.fn)());
    ```

    This change in esbuild mirrors a similar [recent TypeScript compiler change](https://github.com/microsoft/TypeScript/pull/35877), and also makes esbuild more consistent with Babel which already does this transformation.

## 0.8.54

* Fix ordering issue with private class methods ([#901](https://github.com/evanw/esbuild/issues/901))

    This release fixes an ordering issue with private class fields where private methods were not available inside class field initializers. The issue affected code such as the following when the compilation target was set to `es2020` or lower:

    ```js
    class A {
      pub = this.#priv;
      #priv() {
        return 'Inside #priv';
      }
    }
    assert(new A().pub() === 'Inside #priv');
    ```

    With this release, code that does this should now work correctly.

* Fix `--keep-names` for private class members

    Normal class methods and class fields don't need special-casing with esbuild when the `--keep-names` option is enabled because esbuild doesn't rename property names and doesn't transform class syntax in a way that breaks method names, so the names are kept without needing to generate any additional code.

    However, this is not the case for private class methods and private class fields. When esbuild transforms these for `--target=es2020` and earlier, the private class methods and private class field initializers are turned into code that uses a `WeakMap` or a `WeakSet` for access to preserve the privacy semantics. This ends up breaking the `.name` property and previously `--keep-names` didn't handle this edge case.

    With this release, `--keep-names` will also preserve the names of private class methods and private class fields. That means code like this should now work with `--keep-names --target=es2020`:

    ```js
    class Foo {
      #foo() {}
      #bar = () => {}
      test() {
        assert(this.#foo.name === '#foo')
        assert(this.#bar.name === '#bar')
      }
    }
    ```

* Fix cross-chunk import paths ([#899](https://github.com/evanw/esbuild/issues/899))

    This release fixes an issue with the `--chunk-names=` feature where import paths in between two different automatically-generated code splitting chunks were relative to the output directory instead of relative to the importing chunk. This caused an import failure with the imported chunk if the chunk names setting was configured to put the chunks into a subdirectory. This bug has been fixed.

* Remove the guarantee that direct `eval` can access imported symbols

    Using direct `eval` when bundling is not a good idea because esbuild must assume that it can potentially reach anything in any of the containing scopes. Using direct `eval` has the following negative consequences:

    * All names in all containing scopes are frozen and are not renamed during bundling, since the code in the direct `eval` could potentially access them. This prevents code in all scopes containing the call to direct `eval` from being minified or from being removed as dead code.

    * The entire file is converted to CommonJS. This increases code size and decreases performance because exports are now resolved at run-time instead of at compile-time. Normally name collisions with other files are avoided by renaming conflicting symbols, but direct `eval` prevents symbol renaming so name collisions are prevented by wrapping the file in a CommonJS closure instead.

    * Even with all of esbuild's special-casing of direct `eval`, referencing an ESM `import` from direct `eval` still doesn't necessarily work. ESM imports are live bindings to a symbol from another file and are represented by referencing that symbol directly in the flattened bundle. That symbol may use a different name which could break direct `eval`.

    I recently realized that the last consequence of direct `eval` (the problem about not being able to reference `import` symbols) could cause subtle correctness bugs. Specifically esbuild tries to prevent the imported symbol from being renamed, but doing so could cause name collisions that make the resulting bundle crash when it's evaluated. Two files containing direct `eval` that both import the same symbol from a third file but that import it with different aliases create a system of unsatisfiable naming constraints.

    So this release contains these changes to address this:

    1. Direct `eval` is no longer guaranteed to be able to access imported symbols. This means imported symbols may be renamed or removed as dead code even though a call to direct `eval` could theoretically need to access them. If you need this to work, you'll have to store the relevant imports in a variable in a nested scope and move the call to direct `eval` into that nested scope.

    2. Using direct `eval` in a file in ESM format is now a warning. This is because the semantics of direct `eval` are poorly understood (most people don't intend to use direct `eval` at all) and because the negative consequences of bundling code with direct `eval` are usually unexpected and undesired. Of the few valid use cases for direct `eval`, it is usually a good idea to rewrite your code to avoid using direct `eval` in the first place.

        For example, if you write code that looks like this:

        ```js
        export function runCodeWithFeatureFlags(code) {
          let featureFlags = {...}
          eval(code) // "code" should be able to access "featureFlags"
        }
        ```

        you should almost certainly write the code this way instead:

        ```js
        export function runCodeWithFeatureFlags(code) {
          let featureFlags = {...}
          let fn = new Function('featureFlags', code)
          fn(featureFlags)
        }
        ```

        This still gives `code` access to `featureFlags` but avoids all of the negative consequences of bundling code with direct `eval`.

## 0.8.53

* Support chunk and asset file name templates ([#733](https://github.com/evanw/esbuild/issues/733), [#888](https://github.com/evanw/esbuild/issues/888))

    This release introduces the `--chunk-names=` and `--asset-names=` flags. These flags let you customize the output paths for chunks and assets within the output directory. Each output path is a template and currently supports these placeholders:

    * `[name]`: The original name of the file. This will be `chunk` for chunks and will be the original file name (without the extension) for assets.
    * `[hash]`: The content hash of the file. This is not necessarily stable across different esbuild versions but will be stable within the same esbuild version.

    For example, if you want to move all chunks and assets into separate subdirectories, you could use `--chunk-names=chunks/[name]-[hash]` and `--asset-names=assets/[name]-[hash]`. Note that the path template should not include the file extension since the file extension is always automatically added to the end of the path template.

    Additional name template features are planned in the future including a `[dir]` placeholder for the relative path from the `outbase` directory to the original input directory as well as an `--entry-names=` flag, but these extra features have not been implemented yet.

* Handle `this` in class static field initializers ([#885](https://github.com/evanw/esbuild/issues/885))

    When you use `this` in a static field initializer inside a `class` statement or expression, it references the class object itself:

    ```js
    class Foo {
      static Bar = class extends this {
      }
    }
    assert(new Foo.Bar() instanceof Foo)
    ```

    This case previously wasn't handled because doing this is a compile error in TypeScript code. However, JavaScript does allow this so esbuild needs to be able to handle this. This edge case should now work correctly with this release.

* Do not warn about dynamic imports when `.catch()` is detected ([#893](https://github.com/evanw/esbuild/issues/893))

    Previously esbuild avoids warning about unbundled `import()` expressions when using the `try { await import(_) }` pattern, since presumably the `try` block is there to handle the run-time failure of the `import()` expression failing. This release adds some new patterns that will also suppress the warning: `import(_).catch(_)`, `import(_).then(_).catch(_)`, and `import(_).then(_, _)`.

* CSS namespaces are no longer supported

    [CSS namespaces](https://developer.mozilla.org/en-US/docs/Web/CSS/@namespace) are a weird feature that appears to only really be useful for styling XML. And the world has moved on from XHTML to HTML5 so pretty much no one uses CSS namespaces anymore. They are also complicated to support in a bundler because CSS namespaces are file-scoped, which means:

    * Default namespaces can be different in different files, in which case some default namespaces would have to be converted to prefixed namespaces to avoid collisions.

    * Prefixed namespaces from different files can use the same name, in which case some prefixed namespaces would need to be renamed to avoid collisions.

    Instead of implementing all of that for an extremely obscure feature, CSS namespaces are now just explicitly not supported. The code to handle `@namespace` has been removed from esbuild. This will likely not affect anyone, especially because bundling code using CSS namespaces with esbuild didn't even work correctly in the first place.

## 0.8.52

* Fix a concurrent map write with the `--inject:` feature ([#878](https://github.com/evanw/esbuild/issues/878))

    This release fixes an issue where esbuild could potentially crash sometimes with a concurrent map write when using injected files and entry points that were neither relative nor absolute paths. This was an edge case where esbuild's low-level file subsystem was being used without being behind a mutex lock. This regression was likely introduced in version 0.8.21. The cause of the crash has been fixed.

* Provide `kind` to `onResolve` plugins ([#879](https://github.com/evanw/esbuild/issues/879))

    Plugins that add `onResolve` callbacks now have access to the `kind` parameter which tells you what kind of import is being resolved. It will be one of the following values:

    * `"entry-point"` in JS (`api.ResolveEntryPoint` in Go)

        An entry point provided by the user

    * `"import-statement"` in JS (`api.ResolveJSImportStatement` in Go)

        A JavaScript `import` or `export` statement

    * `"require-call"` in JS (`api.ResolveJSRequireCall` in Go)

        A JavaScript call to `require(...)` with a string argument

    * `"dynamic-import"` in JS (`api.ResolveJSDynamicImport` in Go)

        A JavaScript `import(...)` expression with a string argument

    * `"require-resolve"` in JS (`api.ResolveJSRequireResolve` in Go)

        A JavaScript call to `require.resolve(...)` with a string argument

    * `"import-rule"` in JS (`api.ResolveCSSImportRule` in Go)

        A CSS `@import` rule

    * `"url-token"` in JS (`api.ResolveCSSURLToken` in Go)

        A CSS `url(...)` token

    These values are pretty much identical to the `kind` field in the JSON metadata file.

## 0.8.51

* The stderr log format now contains line numbers after file names ([#865](https://github.com/evanw/esbuild/issues/865))

    Error messages in stderr now have a line and column number after the file name.

    Before:

    ```
     > src/structs/RTree.js: warning: Duplicate key "compareMinX" in object literal
        469 │     compareMinX: function (a, b)
            ╵     ~~~~~~~~~~~
       src/structs/RTree.js: note: The original "compareMinX" is here
        206 │     compareMinX: compareNodeMinX,
            ╵     ~~~~~~~~~~~
    ```

    After:

    ```
     > src/structs/RTree.js:469:4: warning: Duplicate key "compareMinX" in object literal
        469 │     compareMinX: function (a, b)
            ╵     ~~~~~~~~~~~
       src/structs/RTree.js:206:4: note: The original "compareMinX" is here
        206 │     compareMinX: compareNodeMinX,
            ╵     ~~~~~~~~~~~
    ```

    This should make log messages slightly easier to parse if you want to parse stderr instead of using esbuild's API. Previously you needed a multi-line regular expression to get the line number, but now that the line number is duplicated in two places you should only need a single-line regular expression.

    Note that this is still the hacky way to get error information and is potentially unstable, since it will break if the log format changes. Log messages are mainly intended for humans. The straightforward and stable way to do this is still to use esbuild's API, which returns log messages as an array of objects.

* Allow `--define` with `import.meta`

    The `--define` feature lets you replace specific identifiers and member expression chains with compile-time constants. However, it previously didn't work with `import.meta` because this is a special case in the grammar. The `import` keyword is not actually an identifier expression. This distinction isn't helpful though, and it's not unreasonable to want to use the `--define` feature to replace `import.meta` properties too.

    With this release, it's now possible to use e.g. `--define:import.meta.foo=123` to replace specific properties accessed off of the `import.meta` object as well as to use e.g. `--define:import.meta={\"foo\":123}` to substitute the entire `import.meta` expression with something else.

* Fix a race condition with multiple injected files ([#871](https://github.com/evanw/esbuild/issues/871))

    Using multiple injected files could cause a data race that trips Go's race detector. The data race has been fixed in this release. The fix was contributed by [@Deleplace](https://github.com/Deleplace).

* Change `--serve` behavior to serve on all interfaces ([#866](https://github.com/evanw/esbuild/issues/866))

    The default address for the `--serve` flag has changed from `127.0.0.1` (serve on the loopback interface) to `0.0.0.0` (serve on all interfaces). You can still manually specify either one using `--serve=127.0.0.1:8000` or `--serve=0.0.0.0:8000`. This just changes the default behavior that happens when you pass `--serve` with no host address (or when you just use the `--servedir=` flag without `--serve=`).

    In addition, you can now also specify an IPv6 address. Previously there was a parsing issue that prevented this. For example, you can pass `--serve=[::1]:8000` to serve on the loopback interface and `--serve=[::]:8000` to serve on all interfaces.

* Change the import resolution rules of absolute paths ([#862](https://github.com/evanw/esbuild/issues/862))

    Previously absolute paths were considered to be pre-resolved by the resolver (in contrast to relative and package paths, which need to be converted to an absolute path). This meant that absolute paths which did not actually exist caused a failure in the loader when it tried to load the path instead of in the resolver when it tried to resolve the path.

    With the previous change in version 0.8.47 to support removing URL query and/or hash parameters from the path, path resolution can now be run multiple times. If path resolution fails and the path contains a `?` and/or `#`, path resolution is re-run with the URL query/hash parameters removed. It is problematic to consider absolute paths to be pre-resolved because it means that paths containing query/hash parameters make the loader try to load the wrong path, and do not run the resolver again with the parameter suffix removed.

    In this release, esbuild will now validate absolute paths in the resolver. So invalid paths will now fail in the resolver and retry without the parameter suffix instead of failing in the loader, which correctly handles a parameter suffix on absolute paths. In addition, this release now handles implicit file extensions on absolute paths. This makes esbuild a more accurate copy of [node's module resolution algorithm](https://nodejs.org/api/modules.html#modules_all_together), which does this as well.

* Output files in `metafile` now have `entryPoint` ([#711](https://github.com/evanw/esbuild/issues/711))

    There is now an optional `entryPoint` property on each output file in the JSON metadata file generated with the `--metafile=` flag. It is only present for output files that are the bundled results of entry point files, and contains the path name of the corresponding input entry point file. This property is not present on other kinds of output files (e.g. code splitting chunks). This feature was contributed by [@remorses](https://github.com/remorses).

## 0.8.50

* Using direct `eval` now pulls in `module` and `exports`

    Use of direct `eval` forces the file to become a CommonJS module and disables dead code elimination in the entire file. The CommonJS closure is necessary to avoid name collisions with other modules, since `eval` means symbols in the file can no longer be renamed to avoid collisions.

    However, the CommonJS `module` and `exports` variables that are arguments to the closure previously weren't considered to be used in this scenario, meaning they may be omitted as dead code for size reasons. This could cause code inside `eval` to behave incorrectly. Now use of direct `eval` automatically counts as a use of both `module` and `exports` so these variables should now always be present in this case.

* Always remove all `"use asm"` directives ([#856](https://github.com/evanw/esbuild/issues/856))

    The asm.js subset of JavaScript has complicated validation rules that are triggered by this directive. The parser and code generator in esbuild was not designed with asm.js in mind and round-tripping asm.js code through esbuild will very likely cause it to no longer validate as asm.js. When this happens, V8 prints a warning and people don't like seeing the warning. The warning looks like this:

    ```
    (node:58335) V8: example.js:3 Invalid asm.js: Unexpected token
    (Use `node --trace-warnings ...` to show where the warning was created)
    ```

    I am deliberately not attempting to preserve the validity of asm.js code because it's a complicated legacy format and it's obsolete now that WebAssembly exists. By removing all `"use asm"` directives, the code will just become normal JavaScript and work fine without generating a warning.

* Fix a variable hoisting edge case ([#857](https://github.com/evanw/esbuild/issues/857))

    It is allowed to use a nested `var` hoisted declaration with the same name as a top-level function declaration. In that case the two symbols should merge and be treated as the same symbol:

    ```js
    async function x() {}
    {
      var x;
    }
    ```

    The parser previously allowed this for regular functions but not for async or generator functions. Now with this release, this behavior is also allowed for these special kinds of functions too.

* Remove empty CSS rules when minifying ([#851](https://github.com/evanw/esbuild/pull/851))

    Empty rules with no content such as `div {}` are now removed when CSS is minified. This change was contributed by [@susiwen8](https://github.com/susiwen8).

## 0.8.49

* Work around a problem with `pnpm` and `NODE_PATH` ([#816](https://github.com/evanw/esbuild/issues/816))

    In version 0.8.43, esbuild added support for node's [`NODE_PATH`](https://nodejs.org/api/modules.html#modules_loading_from_the_global_folders) environment variable which contains a list of global folders to use during path resolution. However, this causes a problem when esbuild is installed with [pnpm](https://pnpm.js.org/), an alternative JavaScript package manager. Specifically pnpm adds a bogus path to `NODE_PATH` that doesn't exist but that has a file as a parent directory. Previously this caused esbuild to fail with the error `not a directory`. Now with this release, esbuild will ignore this bogus path instead of giving an error.

* Add more names to the global no-side-effect list ([#842](https://github.com/evanw/esbuild/issues/842))

    This release adds almost all known globals from the browser and node to the list of known globals. Membership in this list means accessing the global is assumed to have no side effects. That means tree shaking is allowed to remove unused references to these globals. For example, since `HTMLElement` is now in the known globals list, the following class will now be removed when unused:

    ```js
    class MyElement extends HTMLElement {
    }
    ```

    In addition, membership in this list relaxes ordering constraints for the purposes of minification. It allows esbuild to reorder references to these globals past other expressions. For example, since `console.log` is now in the known globals list, the following simplification will now be performed during minification:

    ```js
    // Original
    export default (a) => {
      if (a) console.log(b); else console.log(c)
    }

    // Minified (previous release)
    export default (a) => {
      a ? console.log(b) : console.log(c);
    };

    // Minified (this release)
    export default (a) => {
      console.log(a ? b : c);
    };
    ```

    This transformation is not generally safe because the `console.log` property access might evaluate code which could potentially change the value of `a`. This is only considered safe in this instance because `console.log` is now in the known globals list.

    Note that membership in this list does not say anything about whether the function has side effects when called. It only says that the identifier has no side effects when referenced. So `console.log()` is still considered to have side effects even though `console.log` is now considered to be free of side effects.

    The following globals are not on the list and are considered to have side effects:

    * `scrollX`
    * `scrollY`
    * `innerWidth`
    * `innerHeight`
    * `pageXOffset`
    * `pageYOffset`
    * `localStorage`
    * `sessionStorage`

    Accessing layout-related properties can trigger a layout and accessing storage-related properties can throw an exception if certain privacy settings are enabled. Both of these behaviors are considered side effects.

* Fix a TypeScript parser regression ([#846](https://github.com/evanw/esbuild/issues/846))

    Restrictions on array and object destructuring patterns in the previous release introduced a regression where arrays or objects in TypeScript code could fail to parse if they were wrapped in a double layer of parentheses. This was due to the speculative parsing of arrow function arguments. The regression has been fixed.

* Add the Go-specific `cli.ParseServeOptions()` API ([#834](https://github.com/evanw/esbuild/issues/834))

    This API is specifically for people trying to emulate esbuild's CLI in Go. It lets you share esbuild's logic of parsing the `--serve=` and `--servedir=` flags. Use it like this:

    ```go
    serveOptions, args, err := cli.ParseServeOptions([]string{
      "--serve=8000",
    })
    buildOptions, err := cli.ParseBuildOptions(args)
    result := api.Serve(serveOptions, buildOptions)
    ```

## 0.8.48

* Fix some parsing edge cases ([#835](https://github.com/evanw/esbuild/issues/835))

    This release fixes the following edge cases:

    * Code using `in` inside a template literal inside a for loop initializer such as ``for (let x = `${a in b ? '0' : '1'}`; false; );`` is now allowed. Previously the `in` operator was incorrectly considered to be part of a for-in loop.

    * In TypeScript, it's not valid to have a newline in between the `async` and the `<` tokens inside the code `async <T>() => {}`. Previously this was incorrectly treated as an asynchronous arrow function expression.

    * Code of the form `new async()` must construct the function called `async`. Previously this was incorrectly treated as `new (async())()` instead due to the speculative parsing of asynchronous arrow functions.

    * Code of the form `new async () => {}` must not be allowed. Previously this was incorrectly allowed since the speculative parsing of asynchronous arrow functions did not check the precedence level.

    * It's not valid to start an initializer expression in a for-of loop with the token `let` such as `for (let.foo of bar) {}`. This is now forbidden. In addition, the code generator now respects this rule so `for ((let.foo) of bar) {}` is now printed as `for ((let).foo of bar) {}`.

    * Array and object binding patterns do not allow a comma after rest elements, so code such as `[...a, b] = [c]` is invalid. This case is correctly handled by esbuild. However, it's possible to have both an array or object binding pattern and an array or object literal on the left-hand side of a destructuring assignment such as `[[...a, b].c] = [d]`. In that case it should be allowed for a comma to come after the spread element in the array or object literal expression. Previously this was incorrectly treated as an error by esbuild.

    * It's technically allowed (although perhaps not ever actually useful) to call `super()` from within a default argument initializer like this:

        ```js
        class Derived extends Base {
          constructor(arg = super()) {
          }
        }
        ```

        Previously esbuild did not permit this, which is incorrect. Doing this is now permitted.

    * It is an error to use `arguments` in a class field initializer such as `class { x = arguments[0] }`, but it is not an error to use `arguments` in a computed class property name such as `class { [arguments[0]] = x }` or inside TypeScript decorators such as `class { @decorator(arguments[0]) x() {} }`. Previously all of these cases were an error in esbuild, which is incorrect. Using `arguments` inside computed class property names and TypeScript decorators is now allowed.

    * It is not permitted to use a function declaration inside an if statement such as `if (0) function f() {}` in strict mode. Previously this was allowed, but this is now forbidden.

    * It is not permitted to re-declare a generator and/or asynchronous function declaration inside a block scope:

        ```js
        // This is allowed
        function *a() {}
        function *a() {}

        // This is allowed
        function f() {
          function *b() {}
          function *b() {}
        }

        // This is not allowed
        {
          function *c() {}
          function *c() {}
        }
        ```

        The parser now enforces this rule.

    * Legacy octal escape sequences are octal escape sequences other than `\0` with a single zero. These are forbidden in untagged template literals and in all strings in strict mode code. Previously esbuild didn't enforce this rule, but it is now enforced.

    * Technically the directive prologue is allowed to contain multiple directives, so strict mode should still be applied even if a `"use strict";` directive is preceded by another directive. For example, `"use \000"; "use strict";` should be a syntax error because strict mode is active. This technicality has now been implemented.

    * It is supposed to be a syntax error if a use strict directive is inside a function with a non-simple parameter list, such as `(x = 1) => { 'use strict' }`. Previously esbuild allowed this code, but now this code is a syntax error.

    * It is forbidden for a template literal tag to be an optional chain such as `` a?.b`c` ``. This rule is now enforced by esbuild, so code like this is now a syntax error. In addition, the code generator now avoids generating this syntax by wrapping any optional chain template literal tags in parentheses.

    * According to the standard, all code inside a class statement or expression should be in strict mode. Previously esbuild treated code inside a class as the same strict mode status as the surrounding code, but now code in a class is always interpreted as strict mode code.

    * Duplicate bindings in the same parameter list are not allowed if the parameter list isn't simple, such as in the code `function f(a, [a]) {}`, or if the parameter list belongs to an arrow function or a method. This rule is now enforced by esbuild's parser, so doing this is now a syntax error.

    * Array and object destructuring patterns are only valid if they are not surrounded by parentheses. Previously esbuild incorrectly allowed code such as `([]) = []` and `({}) = {}`. This invalid code is now a syntax error.

    * It is now an error to use the shorthand property syntax `({yield})` inside a generator and `({await})` inside an asynchronous function. Previously those cases were incorrectly allowed.

    * A newline in between `async` and a method name is no longer allowed. Instead, this is a syntax error inside an object literal and a class field inside a class body.

* Remove the local web server feature from the WebAssembly package ([#836](https://github.com/evanw/esbuild/issues/836))

    This feature didn't work anyway (maybe sockets don't work with Go's WebAssembly target?) and including it added around 3mb of unnecessary extra code to the WebAssembly module file. Removing this brings the size of the WebAssembly module from around 11mb down to 8.3mb.

## 0.8.47

* Release native binaries for the Apple M1 chip ([#550](https://github.com/evanw/esbuild/issues/550))

    Previously installing esbuild on a M1 actually installed the x86-64 version, which required the Rosetta 2 translator. This was because Go hadn't yet released support for the M1. Now that Go 1.16.0 has been released, esbuild can support the M1 natively. It's supported by esbuild starting with this release. There are reports of the native version being 1.4x faster than the translated version. This change was contributed by [@rtsao](https://github.com/rtsao).

* Omit warning about `require.someProperty` when targeting CommonJS ([#812](https://github.com/evanw/esbuild/issues/812))

    The `require.cache` property allows introspecting the state of the `require` cache, generally without affecting what is imported/bundled.

    Since esbuild's static analyzer only detects direct calls to `require`, it currently warns about uses of `require` in any situation other than a direct call since that means the value is "escaping" the analyzer. This is meant to detect and warn about indirect calls such as `['fs', 'path'].map(require)`.

    However, this warning is not relevant when accessing a property off of the `require` object such as `require.cache` because a property access does not result in capturing the value of `require`. Now a warning is no longer generated for `require.someProperty` when the output format is `cjs`. This allows for the use of features such as `require.cache` and `require.extensions`. This fix was contributed by [@huonw](https://github.com/huonw).

* Support ignored URL parameters at the end of import paths ([#826](https://github.com/evanw/esbuild/issues/826))

    If path resolution fails, ebuild will now try again with the URL query and/or fragment removed. This helps handle ancient CSS code like this that contains hacks for Internet Explorer:

    ```css
    @font-face {
      src:
        url("./themes/default/assets/fonts/icons.eot?#iefix") format('embedded-opentype'),
        url("./themes/default/assets/fonts/icons.woff2") format('woff2'),
        url("./themes/default/assets/fonts/icons.woff") format('woff'),
        url("./themes/default/assets/fonts/icons.ttf") format('truetype'),
        url("./themes/default/assets/fonts/icons.svg#icons") format('svg');
    }
    ```

    Previously path resolution would fail because these files do not end with the `.eot?#iefix` or `.svg#icons` extensions. Now path resolution should succeed. The URL query and fragment are not unconditionally stripped because there is apparently [code in the wild that uses `#` as a directory name](https://github.com/medikoo/es5-ext/tree/3ddd2066b19e7c25a782869a304ae35d8188c8f1/string/%23). So esbuild will still try to resolve the full import path first and only try to reinterpret the path as a URL if that fails.

* Prevent paths starting with `/` from being used as relative paths on Windows ([#822](https://github.com/evanw/esbuild/issues/822))

    On Windows, absolute paths start with a drive letter such as `C:\...` instead of with a slash like `/...`. This means that paths starting with a `/` can actually be used as relative paths. For example, this means an import of `/subfolder/image.png` will match the file at the path `./subfolder/image.png`. This is problematic for Windows users because they may accidentally make use of these paths and then try to run their code on a non-Windows platform only for it to fail to build.

    Now paths starting with a `/` are always treated as an absolute path on all platforms. This means you can no longer import files at a relative path that starts with `/` on Windows. You should be using a `./` prefix instead.

* Warn when importing a path with the wrong case

    Importing a path with the wrong case (e.g. `File.js` instead of `file.js`) will work on Windows and sometimes on macOS because they have case-insensitive file systems, but it will never work on Linux because it has a case-sensitive file system. To help you make your code more portable and to avoid cross-platform build failures, esbuild now issues a warning when you do this.

## 0.8.46

* Fix minification of `.0` in CSS ([#804](https://github.com/evanw/esbuild/issues/804))

    If you write `.0` instead of `0` in CSS and enabled `--minify`, esbuild would previously minify this token incorrectly (the token was deleted). This bug has been fixed and esbuild should now minify this token to `0`.

* Support range requests in local HTTP server

    The local HTTP server built in to esbuild now supports [range requests](https://developer.mozilla.org/en-US/docs/Web/HTTP/Range_requests), which are necessary for video playback in Safari. This means you can now use `<video>` tags in your HTML pages with esbuild's local HTTP server.

## 0.8.45

* Add the `--servedir=` flag ([#796](https://github.com/evanw/esbuild/issues/796))

    The `--serve` flag starts a local web server and serves the files that would normally be written to the output directory. So for example if you had an entry point called `src/app.ts` and an output directory of `--outdir=www/js`, using esbuild with `--serve` would expose the generated output file via http://localhost:8000/app.js (but not write anything to `www/js`). This can then be used in combination with your normal development server (running concurrently on another port) by adding `<script src="http://localhost:8000/app.js"></script>` in your HTML file. So esbuild with the `--serve` flag is meant to augment your normal development server, not replace it.

    This release introduces a new `--servedir=` flag which gives you the option of replacing your normal development server with esbuild. The directory you pass here will be "underlayed" below the output directory. Specifically when an incoming HTTP request comes in esbuild will first check if it matches one of the generated output files and if so, serve the output file directly from memory. Otherwise esbuild will fall back to serving content from the serve directory on the file system. In other words, server's URL structure behaves like a normal file server in a world where esbuild had written the generated output files to the file system (even though the output files actually only exist in memory).

    So for example if you had an entry point called `src/app.ts` and an output directory of `--outdir=www/js`, using esbuild with `--servedir=www` would expose the entire contents of the `www` directory via http://localhost:8000/ except for the http://localhost:8000/js/app.js URL which would contain the compiled contents of `src/app.ts`. This lets you have a `www/index.html` file containing just `<script src="/js/app.js"></script>` and use one web server instead of two.

    The benefit of doing things this way is that you can use the exact same HTML pages in development and production. In development you can run esbuild with `--servedir=` and esbuild will serve the generated output files directly. For production you can omit that flag and esbuild will write the generated files to the file system. In both cases you should be getting the exact same result in the browser with the exact same code in both development and production.

    This will of course not support all workflows, but that's intentional. This is designed to be a quality-of-life improvement for the simple case of building a small static website with some HTML, JavaScript, and CSS. More advanced setups may prefer to avoid the `--servedir=` feature and e.g. configure a NGINX reverse proxy to esbuild's local server to integrate esbuild into a larger existing development setup.

    One unintended consequence of this feature is that esbuild can now be used as a general local HTTP server via `esbuild --servedir=.`. Without any entry points, esbuild won't actually build anything and will just serve files like a normal web server. This isn't the intended use case but it could perhaps be a useful side effect of this feature.

* Remove absolute paths for disabled packages from source maps ([#786](https://github.com/evanw/esbuild/issues/786))

    This change is similar to the one from the previous release for disabled files, but it applies to package paths instead of relative paths. It's relevant when using packages that override dependencies with alternative packages using the `browser` field in their `package.json` file. Using relative paths instead of absolute paths fixes a determinism issue where build output was different on different systems. This fix was contributed by [@eelco](https://github.com/eelco).

* Handle absolute paths in `tsconfig.json` ([#792](https://github.com/evanw/esbuild/issues/792))

    Some automatically-generated `tsconfig.json` paths can have absolute paths in them. This is allowed by the TypeScript compiler (specifically in the `paths` and `extends` fields). With this release, esbuild now supports absolute paths in `paths` and `extends` too.

* Change the watch mode output format ([#793](https://github.com/evanw/esbuild/issues/793))

    Previously esbuild would print a "..." animation to the console while watch mode was scanning for changes. The intent of this was to a) not take up too much space in the terminal and b) show that esbuild's watch mode isn't frozen. Since the release I have gotten feedback that this isn't desirable. People want more feedback about what's happening and want to be able to run regexes over the stderr stream instead of using esbuild's actual API.

    This release changes the output format for watch mode. Now esbuild will print `[watch] build started` when watch mode triggers a rebuild and `[watch] build finished` when the rebuild is complete. Any build errors will be printed in between those two log messages.

    Note that this means esbuild's watch mode output is now more verbose, especially when there are frequent file changes. If you want to hide these new messages you can use `--log-level=` with a level other than `info`.

## 0.8.44

* Create a logo for esbuild ([#61](https://github.com/evanw/esbuild/issues/61))

    This release introduces a logo for esbuild:

    <p>
      &nbsp; &nbsp; &nbsp;
      <img width="100" height="100" src="https://esbuild.github.io/favicon.svg">
    </p>

    Inspirations for the logo include:

    * **The fast-forward symbol** because esbuild is extremely fast and because one of esbuild's goals is to accelerate the evolution of the whole web tooling ecosystem.

    * **The right-shift symbol** because esbuild's production optimizations make your code smaller and because esbuild itself contains many low-level optimizations for speed.

    Having a logo for esbuild should make it easier to include esbuild in lists of other tools since the other tools often all have logos.

* Add support for node's `--preserve-symlinks` flag ([#781](https://github.com/evanw/esbuild/issues/781))

    This release adds the `--preserve-symlinks` flag which behaves like [the corresponding flag in node](https://nodejs.org/api/cli.html#cli_preserve_symlinks). Without the flag, esbuild and node will use the real path (after resolving symlinks) as the identity of a file. This means that a given file can only be instantiated once. With the flag, esbuild and node will use the original path (without resolving symlinks) as the identity of a file. This means that a given file can be instantiated multiple times, once for every symlink pointing to it. Each copy will have its own identity so the resulting bundle may contain duplicate files. This option is useful if your code relies on this flag in node (or the [`resolve.symlinks` setting in Webpack](https://webpack.js.org/configuration/resolve/#resolvesymlinks)).

* Ignore a leading byte order mark (BOM) in CSS files ([#776](https://github.com/evanw/esbuild/issues/776))

    Some text editors insert a U+FEFF code point at the start of text files. This is a zero-width non-breaking space character. Using one at the start of a file is a convention which is meant to indicate that the contents of the file are UTF-8 encoded. When this is done, the character is called a [byte order mark](https://en.wikipedia.org/wiki/Byte_order_mark).

    Unlike JavaScript, CSS does not treat U+FEFF as whitespace. It is treated as an identifier instead. This was causing esbuild to misinterpret files starting with a BOM as starting with an extra identifier, which could then cause the initial CSS rule in the file to be parsed incorrectly.

    Now esbuild will skip over a BOM if it's present before beginning to parse CSS. This should prevent issues when working with these files.

* Add message notes to the API

    The internal logging system has the ability to attach additional notes to messages to provide more information. These show up as additional log messages in the terminal when using the command-line interface. Here is an example of a note:

    ```
     > src/structs/RTree.js: warning: Duplicate key "compareMinX" in object literal
        469 │     compareMinX: function (a, b)
            ╵     ~~~~~~~~~~~
       src/structs/RTree.js: note: The original "compareMinX" is here
        206 │     compareMinX: compareNodeMinX,
            ╵     ~~~~~~~~~~~
    ```

    With this release, notes are also supported in the JS and Go APIs. This means you can now generate your own notes using plugins as well as inspect the notes generated by esbuild.

* Add origin information to errors from plugins ([#780](https://github.com/evanw/esbuild/issues/780))

    Errors thrown during JavaScript plugin callback evaluation will now be annoated to show where that plugin callback was registered. That looks like this:

    ```
     > example-plugin.js: error: [example-plugin] foo.bar is not a function
        15 │         foo.bar();
           ╵             ^
        at ./example-plugin.js:15:13
        at ./node_modules/esbuild/lib/main.js:750:34

       example-plugin.js: note: This error came from the "onLoad" callback registered here
        13 │       build.onLoad({ filter: /.*/ }, args => {
           ╵             ~~~~~~
        at setup (./example-plugin.js:13:13)
        at handlePlugins (./node_modules/esbuild/lib/main.js:668:7)
    ```

    This should make it easier to debug crashes in plugin code.

* Fix a regression with the synchronous JavaScript API ([#784](https://github.com/evanw/esbuild/issues/784))

    In version 0.8.39, a change was made to avoid dangling esbuild processes when node exits abnormally. The change introduced a periodic ping between the child esbuild process and its host process. If the ping doesn't go through, the child process is able to detect that the host process is no longer there. Then it knows to exit since it's no longer being used.

    This caused a problem with the synchronous JavaScript API calls which run the esbuild child process in a single-response mode. The ping message was interpreted as a second response and tripped up the message protocol. Pings are only useful for the asynchronous API calls. Running the pings during synchronous API calls was unintentional. With this release pings are no longer run for synchronous API calls so this regression should be fixed.

* Remove absolute paths for disabled files from source maps ([#785](https://github.com/evanw/esbuild/issues/785))

    Files can be ignored (i.e. set to empty) using the [`browser` field in `package.json`](https://github.com/defunctzombie/package-browser-field-spec/tree/4f296871cee64e60124841c06c06511885152f19#ignore-a-module). Specifically, you can set the `browser` field to a map where the key is the module name and the value is `false`. This is a convention followed by several bundlers including esbuild.

    Previously ignoring a file caused that file's path to appear as an absolute path in any generated source map. This is problematic because it means different source maps will be generated on different systems, since the absolute path contains system-specific directory information. Now esbuild will treat these paths the same way it treats other paths and will put a relative path in the source map.

## 0.8.43

* Support the `XDG_CACHE_HOME` environment variable ([#757](https://github.com/evanw/esbuild/issues/757))

    On Linux, the install script for esbuild currently caches downloaded binary executables in `~/.cache/esbuild/bin`. This change means esbuild will now try installing to `$XDG_CACHE_HOME/esbuild/bin` instead of the `XDG_CACHE_HOME` environment variable exists. This allows you to customize the cache directory on Linux. The specification that defines `XDG_CACHE_HOME` is [here](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html).

* Further improve constant folding of branches ([#765](https://github.com/evanw/esbuild/issues/765))

    At a high level, this release adds the following substitutions to improve constant folding and dead code elimination:

    * `if (anything && falsyWithSideEffects)` → `if (anything, falsyWithSideEffects)`
    * `if (anything || truthyWithSideEffects)` → `if (anything, truthyWithSideEffects)`
    * `if (anything && truthyNoSideEffects)` → `if (anything)`
    * `if (anything || falsyNoSideEffects)` → `if (anything)`
    * `if (anything, truthyOrFalsy)` → `anything; if (truthyOrFalsy)`

    And also these substitutions for unused expressions:

    * `primitive == primitive` → `primitive, primitive`
    * `typeof identifier` → (remove entirely)

    The actual substitutions are more complex since they are more comprehensive but they essentially result in this high-level behavior. Note that these substitutions are only done when minification is enabled.

* Fix an edge case with CSS variable syntax ([#760](https://github.com/evanw/esbuild/issues/760))

    CSS variables are whitespace-sensitive even though other CSS syntax is mostly not whitespace sensitive. It is apparently common for this to cause problems with CSS tooling that pretty-prints and minifies CSS, including esbuild before this release. Some examples of issues with other tools include [postcss/postcss#1404](https://github.com/postcss/postcss/issues/1404) and [tailwindlabs/tailwindcss#2889](https://github.com/tailwindlabs/tailwindcss/issues/2889). The issue affects code like this:

    ```css
    div {
      --some-var: ;
      some-decl: var(--some-var, );
    }
    ```

    It would be a change in semantics to minify this code to either `--some-var:;` or `var(--some-var,)` due to the whitespace significance of CSS variables, so such transformations are invalid. With this release, esbuild should now preserve whitespace in these two situations (CSS variable declarations and CSS variable references).

* Add support for recursive symlinks during path resolution ([#766](https://github.com/evanw/esbuild/issues/766))

    Previously recursive symlinks (a symlink that points to another symlink) were an unhandled case in the path resolution algorithm. Now these cases should be supported up to a depth of 256 symlinks. This means esbuild's path resolution should now work with multi-level `yarn link` scenarios.

* Fix subtle circular dependency issue ([#758](https://github.com/evanw/esbuild/issues/758))

    If esbuild is used to transform TypeScript to JavaScript without bundling (i.e. each file is transformed individually), the output format is CommonJS, and the original TypeScript code contains an import cycle where at least one of the links in the cycle is an `export * as` re-export statement, there could be certain situations where evaluating the transformed code results in an import being `undefined`. This is caused by the `__esModule` marker being added after the call to `require()` for the first transformed re-export statement. The fix was to move the marker to before the first call to `require()`. The `__esModule` marker is a convention from Babel that esbuild reuses which marks a module as being originally in the ECMAScript module format instead of the CommonJS module format.

* Add support for the `NODE_PATH` environment variable

    This is a rarely-used feature of Node's module resolution algorithm. From [the documentation](https://nodejs.org/api/modules.html#modules_loading_from_the_global_folders):

    > If the `NODE_PATH` environment variable is set to a colon-delimited list of absolute paths, then Node.js will search those paths for modules if they are not found elsewhere.
    >
    > On Windows, `NODE_PATH` is delimited by semicolons (`;`) instead of colons.

    The CLI takes the list of node paths from the value of the `NODE_PATH` environment variable, but the JS and Go APIs take the list as an array of strings instead (called `nodePaths` in JS and `NodePaths` in Go).

## 0.8.42

* Fix crash with block-level function declaration and `--keep-names` ([#755](https://github.com/evanw/esbuild/issues/755))

    This release fixes a crash with block-level function declarations and the `--keep-names` option. The crash affected code that looks like this:

    ```js
    if (true) function f() {}
    assert.strictEqual(f.name, 'f')
    ```

* Disallow additional features in strict mode

    This change improves esbuild's compliance with the JavaScript specification. It is now an error to use legacy octal numeric literals and the identifiers `implements`, `interface`, `let`, `package`, `private`, `protected`, `public`, `static`, and `yield` in strict mode code.

* Basic support for watch mode with plugins ([#752](https://github.com/evanw/esbuild/issues/752))

    With this release, watch mode should now work with simple [on-load plugins](https://esbuild.github.io/plugins/#load-callbacks). Watch mode is implemented by tracking all file system accesses made by esbuild as it does a build. However, this doesn't catch external file system accesses such as those made by plugins. Now if an on-load plugin is used on a path in the `file` namespace, esbuild will also read the file during watch mode so that watch mode is aware of the file system access. Note that there is not yet API support for a plugin to return additional paths for watch mode to monitor.

* Make JavaScript API error format more consistent ([#745](https://github.com/evanw/esbuild/issues/745))

    If a JavaScript error is thrown while validating the build options, the thrown error should now have `errors` and `warnings` properties just like normal build errors. Previously these properties were only present if the build itself failed but not if build options were invalid. This consistency should make it easier to process errors from the build API call.

## 0.8.41

* Fix memory leak with watch mode when using the CLI ([#750](https://github.com/evanw/esbuild/issues/750))

    This release fixes a memory leak when using `--watch` from the CLI (command-line interface). When esbuild was in this state, every incremental build resulted in more memory being consumed. This problem did not affect users of the JS API or Go API, only users of the CLI API.

    The problem was that the GC (garbage collector) was disabled. Oops. This is done by default for speed when you use esbuild via the CLI, which makes sense for most CLI use cases because the process is usually short-lived and doesn't need to waste time cleaning up memory. But it does not make sense for flags that cause esbuild to be a long-running process.

    Previously the only exception to this rule was the `--serve` flag. When I added watch mode, I forgot to enable GC for the `--watch` flag too. With this release, the GC is enabled for both the `--serve` and the `--watch` flags so esbuild should no longer leak memory in watch mode.

* Special-case certain syntax with `--format=esm` ([#749](https://github.com/evanw/esbuild/issues/749))

    You can now no longer use the following syntax features with the `esm` output format:

    * The `with` statement: `with (x) {}`
    * Delete of a bare identifier: `delete x`

    In addition, the following syntax feature is transformed when using the `esm` output format:

    * For-in variable initializers: `for (var x = y in {}) {}` → `x = y; for (var x in {}) {}`

    The reason is because all JavaScript engines interpret code in the `esm` output format as strict mode and these syntax features are disallowed in strict mode. Note that this new strict mode handling behavior in esbuild is only dependent on the output format. It does not depend on the presence or absence of `"use strict"` directives.

* Basic `"use strict"` tracking

    The JavaScript parser now tracks `"use strict"` directives and propagates strict mode status through the code. In addition, files containing the `import` and/or `export` keywords are also considered to be in strict mode. Strict mode handling is complex and esbuild currently doesn't implement all strict mode checks. But the changes in this release are a starting point. It is now an error to use certain syntax features such as a `with` statement within a strict mode scope.

* Fix a minifier bug with `with` statements

    The minifier removes references to local variables if they are unused. However, that's not correct to do inside a `with` statement scope because what appears to be an identifier may actually be a property access, and property accesses could have arbitrary side effects if they resolve to a getter or setter method. Now all identifier expressions inside `with` statements are preserved when minifying.

* Transform block-level function declarations

    Block-level function declarations are now transformed into equivalent syntax that avoids block-level declarations. Strict mode and non-strict mode have subtly incompatible behavior for how block-level function declarations are interpreted. Doing this transformation prevents problems with code that was originally strict mode that is run as non-strict mode and vice versa.

    Now esbuild uses the presence or absence of a strict mode scope to determine how to interpret the block-level function declaration and then converts it to the equivalent unambiguous syntax such that it works the same regardless of whether or not the current scope is in strict mode:

    ```js
    // This original code:
    while (!y) {
      function y() {}
    }

    // is transformed into this code in strict mode:
    while (!y) {
      let y2 = function() {};
    }

    // and into this code when not in strict mode:
    while (!y) {
      let y2 = function() {};
      var y = y2;
    }
    ```

## 0.8.40

* Fix TypeScript parameter decorators on class constructors ([#734](https://github.com/evanw/esbuild/issues/734))

    This release fixes a TypeScript translation bug where parameter decorators on class constructors were translated incorrectly. Affected code looks like this:

    ```js
    class Example {
      constructor(@decorator param: any) {}
    }
    ```

    This bug has been fixed. In addition, decorators are no longer allowed on class constructors themselves because they are not allowed in TypeScript.

* Resolve `browser` entries in `package.json` with no file extension ([#740](https://github.com/evanw/esbuild/issues/740))

    This fix changes how esbuild interprets the `browser` field in `package.json`. It will now remap imports without a file extension to `browser` map entries without a file extension, which improves compatibility with Webpack. Specifically, a `package.json` file with `"browser": {"./file": "./something.js"}` will now match an import of `./file`. Previously the `package.json` file had to contain something like `"browser": {"./file.js": "./something.js"}` instead. Note that for compatibility with the rest of the ecosystem, a remapping of `./file` will counter-intuitively _not_ match an import of `./file.js` even though it works fine in the other direction.

* Warning: npm v7 bug may prevent esbuild installation

    This is a warning for people reading these release notes, not a code change. I have discovered a bug in npm v7 where your `package-lock.json` file can become corrupted such that no `postinstall` scripts are run. This bug affects all packages with `postinstall` scripts, not just esbuild, and happens when running npm v7 on a `package-lock.json` file from npm v6 or earlier. It seems like deleting and regenerating your `package-lock.json` file is a valid workaround that should get esbuild working again.

## 0.8.39

* Fix the JavaScript watch mode API exiting early ([#730](https://github.com/evanw/esbuild/issues/730))

    The previous release contained a bug that caused the JavaScript watch mode API to exit early in some cases. This bug should now be fixed. The problem was caused by some code that shouldn't even need to exist now that you are no longer required to call `stop()` on an esbuild service created by `startService()` (it was made optional in version 0.8.32). I took the opportunity to clean up the internals of esbuild's JavaScript API implementation which ended up removing the entire section of code that contained this bug.

* Add an API option for a per-build working directory ([#689](https://github.com/evanw/esbuild/issues/689))

    You can now use the `absWorkingDir` API option to customize the current working directory. It will default to the value of `process.cwd()` at the time of the call to `startService()` when not specified, which matches the existing behavior. The working directory is used for a few different things including resolving relative paths given as API options to absolute paths and pretty-printing absolute paths as relative paths in log messages.

    In addition to being a useful feature, this change also simplifies esbuild's internals. Previously esbuild had to maintain separate child processes if the current working directory was changed in between build API calls. Now esbuild will always reuse the same child process across all build API calls. The `stop()` call on the `startService()` API is also now a no-op (it doesn't do anything anymore) and the `startService()` API may be removed in future releases.

* Fix stray `esbuild` process after `node` exits ([#643](https://github.com/evanw/esbuild/issues/643))

    I discovered that using esbuild's JavaScript incremental build API could result in the child `esbuild` process not exiting when the parent `node` process exits. This was due to a reference counting issue. The bug has been fixed so this shouldn't happen anymore.

## 0.8.38

* Implement a simple cross-platform watch mode ([#21](https://github.com/evanw/esbuild/issues/21))

    With this release, you can use the `--watch` flag to run esbuild in watch mode which watches the file system for changes and does an incremental build when something has changed. The watch mode implementation uses polling instead of OS-specific file system events for portability.

    Note that it is still possible to implement watch mode yourself using esbuild's incremental build API and a file watcher library of your choice if you don't want to use a polling-based approach. Also note that this watch mode feature is about improving developer convenience and does not have any effect on incremental build time (i.e. watch mode is not faster than other forms of incremental builds).

    The new polling system is intended to use relatively little CPU vs. a traditional polling system that scans the whole directory tree at once. The file system is still scanned regularly but each scan only checks a random subset of your files to reduce CPU usage. This means a change to a file will be picked up soon after the change is made but not necessarily instantly. With the current heuristics, large projects should be completely scanned around every 2 seconds so in the worst case it could take up to 2 seconds for a change to be noticed. However, after a change has been noticed the change's path goes on a short list of recently changed paths which are checked on every scan, so further changes to recently changed files should be noticed almost instantly.

* Add `pluginData` to pass data between plugins ([#696](https://github.com/evanw/esbuild/issues/696))

    You can now return additional data from a plugin in the optional `pluginData` field and it will be passed to the next plugin that runs in the plugin chain. So if you return it from an `onLoad` plugin, it will be passed to the `onResolve` plugins for any imports in that file, and if you return it from an `onResolve` plugin, an arbitrary one will be passed to the `onLoad` plugin when it loads the file (it's arbitrary since the relationship is many-to-one). This is useful to pass data between different plugins without them having to coordinate directly.

## 0.8.37

* Improve ambiguous import handling ([#723](https://github.com/evanw/esbuild/issues/723))

    It is an error to try to import a name from a file where there are multiple matching exports due to multiple `export * from` statements from files which export that name. This release contains a few improvements to ambiguous import handling:

    1. This release fixes a bug where named export shadowing didn't work correctly with multiple levels of re-exports. A named export closer in the re-export chain is supposed to hide a named export deeper in the re-export chain without causing an ambiguous import. The bug caused this case to be incorrectly flagged as an error even though it should have been allowed. This case is now allowed without an error.

    2. Previously the error message just said that there was an ambiguous import but didn't have any additional information. With this release, the error message also points out where the two different exports that have collided are in their original source files. Hopefully this should make it quicker to diagnose these types of issues.

    3. Real JavaScript environments only treat ambiguous imports as an error if they are explicitly a named import. Using the `import * as` syntax and then accessing the ambiguous import with a property access results in `undefined` instead of an error. Previously esbuild also treated this case as an error because it automatically rewrites star-import syntax to named-import syntax to improve tree shaking. With this release, this case is now treated as a warning instead of an error and the import will be automatically replaced with an `undefined` literal in the bundled code.

* Reuse automatically-generated temporary `*.node` files ([#719](https://github.com/evanw/esbuild/pull/719))

    The previous change to hide the automatically-generated N-API native node extensions from Yarn 2 writes these `*.node` files to the system's temporary directory. A new one was being created on each run which is wasteful even though they are only a few kilobytes in size. With this release `*.node` files will now be reused if they are already present in the system's temporary directory, so a new one is no longer created on each run. This fix was contributed by [@kzc](https://github.com/kzc).

* Fix the serve API with `outfile` ([#707](https://github.com/evanw/esbuild/issues/707))

    This release fixes a bug where the serve API did not work with the `outfile` setting. Using this setting with the serve API should now work fine.

* Warn about duplicate keys in object literals

    Using a duplicate key in an object literal such as `{x: 1, x: 2}` is now a warning. This is allowed in JavaScript but results in subsequent keys overwriting the previous key. It's usually a copy/paste error and isn't ever useful so it's worth warning about.

* Avoid generating duplicate keys in JSON metadata

    The `output` map that is generated when the `metafile` feature is active could potentially have duplicate keys if the `file` loader is used, there are multiple entry points, and two or more entry points reference the same file. This is harmless because both keys mapped to the same value, but it's confusing and unnecessary. Duplicate keys are no longer present in the output map in this latest release.

* Make the JSON metafile structure match the type definitions ([#726](https://github.com/evanw/esbuild/pull/726))

    Previously `imports` and/or `exports` could be missing from entries in the `output` map in certain cases (specifically for source maps and files loaded with the `file` loader). This was problematic because the TypeScript type definitions for the metafile say that the `imports` and `exports` properties are non-optional. With this release, the `imports` and `exports` properties are now always present so the existing TypeScript type definitions are now accurate.

* Update from Go 1.15.5 to Go 1.15.7

    The version of Go used to build the released binary executables on npm is now Go 1.15.7. This change shouldn't result in any visible changes to esbuild. It was only upgraded because the Go extension for the VSCode IDE now uses the official `gopls` Go language service and this extension wanted the latest version of Go.

## 0.8.36

* Fix an issue with writing large files to stdout using the WebAssembly executable

    The previous release introduced a regression where large output files written to stdout were incorrectly truncated when using the WebAssembly `esbuild` command. This regression was due to a missing callback to the JavaScript `write()` function when called on the stdout stream. The regression has been fixed.

* Hide the N-API native node extensions from Yarn 2

    The previous release introduced some very small (1-2kb) `*.node` native extensions to fix a bug with node failing to exit properly. However, this causes Yarn 2 to unzip the esbuild package, which is undesirable. This release puts these native node extensions inside JavaScript code instead to hide them from Yarn 2. The native extensions are written to a temporary file at run-time if necessary.

## 0.8.35

* Fix a commonly-missed corner case with `await` inside `**`

    I recently discovered an interesting discussion about JavaScript syntax entitled ["Most implementations seem to have missed that `await x ** 2` is not legal"](https://github.com/tc39/ecma262/issues/2197). Indeed esbuild has missed this, but this is not surprising because V8 has missed this as well and I usually test esbuild against V8 to test if esbuild is conformant with the JavaScript standard. Regardless, it sounds like the result of the discussion is that the specification should stay the same and implementations should be fixed. This release fixes this bug in esbuild's parser. The syntax `await x ** 2` is no longer allowed and parentheses are now preserved for the syntax `(await x) ** 2`.

* Allow namespaced names in JSX syntax ([#702](https://github.com/evanw/esbuild/issues/702))

    XML-style namespaced names with a `:` in the middle are a part of the [JSX specification](http://facebook.github.io/jsx/) but they are explicitly unimplemented by React and TypeScript so esbuild doesn't currently support them. However, there was a user request to support this feature since it's part of the JSX specification and esbuild's JSX support can be used for non-React purposes. So this release now supports namespaced names in JSX expressions:

    ```jsx
    let xml =
      <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
               xmlns:dc="http://purl.org/dc/elements/1.1/">
        <rdf:Description rdf:ID="local-record">
          <dc:title>Local Record</dc:title>
        </rdf:Description>
      </rdf:RDF>
    ```

    This JSX expression is now transformed by esbuild to the following JavaScript:

    ```js
    let xml = React.createElement("rdf:RDF", {
      "xmlns:rdf": "http://www.w3.org/1999/02/22-rdf-syntax-ns#",
      "xmlns:dc": "http://purl.org/dc/elements/1.1/"
    }, React.createElement("rdf:Description", {
      "rdf:ID": "local-record"
    }, React.createElement("dc:title", null, "Local Record")));
    ```

    Note that if you are trying to namespace your React components, this is _not_ the feature to use. You should be using a `.` instead of a `:` for namespacing your React components since `.` resolves to a JavaScript property access.

* Fix `worker: false` in esbuild's browser-based JavaScript API

    The browser-based JavaScript API creates a web worker by default but this can be disabled by passing `worker: false`. When you do this the WebAssembly code is run in the current thread which will lock up the thread. This is mainly useful if you're calling the JavaScript API from within a web worker and you want to avoid creating another nested web worker.

    This option was unintentionally broken when the internal JavaScript web worker source code was moved from an inline function to a string in version 0.5.20. The regression has been fixed and the `worker: false` scenario now has test coverage.

* Fix absolute paths with the `esbuild-wasm` package on Windows ([#687](https://github.com/evanw/esbuild/issues/687))

    The package `esbuild-wasm` has an `esbuild` command implemented using WebAssembly instead of using native code. It uses node's WebAssembly implementation and calls methods on node's `fs` module to access the file system.

    Go's `path/filepath` module has a bug where Windows paths are interpreted as Unix paths when targeting WebAssembly: [golang/go#43768](https://github.com/golang/go/issues/43768). This causes multiple issues including absolute paths such as `C:\path\to\file.js` being interpreted as relative paths (since they don't start with a `/`) and being joined onto the end of other paths.

    To fix this, esbuild now does all of its own path handling instead of using Go's path handling code. The esbuild code base now contains a forked copy of `path/filepath` that can handle both Windows and Unix paths. The decision about which one to use is made at run-time. When targeting WebAssembly, the presence of the `C:\` directory is used to determine if Windows-style paths should be used.

    With this release, it should now be possible to use Windows-style paths with esbuild's WebAssembly implementation on Windows.

* Fix using stdin with the `esbuild-wasm` package on Windows ([#687](https://github.com/evanw/esbuild/issues/687))

    Node has an old bug ([nodejs/node#19831](https://github.com/nodejs/node/issues/19831), [nodejs/node#35997](https://github.com/nodejs/node/issues/35997)) where `fs.read` returns an EOF error at the end of stdin on Windows. This causes Go's WebAssembly implementation to panic when esbuild tries to read from stdin.

    The workaround was to manually check for this case and then ignore the error in this specific case. With this release, it should now be possible to pipe something to the `esbuild` command on Windows.

* Fix stdout and stderr not supporting Unicode in the `esbuild-wasm` package on Windows ([#687](https://github.com/evanw/esbuild/issues/687))

    Node's `fs.write` API is broken when writing Unicode to stdout and stderr on Windows, and this will never be fixed: [nodejs/node#24550](https://github.com/nodejs/node/issues/24550). This is problematic for Go's WebAssembly implementation because it uses this API for writing to all file descriptors.

    The workaround is to manually intercept the file descriptors for stdout and stderr and redirect them to `process.stdout` and `process.stderr` respectively. Passing Unicode text to `write()` on these objects instead of on the `fs` API strangely works fine. So with this release, Unicode text should now display correctly when using esbuild's WebAssembly implementation on Windows (or at least, as correctly as the poor Unicode support in Windows Command Prompt allows).

* Add a hack for faster command-line execution for the WebAssembly module in certain cases

    Node has an unfortunate bug where the node process is unnecessarily kept open while a WebAssembly module is being optimized: https://github.com/nodejs/node/issues/36616. This means cases where running `esbuild` should take a few milliseconds can end up taking many seconds instead.

    The workaround is to force node to exit by ending the process early. This is done in one of two ways depending on the exit code. For non-zero exit codes (i.e. when there is a build error), the `esbuild` command now calls `process.kill(process.pid)` to avoid the hang.

    For zero exit codes, the `esbuild` command now loads a N-API native node extension that calls the operating system's `exit(0)` function. This is done without requiring `node-gyp` by precompiling each supported platform and just including all of them in the `esbuild-wasm` package since they are so small. If this hack doesn't work in certain cases, the process should exit anyway just potentially many seconds later. Currently the only supported platforms for this hack are 64-bit macOS, Windows, and Linux.

* Fix non-absolute paths with the `esbuild-wasm` package in the browser ([#693](https://github.com/evanw/esbuild/issues/693))

    When using esbuild in the browser via WebAssembly, it was not possible to specify an non-absolute output path. Normally you can do this and esbuild will just convert it to an absolute path by resolving it as a relative path from the current working directory. However, Go's WebAssembly implementation has no current working directory so the conversion operation to an absolute path failed, causing esbuild's API to fail.

    With this release, esbuild should now behave as if the current working directory is `/` in the browser. For example, this means calling the `build()` API with `outfile: 'file.js'` should now generate an output file called `/file.js` instead of causing an error.

## 0.8.34

* Fix a parser bug about suffix expressions after an arrow function body ([#701](https://github.com/evanw/esbuild/issues/701))

    The JavaScript parser incorrectly handled suffix expressions after a non-expression arrow function body. In practice, this came up when a semicolon was omitted from the end of an expression statement and the following expression could be considered a suffix expression:

    ```js
    x = () => {}
    (y)
    ```

    This was incorrectly parsed as `(x = () => {})(y);` instead of `x = () => {}; y;`. With this release, this edge case should now be parsed correctly.

* Add new `neutral` platform to help text ([#695](https://github.com/evanw/esbuild/pull/695))

    The new `--platform=neutral` API option that was added in the previous release was incorrectly not listed in the CLI help text for the platform feature. This omission has been fixed. The fix was contributed by [@hardfist](https://github.com/hardfist).

## 0.8.33

* Fix esbuild potentially exiting early during incremental rebuilds

    The change in the previous release to make calling `stop()` optional caused a regression for incremental rebuilds where calling `rebuild()` could potentially cause the process to exit early before the incremental rebuild is completed. This is because the implementation of `rebuild()` was missing a reference count to track that the service is now temporarily needed again. This omission was an oversight, and has now been fixed.

* Fix using the new `sourcesContent` option with the transform API ([#682](https://github.com/evanw/esbuild/issues/682))

    Due to an oversight, the `sourcesContent: false` option that was added in version 0.8.27 didn't work with the JavaScript transform API. This was unintentional and has been fixed. This fix was contributed by [@jschaf](https://github.com/jschaf).

* Insert the object spread shim in constructor methods after the `super()` call ([#678](https://github.com/evanw/esbuild/issues/678))

    This fixes an issue with the transform for object spread to older compile targets. Previously the following code would be transformed to code that crashes when run if the compile target is `es2017` or lower:

    ```js
    class Derived extends Base {
      prop = null;
      constructor({ ...args }) {
        super(args);
      }
    }
    ```

    This code was incorrectly compiled to something like this, which will throw `ReferenceError: Must call super constructor in derived class before accessing 'this' or returning from derived constructor`:

    ```js
    class Derived extends Base {
      constructor(_a) {
        __publicField(this, "prop", null);
        var args = __rest(_a, []);
        super(args);
      }
    }
    ```

    With this release, it will now be compiled to something like this instead:

    ```js
    class Derived extends Base {
      constructor(_a) {
        var args = __rest(_a, []);
        super(args);
        __publicField(this, "prop", null);
      }
    }
    ```

* Add the `--platform=neutral` API option ([#674](https://github.com/evanw/esbuild/issues/674))

    There are currently two platform values: `browser` (the default) and `node`. These settings are a convenient way to configure multiple defaults for other API options for maximum compatibility. However, some users want to configure everything themselves so esbuild does not assume any platform-specific behavior. In this case you can now use `--platform=neutral` to disable platform-specific default values. Note that this means if you want to use npm-style packages you will have to configure a main field yourself with something like `--main-fields=main`.

* Provide minified and non-minified versions of in-browser API library ([#616](https://github.com/evanw/esbuild/issues/616))

    The in-browser JavaScript API libraries for esbuild are in the [esbuild-wasm](https://www.npmjs.com/package/esbuild-wasm) package. There are two: `esbuild-wasm/lib/browser.js` in UMD format and `esbuild-wasm/esm/browser.js` in ESM format. Previously these were minified since they contain a large string of JavaScript that cannot be minified by other tools. Now they are no longer minified, and there are new minified versions available at `esbuild-wasm/lib/browser.min.js` and `esbuild-wasm/esm/browser.min.js`.

## 0.8.32

* Calling `stop()` on the JavaScript API is now optional ([#656](https://github.com/evanw/esbuild/pull/656))

    The JavaScript implementation of esbuild's API now calls `unref()` internally so node will now exit even if the internal long-lived esbuild process is still running. You should no longer need to explicitly call `stop()` on the service returned by `startService()`, which simplifies service lifetime management. This feature was contributed by [@SalvatorePreviti](https://github.com/SalvatorePreviti).

* Fix bug in metafile path generation ([#662](https://github.com/evanw/esbuild/issues/662))

    Certain import path metadata in the JSON file generated by the `--metafile` setting could be incorrect in scenarios with code splitting active and multiple entry points in different subdirectories. The incorrect paths referred to cross-chunk imports of other generated code splitting chunks and were incorrectly relative to the subdirectory inside the output directory instead of relative to the output directory itself. This issue has been fixed.

* Add `kind` to import paths in metafile JSON ([#655](https://github.com/evanw/esbuild/issues/655))

    The `--metafile` flag generates build metadata in JSON format describing the input and output files in the build. Previously import path objects only had a `path` property. With this release, they now also have a `kind` property that describes the way the file was imported. The value is a string that is equal to one of the following values:

    For JavaScript files:

    * `import-statement`
    * `require-call`
    * `dynamic-import`
    * `require-resolve`

    For CSS files:

    * `import-rule`
    * `url-token`

* Add support for TypeScript 4.2 syntax

    Most of the new features included in the [TypeScript 4.2 beta announcement](https://devblogs.microsoft.com/typescript/announcing-typescript-4-2-beta/) are type system features that don't apply to esbuild. But there's one upcoming feature that adds new syntax: `abstract` construct signatures. They look like this:

    ```ts
    let Ctor: abstract new () => HasArea = Shape;
    ```

    This new syntax can now be parsed by esbuild.

* Add `detail` to errors and warnings ([#654](https://github.com/evanw/esbuild/issues/654))

    Errors and warnings returned by the JavaScript and Go APIs now have a `detail` property which contains the original error. This is relevant if a custom JavaScript exception is thrown or a custom Go `error` is returned from inside a plugin callback.

* Disable code warnings inside `node_modules` directories even with plugins ([#666](https://github.com/evanw/esbuild/issues/666))

    Some of the warnings that esbuild generates exist to point out suspicious looking code that is likely a bug. An example is `typeof x == 'null'` since the `typeof` operator never generates the string `null`. Arguably these warnings belong in a linter instead of in esbuild since esbuild is a bundler, but I figured that some warnings about obviously broken code would still be helpful because many people don't run linters. It's part of my quest to improve software quality. And these warnings have caught real bugs in published code so they aren't meaningless. The warning must be considered very unlikely to be a false positive to be included.

    A change was added in version 0.7.4 to exclude files inside `node_modules` directories from these warnings. Even if the warnings flag a real bug, the warning is frustrating as a user because it's mostly non-actionable. The only resolution other than turning off warnings is to file an issue with the package, since code in published packages is immutable.

    However, since then the plugin API has been released and this behavior didn't apply if the import path was resolved by a plugin. It only applied if the import path was resolved by esbuild itself. That problem is fixed in this release. Now these warnings will be omitted from any file with `node_modules` in its path, even if the path originated from a plugin.

* Remove the warning about self-assignment ([#666](https://github.com/evanw/esbuild/issues/666))

    This warning was added in version 0.8.11 and warns about self-assignment such as `x = x`. The rationale is that this is likely a copy/paste error. However, it triggers too often for cross-compiled TypeScript code so the false positive rate is too high. The warning has now been removed.

* Disable constant folding for the `?:` operator when not minifying ([#657](https://github.com/evanw/esbuild/issues/657))

    When minification is not enabled, the `?:` operator will now no longer be simplified if the condition evaluates to `true` or `false`. This could result in slower builds in certain cases because esbuild may now scan more files unnecessarily during bundling. This change was made because of a user request.

## 0.8.31

* Fix minification issue from previous release ([#648](https://github.com/evanw/esbuild/issues/648))

    The minification optimization to omit certain `continue` and `return` statements when it's implied by control flow in version 0.8.29 caused a regression when the branch condition uses a hoisted function:

    ```js
    if (fn()) return;
    ...
    function fn() {}
    ```

    In that case, transforming the code by inverting the condition and moving the following statements inside the branch is not valid because the function is no longer hoisted to above the branch condition. This release fixes the regression by avoiding this optimization in cases like this.

* Add the option `--sourcemap=both` ([#650](https://github.com/evanw/esbuild/issues/650))

    This new option puts the generated source map both an inline `//# sourceMappingURL=` data URL comment inside the output file and in an external file next to the output file. Using it is also possible with the transform API, which will cause it to return both an inline data URL comment in the `code` value and the source map JSON in the `map` value.

* Tree-shake unused code with `--format=iife` ([#639](https://github.com/evanw/esbuild/issues/639))

    When the output format is IIFE (which wraps the code in an immediately-invoked function expression), esbuild now assumes that it's safe to remove unused code. This is an assumption that esbuild always makes when bundling but that esbuild previously didn't make when not bundling. Now esbuild will remove code even when not bundling as long as the output format is IIFE.

    This is only done for the IIFE output format because people are currently using the other formats to compile "partial modules", meaning they expect to be able to append code to esbuild's output and have that appended code be able to reference unused code inside esbuild's output. So it's not safe for esbuild to remove unused code in those cases. The IIFE output format wraps everything in a closure so unused code is not exposed to the module-level scope. Appended code will not be able to access unused code inside the closure so that means it's safe to remove.

## 0.8.30

* Fix `@jsx` and `@jsxFrag` comments without trailing spaces

    The `--jsx-factory` and `--jsx-fragment` settings can be set on a per-file basis using `// @jsx name` or `// @jsxFrag name` comments. Comments of the form `/* @jsx name */` or `/* @jsxFrag name */` will also work. However, there was a bug where comments of the form `/* @jsx name*/` or `/* @jsxFrag name*/` (a multi-line comment without a trailing space at the end) did not work. This bug has been fixed, and you now no longer need a trailing space for multi-line comments.

* Minification improvements

    * The expression before a switch statement is now folded into the value. This means `fn(); switch (x) { ... }` turns into `switch (fn(), x) { ... }`.

    * Uses of `===` and `!==` are converted to `==` or `!=` if the types of both sides can easily be statically determined. This means `(x & 1) === 0` turns into `(x & 1) == 0`.

    * Equality comparisons are removed if both sides are boolean and one side is a constant. This means `!x === true` turns into `!x`.

    * Certain unary and binary operators are now removed if unused. This means `if (a() === b()) {}` turns into `a(), b();`.

    * The comma operator is now extracted from certain expressions. This means `(a, b) + c` turns into `a, b + c`.

    * Minification now takes advantage of the left-associativity of certain operators. This means `a && (b && c)` turns into `a && b && c`.

    * Computed properties that are strings now become no longer computed. This means `{['a']: b}` turns into `{a: b}` and `class { ['a'] = b }` turns into `class { a = b }`.

    * Repeated if-jump statements are now merged. This means `if (a) break; if (b) break;` turns into `if (a || b) break;`.

* Fix issues with nested source maps ([#638](https://github.com/evanw/esbuild/issues/638))

    A nested source map happens when an input file has a valid `//# sourceMappingURL=` comment that points to a valid source map file. In that case, esbuild will read that source map and use it to map back to the original source code from the generated file. This only happens if you enable source map generation in esbuild via `--sourcemap`. This release fixes the following issues:

    * Generated source maps were incorrect when an input file had a nested source map and the input source map had more than one source file. This regression was introduced by an optimization in version 0.8.25 that parallelizes the generation of certain internal source map data structures. The index into the generated `sources` array was incorrectly incremented by 1 for every input file instead of by the number of sources in the input source map. This issue has been fixed and now has test coverage.

    * Generated source maps were incorrect when an input file had a nested source map, the file starts with a local variable, the previous file ends with a local variable of that same type, and the input source map is missing a mapping at the start of the file. An optimization was added in version 0.7.18 that splices together local variable declarations from separate files when they end up adjacent to each other in the generated output file (i.e. `var a=0;var b=2;` becomes `var a=0,b=2;` when `a` and `b` are in separate files). The source map splicing was expecting a mapping at the start of the file and that isn't necessarily the case when using nested source maps. The optimization has been disabled for now to fix source map generation, and this specific case has test coverage.

## 0.8.29

* Allow entry points outside of the `outbase` directory ([#634](https://github.com/evanw/esbuild/issues/634))

    When esbuild generates the output path for a bundled entry point, it computes the relative path from [the `outbase` directory](https://esbuild.github.io/api/#outbase) to the input entry point file and then joins that relative path to the output directory. For example, if there are two entry points `src/pages/home/index.ts` and `src/pages/about/index.ts`, the outbase directory is `src`, and the output directory is `out`, the output directory will contain `out/pages/home/index.js` and `out/pages/about/index.js`.

    However, this means that the `outbase` directory is expected to contain all entry point files (even implicit entry point files from `import()` expressions). If an entry point isn't under the outbase directory then esbuild will to try to write the output file outside of the output directory, since the path of the entry point relative to `outbase` will start with `../` which is then joined to the output directory. This is unintentional. All output files are supposed to be written inside of the output directory.

    This release fixes the problem by creating a directory with the name `_.._` in the output directory for output file paths of entry points that are not inside the `outbase` directory. So if the previous example was bundled with an outbase directory of `temp`, the output directory will contain `out/_.._/pages/home/index.js` and `out/_.._/pages/about/index.js`. Doing this instead of stripping the leading `../` off the relative path is necessary to avoid collisions between different entry points with the same path suffix.

* Minification improvements

    This release contains the following minification improvements:

    * Expressions of the form `!(a == b)` are now converted to `a != b`. This also applies similarly for the other three equality operators.

    * A trailing `continue;` statement inside the body of a loop is now removed.

    * Minification can now omit certain `continue` and `return` statements when it's implied by control flow:

        ```js
        // Before minification
        function fn() {
          if (a) return;
          while (b) {
            if (c) continue;
            d();
          }
        }
        ```

        ```js
        // After minification
        function fn() {
          if (!a)
            for (; b; )
              c || d();
        }
        ```

    * Certain single-use variables are now inlined if the use directly follows the variable:

        ```js
        // Before minification
        let result = fn();
        let callback = result.callback;
        return callback.call(this);
        ```

        ```js
        // After minification
        return fn().callback.call(this);
        ```

        This transformation is only done when it's safe to do so. The safety conditions are complex but at a high level, an expression cannot be reordered past another expression if either of them could possibly have side effects.

## 0.8.28

* Add a `--summary` flag that prints helpful information after a build ([#631](https://github.com/evanw/esbuild/issues/631))

    Normally esbuild's CLI doesn't print anything after doing a build if nothing went wrong. This allows esbuild to be used as part of a more complex chain of tools without the output cluttering the terminal. However, sometimes it is nice to have a quick overview in your terminal of what the build just did. You can now add the `--summary` flag when using the CLI and esbuild will print a summary of what the build generated. It looks something like this:

    ```
    $ ./esbuild --summary --bundle src/Three.js --outfile=build/three.js --sourcemap

      build/three.js      1.0mb ⚠️
      build/three.js.map  1.8mb

    ⚡ Done in 43ms
    ```

* Keep unused imports in TypeScript code in one specific case ([#604](https://github.com/evanw/esbuild/issues/604))

    The official TypeScript compiler always removes imported symbols that aren't used as values when converting TypeScript to JavaScript. This is because these symbols could be types and not removing them could result in a run-time module instantiation failure because of missing exports. This even happens when the `tsconfig.json` setting `"importsNotUsedAsValues"` is set to `"preserve"`. Doing this just keeps the import statement itself but confusingly still removes the imports that aren't used as values.

    Previously esbuild always exactly matched the behavior of the official TypeScript compiler regarding import removal. However, that is problematic when trying to use esbuild to compile a partial module such as when converting TypeScript to JavaScript inside a file written in the [Svelte](https://svelte.dev/) programming language. Here is an example:

    ```html
    <script lang="ts">
      import Counter from './Counter.svelte';
      export let name: string = 'world';
    </script>
    <main>
      <h1>Hello {name}!</h1>
      <Counter />
    </main>
    ```

    The current Svelte compiler plugin for TypeScript only provides esbuild with the contents of the `<script>` tag so to esbuild, the import `Counter` appears to be unused and is removed.

    In this release, esbuild deliberately deviates from the behavior of the official TypeScript compiler if all of these conditions are met:

    * The `"importsNotUsedAsValues"` field in `tsconfig.json` must be present and must not be set to `"remove"`. This is necessary because this is the only case where esbuild can assume that all imports are values instead of types. Any imports that are types will cause a type error when the code is run through the TypeScript type checker. To import types when the `importsNotUsedAsValues` setting is active, you must use the TypeScript-specific `import type` syntax instead.

    * You must not be using esbuild as a bundler. When bundling, esbuild needs to assume that it's not seeing a partial file because the bundling process requires renaming symbols to avoid cross-file name collisions.

    * You must not have identifier minification enabled. It's useless to preserve unused imports in this case because referencing them by name won't work anyway. And keeping the unused imports would be counter-productive to minification since they would be extra unnecessary data in the output file.

    This should hopefully allow esbuild to be used as a TypeScript-to-JavaScript converter for programming languages such as Svelte, at least in many cases. The build pipeline in esbuild wasn't designed for compiling partial modules and this still won't be a fully robust solution (e.g. some variables may be renamed to avoid name collisions in rare cases). But it's possible that these cases are very unlikely to come up in practice. Basically this change to keep unused imports in this case should be useful at best and harmless at worst.

## 0.8.27

* Mark `import.meta` as supported in node 10.4+ ([#626](https://github.com/evanw/esbuild/issues/626))

    It was previously marked as unsupported due to a typo in esbuild's compatibility table, which meant esbuild generated a shim for `import.meta` even when it's not necessary. It should now be marked as supported in node 10.4 and above so the shim will no longer be included when using a sufficiently new target environment such as `--target=node10.4`.

* Fix for when the working directory ends with `/` ([#627](https://github.com/evanw/esbuild/issues/627))

    If the working directory ended in `/`, the last path component would be incorrectly duplicated. This was the case when running esbuild with Yarn 2 (but not Yarn 1) and is problematic because some externally-facing directories reference the current working directory in plugins and in output files. The problem has now been fixed and the last path component is no longer duplicated in this case. This fix was contributed by [@remorses](https://github.com/remorses).

* Add an option to omit `sourcesContent` from generated source maps ([#624](https://github.com/evanw/esbuild/issues/624))

    You can now pass `--sources-content=false` to omit the `sourcesContent` field from generated source maps. The field embeds the original source code inline in the source map and is the largest part of the source map. This is useful if you don't need the original source code and would like a smaller source map (e.g. you only care about stack traces and don't need the source code for debugging).

* Fix exports from ESM files converted to CJS during code splitting ([#617](https://github.com/evanw/esbuild/issues/617))

    This release fixes an edge case where files in ECMAScript module format that are converted to CommonJS format during bundling can generate exports to non-top-level symbols when code splitting is active. These files must be converted to CommonJS format if they are referenced by a `require()` call. When that happens, the symbols in that file are placed inside the CommonJS wrapper closure and are no longer top-level symbols. This means they should no longer be considered exportable for cross-chunk export generation due to code splitting. The result of this fix is that these cases no longer generate output files with module instantiation errors.

* Allow `--define` with array and object literals ([#581](https://github.com/evanw/esbuild/issues/581))

    The `--define` feature allows you to replace identifiers such as `DEBUG` with literal expressions such as `false`. This is valuable because the substitution can then participate in constant folding and dead code elimination. For example, `if (DEBUG) { ... }` could become `if (false) { ... }` which would then be completely removed in minified builds. However, doing this with compound literal expressions such as array and object literals is an anti-pattern because it could easily result in many copies of the same object in the output file.

    This release adds support for array and object literals with `--define` anyway, but they work differently than other `--define` expressions. In this case a separate virtual file is created and configured to be injected into all files similar to how the `--inject` feature works. This means there is only at most one copy of the value in a given output file. However, these values do not participate in constant folding and dead code elimination, since the object can now potentially be mutated at run-time.

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

    This release fixes a regression introduced in version 0.8.19 in which an `import` of an `export {...} from` re-export of a CommonJS module does not include the CommonJS module if it has been marked as `"sideEffects": false` in its `package.json` file. This was the case with the [Ramda](https://ramdajs.com/) library, and was due to an unhandled case in the linker.

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
    * [mrdoob/three.js#11182](https://github.com/mrdoob/three.js/pull/11182): Operator precedence bug in WWOBJLoader2.js

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
