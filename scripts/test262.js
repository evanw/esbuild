const vm = require('vm')
const fs = require('fs')
const path = require('path')
const jsYaml = require('js-yaml')
const isatty = require('tty').isatty(process.stdout.fd)
const { installForTests } = require('./esbuild')

const testDir = path.join(__dirname, '..', 'demo', 'test262', 'test')
const harnessDir = path.join(__dirname, '..', 'demo', 'test262', 'harness')

const progressBarLength = 64
const eraseProgressBar = () => {
  previousProgressBar = null
  let text = `\r${' '.repeat(progressBarLength)}\r`
  if (printNewlineWhenErasing) {
    printNewlineWhenErasing = false
    text += '\n'
  }
  return text
}
let previousProgressBar = null
let printNewlineWhenErasing = false

const resetColor = isatty ? `\x1b[0m` : ''
const boldColor = isatty ? `\x1b[1m` : ''
const dimColor = isatty ? `\x1b[37m` : ''
const underlineColor = isatty ? `\x1b[4m` : ''

const redColor = isatty ? `\x1b[31m` : ''
const greenColor = isatty ? `\x1b[32m` : ''
const blueColor = isatty ? `\x1b[34m` : ''

const cyanColor = isatty ? `\x1b[36m` : ''
const magentaColor = isatty ? `\x1b[35m` : ''
const yellowColor = isatty ? `\x1b[33m` : ''

const redBgRedColor = isatty ? `\x1b[41;31m` : ''
const redBgWhiteColor = isatty ? `\x1b[41;97m` : ''
const greenBgGreenColor = isatty ? `\x1b[42;32m` : ''
const greenBgWhiteColor = isatty ? `\x1b[42;97m` : ''
const blueBgBlueColor = isatty ? `\x1b[44;34m` : ''
const blueBgWhiteColor = isatty ? `\x1b[44;97m` : ''

const cyanBgCyanColor = isatty ? `\x1b[46;36m` : ''
const cyanBgBlackColor = isatty ? `\x1b[46;30m` : ''
const magentaBgMagentaColor = isatty ? `\x1b[45;35m` : ''
const magentaBgBlackColor = isatty ? `\x1b[45;30m` : ''
const yellowBgYellowColor = isatty ? `\x1b[43;33m` : ''
const yellowBgBlackColor = isatty ? `\x1b[43;30m` : ''

const whiteBgWhiteColor = isatty ? `\x1b[107;97m` : ''
const whiteBgBlackColor = isatty ? `\x1b[107;30m` : ''

const skipTheseFeatures = new Set([
  'decorators',
  'regexp-v-flag',
  'regexp-match-indices',
  'regexp-named-groups',
  'regexp-unicode-property-escapes',
])

