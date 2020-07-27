import * as types from "./types"
import * as common from "./common"

declare let WEB_WORKER_SOURCE_CODE: string

let build: typeof types.build = options => {
  throw new Error(`The "build" API only works in node`);
};

let transform: typeof types.transform = (input, options) => {
  throw new Error(`The "transform" API only works in node`);
};

let buildSync: typeof types.buildSync = options => {
  throw new Error(`The "buildSync" API only works in node`);
};

let transformSync: typeof types.transformSync = (input, options) => {
  throw new Error(`The "transformSync" API only works in node`);
};

let startService: typeof types.startService = options => {
  if (!options) throw new Error('Must provide an options object to "startService"');
  if (!options.wasmURL) throw new Error('Must provide the "wasmURL" option');
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
        throw new Error(`The "build" API only works in node`)
      },
      transform: (input, options) =>
        new Promise((resolve, reject) =>
          service.transform(input, options || {}, false, {
            readFile(_, callback) { callback(new Error('Internal error'), null); },
            writeFile(_, callback) { callback(null); },
          }, (err, res) => err ? reject(err) : resolve(res!))),
      stop() {
        worker.terminate()
        afterClose()
      },
    }
  })
}

let api: typeof types = {
  build,
  buildSync,
  transform,
  transformSync,
  startService,
};

module.exports = api;
