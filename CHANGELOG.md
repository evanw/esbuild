# Changelog

## 0.25.4

* Add simple support for CORS to esbuild's development server ([#4125](https://github.com/evanw/esbuild/issues/4125))

    Starting with version 0.25.0, esbuild's development server is no longer configured to serve cross-origin requests. This was a deliberate change to prevent any website you visit from accessing your running esbuild development server. However, this change prevented (by design) certain use cases such as "debugging in production" by having your production website load code from `localhost` where the esbuild development server is running.

    To enable this use case, esbuild is adding a feature to allow [Cross-Origin Resource Sharing](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CORS) (a.k.a. CORS) for [simple requests](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CORS#simple_requests). Specifically, passing your origin to the new `cors` option will now set the `Access-Control-Allow-Origin` response header when the request has a matching `Origin` header. Note that this currently only works for requests that don't send a preflight `OPTIONS` request, as esbuild's development server doesn't currently support `OPTIONS` requests.

    Some examples:

    * **CLI:**

        ```
        esbuild --servedir=. --cors-origin=https://example.com
        ```

    * **JS:**

        ```js
        const ctx = await esbuild.context({})
        await ctx.serve({
          servedir: '.',
          cors: {
            origin: 'https://example.com',
          },
        })
        ```

    * **Go:**

        ```go
        ctx, _ := api.Context(api.BuildOptions{})
        ctx.Serve(api.ServeOptions{
          Servedir: ".",
          CORS: api.CORSOptions{
            Origin: []string{"https://example.com"},
          },
        })
        ```

    The special origin `*` can be used to allow any origin to access esbuild's development server. Note that this means any website you visit will be able to read everything served by esbuild.

* Pass through invalid URLs in source maps unmodified ([#4169](https://github.com/evanw/esbuild/issues/4169))

    This fixes a regression in version 0.25.0 where `sources` in source maps that form invalid URLs were not being passed through to the output. Version 0.25.0 changed the interpretation of `sources` from file paths to URLs, which means that URL parsing can now fail. Previously URLs that couldn't be parsed were replaced with the empty string. With this release, invalid URLs in `sources` should now be passed through unmodified.

* Handle exports named `__proto__` in ES modules ([#4162](https://github.com/evanw/esbuild/issues/4162), [#4163](https://github.com/evanw/esbuild/pull/4163))

    In JavaScript, the special property name `__proto__` sets the prototype when used inside an object literal. Previously esbuild's ESM-to-CommonJS conversion didn't special-case the property name of exports named `__proto__` so the exported getter accidentally became the prototype of the object literal. It's unclear what this affects, if anything, but it's better practice to avoid this by using a computed property name in this case.

    This fix was contributed by [@magic-akari](https://github.com/magic-akari).

## 0.25.3

* Fix lowered `async` arrow functions before `super()` ([#4141](https://github.com/evanw/esbuild/issues/4141), [#4142](https://github.com/evanw/esbuild/pull/4142))

    This change makes it possible to call an `async` arrow function in a constructor before calling `super()` when targeting environments without `async` support, as long as the function body doesn't reference `this`. Here's an example (notice the change from `this` to `null`):

    ```js
    // Original code
    class Foo extends Object {
      constructor() {
        (async () => await foo())()
        super()
      }
    }

    // Old output (with --target=es2016)
    class Foo extends Object {
      constructor() {
        (() => __async(this, null, function* () {
          return yield foo();
        }))();
        super();
      }
    }

    // New output (with --target=es2016)
    class Foo extends Object {
      constructor() {
        (() => __async(null, null, function* () {
          return yield foo();
        }))();
        super();
      }
    }
    ```

    Some background: Arrow functions with the `async` keyword are transformed into generator functions for older language targets such as `--target=es2016`. Since arrow functions capture `this`, the generated code forwards `this` into the body of the generator function. However, JavaScript class syntax forbids using `this` in a constructor before calling `super()`, and this forwarding was problematic since previously happened even when the function body doesn't use `this`. Starting with this release, esbuild will now only forward `this` if it's used within the function body.

    This fix was contributed by [@magic-akari](https://github.com/magic-akari).

* Fix memory leak with `--watch=true` ([#4131](https://github.com/evanw/esbuild/issues/4131), [#4132](https://github.com/evanw/esbuild/pull/4132))

    This release fixes a memory leak with esbuild when `--watch=true` is used instead of `--watch`. Previously using `--watch=true` caused esbuild to continue to use more and more memory for every rebuild, but `--watch=true` should now behave like `--watch` and not leak memory.

    This bug happened because esbuild disables the garbage collector when it's not run as a long-lived process for extra speed, but esbuild's checks for which arguments cause esbuild to be a long-lived process weren't updated for the new `--watch=true` style of boolean command-line flags. This has been an issue since this boolean flag syntax was added in version 0.14.24 in 2022. These checks are unfortunately separate from the regular argument parser because of how esbuild's internals are organized (the command-line interface is exposed as a separate [Go API](https://pkg.go.dev/github.com/evanw/esbuild/pkg/cli) so you can build your own custom esbuild CLI).

    This fix was contributed by [@mxschmitt](https://github.com/mxschmitt).

* More concise output for repeated legal comments ([#4139](https://github.com/evanw/esbuild/issues/4139))

    Some libraries have many files and also use the same legal comment text in all files. Previously esbuild would copy each legal comment to the output file. Starting with this release, legal comments duplicated across separate files will now be grouped in the output file by unique comment content.

* Allow a custom host with the development server ([#4110](https://github.com/evanw/esbuild/issues/4110))

    With this release, you can now use a custom non-IP `host` with esbuild's local development server (either with `--serve=` for the CLI or with the `serve()` call for the API). This was previously possible, but was intentionally broken in [version 0.25.0](https://github.com/evanw/esbuild/releases/v0.25.0) to fix a security issue. This change adds the functionality back except that it's now opt-in and only for a single domain name that you provide.

    For example, if you add a mapping in your `/etc/hosts` file from `local.example.com` to `127.0.0.1` and then use `esbuild --serve=local.example.com:8000`, you will now be able to visit http://local.example.com:8000/ in your browser and successfully connect to esbuild's development server (doing that would previously have been blocked by the browser). This should also work with HTTPS if it's enabled (see esbuild's documentation for how to do that).

* Add a limit to CSS nesting expansion ([#4114](https://github.com/evanw/esbuild/issues/4114))

    With this release, esbuild will now fail with an error if there is too much CSS nesting expansion. This can happen when nested CSS is converted to CSS without nesting for older browsers as expanding CSS nesting is inherently exponential due to the resulting combinatorial explosion. The expansion limit is currently hard-coded and cannot be changed, but is extremely unlikely to trigger for real code. It exists to prevent esbuild from using too much time and/or memory. Here's an example:

    ```css
    a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{color:red}}}}}}}}}}}}}}}}}}}}
    ```

    Previously, transforming this file with `--target=safari1` took 5 seconds and generated 40mb of CSS. Trying to do that will now generate the following error instead:

    ```
    ✘ [ERROR] CSS nesting is causing too much expansion

        example.css:1:60:
          1 │ a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{a,b{color:red}}}}}}}}}}}}}}}}}}}}
            ╵                                                             ^

      CSS nesting expansion was terminated because a rule was generated with 65536 selectors. This limit
      exists to prevent esbuild from using too much time and/or memory. Please change your CSS to use
      fewer levels of nesting.
    ```

* Fix path resolution edge case ([#4144](https://github.com/evanw/esbuild/issues/4144))

    This fixes an edge case where esbuild's path resolution algorithm could deviate from node's path resolution algorithm. It involves a confusing situation where a directory shares the same file name as a file (but without the file extension). See the linked issue for specific details. This appears to be a case where esbuild is correctly following [node's published resolution algorithm](https://nodejs.org/api/modules.html#all-together) but where node itself is doing something different. Specifically the step `LOAD_AS_FILE` appears to be skipped when the input ends with `..`. This release changes esbuild's behavior for this edge case to match node's behavior.

* Update Go from 1.23.7 to 1.23.8 ([#4133](https://github.com/evanw/esbuild/issues/4133), [#4134](https://github.com/evanw/esbuild/pull/4134))

    This should have no effect on existing code as this version change does not change Go's operating system support. It may remove certain reports from vulnerability scanners that detect which version of the Go compiler esbuild uses, such as for CVE-2025-22871.

    As a reminder, esbuild's development server is intended for development, not for production, so I do not consider most networking-related vulnerabilities in Go to be vulnerabilities in esbuild. Please do not use esbuild's development server in production.

## 0.25.2

* Support flags in regular expressions for the API ([#4121](https://github.com/evanw/esbuild/issues/4121))

    The JavaScript plugin API for esbuild takes JavaScript regular expression objects for the `filter` option. Internally these are translated into Go regular expressions. However, this translation previously ignored the `flags` property of the regular expression. With this release, esbuild will now translate JavaScript regular expression flags into Go regular expression flags. Specifically the JavaScript regular expression `/\.[jt]sx?$/i` is turned into the Go regular expression `` `(?i)\.[jt]sx?$` `` internally inside of esbuild's API. This should make it possible to use JavaScript regular expressions with the `i` flag. Note that JavaScript and Go don't support all of the same regular expression features, so this mapping is only approximate.

* Fix node-specific annotations for string literal export names ([#4100](https://github.com/evanw/esbuild/issues/4100))

    When node instantiates a CommonJS module, it scans the AST to look for names to expose via ESM named exports. This is a heuristic that looks for certain patterns such as `exports.NAME = ...` or `module.exports = { ... }`. This behavior is used by esbuild to "annotate" CommonJS code that was converted from ESM with the original ESM export names. For example, when converting the file `export let foo, bar` from ESM to CommonJS, esbuild appends this to the end of the file:

    ```js
    // Annotate the CommonJS export names for ESM import in node:
    0 && (module.exports = {
      bar,
      foo
    });
    ```

    However, this feature previously didn't work correctly for export names that are not valid identifiers, which can be constructed using string literal export names. The generated code contained a syntax error. That problem is fixed in this release:

    ```js
    // Original code
    let foo
    export { foo as "foo!" }

    // Old output (with --format=cjs --platform=node)
    ...
    0 && (module.exports = {
      "foo!"
    });

    // New output (with --format=cjs --platform=node)
    ...
    0 && (module.exports = {
      "foo!": null
    });
    ```

* Basic support for index source maps ([#3439](https://github.com/evanw/esbuild/issues/3439), [#4109](https://github.com/evanw/esbuild/pull/4109))

    The source map specification has an optional mode called [index source maps](https://tc39.es/ecma426/#sec-index-source-map) that makes it easier for tools to create an aggregate JavaScript file by concatenating many smaller JavaScript files with source maps, and then generate an aggregate source map by simply providing the original source maps along with some offset information. My understanding is that this is rarely used in practice. I'm only aware of two uses of it in the wild: [ClojureScript](https://clojurescript.org/) and [Turbopack](https://turbo.build/pack/).

    This release provides basic support for indexed source maps. However, the implementation has not been tested on a real app (just on very simple test input). If you are using index source maps in a real app, please try this out and report back if anything isn't working for you.

    Note that this is also not a complete implementation. For example, index source maps technically allows nesting source maps to an arbitrary depth, while esbuild's implementation in this release only supports a single level of nesting. It's unclear whether supporting more than one level of nesting is important or not given the lack of available test cases.

    This feature was contributed by [@clyfish](https://github.com/clyfish).

## 0.25.1

* Fix incorrect paths in inline source maps ([#4070](https://github.com/evanw/esbuild/issues/4070), [#4075](https://github.com/evanw/esbuild/issues/4075), [#4105](https://github.com/evanw/esbuild/issues/4105))

    This fixes a regression from version 0.25.0 where esbuild didn't correctly resolve relative paths contained within source maps in inline `sourceMappingURL` data URLs. The paths were incorrectly being passed through as-is instead of being resolved relative to the source file containing the `sourceMappingURL` comment, which was due to the data URL not being a file URL. This regression has been fixed, and this case now has test coverage.

* Fix invalid generated source maps ([#4080](https://github.com/evanw/esbuild/issues/4080), [#4082](https://github.com/evanw/esbuild/issues/4082), [#4104](https://github.com/evanw/esbuild/issues/4104), [#4107](https://github.com/evanw/esbuild/issues/4107))

    This release fixes a regression from version 0.24.1 that could cause esbuild to generate invalid source maps. Specifically under certain conditions, esbuild could generate a mapping with an out-of-bounds source index. It was introduced by code that attempted to improve esbuild's handling of "null" entries in source maps (i.e. mappings with a generated position but no original position). This regression has been fixed.

    This fix was contributed by [@jridgewell](https://github.com/jridgewell).

* Fix a regression with non-file source map paths ([#4078](https://github.com/evanw/esbuild/issues/4078))

    The format of paths in source maps that aren't in the `file` namespace was unintentionally changed in version 0.25.0. Path namespaces is an esbuild-specific concept that is optionally available for plugins to use to distinguish paths from `file` paths and from paths meant for other plugins. Previously the namespace was prepended to the path joined with a `:` character, but version 0.25.0 unintentionally failed to prepend the namespace. The previous behavior has been restored.

* Fix a crash with `switch` optimization ([#4088](https://github.com/evanw/esbuild/issues/4088))

    The new code in the previous release to optimize dead code in switch statements accidentally introduced a crash in the edge case where one or more switch case values include a function expression. This is because esbuild now visits the case values first to determine whether any cases are dead code, and then visits the case bodies once the dead code status is known. That triggered some internal asserts that guard against traversing the AST in an unexpected order. This crash has been fixed by changing esbuild to expect the new traversal ordering. Here's an example of affected code:

    ```js
    switch (x) {
      case '':
        return y.map(z => z.value)
      case y.map(z => z.key).join(','):
        return []
    }
    ```

* Update Go from 1.23.5 to 1.23.7 ([#4076](https://github.com/evanw/esbuild/issues/4076), [#4077](https://github.com/evanw/esbuild/pull/4077))

    This should have no effect on existing code as this version change does not change Go's operating system support. It may remove certain reports from vulnerability scanners that detect which version of the Go compiler esbuild uses.

    This PR was contributed by [@MikeWillCook](https://github.com/MikeWillCook).

## 0.25.0

**This release deliberately contains backwards-incompatible changes.** To avoid automatically picking up releases like this, you should either be pinning the exact version of `esbuild` in your `package.json` file (recommended) or be using a version range syntax that only accepts patch upgrades such as `^0.24.0` or `~0.24.0`. See npm's documentation about [semver](https://docs.npmjs.com/cli/v6/using-npm/semver/) for more information.

* Restrict access to esbuild's development server ([GHSA-67mh-4wv8-2f99](https://github.com/evanw/esbuild/security/advisories/GHSA-67mh-4wv8-2f99))

    This change addresses esbuild's first security vulnerability report. Previously esbuild set the `Access-Control-Allow-Origin` header to `*` to allow esbuild's development server to be flexible in how it's used for development. However, this allows the websites you visit to make HTTP requests to esbuild's local development server, which gives read-only access to your source code if the website were to fetch your source code's specific URL. You can read more information in [the report](https://github.com/evanw/esbuild/security/advisories/GHSA-67mh-4wv8-2f99).

    Starting with this release, [CORS](https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS) will now be disabled, and requests will now be denied if the host does not match the one provided to `--serve=`. The default host is `0.0.0.0`, which refers to all of the IP addresses that represent the local machine (e.g. both `127.0.0.1` and `192.168.0.1`). If you want to customize anything about esbuild's development server, you can [put a proxy in front of esbuild](https://esbuild.github.io/api/#serve-proxy) and modify the incoming and/or outgoing requests.

    In addition, the `serve()` API call has been changed to return an array of `hosts` instead of a single `host` string. This makes it possible to determine all of the hosts that esbuild's development server will accept.

    Thanks to [@sapphi-red](https://github.com/sapphi-red) for reporting this issue.

* Delete output files when a build fails in watch mode ([#3643](https://github.com/evanw/esbuild/issues/3643))

    It has been requested for esbuild to delete files when a build fails in watch mode. Previously esbuild left the old files in place, which could cause people to not immediately realize that the most recent build failed. With this release, esbuild will now delete all output files if a rebuild fails. Fixing the build error and triggering another rebuild will restore all output files again.

* Fix correctness issues with the CSS nesting transform ([#3620](https://github.com/evanw/esbuild/issues/3620), [#3877](https://github.com/evanw/esbuild/issues/3877), [#3933](https://github.com/evanw/esbuild/issues/3933), [#3997](https://github.com/evanw/esbuild/issues/3997), [#4005](https://github.com/evanw/esbuild/issues/4005), [#4037](https://github.com/evanw/esbuild/pull/4037), [#4038](https://github.com/evanw/esbuild/pull/4038))

    This release fixes the following problems:

    * Naive expansion of CSS nesting can result in an exponential blow-up of generated CSS if each nesting level has multiple selectors. Previously esbuild sometimes collapsed individual nesting levels using `:is()` to limit expansion. However, this collapsing wasn't correct in some cases, so it has been removed to fix correctness issues.

        ```css
        /* Original code */
        .parent {
          > .a,
          > .b1 > .b2 {
            color: red;
          }
        }

        /* Old output (with --supported:nesting=false) */
        .parent > :is(.a, .b1 > .b2) {
          color: red;
        }

        /* New output (with --supported:nesting=false) */
        .parent > .a,
        .parent > .b1 > .b2 {
          color: red;
        }
        ```

        Thanks to [@tim-we](https://github.com/tim-we) for working on a fix.

    * The `&` CSS nesting selector can be repeated multiple times to increase CSS specificity. Previously esbuild ignored this possibility and incorrectly considered `&&` to have the same specificity as `&`. With this release, this should now work correctly:

        ```css
        /* Original code (color should be red) */
        div {
          && { color: red }
          & { color: blue }
        }

        /* Old output (with --supported:nesting=false) */
        div {
          color: red;
        }
        div {
          color: blue;
        }

        /* New output (with --supported:nesting=false) */
        div:is(div) {
          color: red;
        }
        div {
          color: blue;
        }
        ```

        Thanks to [@CPunisher](https://github.com/CPunisher) for working on a fix.

    * Previously transforming nested CSS incorrectly removed leading combinators from within pseudoclass selectors such as `:where()`. This edge case has been fixed and how has test coverage.

        ```css
        /* Original code */
        a b:has(> span) {
          a & {
            color: green;
          }
        }

        /* Old output (with --supported:nesting=false) */
        a :is(a b:has(span)) {
          color: green;
        }

        /* New output (with --supported:nesting=false) */
        a :is(a b:has(> span)) {
          color: green;
        }
        ```

        This fix was contributed by [@NoremacNergfol](https://github.com/NoremacNergfol).

    * The CSS minifier contains logic to remove the `&` selector when it can be implied, which happens when there is only one and it's the leading token. However, this logic was incorrectly also applied to selector lists inside of pseudo-class selectors such as `:where()`. With this release, the minifier will now avoid applying this logic in this edge case:

        ```css
        /* Original code */
        .a {
          & .b { color: red }
          :where(& .b) { color: blue }
        }

        /* Old output (with --minify) */
        .a{.b{color:red}:where(.b){color:#00f}}

        /* New output (with --minify) */
        .a{.b{color:red}:where(& .b){color:#00f}}
        ```

* Fix some correctness issues with source maps ([#1745](https://github.com/evanw/esbuild/issues/1745), [#3183](https://github.com/evanw/esbuild/issues/3183), [#3613](https://github.com/evanw/esbuild/issues/3613), [#3982](https://github.com/evanw/esbuild/issues/3982))

    Previously esbuild incorrectly treated source map path references as file paths instead of as URLs. With this release, esbuild will now treat source map path references as URLs. This fixes the following problems with source maps:

    * File names in `sourceMappingURL` that contained a space previously did not encode the space as `%20`, which resulted in JavaScript tools (including esbuild) failing to read that path back in when consuming the generated output file. This should now be fixed.

    * Absolute URLs in `sourceMappingURL` that use the `file://` scheme previously attempted to read from a folder called `file:`. These URLs should now be recognized and parsed correctly.

    * Entries in the `sources` array in the source map are now treated as URLs instead of file paths. The correct behavior for this is much more clear now that source maps has a [formal specification](https://tc39.es/ecma426/). Many thanks to those who worked on the specification.

* Fix incorrect package for `@esbuild/netbsd-arm64` ([#4018](https://github.com/evanw/esbuild/issues/4018))

    Due to a copy+paste typo, the binary published to `@esbuild/netbsd-arm64` was not actually for `arm64`, and didn't run in that environment. This release should fix running esbuild in that environment (NetBSD on 64-bit ARM). Sorry about the mistake.

* Fix a minification bug with bitwise operators and bigints ([#4065](https://github.com/evanw/esbuild/issues/4065))

    This change removes an incorrect assumption in esbuild that all bitwise operators result in a numeric integer. That assumption was correct up until the introduction of bigints in ES2020, but is no longer correct because almost all bitwise operators now operate on both numbers and bigints. Here's an example of the incorrect minification:

    ```js
    // Original code
    if ((a & b) !== 0) found = true

    // Old output (with --minify)
    a&b&&(found=!0);

    // New output (with --minify)
    (a&b)!==0&&(found=!0);
    ```

* Fix esbuild incorrectly rejecting valid TypeScript edge case ([#4027](https://github.com/evanw/esbuild/issues/4027))

    The following TypeScript code is valid:

    ```ts
    export function open(async?: boolean): void {
      console.log(async as boolean)
    }
    ```

    Before this version, esbuild would fail to parse this with a syntax error as it expected the token sequence `async as ...` to be the start of an async arrow function expression `async as => ...`. This edge case should be parsed correctly by esbuild starting with this release.

* Transform BigInt values into constructor calls when unsupported ([#4049](https://github.com/evanw/esbuild/issues/4049))

    Previously esbuild would refuse to compile the BigInt literals (such as `123n`) if they are unsupported in the configured target environment (such as with `--target=es6`). The rationale was that they cannot be polyfilled effectively because they change the behavior of JavaScript's arithmetic operators and JavaScript doesn't have operator overloading.

    However, this prevents using esbuild with certain libraries that would otherwise work if BigInt literals were ignored, such as with old versions of the [`buffer` library](https://github.com/feross/buffer) before the library fixed support for running in environments without BigInt support. So with this release, esbuild will now turn BigInt literals into BigInt constructor calls (so `123n` becomes `BigInt(123)`) and generate a warning in this case. You can turn off the warning with `--log-override:bigint=silent` or restore the warning to an error with `--log-override:bigint=error` if needed.

* Change how `console` API dropping works ([#4020](https://github.com/evanw/esbuild/issues/4020))

    Previously the `--drop:console` feature replaced all method calls off of the `console` global with `undefined` regardless of how long the property access chain was (so it applied to `console.log()` and `console.log.call(console)` and `console.log.not.a.method()`). However, it was pointed out that this breaks uses of `console.log.bind(console)`. That's also incompatible with Terser's implementation of the feature, which is where this feature originally came from (it does support `bind`). So with this release, using this feature with esbuild will now only replace one level of method call (unless extended by `call` or `apply`) and will replace the method being called with an empty function in complex cases:

    ```js
    // Original code
    const x = console.log('x')
    const y = console.log.call(console, 'y')
    const z = console.log.bind(console)('z')

    // Old output (with --drop-console)
    const x = void 0;
    const y = void 0;
    const z = (void 0)("z");

    // New output (with --drop-console)
    const x = void 0;
    const y = void 0;
    const z = (() => {
    }).bind(console)("z");
    ```

    This should more closely match Terser's existing behavior.

* Allow BigInt literals as `define` values

    With this release, you can now use BigInt literals as define values, such as with `--define:FOO=123n`. Previously trying to do this resulted in a syntax error.

* Fix a bug with resolve extensions in `node_modules` ([#4053](https://github.com/evanw/esbuild/issues/4053))

    The `--resolve-extensions=` option lets you specify the order in which to try resolving implicit file extensions. For complicated reasons, esbuild reorders TypeScript file extensions after JavaScript ones inside of `node_modules` so that JavaScript source code is always preferred to TypeScript source code inside of dependencies. However, this reordering had a bug that could accidentally change the relative order of TypeScript file extensions if one of them was a prefix of the other. That bug has been fixed in this release. You can see the issue for details.

* Better minification of statically-determined `switch` cases ([#4028](https://github.com/evanw/esbuild/issues/4028))

    With this release, esbuild will now try to trim unused code within `switch` statements when the test expression and `case` expressions are primitive literals. This can arise when the test expression is an identifier that is substituted for a primitive literal at compile time. For example:

    ```js
    // Original code
    switch (MODE) {
      case 'dev':
        installDevToolsConsole()
        break
      case 'prod':
        return
      default:
        throw new Error
    }

    // Old output (with --minify '--define:MODE="prod"')
    switch("prod"){case"dev":installDevToolsConsole();break;case"prod":return;default:throw new Error}

    // New output (with --minify '--define:MODE="prod"')
    return;
    ```

* Emit `/* @__KEY__ */` for string literals derived from property names ([#4034](https://github.com/evanw/esbuild/issues/4034))

    Property name mangling is an advanced feature that shortens certain property names for better minification (I say "advanced feature" because it's very easy to break your code with it). Sometimes you need to store a property name in a string, such as `obj.get('foo')` instead of `obj.foo`. JavaScript minifiers such as esbuild and [Terser](https://terser.org/) have a convention where a `/* @__KEY__ */` comment before the string makes it behave like a property name. So `obj.get(/* @__KEY__ */ 'foo')` allows the contents of the string `'foo'` to be shortened.

    However, esbuild sometimes itself generates string literals containing property names when transforming code, such as when lowering class fields to ES6 or when transforming TypeScript decorators. Previously esbuild didn't generate its own `/* @__KEY__ */` comments in this case, which means that minifying your code by running esbuild again on its own output wouldn't work correctly (this does not affect people that both minify and transform their code in a single step).

    With this release, esbuild will now generate `/* @__KEY__ */` comments for property names in generated string literals. To avoid lots of unnecessary output for people that don't use this advanced feature, the generated comments will only be present when the feature is active. If you want to generate the comments but not actually mangle any property names, you can use a flag that has no effect such as `--reserve-props=.`, which tells esbuild to not mangle any property names (but still activates this feature).

* The `text` loader now strips the UTF-8 BOM if present ([#3935](https://github.com/evanw/esbuild/issues/3935))

    Some software (such as Notepad on Windows) can create text files that start with the three bytes `0xEF 0xBB 0xBF`, which is referred to as the "byte order mark". This prefix is intended to be removed before using the text. Previously esbuild's `text` loader included this byte sequence in the string, which turns into a prefix of `\uFEFF` in a JavaScript string when decoded from UTF-8. With this release, esbuild's `text` loader will now remove these bytes when they occur at the start of the file.

* Omit legal comment output files when empty ([#3670](https://github.com/evanw/esbuild/issues/3670))

    Previously configuring esbuild with `--legal-comment=external` or `--legal-comment=linked` would always generate a `.LEGAL.txt` output file even if it was empty. Starting with this release, esbuild will now only do this if the file will be non-empty. This should result in a more organized output directory in some cases.

* Update Go from 1.23.1 to 1.23.5 ([#4056](https://github.com/evanw/esbuild/issues/4056), [#4057](https://github.com/evanw/esbuild/pull/4057))

    This should have no effect on existing code as this version change does not change Go's operating system support. It may remove certain reports from vulnerability scanners that detect which version of the Go compiler esbuild uses.

    This PR was contributed by [@MikeWillCook](https://github.com/MikeWillCook).

* Allow passing a port of 0 to the development server ([#3692](https://github.com/evanw/esbuild/issues/3692))

    Unix sockets interpret a port of 0 to mean "pick a random unused port in the [ephemeral port](https://en.wikipedia.org/wiki/Ephemeral_port) range". However, esbuild's default behavior when the port is not specified is to pick the first unused port starting from 8000 and upward. This is more convenient because port 8000 is typically free, so you can for example restart the development server and reload your app in the browser without needing to change the port in the URL. Since esbuild is written in Go (which does not have optional fields like JavaScript), not specifying the port in Go means it defaults to 0, so previously passing a port of 0 to esbuild caused port 8000 to be picked.

    Starting with this release, passing a port of 0 to esbuild when using the CLI or the JS API will now pass port 0 to the OS, which will pick a random ephemeral port. To make this possible, the `Port` option in the Go API has been changed from `uint16` to `int` (to allow for additional sentinel values) and passing a port of -1 in Go now picks a random port. Both the CLI and JS APIs now remap an explicitly-provided port of 0 into -1 for the internal Go API.

    Another option would have been to change `Port` in Go from `uint16` to `*uint16` (Go's closest equivalent of `number | undefined`). However, that would make the common case of providing an explicit port in Go very awkward as Go doesn't support taking the address of integer constants. This tradeoff isn't worth it as picking a random ephemeral port is a rare use case. So the CLI and JS APIs should now match standard Unix behavior when the port is 0, but you need to use -1 instead with Go API.

* Minification now avoids inlining constants with direct `eval` ([#4055](https://github.com/evanw/esbuild/issues/4055))

    Direct `eval` can be used to introduce a new variable like this:

    ```js
    const variable = false
    ;(function () {
      eval("var variable = true")
      console.log(variable)
    })()
    ```

    Previously esbuild inlined `variable` here (which became `false`), which changed the behavior of the code. This inlining is now avoided, but please keep in mind that direct `eval` breaks many assumptions that JavaScript tools hold about normal code (especially when bundling) and I do not recommend using it. There are usually better alternatives that have a more localized impact on your code. You can read more about this here: https://esbuild.github.io/link/direct-eval/

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
