const childProcess = require('child_process')
const path = require('path')
const zlib = require('zlib')
const fs = require('fs')
const os = require('os')

const repoDir = path.dirname(__dirname)
const denoDir = path.join(repoDir, 'deno')
const npmDir = path.join(repoDir, 'npm', 'esbuild')
const version = fs.readFileSync(path.join(repoDir, 'version.txt'), 'utf8').trim()
const nodeTarget = 'node10'; // See: https://nodejs.org/en/about/releases/
const umdBrowserTarget = 'es2015'; // Transpiles "async"
const esmBrowserTarget = 'es2017'; // Preserves "async"

const buildNeutralLib = (esbuildPath) => {
  const libDir = path.join(npmDir, 'lib')
  const binDir = path.join(npmDir, 'bin')
  fs.mkdirSync(libDir, { recursive: true })
  fs.mkdirSync(binDir, { recursive: true })

  // Generate "npm/esbuild/install.js"
  childProcess.execFileSync(esbuildPath, [
    path.join(repoDir, 'lib', 'npm', 'node-install.ts'),
    '--outfile=' + path.join(npmDir, 'install.js'),
    '--bundle',
    '--target=' + nodeTarget,
    '--define:ESBUILD_VERSION=' + JSON.stringify(version),
    '--external:esbuild',
    '--platform=node',
    '--log-level=warning',
  ], { cwd: repoDir })

  // Generate "npm/esbuild/lib/main.js"
  childProcess.execFileSync(esbuildPath, [
    path.join(repoDir, 'lib', 'npm', 'node.ts'),
    '--outfile=' + path.join(libDir, 'main.js'),
    '--bundle',
    '--target=' + nodeTarget,
    '--define:WASM=false',
    '--define:ESBUILD_VERSION=' + JSON.stringify(version),
    '--external:esbuild',
    '--platform=node',
    '--log-level=warning',
  ], { cwd: repoDir })

  // Generate "npm/esbuild/bin/esbuild"
  childProcess.execFileSync(esbuildPath, [
    path.join(repoDir, 'lib', 'npm', 'node-shim.ts'),
    '--outfile=' + path.join(binDir, 'esbuild'),
    '--bundle',
    '--target=' + nodeTarget,
    '--external:esbuild',
    '--platform=node',
    '--log-level=warning',
  ], { cwd: repoDir })

  // Generate "npm/esbuild/lib/main.d.ts"
  const types_ts = fs.readFileSync(path.join(repoDir, 'lib', 'shared', 'types.ts'), 'utf8')
  fs.writeFileSync(path.join(libDir, 'main.d.ts'), types_ts)

  // Get supported platforms
  const platforms = { exports: {} }
  new Function('module', 'exports', 'require', childProcess.execFileSync(esbuildPath, [
    path.join(repoDir, 'lib', 'npm', 'node-platform.ts'),
    '--bundle',
    '--target=' + nodeTarget,
    '--external:esbuild',
    '--platform=node',
    '--log-level=warning',
  ], { cwd: repoDir }))(platforms, platforms.exports, require)
  const optionalDependencies = Object.fromEntries(Object.values({
    ...platforms.exports.knownWindowsPackages,
    ...platforms.exports.knownUnixlikePackages,
  }).sort().map(x => [x, version]))

  // Update "npm/esbuild/package.json"
  const pjPath = path.join(npmDir, 'package.json')
  const package_json = JSON.parse(fs.readFileSync(pjPath, 'utf8'))
  package_json.optionalDependencies = optionalDependencies
  fs.writeFileSync(pjPath, JSON.stringify(package_json, null, 2) + '\n')
}

