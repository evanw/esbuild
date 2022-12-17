# Changelog: 2021

This changelog documents all esbuild versions published in the year 2021 (versions 0.8.29 through 0.14.10).

## 0.14.10

* Enable tree shaking of classes with lowered static fields ([#175](https://github.com/evanw/esbuild/issues/175))

    If the configured target environment doesn't support static class fields, they are converted into a call to esbuild's `__publicField` function instead. However, esbuild's tree-shaking pass treated this call as a side effect, which meant that all classes with static fields were ineligible for tree shaking. This release fixes the problem by explicitly ignoring calls to the `__publicField` function during tree shaking side-effect determination. Tree shaking is now enabled for these classes:

    ```js
    // Original code
    class Foo { static foo = 'foo' }
    class Bar { static bar = 'bar' }
    new Bar()

    // Old output (with --tree-shaking=true --target=es6)
    class Foo {
    }
    __publicField(Foo, "foo", "foo");
    class Bar {
    }
    __publicField(Bar, "bar", "bar");
    new Bar();

    // New output (with --tree-shaking=true --target=es6)
    class Bar {
    }
    __publicField(Bar, "bar", "bar");
    new Bar();
    ```

* Treat `--define:foo=undefined` as an undefined literal instead of an identifier ([#1407](https://github.com/evanw/esbuild/issues/1407))

    References to the global variable `undefined` are automatically replaced with the literal value for undefined, which appears as `void 0` when printed. This allows for additional optimizations such as collapsing `undefined ?? bar` into just `bar`. However, this substitution was not done for values specified via `--define:`. As a result, esbuild could potentially miss out on certain optimizations in these cases. With this release, it's now possible to use `--define:` to substitute something with an undefined literal:

    ```js
    // Original code
    let win = typeof window !== 'undefined' ? window : {}

    // Old output (with --define:window=undefined --minify)
    let win=typeof undefined!="undefined"?undefined:{};

    // New output (with --define:window=undefined --minify)
    let win={};
    ```

* Add the `--drop:debugger` flag ([#1809](https://github.com/evanw/esbuild/issues/1809))

    Passing this flag causes all [`debugger;` statements](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Statements/debugger) to be removed from the output. This is similar to the `drop_debugger: true` flag available in the popular UglifyJS and Terser JavaScript minifiers.

* Add the `--drop:console` flag ([#28](https://github.com/evanw/esbuild/issues/28))

    Passing this flag causes all [`console.xyz()` API calls](https://developer.mozilla.org/en-US/docs/Web/API/console#methods) to be removed from the output. This is similar to the `drop_console: true` flag available in the popular UglifyJS and Terser JavaScript minifiers.

    WARNING: Using this flag can introduce bugs into your code! This flag removes the entire call expression including all call arguments. If any of those arguments had important side effects, using this flag will change the behavior of your code. Be very careful when using this flag. If you want to remove console API calls without removing arguments with side effects (which does not introduce bugs), you should mark the relevant API calls as pure instead like this: `--pure:console.log --minify`.

* Inline calls to certain no-op functions when minifying ([#290](https://github.com/evanw/esbuild/issues/290), [#907](https://github.com/evanw/esbuild/issues/907))

    This release makes esbuild inline two types of no-op functions: empty functions and identity functions. These most commonly arise when most of the function body is eliminated as dead code. In the examples below, this happens because we use `--define:window.DEBUG=false` to cause dead code elimination inside the function body of the resulting `if (false)` statement. This inlining is a small code size and performance win but, more importantly, it allows for people to use these features to add useful abstractions that improve the development experience without needing to worry about the run-time performance impact.

    An identity function is a function that just returns its argument. Here's an example of inlining an identity function:

    ```js
    // Original code
    function logCalls(fn) {
      if (window.DEBUG) return function(...args) {
        console.log('calling', fn.name, 'with', args)
        return fn.apply(this, args)
      }
      return fn
    }
    export const foo = logCalls(function foo() {})

    // Old output (with --minify --define:window.DEBUG=false --tree-shaking=true)
    function o(n){return n}export const foo=o(function(){});

    // New output (with --minify --define:window.DEBUG=false --tree-shaking=true)
    export const foo=function(){};
    ```

    An empty function is a function with an empty body. Here's an example of inlining an empty function:

    ```ts
    // Original code
    function assertNotNull(val: Object | null): asserts val is Object {
      if (window.DEBUG && val === null) throw new Error('null assertion failed');
    }
    export const val = getFoo();
    assertNotNull(val);
    console.log(val.bar);

    // Old output (with --minify --define:window.DEBUG=false --tree-shaking=true)
    function l(o){}export const val=getFoo();l(val);console.log(val.bar);

    // New output (with --minify --define:window.DEBUG=false --tree-shaking=true)
    export const val=getFoo();console.log(val.bar);
    ```

    To get this behavior you'll need to use the `function` keyword to define your function since that causes the definition to be hoisted, which eliminates concerns around initialization order. These features also work across modules, so functions are still inlined even if the definition of the function is in a separate module from the call to the function. To get cross-module function inlining to work, you'll need to have bundling enabled and use the `import` and `export` keywords to access the function so that esbuild can see which functions are called. And all of this has been added without an observable impact to compile times.

    I previously wasn't able to add this to esbuild easily because of esbuild's low-pass compilation approach. The compiler only does three full passes over the data for speed. The passes are roughly for parsing, binding, and printing. It's only possible to inline something after binding but it needs to be inlined before printing. Also the way module linking was done made it difficult to roll back uses of symbols that were inlined, so the symbol definitions were not tree shaken even when they became unused due to inlining.

    The linking issue was somewhat resolved when I fixed #128 in the previous release. To implement cross-module inlining of TypeScript enums, I came up with a hack to defer certain symbol uses until the linking phase, which happens after binding but before printing. Another hack is that inlining of TypeScript enums is done directly in the printer to avoid needing another pass.

    The possibility of these two hacks has unblocked these simple function inlining use cases that are now handled. This isn't a fully general approach because optimal inlining is recursive. Inlining something may open up further inlining opportunities, which either requires multiple iterations or a worklist algorithm, both of which don't work when doing late-stage inlining in the printer. But the function inlining that esbuild now implements is still useful even though it's one level deep, and so I believe it's still worth adding.

## 0.14.9

* Implement cross-module tree shaking of TypeScript enum values ([#128](https://github.com/evanw/esbuild/issues/128))

    If your bundle uses TypeScript enums across multiple files, esbuild is able to inline the enum values as long as you export and import the enum using the ES module `export` and `import` keywords. However, this previously still left the definition of the enum in the bundle even when it wasn't used anymore. This was because esbuild's tree shaking (i.e. dead code elimination) is based on information recorded during parsing, and at that point we don't know which imported symbols are inlined enum values and which aren't.

    With this release, esbuild will now remove enum definitions that become unused due to cross-module enum value inlining. Property accesses off of imported symbols are now tracked separately during parsing and then resolved during linking once all inlined enum values are known. This behavior change means esbuild's support for cross-module inlining of TypeScript enums is now finally complete. Here's an example:

    ```js
    // entry.ts
    import { Foo } from './enum'
    console.log(Foo.Bar)

    // enum.ts
    export enum Foo { Bar }
    ```

    Bundling the example code above now results in the enum definition being completely removed from the bundle:

    ```js
    // Old output (with --bundle --minify --format=esm)
    var r=(o=>(o[o.Bar=0]="Bar",o))(r||{});console.log(0);

    // New output (with --bundle --minify --format=esm)
    console.log(0);
    ```

* Fix a regression with `export {} from` and CommonJS ([#1890](https://github.com/evanw/esbuild/issues/1890))

    This release fixes a regression that was introduced by the change in 0.14.7 that avoids calling the `__toESM` wrapper for import statements that are converted to `require` calls and that don't use the `default` or `__esModule` export names. The previous change was correct for the `import {} from` syntax but not for the `export {} from` syntax, which meant that in certain cases with re-exported values, the value of the `default` import could be different than expected. This release fixes the regression.

* Warn about using `module` or `exports` in ESM code ([#1887](https://github.com/evanw/esbuild/issues/1887))

    CommonJS export variables cannot be referenced in ESM code. If you do this, they are treated as global variables instead. This release includes a warning for people that try to use both CommonJS and ES module export styles in the same file. Here's an example:

    ```ts
    export enum Something {
      a,
      b,
    }
    module.exports = { a: 1, b: 2 }
    ```

    Running esbuild on that code now generates a warning that looks like this:

    ```
    ▲ [WARNING] The CommonJS "module" variable is treated as a global variable in an ECMAScript module and may not work as expected

        example.ts:5:0:
          5 │ module.exports = { a: 1, b: 2 }
            ╵ ~~~~~~

      This file is considered to be an ECMAScript module because of the "export" keyword here:

        example.ts:1:0:
          1 │ export enum Something {
            ╵ ~~~~~~
    ```

## 0.14.8

* Add a `resolve` API for plugins ([#641](https://github.com/evanw/esbuild/issues/641), [#1652](https://github.com/evanw/esbuild/issues/1652))

    Plugins now have access to a new API called `resolve` that runs esbuild's path resolution logic and returns the result to the caller. This lets you write plugins that can reuse esbuild's complex built-in path resolution logic to change the inputs and/or adjust the outputs. Here's an example:

    ```js
    let examplePlugin = {
      name: 'example',
      setup(build) {
        build.onResolve({ filter: /^example$/ }, async () => {
          const result = await build.resolve('./foo', { resolveDir: '/bar' })
          if (result.errors.length > 0) return result
          return { ...result, external: true }
        })
      },
    }
    ```

    This plugin intercepts imports to the path `example`, tells esbuild to resolve the import `./foo` in the directory `/bar`, and then forces whatever path esbuild returns to be considered external. Here are some additional details:

    * If you don't pass the optional `resolveDir` parameter, esbuild will still run `onResolve` plugin callbacks but will not attempt any path resolution itself. All of esbuild's path resolution logic depends on the `resolveDir` parameter including looking for packages in `node_modules` directories (since it needs to know where those `node_modules` directories might be).

    * If you want to resolve a file name in a specific directory, make sure the input path starts with `./`. Otherwise the input path will be treated as a package path instead of a relative path. This behavior is identical to esbuild's normal path resolution logic.

    * If path resolution fails, the `errors` property on the returned object will be a non-empty array containing the error information. This function does not always throw an error when it fails. You need to check for errors after calling it.

    * The behavior of this function depends on the build configuration. That's why it's a property of the `build` object instead of being a top-level API call. This also means you can't call it until all plugin `setup` functions have finished since these give plugins the opportunity to adjust the build configuration before it's frozen at the start of the build. So the new `resolve` function is going to be most useful inside your `onResolve` and/or `onLoad` callbacks.

    * There is currently no attempt made to detect infinite path resolution loops. Calling `resolve` from within `onResolve` with the same parameters is almost certainly a bad idea.

* Avoid the CJS-to-ESM wrapper in some cases ([#1831](https://github.com/evanw/esbuild/issues/1831))

    Import statements are converted into `require()` calls when the output format is set to CommonJS. To convert from CommonJS semantics to ES module semantics, esbuild wraps the return value in a call to esbuild's `__toESM()` helper function. However, the conversion is only needed if it's possible that the exports named `default` or `__esModule` could be accessed.

    This release avoids calling this helper function in cases where esbuild knows it's impossible for the `default` or `__esModule` exports to be accessed, which results in smaller and faster code. To get this behavior, you have to use the `import {} from` import syntax:

    ```js
    // Original code
    import { readFile } from "fs";
    readFile();

    // Old output (with --format=cjs)
    var __toESM = (module, isNodeMode) => {
      ...
    };
    var import_fs = __toESM(require("fs"));
    (0, import_fs.readFile)();

    // New output (with --format=cjs)
    var import_fs = require("fs");
    (0, import_fs.readFile)();
    ```

* Strip overwritten function declarations when minifying ([#610](https://github.com/evanw/esbuild/issues/610))

    JavaScript allows functions to be re-declared, with each declaration overwriting the previous declaration. This type of code can sometimes be emitted by automatic code generators. With this release, esbuild now takes this behavior into account when minifying to drop all but the last declaration for a given function:

    ```js
    // Original code
    function foo() { console.log(1) }
    function foo() { console.log(2) }

    // Old output (with --minify)
    function foo(){console.log(1)}function foo(){console.log(2)}

    // New output (with --minify)
    function foo(){console.log(2)}
    ```

* Add support for the Linux IBM Z 64-bit Big Endian platform ([#1864](https://github.com/evanw/esbuild/pull/1864))

    With this release, the esbuild package now includes a Linux binary executable for the IBM System/390 64-bit architecture. This new platform was contributed by [@shahidhs-ibm](https://github.com/shahidhs-ibm).

* Allow whitespace around `:` in JSX elements ([#1877](https://github.com/evanw/esbuild/issues/1877))

    This release allows you to write the JSX `<rdf:Description rdf:ID="foo" />` as `<rdf : Description rdf : ID="foo" />` instead. Doing this is not forbidden by [the JSX specification](https://facebook.github.io/jsx/). While this doesn't work in TypeScript, it does work with other JSX parsers in the ecosystem, so support for this has been added to esbuild.

## 0.14.7

* Cross-module inlining of TypeScript `enum` constants ([#128](https://github.com/evanw/esbuild/issues/128))

    This release adds inlining of TypeScript `enum` constants across separate modules. It activates when bundling is enabled and when the enum is exported via the `export` keyword and imported via the `import` keyword:

    ```ts
    // foo.ts
    export enum Foo { Bar }

    // bar.ts
    import { Foo } from './foo.ts'
    console.log(Foo.Bar)
    ```

    The access to `Foo.Bar` will now be compiled into `0 /* Bar */` even though the enum is defined in a separate file. This inlining was added without adding another pass (which would have introduced a speed penalty) by splitting the code for the inlining between the existing parsing and printing passes. Enum inlining is active whether or not you use `enum` or `const enum` because it improves performance.

    To demonstrate the performance improvement, I compared the performance of the TypeScript compiler built by bundling the TypeScript compiler source code with esbuild before and after this change. The speed of the compiler was measured by using it to type check a small TypeScript code base. Here are the results:

    |      | `tsc` | with esbuild 0.14.6 | with esbuild 0.14.7 |
    |------|-------|---------------------|---------------------|
    | Time | 2.96s | 3.45s               | 2.95s               |

    As you can see, enum inlining gives around a 15% speedup, which puts the esbuild-bundled version at the same speed as the offical TypeScript compiler build (the `tsc` column)!

    The specifics of the benchmark aren't important here since it's just a demonstration of how enum inlining can affect performance. But if you're wondering, I type checked the [Rollup](https://github.com/rollup/rollup) code base using a work-in-progress branch of the TypeScript compiler that's part of the ongoing effort to convert their use of namespaces into ES modules.

* Mark node built-in modules as having no side effects ([#705](https://github.com/evanw/esbuild/issues/705))

    This release marks node built-in modules such as `fs` as being side-effect free. That means unused imports to these modules are now removed when bundling, which sometimes results in slightly smaller code. For example:

    ```js
    // Original code
    import fs from 'fs';
    import path from 'path';
    console.log(path.delimiter);

    // Old output (with --bundle --minify --platform=node --format=esm)
    import"fs";import o from"path";console.log(o.delimiter);

    // New output (with --bundle --minify --platform=node --format=esm)
    import o from"path";console.log(o.delimiter);
    ```

    Note that these modules are only automatically considered side-effect when bundling for node, since they are only known to be side-effect free imports in that environment. However, you can customize this behavior with a plugin by returning `external: true` and `sideEffects: false` in an `onResolve` callback for whatever paths you want to be treated this way.

* Recover from a stray top-level `}` in CSS ([#1876](https://github.com/evanw/esbuild/pull/1876))

    This release fixes a bug where a stray `}` at the top-level of a CSS file would incorrectly truncate the remainder of the file in the output (although not without a warning). With this release, the remainder of the file is now still parsed and printed:

    ```css
    /* Original code */
    .red {
      color: red;
    }
    }
    .blue {
      color: blue;
    }
    .green {
      color: green;
    }

    /* Old output (with --minify) */
    .red{color:red}

    /* New output (with --minify) */
    .red{color:red}} .blue{color:#00f}.green{color:green}
    ```

    This fix was contributed by [@sbfaulkner](https://github.com/sbfaulkner).

## 0.14.6

* Fix a minifier bug with BigInt literals

    Previously expression simplification optimizations in the minifier incorrectly assumed that numeric operators always return numbers. This used to be true but has no longer been true since the introduction of BigInt literals in ES2020. Now numeric operators can return either a number or a BigInt depending on the arguments. This oversight could potentially have resulted in behavior changes. For example, this code printed `false` before being minified and `true` after being minified because esbuild shortened `===` to `==` under the false assumption that both operands were numbers:

    ```js
    var x = 0;
    console.log((x ? 2 : -1n) === -1);
    ```

    The type checking logic has been rewritten to take into account BigInt literals in this release, so this incorrect simplification is no longer applied.

* Enable removal of certain unused template literals ([#1853](https://github.com/evanw/esbuild/issues/1853))

    This release contains improvements to the minification of unused template literals containing primitive values:

    ```js
    // Original code
    `${1}${2}${3}`;
    `${x ? 1 : 2}${y}`;

    // Old output (with --minify)
    ""+1+2+3,""+(x?1:2)+y;

    // New output (with --minify)
    x,`${y}`;
    ```

    This can arise when the template literals are nested inside of another function call that was determined to be unnecessary such as an unused call to a function marked with the `/* @__PURE__ */` pragma.

    This release also fixes a bug with this transformation where minifying the unused expression `` `foo ${bar}` `` into `"" + bar` changed the meaning of the expression. Template string interpolation always calls `toString` while string addition may call `valueOf` instead. This unused expression is now minified to `` `${bar}` ``, which is slightly longer but which avoids the behavior change.

* Allow `keyof`/`readonly`/`infer` in TypeScript index signatures ([#1859](https://github.com/evanw/esbuild/pull/1859))

    This release fixes a bug that prevented these keywords from being used as names in index signatures. The following TypeScript code was previously rejected, but is now accepted:

    ```ts
    interface Foo {
      [keyof: string]: number
    }
    ```

    This fix was contributed by [@magic-akari](https://github.com/magic-akari).

* Avoid warning about `import.meta` if it's replaced ([#1868](https://github.com/evanw/esbuild/issues/1868))

    It's possible to replace the `import.meta` expression using the `--define:` feature. Previously doing that still warned that the `import.meta` syntax was not supported when targeting ES5. With this release, there will no longer be a warning in this case.

## 0.14.5

* Fix an issue with the publishing script

    This release fixes a missing dependency issue in the publishing script where it was previously possible for the published binary executable to have an incorrect version number.

## 0.14.4

* Adjust esbuild's handling of `default` exports and the `__esModule` marker ([#532](https://github.com/evanw/esbuild/issues/532), [#1591](https://github.com/evanw/esbuild/issues/1591), [#1719](https://github.com/evanw/esbuild/issues/1719))

    This change requires some background for context. Here's the history to the best of my understanding:

    When the ECMAScript module `import`/`export` syntax was being developed, the CommonJS module format (used in Node.js) was already widely in use. Because of this the export name called `default` was given special a syntax. Instead of writing `import { default as foo } from 'bar'` you can just write `import foo from 'bar'`. The idea was that when ECMAScript modules (a.k.a. ES modules) were introduced, you could import existing CommonJS modules using the new import syntax for compatibility. Since CommonJS module exports are dynamic while ES module exports are static, it's not generally possible to determine a CommonJS module's export names at module instantiation time since the code hasn't been evaluated yet. So the value of `module.exports` is just exported as the `default` export and the special `default` import syntax gives you easy access to `module.exports` (i.e. `const foo = require('bar')` is the same as `import foo from 'bar'`).

    However, it took a while for ES module syntax to be supported natively by JavaScript runtimes, and people still wanted to start using ES module syntax in the meantime. The [Babel](https://babeljs.io/) JavaScript compiler let you do this. You could transform each ES module file into a CommonJS module file that behaved the same. However, this transformation has a problem: emulating the `import` syntax accurately as described above means that `export default 0` and `import foo from 'bar'` will no longer line up when transformed to CommonJS. The code `export default 0` turns into `module.exports.default = 0` and the code `import foo from 'bar'` turns into `const foo = require('bar')`, meaning `foo` is `0` before the transformation but `foo` is `{ default: 0 }` after the transformation.

    To fix this, Babel sets the property `__esModule` to true as a signal to itself when it converts an ES module to a CommonJS module. Then, when importing a `default` export, it can know to use the value of `module.exports.default` instead of `module.exports` to make sure the behavior of the CommonJS modules correctly matches the behavior of the original ES modules. This fix has been widely adopted across the ecosystem and has made it into other tools such as TypeScript and even esbuild.

    However, when Node.js finally released their ES module implementation, they went with the original implementation where the `default` export is always `module.exports`, which broke compatibility with the existing ecosystem of ES modules that had been cross-compiled into CommonJS modules by Babel. You now have to either add or remove an additional `.default` property depending on whether your code needs to run in a Node environment or in a Babel environment, which created an interoperability headache. In addition, JavaScript tools such as esbuild now need to guess whether you want Node-style or Babel-style `default` imports. There's no way for a tool to know with certainty which one a given file is expecting and if your tool guesses wrong, your code will break.

    This release changes esbuild's heuristics around `default` exports and the `__esModule` marker to attempt to improve compatibility with Webpack and Node, which is what most packages are tuned for. The behavior changes are as follows:

    Old behavior:

    * If an `import` statement is used to load a CommonJS file and a) `module.exports` is an object, b) `module.exports.__esModule` is truthy, and c) the property `default` exists in `module.exports`, then esbuild would set the `default` export to `module.exports.default` (like Babel). Otherwise the `default` export was set to `module.exports` (like Node).

    * If a `require` call is used to load an ES module file, the returned module namespace object had the `__esModule` property set to true. This behaved as if the ES module had been converted to CommonJS via  a Babel-compatible transformation.

    * The `__esModule` marker could inconsistently appear on module namespace objects (i.e. `import * as`) when writing pure ESM code. Specifically, if a module namespace object was materialized then the `__esModule` marker was present, but if it was optimized away then the `__esModule` marker was absent.

    * It was not allowed to create an ES module export named `__esModule`. This avoided generating code that might break due to the inconsistency mentioned above, and also avoided issues with duplicate definitions of `__esModule`.

    New behavior:

    * If an `import` statement is used to load a CommonJS file and a) `module.exports` is an object, b) `module.exports.__esModule` is truthy, and c) the file name does not end in either `.mjs` or `.mts` and the `package.json` file does not contain `"type": "module"`, then esbuild will set the `default` export to `module.exports.default` (like Babel). Otherwise the `default` export is set to `module.exports` (like Node).

        Note that this means the `default` export may now be undefined in situations where it previously wasn't undefined. This matches Webpack's behavior so it should hopefully be more compatible.

        Also note that this means import behavior now depends on the file extension and on the contents of `package.json`. This also matches Webpack's behavior to hopefully improve compatibility.

    * If a `require` call is used to load an ES module file, the returned module namespace object has the `__esModule` property set to `true`. This behaves as if the ES module had been converted to CommonJS via  a Babel-compatible transformation.

    * If an `import` statement or `import()` expression is used to load an ES module, the `__esModule` marker should now never be present on the module namespace object. This frees up the `__esModule` export name for use with ES modules.

    * It's now allowed to use `__esModule` as a normal export name in an ES module. This property will be accessible to other ES modules but will not be accessible to code that loads the ES module using `require`, where they will observe the property set to `true` instead.

## 0.14.3

* Pass the current esbuild instance to JS plugins ([#1790](https://github.com/evanw/esbuild/issues/1790))

    Previously JS plugins that wanted to run esbuild had to `require('esbuild')` to get the esbuild object. However, that could potentially result in a different version of esbuild. This is also more complicated to do outside of node (such as within a browser). With this release, the current esbuild instance is now passed to JS plugins as the `esbuild` property:

    ```js
    let examplePlugin = {
      name: 'example',
      setup(build) {
        console.log(build.esbuild.version)
        console.log(build.esbuild.transformSync('1+2'))
      },
    }
    ```

* Disable `calc()` transform for results with non-finite numbers ([#1839](https://github.com/evanw/esbuild/issues/1839))

    This release disables minification of `calc()` expressions when the result contains `NaN`, `-Infinity`, or `Infinity`. These numbers are valid inside of `calc()` expressions but not outside of them, so the `calc()` expression must be preserved in these cases.

* Move `"use strict"` before injected shim imports ([#1837](https://github.com/evanw/esbuild/issues/1837))

    If a CommonJS file contains a `"use strict"` directive, it could potentially be unintentionally disabled by esbuild when using the "inject" feature when bundling is enabled. This is because the inject feature was inserting a call to the initializer for the injected file before the `"use strict"` directive. In JavaScript, directives do not apply if they come after a non-directive statement. This release fixes the problem by moving the `"use strict"` directive before the initializer for the injected file so it isn't accidentally disabled.

* Pass the ignored path query/hash suffix to `onLoad` plugins ([#1827](https://github.com/evanw/esbuild/issues/1827))

    The built-in `onResolve` handler that comes with esbuild can strip the query/hash suffix off of a path during path resolution. For example, `url("fonts/icons.eot?#iefix")` can be resolved to the file `fonts/icons.eot`. For context, IE8 has a bug where it considers the font face URL to extend to the last `)` instead of the first `)`. In the example below, IE8 thinks the URL for the font is `Example.eot?#iefix') format('eot'), url('Example.ttf') format('truetype` so by adding `?#iefix`, IE8 thinks the URL has a path of `Example.eot` and a query string of `?#iefix') format('eot...` and can load the font file:

    ```css
    @font-face {
      font-family: 'Example';
      src: url('Example.eot?#iefix') format('eot'), url('Example.ttf') format('truetype');
    }
    ```

    However, the suffix is not currently passed to esbuild and plugins may want to use this suffix for something. Previously plugins had to add their own `onResolve` handler if they wanted to use the query suffix. With this release, the suffix can now be returned by plugins from `onResolve` and is now passed to plugins in `onLoad`:

    ```js
    let examplePlugin = {
      name: 'example',
      setup(build) {
        build.onResolve({ filter: /.*/ }, args => {
          return { path: args.path, suffix: '?#iefix' }
        })

        build.onLoad({ filter: /.*/ }, args => {
          console.log({ path: args.path, suffix: args.suffix })
        })
      },
    }
    ```

    The suffix is deliberately not included in the path that's provided to plugins because most plugins won't know to handle this strange edge case and would likely break. Keeping the suffix out of the path means that plugins can opt-in to handling this edge case if they want to, and plugins that aren't aware of this edge case will likely still do something reasonable.

## 0.14.2

* Add `[ext]` placeholder for path templates ([#1799](https://github.com/evanw/esbuild/pull/1799))

    This release adds the `[ext]` placeholder to the `--entry-names=`, `--chunk-names=`, and `--asset-names=` configuration options. The `[ext]` placeholder takes the value of the file extension without the leading `.`, and can be used to place output files with different file extensions into different folders. For example, `--asset-names=assets/[ext]/[name]-[hash]` might generate an output path of `assets/png/image-LSAMBFUD.png`.

    This feature was contributed by [@LukeSheard](https://github.com/LukeSheard).

* Disable star-to-clause transform for external imports ([#1801](https://github.com/evanw/esbuild/issues/1801))

    When bundling is enabled, esbuild automatically transforms `import * as x from 'y'; x.z()` into `import {z} as 'y'; z()` to improve tree shaking. This avoids needing to create the import namespace object `x` if it's unnecessary, which can result in the removal of large amounts of unused code. However, this transform shouldn't be done for external imports because that incorrectly changes the semantics of the import. If the export `z` doesn't exist in the previous example, the value `x.z` is a property access that is undefined at run-time, but the value `z` is an import error that will prevent the code from running entirely. This release fixes the problem by avoiding doing this transform for external imports:

    ```js
    // Original code
    import * as x from 'y';
    x.z();

    // Old output (with --bundle --format=esm --external:y)
    import { z } from "y";
    z();

    // New output (with --bundle --format=esm --external:y)
    import * as x from "y";
    x.z();
    ```

* Disable `calc()` transform for numbers with many fractional digits ([#1821](https://github.com/evanw/esbuild/issues/1821))

    Version 0.13.12 introduced simplification of `calc()` expressions in CSS when minifying. For example, `calc(100% / 4)` turns into `25%`. However, this is problematic for numbers with many fractional digits because either the number is printed with reduced precision, which is inaccurate, or the number is printed with full precision, which could be longer than the original expression. For example, turning `calc(100% / 3)` into `33.33333%` is inaccurate and turning it into `33.333333333333336%` likely isn't desired. In this release, minification of `calc()` is now disabled when any number in the result cannot be represented to full precision with at most five fractional digits.

* Fix an edge case with `catch` scope handling ([#1812](https://github.com/evanw/esbuild/issues/1812))

    This release fixes a subtle edge case with `catch` scope and destructuring assignment. Identifiers in computed properties and/or default values inside the destructuring binding pattern should reference the outer scope, not the inner scope. The fix was to split the destructuring pattern into its own scope, separate from the `catch` body. Here's an example of code that was affected by this edge case:

    ```js
    // Original code
    let foo = 1
    try {
      throw ['a', 'b']
    } catch ({ [foo]: y }) {
      let foo = 2
      assert(y === 'b')
    }

    // Old output (with --minify)
    let foo=1;try{throw["a","b"]}catch({[o]:t}){let o=2;assert(t==="b")}

    // New output (with --minify)
    let foo=1;try{throw["a","b"]}catch({[foo]:t}){let o=2;assert(t==="b")}
    ```

* Go 1.17.2 was upgraded to Go 1.17.4

    The previous release was built with Go 1.17.2, but this release is built with Go 1.17.4. This is just a routine upgrade. There are no changes significant to esbuild outside of some security-related fixes to Go's HTTP stack (but you shouldn't be running esbuild's dev server in production anyway).

    One notable change related to this is that esbuild's publishing script now ensures that git's state is free of uncommitted and/or untracked files before building. Previously this wasn't the case because publishing esbuild involved changing the version number, running the publishing script, and committing at the end, which meant that files were uncommitted during the build process. I also typically had some untracked test files in the same directory during publishing (which is harmless).

    This matters because there's an upcoming change in Go 1.18 where the Go compiler will include metadata about whether there are untracked files or not when doing a build: https://github.com/golang/go/issues/37475. Changing esbuild's publishing script should mean that when esbuild upgrades to Go 1.18, esbuild's binary executables will be marked as being built off of a specific commit without any modifications. This is important for reproducibility. Checking out a specific esbuild commit and building it should give a bitwise-identical binary executable to one that I published. But if this metadata indicated that there were untracked files during the published build, then the resulting executable would no longer be bitwise-identical.

## 0.14.1

* Fix `imports` in `package.json` ([#1807](https://github.com/evanw/esbuild/issues/1807))

    This release contains a fix for the rarely-used [`imports` feature in `package.json` files](https://nodejs.org/api/packages.html#subpath-imports) that lets a package specify a custom remapping for import paths inside that package that start with `#`. Support for `imports` was added in version 0.13.9. However, the field was being incorrectly interpreted as relative to the importing file instead of to the `package.json` file, which caused an import failure when the importing file is in a subdirectory instead of being at the top level of the package. Import paths should now be interpreted as relative to the correct directory which should fix these path resolution failures.

* Isolate implicit sibling scope lookup for `enum` and `namespace`

    The previous release implemented sibling namespaces in TypeScript, which introduces a new kind of scope lookup that doesn't exist in JavaScript. Exported members inside an `enum` or `namespace` block can be implicitly referenced in a sibling `enum` or `namespace` block just by using the name without using a property reference. However, this behavior appears to only work for `enum`-to-`enum` and `namespace`-to-`namespace` interactions. Even though sibling enums and namespaces with the same name can be merged together into the same underlying object, this implicit reference behavior doesn't work for `enum`-to-`namespace` interactions and attempting to do this with a `namespace`-to-`enum` interaction [causes the TypeScript compiler itself to crash](https://github.com/microsoft/TypeScript/issues/46891). Here is an example of how the TypeScript compiler behaves in each case:

    ```ts
    // "b" is accessible
    enum a { b = 1 }
    enum a { c = b }

    // "e" is accessible
    namespace d { export let e = 1 }
    namespace d { export let f = e }

    // "h" is inaccessible
    enum g { h = 1 }
    namespace g { export let i = h }

    // This causes the TypeScript compiler to crash
    namespace j { export let k = 1 }
    enum j { l = k }
    ```

    This release changes the implicit sibling scope lookup behavior to only work for `enum`-to-`enum` and `namespace`-to-`namespace` interactions. These implicit references no longer work with `enum`-to-`namespace` and `namespace`-to-`enum` interactions, which should more accurately match the behavior of the TypeScript compiler.

* Add semicolon insertion before TypeScript-specific definite assignment assertion modifier ([#1810](https://github.com/evanw/esbuild/issues/1810))

    TypeScript lets you add a `!` after a variable declaration to bypass TypeScript's definite assignment analysis:

    ```ts
    let x!: number[];
    initialize();
    x.push(4);

    function initialize() { x = [0, 1, 2, 3]; }
    ```

    This `!` is called a [definite assignment assertion](https://devblogs.microsoft.com/typescript/announcing-typescript-2-7/#definite-assignment-assertions) and tells TypeScript to assume that the variable has been initialized somehow. However, JavaScript's automatic semicolon insertion rules should be able to insert a semicolon before it:

    ```ts
    let a
    !function(){}()
    ```

    Previously the above code was incorrectly considered a syntax error in TypeScript. With this release, this code is now parsed correctly.

* Log output to stderr has been overhauled

    This release changes the way log messages are formatted to stderr. The changes make the kind of message (e.g. error vs. warning vs. note) more obvious, and they also give more room for paragraph-style notes that can provide more detail about the message. Here's an example:

    Before:

    ```
     > example.tsx:14:25: warning: Comparison with -0 using the "===" operator will also match 0
        14 │     case 1: return x === -0
           ╵                          ~~
     > example.tsx:21:23: error: Could not resolve "path" (use "--platform=node" when building for node)
        21 │   const path = require('path')
           ╵                        ~~~~~~
    ```

    After:

    ```
    ▲ [WARNING] Comparison with -0 using the "===" operator will also match 0

        example.tsx:14:25:
          14 │     case 1: return x === -0
             ╵                          ~~

      Floating-point equality is defined such that 0 and -0 are equal, so "x === -0" returns true for
      both 0 and -0. You need to use "Object.is(x, -0)" instead to test for -0.

    ✘ [ERROR] Could not resolve "path"

        example.tsx:21:23:
          21 │   const path = require('path')
             ╵                        ~~~~~~

      The package "path" wasn't found on the file system but is built into node. Are you trying to
      bundle for node? You can use "--platform=node" to do that, which will remove this error.
    ```

    Note that esbuild's formatted log output is for humans, not for machines. If you need to output a stable machine-readable format, you should be using the API for that. Build and transform results have arrays called `errors` and `warnings` with objects that represent the log messages.

* Show inlined enum value names in comments

    When esbuild inlines an enum, it will now put a comment next to it with the original enum name:

    ```ts
    // Original code
    const enum Foo { FOO }
    console.log(Foo.FOO)

    // Old output
    console.log(0);

    // New output
    console.log(0 /* FOO */);
    ```

    This matches the behavior of the TypeScript compiler, and should help with debugging. These comments are not generated if minification is enabled.

## 0.14.0

**This release contains backwards-incompatible changes.** Since esbuild is before version 1.0.0, these changes have been released as a new minor version to reflect this (as [recommended by npm](https://docs.npmjs.com/cli/v6/using-npm/semver/)). You should either be pinning the exact version of `esbuild` in your `package.json` file or be using a version range syntax that only accepts patch upgrades such as `~0.13.0`. See the documentation about [semver](https://docs.npmjs.com/cli/v6/using-npm/semver/) for more information.

* Add support for TypeScript's `preserveValueImports` setting ([#1525](https://github.com/evanw/esbuild/issues/1525))

    TypeScript 4.5, which was just released, added [a new setting called `preserveValueImports`](https://devblogs.microsoft.com/typescript/announcing-typescript-4-5/#preserve-value-imports). This release of esbuild implements support for this new setting. However, this release also changes esbuild's behavior regarding the `importsNotUsedAsValues` setting, so this release is being considered a breaking change. Now esbuild's behavior should more accurately match the behavior of the TypeScript compiler. This is described in more detail below.

    The difference in behavior is around unused imports. By default, unused import names are considered to be types and are completely removed if they are unused. If all import names are removed for a given import statement, then the whole import statement is removed too. The two `tsconfig.json` settings [`importsNotUsedAsValues`](https://www.typescriptlang.org/tsconfig#importsNotUsedAsValues) and [`preserveValueImports`](https://www.typescriptlang.org/tsconfig#preserveValueImports) let you customize this. Here's what the TypeScript compiler's output looks like with these different settings enabled:

    ```ts
    // Original code
    import { unused } from "foo";

    // Default output
    /* (the import is completely removed) */

    // Output with "importsNotUsedAsValues": "preserve"
    import "foo";

    // Output with "preserveValueImports": true
    import { unused } from "foo";
    ```

    Previously, since the `preserveValueImports` setting didn't exist yet, esbuild had treated the `importsNotUsedAsValues` setting as if it were what is now the `preserveValueImports` setting instead. This was a deliberate deviation from how the TypeScript compiler behaves, but was necessary to allow esbuild to be used as a TypeScript-to-JavaScript compiler inside of certain composite languages such as Svelte and Vue. These languages append additional code after converting the TypeScript to JavaScript so unused imports may actually turn out to be used later on:

    ```svelte
    <script>
    import { someFunc } from "./some-module.js";
    </script>
    <button on:click={someFunc}>Click me!</button>
    ```

    Previously the implementers of these languages had to use the `importsNotUsedAsValues` setting as a hack for esbuild to preserve the import statements. With this release, esbuild now follows the behavior of the TypeScript compiler so implementers will need to use the new `preserveValueImports` setting to do this instead. This is the breaking change.

* TypeScript code follows JavaScript class field semantics with `--target=esnext` ([#1480](https://github.com/evanw/esbuild/issues/1480))

    TypeScript 4.3 included a subtle breaking change that wasn't mentioned in the [TypeScript 4.3 blog post](https://devblogs.microsoft.com/typescript/announcing-typescript-4-3/): class fields will now be compiled with different semantics if `"target": "ESNext"` is present in `tsconfig.json`. Specifically in this case `useDefineForClassFields` will default to `true` when not specified instead of `false`. This means class field behavior in TypeScript code will now match JavaScript instead of doing something else:

    ```js
    class Base {
      set foo(value) { console.log('set', value) }
    }
    class Derived extends Base {
      foo = 123
    }
    new Derived()
    ```

    In TypeScript 4.2 and below, the TypeScript compiler would generate code that prints `set 123` when `tsconfig.json` contains `"target": "ESNext"` but in TypeScript 4.3 and above, the TypeScript compiler will now generate code that doesn't print anything. This is the difference between "assign" semantics and "define" semantics.

    Previously you had to create a `tsconfig.json` file and specify `"target": "ESNext"` to get this behavior in esbuild. With this release, you can now also just pass `--target=esnext` to esbuild to force-enable this behavior. Note that esbuild doesn't do this by default even though the default value of `--target=` otherwise behaves like `esnext`. Since TypeScript's compiler doesn't do this behavior by default, it seems like a good idea for esbuild to not do this behavior by default either.

In addition to the breaking changes above, the following changes are also included in this release:

* Allow certain keywords as tuple type labels in TypeScript ([#1797](https://github.com/evanw/esbuild/issues/1797))

    Apparently TypeScript lets you use certain keywords as tuple labels but not others. For example, `type x = [function: number]` is allowed while `type x = [class: number]` isn't. This release replicates this behavior in esbuild's TypeScript parser:

    * Allowed keywords: `false`, `function`, `import`, `new`, `null`, `this`, `true`, `typeof`, `void`

    * Forbidden keywords: `break`, `case`, `catch`, `class`, `const`, `continue`, `debugger`, `default`, `delete`, `do`, `else`, `enum`, `export`, `extends`, `finally`, `for`, `if`, `in`, `instanceof`, `return`, `super`, `switch`, `throw`, `try`, `var`, `while`, `with`

* Support sibling namespaces in TypeScript ([#1410](https://github.com/evanw/esbuild/issues/1410))

    TypeScript has a feature where sibling namespaces with the same name can implicitly reference each other's exports without an explicit property access. This goes against how scope lookup works in JavaScript, so it previously didn't work with esbuild. This release adds support for this feature:

    ```ts
    // Original TypeScript code
    namespace x {
      export let y = 123
    }
    namespace x {
      export let z = y
    }

    // Old JavaScript output
    var x;
    (function(x2) {
      x2.y = 123;
    })(x || (x = {}));
    (function(x2) {
      x2.z = y;
    })(x || (x = {}));

    // New JavaScript output
    var x;
    (function(x2) {
      x2.y = 123;
    })(x || (x = {}));
    (function(x2) {
      x2.z = x2.y;
    })(x || (x = {}));
    ```

    Notice how the identifier `y` is now compiled to the property access `x2.y` which references the export named `y` on the namespace, instead of being left as the identifier `y` which references the global named `y`. This matches how the TypeScript compiler treats namespace objects. This new behavior also works for enums:

    ```ts
    // Original TypeScript code
    enum x {
      y = 123
    }
    enum x {
      z = y + 1
    }

    // Old JavaScript output
    var x;
    (function(x2) {
      x2[x2["y"] = 123] = "y";
    })(x || (x = {}));
    (function(x2) {
      x2[x2["z"] = y + 1] = "z";
    })(x || (x = {}));

    // New JavaScript output
    var x;
    (function(x2) {
      x2[x2["y"] = 123] = "y";
    })(x || (x = {}));
    (function(x2) {
      x2[x2["z"] = 124] = "z";
    })(x || (x = {}));
    ```

    Note that this behavior does **not** work across files. Each file is still compiled independently so the namespaces in each file are still resolved independently per-file. Implicit namespace cross-references still do not work across files. Getting this to work is counter to esbuild's parallel architecture and does not fit in with esbuild's design. It also doesn't make sense with esbuild's bundling model where input files are either in ESM or CommonJS format and therefore each have their own scope.

* Change output for top-level TypeScript enums

    The output format for top-level TypeScript enums has been changed to reduce code size and improve tree shaking, which means that esbuild's enum output is now somewhat different than TypeScript's enum output. The behavior of both output formats should still be equivalent though. Here's an example that shows the difference:

    ```ts
    // Original code
    enum x {
      y = 1,
      z = 2
    }

    // Old output
    var x;
    (function(x2) {
      x2[x2["y"] = 1] = "y";
      x2[x2["z"] = 2] = "z";
    })(x || (x = {}));

    // New output
    var x = /* @__PURE__ */ ((x2) => {
      x2[x2["y"] = 1] = "y";
      x2[x2["z"] = 2] = "z";
      return x2;
    })(x || {});
    ```

    The function expression has been changed to an arrow expression to reduce code size and the enum initializer has been moved into the variable declaration to make it possible to be marked as `/* @__PURE__ */` to improve tree shaking. The `/* @__PURE__ */` annotation is now automatically added when all of the enum values are side-effect free, which means the entire enum definition can be removed as dead code if it's never referenced. Direct enum value references within the same file that have been inlined do not count as references to the enum definition so this should eliminate enums from the output in many cases:

    ```ts
    // Original code
    enum Foo { FOO = 1 }
    enum Bar { BAR = 2 }
    console.log(Foo, Bar.BAR)

    // Old output (with --bundle --minify)
    var n;(function(e){e[e.FOO=1]="FOO"})(n||(n={}));var l;(function(e){e[e.BAR=2]="BAR"})(l||(l={}));console.log(n,2);

    // New output (with --bundle --minify)
    var n=(e=>(e[e.FOO=1]="FOO",e))(n||{});console.log(n,2);
    ```

    Notice how the new output is much shorter because the entire definition for `Bar` has been completely removed as dead code by esbuild's tree shaking.

    The output may seem strange since it would be simpler to just have a plain object literal as an initializer. However, TypeScript's enum feature behaves similarly to TypeScript's namespace feature which means enums can merge with existing enums and/or existing namespaces (and in some cases also existing objects) if the existing definition has the same name. This new output format keeps its similarity to the original output format so that it still handles all of the various edge cases that TypeScript's enum feature supports. Initializing the enum using a plain object literal would not merge with existing definitions and would break TypeScript's enum semantics.

* Fix legal comment parsing in CSS ([#1796](https://github.com/evanw/esbuild/issues/1796))

    Legal comments in CSS either start with `/*!` or contain `@preserve` or `@license` and are preserved by esbuild in the generated CSS output. This release fixes a bug where non-top-level legal comments inside a CSS file caused esbuild to skip any following legal comments even if those following comments are top-level:

    ```css
    /* Original code */
    .example {
      --some-var: var(--tw-empty, /*!*/ /*!*/);
    }
    /*! Some legal comment */
    body {
      background-color: red;
    }

    /* Old output (with --minify) */
    .example{--some-var: var(--tw-empty, )}body{background-color:red}

    /* New output (with --minify) */
    .example{--some-var: var(--tw-empty, )}/*! Some legal comment */body{background-color:red}
    ```

* Fix panic when printing invalid CSS ([#1803](https://github.com/evanw/esbuild/issues/1803))

    This release fixes a panic caused by a conditional CSS `@import` rule with a URL token. Code like this caused esbuild to enter an unexpected state because the case where tokens in the import condition with associated import records wasn't handled. This case is now handled correctly:

    ```css
    @import "example.css" url(foo);
    ```

* Mark `Set` and `Map` with array arguments as pure ([#1791](https://github.com/evanw/esbuild/issues/1791))

    This release introduces special behavior for references to the global `Set` and `Map` constructors that marks them as `/* @__PURE__ */` if they are known to not have any side effects. These constructors evaluate the iterator of whatever is passed to them and the iterator could have side effects, so this is only safe if whatever is passed to them is an array, since the array iterator has no side effects.

    Marking a constructor call as `/* @__PURE__ */` means it's safe to remove if the result is unused. This is an existing feature that you can trigger by manually adding a `/* @__PURE__ */` comment before a constructor call. The difference is that this release contains special behavior to automatically mark `Set` and `Map` as pure for you as long as it's safe to do so. As with all constructor calls that are marked `/* @__PURE__ */`, any internal expressions which could cause side effects are still preserved even though the constructor call itself is removed:

    ```js
    // Original code
    new Map([
      ['a', b()],
      [c(), new Set(['d', e()])],
    ]);

    // Old output (with --minify)
    new Map([["a",b()],[c(),new Set(["d",e()])]]);

    // New output (with --minify)
    b(),c(),e();
    ```

## 0.13.15

* Fix `super` in lowered `async` arrow functions ([#1777](https://github.com/evanw/esbuild/issues/1777))

    This release fixes an edge case that was missed when lowering `async` arrow functions containing `super` property accesses for compile targets that don't support `async` such as with `--target=es6`. The problem was that lowering transforms `async` arrow functions into generator function expressions that are then passed to an esbuild helper function called `__async` that implements the `async` state machine behavior. Since function expressions do not capture `this` and `super` like arrow functions do, this led to a mismatch in behavior which meant that the transform was incorrect. The fix is to introduce a helper function to forward `super` access into the generator function expression body. Here's an example:

    ```js
    // Original code
    class Foo extends Bar {
      foo() { return async () => super.bar() }
    }

    // Old output (with --target=es6)
    class Foo extends Bar {
      foo() {
        return () => __async(this, null, function* () {
          return super.bar();
        });
      }
    }

    // New output (with --target=es6)
    class Foo extends Bar {
      foo() {
        return () => {
          var __superGet = (key) => super[key];
          return __async(this, null, function* () {
            return __superGet("bar").call(this);
          });
        };
      }
    }
    ```

* Avoid merging certain CSS rules with different units ([#1732](https://github.com/evanw/esbuild/issues/1732))

    This release no longer collapses `border-radius`, `margin`, `padding`, and `inset` rules when they have units with different levels of browser support. Collapsing multiple of these rules into a single rule is not equivalent if the browser supports one unit but not the other unit, since one rule would still have applied before the collapse but no longer applies after the collapse due to the whole rule being ignored. For example, Chrome 10 supports the `rem` unit but not the `vw` unit, so the CSS code below should render with rounded corners in Chrome 10. However, esbuild previously merged everything into a single rule which would cause Chrome 10 to ignore the rule and not round the corners. This issue is now fixed:

    ```css
    /* Original CSS */
    div {
      border-radius: 1rem;
      border-top-left-radius: 1vw;
      margin: 0;
      margin-top: 1Q;
      left: 10Q;
      top: 20Q;
      right: 10Q;
      bottom: 20Q;
    }

    /* Old output (with --minify) */
    div{border-radius:1vw 1rem 1rem;margin:1Q 0 0;inset:20Q 10Q}

    /* New output (with --minify) */
    div{border-radius:1rem;border-top-left-radius:1vw;margin:0;margin-top:1Q;inset:20Q 10Q}
    ```

    Notice how esbuild can still collapse rules together when they all share the same unit, even if the unit is one that doesn't have universal browser support such as the unit `Q`. One subtlety is that esbuild now distinguishes between "safe" and "unsafe" units where safe units are old enough that they are guaranteed to work in any browser a user might reasonably use, such as `px`. Safe units are allowed to be collapsed together even if there are multiple different units while multiple different unsafe units are not allowed to be collapsed together. Another detail is that esbuild no longer minifies zero lengths by removing the unit if the unit is unsafe (e.g. `0rem` into `0`) since that could cause a rendering difference if a previously-ignored rule is now no longer ignored due to the unit change. If you are curious, you can learn more about browser support levels for different CSS units in [Mozilla's documentation about CSS length units](https://developer.mozilla.org/en-US/docs/Web/CSS/length).

* Avoid warning about ignored side-effect free imports for empty files ([#1785](https://github.com/evanw/esbuild/issues/1785))

    When bundling, esbuild warns about bare imports such as `import "lodash-es"` when the package has been marked as `"sideEffects": false` in its `package.json` file. This is because the only reason to use a bare import is because you are relying on the side effects of the import, but imports for packages marked as side-effect free are supposed to be removed. If the package indicates that it has no side effects, then this bare import is likely a bug.

    However, some people have packages just for TypeScript type definitions. These package can actually have a side effect as they can augment the type of the global object in TypeScript, even if they are marked with `"sideEffects": false`. To avoid warning in this case, esbuild will now only issue this warning if the imported file is non-empty. If the file is empty, then it's irrelevant whether you import it or not so any import of that file does not indicate a bug. This fixes this case because `.d.ts` files typically end up being empty after esbuild parses them since they typically only contain type declarations.

* Attempt to fix packages broken due to the `node:` prefix ([#1760](https://github.com/evanw/esbuild/issues/1760))

    Some people have started using the node-specific `node:` path prefix in their packages. This prefix forces the following path to be interpreted as a node built-in module instead of a package on the file system. So `require("node:path")` will always import [node's `path` module](https://nodejs.org/api/path.html) and never import [npm's `path` package](https://www.npmjs.com/package/path).

    Adding the `node:` prefix breaks that code with older node versions that don't understand the `node:` prefix. This is a problem with the package, not with esbuild. The package should be adding a fallback if the `node:` prefix isn't available. However, people still want to be able to use these packages with older node versions even though the code is broken. Now esbuild will automatically strip this prefix if it detects that the code will break in the configured target environment (as specified by `--target=`). Note that this only happens during bundling, since import paths are only examined during bundling.

## 0.13.14

* Fix dynamic `import()` on node 12.20+ ([#1772](https://github.com/evanw/esbuild/issues/1772))

    When you use flags such as `--target=node12.20`, esbuild uses that version number to see what features the target environment supports. This consults an internal table that stores which target environments are supported for each feature. For example, `import(x)` is changed into `Promise.resolve().then(() => require(x))` if dynamic `import` expressions are unsupported.

    Previously esbuild's internal table only stored one version number, since features are rarely ever removed in newer versions of software. Either the target environment is before that version and the feature is unsupported, or the target environment is after that version and the feature is supported. This approach has work for all relevant features in all cases except for one: dynamic `import` support in node. This feature is supported in node 12.20.0 up to but not including node 13.0.0, and then is also supported in node 13.2.0 up. The feature table implementation has been changed to store an array of potentially discontiguous version ranges instead of one version number.

    Up until now, esbuild used 13.2.0 as the lowest supported version number to avoid generating dynamic `import` expressions when targeting node versions that don't support it. But with this release, esbuild will now use the more accurate discontiguous version range in this case. This means dynamic `import` expressions can now be generated when targeting versions of node 12.20.0 up to but not including node 13.0.0.

* Avoid merging certain qualified rules in CSS ([#1776](https://github.com/evanw/esbuild/issues/1776))

    A change was introduced in the previous release to merge adjacent CSS rules that have the same content:

    ```css
    /* Original code */
    a { color: red }
    b { color: red }

    /* Minified output */
    a,b{color:red}
    ```

    However, that introduced a regression in cases where the browser considers one selector to be valid and the other selector to be invalid, such as in the following example:

    ```css
    /* This rule is valid, and is applied */
    a { color: red }

    /* This rule is invalid, and is ignored */
    b:-x-invalid { color: red }
    ```

    Merging these two rules into one causes the browser to consider the entire merged rule to be invalid, which disables both rules. This is a change in behavior from the original code.

    With this release, esbuild will now only merge adjacent duplicate rules together if they are known to work in all browsers (specifically, if they are known to work in IE 7 and up). Adjacent duplicate rules will no longer be merged in all other cases including modern pseudo-class selectors such as `:focus`, HTML5 elements such as `video`, and combinators such as `a + b`.

* Minify syntax in the CSS `font`, `font-family`, and `font-weight` properties ([#1756](https://github.com/evanw/esbuild/pull/1756))

    This release includes size reductions for CSS font syntax when minification is enabled:

    ```css
    /* Original code */
    div {
      font: bold 1rem / 1.2 "Segoe UI", sans-serif, "Segoe UI Emoji";
    }

    /* Output with "--minify" */
    div{font:700 1rem/1.2 Segoe UI,sans-serif,"Segoe UI Emoji"}
    ```

    Notice how `bold` has been changed to `700` and the quotes were removed around `"Segoe UI"` since it was safe to do so.

    This feature was contributed by [@sapphi-red](https://github.com/sapphi-red).

## 0.13.13

* Add more information about skipping `"main"` in `package.json` ([#1754](https://github.com/evanw/esbuild/issues/1754))

    Configuring `mainFields: []` breaks most npm packages since it tells esbuild to ignore the `"main"` field in `package.json`, which most npm packages use to specify their entry point. This is not a bug with esbuild because esbuild is just doing what it was told to do. However, people may do this without understanding how npm packages work, and then be confused about why it doesn't work. This release now includes additional information in the error message:

    ```
     > foo.js:1:27: error: Could not resolve "events" (use "--platform=node" when building for node)
         1 │ var EventEmitter = require('events')
           ╵                            ~~~~~~~~
       node_modules/events/package.json:20:2: note: The "main" field was ignored because the list of main fields to use is currently set to []
        20 │   "main": "./events.js",
           ╵   ~~~~~~
    ```

* Fix a tree-shaking bug with `var exports` ([#1739](https://github.com/evanw/esbuild/issues/1739))

    This release fixes a bug where a variable named `var exports = {}` was incorrectly removed by tree-shaking (i.e. dead code elimination). The `exports` variable is a special variable in CommonJS modules that is automatically provided by the CommonJS runtime. CommonJS modules are transformed into something like this before being run:

    ```js
    function(exports, module, require) {
      var exports = {}
    }
    ```

    So using `var exports = {}` should have the same effect as `exports = {}` because the variable `exports` should already be defined. However, esbuild was incorrectly overwriting the definition of the `exports` variable with the one provided by CommonJS. This release merges the definitions together so both are included, which fixes the bug.

* Merge adjacent CSS selector rules with duplicate content ([#1755](https://github.com/evanw/esbuild/issues/1755))

    With this release, esbuild will now merge adjacent selectors when minifying if they have the same content:

    ```css
    /* Original code */
    a { color: red }
    b { color: red }

    /* Old output (with --minify) */
    a{color:red}b{color:red}

    /* New output (with --minify) */
    a,b{color:red}
    ```

* Shorten `top`, `right`, `bottom`, `left` CSS property into `inset` when it is supported ([#1758](https://github.com/evanw/esbuild/pull/1758))

    This release enables collapsing of `inset` related properties:

    ```css
    /* Original code */
    div {
      top: 0;
      right: 0;
      bottom: 0;
      left: 0;
    }

    /* Output with "--minify-syntax" */
    div {
      inset: 0;
    }
    ```

    This minification rule is only enabled when `inset` property is supported by the target environment. Make sure to set esbuild's `target` setting correctly when minifying if the code will be running in an older environment (e.g. earlier than Chrome 87).

    This feature was contributed by [@sapphi-red](https://github.com/sapphi-red).

## 0.13.12

* Implement initial support for simplifying `calc()` expressions in CSS ([#1607](https://github.com/evanw/esbuild/issues/1607))

    This release includes basic simplification of `calc()` expressions in CSS when minification is enabled. The approach mainly follows the official CSS specification, which means it should behave the way browsers behave: https://www.w3.org/TR/css-values-4/#calc-func. This is a basic implementation so there are probably some `calc()` expressions that can be reduced by other tools but not by esbuild. This release mainly focuses on setting up the parsing infrastructure for `calc()` expressions to make it straightforward to implement additional simplifications in the future. Here's an example of this new functionality:

    ```css
    /* Input CSS */
    div {
      width: calc(60px * 4 - 5px * 2);
      height: calc(100% / 4);
    }

    /* Output CSS (with --minify-syntax) */
    div {
      width: 230px;
      height: 25%;
    }
    ```

    Expressions that can't be fully simplified will still be partially simplified into a reduced `calc()` expression:

    ```css
    /* Input CSS */
    div {
      width: calc(100% / 5 - 2 * 1em - 2 * 1px);
    }

    /* Output CSS (with --minify-syntax) */
    div {
      width: calc(20% - 2em - 2px);
    }
    ```

    Note that this transformation doesn't attempt to modify any expression containing a `var()` CSS variable reference. These variable references can contain any number of tokens so it's not safe to move forward with a simplification assuming that `var()` is a single token. For example, `calc(2px * var(--x) * 3)` is not transformed into `calc(6px * var(--x))` in case `var(--x)` contains something like `4 + 5px` (`calc(2px * 4 + 5px * 3)` evaluates to `23px` while `calc(6px * 4 + 5px)` evaluates to `29px`).

* Fix a crash with a legal comment followed by an import ([#1730](https://github.com/evanw/esbuild/issues/1730))

    Version 0.13.10 introduced parsing for CSS legal comments but caused a regression in the code that checks whether there are any rules that come before `@import`. This is not desired because browsers ignore `@import` rules after other non-`@import` rules, so esbuild warns you when you do this. However, legal comments are modeled as rules in esbuild's internal AST even though they aren't actual CSS rules, and the code that performs this check wasn't updated. This release fixes the crash.

## 0.13.11

* Implement class static blocks ([#1558](https://github.com/evanw/esbuild/issues/1558))

    This release adds support for a new upcoming JavaScript feature called [class static blocks](https://github.com/tc39/proposal-class-static-block) that lets you evaluate code inside of a class body. It looks like this:

    ```js
    class Foo {
      static {
        this.foo = 123
      }
    }
    ```

    This can be useful when you want to use `try`/`catch` or access private `#name` fields during class initialization. Doing that without this feature is quite hacky and basically involves creating temporary static fields containing immediately-invoked functions and then deleting the fields after class initialization. Static blocks are much more ergonomic and avoid performance loss due to `delete` changing the object shape.

    Static blocks are transformed for older browsers by moving the static block outside of the class body and into an immediately invoked arrow function after the class definition:

    ```js
    // The transformed version of the example code above
    const _Foo = class {
    };
    let Foo = _Foo;
    (() => {
      _Foo.foo = 123;
    })();
    ```

    In case you're wondering, the additional `let` variable is to guard against the potential reassignment of `Foo` during evaluation such as what happens below. The value of `this` must be bound to the original class, not to the current value of `Foo`:

    ```js
    let bar
    class Foo {
      static {
        bar = () => this
      }
    }
    Foo = null
    console.log(bar()) // This should not be "null"
    ```

* Fix issues with `super` property accesses

    Code containing `super` property accesses may need to be transformed even when they are supported. For example, in ES6 `async` methods are unsupported while `super` properties are supported. An `async` method containing `super` property accesses requires those uses of `super` to be transformed (the `async` function is transformed into a nested generator function and the `super` keyword cannot be used inside nested functions).

    Previously esbuild transformed `super` property accesses into a function call that returned the corresponding property. However, this was incorrect for uses of `super` that write to the inherited setter since a function call is not a valid assignment target. This release fixes writing to a `super` property:

    ```js
    // Original code
    class Base {
      set foo(x) { console.log('set foo to', x) }
    }
    class Derived extends Base {
      async bar() { super.foo = 123 }
    }
    new Derived().bar()

    // Old output with --target=es6 (contains a syntax error)
    class Base {
      set foo(x) {
        console.log("set foo to", x);
      }
    }
    class Derived extends Base {
      bar() {
        var __super = (key) => super[key];
        return __async(this, null, function* () {
          __super("foo") = 123;
        });
      }
    }
    new Derived().bar();

    // New output with --target=es6 (works correctly)
    class Base {
      set foo(x) {
        console.log("set foo to", x);
      }
    }
    class Derived extends Base {
      bar() {
        var __superSet = (key, value) => super[key] = value;
        return __async(this, null, function* () {
          __superSet("foo", 123);
        });
      }
    }
    new Derived().bar();
    ```

    All known edge cases for assignment to a `super` property should now be covered including destructuring assignment and using the unary assignment operators with BigInts.

    In addition, this release also fixes a bug where a `static` class field containing a `super` property access was not transformed when it was moved outside of the class body, which can happen when `static` class fields aren't supported.

    ```js
    // Original code
    class Base {
      static get foo() {
        return 123
      }
    }
    class Derived extends Base {
      static bar = super.foo
    }

    // Old output with --target=es6 (contains a syntax error)
    class Base {
      static get foo() {
        return 123;
      }
    }
    class Derived extends Base {
    }
    __publicField(Derived, "bar", super.foo);

    // New output with --target=es6 (works correctly)
    class Base {
      static get foo() {
        return 123;
      }
    }
    const _Derived = class extends Base {
    };
    let Derived = _Derived;
    __publicField(Derived, "bar", __superStaticGet(_Derived, "foo"));
    ```

    All known edge cases for `super` inside `static` class fields should be handled including accessing `super` after prototype reassignment of the enclosing class object.

## 0.13.10

* Implement legal comment preservation for CSS ([#1539](https://github.com/evanw/esbuild/issues/1539))

    This release adds support for legal comments in CSS the same way they are already supported for JS. A legal comment is one that starts with `/*!` or that contains the text `@license` or `@preserve`. These comments are preserved in output files by esbuild since that follows the intent of the original authors of the code. The specific behavior is controlled via `--legal-comments=` in the CLI and `legalComments` in the JS API, which can be set to any of the following options:

    * `none`: Do not preserve any legal comments
    * `inline`: Preserve all rule-level legal comments
    * `eof`: Move all rule-level legal comments to the end of the file
    * `linked`: Move all rule-level legal comments to a `.LEGAL.txt` file and link to them with a comment
    * `external`: Move all rule-level legal comments to a `.LEGAL.txt` file but to not link to them

    The default behavior is `eof` when bundling and `inline` otherwise.

* Allow uppercase `es*` targets ([#1717](https://github.com/evanw/esbuild/issues/1717))

    With this release, you can now use target names such as `ESNext` instead of `esnext` as the target name in the CLI and JS API. This is important because people don't want to have to call `.toLowerCase()` on target strings from TypeScript's `tsconfig.json` file before passing it to esbuild (TypeScript uses case-agnostic target names).

    This feature was contributed by [@timse](https://github.com/timse).

* Update to Unicode 14.0.0

    The character tables that determine which characters form valid JavaScript identifiers have been updated from Unicode version 13.0.0 to the newly-released Unicode version 14.0.0. I'm not putting an example in the release notes because all of the new characters will likely just show up as little squares since fonts haven't been updated yet. But you can read https://www.unicode.org/versions/Unicode14.0.0/#Summary for more information about the changes.

## 0.13.9

* Add support for `imports` in `package.json` ([#1691](https://github.com/evanw/esbuild/issues/1691))

    This release adds basic support for the `imports` field in `package.json`. It behaves similarly to the `exports` field but only applies to import paths that start with `#`. The `imports` field provides a way for a package to remap its own internal imports for itself, while the `exports` field provides a way for a package to remap its external exports for other packages. This is useful because the `imports` field respects the currently-configured conditions which means that the import mapping can change at run-time. For example:

    ```
    $ cat entry.mjs
    import '#example'

    $ cat package.json
    {
      "imports": {
        "#example": {
          "foo": "./example.foo.mjs",
          "default": "./example.mjs"
        }
      }
    }

    $ cat example.foo.mjs
    console.log('foo is enabled')

    $ cat example.mjs
    console.log('foo is disabled')

    $ node entry.mjs
    foo is disabled

    $ node --conditions=foo entry.mjs
    foo is enabled
    ```

    Now that esbuild supports this feature too, import paths starting with `#` and any provided conditions will be respected when bundling:

    ```
    $ esbuild --bundle entry.mjs | node
    foo is disabled

    $ esbuild --conditions=foo --bundle entry.mjs | node
    foo is enabled
    ```

* Fix using `npm rebuild` with the `esbuild` package ([#1703](https://github.com/evanw/esbuild/issues/1703))

    Version 0.13.4 accidentally introduced a regression in the install script where running `npm rebuild` multiple times could fail after the second time. The install script creates a copy of the binary executable using [`link`](https://man7.org/linux/man-pages/man2/link.2.html) followed by [`rename`](https://www.man7.org/linux/man-pages/man2/rename.2.html). Using `link` creates a hard link which saves space on the file system, and `rename` is used for safety since it atomically replaces the destination.

    However, the `rename` syscall has an edge case where it silently fails if the source and destination are both the same link. This meant that the install script would fail after being run twice in a row. With this release, the install script now deletes the source after calling `rename` in case it has silently failed, so this issue should now be fixed. It should now be safe to use `npm rebuild` with the `esbuild` package.

* Fix invalid CSS minification of `border-radius` ([#1702](https://github.com/evanw/esbuild/issues/1702))

    CSS minification does collapsing of `border-radius` related properties. For example:

    ```css
    /* Original CSS */
    div {
      border-radius: 1px;
      border-top-left-radius: 5px;
    }

    /* Minified CSS */
    div{border-radius:5px 1px 1px}
    ```

    However, this only works for numeric tokens, not identifiers. For example:

    ```css
    /* Original CSS */
    div {
      border-radius: 1px;
      border-top-left-radius: inherit;
    }

    /* Minified CSS */
    div{border-radius:1px;border-top-left-radius:inherit}
    ```

    Transforming this to `div{border-radius:inherit 1px 1px}`, as was done in previous releases of esbuild, is an invalid transformation and results in incorrect CSS. This release of esbuild fixes this CSS transformation bug.

## 0.13.8

* Fix `super` inside arrow function inside lowered `async` function ([#1425](https://github.com/evanw/esbuild/issues/1425))

    When an `async` function is transformed into a regular function for target environments that don't support `async` such as `--target=es6`, references to `super` inside that function must be transformed too since the `async`-to-regular function transformation moves the function body into a nested function, so the `super` references are no longer syntactically valid. However, this transform didn't handle an edge case and `super` references inside of an arrow function were overlooked. This release fixes this bug:

    ```js
    // Original code
    class Foo extends Bar {
      async foo() {
        return () => super.foo()
      }
    }

    // Old output (with --target=es6)
    class Foo extends Bar {
      foo() {
        return __async(this, null, function* () {
          return () => super.foo();
        });
      }
    }

    // New output (with --target=es6)
    class Foo extends Bar {
      foo() {
        var __super = (key) => super[key];
        return __async(this, null, function* () {
          return () => __super("foo").call(this);
        });
      }
    }
    ```

* Remove the implicit `/` after `[dir]` in entry names ([#1661](https://github.com/evanw/esbuild/issues/1661))

    The "entry names" feature lets you customize the way output file names are generated. The `[dir]` and `[name]` placeholders are filled in with the directory name and file name of the corresponding entry point file, respectively.

    Previously `--entry-names=[dir]/[name]` and `--entry-names=[dir][name]` behaved the same because the value used for `[dir]` always had an implicit trailing slash, since it represents a directory. However, some people want to be able to remove the file name with `--entry-names=[dir]` and the implicit trailing slash gets in the way.

    With this release, you can now use the `[dir]` placeholder without an implicit trailing slash getting in the way. For example, the command `esbuild foo/bar/index.js --outbase=. --outdir=out --entry-names=[dir]` previously generated the file `out/foo/bar/.js` but will now generate the file `out/foo/bar.js`.

## 0.13.7

* Minify CSS alpha values correctly ([#1682](https://github.com/evanw/esbuild/issues/1682))

    When esbuild uses the `rgba()` syntax for a color instead of the 8-character hex code (e.g. when `target` is set to Chrome 61 or earlier), the 0-to-255 integer alpha value must be printed as a floating-point fraction between 0 and 1. The fraction was only printed to three decimal places since that is the minimal number of decimal places required for all 256 different alpha values to be uniquely determined. However, using three decimal places does not necessarily result in the shortest result. For example, `128 / 255` is `0.5019607843137255` which is printed as `".502"` using three decimal places, but `".5"` is equivalent because `round(0.5 * 255) == 128`, so printing `".5"` would be better. With this release, esbuild will always use the minimal numeric representation for the alpha value:

    ```css
    /* Original code */
    a { color: #FF800080 }

    /* Old output (with --minify --target=chrome61) */
    a{color:rgba(255,128,0,.502)}

    /* New output (with --minify --target=chrome61) */
    a{color:rgba(255,128,0,.5)}
    ```

* Match node's behavior for core module detection ([#1680](https://github.com/evanw/esbuild/issues/1680))

    Node has a hard-coded list of core modules (e.g. `fs`) that, when required, short-circuit the module resolution algorithm and instead return the corresponding internal core module object. When you pass `--platform=node` to esbuild, esbuild also implements this short-circuiting behavior and doesn't try to bundle these import paths. This was implemented in esbuild using the existing `external` feature (e.g. essentially `--external:fs`). However, there is an edge case where esbuild's `external` feature behaved differently than node.

    Modules specified via esbuild's `external` feature also cause all sub-paths to be excluded as well, so for example `--external:foo` excludes both `foo` and `foo/bar` from the bundle. However, node's core module check is only an exact equality check, so for example `fs` is a core module and bypasses the module resolution algorithm but `fs/foo` is not a core module and causes the module resolution algorithm to search the file system.

    This behavior can be used to load a module on the file system with the same name as one of node's core modules. For example, `require('fs/')` will load the module `fs` from the file system instead of loading node's core `fs` module. With this release, esbuild will now match node's behavior in this edge case. This means the external modules that are automatically added by `--platform=node` now behave subtly differently than `--external:`, which allows code that relies on this behavior to be bundled correctly.

* Fix WebAssembly builds on Go 1.17.2+ ([#1684](https://github.com/evanw/esbuild/pull/1684))

    Go 1.17.2 introduces a change (specifically a [fix for CVE-2021-38297](https://go-review.googlesource.com/c/go/+/354591/)) that causes Go's WebAssembly bootstrap script to throw an error when it's run in situations with many environment variables. One such situation is when the bootstrap script is run inside [GitHub Actions](https://github.com/features/actions). This change was introduced because the bootstrap script writes a copy of the environment variables into WebAssembly memory without any bounds checking, and writing more than 4096 bytes of data ends up writing past the end of the buffer and overwriting who-knows-what. So throwing an error in this situation is an improvement. However, this breaks esbuild which previously (at least seemingly) worked fine.

    With this release, esbuild's WebAssembly bootstrap script that calls out to Go's WebAssembly bootstrap script will now delete all environment variables except for the ones that esbuild checks for, of which there are currently only four: `NO_COLOR`, `NODE_PATH`, `npm_config_user_agent`, and `WT_SESSION`. This should avoid a crash when esbuild is built using Go 1.17.2+ and should reduce the likelihood of memory corruption when esbuild is built using Go 1.17.1 or earlier. This release also updates the Go version that esbuild ships with to version 1.17.2. Note that this problem only affects the `esbuild-wasm` package. The `esbuild` package is not affected.

    See also:

    * https://github.com/golang/go/issues/48797
    * https://github.com/golang/go/issues/49011

## 0.13.6

* Emit decorators for `declare` class fields ([#1675](https://github.com/evanw/esbuild/issues/1675))

    In version 3.7, TypeScript introduced the `declare` keyword for class fields that avoids generating any code for that field:

    ```ts
    // TypeScript input
    class Foo {
      a: number
      declare b: number
    }

    // JavaScript output
    class Foo {
      a;
    }
    ```

    However, it turns out that TypeScript still emits decorators for these omitted fields. With this release, esbuild will now do this too:

    ```ts
    // TypeScript input
    class Foo {
      @decorator a: number;
      @decorator declare b: number;
    }

    // Old JavaScript output
    class Foo {
      a;
    }
    __decorateClass([
      decorator
    ], Foo.prototype, "a", 2);

    // New JavaScript output
    class Foo {
      a;
    }
    __decorateClass([
      decorator
    ], Foo.prototype, "a", 2);
    __decorateClass([
      decorator
    ], Foo.prototype, "b", 2);
    ```

* Experimental support for esbuild on NetBSD ([#1624](https://github.com/evanw/esbuild/pull/1624))

    With this release, esbuild now has a published binary executable for [NetBSD](https://www.netbsd.org/) in the [`esbuild-netbsd-64`](https://www.npmjs.com/package/esbuild-netbsd-64) npm package, and esbuild's installer has been modified to attempt to use it when on NetBSD. Hopefully this makes installing esbuild via npm work on NetBSD. This change was contributed by [@gdt](https://github.com/gdt).

    ⚠️ Note: NetBSD is not one of [Node's supported platforms](https://nodejs.org/api/process.html#process_process_platform), so installing esbuild may or may not work on NetBSD depending on how Node has been patched. This is not a problem with esbuild. ⚠️

* Disable the "esbuild was bundled" warning if `ESBUILD_BINARY_PATH` is provided ([#1678](https://github.com/evanw/esbuild/pull/1678))

    The `ESBUILD_BINARY_PATH` environment variable allows you to substitute an alternate binary executable for esbuild's JavaScript API. This is useful in certain cases such as when debugging esbuild. The JavaScript API has some code that throws an error if it detects that it was bundled before being run, since bundling prevents esbuild from being able to find the path to its binary executable. However, that error is unnecessary if `ESBUILD_BINARY_PATH` is present because an alternate path has been provided. This release disables the warning when `ESBUILD_BINARY_PATH` is present so that esbuild can be used when bundled as long as you also manually specify `ESBUILD_BINARY_PATH`.

    This change was contributed by [@heypiotr](https://github.com/heypiotr).

* Remove unused `catch` bindings when minifying ([#1660](https://github.com/evanw/esbuild/pull/1660))

    With this release, esbuild will now remove unused `catch` bindings when minifying:

    ```js
    // Original code
    try {
      throw 0;
    } catch (e) {
    }

    // Old output (with --minify)
    try{throw 0}catch(t){}

    // New output (with --minify)
    try{throw 0}catch{}
    ```

    This takes advantage of the new [optional catch binding](https://github.com/tc39/proposal-optional-catch-binding) syntax feature that was introduced in ES2019. This minification rule is only enabled when optional catch bindings are supported by the target environment. Specifically, it's not enabled when using `--target=es2018` or older. Make sure to set esbuild's `target` setting correctly when minifying if the code will be running in an older JavaScript environment.

    This change was contributed by [@sapphi-red](https://github.com/sapphi-red).

## 0.13.5

* Improve watch mode accuracy ([#1113](https://github.com/evanw/esbuild/issues/1113))

    Watch mode is enabled by `--watch` and causes esbuild to become a long-running process that automatically rebuilds output files when input files are changed. It's implemented by recording all calls to esbuild's internal file system interface and then invalidating the build whenever these calls would return different values. For example, a call to esbuild's internal `ReadFile()` function is considered to be different if either the presence of the file has changed (e.g. the file didn't exist before but now exists) or the presence of the file stayed the same but the content of the file has changed.

    Previously esbuild's watch mode operated at the `ReadFile()` and `ReadDirectory()` level. When esbuild checked whether a directory entry existed or not (e.g. whether a directory contains a `node_modules` subdirectory or a `package.json` file), it called `ReadDirectory()` which then caused the build to depend on that directory's set of entries. This meant the build would be invalidated even if a new unrelated entry was added or removed, since that still changes the set of entries. This is problematic when using esbuild in environments that constantly create and destroy temporary directory entries in your project directory. In that case, esbuild's watch mode would constantly rebuild as the directory was constantly considered to be dirty.

    With this release, watch mode now operates at the `ReadFile()` and `ReadDirectory().Get()` level. So when esbuild checks whether a directory entry exists or not, the build should now only depend on the presence status for that one directory entry. This should avoid unnecessary rebuilds due to unrelated directory entries being added or removed. The log messages generated using `--watch` will now also mention the specific directory entry whose presence status was changed if a build is invalidated for this reason.

    Note that this optimization does not apply to plugins using the `watchDirs` return value because those paths are only specified at the directory level and do not describe individual directory entries. You can use `watchFiles` or `watchDirs` on the individual entries inside the directory to get a similar effect instead.

* Disallow certain uses of `<` in `.mts` and `.cts` files

    The upcoming version 4.5 of TypeScript is introducing the `.mts` and `.cts` extensions that turn into the `.mjs` and `.cjs` extensions when compiled. However, unlike the existing `.ts` and `.tsx` extensions, expressions that start with `<` are disallowed when they would be ambiguous depending on whether they are parsed in `.ts` or `.tsx` mode. The ambiguity is caused by the overlap between the syntax for JSX elements and the old deprecated syntax for type casts:

    | Syntax                        | `.ts`                | `.tsx`           | `.mts`/`.cts`        |
    |-------------------------------|----------------------|------------------|----------------------|
    | `<x>y`                        | ✅ Type cast         | 🚫 Syntax error   | 🚫 Syntax error      |
    | `<T>() => {}`                 | ✅ Arrow function    | 🚫 Syntax error   | 🚫 Syntax error      |
    | `<x>y</x>`                    | 🚫 Syntax error      | ✅ JSX element    | 🚫 Syntax error      |
    | `<T>() => {}</T>`             | 🚫 Syntax error      | ✅ JSX element    | 🚫 Syntax error      |
    | `<T extends>() => {}</T>`     | 🚫 Syntax error      | ✅ JSX element    | 🚫 Syntax error      |
    | `<T extends={0}>() => {}</T>` | 🚫 Syntax error      | ✅ JSX element    | 🚫 Syntax error      |
    | `<T,>() => {}`                | ✅ Arrow function    | ✅ Arrow function | ✅ Arrow function    |
    | `<T extends X>() => {}`       | ✅ Arrow function    | ✅ Arrow function | ✅ Arrow function    |

    This release of esbuild introduces a syntax error for these ambiguous syntax constructs in `.mts` and `.cts` files to match the new behavior of the TypeScript compiler.

* Do not remove empty `@keyframes` rules ([#1665](https://github.com/evanw/esbuild/issues/1665))

    CSS minification in esbuild automatically removes empty CSS rules, since they have no effect. However, empty `@keyframes` rules still trigger JavaScript animation events so it's incorrect to remove them. To demonstrate that empty `@keyframes` rules still have an effect, here is a bug report for Firefox where it was incorrectly not triggering JavaScript animation events for empty `@keyframes` rules: https://bugzilla.mozilla.org/show_bug.cgi?id=1004377.

    With this release, empty `@keyframes` rules are now preserved during minification:

    ```css
    /* Original CSS */
    @keyframes foo {
      from {}
      to {}
    }

    /* Old output (with --minify) */

    /* New output (with --minify) */
    @keyframes foo{}
    ```

    This fix was contributed by [@eelco](https://github.com/eelco).

* Fix an incorrect duplicate label error ([#1671](https://github.com/evanw/esbuild/pull/1671))

    When labeling a statement in JavaScript, the label must be unique within the enclosing statements since the label determines the jump target of any labeled `break` or `continue` statement:

    ```js
    // This code is valid
    x: y: z: break x;

    // This code is invalid
    x: y: x: break x;
    ```

    However, an enclosing label with the same name *is* allowed as long as it's located in a different function body. Since `break` and `continue` statements can't jump across function boundaries, the label is not ambiguous. This release fixes a bug where esbuild incorrectly treated this valid code as a syntax error:

    ```js
    // This code is valid, but was incorrectly considered a syntax error
    x: (() => {
      x: break x;
    })();
    ```

    This fix was contributed by [@nevkontakte](https://github.com/nevkontakte).

## 0.13.4

* Fix permission issues with the install script ([#1642](https://github.com/evanw/esbuild/issues/1642))

    The `esbuild` package contains a small JavaScript stub file that implements the CLI (command-line interface). Its only purpose is to spawn the binary esbuild executable as a child process and forward the command-line arguments to it.

    The install script contains an optimization that replaces this small JavaScript stub with the actual binary executable at install time to avoid the overhead of unnecessarily creating a new `node` process. This optimization can't be done at package publish time because there is only one `esbuild` package but there are many supported platforms, so the binary executable for the current platform must live outside of the `esbuild` package.

    However, the optimization was implemented with an [unlink](https://www.man7.org/linux/man-pages/man2/unlink.2.html) operation followed by a [link](https://www.man7.org/linux/man-pages/man2/link.2.html) operation. This means that if the first step fails, the package is left in a broken state since the JavaScript stub file is deleted but not yet replaced.

    With this release, the optimization is now implemented with a [link](https://www.man7.org/linux/man-pages/man2/link.2.html) operation followed by a [rename](https://www.man7.org/linux/man-pages/man2/rename.2.html) operation. This should always leave the package in a working state even if either step fails.

* Add a fallback for `npm install esbuild --no-optional` ([#1647](https://github.com/evanw/esbuild/issues/1647))

    The installation method for esbuild's platform-specific binary executable was recently changed in version 0.13.0. Before that version esbuild downloaded it in an install script, and after that version esbuild lets the package manager download it using the `optionalDependencies` feature in `package.json`. This change was made because downloading the binary executable in an install script never really fully worked. The reasons are complex but basically there are a variety of edge cases where people people want to install esbuild in environments that they have customized such that downloading esbuild isn't possible. Using `optionalDependencies` instead lets the package manager deal with it instead, which should work fine in all cases (either that or your package manager has a bug, but that's not esbuild's problem).

    There is one case where this new installation method doesn't work: if you pass the `--no-optional` flag to npm to disable the `optionalDependencies` feature. If you do this, you prevent esbuild from being installed. This is not a problem with esbuild because you are manually enabling a flag to change npm's behavior such that esbuild doesn't install correctly. However, people still want to do this.

    With this release, esbuild will now fall back to the old installation method if the new installation method fails. **THIS MAY NOT WORK.** The new `optionalDependencies` installation method is the only supported way to install esbuild with npm. The old downloading installation method was removed because it doesn't always work. The downloading method is only being provided to try to be helpful but it's not the supported installation method. If you pass `--no-optional` and the download fails due to some environment customization you did, the recommended fix is to just remove the `--no-optional` flag.

* Support the new `.mts` and `.cts` TypeScript file extensions

    The upcoming version 4.5 of TypeScript has two new file extensions: `.mts` and `.cts`. Files with these extensions can be imported using the `.mjs` and `.cjs`, respectively. So the statement `import "./foo.mjs"` in TypeScript can actually succeed even if the file `./foo.mjs` doesn't exist on the file system as long as the file `./foo.mts` does exist. The import path with the `.mjs` extension is automatically re-routed to the corresponding file with the `.mts` extension at type-checking time by the TypeScript compiler. See [the TypeScript 4.5 beta announcement](https://devblogs.microsoft.com/typescript/announcing-typescript-4-5-beta/#new-file-extensions) for details.

    With this release, esbuild will also automatically rewrite `.mjs` to `.mts` and `.cjs` to `.cts` when resolving import paths to files on the file system. This should make it possible to bundle code written in this new style. In addition, the extensions `.mts` and `.cts` are now also considered valid TypeScript file extensions by default along with the `.ts` extension.

* Fix invalid CSS minification of `margin` and `padding` ([#1657](https://github.com/evanw/esbuild/issues/1657))

    CSS minification does collapsing of `margin` and `padding` related properties. For example:

    ```css
    /* Original CSS */
    div {
      margin: auto;
      margin-top: 5px;
      margin-left: 5px;
    }

    /* Minified CSS */
    div{margin:5px auto auto 5px}
    ```

    However, while this works for the `auto` keyword, it doesn't work for other keywords. For example:

    ```css
    /* Original CSS */
    div {
      margin: inherit;
      margin-top: 5px;
      margin-left: 5px;
    }

    /* Minified CSS */
    div{margin:inherit;margin-top:5px;margin-left:5px}
    ```

    Transforming this to `div{margin:5px inherit inherit 5px}`, as was done in previous releases of esbuild, is an invalid transformation and results in incorrect CSS. This release of esbuild fixes this CSS transformation bug.

## 0.13.3

* Support TypeScript type-only import/export specifiers ([#1637](https://github.com/evanw/esbuild/pull/1637))

    This release adds support for a new TypeScript syntax feature in the upcoming version 4.5 of TypeScript. This feature lets you prefix individual imports and exports with the `type` keyword to indicate that they are types instead of values. This helps tools such as esbuild omit them from your source code, and is necessary because esbuild compiles files one-at-a-time and doesn't know at parse time which imports/exports are types and which are values. The new syntax looks like this:

    ```ts
    // Input TypeScript code
    import { type Foo } from 'foo'
    export { type Bar }

    // Output JavaScript code (requires "importsNotUsedAsValues": "preserve" in "tsconfig.json")
    import {} from "foo";
    export {};
    ```

    See [microsoft/TypeScript#45998](https://github.com/microsoft/TypeScript/pull/45998) for full details. From what I understand this is a purely ergonomic improvement since this was already previously possible using a type-only import/export statements like this:

    ```ts
    // Input TypeScript code
    import type { Foo } from 'foo'
    export type { Bar }
    import 'foo'
    export {}

    // Output JavaScript code (requires "importsNotUsedAsValues": "preserve" in "tsconfig.json")
    import "foo";
    export {};
    ```

    This feature was contributed by [@g-plane](https://github.com/g-plane).

## 0.13.2

* Fix `export {}` statements with `--tree-shaking=true` ([#1628](https://github.com/evanw/esbuild/issues/1628))

    The new `--tree-shaking=true` option allows you to force-enable tree shaking in cases where it wasn't previously possible. One such case is when bundling is disabled and there is no output format configured, in which case esbuild just preserves the format of whatever format the input code is in. Enabling tree shaking in this context caused a bug where `export {}` statements were stripped. This release fixes the bug so `export {}` statements should now be preserved when you pass `--tree-shaking=true`. This bug only affected this new functionality and didn't affect existing scenarios.

## 0.13.1

* Fix the `esbuild` package in yarn 2+

    The [yarn package manager](https://yarnpkg.com/) version 2 and above has a mode called [PnP](https://next.yarnpkg.com/features/pnp/) that installs packages inside zip files instead of using individual files on disk, and then hijacks node's `fs` module to pretend that paths to files inside the zip file are actually individual files on disk so that code that wasn't written specifically for yarn still works. Unfortunately that hijacking is incomplete and it still causes certain things to break such as using these zip file paths to create a JavaScript worker thread or to create a child process.

    This was an issue for the new `optionalDependencies` package installation strategy that was just released in version 0.13.0 since the binary executable is now inside of an installed package instead of being downloaded using an install script. When it's installed with yarn 2+ in PnP mode the binary executable is inside a zip file and can't be run. To work around this, esbuild detects yarn's PnP mode and copies the binary executable to a real file outside of the zip file.

    Unfortunately the code to do this didn't create the parent directory before writing to the file path. That caused esbuild's API to crash when it was run for the first time. This didn't come up during testing because the parent directory already existed when the tests were run. This release changes the location of the binary executable from a shared cache directory to inside the esbuild package itself, which should fix this crash. This problem only affected esbuild's JS API when it was run through yarn 2+ with PnP mode active.

## 0.13.0

**This release contains backwards-incompatible changes.** Since esbuild is before version 1.0.0, these changes have been released as a new minor version to reflect this (as [recommended by npm](https://docs.npmjs.com/cli/v6/using-npm/semver/)). You should either be pinning the exact version of `esbuild` in your `package.json` file or be using a version range syntax that only accepts patch upgrades such as `~0.12.0`. See the documentation about [semver](https://docs.npmjs.com/cli/v6/using-npm/semver/) for more information.

* Allow tree shaking to be force-enabled and force-disabled ([#1518](https://github.com/evanw/esbuild/issues/1518), [#1610](https://github.com/evanw/esbuild/issues/1610), [#1611](https://github.com/evanw/esbuild/issues/1611), [#1617](https://github.com/evanw/esbuild/pull/1617))

    This release introduces a breaking change that gives you more control over when tree shaking happens ("tree shaking" here refers to declaration-level dead code removal). Previously esbuild's tree shaking was automatically enabled or disabled for you depending on the situation and there was no manual override to change this. Specifically, tree shaking was only enabled either when bundling was enabled or when the output format was set to `iife` (i.e. wrapped in an immediately-invoked function expression). This was done to avoid issues with people appending code to output files in the `cjs` and `esm` formats and expecting that code to be able to reference code in the output file that isn't otherwise referenced.

    You now have the ability to explicitly force-enable or force-disable tree shaking to bypass this default behavior. This is a breaking change because there is already a setting for tree shaking that does something else, and it has been moved to a separate setting instead. The previous setting allowed you to control whether or not to ignore manual side-effect annotations, which is related to tree shaking since only side-effect free code can be removed as dead code. Specifically you can annotate function calls with `/* @__PURE__ */` to indicate that they can be removed if they are not used, and you can annotate packages with `"sideEffects": false` to indicate that imports of that package can be removed if they are not used. Being able to ignore these annotations is necessary because [they are sometimes incorrect](https://github.com/tensorflow/tfjs/issues/4248). This previous setting has been moved to a separate setting because it actually impacts dead-code removal within expressions, which also applies when minifying with tree-shaking disabled.

    ### Old behavior

    * CLI
        * Ignore side-effect annotations: `--tree-shaking=ignore-annotations`
    * JS
        * Ignore side-effect annotations: `treeShaking: 'ignore-annotations'`
    * Go
        * Ignore side-effect annotations: `TreeShaking: api.TreeShakingIgnoreAnnotations`

    ### New behavior

    * CLI
        * Ignore side-effect annotations: `--ignore-annotations`
        * Force-disable tree shaking: `--tree-shaking=false`
        * Force-enable tree shaking: `--tree-shaking=true`
    * JS
        * Ignore side-effect annotations: `ignoreAnnotations: true`
        * Force-disable tree shaking: `treeShaking: false`
        * Force-enable tree shaking: `treeShaking: true`
    * Go
        * Ignore side-effect annotations: `IgnoreAnnotations: true`
        * Force-disable tree shaking: `TreeShaking: api.TreeShakingFalse`
        * Force-enable tree shaking: `TreeShaking: api.TreeShakingTrue`

* The npm package now uses `optionalDependencies` to install the platform-specific binary executable ([#286](https://github.com/evanw/esbuild/issues/286), [#291](https://github.com/evanw/esbuild/issues/291), [#319](https://github.com/evanw/esbuild/issues/319), [#347](https://github.com/evanw/esbuild/issues/347), [#369](https://github.com/evanw/esbuild/issues/369), [#547](https://github.com/evanw/esbuild/issues/547), [#565](https://github.com/evanw/esbuild/issues/565), [#789](https://github.com/evanw/esbuild/issues/789), [#921](https://github.com/evanw/esbuild/issues/921), [#1193](https://github.com/evanw/esbuild/issues/1193), [#1270](https://github.com/evanw/esbuild/issues/1270), [#1382](https://github.com/evanw/esbuild/issues/1382), [#1422](https://github.com/evanw/esbuild/issues/1422), [#1450](https://github.com/evanw/esbuild/issues/1450), [#1485](https://github.com/evanw/esbuild/issues/1485), [#1546](https://github.com/evanw/esbuild/issues/1546), [#1547](https://github.com/evanw/esbuild/pull/1547), [#1574](https://github.com/evanw/esbuild/issues/1574), [#1609](https://github.com/evanw/esbuild/issues/1609))

    This release changes esbuild's installation strategy in an attempt to improve compatibility with edge cases such as custom registries, custom proxies, offline installations, read-only file systems, or when post-install scripts are disabled. It's being treated as a breaking change out of caution because it's a significant change to how esbuild works with JS package managers, and hasn't been widely tested yet.

    **The old installation strategy** manually downloaded the correct binary executable in a [post-install script](https://docs.npmjs.com/cli/v7/using-npm/scripts). The binary executable is hosted in a separate platform-specific npm package such as [`esbuild-darwin-64`](https://www.npmjs.com/package/esbuild-darwin-64). The install script first attempted to download the package via the `npm` command in case npm had custom network settings configured. If that didn't work, the install script attempted to download the package from https://registry.npmjs.org/ before giving up. This was problematic for many reasons including:

    * Not all of npm's settings can be forwarded due to npm bugs such as https://github.com/npm/cli/issues/2284, and npm has said these bugs will never be fixed.
    * Some people have configured their network environments such that downloading from https://registry.npmjs.org/ will hang instead of either succeeding or failing.
    * The installed package was broken if you used `npm --ignore-scripts` because then the post-install script wasn't run. Some people enable this option so that malicious packages must be run first before being able to do malicious stuff.

    **The new installation strategy** automatically downloads the correct binary executable using npm's `optionalDependencies` feature to depend on all esbuild packages for all platforms but only have the one for the current platform be installed. This is a built-in part of the package manager so my assumption is that it should work correctly in all of these edge cases that currently don't work. And if there's an issue with this, then the problem is with the package manager instead of with esbuild so this should hopefully reduce the maintenance burden on esbuild itself. Changing to this installation strategy has these drawbacks:

    * Old versions of certain package managers (specifically npm and yarn) print lots of useless log messages during the installation, at least one for each platform other than the current one. These messages are harmless and can be ignored. However, they are annoying. There is nothing I can do about this. If you have this problem, one solution is to upgrade your package manager to a newer version.

    * Installation will be significantly slower in old versions of npm, old versions of pnpm, and all versions of yarn. These package managers download all packages for all platforms even though they aren't needed and actually cannot be used. This problem has been fixed in npm and pnpm and the problem has been communicated to yarn: https://github.com/yarnpkg/berry/issues/3317. If you have this problem, one solution is to use a newer version of npm or pnpm as your package manager.

    * This installation strategy does not work if you use `npm --no-optional` since then the package with the binary executable is not installed. If you have this problem, the solution is to not pass the `--no-optional` flag when installing packages.

    * There is still a small post-install script but it's now optional in that the `esbuild` package should still function correctly if post-install scripts are disabled (such as with `npm --ignore-scripts`). This post-install script optimizes the installed package by replacing the `esbuild` JavaScript command shim with the actual binary executable at install time. This avoids the overhead of launching another `node` process when using the `esbuild` command. So keep in mind that installing with `--ignore-scripts` will result in a slower `esbuild` command.

    Despite the drawbacks of the new installation strategy, I believe this change is overall a good thing to move forward with. It should fix edge case scenarios where installing esbuild currently doesn't work at all, and this only comes at the expense of the install script working in a less-optimal way (but still working) if you are using an old version of npm. So I'm going to switch installation strategies and see how it goes.

    The platform-specific binary executables are still hosted on npm in the same way, so anyone who wrote code that downloads builds from npm using the instructions here should not have to change their code: https://esbuild.github.io/getting-started/#download-a-build. However, note that these platform-specific packages no longer specify the `bin` field in `package.json` so the `esbuild` command will no longer be automatically put on your path. The `bin` field had to be removed because of a collision with the `bin` field of the `esbuild` package (now that the `esbuild` package depends on all of these platform-specific packages as optional dependencies).

In addition to the breaking changes above, the following features are also included in this release:

* Treat `x` guarded by `typeof x !== 'undefined'` as side-effect free

    This is a small tree-shaking (i.e. dead code removal) improvement. Global identifier references are considered to potentially have side effects since they will throw a reference error if the global identifier isn't defined, and code with side effects cannot be removed as dead code. However, there's a somewhat-common case where the identifier reference is guarded by a `typeof` check to check that it's defined before accessing it. With this release, code that does this will now be considered to have no side effects which allows it to be tree-shaken:

    ```js
    // Original code
    var __foo = typeof foo !== 'undefined' && foo;
    var __bar = typeof bar !== 'undefined' && bar;
    console.log(__bar);

    // Old output (with --bundle, which enables tree-shaking)
    var __foo = typeof foo !== 'undefined' && foo;
    var __bar = typeof bar !== 'undefined' && bar;
    console.log(__bar);

    // New output (with --bundle, which enables tree-shaking)
    var __bar = typeof bar !== 'undefined' && bar;
    console.log(__bar);
    ```

## 0.12.29

* Fix compilation of abstract class fields in TypeScript ([#1623](https://github.com/evanw/esbuild/issues/1623))

    This release fixes a bug where esbuild could incorrectly include a TypeScript abstract class field in the compiled JavaScript output. This is incorrect because the official TypeScript compiler never does this. Note that this only happened in scenarios where TypeScript's `useDefineForClassFields` setting was set to `true` (or equivalently where TypeScript's `target` setting was set to `ESNext`). Here is the difference:

    ```js
    // Original code
    abstract class Foo {
      abstract foo: any;
    }

    // Old output
    class Foo {
      foo;
    }

    // New output
    class Foo {
    }
    ```

* Proxy from the `__require` shim to `require` ([#1614](https://github.com/evanw/esbuild/issues/1614))

    Some background: esbuild's bundler emulates a CommonJS environment. The bundling process replaces the literal syntax `require(<string>)` with the referenced module at compile-time. However, other uses of `require` such as `require(someFunction())` are not bundled since the value of `someFunction()` depends on code evaluation, and esbuild does not evaluate code at compile-time. So it's possible for some references to `require` to remain after bundling.

    This was causing problems for some CommonJS code that was run in the browser and that expected `typeof require === 'function'` to be true (see [#1202](https://github.com/evanw/esbuild/issues/1202)), since the browser does not provide a global called `require`. Thus esbuild introduced a shim `require` function called `__require` (shown below) and replaced all references to `require` in the bundled code with `__require`:

    ```js
    var __require = x => {
      if (typeof require !== 'undefined') return require(x);
      throw new Error('Dynamic require of "' + x + '" is not supported');
    };
    ```

    However, this broke code that referenced `require.resolve` inside the bundle, which could hypothetically actually work since you could assign your own implementation to `window.require.resolve` (see [#1579](https://github.com/evanw/esbuild/issues/1579)). So the implementation of `__require` was changed to this:

    ```js
    var __require = typeof require !== 'undefined' ? require : x => {
      throw new Error('Dynamic require of "' + x + '" is not supported');
    };
    ```

    However, that broke code that assigned to `window.require` later on after the bundle was loaded ([#1614](https://github.com/evanw/esbuild/issues/1614)). So with this release, the code for `__require` now handles all of these edge cases:

    * `typeof require` is still `function` even if `window.require` is undefined
    * `window.require` can be assigned to either before or after the bundle is loaded
    * `require.resolve` and arbitrary other properties can still be accessed
    * `require` will now forward any number of arguments, not just the first one

    Handling all of these edge cases is only possible with the [Proxy API](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Proxy). So the implementation of `__require` now looks like this:

    ```js
    var __require = (x =>
      typeof require !== 'undefined' ? require :
      typeof Proxy !== 'undefined' ? new Proxy(x, {
        get: (a, b) => (typeof require !== 'undefined' ? require : a)[b]
      }) : x
    )(function(x) {
      if (typeof require !== 'undefined') return require.apply(this, arguments);
      throw new Error('Dynamic require of "' + x + '" is not supported');
    });
    ```

* Consider `typeof x` to have no side effects

    The `typeof` operator does not itself trigger any code evaluation so it can safely be removed if evaluating the operand does not cause any side effects. However, there is a special case of the `typeof` operator when the operand is an identifier expression. In that case no reference error is thrown if the referenced symbol does not exist (e.g. `typeof x` does not throw an error if there is no symbol named `x`). With this release, esbuild will now consider `typeof x` to have no side effects even if evaluating `x` would have side effects (i.e. would throw a reference error):

    ```js
    // Original code
    var unused = typeof React !== 'undefined';

    // Old output
    var unused = typeof React !== 'undefined';

    // New output
    ```

    Note that there is actually an edge case where `typeof x` *can* throw an error: when `x` is being referenced inside of its TDZ, or temporal dead zone (i.e. before it's declared). This applies to `let`, `const`, and `class` symbols. However, esbuild doesn't currently handle TDZ rules so the possibility of errors thrown due to TDZ rules is not currently considered. This typically doesn't matter in real-world code so this hasn't been a priority to fix (and is actually tricky to fix with esbuild's current bundling approach). So esbuild may incorrectly remove a `typeof` expression that actually has side effects. However, esbuild already incorrectly did this in previous releases so its behavior regarding `typeof` and TDZ rules hasn't changed in this release.

## 0.12.28

* Fix U+30FB and U+FF65 in identifier names in ES5 vs. ES6+ ([#1599](https://github.com/evanw/esbuild/issues/1599))

    The ES6 specification caused two code points that were previously valid in identifier names in ES5 to no longer be valid in identifier names in ES6+. The two code points are:

    * `U+30FB` i.e. `KATAKANA MIDDLE DOT` i.e. `・`
    * `U+FF65` i.e. `HALFWIDTH KATAKANA MIDDLE DOT` i.e. `･`

    This means that using ES6+ parsing rules will fail to parse some valid ES5 code, and generating valid ES5 code may fail to be parsed using ES6+ parsing rules. For example, esbuild would previously fail to parse `x.y･` even though it's valid ES5 code (since it's not valid ES6+ code) and esbuild could generate `{y･:x}` when minifying even though it's not valid ES6+ code (since it's valid ES5 code). This problem is the result of my incorrect assumption that ES6 is a superset of ES5.

    As of this release, esbuild will now parse a superset of ES5 and ES6+ and will now quote identifier names when possible if it's not considered to be a valid identifier name in either ES5 or ES6+. In other words, a union of ES5 and ES6 rules is used for parsing and the intersection of ES5 and ES6 rules is used for printing.

* Fix `++` and `--` on class private fields when used with big integers ([#1600](https://github.com/evanw/esbuild/issues/1600))

    Previously when esbuild lowered class private fields (e.g. `#foo`) to older JavaScript syntax, the transform of the `++` and `--` was not correct if the value is a big integer such as `123n`. The transform in esbuild is similar to Babel's transform which [has the same problem](https://github.com/babel/babel/issues/13756). Specifically, the code was transformed into code that either adds or subtracts the number `1` and `123n + 1` throws an exception in JavaScript. This problem has been fixed so this should now work fine starting with this release.

## 0.12.27

* Update JavaScript syntax feature compatibility tables ([#1594](https://github.com/evanw/esbuild/issues/1594))

    Most JavaScript syntax feature compatibility data is able to be obtained automatically via https://kangax.github.io/compat-table/. However, they are missing data for quite a few new JavaScript features (see ([kangax/compat-table#1034](https://github.com/kangax/compat-table/issues/1034))) so data on these new features has to be added manually. This release manually adds a few new entries:

    * Top-level await

        This feature lets you use `await` at the top level of a module, outside of an `async` function. Doing this holds up the entire module instantiation operation until the awaited expression is resolved or rejected. This release marks this feature as supported in Edge 89, Firefox 89, and Safari 15 (it was already marked as supported in Chrome 89 and Node 14.8). The data source for this is https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Operators/await.

    * Arbitrary module namespace identifier names

        This lets you use arbitrary strings as module namespace identifier names as long as they are valid UTF-16 strings. An example is `export { x as "🍕" }` which can then be imported as `import { "🍕" as y } from "./example.js"`. This release marks this feature as supported in Firefox 87 (it was already marked as supported in Chrome 90 and Node 16). The data source for this is https://bugzilla.mozilla.org/show_bug.cgi?id=1670044.

    I would also like to add data for Safari. They have recently added support for arbitrary module namespace identifier names (https://bugs.webkit.org/show_bug.cgi?id=217576) and `export * as` (https://bugs.webkit.org/show_bug.cgi?id=214379). However, I have no idea how to determine which Safari release these bugs correspond to so this compatibility data for Safari has been omitted.

* Avoid unnecessary additional log messages after the server is stopped ([#1589](https://github.com/evanw/esbuild/issues/1589))

    There is a development server built in to esbuild which is accessible via the `serve()` API call. This returns a promise that resolves to an object with a `stop()` method that immediately terminates the development server. Previously calling this could cause esbuild to print stray log messages since `stop()` could cause plugins to be unregistered while a build is still in progress. With this release, calling `stop()` no longer terminates the development server immediately. It now waits for any active builds to finish first so the builds are not interrupted and left in a confusing state.

* Fix an accidental dependency on Go ≥1.17.0 ([#1585](https://github.com/evanw/esbuild/pull/1585))

    The source code of this release no longer uses the `math.MaxInt` constant that was introduced in Go version 1.17.0. This constant was preventing esbuild from being compiled on Go version <1.17.0. This fix was contributed by [@davezuko](https://github.com/davezuko).

## 0.12.26

* Add `--analyze` to print information about the bundle ([#1568](https://github.com/evanw/esbuild/issues/1568))

    The `--metafile=` flag tells esbuild to write information about the bundle into the provided metadata file in JSON format. It contains information about the input files and which other files each one imports, as well as the output files and which input files they include. This information is sufficient to answer many questions such as:

    * Which files are in my bundle?
    * What's are the biggest files in my bundle?
    * Why is this file included in my bundle?

    Previously you had to either write your own code to answer these questions, or use another tool such as https://bundle-buddy.com/esbuild to visualize the data. Starting with this release you can now also use `--analyze` to enable esbuild's built-in visualizer. It looks like this:

    ```
    $ esbuild --bundle example.jsx --outfile=out.js --minify --analyze

      out.js  27.6kb

    ⚡ Done in 6ms

      out.js                                                                    27.6kb  100.0%
       ├ node_modules/react-dom/cjs/react-dom-server.browser.production.min.js  19.2kb   69.8%
       ├ node_modules/react/cjs/react.production.min.js                          5.9kb   21.4%
       ├ node_modules/object-assign/index.js                                     965b     3.4%
       ├ example.jsx                                                             137b     0.5%
       ├ node_modules/react-dom/server.browser.js                                 50b     0.2%
       └ node_modules/react/index.js                                              50b     0.2%
    ```

    This tells you what input files were bundled into each output file as well as the final minified size contribution of each input file as well as the percentage of the output file it takes up. You can also enable verbose analysis with `--analyze=verbose` to see why each input file was included (i.e. which files imported it from the entry point file):

    ```
    $ esbuild --bundle example.jsx --outfile=out.js --minify --analyze=verbose

      out.js  27.6kb

    ⚡ Done in 6ms

      out.js ─────────────────────────────────────────────────────────────────── 27.6kb ─ 100.0%
       ├ node_modules/react-dom/cjs/react-dom-server.browser.production.min.js ─ 19.2kb ── 69.8%
       │  └ node_modules/react-dom/server.browser.js
       │     └ example.jsx
       ├ node_modules/react/cjs/react.production.min.js ───────────────────────── 5.9kb ── 21.4%
       │  └ node_modules/react/index.js
       │     └ example.jsx
       ├ node_modules/object-assign/index.js ──────────────────────────────────── 965b ──── 3.4%
       │  └ node_modules/react-dom/cjs/react-dom-server.browser.production.min.js
       │     └ node_modules/react-dom/server.browser.js
       │        └ example.jsx
       ├ example.jsx ──────────────────────────────────────────────────────────── 137b ──── 0.5%
       ├ node_modules/react-dom/server.browser.js ──────────────────────────────── 50b ──── 0.2%
       │  └ example.jsx
       └ node_modules/react/index.js ───────────────────────────────────────────── 50b ──── 0.2%
          └ example.jsx
    ```

    There is also a JS API for this:

    ```js
    const result = await esbuild.build({
      metafile: true,
      ...
    })
    console.log(await esbuild.analyzeMetafile(result.metafile, {
      verbose: true,
    }))
    ```

    and a Go API:

    ```js
    result := api.Build(api.BuildOptions{
      Metafile: true,
      ...
    })
    fmt.Println(api.AnalyzeMetafile(result.Metafile, api.AnalyzeMetafileOptions{
      Verbose: true,
    }))
    ```

    Note that this is not the only way to visualize this data. If you want a visualization that's different than the information displayed here, you can easily build it yourself using the information in the metafile that is generated with the `--metafile=` flag.

    Also note that this data is intended for humans, not machines. The specific format of this data may change over time which will likely break any tools that try to parse it. You should not write a tool to parse this data. You should be using the information in the JSON metadata file instead. Everything in this visualization is derived from the JSON metadata so you are not losing out on any information by not using esbuild's output.

* Allow `require.resolve` in non-node builds ([#1579](https://github.com/evanw/esbuild/issues/1579))

    With this release, you can now use `require.resolve` in builds when the target platform is set to `browser` instead of `node` as long as the function `window.require.resolve` exists somehow. This was already possible when the platform is `node` but when the platform is `browser`, esbuild generates a no-op shim `require` function for compatibility reasons (e.g. because some code expects `typeof require` must be `"function"` even in the browser). The shim previously had a fallback to `window.require` if it exists, but additional properties of the `require` function such as `require.resolve` were not copied over to the shim. Now the shim function is only used if `window.require` is undefined so additional properties such as `require.resolve` should now work.

    This change was contributed by [@screetBloom](https://github.com/screetBloom).

## 0.12.25

* Fix a TypeScript parsing edge case with the postfix `!` operator ([#1560](https://github.com/evanw/esbuild/issues/1560))

    This release fixes a bug with esbuild's TypeScript parser where the postfix `!` operator incorrectly terminated a member expression after the `new` operator:

    ```js
    // Original input
    new Foo!.Bar();

    // Old output
    new Foo().Bar();

    // New output
    new Foo.Bar();
    ```

    The problem was that `!` was considered a postfix operator instead of part of a member expression. It is now considered to be part of a member expression instead, which fixes this edge case.

* Fix a parsing crash with nested private brand checks

    This release fixes a bug in the parser where code of the form `#a in #b in c` caused a crash. This code now causes a syntax error instead. Private identifiers are allowed when followed by `in`, but only if the operator precedence level is such that the `in` operator is allowed. The parser was missing the operator precedence check.

* Publish x86-64 binary executables for illumos ([#1562](https://github.com/evanw/esbuild/pull/1562))

    This release adds support for the [illumos](https://www.illumos.org/) operating system, which is related to Solaris and SunOS. Support for this platform was contributed by [@hadfl](https://github.com/hadfl).

## 0.12.24

* Fix an edge case with direct `eval` and variable renaming

    Use of the direct `eval` construct causes all variable names in the scope containing the direct `eval` and all of its parent scopes to become "pinned" and unable to be renamed. This is because the dynamically-evaluated code is allowed to reference any of those variables by name. When this happens esbuild avoids renaming any of these variables, which effectively disables minification for most of the file, and avoids renaming any non-pinned variables to the name of a pinned variable.

    However, there was previously a bug where the pinned variable name avoidance only worked for pinned variables in the top-level scope but not in nested scopes. This could result in a non-pinned variable being incorrectly renamed to the name of a pinned variable in certain cases. For example:

    ```js
    // Input to esbuild
    return function($) {
      function foo(arg) {
        return arg + $;
      }
      // Direct "eval" here prevents "$" from being renamed
      // Repeated "$" puts "$" at the top of the character frequency histogram
      return eval(foo($$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$))
    }(2);
    ```

    When this code is minified with `--minify-identifiers`, the non-pinned variable `arg` is incorrectly transformed into `$` resulting in a name collision with the nested pinned variable `$`:

    ```js
    // Old output from esbuild (incorrect)
    return function($) {
      function foo($) {
        return $ + $;
      }
      return eval(foo($$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$));
    }(2);
    ```

    This is because the non-pinned variable `arg` is renamed to the top character in the character frequency histogram `$` (esbuild uses a character frequency histogram for smaller gzipped output sizes) and the pinned variable `$` was incorrectly not present in the list of variable names to avoid. With this release, the output is now correct:

    ```js
    // New output from esbuild (correct)
    return function($) {
      function foo(n) {
        return n + $;
      }
      return eval(foo($$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$));
    }(2);
    ```

    Note that even when esbuild handles direct `eval` correctly, using direct `eval` is not recommended because it disables minification for the file and likely won't work correctly in the presence of scope hoisting optimizations. See https://esbuild.github.io/link/direct-eval for more details.

## 0.12.23

* Parsing of rest arguments in certain TypeScript types ([#1553](https://github.com/evanw/esbuild/issues/1553))

    This release implements parsing of rest arguments inside object destructuring inside arrow functions inside TypeScript type declarations. Support for rest arguments in this specific syntax was not previously implemented. The following code was incorrectly considered a syntax error before this release, but is no longer considered a syntax error:

    ```ts
    type F = ({ ...rest }) => void;
    ```

* Fix error message for `watch: true` and `buildSync` ([#1552](https://github.com/evanw/esbuild/issues/1552))

    Watch mode currently only works with the `build` API. Previously using watch mode with the `buildSync` API caused a confusing error message. This release explicitly disallows doing this, so the error message is now more clear.

* Fix an minification bug with the `--keep-names` option ([#1552](https://github.com/evanw/esbuild/issues/1552))

    This release fixes a subtle bug that happens with `--keep-names --minify` and nested function declarations in strict mode code. It can be triggered by the following code, which was being compiled incorrectly under those flags:

    ```js
    export function outer() {
      {
        function inner() {
          return Math.random();
        }
        const x = inner();
        console.log(x);
      }
    }
    outer();
    ```

    The bug was caused by an unfortunate interaction between a few of esbuild's behaviors:

    1. Function declarations inside of nested scopes behave differently in different situations, so esbuild rewrites this function declaration to a local variable initialized to a function expression instead so that it behaves the same in all situations.

        More specifically, the interpretation of such function declarations depends on whether or not it currently exists in a strict mode context:

        ```
        > (function(){ { function x(){} } return x })()
        function x() {}

        > (function(){ 'use strict'; { function x(){} } return x })()
        ❌ Uncaught ReferenceError: x is not defined
        ```

        The bundling process sometimes erases strict mode context. For example, different files may have different strict mode status but may be merged into a single file which all shares the same strict mode status. Also, files in ESM format are automatically in strict mode but a bundle output file in IIFE format may not be executed in strict mode. Transforming the nested `function` to a `let` in strict mode and a `var` in non-strict mode means esbuild's output will behave reliably in different environments.

    2. The "keep names" feature adds automatic calls to the built-in `__name` helper function to assign the original name to the `.name` property of the minified function object at run-time. That transforms the code into this:

        ```js
        let inner = function() {
          return Math.random();
        };
        __name(inner, "inner");
        const x = inner();
        console.log(x);
        ```

        This injected helper call does not count as a use of the associated function object so that dead-code elimination will still remove the function object as dead code if nothing else uses it. Otherwise dead-code elimination would stop working when the "keep names" feature is enabled.

    3. Minification enables an optimization where an initialized variable with a single use immediately following that variable is transformed by inlining the initializer into the use. So for example `var a = 1; return a` is transformed into `return 1`. This code matches this pattern (initialized single-use variable + use immediately following that variable) so the optimization does the inlining, which transforms the code into this:

        ```js
        __name(function() {
          return Math.random();
        }, "inner");
        const x = inner();
        console.log(x);
        ```

        The code is now incorrect because `inner` actually has two uses, although only one was actually counted.

    This inlining optimization will now be avoided in this specific case, which fixes the bug without regressing dead-code elimination or initialized variable inlining in any other cases.

## 0.12.22

* Make HTTP range requests more efficient ([#1536](https://github.com/evanw/esbuild/issues/1536))

    The local HTTP server built in to esbuild supports [range requests](https://developer.mozilla.org/en-US/docs/Web/HTTP/Range_requests), which are necessary for video playback in Safari. This means you can now use `<video>` tags in your HTML pages with esbuild's local HTTP server.

    Previously this was implemented inefficiently for files that aren't part of the build, but that are read from the underlying fallback directory. In that case the entire file was being read even though only part of the file was needed. In this release, only the part of the file that is needed is read so using HTTP range requests with esbuild in this case will now use less memory.

* Fix CSS minification bug with `box-shadow` and `var()` ([#1538](https://github.com/evanw/esbuild/issues/1538))

    The `box-shadow` property can be specified using 2, 3, or 4 numbers. The 3rd and 4th numbers are the blur radius and spread radius, and can be omitted if zero. When minifying, esbuild has an optimization that removes trailing zeros from runs of numbers within the `box-shadow` property. However, that optimization is not correct in the presence of tokens that are neither a number, a color, nor the token `insert`. These edge cases include `var()` or `calc()` tokens. With this release, esbuild will now do stronger validation and will only remove trailing zeros if the contents of the `box-shadow` property matches the underlying CSS grammar exactly.

    ```css
    /* Original code */
    button {
      box-shadow: 0 0 0 var(--spread) red;
    }

    /* Old minified output */
    button{box-shadow:0 0 var(--spread) red}

    /* New minified output */
    button{box-shadow:0 0 0 var(--spread) red}
    ```

## 0.12.21

* Add support for native esbuild on Windows 64-bit ARM ([#995](https://github.com/evanw/esbuild/issues/995))

    The newly-released Go version 1.17.0 [adds support for Windows 64-bit ARM CPUs](https://golang.org/doc/go1.17#windows), so esbuild can now support these CPUs as well. This release introduces support for `npm install esbuild` on Windows 64-bit ARM.

## 0.12.20

* Avoid the sequence `</style` in CSS output ([#1509](https://github.com/evanw/esbuild/issues/1509))

    The CSS code generator now avoids generating the character sequence `</style` in case you want to embed the CSS output in a `<style>...</style>` tag inside HTML:

    ```css
    /* Original code */
    a:after {
      content: "</style>";
    }

    /* Old output */
    a:after {
      content: "</style>";
    }

    /* New output */
    a:after {
      content: "<\/style>";
    }
    ```

    This mirrors how the JS code generator similarly avoids the character sequence `</script`.

    In addition, the check that escapes `</style` and `</script` is now case-insensitive to match how the browser's HTML parser behaves. So `</STYLE` and `</SCRIPT` are now escaped as well.

* Fix a TypeScript parsing edge case with ASI (Automatic Semicolon Insertion) ([#1512](https://github.com/evanw/esbuild/issues/1512))

    This fixes a parsing bug where TypeScript types consisting of multiple identifiers joined together with a `.` could incorrectly extend onto the next line if the next line started with `<`. This problem was due to ASI; esbuild should be automatically inserting a semicolon at the end of the line:

    ```ts
    let x: {
      <A extends B>(): c.d /* A semicolon should be automatically inserted here */
      <E extends F>(): g.h
    }
    ```

    Previously the above code was incorrectly considered a syntax error since esbuild attempted to parse the parameterized type `c.d<E extends F ? ...>`. With this release, this code is now parsed correctly.

## 0.12.19

* Add support for CSS source maps ([#519](https://github.com/evanw/esbuild/issues/519))

    With this release, esbuild will now generate source maps for CSS output files when `--sourcemap` is enabled. This supports all of the same options as JS source maps including `--sourcemap=inline` and `--sourcemap=external`. In addition, CSS input files with embedded `/*# sourceMappingURL=... */` comments will cause the CSS output file source map to map all the way back to the original inputs. CSS source maps are used by the browser's style inspector to link back to the original source code instead of linking to the bundled source code.

* Fix computed class fields in TypeScript edge case ([#1507](https://github.com/evanw/esbuild/issues/1507))

    If TypeScript code contains computed class fields, the target environment supports class fields so syntax lowering is not necessary, and TypeScript's `useDefineForClassFields` setting is set to `true`, then esbuild had a bug where the computed property names were computed after the class definition and were undefined. Note that TypeScript's `useDefineForClassFields` setting defaults to `true` if `tsconfig.json` contains `"target": "ESNext"`.

    ```ts
    // Original code
    class Foo {
      [foo] = 1;
      @bar [baz] = 2;
    }

    // Old output
    var _a, _b;
    var Foo = class {
      [_a] = 1;
      [_b] = 2;
    };
    _a = foo, _b = baz;
    __decorateClass([
      bar
    ], Foo.prototype, _b, 2);

    // New output
    var _a;
    var Foo = class {
      [foo] = 1;
      [_a = baz] = 2;
    };
    __decorateClass([
      bar
    ], Foo.prototype, _a, 2);
    ```

    The problem in this case is that normally TypeScript moves class field initializers into the special `constructor` method (automatically generating one if one doesn't already exist) so the side effects for class field property names must happen after the class body. But if class fields are supported by the target environment then the side effects must happen inline instead.

## 0.12.18

* Allow implicit `./` in CSS `@import` paths ([#1494](https://github.com/evanw/esbuild/pull/1494))

    In the browser, the paths inside CSS `@import` rules are implicitly relative to the path of the current CSS style sheet. Previously esbuild used node's JS path resolution rules in CSS as well, which required a `./` or `../` prefix for a path to be considered a relative path. Paths without that prefix are considered package paths and are searched for inside `node_modules` instead.

    With this release, esbuild will now first try to interpret the path as a relative path and then fall back to interpreting it as a package path if nothing exists at that relative path. This feature was originally added in version 0.7.18 but only worked for CSS `url()` tokens. In this release it now also works for `@import` rules.

    This feature was contributed by [@pd4d10](https://github.com/pd4d10).

* Fix lowering of nullish coalescing assignment edge case ([#1493](https://github.com/evanw/esbuild/issues/1493))

    This release fixes a bug where lowering of the `??=` nullish coalescing assignment operator failed when the target environment supported nullish coalescing and private class fields but not nullish coalescing assignment. An example target environment with this specific feature support matrix combination is node 14.8. This edge case is now lowered correctly:

    ```js
    // Original code
    class A {
      #a;
      f() {
        this.#a ??= 1;
      }
    }

    // Old output (with --target=node14.8)
    panic: Unexpected expression of type *js_ast.EPrivateIdentifier

    // New output (with --target=node14.8)
    class A {
      #a;
      f() {
        this.#a ?? (this.#a = 1);
      }
    }
    ```

* Fix public fields being inserted before `super()` call ([#1497](https://github.com/evanw/esbuild/issues/1497))

    The helper function that esbuild uses to emulate the new public class field syntax can potentially be inserted into the class constructor before the `super()` call. That is problematic because the helper function makes use of `this`, and `this` must only be used after the `super()` call. This release fixes a case where this happens when minification is enabled:

    ```js
    // Original code
    class A extends B {
      x;
      constructor() {
        f();
        super();
      }
    }

    // Old output (with --minify-syntax --target=es6)
    class A extends B {
      constructor() {
        __publicField(this, "x");
        f(), super();
      }
    }

    // New output (with --minify-syntax --target=es6)
    class A extends B {
      constructor() {
        f();
        super();
        __publicField(this, "x");
      }
    }
    ```

* Fix lowering of static private methods in class expressions ([#1498](https://github.com/evanw/esbuild/issues/1498))

    Previously static private methods were lowered incorrectly when present in class expressions. The class expression itself was missing in the output due to an oversight (variable shadowing). This issue has been fixed:

    ```js
    // Original code
    (class {
      static #x() {}
    });

    // Old output (with --target=es6)
    var _x, _a, x_fn;
    __privateAdd(_a, _x), _x = new WeakSet(), x_fn = function() {
    }, __privateAdd(_a, _x), _a;

    // New output (with --target=es6)
    var _x, _a, x_fn;
    _a = class {
    }, _x = new WeakSet(), x_fn = function() {
    }, __privateAdd(_a, _x), _a;
    ```

## 0.12.17

* Fix a bug with private fields and logical assignment operators ([#1418](https://github.com/evanw/esbuild/issues/1418))

    This release fixes a bug where code using private fields in combination with [logical assignment operators](https://github.com/tc39/proposal-logical-assignment) was transformed incorrectly if the target environment supported logical assignment operators but not private fields. Since logical assignment operators are assignment operators, the entire operator must be transformed even if the operator is supported. This should now work correctly:

    ```js
    // Original code
    class Foo {
      #x
      foo() {
        this.#x &&= 2
        this.#x ||= 2
        this.#x ??= 2
      }
    }

    // Old output
    var _x;
    class Foo {
      constructor() {
        __privateAdd(this, _x, void 0);
      }
      foo() {
        this._x &&= 2;
        this._x ||= 2;
        this._x ??= 2;
      }
    }
    _x = new WeakMap();

    // New output
    var _x, _a;
    class Foo {
      constructor() {
        __privateAdd(this, _x, void 0);
      }
      foo() {
        __privateGet(this, _x) && __privateSet(this, _x, 2);
        __privateGet(this, _x) || __privateSet(this, _x, 2);
        __privateGet(this, _x) ?? __privateSet(this, _x, 2);
      }
    }
    _x = new WeakMap();
    ```

* Fix a hoisting bug in the bundler ([#1455](https://github.com/evanw/esbuild/issues/1455))

    This release fixes a bug where variables declared using `var` inside of top-level `for` loop initializers were not hoisted inside lazily-initialized ES modules (such as those that are generated when bundling code that loads an ES module using `require`). This meant that hoisted function declarations incorrectly didn't have access to these loop variables:

    ```js
    // entry.js
    console.log(require('./esm-file').test())

    // esm-file.js
    for (var i = 0; i < 10; i++) ;
    export function test() { return i }
    ```

    Old output (incorrect):

    ```js
    // esm-file.js
    var esm_file_exports = {};
    __export(esm_file_exports, {
      test: () => test
    });
    function test() {
      return i;
    }
    var init_esm_file = __esm({
      "esm-file.js"() {
        for (var i = 0; i < 10; i++)
          ;
      }
    });

    // entry.js
    console.log((init_esm_file(), esm_file_exports).test());
    ```

    New output (correct):

    ```js
    // esm-file.js
    var esm_file_exports = {};
    __export(esm_file_exports, {
      test: () => test
    });
    function test() {
      return i;
    }
    var i;
    var init_esm_file = __esm({
      "esm-file.js"() {
        for (i = 0; i < 10; i++)
          ;
      }
    });

    // entry.js
    console.log((init_esm_file(), esm_file_exports).test());
    ```

* Fix a code generation bug for private methods ([#1424](https://github.com/evanw/esbuild/issues/1424))

    This release fixes a bug where when private methods are transformed and the target environment is one that supports private methods (such as `esnext`), the member function name was uninitialized and took on the zero value by default. This resulted in the member function name becoming `__create` instead of the correct name since that's the name of the symbol at index 0. Now esbuild always generates a private method symbol even when private methods are supported, so this is no longer an issue:

    ```js
    // Original code
    class Foo {
      #a() { return 'a' }
      #b() { return 'b' }
      static c
    }

    // Old output
    var _a, __create, _b, __create;
    var Foo = class {
      constructor() {
        __privateAdd(this, _a);
        __privateAdd(this, _b);
      }
    };
    _a = new WeakSet();
    __create = function() {
      return "a";
    };
    _b = new WeakSet();
    __create = function() {
      return "b";
    };
    __publicField(Foo, "c");

    // New output
    var _a, a_fn, _b, b_fn;
    var Foo = class {
      constructor() {
        __privateAdd(this, _a);
        __privateAdd(this, _b);
      }
    };
    _a = new WeakSet();
    a_fn = function() {
      return "a";
    };
    _b = new WeakSet();
    b_fn = function() {
      return "b";
    };
    __publicField(Foo, "c");
    ```

* The CLI now stops watch and serve mode when stdin is closed ([#1449](https://github.com/evanw/esbuild/pull/1449))

    To facilitate esbuild being called from the Erlang VM, esbuild's command-line interface will now exit when in `--watch` or `--serve` mode if stdin is closed. This change is necessary because the Erlang VM doesn't have an API for terminating a child process, so it instead closes stdin to indicate that the process is no longer needed.

    Note that this only happens when stdin is not a TTY (i.e. only when the CLI is being used non-interactively) to avoid disrupting the use case of manually moving esbuild to a background job using a Unix terminal.

    This change was contributed by [@josevalim](https://github.com/josevalim).

## 0.12.16

* Remove warning about bad CSS `@`-rules ([#1426](https://github.com/evanw/esbuild/issues/1426))

    The CSS bundler built in to esbuild is only designed with real CSS in mind. Running other languages that compile down to CSS through esbuild without compiling them down to CSS first can be a bad idea since esbuild applies browser-style error recovery to invalid syntax and uses browser-style import order that other languages might not be expecting. This is why esbuild previously generated warnings when it encountered unknown CSS `@`-rules.

    However, some people want to run other non-CSS languages through esbuild's CSS bundler anyway. So with this release, esbuild will no longer generate any warnings if you do this. But keep in mind that doing this is still potentially unsafe. Depending on the input language, using esbuild's CSS bundler to bundle non-CSS code can still potentially alter the semantics of your code.

* Allow `ES2021` in `tsconfig.json` ([#1470](https://github.com/evanw/esbuild/issues/1470))

    TypeScript recently [added support for `ES2021`](https://github.com/microsoft/TypeScript/pull/41239) in `tsconfig.json` so esbuild now supports this too. This has the same effect as if you passed `--target=es2021` to esbuild. Keep in mind that the value of `target` in `tsconfig.json` is only respected if you did not pass a `--target=` value to esbuild.

* Avoid using the `worker_threads` optimization in certain old node versions ([#1462](https://github.com/evanw/esbuild/issues/1462))

    The `worker_threads` optimization makes esbuild's synchronous API calls go much faster than they would otherwise. However, it turns out this optimization cannot be used in certain node versions older than `v12.17.0`, where node throws an error when trying to create the worker. This optimization is now disabled in these scenarios.

    Note that these old node versions are [currently in maintenance](https://nodejs.org/en/about/releases/). I recommend upgrading to a modern version of node if run-time performance is important to you.

* Paths starting with `node:` are implicitly external when bundling for node ([#1466](https://github.com/evanw/esbuild/issues/1466))

    This replicates a new node feature where you can [prefix an import path with `node:`](https://nodejs.org/api/esm.html#esm_node_imports) to load a native node module by that name (such as `import fs from "node:fs/promises"`). These paths also [have special behavior](https://nodejs.org/api/modules.html#modules_core_modules):

    > Core modules can also be identified using the `node:` prefix, in which case it bypasses the `require` cache. For instance, `require('node:http')` will always return the built in HTTP module, even if there is `require.cache` entry by that name.

    With this release, esbuild's built-in resolver will now automatically consider all import paths starting with `node:` as external. This new behavior is only active when the current platform is set to node such as with `--platform=node`. If you need to customize this behavior, you can write a plugin to intercept these paths and treat them differently.

* Consider `\` and `/` to be the same in file paths ([#1459](https://github.com/evanw/esbuild/issues/1459))

    On Windows, there are many different file paths that can refer to the same underlying file. Windows uses a case-insensitive file system so for example `foo.js` and `Foo.js` are the same file. When bundling, esbuild needs to treat both of these paths as the same to avoid incorrectly bundling the file twice. This is case is already handled by identifying files by their lower-case file path.

    The case that wasn't being handled is the fact that Windows supports two different path separators, `/` and `\`, both of which mean the same thing. For example `foo/bar.js` and `foo\bar.js` are the same file. With this release, this case is also handled by esbuild. Files that are imported in multiple places with inconsistent path separators will now be considered the same file instead of bundling the file multiple times.

## 0.12.15

* Fix a bug with `var()` in CSS color lowering ([#1421](https://github.com/evanw/esbuild/issues/1421))

    This release fixes a bug with esbuild's handling of the `rgb` and `hsl` color functions when they contain `var()`. Each `var()` token sequence can be substituted for any number of tokens including zero or more than one, but previously esbuild's output was only correct if each `var()` inside of `rgb` or `hsl` contained exactly one token. With this release, esbuild will now not attempt to transform newer CSS color syntax to older CSS color syntax if it contains `var()`:

    ```
    /* Original code */
    a {
      color: hsl(var(--hs), var(--l));
    }

    /* Old output */
    a {
      color: hsl(var(--hs), ,, var(--l));
    }

    /* New output */
    a {
      color: hsl(var(--hs), var(--l));
    }
    ```

    The bug with the old output above happened because esbuild considered the arguments to `hsl` as matching the pattern `hsl(h s l)` which is the new space-separated form allowed by [CSS Color Module Level 4](https://drafts.csswg.org/css-color/#the-hsl-notation). Then esbuild tried to convert this to the form `hsl(h, s, l)` which is more widely supported by older browsers. But this substitution doesn't work in the presence of `var()`, so it has now been disabled in that case.

## 0.12.14

* Fix the `file` loader with custom namespaces ([#1404](https://github.com/evanw/esbuild/issues/1404))

    This fixes a regression from version 0.12.12 where using a plugin to load an input file with the `file` loader in a custom namespace caused esbuild to write the contents of that input file to the path associated with that namespace instead of to a path inside of the output directory. With this release, the `file` loader should now always copy the file somewhere inside of the output directory.

## 0.12.13

* Fix using JS synchronous API from from non-main threads ([#1406](https://github.com/evanw/esbuild/issues/1406))

    This release fixes an issue with the new implementation of the synchronous JS API calls (`transformSync` and `buildSync`) when they are used from a thread other than the main thread. The problem happened because esbuild's new implementation uses node's `worker_threads` library internally and non-main threads were incorrectly assumed to be esbuild's internal thread instead of potentially another unrelated thread. Now esbuild's synchronous JS APIs should work correctly when called from non-main threads.

## 0.12.12

* Fix `file` loader import paths when subdirectories are present ([#1044](https://github.com/evanw/esbuild/issues/1044))

    Using the `file` loader for a file type causes importing affected files to copy the file into the output directory and to embed the path to the copied file into the code that imported it. However, esbuild previously always embedded the path relative to the output directory itself. This is problematic when the importing code is generated within a subdirectory inside the output directory, since then the relative path is wrong. For example:

    ```
    $ cat src/example/entry.css
    div {
      background: url(../images/image.png);
    }

    $ esbuild --bundle src/example/entry.css --outdir=out --outbase=src --loader:.png=file

    $ find out -type f
    out/example/entry.css
    out/image-55DNWN2R.png

    $ cat out/example/entry.css
    /* src/example/entry.css */
    div {
      background: url(./image-55DNWN2R.png);
    }
    ```

    This is output from the previous version of esbuild. The above asset reference in `out/example/entry.css` is wrong. The path should start with `../` because the two files are in different directories.

    With this release, the asset references present in output files will now be the full relative path from the output file to the asset, so imports should now work correctly when the entry point is in a subdirectory within the output directory. This change affects asset reference paths in both CSS and JS output files.

    Note that if you want asset reference paths to be independent of the subdirectory in which they reside, you can use the `--public-path` setting to provide the common path that all asset reference paths should be constructed relative to. Specifically `--public-path=.` should bring back the old problematic behavior in case you need it.

* Add support for `[dir]` in `--asset-names` ([#1196](https://github.com/evanw/esbuild/pull/1196))

    You can now use path templates such as `--asset-names=[dir]/[name]-[hash]` to copy the input directory structure of your asset files (i.e. input files loaded with the `file` loader) to the output directory. Here's an example:

    ```
    $ cat entry.css
    header {
      background: url(images/common/header.png);
    }
    main {
      background: url(images/home/hero.png);
    }

    $ esbuild --bundle entry.css --outdir=out --asset-names=[dir]/[name]-[hash] --loader:.png=file

    $ find out -type f
    out/images/home/hero-55DNWN2R.png
    out/images/common/header-55DNWN2R.png
    out/entry.css

    $ cat out/entry.css
    /* entry.css */
    header {
      background: url(./images/common/header-55DNWN2R.png);
    }
    main {
      background: url(./images/home/hero-55DNWN2R.png);
    }
    ```

## 0.12.11

* Enable faster synchronous transforms with the JS API by default ([#1000](https://github.com/evanw/esbuild/issues/1000))

    Currently the synchronous JavaScript API calls `transformSync` and `buildSync` spawn a new child process on every call. This is due to limitations with node's `child_process` API. Doing this means `transformSync` and `buildSync` are much slower than `transform` and `build`, which share the same child process across calls.

    This release improves the performance of `transformSync` and `buildSync` by up to 20x. It enables a hack where node's `worker_threads` API and atomics are used to block the main thread while asynchronous communication with a single long-lived child process happens in a worker. Previously this was only enabled when the `ESBUILD_WORKER_THREADS` environment variable was set to `1`. But this experiment has been available for a while (since version 0.9.6) without any reported issues. Now this hack will be enabled by default. It can be disabled by setting `ESBUILD_WORKER_THREADS` to `0` before running node.

* Fix nested output directories with WebAssembly on Windows ([#1399](https://github.com/evanw/esbuild/issues/1399))

    Many functions in Go's standard library have a bug where they do not work on Windows when using Go with WebAssembly. This is a long-standing bug and is a fault with the design of the standard library, so it's unlikely to be fixed. Basically Go's standard library is designed to bake "Windows or not" decision into the compiled executable, but WebAssembly is platform-independent which makes "Windows or not" is a run-time decision instead of a compile-time decision. Oops.

    I have been working around this by trying to avoid using path-related functions in the Go standard library and doing all path manipulation by myself instead. This involved completely replacing Go's `path/filepath` library. However, I missed the `os.MkdirAll` function which is also does path manipulation but is outside of the `path/filepath` package. This meant that nested output directories failed to be created on Windows, which caused a build error. This problem only affected the `esbuild-wasm` package.

    This release manually reimplements nested output directory creation to work around this bug in the Go standard library. So nested output directories should now work on Windows with the `esbuild-wasm` package.

## 0.12.10

* Add a target for ES2021

    It's now possible to use `--target=es2021` to target the newly-released JavaScript version ES2021. The only difference between that and `--target=es2020` is that logical assignment operators such as `a ||= b` are not converted to regular assignment operators such as `a || (a = b)`.

* Minify the syntax `Infinity` to `1 / 0` ([#1385](https://github.com/evanw/esbuild/pull/1385))

    The `--minify-syntax` flag (automatically enabled by `--minify`) will now minify the expression   `Infinity` to `1 / 0`, which uses fewer bytes:

    ```js
    // Original code
    const a = Infinity;

    // Output with "--minify-syntax"
    const a = 1 / 0;
    ```

    This change was contributed by [@Gusted](https://github.com/Gusted).

* Minify syntax in the CSS `transform` property ([#1390](https://github.com/evanw/esbuild/pull/1390))

    This release includes various size reductions for CSS transform matrix syntax when minification is enabled:

    ```css
    /* Original code */
    div {
      transform: translate3d(0, 0, 10px) scale3d(200%, 200%, 1) rotate3d(0, 0, 1, 45deg);
    }

    /* Output with "--minify-syntax" */
    div {
      transform: translateZ(10px) scale(2) rotate(45deg);
    }
    ```

    The `translate3d` to `translateZ` conversion was contributed by [@steambap](https://github.com/steambap).

* Support for the case-sensitive flag in CSS attribute selectors ([#1397](https://github.com/evanw/esbuild/issues/1397))

    You can now use the case-sensitive CSS attribute selector flag `s` such as in `[type="a" s] { list-style: lower-alpha; }`. Previously doing this caused a warning about unrecognized syntax.

## 0.12.9

* Allow `this` with `--define` ([#1361](https://github.com/evanw/esbuild/issues/1361))

    You can now override the default value of top-level `this` with the `--define` feature. Top-level `this` defaults to being `undefined` in ECMAScript modules and `exports` in CommonJS modules. For example:

    ```js
    // Original code
    ((obj) => {
      ...
    })(this);

    // Output with "--define:this=window"
    ((obj) => {
      ...
    })(window);
    ```

    Note that overriding what top-level `this` is will likely break code that uses it correctly. So this new feature is only useful in certain cases.

* Fix CSS minification issue with `!important` and duplicate declarations ([#1372](https://github.com/evanw/esbuild/issues/1372))

    Previously CSS with duplicate declarations for the same property where the first one was marked with `!important` was sometimes minified incorrectly. For example:

    ```css
    .selector {
      padding: 10px !important;
      padding: 0;
    }
    ```

    This was incorrectly minified as `.selector{padding:0}`. The bug affected three properties: `padding`, `margin`, and `border-radius`. With this release, this code will now be minified as `.selector{padding:10px!important;padding:0}` instead which means there is no longer a difference between minified and non-minified code in this case.

## 0.12.8

* Plugins can now specify `sideEffects: false` ([#1009](https://github.com/evanw/esbuild/issues/1009))

    The default path resolution behavior in esbuild determines if a given file can be considered side-effect free (in the [Webpack-specific sense](https://webpack.js.org/guides/tree-shaking/#mark-the-file-as-side-effect-free)) by reading the contents of the nearest enclosing `package.json` file and looking for `"sideEffects": false`. However, up until now this was impossible to achieve in an esbuild plugin because there was no way of returning this metadata back to esbuild.

    With this release, esbuild plugins can now return `sideEffects: false` to mark a file as having no side effects. Here's an example:

    ```js
    esbuild.build({
      entryPoints: ['app.js'],
      bundle: true,
      plugins: [{
        name: 'env-plugin',
        setup(build) {
          build.onResolve({ filter: /^env$/ }, args => ({
            path: args.path,
            namespace: 'some-ns',
            sideEffects: false,
          }))
          build.onLoad({ filter: /.*/, namespace: 'some-ns' }, () => ({
            contents: `export default self.env || (self.env = getEnv())`,
          }))
        },
      }],
    })
    ```

    This plugin creates a virtual module that can be generated by importing the string `env`. However, since the plugin returns `sideEffects: false`, the generated virtual module will not be included in the bundle if all of the imported values from the module `env` end up being unused.

    This feature was contributed by [@chriscasola](https://github.com/chriscasola).

* Remove a warning about unsupported source map comments ([#1358](https://github.com/evanw/esbuild/issues/1358))

    This removes a warning that indicated when a source map comment couldn't be supported. Specifically, this happens when you enable source map generation and esbuild encounters a file with a source map comment pointing to an external file but doesn't have enough information to know where to look for that external file (basically when the source file doesn't have an associated directory to use for path resolution). In this case esbuild can't respect the input source map because it cannot be located. The warning was annoying so it has been removed. Source maps still won't work, however.

## 0.12.7

* Quote object properties that are modern Unicode identifiers ([#1349](https://github.com/evanw/esbuild/issues/1349))

    In ES6 and above, an identifier is a character sequence starting with a character in the `ID_Start` Unicode category and followed by zero or more characters in the `ID_Continue` Unicode category, and these categories must be drawn from Unicode version 5.1 or above.

    But in ES5, an identifier is a character sequence starting with a character in one of the `Lu, Ll, Lt, Lm, Lo, Nl` Unicode categories and followed by zero or more characters in the `Lu, Ll, Lt, Lm, Lo, Nl, Mn, Mc, Nd, Pc` Unicode categories, and these categories must be drawn from Unicode version 3.0 or above.

    Previously esbuild always used the ES6+ identifier validation test when deciding whether to use an identifier or a quoted string to encode an object property but with this release, it will use the ES5 validation test instead:

    ```js
    // Original code
    x.ꓷꓶꓲꓵꓭꓢꓱ = { ꓷꓶꓲꓵꓭꓢꓱ: y };

    // Old output
    x.ꓷꓶꓲꓵꓭꓢꓱ = { ꓷꓶꓲꓵꓭꓢꓱ: y };

    // New output
    x["ꓷꓶꓲꓵꓭꓢꓱ"] = { "ꓷꓶꓲꓵꓭꓢꓱ": y };
    ```

    This approach should ensure maximum compatibility with all JavaScript environments that support ES5 and above. Note that this means minified files containing Unicode properties may be slightly larger than before.

* Ignore `tsconfig.json` files inside `node_modules` ([#1355](https://github.com/evanw/esbuild/issues/1355))

    Package authors often publish their `tsconfig.json` files to npm because of npm's default-include publishing model and because these authors probably don't know about `.npmignore` files. People trying to use these packages with esbuild have historically complained that esbuild is respecting `tsconfig.json` in these cases. The assumption is that the package author published these files by accident.

    With this release, esbuild will no longer respect `tsconfig.json` files when the source file is inside a `node_modules` folder. Note that `tsconfig.json` files inside `node_modules` are still parsed, and extending `tsconfig.json` files from inside a package is still supported.

* Fix missing `--metafile` when using `--watch` ([#1357](https://github.com/evanw/esbuild/issues/1357))

    Due to an oversight, the `--metafile` setting didn't work when `--watch` was also specified. This only affected the command-line interface. With this release, the `--metafile` setting should now work in this case.

* Add a hidden `__esModule` property to modules in ESM format ([#1338](https://github.com/evanw/esbuild/pull/1338))

    Module namespace objects from ESM files will now have a hidden `__esModule` property. This improves compatibility with code that has been converted from ESM syntax to CommonJS by Babel or TypeScript. For example:

    ```js
    // Input TypeScript code
    import x from "y"
    console.log(x)

    // Output JavaScript code from the TypeScript compiler
    var __importDefault = (this && this.__importDefault) || function (mod) {
        return (mod && mod.__esModule) ? mod : { "default": mod };
    };
    Object.defineProperty(exports, "__esModule", { value: true });
    const y_1 = __importDefault(require("y"));
    console.log(y_1.default);
    ```

    If the object returned by `require("y")` doesn't have an `__esModule` property, then `y_1` will be the object `{ "default": require("y") }`. If the file `"y"` is in ESM format and has a default export of, say, the value `null`, that means `y_1` will now be `{ "default": { "default": null } }` and you will need to use `y_1.default.default` to access the default value. Adding an automatically-generated `__esModule` property when converting files in ESM format to CommonJS is required to make this code work correctly (i.e. for the value to be accessible via just `y_1.default` instead).

    With this release, code in ESM format will now have an automatically-generated `__esModule` property to satisfy this convention. The property is non-enumerable so it shouldn't show up when iterating over the properties of the object. As a result, the export name `__esModule` is now reserved for use with esbuild. It's now an error to create an export with the name `__esModule`.

    This fix was contributed by [@lbwa](https://github.com/lbwa).

## 0.12.6

* Improve template literal lowering transformation conformance ([#1327](https://github.com/evanw/esbuild/issues/1327))

    This release contains the following improvements to template literal lowering for environments that don't support tagged template literals natively (such as `--target=es5`):

    * For tagged template literals, the arrays of strings that are passed to the tag function are now frozen and immutable. They are also now cached so they should now compare identical between multiple template evaluations:

        ```js
        // Original code
        console.log(tag`\u{10000}`)

        // Old output
        console.log(tag(__template(["𐀀"], ["\\u{10000}"])));

        // New output
        var _a;
        console.log(tag(_a || (_a = __template(["𐀀"], ["\\u{10000}"]))));
        ```

    * For tagged template literals, the generated code size is now smaller in the common case where there are no escape sequences, since in that case there is no distinction between "raw" and "cooked" values:

        ```js
        // Original code
        console.log(tag`some text without escape sequences`)

        // Old output
        console.log(tag(__template(["some text without escape sequences"], ["some text without escape sequences"])));

        // New output
        var _a;
        console.log(tag(_a || (_a = __template(["some text without escape sequences"]))));
        ```

    * For non-tagged template literals, the generated code now uses chains of `.concat()` calls instead of string addition:

        ```js
        // Original code
        console.log(`an ${example} template ${literal}`)

        // Old output
        console.log("an " + example + " template " + literal);

        // New output
        console.log("an ".concat(example, " template ").concat(literal));
        ```

        The old output was incorrect for several reasons including that `toString` must be called instead of `valueOf` for objects and that passing a `Symbol` instance should throw instead of converting the symbol to a string. Using `.concat()` instead of string addition fixes both of those correctness issues. And you can't use a single `.concat()` call because side effects must happen inline instead of at the end.

* Only respect `target` in `tsconfig.json` when esbuild's target is not configured ([#1332](https://github.com/evanw/esbuild/issues/1332))

    In version 0.12.4, esbuild began respecting the `target` setting in `tsconfig.json`. However, sometimes `tsconfig.json` contains target values that should not be used. With this release, esbuild will now only use the `target` value in `tsconfig.json` as the language level when esbuild's `target` setting is not configured. If esbuild's `target` setting is configured then the `target` value in `tsconfig.json` is now ignored.

* Fix the order of CSS imported from JS ([#1342](https://github.com/evanw/esbuild/pull/1342))

    Importing CSS from JS when bundling causes esbuild to generate a sibling CSS output file next to the resulting JS output file containing the bundled CSS. The order of the imported CSS files in the output was accidentally the inverse order of the order in which the JS files were evaluated. Instead the order of the imported CSS files should match the order in which the JS files were evaluated. This fix was contributed by [@dmitrage](https://github.com/dmitrage).

* Fix an edge case with transforming `export default class` ([#1346](https://github.com/evanw/esbuild/issues/1346))

    Statements of the form `export default class x {}` were incorrectly transformed to `class x {} var y = x; export {y as default}` instead of `class x {} export {x as default}`. Transforming these statements like this is incorrect in the rare case that the class is later reassigned by name within the same file such as `export default class x {} x = null`. Here the imported value should be `null` but was incorrectly the class object instead. This is unlikely to matter in real-world code but it has still been fixed to improve correctness.

## 0.12.5

* Add support for lowering tagged template literals to ES5 ([#297](https://github.com/evanw/esbuild/issues/297))

    This release adds support for lowering tagged template literals such as `` String.raw`\unicode` `` to target environments that don't support them such as `--target=es5` (non-tagged template literals were already supported). Each literal turns into a function call to a helper function:

    ```js
    // Original code
    console.log(String.raw`\unicode`)

    // Lowered code
    console.log(String.raw(__template([void 0], ["\\unicode"])));
    ```

* Change class field behavior to match TypeScript 4.3

    TypeScript 4.3 includes a subtle breaking change that wasn't mentioned in the [TypeScript 4.3 blog post](https://devblogs.microsoft.com/typescript/announcing-typescript-4-3/): class fields will now be compiled with different semantics if `"target": "ESNext"` is present in `tsconfig.json`. Specifically in this case `useDefineForClassFields` will default to `true` when not specified instead of `false`. This means class field behavior in TypeScript code will now match JavaScript instead of doing something else:

    ```js
    class Base {
      set foo(value) { console.log('set', value) }
    }
    class Derived extends Base {
      foo = 123
    }
    new Derived()
    ```

    In TypeScript 4.2 and below, the TypeScript compiler would generate code that prints `set 123` when `tsconfig.json` contains `"target": "ESNext"` but in TypeScript 4.3, the TypeScript compiler will now generate code that doesn't print anything. This is the difference between "assign" semantics and "define" semantics. With this release, esbuild has been changed to follow the TypeScript 4.3 behavior.

* Avoid generating the character sequence `</script>` ([#1322](https://github.com/evanw/esbuild/issues/1322))

    If the output of esbuild is inlined into a `<script>...</script>` tag inside an HTML file, the character sequence `</script>` inside the JavaScript code will accidentally cause the script tag to be terminated early. There are at least four such cases where this can happen:

    ```js
    console.log('</script>')
    console.log(1</script>/.exec(x).length)
    console.log(String.raw`</script>`)
    // @license </script>
    ```

    With this release, esbuild will now handle all of these cases and avoid generating the problematic character sequence:

    ```js
    console.log('<\/script>');
    console.log(1< /script>/.exec(x).length);
    console.log(String.raw(__template(["<\/script>"], ["<\/script>"])));
    // @license <\/script>
    ```

* Change the triple-slash reference comment for Deno ([#1325](https://github.com/evanw/esbuild/issues/1325))

    The comment in esbuild's JavaScript API implementation for Deno that references the TypeScript type declarations has been changed from `/// <reference path="./mod.d.ts" />` to `/// <reference types="./mod.d.ts" />`. This comment was copied from Deno's documentation but apparently Deno's documentation was incorrect. The comment in esbuild's Deno bundle has been changed to reflect Deno's latest documentation.

## 0.12.4

* Reorder name preservation before TypeScript decorator evaluation ([#1316](https://github.com/evanw/esbuild/issues/1316))

    The `--keep-names` option ensures the `.name` property on functions and classes remains the same after bundling. However, this was being enforced after TypeScript decorator evaluation which meant that the decorator could observe the incorrect name. This has been fixed and now `.name` preservation happens before decorator evaluation instead.

* Potential fix for a determinism issue ([#1304](https://github.com/evanw/esbuild/issues/1304))

    This release contains a potential fix for an unverified issue with non-determinism in esbuild. The regression was apparently introduced in 0.11.13 and may be related to parallelism that was introduced around the point where dynamic `import()` expressions are added to the list of entry points. Hopefully this fix should resolve the regression.

* Respect `target` in `tsconfig.json` ([#277](https://github.com/evanw/esbuild/issues/277))

    Each JavaScript file that esbuild bundles will now be transformed according to the [`target`](https://www.typescriptlang.org/tsconfig#target) language level from the nearest enclosing `tsconfig.json` file. This is in addition to esbuild's own `--target` setting; the two settings are merged by transforming any JavaScript language feature that is unsupported in either esbuild's configured `--target` value or the `target` property in the `tsconfig.json` file.

## 0.12.3

* Ensure JSX element names start with a capital letter ([#1309](https://github.com/evanw/esbuild/issues/1309))

    The JSX specification only describes the syntax and says nothing about how to interpret it. But React (and therefore esbuild) treats JSX tags that start with a lower-case ASCII character as strings instead of identifiers. That way the tag `<i/>` always refers to the italic HTML element `i` and never to a local variable named `i`.

    However, esbuild may rename identifiers for any number of reasons such as when minification is enabled. Previously esbuild could sometimes rename identifiers used as tag names such that they start with a lower-case ASCII character. This is problematic when JSX syntax preservation is enabled since subsequent JSX processing would then turn these identifier references into strings.

    With this release, esbuild will now make sure identifiers used in tag names start with an upper-case ASCII character instead when JSX syntax preservation is enabled. This should avoid problems when using esbuild with JSX transformation tools.

* Fix a single hyphen being treated as a CSS name ([#1310](https://github.com/evanw/esbuild/pull/1310))

    CSS identifiers are allowed to start with a `-` character if (approximately) the following character is a letter, an escape sequence, a non-ASCII character, the character `_`, or another `-` character. This check is used in certain places when printing CSS to determine whether a token is a valid identifier and can be printed as such or whether it's an invalid identifier and needs to be quoted as a string. One such place is in attribute selectors such as `[a*=b]`.

    However, esbuild had a bug where a single `-` character was incorrectly treated as a valid identifier in this case. This is because the end of string became U+FFFD (the Unicode replacement character) which is a non-ASCII character and a valid name-start code point. With this release a single `-` character is no longer treated as a valid identifier. This fix was contributed by [@lbwa](https://github.com/lbwa).

## 0.12.2

* Fix various code generation and minification issues ([#1305](https://github.com/evanw/esbuild/issues/1305))

    This release fixes the following issues, which were all identified by running esbuild against the latest UglifyJS test suite:

    * The `in` operator is now surrounded parentheses inside arrow function expression bodies inside `for` loop initializers:

        ```js
        // Original code
        for ((x => y in z); 0; ) ;

        // Old output
        for ((x) => y in z; 0; ) ;

        // New output
        for ((x) => (y in z); 0; ) ;
        ```

        Without this, the `in` operator would cause the for loop to be considered a for-in loop instead.

    * The statement `return undefined;` is no longer minified to `return;` inside async generator functions:

        ```js
        // Original code
        return undefined;

        // Old output
        return;

        // New output
        return void 0;
        ```

        Using `return undefined;` inside an async generator function has the same effect as `return await undefined;` which schedules a task in the event loop and runs code in a different order than just `return;`, which doesn't hide an implicit `await` expression.

    * Property access expressions are no longer inlined in template tag position:

        ```js
        // Original code
        (null, a.b)``, (null, a[b])``;

        // Old output
        a.b``, a[b]``;

        // New output
        (0, a.b)``, (0, a[b])``;
        ```

        The expression `` a.b`c` `` is different than the expression `` (0, a.b)`c` ``. The first calls the function `a.b` with `a` as the value for `this` but the second calls the function `a.b` with the default value for `this` (the global object in non-strict mode or `undefined` in strict mode).

    * Verbatim `__proto__` properties inside object spread are no longer inlined when minifying:

        ```js
        // Original code
        x = { ...{ __proto__: { y: true } } }.y;

        // Old output
        x = { __proto__: { y: !0 } }.y;

        // New output
        x = { ...{ __proto__: { y: !0 } } }.y;
        ```

        A verbatim (i.e. non-computed non-method) property called `__proto__` inside an object literal actually sets the prototype of the surrounding object literal. It does not add an "own property" called `__proto__` to that object literal, so inlining it into the parent object literal would be incorrect. The presence of a `__proto__` property now stops esbuild from applying the object spread inlining optimization when minifying.

    * The value of `this` has now been fixed for lowered private class members that are used as template tags:

        ```js
        // Original code
        x = (new (class {
          a = this.#c``;
          b = 1;
          #c() { return this }
        })).a.b;

        // Old output
        var _c, c_fn, _a;
        x = new (_a = class {
          constructor() {
            __privateAdd(this, _c);
            __publicField(this, "a", __privateMethod(this, _c, c_fn)``);
            __publicField(this, "b", 1);
          }
        }, _c = new WeakSet(), c_fn = function() {
          return this;
        }, _a)().a.b;

        // New output
        var _c, c_fn, _a;
        x = new (_a = class {
          constructor() {
            __privateAdd(this, _c);
            __publicField(this, "a", __privateMethod(this, _c, c_fn).bind(this)``);
            __publicField(this, "b", 1);
          }
        }, _c = new WeakSet(), c_fn = function() {
          return this;
        }, _a)().a.b;
        ```

        The value of `this` here should be an instance of the class because the template tag is a property access expression. However, it was previously the default value (the global object in non-strict mode or `undefined` in strict mode) instead due to the private member transformation, which is incorrect.

    * Invalid escape sequences are now allowed in tagged template literals

        This implements the template literal revision feature: https://github.com/tc39/proposal-template-literal-revision. It allows you to process tagged template literals using custom semantics that don't follow JavaScript escape sequence rules without causing a syntax error:

        ```js
        console.log((x => x.raw)`invalid \unicode escape sequence`)
        ```

## 0.12.1

* Add the ability to preserve JSX syntax ([#735](https://github.com/evanw/esbuild/issues/735))

    You can now pass `--jsx=preserve` to esbuild to prevent JSX from being transformed into JS. Instead, JSX syntax in all input files is preserved throughout the pipeline and is printed as JSX syntax in the generated output files. Note that this means the output files are no longer valid JavaScript code if you enable this setting. This feature is intended to be used when you want to transform the JSX syntax in esbuild's output files by another tool after bundling, usually one with a different JSX-to-JS transform than the one esbuild implements.

* Update the list of built-in node modules ([#1294](https://github.com/evanw/esbuild/issues/1294))

    The list of built-in modules that come with node was outdated, so it has been updated. It now includes new modules such as `wasi` and `_http_common`. Modules in this list are automatically marked as external when esbuild's platform is configured to `node`.

## 0.12.0

**This release contains backwards-incompatible changes.** Since esbuild is before version 1.0.0, these changes have been released as a new minor version to reflect this (as [recommended by npm](https://docs.npmjs.com/cli/v6/using-npm/semver/)). You should either be pinning the exact version of `esbuild` in your `package.json` file or be using a version range syntax that only accepts patch upgrades such as `~0.11.0`. See the documentation about [semver](https://docs.npmjs.com/cli/v6/using-npm/semver/) for more information.

The breaking changes in this release relate to CSS import order and also build scenarios where both the `inject` and `define` API options are used (see below for details). These breaking changes are as follows:

* Fix bundled CSS import order ([#465](https://github.com/evanw/esbuild/issues/465))

    JS and CSS use different import ordering algorithms. In JS, importing a file that has already been imported is a no-op but in CSS, importing a file that has already been imported re-imports the file. A simple way to imagine this is to view each `@import` rule in CSS as being replaced by the contents of that file similar to `#include` in C/C++. However, this is incorrect in the case of `@import` cycles because it would cause infinite expansion. A more accurate way to imagine this is that in CSS, a file is evaluated at the *last* `@import` location while in JS, a file is evaluated at the *first* `import` location.

    Previously esbuild followed JS import order rules for CSS but now esbuild will follow CSS import order rules. This is a breaking change because it means your CSS may behave differently when bundled. Note that CSS import order rules are somewhat unintuitive because evaluation order matters. In CSS, using `@import` multiple times can end up unintentionally erasing overriding styles. For example, consider the following files:

    ```css
    /* entry.css */
    @import "./color.css";
    @import "./background.css";
    ```

    ```css
    /* color.css */
    @import "./reset.css";
    body {
      color: white;
    }
    ```

    ```css
    /* background.css */
    @import "./reset.css";
    body {
      background: black;
    }
    ```

    ```css
    /* reset.css */
    body {
      background: white;
      color: black;
    }
    ```

    Because of how CSS import order works, `entry.css` will now be bundled like this:

    ```css
    /* color.css */
    body {
      color: white;
    }

    /* reset.css */
    body {
      background: white;
      color: black;
    }

    /* background.css */
    body {
      background: black;
    }
    ```

    This means the body will unintuitively be all black! The file `reset.css` is evaluated at the location of the *last* `@import` instead of the *first* `@import`. The fix for this case is to remove the nested imports of `reset.css` and to import `reset.css` exactly once at the top of `entry.css`.

    Note that while the evaluation order of external CSS imports is preserved with respect to other external CSS imports, the evaluation order of external CSS imports is *not* preserved with respect to other internal CSS imports. All external CSS imports are "hoisted" to the top of the bundle. The alternative would be to generate many smaller chunks which is usually undesirable. So in this case esbuild's CSS bundling behavior will not match the browser.

* Fix bundled CSS when using JS code splitting ([#608](https://github.com/evanw/esbuild/issues/608))

    Previously esbuild generated incorrect CSS output when JS code splitting was enabled and the JS code being bundled imported CSS files. CSS code that was reachable via multiple JS entry points was split off into a shared CSS chunk, but that chunk was not actually imported anywhere so the shared CSS was missing. This happened because both CSS and JS code splitting were experimental features that are still in progress and weren't tested together.

    Now esbuild's CSS output should contain all reachable CSS code when JS code splitting is enabled. Note that this does *not* mean code splitting works for CSS files. Each CSS output file simply contains the transitive set of all CSS reachable from the JS entry point including through dynamic `import()` and `require()` expressions. Specifically, the bundler constructs a virtual CSS file for each JS entry point consisting only of `@import` rules for each CSS file imported into a JS file. These `@import` rules are constructed in JS source order, but then the bundler uses CSS import order from that point forward to bundle this virtual CSS file into the final CSS output file.

    This model makes the most sense when CSS files are imported into JS files via JS `import` statements. Importing CSS via `import()` and `require()` (either directly or transitively through multiple intermediate JS files) should still "work" in the sense that all reachable CSS should be included in the output, but in this case esbuild will pick an arbitrary (but consistent) import order. The import order may not match the order that the JS files are evaluated in because JS evaluation order of dynamic imports is only determined at run-time while CSS bundling happens at compile-time.

    It's possible to implement code splitting for CSS such that CSS code used between multiple entry points is shared. However, CSS lacks a mechanism for "lazily" importing code (i.e. disconnecting the import location with the evaluation location) so CSS code splitting could potentially need to generate a huge number of very small chunks to preserve import order. It's unclear if this would end up being a net win or not as far as browser download time. So sharing-based code splitting is currently not supported for CSS.

    It's theoretically possible to implement code splitting for CSS such that CSS from a dynamically-imported JS file (e.g. via `import()`) is placed into a separate chunk. However, due to how `@import` order works this would in theory end up re-evaluating all shared dependencies which could overwrite overloaded styles and unintentionally change the way the page is rendered. For example, constructing a single-page app architecture such that each page is JS-driven and can transition to other JS-driven pages via `import()` could end up with pages that look different depending on what order you visit them in. This is clearly undesirable. The simple way to address this is to just not support dynamic-import code splitting for CSS either.

* Change "define" to have higher priority than "inject" ([#660](https://github.com/evanw/esbuild/issues/660))

    The "define" and "inject" features are both ways of replacing certain expressions in your source code with other things expressions. Previously esbuild's behavior ran "inject" before "define", which could lead to some undesirable behavior. For example (from the `react` npm package):

    ```js
    if (process.env.NODE_ENV === 'production') {
      module.exports = require('./cjs/react.production.min.js');
    } else {
      module.exports = require('./cjs/react.development.js');
    }
    ```

    If you use "define" to replace `process.env.NODE_ENV` with `"production"` and "inject" to replace `process` with a shim that emulates node's process API, then `process` was previously replaced first and then `process.env.NODE_ENV` wasn't matched because `process` referred to the injected shim. This wasn't ideal because it means esbuild didn't detect the branch condition as a constant (since it doesn't know how the shim behaves at run-time) and bundled both the development and production versions of the package.

    With this release, esbuild will now run "define" before "inject". In the above example this means that `process.env.NODE_ENV` will now be replaced with `"production"`, the injected shim will not be included, and only the production version of the package will be bundled. This feature was contributed by [@rtsao](https://github.com/rtsao).

In addition to the breaking changes above, the following features are also included in this release:

* Add support for the `NO_COLOR` environment variable

    The CLI will now omit color if the `NO_COLOR` environment variable is present, which is an existing convention that is followed by some other software. See https://no-color.org/ for more information.

## 0.11.23

* Add a shim function for unbundled uses of `require` ([#1202](https://github.com/evanw/esbuild/issues/1202))

    Modules in CommonJS format automatically get three variables injected into their scope: `module`, `exports`, and `require`. These allow the code to import other modules and to export things from itself. The bundler automatically rewrites uses of `module` and `exports` to refer to the module's exports and certain uses of `require` to a helper function that loads the imported module.

    Not all uses of `require` can be converted though, and un-converted uses of `require` will end up in the output. This is problematic because `require` is only present at run-time if the output is run as a CommonJS module. Otherwise `require` is undefined, which means esbuild's behavior is inconsistent between compile-time and run-time. The `module` and `exports` variables are objects at compile-time and run-time but `require` is a function at compile-time and undefined at run-time. This causes code that checks for `typeof require` to have inconsistent behavior:

    ```js
    if (typeof require === 'function' && typeof exports === 'object' && typeof module === 'object') {
      console.log('CommonJS detected')
    }
    ```

    In the above example, ideally `CommonJS detected` would always be printed since the code is being bundled with a CommonJS-aware bundler. To fix this, esbuild will now substitute references to `require` with a stub `__require` function when bundling if the output format is something other than CommonJS. This should ensure that `require` is now consistent between compile-time and run-time. When bundled, code that uses unbundled references to `require` will now look something like this:

    ```js
    var __require = (x) => {
      if (typeof require !== "undefined")
        return require(x);
      throw new Error('Dynamic require of "' + x + '" is not supported');
    };

    var __commonJS = (cb, mod) => () => (mod || cb((mod = {exports: {}}).exports, mod), mod.exports);

    var require_example = __commonJS((exports, module) => {
      if (typeof __require === "function" && typeof exports === "object" && typeof module === "object") {
        console.log("CommonJS detected");
      }
    });

    require_example();
    ```

* Fix incorrect caching of internal helper function library ([#1292](https://github.com/evanw/esbuild/issues/1292))

    This release fixes a bug where running esbuild multiple times with different configurations sometimes resulted in code that would crash at run-time. The bug was introduced in version 0.11.19 and happened because esbuild's internal helper function library is parsed once and cached per configuration, but the new profiler name option was accidentally not included in the cache key. This option is now included in the cache key so this bug should now be fixed.

* Minor performance improvements

    This release contains some small performance improvements to offset an earlier minor performance regression due to the addition of certain features such as hashing for entry point files. The benchmark times on the esbuild website should now be accurate again (versions of esbuild after the regression but before this release were slightly slower than the benchmark).

## 0.11.22

* Add support for the "import assertions" proposal

    This is new JavaScript syntax that was shipped in Chrome 91. It looks like this:

    ```js
    import './foo.json' assert { type: 'json' }
    import('./bar.json', { assert: { type: 'json' } })
    ```

    On the web, the content type for a given URL is determined by the `Content-Type` HTTP header instead of the file extension. So adding support for importing non-JS content types such as JSON to the web could cause [security issues](https://github.com/WICG/webcomponents/issues/839) since importing JSON from an untrusted source is safe while importing JS from an untrusted source is not.

    Import assertions are a new feature to address this security concern and unblock non-JS content types on the web. They cause the import to fail if the `Content-Type` header doesn't match the expected value. This prevents security issues for data-oriented content types such as JSON since it guarantees that data-oriented content will never accidentally be evaluated as code instead of data. More information about the proposal is available here: https://github.com/tc39/proposal-import-assertions.

    This release includes support for parsing and printing import assertions. They will be printed if the configured target environment supports them (currently only in `esnext` and `chrome91`), otherwise they will be omitted. If they aren't supported in the configured target environment and it's not possible to omit them, which is the case for certain dynamic `import()` expressions, then using them is a syntax error. Import assertions are otherwise unused by the bundler.

* Forbid the token sequence `for ( async of` when not followed by `=>`

    This follows a recently-fixed ambiguity in the JavaScript specification, which you can read about here: https://github.com/tc39/ecma262/pull/2256. Prior to this change in the specification, it was ambiguous whether this token sequence should be parsed as `for ( async of =>` or `for ( async of ;`. V8 and esbuild expected `=>` after `for ( async of` while SpiderMonkey and JavaScriptCore did something else.

    The ambiguity has been removed and the token sequence `for ( async of` is now forbidden by the specification when not followed by `=>`, so esbuild now forbids this as well. Note that the token sequence `for await (async of` is still allowed even when not followed by `=>`. Code such as `for ((async) of []) ;` is still allowed and will now be printed with parentheses to avoid the grammar ambiguity.

* Restrict `super` property access to inside of methods

    You can now only use `super.x` and `super[x]` expressions inside of methods. Previously these expressions were incorrectly allowed everywhere. This means esbuild now follows the JavaScript language specification more closely.

## 0.11.21

* TypeScript `override` for parameter properties ([#1262](https://github.com/evanw/esbuild/pull/1262))

    You can now use the `override` keyword instead of or in addition to the `public`, `private`, `protected`, and `readonly` keywords for declaring a TypeScript parameter property:

    ```ts
    class Derived extends Base {
      constructor(override field: any) {
      }
    }
    ```

    This feature was [recently added to the TypeScript compiler](https://github.com/microsoft/TypeScript/pull/43831) and will presumably be in an upcoming version of the TypeScript language. Support for this feature in esbuild was contributed by [@g-plane](https://github.com/g-plane).

* Fix duplicate export errors due to TypeScript import-equals statements ([#1283](https://github.com/evanw/esbuild/issues/1283))

    TypeScript has a special import-equals statement that is not part of JavaScript. It looks like this:

    ```ts
    import a = foo.a
    import b = a.b
    import c = b.c

    import x = foo.x
    import y = x.y
    import z = y.z

    export let bar = c
    ```

    Each import can be a type or a value and type-only imports need to be eliminated when converting this code to JavaScript, since types do not exist at run-time. The TypeScript compiler generates the following JavaScript code for this example:

    ```js
    var a = foo.a;
    var b = a.b;
    var c = b.c;
    export let bar = c;
    ```

    The `x`, `y`, and `z` import statements are eliminated in esbuild by iterating over imports and exports multiple times and continuing to remove unused TypeScript import-equals statements until none are left. The first pass removes `z` and marks `y` as unused, the second pass removes `y` and marks `x` as unused, and the third pass removes `x`.

    However, this had the side effect of making esbuild incorrectly think that a single export is exported twice (because it's processed more than once). This release fixes that bug by only iterating multiple times over imports, not exports. There should no longer be duplicate export errors for this case.

* Add support for type-only TypeScript import-equals statements ([#1285](https://github.com/evanw/esbuild/pull/1285))

    This adds support for the following new TypeScript syntax that was added in version 4.2:

    ```ts
    import type React = require('react')
    ```

    Unlike `import React = require('react')`, this statement is a type declaration instead of a value declaration and should be omitted from the generated code. See [microsoft/TypeScript#41573](https://github.com/microsoft/TypeScript/pull/41573) for details. This feature was contributed by [@g-plane](https://github.com/g-plane).

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

    In TypeScript, type parameters come after a type and are placed in angle brackets like `Foo<T>`. However, certain built-in types do not accept type parameters including primitive types such as `number`. This means `if (x as number < 1) {}` is not a syntax error while `if (x as Foo < 1) {}` is a syntax error. This release changes TypeScript type parsing to allow type parameters in a more restricted set of situations, which should hopefully better resolve these type parsing ambiguities.

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

    XML-style namespaced names with a `:` in the middle are a part of the [JSX specification](https://facebook.github.io/jsx/) but they are explicitly unimplemented by React and TypeScript so esbuild doesn't currently support them. However, there was a user request to support this feature since it's part of the JSX specification and esbuild's JSX support can be used for non-React purposes. So this release now supports namespaced names in JSX expressions:

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
