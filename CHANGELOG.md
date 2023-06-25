# Changelog

## Unreleased

* Implement transforming `async` generator functions ([#2780](https://github.com/evanw/esbuild/issues/2780))

    With this release, esbuild will now transform `async` generator functions into normal generator functions when the configured target environment doesn't support them. These functions behave similar to normal generator functions except that they use the `Symbol.asyncIterator` interface instead of the `Symbol.iterator` interface and the iteration methods return promises. Here's an example (helper functions are omitted):

    ```js
    // Original code
    async function* foo() {
      yield Promise.resolve(1)
      await new Promise(r => setTimeout(r, 100))
      yield *[Promise.resolve(2)]
    }
    async function bar() {
      for await (const x of foo()) {
        console.log(x)
      }
    }
    bar()

    // New output (with --target=es6)
    function foo() {
      return __asyncGenerator(this, null, function* () {
        yield Promise.resolve(1);
        yield new __await(new Promise((r) => setTimeout(r, 100)));
        yield* __yieldStar([Promise.resolve(2)]);
      });
    }
    function bar() {
      return __async(this, null, function* () {
        try {
          for (var iter = __forAwait(foo()), more, temp, error; more = !(temp = yield iter.next()).done; more = false) {
            const x = temp.value;
            console.log(x);
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
    bar();
    ```

    This is an older feature that was added to JavaScript in ES2018 but I didn't implement the transformation then because it's a rarely-used feature. Note that esbuild already added support for transforming `for await` loops (the other part of the [asynchronous iteration proposal](https://github.com/tc39/proposal-async-iteration)) a year ago, so support for asynchronous iteration should now be complete.

    I have never used this feature myself and code that uses this feature is hard to come by, so this transformation has not yet been tested on real-world code. If you do write code that uses this feature, please let me know if esbuild's `async` generator transformation doesn't work with your code.

## 0.18.7

* Add support for `using` declarations in TypeScript 5.2+ ([#3191](https://github.com/evanw/esbuild/issues/3191))

    TypeScript 5.2 (due to be released in August of 2023) will introduce `using` declarations, which will allow you to automatically dispose of the declared resources when leaving the current scope. You can read the [TypeScript PR for this feature](https://github.com/microsoft/TypeScript/pull/54505) for more information. This release of esbuild adds support for transforming this syntax to target environments without support for `using` declarations (which is currently all targets other than `esnext`). Here's an example (helper functions are omitted):

    ```js
    // Original code
    class Foo {
      [Symbol.dispose]() {
        console.log('cleanup')
      }
    }
    using foo = new Foo;
    foo.bar();

    // New output (with --target=es6)
    var _stack = [];
    try {
      var Foo = class {
        [Symbol.dispose]() {
          console.log("cleanup");
        }
      };
      var foo = __using(_stack, new Foo());
      foo.bar();
    } catch (_) {
      var _error = _, _hasError = true;
    } finally {
      __callDispose(_stack, _error, _hasError);
    }
    ```

    The injected helper functions ensure that the method named `Symbol.dispose` is called on `new Foo` when control exits the scope. Note that as with all new JavaScript APIs, you'll need to polyfill `Symbol.dispose` if it's not present before you use it. This is not something that esbuild does for you because esbuild only handles syntax, not APIs. Polyfilling it can be done with something like this:

    ```js
    Symbol.dispose ||= Symbol('Symbol.dispose')
    ```

    This feature also introduces `await using` declarations which are like `using` declarations but they call `await` on the disposal method (not on the initializer). Here's an example (helper functions are omitted):

    ```js
    // Original code
    class Foo {
      async [Symbol.asyncDispose]() {
        await new Promise(done => {
          setTimeout(done, 1000)
        })
        console.log('cleanup')
      }
    }
    await using foo = new Foo;
    foo.bar();

    // New output (with --target=es2022)
    var _stack = [];
    try {
      var Foo = class {
        async [Symbol.asyncDispose]() {
          await new Promise((done) => {
            setTimeout(done, 1e3);
          });
          console.log("cleanup");
        }
      };
      var foo = __using(_stack, new Foo(), true);
      foo.bar();
    } catch (_) {
      var _error = _, _hasError = true;
    } finally {
      var _promise = __callDispose(_stack, _error, _hasError);
      _promise && await _promise;
    }
    ```

    The injected helper functions ensure that the method named `Symbol.asyncDispose` is called on `new Foo` when control exits the scope, and that the returned promise is awaited. Similarly to `Symbol.dispose`, you'll also need to polyfill `Symbol.asyncDispose` before you use it.

* Add a `--line-limit=` flag to limit line length ([#3170](https://github.com/evanw/esbuild/issues/3170))

    Long lines are common in minified code. However, many tools and text editors can't handle long lines. This release introduces the `--line-limit=` flag to tell esbuild to wrap lines longer than the provided number of bytes. For example, `--line-limit=80` tells esbuild to insert a newline soon after a given line reaches 80 bytes in length. This setting applies to both JavaScript and CSS, and works even when minification is disabled. Note that turning this setting on will make your files bigger, as the extra newlines take up additional space in the file (even after gzip compression).

## 0.18.6

* Fix tree-shaking of classes with decorators ([#3164](https://github.com/evanw/esbuild/issues/3164))

    This release fixes a bug where esbuild incorrectly allowed tree-shaking on classes with decorators. Each decorator is a function call, so classes with decorators must never be tree-shaken. This bug was a regression that was unintentionally introduced in version 0.18.2 by the change that enabled tree-shaking of lowered private fields. Previously decorators were always lowered, and esbuild always considered the automatically-generated decorator code to be a side effect. But this is no longer the case now that esbuild analyzes side effects using the AST before lowering takes place. This bug was fixed by considering any decorator a side effect.

* Fix a minification bug involving function expressions ([#3125](https://github.com/evanw/esbuild/issues/3125))

    When minification is enabled, esbuild does limited inlining of `const` symbols at the top of a scope. This release fixes a bug where inlineable symbols were incorrectly removed assuming that they were inlined. They may not be inlined in cases where they were referenced by earlier constants in the body of a function expression. The declarations involved in these edge cases are now kept instead of being removed:

    ```js
    // Original code
    {
      const fn = () => foo
      const foo = 123
      console.log(fn)
    }

    // Old output (with --minify-syntax)
    console.log((() => foo)());

    // New output (with --minify-syntax)
    {
      const fn = () => foo, foo = 123;
      console.log(fn);
    }
    ```

## 0.18.5

* Implement auto accessors ([#3009](https://github.com/evanw/esbuild/issues/3009))

    This release implements the new auto-accessor syntax from the upcoming [JavaScript decorators proposal](https://github.com/tc39/proposal-decorators). The auto-accessor syntax looks like this:

    ```js
    class Foo {
      accessor foo;
      static accessor bar;
    }
    new Foo().foo = Foo.bar;
    ```

    This syntax is not yet a part of JavaScript but it was [added to TypeScript in version 4.9](https://devblogs.microsoft.com/typescript/announcing-typescript-4-9/#auto-accessors-in-classes). More information about this feature can be found in [microsoft/TypeScript#49705](https://github.com/microsoft/TypeScript/pull/49705). Auto-accessors will be transformed if the target is set to something other than `esnext`:

    ```js
    // Output (with --target=esnext)
    class Foo {
      accessor foo;
      static accessor bar;
    }
    new Foo().foo = Foo.bar;

    // Output (with --target=es2022)
    class Foo {
      #foo;
      get foo() {
        return this.#foo;
      }
      set foo(_) {
        this.#foo = _;
      }
      static #bar;
      static get bar() {
        return this.#bar;
      }
      static set bar(_) {
        this.#bar = _;
      }
    }
    new Foo().foo = Foo.bar;

    // Output (with --target=es2021)
    var _foo, _bar;
    class Foo {
      constructor() {
        __privateAdd(this, _foo, void 0);
      }
      get foo() {
        return __privateGet(this, _foo);
      }
      set foo(_) {
        __privateSet(this, _foo, _);
      }
      static get bar() {
        return __privateGet(this, _bar);
      }
      static set bar(_) {
        __privateSet(this, _bar, _);
      }
    }
    _foo = new WeakMap();
    _bar = new WeakMap();
    __privateAdd(Foo, _bar, void 0);
    new Foo().foo = Foo.bar;
    ```

    You can also now use auto-accessors with esbuild's TypeScript experimental decorator transformation, which should behave the same as decorating the underlying getter/setter pair.

    **Please keep in mind that this syntax is not yet part of JavaScript.** This release enables auto-accessors in `.js` files with the expectation that it will be a part of JavaScript soon. However, esbuild may change or remove this feature in the future if JavaScript ends up changing or removing this feature. Use this feature with caution for now.

* Pass through JavaScript decorators ([#104](https://github.com/evanw/esbuild/issues/104))

    In this release, esbuild now parses decorators from the upcoming [JavaScript decorators proposal](https://github.com/tc39/proposal-decorators) and passes them through to the output unmodified (as long as the language target is set to `esnext`). Transforming JavaScript decorators to environments that don't support them has not been implemented yet. The only decorator transform that esbuild currently implements is still the TypeScript experimental decorator transform, which only works in `.ts` files and which requires `"experimentalDecorators": true` in your `tsconfig.json` file.

* Static fields with assign semantics now use static blocks if possible

    Setting `useDefineForClassFields` to false in TypeScript requires rewriting class fields to assignment statements. Previously this was done by removing the field from the class body and adding an assignment statement after the class declaration. However, this also caused any private fields to also be lowered by necessity (in case a field initializer uses a private symbol, either directly or indirectly). This release changes this transform to use an inline static block if it's supported, which avoids needing to lower private fields in this scenario:

    ```js
    // Original code
    class Test {
      static #foo = 123
      static bar = this.#foo
    }

    // Old output (with useDefineForClassFields=false)
    var _foo;
    const _Test = class _Test {
    };
    _foo = new WeakMap();
    __privateAdd(_Test, _foo, 123);
    _Test.bar = __privateGet(_Test, _foo);
    let Test = _Test;

    // New output (with useDefineForClassFields=false)
    class Test {
      static #foo = 123;
      static {
        this.bar = this.#foo;
      }
    }
    ```

* Fix TypeScript experimental decorators combined with `--mangle-props` ([#3177](https://github.com/evanw/esbuild/issues/3177))

    Previously using TypeScript experimental decorators combined with the `--mangle-props` setting could result in a crash, as the experimental decorator transform was not expecting a mangled property as a class member. This release fixes the crash so you can now combine both of these features together safely.

## 0.18.4

* Bundling no longer unnecessarily transforms class syntax ([#1360](https://github.com/evanw/esbuild/issues/1360), [#1328](https://github.com/evanw/esbuild/issues/1328), [#1524](https://github.com/evanw/esbuild/issues/1524), [#2416](https://github.com/evanw/esbuild/issues/2416))

    When bundling, esbuild automatically converts top-level class statements to class expressions. Previously this conversion had the unfortunate side-effect of also transforming certain other class-related syntax features to avoid correctness issues when the references to the class name within the class body. This conversion has been reworked to avoid doing this:

    ```js
    // Original code
    export class Foo {
      static foo = () => Foo
    }

    // Old output (with --bundle)
    var _Foo = class {
    };
    var Foo = _Foo;
    __publicField(Foo, "foo", () => _Foo);

    // New output (with --bundle)
    var Foo = class _Foo {
      static foo = () => _Foo;
    };
    ```

    This conversion process is very complicated and has many edge cases (including interactions with static fields, static blocks, private class properties, and TypeScript experimental decorators). It should already be pretty robust but a change like this may introduce new unintentional behavior. Please report any issues with this upgrade on the esbuild bug tracker.

    You may be wondering why esbuild needs to do this at all. One reason to do this is that esbuild's bundler sometimes needs to lazily-evaluate a module. For example, a module may end up being both the target of a dynamic `import()` call and a static `import` statement. Lazy module evaluation is done by wrapping the top-level module code in a closure. To avoid a performance hit for static `import` statements, esbuild stores top-level exported symbols outside of the closure and references them directly instead of indirectly.

    Another reason to do this is that multiple JavaScript VMs have had and continue to have performance issues with TDZ (i.e. "temporal dead zone") checks. These checks validate that a let, or const, or class symbol isn't used before it's initialized. Here are two issues with well-known VMs:

    * V8: https://bugs.chromium.org/p/v8/issues/detail?id=13723 (10% slowdown)
    * JavaScriptCore: https://bugs.webkit.org/show_bug.cgi?id=199866 (1,000% slowdown!)

    JavaScriptCore had a severe performance issue as their TDZ implementation had time complexity that was quadratic in the number of variables needing TDZ checks in the same scope (with the top-level scope typically being the worst offender). V8 has ongoing issues with TDZ checks being present throughout the code their JIT generates even when they have already been checked earlier in the same function or when the function in question has already been run (so the checks have already happened).

    Due to esbuild's parallel architecture, esbuild both a) needs to convert class statements into class expressions during parsing and b) doesn't yet know whether this module will need to be lazily-evaluated or not in the parser. So esbuild always does this conversion during bundling in case it's needed for correctness (and also to avoid potentially catastrophic performance issues due to bundling creating a large scope with many TDZ variables).

* Enforce TDZ errors in computed class property keys ([#2045](https://github.com/evanw/esbuild/issues/2045))

    JavaScript allows class property keys to be generated at run-time using code, like this:

    ```js
    class Foo {
      static foo = 'foo'
      static [Foo.foo + '2'] = 2
    }
    ```

    Previously esbuild treated references to the containing class name within computed property keys as a reference to the partially-initialized class object. That meant code that attempted to reference properties of the class object (such as the code above) would get back `undefined` instead of throwing an error.

    This release rewrites references to the containing class name within computed property keys into code that always throws an error at run-time, which is how this JavaScript code is supposed to work. Code that does this will now also generate a warning. You should never write code like this, but it now should be more obvious when incorrect code like this is written.

* Fix an issue with experimental decorators and static fields ([#2629](https://github.com/evanw/esbuild/issues/2629))

    This release also fixes a bug regarding TypeScript experimental decorators and static class fields which reference the enclosing class name in their initializer. This affected top-level classes when bundling was enabled. Previously code that does this could crash because the class name wasn't initialized yet. This case should now be handled correctly:

    ```ts
    // Original code
    class Foo {
      @someDecorator
      static foo = 'foo'
      static bar = Foo.foo.length
    }

    // Old output
    const _Foo = class {
      static foo = "foo";
      static bar = _Foo.foo.length;
    };
    let Foo = _Foo;
    __decorateClass([
      someDecorator
    ], Foo, "foo", 2);

    // New output
    const _Foo = class _Foo {
      static foo = "foo";
      static bar = _Foo.foo.length;
    };
    __decorateClass([
      someDecorator
    ], _Foo, "foo", 2);
    let Foo = _Foo;
    ```

* Fix a minification regression with negative numeric properties ([#3169](https://github.com/evanw/esbuild/issues/3169))

    Version 0.18.0 introduced a regression where computed properties with negative numbers were incorrectly shortened into a non-computed property when minification was enabled. This regression has been fixed:

    ```js
    // Original code
    x = {
      [1]: 1,
      [-1]: -1,
      [NaN]: NaN,
      [Infinity]: Infinity,
      [-Infinity]: -Infinity,
    }

    // Old output (with --minify)
    x={1:1,-1:-1,NaN:NaN,1/0:1/0,-1/0:-1/0};

    // New output (with --minify)
    x={1:1,[-1]:-1,NaN:NaN,[1/0]:1/0,[-1/0]:-1/0};
    ```

## 0.18.3

* Fix a panic due to empty static class blocks ([#3161](https://github.com/evanw/esbuild/issues/3161))

    This release fixes a bug where an internal invariant that was introduced in the previous release was sometimes violated, which then caused a panic. It happened when bundling code containing an empty static class block with both minification and bundling enabled.

## 0.18.2

* Lower static blocks when static fields are lowered ([#2800](https://github.com/evanw/esbuild/issues/2800), [#2950](https://github.com/evanw/esbuild/issues/2950), [#3025](https://github.com/evanw/esbuild/issues/3025))

    This release fixes a bug where esbuild incorrectly did not lower static class blocks when static class fields needed to be lowered. For example, the following code should print `1 2 3` but previously printed `2 1 3` instead due to this bug:

    ```js
    // Original code
    class Foo {
      static x = console.log(1)
      static { console.log(2) }
      static y = console.log(3)
    }

    // Old output (with --supported:class-static-field=false)
    class Foo {
      static {
        console.log(2);
      }
    }
    __publicField(Foo, "x", console.log(1));
    __publicField(Foo, "y", console.log(3));

    // New output (with --supported:class-static-field=false)
    class Foo {
    }
    __publicField(Foo, "x", console.log(1));
    console.log(2);
    __publicField(Foo, "y", console.log(3));
    ```

* Use static blocks to implement `--keep-names` on classes ([#2389](https://github.com/evanw/esbuild/issues/2389))

    This change fixes a bug where the `name` property could previously be incorrect within a class static context when using `--keep-names`. The problem was that the `name` property was being initialized after static blocks were run instead of before. This has been fixed by moving the `name` property initializer into a static block at the top of the class body:

    ```js
    // Original code
    if (typeof Foo === 'undefined') {
      let Foo = class {
        static test = this.name
      }
      console.log(Foo.test)
    }

    // Old output (with --keep-names)
    if (typeof Foo === "undefined") {
      let Foo2 = /* @__PURE__ */ __name(class {
        static test = this.name;
      }, "Foo");
      console.log(Foo2.test);
    }

    // New output (with --keep-names)
    if (typeof Foo === "undefined") {
      let Foo2 = class {
        static {
          __name(this, "Foo");
        }
        static test = this.name;
      };
      console.log(Foo2.test);
    }
    ```

    This change was somewhat involved, especially regarding what esbuild considers to be side-effect free. Some unused classes that weren't removed by tree shaking in previous versions of esbuild may now be tree-shaken. One example is classes with static private fields that are transformed by esbuild into code that doesn't use JavaScript's private field syntax. Previously esbuild's tree shaking analysis ran on the class after syntax lowering, but with this release it will run on the class before syntax lowering, meaning it should no longer be confused by class mutations resulting from automatically-generated syntax lowering code.

## 0.18.1

* Fill in `null` entries in input source maps ([#3144](https://github.com/evanw/esbuild/issues/3144))

    If esbuild bundles input files with source maps and those source maps contain a `sourcesContent` array with `null` entries, esbuild previously copied those `null` entries over to the output source map. With this release, esbuild will now attempt to fill in those `null` entries by looking for a file on the file system with the corresponding name from the `sources` array. This matches esbuild's existing behavior that automatically generates the `sourcesContent` array from the file system if the entire `sourcesContent` array is missing.

* Support `/* @__KEY__ */` comments for mangling property names ([#2574](https://github.com/evanw/esbuild/issues/2574))

    Property mangling is an advanced feature that enables esbuild to minify certain property names, even though it's not possible to automatically determine that it's safe to do so. The safe property names are configured via regular expression such as `--mangle-props=_$` (mangle all properties ending in `_`).

    Sometimes it's desirable to also minify strings containing property names, even though it's not possible to automatically determine which strings are property names. This release makes it possible to do this by annotating those strings with `/* @__KEY__ */`. This is a convention that Terser added earlier this year, and which esbuild is now following too: https://github.com/terser/terser/pull/1365. Using it looks like this:

    ```js
    // Original code
    console.log(
      [obj.mangle_, obj.keep],
      [obj.get('mangle_'), obj.get('keep')],
      [obj.get(/* @__KEY__ */ 'mangle_'), obj.get(/* @__KEY__ */ 'keep')],
    )

    // Old output (with --mangle-props=_$)
    console.log(
      [obj.a, obj.keep],
      [obj.get("mangle_"), obj.get("keep")],
      [obj.get(/* @__KEY__ */ "mangle_"), obj.get(/* @__KEY__ */ "keep")]
    );

    // New output (with --mangle-props=_$)
    console.log(
      [obj.a, obj.keep],
      [obj.get("mangle_"), obj.get("keep")],
      [obj.get(/* @__KEY__ */ "a"), obj.get(/* @__KEY__ */ "keep")]
    );
    ```

* Support `/* @__NO_SIDE_EFFECTS__ */` comments for functions ([#3149](https://github.com/evanw/esbuild/issues/3149))

    Rollup has recently added support for `/* @__NO_SIDE_EFFECTS__ */` annotations before functions to indicate that calls to these functions can be removed if the result is unused (i.e. the calls can be assumed to have no side effects). This release adds basic support for these to esbuild as well, which means esbuild will now parse these comments in input files and preserve them in output files. This should help people that use esbuild in combination with Rollup.

    Note that this doesn't necessarily mean esbuild will treat these calls as having no side effects, as esbuild's parallel architecture currently isn't set up to enable this type of cross-file tree-shaking information (tree-shaking decisions regarding a function call are currently local to the file they appear in). If you want esbuild to consider a function call to have no side effects, make sure you continue to annotate the function call with `/* @__PURE__ */` (which is the previously-established convention for communicating this).

## 0.18.0

**This release deliberately contains backwards-incompatible changes.** To avoid automatically picking up releases like this, you should either be pinning the exact version of `esbuild` in your `package.json` file (recommended) or be using a version range syntax that only accepts patch upgrades such as `^0.17.0` or `~0.17.0`. See npm's documentation about [semver](https://docs.npmjs.com/cli/v6/using-npm/semver/) for more information.

The breaking changes in this release mainly focus on fixing some long-standing issues with esbuild's handling of `tsconfig.json` files. Here are all the changes in this release, in detail:

* Add a way to try esbuild online ([#797](https://github.com/evanw/esbuild/issues/797))

    There is now a way to try esbuild live on esbuild's website without installing it: https://esbuild.github.io/try/. In addition to being able to more easily evaluate esbuild, this should also make it more efficient to generate esbuild bug reports. For example, you can use it to compare the behavior of different versions of esbuild on the same input. The state of the page is stored in the URL for easy sharing. Many thanks to [@hyrious](https://github.com/hyrious) for creating https://hyrious.me/esbuild-repl/, which was the main inspiration for this addition to esbuild's website.

    Two forms of build options are supported: either CLI-style ([example](https://esbuild.github.io/try/#dAAwLjE3LjE5AC0tbG9hZGVyPXRzeCAtLW1pbmlmeSAtLXNvdXJjZW1hcABsZXQgZWw6IEpTWC5FbGVtZW50ID0gPGRpdiAvPg)) or JS-style ([example](https://esbuild.github.io/try/#dAAwLjE3LjE5AHsKICBsb2FkZXI6ICd0c3gnLAogIG1pbmlmeTogdHJ1ZSwKICBzb3VyY2VtYXA6IHRydWUsCn0AbGV0IGVsOiBKU1guRWxlbWVudCA9IDxkaXYgLz4)). Both are converted into a JS object that's passed to esbuild's WebAssembly API. The CLI-style argument parser is a custom one that simulates shell quoting rules, and the JS-style argument parser is also custom and parses a superset of JSON (basically JSON5 + regular expressions). So argument parsing is an approximate simulation of what happens for real but hopefully it should be close enough.

* Changes to esbuild's `tsconfig.json` support ([#3019](https://github.com/evanw/esbuild/issues/3019)):

    This release makes the following changes to esbuild's `tsconfig.json` support:

    * Using experimental decorators now requires `"experimentalDecorators": true` ([#104](https://github.com/evanw/esbuild/issues/104))

        Previously esbuild would always compile decorators in TypeScript code using TypeScript's experimental decorator transform. Now that standard JavaScript decorators are close to being finalized, esbuild will now require you to use `"experimentalDecorators": true` to do this. This new requirement makes it possible for esbuild to introduce a transform for standard JavaScript decorators in TypeScript code in the future. Such a transform has not been implemented yet, however.

    * TypeScript's `target` no longer affects esbuild's `target` ([#2628](https://github.com/evanw/esbuild/issues/2628))

        Some people requested that esbuild support TypeScript's `target` setting, so support for it was added (in [version 0.12.4](https://github.com/evanw/esbuild/releases/v0.12.4)). However, esbuild supports reading from multiple `tsconfig.json` files within a single build, which opens up the possibility that different files in the build have different language targets configured. There isn't really any reason to do this and it can lead to unexpected results. So with this release, the `target` setting in `tsconfig.json` will no longer affect esbuild's own `target` setting. You will have to use esbuild's own target setting instead (which is a single, global value).

    * TypeScript's `jsx` setting no longer causes esbuild to preserve JSX syntax ([#2634](https://github.com/evanw/esbuild/issues/2634))

        TypeScript has a setting called [`jsx`](https://www.typescriptlang.org/tsconfig#jsx) that controls how to transform JSX into JS. The tool-agnostic transform is called `react`, and the React-specific transform is called `react-jsx` (or `react-jsxdev`). There is also a setting called `preserve` which indicates JSX should be passed through untransformed. Previously people would run esbuild with `"jsx": "preserve"` in their `tsconfig.json` files and then be surprised when esbuild preserved their JSX. So with this release, esbuild will now ignore `"jsx": "preserve"` in `tsconfig.json` files. If you want to preserve JSX syntax with esbuild, you now have to use `--jsx=preserve`.

        Note: Some people have suggested that esbuild's equivalent `jsx` setting override the one in `tsconfig.json`. However, some projects need to legitimately have different files within the same build use different transforms (i.e. `react` vs. `react-jsx`) and having esbuild's global `jsx` setting override `tsconfig.json` would prevent this from working. This release ignores `"jsx": "preserve"` but still allows other `jsx` values in `tsconfig.json` files to override esbuild's global `jsx` setting to keep the ability for multiple files within the same build to use different transforms.

    * `useDefineForClassFields` behavior has changed ([#2584](https://github.com/evanw/esbuild/issues/2584), [#2993](https://github.com/evanw/esbuild/issues/2993))

        Class fields in TypeScript look like this (`x` is a class field):

        ```js
        class Foo {
          x = 123
        }
        ```

        TypeScript has legacy behavior that uses assignment semantics instead of define semantics for class fields when [`useDefineForClassFields`](https://www.typescriptlang.org/tsconfig#useDefineForClassFields) is enabled (in which case class fields in TypeScript behave differently than they do in JavaScript, which is arguably "wrong").

        This legacy behavior exists because TypeScript added class fields to TypeScript before they were added to JavaScript. The TypeScript team decided to go with assignment semantics and shipped their implementation. Much later on TC39 decided to go with define semantics for class fields in JavaScript instead. This behaves differently if the base class has a setter with the same name:

        ```js
        class Base {
          set x(_) {
            console.log('x:', _)
          }
        }

        // useDefineForClassFields: false
        class AssignSemantics extends Base {
          constructor() {
            super()
            this.x = 123
          }
        }

        // useDefineForClassFields: true
        class DefineSemantics extends Base {
          constructor() {
            super()
            Object.defineProperty(this, 'x', { value: 123 })
          }
        }

        console.log(
          new AssignSemantics().x, // Calls the setter
          new DefineSemantics().x // Doesn't call the setter
        )
        ```

        When you run `tsc`, the value of `useDefineForClassFields` defaults to `false` when it's not specified and the `target` in `tsconfig.json` is present but earlier than `ES2022`. This sort of makes sense because the class field language feature was added in ES2022, so before ES2022 class fields didn't exist (and thus TypeScript's legacy behavior is active). However, TypeScript's `target` setting currently defaults to `ES3` which unfortunately means that the `useDefineForClassFields` setting currently defaults to false (i.e. to "wrong"). In other words if you run `tsc` with all default settings, class fields will behave incorrectly.

        Previously esbuild tried to do what `tsc` did. That meant esbuild's version of `useDefineForClassFields` was `false` by default, and was also `false` if esbuild's `--target=` was present but earlier than `es2022`. However, TypeScript's legacy class field behavior is becoming increasingly irrelevant and people who expect class fields in TypeScript to work like they do in JavaScript are confused when they use esbuild with default settings. It's also confusing that the behavior of class fields would change if you changed the language target (even though that's exactly how TypeScript works).

        So with this release, esbuild will now only use the information in `tsconfig.json` to determine whether `useDefineForClassFields` is true or not. Specifically `useDefineForClassFields` will be respected if present, otherwise it will be `false` if `target` is present in `tsconfig.json` and is `ES2021` or earlier, otherwise it will be `true`. Targets passed to esbuild's `--target=` setting will no longer affect `useDefineForClassFields`.

        Note that this means different directories in your build can have different values for this setting since esbuild allows different directories to have different `tsconfig.json` files within the same build. This should let you migrate your code one directory at a time without esbuild's `--target=` setting affecting the semantics of your code.

    * Add support for `verbatimModuleSyntax` from TypeScript 5.0

        TypeScript 5.0 added a new option called [`verbatimModuleSyntax`](https://www.typescriptlang.org/tsconfig#verbatimModuleSyntax) that deprecates and replaces two older options, `preserveValueImports` and `importsNotUsedAsValues`. Setting `verbatimModuleSyntax` to true in `tsconfig.json` tells esbuild to not drop unused import statements. Specifically esbuild now treats `"verbatimModuleSyntax": true` as if you had specified both `"preserveValueImports": true` and `"importsNotUsedAsValues": "preserve"`.

    * Add multiple inheritance for `tsconfig.json` from TypeScript 5.0

        TypeScript 5.0 now allows [multiple inheritance for `tsconfig.json` files](https://devblogs.microsoft.com/typescript/announcing-typescript-5-0/#supporting-multiple-configuration-files-in-extends). You can now pass an array of filenames via the `extends` parameter and your `tsconfig.json` will start off containing properties from all of those configuration files, in order. This release of esbuild adds support for this new TypeScript feature.

    * Remove support for `moduleSuffixes` ([#2395](https://github.com/evanw/esbuild/issues/2395))

        The community has requested that esbuild remove support for TypeScript's `moduleSuffixes` feature, so it has been removed in this release. Instead you can use esbuild's `--resolve-extensions=` feature to select which module suffix you want to build with.

    * Apply `--tsconfig=` overrides to `stdin` and virtual files ([#385](https://github.com/evanw/esbuild/issues/385), [#2543](https://github.com/evanw/esbuild/issues/2543))

        When you override esbuild's automatic `tsconfig.json` file detection with `--tsconfig=` to pass a specific `tsconfig.json` file, esbuild previously didn't apply these settings to source code passed via the `stdin` API option or to TypeScript files from plugins that weren't in the `file` namespace. This release changes esbuild's behavior so that settings from `tsconfig.json` also apply to these source code files as well.

    * Support `--tsconfig-raw=` in build API calls ([#943](https://github.com/evanw/esbuild/issues/943), [#2440](https://github.com/evanw/esbuild/issues/2440))

        Previously if you wanted to override esbuild's automatic `tsconfig.json` file detection, you had to create a new `tsconfig.json` file and pass the file name to esbuild via the `--tsconfig=` flag. With this release, you can now optionally use `--tsconfig-raw=` instead to pass the contents of `tsconfig.json` to esbuild directly instead of passing the file name. For example, you can now use `--tsconfig-raw={"compilerOptions":{"experimentalDecorators":true}}` to enable TypeScript experimental decorators directly using a command-line flag (assuming you escape the quotes correctly using your current shell's quoting rules). The `--tsconfig-raw=` flag previously only worked with transform API calls but with this release, it now works with build API calls too.

    * Ignore all `tsconfig.json` files in `node_modules` ([#276](https://github.com/evanw/esbuild/issues/276), [#2386](https://github.com/evanw/esbuild/issues/2386))

        This changes esbuild's behavior that applies `tsconfig.json` to all files in the subtree of the directory containing `tsconfig.json`. In version 0.12.7, esbuild started ignoring `tsconfig.json` files inside `node_modules` folders. The rationale is that people typically do this by mistake and that doing this intentionally is a rare use case that doesn't need to be supported. However, this change only applied to certain syntax-specific settings (e.g. `jsxFactory`) but did not apply to path resolution settings (e.g. `paths`). With this release, esbuild will now ignore all `tsconfig.json` files in `node_modules` instead of only ignoring certain settings.

    * Ignore `tsconfig.json` when resolving paths within `node_modules` ([#2481](https://github.com/evanw/esbuild/issues/2481))

        Previously fields in `tsconfig.json` related to path resolution (e.g. `paths`) were respected for all files in the subtree containing that `tsconfig.json` file, even within a nested `node_modules` subdirectory. This meant that a project's `paths` settings could potentially affect any bundled packages. With this release, esbuild will no longer use `tsconfig.json` settings during path resolution inside nested `node_modules` subdirectories.

    * Prefer `.js` over `.ts` within `node_modules` ([#3019](https://github.com/evanw/esbuild/issues/3019))

        The default list of implicit extensions that esbuild will try appending to import paths contains `.ts` before `.js`. This makes it possible to bundle TypeScript projects that reference other files in the project using extension-less imports (e.g. `./some-file` to load `./some-file.ts` instead of `./some-file.js`). However, this behavior is undesirable within `node_modules` directories. Some package authors publish both their original TypeScript code and their compiled JavaScript code side-by-side. In these cases, esbuild should arguably be using the compiled JavaScript files instead of the original TypeScript files because the TypeScript compilation settings for files within the package should be determined by the package author, not the user of esbuild. So with this release, esbuild will now prefer implicit `.js` extensions over `.ts` when searching for import paths within `node_modules`.

    These changes are intended to improve esbuild's compatibility with `tsc` and reduce the number of unfortunate behaviors regarding `tsconfig.json` and esbuild.

* Add a workaround for bugs in Safari 16.2 and earlier ([#3072](https://github.com/evanw/esbuild/issues/3072))

    Safari's JavaScript parser had a bug (which has now been fixed) where at least something about unary/binary operators nested inside default arguments nested inside either a function or class expression was incorrectly considered a syntax error if that expression was the target of a property assignment. Here are some examples that trigger this Safari bug:

    ```
    ❱ x(function (y = -1) {}.z = 2)
    SyntaxError: Left hand side of operator '=' must be a reference.

    ❱ x(class { f(y = -1) {} }.z = 2)
    SyntaxError: Left hand side of operator '=' must be a reference.
    ```

    It's not clear what the exact conditions are that trigger this bug. However, a workaround for this bug appears to be to post-process your JavaScript to wrap any in function and class declarations that are the direct target of a property access expression in parentheses. That's the workaround that UglifyJS applies for this issue: [mishoo/UglifyJS#2056](https://github.com/mishoo/UglifyJS/pull/2056). So that's what esbuild now does starting with this release:

    ```js
    // Original code
    x(function (y = -1) {}.z = 2, class { f(y = -1) {} }.z = 2)

    // Old output (with --minify --target=safari16.2)
    x(function(c=-1){}.z=2,class{f(c=-1){}}.z=2);

    // New output (with --minify --target=safari16.2)
    x((function(c=-1){}).z=2,(class{f(c=-1){}}).z=2);
    ```

    This fix is not enabled by default. It's only enabled when `--target=` contains Safari 16.2 or earlier, such as with `--target=safari16.2`. You can also explicitly enable or disable this specific transform (called `function-or-class-property-access`) with `--supported:function-or-class-property-access=false`.

* Fix esbuild's TypeScript type declarations to forbid unknown properties ([#3089](https://github.com/evanw/esbuild/issues/3089))

    Version 0.17.0 of esbuild introduced a specific form of function overloads in the TypeScript type definitions for esbuild's API calls that looks like this:

    ```ts
    interface TransformOptions {
      legalComments?: 'none' | 'inline' | 'eof' | 'external'
    }

    interface TransformResult<ProvidedOptions extends TransformOptions = TransformOptions> {
      legalComments: string | (ProvidedOptions['legalComments'] extends 'external' ? never : undefined)
    }

    declare function transformSync<ProvidedOptions extends TransformOptions>(input: string, options?: ProvidedOptions): TransformResult<ProvidedOptions>
    declare function transformSync(input: string, options?: TransformOptions): TransformResult
    ```

    This more accurately reflects how esbuild's JavaScript API behaves. The result object returned by `transformSync` only has the `legalComments` property if you pass `legalComments: 'external'`:

    ```ts
    // These have type "string | undefined"
    transformSync('').legalComments
    transformSync('', { legalComments: 'eof' }).legalComments

    // This has type "string"
    transformSync('', { legalComments: 'external' }).legalComments
    ```

    However, this form of function overloads unfortunately allows typos (e.g. `egalComments`) to pass the type checker without generating an error as TypeScript allows all objects with unknown properties to extend `TransformOptions`. These typos result in esbuild's API throwing an error at run-time.

    To prevent typos during type checking, esbuild's TypeScript type definitions will now use a different form that looks like this:

    ```ts
    type SameShape<Out, In extends Out> = In & { [Key in Exclude<keyof In, keyof Out>]: never }

    interface TransformOptions {
      legalComments?: 'none' | 'inline' | 'eof' | 'external'
    }

    interface TransformResult<ProvidedOptions extends TransformOptions = TransformOptions> {
      legalComments: string | (ProvidedOptions['legalComments'] extends 'external' ? never : undefined)
    }

    declare function transformSync<T extends TransformOptions>(input: string, options?: SameShape<TransformOptions, T>): TransformResult<T>
    ```

    This change should hopefully not affect correct code. It should hopefully introduce type errors only for incorrect code.

* Fix CSS nesting transform for pseudo-elements ([#3119](https://github.com/evanw/esbuild/issues/3119))

    This release fixes esbuild's CSS nesting transform for pseudo-elements (e.g. `::before` and `::after`). The CSS nesting specification says that [the nesting selector does not work with pseudo-elements](https://www.w3.org/TR/css-nesting-1/#ref-for-matches-pseudo%E2%91%A0). This can be seen in the example below: esbuild does not carry the parent pseudo-element `::before` through the nesting selector `&`. However, that doesn't apply to pseudo-elements that are within the same selector. Previously esbuild had a bug where it considered pseudo-elements in both locations as invalid. This release changes esbuild to only consider those from the parent selector invalid, which should align with the specification:

    ```css
    /* Original code */
    a, b::before {
      &.c, &::after {
        content: 'd';
      }
    }

    /* Old output (with --target=chrome90) */
    a:is(.c, ::after) {
      content: "d";
    }

    /* New output (with --target=chrome90) */
    a.c,
    a::after {
      content: "d";
    }
    ```

* Forbid `&` before a type selector in nested CSS

    The people behind the work-in-progress CSS nesting specification have very recently [decided to forbid nested CSS that looks like `&div`](https://github.com/w3c/csswg-drafts/issues/8662#issuecomment-1514977935). You will have to use either `div&` or `&:is(div)` instead. This release of esbuild has been updated to take this new change into consideration. Doing this now generates a warning. The suggested fix is slightly different depending on where in the overall selector it happened:

    ```
    ▲ [WARNING] Cannot use type selector "input" directly after nesting selector "&" [css-syntax-error]

        example.css:2:3:
          2 │   &input {
            │    ~~~~~
            ╵    :is(input)

      CSS nesting syntax does not allow the "&" selector to come before a type selector. You can wrap
      this selector in ":is()" as a workaround. This restriction exists to avoid problems with SASS
      nesting, where the same syntax means something very different that has no equivalent in real CSS
      (appending a suffix to the parent selector).

    ▲ [WARNING] Cannot use type selector "input" directly after nesting selector "&" [css-syntax-error]

        example.css:6:8:
          6 │   .form &input {
            │         ~~~~~~
            ╵         input&

      CSS nesting syntax does not allow the "&" selector to come before a type selector. You can move
      the "&" to the end of this selector as a workaround. This restriction exists to avoid problems
      with SASS nesting, where the same syntax means something very different that has no equivalent in
      real CSS (appending a suffix to the parent selector).
    ```

## 0.17.19

* Fix CSS transform bugs with nested selectors that start with a combinator ([#3096](https://github.com/evanw/esbuild/issues/3096))

    This release fixes several bugs regarding transforming nested CSS into non-nested CSS for older browsers. The bugs were due to lack of test coverage for nested selectors with more than one compound selector where they all start with the same combinator. Here's what some problematic cases look like before and after these fixes:

    ```css
    /* Original code */
    .foo {
      > &a,
      > &b {
        color: red;
      }
    }
    .bar {
      > &a,
      + &b {
        color: green;
      }
    }

    /* Old output (with --target=chrome90) */
    .foo :is(> .fooa, > .foob) {
      color: red;
    }
    .bar :is(> .bara, + .barb) {
      color: green;
    }

    /* New output (with --target=chrome90) */
    .foo > :is(a.foo, b.foo) {
      color: red;
    }
    .bar > a.bar,
    .bar + b.bar {
      color: green;
    }
    ```

* Fix bug with TypeScript parsing of instantiation expressions followed by `=` ([#3111](https://github.com/evanw/esbuild/issues/3111))

    This release fixes esbuild's TypeScript-to-JavaScript conversion code in the case where a potential instantiation expression is followed immediately by a `=` token (such that the trailing `>` becomes a `>=` token). Previously esbuild considered that to still be an instantiation expression, but the official TypeScript compiler considered it to be a `>=` operator instead. This release changes esbuild's interpretation to match TypeScript. This edge case currently [appears to be problematic](https://sucrase.io/#transforms=typescript&compareWithTypeScript=true&code=x%3Cy%3E%3Da%3Cb%3Cc%3E%3E()) for other TypeScript-to-JavaScript converters as well:

    | Original code | TypeScript | esbuild 0.17.18 | esbuild 0.17.19 | Sucrase | Babel |
    |---|---|---|---|---|---|
    | `x<y>=a<b<c>>()` | `x<y>=a();` | `x=a();` | `x<y>=a();` | `x=a()` | Invalid left-hand side in assignment expression |

* Avoid removing unrecognized directives from the directive prologue when minifying ([#3115](https://github.com/evanw/esbuild/issues/3115))

    The [directive prologue](https://262.ecma-international.org/6.0/#sec-directive-prologues-and-the-use-strict-directive) in JavaScript is a sequence of top-level string expressions that come before your code. The only directives that JavaScript engines currently recognize are `use strict` and sometimes `use asm`. However, the people behind React have made up their own directive for their own custom dialect of JavaScript. Previously esbuild only preserved the `use strict` directive when minifying, although you could still write React JavaScript with esbuild using something like `--banner:js="'your directive here';"`. With this release, you can now put arbitrary directives in the entry point and esbuild will preserve them in its minified output:

    ```js
    // Original code
    'use wtf'; console.log(123)

    // Old output (with --minify)
    console.log(123);

    // New output (with --minify)
    "use wtf";console.log(123);
    ```

    Note that this means esbuild will no longer remove certain stray top-level strings when minifying. This behavior is an intentional change because these stray top-level strings are actually part of the directive prologue, and could potentially have semantics assigned to them (as was the case with React).

* Improved minification of binary shift operators

    With this release, esbuild's minifier will now evaluate the `<<` and `>>>` operators if the resulting code would be shorter:

    ```js
    // Original code
    console.log(10 << 10, 10 << 20, -123 >>> 5, -123 >>> 10);

    // Old output (with --minify)
    console.log(10<<10,10<<20,-123>>>5,-123>>>10);

    // New output (with --minify)
    console.log(10240,10<<20,-123>>>5,4194303);
    ```

## 0.17.18

* Fix non-default JSON import error with `export {} from` ([#3070](https://github.com/evanw/esbuild/issues/3070))

    This release fixes a bug where esbuild incorrectly identified statements of the form `export { default as x } from "y" assert { type: "json" }` as a non-default import. The bug did not affect code of the form `import { default as x } from ...` (only code that used the `export` keyword).

* Fix a crash with an invalid subpath import ([#3067](https://github.com/evanw/esbuild/issues/3067))

    Previously esbuild could crash when attempting to generate a friendly error message for an invalid [subpath import](https://nodejs.org/api/packages.html#subpath-imports) (i.e. an import starting with `#`). This happened because esbuild originally only supported the `exports` field and the code for that error message was not updated when esbuild later added support for the `imports` field. This crash has been fixed.

## 0.17.17

* Fix CSS nesting transform for top-level `&` ([#3052](https://github.com/evanw/esbuild/issues/3052))

    Previously esbuild could crash with a stack overflow when lowering CSS nesting rules with a top-level `&`, such as in the code below. This happened because esbuild's CSS nesting transform didn't handle top-level `&`, causing esbuild to inline the top-level selector into itself. This release handles top-level `&` by replacing it with [the `:scope` pseudo-class](https://drafts.csswg.org/selectors-4/#the-scope-pseudo):

    ```css
    /* Original code */
    &,
    a {
      .b {
        color: red;
      }
    }

    /* New output (with --target=chrome90) */
    :is(:scope, a) .b {
      color: red;
    }
    ```

* Support `exports` in `package.json` for `extends` in `tsconfig.json` ([#3058](https://github.com/evanw/esbuild/issues/3058))

    TypeScript 5.0 added the ability to use `extends` in `tsconfig.json` to reference a path in a package whose `package.json` file contains an `exports` map that points to the correct location. This doesn't automatically work in esbuild because `tsconfig.json` affects esbuild's path resolution, so esbuild's normal path resolution logic doesn't apply.

    This release adds support for doing this by adding some additional code that attempts to resolve the `extends` path using the `exports` field. The behavior should be similar enough to esbuild's main path resolution logic to work as expected.

    Note that esbuild always treats this `extends` import as a `require()` import since that's what TypeScript appears to do. Specifically the `require` condition will be active and the `import` condition will be inactive.

* Fix watch mode with `NODE_PATH` ([#3062](https://github.com/evanw/esbuild/issues/3062))

    Node has a rarely-used feature where you can extend the set of directories that node searches for packages using the `NODE_PATH` environment variable. While esbuild supports this too, previously a bug prevented esbuild's watch mode from picking up changes to imported files that were contained directly in a `NODE_PATH` directory. You're supposed to use `NODE_PATH` for packages, but some people abuse this feature by putting files in that directory instead (e.g. `node_modules/some-file.js` instead of `node_modules/some-pkg/some-file.js`). The watch mode bug happens when you do this because esbuild first tries to read `some-file.js` as a directory and then as a file. Watch mode was incorrectly waiting for `some-file.js` to become a valid directory. This release fixes this edge case bug by changing watch mode to watch `some-file.js` as a file when this happens.

## 0.17.16

* Fix CSS nesting transform for triple-nested rules that start with a combinator ([#3046](https://github.com/evanw/esbuild/issues/3046))

    This release fixes a bug with esbuild where triple-nested CSS rules that start with a combinator were not transformed correctly for older browsers. Here's an example of such a case before and after this bug fix:

    ```css
    /* Original input */
    .a {
      color: red;
      > .b {
        color: green;
        > .c {
          color: blue;
        }
      }
    }

    /* Old output (with --target=chrome90) */
    .a {
      color: red;
    }
    .a > .b {
      color: green;
    }
    .a .b > .c {
      color: blue;
    }

    /* New output (with --target=chrome90) */
    .a {
      color: red;
    }
    .a > .b {
      color: green;
    }
    .a > .b > .c {
      color: blue;
    }
    ```

* Support `--inject` with a file loaded using the `copy` loader ([#3041](https://github.com/evanw/esbuild/issues/3041))

    This release now allows you to use `--inject` with a file that is loaded using the `copy` loader. The `copy` loader copies the imported file to the output directory verbatim and rewrites the path in the `import` statement to point to the copied output file. When used with `--inject`, this means the injected file will be copied to the output directory as-is and a bare `import` statement for that file will be inserted in any non-copy output files that esbuild generates.

    Note that since esbuild doesn't parse the contents of copied files, esbuild will not expose any of the export names as usable imports when you do this (in the way that esbuild's `--inject` feature is typically used). However, any side-effects that the injected file has will still occur.

## 0.17.15

* Allow keywords as type parameter names in mapped types ([#3033](https://github.com/evanw/esbuild/issues/3033))

    TypeScript allows type keywords to be used as parameter names in mapped types. Previously esbuild incorrectly treated this as an error. Code that does this is now supported:

    ```ts
    type Foo = 'a' | 'b' | 'c'
    type A = { [keyof in Foo]: number }
    type B = { [infer in Foo]: number }
    type C = { [readonly in Foo]: number }
    ```

* Add annotations for re-exported modules in node ([#2486](https://github.com/evanw/esbuild/issues/2486), [#3029](https://github.com/evanw/esbuild/issues/3029))

    Node lets you import named imports from a CommonJS module using ESM import syntax. However, the allowed names aren't derived from the properties of the CommonJS module. Instead they are derived from an arbitrary syntax-only analysis of the CommonJS module's JavaScript AST.

    To accommodate node doing this, esbuild's ESM-to-CommonJS conversion adds a special non-executable "annotation" for node that describes the exports that node should expose in this scenario. It takes the form `0 && (module.exports = { ... })` and comes at the end of the file (`0 && expr` means `expr` is never evaluated).

    Previously esbuild didn't do this for modules re-exported using the `export * from` syntax. Annotations for these re-exports will now be added starting with this release:

    ```js
    // Original input
    export { foo } from './foo'
    export * from './bar'

    // Old output (with --format=cjs --platform=node)
    ...
    0 && (module.exports = {
      foo
    });

    // New output (with --format=cjs --platform=node)
    ...
    0 && (module.exports = {
      foo,
      ...require("./bar")
    });
    ```

    Note that you need to specify both `--format=cjs` and `--platform=node` to get these node-specific annotations.

* Avoid printing an unnecessary space in between a number and a `.` ([#3026](https://github.com/evanw/esbuild/pull/3026))

    JavaScript typically requires a space in between a number token and a `.` token to avoid the `.` being interpreted as a decimal point instead of a member expression. However, this space is not required if the number token itself contains a decimal point, an exponent, or uses a base other than 10. This release of esbuild now avoids printing the unnecessary space in these cases:

    ```js
    // Original input
    foo(1000 .x, 0 .x, 0.1 .x, 0.0001 .x, 0xFFFF_0000_FFFF_0000 .x)

    // Old output (with --minify)
    foo(1e3 .x,0 .x,.1 .x,1e-4 .x,0xffff0000ffff0000 .x);

    // New output (with --minify)
    foo(1e3.x,0 .x,.1.x,1e-4.x,0xffff0000ffff0000.x);
    ```

* Fix server-sent events with live reload when writing to the file system root ([#3027](https://github.com/evanw/esbuild/issues/3027))

    This release fixes a bug where esbuild previously failed to emit server-sent events for live reload when `outdir` was the file system root, such as `/`. This happened because `/` is the only path on Unix that cannot have a trailing slash trimmed from it, which was fixed by improved path handling.

## 0.17.14

* Allow the TypeScript 5.0 `const` modifier in object type declarations ([#3021](https://github.com/evanw/esbuild/issues/3021))

    The new TypeScript 5.0 `const` modifier was added to esbuild in version 0.17.5, and works with classes, functions, and arrow expressions. However, support for it wasn't added to object type declarations (e.g. interfaces) due to an oversight. This release adds support for these cases, so the following TypeScript 5.0 code can now be built with esbuild:

    ```ts
    interface Foo { <const T>(): T }
    type Bar = { new <const T>(): T }
    ```

* Implement preliminary lowering for CSS nesting ([#1945](https://github.com/evanw/esbuild/issues/1945))

    Chrome has [implemented the new CSS nesting specification](https://developer.chrome.com/articles/css-nesting/) in version 112, which is currently in beta but will become stable very soon. So CSS nesting is now a part of the web platform!

    This release of esbuild can now transform nested CSS syntax into non-nested CSS syntax for older browsers. The transformation relies on the `:is()` pseudo-class in many cases, so the transformation is only guaranteed to work when targeting browsers that support `:is()` (e.g. Chrome 88+). You'll need to set esbuild's [`target`](https://esbuild.github.io/api/#target) to the browsers you intend to support to tell esbuild to do this transformation. You will get a warning if you use CSS nesting syntax with a `target` which includes older browsers that don't support `:is()`.

    The lowering transformation looks like this:

    ```css
    /* Original input */
    a.btn {
      color: #333;
      &:hover { color: #444 }
      &:active { color: #555 }
    }

    /* New output (with --target=chrome88) */
    a.btn {
      color: #333;
    }
    a.btn:hover {
      color: #444;
    }
    a.btn:active {
      color: #555;
    }
    ```

    More complex cases may generate the `:is()` pseudo-class:

    ```css
    /* Original input */
    div, p {
      .warning, .error {
        padding: 20px;
      }
    }

    /* New output (with --target=chrome88) */
    :is(div, p) :is(.warning, .error) {
      padding: 20px;
    }
    ```

    In addition, esbuild now has a special warning message for nested style rules that start with an identifier. This isn't allowed in CSS because the syntax would be ambiguous with the existing declaration syntax. The new warning message looks like this:

    ```
    ▲ [WARNING] A nested style rule cannot start with "p" because it looks like the start of a declaration [css-syntax-error]

        <stdin>:1:7:
          1 │ main { p { margin: auto } }
            │        ^
            ╵        :is(p)

      To start a nested style rule with an identifier, you need to wrap the identifier in ":is(...)" to
      prevent the rule from being parsed as a declaration.
    ```

    Keep in mind that the transformation in this release is a preliminary implementation. CSS has many features that interact in complex ways, and there may be some edge cases that don't work correctly yet.

* Minification now removes unnecessary `&` CSS nesting selectors

    This release introduces the following CSS minification optimizations:

    ```css
    /* Original input */
    a {
      font-weight: bold;
      & {
        color: blue;
      }
      & :hover {
        text-decoration: underline;
      }
    }

    /* Old output (with --minify) */
    a{font-weight:700;&{color:#00f}& :hover{text-decoration:underline}}

    /* New output (with --minify) */
    a{font-weight:700;:hover{text-decoration:underline}color:#00f}
    ```

* Minification now removes duplicates from CSS selector lists

    This release introduces the following CSS minification optimization:

    ```css
    /* Original input */
    div, div { color: red }

    /* Old output (with --minify) */
    div,div{color:red}

    /* New output (with --minify) */
    div{color:red}
    ```

## 0.17.13

* Work around an issue with `NODE_PATH` and Go's WebAssembly internals ([#3001](https://github.com/evanw/esbuild/issues/3001))

    Go's WebAssembly implementation returns `EINVAL` instead of `ENOTDIR` when using the `readdir` syscall on a file. This messes up esbuild's implementation of node's module resolution algorithm since encountering `ENOTDIR` causes esbuild to continue its search (since it's a normal condition) while other encountering other errors causes esbuild to fail with an I/O error (since it's an unexpected condition). You can encounter this issue in practice if you use node's legacy `NODE_PATH` feature to tell esbuild to resolve node modules in a custom directory that was not installed by npm. This release works around this problem by converting `EINVAL` into `ENOTDIR` for the `readdir` syscall.

* Fix a minification bug with CSS `@layer` rules that have parsing errors ([#3016](https://github.com/evanw/esbuild/issues/3016))

    CSS at-rules [require either a `{}` block or a semicolon at the end](https://www.w3.org/TR/css-syntax-3/#consume-at-rule). Omitting both of these causes esbuild to treat the rule as an unknown at-rule. Previous releases of esbuild had a bug that incorrectly removed unknown at-rules without any children during minification if the at-rule token matched an at-rule that esbuild can handle. Specifically [cssnano](https://cssnano.co/) can generate `@layer` rules with parsing errors, and empty `@layer` rules cannot be removed because they have side effects (`@layer` didn't exist when esbuild's CSS support was added, so esbuild wasn't written to handle this). This release changes esbuild to no longer discard `@layer` rules with parsing errors when minifying (the rule `@layer c` has a parsing error):

    ```css
    /* Original input */
    @layer a {
      @layer b {
        @layer c
      }
    }

    /* Old output (with --minify) */
    @layer a.b;

    /* New output (with --minify) */
    @layer a.b.c;
    ```

* Unterminated strings in CSS are no longer an error

    The CSS specification provides [rules for handling parsing errors](https://www.w3.org/TR/CSS22/syndata.html#parsing-errors). One of those rules is that user agents must close strings upon reaching the end of a line (i.e., before an unescaped line feed, carriage return or form feed character), but then drop the construct (declaration or rule) in which the string was found. For example:

    ```css
    p {
      color: green;
      font-family: 'Courier New Times
      color: red;
      color: green;
    }
    ```

    ...would be treated the same as:

    ```css
    p { color: green; color: green; }
    ```

    ...because the second declaration (from `font-family` to the semicolon after `color: red`) is invalid and is dropped.

    Previously using this CSS with esbuild failed to build due to a syntax error, even though the code can be interpreted by a browser. With this release, the code now produces a warning instead of an error, and esbuild prints the invalid CSS such that it stays invalid in the output:

    ```css
    /* esbuild's new non-minified output: */
    p {
      color: green;
      font-family: 'Courier New Times
      color: red;
      color: green;
    }
    ```

    ```css
    /* esbuild's new minified output: */
    p{font-family:'Courier New Times
    color: red;color:green}
    ```

## 0.17.12

* Fix a crash when parsing inline TypeScript decorators ([#2991](https://github.com/evanw/esbuild/issues/2991))

    Previously esbuild's TypeScript parser crashed when parsing TypeScript decorators if the definition of the decorator was inlined into the decorator itself:

    ```ts
    @(function sealed(constructor: Function) {
      Object.seal(constructor);
      Object.seal(constructor.prototype);
    })
    class Foo {}
    ```

    This crash was not noticed earlier because this edge case did not have test coverage. The crash is fixed in this release.

## 0.17.11

* Fix the `alias` feature to always prefer the longest match ([#2963](https://github.com/evanw/esbuild/issues/2963))

    It's possible to configure conflicting aliases such as `--alias:a=b` and `--alias:a/c=d`, which is ambiguous for the import path `a/c/x` (since it could map to either `b/c/x` or `d/x`). Previously esbuild would pick the first matching `alias`, which would non-deterministically pick between one of the possible matches. This release fixes esbuild to always deterministically pick the longest possible match.

* Minify calls to some global primitive constructors ([#2962](https://github.com/evanw/esbuild/issues/2962))

    With this release, esbuild's minifier now replaces calls to `Boolean`/`Number`/`String`/`BigInt` with equivalent shorter code when relevant:

    ```js
    // Original code
    console.log(
      Boolean(a ? (b | c) !== 0 : (c & d) !== 0),
      Number(e ? '1' : '2'),
      String(e ? '1' : '2'),
      BigInt(e ? 1n : 2n),
    )

    // Old output (with --minify)
    console.log(Boolean(a?(b|c)!==0:(c&d)!==0),Number(e?"1":"2"),String(e?"1":"2"),BigInt(e?1n:2n));

    // New output (with --minify)
    console.log(!!(a?b|c:c&d),+(e?"1":"2"),e?"1":"2",e?1n:2n);
    ```

* Adjust some feature compatibility tables for node ([#2940](https://github.com/evanw/esbuild/issues/2940))

    This release makes the following adjustments to esbuild's internal feature compatibility tables for node, which tell esbuild which versions of node are known to support all aspects of that feature:

    * `class-private-brand-checks`: node v16.9+ => node v16.4+ (a decrease)
    * `hashbang`: node v12.0+ => node v12.5+ (an increase)
    * `optional-chain`: node v16.9+ => node v16.1+ (a decrease)
    * `template-literal`: node v4+ => node v10+ (an increase)

    Each of these adjustments was identified by comparing against data from the `node-compat-table` package and was manually verified using old node executables downloaded from https://nodejs.org/download/release/.

## 0.17.10

* Update esbuild's handling of CSS nesting to match the latest specification changes ([#1945](https://github.com/evanw/esbuild/issues/1945))

    The syntax for the upcoming CSS nesting feature has [recently changed](https://webkit.org/blog/13813/try-css-nesting-today-in-safari-technology-preview/). The `@nest` prefix that was previously required in some cases is now gone, and nested rules no longer have to start with `&` (as long as they don't start with an identifier or function token).

    This release updates esbuild's pass-through handling of CSS nesting syntax to match the latest specification changes. So you can now use esbuild to bundle CSS containing nested rules and try them out in a browser that supports CSS nesting (which includes nightly builds of both Chrome and Safari).

    However, I'm not implementing lowering of nested CSS to non-nested CSS for older browsers yet. While the syntax has been decided, the semantics are still in flux. In particular, there is still some debate about changing the fundamental way that CSS nesting works. For example, you might think that the following CSS is equivalent to a `.outer .inner button { ... }` rule:

    ```css
    .inner button {
      .outer & {
        color: red;
      }
    }
    ```

    But instead it's actually equivalent to a `.outer :is(.inner button) { ... }` rule which unintuitively also matches the following DOM structure:

    ```html
    <div class="inner">
      <div class="outer">
        <button></button>
      </div>
    </div>
    ```

    The `:is()` behavior is preferred by browser implementers because it's more memory-efficient, but the straightforward translation into a `.outer .inner button { ... }` rule is preferred by developers used to the existing CSS preprocessing ecosystem (e.g. SASS). It seems premature to commit esbuild to specific semantics for this syntax at this time given the ongoing debate.

* Fix cross-file CSS rule deduplication involving `url()` tokens ([#2936](https://github.com/evanw/esbuild/issues/2936))

    Previously cross-file CSS rule deduplication didn't handle `url()` tokens correctly. These tokens contain references to import paths which may be internal (i.e. in the bundle) or external (i.e. not in the bundle). When comparing two `url()` tokens for equality, the underlying import paths should be compared instead of their references. This release of esbuild fixes `url()` token comparisons. One side effect is that `@font-face` rules should now be deduplicated correctly across files:

    ```css
    /* Original code */
    @import "data:text/css, \
      @import 'http://example.com/style.css'; \
      @font-face { src: url(http://example.com/font.ttf) }";
    @import "data:text/css, \
      @font-face { src: url(http://example.com/font.ttf) }";

    /* Old output (with --bundle --minify) */
    @import"http://example.com/style.css";@font-face{src:url(http://example.com/font.ttf)}@font-face{src:url(http://example.com/font.ttf)}

    /* New output (with --bundle --minify) */
    @import"http://example.com/style.css";@font-face{src:url(http://example.com/font.ttf)}
    ```

## 0.17.9

* Parse rest bindings in TypeScript types ([#2937](https://github.com/evanw/esbuild/issues/2937))

    Previously esbuild was unable to parse the following valid TypeScript code:

    ```ts
    let tuple: (...[e1, e2, ...es]: any) => any
    ```

    This release includes support for parsing code like this.

* Fix TypeScript code translation for certain computed `declare` class fields ([#2914](https://github.com/evanw/esbuild/issues/2914))

    In TypeScript, the key of a computed `declare` class field should only be preserved if there are no decorators for that field. Previously esbuild always preserved the key, but esbuild will now remove the key to match the output of the TypeScript compiler:

    ```ts
    // Original code
    declare function dec(a: any, b: any): any
    declare const removeMe: unique symbol
    declare const keepMe: unique symbol
    class X {
        declare [removeMe]: any
        @dec declare [keepMe]: any
    }

    // Old output
    var _a;
    class X {
    }
    removeMe, _a = keepMe;
    __decorateClass([
      dec
    ], X.prototype, _a, 2);

    // New output
    var _a;
    class X {
    }
    _a = keepMe;
    __decorateClass([
      dec
    ], X.prototype, _a, 2);
    ```

* Fix a crash with path resolution error generation ([#2913](https://github.com/evanw/esbuild/issues/2913))

    In certain situations, a module containing an invalid import path could previously cause esbuild to crash when it attempts to generate a more helpful error message. This crash has been fixed.

## 0.17.8

* Fix a minification bug with non-ASCII identifiers ([#2910](https://github.com/evanw/esbuild/issues/2910))

    This release fixes a bug with esbuild where non-ASCII identifiers followed by a keyword were incorrectly not separated by a space. This bug affected both the `in` and `instanceof` keywords. Here's an example of the fix:

    ```js
    // Original code
    π in a

    // Old output (with --minify --charset=utf8)
    πin a;

    // New output (with --minify --charset=utf8)
    π in a;
    ```

* Fix a regression with esbuild's WebAssembly API in version 0.17.6 ([#2911](https://github.com/evanw/esbuild/issues/2911))

    Version 0.17.6 of esbuild updated the Go toolchain to version 1.20.0. This had the unfortunate side effect of increasing the amount of stack space that esbuild uses (presumably due to some changes to Go's WebAssembly implementation) which could cause esbuild's WebAssembly-based API to crash with a stack overflow in cases where it previously didn't crash. One such case is the package `grapheme-splitter` which contains code that looks like this:

    ```js
    if (
      (0x0300 <= code && code <= 0x036F) ||
      (0x0483 <= code && code <= 0x0487) ||
      (0x0488 <= code && code <= 0x0489) ||
      (0x0591 <= code && code <= 0x05BD) ||
      // ... many hundreds of lines later ...
    ) {
      return;
    }
    ```

    This edge case involves a chain of binary operators that results in an AST over 400 nodes deep. Normally this wouldn't be a problem because Go has growable call stacks, so the call stack would just grow to be as large as needed. However, WebAssembly byte code deliberately doesn't expose the ability to manipulate the stack pointer, so Go's WebAssembly translation is forced to use the fixed-size WebAssembly call stack. So esbuild's WebAssembly implementation is vulnerable to stack overflow in cases like these.

    It's not unreasonable for this to cause a stack overflow, and for esbuild's answer to this problem to be "don't write code like this." That's how many other AST-manipulation tools handle this problem. However, it's possible to implement AST traversal using iteration instead of recursion to work around limited call stack space. This version of esbuild implements this code transformation for esbuild's JavaScript parser and printer, so esbuild's WebAssembly implementation is now able to process the `grapheme-splitter` package (at least when compiled with Go 1.20.0 and run with node's WebAssembly implementation).

## 0.17.7

* Change esbuild's parsing of TypeScript instantiation expressions to match TypeScript 4.8+ ([#2907](https://github.com/evanw/esbuild/issues/2907))

    This release updates esbuild's implementation of instantiation expression erasure to match [microsoft/TypeScript#49353](https://github.com/microsoft/TypeScript/pull/49353). The new rules are as follows (copied from TypeScript's PR description):

    > When a potential type argument list is followed by
    >
    > * a line break,
    > * an `(` token,
    > * a template literal string, or
    > * any token except `<` or `>` that isn't the start of an expression,
    >
    > we consider that construct to be a type argument list. Otherwise we consider the construct to be a `<` relational expression followed by a `>` relational expression.

* Ignore `sideEffects: false` for imported CSS files ([#1370](https://github.com/evanw/esbuild/issues/1370), [#1458](https://github.com/evanw/esbuild/pull/1458), [#2905](https://github.com/evanw/esbuild/issues/2905))

    This release ignores the `sideEffects` annotation in `package.json` for CSS files that are imported into JS files using esbuild's `css` loader. This means that these CSS files are no longer be tree-shaken.

    Importing CSS into JS causes esbuild to automatically create a CSS entry point next to the JS entry point containing the bundled CSS. Previously packages that specified some form of `"sideEffects": false` could potentially cause esbuild to consider one or more of the JS files on the import path to the CSS file to be side-effect free, which would result in esbuild removing that CSS file from the bundle. This was problematic because the removal of that CSS is outwardly observable, since all CSS is global, so it was incorrect for previous versions of esbuild to tree-shake CSS files imported into JS files.

* Add constant folding for certain additional equality cases ([#2394](https://github.com/evanw/esbuild/issues/2394), [#2895](https://github.com/evanw/esbuild/issues/2895))

    This release adds constant folding for expressions similar to the following:

    ```js
    // Original input
    console.log(
      null === 'foo',
      null === undefined,
      null == undefined,
      false === 0,
      false == 0,
      1 === true,
      1 == true,
    )

    // Old output
    console.log(
      null === "foo",
      null === void 0,
      null == void 0,
      false === 0,
      false == 0,
      1 === true,
      1 == true
    );

    // New output
    console.log(
      false,
      false,
      true,
      false,
      true,
      false,
      true
    );
    ```

## 0.17.6

* Fix a CSS parser crash on invalid CSS ([#2892](https://github.com/evanw/esbuild/issues/2892))

    Previously the following invalid CSS caused esbuild's parser to crash:

    ```css
    @media screen
    ```

    The crash was caused by trying to construct a helpful error message assuming that there was an opening `{` token, which is not the case here. This release fixes the crash.

* Inline TypeScript enums that are referenced before their declaration

    Previously esbuild inlined enums within a TypeScript file from top to bottom, which meant that references to TypeScript enum members were only inlined within the same file if they came after the enum declaration. With this release, esbuild will now inline enums even when they are referenced before they are declared:

    ```ts
    // Original input
    export const foo = () => Foo.FOO
    const enum Foo { FOO = 0 }

    // Old output (with --tree-shaking=true)
    export const foo = () => Foo.FOO;
    var Foo = /* @__PURE__ */ ((Foo2) => {
      Foo2[Foo2["FOO"] = 0] = "FOO";
      return Foo2;
    })(Foo || {});

    // New output (with --tree-shaking=true)
    export const foo = () => 0 /* FOO */;
    ```

    This makes esbuild's TypeScript output smaller and faster when processing code that does this. I noticed this issue when I ran the TypeScript compiler's source code through esbuild's bundler. Now that the TypeScript compiler is going to be bundled with esbuild in the upcoming TypeScript 5.0 release, improvements like this will also improve the TypeScript compiler itself!

* Fix esbuild installation on Arch Linux ([#2785](https://github.com/evanw/esbuild/issues/2785), [#2812](https://github.com/evanw/esbuild/issues/2812), [#2865](https://github.com/evanw/esbuild/issues/2865))

    Someone made an unofficial `esbuild` package for Linux that adds the `ESBUILD_BINARY_PATH=/usr/bin/esbuild` environment variable to the user's default environment. This breaks all npm installations of esbuild for users with this unofficial Linux package installed, which has affected many people. Most (all?) people who encounter this problem haven't even installed this unofficial package themselves; instead it was installed for them as a dependency of another Linux package. The problematic change to add the `ESBUILD_BINARY_PATH` environment variable was reverted in the latest version of this unofficial package. However, old versions of this unofficial package are still there and will be around forever. With this release, `ESBUILD_BINARY_PATH` is now ignored by esbuild's install script when it's set to the value `/usr/bin/esbuild`. This should unbreak using npm to install `esbuild` in these problematic Linux environments.

    Note: The `ESBUILD_BINARY_PATH` variable is an undocumented way to override the location of esbuild's binary when esbuild's npm package is installed, which is necessary to substitute your own locally-built esbuild binary when debugging esbuild's npm package. It's only meant for very custom situations and should absolutely not be forced on others by default, especially without their knowledge. I may remove the code in esbuild's installer that reads `ESBUILD_BINARY_PATH` in the future to prevent these kinds of issues. It will unfortunately make debugging esbuild harder. If `ESBUILD_BINARY_PATH` is ever removed, it will be done in a "breaking change" release.

## 0.17.5

* Parse `const` type parameters from TypeScript 5.0

    The TypeScript 5.0 beta announcement adds [`const` type parameters](https://devblogs.microsoft.com/typescript/announcing-typescript-5-0-beta/#const-type-parameters) to the language. You can now add the `const` modifier on a type parameter of a function, method, or class like this:

    ```ts
    type HasNames = { names: readonly string[] };
    const getNamesExactly = <const T extends HasNames>(arg: T): T["names"] => arg.names;
    const names = getNamesExactly({ names: ["Alice", "Bob", "Eve"] });
    ```

    The type of `names` in the above example is `readonly ["Alice", "Bob", "Eve"]`. Marking the type parameter as `const` behaves as if you had written `as const` at every use instead. The above code is equivalent to the following TypeScript, which was the only option before TypeScript 5.0:

    ```ts
    type HasNames = { names: readonly string[] };
    const getNamesExactly = <T extends HasNames>(arg: T): T["names"] => arg.names;
    const names = getNamesExactly({ names: ["Alice", "Bob", "Eve"] } as const);
    ```

    You can read [the announcement](https://devblogs.microsoft.com/typescript/announcing-typescript-5-0-beta/#const-type-parameters) for more information.

* Make parsing generic `async` arrow functions more strict in `.tsx` files

    Previously esbuild's TypeScript parser incorrectly accepted the following code as valid:

    ```tsx
    let fn = async <T> () => {};
    ```

    The official TypeScript parser rejects this code because it thinks it's the identifier `async` followed by a JSX element starting with `<T>`. So with this release, esbuild will now reject this syntax in `.tsx` files too. You'll now have to add a comma after the type parameter to get generic arrow functions like this to parse in `.tsx` files:

    ```tsx
    let fn = async <T,> () => {};
    ```

* Allow the `in` and `out` type parameter modifiers on class expressions

    TypeScript 4.7 added the `in` and `out` modifiers on the type parameters of classes, interfaces, and type aliases. However, while TypeScript supported them on both class expressions and class statements, previously esbuild only supported them on class statements due to an oversight. This release now allows these modifiers on class expressions too:

    ```ts
    declare let Foo: any;
    Foo = class <in T> { };
    Foo = class <out T> { };
    ```

* Update `enum` constant folding for TypeScript 5.0

    TypeScript 5.0 contains an [updated definition of what it considers a constant expression](https://github.com/microsoft/TypeScript/pull/50528):

    > An expression is considered a *constant expression* if it is
    >
    > * a number or string literal,
    > * a unary `+`, `-`, or `~` applied to a numeric constant expression,
    > * a binary `+`, `-`, `*`, `/`, `%`, `**`, `<<`, `>>`, `>>>`, `|`, `&`, `^` applied to two numeric constant expressions,
    > * a binary `+` applied to two constant expressions whereof at least one is a string,
    > * a template expression where each substitution expression is a constant expression,
    > * a parenthesized constant expression,
    > * a dotted name (e.g. `x.y.z`) that references a `const` variable with a constant expression initializer and no type annotation,
    > * a dotted name that references an enum member with an enum literal type, or
    > * a dotted name indexed by a string literal (e.g. `x.y["z"]`) that references an enum member with an enum literal type.

    This impacts esbuild's implementation of TypeScript's `const enum` feature. With this release, esbuild will now attempt to follow these new rules. For example, you can now initialize an `enum` member with a template literal expression that contains a numeric constant:

    ```ts
    // Original input
    const enum Example {
      COUNT = 100,
      ERROR = `Expected ${COUNT} items`,
    }
    console.log(
      Example.COUNT,
      Example.ERROR,
    )

    // Old output (with --tree-shaking=true)
    var Example = /* @__PURE__ */ ((Example2) => {
      Example2[Example2["COUNT"] = 100] = "COUNT";
      Example2[Example2["ERROR"] = `Expected ${100 /* COUNT */} items`] = "ERROR";
      return Example2;
    })(Example || {});
    console.log(
      100 /* COUNT */,
      Example.ERROR
    );

    // New output (with --tree-shaking=true)
    console.log(
      100 /* COUNT */,
      "Expected 100 items" /* ERROR */
    );
    ```

    These rules are not followed exactly due to esbuild's limitations. The rule about dotted references to `const` variables is not followed both because esbuild's enum processing is done in an isolated module setting and because doing so would potentially require esbuild to use a type system, which it doesn't have. For example:

    ```ts
    // The TypeScript compiler inlines this but esbuild doesn't:
    declare const x = 'foo'
    const enum Foo { X = x }
    console.log(Foo.X)
    ```

    Also, the rule that requires converting numbers to a string currently only followed for 32-bit signed integers and non-finite numbers. This is done to avoid accidentally introducing a bug if esbuild's number-to-string operation doesn't exactly match the behavior of a real JavaScript VM. Currently esbuild's number-to-string constant folding is conservative for safety.

* Forbid definite assignment assertion operators on class methods

    In TypeScript, class methods can use the `?` optional property operator but not the `!` definite assignment assertion operator (while class fields can use both):

    ```ts
    class Foo {
      // These are valid TypeScript
      a?
      b!
      x?() {}

      // This is invalid TypeScript
      y!() {}
    }
    ```

    Previously esbuild incorrectly allowed the definite assignment assertion operator with class methods. This will no longer be allowed starting with this release.

## 0.17.4

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

* Support replacing property access expressions with inject

    At a high level, this change means the `inject` feature can now replace all of the same kinds of names as the `define` feature. So `inject` is basically now a more powerful version of `define`, instead of previously only being able to do some of the things that `define` could do.

    Soem background is necessary to understand this change if you aren't already familiar with the `inject` feature. The `inject` feature lets you replace references to global variable with a shim. It works like this:

    1. Put the shim in its own file
    2. Export the shim as the name of the global variable you intend to replace
    3. Pass the file to esbuild using the `inject` feature

    For example, if you inject the following file using `--inject:./injected.js`:

    ```js
    // injected.js
    let processShim = { cwd: () => '/' }
    export { processShim as process }
    ```

    Then esbuild will replace all references to `process` with the `processShim` variable, which will cause `process.cwd()` to return `'/'`. This feature is sort of abusing the ESM export alias syntax to specify the mapping of global variables to shims. But esbuild works this way because using this syntax for that purpose is convenient and terse.

    However, if you wanted to replace a property access expression, the process was more complicated and not as nice. You would have to:

    1. Put the shim in its own file
    2. Export the shim as some random name
    3. Pass the file to esbuild using the `inject` feature
    4. Use esbuild's `define` feature to map the property access expression to the random name you made in step 2

    For example, if you inject the following file using `--inject:./injected2.js --define:process.cwd=someRandomName`:

    ```js
    // injected2.js
    let cwdShim = () => '/'
    export { cwdShim as someRandomName }
    ```

    Then esbuild will replace all references to `process.cwd` with the `cwdShim` variable, which will also cause `process.cwd()` to return `'/'` (but which this time will not mess with other references to `process`, which might be desirable).

    With this release, using the inject feature to replace a property access expression is now as simple as using it to replace an identifier. You can now use JavaScript's ["arbitrary module namespace identifier names"](https://github.com/tc39/ecma262/pull/2154) feature to specify the property access expression directly using a string literal. For example, if you inject the following file using `--inject:./injected3.js`:

    ```js
    // injected3.js
    let cwdShim = () => '/'
    export { cwdShim as 'process.cwd' }
    ```

    Then esbuild will now replace all references to `process.cwd` with the `cwdShim` variable, which will also cause `process.cwd()` to return `'/'` (but which will also not mess with other references to `process`).

    In addition to inserting a shim for a global variable that doesn't exist, another use case is replacing references to static methods on global objects with cached versions to both minify them better and to make access to them potentially faster. For example:

    ```js
    // Injected file
    let cachedMin = Math.min
    let cachedMax = Math.max
    export {
      cachedMin as 'Math.min',
      cachedMax as 'Math.max',
    }

    // Original input
    function clampRGB(r, g, b) {
      return {
        r: Math.max(0, Math.min(1, r)),
        g: Math.max(0, Math.min(1, g)),
        b: Math.max(0, Math.min(1, b)),
      }
    }

    // Old output (with --minify)
    function clampRGB(a,t,m){return{r:Math.max(0,Math.min(1,a)),g:Math.max(0,Math.min(1,t)),b:Math.max(0,Math.min(1,m))}}

    // New output (with --minify)
    var a=Math.min,t=Math.max;function clampRGB(h,M,m){return{r:t(0,a(1,h)),g:t(0,a(1,M)),b:t(0,a(1,m))}}
    ```

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
