const { installForTests, removeRecursiveSync } = require('./esbuild')
const child_process = require('child_process')
const path = require('path')
const fs = require('fs')
const assert = require('assert')
const esbuild = installForTests()

// Create a fresh test directory
const rootTestDir = path.join(__dirname, '.register-test')
removeRecursiveSync(rootTestDir)
fs.mkdirSync(rootTestDir)

const entry = path.join(rootTestDir, 'entry.ts')
fs.writeFileSync(entry, `
  console.log('in entry.ts' as string)
  require('./other.ts')
`)

const other = path.join(rootTestDir, 'other.ts')
fs.writeFileSync(other, `
  console.log('in other.ts' as string)
`)

const register = path.join(rootTestDir, 'register.js')
fs.writeFileSync(register, `
  const esbuild = require(${JSON.stringify(esbuild.ESBUILD_PACKAGE_PATH)});
  const fs = require('fs');
  require.extensions['.ts'] = (mod, filename) => {
    const ts = fs.readFileSync(filename, 'utf8');
    const { code } = esbuild.transformSync(ts, { loader: 'ts' });
    mod._compile(code, filename);
  };
`)

let tests = {
  async fromMainThread() {
    let result = await new Promise((resolve, reject) => child_process.execFile('node', ['-r', register, entry], (err, stdout) => {
      if (err) reject(err)
      else resolve(stdout)
    }))
    assert.strictEqual(result, `in entry.ts\nin other.ts\n`)
  },
}

async function main() {
  // Time out these tests after 5 minutes. This exists to help debug test hangs in CI.
  let minutes = 5
  let timeout = setTimeout(() => {
    console.error(`❌ register tests timed out after ${minutes} minutes, exiting...`)
    process.exit(1)
  }, minutes * 60 * 1000)

  const runTest = async ([name, fn]) => {
    let testDir = path.join(rootTestDir, name)
    try {
      fs.mkdirSync(testDir)
      await fn({ esbuild, testDir })
      removeRecursiveSync(testDir)
      return true
    } catch (e) {
      console.error(`❌ ${name}: ${e && e.message || e}`)
      return false
    }
  }

  // Run all tests in serial
  let allTestsPassed = true
  for (let test of Object.entries(tests)) {
    if (!await runTest(test)) {
      allTestsPassed = false
    }
  }

  if (!allTestsPassed) {
    console.error(`❌ register tests failed`)
    process.exit(1)
  } else {
    console.log(`✅ register tests passed`)
    removeRecursiveSync(rootTestDir)
  }

  clearTimeout(timeout);
}

main().catch(e => setTimeout(() => { throw e }))
