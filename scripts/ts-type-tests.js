const { removeRecursiveSync } = require('./esbuild')
const child_process = require('child_process')
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
  `,
  emptyBuildImport: `
    import * as esbuild from 'esbuild'
    esbuild.buildSync({})
    esbuild.build({})
  `,
  emptyTransformRequire: `
    export {}
    import esbuild = require('esbuild')
    esbuild.transformSync('')
    esbuild.transformSync('', {})
    esbuild.transform('')
    esbuild.transform('', {})
  `,
  emptyTransformImport: `
    import * as esbuild from 'esbuild'
    esbuild.transformSync('')
    esbuild.transformSync('', {})
    esbuild.transform('')
    esbuild.transform('', {})
  `,

  mangleCache: `
    import * as esbuild from 'esbuild'
    esbuild.buildSync({ mangleCache: {} }).mangleCache['x']
    esbuild.build({ mangleCache: {} })
      .then(result => result.mangleCache['x'])
  `,
  writeFalseOutputFiles: `
    import * as esbuild from 'esbuild'
    esbuild.buildSync({ write: false }).outputFiles[0]
    esbuild.build({ write: false })
      .then(result => result.outputFiles[0])
  `,
  metafileTrue: `
    import {build, buildSync, analyzeMetafile} from 'esbuild';
    analyzeMetafile(buildSync({ metafile: true }).metafile)
    build({ metafile: true })
      .then(result => analyzeMetafile(result.metafile))
  `,

  contextMangleCache: `
    import * as esbuild from 'esbuild'
    esbuild.context({ mangleCache: {} })
      .then(context => context.rebuild())
      .then(result => result.mangleCache['x'])
  `,
  contextWriteFalseOutputFiles: `
    import * as esbuild from 'esbuild'
    esbuild.context({ write: false })
      .then(context => context.rebuild())
      .then(result => result.outputFiles[0])
  `,
  contextMetafileTrue: `
    import {context, analyzeMetafile} from 'esbuild';
    context({ metafile: true })
      .then(context => context.rebuild())
      .then(result => analyzeMetafile(result.metafile))
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
      color: true,
      logLevel: 'info',
      logLimit: 0,
      tsconfigRaw: {
        compilerOptions: {
          jsxFactory: '',
          jsxFragmentFactory: '',
          useDefineForClassFields: true,
          importsNotUsedAsValues: 'preserve',
          preserveValueImports: true,
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
      color: true,
      logLevel: 'info',
      logLimit: 0,
      bundle: true,
      splitting: true,
      outfile: '',
      metafile: true,
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
            build.onStart(() => {})
            build.onStart(async () => {})
            build.onStart(() => ({ warnings: [{text: '', location: {file: '', line: 0}}] }))
            build.onStart(async () => ({ warnings: [{text: '', location: {file: '', line: 0}}] }))
            build.onEnd(result => {})
            build.onEnd(async result => {})
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
                suffix: '',
              }
            })
            build.onLoad({filter: /./, namespace: ''}, args => {
              let path: string = args.path;
              let namespace: string = args.namespace;
              let suffix: string = args.suffix;
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
  removeRecursiveSync(testDir)
  fs.mkdirSync(testDir, { recursive: true })
  fs.writeFileSync(path.join(testDir, 'tsconfig.json'), JSON.stringify(tsconfigJson))

  const types = fs.readFileSync(path.join(__dirname, '..', 'lib', 'shared', 'types.ts'), 'utf8')
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

  const tsc = path.join(__dirname, 'node_modules', 'typescript', 'lib', 'tsc.js')
  const allTestsPassed = await new Promise(resolve => {
    const child = child_process.spawn('node', [tsc, '--project', '.'], { cwd: testDir, stdio: 'inherit' })
    child.on('close', exitCode => resolve(exitCode === 0))
  })

  if (!allTestsPassed) {
    console.error(`❌ typescript type tests failed`)
    process.exit(1)
  } else {
    console.log(`✅ typescript type tests passed`)
    removeRecursiveSync(testDir)
  }
}

main().catch(e => {
  setTimeout(() => {
    throw e
  })
})
