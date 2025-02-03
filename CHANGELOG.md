# Changelog

## Unreleased

* Delete output files when a build fails in watch mode ([#3643](https://github.com/evanw/esbuild/issues/3643))

    It has been requested for esbuild to delete files when a build fails in watch mode. Previously esbuild left the old files in place, which could cause people to not immediately realize that the most recent build failed. With this release, esbuild will now delete all output files if a rebuild fails. Fixing the build error and triggering another rebuild will restore all output files again.

* Fix incorrect package for `@esbuild/netbsd-arm64` ([#4018](https://github.com/evanw/esbuild/issues/4018))

    Due to a copy+paste typo, the binary published to `@esbuild/netbsd-arm64` was not actually for `arm64`, and didn't run in that environment. This release should fix running esbuild in that environment (NetBSD on 64-bit ARM). Sorry about the mistake.

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

* The `text` loader now strips the UTF-8 BOM if present ([#3935](https://github.com/evanw/esbuild/issues/3935))

    Some software (such as Notepad on Windows) can create text files that start with the three bytes `0xEF 0xBB 0xBF`, which is referred to as the "byte order mark". This prefix is intended to be removed before using the text. Previously esbuild's `text` loader included this byte sequence in the string, which turns into a prefix of `\uFEFF` in a JavaScript string when decoded from UTF-8. With this release, esbuild's `text` loader will now remove these bytes when they occur at the start of the file.

* Omit legal comment output files when empty ([#3670](https://github.com/evanw/esbuild/issues/3670))

    Previously configuring esbuild with `--legal-comment=external` or `--legal-comment=linked` would always generate a `.LEGAL.txt` output file even if it was empty. Starting with this release, esbuild will now only do this if the file will be non-empty. This should result in a more organized output directory in some cases.

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
