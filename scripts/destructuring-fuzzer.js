function generateTestCase(assign) {
  let sideEffectCount = 0
  let patternCount = 0
  let limit = 10
  let depth = 0

  function choice(n) {
    return Math.random() * n | 0
  }

  function patternAndValue() {
    patternCount++
    switch (choice(3)) {
      case 0: return array()
      case 1: return object()
      case 2: return [assign(), choice(10), choice(10)]
    }
  }

  function sideEffect(result) {
    return `s(${sideEffectCount++}${result ? `, ${result}` : ''})`
  }

  function indent(open, items, close) {
    let tab = '  '
    items = items.map(i => `\n${tab.repeat(depth + 1)}${i}`).join(',')
    return `${open}${items}\n${tab.repeat(depth)}${close}`
  }

  function id() {
    return String.fromCharCode('a'.charCodeAt(0) + choice(3))
  }

  function array() {
    let count = 1 + choice(2)
    let pattern = []
    let value = []

    depth++
    for (let i = 0; i < count; i++) {
      if (patternCount > limit) break
      let [pat, val, defVal] = patternAndValue()
      switch (choice(3)) {
        case 0:
          pattern.push(pat)
          value.push(val)
          break
        case 1:
          pattern.push(`${pat} = ${sideEffect(defVal)}`)
          value.push(val)
          break
        case 2:
          pattern.push(`${pat} = ${sideEffect(defVal)}`)
          value.push(defVal)
          break
      }
      if (choice(10) < 8) value.push(val)
    }
    if (choice(2)) {
      pattern.push(`...${assign()}`)
      if (choice(10) < 8) value.push(choice(10))
    }
    depth--

    return [
      indent('[', pattern, ']'),
      indent('[', value, ']'),
      '[]',
    ]
  }

  function object() {
    let count = 1 + choice(2)
    let pattern = []
    let value = []
    let valKeys = new Set()

    depth++
    for (let i = 0; i < count; i++) {
      if (patternCount > limit) break
      let valKey = id()
      if (valKeys.has(valKey)) continue
      valKeys.add(valKey)
      let patKey = choice() ? valKey : `[${sideEffect(`'${valKey}'`)}]`
      let [pat, val, defVal] = patternAndValue()
      switch (choice(3)) {
        case 0:
          pattern.push(`${patKey}: ${pat}`)
          value.push(`${valKey}: ${val}`)
          break
        case 1:
          pattern.push(`${patKey}: ${pat} = ${sideEffect(defVal)}`)
          value.push(`${valKey}: ${val}`)
          break
        case 2:
          pattern.push(`${patKey}: ${pat} = ${sideEffect(defVal)}`)
          value.push(`${valKey}: ${defVal}`)
          break
      }
    }
    if (choice(2)) {
      pattern.push(`...${assign()}`)
      if (choice(10) < 8) value.push(`${id()}: ${choice(10)}`)
    }
    depth--

    return [
      indent('{', pattern, '}'),
      indent('{', value, '}'),
      '{}',
    ]
  }

  return choice(2) ? array() : object()
}

function evaluate(code) {
  let effectTrace = []
  let assignTarget = {}
  let sideEffect = (id, value) => (effectTrace.push(id), value)
  new Function('a', 's', code)(assignTarget, sideEffect)
  return JSON.stringify({ assignTarget, effectTrace })
}

function generateTestCases(trials) {
  let testCases = []

  while (testCases.length < trials) {
    let ids = []
    let assignCount = 0
    let [pattern, value] = generateTestCase(() => {
      let id = `_${assignCount++}`
      ids.push(id)
      return id
    })
    try {
      evaluate(`(${pattern.replace(/_/g, 'a._')} = ${value});`)
      testCases.push([pattern, value, ids])
    } catch (e) {
    }
  }

  return testCases
}

function AssignmentOperator([pattern, value]) {
  let ts = `(${pattern.replace(/_/g, 'a._')} = ${value});`
  let js = ts
  return { js, ts }
}

function NamespaceExport([pattern, value]) {
  let ts = `namespace a { export var ${`${pattern} = ${value}`} }`
  let js = `(${pattern.replace(/_/g, 'a._')} = ${value});`
  return { js, ts }
}

function ConstDeclaration([pattern, value, ids]) {
  let ts = `const ${pattern} = ${value};${ids.map(id => `\na.${id} = ${id};`).join('')}`
  let js = ts
  return { js, ts }
}

function LetDeclaration([pattern, value, ids]) {
  let ts = `let ${pattern} = ${value};${ids.map(id => `\na.${id} = ${id};`).join('')}`
  let js = ts
  return { js, ts }
}

function VarDeclaration([pattern, value, ids]) {
  let ts = `var ${pattern} = ${value};${ids.map(id => `\na.${id} = ${id};`).join('')}`
  let js = ts
  return { js, ts }
}

function TryCatchBinding([pattern, value, ids]) {
  let ts = `try { throw ${value} } catch (${pattern}) { ${ids.map(id => `a.${id} = ${id};`).join('\n')} }`
  let js = ts
  return { js, ts }
}

function FunctionStatementArguments([pattern, value, ids]) {
  let ts = `function foo(${pattern}) { ${ids.map(id => `a.${id} = ${id};`).join('\n')} }\nfoo(${value});`
  let js = ts
  return { js, ts }
}

function FunctionExpressionArguments([pattern, value, ids]) {
  let ts = `(function(${pattern}) { ${ids.map(id => `a.${id} = ${id};`).join('\n')} })(${value});`
  let js = ts
  return { js, ts }
}

function ArrowFunctionArguments([pattern, value, ids]) {
  let ts = `((${pattern}) => { ${ids.map(id => `a.${id} = ${id};`).join('\n')} })(${value});`
  let js = ts
  return { js, ts }
}

