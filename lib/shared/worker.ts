// This file is part of the web worker source code

interface Go {
  argv: string[]
  importObject: WebAssembly.Imports
  run(instance: WebAssembly.Instance): void
}

declare const ESBUILD_VERSION: string;
declare function postMessage(message: any): void;

onmessage = ({ data: wasm }: { data: WebAssembly.Module | string }) => {
  let decoder = new TextDecoder()
  let fs = (globalThis as any).fs

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

  let go: Go = new (globalThis as any).Go()
  go.argv = ['', `--service=${ESBUILD_VERSION}`]

  // Try to instantiate the module in the worker, then report back to the main thread
  tryToInstantiateModule(wasm, go).then(
    instance => {
      postMessage(null)
      go.run(instance)
    },
    error => {
      postMessage(error)
    },
  )
}

async function tryToInstantiateModule(wasm: WebAssembly.Module | string, go: Go): Promise<WebAssembly.Instance> {
  if (wasm instanceof WebAssembly.Module) {
    return WebAssembly.instantiate(wasm, go.importObject)
  }

  const res = await fetch(wasm)
  if (!res.ok) throw new Error(`Failed to download ${JSON.stringify(wasm)}`);

  // Attempt to use the superior "instantiateStreaming" API first
  if ('instantiateStreaming' in WebAssembly && /^application\/wasm($|;)/i.test(res.headers.get('Content-Type') || '')) {
    const result = await WebAssembly.instantiateStreaming(res, go.importObject)
    return result.instance
  }

  // Otherwise, fall back to the inferior "instantiate" API
  const bytes = await res.arrayBuffer()
  const result = await WebAssembly.instantiate(bytes, go.importObject)
  return result.instance
}
