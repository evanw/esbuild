const childProcess = require('child_process')
const path = require('path')
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
]

function makeAsync(cb) {
  return new Promise((resolve, reject) => {
    cb(error => {
      if (error) reject(error)
      else resolve()
    })
  })
}

async function test(args, files) {
  const thisTestDir = path.join(testDir, '' + testCount++)

  // Test setup
  await makeAsync(cb => fs.mkdir(thisTestDir, cb))
  for (const file in files) {
    await makeAsync(cb => fs.writeFile(path.join(thisTestDir, file), files[file], cb))
  }

  // Run esbuild
  await makeAsync(cb => childProcess.execFile(esbuildPath, args.concat('--outfile=out.js'), { cwd: thisTestDir, stdio: 'pipe' }, cb))

  // Run the resulting out.js file and make sure it exits cleanly
  try {
    await makeAsync(cb => childProcess.exec(`node "${path.join(thisTestDir, 'out.js')}"`, { cwd: thisTestDir, stdio: 'pipe' }, cb))
  } catch (e) {
    console.error(`❌ test failed:
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
  childProcess.execSync(`rm -fr "${testDir}"`, { cwd: __dirname })
  fs.mkdirSync(testDir)

  // Run all tests concurrently
  const allTestsPassed = (await Promise.all(tests())).every(success => success)

  // Clean up test output
  childProcess.execSync(`rm -fr "${testDir}"`, { cwd: __dirname })

  if (!allTestsPassed) {
    process.exit(1)
  }
}

main().catch(e => setTimeout(() => { throw e }))
