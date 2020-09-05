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
  const { default: mkdirp } = await import('mkdirp')
  const { default: rimraf } = await import('rimraf')
  const assert = await import('assert')
  const path = await import('path')
  const util = await import('util')
  const url = await import('url')
  const fs = await import('fs')

  const symlinkAsync = util.promisify(fs.symlink)
  const writeFileAsync = util.promisify(fs.writeFile)
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
        if (test.default || test.prop !== 123) throw 'fail'
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
        let called = false
        class Foo {
          foo
          set foo(x) { called = true }
        }
        new Foo()
        if (!called) throw 'fail'
      `,
    }),
    test(['in.js', '--outfile=node.js', '--target=es6', '--strict'], {
      'in.js': `
        let called = false
        class Foo {
          foo
          set foo(x) { called = true }
        }
        new Foo()
        if (called) throw 'fail'
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
      const hasCJS = args.includes('--format=cjs')
      const hasESM = args.includes('--format=esm')
      const formats = hasCJS || !hasBundle ? ['cjs'] : hasESM ? ['esm'] : ['cjs', 'esm']
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
            mkdirp.sync(path.dirname(filePath))

            // Optionally symlink the file if the test requests it
            if (contents.symlink) await symlinkAsync(contents.symlink.replace('TEST_DIR_ABS_PATH', thisTestDir), filePath)
            else await writeFileAsync(filePath, contents)
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
              await writeFileAsync(path.join(thisTestDir, 'package.json'), '{"type": "commonjs"}')
              testExports = (await import(url.pathToFileURL(`${nodePath}.js`))).default
              break

            case 'esm':
              await writeFileAsync(path.join(thisTestDir, 'package.json'), '{"type": "module"}')
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
          rimraf.sync(thisTestDir, { disableGlob: true })
        }

        catch (e) {
          if (e && e.stderr !== void 0) {
            try {
              assert.strictEqual(e.stderr, expectedStderr);
              return true;
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
        mkdirp.sync(thisTestDir)
        const inputFile = path.join(thisTestDir, 'example.js')
        await writeFileAsync(inputFile, input)

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
  fs.mkdirSync(testDir)

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
