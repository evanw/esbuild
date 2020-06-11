// This fuzzer attempts to find issues with the "scope hoisting" optimizations
// for ES6-style imports and exports. It compares esbuild's behavior with the
// behavior of node's experimental module support.

(async () => {
  // Make sure this script runs as an ES6 module so we can import both ES6 modules and CommonJS modules
  if (typeof require !== 'undefined') {
    const childProcess = require('child_process')
    const child = childProcess.spawn('node', ['--experimental-modules', '--input-type=module'], {
      cwd: __dirname,
      stdio: ['pipe', 'inherit', 'inherit'],
    })
    child.stdin.write(require('fs').readFileSync(__filename))
    child.stdin.end()
    child.on('close', code => process.exit(code))
    return
  }

  const { default: { buildBinary, dirname } } = await import('./esbuild.js');
  const { default: rimraf } = await import('rimraf');
  const childProcess = await import('child_process');
  const util = await import('util');
  const path = await import('path');
  const fs = await import('fs');
  const esbuildPath = buildBinary();
  let failureCount = 0;
  let nextTest = 0;

  function reportFailure(testDir, files, kind, error) {
    failureCount++;
    console.log(`❌ FAILURE ${kind}: ${error}\n  DIR: ${testDir}` +
      Object.keys(files).map(x => `\n  ${x} => ${files[x]}`).join(''));
  }

  function circularObjectToString(root) {
    let map = new Map();
    let counter = 0;
    let visit = obj => {
      if (typeof obj !== 'object') return JSON.stringify(obj);
      if (map.has(obj)) return `$${map.get(obj)}`;
      map.set(obj, counter++);
      const keys = Object.keys(obj).sort();
      return `$${map.get(obj)} = {${keys.map(key =>
        `${JSON.stringify(key)}: ${visit(obj[key])}`).join(', ')}}`;
    };
    return visit(root);
  }

  function checkSameExportObject(a, b) {
    a = circularObjectToString(a);
    b = circularObjectToString(b);
    if (a !== b) throw new Error(`Different exports:\n  ${a}\n  ${b}`);
  }

  async function fuzzOnce(parentDir) {
    const mjs_or_cjs = () => Math.random() < 0.1 ? 'cjs' : 'mjs';
    const names = [
      'a.' + mjs_or_cjs(),
      'b.' + mjs_or_cjs(),
      'c.' + mjs_or_cjs(),
      'd.' + mjs_or_cjs(),
      'e.' + mjs_or_cjs(),
    ];
    const randomName = () => names[Math.random() * names.length | 0];
    const files = {};

    for (const name of names) {
      if (name.endsWith('.cjs')) {
        files[name] = `module.exports = 123`;
      } else {
        switch (Math.random() * 5 | 0) {
          case 0:
            files[name] = `export const foo = 123`;
            break;
          case 1:
            files[name] = `export default 123`;
            break;
          case 2:
            files[name] = `export * from "./${randomName()}"`;
            break;
          case 3:
            files[name] = `export * as foo from "./${randomName()}"`;
            break;
          case 4:
            files[name] = `import * as foo from "./${randomName()}"; export {foo}`;
            break;
        }
      }
    }

    // Write the files to the file system
    const testDir = path.join(parentDir, (nextTest++).toString());
    fs.mkdirSync(testDir);
    for (const name in files) {
      fs.writeFileSync(path.join(testDir, name), files[name]);
    }
    if (nextTest % 100 === 0) console.log(`Checked ${nextTest} test cases`);

    // Load the raw module using node
    const entryPoint = path.join(testDir, names[0]);
    let realExports = await import(entryPoint);
    if (entryPoint.endsWith('.cjs')) realExports = realExports.default;

    // Bundle to a CommonJS module using esbuild
    const cjsFile = path.join(testDir, 'out.cjs');
    await util.promisify(childProcess.execFile)(esbuildPath, [
      '--bundle',
      '--outfile=' + cjsFile,
      '--format=cjs',
      entryPoint,
    ], { stdio: 'pipe' });

    // Validate the CommonJS module bundle
    try {
      let { default: cjsExports } = await import(cjsFile);
      checkSameExportObject(realExports, cjsExports);
    } catch (e) {
      reportFailure(testDir, files, 'cjs', e + '');
      return;
    }

    // Remove data for successful tests
    rimraf.sync(testDir, { disableGlob: true });
  }

  const parentDir = path.join(dirname, '.es6-fuzzer');
  rimraf.sync(parentDir, { disableGlob: true });
  fs.mkdirSync(parentDir);

  // Run a set number of tests in parallel
  let promises = [];
  for (let i = 0; i < 10; i++) {
    let promise = fuzzOnce(parentDir);
    for (let j = 0; j < 100; j++) {
      promise = promise.then(() => fuzzOnce(parentDir));
    }
    promises.push(promise);
  }
  await Promise.all(promises);

  // Remove everything if all tests passed
  if (failureCount === 0) {
    rimraf.sync(parentDir, { disableGlob: true });
  }
})().catch(e => setTimeout(() => { throw e }));
