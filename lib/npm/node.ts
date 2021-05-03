import * as types from "../shared/types";
import * as common from "../shared/common";

import child_process = require('child_process');
import crypto = require('crypto');
import path = require('path');
import fs = require('fs');
import os = require('os');
import tty = require('tty');

declare const ESBUILD_VERSION: string;

// This file is used for both the "esbuild" package and the "esbuild-wasm"
// package. "WASM" will be true for "esbuild-wasm" and false for "esbuild".
declare const WASM: boolean;

let worker_threads: typeof import('worker_threads') | undefined;

// This optimization is opt-in for now because it could break if node has bugs
// with "worker_threads", and node has had such bugs in the past.
//
// TODO: Determine under which conditions this is safe to enable, and then
// replace this check with a check for those conditions.
if (process.env.ESBUILD_WORKER_THREADS) {
  // Don't crash if the "worker_threads" library isn't present
  try {
    worker_threads = require('worker_threads');
  } catch {
  }
}

let esbuildCommandAndArgs = (): [string, string[]] => {
  // This feature was added to give external code a way to modify the binary
  // path without modifying the code itself. Do not remove this because
  // external code relies on this.
  if (process.env.ESBUILD_BINARY_PATH) {
    return [path.resolve(process.env.ESBUILD_BINARY_PATH), []];
  }

  // Try to have a nice error message when people accidentally bundle esbuild
  if (path.basename(__filename) !== 'main.js' || path.basename(__dirname) !== 'lib') {
    throw new Error(
      `The esbuild JavaScript API cannot be bundled. Please mark the "esbuild" ` +
      `package as external so it's not included in the bundle.\n` +
      `\n` +
      `More information: The file containing the code for esbuild's JavaScript ` +
      `API (${__filename}) does not appear to be inside the esbuild package on ` +
      `the file system, which usually means that the esbuild package was bundled ` +
      `into another file. This is problematic because the API needs to run a ` +
      `binary executable inside the esbuild package which is located using a ` +
      `relative path from the API code to the executable. If the esbuild package ` +
      `is bundled, the relative path will be incorrect and the executable won't ` +
      `be found.`);
  }

  if (WASM) {
    return ['node', [path.join(__dirname, '..', 'bin', 'esbuild')]];
  }

  if (process.platform === 'win32') {
    return [path.join(__dirname, '..', 'esbuild.exe'), []];
  }

  // Yarn 2 is deliberately incompatible with binary modules because the
  // developers of Yarn 2 don't think they should be used. See this thread for
  // details: https://github.com/yarnpkg/berry/issues/882.
  //
  // As a compatibility hack we replace the binary with a wrapper script only
  // for Yarn 2. The wrapper script is avoided for other platforms because
  // running the binary directly without going through node first is faster.
  // However, this will make using the JavaScript API with Yarn 2 unnecessarily
  // slow because the wrapper means running the binary will now start another
  // nested node process just to call "spawnSync" and run the actual binary.
  //
  // To work around this workaround, we query for the place the binary is moved
  // to if the original location is replaced by our Yarn 2 compatibility hack.
  // If it exists, we can infer that we are running within Yarn 2 and the
  // JavaScript API should invoke the binary here instead to avoid a slowdown.
  // Calling the binary directly can be over 6x faster than calling the wrapper
  // script instead.
  let pathForYarn2 = path.join(__dirname, '..', 'esbuild');
  if (fs.existsSync(pathForYarn2)) {
    return [pathForYarn2, []];
  }

  return [path.join(__dirname, '..', 'bin', 'esbuild'), []];
};

// Return true if stderr is a TTY
let isTTY = () => tty.isatty(2);

let fsSync: common.StreamFS = {
  readFile(tempFile, callback) {
    try {
      let contents = fs.readFileSync(tempFile, 'utf8');
      try {
        fs.unlinkSync(tempFile);
      } catch {
      }
      callback(null, contents);
    } catch (err) {
      callback(err, null);
    }
  },
  writeFile(contents, callback) {
    try {
      let tempFile = randomFileName();
      fs.writeFileSync(tempFile, contents);
      callback(tempFile);
    } catch {
      callback(null);
    }
  },
};

let fsAsync: common.StreamFS = {
  readFile(tempFile, callback) {
    try {
      fs.readFile(tempFile, 'utf8', (err, contents) => {
        try {
          fs.unlink(tempFile, () => callback(err, contents));
        } catch {
          callback(err, contents);
        }
      });
    } catch (err) {
      callback(err, null);
    }
  },
  writeFile(contents, callback) {
    try {
      let tempFile = randomFileName();
      fs.writeFile(tempFile, contents, err =>
        err !== null ? callback(null) : callback(tempFile));
    } catch {
      callback(null);
    }
  },
};

