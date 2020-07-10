const { installForTests } = require('./esbuild')
const rimraf = require('rimraf')
const assert = require('assert')
const path = require('path')
const util = require('util')
const fs = require('fs')

const readFileAsync = util.promisify(fs.readFile)
const writeFileAsync = util.promisify(fs.writeFile)
const mkdirAsync = util.promisify(fs.mkdir)

const repoDir = path.dirname(__dirname)
const rootTestDir = path.join(repoDir, 'scripts', '.plugin-tests')

let pluginTests = {
  async basicLoader({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const custom = path.join(testDir, 'example.custom')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `
      import x from './example.custom'
      export default x
    `)
    await writeFileAsync(custom, ``)
    await esbuild.build({
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [
        plugin => {
          plugin.setName('name')
          plugin.addLoader({ filter: /\.custom$/ }, args => {
            assert.strictEqual(args.path, custom)
            return { contents: 'this is custom', loader: 'text' }
          })
        },
      ],
    })
    const result = require(output)
    assert.strictEqual(result.default, 'this is custom')
  },

  async basicResolver({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const custom = path.join(testDir, 'example.txt')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `
      import x from 'test'
      export default x
    `)
    await writeFileAsync(custom, `example text`)
    await esbuild.build({
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [
        plugin => {
          plugin.setName('name')
          plugin.addResolver({ filter: /^test$/ }, args => {
            assert.strictEqual(args.path, 'test')
            return { path: custom }
          })
        },
      ],
    })
    const result = require(output)
    assert.strictEqual(result.default, 'example text')
  },

  async fibonacciResolverMemoized({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `
      import x from 'fib(10)'
      export default x
    `)
    await esbuild.build({
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [
        plugin => {
          plugin.setName('name')
          plugin.addResolver({ filter: /^fib\((\d+)\)$/ }, args => {
            return { path: args.path, namespace: 'fib' }
          })
          plugin.addLoader({ filter: /^fib\((\d+)\)$/, namespace: 'fib' }, args => {
            let match = /^fib\((\d+)\)$/.exec(args.path), n = +match[1]
            let contents = n < 2 ? `export default ${n}` : `
              import n1 from 'fib(${n - 1})'
              import n2 from 'fib(${n - 2})'
              export default n1 + n2`
            return { contents }
          })
        },
      ],
    })
    const result = require(output)
    assert.strictEqual(result.default, 55)
  },

  async fibonacciResolverNotMemoized({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `
      import x from 'fib(10)'
      export default x
    `)
    await esbuild.build({
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [
        plugin => {
          plugin.setName('name')
          plugin.addResolver({ filter: /^fib\((\d+)\)/ }, args => {
            return { path: args.path, namespace: 'fib' }
          })
          plugin.addLoader({ filter: /^fib\((\d+)\)/, namespace: 'fib' }, args => {
            let match = /^fib\((\d+)\)/.exec(args.path), n = +match[1]
            let contents = n < 2 ? `export default ${n}` : `
              import n1 from 'fib(${n - 1}) ${args.path}'
              import n2 from 'fib(${n - 2}) ${args.path}'
              export default n1 + n2`
            return { contents }
          })
        },
      ],
    })
    const result = require(output)
    assert.strictEqual(result.default, 55)
  },

  async resolversCalledInSequence({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const nested = path.join(testDir, 'nested.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `
      import x from 'test'
      export default x
    `)
    await writeFileAsync(nested, `
      export default 123
    `)
    let trace = []
    await esbuild.build({
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [
        plugin => {
          plugin.setName('plugin1')
          plugin.addResolver({ filter: /^.*$/ }, () => { trace.push('called first') })
        },
        plugin => {
          plugin.setName('plugin2')
          plugin.addResolver({ filter: /^ignore me$/ }, () => { trace.push('not called') })
        },
        plugin => {
          plugin.setName('plugin3')
          plugin.addResolver({ filter: /^.*$/ }, () => {
            trace.push('called second')
            return { path: nested }
          })
        },
        plugin => {
          plugin.setName('plugin4')
          plugin.addResolver({ filter: /^.*$/ }, () => { trace.push('not called') })
        },
      ],
    })
    const result = require(output)
    assert.strictEqual(result.default, 123)
    assert.deepStrictEqual(trace, [
      'called first',
      'called second',
    ])
  },

  async loadersCalledInSequence({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const nested = path.join(testDir, 'nested.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `
      import x from './nested.js'
      export default x
    `)
    await writeFileAsync(nested, `
      export default 123
    `)
    let trace = []
    await esbuild.build({
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [
        plugin => {
          plugin.setName('plugin1')
          plugin.addLoader({ filter: /^.*$/ }, () => { trace.push('called first') })
        },
        plugin => {
          plugin.setName('plugin2')
          plugin.addLoader({ filter: /^.*$/, namespace: 'ignore-me' }, () => { trace.push('not called') })
        },
        plugin => {
          plugin.setName('plugin3')
          plugin.addLoader({ filter: /^.*$/, namespace: 'file' }, () => {
            trace.push('called second')
            return { contents: 'export default "abc"' }
          })
        },
        plugin => {
          plugin.setName('plugin4')
          plugin.addLoader({ filter: /^.*$/, namespace: 'file' }, () => { trace.push('not called') })
        },
      ],
    })
    const result = require(output)
    assert.strictEqual(result.default, 'abc')
    assert.deepStrictEqual(trace, [
      'called first',
      'called second',
    ])
  },

  async httpRelative({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `
      import x from 'http://example.com/assets/js/example.js'
      export default x
    `)
    await esbuild.build({
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [
        plugin => {
          plugin.setName('name')
          plugin.addResolver({ filter: /^http:\/\// }, args => {
            return { path: args.path, namespace: 'http' }
          })
          plugin.addResolver({ filter: /.*/, namespace: 'http' }, args => {
            return { path: new URL(args.path, args.importer).toString(), namespace: 'http' }
          })
          plugin.addLoader({ filter: /^http:\/\//, namespace: 'http' }, args => {
            switch (args.path) {
              case 'http://example.com/assets/js/example.js':
                return { contents: `import y from './data/base.js'; export default y` }
              case 'http://example.com/assets/js/data/base.js':
                return { contents: `export default 123` }
            }
          })
        },
      ],
    })
    const result = require(output)
    assert.strictEqual(result.default, 123)
  },

  async rewriteExternal({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `
      import {exists} from 'extern'
      export default exists
    `)
    await esbuild.build({
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [
        plugin => {
          plugin.setName('name')
          plugin.addResolver({ filter: /^extern$/ }, () => {
            return { path: 'fs', external: true, namespace: 'for-testing' }
          })
        },
      ],
    })
    const result = require(output)
    assert.strictEqual(result.default, fs.exists)
  },

  async resolveDirInFileModule({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    const example = path.join(testDir, 'example.custom')
    const resolveDir = path.join(testDir, 'target')
    const loadme = path.join(resolveDir, 'loadme.js')
    await mkdirAsync(resolveDir)
    await writeFileAsync(input, `
      import value from './example.custom'
      export default value
    `)
    await writeFileAsync(example, `
      export {default} from './loadme'
    `)
    await writeFileAsync(loadme, `
      export default 123
    `)
    await esbuild.build({
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [
        plugin => {
          plugin.setName('name')
          plugin.addLoader({ filter: /\.custom$/ }, async (args) => {
            return { contents: await readFileAsync(args.path), resolveDir }
          })
        },
      ],
    })
    const result = require(output)
    assert.strictEqual(result.default, 123)
  },

  async noResolveDirInFileModule({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    const example = path.join(testDir, 'example.custom')
    const resolveDir = path.join(testDir, 'target')
    const loadme = path.join(resolveDir, 'loadme.js')
    await mkdirAsync(resolveDir)
    await writeFileAsync(input, `
      import value from './example.custom'
      export default value
    `)
    await writeFileAsync(example, `
      export {default} from './target/loadme'
    `)
    await writeFileAsync(loadme, `
      export default 123
    `)
    await esbuild.build({
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [
        plugin => {
          plugin.setName('name')
          plugin.addLoader({ filter: /\.custom$/ }, async (args) => {
            return { contents: await readFileAsync(args.path) }
          })
        },
      ],
    })
    const result = require(output)
    assert.strictEqual(result.default, 123)
  },

  async resolveDirInVirtualModule({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    const resolveDir = path.join(testDir, 'target')
    const loadme = path.join(resolveDir, 'loadme.js')
    await mkdirAsync(resolveDir)
    await writeFileAsync(input, `
      import value from 'virtual'
      export default value
    `)
    await writeFileAsync(loadme, `
      export default 123
    `)
    await esbuild.build({
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [
        plugin => {
          let contents = `export {default} from './loadme'`
          plugin.setName('name')
          plugin.addResolver({ filter: /^virtual$/ }, () => ({ path: 'virtual', namespace: 'for-testing' }))
          plugin.addLoader({ filter: /.*/, namespace: 'for-testing' }, () => ({ contents, resolveDir }))
        },
      ],
    })
    const result = require(output)
    assert.strictEqual(result.default, 123)
  },

  async noResolveDirInVirtualModule({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    const resolveDir = path.join(testDir, 'target')
    const loadme = path.join(resolveDir, 'loadme.js')
    await mkdirAsync(resolveDir)
    await writeFileAsync(input, `
      import value from 'virtual'
      export default value
    `)
    await writeFileAsync(loadme, `
      export default 123
    `)
    let error
    try {
      await esbuild.build({
        entryPoints: [input], bundle: true, outfile: output, format: 'cjs', logLevel: 'silent', plugins: [
          plugin => {
            let contents = `export {default} from './loadme'`
            plugin.setName('name')
            plugin.addResolver({ filter: /^virtual$/ }, () => ({ path: 'virtual', namespace: 'for-testing' }))
            plugin.addLoader({ filter: /.*/, namespace: 'for-testing' }, () => ({ contents }))
          },
        ],
      })
    } catch (e) {
      error = e
    }
    assert.notStrictEqual(error, void 0)
    assert.strictEqual(error.errors.length, 1)
    assert.strictEqual(error.errors[0].text, `Could not resolve "./loadme" (the "name" plugin didn't set a resolve directory)`)
  },

  async webAssembly({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const wasm = path.join(testDir, 'test.wasm')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `
      import load from './test.wasm'
      export default async (x, y) => (await load()).add(x, y)
    `)
    await writeFileAsync(wasm, Buffer.of(
      // #[wasm_bindgen]
      // pub fn add(x: i32, y: i32) -> i32 { x + y }
      0x00, 0x61, 0x73, 0x6D, 0x01, 0x00, 0x00, 0x00, 0x01, 0x07, 0x01, 0x60,
      0x02, 0x7F, 0x7F, 0x01, 0x7F, 0x03, 0x02, 0x01, 0x00, 0x05, 0x03, 0x01,
      0x00, 0x11, 0x07, 0x10, 0x02, 0x06, 0x6D, 0x65, 0x6D, 0x6F, 0x72, 0x79,
      0x02, 0x00, 0x03, 0x61, 0x64, 0x64, 0x00, 0x00, 0x0A, 0x09, 0x01, 0x07,
      0x00, 0x20, 0x00, 0x20, 0x01, 0x6A, 0x0B,
    ))
    await esbuild.build({
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [
        plugin => {
          plugin.setName('name')
          plugin.addResolver({ filter: /\.wasm$/ }, args => ({
            path: path.isAbsolute(args.path) ? args.path : path.join(args.resolveDir, args.path),
            namespace: args.namespace === 'wasm-stub' ? 'wasm-binary' : 'wasm-stub',
          }))
          plugin.addLoader({ filter: /.*/, namespace: 'wasm-binary' }, async (args) =>
            ({ contents: await readFileAsync(args.path), loader: 'binary' }))
          plugin.addLoader({ filter: /.*/, namespace: 'wasm-stub' }, async (args) => ({
            contents: `import wasm from ${JSON.stringify(args.path)}
              export default async (imports) =>
                (await WebAssembly.instantiate(wasm, imports)).instance.exports` }))
        },
      ],
    })
    const result = require(output)
    assert.strictEqual(await result.default(103, 20), 123)
  },

  async stdinRelative({ esbuild, testDir }) {
    const output = path.join(testDir, 'out.js')
    await esbuild.build({
      stdin: { contents: `import x from "./stdinRelative.js"; export default x` },
      bundle: true, outfile: output, format: 'cjs', plugins: [
        plugin => {
          plugin.setName('name')
          plugin.addResolver({ filter: /.*/ }, args => {
            assert.strictEqual(args.namespace, '')
            assert.strictEqual(args.importer, '<stdin>')
            assert.strictEqual(args.resolveDir, '')
            assert.strictEqual(args.path, './stdinRelative.js')
            return { path: args.path, namespace: 'worked' }
          })
          plugin.addLoader({ filter: /.*/, namespace: 'worked' }, () => {
            return { contents: `export default 123` }
          })
        },
      ],
    })
    const result = require(output)
    assert.strictEqual(result.default, 123)
  },

  async stdinRelativeResolveDir({ esbuild, testDir }) {
    const output = path.join(testDir, 'out', 'out.js')
    await esbuild.build({
      stdin: {
        contents: `import x from "./stdinRelative.js"; export default x`,
        resolveDir: testDir,
      },
      bundle: true, outfile: output, format: 'cjs', plugins: [
        plugin => {
          plugin.setName('name')
          plugin.addResolver({ filter: /.*/ }, args => {
            assert.strictEqual(args.namespace, '')
            assert.strictEqual(args.importer, '<stdin>')
            assert.strictEqual(args.resolveDir, testDir)
            assert.strictEqual(args.path, './stdinRelative.js')
            return { path: args.path, namespace: 'worked' }
          })
          plugin.addLoader({ filter: /.*/, namespace: 'worked' }, () => {
            return { contents: `export default 123` }
          })
        },
      ],
    })
    const result = require(output)
    assert.strictEqual(result.default, 123)
  },
}

async function main() {
  // Start the esbuild service
  const esbuild = installForTests(rootTestDir)
  const service = await esbuild.startService()

  // Run all tests concurrently
  const runTest = async ([name, fn]) => {
    let testDir = path.join(rootTestDir, name)
    try {
      await mkdirAsync(testDir)
      await fn({ esbuild, service, testDir })
      rimraf.sync(testDir, { disableGlob: true })
      return true
    } catch (e) {
      console.error(`❌ ${name}: ${e && e.message || e}`)
      return false
    }
  }
  const tests = Object.entries(pluginTests)
  const allTestsPassed = (await Promise.all(tests.map(runTest))).every(success => success)

  // Clean up test output
  service.stop()

  if (!allTestsPassed) {
    console.error(`❌ plugin tests failed`)
    process.exit(1)
  } else {
    console.log(`✅ plugin tests passed`)
    rimraf.sync(rootTestDir, { disableGlob: true })
  }
}

main().catch(e => setTimeout(() => { throw e }))
