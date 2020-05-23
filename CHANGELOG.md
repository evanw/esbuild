# Changelog

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
