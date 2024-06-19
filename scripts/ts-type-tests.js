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

const testsWithoutErrors = {
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
    esbuild.transformSync('', { mangleCache: {} }).mangleCache['x']
    esbuild.transform('', { mangleCache: {} })
      .then(result => result.mangleCache['x'])
  `,
  legalCommentsExternal: `
    import * as esbuild from 'esbuild'
    esbuild.transformSync('', { legalComments: 'external' }).legalComments.length
    esbuild.transform('', { legalComments: 'external' })
      .then(result => result.legalComments.length)
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

const testsWithErrors = {
  badBuildRequire_invalidOption: `
    export {}
    import esbuild = require('esbuild')
    esbuild.build({ invalidOption: true })
  `,
  badBuildSyncRequire_invalidOption: `
    export {}
    import esbuild = require('esbuild')
    esbuild.buildSync({ invalidOption: true })
  `,
  badBuildImport_invalidOption: `
    import * as esbuild from 'esbuild'
    esbuild.build({ invalidOption: true })
  `,
  badBuildSyncImport_invalidOption: `
    import * as esbuild from 'esbuild'
    esbuild.build({ invalidOption: true })
  `,
  badTransformRequire_invalidOption: `
    export {}
    import esbuild = require('esbuild')
    esbuild.transform('', { invalidOption: true })
  `,
  badTransformSyncRequire_invalidOption: `
    export {}
    import esbuild = require('esbuild')
    esbuild.transformSync('', { invalidOption: true })
  `,
  badTransformImport_invalidOption: `
    import * as esbuild from 'esbuild'
    esbuild.transform('', { invalidOption: true })
  `,
  badTransformSyncImport_invalidOption: `
    import * as esbuild from 'esbuild'
    esbuild.transformSync('', { invalidOption: true })
  `,

  // mangleCache
  mangleCacheBuild_undefined: `
    import * as esbuild from 'esbuild'
    esbuild.build({}).then(result => result.mangleCache['x'])
  `,
  mangleCacheBuildSync_undefined: `
    import * as esbuild from 'esbuild'
    esbuild.buildSync({}).mangleCache['x']
  `,
  mangleCacheTransform_undefined: `
    import * as esbuild from 'esbuild'
    esbuild.transform('', {}).then(result => result.mangleCache['x'])
  `,
  mangleCacheTransformSync_undefined: `
    import * as esbuild from 'esbuild'
    esbuild.transformSync('', {}).mangleCache['x']
  `,

  // legalComments
  legalCommentsTransform_undefined: `
    import * as esbuild from 'esbuild'
    esbuild.transform('', { legalComments: 'eof' }).then(result => result.legalComments.length)
  `,
  legalCommentsTransformSync_undefined: `
    import * as esbuild from 'esbuild'
    esbuild.transformSync('', { legalComments: 'eof' }).legalComments.length
  `,

  // outputFiles
  outputFilesBuild_undefined: `
    import * as esbuild from 'esbuild'
    esbuild.build({}).then(result => result.outputFiles[0])
  `,
  outputFilesBuildSync_undefined: `
    import * as esbuild from 'esbuild'
    esbuild.buildSync({}).outputFiles[0]
  `,
  outputFilesWriteTrueBuild_undefined: `
    import * as esbuild from 'esbuild'
    esbuild.build({ write: true }).then(result => result.outputFiles[0])
  `,
  outputFilesWriteTrueBuildSync_undefined: `
    import * as esbuild from 'esbuild'
    esbuild.buildSync({ write: true }).outputFiles[0]
  `,

  // metafile
  metafileBuild_undefined: `
    import * as esbuild from 'esbuild'
    esbuild.build({}).then(result => esbuild.analyzeMetafile(result.metafile))
  `,
  metafileBuildSync_undefined: `
    import * as esbuild from 'esbuild'
    esbuild.analyzeMetafile(esbuild.buildSync({}).metafile)
  `,
  metafileFalseBuild_undefined: `
    import * as esbuild from 'esbuild'
    esbuild.build({ metafile: false }).then(result => esbuild.analyzeMetafile(result.metafile))
  `,
  metafileFalseBuildSync_undefined: `
    import * as esbuild from 'esbuild'
    esbuild.analyzeMetafile(esbuild.buildSync({ metafile: false }).metafile)
  `,
}

async function main() {
  let testDir = path.join(__dirname, '.ts-types-test')
  removeRecursiveSync(testDir)
  fs.mkdirSync(testDir, { recursive: true })

  const types = fs.readFileSync(path.join(__dirname, '..', 'lib', 'shared', 'types.ts'), 'utf8')
  const tsc = path.join(__dirname, 'node_modules', 'typescript', 'lib', 'tsc.js')
  const esbuild_d_ts = path.join(testDir, 'node_modules', 'esbuild', 'index.d.ts')
  fs.mkdirSync(path.dirname(esbuild_d_ts), { recursive: true })
  fs.writeFileSync(esbuild_d_ts, `
    declare module 'esbuild' {
      ${types.replace(/export declare/g, 'export')}
    }
  `)
  let allTestsPassed = true

  // Check tests without errors
  if (allTestsPassed) {
    const dir = path.join(testDir, 'without-errors')
    fs.mkdirSync(dir, { recursive: true })
    fs.writeFileSync(path.join(dir, 'tsconfig.json'), JSON.stringify(tsconfigJson))
    for (const name in testsWithoutErrors) {
      const input = path.join(dir, name + '.ts')
      fs.writeFileSync(input, testsWithoutErrors[name])
    }
    allTestsPassed &&= await new Promise(resolve => {
      const child = child_process.spawn('node', [tsc, '--project', '.'], { cwd: dir, stdio: 'inherit' })
      child.on('close', exitCode => resolve(exitCode === 0))
    })
  }

  // Check tests with errors
  if (allTestsPassed) {
    const dir = path.join(testDir, 'with-errors')
    fs.mkdirSync(dir, { recursive: true })
    fs.writeFileSync(path.join(dir, 'tsconfig.json'), JSON.stringify(tsconfigJson))
    for (const name in testsWithErrors) {
      const input = path.join(dir, name.split('_')[0] + '.ts')
      fs.writeFileSync(input, testsWithErrors[name])
    }
    try {
      child_process.execFileSync('node', [tsc, '--project', '.'], { cwd: dir })
      throw new Error('Expected an error to be generated')
    } catch (err) {
      const stdout = err.stdout.toString()
      const lines = stdout.split('\n')
      next: for (const name in testsWithErrors) {
        const fileName = name.split('_')[0]
        const expectedText = name.split('_')[1]
        for (const line of lines) {
          if (line.includes(fileName)) {
            if (line.includes(expectedText)) {
              console.log(`\x1B[32mSUCCESS:\x1B[0m ${line}`)
            } else {
              console.log(`\x1B[31mFAILURE: ${line}\x1B[0m`)
              allTestsPassed = false
            }
            continue next
          }
        }
        console.log(`\x1B[31mFAILURE:\x1B[0m ${name}: Could not find expected error in output from "tsc":`)
        process.stdout.write(stdout)
        allTestsPassed = false
        break next
      }
    }
  }

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
