const { installForTests, removeRecursiveSync, writeFileAtomic } = require('./esbuild')
const child_process = require('child_process')
const assert = require('assert')
const path = require('path')
const http = require('http')
const https = require('https')
const events = require('events')
const fs = require('fs')
const vm = require('vm')

const readFileAsync = fs.promises.readFile
const writeFileAsync = fs.promises.writeFile
const mkdirAsync = fs.promises.mkdir

const repoDir = path.dirname(__dirname)
const rootTestDir = path.join(repoDir, 'scripts', '.js-api-tests')
const errorIcon = process.platform !== 'win32' ? '✘' : 'X'

let buildTests = {
  async errorIfEntryPointsNotArray({ esbuild }) {
    try {
      await esbuild.build({
        entryPoints: 'this is not an array',
        logLevel: 'silent',
      })
      throw new Error('Expected build failure');
    } catch (e) {
      if (!e.errors || !e.errors[0] || e.errors[0].text !== '"entryPoints" must be an array or an object') {
        throw e;
      }
    }
  },

  async errorIfBadWorkingDirectory({ esbuild }) {
    try {
      await esbuild.build({
        absWorkingDir: 'what is this? certainly not an absolute path',
        logLevel: 'silent',
        write: false,
      })
      throw new Error('Expected build failure');
    } catch (e) {
      if (e.message !== 'Build failed with 1 error:\nerror: The working directory ' +
        '"what is this? certainly not an absolute path" is not an absolute path') {
        throw e;
      }
    }
  },

  async errorIfGlob({ esbuild }) {
    try {
      await esbuild.build({
        entryPoints: ['./src/*.js'],
        logLevel: 'silent',
        write: false,
      })
      throw new Error('Expected build failure');
    } catch (e) {
      if (!e.errors || !e.errors[0] || e.errors[0].text !== 'Could not resolve "./src/*.js"' ||
        e.errors[0].notes[0].text !== 'It looks like you are trying to use glob syntax (i.e. "*") with esbuild. ' +
        'This syntax is typically handled by your shell, and isn\'t handled by esbuild itself. ' +
        'You must expand glob syntax first before passing your paths to esbuild.') {
        throw e;
      }
    }
  },

  // Verify that it's possible to disable a loader by setting it to "default".
  // In particular, verify that it's possible to disable the special loader ""
  // for extensionless files.
  async errorIfExtensionlessLoaderIsDisabled({ esbuild, testDir }) {
    let entry = path.join(testDir, 'entry.js');
    let what = path.join(testDir, 'what');
    await writeFileAsync(entry, 'import "./what"')
    await writeFileAsync(what, 'foo()')
    await esbuild.build({
      entryPoints: [entry],
      bundle: true,
      write: false,
    })
    try {
      await esbuild.build({
        entryPoints: [entry],
        bundle: true,
        write: false,
        logLevel: 'silent',
        loader: { '': 'default' },
      })
      throw new Error('Expected build failure');
    } catch (e) {
      const relPath = path.relative(process.cwd(), what).split(path.sep).join('/')
      if (!e.errors || !e.errors[0] || e.errors[0].text !== 'Do not know how to load path: ' + relPath) {
        throw e;
      }
    }
  },

  async mangleCacheBuild({ esbuild }) {
    var result = await esbuild.build({
      stdin: {
        contents: `x = { x_: 0, y_: 1, z_: 2 }`,
      },
      mangleProps: /_/,
      mangleCache: { x_: 'FIXED', z_: false },
      write: false,
    })
    assert.strictEqual(result.outputFiles[0].text, 'x = { FIXED: 0, a: 1, z_: 2 };\n')
    assert.deepStrictEqual(result.mangleCache, { x_: 'FIXED', y_: 'a', z_: false })
  },

  async windowsBackslashPathTest({ esbuild, testDir }) {
    let entry = path.join(testDir, 'entry.js');
    let nested = path.join(testDir, 'nested.js');
    let outfile = path.join(testDir, 'out.js');

    // On Windows, backslash and forward slash should be treated the same
    fs.writeFileSync(entry, `
      import ${JSON.stringify(nested)}
      import ${JSON.stringify(nested.split(path.sep).join('/'))}
    `);
    fs.writeFileSync(nested, `console.log('once')`);

    const result = await esbuild.build({
      entryPoints: [entry],
      outfile,
      bundle: true,
      write: false,
      minify: true,
      format: 'esm',
    })

    assert.strictEqual(result.outputFiles[0].text, 'console.log("once");\n')
  },

  async workingDirTest({ esbuild, testDir }) {
    let aDir = path.join(testDir, 'a');
    let bDir = path.join(testDir, 'b');
    let aFile = path.join(aDir, 'a-in.js');
    let bFile = path.join(bDir, 'b-in.js');
    let aOut = path.join(aDir, 'a-out.js');
    let bOut = path.join(bDir, 'b-out.js');
    fs.mkdirSync(aDir, { recursive: true });
    fs.mkdirSync(bDir, { recursive: true });
    fs.writeFileSync(aFile, 'exports.x = true');
    fs.writeFileSync(bFile, 'exports.y = true');

    await Promise.all([
      esbuild.build({
        entryPoints: [path.basename(aFile)],
        outfile: path.basename(aOut),
        absWorkingDir: aDir,
      }),
      esbuild.build({
        entryPoints: [path.basename(bFile)],
        outfile: path.basename(bOut),
        absWorkingDir: bDir,
      }),
    ]);

    assert.strictEqual(require(aOut).x, true)
    assert.strictEqual(require(bOut).y, true)
  },

  async aliasValidity({ esbuild }) {
    const valid = async alias => {
      const result = await esbuild.build({
        stdin: { contents: 'import ' + JSON.stringify(alias) },
        bundle: true,
        alias: { [alias]: 'foo' },
        external: ['foo'],
        format: 'esm',
        write: false,
      })
      assert.strictEqual(result.outputFiles[0].text, '// <stdin>\nimport "foo";\n')
    }

    const invalid = async alias => {
      try {
        await esbuild.build({
          bundle: true,
          alias: { [alias]: 'foo' },
          logLevel: 'silent',
        })
      } catch {
        return
      }
      throw new Error('Expected an error for alias: ' + alias)
    }

    await Promise.all([
      valid('foo'),
      valid('foo/bar'),
      valid('@foo'),
      valid('@foo/bar'),
      valid('@foo/bar/baz'),

      invalid('./foo'),
      invalid('../foo'),
      invalid('/foo'),
      invalid('C:\\foo'),
      invalid('.foo'),
      invalid('foo/'),
      invalid('@foo/'),
      invalid('foo/../bar'),
    ])
  },

  async pathResolverEACCS({ esbuild, testDir }) {
    let outerDir = path.join(testDir, 'outer');
    let innerDir = path.join(outerDir, 'inner');
    let pkgDir = path.join(testDir, 'node_modules', 'pkg');
    let entry = path.join(innerDir, 'entry.js');
    let sibling = path.join(innerDir, 'sibling.js');
    let index = path.join(pkgDir, 'index.js');
    let outfile = path.join(innerDir, 'out.js');
    fs.mkdirSync(pkgDir, { recursive: true });
    fs.mkdirSync(innerDir, { recursive: true });
    fs.writeFileSync(entry, `
      import a from "./sibling.js"
      import b from "pkg"
      export default {a, b}
    `);
    fs.writeFileSync(sibling, `export default 'sibling'`);
    fs.writeFileSync(index, `export default 'pkg'`);
    fs.chmodSync(outerDir, 0o111);

    try {
      await esbuild.build({
        entryPoints: [entry],
        bundle: true,
        outfile,
        format: 'cjs',
      });

      const result = require(outfile);
      assert.deepStrictEqual(result.default, { a: 'sibling', b: 'pkg' });
    }

    finally {
      // Restore permission when the test ends so test cleanup works
      fs.chmodSync(outerDir, 0o755);
    }
  },

  async nodePathsTest({ esbuild, testDir }) {
    let srcDir = path.join(testDir, 'src');
    let pkgDir = path.join(testDir, 'pkg');
    let outfile = path.join(testDir, 'out.js');
    let entry = path.join(srcDir, 'entry.js');
    let other = path.join(pkgDir, 'other.js');
    fs.mkdirSync(srcDir, { recursive: true });
    fs.mkdirSync(pkgDir, { recursive: true });
    fs.writeFileSync(entry, `export {x} from 'other'`);
    fs.writeFileSync(other, `export let x = 123`);

    await esbuild.build({
      entryPoints: [entry],
      outfile,
      bundle: true,
      nodePaths: [pkgDir],
      format: 'cjs',
    })

    assert.strictEqual(require(outfile).x, 123)
  },

  // A local "node_modules" path should be preferred over "NODE_PATH".
  // See: https://github.com/evanw/esbuild/issues/1117
  async nodePathsLocalPreferredTestIssue1117({ esbuild, testDir }) {
    let srcDir = path.join(testDir, 'src');
    let srcOtherDir = path.join(testDir, 'src', 'node_modules', 'other');
    let pkgDir = path.join(testDir, 'pkg');
    let outfile = path.join(testDir, 'out.js');
    let entry = path.join(srcDir, 'entry.js');
    let srcOther = path.join(srcOtherDir, 'index.js');
    let pkgOther = path.join(pkgDir, 'other.js');
    fs.mkdirSync(srcDir, { recursive: true });
    fs.mkdirSync(srcOtherDir, { recursive: true });
    fs.mkdirSync(pkgDir, { recursive: true });
    fs.writeFileSync(entry, `export {x} from 'other'`);
    fs.writeFileSync(srcOther, `export let x = 234`);
    fs.writeFileSync(pkgOther, `export let x = 123`);

    await esbuild.build({
      entryPoints: [entry],
      outfile,
      bundle: true,
      nodePaths: [pkgDir],
      format: 'cjs',
    })

    assert.strictEqual(require(outfile).x, 234)
  },

  async es6_to_cjs({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, 'export default 123')
    const value = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outfile: output,
      format: 'cjs',
    })
    assert.strictEqual(value.outputFiles, void 0)
    const result = require(output)
    assert.strictEqual(result.default, 123)
    assert.strictEqual(result.__esModule, true)
  },

  // Test recursive directory creation
  async recursiveMkdir({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'a/b/c/d/out.js')
    await writeFileAsync(input, 'export default 123')
    await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outfile: output,
      format: 'cjs',
    })
    const result = require(output)
    assert.strictEqual(result.default, 123)
    assert.strictEqual(result.__esModule, true)
  },

  async outExtensionJS({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'in.mjs')
    await writeFileAsync(input, 'console.log("test")')
    await esbuild.build({
      entryPoints: [input],
      outdir: testDir,
      outExtension: { '.js': '.mjs' },
    })
    const mjs = await readFileAsync(output, 'utf8')
    assert.strictEqual(mjs, 'console.log("test");\n')
  },

  async outExtensionCSS({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.css')
    const output = path.join(testDir, 'in.notcss')
    await writeFileAsync(input, 'body {}')
    await esbuild.build({
      entryPoints: [input],
      outdir: testDir,
      outExtension: { '.css': '.notcss' },
    })
    const notcss = await readFileAsync(output, 'utf8')
    assert.strictEqual(notcss, 'body {\n}\n')
  },

  async sourceMapTrue({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    const content = 'exports.foo = 123'
    await writeFileAsync(input, content)
    await esbuild.build({
      entryPoints: [input],
      outfile: output,
      sourcemap: true,
    })
    const result = require(output)
    assert.strictEqual(result.foo, 123)
    const outputFile = await readFileAsync(output, 'utf8')
    const match = /\/\/# sourceMappingURL=(.*)/.exec(outputFile)
    assert.strictEqual(match[1], 'out.js.map')
    const resultMap = await readFileAsync(output + '.map', 'utf8')
    const json = JSON.parse(resultMap)
    assert.strictEqual(json.version, 3)
    assert.strictEqual(json.sources[0], path.basename(input))
    assert.strictEqual(json.sourcesContent[0], content)
  },

  async sourceMapLinked({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    const content = 'exports.foo = 123'
    await writeFileAsync(input, content)
    await esbuild.build({
      entryPoints: [input],
      outfile: output,
      sourcemap: 'linked',
    })
    const result = require(output)
    assert.strictEqual(result.foo, 123)
    const outputFile = await readFileAsync(output, 'utf8')
    const match = /\/\/# sourceMappingURL=(.*)/.exec(outputFile)
    assert.strictEqual(match[1], 'out.js.map')
    const resultMap = await readFileAsync(output + '.map', 'utf8')
    const json = JSON.parse(resultMap)
    assert.strictEqual(json.version, 3)
    assert.strictEqual(json.sources[0], path.basename(input))
    assert.strictEqual(json.sourcesContent[0], content)
  },

  async sourceMapExternal({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    const content = 'exports.foo = 123'
    await writeFileAsync(input, content)
    await esbuild.build({
      entryPoints: [input],
      outfile: output,
      sourcemap: 'external',
    })
    const result = require(output)
    assert.strictEqual(result.foo, 123)
    const outputFile = await readFileAsync(output, 'utf8')
    const match = /\/\/# sourceMappingURL=(.*)/.exec(outputFile)
    assert.strictEqual(match, null)
    const resultMap = await readFileAsync(output + '.map', 'utf8')
    const json = JSON.parse(resultMap)
    assert.strictEqual(json.version, 3)
    assert.strictEqual(json.sources[0], path.basename(input))
    assert.strictEqual(json.sourcesContent[0], content)
  },

  async sourceMapInline({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    const content = 'exports.foo = 123'
    await writeFileAsync(input, content)
    await esbuild.build({
      entryPoints: [input],
      outfile: output,
      sourcemap: 'inline',
    })
    const result = require(output)
    assert.strictEqual(result.foo, 123)
    const outputFile = await readFileAsync(output, 'utf8')
    const match = /\/\/# sourceMappingURL=data:application\/json;base64,(.*)/.exec(outputFile)
    const json = JSON.parse(Buffer.from(match[1], 'base64').toString())
    assert.strictEqual(json.version, 3)
    assert.strictEqual(json.sources[0], path.basename(input))
    assert.strictEqual(json.sourcesContent[0], content)
  },

  async sourceMapBoth({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    const content = 'exports.foo = 123'
    await writeFileAsync(input, content)
    await esbuild.build({
      entryPoints: [input],
      outfile: output,
      sourcemap: 'both',
    })
    const result = require(output)
    assert.strictEqual(result.foo, 123)
    const outputFile = await readFileAsync(output, 'utf8')
    const match = /\/\/# sourceMappingURL=data:application\/json;base64,(.*)/.exec(outputFile)
    const json = JSON.parse(Buffer.from(match[1], 'base64').toString())
    assert.strictEqual(json.version, 3)
    assert.strictEqual(json.sources[0], path.basename(input))
    assert.strictEqual(json.sourcesContent[0], content)
    const outputFileMap = await readFileAsync(output + '.map', 'utf8')
    assert.strictEqual(Buffer.from(match[1], 'base64').toString(), outputFileMap)
  },

  async sourceMapIncludeSourcesContent({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    const content = 'exports.foo = 123'
    await writeFileAsync(input, content)
    await esbuild.build({
      entryPoints: [input],
      outfile: output,
      sourcemap: true,
      sourcesContent: true,
    })
    const result = require(output)
    assert.strictEqual(result.foo, 123)
    const outputFile = await readFileAsync(output, 'utf8')
    const match = /\/\/# sourceMappingURL=(.*)/.exec(outputFile)
    assert.strictEqual(match[1], 'out.js.map')
    const resultMap = await readFileAsync(output + '.map', 'utf8')
    const json = JSON.parse(resultMap)
    assert.strictEqual(json.version, 3)
    assert.strictEqual(json.sources[0], path.basename(input))
    assert.strictEqual(json.sourcesContent[0], content)
  },

  async sourceMapExcludeSourcesContent({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    const content = 'exports.foo = 123'
    await writeFileAsync(input, content)
    await esbuild.build({
      entryPoints: [input],
      outfile: output,
      sourcemap: true,
      sourcesContent: false,
    })
    const result = require(output)
    assert.strictEqual(result.foo, 123)
    const outputFile = await readFileAsync(output, 'utf8')
    const match = /\/\/# sourceMappingURL=(.*)/.exec(outputFile)
    assert.strictEqual(match[1], 'out.js.map')
    const resultMap = await readFileAsync(output + '.map', 'utf8')
    const json = JSON.parse(resultMap)
    assert.strictEqual(json.version, 3)
    assert.strictEqual(json.sources[0], path.basename(input))
    assert.strictEqual(json.sourcesContent, void 0)
  },

  async sourceMapSourceRoot({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    const content = 'exports.foo = 123'
    await writeFileAsync(input, content)
    await esbuild.build({
      entryPoints: [input],
      outfile: output,
      sourcemap: true,
      sourceRoot: 'https://example.com/'
    })
    const result = require(output)
    assert.strictEqual(result.foo, 123)
    const outputFile = await readFileAsync(output, 'utf8')
    const match = /\/\/# sourceMappingURL=(.*)/.exec(outputFile)
    assert.strictEqual(match[1], 'out.js.map')
    const resultMap = await readFileAsync(output + '.map', 'utf8')
    const json = JSON.parse(resultMap)
    assert.strictEqual(json.version, 3)
    assert.strictEqual(json.sources[0], path.basename(input))
    assert.strictEqual(json.sourceRoot, 'https://example.com/')
  },

  async sourceMapWithDisabledFile({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const disabled = path.join(testDir, 'disabled.js')
    const packageJSON = path.join(testDir, 'package.json')
    const output = path.join(testDir, 'out.js')
    const content = 'exports.foo = require("./disabled")'
    await writeFileAsync(input, content)
    await writeFileAsync(disabled, 'module.exports = 123')
    await writeFileAsync(packageJSON, `{"browser": {"./disabled.js": false}}`)
    await esbuild.build({
      entryPoints: [input],
      outfile: output,
      sourcemap: true,
      bundle: true,
    })
    const result = require(output)
    assert.strictEqual(result.foo, void 0)
    const resultMap = await readFileAsync(output + '.map', 'utf8')
    const json = JSON.parse(resultMap)
    assert.strictEqual(json.version, 3)
    assert.strictEqual(json.sources.length, 1)
    assert.strictEqual(json.sources[0], path.basename(input))
    assert.strictEqual(json.sourcesContent[0], content)
  },

  async sourceMapWithEmptyFile({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const empty = path.join(testDir, 'file.empty')
    const output = path.join(testDir, 'out.js')
    const content = 'exports.foo = require("./file.empty")'
    await writeFileAsync(input, content)
    await writeFileAsync(empty, 'module.exports = 123')
    await esbuild.build({
      entryPoints: [input],
      outfile: output,
      sourcemap: true,
      bundle: true,
      loader: { '.empty': 'empty' },
    })
    const result = require(output)
    assert.strictEqual(result.foo, void 0)
    const resultMap = await readFileAsync(output + '.map', 'utf8')
    const json = JSON.parse(resultMap)
    assert.strictEqual(json.version, 3)
    assert.strictEqual(json.sources.length, 1)
    assert.strictEqual(json.sources[0], path.basename(input))
    assert.strictEqual(json.sourcesContent[0], content)
  },

  async sourceMapWithDisabledModule({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const disabled = path.join(testDir, 'node_modules', 'disabled', 'index.js')
    const packageJSON = path.join(testDir, 'package.json')
    const output = path.join(testDir, 'out.js')
    const content = 'exports.foo = require("disabled")'
    await mkdirAsync(path.dirname(disabled), { recursive: true })
    await writeFileAsync(input, content)
    await writeFileAsync(disabled, 'module.exports = 123')
    await writeFileAsync(packageJSON, `{"browser": {"disabled": false}}`)
    await esbuild.build({
      entryPoints: [input],
      outfile: output,
      sourcemap: true,
      bundle: true,
    })
    const result = require(output)
    assert.strictEqual(result.foo, void 0)
    const resultMap = await readFileAsync(output + '.map', 'utf8')
    const json = JSON.parse(resultMap)
    assert.strictEqual(json.version, 3)
    assert.strictEqual(json.sources.length, 1)
    assert.strictEqual(json.sources[0], path.basename(input))
    assert.strictEqual(json.sourcesContent[0], content)
  },

  async resolveExtensionOrder({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js');
    const inputBare = path.join(testDir, 'module.js')
    const inputSomething = path.join(testDir, 'module.something.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, 'exports.result = require("./module").foo')
    await writeFileAsync(inputBare, 'exports.foo = 321')
    await writeFileAsync(inputSomething, 'exports.foo = 123')
    await esbuild.build({
      entryPoints: [input],
      outfile: output,
      format: 'cjs',
      bundle: true,
      resolveExtensions: ['.something.js', '.js'],
    })
    assert.strictEqual(require(output).result, 123)
  },

  async defineObject({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js');
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, 'export default {abc, xyz}')
    await esbuild.build({
      entryPoints: [input],
      outfile: output,
      format: 'cjs',
      bundle: true,
      define: {
        abc: '["a", "b", "c"]',
        xyz: '{"x": 1, "y": 2, "z": 3}',
      },
    })
    assert.deepStrictEqual(require(output).default, {
      abc: ['a', 'b', 'c'],
      xyz: { x: 1, y: 2, z: 3 },
    })
  },

  async inject({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js');
    const inject = path.join(testDir, 'inject.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, 'export default foo * 10 + 4')
    await writeFileAsync(inject, 'export let foo = 123')
    await esbuild.build({
      entryPoints: [input],
      outfile: output,
      format: 'cjs',
      bundle: true,
      inject: [inject],
    })
    assert.strictEqual(require(output).default, 1234)
  },

  async minifyWithoutInject({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js');
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, 'function exampleFn() { return Math.min(4, 3) }')

    const result = await esbuild.build({
      entryPoints: [input],
      outfile: output,
      write: false,
      minify: true,
    })
    assert.strictEqual(3, new Function(result.outputFiles[0].text + '\nreturn exampleFn()')())
  },

  // This should be the same as "minifyWithoutInject" above
  async minifyWithInject({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js');
    const inject = path.join(testDir, 'inject.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, 'function exampleFn() { return Math.min(4, 3) }')
    await writeFileAsync(inject, 'let min = Math.min; export { min as "Math.min" }')

    const result = await esbuild.build({
      entryPoints: [input],
      outfile: output,
      inject: [inject],
      write: false,
      minify: true,
    })
    assert.strictEqual(3, new Function(result.outputFiles[0].text + '\nreturn exampleFn()')())
  },

  async mainFields({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    const mainFieldsDir = path.join(testDir, 'node_modules', 'main-fields-test')
    const mainFieldsA = path.join(mainFieldsDir, 'a.js')
    const mainFieldsB = path.join(mainFieldsDir, 'b.js')
    const mainFieldsPackage = path.join(mainFieldsDir, 'package.json')
    await mkdirAsync(mainFieldsDir, { recursive: true })
    await writeFileAsync(input, 'export * from "main-fields-test"')
    await writeFileAsync(mainFieldsA, 'export let foo = "a"')
    await writeFileAsync(mainFieldsB, 'export let foo = "b"')
    await writeFileAsync(mainFieldsPackage, '{ "a": "./a.js", "b": "./b.js", "c": "./c.js" }')
    await esbuild.build({
      entryPoints: [input],
      outfile: output,
      bundle: true,
      format: 'cjs',
      mainFields: ['c', 'b', 'a'],
    })
    const result = require(output)
    assert.strictEqual(result.foo, 'b')
  },

  async requireAbsolutePath({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const dependency = path.join(testDir, 'dep.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `import value from ${JSON.stringify(dependency)}; export default value`)
    await writeFileAsync(dependency, `export default 123`)
    const value = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outfile: output,
      format: 'cjs',
    })
    assert.strictEqual(value.outputFiles, void 0)
    const result = require(output)
    assert.strictEqual(result.default, 123)
    assert.strictEqual(result.__esModule, true)
  },

  async buildLoaderStdinBase64({ esbuild }) {
    // UTF-16
    var result = await esbuild.build({
      stdin: {
        contents: `\xFF`,
        loader: 'base64',
      },
      write: false,
    })
    assert.strictEqual(result.outputFiles[0].text, `module.exports = "w78=";\n`)

    // Binary
    var result = await esbuild.build({
      stdin: {
        contents: new Uint8Array([0xFF]),
        loader: 'base64',
      },
      write: false,
    })
    assert.strictEqual(result.outputFiles[0].text, `module.exports = "/w==";\n`)
  },

  async fileLoader({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const data = path.join(testDir, 'data.bin')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `export {default as value} from ${JSON.stringify(data)}`)
    await writeFileAsync(data, `stuff`)
    const value = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outfile: output,
      format: 'cjs',
      loader: { '.bin': 'file' },
    })
    assert.strictEqual(value.outputFiles, void 0)
    const result = require(output)
    assert.strictEqual(result.value, './data-BYATPJRB.bin')
    assert.strictEqual(result.__esModule, true)
  },

  async fileLoaderEntryHash({ esbuild, testDir }) {
    const input = path.join(testDir, 'index.js')
    const data = path.join(testDir, 'data.bin')
    const outdir = path.join(testDir, 'out')
    await writeFileAsync(input, `export {default as value} from ${JSON.stringify(data)}`)
    await writeFileAsync(data, `stuff`)
    const result1 = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outdir,
      format: 'cjs',
      loader: { '.bin': 'file' },
      entryNames: '[name]-[hash]',
      write: false,
    })
    await writeFileAsync(data, `more stuff`)
    const result2 = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outdir,
      format: 'cjs',
      loader: { '.bin': 'file' },
      entryNames: '[name]-[hash]',
      write: false,
    })
    assert.strictEqual(result1.outputFiles.length, 2)
    assert.strictEqual(result2.outputFiles.length, 2)

    // Make sure each path is unique. This tests for a bug where the hash in
    // the output filename corresponding to the "index.js" entry point wasn't
    // including the filename for the "file" loader.
    assert.strictEqual(new Set(result1.outputFiles.concat(result2.outputFiles).map(x => x.path)).size, 4)
  },

  async fileLoaderEntryHashNoChange({ esbuild, testDir }) {
    const input = path.join(testDir, 'index.js')
    const data = path.join(testDir, 'data.bin')
    const outdir = path.join(testDir, 'out')
    await writeFileAsync(input, `export {default as value} from ${JSON.stringify(data)}`)
    await writeFileAsync(data, `stuff`)
    const result1 = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outdir,
      format: 'cjs',
      loader: { '.bin': 'file' },
      entryNames: '[name]-[hash]',
      assetNames: '[name]',
      write: false,
    })
    await writeFileAsync(data, `more stuff`)
    const result2 = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outdir,
      format: 'cjs',
      loader: { '.bin': 'file' },
      entryNames: '[name]-[hash]',
      assetNames: '[name]',
      write: false,
    })
    assert.strictEqual(result1.outputFiles.length, 2)
    assert.strictEqual(result2.outputFiles.length, 2)

    // The paths should be the same. The hash augmentation from the previous
    // test should only be checking for a file name difference, not a difference
    // in content, because the JS output file only contains the file name for
    // something using the "file" loader.
    assert.strictEqual(new Set(result1.outputFiles.concat(result2.outputFiles).map(x => x.path)).size, 2)
  },

  async splittingPublicPath({ esbuild, testDir }) {
    const input1 = path.join(testDir, 'a', 'in1.js')
    const input2 = path.join(testDir, 'b', 'in2.js')
    const shared = path.join(testDir, 'c', 'shared.js')
    const outdir = path.join(testDir, 'out')
    await mkdirAsync(path.dirname(input1), { recursive: true })
    await mkdirAsync(path.dirname(input2), { recursive: true })
    await mkdirAsync(path.dirname(shared), { recursive: true })
    await writeFileAsync(input1, `export {default as input1} from ${JSON.stringify(shared)}`)
    await writeFileAsync(input2, `export {default as input2} from ${JSON.stringify(shared)}`)
    await writeFileAsync(shared, `export default function foo() { return 123 }`)
    const value = await esbuild.build({
      entryPoints: [input1, input2],
      bundle: true,
      outdir,
      format: 'esm',
      splitting: true,
      publicPath: 'https://www.example.com/assets',
      write: false,
    })
    assert.deepStrictEqual(value.outputFiles.length, 3)
    assert.deepStrictEqual(value.outputFiles[0].path, path.join(outdir, 'a', 'in1.js'))
    assert.deepStrictEqual(value.outputFiles[1].path, path.join(outdir, 'b', 'in2.js'))
    assert.deepStrictEqual(value.outputFiles[2].path, path.join(outdir, 'chunk-3MCOY2GR.js'))
    assert.deepStrictEqual(value.outputFiles[0].text, `import {
  foo
} from "https://www.example.com/assets/chunk-3MCOY2GR.js";
export {
  foo as input1
};
`)
    assert.deepStrictEqual(value.outputFiles[1].text, `import {
  foo
} from "https://www.example.com/assets/chunk-3MCOY2GR.js";
export {
  foo as input2
};
`)
    assert.deepStrictEqual(value.outputFiles[2].text, `// scripts/.js-api-tests/splittingPublicPath/c/shared.js
function foo() {
  return 123;
}

export {
  foo
};
`)
  },

  async publicPathHashing({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const data = path.join(testDir, 'data.bin')
    const outdir = path.join(testDir, 'out')
    await writeFileAsync(input, `export {default} from ${JSON.stringify(data)}`)
    await writeFileAsync(data, `stuff`)

    const [result1, result2] = await Promise.all([
      esbuild.build({
        entryPoints: [input],
        bundle: true,
        outdir,
        format: 'cjs',
        loader: { '.bin': 'file' },
        entryNames: '[name]-[hash]',
        write: false,
      }),
      esbuild.build({
        entryPoints: [input],
        bundle: true,
        outdir,
        format: 'cjs',
        loader: { '.bin': 'file' },
        entryNames: '[name]-[hash]',
        publicPath: 'https://www.example.com',
        write: false,
      }),
    ])

    const names1 = result1.outputFiles.map(x => path.basename(x.path)).sort()
    const names2 = result2.outputFiles.map(x => path.basename(x.path)).sort()

    // Check that the public path is included in chunk hashes but not asset hashes
    assert.deepStrictEqual(names1, ['data-BYATPJRB.bin', 'in-OGEHLZ72.js'])
    assert.deepStrictEqual(names2, ['data-BYATPJRB.bin', 'in-IF4VVJK4.js'])
  },

  async fileLoaderPublicPath({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const data = path.join(testDir, 'data.bin')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `export {default as value} from ${JSON.stringify(data)}`)
    await writeFileAsync(data, `stuff`)
    const value = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outfile: output,
      format: 'cjs',
      loader: { '.bin': 'file' },
      publicPath: 'https://www.example.com/assets',
    })
    assert.strictEqual(value.outputFiles, void 0)
    const result = require(output)
    assert.strictEqual(result.value, 'https://www.example.com/assets/data-BYATPJRB.bin')
    assert.strictEqual(result.__esModule, true)
  },

  async fileLoaderCSS({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.css')
    const data = path.join(testDir, 'data.bin')
    const output = path.join(testDir, 'out.css')
    await writeFileAsync(input, `body { background: url(${JSON.stringify(data)}) }`)
    await writeFileAsync(data, `stuff`)
    const value = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outfile: output,
      loader: { '.bin': 'file' },
      publicPath: 'https://www.example.com/assets',
    })
    assert.strictEqual(value.outputFiles, void 0)
    assert.strictEqual(await readFileAsync(output, 'utf8'), `/* scripts/.js-api-tests/fileLoaderCSS/in.css */
body {
  background: url(https://www.example.com/assets/data-BYATPJRB.bin);
}
`)
  },

  async fileLoaderWithAssetPath({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const data = path.join(testDir, 'data.bin')
    const outdir = path.join(testDir, 'out')
    await writeFileAsync(input, `export {default as value} from ${JSON.stringify(data)}`)
    await writeFileAsync(data, `stuff`)
    const value = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outdir,
      format: 'cjs',
      loader: { '.bin': 'file' },
      assetNames: 'assets/name=[name]/hash=[hash]',
    })
    assert.strictEqual(value.outputFiles, void 0)
    const result = require(path.join(outdir, path.basename(input)))
    assert.strictEqual(result.value, './assets/name=data/hash=BYATPJRB.bin')
    assert.strictEqual(result.__esModule, true)
    const stuff = fs.readFileSync(path.join(outdir, result.value), 'utf8')
    assert.strictEqual(stuff, 'stuff')
  },

  async fileLoaderWithAssetPathAndPublicPath({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const data = path.join(testDir, 'data.bin')
    const outdir = path.join(testDir, 'out')
    await writeFileAsync(input, `export {default as value} from ${JSON.stringify(data)}`)
    await writeFileAsync(data, `stuff`)
    const value = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outdir,
      format: 'cjs',
      loader: { '.bin': 'file' },
      assetNames: 'assets/name=[name]/hash=[hash]',
      publicPath: 'https://www.example.com',
    })
    assert.strictEqual(value.outputFiles, void 0)
    const result = require(path.join(outdir, path.basename(input)))
    assert.strictEqual(result.value, 'https://www.example.com/assets/name=data/hash=BYATPJRB.bin')
    assert.strictEqual(result.__esModule, true)
    const stuff = fs.readFileSync(path.join(outdir, 'assets', 'name=data', 'hash=BYATPJRB.bin'), 'utf8')
    assert.strictEqual(stuff, 'stuff')
  },

  async fileLoaderBinaryVsText({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const valid = path.join(testDir, 'valid.bin')
    const invalid = path.join(testDir, 'invalid.bin')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `
      import valid from ${JSON.stringify(valid)}
      import invalid from ${JSON.stringify(invalid)}
      console.log(valid, invalid)
    `)
    await writeFileAsync(valid, Buffer.from([0xCF, 0x80]))
    await writeFileAsync(invalid, Buffer.from([0x80, 0xCF]))
    const value = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outfile: output,
      format: 'cjs',
      loader: { '.bin': 'file' },
      write: false,
    })
    assert.strictEqual(value.outputFiles.length, 3)

    // Valid UTF-8 should decode correctly
    assert.deepEqual(value.outputFiles[0].contents, new Uint8Array([207, 128]))
    assert.strictEqual(value.outputFiles[0].text, 'π')

    // Invalid UTF-8 should be preserved as bytes but should be replaced by the U+FFFD replacement character when decoded
    assert.deepEqual(value.outputFiles[1].contents, new Uint8Array([128, 207]))
    assert.strictEqual(value.outputFiles[1].text, '\uFFFD\uFFFD')
  },

  async metafile({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const imported = path.join(testDir, 'imported.js')
    const text = path.join(testDir, 'text.txt')
    const css = path.join(testDir, 'example.css')
    const outputJS = path.join(testDir, 'out.js')
    const outputCSS = path.join(testDir, 'out.css')
    await writeFileAsync(entry, `
      import x from "./imported"
      const y = require("./text.txt")
      import * as z from "./example.css"
      console.log(x, y, z)
    `)
    await writeFileAsync(imported, 'export default 123')
    await writeFileAsync(text, 'some text')
    await writeFileAsync(css, 'body { some: css; }')
    const result = await esbuild.build({
      entryPoints: [entry],
      bundle: true,
      outfile: outputJS,
      metafile: true,
      sourcemap: true,
      loader: { '.txt': 'file' },
    })

    const json = result.metafile
    assert.strictEqual(Object.keys(json.inputs).length, 4)
    assert.strictEqual(Object.keys(json.outputs).length, 5)
    const cwd = process.cwd()
    const makePath = absPath => path.relative(cwd, absPath).split(path.sep).join('/')

    // Check inputs
    assert.deepStrictEqual(json.inputs[makePath(entry)].bytes, 144)
    assert.deepStrictEqual(json.inputs[makePath(entry)].imports, [
      { path: makePath(imported), kind: 'import-statement', original: './imported' },
      { path: makePath(css), kind: 'import-statement', original: './example.css' },
      { path: makePath(text), kind: 'require-call', original: './text.txt' },
    ])
    assert.deepStrictEqual(json.inputs[makePath(imported)].bytes, 18)
    assert.deepStrictEqual(json.inputs[makePath(imported)].imports, [])
    assert.deepStrictEqual(json.inputs[makePath(text)].bytes, 9)
    assert.deepStrictEqual(json.inputs[makePath(text)].imports, [])
    assert.deepStrictEqual(json.inputs[makePath(css)].bytes, 19)
    assert.deepStrictEqual(json.inputs[makePath(css)].imports, [])

    // Check outputs
    assert.strictEqual(typeof json.outputs[makePath(outputJS)].bytes, 'number')
    assert.strictEqual(typeof json.outputs[makePath(outputCSS)].bytes, 'number')
    assert.strictEqual(typeof json.outputs[makePath(outputJS) + '.map'].bytes, 'number')
    assert.strictEqual(typeof json.outputs[makePath(outputCSS) + '.map'].bytes, 'number')
    assert.strictEqual(json.outputs[makePath(outputJS)].entryPoint, makePath(entry))
    assert.strictEqual(json.outputs[makePath(outputCSS)].entryPoint, undefined) // This is deliberately undefined
    assert.deepStrictEqual(json.outputs[makePath(outputJS) + '.map'].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outputJS) + '.map'].exports, [])
    assert.deepStrictEqual(json.outputs[makePath(outputJS) + '.map'].inputs, {})
    assert.deepStrictEqual(json.outputs[makePath(outputCSS) + '.map'].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outputCSS) + '.map'].exports, [])
    assert.deepStrictEqual(json.outputs[makePath(outputCSS) + '.map'].inputs, {})

    // Check inputs for main output
    const outputInputs = json.outputs[makePath(outputJS)].inputs
    assert.strictEqual(Object.keys(outputInputs).length, 4)
    assert.strictEqual(typeof outputInputs[makePath(entry)].bytesInOutput, 'number')
    assert.strictEqual(typeof outputInputs[makePath(imported)].bytesInOutput, 'number')
    assert.strictEqual(typeof outputInputs[makePath(text)].bytesInOutput, 'number')
    assert.strictEqual(typeof outputInputs[makePath(css)].bytesInOutput, 'number')
  },

  async metafileSplitting({ esbuild, testDir }) {
    const entry1 = path.join(testDir, 'entry1.js')
    const entry2 = path.join(testDir, 'entry2.js')
    const imported = path.join(testDir, 'imported.js')
    const outdir = path.join(testDir, 'out')
    await writeFileAsync(entry1, `
      import x, {f1} from "./${path.basename(imported)}"
      console.log(1, x, f1())
      export {x}
    `)
    await writeFileAsync(entry2, `
      import x, {f2} from "./${path.basename(imported)}"
      console.log('entry 2', x, f2())
      export {x as y}
    `)
    await writeFileAsync(imported, `
      export default 123
      export function f1() {}
      export function f2() {}
      console.log('shared')
    `)
    const result = await esbuild.build({
      entryPoints: [entry1, entry2],
      bundle: true,
      outdir,
      metafile: true,
      splitting: true,
      format: 'esm',
    })

    const json = result.metafile
    assert.strictEqual(Object.keys(json.inputs).length, 3)
    assert.strictEqual(Object.keys(json.outputs).length, 3)
    const cwd = process.cwd()
    const makeOutPath = basename => path.relative(cwd, path.join(outdir, basename)).split(path.sep).join('/')
    const makeInPath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')

    // Check metafile
    const inEntry1 = makeInPath(entry1);
    const inEntry2 = makeInPath(entry2);
    const inImported = makeInPath(imported);
    const chunk = 'chunk-YNV25ITT.js';
    const outEntry1 = makeOutPath(path.basename(entry1));
    const outEntry2 = makeOutPath(path.basename(entry2));
    const outChunk = makeOutPath(chunk);

    assert.deepStrictEqual(json.inputs[inEntry1], {
      bytes: 94,
      imports: [{ path: inImported, kind: 'import-statement', original: './' + path.basename(imported) }],
      format: 'esm',
    })
    assert.deepStrictEqual(json.inputs[inEntry2], {
      bytes: 107,
      imports: [{ path: inImported, kind: 'import-statement', original: './' + path.basename(imported) }],
      format: 'esm',
    })
    assert.deepStrictEqual(json.inputs[inImported], {
      bytes: 118,
      imports: [],
      format: 'esm',
    })

    assert.deepStrictEqual(json.outputs[outEntry1].imports, [{ path: makeOutPath(chunk), kind: 'import-statement' }])
    assert.deepStrictEqual(json.outputs[outEntry2].imports, [{ path: makeOutPath(chunk), kind: 'import-statement' }])
    assert.deepStrictEqual(json.outputs[outChunk].imports, [])

    assert.deepStrictEqual(json.outputs[outEntry1].exports, ['x'])
    assert.deepStrictEqual(json.outputs[outEntry2].exports, ['y'])
    assert.deepStrictEqual(json.outputs[outChunk].exports, ['f1', 'f2', 'imported_default'])

    assert.deepStrictEqual(json.outputs[outEntry1].inputs, { [inEntry1]: { bytesInOutput: 40 } })
    assert.deepStrictEqual(json.outputs[outEntry2].inputs, { [inEntry2]: { bytesInOutput: 48 } })
    assert.deepStrictEqual(json.outputs[outChunk].inputs, { [inImported]: { bytesInOutput: 87 } })
  },

  async metafileSplittingPublicPath({ esbuild, testDir }) {
    const entry1 = path.join(testDir, 'entry1.js')
    const entry2 = path.join(testDir, 'entry2.js')
    const imported = path.join(testDir, 'imported.js')
    const outdir = path.join(testDir, 'out')
    await writeFileAsync(entry1, `
      import x, {f1} from "./${path.basename(imported)}"
      console.log(1, x, f1())
      export {x}
    `)
    await writeFileAsync(entry2, `
      import x, {f2} from "./${path.basename(imported)}"
      console.log('entry 2', x, f2())
      export {x as y}
    `)
    await writeFileAsync(imported, `
      export default 123
      export function f1() {}
      export function f2() {}
      console.log('shared')
    `)
    const result = await esbuild.build({
      entryPoints: [entry1, entry2],
      bundle: true,
      outdir,
      metafile: true,
      splitting: true,
      format: 'esm',
      publicPath: 'public',
    })

    const json = result.metafile
    assert.strictEqual(Object.keys(json.inputs).length, 3)
    assert.strictEqual(Object.keys(json.outputs).length, 3)
    const cwd = process.cwd()
    const makeOutPath = basename => path.relative(cwd, path.join(outdir, basename)).split(path.sep).join('/')
    const makeInPath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')

    // Check metafile
    const inEntry1 = makeInPath(entry1);
    const inEntry2 = makeInPath(entry2);
    const inImported = makeInPath(imported);
    const chunk = 'chunk-ELD4XOGW.js';
    const outEntry1 = makeOutPath(path.basename(entry1));
    const outEntry2 = makeOutPath(path.basename(entry2));
    const outChunk = makeOutPath(chunk);

    assert.deepStrictEqual(json.inputs[inEntry1], {
      bytes: 94,
      imports: [{ path: inImported, kind: 'import-statement', original: './' + path.basename(imported) }],
      format: 'esm',
    })
    assert.deepStrictEqual(json.inputs[inEntry2], {
      bytes: 107,
      imports: [{ path: inImported, kind: 'import-statement', original: './' + path.basename(imported) }],
      format: 'esm',
    })
    assert.deepStrictEqual(json.inputs[inImported], {
      bytes: 118,
      imports: [],
      format: 'esm',
    })

    assert.deepStrictEqual(json.outputs[outEntry1].imports, [{ path: makeOutPath(chunk), kind: 'import-statement' }])
    assert.deepStrictEqual(json.outputs[outEntry2].imports, [{ path: makeOutPath(chunk), kind: 'import-statement' }])
    assert.deepStrictEqual(json.outputs[outChunk].imports, [])

    assert.deepStrictEqual(json.outputs[outEntry1].exports, ['x'])
    assert.deepStrictEqual(json.outputs[outEntry2].exports, ['y'])
    assert.deepStrictEqual(json.outputs[outChunk].exports, ['f1', 'f2', 'imported_default'])

    assert.deepStrictEqual(json.outputs[outEntry1].inputs, { [inEntry1]: { bytesInOutput: 40 } })
    assert.deepStrictEqual(json.outputs[outEntry2].inputs, { [inEntry2]: { bytesInOutput: 48 } })
    assert.deepStrictEqual(json.outputs[outChunk].inputs, { [inImported]: { bytesInOutput: 87 } })
  },

  async metafileSplittingDoubleDynamicImport({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const importDir = path.join(testDir, 'import-dir')
    const import1 = path.join(importDir, 'import1.js')
    const import2 = path.join(importDir, 'import2.js')
    const shared = path.join(testDir, 'shared.js')
    const outdir = path.join(testDir, 'out')
    const makeImportPath = (importing, imported) => './' + path.relative(path.dirname(importing), imported).split(path.sep).join('/')
    await mkdirAsync(importDir)
    await writeFileAsync(entry, `
      import ${JSON.stringify(makeImportPath(entry, shared))}
      import(${JSON.stringify(makeImportPath(entry, import1))})
      import(${JSON.stringify(makeImportPath(entry, import2))})
    `)
    await writeFileAsync(import1, `
      import ${JSON.stringify(makeImportPath(import1, shared))}
    `)
    await writeFileAsync(import2, `
      import ${JSON.stringify(makeImportPath(import2, shared))}
    `)
    await writeFileAsync(shared, `
      console.log('side effect')
    `)
    const result = await esbuild.build({
      entryPoints: [entry],
      bundle: true,
      outdir,
      metafile: true,
      splitting: true,
      format: 'esm',
    })

    const json = result.metafile
    assert.strictEqual(Object.keys(json.inputs).length, 4)
    assert.strictEqual(Object.keys(json.outputs).length, 4)
    const cwd = process.cwd()
    const makeOutPath = basename => path.relative(cwd, path.join(outdir, basename)).split(path.sep).join('/')
    const makeInPath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')

    // Check metafile
    const inEntry = makeInPath(entry);
    const inImport1 = makeInPath(import1);
    const inImport2 = makeInPath(import2);
    const inShared = makeInPath(shared);
    const chunk = 'chunk-3GRHLZ7X.js';
    const outEntry = makeOutPath(path.relative(testDir, entry));
    const outImport1 = makeOutPath('import1-SELM3ZIG.js');
    const outImport2 = makeOutPath('import2-3GSTEHBF.js');
    const outChunk = makeOutPath(chunk);

    assert.deepStrictEqual(json.inputs[inEntry], {
      bytes: 112,
      imports: [
        { path: inShared, kind: 'import-statement', original: makeImportPath(entry, shared) },
        { path: inImport1, kind: 'dynamic-import', original: makeImportPath(entry, import1) },
        { path: inImport2, kind: 'dynamic-import', original: makeImportPath(entry, import2) },
      ],
      format: 'esm',
    })
    assert.deepStrictEqual(json.inputs[inImport1], {
      bytes: 35,
      imports: [
        { path: inShared, kind: 'import-statement', original: makeImportPath(import1, shared) },
      ],
      format: 'esm',
    })
    assert.deepStrictEqual(json.inputs[inImport2], {
      bytes: 35,
      imports: [
        { path: inShared, kind: 'import-statement', original: makeImportPath(import2, shared) },
      ],
      format: 'esm',
    })
    assert.deepStrictEqual(json.inputs[inShared], { bytes: 38, imports: [] })

    assert.deepStrictEqual(Object.keys(json.outputs), [
      outEntry,
      outImport1,
      outImport2,
      outChunk,
    ])

    assert.deepStrictEqual(json.outputs[outEntry].imports, [
      { path: outChunk, kind: 'import-statement' },
      { path: outImport1, kind: 'dynamic-import' },
      { path: outImport2, kind: 'dynamic-import' },
    ])
    assert.deepStrictEqual(json.outputs[outImport1].imports, [{ path: outChunk, kind: 'import-statement' }])
    assert.deepStrictEqual(json.outputs[outImport2].imports, [{ path: outChunk, kind: 'import-statement' }])
    assert.deepStrictEqual(json.outputs[outChunk].imports, [])

    assert.deepStrictEqual(json.outputs[outEntry].exports, [])
    assert.deepStrictEqual(json.outputs[outImport1].exports, [])
    assert.deepStrictEqual(json.outputs[outImport2].exports, [])
    assert.deepStrictEqual(json.outputs[outChunk].exports, [])

    assert.deepStrictEqual(json.outputs[outEntry].inputs, { [inEntry]: { bytesInOutput: 66 } })
    assert.deepStrictEqual(json.outputs[outImport1].inputs, { [inImport1]: { bytesInOutput: 0 } })
    assert.deepStrictEqual(json.outputs[outImport2].inputs, { [inImport2]: { bytesInOutput: 0 } })
    assert.deepStrictEqual(json.outputs[outChunk].inputs, { [inShared]: { bytesInOutput: 28 } })
  },

  async metafileCJSInFormatIIFE({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const outfile = path.join(testDir, 'out.js')
    await writeFileAsync(entry, `module.exports = {}`)
    const result = await esbuild.build({
      entryPoints: [entry],
      outfile,
      metafile: true,
      format: 'iife',
    })
    const cwd = process.cwd()
    const makePath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')
    const json = result.metafile
    assert.deepStrictEqual(json.inputs[makePath(entry)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].exports, [])
  },

  async metafileCJSInFormatCJS({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const outfile = path.join(testDir, 'out.js')
    await writeFileAsync(entry, `module.exports = {}`)
    const result = await esbuild.build({
      entryPoints: [entry],
      outfile,
      metafile: true,
      format: 'cjs',
    })
    const cwd = process.cwd()
    const makePath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')
    const json = result.metafile
    assert.deepStrictEqual(json.inputs[makePath(entry)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].exports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].entryPoint, makePath(entry))
  },

  async metafileCJSInFormatESM({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const outfile = path.join(testDir, 'out.js')
    await writeFileAsync(entry, `module.exports = {}`)
    const result = await esbuild.build({
      entryPoints: [entry],
      outfile,
      metafile: true,
      format: 'esm',
    })
    const cwd = process.cwd()
    const makePath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')
    const json = result.metafile
    assert.deepStrictEqual(json.inputs[makePath(entry)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].exports, ['default'])
  },

  async metafileESMInFormatIIFE({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const outfile = path.join(testDir, 'out.js')
    await writeFileAsync(entry, `export let a = 1, b = 2`)
    const result = await esbuild.build({
      entryPoints: [entry],
      outfile,
      metafile: true,
      format: 'iife',
    })
    const cwd = process.cwd()
    const makePath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')
    const json = result.metafile
    assert.deepStrictEqual(json.inputs[makePath(entry)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].exports, [])
  },

  async metafileESMInFormatCJS({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const outfile = path.join(testDir, 'out.js')
    await writeFileAsync(entry, `export let a = 1, b = 2`)
    const result = await esbuild.build({
      entryPoints: [entry],
      outfile,
      metafile: true,
      format: 'cjs',
    })
    const cwd = process.cwd()
    const makePath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')
    const json = result.metafile
    assert.deepStrictEqual(json.inputs[makePath(entry)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].exports, [])
  },

  async metafileESMInFormatESM({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const outfile = path.join(testDir, 'out.js')
    await writeFileAsync(entry, `export let a = 1, b = 2`)
    const result = await esbuild.build({
      entryPoints: [entry],
      outfile,
      metafile: true,
      format: 'esm',
    })
    const cwd = process.cwd()
    const makePath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')
    const json = result.metafile
    assert.deepStrictEqual(json.inputs[makePath(entry)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].exports, ['a', 'b'])
  },

  async metafileNestedExportNames({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const nested1 = path.join(testDir, 'nested1.js')
    const nested2 = path.join(testDir, 'nested2.js')
    const nested3 = path.join(testDir, 'nested3.js')
    const outfile = path.join(testDir, 'out.js')
    await writeFileAsync(entry, `
      export {nested1} from ${JSON.stringify(nested1)}
      export * from ${JSON.stringify(nested2)}
      export let topLevel = 0
    `)
    await writeFileAsync(nested1, `
      import {nested3} from ${JSON.stringify(nested3)}
      export default 1
      export let nested1 = nested3
    `)
    await writeFileAsync(nested2, `
      export default 'nested2'
      export let nested2 = 2
    `)
    await writeFileAsync(nested3, `
      export let nested3 = 3
    `)
    const result = await esbuild.build({
      entryPoints: [entry],
      bundle: true,
      outfile,
      metafile: true,
      format: 'esm',
    })
    const cwd = process.cwd()
    const makePath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')
    const json = result.metafile
    assert.deepStrictEqual(json.inputs[makePath(entry)].imports, [
      { path: makePath(nested1), kind: 'import-statement', original: nested1 },
      { path: makePath(nested2), kind: 'import-statement', original: nested2 },
    ])
    assert.deepStrictEqual(json.inputs[makePath(nested1)].imports, [
      { path: makePath(nested3), kind: 'import-statement', original: nested3 },
    ])
    assert.deepStrictEqual(json.inputs[makePath(nested2)].imports, [])
    assert.deepStrictEqual(json.inputs[makePath(nested3)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].imports, [])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].exports, ['nested1', 'nested2', 'topLevel'])
    assert.deepStrictEqual(json.outputs[makePath(outfile)].entryPoint, makePath(entry))
  },

  async metafileCSS({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.css')
    const imported = path.join(testDir, 'imported.css')
    const image = path.join(testDir, 'example.png')
    const output = path.join(testDir, 'out.css')
    await writeFileAsync(entry, `
      @import "./imported";
      body { background: url(https://example.com/external.png) }
    `)
    await writeFileAsync(imported, `
      a { background: url(./example.png) }
    `)
    await writeFileAsync(image, 'an image')
    const result = await esbuild.build({
      entryPoints: [entry],
      bundle: true,
      outfile: output,
      metafile: true,
      sourcemap: true,
      loader: { '.png': 'dataurl' },
    })

    const json = result.metafile
    assert.strictEqual(Object.keys(json.inputs).length, 3)
    assert.strictEqual(Object.keys(json.outputs).length, 2)
    const cwd = process.cwd()
    const makePath = absPath => path.relative(cwd, absPath).split(path.sep).join('/')

    // Check inputs
    assert.deepStrictEqual(json, {
      inputs: {
        [makePath(entry)]: {
          bytes: 98,
          imports: [
            { path: makePath(imported), kind: 'import-rule', original: './imported' },
            { external: true, kind: 'url-token', path: 'https://example.com/external.png' },
          ]
        },
        [makePath(image)]: { bytes: 8, imports: [] },
        [makePath(imported)]: { bytes: 48, imports: [{ path: makePath(image), kind: 'url-token', original: './example.png' }] },
      },
      outputs: {
        [makePath(output)]: {
          bytes: 253,
          entryPoint: makePath(entry),
          imports: [
            { kind: 'url-token', path: 'data:image/png,an image' },
            { external: true, kind: 'url-token', path: 'https://example.com/external.png' },
          ],
          inputs: {
            [makePath(entry)]: { bytesInOutput: 62 },
            [makePath(imported)]: { bytesInOutput: 51 },
          },
        },
        [makePath(output + '.map')]: {
          bytes: 312,
          exports: [],
          imports: [],
          inputs: {},
        },
      },
    })
  },

  async metafileLoaderFileMultipleEntry({ esbuild, testDir }) {
    const entry1 = path.join(testDir, 'entry1.js')
    const entry2 = path.join(testDir, 'entry2.js')
    const file = path.join(testDir, 'x.file')
    const outdir = path.join(testDir, 'out')
    await writeFileAsync(entry1, `
      export {default} from './x.file'
    `)
    await writeFileAsync(entry2, `
      import z from './x.file'
      console.log(z)
    `)
    await writeFileAsync(file, `This is a file`)
    const result = await esbuild.build({
      entryPoints: [entry1, entry2],
      bundle: true,
      loader: { '.file': 'file' },
      outdir,
      metafile: true,
      format: 'cjs',
    })
    const json = result.metafile
    const cwd = process.cwd()
    const makePath = pathname => path.relative(cwd, pathname).split(path.sep).join('/')
    const fileName = require(path.join(outdir, 'entry1.js')).default
    const fileKey = makePath(path.join(outdir, fileName))
    assert.deepStrictEqual(json.outputs[fileKey].imports, [])
    assert.deepStrictEqual(json.outputs[fileKey].exports, [])
    assert.deepStrictEqual(json.outputs[fileKey].inputs, { [makePath(file)]: { bytesInOutput: 14 } })
  },

  // Test in-memory output files
  async writeFalse({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    const inputCode = 'console.log()'
    await writeFileAsync(input, inputCode)

    const value = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outfile: output,
      sourcemap: true,
      format: 'esm',
      metafile: true,
      write: false,
    })

    assert.strictEqual(fs.existsSync(output), false)
    assert.notStrictEqual(value.outputFiles, void 0)
    assert.strictEqual(value.outputFiles.length, 2)
    assert.strictEqual(value.outputFiles[0].path, output + '.map')
    assert.strictEqual(value.outputFiles[0].contents.constructor, Uint8Array)
    assert.strictEqual(value.outputFiles[1].path, output)
    assert.strictEqual(value.outputFiles[1].contents.constructor, Uint8Array)

    const sourceMap = JSON.parse(Buffer.from(value.outputFiles[0].contents).toString())
    const js = Buffer.from(value.outputFiles[1].contents).toString()
    assert.strictEqual(sourceMap.version, 3)
    assert.strictEqual(js, `// scripts/.js-api-tests/writeFalse/in.js\nconsole.log();\n//# sourceMappingURL=out.js.map\n`)

    const cwd = process.cwd()
    const makePath = file => path.relative(cwd, file).split(path.sep).join('/')
    const meta = value.metafile
    assert.strictEqual(meta.inputs[makePath(input)].bytes, inputCode.length)
    assert.strictEqual(meta.outputs[makePath(output)].bytes, js.length)
    assert.strictEqual(meta.outputs[makePath(output + '.map')].bytes, value.outputFiles[0].contents.length)
  },

  async allowOverwrite({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.mjs')
    await writeFileAsync(input, `export default FOO`)

    // Fail without "allowOverwrite"
    try {
      await esbuild.build({
        entryPoints: [input],
        outfile: input,
        logLevel: 'silent',
      })
      throw new Error('Expected build failure');
    } catch (e) {
      if (!e || !e.errors || !e.errors.length || !e.errors[0].text.includes('Refusing to overwrite input file'))
        throw e
    }

    // Succeed with "allowOverwrite"
    await esbuild.build({
      entryPoints: [input],
      outfile: input,
      allowOverwrite: true,
      define: { FOO: '123' },
    })

    // This needs to use relative paths to avoid breaking on Windows.
    // Importing by absolute path doesn't work on Windows in node.
    const result = await import('./' + path.relative(__dirname, input))

    assert.strictEqual(result.default, 123)
  },

  async splittingRelativeSameDir({ esbuild, testDir }) {
    const inputA = path.join(testDir, 'a.js')
    const inputB = path.join(testDir, 'b.js')
    const inputCommon = path.join(testDir, 'common.js')
    await writeFileAsync(inputA, `
      import x from "./${path.basename(inputCommon)}"
      console.log('a' + x)
    `)
    await writeFileAsync(inputB, `
      import x from "./${path.basename(inputCommon)}"
      console.log('b' + x)
    `)
    await writeFileAsync(inputCommon, `
      export default 'common'
    `)
    const outdir = path.join(testDir, 'out')
    const value = await esbuild.build({
      entryPoints: [inputA, inputB],
      bundle: true,
      outdir,
      format: 'esm',
      splitting: true,
      write: false,
    })
    assert.strictEqual(value.outputFiles.length, 3)

    // These should all use forward slashes, even on Windows
    const chunk = 'chunk-P3NHLAOZ.js'
    assert.strictEqual(Buffer.from(value.outputFiles[0].contents).toString(), `import {
  common_default
} from "./${chunk}";

// scripts/.js-api-tests/splittingRelativeSameDir/a.js
console.log("a" + common_default);
`)
    assert.strictEqual(Buffer.from(value.outputFiles[1].contents).toString(), `import {
  common_default
} from "./${chunk}";

// scripts/.js-api-tests/splittingRelativeSameDir/b.js
console.log("b" + common_default);
`)
    assert.strictEqual(Buffer.from(value.outputFiles[2].contents).toString(), `// scripts/.js-api-tests/splittingRelativeSameDir/common.js
var common_default = "common";

export {
  common_default
};
`)

    assert.strictEqual(value.outputFiles[0].path, path.join(outdir, path.basename(inputA)))
    assert.strictEqual(value.outputFiles[1].path, path.join(outdir, path.basename(inputB)))
    assert.strictEqual(value.outputFiles[2].path, path.join(outdir, chunk))
  },

  async splittingRelativeNestedDir({ esbuild, testDir }) {
    const inputA = path.join(testDir, 'a/demo.js')
    const inputB = path.join(testDir, 'b/demo.js')
    const inputCommon = path.join(testDir, 'common.js')
    await mkdirAsync(path.dirname(inputA)).catch(x => x)
    await mkdirAsync(path.dirname(inputB)).catch(x => x)
    await writeFileAsync(inputA, `
      import x from "../${path.basename(inputCommon)}"
      console.log('a' + x)
    `)
    await writeFileAsync(inputB, `
      import x from "../${path.basename(inputCommon)}"
      console.log('b' + x)
    `)
    await writeFileAsync(inputCommon, `
      export default 'common'
    `)
    const outdir = path.join(testDir, 'out')
    const value = await esbuild.build({
      entryPoints: [inputA, inputB],
      bundle: true,
      outdir,
      format: 'esm',
      splitting: true,
      write: false,
    })
    assert.strictEqual(value.outputFiles.length, 3)

    // These should all use forward slashes, even on Windows
    const chunk = 'chunk-BPDO6GL2.js'
    assert.strictEqual(Buffer.from(value.outputFiles[0].contents).toString(), `import {
  common_default
} from "../${chunk}";

// scripts/.js-api-tests/splittingRelativeNestedDir/a/demo.js
console.log("a" + common_default);
`)
    assert.strictEqual(Buffer.from(value.outputFiles[1].contents).toString(), `import {
  common_default
} from "../${chunk}";

// scripts/.js-api-tests/splittingRelativeNestedDir/b/demo.js
console.log("b" + common_default);
`)
    assert.strictEqual(Buffer.from(value.outputFiles[2].contents).toString(), `// scripts/.js-api-tests/splittingRelativeNestedDir/common.js
var common_default = "common";

export {
  common_default
};
`)

    assert.strictEqual(value.outputFiles[0].path, path.join(outdir, path.relative(testDir, inputA)))
    assert.strictEqual(value.outputFiles[1].path, path.join(outdir, path.relative(testDir, inputB)))
    assert.strictEqual(value.outputFiles[2].path, path.join(outdir, chunk))
  },

  async splittingWithChunkPath({ esbuild, testDir }) {
    const inputA = path.join(testDir, 'a/demo.js')
    const inputB = path.join(testDir, 'b/demo.js')
    const inputCommon = path.join(testDir, 'common.js')
    await mkdirAsync(path.dirname(inputA)).catch(x => x)
    await mkdirAsync(path.dirname(inputB)).catch(x => x)
    await writeFileAsync(inputA, `
      import x from "../${path.basename(inputCommon)}"
      console.log('a' + x)
    `)
    await writeFileAsync(inputB, `
      import x from "../${path.basename(inputCommon)}"
      console.log('b' + x)
    `)
    await writeFileAsync(inputCommon, `
      export default 'common'
    `)
    const outdir = path.join(testDir, 'out')
    const value = await esbuild.build({
      entryPoints: [inputA, inputB],
      bundle: true,
      outdir,
      format: 'esm',
      splitting: true,
      write: false,
      chunkNames: 'chunks/name=[name]/hash=[hash]',
    })
    assert.strictEqual(value.outputFiles.length, 3)

    // These should all use forward slashes, even on Windows
    const chunk = 'chunks/name=chunk/hash=7RIY4OCQ.js'
    assert.strictEqual(value.outputFiles[0].text, `import {
  common_default
} from "../${chunk}";

// scripts/.js-api-tests/splittingWithChunkPath/a/demo.js
console.log("a" + common_default);
`)
    assert.strictEqual(value.outputFiles[1].text, `import {
  common_default
} from "../${chunk}";

// scripts/.js-api-tests/splittingWithChunkPath/b/demo.js
console.log("b" + common_default);
`)
    assert.strictEqual(value.outputFiles[2].text, `// scripts/.js-api-tests/splittingWithChunkPath/common.js
var common_default = "common";

export {
  common_default
};
`)

    assert.strictEqual(value.outputFiles[0].path, path.join(outdir, path.relative(testDir, inputA)))
    assert.strictEqual(value.outputFiles[1].path, path.join(outdir, path.relative(testDir, inputB)))
    assert.strictEqual(value.outputFiles[2].path, path.join(outdir, chunk))
  },

  async splittingWithEntryHashes({ esbuild, testDir }) {
    const inputA = path.join(testDir, 'a/demo.js')
    const inputB = path.join(testDir, 'b/demo.js')
    const inputCommon = path.join(testDir, 'common.js')
    await mkdirAsync(path.dirname(inputA)).catch(x => x)
    await mkdirAsync(path.dirname(inputB)).catch(x => x)
    await writeFileAsync(inputA, `
      import x from "../${path.basename(inputCommon)}"
      console.log('a' + x.name)
    `)
    await writeFileAsync(inputB, `
      import x from "../${path.basename(inputCommon)}"
      console.log('b' + x.name)
    `)
    await writeFileAsync(inputCommon, `
      export default { name: 'common' }
    `)
    const outdir = path.join(testDir, 'out')
    const value = await esbuild.build({
      entryPoints: [inputA, inputB],
      bundle: true,
      outdir,
      format: 'esm',
      splitting: true,
      write: false,
      entryNames: 'entry/name=[name]/hash=[hash]',
      chunkNames: 'chunks/name=[name]/hash=[hash]',
    })
    assert.strictEqual(value.outputFiles.length, 3)

    // These should all use forward slashes, even on Windows
    const chunk = 'chunks/name=chunk/hash=6VLJRT45.js'
    assert.strictEqual(value.outputFiles[0].text, `import {
  common_default
} from "../../${chunk}";

// scripts/.js-api-tests/splittingWithEntryHashes/a/demo.js
console.log("a" + common_default.name);
`)
    assert.strictEqual(value.outputFiles[1].text, `import {
  common_default
} from "../../${chunk}";

// scripts/.js-api-tests/splittingWithEntryHashes/b/demo.js
console.log("b" + common_default.name);
`)
    assert.strictEqual(value.outputFiles[2].text, `// scripts/.js-api-tests/splittingWithEntryHashes/common.js
var common_default = { name: "common" };

export {
  common_default
};
`)

    const outputA = 'entry/name=demo/hash=ZKX5HN4L.js'
    const outputB = 'entry/name=demo/hash=TYTZIN4P.js'
    assert.strictEqual(value.outputFiles[0].path, path.join(outdir, outputA))
    assert.strictEqual(value.outputFiles[1].path, path.join(outdir, outputB))
    assert.strictEqual(value.outputFiles[2].path, path.join(outdir, chunk))
  },

  async splittingWithChunkPathAndCrossChunkImportsIssue899({ esbuild, testDir }) {
    const entry1 = path.join(testDir, 'src', 'entry1.js')
    const entry2 = path.join(testDir, 'src', 'entry2.js')
    const entry3 = path.join(testDir, 'src', 'entry3.js')
    const shared1 = path.join(testDir, 'src', 'shared1.js')
    const shared2 = path.join(testDir, 'src', 'shared2.js')
    const shared3 = path.join(testDir, 'src', 'shared3.js')
    await mkdirAsync(path.join(testDir, 'src')).catch(x => x)
    await writeFileAsync(entry1, `
      import { shared1 } from './shared1';
      import { shared2 } from './shared2';
      export default async function() {
        return shared1() + shared2();
      }
    `)
    await writeFileAsync(entry2, `
      import { shared2 } from './shared2';
      import { shared3 } from './shared3';
      export default async function() {
        return shared2() + shared3();
      }
    `)
    await writeFileAsync(entry3, `
      import { shared3 } from './shared3';
      import { shared1 } from './shared1';
      export default async function() {
        return shared3() + shared1();
      }
    `)
    await writeFileAsync(shared1, `
      import { shared2 } from './shared2';
      export function shared1() {
        return shared2().replace('2', '1');
      }
    `)
    await writeFileAsync(shared2, `
      import { shared3 } from './shared3'
      export function shared2() {
        return 'shared2';
      }
    `)
    await writeFileAsync(shared3, `
      export function shared3() {
        return 'shared3';
      }
    `)
    const outdir = path.join(testDir, 'out')
    await esbuild.build({
      entryPoints: [entry1, entry2, entry3],
      bundle: true,
      outdir,
      format: 'esm',
      splitting: true,
      outExtension: { '.js': '.mjs' },
      chunkNames: 'chunks/[hash]/[name]',
    })

    // This needs to use relative paths to avoid breaking on Windows.
    // Importing by absolute path doesn't work on Windows in node.
    const result1 = await import('./' + path.relative(__dirname, path.join(outdir, 'entry1.mjs')))
    const result2 = await import('./' + path.relative(__dirname, path.join(outdir, 'entry2.mjs')))
    const result3 = await import('./' + path.relative(__dirname, path.join(outdir, 'entry3.mjs')))

    assert.strictEqual(await result1.default(), 'shared1shared2');
    assert.strictEqual(await result2.default(), 'shared2shared3');
    assert.strictEqual(await result3.default(), 'shared3shared1');
  },

  async splittingStaticImportHashChange({ esbuild, testDir }) {
    const input1 = path.join(testDir, 'a', 'in1.js')
    const input2 = path.join(testDir, 'b', 'in2.js')
    const outdir = path.join(testDir, 'out')

    await mkdirAsync(path.dirname(input1), { recursive: true })
    await mkdirAsync(path.dirname(input2), { recursive: true })
    await writeFileAsync(input1, `import ${JSON.stringify(input2)}`)
    await writeFileAsync(input2, `console.log(123)`)

    const result1 = await esbuild.build({
      entryPoints: [input1, input2],
      bundle: true,
      outdir,
      format: 'esm',
      splitting: true,
      write: false,
      entryNames: '[name]-[hash]',
    })

    await writeFileAsync(input2, `console.log(321)`)

    const result2 = await esbuild.build({
      entryPoints: [input1, input2],
      bundle: true,
      outdir,
      format: 'esm',
      splitting: true,
      write: false,
      entryNames: '[name]-[hash]',
    })

    assert.strictEqual(result1.outputFiles.length, 3)
    assert.strictEqual(result2.outputFiles.length, 3)

    // The hashes of both output files must change. Previously there was a bug
    // where hash changes worked for static imports but not for dynamic imports.
    for (const { path: oldPath } of result1.outputFiles)
      for (const { path: newPath } of result2.outputFiles)
        assert.notStrictEqual(oldPath, newPath)
  },

  async splittingDynamicImportHashChangeIssue1076({ esbuild, testDir }) {
    const input1 = path.join(testDir, 'a', 'in1.js')
    const input2 = path.join(testDir, 'b', 'in2.js')
    const outdir = path.join(testDir, 'out')

    await mkdirAsync(path.dirname(input1), { recursive: true })
    await mkdirAsync(path.dirname(input2), { recursive: true })
    await writeFileAsync(input1, `import(${JSON.stringify(input2)})`)
    await writeFileAsync(input2, `console.log(123)`)

    const result1 = await esbuild.build({
      entryPoints: [input1],
      bundle: true,
      outdir,
      format: 'esm',
      splitting: true,
      write: false,
      entryNames: '[name]-[hash]',
    })

    await writeFileAsync(input2, `console.log(321)`)

    const result2 = await esbuild.build({
      entryPoints: [input1],
      bundle: true,
      outdir,
      format: 'esm',
      splitting: true,
      write: false,
      entryNames: '[name]-[hash]',
    })

    assert.strictEqual(result1.outputFiles.length, 2)
    assert.strictEqual(result2.outputFiles.length, 2)

    // The hashes of both output files must change. Previously there was a bug
    // where hash changes worked for static imports but not for dynamic imports.
    for (const { path: oldPath } of result1.outputFiles)
      for (const { path: newPath } of result2.outputFiles)
        assert.notStrictEqual(oldPath, newPath)
  },

  async stdinStdoutBundle({ esbuild, testDir }) {
    const auxiliary = path.join(testDir, 'auxiliary.js')
    await writeFileAsync(auxiliary, 'export default 123')
    const value = await esbuild.build({
      stdin: {
        contents: `
          import x from './auxiliary.js'
          console.log(x)
        `,
        resolveDir: testDir,
      },
      bundle: true,
      write: false,
    })
    assert.strictEqual(value.outputFiles.length, 1)
    assert.strictEqual(value.outputFiles[0].path, '<stdout>')
    assert.strictEqual(Buffer.from(value.outputFiles[0].contents).toString(), `(() => {
  // scripts/.js-api-tests/stdinStdoutBundle/auxiliary.js
  var auxiliary_default = 123;

  // <stdin>
  console.log(auxiliary_default);
})();
`)
  },

  async stdinOutfileBundle({ esbuild, testDir }) {
    const auxiliary = path.join(testDir, 'auxiliary.js')
    const outfile = path.join(testDir, 'out.js')
    await writeFileAsync(auxiliary, 'export default 123')
    const value = await esbuild.build({
      stdin: {
        contents: `
          import x from './auxiliary.js'
          export {x as fromStdin}
        `,
        resolveDir: testDir,
      },
      bundle: true,
      outfile,
      format: 'cjs',
    })
    assert.strictEqual(value.outputFiles, void 0)
    const result = require(outfile)
    assert.strictEqual(result.fromStdin, 123)
  },

  async stdinAndEntryBundle({ esbuild, testDir }) {
    const srcDir = path.join(testDir, 'src')
    const entry = path.join(srcDir, 'entry.js')
    const auxiliary = path.join(srcDir, 'auxiliary.js')
    const outdir = path.join(testDir, 'out')
    await mkdirAsync(srcDir)
    await writeFileAsync(auxiliary, 'export default 123')
    await writeFileAsync(entry, `
      import x from './auxiliary.js'
      export let fromEntry = x
    `)
    const value = await esbuild.build({
      entryPoints: [entry],
      stdin: {
        contents: `
          import x from './src/auxiliary.js'
          export {x as fromStdin}
        `,
        resolveDir: testDir,
      },
      bundle: true,
      outdir,
      format: 'cjs',
    })
    assert.strictEqual(value.outputFiles, void 0)
    const entryResult = require(path.join(outdir, path.basename(entry)))
    assert.strictEqual(entryResult.fromEntry, 123)
    const stdinResult = require(path.join(outdir, path.basename('stdin.js')))
    assert.strictEqual(stdinResult.fromStdin, 123)
  },

  async forceTsConfig({ esbuild, testDir }) {
    // ./tsconfig.json
    // ./a/forced-config.json
    // ./a/b/test-impl.js
    // ./a/b/c/in.js
    const aDir = path.join(testDir, 'a')
    const bDir = path.join(aDir, 'b')
    const cDir = path.join(bDir, 'c')
    await mkdirAsync(aDir).catch(x => x)
    await mkdirAsync(bDir).catch(x => x)
    await mkdirAsync(cDir).catch(x => x)
    const input = path.join(cDir, 'in.js')
    const forced = path.join(bDir, 'test-impl.js')
    const tsconfigIgnore = path.join(testDir, 'tsconfig.json')
    const tsconfigForced = path.join(aDir, 'forced-config.json')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, 'import "test"')
    await writeFileAsync(forced, 'console.log("success")')
    await writeFileAsync(tsconfigIgnore, '{"compilerOptions": {"baseUrl": "./a", "paths": {"test": ["./ignore.js"]}}}')
    await writeFileAsync(tsconfigForced, '{"compilerOptions": {"baseUrl": "./b", "paths": {"test": ["./test-impl.js"]}}}')
    await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outfile: output,
      tsconfig: tsconfigForced,
      format: 'esm',
    })
    const result = await readFileAsync(output, 'utf8')
    assert.strictEqual(result, `// scripts/.js-api-tests/forceTsConfig/a/b/test-impl.js
console.log("success");
`)
  },

  async es5({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const cjs = path.join(testDir, 'cjs.js')
    const esm = path.join(testDir, 'esm.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `
      export {foo} from "./cjs"
      export * as bar from "./esm"
    `)
    await writeFileAsync(cjs, 'exports.foo = 123')
    await writeFileAsync(esm, 'export var foo = 123')
    const value = await esbuild.build({
      entryPoints: [input],
      bundle: true,
      outfile: output,
      format: 'cjs',
      target: 'es5',
    })
    assert.strictEqual(value.outputFiles, void 0)
    const result = require(output)
    assert.strictEqual(result.foo, 123)
    assert.strictEqual(result.bar.foo, 123)
    assert.strictEqual(result.__esModule, true)
    const contents = await readFileAsync(output, 'utf8')
    assert.strictEqual(contents.indexOf('=>'), -1)
    assert.strictEqual(contents.indexOf('const'), -1)
  },

  async outbaseImplicit({ esbuild, testDir }) {
    const outbase = path.join(testDir, 'pages', 'a')
    const b = path.join(outbase, 'b', 'index.js')
    const c = path.join(outbase, 'c', 'index.js')
    const outdir = path.join(testDir, 'outdir')
    await mkdirAsync(path.dirname(b), { recursive: true })
    await mkdirAsync(path.dirname(c), { recursive: true })
    await writeFileAsync(b, 'module.exports = "b"')
    await writeFileAsync(c, 'module.exports = "c"')
    await esbuild.build({
      entryPoints: [
        path.relative(process.cwd(), b),
        path.relative(process.cwd(), c),
      ],
      outdir,
      format: 'cjs',
    })
    const outB = path.join(outdir, path.relative(outbase, b))
    const outC = path.join(outdir, path.relative(outbase, c))
    assert.strictEqual(require(outB), 'b')
    assert.strictEqual(require(outC), 'c')
  },

  async outbaseRelPath({ esbuild, testDir }) {
    const outbase = path.join(testDir, 'pages')
    const b = path.join(outbase, 'a', 'b', 'index.js')
    const c = path.join(outbase, 'a', 'c', 'index.js')
    const outdir = path.join(testDir, 'outdir')
    await mkdirAsync(path.dirname(b), { recursive: true })
    await mkdirAsync(path.dirname(c), { recursive: true })
    await writeFileAsync(b, 'module.exports = "b"')
    await writeFileAsync(c, 'module.exports = "c"')
    await esbuild.build({
      entryPoints: [
        path.relative(process.cwd(), b),
        path.relative(process.cwd(), c),
      ],
      outdir,
      outbase,
      format: 'cjs',
    })
    const outB = path.join(outdir, path.relative(outbase, b))
    const outC = path.join(outdir, path.relative(outbase, c))
    assert.strictEqual(require(outB), 'b')
    assert.strictEqual(require(outC), 'c')
  },

  async outbaseAbsPath({ esbuild, testDir }) {
    const outbase = path.join(testDir, 'pages')
    const b = path.join(outbase, 'a', 'b', 'index.js')
    const c = path.join(outbase, 'a', 'c', 'index.js')
    const outdir = path.join(testDir, 'outdir')
    await mkdirAsync(path.dirname(b), { recursive: true })
    await mkdirAsync(path.dirname(c), { recursive: true })
    await writeFileAsync(b, 'module.exports = "b"')
    await writeFileAsync(c, 'module.exports = "c"')
    await esbuild.build({
      entryPoints: [b, c],
      outdir,
      outbase,
      format: 'cjs',
    })
    const outB = path.join(outdir, path.relative(outbase, b))
    const outC = path.join(outdir, path.relative(outbase, c))
    assert.strictEqual(require(outB), 'b')
    assert.strictEqual(require(outC), 'c')
  },

  async bundleTreeShakingDefault({ esbuild }) {
    const { outputFiles } = await esbuild.build({
      stdin: {
        contents: `
          let removeMe1 = /* @__PURE__ */ fn();
          let removeMe2 = <div/>;
        `,
        loader: 'jsx',
      },
      write: false,
      bundle: true,
    })
    assert.strictEqual(outputFiles[0].text, `(() => {\n})();\n`)
  },

  async bundleTreeShakingTrue({ esbuild }) {
    const { outputFiles } = await esbuild.build({
      stdin: {
        contents: `
          let removeMe1 = /* @__PURE__ */ fn();
          let removeMe2 = <div/>;
        `,
        loader: 'jsx',
      },
      write: false,
      bundle: true,
      treeShaking: true,
    })
    assert.strictEqual(outputFiles[0].text, `(() => {\n})();\n`)
  },

  async bundleTreeShakingIgnoreAnnotations({ esbuild }) {
    const { outputFiles } = await esbuild.build({
      stdin: {
        contents: `
          let keepMe1 = /* @__PURE__ */ fn();
          let keepMe2 = <div/>;
        `,
        loader: 'jsx',
      },
      write: false,
      bundle: true,
      ignoreAnnotations: true,
    })
    assert.strictEqual(outputFiles[0].text, `(() => {
  // <stdin>
  var keepMe1 = fn();
  var keepMe2 = React.createElement("div", null);
})();
`)
  },

  async externalWithWildcard({ esbuild }) {
    const { outputFiles } = await esbuild.build({
      stdin: {
        contents: `require('/assets/file.png')`,
      },
      write: false,
      bundle: true,
      external: ['/assets/*.png'],
      platform: 'node',
    })
    assert.strictEqual(outputFiles[0].text, `// <stdin>
require("/assets/file.png");
`)
  },

  async externalPackages({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const pkgPath = path.join(testDir, 'node_modules', 'pkg', 'path.js')
    const dirPath = path.join(testDir, 'dir', 'path.js')
    await mkdirAsync(path.dirname(pkgPath), { recursive: true })
    await mkdirAsync(path.dirname(dirPath), { recursive: true })
    await writeFileAsync(input, `
      import 'pkg/path.js'
      import './dir/path.js'
      import 'before/alias'
    `)
    await writeFileAsync(pkgPath, `console.log('pkg')`)
    await writeFileAsync(dirPath, `console.log('dir')`)
    const { outputFiles } = await esbuild.build({
      entryPoints: [input],
      write: false,
      bundle: true,
      packages: 'external',
      format: 'esm',
      alias: { 'before': 'after' },
    })
    assert.strictEqual(outputFiles[0].text, `// scripts/.js-api-tests/externalPackages/in.js
import "pkg/path.js";

// scripts/.js-api-tests/externalPackages/dir/path.js
console.log("dir");

// scripts/.js-api-tests/externalPackages/in.js
import "after/alias";
`)
  },

  async errorInvalidExternalWithTwoWildcards({ esbuild }) {
    try {
      await esbuild.build({
        entryPoints: ['in.js'],
        external: ['a*b*c'],
        write: false,
        logLevel: 'silent',
      })
      throw new Error('Expected build failure');
    } catch (e) {
      if (e.message !== 'Build failed with 1 error:\nerror: External path "a*b*c" cannot have more than one "*" wildcard') {
        throw e;
      }
    }
  },

  async jsBannerBuild({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const outfile = path.join(testDir, 'out.js')
    await writeFileAsync(input, `if (!bannerDefined) throw 'fail'`)
    await esbuild.build({
      entryPoints: [input],
      outfile,
      banner: { js: 'const bannerDefined = true' },
    })
    require(outfile)
  },

  async jsFooterBuild({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const outfile = path.join(testDir, 'out.js')
    await writeFileAsync(input, `footer()`)
    await esbuild.build({
      entryPoints: [input],
      outfile,
      footer: { js: 'function footer() {}' },
    })
    require(outfile)
  },

  async jsBannerFooterBuild({ esbuild, testDir }) {
    const aPath = path.join(testDir, 'a.js')
    const bPath = path.join(testDir, 'b.js')
    const outdir = path.join(testDir, 'out')
    await writeFileAsync(aPath, `module.exports = { banner: bannerDefined, footer };`)
    await writeFileAsync(bPath, `module.exports = { banner: bannerDefined, footer };`)
    await esbuild.build({
      entryPoints: [aPath, bPath],
      outdir,
      banner: { js: 'const bannerDefined = true' },
      footer: { js: 'function footer() {}' },
    })
    const a = require(path.join(outdir, path.basename(aPath)))
    const b = require(path.join(outdir, path.basename(bPath)))
    if (!a.banner || !b.banner) throw 'fail'
    a.footer()
    b.footer()
  },

  async cssBannerFooterBuild({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.css')
    const outfile = path.join(testDir, 'out.css')
    await writeFileAsync(input, `div { color: red }`)
    await esbuild.build({
      entryPoints: [input],
      outfile,
      banner: { css: '/* banner */' },
      footer: { css: '/* footer */' },
    })
    const code = await readFileAsync(outfile, 'utf8')
    assert.strictEqual(code, `/* banner */\ndiv {\n  color: red;\n}\n/* footer */\n`)
  },

  async buildRelativeIssue693({ esbuild }) {
    const result = await esbuild.build({
      stdin: {
        contents: `const x=1`,
      },
      write: false,
      outfile: 'esbuild.js',
    });
    assert.strictEqual(result.outputFiles.length, 1)
    assert.strictEqual(result.outputFiles[0].path, path.join(process.cwd(), 'esbuild.js'))
    assert.strictEqual(result.outputFiles[0].text, 'const x = 1;\n')
  },

  async rebuildBasic({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    const context = await esbuild.context({
      entryPoints: [input],
      outfile: output,
      format: 'esm',
    });

    // Build 1
    await writeFileAsync(input, `console.log('abc')`)
    const result1 = await context.rebuild();
    assert.strictEqual(result1.outputFiles, void 0)
    assert.strictEqual(await readFileAsync(output, 'utf8'), `console.log("abc");\n`)

    // Build 2
    await writeFileAsync(input, `console.log('xyz')`)
    const result2 = await context.rebuild();
    assert.strictEqual(result2.outputFiles, void 0)
    assert.strictEqual(await readFileAsync(output, 'utf8'), `console.log("xyz");\n`)

    // Build 3
    await writeFileAsync(input, `console.log(123)`)
    const result3 = await context.rebuild();
    assert.strictEqual(result3.outputFiles, void 0)
    assert.strictEqual(await readFileAsync(output, 'utf8'), `console.log(123);\n`)

    // Further rebuilds should not be possible after a dispose
    context.dispose()
    try {
      await context.rebuild()
      throw new Error('Expected an error to be thrown')
    } catch (e) {
      assert.strictEqual(e.message, 'Cannot rebuild')
    }
  },

  async rebuildMerging({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, `console.log(123)`)

    let resolveWait
    let waitPromise = new Promise(resolve => {
      resolveWait = resolve
    })
    const waitPlugin = {
      name: 'wait-plugin',
      setup(build) {
        build.onStart(() => waitPromise)
      },
    }

    const context = await esbuild.context({
      entryPoints: [input],
      outfile: output,
      format: 'esm',
      plugins: [waitPlugin],
      write: false,
    });

    try {
      // Do two rebuilds
      const rebuild1 = context.rebuild()
      const rebuild2 = context.rebuild()

      // Let the build end
      resolveWait()

      // Get both rebuild results
      const result1 = await rebuild1;
      const result2 = await rebuild2;
      assert.strictEqual(result1, result2)
      assert.strictEqual(result1.outputFiles.length, 1)
      assert.strictEqual(result1.outputFiles[0].text, 'console.log(123);\n')

      // Make an edit
      await writeFileAsync(input, `console.log(234)`)

      // Do two more rebuilds
      waitPromise = new Promise(resolve => {
        resolveWait = resolve
      })
      const rebuild3 = context.rebuild()
      const rebuild4 = context.rebuild()

      // Let the build end
      resolveWait()

      // Get both rebuild results
      const result3 = await rebuild3;
      const result4 = await rebuild4;
      assert.strictEqual(result3, result4)
      assert.notStrictEqual(result3, result1)
      assert.strictEqual(result3.outputFiles.length, 1)
      assert.strictEqual(result3.outputFiles[0].text, 'console.log(234);\n')
    } finally {
      await context.dispose()
    }
  },

  async rebuildIndependent({ esbuild, testDir }) {
    const inputA = path.join(testDir, 'in-a.js')
    const inputB = path.join(testDir, 'in-b.js')
    const outputA = path.join(testDir, 'out-a.js')
    const outputB = path.join(testDir, 'out-b.js')
    const contextA = await esbuild.context({
      entryPoints: [inputA],
      outfile: outputA,
      format: 'esm',
    })
    const contextB = await esbuild.context({
      entryPoints: [inputB],
      outfile: outputB,
      format: 'esm',
    })

    // Build 1
    await writeFileAsync(inputA, `console.log('a')`)
    await writeFileAsync(inputB, `console.log('b')`)
    assert.notStrictEqual(contextA.rebuild, contextB.rebuild)
    const resultA1 = await contextA.rebuild()
    const resultB1 = await contextB.rebuild()
    assert.strictEqual(resultA1.outputFiles, void 0)
    assert.strictEqual(resultB1.outputFiles, void 0)
    assert.strictEqual(await readFileAsync(outputA, 'utf8'), `console.log("a");\n`)
    assert.strictEqual(await readFileAsync(outputB, 'utf8'), `console.log("b");\n`)

    // Build 2
    await writeFileAsync(inputA, `console.log(1)`)
    await writeFileAsync(inputB, `console.log(2)`)
    const promiseA = contextA.rebuild();
    const promiseB = contextB.rebuild();
    const resultA2 = await promiseA;
    const resultB2 = await promiseB;
    assert.strictEqual(resultA2.outputFiles, void 0)
    assert.strictEqual(resultB2.outputFiles, void 0)
    assert.strictEqual(await readFileAsync(outputA, 'utf8'), `console.log(1);\n`)
    assert.strictEqual(await readFileAsync(outputB, 'utf8'), `console.log(2);\n`)

    // Further rebuilds should not be possible after a dispose
    contextA.dispose()
    try {
      await contextA.rebuild()
      throw new Error('Expected an error to be thrown (context A)')
    } catch (e) {
      assert.strictEqual(e.message, 'Cannot rebuild')
    }

    // Build 3
    await writeFileAsync(inputB, `console.log(3)`)
    const resultB3 = await contextB.rebuild()
    assert.strictEqual(resultB3.outputFiles, void 0)
    assert.strictEqual(await readFileAsync(outputB, 'utf8'), `console.log(3);\n`)

    // Further rebuilds should not be possible after a dispose
    contextB.dispose()
    try {
      await contextB.rebuild()
      throw new Error('Expected an error to be thrown (context B)')
    } catch (e) {
      assert.strictEqual(e.message, 'Cannot rebuild')
    }
  },

  async rebuildParallel({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    const context = await esbuild.context({
      entryPoints: [input],
      outfile: output,
      format: 'esm',
    })

    try {
      // Build 1
      await writeFileAsync(input, `console.log('abc')`)
      const result1 = await context.rebuild()
      assert.strictEqual(result1.outputFiles, void 0)
      assert.strictEqual(await readFileAsync(output, 'utf8'), `console.log("abc");\n`)

      // Build 2
      await writeFileAsync(input, `console.log('xyz')`)
      const promise2A = context.rebuild();
      const promise2B = context.rebuild();
      const result2A = await promise2A;
      const result2B = await promise2B;
      assert.strictEqual(result2A.outputFiles, void 0)
      assert.strictEqual(result2B.outputFiles, void 0)
      assert.strictEqual(await readFileAsync(output, 'utf8'), `console.log("xyz");\n`)

      // Build 3
      await writeFileAsync(input, `console.log(123)`)
      const promise3A = context.rebuild();
      const promise3B = context.rebuild();
      const result3A = await promise3A;
      const result3B = await promise3B;
      assert.strictEqual(result3A.outputFiles, void 0)
      assert.strictEqual(result3B.outputFiles, void 0)
      assert.strictEqual(await readFileAsync(output, 'utf8'), `console.log(123);\n`)
    } finally {
      context.dispose()
    }

    // Further rebuilds should not be possible after a dispose
    try {
      await context.rebuild()
      throw new Error('Expected an error to be thrown')
    } catch (e) {
      assert.strictEqual(e.message, 'Cannot rebuild')
    }
  },

  async rebuildCancel({ esbuild }) {
    let loopForever = true
    let onEndResult
    let onLoadCallback

    const context = await esbuild.context({
      entryPoints: ['entry'],
      bundle: true,
      write: false,
      logLevel: 'silent',
      format: 'esm',
      plugins: [{
        name: '∞',
        setup(build) {
          build.onResolve({ filter: /.*/ }, args => {
            return { path: args.path, namespace: '∞' }
          })
          build.onLoad({ filter: /.*/ }, async (args) => {
            onLoadCallback()
            if (!loopForever) return { contents: 'foo()' }
            await new Promise(r => setTimeout(r, 10))
            return { contents: 'import ' + JSON.stringify(args.path + '.') }
          })
          build.onEnd(result => {
            onEndResult = result
          })
        },
      }],
    })

    try {
      // Build 1
      {
        // Stop the build when "onLoad" has been called at least 5 times
        const shouldCancelPromise = new Promise(resolve => {
          let count = 0
          onLoadCallback = () => {
            if (++count > 5) resolve()
          }
        })

        // Start a build
        const buildPromise = context.rebuild()

        // Add a dummy catch handler to avoid terminating due to an unhandled exception
        buildPromise.catch(() => { })

        // Cancel the build
        await shouldCancelPromise
        await context.cancel()

        // Check the result
        try {
          await buildPromise
          throw new Error('Expected an error to be thrown')
        } catch (error) {
          assert.strictEqual(error.message, `Build failed with 1 error:\nerror: The build was canceled`)
          assert.strictEqual(error.errors.length, 1)
          assert.strictEqual(error.warnings.length, 0)
          assert.notStrictEqual(onEndResult, undefined)
          assert.strictEqual(onEndResult.errors.length, 1)
          assert.strictEqual(onEndResult.errors[0].text, 'The build was canceled')
          assert.strictEqual(onEndResult.warnings.length, 0)
          assert.strictEqual(onEndResult.outputFiles.length, 0)
        }
      }

      // Build 2
      {
        // Stop the build when "onLoad" has been called at least 5 times
        const shouldCancelPromise = new Promise(resolve => {
          let count = 0
          onLoadCallback = () => {
            if (++count > 5) resolve()
          }
        })

        // Start a build
        const buildPromise = context.rebuild()

        // Add a dummy catch handler to avoid terminating due to an unhandled exception
        buildPromise.catch(() => { })

        // Cancel the build
        await shouldCancelPromise
        await context.cancel()

        // Check the result
        try {
          await buildPromise
          throw new Error('Expected an error to be thrown')
        } catch (error) {
          assert.strictEqual(error.message, `Build failed with 1 error:\nerror: The build was canceled`)
          assert.strictEqual(error.errors.length, 1)
          assert.strictEqual(error.warnings.length, 0)
          assert.notStrictEqual(onEndResult, undefined)
          assert.strictEqual(onEndResult.errors.length, 1)
          assert.strictEqual(onEndResult.errors[0].text, 'The build was canceled')
          assert.strictEqual(onEndResult.warnings.length, 0)
          assert.strictEqual(onEndResult.outputFiles.length, 0)
        }
      }

      // Build 3
      loopForever = false
      {
        const result = await context.rebuild()
        assert.strictEqual(result.errors.length, 0)
        assert.strictEqual(result.warnings.length, 0)
        assert.strictEqual(result.outputFiles.length, 1)
        assert.strictEqual(result.outputFiles[0].text, `// ∞:entry\nfoo();\n`)
        assert.strictEqual(onEndResult, result)
      }
    } finally {
      context.dispose()
    }
  },

  // This test checks for races between manual "rebuild()" and "cancel()".
  // Ideally calling "cancel()" after "rebuild()" will always cancel it even
  // if the Go side hasn't started running the rebuild yet. Since Go is multi-
  // threaded, Go can actually start running the "cancel()" before the
  // "rebuild()" (i.e. in the other order). The Go code does some complex stuff
  // with mutexes to try to make this ideal behavior happen despite multi-
  // threading.
  async rapidRebuildCancel({ esbuild }) {
    const context = await esbuild.context({
      entryPoints: ['entry'],
      logLevel: 'silent',
      plugins: [{
        name: '∞',
        setup(build) {
          build.onResolve({ filter: /.*/ }, args => {
            return { path: args.path, namespace: '∞' }
          })
          build.onLoad({ filter: /.*/ }, async (args) => {
            await new Promise(r => setTimeout(r, 10))
            return { contents: 'import ' + JSON.stringify(args.path + '.') }
          })
        },
      }],
    })

    try {
      const promises = []
      for (let i = 0; i < 100; i++) {
        const promise = context.rebuild()
        promise.catch(() => { /* avoid termination due to an uncaught exception */ })
        promises.push(promise)
        await context.cancel()
      }

      for (let i = 0; i < promises.length; i++) {
        try {
          await promises[i]
          throw new Error('Expected an error to be thrown for rebuild ' + i)
        } catch (err) {
          if (!err.errors || err.errors.length !== 1 || err.errors[0].text !== 'The build was canceled')
            throw err
        }
      }
    } finally {
      context.dispose()
    }
  },

  async bundleAvoidTDZ({ esbuild }) {
    var { outputFiles } = await esbuild.build({
      stdin: {
        contents: `
          class Foo {
            // The above line will be transformed into "var". However, the
            // symbol "Foo" must still be defined before the class body ends.
            static foo = new Foo
          }
          if (!(Foo.foo instanceof Foo))
            throw 'fail'
        `,
      },
      bundle: true,
      write: false,
    })
    assert.strictEqual(outputFiles.length, 1)
    new Function(outputFiles[0].text)()
  },

  async bundleTSAvoidTDZ({ esbuild }) {
    var { outputFiles } = await esbuild.build({
      stdin: {
        contents: `
          class Foo {
            // The above line will be transformed into "var". However, the
            // symbol "Foo" must still be defined before the class body ends.
            static foo = new Foo
          }
          if (!(Foo.foo instanceof Foo))
            throw 'fail'
        `,
        loader: 'ts',
      },
      bundle: true,
      write: false,
    })
    assert.strictEqual(outputFiles.length, 1)
    new Function(outputFiles[0].text)()
  },

  async bundleTSDecoratorAvoidTDZ({ esbuild }) {
    var { outputFiles } = await esbuild.build({
      stdin: {
        contents: `
          class Bar {}
          var oldFoo
          function swap(target) {
            oldFoo = target
            return Bar
          }
          @swap
          class Foo {
            bar() { return new Foo }
            static foo = new Foo
          }
          if (!(oldFoo.foo instanceof oldFoo))
            throw 'fail: foo'
          if (!(oldFoo.foo.bar() instanceof Bar))
            throw 'fail: bar'
        `,
        loader: 'ts',
      },
      bundle: true,
      write: false,
    })
    assert.strictEqual(outputFiles.length, 1)
    new Function(outputFiles[0].text)()
  },

  async automaticEntryPointOutputPathsWithDot({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.file.ts')
    const css = path.join(testDir, 'file.css')
    await writeFileAsync(input, `import './file.css'; console.log('test')`)
    await writeFileAsync(css, `body { color: red }`)
    var { outputFiles } = await esbuild.build({
      entryPoints: [input],
      outdir: testDir,
      bundle: true,
      write: false,
    })
    assert.strictEqual(outputFiles.length, 2)
    assert.strictEqual(outputFiles[0].path, path.join(testDir, 'in.file.js'))
    assert.strictEqual(outputFiles[1].path, path.join(testDir, 'in.file.css'))
  },

  async customEntryPointOutputPathsWithDot({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.file.ts')
    const css = path.join(testDir, 'file.css')
    await writeFileAsync(input, `import './file.css'; console.log('test')`)
    await writeFileAsync(css, `body { color: red }`)
    var { outputFiles } = await esbuild.build({
      entryPoints: {
        'out.test': input,
      },
      outdir: testDir,
      bundle: true,
      write: false,
    })
    assert.strictEqual(outputFiles.length, 2)
    assert.strictEqual(outputFiles[0].path, path.join(testDir, 'out.test.js'))
    assert.strictEqual(outputFiles[1].path, path.join(testDir, 'out.test.css'))
  },

  async customEntryPointOutputPathsRel({ esbuild, testDir }) {
    const input1 = path.join(testDir, 'in1.js')
    const input2 = path.join(testDir, 'in2.js')
    const output1 = 'out/1.cjs'
    const output2 = 'out/2.mjs'
    await writeFileAsync(input1, `console.log('in1')`)
    await writeFileAsync(input2, `console.log('in2')`)
    var { outputFiles } = await esbuild.build({
      entryPoints: {
        [output1]: input1,
        [output2]: input2,
      },
      entryNames: 'entry/[dir]/[hash]-[name]',
      outdir: testDir,
      write: false,
    })
    assert.strictEqual(outputFiles.length, 2)
    assert.strictEqual(outputFiles[0].path, path.join(testDir, 'entry', 'out', 'CXHWNMAN-1.cjs.js'))
    assert.strictEqual(outputFiles[1].path, path.join(testDir, 'entry', 'out', 'EYSNILNO-2.mjs.js'))
  },

  async customEntryPointOutputPathsAbs({ esbuild, testDir }) {
    const input1 = path.join(testDir, 'in1.js')
    const input2 = path.join(testDir, 'in2.js')
    const output1 = path.join(testDir, 'out/1')
    const output2 = path.join(testDir, 'out/2')
    await writeFileAsync(input1, `console.log('in1')`)
    await writeFileAsync(input2, `console.log('in2')`)
    var { outputFiles } = await esbuild.build({
      entryPoints: {
        [output1]: input1,
        [output2]: input2,
      },
      entryNames: 'entry/[dir]/[hash]-[name]',
      outdir: testDir,
      write: false,
    })
    assert.strictEqual(outputFiles.length, 2)
    assert.strictEqual(outputFiles[0].path, path.join(testDir, 'entry', 'out', 'TIORPBNU-1.js'))
    assert.strictEqual(outputFiles[1].path, path.join(testDir, 'entry', 'out', '3KY7NOSR-2.js'))
  },

  async customEntryPointOutputPathsDuplicates({ esbuild, testDir }) {
    const input1 = path.join(testDir, 'foo.js')
    const input2 = path.join(testDir, 'bar.css')
    await writeFileAsync(input1, `foo()`)
    await writeFileAsync(input2, `.bar {}`)
    var { outputFiles } = await esbuild.build({
      entryPoints: [
        { in: input1, out: 'abc' },
        { in: input2, out: 'abc' },
      ],
      outdir: testDir,
      write: false,
    })
    assert.strictEqual(outputFiles.length, 2)
    assert.strictEqual(outputFiles[0].path, path.join(testDir, 'abc.js'))
    assert.strictEqual(outputFiles[1].path, path.join(testDir, 'abc.css'))
  },

  async nodeColonPrefixImport({ esbuild }) {
    const tryTargetESM = async target => {
      const result = await esbuild.build({
        stdin: { contents: `import fs from 'node:fs'; import('node:fs'); fs()` },
        bundle: true,
        platform: 'node',
        target,
        format: 'esm',
        write: false,
      })
      const code = result.outputFiles[0].text
      return code.slice(code.indexOf(`// <stdin>\n`))
    }

    assert.strictEqual(await tryTargetESM('node14.13.1'), `// <stdin>\nimport fs from "node:fs";\nimport("node:fs");\nfs();\n`)
    assert.strictEqual(await tryTargetESM('node14.13.0'), `// <stdin>\nimport fs from "fs";\nimport("fs");\nfs();\n`)
    assert.strictEqual(await tryTargetESM('node13'), `// <stdin>\nimport fs from "fs";\nPromise.resolve().then(() => __toESM(__require("fs")));\nfs();\n`)
    assert.strictEqual(await tryTargetESM('node12.99'), `// <stdin>\nimport fs from "node:fs";\nimport("node:fs");\nfs();\n`)
    assert.strictEqual(await tryTargetESM('node12.20'), `// <stdin>\nimport fs from "node:fs";\nimport("node:fs");\nfs();\n`)
    assert.strictEqual(await tryTargetESM('node12.19'), `// <stdin>\nimport fs from "fs";\nPromise.resolve().then(() => __toESM(__require("fs")));\nfs();\n`)
  },

  async nodeColonPrefixRequire({ esbuild }) {
    const tryTargetESM = async target => {
      const result = await esbuild.build({
        stdin: { contents: `require('node:fs'); require.resolve('node:fs')` },
        bundle: true,
        platform: 'node',
        target,
        format: 'cjs',
        write: false,
      })
      const code = result.outputFiles[0].text
      return code.slice(code.indexOf(`// <stdin>\n`))
    }

    assert.strictEqual(await tryTargetESM('node16'), `// <stdin>\nrequire("node:fs");\nrequire.resolve("node:fs");\n`)
    assert.strictEqual(await tryTargetESM('node15.99'), `// <stdin>\nrequire("fs");\nrequire.resolve("fs");\n`)
    assert.strictEqual(await tryTargetESM('node15'), `// <stdin>\nrequire("fs");\nrequire.resolve("fs");\n`)
    assert.strictEqual(await tryTargetESM('node14.99'), `// <stdin>\nrequire("node:fs");\nrequire.resolve("node:fs");\n`)
    assert.strictEqual(await tryTargetESM('node14.18'), `// <stdin>\nrequire("node:fs");\nrequire.resolve("node:fs");\n`)
    assert.strictEqual(await tryTargetESM('node14.17'), `// <stdin>\nrequire("fs");\nrequire.resolve("fs");\n`)
  },

  async nodeColonPrefixImportTurnedIntoRequire({ esbuild }) {
    const tryTargetESM = async target => {
      const result = await esbuild.build({
        stdin: { contents: `import fs from 'node:fs'; import('node:fs'); fs()` },
        bundle: true,
        platform: 'node',
        target,
        format: 'cjs',
        write: false,
      })
      const code = result.outputFiles[0].text
      return code.slice(code.indexOf(`// <stdin>\n`))
    }

    assert.strictEqual(await tryTargetESM('node16'), `// <stdin>\nvar import_node_fs = __toESM(require("node:fs"));\nimport("node:fs");\n(0, import_node_fs.default)();\n`)
    assert.strictEqual(await tryTargetESM('node15.99'), `// <stdin>\nvar import_node_fs = __toESM(require("fs"));\nimport("fs");\n(0, import_node_fs.default)();\n`)
    assert.strictEqual(await tryTargetESM('node15'), `// <stdin>\nvar import_node_fs = __toESM(require("fs"));\nimport("fs");\n(0, import_node_fs.default)();\n`)
    assert.strictEqual(await tryTargetESM('node14.99'), `// <stdin>\nvar import_node_fs = __toESM(require("node:fs"));\nimport("node:fs");\n(0, import_node_fs.default)();\n`)
    assert.strictEqual(await tryTargetESM('node14.18'), `// <stdin>\nvar import_node_fs = __toESM(require("node:fs"));\nimport("node:fs");\n(0, import_node_fs.default)();\n`)
    assert.strictEqual(await tryTargetESM('node14.17'), `// <stdin>\nvar import_node_fs = __toESM(require("fs"));\nimport("fs");\n(0, import_node_fs.default)();\n`)
  },

  async zipFile({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const zip = path.join(testDir, 'test.zip')

    await writeFileAsync(entry, `
      import foo from './test.zip/foo.js'
      import bar from './test.zip/bar/bar.js'

      import __virtual__1 from './test.zip/__virtual__/ignored/0/foo.js'
      import __virtual__2 from './test.zip/ignored/__virtual__/ignored/1/foo.js'
      import __virtual__3 from './test.zip/__virtual__/ignored/1/test.zip/foo.js'

      import $$virtual1 from './test.zip/$$virtual/ignored/0/foo.js'
      import $$virtual2 from './test.zip/ignored/$$virtual/ignored/1/foo.js'
      import $$virtual3 from './test.zip/$$virtual/ignored/1/test.zip/foo.js'

      console.log({
        foo,
        bar,

        __virtual__1,
        __virtual__2,
        __virtual__3,

        $$virtual1,
        $$virtual2,
        $$virtual3,
      })
    `)

    // This uses the real file system instead of the mock file system so that
    // we can check that everything works as expected on Windows, which is not
    // a POSIX environment.
    await writeFileAsync(zip, Buffer.from(
      `UEsDBAoAAgAAAG1qCFUSAXosFQAAABUAAAAGABwAZm9vLmpzVVQJAAOeRfFioEXxYnV4C` +
      `wABBPUBAAAEFAAAAGV4cG9ydCBkZWZhdWx0ICdmb28nClBLAwQKAAIAAABzaghVwuDbLR` +
      `UAAAAVAAAACgAcAGJhci9iYXIuanNVVAkAA6lF8WKrRfFidXgLAAEE9QEAAAQUAAAAZXh` +
      `wb3J0IGRlZmF1bHQgJ2JhcicKUEsBAh4DCgACAAAAbWoIVRIBeiwVAAAAFQAAAAYAGAAA` +
      `AAAAAQAAAKSBAAAAAGZvby5qc1VUBQADnkXxYnV4CwABBPUBAAAEFAAAAFBLAQIeAwoAA` +
      `gAAAHNqCFXC4NstFQAAABUAAAAKABgAAAAAAAEAAACkgVUAAABiYXIvYmFyLmpzVVQFAA` +
      `OpRfFidXgLAAEE9QEAAAQUAAAAUEsFBgAAAAACAAIAnAAAAK4AAAAAAA==`, 'base64'))

    const value = await esbuild.build({
      entryPoints: [entry],
      bundle: true,
      write: false,
    })

    assert.strictEqual(value.outputFiles.length, 1)
    assert.strictEqual(value.outputFiles[0].text, `(() => {
  // scripts/.js-api-tests/zipFile/test.zip/foo.js
  var foo_default = "foo";

  // scripts/.js-api-tests/zipFile/test.zip/bar/bar.js
  var bar_default = "bar";

  // scripts/.js-api-tests/zipFile/test.zip/__virtual__/ignored/0/foo.js
  var foo_default2 = "foo";

  // scripts/.js-api-tests/zipFile/test.zip/ignored/__virtual__/ignored/1/foo.js
  var foo_default3 = "foo";

  // scripts/.js-api-tests/zipFile/test.zip/__virtual__/ignored/1/test.zip/foo.js
  var foo_default4 = "foo";

  // scripts/.js-api-tests/zipFile/test.zip/$$virtual/ignored/0/foo.js
  var foo_default5 = "foo";

  // scripts/.js-api-tests/zipFile/test.zip/ignored/$$virtual/ignored/1/foo.js
  var foo_default6 = "foo";

  // scripts/.js-api-tests/zipFile/test.zip/$$virtual/ignored/1/test.zip/foo.js
  var foo_default7 = "foo";

  // scripts/.js-api-tests/zipFile/entry.js
  console.log({
    foo: foo_default,
    bar: bar_default,
    __virtual__1: foo_default2,
    __virtual__2: foo_default3,
    __virtual__3: foo_default4,
    $$virtual1: foo_default5,
    $$virtual2: foo_default6,
    $$virtual3: foo_default7
  });
})();
`)
  },

  async yarnPnP_pnp_data_json({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const manifest = path.join(testDir, '.pnp.data.json')
    const leftPad = path.join(testDir, '.yarn', 'cache', 'left-pad-zip', 'node_modules', 'left-pad', 'index.js')

    await writeFileAsync(entry, `
      import leftPad from 'left-pad'
      console.log(leftPad())
    `)

    await writeFileAsync(manifest, `{
      "packageRegistryData": [
        [null, [
          [null, {
            "packageLocation": "./",
            "packageDependencies": [
              ["left-pad", "npm:1.3.0"]
            ],
            "linkType": "SOFT"
          }]
        ]],
        ["left-pad", [
          ["npm:1.3.0", {
            "packageLocation": "./.yarn/cache/left-pad-zip/node_modules/left-pad/",
            "packageDependencies": [
              ["left-pad", "npm:1.3.0"]
            ],
            "linkType": "HARD"
          }]
        ]]
      ]
    }`)

    await mkdirAsync(path.dirname(leftPad), { recursive: true })
    await writeFileAsync(leftPad, `export default function() {}`)

    const value = await esbuild.build({
      entryPoints: [entry],
      bundle: true,
      write: false,
      absWorkingDir: testDir,
    })

    assert.strictEqual(value.outputFiles.length, 1)
    assert.strictEqual(value.outputFiles[0].text, `(() => {
  // .yarn/cache/left-pad-zip/node_modules/left-pad/index.js
  function left_pad_default() {
  }

  // entry.js
  console.log(left_pad_default());
})();
`)
  },

  async yarnPnP_pnp_js_object_literal({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const manifest = path.join(testDir, '.pnp.js')
    const leftPad = path.join(testDir, '.yarn', 'cache', 'left-pad-zip', 'node_modules', 'left-pad', 'index.js')

    await writeFileAsync(entry, `
      import leftPad from 'left-pad'
      console.log(leftPad())
    `)

    await writeFileAsync(manifest, `#!/usr/bin/env node
      /* eslint-disable */

      try {
        Object.freeze({}).detectStrictMode = true;
      } catch (error) {
        throw new Error();
      }

      function $$SETUP_STATE(hydrateRuntimeState, basePath) {
        return hydrateRuntimeState({
          "packageRegistryData": [
            [null, [
              [null, {
                "packageLocation": "./",
                "packageDependencies": [
                  ["left-pad", "npm:1.3.0"]
                ],
                "linkType": "SOFT"
              }]
            ]],
            ["left-pad", [
              ["npm:1.3.0", {
                "packageLocation": "./.yarn/cache/left-pad-zip/node_modules/left-pad/",
                "packageDependencies": [
                  ["left-pad", "npm:1.3.0"]
                ],
                "linkType": "HARD"
              }]
            ]]
          ]
        })
      }
    `)

    await mkdirAsync(path.dirname(leftPad), { recursive: true })
    await writeFileAsync(leftPad, `export default function() {}`)

    const value = await esbuild.build({
      entryPoints: [entry],
      bundle: true,
      write: false,
      absWorkingDir: testDir,
    })

    assert.strictEqual(value.outputFiles.length, 1)
    assert.strictEqual(value.outputFiles[0].text, `(() => {
  // .yarn/cache/left-pad-zip/node_modules/left-pad/index.js
  function left_pad_default() {
  }

  // entry.js
  console.log(left_pad_default());
})();
`)
  },

  async yarnPnP_pnp_cjs_JSON_parse_string_literal({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const manifest = path.join(testDir, '.pnp.cjs')
    const leftPad = path.join(testDir, '.yarn', 'cache', 'left-pad-zip', 'node_modules', 'left-pad', 'index.js')

    await writeFileAsync(entry, `
      import leftPad from 'left-pad'
      console.log(leftPad())
    `)

    await writeFileAsync(manifest, `#!/usr/bin/env node
      /* eslint-disable */

      try {
        Object.freeze({}).detectStrictMode = true;
      } catch (error) {
        throw new Error();
      }

      function $$SETUP_STATE(hydrateRuntimeState, basePath) {
        return hydrateRuntimeState(JSON.parse('{\\
          "packageRegistryData": [\\
            [null, [\\
              [null, {\\
                "packageLocation": "./",\\
                "packageDependencies": [\\
                  ["left-pad", "npm:1.3.0"]\\
                ],\\
                "linkType": "SOFT"\\
              }]\\
            ]],\\
            ["left-pad", [\\
              ["npm:1.3.0", {\\
                "packageLocation": "./.yarn/cache/left-pad-zip/node_modules/left-pad/",\\
                "packageDependencies": [\\
                  ["left-pad", "npm:1.3.0"]\\
                ],\\
                "linkType": "HARD"\\
              }]\\
            ]]\\
          ]\\
        }'))
      }
    `)

    await mkdirAsync(path.dirname(leftPad), { recursive: true })
    await writeFileAsync(leftPad, `export default function() {}`)

    const value = await esbuild.build({
      entryPoints: [entry],
      bundle: true,
      write: false,
      absWorkingDir: testDir,
    })

    assert.strictEqual(value.outputFiles.length, 1)
    assert.strictEqual(value.outputFiles[0].text, `(() => {
  // .yarn/cache/left-pad-zip/node_modules/left-pad/index.js
  function left_pad_default() {
  }

  // entry.js
  console.log(left_pad_default());
})();
`)
  },

  async yarnPnP_pnp_cjs_JSON_parse_identifier({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const manifest = path.join(testDir, '.pnp.cjs')
    const leftPad = path.join(testDir, '.yarn', 'cache', 'left-pad-zip', 'node_modules', 'left-pad', 'index.js')

    await writeFileAsync(entry, `
      import leftPad from 'left-pad'
      console.log(leftPad())
    `)

    await writeFileAsync(manifest, `#!/usr/bin/env node
      /* eslint-disable */

      try {
        Object.freeze({}).detectStrictMode = true;
      } catch (error) {
        throw new Error();
      }

      const RAW_RUNTIME_STATE = '{\\
        "packageRegistryData": [\\
          [null, [\\
            [null, {\\
              "packageLocation": "./",\\
              "packageDependencies": [\\
                ["left-pad", "npm:1.3.0"]\\
              ],\\
              "linkType": "SOFT"\\
            }]\\
          ]],\\
          ["left-pad", [\\
            ["npm:1.3.0", {\\
              "packageLocation": "./.yarn/cache/left-pad-zip/node_modules/left-pad/",\\
              "packageDependencies": [\\
                ["left-pad", "npm:1.3.0"]\\
              ],\\
              "linkType": "HARD"\\
            }]\\
          ]]\\
        ]\\
      }'

      function $$SETUP_STATE(hydrateRuntimeState, basePath) {
        return hydrateRuntimeState(JSON.parse(RAW_RUNTIME_STATE))
      }
    `)

    await mkdirAsync(path.dirname(leftPad), { recursive: true })
    await writeFileAsync(leftPad, `export default function() {}`)

    const value = await esbuild.build({
      entryPoints: [entry],
      bundle: true,
      write: false,
      absWorkingDir: testDir,
    })

    assert.strictEqual(value.outputFiles.length, 1)
    assert.strictEqual(value.outputFiles[0].text, `(() => {
  // .yarn/cache/left-pad-zip/node_modules/left-pad/index.js
  function left_pad_default() {
  }

  // entry.js
  console.log(left_pad_default());
})();
`)
  },

  async yarnPnP_ignoreNestedManifests({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const foo = path.join(testDir, 'node_modules', 'foo', 'index.js')
    const bar = path.join(testDir, 'node_modules', 'bar', 'index.js')
    const manifest = path.join(testDir, '.pnp.data.json')

    await writeFileAsync(entry, `
      import foo from 'foo'
      console.log(foo)
    `)

    await mkdirAsync(path.dirname(foo), { recursive: true })
    await writeFileAsync(foo, `
      import bar from 'bar'
      export default 'foo' + bar
    `)

    await mkdirAsync(path.dirname(bar), { recursive: true })
    await writeFileAsync(bar, `
      export default 'bar'
    `)

    await writeFileAsync(manifest, `{
      "packageRegistryData": [
        [null, [
          [null, {
            "packageLocation": "./",
            "packageDependencies": [
              ["foo", "npm:1.0.0"],
              ["bar", "npm:1.0.0"]
            ],
            "linkType": "SOFT"
          }]
        ]],
        ["foo", [
          ["npm:1.0.0", {
            "packageLocation": "./__virtual__/whatever/0/node_modules/foo/",
            "packageDependencies": [
              ["bar", "npm:1.0.0"]
            ],
            "linkType": "HARD"
          }]
        ]],
        ["bar", [
          ["npm:1.0.0", {
            "packageLocation": "./__virtual__/whatever/0/node_modules/bar/",
            "packageDependencies": [],
            "linkType": "HARD"
          }]
        ]]
      ]
    }`)

    const value = await esbuild.build({
      entryPoints: [entry],
      bundle: true,
      write: false,
      absWorkingDir: testDir,
    })

    assert.strictEqual(value.outputFiles.length, 1)
    assert.strictEqual(value.outputFiles[0].text, `(() => {
  // __virtual__/whatever/0/node_modules/bar/index.js
  var bar_default = "bar";

  // __virtual__/whatever/0/node_modules/foo/index.js
  var foo_default = "foo" + bar_default;

  // entry.js
  console.log(foo_default);
})();
`)
  },

  async yarnPnP_tsconfig({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const tsconfigExtends = path.join(testDir, 'tsconfig.json')
    const tsconfigBase = path.join(testDir, 'foo', 'tsconfig.json')
    const manifest = path.join(testDir, '.pnp.data.json')

    await writeFileAsync(entry, `
      x **= 2
    `)

    await writeFileAsync(tsconfigExtends, `{
      "extends": "@scope/base/tsconfig.json",
    }`)

    await mkdirAsync(path.dirname(tsconfigBase), { recursive: true })
    await writeFileAsync(tsconfigBase, `{
      "compilerOptions": {
        "target": "ES5"
      }
    }`)

    await writeFileAsync(manifest, `{
      "packageRegistryData": [
        [null, [
          [null, {
            "packageLocation": "./",
            "packageDependencies": [
              ["@scope/base", "npm:1.0.0"]
            ],
            "linkType": "SOFT"
          }]
        ]],
        ["@scope/base", [
          ["npm:1.0.0", {
            "packageLocation": "./foo/",
            "packageDependencies": [],
            "linkType": "HARD"
          }]
        ]]
      ]
    }`)

    const value = await esbuild.build({
      entryPoints: [entry],
      bundle: true,
      write: false,
      absWorkingDir: testDir,
    })

    assert.strictEqual(value.outputFiles.length, 1)
    assert.strictEqual(value.outputFiles[0].text, `(() => {
  var __pow = Math.pow;

  // entry.js
  x = __pow(x, 2);
})();
`)
  },

  async yarnPnP_indexJs({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const fooIndex = path.join(testDir, 'node_modules', '@some', 'pkg', 'index.js')
    const fooFoo = path.join(testDir, 'node_modules', '@some', 'pkg', 'foo.js')
    const manifest = path.join(testDir, '.pnp.data.json')

    await writeFileAsync(entry, `
      import x from '@some/pkg'
      x()
    `)

    await mkdirAsync(path.dirname(fooIndex), { recursive: true })
    await writeFileAsync(fooIndex, `export default success`)

    await mkdirAsync(path.dirname(fooFoo), { recursive: true })
    await writeFileAsync(fooFoo, `failure!`)

    await writeFileAsync(manifest, `{
      "packageRegistryData": [
        [null, [
          [null, {
            "packageLocation": "./",
            "packageDependencies": [
              ["@some/pkg", "npm:1.0.0"]
            ],
            "linkType": "SOFT"
          }]
        ]],
        ["@some/pkg", [
          ["npm:1.0.0", {
            "packageLocation": "./node_modules/@some/pkg/",
            "packageDependencies": [],
            "linkType": "HARD"
          }]
        ]]
      ]
    }`)

    const value = await esbuild.build({
      entryPoints: [entry],
      bundle: true,
      write: false,
      absWorkingDir: testDir,
    })

    assert.strictEqual(value.outputFiles.length, 1)
    assert.strictEqual(value.outputFiles[0].text, `(() => {
  // node_modules/@some/pkg/index.js
  var pkg_default = success;

  // entry.js
  pkg_default();
})();
`)
  },

  async yarnPnP_depOfVirtual({ esbuild, testDir }) {
    const entry = path.join(testDir, 'entry.js')
    const pkg = path.join(testDir, '.yarn', 'cache', 'pkg-zip', 'node_modules', 'pkg', 'index.js')
    const dep = path.join(testDir, '.yarn', 'cache', 'dep-zip', 'node_modules', 'dep', 'index.js')
    const manifest = path.join(testDir, '.pnp.data.json')

    await writeFileAsync(entry, `import 'pkg'`)

    await mkdirAsync(path.dirname(pkg), { recursive: true })
    await writeFileAsync(pkg, `import 'dep'`)

    await mkdirAsync(path.dirname(dep), { recursive: true })
    await writeFileAsync(dep, `success()`)

    await writeFileAsync(manifest, `{
      "packageRegistryData": [
        [null, [
          [null, {
            "packageLocation": "./",
            "packageDependencies": [
              ["pkg", "virtual:some-path"]
            ],
            "linkType": "SOFT"
          }]
        ]],
        ["demo", [
          ["workspace:.", {
            "packageLocation": "./",
            "packageDependencies": [
              ["demo", "workspace:."],
              ["pkg", "virtual:some-path"]
            ],
            "linkType": "SOFT"
          }]
        ]],
        ["pkg", [
          ["npm:1.0.0", {
            "packageLocation": "./.yarn/cache/pkg-zip/node_modules/pkg/",
            "packageDependencies": [
              ["pkg", "npm:1.0.0"]
            ],
            "linkType": "SOFT"
          }],
          ["virtual:some-path", {
            "packageLocation": "./.yarn/__virtual__/pkg-virtual/0/cache/pkg-zip/node_modules/pkg/",
            "packageDependencies": [
              ["pkg", "virtual:some-path"],
              ["dep", "npm:1.0.0"]
            ],
            "linkType": "HARD"
          }]
        ]],
        ["dep", [
          ["npm:1.0.0", {
            "packageLocation": "./.yarn/cache/dep-zip/node_modules/dep/",
            "packageDependencies": [
              ["dep", "npm:1.0.0"]
            ],
            "linkType": "HARD"
          }]
        ]]
      ]
    }`)

    const value = await esbuild.build({
      entryPoints: [entry],
      bundle: true,
      write: false,
      absWorkingDir: testDir,
    })

    assert.strictEqual(value.outputFiles.length, 1)
    assert.strictEqual(value.outputFiles[0].text, `(() => {
  // .yarn/cache/dep-zip/node_modules/dep/index.js
  success();
})();
`)
  },
}

function fetch(host, port, path, { headers, method = 'GET' } = {}) {
  return new Promise((resolve, reject) => {
    http.request({ method, host, port, path, headers }, res => {
      const chunks = []
      res.on('data', chunk => chunks.push(chunk))
      res.on('end', () => {
        const content = Buffer.concat(chunks)
        if (res.statusCode < 200 || res.statusCode > 299) {
          const error = new Error(`${res.statusCode} when fetching "${path}": ${content}`)
          error.statusCode = res.statusCode
          reject(error)
        } else {
          content.headers = res.headers
          resolve(content)
        }
      })
    }).on('error', reject).end()
  })
}

function fetchHTTPS(host, port, path, { certfile }) {
  return new Promise((resolve, reject) => {
    const checkServerIdentity = (hostname, cert) => {
      // I'm not sure why node seems to always reject the host "127.0.0.1"
      assert.strictEqual(hostname, '127.0.0.1')
      assert.strictEqual(cert.subject.CN, '127.0.0.1')
    }
    const ca = [fs.readFileSync(certfile, 'utf8')]
    https.get({ host, port, path, ca, checkServerIdentity }, res => {
      const chunks = []
      res.on('data', chunk => chunks.push(chunk))
      res.on('end', () => {
        const content = Buffer.concat(chunks)
        if (res.statusCode < 200 || res.statusCode > 299) {
          const error = new Error(`${res.statusCode} when fetching "${path}": ${content}`)
          error.statusCode = res.statusCode
          reject(error)
        } else {
          content.headers = res.headers
          resolve(content)
        }
      })
    }).on('error', reject)
  })
}

const makeEventStream = (host, port, path) => {
  return new Promise((resolve, reject) => {
    const headers = {
      'Accept': 'text/event-stream',
    }

    http.get({ host, port, path, headers }, res => {
      if (res.statusCode !== 200) {
        reject(new Error(`${res.statusCode} when fetching "${path}"`))
        return
      }

      const stream = new events.EventEmitter
      let buffer = ''

      res.on('data', chunk => {
        buffer += chunk
        while (true) {
          const index = buffer.indexOf('\n\n')
          if (index < 0) break

          const lines = buffer.slice(0, index).split('\n')
          const fields = Object.create(null)
          buffer = buffer.slice(index + 2)

          for (const line of lines) {
            const colon = line.indexOf(':')
            if (colon >= 0) {
              const key = line.slice(0, colon).trim()
              const value = line.slice(colon + 1).trim()
              fields[key] = value
            }
          }

          if ('data' in fields) {
            stream.emit('event' in fields ? fields.event : 'message', fields)
          }
        }
      })

      res.on('close', () => stream.emit('close'))

      stream.destroy = () => res.destroy()

      stream.waitFor = name => new Promise((resolve, reject) => {
        const timeout = setTimeout(() => {
          reject(new Error('Timeout after 30 seconds'))
          stream.destroy()
        }, 30 * 1000)

        stream.once(name, value => {
          clearTimeout(timeout)
          resolve(value)
        })
      })

      resolve(stream)
    }).on('error', reject)
  })
}

const makeRebuildUntilPlugin = () => {
  let onEnd

  return {
    rebuildUntil: (mutator, condition) => new Promise((resolve, reject) => {
      let timeout = setTimeout(() => reject(new Error('Timeout after 30 seconds')), 30 * 1000)
      onEnd = result => {
        try { if (result && condition(result)) clearTimeout(timeout), resolve(result) }
        catch (e) { clearTimeout(timeout), reject(e) }
      }
      mutator()
    }),

    plugin: {
      name: 'rebuildUntil',
      setup(build) {
        build.onEnd(result => onEnd(result))
      },
    },
  }
}

let watchTests = {
  async watchTwice({ esbuild }) {
    const context = await esbuild.context({})
    try {
      // Watch once
      await context.watch()

      // Watch twice
      try {
        await context.watch()
        throw new Error('Expected an error to be thrown')
      } catch (err) {
        assert.strictEqual(err.message, 'Watch mode has already been enabled')
      }
    } finally {
      await context.dispose()
    }
  },

  async watchEditSession({ esbuild, testDir }) {
    const srcDir = path.join(testDir, 'src')
    const outfile = path.join(testDir, 'out.js')
    const input = path.join(srcDir, 'in.js')
    await mkdirAsync(srcDir, { recursive: true })
    await writeFileAsync(input, `throw 1`)

    const { rebuildUntil, plugin } = makeRebuildUntilPlugin()
    const context = await esbuild.context({
      entryPoints: [input],
      outfile,
      format: 'esm',
      logLevel: 'silent',
      plugins: [plugin],
    })

    try {
      const result = await rebuildUntil(
        () => context.watch(),
        () => true,
      )
      assert.strictEqual(result.outputFiles, void 0)
      assert.strictEqual(result.errors.length, 0)
      assert.strictEqual(await readFileAsync(outfile, 'utf8'), 'throw 1;\n')

      // First rebuild: edit
      {
        const result2 = await rebuildUntil(
          () => writeFileAtomic(input, `throw 2`),
          () => fs.readFileSync(outfile, 'utf8') === 'throw 2;\n',
        )
        assert.strictEqual(result2.outputFiles, void 0)
        assert.strictEqual(result2.errors.length, 0)
        assert.strictEqual(await readFileAsync(outfile, 'utf8'), 'throw 2;\n')
      }

      // Second rebuild: edit
      {
        const result2 = await rebuildUntil(
          () => writeFileAtomic(input, `throw 3`),
          () => fs.readFileSync(outfile, 'utf8') === 'throw 3;\n',
        )
        assert.strictEqual(result2.outputFiles, void 0)
        assert.strictEqual(result2.errors.length, 0)
        assert.strictEqual(await readFileAsync(outfile, 'utf8'), 'throw 3;\n')
      }

      // Third rebuild: syntax error
      {
        const result2 = await rebuildUntil(
          () => writeFileAtomic(input, `throw 1 2`),
          result => result.errors.length > 0,
        )
        assert.strictEqual(result2.errors.length, 1)
        assert.strictEqual(result2.errors[0].text, 'Expected ";" but found "2"')
        assert.strictEqual(await readFileAsync(outfile, 'utf8'), 'throw 3;\n')
      }

      // Fourth rebuild: edit
      {
        const result2 = await rebuildUntil(
          () => writeFileAtomic(input, `throw 4`),
          () => fs.readFileSync(outfile, 'utf8') === 'throw 4;\n',
        )
        assert.strictEqual(result2.outputFiles, void 0)
        assert.strictEqual(result2.errors.length, 0)
        assert.strictEqual(await readFileAsync(outfile, 'utf8'), 'throw 4;\n')
      }

      // Fifth rebuild: delete
      {
        const result2 = await rebuildUntil(
          () => fs.promises.unlink(input),
          result => result.errors.length > 0,
        )
        assert.strictEqual(result2.errors.length, 1)
        assert.strictEqual(await readFileAsync(outfile, 'utf8'), 'throw 4;\n')
        assert.strictEqual(await readFileAsync(outfile, 'utf8'), 'throw 4;\n')
      }

      // Sixth rebuild: restore
      {
        const result2 = await rebuildUntil(
          () => writeFileAtomic(input, `throw 5`),
          () => fs.readFileSync(outfile, 'utf8') === 'throw 5;\n',
        )
        assert.strictEqual(result2.outputFiles, void 0)
        assert.strictEqual(result2.errors.length, 0)
        assert.strictEqual(await readFileAsync(outfile, 'utf8'), 'throw 5;\n')
      }
    } finally {
      await context.dispose()
    }
  },

  async watchWriteFalse({ esbuild, testDir }) {
    const srcDir = path.join(testDir, 'src')
    const outdir = path.join(testDir, 'out')
    const input = path.join(srcDir, 'in.js')
    const output = path.join(outdir, 'in.js')
    await mkdirAsync(srcDir, { recursive: true })
    await writeFileAsync(input, `throw 1`)

    const { rebuildUntil, plugin } = makeRebuildUntilPlugin()
    const context = await esbuild.context({
      entryPoints: [input],
      outdir,
      format: 'esm',
      logLevel: 'silent',
      write: false,
      plugins: [plugin],
    })

    try {
      const result = await rebuildUntil(
        () => context.watch(),
        () => true,
      )
      assert.strictEqual(result.errors.length, 0)
      assert.strictEqual(result.outputFiles[0].text, 'throw 1;\n')
      assert.strictEqual(fs.existsSync(output), false)

      // First rebuild: edit
      {
        const result2 = await rebuildUntil(
          () => writeFileAtomic(input, `throw 2`),
          res => res.outputFiles[0].text === 'throw 2;\n',
        )
        assert.strictEqual(result2.errors.length, 0)
        assert.strictEqual(fs.existsSync(output), false)
      }

      // Second rebuild: edit
      {
        const result2 = await rebuildUntil(
          () => writeFileAtomic(input, `throw 3`),
          res => res.outputFiles[0].text === 'throw 3;\n',
        )
        assert.strictEqual(result2.errors.length, 0)
        assert.strictEqual(fs.existsSync(output), false)
      }
    } finally {
      await context.dispose()
    }
  },

  async watchMetafile({ esbuild, testDir }) {
    const srcDir = path.join(testDir, 'src')
    const outdir = path.join(testDir, 'out')
    const input = path.join(srcDir, 'in.js')
    const output = path.join(outdir, 'in.js')
    await mkdirAsync(srcDir, { recursive: true })
    await writeFileAsync(input, `foo()`)

    const { rebuildUntil, plugin } = makeRebuildUntilPlugin()
    const context = await esbuild.context({
      entryPoints: [input],
      outdir,
      format: 'esm',
      logLevel: 'silent',
      metafile: true,
      plugins: [plugin],
    })

    try {
      const result = await rebuildUntil(
        () => context.watch(),
        () => true,
      )
      assert.strictEqual(result.errors.length, 0)
      const relInput = path.relative(process.cwd(), input).split(path.sep).join('/')
      assert.strictEqual(result.metafile.inputs[relInput].bytes, 5)
      assert.strictEqual(await readFileAsync(output, 'utf8'), 'foo();\n')

      // Rebuild and check that the metafile has been updated
      {
        const result2 = await rebuildUntil(
          () => writeFileAtomic(input, `foo(123)`),
          () => fs.readFileSync(output, 'utf8') === 'foo(123);\n',
        )
        assert.strictEqual(result2.errors.length, 0)
        assert.strictEqual(result2.metafile.inputs[relInput].bytes, 8)
      }
    } finally {
      await context.dispose()
    }
  },

  async watchMangleCache({ esbuild, testDir }) {
    const srcDir = path.join(testDir, 'src')
    const outdir = path.join(testDir, 'out')
    const input = path.join(srcDir, 'in.js')
    const output = path.join(outdir, 'in.js')
    await mkdirAsync(srcDir, { recursive: true })
    await writeFileAsync(input, `foo()`)

    const { rebuildUntil, plugin } = makeRebuildUntilPlugin()
    const context = await esbuild.context({
      entryPoints: [input],
      outdir,
      format: 'esm',
      logLevel: 'silent',
      mangleProps: /./,
      mangleCache: {},
      plugins: [plugin],
    })

    try {
      const result = await rebuildUntil(
        () => context.watch(),
        () => true,
      )
      assert.strictEqual(result.errors.length, 0)
      assert.strictEqual(JSON.stringify(result.mangleCache), '{}')

      // Rebuild and check that the mangle cache has been updated
      {
        const result2 = await rebuildUntil(
          () => writeFileAtomic(input, `foo(bar.baz)`),
          () => fs.readFileSync(output, 'utf8') === 'foo(bar.a);\n',
        )
        assert.strictEqual(result2.errors.length, 0)
        assert.strictEqual(JSON.stringify(result2.mangleCache), '{"baz":"a"}')
      }
    } finally {
      await context.dispose()
    }
  },
}

let serveTests = {
  async serveTwice({ esbuild }) {
    const context = await esbuild.context({})
    try {
      // Serve once
      await context.serve()

      // Serve twice
      try {
        await context.serve()
        throw new Error('Expected an error to be thrown')
      } catch (err) {
        assert.strictEqual(err.message, 'Serve mode has already been enabled')
      }
    } finally {
      await context.dispose()
    }
  },

  async serveWatch({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.ts')
    const outfile = path.join(testDir, 'out.js')

    const { rebuildUntil, plugin } = makeRebuildUntilPlugin()
    const context = await esbuild.context({
      entryPoints: [input],
      outfile,
      logLevel: 'silent',
      plugins: [plugin],
    })

    try {
      await context.watch()
      const server = await context.serve({
        host: '127.0.0.1',
      })

      // Try fetching the non-existent file
      try {
        await fetch(server.host, server.port, '/' + path.basename(outfile))
        throw new Error('Expected an error to be thrown')
      } catch (err) {
        if (err.statusCode !== 503) throw err
      }

      // Check that watch mode works
      for (let i = 0; i < 5; i++) {
        const result = await rebuildUntil(
          () => writeFileAtomic(input, `throw ${i}`),
          () => fs.readFileSync(outfile, 'utf8') === `throw ${i};\n`,
        )
        assert.strictEqual(result.errors.length, 0)
        assert.strictEqual(fs.readFileSync(outfile, 'utf8'), `throw ${i};\n`)
      }

      // Try fetching the file now
      const data = await fetch(server.host, server.port, '/' + path.basename(outfile))
      assert.strictEqual(data.toString(), 'throw 4;\n')
    } finally {
      await context.dispose()
    }
  },

  async serveBasic({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    await writeFileAsync(input, `console.log(123)`)

    let onRequest;

    const context = await esbuild.context({
      entryPoints: [input],
      format: 'esm',
      outdir: testDir,
      write: false,
    });
    try {
      const result = await context.serve({
        host: '127.0.0.1',
        onRequest: args => onRequest(args),
      })
      assert.strictEqual(result.host, '127.0.0.1');
      assert.strictEqual(typeof result.port, 'number');

      // GET /in.js
      {
        const singleRequestPromise = new Promise(resolve => { onRequest = resolve });
        const buffer = await fetch(result.host, result.port, '/in.js')
        assert.strictEqual(buffer.toString(), `console.log(123);\n`);
        assert.strictEqual(fs.readFileSync(input, 'utf8'), `console.log(123)`)

        let singleRequest = await singleRequestPromise;
        assert.strictEqual(singleRequest.method, 'GET');
        assert.strictEqual(singleRequest.path, '/in.js');
        assert.strictEqual(singleRequest.status, 200);
        assert.strictEqual(typeof singleRequest.remoteAddress, 'string');
        assert.strictEqual(typeof singleRequest.timeInMS, 'number');
      }

      // HEAD /in.js
      {
        const singleRequestPromise = new Promise(resolve => { onRequest = resolve });
        const buffer = await fetch(result.host, result.port, '/in.js', { method: 'HEAD' })
        assert.strictEqual(buffer.toString(), ``); // HEAD omits the content
        assert.strictEqual(fs.readFileSync(input, 'utf8'), `console.log(123)`)

        let singleRequest = await singleRequestPromise;
        assert.strictEqual(singleRequest.method, 'HEAD');
        assert.strictEqual(singleRequest.path, '/in.js');
        assert.strictEqual(singleRequest.status, 200);
        assert.strictEqual(typeof singleRequest.remoteAddress, 'string');
        assert.strictEqual(typeof singleRequest.timeInMS, 'number');
      }
    } finally {
      await context.dispose();
    }
  },

  async serveBasicHTTPS({ esbuild, testDir }) {
    const run = command => new Promise((resolve, reject) => {
      child_process.execFile(command.shift(), command, (error, stdout, stderr) => {
        if (error) reject(error)
        else resolve()
      })
    })

    try {
      await run(['which', 'openssl'])
    } catch {
      console.warn('Skipping HTTPS tests because the "openssl" command was not found')
      return
    }

    const keyfile = path.join(testDir, 'key.pem')
    const certfile = path.join(testDir, 'cert.pem')
    await run(['openssl', 'req', '-x509', '-newkey', 'rsa:4096', '-keyout', keyfile, '-out', certfile, '-days', '9999', '-nodes', '-subj', '/CN=127.0.0.1'])

    const input = path.join(testDir, 'in.js')
    await writeFileAsync(input, `console.log(123)`)

    let onRequest;
    let singleRequestPromise = new Promise(resolve => {
      onRequest = resolve;
    });

    const context = await esbuild.context({
      entryPoints: [input],
      format: 'esm',
      outdir: testDir,
      write: false,
    });
    try {
      const result = await context.serve({
        host: '127.0.0.1',
        keyfile,
        certfile,
        onRequest,
      })
      assert.strictEqual(result.host, '127.0.0.1');
      assert.strictEqual(typeof result.port, 'number');

      const buffer = await fetchHTTPS(result.host, result.port, '/in.js', { certfile })
      assert.strictEqual(buffer.toString(), `console.log(123);\n`);
      assert.strictEqual(fs.readFileSync(input, 'utf8'), `console.log(123)`)

      let singleRequest = await singleRequestPromise;
      assert.strictEqual(singleRequest.method, 'GET');
      assert.strictEqual(singleRequest.path, '/in.js');
      assert.strictEqual(singleRequest.status, 200);
      assert.strictEqual(typeof singleRequest.remoteAddress, 'string');
      assert.strictEqual(typeof singleRequest.timeInMS, 'number');
    } finally {
      await context.dispose();
    }
  },

  async serveOutfile({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const outfile = path.join(testDir, 'out.js')
    await writeFileAsync(input, `console.log(123)`)

    let onRequest;
    let singleRequestPromise = new Promise(resolve => {
      onRequest = resolve;
    });

    const context = await esbuild.context({
      entryPoints: [input],
      format: 'esm',
      outfile,
    });
    try {
      const result = await context.serve({
        host: '127.0.0.1',
        onRequest,
      })
      assert.strictEqual(result.host, '127.0.0.1');
      assert.strictEqual(typeof result.port, 'number');

      const buffer = await fetch(result.host, result.port, '/out.js')
      assert.strictEqual(buffer.toString(), `console.log(123);\n`);

      let singleRequest = await singleRequestPromise;
      assert.strictEqual(singleRequest.method, 'GET');
      assert.strictEqual(singleRequest.path, '/out.js');
      assert.strictEqual(singleRequest.status, 200);
      assert.strictEqual(typeof singleRequest.remoteAddress, 'string');
      assert.strictEqual(typeof singleRequest.timeInMS, 'number');

      try {
        await fetch(result.host, result.port, '/in.js')
        throw new Error('Expected a 404 error for "/in.js"')
      } catch (err) {
        if (err.message !== '404 when fetching "/in.js": 404 - Not Found')
          throw err
      }
    } finally {
      await context.dispose();
    }
  },

  async serveWithFallbackDir({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const wwwDir = path.join(testDir, 'www')
    const index = path.join(wwwDir, 'index.html')
    await mkdirAsync(wwwDir, { recursive: true })
    await writeFileAsync(input, `console.log(123)`)
    await writeFileAsync(index, `<!doctype html>`)

    let onRequest;
    let nextRequestPromise;

    (function generateNewPromise() {
      nextRequestPromise = new Promise(resolve => {
        onRequest = args => {
          generateNewPromise();
          resolve(args);
        };
      });
    })();

    const context = await esbuild.context({
      entryPoints: [input],
      format: 'esm',
      outdir: wwwDir,
      write: false,
    });
    try {
      const result = await context.serve({
        host: '127.0.0.1',
        onRequest: args => onRequest(args),
        servedir: wwwDir,
      })
      assert.strictEqual(result.host, '127.0.0.1');
      assert.strictEqual(typeof result.port, 'number');

      let promise, buffer, req;

      promise = nextRequestPromise;
      buffer = await fetch(result.host, result.port, '/in.js')
      assert.strictEqual(buffer.toString(), `console.log(123);\n`);
      assert.strictEqual(fs.existsSync(path.join(wwwDir, path.basename(input))), false);
      req = await promise;
      assert.strictEqual(req.method, 'GET');
      assert.strictEqual(req.path, '/in.js');
      assert.strictEqual(req.status, 200);
      assert.strictEqual(typeof req.remoteAddress, 'string');
      assert.strictEqual(typeof req.timeInMS, 'number');

      promise = nextRequestPromise;
      buffer = await fetch(result.host, result.port, '/')
      assert.strictEqual(buffer.toString(), `<!doctype html>`);
      req = await promise;
      assert.strictEqual(req.method, 'GET');
      assert.strictEqual(req.path, '/');
      assert.strictEqual(req.status, 200);
      assert.strictEqual(typeof req.remoteAddress, 'string');
      assert.strictEqual(typeof req.timeInMS, 'number');
    } finally {
      await context.dispose();
    }
  },

  async serveWithFallbackDirAndSiblingOutputDir({ esbuild, testDir }) {
    const context = await esbuild.context({
      entryPoints: [path.join(testDir, 'in.js')],
      outdir: 'out',
    });
    try {
      await context.serve({
        servedir: 'www',
      });
      throw new Error('Expected an error to be thrown')
    } catch (e) {
      assert.strictEqual(e.message, `Output directory "out" must be contained in serve directory "www"`)
    } finally {
      await context.dispose();
    }
  },

  async serveWithFallbackDirAndParentOutputDir({ esbuild, testDir }) {
    const context = await esbuild.context({
      entryPoints: [path.join(testDir, 'in.js')],
      outdir: testDir,
      absWorkingDir: testDir,
    });
    try {
      await context.serve({
        servedir: path.join(testDir, 'www'),
      })
      throw new Error('Expected an error to be thrown')
    } catch (e) {
      assert.strictEqual(e.message, `Output directory "." must be contained in serve directory "www"`)
    } finally {
      await context.dispose();
    }
  },

  async serveWithFallbackDirAndOutputDir({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const outputDir = path.join(testDir, 'www/out')
    const wwwDir = path.join(testDir, 'www')
    const index = path.join(wwwDir, 'index.html')
    await mkdirAsync(wwwDir, { recursive: true })
    await writeFileAsync(input, `console.log(123)`)
    await writeFileAsync(index, `<!doctype html>`)

    let onRequest;
    let nextRequestPromise;

    (function generateNewPromise() {
      nextRequestPromise = new Promise(resolve => {
        onRequest = args => {
          generateNewPromise();
          resolve(args);
        };
      });
    })();

    const context = await esbuild.context({
      entryPoints: [input],
      format: 'esm',
      outdir: outputDir,
    })
    try {
      const result = await context.serve({
        host: '127.0.0.1',
        onRequest: args => onRequest(args),
        servedir: wwwDir,
      })
      assert.strictEqual(result.host, '127.0.0.1');
      assert.strictEqual(typeof result.port, 'number');

      let promise, buffer, req;

      promise = nextRequestPromise;
      buffer = await fetch(result.host, result.port, '/out/in.js')
      assert.strictEqual(buffer.toString(), `console.log(123);\n`);
      req = await promise;
      assert.strictEqual(req.method, 'GET');
      assert.strictEqual(req.path, '/out/in.js');
      assert.strictEqual(req.status, 200);
      assert.strictEqual(typeof req.remoteAddress, 'string');
      assert.strictEqual(typeof req.timeInMS, 'number');

      promise = nextRequestPromise;
      buffer = await fetch(result.host, result.port, '/')
      assert.strictEqual(buffer.toString(), `<!doctype html>`);
      req = await promise;
      assert.strictEqual(req.method, 'GET');
      assert.strictEqual(req.path, '/');
      assert.strictEqual(req.status, 200);
      assert.strictEqual(typeof req.remoteAddress, 'string');
      assert.strictEqual(typeof req.timeInMS, 'number');

      // Make sure the output directory prefix requires a slash separator
      promise = nextRequestPromise;
      try {
        await fetch(result.host, result.port, '/outin.js')
        throw new Error('Expected an error to be thrown')
      } catch (err) {
        if (err.statusCode !== 404) throw err
      }
      req = await promise;
      assert.strictEqual(req.method, 'GET');
      assert.strictEqual(req.path, '/outin.js');
      assert.strictEqual(req.status, 404);
      assert.strictEqual(typeof req.remoteAddress, 'string');
      assert.strictEqual(typeof req.timeInMS, 'number');
    } finally {
      await context.dispose();
    }
  },

  async serveWithFallbackDirNoEntryPoints({ esbuild, testDir }) {
    const index = path.join(testDir, 'index.html')
    await writeFileAsync(index, `<!doctype html>`)

    let onRequest;
    let nextRequestPromise;

    (function generateNewPromise() {
      nextRequestPromise = new Promise(resolve => {
        onRequest = args => {
          generateNewPromise();
          resolve(args);
        };
      });
    })();

    const context = await esbuild.context({});
    try {
      const result = await context.serve({
        host: '127.0.0.1',
        onRequest: args => onRequest(args),
        servedir: testDir,
      })
      assert.strictEqual(result.host, '127.0.0.1');
      assert.strictEqual(typeof result.port, 'number');

      let promise, buffer, req;

      promise = nextRequestPromise;
      buffer = await fetch(result.host, result.port, '/')
      assert.strictEqual(buffer.toString(), `<!doctype html>`);
      req = await promise;
      assert.strictEqual(req.method, 'GET');
      assert.strictEqual(req.path, '/');
      assert.strictEqual(req.status, 200);
      assert.strictEqual(typeof req.remoteAddress, 'string');
      assert.strictEqual(typeof req.timeInMS, 'number');

      // Check that removing the file removes it from the directory listing (i.e. the
      // "fs.FS" object in Go does not cache the result of calling "ReadDirectory")
      await fs.promises.unlink(index)
      promise = nextRequestPromise;
      buffer = await fetch(result.host, result.port, '/')
      assert.notStrictEqual(buffer.toString(), '<!doctype html>')
      req = await promise;
      assert.strictEqual(req.method, 'GET');
      assert.strictEqual(req.path, '/');
      assert.strictEqual(req.status, 200);
      assert.strictEqual(typeof req.remoteAddress, 'string');
      assert.strictEqual(typeof req.timeInMS, 'number');
    } finally {
      await context.dispose();
    }
  },

  async serveRange({ esbuild, testDir }) {
    const big = path.join(testDir, 'big.txt')
    const byteCount = 16 * 1024 * 1024
    const buffer = require('crypto').randomBytes(byteCount)
    await writeFileAsync(big, buffer)

    const context = await esbuild.context({});
    try {
      const result = await context.serve({
        host: '127.0.0.1',
        servedir: testDir,
      })

      // Test small to big ranges
      const minLength = 1
      const maxLength = buffer.length

      for (let i = 0, n = 16; i < n; i++) {
        const length = Math.round(minLength + (maxLength - minLength) * i / (n - 1))
        const start = Math.floor(Math.random() * (buffer.length - length))
        const headers = {
          // Subtract 1 because range headers are inclusive on both ends
          Range: `bytes=${start}-${start + length - 1}`,
        }
        const fetched = await fetch(result.host, result.port, '/big.txt', { headers })
        delete fetched.headers.date // This changes every time
        delete fetched.headers.connection // Node v19+ no longer sends this
        const expected = buffer.slice(start, start + length)
        expected.headers = {
          'access-control-allow-origin': '*',
          'content-length': `${length}`,
          'content-range': `bytes ${start}-${start + length - 1}/${byteCount}`,
          'content-type': 'application/octet-stream',
        }
        assert.deepStrictEqual(fetched, expected)
      }
    } finally {
      await context.dispose();
    }
  },

  async serveWatchCopyLoaderWithEntryPoint({ esbuild, testDir }) {
    const ts = path.join(testDir, 'script.ts')
    const html = path.join(testDir, 'index.html')
    const outdir = path.join(testDir, 'out')
    await writeFileAsync(ts, `console.log(123)`)
    await writeFileAsync(html, `<script src=script.js></script>`)

    const context = await esbuild.context({
      entryPoints: [ts, html],
      outdir,
      loader: { '.html': 'copy' },
    });
    try {
      const server = await context.serve({
        host: '127.0.0.1',
        servedir: testDir,
      })
      await context.watch()

      const js = await fetch(server.host, server.port, '/out/script.js')
      assert.strictEqual(js.toString(), `console.log(123);\n`);

      const explicitHTML = await fetch(server.host, server.port, '/out/index.html')
      assert.strictEqual(explicitHTML.toString(), `<script src=script.js></script>`);

      // The server should support implicit "index.html" extensions on entry point files
      const implicitHTML = await fetch(server.host, server.port, '/out/')
      assert.strictEqual(implicitHTML.toString(), `<script src=script.js></script>`);

      // Make a change to the HTML
      await writeFileAsync(html, `<!DOCTYPE html><script src=script.js></script>`)

      // Wait for watch mode to rebuild
      const start = Date.now()
      while (true) {
        if (Date.now() - start > 30 * 1000) throw new Error('Timeout after 30 seconds')
        const toCheck = fs.readFileSync(path.join(outdir, 'index.html'), 'utf8')
        if (toCheck === `<!DOCTYPE html><script src=script.js></script>`) break
      }
    } finally {
      await context.dispose();
    }
  },

  async serveWithoutServedirWatchLiveReload({ esbuild, testDir }) {
    const js = path.join(testDir, 'app.js')
    const css = path.join(testDir, 'app.css')
    const outdir = path.join(testDir, 'out')
    await writeFileAsync(css, ``)

    let endPromise
    const context = await esbuild.context({
      entryPoints: [js],
      outdir,
      bundle: true,
      logLevel: 'silent',
    });

    try {
      const server = await context.serve({
        host: '127.0.0.1',
      })
      const stream = await makeEventStream(server.host, server.port, '/esbuild')
      await context.rebuild().then(
        () => Promise.reject(new Error('Expected an error to be thrown')),
        () => { /* Ignore the build error due to the missing JS file */ },
      )

      // Event 1: a new JavaScript file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(js, ``)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: ['/app.js'], removed: [], updated: [] })

      // Event 2: edit the JavaScript file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(js, `foo()`)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: [], removed: [], updated: ['/app.js'] })

      // Event 3: a new CSS file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(js, `import "./app.css"; foo()`)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: ['/app.css'], removed: [], updated: [] })

      // Event 4: edit the CSS file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(css, `a { color: red }`)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: [], removed: [], updated: ['/app.css'] })

      // Event 5: remove the CSS file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(js, `bar()`)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: [], removed: ['/app.css'], updated: ['/app.js'] })

      // Wait for the stream to end once we call "dispose()" below
      endPromise = stream.waitFor('close')
    }

    finally {
      await context.dispose();
    }

    // This stream should end once "dispose()" is called above
    await endPromise
  },

  async serveWithServedirWatchLiveReload({ esbuild, testDir }) {
    const js = path.join(testDir, 'app.js')
    const css = path.join(testDir, 'app.css')
    const outdir = path.join(testDir, 'out')
    await writeFileAsync(css, ``)

    let endPromise
    const context = await esbuild.context({
      entryPoints: [js],
      outdir,
      bundle: true,
      logLevel: 'silent',
    });

    try {
      const server = await context.serve({
        host: '127.0.0.1',
        servedir: testDir,
      })
      const stream = await makeEventStream(server.host, server.port, '/esbuild')
      await context.rebuild().then(
        () => Promise.reject(new Error('Expected an error to be thrown')),
        () => { /* Ignore the build error due to the missing JS file */ },
      )

      // Event 1: a new JavaScript file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(js, ``)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: ['/out/app.js'], removed: [], updated: [] })

      // Event 2: edit the JavaScript file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(js, `foo()`)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: [], removed: [], updated: ['/out/app.js'] })

      // Event 3: a new CSS file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(js, `import "./app.css"; foo()`)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: ['/out/app.css'], removed: [], updated: [] })

      // Event 4: edit the CSS file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(css, `a { color: red }`)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: [], removed: [], updated: ['/out/app.css'] })

      // Event 5: remove the CSS file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(js, `bar()`)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: [], removed: ['/out/app.css'], updated: ['/out/app.js'] })

      // Wait for the stream to end once we call "dispose()" below
      endPromise = stream.waitFor('close')
    }

    finally {
      await context.dispose();
    }

    // This stream should end once "dispose()" is called above
    await endPromise
  },

  async serveWithoutServedirWatchLiveReloadPublicPath({ esbuild, testDir }) {
    const js = path.join(testDir, 'app.js')
    const css = path.join(testDir, 'app.css')
    const outdir = path.join(testDir, 'out')
    await writeFileAsync(css, ``)

    let endPromise
    const context = await esbuild.context({
      entryPoints: [js],
      outdir,
      bundle: true,
      logLevel: 'silent',
      publicPath: 'http://example.com/about',
    });

    try {
      const server = await context.serve({
        host: '127.0.0.1',
      })
      const stream = await makeEventStream(server.host, server.port, '/esbuild')
      await context.rebuild().then(
        () => Promise.reject(new Error('Expected an error to be thrown')),
        () => { /* Ignore the build error due to the missing JS file */ },
      )

      // Event 1: a new JavaScript file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(js, ``)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: ['http://example.com/about/app.js'], removed: [], updated: [] })

      // Event 2: edit the JavaScript file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(js, `foo()`)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: [], removed: [], updated: ['http://example.com/about/app.js'] })

      // Event 3: a new CSS file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(js, `import "./app.css"; foo()`)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: ['http://example.com/about/app.css'], removed: [], updated: [] })

      // Event 4: edit the CSS file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(css, `a { color: red }`)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: [], removed: [], updated: ['http://example.com/about/app.css'] })

      // Event 5: remove the CSS file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(js, `bar()`)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: [], removed: ['http://example.com/about/app.css'], updated: ['http://example.com/about/app.js'] })

      // Wait for the stream to end once we call "dispose()" below
      endPromise = stream.waitFor('close')
    }

    finally {
      await context.dispose();
    }

    // This stream should end once "dispose()" is called above
    await endPromise
  },

  async serveWithServedirWatchLiveReloadPublicPath({ esbuild, testDir }) {
    const js = path.join(testDir, 'app.js')
    const css = path.join(testDir, 'app.css')
    const outdir = path.join(testDir, 'out')
    await writeFileAsync(css, ``)

    let endPromise
    const context = await esbuild.context({
      entryPoints: [js],
      outdir,
      bundle: true,
      logLevel: 'silent',
      publicPath: 'http://example.com/about',
    });

    try {
      const server = await context.serve({
        host: '127.0.0.1',
        servedir: testDir,
      })
      const stream = await makeEventStream(server.host, server.port, '/esbuild')
      await context.rebuild().then(
        () => Promise.reject(new Error('Expected an error to be thrown')),
        () => { /* Ignore the build error due to the missing JS file */ },
      )

      // Event 1: a new JavaScript file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(js, ``)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: ['http://example.com/about/out/app.js'], removed: [], updated: [] })

      // Event 2: edit the JavaScript file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(js, `foo()`)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: [], removed: [], updated: ['http://example.com/about/out/app.js'] })

      // Event 3: a new CSS file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(js, `import "./app.css"; foo()`)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: ['http://example.com/about/out/app.css'], removed: [], updated: [] })

      // Event 4: edit the CSS file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(css, `a { color: red }`)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: [], removed: [], updated: ['http://example.com/about/out/app.css'] })

      // Event 5: remove the CSS file
      var eventPromise = stream.waitFor('change')
      await writeFileAsync(js, `bar()`)
      await context.rebuild()
      var data = JSON.parse((await eventPromise).data)
      assert.deepStrictEqual(data, { added: [], removed: ['http://example.com/about/out/app.css'], updated: ['http://example.com/about/out/app.js'] })

      // Wait for the stream to end once we call "dispose()" below
      endPromise = stream.waitFor('close')
    }

    finally {
      await context.dispose();
    }

    // This stream should end once "dispose()" is called above
    await endPromise
  },
}

async function futureSyntax(esbuild, js, targetBelow, targetAbove) {
  failure: {
    try { await esbuild.transform(js, { target: targetBelow }) }
    catch { break failure }
    throw new Error(`Expected failure for ${targetBelow}: ${js}`)
  }

  try { await esbuild.transform(js, { target: targetAbove }) }
  catch (e) { throw new Error(`Expected success for ${targetAbove}: ${js}\n${e}`) }
}

let transformTests = {
  async transformWithNonString({ esbuild }) {
    try {
      await esbuild.transform(Object.create({ toString() { return '1+2' } }))
      throw new Error('Expected an error to be thrown');
    } catch (e) {
      assert.strictEqual(e.errors ? e.errors[0].text : e + '', 'The input to "transform" must be a string or a Uint8Array')
    }
  },

  async version({ esbuild }) {
    const version = fs.readFileSync(path.join(repoDir, 'version.txt'), 'utf8').trim()
    assert.strictEqual(esbuild.version, version);
  },

  async ignoreUndefinedOptions({ esbuild }) {
    // This should not throw
    await esbuild.transform(``, { jsxFactory: void 0 })
  },

  async throwOnBadOptions({ esbuild }) {
    // This should throw
    try {
      await esbuild.transform(``, { jsxFactory: ['React', 'createElement'] })
      throw new Error('Expected transform failure');
    } catch (e) {
      if (!e.errors || !e.errors[0] || e.errors[0].text !== '"jsxFactory" must be a string') {
        throw e;
      }
    }
  },

  async transformLoaderBase64({ esbuild }) {
    // UTF-16
    var result = await esbuild.transform(`\xFF`, { loader: 'base64' })
    assert.strictEqual(result.code, `module.exports = "w78=";\n`)

    // Binary
    var result = await esbuild.transform(new Uint8Array([0xFF]), { loader: 'base64' })
    assert.strictEqual(result.code, `module.exports = "/w==";\n`)
  },

  async avoidTDZ({ esbuild }) {
    var { code } = await esbuild.transform(`
      class Foo {
        // The above line will be transformed into "var". However, the
        // symbol "Foo" must still be defined before the class body ends.
        static foo = new Foo
      }
      if (!(Foo.foo instanceof Foo))
        throw 'fail'
    `)
    new Function(code)()
  },

  async tsAvoidTDZ({ esbuild }) {
    var { code } = await esbuild.transform(`
      class Foo {
        // The above line will be transformed into "var". However, the
        // symbol "Foo" must still be defined before the class body ends.
        static foo = new Foo
      }
      if (!(Foo.foo instanceof Foo))
        throw 'fail'
    `, {
      loader: 'ts',
    })
    new Function(code)()
  },

  // Note: The TypeScript compiler's transformer doesn't handle this case.
  // Using "this" like this is a syntax error instead. However, we can handle
  // it without too much trouble so we handle it here. This is defensive in
  // case the TypeScript compiler team fixes this in the future.
  async tsAvoidTDZThis({ esbuild }) {
    var { code } = await esbuild.transform(`
      class Foo {
        static foo = 123
        static bar = this.foo // "this" must be rewritten when the property is relocated
      }
      if (Foo.bar !== 123) throw 'fail'
    `, {
      loader: 'ts',
      tsconfigRaw: {
        compilerOptions: {
          useDefineForClassFields: false,
        },
      },
    })
    new Function(code)()
  },

  async tsDecoratorAvoidTDZ({ esbuild }) {
    var { code } = await esbuild.transform(`
      class Bar {}
      var oldFoo
      function swap(target) {
        oldFoo = target
        return Bar
      }
      @swap
      class Foo {
        bar() { return new Foo }
        static foo = new Foo
      }
      if (!(oldFoo.foo instanceof oldFoo))
        throw 'fail: foo'
      if (!(oldFoo.foo.bar() instanceof Bar))
        throw 'fail: bar'
    `, {
      loader: 'ts',
    })
    new Function(code)()
  },

  async mangleQuotedTransform({ esbuild }) {
    var { code } = await esbuild.transform(`x.foo_ = 'foo_' in x`, {
      mangleProps: /_/,
      mangleQuoted: true,
    })
    assert.strictEqual(code, 'x.a = "a" in x;\n')
  },

  async mangleCacheTransform({ esbuild }) {
    var { code, mangleCache } = await esbuild.transform(`x = { x_: 0, y_: 1, z_: 2 }`, {
      mangleProps: /_/,
      mangleCache: { x_: 'FIXED', z_: false },
    })
    assert.strictEqual(code, 'x = { FIXED: 0, a: 1, z_: 2 };\n')
    assert.deepStrictEqual(mangleCache, { x_: 'FIXED', y_: 'a', z_: false })
  },

  async jsBannerTransform({ esbuild }) {
    var { code } = await esbuild.transform(`
      if (!bannerDefined) throw 'fail'
    `, {
      banner: 'const bannerDefined = true',
    })
    new Function(code)()
  },

  async jsFooterTransform({ esbuild }) {
    var { code } = await esbuild.transform(`
      footer()
    `, {
      footer: 'function footer() {}',
    })
    new Function(code)()
    new Function(code)()
  },

  async jsBannerFooterTransform({ esbuild }) {
    var { code } = await esbuild.transform(`
      return { banner: bannerDefined, footer };
    `, {
      banner: 'const bannerDefined = true',
      footer: 'function footer() {}',
    })
    const result = new Function(code)()
    if (!result.banner) throw 'fail'
    result.footer()
  },

  async cssBannerFooterTransform({ esbuild }) {
    var { code } = await esbuild.transform(`
      div { color: red }
    `, {
      loader: 'css',
      banner: '/* banner */',
      footer: '/* footer */',
    })
    assert.strictEqual(code, `/* banner */\ndiv {\n  color: red;\n}\n/* footer */\n`)
  },

  async transformDirectEval({ esbuild }) {
    var { code } = await esbuild.transform(`
      export let abc = 123
      eval('console.log(abc)')
    `, {
      minify: true,
    })
    assert.strictEqual(code, `export let abc=123;eval("console.log(abc)");\n`)
  },

  async tsconfigRawImportsNotUsedAsValuesDefault({ esbuild }) {
    const { code } = await esbuild.transform(`import {T} from 'path'`, {
      tsconfigRaw: {
        compilerOptions: {},
      },
      loader: 'ts',
    })
    assert.strictEqual(code, ``)
  },

  async tsconfigRawImportsNotUsedAsValuesRemove({ esbuild }) {
    const { code } = await esbuild.transform(`import {T} from 'path'`, {
      tsconfigRaw: {
        compilerOptions: {
          importsNotUsedAsValues: 'remove',
        },
      },
      loader: 'ts',
    })
    assert.strictEqual(code, ``)
  },

  async tsconfigRawImportsNotUsedAsValuesPreserve({ esbuild }) {
    const { code } = await esbuild.transform(`import {T} from 'path'`, {
      tsconfigRaw: {
        compilerOptions: {
          importsNotUsedAsValues: 'preserve',
        },
      },
      loader: 'ts',
    })
    assert.strictEqual(code, `import "path";\n`)
  },

  async tsconfigRawImportsNotUsedAsValuesError({ esbuild }) {
    const { code } = await esbuild.transform(`import {T} from 'path'`, {
      tsconfigRaw: {
        compilerOptions: {
          importsNotUsedAsValues: 'error',
        },
      },
      loader: 'ts',
    })
    assert.strictEqual(code, `import "path";\n`)
  },

  async tsconfigRawImportsNotUsedAsValuesRemoveJS({ esbuild }) {
    const { code } = await esbuild.transform(`import {T} from 'path'`, {
      tsconfigRaw: {
        compilerOptions: {
          importsNotUsedAsValues: 'remove',
        },
      },
    })
    assert.strictEqual(code, `import { T } from "path";\n`)
  },

  async tsconfigRawImportsNotUsedAsValuesPreserveJS({ esbuild }) {
    const { code } = await esbuild.transform(`import {T} from 'path'`, {
      tsconfigRaw: {
        compilerOptions: {
          importsNotUsedAsValues: 'preserve',
        },
      },
    })
    assert.strictEqual(code, `import { T } from "path";\n`)
  },

  async tsconfigRawPreserveValueImportsDefault({ esbuild }) {
    const { code } = await esbuild.transform(`import {T} from 'path'`, {
      tsconfigRaw: {
        compilerOptions: {},
      },
      loader: 'ts',
    })
    assert.strictEqual(code, ``)
  },

  async tsconfigRawPreserveValueImportsFalse({ esbuild }) {
    const { code } = await esbuild.transform(`import {T} from 'path'`, {
      tsconfigRaw: {
        compilerOptions: {
          preserveValueImports: false,
        },
      },
      loader: 'ts',
    })
    assert.strictEqual(code, ``)
  },

  async tsconfigRawPreserveValueImportsTrue({ esbuild }) {
    const { code } = await esbuild.transform(`import {T} from 'path'`, {
      tsconfigRaw: {
        compilerOptions: {
          preserveValueImports: true,
        },
      },
      loader: 'ts',
    })
    assert.strictEqual(code, `import { T } from "path";\n`)
  },

  async tsconfigRawPreserveValueImportsTrueMinifyIdentifiers({ esbuild }) {
    const { code } = await esbuild.transform(`import {T} from 'path'`, {
      tsconfigRaw: {
        compilerOptions: {
          preserveValueImports: true,
        },
      },
      loader: 'ts',
      minifyIdentifiers: true,
    })
    assert.strictEqual(code, `import "path";\n`)
  },

  async tsconfigRawPreserveValueImportsTrueImportsNotUsedAsValuesRemove({ esbuild }) {
    const { code } = await esbuild.transform(`import {T} from 'path'`, {
      tsconfigRaw: {
        compilerOptions: {
          importsNotUsedAsValues: 'remove',
          preserveValueImports: true,
        },
      },
      loader: 'ts',
    })
    assert.strictEqual(code, `import { T } from "path";\n`)
  },

  async tsconfigRawCommentsInJSON({ esbuild }) {
    // Can use a string, which allows weird TypeScript pseudo-JSON with comments and trailing commas
    const { code: code5 } = await esbuild.transform(`import {T} from 'path'`, {
      tsconfigRaw: `{
        "compilerOptions": {
          "preserveValueImports": true, // there is a trailing comment here
        },
      }`,
      loader: 'ts',
    })
    assert.strictEqual(code5, `import { T } from "path";\n`)
  },

  async tsconfigRawUseDefineForClassFields({ esbuild }) {
    const { code: code1 } = await esbuild.transform(`class Foo { foo }`, {
      tsconfigRaw: {
        compilerOptions: {
          useDefineForClassFields: false,
        },
      },
      loader: 'ts',
    })
    assert.strictEqual(code1, `class Foo {\n}\n`)

    const { code: code2 } = await esbuild.transform(`class Foo { foo }`, {
      tsconfigRaw: {
        compilerOptions: {
          useDefineForClassFields: true,
        },
      },
      loader: 'ts',
    })
    assert.strictEqual(code2, `class Foo {\n  foo;\n}\n`)
  },

  async tsconfigRawAlwaysStrict({ esbuild }) {
    const input = `console.log(123)`

    assert.strictEqual((await esbuild.transform(input, { loader: 'ts', tsconfigRaw: { compilerOptions: { strict: false } } })).code, `console.log(123);\n`)
    assert.strictEqual((await esbuild.transform(input, { loader: 'ts', tsconfigRaw: { compilerOptions: { alwaysStrict: false } } })).code, `console.log(123);\n`)
    assert.strictEqual((await esbuild.transform(input, { loader: 'ts', tsconfigRaw: { compilerOptions: { alwaysStrict: false, strict: true } } })).code, `console.log(123);\n`)

    assert.strictEqual((await esbuild.transform(input, { loader: 'ts', tsconfigRaw: { compilerOptions: { strict: true } } })).code, `"use strict";\nconsole.log(123);\n`)
    assert.strictEqual((await esbuild.transform(input, { loader: 'ts', tsconfigRaw: { compilerOptions: { alwaysStrict: true } } })).code, `"use strict";\nconsole.log(123);\n`)
    assert.strictEqual((await esbuild.transform(input, { loader: 'ts', tsconfigRaw: { compilerOptions: { alwaysStrict: true, strict: false } } })).code, `"use strict";\nconsole.log(123);\n`)
  },

  async tsconfigRawTarget({ esbuild }) {
    // The "target" from "tsconfig.json" should apply, but esbuild's "target" should override

    assert.strictEqual((await esbuild.transform('x => x', { loader: 'ts', tsconfigRaw: { compilerOptions: { target: 'ES6' } } })).code, `(x) => x;\n`)
    assert.strictEqual((await esbuild.transform('x => x', { loader: 'ts', tsconfigRaw: { compilerOptions: { target: 'ES5' } } })).code, `(function(x) {\n  return x;\n});\n`)

    assert.strictEqual((await esbuild.transform('x => x', { loader: 'ts', target: 'es6' })).code, `(x) => x;\n`)
    assert.strictEqual((await esbuild.transform('x => x', { loader: 'ts', target: 'es5' })).code, `(function(x) {\n  return x;\n});\n`)

    assert.strictEqual((await esbuild.transform('x => x', { loader: 'ts', target: 'es5', tsconfigRaw: { compilerOptions: { target: 'ES6' } } })).code, `(function(x) {\n  return x;\n});\n`)
    assert.strictEqual((await esbuild.transform('x => x', { loader: 'ts', target: 'es6', tsconfigRaw: { compilerOptions: { target: 'ES5' } } })).code, `(x) => x;\n`)
  },

  async tsImplicitUseDefineForClassFields({ esbuild }) {
    var { code } = await esbuild.transform(`class Foo { foo }`, {
      loader: 'ts',
    })
    assert.strictEqual(code, `class Foo {\n}\n`)

    var { code } = await esbuild.transform(`class Foo { foo }`, {
      target: 'es2021',
      loader: 'ts',
    })
    assert.strictEqual(code, `class Foo {\n}\n`)

    var { code } = await esbuild.transform(`class Foo { foo }`, {
      target: 'es2022',
      loader: 'ts',
    })
    assert.strictEqual(code, `class Foo {\n  foo;\n}\n`)

    var { code } = await esbuild.transform(`class Foo { foo }`, {
      target: 'esnext',
      loader: 'ts',
    })
    assert.strictEqual(code, `class Foo {\n  foo;\n}\n`)
  },

  async tsconfigRawJSX({ esbuild }) {
    const { code: code1 } = await esbuild.transform(`<><div/></>`, {
      tsconfigRaw: {
        compilerOptions: {
        },
      },
      loader: 'jsx',
    })
    assert.strictEqual(code1, `/* @__PURE__ */ React.createElement(React.Fragment, null, /* @__PURE__ */ React.createElement("div", null));\n`)

    const { code: code2 } = await esbuild.transform(`<><div/></>`, {
      tsconfigRaw: {
        compilerOptions: {
          jsxFactory: 'factory',
          jsxFragmentFactory: 'fragment',
        },
      },
      loader: 'jsx',
    })
    assert.strictEqual(code2, `/* @__PURE__ */ factory(fragment, null, /* @__PURE__ */ factory("div", null));\n`)

    const { code: code3 } = await esbuild.transform(`<><div/></>`, {
      tsconfigRaw: {
        compilerOptions: {
          jsx: 'react-jsx'
        },
      },
      loader: 'jsx',
    })
    assert.strictEqual(code3, `import { Fragment, jsx } from "react/jsx-runtime";\n/* @__PURE__ */ jsx(Fragment, { children: /* @__PURE__ */ jsx("div", {}) });\n`)

    const { code: code4 } = await esbuild.transform(`<><div/></>`, {
      tsconfigRaw: {
        compilerOptions: {
          jsx: 'react-jsx',
          jsxImportSource: 'notreact'
        },
      },
      loader: 'jsx',
    })
    assert.strictEqual(code4, `import { Fragment, jsx } from "notreact/jsx-runtime";\n/* @__PURE__ */ jsx(Fragment, { children: /* @__PURE__ */ jsx("div", {}) });\n`)

    const { code: code5 } = await esbuild.transform(`<><div/></>`, {
      tsconfigRaw: {
        compilerOptions: {
          jsx: 'react-jsxdev'
        },
      },
      loader: 'jsx',
    })
    assert.strictEqual(code5, `import { Fragment, jsxDEV } from "react/jsx-dev-runtime";\n/* @__PURE__ */ jsxDEV(Fragment, { children: /* @__PURE__ */ jsxDEV("div", {}, void 0, false, {\n  fileName: "<stdin>",\n  lineNumber: 1,\n  columnNumber: 3\n}, this) }, void 0, false, {\n  fileName: "<stdin>",\n  lineNumber: 1,\n  columnNumber: 1\n}, this);\n`)
  },

  // Note: tree shaking is disabled when the output format isn't IIFE
  async treeShakingDefault({ esbuild }) {
    const { code } = await esbuild.transform(`
      var unused = 123
      var used = 234
      export { used }
    `, {
      loader: 'jsx',
      format: 'esm',
      treeShaking: undefined,
    })
    assert.strictEqual(code, `var unused = 123;\nvar used = 234;\nexport {\n  used\n};\n`)
  },

  async treeShakingFalse({ esbuild }) {
    const { code } = await esbuild.transform(`
      var unused = 123
      var used = 234
      export { used }
    `, {
      loader: 'jsx',
      format: 'esm',
      treeShaking: false,
    })
    assert.strictEqual(code, `var unused = 123;\nvar used = 234;\nexport {\n  used\n};\n`)
  },

  async treeShakingTrue({ esbuild }) {
    const { code } = await esbuild.transform(`
      var unused = 123
      var used = 234
      export { used }
    `, {
      loader: 'jsx',
      format: 'esm',
      treeShaking: true,
    })
    assert.strictEqual(code, `var used = 234;\nexport {\n  used\n};\n`)
  },

  // Note: tree shaking is enabled when the output format is IIFE
  async treeShakingDefaultIIFE({ esbuild }) {
    const { code } = await esbuild.transform(`var unused = 123`, {
      loader: 'jsx',
      format: 'iife',
      treeShaking: undefined,
    })
    assert.strictEqual(code, `(() => {\n})();\n`)
  },

  async treeShakingFalseIIFE({ esbuild }) {
    const { code } = await esbuild.transform(`var unused = 123`, {
      loader: 'jsx',
      format: 'iife',
      treeShaking: false,
    })
    assert.strictEqual(code, `(() => {\n  var unused = 123;\n})();\n`)
  },

  async treeShakingTrueIIFE({ esbuild }) {
    const { code } = await esbuild.transform(`var unused = 123`, {
      loader: 'jsx',
      format: 'iife',
      treeShaking: true,
    })
    assert.strictEqual(code, `(() => {\n})();\n`)
  },

  async ignoreAnnotationsDefault({ esbuild }) {
    const { code } = await esbuild.transform(`/* @__PURE__ */ fn(); <div/>`, {
      loader: 'jsx',
      minifySyntax: true,
    })
    assert.strictEqual(code, ``)
  },

  async ignoreAnnotationsFalse({ esbuild }) {
    const { code } = await esbuild.transform(`/* @__PURE__ */ fn(); <div/>`, {
      loader: 'jsx',
      minifySyntax: true,
      ignoreAnnotations: false,
    })
    assert.strictEqual(code, ``)
  },

  async ignoreAnnotationsTrue({ esbuild }) {
    const { code } = await esbuild.transform(`/* @__PURE__ */ fn(); <div/>`, {
      loader: 'jsx',
      minifySyntax: true,
      ignoreAnnotations: true,
    })
    assert.strictEqual(code, `fn(), React.createElement("div", null);\n`)
  },

  async jsCharsetDefault({ esbuild }) {
    const { code } = await esbuild.transform(`let π = 'π'`, {})
    assert.strictEqual(code, `let \\u03C0 = "\\u03C0";\n`)
  },

  async jsCharsetASCII({ esbuild }) {
    const { code } = await esbuild.transform(`let π = 'π'`, { charset: 'ascii' })
    assert.strictEqual(code, `let \\u03C0 = "\\u03C0";\n`)
  },

  async jsCharsetUTF8({ esbuild }) {
    const { code } = await esbuild.transform(`let π = 'π'`, { charset: 'utf8' })
    assert.strictEqual(code, `let π = "π";\n`)
  },

  async cssCharsetDefault({ esbuild }) {
    const { code } = await esbuild.transform(`.π:after { content: 'π' }`, { loader: 'css' })
    assert.strictEqual(code, `.\\3c0:after {\n  content: "\\3c0";\n}\n`)
  },

  async cssCharsetASCII({ esbuild }) {
    const { code } = await esbuild.transform(`.π:after { content: 'π' }`, { loader: 'css', charset: 'ascii' })
    assert.strictEqual(code, `.\\3c0:after {\n  content: "\\3c0";\n}\n`)
  },

  async cssCharsetUTF8({ esbuild }) {
    const { code } = await esbuild.transform(`.π:after { content: 'π' }`, { loader: 'css', charset: 'utf8' })
    assert.strictEqual(code, `.π:after {\n  content: "π";\n}\n`)
  },

  async cssSyntaxErrorWarning({ esbuild }) {
    const { code } = await esbuild.transform(`. {}`, { loader: 'css' })
    assert.strictEqual(code, `.\\  {\n}\n`)
  },

  async cssSyntaxErrorWarningOverride({ esbuild }) {
    try {
      await esbuild.transform(`. {}`, { loader: 'css', logOverride: { 'css-syntax-error': 'error' } })
      throw new Error('Expected a transform failure')
    } catch (e) {
      assert.strictEqual((e && e.message || e) + '', `Transform failed with 1 error:\n<stdin>:1:1: ERROR: Expected identifier but found whitespace`)
    }
  },

  async cssMinify({ esbuild }) {
    const { code } = await esbuild.transform(`div { color: #abcd }`, { loader: 'css', minify: true })
    assert.strictEqual(code, `div{color:#abcd}\n`)
  },

  // Using an "es" target shouldn't affect CSS
  async cssMinifyTargetES6({ esbuild }) {
    const { code } = await esbuild.transform(`div { color: #abcd }`, { loader: 'css', minify: true, target: 'es6' })
    assert.strictEqual(code, `div{color:#abcd}\n`)
  },

  // Using a "node" target shouldn't affect CSS
  async cssMinifyTargetNode({ esbuild }) {
    const { code } = await esbuild.transform(`div { color: #abcd }`, { loader: 'css', minify: true, target: 'node8' })
    assert.strictEqual(code, `div{color:#abcd}\n`)
  },

  // Using an older browser target should affect CSS
  async cssMinifyTargetChrome8({ esbuild }) {
    const { code } = await esbuild.transform(`div { color: #abcd }`, { loader: 'css', minify: true, target: 'chrome8' })
    assert.strictEqual(code, `div{color:rgba(170,187,204,.867)}\n`)
  },

  // Using a newer browser target shouldn't affect CSS
  async cssMinifyTargetChrome80({ esbuild }) {
    const { code } = await esbuild.transform(`div { color: #abcd }`, { loader: 'css', minify: true, target: 'chrome80' })
    assert.strictEqual(code, `div{color:#abcd}\n`)
  },

  async cjs_require({ esbuild }) {
    const { code } = await esbuild.transform(`const {foo} = require('path')`, {})
    assert.strictEqual(code, `const { foo } = require("path");\n`)
  },

  async cjs_exports({ esbuild }) {
    const { code } = await esbuild.transform(`exports.foo = 123`, {})
    assert.strictEqual(code, `exports.foo = 123;\n`)
  },

  async es6_import({ esbuild }) {
    const { code } = await esbuild.transform(`import {foo} from 'path'`, {})
    assert.strictEqual(code, `import { foo } from "path";\n`)
  },

  async es6_export({ esbuild }) {
    const { code } = await esbuild.transform(`export const foo = 123`, {})
    assert.strictEqual(code, `export const foo = 123;\n`)
  },

  async es6_import_to_iife({ esbuild }) {
    const { code } = await esbuild.transform(`import {exists} from "fs"; if (!exists) throw 'fail'`, { format: 'iife' })
    new Function('require', code)(require)
  },

  async es6_import_star_to_iife({ esbuild }) {
    const { code } = await esbuild.transform(`import * as fs from "fs"; if (!fs.exists) throw 'fail'`, { format: 'iife' })
    new Function('require', code)(require)
  },

  async es6_export_to_iife({ esbuild }) {
    const { code } = await esbuild.transform(`export {exists} from "fs"`, { format: 'iife', globalName: 'out' })
    const out = new Function('require', code + ';return out')(require)
    if (out.exists !== fs.exists) throw 'fail'
  },

  async es6_export_star_to_iife({ esbuild }) {
    const { code } = await esbuild.transform(`export * from "fs"`, { format: 'iife', globalName: 'out' })
    const out = new Function('require', code + ';return out')(require)
    if (out.exists !== fs.exists) throw 'fail'
  },

  async es6_export_star_as_to_iife({ esbuild }) {
    const { code } = await esbuild.transform(`export * as fs from "fs"`, { format: 'iife', globalName: 'out' })
    const out = new Function('require', code + ';return out')(require)
    if (out.fs.exists !== fs.exists) throw 'fail'
  },

  async es6_import_to_cjs({ esbuild }) {
    const { code } = await esbuild.transform(`import {exists} from "fs"; if (!exists) throw 'fail'`, { format: 'cjs' })
    new Function('require', code)(require)
  },

  async es6_import_star_to_cjs({ esbuild }) {
    const { code } = await esbuild.transform(`import * as fs from "fs"; if (!fs.exists) throw 'fail'`, { format: 'cjs' })
    new Function('require', code)(require)
  },

  async es6_export_to_cjs({ esbuild }) {
    const { code } = await esbuild.transform(`export {exists} from "fs"`, { format: 'cjs' })
    const module = { exports: {} }
    new Function('module', 'exports', 'require', code)(module, module.exports, require)
    if (module.exports.exists !== fs.exists) throw 'fail'
  },

  async es6_export_star_to_cjs({ esbuild }) {
    const { code } = await esbuild.transform(`export * from "fs"`, { format: 'cjs' })
    const module = { exports: {} }
    new Function('module', 'exports', 'require', code)(module, module.exports, require)
    if (module.exports.exists !== fs.exists) throw 'fail'
  },

  async es6_export_star_as_to_cjs({ esbuild }) {
    const { code } = await esbuild.transform(`export * as fs from "fs"`, { format: 'cjs' })
    const module = { exports: {} }
    new Function('module', 'exports', 'require', code)(module, module.exports, require)
    if (module.exports.fs.exists !== fs.exists) throw 'fail'
  },

  async es6_import_to_esm({ esbuild }) {
    const { code } = await esbuild.transform(`import {exists} from "fs"; if (!exists) throw 'fail'`, { format: 'esm' })
    assert.strictEqual(code, `import { exists } from "fs";\nif (!exists)\n  throw "fail";\n`)
  },

  async es6_import_star_to_esm({ esbuild }) {
    const { code } = await esbuild.transform(`import * as fs from "fs"; if (!fs.exists) throw 'fail'`, { format: 'esm' })
    assert.strictEqual(code, `import * as fs from "fs";\nif (!fs.exists)\n  throw "fail";\n`)
  },

  async es6_export_to_esm({ esbuild }) {
    const { code } = await esbuild.transform(`export {exists} from "fs"`, { format: 'esm' })
    assert.strictEqual(code, `import { exists } from "fs";\nexport {\n  exists\n};\n`)
  },

  async es6_export_star_to_esm({ esbuild }) {
    const { code } = await esbuild.transform(`export * from "fs"`, { format: 'esm' })
    assert.strictEqual(code, `export * from "fs";\n`)
  },

  async es6_export_star_as_to_esm({ esbuild }) {
    const { code } = await esbuild.transform(`export * as fs from "fs"`, { format: 'esm' })
    assert.strictEqual(code, `import * as fs from "fs";\nexport {\n  fs\n};\n`)
  },

  async iifeGlobalName({ esbuild }) {
    const { code } = await esbuild.transform(`export default 123`, { format: 'iife', globalName: 'testName' })
    const globals = {}
    vm.createContext(globals)
    vm.runInContext(code, globals)
    assert.strictEqual(globals.testName.default, 123)
  },

  async iifeGlobalNameCompound({ esbuild }) {
    const { code } = await esbuild.transform(`export default 123`, { format: 'iife', globalName: 'test.name' })
    const globals = {}
    vm.createContext(globals)
    vm.runInContext(code, globals)
    assert.strictEqual(globals.test.name.default, 123)
  },

  async iifeGlobalNameString({ esbuild }) {
    const { code } = await esbuild.transform(`export default 123`, { format: 'iife', globalName: 'test["some text"]' })
    const globals = {}
    vm.createContext(globals)
    vm.runInContext(code, globals)
    assert.strictEqual(globals.test['some text'].default, 123)
  },

  async iifeGlobalNameUnicodeEscape({ esbuild }) {
    const { code } = await esbuild.transform(`export default 123`, { format: 'iife', globalName: 'π["π 𐀀"].𐀀["𐀀 π"]' })
    const globals = {}
    vm.createContext(globals)
    vm.runInContext(code, globals)
    assert.strictEqual(globals.π["π 𐀀"].𐀀["𐀀 π"].default, 123)
    assert.strictEqual(code.slice(0, code.indexOf('(() => {\n')), `var \\u03C0;
(((\\u03C0 ||= {})["\\u03C0 \\uD800\\uDC00"] ||= {})["\\uD800\\uDC00"] ||= {})["\\uD800\\uDC00 \\u03C0"] = `)
  },

  async iifeGlobalNameUnicodeNoEscape({ esbuild }) {
    const { code } = await esbuild.transform(`export default 123`, { format: 'iife', globalName: 'π["π 𐀀"].𐀀["𐀀 π"]', charset: 'utf8' })
    const globals = {}
    vm.createContext(globals)
    vm.runInContext(code, globals)
    assert.strictEqual(globals.π["π 𐀀"].𐀀["𐀀 π"].default, 123)
    assert.strictEqual(code.slice(0, code.indexOf('(() => {\n')), `var π;
(((π ||= {})["π 𐀀"] ||= {})["𐀀"] ||= {})["𐀀 π"] = `)
  },

  async iifeGlobalNameUnicodeEscapeNoLogicalAssignment({ esbuild }) {
    const { code } = await esbuild.transform(`export default 123`, { format: 'iife', globalName: 'π["π 𐀀"].𐀀["𐀀 π"]', supported: { 'logical-assignment': false } })
    const globals = {}
    vm.createContext(globals)
    vm.runInContext(code, globals)
    assert.strictEqual(globals.π["π 𐀀"].𐀀["𐀀 π"].default, 123)
    assert.strictEqual(code.slice(0, code.indexOf('(() => {\n')), `var \\u03C0 = \\u03C0 || {};
\\u03C0["\\u03C0 \\uD800\\uDC00"] = \\u03C0["\\u03C0 \\uD800\\uDC00"] || {};
\\u03C0["\\u03C0 \\uD800\\uDC00"]["\\uD800\\uDC00"] = \\u03C0["\\u03C0 \\uD800\\uDC00"]["\\uD800\\uDC00"] || {};
\\u03C0["\\u03C0 \\uD800\\uDC00"]["\\uD800\\uDC00"]["\\uD800\\uDC00 \\u03C0"] = `)
  },

  async iifeGlobalNameUnicodeNoEscapeNoLogicalAssignment({ esbuild }) {
    const { code } = await esbuild.transform(`export default 123`, { format: 'iife', globalName: 'π["π 𐀀"].𐀀["𐀀 π"]', supported: { 'logical-assignment': false }, charset: 'utf8' })
    const globals = {}
    vm.createContext(globals)
    vm.runInContext(code, globals)
    assert.strictEqual(globals.π["π 𐀀"].𐀀["𐀀 π"].default, 123)
    assert.strictEqual(code.slice(0, code.indexOf('(() => {\n')),
      `var π = π || {};
π["π 𐀀"] = π["π 𐀀"] || {};
π["π 𐀀"]["𐀀"] = π["π 𐀀"]["𐀀"] || {};
π["π 𐀀"]["𐀀"]["𐀀 π"] = `)
  },

  async jsx({ esbuild }) {
    const { code } = await esbuild.transform(`console.log(<div/>)`, { loader: 'jsx' })
    assert.strictEqual(code, `console.log(/* @__PURE__ */ React.createElement("div", null));\n`)
  },

  async jsxTransform({ esbuild }) {
    const { code } = await esbuild.transform(`console.log(<div/>)`, { loader: 'jsx', jsx: 'transform' })
    assert.strictEqual(code, `console.log(/* @__PURE__ */ React.createElement("div", null));\n`)
  },

  async jsxPreserve({ esbuild }) {
    const { code } = await esbuild.transform(`console.log(<div/>)`, { loader: 'jsx', jsx: 'preserve' })
    assert.strictEqual(code, `console.log(<div />);\n`)
  },

  async jsxRuntimeAutomatic({ esbuild }) {
    const { code } = await esbuild.transform(`console.log(<div/>)`, { loader: 'jsx', jsx: 'automatic' })
    assert.strictEqual(code, `import { jsx } from "react/jsx-runtime";\nconsole.log(/* @__PURE__ */ jsx("div", {}));\n`)
  },

  async jsxDev({ esbuild }) {
    const { code } = await esbuild.transform(`console.log(<div/>)`, { loader: 'jsx', jsx: 'automatic', jsxDev: true })
    assert.strictEqual(code, `import { jsxDEV } from "react/jsx-dev-runtime";\nconsole.log(/* @__PURE__ */ jsxDEV("div", {}, void 0, false, {\n  fileName: "<stdin>",\n  lineNumber: 1,\n  columnNumber: 13\n}, this));\n`)
  },

  async jsxImportSource({ esbuild }) {
    const { code } = await esbuild.transform(`console.log(<div/>)`, { loader: 'jsx', jsx: 'automatic', jsxImportSource: 'notreact' })
    assert.strictEqual(code, `import { jsx } from "notreact/jsx-runtime";\nconsole.log(/* @__PURE__ */ jsx("div", {}));\n`)
  },

  async jsxSideEffects({ esbuild }) {
    const { code } = await esbuild.transform(`<b/>`, { loader: 'jsx', jsxSideEffects: true })
    assert.strictEqual(code, `React.createElement("b", null);\n`)
  },

  async ts({ esbuild }) {
    const { code } = await esbuild.transform(`enum Foo { FOO }`, { loader: 'ts' })
    assert.strictEqual(code, `var Foo = /* @__PURE__ */ ((Foo2) => {\n  Foo2[Foo2["FOO"] = 0] = "FOO";\n  return Foo2;\n})(Foo || {});\n`)
  },

  async tsx({ esbuild }) {
    const { code } = await esbuild.transform(`console.log(<Foo<T>/>)`, { loader: 'tsx' })
    assert.strictEqual(code, `console.log(/* @__PURE__ */ React.createElement(Foo, null));\n`)
  },

  async minify({ esbuild }) {
    const { code } = await esbuild.transform(`console.log("a" + "b" + c)`, { minify: true })
    assert.strictEqual(code, `console.log("ab"+c);\n`)
  },

  async keepConsole({ esbuild }) {
    const { code } = await esbuild.transform(`console.log('foo')`, { drop: [] })
    assert.strictEqual(code, `console.log("foo");\n`)
  },

  async dropConsole({ esbuild }) {
    const { code } = await esbuild.transform(`
      console('foo')
      console.log('foo')
      console.log(foo())
      x = console.log(bar())
      console.abc.xyz('foo')
      console['log']('foo')
      console[abc][xyz]('foo')
      console[foo()][bar()]('foo')
    `, { drop: ['console'] })
    assert.strictEqual(code, `console("foo");\nx = void 0;\n`)
  },

  async keepDebugger({ esbuild }) {
    const { code } = await esbuild.transform(`if (x) debugger`, { drop: [] })
    assert.strictEqual(code, `if (x)\n  debugger;\n`)
  },

  async dropDebugger({ esbuild }) {
    const { code } = await esbuild.transform(`if (x) debugger`, { drop: ['debugger'] })
    assert.strictEqual(code, `if (x)\n  ;\n`)
  },

  async define({ esbuild }) {
    const define = { 'process.env.NODE_ENV': '"something"' }

    const { code: code1 } = await esbuild.transform(`console.log(process.env.NODE_ENV)`, { define })
    assert.strictEqual(code1, `console.log("something");\n`)

    const { code: code2 } = await esbuild.transform(`console.log(process.env['NODE_ENV'])`, { define })
    assert.strictEqual(code2, `console.log("something");\n`)

    const { code: code3 } = await esbuild.transform(`console.log(process['env'].NODE_ENV)`, { define })
    assert.strictEqual(code3, `console.log("something");\n`)

    const { code: code4 } = await esbuild.transform(`console.log(process['env']['NODE_ENV'])`, { define })
    assert.strictEqual(code4, `console.log("something");\n`)

    const { code: code5 } = await esbuild.transform(`console.log(process.env.NODE_ENV)`, {})
    assert.strictEqual(code5, `console.log(process.env.NODE_ENV);\n`)

    const { code: code6 } = await esbuild.transform(`console.log(process.env.NODE_ENV)`, { platform: 'browser' })
    assert.strictEqual(code6, `console.log(process.env.NODE_ENV);\n`)
  },

  async defineBuiltInConstants({ esbuild }) {
    const define = { a: 'NaN', b: 'Infinity', c: 'undefined', d: 'something', e: 'null' }
    const { code } = await esbuild.transform(`console.log([typeof a, typeof b, typeof c, typeof d, typeof e])`, { define })
    assert.strictEqual(code, `console.log(["number", "number", "undefined", typeof something, "object"]);\n`)
  },

  async defineArray({ esbuild }) {
    const define = { 'process.env.NODE_ENV': '[1,2,3]', 'something.else': '[2,3,4]' }
    const { code } = await esbuild.transform(`console.log(process.env.NODE_ENV)`, { define })
    assert.strictEqual(code, `var define_process_env_NODE_ENV_default = [1, 2, 3];\nconsole.log(define_process_env_NODE_ENV_default);\n`)
  },

  async defineThis({ esbuild }) {
    const { code } = await esbuild.transform(`console.log(a, b); export {}`, { define: { a: 'this', b: 'this.foo' }, format: 'esm' })
    assert.strictEqual(code, `console.log(void 0, (void 0).foo);\n`)
  },

  async defineImportMetaESM({ esbuild }) {
    const { code } = await esbuild.transform(`console.log(a, b); export {}`, { define: { a: 'import.meta', b: 'import.meta.foo' }, format: 'esm' })
    assert.strictEqual(code, `console.log(import.meta, import.meta.foo);\n`)
  },

  async defineImportMetaIIFE({ esbuild }) {
    const { code } = await esbuild.transform(`console.log(a, b); export {}`, { define: { a: 'import.meta', b: 'import.meta.foo' }, format: 'iife' })
    assert.strictEqual(code, `(() => {\n  const import_meta = {};\n  console.log(import_meta, import_meta.foo);\n})();\n`)
  },

  async json({ esbuild }) {
    const { code } = await esbuild.transform(`{ "x": "y" }`, { loader: 'json' })
    assert.strictEqual(code, `module.exports = { x: "y" };\n`)
  },

  async jsonMinified({ esbuild }) {
    const { code } = await esbuild.transform(`{ "x": "y" }`, { loader: 'json', minify: true })
    const module = {}
    new Function('module', code)(module)
    assert.deepStrictEqual(module.exports, { x: 'y' })
  },

  async jsonESM({ esbuild }) {
    const { code } = await esbuild.transform(`{ "x": "y" }`, { loader: 'json', format: 'esm' })
    assert.strictEqual(code, `var x = "y";\nvar stdin_default = { x };\nexport {\n  stdin_default as default,\n  x\n};\n`)
  },

  async jsonInvalidIdentifierStart({ esbuild }) {
    // This character is a valid "ID_Continue" but not a valid "ID_Start" so it must be quoted
    const { code } = await esbuild.transform(`{ "\\uD835\\uDFCE": "y" }`, { loader: 'json' })
    assert.strictEqual(code, `module.exports = { "\\u{1D7CE}": "y" };\n`)
  },

  async text({ esbuild }) {
    const { code } = await esbuild.transform(`This is some text`, { loader: 'text' })
    assert.strictEqual(code, `module.exports = "This is some text";\n`)
  },

  async textESM({ esbuild }) {
    const { code } = await esbuild.transform(`This is some text`, { loader: 'text', format: 'esm' })
    assert.strictEqual(code, `var stdin_default = "This is some text";\nexport {\n  stdin_default as default\n};\n`)
  },

  async base64({ esbuild }) {
    const { code } = await esbuild.transform(`\x00\x01\x02`, { loader: 'base64' })
    assert.strictEqual(code, `module.exports = "AAEC";\n`)
  },

  async dataurl({ esbuild }) {
    const { code: code1 } = await esbuild.transform(`\x00\x01\x02`, { loader: 'dataurl' })
    assert.strictEqual(code1, `module.exports = "data:application/octet-stream,%00%01%02";\n`)

    const { code: code2 } = await esbuild.transform(`\xFD\xFE\xFF`, { loader: 'dataurl' })
    assert.strictEqual(code2, `module.exports = "data:text/plain;charset=utf-8,\\xFD\\xFE\\xFF";\n`)

    const { code: code3 } = await esbuild.transform(new Uint8Array([0xFD, 0xFE, 0xFF]), { loader: 'dataurl' })
    assert.strictEqual(code3, `module.exports = "data:text/plain;charset=utf-8;base64,/f7/";\n`)
  },

  async sourceMapTrueWithName({ esbuild }) {
    const { code, map } = await esbuild.transform(`let       x`, { sourcemap: true, sourcefile: 'afile.js' })
    assert.strictEqual(code, `let x;\n`)
    await assertSourceMap(map, 'afile.js')
  },

  async sourceMapLinkedWithName({ esbuild }) {
    try {
      await esbuild.transform(`let       x`, { sourcemap: 'linked', sourcefile: 'afile.js' })
      throw new Error('Expected a transform failure')
    } catch (e) {
      assert.strictEqual(e + '', `Error: Transform failed with 1 error:\nerror: Cannot transform with linked source maps`)
    }
  },

  async sourceMapExternalWithName({ esbuild }) {
    const { code, map } = await esbuild.transform(`let       x`, { sourcemap: 'external', sourcefile: 'afile.js' })
    assert.strictEqual(code, `let x;\n`)
    await assertSourceMap(map, 'afile.js')
  },

  async sourceMapInlineWithName({ esbuild }) {
    const { code, map } = await esbuild.transform(`let       x`, { sourcemap: 'inline', sourcefile: 'afile.js' })
    assert(code.startsWith(`let x;\n//# sourceMappingURL=`))
    assert.strictEqual(map, '')
    const base64 = code.slice(code.indexOf('base64,') + 'base64,'.length)
    await assertSourceMap(Buffer.from(base64.trim(), 'base64').toString(), 'afile.js')
  },

  async sourceMapBothWithName({ esbuild }) {
    const { code, map } = await esbuild.transform(`let       x`, { sourcemap: 'both', sourcefile: 'afile.js' })
    assert(code.startsWith(`let x;\n//# sourceMappingURL=`))
    await assertSourceMap(map, 'afile.js')
    const base64 = code.slice(code.indexOf('base64,') + 'base64,'.length)
    await assertSourceMap(Buffer.from(base64.trim(), 'base64').toString(), 'afile.js')
  },

  async sourceMapRoot({ esbuild }) {
    const { code, map } = await esbuild.transform(`let       x`, { sourcemap: true, sourcefile: 'afile.js', sourceRoot: "https://example.com/" })
    assert.strictEqual(code, `let x;\n`)
    assert.strictEqual(JSON.parse(map).sourceRoot, 'https://example.com/');
  },

  async numericLiteralPrinting({ esbuild }) {
    async function checkLiteral(text) {
      const { code } = await esbuild.transform(`return ${text}`, { minify: true })
      assert.strictEqual(+text, new Function(code)())
    }
    const promises = []
    for (let i = 0; i < 10; i++) {
      for (let j = 0; j < 10; j++) {
        promises.push(checkLiteral(`0.${'0'.repeat(i)}${'123456789'.slice(0, j)}`))
        promises.push(checkLiteral(`1${'0'.repeat(i)}.${'123456789'.slice(0, j)}`))
        promises.push(checkLiteral(`1${'123456789'.slice(0, j)}${'0'.repeat(i)}`))
      }
    }
    await Promise.all(promises)
  },

  async tryCatchScopeMerge({ esbuild }) {
    const code = `
      var x = 1
      if (x !== 1) throw 'fail'
      try {
        throw 2
      } catch (x) {
        if (x !== 2) throw 'fail'
        {
          if (x !== 2) throw 'fail'
          var x = 3
          if (x !== 3) throw 'fail'
        }
        if (x !== 3) throw 'fail'
      }
      if (x !== 1) throw 'fail'
    `;
    new Function(code)(); // Verify that the code itself is correct
    new Function((await esbuild.transform(code)).code)();
  },

  async nestedFunctionHoist({ esbuild }) {
    const code = `
      if (x !== void 0) throw 'fail'
      {
        if (x !== void 0) throw 'fail'
        {
          x()
          function x() {}
          x()
        }
        x()
      }
      x()
    `;
    new Function(code)(); // Verify that the code itself is correct
    new Function((await esbuild.transform(code)).code)();
  },

  async nestedFunctionHoistBefore({ esbuild }) {
    const code = `
      var x = 1
      if (x !== 1) throw 'fail'
      {
        if (x !== 1) throw 'fail'
        {
          x()
          function x() {}
          x()
        }
        x()
      }
      x()
    `;
    new Function(code)(); // Verify that the code itself is correct
    new Function((await esbuild.transform(code)).code)();
  },

  async nestedFunctionHoistAfter({ esbuild }) {
    const code = `
      if (x !== void 0) throw 'fail'
      {
        if (x !== void 0) throw 'fail'
        {
          x()
          function x() {}
          x()
        }
        x()
      }
      x()
      var x = 1
    `;
    new Function(code)(); // Verify that the code itself is correct
    new Function((await esbuild.transform(code)).code)();
  },

  async nestedFunctionShadowBefore({ esbuild }) {
    const code = `
      let x = 1
      if (x !== 1) throw 'fail'
      {
        if (x !== 1) throw 'fail'
        {
          x()
          function x() {}
          x()
        }
        if (x !== 1) throw 'fail'
      }
      if (x !== 1) throw 'fail'
    `;
    new Function(code)(); // Verify that the code itself is correct
    new Function((await esbuild.transform(code)).code)();
  },

  async nestedFunctionShadowAfter({ esbuild }) {
    const code = `
      try { x; throw 'fail' } catch (e) { if (!(e instanceof ReferenceError)) throw e }
      {
        try { x; throw 'fail' } catch (e) { if (!(e instanceof ReferenceError)) throw e }
        {
          x()
          function x() {}
          x()
        }
        try { x; throw 'fail' } catch (e) { if (!(e instanceof ReferenceError)) throw e }
      }
      try { x; throw 'fail' } catch (e) { if (!(e instanceof ReferenceError)) throw e }
      let x = 1
    `;
    new Function(code)(); // Verify that the code itself is correct
    new Function((await esbuild.transform(code)).code)();
  },

  async sourceMapControlCharacterEscapes({ esbuild }) {
    let chars = ''
    for (let i = 0; i < 32; i++) chars += String.fromCharCode(i);
    const input = `return \`${chars}\``;
    const { code, map } = await esbuild.transform(input, { sourcemap: true, sourcefile: 'afile.code' })
    const fn = new Function(code)
    assert.strictEqual(fn(), chars.replace('\r', '\n'))
    const json = JSON.parse(map)
    assert.strictEqual(json.version, 3)
    assert.strictEqual(json.sourcesContent.length, 1)
    assert.strictEqual(json.sourcesContent[0], input)
  },

  async transformLegalCommentsJS({ esbuild }) {
    assert.strictEqual((await esbuild.transform(`//!x\ny()`, { legalComments: 'none' })).code, `y();\n`)
    assert.strictEqual((await esbuild.transform(`//!x\ny()`, { legalComments: 'inline' })).code, `//!x\ny();\n`)

    const eofResult = await esbuild.transform(`//!x\ny()`, { legalComments: 'eof' })
    assert.strictEqual(eofResult.code, `y();\n//!x\n`)
    assert.strictEqual(eofResult.legalComments, undefined)

    const externalResult = await esbuild.transform(`//!x\ny()`, { legalComments: 'external' })
    assert.strictEqual(externalResult.code, `y();\n`)
    assert.strictEqual(externalResult.legalComments, `//!x\n`)

    try {
      await esbuild.transform(``, { legalComments: 'linked' })
      throw new Error('Expected a transform failure')
    } catch (e) {
      if (!e || !e.errors || !e.errors[0] || e.errors[0].text !== 'Cannot transform with linked legal comments')
        throw e
    }
  },

  async transformLegalCommentsCSS({ esbuild }) {
    assert.strictEqual((await esbuild.transform(`/*!x*/\ny{}`, { loader: 'css', legalComments: 'none' })).code, `y {\n}\n`)
    assert.strictEqual((await esbuild.transform(`/*!x*/\ny{}`, { loader: 'css', legalComments: 'inline' })).code, `/*!x*/\ny {\n}\n`)

    const eofResult = await esbuild.transform(`/*!x*/\ny{}`, { loader: 'css', legalComments: 'eof' })
    assert.strictEqual(eofResult.code, `y {\n}\n/*!x*/\n`)
    assert.strictEqual(eofResult.legalComments, undefined)

    const externalResult = await esbuild.transform(`/*!x*/\ny{}`, { loader: 'css', legalComments: 'external' })
    assert.strictEqual(externalResult.code, `y {\n}\n`)
    assert.strictEqual(externalResult.legalComments, `/*!x*/\n`)

    try {
      await esbuild.transform(``, { legalComments: 'linked' })
      throw new Error('Expected a transform failure')
    } catch (e) {
      if (!e || !e.errors || !e.errors[0] || e.errors[0].text !== 'Cannot transform with linked legal comments')
        throw e
    }
  },

  async tsDecorators({ esbuild }) {
    const { code } = await esbuild.transform(`
      let observed = [];
      let on = key => (...args) => {
        observed.push({ key, args });
      };

      @on('class')
      class Foo {
        @on('field') field;
        @on('method') method() { }
        @on('staticField') static staticField;
        @on('staticMethod') static staticMethod() { }
        fn(@on('param') x) { }
        static staticFn(@on('staticParam') x) { }
      }

      // This is what the TypeScript compiler itself generates
      let expected = [
        { key: 'field', args: [Foo.prototype, 'field', undefined] },
        { key: 'method', args: [Foo.prototype, 'method', { value: Foo.prototype.method, writable: true, enumerable: false, configurable: true }] },
        { key: 'param', args: [Foo.prototype, 'fn', 0] },
        { key: 'staticField', args: [Foo, 'staticField', undefined] },
        { key: 'staticMethod', args: [Foo, 'staticMethod', { value: Foo.staticMethod, writable: true, enumerable: false, configurable: true }] },
        { key: 'staticParam', args: [Foo, 'staticFn', 0] },
        { key: 'class', args: [Foo] }
      ];

      return {observed, expected};
    `, { loader: 'ts' });
    const { observed, expected } = new Function(code)();
    assert.deepStrictEqual(observed, expected);
  },

  async pureCallPrint({ esbuild }) {
    const { code: code1 } = await esbuild.transform(`print(123, foo)`, { minifySyntax: true, pure: [] })
    assert.strictEqual(code1, `print(123, foo);\n`)

    const { code: code2 } = await esbuild.transform(`print(123, foo)`, { minifySyntax: true, pure: ['print'] })
    assert.strictEqual(code2, `foo;\n`)
  },

  async pureCallConsoleLog({ esbuild }) {
    const { code: code1 } = await esbuild.transform(`console.log(123, foo)`, { minifySyntax: true, pure: [] })
    assert.strictEqual(code1, `console.log(123, foo);\n`)

    const { code: code2 } = await esbuild.transform(`console.log(123, foo)`, { minifySyntax: true, pure: ['console.log'] })
    assert.strictEqual(code2, `foo;\n`)
  },

  async nameCollisionEvalRename({ esbuild }) {
    const { code } = await esbuild.transform(`
      // "arg" must not be renamed to "arg2"
      return function(arg2) {
        function foo(arg) {
          return arg + arg2;
        }
        // "eval" prevents "arg2" from being renamed
        // "arg" below causes "arg" above to be renamed
        return eval(foo(1)) + arg
      }(2);
    `)
    const result = new Function('arg', code)(10)
    assert.strictEqual(result, 13)
  },

  async nameCollisionEvalMinify({ esbuild }) {
    const { code } = await esbuild.transform(`
      // "arg" must not be renamed to "$"
      return function($) {
        function foo(arg) {
          return arg + $;
        }
        // "eval" prevents "$" from being renamed
        // Repeated "$" puts "$" at the top of the character frequency histogram
        return eval(foo($$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$))
      }(2);
    `, { minifyIdentifiers: true })
    const result = new Function('$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$', code)(1)
    assert.strictEqual(result, 3)
  },

  async singleUseExpressionSubstitution({ esbuild }) {
    function run(code) {
      try {
        return JSON.stringify(new Function(code)())
      } catch (error) {
        return error + ''
      }
    }
    let bugs = ''
    for (let input of [
      `let fn = () => { throw new Error }; let x = undef; return fn() + x`,
      `let fn = () => { throw new Error }; let x = fn(); return undef + x`,

      `let fn = () => arg0 = 0; let x = fn(); return arg0 + x`,
      `let fn = () => arg0 = 0; let x = fn(); return arg0 = x`,
      `let fn = () => arg0 = 0; let x = fn(); return arg0 += x`,
      `let fn = () => arg0 = 0; let x = fn(); return arg0 ||= x`,
      `let fn = () => arg0 = 0; let x = fn(); return arg0 &&= x`,

      `let fn = () => arg0 = 0; let obj = [1]; let x = arg0; return obj[fn()] + x`,
      `let fn = () => arg0 = 0; let obj = [1]; let x = arg0; return obj[fn()] = x`,
      `let fn = () => arg0 = 0; let obj = [1]; let x = arg0; return obj[fn()] += x`,
      `let fn = () => arg0 = 0; let obj = [1]; let x = arg0; return obj[fn()] ||= x`,
      `let fn = () => arg0 = 0; let obj = [1]; let x = arg0; return obj[fn()] &&= x`,

      `let obj = { get y() { arg0 = 0; return 1 } }; let x = obj.y; return arg0 + x`,
      `let obj = { get y() { arg0 = 0; return 1 } }; let x = arg0; return obj.y + x`,

      `let x = undef; return arg0 || x`,
      `let x = undef; return arg0 && x`,
      `let x = undef; return arg0 ? x : 1`,
      `let x = undef; return arg0 ? 1 : x`,

      `let fn = () => { throw new Error }; let x = fn(); return arg0 || x`,
      `let fn = () => { throw new Error }; let x = fn(); return arg0 && x`,
      `let fn = () => { throw new Error }; let x = fn(); return arg0 ? x : 1`,
      `let fn = () => { throw new Error }; let x = fn(); return arg0 ? 1 : x`,
    ]) {
      input = `function f(arg0) { ${input} } return f(123)`
      const { code: minified } = await esbuild.transform(input, { minify: true })
      if (run(input) !== run(minified)) bugs += '\n  ' + input
    }
    if (bugs !== '') throw new Error('Single-use expression substitution bugs:' + bugs)
  },

  async platformNode({ esbuild }) {
    const { code } = await esbuild.transform(`export let foo = 123`, { format: 'cjs', platform: 'node' })
    assert(code.slice(code.indexOf('let foo')), `let foo = 123;
// Annotate the CommonJS export names for ESM import in node:
0 && (module.exports = {
  foo
});
`)
  },

  async dynamicImportString({ esbuild }) {
    const { code } = await esbuild.transform(`import('foo')`, { target: 'chrome63' })
    assert.strictEqual(code, `import("foo");\n`)
  },

  async dynamicImportStringES6({ esbuild }) {
    const fromPromiseResolve = text => text.slice(text.indexOf('Promise.resolve'))
    const { code } = await esbuild.transform(`import('foo')`, { target: 'chrome62' })
    assert.strictEqual(fromPromiseResolve(code), `Promise.resolve().then(() => __toESM(require("foo")));\n`)
  },

  async dynamicImportStringES5({ esbuild }) {
    const fromPromiseResolve = text => text.slice(text.indexOf('Promise.resolve'))
    const { code } = await esbuild.transform(`import('foo')`, { target: 'chrome48' })
    assert.strictEqual(fromPromiseResolve(code), `Promise.resolve().then(function() {\n  return __toESM(require("foo"));\n});\n`)
  },

  async dynamicImportStringES5Minify({ esbuild }) {
    const fromPromiseResolve = text => text.slice(text.indexOf('Promise.resolve'))
    const { code } = await esbuild.transform(`import('foo')`, { target: 'chrome48', minifyWhitespace: true })
    assert.strictEqual(fromPromiseResolve(code), `Promise.resolve().then(function(){return __toESM(require("foo"))});\n`)
  },

  async dynamicImportStringNode12_19({ esbuild }) {
    const fromPromiseResolve = text => text.slice(text.indexOf('Promise.resolve'))
    const { code } = await esbuild.transform(`import('foo')`, { target: 'node12.19' })
    assert.strictEqual(fromPromiseResolve(code), `Promise.resolve().then(() => __toESM(require("foo")));\n`)
  },

  async dynamicImportStringNode12_20({ esbuild }) {
    const { code } = await esbuild.transform(`import('foo')`, { target: 'node12.20' })
    assert.strictEqual(code, `import("foo");\n`)
  },

  async dynamicImportStringNode13({ esbuild }) {
    const fromPromiseResolve = text => text.slice(text.indexOf('Promise.resolve'))
    const { code } = await esbuild.transform(`import('foo')`, { target: 'node13' })
    assert.strictEqual(fromPromiseResolve(code), `Promise.resolve().then(() => __toESM(require("foo")));\n`)
  },

  async dynamicImportStringNode13_1({ esbuild }) {
    const fromPromiseResolve = text => text.slice(text.indexOf('Promise.resolve'))
    const { code } = await esbuild.transform(`import('foo')`, { target: 'node13.1' })
    assert.strictEqual(fromPromiseResolve(code), `Promise.resolve().then(() => __toESM(require("foo")));\n`)
  },

  async dynamicImportStringNode13_2({ esbuild }) {
    const { code } = await esbuild.transform(`import('foo')`, { target: 'node13.2' })
    assert.strictEqual(code, `import("foo");\n`)
  },

  async dynamicImportExpression({ esbuild }) {
    const { code } = await esbuild.transform(`import(foo)`, { target: 'chrome63' })
    assert.strictEqual(code, `import(foo);\n`)
  },

  async dynamicImportExpressionES6({ esbuild }) {
    const fromPromiseResolve = text => text.slice(text.indexOf('Promise.resolve'))
    const { code: code2 } = await esbuild.transform(`import(foo)`, { target: 'chrome62' })
    assert.strictEqual(fromPromiseResolve(code2), `Promise.resolve().then(() => __toESM(require(foo)));\n`)
  },

  async dynamicImportExpressionES5({ esbuild }) {
    const fromPromiseResolve = text => text.slice(text.indexOf('Promise.resolve'))
    const { code: code3 } = await esbuild.transform(`import(foo)`, { target: 'chrome48' })
    assert.strictEqual(fromPromiseResolve(code3), `Promise.resolve().then(function() {\n  return __toESM(require(foo));\n});\n`)
  },

  async dynamicImportExpressionES5Minify({ esbuild }) {
    const fromPromiseResolve = text => text.slice(text.indexOf('Promise.resolve'))
    const { code: code4 } = await esbuild.transform(`import(foo)`, { target: 'chrome48', minifyWhitespace: true })
    assert.strictEqual(fromPromiseResolve(code4), `Promise.resolve().then(function(){return __toESM(require(foo))});\n`)
  },

  async inlineScript({ esbuild }) {
    let p
    assert.strictEqual((await esbuild.transform(`x = '</script>'`, {})).code, `x = "<\\/script>";\n`)
    assert.strictEqual((await esbuild.transform(`x = '</script> inline'`, { supported: { 'inline-script': true } })).code, `x = "<\\/script> inline";\n`)
    assert.strictEqual((await esbuild.transform(`x = '</script> noinline'`, { supported: { 'inline-script': false } })).code, `x = "</script> noinline";\n`)

    p = { platform: 'browser' }
    assert.strictEqual((await esbuild.transform(`x = '</script> browser'`, { ...p })).code, `x = "<\\/script> browser";\n`)
    assert.strictEqual((await esbuild.transform(`x = '</script> browser inline'`, { ...p, supported: { 'inline-script': true } })).code, `x = "<\\/script> browser inline";\n`)
    assert.strictEqual((await esbuild.transform(`x = '</script> browser noinline'`, { ...p, supported: { 'inline-script': false } })).code, `x = "</script> browser noinline";\n`)

    p = { platform: 'node' }
    assert.strictEqual((await esbuild.transform(`x = '</script> node'`, { ...p })).code, `x = "</script> node";\n`)
    assert.strictEqual((await esbuild.transform(`x = '</script> node inline'`, { ...p, supported: { 'inline-script': true } })).code, `x = "<\\/script> node inline";\n`)
    assert.strictEqual((await esbuild.transform(`x = '</script> node noinline'`, { ...p, supported: { 'inline-script': false } })).code, `x = "</script> node noinline";\n`)

    p = { platform: 'neutral' }
    assert.strictEqual((await esbuild.transform(`x = '</script> neutral'`, { ...p })).code, `x = "</script> neutral";\n`)
    assert.strictEqual((await esbuild.transform(`x = '</script> neutral inline'`, { ...p, supported: { 'inline-script': true } })).code, `x = "<\\/script> neutral inline";\n`)
    assert.strictEqual((await esbuild.transform(`x = '</script> neutral noinline'`, { ...p, supported: { 'inline-script': false } })).code, `x = "</script> neutral noinline";\n`)

    assert.strictEqual((await esbuild.transform(`x = '</script>'`, { target: 'esnext' })).code, `x = "<\\/script>";\n`)
    assert.strictEqual((await esbuild.transform(`x = '</script>'`, { target: 'es2020' })).code, `x = "<\\/script>";\n`)
    assert.strictEqual((await esbuild.transform(`x = '</script>'`, { target: 'es6' })).code, `x = "<\\/script>";\n`)
    assert.strictEqual((await esbuild.transform(`x = '</script>'`, { target: 'chrome999' })).code, `x = "<\\/script>";\n`)
    assert.strictEqual((await esbuild.transform(`x = '</script>'`, { target: 'chrome0' })).code, `x = "<\\/script>";\n`)
  },

  async inlineStyle({ esbuild }) {
    let p = { loader: 'css' }
    assert.strictEqual((await esbuild.transform(`x { y: '</style>' }`, { ...p })).code, `x {\n  y: "<\\/style>";\n}\n`)
    assert.strictEqual((await esbuild.transform(`x { y: '</style> inline' }`, { ...p, supported: { 'inline-style': true } })).code, `x {\n  y: "<\\/style> inline";\n}\n`)
    assert.strictEqual((await esbuild.transform(`x { y: '</style> noinline' }`, { ...p, supported: { 'inline-style': false } })).code, `x {\n  y: "</style> noinline";\n}\n`)

    p = { loader: 'css', platform: 'browser' }
    assert.strictEqual((await esbuild.transform(`x { y: '</style> browser' }`, { ...p })).code, `x {\n  y: "<\\/style> browser";\n}\n`)
    assert.strictEqual((await esbuild.transform(`x { y: '</style> browser inline' }`, { ...p, supported: { 'inline-style': true } })).code, `x {\n  y: "<\\/style> browser inline";\n}\n`)
    assert.strictEqual((await esbuild.transform(`x { y: '</style> browser noinline' }`, { ...p, supported: { 'inline-style': false } })).code, `x {\n  y: "</style> browser noinline";\n}\n`)

    p = { loader: 'css', platform: 'node' }
    assert.strictEqual((await esbuild.transform(`x { y: '</style> node' }`, { ...p })).code, `x {\n  y: "</style> node";\n}\n`)
    assert.strictEqual((await esbuild.transform(`x { y: '</style> node inline' }`, { ...p, supported: { 'inline-style': true } })).code, `x {\n  y: "<\\/style> node inline";\n}\n`)
    assert.strictEqual((await esbuild.transform(`x { y: '</style> node noinline' }`, { ...p, supported: { 'inline-style': false } })).code, `x {\n  y: "</style> node noinline";\n}\n`)

    p = { loader: 'css', platform: 'neutral' }
    assert.strictEqual((await esbuild.transform(`x { y: '</style> neutral' }`, { ...p })).code, `x {\n  y: "</style> neutral";\n}\n`)
    assert.strictEqual((await esbuild.transform(`x { y: '</style> neutral inline' }`, { ...p, supported: { 'inline-style': true } })).code, `x {\n  y: "<\\/style> neutral inline";\n}\n`)
    assert.strictEqual((await esbuild.transform(`x { y: '</style> neutral noinline' }`, { ...p, supported: { 'inline-style': false } })).code, `x {\n  y: "</style> neutral noinline";\n}\n`)

    p = { loader: 'css' }
    assert.strictEqual((await esbuild.transform(`x { y: '</style>' }`, { ...p, target: 'esnext' })).code, `x {\n  y: "<\\/style>";\n}\n`)
    assert.strictEqual((await esbuild.transform(`x { y: '</style>' }`, { ...p, target: 'es2020' })).code, `x {\n  y: "<\\/style>";\n}\n`)
    assert.strictEqual((await esbuild.transform(`x { y: '</style>' }`, { ...p, target: 'es6' })).code, `x {\n  y: "<\\/style>";\n}\n`)
    assert.strictEqual((await esbuild.transform(`x { y: '</style>' }`, { ...p, target: 'chrome999' })).code, `x {\n  y: "<\\/style>";\n}\n`)
    assert.strictEqual((await esbuild.transform(`x { y: '</style>' }`, { ...p, target: 'chrome0' })).code, `x {\n  y: "<\\/style>";\n}\n`)
  },

  async typeofEqualsUndefinedTarget({ esbuild }) {
    assert.strictEqual((await esbuild.transform(`a = typeof b !== 'undefined'`, { minify: true })).code, `a=typeof b<"u";\n`)
    assert.strictEqual((await esbuild.transform(`a = typeof b !== 'undefined'`, { minify: true, target: 'es2020' })).code, `a=typeof b<"u";\n`)
    assert.strictEqual((await esbuild.transform(`a = typeof b !== 'undefined'`, { minify: true, target: 'chrome11' })).code, `a=typeof b<"u";\n`)

    assert.strictEqual((await esbuild.transform(`a = typeof b !== 'undefined'`, { minify: true, target: 'es2019' })).code, `a=typeof b!="undefined";\n`)
    assert.strictEqual((await esbuild.transform(`a = typeof b !== 'undefined'`, { minify: true, target: 'ie11' })).code, `a=typeof b!="undefined";\n`)
  },

  async caseInsensitiveTarget({ esbuild }) {
    assert.strictEqual((await esbuild.transform(`a ||= b`, { target: 'eS5' })).code, `a || (a = b);\n`)
    assert.strictEqual((await esbuild.transform(`a ||= b`, { target: 'eSnExT' })).code, `a ||= b;\n`)
  },

  async multipleEngineTargets({ esbuild }) {
    const check = async (target, expected) =>
      assert.strictEqual((await esbuild.transform(`foo(a ?? b)`, { target })).code, expected)
    await Promise.all([
      check('es2020', `foo(a ?? b);\n`),
      check('es2019', `foo(a != null ? a : b);\n`),

      check('chrome80', `foo(a ?? b);\n`),
      check('chrome79', `foo(a != null ? a : b);\n`),

      check(['es2020', 'chrome80'], `foo(a ?? b);\n`),
      check(['es2020', 'chrome79'], `foo(a != null ? a : b);\n`),
      check(['es2019', 'chrome80'], `foo(a != null ? a : b);\n`),
    ])
  },

  async multipleEngineTargetsNotSupported({ esbuild }) {
    try {
      await esbuild.transform(`0n`, { target: ['es5', 'chrome1', 'safari2', 'firefox3'] })
      throw new Error('Expected an error to be thrown')
    } catch (e) {
      assert.strictEqual(e.errors[0].text,
        'Big integer literals are not available in the configured target environment ("chrome1", "es5", "firefox3", "safari2")')
    }
  },

  async supported({ esbuild }) {
    const check = async (options, input, expected) => {
      try {
        assert.strictEqual((await esbuild.transform(input, options)).code, expected)
      } catch (e) {
        if (e.errors) assert.strictEqual(e.errors[0].text, expected)
        else throw e
      }
    }

    await Promise.all([
      // JS: lower
      check({ supported: { arrow: true } }, `x = () => y`, `x = () => y;\n`),
      check({ supported: { arrow: false } }, `x = () => y`, `x = function() {\n  return y;\n};\n`),
      check({ supported: { arrow: true }, target: 'es5' }, `x = () => y`, `x = () => y;\n`),
      check({ supported: { arrow: false }, target: 'es5' }, `x = () => y`, `x = function() {\n  return y;\n};\n`),
      check({ supported: { arrow: true }, target: 'es2022' }, `x = () => y`, `x = () => y;\n`),
      check({ supported: { arrow: false }, target: 'es2022' }, `x = () => y`, `x = function() {\n  return y;\n};\n`),

      // JS: error
      check({ supported: { bigint: true } }, `x = 1n`, `x = 1n;\n`),
      check({ supported: { bigint: false } }, `x = 1n`, `Big integer literals are not available in the configured target environment`),
      check({ supported: { bigint: true }, target: 'es5' }, `x = 1n`, `x = 1n;\n`),
      check({ supported: { bigint: false }, target: 'es5' }, `x = 1n`, `Big integer literals are not available in the configured target environment ("es5" + 1 override)`),
      check({ supported: { bigint: true }, target: 'es2022' }, `x = 1n`, `x = 1n;\n`),
      check({ supported: { bigint: false }, target: 'es2022' }, `x = 1n`, `Big integer literals are not available in the configured target environment ("es2022" + 1 override)`),

      // CSS: lower
      check({ supported: { 'hex-rgba': true }, loader: 'css' }, `a { color: #1234 }`, `a {\n  color: #1234;\n}\n`),
      check({ supported: { 'hex-rgba': false }, loader: 'css' }, `a { color: #1234 }`, `a {\n  color: rgba(17, 34, 51, 0.267);\n}\n`),

      // Check for "+ 2 overrides"
      check({ supported: { bigint: false, arrow: true }, target: 'es2022' }, `x = 1n`, `Big integer literals are not available in the configured target environment ("es2022" + 2 overrides)`),
    ])
  },

  async regExpFeatures({ esbuild }) {
    const check = async (target, input, expected) =>
      assert.strictEqual((await esbuild.transform(input, { target })).code, expected)

    await Promise.all([
      // RegExpStickyAndUnicodeFlags
      check('es6', `x1 = /./y`, `x1 = /./y;\n`),
      check('es6', `x2 = /./u`, `x2 = /./u;\n`),
      check('es5', `x3 = /./y`, `x3 = new RegExp(".", "y");\n`),
      check('es5', `x4 = /./u`, `x4 = new RegExp(".", "u");\n`),

      // RegExpDotAllFlag
      check('es2018', `x1 = /a.b/s`, `x1 = /a.b/s;\n`),
      check('es2017', `x2 = /a.b/s`, `x2 = new RegExp("a.b", "s");\n`),

      // RegExpLookbehindAssertions
      check('es2018', `x1 = /(?<=x)/`, `x1 = /(?<=x)/;\n`),
      check('es2018', `x2 = /(?<!x)/`, `x2 = /(?<!x)/;\n`),
      check('es2017', `x3 = /(?<=x)/`, `x3 = new RegExp("(?<=x)");\n`),
      check('es2017', `x4 = /(?<!x)/`, `x4 = new RegExp("(?<!x)");\n`),

      // RegExpNamedCaptureGroups
      check('es2018', `x1 = /(?<a>b)/`, `x1 = /(?<a>b)/;\n`),
      check('es2017', `x2 = /(?<a>b)/`, `x2 = new RegExp("(?<a>b)");\n`),

      // RegExpUnicodePropertyEscapes
      check('es2018', `x1 = /\\p{Emoji}/u`, `x1 = /\\p{Emoji}/u;\n`),
      check('es2017', `x2 = /\\p{Emoji}/u`, `x2 = new RegExp("\\\\p{Emoji}", "u");\n`),

      // RegExpMatchIndices
      check('es2022', `x1 = /y/d`, `x1 = /y/d;\n`),
      check('es2021', `x2 = /y/d`, `x2 = new RegExp("y", "d");\n`),

      // RegExpSetNotation
      check('esnext', `x1 = /[\\p{White_Space}&&\\p{ASCII}]/v`, `x1 = /[\\p{White_Space}&&\\p{ASCII}]/v;\n`),
      check('es2022', `x2 = /[\\p{White_Space}&&\\p{ASCII}]/v`, `x2 = new RegExp("[\\\\p{White_Space}&&\\\\p{ASCII}]", "v");\n`),
    ])
  },

  // Future syntax
  bigInt: ({ esbuild }) => futureSyntax(esbuild, '123n', 'es2019', 'es2020'),
  bigIntKey: ({ esbuild }) => futureSyntax(esbuild, '({123n: 0})', 'es2019', 'es2020'),
  bigIntPattern: ({ esbuild }) => futureSyntax(esbuild, 'let {123n: x} = y', 'es2019', 'es2020'),
  nonIdArrayRest: ({ esbuild }) => futureSyntax(esbuild, 'let [...[x]] = y', 'es2015', 'es2016'),
  topLevelAwait: ({ esbuild }) => futureSyntax(esbuild, 'await foo', 'es2020', 'esnext'),
  topLevelForAwait: ({ esbuild }) => futureSyntax(esbuild, 'for await (foo of bar) ;', 'es2020', 'esnext'),

  // Future syntax: async generator functions
  asyncGenFnStmt: ({ esbuild }) => futureSyntax(esbuild, 'async function* foo() {}', 'es2017', 'es2018'),
  asyncGenFnExpr: ({ esbuild }) => futureSyntax(esbuild, '(async function*() {})', 'es2017', 'es2018'),
  asyncGenObjFn: ({ esbuild }) => futureSyntax(esbuild, '({ async* foo() {} })', 'es2017', 'es2018'),
  asyncGenClassStmtFn: ({ esbuild }) => futureSyntax(esbuild, 'class Foo { async* foo() {} }', 'es2017', 'es2018'),
  asyncGenClassExprFn: ({ esbuild }) => futureSyntax(esbuild, '(class { async* foo() {} })', 'es2017', 'es2018'),
}

function registerClassPrivateTests(target) {
  let contents = `
    class Field { #foo = 123; bar = this.#foo }
    if (new Field().bar !== 123) throw 'fail: field'

    class Method { bar = this.#foo(); #foo() { return 123 } }
    if (new Method().bar !== 123) throw 'fail: method'

    class Accessor { bar = this.#foo; get #foo() { return 123 } }
    if (new Accessor().bar !== 123) throw 'fail: accessor'

    class StaticField { static #foo = 123; static bar = StaticField.#foo }
    if (StaticField.bar !== 123) throw 'fail: static field'

    class StaticMethod { static bar = StaticMethod.#foo(); static #foo() { return 123 } }
    if (StaticMethod.bar !== 123) throw 'fail: static method'

    class StaticAccessor { static bar = StaticAccessor.#foo; static get #foo() { return 123 } }
    if (StaticAccessor.bar !== 123) throw 'fail: static accessor'

    class StaticFieldThis { static #foo = 123; static bar = this.#foo }
    if (StaticFieldThis.bar !== 123) throw 'fail: static field'

    class StaticMethodThis { static bar = this.#foo(); static #foo() { return 123 } }
    if (StaticMethodThis.bar !== 123) throw 'fail: static method'

    class StaticAccessorThis { static bar = this.#foo; static get #foo() { return 123 } }
    if (StaticAccessorThis.bar !== 123) throw 'fail: static accessor'

    class FieldFromStatic { #foo = 123; static bar = new FieldFromStatic().#foo }
    if (FieldFromStatic.bar !== 123) throw 'fail: field from static'

    class MethodFromStatic { static bar = new MethodFromStatic().#foo(); #foo() { return 123 } }
    if (MethodFromStatic.bar !== 123) throw 'fail: method from static'

    class AccessorFromStatic { static bar = new AccessorFromStatic().#foo; get #foo() { return 123 } }
    if (AccessorFromStatic.bar !== 123) throw 'fail: accessor from static'
  `

  // Test this code as JavaScript
  let buildOptions = {
    stdin: { contents },
    bundle: true,
    write: false,
    target,
  }
  transformTests[`transformClassPrivate_${target[0]}`] = async ({ esbuild }) =>
    new Function((await esbuild.transform(contents, { target })).code)()
  buildTests[`buildClassPrivate_${target[0]}`] = async ({ esbuild }) =>
    new Function((await esbuild.build(buildOptions)).outputFiles[0].text)()

  // Test this code as TypeScript
  let buildOptionsTS = {
    stdin: { contents, loader: 'ts' },
    bundle: true,
    write: false,
  }
  transformTests[`tsTransformClassPrivate_${target[0]}`] = async ({ esbuild }) =>
    new Function((await esbuild.transform(contents, { target, loader: 'ts' })).code)()
  buildTests[`tsBuildClassPrivate_${target[0]}`] = async ({ esbuild }) =>
    new Function((await esbuild.build(buildOptionsTS)).outputFiles[0].text)()
}

for (let es of ['es2015', 'es2016', 'es2017', 'es2018', 'es2019', 'es2020', 'esnext'])
  registerClassPrivateTests([es])
for (let chrome = 49; chrome < 100; chrome++)
  registerClassPrivateTests([`chrome${chrome}`])
for (let firefox = 45; firefox < 100; firefox++)
  registerClassPrivateTests([`firefox${firefox}`])
for (let edge = 13; edge < 100; edge++)
  registerClassPrivateTests([`edge${edge}`])
for (let safari = 10; safari < 20; safari++)
  registerClassPrivateTests([`safari${safari}`])

let formatTests = {
  async formatMessages({ esbuild }) {
    const messages = await esbuild.formatMessages([
      { text: 'This is an error' },
      { text: 'Another error', location: { file: 'file.js' } },
    ], {
      kind: 'error',
    })
    assert.strictEqual(messages.length, 2)
    assert.strictEqual(messages[0], `${errorIcon} [ERROR] This is an error\n\n`)
    assert.strictEqual(messages[1], `${errorIcon} [ERROR] Another error\n\n    file.js:0:0:\n      0 │ \n        ╵ ^\n\n`)
  },
}

let analyzeTests = {
  async analyzeMetafile({ esbuild }) {
    const metafile = {
      "inputs": {
        "entry.js": {
          "bytes": 50,
          "imports": [
            {
              "path": "lib.js",
              "kind": "import-statement"
            }
          ]
        },
        "lib.js": {
          "bytes": 200,
          "imports": []
        }
      },
      "outputs": {
        "out.js": {
          "imports": [],
          "exports": [],
          "entryPoint": "entry.js",
          "inputs": {
            "entry.js": {
              "bytesInOutput": 25
            },
            "lib.js": {
              "bytesInOutput": 50
            }
          },
          "bytes": 100
        }
      }
    }
    assert.strictEqual(await esbuild.analyzeMetafile(metafile), `
  out.js       100b   100.0%
   ├ lib.js     50b    50.0%
   └ entry.js   25b    25.0%
`)
    assert.strictEqual(await esbuild.analyzeMetafile(metafile, { verbose: true }), `
  out.js ────── 100b ── 100.0%
   ├ lib.js ──── 50b ─── 50.0%
   │  └ entry.js
   └ entry.js ── 25b ─── 25.0%
`)
  },
}

let functionScopeCases = [
  'function x() {} { var x }',
  'function* x() {} { var x }',
  'async function x() {} { var x }',
  'async function* x() {} { var x }',
  '{ var x } function x() {}',
  '{ var x } function* x() {}',
  '{ var x } async function x() {}',
  '{ var x } async function* x() {}',

  '{ function x() {} { var x } }',
  '{ function* x() {} { var x } }',
  '{ async function x() {} { var x } }',
  '{ async function* x() {} { var x } }',
  '{ { var x } function x() {} }',
  '{ { var x } function* x() {} }',
  '{ { var x } async function x() {} }',
  '{ { var x } async function* x() {} }',

  'function f() { function x() {} { var x } }',
  'function f() { function* x() {} { var x } }',
  'function f() { async function x() {} { var x } }',
  'function f() { async function* x() {} { var x } }',
  'function f() { { var x } function x() {} }',
  'function f() { { var x } function* x() {} }',
  'function f() { { var x } async function x() {} }',
  'function f() { { var x } async function* x() {} }',

  'function f() { { function x() {} { var x } }}',
  'function f() { { function* x() {} { var x } }}',
  'function f() { { async function x() {} { var x } }}',
  'function f() { { async function* x() {} { var x } }}',
  'function f() { { { var x } function x() {} }}',
  'function f() { { { var x } function* x() {} }}',
  'function f() { { { var x } async function x() {} }}',
  'function f() { { { var x } async function* x() {} }}',
];

{
  let counter = 0;
  for (let kind of ['var', 'let', 'const']) {
    for (let code of functionScopeCases) {
      code = code.replace('var', kind)
      transformTests['functionScope' + counter++] = async ({ esbuild }) => {
        let esbuildError
        let nodeError
        try { await esbuild.transform(code) } catch (e) { esbuildError = e }
        try { new Function(code)() } catch (e) { nodeError = e }
        if (!esbuildError !== !nodeError) {
          throw new Error(`
            code: ${code}
            esbuild: ${esbuildError}
            node: ${nodeError}
          `)
        }
      }
    }
  }
}

let syncTests = {
  async defaultExport({ esbuild }) {
    assert.strictEqual(typeof esbuild.version, 'string')
    assert.strictEqual(esbuild.version, esbuild.default.version)
    assert.strictEqual(esbuild.version, esbuild.default.default.version)
    assert.strictEqual(esbuild.version, esbuild.default.default.default.version)
  },

  async buildSync({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, 'export default 123')
    esbuild.buildSync({ entryPoints: [input], bundle: true, outfile: output, format: 'cjs' })
    const result = require(output)
    assert.strictEqual(result.default, 123)
    assert.strictEqual(result.__esModule, true)
  },

  async buildSyncOutputFiles({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    const output = path.join(testDir, 'out.js')
    await writeFileAsync(input, 'module.exports = 123')
    let prettyPath = path.relative(process.cwd(), input).split(path.sep).join('/')
    let text = `// ${prettyPath}\nmodule.exports = 123;\n`
    let result = esbuild.buildSync({ entryPoints: [input], bundle: true, outfile: output, format: 'cjs', write: false })
    assert.strictEqual(result.outputFiles.length, 1)
    assert.strictEqual(result.outputFiles[0].path, output)
    assert.strictEqual(result.outputFiles[0].text, text)
    assert.deepStrictEqual(result.outputFiles[0].contents, new Uint8Array(Buffer.from(text)))
  },

  async transformSyncJSMap({ esbuild }) {
    const { code, map } = esbuild.transformSync(`1+2`, { sourcemap: true })
    assert.strictEqual(code, `1 + 2;\n`)
    assert.strictEqual(map, `{
  "version": 3,
  "sources": ["<stdin>"],
  "sourcesContent": ["1+2"],
  "mappings": "AAAA,IAAE;",
  "names": []
}
`)
  },

  async transformSyncJSMapNoContent({ esbuild }) {
    const { code, map } = esbuild.transformSync(`1+2`, { sourcemap: true, sourcesContent: false })
    assert.strictEqual(code, `1 + 2;\n`)
    assert.strictEqual(map, `{
  "version": 3,
  "sources": ["<stdin>"],
  "mappings": "AAAA,IAAE;",
  "names": []
}
`)
  },

  async transformSyncCSS({ esbuild }) {
    const { code, map } = esbuild.transformSync(`a{b:c}`, { loader: 'css' })
    assert.strictEqual(code, `a {\n  b: c;\n}\n`)
    assert.strictEqual(map, '')
  },

  async transformSyncWithNonString({ esbuild }) {
    try {
      esbuild.transformSync(Object.create({ toString() { return '1+2' } }))
      throw new Error('Expected an error to be thrown');
    } catch (e) {
      assert.strictEqual(e.errors ? e.errors[0].text : e + '', 'The input to "transform" must be a string or a Uint8Array')
    }
  },

  async transformSync100x({ esbuild }) {
    for (let i = 0; i < 100; i++) {
      const { code } = esbuild.transformSync(`console.log(1+${i})`, {})
      assert.strictEqual(code, `console.log(1 + ${i});\n`)
    }
  },

  async buildSyncThrow({ esbuild, testDir }) {
    const input = path.join(testDir, 'in.js')
    try {
      const output = path.join(testDir, 'out.js')
      await writeFileAsync(input, '1+')
      esbuild.buildSync({ entryPoints: [input], bundle: true, outfile: output, format: 'cjs', logLevel: 'silent' })
      const result = require(output)
      assert.strictEqual(result.default, 123)
      assert.strictEqual(result.__esModule, true)
      throw new Error('Expected an error to be thrown');
    } catch (error) {
      assert(error instanceof Error, 'Must be an Error object');
      assert.strictEqual(error.message, `Build failed with 1 error:
${path.relative(process.cwd(), input).split(path.sep).join('/')}:1:2: ERROR: Unexpected end of file`);
      assert.strictEqual(error.errors.length, 1);
      assert.strictEqual(error.warnings.length, 0);
    }
  },

  async transformThrow({ esbuild }) {
    try {
      await esbuild.transform(`1+`, {})
      throw new Error('Expected an error to be thrown');
    } catch (error) {
      assert(error instanceof Error, 'Must be an Error object');
      assert.strictEqual(error.message, `Transform failed with 1 error:\n<stdin>:1:2: ERROR: Unexpected end of file`);
      assert.strictEqual(error.errors.length, 1);
      assert.strictEqual(error.warnings.length, 0);
    }
  },

  async formatMessagesSync({ esbuild }) {
    const messages = esbuild.formatMessagesSync([
      { text: 'This is an error' },
      { text: 'Another error', location: { file: 'file.js' } },
    ], {
      kind: 'error',
    })
    assert.strictEqual(messages.length, 2)
    assert.strictEqual(messages[0], `${errorIcon} [ERROR] This is an error\n\n`)
    assert.strictEqual(messages[1], `${errorIcon} [ERROR] Another error\n\n    file.js:0:0:\n      0 │ \n        ╵ ^\n\n`)
  },

  async analyzeMetafileSync({ esbuild }) {
    const metafile = {
      "inputs": {
        "entry.js": {
          "bytes": 50,
          "imports": [
            {
              "path": "lib.js",
              "kind": "import-statement"
            }
          ]
        },
        "lib.js": {
          "bytes": 200,
          "imports": []
        }
      },
      "outputs": {
        "out.js": {
          "imports": [],
          "exports": [],
          "entryPoint": "entry.js",
          "inputs": {
            "entry.js": {
              "bytesInOutput": 25
            },
            "lib.js": {
              "bytesInOutput": 50
            }
          },
          "bytes": 100
        }
      }
    }
    assert.strictEqual(esbuild.analyzeMetafileSync(metafile), `
  out.js       100b   100.0%
   ├ lib.js     50b    50.0%
   └ entry.js   25b    25.0%
`)
    assert.strictEqual(esbuild.analyzeMetafileSync(metafile, { verbose: true }), `
  out.js ────── 100b ── 100.0%
   ├ lib.js ──── 50b ─── 50.0%
   │  └ entry.js
   └ entry.js ── 25b ─── 25.0%
`)
  },
}

let childProcessTests = {
  // More info about this test case: https://github.com/evanw/esbuild/issues/2727
  async testIncrementalChildProcessExit({ testDir, esbuild }) {
    const file = path.join(testDir, 'build.js')

    await writeFileAsync(file, `
      const esbuild = require(${JSON.stringify(esbuild.ESBUILD_PACKAGE_PATH)})
      esbuild.context({
        entryPoints: [],
      })
      .then(context => context.rebuild())
      .then(() => {
        console.log('success')
        process.exit(0)
      })
    `)

    let timeout
    const detectHangPromise = new Promise((_, reject) => {
      timeout = setTimeout(() => {
        reject(new Error('Timed out waiting for keep-alive check to terminate'))
      }, 5 * 60 * 1000)
    })

    const testKeepAlivePingPromise = new Promise((resolve, reject) => {
      child_process.execFile('node', [file], {
        stdio: [
          'inherit',
          'inherit',
          'pipe', // This is important for the test to check for the hang
        ],
      }, (error, stdout, stderr) => {
        clearTimeout(timeout)
        if (error) reject(error)
        else if (stdout !== 'success\n') reject(new Error('Unexpected stdout: ' + JSON.stringify(stdout)))
        else if (stderr !== '') reject(new Error('Unexpected stderr: ' + JSON.stringify(stderr)))
        else resolve()
      })
    })

    await Promise.race([
      detectHangPromise,
      testKeepAlivePingPromise,
    ])
  },

  async testWatchStdoutChildProcess({ testDir, esbuild }) {
    const sequence = [
      {
        input: 'console.log(1+2)',
        stdout: ['console.log(1 + 2);'],
        stderr: ['[watch] build finished, watching for changes...'],
      },
      {
        input: 'console.log(2+3)',
        stdout: ['console.log(2 + 3);'],
        stderr: ['[watch] build started (change: "in.js")', '[watch] build finished'],
      },
      {
        input: 'console.log(3+4)',
        stdout: ['console.log(3 + 4);'],
        stderr: ['[watch] build started (change: "in.js")', '[watch] build finished'],
      },
    ]

    const infile = path.join(testDir, 'in.js')
    const file = path.join(testDir, 'build.js')
    await writeFileAsync(infile, sequence[0].input)
    await writeFileAsync(file, `
      const esbuild = require(${JSON.stringify(esbuild.ESBUILD_PACKAGE_PATH)})
      esbuild.context({
        entryPoints: [${JSON.stringify(infile)}],
        logLevel: 'info',
      }).then(ctx => ctx.watch())
    `)

    // Start the child
    const maxSeconds = 60
    const child = child_process.spawn('node', [file], {
      cwd: testDir,
      stdio: ['inherit', 'pipe', 'pipe'],
      timeout: maxSeconds * 1000,
    })

    // Make sure the child is always killed
    try {
      for (const { input, stdout: expectedStdout, stderr: expectedStderr } of sequence) {
        let totalStdout = ''
        let totalStderr = ''
        let stdoutBuffer = ''
        let stderrBuffer = ''
        const onstdout = data => {
          totalStdout += data
          stdoutBuffer += data
          check()
        }
        const onstderr = data => {
          totalStderr += data
          stderrBuffer += data
          check()
        }
        let check = () => { }

        child.stdout.on('data', onstdout)
        child.stderr.on('data', onstderr)

        await new Promise((resolve, reject) => {
          const seconds = 30
          const timeout = setTimeout(() => reject(new Error(
            `Watch mode + stdout test failed to match expected output after ${seconds} seconds
  input: ${JSON.stringify(input)}
  stdout: ${JSON.stringify(totalStdout)}
  stderr: ${JSON.stringify(totalStderr)}
`)), seconds * 1000)

          check = () => {
            let index

            while ((index = stdoutBuffer.indexOf('\n')) >= 0) {
              const line = stdoutBuffer.slice(0, index)
              stdoutBuffer = stdoutBuffer.slice(index + 1)
              if (line === expectedStdout[0]) expectedStdout.shift()
            }

            while ((index = stderrBuffer.indexOf('\n')) >= 0) {
              const line = stderrBuffer.slice(0, index)
              stderrBuffer = stderrBuffer.slice(index + 1)
              if (line === expectedStderr[0]) expectedStderr.shift()
            }

            if (!expectedStdout.length && !expectedStderr.length) {
              clearTimeout(timeout)
              resolve()
            }
          }

          writeFileAtomic(infile, input)
        })

        child.stdout.off('data', onstdout)
        child.stderr.off('data', onstderr)
      }
    } finally {
      child.kill()
    }
  },
}

async function assertSourceMap(jsSourceMap, source) {
  jsSourceMap = JSON.parse(jsSourceMap)
  assert.deepStrictEqual(jsSourceMap.version, 3)
  assert.deepStrictEqual(jsSourceMap.sources, [source])
  assert.deepStrictEqual(jsSourceMap.sourcesContent, ['let       x'])
  assert.deepStrictEqual(jsSourceMap.mappings, 'AAAA,IAAU;')
}

async function main() {
  const esbuild = installForTests()

  // Create a fresh test directory
  removeRecursiveSync(rootTestDir)
  fs.mkdirSync(rootTestDir)

  // Time out these tests after 5 minutes. This exists to help debug test hangs in CI.
  let minutes = 5
  let timeout = setTimeout(() => {
    console.error(`❌ js api tests timed out after ${minutes} minutes, exiting...`)
    process.exit(1)
  }, minutes * 60 * 1000)

  // Run all tests concurrently
  const runTest = async (name, fn) => {
    let testDir = path.join(rootTestDir, name)
    try {
      await mkdirAsync(testDir)
      await fn({ esbuild, testDir })
      removeRecursiveSync(testDir)
      return true
    } catch (e) {
      console.error(`❌ ${name}: ${e && e.message || e}`)
      return false
    }
  }

  const tests = [
    ...Object.entries(buildTests),
    ...Object.entries(watchTests),
    ...Object.entries(serveTests),
    ...Object.entries(transformTests),
    ...Object.entries(formatTests),
    ...Object.entries(analyzeTests),
    ...Object.entries(syncTests),
    ...Object.entries(childProcessTests),
  ]

  const allTestsPassed = (await Promise.all(tests.map(([name, fn]) => {
    const promise = runTest(name, fn)

    // Time out each individual test after 3 minutes. This exists to help debug test hangs in CI.
    const minutes = 3
    const timeout = setTimeout(() => {
      console.error(`❌ the test "${name}" timed out after ${minutes} minutes, exiting...`)
      process.exit(1)
    }, minutes * 60 * 1000)
    return promise.finally(() => clearTimeout(timeout))
  }))).every(success => success)

  if (!allTestsPassed) {
    console.error(`❌ js api tests failed`)
    process.exit(1)
  } else {
    console.log(`✅ js api tests passed`)
    removeRecursiveSync(rootTestDir)
  }

  clearTimeout(timeout);
}

main().catch(e => setTimeout(() => { throw e }))
