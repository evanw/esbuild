const { installForTests } = require('./esbuild')
const { SourceMapConsumer } = require('source-map')
const rimraf = require('rimraf')
const assert = require('assert')
const path = require('path')
const util = require('util')
const fs = require('fs')

const readFileAsync = util.promisify(fs.readFile)
const writeFileAsync = util.promisify(fs.writeFile)
const mkdirAsync = util.promisify(fs.mkdir)

const repoDir = path.dirname(__dirname)
const testDir = path.join(repoDir, 'scripts', '.js-api-tests')

let buildTests = {
  async es6_to_cjs({ esbuild }) {
    const input = path.join(testDir, 'es6_to_cjs-in.js')
    const output = path.join(testDir, 'es6_to_cjs-out.js')
    await writeFileAsync(input, 'export default 123')
    const value = await esbuild.build({ entryPoints: [input], bundle: true, outfile: output, format: 'cjs' })
    assert.strictEqual(value.outputFiles, void 0)
    const result = require(output)
    assert.strictEqual(result.default, 123)
    assert.strictEqual(result.__esModule, true)
  },

  // Test recursive directory creation
  async recursiveMkdir({ esbuild }) {
    const input = path.join(testDir, 'recursiveMkdir-in.js')
    const output = path.join(testDir, 'a/b/c/d/recursiveMkdir-out.js')
    await writeFileAsync(input, 'export default 123')
    await esbuild.build({ entryPoints: [input], bundle: true, outfile: output, format: 'cjs' })
    const result = require(output)
    assert.strictEqual(result.default, 123)
    assert.strictEqual(result.__esModule, true)
  },

  async sourceMap({ esbuild }) {
    const input = path.join(testDir, 'sourceMap-in.js')
    const output = path.join(testDir, 'sourceMap-out.js')
    await writeFileAsync(input, 'exports.foo = 123')
    await esbuild.build({ entryPoints: [input], outfile: output, sourcemap: true })
    const result = require(output)
    assert.strictEqual(result.foo, 123)
    const outputFile = await readFileAsync(output, 'utf8')
    const match = /\/\/# sourceMappingURL=(.*)/.exec(outputFile)
    assert.strictEqual(match[1], 'sourceMap-out.js.map')
    const resultMap = await readFileAsync(output + '.map', 'utf8')
    const json = JSON.parse(resultMap)
    assert.strictEqual(json.version, 3)
  },

  async sourceMapExternal({ esbuild }) {
    const input = path.join(testDir, 'sourceMapExternal-in.js')
    const output = path.join(testDir, 'sourceMapExternal-out.js')
    await writeFileAsync(input, 'exports.foo = 123')
    await esbuild.build({ entryPoints: [input], outfile: output, sourcemap: 'external' })
    const result = require(output)
    assert.strictEqual(result.foo, 123)
    const outputFile = await readFileAsync(output, 'utf8')
    const match = /\/\/# sourceMappingURL=(.*)/.exec(outputFile)
    assert.strictEqual(match, null)
    const resultMap = await readFileAsync(output + '.map', 'utf8')
    const json = JSON.parse(resultMap)
    assert.strictEqual(json.version, 3)
  },

  async sourceMapInline({ esbuild }) {
    const input = path.join(testDir, 'sourceMapInline-in.js')
    const output = path.join(testDir, 'sourceMapInline-out.js')
    await writeFileAsync(input, 'exports.foo = 123')
    await esbuild.build({ entryPoints: [input], outfile: output, sourcemap: 'inline' })
    const result = require(output)
    assert.strictEqual(result.foo, 123)
    const outputFile = await readFileAsync(output, 'utf8')
    const match = /\/\/# sourceMappingURL=data:application\/json;base64,(.*)/.exec(outputFile)
    const json = JSON.parse(Buffer.from(match[1], 'base64').toString())
    assert.strictEqual(json.version, 3)
  },

  async resolveExtensionOrder({ esbuild }) {
    const input = path.join(testDir, 'resolveExtensionOrder-in.js');
    const inputBare = path.join(testDir, 'resolveExtensionOrder-module.js')
    const inputSomething = path.join(testDir, 'resolveExtensionOrder-module.something.js')
    const output = path.join(testDir, 'resolveExtensionOrder-out.js')
    await writeFileAsync(input, 'exports.result = require("./resolveExtensionOrder-module").foo')
    await writeFileAsync(inputBare, 'exports.foo = 321')
    await writeFileAsync(inputSomething, 'exports.foo = 123')
    await esbuild.build({
      entryPoints: [input],
      outfile: output,
      format: 'cjs',
      bundle: true,
      resolveExtensions: ['.something.js', '.js'],
    })
    assert.strictEqual(require(output).result, 123)
  },

  async metafile({ esbuild }) {
    const entry = path.join(testDir, 'metafile-entry.js')
    const imported = path.join(testDir, 'metafile-imported.js')
    const text = path.join(testDir, 'metafile-text.txt')
    const output = path.join(testDir, 'metafile-out.js')
    const meta = path.join(testDir, 'metafile-meta.json')
    await writeFileAsync(entry, `
      import x from "./metafile-imported"
      import y from "./metafile-text.txt"
      console.log(x, y)
    `)
    await writeFileAsync(imported, 'export default 123')
    await writeFileAsync(text, 'some text')
    await esbuild.build({
      entryPoints: [entry],
      bundle: true,
      outfile: output,
      metafile: meta,
      sourcemap: true,
      loader: { '.txt': 'file' },
    })

    const json = JSON.parse(await readFileAsync(meta))
    assert.strictEqual(Object.keys(json.inputs).length, 3)
    assert.strictEqual(Object.keys(json.outputs).length, 3)
    const cwd = process.cwd()
    const makePath = absPath => path.relative(cwd, absPath).split(path.sep).join('/')

    // Check inputs
    assert.deepStrictEqual(json.inputs[makePath(entry)].bytes, 113)
    assert.deepStrictEqual(json.inputs[makePath(entry)].imports, [
      { path: makePath(imported) },
      { path: makePath(text) },
    ])
    assert.deepStrictEqual(json.inputs[makePath(imported)].bytes, 18)
    assert.deepStrictEqual(json.inputs[makePath(imported)].imports, [])
    assert.deepStrictEqual(json.inputs[makePath(text)].bytes, 9)
    assert.deepStrictEqual(json.inputs[makePath(text)].imports, [])

    // Check outputs
    assert.strictEqual(typeof json.outputs[makePath(output)].bytes, 'number')
    assert.strictEqual(typeof json.outputs[makePath(output) + '.map'].bytes, 'number')
    assert.deepStrictEqual(json.outputs[makePath(output) + '.map'].inputs, {})

    // Check inputs for main output
    const outputInputs = json.outputs[makePath(output)].inputs
    assert.strictEqual(Object.keys(outputInputs).length, 3)
    assert.strictEqual(typeof outputInputs[makePath(entry)].bytesInOutput, 'number')
    assert.strictEqual(typeof outputInputs[makePath(imported)].bytesInOutput, 'number')
    assert.strictEqual(typeof outputInputs[makePath(text)].bytesInOutput, 'number')
  },

  async metafileSplitting({ esbuild }) {
    const entry1 = path.join(testDir, 'metafileSplitting-entry1.js')
    const entry2 = path.join(testDir, 'metafileSplitting-entry2.js')
    const imported = path.join(testDir, 'metafileSplitting-imported.js')
    const outdir = path.join(testDir, 'metafileSplitting-out')
    const metafile = path.join(testDir, 'metafileSplitting-meta.json')
    await writeFileAsync(entry1, `
      import x from "./${path.basename(imported)}"
      console.log(1, x)
    `)
    await writeFileAsync(entry2, `
      import x from "./${path.basename(imported)}"
      console.log(2, x)
    `)
    await writeFileAsync(imported, 'export default 123')
    await esbuild.build({
      entryPoints: [entry1, entry2],
      bundle: true,
      outdir,
      metafile,
      splitting: true,
      format: 'esm',
    })

    const json = JSON.parse(await readFileAsync(metafile))
    assert.strictEqual(Object.keys(json.inputs).length, 3)
    assert.strictEqual(Object.keys(json.outputs).length, 3)
    const cwd = process.cwd()
    const makePath = basename => path.relative(cwd, path.join(outdir, basename)).split(path.sep).join('/')

    // Check outputs
    const chunk = 'chunk.pFRXOXgr.js';
    assert.deepStrictEqual(json.outputs[makePath(path.basename(entry1))].imports, [{ path: makePath(chunk) }])
    assert.deepStrictEqual(json.outputs[makePath(path.basename(entry2))].imports, [{ path: makePath(chunk) }])
    assert.deepStrictEqual(json.outputs[makePath(chunk)].imports, [])
  },

  // Test in-memory output files
  async writeFalse({ esbuild }) {
    const input = path.join(testDir, 'writeFalse.js')
    const output = path.join(testDir, 'writeFalse-out.js')
    await writeFileAsync(input, 'console.log()')
    const value = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outfile: output,
      sourcemap: true,
      format: 'esm',
      write: false,
    })
    assert.strictEqual(await fs.existsSync(output), false)
    assert.notStrictEqual(value.outputFiles, void 0)
    assert.strictEqual(value.outputFiles.length, 2)
    assert.strictEqual(value.outputFiles[0].path, output + '.map')
    assert.strictEqual(value.outputFiles[0].contents.constructor, Uint8Array)
    assert.strictEqual(value.outputFiles[1].path, output)
    assert.strictEqual(value.outputFiles[1].contents.constructor, Uint8Array)
    const sourceMap = JSON.parse(Buffer.from(value.outputFiles[0].contents).toString())
    const js = Buffer.from(value.outputFiles[1].contents).toString()
    assert.strictEqual(sourceMap.version, 3)
    assert.strictEqual(js, `// scripts/.js-api-tests/writeFalse.js\nconsole.log();\n//# sourceMappingURL=writeFalse-out.js.map\n`)
  },

  async splittingRelativeSameDir({ esbuild }) {
    const inputA = path.join(testDir, 'splittingRelativeSameDir-a.js')
    const inputB = path.join(testDir, 'splittingRelativeSameDir-b.js')
    const inputCommon = path.join(testDir, 'splittingRelativeSameDir-common.js')
    await writeFileAsync(inputA, `
      import x from "./${path.basename(inputCommon)}"
      console.log('a' + x)
    `)
    await writeFileAsync(inputB, `
      import x from "./${path.basename(inputCommon)}"
      console.log('b' + x)
    `)
    await writeFileAsync(inputCommon, `
      export default 'common'
    `)
    const outdir = path.join(testDir, 'splittingRelativeSameDir-out')
    const value = await esbuild.build({ entryPoints: [inputA, inputB], bundle: true, outdir, format: 'esm', splitting: true, write: false })
    assert.strictEqual(value.outputFiles.length, 3)

    // These should all use forward slashes, even on Windows
    assert.strictEqual(Buffer.from(value.outputFiles[0].contents).toString(), `import {
  splittingRelativeSameDir_common_default
} from "./chunk.4JtreZIq.js";

// scripts/.js-api-tests/splittingRelativeSameDir-a.js
console.log("a" + splittingRelativeSameDir_common_default);
`)
    assert.strictEqual(Buffer.from(value.outputFiles[1].contents).toString(), `import {
  splittingRelativeSameDir_common_default
} from "./chunk.4JtreZIq.js";

// scripts/.js-api-tests/splittingRelativeSameDir-b.js
console.log("b" + splittingRelativeSameDir_common_default);
`)
    assert.strictEqual(Buffer.from(value.outputFiles[2].contents).toString(), `// scripts/.js-api-tests/splittingRelativeSameDir-common.js
var splittingRelativeSameDir_common_default = "common";

export {
  splittingRelativeSameDir_common_default
};
`)

    assert.strictEqual(value.outputFiles[0].path, path.join(outdir, path.basename(inputA)))
    assert.strictEqual(value.outputFiles[1].path, path.join(outdir, path.basename(inputB)))
    assert.strictEqual(value.outputFiles[2].path, path.join(outdir, 'chunk.4JtreZIq.js'))
  },

  async splittingRelativeNestedDir({ esbuild }) {
    const inputA = path.join(testDir, 'splittingRelativeNestedDir-a/demo.js')
    const inputB = path.join(testDir, 'splittingRelativeNestedDir-b/demo.js')
    const inputCommon = path.join(testDir, 'splittingRelativeNestedDir-common.js')
    await mkdirAsync(path.dirname(inputA)).catch(x => x)
    await mkdirAsync(path.dirname(inputB)).catch(x => x)
    await writeFileAsync(inputA, `
      import x from "../${path.basename(inputCommon)}"
      console.log('a' + x)
    `)
    await writeFileAsync(inputB, `
      import x from "../${path.basename(inputCommon)}"
      console.log('b' + x)
    `)
    await writeFileAsync(inputCommon, `
      export default 'common'
    `)
    const outdir = path.join(testDir, 'splittingRelativeNestedDir-out')
    const value = await esbuild.build({ entryPoints: [inputA, inputB], bundle: true, outdir, format: 'esm', splitting: true, write: false })
    assert.strictEqual(value.outputFiles.length, 3)

    // These should all use forward slashes, even on Windows
    assert.strictEqual(Buffer.from(value.outputFiles[0].contents).toString(), `import {
  splittingRelativeNestedDir_common_default
} from "../chunk._R_iWKlj.js";

// scripts/.js-api-tests/splittingRelativeNestedDir-a/demo.js
console.log("a" + splittingRelativeNestedDir_common_default);
`)
    assert.strictEqual(Buffer.from(value.outputFiles[1].contents).toString(), `import {
  splittingRelativeNestedDir_common_default
} from "../chunk._R_iWKlj.js";

// scripts/.js-api-tests/splittingRelativeNestedDir-b/demo.js
console.log("b" + splittingRelativeNestedDir_common_default);
`)
    assert.strictEqual(Buffer.from(value.outputFiles[2].contents).toString(), `// scripts/.js-api-tests/splittingRelativeNestedDir-common.js
var splittingRelativeNestedDir_common_default = "common";

export {
  splittingRelativeNestedDir_common_default
};
`)

    assert.strictEqual(value.outputFiles[0].path, path.join(outdir, path.relative(testDir, inputA)))
    assert.strictEqual(value.outputFiles[1].path, path.join(outdir, path.relative(testDir, inputB)))
    assert.strictEqual(value.outputFiles[2].path, path.join(outdir, 'chunk._R_iWKlj.js'))
  },
}

