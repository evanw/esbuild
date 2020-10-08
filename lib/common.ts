import * as types from "./types";
import * as protocol from "./stdio_protocol";

declare const ESBUILD_VERSION: string;

function validateTarget(target: string): string {
  target += ''
  if (target.indexOf(',') >= 0) throw new Error(`Invalid target: ${target}`)
  return target
}

let mustBeBoolean = (value: boolean | undefined): string | null =>
  typeof value === 'boolean' ? null : 'a boolean';

let mustBeString = (value: string | undefined): string | null =>
  typeof value === 'string' ? null : 'a string';

let mustBeInteger = (value: number | undefined): string | null =>
  typeof value === 'number' && value === (value | 0) ? null : 'an integer';

let mustBeArray = (value: string[] | undefined): string | null =>
  Array.isArray(value) ? null : 'an array';

let mustBeObject = (value: Object | undefined): string | null =>
  typeof value === 'object' && value !== null && !Array.isArray(value) ? null : 'an object';

let mustBeStringOrBoolean = (value: string | boolean | undefined): string | null =>
  typeof value === 'string' || typeof value === 'boolean' ? null : 'a string or a boolean';

let mustBeStringOrArray = (value: string | string[] | undefined): string | null =>
  typeof value === 'string' || Array.isArray(value) ? null : 'a string or an array';

let mustBeBooleanOrArray = (value: boolean | string[] | undefined): string | null =>
  typeof value === 'boolean' || Array.isArray(value) ? null : 'a boolean or an array';

type OptionKeys = { [key: string]: boolean };

function getFlag<T, K extends keyof T>(object: T, keys: OptionKeys, key: K, mustBeFn: (value: T[K]) => string | null): T[K] | undefined {
  let value = object[key];
  keys[key + ''] = true;
  if (value === undefined) return undefined;
  let mustBe = mustBeFn(value);
  if (mustBe !== null) throw new Error(`"${key}" must be ${mustBe}`);
  return value;
}

function checkForInvalidFlags(object: Object, keys: OptionKeys): void {
  for (let key in object) {
    if (!(key in keys)) {
      throw new Error(`Invalid option: "${key}"`);
    }
  }
}

type CommonOptions = types.BuildOptions | types.TransformOptions;

function pushLogFlags(flags: string[], options: CommonOptions, keys: OptionKeys, isTTY: boolean, logLevelDefault: types.LogLevel): void {
  let color = getFlag(options, keys, 'color', mustBeBoolean);
  let logLevel = getFlag(options, keys, 'logLevel', mustBeString);
  let errorLimit = getFlag(options, keys, 'errorLimit', mustBeInteger);

  if (color) flags.push(`--color=${color}`);
  else if (isTTY) flags.push(`--color=true`); // This is needed to fix "execFileSync" which buffers stderr
  flags.push(`--log-level=${logLevel || logLevelDefault}`);
  flags.push(`--error-limit=${errorLimit || 0}`);
}

