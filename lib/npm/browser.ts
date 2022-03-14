import * as types from "../shared/types"
import * as common from "../shared/common"
import * as ourselves from "./browser"

declare const ESBUILD_VERSION: string;
declare let WEB_WORKER_SOURCE_CODE: string
declare let WEB_WORKER_FUNCTION: (postMessage: (data: Uint8Array) => void) => (event: { data: Uint8Array | ArrayBuffer }) => void

export let version = ESBUILD_VERSION;

export let build: typeof types.build = (options: types.BuildOptions): Promise<any> =>
  ensureServiceIsRunning().build(options);

export const serve: typeof types.serve = () => {
  throw new Error(`The "serve" API only works in node`);
};

export const transform: typeof types.transform = (input, options) =>
  ensureServiceIsRunning().transform(input, options);

export const formatMessages: typeof types.formatMessages = (messages, options) =>
  ensureServiceIsRunning().formatMessages(messages, options);

export const analyzeMetafile: typeof types.analyzeMetafile = (metafile, options) =>
  ensureServiceIsRunning().analyzeMetafile(metafile, options);

export const buildSync: typeof types.buildSync = () => {
  throw new Error(`The "buildSync" API only works in node`);
};

export const transformSync: typeof types.transformSync = () => {
  throw new Error(`The "transformSync" API only works in node`);
};

export const formatMessagesSync: typeof types.formatMessagesSync = () => {
  throw new Error(`The "formatMessagesSync" API only works in node`);
};

export const analyzeMetafileSync: typeof types.analyzeMetafileSync = () => {
  throw new Error(`The "analyzeMetafileSync" API only works in node`);
};

interface Service {
  build: typeof types.build;
  transform: typeof types.transform;
  formatMessages: typeof types.formatMessages;
  analyzeMetafile: typeof types.analyzeMetafile;
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
  if (!wasmURL) throw new Error('Must provide the "wasmURL" option');
  wasmURL += '';
  if (initializePromise) throw new Error('Cannot call "initialize" more than once');
  initializePromise = startRunningService(wasmURL, useWorker);
  initializePromise.catch(() => {
    // Let the caller try again if this fails
    initializePromise = void 0;
  });
  return initializePromise;
}

const startRunningService = async (wasmURL: string, useWorker: boolean): Promise<void> => {
  let res = await fetch(wasmURL);
  if (!res.ok) throw new Error(`Failed to download ${JSON.stringify(wasmURL)}`);
  let wasm = await res.arrayBuffer();
  let worker: {
    onmessage: ((event: any) => void) | null
    postMessage: (data: Uint8Array | ArrayBuffer) => void
    terminate: () => void
  }

  if (useWorker) {
    // Run esbuild off the main thread
    let blob = new Blob([`onmessage=${WEB_WORKER_SOURCE_CODE}(postMessage)`], { type: 'text/javascript' })
    worker = new Worker(URL.createObjectURL(blob))
  } else {
    // Run esbuild on the main thread
    let onmessage = WEB_WORKER_FUNCTION((data: Uint8Array) => worker.onmessage!({ data }))
    worker = {
      onmessage: null,
      postMessage: data => onmessage({ data }),
      terminate() {
      },
    }
  }

  worker.postMessage(wasm)
  worker.onmessage = ({ data }) => readFromStdout(data)

  let { readFromStdout, service } = common.createChannel({
    writeToStdin(bytes) {
      worker.postMessage(bytes)
    },
    isSync: false,
    isBrowser: true,
    esbuild: ourselves,
  })

  longLivedService = {
    build: (options: types.BuildOptions): Promise<any> =>
      new Promise<types.BuildResult>((resolve, reject) =>
        service.buildOrServe({
          callName: 'build',
          refs: null,
          serveOptions: null,
          options,
          isTTY: false,
          defaultWD: '/',
          callback: (err, res) => err ? reject(err) : resolve(res as types.BuildResult),
        })),
    transform: (input, options) =>
      new Promise((resolve, reject) =>
        service.transform({
          callName: 'transform',
          refs: null,
          input,
          options: options || {},
          isTTY: false,
          fs: {
            readFile(_, callback) { callback(new Error('Internal error'), null); },
            writeFile(_, callback) { callback(null); },
          },
          callback: (err, res) => err ? reject(err) : resolve(res!),
        })),
    formatMessages: (messages, options) =>
      new Promise((resolve, reject) =>
        service.formatMessages({
          callName: 'formatMessages',
          refs: null,
          messages,
          options,
          callback: (err, res) => err ? reject(err) : resolve(res!),
        })),
    analyzeMetafile: (metafile, options) =>
      new Promise((resolve, reject) =>
        service.analyzeMetafile({
          callName: 'analyzeMetafile',
          refs: null,
          metafile: typeof metafile === 'string' ? metafile : JSON.stringify(metafile),
          options,
          callback: (err, res) => err ? reject(err) : resolve(res!),
        })),
  }
}

// Export this module's exports as an export named "default" to try to work
// around problems due to the "default" import mess. See the comment in
// "node.ts" for more detail.
export default ourselves
