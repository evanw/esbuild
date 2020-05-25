const { installForTests } = require('./esbuild')
const { SourceMapConsumer } = require('source-map')
const rimraf = require('rimraf')
const assert = require('assert')
const path = require('path')
const util = require('util')
const fs = require('fs')

const repoDir = path.dirname(__dirname)
const testDir = path.join(repoDir, 'scripts', '.js-api-tests')

let buildTests = {
  async es6_to_cjs({ esbuild }) {
    const input = path.join(testDir, '1-in.js')
    const output = path.join(testDir, '1-out.js')
    await util.promisify(fs.writeFile)(input, 'export default 123')
    await esbuild.build({ entryPoints: [input], bundle: true, outfile: output, format: 'cjs' })
    const result = require(output)
    assert.strictEqual(result.default, 123)
    assert.strictEqual(result.__esModule, true)
  },

  async sourceMap({ esbuild }) {
    const input = path.join(testDir, '2-in.js')
    const output = path.join(testDir, '2-out.js')
    await util.promisify(fs.writeFile)(input, 'exports.foo = 123')
    await esbuild.build({ entryPoints: [input], outfile: output, sourcemap: true })
    const result = require(output)
    assert.strictEqual(result.foo, 123)
    const resultMap = await util.promisify(fs.readFile)(output + '.map', 'utf8')
    const json = JSON.parse(resultMap)
    assert.strictEqual(json.version, 3)
  },

  async sourceMapInline({ esbuild }) {
    const input = path.join(testDir, '3-in.js')
    const output = path.join(testDir, '3-out.js')
    await util.promisify(fs.writeFile)(input, 'exports.foo = 123')
    await esbuild.build({ entryPoints: [input], outfile: output, sourcemap: 'inline' })
    const result = require(output)
    assert.strictEqual(result.foo, 123)
    const outputFile = await util.promisify(fs.readFile)(output, 'utf8')
    const match = /\/\/# sourceMappingURL=data:application\/json;base64,(.*)/.exec(outputFile)
    const json = JSON.parse(Buffer.from(match[1], 'base64').toString())
    assert.strictEqual(json.version, 3)
  },
}

let transformTests = {
  async cjs_require({ service }) {
    const { js } = await service.transform(`const {foo} = require('path')`, {})
    assert.strictEqual(js, `const {foo} = require("path");\n`)
  },

  async cjs_exports({ service }) {
    const { js } = await service.transform(`exports.foo = 123`, {})
    assert.strictEqual(js, `exports.foo = 123;\n`)
  },

  async es6_import({ service }) {
    const { js } = await service.transform(`import {foo} from 'path'`, {})
    assert.strictEqual(js, `import {foo} from "path";\n`)
  },

  async es6_export({ service }) {
    const { js } = await service.transform(`export const foo = 123`, {})
    assert.strictEqual(js, `export const foo = 123;\n`)
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

  async dataurl({ service }) {
    const { js } = await service.transform(`\x00\x01\x02`, { loader: 'dataurl' })
    assert.strictEqual(js, `module.exports = "data:application/octet-stream;base64,AAEC";\n`)
  },

  async sourceMap({ service }) {
    const { js, jsSourceMap } = await service.transform(`let       x`, { sourcemap: true })
    assert.strictEqual(js, `let x;\n`)
    await assertSourceMap(jsSourceMap, '/input.js')
  },

  async sourceMapWithName({ service }) {
    const { js, jsSourceMap } = await service.transform(`let       x`, { sourcemap: true, sourcefile: 'afile.js' })
    assert.strictEqual(js, `let x;\n`)
    await assertSourceMap(jsSourceMap, 'afile.js')
  },

  async sourceMapInline({ service }) {
    const { js, jsSourceMap } = await service.transform(`let       x`, { sourcemap: 'inline' })
    assert(js.startsWith(`let x;\n//# sourceMappingURL=`))
    assert.strictEqual(jsSourceMap, undefined)
    const base64 = js.slice(js.indexOf('base64,') + 'base64,'.length)
    await assertSourceMap(Buffer.from(base64.trim(), 'base64').toString(), '/input.js')
  },

  async sourceMapInlineWithName({ service }) {
    const { js, jsSourceMap } = await service.transform(`let       x`, { sourcemap: 'inline', sourcefile: 'afile.js' })
    assert(js.startsWith(`let x;\n//# sourceMappingURL=`))
    assert.strictEqual(jsSourceMap, undefined)
    const base64 = js.slice(js.indexOf('base64,') + 'base64,'.length)
    await assertSourceMap(Buffer.from(base64.trim(), 'base64').toString(), 'afile.js')
  },
}

async function assertSourceMap(jsSourceMap, source) {
  const map = await new SourceMapConsumer(jsSourceMap)
  const original = map.originalPositionFor({ line: 1, column: 4 })
  assert.strictEqual(original.source, source)
  assert.strictEqual(original.line, 1)
  assert.strictEqual(original.column, 10)
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
      if (e.errors) console.error(e.errors.map(x => x.text).join('\n'))
      return false
    })
  const tests = [...Object.entries(buildTests), ...Object.entries(transformTests)]
  const allTestsPassed = (await Promise.all(tests.map(runTest))).every(success => success)

  // Clean up test output
  service.stop()
  rimraf.sync(testDir, { disableGlob: true })

  if (!allTestsPassed) {
    console.error(`❌ js api tests failed`)
    process.exit(1)
  } else {
    console.log(`✅ js api tests passed`)
  }
}

main().catch(e => setTimeout(() => { throw e }))
