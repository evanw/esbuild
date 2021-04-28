// To run this, you must first build the Deno package with "make platform-deno"
import * as esbuild from '../deno/mod.js'
import * as path from 'https://deno.land/std@0.93.0/path/mod.ts'
import * as asserts from 'https://deno.land/std@0.93.0/testing/asserts.ts'

const rootTestDir = path.join(path.dirname(path.fromFileUrl(import.meta.url)), '.deno-tests')

let tests = {
  async basicBuild({ testDir }) {
    const input = path.join(testDir, 'in.ts')
    const dep = path.join(testDir, 'dep.ts')
    const output = path.join(testDir, 'out.ts')
    await Deno.writeTextFile(input, 'import dep from "./dep.ts"; export default dep === 123')
    await Deno.writeTextFile(dep, 'export default 123')
    await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outfile: output,
      format: 'esm',
    })
    const result = await import(path.toFileUrl(output))
    asserts.assertStrictEquals(result.default, true)
  },

  async largeBuild() {
    // This should be large enough to be bigger than Deno's write buffer
    let x = '0'
    for (let i = 0; i < 1000; i++)x += '+' + i
    x += ','
    let y = 'return['
    for (let i = 0; i < 1000; i++)y += x
    y += ']'
    const result = await esbuild.build({
      stdin: {
        contents: y,
      },
      write: false,
      minify: true,
    })
    asserts.assertStrictEquals(result.outputFiles[0].text, y.slice(0, -2) + '];\n')
  },

  async basicTransform() {
    const ts = 'let x: number = 1+2'
    const result = await esbuild.transform(ts, { loader: 'ts' })
    asserts.assertStrictEquals(result.code, 'let x = 1 + 2;\n')
  },

  async largeTransform() {
    // This should be large enough to use the file system passing optimization
    let x = '0'
    for (let i = 0; i < 1000; i++)x += '+' + i
    x += ','
    let y = 'return['
    for (let i = 0; i < 1000; i++)y += x
    y += ']'
    const result = await esbuild.transform(y, { minify: true })
    asserts.assertStrictEquals(result.code, y.slice(0, -2) + '];\n')
  },
}

async function main() {
  try {
    Deno.removeSync(rootTestDir, { recursive: true })
  } catch {
  }
  Deno.mkdirSync(rootTestDir, { recursive: true })

  // Run all tests concurrently
  const runTest = async ([name, fn]) => {
    let testDir = path.join(rootTestDir, name)
    try {
      await Deno.mkdir(testDir, { recursive: true })
      await fn({ testDir })
      await Deno.remove(testDir, { recursive: true }).catch(() => null)
      return true
    } catch (e) {
      console.error(`❌ ${name}: ${e && e.stack || e}`)
      return false
    }
  }
  const allTestsPassed = (await Promise.all(Object.entries(tests).map(runTest))).every(success => success)
  esbuild.stop()

  if (!allTestsPassed) {
    console.error(`❌ deno tests failed`)
    Deno.exit(1)
  } else {
    console.log(`✅ deno tests passed`)
    try {
      Deno.removeSync(rootTestDir, { recursive: true })
    } catch {
    }
  }
}

await main()
