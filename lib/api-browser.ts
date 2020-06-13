import * as types from "./api-types"
import * as common from "./api-common"

export interface BrowserOptions {
  wasmURL: string
  worker?: boolean
}

declare let Go: any
declare let postMessage: any
declare let WASM_EXEC_JS: string

// This function is never called directly. Instead we call toString() on it
// and use it to construct the source code for the worker.
let workerThread = () => {
  onmessage = ({ data: wasm }) => {
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
    go.argv = ['', '--service']

    WebAssembly.instantiate(wasm, go.importObject)
      .then(({ instance }) => go.run(instance))
  }
}

export let startService = (options: BrowserOptions): Promise<types.Service> => {
  return fetch(options.wasmURL).then(r => r.arrayBuffer()).then(wasm => {
    let cloneGlobal = () => {
      // Clone the global object to prevent the Go wrapper from polluting the actual global object
      for (let obj: any = self; obj; obj = Object.getPrototypeOf(obj))
        for (let key of Object.getOwnPropertyNames(obj))
          (global as any)[key] = (self as any)[key]
    };
    let code = `{let global={};(${cloneGlobal})();${WASM_EXEC_JS};(${workerThread})();}`
    let worker: {
      onmessage: ((event: any) => void) | null
      postMessage: (data: Uint8Array | ArrayBuffer) => void
      terminate: () => void
    }

    if (options.worker !== false) {
      // Run esbuild off the main thread
      let blob = new Blob([code], { type: 'application/javascript' })
      worker = new Worker(URL.createObjectURL(blob))
    } else {
      // Run esbuild on the main thread
      let fn = new Function('postMessage', code + `var onmessage; return m => onmessage(m)`)
      let onmessage = fn((data: Uint8Array) => worker.onmessage!({ data }))
      worker = {
        onmessage: null,
        postMessage: data => onmessage({ data }),
        terminate() {
        },
      }
    }

    worker.postMessage(wasm)
    worker.onmessage = ({ data }) => readFromStdout(data)

    let { readFromStdout, afterClose, service } = common.createChannel({
      writeToStdin(bytes) {
        worker.postMessage(bytes)
      },
    })

    return {
      build(options) {
        throw new Error('Not implemented yet')
      },
      transform: (input, options) =>
        new Promise((resolve, reject) =>
          service.transform(input, options, false, (err, res) =>
            err ? reject(err) : resolve(res!))),
      stop() {
        worker.terminate()
        afterClose()
      },
    }
  })
}