const skipTheseTests = new Set([
  // Skip these tests because esbuild deliberately always supports ESM syntax
  // in all files. Also esbuild doesn't support the script goal at all.
  'language/expressions/import.meta/syntax/goal-script.js',
  'language/global-code/export.js',
  'language/global-code/import.js',

  // Skip these tests because we deliberately support top-level return (input
  // files are treated as CommonJS and/or ESM but never as global code, and
  // top-level return is allowed in CommonJS)
  'language/statements/return/S12.9_A1_T1.js',
  'language/statements/return/S12.9_A1_T10.js',
  'language/statements/return/S12.9_A1_T2.js',
  'language/statements/return/S12.9_A1_T3.js',
  'language/statements/return/S12.9_A1_T4.js',
  'language/statements/return/S12.9_A1_T5.js',
  'language/statements/return/S12.9_A1_T6.js',
  'language/statements/return/S12.9_A1_T7.js',
  'language/statements/return/S12.9_A1_T8.js',
  'language/statements/return/S12.9_A1_T9.js',
  'language/global-code/return.js',

  // Skip these tests because we deliberately support parsing top-level await
  // in all files. Files containing top-level await are always interpreted as
  // ESM, never as CommonJS.
  'language/expressions/assignmenttargettype/simple-basic-identifierreference-await.js',
  'language/expressions/await/await-BindingIdentifier-in-global.js',
  'language/expressions/await/await-in-global.js',
  'language/expressions/await/await-in-nested-function.js',
  'language/expressions/await/await-in-nested-generator.js',
  'language/expressions/class/class-name-ident-await-escaped.js',
  'language/expressions/class/class-name-ident-await.js',
  'language/expressions/class/static-init-await-reference.js',
  'language/expressions/dynamic-import/2nd-param-await-ident.js',
  'language/expressions/dynamic-import/assignment-expression/await-identifier.js',
  'language/expressions/function/static-init-await-reference.js',
  'language/expressions/generators/static-init-await-reference.js',
  'language/expressions/in/private-field-rhs-await-absent.js',
  'language/expressions/object/identifier-shorthand-await-strict-mode.js',
  'language/expressions/object/method-definition/static-init-await-reference-accessor.js',
  'language/expressions/object/method-definition/static-init-await-reference-generator.js',
  'language/expressions/object/method-definition/static-init-await-reference-normal.js',
  'language/module-code/top-level-await/new-await-script-code.js',
  'language/reserved-words/await-script.js',
  'language/statements/class/class-name-ident-await-escaped.js',
  'language/statements/class/class-name-ident-await.js',
  'language/statements/labeled/value-await-non-module-escaped.js',
  'language/statements/labeled/value-await-non-module.js',

  // Skip these tests because we don't currently validate the contents of
  // regular expressions. We could do this but it's not necessary to parse
  // JavaScript successfully since we parse enough of it to be able to
  // determine where the regular expression ends (just "\" and "[]" pairs).
  'language/literals/regexp/early-err-pattern.js',
  'language/literals/regexp/invalid-braced-quantifier-exact.js',
  'language/literals/regexp/invalid-braced-quantifier-lower.js',
  'language/literals/regexp/invalid-braced-quantifier-range.js',
  'language/literals/regexp/invalid-optional-lookbehind.js',
  'language/literals/regexp/invalid-optional-negative-lookbehind.js',
  'language/literals/regexp/invalid-range-lookbehind.js',
  'language/literals/regexp/invalid-range-negative-lookbehind.js',
  'language/literals/regexp/u-invalid-class-escape.js',
  'language/literals/regexp/u-invalid-extended-pattern-char.js',
  'language/literals/regexp/u-invalid-identity-escape.js',
  'language/literals/regexp/u-invalid-legacy-octal-escape.js',
  'language/literals/regexp/u-invalid-non-empty-class-ranges-no-dash-a.js',
  'language/literals/regexp/u-invalid-non-empty-class-ranges-no-dash-ab.js',
  'language/literals/regexp/u-invalid-non-empty-class-ranges-no-dash-b.js',
  'language/literals/regexp/u-invalid-non-empty-class-ranges.js',
  'language/literals/regexp/u-invalid-oob-decimal-escape.js',
  'language/literals/regexp/u-invalid-optional-lookahead.js',
  'language/literals/regexp/u-invalid-optional-lookbehind.js',
  'language/literals/regexp/u-invalid-optional-negative-lookahead.js',
  'language/literals/regexp/u-invalid-optional-negative-lookbehind.js',
  'language/literals/regexp/u-invalid-range-lookahead.js',
  'language/literals/regexp/u-invalid-range-lookbehind.js',
  'language/literals/regexp/u-invalid-range-negative-lookahead.js',
  'language/literals/regexp/u-invalid-range-negative-lookbehind.js',
  'language/literals/regexp/u-unicode-esc-bounds.js',
  'language/literals/regexp/u-unicode-esc-non-hex.js',
  'language/literals/regexp/unicode-escape-nls-err.js',
])

const skipEvaluatingTheseIncludes = new Set([
  'nativeFunctionMatcher.js', // We don't preserve "toString()" on functions
])

const skipEvaluatingTheseFeatures = new Set([
  // Node's version of V8 doesn't implement these
  'hashbang',
  'legacy-regexp',
  'regexp-duplicate-named-groups',
  'symbols-as-weakmap-keys',
  'tail-call-optimization',

  // We don't care about API-related things
  'ArrayBuffer',
  'change-array-by-copy',
  'DataView',
  'resizable-arraybuffer',
  'ShadowRealm',
  'SharedArrayBuffer',
  'String.prototype.toWellFormed',
  'Symbol.match',
  'Symbol.replace',
  'Symbol.unscopables',
  'Temporal',
  'TypedArray',
])

