const childProcess = require('child_process')
const rimraf = require('rimraf')
const path = require('path')
const fs = require('fs')

const repoDir = path.dirname(__dirname)
const npmDir = path.join(repoDir, 'npm', 'esbuild')

function buildNativeLib(esbuildPath) {
  const libDir = path.join(npmDir, 'lib')
  try {
    fs.mkdirSync(libDir)
  } catch (e) {
  }

  // Generate "npm/esbuild/lib/main.js"
  childProcess.execFileSync(esbuildPath, [
    '--bundle',
    path.join(repoDir, 'lib', 'api-node.ts'),
    '--outfile=' + path.join(libDir, 'main.js'),
    '--format=cjs',
    '--define:WASM=false',
    '--platform=node',
  ], { cwd: repoDir })

  // Generate "npm/esbuild/lib/main.d.ts"
  fs.copyFileSync(
    path.join(repoDir, 'lib', 'api-types.ts'),
    path.join(libDir, 'main.d.ts'),
  )
}

function buildWasmLib(esbuildPath) {
  const npmWasmDir = path.join(repoDir, 'npm', 'esbuild-wasm')
  const libDir = path.join(npmWasmDir, 'lib')
  try {
    fs.mkdirSync(libDir)
  } catch (e) {
  }

  // Generate "npm/esbuild-wasm/lib/main.js"
  childProcess.execFileSync(esbuildPath, [
    '--bundle',
    path.join(repoDir, 'lib', 'api-node.ts'),
    '--outfile=' + path.join(libDir, 'main.js'),
    '--format=cjs',
    '--define:WASM=true',
    '--platform=node',
  ], { cwd: repoDir })

  // Generate "npm/esbuild-wasm/lib/main.d.ts"
  fs.copyFileSync(
    path.join(repoDir, 'lib', 'api-types.ts'),
    path.join(libDir, 'main.d.ts'),
  )

  // Generate "npm/esbuild-wasm/browser.js"
  const wasm_exec_js = path.join(npmWasmDir, 'wasm_exec.js')
  childProcess.execFileSync(esbuildPath, [
    '--bundle',
    path.join(repoDir, 'lib', 'api-browser.ts'),
    '--outfile=' + path.join(libDir, 'browser.js'),
    '--format=cjs',
    '--define:WASM_EXEC_JS=' + JSON.stringify(fs.readFileSync(wasm_exec_js, 'utf8')),
  ], { cwd: repoDir })
}

exports.buildBinary = () => {
  childProcess.execSync(`go build ./cmd/esbuild`, { cwd: repoDir, stdio: 'ignore' })
  return path.join(repoDir, process.platform === 'win32' ? 'esbuild.exe' : 'esbuild')
}

exports.installForTests = dir => {
  // Create a fresh test directory
  rimraf.sync(dir, { disableGlob: true })
  fs.mkdirSync(dir)

  // Build the "esbuild" binary and library
  const esbuildPath = exports.buildBinary()
  buildNativeLib(esbuildPath)

  // Install the "esbuild" package
  const env = { ...process.env, ESBUILD_BIN_PATH_FOR_TESTS: esbuildPath }
  const version = require(path.join(npmDir, 'package.json')).version
  fs.writeFileSync(path.join(dir, 'package.json'), '{}')
  childProcess.execSync(`npm pack --silent "${npmDir}"`, { cwd: dir, stdio: 'inherit' })
  childProcess.execSync(`npm install --silent --no-audit --progress=false esbuild-${version}.tgz`, { cwd: dir, env, stdio: 'inherit' })

  // Evaluate the code
  return require(path.join(dir, 'node_modules', 'esbuild'))
}

// This is helpful for ES6 modules which don't have access to __dirname
exports.dirname = __dirname

// The main Makefile invokes this script before publishing
if (require.main === module) {
  if (process.argv.indexOf('--wasm') >= 0) {
    buildWasmLib(process.argv[2])
  } else {
    buildNativeLib(process.argv[2])
  }
}
