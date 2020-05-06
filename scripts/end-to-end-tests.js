const childProcess = require('child_process')
const path = require('path')
const util = require('util')
const fs = require('fs')
const testDir = path.join(__dirname, '.end-to-end-tests')
const esbuildPath = path.join(__dirname, '..', 'esbuild')
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

  // Tests for symlinks
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
]

// Test CommonJS export (internal and external)
for (let isExternal of [false, true]) {
  const args = isExternal ? ['--format=cjs', 'foo.js', '--outfile=out.js'] : ['bar.js', '--outfile=node.js']
  const innerName = isExternal ? 'out.js' : 'foo.js'
  const outerName = isExternal ? 'node.js' : 'bar.js'
  tests.push(
    test(['--bundle'].concat(args), {
      'foo.js': `exports.foo = 123`,
      [outerName]: `const out = require('./${innerName}'); if (out.__esModule || out.foo !== 123) throw 'fail'`,
    }),
    test(['--bundle'].concat(args), {
      'foo.js': `module.exports = 123`,
      [outerName]: `const out = require('./${innerName}'); if (out.__esModule || out !== 123) throw 'fail'`,
    }),
    test(['--bundle'].concat(args), {
      'foo.js': `export const foo = 123`,
      [outerName]: `const out = require('./${innerName}'); if (!out.__esModule || out.foo !== 123) throw 'fail'`,
    }),
    test(['--bundle'].concat(args), {
      'foo.js': `export default 123`,
      [outerName]: `const out = require('./${innerName}'); if (!out.__esModule || out.default !== 123) throw 'fail'`,
    }),
  )
}

// Test CommonJS import (internal and external)
for (let isExternal of [false, true]) {
  const external = isExternal ? ['--external:foo'] : []
  const name = isExternal ? 'index.js' : 'node.js'
  tests.push(
    test(['--bundle', 'in.js', '--outfile=node.js'].concat(external), {
      'in.js': `const foo = require('foo'); if (!foo.bar.endsWith('${name}')) throw 'fail'`,
      'node_modules/foo/index.js': `exports.bar = __filename`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'].concat(external), {
      'in.js': `const foo = require('foo'); if (!foo.endsWith('${name}')) throw 'fail'`,
      'node_modules/foo/index.js': `module.exports = __filename`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'].concat(external), {
      'in.js': `import * as foo from 'foo'; if (!foo.bar.endsWith('${name}')) throw 'fail'`,
      'node_modules/foo/index.js': `exports.bar = __filename`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'].concat(external), {
      'in.js': `import * as foo from 'foo'; if (!foo.default.endsWith('${name}')) throw 'fail'`,
      'node_modules/foo/index.js': `module.exports = __filename`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'].concat(external), {
      'in.js': `import foo from 'foo'; if (!foo.endsWith('${name}')) throw 'fail'`,
      'node_modules/foo/index.js': `module.exports = __filename`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'].concat(external), {
      'in.js': `import * as foo from 'foo'; if (!foo.default.default.endsWith('${name}')) throw 'fail'`,
      'node_modules/foo/index.js': `module.exports = {default: __filename}`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'].concat(external), {
      'in.js': `import * as foo from 'foo'; if (!foo.default.endsWith('${name}')) throw 'fail'`,
      'node_modules/foo/index.js': `module.exports = {__esModule: true, default: __filename}`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'].concat(external), {
      'in.js': `import foo from 'foo'; if (!foo.default.endsWith('${name}')) throw 'fail'`,
      'node_modules/foo/index.js': `module.exports = {default: __filename}`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'].concat(external), {
      'in.js': `import foo from 'foo'; if (!foo.endsWith('${name}')) throw 'fail'`,
      'node_modules/foo/index.js': `module.exports = {__esModule: true, default: __filename}`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'].concat(external), {
      'in.js': `import('foo').then(foo => setTimeout(() => { if (!foo.bar.endsWith('${name}')) throw 'fail' }))`,
      'node_modules/foo/index.js': `exports.bar = __filename`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'].concat(external), {
      'in.js': `import('foo').then(foo => setTimeout(() => { if (!foo.default.endsWith('${name}')) throw 'fail' }))`,
      'node_modules/foo/index.js': `module.exports = __filename`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'].concat(external), {
      'in.js': `import('foo').then(foo => setTimeout(() => { if (!foo.default.default.endsWith('${name}')) throw 'fail' }))`,
      'node_modules/foo/index.js': `module.exports = {default: __filename}`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'].concat(external), {
      'in.js': `import('foo').then(foo => setTimeout(() => { if (!foo.default.endsWith('${name}')) throw 'fail' }))`,
      'node_modules/foo/index.js': `module.exports = {__esModule: true, default: __filename}`,
    }),
  )
}

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

function test(args, files) {
  return async () => {
    try {
      const thisTestDir = path.join(testDir, '' + testCount++)

      // Test setup
      for (const file in files) {
        const filePath = path.join(thisTestDir, file)
        const contents = files[file]
        await util.promisify(childProcess.exec)(`mkdir -p "${path.dirname(filePath)}"`)

        // Optionally symlink the file if the test requests it
        if (contents.symlink) await util.promisify(fs.symlink)(contents.symlink, filePath)
        else await util.promisify(fs.writeFile)(filePath, contents)
      }

      // Run esbuild
      await util.promisify(childProcess.execFile)(esbuildPath, args, { cwd: thisTestDir, stdio: 'pipe' })

      // Run the resulting node.js file and make sure it exits cleanly
      require(path.join(thisTestDir, 'node.js'))
    }

    catch (e) {
      console.error(`❌ test failed: ${e && e.message || e}
  args: ${args.join(' ')}
  files: ${Object.entries(files).map(([k, v]) => `\n    ${k}: ${v}`).join('')}`)
      return false
    }

    return true
  }
}

async function main() {
  // Make sure esbuild is built
  childProcess.execSync('make', { cwd: path.dirname(__dirname), stdio: 'pipe' })

  // Create a fresh test directory
  childProcess.execSync(`rm -fr "${testDir}"`)
  fs.mkdirSync(testDir)

  // Run all tests concurrently
  const allTestsPassed = (await Promise.all(tests.map(test => test()))).every(success => success)

  // Clean up test output
  childProcess.execSync(`rm -fr "${testDir}"`)

  if (!allTestsPassed) {
    process.exit(1)
  }
}

main().catch(e => setTimeout(() => { throw e }))
