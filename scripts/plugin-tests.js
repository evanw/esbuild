const { installForTests, removeRecursiveSync } = require('./esbuild')
const assert = require('assert')
const path = require('path')
const util = require('util')
const url = require('url')
const fs = require('fs')

const readFileAsync = util.promisify(fs.readFile)
const writeFileAsync = util.promisify(fs.writeFile)
const mkdirAsync = util.promisify(fs.mkdir)

const repoDir = path.dirname(__dirname)
const rootTestDir = path.join(repoDir, 'scripts', '.plugin-tests')

let pluginTests = {
  async noPluginsWithBuildSync({ esbuild }) {
    try {
      esbuild.buildSync({
        entryPoints: [], logLevel: 'silent', plugins: [{
          name: 'name',
          setup() { },
        }],
      })
      throw new Error('Expected an error to be thrown')
    } catch (e) {
      assert.strictEqual(e.message, 'Cannot use plugins in synchronous API calls')
    }
  },

  async emptyArray({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `export default 123`)
    await esbuild.build({
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [],
    })
    const result = require(output)
    assert.strictEqual(result.default, 123)
  },

  async emptyArrayWithBuildSync({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `export default 123`)
    esbuild.buildSync({
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [],
    })
    const result = require(output)
    assert.strictEqual(result.default, 123)
  },

  async invalidRegExp({ esbuild }) {
    for (const filter of [/x(?=y)/, /x(?!y)/, /x(?<=y)/, /x(?<!y)/, /(x)\1/]) {
      // onResolve
      try {
        await esbuild.build({
          entryPoints: ['invalid.js'], write: false, plugins: [{
            name: 'name',
            setup(build) {
              build.onResolve({ filter }, () => { })
            },
          }],
        })
        throw new Error(`Expected filter ${filter} to fail`)
      } catch (e) {
        assert.strictEqual(e.message, `[name] "onResolve" filter is not a valid Go regular expression: ${JSON.stringify(filter.source)}`)
      }

      // onLoad
      try {
        await esbuild.build({
          entryPoints: ['invalid.js'], write: false, plugins: [{
            name: 'name',
            setup(build) {
              build.onLoad({ filter }, () => { })
            },
          }],
        })
        throw new Error(`Expected filter ${filter} to fail`)
      } catch (e) {
        assert.strictEqual(e.message, `[name] "onLoad" filter is not a valid Go regular expression: ${JSON.stringify(filter.source)}`)
      }
    }
  },

  async pluginMissingName({ esbuild }) {
    try {
      await esbuild.build({
        entryPoints: [],
        logLevel: 'silent',
        plugins: [{
          setup(build) {
          },
        }],
      })
    } catch (e) {
      assert.strictEqual(e.message, 'Plugin at index 0 is missing a name')
    }
  },

  async pluginMissingSetup({ esbuild }) {
    try {
      await esbuild.build({
        entryPoints: [],
        logLevel: 'silent',
        plugins: [{
          name: 'x',
        }],
      })
    } catch (e) {
      assert.strictEqual(e.message, '[x] Plugin is missing a setup function')
    }
  },

  async badPluginProperty({ esbuild }) {
    try {
      await esbuild.build({
        entryPoints: [],
        logLevel: 'silent',
        plugins: [{
          name: 'x',
          someRandomProperty: void 0,
          setup(build) {
          },
        }],
      })
    } catch (e) {
      assert.strictEqual(e.message, `Invalid option on plugin "x": "someRandomProperty"`)
    }
  },

  async badPluginOnResolveProperty({ esbuild }) {
    try {
      await esbuild.build({
        entryPoints: ['entry'],
        logLevel: 'silent',
        plugins: [{
          name: 'x',
          setup(build) {
            build.onResolve({ whatIsThis: void 0 }, () => {
            })
          },
        }],
      })
    } catch (e) {
      assert.strictEqual(e.message, `Invalid option in onResolve() call for plugin "x": "whatIsThis"`)
    }

    try {
      await esbuild.build({
        entryPoints: ['entry'],
        logLevel: 'silent',
        write: false,
        plugins: [{
          name: 'x',
          setup(build) {
            build.onResolve({ filter: /.*/ }, () => {
              return '/'
            })
          },
        }],
      })
    } catch (e) {
      assert(e.message.endsWith('error: [x] Expected onResolve() callback in plugin "x" to return an object'), e.message)
    }

    try {
      await esbuild.build({
        entryPoints: ['entry'],
        logLevel: 'silent',
        write: false,
        plugins: [{
          name: 'x',
          setup(build) {
            build.onResolve({ filter: /.*/ }, () => {
              return { thisIsWrong: void 0 }
            })
          },
        }],
      })
    } catch (e) {
      assert(e.message.endsWith('error: [x] Invalid option from onResolve() callback in plugin "x": "thisIsWrong"'), e.message)
    }
  },

  async badPluginOnLoadProperty({ esbuild }) {
    try {
      await esbuild.build({
        entryPoints: ['entry'],
        logLevel: 'silent',
        plugins: [{
          name: 'x',
          setup(build) {
            build.onLoad({ whatIsThis: void 0 }, () => {
            })
          },
        }],
      })
    } catch (e) {
      assert.strictEqual(e.message, `Invalid option in onLoad() call for plugin "x": "whatIsThis"`)
    }

    try {
      await esbuild.build({
        entryPoints: ['entry'],
        logLevel: 'silent',
        write: false,
        plugins: [{
          name: 'x',
          setup(build) {
            build.onResolve({ filter: /.*/ }, () => {
              return { path: 'y', namespace: 'z' }
            })
            build.onLoad({ filter: /.*/ }, () => {
              return ""
            })
          },
        }],
      })
    } catch (e) {
      assert(e.message.endsWith(`error: [x] Expected onLoad() callback in plugin "x" to return an object`), e.message)
    }

    try {
      await esbuild.build({
        entryPoints: ['entry'],
        logLevel: 'silent',
        write: false,
        plugins: [{
          name: 'x',
          setup(build) {
            build.onResolve({ filter: /.*/ }, () => {
              return { path: 'y', namespace: 'z' }
            })
            build.onLoad({ filter: /.*/ }, () => {
              return { thisIsWrong: void 0 }
            })
          },
        }],
      })
    } catch (e) {
      assert(e.message.endsWith('error: [x] Invalid option from onLoad() callback in plugin "x": "thisIsWrong"'), e.message)
    }
  },

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
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [{
        name: 'name',
        setup(build) {
          build.onLoad({ filter: /\.custom$/ }, args => {
            assert.strictEqual(args.path, custom)
            return { contents: 'this is custom', loader: 'text' }
          })
        },
      }],
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
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [{
        name: 'name',
        setup(build) {
          build.onResolve({ filter: /^test$/ }, args => {
            assert.strictEqual(args.path, 'test')
            return { path: custom }
          })
        },
      }],
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
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [{
        name: 'name',
        setup(build) {
          build.onResolve({ filter: /^fib\((\d+)\)$/ }, args => {
            return { path: args.path, namespace: 'fib' }
          })
          build.onLoad({ filter: /^fib\((\d+)\)$/, namespace: 'fib' }, args => {
            let match = /^fib\((\d+)\)$/.exec(args.path), n = +match[1]
            let contents = n < 2 ? `export default ${n}` : `
              import n1 from 'fib(${n - 1})'
              import n2 from 'fib(${n - 2})'
              export default n1 + n2`
            return { contents }
          })
        },
      }],
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
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [{
        name: 'name',
        setup(build) {
          build.onResolve({ filter: /^fib\((\d+)\)/ }, args => {
            return { path: args.path, namespace: 'fib' }
          })
          build.onLoad({ filter: /^fib\((\d+)\)/, namespace: 'fib' }, args => {
            let match = /^fib\((\d+)\)/.exec(args.path), n = +match[1]
            let contents = n < 2 ? `export default ${n}` : `
              import n1 from 'fib(${n - 1}) ${args.path}'
              import n2 from 'fib(${n - 2}) ${args.path}'
              export default n1 + n2`
            return { contents }
          })
        },
      }],
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
        {
          name: 'plugin1',
          setup(build) {
            build.onResolve({ filter: /^.*$/ }, () => { trace.push('called first') })
          },
        },
        {
          name: 'plugin2',
          setup(build) {
            build.onResolve({ filter: /^ignore me$/ }, () => { trace.push('not called') })
          },
        },
        {
          name: 'plugin3',
          setup(build) {
            build.onResolve({ filter: /^.*$/ }, () => {
              trace.push('called second')
              return { path: nested }
            })
          },
        },
        {
          name: 'plugin4',
          setup(build) {
            build.onResolve({ filter: /^.*$/ }, () => { trace.push('not called') })
          },
        }
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
        {
          name: 'plugin1',
          setup(build) {
            build.onLoad({ filter: /^.*$/ }, () => { trace.push('called first') })
          },
        },
        {
          name: 'plugin2',
          setup(build) {
            build.onLoad({ filter: /^.*$/, namespace: 'ignore-me' }, () => { trace.push('not called') })
          },
        },
        {
          name: 'plugin3',
          setup(build) {
            build.onLoad({ filter: /^.*$/, namespace: 'file' }, () => {
              trace.push('called second')
              return { contents: 'export default "abc"' }
            })
          },
        },
        {
          name: 'plugin4',
          setup(build) {
            build.onLoad({ filter: /^.*$/, namespace: 'file' }, () => { trace.push('not called') })
          },
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
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [{
        name: 'name',
        setup(build) {
          build.onResolve({ filter: /^http:\/\// }, args => {
            return { path: args.path, namespace: 'http' }
          })
          build.onResolve({ filter: /.*/, namespace: 'http' }, args => {
            return { path: new URL(args.path, args.importer).toString(), namespace: 'http' }
          })
          build.onLoad({ filter: /^http:\/\//, namespace: 'http' }, args => {
            switch (args.path) {
              case 'http://example.com/assets/js/example.js':
                return { contents: `import y from './data/base.js'; export default y` }
              case 'http://example.com/assets/js/data/base.js':
                return { contents: `export default 123` }
            }
          })
        },
      }],
    })
    const result = require(output)
    assert.strictEqual(result.default, 123)
  },

  async rewriteExternalWithNamespace({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `
      import {exists} from 'extern'
      export default exists
    `)
    await esbuild.build({
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [{
        name: 'name',
        setup(build) {
          build.onResolve({ filter: /^extern$/ }, () => {
            return { path: 'fs', external: true, namespace: 'for-testing' }
          })
        },
      }],
    })
    const result = require(output)
    assert.strictEqual(result.default, fs.exists)
  },

  async rewriteExternalWithoutNamespace({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `
      import {exists} from 'extern'
      export default exists
    `)
    await esbuild.build({
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [{
        name: 'name',
        setup(build) {
          build.onResolve({ filter: /^extern$/ }, () => {
            return { path: 'fs', external: true }
          })
        },
      }],
    })
    const result = require(output)
    assert.strictEqual(result.default, fs.exists)
  },

  async rewriteExternalWithFileNamespace({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const outdir = path.join(testDir, 'out')
    const outdir2 = path.join(testDir, 'out2')
    const target = path.join(outdir2, 'target.js')
    await writeFileAsync(input, `
      import {exists} from 'extern'
      export default exists
    `)
    await mkdirAsync(outdir2, { recursive: true })
    await writeFileAsync(target, `
      module.exports = require('fs')
    `)
    await esbuild.build({
      entryPoints: [input], bundle: true, outdir, format: 'cjs', plugins: [{
        name: 'name',
        setup(build) {
          build.onResolve({ filter: /^extern$/ }, () => {
            return { path: path.join(outdir, 'target'), external: true, namespace: 'file' }
          })
        },
      }],
    })

    // Move the file to show that the output has a relative path
    await fs.promises.rename(path.join(outdir, 'in.js'), path.join(outdir2, 'in.js'))

    const result = require(path.join(outdir2, 'in.js'))
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
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [{
        name: 'name',
        setup(build) {
          build.onLoad({ filter: /\.custom$/ }, async (args) => {
            return { contents: await readFileAsync(args.path), resolveDir }
          })
        },
      }],
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
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [{
        name: 'name',
        setup(build) {
          build.onLoad({ filter: /\.custom$/ }, async (args) => {
            return { contents: await readFileAsync(args.path) }
          })
        },
      }],
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
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [{
        name: 'name',
        setup(build) {
          let contents = `export {default} from './loadme'`
          build.onResolve({ filter: /^virtual$/ }, () => ({ path: 'virtual', namespace: 'for-testing' }))
          build.onLoad({ filter: /.*/, namespace: 'for-testing' }, () => ({ contents, resolveDir }))
        },
      }],
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
        entryPoints: [input], bundle: true, outfile: output, format: 'cjs', logLevel: 'silent', plugins: [{
          name: 'name',
          setup(build) {
            let contents = `export {default} from './loadme'`
            build.onResolve({ filter: /^virtual$/ }, () => ({ path: 'virtual', namespace: 'for-testing' }))
            build.onLoad({ filter: /.*/, namespace: 'for-testing' }, () => ({ contents }))
          },
        }],
      })
    } catch (e) {
      error = e
    }
    assert.notStrictEqual(error, void 0)
    if (!Array.isArray(error.errors)) throw error
    assert.strictEqual(error.errors.length, 1)
    assert.strictEqual(error.errors[0].text, `Could not resolve "./loadme" (the plugin "name" didn't set a resolve directory)`)
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
      entryPoints: [input], bundle: true, outfile: output, format: 'cjs', plugins: [{
        name: 'name',
        setup(build) {
          build.onResolve({ filter: /\.wasm$/ }, args => ({
            path: path.isAbsolute(args.path) ? args.path : path.join(args.resolveDir, args.path),
            namespace: args.namespace === 'wasm-stub' ? 'wasm-binary' : 'wasm-stub',
          }))
          build.onLoad({ filter: /.*/, namespace: 'wasm-binary' }, async (args) =>
            ({ contents: await readFileAsync(args.path), loader: 'binary' }))
          build.onLoad({ filter: /.*/, namespace: 'wasm-stub' }, async (args) => ({
            contents: `import wasm from ${JSON.stringify(args.path)}
              export default async (imports) =>
                (await WebAssembly.instantiate(wasm, imports)).instance.exports` }))
        },
      }],
    })
    const result = require(output)
    assert.strictEqual(await result.default(103, 20), 123)
  },

  async virtualEntryPoints({ esbuild, testDir }) {
    const result = await esbuild.build({
      entryPoints: ['1', '2', 'a<>:"|?*b', 'a/b/c.d.e'],
      bundle: true, write: false, outdir: testDir, format: 'esm', plugins: [{
        name: 'name',
        setup(build) {
          build.onResolve({ filter: /.*/ }, args => {
            return { path: `input ${args.path}`, namespace: 'virtual-ns' }
          })
          build.onLoad({ filter: /.*/, namespace: 'virtual-ns' }, args => {
            return { contents: `console.log(${JSON.stringify(args.path)})` }
          })
        },
      }],
    })
    assert.strictEqual(result.outputFiles.length, 4)
    assert.strictEqual(result.outputFiles[0].path, path.join(testDir, 'input 1.js'))
    assert.strictEqual(result.outputFiles[1].path, path.join(testDir, 'input 2.js'))
    assert.strictEqual(result.outputFiles[2].path, path.join(testDir, 'input a_b.js'))
    assert.strictEqual(result.outputFiles[3].path, path.join(testDir, 'c.d.js'))
    assert.strictEqual(result.outputFiles[0].text, `// virtual-ns:input 1\nconsole.log("input 1");\n`)
    assert.strictEqual(result.outputFiles[1].text, `// virtual-ns:input 2\nconsole.log("input 2");\n`)
    assert.strictEqual(result.outputFiles[2].text, `// virtual-ns:input a<>:"|?*b\nconsole.log('input a<>:"|?*b');\n`)
    assert.strictEqual(result.outputFiles[3].text, `// virtual-ns:input a/b/c.d.e\nconsole.log("input a/b/c.d.e");\n`)
  },

  async stdinImporter({ esbuild, testDir }) {
    const output = path.join(testDir, 'out.js')
    await esbuild.build({
      stdin: {
        contents: `import x from "plugin"; export default x`,
        sourcefile: 'stdin-sourcefile',
      },
      bundle: true, outfile: output, format: 'cjs', plugins: [{
        name: 'name',
        setup(build) {
          build.onResolve({ filter: /^plugin$/ }, args => {
            assert.strictEqual(args.namespace, '')
            assert.strictEqual(args.importer, 'stdin-sourcefile')
            assert.strictEqual(args.resolveDir, '')
            assert.strictEqual(args.path, 'plugin')
            return { path: args.path, namespace: 'worked' }
          })
          build.onLoad({ filter: /.*/, namespace: 'worked' }, () => {
            return { contents: `export default 123` }
          })
        },
      }],
    })
    const result = require(output)
    assert.strictEqual(result.default, 123)
  },

  async stdinImporterResolveDir({ esbuild, testDir }) {
    const output = path.join(testDir, 'out.js')
    await esbuild.build({
      stdin: {
        contents: `import x from "plugin"; export default x`,
        sourcefile: 'stdin-sourcefile',
        resolveDir: testDir,
      },
      bundle: true, outfile: output, format: 'cjs', plugins: [{
        name: 'name',
        setup(build) {
          build.onResolve({ filter: /^plugin$/ }, args => {
            assert.strictEqual(args.namespace, 'file')
            assert.strictEqual(args.importer, path.join(testDir, 'stdin-sourcefile'))
            assert.strictEqual(args.resolveDir, testDir)
            assert.strictEqual(args.path, 'plugin')
            return { path: args.path, namespace: 'worked' }
          })
          build.onLoad({ filter: /.*/, namespace: 'worked' }, () => {
            return { contents: `export default 123` }
          })
        },
      }],
    })
    const result = require(output)
    assert.strictEqual(result.default, 123)
  },

  async stdinAbsoluteImporterResolveDir({ esbuild, testDir }) {
    const output = path.join(testDir, 'out.js')
    await esbuild.build({
      stdin: {
        contents: `import x from "plugin"; export default x`,
        sourcefile: path.join(testDir, 'stdin-sourcefile'),
        resolveDir: testDir,
      },
      bundle: true, outfile: output, format: 'cjs', plugins: [{
        name: 'name',
        setup(build) {
          build.onResolve({ filter: /^plugin$/ }, args => {
            assert.strictEqual(args.namespace, 'file')
            assert.strictEqual(args.importer, path.join(testDir, 'stdin-sourcefile'))
            assert.strictEqual(args.resolveDir, testDir)
            assert.strictEqual(args.path, 'plugin')
            return { path: args.path, namespace: 'worked' }
          })
          build.onLoad({ filter: /.*/, namespace: 'worked' }, () => {
            return { contents: `export default 123` }
          })
        },
      }],
    })
    const result = require(output)
    assert.strictEqual(result.default, 123)
  },

  async stdinRelative({ esbuild, testDir }) {
    const output = path.join(testDir, 'out.js')
    await esbuild.build({
      stdin: {
        contents: `import x from "./stdinRelative.js"; export default x`,
      },
      bundle: true, outfile: output, format: 'cjs', plugins: [{
        name: 'name',
        setup(build) {
          build.onResolve({ filter: /.*/ }, args => {
            assert.strictEqual(args.namespace, '')
            assert.strictEqual(args.importer, '<stdin>')
            assert.strictEqual(args.resolveDir, '')
            assert.strictEqual(args.path, './stdinRelative.js')
            return { path: args.path, namespace: 'worked' }
          })
          build.onLoad({ filter: /.*/, namespace: 'worked' }, () => {
            return { contents: `export default 123` }
          })
        },
      }],
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
      bundle: true, outfile: output, format: 'cjs', plugins: [{
        name: 'name',
        setup(build) {
          build.onResolve({ filter: /.*/ }, args => {
            assert.strictEqual(args.namespace, '')
            assert.strictEqual(args.importer, '<stdin>')
            assert.strictEqual(args.resolveDir, testDir)
            assert.strictEqual(args.path, './stdinRelative.js')
            return { path: args.path, namespace: 'worked' }
          })
          build.onLoad({ filter: /.*/, namespace: 'worked' }, () => {
            return { contents: `export default 123` }
          })
        },
      }],
    })
    const result = require(output)
    assert.strictEqual(result.default, 123)
  },

  async externalRequire({ esbuild, testDir }) {
    const externalPlugin = external => ({
      name: 'external',
      setup(build) {
        let escape = text => `^${text.replace(/[-\/\\^$*+?.()|[\]{}]/g, '\\$&')}$`
        let filter = new RegExp(external.map(escape).join('|'))
        build.onResolve({ filter: /.*/, namespace: 'external' }, args => ({
          path: args.path, external: true
        }))
        build.onResolve({ filter }, args => ({
          path: args.path, namespace: 'external'
        }))
        build.onLoad({ filter: /.*/, namespace: 'external' }, args => ({
          contents: `import * as all from ${JSON.stringify(args.path)}; module.exports = all`
        }))
      },
    })
    const outfile = path.join(testDir, 'out', 'output.mjs')
    await esbuild.build({
      stdin: {
        contents: `const fs = require('fs')
          const url = require('url')
          const path = require('path')
          export default fs.readdirSync(path.dirname(url.fileURLToPath(import.meta.url)))
        `,
      },
      bundle: true, outfile, format: 'esm', plugins: [
        externalPlugin(['fs', 'url', 'path'])
      ],
    })
    const result = await import(url.pathToFileURL(outfile))
    assert.deepStrictEqual(result.default, [path.basename(outfile)])
  },

  async newlineInPath({ esbuild }) {
    // Using a path with a newline shouldn't cause a syntax error when the path is printed in a comment
    for (let nl of ['\r', '\n', '\r\n', '\u2028', '\u2029']) {
      let problem = `a b${nl}c d`
      const plugin = {
        name: 'test',
        setup(build) {
          build.onResolve({ filter: /.*/ }, args => ({
            path: args.path, namespace: 'test',
          }))
          build.onLoad({ filter: /.*/, namespace: 'test' }, args => ({
            contents: `return ${JSON.stringify(args.path)}`
          }))
        },
      }
      let result = await esbuild.build({
        entryPoints: [problem],
        bundle: true, write: false, format: 'cjs', plugins: [plugin],
      })
      let value = new Function(result.outputFiles[0].text)()
      assert.deepStrictEqual(value, problem)
    }
  },

  async newlineInNamespace({ esbuild }) {
    // Using a namespace with a newline shouldn't cause a syntax error when the namespace is printed in a comment
    for (let nl of ['\r', '\n', '\r\n', '\u2028', '\u2029']) {
      let problem = `a b${nl}c d`
      const plugin = {
        name: 'test',
        setup(build) {
          build.onResolve({ filter: /.*/ }, args => ({
            path: args.path, namespace: problem,
          }))
          build.onLoad({ filter: /.*/, namespace: problem }, args => ({
            contents: `return ${JSON.stringify(args.namespace)}`
          }))
        },
      }
      let result = await esbuild.build({
        entryPoints: ['entry'],
        bundle: true, write: false, format: 'cjs', plugins: [plugin],
      })
      let value = new Function(result.outputFiles[0].text)()
      assert.deepStrictEqual(value, problem)
    }
  },

  async transformUndefinedDetailForError({ esbuild }) {
    try {
      await esbuild.transform('x y')
      throw new Error('Expected an error to be thrown')
    } catch (e) {
      assert.deepStrictEqual(e.warnings, [])
      assert.deepStrictEqual(e.errors, [{
        text: 'Expected ";" but found "y"',
        location: {
          file: '<stdin>',
          line: 1,
          column: 2,
          length: 1,
          lineText: 'x y',
        },
        detail: void 0,
      }])
    }
  },

  async transformUndefinedDetailForWarning({ esbuild }) {
    const result = await esbuild.transform('typeof x == "null"')
    assert.deepStrictEqual(result.warnings, [{
      text: 'The "typeof" operator will never evaluate to "null"',
      location: {
        file: '<stdin>',
        line: 1,
        column: 12,
        length: 6,
        lineText: 'typeof x == "null"',
      },
      detail: void 0,
    }])
  },

  async buildUndefinedDetailForError({ esbuild }) {
    try {
      await esbuild.build({
        stdin: { contents: 'x y' },
        write: false,
        logLevel: 'silent',
      })
      throw new Error('Expected an error to be thrown')
    } catch (e) {
      assert.deepStrictEqual(e.warnings, [])
      assert.deepStrictEqual(e.errors, [{
        text: 'Expected ";" but found "y"',
        location: {
          file: '<stdin>',
          line: 1,
          column: 2,
          length: 1,
          lineText: 'x y',
        },
        detail: void 0,
      }])
    }
  },

  async buildUndefinedDetailForWarning({ esbuild }) {
    const result = await esbuild.build({
      stdin: { contents: 'typeof x == "null"' },
      write: false,
      logLevel: 'silent',
    })
    assert.deepStrictEqual(result.warnings, [{
      text: 'The "typeof" operator will never evaluate to "null"',
      location: {
        file: '<stdin>',
        line: 1,
        column: 12,
        length: 6,
        lineText: 'typeof x == "null"',
      },
      detail: void 0,
    }])
  },

  async specificDetailForOnResolvePluginThrowError({ esbuild }) {
    const theError = new Error('theError');
    try {
      await esbuild.build({
        entryPoints: ['entry'],
        write: false,
        logLevel: 'silent',
        plugins: [{
          name: 'plugin',
          setup(build) {
            build.onResolve({ filter: /.*/ }, () => {
              throw theError;
            })
          },
        }],
      })
      throw new Error('Expected an error to be thrown')
    } catch (e) {
      assert.strictEqual(e.warnings.length, 0)
      assert.strictEqual(e.errors.length, 1)
      assert.strictEqual(e.errors[0].text, '[plugin] theError')
      assert.strictEqual(e.errors[0].detail, theError)
    }
  },

  async specificDetailForOnLoadPluginThrowError({ esbuild }) {
    const theError = new Error('theError');
    try {
      await esbuild.build({
        entryPoints: ['entry'],
        write: false,
        logLevel: 'silent',
        plugins: [{
          name: 'plugin',
          setup(build) {
            build.onResolve({ filter: /.*/ }, () => ({ path: 'abc', namespace: 'xyz' }))
            build.onLoad({ filter: /.*/ }, () => {
              throw theError;
            })
          },
        }],
      })
      throw new Error('Expected an error to be thrown')
    } catch (e) {
      assert.strictEqual(e.warnings.length, 0)
      assert.strictEqual(e.errors.length, 1)
      assert.strictEqual(e.errors[0].text, '[plugin] theError')
      assert.strictEqual(e.errors[0].detail, theError)
    }
  },

  async specificDetailForOnResolvePluginReturnError({ esbuild }) {
    const theError = new Error('theError');
    try {
      await esbuild.build({
        entryPoints: ['entry'],
        write: false,
        logLevel: 'silent',
        plugins: [{
          name: 'plugin',
          setup(build) {
            build.onResolve({ filter: /.*/ }, () => {
              return { errors: [{ text: 'some error', detail: theError }] };
            })
          },
        }],
      })
      throw new Error('Expected an error to be thrown')
    } catch (e) {
      assert.strictEqual(e.warnings.length, 0)
      assert.strictEqual(e.errors.length, 1)
      assert.strictEqual(e.errors[0].text, '[plugin] some error')
      assert.strictEqual(e.errors[0].detail, theError)
    }
  },

  async specificDetailForOnResolvePluginReturnWarning({ esbuild }) {
    const theError = new Error('theError');
    const result = await esbuild.build({
      entryPoints: ['entry'],
      write: false,
      logLevel: 'silent',
      plugins: [{
        name: 'plugin',
        setup(build) {
          build.onResolve({ filter: /.*/ }, () => {
            return { path: 'abc', namespace: 'xyz', warnings: [{ text: 'some warning', detail: theError }] };
          })
          build.onLoad({ filter: /.*/ }, () => ({ contents: '' }))
        },
      }],
    })
    assert.strictEqual(result.warnings.length, 1)
    assert.strictEqual(result.warnings[0].text, '[plugin] some warning')
    assert.strictEqual(result.warnings[0].detail, theError)
  },

  async specificDetailForOnLoadPluginReturnError({ esbuild }) {
    const theError = new Error('theError');
    try {
      await esbuild.build({
        entryPoints: ['entry'],
        write: false,
        logLevel: 'silent',
        plugins: [{
          name: 'plugin',
          setup(build) {
            build.onResolve({ filter: /.*/ }, () => ({ path: 'abc', namespace: 'xyz' }))
            build.onLoad({ filter: /.*/ }, () => {
              return { errors: [{ text: 'some error', detail: theError }] };
            })
          },
        }],
      })
      throw new Error('Expected an error to be thrown')
    } catch (e) {
      assert.strictEqual(e.warnings.length, 0)
      assert.strictEqual(e.errors.length, 1)
      assert.strictEqual(e.errors[0].text, '[plugin] some error')
      assert.strictEqual(e.errors[0].detail, theError)
    }
  },

  async specificDetailForOnLoadPluginReturnWarning({ esbuild }) {
    const theError = new Error('theError');
    const result = await esbuild.build({
      entryPoints: ['entry'],
      write: false,
      logLevel: 'silent',
      plugins: [{
        name: 'plugin',
        setup(build) {
          build.onResolve({ filter: /.*/ }, () => ({ path: 'abc', namespace: 'xyz' }))
          build.onLoad({ filter: /.*/ }, () => {
            return { contents: '', warnings: [{ text: 'some warning', detail: theError }] };
          })
        },
      }],
    })
    assert.strictEqual(result.warnings.length, 1)
    assert.strictEqual(result.warnings[0].text, '[plugin] some warning')
    assert.strictEqual(result.warnings[0].detail, theError)
  },

  async pluginDataResolveToLoad({ esbuild }) {
    const theObject = {}
    const result = await esbuild.build({
      entryPoints: ['entry'],
      write: false,
      plugins: [{
        name: 'plugin',
        setup(build) {
          build.onResolve({ filter: /.*/ }, () => ({
            path: 'abc',
            namespace: 'xyz',
            pluginData: theObject,
          }))
          build.onLoad({ filter: /.*/ }, args => {
            assert.strictEqual(args.pluginData, theObject)
            return { contents: 'foo()' };
          })
        },
      }],
    })
    assert.strictEqual(result.outputFiles[0].text, 'foo();\n')
  },

  async pluginDataResolveToLoadNested({ esbuild }) {
    const theObject = {}
    const result = await esbuild.build({
      entryPoints: ['entry'],
      write: false,
      bundle: true,
      format: 'esm',
      plugins: [{
        name: 'plugin',
        setup(build) {
          build.onResolve({ filter: /.*/ }, args => {
            if (args.path === 'entry') return { path: 'entry', namespace: 'xyz' }
            return {
              path: 'nested',
              namespace: 'xyz',
              pluginData: theObject,
            }
          })
          build.onLoad({ filter: /.*/ }, args => {
            if (args.path === 'entry') return { contents: 'import "nested"' };
            assert.strictEqual(args.pluginData, theObject)
            return { contents: 'foo()' };
          })
        },
      }],
    })
    assert.strictEqual(result.outputFiles[0].text, '// xyz:nested\nfoo();\n')
  },

  async pluginDataLoadToResolve({ esbuild }) {
    const theObject = {}
    const result = await esbuild.build({
      entryPoints: ['entry'],
      write: false,
      plugins: [{
        name: 'plugin',
        setup(build) {
          build.onResolve({ filter: /.*/ }, args => {
            if (args === 'import') {
              assert.strictEqual(args.pluginData, theObject)
              return { external: true }
            }
            return { path: 'abc', namespace: 'xyz' }
          })
          build.onLoad({ filter: /.*/ }, () => ({
            contents: 'import("import")',
            pluginData: theObject,
          }))
        },
      }],
    })
    assert.strictEqual(result.outputFiles[0].text, 'import("import");\n')
  },

  async pluginWithWatchMode({ esbuild, service, testDir }) {
    for (const toTest of [esbuild, service]) {
      const srcDir = path.join(testDir, 'src')
      const outfile = path.join(testDir, 'out.js')
      const input = path.join(srcDir, 'in.js')
      const example = path.join(srcDir, 'example.js')
      await mkdirAsync(srcDir, { recursive: true })
      await writeFileAsync(input, `import {x} from "./example.js"; exports.x = x`)
      await writeFileAsync(example, `export let x = 1`)

      let onRebuild = () => { }
      const result = await toTest.build({
        entryPoints: [input],
        outfile,
        format: 'cjs',
        watch: {
          onRebuild: (...args) => onRebuild(args),
        },
        bundle: true,
        plugins: [
          {
            name: 'some-plugin',
            setup(build) {
              build.onLoad({ filter: /example\.js$/ }, async (args) => {
                const contents = await fs.promises.readFile(args.path, 'utf8')
                return { contents }
              })
            },
          },
        ],
      })
      const rebuildUntil = (mutator, condition) => {
        let timeout
        return new Promise((resolve, reject) => {
          timeout = setTimeout(() => reject(new Error('Timeout after 30 seconds')), 30 * 1000)
          onRebuild = args => {
            try { if (condition(...args)) clearTimeout(timeout), resolve(args) }
            catch (e) { clearTimeout(timeout), reject(e) }
          }
          mutator()
        })
      }

      try {
        let code = await readFileAsync(outfile, 'utf8')
        let exports = {}
        new Function('exports', code)(exports)
        assert.strictEqual(result.outputFiles, void 0)
        assert.strictEqual(typeof result.stop, 'function')
        assert.strictEqual(exports.x, 1)

        // First rebuild: edit
        {
          const [error2, result2] = await rebuildUntil(
            () => writeFileAsync(example, `export let x = 2`),
            () => fs.readFileSync(outfile, 'utf8') !== code,
          )
          code = await readFileAsync(outfile, 'utf8')
          exports = {}
          new Function('exports', code)(exports)
          assert.strictEqual(error2, null)
          assert.strictEqual(result2.outputFiles, void 0)
          assert.strictEqual(result2.stop, result.stop)
          assert.strictEqual(exports.x, 2)
        }
      } finally {
        result.stop()
      }
    }
  },
}

async function main() {
  const esbuild = installForTests()

  // Create a fresh test directory
  removeRecursiveSync(rootTestDir)
  fs.mkdirSync(rootTestDir)

  // Time out these tests after 5 minutes. This exists to help debug test hangs in CI.
  let minutes = 5
  let timeout = setTimeout(() => {
    console.error(`❌ js api tests timed out after ${minutes} minutes, exiting...`)
    process.exit(1)
  }, minutes * 60 * 1000)

  // Start the esbuild service
  const service = await esbuild.startService()

  // Run all tests concurrently
  const runTest = async ([name, fn]) => {
    let testDir = path.join(rootTestDir, name)
    try {
      await mkdirAsync(testDir)
      await fn({ esbuild, service, testDir })
      removeRecursiveSync(testDir)
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
    removeRecursiveSync(rootTestDir)
  }

  clearTimeout(timeout);
}

main().catch(e => setTimeout(() => { throw e }))
