import * as types from "./api-types";
import * as common from "./api-common";
import * as child_process from "child_process";
import * as path from "path";
import * as os from "os";
import { isatty } from "tty";

// This file is used for both the "esbuild" package and the "esbuild-wasm"
// package. "WASM" will be true for "esbuild-wasm" and false for "esbuild".
declare let WASM: boolean;

let esbuildCommandAndArgs = (): [string, string[]] => {
  let platform = process.platform;
  let arch = os.arch();

  if (WASM) {
    return ['node', [path.join(__dirname, '..', 'bin', 'esbuild')]];
  }

  if (platform === 'win32' && arch === 'x64') {
    return [path.join(__dirname, '..', 'esbuild.exe'), []];
  }

  return [path.join(__dirname, '..', 'bin', 'esbuild'), []];
};

// Return true if stderr is a TTY
let isTTY = () => isatty(2);

export let build: typeof types.build = options => {
  return startService().then(service => {
    let promise = service.build(options);
    promise.then(service.stop, service.stop); // Kill the service afterwards
    return promise;
  });
};

export let transform: typeof types.transform = (input, options) => {
  return startService().then(service => {
    let promise = service.transform(input, options);
    promise.then(service.stop, service.stop); // Kill the service afterwards
    return promise;
  });
};

export let buildSync: typeof types.buildSync = options => {
  let result: types.BuildResult;
  runServiceSync(service => service.build(options, isTTY(), (err, res) => {
    if (err) throw err;
    result = res!;
  }));
  return result!;
};

export let transformSync: typeof types.transformSync = (input, options) => {
  let result: types.TransformResult;
  runServiceSync(service => service.transform(input, options, isTTY(), (err, res) => {
    if (err) throw err;
    result = res!;
  }));
  return result!;
};

export let startService: typeof types.startService = () => {
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
  });
  readFromStdout(stdout);
  afterClose();
};
