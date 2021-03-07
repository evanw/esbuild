const puppeteer = require('puppeteer')
const http = require('http')
const path = require('path')
const url = require('url')
const fs = require('fs')

const js = fs.readFileSync(path.join(__dirname, '..', '..', 'npm', 'esbuild-wasm', 'lib', 'browser.js'))
const jsMin = fs.readFileSync(path.join(__dirname, '..', '..', 'npm', 'esbuild-wasm', 'lib', 'browser.min.js'))
const esm = fs.readFileSync(path.join(__dirname, '..', '..', 'npm', 'esbuild-wasm', 'esm', 'browser.js'))
const esmMin = fs.readFileSync(path.join(__dirname, '..', '..', 'npm', 'esbuild-wasm', 'esm', 'browser.min.js'))
const wasm = fs.readFileSync(path.join(__dirname, '..', '..', 'npm', 'esbuild-wasm', 'esbuild.wasm'))

// This is converted to a string and run inside the browser
async function runAllTests({ esbuild, service }) {
  function setupForProblemCSS(prefix) {
    // https://github.com/tailwindlabs/tailwindcss/issues/2889
    const original = `
      /* Variant 1 */
      .${prefix}-v1 { --a: ; --b: ; max-width: var(--a) var(--b); }
      .${prefix}-a { --a: 1px; }
      .${prefix}-b { --b: 2px; }

      /* Variant 2 */
      .${prefix}-v2 { max-width: var(--a, ) var(--b, ); }
      .${prefix}-a { --a: 1px; }
      .${prefix}-b { --b: 2px; }
    `
    const style = document.createElement('style')
    const test1a = document.createElement('div')
    const test1b = document.createElement('div')
    const test2a = document.createElement('div')
    const test2b = document.createElement('div')
    document.head.appendChild(style)
    document.body.appendChild(test1a)
    document.body.appendChild(test1b)
    document.body.appendChild(test2a)
    document.body.appendChild(test2b)
    test1a.className = `${prefix}-v1 ${prefix}-a`
    test1b.className = `${prefix}-v1 ${prefix}-b`
    test2a.className = `${prefix}-v2 ${prefix}-a`
    test2b.className = `${prefix}-v2 ${prefix}-b`
    return [original, css => {
      style.textContent = css
      assertStrictEqual(getComputedStyle(test1a).maxWidth, `1px`)
      assertStrictEqual(getComputedStyle(test1b).maxWidth, `2px`)
      assertStrictEqual(getComputedStyle(test2a).maxWidth, `1px`)
      assertStrictEqual(getComputedStyle(test2b).maxWidth, `2px`)
    }]
  }

  const tests = {
    async transformJS() {
      const { code } = await service.transform('1+2')
      assertStrictEqual(code, '1 + 2;\n')
    },

    async transformTS() {
      const { code } = await service.transform('1 as any + <any>2', { loader: 'ts' })
      assertStrictEqual(code, '1 + 2;\n')
    },

    async transformCSS() {
      const { code } = await service.transform('div { color: red }', { loader: 'css' })
      assertStrictEqual(code, 'div {\n  color: red;\n}\n')
    },

    async problemCSSOriginal() {
      const [original, runAsserts] = setupForProblemCSS('original')
      runAsserts(original)
    },

    async problemCSSPrettyPrinted() {
      const [original, runAsserts] = setupForProblemCSS('pretty-print')
      const { code: prettyPrinted } = await service.transform(original, { loader: 'css' })
      runAsserts(prettyPrinted)
    },

    async problemCSSMinified() {
      const [original, runAsserts] = setupForProblemCSS('pretty-print')
      const { code: minified } = await service.transform(original, { loader: 'css', minify: true })
      runAsserts(minified)
    },

    async buildFib() {
      const fibonacciPlugin = {
        name: 'fib',
        setup(build) {
          build.onResolve({ filter: /^fib\((\d+)\)/ }, args => {
            return { path: args.path, namespace: 'fib' }
          })
          build.onLoad({ filter: /^fib\((\d+)\)/, namespace: 'fib' }, args => {
            let match = /^fib\((\d+)\)/.exec(args.path), n = +match[1]
            let contents = n < 2 ? `export default ${n}` : `
              import n1 from 'fib(${n - 1}) ${args.path}'
              import n2 from 'fib(${n - 2}) ${args.path}'
              export default n1 + n2`
            return { contents }
          })
        },
      }
      const result = await service.build({
        stdin: {
          contents: `
            import x from 'fib(10)'
            return x
          `,
        },
        format: 'cjs',
        bundle: true,
        plugins: [fibonacciPlugin],
      })
      assertStrictEqual(result.outputFiles.length, 1)
      assertStrictEqual(result.outputFiles[0].path, '<stdout>')
      const code = result.outputFiles[0].text
      const fib10 = new Function(code)()
      assertStrictEqual(fib10, 55)
    },

    async buildRelativeIssue693() {
      const result = await service.build({
        stdin: {
          contents: `const x=1`,
        },
        write: false,
        outfile: 'esbuild.js',
      });
      assertStrictEqual(result.outputFiles.length, 1)
      assertStrictEqual(result.outputFiles[0].path, '/esbuild.js')
      assertStrictEqual(result.outputFiles[0].text, 'const x = 1;\n')
    },

    async serve() {
      expectThrownError(service.serve, 'The "serve" API only works in node')
    },

    async esbuildBuild() {
      expectThrownError(esbuild.build, 'The "build" API only works in node')
    },

    async esbuildTransform() {
      expectThrownError(esbuild.transform, 'The "transform" API only works in node')
    },

    async esbuildBuildSync() {
      expectThrownError(esbuild.buildSync, 'The "buildSync" API only works in node')
    },

    async esbuildTransformSync() {
      expectThrownError(esbuild.transformSync, 'The "transformSync" API only works in node')
    },
  }

  function expectThrownError(fn, err) {
    try {
      fn()
      throw new Error('Expected an error to be thrown')
    } catch (e) {
      assertStrictEqual(e.message, err)
    }
  }

  function assertStrictEqual(a, b) {
    if (a !== b) {
      throw new Error(`Assertion failed:
  Observed: ${JSON.stringify(a)}
  Expected: ${JSON.stringify(b)}`);
    }
  }

  async function runTest(test) {
    try {
      await tests[test]()
    } catch (e) {
      testFail(`[${test}] ` + (e && e.message || e))
    }
  }

  const promises = []
  for (const test in tests) {
    promises.push(runTest(test))
  }
  await Promise.all(promises)
}

