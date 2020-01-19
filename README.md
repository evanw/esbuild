# esbuild

This is a JavaScript bundler and minifier. It packages up JavaScript code for distribution on the web.

## Why?

Why build another JavaScript build tool? The current build tools for the web are at least an order of magnitude slower than they should be. I'm hoping that this project serves as an "existence proof" that our JavaScript tooling can be much, much faster.

## Benchmarks

The use case I have in mind is packaging a large codebase for production. This includes minifying the code, which reduces network transfer time, and producing source maps, which are important for debugging errors in production. Ideally the build tool should also build quickly without having to warm up a cache first.

My main benchmark approximates a large codebase by duplicating the [three.js](https://github.com/mrdoob/three.js) library 10 times and building a single bundle from scratch, without any caches. For this benchmark, esbuild is **10-100x faster** than the other JavaScript bundlers I tested (webpack, rollup, and parcel). The benchmark can be run with `make bench-three`.

![](images/benchmark.png)

| Bundler         |    Time | Relative slowdown | Absolute speed | Output size |
| :-------------- | ------: | ----------------: | -------------: | ----------: |
| esbuild         |   0.58s |                1x |    1028 kloc/s |      5.83mb |
| rollup + terser |  43.56s |               75x |      14 kloc/s |      5.96mb |
| webpack         |  47.85s |               83x |      12 kloc/s |      5.82mb |
| parcel          | 120.45s |              208x |       5 kloc/s |      5.90mb |

I'm running esbuild with `--bundle --minify --sourcemap`. I used the `rollup-plugin-terser` plugin because rollup itself doesn't support minification. Webpack uses `--mode=production --devtool=sourcemap`. Parcel uses the default options. The tests were done on a 6-core 2019 MacBook Pro with 16gb of RAM.

## Why is it fast?

Several reasons:

* It's written in Go, a language that compiles to native code
* Parsing, printing, and source map generation are all fully parallelized
* Everything is done in very few passes without expensive data transformations
* Code is written with speed in mind, and tries to avoid unnecessary allocations

## Status

Currently supported:

* CommonJS modules
* ES6 modules
* Bundling with static binding of ES6 modules using `--bundle`
* Full minification with `--minify` (whitespace, identifiers, and mangling)
* Full source map support when `--sourcemap` is enabled
* JSX-to-JavaScript conversion for `.jsx` files
* Compile-time identifier substitutions via `--define`
* Path substitution using the `browser` field in `package.json`
* Automatic detection of `baseUrl` in `tsconfig.json`

This is a hobby project that I wrote over the 2019-2020 winter break. I believe that it's relatively complete and functional. However, it's brand new code and probably has a lot of bugs. It also hasn't yet been used in production by anyone. Use at your own risk.

Also keep in mind that this doesn't currently do any lowering to e.g. ES6 or ES5. All syntax is passed through unmodified so the latest JavaScript features won't work in older browsers. This is easy to add but I haven't gotten to it yet.

I don't personally want to run a large open source project, so I'm not looking for contributions at this time.

## Install

The executable can be built using `make`, assuming you have the Go language toolchain installed. Prebuilt binaries are currently available on npm under separate packages:

```
npm install -g esbuild-linux-64   # for Linux
npm install -g esbuild-darwin-64  # for macOS
npm install -g esbuild-windows-64 # for Windows
npm install -g esbuild-wasm       # for all other platforms
```

This adds a command called `esbuild`.

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

  --minify              Sets all --minify-* flags
  --minify-whitespace   Remove whitespace
  --minify-identifiers  Shorten identifiers
  --minify-syntax       Use equivalent but shorter syntax

  --define:K=V          Substitute K with V while parsing
  --jsx-factory=...     What to use instead of React.createElement
  --jsx-fragment=...    What to use instead of React.Fragment

  --trace=...           Write a CPU trace to this file
  --cpuprofile=...      Write a CPU profile to this file

Example:
  # Produces dist/entry_point.js and dist/entry_point.js.map
  esbuild --bundle entry_point.js --outdir=dist --minify --sourcemap
```