exports.buildWasmLib = async (esbuildPath) => {
  // Asynchronously start building the WebAssembly module
  const npmWasmDir = path.join(repoDir, 'npm', 'esbuild-wasm')
  const goBuildPromise = new Promise((resolve, reject) => childProcess.execFile('go',
    [
      'build',
      '-o', path.join(npmWasmDir, 'esbuild.wasm'),
      '-ldflags=-s -w', // This removes ~0.14mb of unnecessary WebAssembly code
      path.join(repoDir, 'cmd', 'esbuild'),
    ],
    { cwd: repoDir, stdio: 'inherit', env: { ...process.env, GOOS: 'js', GOARCH: 'wasm' } },
    err => err ? reject(err) : resolve()))

  const libDir = path.join(npmWasmDir, 'lib')
  const esmDir = path.join(npmWasmDir, 'esm')
  fs.mkdirSync(libDir, { recursive: true })
  fs.mkdirSync(esmDir, { recursive: true })

  // Generate "npm/esbuild-wasm/wasm_exec.js"
  const toReplace = 'global.fs = fs;';
  const GOROOT = childProcess.execFileSync('go', ['env', 'GOROOT']).toString().trim();
  let wasm_exec_js = fs.readFileSync(path.join(GOROOT, 'misc', 'wasm', 'wasm_exec.js'), 'utf8');
  let index = wasm_exec_js.indexOf(toReplace);
  if (index === -1) throw new Error(`Failed to find ${JSON.stringify(toReplace)} in Go JS shim code`);
  wasm_exec_js = wasm_exec_js.replace(toReplace, `
    global.fs = Object.assign({}, fs, {
      // Hack around a Unicode bug in node: https://github.com/nodejs/node/issues/24550
      write(fd, buf, offset, length, position, callback) {
        if (offset === 0 && length === buf.length && position === null) {
          if (fd === process.stdout.fd) {
            try {
              process.stdout.write(buf, err => err ? callback(err, 0, null) : callback(null, length, buf));
            } catch (err) {
              callback(err, 0, null);
            }
            return;
          }
          if (fd === process.stderr.fd) {
            try {
              process.stderr.write(buf, err => err ? callback(err, 0, null) : callback(null, length, buf));
            } catch (err) {
              callback(err, 0, null);
            }
            return;
          }
        }
        fs.write(fd, buf, offset, length, position, callback);
      },
    });
  `);
  fs.writeFileSync(path.join(npmWasmDir, 'wasm_exec.js'), wasm_exec_js);

  // Generate "npm/esbuild-wasm/lib/main.js"
  childProcess.execFileSync(esbuildPath, [
    path.join(repoDir, 'lib', 'npm', 'node.ts'),
    '--outfile=' + path.join(libDir, 'main.js'),
    '--bundle',
    '--target=' + nodeTarget,
    '--format=cjs',
    '--define:WASM=true',
    '--define:ESBUILD_VERSION=' + JSON.stringify(version),
    '--external:esbuild',
    '--platform=node',
    '--log-level=warning',
  ], { cwd: repoDir })

  // Generate "npm/esbuild-wasm/lib/main.d.ts" and "npm/esbuild-wasm/lib/browser.d.ts"
  const types_ts = fs.readFileSync(path.join(repoDir, 'lib', 'shared', 'types.ts'), 'utf8')
  fs.writeFileSync(path.join(libDir, 'main.d.ts'), types_ts)
  fs.writeFileSync(path.join(libDir, 'browser.d.ts'), types_ts)
  fs.writeFileSync(path.join(esmDir, 'browser.d.ts'), types_ts)

  for (const minify of [false, true]) {
    const minifyFlags = minify ? ['--minify'] : []
    let wasmWorkerCode = {}

    for (const [format, target] of Object.entries({ umd: umdBrowserTarget, esm: esmBrowserTarget })) {
      // Process "npm/esbuild-wasm/wasm_exec.js"
      let wasmExecCode = wasm_exec_js;
      if (minify) {
        const wasmExecMin = childProcess.execFileSync(esbuildPath, [
          '--target=' + target,
        ].concat(minifyFlags), { cwd: repoDir, input: wasmExecCode }).toString()
        const commentLines = wasmExecCode.split('\n')
        const firstNonComment = commentLines.findIndex(line => !line.startsWith('//'))
        wasmExecCode = '\n' + commentLines.slice(0, firstNonComment).concat(wasmExecMin).join('\n')
      }

      // Process "lib/worker.ts"
      const workerCode = childProcess.execFileSync(esbuildPath, [
        path.join(repoDir, 'lib', 'npm', 'worker.ts'),
        '--target=' + target,
        '--define:ESBUILD_VERSION=' + JSON.stringify(version),
        '--log-level=warning',
      ].concat(minifyFlags), { cwd: repoDir }).toString().trim()

      wasmWorkerCode[format] = wasmExecCode + workerCode
    }

    // Generate "npm/esbuild-wasm/lib/browser.*"
    const umdPrefix = `(module=>{`
    const umdSuffix = `})(typeof module==="object"?module:{set exports(x){(typeof self!=="undefined"?self:this).esbuild=x}});`
    const browserCJS = childProcess.execFileSync(esbuildPath, [
      path.join(repoDir, 'lib', 'npm', 'browser.ts'),
      '--bundle',
      '--target=' + umdBrowserTarget,
      '--format=cjs',
      '--define:ESBUILD_VERSION=' + JSON.stringify(version),
      '--define:WEB_WORKER_SOURCE_CODE=' + JSON.stringify(wasmWorkerCode.umd),
      '--banner:js=' + umdPrefix,
      '--footer:js=' + umdSuffix,
      '--log-level=warning',
    ].concat(minifyFlags), { cwd: repoDir }).toString()
    fs.writeFileSync(path.join(libDir, minify ? 'browser.min.js' : 'browser.js'), browserCJS)

    // Generate "npm/esbuild-wasm/esm/browser.min.js"
    const browserESM = childProcess.execFileSync(esbuildPath, [
      path.join(repoDir, 'lib', 'npm', 'browser.ts'),
      '--bundle',
      '--target=' + esmBrowserTarget,
      '--format=esm',
      '--define:ESBUILD_VERSION=' + JSON.stringify(version),
      '--define:WEB_WORKER_SOURCE_CODE=' + JSON.stringify(wasmWorkerCode.esm),
      '--log-level=warning',
    ].concat(minifyFlags), { cwd: repoDir }).toString()
    fs.writeFileSync(path.join(esmDir, minify ? 'browser.min.js' : 'browser.js'), browserESM)
  }

  // Generate the "exit0" stubs
  const exit0Map = {};
  const exit0Dir = path.join(repoDir, 'lib', 'npm', 'exit0');
  for (const entry of fs.readdirSync(exit0Dir)) {
    if (entry.endsWith('.node')) {
      const absPath = path.join(exit0Dir, entry);
      const compressed = zlib.deflateRawSync(fs.readFileSync(absPath), { level: 9 });
      exit0Map[entry] = compressed.toString('base64');
    }
  }
  fs.writeFileSync(path.join(npmWasmDir, 'exit0.js'), `
// Each of these is a native module that calls "exit(0)". This is a workaround
// for https://github.com/nodejs/node/issues/36616. These native modules are
// stored in a string both to make them smaller and to hide them from Yarn 2,
// since they make Yarn 2 unzip this package.

module.exports = ${JSON.stringify(exit0Map, null, 2)};
`);

  // Join with the asynchronous WebAssembly build
  await goBuildPromise;
}

