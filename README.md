# esbuild

This is a JavaScript bundler and minifier. It packages up JavaScript and TypeScript code for distribution on the web.

## Why?

Why build another JavaScript build tool? The current build tools for the web are at least an order of magnitude slower than they should be. I'm hoping that this project serves as an "existence proof" that our JavaScript tooling can be much, much faster.

## Benchmarks

The use case I have in mind is packaging a large codebase for production. This includes minifying the code, which reduces network transfer time, and producing source maps, which are important for debugging errors in production. Ideally the build tool should also build quickly without having to warm up a cache first.

I currently have two benchmarks that I'm using to measure esbuild performance. For these benchmarks, esbuild is **10-100x faster** than the other JavaScript bundlers I tested:

![](images/benchmark.png)

Here are the details about each benchmark:

* #### JavaScript benchmark

  This benchmark approximates a large JavaScript codebase by duplicating the [three.js](https://github.com/mrdoob/three.js) library 10 times and building a single bundle from scratch, without any caches. The benchmark can be run with `make bench-three`.

    | Bundler         |    Time | Relative slowdown | Absolute speed | Output size |
    | :-------------- | ------: | ----------------: | -------------: | ----------: |
    | esbuild         |   0.54s |                1x |  1013.8 kloc/s |      5.83mb |
    | rollup + terser |  40.48s |               75x |    13.5 kloc/s |      5.80mb |
    | webpack         |  46.46s |               86x |    11.8 kloc/s |      5.97mb |
    | parcel          | 124.65s |              231x |     4.4 kloc/s |      5.90mb |
    | fuse-box@next   | 172.56s |              320x |     3.2 kloc/s |      6.55mb |

    Each time reported is the best of three runs. I'm running esbuild with `--bundle --minify --sourcemap`. I used the `rollup-plugin-terser` plugin because Rollup itself doesn't support minification. Webpack uses `--mode=production --devtool=sourcemap`. Parcel uses the default options. FuseBox is configured with `useSingleBundle: true`. Absolute speed is based on the total line count including comments and blank lines, which is currently 547,441. The tests were done on a 6-core 2019 MacBook Pro with 16gb of RAM.

* #### TypeScript benchmark

  This benchmark uses the [Rome](https://github.com/facebookexperimental/rome) build tool to approximate a large TypeScript codebase. All code must be combined into a single minified bundle with source maps and the resulting bundle must work correctly. The benchmark can be run with `make bench-rome`.

    | Bundler         |    Time | Relative slowdown | Absolute speed | Output size |
    | :-------------- | ------: | ----------------: | -------------: | ----------: |
    | esbuild         |   0.13s |                1x |  1014.1 kloc/s |      0.99mb |
    | parcel          |  15.89s |              122x |     8.3 kloc/s |      1.50mb |
    | webpack         |  18.37s |              141x |     7.2 kloc/s |      1.22mb |

    Each time reported is the best of three runs. I'm running esbuild with `--bundle --minify --sourcemap --platform=node`. Webpack uses `ts-loader` with `transpileOnly: true` and `--mode=production --devtool=sourcemap`. Parcel uses `--target node --bundle-node-modules`. Absolute speed is based on the total line count including comments and blank lines, which is currently 131,836. The tests were done on a 6-core 2019 MacBook Pro with 16gb of RAM.

    The results don't include Rollup because I couldn't get it to work. I tried `rollup-plugin-typescript`, `@rollup/plugin-typescript`, and `@rollup/plugin-sucrase` and they all didn't work for different reasons relating to TypeScript compilation. And I'm not familiar with FuseBox so I'm not sure how work around build failures due to the use of builtin node modules.

## Why is it fast?

Several reasons:

* It's written in Go, a language that compiles to native code
* Parsing, printing, and source map generation are all fully parallelized
* Everything is done in very few passes without expensive data transformations
* Code is written with speed in mind, and tries to avoid unnecessary allocations

## Status

Currently supported:

* JavaScript and TypeScript syntax
* CommonJS and ES6 modules
* JSX-to-JavaScript conversion
* Bundling with static binding of ES6 modules using `--bundle`
* Full minification with `--minify` (whitespace, identifiers, and mangling)
* Full source map support when `--sourcemap` is enabled
* Compile-time identifier substitutions via `--define`
* Path substitution using the `browser` field in `package.json`
* Automatic detection of `baseUrl` in `tsconfig.json`
* Conversion of some newer syntax for older browsers (all syntax in [ES2019 and later](https://github.com/tc39/proposals/blob/master/finished-proposals.md) except for BigInt)

Disclaimers:

* This is a hobby project. I believe that it's relatively complete and functional. However, it's brand new code and you might hit some bugs.
* This hasn't yet been used in production by anyone. Use at your own risk.
* I don't personally want to run a large community project, so I'm not looking for contributions at this time.
* The code in this repo isn't intended to be built upon. I'm may change the internals in a backwards-incompatible way at any time to improve performance or introduce new features.

## Install

A prebuilt binary can be installed using npm:

* **Local install (recommended)**

    This installs the `esbuild` command locally in your project's `package.json` file:

    ```
    npm install --save-dev esbuild
    ```

    Invoke it using `npx esbuild [arguments]`. Note that this uses the `npx` package runner command, not the `npm` package manager command.

    This is the recommended project-based workflow because it allows you to have a different version of `esbuild` for each project and it ensures that everyone working on a given project has the same version of `esbuild`.

* **Global install**

    This adds a global command called `esbuild` to your path:

    ```
    npm install --global esbuild
    ```

    Invoke it using `esbuild [arguments]`.

    A global install can be handy if you want to run `esbuild` outside of a project context for one-off file manipulation tasks.

The `esbuild` package should work on 64-bit macOS, Linux, and Windows systems. It contains an install script that downloads the appropriate package for the current platform. If the install script isn't working or you need to run esbuild on an unsupported platform, there is a fallback WebAssembly package called [esbuild-wasm](https://npmjs.com/package/esbuild-wasm) that should work on all platforms.

For development, the executable can be built by running `make` (assuming you have the Go language toolchain installed).

## Usage

The command-line interface takes a list of entry points and produces one bundle file per entry point. Here are the available options:

```
Usage:
  esbuild [options] [entry points]

Options:
  --name=...            The name of the module
  --bundle              Bundle all dependencies into the output files
  --outfile=...         The output file (for one entry point)
  --outdir=...          The output directory (for multiple entry points)
  --sourcemap           Emit a source map
  --error-limit=...     Maximum error count or 0 to disable (default 10)
  --target=...          Language target (default esnext)
  --platform=...        Platform target (browser or node, default browser)
  --external:M          Exclude module M from the bundle
  --loader:X=L          Use loader L to load file extension X, where L is
                        one of: js, jsx, ts, tsx, json, text, base64

  --minify              Sets all --minify-* flags
  --minify-whitespace   Remove whitespace
  --minify-identifiers  Shorten identifiers
  --minify-syntax       Use equivalent but shorter syntax

  --define:K=V          Substitute K with V while parsing
  --jsx-factory=...     What to use instead of React.createElement
  --jsx-fragment=...    What to use instead of React.Fragment

  --trace=...           Write a CPU trace to this file
  --cpuprofile=...      Write a CPU profile to this file
  --version             Print the current version and exit

Examples:
  # Produces dist/entry_point.js and dist/entry_point.js.map
  esbuild --bundle entry_point.js --outdir=dist --minify --sourcemap

  # Allow JSX syntax in .js files
  esbuild --bundle entry_point.js --outfile=out.js --loader:.js=jsx

  # Substitute the identifier RELEASE for the literal true
  esbuild example.js --outfile=out.js --define:RELEASE=true
```

## Using with React

To use esbuild with [React](https://reactjs.org/):

* Either put all JSX syntax in `.jsx` files instead of `.js` files, or use `--loader:.js=jsx` to use the JSX loader for `.js` files.

* If you're using TypeScript, pass esbuild your `.tsx` file as the entry point. There should be no need to convert TypeScript files to JavaScript first because because esbuild parses TypeScript syntax itself.

    *Note that esbuild does not do any type checking, so you'll want to run `tsc -noEmit` in parallel to check types.*

* If you're using esbuild to bundle React yourself instead of including it with a `<script>` tag in your HTML, you'll need to pass `'--define:process.env.NODE_ENV="development"'` or `'--define:process.env.NODE_ENV="production"'` to esbuild on the command line.

* If you're using [Preact](https://preactjs.com/) instead of React, you'll also need to pass `--jsx-factory=preact.h --jsx-fragment=preact.Fragment` to esbuild on the command line.

For example, if you have a file called `example.tsx` with the following contents:

```js
import * as React from 'react'
import * as ReactDOM from 'react-dom'

ReactDOM.render(
  <h1>Hello, world!</h1>,
  document.getElementById('root')
);
```

Use this for a development build:

```
esbuild example.tsx --bundle '--define:process.env.NODE_ENV="development"' --outfile=out.js
```

Use this for a production build:

```
esbuild example.tsx --bundle '--define:process.env.NODE_ENV="production"' --minify --outfile=out.js
```
