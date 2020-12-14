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

let mustBeRegExp = (value: RegExp | undefined): string | null =>
  value instanceof RegExp ? null : 'a RegExp object';

let mustBeInteger = (value: number | undefined): string | null =>
  typeof value === 'number' && value === (value | 0) ? null : 'an integer';

let mustBeFunction = (value: Function | undefined): string | null =>
  typeof value === 'function' ? null : 'a function';

let mustBeArray = <T>(value: T[] | undefined): string | null =>
  Array.isArray(value) ? null : 'an array';

let mustBeObject = (value: Object | undefined): string | null =>
  typeof value === 'object' && value !== null && !Array.isArray(value) ? null : 'an object';

let mustBeObjectOrNull = (value: Object | null | undefined): string | null =>
  typeof value === 'object' && !Array.isArray(value) ? null : 'an object or null';

let mustBeStringOrBoolean = (value: string | boolean | undefined): string | null =>
  typeof value === 'string' || typeof value === 'boolean' ? null : 'a string or a boolean';

let mustBeStringOrObject = (value: string | Object | undefined): string | null =>
  typeof value === 'string' || typeof value === 'object' && value !== null && !Array.isArray(value) ? null : 'a string or an object';

let mustBeStringOrArray = (value: string | string[] | undefined): string | null =>
  typeof value === 'string' || Array.isArray(value) ? null : 'a string or an array';

let mustBeStringOrUint8Array = (value: string | Uint8Array | undefined): string | null =>
  typeof value === 'string' || value instanceof Uint8Array ? null : 'a string or a Uint8Array';

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

export function validateServiceOptions(options: types.ServiceOptions): types.ServiceOptions {
  let keys: OptionKeys = Object.create(null);
  let wasmURL = getFlag(options, keys, 'wasmURL', mustBeString);
  let worker = getFlag(options, keys, 'worker', mustBeBoolean);
  checkForInvalidFlags(options, keys);
  return {
    wasmURL,
    worker,
  };
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
  let minify = getFlag(options, keys, 'minify', mustBeBoolean);
  let minifySyntax = getFlag(options, keys, 'minifySyntax', mustBeBoolean);
  let minifyWhitespace = getFlag(options, keys, 'minifyWhitespace', mustBeBoolean);
  let minifyIdentifiers = getFlag(options, keys, 'minifyIdentifiers', mustBeBoolean);
  let charset = getFlag(options, keys, 'charset', mustBeString);
  let treeShaking = getFlag(options, keys, 'treeShaking', mustBeStringOrBoolean);
  let jsxFactory = getFlag(options, keys, 'jsxFactory', mustBeString);
  let jsxFragment = getFlag(options, keys, 'jsxFragment', mustBeString);
  let define = getFlag(options, keys, 'define', mustBeObject);
  let pure = getFlag(options, keys, 'pure', mustBeArray);
  let avoidTDZ = getFlag(options, keys, 'avoidTDZ', mustBeBoolean);
  let keepNames = getFlag(options, keys, 'keepNames', mustBeBoolean);
  let banner = getFlag(options, keys, 'banner', mustBeString);
  let footer = getFlag(options, keys, 'footer', mustBeString);

  if (target) {
    if (Array.isArray(target)) flags.push(`--target=${Array.from(target).map(validateTarget).join(',')}`)
    else flags.push(`--target=${validateTarget(target)}`)
  }
  if (format) flags.push(`--format=${format}`);
  if (globalName) flags.push(`--global-name=${globalName}`);

  if (minify) flags.push('--minify');
  if (minifySyntax) flags.push('--minify-syntax');
  if (minifyWhitespace) flags.push('--minify-whitespace');
  if (minifyIdentifiers) flags.push('--minify-identifiers');
  if (charset) flags.push(`--charset=${charset}`);
  if (treeShaking !== void 0 && treeShaking !== true) flags.push(`--tree-shaking=${treeShaking}`);

  if (jsxFactory) flags.push(`--jsx-factory=${jsxFactory}`);
  if (jsxFragment) flags.push(`--jsx-fragment=${jsxFragment}`);
  if (define) {
    for (let key in define) {
      if (key.indexOf('=') >= 0) throw new Error(`Invalid define: ${key}`);
      flags.push(`--define:${key}=${define[key]}`);
    }
  }
  if (pure) for (let fn of pure) flags.push(`--pure:${fn}`);
  if (avoidTDZ) flags.push(`--avoid-tdz`);
  if (keepNames) flags.push(`--keep-names`);

  if (banner) flags.push(`--banner=${banner}`);
  if (footer) flags.push(`--footer=${footer}`);
}