async function futureSyntax(service, js, targetBelow, targetAbove) {
  failure: {
    try { await service.transform(js, { target: targetBelow }) }
    catch { break failure }
    throw new Error(`Expected failure for ${targetBelow}: ${js}`)
  }

  try { await service.transform(js, { target: targetAbove }) }
  catch (e) { throw new Error(`Expected success for ${targetAbove}: ${js}\n${e}`) }
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
    assert.strictEqual(js, `console.log(/* @__PURE__ */ React.createElement("div", null));\n`)
  },

  async ts({ service }) {
    const { js } = await service.transform(`enum Foo { FOO }`, { loader: 'ts' })
    assert.strictEqual(js, `var Foo;\n(function(Foo2) {\n  Foo2[Foo2["FOO"] = 0] = "FOO";\n})(Foo || (Foo = {}));\n`)
  },

  async tsx({ service }) {
    const { js } = await service.transform(`console.log(<Foo<T>/>)`, { loader: 'tsx' })
    assert.strictEqual(js, `console.log(/* @__PURE__ */ React.createElement(Foo, null));\n`)
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
    assert.strictEqual(js, `module.exports = {x: "y"};\n`)
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

  async sourceMapWithName({ service }) {
    const { js, jsSourceMap } = await service.transform(`let       x`, { sourcemap: true, sourcefile: 'afile.js' })
    assert.strictEqual(js, `let x;\n`)
    await assertSourceMap(jsSourceMap, 'afile.js')
  },

  async sourceMapExternalWithName({ service }) {
    const { js, jsSourceMap } = await service.transform(`let       x`, { sourcemap: 'external', sourcefile: 'afile.js' })
    assert.strictEqual(js, `let x;\n`)
    await assertSourceMap(jsSourceMap, 'afile.js')
  },

  async sourceMapInlineWithName({ service }) {
    const { js, jsSourceMap } = await service.transform(`let       x`, { sourcemap: 'inline', sourcefile: 'afile.js' })
    assert(js.startsWith(`let x;\n//# sourceMappingURL=`))
    assert.strictEqual(jsSourceMap, '')
    const base64 = js.slice(js.indexOf('base64,') + 'base64,'.length)
    await assertSourceMap(Buffer.from(base64.trim(), 'base64').toString(), 'afile.js')
  },

  async numericLiteralPrinting({ service }) {
    async function checkLiteral(text) {
      const { js } = await service.transform(`return ${text}`, { minify: true })
      assert.strictEqual(+text, new Function(js)())
    }
    const promises = []
    for (let i = 0; i < 10; i++) {
      for (let j = 0; j < 10; j++) {
        promises.push(checkLiteral(`0.${'0'.repeat(i)}${'123456789'.slice(0, j)}`))
        promises.push(checkLiteral(`1${'0'.repeat(i)}.${'123456789'.slice(0, j)}`))
        promises.push(checkLiteral(`1${'123456789'.slice(0, j)}${'0'.repeat(i)}`))
      }
    }
    await Promise.all(promises)
  },

  async nullishCoalescingLoose({ service }) {
    const { js } = await service.transform(`a ?? b`, { target: 'es2019', strict: false })
    assert.strictEqual(js, `a != null ? a : b;\n`)
  },

  async nullishCoalescingStrict({ service }) {
    const { js } = await service.transform(`a ?? b`, { target: 'es2019', strict: true })
    assert.strictEqual(js, `a !== null && a !== void 0 ? a : b;\n`)
  },

  async nullishCoalescingStrictExplicit({ service }) {
    const { js } = await service.transform(`a ?? b`, { target: 'es2019', strict: ['nullish-coalescing'] })
    assert.strictEqual(js, `a !== null && a !== void 0 ? a : b;\n`)
  },

  async pureCallPrint({ service }) {
    const { js: js1 } = await service.transform(`print(123, foo)`, { minifySyntax: true, pure: [] })
    assert.strictEqual(js1, `print(123, foo);\n`)

    const { js: js2 } = await service.transform(`print(123, foo)`, { minifySyntax: true, pure: ['print'] })
    assert.strictEqual(js2, `foo;\n`)
  },

  async pureCallConsoleLog({ service }) {
    const { js: js1 } = await service.transform(`console.log(123, foo)`, { minifySyntax: true, pure: [] })
    assert.strictEqual(js1, `console.log(123, foo);\n`)

    const { js: js2 } = await service.transform(`console.log(123, foo)`, { minifySyntax: true, pure: ['console.log'] })
    assert.strictEqual(js2, `foo;\n`)
  },

  async multipleEngineTargets({ service }) {
    const check = async (target, expected) =>
      assert.strictEqual((await service.transform(`foo(a ?? b)`, { target })).js, expected)
    await Promise.all([
      check('es2020', `foo(a ?? b);\n`),
      check('es2019', `foo(a != null ? a : b);\n`),

      check('chrome80', `foo(a ?? b);\n`),
      check('chrome79', `foo(a != null ? a : b);\n`),

      check(['es2020', 'chrome80'], `foo(a ?? b);\n`),
      check(['es2020', 'chrome79'], `foo(a != null ? a : b);\n`),
      check(['es2019', 'chrome80'], `foo(a != null ? a : b);\n`),
    ])
  },

  // Future syntax
  forAwait: ({ service }) => futureSyntax(service, 'async function foo() { for await (let x of y) {} }', 'es2017', 'es2018'),
  bigInt: ({ service }) => futureSyntax(service, '123n', 'es2019', 'es2020'),
  nonIdArrayRest: ({ service }) => futureSyntax(service, 'let [...[x]] = y', 'es2015', 'es2016'),

  // Future syntax: async generator functions
  asyncGenFnStmt: ({ service }) => futureSyntax(service, 'async function* foo() {}', 'es2017', 'es2018'),
  asyncGenFnExpr: ({ service }) => futureSyntax(service, '(async function*() {})', 'es2017', 'es2018'),
  asyncGenObjFn: ({ service }) => futureSyntax(service, '({ async* foo() {} })', 'es2017', 'es2018'),
  asyncGenClassStmtFn: ({ service }) => futureSyntax(service, 'class Foo { async* foo() {} }', 'es2017', 'es2018'),
  asyncGenClassExprFn: ({ service }) => futureSyntax(service, '(class { async* foo() {} })', 'es2017', 'es2018'),
}

let syncTests = {
  async buildSync({ esbuild }) {
    const input = path.join(testDir, 'buildSync-in.js')
    const output = path.join(testDir, 'buildSync-out.js')
    await writeFileAsync(input, 'export default 123')
    esbuild.buildSync({ entryPoints: [input], bundle: true, outfile: output, format: 'cjs' })
    const result = require(output)
    assert.strictEqual(result.default, 123)
    assert.strictEqual(result.__esModule, true)
  },

  async transformSync({ esbuild }) {
    const { js } = esbuild.transformSync(`console.log(1+2)`, {})
    assert.strictEqual(js, `console.log(1 + 2);\n`)
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
      return false
    })
  const tests = [
    ...Object.entries(buildTests),
    ...Object.entries(transformTests),
    ...Object.entries(syncTests),
  ]
  const allTestsPassed = (await Promise.all(tests.map(runTest))).every(success => success)

  // Clean up test output
  service.stop()

  if (!allTestsPassed) {
    console.error(`❌ js api tests failed`)
    process.exit(1)
  } else {
    console.log(`✅ js api tests passed`)
    rimraf.sync(testDir, { disableGlob: true })
  }
}

main().catch(e => setTimeout(() => { throw e }))
