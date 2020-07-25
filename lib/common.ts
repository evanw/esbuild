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
  if (options.define) {
    for (let key in options.define) {
      if (key.indexOf('=') >= 0) throw new Error(`Invalid define: ${key}`);
      flags.push(`--define:${key}=${options.define[key]}`);
    }
  }
  if (options.pure) for (let fn of options.pure) flags.push(`--pure:${fn}`);

  if (options.color) flags.push(`--color=${options.color}`);
  else if (isTTY) flags.push(`--color=true`); // This is needed to fix "execFileSync" which buffers stderr
  flags.push(`--log-level=${options.logLevel || logLevelDefault}`);
  flags.push(`--error-limit=${options.errorLimit || 0}`);
}

function flagsForBuildOptions(options: types.BuildOptions, isTTY: boolean): [string[], string | null, string | null] {
  let flags: string[] = [];
  let stdinContents: string | null = null;
  let stdinResolveDir: string | null = null;
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
  if (options.tsconfig) flags.push(`--tsconfig=${options.tsconfig}`);
  if (options.resolveExtensions) flags.push(`--resolve-extensions=${options.resolveExtensions.join(',')}`);
  if (options.external) for (let name of options.external) flags.push(`--external:${name}`);
  if (options.loader) {
    for (let ext in options.loader) {
      if (ext.indexOf('=') >= 0) throw new Error(`Invalid extension: ${ext}`);
      flags.push(`--loader:${ext}=${options.loader[ext]}`);
    }
  }
  if (options.outExtension) {
    for (let ext in options.outExtension) {
      if (ext.indexOf('=') >= 0) throw new Error(`Invalid extension: ${ext}`);
      flags.push(`--out-extension:${ext}=${options.outExtension[ext]}`);
    }
  }

  if (options.entryPoints) {
    for (let entryPoint of options.entryPoints) {
      if (entryPoint.startsWith('-')) throw new Error(`Invalid entry point: ${entryPoint}`);
      flags.push(entryPoint);
    }
  }

  if (options.stdin) {
    let { contents, resolveDir, sourcefile, loader } = options.stdin;
    if (sourcefile) flags.push(`--sourcefile=${sourcefile}`);
    if (loader) flags.push(`--loader=${loader}`);
    if (resolveDir) stdinResolveDir = resolveDir + '';
    stdinContents = contents ? contents + '' : '';
  }

  return [flags, stdinContents, stdinResolveDir];
}

function flagsForTransformOptions(options: types.TransformOptions, isTTY: boolean): string[] {
  let flags: string[] = [];
  pushCommonFlags(flags, options, isTTY, 'silent');

  if (options.sourcemap) flags.push(`--sourcemap=${options.sourcemap === true ? 'external' : options.sourcemap}`);
  if (options.sourcefile) flags.push(`--sourcefile=${options.sourcefile}`);
  if (options.loader) flags.push(`--loader=${options.loader}`);

  return flags;
}

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
  let callbacks = new Map<number, (error: string | null, response: protocol.Value) => void>();
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
    for (let callback of callbacks.values()) {
      callback('The service was stopped', null);
    }
    callbacks.clear();
  };

  let sendRequest = <Req, Res>(value: [string, Req], callback: (error: string | null, response: Res | null) => void): void => {
    if (isClosed) return callback('The service is no longer running', null);
    let id = nextID++;
    callbacks.set(id, callback as any);
    options.writeToStdin(protocol.encodePacket({ id, isRequest: true, value: value as any }));
  };

  let sendResponse = (id: number, value: protocol.Value): void => {
    if (isClosed) throw new Error('The service is no longer running');
    options.writeToStdin(protocol.encodePacket({ id, isRequest: false, value }));
  };

  let handleRequest = (id: number, command: string, request: protocol.Value) => {
    // Catch exceptions in the code below so they get passed to the caller
    try {
      switch (command) {
        default:
          throw new Error(`Invalid command: ` + command);
      }
    } catch (e) {
      let error = 'Internal error'
      try {
        error = e + '';
      } catch {
      }
      sendResponse(id, { error });
    }
  };

  let handleIncomingPacket = (bytes: Uint8Array): void => {
    let packet = protocol.decodePacket(bytes) as any;

    if (packet.isRequest) {
      handleRequest(packet.id, packet.value[0], packet.value[1]);
    }

    else {
      let callback = callbacks.get(packet.id)!;
      callbacks.delete(packet.id);
      if (packet.value.error) callback(packet.value.error, {});
      else callback(null, packet.value);
    }
  };

  return {
    readFromStdout,
    afterClose,

    service: {
      build(options, isTTY, callback) {
        let [flags, stdin, resolveDir] = flagsForBuildOptions(options, isTTY);
        let write = options.write !== false;
        sendRequest<protocol.BuildRequest, protocol.BuildResponse>(
          ['build', { flags, write, stdin, resolveDir }],
          (error, response) => {
            if (error) return callback(new Error(error), null);
            let errors = response!.errors;
            let warnings = response!.warnings;
            if (errors.length > 0) return callback(failureErrorWithLog('Build failed', errors, warnings), null);
            let result: types.BuildResult = { warnings };
            if (!write) result.outputFiles = response!.outputFiles;
            callback(null, result);
          },
        );
      },

      transform(input, options, isTTY, callback) {
        let flags = flagsForTransformOptions(options, isTTY);
        sendRequest<protocol.TransformRequest, protocol.TransformResponse>(
          ['transform', { flags, input }],
          (error, response) => {
            if (error) return callback(new Error(error), null);
            let errors = response!.errors;
            let warnings = response!.warnings;
            if (errors.length > 0) return callback(failureErrorWithLog('Transform failed', errors, warnings), null);
            callback(null, { warnings, js: response!.js, jsSourceMap: response!.jsSourceMap });
          },
        );
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
