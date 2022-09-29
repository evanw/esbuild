const child_process = require('child_process')
const esbuild = require('./esbuild')
const path = require('path')
const fs = require('fs')

const ESBUILD_BINARY_PATH = esbuild.buildBinary()
const rootDir = path.join(__dirname, '..', 'require', 'yarnpnp')

function run(command) {
  console.log('\n\033[37m' + '$ ' + command + '\033[0m')
  child_process.execSync(command, { cwd: rootDir, stdio: 'inherit' })
}

function modTime(file) {
  return fs.statSync(file).mtimeMs
}

function reinstallYarnIfNeeded() {
  const yarnPath = path.join(rootDir, '.yarn', 'releases', 'yarn-4.0.0-rc.22.cjs')

  if (fs.existsSync(yarnPath) && modTime(yarnPath) > Math.max(
    modTime(path.join(rootDir, 'package.json')),
    modTime(path.join(rootDir, 'yarn.lock')),
  )) {
    return
  }

  fs.rmSync(path.join(rootDir, '.pnp.cjs'), { recursive: true, force: true })
  fs.rmSync(path.join(rootDir, '.pnp.loader.mjs'), { recursive: true, force: true })
  fs.rmSync(path.join(rootDir, '.yarn'), { recursive: true, force: true })
  fs.rmSync(path.join(rootDir, '.yarnrc.yml'), { recursive: true, force: true })

  try {
    run('yarn set version 4.0.0-rc.22')
  } catch {
    run('npm i -g yarn') // Install Yarn globally if it's not already installed
    run('yarn set version 4.0.0-rc.22')
  }

  const rc = fs.readFileSync(path.join(rootDir, '.yarnrc.yml'), 'utf8')
  fs.writeFileSync(path.join(rootDir, '.yarnrc.yml'), `
pnpEnableEsmLoader: true
pnpIgnorePatterns: ["./bar/**"]

# Note: Yarn 4 defaults to "enableGlobalCache: true" which doesn't
# work on Windows due to cross-drive issues with relative paths.
# Explicitly set "enableGlobalCache: false" to avoid this issue.
enableGlobalCache: false

` + rc)

  run('yarn install')
}

function runTests() {
  // Make sure the tests are valid
  run('yarn node in.mjs')

  // Test the native build
  child_process.execFileSync(ESBUILD_BINARY_PATH, [
    'in.mjs',
    '--bundle',
    '--log-level=debug',
    '--platform=node',
    '--outfile=out-native.js',
  ], { cwd: rootDir, stdio: 'inherit' })
  run('node out-native.js')

  // Test the WebAssembly build
  esbuild.buildWasmLib(ESBUILD_BINARY_PATH)
  run('node ../../npm/esbuild-wasm/bin/esbuild in.mjs --bundle --log-level=debug --platform=node --outfile=out-wasm.js')
  run('node out-wasm.js')

  // Test the WebAssembly build when run through Yarn's file system shim
  esbuild.buildWasmLib(ESBUILD_BINARY_PATH)
  run('yarn node ../../npm/esbuild-wasm/bin/esbuild in.mjs --bundle --log-level=debug --platform=node --outfile=out-wasm-yarn.js')
  run('node out-wasm-yarn.js')
}

reinstallYarnIfNeeded()
runTests()
