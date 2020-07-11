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
const rootTestDir = path.join(repoDir, 'scripts', '.js-api-tests')

let buildTests = {
  async es6_to_cjs({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, 'export default 123')
    const value = await esbuild.build({ entryPoints: [input], bundle: true, outfile: output, format: 'cjs' })
    assert.strictEqual(value.outputFiles, void 0)
    const result = require(output)
    assert.strictEqual(result.default, 123)
    assert.strictEqual(result.__esModule, true)
  },

  // Test recursive directory creation
  async recursiveMkdir({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'a/b/c/d/out.js')
    await writeFileAsync(input, 'export default 123')
    await esbuild.build({ entryPoints: [input], bundle: true, outfile: output, format: 'cjs' })
    const result = require(output)
    assert.strictEqual(result.default, 123)
    assert.strictEqual(result.__esModule, true)
  },

  async sourceMap({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, 'exports.foo = 123')
    await esbuild.build({ entryPoints: [input], outfile: output, sourcemap: true })
    const result = require(output)
    assert.strictEqual(result.foo, 123)
    const outputFile = await readFileAsync(output, 'utf8')
    const match = /\/\/# sourceMappingURL=(.*)/.exec(outputFile)
    assert.strictEqual(match[1], 'out.js.map')
    const resultMap = await readFileAsync(output + '.map', 'utf8')
    const json = JSON.parse(resultMap)
    assert.strictEqual(json.version, 3)
  },

  async sourceMapExternal({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
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

  async sourceMapInline({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, 'exports.foo = 123')
    await esbuild.build({ entryPoints: [input], outfile: output, sourcemap: 'inline' })
    const result = require(output)
    assert.strictEqual(result.foo, 123)
    const outputFile = await readFileAsync(output, 'utf8')
    const match = /\/\/# sourceMappingURL=data:application\/json;base64,(.*)/.exec(outputFile)
    const json = JSON.parse(Buffer.from(match[1], 'base64').toString())
    assert.strictEqual(json.version, 3)
  },

  async resolveExtensionOrder({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js');
    const inputBare = path.join(testDir, 'module.js')
    const inputSomething = path.join(testDir, 'module.something.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, 'exports.result = require("./module").foo')
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

  async metafile({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const imported = path.join(testDir, 'imported.js')
    const text = path.join(testDir, 'text.txt')
    const output = path.join(testDir, 'out.js')
    const meta = path.join(testDir, 'meta.json')
    await writeFileAsync(entry, `
      import x from "./imported"
      import y from "./text.txt"
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
    assert.deepStrictEqual(json.inputs[makePath(entry)].bytes, 95)
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

  async metafileSplitting({ esbuild, testDir }) {
    const entry1 = path.join(testDir, 'entry1.js')
    const entry2 = path.join(testDir, 'entry2.js')
    const imported = path.join(testDir, 'imported.js')
    const outdir = path.join(testDir, 'out')
    const metafile = path.join(testDir, 'meta.json')
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
    const chunk = 'chunk.3YfO7hrU.js';
    assert.deepStrictEqual(json.outputs[makePath(path.basename(entry1))].imports, [{ path: makePath(chunk) }])
    assert.deepStrictEqual(json.outputs[makePath(path.basename(entry2))].imports, [{ path: makePath(chunk) }])
    assert.deepStrictEqual(json.outputs[makePath(chunk)].imports, [])
  },

  // Test in-memory output files
  async writeFalse({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
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
    assert.strictEqual(js, `// scripts/.js-api-tests/writeFalse/in.js\nconsole.log();\n//# sourceMappingURL=out.js.map\n`)
  },

  async splittingRelativeSameDir({ esbuild, testDir }) {
    const inputA = path.join(testDir, 'a.js')
    const inputB = path.join(testDir, 'b.js')
    const inputCommon = path.join(testDir, 'common.js')
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
    const outdir = path.join(testDir, 'out')
    const value = await esbuild.build({ entryPoints: [inputA, inputB], bundle: true, outdir, format: 'esm', splitting: true, write: false })
    assert.strictEqual(value.outputFiles.length, 3)

    // These should all use forward slashes, even on Windows
    assert.strictEqual(Buffer.from(value.outputFiles[0].contents).toString(), `import {
  common_default
} from "./chunk.xL6KqlYO.js";

// scripts/.js-api-tests/splittingRelativeSameDir/a.js
console.log("a" + common_default);
`)
    assert.strictEqual(Buffer.from(value.outputFiles[1].contents).toString(), `import {
  common_default
} from "./chunk.xL6KqlYO.js";

// scripts/.js-api-tests/splittingRelativeSameDir/b.js
console.log("b" + common_default);
`)
    assert.strictEqual(Buffer.from(value.outputFiles[2].contents).toString(), `// scripts/.js-api-tests/splittingRelativeSameDir/common.js
var common_default = "common";

export {
  common_default
};
`)

    assert.strictEqual(value.outputFiles[0].path, path.join(outdir, path.basename(inputA)))
    assert.strictEqual(value.outputFiles[1].path, path.join(outdir, path.basename(inputB)))
    assert.strictEqual(value.outputFiles[2].path, path.join(outdir, 'chunk.xL6KqlYO.js'))
  },

  async splittingRelativeNestedDir({ esbuild, testDir }) {
    const inputA = path.join(testDir, 'a/demo.js')
    const inputB = path.join(testDir, 'b/demo.js')
    const inputCommon = path.join(testDir, 'common.js')
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
    const outdir = path.join(testDir, 'out')
    const value = await esbuild.build({ entryPoints: [inputA, inputB], bundle: true, outdir, format: 'esm', splitting: true, write: false })
    assert.strictEqual(value.outputFiles.length, 3)

    // These should all use forward slashes, even on Windows
    assert.strictEqual(Buffer.from(value.outputFiles[0].contents).toString(), `import {
  common_default
} from "../chunk.uWHCLtU_.js";

// scripts/.js-api-tests/splittingRelativeNestedDir/a/demo.js
console.log("a" + common_default);
`)
    assert.strictEqual(Buffer.from(value.outputFiles[1].contents).toString(), `import {
  common_default
} from "../chunk.uWHCLtU_.js";

// scripts/.js-api-tests/splittingRelativeNestedDir/b/demo.js
console.log("b" + common_default);
`)
    assert.strictEqual(Buffer.from(value.outputFiles[2].contents).toString(), `// scripts/.js-api-tests/splittingRelativeNestedDir/common.js
var common_default = "common";

export {
  common_default
};
`)

    assert.strictEqual(value.outputFiles[0].path, path.join(outdir, path.relative(testDir, inputA)))
    assert.strictEqual(value.outputFiles[1].path, path.join(outdir, path.relative(testDir, inputB)))
    assert.strictEqual(value.outputFiles[2].path, path.join(outdir, 'chunk.uWHCLtU_.js'))
  },

  async stdinStdoutBundle({ esbuild, testDir }) {
    const aux = path.join(testDir, 'aux.js')
    await writeFileAsync(aux, 'export default 123')
    const value = await esbuild.build({
      stdin: {
        contents: `
          import x from './aux.js'
          console.log(x)
        `,
        resolveDir: testDir,
      },
      bundle: true,
      write: false,
    })
    assert.strictEqual(value.outputFiles.length, 1)
    assert.strictEqual(value.outputFiles[0].path, '<stdout>')
    assert.strictEqual(Buffer.from(value.outputFiles[0].contents).toString(), `(() => {
  // scripts/.js-api-tests/stdinStdoutBundle/aux.js
  var aux_default = 123;

  // <stdin>
  console.log(aux_default);
})();
`)
  },

  async stdinOutfileBundle({ esbuild, testDir }) {
    const aux = path.join(testDir, 'aux.js')
    const outfile = path.join(testDir, 'out.js')
    await writeFileAsync(aux, 'export default 123')
    const value = await esbuild.build({
      stdin: {
        contents: `
          import x from './aux.js'
          export {x as fromStdin}
        `,
        resolveDir: testDir,
      },
      bundle: true,
      outfile,
      format: 'cjs',
    })
    assert.strictEqual(value.outputFiles, void 0)
    const result = require(outfile)
    assert.strictEqual(result.fromStdin, 123)
  },

  async stdinAndEntryBundle({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const aux = path.join(testDir, 'aux.js')
    const outdir = path.join(testDir, 'out')
    await writeFileAsync(aux, 'export default 123')
    await writeFileAsync(entry, `
      import x from './aux.js'
      export let fromEntry = x
    `)
    const value = await esbuild.build({
      entryPoints: [entry],
      stdin: {
        contents: `
          import x from './${path.relative(process.cwd(), aux).replace(/\\/g, '/')}'
          export {x as fromStdin}
        `,
        sourcefile: 'der',
      },
      bundle: true,
      outdir,
      format: 'cjs',
    })
    assert.strictEqual(value.outputFiles, void 0)
    const entryResult = require(path.join(outdir, path.basename(entry)))
    assert.strictEqual(entryResult.fromEntry, 123)
    const stdinResult = require(path.join(outdir, path.basename('stdin.js')))
    assert.strictEqual(stdinResult.fromStdin, 123)
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
  async buildSync({ esbuild, testDir }) {
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
  const esbuild = installForTests(rootTestDir)
  const service = await esbuild.startService()

  // Run all tests concurrently
  const runTest = async ([name, fn]) => {
    let testDir = path.join(rootTestDir, name)
    try {
      await mkdirAsync(testDir)
      await fn({ esbuild, service, testDir })
      rimraf.sync(testDir, { disableGlob: true })
      return true
    } catch (e) {
      console.error(`❌ ${name}: ${e && e.message || e}`)
      return false
    }
  }
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
    rimraf.sync(rootTestDir, { disableGlob: true })
  }
}

main().catch(e => setTimeout(() => { throw e }))
