const { SourceMapConsumer } = require('source-map')
const { buildBinary, removeRecursiveSync } = require('./esbuild')
const childProcess = require('child_process')
const path = require('path')
const util = require('util')
const fs = require('fs').promises

const execFileAsync = util.promisify(childProcess.execFile)

const esbuildPath = buildBinary()
const testDir = path.join(__dirname, '.verify-source-map')
let tempDirCount = 0

const toSearchBundle = {
  a0: 'a.js',
  a1: 'a.js',
  a2: 'a.js',
  b0: 'b-dir/b.js',
  b1: 'b-dir/b.js',
  b2: 'b-dir/b.js',
  c0: 'b-dir/c-dir/c.js',
  c1: 'b-dir/c-dir/c.js',
  c2: 'b-dir/c-dir/c.js',
}

const toSearchNoBundle = {
  a0: 'a.js',
  a1: 'a.js',
  a2: 'a.js',
}

const toSearchNoBundleTS = {
  a0: 'a.ts',
  a1: 'a.ts',
  a2: 'a.ts',
}

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

const testCaseDiscontiguous = {
  'a.js': `
    import {b0} from './b-dir/b.js'
    import {c0} from './b-dir/c-dir/c.js'
    function a0() { a1("a0") }
    function a1() { a2("a1") }
    function a2() { b0("a2") }
    a0(b0, c0)
  `,
  'b-dir/b.js': `
    exports.b0 = function() { b1("b0") }
    function b1() { b2("b1") }
    function b2() { c0("b2") }
  `,
  'b-dir/c-dir/c.js': `
    export function c0() { c1("c0") }
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

const testCaseEmptyFile = {
  'entry.js': `
    import './before'
    import {fn} from './re-export'
    import './after'
    fn()
  `,
  're-export.js': `
    // This file will be empty in the generated code, which was causing
    // an off-by-one error with the source index in the source map
    export {default as fn} from './test'
  `,
  'test.js': `
    export default function() {
      console.log("test")
    }
  `,
  'before.js': `
    console.log("before")
  `,
  'after.js': `
    console.log("after")
  `,
}

const toSearchEmptyFile = {
  before: 'before.js',
  test: 'test.js',
  after: 'after.js',
}

const testCaseNonJavaScriptFile = {
  'entry.js': `
    import './before'
    import text from './file.txt'
    import './after'
    console.log(text)
  `,
  'file.txt': `
    This is some text.
  `,
  'before.js': `
    console.log("before")
  `,
  'after.js': `
    console.log("after")
  `,
}

const toSearchNonJavaScriptFile = {
  before: 'before.js',
  after: 'after.js',
}

const testCaseCodeSplitting = {
  'out.ts': `
    import value from './shared'
    console.log("out", value)
  `,
  'other.ts': `
    import value from './shared'
    console.log("other", value)
  `,
  'shared.ts': `
    export default 123
  `,
}

const toSearchCodeSplitting = {
  out: 'out.ts',
}

const testCaseUnicode = {
  'entry.js': `
    import './a'
    import './b'
  `,
  'a.js': `
    console.log('🍕🍕🍕', "a")
  `,
  'b.js': `
    console.log({𐀀: "b"})
  `,
}

const toSearchUnicode = {
  a: 'a.js',
  b: 'b.js',
}

const testCasePartialMappings = {
  // The "mappings" value is "A,Q,I;A,Q,I;A,Q,I;AAMA,QAAQ,IAAI;" which contains
  // partial mappings without original locations. This used to throw things off.
  'entry.js': `console.log(1);
console.log(2);
console.log(3);
console.log("entry");
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKIC` +
    `Aic291cmNlcyI6IFsiZW50cnkuanMiXSwKICAic291cmNlc0NvbnRlbnQiOiBbImNvbnNvb` +
    `GUubG9nKDEpXG5cbmNvbnNvbGUubG9nKDIpXG5cbmNvbnNvbGUubG9nKDMpXG5cbmNvbnNv` +
    `bGUubG9nKFwiZW50cnlcIilcbiJdLAogICJtYXBwaW5ncyI6ICJBLFEsSTtBLFEsSTtBLFE` +
    `sSTtBQU1BLFFBQVEsSUFBSTsiLAogICJuYW1lcyI6IFtdCn0=
`,
}

const toSearchPartialMappings = {
  entry: 'entry.js',
}

const testCaseComplex = {
  // "fuse.js" is included because it has a nested source map of some complexity.
  // "react" is included after that because it's a big blob of code and helps
  // make sure stuff after a nested source map works ok.
  'entry.js': `
    import Fuse from 'fuse.js'
    import * as React from 'react'
    console.log(Fuse, React)
  `,
}

const toSearchComplex = {
  'Score average:': '../../node_modules/fuse.js/dist/webpack:/src/index.js',
  '0123456789': '../../node_modules/object-assign/index.js',
  'forceUpdate': '../../node_modules/react/cjs/react.production.min.js',
};

async function check(kind, testCase, toSearch, { flags, entryPoints, crlf }) {
  let failed = 0

  try {
    const recordCheck = (success, message) => {
      if (!success) {
        failed++
        console.error(`❌ [${kind}] ${message}`)
      }
    }

    const tempDir = path.join(testDir, '' + tempDirCount++)
    await fs.mkdir(tempDir, { recursive: true })

    for (const name in testCase) {
      if (name !== '<stdin>') {
        const tempPath = path.join(tempDir, name)
        let code = testCase[name]
        await fs.mkdir(path.dirname(tempPath), { recursive: true })
        if (crlf) code = code.replace(/\n/g, '\r\n')
        await fs.writeFile(tempPath, code)
      }
    }

    const files = Object.keys(testCase)
    const args = ['--sourcemap'].concat(flags)
    const isStdin = '<stdin>' in testCase
    let stdout = ''

    await new Promise((resolve, reject) => {
      args.unshift(...entryPoints)
      const child = childProcess.spawn(esbuildPath, args, { cwd: tempDir, stdio: ['pipe', 'pipe', 'inherit'] })
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
      outJs = await fs.readFile(path.join(tempDir, 'out.js'), 'utf8')
      recordCheck(outJs.includes(`//# sourceMappingURL=out.js.map\n`), `.js file links to .js.map`)
      outJsMap = await fs.readFile(path.join(tempDir, 'out.js.map'), 'utf8')
    }

    // Check the mapping of various key locations back to the original source
    const checkMap = (out, map) => {
      for (const id in toSearch) {
        const outIndex = out.indexOf(`"${id}"`)
        if (outIndex < 0) throw new Error(`Failed to find "${id}" in output`)
        const outLines = out.slice(0, outIndex).split('\n')
        const outLine = outLines.length
        const outColumn = outLines[outLines.length - 1].length
        const { source, line, column } = map.originalPositionFor({ line: outLine, column: outColumn })

        const inSource = isStdin ? '<stdin>' : toSearch[id];
        recordCheck(source === inSource, `expected: ${inSource} observed: ${source}`)

        const inJs = map.sourceContentFor(source)
        let inIndex = inJs.indexOf(`"${id}"`)
        if (inIndex < 0) inIndex = inJs.indexOf(`'${id}'`)
        if (inIndex < 0) throw new Error(`Failed to find "${id}" in input`)
        const inLines = inJs.slice(0, inIndex).split('\n')
        const inLine = inLines.length
        const inColumn = inLines[inLines.length - 1].length

        const expected = JSON.stringify({ source, line: inLine, column: inColumn })
        const observed = JSON.stringify({ source, line, column })
        recordCheck(expected === observed, `expected: ${expected} observed: ${observed}`)
      }
    }

    const sources = JSON.parse(outJsMap).sources
    for (let source of sources) {
      if (sources.filter(s => s === source).length > 1) {
        throw new Error(`Duplicate source ${JSON.stringify(source)} found in source map`)
      }
    }

    const outMap = await new SourceMapConsumer(outJsMap)
    checkMap(outJs, outMap)

    // Check that every generated location has an associated original position.
    // This only works when not bundling because bundling includes runtime code.
    if (flags.indexOf('--bundle') < 0) {
      // The last line doesn't have a source map entry, but that should be ok.
      const outLines = outJs.trimRight().split('\n');

      for (let outLine = 0; outLine < outLines.length; outLine++) {
        if (outLines[outLine].startsWith('#!') || outLines[outLine].startsWith('//')) {
          // Ignore the hashbang line and the source map comment itself
          continue;
        }

        for (let outColumn = 0; outColumn <= outLines[outLine].length; outColumn++) {
          const { line, column } = outMap.originalPositionFor({ line: outLine + 1, column: outColumn })
          recordCheck(line !== null && column !== null, `missing location for line ${outLine} and column ${outColumn}`)
        }
      }
    }

    // Bundle again to test nested source map chaining
    for (let order of [0, 1, 2]) {
      const fileToTest = isStdin ? 'stdout.js' : 'out.js'
      const nestedEntry = path.join(tempDir, 'nested-entry.js')
      if (isStdin) await fs.writeFile(path.join(tempDir, fileToTest), outJs)
      await fs.writeFile(path.join(tempDir, 'extra.js'), `console.log('extra')`)
      await fs.writeFile(nestedEntry,
        order === 1 ? `import './${fileToTest}'; import './extra.js'` :
          order === 2 ? `import './extra.js'; import './${fileToTest}'` :
            `import './${fileToTest}'`)
      await execFileAsync(esbuildPath, [nestedEntry, '--bundle', '--outfile=' + path.join(tempDir, 'out2.js'), '--sourcemap'], { cwd: testDir })

      const out2Js = await fs.readFile(path.join(tempDir, 'out2.js'), 'utf8')
      recordCheck(out2Js.includes(`//# sourceMappingURL=out2.js.map\n`), `.js file links to .js.map`)
      const out2JsMap = await fs.readFile(path.join(tempDir, 'out2.js.map'), 'utf8')

      const out2Map = await new SourceMapConsumer(out2JsMap)
      checkMap(out2Js, out2Map)
    }

    if (!failed) removeRecursiveSync(tempDir)
  }

  catch (e) {
    console.error(`❌ [${kind}] ${e && e.message || e}`)
    failed++
  }

  return failed
}

