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
    return
  }

  const childProcess = await import('child_process')
  const { default: { buildBinary, dirname } } = await import('./esbuild.js')
  const { default: mkdirp } = await import('mkdirp')
  const { default: rimraf } = await import('rimraf')
  const path = await import('path')
  const util = await import('util')
  const fs = await import('fs')
  const testDir = path.join(dirname, '.end-to-end-tests')
  const esbuildPath = buildBinary()
  let testCount = 0

  let tests = [
    // Tests for "--define"
    test(['--define:foo=null', 'in.js', '--outfile=node.js'], { 'in.js': `if (foo !== null) throw 'fail'` }),
    test(['--define:foo=true', 'in.js', '--outfile=node.js'], { 'in.js': `if (foo !== true) throw 'fail'` }),
    test(['--define:foo=false', 'in.js', '--outfile=node.js'], { 'in.js': `if (foo !== false) throw 'fail'` }),
    test(['--define:foo="abc"', 'in.js', '--outfile=node.js'], { 'in.js': `if (foo !== "abc") throw 'fail'` }),
    test(['--define:foo=123.456', 'in.js', '--outfile=node.js'], { 'in.js': `if (foo !== 123.456) throw 'fail'` }),
    test(['--define:foo=-123.456', 'in.js', '--outfile=node.js'], { 'in.js': `if (foo !== -123.456) throw 'fail'` }),
    test(['--define:foo=global', 'in.js', '--outfile=node.js'], { 'in.js': `foo.bar = 123; if (bar !== 123) throw 'fail'` }),
    test(['--define:foo=bar', 'in.js', '--outfile=node.js'], { 'in.js': `let bar = {x: 123}; if (foo.x !== 123) throw 'fail'` }),
  ]

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

          // Run the resulting node.js file and make sure it exits cleanly
          const nodePath = path.join(thisTestDir, 'node')
          let testExports
          switch (format) {
            case 'cjs':
              ({ default: testExports } = await import(nodePath + '.js'))
              break

            case 'esm':
              if (!(await util.promisify(fs.exists)(nodePath + '.mjs')))
                await util.promisify(fs.rename)(nodePath + '.js', nodePath + '.mjs')
              testExports = await import(nodePath + '.mjs')
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