const buildDenoLib = (esbuildPath) => {
  // Generate "deno/esbuild/mod.js"
  childProcess.execFileSync(esbuildPath, [
    path.join(repoDir, 'lib', 'deno', 'mod.ts'),
    '--bundle',
    '--outfile=' + path.join(denoDir, 'mod.js'),
    '--target=es2020',
    '--define:ESBUILD_VERSION=' + JSON.stringify(version),
    '--platform=neutral',
    '--log-level=warning',
    '--banner:js=/// <reference types="./mod.d.ts" />',
  ], { cwd: repoDir })

  // Generate "deno/esbuild/mod.d.ts"
  const types_ts = fs.readFileSync(path.join(repoDir, 'lib', 'shared', 'types.ts'), 'utf8')
  fs.writeFileSync(path.join(denoDir, 'mod.d.ts'), types_ts +
    `\n// Unlike node, Deno lacks the necessary APIs to clean up child processes` +
    `\n// automatically. You must manually call stop() in Deno when you're done` +
    `\n// using esbuild or Deno will continue running forever.` +
    `\nexport function stop(): void;` +
    `\n`)
}

// Writing a file atomically is important for watch mode tests since we don't
// want to read the file after it has been truncated but before the new contents
// have been written.
exports.writeFileAtomic = (where, contents) => {
  // Note: Can't use "os.tmpdir()" because that doesn't work on Windows. CI runs
  // tests on D:\ and the temporary directory is on C:\ or the other way around.
  // And apparently it's impossible to move files between C:\ and D:\ or something.
  // So we have to write the file in the same directory as the destination. This is
  // unfortunate because it will unnecessarily trigger extra watch mode rebuilds.
  // So we have to make our tests extra robust so they can still work with random
  // extra rebuilds thrown in.
  const file = path.join(path.dirname(where), '.esbuild-atomic-file-' + Math.random().toString(36).slice(2))
  fs.writeFileSync(file, contents)
  fs.renameSync(file, where)
}