const skipEvaluatingTheseTests = new Set([
  // This test is skipped because it crashes V8:
  //
  //   #
  //   # Fatal error in , line 0
  //   # Check failed: i < self->length().
  //   #
  //   #
  //   #
  //   #FailureMessage Object: 0x7fffac9a1c30
  //    1: 0xc2abf1  [node]
  //    2: 0x1f231d4 V8_Fatal(char const*, ...) [node]
  //    3: 0xdac209 v8::FixedArray::Get(v8::Local<v8::Context>, int) const [node]
  //    4: 0xb74ab7  [node]
  //    5: 0xb76a2d  [node]
  //    6: 0xf2436f v8::internal::Isolate::RunHostImportModuleDynamicallyCallback(v8::internal::MaybeHandle<v8::internal::Script>,
  //                v8::internal::Handle<v8::internal::Object>, v8::internal::MaybeHandle<v8::internal::Object>) [node]
  //    7: 0x137e3db v8::internal::Runtime_DynamicImportCall(int, unsigned long*, v8::internal::Isolate*) [node]
  //    8: 0x17fd4f4  [node]
  //
  'language/expressions/dynamic-import/2nd-param-assert-enumeration.js',
])

function findFiles() {
  function visit(dir) {
    for (const entry of fs.readdirSync(dir)) {
      const fullEntry = path.join(dir, entry)
      const stats = fs.statSync(fullEntry)
      if (stats.isDirectory()) {
        visit(fullEntry)
      } else if (stats.isFile() && entry.endsWith('.js') && !entry.includes('_FIXTURE')) {
        files.push(fullEntry)
      }
    }
  }

  const files = []
  for (const entry of fs.readdirSync(testDir)) {
    if (entry === 'staging' || entry === 'intl402' || entry === 'built-ins') continue // We're not interested in these
    visit(path.join(testDir, entry))
  }

  // Reverse for faster iteration times because many of the more interesting tests come last
  return files.reverse()
}

async function checkTransformAPI({ esbuild, file, content, yaml }) {
  if (yaml.flags) {
    if (yaml.flags.includes('onlyStrict')) content = '"use strict";' + content
    if (yaml.flags.includes('module')) content += '\nexport {}'
  }

  // Step 1: Try transforming the file normally
  const shouldParse = !yaml.negative || yaml.negative.phase !== 'parse'
  let result
  try {
    result = await esbuild.transform(content, { sourcefile: file })
  } catch (error) {
    if (shouldParse) {
      error.kind = 'Transform'
      throw error
    }
    return // Stop now if this test is supposed to fail
  }
  if (!shouldParse) {
    const error = new Error('Unexpected successful transform')
    error.kind = 'Transform'
    throw error
  }

  // Step 2: Try transforming the output again (this should always succeed)
  let result2
  try {
    result2 = await esbuild.transform(result.code, { sourcefile: file })
  } catch (error) {
    error.kind = 'Reparse'
    throw error
  }

  // Step 3: The output should be the same the second time
  if (result2.code !== result.code) {
    const lines = result.code.split('\n')
    const lines2 = result2.code.split('\n')
    let i = 0
    while (i < lines.length && i < lines2.length && lines[i] === lines2[i]) i++
    const error = { toString: () => `${redColor}-${lines[i]}\n${greenColor}+${lines2[i]}${resetColor}` }
    error.kind = 'Reprint'
    throw error
  }

  // Step 4: Try minifying the output once
  let result4
  try {
    result4 = await esbuild.transform(result2.code, { sourcefile: file, minify: true })
  } catch (error) {
    error.kind = 'Minify'
    throw error
  }

  // Step 5: Try minifying the output again
  let result5
  try {
    result5 = await esbuild.transform(result4.code, { sourcefile: file, minify: true })
  } catch (error) {
    error.kind = 'Minify'
    throw error
  }
}

