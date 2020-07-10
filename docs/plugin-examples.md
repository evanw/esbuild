# Example plugins

## Svelte plugin

```js
let svelte = require('svelte/compiler')
let path = require('path')
let util = require('util')
let fs = require('fs')

// import value from './example.svelte'
let sveltePlugin = plugin => {
  plugin.setName('svelte');
  plugin.addLoader({ filter: /\.svelte$/ }, async (args) => {
    let convertMessage = ({ message, start, end }) => ({
      text: message,
      location: start && end && {
        file: filename,
        line: start.line,
        column: start.column,
        length: start.line === end.line ? end.column - start.column : 0,
        lineText: source.split(/\r\n|\r|\n/g)[start.line - 1],
      },
    })
    let source = await util.promisify(fs.readFile)(args.path, 'utf8')
    let filename = path.relative(process.cwd(), args.path)
    try {
      let { js, warnings } = svelte.compile(source, { filename })
      let contents = js.code + `//# sourceMappingURL=` + js.map.toUrl()
      return { contents, warnings: warnings.map(convertMessage) }
    } catch (e) {
      return { errors: [convertMessage(e)] }
    }
  })
}
```

## PnP Yarn plugin

```js
let { PosixFS, ZipOpenFS } = require(`@yarnpkg/fslib`)
let libzip = require(`@yarnpkg/libzip`).getLibzipSync()
let zipOpenFs = new ZipOpenFS({ libzip })
let crossFs = new PosixFS(zipOpenFs)
let pnpapi = require('pnpapi')

// More info: https://classic.yarnpkg.com/en/docs/pnp/
let pnpYarnPlugin = plugin => {
  plugin.setName('pnp')
  plugin.addResolver({ filter: /.*/ }, args => {
    return { path: pnpapi.resolveRequest(args.path, args.resolveDir + '/') }
  })
  plugin.addLoader({ filter: /.*/ }, args => {
    return { contents: crossFs.readFileSync(args.path), loader: 'default' }
  })
}
```

### YAML plugin

```js
let YAML = require('js-yaml')
let util = require('util')
let fs = require('fs')

// import value from './example.yaml'
let yamlLoader = plugin => {
  plugin.setName('yaml-loader')
  plugin.addLoader({ filter: /\.ya?ml$/ }, async (args) => {
    let source = await util.promisify(fs.readFile)(args.path, 'utf8')
    try {
      let contents = JSON.stringify(YAML.safeLoad(source), null, 2)
      return { contents, loader: 'json' }
    } catch (e) {
      return {
        errors: [{
          text: (e && e.reason) || (e && e.message) || e,
          location: e.mark && {
            line: e.mark.line,
            column: e.mark.column,
            lineText: source.split(/\r\n|\r|\n/g)[e.mark.line],
          },
        }],
      }
    }
  })
}
```

### WebAssembly plugin

```js
let path = require('path')
let util = require('util')
let fs = require('fs')

// import load from './example.wasm'
// load(imports).then(exports => { ... })
let wasmPlugin = plugin => {
  plugin.setName('wasm-loader')

  plugin.addResolver({ filter: /\.wasm$/ }, args => ({
    path: path.isAbsolute(args.path) ? args.path : path.join(args.resolveDir, args.path),
    namespace: args.namespace === 'wasm-stub' ? 'wasm-binary' : 'wasm-stub' }))

  plugin.addLoader({ filter: /.*/, namespace: 'wasm-binary' }, async (args) => ({
    contents: await util.promisify(fs.readFile)(args.path), loader: 'binary' }))

  plugin.addLoader({ filter: /.*/, namespace: 'wasm-stub' }, async (args) => ({
    contents: `import wasm from ${JSON.stringify(args.path)}
      export default async (imports) =>
        (await WebAssembly.instantiate(wasm, imports)).instance.exports` }))
}
```

### SVG optimizer plugin

```js
let SVGO = require('svgo')
let util = require('util')
let fs = require('fs')

// import value from './example.svg'
let svgOptimizer = plugin => {
  plugin.setName('svg-optimizer')
  plugin.addLoader({ filter: /\.svg$/ }, async (args) => {
    let source = await util.promisify(fs.readFile)(args.path, 'utf8')
    try {
      let { data: contents } = await new SVGO().optimize(source)
      return { contents, loader: 'text' }
    } catch (e) {
      let match = /^(.*)\nLine: (\d+)\nColumn: (\d+)\nChar: (.*)$/.exec(e + '')
      return {
        errors: [{
          text: match ? match[1] : (e && e.message) || e,
          location: match && {
            line: +match[2],
            column: +match[3] - 1,
            length: match[4].length,
            lineText: source.split(/\r\n|\r|\n/g)[+match[2]],
          },
        }],
      }
    }
  })
}
```

### CoffeeScript plugin

```js
// import value from './example.coffee'
let CoffeeScript = require('coffeescript')
let util = require('util')
let fs = require('fs')

let coffeeLoader = plugin => {
  plugin.setName('coffee-loader')
  plugin.addLoader({ filter: /\.coffee$/ }, async (args) => {
    let source = await util.promisify(fs.readFile)(args.path, 'utf8')
    try {
      let filename = path.relative(process.cwd(), args.path)
      return { contents: CoffeeScript.compile(source, { inlineMap: true, filename }) }
    } catch (e) {
      return {
        errors: [{
          text: (e && e.message) || e,
          location: e.location && {
            line: e.location.first_line,
            column: e.location.first_column,
            length: e.location.last_column - e.location.first_column + 1,
            lineText: source.split(/\r\n|\r|\n/g)[e.location.first_line],
          },
        }],
      }
    }
  })
}
```

## HTTP plugin

```js
// import value from 'https://unpkg.com/lodash-es@4.17.15/lodash.js'
let https = require('https')
let http = require('http')

let httpLoader = plugin => {
  plugin.setName('http')
  plugin.addResolver({ filter: /^https?:\/\// }, args => {
    return { path: args.path, namespace: 'http' }
  })
  plugin.addResolver({ filter: /.*/, namespace: 'http' }, args => {
    return { path: new URL(args.path, args.importer), namespace: 'http' }
  })
  plugin.addLoader({ filter: /^https?:\/\//, namespace: 'http' }, async (args) => {
    let contents = await new Promise((resolve, reject) => {
      let lib = args.path.startsWith('https') ? https : http
      console.log(`Downloading: ${args.path}`)
      lib.get(args.path, res => {
        let chunks = []
        res.on('data', chunk => chunks.push(chunk))
        res.on('end', () => resolve(Buffer.concat(chunks)))
      }).on('error', reject)
    })
    return { contents }
  })
}
```

## Fibonacci plugin

```js
// import value from 'fib(10)'
let fibonacciLoader = plugin => {
  plugin.setName('fibonacci')
  plugin.addResolver({ filter: /^fib\((\d+)\)/ }, args => {
    return { path: args.path, namespace: 'fibonacci' }
  })
  plugin.addLoader({ filter: /^fib\((\d+)\)/, namespace: 'fibonacci' }, args => {
    let match = /^fib\((\d+)\)/.exec(args.path), n = +match[1]
    let contents = n < 2 ? `export default ${n}` : `
      import n1 from 'fib(${n - 1}) ${args.path}'
      import n2 from 'fib(${n - 2}) ${args.path}'
      export default n1 + n2`
    return { contents }
  })
}
```
