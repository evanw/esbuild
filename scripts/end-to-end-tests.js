(async () => {
  // Make sure this script runs as an ES6 module so we can import both ES6 modules and CommonJS modules
  if (typeof require !== 'undefined') {
    const childProcess = require('child_process')
    const child = childProcess.spawn('node', ['--experimental-modules', '--input-type=module'], {
      cwd: __dirname,
      stdio: ['pipe', 'inherit', 'inherit'],
    })
    child.stdin.write(require('fs').readFileSync(__filename))
    child.stdin.end()
    child.on('close', code => process.exit(code))
    return
  }

  const childProcess = await import('child_process')
  const { default: { buildBinary, dirname, removeRecursiveSync } } = await import('./esbuild.js')
  const assert = await import('assert')
  const path = await import('path')
  const util = await import('util')
  const url = await import('url')
  const fs = (await import('fs')).promises

  const execFileAsync = util.promisify(childProcess.execFile)
  const execAsync = util.promisify(childProcess.exec)

  const testDir = path.join(dirname, '.end-to-end-tests')
  const esbuildPath = buildBinary()
  const tests = []
  let testCount = 0

  // Tests for "--define"
  tests.push(
    test(['--define:foo=null', 'in.js', '--outfile=node.js'], { 'in.js': `if (foo !== null) throw 'fail'` }),
    test(['--define:foo=true', 'in.js', '--outfile=node.js'], { 'in.js': `if (foo !== true) throw 'fail'` }),
    test(['--define:foo=false', 'in.js', '--outfile=node.js'], { 'in.js': `if (foo !== false) throw 'fail'` }),
    test(['--define:foo="abc"', 'in.js', '--outfile=node.js'], { 'in.js': `if (foo !== "abc") throw 'fail'` }),
    test(['--define:foo=123.456', 'in.js', '--outfile=node.js'], { 'in.js': `if (foo !== 123.456) throw 'fail'` }),
    test(['--define:foo=-123.456', 'in.js', '--outfile=node.js'], { 'in.js': `if (foo !== -123.456) throw 'fail'` }),
    test(['--define:foo=global', 'in.js', '--outfile=node.js'], { 'in.js': `foo.bar = 123; if (bar !== 123) throw 'fail'` }),
    test(['--define:foo=bar', 'in.js', '--outfile=node.js'], { 'in.js': `let bar = {x: 123}; if (foo.x !== 123) throw 'fail'` }),
    test(['--define:a.x=1', 'in.js', '--outfile=node.js'], { 'in.js': `if (a.x !== 1) throw 'fail'` }),
    test(['--define:a.x=1', '--define:a.y=2', 'in.js', '--outfile=node.js'], { 'in.js': `if (a.x + a.y !== 3) throw 'fail'` }),
    test(['--define:a.x=1', '--define:b.y=2', 'in.js', '--outfile=node.js'], { 'in.js': `if (a.x + b.y !== 3) throw 'fail'` }),
    test(['--define:a.x=1', '--define:b.x=2', 'in.js', '--outfile=node.js'], { 'in.js': `if (a.x + b.x !== 3) throw 'fail'` }),
    test(['--define:x=y', '--define:y=x', 'in.js', '--outfile=node.js'], {
      'in.js': `eval('var x="x",y="y"'); if (x + y !== 'yx') throw 'fail'`,
    }),
  )

  // Test recursive directory creation
  tests.push(
    test(['entry.js', '--outfile=a/b/c/d/index.js'], {
      'entry.js': `exports.foo = 123`,
      'node.js': `const ns = require('./a/b/c/d'); if (ns.foo !== 123) throw 'fail'`,
    }),
  )

  // Test bogus paths with a file as a parent directory (this happens when you use "pnpx esbuild")
  tests.push(
    test(['entry.js', '--bundle'], {
      'entry.js': `import "./file.js/what/is/this"`,
      'file.js': `some file`,
    }, {
      expectedStderr: ` > entry.js:1:7: error: Could not resolve "./file.js/what/is/this"
    1 │ import "./file.js/what/is/this"
      ╵        ~~~~~~~~~~~~~~~~~~~~~~~~

`,
    }),
  )

  // Test resolving paths with a question mark (an invalid path on Windows)
  tests.push(
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `
        import x from "./file.js?ignore-me"
        if (x !== 123) throw 'fail'
      `,
      'file.js': `export default 123`,
    }),
  )

  // Test coverage for a special JSX error message
  tests.push(
    test(['example.jsx', '--outfile=node.js'], {
      'example.jsx': `let button = <Button content="some so-called \\"button text\\"" />`,
    }, {
      expectedStderr: ` > example.jsx:1:58: error: Unexpected backslash in JSX element
    1 │ let button = <Button content="some so-called \\"button text\\"" />
      ╵                                                           ^
   example.jsx:1:45: note: Quoted JSX attributes use XML-style escapes instead of JavaScript-style escapes
    1 │ let button = <Button content="some so-called \\"button text\\"" />
      │                                              ~~
      ╵                                              &quot;
   example.jsx:1:29: note: Consider using a JavaScript string inside {...} instead of a quoted JSX attribute
    1 │ let button = <Button content="some so-called \\"button text\\"" />
      │                              ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
      ╵                              {"some so-called \\"button text\\""}

`,
    }),
    test(['example.jsx', '--outfile=node.js'], {
      'example.jsx': `let button = <Button content='some so-called \\'button text\\'' />`,
    }, {
      expectedStderr: ` > example.jsx:1:58: error: Unexpected backslash in JSX element
    1 │ let button = <Button content='some so-called \\'button text\\'' />
      ╵                                                           ^
   example.jsx:1:45: note: Quoted JSX attributes use XML-style escapes instead of JavaScript-style escapes
    1 │ let button = <Button content='some so-called \\'button text\\'' />
      │                                              ~~
      ╵                                              &apos;
   example.jsx:1:29: note: Consider using a JavaScript string inside {...} instead of a quoted JSX attribute
    1 │ let button = <Button content='some so-called \\'button text\\'' />
      │                              ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
      ╵                              {'some so-called \\'button text\\''}

`,
    }),
  )

  // Test the "browser" field in "package.json"
  tests.push(
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `require('foo')`,
      'package.json': `{ "browser": { "./foo": "./file" } }`,
      'file.js': `var works = true`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `require('foo')`,
      'package.json': `{ "browser": { "foo": "./file" } }`,
      'file.js': `var works = true`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `require('./foo')`,
      'package.json': `{ "browser": { "./foo": "./file" } }`,
      'file.js': `var works = true`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `require('./foo')`,
      'package.json': `{ "browser": { "foo": "./file" } }`,
      'file.js': `var works = true`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `require('pkg/foo/bar')`,
      'node_modules/pkg/package.json': `{ "browser": { "./foo/bar": "./file" } }`,
      'node_modules/pkg/foo/bar.js': `invalid syntax`,
      'node_modules/pkg/file.js': `var works = true`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `require('pkg/foo/bar')`,
      'node_modules/pkg/package.json': `{ "browser": { "foo/bar": "./file" } }`,
      'node_modules/pkg/foo/bar.js': `invalid syntax`,
      'node_modules/pkg/file.js': `var works = true`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `require('pkg/foo/bar')`,
      'node_modules/pkg/package.json': `{ "browser": { "./foo/bar": "./file" } }`,
      'node_modules/pkg/file.js': `var works = true`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `require('pkg/foo/bar')`,
      'node_modules/pkg/package.json': `{ "browser": { "foo/bar": "./file" } }`,
      'node_modules/pkg/file.js': `var works = true`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `require('pkg')`,
      'node_modules/pkg/index.js': `require('foo/bar')`,
      'node_modules/pkg/package.json': `{ "browser": { "./foo/bar": "./file" } }`,
      'node_modules/pkg/file.js': `var works = true`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `require('pkg')`,
      'node_modules/pkg/index.js': `require('foo/bar')`,
      'node_modules/pkg/package.json': `{ "browser": { "foo/bar": "./file" } }`,
      'node_modules/pkg/file.js': `var works = true`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `require('pkg')`,
      'node_modules/pkg/index.js': `throw 'fail'`,
      'node_modules/pkg/package.json': `{ "browser": { "./index.js": "./file" } }`,
      'node_modules/pkg/file.js': `var works = true`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `require('pkg')`,
      'node_modules/pkg/package.json': `{ "browser": { "./index.js": "./file" } }`,
      'node_modules/pkg/file.js': `var works = true`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `require('pkg')`,
      'node_modules/pkg/index.js': `throw 'fail'`,
      'node_modules/pkg/package.json': `{ "browser": { "./index": "./file" } }`,
      'node_modules/pkg/file.js': `var works = true`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `require('pkg')`,
      'node_modules/pkg/package.json': `{ "browser": { "./index": "./file" } }`,
      'node_modules/pkg/file.js': `var works = true`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `require('pkg')`,
      'node_modules/pkg/main.js': `throw 'fail'`,
      'node_modules/pkg/package.json': `{ "main": "./main",\n  "browser": { "./main.js": "./file" } }`,
      'node_modules/pkg/file.js': `var works = true`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `require('pkg')`,
      'node_modules/pkg/package.json': `{ "main": "./main",\n  "browser": { "./main.js": "./file" } }`,
      'node_modules/pkg/file.js': `var works = true`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `require('pkg')`,
      'package.json': `{ "browser": { "pkg2": "pkg3" } }`,
      'node_modules/pkg/index.js': `require('pkg2')`,
      'node_modules/pkg/package.json': `{ "browser": { "pkg2": "./file" } }`,
      'node_modules/pkg/file.js': `var works = true`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `require('pkg')`,
      'package.json': `{ "browser": { "pkg2": "pkg3" } }`,
      'node_modules/pkg/index.js': `require('pkg2')`,
      'node_modules/pkg2/index.js': `throw 'fail'`,
      'node_modules/pkg3/index.js': `var works = true`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `require('pkg')`,
      'package.json': `{ "browser": { "pkg2": "pkg3" } }`,
      'node_modules/pkg/index.js': `require('pkg2')`,
      'node_modules/pkg/package.json': `{ "browser": { "./pkg2": "./file" } }`,
      'node_modules/pkg/file.js': `var works = true`,
    }),
  )

  // Test arbitrary module namespace identifier names
  // See https://github.com/tc39/ecma262/pull/2154
  tests.push(
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `import {'*' as star} from './export.js'; if (star !== 123) throw 'fail'`,
      'export.js': `let foo = 123; export {foo as '*'}`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `import {'\\0' as bar} from './export.js'; if (bar !== 123) throw 'fail'`,
      'export.js': `let foo = 123; export {foo as '\\0'}`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `import {'\\uD800\\uDC00' as bar} from './export.js'; if (bar !== 123) throw 'fail'`,
      'export.js': `let foo = 123; export {foo as '\\uD800\\uDC00'}`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `import {'🍕' as bar} from './export.js'; if (bar !== 123) throw 'fail'`,
      'export.js': `let foo = 123; export {foo as '🍕'}`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `import {' ' as bar} from './export.js'; if (bar !== 123) throw 'fail'`,
      'export.js': `export let foo = 123; export {foo as ' '} from './export.js'`,
    }),
    test(['entry.js', '--bundle', '--outfile=node.js'], {
      'entry.js': `import {'' as ab} from './export.js'; if (ab.foo !== 123 || ab.bar !== 234) throw 'fail'`,
      'export.js': `export let foo = 123, bar = 234; export * as '' from './export.js'`,
    }),
  )

  // Tests for symlinks
  //
  // Note: These are disabled on Windows because they fail when run with GitHub
  // Actions. I'm not sure what the issue is because they pass for me when run in
  // my Windows VM (Windows 10 in VirtualBox on macOS).
  if (process.platform !== 'win32') {
    tests.push(
      // Without preserve symlinks
      test(['--bundle', 'in.js', '--outfile=node.js'], {
        'in.js': `import {foo} from 'foo'; if (foo !== 123) throw 'fail'`,
        'registry/node_modules/foo/index.js': `export {bar as foo} from 'bar'`,
        'registry/node_modules/bar/index.js': `export const bar = 123`,
        'node_modules/foo': { symlink: `../registry/node_modules/foo` },
      }),
      test(['--bundle', 'in.js', '--outfile=node.js'], {
        'in.js': `import {foo} from 'foo'; if (foo !== 123) throw 'fail'`,
        'registry/node_modules/foo/index.js': `export {bar as foo} from 'bar'`,
        'registry/node_modules/bar/index.js': `export const bar = 123`,
        'node_modules/foo/index.js': { symlink: `../../registry/node_modules/foo/index.js` },
      }),
      test(['--bundle', 'in.js', '--outfile=node.js'], {
        'in.js': `import {foo} from 'foo'; if (foo !== 123) throw 'fail'`,
        'registry/node_modules/foo/index.js': `export {bar as foo} from 'bar'`,
        'registry/node_modules/bar/index.js': `export const bar = 123`,
        'node_modules/foo': { symlink: `TEST_DIR_ABS_PATH/registry/node_modules/foo` },
      }),
      test(['--bundle', 'in.js', '--outfile=node.js'], {
        'in.js': `import {foo} from 'foo'; if (foo !== 123) throw 'fail'`,
        'registry/node_modules/foo/index.js': `export {bar as foo} from 'bar'`,
        'registry/node_modules/bar/index.js': `export const bar = 123`,
        'node_modules/foo/index.js': { symlink: `TEST_DIR_ABS_PATH/registry/node_modules/foo/index.js` },
      }),

      // With preserve symlinks
      test(['--bundle', 'src/in.js', '--outfile=node.js', '--preserve-symlinks'], {
        'src/in.js': `import {foo} from 'foo'; if (foo !== 123) throw 'fail'`,
        'registry/node_modules/foo/index.js': `export {bar as foo} from 'bar'`,
        'src/node_modules/bar/index.js': `export const bar = 123`,
        'src/node_modules/foo': { symlink: `../../registry/node_modules/foo` },
      }),
      test(['--bundle', 'src/in.js', '--outfile=node.js', '--preserve-symlinks'], {
        'src/in.js': `import {foo} from 'foo'; if (foo !== 123) throw 'fail'`,
        'registry/node_modules/foo/index.js': `export {bar as foo} from 'bar'`,
        'src/node_modules/bar/index.js': `export const bar = 123`,
        'src/node_modules/foo/index.js': { symlink: `../../../registry/node_modules/foo/index.js` },
      }),
      test(['--bundle', 'src/in.js', '--outfile=node.js', '--preserve-symlinks'], {
        'src/in.js': `import {foo} from 'foo'; if (foo !== 123) throw 'fail'`,
        'registry/node_modules/foo/index.js': `export {bar as foo} from 'bar'`,
        'src/node_modules/bar/index.js': `export const bar = 123`,
        'src/node_modules/foo': { symlink: `TEST_DIR_ABS_PATH/registry/node_modules/foo` },
      }),
      test(['--bundle', 'src/in.js', '--outfile=node.js', '--preserve-symlinks'], {
        'src/in.js': `import {foo} from 'foo'; if (foo !== 123) throw 'fail'`,
        'registry/node_modules/foo/index.js': `export {bar as foo} from 'bar'`,
        'src/node_modules/bar/index.js': `export const bar = 123`,
        'src/node_modules/foo/index.js': { symlink: `TEST_DIR_ABS_PATH/registry/node_modules/foo/index.js` },
      }),

      // This is a test for https://github.com/evanw/esbuild/issues/222
      test(['--bundle', 'src/in.js', '--outfile=out/node.js', '--metafile=out/meta.json', '--platform=node', '--format=cjs'], {
        'a/b/src/in.js': `
          import {metafile} from './load'
          const assert = require('assert')
          assert.deepStrictEqual(Object.keys(metafile.inputs), ['src/load.js', 'src/in.js'])
          assert.strictEqual(metafile.inputs['src/in.js'].imports[0].path, 'src/load.js')
        `,
        'a/b/src/load.js': `
          export var metafile
          // Hide the import path from the bundler
          try {
            let path = './meta.json'
            metafile = require(path)
          } catch (e) {
          }
        `,
        'node.js': `
          require('./a/b/out/node')
        `,
        'c': { symlink: `a/b` },
      }, { cwd: 'c' }),

      // This is a test for https://github.com/evanw/esbuild/issues/766
      test(['--bundle', 'impl/index.mjs', '--outfile=node.js', '--format=cjs', '--resolve-extensions=.mjs'], {
        'config/yarn/link/@monorepo-source/a': { symlink: `../../../../monorepo-source/packages/a` },
        'config/yarn/link/@monorepo-source/b': { symlink: `../../../../monorepo-source/packages/b` },
        'impl/node_modules/@monorepo-source/b': { symlink: `../../../config/yarn/link/@monorepo-source/b` },
        'impl/index.mjs': `
          import { fn } from '@monorepo-source/b';
          if (fn() !== 123) throw 'fail';
        `,
        'monorepo-source/packages/a/index.mjs': `
          export function foo() { return 123; }
        `,
        'monorepo-source/packages/b/node_modules/@monorepo-source/a': { symlink: `../../../../../config/yarn/link/@monorepo-source/a` },
        'monorepo-source/packages/b/index.mjs': `
          import { foo } from '@monorepo-source/a';
          export function fn() { return foo(); }
        `,
      }),
    )
  }

  // Test custom output paths
  tests.push(
    test(['node=entry.js', '--outdir=.'], {
      'entry.js': ``,
    }),
  )

  // Make sure that the "asm.js" directive is removed
  tests.push(
    test(['in.js', '--outfile=node.js'], {
      'in.js': `
        function foo() { 'use asm'; eval("/* not asm.js */") }
        let emitWarning = process.emitWarning
        let failed = false
        try {
          process.emitWarning = () => failed = true
          foo()
        } finally {
          process.emitWarning = emitWarning
        }
        if (failed) throw 'fail'
      `,
    }),
  )

  // Check object rest lowering
  // https://github.com/evanw/esbuild/issues/956
  tests.push(
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        let v, o = {b: 3, c: 5}, e = ({b: v, ...o} = o);
        if (o === e || o.b !== void 0 || o.c !== 5 || e.b !== 3 || e.c !== 5 || v !== 3) throw 'fail'
      `,
    }),
  )

  // Check object spread lowering
  // https://github.com/evanw/esbuild/issues/1017
  const objectAssignSemantics = `
    var a, b, c, p, s = Symbol('s')

    // Getter
    a = { x: 1 }
    b = { get x() {}, ...a }
    if (b.x !== a.x) throw 'fail: 1'

    // Symbol getter
    a = {}
    a[s] = 1
    p = {}
    Object.defineProperty(p, s, { get: () => {} })
    b = { __proto__: p, ...a }
    if (b[s] !== a[s]) throw 'fail: 2'

    // Non-enumerable
    a = {}
    Object.defineProperty(a, 'x', { value: 1 })
    b = { ...a }
    if (b.x === a.x) throw 'fail: 3'

    // Symbol non-enumerable
    a = {}
    Object.defineProperty(a, s, { value: 1 })
    b = { ...a }
    if (b[s] === a[s]) throw 'fail: 4'

    // Prototype
    a = Object.create({ x: 1 })
    b = { ...a }
    if (b.x === a.x) throw 'fail: 5'

    // Symbol prototype
    p = {}
    p[s] = 1
    a = Object.create(p)
    b = { ...a }
    if (b[s] === a[s]) throw 'fail: 6'

    // Getter evaluation 1
    a = 1
    b = 10
    p = { get x() { return a++ }, ...{ get y() { return b++ } } }
    if (
      p.x !== 1 || p.x !== 2 || p.x !== 3 ||
      p.y !== 10 || p.y !== 10 || p.y !== 10
    ) throw 'fail: 7'

    // Getter evaluation 2
    a = 1
    b = 10
    p = { ...{ get x() { return a++ } }, get y() { return b++ } }
    if (
      p.x !== 1 || p.x !== 1 || p.x !== 1 ||
      p.y !== 10 || p.y !== 11 || p.y !== 12
    ) throw 'fail: 8'

    // Getter evaluation 3
    a = 1
    b = 10
    c = 100
    p = { ...{ get x() { return a++ } }, get y() { return b++ }, ...{ get z() { return c++ } } }
    if (
      p.x !== 1 || p.x !== 1 || p.x !== 1 ||
      p.y !== 10 || p.y !== 11 || p.y !== 12 ||
      p.z !== 100 || p.z !== 100 || p.z !== 100
    ) throw 'fail: 9'

    // Inline prototype property
    p = { ...{ __proto__: null } }
    if (Object.prototype.hasOwnProperty.call(p, '__proto__') || Object.getPrototypeOf(p) === null) throw 'fail: 10'
  `
  tests.push(
    test(['in.js', '--outfile=node.js'], {
      'in.js': objectAssignSemantics,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': objectAssignSemantics,
    }),
    test(['in.js', '--outfile=node.js', '--target=es5'], {
      'in.js': objectAssignSemantics,
    }),
    test(['in.js', '--outfile=node.js', '--minify-syntax'], {
      'in.js': objectAssignSemantics,
    }),
  )

  // Check template literal lowering
  for (const target of ['--target=es5', '--target=es6']) {
    tests.push(
      // Untagged template literals
      test(['in.js', '--outfile=node.js', target], {
        'in.js': `
          var obj = {
            toString: () => 'b',
            valueOf: () => 0,
          }
          if (\`\${obj}\` !== 'b') throw 'fail'
          if (\`a\${obj}\` !== 'ab') throw 'fail'
          if (\`\${obj}c\` !== 'bc') throw 'fail'
          if (\`a\${obj}c\` !== 'abc') throw 'fail'
        `,
        'in.js': `
          var obj = {}
          obj[Symbol.toPrimitive] = hint => {
            if (hint !== 'string') throw 'fail'
            return 'b'
          }
          if (\`\${obj}\` !== 'b') throw 'fail'
          if (\`a\${obj}\` !== 'ab') throw 'fail'
          if (\`\${obj}c\` !== 'bc') throw 'fail'
          if (\`a\${obj}c\` !== 'abc') throw 'fail'
        `,
        'in.js': `
          var list = []
          var trace = x => list.push(x)
          var obj2 = { toString: () => trace(2) }
          var obj4 = { toString: () => trace(4) }
          \`\${trace(1), obj2}\${trace(3), obj4}\`
          if (list.join('') !== '1234') throw 'fail'
        `,
        'in.js': `
          x: {
            try {
              \`\${Symbol('y')}\`
            } catch {
              break x
            }
            throw 'fail'
          }
        `,
      }),

      // Tagged template literals
      test(['in.js', '--outfile=node.js', target], {
        'in.js': `
          if ((x => x[0] === 'y' && x.raw[0] === 'y')\`y\` !== true) throw 'fail'
          if ((x => x[0] === 'y' && x.raw[0] === 'y')\`y\${0}\` !== true) throw 'fail'
          if ((x => x[1] === 'y' && x.raw[1] === 'y')\`\${0}y\` !== true) throw 'fail'
        `,
      }),
      test(['in.js', '--outfile=node.js', target], {
        'in.js': `
          if ((x => x[0] === '\\xFF' && x.raw[0] === '\\\\xFF')\`\\xFF\` !== true) throw 'fail'
          if ((x => x[0] === '\\xFF' && x.raw[0] === '\\\\xFF')\`\\xFF\${0}\` !== true) throw 'fail'
          if ((x => x[1] === '\\xFF' && x.raw[1] === '\\\\xFF')\`\${0}\\xFF\` !== true) throw 'fail'
        `,
      }),
      test(['in.js', '--outfile=node.js', target], {
        'in.js': `
          if ((x => x[0] === void 0 && x.raw[0] === '\\\\u')\`\\u\` !== true) throw 'fail'
          if ((x => x[0] === void 0 && x.raw[0] === '\\\\u')\`\\u\${0}\` !== true) throw 'fail'
          if ((x => x[1] === void 0 && x.raw[1] === '\\\\u')\`\${0}\\u\` !== true) throw 'fail'
        `,
      }),
      test(['in.js', '--outfile=node.js', target], {
        'in.js': `
          if ((x => x !== x.raw)\`y\` !== true) throw 'fail'
        `,
      }),
      test(['in.js', '--outfile=node.js', target], {
        'in.js': `
          if ((x => (x.length = 2, x.length))\`y\` !== 1) throw 'fail'
        `,
      }),
      test(['in.js', '--outfile=node.js', target], {
        'in.js': `
          if ((x => (x.raw.length = 2, x.raw.length))\`y\` !== 1) throw 'fail'
        `,
      }),
      test(['in.js', '--outfile=node.js', target], {
        'in.js': `
          var count = 0
          var foo = () => (() => ++count)\`y\`;
          if (foo() !== 1 || foo() !== 2) throw 'fail'
        `,
      }),
      test(['in.js', '--outfile=node.js', target], {
        'in.js': `
          var foo = () => (x => x)\`y\`;
          if (foo() !== foo()) throw 'fail'
        `,
      }),
      test(['in.js', '--outfile=node.js', target], {
        'in.js': `
          var foo = () => (x => x)\`y\`;
          var bar = () => (x => x)\`y\`;
          if (foo() === bar()) throw 'fail'
        `,
      }),
    )
  }

  let simpleCyclicImportTestCase542 = {
    'in.js': `
      import {Test} from './lib';
      export function fn() {
        return 42;
      }
      export const foo = [Test];
      if (Test.method() !== 42) throw 'fail'
    `,
    'lib.js': `
      import {fn} from './in';
      export class Test {
        static method() {
          return fn();
        }
      }
    `,
  }

  // Test internal import order
  tests.push(
    // See https://github.com/evanw/esbuild/issues/421
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `
        import {foo} from './cjs'
        import {bar} from './esm'
        if (foo !== 1 || bar !== 2) throw 'fail'
      `,
      'cjs.js': `exports.foo = 1; global.internal_import_order_test1 = 2`,
      'esm.js': `export let bar = global.internal_import_order_test1`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `
        if (foo !== 3 || bar !== 4) throw 'fail'
        import {foo} from './cjs'
        import {bar} from './esm'
      `,
      'cjs.js': `exports.foo = 3; global.internal_import_order_test2 = 4`,
      'esm.js': `export let bar = global.internal_import_order_test2`,
    }),

    // See https://github.com/evanw/esbuild/issues/542
    test(['--bundle', 'in.js', '--outfile=node.js'], simpleCyclicImportTestCase542),
    test(['--bundle', 'in.js', '--outfile=node.js', '--format=iife'], simpleCyclicImportTestCase542),
    test(['--bundle', 'in.js', '--outfile=node.js', '--format=iife', '--global-name=someName'], simpleCyclicImportTestCase542),
  )

  // Test CommonJS semantics
  tests.push(
    // "module.require" should work with internal modules
    test(['--bundle', 'in.js', '--outfile=out.js', '--format=cjs'], {
      'in.js': `export {foo, req} from './foo'`,
      'foo.js': `exports.req = module.require; exports.foo = module.require('./bar')`,
      'bar.js': `exports.bar = 123`,
      'node.js': `if (require('./out').foo.bar !== 123 || require('./out').req !== undefined) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=out.js', '--format=cjs'], {
      'in.js': `export {foo, req} from './foo'`,
      'foo.js': `exports.req = module['require']; exports.foo = module['require']('./bar')`,
      'bar.js': `exports.bar = 123`,
      'node.js': `if (require('./out').foo.bar !== 123 || require('./out').req !== undefined) throw 'fail'`,
    }),

    // "module.require" should work with external modules
    test(['--bundle', 'in.js', '--outfile=out.js', '--format=cjs', '--external:fs'], {
      'in.js': `export {foo} from './foo'`,
      'foo.js': `exports.foo = module.require('fs').exists`,
      'node.js': `if (require('./out').foo !== require('fs').exists) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=out.js', '--format=cjs'], {
      'in.js': `export {foo} from './foo'`,
      'foo.js': `let fn = (m, p) => m.require(p); exports.foo = fn(module, 'fs').exists`,
      'node.js': `try { require('./out') } catch (e) { return } throw 'fail'`,
    }),

    // "module.exports" should behave like a normal property
    test(['--bundle', 'in.js', '--outfile=out.js', '--format=cjs'], {
      'in.js': `export {foo} from './foo'`,
      'foo.js': `exports.foo = module.exports`,
      'node.js': `if (require('./out').foo !== require('./out').foo.foo) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=out.js', '--format=cjs'], {
      'in.js': `export {default} from './foo'`,
      'foo.js': `module.exports = 123`,
      'node.js': `if (require('./out').default !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=out.js', '--format=cjs'], {
      'in.js': `export {default} from './foo'`,
      'foo.js': `let m = module; m.exports = 123`,
      'node.js': `if (require('./out').default !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=out.js', '--format=cjs'], {
      'in.js': `export {default} from './foo'`,
      'foo.js': `let fn = (m, x) => m.exports = x; fn(module, 123)`,
      'node.js': `if (require('./out').default !== 123) throw 'fail'`,
    }),

    // Deferred require shouldn't affect import
    test(['--bundle', 'in.js', '--outfile=node.js', '--format=cjs'], {
      'in.js': `
        import { foo } from './a'
        import './b'
        if (foo !== 123) throw 'fail'
      `,
      'a.js': `
        export let foo = 123
      `,
      'b.js': `
        setTimeout(() => require('./a'), 0)
      `,
    }),

    // Test the run-time value of "typeof require"
    test(['--bundle', 'in.js', '--outfile=out.js', '--format=iife'], {
      'in.js': `check(typeof require)`,
      'node.js': `
        const out = require('fs').readFileSync(__dirname + '/out.js', 'utf8')
        const check = x => value = x
        let value
        new Function('check', 'require', out)(check)
        if (value !== 'function') throw 'fail'
      `,
    }),
    test(['--bundle', 'in.js', '--outfile=out.js', '--format=esm'], {
      'in.js': `check(typeof require)`,
      'node.js': `
        import fs from 'fs'
        import path from 'path'
        import url from 'url'
        const __dirname = path.dirname(url.fileURLToPath(import.meta.url))
        const out = fs.readFileSync(__dirname + '/out.js', 'utf8')
        const check = x => value = x
        let value
        new Function('check', 'require', out)(check)
        if (value !== 'function') throw 'fail'
      `,
    }),
    test(['--bundle', 'in.js', '--outfile=out.js', '--format=cjs'], {
      'in.js': `check(typeof require)`,
      'node.js': `
        const out = require('fs').readFileSync(__dirname + '/out.js', 'utf8')
        const check = x => value = x
        let value
        new Function('check', 'require', out)(check)
        if (value !== 'undefined') throw 'fail'
      `,
    }),
  )

  // Test internal CommonJS export
  tests.push(
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `const out = require('./foo'); if (out.__esModule || out.foo !== 123) throw 'fail'`,
      'foo.js': `exports.foo = 123`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `const out = require('./foo'); if (out.__esModule || out !== 123) throw 'fail'`,
      'foo.js': `module.exports = 123`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `const out = require('./foo'); if (!out.__esModule || out.foo !== 123) throw 'fail'`,
      'foo.js': `export const foo = 123`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `const out = require('./foo'); if (!out.__esModule || out.default !== 123) throw 'fail'`,
      'foo.js': `export default 123`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `const out = require('./foo'); if (!out.__esModule || out.default !== null) throw 'fail'`,
      'foo.js': `export default function x() {} x = null`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `const out = require('./foo'); if (!out.__esModule || out.default !== null) throw 'fail'`,
      'foo.js': `export default class x {} x = null`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `
        // This is the JavaScript generated by "tsc" for the following TypeScript:
        //
        //   import fn from './foo'
        //   if (typeof fn !== 'function') throw 'fail'
        //
        "use strict";
        var __importDefault = (this && this.__importDefault) || function (mod) {
          return (mod && mod.__esModule) ? mod : { "default": mod };
        };
        Object.defineProperty(exports, "__esModule", { value: true });
        const foo_1 = __importDefault(require("./foo"));
        if (typeof foo_1.default !== 'function')
          throw 'fail';
      `,
      'foo.js': `export default function fn() {}`,
    }),

    // Self export
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `exports.foo = 123; const out = require('./in'); if (out.__esModule || out.foo !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `module.exports = 123; const out = require('./in'); if (out.__esModule || out !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js', '--format=cjs'], {
      'in.js': `export const foo = 123; const out = require('./in'); if (!out.__esModule || out.foo !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js', '--format=cjs', '--minify'], {
      'in.js': `export const foo = 123; const out = require('./in'); if (!out.__esModule || out.foo !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js', '--format=cjs'], {
      'in.js': `export default 123; const out = require('./in'); if (!out.__esModule || out.default !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js', '--format=esm'], {
      'in.js': `export const foo = 123; const out = require('./in'); if (!out.__esModule || out.foo !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js', '--format=esm', '--minify'], {
      'in.js': `export const foo = 123; const out = require('./in'); if (!out.__esModule || out.foo !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js', '--format=esm'], {
      'in.js': `export default 123; const out = require('./in'); if (!out.__esModule || out.default !== 123) throw 'fail'`,
    }),

    // Test bundled and non-bundled double export star
    test(['node.ts', '--bundle', '--format=cjs', '--outdir=.'], {
      'node.ts': `
        import {a, b} from './re-export'
        if (a !== 'a' || b !== 'b') throw 'fail'
      `,
      're-export.ts': `
        export * from './a'
        export * from './b'
      `,
      'a.ts': `
        export let a = 'a'
      `,
      'b.ts': `
        export let b = 'b'
      `,
    }),
    test(['node.ts', '--bundle', '--format=cjs', '--outdir=.'], {
      'node.ts': `
        import {a, b} from './re-export'
        if (a !== 'a' || b !== 'b') throw 'fail'

        // Try forcing all of these modules to be wrappers
        require('./node')
        require('./re-export')
        require('./a')
        require('./b')
      `,
      're-export.ts': `
        export * from './a'
        export * from './b'
      `,
      'a.ts': `
        export let a = 'a'
      `,
      'b.ts': `
        export let b = 'b'
      `,
    }),
    test(['node.ts', '--bundle', '--format=cjs', '--outdir=.'], {
      'node.ts': `
        import {a, b, c, d} from './re-export'
        if (a !== 'a' || b !== 'b' || c !== 'c' || d !== 'd') throw 'fail'

        // Try forcing all of these modules to be wrappers
        require('./node')
        require('./re-export')
        require('./a')
        require('./b')
      `,
      're-export.ts': `
        export * from './a'
        export * from './b'
        export * from './d'
      `,
      'a.ts': `
        export let a = 'a'
      `,
      'b.ts': `
        exports.b = 'b'
      `,
      'c.ts': `
        exports.c = 'c'
      `,
      'd.ts': `
        export * from './c'
        export let d = 'd'
      `,
    }),
    test(['node.ts', 're-export.ts', 'a.ts', 'b.ts', '--format=cjs', '--outdir=.'], {
      'node.ts': `
        import {a, b} from './re-export'
        if (a !== 'a' || b !== 'b') throw 'fail'
      `,
      're-export.ts': `
        export * from './a'
        export * from './b'
      `,
      'a.ts': `
        export let a = 'a'
      `,
      'b.ts': `
        export let b = 'b'
      `,
    }),
    test(['entry1.js', 'entry2.js', '--splitting', '--bundle', '--format=esm', '--outdir=out'], {
      'entry1.js': `
        import { abc, def, xyz } from './a'
        export default [abc, def, xyz]
      `,
      'entry2.js': `
        import * as x from './b'
        export default x
      `,
      'a.js': `
        export let abc = 'abc'
        export * from './b'
      `,
      'b.js': `
        export * from './c'
        export const def = 'def'
      `,
      'c.js': `
        exports.xyz = 'xyz'
      `,
      'node.js': `
        import entry1 from './out/entry1.js'
        import entry2 from './out/entry2.js'
        if (entry1[0] !== 'abc' || entry1[1] !== 'def' || entry1[2] !== 'xyz') throw 'fail'
        if (entry2.def !== 'def' || entry2.xyz !== 'xyz') throw 'fail'
      `,
    }),

    // Complex circular bundled and non-bundled import case (https://github.com/evanw/esbuild/issues/758)
    test(['node.ts', '--bundle', '--format=cjs', '--outdir=.'], {
      'node.ts': `
        import {a} from './re-export'
        let fn = a()
        if (fn === a || fn() !== a) throw 'fail'
      `,
      're-export.ts': `
        export * from './a'
      `,
      'a.ts': `
        import {b} from './b'
        export let a = () => b
      `,
      'b.ts': `
        import {a} from './re-export'
        export let b = () => a
      `,
    }),
    test(['node.ts', '--bundle', '--format=cjs', '--outdir=.'], {
      'node.ts': `
        import {a} from './re-export'
        let fn = a()
        if (fn === a || fn() !== a) throw 'fail'

        // Try forcing all of these modules to be wrappers
        require('./node')
        require('./re-export')
        require('./a')
        require('./b')
      `,
      're-export.ts': `
        export * from './a'
      `,
      'a.ts': `
        import {b} from './b'
        export let a = () => b
      `,
      'b.ts': `
        import {a} from './re-export'
        export let b = () => a
      `,
    }),
    test(['node.ts', 're-export.ts', 'a.ts', 'b.ts', '--format=cjs', '--outdir=.'], {
      'node.ts': `
        import {a} from './re-export'
        let fn = a()

        // Note: The "void 0" is different here. This case broke when fixing
        // something else ("default" export semantics in node). This test still
        // exists to document this broken behavior.
        if (fn === a || fn() !== void 0) throw 'fail'
      `,
      're-export.ts': `
        export * from './a'
      `,
      'a.ts': `
        import {b} from './b'
        export let a = () => b
      `,
      'b.ts': `
        import {a} from './re-export'
        export let b = () => a
      `,
    }),

    // Use "eval" to access CommonJS variables
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `if (require('./eval').foo !== 123) throw 'fail'`,
      'eval.js': `eval('exports.foo = 123')`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `if (require('./eval').foo !== 123) throw 'fail'`,
      'eval.js': `eval('module.exports = {foo: 123}')`,
    }),
  )

  // Test internal ES6 export
  for (const minify of [[], ['--minify']]) {
    for (const target of ['es5', 'es6']) {
      tests.push(
        test(['--bundle', 'in.js', '--outfile=node.js', '--target=' + target].concat(minify), {
          'in.js': `import * as out from './foo'; if (out.foo !== 123) throw 'fail'`,
          'foo.js': `exports.foo = 123`,
        }),
        test(['--bundle', 'in.js', '--outfile=node.js', '--target=' + target].concat(minify), {
          'in.js': `import * as out from './foo'; if (out.default !== 123) throw 'fail'`,
          'foo.js': `module.exports = 123`,
        }),
        test(['--bundle', 'in.js', '--outfile=node.js', '--target=' + target].concat(minify), {
          'in.js': `import * as out from './foo'; if (out.default !== null) throw 'fail'`,
          'foo.js': `module.exports = null`,
        }),
        test(['--bundle', 'in.js', '--outfile=node.js', '--target=' + target].concat(minify), {
          'in.js': `import * as out from './foo'; if (out.default !== void 0) throw 'fail'`,
          'foo.js': `module.exports = void 0`,
        }),
        test(['--bundle', 'in.js', '--outfile=node.js', '--target=' + target].concat(minify), {
          'in.js': `import * as out from './foo'; if (out.foo !== 123) throw 'fail'`,
          'foo.js': `export var foo = 123`,
        }),
        test(['--bundle', 'in.js', '--outfile=node.js', '--target=' + target].concat(minify), {
          'in.js': `import * as out from './foo'; if (out.default !== 123) throw 'fail'`,
          'foo.js': `export default 123`,
        }),

        // Self export
        test(['--bundle', 'in.js', '--outfile=node.js', '--target=' + target].concat(minify), {
          // Exporting like this doesn't work, but that's ok
          'in.js': `exports.foo = 123; import * as out from './in'; if (out.foo !== undefined) throw 'fail'`,
        }),
        test(['--bundle', 'in.js', '--outfile=node.js', '--target=' + target].concat(minify), {
          // Exporting like this doesn't work, but that's ok
          'in.js': `module.exports = {foo: 123}; import * as out from './in'; if (out.foo !== undefined) throw 'fail'`,
        }),
        test(['--bundle', 'in.js', '--outfile=node.js', '--target=' + target].concat(minify), {
          'in.js': `export var foo = 123; import * as out from './in'; if (out.foo !== 123) throw 'fail'`,
        }),
        test(['--bundle', 'in.js', '--outfile=node.js', '--target=' + target].concat(minify), {
          'in.js': `export default 123; import * as out from './in'; if (out.default !== 123) throw 'fail'`,
        }),

        // Check the value of "this"
        test(['--bundle', 'in.js', '--outfile=node.js', '--target=' + target].concat(minify), {
          'in.js': `import {foo} from './foo'; if (foo() !== (function() { return this })()) throw 'fail'`,
          'foo.js': `export function foo() { return this }`,
        }),
        test(['--bundle', 'in.js', '--outfile=node.js', '--target=' + target].concat(minify), {
          'in.js': `import foo from './foo'; if (foo() !== (function() { return this })()) throw 'fail'`,
          'foo.js': `export default function() { return this }`,
        }),
        test(['--bundle', 'in.js', '--outfile=node.js', '--target=' + target].concat(minify), {
          'in.js': `import {foo} from './foo'; require('./foo'); if (foo() !== (function() { return this })()) throw 'fail'`,
          'foo.js': `export function foo() { return this }`,
        }),
        test(['--bundle', 'in.js', '--outfile=node.js', '--target=' + target].concat(minify), {
          'in.js': `import foo from './foo'; require('./foo'); if (foo() !== (function() { return this })()) throw 'fail'`,
          'foo.js': `export default function() { return this }`,
        }),
        test(['--bundle', '--external:./foo', '--format=cjs', 'in.js', '--outfile=node.js', '--target=' + target].concat(minify), {
          'in.js': `import {foo} from './foo'; if (foo() !== (function() { return this })()) throw 'fail'`,
          'foo.js': `exports.foo = function() { return this }`,
        }),
        test(['--bundle', '--external:./foo', '--format=cjs', 'in.js', '--outfile=node.js', '--target=' + target].concat(minify), {
          'in.js': `import foo from './foo'; if (foo() !== (function() { return this })()) throw 'fail'`,
          'foo.js': `module.exports = function() { return this }`,
        }),
      )
    }

    tests.push(
      // Make sure entry points where a dependency has top-level await are awaited
      test(['--bundle', 'in.js', '--outfile=out.js', '--format=esm'].concat(minify), {
        'in.js': `import './foo'; import('./in.js'); throw 'fail'`,
        'foo.js': `throw await 'stop'`,
        'node.js': `export let async = async () => { try { await import('./out.js') } catch (e) { if (e === 'stop') return } throw 'fail' }`,
      }, { async: true }),

      // Self export
      test(['--bundle', 'in.js', '--outfile=node.js', '--format=esm'].concat(minify), {
        'in.js': `export default 123; export let async = async () => { const out = await import('./in'); if (out.default !== 123) throw 'fail' }`,
      }, { async: true }),
      test(['--bundle', 'in.js', '--outfile=node.js', '--format=esm'].concat(minify), {
        'in.js': `export default 123; import * as out from './in'; export let async = async () => { await import('./in'); if (out.default !== 123) throw 'fail' }`,
      }, { async: true }),

      // Inject
      test(['--bundle', 'node.ts', 'node2.ts', '--outdir=.', '--format=esm', '--inject:foo.js', '--splitting'].concat(minify), {
        'node.ts': `if (foo.bar !== 123) throw 'fail'`,
        'node2.ts': `throw [foo.bar, require('./node2.ts')] // Force this file to be lazily initialized so foo.js is lazily initialized`,
        'foo.js': `export let foo = {bar: 123}`,
      }),
    )
  }

  // Check that duplicate top-level exports don't collide in the presence of "eval"
  tests.push(
    test(['--bundle', '--format=esm', 'in.js', '--outfile=node.js'], {
      'in.js': `
        import a from './a'
        if (a !== 'runner1.js') throw 'fail'
      `,
      'a.js': `
        import { run } from './runner1'
        export default run()
      `,
      'runner1.js': `
        let data = eval('"runner1" + ".js"')
        export function run() { return data }

        // Do this here instead of in "in.js" so that log order is deterministic
        import b from './b'
        if (b !== 'runner2.js') throw 'fail'
      `,
      'b.js': `
        import { run } from './runner2'
        export default run()
      `,
      'runner2.js': `
        let data = eval('"runner2" + ".js"')
        export function run() { return data }
      `,
    }, {
      expectedStderr: ` > runner1.js:2:19: warning: Using direct eval with a bundler is not recommended and may cause problems (more info: https://esbuild.github.io/link/direct-eval)
    2 │         let data = eval('"runner1" + ".js"')
      ╵                    ~~~~

 > runner2.js:2:19: warning: Using direct eval with a bundler is not recommended and may cause problems (more info: https://esbuild.github.io/link/direct-eval)
    2 │         let data = eval('"runner2" + ".js"')
      ╵                    ~~~~

`,
    }),
    test(['--bundle', '--format=esm', '--splitting', 'in.js', 'in2.js', '--outdir=out'], {
      'in.js': `
        import a from './a'
        import b from './b'
        export default [a, b]
      `,
      'a.js': `
        import { run } from './runner1'
        export default run()
      `,
      'runner1.js': `
        let data = eval('"runner1" + ".js"')
        export function run() { return data }
      `,
      'b.js': `
        import { run } from './runner2'
        export default run()
      `,
      'runner2.js': `
        let data = eval('"runner2" + ".js"')
        export function run() { return data }
      `,
      'in2.js': `
        import { run } from './runner2'
        export default run()
      `,
      'node.js': `
        import ab from './out/in.js'
        if (ab[0] !== 'runner1.js' || ab[1] !== 'runner2.js') throw 'fail'
      `,
    }, {
      expectedStderr: ` > runner2.js:2:19: warning: Using direct eval with a bundler is not recommended and may cause problems (more info: https://esbuild.github.io/link/direct-eval)
    2 │         let data = eval('"runner2" + ".js"')
      ╵                    ~~~~

 > runner1.js:2:19: warning: Using direct eval with a bundler is not recommended and may cause problems (more info: https://esbuild.github.io/link/direct-eval)
    2 │         let data = eval('"runner1" + ".js"')
      ╵                    ~~~~

`,
    }),
  )

  // Test "default" exports in ESM-to-CommonJS conversion scenarios
  tests.push(
    test(['in.js', '--outfile=node.js', '--format=cjs'], {
      'in.js': `import def from './foo'; if (def !== 123) throw 'fail'`,
      'foo.js': `exports.__esModule = true; exports.default = 123`,
    }),
    test(['in.js', '--outfile=node.js', '--format=cjs'], {
      'in.js': `import * as ns from './foo'; if (ns.default !== 123) throw 'fail'`,
      'foo.js': `exports.__esModule = true; exports.default = 123`,
    }),
    test(['in.js', '--outfile=node.js', '--format=cjs'], {
      'in.js': `import def from './foo'; if (!def || def.foo !== 123) throw 'fail'`,
      'foo.js': `exports.__esModule = true; exports.foo = 123`,
    }),
    test(['in.js', '--outfile=node.js', '--format=cjs'], {
      'in.js': `import * as ns from './foo'; if (!ns.default || ns.default.foo !== 123) throw 'fail'`,
      'foo.js': `exports.__esModule = true; exports.foo = 123`,
    }),
  )

  // Test external CommonJS export
  tests.push(
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=cjs'], {
      'foo.js': `exports.foo = 123`,
      'node.js': `const out = require('./out'); if (out.__esModule || out.foo !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=cjs'], {
      'foo.js': `module.exports = 123`,
      'node.js': `const out = require('./out'); if (out.__esModule || out !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=esm'], {
      'foo.js': `exports.foo = 123`,
      'node.js': `import out from './out.js'; if (out.foo !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=esm'], {
      'foo.js': `module.exports = 123`,
      'node.js': `import out from './out.js'; if (out !== 123) throw 'fail'`,
    }),
  )

  // Test external ES6 export
  tests.push(
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=cjs'], {
      'foo.js': `export const foo = 123`,
      'node.js': `const out = require('./out'); if (!out.__esModule || out.foo !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=cjs'], {
      'foo.js': `export default 123`,
      'node.js': `const out = require('./out'); if (!out.__esModule || out.default !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=esm'], {
      'foo.js': `export const foo = 123`,
      'node.js': `import {foo} from './out.js'; if (foo !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=esm'], {
      'foo.js': `export default 123`,
      'node.js': `import out from './out.js'; if (out !== 123) throw 'fail'`,
    }),

    // External package
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=cjs', '--external:fs'], {
      'foo.js': `import {exists} from "fs"; export {exists}`,
      'node.js': `const out = require('./out'); if (!out.__esModule || out.exists !== require('fs').exists) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=esm', '--external:fs'], {
      'foo.js': `import {exists} from "fs"; export {exists}`,
      'node.js': `import {exists} from "./out.js"; import * as fs from "fs"; if (exists !== fs.exists) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=cjs', '--external:fs'], {
      'foo.js': `import * as fs from "fs"; export let exists = fs.exists`,
      'node.js': `const out = require('./out'); if (!out.__esModule || out.exists !== require('fs').exists) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=esm', '--external:fs'], {
      'foo.js': `import * as fs from "fs"; export let exists = fs.exists`,
      'node.js': `import {exists} from "./out.js"; import * as fs from "fs"; if (exists !== fs.exists) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=cjs', '--external:fs'], {
      'foo.js': `export {exists} from "fs"`,
      'node.js': `const out = require('./out'); if (!out.__esModule || out.exists !== require('fs').exists) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=esm', '--external:fs'], {
      'foo.js': `export {exists} from "fs"`,
      'node.js': `import {exists} from "./out.js"; import * as fs from "fs"; if (exists !== fs.exists) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=cjs', '--external:fs'], {
      'foo.js': `export * from "fs"`,
      'node.js': `const out = require('./out'); if (!out.__esModule || out.exists !== require('fs').exists) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=esm', '--external:fs'], {
      'foo.js': `export * from "fs"`,
      'node.js': `import {exists} from "./out.js"; import * as fs from "fs"; if (exists !== fs.exists) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=cjs', '--external:fs'], {
      'foo.js': `export * as star from "fs"`,
      'node.js': `const out = require('./out'); if (!out.__esModule || out.star.exists !== require('fs').exists) throw 'fail'`,
    }),
    test(['--bundle', 'foo.js', '--outfile=out.js', '--format=esm', '--external:fs'], {
      'foo.js': `export * as star from "fs"`,
      'node.js': `import {star} from "./out.js"; import * as fs from "fs"; if (star.exists !== fs.exists) throw 'fail'`,
    }),
  )

  // ES6 export star of CommonJS module
  tests.push(
    // Internal
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import * as ns from './re-export'; if (ns.foo !== 123) throw 'fail'`,
      're-export.js': `export * from './commonjs'`,
      'commonjs.js': `exports.foo = 123`,
    }),
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import {foo} from './re-export'; if (foo !== 123) throw 'fail'`,
      're-export.js': `export * from './commonjs'`,
      'commonjs.js': `exports.foo = 123`,
    }),

    // External
    test(['--bundle', 'entry.js', '--outfile=node.js', '--external:fs'], {
      'entry.js': `import * as ns from './re-export'; if (typeof ns.exists !== 'function') throw 'fail'`,
      're-export.js': `export * from 'fs'`,
    }),
    test(['--bundle', 'entry.js', '--outfile=node.js', '--external:fs'], {
      'entry.js': `import {exists} from './re-export'; if (typeof exists !== 'function') throw 'fail'`,
      're-export.js': `export * from 'fs'`,
    }),

    // External (masked)
    test(['--bundle', 'entry.js', '--outfile=node.js', '--external:fs'], {
      'entry.js': `import * as ns from './re-export'; if (ns.exists !== 123) throw 'fail'`,
      're-export.js': `export * from 'fs'; export let exists = 123`,
    }),
    test(['--bundle', 'entry.js', '--outfile=node.js', '--external:fs'], {
      'entry.js': `import {exists} from './re-export'; if (exists !== 123) throw 'fail'`,
      're-export.js': `export * from 'fs'; export let exists = 123`,
    }),

    // Export CommonJS export from ES6 module
    test(['--bundle', 'entry.js', '--outfile=out.js', '--format=cjs'], {
      'entry.js': `export {bar} from './foo'`,
      'foo.js': `exports.bar = 123`,
      'node.js': `const out = require('./out.js'); if (out.bar !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'entry.js', '--outfile=out.js', '--format=esm'], {
      'entry.js': `export {bar} from './foo'`,
      'foo.js': `exports.bar = 123`,
      'node.js': `import {bar} from './out.js'; if (bar !== 123) throw 'fail'`,
    }),
  )

  // Test imports from modules without any imports
  tests.push(
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `
        import * as ns from 'pkg'
        if (ns.default === void 0) throw 'fail'
      `,
      'node_modules/pkg/index.js': ``,
    }, {}),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `
        import * as ns from 'pkg/index.cjs'
        if (ns.default === void 0) throw 'fail'
      `,
      'node_modules/pkg/index.cjs': ``,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `
        import * as ns from 'pkg/index.mjs'
        if (ns.default !== void 0) throw 'fail'
      `,
      'node_modules/pkg/index.mjs': ``,
    }, {
      expectedStderr: ` > in.js:3:15: warning: Import "default" will always be undefined because there is no matching export
    3 │         if (ns.default !== void 0) throw 'fail'
      ╵                ~~~~~~~

`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `
        import * as ns from 'pkg'
        if (ns.default === void 0) throw 'fail'
      `,
      'node_modules/pkg/package.json': `{
        "type": "commonjs"
      }`,
      'node_modules/pkg/index.js': ``,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `
        import * as ns from 'pkg'
        if (ns.default !== void 0) throw 'fail'
      `,
      'node_modules/pkg/package.json': `{
        "type": "module"
      }`,
      'node_modules/pkg/index.js': ``,
    }, {
      expectedStderr: ` > in.js:3:15: warning: Import "default" will always be undefined because there is no matching export
    3 │         if (ns.default !== void 0) throw 'fail'
      ╵                ~~~~~~~

`,
    }),
  )

  // Test imports not being able to access the namespace object
  tests.push(
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `
        import {foo} from './esm'
        if (foo !== 123) throw 'fail'
      `,
      'esm.js': `Object.defineProperty(exports, 'foo', {value: 123, enumerable: false})`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `
        import * as ns from './esm'
        if (ns[Math.random() < 2 && 'foo'] !== 123) throw 'fail'
      `,
      'esm.js': `Object.defineProperty(exports, 'foo', {value: 123, enumerable: false})`,
    }),
  )

  // Test imports of properties from the prototype chain of "module.exports" for Webpack compatibility
  tests.push(
    // Imports
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `
        import def from './cjs-proto'
        import {prop} from './cjs-proto'
        if (def.prop !== 123 || prop !== 123) throw 'fail'
      `,
      'cjs-proto.js': `module.exports = Object.create({prop: 123})`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `
        import def, {prop} from './cjs-proto' // The TypeScript compiler fails with this syntax
        if (def.prop !== 123 || prop !== 123) throw 'fail'
      `,
      'cjs-proto.js': `module.exports = Object.create({prop: 123})`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `
        import * as star from './cjs-proto'
        if (!star.default || star.default.prop !== 123 || star.prop !== 123) throw 'fail'
      `,
      'cjs-proto.js': `module.exports = Object.create({prop: 123})`,
    }),

    // Re-exports
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `
        import * as test from './reexport'
        if (test.def.prop !== 123 || test.prop !== 123) throw 'fail'
      `,
      'reexport.js': `
        export {default as def} from './cjs-proto'
        export {prop} from './cjs-proto'
      `,
      'cjs-proto.js': `module.exports = Object.create({prop: 123})`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `
        import * as test from './reexport'
        if (test.def.prop !== 123 || test.prop !== 123) throw 'fail'
      `,
      'reexport.js': `
        export {default as def, prop} from './cjs-proto' // The TypeScript compiler fails with this syntax
      `,
      'cjs-proto.js': `module.exports = Object.create({prop: 123})`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `
        import * as test from './reexport'
        // Note: the specification says to ignore default exports in "export * from"
        // Note: re-exporting prototype properties using "export * from" is not supported
        if (test.default || test.prop !== void 0) throw 'fail'
      `,
      'reexport.js': `
        export * from './cjs-proto'
      `,
      'cjs-proto.js': `module.exports = Object.create({prop: 123})`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `
        import {star} from './reexport'
        if (!star.default || star.default.prop !== 123 || star.prop !== 123) throw 'fail'
      `,
      'reexport.js': `
        export * as star from './cjs-proto'
      `,
      'cjs-proto.js': `module.exports = Object.create({prop: 123})`,
    }),
  )

  // Test for format conversion without bundling
  tests.push(
    // ESM => ESM
    test(['in.js', '--outfile=node.js', '--format=esm'], {
      'in.js': `
        import {exists} from 'fs'
        if (!exists) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--format=esm'], {
      'in.js': `
        import fs from 'fs'
        if (!fs.exists) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--format=esm'], {
      'in.js': `
        import * as fs from 'fs'
        if (!fs.exists) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--format=esm'], {
      'in.js': `
        let fn = async () => {
          let fs = await import('fs')
          if (!fs.exists) throw 'fail'
        }
        export {fn as async}
      `,
    }, { async: true }),
    test(['in.js', '--outfile=out.js', '--format=esm'], {
      'in.js': `
        export let foo = 'abc'
        export default function() {
          return 123
        }
      `,
      'node.js': `
        import * as out from './out.js'
        if (out.foo !== 'abc' || out.default() !== 123) throw 'fail'
      `,
    }),

    // ESM => CJS
    test(['in.js', '--outfile=node.js', '--format=cjs'], {
      'in.js': `
        import {exists} from 'fs'
        if (!exists) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--format=cjs'], {
      'in.js': `
        import fs from 'fs'
        if (!fs.exists) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--format=cjs'], {
      'in.js': `
        import * as fs from 'fs'
        if (!fs.exists) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--format=cjs'], {
      'in.js': `
        let fn = async () => {
          let fs = await import('fs')
          if (!fs.exists) throw 'fail'
        }
        export {fn as async}
      `,
    }, { async: true }),
    test(['in.js', '--outfile=out.js', '--format=cjs'], {
      'in.js': `
        export let foo = 'abc'
        export default function() {
          return 123
        }
      `,
      'node.js': `
        const out = require('./out.js')
        if (out.foo !== 'abc' || out.default() !== 123) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=out.cjs', '--format=cjs', '--platform=node'], {
      'in.js': `
        export let foo = 123
        let bar = 234
        export { bar as if }
        export default 345
      `,
      'node.js': `
        exports.async = async () => {
          let out = await import('./out.cjs')
          let keys = Object.keys(out)
          if (
            !keys.includes('default') || !keys.includes('foo') || !keys.includes('if') ||
            out.foo !== 123 || out.if !== 234 ||
            out.default.foo !== 123 || out.default.if !== 234 || out.default.default !== 345
          ) throw 'fail'
        }
      `,
    }, { async: true }),

    // ESM => IIFE
    test(['in.js', '--outfile=node.js', '--format=iife'], {
      'in.js': `
        import {exists} from 'fs'
        if (!exists) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--format=iife'], {
      'in.js': `
        import fs from 'fs'
        if (!fs.exists) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--format=iife'], {
      'in.js': `
        import * as fs from 'fs'
        if (!fs.exists) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=out.js', '--format=iife', '--global-name=test'], {
      'in.js': `
        let fn = async () => {
          let fs = await import('fs')
          if (!fs.exists) throw 'fail'
        }
        export {fn as async}
      `,
      'node.js': `
        const code = require('fs').readFileSync(__dirname + '/out.js', 'utf8')
        const out = new Function('require', code + '; return test')(require)
        exports.async = out.async
      `,
    }, { async: true }),
    test(['in.js', '--outfile=out.js', '--format=iife', '--global-name=test'], {
      'in.js': `
        export let foo = 'abc'
        export default function() {
          return 123
        }
      `,
      'node.js': `
        const code = require('fs').readFileSync(__dirname + '/out.js', 'utf8')
        const out = new Function(code + '; return test')()
        if (out.foo !== 'abc' || out.default() !== 123) throw 'fail'
      `,
    }),

    // JSON
    test(['in.json', '--outfile=out.js', '--format=esm'], {
      'in.json': `{"foo": 123}`,
      'node.js': `
        import def from './out.js'
        import {foo} from './out.js'
        if (foo !== 123 || def.foo !== 123) throw 'fail'
      `,
    }),
    test(['in.json', '--outfile=out.js', '--format=cjs'], {
      'in.json': `{"foo": 123}`,
      'node.js': `
        const out = require('./out.js')
        if (out.foo !== 123) throw 'fail'
      `,
    }),
    test(['in.json', '--outfile=out.js', '--format=iife', '--global-name=test'], {
      'in.json': `{"foo": 123}`,
      'node.js': `
        const code = require('fs').readFileSync(__dirname + '/out.js', 'utf8')
        const out = new Function(code + '; return test')()
        if (out.foo !== 123) throw 'fail'
      `,
    }),

    // CJS => CJS
    test(['in.js', '--outfile=node.js', '--format=cjs'], {
      'in.js': `
        const {exists} = require('fs')
        if (!exists) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--format=cjs'], {
      'in.js': `
        const fs = require('fs')
        if (!fs.exists) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=out.js', '--format=cjs'], {
      'in.js': `
        module.exports = 123
      `,
      'node.js': `
        const out = require('./out.js')
        if (out !== 123) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=out.js', '--format=cjs'], {
      'in.js': `
        exports.foo = 123
      `,
      'node.js': `
        const out = require('./out.js')
        if (out.foo !== 123) throw 'fail'
      `,
    }),

    // CJS => IIFE
    test(['in.js', '--outfile=out.js', '--format=iife'], {
      'in.js': `
        const {exists} = require('fs')
        if (!exists) throw 'fail'
      `,
      'node.js': `
        const code = require('fs').readFileSync(__dirname + '/out.js', 'utf8')
        new Function('require', code)(require)
      `,
    }),
    test(['in.js', '--outfile=out.js', '--format=iife'], {
      'in.js': `
        const fs = require('fs')
        if (!fs.exists) throw 'fail'
      `,
      'node.js': `
        const code = require('fs').readFileSync(__dirname + '/out.js', 'utf8')
        new Function('require', code)(require)
      `,
    }),
    test(['in.js', '--outfile=out.js', '--format=iife', '--global-name=test'], {
      'in.js': `
        module.exports = 123
      `,
      'node.js': `
        const code = require('fs').readFileSync(__dirname + '/out.js', 'utf8')
        const out = new Function(code + '; return test')()
        if (out !== 123) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=out.js', '--format=iife', '--global-name=test'], {
      'in.js': `
        exports.foo = 123
      `,
      'node.js': `
        const code = require('fs').readFileSync(__dirname + '/out.js', 'utf8')
        const out = new Function(code + '; return test')()
        if (out.foo !== 123) throw 'fail'
      `,
    }),

    // CJS => ESM
    test(['in.js', '--outfile=out.js', '--format=esm'], {
      'in.js': `
        const {exists} = require('fs')
        if (!exists) throw 'fail'
      `,
      'node.js': `
        let fn = async () => {
          let error
          await import('./out.js').catch(x => error = x)
          if (!error || !error.message.includes('require is not defined')) throw 'fail'
        }
        export {fn as async}
      `,
    }, {
      async: true,
      expectedStderr: ` > in.js:2:25: warning: Converting "require" to "esm" is currently not supported
    2 │         const {exists} = require('fs')
      ╵                          ~~~~~~~

`,
    }),
    test(['in.js', '--outfile=out.js', '--format=esm'], {
      'in.js': `
        const fs = require('fs')
        if (!fs.exists) throw 'fail'
      `,
      'node.js': `
        let fn = async () => {
          let error
          await import('./out.js').catch(x => error = x)
          if (!error || !error.message.includes('require is not defined')) throw 'fail'
        }
        export {fn as async}
      `,
    }, {
      async: true,
      expectedStderr: ` > in.js:2:19: warning: Converting "require" to "esm" is currently not supported
    2 │         const fs = require('fs')
      ╵                    ~~~~~~~

`,
    }),
    test(['in.js', '--outfile=out.js', '--format=esm'], {
      'in.js': `
        module.exports = 123
      `,
      'node.js': `
        import out from './out.js'
        if (out !== 123) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=out.js', '--format=esm'], {
      'in.js': `
        exports.foo = 123
      `,
      'node.js': `
        import out from './out.js'
        if (out.foo !== 123) throw 'fail'
      `,
    }),
  )

  // This shouldn't cause a syntax error
  // https://github.com/evanw/esbuild/issues/1082
  tests.push(
    test(['in.js', '--outfile=node.js', '--minify', '--bundle'], {
      'in.js': `
        return import('./in.js')
      `,
    }),
  )

  // Check for file names of wrapped modules in non-minified stack traces (for profiling)
  // Context: https://github.com/evanw/esbuild/pull/1236
  tests.push(
    test(['entry.js', '--outfile=node.js', '--bundle'], {
      'entry.js': `
        try {
          require('./src/a')
        } catch (e) {
          if (!e.stack.includes('at __require') || !e.stack.includes('at src/a.ts') || !e.stack.includes('at src/b.ts'))
            throw new Error(e.stack)
        }
      `,
      'src/a.ts': `require('./b')`,
      'src/b.ts': `throw new Error('fail')`,
    }),
    test(['entry.js', '--outfile=node.js', '--bundle', '--minify-identifiers'], {
      'entry.js': `
        try {
          require('./src/a')
        } catch (e) {
          if (e.stack.includes('at __require') || e.stack.includes('at src/a.ts') || e.stack.includes('at src/b.ts'))
            throw new Error(e.stack)
        }
      `,
      'src/a.ts': `require('./b')`,
      'src/b.ts': `throw new Error('fail')`,
    }),
    test(['entry.js', '--outfile=node.js', '--bundle'], {
      'entry.js': `
        try {
          require('./src/a')
        } catch (e) {
          if (!e.stack.includes('at __init') || !e.stack.includes('at src/a.ts') || !e.stack.includes('at src/b.ts'))
            throw new Error(e.stack)
        }
      `,
      'src/a.ts': `export let esm = true; require('./b')`,
      'src/b.ts': `export let esm = true; throw new Error('fail')`,
    }),
    test(['entry.js', '--outfile=node.js', '--bundle', '--minify-identifiers'], {
      'entry.js': `
        try {
          require('./src/a')
        } catch (e) {
          if (e.stack.includes('at __init') || e.stack.includes('at src/a.ts') || e.stack.includes('at src/b.ts'))
            throw new Error(e.stack)
        }
      `,
      'src/a.ts': `export let esm = true; require('./b')`,
      'src/b.ts': `export let esm = true; throw new Error('fail')`,
    }),
  )

  // This shouldn't crash
  // https://github.com/evanw/esbuild/issues/1080
  tests.push(
    // Various CommonJS cases
    test(['in.js', '--outfile=node.js', '--define:foo={"x":0}', '--bundle'], {
      'in.js': `if (foo.x !== 0) throw 'fail'; return`,
    }),
    test(['in.js', '--outfile=node.js', '--define:foo.bar={"x":0}', '--bundle'], {
      'in.js': `if (foo.bar.x !== 0) throw 'fail'; return`,
    }),
    test(['in.js', '--outfile=node.js', '--define:module={"x":0}', '--bundle'], {
      'in.js': `if (module.x !== void 0) throw 'fail'; return`,
    }),
    test(['in.js', '--outfile=node.js', '--define:module.foo={"x":0}', '--bundle'], {
      'in.js': `if (module.foo !== void 0) throw 'fail'; return`,
    }),
    test(['in.js', '--outfile=node.js', '--define:exports={"x":0}', '--bundle'], {
      'in.js': `if (exports.x !== void 0) throw 'fail'; return`,
    }),
    test(['in.js', '--outfile=node.js', '--define:exports.foo={"x":0}', '--bundle'], {
      'in.js': `if (exports.foo !== void 0) throw 'fail'; return`,
    }),
    test(['in.js', '--outfile=node.js', '--define:foo=["x"]', '--bundle'], {
      'in.js': `if (foo[0] !== 'x') throw 'fail'; return`,
    }),
    test(['in.js', '--outfile=node.js', '--define:foo.bar=["x"]', '--bundle'], {
      'in.js': `if (foo.bar[0] !== 'x') throw 'fail'; return`,
    }),
    test(['in.js', '--outfile=node.js', '--define:module=["x"]', '--bundle'], {
      'in.js': `if (module[0] !== void 0) throw 'fail'; return`,
    }),
    test(['in.js', '--outfile=node.js', '--define:module.foo=["x"]', '--bundle'], {
      'in.js': `if (module.foo !== void 0) throw 'fail'; return`,
    }),
    test(['in.js', '--outfile=node.js', '--define:exports=["x"]', '--bundle'], {
      'in.js': `if (exports[0] !== void 0) throw 'fail'; return`,
    }),
    test(['in.js', '--outfile=node.js', '--define:exports.foo=["x"]', '--bundle'], {
      'in.js': `if (exports.foo !== void 0) throw 'fail'; return`,
    }),

    // Various ESM cases
    test(['in.js', '--outfile=node.js', '--bundle', '--log-level=error'], {
      'in.js': `import "pkg"`,
      'node_modules/pkg/package.json': `{ "sideEffects": false }`,
      'node_modules/pkg/index.js': `module.exports = null; throw 'fail'`,
    }),
    test(['in.js', '--outfile=node.js', '--define:foo={"x":0}', '--bundle'], {
      'in.js': `if (foo.x !== 0) throw 'fail'; export {}`,
    }),
    test(['in.js', '--outfile=node.js', '--define:foo.bar={"x":0}', '--bundle'], {
      'in.js': `if (foo.bar.x !== 0) throw 'fail'; export {}`,
    }),
    test(['in.js', '--outfile=node.js', '--define:module={"x":0}', '--bundle'], {
      'in.js': `if (module.x !== 0) throw 'fail'; export {}`,
    }),
    test(['in.js', '--outfile=node.js', '--define:module.foo={"x":0}', '--bundle'], {
      'in.js': `if (module.foo.x !== 0) throw 'fail'; export {}`,
    }),
    test(['in.js', '--outfile=node.js', '--define:exports={"x":0}', '--bundle'], {
      'in.js': `if (exports.x !== 0) throw 'fail'; export {}`,
    }),
    test(['in.js', '--outfile=node.js', '--define:exports.foo={"x":0}', '--bundle'], {
      'in.js': `if (exports.foo.x !== 0) throw 'fail'; export {}`,
    }),
    test(['in.js', '--outfile=node.js', '--define:foo=["x"]', '--bundle'], {
      'in.js': `if (foo[0] !== 'x') throw 'fail'; export {}`,
    }),
    test(['in.js', '--outfile=node.js', '--define:foo.bar=["x"]', '--bundle'], {
      'in.js': `if (foo.bar[0] !== 'x') throw 'fail'; export {}`,
    }),
    test(['in.js', '--outfile=node.js', '--define:module=["x"]', '--bundle'], {
      'in.js': `if (module[0] !== 'x') throw 'fail'; export {}`,
    }),
    test(['in.js', '--outfile=node.js', '--define:module.foo=["x"]', '--bundle'], {
      'in.js': `if (module.foo[0] !== 'x') throw 'fail'; export {}`,
    }),
    test(['in.js', '--outfile=node.js', '--define:exports=["x"]', '--bundle'], {
      'in.js': `if (exports[0] !== 'x') throw 'fail'; export {}`,
    }),
    test(['in.js', '--outfile=node.js', '--define:exports.foo=["x"]', '--bundle'], {
      'in.js': `if (exports.foo[0] !== 'x') throw 'fail'; export {}`,
    }),
  )

  // Check for "sideEffects: false" wrapper handling
  // https://github.com/evanw/esbuild/issues/1088
  for (const pkgJSON of [`{}`, `{"sideEffects": false}`]) {
    for (const entry of [
      `export let async = async () => { if (require("pkg").foo() !== 123) throw 'fail' }`,
      `export let async = () => import("pkg").then(x => { if (x.foo() !== 123) throw 'fail' })`,
    ]) {
      for (const index of [`export {foo} from "./foo.js"`, `import {foo} from "./foo.js"; export {foo}`]) {
        for (const foo of [`export let foo = () => 123`, `exports.foo = () => 123`]) {
          tests.push(test(['in.js', '--outfile=node.js', '--bundle'], {
            'in.js': entry,
            'node_modules/pkg/package.json': pkgJSON,
            'node_modules/pkg/index.js': index,
            'node_modules/pkg/foo.js': foo,
          }, { async: true }))
        }
      }
    }
    for (const entry of [
      `export let async = async () => { try { require("pkg") } catch (e) { return } throw 'fail' }`,
      `export let async = () => import("pkg").then(x => { throw 'fail' }, () => {})`,
    ]) {
      tests.push(test(['in.js', '--outfile=node.js', '--bundle'], {
        'in.js': entry,
        'node_modules/pkg/package.json': pkgJSON,
        'node_modules/pkg/index.js': `
          export {foo} from './b.js'
        `,
        'node_modules/pkg/b.js': `
          export {foo} from './c.js'
          throw 'stop'
        `,
        'node_modules/pkg/c.js': `
          export let foo = () => 123
        `,
      }, { async: true }))
    }
  }

  // Tests for "arguments" scope issues
  tests.push(
    test(['in.js', '--outfile=node.js', '--minify'], {
      'in.js': `
        function arguments() {
          return arguments.length
        }
        if (arguments(0, 1) !== 2) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--minify'], {
      'in.js': `
        let value = (function arguments() {
          return arguments.length
        })(0, 1)
        if (value !== 2) throw 'fail'
      `,
    }),
  )

  // Tests for catch scope issues
  tests.push(
    test(['in.js', '--outfile=node.js', '--minify'], {
      'in.js': `
        var x = 0, y = []
        try {
          throw 1
        } catch (x) {
          y.push(x)
          var x = 2
          y.push(x)
        }
        y.push(x)
        if (y + '' !== '1,2,0') throw 'fail: ' + y
      `,
    }),
    test(['in.js', '--outfile=node.js', '--minify'], {
      'in.js': `
        var x = 0, y = []
        try {
          throw 1
        } catch (x) {
          y.push(x)
          var x = 2
          y.push(x)
        }
        finally { x = 3 }
        y.push(x)
        if (y + '' !== '1,2,3') throw 'fail: ' + y
      `,
    }),
    test(['in.js', '--outfile=node.js', '--minify'], {
      'in.js': `
        var y = []
        try {
          throw 1
        } catch (x) {
          y.push(x)
          var x = 2
          y.push(x)
        }
        y.push(x)
        if (y + '' !== '1,2,') throw 'fail: ' + y
      `,
    }),
    test(['in.js', '--outfile=node.js', '--minify'], {
      'in.js': `
        var y = []
        try {
          throw 1
        } catch (x) {
          y.push(x)
          x = 2
          y.push(x)
        }
        y.push(typeof x)
        if (y + '' !== '1,2,undefined') throw 'fail: ' + y
      `,
    }),
    test(['in.js', '--outfile=node.js', '--minify'], {
      'in.js': `
        var y = []
        try {
          throw 1
        } catch (x) {
          y.push(x)
          try {
            throw 2
          } catch (x) {
            y.push(x)
            var x = 3
            y.push(x)
          }
          y.push(x)
        }
        y.push(x)
        if (y + '' !== '1,2,3,1,') throw 'fail: ' + y
      `,
    }),
    test(['in.js', '--outfile=node.js', '--minify'], {
      'in.js': `
        var y = []
        try { x; y.push('fail') } catch (e) {}
        try {
          throw 1
        } catch (x) {
          y.push(x)
        }
        try { x; y.push('fail') } catch (e) {}
        if (y + '' !== '1') throw 'fail: ' + y
      `,
    }),
  )

  // Test cyclic import issues (shouldn't crash on evaluation)
  tests.push(
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import * as foo from './foo'; export default {foo, bar: require('./bar')}`,
      'foo.js': `import * as a from './entry'; import * as b from './bar'; export default {a, b}`,
      'bar.js': `const entry = require('./entry'); export function foo() { return entry }`,
    }),
  )

  // Test directive preservation
  tests.push(
    // The "__pow" symbol must not be hoisted above "use strict"
    test(['entry.js', '--outfile=node.js', '--target=es6'], {
      'entry.js': `
        'use strict'
        function f(a) {
          a **= 2
          return [a, arguments[0]]
        }
        let pair = f(2)
        if (pair[0] !== 4 || pair[1] !== 2) throw 'fail'
      `,
    }),
    test(['entry.js', '--outfile=node.js', '--target=es6'], {
      'entry.js': `
        //! @license comment
        'use strict'
        function f(a) {
          a **= 2
          return [a, arguments[0]]
        }
        let pair = f(2)
        if (pair[0] !== 4 || pair[1] !== 2) throw 'fail'
      `,
    }),
  )

  // Test certain minification transformations
  for (const minify of [[], ['--minify-syntax']]) {
    tests.push(
      test(['in.js', '--outfile=node.js'].concat(minify), {
        'in.js': `let fn = (x) => { if (x && y) return; function y() {} throw 'fail' }; fn(fn)`,
      }),
      test(['in.js', '--outfile=node.js'].concat(minify), {
        'in.js': `let fn = (a, b) => { if (a && (x = () => y) && b) return; var x; let y = 123; if (x() !== 123) throw 'fail' }; fn(fn)`,
      }),
    )

    // Check property access simplification
    for (const access of [['.a'], ['["a"]']]) {
      tests.push(
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `if ({a: 1}${access} !== 1) throw 'fail'`,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `if ({a: {a: 1}}${access}${access} !== 1) throw 'fail'`,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `if ({a: {b: 1}}${access}.b !== 1) throw 'fail'`,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `if ({b: {a: 1}}.b${access} !== 1) throw 'fail'`,
        }),
        test(['in.js', '--outfile=node.js', '--log-level=error'].concat(minify), {
          'in.js': `if ({a: 1, a: 2}${access} !== 2) throw 'fail'`,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `if ({a: 1, [String.fromCharCode(97)]: 2}${access} !== 2) throw 'fail'`,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `let a = {a: 1}; if ({...a}${access} !== 1) throw 'fail'`,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `if ({ get a() { return 1 } }${access} !== 1) throw 'fail'`,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `if ({ __proto__: {a: 1} }${access} !== 1) throw 'fail'`,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `if ({ __proto__: null, a: 1 }${access} !== 1) throw 'fail'`,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `if ({ __proto__: null, b: 1 }${access} !== void 0) throw 'fail'`,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `if ({ __proto__: null }.__proto__ !== void 0) throw 'fail'`,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `if ({ ['__proto__']: null }.__proto__ !== null) throw 'fail'`,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `let x = 100; if ({ b: ++x, a: 1 }${access} !== 1 || x !== 101) throw 'fail'`,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `if ({ a: function() { return this.b }, b: 1 }${access}() !== 1) throw 'fail'`,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `if (({a: 2}${access} = 1) !== 1) throw 'fail'`,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `if ({a: 1}${access}++ !== 1) throw 'fail'`,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `if (++{a: 1}${access} !== 2) throw 'fail'`,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `
            Object.defineProperty(Object.prototype, 'MIN_OBJ_LIT', {value: 1})
            if ({}.MIN_OBJ_LIT !== 1) throw 'fail'
          `,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `
            let x = false
            function y() { x = true }
            if ({ b: y(), a: 1 }${access} !== 1 || !x) throw 'fail'
          `,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `
            try { new ({ a() {} }${access}); throw 'fail' }
            catch (e) { if (e === 'fail') throw e }
          `,
        }),
        test(['in.js', '--outfile=node.js'].concat(minify), {
          'in.js': `
            let x = 1;
            ({ set a(y) { x = y } }${access} = 2);
            if (x !== 2) throw 'fail'
          `,
        }),
      )
    }
  }

  // Test minification of top-level symbols
  tests.push(
    test(['in.js', '--outfile=node.js', '--minify'], {
      // Top-level names should not be minified
      'in.js': `function foo() {} if (foo.name !== 'foo') throw 'fail'`,
    }),
    test(['in.js', '--outfile=node.js', '--minify'], {
      // Nested names should be minified
      'in.js': `(() => { function foo() {} if (foo.name === 'foo') throw 'fail' })()`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--target=es6'], {
      // Importing the "__pow()" runtime function should not affect top-level name minification
      'in.js': `let _8 = 2 ** 3; function foo8() {} if (foo8.name !== 'foo' + _8) throw 'fail'`,
    }),
  )

  // Test name preservation
  for (let flags of [[], ['--minify', '--keep-names']]) {
    tests.push(
      // Arrow functions
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let fn = () => {}; if (fn.name !== 'fn') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let fn; fn = () => {}; if (fn.name !== 'fn') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let [fn = () => {}] = []; if (fn.name !== 'fn') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let fn; [fn = () => {}] = []; if (fn.name !== 'fn') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let {fn = () => {}} = {}; if (fn.name !== 'fn') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let {prop: fn = () => {}} = {}; if (fn.name !== 'fn') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let fn; ({fn = () => {}} = {}); if (fn.name !== 'fn') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let fn; ({prop: fn = () => {}} = {}); if (fn.name !== 'fn') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let obj = {}; obj.fn = () => {}; if (obj.fn.name !== '') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let obj = {}; obj['fn'] = () => {}; if (obj.fn.name !== '') throw 'fail' })()`,
      }),

      // Functions
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { function foo() {} if (foo.name !== 'foo') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let fn = function foo() {}; if (fn.name !== 'foo') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let fn = function() {}; if (fn.name !== 'fn') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let fn; fn = function() {}; if (fn.name !== 'fn') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let [fn = function() {}] = []; if (fn.name !== 'fn') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let fn; [fn = function() {}] = []; if (fn.name !== 'fn') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let {fn = function() {}} = {}; if (fn.name !== 'fn') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let {prop: fn = function() {}} = {}; if (fn.name !== 'fn') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let fn; ({fn = function() {}} = {}); if (fn.name !== 'fn') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let fn; ({prop: fn = function() {}} = {}); if (fn.name !== 'fn') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let obj = {}; obj.fn = function() {}; if (obj.fn.name !== '') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let obj = {}; obj['fn'] = function() {}; if (obj.fn.name !== '') throw 'fail' })()`,
      }),

      // Classes
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { class foo {} if (foo.name !== 'foo') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let cls = class foo {}; if (cls.name !== 'foo') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let cls = class {}; if (cls.name !== 'cls') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let cls; cls = class {}; if (cls.name !== 'cls') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let [cls = class {}] = []; if (cls.name !== 'cls') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let cls; [cls = class {}] = []; if (cls.name !== 'cls') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let {cls = class {}} = {}; if (cls.name !== 'cls') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let {prop: cls = class {}} = {}; if (cls.name !== 'cls') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let cls; ({cls = class {}} = {}); if (cls.name !== 'cls') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let cls; ({prop: cls = class {}} = {}); if (cls.name !== 'cls') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let obj = {}; obj.cls = class {}; if (obj.cls.name !== '') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let obj = {}; obj['cls'] = class {}; if (obj.cls.name !== '') throw 'fail' })()`,
      }),

      // Methods
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let obj = { foo() {} }; if (obj.foo.name !== 'foo') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let obj = { foo: () => {} }; if (obj.foo.name !== 'foo') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { class Foo { foo() {} }; if (new Foo().foo.name !== 'foo') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { class Foo { static foo() {} }; if (Foo.foo.name !== 'foo') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let Foo = class { foo() {} }; if (new Foo().foo.name !== 'foo') throw 'fail' })()`,
      }),
      test(['in.js', '--outfile=node.js', '--bundle'].concat(flags), {
        'in.js': `(() => { let Foo = class { static foo() {} }; if (Foo.foo.name !== 'foo') throw 'fail' })()`,
      }),
    )
  }
  tests.push(
    // Arrow functions
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle'], {
      'in.js': `import foo from './other'; if (foo.name !== 'default') throw 'fail'`,
      'other.js': `export default () => {}`,
    }),

    // Functions
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle'], {
      'in.js': `import foo from './other'; if (foo.name !== 'foo') throw 'fail'`,
      'other.js': `export default function foo() {}`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle'], {
      'in.js': `import foo from './other'; if (foo.name !== 'default') throw 'fail'`,
      'other.js': `export default function() {}`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle'], {
      'in.js': `import foo from './other'; if (foo.name !== 'default') throw 'fail'`,
      'other.js': `export default (function() {})`,
    }),

    // Classes
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle'], {
      'in.js': `import foo from './other'; if (foo.name !== 'foo') throw 'fail'`,
      'other.js': `export default class foo {}`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle'], {
      'in.js': `import foo from './other'; if (foo.name !== 'default') throw 'fail'`,
      'other.js': `export default class {}`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle'], {
      'in.js': `import foo from './other'; if (foo.name !== 'default') throw 'fail'`,
      'other.js': `export default (class {})`,
    }),
    test(['in.js', '--outfile=out.js', '--minify', '--keep-names', '--format=esm'], {
      'node.js': `import foo from './out.js'; if (foo.name !== 'foo') throw 'fail'`,
      'in.js': `export default class foo {}`,
    }),
    test(['in.js', '--outfile=out.js', '--minify', '--keep-names', '--format=esm'], {
      'node.js': `import foo from './out.js'; if (foo.name !== 'default') throw 'fail'`,
      'in.js': `export default class {}`,
    }),
    test(['in.js', '--outfile=out.js', '--minify', '--keep-names', '--format=esm'], {
      'node.js': `import foo from './out.js'; if (foo.name !== 'default') throw 'fail'`,
      'in.js': `export default (class {})`,
    }),

    // Class fields
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle', '--target=es6'], {
      'in.js': `(() => { class Foo { foo = () => {} } if (new Foo().foo.name !== 'foo') throw 'fail' })()`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle', '--target=es6'], {
      'in.js': `(() => { class Foo { static foo = () => {} } if (Foo.foo.name !== 'foo') throw 'fail' })()`,
    }),

    // Private methods
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle', '--target=es6'], {
      'in.js': `(() => { class foo { a() { return this.#b } #b() {} } if (foo.name !== 'foo') throw 'fail' })()`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle', '--target=es6'], {
      'in.js': `(() => { let cls = class foo { a() { return this.#b } #b() {} }; if (cls.name !== 'foo') throw 'fail' })()`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle', '--target=es6'], {
      'in.js': `(() => { let cls = class { a() { return this.#b } #b() {} }; if (cls.name !== 'cls') throw 'fail' })()`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle', '--target=es6'], {
      'in.js': `(() => { let cls; cls = class { a() { return this.#b } #b() {} }; if (cls.name !== 'cls') throw 'fail' })()`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle', '--target=es6'], {
      'in.js': `(() => { let [cls = class { a() { return this.#b } #b() {} }] = []; if (cls.name !== 'cls') throw 'fail' })()`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle', '--target=es6'], {
      'in.js': `(() => { let cls; [cls = class { a() { return this.#b } #b() {} }] = []; if (cls.name !== 'cls') throw 'fail' })()`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle', '--target=es6'], {
      'in.js': `(() => { let {cls = class { a() { return this.#b } #b() {} }} = {}; if (cls.name !== 'cls') throw 'fail' })()`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle', '--target=es6'], {
      'in.js': `(() => { let {prop: cls = class { a() { return this.#b } #b() {} }} = {}; if (cls.name !== 'cls') throw 'fail' })()`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle', '--target=es6'], {
      'in.js': `(() => { let cls; ({cls = class { a() { return this.#b } #b() {} }} = {}); if (cls.name !== 'cls') throw 'fail' })()`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle', '--target=es6'], {
      'in.js': `(() => { let cls; ({prop: cls = class { a() { return this.#b } #b() {} }} = {}); if (cls.name !== 'cls') throw 'fail' })()`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle', '--target=es6'], {
      'in.js': `import foo from './other'; if (foo.name !== 'foo') throw 'fail'`,
      'other.js': `export default class foo { a() { return this.#b } #b() {} }`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle', '--target=es6'], {
      'in.js': `import foo from './other'; if (foo.name !== 'default') throw 'fail'`,
      'other.js': `export default class { a() { return this.#b } #b() {} }`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--bundle', '--target=es6'], {
      'in.js': `import foo from './other'; if (foo.name !== 'default') throw 'fail'`,
      'other.js': `export default (class { a() { return this.#b } #b() {} })`,
    }),
    test(['in.js', '--outfile=out.js', '--minify', '--keep-names', '--format=esm', '--target=es6'], {
      'node.js': `import foo from './out.js'; if (foo.name !== 'foo') throw 'fail'`,
      'in.js': `export default class foo { a() { return this.#b } #b() {} }`,
    }),
    test(['in.js', '--outfile=out.js', '--minify', '--keep-names', '--format=esm', '--target=es6'], {
      'node.js': `import foo from './out.js'; if (foo.name !== 'default') throw 'fail'`,
      'in.js': `export default class { a() { return this.#b } #b() {} }`,
    }),
    test(['in.js', '--outfile=out.js', '--minify', '--keep-names', '--format=esm', '--target=es6'], {
      'node.js': `import foo from './out.js'; if (foo.name !== 'default') throw 'fail'`,
      'in.js': `export default (class { a() { return this.#b } #b() {} })`,
    }),

    // Private fields
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--format=esm', '--target=es6'], {
      'in.js': `class Foo { foo = this.#foo; #foo() {} } if (new Foo().foo.name !== '#foo') throw 'fail'`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--format=esm', '--target=es6'], {
      'in.js': `class Foo { static foo = this.#foo; static #foo() {} } if (Foo.foo.name !== '#foo') throw 'fail'`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--format=esm', '--target=es6'], {
      'in.js': `class Foo { #foo = function() {}; foo = this.#foo } if (new Foo().foo.name !== '#foo') throw 'fail'`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--format=esm', '--target=es6'], {
      'in.js': `class Foo { static #foo = function() {}; static foo = this.#foo } if (Foo.foo.name !== '#foo') throw 'fail'`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--format=esm', '--target=es6'], {
      'in.js': `class Foo { #foo = () => {}; foo = this.#foo } if (new Foo().foo.name !== '#foo') throw 'fail'`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--format=esm', '--target=es6'], {
      'in.js': `class Foo { static #foo = () => {}; static foo = this.#foo } if (Foo.foo.name !== '#foo') throw 'fail'`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--format=esm', '--target=es6'], {
      'in.js': `class Foo { #foo = class {}; foo = this.#foo } if (new Foo().foo.name !== '#foo') throw 'fail'`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--format=esm', '--target=es6'], {
      'in.js': `class Foo { static #foo = class {}; static foo = this.#foo } if (Foo.foo.name !== '#foo') throw 'fail'`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--format=esm', '--target=es6'], {
      'in.js': `class Foo { #foo = class { #bar = 123; bar = this.#bar }; foo = this.#foo } if (new Foo().foo.name !== '#foo') throw 'fail'`,
    }),
    test(['in.js', '--outfile=node.js', '--minify', '--keep-names', '--format=esm', '--target=es6'], {
      'in.js': `class Foo { static #foo = class { #bar = 123; bar = this.#bar }; static foo = this.#foo } if (Foo.foo.name !== '#foo') throw 'fail'`,
    }),
  )

  // Test minification of hoisted top-level symbols declared in nested scopes.
  // Previously this code was incorrectly transformed into this, which crashes:
  //
  //   var c = false;
  //   var d = function a() {
  //     b[a]();
  //   };
  //   for (var a = 0, b = [() => c = true]; a < b.length; a++) {
  //     d();
  //   }
  //   export default c;
  //
  // The problem is that "var i" is declared in a nested scope but hoisted to
  // the top-level scope. So it's accidentally assigned a nested scope slot
  // even though it's a top-level symbol, not a nested scope symbol.
  tests.push(
    test(['in.js', '--outfile=out.js', '--format=esm', '--minify', '--bundle'], {
      'in.js': `
        var worked = false
        var loop = function fn() {
          array[i]();
        };
        for (var i = 0, array = [() => worked = true]; i < array.length; i++) {
          loop();
        }
        export default worked
      `,
      'node.js': `
        import worked from './out.js'
        if (!worked) throw 'fail'
      `,
    }),
  )

  // Test hoisting variables inside for loop initializers outside of lazy ESM
  // wrappers. Previously this didn't work due to a bug that considered for
  // loop initializers to already be in the top-level scope. For more info
  // see: https://github.com/evanw/esbuild/issues/1455.
  tests.push(
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `
        if (require('./nested').foo() !== 10) throw 'fail'
      `,
      'nested.js': `
        for (var i = 0; i < 10; i++) ;
        export function foo() { return i }
      `,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `
        if (require('./nested').foo() !== 'c') throw 'fail'
      `,
      'nested.js': `
        for (var i in {a: 1, b: 2, c: 3}) ;
        export function foo() { return i }
      `,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `
        if (require('./nested').foo() !== 3) throw 'fail'
      `,
      'nested.js': `
        for (var i of [1, 2, 3]) ;
        export function foo() { return i }
      `,
    }),
    test(['in.js', '--outfile=node.js', '--bundle', '--target=es6'], {
      'in.js': `
        if (JSON.stringify(require('./nested').foo()) !== '{"b":2,"c":3}') throw 'fail'
      `,
      'nested.js': `
        for (var {a, ...i} = {a: 1, b: 2, c: 3}; 0; ) ;
        export function foo() { return i }
      `,
    }),
    test(['in.js', '--outfile=node.js', '--bundle', '--target=es6'], {
      'in.js': `
        if (JSON.stringify(require('./nested').foo()) !== '{"0":"c"}') throw 'fail'
      `,
      'nested.js': `
        for (var {a, ...i} in {a: 1, b: 2, c: 3}) ;
        export function foo() { return i }
      `,
    }),
    test(['in.js', '--outfile=node.js', '--bundle', '--target=es6'], {
      'in.js': `
        if (JSON.stringify(require('./nested').foo()) !== '{"b":2,"c":3}') throw 'fail'
      `,
      'nested.js': `
        for (var {a, ...i} of [{a: 1, b: 2, c: 3}]) ;
        export function foo() { return i }
      `,
    }),
  )

  // Test tree shaking
  tests.push(
    // Keep because used (ES6)
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import * as foo from './foo'; if (global.dce0 !== 123 || foo.abc !== 'abc') throw 'fail'`,
      'foo/index.js': `global.dce0 = 123; export const abc = 'abc'`,
      'foo/package.json': `{ "sideEffects": false }`,
    }),

    // Remove because unused (ES6)
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import * as foo from './foo'; if (global.dce1 !== void 0) throw 'fail'`,
      'foo/index.js': `global.dce1 = 123; export const abc = 'abc'`,
      'foo/package.json': `{ "sideEffects": false }`,
    }),

    // Keep because side effects (ES6)
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import * as foo from './foo'; if (global.dce2 !== 123) throw 'fail'`,
      'foo/index.js': `global.dce2 = 123; export const abc = 'abc'`,
      'foo/package.json': `{ "sideEffects": true }`,
    }),

    // Keep because used (CommonJS)
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import foo from './foo'; if (global.dce3 !== 123 || foo.abc !== 'abc') throw 'fail'`,
      'foo/index.js': `global.dce3 = 123; exports.abc = 'abc'`,
      'foo/package.json': `{ "sideEffects": false }`,
    }),

    // Remove because unused (CommonJS)
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import foo from './foo'; if (global.dce4 !== void 0) throw 'fail'`,
      'foo/index.js': `global.dce4 = 123; exports.abc = 'abc'`,
      'foo/package.json': `{ "sideEffects": false }`,
    }),

    // Keep because side effects (CommonJS)
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import foo from './foo'; if (global.dce5 !== 123) throw 'fail'`,
      'foo/index.js': `global.dce5 = 123; exports.abc = 'abc'`,
      'foo/package.json': `{ "sideEffects": true }`,
    }),

    // Note: Tree shaking this could technically be considered incorrect because
    // the import is for a property whose getter in this case has a side effect.
    // However, this is very unlikely and the vast majority of the time people
    // would likely rather have the code be tree-shaken. This test case enforces
    // the technically incorrect behavior as documentation that this edge case
    // is being ignored.
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import {foo, bar} from './foo'; let unused = foo; if (bar) throw 'expected "foo" to be tree-shaken'`,
      'foo.js': `module.exports = {get foo() { module.exports.bar = 1 }, bar: 0}`,
    }),

    // Test for an implicit and explicit "**/" prefix (see https://github.com/evanw/esbuild/issues/1184)
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import './foo'; if (global.dce6 !== 123) throw 'fail'`,
      'foo/dir/x.js': `global.dce6 = 123`,
      'foo/package.json': `{ "main": "dir/x", "sideEffects": ["x.*"] }`,
    }),
    test(['--bundle', 'entry.js', '--outfile=node.js'], {
      'entry.js': `import './foo'; if (global.dce6 !== 123) throw 'fail'`,
      'foo/dir/x.js': `global.dce6 = 123`,
      'foo/package.json': `{ "main": "dir/x", "sideEffects": ["**/x.*"] }`,
    }),
  )

  // Test obscure CommonJS symbol edge cases
  tests.push(
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `const ns = require('./foo'); if (ns.foo !== 123 || ns.bar !== 123) throw 'fail'`,
      'foo.js': `var exports, module; module.exports.foo = 123; exports.bar = exports.foo`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `require('./foo'); require('./bar')`,
      'foo.js': `let exports; if (exports !== void 0) throw 'fail'`,
      'bar.js': `let module; if (module !== void 0) throw 'fail'`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `const ns = require('./foo'); if (ns.foo !== void 0 || ns.default.foo !== 123) throw 'fail'`,
      'foo.js': `var exports = {foo: 123}; export default exports`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `const ns = require('./foo'); if (ns !== 123) throw 'fail'`,
      'foo.ts': `let module = 123; export = module`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `require('./foo')`,
      'foo.js': `var require; if (require !== void 0) throw 'fail'`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `require('./foo')`,
      'foo.js': `var require = x => x; if (require('does not exist') !== 'does not exist') throw 'fail'`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `const ns = require('./foo'); if (ns.a !== 123 || ns.b.a !== 123) throw 'fail'`,
      'foo.js': `exports.a = 123; exports.b = this`,
    }),
    test(['in.js', '--outfile=node.js', '--bundle', '--log-level=error'], {
      'in.js': `const ns = require('./foo'); if (ns.a !== 123 || ns.b !== void 0) throw 'fail'`,
      'foo.js': `export let a = 123, b = this`,
    }),
  )

  // Optional chain lowering tests
  for (let [code, expected] of [
    ['array?.map?.(x => -x).filter', '[].filter'],
    ['array?.map?.(x => -x)["filter"]', '[].filter'],
    ['array?.map?.(x => -x).filter(x => x < -1)', '[-2, -3]'],
    ['array?.map?.(x => -x)["filter"](x => x < -1)', '[-2, -3]'],
  ]) {
    tests.push(
      test(['in.js', '--outfile=node.js', '--target=es6', '--format=esm'], {
        'in.js': `
          import * as assert from 'assert';
          let array = [1, 2, 3];
          let result = ${code};
          assert.deepStrictEqual(result, ${expected});
        `,
      }),
      test(['in.js', '--outfile=node.js', '--target=es6', '--format=esm'], {
        'in.js': `
          import * as assert from 'assert';
          function test(array, result = ${code}) {
            return result
          }
          assert.deepStrictEqual(test([1, 2, 3]), ${expected});
        `,
      }),
    )
  }

  // Class lowering tests
  tests.push(
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Foo {
          foo = 123
          self = this
          #method() {
            if (this.foo !== 123) throw 'fail'
          }
          bar() {
            let that = () => this
            that().#method()
            that().#method?.()
            that()?.#method()
            that()?.#method?.()
            that().self.#method()
            that().self.#method?.()
            that().self?.#method()
            that().self?.#method?.()
            that()?.self.#method()
            that()?.self.#method?.()
            that()?.self?.#method()
            that()?.self?.#method?.()
          }
        }
        new Foo().bar()
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Foo {
          foo = 123
          get #bar() { return this.foo }
          set #bar(x) { this.foo = x }
          bar() {
            let that = () => this
            that().#bar **= 2
            if (this.foo !== 15129) throw 'fail'
          }
        }
        new Foo().bar()
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        let bar
        class Foo {
          get #foo() { bar = new Foo; return this.result }
          set #foo(x) { this.result = x }
          bar() {
            bar = this
            bar.result = 2
            ++bar.#foo
          }
        }
        let foo = new Foo()
        foo.bar()
        if (foo === bar || foo.result !== 3 || bar.result !== void 0) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        let bar
        class Foo {
          get #foo() { bar = new Foo; return this.result }
          set #foo(x) { this.result = x }
          bar() {
            bar = this
            bar.result = 2
            bar.#foo *= 3
          }
        }
        let foo = new Foo()
        foo.bar()
        if (foo === bar || foo.result !== 6 || bar.result !== void 0) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        let bar
        class Foo {
          get #foo() { bar = new Foo; return this.result }
          set #foo(x) { this.result = x }
          bar() {
            bar = this
            bar.result = 2
            bar.#foo **= 3
          }
        }
        let foo = new Foo()
        foo.bar()
        if (foo === bar || foo.result !== 8 || bar.result !== void 0) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        function print(x) {
          return typeof x + ':' + x
        }

        function check(before, op, after) {
          let result = new Foo(before)[op]()
          if (result !== after) throw before + ' ' + op + ' should be ' + after + ' but was ' + result
        }

        class Foo {
          #foo
          constructor(foo) { this.#foo = foo }
          preInc = () => print(++this.#foo) + ' ' + print(this.#foo)
          preDec = () => print(--this.#foo) + ' ' + print(this.#foo)
          postInc = () => print(this.#foo++) + ' ' + print(this.#foo)
          postDec = () => print(this.#foo--) + ' ' + print(this.#foo)
        }

        check(123, 'preInc', 'number:124 number:124')
        check(123, 'preDec', 'number:122 number:122')
        check(123, 'postInc', 'number:123 number:124')
        check(123, 'postDec', 'number:123 number:122')

        check('123', 'preInc', 'number:124 number:124')
        check('123', 'preDec', 'number:122 number:122')
        check('123', 'postInc', 'number:123 number:124')
        check('123', 'postDec', 'number:123 number:122')

        check('x', 'preInc', 'number:NaN number:NaN')
        check('x', 'preDec', 'number:NaN number:NaN')
        check('x', 'postInc', 'number:NaN number:NaN')
        check('x', 'postDec', 'number:NaN number:NaN')

        check(BigInt(123), 'preInc', 'bigint:124 bigint:124')
        check(BigInt(123), 'preDec', 'bigint:122 bigint:122')
        check(BigInt(123), 'postInc', 'bigint:123 bigint:124')
        check(BigInt(123), 'postDec', 'bigint:123 bigint:122')
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        function print(x) {
          return typeof x + ':' + x
        }

        function check(before, op, after) {
          let result = new Foo(before)[op]()
          if (result !== after) throw before + ' ' + op + ' should be ' + after + ' but was ' + result
        }

        class Foo {
          get #foo() { return this.__foo }
          set #foo(x) { this.__foo = x }
          constructor(foo) { this.#foo = foo }
          preInc = () => print(++this.#foo) + ' ' + print(this.#foo)
          preDec = () => print(--this.#foo) + ' ' + print(this.#foo)
          postInc = () => print(this.#foo++) + ' ' + print(this.#foo)
          postDec = () => print(this.#foo--) + ' ' + print(this.#foo)
        }

        check(123, 'preInc', 'number:124 number:124')
        check(123, 'preDec', 'number:122 number:122')
        check(123, 'postInc', 'number:123 number:124')
        check(123, 'postDec', 'number:123 number:122')

        check('123', 'preInc', 'number:124 number:124')
        check('123', 'preDec', 'number:122 number:122')
        check('123', 'postInc', 'number:123 number:124')
        check('123', 'postDec', 'number:123 number:122')

        check('x', 'preInc', 'number:NaN number:NaN')
        check('x', 'preDec', 'number:NaN number:NaN')
        check('x', 'postInc', 'number:NaN number:NaN')
        check('x', 'postDec', 'number:NaN number:NaN')

        check(BigInt(123), 'preInc', 'bigint:124 bigint:124')
        check(BigInt(123), 'preDec', 'bigint:122 bigint:122')
        check(BigInt(123), 'postInc', 'bigint:123 bigint:124')
        check(BigInt(123), 'postDec', 'bigint:123 bigint:122')
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        function print(x) {
          return typeof x + ':' + x
        }

        function check(before, op, after) {
          Foo.setup(before)
          let result = Foo[op]()
          if (result !== after) throw before + ' ' + op + ' should be ' + after + ' but was ' + result
        }

        class Foo {
          static #foo
          static setup(x) { Foo.#foo = x }
          static preInc = () => print(++Foo.#foo) + ' ' + print(Foo.#foo)
          static preDec = () => print(--Foo.#foo) + ' ' + print(Foo.#foo)
          static postInc = () => print(Foo.#foo++) + ' ' + print(Foo.#foo)
          static postDec = () => print(Foo.#foo--) + ' ' + print(Foo.#foo)
        }

        check(123, 'preInc', 'number:124 number:124')
        check(123, 'preDec', 'number:122 number:122')
        check(123, 'postInc', 'number:123 number:124')
        check(123, 'postDec', 'number:123 number:122')

        check('123', 'preInc', 'number:124 number:124')
        check('123', 'preDec', 'number:122 number:122')
        check('123', 'postInc', 'number:123 number:124')
        check('123', 'postDec', 'number:123 number:122')

        check('x', 'preInc', 'number:NaN number:NaN')
        check('x', 'preDec', 'number:NaN number:NaN')
        check('x', 'postInc', 'number:NaN number:NaN')
        check('x', 'postDec', 'number:NaN number:NaN')

        check(BigInt(123), 'preInc', 'bigint:124 bigint:124')
        check(BigInt(123), 'preDec', 'bigint:122 bigint:122')
        check(BigInt(123), 'postInc', 'bigint:123 bigint:124')
        check(BigInt(123), 'postDec', 'bigint:123 bigint:122')
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        function print(x) {
          return typeof x + ':' + x
        }

        function check(before, op, after) {
          Foo.setup(before)
          let result = Foo[op]()
          if (result !== after) throw before + ' ' + op + ' should be ' + after + ' but was ' + result
        }

        class Foo {
          static get #foo() { return this.__foo }
          static set #foo(x) { this.__foo = x }
          static setup(x) { this.#foo = x }
          static preInc = () => print(++this.#foo) + ' ' + print(this.#foo)
          static preDec = () => print(--this.#foo) + ' ' + print(this.#foo)
          static postInc = () => print(this.#foo++) + ' ' + print(this.#foo)
          static postDec = () => print(this.#foo--) + ' ' + print(this.#foo)
        }

        check(123, 'preInc', 'number:124 number:124')
        check(123, 'preDec', 'number:122 number:122')
        check(123, 'postInc', 'number:123 number:124')
        check(123, 'postDec', 'number:123 number:122')

        check('123', 'preInc', 'number:124 number:124')
        check('123', 'preDec', 'number:122 number:122')
        check('123', 'postInc', 'number:123 number:124')
        check('123', 'postDec', 'number:123 number:122')

        check('x', 'preInc', 'number:NaN number:NaN')
        check('x', 'preDec', 'number:NaN number:NaN')
        check('x', 'postInc', 'number:NaN number:NaN')
        check('x', 'postDec', 'number:NaN number:NaN')

        check(BigInt(123), 'preInc', 'bigint:124 bigint:124')
        check(BigInt(123), 'preDec', 'bigint:122 bigint:122')
        check(BigInt(123), 'postInc', 'bigint:123 bigint:124')
        check(BigInt(123), 'postDec', 'bigint:123 bigint:122')
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        function expect(fn, msg) {
          try {
            fn()
          } catch (e) {
            if (e instanceof TypeError && e.message === msg) return
          }
          throw 'expected ' + msg
        }
        class Foo {
          #foo
          #method() {}
          get #getter() {}
          set #setter(x) {}
          bar() {
            let obj = {}
            expect(() => obj.#foo, 'Cannot read from private field')
            expect(() => obj.#foo = 1, 'Cannot write to private field')
            expect(() => obj.#getter, 'Cannot read from private field')
            expect(() => obj.#setter = 1, 'Cannot write to private field')
            expect(() => obj.#method, 'Cannot access private method')
            expect(() => obj.#method = 1, 'Cannot write to private field')
            expect(() => this.#setter, 'member.get is not a function')
            expect(() => this.#getter = 1, 'member.set is not a function')
            expect(() => this.#method = 1, 'member.set is not a function')
          }
        }
        new Foo().bar()
      `,
    }, {
      expectedStderr: ` > in.js:22:29: warning: Writing to read-only method "#method" will throw
    22 │             expect(() => obj.#method = 1, 'Cannot write to private f...
       ╵                              ~~~~~~~

 > in.js:23:30: warning: Reading from setter-only property "#setter" will throw
    23 │             expect(() => this.#setter, 'member.get is not a function')
       ╵                               ~~~~~~~

 > in.js:24:30: warning: Writing to getter-only property "#getter" will throw
    24 │             expect(() => this.#getter = 1, 'member.set is not a func...
       ╵                               ~~~~~~~

 > in.js:25:30: warning: Writing to read-only method "#method" will throw
    25 │             expect(() => this.#method = 1, 'member.set is not a func...
       ╵                               ~~~~~~~

`,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        let setterCalls = 0
        class Foo {
          key
          set key(x) { setterCalls++ }
        }
        let foo = new Foo()
        if (setterCalls !== 0 || !foo.hasOwnProperty('key') || foo.key !== void 0) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        let setterCalls = 0
        class Foo {
          key = 123
          set key(x) { setterCalls++ }
        }
        let foo = new Foo()
        if (setterCalls !== 0 || !foo.hasOwnProperty('key') || foo.key !== 123) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        let toStringCalls = 0
        let setterCalls = 0
        class Foo {
          [{toString() {
            toStringCalls++
            return 'key'
          }}]
          set key(x) { setterCalls++ }
        }
        let foo = new Foo()
        if (setterCalls !== 0 || toStringCalls !== 1 || !foo.hasOwnProperty('key') || foo.key !== void 0) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        let toStringCalls = 0
        let setterCalls = 0
        class Foo {
          [{toString() {
            toStringCalls++
            return 'key'
          }}] = 123
          set key(x) { setterCalls++ }
        }
        let foo = new Foo()
        if (setterCalls !== 0 || toStringCalls !== 1 || !foo.hasOwnProperty('key') || foo.key !== 123) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        let key = Symbol('key')
        let setterCalls = 0
        class Foo {
          [key]
          set [key](x) { setterCalls++ }
        }
        let foo = new Foo()
        if (setterCalls !== 0 || !foo.hasOwnProperty(key) || foo[key] !== void 0) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        let key = Symbol('key')
        let setterCalls = 0
        class Foo {
          [key] = 123
          set [key](x) { setterCalls++ }
        }
        let foo = new Foo()
        if (setterCalls !== 0 || !foo.hasOwnProperty(key) || foo[key] !== 123) throw 'fail'
      `,
    }),

    // Test class re-assignment
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Foo {
          foo = () => this
        }
        let foo = new Foo()
        if (foo.foo() !== foo) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Foo {
          static foo = () => this
        }
        let old = Foo
        let foo = Foo.foo
        Foo = class Bar {}
        if (foo() !== old) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Foo {
          bar = 'works'
          foo = () => class {
            [this.bar]
          }
        }
        let foo = new Foo().foo
        if (!('works' in new (foo()))) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Foo {
          static bar = 'works'
          static foo = () => class {
            [this.bar]
          }
        }
        let foo = Foo.foo
        Foo = class Bar {}
        if (!('works' in new (foo()))) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Foo {
          static foo() { return this.#foo }
          static #foo = Foo
        }
        let old = Foo
        Foo = class Bar {}
        if (old.foo() !== old) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Foo {
          static foo() { return this.#foo() }
          static #foo() { return Foo }
        }
        let old = Foo
        Foo = class Bar {}
        if (old.foo() !== old) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        try {
          class Foo {
            static foo() { return this.#foo }
            static #foo = Foo = class Bar {}
          }
          throw 'fail'
        } catch (e) {
          if (!(e instanceof TypeError))
            throw e
        }
      `,
    }, {
      expectedStderr: ` > in.js:5:26: warning: This assignment will throw because "Foo" is a constant
    5 │             static #foo = Foo = class Bar {}
      ╵                           ~~~
   in.js:3:16: note: "Foo" was declared a constant here
    3 │           class Foo {
      ╵                 ~~~

`,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Foo {
          static foo() { return this.#foo() }
          static #foo() { Foo = class Bar{} }
        }
        try {
          Foo.foo()
          throw 'fail'
        } catch (e) {
          if (!(e instanceof TypeError))
            throw e
        }
      `,
    }, {
      expectedStderr: ` > in.js:4:26: warning: This assignment will throw because "Foo" is a constant
    4 │           static #foo() { Foo = class Bar{} }
      ╵                           ~~~
   in.js:2:14: note: "Foo" was declared a constant here
    2 │         class Foo {
      ╵               ~~~

`,
    }),

    // Issue: https://github.com/evanw/esbuild/issues/901
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class A {
          pub = this.#priv;
          #priv() {
            return 'Inside #priv';
          }
        }
        if (new A().pub() !== 'Inside #priv') throw 'fail';
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class A {
          static pub = this.#priv;
          static #priv() {
            return 'Inside #priv';
          }
        }
        if (A.pub() !== 'Inside #priv') throw 'fail';
      `,
    }),

    // Issue: https://github.com/evanw/esbuild/issues/1066
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Test {
          #x = 2;
          #y = [];
          z = 2;

          get x() { return this.#x; }
          get y() { return this.#y; }

          world() {
            return [1,[2,3],4];
          }

          hello() {
            [this.#x,this.#y,this.z] = this.world();
          }
        }

        var t = new Test();
        t.hello();
        if (t.x !== 1 || t.y[0] !== 2 || t.y[1] !== 3 || t.z !== 4) throw 'fail';
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Foo {
          #a
          #b
          #c
          foo() {
            [this.#a, this.#b, this.#c] = {
              [Symbol.iterator]() {
                let value = 0
                return {
                  next() {
                    return { value: ++value, done: false }
                  }
                }
              }
            }
            return [this.#a, this.#b, this.#c].join(' ')
          }
        }
        if (new Foo().foo() !== '1 2 3') throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Foo {
          #a
          #b
          #c
          #d
          #e
          #f
          foo() {
            [
              {x: this.#a},
              [[, this.#b, ,]],
              {y: this.#c = 3},
              {x: this.x, y: this.y, ...this.#d},
              [, , ...this.#e],
              [{x: [{y: [this.#f]}]}],
            ] = [
              {x: 1},
              [[1, 2, 3]],
              {},
              {x: 2, y: 3, z: 4, w: 5},
              [4, 5, 6, 7, 8],
              [{x: [{y: [9]}]}],
            ]
            return JSON.stringify([
              this.#a,
              this.#b,
              this.#c,
              this.#d,
              this.#e,
              this.#f,
            ])
          }
        }
        if (new Foo().foo() !== '[1,2,3,{"z":4,"w":5},[6,7,8],9]') throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Foo {
          values = []
          set #a(a) { this.values.push(a) }
          set #b(b) { this.values.push(b) }
          set #c(c) { this.values.push(c) }
          set #d(d) { this.values.push(d) }
          set #e(e) { this.values.push(e) }
          set #f(f) { this.values.push(f) }
          foo() {
            [
              {x: this.#a},
              [[, this.#b, ,]],
              {y: this.#c = 3},
              {x: this.x, y: this.y, ...this.#d},
              [, , ...this.#e],
              [{x: [{y: [this.#f]}]}],
            ] = [
              {x: 1},
              [[1, 2, 3]],
              {},
              {x: 2, y: 3, z: 4, w: 5},
              [4, 5, 6, 7, 8],
              [{x: [{y: [9]}]}],
            ]
            return JSON.stringify(this.values)
          }
        }
        if (new Foo().foo() !== '[1,2,3,{"z":4,"w":5},[6,7,8],9]') throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Foo {
          #a
          #b
          #c
          #d
          #e
          #f
          foo() {
            for ([
              {x: this.#a},
              [[, this.#b, ,]],
              {y: this.#c = 3},
              {x: this.x, y: this.y, ...this.#d},
              [, , ...this.#e],
              [{x: [{y: [this.#f]}]}],
            ] of [[
              {x: 1},
              [[1, 2, 3]],
              {},
              {x: 2, y: 3, z: 4, w: 5},
              [4, 5, 6, 7, 8],
              [{x: [{y: [9]}]}],
            ]]) ;
            return JSON.stringify([
              this.#a,
              this.#b,
              this.#c,
              this.#d,
              this.#e,
              this.#f,
            ])
          }
        }
        if (new Foo().foo() !== '[1,2,3,{"z":4,"w":5},[6,7,8],9]') throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Foo {
          #a
          #b() {}
          get #c() {}
          set #d(x) {}
          bar(x) {
            return #a in x && #b in x && #c in x && #d in x
          }
        }
        let foo = new Foo()
        if (foo.bar(foo) !== true || foo.bar(Foo) !== false) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Foo {
          #a
          #b() {}
          get #c() {}
          set #d(x) {}
          bar(x) {
            return #a in x && #b in x && #c in x && #d in x
          }
        }
        function mustFail(x) {
          let foo = new Foo()
          try {
            foo.bar(x)
          } catch (e) {
            if (e instanceof TypeError) return
            throw e
          }
          throw 'fail'
        }
        mustFail(null)
        mustFail(void 0)
        mustFail(0)
        mustFail('')
        mustFail(Symbol('x'))
      `,
    }),

    test(['in.ts', '--outfile=node.js', '--target=es6'], {
      'in.ts': `
        let b = 0
        class Foo {
          a
          [(() => ++b)()]
          declare c
          declare [(() => ++b)()]
        }
        const foo = new Foo
        if (b !== 1 || 'a' in foo || 1 in foo || 'c' in foo || 2 in foo) throw 'fail'
      `,
    }),
    test(['in.ts', '--outfile=node.js', '--target=es6'], {
      'in.ts': `
        let b = 0
        class Foo {
          a
          [(() => ++b)()]
          declare c
          declare [(() => ++b)()]
        }
        const foo = new Foo
        if (b !== 1 || !('a' in foo) || !(1 in foo) || 'c' in foo || 2 in foo) throw 'fail'
      `,
      'tsconfig.json': `{
        "compilerOptions": {
          "useDefineForClassFields": true
        }
      }`
    }),

    // Validate "branding" behavior
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Base { constructor(x) { return x } }
        class Derived extends Base { #y = true; static is(z) { return z.#y } }
        const foo = {}
        try { Derived.is(foo); throw 'fail 1' } catch (e) { if (e === 'fail 1') throw e }
        new Derived(foo)
        if (Derived.is(foo) !== true) throw 'fail 2'
        try { new Derived(foo); throw 'fail 3' } catch (e) { if (e === 'fail 3') throw e }
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Base { constructor(x) { return x } }
        class Derived extends Base { #y = true; static is(z) { return z.#y } }
        const foo = 123
        try { Derived.is(foo); throw 'fail 1' } catch (e) { if (e === 'fail 1') throw e }
        new Derived(foo)
        try { Derived.is(foo); throw 'fail 2' } catch (e) { if (e === 'fail 2') throw e }
        new Derived(foo)
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Base { constructor(x) { return x } }
        class Derived extends Base { #y = true; static is(z) { return z.#y } }
        const foo = null
        try { Derived.is(foo); throw 'fail 1' } catch (e) { if (e === 'fail 1') throw e }
        new Derived(foo)
        try { Derived.is(foo); throw 'fail 2' } catch (e) { if (e === 'fail 2') throw e }
        new Derived(foo)
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Base { constructor(x) { return x } }
        class Derived extends Base { #y() { return true } static is(z) { return z.#y } }
        const foo = {}
        try { Derived.is(foo); throw 'fail 1' } catch (e) { if (e === 'fail 1') throw e }
        new Derived(foo)
        if (Derived.is(foo)() !== true) throw 'fail 2'
        try { new Derived(foo); throw 'fail 3' } catch (e) { if (e === 'fail 3') throw e }
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Base { constructor(x) { return x } }
        class Derived extends Base { #y() {} static is(z) { return z.#y } }
        const foo = 123
        try { Derived.is(foo); throw 'fail 1' } catch (e) { if (e === 'fail 1') throw e }
        new Derived(foo)
        try { Derived.is(foo); throw 'fail 2' } catch (e) { if (e === 'fail 2') throw e }
        new Derived(foo)
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        class Base { constructor(x) { return x } }
        class Derived extends Base { #y() {} static is(z) { return z.#y } }
        const foo = null
        try { Derived.is(foo); throw 'fail 1' } catch (e) { if (e === 'fail 1') throw e }
        new Derived(foo)
        try { Derived.is(foo); throw 'fail 2' } catch (e) { if (e === 'fail 2') throw e }
        new Derived(foo)
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        let a, b, c, x = 123
        class Foo {
          #a() { a = { this: this, args: arguments } }
          get #b() { return function () { b = { this: this, args: arguments } } }
          #c = function () { c = { this: this, args: arguments } }
          bar() { (this.#a)\`a\${x}aa\`; (this.#b)\`b\${x}bb\`; (this.#c)\`c\${x}cc\` }
        }
        new Foo().bar()
        if (!(a.this instanceof Foo) || !(b.this instanceof Foo) || !(c.this instanceof Foo)) throw 'fail'
        if (JSON.stringify([...a.args, ...b.args, ...c.args]) !== JSON.stringify([['a', 'aa'], 123, ['b', 'bb'], 123, ['c', 'cc'], 123])) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        let a, b, c, x = 123
        class Foo {
          #a() { a = { this: this, args: arguments } }
          get #b() { return function () { b = { this: this, args: arguments } } }
          #c = function () { c = { this: this, args: arguments } }
          bar() { (0, this.#a)\`a\${x}aa\`; (0, this.#b)\`b\${x}bb\`; (0, this.#c)\`c\${x}cc\` }
        }
        new Foo().bar()
        if (a.this instanceof Foo || b.this instanceof Foo || c.this instanceof Foo) throw 'fail'
        if (JSON.stringify([...a.args, ...b.args, ...c.args]) !== JSON.stringify([['a', 'aa'], 123, ['b', 'bb'], 123, ['c', 'cc'], 123])) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        let it
        class Foo {
          constructor() { it = this; it = it.#fn\`\` }
          get #fn() { it = null; return function() { return this } }
        }
        new Foo
        if (!(it instanceof Foo)) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        let it
        class Foo {
          constructor() { it = this; it = it.#fn() }
          get #fn() { it = null; return function() { return this } }
        }
        new Foo
        if (!(it instanceof Foo)) throw 'fail'
      `,
    }),
  )

  // Async lowering tests
  tests.push(
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        exports.async = async () => {
          const value = await Promise.resolve(123)
          if (value !== 123) throw 'fail'

          let uncaught = false
          let caught = false
          try {
            await Promise.reject(234)
            uncaught = true
          } catch (error) {
            if (error !== 234) throw 'fail'
            caught = true
          }
          if (uncaught || !caught) throw 'fail'
        }
      `,
    }, { async: true }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        async function throws() {
          throw 123
        }
        exports.async = () => throws().then(
          () => {
            throw 'fail'
          },
          error => {
            if (error !== 123) throw 'fail'
          }
        )
      `,
    }, { async: true }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        exports.async = async () => {
          "use strict"
          async function foo() {
            return [this, arguments]
          }
          let [t, a] = await foo.call(0, 1, 2, 3)
          if (t !== 0 || a.length !== 3 || a[0] !== 1 || a[1] !== 2 || a[2] !== 3) throw 'fail'
        }
      `,
    }, { async: true }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      'in.js': `
        let couldThrow = () => 'b'
        exports.async = async () => {
          "use strict"
          async function f0() {
            let bar = async (x, y) => [x, y, this, arguments]
            return await bar('a', 'b')
          }
          async function f1() {
            let bar = async (x, ...y) => [x, y[0], this, arguments]
            return await bar('a', 'b')
          }
          async function f2() {
            let bar = async (x, y = 'b') => [x, y, this, arguments]
            return await bar('a')
          }
          async function f3() {
            let bar = async (x, y = couldThrow()) => [x, y, this, arguments]
            return await bar('a')
          }
          async function f4() {
            let bar = async (x, y = couldThrow()) => (() => [x, y, this, arguments])()
            return await bar('a')
          }
          async function f5() {
            let bar = () => async (x, y = couldThrow()) => [x, y, this, arguments]
            return await bar()('a')
          }
          async function f6() {
            let bar = async () => async (x, y = couldThrow()) => [x, y, this, arguments]
            return await (await bar())('a')
          }
          for (let foo of [f0, f1, f2, f3, f4, f5, f6]) {
            let [x, y, t, a] = await foo.call(0, 1, 2, 3)
            if (x !== 'a' || y !== 'b' || t !== 0 || a.length !== 3 || a[0] !== 1 || a[1] !== 2 || a[2] !== 3) throw 'fail'
          }
        }
      `,
    }, { async: true }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      // The async transform must not change the argument count
      'in.js': `
        async function a(x, y) {}
        if (a.length !== 2) throw 'fail: a'

        async function b(x, y = x(), z) {}
        if (b.length !== 1) throw 'fail: b'

        async function c(x, y, ...z) {}
        if (c.length !== 2) throw 'fail: c'

        let d = async function(x, y) {}
        if (d.length !== 2) throw 'fail: d'

        let e = async function(x, y = x(), z) {}
        if (e.length !== 1) throw 'fail: e'

        let f = async function(x, y, ...z) {}
        if (f.length !== 2) throw 'fail: f'

        let g = async (x, y) => {}
        if (g.length !== 2) throw 'fail: g'

        let h = async (x, y = x(), z) => {}
        if (h.length !== 1) throw 'fail: h'

        let i = async (x, y, ...z) => {}
        if (i.length !== 2) throw 'fail: i'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      // Functions must be able to access default arguments past the last non-default argument
      'in.js': `
        exports.async = async () => {
          async function a(x, y = 0) { return y }
          let b = async function(x, y = 0) { return y }
          let c = async (x, y = 0) => y
          for (let fn of [a, b, c]) {
            if ((await fn('x', 'y')) !== 'y') throw 'fail: ' + fn
          }
        }
      `,
    }, { async: true }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      // Functions must be able to access arguments past the argument count using "arguments"
      'in.js': `
        exports.async = async () => {
          async function a() { return arguments[2] }
          async function b(x, y) { return arguments[2] }
          async function c(x, y = x) { return arguments[2] }
          let d = async function() { return arguments[2] }
          let e = async function(x, y) { return arguments[2] }
          let f = async function(x, y = x) { return arguments[2] }
          for (let fn of [a, b, c, d, e, f]) {
            if ((await fn('x', 'y', 'z')) !== 'z') throw 'fail: ' + fn
          }
        }
      `,
    }, { async: true }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      // Functions must be able to access arguments past the argument count using a rest argument
      'in.js': `
        exports.async = async () => {
          async function a(...rest) { return rest[3] }
          async function b(x, y, ...rest) { return rest[1] }
          async function c(x, y = x, ...rest) { return rest[1] }
          let d = async function(...rest) { return rest[3] }
          let e = async function(x, y, ...rest) { return rest[1] }
          let f = async function(x, y = x, ...rest) { return rest[1] }
          let g = async (...rest) => rest[3]
          let h = async (x, y, ...rest) => rest[1]
          let i = async (x, y = x, ...rest) => rest[1]
          for (let fn of [a, b, c, d, e, f, g, h, i]) {
            if ((await fn(11, 22, 33, 44)) !== 44) throw 'fail: ' + fn
          }
        }
      `,
    }, { async: true }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      // Functions must be able to modify arguments using "arguments"
      'in.js': `
        exports.async = async () => {
          async function a(x) { let y = [x, arguments[0]]; arguments[0] = 'y'; return y.concat(x, arguments[0]) }
          let b = async function(x) { let y = [x, arguments[0]]; arguments[0] = 'y'; return y.concat(x, arguments[0]) }
          for (let fn of [a, b]) {
            let values = (await fn('x')) + ''
            if (values !== 'x,x,y,y') throw 'fail: ' + values
          }
        }
      `,
    }, { async: true }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      // Errors in the evaluation of async function arguments should reject the resulting promise
      'in.js': `
        exports.async = async () => {
          let expected = new Error('You should never see this error')
          let throws = () => { throw expected }
          async function a(x, y = throws()) {}
          async function b({ [throws()]: x }) {}
          let c = async function (x, y = throws()) {}
          let d = async function ({ [throws()]: x }) {}
          let e = async (x, y = throws()) => {}
          let f = async ({ [throws()]: x }) => {}
          for (let fn of [a, b, c, d, e, f]) {
            let promise = fn({})
            try {
              await promise
            } catch (e) {
              if (e === expected) continue
            }
            throw 'fail: ' + fn
          }
        }
      `,
    }, { async: true }),
    test(['in.js', '--outfile=node.js', '--target=es6'], {
      // Functions handle "super" property accesses
      'in.js': `
        exports.async = async () => {
          class Base {
            foo(x, y) {
              return x + y
            }
          }
          class Derived extends Base {
            async test(key) {
              return [
                await super.foo,
                await super[key],

                await super.foo.name,
                await super[key].name,
                await super.foo?.name,
                await super[key]?.name,
                await super._foo?.name,
                await super['_' + key]?.name,

                await super.foo(1, 2),
                await super[key](1, 2),
                await super.foo?.(1, 2),
                await super[key]?.(1, 2),
                await super._foo?.(1, 2),
                await super['_' + key]?.(1, 2),
              ]
            }
          }
          let d = new Derived
          let observed = await d.test('foo')
          let expected = [
            d.foo, d.foo,
            d.foo.name, d.foo.name, d.foo.name, d.foo.name, void 0, void 0,
            3, 3, 3, 3, void 0, void 0,
          ]
          for (let i = 0; i < expected.length; i++) {
            if (observed[i] !== expected[i]) {
              console.log(i, observed[i], expected[i])
              throw 'fail'
            }
          }
        }
      `,
    }, { async: true }),
  )

  // Function hoisting tests
  tests.push(
    test(['in.js', '--outfile=node.js'], {
      'in.js': `
        if (1) {
          function f() {
            return f
          }
          f = null
        }
        if (typeof f !== 'function' || f() !== null) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js'], {
      'in.js': `
        'use strict'
        if (1) {
          function f() {
            return f
          }
          f = null
        }
        if (typeof f !== 'undefined') throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--format=esm'], {
      'in.js': `
        export {}
        if (1) {
          function f() {
            return f
          }
          f = null
        }
        if (typeof f !== 'undefined') throw 'fail'
      `,
    }),
    test(['in.js', '--bundle', '--outfile=node.js'], {
      'in.js': `
        if (1) {
          function f() {
            return f
          }
          f = null
        }
        if (typeof f !== 'function' || f() !== null) throw 'fail'
      `,
    }),
    test(['in.js', '--bundle', '--outfile=node.js'], {
      'in.js': `
        var f
        if (1) {
          function f() {
            return f
          }
          f = null
        }
        if (typeof f !== 'function' || f() !== null) throw 'fail'
      `,
    }),
    test(['in.js', '--bundle', '--outfile=node.js'], {
      'in.js': `
        'use strict'
        if (1) {
          function f() {
            return f
          }
        }
        if (typeof f !== 'undefined') throw 'fail'
      `,
    }),
    test(['in.js', '--bundle', '--outfile=node.js'], {
      'in.js': `
        export {}
        if (1) {
          function f() {
            return f
          }
        }
        if (typeof f !== 'undefined') throw 'fail'
      `,
    }),
    test(['in.js', '--bundle', '--outfile=node.js'], {
      'in.js': `
        var f = 1
        if (1) {
          function f() {
            return f
          }
          f = null
        }
        if (typeof f !== 'function' || f() !== null) throw 'fail'
      `,
    }),
    test(['in.js', '--bundle', '--outfile=node.js'], {
      'in.js': `
        'use strict'
        var f = 1
        if (1) {
          function f() {
            return f
          }
        }
        if (f !== 1) throw 'fail'
      `,
    }),
    test(['in.js', '--bundle', '--outfile=node.js'], {
      'in.js': `
        export {}
        var f = 1
        if (1) {
          function f() {
            return f
          }
        }
        if (f !== 1) throw 'fail'
      `,
    }),
    test(['in.js', '--bundle', '--outfile=node.js'], {
      'in.js': `
        import {f, g} from './other'
        if (f !== void 0 || g !== 'g') throw 'fail'
      `,
      'other.js': `
        'use strict'
        var f
        if (1) {
          function f() {
            return f
          }
        }
        exports.f = f
        exports.g = 'g'
      `,
    }),
    test(['in.js', '--bundle', '--outfile=node.js'], {
      'in.js': `
        let f = 1
        // This should not be turned into "if (1) let f" because that's a syntax error
        if (1)
          function f() {
            return f
          }
        if (f !== 1) throw 'fail'
      `,
    }),
    test(['in.js', '--bundle', '--outfile=node.js'], {
      'in.js': `
        x: function f() { return 1 }
        if (f() !== 1) throw 'fail'
      `,
    }),
    test(['in.ts', '--outfile=node.js'], {
      'in.ts': `
        if (1) {
          var a = 'a'
          for (var b = 'b'; 0; ) ;
          for (var c in { c: 0 }) ;
          for (var d of ['d']) ;
          for (var e = 'e' in {}) ;
          function f() { return 'f' }
        }
        const observed = JSON.stringify({ a, b, c, d, e, f: f() })
        const expected = JSON.stringify({ a: 'a', b: 'b', c: 'c', d: 'd', e: 'e', f: 'f' })
        if (observed !== expected) throw observed
      `,
    }),
    test(['in.ts', '--bundle', '--outfile=node.js'], {
      'in.ts': `
        if (1) {
          var a = 'a'
          for (var b = 'b'; 0; ) ;
          for (var c in { c: 0 }) ;
          for (var d of ['d']) ;
          for (var e = 'e' in {}) ;
          function f() { return 'f' }
        }
        const observed = JSON.stringify({ a, b, c, d, e, f: f() })
        const expected = JSON.stringify({ a: 'a', b: 'b', c: 'c', d: 'd', e: 'e', f: 'f' })
        if (observed !== expected) throw observed
      `,
    }),
    test(['in.js', '--outfile=node.js', '--keep-names'], {
      'in.js': `
        var f
        if (1) function f() { return f }
        if (typeof f !== 'function' || f.name !== 'f') throw 'fail'
      `,
    }),
    test(['in.js', '--bundle', '--outfile=node.js', '--keep-names'], {
      'in.js': `
        var f
        if (1) function f() { return f }
        if (typeof f !== 'function' || f.name !== 'f') throw 'fail'
      `,
    }),
    test(['in.ts', '--outfile=node.js', '--keep-names'], {
      'in.ts': `
        if (1) {
          var a = 'a'
          for (var b = 'b'; 0; ) ;
          for (var c in { c: 0 }) ;
          for (var d of ['d']) ;
          for (var e = 'e' in {}) ;
          function f() {}
        }
        const observed = JSON.stringify({ a, b, c, d, e, f: f.name })
        const expected = JSON.stringify({ a: 'a', b: 'b', c: 'c', d: 'd', e: 'e', f: 'f' })
        if (observed !== expected) throw observed
      `,
    }),
    test(['in.ts', '--bundle', '--outfile=node.js', '--keep-names'], {
      'in.ts': `
        if (1) {
          var a = 'a'
          for (var b = 'b'; 0; ) ;
          for (var c in { c: 0 }) ;
          for (var d of ['d']) ;
          for (var e = 'e' in {}) ;
          function f() {}
        }
        const observed = JSON.stringify({ a, b, c, d, e, f: f.name })
        const expected = JSON.stringify({ a: 'a', b: 'b', c: 'c', d: 'd', e: 'e', f: 'f' })
        if (observed !== expected) throw observed
      `,
    }),
  )

  // Object rest pattern tests
  tests.push(
    // Test the correctness of side effect order for the TypeScript namespace exports
    test(['in.ts', '--outfile=node.js'], {
      'in.ts': `
        function fn() {
          let trail = []
          let t = k => (trail.push(k), k)
          let [
            { [t('a')]: a } = { a: t('x') },
            { [t('b')]: b, ...c } = { b: t('y') },
            { [t('d')]: d } = { d: t('z') },
          ] = [{ a: 1 }, { b: 2, bb: 3 }]
          return JSON.stringify({a, b, c, d, trail})
        }
        namespace ns {
          let trail = []
          let t = k => (trail.push(k), k)
          export let [
            { [t('a')]: a } = { a: t('x') },
            { [t('b')]: b, ...c } = { b: t('y') },
            { [t('d')]: d } = { d: t('z') },
          ] = [{ a: 1 }, { b: 2, bb: 3 }]
          export let result = JSON.stringify({a, b, c, d, trail})
        }
        if (fn() !== ns.result) throw 'fail'
      `,
    }),

    // Test the array and object rest patterns in TypeScript namespace exports
    test(['in.ts', '--outfile=node.js'], {
      'in.ts': `
        let obj = {};
        ({a: obj.a, ...obj.b} = {a: 1, b: 2, c: 3});
        [obj.c, , ...obj.d] = [1, 2, 3];
        ({e: obj.e, f: obj.f = 'f'} = {e: 'e'});
        [obj.g, , obj.h = 'h'] = ['g', 'gg'];
        namespace ns {
          export let {a, ...b} = {a: 1, b: 2, c: 3};
          export let [c, , ...d] = [1, 2, 3];
          export let {e, f = 'f'} = {e: 'e'};
          export let [g, , h = 'h'] = ['g', 'gg'];
        }
        if (JSON.stringify(obj) !== JSON.stringify(ns)) throw 'fail'
      `,
    }),

    // Test the initializer being overwritten
    test(['in.ts', '--outfile=node.js', '--target=es6'], {
      'in.ts': `
        var z = {x: {z: 'z'}, y: 'y'}, {x: z, ...y} = z
        if (y.y !== 'y' || z.z !== 'z') throw 'fail'
      `,
    }),
    test(['in.ts', '--outfile=node.js', '--target=es6'], {
      'in.ts': `
        var z = {x: {x: 'x'}, y: 'y'}, {[(z = {z: 'z'}, 'x')]: x, ...y} = z
        if (x.x !== 'x' || y.y !== 'y' || z.z !== 'z') throw 'fail'
      `,
    }),
  )

  // Code splitting tests
  tests.push(
    // Code splitting via sharing
    test(['a.js', 'b.js', '--outdir=out', '--splitting', '--format=esm', '--bundle'], {
      'a.js': `
        import * as ns from './common'
        export let a = 'a' + ns.foo
      `,
      'b.js': `
        import * as ns from './common'
        export let b = 'b' + ns.foo
      `,
      'common.js': `
        export let foo = 123
      `,
      'node.js': `
        import {a} from './out/a.js'
        import {b} from './out/b.js'
        if (a !== 'a123' || b !== 'b123') throw 'fail'
      `,
    }),

    // Code splitting via sharing with name templates
    test([
      'a.js', 'b.js', '--outdir=out', '--splitting', '--format=esm', '--bundle',
      '--entry-names=[name][dir]x', '--chunk-names=[name]/[hash]',
    ], {
      'a.js': `
        import * as ns from './common'
        export let a = 'a' + ns.foo
      `,
      'b.js': `
        import * as ns from './common'
        export let b = 'b' + ns.foo
      `,
      'common.js': `
        export let foo = 123
      `,
      'node.js': `
        import {a} from './out/a/x.js'
        import {b} from './out/b/x.js'
        if (a !== 'a123' || b !== 'b123') throw 'fail'
      `,
    }),

    // Code splitting via sharing with name templates
    test([
      'pages/a/index.js', 'pages/b/index.js', '--outbase=.',
      '--outdir=out', '--splitting', '--format=esm', '--bundle',
      '--entry-names=[name][dir]y', '--chunk-names=[name]/[hash]',
    ], {
      'pages/a/index.js': `
        import * as ns from '../common'
        export let a = 'a' + ns.foo
      `,
      'pages/b/index.js': `
        import * as ns from '../common'
        export let b = 'b' + ns.foo
      `,
      'pages/common.js': `
        export let foo = 123
      `,
      'node.js': `
        import {a} from './out/index/pages/a/y.js'
        import {b} from './out/index/pages/b/y.js'
        if (a !== 'a123' || b !== 'b123') throw 'fail'
      `,
    }),

    // Code splitting via ES6 module double-imported with sync and async imports
    test(['a.js', '--outdir=out', '--splitting', '--format=esm', '--bundle'], {
      'a.js': `
        import * as ns1 from './b'
        export default async function () {
          const ns2 = await import('./b')
          return [ns1.foo, -ns2.foo]
        }
      `,
      'b.js': `
        export let foo = 123
      `,
      'node.js': `
        export let async = async () => {
          const {default: fn} = await import('./out/a.js')
          const [a, b] = await fn()
          if (a !== 123 || b !== -123) throw 'fail'
        }
      `,
    }, { async: true }),

    // Code splitting via CommonJS module double-imported with sync and async imports
    test(['a.js', '--outdir=out', '--splitting', '--format=esm', '--bundle'], {
      'a.js': `
        import * as ns1 from './b.cjs'
        export default async function () {
          const ns2 = await import('./b.cjs')
          return [ns1.foo, -ns2.default.foo]
        }
      `,
      'b.cjs': `
        exports.foo = 123
      `,
      'node.js': `
        export let async = async () => {
          const {default: fn} = await import('./out/a.js')
          const [a, b] = await fn()
          if (a !== 123 || b !== -123) throw 'fail'
        }
      `,
    }, { async: true }),

    // Identical output chunks should not be shared
    test(['a.js', 'b.js', 'c.js', '--outdir=out', '--splitting', '--format=esm', '--bundle', '--minify'], {
      'a.js': `
        import {foo as common1} from './common1'
        import {foo as common2} from './common2'
        export let a = [common1, common2]
      `,
      'b.js': `
        import {foo as common2} from './common2'
        import {foo as common3} from './common3'
        export let b = [common2, common3]
      `,
      'c.js': `
        import {foo as common3} from './common3'
        import {foo as common1} from './common1'
        export let c = [common3, common1]
      `,
      'common1.js': `
        export let foo = {}
      `,
      'common2.js': `
        export let foo = {}
      `,
      'common3.js': `
        export let foo = {}
      `,
      'node.js': `
        import {a} from './out/a.js'
        import {b} from './out/b.js'
        import {c} from './out/c.js'
        if (a[0] === a[1]) throw 'fail'
        if (b[0] === b[1]) throw 'fail'
        if (c[0] === c[1]) throw 'fail'
      `,
    }),
    test(['a.js', 'b.js', 'c.js', '--outdir=out', '--splitting', '--format=esm', '--bundle', '--minify'], {
      'a.js': `
        export {a} from './common'
      `,
      'b.js': `
        export {b} from './common'
      `,
      'c.js': `
        export {a as ca, b as cb} from './common'
      `,
      'common.js': `
        export let a = {}
        export let b = {}
      `,
      'node.js': `
        import {a} from './out/a.js'
        import {b} from './out/b.js'
        import {ca, cb} from './out/c.js'
        if (a === b || ca === cb || a !== ca || b !== cb) throw 'fail'
      `,
    }),

    // "sideEffects": false
    // https://github.com/evanw/esbuild/issues/1081
    test(['entry.js', '--outdir=out', '--splitting', '--format=esm', '--bundle', '--chunk-names=[name]'], {
      'entry.js': `import('./a'); import('./b')`,
      'a.js': `import { bar } from './shared'; bar()`,
      'b.js': `import './shared'`,
      'shared.js': `import { foo } from './foo'; export let bar = foo`,
      'foo/index.js': `export let foo = () => {}`,
      'foo/package.json': `{ "sideEffects": false }`,
      'node.js': `
        import path from 'path'
        import url from 'url'
        const __dirname = path.dirname(url.fileURLToPath(import.meta.url))

        // Read the output files
        import fs from 'fs'
        const a = fs.readFileSync(path.join(__dirname, 'out', 'a.js'), 'utf8')
        const chunk = fs.readFileSync(path.join(__dirname, 'out', 'chunk.js'), 'utf8')

        // Make sure the two output files don't import each other
        import assert from 'assert'
        assert.notStrictEqual(chunk.includes('a.js'), a.includes('chunk.js'), 'chunks must not import each other')
      `,
    }),
    test(['entry.js', '--outdir=out', '--splitting', '--format=esm', '--bundle'], {
      'entry.js': `await import('./a'); await import('./b')`,
      'a.js': `import { bar } from './shared'; bar()`,
      'b.js': `import './shared'`,
      'shared.js': `import { foo } from './foo'; export let bar = foo`,
      'foo/index.js': `export let foo = () => {}`,
      'foo/package.json': `{ "sideEffects": false }`,
      'node.js': `
        // This must not crash
        import './out/entry.js'
      `,
    }),

    // Code splitting where only one entry point uses the runtime
    // https://github.com/evanw/esbuild/issues/1123
    test(['a.js', 'b.js', '--outdir=out', '--splitting', '--format=esm', '--bundle'], {
      'a.js': `
        import * as foo from './shared'
        export default foo
      `,
      'b.js': `
        import {bar} from './shared'
        export default bar
      `,
      'shared.js': `
        export function foo() {
          return 'foo'
        }
        export function bar() {
          return 'bar'
        }
      `,
      'node.js': `
        import a from './out/a.js'
        import b from './out/b.js'
        if (a.foo() !== 'foo') throw 'fail'
        if (b() !== 'bar') throw 'fail'
      `,
    }),

    // Code splitting with a dynamic import that imports a CSS file
    // https://github.com/evanw/esbuild/issues/1125
    test(['parent.js', '--outdir=out', '--splitting', '--format=esm', '--bundle'], {
      'parent.js': `
        // This should import the primary JS chunk, not the secondary CSS chunk
        await import('./child')
      `,
      'child.js': `
        import './foo.css'
      `,
      'foo.css': `
        body {
          color: black;
        }
      `,
      'node.js': `
        import './out/parent.js'
      `,
    }),

    // Code splitting with an entry point that exports two different
    // symbols with the same original name (minified and not minified)
    // https://github.com/evanw/esbuild/issues/1201
    test(['entry1.js', 'entry2.js', '--outdir=out', '--splitting', '--format=esm', '--bundle'], {
      'test1.js': `export const sameName = { test: 1 }`,
      'test2.js': `export const sameName = { test: 2 }`,
      'entry1.js': `
        export { sameName } from './test1.js'
        export { sameName as renameVar } from './test2.js'
      `,
      'entry2.js': `export * from './entry1.js'`,
      'node.js': `
        import { sameName as a, renameVar as b } from './out/entry1.js'
        import { sameName as c, renameVar as d } from './out/entry2.js'
        if (a.test !== 1 || b.test !== 2 || c.test !== 1 || d.test !== 2) throw 'fail'
      `,
    }),
    test(['entry1.js', 'entry2.js', '--outdir=out', '--splitting', '--format=esm', '--bundle', '--minify'], {
      'test1.js': `export const sameName = { test: 1 }`,
      'test2.js': `export const sameName = { test: 2 }`,
      'entry1.js': `
        export { sameName } from './test1.js'
        export { sameName as renameVar } from './test2.js'
      `,
      'entry2.js': `export * from './entry1.js'`,
      'node.js': `
        import { sameName as a, renameVar as b } from './out/entry1.js'
        import { sameName as c, renameVar as d } from './out/entry2.js'
        if (a.test !== 1 || b.test !== 2 || c.test !== 1 || d.test !== 2) throw 'fail'
      `,
    }),

    // https://github.com/evanw/esbuild/issues/1252
    test(['client.js', 'utilities.js', '--splitting', '--bundle', '--format=esm', '--outdir=out'], {
      'client.js': `export { Observable } from './utilities'`,
      'utilities.js': `export { Observable } from './observable'`,
      'observable.js': `
        import Observable from './zen-observable'
        export { Observable }
      `,
      'zen-observable.js': `module.exports = 123`,
      'node.js': `
        import {Observable as x} from './out/client.js'
        import {Observable as y} from './out/utilities.js'
        if (x !== 123 || y !== 123) throw 'fail'
      `,
    })
  )

  // Test the binary loader
  for (const length of [0, 1, 2, 3, 4, 5, 6, 7, 8, 256]) {
    const code = `
      import bytes from './data.bin'
      if (!(bytes instanceof Uint8Array)) throw 'not Uint8Array'
      if (bytes.length !== ${length}) throw 'Uint8Array.length !== ${length}'
      if (bytes.buffer.byteLength !== ${length}) throw 'ArrayBuffer.byteLength !== ${length}'
      for (let i = 0; i < ${length}; i++) if (bytes[i] !== (i ^ 0x55)) throw 'bad element ' + i
    `
    const data = Buffer.from([...' '.repeat(length)].map((_, i) => i ^ 0x55))
    tests.push(
      test(['entry.js', '--bundle', '--outfile=node.js', '--loader:.bin=binary', '--platform=browser'], {
        'entry.js': code,
        'data.bin': data,
      }),
      test(['entry.js', '--bundle', '--outfile=node.js', '--loader:.bin=binary', '--platform=node'], {
        'entry.js': code,
        'data.bin': data,
      }),
    )
  }

  // Test file handle errors other than ENOENT
  {
    const errorText = process.platform === 'win32' ? 'The handle is invalid.' : 'is a directory';
    tests.push(
      test(['src/entry.js', '--bundle', '--outfile=node.js', '--sourcemap'], {
        'src/entry.js': `
        //# sourceMappingURL=entry.js.map
      `,
        'src/entry.js.map/x': ``,
      }, {
        expectedStderr: ` > src/entry.js:2:29: warning: Cannot read file "src/entry.js.map": ${errorText}
    2 │         //# sourceMappingURL=entry.js.map
      ╵                              ~~~~~~~~~~~~

`,
      }),
      test(['src/entry.js', '--bundle', '--outfile=node.js'], {
        'src/entry.js': ``,
        'src/tsconfig.json': `{"extends": "./base.json"}`,
        'src/base.json/x': ``,
      }, {
        expectedStderr: ` > src/tsconfig.json:1:12: error: Cannot read file "src/base.json": ${errorText}
    1 │ {"extends": "./base.json"}
      ╵             ~~~~~~~~~~~~~

`,
      }),
      test(['src/entry.js', '--bundle', '--outfile=node.js'], {
        'src/entry.js': ``,
        'src/tsconfig.json': `{"extends": "foo"}`,
        'node_modules/foo/tsconfig.json/x': ``,
      }, {
        expectedStderr: ` > src/tsconfig.json:1:12: error: Cannot read file "node_modules/foo/tsconfig.json": ${errorText}
    1 │ {"extends": "foo"}
      ╵             ~~~~~

`,
      }),
      test(['src/entry.js', '--bundle', '--outfile=node.js'], {
        'src/entry.js': ``,

        // These missing directories shouldn't cause any errors on Windows
        'package.json': `{
          "main": "dist/cjs/index.js",
          "module": "dist/esm/index.js"
        }`,
      }),
    )
  }

  // Test a special-case error message for people trying to use "'--" on Windows
  tests.push(
    test(['in.js', `'--define:process.env.NODE_ENV="production"'`], {
      'in.js': ``,
    }, {
      expectedStderr: ` > error: Unexpected single quote character before flag (use \\" to ` +
        `escape double quotes): '--define:process.env.NODE_ENV="production"'

`,
    }),
  )

  // Test injecting banner and footer
  tests.push(
    test(['in.js', '--outfile=node.js', '--banner:js=const bannerDefined = true;'], {
      'in.js': `if (!bannerDefined) throw 'fail'`
    }),
    test(['in.js', '--outfile=node.js', '--footer:js=function footer() { }'], {
      'in.js': `footer()`
    }),
    test(['a.js', 'b.js', '--outdir=out', '--bundle', '--format=cjs', '--banner:js=const bannerDefined = true;', '--footer:js=function footer() { }'], {
      'a.js': `
        module.exports = { banner: bannerDefined, footer };
      `,
      'b.js': `
        module.exports = { banner: bannerDefined, footer };
      `,
      'node.js': `
        const a = require('./out/a');
        const b = require('./out/b');

        if (!a.banner || !b.banner) throw 'fail';
        a.footer();
        b.footer();
      `
    }),
  )

  // Test "exports" in package.json
  for (const flags of [[], ['--bundle']]) {
    tests.push(
      test(['in.js', '--outfile=node.js', '--format=esm'].concat(flags), {
        'in.js': `import abc from 'pkg'; if (abc !== 123) throw 'fail'`,
        'package.json': `{ "type": "module" }`,
        'node_modules/pkg/subdir/foo.js': `export default 123`,
        'node_modules/pkg/package.json': `{
          "type": "module",
          "exports": {
            ".": "./subdir/foo.js"
          }
        }`,
      }),
      test(['in.js', '--outfile=node.js', '--format=esm'].concat(flags), {
        'in.js': `import abc from 'pkg'; if (abc !== 123) throw 'fail'`,
        'package.json': `{ "type": "module" }`,
        'node_modules/pkg/subdir/foo.js': `export default 123`,
        'node_modules/pkg/package.json': `{
          "type": "module",
          "exports": {
            ".": {
              "default": "./subdir/foo.js"
            }
          }
        }`,
      }),
      test(['in.js', '--outfile=node.js', '--format=esm'].concat(flags), {
        'in.js': `import abc from 'pkg'; if (abc !== 123) throw 'fail'`,
        'package.json': `{ "type": "module" }`,
        'node_modules/pkg/subdir/foo.js': `export default 123`,
        'node_modules/pkg/package.json': `{
          "type": "module",
          "exports": {
            "default": "./subdir/foo.js"
          }
        }`,
      }),
      test(['in.js', '--outfile=node.js', '--format=esm'].concat(flags), {
        'in.js': `import abc from 'pkg/foo.js'; if (abc !== 123) throw 'fail'`,
        'package.json': `{ "type": "module" }`,
        'node_modules/pkg/subdir/foo.js': `export default 123`,
        'node_modules/pkg/package.json': `{
          "type": "module",
          "exports": {
            "./": "./subdir/"
          }
        }`,
      }),
      test(['in.js', '--outfile=node.js', '--format=esm'].concat(flags), {
        'in.js': `import abc from 'pkg/foo.js'; if (abc !== 123) throw 'fail'`,
        'package.json': `{ "type": "module" }`,
        'node_modules/pkg/subdir/foo.js': `export default 123`,
        'node_modules/pkg/package.json': `{
          "type": "module",
          "exports": {
            "./": {
              "default": "./subdir/"
            }
          }
        }`,
      }),
      test(['in.js', '--outfile=node.js', '--format=esm'].concat(flags), {
        'in.js': `import abc from 'pkg/dir/foo.js'; if (abc !== 123) throw 'fail'`,
        'package.json': `{ "type": "module" }`,
        'node_modules/pkg/subdir/foo.js': `export default 123`,
        'node_modules/pkg/package.json': `{
          "type": "module",
          "exports": {
            "./dir/": "./subdir/"
          }
        }`,
      }),
      test(['in.js', '--outfile=node.js', '--format=esm'].concat(flags), {
        'in.js': `import abc from 'pkg/dir/foo.js'; if (abc !== 123) throw 'fail'`,
        'package.json': `{ "type": "module" }`,
        'node_modules/pkg/subdir/foo.js': `export default 123`,
        'node_modules/pkg/package.json': `{
          "type": "module",
          "exports": {
            "./dir/": {
              "default": "./subdir/"
            }
          }
        }`,
      }),
      test(['in.js', '--outfile=node.js', '--format=esm'].concat(flags), {
        'in.js': `import abc from '@scope/pkg'; if (abc !== 123) throw 'fail'`,
        'package.json': `{ "type": "module" }`,
        'node_modules/@scope/pkg/subdir/foo.js': `export default 123`,
        'node_modules/@scope/pkg/package.json': `{
          "type": "module",
          "exports": {
            ".": "./subdir/foo.js"
          }
        }`,
      }),
      test(['in.js', '--outfile=node.js', '--format=esm'].concat(flags), {
        'in.js': `import abc from '@scope/pkg'; if (abc !== 123) throw 'fail'`,
        'package.json': `{ "type": "module" }`,
        'node_modules/@scope/pkg/subdir/foo.js': `export default 123`,
        'node_modules/@scope/pkg/package.json': `{
          "type": "module",
          "exports": {
            ".": {
              "default": "./subdir/foo.js"
            }
          }
        }`,
      }),
      test(['in.js', '--outfile=node.js', '--format=esm'].concat(flags), {
        'in.js': `import abc from '@scope/pkg'; if (abc !== 123) throw 'fail'`,
        'package.json': `{ "type": "module" }`,
        'node_modules/@scope/pkg/subdir/foo.js': `export default 123`,
        'node_modules/@scope/pkg/package.json': `{
          "type": "module",
          "exports": {
            "default": "./subdir/foo.js"
          }
        }`,
      }),
      test(['in.js', '--outfile=node.js', '--format=esm'].concat(flags), {
        'in.js': `import abc from '@scope/pkg/foo.js'; if (abc !== 123) throw 'fail'`,
        'package.json': `{ "type": "module" }`,
        'node_modules/@scope/pkg/subdir/foo.js': `export default 123`,
        'node_modules/@scope/pkg/package.json': `{
          "type": "module",
          "exports": {
            "./": "./subdir/"
          }
        }`,
      }),
      test(['in.js', '--outfile=node.js', '--format=esm'].concat(flags), {
        'in.js': `import abc from '@scope/pkg/foo.js'; if (abc !== 123) throw 'fail'`,
        'package.json': `{ "type": "module" }`,
        'node_modules/@scope/pkg/subdir/foo.js': `export default 123`,
        'node_modules/@scope/pkg/package.json': `{
          "type": "module",
          "exports": {
            "./": {
              "default": "./subdir/"
            }
          }
        }`,
      }),
      test(['in.js', '--outfile=node.js', '--format=esm'].concat(flags), {
        'in.js': `import abc from '@scope/pkg/dir/foo.js'; if (abc !== 123) throw 'fail'`,
        'package.json': `{ "type": "module" }`,
        'node_modules/@scope/pkg/subdir/foo.js': `export default 123`,
        'node_modules/@scope/pkg/package.json': `{
          "type": "module",
          "exports": {
            "./dir/": "./subdir/"
          }
        }`,
      }),
      test(['in.js', '--outfile=node.js', '--format=esm'].concat(flags), {
        'in.js': `import abc from '@scope/pkg/dir/foo.js'; if (abc !== 123) throw 'fail'`,
        'package.json': `{ "type": "module" }`,
        'node_modules/@scope/pkg/subdir/foo.js': `export default 123`,
        'node_modules/@scope/pkg/package.json': `{
          "type": "module",
          "exports": {
            "./dir/": {
              "default": "./subdir/"
            }
          }
        }`,
      }),
      test(['in.js', '--outfile=node.js', '--format=esm'].concat(flags), {
        'in.js': `import abc from 'pkg/dirwhat'; if (abc !== 123) throw 'fail'`,
        'package.json': `{ "type": "module" }`,
        'node_modules/pkg/sub/what/dirwhat/foo.js': `export default 123`,
        'node_modules/pkg/package.json': `{
          "type": "module",
          "exports": {
            "./di*": "./nope.js",
            "./dir*": "./sub/*/dir*/foo.js",
            "./long*": "./nope.js",
            "./d*": "./nope.js"
          }
        }`,
      }),
      test(['in.js', '--outfile=node.js', '--format=cjs'].concat(flags), {
        'in.js': `const abc = require('pkg/dir/test'); if (abc !== 123) throw 'fail'`,
        'package.json': `{ "type": "module" }`,
        'node_modules/pkg/sub/test.js': `module.exports = 123`,
        'node_modules/pkg/package.json': `{
          "exports": {
            "./dir/": "./sub/"
          }
        }`,
      }),
      test(['in.js', '--outfile=node.js', '--format=cjs'].concat(flags), {
        'in.js': `const abc = require('pkg/dir/test'); if (abc !== 123) throw 'fail'`,
        'package.json': `{ "type": "module" }`,
        'node_modules/pkg/sub/test/index.js': `module.exports = 123`,
        'node_modules/pkg/package.json': `{
          "exports": {
            "./dir/": "./sub/"
          }
        }`,
      }),
      test(['in.js', '--outfile=node.js', '--format=esm'].concat(flags), {
        'in.js': `import abc from 'pkg/foo'; if (abc !== 123) throw 'fail'`,
        'package.json': `{ "type": "module" }`,
        'node_modules/pkg/yes.js': `export default 123`,
        'node_modules/pkg/package.json': `{
          "type": "module",
          "exports": {
            "./foo": [
              { "unused": "./no.js" },
              "./yes.js"
            ]
          }
        }`,
      }),
      test(['in.js', '--outfile=node.js', '--format=esm'].concat(flags), {
        'in.js': `import abc from 'pkg/foo'; if (abc !== 123) throw 'fail'`,
        'package.json': `{ "type": "module" }`,
        'node_modules/pkg/yes.js': `export default 123`,
        'node_modules/pkg/package.json': `{
          "type": "module",
          "exports": {
            "./foo": [
              { "default": "./yes.js" },
              "./no.js"
            ]
          }
        }`,
      }),
    )
  }

  // Top-level await tests
  tests.push(
    test(['in.js', '--outdir=out', '--format=esm', '--bundle'], {
      'in.js': `
        function foo() {
          globalThis.tlaTrace.push(2)
          return import('./a.js')
        }

        globalThis.tlaTrace = []
        globalThis.tlaTrace.push(1)
        const it = (await foo()).default
        globalThis.tlaTrace.push(6)
        if (it !== 123 || globalThis.tlaTrace.join(',') !== '1,2,3,4,5,6') throw 'fail'
      `,
      'a.js': `
        globalThis.tlaTrace.push(5)
        export { default } from './b.js'
      `,
      'b.js': `
        globalThis.tlaTrace.push(3)
        export default await Promise.resolve(123)
        globalThis.tlaTrace.push(4)
      `,
      'node.js': `
        import './out/in.js'
      `,
    }),
  )

  // Test writing to stdout
  tests.push(
    // These should succeed
    testStdout('exports.foo = 123', [], async (build) => {
      const stdout = await build()
      assert.strictEqual(stdout, `exports.foo = 123;\n`)
    }),
    testStdout('exports.foo = 123', ['--bundle', '--format=cjs'], async (build) => {
      const stdout = await build()
      assert.strictEqual(stdout, `// example.js\nexports.foo = 123;\n`)
    }),
    testStdout('exports.foo = 123', ['--sourcemap'], async (build) => {
      const stdout = await build()
      const start = `exports.foo = 123;\n//# sourceMappingURL=data:application/json;base64,`
      assert.default(stdout.startsWith(start))
      const json = JSON.parse(Buffer.from(stdout.slice(start.length), 'base64').toString())
      assert.strictEqual(json.version, 3)
      assert.deepStrictEqual(json.sources, ['example.js'])
    }),
    testStdout('exports.foo = 123', ['--bundle', '--format=cjs', '--sourcemap'], async (build) => {
      const stdout = await build()
      const start = `// example.js\nexports.foo = 123;\n//# sourceMappingURL=data:application/json;base64,`
      assert.default(stdout.startsWith(start))
      const json = JSON.parse(Buffer.from(stdout.slice(start.length), 'base64').toString())
      assert.strictEqual(json.version, 3)
      assert.deepStrictEqual(json.sources, ['example.js'])
    }),
    testStdout('stuff', ['--loader:.js=text'], async (build) => {
      const stdout = await build()
      assert.strictEqual(stdout, `module.exports = "stuff";\n`)
    }),

    // These should fail
    testStdout('exports.foo = 123', ['--metafile=graph.json'], async (build) => {
      try { await build() } catch (e) { return }
      throw new Error('Expected build failure for "--metafile"')
    }),
    testStdout('exports.foo = 123', ['--sourcemap=external'], async (build) => {
      try { await build() } catch (e) { return }
      throw new Error('Expected build failure for "--metafile"')
    }),
    testStdout('exports.foo = 123', ['--loader:.js=file'], async (build) => {
      try { await build() } catch (e) { return }
      throw new Error('Expected build failure for "--metafile"')
    }),
  )

  // Test for a Windows-specific issue where paths starting with "/" could be
  // treated as relative paths, leading to inconvenient cross-platform failures:
  // https://github.com/evanw/esbuild/issues/822
  tests.push(
    test(['in.js', '--bundle'], {
      'in.js': `
        import "/file.js"
      `,
      'file.js': `This file should not be imported on Windows`,
    }, {
      expectedStderr: ` > in.js:2:15: error: Could not resolve "/file.js"
    2 │         import "/file.js"
      ╵                ~~~~~~~~~~

`,
    }),
  )

  // Test that importing a path with the wrong case works ok. This is necessary
  // to handle case-insensitive file systems.
  if (process.platform === 'darwin' || process.platform === 'win32') {
    tests.push(
      test(['in.js', '--bundle', '--outfile=node.js'], {
        'in.js': `
          import x from "./File1.js"
          import y from "./file2.js"
          if (x !== 123 || y !== 234) throw 'fail'
        `,
        'file1.js': `export default 123`,
        'File2.js': `export default 234`,
      }, {
        expectedStderr: ` > in.js:2:24: warning: Use "file1.js" instead of "File1.js" to avoid issues with case-sensitive file systems
    2 │           import x from "./File1.js"
      ╵                         ~~~~~~~~~~~~

 > in.js:3:24: warning: Use "File2.js" instead of "file2.js" to avoid issues with case-sensitive file systems
    3 │           import y from "./file2.js"
      ╵                         ~~~~~~~~~~~~

`,
      }),
      test(['in.js', '--bundle', '--outfile=node.js'], {
        'in.js': `
          import x from "./Dir1/file.js"
          import y from "./dir2/file.js"
          if (x !== 123 || y !== 234) throw 'fail'
        `,
        'dir1/file.js': `export default 123`,
        'Dir2/file.js': `export default 234`,
      }),

      // Warn when importing something inside node_modules
      test(['in.js', '--bundle', '--outfile=node.js'], {
        'in.js': `
          import x from "pkg/File1.js"
          import y from "pkg/file2.js"
          if (x !== 123 || y !== 234) throw 'fail'
        `,
        'node_modules/pkg/file1.js': `export default 123`,
        'node_modules/pkg/File2.js': `export default 234`,
      }, {
        expectedStderr: ` > in.js:2:24: warning: Use "node_modules/pkg/file1.js" instead of "node_modules/pkg/File1.js" to avoid issues with case-sensitive file systems
    2 │           import x from "pkg/File1.js"
      ╵                         ~~~~~~~~~~~~~~

 > in.js:3:24: warning: Use "node_modules/pkg/File2.js" instead of "node_modules/pkg/file2.js" to avoid issues with case-sensitive file systems
    3 │           import y from "pkg/file2.js"
      ╵                         ~~~~~~~~~~~~~~

`,
      }),

      // Don't warn when the importer is inside node_modules
      test(['in.js', '--bundle', '--outfile=node.js'], {
        'in.js': `
          import {x, y} from "pkg"
          if (x !== 123 || y !== 234) throw 'fail'
        `,
        'node_modules/pkg/index.js': `
          export {default as x} from "./File1.js"
          export {default as y} from "./file2.js"
        `,
        'node_modules/pkg/file1.js': `export default 123`,
        'node_modules/pkg/File2.js': `export default 234`,
      }),
    )
  }

  function test(args, files, options) {
    return async () => {
      const hasBundle = args.includes('--bundle')
      const hasIIFE = args.includes('--format=iife')
      const hasUMD = args.includes('--format=umd')
      const hasCJS = args.includes('--format=cjs')
      const hasESM = args.includes('--format=esm')
      const formats = hasIIFE ? ['iife'] : hasUMD ? ['umd'] : hasESM ? ['esm'] : hasCJS || !hasBundle ? ['cjs'] : ['cjs', 'esm']
      const expectedStderr = options && options.expectedStderr || '';

      // If the test doesn't specify a format, test both formats
      for (const format of formats) {
        const formatArg = `--format=${format}`
        const logLevelArgs = args.some(arg => arg.startsWith('--log-level=')) ? [] : ['--log-level=warning']
        const modifiedArgs = (!hasBundle || args.includes(formatArg) ? args : args.concat(formatArg)).concat(logLevelArgs)
        const thisTestDir = path.join(testDir, '' + testCount++)

        try {
          // Test setup
          for (const file in files) {
            const filePath = path.join(thisTestDir, file)
            const contents = files[file]
            await fs.mkdir(path.dirname(filePath), { recursive: true })

            // Optionally symlink the file if the test requests it
            if (contents.symlink) await fs.symlink(contents.symlink.replace('TEST_DIR_ABS_PATH', thisTestDir), filePath)
            else await fs.writeFile(filePath, contents)
          }

          // Run esbuild
          let stderr
          if (options && options.cwd) {
            // Use the shell to set the working directory instead of using node's
            // "child_process" module. For some reason it looks like node doesn't
            // handle symlinks correctly and some of these tests check esbuild's
            // behavior in the presence of symlinks. Using the shell is the only
            // way I could find to do this correctly.
            const quote = arg => arg.replace(/([#!"$&'()*,:;<=>?@\[\\\]^`{|}])/g, '\\$1')
            const cwd = path.join(thisTestDir, options.cwd)
            const command = ['cd', quote(cwd), '&&', quote(esbuildPath)].concat(modifiedArgs.map(quote)).join(' ')
            stderr = (await execAsync(command, { stdio: 'pipe' })).stderr
          } else {
            stderr = (await execFileAsync(esbuildPath, modifiedArgs, { cwd: thisTestDir, stdio: 'pipe' })).stderr
          }
          assert.strictEqual(stderr, expectedStderr);

          // Run the resulting node.js file and make sure it exits cleanly. The
          // use of "pathToFileURL" is a workaround for a problem where node
          // only supports absolute paths on Unix-style systems, not on Windows.
          // See https://github.com/nodejs/node/issues/31710 for more info.
          const nodePath = path.join(thisTestDir, 'node')
          let testExports
          switch (format) {
            case 'cjs':
            case 'iife':
            case 'umd':
              await fs.writeFile(path.join(thisTestDir, 'package.json'), '{"type": "commonjs"}')
              testExports = (await import(url.pathToFileURL(`${nodePath}.js`))).default
              break

            case 'esm':
              await fs.writeFile(path.join(thisTestDir, 'package.json'), '{"type": "module"}')
              testExports = await import(url.pathToFileURL(`${nodePath}.js`))
              break
          }

          // If this is an async test, run the async part
          if (options && options.async) {
            if (!(testExports.async instanceof Function))
              throw new Error('Expected async instanceof Function')
            await testExports.async()
          }

          // Clean up test output
          removeRecursiveSync(thisTestDir)
        }

        catch (e) {
          if (e && e.stderr !== void 0) {
            try {
              assert.strictEqual(e.stderr, expectedStderr);

              // Clean up test output
              removeRecursiveSync(thisTestDir)
              continue;
            } catch (e2) {
              e = e2;
            }
          }
          console.error(`❌ test failed: ${e && e.message || e}
  dir: ${path.relative(dirname, thisTestDir)}
  args: ${modifiedArgs.join(' ')}
  files: ${Object.entries(files).map(([k, v]) => `\n    ${k}: ${JSON.stringify(v)}`).join('')}
`)
          return false
        }
      }

      return true
    }
  }

  // There's a feature where bundling without "outfile" or "outdir" writes to stdout instead
  function testStdout(input, args, callback) {
    return async () => {
      const thisTestDir = path.join(testDir, '' + testCount++)

      try {
        await fs.mkdir(thisTestDir, { recursive: true })
        const inputFile = path.join(thisTestDir, 'example.js')
        await fs.writeFile(inputFile, input)

        // Run whatever check the caller is doing
        await callback(async () => {
          const { stdout } = await execFileAsync(
            esbuildPath, [inputFile, '--log-level=warning'].concat(args), { cwd: thisTestDir, stdio: 'pipe' })
          return stdout
        })

        // Clean up test output
        removeRecursiveSync(thisTestDir)
      } catch (e) {
        console.error(`❌ test failed: ${e && e.message || e}
  dir: ${path.relative(dirname, thisTestDir)}`)
        return false
      }

      return true
    }
  }

  // Create a fresh test directory
  removeRecursiveSync(testDir)
  await fs.mkdir(testDir, { recursive: true })

  // Run tests in batches so they work in CI, which has a limited memory ceiling
  let allTestsPassed = true
  let batch = 32
  for (let i = 0; i < tests.length; i += batch) {
    let promises = []
    for (let test of tests.slice(i, i + batch)) {
      let promise = test()
      promise.then(
        success => { if (!success) allTestsPassed = false },
        () => allTestsPassed = false,
      )
      promises.push(promise)
    }
    await Promise.all(promises)
  }

  if (!allTestsPassed) {
    console.error(`❌ end-to-end tests failed`)
    process.exit(1)
  } else {
    console.log(`✅ end-to-end tests passed`)
    removeRecursiveSync(testDir)
  }
})().catch(e => setTimeout(() => { throw e }))
