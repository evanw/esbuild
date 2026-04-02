# Changelog

## 0.28.0

* Add support for `with { type: 'text' }` imports ([#4435](https://github.com/evanw/esbuild/issues/4435))

    The [import text](https://github.com/tc39/proposal-import-text) proposal has reached stage 3 in the TC39 process, which means that it's recommended for implementation. It has also already been implemented by [Deno](https://docs.deno.com/examples/importing_text/) and [Bun](https://bun.com/docs/guides/runtime/import-html). So with this release, esbuild also adds support for it. This behaves exactly the same as esbuild's existing [`text` loader](https://esbuild.github.io/content-types/#text). Here's an example:

    ```js
    import string from './example.txt' with { type: 'text' }
    console.log(string)
    ```

* Add integrity checks to fallback download path ([#4343](https://github.com/evanw/esbuild/issues/4343))

    Installing esbuild via npm is somewhat complicated with several different edge cases (see [esbuild's documentation](https://esbuild.github.io/getting-started/#additional-npm-flags) for details). If the regular installation of esbuild's platform-specific package fails, esbuild's install script attempts to download the platform-specific package itself (first with the `npm` command, and then with a HTTP request to `registry.npmjs.org` as a last resort).

    This last resort path previously didn't have any integrity checks. With this release, esbuild will now verify that the hash of the downloaded binary matches the expected hash for the current release. This means the hashes for all of esbuild's platform-specific binary packages will now be embedded in the top-level `esbuild` package. Hopefully this should work without any problems. But just in case, this change is being done as a breaking change release.

* Update the Go compiler from 1.25.7 to 1.26.1

    This upgrade should not affect anything. However, there have been some significant internal changes to the Go compiler, so esbuild could potentially behave differently in certain edge cases:

    - It now uses the [new garbage collector](https://go.dev/doc/go1.26#new-garbage-collector) that comes with Go 1.26.
    - The Go compiler is now more aggressive with allocating memory on the stack.
    - The executable format that the Go linker uses has undergone several changes.
    - The WebAssembly build now unconditionally makes use of the sign extension and non-trapping floating-point to integer conversion instructions.

    You can read the [Go 1.26 release notes](https://go.dev/doc/go1.26) for more information.

## 0.27.7

* Fix lowering of define semantics for TypeScript parameter properties ([#4421](https://github.com/evanw/esbuild/issues/4421))

    The previous release incorrectly generated class fields for TypeScript parameter properties even when the configured target environment does not support class fields. With this release, the generated class fields will now be correctly lowered in this case:

    ```ts
    // Original code
    class Foo {
      constructor(public x = 1) {}
      y = 2
    }

    // Old output (with --loader=ts --target=es2021)
    class Foo {
      constructor(x = 1) {
        this.x = x;
        __publicField(this, "y", 2);
      }
      x;
    }

    // New output (with --loader=ts --target=es2021)
    class Foo {
      constructor(x = 1) {
        __publicField(this, "x", x);
        __publicField(this, "y", 2);
      }
    }
    ```

## 0.27.5

* Fix for an async generator edge case ([#4401](https://github.com/evanw/esbuild/issues/4401), [#4417](https://github.com/evanw/esbuild/pull/4417))

    Support for transforming async generators into the equivalent state machine was added in version 0.19.0. However, the generated state machine didn't work correctly when polling async generators concurrently, such as in the following code:

    ```js
    async function* inner() { yield 1; yield 2 }
    async function* outer() { yield* inner() }
    let gen = outer()
    for await (let x of [gen.next(), gen.next()]) console.log(x)
    ```

    Previously esbuild's output of the above code behaved incorrectly when async generators were transformed (such as with `--supported:async-generator=false`). The transformation should be fixed starting with this release.

    This fix was contributed by [@2767mr](https://github.com/2767mr).

* Fix a regression when `metafile` is enabled ([#4420](https://github.com/evanw/esbuild/issues/4420), [#4418](https://github.com/evanw/esbuild/pull/4418))

    This release fixes a regression introduced by the previous release. When `metafile: true` was enabled in esbuild's JavaScript API, builds with build errors were incorrectly throwing an error about an empty JSON string instead of an object containing the build errors.

* Use define semantics for TypeScript parameter properties ([#4421](https://github.com/evanw/esbuild/issues/4421))

    Parameter properties are a TypeScript-specific code generation feature that converts constructor parameters into class fields when they are prefixed by certain keywords. When `"useDefineForClassFields": true` is present in `tsconfig.json`, the TypeScript compiler automatically generates class field declarations for parameter properties. Previously esbuild didn't do this, but esbuild will now do this starting with this release:

    ```ts
    // Original code
    class Foo {
      constructor(public x: number) {}
    }

    // Old output (with --loader=ts)
    class Foo {
      constructor(x) {
        this.x = x;
      }
    }

    // New output (with --loader=ts)
    class Foo {
      constructor(x) {
        this.x = x;
      }
      x;
    }
    ```

* Allow `es2025` as a target in `tsconfig.json` ([#4432](https://github.com/evanw/esbuild/issues/4432))

    TypeScript recently [added `es2025`](https://devblogs.microsoft.com/typescript/announcing-typescript-6-0/#es2025-option-for-target-and-lib) as a compilation target, so esbuild now supports this in the `target` field of `tsconfig.json` files, such as in the following configuration file:

    ```json
    {
      "compilerOptions": {
        "target": "ES2025"
      }
    }
    ```

    As a reminder, the only thing that esbuild uses this field for is determining whether or not to use legacy TypeScript behavior for class fields. You can read more in [the documentation](https://esbuild.github.io/content-types/#tsconfig-json).

## 0.27.4

* Fix a regression with CSS media queries ([#4395](https://github.com/evanw/esbuild/issues/4395), [#4405](https://github.com/evanw/esbuild/issues/4405), [#4406](https://github.com/evanw/esbuild/issues/4406))

    Version 0.25.11 of esbuild introduced support for parsing media queries. This unintentionally introduced a regression with printing media queries that use the `<media-type> and <media-condition-without-or>` grammar. Specifically, esbuild was failing to wrap an `or` clause with parentheses when inside `<media-condition-without-or>`. This release fixes the regression.

    Here is an example:

    ```css
    /* Original code */
    @media only screen and ((min-width: 10px) or (min-height: 10px)) {
      a { color: red }
    }

    /* Old output (incorrect) */
    @media only screen and (min-width: 10px) or (min-height: 10px) {
      a {
        color: red;
      }
    }

    /* New output (correct) */
    @media only screen and ((min-width: 10px) or (min-height: 10px)) {
      a {
        color: red;
      }
    }
    ```

* Fix an edge case with the `inject` feature ([#4407](https://github.com/evanw/esbuild/issues/4407))

    This release fixes an edge case where esbuild's `inject` feature could not be used with arbitrary module namespace names exported using an `export {} from` statement with bundling disabled and a target environment where arbitrary module namespace names is unsupported.

    With the fix, the following `inject` file:

    ```js
    import jquery from 'jquery';
    export { jquery as 'window.jQuery' };
    ```

    Can now always be rewritten as this without esbuild sometimes incorrectly generating an error:

    ```js
    export { default as 'window.jQuery' } from 'jquery';
    ```

* Attempt to improve API handling of huge metafiles ([#4329](https://github.com/evanw/esbuild/issues/4329), [#4415](https://github.com/evanw/esbuild/issues/4415))

    This release contains a few changes that attempt to improve the behavior of esbuild's JavaScript API with huge metafiles (esbuild's name for the build metadata, formatted as a JSON object). The JavaScript API is designed to return the metafile JSON as a JavaScript object in memory, which makes it easy to access from within a JavaScript-based plugin. Multiple people have encountered issues where this API breaks down with a pathologically-large metafile.

    The primary issue is that V8 has an implementation-specific maximum string length, so using the `JSON.parse` API with large enough strings is impossible. This release will now attempt to use a fallback JavaScript-based JSON parser that operates directly on the UTF8-encoded JSON bytes instead of using `JSON.parse` when the JSON metafile is too big to fit in a JavaScript string. The new fallback path has not yet been heavily-tested. The metafile will also now be generated with whitespace removed if the bundle is significantly large, which will reduce the size of the metafile JSON slightly.

    However, hitting this case is potentially a sign that something else is wrong. Ideally you wouldn't be building something so enormous that the build metadata can't even fit inside a JavaScript string. You may want to consider optimizing your project, or breaking up your project into multiple parts that are built independently. Another option could potentially be to use esbuild's command-line API instead of its JavaScript API, which is more efficient (although of course then you can't use JavaScript plugins, so it may not be an option).

## 0.27.3

* Preserve URL fragments in data URLs ([#4370](https://github.com/evanw/esbuild/issues/4370))

    Consider the following HTML, CSS, and SVG:

    * `index.html`:

        ```html
        <!DOCTYPE html>
        <html>
          <head><link rel="stylesheet" href="icons.css"></head>
          <body><div class="triangle"></div></body>
        </html>
        ```

    * `icons.css`:

        ```css
        .triangle {
          width: 10px;
          height: 10px;
          background: currentColor;
          clip-path: url(./triangle.svg#x);
        }
        ```

    * `triangle.svg`:

        ```xml
        <svg xmlns="http://www.w3.org/2000/svg">
          <defs>
            <clipPath id="x">
              <path d="M0 0H10V10Z"/>
            </clipPath>
          </defs>
        </svg>
        ```

    The CSS uses a URL fragment (the `#x`) to reference the `clipPath` element in the SVG file. Previously esbuild's CSS bundler didn't preserve the URL fragment when bundling the SVG using the `dataurl` loader, which broke the bundled CSS. With this release, esbuild will now preserve the URL fragment in the bundled CSS:

    ```css
    /* icons.css */
    .triangle {
      width: 10px;
      height: 10px;
      background: currentColor;
      clip-path: url('data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg"><defs><clipPath id="x"><path d="M0 0H10V10Z"/></clipPath></defs></svg>#x');
    }
    ```

* Parse and print CSS `@scope` rules ([#4322](https://github.com/evanw/esbuild/issues/4322))

    This release includes dedicated support for parsing `@scope` rules in CSS. These rules include optional "start" and "end" selector lists. One important consequence of this is that the local/global status of names in selector lists is now respected, which improves the correctness of esbuild's support for [CSS modules](https://esbuild.github.io/content-types/#local-css). Minification of selectors inside `@scope` rules has also improved slightly.

    Here's an example:

    ```css
    /* Original code */
    @scope (:global(.foo)) to (:local(.bar)) {
      .bar {
        color: red;
      }
    }

    /* Old output (with --loader=local-css --minify) */
    @scope (:global(.foo)) to (:local(.bar)){.o{color:red}}

    /* New output (with --loader=local-css --minify) */
    @scope(.foo)to (.o){.o{color:red}}
    ```

* Fix a minification bug with lowering of `for await` ([#4378](https://github.com/evanw/esbuild/pull/4378), [#4385](https://github.com/evanw/esbuild/pull/4385))

    This release fixes a bug where the minifier would incorrectly strip the variable in the automatically-generated `catch` clause of lowered `for await` loops. The code that generated the loop previously failed to mark the internal variable references as used.

* Update the Go compiler from v1.25.5 to v1.25.7 ([#4383](https://github.com/evanw/esbuild/issues/4383), [#4388](https://github.com/evanw/esbuild/pull/4388))

    This PR was contributed by [@MikeWillCook](https://github.com/MikeWillCook).

## 2025

All esbuild versions published in the year 2025 (versions 0.25.0 through 0.27.2) can be found in [CHANGELOG-2025.md](./CHANGELOG-2025.md).

## 2024

All esbuild versions published in the year 2024 (versions 0.19.12 through 0.24.2) can be found in [CHANGELOG-2024.md](./CHANGELOG-2024.md).

## 2023

All esbuild versions published in the year 2023 (versions 0.16.13 through 0.19.11) can be found in [CHANGELOG-2023.md](./CHANGELOG-2023.md).

## 2022

All esbuild versions published in the year 2022 (versions 0.14.11 through 0.16.12) can be found in [CHANGELOG-2022.md](./CHANGELOG-2022.md).

## 2021

All esbuild versions published in the year 2021 (versions 0.8.29 through 0.14.10) can be found in [CHANGELOG-2021.md](./CHANGELOG-2021.md).

## 2020

All esbuild versions published in the year 2020 (versions 0.3.0 through 0.8.28) can be found in [CHANGELOG-2020.md](./CHANGELOG-2020.md).