function flagsForBuildOptions(options: types.BuildOptions, isTTY: boolean, logLevelDefault: types.LogLevel, writeDefault: boolean):
  [string[], boolean, types.Plugin[] | undefined, string | null, string | null, boolean] {
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
  let outbase = getFlag(options, keys, 'outbase', mustBeString);
  let platform = getFlag(options, keys, 'platform', mustBeString);
  let tsconfig = getFlag(options, keys, 'tsconfig', mustBeString);
  let resolveExtensions = getFlag(options, keys, 'resolveExtensions', mustBeArray);
  let mainFields = getFlag(options, keys, 'mainFields', mustBeArray);
  let external = getFlag(options, keys, 'external', mustBeArray);
  let loader = getFlag(options, keys, 'loader', mustBeObject);
  let outExtension = getFlag(options, keys, 'outExtension', mustBeObject);
  let publicPath = getFlag(options, keys, 'publicPath', mustBeString);
  let inject = getFlag(options, keys, 'inject', mustBeArray);
  let entryPoints = getFlag(options, keys, 'entryPoints', mustBeArray);
  let stdin = getFlag(options, keys, 'stdin', mustBeObject);
  let write = getFlag(options, keys, 'write', mustBeBoolean) ?? writeDefault; // Default to true if not specified
  let incremental = getFlag(options, keys, 'incremental', mustBeBoolean) === true;
  let plugins = getFlag(options, keys, 'plugins', mustBeArray);
  checkForInvalidFlags(options, keys);

  if (sourcemap) flags.push(`--sourcemap${sourcemap === true ? '' : `=${sourcemap}`}`);
  if (bundle) flags.push('--bundle');
  if (splitting) flags.push('--splitting');
  if (metafile) flags.push(`--metafile=${metafile}`);
  if (outfile) flags.push(`--outfile=${outfile}`);
  if (outdir) flags.push(`--outdir=${outdir}`);
  if (outbase) flags.push(`--outbase=${outbase}`);
  if (platform) flags.push(`--platform=${platform}`);
  if (tsconfig) flags.push(`--tsconfig=${tsconfig}`);
  if (resolveExtensions) {
    let values: string[] = [];
    for (let value of resolveExtensions) {
      value += '';
      if (value.indexOf(',') >= 0) throw new Error(`Invalid resolve extension: ${value}`);
      values.push(value);
    }
    flags.push(`--resolve-extensions=${values.join(',')}`);
  }
  if (publicPath) flags.push(`--public-path=${publicPath}`);
  if (mainFields) {
    let values: string[] = [];
    for (let value of mainFields) {
      value += '';
      if (value.indexOf(',') >= 0) throw new Error(`Invalid main field: ${value}`);
      values.push(value);
    }
    flags.push(`--main-fields=${values.join(',')}`);
  }
  if (external) for (let name of external) flags.push(`--external:${name}`);
  if (inject) for (let path of inject) flags.push(`--inject:${path}`);
  if (loader) {
    for (let ext in loader) {
      if (ext.indexOf('=') >= 0) throw new Error(`Invalid loader extension: ${ext}`);
      flags.push(`--loader:${ext}=${loader[ext]}`);
    }
  }
  if (outExtension) {
    for (let ext in outExtension) {
      if (ext.indexOf('=') >= 0) throw new Error(`Invalid out extension: ${ext}`);
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

  return [flags, write, plugins, stdinContents, stdinResolveDir, incremental];
}

function flagsForTransformOptions(options: types.TransformOptions, isTTY: boolean, logLevelDefault: types.LogLevel): string[] {
  let flags: string[] = [];
  let keys: OptionKeys = Object.create(null);
  pushLogFlags(flags, options, keys, isTTY, logLevelDefault);
  pushCommonFlags(flags, options, keys);

  let sourcemap = getFlag(options, keys, 'sourcemap', mustBeStringOrBoolean);
  let tsconfigRaw = getFlag(options, keys, 'tsconfigRaw', mustBeStringOrObject);
  let sourcefile = getFlag(options, keys, 'sourcefile', mustBeString);
  let loader = getFlag(options, keys, 'loader', mustBeString);
  checkForInvalidFlags(options, keys);

  if (sourcemap) flags.push(`--sourcemap=${sourcemap === true ? 'external' : sourcemap}`);
  if (tsconfigRaw) flags.push(`--tsconfig-raw=${typeof tsconfigRaw === 'string' ? tsconfigRaw : JSON.stringify(tsconfigRaw)}`);
  if (sourcefile) flags.push(`--sourcefile=${sourcefile}`);
  if (loader) flags.push(`--loader=${loader}`);

  return flags;
}

export interface StreamIn {
  writeToStdin: (data: Uint8Array) => void;
  readFileSync?: (path: string, encoding: 'utf8') => string;
  isSync: boolean;
  isBrowser: boolean;
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
  buildOrServe(
    serveOptions: types.ServeOptions | null,
    options: types.BuildOptions,
    isTTY: boolean,
    callback: (err: Error | null, res: types.BuildResult | types.ServeResult | null) => void,
  ): void;

  transform(
    input: string,
    options: types.TransformOptions,
    isTTY: boolean,
    fs: StreamFS,
    callback: (err: Error | null, res: types.TransformResult | null) => void,
  ): void;
}

// This can't use any promises in the main execution flow because it must work
// for both sync and async code. There is an exception for plugin code because
// that can't work in sync code anyway.
export function createChannel(streamIn: StreamIn): StreamOut {
  type PluginCallback = (request: protocol.OnResolveRequest | protocol.OnLoadRequest) =>
    Promise<protocol.OnResolveResponse | protocol.OnLoadResponse>;

  interface ServeCallbacks {
    onRequest: types.ServeOptions['onRequest'];
    onWait: (error: string | null) => void;
  }

  let responseCallbacks = new Map<number, (error: string | null, response: protocol.Value) => void>();
  let pluginCallbacks = new Map<number, PluginCallback>();
  let serveCallbacks = new Map<number, ServeCallbacks>();
  let nextServeID = 0;
  let isClosed = false;
  let nextRequestID = 0;
  let nextBuildKey = 0;

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
    for (let callbacks of serveCallbacks.values()) {
      callbacks.onWait('The service was stopped');
    }
    serveCallbacks.clear();
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

  type RequestType =
    | protocol.OnResolveRequest
    | protocol.OnLoadRequest
    | protocol.OnRequestRequest
    | protocol.OnWaitRequest

  let handleRequest = async (id: number, request: RequestType) => {
    // Catch exceptions in the code below so they get passed to the caller
    try {
      switch (request.command) {
        case 'resolve': {
          let callback = pluginCallbacks.get(request.key);
          sendResponse(id, await callback!(request) as any);
          break;
        }

        case 'load': {
          let callback = pluginCallbacks.get(request.key);
          sendResponse(id, await callback!(request) as any);
          break;
        }

        case 'serve-request': {
          let callbacks = serveCallbacks.get(request.serveID);
          sendResponse(id, {});
          if (callbacks && callbacks.onRequest) callbacks.onRequest(request.args);
          break;
        }

        case 'serve-wait': {
          let callbacks = serveCallbacks.get(request.serveID);
          sendResponse(id, {});
          if (callbacks) callbacks.onWait(request.error);
          break;
        }

        default:
          throw new Error(`Invalid command: ` + (request as any)!.command);
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

  let handlePlugins = (plugins: types.Plugin[], request: protocol.BuildRequest, buildKey: number) => {
    if (streamIn.isSync) throw new Error('Cannot use plugins in synchronous API calls');

    let onResolveCallbacks: {
      [id: number]: (args: types.OnResolveArgs) =>
        (types.OnResolveResult | null | undefined | Promise<types.OnResolveResult | null | undefined>)
    } = {};
    let onLoadCallbacks: {
      [id: number]: (args: types.OnLoadArgs) =>
        (types.OnLoadResult | null | undefined | Promise<types.OnLoadResult | null | undefined>)
    } = {};
    let nextCallbackID = 0;
    let i = 0;

    request.plugins = [];

    for (let item of plugins) {
      let name = item.name;
      let setup = item.setup;
      let plugin: protocol.BuildPlugin = {
        name: name + '',
        onResolve: [],
        onLoad: [],
      };
      if (typeof name !== 'string' || name === '') throw new Error(`Plugin at index ${i} is missing a name`);
      if (typeof setup !== 'function') throw new Error(`[${plugin.name}] Missing a setup function`);
      i++;

      setup({
        onResolve(options, callback) {
          let keys: OptionKeys = {};
          let filter = getFlag(options, keys, 'filter', mustBeRegExp);
          let namespace = getFlag(options, keys, 'namespace', mustBeString);
          checkForInvalidFlags(options, keys);
          if (filter == null) throw new Error(`[${plugin.name}] "onResolve" is missing a filter`);
          let id = nextCallbackID++;
          onResolveCallbacks[id] = callback;
          plugin.onResolve.push({ id, filter: filter.source, namespace: namespace || '' });
        },

        onLoad(options, callback) {
          let keys: OptionKeys = {};
          let filter = getFlag(options, keys, 'filter', mustBeRegExp);
          let namespace = getFlag(options, keys, 'namespace', mustBeString);
          checkForInvalidFlags(options, keys);
          if (filter == null) throw new Error(`[${plugin.name}] "onLoad" is missing a filter`);
          let id = nextCallbackID++;
          onLoadCallbacks[id] = callback;
          plugin.onLoad.push({ id, filter: filter.source, namespace: namespace || '' });
        },
      });

      request.plugins.push(plugin);
    }

    pluginCallbacks.set(buildKey, async (request) => {
      switch (request.command) {
        case 'resolve': {
          let response: protocol.OnResolveResponse = {};
          for (let id of request.ids) {
            try {
              let callback = onResolveCallbacks[id];
              let result = await callback({
                path: request.path,
                importer: request.importer,
                namespace: request.namespace,
                resolveDir: request.resolveDir,
              });

              if (result != null) {
                if (typeof result !== 'object') throw new Error('Expected resolver plugin to return an object');
                let keys: OptionKeys = {};
                let pluginName = getFlag(result, keys, 'pluginName', mustBeString);
                let path = getFlag(result, keys, 'path', mustBeString);
                let namespace = getFlag(result, keys, 'namespace', mustBeString);
                let external = getFlag(result, keys, 'external', mustBeBoolean);
                let errors = getFlag(result, keys, 'errors', mustBeArray);
                let warnings = getFlag(result, keys, 'warnings', mustBeArray);
                checkForInvalidFlags(result, keys);

                response.id = id;
                if (pluginName != null) response.pluginName = pluginName;
                if (path != null) response.path = path;
                if (namespace != null) response.namespace = namespace;
                if (external != null) response.external = external;
                if (errors != null) response.errors = sanitizeMessages(errors);
                if (warnings != null) response.warnings = sanitizeMessages(warnings);
                break;
              }
            } catch (e) {
              return { id, errors: [await extractErrorMessageV8(e, streamIn)] };
            }
          }
          return response;
        }

        case 'load': {
          let response: protocol.OnLoadResponse = {};
          for (let id of request.ids) {
            try {
              let callback = onLoadCallbacks[id];
              let result = await callback({
                path: request.path,
                namespace: request.namespace,
              });

              if (result != null) {
                if (typeof result !== 'object') throw new Error('Expected loader plugin to return an object');
                let keys: OptionKeys = {};
                let pluginName = getFlag(result, keys, 'pluginName', mustBeString);
                let contents = getFlag(result, keys, 'contents', mustBeStringOrUint8Array);
                let resolveDir = getFlag(result, keys, 'resolveDir', mustBeString);
                let loader = getFlag(result, keys, 'loader', mustBeString);
                let errors = getFlag(result, keys, 'errors', mustBeArray);
                let warnings = getFlag(result, keys, 'warnings', mustBeArray);
                checkForInvalidFlags(result, keys);

                response.id = id;
                if (pluginName != null) response.pluginName = pluginName;
                if (contents instanceof Uint8Array) response.contents = contents;
                else if (contents != null) response.contents = protocol.encodeUTF8(contents);
                if (resolveDir != null) response.resolveDir = resolveDir;
                if (loader != null) response.loader = loader;
                if (errors != null) response.errors = sanitizeMessages(errors);
                if (warnings != null) response.warnings = sanitizeMessages(warnings);
                break;
              }
            } catch (e) {
              return { id, errors: [await extractErrorMessageV8(e, streamIn)] };
            }
          }
          return response;
        }

        default:
          throw new Error(`Invalid command: ` + (request as any).command);
      }
    });

    return () => pluginCallbacks.delete(buildKey);
  };

  interface ServeData {
    wait: Promise<void>
    stop: () => void
  }

  let buildServeData = (options: types.ServeOptions, request: protocol.BuildRequest): ServeData => {
    let keys: OptionKeys = {};
    let port = getFlag(options, keys, 'port', mustBeInteger);
    let host = getFlag(options, keys, 'host', mustBeString);
    let onRequest = getFlag(options, keys, 'onRequest', mustBeFunction);
    let serveID = nextServeID++;
    let onWait: ServeCallbacks['onWait'];
    let wait = new Promise<void>((resolve, reject) => {
      onWait = error => {
        serveCallbacks.delete(serveID);
        if (error !== null) reject(new Error(error));
        else resolve();
      };
    });
    request.serve = { serveID };
    checkForInvalidFlags(options, keys);
    if (port !== void 0) request.serve.port = port;
    if (host !== void 0) request.serve.host = host;
    serveCallbacks.set(serveID, {
      onRequest,
      onWait: onWait!,
    });
    return {
      wait,
      stop() {
        sendRequest<protocol.ServeStopRequest, null>({ command: 'serve-stop', serveID }, () => {
          // We don't care about the result
        });
      },
    };
  };

  return {
    readFromStdout,
    afterClose,

    service: {
      buildOrServe(serveOptions, options, isTTY, callback) {
        const logLevelDefault = 'info';
        try {
          let key = nextBuildKey++;
          let writeDefault = !streamIn.isBrowser;
          let [flags, write, plugins, stdin, resolveDir, incremental] = flagsForBuildOptions(options, isTTY, logLevelDefault, writeDefault);
          let request: protocol.BuildRequest = { command: 'build', key, flags, write, stdin, resolveDir, incremental };
          let serve = serveOptions && buildServeData(serveOptions, request);
          let pluginCleanup = plugins && plugins.length > 0 && handlePlugins(plugins, request, key);

          // Factor out response handling so it can be reused for rebuilds
          let rebuild: types.BuildResult['rebuild'] | undefined;
          let buildResponseToResult = (
            response: protocol.BuildResponse | null,
            callback: (error: Error | null, result: types.BuildResult | null) => void,
          ): void => {
            let errors = response!.errors;
            let warnings = response!.warnings;
            if (errors.length > 0) return callback(failureErrorWithLog('Build failed', errors, warnings), null);
            let result: types.BuildResult = { warnings };
            if (response!.outputFiles) result.outputFiles = response!.outputFiles.map(convertOutputFiles);

            // Handle incremental rebuilds
            if (response!.rebuildID !== void 0) {
              if (!rebuild) {
                let isDisposed = false;
                (rebuild as any) = () => new Promise<types.BuildResult>((resolve, reject) => {
                  if (isDisposed || isClosed) throw new Error('Cannot rebuild');
                  sendRequest<protocol.RebuildRequest, protocol.BuildResponse>({ command: 'rebuild', rebuildID: response!.rebuildID! },
                    (error2, response2) => {
                      if (error2) return callback(new Error(error2), null);
                      buildResponseToResult(response2, (error3, result3) => {
                        if (error3) reject(error3);
                        else resolve(result3!);
                      });
                    });
                });
                rebuild!.dispose = () => {
                  isDisposed = true;
                  if (pluginCleanup) pluginCleanup();
                  sendRequest<protocol.RebuildDisposeRequest, null>({ command: 'rebuild-dispose', rebuildID: response!.rebuildID! }, () => {
                    // We don't care about the result
                  });
                };
              }
              result.rebuild = rebuild;
            }
            return callback(null, result);
          };

          if (write && streamIn.isBrowser) throw new Error(`Cannot enable "write" in the browser`);
          if (incremental && streamIn.isSync) throw new Error(`Cannot use "incremental" with a synchronous build`);
          sendRequest<protocol.BuildRequest, protocol.BuildResponse>(request, (error, response) => {
            if (error) return callback(new Error(error), null);
            if (serve) {
              let serveResponse = response as any as protocol.ServeResponse;
              let result: types.ServeResult = {
                port: serveResponse.port,
                host: serveResponse.host,
                wait: serve.wait,
                stop: serve.stop,
              };
              return callback(null, result);
            }
            if (pluginCleanup && !incremental) pluginCleanup();
            return buildResponseToResult(response!, callback);
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
              input: inputPath !== null ? inputPath : input,
            };
            sendRequest<protocol.TransformRequest, protocol.TransformResponse>(request, (error, response) => {
              if (error) return callback(new Error(error), null);
              let errors = response!.errors;
              let warnings = response!.warnings;
              let outstanding = 1;
              let next = () => --outstanding === 0 && callback(null, { warnings, code: response!.code, map: response!.map });
              if (errors.length > 0) return callback(failureErrorWithLog('Transform failed', errors, warnings), null);

              // Read the JavaScript file from the file system
              if (response!.codeFS) {
                outstanding++;
                fs.readFile(response!.code, (err, contents) => {
                  if (err !== null) {
                    callback(err, null);
                  } else {
                    response!.code = contents!;
                    next();
                  }
                });
              }

              // Read the source map file from the file system
              if (response!.mapFS) {
                outstanding++;
                fs.readFile(response!.map, (err, contents) => {
                  if (err !== null) {
                    callback(err, null);
                  } else {
                    response!.map = contents!;
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
        if (input.length > 1024 * 1024) {
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
            namespace: 'file',
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

function sanitizeMessages(messages: types.PartialMessage[]): types.Message[] {
  let messagesClone: types.Message[] = [];

  for (const message of messages) {
    let keys: OptionKeys = {};
    let text = getFlag(message, keys, 'text', mustBeString);
    let location = getFlag(message, keys, 'location', mustBeObjectOrNull);
    checkForInvalidFlags(message, keys);

    let locationClone: types.Message['location'] = null;
    if (location != null) {
      let keys: OptionKeys = {};
      let file = getFlag(location, keys, 'file', mustBeString);
      let namespace = getFlag(location, keys, 'namespace', mustBeString);
      let line = getFlag(location, keys, 'line', mustBeInteger);
      let column = getFlag(location, keys, 'column', mustBeInteger);
      let length = getFlag(location, keys, 'length', mustBeInteger);
      let lineText = getFlag(location, keys, 'lineText', mustBeString);
      checkForInvalidFlags(location, keys);

      locationClone = {
        file: file || '',
        namespace: namespace || '',
        line: line || 0,
        column: column || 0,
        length: length || 0,
        lineText: lineText || '',
      };
    }

    messagesClone.push({
      text: text || '',
      location: locationClone,
    });
  }

  return messagesClone;
}

function convertOutputFiles({ path, contents }: protocol.BuildOutputFile): types.OutputFile {
  let text: string | null = null;
  return {
    path,
    contents,
    get text() {
      if (text === null) text = protocol.decodeUTF8(contents);
      return text;
    },
  }
}
