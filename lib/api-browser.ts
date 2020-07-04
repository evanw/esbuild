import * as types from "./api-types"
import * as common from "./api-common"

export interface BrowserOptions {
  wasmURL: string
  worker?: boolean
}

declare let WEB_WORKER_SOURCE_CODE: string

export let startService = (options: BrowserOptions): Promise<types.Service> => {
  return fetch(options.wasmURL).then(r => r.arrayBuffer()).then(wasm => {
    let code = `{` +
      `let global={};` +
      `for(let o=self;o;o=Object.getPrototypeOf(o))` +
      `for(let k of Object.getOwnPropertyNames(o))` +
      `global[k]=self[k];` +
      WEB_WORKER_SOURCE_CODE +
      `}`
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