let pages = {};

for (let format of ['iife', 'esm']) {
  for (let min of [false, true]) {
    for (let async of [false, true]) {
      let code = `
        window.testStart = function() {
          esbuild.startService({
            wasmURL: '/esbuild.wasm',
            worker: ${async},
          }).then(service => {
            return (${runAllTests})({ esbuild, service })
          }).then(() => {
            testDone()
          }).catch(e => {
            testFail('' + (e && e.stack || e))
            testDone()
          })
        }
      `;
      let page;
      if (format === 'esm') {
        page = `
          <script type="module">
            import * as esbuild from '/esm/browser${min ? '.min' : ''}.js'
            ${code}
          </script>
        `;
      } else {
        page = `
          <script src="/lib/browser${min ? '.min' : ''}.js"></script>
          <script>${code}</script>
        `;
      }
      pages[format + (min ? 'Min' : '') + (async ? 'Async' : '')] = page;
    }
  }
}

const server = http.createServer((req, res) => {
  if (req.method === 'GET' && req.url) {
    if (req.url === '/lib/browser.js') {
      res.writeHead(200, { 'Content-Type': 'text/javascript' })
      res.end(js)
      return
    }

    if (req.url === '/lib/browser.min.js') {
      res.writeHead(200, { 'Content-Type': 'text/javascript' })
      res.end(jsMin)
      return
    }

    if (req.url === '/esm/browser.js') {
      res.writeHead(200, { 'Content-Type': 'text/javascript' })
      res.end(esm)
      return
    }

    if (req.url === '/esm/browser.min.js') {
      res.writeHead(200, { 'Content-Type': 'text/javascript' })
      res.end(esmMin)
      return
    }

    if (req.url === '/esbuild.wasm') {
      res.writeHead(200, { 'Content-Type': 'application/wasm' })
      res.end(wasm)
      return
    }

    if (req.url.startsWith('/page/')) {
      let key = req.url.slice('/page/'.length)
      if (Object.prototype.hasOwnProperty.call(pages, key)) {
        res.writeHead(200, { 'Content-Type': 'text/html' })
        res.end(`
          <!doctype html>
          <html>
            <head>
              <meta charset="utf8">
            </head>
            <body>
              ${pages[key]}
            </body>
          </html>
        `)
        return
      }
    }
  }

  console.log(`[http] ${req.method} ${req.url}`)
  res.writeHead(404)
  res.end()
})

server.listen()
const { address, port } = server.address()
const serverURL = url.format({ protocol: 'http', hostname: address, port })
console.log(`[http] listening on ${serverURL}`)

async function main() {
  const browser = await puppeteer.launch()
  const promises = []
  let allTestsPassed = true

  async function runPage(key) {
    try {
      const page = await browser.newPage()
      page.on('console', obj => console.log(`[console.${obj.type()}] ${obj.text()}`))
      page.exposeFunction('testFail', error => {
        console.log(`❌ ${error}`)
        allTestsPassed = false
      })
      let testDone = new Promise(resolve => {
        page.exposeFunction('testDone', resolve)
      })
      await page.goto(`${serverURL}/page/${key}`, { waitUntil: 'domcontentloaded' })
      await page.evaluate('testStart()')
      await testDone
      await page.close()
    } catch (e) {
      allTestsPassed = false
      console.log(`❌ ${key}: ${e && e.message || e}`)
    }
  }

  for (let key in pages) {
    await runPage(key)
  }

  await browser.close()
  server.close()

  if (!allTestsPassed) {
    console.error(`❌ browser test failed`)
    process.exit(1)
  } else {
    console.log(`✅ browser test passed`)
  }
}

main().catch(error => setTimeout(() => { throw error }))