function pushCommonFlags(flags: string[], options: CommonOptions, keys: OptionKeys): void {
  let target = getFlag(options, keys, 'target', mustBeStringOrArray);
  let format = getFlag(options, keys, 'format', mustBeString);
  let globalName = getFlag(options, keys, 'globalName', mustBeString);
  let strict = getFlag(options, keys, 'strict', mustBeBooleanOrArray);
  let minify = getFlag(options, keys, 'minify', mustBeBoolean);
  let minifySyntax = getFlag(options, keys, 'minifySyntax', mustBeBoolean);
  let minifyWhitespace = getFlag(options, keys, 'minifyWhitespace', mustBeBoolean);
  let minifyIdentifiers = getFlag(options, keys, 'minifyIdentifiers', mustBeBoolean);
  let jsxFactory = getFlag(options, keys, 'jsxFactory', mustBeString);
  let jsxFragment = getFlag(options, keys, 'jsxFragment', mustBeString);
  let define = getFlag(options, keys, 'define', mustBeObject);
  let pure = getFlag(options, keys, 'pure', mustBeArray);

  if (target) {
    if (Array.isArray(target)) flags.push(`--target=${Array.from(target).map(validateTarget).join(',')}`)
    else flags.push(`--target=${validateTarget(target)}`)
  }
  if (format) flags.push(`--format=${format}`);
  if (globalName) flags.push(`--global-name=${globalName}`);
  if (strict === true) flags.push(`--strict`);
  else if (strict) for (let key of strict) flags.push(`--strict:${key}`);

  if (minify) flags.push('--minify');
  if (minifySyntax) flags.push('--minify-syntax');
  if (minifyWhitespace) flags.push('--minify-whitespace');
  if (minifyIdentifiers) flags.push('--minify-identifiers');

  if (jsxFactory) flags.push(`--jsx-factory=${jsxFactory}`);
  if (jsxFragment) flags.push(`--jsx-fragment=${jsxFragment}`);
  if (define) {
    for (let key in define) {
      if (key.indexOf('=') >= 0) throw new Error(`Invalid define: ${key}`);
      flags.push(`--define:${key}=${define[key]}`);
    }
  }
  if (pure) for (let fn of pure) flags.push(`--pure:${fn}`);
}

function flagsForBuildOptions(options: types.BuildOptions, isTTY: boolean, logLevelDefault: types.LogLevel): [string[], boolean, string | null, string | null] {
  let flags: string[] = [];
  let keys: OptionKeys = Object.create(null);
  let stdinContents: string | null = null;
  let stdinResolveDir: string | null = null;
  pushLogFlags(flags, options, keys, isTTY, logLevelDefault);
  pushCommonFlags(flags, options, keys);

  let sourcemap = getFlag(options, keys, 'sourcemap', mustBeStringOrBoolean);
  let bundle = getFlag(options, keys, 'bundle', mustBeBoolean);
  let splitting = getFlag(options, keys, 'splitting', mustBeBoolean);
  let metafile = getFlag(options, keys, 'metafile', mustBeString);
  let outfile = getFlag(options, keys, 'outfile', mustBeString);
  let outdir = getFlag(options, keys, 'outdir', mustBeString);
  let platform = getFlag(options, keys, 'platform', mustBeString);
  let tsconfig = getFlag(options, keys, 'tsconfig', mustBeString);
  let resolveExtensions = getFlag(options, keys, 'resolveExtensions', mustBeArray);
  let mainFields = getFlag(options, keys, 'mainFields', mustBeArray);
  let external = getFlag(options, keys, 'external', mustBeArray);
  let loader = getFlag(options, keys, 'loader', mustBeObject);
  let outExtension = getFlag(options, keys, 'outExtension', mustBeObject);
  let entryPoints = getFlag(options, keys, 'entryPoints', mustBeArray);
  let stdin = getFlag(options, keys, 'stdin', mustBeObject);
  let write = getFlag(options, keys, 'write', mustBeBoolean) !== false;
  checkForInvalidFlags(options, keys);

  if (sourcemap) flags.push(`--sourcemap${sourcemap === true ? '' : `=${sourcemap}`}`);
  if (bundle) flags.push('--bundle');
  if (splitting) flags.push('--splitting');
  if (metafile) flags.push(`--metafile=${metafile}`);
  if (outfile) flags.push(`--outfile=${outfile}`);
  if (outdir) flags.push(`--outdir=${outdir}`);
  if (platform) flags.push(`--platform=${platform}`);
  if (tsconfig) flags.push(`--tsconfig=${tsconfig}`);
  if (resolveExtensions) flags.push(`--resolve-extensions=${resolveExtensions.join(',')}`);
  if (mainFields) flags.push(`--resolve-extensions=${mainFields.join(',')}`);
  if (external) for (let name of external) flags.push(`--external:${name}`);
  if (loader) {
    for (let ext in loader) {
      if (ext.indexOf('=') >= 0) throw new Error(`Invalid extension: ${ext}`);
      flags.push(`--loader:${ext}=${loader[ext]}`);
    }
  }
  if (outExtension) {
    for (let ext in outExtension) {
      if (ext.indexOf('=') >= 0) throw new Error(`Invalid extension: ${ext}`);
      flags.push(`--out-extension:${ext}=${outExtension[ext]}`);
    }
  }

  if (entryPoints) {
    for (let entryPoint of entryPoints) {
      entryPoint += '';
      if (entryPoint.startsWith('-')) throw new Error(`Invalid entry point: ${entryPoint}`);
      flags.push(entryPoint);
    }
  }

  if (stdin) {
    let stdinKeys: OptionKeys = Object.create(null);
    let contents = getFlag(stdin, stdinKeys, 'contents', mustBeString);
    let resolveDir = getFlag(stdin, stdinKeys, 'resolveDir', mustBeString);
    let sourcefile = getFlag(stdin, stdinKeys, 'sourcefile', mustBeString);
    let loader = getFlag(stdin, stdinKeys, 'loader', mustBeString);
    checkForInvalidFlags(stdin, stdinKeys);

    if (sourcefile) flags.push(`--sourcefile=${sourcefile}`);
    if (loader) flags.push(`--loader=${loader}`);
    if (resolveDir) stdinResolveDir = resolveDir + '';
    stdinContents = contents ? contents + '' : '';
  }

  return [flags, write, stdinContents, stdinResolveDir];
}

