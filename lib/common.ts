import * as types from "./types";
import * as protocol from "./stdio_protocol";

function validateTarget(target: string): string {
  target += ''
  if (target.indexOf(',') >= 0) throw new Error(`Invalid target: ${target}`)
  return target
}

function pushCommonFlags(flags: string[], options: types.CommonOptions, isTTY: boolean, logLevelDefault: types.LogLevel): void {
  if (options.target) {
    if (options.target instanceof Array) flags.push(`--target=${Array.from(options.target).map(validateTarget).join(',')}`)
    else flags.push(`--target=${validateTarget(options.target)}`)
  }
  if (options.strict === true) flags.push(`--strict`);
  else if (options.strict) for (let key of options.strict) flags.push(`--strict:${key}`);

  if (options.minify) flags.push('--minify');
  if (options.minifySyntax) flags.push('--minify-syntax');
  if (options.minifyWhitespace) flags.push('--minify-whitespace');
  if (options.minifyIdentifiers) flags.push('--minify-identifiers');

  if (options.jsxFactory) flags.push(`--jsx-factory=${options.jsxFactory}`);
  if (options.jsxFragment) flags.push(`--jsx-fragment=${options.jsxFragment}`);
  if (options.define) for (let key in options.define) flags.push(`--define:${key}=${options.define[key]}`);
  if (options.pure) for (let fn of options.pure) flags.push(`--pure:${fn}`);

  if (options.color) flags.push(`--color=${options.color}`);
  else if (isTTY) flags.push(`--color=true`); // This is needed to fix "execFileSync" which buffers stderr
  flags.push(`--log-level=${options.logLevel || logLevelDefault}`);
  flags.push(`--error-limit=${options.errorLimit || 0}`);
}

function flagsForBuildOptions(options: types.BuildOptions, isTTY: boolean): string[] {
  let flags: string[] = [];
  pushCommonFlags(flags, options, isTTY, 'info');

  if (options.sourcemap) flags.push(`--sourcemap${options.sourcemap === true ? '' : `=${options.sourcemap}`}`);
  if (options.globalName) flags.push(`--global-name=${options.globalName}`);
  if (options.bundle) flags.push('--bundle');
  if (options.splitting) flags.push('--splitting');
  if (options.metafile) flags.push(`--metafile=${options.metafile}`);
  if (options.outfile) flags.push(`--outfile=${options.outfile}`);
  if (options.outdir) flags.push(`--outdir=${options.outdir}`);
  if (options.platform) flags.push(`--platform=${options.platform}`);
  if (options.format) flags.push(`--format=${options.format}`);
  if (options.resolveExtensions) flags.push(`--resolve-extensions=${options.resolveExtensions.join(',')}`);
  if (options.external) for (let name of options.external) flags.push(`--external:${name}`);
  if (options.loader) for (let ext in options.loader) flags.push(`--loader:${ext}=${options.loader[ext]}`);
  if (options.write === false) flags.push(`--write=false`);

  for (let entryPoint of options.entryPoints) {
    if (entryPoint.startsWith('-')) throw new Error(`Invalid entry point: ${entryPoint}`);
    flags.push(entryPoint);
  }

  return flags;
}

function flagsForTransformOptions(options: types.TransformOptions, isTTY: boolean): string[] {
  let flags: string[] = [];
  pushCommonFlags(flags, options, isTTY, 'silent');

  if (options.sourcemap) flags.push(`--sourcemap=${options.sourcemap === true ? 'external' : options.sourcemap}`);
  if (options.sourcefile) flags.push(`--sourcefile=${options.sourcefile}`);
  if (options.loader) flags.push(`--loader=${options.loader}`);

  return flags;
}

type ResponseCallback = (err: string | null, res: protocol.Response) => void;

export interface StreamIn {
  writeToStdin: (data: Uint8Array) => void,
}

export interface StreamOut {
  readFromStdout: (data: Uint8Array) => void;
  afterClose: () => void;
  service: StreamService;
}

export interface StreamService {
  build(options: types.BuildOptions, isTTY: boolean, callback: (err: Error | null, res: types.BuildResult | null) => void): void;
  transform(input: string, options: types.TransformOptions, isTTY: boolean, callback: (err: Error | null, res: types.TransformResult | null) => void): void;
}

