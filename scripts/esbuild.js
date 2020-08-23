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

  // Generate "npm/esbuild/install.js"
  childProcess.execFileSync(esbuildPath, [
    path.join(repoDir, 'lib', 'install.ts'),
    '--outfile=' + path.join(npmDir, 'install.js'),
    '--target=es2015',
    '--platform=node',
  ], { cwd: repoDir })

  // Generate "npm/esbuild/lib/main.js"
  childProcess.execFileSync(esbuildPath, [
    path.join(repoDir, 'lib', 'node.ts'),
    '--outfile=' + path.join(libDir, 'main.js'),
    '--bundle',
    '--target=es2015',
    '--format=cjs',
    '--define:WASM=false',
    '--platform=node',
  ], { cwd: repoDir })

  // Generate "npm/esbuild/lib/main.d.ts"
  const types_ts = fs.readFileSync(path.join(repoDir, 'lib', 'types.ts'), 'utf8')
  fs.writeFileSync(path.join(libDir, 'main.d.ts'), types_ts)
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
    path.join(repoDir, 'lib', 'node.ts'),
    '--outfile=' + path.join(libDir, 'main.js'),
    '--bundle',
    '--target=es2015',
    '--format=cjs',
    '--define:WASM=true',
    '--platform=node',
  ], { cwd: repoDir })

  // Generate "npm/esbuild-wasm/lib/main.d.ts" and "npm/esbuild-wasm/lib/browser.d.ts"
  const types_ts = fs.readFileSync(path.join(repoDir, 'lib', 'types.ts'), 'utf8')
  fs.writeFileSync(path.join(libDir, 'main.d.ts'), types_ts)
  fs.writeFileSync(path.join(libDir, 'browser.d.ts'), types_ts)

  // Minify "npm/esbuild-wasm/wasm_exec.js"
  const wasm_exec_js = path.join(npmWasmDir, 'wasm_exec.js')
  const wasmExecMin = childProcess.execFileSync(esbuildPath, [
    wasm_exec_js,
    '--minify',
  ], { cwd: repoDir }).toString()
  const commentLines = fs.readFileSync(wasm_exec_js, 'utf8').split('\n')
  const firstNonComment = commentLines.findIndex(line => !line.startsWith('//'))
  const wasmExecMinCode = '\n' + commentLines.slice(0, firstNonComment).concat(wasmExecMin).join('\n')

  // Minify "lib/worker.ts"
  const workerMinCode = childProcess.execFileSync(esbuildPath, [
    path.join(repoDir, 'lib', 'worker.ts'),
    '--minify',
  ], { cwd: repoDir }).toString().trim()

  // Generate "npm/esbuild-wasm/browser.js"
  const umdPrefix = `Object.assign(typeof exports==="object"?exports:(typeof self!=="undefined"?self:this).esbuild={},(module=>{`
  const umdSuffix = `return module})({}).exports);\n`
  const browserJs = childProcess.execFileSync(esbuildPath, [
    path.join(repoDir, 'lib', 'browser.ts'),
    '--bundle',
    '--target=es2015',
    '--minify',
    '--format=cjs',
    '--define:WEB_WORKER_SOURCE_CODE=' + JSON.stringify(wasmExecMinCode + workerMinCode),
  ], { cwd: repoDir }).toString()
  fs.writeFileSync(path.join(libDir, 'browser.js'), umdPrefix + browserJs.trim() + umdSuffix)

  // Generate "npm/esbuild-wasm/browser.mjs"
  const browserMjs = childProcess.execFileSync(esbuildPath, [
    path.join(repoDir, 'lib', 'browser.ts'),
    '--bundle',
    '--target=es2020',
    '--format=esm',
    '--define:WEB_WORKER_SOURCE_CODE=' + JSON.stringify(wasmExecMinCode + workerMinCode),
  ], { cwd: repoDir }).toString()
  fs.writeFileSync(path.join(libDir, 'browser.mjs'), browserMjs.trim())
}

exports.buildBinary = () => {
  childProcess.execFileSync('go', ['build', '-ldflags=-s -w', './cmd/esbuild'], { cwd: repoDir, stdio: 'ignore' })
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