exports.buildBinary = () => {
  childProcess.execFileSync('go', ['build', '-ldflags=-s -w', './cmd/esbuild'], { cwd: repoDir, stdio: 'ignore' })
  return path.join(repoDir, process.platform === 'win32' ? 'esbuild.exe' : 'esbuild')
}

exports.removeRecursiveSync = path => {
  try {
    fs.rmSync(path, { recursive: true })
  } catch (e) {
    // Removing stuff on Windows is flaky and unreliable. Don't fail tests
    // on CI if Windows is just being a pain. Common causes of flakes include
    // random EPERM and ENOTEMPTY errors.
    //
    // The general "solution" to this is to try asking Windows to redo the
    // failing operation repeatedly until eventually giving up after a
    // timeout. But that doesn't guarantee that flakes will be fixed so we
    // just give up instead. People that want reasonable file system
    // behavior on Windows should use WSL instead.
  }
}

const updateVersionPackageJSON = pathToPackageJSON => {
  const version = fs.readFileSync(path.join(path.dirname(__dirname), 'version.txt'), 'utf8').trim()
  const json = JSON.parse(fs.readFileSync(pathToPackageJSON, 'utf8'))
  if (json.version !== version) {
    json.version = version
    fs.writeFileSync(pathToPackageJSON, JSON.stringify(json, null, 2) + '\n')
  }
}

exports.installForTests = () => {
  // Build the "esbuild" binary and library
  const esbuildPath = exports.buildBinary()
  buildNeutralLib(esbuildPath)

  // Install the "esbuild" package to a temporary directory. On Windows, it's
  // sometimes randomly impossible to delete this installation directory. My
  // best guess is that this is because the esbuild process is kept alive until
  // the process exits for "buildSync" and "transformSync", and that sometimes
  // prevents Windows from deleting the directory it's in. The call in tests to
  // "rimraf.sync()" appears to hang when this happens. Other operating systems
  // don't have a problem with this. This has only been a problem on the Windows
  // VM in GitHub CI. I cannot reproduce this issue myself.
  const installDir = path.join(os.tmpdir(), 'esbuild-' + Math.random().toString(36).slice(2))
  const env = { ...process.env, ESBUILD_BINARY_PATH: esbuildPath }
  fs.mkdirSync(installDir)
  fs.writeFileSync(path.join(installDir, 'package.json'), '{}')
  childProcess.execSync(`npm pack --silent "${npmDir}"`, { cwd: installDir, stdio: 'inherit' })
  childProcess.execSync(`npm install --silent --no-audit --progress=false esbuild-${version}.tgz`, { cwd: installDir, env, stdio: 'inherit' })

  // Evaluate the code
  const ESBUILD_PACKAGE_PATH = path.join(installDir, 'node_modules', 'esbuild')
  const mod = require(ESBUILD_PACKAGE_PATH)
  Object.defineProperty(mod, 'ESBUILD_PACKAGE_PATH', { value: ESBUILD_PACKAGE_PATH })
  return mod
}

// This is helpful for ES6 modules which don't have access to __dirname
exports.dirname = __dirname

// The main Makefile invokes this script before publishing
if (require.main === module) {
  if (process.argv.indexOf('--wasm') >= 0) exports.buildWasmLib(process.argv[2])
  else if (process.argv.indexOf('--deno') >= 0) buildDenoLib(process.argv[2])
  else if (process.argv.indexOf('--version') >= 0) updateVersionPackageJSON(process.argv[2])
  else if (process.argv.indexOf('--neutral') >= 0) buildNeutralLib(process.argv[2])
  else throw new Error('Expected a flag')
}
