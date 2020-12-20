import * as types from "./types";
import * as common from "./common";

import child_process = require('child_process');
import crypto = require('crypto');
import path = require('path');
import fs = require('fs');
import os = require('os');
import tty = require('tty');

let worker_threads: typeof import('worker_threads') | undefined;

// Don't crash if the "worker_threads" library isn't present
try {
  worker_threads = require('worker_threads');
} catch {
}

declare const ESBUILD_VERSION: string;

// This file is used for both the "esbuild" package and the "esbuild-wasm"
// package. "WASM" will be true for "esbuild-wasm" and false for "esbuild".
declare const WASM: boolean;

let esbuildCommandAndArgs = (): [string, string[]] => {
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
  // This is a performance improvement of about 0.1 seconds for Yarn 2 on my
  // machine.
  let pathForYarn2 = path.join(__dirname, 'esbuild');
  if (fs.existsSync(pathForYarn2)) {
    return [pathForYarn2, []];
  }

  return [path.join(__dirname, '..', 'bin', 'esbuild'), []];
};

// Return true if stderr is a TTY
let isTTY = () => tty.isatty(2);

export let version = ESBUILD_VERSION;

export let build: typeof types.build = (options: types.BuildOptions): Promise<any> => {
  return startService().then<types.BuildResult>(service => {
    return service.build(options).then(result => {
      if (result.rebuild) {
        let old = result.rebuild.dispose;
        result.rebuild.dispose = () => {
          old();
          service.stop();
        };
      }
      else service.stop();
      return result;
    }, error => {
      service.stop();
      throw error;
    });
  });
};

export let serve: typeof types.serve = (serveOptions, buildOptions) => {
  return startService().then(service => {
    return service.serve(serveOptions, buildOptions).then(result => {
      result.wait.then(service.stop, service.stop);
      return result;
    }, error => {
      service.stop();
      throw error;
    });
  });
};

export let transform: typeof types.transform = (input, options) => {
  input += '';
  return startService().then(service => {
    let promise = service.transform(input, options);
    promise.then(service.stop, service.stop);
    return promise;
  });
};

export let buildSync: typeof types.buildSync = (options: types.BuildOptions): any => {
  // Try using a long-lived worker thread to avoid repeated start-up overhead
  if (worker_threads) {
    if (!workerThreadService) workerThreadService = startWorkerThreadService(worker_threads);
    return workerThreadService.buildSync(options);
  }

  // Otherwise, fall back to running a dedicated child process
  let result: types.BuildResult;
  runServiceSync(service => service.buildOrServe('buildSync', null, options, isTTY(), (err, res) => {
    if (err) throw err;
    result = res as types.BuildResult;
  }));
  return result!;
};

export let transformSync: typeof types.transformSync = (input, options) => {
  input += '';

  // Try using a long-lived worker thread to avoid repeated start-up overhead
  if (worker_threads) {
    if (!workerThreadService) workerThreadService = startWorkerThreadService(worker_threads);
    return workerThreadService.transformSync(input, options);
  }

  // Otherwise, fall back to running a dedicated child process
  let result: types.TransformResult;
  runServiceSync(service => service.transform('transformSync', input, options || {}, isTTY(), {
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
  }, (err, res) => {
    if (err) throw err;
    result = res!;
  }));
  return result!;
};

export let startService: typeof types.startService = common.referenceCountedService(options => {
  options = common.validateServiceOptions(options || {});
  if (options.wasmURL) throw new Error(`The "wasmURL" option only works in the browser`)
  if (options.worker) throw new Error(`The "worker" option only works in the browser`)
  let [command, args] = esbuildCommandAndArgs();
  let child = child_process.spawn(command, args.concat(`--service=${ESBUILD_VERSION}`), {
    cwd: process.cwd(),
    windowsHide: true,
    stdio: ['pipe', 'pipe', 'inherit'],
  });
  let { readFromStdout, afterClose, service } = common.createChannel({
    writeToStdin(bytes) {
      child.stdin.write(bytes);
    },
    readFileSync: fs.readFileSync,
    isSync: false,
    isBrowser: false,
  });
  child.stdout.on('data', readFromStdout);
  child.stdout.on('end', afterClose);

  // Create an asynchronous Promise-based API
  return Promise.resolve({
    build: (options: types.BuildOptions): Promise<any> =>
      new Promise<types.BuildResult>((resolve, reject) =>
        service.buildOrServe('build', null, options, isTTY(), (err, res) =>
          err ? reject(err) : resolve(res as types.BuildResult))),
    serve: (serveOptions, buildOptions) => {
      if (serveOptions === null || typeof serveOptions !== 'object')
        throw new Error('The first argument must be an object')
      return new Promise((resolve, reject) =>
        service.buildOrServe('serve', serveOptions, buildOptions, isTTY(), (err, res) =>
          err ? reject(err) : resolve(res as types.ServeResult)))
    },
    transform: (input, options) => {
      input += '';
      return new Promise((resolve, reject) =>
        service.transform('transform', input, options || {}, isTTY(), {
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
        }, (err, res) => err ? reject(err) : resolve(res!)));
    },
    stop() { child.kill(); },
  });
});

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
    cwd: process.cwd(),
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
  transformSync(input: string, options?: types.TransformOptions): types.TransformResult;
}

let workerThreadService: WorkerThreadService | null = null;

