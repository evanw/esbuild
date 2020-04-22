// This is a simple fuzzer to detect JavaScript parser issues

const fs = require('fs')
const os = require('os')
const path = require('path')
const { generate } = require('escodegen')
const child_process = require('child_process')
const { randomAST } = require('./estree-generator')
const esbuildPath = path.join(__dirname, '..', '..', 'esbuild')

function fuzz() {
  const tempDir = os.tmpdir()
  const inPath = path.join(tempDir, 'esbuild.in.js')
  const outPath = path.join(tempDir, 'esbuild.out.js')
  const targets = ['es2015', 'es2016', 'es2017', 'es2018', 'es2019', 'es2020', 'esnext']
  const loaders = ['js', 'jsx', 'ts', 'tsx']
  let testCaseCount = 0
  let printedProgress = false
  child_process.execSync('make', { cwd: path.join(__dirname, '..', '..') })

  while (true) {
    const target = targets[Math.random() * targets.length | 0]
    const loader = loaders[Math.random() * loaders.length | 0]
    const args = ['--loader:.js=' + loader, '--target=' + target]

    // Visualize progress
    if (++testCaseCount % 10 === 0) {
      process.stdout.write(`\rTest count: ${testCaseCount}`)
      printedProgress = true
    }

    // Generate a random AST
    let random = createRandomForRecording()
    let ast = randomAST(random)

    // See if esbuild fails
    let result = run({ ast, inPath, outPath, args })
    if (result.kind === 'success' || result.kind === 'uninteresting') {
      continue
    }

    // Make sure to not overwrite the progress message
    if (printedProgress) {
      process.stdout.write('\n')
      printedProgress = false
    }

    // If so, find the minimal AST that will reproduce the problem
    random = random.forPlayback()
    while (true) {
      // Generate a trimmed AST using the same random sequence
      const ast2 = randomAST(random)
      if (!random.didChange()) {
        break
      }

      // See if esbuild fails on that too
      const result2 = run({ ast: ast2, inPath, outPath, args })
      if (result2.kind !== result.kind || result2.text !== result.text) {
        random.reject()
        continue
      }

      // If so, swap with the trimmed AST
      random.accept()
      result = result2
      ast = ast2
    }

    // Print the resulting AST and error messages
    console.log('='.repeat(80), loader, target)
    console.log(result.output.trimRight())

    console.log('-'.repeat(20))
    console.log(generate(ast))

    console.log('-'.repeat(20))
    console.log(JSON.stringify(ast))
  }
}

function run({ ast, inPath, outPath, args }) {
  fs.writeFileSync(inPath, generate(ast))

  try {
    child_process.execFileSync(esbuildPath, args.concat(inPath, '--outfile=' + outPath), { stdio: 'pipe' })
    return { kind: 'success' }
  }

  catch (e) {
    let stderr = e.stderr.toString()

    // Panics are always interesting
    if (stderr.includes('panic')) {
      return { kind: 'panic', output: stderr }
    }

    for (let match, regex = /error: (.*)/g; match = regex.exec(stderr);) {
      let text = match[1]
      if (
        // Ignore uninteresting errors
        !/has already been declared/.test(text) &&
        !/This constant must be initialized/.test(text) &&
        !/There is no containing label named/.test(text) &&
        !/Multiple exports with the same name/.test(text) &&
        !/Multiple default clauses are not allowed/.test(text) &&
        !/loop variables cannot have an initializer/.test(text)
      ) {
        return { kind: 'interesting', text, output: stderr }
      }
    }
  }

  return { kind: 'uninteresting' }
}

// The AST generator takes a random object. The first time through, it records
// all random choices made in a log. The subsequent times through, it plays
// them back to generate the same AST. Each subsequent time will attempt to
// delete a single group from the AST. Groups are denoted by push() and pop(),
// which isolate all random choices made inside that group. The generator
// currently only uses groups in the "array" combinator.

function createRandomForRecording() {
  let log = []
  let stack = []
  return {
    forPlayback() {
      return createRandomForPlayback(log, 0)
    },
    choice(count) {
      const value = Math.random() * count | 0
      log.push({ kind: 'choice', value })
      return value
    },
    push() {
      const children = []
      log.push({ kind: 'push', children })
      stack.push(log)
      log = children
      return true
    },
    pop() {
      log = stack.pop()
    },
  }
}

function createRandomForPlayback(log) {
  let changedEvent = null
  let stack = []
  let index = 0
  return {
    accept() {
      changedEvent = null
      index = 0
    },
    reject() {
      if (changedEvent) changedEvent.skip = false
      changedEvent = null
      index = 0
    },
    didChange() {
      return !!changedEvent
    },
    choice() {
      const event = log[index++]
      if (event.kind !== 'choice') throw new Error('Expected choice')
      return event.value
    },
    push() {
      let event = log[index++]
      if (event.kind !== 'push') throw new Error('Expected choice')
      if (!changedEvent && event.skip === undefined) {
        event.skip = true
        changedEvent = event
      }
      if (event.skip === true) return false
      stack.push({ log, index })
      log = event.children
      index = 0
      return true
    },
    pop() {
      ({ log, index } = stack.pop())
    },
  }
}

fuzz()