function flagsForTransformOptions(options: types.TransformOptions, isTTY: boolean, logLevelDefault: types.LogLevel): string[] {
  let flags: string[] = [];
  let keys: OptionKeys = Object.create(null);
  pushLogFlags(flags, options, keys, isTTY, logLevelDefault);
  pushCommonFlags(flags, options, keys);

  let sourcemap = getFlag(options, keys, 'sourcemap', mustBeStringOrBoolean);
  let sourcefile = getFlag(options, keys, 'sourcefile', mustBeString);
  let loader = getFlag(options, keys, 'loader', mustBeString);
  checkForInvalidFlags(options, keys);

  if (sourcemap) flags.push(`--sourcemap=${sourcemap === true ? 'external' : sourcemap}`);
  if (sourcefile) flags.push(`--sourcefile=${sourcefile}`);
  if (loader) flags.push(`--loader=${loader}`);

  return flags;
}

export interface StreamIn {
  writeToStdin: (data: Uint8Array) => void;
  readFileSync?: (path: string, encoding: 'utf8') => string;
}

export interface StreamOut {
  readFromStdout: (data: Uint8Array) => void;
  afterClose: () => void;
  service: StreamService;
}

export interface StreamFS {
  writeFile(contents: string, callback: (path: string | null) => void): void;
  readFile(path: string, callback: (err: Error | null, contents: string | null) => void): void;
}

export interface StreamService {
  build(
    options: types.BuildOptions,
    isTTY: boolean,
    callback: (err: Error | null, res: types.BuildResult | null) => void,
  ): void;

  transform(
    input: string,
    options: types.TransformOptions,
    isTTY: boolean,
    fs: StreamFS,
    callback: (err: Error | null, res: types.TransformResult | null) => void,
  ): void;
}