let startWorkerThreadService = (worker_threads: typeof import('worker_threads')): WorkerThreadService => {
  let { port1: mainPort, port2: workerPort } = new worker_threads.MessageChannel();
  let { port1: mainWatchdogPort, port2: watchdogPort } = new worker_threads.MessageChannel();
  const workerCode = `
    start();
    function start() {
      const {workerData} = require('worker_threads');
      if(workerData) {
        require(workerData.mainPath).startSyncServiceWorker();
      }
      else setImmediate(start);
    }
  `;
  const watchdogCode = `
    start();
    function start() {
      const {workerData, Worker} = require('worker_threads');
      if(!workerData) return setImmediate(start);
      const {watchdogPort, workerPort, mainPath, workerCode, env, execArgv} = workerData;

      let failed = false;
      const buffers = new Map();
      watchdogPort.on('message', message => {
        const {type, id, sharedBuffer} = message;
        switch(type) {
          case 'start':
            buffers.set(id, sharedBuffer);
            if(failed) notifyFailure();
            break;
          case 'stop':
            buffers.delete(id);
          break;
          default:
            notifyFailure();
        }
      });

      const worker = new Worker(workerCode, {
        eval: true,
        workerData: {
          workerPort,
          mainPath
        },
        transferList: [workerPort],
        execArgv,
        env
      });
      worker.on('exit', notifyFailure);
      process.on('exit', notifyFailure);
      function notifyFailure() {
        failed = true;
        for(const [id, sharedBuffer] of buffers.entries()) {
          const sharedBufferView = new Int32Array(sharedBuffer);
          Atomics.add(sharedBufferView, 0, 4);
          Atomics.notify(sharedBufferView, 0, Infinity);
          buffers.delete(id);
        }
      }
      worker.unref();
    }
  `
  let worker = new worker_threads.Worker(watchdogCode, {
    eval: true,
    execArgv: [],
    env: {},
    workerData: {
      workerCode,
      watchdogPort,
      workerPort,
      mainPath: __filename,
      execArgv: process.execArgv,
      env: {...process.env}
    },
    transferList: [workerPort, watchdogPort],
  });
  let nextID = 0;
  let wasStopped = false;

  // This forbids options which would cause structured clone errors
  let validateBuildSyncOptions = (options: types.BuildOptions | undefined): void => {
    if (!options) return
    let plugins = options.plugins
    let incremental = options.incremental
    if (plugins && plugins.length > 0) throw new Error(`Cannot use plugins in synchronous API calls`);
    if (incremental) throw new Error(`Cannot use "incremental" with a synchronous build`);
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

    // Notify the watchdog that we are blocked.  It will wake us up if
    // the worker_thread dies for any reason, to avoid us hanging.
    const watchdogId = `${ worker_threads!.threadId }-${ id }`;
    mainWatchdogPort.postMessage({
        type: 'start',
        id: watchdogId,
        sharedBuffer
    });
    // Send the message to the worker. Note that the worker could potentially
    // complete the request before this thread returns from this call.
    let msg: MainToWorkerMessage = { sharedBuffer, id, command, args };
    mainPort.postMessage(msg);

    // If the value hasn't changed (i.e. the request hasn't been completed,
    // wait until the worker thread notifies us that the request is complete).
    //
    // Otherwise, if the value has changed, the request has already been
    // completed. Don't wait in that case because the notification may never
    // arrive if it has already been sent.
    let status = Atomics.wait(sharedBufferView, 0, 0);
    if (status !== 'ok' && status !== 'not-equal') throw new Error('Internal error: Atomics.wait() failed: ' + status);
    if (sharedBufferView[0] > 1) throw new Error('Internal error: worker_thread terminated');

    watchdogPort.postMessage({
        type: 'stop',
        id: watchdogId
    });

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
  };
};

export let startSyncServiceWorker = () => {
  let workerData = worker_threads!.workerData;
  if(!workerData) {
    setImmediate(startSyncServiceWorker);
  }
  let workerPort: import('worker_threads').MessagePort = workerData.workerPort;
  let notifyWorkerFailedCallbacks = new Set<() => void>();
  // Catch unexpected worker failures and notify all blocked threads to avoid hangs.
  // TODO Also bind unhandledRejection?
  process.on('uncaughtException', (err: any, origin: any) => {
    for(const notifyWorkerFailedCallback of notifyWorkerFailedCallbacks) {
      try {
        notifyWorkerFailedCallback();
      } catch {}
    }
    throw err;
  });
  let servicePromise = startService();

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

  workerPort.on('message', onMessage);
  // Because of potential setImmediate delay above, there may already be messages to receive.
  while(true) {
    const msgWrapper = worker_threads!.receiveMessageOnPort(workerPort);
    if(msgWrapper) onMessage(msgWrapper.message as MainToWorkerMessage);
    else break;
  }
  function onMessage(msg: MainToWorkerMessage) {
    (async () => {
      let { sharedBuffer, id, command, args } = msg;
      let sharedBufferView = new Int32Array(sharedBuffer);
      function onWorkerFailed() {
        Atomics.add(sharedBufferView, 0, 2);
        Atomics.notify(sharedBufferView, 0, Infinity);
      }
      notifyWorkerFailedCallbacks.add(onWorkerFailed);

      try {
        const service = await servicePromise;
        if (command === 'build') {
          workerPort.postMessage({ id, resolve: await service.build(args[0]) });
        } else if (command === 'transform') {
          workerPort.postMessage({ id, resolve: await service.transform(args[0], args[1]) });
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

      notifyWorkerFailedCallbacks.delete(onWorkerFailed);
    })();
  }
};
