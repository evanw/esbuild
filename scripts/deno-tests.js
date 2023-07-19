// To run this, you must first build the Deno package with "make platform-deno"
import * as esbuildNative from '../deno/mod.js'
import * as esbuildWASM from '../deno/wasm.js'
import * as path from 'https://deno.land/std@0.95.0/path/mod.ts'
import * as asserts from 'https://deno.land/std@0.95.0/testing/asserts.ts'

const __dirname = path.dirname(path.fromFileUrl(import.meta.url))
const rootTestDir = path.join(__dirname, '.deno-tests')
const wasmModule = await WebAssembly.compile(await Deno.readFile(path.join(__dirname, '..', 'deno', 'esbuild.wasm')))

try {
  Deno.removeSync(rootTestDir, { recursive: true })
} catch {
}
Deno.mkdirSync(rootTestDir, { recursive: true })

function test(name, backends, fn) {
  const singleTest = (name, fn) => Deno.test({
    name,
    fn: () => new Promise((resolve, reject) => {
      const minutes = 5
      const timeout = setTimeout(() => reject(new Error(`Timeout for "${name}" after ${minutes} minutes`)), minutes * 60 * 1000)
      const cancel = () => clearTimeout(timeout)
      const promise = fn()
      promise.then(cancel, cancel)
      promise.then(resolve, reject)
    }),

    // It's ok that the Go WebAssembly runtime uses "setTimeout"
    sanitizeResources: false,
    sanitizeOps: false,
  })

  for (const backend of backends) {
    switch (backend) {
      case 'native':
        singleTest(name + '-native', async () => {
          let testDir = path.join(rootTestDir, name + '-native')
          await Deno.mkdir(testDir, { recursive: true })
          try {
            await fn({ esbuild: esbuildNative, testDir })
            await Deno.remove(testDir, { recursive: true }).catch(() => null)
          } finally {
            esbuildNative.stop()
          }
        })
        break

      case 'wasm-main':
        singleTest(name + '-wasm-main', async () => {
          let testDir = path.join(rootTestDir, name + '-wasm-main')
          await esbuildWASM.initialize({ wasmModule, worker: false })
          await Deno.mkdir(testDir, { recursive: true })
          try {
            await fn({ esbuild: esbuildWASM, testDir })
            await Deno.remove(testDir, { recursive: true }).catch(() => null)
          } finally {
            esbuildWASM.stop()
          }
        })
        break

      case 'wasm-worker':
        singleTest(name + '-wasm-worker', async () => {
          let testDir = path.join(rootTestDir, name + '-wasm-worker')
          await esbuildWASM.initialize({ wasmModule, worker: true })
          await Deno.mkdir(testDir, { recursive: true })
          try {
            await fn({ esbuild: esbuildWASM, testDir })
            await Deno.remove(testDir, { recursive: true }).catch(() => null)
          } finally {
            esbuildWASM.stop()
          }
        })
        break

      case 'wasm-main-sys':
        // WASM with sys access not supported on Windows because Go assumes that
        // inside of WASM all FS operations operate on Unix-style paths, but
        // Windows uses backslashes. This is a limitation of Go's WASM support
        // and unlikely to be fixable in esbuild right now.
        if (Deno.build.os === "windows") break;
        singleTest(name + '-wasm-main-sys', async () => {
          let testDir = path.join(rootTestDir, name + '-wasm-main-sys')
          await esbuildWASM.initialize({
            wasmModule,
            worker: false,
            wasmSystemAccess: {
              fsSpecifier: "node:fs",
              processSpecifier: "node:process",
            }
          })
          await Deno.mkdir(testDir, { recursive: true })
          try {
            await fn({ esbuild: esbuildWASM, testDir })
            await Deno.remove(testDir, { recursive: true }).catch(() => null)
          } finally {
            esbuildWASM.stop()
          }
        })
        break
  

      case 'wasm-main-sys-namespace':
        // WASM with sys access not supported on Windows because Go assumes that
        // inside of WASM all FS operations operate on Unix-style paths, but
        // Windows uses backslashes. This is a limitation of Go's WASM support
        // and unlikely to be fixable in esbuild right now.
        if (Deno.build.os === "windows") break;
        singleTest(name + '-wasm-main-sys-namespace', async () => {
          let testDir = path.join(rootTestDir, name + '-wasm-main-sys-namespace')
          await esbuildWASM.initialize({
            wasmModule,
            worker: false,
            wasmSystemAccess: {
              fsNamespace: await import("node:fs"),
              processNamespace: await import("node:process"),
            }
          })
          await Deno.mkdir(testDir, { recursive: true })
          try {
            await fn({ esbuild: esbuildWASM, testDir })
            await Deno.remove(testDir, { recursive: true }).catch(() => null)
          } finally {
            esbuildWASM.stop()
          }
        })
        break
  
      case 'wasm-worker-sys':
          // WASM with sys access not supported on Windows because Go assumes that
          // inside of WASM all FS operations operate on Unix-style paths, but
          // Windows uses backslashes. This is a limitation of Go's WASM support
          // and unlikely to be fixable in esbuild right now.
          if (Deno.build.os === "windows") break;
          singleTest(name + '-wasm-worker-sys', async () => {
            let testDir = path.join(rootTestDir, name + '-wasm-worker-sys')
            await esbuildWASM.initialize({
              wasmModule,
              worker: true,
              wasmSystemAccess: {
                fsSpecifier: "node:fs",
                processSpecifier: "node:process",
              }
            })
            await Deno.mkdir(testDir, { recursive: true })
            try {
              await fn({ esbuild: esbuildWASM, testDir })
              await Deno.remove(testDir, { recursive: true }).catch(() => null)
            } finally {
              esbuildWASM.stop()
            }
          })
          break
  
    }
  }
}

