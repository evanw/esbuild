(async () => {
  // Make sure this script runs as an ES6 module so we can import both ES6 modules and CommonJS modules
  if (typeof require !== 'undefined') {
    const childProcess = require('child_process')
    const child = childProcess.spawn('node', ['--experimental-modules', '--input-type=module'], {
      cwd: __dirname,
      stdio: ['pipe', 'inherit', 'inherit'],
    })
    child.stdin.write(require('fs').readFileSync(__filename))
    child.stdin.end()
    child.on('close', code => process.exit(code))
    return
  }

  const childProcess = await import('child_process')
  const { default: { buildBinary, dirname } } = await import('./esbuild.js')
  const { default: mkdirp } = await import('mkdirp')
  const { default: rimraf } = await import('rimraf')
  const assert = await import('assert')
  const path = await import('path')
  const util = await import('util')
  const url = await import('url')
  const fs = await import('fs')
  const testDir = path.join(dirname, '.end-to-end-tests')
  const esbuildPath = buildBinary()
  const tests = []
  let testCount = 0

  // Tests for "--define"
  tests.push(
    test(['--define:foo=null', 'in.js', '--outfile=node.js'], { 'in.js': `if (foo !== null) throw 'fail'` }),
    test(['--define:foo=true', 'in.js', '--outfile=node.js'], { 'in.js': `if (foo !== true) throw 'fail'` }),
    test(['--define:foo=false', 'in.js', '--outfile=node.js'], { 'in.js': `if (foo !== false) throw 'fail'` }),
    test(['--define:foo="abc"', 'in.js', '--outfile=node.js'], { 'in.js': `if (foo !== "abc") throw 'fail'` }),
    test(['--define:foo=123.456', 'in.js', '--outfile=node.js'], { 'in.js': `if (foo !== 123.456) throw 'fail'` }),
    test(['--define:foo=-123.456', 'in.js', '--outfile=node.js'], { 'in.js': `if (foo !== -123.456) throw 'fail'` }),
    test(['--define:foo=global', 'in.js', '--outfile=node.js'], { 'in.js': `foo.bar = 123; if (bar !== 123) throw 'fail'` }),
    test(['--define:foo=bar', 'in.js', '--outfile=node.js'], { 'in.js': `let bar = {x: 123}; if (foo.x !== 123) throw 'fail'` }),
    test(['--define:a.x=1', 'in.js', '--outfile=node.js'], { 'in.js': `if (a.x !== 1) throw 'fail'` }),
    test(['--define:a.x=1', '--define:a.y=2', 'in.js', '--outfile=node.js'], { 'in.js': `if (a.x + a.y !== 3) throw 'fail'` }),
    test(['--define:a.x=1', '--define:b.y=2', 'in.js', '--outfile=node.js'], { 'in.js': `if (a.x + b.y !== 3) throw 'fail'` }),
    test(['--define:a.x=1', '--define:b.x=2', 'in.js', '--outfile=node.js'], { 'in.js': `if (a.x + b.x !== 3) throw 'fail'` }),
    test(['--define:x=y', '--define:y=x', 'in.js', '--outfile=node.js'], {
      'in.js': `eval('var x="x",y="y"'); if (x + y !== 'yx') throw 'fail'`,
    }),
  )

  // Test recursive directory creation
  tests.push(
    test(['entry.js', '--outfile=a/b/c/d/index.js'], {
      'entry.js': `exports.foo = 123`,
      'node.js': `const ns = require('./a/b/c/d'); if (ns.foo !== 123) throw 'fail'`,
    }),
  )

  // Tests for symlinks
  //
  // Note: These are disabled on Windows because they fail when run with GitHub
  // Actions. I'm not sure what the issue is because they pass for me when run in
  // my Windows VM (Windows 10 in VirtualBox on macOS).
  if (process.platform !== 'win32') {
    tests.push(
      test(['--bundle', 'in.js', '--outfile=node.js'], {
        'in.js': `import {foo} from 'foo'; if (foo !== 123) throw 'fail'`,
        'registry/node_modules/foo/index.js': `export {bar as foo} from 'bar'`,
        'registry/node_modules/bar/index.js': `export const bar = 123`,
        'node_modules/foo': { symlink: `../registry/node_modules/foo` },
      }),
      test(['--bundle', 'in.js', '--outfile=node.js'], {
        'in.js': `import {foo} from 'foo'; if (foo !== 123) throw 'fail'`,
        'registry/node_modules/foo/index.js': `export {bar as foo} from 'bar'`,
        'registry/node_modules/bar/index.js': `export const bar = 123`,
        'node_modules/foo/index.js': { symlink: `../../registry/node_modules/foo/index.js` },
      }),
    )
  }

  // Test internal CommonJS export
  tests.push(
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `const out = require('./foo'); if (out.__esModule || out.foo !== 123) throw 'fail'`,
      'foo.js': `exports.foo = 123`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `const out = require('./foo'); if (out.__esModule || out !== 123) throw 'fail'`,
      'foo.js': `module.exports = 123`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `const out = require('./foo'); if (!out.__esModule || out.foo !== 123) throw 'fail'`,
      'foo.js': `export const foo = 123`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `const out = require('./foo'); if (!out.__esModule || out.default !== 123) throw 'fail'`,
      'foo.js': `export default 123`,
    }),

    // Self export
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `exports.foo = 123; const out = require('./in'); if (out.__esModule || out.foo !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `module.exports = 123; const out = require('./in'); if (out.__esModule || out !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `export const foo = 123; const out = require('./in'); if (!out.__esModule || out.foo !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `export default 123; const out = require('./in'); if (!out.__esModule || out.default !== 123) throw 'fail'`,
    }),
  )

  // Test internal ES6 export
  tests.push(
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `import * as out from './foo'; if (out.foo !== 123) throw 'fail'`,
      'foo.js': `exports.foo = 123`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `import * as out from './foo'; if (out.default !== 123) throw 'fail'`,
      'foo.js': `module.exports = 123`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `import * as out from './foo'; if (out.foo !== 123) throw 'fail'`,
      'foo.js': `export const foo = 123`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `import * as out from './foo'; if (out.default !== 123) throw 'fail'`,
      'foo.js': `export default 123`,
    }),

    // Self export
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      // Exporting like this doesn't work, but that's ok
      'in.js': `exports.foo = 123; import * as out from './in'; if (out.foo !== undefined) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      // Exporting like this doesn't work, but that's ok
      'in.js': `module.exports = {foo: 123}; import * as out from './in'; if (out.foo !== undefined) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `export const foo = 123; import * as out from './in'; if (out.foo !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `export default 123; import * as out from './in'; if (out.default !== 123) throw 'fail'`,
    }),
  )

  // Test external CommonJS export
  tests.push(
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=cjs'], {
      'foo.js': `exports.foo = 123`,
      'node.js': `const out = require('./out'); if (out.__esModule || out.foo !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=cjs'], {
      'foo.js': `module.exports = 123`,
      'node.js': `const out = require('./out'); if (out.__esModule || out !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.mjs', '--format=esm'], {
      'foo.js': `exports.foo = 123`,
      'node.mjs': `import out from './out.mjs'; if (out.foo !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.mjs', '--format=esm'], {
      'foo.js': `module.exports = 123`,
      'node.mjs': `import out from './out.mjs'; if (out !== 123) throw 'fail'`,
    }),
  )

  // Test external ES6 export
  tests.push(
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=cjs'], {
      'foo.js': `export const foo = 123`,
      'node.js': `const out = require('./out'); if (!out.__esModule || out.foo !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=cjs'], {
      'foo.js': `export default 123`,
      'node.js': `const out = require('./out'); if (!out.__esModule || out.default !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.mjs', '--format=esm'], {
      'foo.js': `export const foo = 123`,
      'node.mjs': `import {foo} from './out.mjs'; if (foo !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.mjs', '--format=esm'], {
      'foo.js': `export default 123`,
      'node.mjs': `import out from './out.mjs'; if (out !== 123) throw 'fail'`,
    }),
  )

  // ES6 export star of CommonJS module
  tests.push(
    // Internal
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import * as ns from './re-export'; if (ns.foo !== 123) throw 'fail'`,
      're-export.js': `export * from './commonjs'`,
      'commonjs.js': `exports.foo = 123`,
    }),
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import {foo} from './re-export'; if (foo !== 123) throw 'fail'`,
      're-export.js': `export * from './commonjs'`,
      'commonjs.js': `exports.foo = 123`,
    }),

    // External
    test(['--bundle', 'entry.js', '--outfile=node.js', '--external:fs'], {
      'entry.js': `import * as ns from './re-export'; if (typeof ns.exists !== 'function') throw 'fail'`,
      're-export.js': `export * from 'fs'`,
    }),
    test(['--bundle', 'entry.js', '--outfile=node.js', '--external:fs'], {
      'entry.js': `import {exists} from './re-export'; if (typeof exists !== 'function') throw 'fail'`,
      're-export.js': `export * from 'fs'`,
    }),

    // External (masked)
    test(['--bundle', 'entry.js', '--outfile=node.js', '--external:fs'], {
      'entry.js': `import * as ns from './re-export'; if (ns.exists !== 123) throw 'fail'`,
      're-export.js': `export * from 'fs'; export let exists = 123`,
    }),
    test(['--bundle', 'entry.js', '--outfile=node.js', '--external:fs'], {
      'entry.js': `import {exists} from './re-export'; if (exists !== 123) throw 'fail'`,
      're-export.js': `export * from 'fs'; export let exists = 123`,
    }),
  )

  // Tests for catch scope issues
  tests.push(
    test(['in.js', '--outfile=node.js', '--minify'], {
      'in.js': `
      var x = 0, y = []
      try {
        throw 1
      } catch (x) {
        y.push(x)
        var x = 2
        y.push(x)
      }
      y.push(x)
      if (y + '' !== '1,2,0') throw 'fail: ' + y
    `,
    }),
    test(['in.js', '--outfile=node.js', '--minify'], {
      'in.js': `
      var x = 0, y = []
      try {
        throw 1
      } catch (x) {
        y.push(x)
        var x = 2
        y.push(x)
      }
      finally { x = 3 }
      y.push(x)
      if (y + '' !== '1,2,3') throw 'fail: ' + y
    `,
    }),
    test(['in.js', '--outfile=node.js', '--minify'], {
      'in.js': `
      var y = []
      try {
        throw 1
      } catch (x) {
        y.push(x)
        var x = 2
        y.push(x)
      }
      y.push(x)
      if (y + '' !== '1,2,') throw 'fail: ' + y
    `,
    }),
    test(['in.js', '--outfile=node.js', '--minify'], {
      'in.js': `
      var y = []
      try {
        throw 1
      } catch (x) {
        y.push(x)
        x = 2
        y.push(x)
      }
      y.push(typeof x)
      if (y + '' !== '1,2,undefined') throw 'fail: ' + y
    `,
    }),
    test(['in.js', '--outfile=node.js', '--minify'], {
      'in.js': `
      var y = []
      try {
        throw 1
      } catch (x) {
        y.push(x)
        try {
          throw 2
        } catch (x) {
          y.push(x)
          var x = 3
          y.push(x)
        }
        y.push(x)
      }
      y.push(x)
      if (y + '' !== '1,2,3,1,') throw 'fail: ' + y
    `,
    }),
  )

  // Test cyclic import issues (shouldn't crash on evaluation)
  tests.push(
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import * as foo from './foo'; export default {foo, bar: require('./bar')}`,
      'foo.js': `import * as a from './entry'; import * as b from './bar'; export default {a, b}`,
      'bar.js': `const entry = require('./entry'); export function foo() { return entry }`,
    }),
  )

  // Test directive preservation
  tests.push(
    // The "__pow" symbol must not be hoisted above "use strict"
    test(['entry.js', '--outfile=node.js', '--target=es6'], {
      'entry.js': `
        'use strict'
        function f(a) {
          a **= 2
          return [a, arguments[0]]
        }
        let pair = f(2)
        if (pair[0] !== 4 || pair[1] !== 2) throw 'fail'
      `,
    }),
  )

  // Test minification of top-level symbols
  tests.push(
    test(['in.js', '--outfile=node.js', '--minify'], {
      // Top-level names should not be minified
      'in.js': `function foo() {} if (foo.name !== 'foo') throw 'fail'`,
    }),
    test(['in.js', '--outfile=node.js', '--minify'], {
      // Nested names should be minified
      'in.js': `(() => { function foo() {} if (foo.name === 'foo') throw 'fail' })()`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--target=es6'], {
      // Importing the "__pow()" runtime function should not affect top-level name minification
      'in.js': `let _8 = 2 ** 3; function foo8() {} if (foo8.name !== 'foo' + _8) throw 'fail'`,
    }),
  )

  // Test tree shaking
  tests.push(
    // Keep because used (ES6)
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import * as foo from './foo'; if (global.dce0 !== 123 || foo.abc !== 'abc') throw 'fail'`,
      'foo/index.js': `global.dce0 = 123; export const abc = 'abc'`,
      'foo/package.json': `{ "sideEffects": false }`,
    }),

    // Remove because unused (ES6)
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import * as foo from './foo'; if (global.dce1 !== void 0) throw 'fail'`,
      'foo/index.js': `global.dce1 = 123; export const abc = 'abc'`,
      'foo/package.json': `{ "sideEffects": false }`,
    }),

    // Keep because side effects (ES6)
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import * as foo from './foo'; if (global.dce2 !== 123) throw 'fail'`,
      'foo/index.js': `global.dce2 = 123; export const abc = 'abc'`,
      'foo/package.json': `{ "sideEffects": true }`,
    }),

    // Keep because used (CommonJS)
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import foo from './foo'; if (global.dce3 !== 123 || foo.abc !== 'abc') throw 'fail'`,
      'foo/index.js': `global.dce3 = 123; exports.abc = 'abc'`,
      'foo/package.json': `{ "sideEffects": false }`,
    }),

    // Remove because unused (CommonJS)
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import foo from './foo'; if (global.dce4 !== void 0) throw 'fail'`,
      'foo/index.js': `global.dce4 = 123; exports.abc = 'abc'`,
      'foo/package.json': `{ "sideEffects": false }`,
    }),

    // Keep because side effects (CommonJS)
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import foo from './foo'; if (global.dce5 !== 123) throw 'fail'`,
      'foo/index.js': `global.dce5 = 123; exports.abc = 'abc'`,
      'foo/package.json': `{ "sideEffects": true }`,
    }),
  )

  // Test obscure CommonJS symbol edge cases
  tests.push(
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `const ns = require('./foo'); if (ns.foo !== 123 || ns.bar !== 123) throw 'fail'`,
      'foo.js': `var exports, module; module.exports.foo = 123; exports.bar = exports.foo`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `require('./foo'); require('./bar')`,
      'foo.js': `let exports; if (exports !== void 0) throw 'fail'`,
      'bar.js': `let module; if (module !== void 0) throw 'fail'`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `const ns = require('./foo'); if (ns.foo !== void 0 || ns.default.foo !== 123) throw 'fail'`,
      'foo.js': `var exports = {foo: 123}; export default exports`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `const ns = require('./foo'); if (ns !== 123) throw 'fail'`,
      'foo.ts': `let module = 123; export = module`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `require('./foo')`,
      'foo.js': `var require; if (require !== void 0) throw 'fail'`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `require('./foo')`,
      'foo.js': `var require = x => x; if (require('does not exist') !== 'does not exist') throw 'fail'`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `const ns = require('./foo'); if (ns.a !== 123 || ns.b.a !== 123) throw 'fail'`,
      'foo.js': `exports.a = 123; exports.b = this`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `const ns = require('./foo'); if (ns.a !== 123 || ns.b !== void 0) throw 'fail'`,
      'foo.js': `export let a = 123, b = this`,
    }),
  )

  // Test writing to stdout
  tests.push(
    // These should succeed
    testStdout('exports.foo = 123', [], async (build) => {
      const stdout = await build()
      assert.strictEqual(stdout, `exports.foo = 123;\n`)
    }),
    testStdout('exports.foo = 123', ['--bundle', '--format=cjs'], async (build) => {
      const stdout = await build()
      assert.strictEqual(stdout, `// example.js\nexports.foo = 123;\n`)
    }),
    testStdout('exports.foo = 123', ['--sourcemap'], async (build) => {
      const stdout = await build()
      const start = `exports.foo = 123;\n//# sourceMappingURL=data:application/json;base64,`
      assert.default(stdout.startsWith(start))
      const json = JSON.parse(Buffer.from(stdout.slice(start.length), 'base64').toString())
      assert.strictEqual(json.version, 3)
      assert.deepStrictEqual(json.sources, ['example.js'])
    }),
    testStdout('exports.foo = 123', ['--bundle', '--format=cjs', '--sourcemap'], async (build) => {
      const stdout = await build()
      const start = `// example.js\nexports.foo = 123;\n//# sourceMappingURL=data:application/json;base64,`
      assert.default(stdout.startsWith(start))
      const json = JSON.parse(Buffer.from(stdout.slice(start.length), 'base64').toString())
      assert.strictEqual(json.version, 3)
      assert.deepStrictEqual(json.sources, ['example.js'])
    }),
    testStdout('stuff', ['--loader:.js=text'], async (build) => {
      const stdout = await build()
      assert.strictEqual(stdout, `module.exports = "stuff";\n`)
    }),

    // These should fail
    testStdout('exports.foo = 123', ['--metafile=graph.json'], async (build) => {
      try { await build() } catch (e) { return }
      throw new Error('Expected build failure for "--metafile"')
    }),
    testStdout('exports.foo = 123', ['--sourcemap=external'], async (build) => {
      try { await build() } catch (e) { return }
      throw new Error('Expected build failure for "--metafile"')
    }),
    testStdout('exports.foo = 123', ['--loader:.js=file'], async (build) => {
      try { await build() } catch (e) { return }
      throw new Error('Expected build failure for "--metafile"')
    }),
  )

  function test(args, files, options) {
    return async () => {
      const hasBundle = args.includes('--bundle')
      const hasCJS = args.includes('--format=cjs')
      const hasESM = args.includes('--format=esm')
      const formats = hasCJS || !hasBundle ? ['cjs'] : hasESM ? ['esm'] : ['cjs', 'esm']

      // If the test doesn't specify a format, test both formats
      for (const format of formats) {
        const formatArg = `--format=${format}`
        const modifiedArgs = !hasBundle || args.includes(formatArg) ? args : args.concat(formatArg)
        const thisTestDir = path.join(testDir, '' + testCount++)

        try {
          // Test setup
          for (const file in files) {
            const filePath = path.join(thisTestDir, file)
            const contents = files[file]
            mkdirp.sync(path.dirname(filePath))

            // Optionally symlink the file if the test requests it
            if (contents.symlink) await util.promisify(fs.symlink)(contents.symlink, filePath)
            else await util.promisify(fs.writeFile)(filePath, contents)
          }

          // Run esbuild
          await util.promisify(childProcess.execFile)(esbuildPath, modifiedArgs,
            { cwd: thisTestDir, stdio: 'pipe' })

          // Run the resulting node.js file and make sure it exits cleanly. The
          // use of "pathToFileURL" is a workaround for a problem where node
          // only supports absolute paths on Unix-style systems, not on Windows.
          // See https://github.com/nodejs/node/issues/31710 for more info.
          const nodePath = path.join(thisTestDir, 'node')
          let testExports
          switch (format) {
            case 'cjs':
              ({ default: testExports } = await import(url.pathToFileURL(`${nodePath}.js`)))
              break

            case 'esm':
              if (!(await util.promisify(fs.exists)(nodePath + '.mjs')))
                await util.promisify(fs.rename)(nodePath + '.js', nodePath + '.mjs')
              testExports = await import(url.pathToFileURL(`${nodePath}.mjs`))
              break
          }

          // If this is an async test, run the async part
          if (options && options.async) {
            const AsyncFunction = async function () { }.constructor
            if (!(testExports.async instanceof AsyncFunction))
              throw new Error('Expected async instanceof AsyncFunction')
            await testExports.async()
          }

          // Clean up test output
          rimraf.sync(thisTestDir, { disableGlob: true })
        }

        catch (e) {
          console.error(`❌ test failed: ${e && e.message || e}
  dir: ${path.relative(dirname, thisTestDir)}
  args: ${modifiedArgs.join(' ')}
  files: ${Object.entries(files).map(([k, v]) => `\n    ${k}: ${JSON.stringify(v)}`).join('')}
`)
          return false
        }
      }

      return true
    }
  }

  // There's a feature where bundling without "outfile" or "outdir" writes to stdout instead
  function testStdout(input, args, callback) {
    return async () => {
      const thisTestDir = path.join(testDir, '' + testCount++)

      try {
        mkdirp.sync(thisTestDir)
        const inputFile = path.join(thisTestDir, 'example.js')
        await util.promisify(fs.writeFile)(inputFile, input)

        // Run whatever check the caller is doing
        await callback(async () => {
          const { stdout } = await util.promisify(childProcess.execFile)(
            esbuildPath, [inputFile].concat(args), { cwd: thisTestDir, stdio: 'pipe' })
          return stdout
        })

        // Clean up test output
        rimraf.sync(thisTestDir, { disableGlob: true })
      } catch (e) {
        console.error(`❌ test failed: ${e && e.message || e}
  dir: ${path.relative(dirname, thisTestDir)}`)
        return false
      }

      return true
    }
  }

  // Create a fresh test directory
  rimraf.sync(testDir, { disableGlob: true })
  fs.mkdirSync(testDir)

  // Run all tests concurrently
  const allTestsPassed = (await Promise.all(tests.map(test => test()))).every(success => success)

  if (!allTestsPassed) {
    console.error(`❌ end-to-end tests failed`)
    process.exit(1)
  } else {
    console.log(`✅ end-to-end tests passed`)

    // Clean up test output
    rimraf.sync(testDir, { disableGlob: true })
  }
})().catch(e => setTimeout(() => { throw e }))