// This can't use any promises because it must work for both sync and async code
export function createChannel(streamIn: StreamIn): StreamOut {
  let responseCallbacks = new Map<number, (error: string | null, response: protocol.Value) => void>();
  let isClosed = false;
  let nextRequestID = 0;

  // Use a long-lived buffer to store stdout data
  let stdout = new Uint8Array(16 * 1024);
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

    // Process all complete (i.e. not partial) packets
    let offset = 0;
    while (offset + 4 <= stdoutUsed) {
      let length = protocol.readUInt32LE(stdout, offset);
      if (offset + 4 + length > stdoutUsed) {
        break;
      }
      offset += 4;
      handleIncomingPacket(stdout.slice(offset, offset + length));
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
    for (let callback of responseCallbacks.values()) {
      callback('The service was stopped', null);
    }
    responseCallbacks.clear();
  };

  let sendRequest = <Req, Res>(value: Req, callback: (error: string | null, response: Res | null) => void): void => {
    if (isClosed) return callback('The service is no longer running', null);
    let id = nextRequestID++;
    responseCallbacks.set(id, callback as any);
    streamIn.writeToStdin(protocol.encodePacket({ id, isRequest: true, value: value as any }));
  };

  let sendResponse = (id: number, value: protocol.Value): void => {
    if (isClosed) throw new Error('The service is no longer running');
    streamIn.writeToStdin(protocol.encodePacket({ id, isRequest: false, value }));
  };

  let handleRequest = async (id: number, request: any) => {
    // Catch exceptions in the code below so they get passed to the caller
    try {
      let command = request.command;
      switch (command) {
        default:
          throw new Error(`Invalid command: ` + command);
      }
    } catch (e) {
      sendResponse(id, { errors: [await extractErrorMessageV8(e, streamIn)] } as any);
    }
  };

  let isFirstPacket = true;

  let handleIncomingPacket = (bytes: Uint8Array): void => {
    // The first packet is a version check
    if (isFirstPacket) {
      isFirstPacket = false;

      // Validate the binary's version number to make sure esbuild was installed
      // correctly. This check was added because some people have reported
      // errors that appear to indicate an incorrect installation.
      let binaryVersion = String.fromCharCode(...bytes);
      if (binaryVersion !== ESBUILD_VERSION) {
        throw new Error(`Cannot start service: Host version "${ESBUILD_VERSION}" does not match binary version ${JSON.stringify(binaryVersion)}`);
      }
      return;
    }

    let packet = protocol.decodePacket(bytes) as any;

    if (packet.isRequest) {
      handleRequest(packet.id, packet.value);
    }

    else {
      let callback = responseCallbacks.get(packet.id)!;
      responseCallbacks.delete(packet.id);
      if (packet.value.error) callback(packet.value.error, {});
      else callback(null, packet.value);
    }
  };

  return {
    readFromStdout,
    afterClose,

    service: {
      build(options, isTTY, callback) {
        const logLevelDefault = 'info';
        try {
          let [flags, write, stdin, resolveDir] = flagsForBuildOptions(options, isTTY, logLevelDefault);
          let request: protocol.BuildRequest = { command: 'build', flags, write, stdin, resolveDir };
          sendRequest<protocol.BuildRequest, protocol.BuildResponse>(request, (error, response) => {
            if (error) return callback(new Error(error), null);
            let errors = response!.errors;
            let warnings = response!.warnings;
            if (errors.length > 0) return callback(failureErrorWithLog('Build failed', errors, warnings), null);
            let result: types.BuildResult = { warnings };
            if (!write) result.outputFiles = response!.outputFiles;
            callback(null, result);
          });
        } catch (e) {
          let flags: string[] = [];
          try { pushLogFlags(flags, options, {}, isTTY, logLevelDefault) } catch { }
          sendRequest({ command: 'error', flags, error: extractErrorMessageV8(e, streamIn) }, () => {
            callback(e, null);
          });
        }
      },

      transform(input, options, isTTY, fs, callback) {
        const logLevelDefault = 'silent';

        // Ideally the "transform()" API would be faster than calling "build()"
        // since it doesn't need to touch the file system. However, performance
        // measurements with large files on macOS indicate that sending the data
        // over the stdio pipe can be 2x slower than just using a temporary file.
        //
        // This appears to be an OS limitation. Both the JavaScript and Go code
        // are using large buffers but the pipe only writes data in 8kb chunks.
        // An investigation seems to indicate that this number is hard-coded into
        // the OS source code. Presumably files are faster because the OS uses
        // a larger chunk size, or maybe even reads everything in one syscall.
        //
        // The cross-over size where this starts to be faster is around 1mb on
        // my machine. In that case, this code tries to use a temporary file if
        // possible but falls back to sending the data over the stdio pipe if
        // that doesn't work.
        let start = (inputPath: string | null) => {
          try {
            let flags = flagsForTransformOptions(options, isTTY, logLevelDefault);
            let request: protocol.TransformRequest = {
              command: 'transform',
              flags,
              inputFS: inputPath !== null,
              input: inputPath !== null ? inputPath : input + '',
            };
            sendRequest<protocol.TransformRequest, protocol.TransformResponse>(request, (error, response) => {
              if (error) return callback(new Error(error), null);
              let errors = response!.errors;
              let warnings = response!.warnings;
              let outstanding = 1;
              let next = () => --outstanding === 0 && callback(null, { warnings, js: response!.js, jsSourceMap: response!.jsSourceMap });
              if (errors.length > 0) return callback(failureErrorWithLog('Transform failed', errors, warnings), null);

              // Read the JavaScript file from the file system
              if (response!.jsFS) {
                outstanding++;
                fs.readFile(response!.js, (err, contents) => {
                  if (err !== null) {
                    callback(err, null);
                  } else {
                    response!.js = contents!;
                    next();
                  }
                });
              }

              // Read the source map file from the file system
              if (response!.jsSourceMapFS) {
                outstanding++;
                fs.readFile(response!.jsSourceMap, (err, contents) => {
                  if (err !== null) {
                    callback(err, null);
                  } else {
                    response!.jsSourceMap = contents!;
                    next();
                  }
                });
              }

              next();
            });
          } catch (e) {
            let flags: string[] = [];
            try { pushLogFlags(flags, options, {}, isTTY, logLevelDefault) } catch { }
            sendRequest({ command: 'error', flags, error: extractErrorMessageV8(e, streamIn) }, () => {
              callback(e, null);
            });
          }
        };
        if (typeof input === 'string' && input.length > 1024 * 1024) {
          let next = start;
          start = () => fs.writeFile(input, next);
        }
        start(null);
      },
    },
  };
}

