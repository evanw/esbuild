# Changelog

## 0.16.14

* Preserve some comments in expressions ([#2721](https://github.com/evanw/esbuild/issues/2721))

    Various tools give semantic meaning to comments embedded inside of expressions. For example, Webpack and Vite have special "magic comments" that can be used to affect code splitting behavior:

    ```js
    import(/* webpackChunkName: "foo" */ '../foo');
    import(/* @vite-ignore */ dynamicVar);
    new Worker(/* webpackChunkName: "bar" */ new URL("../bar.ts", import.meta.url));
    new Worker(new URL('./path', import.meta.url), /* @vite-ignore */ dynamicOptions);
    ```

    Since esbuild can be used as a preprocessor for these tools (e.g. to strip TypeScript types), it can be problematic if esbuild doesn't do additional work to try to retain these comments. Previously esbuild special-cased Webpack comments in these specific locations in the AST. But Vite would now like to use similar comments, and likely other tools as well.

    So with this release, esbuild now will attempt to preserve some comments inside of expressions in more situations than before. This behavior is mainly intended to preserve these special "magic comments" that are meant for other tools to consume, although esbuild will no longer only preserve Webpack-specific comments so it should now be tool-agnostic. There is no guarantee that all such comments will be preserved (especially when `--minify-syntax` is enabled). So this change does *not* mean that esbuild is now usable as a code formatter. In particular comment preservation is more likely to happen with leading comments than with trailing comments. You should put comments that you want to be preserved *before* the relevant expression instead of after it. Also note that this change does not retain any more statement-level comments than before (i.e. comments not embedded inside of expressions). Comment preservation is not enabled when `--minify-whitespace` is enabled (which is automatically enabled when you use `--minify`).

## 0.16.13

* Publish a new bundle visualization tool

    While esbuild provides bundle metadata via the `--metafile` flag, previously esbuild left analysis of it completely up to third-party tools (well, outside of the rudimentary `--analyze` flag). However, the esbuild website now has a built-in bundle visualization tool:

    * https://esbuild.github.io/analyze/

    You can pass `--metafile` to esbuild to output bundle metadata, then upload that JSON file to this tool to visualize your bundle. This is helpful for answering questions such as:

    * Which packages are included in my bundle?
    * How did a specific file get included?
    * How small did a specific file compress to?
    * Was a specific file tree-shaken or not?

    I'm publishing this tool because I think esbuild should provide *some* answer to "how do I visualize my bundle" without requiring people to reach for third-party tools. At the moment the tool offers two types of visualizations: a radial "sunburst chart" and a linear "flame chart". They serve slightly different but overlapping use cases (e.g. the sunburst chart is more keyboard-accessible while the flame chart is easier with the mouse). This tool may continue to evolve over time.

* Fix `--metafile` and `--mangle-cache` with `--watch` ([#1357](https://github.com/evanw/esbuild/issues/1357))

    The CLI calls the Go API and then also writes out the metafile and/or mangle cache JSON files if those features are enabled. This extra step is necessary because these files are returned by the Go API as in-memory strings. However, this extra step accidentally didn't happen for all builds after the initial build when watch mode was enabled. This behavior used to work but it was broken in version 0.14.18 by the introduction of the mangle cache feature. This release fixes the combination of these features, so the metafile and mangle cache features should now work with watch mode. This behavior was only broken for the CLI, not for the JS or Go APIs.

* Add an `original` field to the metafile

    The metadata file JSON now has an additional field: each import in an input file now contains the pre-resolved path in the `original` field in addition to the post-resolved path in the `path` field. This means it's now possible to run certain additional analysis over your bundle. For example, you should be able to use this to detect when the same package subpath is represented multiple times in the bundle, either because multiple versions of a package were bundled or because a package is experiencing the [dual-package hazard](https://nodejs.org/api/packages.html#dual-package-hazard).

## 0.16.12

* Loader defaults to `js` for extensionless files ([#2776](https://github.com/evanw/esbuild/issues/2776))

    Certain packages contain files without an extension. For example, the `yargs` package contains the file `yargs/yargs` which has no extension. Node, Webpack, and Parcel can all understand code that imports `yargs/yargs` because they assume that the file is JavaScript. However, esbuild was previously unable to understand this code because it relies on the file extension to tell it how to interpret the file. With this release, esbuild will now assume files without an extension are JavaScript files. This can be customized by setting the loader for `""` (the empty string, representing files without an extension) to another loader. For example, if you want files without an extension to be treated as CSS instead, you can do that like this:

    * CLI:

        ```
        esbuild --bundle --loader:=css
        ```

    * JS:

        ```js
        esbuild.build({
          bundle: true,
          loader: { '': 'css' },
        })
        ```

    * Go:

        ```go
        api.Build(api.BuildOptions{
          Bundle: true,
          Loader: map[string]api.Loader{"": api.LoaderCSS},
        })
        ```

    In addition, the `"type"` field in `package.json` files now only applies to files with an explicit `.js`, `.jsx`, `.ts`, or `.tsx` extension. Previously it was incorrectly applied by esbuild to all files that had an extension other than `.mjs`, `.mts`, `.cjs`, or `.cts` including extensionless files. So for example an extensionless file in a `"type": "module"` package is now treated as CommonJS instead of ESM.

## 0.16.11

* Avoid a syntax error in the presence of direct `eval` ([#2761](https://github.com/evanw/esbuild/issues/2761))

    The behavior of nested `function` declarations in JavaScript depends on whether the code is run in strict mode or not. It would be problematic if esbuild preserved nested `function` declarations in its output because then the behavior would depend on whether the output was run in strict mode or not instead of respecting the strict mode behavior of the original source code. To avoid this, esbuild transforms nested `function` declarations to preserve the intended behavior of the original source code regardless of whether the output is run in strict mode or not:

    ```js
    // Original code
    if (true) {
      function foo() {}
      console.log(!!foo)
      foo = null
      console.log(!!foo)
    }
    console.log(!!foo)

    // Transformed code
    if (true) {
      let foo2 = function() {
      };
      var foo = foo2;
      console.log(!!foo2);
      foo2 = null;
      console.log(!!foo2);
    }
    console.log(!!foo);
    ```

    In the above example, the original code should print `true false true` because it's not run in strict mode (it doesn't contain `"use strict"` and is not an ES module). The code that esbuild generates has been transformed such that it prints `true false true` regardless of whether it's run in strict mode or not.

    However, this transformation is impossible if the code contains direct `eval` because direct `eval` "poisons" all containing scopes by preventing anything in those scopes from being renamed. That prevents esbuild from splitting up accesses to `foo` into two separate variables with different names. Previously esbuild still did this transformation but with two variables both named `foo`, which is a syntax error. With this release esbuild will now skip doing this transformation when direct `eval` is present to avoid generating code with a syntax error. This means that the generated code may no longer behave as intended since the behavior depends on the run-time strict mode setting instead of the strict mode setting present in the original source code. To fix this problem, you will need to remove the use of direct `eval`.

* Fix a bundling scenario involving multiple symlinks ([#2773](https://github.com/evanw/esbuild/issues/2773), [#2774](https://github.com/evanw/esbuild/issues/2774))

    This release contains a fix for a bundling scenario involving an import path where multiple path segments are symlinks. Previously esbuild was unable to resolve certain import paths in this scenario, but these import paths should now work starting with this release. This fix was contributed by [@onebytegone](https://github.com/onebytegone).

## 0.16.10

* Change the default "legal comment" behavior again ([#2745](https://github.com/evanw/esbuild/issues/2745))

    The legal comments feature automatically gathers comments containing `@license` or `@preserve` and puts the comments somewhere (either in the generated code or in a separate file). This behavior used to be on by default but was disabled by default in version 0.16.0 because automatically inserting comments is potentially confusing and misleading. These comments can appear to be assigning the copyright of your code to another entity. And this behavior can be especially problematic if it happens automatically by default since you may not even be aware of it happening. For example, if you bundle the TypeScript compiler the preserving legal comments means your source code would contain this comment, which appears to be assigning the copyright of all of your code to Microsoft:

    ```js
    /*! *****************************************************************************
    Copyright (c) Microsoft Corporation. All rights reserved.
    Licensed under the Apache License, Version 2.0 (the "License"); you may not use
    this file except in compliance with the License. You may obtain a copy of the
    License at http://www.apache.org/licenses/LICENSE-2.0

    THIS CODE IS PROVIDED ON AN *AS IS* BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
    KIND, EITHER EXPRESS OR IMPLIED, INCLUDING WITHOUT LIMITATION ANY IMPLIED
    WARRANTIES OR CONDITIONS OF TITLE, FITNESS FOR A PARTICULAR PURPOSE,
    MERCHANTABLITY OR NON-INFRINGEMENT.

    See the Apache Version 2.0 License for specific language governing permissions
    and limitations under the License.
    ***************************************************************************** */
    ```

    However, people have asked for this feature to be re-enabled by default. To resolve the confusion about what these comments are applying to, esbuild's default behavior will now be to attempt to describe which package the comments are coming from. So while this feature has been re-enabled by default, the output will now look something like this instead:

    ```js
    /*! Bundled license information:

    typescript/lib/typescript.js:
      (*! *****************************************************************************
      Copyright (c) Microsoft Corporation. All rights reserved.
      Licensed under the Apache License, Version 2.0 (the "License"); you may not use
      this file except in compliance with the License. You may obtain a copy of the
      License at http://www.apache.org/licenses/LICENSE-2.0

      THIS CODE IS PROVIDED ON AN *AS IS* BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
      KIND, EITHER EXPRESS OR IMPLIED, INCLUDING WITHOUT LIMITATION ANY IMPLIED
      WARRANTIES OR CONDITIONS OF TITLE, FITNESS FOR A PARTICULAR PURPOSE,
      MERCHANTABLITY OR NON-INFRINGEMENT.

      See the Apache Version 2.0 License for specific language governing permissions
      and limitations under the License.
      ***************************************************************************** *)
    */
    ```

    Note that you can still customize this behavior with the `--legal-comments=` flag. For example, you can use `--legal-comments=none` to turn this off, or you can use `--legal-comments=linked` to put these comments in a separate `.LEGAL.txt` file instead.

* Enable `external` legal comments with the transform API ([#2390](https://github.com/evanw/esbuild/issues/2390))

    Previously esbuild's transform API only supported `none`, `inline`, or `eof` legal comments. With this release, `external` legal comments are now also supported with the transform API. This only applies to the JS and Go APIs, not to the CLI, and looks like this:

    * JS:

        ```js
        const { code, legalComments } = await esbuild.transform(input, {
          legalComments: 'external',
        })
        ```

    * Go:

        ```go
        result := api.Transform(input, api.TransformOptions{
          LegalComments: api.LegalCommentsEndOfFile,
        })
        code := result.Code
        legalComments := result.LegalComments
        ```

* Fix duplicate function declaration edge cases ([#2757](https://github.com/evanw/esbuild/issues/2757))

    The change in the previous release to forbid duplicate function declarations in certain cases accidentally forbid some edge cases that should have been allowed. Specifically duplicate function declarations are forbidden in nested blocks in strict mode and at the top level of modules, but are allowed when they are declared at the top level of function bodies. This release fixes the regression by re-allowing the last case.

* Allow package subpaths with `alias` ([#2715](https://github.com/evanw/esbuild/issues/2715))

    Previously the names passed to the `alias` feature had to be the name of a package (with or without a package scope). With this release, you can now also use the `alias` feature with package subpaths. So for example you can now create an alias that substitutes `@org/pkg/lib` with something else.

## 0.16.9

* Update to Unicode 15.0.0

    The character tables that determine which characters form valid JavaScript identifiers have been updated from Unicode version 14.0.0 to the newly-released Unicode version 15.0.0. I'm not putting an example in the release notes because all of the new characters will likely just show up as little squares since fonts haven't been updated yet. But you can read https://www.unicode.org/versions/Unicode15.0.0/#Summary for more information about the changes.

* Disallow duplicate lexically-declared names in nested blocks and in strict mode

    In strict mode or in a nested block, it's supposed to be a syntax error to declare two symbols with the same name unless all duplicate entries are either `function` declarations or all `var` declarations. However, esbuild was overly permissive and allowed this when duplicate entries were either `function` declarations or `var` declarations (even if they were mixed). This check has now been made more restrictive to match the JavaScript specification:

    ```js
    // JavaScript allows this
    var a
    function a() {}
    {
      var b
      var b
      function c() {}
      function c() {}
    }

    // JavaScript doesn't allow this
    {
      var d
      function d() {}
    }
    ```

* Add a type declaration for the new `empty` loader ([#2755](https://github.com/evanw/esbuild/pull/2755))

    I forgot to add this in the previous release. It has now been added.

    This fix was contributed by [@fz6m](https://github.com/fz6m).

* Add support for the `v` flag in regular expression literals

    People are currently working on adding a `v` flag to JavaScript regular expresions. You can read more about this flag here: https://v8.dev/features/regexp-v-flag. This release adds support for parsing this flag, so esbuild will now no longer consider regular expression literals with this flag to be a syntax error. If the target is set to something other than `esnext`, esbuild will transform regular expression literals containing this flag into a `new RegExp()` constructor call so the resulting code doesn't have a syntax error. This enables you to provide a polyfill for `RegExp` that implements the `v` flag to get your code to work at run-time. While esbuild doesn't typically adopt proposals until they're already shipping in a real JavaScript run-time, I'm adding it now because a) esbuild's implementation doesn't need to change as the proposal evolves, b) this isn't really new syntax since regular expression literals already have flags, and c) esbuild's implementation is a trivial pass-through anyway.

* Avoid keeping the name of classes with static `name` properties

    The `--keep-names` property attempts to preserve the original value of the `name` property for functions and classes even when identifiers are renamed by the minifier or to avoid a name collision. This is currently done by generating code to assign a string to the `name` property on the function or class object. However, this should not be done for classes with a static `name` property since in that case the explicitly-defined `name` property overwrites the automatically-generated class name. With this release, esbuild will now no longer attempt to preserve the `name` property for classes with a static `name` property.

## 0.16.8

* Allow plugins to resolve injected files ([#2754](https://github.com/evanw/esbuild/issues/2754))

    Previously paths passed to the `inject` feature were always interpreted as file system paths. This meant that `onResolve` plugins would not be run for them and esbuild's default path resolver would always be used. This meant that the `inject` feature couldn't be used in the browser since the browser doesn't have access to a file system. This release runs paths passed to `inject` through esbuild's full path resolution pipeline so plugins now have a chance to handle them using `onResolve` callbacks. This makes it possible to write a plugin that makes esbuild's `inject` work in the browser.

* Add the `empty` loader ([#1541](https://github.com/evanw/esbuild/issues/1541), [#2753](https://github.com/evanw/esbuild/issues/2753))

    The new `empty` loader tells esbuild to pretend that a file is empty. So for example `--loader:.css=empty` effectively skips all imports of `.css` files in JavaScript so that they aren't included in the bundle, since `import "./some-empty-file"` in JavaScript doesn't bundle anything. You can also use the `empty` loader to remove asset references in CSS files. For example `--loader:.png=empty` causes esbuild to replace asset references such as `url(image.png)` with `url()` so that they are no longer included in the resulting style sheet.

* Fix `</script>` and `</style>` escaping for non-default targets ([#2748](https://github.com/evanw/esbuild/issues/2748))

    The change in version 0.16.0 to give control over `</script>` escaping via `--supported:inline-script=false` or `--supported:inline-script=true` accidentally broke automatic escaping of `</script>` when an explicit `target` setting is specified. This release restores the correct automatic escaping of `</script>` (which should not depend on what `target` is set to).

* Enable the `exports` field with `NODE_PATHS` ([#2752](https://github.com/evanw/esbuild/issues/2752))

    Node has a rarely-used feature where you can extend the set of directories that node searches for packages using the `NODE_PATHS` environment variable. While esbuild supports this too, previously it only supported the old `main` field path resolution but did not support the new `exports` field package resolution. This release makes the path resolution rules the same again for both `node_modules` directories and `NODE_PATHS` directories.

## 0.16.7

* Include `file` loader strings in metafile imports ([#2731](https://github.com/evanw/esbuild/issues/2731))

    Bundling a file with the `file` loader copies that file to the output directory and imports a module with the path to the copied file in the `default` export. Previously when bundling with the `file` loader, there was no reference in the metafile from the JavaScript file containing the path string to the copied file. With this release, there will now be a reference in the metafile in the `imports` array with the kind `file-loader`:

    ```diff
     {
       ...
       "outputs": {
         "out/image-55CCFTCE.svg": {
           ...
         },
         "out/entry.js": {
           "imports": [
    +        {
    +          "path": "out/image-55CCFTCE.svg",
    +          "kind": "file-loader"
    +        }
           ],
           ...
         }
       }
     }
    ```

* Fix byte counts in metafile regarding references to other output files ([#2071](https://github.com/evanw/esbuild/issues/2071))

    Previously files that contained references to other output files had slightly incorrect metadata for the byte counts of input files which contributed to that output file. So for example if `app.js` imports `image.png` using the file loader and esbuild generates `out.js` and `image-LSAMBFUD.png`, the metadata for how many bytes of `out.js` are from `app.js` was slightly off (the metadata for the byte count of `out.js` was still correct). The reason is because esbuild substitutes the final paths for references between output files toward the end of the build to handle cyclic references, and the byte counts needed to be adjusted as well during the path substitution. This release fixes these byte counts (specifically the `bytesInOutput` values).

* The alias feature now strips a trailing slash ([#2730](https://github.com/evanw/esbuild/issues/2730))

    People sometimes add a trailing slash to the name of one of node's built-in modules to force node to import from the file system instead of importing the built-in module. For example, importing `util` imports node's built-in module called `util` but importing `util/` tries to find a package called `util` on the file system. Previously attempting to use esbuild's package alias feature to replace imports to `util` with a specific file would fail because the file path would also gain a trailing slash (e.g. mapping `util` to `./file.js` turned `util/` into `./file.js/`). With this release, esbuild will now omit the path suffix if it's a single trailing slash, which should now allow you to successfully apply aliases to these import paths.

## 0.16.6

* Do not mark subpath imports as external with `--packages=external` ([#2741](https://github.com/evanw/esbuild/issues/2741))

    Node has a feature called [subpath imports](https://nodejs.org/api/packages.html#subpath-imports) where special import paths that start with `#` are resolved using the `imports` field in the `package.json` file of the enclosing package. The intent of the newly-added `--packages=external` setting is to exclude a package's dependencies from the bundle. Since a package's subpath imports are only accessible within that package, it's wrong for them to be affected by `--packages=external`. This release changes esbuild so that `--packages=external` no longer affects subpath imports.

* Forbid invalid numbers in JSON files

    Previously esbuild parsed numbers in JSON files using the same syntax as JavaScript. But starting from this release, esbuild will now parse them with JSON syntax instead. This means the following numbers are no longer allowed by esbuild in JSON files:

    * Legacy octal literals (non-zero integers starting with `0`)
    * The `0b`, `0o`, and `0x` numeric prefixes
    * Numbers containing `_` such as `1_000`
    * Leading and trailing `.` such as `0.` and `.0`
    * Numbers with a space after the `-` such as `- 1`

* Add external imports to metafile ([#905](https://github.com/evanw/esbuild/issues/905), [#1768](https://github.com/evanw/esbuild/issues/1768), [#1933](https://github.com/evanw/esbuild/issues/1933), [#1939](https://github.com/evanw/esbuild/issues/1939))

    External imports now appear in `imports` arrays in the metafile (which is present when bundling with `metafile: true`) next to normal imports, but additionally have `external: true` to set them apart. This applies both to files in the `inputs` section and the `outputs` section. Here's an example:

    ```diff
     {
       "inputs": {
         "style.css": {
           "bytes": 83,
           "imports": [
    +        {
    +          "path": "https://cdn.jsdelivr.net/npm/bootstrap@5.2.3/dist/css/bootstrap.min.css",
    +          "kind": "import-rule",
    +          "external": true
    +        }
           ]
         },
         "app.js": {
           "bytes": 100,
           "imports": [
    +        {
    +          "path": "https://cdn.jsdelivr.net/npm/bootstrap@5.2.3/dist/js/bootstrap.min.js",
    +          "kind": "import-statement",
    +          "external": true
    +        },
             {
               "path": "style.css",
               "kind": "import-statement"
             }
           ]
         }
       },
       "outputs": {
         "out/app.js": {
           "imports": [
    +        {
    +          "path": "https://cdn.jsdelivr.net/npm/bootstrap@5.2.3/dist/js/bootstrap.min.js",
    +          "kind": "require-call",
    +          "external": true
    +        }
           ],
           "exports": [],
           "entryPoint": "app.js",
           "cssBundle": "out/app.css",
           "inputs": {
             "app.js": {
               "bytesInOutput": 113
             },
             "style.css": {
               "bytesInOutput": 0
             }
           },
           "bytes": 528
         },
         "out/app.css": {
           "imports": [
    +        {
    +          "path": "https://cdn.jsdelivr.net/npm/bootstrap@5.2.3/dist/css/bootstrap.min.css",
    +          "kind": "import-rule",
    +          "external": true
    +        }
           ],
           "inputs": {
             "style.css": {
               "bytesInOutput": 0
             }
           },
           "bytes": 100
         }
       }
     }
    ```

    One additional useful consequence of this is that the `imports` array is now populated when bundling is disabled. So you can now use esbuild with bundling disabled to inspect a file's imports.

## 0.16.5

* Make it easy to exclude all packages from a bundle ([#1958](https://github.com/evanw/esbuild/issues/1958), [#1975](https://github.com/evanw/esbuild/issues/1975), [#2164](https://github.com/evanw/esbuild/issues/2164), [#2246](https://github.com/evanw/esbuild/issues/2246), [#2542](https://github.com/evanw/esbuild/issues/2542))

    When bundling for node, it's often necessary to exclude npm packages from the bundle since they weren't designed with esbuild bundling in mind and don't work correctly after being bundled. For example, they may use `__dirname` and run-time file system calls to load files, which doesn't work after bundling with esbuild. Or they may compile a native `.node` extension that has similar expectations about the layout of the file system that are no longer true after bundling (even if the `.node` extension is copied next to the bundle).

    The way to get this to work with esbuild is to use the `--external:` flag. For example, the [`fsevents`](https://www.npmjs.com/package/fsevents) package contains a native `.node` extension and shouldn't be bundled. To bundle code that uses it, you can pass `--external:fsevents` to esbuild to exclude it from your bundle. You will then need to ensure that the `fsevents` package is still present when you run your bundle (e.g. by publishing your bundle to npm as a package with a dependency on `fsevents`).

    It was possible to automatically do this for all of your dependencies, but it was inconvenient. You had to write some code that read your `package.json` file and passed the keys of the `dependencies`, `devDependencies`, `peerDependencies`, and/or `optionalDependencies` maps to esbuild as external packages (either that or write a plugin to mark all package paths as external). Previously esbuild's recommendation for making this easier was to do `--external:./node_modules/*` (added in version 0.14.13). However, this was a bad idea because it caused compatibility problems with many node packages as it caused esbuild to mark the post-resolve path as external instead of the pre-resolve path. Doing that could break packages that are published as both CommonJS and ESM if esbuild's bundler is also used to do a module format conversion.

    With this release, you can now do the following to automatically exclude all packages from your bundle:

    * CLI:

        ```
        esbuild --bundle --packages=external
        ```

    * JS:

        ```js
        esbuild.build({
          bundle: true,
          packages: 'external',
        })
        ```

    * Go:

        ```go
        api.Build(api.BuildOptions{
          Bundle:   true,
          Packages: api.PackagesExternal,
        })
        ```

    Doing `--external:./node_modules/*` is still possible and still has the same behavior, but is no longer recommended. I recommend that you use the new `packages` feature instead.

* Fix some subtle bugs with tagged template literals

    This release fixes a bug where minification could incorrectly change the value of `this` within tagged template literal function calls:

    ```js
    // Original code
    function f(x) {
      let z = y.z
      return z``
    }

    // Old output (with --minify)
    function f(n){return y.z``}

    // New output (with --minify)
    function f(n){return(0,y.z)``}
    ```

    This release also fixes a bug where using optional chaining with `--target=es2019` or earlier could incorrectly change the value of `this` within tagged template literal function calls:

    ```js
    // Original code
    var obj = {
      foo: function() {
        console.log(this === obj);
      }
    };
    (obj?.foo)``;

    // Old output (with --target=es6)
    var obj = {
      foo: function() {
        console.log(this === obj);
      }
    };
    (obj == null ? void 0 : obj.foo)``;

    // New output (with --target=es6)
    var __freeze = Object.freeze;
    var __defProp = Object.defineProperty;
    var __template = (cooked, raw) => __freeze(__defProp(cooked, "raw", { value: __freeze(raw || cooked.slice()) }));
    var _a;
    var obj = {
      foo: function() {
        console.log(this === obj);
      }
    };
    (obj == null ? void 0 : obj.foo).call(obj, _a || (_a = __template([""])));
    ```

* Some slight minification improvements

    The following minification improvements were implemented:

    * `if (~a !== 0) throw x;` => `if (~a) throw x;`
    * `if ((a | b) !== 0) throw x;` => `if (a | b) throw x;`
    * `if ((a & b) !== 0) throw x;` => `if (a & b) throw x;`
    * `if ((a ^ b) !== 0) throw x;` => `if (a ^ b) throw x;`
    * `if ((a << b) !== 0) throw x;` => `if (a << b) throw x;`
    * `if ((a >> b) !== 0) throw x;` => `if (a >> b) throw x;`
    * `if ((a >>> b) !== 0) throw x;` => `if (a >>> b) throw x;`
    * `if (!!a || !!b) throw x;` => `if (a || b) throw x;`
    * `if (!!a && !!b) throw x;` => `if (a && b) throw x;`
    * `if (a ? !!b : !!c) throw x;` => `if (a ? b : c) throw x;`

## 0.16.4

* Fix binary downloads from the `@esbuild/` scope for Deno ([#2729](https://github.com/evanw/esbuild/issues/2729))

    Version 0.16.0 of esbuild moved esbuild's binary executables into npm packages under the `@esbuild/` scope, which accidentally broke the binary downloader script for Deno. This release fixes this script so it should now be possible to use esbuild version 0.16.4+ with Deno.

## 0.16.3

* Fix a hang with the JS API in certain cases ([#2727](https://github.com/evanw/esbuild/issues/2727))

    A change that was made in version 0.15.13 accidentally introduced a case when using esbuild's JS API could cause the node process to fail to exit. The change broke esbuild's watchdog timer, which detects if the parent process no longer exists and then automatically exits esbuild. This hang happened when you ran node as a child process with the `stderr` stream set to `pipe` instead of `inherit`, in the child process you call esbuild's JS API and pass `incremental: true` but do not call `dispose()` on the returned `rebuild` object, and then call `process.exit()`. In that case the parent node process was still waiting for the esbuild process that was created by the child node process to exit. The change made in version 0.15.13 was trying to avoid using Go's `sync.WaitGroup` API incorrectly because the API is not thread-safe. Instead of doing this, I have now reverted that change and implemented a thread-safe version of the `sync.WaitGroup` API for esbuild to use instead.

## 0.16.2

* Fix `process.env.NODE_ENV` substitution when transforming ([#2718](https://github.com/evanw/esbuild/issues/2718))

    Version 0.16.0 introduced an unintentional regression that caused `process.env.NODE_ENV` to be automatically substituted with either `"development"` or `"production"` when using esbuild's `transform` API. This substitution is a necessary feature of esbuild's `build` API because the React framework crashes when you bundle it without doing this. But the `transform` API is typically used as part of a larger build pipeline so the benefit of esbuild doing this automatically is not as clear, and esbuild previously didn't do this.

    However, version 0.16.0 switched the default value of the `platform` setting for the `transform` API from `neutral` to `browser`, both to align it with esbuild's documentation (which says `browser` is the default value) and because escaping the `</script>` character sequence is now tied to the `browser` platform (see the release notes for version 0.16.0 for details). That accidentally enabled automatic substitution of `process.env.NODE_ENV` because esbuild always did that for code meant for the browser. To fix this regression, esbuild will now only automatically substitute `process.env.NODE_ENV` when using the `build` API.

* Prevent `define` from substituting constants into assignment position ([#2719](https://github.com/evanw/esbuild/issues/2719))

    The `define` feature lets you replace certain expressions with constants. For example, you could use it to replace references to the global property reference `window.DEBUG` with `false` at compile time, which can then potentially help esbuild remove unused code from your bundle. It's similar to [DefinePlugin](https://webpack.js.org/plugins/define-plugin/) in Webpack.

    However, if you write code such as `window.DEBUG = true` and then defined `window.DEBUG` to `false`, esbuild previously generated the output `false = true` which is a syntax error in JavaScript. This behavior is not typically a problem because it doesn't make sense to substitute `window.DEBUG` with a constant if its value changes at run-time (Webpack's `DefinePlugin` also generates `false = true` in this case). But it can be alarming to have esbuild generate code with a syntax error.

    So with this release, esbuild will no longer substitute `define` constants into assignment position to avoid generating code with a syntax error. Instead esbuild will generate a warning, which currently looks like this:

    ```
    ▲ [WARNING] Suspicious assignment to defined constant "window.DEBUG" [assign-to-define]

        example.js:1:0:
          1 │ window.DEBUG = true
            ╵ ~~~~~~~~~~~~

      The expression "window.DEBUG" has been configured to be replaced with a constant using the
      "define" feature. If this expression is supposed to be a compile-time constant, then it doesn't
      make sense to assign to it here. Or if this expression is supposed to change at run-time, this
      "define" substitution should be removed.
    ```

* Fix a regression with `npm install --no-optional` ([#2720](https://github.com/evanw/esbuild/issues/2720))

    Normally when you install esbuild with `npm install`, npm itself is the tool that downloads the correct binary executable for the current platform. This happens because of how esbuild's primary package uses npm's `optionalDependencies` feature. However, if you deliberately disable this with `npm install --no-optional` then esbuild's install script will attempt to repair the installation by manually downloading and extracting the binary executable from the package that was supposed to be installed.

    The change in version 0.16.0 to move esbuild's nested packages into the `@esbuild/` scope unintentionally broke this logic because of how npm's URL structure is different for scoped packages vs. normal packages. It was actually already broken for a few platforms earlier because esbuild already had packages for some platforms in the `@esbuild/` scope, but I didn't discover this then because esbuild's integration tests aren't run on all platforms. Anyway, this release contains some changes to the install script that should hopefully get this scenario working again.

## 0.16.1

This is a hotfix for the previous release.

* Re-allow importing JSON with the `copy` loader using an import assertion

    The previous release made it so when `assert { type: 'json' }` is present on an import statement, esbuild validated that the `json` loader was used. This is what an import assertion is supposed to do. However, I forgot about the relatively new `copy` loader, which sort of behaves as if the import path was marked as external (and thus not loaded at all) except that the file is copied to the output directory and the import path is rewritten to point to the copy. In this case whatever JavaScript runtime ends up running the code is the one to evaluate the import assertion. So esbuild should really allow this case as well. With this release, esbuild now allows both the `json` and `copy` loaders when an `assert { type: 'json' }` import assertion is present.

## 0.16.0

**This release deliberately contains backwards-incompatible changes.** To avoid automatically picking up releases like this, you should either be pinning the exact version of `esbuild` in your `package.json` file (recommended) or be using a version range syntax that only accepts patch upgrades such as `^0.15.0` or `~0.15.0`. See npm's documentation about [semver](https://docs.npmjs.com/cli/v6/using-npm/semver/) for more information.

* Move all binary executable packages to the `@esbuild/` scope

    Binary package executables for esbuild are published as individual packages separate from the main `esbuild` package so you only have to download the relevant one for the current platform when you install esbuild. This release moves all of these packages under the `@esbuild/` scope to avoid collisions with 3rd-party packages. It also changes them to a consistent naming scheme that uses the `os` and `cpu` names from node.

    The package name changes are as follows:

    * `@esbuild/linux-loong64` => `@esbuild/linux-loong64` (no change)
    * `esbuild-android-64` => `@esbuild/android-x64`
    * `esbuild-android-arm64` => `@esbuild/android-arm64`
    * `esbuild-darwin-64` => `@esbuild/darwin-x64`
    * `esbuild-darwin-arm64` => `@esbuild/darwin-arm64`
    * `esbuild-freebsd-64` => `@esbuild/freebsd-x64`
    * `esbuild-freebsd-arm64` => `@esbuild/freebsd-arm64`
    * `esbuild-linux-32` => `@esbuild/linux-ia32`
    * `esbuild-linux-64` => `@esbuild/linux-x64`
    * `esbuild-linux-arm` => `@esbuild/linux-arm`
    * `esbuild-linux-arm64` => `@esbuild/linux-arm64`
    * `esbuild-linux-mips64le` => `@esbuild/linux-mips64el`
    * `esbuild-linux-ppc64le` => `@esbuild/linux-ppc64`
    * `esbuild-linux-riscv64` => `@esbuild/linux-riscv64`
    * `esbuild-linux-s390x` => `@esbuild/linux-s390x`
    * `esbuild-netbsd-64` => `@esbuild/netbsd-x64`
    * `esbuild-openbsd-64` => `@esbuild/openbsd-x64`
    * `esbuild-sunos-64` => `@esbuild/sunos-x64`
    * `esbuild-wasm` => `esbuild-wasm` (no change)
    * `esbuild-windows-32` => `@esbuild/win32-ia32`
    * `esbuild-windows-64` => `@esbuild/win32-x64`
    * `esbuild-windows-arm64` => `@esbuild/win32-arm64`
    * `esbuild` => `esbuild` (no change)

    Normal usage of the `esbuild` and `esbuild-wasm` packages should not be affected. These name changes should only affect tools that hard-coded the individual binary executable package names into custom esbuild downloader scripts.

    This change was not made with performance in mind. But as a bonus, installing esbuild with npm may potentially happen faster now. This is because npm's package installation protocol is inefficient: it always downloads metadata for all past versions of each package even when it only needs metadata about a single version. This makes npm package downloads O(n) in the number of published versions, which penalizes packages like esbuild that are updated regularly. Since most of esbuild's package names have now changed, npm will now need to download much less data when installing esbuild (8.72mb of package manifests before this change → 0.06mb of package manifests after this change). However, this is only a temporary improvement. Installing esbuild will gradually get slower again as further versions of esbuild are published.

* Publish a shell script that downloads esbuild directly

    In addition to all of the existing ways to install esbuild, you can now also download esbuild directly like this:

    ```sh
    curl -fsSL https://esbuild.github.io/dl/latest | sh
    ```

    This runs a small shell script that downloads the latest `esbuild` binary executable to the current directory. This can be convenient on systems that don't have `npm` installed or when you just want to get a copy of esbuild quickly without any extra steps. If you want a specific version of esbuild (starting with this version onward), you can provide that version in the URL instead of `latest`:

    ```sh
    curl -fsSL https://esbuild.github.io/dl/v0.16.0 | sh
    ```

    Note that the download script needs to be able to access registry.npmjs.org to be able to complete the download. This download script doesn't yet support all of the platforms that esbuild supports because I lack the necessary testing environments. If the download script doesn't work for you because you're on an unsupported platform, please file an issue on the esbuild repo so we can add support for it.

* Fix some parameter names for the Go API

    This release changes some parameter names for the Go API to be consistent with the JavaScript and CLI APIs:

    * `OutExtensions` => `OutExtension`
    * `JSXMode` => `JSX`

* Add additional validation of API parameters

    The JavaScript API now does some additional validation of API parameters to catch incorrect uses of esbuild's API. The biggest impact of this is likely that esbuild now strictly only accepts strings with the `define` parameter. This would already have been a type error with esbuild's TypeScript type definitions, but it was previously not enforced for people using esbuild's API JavaScript without TypeScript.

    The `define` parameter appears at first glance to take a JSON object if you aren't paying close attention, but this actually isn't true. Values for `define` are instead strings of JavaScript code. This means you have to use `define: { foo: '"bar"' }` to replace `foo` with the string `"bar"`. Using `define: { foo: 'bar' }` actually replaces `foo` with the identifier `bar`. Previously esbuild allowed you to pass `define: { foo: false }` and `false` was automatically converted into a string, which made it more confusing to understand what `define` actually represents. Starting with this release, passing non-string values such as with `define: { foo: false }` will no longer be allowed. You will now have to write `define: { foo: 'false' }` instead.

* Generate shorter data URLs if possible ([#1843](https://github.com/evanw/esbuild/issues/1843))

    Loading a file with esbuild's `dataurl` loader generates a JavaScript module with a [data URL](https://developer.mozilla.org/en-US/docs/Web/HTTP/Basics_of_HTTP/Data_URLs) for that file in a string as a single default export. Previously the data URLs generated by esbuild all used [base64 encoding](https://en.wikipedia.org/wiki/Base64). However, this is unnecessarily long for most textual data (e.g. SVG images). So with this release, esbuild's `dataurl` loader will now use percent encoding instead of base64 encoding if the result will be shorter. This can result in ~25% smaller data URLs for large SVGs. If you want the old behavior, you can use the `base64` loader instead and then construct the data URL yourself.

* Avoid marking entry points as external ([#2382](https://github.com/evanw/esbuild/issues/2382))

    Previously you couldn't specify `--external:*` to mark all import paths as external because that also ended up making the entry point itself external, which caused the build to fail. With this release, esbuild's `external` API parameter no longer applies to entry points so using `--external:*` is now possible.

    One additional consequence of this change is that the `kind` parameter is now required when calling the `resolve()` function in esbuild's plugin API. Previously the `kind` parameter defaulted to `entry-point`, but that no longer interacts with `external` so it didn't seem wise for this to continue to be the default. You now have to specify `kind` so that the path resolution mode is explicit.

* Disallow non-`default` imports when `assert { type: 'json' }` is present

    There is now standard behavior for importing a JSON file into an ES module using an `import` statement. However, it requires you to place the `assert { type: 'json' }` import assertion after the import path. This import assertion tells the JavaScript runtime to throw an error if the import does not end up resolving to a JSON file. On the web, the type of a file is determined by the `Content-Type` HTTP header instead of by the file extension. The import assertion prevents security problems on the web where a `.json` file may actually resolve to a JavaScript file containing malicious code, which is likely not expected for an import that is supposed to only contain pure side-effect free data.

    By default, esbuild uses the file extension to determine the type of a file, so this import assertion is unnecessary with esbuild. However, esbuild's JSON import feature has a non-standard extension that allows you to import top-level properties of the JSON object as named imports. For example, esbuild lets you do this:

    ```js
    import { version } from './package.json'
    ```

    This is useful for tree-shaking when bundling because it means esbuild will only include the the `version` field of `package.json` in your bundle. This is non-standard behavior though and doesn't match the behavior of what happens when you import JSON in a real JavaScript runtime (after adding `assert { type: 'json' }`). In a real JavaScript runtime the only thing you can import is the `default` import. So with this release, esbuild will now prevent you from importing non-`default` import names if `assert { type: 'json' }` is present. This ensures that code containing `assert { type: 'json' }` isn't relying on non-standard behavior that won't work everywhere. So the following code is now an error with esbuild when bundling:

    ```js
    import { version } from './package.json' assert { type: 'json' }
    ```

    In addition, adding `assert { type: 'json' }` to an import statement now means esbuild will generate an error if the loader for the file is anything other than `json`, which is required by the import assertion specification.

* Provide a way to disable automatic escaping of `</script>` ([#2649](https://github.com/evanw/esbuild/issues/2649))

    If you inject esbuild's output into a script tag in an HTML file, code containing the literal characters `</script>` will cause the tag to be ended early which will break the code:

    ```html
    <script>
      console.log("</script>");
    </script>
    ```

    To avoid this, esbuild automatically escapes these strings in generated JavaScript files (e.g. `"</script>"` becomes `"<\/script>"` instead). This also applies to `</style>` in generated CSS files. Previously this always happened and there wasn't a way to turn this off.

    With this release, esbuild will now only do this if the `platform` setting is set to `browser` (the default value). Setting `platform` to `node` or `neutral` will disable this behavior. This behavior can also now be disabled with `--supported:inline-script=false` (for JS) and `--supported:inline-style=false` (for CSS).

* Throw an early error if decoded UTF-8 text isn't a `Uint8Array` ([#2532](https://github.com/evanw/esbuild/issues/2532))

    If you run esbuild's JavaScript API in a broken JavaScript environment where `new TextEncoder().encode("") instanceof Uint8Array` is false, then esbuild's API will fail with a confusing serialization error message that makes it seem like esbuild has a bug even though the real problem is that the JavaScript environment itself is broken. This can happen when using the test framework called [Jest](https://jestjs.io/). With this release, esbuild's API will now throw earlier when it detects that the environment is unable to encode UTF-8 text correctly with an error message that makes it more clear that this is not a problem with esbuild.

* Change the default "legal comment" behavior

    The legal comments feature automatically gathers comments containing `@license` or `@preserve` and puts the comments somewhere (either in the generated code or in a separate file). People sometimes want this to happen so that the their dependencies' software licenses are retained in the generated output code. By default esbuild puts these comments at the end of the file when bundling. However, people sometimes find this confusing because these comments can be very generic and may not mention which library they come from. So with this release, esbuild will now discard legal comments by default. You now have to opt-in to preserving them if you want this behavior.

* Enable the `module` condition by default ([#2417](https://github.com/evanw/esbuild/issues/2417))

    Package authors want to be able to use the new [`exports`](https://nodejs.org/api/packages.html#conditional-exports) field in `package.json` to provide tree-shakable ESM code for ESM-aware bundlers while simultaneously providing fallback CommonJS code for other cases.

    Node's proposed way to do this involves using the `import` and `require` export conditions so that you get the ESM code if you use an import statement and the CommonJS code if you use a require call. However, this has a major drawback: if some code in the bundle uses an import statement and other code in the bundle uses a require call, then you'll get two copies of the same package in the bundle. This is known as the [dual package hazard](https://nodejs.org/api/packages.html#dual-package-hazard) and can lead to bloated bundles or even worse to subtle logic bugs.

    Webpack supports an alternate solution: an export condition called `module` that takes effect regardless of whether the package was imported using an import statement or a require call. This works because bundlers such as Webpack support importing a ESM using a require call (something node doesn't support). You could already do this with esbuild using `--conditions=module` but you previously had to explicitly enable this. Package authors are concerned that esbuild users won't know to do this and will get suboptimal output with their package, so they have requested for esbuild to do this automatically.

    So with this release, esbuild will now automatically add the `module` condition when there aren't any custom `conditions` already configured. You can disable this with `--conditions=` or `conditions: []` (i.e. explicitly clearing all custom conditions).

* Rename the `master` branch to `main`

    The primary branch for this repository was previously called `master` but is now called `main`. This change mirrors a similar change in many other projects.

* Remove esbuild's `_exit(0)` hack for WebAssembly ([#714](https://github.com/evanw/esbuild/issues/714))

    Node had an unfortunate bug where the node process is unnecessarily kept open while a WebAssembly module is being optimized: https://github.com/nodejs/node/issues/36616. This means cases where running `esbuild` should take a few milliseconds can end up taking many seconds instead.

    The workaround was to force node to exit by ending the process early. This was done by esbuild in one of two ways depending on the exit code. For non-zero exit codes (i.e. when there is a build error), the `esbuild` command could just call `process.kill(process.pid)` to avoid the hang. But for zero exit codes, esbuild had to load a N-API native node extension that calls the operating system's `exit(0)` function.

    However, this problem has essentially been fixed in node starting with version 18.3.0. So I have removed this hack from esbuild. If you are using an earlier version of node with `esbuild-wasm` and you don't want the `esbuild` command to hang for a while when exiting, you can upgrade to node 18.3.0 or higher to remove the hang.

    The fix came from a V8 upgrade: [this commit](https://github.com/v8/v8/commit/bfe12807c14c91714c7db1485e6b265439375e16) enabled [dynamic tiering for WebAssembly](https://v8.dev/blog/wasm-dynamic-tiering) by default for all projects that use V8's WebAssembly implementation. Previously all functions in the WebAssembly module were optimized in a single batch job but with dynamic tiering, V8 now optimizes individual WebAssembly functions as needed. This avoids unnecessary WebAssembly compilation which allows node to exit on time.

## 0.15.18

* Performance improvements for both JS and CSS

    This release brings noticeable performance improvements for JS parsing and for CSS parsing and printing. Here's an example benchmark for using esbuild to pretty-print a single large minified CSS file and JS file:

    | Test case      | Previous release | This release       |
    |----------------|------------------|--------------------|
    | 4.8mb CSS file | 19ms             | 11ms (1.7x faster) |
    | 5.8mb JS file  | 36ms             | 32ms (1.1x faster) |

    The performance improvements were very straightforward:

    * Identifiers were being scanned using a generic character advancement function instead of using custom inline code. Advancing past each character involved UTF-8 decoding as well as updating multiple member variables. This was sped up using loop that skips UTF-8 decoding entirely and that only updates member variables once at the end. This is faster because identifiers are plain ASCII in the vast majority of cases, so Unicode decoding is almost always unnecessary.

    * CSS identifiers and CSS strings were still being printed one character at a time. Apparently I forgot to move this part of esbuild's CSS infrastructure beyond the proof-of-concept stage. These were both very obvious in the profiler, so I think maybe I have just never profiled esbuild's CSS printing before?

    * There was unnecessary work being done that was related to source maps when source map output was disabled. I likely haven't observed this before because esbuild's benchmarks always have source maps enabled. This work is now disabled when it's not going to be used.

    I definitely should have caught these performance issues earlier. Better late than never I suppose.

## 0.15.17

* Search for missing source map code on the file system ([#2711](https://github.com/evanw/esbuild/issues/2711))

    [Source maps](https://sourcemaps.info/spec.html) are JSON files that map from compiled code back to the original code. They provide the original source code using two arrays: `sources` (required) and `sourcesContent` (optional). When bundling is enabled, esbuild is able to bundle code with source maps that was compiled by other tools (e.g. with Webpack) and emit source maps that map all the way back to the original code (e.g. before Webpack compiled it).

    Previously if the input source maps omitted the optional `sourcesContent` array, esbuild would use `null` for the source content in the source map that it generates (since the source content isn't available). However, sometimes the original source code is actually still present on the file system. With this release, esbuild will now try to find the original source code using the path in the `sources` array and will use that instead of `null` if it was found.

* Fix parsing bug with TypeScript `infer` and `extends` ([#2712](https://github.com/evanw/esbuild/issues/2712))

    This release fixes a bug where esbuild incorrectly failed to parse valid TypeScript code that nests `extends` inside `infer` inside `extends`, such as in the example below:

    ```ts
    type A<T> = {};
    type B = {} extends infer T extends {} ? A<T> : never;
    ```

    TypeScript code that does this should now be parsed correctly.

* Use `WebAssembly.instantiateStreaming` if available ([#1036](https://github.com/evanw/esbuild/pull/1036), [#1900](https://github.com/evanw/esbuild/pull/1900))

    Currently the WebAssembly version of esbuild uses `fetch` to download `esbuild.wasm` and then `WebAssembly.instantiate` to compile it. There is a newer API called `WebAssembly.instantiateStreaming` that both downloads and compiles at the same time, which can be a performance improvement if both downloading and compiling are slow. With this release, esbuild now attempts to use `WebAssembly.instantiateStreaming` and falls back to the original approach if that fails.

    The implementation for this builds on a PR by [@lbwa](https://github.com/lbwa).

* Preserve Webpack comments inside constructor calls ([#2439](https://github.com/evanw/esbuild/issues/2439))

    This improves the use of esbuild as a faster TypeScript-to-JavaScript frontend for Webpack, which has special [magic comments](https://webpack.js.org/api/module-methods/#magic-comments) inside `new Worker()` expressions that affect Webpack's behavior.

## 0.15.16

* Add a package alias feature ([#2191](https://github.com/evanw/esbuild/issues/2191))

    With this release, you can now easily substitute one package for another at build time with the new `alias` feature. For example, `--alias:oldpkg=newpkg` replaces all imports of `oldpkg` with `newpkg`. One use case for this is easily replacing a node-only package with a browser-friendly package in 3rd-party code that you don't control. These new substitutions happen first before all of esbuild's existing path resolution logic.

    Note that when an import path is substituted using an alias, the resulting import path is resolved in the working directory instead of in the directory containing the source file with the import path. If needed, the working directory can be set with the `cd` command when using the CLI or with the `absWorkingDir` setting when using the JS or Go APIs.

* Fix crash when pretty-printing minified JSX with object spread of object literal with computed property ([#2697](https://github.com/evanw/esbuild/issues/2697))

    JSX elements are translated to JavaScript function calls and JSX element attributes are translated to properties on a JavaScript object literal. These properties are always either strings (e.g. in `<x y />`, `y` is a string) or an object spread (e.g. in `<x {...y} />`, `y` is an object spread) because JSX doesn't provide syntax for directly passing a computed property as a JSX attribute. However, esbuild's minifier has a rule that tries to inline object spread with an inline object literal in JavaScript. For example, `x = { ...{ y } }` is minified to `x={y}` when minification is enabled. This means that there is a way to generate a non-string non-spread JSX attribute in esbuild's internal representation. One example is with `<x {...{ [y]: z }} />`. When minification is enabled, esbuild's internal representation of this is something like `<x [y]={z} />` due to object spread inlining, which is not valid JSX syntax. If this internal representation is then pretty-printed as JSX using `--minify --jsx=preserve`, esbuild previously crashed when trying to print this invalid syntax. With this release, esbuild will now print `<x {...{[y]:z}}/>` in this scenario instead of crashing.

## 0.15.15

* Remove duplicate CSS rules across files ([#2688](https://github.com/evanw/esbuild/issues/2688))

    When two or more CSS rules are exactly the same (even if they are not adjacent), all but the last one can safely be removed:

    ```css
    /* Before */
    a { color: red; }
    span { font-weight: bold; }
    a { color: red; }

    /* After */
    span { font-weight: bold; }
    a { color: red; }
    ```

    Previously esbuild only did this transformation within a single source file. But with this release, esbuild will now do this transformation across source files, which may lead to smaller CSS output if the same rules are repeated across multiple CSS source files in the same bundle. This transformation is only enabled when minifying (specifically when syntax minification is enabled).

* Add `deno` as a valid value for `target` ([#2686](https://github.com/evanw/esbuild/issues/2686))

    The `target` setting in esbuild allows you to enable or disable JavaScript syntax features for a given version of a set of target JavaScript VMs. Previously [Deno](https://deno.land/) was not one of the JavaScript VMs that esbuild supported with `target`, but it will now be supported starting from this release. For example, versions of Deno older than v1.2 don't support the new `||=` operator, so adding e.g. `--target=deno1.0` to esbuild now lets you tell esbuild to transpile `||=` to older JavaScript.

* Fix the `esbuild-wasm` package in Node v19 ([#2683](https://github.com/evanw/esbuild/issues/2683))

    A recent change to Node v19 added a non-writable `crypto` property to the global object: https://github.com/nodejs/node/pull/44897. This conflicts with Go's WebAssembly shim code, which overwrites the global `crypto` property. As a result, all Go-based WebAssembly code that uses the built-in shim (including esbuild) is now broken on Node v19. This release of esbuild fixes the issue by reconfiguring the global `crypto` property to be writable before invoking Go's WebAssembly shim code.

* Fix CSS dimension printing exponent confusion edge case ([#2677](https://github.com/evanw/esbuild/issues/2677))

    In CSS, a dimension token has a numeric "value" part and an identifier "unit" part. For example, the dimension token `32px` has a value of `32` and a unit of `px`. The unit can be any valid CSS identifier. The value can be any number in floating-point format including an optional exponent (e.g. `-3.14e-0` has an exponent of `e-0`). The full details of this syntax are here: https://www.w3.org/TR/css-syntax-3/.

    To maintain the integrity of the dimension token through the printing process, esbuild must handle the edge case where the unit looks like an exponent. One such case is the dimension `1e\32` which has the value `1` and the unit `e2`. It would be bad if this dimension token was printed such that a CSS parser would parse it as a number token with the value `1e2` instead of a dimension token. The way esbuild currently does this is to escape the leading `e` in the dimension unit, so esbuild would parse `1e\32` but print `1\65 2` (both `1e\32` and `1\65 2` represent a dimension token with a value of `1` and a unit of `e2`).

    However, there is an even narrower edge case regarding this edge case. If the value part of the dimension token itself has an `e`, then it's not necessary to escape the `e` in the dimension unit because a CSS parser won't confuse the unit with the exponent even though it looks like one (since a number can only have at most one exponent). This came up because the grammar for the CSS `unicode-range` property uses a hack that lets you specify a hexadecimal range without quotes even though CSS has no token for a hexadecimal range. The hack is to allow the hexadecimal range to be parsed as a dimension token and optionally also a number token. Here is the grammar for `unicode-range`:

    ```
    unicode-range =
      <urange>#

    <urange> =
      u '+' <ident-token> '?'*            |
      u <dimension-token> '?'*            |
      u <number-token> '?'*               |
      u <number-token> <dimension-token>  |
      u <number-token> <number-token>     |
      u '+' '?'+
    ```

    and here is an example `unicode-range` declaration that was problematic for esbuild:

    ```css
    @font-face {
      unicode-range: U+0e2e-0e2f;
    }
    ```

    This is parsed as a dimension with a value of `+0e2` and a unit of `e-0e2f`. This was problematic for esbuild because the unit starts with `e-0` which could be confused with an exponent when appended after a number, so esbuild was escaping the `e` character in the unit. However, this escaping is unnecessary because in this case the dimension value already has an exponent in it. With this release, esbuild will no longer unnecessarily escape the `e` in the dimension unit in these cases, which should fix the printing of `unicode-range` declarations.

    An aside: You may be wondering why esbuild is trying to escape the `e` at all and why it doesn't just pass through the original source code unmodified. The reason why esbuild does this is that, for robustness, esbuild's AST generally tries to omit semantically-unrelated information and esbuild's code printers always try to preserve the semantics of the underlying AST. That way the rest of esbuild's internals can just deal with semantics instead of presentation. They don't have to think about how the AST will be printed when changing the AST. This is the same reason that esbuild's JavaScript AST doesn't have a "parentheses" node (e.g. `a * (b + c)` is represented by the AST `multiply(a, add(b, c))` instead of `multiply(a, parentheses(add(b, c)))`). Instead, the printer automatically inserts parentheses as necessary to maintain the semantics of the AST, which means all of the optimizations that run over the AST don't have to worry about keeping the parentheses up to date. Similarly, the CSS AST for the dimension token stores the actual unit and the printer makes sure the unit is properly escaped depending on what value it's placed after. All of the other code operating on CSS ASTs doesn't have to worry about parsing escapes to compare units or about keeping escapes up to date when the AST is modified. Hopefully that makes sense.

* Attempt to avoid creating the `node_modules/.cache` directory for people that use Yarn 2+ in Plug'n'Play mode ([#2685](https://github.com/evanw/esbuild/issues/2685))

    When Yarn's PnP mode is enabled, packages installed by Yarn may or may not be put inside `.zip` files. The specific heuristics for when this happens change over time in between Yarn versions. This is problematic for esbuild because esbuild's JavaScript package needs to execute a binary file inside the package. Yarn makes extensive modifications to Node's file system APIs at run time to pretend that `.zip` files are normal directories and to make it hard to tell whether a file is real or not (since in theory it doesn't matter). But they haven't modified Node's `child_process.execFileSync` API so attempting to execute a file inside a zip file fails. To get around this, esbuild previously used Node's file system APIs to copy the binary executable to another location before invoking `execFileSync`. Under the hood this caused Yarn to extract the file from the zip file into a real file that can then be run.

    However, esbuild copied its executable into `node_modules/.cache/esbuild`. This is the [official recommendation from the Yarn team](https://yarnpkg.com/advanced/rulebook/#packages-should-never-write-inside-their-own-folder-outside-of-postinstall) for where packages are supposed to put these types of files when Yarn PnP is being used. However, users of Yarn PnP with esbuild find this really annoying because they don't like looking at the `node_modules` directory. With this release, esbuild now sets `"preferUnplugged": true` in its `package.json` files, which tells newer versions of Yarn to not put esbuild's packages in a zip file. There may exist older versions of Yarn that don't support `preferUnplugged`. In that case esbuild should still copy the executable to a cache directory, so it should still run (hopefully, since I haven't tested this myself). Note that esbuild setting `"preferUnplugged": true` may have the side effect of esbuild taking up more space on the file system in the event that multiple platforms are installed simultaneously, or that you're using an older version of Yarn that always installs packages for all platforms. In that case you may want to update to a newer version of Yarn since Yarn has recently changed to only install packages for the current platform.

## 0.15.14

* Fix parsing of TypeScript `infer` inside a conditional `extends` ([#2675](https://github.com/evanw/esbuild/issues/2675))

    Unlike JavaScript, parsing TypeScript sometimes requires backtracking. The `infer A` type operator can take an optional constraint of the form `infer A extends B`. However, this syntax conflicts with the similar conditional type operator `A extends B ? C : D` in cases where the syntax is combined, such as `infer A extends B ? C : D`. This is supposed to be parsed as `(infer A) extends B ? C : D`. Previously esbuild incorrectly parsed this as `(infer A extends B) ? C : D` instead, which is a parse error since the `?:` conditional operator requires the `extends` keyword as part of the conditional type. TypeScript disambiguates by speculatively parsing the `extends` after the `infer`, but backtracking if a `?` token is encountered afterward. With this release, esbuild should now do the same thing, so esbuild should now correctly parse these types. Here's a real-world example of such a type:

    ```ts
    type Normalized<T> = T extends Array<infer A extends object ? infer A : never>
      ? Dictionary<Normalized<A>>
      : {
          [P in keyof T]: T[P] extends Array<infer A extends object ? infer A : never>
            ? Dictionary<Normalized<A>>
            : Normalized<T[P]>
        }
    ```

* Avoid unnecessary watch mode rebuilds when debug logging is enabled ([#2661](https://github.com/evanw/esbuild/issues/2661))

    When debug-level logs are enabled (such as with `--log-level=debug`), esbuild's path resolution subsystem generates debug log messages that say something like "Read 20 entries for directory /home/user" to help you debug what esbuild's path resolution is doing. This caused esbuild's watch mode subsystem to add a dependency on the full list of entries in that directory since if that changes, the generated log message would also have to be updated. However, meant that on systems where a parent directory undergoes constant directory entry churn, esbuild's watch mode would continue to rebuild if `--log-level=debug` was passed.

    With this release, these debug log messages are now generated by "peeking" at the file system state while bypassing esbuild's watch mode dependency tracking. So now watch mode doesn't consider the count of directory entries in these debug log messages to be a part of the build that needs to be kept up to date when the file system state changes.

## 0.15.13

* Add support for the TypeScript 4.9 `satisfies` operator ([#2509](https://github.com/evanw/esbuild/pull/2509))

    TypeScript 4.9 introduces a new operator called `satisfies` that lets you check that a given value satisfies a less specific type without casting it to that less specific type and without generating any additional code at run-time. It looks like this:

    ```ts
    const value = { foo: 1, bar: false } satisfies Record<string, number | boolean>
    console.log(value.foo.toFixed(1)) // TypeScript knows that "foo" is a number here
    ```

    Before this existed, you could use a cast with `as` to check that a value satisfies a less specific type, but that removes any additional knowledge that TypeScript has about that specific value:

    ```ts
    const value = { foo: 1, bar: false } as Record<string, number | boolean>
    console.log(value.foo.toFixed(1)) // TypeScript no longer knows that "foo" is a number
    ```

    You can read more about this feature in [TypeScript's blog post for 4.9](https://devblogs.microsoft.com/typescript/announcing-typescript-4-9-rc/#the-satisfies-operator) as well as [the associated TypeScript issue for this feature](https://github.com/microsoft/TypeScript/issues/47920).

    This feature was implemented in esbuild by [@magic-akari](https://github.com/magic-akari).

* Fix watch mode constantly rebuilding if the parent directory is inaccessible ([#2640](https://github.com/evanw/esbuild/issues/2640))

    Android is unusual in that it has an inaccessible directory in the path to the root, which esbuild was not originally built to handle. To handle cases like this, the path resolution layer in esbuild has a hack where it treats inaccessible directories as empty. However, esbuild's watch implementation currently triggers a rebuild if a directory previously encountered an error but the directory now exists. The assumption is that the previous error was caused by the directory not existing. Although that's usually the case, it's not the case for this particular parent directory on Android. Instead the error is that the directory previously existed but was inaccessible.

    This discrepancy between esbuild's path resolution layer and its watch mode was causing watch mode to rebuild continuously on Android. With this release, esbuild's watch mode instead checks for an error status change in the `readdir` file system call, so watch mode should no longer rebuild continuously on Android.

* Apply a fix for a rare deadlock with the JavaScript API ([#1842](https://github.com/evanw/esbuild/issues/1842), [#2485](https://github.com/evanw/esbuild/issues/2485))

    There have been reports of esbuild sometimes exiting with an "all goroutines are asleep" deadlock message from the Go language runtime. This issue hasn't made much progress until recently, where a possible cause was discovered (thanks to [@jfirebaugh](https://github.com/jfirebaugh) for the investigation). This release contains a possible fix for that possible cause, so this deadlock may have been fixed. The fix cannot be easily verified because the deadlock is non-deterministic and rare. If this was indeed the cause, then this issue only affected the JavaScript API in situations where esbuild was already in the process of exiting.

    In detail: The underlying cause is that Go's [`sync.WaitGroup`](https://pkg.go.dev/sync#WaitGroup) API for waiting for a set of goroutines to finish is not fully thread-safe. Specifically it's not safe to call `Add()` concurrently with `Wait()` when the wait group counter is zero due to a data race. This situation could come up with esbuild's JavaScript API when the host JavaScript process closes the child process's stdin and the child process (with no active tasks) calls `Wait()` to check that there are no active tasks, at the same time as esbuild's watchdog timer calls `Add()` to add an active task (that pings the host to see if it's still there). The fix in this release is to avoid calling `Add()` once we learn that stdin has been closed but before we call `Wait()`.

## 0.15.12

* Fix minifier correctness bug with single-use substitutions ([#2619](https://github.com/evanw/esbuild/issues/2619))

    When minification is enabled, esbuild will attempt to eliminate variables that are only used once in certain cases. For example, esbuild minifies this code:

    ```js
    function getEmailForUser(name) {
      let users = db.table('users');
      let user = users.find({ name });
      let email = user?.get('email');
      return email;
    }
    ```

    into this code:

    ```js
    function getEmailForUser(e){return db.table("users").find({name:e})?.get("email")}
    ```

    However, this transformation had a bug where esbuild did not correctly consider the "read" part of binary read-modify-write assignment operators. For example, it's incorrect to minify the following code into `bar += fn()` because the call to `fn()` might modify `bar`:

    ```js
    const foo = fn();
    bar += foo;
    ```

    In addition to fixing this correctness bug, this release also improves esbuild's output in the case where all values being skipped over are primitives:

    ```js
    function toneMapLuminance(r, g, b) {
      let hdr = luminance(r, g, b)
      let decay = 1 / (1 + hdr)
      return 1 - decay
    }
    ```

    Previous releases of esbuild didn't substitute these single-use variables here, but esbuild will now minify this to the following code starting with this release:

    ```js
    function toneMapLuminance(e,n,a){return 1-1/(1+luminance(e,n,a))}
    ```

## 0.15.11

* Fix various edge cases regarding template tags and `this` ([#2610](https://github.com/evanw/esbuild/issues/2610))

    This release fixes some bugs where the value of `this` wasn't correctly preserved when evaluating template tags in a few edge cases. These edge cases are listed below:

    ```js
    async function test() {
      class Foo { foo() { return this } }
      class Bar extends Foo {
        a = async () => super.foo``
        b = async () => super['foo']``
        c = async (foo) => super[foo]``
      }
      function foo() { return this }
      const obj = { foo }
      const bar = new Bar
      console.log(
        (await bar.a()) === bar,
        (await bar.b()) === bar,
        (await bar.c('foo')) === bar,
        { foo }.foo``.foo === foo,
        (true && obj.foo)`` !== obj,
        (false || obj.foo)`` !== obj,
        (null ?? obj.foo)`` !== obj,
      )
    }
    test()
    ```

    Each edge case in the code above previously incorrectly printed `false` when run through esbuild with `--minify --target=es6` but now correctly prints `true`. These edge cases are unlikely to have affected real-world code.

## 0.15.10

* Add support for node's "pattern trailers" syntax ([#2569](https://github.com/evanw/esbuild/issues/2569))

    After esbuild implemented node's `exports` feature in `package.json`, node changed the feature to also allow text after `*` wildcards in patterns. Previously the `*` was required to be at the end of the pattern. It lets you do something like this:

    ```json
    {
      "exports": {
        "./features/*": "./features/*.js",
        "./features/*.js": "./features/*.js"
      }
    }
    ```

    With this release, esbuild now supports these types of patterns too.

* Fix subpath imports with Yarn PnP ([#2545](https://github.com/evanw/esbuild/issues/2545))

    Node has a little-used feature called [subpath imports](https://nodejs.org/api/packages.html#subpath-imports) which are package-internal imports that start with `#` and that go through the `imports` map in `package.json`. Previously esbuild had a bug that caused esbuild to not handle these correctly in packages installed via Yarn's "Plug'n'Play" installation strategy. The problem was that subpath imports were being checked after Yarn PnP instead of before. This release reorders these checks, which should allow subpath imports to work in this case.

* Link from JS to CSS in the metafile ([#1861](https://github.com/evanw/esbuild/issues/1861), [#2565](https://github.com/evanw/esbuild/issues/2565))

    When you import CSS into a bundled JS file, esbuild creates a parallel CSS bundle next to your JS bundle. So if `app.ts` imports some CSS files and you bundle it, esbuild will give you `app.js` and `app.css`. You would then add both `<script src="app.js"></script>` and `<link href="app.css" rel="stylesheet">` to your HTML to include everything in the page. This approach is more efficient than having esbuild insert additional JavaScript into `app.js` that downloads and includes `app.css` because it means the browser can download and parse both the CSS and the JS in parallel (and potentially apply the CSS before the JS has even finished downloading).

    However, sometimes it's difficult to generate the `<link>` tag. One case is when you've added `[hash]` to the [entry names](https://esbuild.github.io/api/#entry-names) setting to include a content hash in the file name. Then the file name will look something like `app-GX7G2SBE.css` and may change across subsequent builds. You can tell esbuild to generate build metadata using the `metafile` API option but the metadata only tells you which generated JS bundle corresponds to a JS entry point (via the `entryPoint` property), not which file corresponds to the associated CSS bundle. Working around this was hacky and involved string manipulation.

    This release adds the `cssBundle` property to the metafile to make this easier. It's present on the metadata for the generated JS bundle and points to the associated CSS bundle. So to generate the HTML tags for a given JS entry point, you first find the output file with the `entryPoint` you are looking for (and put that in a `<script>` tag), then check for the `cssBundle` property to find the associated CSS bundle (and put that in a `<link>` tag).

    One thing to note is that there is deliberately no `jsBundle` property mapping the other way because it's not a 1:1 relationship. Two JS bundles can share the same CSS bundle in the case where the associated CSS bundles have the same name and content. In that case there would be no one value for a hypothetical `jsBundle` property to have.

## 0.15.9

* Fix an obscure npm package installation issue with `--omit=optional` ([#2558](https://github.com/evanw/esbuild/issues/2558))

    The previous release introduced a regression with `npm install esbuild --omit=optional` where the file `node_modules/.bin/esbuild` would no longer be present after installation. That could cause any package scripts which used the `esbuild` command to no longer work. This release fixes the regression so `node_modules/.bin/esbuild` should now be present again after installation. This regression only affected people installing esbuild using `npm` with either the `--omit=optional` or `--no-optional` flag, which is a somewhat unusual situation.

    **More details:**

    The reason for this regression is due to some obscure npm implementation details. Since the Go compiler doesn't support trivial cross-compiling on certain Android platforms, esbuild's installer installs a WebAssembly shim on those platforms instead. In the previous release I attempted to simplify esbuild's WebAssembly shims to depend on the `esbuild-wasm` package instead of including another whole copy of the WebAssembly binary (to make publishing faster and to save on file system space after installation). However, both the `esbuild` package and the `esbuild-wasm` package provide a binary called `esbuild` and it turns out that adding `esbuild-wasm` as a nested dependency of the `esbuild` package (specifically `esbuild` optionally depends on `@esbuild/android-arm` which depends on `esbuild-wasm`) caused npm to be confused about what `node_modules/.bin/esbuild` is supposed to be.

    It's pretty strange and unexpected that disabling the installation of optional dependencies altogether would suddenly cause an optional dependency's dependency to conflict with the top-level package. What happens under the hood is that if `--omit=optional` is present, npm attempts to uninstall the `esbuild-wasm` nested dependency at the end of `npm install` (even though the `esbuild-wasm` package was never installed due to `--omit=optional`). This uninstallation causes `node_modules/.bin/esbuild` to be deleted.

    After doing a full investigation, I discovered that npm's handling of the `.bin` directory is deliberately very brittle. When multiple packages in the dependency tree put something in `.bin` with the same name, the end result is non-deterministic/random. What you get in `.bin` might be from one package, from the other package, or might be missing entirely. The workaround suggested by npm is to just avoid having two packages that put something in `.bin` with the same name. So this was fixed by making the `@esbuild/android-arm` and `esbuild-android-64` packages each include another whole copy of the WebAssembly binary, which works because these packages don't put anything in `.bin`.

## 0.15.8

* Fix JSX name collision edge case ([#2534](https://github.com/evanw/esbuild/issues/2534))

    Code generated by esbuild could have a name collision in the following edge case:

    * The JSX transformation mode is set to `automatic`, which causes `import` statements to be inserted
    * An element uses a `{...spread}` followed by a `key={...}`, which uses the legacy `createElement` fallback imported from `react`
    * Another import uses a name that ends with `react` such as `@remix-run/react`
    * The output format has been set to CommonJS so that `import` statements are converted into require calls

    In this case, esbuild previously generated two variables with the same name `import_react`, like this:

    ```js
    var import_react = require("react");
    var import_react2 = require("@remix-run/react");
    ```

    That bug is fixed in this release. The code generated by esbuild no longer contains a name collision.

* Fall back to WebAssembly on Android ARM ([#1556](https://github.com/evanw/esbuild/issues/1556), [#1578](https://github.com/evanw/esbuild/issues/1578), [#2335](https://github.com/evanw/esbuild/issues/2335), [#2526](https://github.com/evanw/esbuild/issues/2526))

    Go's compiler supports trivial cross-compiling to almost all platforms without installing any additional software other than the Go compiler itself. This has made it very easy for esbuild to publish native binary executables for many platforms. However, it strangely doesn't support cross-compiling to Android ARM without installing the Android build tools.

    So instead of publishing a native esbuild binary executable to npm, this release publishes a WebAssembly fallback build. This is essentially the same as the `esbuild-wasm` package but it's installed automatically when you install the `esbuild` package on Android ARM. So packages that depend on the `esbuild` package should now work on Android ARM. This change has not yet been tested end-to-end because I don't have a 32-bit Android ARM device myself, but in theory it should work.

    This inherits the drawbacks of WebAssembly including significantly slower performance than native as well as potentially also more severe memory usage limitations and lack of certain features (e.g. `--serve`). If you want to use a native binary executable of esbuild on Android ARM, you may be able to build it yourself from source after installing the Android build tools.

* Attempt to better support Yarn's `ignorePatternData` feature ([#2495](https://github.com/evanw/esbuild/issues/2495))

    Part of resolving paths in a project using Yarn's Plug'n'Play feature involves evaluating a regular expression in the `ignorePatternData` property of `.pnp.data.json`. However, it turns out that the particular regular expressions generated by Yarn use some syntax that works with JavaScript regular expressions but that does not work with Go regular expressions.

    In this release, esbuild will now strip some of the the problematic syntax from the regular expression before compiling it, which should hopefully allow it to be compiled by Go's regular expression engine. The specific character sequences that esbuild currently strips are as follows:

    * `(?!\.)`
    * `(?!(?:^|\/)\.)`
    * `(?!\.{1,2}(?:\/|$))`
    * `(?!(?:^|\/)\.{1,2}(?:\/|$))`

    These seem to be used by Yarn to avoid the `.` and `..` path segments in the middle of relative paths. The removal of these character sequences seems relatively harmless in this case since esbuild shouldn't ever generate such path segments. This change should add support to esbuild for Yarn's [`pnpIgnorePatterns`](https://yarnpkg.com/configuration/yarnrc/#pnpIgnorePatterns) feature.

* Fix non-determinism issue with legacy block-level function declarations and strict mode ([#2537](https://github.com/evanw/esbuild/issues/2537))

    When function declaration statements are nested inside a block in strict mode, they are supposed to only be available within that block's scope. But in "sloppy mode" (which is what non-strict mode is commonly called), they are supposed to be available within the whole function's scope:

    ```js
    // This returns 1 due to strict mode
    function test1() {
      'use strict'
      function fn() { return 1 }
      if (true) { function fn() { return 2 } }
      return fn()
    }

    // This returns 2 due to sloppy mode
    function test2() {
      function fn() { return 1 }
      if (true) { function fn() { return 2 } }
      return fn()
    }
    ```

    To implement this, esbuild compiles these two functions differently to reflect their different semantics:

    ```js
    function test1() {
      "use strict";
      function fn() {
        return 1;
      }
      if (true) {
        let fn2 = function() {
          return 2;
        };
      }
      return fn();
    }
    function test2() {
      function fn() {
        return 1;
      }
      if (true) {
        let fn2 = function() {
          return 2;
        };
        var fn = fn2;
      }
      return fn();
    }
    ```

    However, the compilation had a subtle bug where the automatically-generated function-level symbols for multible hoisted block-level function declarations in the same block a sloppy-mode context were generated in a random order if the output was in strict mode, which could be the case if TypeScript's `alwaysStrict` setting was set to true. This lead to non-determinism in the output as the minifier would randomly exchange the generated names for these symbols on different runs. This bug has been fixed by sorting the keys of the unordered map before iterating over them.

* Fix parsing of `@keyframes` with string identifiers ([#2555](https://github.com/evanw/esbuild/issues/2555))

    Firefox supports `@keyframes` with string identifier names. Previously this was treated as a syntax error by esbuild as it doesn't work in any other browser. The specification allows for this however, so it's technically not a syntax error (even though it would be unwise to use this feature at the moment). There was also a bug where esbuild would remove the identifier name in this case as the syntax wasn't recognized.

    This release changes esbuild's parsing of `@keyframes` to now consider this case to be an unrecognized CSS rule. That means it will be passed through unmodified (so you can now use esbuild to bundle this Firefox-specific CSS) but the CSS will not be pretty-printed or minified. I don't think it makes sense for esbuild to have special code to handle this Firefox-specific syntax at this time. This decision can be revisited in the future if other browsers add support for this feature.

* Add the `--jsx-side-effects` API option ([#2539](https://github.com/evanw/esbuild/issues/2539), [#2546](https://github.com/evanw/esbuild/pull/2546))

    By default esbuild assumes that JSX expressions are side-effect free, which means they are annoated with `/* @__PURE__ */` comments and are removed during bundling when they are unused. This follows the common use of JSX for virtual DOM and applies to the vast majority of JSX libraries. However, some people have written JSX libraries that don't have this property. JSX expressions can have arbitrary side effects and can't be removed. If you are using such a library, you can now pass `--jsx-side-effects` to tell esbuild that JSX expressions have side effects so it won't remove them when they are unused.

    This feature was contributed by [@rtsao](https://github.com/rtsao).

## 0.15.7

* Add `--watch=forever` to allow esbuild to never terminate ([#1511](https://github.com/evanw/esbuild/issues/1511), [#1885](https://github.com/evanw/esbuild/issues/1885))

    Currently using esbuild's watch mode via `--watch` from the CLI will stop watching if stdin is closed. The rationale is that stdin is automatically closed by the OS when the parent process exits, so stopping watch mode when stdin is closed ensures that esbuild's watch mode doesn't keep running forever after the parent process has been closed. For example, it would be bad if you wrote a shell script that did `esbuild --watch &` to run esbuild's watch mode in the background, and every time you run the script it creates a new `esbuild` process that runs forever.

    However, there are cases when it makes sense for esbuild's watch mode to never exit. One such case is within a short-lived VM where the lifetime of all processes inside the VM is expected to be the lifetime of the VM. Previously you could easily do this by piping the output of a long-lived command into esbuild's stdin such as `sleep 999999999 | esbuild --watch &`. However, this possibility often doesn't occur to people, and it also doesn't work on Windows. People also sometimes attempt to keep esbuild open by piping an infinite stream of data to esbuild such as with `esbuild --watch </dev/zero &` which causes esbuild to spin at 100% CPU. So with this release, esbuild now has a `--watch=forever` flag that will not stop watch mode when stdin is closed.

* Work around `PATH` without `node` in install script ([#2519](https://github.com/evanw/esbuild/issues/2519))

    Some people install esbuild's npm package in an environment without the `node` command in their `PATH`. This fails on Windows because esbuild's install script runs the `esbuild` command before exiting as a sanity check, and on Windows the `esbuild` command has to be a JavaScript file because of some internal details about how npm handles the `bin` folder (specifically the `esbuild` command lacks the `.exe` extension, which is required on Windows). This release attempts to work around this problem by using `process.execPath` instead of `"node"` as the command for running node. In theory this means the installer can now still function on Windows if something is wrong with `PATH`.

## 0.15.6

* Lower `for await` loops ([#1930](https://github.com/evanw/esbuild/issues/1930))

    This release lowers `for await` loops to the equivalent `for` loop containing `await` when esbuild is configured such that `for await` loops are unsupported. This transform still requires at least generator functions to be supported since esbuild's lowering of `await` currently relies on generators. This new transformation is mostly modeled after what the TypeScript compiler does. Here's an example:

    ```js
    async function f() {
      for await (let x of y)
        x()
    }
    ```

    The code above will now become the following code with `--target=es2017` (omitting the code for the `__forAwait` helper function):

    ```js
    async function f() {
      try {
        for (var iter = __forAwait(y), more, temp, error; more = !(temp = await iter.next()).done; more = false) {
          let x = temp.value;
          x();
        }
      } catch (temp) {
        error = [temp];
      } finally {
        try {
          more && (temp = iter.return) && await temp.call(iter);
        } finally {
          if (error)
            throw error[0];
        }
      }
    }
    ```

* Automatically fix invalid `supported` configurations ([#2497](https://github.com/evanw/esbuild/issues/2497))

    The `--target=` setting lets you tell esbuild to target a specific version of one or more JavaScript runtimes such as `chrome80,node14` and esbuild will restrict its output to only those features supported by all targeted JavaScript runtimes. More recently, esbuild introduced the `--supported:` setting that lets you override which features are supported on a per-feature basis. However, this now lets you configure nonsensical things such as `--supported:async-await=false --supported:async-generator=true`. Previously doing this could result in esbuild building successfully but producing invalid output.

    Starting with this release, esbuild will now attempt to automatically fix nonsensical feature override configurations by introducing more overrides until the configuration makes sense. So now the configuration from previous example will be changed such that `async-await=false` implies `async-generator=false`. The full list of implications that were introduced is below:

    * `async-await=false` implies:
        * `async-generator=false`
        * `for-await=false`
        * `top-level-await=false`

    * `generator=false` implies:
        * `async-generator=false`

    * `object-accessors=false` implies:
        * `class-private-accessor=false`
        * `class-private-static-accessor=false`

    * `class-field=false` implies:
        * `class-private-field=false`

    * `class-static-field=false` implies:
        * `class-private-static-field=false`

    * `class=false` implies:
        * `class-field=false`
        * `class-private-accessor=false`
        * `class-private-brand-check=false`
        * `class-private-field=false`
        * `class-private-method=false`
        * `class-private-static-accessor=false`
        * `class-private-static-field=false`
        * `class-private-static-method=false`
        * `class-static-blocks=false`
        * `class-static-field=false`

* Implement a small minification improvement ([#2496](https://github.com/evanw/esbuild/issues/2496))

    Some people write code that contains a label with an immediate break such as `x: break x`. Previously this code was not removed during minification but it will now be removed during minification starting with this release.

* Fix installing esbuild via Yarn with `enableScripts: false` configured ([#2457](https://github.com/evanw/esbuild/pull/2457))

    If esbuild is installed with Yarn with the `enableScripts: false` setting configured, then Yarn will not "unplug" the `esbuild` package (i.e. it will keep the entire package inside a `.zip` file). This messes with esbuild's library code that extracts the platform-specific binary executable because that code copies the binary executable into the esbuild package directory, and Yarn's `.zip` file system shim doesn't let you write to a directory inside of a `.zip` file. This release fixes this problem by writing to the `node_modules/.cache/esbuild` directory instead in this case. So you should now be able to use esbuild with Yarn when `enableScripts: false` is configured.

    This fix was contributed by [@jonaskuske](https://github.com/jonaskuske).

## 0.15.5

* Fix issues with Yarn PnP and Yarn's workspaces feature ([#2476](https://github.com/evanw/esbuild/issues/2476))

    This release makes sure esbuild works with a Yarn feature called [workspaces](https://yarnpkg.com/features/workspaces/). Previously esbuild wasn't tested in this scenario, but this scenario now has test coverage. Getting this to work involved further tweaks to esbuild's custom code for what happens after Yarn PnP's path resolution algorithm runs, which is not currently covered by Yarn's PnP specification. These tweaks also fix `exports` map resolution with Yarn PnP for non-empty subpaths, which wasn't previously working.

## 0.15.4

* Consider TypeScript import assignments to be side-effect free ([#2468](https://github.com/evanw/esbuild/issues/2468))

    TypeScript has a [legacy import syntax](https://www.typescriptlang.org/docs/handbook/namespaces.html#aliases) for working with TypeScript namespaces that looks like this:

    ```ts
    import { someNamespace } from './some-file'
    import bar = someNamespace.foo;

    // some-file.ts
    export namespace someNamespace {
      export let foo = 123
    }
    ```

    Since esbuild converts TypeScript into JavaScript one file at a time, it doesn't know if `bar` is supposed to be a value or a type (or both, which TypeScript actually allows in this case). This is problematic because values are supposed to be kept during the conversion but types are supposed to be removed during the conversion. Currently esbuild keeps `bar` in the output, which is done because `someNamespace.foo` is a property access and property accesses run code that could potentially have a side effect (although there is no side effect in this case).

    With this release, esbuild will now consider `someNamespace.foo` to have no side effects. This means `bar` will now be removed when bundling and when tree shaking is enabled. Note that it will still not be removed when tree shaking is disabled. This is because in this mode, esbuild supports adding additional code to the end of the generated output that's in the same scope as the module. That code could potentially make use of `bar`, so it would be incorrect to remove it. If you want `bar` to be removed, you'll have to enable tree shaking (which tells esbuild that nothing else depends on the unexported top-level symbols in the generated output).

* Change the order of the banner and the `"use strict"` directive ([#2467](https://github.com/evanw/esbuild/issues/2467))

    Previously the top of the file contained the following things in order:

    1. The hashbang comment (see below) from the source code, if present
    2. The `"use strict"` directive from the source code, if present
    3. The content of esbuild's `banner` API option, if specified

    This was problematic for people that used the `banner` API option to insert the hashbang comment instead of using esbuild's hashbang comment preservation feature. So with this release, the order has now been changed to:

    1. The hashbang comment (see below) from the source code, if present
    2. The content of esbuild's `banner` API option, if specified
    3. The `"use strict"` directive from the source code, if present

    I'm considering this change to be a bug fix instead of a breaking change because esbuild's documentation states that the `banner` API option can be used to "insert an arbitrary string at the beginning of generated JavaScript files". While this isn't technically true because esbuild may still insert the original hashbang comment before the banner, it's at least more correct now because the banner will now come before the `"use strict"` directive.

    For context: JavaScript files recently allowed using a [hashbang comment](https://github.com/tc39/proposal-hashbang), which starts with `#!` and which must start at the very first character of the file. It allows Unix systems to execute the file directly as a script without needing to prefix it by the `node` command. This comment typically has the value `#!/usr/bin/env node`. Hashbang comments will be a part of ES2023 when it's released next year.

* Fix `exports` maps with Yarn PnP path resolution ([#2473](https://github.com/evanw/esbuild/issues/2473))

    The Yarn PnP specification says that to resolve a package path, you first resolve it to the absolute path of a directory, and then you run node's module resolution algorithm on it. Previously esbuild followed this part of the specification. However, doing this means that `exports` in `package.json` is not respected because node's module resolution algorithm doesn't interpret `exports` for absolute paths. So with this release, esbuild will now use a modified algorithm that deviates from both specifications but that should hopefully behave more similar to what Yarn actually does: node's module resolution algorithm is run with the original import path but starting from the directory returned by Yarn PnP.

## 0.15.3

* Change the Yarn PnP manifest to a singleton ([#2463](https://github.com/evanw/esbuild/issues/2463))

    Previously esbuild searched for the Yarn PnP manifest in the parent directories of each file. But with Yarn's `enableGlobalCache` setting it's possible to configure Yarn PnP's implementation to reach outside of the directory subtree containing the Yarn PnP manifest. This was causing esbuild to fail to bundle projects with the `enableGlobalCache` setting enabled.

    To handle this case, *esbuild will now only search for the Yarn PnP manifest in the current working directory of the esbuild process*. If you're using esbuild's CLI, this means you will now have to `cd` into the appropriate directory first. If you're using esbuild's API, you can override esbuild's value for the current working directory with the `absWorkingDir` API option.

* Fix Yarn PnP resolution failures due to backslashes in paths on Windows ([#2462](https://github.com/evanw/esbuild/issues/2462))

    Previously dependencies of a Yarn PnP virtual dependency failed to resolve on Windows. This was because Windows uses `\` instead of `/` as a path separator, and the path manipulation algorithms used for Yarn PnP expected `/`. This release converts `\` into `/` in Windows paths, which fixes this issue.

* Fix `sideEffects` patterns containing slashes on Windows ([#2465](https://github.com/evanw/esbuild/issues/2465))

    The `sideEffects` field in `package.json` lets you specify an array of patterns to mark which files have side effects (which causes all other files to be considered to not have side effects by exclusion). That looks like this:

    ```json
    "sideEffects": [
      "**/index.js",
      "**/index.prod.js"
    ]
    ```

    However, the presence of the `/` character in the pattern meant that the pattern failed to match Windows-style paths, which broke `sideEffects` on Windows in this case. This release fixes this problem by adding additional code to handle Windows-style paths.

## 0.15.2

* Fix Yarn PnP issue with packages containing `index.js` ([#2455](https://github.com/evanw/esbuild/issues/2455), [#2461](https://github.com/evanw/esbuild/issues/2461))

    Yarn PnP's tests require the resolved paths to end in `/`. That's not how the rest of esbuild's internals work, however, and doing this messed up esbuild's node module path resolution regarding automatically-detected `index.js` files. Previously packages that relied on implicit `index.js` resolution rules didn't work with esbuild under Yarn PnP. Removing this slash has fixed esbuild's path resolution behavior regarding `index.js`, which should now the same both with and without Yarn PnP.

* Fix Yarn PnP support for `extends` in `tsconfig.json` ([#2456](https://github.com/evanw/esbuild/issues/2456))

    Previously using `extends` in `tsconfig.json` with a path in a Yarn PnP package didn't work. This is because the process of setting up package path resolution rules requires parsing `tsconfig.json` files (due to the `baseUrl` and `paths` features) and resolving `extends` to a package path requires package path resolution rules to already be set up, which is a circular dependency. This cycle is broken by using special rules for `extends` in `tsconfig.json` that bypasses esbuild's normal package path resolution process. This is why using `extends` with a Yarn PnP package didn't automatically work. With this release, these special rules have been modified to check for a Yarn PnP manifest so this case should work now.

* Fix Yarn PnP support in `esbuild-wasm` ([#2458](https://github.com/evanw/esbuild/issues/2458))

    When running esbuild via WebAssembly, Yarn PnP support previously failed because Go's file system internals return `EINVAL` when trying to read a `.zip` file as a directory when run with WebAssembly. This was unexpected because Go's file system internals return `ENOTDIR` for this case on native. This release updates esbuild to treat `EINVAL` like `ENOTDIR` in this case, which fixes using `esbuild-wasm` to bundle a Yarn PnP project.

    Note that to be able to use `esbuild-wasm` for Yarn PnP successfully, you currently have to run it using `node` instead of `yarn node`. This is because the file system shim that Yarn overwrites node's native file system API with currently generates invalid file descriptors with negative values when inside a `.zip` file. This prevents esbuild from working correctly because Go's file system internals don't expect syscalls that succeed without an error to return an invalid file descriptor. Yarn is working on fixing their use of invalid file descriptors.

## 0.15.1

* Update esbuild's Yarn Plug'n'Play implementation to match the latest specification changes ([#2452](https://github.com/evanw/esbuild/issues/2452), [#2453](https://github.com/evanw/esbuild/pull/2453))

    This release updates esbuild's implementation of Yarn Plug'n'Play to match some changes to Yarn's specification that just landed. The changes are as follows:

    * Check for platform-specific absolute paths instead of always for the `/` prefix

        The specification previously said that Yarn Plug'n'Play path resolution rules should not apply for paths that start with `/`. The intent was to avoid accidentally processing absolute paths. However, absolute paths on Windows such as `C:\project` start with drive letters instead of with `/`. So the specification was changed to instead explicitly avoid processing absolute paths.

    * Make `$$virtual` an alias for `__virtual__`

        Supporting Yarn-style path resolution requires implementing a custom Yarn-specific path traversal scheme where certain path segments are considered no-ops. Specifically any path containing segments of the form `__virtual__/<whatever>/<n>` where `<n>` is an integer must be treated as if they were `n` times the `..` operator instead (the `<whatever>` path segment is ignored). So `/path/to/project/__virtual__/xyz/2/foo.js` maps to the underlying file `/path/to/project/../../foo.js`. This scheme makes it possible for Yarn to get node (and esbuild) to load the same file multiple times (which is sometimes required for correctness) without actually duplicating the file on the file system.

        However, old versions of Yarn used to use `$$virtual` instead of `__virtual__`. This was changed because `$$virtual` was error-prone due to the use of the `$` character, which can cause bugs when it's not correctly escaped within regular expressions. Now that esbuild makes `$$virtual` an alias for `__virtual__`, esbuild should now work with manifests from these old Yarn versions.

    * Ignore PnP manifests in virtual directories

        The specification describes the algorithm for how to find the Plug'n'Play manifest when starting from a certain point in the file system: search through all parent directories in reverse order until the manifest is found. However, this interacts poorly with virtual paths since it can end up finding a virtual copy of the manifest instead of the original. To avoid this, esbuild now ignores manifests in virtual directories so that the search for the manifest will continue and find the original manifest in another parent directory later on.

    These fixes mean that esbuild's implementation of Plug'n'Play now matches Yarn's implementation more closely, and esbuild can now correctly build more projects that use Plug'n'Play.

## 0.15.0

**This release contains backwards-incompatible changes.** Since esbuild is before version 1.0.0, these changes have been released as a new minor version to reflect this (as [recommended by npm](https://docs.npmjs.com/cli/v6/using-npm/semver/)). You should either be pinning the exact version of `esbuild` in your `package.json` file or be using a version range syntax that only accepts patch upgrades such as `~0.14.0`. See the documentation about [semver](https://docs.npmjs.com/cli/v6/using-npm/semver/) for more information.

* Implement the Yarn Plug'n'Play module resolution algorithm ([#154](https://github.com/evanw/esbuild/issues/154), [#237](https://github.com/evanw/esbuild/issues/237), [#1263](https://github.com/evanw/esbuild/issues/1263), [#2451](https://github.com/evanw/esbuild/pull/2451))

    [Node](https://nodejs.org/) comes with a package manager called [npm](https://www.npmjs.com/), which installs packages into a `node_modules` folder. Node and esbuild both come with built-in rules for resolving import paths to packages within `node_modules`, so packages installed via npm work automatically without any configuration. However, many people use an alternative package manager called [Yarn](https://yarnpkg.com/). While Yarn can install packages using `node_modules`, it also offers a different package installation strategy called [Plug'n'Play](https://yarnpkg.com/features/pnp/), which is often shortened to "PnP" (not to be confused with [pnpm](https://pnpm.io/), which is an entirely different unrelated package manager).

    Plug'n'Play installs packages as `.zip` files on your file system. The packages are never actually unzipped. Since Node doesn't know anything about Yarn's package installation strategy, this means you can no longer run your code with Node as it won't be able to find your packages. Instead, you need to run your code with Yarn, which applies patches to Node's file system APIs before running your code. These patches attempt to make zip files seem like normal directories. When running under Yarn, using Node's file system API to read `./some.zip/lib/file.js` actually automatically extracts `lib/file.js` from `./some.zip` at run-time as if it was a normal file. Other file system APIs behave similarly. However, these patches don't work with esbuild because esbuild is not written in JavaScript; it's a native binary executable that interacts with the file system directly through the operating system.

    Previously the workaround for using esbuild with Plug'n'Play was to use the [`@yarnpkg/esbuild-plugin-pnp`](https://www.npmjs.com/package/@yarnpkg/esbuild-plugin-pnp) plugin with esbuild's JavaScript API. However, this wasn't great because the plugin needed to potentially intercept every single import path and file load to check whether it was a Plug'n'Play package, which has an unusually high performance cost. It also meant that certain subtleties of path resolution rules within a `.zip` file could differ slightly from the way esbuild normally works since path resolution inside `.zip` files was implemented by Yarn, not by esbuild (which is due to a limitation of esbuild's plugin API).

    With this release, esbuild now contains an independent implementation of Yarn's Plug'n'Play algorithm (which is used when esbuild finds a `.pnp.js`, `.pnp.cjs`, or `.pnp.data.json` file in the directory tree). Creating additional implementations of this algorithm recently became possible because Yarn's package manifest format was recently documented: https://yarnpkg.com/advanced/pnp-spec/. This should mean that you can now use esbuild to bundle Plug'n'Play projects without any additional configuration (so you shouldn't need `@yarnpkg/esbuild-plugin-pnp` anymore). Bundling these projects should now happen much faster as Yarn no longer even needs to be run at all. Bundling the Yarn codebase itself with esbuild before and after this change seems to demonstrate over a 10x speedup (3.4s to 0.24s). And path resolution rules within Yarn packages should now be consistent with how esbuild handles regular Node packages. For example, fields such as `module` and `browser` in `package.json` files within `.zip` files should now be respected.

    Keep in mind that this is brand new code and there may be some initial issues to work through before esbuild's implementation is solid. Yarn's Plug'n'Play specification is also brand new and may need some follow-up edits to guide new implementations to match Yarn's exact behavior. If you try this out, make sure to test it before committing to using it, and let me know if anything isn't working as expected. Should you need to debug esbuild's path resolution, you may find `--log-level=verbose` helpful.

## 0.14.54

* Fix optimizations for calls containing spread arguments ([#2445](https://github.com/evanw/esbuild/issues/2445))

    This release fixes the handling of spread arguments in the optimization of `/* @__PURE__ */` comments, empty functions, and identity functions:

    ```js
    // Original code
    function empty() {}
    function identity(x) { return x }
    /* @__PURE__ */ a(...x)
    /* @__PURE__ */ new b(...x)
    empty(...x)
    identity(...x)

    // Old output (with --minify --tree-shaking=true)
    ...x;...x;...x;...x;

    // New output (with --minify --tree-shaking=true)
    function identity(n){return n}[...x];[...x];[...x];identity(...x);
    ```

    Previously esbuild assumed arguments with side effects could be directly inlined. This is almost always true except for spread arguments, which are not syntactically valid on their own and which have the side effect of causing iteration, which might have further side effects. Now esbuild will wrap these elements in an unused array so that they are syntactically valid and so that the iteration side effects are preserved.

## 0.14.53

This release fixes a minor issue with the previous release: I had to rename the package `esbuild-linux-loong64` to `@esbuild/linux-loong64` in the contributed PR because someone registered the package name before I could claim it, and I missed a spot. Hopefully everything is working after this release. I plan to change all platform-specific package names to use the `@esbuild/` scope at some point to avoid this problem in the future.

## 0.14.52

* Allow binary data as input to the JS `transform` and `build` APIs ([#2424](https://github.com/evanw/esbuild/issues/2424))

    Previously esbuild's `transform` and `build` APIs could only take a string. However, some people want to use esbuild to convert binary data to base64 text. This is problematic because JavaScript strings represent UTF-16 text and esbuild internally operates on arrays of bytes, so all strings coming from JavaScript undergo UTF-16 to UTF-8 conversion before use. This meant that using esbuild in this way was doing base64 encoding of the UTF-8 encoding of the text, which was undesired.

    With this release, esbuild now accepts `Uint8Array` in addition to string as an input format for the `transform` and `build` APIs. Now you can use esbuild to convert binary data to base64 text:

    ```js
    // Original code
    import esbuild from 'esbuild'
    console.log([
      (await esbuild.transform('\xFF', { loader: 'base64' })).code,
      (await esbuild.build({ stdin: { contents: '\xFF', loader: 'base64' }, write: false })).outputFiles[0].text,
    ])
    console.log([
      (await esbuild.transform(new Uint8Array([0xFF]), { loader: 'base64' })).code,
      (await esbuild.build({ stdin: { contents: new Uint8Array([0xFF]), loader: 'base64' }, write: false })).outputFiles[0].text,
    ])

    // Old output
    [ 'module.exports = "w78=";\n', 'module.exports = "w78=";\n' ]
    /* ERROR: The input to "transform" must be a string */

    // New output
    [ 'module.exports = "w78=";\n', 'module.exports = "w78=";\n' ]
    [ 'module.exports = "/w==";\n', 'module.exports = "/w==";\n' ]
    ```

* Update the getter for `text` in build results ([#2423](https://github.com/evanw/esbuild/issues/2423))

    Output files in build results returned from esbuild's JavaScript API have both a `contents` and a `text` property to return the contents of the output file. The `contents` property is a binary UTF-8 Uint8Array and the `text` property is a JavaScript UTF-16 string. The `text` property is a getter that does the UTF-8 to UTF-16 conversion only if it's needed for better performance.

    Previously if you mutate the build results object, you had to overwrite both `contents` and `text` since the value returned from the `text` getter is the original text returned by esbuild. Some people find this confusing so with this release, the getter for `text` has been updated to do the UTF-8 to UTF-16 conversion on the current value of the `contents` property instead of the original value.

* Publish builds for Linux LoongArch 64-bit ([#1804](https://github.com/evanw/esbuild/issues/1804), [#2373](https://github.com/evanw/esbuild/pull/2373))

    This release upgrades to [Go 1.19](https://go.dev/doc/go1.19), which now includes support for LoongArch 64-bit processors. LoongArch 64-bit builds of esbuild will now be published to npm, which means that in theory they can now be installed with `npm install esbuild`. This was contributed by [@beyond-1234](https://github.com/beyond-1234).

## 0.14.51

* Add support for React 17's `automatic` JSX transform ([#334](https://github.com/evanw/esbuild/issues/334), [#718](https://github.com/evanw/esbuild/issues/718), [#1172](https://github.com/evanw/esbuild/issues/1172), [#2318](https://github.com/evanw/esbuild/issues/2318), [#2349](https://github.com/evanw/esbuild/pull/2349))

    This adds support for the [new "automatic" JSX runtime from React 17+](https://reactjs.org/blog/2020/09/22/introducing-the-new-jsx-transform.html) to esbuild for both the build and transform APIs.

    **New CLI flags and API options:**
    - `--jsx`, `jsx` &mdash; Set this to `"automatic"` to opt in to this new transform
    - `--jsx-dev`, `jsxDev` &mdash; Toggles development mode for the automatic runtime
    - `--jsx-import-source`, `jsxImportSource` &mdash; Overrides the root import for runtime functions (default `"react"`)

    **New JSX pragma comments:**
    - `@jsxRuntime` &mdash; Sets the runtime (`automatic` or `classic`)
    - `@jsxImportSource` &mdash; Sets the import source (only valid with automatic runtime)

    The existing `@jsxFragment` and `@jsxFactory` pragma comments are only valid with "classic" runtime.

    **TSConfig resolving:**
    Along with accepting the new options directly via CLI or API, option inference from `tsconfig.json` compiler options was also implemented:

    - `"jsx": "preserve"` or `"jsx": "react-native"` &rarr; Same as `--jsx=preserve` in esbuild
    - `"jsx": "react"` &rarr; Same as `--jsx=transform` in esbuild (which is the default behavior)
    - `"jsx": "react-jsx"` &rarr; Same as `--jsx=automatic` in esbuild
    - `"jsx": "react-jsxdev"` &rarr; Same as `--jsx=automatic --jsx-dev` in esbuild

    It also reads the value of `"jsxImportSource"` from `tsconfig.json` if specified.

    For `react-jsx` it's important to note that it doesn't implicitly disable `--jsx-dev`. This is to support the case where a user sets `"react-jsx"` in their `tsconfig.json` but then toggles development mode directly in esbuild.

    **esbuild vs Babel vs TS vs...**

    There are a few differences between the various technologies that implement automatic JSX runtimes. The JSX transform in esbuild follows a mix of Babel's and TypeScript's behavior:

    - When an element has `__source` or `__self` props:
        - Babel: Print an error about a deprecated transform plugin
        - TypeScript: Allow the props
        - swc: Hard crash
        - **esbuild**: Print an error &mdash; Following Babel was chosen for this one because this might help people catch configuration issues where JSX files are being parsed by multiple tools

    - Element has an "implicit true" key prop, e.g. `<a key />`:
        - Babel: Print an error indicating that "key" props require an explicit value
        - TypeScript: Silently omit the "key" prop
        - swc: Hard crash
        - **esbuild**: Print an error like Babel &mdash; This might help catch legitimate programming mistakes

    - Element has spread children, e.g. `<a>{...children}</a>`
        - Babel: Print an error stating that React doesn't support spread children
        - TypeScript: Use static jsx function and pass children as-is, including spread operator
        - swc: same as Babel
        - **esbuild**: Same as TypeScript

    Also note that TypeScript has some bugs regarding JSX development mode and the generation of `lineNumber` and `columnNumber` values. Babel's values are accurate though, so esbuild's line and column numbers match Babel. Both numbers are 1-based and columns are counted in terms of UTF-16 code units.

    This feature was contributed by [@jgoz](https://github.com/jgoz).

## 0.14.50

* Emit `names` in source maps ([#1296](https://github.com/evanw/esbuild/issues/1296))

    The [source map specification](https://sourcemaps.info/spec.html) includes an optional `names` field that can associate an identifier with a mapping entry. This can be used to record the original name for an identifier, which is useful if the identifier was renamed to something else in the generated code. When esbuild was originally written, this field wasn't widely used, but now there are some debuggers that make use of it to provide better debugging of minified code. With this release, esbuild now includes a `names` field in the source maps that it generates. To save space, the original name is only recorded when it's different from the final name.

* Update parser for arrow functions with initial default type parameters in `.tsx` files ([#2410](https://github.com/evanw/esbuild/issues/2410))

    TypeScript 4.6 introduced a [change to the parsing of JSX syntax in `.tsx` files](https://github.com/microsoft/TypeScript/issues/47062). Now a `<` token followed by an identifier and then a `=` token is parsed as an arrow function with a default type parameter instead of as a JSX element. This release updates esbuild's parser to match TypeScript's parser.

* Fix an accidental infinite loop with `--define` substitution ([#2407](https://github.com/evanw/esbuild/issues/2407))

    This is a fix for a regression that was introduced in esbuild version 0.14.44 where certain `--define` substitutions could result in esbuild crashing with a stack overflow. The problem was an incorrect fix for #2292. The fix merged the code paths for `--define` and `--jsx-factory` rewriting since the value substitution is now the same for both. However, doing this accidentally made `--define` substitution recursive since the JSX factory needs to be able to match against `--define` substitutions to integrate with the `--inject` feature. The fix is to only do one additional level of matching against define substitutions, and to only do this for JSX factories. Now these cases are able to build successfully without a stack overflow.

* Include the "public path" value in hashes ([#2403](https://github.com/evanw/esbuild/issues/2403))

    The `--public-path=` configuration value affects the paths that esbuild uses to reference files from other files and is used in various situations such as cross-chunk imports in JS and references to asset files from CSS files. However, it wasn't included in the hash calculations used for file names due to an oversight. This meant that changing the public path setting incorrectly didn't result in the hashes in file names changing even though the contents of the files changed. This release fixes the issue by including a hash of the public path in all non-asset output files.

* Fix a cross-platform consistency bug ([#2383](https://github.com/evanw/esbuild/issues/2383))

    Previously esbuild would minify `0xFFFF_FFFF_FFFF_FFFF` as `0xffffffffffffffff` (18 bytes) on arm64 chips and as `18446744073709552e3` (19 bytes) on x86_64 chips. The reason was that the number was converted to a 64-bit unsigned integer internally for printing as hexadecimal, the 64-bit floating-point number `0xFFFF_FFFF_FFFF_FFFF` is actually `0x1_0000_0000_0000_0180` (i.e. it's rounded up, not down), and converting `float64` to `uint64` is implementation-dependent in Go when the input is out of bounds. This was fixed by changing the upper limit for which esbuild uses hexadecimal numbers during minification to `0xFFFF_FFFF_FFFF_F800`, which is the next representable 64-bit floating-point number below `0x1_0000_0000_0000_0180`, and which fits in a `uint64`. As a result, esbuild will now consistently never minify `0xFFFF_FFFF_FFFF_FFFF` as `0xffffffffffffffff` anymore, which means the output should now be consistent across platforms.

* Fix a hang with the synchronous API when the package is corrupted ([#2396](https://github.com/evanw/esbuild/issues/2396))

    An error message is already thrown when the esbuild package is corrupted and esbuild can't be run. However, if you are using a synchronous call in the JavaScript API in worker mode, esbuild will use a child worker to initialize esbuild once so that the overhead of initializing esbuild can be amortized across multiple synchronous API calls. However, errors thrown during initialization weren't being propagated correctly which resulted in a hang while the main thread waited forever for the child worker to finish initializing. With this release, initialization errors are now propagated correctly so calling a synchronous API call when the package is corrupted should now result in an error instead of a hang.

* Fix `tsconfig.json` files that collide with directory names ([#2411](https://github.com/evanw/esbuild/issues/2411))

    TypeScript lets you write `tsconfig.json` files with `extends` clauses that refer to another config file using an implicit `.json` file extension. However, if the config file without the `.json` extension existed as a directory name, esbuild and TypeScript had different behavior. TypeScript ignores the directory and continues looking for the config file by adding the `.json` extension while esbuild previously terminated the search and then failed to load the config file (because it's a directory). With this release, esbuild will now ignore exact matches when resolving `extends` fields in `tsconfig.json` files if the exact match results in a directory.

* Add `platform` to the transform API ([#2362](https://github.com/evanw/esbuild/issues/2362))

    The `platform` option is mainly relevant for bundling because it mostly affects path resolution (e.g. activating the `"browser"` field in `package.json` files), so it was previously only available for the build API. With this release, it has additionally be made available for the transform API for a single reason: you can now set `--platform=node` when transforming a string so that esbuild will add export annotations for node, which is only relevant when `--format=cjs` is also present.

    This has to do with an implementation detail of node that parses the AST of CommonJS files to discover named exports when importing CommonJS from ESM. However, this new addition to esbuild's API is of questionable usefulness. Node's loader API (the main use case for using esbuild's transform API like this) actually bypasses the content returned from the loader and parses the AST that's present on the file system, so you won't actually be able to use esbuild's API for this. See the linked issue for more information.

## 0.14.49

* Keep inlined constants when direct `eval` is present ([#2361](https://github.com/evanw/esbuild/issues/2361))

    Version 0.14.19 of esbuild added inlining of certain `const` variables during minification, which replaces all references to the variable with the initializer and then removes the variable declaration. However, this could generate incorrect code when direct `eval` is present because the direct `eval` could reference the constant by name. This release fixes the problem by preserving the `const` variable declaration in this case:

    ```js
    // Original code
    console.log((() => { const x = 123; return x + eval('x') }))

    // Old output (with --minify)
    console.log(()=>123+eval("x"));

    // New output (with --minify)
    console.log(()=>{const x=123;return 123+eval("x")});
    ```

* Fix an incorrect error in TypeScript when targeting ES5 ([#2375](https://github.com/evanw/esbuild/issues/2375))

    Previously when compiling TypeScript code to ES5, esbuild could incorrectly consider the following syntax forms as a transformation error:

    ```ts
    0 ? ([]) : 1 ? ({}) : 2;
    ```

    The error messages looked like this:

    ```
    ✘ [ERROR] Transforming destructuring to the configured target environment ("es5") is not supported yet

        example.ts:1:5:
          1 │ 0 ? ([]) : 1 ? ({}) : 2;
            ╵      ^

    ✘ [ERROR] Transforming destructuring to the configured target environment ("es5") is not supported yet

        example.ts:1:16:
          1 │ 0 ? ([]) : 1 ? ({}) : 2;
            ╵                 ^
    ```

    These parenthesized literals followed by a colon look like the start of an arrow function expression followed by a TypeScript return type (e.g. `([]) : 1` could be the start of the TypeScript arrow function `([]): 1 => 1`). Unlike in JavaScript, parsing arrow functions in TypeScript requires backtracking. In this case esbuild correctly determined that this expression wasn't an arrow function after all but the check for destructuring was incorrectly not covered under the backtracking process. With this release, the error message is now only reported if the parser successfully parses an arrow function without backtracking.

* Fix generated TypeScript `enum` comments containing `*/` ([#2369](https://github.com/evanw/esbuild/issues/2369), [#2371](https://github.com/evanw/esbuild/pull/2371))

    TypeScript `enum` values that are equal to a number or string literal are inlined (references to the enum are replaced with the literal value) and have a `/* ... */` comment after them with the original enum name to improve readability. However, this comment is omitted if the enum name contains the character sequence `*/` because that would end the comment early and cause a syntax error:

    ```ts
    // Original TypeScript
    enum Foo { '/*' = 1, '*/' = 2 }
    console.log(Foo['/*'], Foo['*/'])

    // Generated JavaScript
    console.log(1 /* /* */, 2);
    ```

    This was originally handled correctly when TypeScript `enum` inlining was initially implemented since it was only supported within a single file. However, when esbuild was later extended to support TypeScript `enum` inlining across files, this special case where the enum name contains `*/` was not handled in that new code. Starting with this release, esbuild will now handle enums with names containing `*/` correctly when they are inlined across files:

    ```ts
    // foo.ts
    export enum Foo { '/*' = 1, '*/' = 2 }

    // bar.ts
    import { Foo } from './foo'
    console.log(Foo['/*'], Foo['*/'])

    // Old output (with --bundle --format=esm)
    console.log(1 /* /* */, 2 /* */ */);

    // New output (with --bundle --format=esm)
    console.log(1 /* /* */, 2);
    ```

    This fix was contributed by [@magic-akari](https://github.com/magic-akari).

* Allow `declare` class fields to be initialized ([#2380](https://github.com/evanw/esbuild/issues/2380))

    This release fixes an oversight in the TypeScript parser that disallowed initializers for `declare` class fields. TypeScript actually allows the following limited initializer expressions for `readonly` fields:

    ```ts
    declare const enum a { b = 0 }

    class Foo {
      // These are allowed by TypeScript
      declare readonly a = 0
      declare readonly b = -0
      declare readonly c = 0n
      declare readonly d = -0n
      declare readonly e = 'x'
      declare readonly f = `x`
      declare readonly g = a.b
      declare readonly h = a['b']

      // These are not allowed by TypeScript
      declare readonly x = (0)
      declare readonly y = null
      declare readonly z = -a.b
    }
    ```

    So with this release, esbuild now allows initializers for `declare` class fields too. To future-proof this in case TypeScript allows more expressions as initializers in the future (such as `null`), esbuild will allow any expression as an initializer and will leave the specifics of TypeScript's special-casing here to the TypeScript type checker.

* Fix a bug in esbuild's feature compatibility table generator ([#2365](https://github.com/evanw/esbuild/issues/2365))

    Passing specific JavaScript engines to esbuild's `--target` flag restricts esbuild to only using JavaScript features that are supported on those engines in the output files that esbuild generates. The data for this feature is automatically derived from this compatibility table with a script: https://kangax.github.io/compat-table/.

    However, the script had a bug that could incorrectly consider a JavaScript syntax feature to be supported in a given engine even when it doesn't actually work in that engine. Specifically this bug happened when a certain aspect of JavaScript syntax has always worked incorrectly in that engine and the bug in that engine has never been fixed. This situation hasn't really come up before because previously esbuild pretty much only targeted JavaScript engines that always fix their bugs, but the two new JavaScript engines that were added in the previous release ([Hermes](https://hermesengine.dev/) and [Rhino](https://github.com/mozilla/rhino)) have many aspects of the JavaScript specification that have never been implemented, and may never be implemented. For example, the `let` and `const` keywords are not implemented correctly in those engines.

    With this release, esbuild's compatibility table generator script has been fixed and as a result, esbuild will now correctly consider a JavaScript syntax feature to be unsupported in a given engine if there is some aspect of that syntax that is broken in all known versions of that engine. This means that the following JavaScript syntax features are no longer considered to be supported by these engines (represented using esbuild's internal names for these syntax features):

    Hermes:
    - `arrow`
    - `const-and-let`
    - `default-argument`
    - `generator`
    - `optional-catch-binding`
    - `optional-chain`
    - `rest-argument`
    - `template-literal`

    Rhino:
    - `arrow`
    - `const-and-let`
    - `destructuring`
    - `for-of`
    - `generator`
    - `object-extensions`
    - `template-literal`

    IE:
    - `const-and-let`

## 0.14.48

* Enable using esbuild in Deno via WebAssembly ([#2323](https://github.com/evanw/esbuild/issues/2323))

    The native implementation of esbuild is much faster than the WebAssembly version, but some people don't want to give Deno the `--allow-run` permission necessary to run esbuild and are ok waiting longer for their builds to finish when using the WebAssembly backend. With this release, you can now use esbuild via WebAssembly in Deno. To do this you will need to import from `wasm.js` instead of `mod.js`:

    ```js
    import * as esbuild from 'https://deno.land/x/esbuild@v0.14.48/wasm.js'
    const ts = 'let test: boolean = true'
    const result = await esbuild.transform(ts, { loader: 'ts' })
    console.log('result:', result)
    ```

    Make sure you run Deno with `--allow-net` so esbuild can download the WebAssembly module. Using esbuild like this starts up a worker thread that runs esbuild in parallel (unless you call `esbuild.initialize({ worker: false })` to tell esbuild to run on the main thread). If you want to, you can call `esbuild.stop()` to terminate the worker if you won't be using esbuild anymore and you want to reclaim the memory.

    Note that Deno appears to have a bug where background WebAssembly optimization can prevent the process from exiting for many seconds. If you are trying to use Deno and WebAssembly to run esbuild quickly, you may need to manually call `Deno.exit(0)` after your code has finished running.

* Add support for font file MIME types ([#2337](https://github.com/evanw/esbuild/issues/2337))

    This release adds support for font file MIME types to esbuild, which means they are now recognized by the built-in local web server and they are now used when a font file is loaded using the `dataurl` loader. The full set of newly-added file extension MIME type mappings is as follows:

    * `.eot` => `application/vnd.ms-fontobject`
    * `.otf` => `font/otf`
    * `.sfnt` => `font/sfnt`
    * `.ttf` => `font/ttf`
    * `.woff` => `font/woff`
    * `.woff2` => `font/woff2`

* Remove `"use strict";` when targeting ESM ([#2347](https://github.com/evanw/esbuild/issues/2347))

    All ES module code is automatically in strict mode, so a `"use strict";` directive is unnecessary. With this release, esbuild will now remove the `"use strict";` directive if the output format is ESM. This change makes the generated output file a few bytes smaller:

    ```js
    // Original code
    'use strict'
    export let foo = 123

    // Old output (with --format=esm --minify)
    "use strict";let t=123;export{t as foo};

    // New output (with --format=esm --minify)
    let t=123;export{t as foo};
    ```

* Attempt to have esbuild work with Deno on FreeBSD ([#2356](https://github.com/evanw/esbuild/issues/2356))

    Deno doesn't support FreeBSD, but it's possible to build Deno for FreeBSD with some additional patches on top. This release of esbuild changes esbuild's Deno installer to download esbuild's FreeBSD binary in this situation. This configuration is unsupported although in theory everything should work.

* Add some more target JavaScript engines ([#2357](https://github.com/evanw/esbuild/issues/2357))

    This release adds the [Rhino](https://github.com/mozilla/rhino) and [Hermes](https://hermesengine.dev/) JavaScript engines to the set of engine identifiers that can be passed to the `--target` flag. You can use this to restrict esbuild to only using JavaScript features that are supported on those engines in the output files that esbuild generates.

## 0.14.47

* Make global names more compact when `||=` is available ([#2331](https://github.com/evanw/esbuild/issues/2331))

    With this release, the code esbuild generates for the `--global-name=` setting is now slightly shorter when you don't configure esbuild such that the `||=` operator is unsupported (e.g. with `--target=chrome80` or `--supported:logical-assignment=false`):

    ```js
    // Original code
    exports.foo = 123

    // Old output (with --format=iife --global-name=foo.bar.baz --minify)
    var foo=foo||{};foo.bar=foo.bar||{};foo.bar.baz=(()=>{var b=(a,o)=>()=>(o||a((o={exports:{}}).exports,o),o.exports);var c=b(f=>{f.foo=123});return c();})();

    // New output (with --format=iife --global-name=foo.bar.baz --minify)
    var foo;((foo||={}).bar||={}).baz=(()=>{var b=(a,o)=>()=>(o||a((o={exports:{}}).exports,o),o.exports);var c=b(f=>{f.foo=123});return c();})();
    ```

* Fix `--mangle-quoted=false` with `--minify-syntax=true`

    If property mangling is active and `--mangle-quoted` is disabled, quoted properties are supposed to be preserved. However, there was a case when this didn't happen if `--minify-syntax` was enabled, since that internally transforms `x['y']` into `x.y` to reduce code size. This issue has been fixed:

    ```js
    // Original code
    x.foo = x['bar'] = { foo: y, 'bar': z }

    // Old output (with --mangle-props=. --mangle-quoted=false --minify-syntax=true)
    x.a = x.b = { a: y, bar: z };

    // New output (with --mangle-props=. --mangle-quoted=false --minify-syntax=true)
    x.a = x.bar = { a: y, bar: z };
    ```

    Notice how the property `foo` is always used unquoted but the property `bar` is always used quoted, so `foo` should be consistently mangled while `bar` should be consistently not mangled.

* Fix a minification bug regarding `this` and property initializers

    When minification is enabled, esbuild attempts to inline the initializers of variables that have only been used once into the start of the following expression to reduce code size. However, there was a bug where this transformation could change the value of `this` when the initializer is a property access and the start of the following expression is a call expression. This release fixes the bug:

    ```js
    // Original code
    function foo(obj) {
      let fn = obj.prop;
      fn();
    }

    // Old output (with --minify)
    function foo(f){f.prop()}

    // New output (with --minify)
    function foo(o){let f=o.prop;f()}
    ```

## 0.14.46

* Add the ability to override support for individual syntax features ([#2060](https://github.com/evanw/esbuild/issues/2060), [#2290](https://github.com/evanw/esbuild/issues/2290), [#2308](https://github.com/evanw/esbuild/issues/2308))

    The `target` setting already lets you configure esbuild to restrict its output by only making use of syntax features that are known to be supported in the configured target environment. For example, setting `target` to `chrome50` causes esbuild to automatically transform optional chain expressions into the equivalent older JavaScript and prevents you from using BigInts, among many other things. However, sometimes you may want to customize this set of unsupported syntax features at the individual feature level.

    Some examples of why you might want to do this:

    * JavaScript runtimes often do a quick implementation of newer syntax features that is slower than the equivalent older JavaScript, and you can get a speedup by telling esbuild to pretend this syntax feature isn't supported. For example, V8 has a [long-standing performance bug regarding object spread](https://bugs.chromium.org/p/v8/issues/detail?id=11536) that can be avoided by manually copying properties instead of using object spread syntax. Right now esbuild hard-codes this optimization if you set `target` to a V8-based runtime.

    * There are many less-used JavaScript runtimes in addition to the ones present in browsers, and these runtimes sometimes just decide not to implement parts of the specification, which might make sense for runtimes intended for embedded environments. For example, the developers behind Facebook's JavaScript runtime [Hermes](https://hermesengine.dev/) have decided to not implement classes despite it being a major JavaScript feature that was added seven years ago and that is used in virtually every large JavaScript project.

    * You may be processing esbuild's output with another tool, and you may want esbuild to transform certain features and the other tool to transform certain other features. For example, if you are using esbuild to transform files individually to ES5 but you are then feeding the output into Webpack for bundling, you may want to preserve `import()` expressions even though they are a syntax error in ES5.

    With this release, you can now use `--supported:feature=false` to force `feature` to be unsupported. This will cause esbuild to either rewrite code that uses the feature into older code that doesn't use the feature (if esbuild is able to), or to emit a build error (if esbuild is unable to). For example, you can use `--supported:arrow=false` to turn arrow functions into function expressions and `--supported:bigint=false` to make it an error to use a BigInt literal. You can also use `--supported:feature=true` to force it to be supported, which means esbuild will pass it through without transforming it. Keep in mind that this is an advanced feature. For most use cases you will probably want to just use `target` instead of using this.

    The full set of currently-allowed features are as follows:

    **JavaScript:**
    * `arbitrary-module-namespace-names`
    * `array-spread`
    * `arrow`
    * `async-await`
    * `async-generator`
    * `bigint`
    * `class`
    * `class-field`
    * `class-private-accessor`
    * `class-private-brand-check`
    * `class-private-field`
    * `class-private-method`
    * `class-private-static-accessor`
    * `class-private-static-field`
    * `class-private-static-method`
    * `class-static-blocks`
    * `class-static-field`
    * `const-and-let`
    * `default-argument`
    * `destructuring`
    * `dynamic-import`
    * `exponent-operator`
    * `export-star-as`
    * `for-await`
    * `for-of`
    * `generator`
    * `hashbang`
    * `import-assertions`
    * `import-meta`
    * `logical-assignment`
    * `nested-rest-binding`
    * `new-target`
    * `node-colon-prefix-import`
    * `node-colon-prefix-require`
    * `nullish-coalescing`
    * `object-accessors`
    * `object-extensions`
    * `object-rest-spread`
    * `optional-catch-binding`
    * `optional-chain`
    * `regexp-dot-all-flag`
    * `regexp-lookbehind-assertions`
    * `regexp-match-indices`
    * `regexp-named-capture-groups`
    * `regexp-sticky-and-unicode-flags`
    * `regexp-unicode-property-escapes`
    * `rest-argument`
    * `template-literal`
    * `top-level-await`
    * `typeof-exotic-object-is-object`
    * `unicode-escapes`

    **CSS:**
    * `hex-rgba`
    * `rebecca-purple`
    * `modern-rgb-hsl`
    * `inset-property`
    * `nesting`

    Since you can now specify `--supported:object-rest-spread=false` yourself to work around the V8 performance issue mentioned above, esbuild will no longer automatically transform all instances of object spread when targeting a V8-based JavaScript runtime going forward.

    _Note that JavaScript feature transformation is very complex and allowing full customization of the set of supported syntax features could cause bugs in esbuild due to new interactions between multiple features that were never possible before. Consider this to be an experimental feature._

* Implement `extends` constraints on `infer` type variables ([#2330](https://github.com/evanw/esbuild/issues/2330))

    TypeScript 4.7 introduced the ability to write an `extends` constraint after an `infer` type variable, which looks like this:

    ```ts
    type FirstIfString<T> =
      T extends [infer S extends string, ...unknown[]]
        ? S
        : never;
    ```

    You can read the blog post for more details: https://devblogs.microsoft.com/typescript/announcing-typescript-4-7/#extends-constraints-on-infer-type-variables. Previously this was a syntax error in esbuild but with this release, esbuild can now parse this syntax correctly.

* Allow `define` to match optional chain expressions ([#2324](https://github.com/evanw/esbuild/issues/2324))

    Previously esbuild's `define` feature only matched member expressions that did not use optional chaining. With this release, esbuild will now also match those that use optional chaining:

    ```js
    // Original code
    console.log(a.b, a?.b)

    // Old output (with --define:a.b=c)
    console.log(c, a?.b);

    // New output (with --define:a.b=c)
    console.log(c, c);
    ```

    This is for compatibility with Webpack's [`DefinePlugin`](https://webpack.js.org/plugins/define-plugin/), which behaves the same way.

## 0.14.45

* Add a log message for ambiguous re-exports ([#2322](https://github.com/evanw/esbuild/issues/2322))

    In JavaScript, you can re-export symbols from another file using `export * from './another-file'`. When you do this from multiple files that export different symbols with the same name, this creates an ambiguous export which is causes that name to not be exported. This is harmless if you don't plan on using the ambiguous export name, so esbuild doesn't have a warning for this. But if you do want a warning for this (or if you want to make it an error), you can now opt-in to seeing this log message with `--log-override:ambiguous-reexport=warning` or `--log-override:ambiguous-reexport=error`. The log message looks like this:

    ```
    ▲ [WARNING] Re-export of "common" in "example.js" is ambiguous and has been removed [ambiguous-reexport]

      One definition of "common" comes from "a.js" here:

        a.js:2:11:
          2 │ export let common = 2
            ╵            ~~~~~~

      Another definition of "common" comes from "b.js" here:

        b.js:3:14:
          3 │ export { b as common }
            ╵               ~~~~~~
    ```

* Optimize the output of the JSON loader ([#2161](https://github.com/evanw/esbuild/issues/2161))

    The `json` loader (which is enabled by default for `.json` files) parses the file as JSON and generates a JavaScript file with the parsed expression as the `default` export. This behavior is standard and works in both node and the browser (well, as long as you use an [import assertion](https://v8.dev/features/import-assertions)). As an extension, esbuild also allows you to import additional top-level properties of the JSON object directly as a named export. This is beneficial for tree shaking. For example:

    ```js
    import { version } from 'esbuild/package.json'
    console.log(version)
    ```

    If you bundle the above code with esbuild, you'll get something like the following:

    ```js
    // node_modules/esbuild/package.json
    var version = "0.14.44";

    // example.js
    console.log(version);
    ```

    Most of the `package.json` file is irrelevant and has been omitted from the output due to tree shaking. The way esbuild implements this is to have the JavaScript file that's generated from the JSON look something like this with a separate exported variable for each property on the top-level object:

    ```js
    // node_modules/esbuild/package.json
    export var name = "esbuild";
    export var version = "0.14.44";
    export var repository = "https://github.com/evanw/esbuild";
    export var bin = {
      esbuild: "bin/esbuild"
    };
    ...
    export default {
      name,
      version,
      repository,
      bin,
      ...
    };
    ```

    However, this means that if you import the `default` export instead of a named export, you will get non-optimal output. The `default` export references all top-level properties, leading to many unnecessary variables in the output. With this release esbuild will now optimize this case to only generate additional variables for top-level object properties that are actually imported:

    ```js
    // Original code
    import all, { bar } from 'data:application/json,{"foo":[1,2,3],"bar":[4,5,6]}'
    console.log(all, bar)

    // Old output (with --bundle --minify --format=esm)
    var a=[1,2,3],l=[4,5,6],r={foo:a,bar:l};console.log(r,l);

    // New output (with --bundle --minify --format=esm)
    var l=[4,5,6],r={foo:[1,2,3],bar:l};console.log(r,l);
    ```

    Notice how there is no longer an unnecessary generated variable for `foo` since it's never imported. And if you only import the `default` export, esbuild will now reproduce the original JSON object in the output with all top-level properties compactly inline.

* Add `id` to warnings returned from the API

    With this release, warnings returned from esbuild's API now have an `id` property. This identifies which kind of log message it is, which can be used to more easily filter out certain warnings. For example, reassigning a `const` variable will generate a message with an `id` of `"assign-to-constant"`. This also gives you the identifier you need to apply a log override for that kind of message: https://esbuild.github.io/api/#log-override.

## 0.14.44

* Add a `copy` loader ([#2255](https://github.com/evanw/esbuild/issues/2255))

    You can configure the "loader" for a specific file extension in esbuild, which is a way of telling esbuild how it should treat that file. For example, the `text` loader means the file is imported as a string while the `binary` loader means the file is imported as a `Uint8Array`. If you want the imported file to stay a separate file, the only option was previously the `file` loader (which is intended to be similar to Webpack's [`file-loader`](https://v4.webpack.js.org/loaders/file-loader/) package). This loader copies the file to the output directory and imports the path to that output file as a string. This is useful for a web application because you can refer to resources such as `.png` images by importing them for their URL. However, it's not helpful if you need the imported file to stay a separate file but to still behave the way it normally would when the code is run without bundling.

    With this release, there is now a new loader called `copy` that copies the loaded file to the output directory and then rewrites the path of the import statement or `require()` call to point to the copied file instead of the original file. This will automatically add a content hash to the output name by default (which can be configured with the `--asset-names=` setting). You can use this by specifying `copy` for a specific file extension, such as with `--loader:.png=copy`.

* Fix a regression in arrow function lowering ([#2302](https://github.com/evanw/esbuild/pull/2302))

    This release fixes a regression with lowering arrow functions to function expressions in ES5. This feature was introduced in version 0.7.2 and regressed in version 0.14.30.

    In JavaScript, regular `function` expressions treat `this` as an implicit argument that is determined by how the function is called, but arrow functions treat `this` as a variable that is captured in the closure from the surrounding lexical scope. This is emulated in esbuild by storing the value of `this` in a variable before changing the arrow function into a function expression.

    However, the code that did this didn't treat `this` expressions as a usage of that generated variable. Version 0.14.30 began omitting unused generated variables, which caused the transformation of `this` to break. This regression happened due to missing test coverage. With this release, the problem has been fixed:

    ```js
    // Original code
    function foo() {
      return () => this
    }

    // Old output (with --target=es5)
    function foo() {
      return function() {
        return _this;
      };
    }

    // New output (with --target=es5)
    function foo() {
      var _this = this;
      return function() {
        return _this;
      };
    }
    ```

    This fix was contributed by [@nkeynes](https://github.com/nkeynes).

* Allow entity names as define values ([#2292](https://github.com/evanw/esbuild/issues/2292))

    The "define" feature allows you to replace certain expressions with certain other expressions at compile time. For example, you might want to replace the global identifier `IS_PRODUCTION` with the boolean value `true` when building for production. Previously the only expressions you could substitute in were either identifier expressions or anything that is valid JSON syntax. This limitation exists because supporting more complex expressions is more complex (for example, substituting in a `require()` call could potentially pull in additional files, which would need to be handled). With this release, you can now also now define something as a member expression chain of the form `foo.abc.xyz`.

* Implement package self-references ([#2312](https://github.com/evanw/esbuild/issues/2312))

    This release implements a rarely-used feature in node where a package can import itself by name instead of using relative imports. You can read more about this feature here: https://nodejs.org/api/packages.html#self-referencing-a-package-using-its-name. For example, assuming the `package.json` in a given package looks like this:

    ```json
    // package.json
    {
      "name": "a-package",
      "exports": {
        ".": "./main.mjs",
        "./foo": "./foo.js"
      }
    }
    ```

    Then any module in that package can reference an export in the package itself:

    ```js
    // ./a-module.mjs
    import { something } from 'a-package'; // Imports "something" from ./main.mjs.
    ```

    Self-referencing is also available when using `require`, both in an ES module, and in a CommonJS one. For example, this code will also work:

    ```js
    // ./a-module.js
    const { something } = require('a-package/foo'); // Loads from ./foo.js.
    ```

* Add a warning for assigning to an import ([#2319](https://github.com/evanw/esbuild/issues/2319))

    Import bindings are immutable in JavaScript, and assigning to them will throw an error. So instead of doing this:

    ```js
    import { foo } from 'foo'
    foo++
    ```

    You need to do something like this instead:

    ```js
    import { foo, setFoo } from 'foo'
    setFoo(foo + 1)
    ```

    This is already an error if you try to bundle this code with esbuild. However, this was previously allowed silently when bundling is disabled, which can lead to confusion for people who don't know about this aspect of how JavaScript works. So with this release, there is now a warning when you do this:

    ```
    ▲ [WARNING] This assignment will throw because "foo" is an import [assign-to-import]

        example.js:2:0:
          2 │ foo++
            ╵ ~~~

      Imports are immutable in JavaScript. To modify the value of this import, you must export a setter
      function in the imported file (e.g. "setFoo") and then import and call that function here instead.
    ```

    This new warning can be turned off with `--log-override:assign-to-import=silent` if you don't want to see it.

* Implement `alwaysStrict` in `tsconfig.json` ([#2264](https://github.com/evanw/esbuild/issues/2264))

    This release adds `alwaysStrict` to the set of TypeScript `tsconfig.json` configuration values that esbuild supports. When this is enabled, esbuild will forbid syntax that isn't allowed in strict mode and will automatically insert `"use strict";` at the top of generated output files. This matches the behavior of the TypeScript compiler: https://www.typescriptlang.org/tsconfig#alwaysStrict.

## 0.14.43

* Fix TypeScript parse error whe a generic function is the first type argument ([#2306](https://github.com/evanw/esbuild/issues/2306))

    In TypeScript, the `<<` token may need to be split apart into two `<` tokens if it's present in a type argument context. This was already correctly handled for all type expressions and for identifier expressions such as in the following code:

    ```ts
    // These cases already worked in the previous release
    let foo: Array<<T>() => T>;
    bar<<T>() => T>;
    ```

    However, normal expressions of the following form were previously incorrectly treated as syntax errors:

    ```ts
    // These cases were broken but have now been fixed
    foo.bar<<T>() => T>;
    foo?.<<T>() => T>();
    ```

    With this release, these cases now parsed correctly.

* Fix minification regression with pure IIFEs ([#2279](https://github.com/evanw/esbuild/issues/2279))

    An Immediately Invoked Function Expression (IIFE) is a function call to an anonymous function, and is a way of introducing a new function-level scope in JavaScript since JavaScript lacks a way to do this otherwise. And a pure function call is a function call with the special `/* @__PURE__ */` comment before it, which tells JavaScript build tools that the function call can be considered to have no side effects (and can be removed if it's unused).

    Version 0.14.9 of esbuild introduced a regression that changed esbuild's behavior when these two features were combined. If the IIFE body contains a single expression, the resulting output still contained that expression instead of being empty. This is a minor regression because you normally wouldn't write code like this, so this shouldn't come up in practice, and it doesn't cause any correctness issues (just larger-than-necessary output). It's unusual that you would tell esbuild "remove this if the result is unused" and then not store the result anywhere, since the result is unused by construction. But regardless, the issue has now been fixed.

    For example, the following code is a pure IIFE, which means it should be completely removed when minification is enabled. Previously it was replaced by the contents of the IIFE but it's now completely removed:

    ```js
    // Original code
    /* @__PURE__ */ (() => console.log(1))()

    // Old output (with --minify)
    console.log(1);

    // New output (with --minify)
    ```

* Add log messages for indirect `require` references ([#2231](https://github.com/evanw/esbuild/issues/2231))

    A long time ago esbuild used to warn about indirect uses of `require` because they break esbuild's ability to analyze the dependencies of the code and cause dependencies to not be bundled, resulting in a potentially broken bundle. However, this warning was removed because many people wanted the warning to be removed. Some packages have code that uses `require` like this but on a code path that isn't used at run-time, so their code still happens to work even though the bundle is incomplete. For example, the following code will _not_ bundle `bindings`:

    ```js
    // Prevent React Native packager from seeing modules required with this
    const nodeRequire = require;

    function getRealmConstructor(environment) {
      switch (environment) {
        case "node.js":
        case "electron":
          return nodeRequire("bindings")("realm.node").Realm;
      }
    }
    ```

    Version 0.11.11 of esbuild removed this warning, which means people no longer have a way to know at compile time whether their bundle is broken in this way. Now that esbuild has custom log message levels, this warning can be added back in a way that should make both people happy. With this release, there is now a log message for this that defaults to the `debug` log level, which normally isn't visible. You can either do `--log-override:indirect-require=warning` to make this log message a warning (and therefore visible) or use `--log-level=debug` to see this and all other `debug` log messages.

## 0.14.42

* Fix a parser hang on invalid CSS ([#2276](https://github.com/evanw/esbuild/issues/2276))

    Previously invalid CSS with unbalanced parentheses could cause esbuild's CSS parser to hang. An example of such an input is the CSS file `:x(`. This hang has been fixed.

* Add support for custom log message levels

    This release allows you to override the default log level of esbuild's individual log messages. For example, CSS syntax errors are treated as warnings instead of errors by default because CSS grammar allows for rules containing syntax errors to be ignored. However, if you would like for esbuild to consider CSS syntax errors to be build errors, you can now configure that like this:

    * CLI

        ```sh
        $ esbuild example.css --log-override:css-syntax-error=error
        ```

    * JS API

        ```js
        let result = await esbuild.build({
          entryPoints: ['example.css'],
          logOverride: {
            'css-syntax-error': 'error',
          },
        })
        ```

    * Go API

        ```go
        result := api.Build(api.BuildOptions{
          EntryPoints: []string{"example.ts"},
          LogOverride: map[string]api.LogLevel{
            "css-syntax-error": api.LogLevelError,
          },
        })
        ```

    You can also now use this feature to silence warnings that you are not interested in. Log messages are referred to by their identifier. Each identifier is stable (i.e. shouldn't change over time) except there is no guarantee that the log message will continue to exist. A given log message may potentially be removed in the future, in which case esbuild will ignore log levels set for that identifier. The current list of supported log level identifiers for use with this feature can be found below:

    **JavaScript:**
    * `assign-to-constant`
    * `call-import-namespace`
    * `commonjs-variable-in-esm`
    * `delete-super-property`
    * `direct-eval`
    * `duplicate-case`
    * `duplicate-object-key`
    * `empty-import-meta`
    * `equals-nan`
    * `equals-negative-zero`
    * `equals-new-object`
    * `html-comment-in-js`
    * `impossible-typeof`
    * `private-name-will-throw`
    * `semicolon-after-return`
    * `suspicious-boolean-not`
    * `this-is-undefined-in-esm`
    * `unsupported-dynamic-import`
    * `unsupported-jsx-comment`
    * `unsupported-regexp`
    * `unsupported-require-call`

    **CSS:**
    * `css-syntax-error`
    * `invalid-@charset`
    * `invalid-@import`
    * `invalid-@nest`
    * `invalid-@layer`
    * `invalid-calc`
    * `js-comment-in-css`
    * `unsupported-@charset`
    * `unsupported-@namespace`
    * `unsupported-css-property`

    **Bundler:**
    * `different-path-case`
    * `ignored-bare-import`
    * `ignored-dynamic-import`
    * `import-is-undefined`
    * `package.json`
    * `require-resolve-not-external`
    * `tsconfig.json`

    **Source maps:**
    * `invalid-source-mappings`
    * `sections-in-source-map`
    * `missing-source-map`
    * `unsupported-source-map-comment`

    Documentation about which identifiers correspond to which log messages will be added in the future, but hasn't been written yet. Note that it's not possible to configure the log level for a build error. This is by design because changing that would cause esbuild to incorrectly proceed in the building process generate invalid build output. You can only configure the log level for non-error log messages (although you can turn non-errors into errors).

## 0.14.41

* Fix a minification regression in 0.14.40 ([#2270](https://github.com/evanw/esbuild/issues/2270), [#2271](https://github.com/evanw/esbuild/issues/2271), [#2273](https://github.com/evanw/esbuild/pull/2273))

    Version 0.14.40 substituted string property keys with numeric property keys if the number has the same string representation as the original string. This was done in three places: computed member expressions, object literal properties, and class fields. However, negative numbers are only valid in computed member expressions while esbuild incorrectly applied this substitution for negative numbers in all places. This release fixes the regression by only doing this substitution for negative numbers in computed member expressions.

    This fix was contributed by [@susiwen8](https://github.com/susiwen8).

## 0.14.40

* Correct esbuild's implementation of `"preserveValueImports": true` ([#2268](https://github.com/evanw/esbuild/issues/2268))

    TypeScript's [`preserveValueImports` setting](https://www.typescriptlang.org/tsconfig#preserveValueImports) tells the compiler to preserve unused imports, which can sometimes be necessary because otherwise TypeScript will remove unused imports as it assumes they are type annotations. This setting is useful for programming environments that strip TypeScript types as part of a larger code transformation where additional code is appended later that will then make use of those unused imports, such as with [Svelte](https://svelte.dev/) or [Vue](https://vuejs.org/).

    This release fixes an issue where esbuild's implementation of `preserveValueImports` diverged from the official TypeScript compiler. If the import clause is present but empty of values (even if it contains types), then the import clause should be considered a type-only import clause. This was an oversight, and has now been fixed:

    ```ts
    // Original code
    import "keep"
    import { k1 } from "keep"
    import k2, { type t1 } from "keep"
    import {} from "remove"
    import { type t2 } from "remove"

    // Old output under "preserveValueImports": true
    import "keep";
    import { k1 } from "keep";
    import k2, {} from "keep";
    import {} from "remove";
    import {} from "remove";

    // New output under "preserveValueImports": true (matches the TypeScript compiler)
    import "keep";
    import { k1 } from "keep";
    import k2 from "keep";
    ```

* Avoid regular expression syntax errors in older browsers ([#2215](https://github.com/evanw/esbuild/issues/2215))

    Previously esbuild always passed JavaScript regular expression literals through unmodified from the input to the output. This is undesirable when the regular expression uses newer features that the configured target environment doesn't support. For example, the `d` flag (i.e. the [match indices feature](https://v8.dev/features/regexp-match-indices)) is new in ES2022 and doesn't work in older browsers. If esbuild generated a regular expression literal containing the `d` flag, then older browsers would consider esbuild's output to be a syntax error and none of the code would run.

    With this release, esbuild now detects when an unsupported feature is being used and converts the regular expression literal into a `new RegExp()` constructor instead. One consequence of this is that the syntax error is transformed into a run-time error, which allows the output code to run (and to potentially handle the run-time error). Another consequence of this is that it allows you to include a polyfill that overwrites the `RegExp` constructor in older browsers with one that supports modern features. Note that esbuild does not handle polyfills for you, so you will need to include a `RegExp` polyfill yourself if you want one.

    ```js
    // Original code
    console.log(/b/d.exec('abc').indices)

    // New output (with --target=chrome90)
    console.log(/b/d.exec("abc").indices);

    // New output (with --target=chrome89)
    console.log(new RegExp("b", "d").exec("abc").indices);
    ```

    This is currently done transparently without a warning. If you would like to debug this transformation to see where in your code esbuild is transforming regular expression literals and why, you can pass `--log-level=debug` to esbuild and review the information present in esbuild's debug logs.

* Add Opera to more internal feature compatibility tables ([#2247](https://github.com/evanw/esbuild/issues/2247), [#2252](https://github.com/evanw/esbuild/pull/2252))

    The internal compatibility tables that esbuild uses to determine which environments support which features are derived from multiple sources. Most of it is automatically derived from [these ECMAScript compatibility tables](https://kangax.github.io/compat-table/), but missing information is manually copied from [MDN](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/), GitHub PR comments, and various other websites. Version 0.14.35 of esbuild introduced Opera as a possible target environment which was automatically picked up by the compatibility table script, but the manually-copied information wasn't updated to include Opera. This release fixes this omission so Opera feature compatibility should now be accurate.

    This was contributed by [@lbwa](https://github.com/lbwa).

* Ignore `EPERM` errors on directories ([#2261](https://github.com/evanw/esbuild/issues/2261))

    Previously bundling with esbuild when inside a sandbox environment which does not have permission to access the parent directory did not work because esbuild would try to read the directory to search for a `node_modules` folder and would then fail the build when that failed. In practice this caused issues with running esbuild with `sandbox-exec` on macOS. With this release, esbuild will treat directories with permission failures as empty to allow for the `node_modules` search to continue past the denied directory and into its parent directory. This means it should now be possible to bundle with esbuild in these situations. This fix is similar to the fix in version 0.9.1 but is for `EPERM` while that fix was for `EACCES`.

* Remove an irrelevant extra `"use strict"` directive ([#2264](https://github.com/evanw/esbuild/issues/2264))

    The presence of a `"use strict"` directive in the output file is controlled by the presence of one in the entry point. However, there was a bug that would include one twice if the output format is ESM. This bug has been fixed.

* Minify strings into integers inside computed properties ([#2214](https://github.com/evanw/esbuild/issues/2214))

    This release now minifies `a["0"]` into `a[0]` when the result is equivalent:

    ```js
    // Original code
    console.log(x['0'], { '0': x }, class { '0' = x })

    // Old output (with --minify)
    console.log(x["0"],{"0":x},class{"0"=x});

    // New output (with --minify)
    console.log(x[0],{0:x},class{0=x});
    ```

    This transformation currently only happens when the numeric property represents an integer within the signed 32-bit integer range.

## 0.14.39

* Fix code generation for `export default` and `/* @__PURE__ */` call ([#2203](https://github.com/evanw/esbuild/issues/2203))

    The `/* @__PURE__ */` comment annotation can be added to function calls to indicate that they are side-effect free. These annotations are passed through into the output by esbuild since many JavaScript tools understand them. However, there was an edge case where printing this comment before a function call caused esbuild to fail to parenthesize a function literal because it thought it was no longer at the start of the expression. This problem has been fixed:

    ```js
    // Original code
    export default /* @__PURE__ */ (function() {
    })()

    // Old output
    export default /* @__PURE__ */ function() {
    }();

    // New output
    export default /* @__PURE__ */ (function() {
    })();
    ```

* Preserve `...` before JSX child expressions ([#2245](https://github.com/evanw/esbuild/issues/2245))

    TypeScript 4.5 changed how JSX child expressions that start with `...` are emitted. Previously the `...` was omitted but starting with TypeScript 4.5, the `...` is now preserved instead. This release updates esbuild to match TypeScript's new output in this case:

    ```jsx
    // Original code
    console.log(<a>{...b}</a>)

    // Old output
    console.log(/* @__PURE__ */ React.createElement("a", null, b));

    // New output
    console.log(/* @__PURE__ */ React.createElement("a", null, ...b));
    ```

    Note that this behavior is TypeScript-specific. Babel doesn't support the `...` token at all (it gives the error "Spread children are not supported in React").

* Slightly adjust esbuild's handling of the `browser` field in `package.json` ([#2239](https://github.com/evanw/esbuild/issues/2239))

    This release changes esbuild's interpretation of `browser` path remapping to fix a regression that was introduced in esbuild version 0.14.21. Browserify has a bug where it incorrectly matches package paths to relative paths in the `browser` field, and esbuild replicates this bug for compatibility with Browserify. I have a set of tests that I use to verify that esbuild's replication of this Browserify is accurate here: https://github.com/evanw/package-json-browser-tests. However, I was missing a test case and esbuild's behavior diverges from Browserify in this case. This release now handles this edge case as well:

    * `entry.js`:

        ```js
        require('pkg/sub')
        ```

    * `node_modules/pkg/package.json`:

        ```json
        {
          "browser": {
            "./sub": "./sub/foo.js",
            "./sub/sub.js": "./sub/foo.js"
          }
        }
        ```

    * `node_modules/pkg/sub/foo.js`:

        ```js
        require('sub')
        ```

    * `node_modules/sub/index.js`:

        ```js
        console.log('works')
        ```

    The import path `sub` in `require('sub')` was previously matching the remapping `"./sub/sub.js": "./sub/foo.js"` but with this release it should now no longer match that remapping. Now `require('sub')` will only match the remapping `"./sub/sub": "./sub/foo.js"` (without the trailing `.js`). Browserify apparently only matches without the `.js` suffix here.

## 0.14.38

* Further fixes to TypeScript 4.7 instantiation expression parsing ([#2201](https://github.com/evanw/esbuild/issues/2201))

    This release fixes some additional edge cases with parsing instantiation expressions from the upcoming version 4.7 of TypeScript. Previously it was allowed for an instantiation expression to precede a binary operator but with this release, that's no longer allowed. This was sometimes valid in the TypeScript 4.7 beta but is no longer allowed in the latest version of TypeScript 4.7. Fixing this also fixed a regression that was introduced by the previous release of esbuild:

    | Code           | TS 4.6.3     | TS 4.7.0 beta | TS 4.7.0 nightly | esbuild 0.14.36 | esbuild 0.14.37 | esbuild 0.14.38 |
    |----------------|--------------|---------------|------------------|-----------------|-----------------|-----------------|
    | `a<b> == c<d>` | Invalid      | `a == c`      | Invalid          | `a == c`        | `a == c`        | Invalid         |
    | `a<b> in c<d>` | Invalid      | Invalid       | Invalid          | Invalid         | `a in c`        | Invalid         |
    | `a<b>>=c<d>`   | Invalid      | Invalid       | Invalid          | Invalid         | `a >= c`        | Invalid         |
    | `a<b>=c<d>`    | Invalid      | `a < b >= c`  | `a = c`          | `a < b >= c`    | `a = c`         | `a = c`         |
    | `a<b>>c<d>`    | `a < b >> c` | `a < b >> c`  | `a < b >> c`     | `a < b >> c`    | `a > c`         | `a < b >> c`    |

    This table illustrates some of the more significant changes between all of these parsers. The most important part is that esbuild 0.14.38 now matches the behavior of the latest TypeScript compiler for all of these cases.

## 0.14.37

* Add support for TypeScript's `moduleSuffixes` field from TypeScript 4.7

    The upcoming version of TypeScript adds the `moduleSuffixes` field to `tsconfig.json` that introduces more rules to import path resolution. Setting `moduleSuffixes` to `[".ios", ".native", ""]` will try to look at the relative files `./foo.ios.ts`, `./foo.native.ts`, and finally `./foo.ts` for an import path of `./foo`. Note that the empty string `""` in `moduleSuffixes` is necessary for TypeScript to also look-up `./foo.ts`. This was announced in the [TypeScript 4.7 beta blog post](https://devblogs.microsoft.com/typescript/announcing-typescript-4-7-beta/#resolution-customization-with-modulesuffixes).

* Match the new ASI behavior from TypeScript nightly builds ([#2188](https://github.com/evanw/esbuild/pull/2188))

    This release updates esbuild to match some very recent behavior changes in the TypeScript parser regarding automatic semicolon insertion. For more information, see TypeScript issues #48711 and #48654 (I'm not linking to them directly to avoid Dependabot linkback spam on these issues due to esbuild's popularity). The result is that the following TypeScript code is now considered valid TypeScript syntax:

    ```ts
    class A<T> {}
    new A<number> /* ASI now happens here */
    if (0) {}

    interface B {
      (a: number): typeof a /* ASI now happens here */
      <T>(): void
    }
    ```

    This fix was contributed by [@g-plane](https://github.com/g-plane).

## 0.14.36

* Revert path metadata validation for now ([#2177](https://github.com/evanw/esbuild/issues/2177))

    This release reverts the path metadata validation that was introduced in the previous release. This validation has uncovered a potential issue with how esbuild handles `onResolve` callbacks in plugins that will need to be fixed before path metadata validation is re-enabled.

## 0.14.35

* Add support for parsing `typeof` on #private fields from TypeScript 4.7 ([#2174](https://github.com/evanw/esbuild/pull/2174))

    The upcoming version of TypeScript now lets you use `#private` fields in `typeof` type expressions:

    https://devblogs.microsoft.com/typescript/announcing-typescript-4-7-beta/#typeof-on-private-fields

    ```ts
    class Container {
      #data = "hello!";

      get data(): typeof this.#data {
        return this.#data;
      }

      set data(value: typeof this.#data) {
        this.#data = value;
      }
    }
    ```

    With this release, esbuild can now parse these new type expressions as well. This feature was contributed by [@magic-akari](https://github.com/magic-akari).

* Add Opera and IE to internal CSS feature support matrix ([#2170](https://github.com/evanw/esbuild/pull/2170))

    Version 0.14.18 of esbuild added Opera and IE as available target environments, and added them to the internal JS feature support matrix. CSS feature support was overlooked, however. This release adds knowledge of Opera and IE to esbuild's internal CSS feature support matrix:

    ```css
    /* Original input */
    a {
      color: rgba(0, 0, 0, 0.5);
    }

    /* Old output (with --target=opera49 --minify) */
    a{color:rgba(0,0,0,.5)}

    /* New output (with --target=opera49 --minify) */
    a{color:#00000080}
    ```

    The fix for this issue was contributed by [@sapphi-red](https://github.com/sapphi-red).

* Change TypeScript class field behavior when targeting ES2022

    TypeScript 4.3 introduced a breaking change where class field behavior changes from assign semantics to define semantics when the `target` setting in `tsconfig.json` is set to `ESNext`. Specifically, the default value for TypeScript's `useDefineForClassFields` setting when unspecified is `true` if and only if `target` is `ESNext`. TypeScript 4.6 introduced another change where this behavior now happens for both `ESNext` and `ES2022`. Presumably this will be the case for `ES2023` and up as well. With this release, esbuild's behavior has also been changed to match. Now configuring esbuild with `--target=es2022` will also cause TypeScript files to use the new class field behavior.

* Validate that path metadata returned by plugins is consistent

    The plugin API assumes that all metadata for the same path returned by a plugin's `onResolve` callback is consistent. Previously this assumption was just assumed without any enforcement. Starting with this release, esbuild will now enforce this by generating a build error if this assumption is violated. The lack of validation has not been an issue (I have never heard of this being a problem), but it still seems like a good idea to enforce it. Here's a simple example of a plugin that generates inconsistent `sideEffects` metadata:

    ```js
    let buggyPlugin = {
      name: 'buggy',
      setup(build) {
        let count = 0
        build.onResolve({ filter: /^react$/ }, args => {
          return {
            path: require.resolve(args.path),
            sideEffects: count++ > 0,
          }
        })
      },
    }
    ```

    Since esbuild processes everything in parallel, the set of metadata that ends up being used for a given path is essentially random since it's whatever the task scheduler decides to schedule first. Thus if a plugin does not consistently provide the same metadata for a given path, subsequent builds may return different results. This new validation check prevents this problem.

    Here's the new error message that's shown when this happens:

    ```
    ✘ [ERROR] [plugin buggy] Detected inconsistent metadata for the path "node_modules/react/index.js" when it was imported here:

        button.tsx:1:30:
          1 │ import { createElement } from 'react'
            ╵                               ~~~~~~~

      The original metadata for that path comes from when it was imported here:

        app.tsx:1:23:
          1 │ import * as React from 'react'
            ╵                        ~~~~~~~

      The difference in metadata is displayed below:

       {
      -  "sideEffects": true,
      +  "sideEffects": false,
       }

      This is a bug in the "buggy" plugin. Plugins provide metadata for a given path in an "onResolve"
      callback. All metadata provided for the same path must be consistent to ensure deterministic
      builds. Due to parallelism, one set of provided metadata will be randomly chosen for a given path,
      so providing inconsistent metadata for the same path can cause non-determinism.
    ```

* Suggest enabling a missing condition when `exports` map fails ([#2163](https://github.com/evanw/esbuild/issues/2163))

    This release adds another suggestion to the error message that happens when an `exports` map lookup fails if the failure could potentially be fixed by adding a missing condition. Here's what the new error message looks like (which now suggests `--conditions=module` as a possible workaround):

    ```
    ✘ [ERROR] Could not resolve "@sentry/electron/main"

        index.js:1:24:
          1 │ import * as Sentry from '@sentry/electron/main'
            ╵                         ~~~~~~~~~~~~~~~~~~~~~~~

      The path "./main" is not currently exported by package "@sentry/electron":

        node_modules/@sentry/electron/package.json:8:13:
          8 │   "exports": {
            ╵              ^

      None of the conditions provided ("require", "module") match any of the currently active conditions
      ("browser", "default", "import"):

        node_modules/@sentry/electron/package.json:16:14:
          16 │     "./main": {
             ╵               ^

      Consider enabling the "module" condition if this package expects it to be enabled. You can use
      "--conditions=module" to do that:

        node_modules/@sentry/electron/package.json:18:6:
          18 │       "module": "./esm/main/index.js"
             ╵       ~~~~~~~~

      Consider using a "require()" call to import this file, which will work because the "require"
      condition is supported by this package:

        index.js:1:24:
          1 │ import * as Sentry from '@sentry/electron/main'
            ╵                         ~~~~~~~~~~~~~~~~~~~~~~~

      You can mark the path "@sentry/electron/main" as external to exclude it from the bundle, which
      will remove this error.
    ```

    This particular package had an issue where it was using the Webpack-specific `module` condition without providing a `default` condition. It looks like the intent in this case was to use the standard `import` condition instead. This specific change wasn't suggested here because this error message is for package consumers, not package authors.

## 0.14.34

Something went wrong with the publishing script for the previous release. Publishing again.

## 0.14.33

* Fix a regression regarding `super` ([#2158](https://github.com/evanw/esbuild/issues/2158))

    This fixes a regression from the previous release regarding classes with a super class, a private member, and a static field in the scenario where the static field needs to be lowered but where private members are supported by the configured target environment. In this scenario, esbuild could incorrectly inject the instance field initializers that use `this` into the constructor before the call to `super()`, which is invalid. This problem has now been fixed (notice that `this` is now used after `super()` instead of before):

    ```js
    // Original code
    class Foo extends Object {
      static FOO;
      constructor() {
        super();
      }
      #foo;
    }

    // Old output (with --bundle)
    var _foo;
    var Foo = class extends Object {
      constructor() {
        __privateAdd(this, _foo, void 0);
        super();
      }
    };
    _foo = new WeakMap();
    __publicField(Foo, "FOO");

    // New output (with --bundle)
    var _foo;
    var Foo = class extends Object {
      constructor() {
        super();
        __privateAdd(this, _foo, void 0);
      }
    };
    _foo = new WeakMap();
    __publicField(Foo, "FOO");
    ```

    During parsing, esbuild scans the class and makes certain decisions about the class such as whether to lower all static fields, whether to lower each private member, or whether calls to `super()` need to be tracked and adjusted. Previously esbuild made two passes through the class members to compute this information. However, with the new `super()` call lowering logic added in the previous release, we now need three passes to capture the whole dependency chain for this case: 1) lowering static fields requires 2) lowering private members which requires 3) adjusting `super()` calls.

    The reason lowering static fields requires lowering private members is because lowering static fields moves their initializers outside of the class body, where they can't access private members anymore. Consider this code:

    ```js
    class Foo {
      get #foo() {}
      static bar = new Foo().#foo
    }
    ```

    We can't just lower static fields without also lowering private members, since that causes a syntax error:

    ```js
    class Foo {
      get #foo() {}
    }
    Foo.bar = new Foo().#foo;
    ```

    And the reason lowering private members requires adjusting `super()` calls is because the injected private member initializers use `this`, which is only accessible after `super()` calls in the constructor.

* Fix an issue with `--keep-names` not keeping some names ([#2149](https://github.com/evanw/esbuild/issues/2149))

    This release fixes a regression with `--keep-names` from version 0.14.26. PR [#2062](https://github.com/evanw/esbuild/pull/2062) attempted to remove superfluous calls to the `__name` helper function by omitting calls of the form `__name(foo, "foo")` where the name of the symbol in the first argument is equal to the string in the second argument. This was assuming that the initializer for the symbol would automatically be assigned the expected `.name` property by the JavaScript VM, which turned out to be an incorrect assumption. To fix the regression, this PR has been reverted.

    The assumption is true in many cases but isn't true when the initializer is moved into another automatically-generated variable, which can sometimes be necessary during the various syntax transformations that esbuild does. For example, consider the following code:

    ```js
    class Foo {
      static get #foo() { return Foo.name }
      static get foo() { return this.#foo }
    }
    let Bar = Foo
    Foo = { name: 'Bar' }
    console.log(Foo.name, Bar.name)
    ```

    This code should print `Bar Foo`. With `--keep-names --target=es6` that code is lowered by esbuild into the following code (omitting the helper function definitions for brevity):

    ```js
    var _foo, foo_get;
    const _Foo = class {
      static get foo() {
        return __privateGet(this, _foo, foo_get);
      }
    };
    let Foo = _Foo;
    __name(Foo, "Foo");
    _foo = new WeakSet();
    foo_get = /* @__PURE__ */ __name(function() {
      return _Foo.name;
    }, "#foo");
    __privateAdd(Foo, _foo);
    let Bar = Foo;
    Foo = { name: "Bar" };
    console.log(Foo.name, Bar.name);
    ```

    The injection of the automatically-generated `_Foo` variable is necessary to preserve the semantics of the captured `Foo` binding for methods defined within the class body, even when the definition needs to be moved outside of the class body during code transformation. Due to a JavaScript quirk, this binding is immutable and does not change even if `Foo` is later reassigned. The PR that was reverted was incorrectly removing the call to `__name(Foo, "Foo")`, which turned out to be necessary after all in this case.

* Print some large integers using hexadecimal when minifying ([#2162](https://github.com/evanw/esbuild/issues/2162))

    When `--minify` is active, esbuild will now use one fewer byte to represent certain large integers:

    ```js
    // Original code
    x = 123456787654321;

    // Old output (with --minify)
    x=123456787654321;

    // New output (with --minify)
    x=0x704885f926b1;
    ```

    This works because a hexadecimal representation can be shorter than a decimal representation starting at around 10<sup>12</sup> and above.

    _This optimization made me realize that there's probably an opportunity to optimize printed numbers for smaller gzipped size instead of or in addition to just optimizing for minimal uncompressed byte count. The gzip algorithm does better with repetitive sequences, so for example `0xFFFFFFFF` is probably a better representation than `4294967295` even though the byte counts are the same. As far as I know, no JavaScript minifier does this optimization yet. I don't know enough about how gzip works to know if this is a good idea or what the right metric for this might be._

* Add Linux ARM64 support for Deno ([#2156](https://github.com/evanw/esbuild/issues/2156))

    This release adds Linux ARM64 support to esbuild's [Deno](https://deno.land/) API implementation, which allows esbuild to be used with Deno on a Raspberry Pi.

## 0.14.32

* Fix `super` usage in lowered private methods ([#2039](https://github.com/evanw/esbuild/issues/2039))

    Previously esbuild failed to transform `super` property accesses inside private methods in the case when private methods have to be lowered because the target environment doesn't support them. The generated code still contained the `super` keyword even though the method was moved outside of the class body, which is a syntax error in JavaScript. This release fixes this transformation issue and now produces valid code:

    ```js
    // Original code
    class Derived extends Base {
      #foo() { super.foo() }
      bar() { this.#foo() }
    }

    // Old output (with --target=es6)
    var _foo, foo_fn;
    class Derived extends Base {
      constructor() {
        super(...arguments);
        __privateAdd(this, _foo);
      }
      bar() {
        __privateMethod(this, _foo, foo_fn).call(this);
      }
    }
    _foo = new WeakSet();
    foo_fn = function() {
      super.foo();
    };

    // New output (with --target=es6)
    var _foo, foo_fn;
    const _Derived = class extends Base {
      constructor() {
        super(...arguments);
        __privateAdd(this, _foo);
      }
      bar() {
        __privateMethod(this, _foo, foo_fn).call(this);
      }
    };
    let Derived = _Derived;
    _foo = new WeakSet();
    foo_fn = function() {
      __superGet(_Derived.prototype, this, "foo").call(this);
    };
    ```

    Because of this change, lowered `super` property accesses on instances were rewritten so that they can exist outside of the class body. This rewrite affects code generation for all `super` property accesses on instances including those inside lowered `async` functions. The new approach is different but should be equivalent to the old approach:

    ```js
    // Original code
    class Foo {
      foo = async () => super.foo()
    }

    // Old output (with --target=es6)
    class Foo {
      constructor() {
        __publicField(this, "foo", () => {
          var __superGet = (key) => super[key];
          return __async(this, null, function* () {
            return __superGet("foo").call(this);
          });
        });
      }
    }

    // New output (with --target=es6)
    class Foo {
      constructor() {
        __publicField(this, "foo", () => __async(this, null, function* () {
          return __superGet(Foo.prototype, this, "foo").call(this);
        }));
      }
    }
    ```

* Fix some tree-shaking bugs regarding property side effects

    This release fixes some cases where side effects in computed properties were not being handled correctly. Specifically primitives and private names as properties should not be considered to have side effects, and object literals as properties should be considered to have side effects:

    ```js
    // Original code
    let shouldRemove = { [1]: 2 }
    let shouldRemove2 = class { #foo }
    let shouldKeep = class { [{ toString() { sideEffect() } }] }

    // Old output (with --tree-shaking=true)
    let shouldRemove = { [1]: 2 };
    let shouldRemove2 = class {
      #foo;
    };

    // New output (with --tree-shaking=true)
    let shouldKeep = class {
      [{ toString() {
        sideEffect();
      } }];
    };
    ```

* Add the `wasmModule` option to the `initialize` JS API ([#1093](https://github.com/evanw/esbuild/issues/1093))

    The `initialize` JS API must be called when using esbuild in the browser to provide the WebAssembly module for esbuild to use. Previously the only way to do that was using the `wasmURL` API option like this:

    ```js
    await esbuild.initialize({
      wasmURL: '/node_modules/esbuild-wasm/esbuild.wasm',
    })
    console.log(await esbuild.transform('1+2'))
    ```

    With this release, you can now also initialize esbuild using a `WebAssembly.Module` instance using the `wasmModule` API option instead. The example above is equivalent to the following code:

    ```js
    await esbuild.initialize({
      wasmModule: await WebAssembly.compileStreaming(fetch('/node_modules/esbuild-wasm/esbuild.wasm'))
    })
    console.log(await esbuild.transform('1+2'))
    ```

    This could be useful for environments where you want more control over how the WebAssembly download happens or where downloading the WebAssembly module is not possible.

## 0.14.31

* Add support for parsing "optional variance annotations" from TypeScript 4.7 ([#2102](https://github.com/evanw/esbuild/pull/2102))

    The upcoming version of TypeScript now lets you specify `in` and/or `out` on certain type parameters (specifically only on a type alias, an interface declaration, or a class declaration). These modifiers control type parameter covariance and contravariance:

    ```ts
    type Provider<out T> = () => T;
    type Consumer<in T> = (x: T) => void;
    type Mapper<in T, out U> = (x: T) => U;
    type Processor<in out T> = (x: T) => T;
    ```

    With this release, esbuild can now parse these new type parameter modifiers. This feature was contributed by [@magic-akari](https://github.com/magic-akari).

* Improve support for `super()` constructor calls in nested locations ([#2134](https://github.com/evanw/esbuild/issues/2134))

    In JavaScript, derived classes must call `super()` somewhere in the `constructor` method before being able to access `this`. Class public instance fields, class private instance fields, and TypeScript constructor parameter properties can all potentially cause code which uses `this` to be inserted into the constructor body, which must be inserted after the `super()` call. To make these insertions straightforward to implement, the TypeScript compiler doesn't allow calling `super()` somewhere other than in a root-level statement in the constructor body in these cases.

    Previously esbuild's class transformations only worked correctly when `super()` was called in a root-level statement in the constructor body, just like the TypeScript compiler. But with this release, esbuild should now generate correct code as long as the call to `super()` appears anywhere in the constructor body:

    ```ts
    // Original code
    class Foo extends Bar {
      constructor(public skip = false) {
        if (skip) {
          super(null)
          return
        }
        super({ keys: [] })
      }
    }

    // Old output (incorrect)
    class Foo extends Bar {
      constructor(skip = false) {
        if (skip) {
          super(null);
          return;
        }
        super({ keys: [] });
        this.skip = skip;
      }
    }

    // New output (correct)
    class Foo extends Bar {
      constructor(skip = false) {
        var __super = (...args) => {
          super(...args);
          this.skip = skip;
        };
        if (skip) {
          __super(null);
          return;
        }
        __super({ keys: [] });
      }
    }
    ```

* Add support for the new `@container` CSS rule ([#2127](https://github.com/evanw/esbuild/pull/2127))

    This release adds support for [`@container`](https://drafts.csswg.org/css-contain-3/#container-rule) in CSS files. This means esbuild will now pretty-print and minify these rules better since it now better understands the internal structure of these rules:

    ```css
    /* Original code */
    @container (width <= 150px) {
      #inner {
        color: yellow;
      }
    }

    /* Old output (with --minify) */
    @container (width <= 150px){#inner {color: yellow;}}

    /* New output (with --minify) */
    @container (width <= 150px){#inner{color:#ff0}}
    ```

    This was contributed by [@yisibl](https://github.com/yisibl).

* Avoid CSS cascade-dependent keywords in the `font-family` property ([#2135](https://github.com/evanw/esbuild/pull/2135))

    In CSS, [`initial`](https://developer.mozilla.org/en-US/docs/Web/CSS/initial), [`inherit`](https://developer.mozilla.org/en-US/docs/Web/CSS/inherit), and [`unset`](https://developer.mozilla.org/en-US/docs/Web/CSS/unset) are [CSS-wide keywords](https://drafts.csswg.org/css-values-4/#css-wide-keywords) which means they have special behavior when they are specified as a property value. For example, while `font-family: 'Arial'` (as a string) and `font-family: Arial` (as an identifier) are the same, `font-family: 'inherit'` (as a string) uses the font family named `inherit` but `font-family: inherit` (as an identifier) inherits the font family from the parent element. This means esbuild must not unquote these CSS-wide keywords (and `default`, which is also reserved) during minification to avoid changing the meaning of the minified CSS.

    The current draft of the new CSS Cascading and Inheritance Level 5 specification adds another concept called [cascade-dependent keywords](https://drafts.csswg.org/css-cascade-5/#defaulting-keywords) of which there are two: [`revert`](https://developer.mozilla.org/en-US/docs/Web/CSS/revert) and [`revert-layer`](https://developer.mozilla.org/en-US/docs/Web/CSS/revert-layer). This release of esbuild guards against unquoting these additional keywords as well to avoid accidentally breaking pages that use a font with the same name:

    ```css
    /* Original code */
    a { font-family: 'revert'; }
    b { font-family: 'revert-layer', 'Segoe UI', serif; }

    /* Old output (with --minify) */
    a{font-family:revert}b{font-family:revert-layer,Segoe UI,serif}

    /* New output (with --minify) */
    a{font-family:"revert"}b{font-family:"revert-layer",Segoe UI,serif}
    ```

    This fix was contributed by [@yisibl](https://github.com/yisibl).

## 0.14.30

* Change the context of TypeScript parameter decorators ([#2147](https://github.com/evanw/esbuild/issues/2147))

    While TypeScript parameter decorators are expressions, they are not evaluated where they exist in the code. They are moved to after the class declaration and evaluated there instead. Specifically this TypeScript code:

    ```ts
    class Class {
      method(@decorator() arg) {}
    }
    ```

    becomes this JavaScript code:

    ```js
    class Class {
      method(arg) {}
    }
    __decorate([
      __param(0, decorator())
    ], Class.prototype, "method", null);
    ```

    This has several consequences:

    * Whether `await` is allowed inside a decorator expression or not depends on whether the class declaration itself is in an `async` context or not. With this release, you can now use `await` inside a decorator expression when the class declaration is either inside an `async` function or is at the top-level of an ES module and top-level await is supported. Note that the TypeScript compiler currently has a bug regarding this edge case: https://github.com/microsoft/TypeScript/issues/48509.

        ```ts
        // Using "await" inside a decorator expression is now allowed
        async function fn(foo: Promise<any>) {
          class Class {
            method(@decorator(await foo) arg) {}
          }
          return Class
        }
        ```

        Also while TypeScript currently allows `await` to be used like this in `async` functions, it doesn't currently allow `yield` to be used like this in generator functions. It's not yet clear whether this behavior with `yield` is a bug or by design, so I haven't made any changes to esbuild's handling of `yield` inside decorator expressions in this release.

    * Since the scope of a decorator expression is the scope enclosing the class declaration, they cannot access private identifiers. Previously this was incorrectly allowed but with this release, esbuild no longer allows this. Note that the TypeScript compiler currently has a bug regarding this edge case: https://github.com/microsoft/TypeScript/issues/48515.

        ```ts
        // Using private names inside a decorator expression is no longer allowed
        class Class {
          static #priv = 123
          method(@decorator(Class.#priv) arg) {}
        }
        ```

    * Since the scope of a decorator expression is the scope enclosing the class declaration, identifiers inside parameter decorator expressions should never be resolved to a parameter of the enclosing method. Previously this could happen, which was a bug with esbuild. This bug no longer happens in this release.

        ```ts
        // Name collisions now resolve to the outer name instead of the inner name
        let arg = 1
        class Class {
          method(@decorator(arg) arg = 2) {}
        }
        ```

        Specifically previous versions of esbuild generated the following incorrect JavaScript (notice the use of `arg2`):

        ```js
        let arg = 1;
        class Class {
          method(arg2 = 2) {
          }
        }
        __decorateClass([
          __decorateParam(0, decorator(arg2))
        ], Class.prototype, "method", 1);
        ```

        This release now generates the following correct JavaScript (notice the use of `arg`):

        ```js
        let arg = 1;
        class Class {
          method(arg2 = 2) {
          }
        }
        __decorateClass([
          __decorateParam(0, decorator(arg))
        ], Class.prototype, "method", 1);
        ```

* Fix some obscure edge cases with `super` property access

    This release fixes the following obscure problems with `super` when targeting an older JavaScript environment such as `--target=es6`:

    1. The compiler could previously crash when a lowered `async` arrow function contained a class with a field initializer that used a `super` property access:

        ```js
        let foo = async () => class extends Object {
          bar = super.toString
        }
        ```

    2. The compiler could previously generate incorrect code when a lowered `async` method of a derived class contained a nested class with a computed class member involving a `super` property access on the derived class:

        ```js
        class Base {
          foo() { return 'bar' }
        }
        class Derived extends Base {
          async foo() {
            return new class { [super.foo()] = 'success' }
          }
        }
        new Derived().foo().then(obj => console.log(obj.bar))
        ```

    3. The compiler could previously generate incorrect code when a default-exported class contained a `super` property access inside a lowered static private class field:

        ```js
        class Foo {
          static foo = 123
        }
        export default class extends Foo {
          static #foo = super.foo
          static bar = this.#foo
        }
        ```

## 0.14.29

* Fix a minification bug with a double-nested `if` inside a label followed by `else` ([#2139](https://github.com/evanw/esbuild/issues/2139))

    This fixes a minification bug that affects the edge case where `if` is followed by `else` and the `if` contains a label that contains a nested `if`. Normally esbuild's AST printer automatically wraps the body of a single-statement `if` in braces to avoid the "dangling else" `if`/`else` ambiguity common to C-like languages (where the `else` accidentally becomes associated with the inner `if` instead of the outer `if`). However, I was missing automatic wrapping of label statements, which did not have test coverage because they are a rarely-used feature. This release fixes the bug:

    ```js
    // Original code
    if (a)
      b: {
        if (c) break b
      }
    else if (d)
      e()

    // Old output (with --minify)
    if(a)e:if(c)break e;else d&&e();

    // New output (with --minify)
    if(a){e:if(c)break e}else d&&e();
    ```

* Fix edge case regarding `baseUrl` and `paths` in `tsconfig.json` ([#2119](https://github.com/evanw/esbuild/issues/2119))

    In `tsconfig.json`, TypeScript forbids non-relative values inside `paths` if `baseUrl` is not present, and esbuild does too. However, TypeScript checked this after the entire `tsconfig.json` hierarchy was parsed while esbuild incorrectly checked this immediately when parsing the file containing the `paths` map. This caused incorrect warnings to be generated for `tsconfig.json` files that specify a `baseUrl` value and that inherit a `paths` value from an `extends` clause. Now esbuild will only check for non-relative `paths` values after the entire hierarchy has been parsed to avoid generating incorrect warnings.

* Better handle errors where the esbuild binary executable is corrupted or missing ([#2129](https://github.com/evanw/esbuild/issues/2129))

    If the esbuild binary executable is corrupted or missing, previously there was one situation where esbuild's JavaScript API could hang instead of generating an error. This release changes esbuild's library code to generate an error instead in this case.

## 0.14.28

* Add support for some new CSS rules ([#2115](https://github.com/evanw/esbuild/issues/2115), [#2116](https://github.com/evanw/esbuild/issues/2116), [#2117](https://github.com/evanw/esbuild/issues/2117))

    This release adds support for [`@font-palette-values`](https://drafts.csswg.org/css-fonts-4/#font-palette-values), [`@counter-style`](https://developer.mozilla.org/en-US/docs/Web/CSS/@counter-style), and [`@font-feature-values`](https://developer.mozilla.org/en-US/docs/Web/CSS/@font-feature-values). This means esbuild will now pretty-print and minify these rules better since it now better understands the internal structure of these rules:

    ```css
    /* Original code */
    @font-palette-values --Foo { base-palette: 1; }
    @counter-style bar { symbols: b a r; }
    @font-feature-values Bop { @styleset { test: 1; } }

    /* Old output (with --minify) */
    @font-palette-values --Foo{base-palette: 1;}@counter-style bar{symbols: b a r;}@font-feature-values Bop{@styleset {test: 1;}}

    /* New output (with --minify) */
    @font-palette-values --Foo{base-palette:1}@counter-style bar{symbols:b a r}@font-feature-values Bop{@styleset{test:1}}
    ```

* Upgrade to Go 1.18.0 ([#2105](https://github.com/evanw/esbuild/issues/2105))

    Binary executables for this version are now published with Go version 1.18.0. The [Go release notes](https://go.dev/doc/go1.18) say that the linker generates smaller binaries and that on 64-bit ARM chips, compiled binaries run around 10% faster. On an M1 MacBook Pro, esbuild's benchmark runs approximately 8% faster than before and the binary executable is approximately 4% smaller than before.

    This also fixes a regression from version 0.14.26 of esbuild where the browser builds of the `esbuild-wasm` package could fail to be bundled due to the use of built-in node libraries. The primary WebAssembly shim for Go 1.18.0 no longer uses built-in node libraries.

## 0.14.27

* Avoid generating an enumerable `default` import for CommonJS files in Babel mode ([#2097](https://github.com/evanw/esbuild/issues/2097))

    Importing a CommonJS module into an ES module can be done in two different ways. In node mode the `default` import is always set to `module.exports`, while in Babel mode the `default` import passes through to `module.exports.default` instead. Node mode is triggered when the importing file ends in `.mjs`, has `type: "module"` in its `package.json` file, or the imported module does not have a `__esModule` marker.

    Previously esbuild always created the forwarding `default` import in Babel mode, even if `module.exports` had no property called `default`. This was problematic because the getter named `default` still showed up as a property on the imported namespace object, and could potentially interfere with code that iterated over the properties of the imported namespace object. With this release the getter named `default` will now only be added in Babel mode if the `default` property exists at the time of the import.

* Fix a circular import edge case regarding ESM-to-CommonJS conversion ([#1894](https://github.com/evanw/esbuild/issues/1894), [#2059](https://github.com/evanw/esbuild/pull/2059))

    This fixes a regression that was introduced in version 0.14.5 of esbuild. Ever since that version, esbuild now creates two separate export objects when you convert an ES module file into a CommonJS module: one for ES modules and one for CommonJS modules. The one for CommonJS modules is written to `module.exports` and exported from the file, and the one for ES modules is internal and can be accessed by bundling code that imports the entry point (for example, the entry point might import itself to be able to inspect its own exports).

    The reason for these two separate export objects is that CommonJS modules are supposed to see a special export called `__esModule` which indicates that the module used to be an ES module, while ES modules are not supposed to see any automatically-added export named `__esModule`. This matters for real-world code both because people sometimes iterate over the properties of ES module export namespace objects and because some people write ES module code containing their own exports named `__esModule` that they expect other ES module code to be able to read.

    However, this change to split exports into two separate objects broke ES module re-exports in the edge case where the imported module is involved in an import cycle. This happened because the CommonJS `module.exports` object was no longer mutated as exports were added. Instead it was being initialized at the end of the generated file after the import statements to other modules (which are converted into `require()` calls). This release changes `module.exports` initialization to happen earlier in the file and then double-writes further exports to both the ES module and CommonJS module export objects.

    This fix was contributed by [@indutny](https://github.com/indutny).

## 0.14.26

* Fix a tree shaking regression regarding `var` declarations ([#2080](https://github.com/evanw/esbuild/issues/2080), [#2085](https://github.com/evanw/esbuild/pull/2085), [#2098](https://github.com/evanw/esbuild/issues/2098), [#2099](https://github.com/evanw/esbuild/issues/2099))

    Version 0.14.8 of esbuild enabled removal of duplicate function declarations when minification is enabled (see [#610](https://github.com/evanw/esbuild/issues/610)):

    ```js
    // Original code
    function x() { return 1 }
    console.log(x())
    function x() { return 2 }

    // Output (with --minify-syntax)
    console.log(x());
    function x() {
      return 2;
    }
    ```

    This transformation is safe because function declarations are "hoisted" in JavaScript, which means they are all done first before any other code is evaluted. This means the last function declaration will overwrite all previous function declarations with the same name.

    However, this introduced an unintentional regression for `var` declarations in which all but the last declaration was dropped if tree-shaking was enabled. This only happens for top-level `var` declarations that re-declare the same variable multiple times. This regression has now been fixed:

    ```js
    // Original code
    var x = 1
    console.log(x)
    var x = 2

    // Old output (with --tree-shaking=true)
    console.log(x);
    var x = 2;

    // New output (with --tree-shaking=true)
    var x = 1;
    console.log(x);
    var x = 2;
    ```

    This case now has test coverage.

* Add support for parsing "instantiation expressions" from TypeScript 4.7 ([#2038](https://github.com/evanw/esbuild/pull/2038))

    The upcoming version of TypeScript now lets you specify `<...>` type parameters on a JavaScript identifier without using a call expression:

    ```ts
    const ErrorMap = Map<string, Error>;  // new () => Map<string, Error>
    const errorMap = new ErrorMap();  // Map<string, Error>
    ```

    With this release, esbuild can now parse these new type annotations. This feature was contributed by [@g-plane](https://github.com/g-plane).

* Avoid `new Function` in esbuild's library code ([#2081](https://github.com/evanw/esbuild/issues/2081))

    Some JavaScript environments such as Cloudflare Workers or Deno Deploy don't allow `new Function` because they disallow dynamic JavaScript evaluation. Previously esbuild's WebAssembly-based library used this to construct the WebAssembly worker function. With this release, the code is now inlined without using `new Function` so it will be able to run even when this restriction is in place.

* Drop superfluous `__name()` calls ([#2062](https://github.com/evanw/esbuild/pull/2062))

    When the `--keep-names` option is specified, esbuild inserts calls to a `__name` helper function to ensure that the `.name` property on function and class objects remains consistent even if the function or class name is renamed to avoid a name collision or because name minification is enabled. With this release, esbuild will now try to omit these calls to the `__name` helper function when the name of the function or class object was not renamed during the linking process after all:

    ```js
    // Original code
    import { foo as foo1 } from 'data:text/javascript,export function foo() { return "foo1" }'
    import { foo as foo2 } from 'data:text/javascript,export function foo() { return "foo2" }'
    console.log(foo1.name, foo2.name)

    // Old output (with --bundle --keep-names)
    (() => {
      var __defProp = Object.defineProperty;
      var __name = (target, value) => __defProp(target, "name", { value, configurable: true });
      function foo() {
        return "foo1";
      }
      __name(foo, "foo");
      function foo2() {
        return "foo2";
      }
      __name(foo2, "foo");
      console.log(foo.name, foo2.name);
    })();

    // New output (with --bundle --keep-names)
    (() => {
      var __defProp = Object.defineProperty;
      var __name = (target, value) => __defProp(target, "name", { value, configurable: true });
      function foo() {
        return "foo1";
      }
      function foo2() {
        return "foo2";
      }
      __name(foo2, "foo");
      console.log(foo.name, foo2.name);
    })();
    ```

    Notice how one of the calls to `__name` is now no longer printed. This change was contributed by [@indutny](https://github.com/indutny).

## 0.14.25

* Reduce minification of CSS transforms to avoid Safari bugs ([#2057](https://github.com/evanw/esbuild/issues/2057))

    In Safari, applying a 3D CSS transform to an element can cause it to render in a different order than applying a 2D CSS transform even if the transformation matrix is identical. I believe this is a bug in Safari because the [CSS `transform` specification](https://drafts.csswg.org/css-transforms-1/#transform-rendering) doesn't seem to distinguish between 2D and 3D transforms as far as rendering order:

    > For elements whose layout is governed by the CSS box model, any value other than `none` for the `transform` property results in the creation of a stacking context.

    This bug means that minifying a 3D transform into a 2D transform must be avoided even though it's a valid transformation because it can cause rendering differences in Safari. Previously esbuild sometimes minified 3D CSS transforms into 2D CSS transforms but with this release, esbuild will no longer do that:

    ```css
    /* Original code */
    div { transform: matrix3d(2, 0, 0, 0, 0, 2, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1) }

    /* Old output (with --minify) */
    div{transform:scale(2)}

    /* New output (with --minify) */
    div{transform:scale3d(2,2,1)}
    ```

* Minification now takes advantage of the `?.` operator

    This adds new code minification rules that shorten code with the `?.` optional chaining operator when the result is equivalent:

    ```ts
    // Original code
    let foo = (x) => {
      if (x !== null && x !== undefined) x.y()
      return x === null || x === undefined ? undefined : x.z
    }

    // Old output (with --minify)
    let foo=n=>(n!=null&&n.y(),n==null?void 0:n.z);

    // New output (with --minify)
    let foo=n=>(n?.y(),n?.z);
    ```

    This only takes effect when minification is enabled and when the configured target environment is known to support the optional chaining operator. As always, make sure to set `--target=` to the appropriate language target if you are running the minified code in an environment that doesn't support the latest JavaScript features.

* Add source mapping information for some non-executable tokens ([#1448](https://github.com/evanw/esbuild/issues/1448))

    Code coverage tools can generate reports that tell you if any code exists that has not been run (or "covered") during your tests. You can use this information to add additional tests for code that isn't currently covered.

    Some popular JavaScript code coverage tools have bugs where they incorrectly consider lines without any executable code as uncovered, even though there's no test you could possibly write that would cause those lines to be executed. For example, they apparently complain about the lines that only contain the trailing `}` token of an object literal.

    With this release, esbuild now generates source mappings for some of these trailing non-executable tokens. This may not successfully work around bugs in code coverage tools because there are many non-executable tokens in JavaScript and esbuild doesn't map them all (the drawback of mapping these extra tokens is that esbuild will use more memory, build more slowly, and output a bigger source map). The true solution is to fix the bugs in the code coverage tools in the first place.

* Fall back to WebAssembly on Android x64 ([#2068](https://github.com/evanw/esbuild/issues/2068))

    Go's compiler supports trivial cross-compiling to almost all platforms without installing any additional software other than the Go compiler itself. This has made it very easy for esbuild to publish native binary executables for many platforms. However, it strangely doesn't support cross-compiling to Android x64 without installing the Android build tools. So instead of publishing a native esbuild binary executable to npm, this release publishes a WebAssembly fallback build. This is essentially the same as the `esbuild-wasm` package but it's installed automatically when you install the `esbuild` package on Android x64. So packages that depend on the `esbuild` package should now work on Android x64. If you want to use a native binary executable of esbuild on Android x64, you may be able to build it yourself from source after installing the Android build tools.

* Update to Go 1.17.8

    The version of the Go compiler used to compile esbuild has been upgraded from Go 1.17.7 to Go 1.17.8, which fixes the RISC-V 64-bit build. Compiler optimizations for the RISC-V 64-bit build have now been re-enabled.

## 0.14.24

* Allow `es2022` as a target environment ([#2012](https://github.com/evanw/esbuild/issues/2012))

    TypeScript recently [added support for `es2022`](https://devblogs.microsoft.com/typescript/announcing-typescript-4-6/#target-es2022) as a compilation target so esbuild now supports this too. Support for this is preliminary as there is no published ES2022 specification yet (i.e. https://tc39.es/ecma262/2021/ exists but https://tc39.es/ecma262/2022/ is a 404 error). The meaning of esbuild's `es2022` target may change in the future when the specification is finalized. Right now I have made the `es2022` target enable support for the syntax-related [finished proposals](https://github.com/tc39/proposals/blob/main/finished-proposals.md) that are marked as `2022`:

    * Class fields
    * Class private members
    * Class static blocks
    * Ergonomic class private member checks
    * Top-level await

    I have also included the "arbitrary module namespace names" feature since I'm guessing it will end up in the ES2022 specification (this syntax feature was added to the specification without a proposal). TypeScript has [not added support for this yet](https://github.com/microsoft/TypeScript/issues/40594).

* Match `define` to strings in index expressions ([#2050](https://github.com/evanw/esbuild/issues/2050))

    With this release, configuring `--define:foo.bar=baz` now matches and replaces both `foo.bar` and `foo['bar']` expressions in the original source code. This is necessary for people who have enabled TypeScript's [`noPropertyAccessFromIndexSignature` feature](https://www.typescriptlang.org/tsconfig#noPropertyAccessFromIndexSignature), which prevents you from using normal property access syntax on a type with an index signature such as in the following code:

    ```ts
    declare let foo: { [key: string]: any }
    foo.bar // This is a type error if noPropertyAccessFromIndexSignature is enabled
    foo['bar']
    ```

    Previously esbuild would generate the following output with `--define:foo.bar=baz`:

    ```js
    baz;
    foo["bar"];
    ```

    Now esbuild will generate the following output instead:

    ```js
    baz;
    baz;
    ```

* Add `--mangle-quoted` to mangle quoted properties ([#218](https://github.com/evanw/esbuild/issues/218))

    The `--mangle-props=` flag tells esbuild to automatically rename all properties matching the provided regular expression to shorter names to save space. Previously esbuild never modified the contents of string literals. In particular, `--mangle-props=_` would mangle `foo._bar` but not `foo['_bar']`. There are some coding patterns where renaming quoted property names is desirable, such as when using TypeScript's [`noPropertyAccessFromIndexSignature` feature](https://www.typescriptlang.org/tsconfig#noPropertyAccessFromIndexSignature) or when using TypeScript's [discriminated union narrowing behavior](https://www.typescriptlang.org/docs/handbook/2/narrowing.html#discriminated-unions):

    ```ts
    interface Foo { _foo: string }
    interface Bar { _bar: number }
    declare const value: Foo | Bar
    console.log('_foo' in value ? value._foo : value._bar)
    ```

    The `'_foo' in value` check tells TypeScript to narrow the type of `value` to `Foo` in the true branch and to `Bar` in the false branch. Previously esbuild didn't mangle the property name `'_foo'` because it was inside a string literal. With this release, you can now use `--mangle-quoted` to also rename property names inside string literals:

    ```js
    // Old output (with --mangle-props=_)
    console.log("_foo" in value ? value.a : value.b);

    // New output (with --mangle-props=_ --mangle-quoted)
    console.log("a" in value ? value.a : value.b);
    ```

* Parse and discard TypeScript `export as namespace` statements ([#2070](https://github.com/evanw/esbuild/issues/2070))

    TypeScript `.d.ts` type declaration files can sometimes contain statements of the form `export as namespace foo;`. I believe these serve to declare that the module adds a property of that name to the global object. You aren't supposed to feed `.d.ts` files to esbuild so this normally doesn't matter, but sometimes esbuild can end up having to parse them. One such case is if you import a type-only package who's `main` field in `package.json` is a `.d.ts` file.

    Previously esbuild only allowed `export as namespace` statements inside a `declare` context:

    ```ts
    declare module Foo {
      export as namespace foo;
    }
    ```

    Now esbuild will also allow these statements outside of a `declare` context:

    ```ts
    export as namespace foo;
    ```

    These statements are still just ignored and discarded.

* Strip import assertions from unrecognized `import()` expressions ([#2036](https://github.com/evanw/esbuild/issues/2036))

    The new "import assertions" JavaScript language feature adds an optional second argument to dynamic `import()` expressions, which esbuild does support. However, this optional argument must be stripped when targeting older JavaScript environments for which this second argument would be a syntax error. Previously esbuild failed to strip this second argument in cases when the first argument to `import()` wasn't a string literal. This problem is now fixed:

    ```js
    // Original code
    console.log(import(foo, { assert: { type: 'json' } }))

    // Old output (with --target=es6)
    console.log(import(foo, { assert: { type: "json" } }));

    // New output (with --target=es6)
    console.log(import(foo));
    ```

* Remove simplified statement-level literal expressions ([#2063](https://github.com/evanw/esbuild/issues/2063))

    With this release, esbuild now removes simplified statement-level expressions if the simplified result is a literal expression even when minification is disabled. Previously this was only done when minification is enabled. This change was only made because some people are bothered by seeing top-level literal expressions. This change has no effect on code behavior.

* Ignore `.d.ts` rules in `paths` in `tsconfig.json` files ([#2074](https://github.com/evanw/esbuild/issues/2074), [#2075](https://github.com/evanw/esbuild/pull/2075))

    TypeScript's `tsconfig.json` configuration file has a `paths` field that lets you remap import paths to alternative files on the file system. This field is interpreted by esbuild during bundling so that esbuild's behavior matches that of the TypeScript type checker. However, people sometimes override import paths to JavaScript files to instead point to a `.d.ts` TypeScript type declaration file for that JavaScript file. The intent of this is to just use the remapping for type information and not to actually import the `.d.ts` file during the build.

    With this release, esbuild will now ignore rules in `paths` that result in a `.d.ts` file during path resolution. This means code that does this should now be able to be bundled without modifying its `tsconfig.json` file to remove the `.d.ts` rule. This change was contributed by [@magic-akari](https://github.com/magic-akari).

* Disable Go compiler optimizations for the Linux RISC-V 64bit build ([#2035](https://github.com/evanw/esbuild/pull/2035))

    Go's RISC-V 64bit compiler target has a fatal compiler optimization bug that causes esbuild to crash when it's run: https://github.com/golang/go/issues/51101. As a temporary workaround until a version of the Go compiler with the fix is published, Go compiler optimizations have been disabled for RISC-V. The 7.7mb esbuild binary executable for RISC-V is now 8.7mb instead. This workaround was contributed by [@piggynl](https://github.com/piggynl).

## 0.14.23

* Update feature database to indicate that node 16.14+ supports import assertions ([#2030](https://github.com/evanw/esbuild/issues/2030))

    Node versions 16.14 and above now support import assertions according to [these release notes](https://github.com/nodejs/node/blob/6db686710ee1579452b2908a7a41b91cb729b944/doc/changelogs/CHANGELOG_V16.md#16.14.0). This release updates esbuild's internal feature compatibility database with this information, so esbuild no longer strips import assertions with `--target=node16.14`:

    ```js
    // Original code
    import data from './package.json' assert { type: 'json' }
    console.log(data)

    // Old output (with --target=node16.14)
    import data from "./package.json";
    console.log(data);

    // New output (with --target=node16.14)
    import data from "./package.json" assert { type: "json" };
    console.log(data);
    ```

* Basic support for CSS `@layer` rules ([#2027](https://github.com/evanw/esbuild/issues/2027))

    This adds basic parsing support for a new CSS feature called `@layer` that changes how the CSS cascade works. Adding parsing support for this rule to esbuild means esbuild can now minify the contents of `@layer` rules:

    ```css
    /* Original code */
    @layer a {
      @layer b {
        div {
          color: yellow;
          margin: 0.0px;
        }
      }
    }

    /* Old output (with --minify) */
    @layer a{@layer b {div {color: yellow; margin: 0px;}}}

    /* New output (with --minify) */
    @layer a.b{div{color:#ff0;margin:0}}
    ```

    You can read more about `@layer` here:

    * Documentation: https://developer.mozilla.org/en-US/docs/Web/CSS/@layer
    * Motivation: https://developer.chrome.com/blog/cascade-layers/

    Note that the support added in this release is only for parsing and printing `@layer` rules. The bundler does not yet know about these rules and bundling with `@layer` may result in behavior changes since these new rules have unusual ordering constraints that behave differently than all other CSS rules. Specifically the order is derived from the _first_ instance while with every other CSS rule, the order is derived from the _last_ instance.

## 0.14.22

* Preserve whitespace for token lists that look like CSS variable declarations ([#2020](https://github.com/evanw/esbuild/issues/2020))

    Previously esbuild removed the whitespace after the CSS variable declaration in the following CSS:

    ```css
    /* Original input */
    @supports (--foo: ){html{background:green}}

    /* Previous output */
    @supports (--foo:){html{background:green}}
    ```

    However, that broke rendering in Chrome as it caused Chrome to ignore the entire rule. This did not break rendering in Firefox and Safari, so there's a browser bug either with Chrome or with both Firefox and Safari. In any case, esbuild now preserves whitespace after the CSS variable declaration in this case.

* Ignore legal comments when merging adjacent duplicate CSS rules ([#2016](https://github.com/evanw/esbuild/issues/2016))

    This release now generates more compact minified CSS when there are legal comments in between two adjacent rules with identical content:

    ```css
    /* Original code */
    a { color: red }
    /* @preserve */
    b { color: red }

    /* Old output (with --minify) */
    a{color:red}/* @preserve */b{color:red}

    /* New output (with --minify) */
    a,b{color:red}/* @preserve */
    ```

* Block `onResolve` and `onLoad` until `onStart` ends ([#1967](https://github.com/evanw/esbuild/issues/1967))

    This release changes the semantics of the `onStart` callback. All `onStart` callbacks from all plugins are run concurrently so that a slow plugin doesn't hold up the entire build. That's still the case. However, previously the only thing waiting for the `onStart` callbacks to finish was the end of the build. This meant that `onResolve` and/or `onLoad` callbacks could sometimes run before `onStart` had finished. This was by design but violated user expectations. With this release, all `onStart` callbacks must finish before any `onResolve` and/or `onLoad` callbacks are run.

* Add a self-referential `default` export to the JS API ([#1897](https://github.com/evanw/esbuild/issues/1897))

    Some people try to use esbuild's API using `import esbuild from 'esbuild'` instead of `import * as esbuild from 'esbuild'` (i.e. using a default import instead of a namespace import). There is no `default` export so that wasn't ever intended to work. But it would work sometimes depending on which tools you used and how they were configured so some people still wrote code this way. This release tries to make that work by adding a self-referential `default` export that is equal to esbuild's module namespace object.

    More detail: The published package for esbuild's JS API is in CommonJS format, although the source code for esbuild's JS API is in ESM format. The original ESM code for esbuild's JS API has no export named `default` so using a default import like this doesn't work with Babel-compatible toolchains (since they respect the semantics of the original ESM code). However, it happens to work with node-compatible toolchains because node's implementation of importing CommonJS from ESM broke compatibility with existing conventions and automatically creates a `default` export which is set to `module.exports`. This is an unfortunate compatibility headache because it means the `default` import only works sometimes. This release tries to fix this by explicitly creating a self-referential `default` export. It now doesn't matter if you do `esbuild.build()`, `esbuild.default.build()`, or `esbuild.default.default.build()` because they should all do the same thing. Hopefully this means people don't have to deal with this problem anymore.

* Handle `write` errors when esbuild's child process is killed ([#2007](https://github.com/evanw/esbuild/issues/2007))

    If you type Ctrl+C in a terminal when a script that uses esbuild's JS library is running, esbuild's child process may be killed before the parent process. In that case calls to the `write()` syscall may fail with an `EPIPE` error. Previously this resulted in an uncaught exception because esbuild didn't handle this case. Starting with this release, esbuild should now catch these errors and redirect them into a general `The service was stopped` error which should be returned from whatever top-level API calls were in progress.

* Better error message when browser WASM bugs are present ([#1863](https://github.com/evanw/esbuild/issues/1863))

    Safari's WebAssembly implementation appears to be broken somehow, at least when running esbuild. Sometimes this manifests as a stack overflow and sometimes as a Go panic. Previously a Go panic resulted in the error message `Can't find variable: fs` but this should now result in the Go panic being printed to the console. Using esbuild's WebAssembly library in Safari is still broken but now there's a more helpful error message.

    More detail: When Go panics, it prints a stack trace to stderr (i.e. file descriptor 2). Go's WebAssembly shim calls out to node's `fs.writeSync()` function to do this, and it converts calls to `fs.writeSync()` into calls to `console.log()` in the browser by providing a shim for `fs`. However, Go's shim code stores the shim on `window.fs` in the browser. This is undesirable because it pollutes the global scope and leads to brittle code that can break if other code also uses `window.fs`. To avoid this, esbuild shadows the global object by wrapping Go's shim. But that broke bare references to `fs` since the shim is no longer stored on `window.fs`. This release now stores the shim in a local variable named `fs` so that bare references to `fs` work correctly.

* Undo incorrect dead-code elimination with destructuring ([#1183](https://github.com/evanw/esbuild/issues/1183))

    Previously esbuild eliminated these statements as dead code if tree-shaking was enabled:

    ```js
    let [a] = {}
    let { b } = null
    ```

    This is incorrect because both of these lines will throw an error when evaluated. With this release, esbuild now preserves these statements even when tree shaking is enabled.

* Update to Go 1.17.7

    The version of the Go compiler used to compile esbuild has been upgraded from Go 1.17.6 to Go 1.17.7, which contains a few [compiler and security bug fixes](https://github.com/golang/go/issues?q=milestone%3AGo1.17.7+label%3ACherryPickApproved).

## 0.14.21

* Handle an additional `browser` map edge case ([#2001](https://github.com/evanw/esbuild/pull/2001), [#2002](https://github.com/evanw/esbuild/issues/2002))

    There is a community convention around the `browser` field in `package.json` that allows remapping import paths within a package when the package is bundled for use within a browser. There isn't a rigorous definition of how it's supposed to work and every bundler implements it differently. The approach esbuild uses is to try to be "maximally compatible" in that if at least one bundler exhibits a particular behavior regarding the `browser` map that allows a mapping to work, then esbuild also attempts to make that work.

    I have a collection of test cases for this going here: https://github.com/evanw/package-json-browser-tests. However, I was missing test coverage for the edge case where a package path import in a subdirectory of the package could potentially match a remapping. The "maximally compatible" approach means replicating bugs in Browserify's implementation of the feature where package paths are mistaken for relative paths and are still remapped. Here's a specific example of an edge case that's now handled:

    * `entry.js`:

        ```js
        require('pkg/sub')
        ```

    * `node_modules/pkg/package.json`:

        ```json
        {
          "browser": {
            "./sub": "./sub/foo.js",
            "./sub/sub": "./sub/bar.js"
          }
        }
        ```

    * `node_modules/pkg/sub/foo.js`:

        ```js
        require('sub')
        ```

    * `node_modules/pkg/sub/bar.js`:

        ```js
        console.log('works')
        ```

    The import path `sub` in `require('sub')` is mistaken for a relative path by Browserify due to a bug in Browserify, so Browserify treats it as if it were `./sub` instead. This is a Browserify-specific behavior and currently doesn't happen in any other bundler (except for esbuild, which attempts to replicate Browserify's bug).

    Previously esbuild was incorrectly resolving `./sub` relative to the top-level package directory instead of to the subdirectory in this case, which meant `./sub` was incorrectly matching `"./sub": "./sub/foo.js"` instead of `"./sub/sub": "./sub/bar.js"`. This has been fixed so esbuild can now emulate Browserify's bug correctly in this edge case.

* Support for esbuild with Linux on RISC-V 64bit ([#2000](https://github.com/evanw/esbuild/pull/2000))

    With this release, esbuild now has a published binary executable for the RISC-V 64bit architecture in the [`esbuild-linux-riscv64`](https://www.npmjs.com/package/esbuild-linux-riscv64) npm package. This change was contributed by [@piggynl](https://github.com/piggynl).

## 0.14.20

* Fix property mangling and keyword properties ([#1998](https://github.com/evanw/esbuild/issues/1998))

    Previously enabling property mangling with `--mangle-props=` failed to add a space before property names after a keyword. This bug has been fixed:

    ```js
    // Original code
    class Foo {
      static foo = {
        get bar() {}
      }
    }

    // Old output (with --minify --mangle-props=.)
    class Foo{statics={gett(){}}}

    // New output (with --minify --mangle-props=.)
    class Foo{static s={get t(){}}}
    ```

## 0.14.19

* Special-case `const` inlining at the top of a scope ([#1317](https://github.com/evanw/esbuild/issues/1317), [#1981](https://github.com/evanw/esbuild/issues/1981))

    The minifier now inlines `const` variables (even across modules during bundling) if a certain set of specific requirements are met:

    * All `const` variables to be inlined are at the top of their scope
    * That scope doesn't contain any `import` or `export` statements with paths
    * All constants to be inlined are `null`, `undefined`, `true`, `false`, an integer, or a short real number
    * Any expression outside of a small list of allowed ones stops constant identification

    Practically speaking this basically means that you can trigger this optimization by just putting the constants you want inlined into a separate file (e.g. `constants.js`) and bundling everything together.

    These specific conditions are present to avoid esbuild unintentionally causing any behavior changes by inlining constants when the variable reference could potentially be evaluated before being declared. It's possible to identify more cases where constants can be inlined but doing so may require complex call graph analysis so it has not been implemented. Although these specific heuristics may change over time, this general approach to constant inlining should continue to work going forward.

    Here's an example:

    ```js
    // Original code
    const bold = 1 << 0;
    const italic = 1 << 1;
    const underline = 1 << 2;
    const font = bold | italic | underline;
    console.log(font);

    // Old output (with --minify --bundle)
    (()=>{var o=1<<0,n=1<<1,c=1<<2,t=o|n|c;console.log(t);})();

    // New output (with --minify --bundle)
    (()=>{console.log(7);})();
    ```

## 0.14.18

* Add the `--mangle-cache=` feature ([#1977](https://github.com/evanw/esbuild/issues/1977))

    This release adds a cache API for the newly-released `--mangle-props=` feature. When enabled, all mangled property renamings are recorded in the cache during the initial build. Subsequent builds reuse the renamings stored in the cache and add additional renamings for any newly-added properties. This has a few consequences:

    * You can customize what mangled properties are renamed to by editing the cache before passing it to esbuild (the cache is a map of the original name to the mangled name).

    * The cache serves as a list of all properties that were mangled. You can easily scan it to see if there are any unexpected property renamings.

    * You can disable mangling for individual properties by setting the renamed value to `false` instead of to a string. This is similar to the `--reserve-props=` setting but on a per-property basis.

    * You can ensure consistent renaming between builds (e.g. a main-thread file and a web worker, or a library and a plugin). Without this feature, each build would do an independent renaming operation and the mangled property names likely wouldn't be consistent.

    Here's how to use it:

    * CLI

        ```sh
        $ esbuild example.ts --mangle-props=_$ --mangle-cache=cache.json
        ```

    * JS API

        ```js
        let result = await esbuild.build({
          entryPoints: ['example.ts'],
          mangleProps: /_$/,
          mangleCache: {
            customRenaming_: '__c',
            disabledRenaming_: false,
          },
        })
        let updatedMangleCache = result.mangleCache
        ```

    * Go API

        ```go
        result := api.Build(api.BuildOptions{
          EntryPoints: []string{"example.ts"},
          MangleProps: "_$",
          MangleCache: map[string]interface{}{
            "customRenaming_":   "__c",
            "disabledRenaming_": false,
          },
        })
        updatedMangleCache := result.MangleCache
        ```

    The above code would do something like the following:

    ```js
    // Original code
    x = {
      customRenaming_: 1,
      disabledRenaming_: 2,
      otherProp_: 3,
    }

    // Generated code
    x = {
      __c: 1,
      disabledRenaming_: 2,
      a: 3
    };

    // Updated mangle cache
    {
      "customRenaming_": "__c",
      "disabledRenaming_": false,
      "otherProp_": "a"
    }
    ```

* Add `opera` and `ie` as possible target environments

    You can now target [Opera](https://www.opera.com/) and/or [Internet Explorer](https://www.microsoft.com/en-us/download/internet-explorer.aspx) using the `--target=` setting. For example, `--target=opera45,ie9` targets Opera 45 and Internet Explorer 9. This change does not add any additional features to esbuild's code transformation pipeline to transform newer syntax so that it works in Internet Explorer. It just adds information about what features are supported in these browsers to esbuild's internal feature compatibility table.

* Minify `typeof x !== 'undefined'` to `typeof x < 'u'`

    This release introduces a small improvement for code that does a lot of `typeof` checks against `undefined`:

    ```js
    // Original code
    y = typeof x !== 'undefined';

    // Old output (with --minify)
    y=typeof x!="undefined";

    // New output (with --minify)
    y=typeof x<"u";
    ```

    This transformation is only active when minification is enabled, and is disabled if the language target is set lower than ES2020 or if Internet Explorer is set as a target environment. Before ES2020, implementations were allowed to return non-standard values from the `typeof` operator for a few objects. Internet Explorer took advantage of this to sometimes return the string `'unknown'` instead of `'undefined'`. But this has been removed from the specification and Internet Explorer was the only engine to do this, so this minification is valid for code that does not need to target Internet Explorer.

## 0.14.17

* Attempt to fix an install script issue on Ubuntu Linux ([#1711](https://github.com/evanw/esbuild/issues/1711))

    There have been some reports of esbuild failing to install on Ubuntu Linux for a while now. I haven't been able to reproduce this myself due to lack of reproduction instructions until today, when I learned that the issue only happens when you install node from the [Snap Store](https://snapcraft.io/) instead of downloading the [official version of node](https://nodejs.org/dist/).

    The problem appears to be that when node is installed from the Snap Store, install scripts are run with stderr not being writable? This then appears to cause a problem for esbuild's install script when it uses `execFileSync` to validate that the esbuild binary is working correctly. This throws the error `EACCES: permission denied, write` even though this particular command never writes to stderr.

    Node's documentation says that stderr for `execFileSync` defaults to that of the parent process. Forcing it to `'pipe'` instead appears to fix the issue, although I still don't fully understand what's happening or why. I'm publishing this small change regardless to see if it fixes this install script edge case.

* Avoid a syntax error due to `--mangle-props=.` and `super()` ([#1976](https://github.com/evanw/esbuild/issues/1976))

    This release fixes an issue where passing `--mangle-props=.` (i.e. telling esbuild to mangle every single property) caused a syntax error with code like this:

    ```js
    class Foo {}
    class Bar extends Foo {
      constructor() {
        super();
      }
    }
    ```

    The problem was that `constructor` was being renamed to another method, which then made it no longer a constructor, which meant that `super()` was now a syntax error. I have added a workaround that avoids renaming any property named `constructor` so that esbuild doesn't generate a syntax error here.

    Despite this fix, I highly recommend not using `--mangle-props=.` because your code will almost certainly be broken. You will have to manually add every single property that you don't want mangled to `--reserve-props=` which is an excessive maintenance burden (e.g. reserve `parse` to use `JSON.parse`). Instead I recommend using a common pattern for all properties you intend to be mangled that is unlikely to appear in the APIs you use such as "ends in an underscore." This is an opt-in approach instead of an opt-out approach. It also makes it obvious when reading the code which properties will be mangled and which ones won't be.

## 0.14.16

* Support property name mangling with some TypeScript syntax features

    The newly-released `--mangle-props=` feature previously only affected JavaScript syntax features. This release adds support for using mangle props with certain TypeScript syntax features:

    * **TypeScript parameter properties**

        Parameter properties are a TypeScript-only shorthand way of initializing a class field directly from the constructor argument list. Previously parameter properties were not treated as properties to be mangled. They should now be handled correctly:

        ```ts
        // Original code
        class Foo {
          constructor(public foo_) {}
        }
        new Foo().foo_;

        // Old output (with --minify --mangle-props=_)
        class Foo{constructor(c){this.foo_=c}}new Foo().o;

        // New output (with --minify --mangle-props=_)
        class Foo{constructor(o){this.c=o}}new Foo().c;
        ```

    * **TypeScript namespaces**

        Namespaces are a TypeScript-only way to add properties to an object. Previously exported namespace members were not treated as properties to be mangled. They should now be handled correctly:

        ```ts
        // Original code
        namespace ns {
          export let foo_ = 1;
          export function bar_(x) {}
        }
        ns.bar_(ns.foo_);

        // Old output (with --minify --mangle-props=_)
        var ns;(e=>{e.foo_=1;function t(a){}e.bar_=t})(ns||={}),ns.e(ns.o);

        // New output (with --minify --mangle-props=_)
        var ns;(e=>{e.e=1;function o(p){}e.t=o})(ns||={}),ns.t(ns.e);
        ```

* Fix property name mangling for lowered class fields

    This release fixes a compiler crash with `--mangle-props=` and class fields that need to be transformed to older versions of JavaScript. The problem was that doing this is an unusual case where the mangled property name must be represented as a string instead of as a property name, which previously wasn't implemented. This case should now work correctly:

    ```js
    // Original code
    class Foo {
      static foo_;
    }
    Foo.foo_ = 0;

    // New output (with --mangle-props=_ --target=es6)
    class Foo {
    }
    __publicField(Foo, "a");
    Foo.a = 0;
    ```

## 0.14.15

* Add property name mangling with `--mangle-props=` ([#218](https://github.com/evanw/esbuild/issues/218))

    ⚠️ **Using this feature can break your code in subtle ways.** Do not use this feature unless you know what you are doing, and you know exactly how it will affect both your code and all of your dependencies. ⚠️

    This release introduces property name mangling, which is similar to an existing feature from the popular [UglifyJS](github.com/mishoo/uglifyjs) and [Terser](github.com/terser/terser) JavaScript minifiers. This setting lets you pass a regular expression to esbuild to tell esbuild to automatically rename all properties that match this regular expression. It's useful when you want to minify certain property names in your code either to make the generated code smaller or to somewhat obfuscate your code's intent.

    Here's an example that uses the regular expression `_$` to mangle all properties ending in an underscore, such as `foo_`:

    ```
    $ echo 'console.log({ foo_: 0 }.foo_)' | esbuild --mangle-props=_$
    console.log({ a: 0 }.a);
    ```

    Only mangling properties that end in an underscore is a reasonable heuristic because normal JS code doesn't typically contain identifiers like that. Browser APIs also don't use this naming convention so this also avoids conflicts with browser APIs. If you want to avoid mangling names such as [`__defineGetter__`](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Object/__defineGetter__) you could consider using a more complex regular expression such as `[^_]_$` (i.e. must end in a non-underscore followed by an underscore).

    This is a separate setting instead of being part of the minify setting because it's an unsafe transformation that does not work on arbitrary JavaScript code. It only works if the provided regular expression matches all of the properties that you want mangled and does not match any of the properties that you don't want mangled. It also only works if you do not under any circumstances reference a property name to be mangled as a string. For example, it means you can't use `Object.defineProperty(obj, 'prop', ...)` or `obj['prop']` with a mangled property. Specifically the following syntax constructs are the only ones eligible for property mangling:

    | Syntax                          | Example                 |
    |---------------------------------|-------------------------|
    | Dot property access             | `x.foo_`                |
    | Dot optional chain              | `x?.foo_`               |
    | Object properties               | `x = { foo_: y }`       |
    | Object methods                  | `x = { foo_() {} }`     |
    | Class fields                    | `class x { foo_ = y }`  |
    | Class methods                   | `class x { foo_() {} }` |
    | Object destructuring binding    | `let { foo_: x } = y`   |
    | Object destructuring assignment | `({ foo_: x } = y)`     |
    | JSX element names               | `<X.foo_></X.foo_>`     |
    | JSX attribute names             | `<X foo_={y} />`        |

    You can avoid property mangling for an individual property by quoting it as a string. However, you must consistently use quotes or no quotes for a given property everywhere for this to work. For example, `print({ foo_: 0 }.foo_)` will be mangled into `print({ a: 0 }.a)` while `print({ 'foo_': 0 }['foo_'])` will not be mangled.

    When using this feature, keep in mind that property names are only consistently mangled within a single esbuild API call but not across esbuild API calls. Each esbuild API call does an independent property mangling operation so output files generated by two different API calls may mangle the same property to two different names, which could cause the resulting code to behave incorrectly.

    If you would like to exclude certain properties from mangling, you can reserve them with the `--reserve-props=` setting. For example, this uses the regular expression `^__.*__$` to reserve all properties that start and end with two underscores, such as `__foo__`:

    ```
    $ echo 'console.log({ __foo__: 0 }.__foo__)' | esbuild --mangle-props=_$
    console.log({ a: 0 }.a);

    $ echo 'console.log({ __foo__: 0 }.__foo__)' | esbuild --mangle-props=_$ "--reserve-props=^__.*__$"
    console.log({ __foo__: 0 }.__foo__);
    ```

* Mark esbuild as supporting node v12+ ([#1970](https://github.com/evanw/esbuild/issues/1970))

    Someone requested that esbuild populate the `engines.node` field in `package.json`. This release adds the following to each `package.json` file that esbuild publishes:

    ```json
    "engines": {
      "node": ">=12"
    },
    ```

    This was chosen because it's the oldest version of node that's currently still receiving support from the node team, and so is the oldest version of node that esbuild supports: https://nodejs.org/en/about/releases/.

* Remove error recovery for invalid `//` comments in CSS ([#1965](https://github.com/evanw/esbuild/issues/1965))

    Previously esbuild treated `//` as a comment in CSS and generated a warning, even though comments in CSS use `/* ... */` instead. This allowed you to run esbuild on CSS intended for certain CSS preprocessors that support single-line comments.

    However, some people are changing from another build tool to esbuild and have a code base that relies on `//` being preserved even though it's nonsense CSS and causes the entire surrounding rule to be discarded by the browser. Presumably this nonsense CSS ended up there at some point due to an incorrectly-configured build pipeline and the site now relies on that entire rule being discarded. If esbuild interprets `//` as a comment, it could cause the rule to no longer be discarded or even cause something else to happen.

    With this release, esbuild no longer treats `//` as a comment in CSS. It still warns about it but now passes it through unmodified. This means it's no longer possible to run esbuild on CSS code containing single-line comments but it means that esbuild's behavior regarding these nonsensical CSS rules more accurately represents what happens in a browser.

## 0.14.14

* Fix bug with filename hashes and the `file` loader ([#1957](https://github.com/evanw/esbuild/issues/1957))

    This release fixes a bug where if a file name template has the `[hash]` placeholder (either `--entry-names=` or `--chunk-names=`), the hash that esbuild generates didn't include the content of the string generated by the `file` loader. Importing a file with the `file` loader causes the imported file to be copied to the output directory and causes the imported value to be the relative path from the output JS file to that copied file. This bug meant that if the `--asset-names=` setting also contained `[hash]` and the file loaded with the `file` loader was changed, the hash in the copied file name would change but the hash of the JS file would not change, which could potentially result in a stale JS file being loaded. Now the hash of the JS file will be changed too which fixes the reload issue.

* Prefer the `import` condition for entry points ([#1956](https://github.com/evanw/esbuild/issues/1956))

    The `exports` field in `package.json` maps package subpaths to file paths. The mapping can be conditional, which lets it vary in different situations. For example, you can have an `import` condition that applies when the subpath originated from a JS import statement, and a `require` condition that applies when the subpath originated from a JS require call. These are supposed to be mutually exclusive according to the specification: https://nodejs.org/api/packages.html#conditional-exports.

    However, there's a situation with esbuild where it's not immediately obvious which one should be applied: when a package name is specified as an entry point. For example, this can happen if you do `esbuild --bundle some-pkg` on the command line. In this situation `some-pkg` does not originate from either a JS import statement or a JS require call. Previously esbuild just didn't apply the `import` or `require` conditions. But that could result in path resolution failure if the package doesn't provide a back-up `default` condition, as is the case with the `is-plain-object` package.

    Starting with this release, esbuild will now use the `import` condition in this case. This appears to be how Webpack and Rollup handle this situation so this change makes esbuild consistent with other tools in the ecosystem. Parcel (the other major bundler) just doesn't handle this case at all so esbuild's behavior is not at odds with Parcel's behavior here.

* Make parsing of invalid `@keyframes` rules more robust ([#1959](https://github.com/evanw/esbuild/issues/1959))

    This improves esbuild's parsing of certain malformed `@keyframes` rules to avoid them affecting the following rule. This fix only affects invalid CSS files, and does not change any behavior for files containing valid CSS. Here's an example of the fix:

    ```css
    /* Original code */
    @keyframes x { . }
    @keyframes y { 1% { a: b; } }

    /* Old output (with --minify) */
    @keyframes x{y{1% {a: b;}}}

    /* New output (with --minify) */
    @keyframes x{.}@keyframes y{1%{a:b}}
    ```

## 0.14.13

* Be more consistent about external paths ([#619](https://github.com/evanw/esbuild/issues/619))

    The rules for marking paths as external using `--external:` grew over time as more special-cases were added. This release reworks the internal representation to be more straightforward and robust. A side effect is that wildcard patterns can now match post-resolve paths in addition to pre-resolve paths. Specifically you can now do `--external:./node_modules/*` to mark all files in the `./node_modules/` directory as external.

    This is the updated logic:

    * Before path resolution begins, import paths are checked against everything passed via an `--external:` flag. In addition, if something looks like a package path (i.e. doesn't start with `/` or `./` or `../`), import paths are checked to see if they have that package path as a path prefix (so `--external:@foo/bar` matches the import path `@foo/bar/baz`).

    * After path resolution ends, the absolute paths are checked against everything passed via `--external:` that doesn't look like a package path (i.e. that starts with `/` or `./` or `../`). But before checking, the pattern is transformed to be relative to the current working directory.

* Attempt to explain why esbuild can't run ([#1819](https://github.com/evanw/esbuild/issues/1819))

    People sometimes try to install esbuild on one OS and then copy the `node_modules` directory over to another OS without reinstalling. This works with JavaScript code but doesn't work with esbuild because esbuild is a native binary executable. This release attempts to offer a helpful error message when this happens. It looks like this:

    ```
    $ ./node_modules/.bin/esbuild
    ./node_modules/esbuild/bin/esbuild:106
              throw new Error(`
              ^

    Error:
    You installed esbuild on another platform than the one you're currently using.
    This won't work because esbuild is written with native code and needs to
    install a platform-specific binary executable.

    Specifically the "esbuild-linux-arm64" package is present but this platform
    needs the "esbuild-darwin-arm64" package instead. People often get into this
    situation by installing esbuild on Windows or macOS and copying "node_modules"
    into a Docker image that runs Linux, or by copying "node_modules" between
    Windows and WSL environments.

    If you are installing with npm, you can try not copying the "node_modules"
    directory when you copy the files over, and running "npm ci" or "npm install"
    on the destination platform after the copy. Or you could consider using yarn
    instead which has built-in support for installing a package on multiple
    platforms simultaneously.

    If you are installing with yarn, you can try listing both this platform and the
    other platform in your ".yarnrc.yml" file using the "supportedArchitectures"
    feature: https://yarnpkg.com/configuration/yarnrc/#supportedArchitectures
    Keep in mind that this means multiple copies of esbuild will be present.

    Another alternative is to use the "esbuild-wasm" package instead, which works
    the same way on all platforms. But it comes with a heavy performance cost and
    can sometimes be 10x slower than the "esbuild" package, so you may also not
    want to do that.

        at generateBinPath (./node_modules/esbuild/bin/esbuild:106:17)
        at Object.<anonymous> (./node_modules/esbuild/bin/esbuild:161:39)
        at Module._compile (node:internal/modules/cjs/loader:1101:14)
        at Object.Module._extensions..js (node:internal/modules/cjs/loader:1153:10)
        at Module.load (node:internal/modules/cjs/loader:981:32)
        at Function.Module._load (node:internal/modules/cjs/loader:822:12)
        at Function.executeUserEntryPoint [as runMain] (node:internal/modules/run_main:81:12)
        at node:internal/main/run_main_module:17:47
    ```

## 0.14.12

* Ignore invalid `@import` rules in CSS ([#1946](https://github.com/evanw/esbuild/issues/1946))

    In CSS, `@import` rules must come first before any other kind of rule (except for `@charset` rules). Previously esbuild would warn about incorrectly ordered `@import` rules and then hoist them to the top of the file. This broke people who wrote invalid `@import` rules in the middle of their files and then relied on them being ignored. With this release, esbuild will now ignore invalid `@import` rules and pass them through unmodified. This more accurately follows the CSS specification. Note that this behavior differs from other tools like Parcel, which does hoist CSS `@import` rules.

* Print invalid CSS differently ([#1947](https://github.com/evanw/esbuild/issues/1947))

    This changes how esbuild prints nested `@import` statements that are missing a trailing `;`, which is invalid CSS. The result is still partially invalid CSS, but now printed in a better-looking way:

    ```css
    /* Original code */
    .bad { @import url("other") }
    .red { background: red; }

    /* Old output (with --minify) */
    .bad{@import url(other) } .red{background: red;}}

    /* New output (with --minify) */
    .bad{@import url(other);}.red{background:red}
    ```

* Warn about CSS nesting syntax ([#1945](https://github.com/evanw/esbuild/issues/1945))

    There's a proposed [CSS syntax for nesting rules](https://drafts.csswg.org/css-nesting/) using the `&` selector, but it's not currently implemented in any browser. Previously esbuild silently passed the syntax through untransformed. With this release, esbuild will now warn when you use nesting syntax with a `--target=` setting that includes a browser.

* Warn about `}` and `>` inside JSX elements

    The `}` and `>` characters are invalid inside JSX elements according to [the JSX specification](https://facebook.github.io/jsx/) because they commonly result from typos like these that are hard to catch in code reviews:

    ```jsx
    function F() {
      return <div>></div>;
    }
    function G() {
      return <div>{1}}</div>;
    }
    ```

    The TypeScript compiler already [treats this as an error](https://github.com/microsoft/TypeScript/issues/36341), so esbuild now treats this as an error in TypeScript files too. That looks like this:

    ```
    ✘ [ERROR] The character ">" is not valid inside a JSX element

        example.tsx:2:14:
          2 │   return <div>></div>;
            │               ^
            ╵               {'>'}

      Did you mean to escape it as "{'>'}" instead?

    ✘ [ERROR] The character "}" is not valid inside a JSX element

        example.tsx:5:17:
          5 │   return <div>{1}}</div>;
            │                  ^
            ╵                  {'}'}

      Did you mean to escape it as "{'}'}" instead?
    ```

    Babel doesn't yet treat this as an error, so esbuild only warns about these characters in JavaScript files for now. Babel 8 [treats this as an error](https://github.com/babel/babel/issues/11042) but Babel 8 [hasn't been released yet](https://github.com/babel/babel/issues/10746). If you see this warning, I recommend fixing the invalid JSX syntax because it will become an error in the future.

* Warn about basic CSS property typos

    This release now generates a warning if you use a CSS property that is one character off from a known CSS property:

    ```
    ▲ [WARNING] "marign-left" is not a known CSS property

        example.css:2:2:
          2 │   marign-left: 12px;
            │   ~~~~~~~~~~~
            ╵   margin-left

      Did you mean "margin-left" instead?
    ```

## 0.14.11

* Fix a bug with enum inlining ([#1903](https://github.com/evanw/esbuild/issues/1903))

    The new TypeScript enum inlining behavior had a bug where it worked correctly if you used `export enum Foo` but not if you used `enum Foo` and then later `export { Foo }`. This release fixes the bug so enum inlining now works correctly in this case.

* Warn about `module.exports.foo = ...` in ESM ([#1907](https://github.com/evanw/esbuild/issues/1907))

    The `module` variable is treated as a global variable reference instead of as a CommonJS module reference in ESM code, which can cause problems for people that try to use both CommonJS and ESM exports in the same file. There has been a warning about this since version 0.14.9. However, the warning only covered cases like `exports.foo = bar` and `module.exports = bar` but not `module.exports.foo = bar`. This last case is now handled;

    ```
    ▲ [WARNING] The CommonJS "module" variable is treated as a global variable in an ECMAScript module and may not work as expected

        example.ts:2:0:
          2 │ module.exports.b = 1
            ╵ ~~~~~~

      This file is considered to be an ECMAScript module because of the "export" keyword here:

        example.ts:1:0:
          1 │ export let a = 1
            ╵ ~~~~~~
    ```

* Enable esbuild's CLI with Deno ([#1913](https://github.com/evanw/esbuild/issues/1913))

    This release allows you to use Deno as an esbuild installer, without also needing to use esbuild's JavaScript API. You can now use esbuild's CLI with Deno:

    ```
    deno run --allow-all "https://deno.land/x/esbuild@v0.14.11/mod.js" --version
    ```

## 2021

All esbuild versions published in the year 2021 (versions 0.8.29 through 0.14.10) can be found in [CHANGELOG-2021.md](./CHANGELOG-2021.md).

## 2020

All esbuild versions published in the year 2020 (versions 0.3.0 through 0.8.28) can be found in [CHANGELOG-2020.md](./CHANGELOG-2020.md).
