const { SourceMapConsumer } = require('source-map')
const { buildBinary } = require('./esbuild')
const childProcess = require('child_process')
const mkdirp = require('mkdirp')
const rimraf = require('rimraf')
const path = require('path')
const util = require('util')
const fs = require('fs')

const readFileAsync = util.promisify(fs.readFile)
const writeFileAsync = util.promisify(fs.writeFile)

const testDir = path.join(__dirname, '.verify-source-map')
let tempDirCount = 0

const toSearchBundle = [
  'a0', 'a1', 'a2',
  'b0', 'b1', 'b2',
  'c0', 'c1', 'c2',
]

const toSearchNoBundle = [
  'a0', 'a1', 'a2',
]

const testCaseES6 = {
  'a.js': `
    import {b0} from './b-dir/b'
    function a0() { a1("a0") }
    function a1() { a2("a1") }
    function a2() { b0("a2") }
    a0()
  `,
  'b-dir/b.js': `
    import {c0} from './c-dir/c'
    export function b0() { b1("b0") }
    function b1() { b2("b1") }
    function b2() { c0("b2") }
  `,
  'b-dir/c-dir/c.js': `
    export function c0() { c1("c0") }
    function c1() { c2("c1") }
    function c2() { throw new Error("c2") }
  `,
}

const testCaseCommonJS = {
  'a.js': `
    const {b0} = require('./b-dir/b')
    function a0() { a1("a0") }
    function a1() { a2("a1") }
    function a2() { b0("a2") }
    a0()
  `,
  'b-dir/b.js': `
    const {c0} = require('./c-dir/c')
    exports.b0 = function() { b1("b0") }
    function b1() { b2("b1") }
    function b2() { c0("b2") }
  `,
  'b-dir/c-dir/c.js': `
    exports.c0 = function() { c1("c0") }
    function c1() { c2("c1") }
    function c2() { throw new Error("c2") }
  `,
}

const testCaseTypeScriptRuntime = {
  'a.ts': `
    namespace Foo {
      export var {a, ...b} = foo() // This requires a runtime function to handle
      console.log(a, b)
    }
    function a0() { a1("a0") }
    function a1() { a2("a1") }
    function a2() { throw new Error("a2") }
    a0()
  `,
}

const testCaseStdin = {
  '<stdin>': `#!/usr/bin/env node
    function a0() { a1("a0") }
    function a1() { a2("a1") }
    function a2() { throw new Error("a2") }
    a0()
  `,
}

async function check(kind, testCase, toSearch, flags) {
  let failed = 0

  try {
    const recordCheck = (success, message) => {
      if (!success) {
        failed++
        console.error(`❌ [${kind}] ${message}`)
      }
    }

    const tempDir = path.join(testDir, '' + tempDirCount++)
    mkdirp.sync(tempDir)

    for (const name in testCase) {
      if (name !== '<stdin>') {
        const tempPath = path.join(tempDir, name)
        mkdirp.sync(path.dirname(tempPath))
        await writeFileAsync(tempPath, testCase[name])
      }
    }

    const esbuildPath = buildBinary()
    const files = Object.keys(testCase)
    const args = ['--sourcemap'].concat(flags)
    const isStdin = '<stdin>' in testCase
    let stdout = ''

    await new Promise((resolve, reject) => {
      if (!isStdin) args.unshift(files[0])
      const child = childProcess.spawn(esbuildPath, args, { cwd: tempDir, stdio: 'pipe' })
      if (isStdin) child.stdin.write(testCase['<stdin>'])
      child.stdin.end()
      child.stdout.on('data', chunk => stdout += chunk.toString())
      child.stdout.on('end', resolve)
      child.on('error', reject)
    })

    let outJs
    let outJsMap

    if (isStdin) {
      outJs = stdout
      recordCheck(outJs.includes(`//# sourceMappingURL=data:application/json;base64,`), `.js file contains source map`)
      outJsMap = Buffer.from(outJs.slice(outJs.indexOf('base64,') + 'base64,'.length).trim(), 'base64').toString()
    }

    else {
      outJs = await readFileAsync(path.join(tempDir, 'out.js'), 'utf8')
      recordCheck(outJs.includes(`//# sourceMappingURL=out.js.map\n`), `.js file links to .js.map`)
      outJsMap = await readFileAsync(path.join(tempDir, 'out.js.map'), 'utf8')
    }

    const map = await new SourceMapConsumer(outJsMap)

    for (const id of toSearch) {
      const inSource = isStdin ? '<stdin>' : files.find(x => path.basename(x).startsWith(id[0]))
      const inJs = testCase[inSource]
      const inIndex = inJs.indexOf(`"${id}"`)
      const outIndex = outJs.indexOf(`"${id}"`)

      if (inIndex < 0) throw new Error(`Failed to find "${id}" in input`)
      if (outIndex < 0) throw new Error(`Failed to find "${id}" in output`)

      const inLines = inJs.slice(0, inIndex).split('\n')
      const inLine = inLines.length
      const inColumn = inLines[inLines.length - 1].length

      const outLines = outJs.slice(0, outIndex).split('\n')
      const outLine = outLines.length
      const outColumn = outLines[outLines.length - 1].length

      const { source, line, column } = map.originalPositionFor({ line: outLine, column: outColumn })
      const expected = JSON.stringify({ source: inSource, line: inLine, column: inColumn })
      const observed = JSON.stringify({ source, line, column })
      recordCheck(expected === observed, `expected: ${expected} observed: ${observed}`)
    }

    if (!failed) rimraf.sync(tempDir, { disableGlob: true })
  }

  catch (e) {
    console.error(`❌ [${kind}] ${e && e.message || e}`)
    failed++
  }

  return failed
}

async function main() {
  const promises = []
  for (const minify of [false, true]) {
    const flags = minify ? ['--minify'] : []
    const suffix = minify ? '-min' : ''
    promises.push(
      check('commonjs' + suffix, testCaseCommonJS, toSearchBundle, flags.concat('--outfile=out.js', '--bundle')),
      check('es6' + suffix, testCaseES6, toSearchBundle, flags.concat('--outfile=out.js', '--bundle')),
      check('ts' + suffix, testCaseTypeScriptRuntime, toSearchNoBundle, flags.concat('--outfile=out.js')),
      check('stdin-stdout' + suffix, testCaseStdin, toSearchNoBundle, flags.concat('--sourcefile=<stdin>')),
    )
  }

  const failed = (await Promise.all(promises)).reduce((a, b) => a + b, 0)
  if (failed > 0) {
    console.error(`❌ verify source map failed`)
    process.exit(1)
  } else {
    console.log(`✅ verify source map passed`)
    rimraf.sync(testDir, { disableGlob: true })
  }
}

main().catch(e => setTimeout(() => { throw e }))
