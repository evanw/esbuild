const { removeRecursiveSync, buildWasmLib } = require('./esbuild.js');
const child_process = require('child_process');
const assert = require('assert');
const path = require('path');
const fs = require('fs');

const tests = {
  basicStdinTest({ testDir, esbuildPath }) {
    const stdout = child_process.execFileSync('node', [
      esbuildPath,
      '--format=cjs',
    ], {
      stdio: ['pipe', 'pipe', 'inherit'],
      cwd: testDir,
      input: `export default 1+2`,
    }).toString();

    // Check that the bundle is valid
    const module = { exports: {} };
    new Function('module', 'exports', stdout)(module, module.exports);
    assert.deepStrictEqual(module.exports.default, 3);
  },

  stdinOutfileTest({ testDir, esbuildPath }) {
    const outfile = path.join(testDir, 'out.js')
    child_process.execFileSync('node', [
      esbuildPath,
      '--bundle',
      '--format=cjs',
      '--outfile=' + outfile,
    ], {
      stdio: ['pipe', 'pipe', 'inherit'],
      cwd: testDir,
      input: `export default 1+2`,
    }).toString();

    // Check that the bundle is valid
    const exports = require(outfile);
    assert.deepStrictEqual(exports.default, 3);
  },

  importRelativeFileTest({ testDir, esbuildPath }) {
    const outfile = path.join(testDir, 'out.js')
    const packageJSON = path.join(__dirname, '..', 'npm', 'esbuild-wasm', 'package.json');
    child_process.execFileSync('node', [
      esbuildPath,
      '--bundle',
      '--format=cjs',
      '--outfile=' + outfile,
    ], {
      stdio: ['pipe', 'pipe', 'inherit'],
      cwd: testDir,
      input: `export {default} from ` + JSON.stringify('./' + path.relative(testDir, packageJSON)),
    }).toString();

    // Check that the bundle is valid
    const exports = require(outfile);
    assert.deepStrictEqual(exports.default, require(packageJSON));
  },

  importAbsoluteFileTest({ testDir, esbuildPath }) {
    const outfile = path.join(testDir, 'out.js')
    const packageJSON = path.join(__dirname, '..', 'npm', 'esbuild-wasm', 'package.json');
    child_process.execFileSync('node', [
      esbuildPath,
      '--bundle',
      '--format=cjs',
      '--outfile=' + outfile,
    ], {
      stdio: ['pipe', 'pipe', 'inherit'],
      cwd: testDir,
      input: `export {default} from ` + JSON.stringify(packageJSON),
    }).toString();

    // Check that the bundle is valid
    const exports = require(outfile);
    assert.deepStrictEqual(exports.default, require(packageJSON));
  },
};

function runTest({ testDir, esbuildPath, test }) {
  try {
    fs.mkdirSync(testDir, { recursive: true })
    test({ testDir, esbuildPath })
    return true
  } catch (e) {
    console.error(`❌ ${test.name} failed: ${e && e.message || e}`)
    return false
  }
}

async function main() {
  // Generate the WebAssembly module
  await buildWasmLib(path.join(__dirname, '..', process.platform === 'win32' ? 'esbuild.exe' : 'esbuild'));

  const esbuildPath = path.join(__dirname, '..', 'npm', 'esbuild-wasm', 'bin', 'esbuild');
  const testDir = path.join(__dirname, '.wasm-tests')

  // Run all tests in serial because WebAssembly compilation is a CPU hog
  let allTestsPassed = true;
  for (const test in tests) {
    if (!runTest({
      testDir: path.join(testDir, test),
      test: tests[test],
      esbuildPath,
    })) {
      allTestsPassed = false;
    }
  }

  if (!allTestsPassed) {
    console.error(`❌ wasm-tests failed`)
    process.exit(1)
  } else {
    console.log(`✅ wasm-tests passed`)
    removeRecursiveSync(testDir)
  }
}

main().catch(e => setTimeout(() => { throw e }))