async function checkBuildAPI({ esbuild, file, content, yaml }) {
  const plugins = []
  if (yaml.flags) {
    const isOnlyStrict = yaml.flags.includes('onlyStrict')
    const isModule = yaml.flags.includes('module')
    if (isOnlyStrict || isModule) {
      plugins.push({
        name: 'modify',
        setup(build) {
          build.onLoad({ filter: /./ }, args => {
            if (args.path === file) {
              let loaded = content
              if (isOnlyStrict) loaded = '"use strict";' + loaded
              if (isModule) loaded += '\nexport {}'
              return { contents: loaded }
            }
          })
        },
      })
    }
  }

  // Step 1: Try building the file normally
  const isModule = yaml.flags && yaml.flags.includes('module')
  const isDynamicImport = yaml.flags && yaml.flags.includes('dynamic-import')
  const isAsync = yaml.flags && yaml.flags.includes('async')
  const shouldParse = !yaml.negative || yaml.negative.phase === 'runtime'
  let result
  try {
    const options = {
      entryPoints: [file],
      write: false,
      keepNames: true,
      logLevel: 'silent',
      plugins,
      target: 'node' + process.version.slice(1),
      logOverride: { 'direct-eval': 'warning' },
    }
    if (isModule || isDynamicImport || isAsync) {
      options.bundle = true
      options.format = isModule ? 'esm' : 'iife'
      options.external = [
        '', // Some tests use this as a dummy argument to an "import('')" that is never evaluated
      ]
    }
    result = await esbuild.build(options)
  } catch (error) {
    if (shouldParse) {
      error.kind = 'Build'
      throw error
    }
    return // Stop now if this test is supposed to fail
  }
  if (!shouldParse) {
    const error = new Error('Unexpected successful build')
    error.kind = 'Build'
    throw error
  }

  // Don't evaluate problematic files
  const hasDirectEval = result.warnings.some(msg => msg.id === 'direct-eval')
  if (
    hasDirectEval ||
    skipEvaluatingTheseTests.has(path.relative(testDir, file)) ||
    (yaml.includes && yaml.includes.some(include => skipEvaluatingTheseIncludes.has(include))) ||
    (yaml.features && yaml.features.some(feature => skipEvaluatingTheseFeatures.has(feature)))
  ) {
    return
  }

  // Step 3: Try evaluating the file using node
  const importDir = path.dirname(file)
  const shouldEvaluate = !yaml.negative
  try {
    await runCodeInHarness(yaml, content, importDir)
  } catch (error) {
    // Ignore tests that fail when run in node
    if (shouldEvaluate) console.log(eraseProgressBar() + dimColor + `IGNORING ${path.relative(testDir, file)}: ${error}` + resetColor)
    return
  }
  if (!shouldEvaluate) {
    // Ignore tests that incorrectly pass when run in node
    return
  }

  // Step 3: Try evaluating the file we generated
  const code = result.outputFiles[0].text
  try {
    await runCodeInHarness(yaml, code, importDir)
  } catch (error) {
    if (shouldEvaluate) {
      if (typeof error === 'string') error = new Error(error)
      error.kind = 'Evaluate'
      throw error
    }
    return // Stop now if this test is supposed to fail
  }
  if (!shouldEvaluate) {
    const error = new Error('Unexpected successful evaluation')
    error.kind = 'Evaluate'
    throw error
  }

  // Step 4: If evaluation worked and was supposed to work, check to see
  // if it still works after running esbuild's syntax lowering pass on it
  for (let version = 2015; version <= 2022; version++) {
    let result
    try {
      result = await esbuild.transform(code, { sourcefile: file, target: `es${version}` })
    } catch (error) {
      continue // This means esbuild doesn't support lowering this code to this old a version
    }
    try {
      await runCodeInHarness(yaml, code, importDir)
    } catch (error) {
      if (typeof error === 'string') error = new Error(error)
      error.kind = 'Lower'
      throw error
    }
    break
  }
}

