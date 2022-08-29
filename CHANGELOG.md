# Changelog

## Unreleased

* Automatically fix invalid `supported` configurations ([#2497](https://github.com/evanw/esbuild/issues/2497))

    The `--target=` setting lets you tell esbuild to target a specific version of one or more JavaScript runtimes such as `chrome80,node14` and esbuild will restrict its output to only those features supported by all targeted JavaScript runtimes. More recently, esbuild introduced the `--supported:` setting that lets you override which features are supported on a per-feature basis. However, this now lets you configure nonsensical things such as `--supported:async-await=false --supported:async-generator=true`. Previously doing this could result in esbuild building successfully but producing invalid output.

    Starting with this release, esbuild will now attempt to automatically fix nonsensical feature override configurations by introducing more overrides until the configuration makes sense. So now the configuration from previous example will be changed such that `async-await=false` implies `async-generator=false`. The full list of implications that were introduced is below:

    * `async-await=false` implies:
        * `async-generator=false`
        * `for-await=false`
        * `top-level-await=false`

    * `generator=false` implies:
        * `async-generator=false`
        * `for-await=false`

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

    The upcoming version of TypeScript adds the `moduleSuffixes` field to `tsconfig.json` that introduces more rules to import path resolution. Setting `moduleSuffixes` to `[".ios", ".native", ""]` will try to look at the the relative files `./foo.ios.ts`, `./foo.native.ts`, and finally `./foo.ts` for an import path of `./foo`. Note that the empty string `""` in `moduleSuffixes` is necessary for TypeScript to also look-up `./foo.ts`. This was announced in the [TypeScript 4.7 beta blog post](https://devblogs.microsoft.com/typescript/announcing-typescript-4-7-beta/#resolution-customization-with-modulesuffixes).

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

    The upcoming version of TypeScript now lets you specify `in` and/or `out` on certain type parameters (specifically only on a type alias, an interface declaration, or a class declaration). These modifiers control type paramemter covariance and contravariance:

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

    The character tables that determine which characters form valid JavaScript identifiers have been updated from Unicode version 13.0.0 to the newly release Unicode version 14.0.0. I'm not putting an example in the release notes because all of the new characters will likely just show up as little squares since fonts haven't been updated yet. But you can read https://www.unicode.org/versions/Unicode14.0.0/#Summary for more information about the changes.

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

    [Browserify](https://browserify.org/) generates code containing the expression `typeof require == "function" && require` which then ends up in a lot of npm packages. This expression is problematic because bundling involves statically determining all source files and their dependencies. Using `require` dynamically like this defeats the static analysis. It's also problematic because esbuild replaces `typeof require == "function"` with `true` since `require` is a function at compile-time when bundling. Then `true && require` becomes `require` in the generated code, which crashes at run time.

    Previously esbuild would generate an error for these expressions. Now esbuild replaces `typeof require == "function" && require` with `false` when targeting the browser and `require` when targeting node. This matches the intent of the browserify prelude snippet and allows esbuild to build libraries containing this code without errors or warnings.

* Allow dynamic dependencies ([#113](https://github.com/evanw/esbuild/issues/113))

    Bundling `require()` or `import()` when the argument isn't a string literal is a dynamic dependency. The dependency path relies on dynamic run-time behavior and cannot be statically determined by esbuild at bundle time.

    Dynamic dependencies used to be an error but are now just a warning. Builds containing them now succeed and the generated code contains the `require()` or `import()` expression. This is useful either when the dynamic dependency is intentional or when you know the dynamic dependency won't ever be triggered. Doing this still generates a warning to alert you that some code was excluded from the bundle and because these expressions may still crash at run time if the imported path isn't valid.

## 0.5.2

* Fix a regression with `--define` and identifiers

    The API refactor introduced a regression where using a `--define` flag to replace something with an identifier followed by another `--define` flag unintentionally caused the first `--define` to use the value from the second `--define` for replacement. This regression was caused by a loop that was added around a Go closure, which caused all closures in that loop to close over the same variable. The bug has been fixed.

* Fix interpretation of legacy `-->` single-line HTML comments

    The `-->` sequence starts a single-line comment similar to `//`. This is legacy behavior from [annex B](https://www.ecma-international.org/ecma-262/6.0/#sec-html-like-comments) under the name `SingleLineHTMLCloseComment`. However, `-->` was incorrectly treated as the start of a comment even when it didn't come at the beginning of the line. Now `-->` only starts a comment if there are no tokens before it on that line.

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
