import * as types from "./types"
import * as common from "./common"

declare const ESBUILD_VERSION: string;
declare let WEB_WORKER_SOURCE_CODE: string

export let version = ESBUILD_VERSION;

export let build: typeof types.build = (options: types.BuildOptions): Promise<any> =>
  ensureServiceIsRunning().build(options);

export const serve: typeof types.serve = () => {
  throw new Error(`The "serve" API only works in node`);
};

export const transform: typeof types.transform = (input, options) =>
  ensureServiceIsRunning().transform(input, options);

export const buildSync: typeof types.buildSync = () => {
  throw new Error(`The "buildSync" API only works in node`);
};

export const transformSync: typeof types.transformSync = () => {
  throw new Error(`The "transformSync" API only works in node`);
};

interface Service {
  build: typeof types.build;
  transform: typeof types.transform;
}

let initializePromise: Promise<void> | undefined;
let longLivedService: Service | undefined;

let ensureServiceIsRunning = (): Service => {
  if (longLivedService) return longLivedService;
  if (initializePromise) throw new Error('You need to wait for the promise returned from "initialize" to be resolved before calling this');
  throw new Error('You need to call "initialize" before calling this');
}

export const initialize: typeof types.initialize = options => {
  options = common.validateInitializeOptions(options || {});
  let wasmURL = options.wasmURL;
  let useWorker = options.worker !== false;
  let verifyWasmURL = options.verifyWasmURL !== false;
  if (!wasmURL) throw new Error('Must provide the "wasmURL" option');
  wasmURL += '';
  if (initializePromise) throw new Error('Cannot call "initialize" more than once');
  initializePromise = startRunningService(wasmURL, useWorker, verifyWasmURL);
  initializePromise.catch(() => {
    // Let the caller try again if this fails
    initializePromise = void 0;
  });
  return initializePromise;
}

const startRunningService = async (wasmURL: string, useWorker: boolean, verifyWasmURL: boolean ): Promise<void> => {
  let wasm: ArrayBuffer | void;
  let url: URL;
  // Per https://webassembly.org/docs/web/#webassemblyinstantiatestreaming, 
  // Automatically use 'instantiateStreaming' when available and:
  // - The Response is CORS-same-origin
  // - The Response represents an ok status
  // - The Response Matches the `application/wasm` MIME type
  if ('instantiateStreaming' in WebAssembly && (url = new URL(wasmURL, location.href), url.origin === location.origin)) {
    // If its a relative URL, it must be made absolute since the href of the worker might be a blob
    wasmURL = url.toString();

    if (verifyWasmURL) {
      const resp = await fetch(wasmURL, {
        method: "HEAD",
        // micro-optimization: try to keep the connection open for longer to reduce the added latency for fetching the WASM
        keepalive: true
      });

      if (!resp.ok) {
        throw new Error(`Failed to download ${JSON.stringify(wasmURL)}`);
      } else if (!resp.headers.get("Content-Type")?.includes("application/wasm")) {
        let res = await fetch(wasmURL);
        if (!res.ok) throw new Error(`Failed to download ${JSON.stringify(wasmURL)}`);
        // Log this after so they don't see two logs.
        console.info(`Make esbuild-wasm load faster by setting the "Content-Type" header to "application/wasm" in ${JSON.stringify(wasmURL)}. Learn more at https://v8.dev/blog/wasm-code-caching#stream.`)
        wasm = await res.arrayBuffer();
      }
    }
  } else {
    let res = await fetch(wasmURL);
    if (!res.ok) throw new Error(`Failed to download ${JSON.stringify(wasmURL)}`);
    wasm = await res.arrayBuffer();
  }

  let code = `{` +
    `let global={ESBUILD_WASM_URL: ${wasm ? '""' : JSON.stringify(wasmURL)}};` +
    `for(let o=self;o;o=Object.getPrototypeOf(o))` +
    `for(let k of Object.getOwnPropertyNames(o))` +
    `if(!(k in global))` +
    `Object.defineProperty(global,k,{get:()=>self[k]});` +
    WEB_WORKER_SOURCE_CODE +
    `}`
  let worker: {
    onmessage: ((event: any) => void) | null
    postMessage: (data: Uint8Array | ArrayBuffer) => void
    terminate: () => void
  }

  if (useWorker) {
    // Run esbuild off the main thread
    let blob = new Blob([code], { type: 'text/javascript' })
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

  if (typeof wasm === 'undefined') {
    worker.postMessage(new ArrayBuffer(0))
  } else {
    worker.postMessage(wasm)
  }

  worker.onmessage = ({ data }) => readFromStdout(data)

  let { readFromStdout, service } = common.createChannel({
    writeToStdin(bytes) {
      worker.postMessage(bytes)
    },
    isSync: false,
    isBrowser: true,
  })

  longLivedService = {
    build: (options: types.BuildOptions): Promise<any> =>
      new Promise<types.BuildResult>((resolve, reject) =>
        service.buildOrServe('build', null, null, options, false, '/', (err, res) =>
          err ? reject(err) : resolve(res as types.BuildResult))),
    transform: (input, options) => {
      return new Promise((resolve, reject) =>
        service.transform('transform', null, input, options || {}, false, {
          readFile(_, callback) { callback(new Error('Internal error'), null); },
          writeFile(_, callback) { callback(null); },
        }, (err, res) => err ? reject(err) : resolve(res!)))
    },
  }
}