async function main() {
  const startTime = Date.now()

  // Get this warning out of the way so it's not in the middle of our output.
  // Note: If this constructor is missing, you need to run node with the
  // "--experimental-vm-modules" flag (and use a version of node that has it).
  {
    const temp = new vm.SourceTextModule('')
    await temp.link(() => { throw new Error })
    await temp.evaluate()
  }

  console.log(`\n${dimColor}Finding tests...${resetColor}`)
  const files = findFiles()
  console.log(`Found ${files.length} test files`)

  console.log(`\n${dimColor}Installing esbuild...${resetColor}`)
  const esbuild = installForTests()

  console.log(`\n${dimColor}Running tests...${resetColor}\n`)
  const errorCounts = {}
  let skippedCount = 0

  await forEachInParallel(files, 32, async (file) => {
    // Don't parse files that esbuild deliberately handles differently
    if (skipTheseTests.has(path.relative(testDir, file))) {
      skippedCount++
      return
    }

    try {
      const content = fs.readFileSync(file, 'utf8')
      const start = content.indexOf('/*---')
      const end = content.indexOf('---*/')
      if (start < 0 || end < 0) throw new Error(`Missing YAML metadata`)
      const yaml = jsYaml.safeLoad(content.slice(start + 5, end))

      // Don't parse files that test things we don't care about
      if (yaml.features && yaml.features.some(feature => skipTheseFeatures.has(feature))) {
        skippedCount++
        return
      }

      await checkTransformAPI({ esbuild, file, content, yaml })
      await checkBuildAPI({ esbuild, file, content, yaml })
    }

    catch (error) {
      errorCounts[error.kind] = (errorCounts[error.kind] || 0) + 1
      printError(file, error)
    }
  })

  const table = []
  table.push(['Total tests', `${files.length}`])
  table.push(['Tests ran', `${files.length - skippedCount}`])
  table.push(['Tests skipped', `${skippedCount}`])
  for (const kind of Object.keys(errorCounts).sort()) {
    table.push([kind + ' errors', `${errorCounts[kind]}`])
  }
  const seconds = (Date.now() - startTime) / 1000
  const minutes = Math.floor(seconds / 60)
  table.push(['Time taken', `${minutes ? `${minutes} min ${+(seconds - minutes * 60).toFixed(1)} sec` : `${+seconds.toFixed(1)} sec`}`])
  const maxLength = Math.max(...table.map(x => x[0].length))
  printNewlineWhenErasing = true
  process.stdout.write(eraseProgressBar())
  for (const [key, value] of table) {
    console.log(`${boldColor}${(key + ':').padEnd(maxLength + 1)}${resetColor} ${value}`)
  }
}

function forEachInParallel(items, batchSize, callback) {
  return new Promise((resolve, reject) => {
    let inFlight = 0
    let i = 0

    function next() {
      if (i === items.length && inFlight === 0) {
        process.stdout.write(eraseProgressBar())
        return resolve()
      }

      const completed = Math.floor(progressBarLength * i / items.length)
      if (previousProgressBar !== completed) {
        previousProgressBar = completed
        const progressHead = '\u2501'.repeat(Math.max(0, completed - 1))
        const progressBoundary = completed ? '\u252B' : ''
        const progressTail = '\u2500'.repeat(progressBarLength - completed)
        process.stdout.write(`\r` + greenColor + progressHead + progressBoundary + dimColor + progressTail + resetColor)
      }

      while (i < items.length && inFlight < batchSize) {
        inFlight++
        callback(items[i++]).then(() => {
          inFlight--
          next()
        }, reject)
      }
    }

    next()
  })
}

const harnessFiles = new Map
let defaultHarness = ''

for (const entry of fs.readdirSync(harnessDir)) {
  if (entry.startsWith('.') || !entry.endsWith('.js')) {
    continue
  }
  const file = path.join(harnessDir, entry)
  const content = fs.readFileSync(file, 'utf8')
  if (entry === 'assert.js' || entry === 'sta.js') {
    defaultHarness += content
    continue
  }
  harnessFiles.set(entry, content)
}

function createHarnessForTest(yaml) {
  let harness = defaultHarness

  if (yaml.includes) {
    for (const include of yaml.includes) {
      const content = harnessFiles.get(include)
      if (!content) throw new Error(`Included file is missing: ${include}`)
      harness += content
    }
  }

  return harness
}

