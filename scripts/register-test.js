const { installForTests } = require('./esbuild')
const child_process = require('child_process')
const path = require('path')
const fs = require('fs')
const rimraf = require('rimraf')
const assert = require('assert')
const esbuild = installForTests()

// Create a fresh test directory
const rootTestDir = path.join(__dirname, '.register-test')
rimraf.sync(rootTestDir, { disableGlob: true })
fs.mkdirSync(rootTestDir)

const entry = path.join(rootTestDir, 'entry.ts')
fs.writeFileSync(entry, `
  console.log('worked' as string)
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
  assert.strictEqual(result, `worked\n`)
}

main().then(
  () => {
    console.log(`✅ register test passed`)
    try {
      rimraf.sync(rootTestDir, { disableGlob: true })
    } catch (e) {
      // This doesn't work on Windows due to "EPERM: operation not permitted"
      // but that's ok for CI because the VM will just be thrown away anyway.
    }
  },
  e => {
    console.error(`❌ register test failed: ${e && e.message || e}`)
    process.exit(1)
  },
)
