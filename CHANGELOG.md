# Changelog

## 0.23.0

**_This release deliberately contains backwards-incompatible changes._** To avoid automatically picking up releases like this, you should either be pinning the exact version of `esbuild` in your `package.json` file (recommended) or be using a version range syntax that only accepts patch upgrades such as `^0.22.0` or `~0.22.0`. See npm's documentation about [semver](https://docs.npmjs.com/cli/v6/using-npm/semver/) for more information.

* Revert the recent change to avoid bundling dependencies for node ([#3819](https://github.com/evanw/esbuild/issues/3819))

    This release reverts the recent change in version 0.22.0 that made `--packages=external` the default behavior with `--platform=node`.  The default is now back to `--packages=bundle`.

    I've just been made aware that Amazon doesn't pin their dependencies in their "AWS CDK" product, which means that whenever esbuild publishes a new release, many people (potentially everyone?) using their SDK around the world instantly starts using it without Amazon checking that it works first. This change in version 0.22.0 happened to break their SDK. I'm amazed that things haven't broken before this point. This revert attempts to avoid these problems for Amazon's customers. Hopefully Amazon will pin their dependencies in the future.

    In addition, this is probably a sign that esbuild is used widely enough that it now needs to switch to a more complicated release model. I may have esbuild use a beta channel model for further development.

* Fix preserving collapsed JSX whitespace ([#3818](https://github.com/evanw/esbuild/issues/3818))

    When transformed, certain whitespace inside JSX elements is ignored completely if it collapses to an empty string. However, the whitespace should only be ignored if the JSX is being transformed, not if it's being preserved. This release fixes a bug where esbuild was previously incorrectly ignoring collapsed whitespace with `--jsx=preserve`. Here is an example:

    ```jsx
    // Original code
    <Foo>
      <Bar />
    </Foo>

    // Old output (with --jsx=preserve)
    <Foo><Bar /></Foo>;

    // New output (with --jsx=preserve)
    <Foo>
      <Bar />
    </Foo>;
    ```

## 0.22.0

**This release deliberately contains backwards-incompatible changes.** To avoid automatically picking up releases like this, you should either be pinning the exact version of `esbuild` in your `package.json` file (recommended) or be using a version range syntax that only accepts patch upgrades such as `^0.21.0` or `~0.21.0`. See npm's documentation about [semver](https://docs.npmjs.com/cli/v6/using-npm/semver/) for more information.

* Omit packages from bundles by default when targeting node ([#1874](https://github.com/evanw/esbuild/issues/1874), [#2830](https://github.com/evanw/esbuild/issues/2830), [#2846](https://github.com/evanw/esbuild/issues/2846), [#2915](https://github.com/evanw/esbuild/issues/2915), [#3145](https://github.com/evanw/esbuild/issues/3145), [#3294](https://github.com/evanw/esbuild/issues/3294), [#3323](https://github.com/evanw/esbuild/issues/3323), [#3582](https://github.com/evanw/esbuild/issues/3582), [#3809](https://github.com/evanw/esbuild/issues/3809), [#3815](https://github.com/evanw/esbuild/issues/3815))

    This breaking change is an experiment. People are commonly confused when using esbuild to bundle code for node (i.e. for `--platform=node`) because some packages may not be intended for bundlers, and may use node-specific features that don't work with a bundler. Even though esbuild's "getting started" instructions say to use `--packages=external` to work around this problem, many people don't read the documentation and don't do this, and are then confused when it doesn't work. So arguably this is a bad default behavior for esbuild to have if people keep tripping over this.

    With this release, esbuild will now omit packages from the bundle by default when the platform is `node` (i.e. the previous behavior of `--packages=external` is now the default in this case). _Note that your dependencies must now be present on the file system when your bundle is run._ If you don't want this behavior, you can do `--packages=bundle` to allow packages to be included in the bundle (i.e. the previous default behavior). Note that `--packages=bundle` doesn't mean all packages are bundled, just that packages are allowed to be bundled. You can still exclude individual packages from the bundle using `--external:` even when `--packages=bundle` is present.

    The `--packages=` setting considers all import paths that "look like" package imports in the original source code to be package imports. Specifically import paths that don't start with a path segment of `/` or `.` or `..` are considered to be package imports. The only two exceptions to this rule are [subpath imports](https://nodejs.org/api/packages.html#subpath-imports) (which start with a `#` character) and TypeScript path remappings via `paths` and/or `baseUrl` in `tsconfig.json` (which are applied first).

* Drop support for older platforms ([#3802](https://github.com/evanw/esbuild/issues/3802))

    This release drops support for the following operating systems:

    * Windows 7
    * Windows 8
    * Windows Server 2008
    * Windows Server 2012

    This is because the Go programming language dropped support for these operating system versions in [Go 1.21](https://go.dev/doc/go1.21#windows), and this release updates esbuild from Go 1.20 to Go 1.22.

    Note that this only affects the binary esbuild executables that are published to the `esbuild` npm package. It's still possible to compile esbuild's source code for these older operating systems. If you need to, you can compile esbuild for yourself using an older version of the Go compiler (before Go version 1.21). That might look something like this:

    ```
    git clone https://github.com/evanw/esbuild.git
    cd esbuild
    go build ./cmd/esbuild
    ./esbuild.exe --version
    ```

    In addition, this release increases the minimum required node version for esbuild's JavaScript API from node 12 to node 18. Node 18 is the oldest version of node that is still being supported (see node's [release schedule](https://nodejs.org/en/about/previous-releases) for more information). This increase is because of an incompatibility between the JavaScript that the Go compiler generates for the `esbuild-wasm` package and versions of node before node 17.4 (specifically the `crypto.getRandomValues` function).

* Update `await using` behavior to match TypeScript

    TypeScript 5.5 subtly changes the way `await using` behaves. This release updates esbuild to match these changes in TypeScript. You can read more about these changes in [microsoft/TypeScript#58624](https://github.com/microsoft/TypeScript/pull/58624).

* Allow `es2024` as a target environment

    The ECMAScript 2024 specification was just approved, so it has been added to esbuild as a possible compilation target. You can read more about the features that it adds here: [https://2ality.com/2024/06/ecmascript-2024.html](https://2ality.com/2024/06/ecmascript-2024.html). The only addition that's relevant for esbuild is the regular expression `/v` flag. With `--target=es2024`, regular expressions that use the `/v` flag will now be passed through untransformed instead of being transformed into a call to `new RegExp`.

* Publish binaries for OpenBSD on 64-bit ARM ([#3665](https://github.com/evanw/esbuild/issues/3665), [#3674](https://github.com/evanw/esbuild/pull/3674))

    With this release, you should now be able to install the `esbuild` npm package in OpenBSD on 64-bit ARM, such as on an Apple device with an M1 chip.

    This was contributed by [@ikmckenz](https://github.com/ikmckenz).

* Publish binaries for WASI (WebAssembly System Interface) preview 1 ([#3300](https://github.com/evanw/esbuild/issues/3300), [#3779](https://github.com/evanw/esbuild/pull/3779))

    The upcoming WASI (WebAssembly System Interface) standard is going to be a way to run WebAssembly outside of a JavaScript host environment. In this scenario you only need a `.wasm` file without any supporting JavaScript code. Instead of JavaScript providing the APIs for the host environment, the WASI standard specifies a "system interface" that WebAssembly code can access directly (e.g. for file system access).

    Development versions of the WASI specification are being released using preview numbers. The people behind WASI are currently working on preview 2 but the Go compiler has [released support for preview 1](https://go.dev/blog/wasi), which from what I understand is now considered an unsupported legacy release. However, some people have requested that esbuild publish binary executables that support WASI preview 1 so they can experiment with them.

    This release publishes esbuild precompiled for WASI preview 1 to the `@esbuild/wasi-preview1` package on npm (specifically the file `@esbuild/wasi-preview1/esbuild.wasm`). This binary executable has not been tested and won't be officially supported, as it's for an old preview release of a specification that has since moved in another direction. If it works for you, great! If not, then you'll likely have to wait for the ecosystem to evolve before using esbuild with WASI. For example, it sounds like perhaps WASI preview 1 doesn't include support for opening network sockets so esbuild's local development server is unlikely to work with WASI preview 1.

* Warn about `onResolve` plugins not setting a path ([#3790](https://github.com/evanw/esbuild/issues/3790))

    Plugins that return values from `onResolve` without resolving the path (i.e. without setting either `path` or `external: true`) will now cause a warning. This is because esbuild only uses return values from `onResolve` if it successfully resolves the path, and it's not good for invalid input to be silently ignored.

* Add a new Go API for running the CLI with plugins ([#3539](https://github.com/evanw/esbuild/pull/3539))

    With esbuild's Go API, you can now call `cli.RunWithPlugins(args, plugins)` to pass an array of esbuild plugins to be used during the build process. This allows you to create a CLI that behaves similarly to esbuild's CLI but with additional Go plugins enabled.

    This was contributed by [@edewit](https://github.com/edewit).

## 0.21.5

* Fix `Symbol.metadata` on classes without a class decorator ([#3781](https://github.com/evanw/esbuild/issues/3781))

    This release fixes a bug with esbuild's support for the [decorator metadata proposal](https://github.com/tc39/proposal-decorator-metadata). Previously esbuild only added the `Symbol.metadata` property to decorated classes if there was a decorator on the class element itself. However, the proposal says that the `Symbol.metadata` property should be present on all classes that have any decorators at all, not just those with a decorator on the class element itself.

* Allow unknown import attributes to be used with the `copy` loader ([#3792](https://github.com/evanw/esbuild/issues/3792))

    Import attributes (the `with` keyword on `import` statements) are allowed to alter how that path is loaded. For example, esbuild cannot assume that it knows how to load `./bagel.js` as type `bagel`:

    ```js
    // This is an error with "--bundle" without also using "--external:./bagel.js"
    import tasty from "./bagel.js" with { type: "bagel" }
    ```

    Because of that, bundling this code with esbuild is an error unless the file `./bagel.js` is external to the bundle (such as with `--bundle --external:./bagel.js`).

    However, there is an additional case where it's ok for esbuild to allow this: if the file is loaded using the `copy` loader. That's because the `copy` loader behaves similarly to `--external` in that the file is left external to the bundle. The difference is that the `copy` loader copies the file into the output folder and rewrites the import path while `--external` doesn't. That means the following will now work with the `copy` loader (such as with `--bundle --loader:.bagel=copy`):

    ```js
    // This is no longer an error with "--bundle" and "--loader:.bagel=copy"
    import tasty from "./tasty.bagel" with { type: "bagel" }
    ```

* Support import attributes with glob-style imports ([#3797](https://github.com/evanw/esbuild/issues/3797))

    This release adds support for import attributes (the `with` option) to glob-style imports (dynamic imports with certain string literal patterns as paths). These imports previously didn't support import attributes due to an oversight. So code like this will now work correctly:

    ```ts
    async function loadLocale(locale: string): Locale {
      const data = await import(`./locales/${locale}.data`, { with: { type: 'json' } })
      return unpackLocale(locale, data)
    }
    ```

    Previously this didn't work even though esbuild normally supports forcing the JSON loader using an import attribute. Attempting to do this used to result in the following error:

    ```
    ✘ [ERROR] No loader is configured for ".data" files: locales/en-US.data

        example.ts:2:28:
          2 │   const data = await import(`./locales/${locale}.data`, { with: { type: 'json' } })
            ╵                             ~~~~~~~~~~~~~~~~~~~~~~~~~~
    ```

    In addition, this change means plugins can now access the contents of `with` for glob-style imports.

* Support `${configDir}` in `tsconfig.json` files ([#3782](https://github.com/evanw/esbuild/issues/3782))

    This adds support for a new feature from the upcoming TypeScript 5.5 release. The character sequence `${configDir}` is now respected at the start of `baseUrl` and `paths` values, which are used by esbuild during bundling to correctly map import paths to file system paths. This feature lets base `tsconfig.json` files specified via `extends` refer to the directory of the top-level `tsconfig.json` file. Here is an example:

    ```json
    {
      "compilerOptions": {
        "paths": {
          "js/*": ["${configDir}/dist/js/*"]
        }
      }
    }
    ```

    You can read more in [TypeScript's blog post about their upcoming 5.5 release](https://devblogs.microsoft.com/typescript/announcing-typescript-5-5-rc/#the-configdir-template-variable-for-configuration-files). Note that this feature does not make use of template literals (you need to use `"${configDir}/dist/js/*"` not `` `${configDir}/dist/js/*` ``). The syntax for `tsconfig.json` is still just JSON with comments, and JSON syntax does not allow template literals. This feature only recognizes `${configDir}` in strings for certain path-like properties, and only at the beginning of the string.

* Fix internal error with `--supported:object-accessors=false` ([#3794](https://github.com/evanw/esbuild/issues/3794))

    This release fixes a regression in 0.21.0 where some code that was added to esbuild's internal runtime library of helper functions for JavaScript decorators fails to parse when you configure esbuild with `--supported:object-accessors=false`. The reason is that esbuild introduced code that does `{ get [name]() {} }` which uses both the `object-extensions` feature for the `[name]` and the `object-accessors` feature for the `get`, but esbuild was incorrectly only checking for `object-extensions` and not for `object-accessors`. Additional tests have been added to avoid this type of issue in the future. A workaround for this issue in earlier releases is to also add `--supported:object-extensions=false`.

## 0.21.4

* Update support for import assertions and import attributes in node ([#3778](https://github.com/evanw/esbuild/issues/3778))

    Import assertions (the `assert` keyword) have been removed from node starting in v22.0.0. So esbuild will now strip them and generate a warning with `--target=node22` or above:

    ```
    ▲ [WARNING] The "assert" keyword is not supported in the configured target environment ("node22") [assert-to-with]

        example.mjs:1:40:
          1 │ import json from "esbuild/package.json" assert { type: "json" }
            │                                         ~~~~~~
            ╵                                         with

      Did you mean to use "with" instead of "assert"?
    ```

    Import attributes (the `with` keyword) have been backported to node 18 starting in v18.20.0. So esbuild will no longer strip them with `--target=node18.N` if `N` is 20 or greater.

* Fix `for await` transform when a label is present

    This release fixes a bug where the `for await` transform, which wraps the loop in a `try` statement, previously failed to also move the loop's label into the `try` statement. This bug only affects code that uses both of these features in combination. Here's an example of some affected code:

    ```js
    // Original code
    async function test() {
      outer: for await (const x of [Promise.resolve([0, 1])]) {
        for (const y of x) if (y) break outer
        throw 'fail'
      }
    }

    // Old output (with --target=es6)
    function test() {
      return __async(this, null, function* () {
        outer: try {
          for (var iter = __forAwait([Promise.resolve([0, 1])]), more, temp, error; more = !(temp = yield iter.next()).done; more = false) {
            const x = temp.value;
            for (const y of x) if (y) break outer;
            throw "fail";
          }
        } catch (temp) {
          error = [temp];
        } finally {
          try {
            more && (temp = iter.return) && (yield temp.call(iter));
          } finally {
            if (error)
              throw error[0];
          }
        }
      });
    }

    // New output (with --target=es6)
    function test() {
      return __async(this, null, function* () {
        try {
          outer: for (var iter = __forAwait([Promise.resolve([0, 1])]), more, temp, error; more = !(temp = yield iter.next()).done; more = false) {
            const x = temp.value;
            for (const y of x) if (y) break outer;
            throw "fail";
          }
        } catch (temp) {
          error = [temp];
        } finally {
          try {
            more && (temp = iter.return) && (yield temp.call(iter));
          } finally {
            if (error)
              throw error[0];
          }
        }
      });
    }
    ```

* Do additional constant folding after cross-module enum inlining ([#3416](https://github.com/evanw/esbuild/issues/3416), [#3425](https://github.com/evanw/esbuild/issues/3425))

    This release adds a few more cases where esbuild does constant folding after cross-module enum inlining.

    ```ts
    // Original code: enum.ts
    export enum Platform {
      WINDOWS = 'windows',
      MACOS = 'macos',
      LINUX = 'linux',
    }

    // Original code: main.ts
    import { Platform } from './enum';
    declare const PLATFORM: string;
    export function logPlatform() {
      if (PLATFORM == Platform.WINDOWS) console.log('Windows');
      else if (PLATFORM == Platform.MACOS) console.log('macOS');
      else if (PLATFORM == Platform.LINUX) console.log('Linux');
      else console.log('Other');
    }

    // Old output (with --bundle '--define:PLATFORM="macos"' --minify --format=esm)
    function n(){"windows"=="macos"?console.log("Windows"):"macos"=="macos"?console.log("macOS"):"linux"=="macos"?console.log("Linux"):console.log("Other")}export{n as logPlatform};

    // New output (with --bundle '--define:PLATFORM="macos"' --minify --format=esm)
    function n(){console.log("macOS")}export{n as logPlatform};
    ```

* Pass import attributes to on-resolve plugins ([#3384](https://github.com/evanw/esbuild/issues/3384), [#3639](https://github.com/evanw/esbuild/issues/3639), [#3646](https://github.com/evanw/esbuild/issues/3646))

    With this release, on-resolve plugins will now have access to the import attributes on the import via the `with` property of the arguments object. This mirrors the `with` property of the arguments object that's already passed to on-load plugins. In addition, you can now pass `with` to the `resolve()` API call which will then forward that value on to all relevant plugins. Here's an example of a plugin that can now be written:

    ```js
    const examplePlugin = {
      name: 'Example plugin',
      setup(build) {
        build.onResolve({ filter: /.*/ }, args => {
          if (args.with.type === 'external')
            return { external: true }
        })
      }
    }

    require('esbuild').build({
      stdin: {
        contents: `
          import foo from "./foo" with { type: "external" }
          foo()
        `,
      },
      bundle: true,
      format: 'esm',
      write: false,
      plugins: [examplePlugin],
    }).then(result => {
      console.log(result.outputFiles[0].text)
    })
    ```

* Formatting support for the `@position-try` rule ([#3773](https://github.com/evanw/esbuild/issues/3773))

    Chrome shipped this new CSS at-rule in version 125 as part of the [CSS anchor positioning API](https://developer.chrome.com/blog/anchor-positioning-api). With this release, esbuild now knows to expect a declaration list inside of the `@position-try` body block and will format it appropriately.

* Always allow internal string import and export aliases ([#3343](https://github.com/evanw/esbuild/issues/3343))

    Import and export names can be string literals in ES2022+. Previously esbuild forbid any usage of these aliases when the target was below ES2022. Starting with this release, esbuild will only forbid such usage when the alias would otherwise end up in output as a string literal. String literal aliases that are only used internally in the bundle and are "compiled away" are no longer errors. This makes it possible to use string literal aliases with esbuild's `inject` feature even when the target is earlier than ES2022.

## 0.21.3

* Implement the decorator metadata proposal ([#3760](https://github.com/evanw/esbuild/issues/3760))

    This release implements the [decorator metadata proposal](https://github.com/tc39/proposal-decorator-metadata), which is a sub-proposal of the [decorators proposal](https://github.com/tc39/proposal-decorators). Microsoft shipped the decorators proposal in [TypeScript 5.0](https://devblogs.microsoft.com/typescript/announcing-typescript-5-0/#decorators) and the decorator metadata proposal in [TypeScript 5.2](https://devblogs.microsoft.com/typescript/announcing-typescript-5-2/#decorator-metadata), so it's important that esbuild also supports both of these features. Here's a quick example:

    ```js
    // Shim the "Symbol.metadata" symbol
    Symbol.metadata ??= Symbol('Symbol.metadata')

    const track = (_, context) => {
      (context.metadata.names ||= []).push(context.name)
    }

    class Foo {
      @track foo = 1
      @track bar = 2
    }

    // Prints ["foo", "bar"]
    console.log(Foo[Symbol.metadata].names)
    ```

    **⚠️ WARNING ⚠️**

    This proposal has been marked as "stage 3" which means "recommended for implementation". However, it's still a work in progress and isn't a part of JavaScript yet, so keep in mind that any code that uses JavaScript decorator metadata may need to be updated as the feature continues to evolve. If/when that happens, I will update esbuild's implementation to match the specification. I will not be supporting old versions of the specification.

* Fix bundled decorators in derived classes ([#3768](https://github.com/evanw/esbuild/issues/3768))

    In certain cases, bundling code that uses decorators in a derived class with a class body that references its own class name could previously generate code that crashes at run-time due to an incorrect variable name. This problem has been fixed. Here is an example of code that was compiled incorrectly before this fix:

    ```js
    class Foo extends Object {
      @(x => x) foo() {
        return Foo
      }
    }
    console.log(new Foo().foo())
    ```

* Fix `tsconfig.json` files inside symlinked directories ([#3767](https://github.com/evanw/esbuild/issues/3767))

    This release fixes an issue with a scenario involving a `tsconfig.json` file that `extends` another file from within a symlinked directory that uses the `paths` feature. In that case, the implicit `baseURL` value should be based on the real path (i.e. after expanding all symbolic links) instead of the original path. This was already done for other files that esbuild resolves but was not yet done for `tsconfig.json` because it's special-cased (the regular path resolver can't be used because the information inside `tsconfig.json` is involved in path resolution). Note that this fix no longer applies if the `--preserve-symlinks` setting is enabled.

## 0.21.2

* Correct `this` in field and accessor decorators ([#3761](https://github.com/evanw/esbuild/issues/3761))

    This release changes the value of `this` in initializers for class field and accessor decorators from the module-level `this` value to the appropriate `this` value for the decorated element (either the class or the instance). It was previously incorrect due to lack of test coverage. Here's an example of a decorator that doesn't work without this change:

    ```js
    const dec = () => function() { this.bar = true }
    class Foo { @dec static foo }
    console.log(Foo.bar) // Should be "true"
    ```

* Allow `es2023` as a target environment ([#3762](https://github.com/evanw/esbuild/issues/3762))

    TypeScript recently [added `es2023`](https://github.com/microsoft/TypeScript/pull/58140) as a compilation target, so esbuild now supports this too. There is no difference between a target of `es2022` and `es2023` as far as esbuild is concerned since the 2023 edition of JavaScript doesn't introduce any new syntax features.

## 0.21.1

* Fix a regression with `--keep-names` ([#3756](https://github.com/evanw/esbuild/issues/3756))

    The previous release introduced a regression with the `--keep-names` setting and object literals with `get`/`set` accessor methods, in which case the generated code contained syntax errors. This release fixes the regression:

    ```js
    // Original code
    x = { get y() {} }

    // Output from version 0.21.0 (with --keep-names)
    x = { get y: /* @__PURE__ */ __name(function() {
    }, "y") };

    // Output from this version (with --keep-names)
    x = { get y() {
    } };
    ```

## 0.21.0

This release doesn't contain any deliberately-breaking changes. However, it contains a very complex new feature and while all of esbuild's tests pass, I would not be surprised if an important edge case turns out to be broken. So I'm releasing this as a breaking change release to avoid causing any trouble. As usual, make sure to test your code when you upgrade.

* Implement the JavaScript decorators proposal ([#104](https://github.com/evanw/esbuild/issues/104))

    With this release, esbuild now contains an implementation of the upcoming [JavaScript decorators proposal](https://github.com/tc39/proposal-decorators). This is the same feature that shipped in [TypeScript 5.0](https://devblogs.microsoft.com/typescript/announcing-typescript-5-0/#decorators) and has been highly-requested on esbuild's issue tracker. You can read more about them in that blog post and in this other (now slightly outdated) extensive blog post here: https://2ality.com/2022/10/javascript-decorators.html. Here's a quick example:

    ```js
    const log = (fn, context) => function() {
      console.log(`before ${context.name}`)
      const it = fn.apply(this, arguments)
      console.log(`after ${context.name}`)
      return it
    }

    class Foo {
      @log static foo() {
        console.log('in foo')
      }
    }

    // Logs "before foo", "in foo", "after foo"
    Foo.foo()
    ```

    Note that this feature is different than the existing "TypeScript experimental decorators" feature that esbuild already implements. It uses similar syntax but behaves very differently, and the two are not compatible (although it's sometimes possible to write decorators that work with both). TypeScript experimental decorators will still be supported by esbuild going forward as they have been around for a long time, are very widely used, and let you do certain things that are not possible with JavaScript decorators (such as decorating function parameters). By default esbuild will parse and transform JavaScript decorators, but you can tell esbuild to parse and transform TypeScript experimental decorators instead by setting `"experimentalDecorators": true` in your `tsconfig.json` file.

    Probably at least half of the work for this feature went into creating a test suite that exercises many of the proposal's edge cases: https://github.com/evanw/decorator-tests. It has given me a reasonable level of confidence that esbuild's initial implementation is acceptable. However, I don't have access to a significant sample of real code that uses JavaScript decorators. If you're currently using JavaScript decorators in a real code base, please try out esbuild's implementation and let me know if anything seems off.

    **⚠️ WARNING ⚠️**

    This proposal has been in the works for a very long time (work began around 10 years ago in 2014) and it is finally getting close to becoming part of the JavaScript language. However, it's still a work in progress and isn't a part of JavaScript yet, so keep in mind that any code that uses JavaScript decorators may need to be updated as the feature continues to evolve. The decorators proposal is pretty close to its final form but it can and likely will undergo some small behavioral adjustments before it ends up becoming a part of the standard. If/when that happens, I will update esbuild's implementation to match the specification. I will not be supporting old versions of the specification.

* Optimize the generated code for private methods

    Previously when lowering private methods for old browsers, esbuild would generate one `WeakSet` for each private method. This mirrors similar logic for generating one `WeakSet` for each private field. Using a separate `WeakMap` for private fields is necessary as their assignment can be observable:

    ```js
    let it
    class Bar {
      constructor() {
        it = this
      }
    }
    class Foo extends Bar {
      #x = 1
      #y = null.foo
      static check() {
        console.log(#x in it, #y in it)
      }
    }
    try { new Foo } catch {}
    Foo.check()
    ```

    This prints `true false` because this partially-initialized instance has `#x` but not `#y`. In other words, it's not true that all class instances will always have all of their private fields. However, the assignment of private methods to a class instance is not observable. In other words, it's true that all class instances will always have all of their private methods. This means esbuild can lower private methods into code where all methods share a single `WeakSet`, which is smaller, faster, and uses less memory. Other JavaScript processing tools such as the TypeScript compiler already make this optimization. Here's what this change looks like:

    ```js
    // Original code
    class Foo {
      #x() { return this.#x() }
      #y() { return this.#y() }
      #z() { return this.#z() }
    }

    // Old output (--supported:class-private-method=false)
    var _x, x_fn, _y, y_fn, _z, z_fn;
    class Foo {
      constructor() {
        __privateAdd(this, _x);
        __privateAdd(this, _y);
        __privateAdd(this, _z);
      }
    }
    _x = new WeakSet();
    x_fn = function() {
      return __privateMethod(this, _x, x_fn).call(this);
    };
    _y = new WeakSet();
    y_fn = function() {
      return __privateMethod(this, _y, y_fn).call(this);
    };
    _z = new WeakSet();
    z_fn = function() {
      return __privateMethod(this, _z, z_fn).call(this);
    };

    // New output (--supported:class-private-method=false)
    var _Foo_instances, x_fn, y_fn, z_fn;
    class Foo {
      constructor() {
        __privateAdd(this, _Foo_instances);
      }
    }
    _Foo_instances = new WeakSet();
    x_fn = function() {
      return __privateMethod(this, _Foo_instances, x_fn).call(this);
    };
    y_fn = function() {
      return __privateMethod(this, _Foo_instances, y_fn).call(this);
    };
    z_fn = function() {
      return __privateMethod(this, _Foo_instances, z_fn).call(this);
    };
    ```

* Fix an obscure bug with lowering class members with computed property keys

    When class members that use newer syntax features are transformed for older target environments, they sometimes need to be relocated. However, care must be taken to not reorder any side effects caused by computed property keys. For example, the following code must evaluate `a()` then `b()` then `c()`:

    ```js
    class Foo {
      [a()]() {}
      [b()];
      static { c() }
    }
    ```

    Previously esbuild did this by shifting the computed property key _forward_ to the next spot in the evaluation order. Classes evaluate all computed keys first and then all static class elements, so if the last computed key needs to be shifted, esbuild previously inserted a static block at start of the class body, ensuring it came before all other static class elements:

    ```js
    var _a;
    class Foo {
      constructor() {
        __publicField(this, _a);
      }
      static {
        _a = b();
      }
      [a()]() {
      }
      static {
        c();
      }
    }
    ```

    However, this could cause esbuild to accidentally generate a syntax error if the computed property key contains code that isn't allowed in a static block, such as an `await` expression. With this release, esbuild fixes this problem by shifting the computed property key _backward_ to the previous spot in the evaluation order instead, which may push it into the `extends` clause or even before the class itself:

    ```js
    // Original code
    class Foo {
      [a()]() {}
      [await b()];
      static { c() }
    }

    // Old output (with --supported:class-field=false)
    var _a;
    class Foo {
      constructor() {
        __publicField(this, _a);
      }
      static {
        _a = await b();
      }
      [a()]() {
      }
      static {
        c();
      }
    }

    // New output (with --supported:class-field=false)
    var _a, _b;
    class Foo {
      constructor() {
        __publicField(this, _a);
      }
      [(_b = a(), _a = await b(), _b)]() {
      }
      static {
        c();
      }
    }
    ```

* Fix some `--keep-names` edge cases

    The [`NamedEvaluation` syntax-directed operation](https://tc39.es/ecma262/#sec-runtime-semantics-namedevaluation) in the JavaScript specification gives certain anonymous expressions a `name` property depending on where they are in the syntax tree. For example, the following initializers convey a `name` value:

    ```js
    var foo = function() {}
    var bar = class {}
    console.log(foo.name, bar.name)
    ```

    When you enable esbuild's `--keep-names` setting, esbuild generates additional code to represent this `NamedEvaluation` operation so that the value of the `name` property persists even when the identifiers are renamed (e.g. due to minification).

    However, I recently learned that esbuild's implementation of `NamedEvaluation` is missing a few cases. Specifically esbuild was missing property definitions, class initializers, logical-assignment operators. These cases should now all be handled:

    ```js
    var obj = { foo: function() {} }
    class Foo0 { foo = function() {} }
    class Foo1 { static foo = function() {} }
    class Foo2 { accessor foo = function() {} }
    class Foo3 { static accessor foo = function() {} }
    foo ||= function() {}
    foo &&= function() {}
    foo ??= function() {}
    ```

## 0.20.2

* Support TypeScript experimental decorators on `abstract` class fields ([#3684](https://github.com/evanw/esbuild/issues/3684))

    With this release, you can now use TypeScript experimental decorators on `abstract` class fields. This was silently compiled incorrectly in esbuild 0.19.7 and below, and was an error from esbuild 0.19.8 to esbuild 0.20.1. Code such as the following should now work correctly:

    ```ts
    // Original code
    const log = (x: any, y: string) => console.log(y)
    abstract class Foo { @log abstract foo: string }
    new class extends Foo { foo = '' }

    // Old output (with --loader=ts --tsconfig-raw={\"compilerOptions\":{\"experimentalDecorators\":true}})
    const log = (x, y) => console.log(y);
    class Foo {
    }
    new class extends Foo {
      foo = "";
    }();

    // New output (with --loader=ts --tsconfig-raw={\"compilerOptions\":{\"experimentalDecorators\":true}})
    const log = (x, y) => console.log(y);
    class Foo {
    }
    __decorateClass([
      log
    ], Foo.prototype, "foo", 2);
    new class extends Foo {
      foo = "";
    }();
    ```

* JSON loader now preserves `__proto__` properties ([#3700](https://github.com/evanw/esbuild/issues/3700))

    Copying JSON source code into a JavaScript file will change its meaning if a JSON object contains the `__proto__` key. A literal `__proto__` property in a JavaScript object literal sets the prototype of the object instead of adding a property named `__proto__`, while a literal `__proto__` property in a JSON object literal just adds a property named `__proto__`. With this release, esbuild will now work around this problem by converting JSON to JavaScript with a computed property key in this case:

    ```js
    // Original code
    import data from 'data:application/json,{"__proto__":{"fail":true}}'
    if (Object.getPrototypeOf(data)?.fail) throw 'fail'

    // Old output (with --bundle)
    (() => {
      // <data:application/json,{"__proto__":{"fail":true}}>
      var json_proto_fail_true_default = { __proto__: { fail: true } };

      // entry.js
      if (Object.getPrototypeOf(json_proto_fail_true_default)?.fail)
        throw "fail";
    })();

    // New output (with --bundle)
    (() => {
      // <data:application/json,{"__proto__":{"fail":true}}>
      var json_proto_fail_true_default = { ["__proto__"]: { fail: true } };

      // example.mjs
      if (Object.getPrototypeOf(json_proto_fail_true_default)?.fail)
        throw "fail";
    })();
    ```

* Improve dead code removal of `switch` statements ([#3659](https://github.com/evanw/esbuild/issues/3659))

    With this release, esbuild will now remove `switch` statements in branches when minifying if they are known to never be evaluated:

    ```js
    // Original code
    if (true) foo(); else switch (bar) { case 1: baz(); break }

    // Old output (with --minify)
    if(1)foo();else switch(bar){case 1:}

    // New output (with --minify)
    foo();
    ```

* Empty enums should behave like an object literal ([#3657](https://github.com/evanw/esbuild/issues/3657))

    TypeScript allows you to create an empty enum and add properties to it at run time. While people usually use an empty object literal for this instead of a TypeScript enum, esbuild's enum transform didn't anticipate this use case and generated `undefined` instead of `{}` for an empty enum. With this release, you can now use an empty enum to generate an empty object literal.

    ```ts
    // Original code
    enum Foo {}

    // Old output (with --loader=ts)
    var Foo = /* @__PURE__ */ ((Foo2) => {
    })(Foo || {});

    // New output (with --loader=ts)
    var Foo = /* @__PURE__ */ ((Foo2) => {
      return Foo2;
    })(Foo || {});
    ```

* Handle Yarn Plug'n'Play edge case with `tsconfig.json` ([#3698](https://github.com/evanw/esbuild/issues/3698))

    Previously a `tsconfig.json` file that `extends` another file in a package with an `exports` map failed to work when Yarn's Plug'n'Play resolution was active. This edge case should work now starting with this release.

* Work around issues with Deno 1.31+ ([#3682](https://github.com/evanw/esbuild/issues/3682))

    Version 0.20.0 of esbuild changed how the esbuild child process is run in esbuild's API for Deno. Previously it used `Deno.run` but that API is being removed in favor of `Deno.Command`. As part of this change, esbuild is now calling the new `unref` function on esbuild's long-lived child process, which is supposed to allow Deno to exit when your code has finished running even though the child process is still around (previously you had to explicitly call esbuild's `stop()` function to terminate the child process for Deno to be able to exit).

    However, this introduced a problem for Deno's testing API which now fails some tests that use esbuild with `error: Promise resolution is still pending but the event loop has already resolved`. It's unclear to me why this is happening. The call to `unref` was recommended by someone on the Deno core team, and calling Node's equivalent `unref` API has been working fine for esbuild in Node for a long time. It could be that I'm using it incorrectly, or that there's some reference counting and/or garbage collection bug in Deno's internals, or that Deno's `unref` just works differently than Node's `unref`. In any case, it's not good for Deno tests that use esbuild to be failing.

    In this release, I am removing the call to `unref` to fix this issue. This means that you will now have to call esbuild's `stop()` function to allow Deno to exit, just like you did before esbuild version 0.20.0 when this regression was introduced.

    Note: This regression wasn't caught earlier because Deno doesn't seem to fail tests that have outstanding `setTimeout` calls, which esbuild's test harness was using to enforce a maximum test runtime. Adding a `setTimeout` was allowing esbuild's Deno tests to succeed. So this regression doesn't necessarily apply to all people using tests in Deno.

## 0.20.1

* Fix a bug with the CSS nesting transform ([#3648](https://github.com/evanw/esbuild/issues/3648))

    This release fixes a bug with the CSS nesting transform for older browsers where the generated CSS could be incorrect if a selector list contained a pseudo element followed by another selector. The bug was caused by incorrectly mutating the parent rule's selector list when filtering out pseudo elements for the child rules:

    ```css
    /* Original code */
    .foo {
      &:after,
      & .bar {
        color: red;
      }
    }

    /* Old output (with --supported:nesting=false) */
    .foo .bar,
    .foo .bar {
      color: red;
    }

    /* New output (with --supported:nesting=false) */
    .foo:after,
    .foo .bar {
      color: red;
    }
    ```

* Constant folding for JavaScript inequality operators ([#3645](https://github.com/evanw/esbuild/issues/3645))

    This release introduces constant folding for the `< > <= >=` operators. The minifier will now replace these operators with `true` or `false` when both sides are compile-time numeric or string constants:

    ```js
    // Original code
    console.log(1 < 2, '🍕' > '🧀')

    // Old output (with --minify)
    console.log(1<2,"🍕">"🧀");

    // New output (with --minify)
    console.log(!0,!1);
    ```

* Better handling of `__proto__` edge cases ([#3651](https://github.com/evanw/esbuild/pull/3651))

    JavaScript object literal syntax contains a special case where a non-computed property with a key of `__proto__` sets the prototype of the object. This does not apply to computed properties or to properties that use the shorthand property syntax introduced in ES6. Previously esbuild didn't correctly preserve the "sets the prototype" status of properties inside an object literal, meaning a property that sets the prototype could accidentally be transformed into one that doesn't and vice versa. This has now been fixed:

    ```js
    // Original code
    function foo(__proto__) {
      return { __proto__: __proto__ } // Note: sets the prototype
    }
    function bar(__proto__, proto) {
      {
        let __proto__ = proto
        return { __proto__ } // Note: doesn't set the prototype
      }
    }

    // Old output
    function foo(__proto__) {
      return { __proto__ }; // Note: no longer sets the prototype (WRONG)
    }
    function bar(__proto__, proto) {
      {
        let __proto__2 = proto;
        return { __proto__: __proto__2 }; // Note: now sets the prototype (WRONG)
      }
    }

    // New output
    function foo(__proto__) {
      return { __proto__: __proto__ }; // Note: sets the prototype (correct)
    }
    function bar(__proto__, proto) {
      {
        let __proto__2 = proto;
        return { ["__proto__"]: __proto__2 }; // Note: doesn't set the prototype (correct)
      }
    }
    ```

* Fix cross-platform non-determinism with CSS color space transformations ([#3650](https://github.com/evanw/esbuild/issues/3650))

    The Go compiler takes advantage of "fused multiply and add" (FMA) instructions on certain processors which do the operation `x*y + z` without intermediate rounding. This causes esbuild's CSS color space math to differ on different processors (currently `ppc64le` and `s390x`), which breaks esbuild's guarantee of deterministic output. To avoid this, esbuild's color space math now inserts a `float64()` cast around every single math operation. This tells the Go compiler not to use the FMA optimization.

* Fix a crash when resolving a path from a directory that doesn't exist ([#3634](https://github.com/evanw/esbuild/issues/3634))

    This release fixes a regression where esbuild could crash when resolving an absolute path if the source directory for the path resolution operation doesn't exist. While this situation doesn't normally come up, it could come up when running esbuild concurrently with another operation that mutates the file system as esbuild is doing a build (such as using `git` to switch branches). The underlying problem was a regression that was introduced in version 0.18.0.

## 0.20.0

**This release deliberately contains backwards-incompatible changes.** To avoid automatically picking up releases like this, you should either be pinning the exact version of `esbuild` in your `package.json` file (recommended) or be using a version range syntax that only accepts patch upgrades such as `^0.19.0` or `~0.19.0`. See npm's documentation about [semver](https://docs.npmjs.com/cli/v6/using-npm/semver/) for more information.

This time there is only one breaking change, and it only matters for people using Deno. Deno tests that use esbuild will now fail unless you make the change described below.

* Work around API deprecations in Deno 1.40.x ([#3609](https://github.com/evanw/esbuild/issues/3609), [#3611](https://github.com/evanw/esbuild/pull/3611))

    [Deno 1.40.0](https://deno.com/blog/v1.40) was just released and introduced run-time warnings about certain APIs that esbuild uses. With this release, esbuild will work around these run-time warnings by using newer APIs if they are present and falling back to the original APIs otherwise. This should avoid the warnings without breaking compatibility with older versions of Deno.

    Unfortunately, doing this introduces a breaking change. The newer child process APIs lack a way to synchronously terminate esbuild's child process, so calling `esbuild.stop()` from within a Deno test is no longer sufficient to prevent Deno from failing a test that uses esbuild's API (Deno fails tests that create a child process without killing it before the test ends). To work around this, esbuild's `stop()` function has been changed to return a promise, and you now have to change `esbuild.stop()` to `await esbuild.stop()` in all of your Deno tests.

* Reorder implicit file extensions within `node_modules` ([#3341](https://github.com/evanw/esbuild/issues/3341), [#3608](https://github.com/evanw/esbuild/issues/3608))

    In [version 0.18.0](https://github.com/evanw/esbuild/releases/v0.18.0), esbuild changed the behavior of implicit file extensions within `node_modules` directories (i.e. in published packages) to prefer `.js` over `.ts` even when the `--resolve-extensions=` order prefers `.ts` over `.js` (which it does by default). However, doing that also accidentally made esbuild prefer `.css` over `.ts`, which caused problems for people that published packages containing both TypeScript and CSS in files with the same name.

    With this release, esbuild will reorder TypeScript file extensions immediately after the last JavaScript file extensions in the implicit file extension order instead of putting them at the end of the order. Specifically the default implicit file extension order is `.tsx,.ts,.jsx,.js,.css,.json` which used to become `.jsx,.js,.css,.json,.tsx,.ts` in `node_modules` directories. With this release it will now become `.jsx,.js,.tsx,.ts,.css,.json` instead.

    Why even rewrite the implicit file extension order at all? One reason is because the `.js` file is more likely to behave correctly than the `.ts` file. The behavior of the `.ts` file  may depend on `tsconfig.json` and the `tsconfig.json` file may not even be published, or may use `extends` to refer to a base `tsconfig.json` file that wasn't published. People can get into this situation when they forget to add all `.ts` files to their `.npmignore` file before publishing to npm. Picking `.js` over `.ts` helps make it more likely that resulting bundle will behave correctly.

## 0.19.12

* The "preserve" JSX mode now preserves JSX text verbatim ([#3605](https://github.com/evanw/esbuild/issues/3605))

    The [JSX specification](https://facebook.github.io/jsx/) deliberately doesn't specify how JSX text is supposed to be interpreted and there is no canonical way to interpret JSX text. Two most popular interpretations are Babel and TypeScript. Yes [they are different](https://twitter.com/jarredsumner/status/1456118847937781764) (esbuild [deliberately follows TypeScript](https://twitter.com/evanwallace/status/1456122279453208576) by the way).

    Previously esbuild normalized text to the TypeScript interpretation when the "preserve" JSX mode is active. However, "preserve" should arguably reproduce the original JSX text verbatim so that whatever JSX transform runs after esbuild is free to interpret it however it wants. So with this release, esbuild will now pass JSX text through unmodified:

    ```jsx
    // Original code
    let el =
      <a href={'/'} title='&apos;&quot;'> some text
        {foo}
          more text </a>

    // Old output (with --loader=jsx --jsx=preserve)
    let el = <a href="/" title={`'"`}>
      {" some text"}
      {foo}
      {"more text "}
    </a>;

    // New output (with --loader=jsx --jsx=preserve)
    let el = <a href={"/"} title='&apos;&quot;'> some text
        {foo}
          more text </a>;
    ```

* Allow JSX elements as JSX attribute values

    JSX has an obscure feature where you can use JSX elements in attribute position without surrounding them with `{...}`. It looks like this:

    ```jsx
    let el = <div data-ab=<><a/><b/></>/>;
    ```

    I think I originally didn't implement it even though it's part of the [JSX specification](https://facebook.github.io/jsx/) because it previously didn't work in TypeScript (and potentially also in Babel?). However, support for it was [silently added in TypeScript 4.8](https://github.com/microsoft/TypeScript/pull/47994) without me noticing and Babel has also since fixed their [bugs regarding this feature](https://github.com/babel/babel/pull/6006). So I'm adding it to esbuild too now that I know it's widely supported.

    Keep in mind that there is some ongoing discussion about [removing this feature from JSX](https://github.com/facebook/jsx/issues/53). I agree that the syntax seems out of place (it does away with the elegance of "JSX is basically just XML with `{...}` escapes" for something arguably harder to read, which doesn't seem like a good trade-off), but it's in the specification and TypeScript and Babel both implement it so I'm going to have esbuild implement it too. However, I reserve the right to remove it from esbuild if it's ever removed from the specification in the future. So use it with caution.

* Fix a bug with TypeScript type parsing ([#3574](https://github.com/evanw/esbuild/issues/3574))

    This release fixes a bug with esbuild's TypeScript parser where a conditional type containing a union type that ends with an infer type that ends with a constraint could fail to parse. This was caused by the "don't parse a conditional type" flag not getting passed through the union type parser. Here's an example of valid TypeScript code that previously failed to parse correctly:

    ```ts
    type InferUnion<T> = T extends { a: infer U extends number } | infer U extends number ? U : never
    ```

## 2023

All esbuild versions published in the year 2022 (versions 0.16.13 through 0.19.11) can be found in [CHANGELOG-2023.md](./CHANGELOG-2023.md).

## 2022

All esbuild versions published in the year 2022 (versions 0.14.11 through 0.16.12) can be found in [CHANGELOG-2022.md](./CHANGELOG-2022.md).

## 2021

All esbuild versions published in the year 2021 (versions 0.8.29 through 0.14.10) can be found in [CHANGELOG-2021.md](./CHANGELOG-2021.md).

## 2020

All esbuild versions published in the year 2020 (versions 0.3.0 through 0.8.28) can be found in [CHANGELOG-2020.md](./CHANGELOG-2020.md).