async function runCodeInHarness(yaml, code, importDir) {
  const context = {}
  const isAsync = yaml.flags && yaml.flags.includes('async')
  const isModule = yaml.flags && yaml.flags.includes('module')
  const isRaw = yaml.flags && yaml.flags.includes('raw')

  // See: https://github.com/nodejs/node/issues/36351
  const unique = () => '//' + Math.random()

  const runCode = async () => {
    const moduleCache = new Map
    const dynamicImportCache = new Map

    const findModule = (modulePath) => {
      let module = moduleCache.get(modulePath)
      if (!module) {
        const code = fs.readFileSync(modulePath, 'utf8')
        if (modulePath.endsWith('json')) {
          const evaluate = function () {
            this.setExport('default', vm.runInContext('JSON.parse', context)(code))
          }
          module = new vm.SyntheticModule(['default'], evaluate, { context })
        } else {
          module = new vm.SourceTextModule(code + unique(), { context, importModuleDynamically })
        }
        moduleCache.set(modulePath, module)
      }
      return module
    }

    const linker = (specifier, referencingModule) => {
      return findModule(path.join(importDir, specifier))
    }

    const importModuleDynamically = (specifier, script) => {
      const where = path.join(importDir, specifier)
      let promise = dynamicImportCache.get(where)
      if (!promise) {
        const module = findModule(where, context)
        if (module.status === 'unlinked') {
          promise = module.link(linker)
            .then(() => module.evaluate())
            .then(() => module)
        } else {
          promise = Promise.resolve(module)
        }
        dynamicImportCache.set(where, promise)
      }
      return promise
    }

    vm.createContext(context)
    if (!isRaw) vm.runInContext(createHarnessForTest(yaml), context)

    if (isModule) {
      const module = new vm.SourceTextModule(code + unique(), { context, importModuleDynamically })
      await module.link(linker)
      await module.evaluate()
    } else {
      const script = new vm.Script(code, { importModuleDynamically })
      script.runInContext(context)
    }
  }

  if (isAsync) {
    await new Promise((resolve, reject) => {
      context.$DONE = err => err ? reject(err) : resolve()
      runCode(code, context).catch(reject)
    })
  } else {
    await runCode(code, context)
  }
}

function printError(file, error) {
  let detail

  if (error.errors) {
    const { text, location } = error.errors[0]
    if (location) {
      const { file, line, column, lineText, length } = location
      detail = '  ' + dimColor + path.basename(file) + ':' + line + ':' + column + ': ' + resetColor + text + '\n' +
        '  ' + dimColor + lineText.slice(0, column) + greenColor + lineText.slice(column, column + length) + dimColor + lineText.slice(column + length) + resetColor + '\n' +
        '  ' + greenColor + ' '.repeat(column) + (length > 1 ? '~'.repeat(length) : '^') + resetColor
    } else {
      detail = dimColor + ('\n' + text).split('\n').join('\n  ').slice(1) + resetColor
    }
  } else {
    detail = dimColor + ('\n' + error).split('\n').join('\n  ').slice(1) + resetColor
  }

  const prettyPath = path.relative(testDir, file)
  printNewlineWhenErasing = true
  console.log(eraseProgressBar() + tagMap[error.kind] + ' ' + prettyPath + '\n' + detail)
  printNewlineWhenErasing = true
}

const tagMap = {
  Transform: redBgRedColor + `[` + redBgWhiteColor + `TRANSFORM ERROR` + redBgRedColor + `]` + resetColor,
  Build: magentaBgMagentaColor + `[` + magentaBgBlackColor + `BUILD ERROR` + magentaBgMagentaColor + `]` + resetColor,
  Reparse: yellowBgYellowColor + `[` + yellowBgBlackColor + `REPARSE ERROR` + yellowBgYellowColor + `]` + resetColor,
  Reprint: cyanBgCyanColor + `[` + cyanBgBlackColor + `REPRINT ERROR` + cyanBgCyanColor + `]` + resetColor,
  Minify: blueBgBlueColor + `[` + blueBgWhiteColor + `MINIFY ERROR` + blueBgBlueColor + `]` + resetColor,
  Evaluate: greenBgGreenColor + `[` + greenBgWhiteColor + `EVALUATE ERROR` + greenBgGreenColor + `]` + resetColor,
  Lower: whiteBgWhiteColor + `[` + whiteBgBlackColor + `LOWER ERROR` + whiteBgWhiteColor + `]` + resetColor,
}

process.on('unhandledRejection', () => {
  // Don't exit when a test does this
})

main().catch(e => setTimeout(() => {
  throw e
}))