export let version = ESBUILD_VERSION;

export let build: typeof types.build = (options: types.BuildOptions): Promise<any> =>
  ensureServiceIsRunning().build(options);

export let serve: typeof types.serve = (serveOptions, buildOptions) =>
  ensureServiceIsRunning().serve(serveOptions, buildOptions);

export let transform: typeof types.transform = (input, options) =>
  ensureServiceIsRunning().transform(input, options);

export let formatMessages: typeof types.formatMessages = (messages, options) =>
  ensureServiceIsRunning().formatMessages(messages, options);

export let buildSync: typeof types.buildSync = (options: types.BuildOptions): any => {
  // Try using a long-lived worker thread to avoid repeated start-up overhead
  if (worker_threads) {
    if (!workerThreadService) workerThreadService = startWorkerThreadService(worker_threads);
    return workerThreadService.buildSync(options);
  }

  let result: types.BuildResult;
  runServiceSync(service => service.buildOrServe({
    callName: 'buildSync',
    refs: null,
    serveOptions: null,
    options,
    isTTY: isTTY(),
    defaultWD,
    callback: (err, res) => { if (err) throw err; result = res as types.BuildResult },
  }));
  return result!;
};

export let transformSync: typeof types.transformSync = (input, options) => {
  // Try using a long-lived worker thread to avoid repeated start-up overhead
  if (worker_threads) {
    if (!workerThreadService) workerThreadService = startWorkerThreadService(worker_threads);
    return workerThreadService.transformSync(input, options);
  }

  let result: types.TransformResult;
  runServiceSync(service => service.transform({
    callName: 'transformSync',
    refs: null,
    input,
    options: options || {},
    isTTY: isTTY(),
    fs: fsSync,
    callback: (err, res) => { if (err) throw err; result = res! },
  }));
  return result!;
};

export let formatMessagesSync: typeof types.formatMessagesSync = (messages, options) => {
  // Try using a long-lived worker thread to avoid repeated start-up overhead
  if (worker_threads) {
    if (!workerThreadService) workerThreadService = startWorkerThreadService(worker_threads);
    return workerThreadService.formatMessagesSync(messages, options);
  }

  let result: string[];
  runServiceSync(service => service.formatMessages({
    callName: 'formatMessagesSync',
    refs: null,
    messages,
    options,
    callback: (err, res) => { if (err) throw err; result = res! },
  }));
  return result!;
};

let initializeWasCalled = false;

export let initialize: typeof types.initialize = options => {
  options = common.validateInitializeOptions(options || {});
  if (options.wasmURL) throw new Error(`The "wasmURL" option only works in the browser`)
  if (options.worker) throw new Error(`The "worker" option only works in the browser`)
  if (initializeWasCalled) throw new Error('Cannot call "initialize" more than once')
  ensureServiceIsRunning()
  initializeWasCalled = true
  return Promise.resolve();
}

interface Service {
  build: typeof types.build;
  serve: typeof types.serve;
  transform: typeof types.transform;
  formatMessages: typeof types.formatMessages;
}

let defaultWD = process.cwd();
let longLivedService: Service | undefined;

