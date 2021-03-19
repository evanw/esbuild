import * as types from "./types";
import * as common from "./common";

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

let esbuildCommandAndArgs = (): [string, string[]] => {
  // This feature was added to give external code a way to modify the binary
  // path without modifying the code itself. Do not remove this because
  // external code relies on this.
  if (process.env.ESBUILD_BINARY_PATH) {
    return [path.resolve(process.env.ESBUILD_BINARY_PATH), []];
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

export let version = ESBUILD_VERSION;

export let build: typeof types.build = (options: types.BuildOptions): Promise<any> =>
  ensureServiceIsRunning().build(options);

export let serve: typeof types.serve = (serveOptions, buildOptions) =>
  ensureServiceIsRunning().serve(serveOptions, buildOptions);

export let transform: typeof types.transform = (input, options) =>
  ensureServiceIsRunning().transform(input, options);

export let buildSync: typeof types.buildSync = (options: types.BuildOptions): any => {
  let result: types.BuildResult;
  runServiceSync(service => service.buildOrServe('buildSync', null, null, options, isTTY(), process.cwd(), (err, res) => {
    if (err) throw err;
    result = res as types.BuildResult;
  }));
  return result!;
};

export let transformSync: typeof types.transformSync = (input, options) => {
  let result: types.TransformResult;
  runServiceSync(service => service.transform('transformSync', null, input, options || {}, isTTY(), {
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
}

let defaultWD = process.cwd();
let longLivedService: Service | undefined;

let ensureServiceIsRunning = (): Service => {
  if (longLivedService) return longLivedService;
  let [command, args] = esbuildCommandAndArgs();
  let child = child_process.spawn(command, args.concat(`--service=${ESBUILD_VERSION}`, '--ping'), {
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
        service.buildOrServe('build', refs, null, options, isTTY(), defaultWD, (err, res) => {
          if (err) {
            reject(err)
          } else {
            resolve(res as types.BuildResult);
          }
        })
      })
    },
    serve: (serveOptions, buildOptions) => {
      if (serveOptions === null || typeof serveOptions !== 'object')
        throw new Error('The first argument must be an object')
      return new Promise((resolve, reject) =>
        service.buildOrServe('serve', refs, serveOptions, buildOptions, isTTY(), defaultWD, (err, res) => {
          if (err) {
            reject(err);
          } else {
            resolve(res as types.ServeResult);
          }
        }))
    },
    transform: (input, options) => {
      return new Promise((resolve, reject) =>
        service.transform('transform', refs, input, options || {}, isTTY(), {
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
