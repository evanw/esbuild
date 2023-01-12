# Changelog

## 0.16.17

* Fix additional comment-related regressions ([#2814](https://github.com/evanw/esbuild/issues/2814))

    This release fixes more edge cases where the new comment preservation behavior that was added in 0.16.14 could introduce syntax errors. Specifically:

    ```js
    x = () => (/* comment */ {})
    for ((/* comment */ let).x of y) ;
    function *f() { yield (/* comment */class {}) }
    ```

    These cases caused esbuild to generate code with a syntax error in version 0.16.14 or above. These bugs have now been fixed.

## 0.16.16

* Fix a regression caused by comment preservation ([#2805](https://github.com/evanw/esbuild/issues/2805))

    The new comment preservation behavior that was added in 0.16.14 introduced a regression where comments in certain locations could cause esbuild to omit certain necessary parentheses in the output. The outermost parentheses were incorrectly removed for the following syntax forms, which then introduced syntax errors:

    ```js
    (/* comment */ { x: 0 }).x;
    (/* comment */ function () { })();
    (/* comment */ class { }).prototype;
    ```

    This regression has been fixed.

## 0.16.15

* Add `format` to input files in the JSON metafile data

    When `--metafile` is enabled, input files may now have an additional `format` field that indicates the export format used by this file. When present, the value will either be `cjs` for CommonJS-style exports or `esm` for ESM-style exports. This can be useful in bundle analysis.

    For example, esbuild's new [Bundle Size Analyzer](https://esbuild.github.io/analyze/) now uses this information to visualize whether ESM or CommonJS was used for each directory and file of source code (click on the CJS/ESM bar at the top).

    This information is helpful when trying to reduce the size of your bundle. Using the ESM variant of a dependency instead of the CommonJS variant always results in a faster and smaller bundle because it omits CommonJS wrappers, and also may result in better tree-shaking as it allows esbuild to perform tree-shaking at the statement level instead of the module level.

* Fix a bundling edge case with dynamic import ([#2793](https://github.com/evanw/esbuild/issues/2793))

    This release fixes a bug where esbuild's bundler could produce incorrect output. The problematic edge case involves the entry point importing itself using a dynamic `import()` expression in an imported file, like this:

    ```js
    // src/a.js
    export const A = 42;

    // src/b.js
    export const B = async () => (await import(".")).A

    // src/index.js
    export * from "./a"
    export * from "./b"
    ```

* Remove new type syntax from type declarations in the `esbuild` package ([#2798](https://github.com/evanw/esbuild/issues/2798))

    Previously you needed to use TypeScript 4.3 or newer when using the `esbuild` package from TypeScript code due to the use of a getter in an interface in `node_modules/esbuild/lib/main.d.ts`. This release removes this newer syntax to allow people with versions of TypeScript as far back as TypeScript 3.5 to use this latest version of the `esbuild` package. Here is change that was made to esbuild's type declarations:

    ```diff
     export interface OutputFile {
       /** "text" as bytes */
       contents: Uint8Array;
       /** "contents" as text (changes automatically with "contents") */
    -  get text(): string;
    +  readonly text: string;
     }
    ```

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

## 2022

All esbuild versions published in the year 2022 (versions 0.14.11 through 0.16.12) can be found in [CHANGELOG-2022.md](./CHANGELOG-2022.md).

## 2021

All esbuild versions published in the year 2021 (versions 0.8.29 through 0.14.10) can be found in [CHANGELOG-2021.md](./CHANGELOG-2021.md).

## 2020

All esbuild versions published in the year 2020 (versions 0.3.0 through 0.8.28) can be found in [CHANGELOG-2020.md](./CHANGELOG-2020.md).