function extractErrorMessageV8(e: any, streamIn: StreamIn): types.Message {
  let text = 'Internal error'
  let location: types.Location | null = null

  try {
    text = ((e && e.message) || e) + '';
  } catch {
  }

  // Optionally attempt to extract the file from the stack trace, works in V8/node
  try {
    let stack = e.stack + ''
    let lines = stack.split('\n', 3)
    let at = '    at '

    // Check to see if this looks like a V8 stack trace
    if (streamIn.readFileSync && !lines[0].startsWith(at) && lines[1].startsWith(at)) {
      let line = lines[1].slice(at.length)
      while (true) {
        // Unwrap a function name
        let match = /^\S+ \((.*)\)$/.exec(line)
        if (match) {
          line = match[1]
          continue
        }

        // Unwrap an eval wrapper
        match = /^eval at \S+ \((.*)\)(?:, \S+:\d+:\d+)?$/.exec(line)
        if (match) {
          line = match[1]
          continue
        }

        // Match on the file location
        match = /^(\S+):(\d+):(\d+)$/.exec(line)
        if (match) {
          let contents = streamIn.readFileSync(match[1], 'utf8')
          let lineText = contents.split(/\r\n|\r|\n|\u2028|\u2029/)[+match[2] - 1] || ''
          location = {
            file: match[1],
            line: +match[2],
            column: +match[3] - 1,
            length: 0,
            lineText: lineText + '\n' + lines.slice(1).join('\n'),
          }
        }
        break
      }
    }
  } catch {
  }

  return { text, location }
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
