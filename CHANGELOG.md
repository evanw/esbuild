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
