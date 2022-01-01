// This file is part of the web worker source code

declare const ESBUILD_VERSION: string;

async function loadWasm(
  url: string,
  importObject?: WebAssembly.Imports
): Promise<WebAssembly.WebAssemblyInstantiatedSource> {
  const response = await fetch(url)
  if (!response.ok)
    throw new Error(`Failed to download ${JSON.stringify(url)}`)

  const mimeType = response.headers.get("Content-Type") ?? ""; // case-insensitive
  // wasm mime type header is required for instantiateStreaming api
  // https://www.w3.org/TR/wasm-web-api-1/#streaming-modules
  const isWasmMime = mimeType.includes("application/wasm")
  if (typeof WebAssembly.instantiateStreaming === "function" && isWasmMime) {
    return WebAssembly.instantiateStreaming(response, importObject)
  } else {
    return WebAssembly.instantiate(await response.arrayBuffer(), importObject)
  }
}

let decoder = new TextDecoder()
let fs = (global as any).fs

let stderr = ''
fs.writeSync = (fd: number, buffer: Uint8Array) => {
  if (fd === 1) {
    postMessage(buffer)
  } else if (fd === 2) {
    stderr += decoder.decode(buffer)
    let parts = stderr.split('\n')
    if (parts.length > 1) console.log(parts.slice(0, -1).join('\n'))
    stderr = parts[parts.length - 1]
  } else {
    throw new Error('Bad write')
  }
  return buffer.length
}

let stdin: Uint8Array[] = []
let resumeStdin: () => void
let stdinPos = 0

onmessage = ({ data }) => {
  if (data.length > 0) {
    stdin.push(data)
    if (resumeStdin) resumeStdin()
  }
}

fs.read = (
  fd: number, buffer: Uint8Array, offset: number, length: number,
  position: null, callback: (err: Error | null, count?: number) => void,
) => {
  if (fd !== 0 || offset !== 0 || length !== buffer.length || position !== null) {
    throw new Error('Bad read')
  }

  if (stdin.length === 0) {
    resumeStdin = () => fs.read(fd, buffer, offset, length, position, callback)
    return
  }

  let first = stdin[0]
  let count = Math.max(0, Math.min(length, first.length - stdinPos))
  buffer.set(first.subarray(stdinPos, stdinPos + count), offset)
  stdinPos += count
  if (stdinPos === first.length) {
    stdin.shift()
    stdinPos = 0
  }
  callback(null, count)
}

let go = new (global as any).Go()
go.argv = ['', `--service=${ESBUILD_VERSION}`]

loadWasm((global as any).WASM_URL, go.importObject)
  .then(async ({ instance }) => {
    postMessage({ type: 'done' })
    return go.run(instance)
  })
  .catch(err => postMessage({ type: 'error', error: err?.message ?? err }))