function ObjectMethodArguments([pattern, value, ids]) {
  let ts = `({ foo(${pattern}) { ${ids.map(id => `a.${id} = ${id};`).join('\n')} } }).foo(${value});`
  let js = ts
  return { js, ts }
}

function ClassStatementMethodArguments([pattern, value, ids]) {
  let ts = `class Foo { foo(${pattern}) { ${ids.map(id => `a.${id} = ${id};`).join('\n')} } }\nnew Foo().foo(${value});`
  let js = ts
  return { js, ts }
}

function ClassExpressionMethodArguments([pattern, value, ids]) {
  let ts = `(new (class { foo(${pattern}) { ${ids.map(id => `a.${id} = ${id};`).join('\n')} } })).foo(${value});`
  let js = ts
  return { js, ts }
}

function ForLoopConst([pattern, value, ids]) {
  let ts = `var i; for (const ${pattern} = ${value}; i < 1; i++) { ${ids.map(id => `a.${id} = ${id};`).join('\n')} }`
  let js = ts
  return { js, ts }
}

function ForLoopLet([pattern, value, ids]) {
  let ts = `for (let ${pattern} = ${value}, i = 0; i < 1; i++) { ${ids.map(id => `a.${id} = ${id};`).join('\n')} }`
  let js = ts
  return { js, ts }
}

function ForLoopVar([pattern, value, ids]) {
  let ts = `for (var ${pattern} = ${value}, i = 0; i < 1; i++) { ${ids.map(id => `a.${id} = ${id};`).join('\n')} }`
  let js = ts
  return { js, ts }
}

function ForLoop([pattern, value]) {
  let ts = `for (${pattern.replace(/_/g, 'a._')} = ${value}; 0; ) ;`
  let js = ts
  return { js, ts }
}

function ForOfLoopConst([pattern, value, ids]) {
  let ts = `for (const ${pattern} of [${value}]) { ${ids.map(id => `a.${id} = ${id};`).join('\n')} }`
  let js = ts
  return { js, ts }
}

function ForOfLoopLet([pattern, value, ids]) {
  let ts = `for (let ${pattern} of [${value}]) { ${ids.map(id => `a.${id} = ${id};`).join('\n')} }`
  let js = ts
  return { js, ts }
}

function ForOfLoopVar([pattern, value, ids]) {
  let ts = `for (var ${pattern} of [${value}]) { ${ids.map(id => `a.${id} = ${id};`).join('\n')} }`
  let js = ts
  return { js, ts }
}

function ForOfLoop([pattern, value]) {
  let ts = `for (${pattern.replace(/_/g, 'a._')} of [${value}]) ;`
  let js = ts
  return { js, ts }
}

async function verify(test, transform, testCases) {
  let indent = t => t.replace(/\n/g, '\n  ')
  let newline = false
  console.log(`${test.name} (${transform.name}):`)

  await concurrentMap(testCases, 20, async (testCase) => {
    let { js, ts } = test(testCase)
    let expected
    try {
      expected = evaluate(js)
    } catch (e) {
      return
    }

    let transformed
    let actual
    try {
      transformed = await transform(ts)
      actual = evaluate(transformed)
    } catch (e) {
      actual = e + ''
    }

    if (actual !== expected) {
      process.stdout.write('X')
      newline = true

      if (process.argv.indexOf('--verbose') >= 0) {
        console.log('\n' + '='.repeat(80))
        console.log(indent(`Original code:\n${ts}`))
        console.log(indent(`Transformed code:\n${transformed}`))
        console.log(indent(`Expected output:\n${expected}`))
        console.log(indent(`Actual output:\n${actual}`))
        newline = false
      }
    } else {
      process.stdout.write('-')
      newline = true
    }
  })

  if (newline) process.stdout.write('\n')
}

function concurrentMap(items, batch, callback) {
  return new Promise((resolve, reject) => {
    let index = 0
    let pending = 0
    let next = () => {
      if (index === items.length && pending === 0) {
        resolve()
      } else if (index < items.length) {
        let item = items[index++]
        pending++
        callback(item).then(() => {
          pending--
          next()
        }, e => {
          items.length = 0
          reject(e)
        })
      }
    }
    for (let i = 0; i < batch; i++)next()
  })
}

async function main() {
  let rimraf = require('rimraf')
  let path = require('path')
  let installDir = path.join(__dirname, '.destructuring-fuzzer')

  let es = require('./esbuild').installForTests(installDir)
  let esbuild = async (x) => (await es.transform(x, { target: 'es6', loader: 'ts' })).js.trim()

  console.log(`
Options:
  --verbose = Print details for failures

Legend:
  - = The transform function passed
  X = The transform function failed
`)

  let tests = [
    AssignmentOperator,
    NamespaceExport,
    ConstDeclaration,
    LetDeclaration,
    VarDeclaration,
    TryCatchBinding,
    FunctionStatementArguments,
    FunctionExpressionArguments,
    ArrowFunctionArguments,
    ObjectMethodArguments,
    ClassStatementMethodArguments,
    ClassExpressionMethodArguments,
    ForLoopConst,
    ForLoopLet,
    ForLoopVar,
    ForLoop,
    ForOfLoopConst,
    ForOfLoopLet,
    ForOfLoopVar,
    ForOfLoop,
  ]
  let transforms = [
    esbuild,
  ]
  let testCases = generateTestCases(100)

  for (let transform of transforms) {
    for (let test of tests)
      await verify(test, transform, testCases)
  }

  rimraf.sync(installDir, { disableGlob: true })
}

main().catch(e => setTimeout(() => { throw e }))
