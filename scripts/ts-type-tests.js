const child_process = require('child_process')
const rimraf = require('rimraf')
const path = require('path')
const fs = require('fs')

const tsconfigJson = {
  compilerOptions: {
    module: 'CommonJS',
    strict: true,
  },
}

const tests = {
  emptyBuildRequire: `
    export {}
    import esbuild = require('esbuild')
    esbuild.buildSync({})
    esbuild.build({})
    esbuild.startService().then(service => service.build({}))
  `,
  emptyBuildImport: `
    import * as esbuild from 'esbuild'
    esbuild.buildSync({})
    esbuild.build({})
    esbuild.startService().then(service => service.build({}))
  `,
  emptyTransformRequire: `
    export {}
    import esbuild = require('esbuild')
    esbuild.transformSync('')
    esbuild.transformSync('', {})
    esbuild.transform('')
    esbuild.transform('', {})
    esbuild.startService().then(service => service.transform(''))
    esbuild.startService().then(service => service.transform('', {}))
  `,
  emptyTransformImport: `
    import * as esbuild from 'esbuild'
    esbuild.transformSync('')
    esbuild.transformSync('', {})
    esbuild.transform('')
    esbuild.transform('', {})
    esbuild.startService().then(service => service.transform(''))
    esbuild.startService().then(service => service.transform('', {}))
  `,
  allOptionsTransform: `
    export {}
    import {transform} from 'esbuild'
    transform('', {
      sourcemap: true,
      format: 'iife',
      globalName: '',
      target: 'esnext',
      minify: true,
      minifyWhitespace: true,
      minifyIdentifiers: true,
      minifySyntax: true,
      charset: 'utf8',
      jsxFactory: '',
      jsxFragment: '',
      define: { 'x': 'y' },
      pure: ['x'],
      avoidTDZ: true,
      color: true,
      logLevel: 'info',
      errorLimit: 0,
      tsconfigRaw: {
        compilerOptions: {
          jsxFactory: '',
          jsxFragmentFactory: '',
          useDefineForClassFields: true,
          importsNotUsedAsValues: 'preserve',
        },
      },
      sourcefile: '',
      loader: 'ts',
    }).then(result => {
      let code: string = result.code;
      let map: string = result.map;
      for (let msg of result.warnings) {
        let text: string = msg.text
        if (msg.location !== null) {
          let file: string = msg.location.file;
          let namespace: string = msg.location.namespace;
          let line: number = msg.location.line;
          let column: number = msg.location.column;
          let length: number = msg.location.length;
          let lineText: string = msg.location.lineText;
        }
      }
    })
  `,
  allOptionsBuild: `
    export {}
    import {build} from 'esbuild'
    build({
      sourcemap: true,
      format: 'iife',
      globalName: '',
      target: 'esnext',
      minify: true,
      minifyWhitespace: true,
      minifyIdentifiers: true,
      minifySyntax: true,
      charset: 'utf8',
      jsxFactory: '',
      jsxFragment: '',
      define: { 'x': 'y' },
      pure: ['x'],
      avoidTDZ: true,
      color: true,
      logLevel: 'info',
      errorLimit: 0,
      bundle: true,
      splitting: true,
      outfile: '',
      metafile: '',
      outdir: '',
      outbase: '',
      platform: 'node',
      external: ['x'],
      loader: { 'x': 'ts' },
      resolveExtensions: ['x'],
      mainFields: ['x'],
      write: true,
      tsconfig: 'x',
      outExtension: { 'x': 'y' },
      publicPath: 'x',
      inject: ['x'],
      entryPoints: ['x'],
      stdin: {
        contents: '',
        resolveDir: '',
        sourcefile: '',
        loader: 'ts',
      },
      plugins: [
        {
          name: 'x',
          setup(build) {
            build.onResolve({filter: /./}, () => undefined)
            build.onLoad({filter: /./}, () => undefined)
            build.onResolve({filter: /./, namespace: ''}, args => {
              let path: string = args.path;
              let importer: string = args.importer;
              let namespace: string = args.namespace;
              let resolveDir: string = args.resolveDir;
              if (Math.random()) return
              if (Math.random()) return {}
              return {
                pluginName: '',
                errors: [
                  {},
                  {text: ''},
                  {text: '', location: {}},
                  {location: {file: '', line: 0}},
                  {location: {file: '', namespace: '', line: 0, column: 0, length: 0, lineText: ''}},
                ],
                warnings: [
                  {},
                  {text: ''},
                  {text: '', location: {}},
                  {location: {file: '', line: 0}},
                  {location: {file: '', namespace: '', line: 0, column: 0, length: 0, lineText: ''}},
                ],
                path: '',
                external: true,
                namespace: '',
              }
            })
            build.onLoad({filter: /./, namespace: ''}, args => {
              let path: string = args.path;
              let namespace: string = args.namespace;
              if (Math.random()) return
              if (Math.random()) return {}
              return {
                pluginName: '',
                errors: [
                  {},
                  {text: ''},
                  {text: '', location: {}},
                  {location: {file: '', line: 0}},
                  {location: {file: '', namespace: '', line: 0, column: 0, length: 0, lineText: ''}},
                ],
                warnings: [
                  {},
                  {text: ''},
                  {text: '', location: {}},
                  {location: {file: '', line: 0}},
                  {location: {file: '', namespace: '', line: 0, column: 0, length: 0, lineText: ''}},
                ],
                contents: '',
                resolveDir: '',
                loader: 'ts',
              }
            })
          },
        }
      ],
    }).then(result => {
      if (result.outputFiles !== undefined) {
        for (let file of result.outputFiles) {
          let path: string = file.path
          let bytes: Uint8Array = file.contents
        }
      }
      for (let msg of result.warnings) {
        let text: string = msg.text
        if (msg.location !== null) {
          let file: string = msg.location.file;
          let namespace: string = msg.location.namespace;
          let line: number = msg.location.line;
          let column: number = msg.location.column;
          let length: number = msg.location.length;
          let lineText: string = msg.location.lineText;
        }
      }
    })
  `,
}

async function main() {
  let testDir = path.join(__dirname, '.ts-types-test')
  rimraf.sync(testDir, { disableGlob: true })
  fs.mkdirSync(testDir, { recursive: true })
  fs.writeFileSync(path.join(testDir, 'tsconfig.json'), JSON.stringify(tsconfigJson))

  const types = fs.readFileSync(path.join(__dirname, '..', 'lib', 'types.ts'), 'utf8')
  const esbuild_d_ts = path.join(testDir, 'node_modules', 'esbuild', 'index.d.ts')
  fs.mkdirSync(path.dirname(esbuild_d_ts), { recursive: true })
  fs.writeFileSync(esbuild_d_ts, `
    declare module 'esbuild' {
      ${types.replace(/export declare/g, 'export')}
    }
  `)

  let files = []
  for (const name in tests) {
    const input = path.join(testDir, name + '.ts')
    fs.writeFileSync(input, tests[name])
    files.push(input)
  }

  const tsc = path.join(__dirname, 'node_modules', '.bin', 'tsc')
  const allTestsPassed = await new Promise(resolve => {
    const child = child_process.spawn('node', [tsc, '--project', '.'], { cwd: testDir, stdio: 'inherit' })
    child.on('close', exitCode => resolve(exitCode === 0))
  })

  if (!allTestsPassed) {
    console.error(`❌ typescript type tests failed`)
    process.exit(1)
  } else {
    console.log(`✅ typescript type tests passed`)
    rimraf.sync(testDir, { disableGlob: true })
  }
}

main()
