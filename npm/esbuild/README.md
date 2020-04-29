# esbuild

This is a JavaScript bundler and minifier. See https://github.com/evanw/esbuild for details.

## JavaScript API

In addition to exposing the `esbuild` command-line tool, this package also exposes a JavaScript API that can be used to invoke the command-line tool from JavaScript. The API is a single function called `build()`.

Example usage:

```js
const esbuild = require('esbuild')
const fs = require('fs')

esbuild.build({
  entryPoints: ['./example.ts'],
  minify: true,
  bundle: true,
  outfile: './example.min.js',
}).then(
  ({ stderr, warnings }) => {
    const output = fs.readFileSync('./example.min.js', 'utf8')
    console.log('success', { output, stderr, warnings })
  },
  ({ stderr, errors, warnings }) => {
    console.error('failure', { stderr, errors, warnings })
  }
)
```

See [the TypeScript type definitions](./lib/main.d.ts) for the complete set of options.