let ensureServiceIsRunning = (): Service => {
  if (longLivedService) return longLivedService;
  let [command, args] = esbuildCommandAndArgs();
  let child = child_process.spawn(command, args.concat(`--service=${ESBUILD_VERSION}`, '--ping'), {
    windowsHide: true,
    stdio: ['pipe', 'pipe', 'inherit'],
    cwd: defaultWD,
  });

  let { readFromStdout, afterClose, service } = common.createChannel({
    writeToStdin(bytes) {
      child.stdin.write(bytes);
    },
    readFileSync: fs.readFileSync,
    isSync: false,
    isBrowser: false,
  });

  const stdin: typeof child.stdin & { unref?(): void } = child.stdin;
  const stdout: typeof child.stdout & { unref?(): void } = child.stdout;

  stdout.on('data', readFromStdout);
  stdout.on('end', afterClose);

  let refCount = 0;
  child.unref();
  if (stdin.unref) {
    stdin.unref();
  }
  if (stdout.unref) {
    stdout.unref();
  }

  const refs: common.Refs = {
    ref() { if (++refCount === 1) child.ref(); },
    unref() { if (--refCount === 0) child.unref(); },
  }

  longLivedService = {
    build: (options: types.BuildOptions): Promise<any> => {
      return new Promise<types.BuildResult>((resolve, reject) => {
        service.buildOrServe({
          callName: 'build',
          refs,
          serveOptions: null,
          options,
          isTTY: isTTY(),
          defaultWD,
          callback: (err, res) => err ? reject(err) : resolve(res as types.BuildResult),
        })
      })
    },
    serve: (serveOptions, buildOptions) => {
      if (serveOptions === null || typeof serveOptions !== 'object')
        throw new Error('The first argument must be an object')
      return new Promise((resolve, reject) =>
        service.buildOrServe({
          callName: 'serve',
          refs,
          serveOptions,
          options: buildOptions,
          isTTY: isTTY(),
          defaultWD, callback: (err, res) => err ? reject(err) : resolve(res as types.ServeResult),
        }))
    },
    transform: (input, options) => {
      return new Promise((resolve, reject) =>
        service.transform({
          callName: 'transform',
          refs,
          input,
          options: options || {},
          isTTY: isTTY(),
          fs: fsAsync,
          callback: (err, res) => err ? reject(err) : resolve(res!),
        }));
    },
    formatMessages: (messages, options) => {
      return new Promise((resolve, reject) =>
        service.formatMessages({
          callName: 'formatMessages',
          refs,
          messages,
          options,
          callback: (err, res) => err ? reject(err) : resolve(res!),
        }));
    },
  };
  return longLivedService;
}

let runServiceSync = (callback: (service: common.StreamService) => void): void => {
  let [command, args] = esbuildCommandAndArgs();
  let stdin = new Uint8Array();
  let { readFromStdout, afterClose, service } = common.createChannel({
    writeToStdin(bytes) {
      if (stdin.length !== 0) throw new Error('Must run at most one command');
      stdin = bytes;
    },
    isSync: true,
    isBrowser: false,
  });
  callback(service);
  let stdout = child_process.execFileSync(command, args.concat(`--service=${ESBUILD_VERSION}`), {
    cwd: defaultWD,
    windowsHide: true,
    input: stdin,

    // We don't know how large the output could be. If it's too large, the
    // command will fail with ENOBUFS. Reserve 16mb for now since that feels
    // like it should be enough. Also allow overriding this with an environment
    // variable.
    maxBuffer: +process.env.ESBUILD_MAX_BUFFER! || 16 * 1024 * 1024,
  });
  readFromStdout(stdout);
  afterClose();
};

let randomFileName = () => {
  return path.join(os.tmpdir(), `esbuild-${crypto.randomBytes(32).toString('hex')}`);
};

interface MainToWorkerMessage {
  sharedBuffer: SharedArrayBuffer;
  id: number;
  command: string;
  args: any[];
}

interface WorkerThreadService {
  buildSync(options: types.BuildOptions): types.BuildResult;
  transformSync: typeof types.transformSync;
  formatMessagesSync: typeof types.formatMessagesSync;
}

let workerThreadService: WorkerThreadService | null = null;

