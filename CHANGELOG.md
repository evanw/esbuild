# Changelog

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
