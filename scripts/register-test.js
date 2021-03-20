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

async function main() {
  let result
  let promise = new Promise((resolve, reject) => child_process.execFile('node', ['-r', register, entry], (err, stdout) => {
    if (err) {
      reject(err)
    } else {
      result = stdout
      resolve()
    }
  }))
  let timeout
  let wait = new Promise((_, reject) => {
    timeout = setTimeout(() => reject(new Error('This test timed out')), 60 * 1000)
  })
  await Promise.race([promise, wait])
  clearTimeout(timeout)
  assert.strictEqual(result, `in entry.ts\nin other.ts\n`)
}

main().then(
  () => {
    console.log(`✅ register test passed`)
    removeRecursiveSync(rootTestDir)
  },
  e => {
    console.error(`❌ register test failed: ${e && e.message || e}`)
    process.exit(1)
  },
)
