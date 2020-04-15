const { SourceMapConsumer } = require('source-map')
const childProcess = require('child_process')
const path = require('path')
const fs = require('fs')

const toSearch = [
  'a0', 'a1', 'a2',
  'b0', 'b1', 'b2',
  'c0', 'c1', 'c2',
]

const testCaseES6 = {
  'a.js': `
    import {b0} from './b'
    function a0() { a1("a0") }
    function a1() { a2("a1") }
    function a2() { b0("a2") }
    a0()
  `,
  'b.js': `
    import {c0} from './c'
    export function b0() { b1("b0") }
    function b1() { b2("b1") }
    function b2() { c0("b2") }
  `,
  'c.js': `
    export function c0() { c1("c0") }
    function c1() { c2("c1") }
    function c2() { throw new Error("c2") }
  `,
}

const testCaseCommonJS = {
  'a.js': `
    const {b0} = require('./b')
    function a0() { a1("a0") }
    function a1() { a2("a1") }
    function a2() { b0("a2") }
    a0()
  `,
  'b.js': `
    const {c0} = require('./c')
    exports.b0 = function() { b1("b0") }
    function b1() { b2("b1") }
    function b2() { c0("b2") }
  `,
  'c.js': `
    exports.c0 = function() { c1("c0") }
    function c1() { c2("c1") }
    function c2() { throw new Error("c2") }
  `,
}

async function check(kind, testCase) {
  let failed = 0
  const recordCheck = (success, message) => {
    if (!success) {
      failed++
      console.error(`❌ [${kind}] ${message}`)
    }
  }

  const tempDir = path.join(__dirname, '.temp')
  try { fs.mkdirSync(tempDir) } catch (e) { }

  for (const name in testCase) {
    fs.writeFileSync(path.join(tempDir, name), testCase[name])
  }

  const esbuildPath = path.join(__dirname, '..', 'esbuild')
  const args = ['a.js', '--bundle', '--minify', '--sourcemap', '--outfile=out.js']
  childProcess.execFileSync(esbuildPath, args, { cwd: tempDir, stdio: 'pipe' })

  const outJs = fs.readFileSync(path.join(tempDir, 'out.js'), 'utf8')
  const outJsMap = fs.readFileSync(path.join(tempDir, 'out.js.map'), 'utf8')
  const map = await new SourceMapConsumer(outJsMap)

  const isLinked = outJs.includes(`//# sourceMappingURL=out.js.map\n`)
  recordCheck(isLinked, `.js file links to .js.map`)

  for (const id of toSearch) {
    const inSource = id[0] + '.js'
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

  childProcess.execSync(`rm -fr "${tempDir}"`, { cwd: __dirname })
  return failed
}

async function main() {
  childProcess.execSync('make', { cwd: path.dirname(__dirname), stdio: 'pipe' })

  let failed = 0
  failed += await check('commonjs', testCaseCommonJS)
  failed += await check('es6', testCaseES6)
  if (failed > 0) process.exit(1)
}

main().catch(e => setTimeout(() => { throw e }))
