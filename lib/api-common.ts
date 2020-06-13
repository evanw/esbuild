import * as types from "./api-types";

function pushCommonFlags(flags: string[], options: types.CommonOptions, isTTY: boolean, logLevelDefault: types.LogLevel): void {
  if (options.target) flags.push(`--target=${options.target}`);

  if (options.minify) flags.push('--minify');
  if (options.minifySyntax) flags.push('--minify-syntax');
  if (options.minifyWhitespace) flags.push('--minify-whitespace');
  if (options.minifyIdentifiers) flags.push('--minify-identifiers');

  if (options.jsxFactory) flags.push(`--jsx-factory=${options.jsxFactory}`);
  if (options.jsxFragment) flags.push(`--jsx-fragment=${options.jsxFragment}`);
  if (options.define) for (let key in options.define) flags.push(`--define:${key}=${options.define[key]}`);

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
  if (options.metafile) flags.push(`--metafile=${options.metafile}`);
  if (options.outfile) flags.push(`--outfile=${options.outfile}`);
  if (options.outdir) flags.push(`--outdir=${options.outdir}`);
  if (options.platform) flags.push(`--platform=${options.platform}`);
  if (options.format) flags.push(`--format=${options.format}`);
  if (options.resolveExtensions) flags.push(`--resolve-extensions=${options.resolveExtensions.join(',')}`);
  if (options.external) for (let name of options.external) flags.push(`--external:${name}`);
  if (options.loader) for (let ext in options.loader) flags.push(`--loader:${ext}=${options.loader[ext]}`);

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
type Response = { [key: string]: string };
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

// This can't use any promises because it must work for both sync and async code
export function createChannel(options: StreamIn): StreamOut {
  let requests = new Map<string, ResponseCallback>();
  let encoder = new TextEncoder();
  let decoder = new TextDecoder();
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

    // Process all complete (i.e. not partial) responses
    let offset = 0;
    while (offset + 4 <= stdoutUsed) {
      let length = readUInt32LE(stdout, offset);
      if (offset + 4 + length > stdoutUsed) {
        break;
      }
      offset += 4;
      handleResponse(stdout.slice(offset, offset + length));
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
    for (let callback of requests.values()) {
      callback('The service was stopped', {});
    }
    requests.clear();
  };

  let sendRequest = (request: Request, callback: ResponseCallback): void => {
    if (isClosed) return callback('The service is no longer running', {});

    // Allocate an id for this request
    let id = (nextID++).toString();
    requests.set(id, callback);

    // Figure out how long the request will be
    let argBuffers = [encoder.encode(id)];
    let length = 4 + 4 + 4 + argBuffers[0].length;
    for (let arg of request) {
      let argBuffer = encoder.encode(arg);
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
    writeUint32(argBuffers.length);
    for (let argBuffer of argBuffers) {
      writeUint32(argBuffer.length);
      bytes.set(argBuffer, offset);
      offset += argBuffer.length;
    }
    options.writeToStdin(bytes);
  };

  let handleResponse = (bytes: Uint8Array): void => {
    let offset = 0;
    let eat = (n: number) => {
      offset += n;
      if (offset > bytes.length) throw new Error('Invalid message');
      return offset - n;
    };
    let count = readUInt32LE(bytes, eat(4));
    let response: Response = {};
    let id;

    // Parse the response into a map
    for (let i = 0; i < count; i++) {
      let keyLength = readUInt32LE(bytes, eat(4));
      let key = decoder.decode(bytes.slice(offset, eat(keyLength) + keyLength));
      let valueLength = readUInt32LE(bytes, eat(4));
      let value = decoder.decode(bytes.slice(offset, eat(valueLength) + valueLength));
      if (key === 'id') id = value;
      else response[key] = value;
    }

    // Dispatch the response
    if (!id) throw new Error('Invalid message');
    let callback = requests.get(id)!;
    requests.delete(id);
    if (response.error) callback(response.error, {});
    else callback(null, response);
  };

  return {
    readFromStdout,
    afterClose,

    service: {
      build(options, isTTY, callback) {
        let flags = flagsForBuildOptions(options, isTTY);
        sendRequest(['build'].concat(flags), (error, response) => {
          if (error) return callback(failureErrorWithLog(error, [], []), null);
          let errors = jsonToMessages(response.errors);
          let warnings = jsonToMessages(response.warnings);
          if (errors.length > 0) return callback(failureErrorWithLog('Build failed', errors, warnings), null);
          callback(null, { warnings });
        });
      },

      transform(input, options, isTTY, callback) {
        let flags = flagsForTransformOptions(options, isTTY);
        sendRequest(['transform', input].concat(flags), (error, response) => {
          if (error) return callback(failureErrorWithLog(error, [], []), null);
          let errors = jsonToMessages(response.errors);
          let warnings = jsonToMessages(response.warnings);
          if (errors.length > 0) return callback(failureErrorWithLog('Transform failed', errors, warnings), null);
          callback(null, { warnings, js: response.js, jsSourceMap: response.jsSourceMap });
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
