import * as types from "./api-types";

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

type Request = string[];
type Response = { [key: string]: Uint8Array };
type ResponseCallback = (err: string | null, res: Response) => void;

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

let encodeUTF8: (text: string) => Uint8Array
let decodeUTF8: (bytes: Uint8Array) => string

// For the browser and node 12.x
if (typeof TextEncoder !== 'undefined' && typeof TextDecoder !== 'undefined') {
  let encoder = new TextEncoder();
  let decoder = new TextDecoder();
  encodeUTF8 = text => encoder.encode(text);
  decodeUTF8 = bytes => decoder.decode(bytes);
}

// For node 10.x
else if (typeof Buffer !== 'undefined') {
  encodeUTF8 = text => Buffer.from(text);
  decodeUTF8 = bytes => Buffer.from(bytes).toString();
}

else {
  throw new Error('No UTF-8 codec found');
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
      let length = readUInt32LE(stdout, offset);
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

  let sendRequest = (request: Request, callback: ResponseCallback): void => {
    if (isClosed) return callback('The service is no longer running', {});

    // Allocate an id for this request
    let id = nextID++;
    callbacks.set(id, callback);

    // Figure out how long the request will be
    let argBuffers: Uint8Array[] = [];
    let length = 12;
    for (let arg of request) {
      let argBuffer = encodeUTF8(arg);
      argBuffers.push(argBuffer);
      length += 4 + argBuffer.length;
    }

    // Write out the request message
    let bytes = new Uint8Array(length);
    let offset = 0;
    let writeUint32 = (value: number) => {
      writeUInt32LE(bytes, value, offset);
      offset += 4;
    };
    writeUint32(length - 4);
    writeUint32(id << 1);
    writeUint32(argBuffers.length);
    for (let argBuffer of argBuffers) {
      writeUint32(argBuffer.length);
      bytes.set(argBuffer, offset);
      offset += argBuffer.length;
    }
    options.writeToStdin(bytes);
  };

  let sendResponse = (id: number, response: Response): void => {
    if (isClosed) throw new Error('The service is no longer running');

    // Figure out how long the response will be
    let keyBuffers: { [key: string]: Uint8Array } = {};
    let length = 12;
    let count = 0;
    for (let key in response) {
      let keyBuffer = encodeUTF8(key);
      let value = response[key];
      keyBuffers[key] = keyBuffer;
      length += 4 + keyBuffer.length + 4 + value.length;
      count++;
    }

    // Write out the request message
    let bytes = new Uint8Array(length);
    let offset = 0;
    let writeUint32 = (value: number) => {
      writeUInt32LE(bytes, value, offset);
      offset += 4;
    };
    writeUint32(length - 4);
    writeUint32((id << 1) | 1);
    writeUint32(count);
    for (let key in response) {
      let keyBuffer = keyBuffers[key];
      let value = response[key];
      writeUint32(keyBuffer.length);
      bytes.set(keyBuffer, offset);
      offset += keyBuffer.length;
      writeUint32(value.length);
      bytes.set(value, offset);
      offset += value.length;
    }
    options.writeToStdin(bytes);
  };

  let handleRequest = (id: number, command: string, request: Request) => {
    // Catch exceptions in the code below so they get passed to the caller
    try {
      switch (command) {
        default:
          throw new Error(`Invalid command: ` + command);
      }
    } catch (e) {
      sendResponse(id, {
        error: encodeUTF8(e + ''),
      });
    }
  };

  let handleIncomingMessage = (bytes: Uint8Array): void => {
    let offset = 0;
    let eat = (n: number) => {
      offset += n;
      if (offset > bytes.length) throw new Error('Invalid message');
      return offset - n;
    };
    let id = readUInt32LE(bytes, eat(4));
    let count = readUInt32LE(bytes, eat(4));
    let isRequest = !(id & 1);
    id >>>= 1;

    if (isRequest) {
      let request: Request = [];

      // Parse the request into an array
      for (let i = 0; i < count; i++) {
        let valueLength = readUInt32LE(bytes, eat(4));
        let value = bytes.slice(offset, eat(valueLength) + valueLength);
        request.push(decodeUTF8(value));
      }

      // Dispatch the request
      if (request.length < 1 || offset !== bytes.length) throw new Error('Invalid request');
      handleRequest(id, request[0], request.slice(1));
    } else {
      let response: Response = {};

      // Parse the response into a map
      for (let i = 0; i < count; i++) {
        let keyLength = readUInt32LE(bytes, eat(4));
        let key = decodeUTF8(bytes.slice(offset, eat(keyLength) + keyLength));
        let valueLength = readUInt32LE(bytes, eat(4));
        let value = bytes.slice(offset, eat(valueLength) + valueLength);
        response[key] = value;
      }

      // Dispatch the response
      if (offset !== bytes.length) throw new Error('Invalid response');
      let callback = callbacks.get(id)!;
      callbacks.delete(id);
      if (response.error) callback(decodeUTF8(response.error), {});
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
          let errors = jsonToMessages(decodeUTF8(response.errors));
          let warnings = jsonToMessages(decodeUTF8(response.warnings));
          if (errors.length > 0) return callback(failureErrorWithLog('Build failed', errors, warnings), null);
          let result: types.BuildResult = { warnings };
          if (options.write === false) result.outputFiles = decodeOutputFiles(response.outputFiles);
          callback(null, result);
        });
      },

      transform(input, options, isTTY, callback) {
        let flags = flagsForTransformOptions(options, isTTY);
        sendRequest(['transform', input].concat(flags), (error, response) => {
          if (error) return callback(new Error(error), null);
          let errors = jsonToMessages(decodeUTF8(response.errors));
          let warnings = jsonToMessages(decodeUTF8(response.warnings));
          if (errors.length > 0) return callback(failureErrorWithLog('Transform failed', errors, warnings), null);
          callback(null, { warnings, js: decodeUTF8(response.js), jsSourceMap: decodeUTF8(response.jsSourceMap) });
        });
      },
    },
  };
}

function readUInt32LE(buffer: Uint8Array, offset: number): number {
  return buffer[offset++] |
    (buffer[offset++] << 8) |
    (buffer[offset++] << 16) |
    (buffer[offset++] << 24);
}

function writeUInt32LE(buffer: Uint8Array, value: number, offset: number): void {
  buffer[offset++] = value;
  buffer[offset++] = value >> 8;
  buffer[offset++] = value >> 16;
  buffer[offset++] = value >> 24;
}

function jsonToMessages(json: string): types.Message[] {
  let parts = JSON.parse(json);
  let messages: types.Message[] = [];
  for (let i = 0; i < parts.length; i += 6) {
    messages.push({
      text: parts[i],
      location: parts[i + 1] < 0 ? null : {
        length: parts[i + 1],
        file: parts[i + 2],
        line: parts[i + 3],
        column: parts[i + 4],
        lineText: parts[i + 5],
      },
    });
  }
  return messages;
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

function decodeOutputFiles(bytes: Uint8Array): types.OutputFile[] {
  let outputFiles: types.OutputFile[] = [];
  let offset = 0;
  let count = readUInt32LE(bytes, offset);
  offset += 4;
  for (let i = 0; i < count; i++) {
    let pathLength = readUInt32LE(bytes, offset);
    let path = decodeUTF8(bytes.slice(offset + 4, offset + 4 + pathLength));
    offset += 4 + pathLength;
    let contentsLength = readUInt32LE(bytes, offset);
    let contents = new Uint8Array(bytes.slice(offset + 4, offset + 4 + contentsLength));
    offset += 4 + contentsLength;
    outputFiles.push({ path, contents });
  }
  return outputFiles;
}