let startWorkerThreadService = (worker_threads: typeof import('worker_threads')): WorkerThreadService => {
  let { port1: mainPort, port2: workerPort } = new worker_threads.MessageChannel();
  let worker = new worker_threads.Worker(__filename, {
    workerData: { workerPort, defaultWD },
    transferList: [workerPort],

    // From node's documentation: https://nodejs.org/api/worker_threads.html
    //
    //   Take care when launching worker threads from preload scripts (scripts loaded
    //   and run using the `-r` command line flag). Unless the `execArgv` option is
    //   explicitly set, new Worker threads automatically inherit the command line flags
    //   from the running process and will preload the same preload scripts as the main
    //   thread. If the preload script unconditionally launches a worker thread, every
    //   thread spawned will spawn another until the application crashes.
    //
    execArgv: [],
  });
  let nextID = 0;
  let wasStopped = false;

  // This forbids options which would cause structured clone errors
  let fakeBuildError = (text: string) => {
    let error: any = new Error(`Build failed with 1 error:\nerror: ${text}`);
    let errors: types.Message[] = [{ pluginName: '', text, location: null, notes: [], detail: void 0 }];
    error.errors = errors;
    error.warnings = [];
    return error;
  };
  let validateBuildSyncOptions = (options: types.BuildOptions | undefined): void => {
    if (!options) return
    let plugins = options.plugins
    let incremental = options.incremental
    if (plugins && plugins.length > 0) throw fakeBuildError(`Cannot use plugins in synchronous API calls`);
    if (incremental) throw fakeBuildError(`Cannot use "incremental" with a synchronous build`);
  };

  // MessagePort doesn't copy the properties of Error objects. We still want
  // error objects to have extra properties such as "warnings" so implement the
  // property copying manually.
  let applyProperties = (object: any, properties: Record<string, any>): void => {
    for (let key in properties) {
      object[key] = properties[key];
    }
  };

  let runCallSync = (command: string, args: any[]): any => {
    if (wasStopped) throw new Error('The service was stopped');
    let id = nextID++;

    // Make a fresh shared buffer for every request. That way we can't have a
    // race where a notification from the previous call overlaps with this call.
    let sharedBuffer = new SharedArrayBuffer(8);
    let sharedBufferView = new Int32Array(sharedBuffer);

    // Send the message to the worker. Note that the worker could potentially
    // complete the request before this thread returns from this call.
    let msg: MainToWorkerMessage = { sharedBuffer, id, command, args };
    worker.postMessage(msg);

    // If the value hasn't changed (i.e. the request hasn't been completed,
    // wait until the worker thread notifies us that the request is complete).
    //
    // Otherwise, if the value has changed, the request has already been
    // completed. Don't wait in that case because the notification may never
    // arrive if it has already been sent.
    let status = Atomics.wait(sharedBufferView, 0, 0);
    if (status !== 'ok' && status !== 'not-equal') throw new Error('Internal error: Atomics.wait() failed: ' + status);

    let { message: { id: id2, resolve, reject, properties } } = worker_threads!.receiveMessageOnPort(mainPort)!;
    if (id !== id2) throw new Error(`Internal error: Expected id ${id} but got id ${id2}`);
    if (reject) {
      applyProperties(reject, properties);
      throw reject;
    }
    return resolve;
  };

  // Calling unref() on a worker will allow the thread to exit if it's the last
  // only active handle in the event system. This means node will still exit
  // when there are no more event handlers from the main thread. So there's no
  // need to have a "stop()" function.
  worker.unref();

  return {
    buildSync(options) {
      validateBuildSyncOptions(options);
      return runCallSync('build', [options]);
    },
    transformSync(input, options) {
      return runCallSync('transform', [input, options]);
    },
    formatMessagesSync(messages, options) {
      return runCallSync('formatMessages', [messages, options]);
    },
  };
};

let startSyncServiceWorker = () => {
  let workerPort: import('worker_threads').MessagePort = worker_threads!.workerData.workerPort;
  let parentPort = worker_threads!.parentPort!;
  let service = ensureServiceIsRunning();

  // Take the default working directory from the main thread because we want it
  // to be consistent. This will be the working directory that was current at
  // the time the "esbuild" package was first imported.
  defaultWD = worker_threads!.workerData.defaultWD;

  // MessagePort doesn't copy the properties of Error objects. We still want
  // error objects to have extra properties such as "warnings" so implement the
  // property copying manually.
  let extractProperties = (object: any): Record<string, any> => {
    let properties: Record<string, any> = {};
    if (object && typeof object === 'object') {
      for (let key in object) {
        properties[key] = object[key];
      }
    }
    return properties;
  };

  parentPort.on('message', (msg: MainToWorkerMessage) => {
    (async () => {
      let { sharedBuffer, id, command, args } = msg;
      let sharedBufferView = new Int32Array(sharedBuffer);

      try {
        if (command === 'build') {
          workerPort.postMessage({ id, resolve: await service.build(args[0]) });
        } else if (command === 'transform') {
          workerPort.postMessage({ id, resolve: await service.transform(args[0], args[1]) });
        } else if (command === 'formatMessages') {
          workerPort.postMessage({ id, resolve: await service.formatMessages(args[0], args[1]) });
        } else {
          throw new Error(`Invalid command: ${command}`);
        }
      } catch (reject) {
        workerPort.postMessage({ id, reject, properties: extractProperties(reject) });
      }

      // The message has already been posted by this point, so it should be
      // safe to wake the main thread. The main thread should always get the
      // message we sent above.

      // First, change the shared value. That way if the main thread attempts
      // to wait for us after this point, the wait will fail because the shared
      // value has changed.
      Atomics.add(sharedBufferView, 0, 1);

      // Then, wake the main thread. This handles the case where the main
      // thread was already waiting for us before the shared value was changed.
      Atomics.notify(sharedBufferView, 0, Infinity);
    })();
  });
};

// If we're in the worker thread, start the worker code
if (worker_threads && !worker_threads.isMainThread) {
  startSyncServiceWorker();
}
