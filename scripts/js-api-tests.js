const { installForTests } = require('./esbuild')
const { SourceMapConsumer } = require('source-map')
const rimraf = require('rimraf')
const assert = require('assert')
const path = require('path')
const http = require('http')
const fs = require('fs')
const vm = require('vm')

const readFileAsync = fs.promises.readFile
const writeFileAsync = fs.promises.writeFile
const mkdirAsync = fs.promises.mkdir

const repoDir = path.dirname(__dirname)
const rootTestDir = path.join(repoDir, 'scripts', '.js-api-tests')

let buildTests = {
  async errorIfEntryPointsNotArray({ esbuild }) {
    try {
      await esbuild.build({ entryPoints: 'this is not an array', logLevel: 'silent' })
      throw new Error('Expected build failure');
    } catch (e) {
      if (e.message !== '"entryPoints" must be an array') {
        throw e;
      }
    }
  },

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

  async outExtensionJS({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'in.mjs')
    await writeFileAsync(input, 'console.log("test")')
    await esbuild.build({ entryPoints: [input], outdir: testDir, outExtension: { '.js': '.mjs' } })
    const mjs = await readFileAsync(output, 'utf8')
    assert.strictEqual(mjs, 'console.log("test");\n')
  },

  async outExtensionCSS({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.css')
    const output = path.join(testDir, 'in.notcss')
    await writeFileAsync(input, 'body {}')
    await esbuild.build({ entryPoints: [input], outdir: testDir, outExtension: { '.css': '.notcss' } })
    const notcss = await readFileAsync(output, 'utf8')
    assert.strictEqual(notcss, 'body {\n}\n')
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

  async inject({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js');
    const inject = path.join(testDir, 'inject.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, 'export default foo * 10 + 4')
    await writeFileAsync(inject, 'export let foo = 123')
    await esbuild.build({
      entryPoints: [input],
      outfile: output,
      format: 'cjs',
      bundle: true,
      inject: [inject],
    })
    assert.strictEqual(require(output).default, 1234)
  },

  async mainFields({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    const mainFieldsDir = path.join(testDir, 'node_modules', 'main-fields-test')
    const mainFieldsA = path.join(mainFieldsDir, 'a.js')
    const mainFieldsB = path.join(mainFieldsDir, 'b.js')
    const mainFieldsPackage = path.join(mainFieldsDir, 'package.json')
    await mkdirAsync(mainFieldsDir, { recursive: true })
    await writeFileAsync(input, 'export * from "main-fields-test"')
    await writeFileAsync(mainFieldsA, 'export let foo = "a"')
    await writeFileAsync(mainFieldsB, 'export let foo = "b"')
    await writeFileAsync(mainFieldsPackage, '{ "a": "./a.js", "b": "./b.js", "c": "./c.js" }')
    await esbuild.build({ entryPoints: [input], outfile: output, bundle: true, format: 'cjs', mainFields: ['c', 'b', 'a'] })
    const result = require(output)
    assert.strictEqual(result.foo, 'b')
  },

  async requireAbsolutePath({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const dependency = path.join(testDir, 'dep.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `import value from ${JSON.stringify(dependency)}; export default value`)
    await writeFileAsync(dependency, `export default 123`)
    const value = await esbuild.build({ entryPoints: [input], bundle: true, outfile: output, format: 'cjs' })
    assert.strictEqual(value.outputFiles, void 0)
    const result = require(output)
    assert.strictEqual(result.default, 123)
    assert.strictEqual(result.__esModule, true)
  },

  async fileLoader({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const data = path.join(testDir, 'data.bin')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `export {default as value} from ${JSON.stringify(data)}`)
    await writeFileAsync(data, `stuff`)
    const value = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outfile: output,
      format: 'cjs',
      loader: { '.bin': 'file' },
    })
    assert.strictEqual(value.outputFiles, void 0)
    const result = require(output)
    assert.strictEqual(result.value, 'data.L3XDQOAT.bin')
    assert.strictEqual(result.__esModule, true)
  },

  async splittingPublicPath({ esbuild, testDir }) {
    const input1 = path.join(testDir, 'a', 'in1.js')
    const input2 = path.join(testDir, 'b', 'in2.js')
    const shared = path.join(testDir, 'c', 'shared.js')
    const outdir = path.join(testDir, 'out')
    await mkdirAsync(path.dirname(input1), { recursive: true })
    await mkdirAsync(path.dirname(input2), { recursive: true })
    await mkdirAsync(path.dirname(shared), { recursive: true })
    await writeFileAsync(input1, `export {default as input1} from ${JSON.stringify(shared)}`)
    await writeFileAsync(input2, `export {default as input2} from ${JSON.stringify(shared)}`)
    await writeFileAsync(shared, `export default function foo() { return 123 }`)
    const value = await esbuild.build({
      entryPoints: [input1, input2],
      bundle: true,
      outdir,
      format: 'esm',
      splitting: true,
      publicPath: 'https://www.example.com/assets',
      write: false,
    })
    assert.deepStrictEqual(value.outputFiles.length, 3)
    assert.deepStrictEqual(value.outputFiles[0].path, path.join(outdir, 'a', 'in1.js'))
    assert.deepStrictEqual(value.outputFiles[1].path, path.join(outdir, 'b', 'in2.js'))
    assert.deepStrictEqual(value.outputFiles[2].path, path.join(outdir, 'chunk.F3VMEPVO.js'))
    assert.deepStrictEqual(value.outputFiles[0].text, `import {
  foo
} from "https://www.example.com/assets/chunk.F3VMEPVO.js";
export {
  foo as input1
};
`)
    assert.deepStrictEqual(value.outputFiles[1].text, `import {
  foo
} from "https://www.example.com/assets/chunk.F3VMEPVO.js";
export {
  foo as input2
};
`)
    assert.deepStrictEqual(value.outputFiles[2].text, `// scripts/.js-api-tests/splittingPublicPath/c/shared.js
function foo() {
  return 123;
}

export {
  foo
};
`)
  },

  async fileLoaderPublicPath({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const data = path.join(testDir, 'data.bin')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `export {default as value} from ${JSON.stringify(data)}`)
    await writeFileAsync(data, `stuff`)
    const value = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outfile: output,
      format: 'cjs',
      loader: { '.bin': 'file' },
      publicPath: 'https://www.example.com/assets',
    })
    assert.strictEqual(value.outputFiles, void 0)
    const result = require(output)
    assert.strictEqual(result.value, 'https://www.example.com/assets/data.L3XDQOAT.bin')
    assert.strictEqual(result.__esModule, true)
  },

  async fileLoaderCSS({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.css')
    const data = path.join(testDir, 'data.bin')
    const output = path.join(testDir, 'out.css')
    await writeFileAsync(input, `body { background: url(${JSON.stringify(data)}) }`)
    await writeFileAsync(data, `stuff`)
    const value = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outfile: output,
      loader: { '.bin': 'file' },
      publicPath: 'https://www.example.com/assets',
    })
    assert.strictEqual(value.outputFiles, void 0)
    assert.strictEqual(await readFileAsync(output, 'utf8'), `/* scripts/.js-api-tests/fileLoaderCSS/in.css */
body {
  background: url(https://www.example.com/assets/data.L3XDQOAT.bin);
}
`)
  },

  async fileLoaderBinaryVsText({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const valid = path.join(testDir, 'valid.bin')
    const invalid = path.join(testDir, 'invalid.bin')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `
      import valid from ${JSON.stringify(valid)}
      import invalid from ${JSON.stringify(invalid)}
      console.log(valid, invalid)
    `)
    await writeFileAsync(valid, Buffer.from([0xCF, 0x80]))
    await writeFileAsync(invalid, Buffer.from([0x80, 0xCF]))
    const value = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outfile: output,
      format: 'cjs',
      loader: { '.bin': 'file' },
      write: false,
    })
    assert.strictEqual(value.outputFiles.length, 3)

    // Valid UTF-8 should decode correctly
    assert.deepEqual(value.outputFiles[0].contents, new Uint8Array([207, 128]))
    assert.strictEqual(value.outputFiles[0].text, 'π')

    // Invalid UTF-8 should be preserved as bytes but should be replaced by the U+FFFD replacement character when decoded
    assert.deepEqual(value.outputFiles[1].contents, new Uint8Array([128, 207]))
    assert.strictEqual(value.outputFiles[1].text, '\uFFFD\uFFFD')
  },

  async metafile({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const imported = path.join(testDir, 'imported.js')
    const text = path.join(testDir, 'text.txt')
    const css = path.join(testDir, 'example.css')
    const output = path.join(testDir, 'out.js')
    const meta = path.join(testDir, 'meta.json')
    await writeFileAsync(entry, `
      import x from "./imported"
      import y from "./text.txt"
      import * as z from "./example.css"
      console.log(x, y, z)
    `)
    await writeFileAsync(imported, 'export default 123')
    await writeFileAsync(text, 'some text')
    await writeFileAsync(css, 'body { some: css; }')
    await esbuild.build({
      entryPoints: [entry],
      bundle: true,
      outfile: output,
      metafile: meta,
      sourcemap: true,
      loader: { '.txt': 'file' },
    })

    const json = JSON.parse(await readFileAsync(meta))
    assert.strictEqual(Object.keys(json.inputs).length, 4)
    assert.strictEqual(Object.keys(json.outputs).length, 4)
    const cwd = process.cwd()
    const makePath = absPath => path.relative(cwd, absPath).split(path.sep).join('/')

    // Check inputs
    assert.deepStrictEqual(json.inputs[makePath(entry)].bytes, 139)
    assert.deepStrictEqual(json.inputs[makePath(entry)].imports, [
      { path: makePath(imported) },
      { path: makePath(text) },
      { path: makePath(css) },
    ])
    assert.deepStrictEqual(json.inputs[makePath(imported)].bytes, 18)
    assert.deepStrictEqual(json.inputs[makePath(imported)].imports, [])
    assert.deepStrictEqual(json.inputs[makePath(text)].bytes, 9)
    assert.deepStrictEqual(json.inputs[makePath(text)].imports, [])
    assert.deepStrictEqual(json.inputs[makePath(css)].bytes, 19)
    assert.deepStrictEqual(json.inputs[makePath(css)].imports, [])

    // Check outputs
    assert.strictEqual(typeof json.outputs[makePath(output)].bytes, 'number')
    assert.strictEqual(typeof json.outputs[makePath(output) + '.map'].bytes, 'number')
    assert.deepStrictEqual(json.outputs[makePath(output) + '.map'].inputs, {})

    // Check inputs for main output
    const outputInputs = json.outputs[makePath(output)].inputs
    assert.strictEqual(Object.keys(outputInputs).length, 4)
    assert.strictEqual(typeof outputInputs[makePath(entry)].bytesInOutput, 'number')
    assert.strictEqual(typeof outputInputs[makePath(imported)].bytesInOutput, 'number')
    assert.strictEqual(typeof outputInputs[makePath(text)].bytesInOutput, 'number')
    assert.strictEqual(typeof outputInputs[makePath(css)].bytesInOutput, 'number')
  },

  async metafileSplitting({ esbuild, testDir }) {
    const entry1 = path.join(testDir, 'entry1.js')
    const entry2 = path.join(testDir, 'entry2.js')
    const imported = path.join(testDir, 'imported.js')
    const outdir = path.join(testDir, 'out')
    const metafile = path.join(testDir, 'meta.json')
    await writeFileAsync(entry1, `
      import x, {f1} from "./${path.basename(imported)}"
      console.log(1, x, f1())
      export {x}
    `)
    await writeFileAsync(entry2, `
      import x, {f2} from "./${path.basename(imported)}"
      console.log('entry 2', x, f2())
      export {x as y}
    `)
    await writeFileAsync(imported, `
      export default 123
      export function f1() {}
      export function f2() {}
      console.log('shared')
    `)
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
    const makeOutPath = basename => path.relative(cwd, path.join(outdir, basename)).split(path.sep).join('/')
    const makeInPath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')

    // Check metafile
    const inEntry1 = makeInPath(entry1);
    const inEntry2 = makeInPath(entry2);
    const inImported = makeInPath(imported);
    const chunk = 'chunk.F5FFNMME.js';
    const outEntry1 = makeOutPath(path.basename(entry1));
    const outEntry2 = makeOutPath(path.basename(entry2));
    const outChunk = makeOutPath(chunk);

    assert.deepStrictEqual(json.inputs[inEntry1], { bytes: 94, imports: [{ path: inImported }] })
    assert.deepStrictEqual(json.inputs[inEntry2], { bytes: 107, imports: [{ path: inImported }] })
    assert.deepStrictEqual(json.inputs[inImported], { bytes: 118, imports: [] })

    assert.deepStrictEqual(json.outputs[outEntry1].imports, [{ path: makeOutPath(chunk) }])
    assert.deepStrictEqual(json.outputs[outEntry2].imports, [{ path: makeOutPath(chunk) }])
    assert.deepStrictEqual(json.outputs[outChunk].imports, [])

    assert.deepStrictEqual(json.outputs[outEntry1].exports, ['x'])
    assert.deepStrictEqual(json.outputs[outEntry2].exports, ['y'])
    assert.deepStrictEqual(json.outputs[outChunk].exports, ['imported_default'])

    assert.deepStrictEqual(json.outputs[outEntry1].inputs, { [inImported]: { bytesInOutput: 18 }, [inEntry1]: { bytesInOutput: 40 } })
    assert.deepStrictEqual(json.outputs[outEntry2].inputs, { [inImported]: { bytesInOutput: 18 }, [inEntry2]: { bytesInOutput: 48 } })
    assert.deepStrictEqual(json.outputs[outChunk].inputs, { [inImported]: { bytesInOutput: 51 } })
  },

  async metafileSplittingPublicPath({ esbuild, testDir }) {
    const entry1 = path.join(testDir, 'entry1.js')
    const entry2 = path.join(testDir, 'entry2.js')
    const imported = path.join(testDir, 'imported.js')
    const outdir = path.join(testDir, 'out')
    const metafile = path.join(testDir, 'meta.json')
    await writeFileAsync(entry1, `
      import x, {f1} from "./${path.basename(imported)}"
      console.log(1, x, f1())
      export {x}
    `)
    await writeFileAsync(entry2, `
      import x, {f2} from "./${path.basename(imported)}"
      console.log('entry 2', x, f2())
      export {x as y}
    `)
    await writeFileAsync(imported, `
      export default 123
      export function f1() {}
      export function f2() {}
      console.log('shared')
    `)
    await esbuild.build({
      entryPoints: [entry1, entry2],
      bundle: true,
      outdir,
      metafile,
      splitting: true,
      format: 'esm',
      publicPath: 'public',
    })

    const json = JSON.parse(await readFileAsync(metafile))
    assert.strictEqual(Object.keys(json.inputs).length, 3)
    assert.strictEqual(Object.keys(json.outputs).length, 3)
    const cwd = process.cwd()
    const makeOutPath = basename => path.relative(cwd, path.join(outdir, basename)).split(path.sep).join('/')
    const makeInPath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')

    // Check metafile
    const inEntry1 = makeInPath(entry1);
    const inEntry2 = makeInPath(entry2);
    const inImported = makeInPath(imported);
    const chunk = 'chunk.CAAA46JO.js';
    const outEntry1 = makeOutPath(path.basename(entry1));
    const outEntry2 = makeOutPath(path.basename(entry2));
    const outChunk = makeOutPath(chunk);

    assert.deepStrictEqual(json.inputs[inEntry1], { bytes: 94, imports: [{ path: inImported }] })
    assert.deepStrictEqual(json.inputs[inEntry2], { bytes: 107, imports: [{ path: inImported }] })
    assert.deepStrictEqual(json.inputs[inImported], { bytes: 118, imports: [] })

    assert.deepStrictEqual(json.outputs[outEntry1].imports, [{ path: makeOutPath(chunk) }])
    assert.deepStrictEqual(json.outputs[outEntry2].imports, [{ path: makeOutPath(chunk) }])
    assert.deepStrictEqual(json.outputs[outChunk].imports, [])

    assert.deepStrictEqual(json.outputs[outEntry1].exports, ['x'])
    assert.deepStrictEqual(json.outputs[outEntry2].exports, ['y'])
    assert.deepStrictEqual(json.outputs[outChunk].exports, ['imported_default'])

    assert.deepStrictEqual(json.outputs[outEntry1].inputs, { [inImported]: { bytesInOutput: 18 }, [inEntry1]: { bytesInOutput: 40 } })
    assert.deepStrictEqual(json.outputs[outEntry2].inputs, { [inImported]: { bytesInOutput: 18 }, [inEntry2]: { bytesInOutput: 48 } })
    assert.deepStrictEqual(json.outputs[outChunk].inputs, { [inImported]: { bytesInOutput: 51 } })
  },

  async metafileCJSInFormatIIFE({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const outfile = path.join(testDir, 'out.js')
    const metafile = path.join(testDir, 'meta.json')
    await writeFileAsync(entry, `module.exports = {}`)
    await esbuild.build({
      entryPoints: [entry],
      outfile,
      metafile,
      format: 'iife',
    })
    const cwd = process.cwd()
    const makePath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')
    const json = JSON.parse(await readFileAsync(metafile))
    assert.deepStrictEqual(json.inputs[makePath(entry)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].exports, [])
  },

  async metafileCJSInFormatCJS({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const outfile = path.join(testDir, 'out.js')
    const metafile = path.join(testDir, 'meta.json')
    await writeFileAsync(entry, `module.exports = {}`)
    await esbuild.build({
      entryPoints: [entry],
      outfile,
      metafile,
      format: 'cjs',
    })
    const cwd = process.cwd()
    const makePath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')
    const json = JSON.parse(await readFileAsync(metafile))
    assert.deepStrictEqual(json.inputs[makePath(entry)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].exports, [])
  },

  async metafileCJSInFormatESM({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const outfile = path.join(testDir, 'out.js')
    const metafile = path.join(testDir, 'meta.json')
    await writeFileAsync(entry, `module.exports = {}`)
    await esbuild.build({
      entryPoints: [entry],
      outfile,
      metafile,
      format: 'esm',
    })
    const cwd = process.cwd()
    const makePath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')
    const json = JSON.parse(await readFileAsync(metafile))
    assert.deepStrictEqual(json.inputs[makePath(entry)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].exports, ['default'])
  },

  async metafileESMInFormatIIFE({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const outfile = path.join(testDir, 'out.js')
    const metafile = path.join(testDir, 'meta.json')
    await writeFileAsync(entry, `export let a = 1, b = 2`)
    await esbuild.build({
      entryPoints: [entry],
      outfile,
      metafile,
      format: 'iife',
    })
    const cwd = process.cwd()
    const makePath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')
    const json = JSON.parse(await readFileAsync(metafile))
    assert.deepStrictEqual(json.inputs[makePath(entry)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].exports, [])
  },

  async metafileESMInFormatCJS({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const outfile = path.join(testDir, 'out.js')
    const metafile = path.join(testDir, 'meta.json')
    await writeFileAsync(entry, `export let a = 1, b = 2`)
    await esbuild.build({
      entryPoints: [entry],
      outfile,
      metafile,
      format: 'cjs',
    })
    const cwd = process.cwd()
    const makePath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')
    const json = JSON.parse(await readFileAsync(metafile))
    assert.deepStrictEqual(json.inputs[makePath(entry)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].exports, [])
  },

  async metafileESMInFormatESM({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const outfile = path.join(testDir, 'out.js')
    const metafile = path.join(testDir, 'meta.json')
    await writeFileAsync(entry, `export let a = 1, b = 2`)
    await esbuild.build({
      entryPoints: [entry],
      outfile,
      metafile,
      format: 'esm',
    })
    const cwd = process.cwd()
    const makePath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')
    const json = JSON.parse(await readFileAsync(metafile))
    assert.deepStrictEqual(json.inputs[makePath(entry)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].exports, ['a', 'b'])
  },

  async metafileNestedExportNames({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const nested1 = path.join(testDir, 'nested1.js')
    const nested2 = path.join(testDir, 'nested2.js')
    const nested3 = path.join(testDir, 'nested3.js')
    const outfile = path.join(testDir, 'out.js')
    const metafile = path.join(testDir, 'meta.json')
    await writeFileAsync(entry, `
      export {nested1} from ${JSON.stringify(nested1)}
      export * from ${JSON.stringify(nested2)}
      export let topLevel = 0
    `)
    await writeFileAsync(nested1, `
      import {nested3} from ${JSON.stringify(nested3)}
      export default 1
      export let nested1 = nested3
    `)
    await writeFileAsync(nested2, `
      export default 'nested2'
      export let nested2 = 2
    `)
    await writeFileAsync(nested3, `
      export let nested3 = 3
    `)
    await esbuild.build({
      entryPoints: [entry],
      bundle: true,
      outfile,
      metafile,
      format: 'esm',
    })
    const cwd = process.cwd()
    const makePath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')
    const json = JSON.parse(await readFileAsync(metafile))
    assert.deepStrictEqual(json.inputs[makePath(entry)].imports, [{ path: makePath(nested1) }, { path: makePath(nested2) }])
    assert.deepStrictEqual(json.inputs[makePath(nested1)].imports, [{ path: makePath(nested3) }])
    assert.deepStrictEqual(json.inputs[makePath(nested2)].imports, [])
    assert.deepStrictEqual(json.inputs[makePath(nested3)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].exports, ['nested1', 'nested2', 'topLevel'])
  },

  async metafileCSS({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.css')
    const imported = path.join(testDir, 'imported.css')
    const image = path.join(testDir, 'example.png')
    const output = path.join(testDir, 'out.css')
    const meta = path.join(testDir, 'meta.json')
    await writeFileAsync(entry, `
      @import "./imported";
      body { background: url(https://example.com/external.png) }
    `)
    await writeFileAsync(imported, `
      a { background: url(./example.png) }
    `)
    await writeFileAsync(image, 'an image')
    await esbuild.build({
      entryPoints: [entry],
      bundle: true,
      outfile: output,
      metafile: meta,
      sourcemap: true,
      loader: { '.png': 'dataurl' },
    })

    const json = JSON.parse(await readFileAsync(meta))
    assert.strictEqual(Object.keys(json.inputs).length, 3)
    assert.strictEqual(Object.keys(json.outputs).length, 1)
    const cwd = process.cwd()
    const makePath = absPath => path.relative(cwd, absPath).split(path.sep).join('/')

    // Check inputs
    assert.deepStrictEqual(json, {
      inputs: {
        [makePath(entry)]: { bytes: 98, imports: [{ path: makePath(imported) }] },
        [makePath(image)]: { bytes: 8, imports: [] },
        [makePath(imported)]: { bytes: 48, imports: [{ path: makePath(image) }] },
      },
      outputs: {
        [makePath(output)]: {
          bytes: 227,
          imports: [],
          inputs: {
            [makePath(entry)]: { bytesInOutput: 62 },
            [makePath(imported)]: { bytesInOutput: 61 },
          },
        },
      },
    })
  },

  // Test in-memory output files
  async writeFalse({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    const metafile = path.join(testDir, 'meta.json')
    const inputCode = 'console.log()'
    await writeFileAsync(input, inputCode)

    const value = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outfile: output,
      sourcemap: true,
      format: 'esm',
      metafile,
      write: false,
    })

    assert.strictEqual(fs.existsSync(output), false)
    assert.notStrictEqual(value.outputFiles, void 0)
    assert.strictEqual(value.outputFiles.length, 3)
    assert.strictEqual(value.outputFiles[0].path, output + '.map')
    assert.strictEqual(value.outputFiles[0].contents.constructor, Uint8Array)
    assert.strictEqual(value.outputFiles[1].path, output)
    assert.strictEqual(value.outputFiles[1].contents.constructor, Uint8Array)
    assert.strictEqual(value.outputFiles[2].path, metafile)
    assert.strictEqual(value.outputFiles[2].contents.constructor, Uint8Array)

    const sourceMap = JSON.parse(Buffer.from(value.outputFiles[0].contents).toString())
    const js = Buffer.from(value.outputFiles[1].contents).toString()
    assert.strictEqual(sourceMap.version, 3)
    assert.strictEqual(js, `// scripts/.js-api-tests/writeFalse/in.js\nconsole.log();\n//# sourceMappingURL=out.js.map\n`)

    const cwd = process.cwd()
    const makePath = file => path.relative(cwd, file).split(path.sep).join('/')
    const meta = JSON.parse(Buffer.from(value.outputFiles[2].contents).toString())
    assert.strictEqual(meta.inputs[makePath(input)].bytes, inputCode.length)
    assert.strictEqual(meta.outputs[makePath(output)].bytes, js.length)
    assert.strictEqual(meta.outputs[makePath(output + '.map')].bytes, value.outputFiles[0].contents.length)
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
    const chunk = 'chunk.CCY6SQWP.js'
    assert.strictEqual(Buffer.from(value.outputFiles[0].contents).toString(), `import {
  common_default
} from "./${chunk}";

// scripts/.js-api-tests/splittingRelativeSameDir/a.js
console.log("a" + common_default);
`)
    assert.strictEqual(Buffer.from(value.outputFiles[1].contents).toString(), `import {
  common_default
} from "./${chunk}";

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
    assert.strictEqual(value.outputFiles[2].path, path.join(outdir, chunk))
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
    const chunk = 'chunk.5UCSUUDJ.js'
    assert.strictEqual(Buffer.from(value.outputFiles[0].contents).toString(), `import {
  common_default
} from "../${chunk}";

// scripts/.js-api-tests/splittingRelativeNestedDir/a/demo.js
console.log("a" + common_default);
`)
    assert.strictEqual(Buffer.from(value.outputFiles[1].contents).toString(), `import {
  common_default
} from "../${chunk}";

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
    assert.strictEqual(value.outputFiles[2].path, path.join(outdir, chunk))
  },

  async stdinStdoutBundle({ esbuild, testDir }) {
    const auxiliary = path.join(testDir, 'auxiliary.js')
    await writeFileAsync(auxiliary, 'export default 123')
    const value = await esbuild.build({
      stdin: {
        contents: `
          import x from './auxiliary.js'
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
  // scripts/.js-api-tests/stdinStdoutBundle/auxiliary.js
  var auxiliary_default = 123;

  // <stdin>
  console.log(auxiliary_default);
})();
`)
  },

  async stdinOutfileBundle({ esbuild, testDir }) {
    const auxiliary = path.join(testDir, 'auxiliary.js')
    const outfile = path.join(testDir, 'out.js')
    await writeFileAsync(auxiliary, 'export default 123')
    const value = await esbuild.build({
      stdin: {
        contents: `
          import x from './auxiliary.js'
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
    const srcDir = path.join(testDir, 'src')
    const entry = path.join(srcDir, 'entry.js')
    const auxiliary = path.join(srcDir, 'auxiliary.js')
    const outdir = path.join(testDir, 'out')
    await mkdirAsync(srcDir)
    await writeFileAsync(auxiliary, 'export default 123')
    await writeFileAsync(entry, `
      import x from './auxiliary.js'
      export let fromEntry = x
    `)
    const value = await esbuild.build({
      entryPoints: [entry],
      stdin: {
        contents: `
          import x from './src/auxiliary.js'
          export {x as fromStdin}
        `,
        resolveDir: testDir,
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

  async forceTsConfig({ esbuild, testDir }) {
    // ./tsconfig.json
    // ./a/forced-config.json
    // ./a/b/test-impl.js
    // ./a/b/c/in.js
    const aDir = path.join(testDir, 'a')
    const bDir = path.join(aDir, 'b')
    const cDir = path.join(bDir, 'c')
    await mkdirAsync(aDir).catch(x => x)
    await mkdirAsync(bDir).catch(x => x)
    await mkdirAsync(cDir).catch(x => x)
    const input = path.join(cDir, 'in.js')
    const forced = path.join(bDir, 'test-impl.js')
    const tsconfigIgnore = path.join(testDir, 'tsconfig.json')
    const tsconfigForced = path.join(aDir, 'forced-config.json')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, 'import "test"')
    await writeFileAsync(forced, 'console.log("success")')
    await writeFileAsync(tsconfigIgnore, '{"compilerOptions": {"baseUrl": "./a", "paths": {"test": ["./ignore.js"]}}}')
    await writeFileAsync(tsconfigForced, '{"compilerOptions": {"baseUrl": "./b", "paths": {"test": ["./test-impl.js"]}}}')
    await esbuild.build({ entryPoints: [input], bundle: true, outfile: output, tsconfig: tsconfigForced, format: 'esm' })
    const result = await readFileAsync(output, 'utf8')
    assert.strictEqual(result, `// scripts/.js-api-tests/forceTsConfig/a/b/test-impl.js
console.log("success");
`)
  },

  async es5({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const cjs = path.join(testDir, 'cjs.js')
    const esm = path.join(testDir, 'esm.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `
      export {foo} from "./cjs"
      export * as bar from "./esm"
    `)
    await writeFileAsync(cjs, 'exports.foo = 123')
    await writeFileAsync(esm, 'export var foo = 123')
    const value = await esbuild.build({ entryPoints: [input], bundle: true, outfile: output, format: 'cjs', target: 'es5' })
    assert.strictEqual(value.outputFiles, void 0)
    const result = require(output)
    assert.strictEqual(result.foo, 123)
    assert.strictEqual(result.bar.foo, 123)
    assert.strictEqual(result.__esModule, true)
    const contents = await readFileAsync(output, 'utf8')
    assert.strictEqual(contents.indexOf('=>'), -1)
    assert.strictEqual(contents.indexOf('const'), -1)
  },

  async outbase({ esbuild, testDir }) {
    const outbase = path.join(testDir, 'pages')
    const b = path.join(outbase, 'a', 'b', 'index.js')
    const c = path.join(outbase, 'a', 'c', 'index.js')
    const outdir = path.join(testDir, 'outdir')
    await mkdirAsync(path.dirname(b), { recursive: true })
    await mkdirAsync(path.dirname(c), { recursive: true })
    await writeFileAsync(b, 'module.exports = "b"')
    await writeFileAsync(c, 'module.exports = "c"')
    await esbuild.build({ entryPoints: [b, c], outdir, outbase, format: 'cjs' })
    const outB = path.join(outdir, path.relative(outbase, b))
    const outC = path.join(outdir, path.relative(outbase, c))
    assert.strictEqual(require(outB), 'b')
    assert.strictEqual(require(outC), 'c')
  },

  async bundleTreeShakingDefault({ esbuild }) {
    const { outputFiles } = await esbuild.build({
      stdin: {
        contents: `
          let removeMe1 = /* @__PURE__ */ fn();
          let removeMe2 = <div/>;
        `,
        loader: 'jsx',
      },
      write: false,
      bundle: true,
    })
    assert.strictEqual(outputFiles[0].text, `(() => {\n})();\n`)
  },

  async bundleTreeShakingTrue({ esbuild }) {
    const { outputFiles } = await esbuild.build({
      stdin: {
        contents: `
          let removeMe1 = /* @__PURE__ */ fn();
          let removeMe2 = <div/>;
        `,
        loader: 'jsx',
      },
      write: false,
      bundle: true,
      treeShaking: true,
    })
    assert.strictEqual(outputFiles[0].text, `(() => {\n})();\n`)
  },

  async bundleTreeShakingIgnoreAnnotations({ esbuild }) {
    const { outputFiles } = await esbuild.build({
      stdin: {
        contents: `
          let keepMe1 = /* @__PURE__ */ fn();
          let keepMe2 = <div/>;
        `,
        loader: 'jsx',
      },
      write: false,
      bundle: true,
      treeShaking: 'ignore-annotations',
    })
    assert.strictEqual(outputFiles[0].text, `(() => {
  // <stdin>
  var keepMe1 = fn();
  var keepMe2 = React.createElement("div", null);
})();
`)
  },

  async externalWithWildcard({ esbuild }) {
    const { outputFiles } = await esbuild.build({
      stdin: {
        contents: `require('/assets/file.png')`,
      },
      write: false,
      bundle: true,
      external: ['/assets/*.png'],
    })
    assert.strictEqual(outputFiles[0].text, `(() => {
  // <stdin>
  require("/assets/file.png");
})();
`)
  },

  async errorInvalidExternalWithTwoWildcards({ esbuild }) {
    try {
      await esbuild.build({ entryPoints: ['in.js'], external: ['a*b*c'], write: false, logLevel: 'silent' })
      throw new Error('Expected build failure');
    } catch (e) {
      if (e.message !== 'Build failed with 1 error:\nerror: External path "a*b*c" cannot have more than one "*" wildcard') {
        throw e;
      }
    }
  },

  async jsBannerBuild({ service, testDir }) {
    const input = path.join(testDir, 'in.js')
    const outfile = path.join(testDir, 'out.js')
    await writeFileAsync(input, `if (!bannerDefined) throw 'fail'`)
    await service.build({ entryPoints: [input], outfile, banner: 'const bannerDefined = true' })
    require(outfile)
  },

  async jsFooterBuild({ service, testDir }) {
    const input = path.join(testDir, 'in.js')
    const outfile = path.join(testDir, 'out.js')
    await writeFileAsync(input, `footer()`)
    await service.build({ entryPoints: [input], outfile, footer: 'function footer() {}' })
    require(outfile)
  },

  async jsBannerFooterBuild({ service, testDir }) {
    const aPath = path.join(testDir, 'a.js')
    const bPath = path.join(testDir, 'b.js')
    const outdir = path.join(testDir, 'out')
    await writeFileAsync(aPath, `module.exports = { banner: bannerDefined, footer };`)
    await writeFileAsync(bPath, `module.exports = { banner: bannerDefined, footer };`)
    await service.build({ entryPoints: [aPath, bPath], outdir, banner: 'const bannerDefined = true', footer: 'function footer() {}' })
    const a = require(path.join(outdir, path.basename(aPath)))
    const b = require(path.join(outdir, path.basename(bPath)))
    if (!a.banner || !b.banner) throw 'fail'
    a.footer()
    b.footer()
  },

  async cssBannerFooterBuild({ service, testDir }) {
    const input = path.join(testDir, 'in.css')
    const outfile = path.join(testDir, 'out.css')
    await writeFileAsync(input, `div { color: red }`)
    await service.build({ entryPoints: [input], outfile, banner: '/* banner */', footer: '/* footer */' })
    const code = await readFileAsync(outfile, 'utf8')
    assert.strictEqual(code, `div {\n  color: red;\n}\n`)
  },

  async noRebuild({ esbuild, service, testDir }) {
    for (const toTest of [esbuild, service]) {
      const input = path.join(testDir, 'in.js')
      const output = path.join(testDir, 'out.js')
      await writeFileAsync(input, `console.log('abc')`)
      const result1 = await toTest.build({ entryPoints: [input], outfile: output, format: 'esm', incremental: false })
      assert.strictEqual(result1.outputFiles, void 0)
      assert.strictEqual(await readFileAsync(output, 'utf8'), `console.log("abc");\n`)
      assert.strictEqual(result1.rebuild, void 0)
    }
  },

  async rebuildBasic({ esbuild, service, testDir }) {
    for (const toTest of [esbuild, service]) {
      const input = path.join(testDir, 'in.js')
      const output = path.join(testDir, 'out.js')

      // Build 1
      await writeFileAsync(input, `console.log('abc')`)
      const result1 = await toTest.build({ entryPoints: [input], outfile: output, format: 'esm', incremental: true })
      assert.strictEqual(result1.outputFiles, void 0)
      assert.strictEqual(await readFileAsync(output, 'utf8'), `console.log("abc");\n`)

      // Build 2
      await writeFileAsync(input, `console.log('xyz')`)
      const result2 = await result1.rebuild();
      assert.strictEqual(result2.rebuild, result1.rebuild)
      assert.strictEqual(result2.outputFiles, void 0)
      assert.strictEqual(await readFileAsync(output, 'utf8'), `console.log("xyz");\n`)

      // Build 3
      await writeFileAsync(input, `console.log(123)`)
      const result3 = await result1.rebuild();
      assert.strictEqual(result3.rebuild, result1.rebuild)
      assert.strictEqual(result3.outputFiles, void 0)
      assert.strictEqual(await readFileAsync(output, 'utf8'), `console.log(123);\n`)

      // Further rebuilds should not be possible after a dispose
      result1.rebuild.dispose()
      try {
        await result1.rebuild()
        throw new Error('Expected an error to be thrown')
      } catch (e) {
        assert.strictEqual(e.message, 'Cannot rebuild')
      }
    }
  },

  async rebuildIndependent({ esbuild, service, testDir }) {
    for (const toTest of [esbuild, service]) {
      const inputA = path.join(testDir, 'in-a.js')
      const inputB = path.join(testDir, 'in-b.js')
      const outputA = path.join(testDir, 'out-a.js')
      const outputB = path.join(testDir, 'out-b.js')

      // Build 1
      await writeFileAsync(inputA, `console.log('a')`)
      await writeFileAsync(inputB, `console.log('b')`)
      const resultA1 = await toTest.build({ entryPoints: [inputA], outfile: outputA, format: 'esm', incremental: true })
      const resultB1 = await toTest.build({ entryPoints: [inputB], outfile: outputB, format: 'esm', incremental: true })
      assert.notStrictEqual(resultA1.rebuild, resultB1.rebuild)
      assert.strictEqual(resultA1.outputFiles, void 0)
      assert.strictEqual(resultB1.outputFiles, void 0)
      assert.strictEqual(await readFileAsync(outputA, 'utf8'), `console.log("a");\n`)
      assert.strictEqual(await readFileAsync(outputB, 'utf8'), `console.log("b");\n`)

      // Build 2
      await writeFileAsync(inputA, `console.log(1)`)
      await writeFileAsync(inputB, `console.log(2)`)
      const promiseA = resultA1.rebuild();
      const promiseB = resultB1.rebuild();
      const resultA2 = await promiseA;
      const resultB2 = await promiseB;
      assert.strictEqual(resultA2.rebuild, resultA1.rebuild)
      assert.strictEqual(resultB2.rebuild, resultB1.rebuild)
      assert.strictEqual(resultA2.outputFiles, void 0)
      assert.strictEqual(resultB2.outputFiles, void 0)
      assert.strictEqual(await readFileAsync(outputA, 'utf8'), `console.log(1);\n`)
      assert.strictEqual(await readFileAsync(outputB, 'utf8'), `console.log(2);\n`)

      // Further rebuilds should not be possible after a dispose
      resultA1.rebuild.dispose()
      try {
        await resultA1.rebuild()
        throw new Error('Expected an error to be thrown')
      } catch (e) {
        assert.strictEqual(e.message, 'Cannot rebuild')
      }

      // Build 3
      await writeFileAsync(inputB, `console.log(3)`)
      const resultB3 = await resultB1.rebuild()
      assert.strictEqual(resultB3.rebuild, resultB1.rebuild)
      assert.strictEqual(resultB3.outputFiles, void 0)
      assert.strictEqual(await readFileAsync(outputB, 'utf8'), `console.log(3);\n`)

      // Further rebuilds should not be possible after a dispose
      resultB1.rebuild.dispose()
      try {
        await resultB1.rebuild()
        throw new Error('Expected an error to be thrown')
      } catch (e) {
        assert.strictEqual(e.message, 'Cannot rebuild')
      }
    }
  },

  async rebuildParallel({ esbuild, service, testDir }) {
    for (const toTest of [esbuild, service]) {
      const input = path.join(testDir, 'in.js')
      const output = path.join(testDir, 'out.js')

      // Build 1
      await writeFileAsync(input, `console.log('abc')`)
      const result1 = await toTest.build({ entryPoints: [input], outfile: output, format: 'esm', incremental: true })
      assert.strictEqual(result1.outputFiles, void 0)
      assert.strictEqual(await readFileAsync(output, 'utf8'), `console.log("abc");\n`)

      // Build 2
      await writeFileAsync(input, `console.log('xyz')`)
      const promise2A = result1.rebuild();
      const promise2B = result1.rebuild();
      const result2A = await promise2A;
      const result2B = await promise2B;
      assert.strictEqual(result2A.rebuild, result1.rebuild)
      assert.strictEqual(result2B.rebuild, result1.rebuild)
      assert.strictEqual(result2A.outputFiles, void 0)
      assert.strictEqual(result2B.outputFiles, void 0)
      assert.strictEqual(await readFileAsync(output, 'utf8'), `console.log("xyz");\n`)

      // Build 3
      await writeFileAsync(input, `console.log(123)`)
      const promise3A = result1.rebuild();
      const promise3B = result1.rebuild();
      const result3A = await promise3A;
      const result3B = await promise3B;
      assert.strictEqual(result3A.rebuild, result1.rebuild)
      assert.strictEqual(result3B.rebuild, result1.rebuild)
      assert.strictEqual(result3A.outputFiles, void 0)
      assert.strictEqual(result3B.outputFiles, void 0)
      assert.strictEqual(await readFileAsync(output, 'utf8'), `console.log(123);\n`)

      // Further rebuilds should not be possible after a dispose
      result1.rebuild.dispose()
      try {
        await result1.rebuild()
        throw new Error('Expected an error to be thrown')
      } catch (e) {
        assert.strictEqual(e.message, 'Cannot rebuild')
      }
    }
  },

  async bundleAvoidTDZ({ service }) {
    var { outputFiles } = await service.build({
      stdin: {
        contents: `
          class Foo {
            // The above line will be transformed into "var". However, the
            // symbol "Foo" must still be defined before the class body ends.
            static foo = new Foo
          }
          if (!(Foo.foo instanceof Foo))
            throw 'fail'
        `,
      },
      bundle: true,
      write: false,
    })
    assert.strictEqual(outputFiles.length, 1)
    new Function(outputFiles[0].text)()
  },

  async bundleTSAvoidTDZ({ service }) {
    var { outputFiles } = await service.build({
      stdin: {
        contents: `
          class Foo {
            // The above line will be transformed into "var". However, the
            // symbol "Foo" must still be defined before the class body ends.
            static foo = new Foo
          }
          if (!(Foo.foo instanceof Foo))
            throw 'fail'
        `,
        loader: 'ts',
      },
      bundle: true,
      write: false,
    })
    assert.strictEqual(outputFiles.length, 1)
    new Function(outputFiles[0].text)()
  },

  async bundleTSDecoratorAvoidTDZ({ service }) {
    var { outputFiles } = await service.build({
      stdin: {
        contents: `
          class Bar {}
          var oldFoo
          function swap(target) {
            oldFoo = target
            return Bar
          }
          @swap
          class Foo {
            bar() { return new Foo }
            static foo = new Foo
          }
          if (!(oldFoo.foo instanceof oldFoo))
            throw 'fail: foo'
          if (!(oldFoo.foo.bar() instanceof Bar))
            throw 'fail: bar'
        `,
        loader: 'ts',
      },
      bundle: true,
      write: false,
    })
    assert.strictEqual(outputFiles.length, 1)
    new Function(outputFiles[0].text)()
  },
}

let serveTests = {
  async basic({ esbuild, service, testDir }) {
    for (const toTest of [esbuild, service]) {
      const input = path.join(testDir, 'in.js')
      await writeFileAsync(input, `console.log(123)`)

      let onRequest;
      let singleRequestPromise = new Promise(resolve => {
        onRequest = resolve;
      });

      const result = await toTest.serve({ onRequest }, { entryPoints: [input], format: 'esm' })
      assert.strictEqual(result.host, '127.0.0.1');
      assert.strictEqual(typeof result.port, 'number');

      const buffer = await new Promise((resolve, reject) => {
        http.get({
          host: result.host,
          port: result.port,
          path: '/in.js',
        }, res => {
          const chunks = []
          res.on('data', chunk => chunks.push(chunk))
          res.on('end', () => resolve(Buffer.concat(chunks)))
        }).on('error', reject)
      })
      assert.strictEqual(buffer.toString(), `console.log(123);\n`);

      let singleRequest = await singleRequestPromise;
      assert.strictEqual(singleRequest.method, 'GET');
      assert.strictEqual(singleRequest.path, '/in.js');
      assert.strictEqual(singleRequest.status, 200);
      assert.strictEqual(typeof singleRequest.remoteAddress, 'string');
      assert.strictEqual(typeof singleRequest.timeInMS, 'number');

      result.stop();
      await result.wait;
    }
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
  async transformWithNonString({ esbuild, service }) {
    for (let toTest of [esbuild, service]) {
      try {
        // Do not "await" here. The error should be thrown outside of the promise.
        toTest.transform({ toString() { throw new Error('toString() error') } })
        throw new Error('Expected an error to be thrown');
      } catch (e) {
        assert.strictEqual(e.message, 'toString() error')
      }

      var { code } = await toTest.transform(Buffer.from(`1+2`))
      assert.strictEqual(code, `1 + 2;\n`)
    }
  },

  async version({ esbuild }) {
    const version = fs.readFileSync(path.join(repoDir, 'version.txt'), 'utf8').trim()
    assert.strictEqual(esbuild.version, version);
  },

  async ignoreUndefinedOptions({ service }) {
    // This should not throw
    await service.transform(``, { jsxFactory: void 0 })
  },

  async ignoreUndefinedOptions({ service }) {
    // This should throw
    try {
      await service.transform(``, { jsxFactory: ['React', 'createElement'] })
      throw new Error('Expected transform failure');
    } catch (e) {
      if (e.message !== '"jsxFactory" must be a string') {
        throw e;
      }
    }
  },

  async avoidTDZ({ service }) {
    var { code } = await service.transform(`
      class Foo {
        // The above line will be transformed into "var". However, the
        // symbol "Foo" must still be defined before the class body ends.
        static foo = new Foo
      }
      if (!(Foo.foo instanceof Foo))
        throw 'fail'
    `)
    new Function(code)()
  },

  async tsAvoidTDZ({ service }) {
    var { code } = await service.transform(`
      class Foo {
        // The above line will be transformed into "var". However, the
        // symbol "Foo" must still be defined before the class body ends.
        static foo = new Foo
      }
      if (!(Foo.foo instanceof Foo))
        throw 'fail'
    `, {
      loader: 'ts',
    })
    new Function(code)()
  },

  async tsDecoratorAvoidTDZ({ service }) {
    var { code } = await service.transform(`
      class Bar {}
      var oldFoo
      function swap(target) {
        oldFoo = target
        return Bar
      }
      @swap
      class Foo {
        bar() { return new Foo }
        static foo = new Foo
      }
      if (!(oldFoo.foo instanceof oldFoo))
        throw 'fail: foo'
      if (!(oldFoo.foo.bar() instanceof Bar))
        throw 'fail: bar'
    `, {
      loader: 'ts',
    })
    new Function(code)()
  },

  async jsBannerTransform({ service }) {
    var { code } = await service.transform(`
      if (!bannerDefined) throw 'fail'
    `, {
      banner: 'const bannerDefined = true',
    })
    new Function(code)()
  },

  async jsFooterTransform({ service }) {
    var { code } = await service.transform(`
      footer()
    `, {
      footer: 'function footer() {}',
    })
    new Function(code)()
    new Function(code)()
  },

  async jsBannerFooterTransform({ service }) {
    var { code } = await service.transform(`
      return { banner: bannerDefined, footer };
    `, {
      banner: 'const bannerDefined = true',
      footer: 'function footer() {}',
    })
    const result = new Function(code)()
    if (!result.banner) throw 'fail'
    result.footer()
  },

  async cssBannerFooterTransform({ service }) {
    var { code } = await service.transform(`
      div { color: red }
    `, {
      loader: 'css',
      banner: '/* banner */',
      footer: '/* footer */',
    })
    assert.strictEqual(code, `div {\n  color: red;\n}\n`)
  },

  async transformDirectEval({ service }) {
    var { code } = await service.transform(`
      export let abc = 123
      eval('console.log(abc)')
    `, {
      minify: true,
    })
    assert.strictEqual(code, `export let abc=123;eval("console.log(abc)");\n`)
  },

  async tsconfigRaw({ service }) {
    const { code: code1 } = await service.transform(`import {T} from 'path'`, {
      tsconfigRaw: {
        compilerOptions: {
          importsNotUsedAsValues: 'remove',
        },
      },
      loader: 'ts',
    })
    assert.strictEqual(code1, ``)

    const { code: code2 } = await service.transform(`import {T} from 'path'`, {
      tsconfigRaw: {
        compilerOptions: {
          importsNotUsedAsValues: 'preserve',
        },
      },
      loader: 'ts',
    })
    assert.strictEqual(code2, `import "path";\n`)

    // Can use a string, which allows weird TypeScript pseudo-JSON with comments and trailing commas
    const { code: code3 } = await service.transform(`import {T} from 'path'`, {
      tsconfigRaw: `{
        "compilerOptions": {
          "importsNotUsedAsValues": "preserve", // there is a trailing comment here
        },
      }`,
      loader: 'ts',
    })
    assert.strictEqual(code3, `import "path";\n`)
  },

  async tsconfigRawImportsNotUsedAsValues({ service }) {
    const { code: code1 } = await service.transform(`class Foo { foo }`, {
      tsconfigRaw: {
        compilerOptions: {
          useDefineForClassFields: false,
        },
      },
      loader: 'ts',
    })
    assert.strictEqual(code1, `class Foo {\n}\n`)

    const { code: code2 } = await service.transform(`class Foo { foo }`, {
      tsconfigRaw: {
        compilerOptions: {
          useDefineForClassFields: true,
        },
      },
      loader: 'ts',
    })
    assert.strictEqual(code2, `class Foo {\n  foo;\n}\n`)
  },

  async tsconfigRawJSX({ service }) {
    const { code: code1 } = await service.transform(`<><div/></>`, {
      tsconfigRaw: {
        compilerOptions: {
        },
      },
      loader: 'jsx',
    })
    assert.strictEqual(code1, `/* @__PURE__ */ React.createElement(React.Fragment, null, /* @__PURE__ */ React.createElement("div", null));\n`)

    const { code: code2 } = await service.transform(`<><div/></>`, {
      tsconfigRaw: {
        compilerOptions: {
          jsxFactory: 'factory',
          jsxFragmentFactory: 'fragment',
        },
      },
      loader: 'jsx',
    })
    assert.strictEqual(code2, `/* @__PURE__ */ factory(fragment, null, /* @__PURE__ */ factory("div", null));\n`)
  },

  async treeShakingDefault({ service }) {
    const { code } = await service.transform(`/* @__PURE__ */ fn(); <div/>`, {
      loader: 'jsx',
      minifySyntax: true,
    })
    assert.strictEqual(code, ``)
  },

  async treeShakingTrue({ service }) {
    const { code } = await service.transform(`/* @__PURE__ */ fn(); <div/>`, {
      loader: 'jsx',
      minifySyntax: true,
      treeShaking: true,
    })
    assert.strictEqual(code, ``)
  },

  async treeShakingIgnoreAnnotations({ service }) {
    const { code } = await service.transform(`/* @__PURE__ */ fn(); <div/>`, {
      loader: 'jsx',
      minifySyntax: true,
      treeShaking: 'ignore-annotations',
    })
    assert.strictEqual(code, `fn(), React.createElement("div", null);\n`)
  },

  async jsCharsetDefault({ service }) {
    const { code } = await service.transform(`let π = 'π'`, {})
    assert.strictEqual(code, `let \\u03C0 = "\\u03C0";\n`)
  },

  async jsCharsetASCII({ service }) {
    const { code } = await service.transform(`let π = 'π'`, { charset: 'ascii' })
    assert.strictEqual(code, `let \\u03C0 = "\\u03C0";\n`)
  },

  async jsCharsetUTF8({ service }) {
    const { code } = await service.transform(`let π = 'π'`, { charset: 'utf8' })
    assert.strictEqual(code, `let π = "π";\n`)
  },

  async cssCharsetDefault({ service }) {
    const { code } = await service.transform(`.π:after { content: 'π' }`, { loader: 'css' })
    assert.strictEqual(code, `.\\3c0:after {\n  content: "\\3c0";\n}\n`)
  },

  async cssCharsetASCII({ service }) {
    const { code } = await service.transform(`.π:after { content: 'π' }`, { loader: 'css', charset: 'ascii' })
    assert.strictEqual(code, `.\\3c0:after {\n  content: "\\3c0";\n}\n`)
  },

  async cssCharsetUTF8({ service }) {
    const { code } = await service.transform(`.π:after { content: 'π' }`, { loader: 'css', charset: 'utf8' })
    assert.strictEqual(code, `.π:after {\n  content: "π";\n}\n`)
  },

  async cjs_require({ service }) {
    const { code } = await service.transform(`const {foo} = require('path')`, {})
    assert.strictEqual(code, `const {foo} = require("path");\n`)
  },

  async cjs_exports({ service }) {
    const { code } = await service.transform(`exports.foo = 123`, {})
    assert.strictEqual(code, `exports.foo = 123;\n`)
  },

  async es6_import({ service }) {
    const { code } = await service.transform(`import {foo} from 'path'`, {})
    assert.strictEqual(code, `import {foo} from "path";\n`)
  },

  async es6_export({ service }) {
    const { code } = await service.transform(`export const foo = 123`, {})
    assert.strictEqual(code, `export const foo = 123;\n`)
  },

  async es6_import_to_iife({ service }) {
    const { code } = await service.transform(`import {exists} from "fs"; if (!exists) throw 'fail'`, { format: 'iife' })
    new Function('require', code)(require)
  },

  async es6_import_star_to_iife({ service }) {
    const { code } = await service.transform(`import * as fs from "fs"; if (!fs.exists) throw 'fail'`, { format: 'iife' })
    new Function('require', code)(require)
  },

  async es6_export_to_iife({ service }) {
    const { code } = await service.transform(`export {exists} from "fs"`, { format: 'iife', globalName: 'out' })
    const out = new Function('require', code + ';return out')(require)
    if (out.exists !== fs.exists) throw 'fail'
  },

  async es6_export_star_to_iife({ service }) {
    const { code } = await service.transform(`export * from "fs"`, { format: 'iife', globalName: 'out' })
    const out = new Function('require', code + ';return out')(require)
    if (out.exists !== fs.exists) throw 'fail'
  },

  async es6_export_star_as_to_iife({ service }) {
    const { code } = await service.transform(`export * as fs from "fs"`, { format: 'iife', globalName: 'out' })
    const out = new Function('require', code + ';return out')(require)
    if (out.fs.exists !== fs.exists) throw 'fail'
  },

  async es6_import_to_cjs({ service }) {
    const { code } = await service.transform(`import {exists} from "fs"; if (!exists) throw 'fail'`, { format: 'cjs' })
    new Function('require', code)(require)
  },

  async es6_import_star_to_cjs({ service }) {
    const { code } = await service.transform(`import * as fs from "fs"; if (!fs.exists) throw 'fail'`, { format: 'cjs' })
    new Function('require', code)(require)
  },

  async es6_export_to_cjs({ service }) {
    const { code } = await service.transform(`export {exists} from "fs"`, { format: 'cjs' })
    const exports = {}
    new Function('require', 'exports', code)(require, exports)
    if (exports.exists !== fs.exists) throw 'fail'
  },

  async es6_export_star_to_cjs({ service }) {
    const { code } = await service.transform(`export * from "fs"`, { format: 'cjs' })
    const exports = {}
    new Function('require', 'exports', code)(require, exports)
    if (exports.exists !== fs.exists) throw 'fail'
  },

  async es6_export_star_as_to_cjs({ service }) {
    const { code } = await service.transform(`export * as fs from "fs"`, { format: 'cjs' })
    const exports = {}
    new Function('require', 'exports', code)(require, exports)
    if (exports.fs.exists !== fs.exists) throw 'fail'
  },

  async es6_import_to_esm({ service }) {
    const { code } = await service.transform(`import {exists} from "fs"; if (!exists) throw 'fail'`, { format: 'esm' })
    assert.strictEqual(code, `import {exists} from "fs";\nif (!exists)\n  throw "fail";\n`)
  },

  async es6_import_star_to_esm({ service }) {
    const { code } = await service.transform(`import * as fs from "fs"; if (!fs.exists) throw 'fail'`, { format: 'esm' })
    assert.strictEqual(code, `import * as fs from "fs";\nif (!fs.exists)\n  throw "fail";\n`)
  },

  async es6_export_to_esm({ service }) {
    const { code } = await service.transform(`export {exists} from "fs"`, { format: 'esm' })
    assert.strictEqual(code, `import {exists} from "fs";\nexport {\n  exists\n};\n`)
  },

  async es6_export_star_to_esm({ service }) {
    const { code } = await service.transform(`export * from "fs"`, { format: 'esm' })
    assert.strictEqual(code, `export * from "fs";\n`)
  },

  async es6_export_star_as_to_esm({ service }) {
    const { code } = await service.transform(`export * as fs from "fs"`, { format: 'esm' })
    assert.strictEqual(code, `import * as fs from "fs";\nexport {\n  fs\n};\n`)
  },

  async iifeGlobalName({ service }) {
    const { code } = await service.transform(`export default 123`, { format: 'iife', globalName: 'testName' })
    const globals = {}
    vm.createContext(globals)
    vm.runInContext(code, globals)
    assert.strictEqual(globals.testName.default, 123)
  },

  async iifeGlobalNameCompound({ service }) {
    const { code } = await service.transform(`export default 123`, { format: 'iife', globalName: 'test.name' })
    const globals = {}
    vm.createContext(globals)
    vm.runInContext(code, globals)
    assert.strictEqual(globals.test.name.default, 123)
  },

  async iifeGlobalNameString({ service }) {
    const { code } = await service.transform(`export default 123`, { format: 'iife', globalName: 'test["some text"]' })
    const globals = {}
    vm.createContext(globals)
    vm.runInContext(code, globals)
    assert.strictEqual(globals.test['some text'].default, 123)
  },

  async iifeGlobalNameUnicodeEscape({ service }) {
    const { code } = await service.transform(`export default 123`, { format: 'iife', globalName: 'π["π 𐀀"].𐀀["𐀀 π"]' })
    const globals = {}
    vm.createContext(globals)
    vm.runInContext(code, globals)
    assert.strictEqual(globals.π["π 𐀀"].𐀀["𐀀 π"].default, 123)
    assert.strictEqual(code.slice(0, code.indexOf('(() => {\n')), `var \\u03C0 = \\u03C0 || {};
\\u03C0["\\u03C0 \\uD800\\uDC00"] = \\u03C0["\\u03C0 \\uD800\\uDC00"] || {};
\\u03C0["\\u03C0 \\uD800\\uDC00"].\\u{10000} = \\u03C0["\\u03C0 \\uD800\\uDC00"].\\u{10000} || {};
\\u03C0["\\u03C0 \\uD800\\uDC00"].\\u{10000}["\\uD800\\uDC00 \\u03C0"] = `)
  },

  async iifeGlobalNameUnicodeNoEscape({ service }) {
    const { code } = await service.transform(`export default 123`, { format: 'iife', globalName: 'π["π 𐀀"].𐀀["𐀀 π"]', charset: 'utf8' })
    const globals = {}
    vm.createContext(globals)
    vm.runInContext(code, globals)
    assert.strictEqual(globals.π["π 𐀀"].𐀀["𐀀 π"].default, 123)
    assert.strictEqual(code.slice(0, code.indexOf('(() => {\n')),
      `var π = π || {};
π["π 𐀀"] = π["π 𐀀"] || {};
π["π 𐀀"].𐀀 = π["π 𐀀"].𐀀 || {};
π["π 𐀀"].𐀀["𐀀 π"] = `)
  },

  async jsx({ service }) {
    const { code } = await service.transform(`console.log(<div/>)`, { loader: 'jsx' })
    assert.strictEqual(code, `console.log(/* @__PURE__ */ React.createElement("div", null));\n`)
  },

  async ts({ service }) {
    const { code } = await service.transform(`enum Foo { FOO }`, { loader: 'ts' })
    assert.strictEqual(code, `var Foo;\n(function(Foo2) {\n  Foo2[Foo2["FOO"] = 0] = "FOO";\n})(Foo || (Foo = {}));\n`)
  },

  async tsx({ service }) {
    const { code } = await service.transform(`console.log(<Foo<T>/>)`, { loader: 'tsx' })
    assert.strictEqual(code, `console.log(/* @__PURE__ */ React.createElement(Foo, null));\n`)
  },

  async minify({ service }) {
    const { code } = await service.transform(`console.log("a" + "b" + c)`, { minify: true })
    assert.strictEqual(code, `console.log("ab"+c);\n`)
  },

  async define({ service }) {
    const define = { 'process.env.NODE_ENV': '"production"' }
    const { code } = await service.transform(`console.log(process.env.NODE_ENV)`, { define })
    assert.strictEqual(code, `console.log("production");\n`)
  },

  async defineWarning({ service }) {
    const define = { 'process.env.NODE_ENV': 'production' }
    const { code, warnings } = await service.transform(`console.log(process.env.NODE_ENV)`, { define })
    assert.strictEqual(code, `console.log(production);\n`)
    assert.strictEqual(warnings.length, 1)
    assert.strictEqual(warnings[0].text,
      `"process.env.NODE_ENV" is defined as an identifier instead of a string (surround "production" with double quotes to get a string)`)
  },

  async json({ service }) {
    const { code } = await service.transform(`{ "x": "y" }`, { loader: 'json' })
    assert.strictEqual(code, `module.exports = {x: "y"};\n`)
  },

  async jsonMinified({ service }) {
    const { code } = await service.transform(`{ "x": "y" }`, { loader: 'json', minify: true })
    const module = {}
    new Function('module', code)(module)
    assert.deepStrictEqual(module.exports, { x: 'y' })
  },

  async jsonESM({ service }) {
    const { code } = await service.transform(`{ "x": "y" }`, { loader: 'json', format: 'esm' })
    assert.strictEqual(code, `var x = "y";\nvar stdin_default = {x};\nexport {\n  stdin_default as default,\n  x\n};\n`)
  },

  async text({ service }) {
    const { code } = await service.transform(`This is some text`, { loader: 'text' })
    assert.strictEqual(code, `module.exports = "This is some text";\n`)
  },

  async textESM({ service }) {
    const { code } = await service.transform(`This is some text`, { loader: 'text', format: 'esm' })
    assert.strictEqual(code, `var stdin_default = "This is some text";\nexport {\n  stdin_default as default\n};\n`)
  },

  async base64({ service }) {
    const { code } = await service.transform(`\x00\x01\x02`, { loader: 'base64' })
    assert.strictEqual(code, `module.exports = "AAEC";\n`)
  },

  async dataurl({ service }) {
    const { code } = await service.transform(`\x00\x01\x02`, { loader: 'dataurl' })
    assert.strictEqual(code, `module.exports = "data:application/octet-stream;base64,AAEC";\n`)
  },

  async sourceMapWithName({ service }) {
    const { code, map } = await service.transform(`let       x`, { sourcemap: true, sourcefile: 'afile.js' })
    assert.strictEqual(code, `let x;\n`)
    await assertSourceMap(map, 'afile.js')
  },

  async sourceMapExternalWithName({ service }) {
    const { code, map } = await service.transform(`let       x`, { sourcemap: 'external', sourcefile: 'afile.js' })
    assert.strictEqual(code, `let x;\n`)
    await assertSourceMap(map, 'afile.js')
  },

  async sourceMapInlineWithName({ service }) {
    const { code, map } = await service.transform(`let       x`, { sourcemap: 'inline', sourcefile: 'afile.js' })
    assert(code.startsWith(`let x;\n//# sourceMappingURL=`))
    assert.strictEqual(map, '')
    const base64 = code.slice(code.indexOf('base64,') + 'base64,'.length)
    await assertSourceMap(Buffer.from(base64.trim(), 'base64').toString(), 'afile.js')
  },

  async numericLiteralPrinting({ service }) {
    async function checkLiteral(text) {
      const { code } = await service.transform(`return ${text}`, { minify: true })
      assert.strictEqual(+text, new Function(code)())
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

  async transpilersPrinting({ service }) {
    async function check(source, expected, opts) {
      const { code } = await service.transform(source, opts)
      assert.strictEqual(code, expected)
    }
    const promises = [
      check('console.log("a")', 'console.log("a");\n', {}),
      check('console.log("b")', '', { removeConsole: true }),
      check('debugger', 'debugger;\n', {}),
      check('debugger', '', { removeDebugger: true }),
    ]
    await Promise.all(promises)
  },

  async tryCatchScopeMerge({ service }) {
    const code = `
      var x = 1
      if (x !== 1) throw 'fail'
      try {
        throw 2
      } catch (x) {
        if (x !== 2) throw 'fail'
        {
          if (x !== 2) throw 'fail'
          var x = 3
          if (x !== 3) throw 'fail'
        }
        if (x !== 3) throw 'fail'
      }
      if (x !== 1) throw 'fail'
    `;
    new Function(code)(); // Verify that the code itself is correct
    new Function((await service.transform(code)).code)();
  },

  async nestedFunctionHoist({ service }) {
    const code = `
      if (x !== void 0) throw 'fail'
      {
        if (x !== void 0) throw 'fail'
        {
          x()
          function x() {}
          x()
        }
        x()
      }
      x()
    `;
    new Function(code)(); // Verify that the code itself is correct
    new Function((await service.transform(code)).code)();
  },

  async nestedFunctionHoistBefore({ service }) {
    const code = `
      var x = 1
      if (x !== 1) throw 'fail'
      {
        if (x !== 1) throw 'fail'
        {
          x()
          function x() {}
          x()
        }
        x()
      }
      x()
    `;
    new Function(code)(); // Verify that the code itself is correct
    new Function((await service.transform(code)).code)();
  },

  async nestedFunctionHoistAfter({ service }) {
    const code = `
      if (x !== void 0) throw 'fail'
      {
        if (x !== void 0) throw 'fail'
        {
          x()
          function x() {}
          x()
        }
        x()
      }
      x()
      var x = 1
    `;
    new Function(code)(); // Verify that the code itself is correct
    new Function((await service.transform(code)).code)();
  },

  async nestedFunctionShadowBefore({ service }) {
    const code = `
      let x = 1
      if (x !== 1) throw 'fail'
      {
        if (x !== 1) throw 'fail'
        {
          x()
          function x() {}
          x()
        }
        if (x !== 1) throw 'fail'
      }
      if (x !== 1) throw 'fail'
    `;
    new Function(code)(); // Verify that the code itself is correct
    new Function((await service.transform(code)).code)();
  },

  async nestedFunctionShadowAfter({ service }) {
    const code = `
      try { x; throw 'fail' } catch (e) { if (!(e instanceof ReferenceError)) throw e }
      {
        try { x; throw 'fail' } catch (e) { if (!(e instanceof ReferenceError)) throw e }
        {
          x()
          function x() {}
          x()
        }
        try { x; throw 'fail' } catch (e) { if (!(e instanceof ReferenceError)) throw e }
      }
      try { x; throw 'fail' } catch (e) { if (!(e instanceof ReferenceError)) throw e }
      let x = 1
    `;
    new Function(code)(); // Verify that the code itself is correct
    new Function((await service.transform(code)).code)();
  },

  async sourceMapControlCharacterEscapes({ service }) {
    let chars = ''
    for (let i = 0; i < 32; i++) chars += String.fromCharCode(i);
    const input = `return \`${chars}\``;
    const { code, map } = await service.transform(input, { sourcemap: true, sourcefile: 'afile.code' })
    const fn = new Function(code)
    assert.strictEqual(fn(), chars.replace('\r', '\n'))
    const json = JSON.parse(map)
    assert.strictEqual(json.version, 3)
    assert.strictEqual(json.sourcesContent.length, 1)
    assert.strictEqual(json.sourcesContent[0], input)
  },

  async tsDecorators({ service }) {
    const { code } = await service.transform(`
      let observed = [];
      let on = key => (...args) => {
        observed.push({ key, args });
      };

      @on('class')
      class Foo {
        @on('field') field;
        @on('method') method() { }
        @on('staticField') static staticField;
        @on('staticMethod') static staticMethod() { }
        fn(@on('param') x) { }
        static staticFn(@on('staticParam') x) { }
      }

      // This is what the TypeScript compiler itself generates
      let expected = [
        { key: 'field', args: [Foo.prototype, 'field', undefined] },
        { key: 'method', args: [Foo.prototype, 'method', { value: Foo.prototype.method, writable: true, enumerable: false, configurable: true }] },
        { key: 'param', args: [Foo.prototype, 'fn', 0] },
        { key: 'staticField', args: [Foo, 'staticField', undefined] },
        { key: 'staticMethod', args: [Foo, 'staticMethod', { value: Foo.staticMethod, writable: true, enumerable: false, configurable: true }] },
        { key: 'staticParam', args: [Foo, 'staticFn', 0] },
        { key: 'class', args: [Foo] }
      ];

      return {observed, expected};
    `, { loader: 'ts' });
    const { observed, expected } = new Function(code)();
    assert.deepStrictEqual(observed, expected);
  },

  async pureCallPrint({ service }) {
    const { code: code1 } = await service.transform(`print(123, foo)`, { minifySyntax: true, pure: [] })
    assert.strictEqual(code1, `print(123, foo);\n`)

    const { code: code2 } = await service.transform(`print(123, foo)`, { minifySyntax: true, pure: ['print'] })
    assert.strictEqual(code2, `foo;\n`)
  },

  async pureCallConsoleLog({ service }) {
    const { code: code1 } = await service.transform(`console.log(123, foo)`, { minifySyntax: true, pure: [] })
    assert.strictEqual(code1, `console.log(123, foo);\n`)

    const { code: code2 } = await service.transform(`console.log(123, foo)`, { minifySyntax: true, pure: ['console.log'] })
    assert.strictEqual(code2, `foo;\n`)
  },

  async multipleEngineTargets({ service }) {
    const check = async (target, expected) =>
      assert.strictEqual((await service.transform(`foo(a ?? b)`, { target })).code, expected)
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
  bigIntKey: ({ service }) => futureSyntax(service, '({123n: 0})', 'es2019', 'es2020'),
  bigIntPattern: ({ service }) => futureSyntax(service, 'let {123n: x} = y', 'es2019', 'es2020'),
  nonIdArrayRest: ({ service }) => futureSyntax(service, 'let [...[x]] = y', 'es2015', 'es2016'),
  topLevelAwait: ({ service }) => futureSyntax(service, 'await foo', 'es2020', 'esnext'),
  topLevelForAwait: ({ service }) => futureSyntax(service, 'for await (foo of bar) ;', 'es2020', 'esnext'),

  // Future syntax: async generator functions
  asyncGenFnStmt: ({ service }) => futureSyntax(service, 'async function* foo() {}', 'es2017', 'es2018'),
  asyncGenFnExpr: ({ service }) => futureSyntax(service, '(async function*() {})', 'es2017', 'es2018'),
  asyncGenObjFn: ({ service }) => futureSyntax(service, '({ async* foo() {} })', 'es2017', 'es2018'),
  asyncGenClassStmtFn: ({ service }) => futureSyntax(service, 'class Foo { async* foo() {} }', 'es2017', 'es2018'),
  asyncGenClassExprFn: ({ service }) => futureSyntax(service, '(class { async* foo() {} })', 'es2017', 'es2018'),
}

let syncTests = {
  async buildSync({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, 'export default 123')
    esbuild.buildSync({ entryPoints: [input], bundle: true, outfile: output, format: 'cjs' })
    const result = require(output)
    assert.strictEqual(result.default, 123)
    assert.strictEqual(result.__esModule, true)
  },

  async buildSyncOutputFiles({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, 'module.exports = 123')
    let prettyPath = path.relative(process.cwd(), input).replace(/\\/g, '/')
    let text = `// ${prettyPath}\nmodule.exports = 123;\n`
    let result = esbuild.buildSync({ entryPoints: [input], bundle: true, outfile: output, format: 'cjs', write: false })
    assert.strictEqual(result.outputFiles.length, 1)
    assert.strictEqual(result.outputFiles[0].path, output)
    assert.strictEqual(result.outputFiles[0].text, text)
    assert.deepStrictEqual(result.outputFiles[0].contents, new Uint8Array(Buffer.from(text)))
  },

  async transformSyncJSMap({ esbuild }) {
    const { code, map } = esbuild.transformSync(`1+2`, { sourcemap: true })
    assert.strictEqual(code, `1 + 2;\n`)
    assert.strictEqual(map, `{
  "version": 3,
  "sources": ["<stdin>"],
  "sourcesContent": ["1+2"],
  "mappings": "AAAA,IAAE;",
  "names": []
}
`)
  },

  async transformSyncCSS({ esbuild }) {
    const { code, map } = esbuild.transformSync(`a{b:c}`, { loader: 'css' })
    assert.strictEqual(code, `a {\n  b: c;\n}\n`)
    assert.strictEqual(map, '')
  },

  async transformSyncWithNonString({ esbuild }) {
    try {
      esbuild.transformSync({ toString() { throw new Error('toString() error') } })
      throw new Error('Expected an error to be thrown');
    } catch (e) {
      assert.strictEqual(e.message, 'toString() error')
    }

    var { code } = await esbuild.transformSync(Buffer.from(`1+2`))
    assert.strictEqual(code, `1 + 2;\n`)
  },

  async transformSync100x({ esbuild }) {
    for (let i = 0; i < 100; i++) {
      const { code } = esbuild.transformSync(`console.log(1+${i})`, {})
      assert.strictEqual(code, `console.log(1 + ${i});\n`)
    }
  },

  async buildSyncThrow({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    try {
      const output = path.join(testDir, 'out.js')
      await writeFileAsync(input, '1+')
      esbuild.buildSync({ entryPoints: [input], bundle: true, outfile: output, format: 'cjs', logLevel: 'silent' })
      const result = require(output)
      assert.strictEqual(result.default, 123)
      assert.strictEqual(result.__esModule, true)
      throw new Error('Expected an error to be thrown');
    } catch (error) {
      assert(error instanceof Error, 'Must be an Error object');
      assert.strictEqual(error.message, `Build failed with 1 error:
${path.relative(process.cwd(), input).replace(/\\/g, '/')}:1:2: error: Unexpected end of file`);
      assert.strictEqual(error.errors.length, 1);
      assert.strictEqual(error.warnings.length, 0);
    }
  },

  async buildSyncIncrementalThrow({ esbuild, testDir }) {
    try {
      const input = path.join(testDir, 'in.js')
      const output = path.join(testDir, 'out.js')
      await writeFileAsync(input, '1+')
      esbuild.buildSync({ entryPoints: [input], bundle: true, outfile: output, format: 'cjs', logLevel: 'silent', incremental: true })
      const result = require(output)
      assert.strictEqual(result.default, 123)
      assert.strictEqual(result.__esModule, true)
      throw new Error('Expected an error to be thrown');
    } catch (error) {
      assert(error instanceof Error, 'Must be an Error object');
      assert.strictEqual(error.message, `Cannot use "incremental" with a synchronous build`);
      assert.strictEqual(error.errors, void 0);
      assert.strictEqual(error.warnings, void 0);
    }
  },

  async transformThrow({ service }) {
    try {
      await service.transform(`1+`, {})
      throw new Error('Expected an error to be thrown');
    } catch (error) {
      assert(error instanceof Error, 'Must be an Error object');
      assert.strictEqual(error.message, `Transform failed with 1 error:\n<stdin>:1:2: error: Unexpected end of file`);
      assert.strictEqual(error.errors.length, 1);
      assert.strictEqual(error.warnings.length, 0);
    }
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
  const esbuild = installForTests()

  // Create a fresh test directory
  rimraf.sync(rootTestDir, { disableGlob: true })
  fs.mkdirSync(rootTestDir)

  // Time out these tests after 5 minutes. This exists to help debug test hangs in CI.
  let minutes = 5
  let timeout = setTimeout(() => {
    console.error(`❌ js api tests timed out after ${minutes} minutes, exiting...`)
    process.exit(1)
  }, minutes * 60 * 1000)

  // Start the esbuild service
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
    ...Object.entries(serveTests),
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

    // This randomly fails with EPERM on Windows in CI (GitHub Actions):
    //
    //   Error: EPERM: operation not permitted: unlink 'esbuild\scripts\.js-api-tests\node_modules\esbuild\esbuild.exe'
    //       at Object.unlinkSync (fs.js)
    //       at fixWinEPERMSync (esbuild\scripts\node_modules\rimraf\rimraf.js)
    //       at rimrafSync (esbuild\scripts\node_modules\rimraf\rimraf.js)
    //
    // From searching related issues on GitHub it looks like apparently this is
    // just how Windows works? It's kind of hard to believe something as
    // fundamental as file operations is broken on Windows. It sounds like the
    // file system implementation on Windows has race conditions or something.
    // Anyway, deleting this is not important for the success of the test so
    // just ignore errors here.
    try {
      rimraf.sync(rootTestDir, { disableGlob: true })
    } catch (e) {
    }
  }

  clearTimeout(timeout);
}

main().catch(e => setTimeout(() => { throw e }))
