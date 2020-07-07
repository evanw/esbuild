# JavaScript API Documentation

There are two JavaScript APIs for esbuild, [one for node](#node-api) and [one for the browser](#browser-api). These APIs make it possible to integrate esbuild with other tools in the JavaScript ecosystem. Both APIs work by running the `esbuild` Go executable and communicating with it over stdin and stdout, although all of these details are hidden behind the API.

## Node API

Install: `npm install esbuild`

### Running a build

The `build()` and `buildSync()` APIs are the same as invoking the command-line tool. It reads from files on disk and writes back to files on disk. Using this API can be more convenient than managing a lot of command-line flags and also works on all platforms, unlike shell scripts. This is similar to "config files" from other bundlers.

Example build script:

```js
const { build } = require('esbuild')

build({
  entryPoints: ['./src/main.ts'],
  outfile: './dist/main.js',
  minify: true,
  bundle: true,
}).catch(() => process.exit(1))
```

See [the TypeScript type definitions](../lib/api-types.ts) for the complete set of options.

### Transforming a file

The `transform()` and `transformSync()` APIs transform a single file in memory. It can be used to minify JavaScript, convert TypeScript/JSX to JavaScript, or convert newer JavaScript to older JavaScript. It's roughly equivalent to running `build()` on a single file with `bundle: false`.

Example usage:

```js
const jsx = `
  import * as React from 'react'
  import * as ReactDOM from 'react-dom'

  ReactDOM.render(
    <h1>Hello, world!</h1>,
    document.getElementById('root')
  );
`

const { transformSync } = require('esbuild')
const { js } = transformSync(jsx, { loader: 'jsx' })
process.stdout.write(js)
```

See [the TypeScript type definitions](../lib/api-types.ts) for the complete set of options.

### Use a service for optimal performance

The functions described above don't have optimal performance because every call creates a new child process, which adds extra overhead. If you need to make repeated calls to esbuild's API you'll probably want to use the service API, which creates a single long-lived child process and shares it between all API calls.

Example usage:

```js
(async () => {
  // Start the esbuild child process once
  const { startService } = require('esbuild')
  const service = await startService()

  try {
    // Call transform() many times without the overhead of starting a service
    const [a, b, c] = await Promise.all([
      service.transform('2 ** 3', { target: 'es2015' }),
      service.transform('<div></div>', { loader: 'jsx' }),
      service.transform('enum Foo { A, B }', { loader: 'ts' }),
    ])
    console.log([a.js, b.js, c.js])
  }

  finally {
    // The child process can be explicitly killed when it's no longer needed
    service.stop()
  }
})()
```

## Browser API

Install: `npm install esbuild-wasm`

This API provides a way to run the `transform()` function from the [node API](#node-api) in the browser. This runs the `esbuild` binary in a web worker using WebAssembly. It's mostly the same as the node API except that you are required to start a service and the `build()` API call is not available. You also need to provide the URL of the WebAssembly file when creating the service.

Example usage:

```ts
(async () => {
  // Start the esbuild web worker once
  const { startService } = require('esbuild-wasm/lib/browser')
  const wasmURL = 'node_modules/esbuild-wasm/esbuild.wasm'
  const service = await startService({ wasmURL })

  try {
    // Call transform() many times without the overhead of starting a service
    const [a, b, c] = await Promise.all([
      service.transform('2 ** 3', { target: 'es2015' }),
      service.transform('<div></div>', { loader: 'jsx' }),
      service.transform('enum Foo { A, B }', { loader: 'ts' }),
    ])
    console.log([a.js, b.js, c.js])
  }

  finally {
    // The web worker can be explicitly killed when it's no longer needed
    service.stop()
  }
})()
```

You can also use this API as a script tag in a HTML file. In this case the API creates a global called `esbuild` with a `startService` property:

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

If you're already running this code from a worker and don't want `startService()` to create another worker, you can pass `startService({ wasmURL, worker: false })`. Then it will create a WebAssembly module in the same thread as the thread that calls `startService()`.

The `esbuild-wasm` package can also be used from node in addition to being used from the browser. It exposes the exact same [node API](#node-api) as the `esbuild` package. To use it, just substitute `require('esbuild')` with `require('esbuild-wasm')` in the examples for the node API. However, you should always prefer the `esbuild` package if at all possible since it offers substantially better performance (in some cases it's around 10x faster). The node API in the `esbuild-wasm` package is only for compatibility with obscure platforms that the `esbuild` package doesn't have a native executable for.
