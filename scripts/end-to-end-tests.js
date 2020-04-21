const childProcess = require('child_process')
const path = require('path')
const util = require('util')
const fs = require('fs')
const testDir = path.join(__dirname, '.end-to-end-tests')
const esbuildPath = path.join(__dirname, '..', 'esbuild')
let testCount = 0

let tests = () => [
  // Tests for "--define"
  test(['--define:foo=null', 'in.js'], { 'in.js': `if (foo !== null) throw new Error('Test failed')` }),
  test(['--define:foo=true', 'in.js'], { 'in.js': `if (foo !== true) throw new Error('Test failed')` }),
  test(['--define:foo=false', 'in.js'], { 'in.js': `if (foo !== false) throw new Error('Test failed')` }),
  test(['--define:foo="abc"', 'in.js'], { 'in.js': `if (foo !== "abc") throw new Error('Test failed')` }),
  test(['--define:foo=123.456', 'in.js'], { 'in.js': `if (foo !== 123.456) throw new Error('Test failed')` }),
  test(['--define:foo=-123.456', 'in.js'], { 'in.js': `if (foo !== -123.456) throw new Error('Test failed')` }),
  test(['--define:foo=global', 'in.js'], { 'in.js': `foo.bar = 123; if (bar !== 123) throw new Error('Test failed')` }),
  test(['--define:foo=bar', 'in.js'], { 'in.js': `let bar = {x: 123}; if (foo.x !== 123) throw new Error('Test failed')` }),

  // Tests for symlinks
  test(['--bundle', 'in.js'], {
    'in.js': `import {foo} from 'foo'; if (foo !== 123) throw new Error('Test failed')`,
    'registry/node_modules/foo/index.js': `export {bar as foo} from 'bar'`,
    'registry/node_modules/bar/index.js': `export const bar = 123`,
    'node_modules/foo': { symlink: `../registry/node_modules/foo` },
  }),
  test(['--bundle', 'in.js'], {
    'in.js': `import {foo} from 'foo'; if (foo !== 123) throw new Error('Test failed')`,
    'registry/node_modules/foo/index.js': `export {bar as foo} from 'bar'`,
    'registry/node_modules/bar/index.js': `export const bar = 123`,
    'node_modules/foo/index.js': { symlink: `../../registry/node_modules/foo/index.js` },
  }),
]

async function test(args, files) {
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
    await util.promisify(childProcess.execFile)(esbuildPath, args.concat('--outfile=out.js'), { cwd: thisTestDir, stdio: 'pipe' })

    // Run the resulting out.js file and make sure it exits cleanly
    await util.promisify(childProcess.exec)(`node "${path.join(thisTestDir, 'out.js')}"`, { cwd: thisTestDir, stdio: 'pipe' })
  }

  catch (e) {
    console.error(`❌ test failed: ${e && e.message || e}
  args: ${args.map(x => `\n    ${x}`).join('')}
  files: ${Object.entries(files).map(([k, v]) => `\n    ${k}: ${v}`).join('')}`)
    return false
  }

  return true
}

async function main() {
  // Make sure esbuild is built
  childProcess.execSync('make', { cwd: path.dirname(__dirname), stdio: 'pipe' })

  // Create a fresh test directory
  childProcess.execSync(`rm -fr "${testDir}"`)
  fs.mkdirSync(testDir)

  // Run all tests concurrently
  const allTestsPassed = (await Promise.all(tests())).every(success => success)

  // Clean up test output
  childProcess.execSync(`rm -fr "${testDir}"`)

  if (!allTestsPassed) {
    process.exit(1)
  }
}

main().catch(e => setTimeout(() => { throw e }))
