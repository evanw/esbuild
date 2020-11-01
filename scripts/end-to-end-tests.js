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
  const { default: { buildBinary, dirname } } = await import('./esbuild.js')
  const { default: rimraf } = await import('rimraf')
  const assert = await import('assert')
  const path = await import('path')
  const util = await import('util')
  const url = await import('url')
  const fs = (await import('fs')).promises

  const execFileAsync = util.promisify(childProcess.execFile)

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

  // Tests for symlinks
  //
  // Note: These are disabled on Windows because they fail when run with GitHub
  // Actions. I'm not sure what the issue is because they pass for me when run in
  // my Windows VM (Windows 10 in VirtualBox on macOS).
  if (process.platform !== 'win32') {
    tests.push(
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
    )
  }

  // Test internal import order (see https://github.com/evanw/esbuild/issues/421)
  tests.push(
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

    // Self export
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `exports.foo = 123; const out = require('./in'); if (out.__esModule || out.foo !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `module.exports = 123; const out = require('./in'); if (out.__esModule || out !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `export const foo = 123; const out = require('./in'); if (!out.__esModule || out.foo !== 123) throw 'fail'`,
    }),
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `export default 123; const out = require('./in'); if (!out.__esModule || out.default !== 123) throw 'fail'`,
    }),

    // Complex circular import case, must not crash
    test(['--bundle', 'in.js', '--outfile=node.js'], {
      'in.js': `
        import {bar} from './re-export'
        if (bar() !== 123) throw 'fail'
      `,
      're-export.js': `
        export * from './foo'
        export * from './bar'
      `,
      'foo.js': `
        export function foo() {
          return module.exports.foo ? 123 : 234 // "module" makes this a CommonJS file
        }
      `,
      'bar.js': `
        import {getFoo} from './get'
        export let bar = getFoo(module) // "module" makes this a CommonJS file
      `,
      'get.js': `
        import {foo} from './foo'
        export function getFoo() {
          return foo
        }
      `,
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
      )
    }
  }

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
          if (!error || error.message !== 'require is not defined') throw 'fail'
        }
        export {fn as async}
      `,
    }, {
      async: true,
      expectedStderr: `in.js:2:25: warning: Converting "require" to "esm" is currently not supported
        const {exists} = require('fs')
                         ~~~~~~~
1 warning
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
          if (!error || error.message !== 'require is not defined') throw 'fail'
        }
        export {fn as async}
      `,
    }, {
      async: true,
      expectedStderr: `in.js:2:19: warning: Converting "require" to "esm" is currently not supported
        const fs = require('fs')
                   ~~~~~~~
1 warning
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
  )

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
    test(['in.js', '--outfile=node.js', '--bundle'], {
      'in.js': `const ns = require('./foo'); if (ns.a !== 123 || ns.b !== void 0) throw 'fail'`,
      'foo.js': `export let a = 123, b = this`,
    }),
  )

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
      expectedStderr: `in.js:23:30: warning: Reading from setter-only property "#setter" will throw
            expect(() => this.#setter, 'member.get is not a function')
                              ~~~~~~~
in.js:24:30: warning: Writing to getter-only property "#getter" will throw
            expect(() => this.#getter = 1, 'member.set is not a function')
                              ~~~~~~~
2 warnings
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
        import * as ns1 from './b'
        export default async function () {
          const ns2 = await import('./b')
          return [ns1.foo, -ns2.default.foo]
        }
      `,
      'b.js': `
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
        expectedStderr: `src/entry.js:2:29: error: Cannot read file "src/entry.js.map": ${errorText}
        //# sourceMappingURL=entry.js.map
                             ~~~~~~~~~~~~
1 error
` ,
      }),
      test(['src/entry.js', '--bundle', '--outfile=node.js'], {
        'src/entry.js/x': ``,
      }, {
        expectedStderr: `error: Cannot read file "src/entry.js": ${errorText}
1 error
`,
      }),
      test(['src/entry.js', '--bundle', '--outfile=node.js'], {
        'src/entry.js': ``,
        'src/tsconfig.json': `{"extends": "./base.json"}`,
        'src/base.json/x': ``,
      }, {
        expectedStderr: `src/tsconfig.json:1:12: error: Cannot read file "src/base.json": ${errorText}
{"extends": "./base.json"}
            ~~~~~~~~~~~~~
1 error
`,
      }),
      test(['src/entry.js', '--bundle', '--outfile=node.js'], {
        'src/entry.js': ``,
        'src/tsconfig.json': `{"extends": "foo"}`,
        'node_modules/foo/tsconfig.json/x': ``,
      }, {
        expectedStderr: `src/tsconfig.json:1:12: error: Cannot read file "node_modules/foo/tsconfig.json": ${errorText}
{"extends": "foo"}
            ~~~~~
1 error
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

  function test(args, files, options) {
    return async () => {
      const hasBundle = args.includes('--bundle')
      const hasIIFE = args.includes('--format=iife')
      const hasCJS = args.includes('--format=cjs')
      const hasESM = args.includes('--format=esm')
      const formats = hasIIFE ? ['iife'] : hasESM ? ['esm'] : hasCJS || !hasBundle ? ['cjs'] : ['cjs', 'esm']
      const expectedStderr = options && options.expectedStderr || '';

      // If the test doesn't specify a format, test both formats
      for (const format of formats) {
        const formatArg = `--format=${format}`
        const modifiedArgs = !hasBundle || args.includes(formatArg) ? args : args.concat(formatArg)
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
          const { stderr } = await execFileAsync(esbuildPath, modifiedArgs,
            { cwd: thisTestDir, stdio: 'pipe' })
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
          rimraf.sync(thisTestDir, { disableGlob: true });
        }

        catch (e) {
          if (e && e.stderr !== void 0) {
            try {
              assert.strictEqual(e.stderr, expectedStderr);

              // Clean up test output
              rimraf.sync(thisTestDir, { disableGlob: true });
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
            esbuildPath, [inputFile].concat(args), { cwd: thisTestDir, stdio: 'pipe' })
          return stdout
        })

        // Clean up test output
        rimraf.sync(thisTestDir, { disableGlob: true })
      } catch (e) {
        console.error(`❌ test failed: ${e && e.message || e}
  dir: ${path.relative(dirname, thisTestDir)}`)
        return false
      }

      return true
    }
  }

  // Create a fresh test directory
  rimraf.sync(testDir, { disableGlob: true })
  await fs.mkdir(testDir, { recursive: true })

  // Run all tests concurrently
  const allTestsPassed = (await Promise.all(tests.map(test => test()))).every(success => success)

  if (!allTestsPassed) {
    console.error(`❌ end-to-end tests failed`)
    process.exit(1)
  } else {
    console.log(`✅ end-to-end tests passed`)

    // Clean up test output
    rimraf.sync(testDir, { disableGlob: true })
  }
})().catch(e => setTimeout(() => { throw e }))
