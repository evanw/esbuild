const http = require('http')
const path = require('path')
const url = require('url')
const fs = require('fs')

const js = fs.readFileSync(path.join(__dirname, '..', '..', 'npm', 'esbuild-wasm', 'lib', 'browser.js'))
const jsMin = fs.readFileSync(path.join(__dirname, '..', '..', 'npm', 'esbuild-wasm', 'lib', 'browser.min.js'))
const esm = fs.readFileSync(path.join(__dirname, '..', '..', 'npm', 'esbuild-wasm', 'esm', 'browser.js'))
const esmMin = fs.readFileSync(path.join(__dirname, '..', '..', 'npm', 'esbuild-wasm', 'esm', 'browser.min.js'))
const wasm = fs.readFileSync(path.join(__dirname, '..', '..', 'npm', 'esbuild-wasm', 'esbuild.wasm'))
const html = fs.readFileSync(path.join(__dirname, '..', '..', 'scripts', 'browser', 'index.html'))

const server = http.createServer((req, res) => {
  console.log(`[http] ${req.method} ${req.url}`)

  try {
    if (req.method === 'GET' && req.url) {
      const parsed = url.parse(req.url)

      if (parsed.pathname === '/npm/esbuild-wasm/lib/browser.js') {
        res.writeHead(200, { 'Content-Type': 'text/javascript' })
        res.end(js)
        return
      }

      if (parsed.pathname === '/npm/esbuild-wasm/lib/browser.min.js') {
        res.writeHead(200, { 'Content-Type': 'text/javascript' })
        res.end(jsMin)
        return
      }

      if (parsed.pathname === '/npm/esbuild-wasm/esm/browser.js') {
        res.writeHead(200, { 'Content-Type': 'text/javascript' })
        res.end(esm)
        return
      }

      if (parsed.pathname === '/npm/esbuild-wasm/esm/browser.min.js') {
        res.writeHead(200, { 'Content-Type': 'text/javascript' })
        res.end(esmMin)
        return
      }

      if (parsed.pathname === '/npm/esbuild-wasm/esbuild.wasm') {
        res.writeHead(200, { 'Content-Type': 'application/wasm' })
        res.end(wasm)
        return
      }

      if (parsed.pathname === '/scripts/browser/index.html') {
        res.writeHead(200, { 'Content-Type': 'text/html' })
        res.end(html)
        return
      }
    }

    res.writeHead(404)
    res.end('404 Not Found')
  }

  catch (err) {
    res.writeHead(500)
    res.end('500 Internal Server Error')
    console.error(err)
  }
})

server.listen()
const { address, port } = server.address()
const serverURL = url.format({ protocol: 'http', hostname: address, port })
console.log(`[http] listening on ${serverURL}`)

async function main() {
  let allTestsPassed = true
  try {
    const browser = await require('puppeteer').launch()

    const page = await browser.newPage()
    page.on('console', obj => {
      console.log(`[console.${obj.type()}] ${obj.text()}`)
    })

    page.exposeFunction('testBegin', args => {
      const { esm, min, worker, url } = JSON.parse(args)
      console.log(`💬 config: esm=${esm}, min=${min}, worker=${worker}, url=${url}`)
    })

    page.exposeFunction('testEnd', args => {
      if (args === null) console.log(`👍 success`)
      else {
        const { test, stack, error } = JSON.parse(args)
        console.log(`❌ error${test ? ` [${test}]` : ``}: ${error}`)
        allTestsPassed = false
      }
    })

    const testDone = new Promise(resolve => {
      page.exposeFunction('testDone', resolve)
    })

    await page.goto(`${serverURL}/scripts/browser/index.html`, { waitUntil: 'domcontentloaded' })
    await testDone
    await page.close()
    await browser.close()
  }

  catch (e) {
    allTestsPassed = false
    console.log(`❌ error: ${e && e.stack || e && e.message || e}`)
  }

  server.close()

  if (!allTestsPassed) {
    console.error(`❌ browser test failed`)
    process.exit(1)
  } else {
    console.log(`✅ browser test passed`)
  }
}

main().catch(error => setTimeout(() => { throw error }))