window.addEventListener("unload", (e) => {
  try {
    Deno.removeSync(rootTestDir, { recursive: true })
  } catch {
    // root test dir possibly already removed, so ignore
  }
})

// This test doesn't run in WebAssembly because it requires file system access
test("basicBuild", ['native', 'wasm-main-sys', 'wasm-main-sys-namespace', 'wasm-worker-sys'], async ({ esbuild, testDir }) => {
  const input = path.join(testDir, 'in.ts')
  const dep = path.join(testDir, 'dep.ts')
  const output = path.join(testDir, 'out.ts')
  await Deno.writeTextFile(input, 'import dep from "./dep.ts"; export default dep === 123')
  await Deno.writeTextFile(dep, 'export default 123')
  await esbuild.build({
    entryPoints: [input],
    bundle: true,
    outfile: output,
    write: true,
    format: 'esm',
  })
  const result = await import(path.toFileUrl(output))
  asserts.assertStrictEquals(result.default, true)
})

test("basicContext", ['native', 'wasm-main-sys', 'wasm-main-sys-namespace', 'wasm-worker-sys'], async ({ esbuild, testDir }) => {
  const input = path.join(testDir, 'in.ts')
  const dep = path.join(testDir, 'dep.ts')
  const output = path.join(testDir, 'out.ts')
  await Deno.writeTextFile(input, 'import dep from "./dep.ts"; export default dep === 123')
  await Deno.writeTextFile(dep, 'export default 123')
  const ctx = await esbuild.context({
    entryPoints: ['in.ts'],
    bundle: true,
    outfile: output,
    format: 'esm',
    absWorkingDir: testDir,
  })
  const { errors, warnings } = await ctx.rebuild()
  asserts.assertStrictEquals(errors.length, 0)
  asserts.assertStrictEquals(warnings.length, 0)
  await ctx.dispose()
  const result = await import(path.toFileUrl(output))
  asserts.assertStrictEquals(result.default, true)
})

test("basicPlugin", ['native', 'wasm-main', 'wasm-worker'], async ({ esbuild }) => {
  const build = await esbuild.build({
    entryPoints: ['<entry>'],
    bundle: true,
    format: 'esm',
    write: false,
    plugins: [{
      name: 'plug',
      setup(build) {
        build.onResolve({ filter: /^<.*>$/ }, args => ({ path: args.path, namespace: '<>' }))
        build.onLoad({ filter: /^<entry>$/ }, () => ({ contents: `import dep from "<dep>"; export default dep === 123` }))
        build.onLoad({ filter: /^<dep>$/ }, () => ({ contents: `export default 123` }))
      },
    }],
  })
  const result = await import('data:application/javascript;base64,' + btoa(build.outputFiles[0].text))
  asserts.assertStrictEquals(result.default, true)
})

test("basicTransform", ['native', 'wasm-main', 'wasm-worker'], async ({ esbuild }) => {
  const ts = 'let x: number = 1+2'
  const result = await esbuild.transform(ts, { loader: 'ts' })
  asserts.assertStrictEquals(result.code, 'let x = 1 + 2;\n')
})

// This test doesn't run in WebAssembly because of a stack overflow
test("largeTransform", ['native'], async ({ esbuild }) => {
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

test("analyzeMetafile", ['native', 'wasm-main', 'wasm-worker'], async ({ esbuild }) => {
  const result = await esbuild.analyzeMetafile({
    outputs: {
      'out.js': {
        bytes: 4096,
        inputs: {
          'in.js': {
            bytesInOutput: 1024,
          },
        },
      },
    },
  })
  asserts.assertStrictEquals(result, `
  out.js    4.0kb  100.0%
   └ in.js  1.0kb   25.0%
`)
})
