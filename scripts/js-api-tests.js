const { installForTests } = require('./esbuild')
const childProcess = require('child_process')
const assert = require('assert')
const slash = require('slash')
const path = require('path')
const util = require('util')
const fs = require('fs')

const repoDir = path.dirname(__dirname)
const testDir = path.join(repoDir, 'scripts', '.js-api-tests')

let tests = {
  async build({ esbuild }) {
    const input = `.${slash(path.join(testDir, 'input.js'))}`
    const output = `.${slash(path.join(testDir, 'output.js'))}`
    await util.promisify(fs.writeFile)(input, 'export default 123')
    await esbuild.build({ entryPoints: [input], bundle: true, outfile: output, format: 'cjs' })
    const result = require(output)
    assert.strictEqual(result.default, 123)
    assert.strictEqual(result.__esModule, true)
  },

  async jsx({ service }) {
    const { js } = await service.transform(`console.log(<div/>)`, { loader: 'jsx' })
    assert.strictEqual(js, `console.log(React.createElement("div", null));\n`)
  },

  async ts({ service }) {
    const { js } = await service.transform(`enum Foo { FOO }`, { loader: 'ts' })
    assert.strictEqual(js, `var Foo;\n(function(Foo2) {\n  Foo2[Foo2["FOO"] = 0] = "FOO";\n})(Foo || (Foo = {}));\n`)
  },

  async tsx({ service }) {
    const { js } = await service.transform(`console.log(<Foo<T>/>)`, { loader: 'tsx' })
    assert.strictEqual(js, `console.log(React.createElement(Foo, null));\n`)
  },

  async minify({ service }) {
    const { js } = await service.transform(`console.log("a" + "b" + c)`, { minify: true })
    assert.strictEqual(js, `console.log("ab"+c);\n`)
  },

  async define({ service }) {
    const define = { 'process.env.NODE_ENV': '"production"' }
    const { js } = await service.transform(`console.log(process.env.NODE_ENV)`, { define })
    assert.strictEqual(js, `console.log("production");\n`)
  },

  async json({ service }) {
    const { js } = await service.transform(`{ "x": "y" }`, { loader: 'json' })
    assert.strictEqual(js, `module.exports = {\n  x: "y"\n};\n`)
  },

  async text({ service }) {
    const { js } = await service.transform(`This is some text`, { loader: 'text' })
    assert.strictEqual(js, `module.exports = "This is some text";\n`)
  },

  async base64({ service }) {
    const { js } = await service.transform(`\x00\x01\x02`, { loader: 'base64' })
    assert.strictEqual(js, `module.exports = "AAEC";\n`)
  },
}

async function main() {
  // Start the esbuild service
  const esbuild = installForTests(testDir)
  const service = await esbuild.startService()

  // Run all tests concurrently
  const runTest = ([name, fn]) => fn({ esbuild, service }).then(
    () => true,
    e => {
      console.error(`❌ ${name}: ${e && e.message || e}`)
      return false
    })
  const allTestsPassed = (await Promise.all(Object.entries(tests).map(runTest))).every(success => success)

  // Clean up test output
  service.stop()
  childProcess.execSync(`rm -fr "${testDir}"`)

  if (!allTestsPassed) {
    process.exit(1)
  }
}

main().catch(e => setTimeout(() => { throw e }))
