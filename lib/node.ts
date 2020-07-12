import * as types from "./types";
import * as common from "./common";
import * as child_process from "child_process";
import * as path from "path";
import { isatty } from "tty";

// This file is used for both the "esbuild" package and the "esbuild-wasm"
// package. "WASM" will be true for "esbuild-wasm" and false for "esbuild".
declare let WASM: boolean;

let esbuildCommandAndArgs = (): [string, string[]] => {
  if (WASM) {
    return ['node', [path.join(__dirname, '..', 'bin', 'esbuild')]];
  }

  if (process.platform === 'win32') {
    return [path.join(__dirname, '..', 'esbuild.exe'), []];
  }

  return [path.join(__dirname, '..', 'bin', 'esbuild'), []];
};

// Return true if stderr is a TTY
let isTTY = () => isatty(2);

let build: typeof types.build = options => {
  return startService().then(service => {
    let promise = service.build(options);
    promise.then(service.stop, service.stop); // Kill the service afterwards
    return promise;
  });
};

let transform: typeof types.transform = (input, options) => {
  return startService().then(service => {
    let promise = service.transform(input, options);
    promise.then(service.stop, service.stop); // Kill the service afterwards
    return promise;
  });
};

let buildSync: typeof types.buildSync = options => {
  let result: types.BuildResult;
  runServiceSync(service => service.build(options, isTTY(), (err, res) => {
    if (err) throw err;
    result = res!;
  }));
  return result!;
};

let transformSync: typeof types.transformSync = (input, options) => {
  let result: types.TransformResult;
  runServiceSync(service => service.transform(input, options, isTTY(), (err, res) => {
    if (err) throw err;
    result = res!;
  }));
  return result!;
};

let startService: typeof types.startService = options => {
  if (options) {
    if (options.wasmURL) throw new Error(`The "wasmURL" option only works in the browser`)
    if (options.worker) throw new Error(`The "worker" option only works in the browser`)
  }
  let [command, args] = esbuildCommandAndArgs();
  let child = child_process.spawn(command, args.concat('--service'), {
    cwd: process.cwd(),
    windowsHide: true,
    stdio: ['pipe', 'pipe', 'inherit'],
  });
  let { readFromStdout, afterClose, service } = common.createChannel({
    writeToStdin(bytes) {
      child.stdin.write(bytes);
    },
  });
  child.stdout.on('data', readFromStdout);
  child.stdout.on('end', afterClose);

  // Create an asynchronous Promise-based API
  return Promise.resolve({
    build: options =>
      new Promise((resolve, reject) =>
        service.build(options, isTTY(), (err, res) =>
          err ? reject(err) : resolve(res!))),
    transform: (input, options) =>
      new Promise((resolve, reject) =>
        service.transform(input, options, isTTY(), (err, res) =>
          err ? reject(err) : resolve(res!))),
    stop() { child.kill(); },
  });
};

let runServiceSync = (callback: (service: common.StreamService) => void): void => {
  let [command, args] = esbuildCommandAndArgs();
  let stdin = new Uint8Array();
  let { readFromStdout, afterClose, service } = common.createChannel({
    writeToStdin(bytes) {
      if (stdin.length !== 0) throw new Error('Must run at most one command');
      stdin = bytes;
    },
  });
  callback(service);
  let stdout = child_process.execFileSync(command, args.concat('--service'), {
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

let api: typeof types = {
  build,
  buildSync,
  transform,
  transformSync,
  startService,
};

module.exports = api;
