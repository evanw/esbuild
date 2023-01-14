// This test verifies that:
//  - a running service will not prevent NodeJS to exit if there is no compilation in progress.
//  - the NodeJS process will continue running if there is a serve() active or a transform or build in progress.

const assert = require('assert')
const { fork } = require('child_process');

// The tests to run in the child process
async function tests() {
  const esbuild = require('./esbuild').installForTests()

  async function testTransform() {
    const t1 = await esbuild.transform(`1+2`)
    const t2 = await esbuild.transform(`1+3`)
    assert.strictEqual(t1.code, `1 + 2;\n`)
    assert.strictEqual(t2.code, `1 + 3;\n`)
  }

  async function testServe() {
    const context = await esbuild.context({})
    try {
      const server = await context.serve({})
      assert.strictEqual(server.host, '0.0.0.0')
      assert.strictEqual(typeof server.port, 'number')
    } finally {
      await context.dispose()
    }
  }

  async function testBuild() {
    const context = await esbuild.context({
      stdin: { contents: '1+2' },
      write: false,
    })
    try {

      const result = await context.rebuild()
      assert.deepStrictEqual(result.outputFiles.length, 1);
      assert.deepStrictEqual(result.outputFiles[0].text, '1 + 2;\n');

      const result2 = await context.rebuild()
      assert.deepStrictEqual(result2.outputFiles.length, 1);
      assert.deepStrictEqual(result2.outputFiles[0].text, '1 + 2;\n');
    } finally {
      await context.dispose()
    }
  }

  async function testWatchAndIncremental() {
    const context = await esbuild.context({
      stdin: { contents: '1+2' },
      write: false,
    })
    try {
      await context.watch()

      const result = await context.rebuild()
      assert.deepStrictEqual(result.outputFiles.length, 1);
      assert.deepStrictEqual(result.outputFiles[0].text, '1 + 2;\n');

    } finally {
      await context.dispose()
    }
  }

  await testTransform()
  await testServe()
  await testBuild()
  await testWatchAndIncremental()
}

// Called when this is the child process to run the tests.
function runTests() {
  process.exitCode = 1;
  tests().then(() => {
    process.exitCode = 0;
  }, (error) => {
    console.error('❌', error)
  });
}

// A child process need to be started to verify that a running service is not hanging node.
function startChildProcess() {
  const child = fork(__filename, ['__forked__'], { stdio: 'inherit', env: process.env });

  const timeout = setTimeout(() => {
    console.error('❌ node unref test timeout - child_process.unref() broken?')
    process.exit(1);
  }, 5 * 60 * 1000);

  child.on('error', (error) => {
    console.error('❌', error);
    process.exit(1);
  })

  child.on('exit', (code) => {
    clearTimeout(timeout);
    if (code) {
      console.error(`❌ node unref tests failed: child exited with code ${code}`)
      process.exit(1);
    } else {
      console.log(`✅ node unref tests passed`)
    }
  })
}

if (process.argv[2] === '__forked__') {
  runTests();
} else {
  startChildProcess();
}
