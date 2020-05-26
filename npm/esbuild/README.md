# esbuild

This is a JavaScript bundler and minifier. See https://github.com/evanw/esbuild for details.

## JavaScript API usage

In addition to exposing the `esbuild` command-line tool, this package also exposes a JavaScript API that can be used to invoke the command-line tool from JavaScript.

### Running a build

The `build()` API is the same as invoking the command-line tool. It reads from files on disk and writes back to files on disk. Using this API can be more convenient than managing a lot of command-line flags and also works on all platforms, unlike shell scripts. This is similar to "config files" from other bundlers.

Example build script:

```js
const { build } = require('esbuild')

build({
  stdio: 'inherit',
  entryPoints: ['./src/main.ts'],
  outfile: './dist/main.js',
  minify: true,
  bundle: true,
}).catch(() => process.exit(1))
```

See [the TypeScript type definitions](https://github.com/evanw/esbuild/blob/master/npm/esbuild/lib/main.d.ts) for the complete set of options.

### Transforming a file

The `transform()` API transforms a single file in memory. It can be used to minify JavaScript, convert TypeScript/JSX to JavaScript, or convert newer JavaScript to older JavaScript. It's roughly equivalent to running `build()` on a single file with `bundle: false`.

To access this API you need to start a service, which is a long-lived `esbuild` child process that is then reused. You can use the service to transform many files without the overhead of starting up a new child process each time.

Example usage:

```js
(async () => {
  const jsx = `
    import * as React from 'react'
    import * as ReactDOM from 'react-dom'

    ReactDOM.render(
      <h1>Hello, world!</h1>,
      document.getElementById('root')
    );
  `

  // Start the esbuild child process once
  const esbuild = require('esbuild')
  const service = await esbuild.startService()

  // This can be called many times without the overhead of starting a service
  const { js } = await service.transform(jsx, { loader: 'jsx' })
  console.log(js)

  // The child process can be explicitly killed when it's no longer needed
  service.stop()
})()
```

See [the TypeScript type definitions](https://github.com/evanw/esbuild/blob/master/npm/esbuild/lib/main.d.ts) for the complete set of options.
