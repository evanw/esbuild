// To run this, you must first build the Deno package with "make platform-deno"
import * as esbuild from '../deno/mod.js'
import * as path from 'https://deno.land/std@0.95.0/path/mod.ts'
import * as asserts from 'https://deno.land/std@0.95.0/testing/asserts.ts'

const rootTestDir = path.join(path.dirname(path.fromFileUrl(import.meta.url)), '.deno-tests')
let testDidFail = false

try {
  Deno.removeSync(rootTestDir, { recursive: true })
} catch {
}
Deno.mkdirSync(rootTestDir, { recursive: true })

function test(name, fn) {
  let testDir = path.join(rootTestDir, name)
  Deno.test(name, async () => {
    await Deno.mkdir(testDir, { recursive: true })
    try {
      await fn({ testDir })
      await Deno.remove(testDir, { recursive: true }).catch(() => null)
    } catch (e) {
      testDidFail = true
      throw e
    } finally {
      esbuild.stop()
    }
  })
}

window.addEventListener("unload", (e) => {
  if (testDidFail) {
    console.error(`❌ deno tests failed`)
  } else {
    console.log(`✅ deno tests passed`)
    try {
      Deno.removeSync(rootTestDir, { recursive: true })
    } catch {
      // root test dir possibly already removed, so ignore
    }
  }
})

test("basicBuild", async ({ testDir }) => {
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
})

test("basicTransform", async () => {
  const ts = 'let x: number = 1+2'
  const result = await esbuild.transform(ts, { loader: 'ts' })
  asserts.assertStrictEquals(result.code, 'let x = 1 + 2;\n')
})

test("largeTransform", async () => {
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
})