// This can't use any promises because it must work for both sync and async code
export function createChannel(options: StreamIn): StreamOut {
  let callbacks = new Map<number, ResponseCallback>();
  let isClosed = false;
  let nextID = 0;

  // Use a long-lived buffer to store stdout data
  let stdout = new Uint8Array(4096);
  let stdoutUsed = 0;
  let readFromStdout = (chunk: Uint8Array) => {
    // Append the chunk to the stdout buffer, growing it as necessary
    let limit = stdoutUsed + chunk.length;
    if (limit > stdout.length) {
      let swap = new Uint8Array(limit * 2);
      swap.set(stdout);
      stdout = swap;
    }
    stdout.set(chunk, stdoutUsed);
    stdoutUsed += chunk.length;

    // Process all complete (i.e. not partial) messages
    let offset = 0;
    while (offset + 4 <= stdoutUsed) {
      let length = protocol.readUInt32LE(stdout, offset);
      if (offset + 4 + length > stdoutUsed) {
        break;
      }
      offset += 4;
      handleIncomingMessage(stdout.slice(offset, offset + length));
      offset += length;
    }
    if (offset > 0) {
      stdout.set(stdout.slice(offset));
      stdoutUsed -= offset;
    }
  };

  let afterClose = () => {
    // When the process is closed, fail all pending requests
    isClosed = true;
    for (let callback of callbacks.values()) {
      callback('The service was stopped', {});
    }
    callbacks.clear();
  };

  let sendRequest = (request: protocol.Request, callback: ResponseCallback): void => {
    if (isClosed) return callback('The service is no longer running', {});
    let id = nextID++;
    callbacks.set(id, callback);
    options.writeToStdin(protocol.encodeRequest(id, request));
  };

  let sendResponse = (id: number, response: protocol.Response): void => {
    if (isClosed) throw new Error('The service is no longer running');
    options.writeToStdin(protocol.encodeResponse(id, response));
  };

  let handleRequest = (id: number, command: string, request: protocol.Request) => {
    // Catch exceptions in the code below so they get passed to the caller
    try {
      switch (command) {
        default:
          throw new Error(`Invalid command: ` + command);
      }
    } catch (e) {
      sendResponse(id, {
        error: protocol.encodeUTF8(e + ''),
      });
    }
  };

  let handleIncomingMessage = (bytes: Uint8Array): void => {
    let [id, request, response] = protocol.decodeRequestOrResponse(bytes);

    if (request !== null) {
      if (request.length < 1) throw new Error('Invalid request');
      handleRequest(id, request[0], request.slice(1));
    }

    else if (response !== null) {
      let callback = callbacks.get(id)!;
      callbacks.delete(id);
      if (response.error) callback(protocol.decodeUTF8(response.error), {});
      else callback(null, response);
    }
  };

  return {
    readFromStdout,
    afterClose,

    service: {
      build(options, isTTY, callback) {
        let flags = flagsForBuildOptions(options, isTTY);
        sendRequest(['build'].concat(flags), (error, response) => {
          if (error) return callback(new Error(error), null);
          let errors = protocol.jsonToMessages(response.errors);
          let warnings = protocol.jsonToMessages(response.warnings);
          if (errors.length > 0) return callback(failureErrorWithLog('Build failed', errors, warnings), null);
          let result: types.BuildResult = { warnings };
          if (options.write === false) result.outputFiles = protocol.decodeOutputFiles(response.outputFiles);
          callback(null, result);
        });
      },

      transform(input, options, isTTY, callback) {
        let flags = flagsForTransformOptions(options, isTTY);
        sendRequest(['transform', input].concat(flags), (error, response) => {
          if (error) return callback(new Error(error), null);
          let errors = protocol.jsonToMessages(response.errors);
          let warnings = protocol.jsonToMessages(response.warnings);
          if (errors.length > 0) return callback(failureErrorWithLog('Transform failed', errors, warnings), null);
          callback(null, { warnings, js: protocol.decodeUTF8(response.js), jsSourceMap: protocol.decodeUTF8(response.jsSourceMap) });
        });
      },
    },
  };
}

function failureErrorWithLog(text: string, errors: types.Message[], warnings: types.Message[]): Error {
  let limit = 5
  let summary = errors.length < 1 ? '' : ` with ${errors.length} error${errors.length < 2 ? '' : 's'}:` +
    errors.slice(0, limit + 1).map((e, i) => {
      if (i === limit) return '\n...';
      if (!e.location) return `\nerror: ${e.text}`;
      let { file, line, column } = e.location;
      return `\n${file}:${line}:${column}: error: ${e.text}`;
    }).join('');
  let error: any = new Error(`${text}${summary}`);
  error.errors = errors;
  error.warnings = warnings;
  return error;
}