async function main() {
  const promises = []
  for (const crlf of [false, true]) {
    for (const minify of [false, true]) {
      const flags = minify ? ['--minify'] : []
      const suffix = (crlf ? '-crlf' : '') + (minify ? '-min' : '')
      promises.push(
        check('commonjs' + suffix, testCaseCommonJS, toSearchBundle, {
          flags: flags.concat('--outfile=out.js', '--bundle'),
          entryPoints: ['a.js'],
          crlf,
        }),
        check('es6' + suffix, testCaseES6, toSearchBundle, {
          flags: flags.concat('--outfile=out.js', '--bundle'),
          entryPoints: ['a.js'],
          crlf,
        }),
        check('discontiguous' + suffix, testCaseDiscontiguous, toSearchBundle, {
          flags: flags.concat('--outfile=out.js', '--bundle'),
          entryPoints: ['a.js'],
          crlf,
        }),
        check('ts' + suffix, testCaseTypeScriptRuntime, toSearchNoBundleTS, {
          flags: flags.concat('--outfile=out.js'),
          entryPoints: ['a.ts'],
          crlf,
        }),
        check('stdin-stdout' + suffix, testCaseStdin, toSearchNoBundle, {
          flags: flags.concat('--sourcefile=<stdin>'),
          entryPoints: [],
          crlf,
        }),
        check('empty' + suffix, testCaseEmptyFile, toSearchEmptyFile, {
          flags: flags.concat('--outfile=out.js', '--bundle'),
          entryPoints: ['entry.js'],
          crlf,
        }),
        check('non-js' + suffix, testCaseNonJavaScriptFile, toSearchNonJavaScriptFile, {
          flags: flags.concat('--outfile=out.js', '--bundle'),
          entryPoints: ['entry.js'],
          crlf,
        }),
        check('splitting' + suffix, testCaseCodeSplitting, toSearchCodeSplitting, {
          flags: flags.concat('--outdir=.', '--bundle', '--splitting', '--format=esm'),
          entryPoints: ['out.ts', 'other.ts'],
          crlf,
        }),
        check('unicode' + suffix, testCaseUnicode, toSearchUnicode, {
          flags: flags.concat('--outfile=out.js', '--bundle'),
          entryPoints: ['entry.js'],
          crlf,
        }),
        check('dummy' + suffix, testCasePartialMappings, toSearchPartialMappings, {
          flags: flags.concat('--outfile=out.js', '--bundle'),
          entryPoints: ['entry.js'],
          crlf,
        }),
        check('banner-footer' + suffix, testCaseES6, toSearchBundle, {
          flags: flags.concat('--outfile=out.js', '--bundle', '--banner="/* LICENSE abc */"', '--footer="/* end of file banner */"'),
          entryPoints: ['a.js'],
          crlf,
        }),
        check('complex' + suffix, testCaseComplex, toSearchComplex, {
          flags: flags.concat('--outfile=out.js', '--bundle', '--define:process.env.NODE_ENV="production"'),
          entryPoints: ['entry.js'],
          crlf,
        }),
      )
    }
  }

  const failed = (await Promise.all(promises)).reduce((a, b) => a + b, 0)
  if (failed > 0) {
    console.error(`❌ verify source map failed`)
    process.exit(1)
  } else {
    console.log(`✅ verify source map passed`)
    removeRecursiveSync(testDir)
  }
}

main().catch(e => setTimeout(() => { throw e }))
