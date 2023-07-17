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

const testCasePartialMappingsPercentEscape = {
  // The "mappings" value is "A,Q,I;A,Q,I;A,Q,I;AAMA,QAAQ,IAAI;" which contains
  // partial mappings without original locations. This used to throw things off.
  'entry.js': `console.log(1);
console.log(2);
console.log(3);
console.log("entry");
//# sourceMappingURL=data:,%7B%22version%22%3A3%2C%22sources%22%3A%5B%22entr` +
    `y.js%22%5D%2C%22sourcesContent%22%3A%5B%22console.log(1)%5Cn%5Cnconsole` +
    `.log(2)%5Cn%5Cnconsole.log(3)%5Cn%5Cnconsole.log(%5C%22entry%5C%22)%5Cn` +
    `%22%5D%2C%22mappings%22%3A%22A%2CQ%2CI%3BA%2CQ%2CI%3BA%2CQ%2CI%3BAAMA%2` +
    `CQAAQ%2CIAAI%3B%22%2C%22names%22%3A%5B%5D%7D
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
  '[object Array]': '../../node_modules/fuse.js/dist/webpack:/src/helpers/is_array.js',
  'Score average:': '../../node_modules/fuse.js/dist/webpack:/src/index.js',
  '0123456789': '../../node_modules/object-assign/index.js',
  'forceUpdate': '../../node_modules/react/cjs/react.production.min.js',
};

const testCaseDynamicImport = {
  'entry.js': `
    const then = (x) => console.log("imported", x);
    console.log([import("./ext/a.js").then(then), import("./ext/ab.js").then(then), import("./ext/abc.js").then(then)]);
    console.log([import("./ext/abc.js").then(then), import("./ext/ab.js").then(then), import("./ext/a.js").then(then)]);
  `,
  'ext/a.js': `
    export default 'a'
  `,
  'ext/ab.js': `
    export default 'ab'
  `,
  'ext/abc.js': `
    export default 'abc'
  `,
}

const toSearchDynamicImport = {
  './ext/a.js': 'entry.js',
  './ext/ab.js': 'entry.js',
  './ext/abc.js': 'entry.js',
};

const toSearchBundleCSS = {
  a0: 'a.css',
  a1: 'a.css',
  a2: 'a.css',
  b0: 'b-dir/b.css',
  b1: 'b-dir/b.css',
  b2: 'b-dir/b.css',
  c0: 'b-dir/c-dir/c.css',
  c1: 'b-dir/c-dir/c.css',
  c2: 'b-dir/c-dir/c.css',
}

const testCaseBundleCSS = {
  'entry.css': `
    @import "a.css";
  `,
  'a.css': `
    @import "b-dir/b.css";
    a:nth-child(0):after { content: "a0"; }
    a:nth-child(1):after { content: "a1"; }
    a:nth-child(2):after { content: "a2"; }
  `,
  'b-dir/b.css': `
    @import "c-dir/c.css";
    b:nth-child(0):after { content: "b0"; }
    b:nth-child(1):after { content: "b1"; }
    b:nth-child(2):after { content: "b2"; }
  `,
  'b-dir/c-dir/c.css': `
    c:nth-child(0):after { content: "c0"; }
    c:nth-child(1):after { content: "c1"; }
    c:nth-child(2):after { content: "c2"; }
  `,
}

const testCaseJSXRuntime = {
  'entry.jsx': `
    import { A0, A1, A2 } from './a.jsx';
    console.log(<A0><A1/><A2/></A0>)
  `,
  'a.jsx': `
    import {jsx} from './b-dir/b'
    import {Fragment} from './b-dir/c-dir/c'
    export function A0() { return <Fragment id="A0"><>a0</></Fragment> }
    export function A1() { return <div {...jsx} data-testid="A1">a1</div> }
    export function A2() { return <A1 id="A2"><a/><b/></A1> }
  `,
  'b-dir/b.js': `
    export const jsx = {id: 'jsx'}
  `,
  'b-dir/c-dir/c.jsx': `
    exports.Fragment = function() { return <></> }
  `,
}

const toSearchJSXRuntime = {
  A0: 'a.jsx',
  A1: 'a.jsx',
  A2: 'a.jsx',
  jsx: 'b-dir/b.js',
}

const testCaseNames = {
  'entry.js': `
    import "./nested1"

    // Test regular name positions
    var /**/foo = /**/foo || 0
    function /**/fn(/**/bar) {}
    class /**/cls {}
    keep(fn, cls) // Make sure these aren't removed

    // Test property mangling name positions
    var { /**/mangle_: bar } = foo
    var { /**/'mangle_': bar } = foo
    foo./**/mangle_ = 1
    foo[/**/'mangle_']
    foo = { /**/mangle_: 0 }
    foo = { /**/'mangle_': 0 }
    foo = class { /**/mangle_ = 0 }
    foo = class { /**/'mangle_' = 0 }
    foo = /**/'mangle_' in bar
  `,
  'nested1.js': `
    import { foo } from './nested2'
    foo(bar)
  `,
  'nested2.jsx': `
    export let /**/foo = /**/bar => /**/bar()
  `
}

const testCaseMissingSourcesContent = {
  'foo.js': `// foo.ts
var foo = { bar: "bar" };
console.log({ foo });
//# sourceMappingURL=maps/foo.js.map
`,
  'maps/foo.js.map': `{
  "version": 3,
  "sources": ["src/foo.ts"],
  "mappings": ";AAGA,IAAM,MAAW,EAAE,KAAK,MAAM;AAC9B,QAAQ,IAAI,EAAE,IAAI,CAAC;",
  "names": []
}
`,
  'maps/src/foo.ts': `interface Foo {
  bar: string
}
const foo: Foo = { bar: 'bar' }
console.log({ foo })
`,
}

const toSearchMissingSourcesContent = {
  bar: 'src/foo.ts',
}

// The "null" should be filled in by the contents of "bar.ts"
const testCaseNullSourcesContent = {
  'entry.js': `import './foo.js'\n`,
  'foo.ts': `import './bar.ts'\nconsole.log("foo")`,
  'bar.ts': `console.log("bar")\n`,
  'foo.js': `(() => {
  // bar.ts
  console.log("bar");

  // foo.ts
  console.log("foo");
})();
//# sourceMappingURL=foo.js.map
`,
  'foo.js.map': `{
  "version": 3,
  "sources": ["bar.ts", "foo.ts"],
  "sourcesContent": [null, "import './bar.ts'\\nconsole.log(\\"foo\\")"],
  "mappings": ";;AAAA,UAAQ,IAAI,KAAK;;;ACCjB,UAAQ,IAAI,KAAK;",
  "names": []
}
`,
}

const toSearchNullSourcesContent = {
  bar: 'bar.ts',
}

async function check(kind, testCase, toSearch, { ext, flags, entryPoints, crlf, followUpFlags = [] }) {
  let failed = 0

  try {
    const recordCheck = (success, message) => {
      if (!success) {
        failed++
        console.error(`❌ [${kind}] ${message}`)
      }
    }

    const tempDir = path.join(testDir, `${kind}-${tempDirCount++}`)
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

    const args = ['--sourcemap', '--log-level=warning'].concat(flags)
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

    let outCode
    let outCodeMap

    if (isStdin) {
      outCode = stdout
      recordCheck(outCode.includes(`# sourceMappingURL=data:application/json;base64,`), `.${ext} file must contain source map`)
      outCodeMap = Buffer.from(outCode.slice(outCode.indexOf('base64,') + 'base64,'.length).trim(), 'base64').toString()
    }

    else {
      outCode = await fs.readFile(path.join(tempDir, `out.${ext}`), 'utf8')
      recordCheck(outCode.includes(`# sourceMappingURL=out.${ext}.map`), `.${ext} file must link to .${ext}.map`)
      outCodeMap = await fs.readFile(path.join(tempDir, `out.${ext}.map`), 'utf8')
    }

    // Check the mapping of various key locations back to the original source
    const checkMap = (out, map) => {
      for (const id in toSearch) {
        const outIndex = out.indexOf(`"${id}"`)
        if (outIndex < 0) throw new Error(`Failed to find "${id}" in output`)
        const outLines = out.slice(0, outIndex).split('\n')
        const outLine = outLines.length
        const outLastLine = outLines[outLines.length - 1]
        let outColumn = outLastLine.length
        const { source, line, column } = map.originalPositionFor({ line: outLine, column: outColumn })

        const inSource = isStdin ? '<stdin>' : toSearch[id];
        recordCheck(source === inSource, `expected source: ${inSource}, observed source: ${source}`)

        const inCode = map.sourceContentFor(source)
        if (inCode === null) throw new Error(`Got null for source content for "${source}"`)
        let inIndex = inCode.indexOf(`"${id}"`)
        if (inIndex < 0) inIndex = inCode.indexOf(`'${id}'`)
        if (inIndex < 0) throw new Error(`Failed to find "${id}" in input`)
        const inLines = inCode.slice(0, inIndex).split('\n')
        const inLine = inLines.length
        const inLastLine = inLines[inLines.length - 1]
        let inColumn = inLastLine.length

        const expected = JSON.stringify({ source, line: inLine, column: inColumn })
        const observed = JSON.stringify({ source, line, column })
        recordCheck(expected === observed, `expected original position: ${expected}, observed original position: ${observed}`)

        // Also check the reverse mapping
        const positions = map.allGeneratedPositionsFor({ source, line: inLine, column: inColumn })
        recordCheck(positions.length > 0, `expected generated positions: 1, observed generated positions: ${positions.length}`)
        let found = false
        for (const { line, column } of positions) {
          if (line === outLine && column === outColumn) {
            found = true
            break
          }
        }
        const expectedPosition = JSON.stringify({ line: outLine, column: outColumn })
        const observedPositions = JSON.stringify(positions)
        recordCheck(found, `expected generated position: ${expectedPosition}, observed generated positions: ${observedPositions}`)
      }
    }

    const sources = JSON.parse(outCodeMap).sources
    for (let source of sources) {
      if (sources.filter(s => s === source).length > 1) {
        throw new Error(`Duplicate source ${JSON.stringify(source)} found in source map`)
      }
    }

    const outMap = await new SourceMapConsumer(outCodeMap)
    checkMap(outCode, outMap)

    // Check that every generated location has an associated original position.
    // This only works when not bundling because bundling includes runtime code.
    if (flags.indexOf('--bundle') < 0) {
      // The last line doesn't have a source map entry, but that should be ok.
      const outLines = outCode.trimRight().split('\n');

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
      const fileToTest = isStdin ? `stdout.${ext}` : `out.${ext}`
      const nestedEntry = path.join(tempDir, `nested-entry.${ext}`)
      if (isStdin) await fs.writeFile(path.join(tempDir, fileToTest), outCode)
      await fs.writeFile(path.join(tempDir, `extra.${ext}`), `console.log('extra')`)
      const importKeyword = ext === 'css' ? '@import' : 'import'
      await fs.writeFile(nestedEntry,
        order === 1 ? `${importKeyword} './${fileToTest}'; ${importKeyword} './extra.${ext}'` :
          order === 2 ? `${importKeyword} './extra.${ext}'; ${importKeyword} './${fileToTest}'` :
            `${importKeyword} './${fileToTest}'`)
      await execFileAsync(esbuildPath, [
        nestedEntry,
        '--bundle',
        '--outfile=' + path.join(tempDir, `out2.${ext}`),
        '--sourcemap',
      ].concat(followUpFlags), { cwd: testDir })

      const out2Code = await fs.readFile(path.join(tempDir, `out2.${ext}`), 'utf8')
      recordCheck(out2Code.includes(`# sourceMappingURL=out2.${ext}.map`), `.${ext} file must link to .${ext}.map`)
      const out2CodeMap = await fs.readFile(path.join(tempDir, `out2.${ext}.map`), 'utf8')

      const out2Map = await new SourceMapConsumer(out2CodeMap)
      checkMap(out2Code, out2Map)
    }

    if (!failed) removeRecursiveSync(tempDir)
  }

  catch (e) {
    console.error(`❌ [${kind}] ${e && e.message || e}`)
    failed++
  }

  return failed
}

