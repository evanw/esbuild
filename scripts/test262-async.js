// This file runs test262 tests for the async iteration proposal:
// https://github.com/tc39/proposal-async-iteration. It's intended
// to be informative (to identify failure points) but not to be
// run in CI. Not even node/V8 passes all of these tests.

const esbuild = require('./esbuild').installForTests()
const ts = require('./node_modules/typescript')
const fs = require('fs')
const vm = require('vm')

const test262Dir = __dirname + '/../demo/test262'
const shim = ``
  + fs.readFileSync(test262Dir + '/harness/assert.js', 'utf8')
  + fs.readFileSync(test262Dir + '/harness/asyncHelpers.js', 'utf8')
  + fs.readFileSync(test262Dir + '/harness/compareArray.js', 'utf8')
  + fs.readFileSync(test262Dir + '/harness/propertyHelper.js', 'utf8')
  + fs.readFileSync(test262Dir + '/harness/sta.js', 'utf8')

async function main() {
  const tests = []

  for (const dir of [
    test262Dir + '/test/language/statements/async-generator',
    test262Dir + '/test/language/statements/for-await-of',
  ]) {
    for (const name of fs.readdirSync(dir)) {
      if (name.endsWith('.js')) {
        tests.push(dir + '/' + name)
      }
    }
  }

  for (const test of tests) {
    const stuck = () => console.log(`❌ ${test}: stuck`)
    process.on('exit', stuck)

    let js = fs.readFileSync(test, 'utf8')
    let negative = /negative:/.test(js)

    let flags = /flags:.*/.exec(js)
    flags = flags && new Set(/flags: \[([^\]]*)\]/.exec(js)[1].split(', '))

    let didFail = false

    const check = async (kind, codePromise) => {
      let error
      try {
        let { code } = await codePromise
        code = shim + code
        if (flags && flags.has('onlyStrict')) {
          code = '"use strict";\n' + code
        }
        await new Promise((resolve, reject) => {
          if (flags && flags.has('async')) {
            vm.runInContext(code, vm.createContext({ $DONE: err => err ? reject(err) : resolve() }))
          } else {
            vm.runInContext(code, vm.createContext({}))
            resolve()
          }
        })
      } catch (err) {
        error = err
      }
      if (!error === !negative) {
        return true
      }
      if (!didFail) {
        didFail = true
        console.log(`\n❌ \x1B[37m${test}\x1B[0m`)
      }
      if (kind === 'native') kind = '\x1B[32m[' + kind + ']\x1B[0m'
      else if (kind.startsWith('ts')) kind = '\x1B[34m[' + kind + ']\x1B[0m'
      else kind = '\x1B[33m[' + kind + ']\x1B[0m'
      console.log(`  ${kind} ${(error + '' || 'unexpected success').replace(/\n/g, '\\n')}`)
      return false
    }

    const tsTranspile = async (compilerOptions) => {
      const { outputText } = ts.transpileModule(js, { compilerOptions })
      return { code: outputText }
    }

    // See if node's native support checks out (it currently doesn't due to recent specification changes)
    await check('native', { code: js })

    // Ignore tests that check whether "yield" can be used as a valid
    // identifier. This obviously doesn't work when we must transform
    // async functions into generator functions, but that's fine.
    const hasYieldKeyword = test.includes('yield-ident-valid') || test.includes('array-elem-target-yield-valid')

    // Check "tsc"
    await check('ts=esnext', tsTranspile({ target: ts.ScriptTarget.ESNext }))
    await check('ts=es2017', tsTranspile({ target: ts.ScriptTarget.ES2017 }))
    if (!hasYieldKeyword) {
      await check('ts=es2016', { code: ts.transpileModule(js, { compilerOptions: { target: ts.ScriptTarget.ES2016 } }).outputText })
    }

    // Check "esbuild"
    await check('target=esnext', esbuild.transform(js, { keepNames: true, target: 'esnext' }))
    await check('target=es2017', esbuild.transform(js, { keepNames: true, target: 'es2017' }))
    await check('for-await=false', esbuild.transform(js, { keepNames: true, supported: { 'for-await': false } }))
    if (!hasYieldKeyword) {
      await check('target=es2016', esbuild.transform(js, { keepNames: true, target: 'es2016' }))
      await check('async-await=false', esbuild.transform(js, { keepNames: true, supported: { 'async-await': false } }))
      await check('for-await=false,async-await=false', esbuild.transform(js, { keepNames: true, supported: { 'for-await': false, 'async-await': false } }))
    }

    process.off('exit', stuck)
  }

  console.log(`Done`)
}

main()
