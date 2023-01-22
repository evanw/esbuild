# Changelog

## Unreleased

* Implement HTTP `HEAD` requests in serve mode ([#2851](https://github.com/evanw/esbuild/issues/2851))

    Previously esbuild's serve mode only responded to HTTP `GET` requests. With this release, esbuild's serve mode will also respond to HTTP `HEAD` requests, which are just like HTTP `GET` requests except that the body of the response is omitted.

* Permit top-level await in dead code branches ([#2853](https://github.com/evanw/esbuild/issues/2853))

    Adding top-level await to a file has a few consequences with esbuild:

    1. It causes esbuild to assume that the input module format is ESM, since top-level await is only syntactically valid in ESM. That prevents you from using `module` and `exports` for exports and also enables strict mode, which disables certain syntax and changes how function hoisting works (among other things).
    2. This will cause esbuild to fail the build if either top-level await isn't supported by your language target (e.g. it's not supported in ES2021) or if top-level await isn't supported by the chosen output format (e.g. it's not supported with CommonJS).
    3. Doing this will prevent you from using `require()` on this file or on any file that imports this file (even indirectly), since the `require()` function doesn't return a promise and so can't represent top-level await.

    This release relaxes these rules slightly: rules 2 and 3 will now no longer apply when esbuild has identified the code branch as dead code, such as when it's behind an `if (false)` check. This should make it possible to use esbuild to convert code into different output formats that only uses top-level await conditionally. This release does not relax rule 1. Top-level await will still cause esbuild to unconditionally consider the input module format to be ESM, even when the top-level `await` is in a dead code branch. This is necessary because whether the input format is ESM or not affects the whole file, not just the dead code branch.

* Fix entry points where the entire file name is the extension ([#2861](https://github.com/evanw/esbuild/issues/2861))

    Previously if you passed esbuild an entry point where the file extension is the entire file name, esbuild would use the parent directory name to derive the name of the output file. For example, if you passed esbuild a file `./src/.ts` then the output name would be `src.js`. This bug happened because esbuild first strips the file extension to get `./src/` and then joins the path with the working directory to get the absolute path (e.g. `join("/working/dir", "./src/")` gives `/working/dir/src`). However, the join operation also canonicalizes the path which strips the trailing `/`. Later esbuild uses the "base name" operation to extract the name of the output file. Since there is no trailing `/`, esbuild returns `"src"` as the base name instead of `""`, which causes esbuild to incorrectly include the directory name in the output file name. This release fixes this bug by deferring the stripping of the file extension until after all path manipulations have been completed. So now the file `./src/.ts` will generate an output file named `.js`.

## 0.17.3

* Fix incorrect CSS minification for certain rules ([#2838](https://github.com/evanw/esbuild/issues/2838))

    Certain rules such as `@media` could previously be minified incorrectly. Due to a typo in the duplicate rule checker, two known `@`-rules that share the same hash code were incorrectly considered to be equal. This problem was made worse by the rule hashing code considering two unknown declarations (such as CSS variables) to have the same hash code, which also isn't optimal from a performance perspective. Both of these issues have been fixed:

    ```css
    /* Original input */
    @media (prefers-color-scheme: dark) { body { --VAR-1: #000; } }
    @media (prefers-color-scheme: dark) { body { --VAR-2: #000; } }

    /* Old output (with --minify) */
    @media (prefers-color-scheme: dark){body{--VAR-2: #000}}

    /* New output (with --minify) */
    @media (prefers-color-scheme: dark){body{--VAR-1: #000}}@media (prefers-color-scheme: dark){body{--VAR-2: #000}}
    ```

## 0.17.2

* Add `onDispose` to the plugin API ([#2140](https://github.com/evanw/esbuild/issues/2140), [#2205](https://github.com/evanw/esbuild/issues/2205))

    If your plugin wants to perform some cleanup after it's no longer going to be used, you can now use the `onDispose` API to register a callback for cleanup-related tasks. For example, if a plugin starts a long-running child process then it may want to terminate that process when the plugin is discarded. Previously there was no way to do this. Here's an example:

    ```js
    let examplePlugin = {
      name: 'example',
      setup(build) {
        build.onDispose(() => {
          console.log('This plugin is no longer used')
        })
      },
    }
    ```

    These `onDispose` callbacks will be called after every `build()` call regardless of whether the build failed or not as well as after the first `dispose()` call on a given build context.

## 0.17.1

* Make it possible to cancel a build ([#2725](https://github.com/evanw/esbuild/issues/2725))

    The context object introduced in version 0.17.0 has a new `cancel()` method. You can use it to cancel a long-running build so that you can start a new one without needing to wait for the previous one to finish. When this happens, the previous build should always have at least one error and have no output files (i.e. it will be a failed build).

    Using it might look something like this:

    * JS:

        ```js
        let ctx = await esbuild.context({
          // ...
        })

        let rebuildWithTimeLimit = timeLimit => {
          let timeout = setTimeout(() => ctx.cancel(), timeLimit)
          return ctx.rebuild().finally(() => clearTimeout(timeout))
        }

        let build = await rebuildWithTimeLimit(500)
        ```

    * Go:

        ```go
        ctx, err := api.Context(api.BuildOptions{
          // ...
        })
        if err != nil {
          return
        }

        rebuildWithTimeLimit := func(timeLimit time.Duration) api.BuildResult {
          t := time.NewTimer(timeLimit)
          go func() {
            <-t.C
            ctx.Cancel()
          }()
          result := ctx.Rebuild()
          t.Stop()
          return result
        }

        build := rebuildWithTimeLimit(500 * time.Millisecond)
        ```

    This API is a quick implementation and isn't maximally efficient, so the build may continue to do some work for a little bit before stopping. For example, I have added stop points between each top-level phase of the bundler and in the main module graph traversal loop, but I haven't added fine-grained stop points within the internals of the linker. How quickly esbuild stops can be improved in future releases. This means you'll want to wait for `cancel()` and/or the previous `rebuild()` to finish (i.e. await the returned promise in JavaScript) before starting a new build, otherwise `rebuild()` will give you the just-canceled build that still hasn't ended yet. Note that `onEnd` callbacks will still be run regardless of whether or not the build was canceled.

* Fix server-sent events without `servedir` ([#2827](https://github.com/evanw/esbuild/issues/2827))

    The server-sent events for live reload were incorrectly using `servedir` to calculate the path to modified output files. This means events couldn't be sent when `servedir` wasn't specified. This release uses the internal output directory (which is always present) instead of `servedir` (which might be omitted), so live reload should now work when `servedir` is not specified.

* Custom entry point output paths now work with the `copy` loader ([#2828](https://github.com/evanw/esbuild/issues/2828))

    Entry points can optionally provide custom output paths to change the path of the generated output file. For example, `esbuild foo=abc.js bar=xyz.js --outdir=out` generates the files `out/foo.js` and `out/bar.js`. However, this previously didn't work when using the `copy` loader due to an oversight. This bug has been fixed. For example, you can now do `esbuild foo=abc.html bar=xyz.html --outdir=out --loader:.html=copy` to generate the files `out/foo.html` and `out/bar.html`.

* The JS API can now take an array of objects ([#2828](https://github.com/evanw/esbuild/issues/2828))

    Previously it was not possible to specify two entry points with the same custom output path using the JS API, although it was possible to do this with the Go API and the CLI. This will not cause a collision if both entry points use different extensions (e.g. if one uses `.js` and the other uses `.css`). You can now pass the JS API an array of objects to work around this API limitation:

    ```js
    // The previous API didn't let you specify duplicate output paths
    let result = await esbuild.build({
      entryPoints: {
        // This object literal contains a duplicate key, so one is ignored
        'dist': 'foo.js',
        'dist': 'bar.css',
      },
    })

    // You can now specify duplicate output paths as an array of objects
    let result = await esbuild.build({
      entryPoints: [
        { in: 'foo.js', out: 'dist' },
        { in: 'bar.css', out: 'dist' },
      ],
    })
    ```

## 0.17.0

**This release deliberately contains backwards-incompatible changes.** To avoid automatically picking up releases like this, you should either be pinning the exact version of `esbuild` in your `package.json` file (recommended) or be using a version range syntax that only accepts patch upgrades such as `^0.16.0` or `~0.16.0`. See npm's documentation about [semver](https://docs.npmjs.com/cli/v6/using-npm/semver/) for more information.

At a high level, the breaking changes in this release fix some long-standing issues with the design of esbuild's incremental, watch, and serve APIs. This release also introduces some exciting new features such as live reloading. In detail:

* Move everything related to incremental builds to a new `context` API ([#1037](https://github.com/evanw/esbuild/issues/1037), [#1606](https://github.com/evanw/esbuild/issues/1606), [#2280](https://github.com/evanw/esbuild/issues/2280), [#2418](https://github.com/evanw/esbuild/issues/2418))

    This change removes the `incremental` and `watch` options as well as the `serve()` method, and introduces a new `context()` method. The context method takes the same arguments as the `build()` method but only validates its arguments and does not do an initial build. Instead, builds can be triggered using the `rebuild()`, `watch()`, and `serve()` methods on the returned context object. The new context API looks like this:

    ```js
    // Create a context for incremental builds
    const context = await esbuild.context({
      entryPoints: ['app.ts'],
      bundle: true,
    })

    // Manually do an incremental build
    const result = await context.rebuild()

    // Enable watch mode
    await context.watch()

    // Enable serve mode
    await context.serve()

    // Dispose of the context
    context.dispose()
    ```

    The switch to the context API solves a major issue with the previous API which is that if the initial build fails, a promise is thrown in JavaScript which prevents you from accessing the returned result object. That prevented you from setting up long-running operations such as watch mode when the initial build contained errors. It also makes tearing down incremental builds simpler as there is now a single way to do it instead of three separate ways.

    In addition, this release also makes some subtle changes to how incremental builds work. Previously every call to `rebuild()` started a new build. If you weren't careful, then builds could actually overlap. This doesn't cause any problems with esbuild itself, but could potentially cause problems with plugins (esbuild doesn't even give you a way to identify which overlapping build a given plugin callback is running on). Overlapping builds also arguably aren't useful, or at least aren't useful enough to justify the confusion and complexity that they bring. With this release, there is now only ever a single active build per context. Calling `rebuild()` before the previous rebuild has finished now "merges" with the existing rebuild instead of starting a new build.

* Allow using `watch` and `serve` together ([#805](https://github.com/evanw/esbuild/issues/805), [#1650](https://github.com/evanw/esbuild/issues/1650), [#2576](https://github.com/evanw/esbuild/issues/2576))

    Previously it was not possible to use watch mode and serve mode together. The rationale was that watch mode is one way of automatically rebuilding your project and serve mode is another (since serve mode automatically rebuilds on every request). However, people want to combine these two features to make "live reloading" where the browser automatically reloads the page when files are changed on the file system.

    This release now allows you to use these two features together. You can only call the `watch()` and `serve()` APIs once each per context, but if you call them together on the same context then esbuild will automatically rebuild both when files on the file system are changed *and* when the server serves a request.

* Support "live reloading" through server-sent events ([#802](https://github.com/evanw/esbuild/issues/802))

    [Server-sent events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/Using_server-sent_events) are a simple way to pass one-directional messages asynchronously from the server to the client. Serve mode now provides a `/esbuild` endpoint with an `change` event that triggers every time esbuild's output changes. So you can now implement simple "live reloading" (i.e. reloading the page when a file is edited and saved) like this:

    ```js
    new EventSource('/esbuild').addEventListener('change', () => location.reload())
    ```

    The event payload is a JSON object with the following shape:

    ```ts
    interface ChangeEvent {
      added: string[]
      removed: string[]
      updated: string[]
    }
    ```

    This JSON should also enable more complex live reloading scenarios. For example, the following code hot-swaps changed CSS `<link>` tags in place without reloading the page (but still reloads when there are other types of changes):

    ```js
    new EventSource('/esbuild').addEventListener('change', e => {
      const { added, removed, updated } = JSON.parse(e.data)
      if (!added.length && !removed.length && updated.length === 1) {
        for (const link of document.getElementsByTagName("link")) {
          const url = new URL(link.href)
          if (url.host === location.host && url.pathname === updated[0]) {
            const next = link.cloneNode()
            next.href = updated[0] + '?' + Math.random().toString(36).slice(2)
            next.onload = () => link.remove()
            link.parentNode.insertBefore(next, link.nextSibling)
            return
          }
        }
      }
      location.reload()
    })
    ```

    Implementing live reloading like this has a few known caveats:

    * These events only trigger when esbuild's output changes. They do not trigger when files unrelated to the build being watched are changed. If your HTML file references other files that esbuild doesn't know about and those files are changed, you can either manually reload the page or you can implement your own live reloading infrastructure instead of using esbuild's built-in behavior.

    * The `EventSource` API is supposed to automatically reconnect for you. However, there's a bug in Firefox that breaks this if the server is ever temporarily unreachable: https://bugzilla.mozilla.org/show_bug.cgi?id=1809332. Workarounds are to use any other browser, to manually reload the page if this happens, or to write more complicated code that manually closes and re-creates the `EventSource` object if there is a connection error. I'm hopeful that this bug will be fixed.

    * Browser vendors have decided to not implement HTTP/2 without TLS. This means that each `/esbuild` event source will take up one of your precious 6 simultaneous per-domain HTTP/1.1 connections. So if you open more than six HTTP tabs that use this live-reloading technique, you will be unable to use live reloading in some of those tabs (and other things will likely also break). The workaround is to enable HTTPS, which is now possible to do in esbuild itself (see below).

* Add built-in support for HTTPS ([#2169](https://github.com/evanw/esbuild/issues/2169))

    You can now tell esbuild's built-in development server to use HTTPS instead of HTTP. This is sometimes necessary because browser vendors have started making modern web features unavailable to HTTP websites. Previously you had to put a proxy in front of esbuild to enable HTTPS since esbuild's development server only supported HTTP. But with this release, you can now enable HTTPS with esbuild without an additional proxy.

    To enable HTTPS with esbuild:

    1. Generate a self-signed certificate. There are many ways to do this. Here's one way, assuming you have `openssl` installed:

        ```
        openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 9999 -nodes -subj /CN=127.0.0.1
        ```

    2. Add `--keyfile=key.pem` and `--certfile=cert.pem` to your esbuild development server command
    3. Click past the scary warning in your browser when you load your page

    If you have more complex needs than this, you can still put a proxy in front of esbuild and use that for HTTPS instead. Note that if you see the message "Client sent an HTTP request to an HTTPS server" when you load your page, then you are using the incorrect protocol. Replace `http://` with `https://` in your browser's URL bar.

    Keep in mind that esbuild's HTTPS support has nothing to do with security. The only reason esbuild now supports HTTPS is because browsers have made it impossible to do local development with certain modern web features without jumping through these extra hoops. *Please do not use esbuild's development server for anything that needs to be secure.* It's only intended for local development and no considerations have been made for production environments whatsoever.

* Better support copying `index.html` into the output directory ([#621](https://github.com/evanw/esbuild/issues/621), [#1771](https://github.com/evanw/esbuild/issues/1771))

    Right now esbuild only supports JavaScript and CSS as first-class content types. Previously this meant that if you were building a website with a HTML file, a JavaScript file, and a CSS file, you could use esbuild to build the JavaScript file and the CSS file into the output directory but not to copy the HTML file into the output directory. You needed a separate `cp` command for that.

    Or so I thought. It turns out that the `copy` loader added in version 0.14.44 of esbuild is sufficient to have esbuild copy the HTML file into the output directory as well. You can add something like `index.html --loader:.html=copy` and esbuild will copy `index.html` into the output directory for you. The benefits of this are a) you don't need a separate `cp` command and b) the `index.html` file will automatically be re-copied when esbuild is in watch mode and the contents of `index.html` are edited. This also goes for other non-HTML file types that you might want to copy.

    This pretty much already worked. The one thing that didn't work was that esbuild's built-in development server previously only supported implicitly loading `index.html` (e.g. loading `/about/index.html` when you visit `/about/`) when `index.html` existed on the file system. Previously esbuild didn't support implicitly loading `index.html` if it was a build result. That bug has been fixed with this release so it should now be practical to use the `copy` loader to do this.

* Fix `onEnd` not being called in serve mode ([#1384](https://github.com/evanw/esbuild/issues/1384))

    Previous releases had a bug where plugin `onEnd` callbacks weren't called when using the top-level `serve()` API. This API no longer exists and the internals have been reimplemented such that `onEnd` callbacks should now always be called at the end of every build.

* Incremental builds now write out build results differently ([#2104](https://github.com/evanw/esbuild/issues/2104))

    Previously build results were always written out after every build. However, this could cause the output directory to fill up with files from old builds if code splitting was enabled, since the file names for code splitting chunks contain content hashes and old files were not deleted.

    With this release, incremental builds in esbuild will now delete old output files from previous builds that are no longer relevant. Subsequent incremental builds will also no longer overwrite output files whose contents haven't changed since the previous incremental build.

* The `onRebuild` watch mode callback was removed ([#980](https://github.com/evanw/esbuild/issues/980), [#2499](https://github.com/evanw/esbuild/issues/2499))

    Previously watch mode accepted an `onRebuild` callback which was called whenever watch mode rebuilt something. This was not great in practice because if you are running code after a build, you likely want that code to run after every build, not just after the second and subsequent builds. This release removes option to provide an `onRebuild` callback. You can create a plugin with an `onEnd` callback instead. The `onEnd` plugin API already exists, and is a way to run some code after every build.

* You can now return errors from `onEnd` ([#2625](https://github.com/evanw/esbuild/issues/2625))

    It's now possible to add additional build errors and/or warnings to the current build from within your `onEnd` callback by returning them in an array. This is identical to how the `onStart` callback already works. The evaluation of `onEnd` callbacks have been moved around a bit internally to make this possible.

    Note that the build will only fail (i.e. reject the promise) if the additional errors are returned from `onEnd`. Adding additional errors to the result object that's passed to `onEnd` won't affect esbuild's behavior at all.

* Print URLs and ports from the Go and JS APIs ([#2393](https://github.com/evanw/esbuild/issues/2393))

    Previously esbuild's CLI printed out something like this when serve mode is active:

    ```
     > Local:   http://127.0.0.1:8000/
     > Network: http://192.168.0.1:8000/
    ```

    The CLI still does this, but now the JS and Go serve mode APIs will do this too. This only happens when the log level is set to `verbose`, `debug`, or `info` but not when it's set to `warning`, `error`, or `silent`.

### Upgrade guide for existing code:

* Rebuild (a.k.a. incremental build):

    Before:

    ```js
    const result = await esbuild.build({ ...buildOptions, incremental: true });
    builds.push(result);
    for (let i = 0; i < 4; i++) builds.push(await result.rebuild());
    await result.rebuild.dispose(); // To free resources
    ```

    After:

    ```js
    const ctx = await esbuild.context(buildOptions);
    for (let i = 0; i < 5; i++) builds.push(await ctx.rebuild());
    await ctx.dispose(); // To free resources
    ```

    Previously the first build was done differently than subsequent builds. Now both the first build and subsequent builds are done using the same API.

* Serve:

    Before:

    ```js
    const serveResult = await esbuild.serve(serveOptions, buildOptions);
    ...
    serveResult.stop(); await serveResult.wait; // To free resources
    ```

    After:

    ```js
    const ctx = await esbuild.context(buildOptions);
    const serveResult = await ctx.serve(serveOptions);
    ...
    await ctx.dispose(); // To free resources
    ```

* Watch:

    Before:

    ```js
    const result = await esbuild.build({ ...buildOptions, watch: true });
    ...
    result.stop(); // To free resources
    ```

    After:

    ```js
    const ctx = await esbuild.context(buildOptions);
    await ctx.watch();
    ...
    await ctx.dispose(); // To free resources
    ```

* Watch with `onRebuild`:

    Before:

    ```js
    const onRebuild = (error, result) => {
      if (error) console.log('subsequent build:', error);
      else console.log('subsequent build:', result);
    };
    try {
      const result = await esbuild.build({ ...buildOptions, watch: { onRebuild } });
      console.log('first build:', result);
      ...
      result.stop(); // To free resources
    } catch (error) {
      console.log('first build:', error);
    }
    ```

    After:

    ```js
    const plugins = [{
      name: 'my-plugin',
      setup(build) {
        let count = 0;
        build.onEnd(result => {
          if (count++ === 0) console.log('first build:', result);
          else console.log('subsequent build:', result);
        });
      },
    }];
    const ctx = await esbuild.context({ ...buildOptions, plugins });
    await ctx.watch();
    ...
    await ctx.dispose(); // To free resources
    ```

    The `onRebuild` function has now been removed. The replacement is to make a plugin with an `onEnd` callback.

    Previously `onRebuild` did not fire for the first build (only for subsequent builds). This was usually problematic, so using `onEnd` instead of `onRebuild` is likely less error-prone. But if you need to emulate the old behavior of `onRebuild` that ignores the first build, then you'll need to manually count and ignore the first build in your plugin (as demonstrated above).

Notice how all of these API calls are now done off the new context object. You should now be able to use all three kinds of incremental builds (`rebuild`, `serve`, and `watch`) together on the same context object. Also notice how calling `dispose` on the context is now the common way to discard the context and free resources in all of these situations.

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