async function checkNames(kind, testCase, { ext, flags, entryPoints, crlf }) {
  let failed = 0

  try {
    const recordCheck = (success, message) => {
      if (!success) {
        failed++
        console.error(`❌ [${kind}] ${message}`)
      }
    }

    const tempDir = path.join(testDir, `${kind}-${tempDirCount++}`)
    await fs.mkdir(tempDir, { recursive: true })

    for (const name in testCase) {
      const tempPath = path.join(tempDir, name)
      let code = testCase[name]
      await fs.mkdir(path.dirname(tempPath), { recursive: true })
      if (crlf) code = code.replace(/\n/g, '\r\n')
      await fs.writeFile(tempPath, code)
    }

    const args = ['--sourcemap', '--log-level=warning'].concat(flags)
    let stdout = ''

    await new Promise((resolve, reject) => {
      args.unshift(...entryPoints)
      const child = childProcess.spawn(esbuildPath, args, { cwd: tempDir, stdio: ['pipe', 'pipe', 'inherit'] })
      child.stdin.end()
      child.stdout.on('data', chunk => stdout += chunk.toString())
      child.stdout.on('end', resolve)
      child.on('error', reject)
    })

    const outCode = await fs.readFile(path.join(tempDir, `out.${ext}`), 'utf8')
    recordCheck(outCode.includes(`# sourceMappingURL=out.${ext}.map`), `.${ext} file must link to .${ext}.map`)
    const outCodeMap = await fs.readFile(path.join(tempDir, `out.${ext}.map`), 'utf8')

    // Check the mapping of various key locations back to the original source
    const checkMap = (out, map) => {
      const undoQuotes = x => `'"`.includes(x[0]) ? (0, eval)(x) : x.startsWith('(') ? x.slice(1, -1) : x
      const generatedLines = out.split(/\r\n|\r|\n/g)

      for (let i = 0; i < map.sources.length; i++) {
        const source = map.sources[i]
        const content = map.sourcesContent[i];
        let index = 0

        // The names for us to check are prefixed by "/**/" right before to mark them
        const parts = content.split(/(\/\*\*\/(?:\w+|'\w+'|"\w+"))/g)

        for (let j = 1; j < parts.length; j += 2) {
          const expectedName = undoQuotes(parts[j].slice(4))
          index += parts[j - 1].length

          const prefixLines = content.slice(0, index + 4).split(/\r\n|\r|\n/g)
          const line = prefixLines.length
          const column = prefixLines[prefixLines.length - 1].length
          index += parts[j].length

          // There may be multiple mappings if the expression is spread across
          // multiple lines. Check each one to see if any pass the checks.
          const allGenerated = map.allGeneratedPositionsFor({ source, line, column })
          for (let i = 0; i < allGenerated.length; i++) {
            const canSkip = i + 1 < allGenerated.length // Don't skip the last one
            const generated = allGenerated[i]
            const original = map.originalPositionFor(generated)
            if (canSkip && (original.source !== source || original.line !== line || original.column !== column)) continue
            recordCheck(original.source === source && original.line === line && original.column === column,
              `\n` +
              `\n  original position:               ${JSON.stringify({ source, line, column })}` +
              `\n  maps to generated position:      ${JSON.stringify(generated)}` +
              `\n  which maps to original position: ${JSON.stringify(original)}` +
              `\n`)

            if (original.source === source && original.line === line && original.column === column) {
              const generatedContentAfter = generatedLines[generated.line - 1].slice(generated.column)
              const matchAfter = /^(?:\w+|'\w+'|"\w+"|\(\w+\))/.exec(generatedContentAfter)
              if (canSkip && matchAfter === null) continue
              recordCheck(matchAfter !== null, `expected the identifier ${JSON.stringify(expectedName)} starting on line ${generated.line} here: ${generatedContentAfter.slice(0, 100)}`)

              if (matchAfter !== null) {
                const observedName = undoQuotes(matchAfter[0])
                if (canSkip && expectedName !== (original.name || observedName)) continue
                recordCheck(expectedName === (original.name || observedName),
                  `\n` +
                  `\n  generated position: ${JSON.stringify(generated)}` +
                  `\n  original position:  ${JSON.stringify(original)}` +
                  `\n` +
                  `\n  original name:  ${JSON.stringify(expectedName)}` +
                  `\n  generated name: ${JSON.stringify(observedName)}` +
                  `\n  mapping name:   ${JSON.stringify(original.name)}` +
                  `\n`)
              }
            }

            break
          }
        }
      }
    }

    const outMap = await new SourceMapConsumer(outCodeMap)
    checkMap(outCode, outMap)

    // Bundle again to test nested source map chaining
    for (let order of [0, 1, 2]) {
      const fileToTest = `out.${ext}`
      const nestedEntry = path.join(tempDir, `nested-entry.${ext}`)
      await fs.writeFile(path.join(tempDir, `extra.${ext}`), `console.log('extra')`)
      await fs.writeFile(nestedEntry,
        order === 1 ? `import './${fileToTest}'; import './extra.${ext}'` :
          order === 2 ? `import './extra.${ext}'; import './${fileToTest}'` :
            `import './${fileToTest}'`)
      await execFileAsync(esbuildPath, [
        nestedEntry,
        '--bundle',
        '--outfile=' + path.join(tempDir, `out2.${ext}`),
        '--sourcemap',
      ], { cwd: testDir })

      const out2Code = await fs.readFile(path.join(tempDir, `out2.${ext}`), 'utf8')
      recordCheck(out2Code.includes(`# sourceMappingURL=out2.${ext}.map`), `.${ext} file must link to .${ext}.map`)
      const out2CodeMap = await fs.readFile(path.join(tempDir, `out2.${ext}.map`), 'utf8')

      const out2Map = await new SourceMapConsumer(out2CodeMap)
      checkMap(out2Code, out2Map)
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
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle'),
          entryPoints: ['a.js'],
          crlf,
        }),
        check('es6' + suffix, testCaseES6, toSearchBundle, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle'),
          entryPoints: ['a.js'],
          crlf,
        }),
        check('discontiguous' + suffix, testCaseDiscontiguous, toSearchBundle, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle'),
          entryPoints: ['a.js'],
          crlf,
        }),
        check('ts' + suffix, testCaseTypeScriptRuntime, toSearchNoBundleTS, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js'),
          entryPoints: ['a.ts'],
          crlf,
        }),
        check('stdin-stdout' + suffix, testCaseStdin, toSearchNoBundle, {
          ext: 'js',
          flags: flags.concat('--sourcefile=<stdin>'),
          entryPoints: [],
          crlf,
        }),
        check('empty' + suffix, testCaseEmptyFile, toSearchEmptyFile, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle'),
          entryPoints: ['entry.js'],
          crlf,
        }),
        check('non-js' + suffix, testCaseNonJavaScriptFile, toSearchNonJavaScriptFile, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle'),
          entryPoints: ['entry.js'],
          crlf,
        }),
        check('splitting' + suffix, testCaseCodeSplitting, toSearchCodeSplitting, {
          ext: 'js',
          flags: flags.concat('--outdir=.', '--bundle', '--splitting', '--format=esm'),
          entryPoints: ['out.ts', 'other.ts'],
          crlf,
        }),
        check('unicode' + suffix, testCaseUnicode, toSearchUnicode, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle', '--charset=utf8'),
          entryPoints: ['entry.js'],
          crlf,
        }),
        check('unicode-globalName' + suffix, testCaseUnicode, toSearchUnicode, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle', '--global-name=πππ', '--charset=utf8'),
          entryPoints: ['entry.js'],
          crlf,
        }),
        check('dummy' + suffix, testCasePartialMappings, toSearchPartialMappings, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle'),
          entryPoints: ['entry.js'],
          crlf,
        }),
        check('dummy' + suffix, testCasePartialMappingsPercentEscape, toSearchPartialMappings, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle'),
          entryPoints: ['entry.js'],
          crlf,
        }),
        check('banner-footer' + suffix, testCaseES6, toSearchBundle, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle', '--banner:js="/* LICENSE abc */"', '--footer:js="/* end of file banner */"'),
          entryPoints: ['a.js'],
          crlf,
        }),
        check('complex' + suffix, testCaseComplex, toSearchComplex, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle', '--define:process.env.NODE_ENV="production"'),
          entryPoints: ['entry.js'],
          crlf,
        }),
        check('dynamic-import' + suffix, testCaseDynamicImport, toSearchDynamicImport, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle', '--external:./ext/*', '--format=esm'),
          entryPoints: ['entry.js'],
          crlf,
          followUpFlags: ['--external:./ext/*', '--format=esm'],
        }),
        check('dynamic-require' + suffix, testCaseDynamicImport, toSearchDynamicImport, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle', '--external:./ext/*', '--format=cjs'),
          entryPoints: ['entry.js'],
          crlf,
          followUpFlags: ['--external:./ext/*', '--format=cjs'],
        }),
        check('bundle-css' + suffix, testCaseBundleCSS, toSearchBundleCSS, {
          ext: 'css',
          flags: flags.concat('--outfile=out.css', '--bundle'),
          entryPoints: ['entry.css'],
          crlf,
        }),
        check('jsx-runtime' + suffix, testCaseJSXRuntime, toSearchJSXRuntime, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle', '--jsx=automatic', '--external:react/jsx-runtime'),
          entryPoints: ['entry.jsx'],
          crlf,
        }),
        check('jsx-dev-runtime' + suffix, testCaseJSXRuntime, toSearchJSXRuntime, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle', '--jsx=automatic', '--jsx-dev', '--external:react/jsx-dev-runtime'),
          entryPoints: ['entry.jsx'],
          crlf,
        }),

        // Checks for the "names" field
        checkNames('names' + suffix, testCaseNames, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle'),
          entryPoints: ['entry.js'],
          crlf,
        }),
        checkNames('names-mangle' + suffix, testCaseNames, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle', '--mangle-props=^mangle_$'),
          entryPoints: ['entry.js'],
          crlf,
        }),
        checkNames('names-mangle-quoted' + suffix, testCaseNames, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle', '--mangle-props=^mangle_$', '--mangle-quoted'),
          entryPoints: ['entry.js'],
          crlf,
        }),

        // Checks for loading missing "sourcesContent" in nested source maps
        check('missing-sources-content' + suffix, testCaseMissingSourcesContent, toSearchMissingSourcesContent, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle'),
          entryPoints: ['foo.js'],
          crlf,
        }),

        // Checks for null entries in "sourcesContent" in nested source maps
        check('null-sources-content' + suffix, testCaseNullSourcesContent, toSearchNullSourcesContent, {
          ext: 'js',
          flags: flags.concat('--outfile=out.js', '--bundle'),
          entryPoints: ['foo.js'],
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
