import * as types from "./types";
import * as protocol from "./stdio_protocol";

declare const ESBUILD_VERSION: string;

function validateTarget(target: string): string {
  target += ''
  if (target.indexOf(',') >= 0) throw new Error(`Invalid target: ${target}`)
  return target
}

let canBeAnything = () => null;

let mustBeBoolean = (value: boolean | undefined): string | null =>
  typeof value === 'boolean' ? null : 'a boolean';

let mustBeBooleanOrObject = (value: Object | boolean | undefined): string | null =>
  typeof value === 'boolean' || (typeof value === 'object' && !Array.isArray(value)) ? null : 'a boolean or an object';

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

let mustBeArrayOrRecord = <T extends string>(value: T[] | Record<T, T> | undefined): string | null =>
  typeof value === 'object' && value !== null ? null : 'an array or an object';

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

function checkForInvalidFlags(object: Object, keys: OptionKeys, where: string): void {
  for (let key in object) {
    if (!(key in keys)) {
      throw new Error(`Invalid option ${where}: "${key}"`);
    }
  }
}

export function validateInitializeOptions(options: types.InitializeOptions): types.InitializeOptions {
  let keys: OptionKeys = Object.create(null);
  let wasmURL = getFlag(options, keys, 'wasmURL', mustBeString);
  let worker = getFlag(options, keys, 'worker', mustBeBoolean);
  checkForInvalidFlags(options, keys, 'in startService() call');
  return {
    wasmURL,
    worker,
  };
}

type CommonOptions = types.BuildOptions | types.TransformOptions;

function pushLogFlags(flags: string[], options: CommonOptions, keys: OptionKeys, isTTY: boolean, logLevelDefault: types.LogLevel): void {
  let color = getFlag(options, keys, 'color', mustBeBoolean);
  let logLevel = getFlag(options, keys, 'logLevel', mustBeString);
  let logLimit = getFlag(options, keys, 'logLimit', mustBeInteger);

  if (color) flags.push(`--color=${color}`);
  else if (isTTY) flags.push(`--color=true`); // This is needed to fix "execFileSync" which buffers stderr
  flags.push(`--log-level=${logLevel || logLevelDefault}`);
  flags.push(`--log-limit=${logLimit || 0}`);
}

function pushCommonFlags(flags: string[], options: CommonOptions, keys: OptionKeys): void {
  let legalComments = getFlag(options, keys, 'legalComments', mustBeString);
  let sourceRoot = getFlag(options, keys, 'sourceRoot', mustBeString);
  let sourcesContent = getFlag(options, keys, 'sourcesContent', mustBeBoolean);
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
  let keepNames = getFlag(options, keys, 'keepNames', mustBeBoolean);

  if (legalComments) flags.push(`--legal-comments=${legalComments}`);
  if (sourceRoot !== void 0) flags.push(`--source-root=${sourceRoot}`);
  if (sourcesContent !== void 0) flags.push(`--sources-content=${sourcesContent}`);
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
  if (keepNames) flags.push(`--keep-names`);
}

function flagsForBuildOptions(
  callName: string,
  options: types.BuildOptions,
  isTTY: boolean,
  logLevelDefault: types.LogLevel,
  writeDefault: boolean,
): {
  entries: [string, string][],
  flags: string[],
  write: boolean,
  stdinContents: string | null,
  stdinResolveDir: string | null,
  absWorkingDir: string | undefined,
  incremental: boolean,
  nodePaths: string[],
  watch: types.WatchMode | null,
} {
  let flags: string[] = [];
  let entries: [string, string][] = [];
  let keys: OptionKeys = Object.create(null);
  let stdinContents: string | null = null;
  let stdinResolveDir: string | null = null;
  let watchMode: types.WatchMode | null = null;
  pushLogFlags(flags, options, keys, isTTY, logLevelDefault);
  pushCommonFlags(flags, options, keys);

  let sourcemap = getFlag(options, keys, 'sourcemap', mustBeStringOrBoolean);
  let bundle = getFlag(options, keys, 'bundle', mustBeBoolean);
  let watch = getFlag(options, keys, 'watch', mustBeBooleanOrObject);
  let splitting = getFlag(options, keys, 'splitting', mustBeBoolean);
  let preserveSymlinks = getFlag(options, keys, 'preserveSymlinks', mustBeBoolean);
  let metafile = getFlag(options, keys, 'metafile', mustBeBoolean);
  let outfile = getFlag(options, keys, 'outfile', mustBeString);
  let outdir = getFlag(options, keys, 'outdir', mustBeString);
  let outbase = getFlag(options, keys, 'outbase', mustBeString);
  let platform = getFlag(options, keys, 'platform', mustBeString);
  let tsconfig = getFlag(options, keys, 'tsconfig', mustBeString);
  let resolveExtensions = getFlag(options, keys, 'resolveExtensions', mustBeArray);
  let nodePathsInput = getFlag(options, keys, 'nodePaths', mustBeArray);
  let mainFields = getFlag(options, keys, 'mainFields', mustBeArray);
  let conditions = getFlag(options, keys, 'conditions', mustBeArray);
  let external = getFlag(options, keys, 'external', mustBeArray);
  let loader = getFlag(options, keys, 'loader', mustBeObject);
  let outExtension = getFlag(options, keys, 'outExtension', mustBeObject);
  let publicPath = getFlag(options, keys, 'publicPath', mustBeString);
  let entryNames = getFlag(options, keys, 'entryNames', mustBeString);
  let chunkNames = getFlag(options, keys, 'chunkNames', mustBeString);
  let assetNames = getFlag(options, keys, 'assetNames', mustBeString);
  let inject = getFlag(options, keys, 'inject', mustBeArray);
  let banner = getFlag(options, keys, 'banner', mustBeObject);
  let footer = getFlag(options, keys, 'footer', mustBeObject);
  let entryPoints = getFlag(options, keys, 'entryPoints', mustBeArrayOrRecord);
  let absWorkingDir = getFlag(options, keys, 'absWorkingDir', mustBeString);
  let stdin = getFlag(options, keys, 'stdin', mustBeObject);
  let write = getFlag(options, keys, 'write', mustBeBoolean) ?? writeDefault; // Default to true if not specified
  let allowOverwrite = getFlag(options, keys, 'allowOverwrite', mustBeBoolean);
  let incremental = getFlag(options, keys, 'incremental', mustBeBoolean) === true;
  keys.plugins = true; // "plugins" has already been read earlier
  checkForInvalidFlags(options, keys, `in ${callName}() call`);

  if (sourcemap) flags.push(`--sourcemap${sourcemap === true ? '' : `=${sourcemap}`}`);
  if (bundle) flags.push('--bundle');
  if (allowOverwrite) flags.push('--allow-overwrite');
  if (watch) {
    flags.push('--watch');
    if (typeof watch === 'boolean') {
      watchMode = {};
    } else {
      let watchKeys: OptionKeys = Object.create(null);
      let onRebuild = getFlag(watch, watchKeys, 'onRebuild', mustBeFunction);
      checkForInvalidFlags(watch, watchKeys, `on "watch" in ${callName}() call`);
      watchMode = { onRebuild };
    }
  }
  if (splitting) flags.push('--splitting');
  if (preserveSymlinks) flags.push('--preserve-symlinks');
  if (metafile) flags.push(`--metafile`);
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
  if (entryNames) flags.push(`--entry-names=${entryNames}`);
  if (chunkNames) flags.push(`--chunk-names=${chunkNames}`);
  if (assetNames) flags.push(`--asset-names=${assetNames}`);
  if (mainFields) {
    let values: string[] = [];
    for (let value of mainFields) {
      value += '';
      if (value.indexOf(',') >= 0) throw new Error(`Invalid main field: ${value}`);
      values.push(value);
    }
    flags.push(`--main-fields=${values.join(',')}`);
  }
  if (conditions) {
    let values: string[] = [];
    for (let value of conditions) {
      value += '';
      if (value.indexOf(',') >= 0) throw new Error(`Invalid condition: ${value}`);
      values.push(value);
    }
    flags.push(`--conditions=${values.join(',')}`);
  }
  if (external) for (let name of external) flags.push(`--external:${name}`);
  if (banner) {
    for (let type in banner) {
      if (type.indexOf('=') >= 0) throw new Error(`Invalid banner file type: ${type}`);
      flags.push(`--banner:${type}=${banner[type]}`);
    }
  }
  if (footer) {
    for (let type in footer) {
      if (type.indexOf('=') >= 0) throw new Error(`Invalid footer file type: ${type}`);
      flags.push(`--footer:${type}=${footer[type]}`);
    }
  }
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
    if (Array.isArray(entryPoints)) {
      for (let entryPoint of entryPoints) {
        entries.push(['', entryPoint + '']);
      }
    } else {
      for (let [key, value] of Object.entries(entryPoints)) {
        entries.push([key + '', value + '']);
      }
    }
  }

  if (stdin) {
    let stdinKeys: OptionKeys = Object.create(null);
    let contents = getFlag(stdin, stdinKeys, 'contents', mustBeString);
    let resolveDir = getFlag(stdin, stdinKeys, 'resolveDir', mustBeString);
    let sourcefile = getFlag(stdin, stdinKeys, 'sourcefile', mustBeString);
    let loader = getFlag(stdin, stdinKeys, 'loader', mustBeString);
    checkForInvalidFlags(stdin, stdinKeys, 'in "stdin" object');

    if (sourcefile) flags.push(`--sourcefile=${sourcefile}`);
    if (loader) flags.push(`--loader=${loader}`);
    if (resolveDir) stdinResolveDir = resolveDir + '';
    stdinContents = contents ? contents + '' : '';
  }

  let nodePaths: string[] = [];
  if (nodePathsInput) {
    for (let value of nodePathsInput) {
      value += '';
      nodePaths.push(value);
    }
  }

  return {
    entries,
    flags,
    write,
    stdinContents,
    stdinResolveDir,
    absWorkingDir,
    incremental,
    nodePaths,
    watch: watchMode,
  };
}

function flagsForTransformOptions(
  callName: string,
  options: types.TransformOptions,
  isTTY: boolean,
  logLevelDefault: types.LogLevel,
): string[] {
  let flags: string[] = [];
  let keys: OptionKeys = Object.create(null);
  pushLogFlags(flags, options, keys, isTTY, logLevelDefault);
  pushCommonFlags(flags, options, keys);

  let sourcemap = getFlag(options, keys, 'sourcemap', mustBeStringOrBoolean);
  let tsconfigRaw = getFlag(options, keys, 'tsconfigRaw', mustBeStringOrObject);
  let sourcefile = getFlag(options, keys, 'sourcefile', mustBeString);
  let loader = getFlag(options, keys, 'loader', mustBeString);
  let banner = getFlag(options, keys, 'banner', mustBeString);
  let footer = getFlag(options, keys, 'footer', mustBeString);
  checkForInvalidFlags(options, keys, `in ${callName}() call`);

  if (sourcemap) flags.push(`--sourcemap=${sourcemap === true ? 'external' : sourcemap}`);
  if (tsconfigRaw) flags.push(`--tsconfig-raw=${typeof tsconfigRaw === 'string' ? tsconfigRaw : JSON.stringify(tsconfigRaw)}`);
  if (sourcefile) flags.push(`--sourcefile=${sourcefile}`);
  if (loader) flags.push(`--loader=${loader}`);
  if (banner) flags.push(`--banner=${banner}`);
  if (footer) flags.push(`--footer=${footer}`);

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

export interface Refs {
  ref(): void;
  unref(): void;
}

export interface StreamService {
  buildOrServe(args: {
    callName: string,
    refs: Refs | null,
    serveOptions: types.ServeOptions | null,
    options: types.BuildOptions,
    isTTY: boolean,
    defaultWD: string,
    callback: (err: Error | null, res: types.BuildResult | types.ServeResult | null) => void,
  }): void;

  transform(args: {
    callName: string,
    refs: Refs | null,
    input: string,
    options: types.TransformOptions,
    isTTY: boolean,
    fs: StreamFS,
    callback: (err: Error | null, res: types.TransformResult | null) => void,
  }): void;

  formatMessages(args: {
    callName: string,
    refs: Refs | null,
    messages: types.PartialMessage[],
    options: types.FormatMessagesOptions,
    callback: (err: Error | null, res: string[] | null) => void,
  }): void;
}

// This can't use any promises in the main execution flow because it must work
// for both sync and async code. There is an exception for plugin code because
// that can't work in sync code anyway.
export function createChannel(streamIn: StreamIn): StreamOut {
  type PluginCallback = (request: protocol.OnStartRequest | protocol.OnResolveRequest | protocol.OnLoadRequest) =>
    Promise<protocol.OnStartResponse | protocol.OnResolveResponse | protocol.OnLoadResponse>;

  type WatchCallback = (error: Error | null, response: any) => void;

  interface ServeCallbacks {
    onRequest: types.ServeOptions['onRequest'];
    onWait: (error: string | null) => void;
  }

  let responseCallbacks = new Map<number, (error: string | null, response: protocol.Value) => void>();
  let pluginCallbacks = new Map<number, PluginCallback>();
  let watchCallbacks = new Map<number, WatchCallback>();
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
      handleIncomingPacket(stdout.subarray(offset, offset + length));
      offset += length;
    }
    if (offset > 0) {
      stdout.copyWithin(0, offset, stdoutUsed);
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
    for (let callback of watchCallbacks.values()) {
      try {
        callback(new Error('The service was stopped'), null);
      } catch (e) {
        console.error(e)
      }
    }
    watchCallbacks.clear();
  };

  let sendRequest = <Req, Res>(refs: Refs | null, value: Req, callback: (error: string | null, response: Res | null) => void): void => {
    if (isClosed) return callback('The service is no longer running', null);
    let id = nextRequestID++;
    responseCallbacks.set(id, (error, response) => {
      try {
        callback(error, response as any);
      } finally {
        if (refs) refs.unref() // Do this after the callback so the callback can extend the lifetime if needed
      }
    });
    if (refs) refs.ref()
    streamIn.writeToStdin(protocol.encodePacket({ id, isRequest: true, value: value as any }));
  };

  let sendResponse = (id: number, value: protocol.Value): void => {
    if (isClosed) throw new Error('The service is no longer running');
    streamIn.writeToStdin(protocol.encodePacket({ id, isRequest: false, value }));
  };

  type RequestType =
    | protocol.PingRequest
    | protocol.OnStartRequest
    | protocol.OnResolveRequest
    | protocol.OnLoadRequest
    | protocol.OnRequestRequest
    | protocol.OnWaitRequest
    | protocol.OnWatchRebuildRequest

  let handleRequest = async (id: number, request: RequestType) => {
    // Catch exceptions in the code below so they get passed to the caller
    try {
      switch (request.command) {
        case 'ping': {
          sendResponse(id, {});
          break;
        }

        case 'start': {
          let callback = pluginCallbacks.get(request.key);
          if (!callback) sendResponse(id, {});
          else sendResponse(id, await callback!(request) as any);
          break;
        }

        case 'resolve': {
          let callback = pluginCallbacks.get(request.key);
          if (!callback) sendResponse(id, {});
          else sendResponse(id, await callback!(request) as any);
          break;
        }

        case 'load': {
          let callback = pluginCallbacks.get(request.key);
          if (!callback) sendResponse(id, {});
          else sendResponse(id, await callback!(request) as any);
          break;
        }

        case 'serve-request': {
          let callbacks = serveCallbacks.get(request.serveID);
          if (callbacks && callbacks.onRequest) callbacks.onRequest(request.args);
          sendResponse(id, {});
          break;
        }

        case 'serve-wait': {
          let callbacks = serveCallbacks.get(request.serveID);
          if (callbacks) callbacks.onWait(request.error);
          sendResponse(id, {});
          break;
        }

        case 'watch-rebuild': {
          let callback = watchCallbacks.get(request.watchID);
          try {
            if (callback) callback(null, request.args);
          } catch (err) {
            console.error(err);
          }
          sendResponse(id, {});
          break;
        }

        default:
          throw new Error(`Invalid command: ` + (request as any)!.command);
      }
    } catch (e) {
      sendResponse(id, { errors: [extractErrorMessageV8(e, streamIn, null, void 0, '')] } as any);
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

  type RunOnEndCallbacks = (result: types.BuildResult, done: () => void) => void;

  let handlePlugins = async (
    initialOptions: types.BuildOptions,
    plugins: types.Plugin[],
    buildKey: number,
    stash: ObjectStash,
  ): Promise<
    | { ok: true, requestPlugins: protocol.BuildPlugin[], runOnEndCallbacks: RunOnEndCallbacks, pluginRefs: Refs }
    | { ok: false, error: any, pluginName: string }
  > => {
    let onStartCallbacks: {
      name: string,
      note: () => types.Note | undefined,
      callback: () => (types.OnStartResult | null | undefined | Promise<types.OnStartResult | null | undefined>),
    }[] = [];

    let onEndCallbacks: {
      name: string,
      note: () => types.Note | undefined,
      callback: (result: types.BuildResult) => (undefined | Promise<undefined>),
    }[] = [];

    let onResolveCallbacks: {
      [id: number]: {
        name: string,
        note: () => types.Note | undefined,
        callback: (args: types.OnResolveArgs) =>
          (types.OnResolveResult | null | undefined | Promise<types.OnResolveResult | null | undefined>),
      },
    } = {};

    let onLoadCallbacks: {
      [id: number]: {
        name: string,
        note: () => types.Note | undefined,
        callback: (args: types.OnLoadArgs) =>
          (types.OnLoadResult | null | undefined | Promise<types.OnLoadResult | null | undefined>),
      },
    } = {};

    let nextCallbackID = 0;
    let i = 0;
    let requestPlugins: protocol.BuildPlugin[] = [];

    // Clone the plugin array to guard against mutation during iteration
    plugins = [...plugins];

    for (let item of plugins) {
      let keys: OptionKeys = {};
      if (typeof item !== 'object') throw new Error(`Plugin at index ${i} must be an object`);
      let name = getFlag(item, keys, 'name', mustBeString);
      if (typeof name !== 'string' || name === '') throw new Error(`Plugin at index ${i} is missing a name`);
      try {
        let setup = getFlag(item, keys, 'setup', mustBeFunction);
        if (typeof setup !== 'function') throw new Error(`Plugin is missing a setup function`);
        checkForInvalidFlags(item, keys, `on plugin ${JSON.stringify(name)}`);

        let plugin: protocol.BuildPlugin = {
          name,
          onResolve: [],
          onLoad: [],
        };
        i++;

        let promise = setup({
          initialOptions,

          onStart(callback) {
            let registeredText = `This error came from the "onStart" callback registered here`
            let registeredNote = extractCallerV8(new Error(registeredText), streamIn, 'onStart');
            onStartCallbacks.push({ name: name!, callback, note: registeredNote });
          },

          onEnd(callback) {
            let registeredText = `This error came from the "onEnd" callback registered here`
            let registeredNote = extractCallerV8(new Error(registeredText), streamIn, 'onEnd');
            onEndCallbacks.push({ name: name!, callback, note: registeredNote });
          },

          onResolve(options, callback) {
            let registeredText = `This error came from the "onResolve" callback registered here`
            let registeredNote = extractCallerV8(new Error(registeredText), streamIn, 'onResolve');
            let keys: OptionKeys = {};
            let filter = getFlag(options, keys, 'filter', mustBeRegExp);
            let namespace = getFlag(options, keys, 'namespace', mustBeString);
            checkForInvalidFlags(options, keys, `in onResolve() call for plugin ${JSON.stringify(name)}`);
            if (filter == null) throw new Error(`onResolve() call is missing a filter`);
            let id = nextCallbackID++;
            onResolveCallbacks[id] = { name: name!, callback, note: registeredNote };
            plugin.onResolve.push({ id, filter: filter.source, namespace: namespace || '' });
          },

          onLoad(options, callback) {
            let registeredText = `This error came from the "onLoad" callback registered here`
            let registeredNote = extractCallerV8(new Error(registeredText), streamIn, 'onLoad');
            let keys: OptionKeys = {};
            let filter = getFlag(options, keys, 'filter', mustBeRegExp);
            let namespace = getFlag(options, keys, 'namespace', mustBeString);
            checkForInvalidFlags(options, keys, `in onLoad() call for plugin ${JSON.stringify(name)}`);
            if (filter == null) throw new Error(`onLoad() call is missing a filter`);
            let id = nextCallbackID++;
            onLoadCallbacks[id] = { name: name!, callback, note: registeredNote };
            plugin.onLoad.push({ id, filter: filter.source, namespace: namespace || '' });
          },
        });

        // Await a returned promise if there was one. This allows plugins to do
        // some asynchronous setup while still retaining the ability to modify
        // the build options. This deliberately serializes asynchronous plugin
        // setup instead of running them concurrently so that build option
        // modifications are easier to reason about.
        if (promise) await promise;

        requestPlugins.push(plugin);
      } catch (e) {
        return { ok: false, error: e, pluginName: name }
      }
    }

    const callback: PluginCallback = async (request) => {
      switch (request.command) {
        case 'start': {
          let response: protocol.OnStartResponse = { errors: [], warnings: [] };
          await Promise.all(onStartCallbacks.map(async ({ name, callback, note }) => {
            try {
              let result = await callback();

              if (result != null) {
                if (typeof result !== 'object') throw new Error(`Expected onStart() callback in plugin ${JSON.stringify(name)} to return an object`);
                let keys: OptionKeys = {};
                let errors = getFlag(result, keys, 'errors', mustBeArray);
                let warnings = getFlag(result, keys, 'warnings', mustBeArray);
                checkForInvalidFlags(result, keys, `from onStart() callback in plugin ${JSON.stringify(name)}`);

                if (errors != null) response.errors!.push(...sanitizeMessages(errors, 'errors', stash, name));
                if (warnings != null) response.warnings!.push(...sanitizeMessages(warnings, 'warnings', stash, name));
              }
            } catch (e) {
              response.errors!.push(extractErrorMessageV8(e, streamIn, stash, note && note(), name));
            }
          }))
          return response;
        }

        case 'resolve': {
          let response: protocol.OnResolveResponse = {}, name = '', callback, note;
          for (let id of request.ids) {
            try {
              ({ name, callback, note } = onResolveCallbacks[id]);
              let result = await callback({
                path: request.path,
                importer: request.importer,
                namespace: request.namespace,
                resolveDir: request.resolveDir,
                kind: request.kind,
                pluginData: stash.load(request.pluginData),
              });

              if (result != null) {
                if (typeof result !== 'object') throw new Error(`Expected onResolve() callback in plugin ${JSON.stringify(name)} to return an object`);
                let keys: OptionKeys = {};
                let pluginName = getFlag(result, keys, 'pluginName', mustBeString);
                let path = getFlag(result, keys, 'path', mustBeString);
                let namespace = getFlag(result, keys, 'namespace', mustBeString);
                let external = getFlag(result, keys, 'external', mustBeBoolean);
                let pluginData = getFlag(result, keys, 'pluginData', canBeAnything);
                let errors = getFlag(result, keys, 'errors', mustBeArray);
                let warnings = getFlag(result, keys, 'warnings', mustBeArray);
                let watchFiles = getFlag(result, keys, 'watchFiles', mustBeArray);
                let watchDirs = getFlag(result, keys, 'watchDirs', mustBeArray);
                checkForInvalidFlags(result, keys, `from onResolve() callback in plugin ${JSON.stringify(name)}`);

                response.id = id;
                if (pluginName != null) response.pluginName = pluginName;
                if (path != null) response.path = path;
                if (namespace != null) response.namespace = namespace;
                if (external != null) response.external = external;
                if (pluginData != null) response.pluginData = stash.store(pluginData);
                if (errors != null) response.errors = sanitizeMessages(errors, 'errors', stash, name);
                if (warnings != null) response.warnings = sanitizeMessages(warnings, 'warnings', stash, name);
                if (watchFiles != null) response.watchFiles = sanitizeStringArray(watchFiles, 'watchFiles');
                if (watchDirs != null) response.watchDirs = sanitizeStringArray(watchDirs, 'watchDirs');
                break;
              }
            } catch (e) {
              return { id, errors: [extractErrorMessageV8(e, streamIn, stash, note && note(), name)] };
            }
          }
          return response;
        }

        case 'load': {
          let response: protocol.OnLoadResponse = {}, name = '', callback, note;
          for (let id of request.ids) {
            try {
              ({ name, callback, note } = onLoadCallbacks[id]);
              let result = await callback({
                path: request.path,
                namespace: request.namespace,
                pluginData: stash.load(request.pluginData),
              });

              if (result != null) {
                if (typeof result !== 'object') throw new Error(`Expected onLoad() callback in plugin ${JSON.stringify(name)} to return an object`);
                let keys: OptionKeys = {};
                let pluginName = getFlag(result, keys, 'pluginName', mustBeString);
                let contents = getFlag(result, keys, 'contents', mustBeStringOrUint8Array);
                let resolveDir = getFlag(result, keys, 'resolveDir', mustBeString);
                let pluginData = getFlag(result, keys, 'pluginData', canBeAnything);
                let loader = getFlag(result, keys, 'loader', mustBeString);
                let errors = getFlag(result, keys, 'errors', mustBeArray);
                let warnings = getFlag(result, keys, 'warnings', mustBeArray);
                let watchFiles = getFlag(result, keys, 'watchFiles', mustBeArray);
                let watchDirs = getFlag(result, keys, 'watchDirs', mustBeArray);
                checkForInvalidFlags(result, keys, `from onLoad() callback in plugin ${JSON.stringify(name)}`);

                response.id = id;
                if (pluginName != null) response.pluginName = pluginName;
                if (contents instanceof Uint8Array) response.contents = contents;
                else if (contents != null) response.contents = protocol.encodeUTF8(contents);
                if (resolveDir != null) response.resolveDir = resolveDir;
                if (pluginData != null) response.pluginData = stash.store(pluginData);
                if (loader != null) response.loader = loader;
                if (errors != null) response.errors = sanitizeMessages(errors, 'errors', stash, name);
                if (warnings != null) response.warnings = sanitizeMessages(warnings, 'warnings', stash, name);
                if (watchFiles != null) response.watchFiles = sanitizeStringArray(watchFiles, 'watchFiles');
                if (watchDirs != null) response.watchDirs = sanitizeStringArray(watchDirs, 'watchDirs');
                break;
              }
            } catch (e) {
              return { id, errors: [extractErrorMessageV8(e, streamIn, stash, note && note(), name)] };
            }
          }
          return response;
        }

        default:
          throw new Error(`Invalid command: ` + (request as any).command);
      }
    }

    let runOnEndCallbacks: RunOnEndCallbacks = (result, done) => done();

    if (onEndCallbacks.length > 0) {
      runOnEndCallbacks = (result, done) => {
        (async () => {
          for (const { name, callback, note } of onEndCallbacks) {
            try {
              await callback(result)
            } catch (e) {
              result.errors.push(extractErrorMessageV8(e, streamIn, stash, note && note(), name))
            }
          }
        })().then(done)
      }
    }

    let refCount = 0;
    return {
      ok: true,
      requestPlugins,
      runOnEndCallbacks,
      pluginRefs: {
        ref() { if (++refCount === 1) pluginCallbacks.set(buildKey, callback); },
        unref() { if (--refCount === 0) pluginCallbacks.delete(buildKey) },
      },
    }
  };

  interface ServeData {
    wait: Promise<void>
    stop: () => void
  }

  let buildServeData = (refs: Refs | null, options: types.ServeOptions, request: protocol.BuildRequest): ServeData => {
    let keys: OptionKeys = {};
    let port = getFlag(options, keys, 'port', mustBeInteger);
    let host = getFlag(options, keys, 'host', mustBeString);
    let servedir = getFlag(options, keys, 'servedir', mustBeString);
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
    checkForInvalidFlags(options, keys, `in serve() call`);
    if (port !== void 0) request.serve.port = port;
    if (host !== void 0) request.serve.host = host;
    if (servedir !== void 0) request.serve.servedir = servedir;
    serveCallbacks.set(serveID, {
      onRequest,
      onWait: onWait!,
    });
    return {
      wait,
      stop() {
        sendRequest<protocol.ServeStopRequest, null>(refs, { command: 'serve-stop', serveID }, () => {
          // We don't care about the result
        });
      },
    };
  };

  const buildLogLevelDefault = 'warning';
  const transformLogLevelDefault = 'silent';

  let buildOrServe: StreamService['buildOrServe'] = args => {
    let key = nextBuildKey++;
    const details = createObjectStash();
    let plugins: types.Plugin[] | undefined;
    let { refs, options, isTTY, callback } = args;
    if (typeof options === 'object') {
      let value = options.plugins;
      if (value !== void 0) {
        if (!Array.isArray(value)) throw new Error(`"plugins" must be an array`);
        plugins = value;
      }
    }
    let handleError = (e: any, pluginName: string) => {
      let flags: string[] = [];
      try { pushLogFlags(flags, options, {}, isTTY, buildLogLevelDefault) } catch { }
      const error = extractErrorMessageV8(e, streamIn, details, void 0, pluginName)
      sendRequest(refs, { command: 'error', flags, error }, () => {
        error.detail = details.load(error.detail);
        callback(failureErrorWithLog('Build failed', [error], []), null);
      });
    };
    if (plugins && plugins.length > 0) {
      if (streamIn.isSync) return handleError(new Error('Cannot use plugins in synchronous API calls'), '');

      // Plugins can use async/await because they can't be run with "buildSync"
      handlePlugins(options, plugins, key, details).then(
        result => {
          if (!result.ok) {
            handleError(result.error, result.pluginName);
          } else {
            try {
              buildOrServeContinue({
                ...args,
                key,
                details,
                requestPlugins: result.requestPlugins,
                runOnEndCallbacks: result.runOnEndCallbacks,
                pluginRefs: result.pluginRefs,
              })
            } catch (e) {
              handleError(e, '');
            }
          }
        },
        e => handleError(e, ''),
      )
    } else {
      try {
        buildOrServeContinue({
          ...args,
          key,
          details,
          requestPlugins: null,
          runOnEndCallbacks: (result, done) => done(),
          pluginRefs: null,
        });
      } catch (e) {
        handleError(e, '');
      }
    }
  }

  // "buildOrServe" cannot be written using async/await due to "buildSync" and
  // must be written in continuation-passing style instead. Sorry about all of
  // the arguments, but these are passed explicitly instead of using another
  // nested closure because this function is already huge and I didn't want to
  // make it any bigger.
  let buildOrServeContinue = ({
    callName,
    refs: callerRefs,
    serveOptions,
    options,
    isTTY,
    defaultWD,
    callback,
    key,
    details,
    requestPlugins,
    runOnEndCallbacks,
    pluginRefs,
  }: {
    callName: string,
    refs: Refs | null,
    serveOptions: types.ServeOptions | null,
    options: types.BuildOptions,
    isTTY: boolean,
    defaultWD: string,
    callback: (err: Error | null, res: types.BuildResult | types.ServeResult | null) => void,
    key: number,
    details: ObjectStash,
    requestPlugins: protocol.BuildPlugin[] | null,
    runOnEndCallbacks: RunOnEndCallbacks,
    pluginRefs: Refs | null,
  }) => {
    const refs = {
      ref() {
        if (pluginRefs) pluginRefs.ref()
        if (callerRefs) callerRefs.ref()
      },
      unref() {
        if (pluginRefs) pluginRefs.unref()
        if (callerRefs) callerRefs.unref()
      },
    }
    let writeDefault = !streamIn.isBrowser;
    let {
      entries,
      flags,
      write,
      stdinContents,
      stdinResolveDir,
      absWorkingDir,
      incremental,
      nodePaths,
      watch,
    } = flagsForBuildOptions(callName, options, isTTY, buildLogLevelDefault, writeDefault);
    let request: protocol.BuildRequest = {
      command: 'build',
      key,
      entries,
      flags,
      write,
      stdinContents,
      stdinResolveDir,
      absWorkingDir: absWorkingDir || defaultWD,
      incremental,
      nodePaths,
      hasOnRebuild: !!(watch && watch.onRebuild),
    };
    if (requestPlugins) request.plugins = requestPlugins;
    let serve = serveOptions && buildServeData(refs, serveOptions, request);

    // Factor out response handling so it can be reused for rebuilds
    let rebuild: types.BuildResult['rebuild'] | undefined;
    let stop: types.BuildResult['stop'] | undefined;
    let copyResponseToResult = (response: protocol.BuildResponse, result: types.BuildResult) => {
      if (response.outputFiles) result.outputFiles = response!.outputFiles.map(convertOutputFiles);
      if (response.metafile) result.metafile = JSON.parse(response!.metafile);
      if (response.writeToStdout !== void 0) console.log(protocol.decodeUTF8(response!.writeToStdout).replace(/\n$/, ''));
    };
    let buildResponseToResult = (
      response: protocol.BuildResponse | null,
      callback: (error: types.BuildFailure | null, result: types.BuildResult | null) => void,
    ): void => {
      let result: types.BuildResult = {
        errors: replaceDetailsInMessages(response!.errors, details),
        warnings: replaceDetailsInMessages(response!.warnings, details),
      };
      copyResponseToResult(response!, result);
      runOnEndCallbacks(result, () => {
        if (result.errors.length > 0) {
          return callback(failureErrorWithLog('Build failed', result.errors, result.warnings), null);
        }

        // Handle incremental rebuilds
        if (response!.rebuildID !== void 0) {
          if (!rebuild) {
            let isDisposed = false;
            (rebuild as any) = () => new Promise<types.BuildResult>((resolve, reject) => {
              if (isDisposed || isClosed) throw new Error('Cannot rebuild');
              sendRequest<protocol.RebuildRequest, protocol.BuildResponse>(refs, { command: 'rebuild', rebuildID: response!.rebuildID! },
                (error2, response2) => {
                  if (error2) {
                    const message: types.Message = { pluginName: '', text: error2, location: null, notes: [], detail: void 0 };
                    return callback(failureErrorWithLog('Build failed', [message], []), null);
                  }
                  buildResponseToResult(response2, (error3, result3) => {
                    if (error3) reject(error3);
                    else resolve(result3!);
                  });
                });
            });
            refs.ref()
            rebuild!.dispose = () => {
              if (isDisposed) return;
              isDisposed = true;
              sendRequest<protocol.RebuildDisposeRequest, null>(refs, { command: 'rebuild-dispose', rebuildID: response!.rebuildID! }, () => {
                // We don't care about the result
              });
              refs.unref() // Do this after the callback so "sendRequest" can extend the lifetime
            };
          }
          result.rebuild = rebuild;
        }

        // Handle watch mode
        if (response!.watchID !== void 0) {
          if (!stop) {
            let isStopped = false;
            refs.ref()
            stop = () => {
              if (isStopped) return;
              isStopped = true;
              watchCallbacks.delete(response!.watchID!);
              sendRequest<protocol.WatchStopRequest, null>(refs, { command: 'watch-stop', watchID: response!.watchID! }, () => {
                // We don't care about the result
              });
              refs.unref() // Do this after the callback so "sendRequest" can extend the lifetime
            }
            if (watch && watch.onRebuild) {
              watchCallbacks.set(response!.watchID, (serviceStopError, watchResponse) => {
                if (serviceStopError) return watch!.onRebuild!(serviceStopError as any, null);
                let result2: types.BuildResult = {
                  errors: replaceDetailsInMessages(watchResponse.errors, details),
                  warnings: replaceDetailsInMessages(watchResponse.warnings, details),
                };
                runOnEndCallbacks(result2, () => {
                  if (result2.errors.length > 0) {
                    return watch!.onRebuild!(failureErrorWithLog('Build failed', result2.errors, result2.warnings), null);
                  }
                  copyResponseToResult(watchResponse, result2);
                  if (watchResponse.rebuildID !== void 0) result2.rebuild = rebuild;
                  result2.stop = stop;
                  watch!.onRebuild!(null, result2);
                });
              });
            }
          }
          result.stop = stop;
        }

        callback(null, result);
      });
    };

    if (write && streamIn.isBrowser) throw new Error(`Cannot enable "write" in the browser`);
    if (incremental && streamIn.isSync) throw new Error(`Cannot use "incremental" with a synchronous build`);
    sendRequest<protocol.BuildRequest, protocol.BuildResponse>(refs, request, (error, response) => {
      if (error) return callback(new Error(error), null);
      if (serve) {
        let serveResponse = response as any as protocol.ServeResponse;
        let isStopped = false

        // Add a ref/unref for "stop()"
        refs.ref()
        let result: types.ServeResult = {
          port: serveResponse.port,
          host: serveResponse.host,
          wait: serve.wait,
          stop() {
            if (isStopped) return
            isStopped = true
            serve!.stop();
            refs.unref() // Do this after the callback so "stop" can extend the lifetime
          },
        };

        // Add a ref/unref for "wait". This must be done independently of
        // "stop()" in case the response to "stop()" comes in first before
        // the request for "wait". Without this ref/unref, node may close
        // the child's stdin pipe after the "stop()" but before the "wait"
        // which will cause things to break. This caused a test failure.
        refs.ref()
        serve.wait.then(refs.unref, refs.unref)

        return callback(null, result);
      }
      return buildResponseToResult(response!, callback);
    });
  };

  let transform: StreamService['transform'] = ({ callName, refs, input, options, isTTY, fs, callback }) => {
    const details = createObjectStash();

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
        if (typeof input !== 'string') throw new Error('The input to "transform" must be a string');
        let flags = flagsForTransformOptions(callName, options, isTTY, transformLogLevelDefault);
        let request: protocol.TransformRequest = {
          command: 'transform',
          flags,
          inputFS: inputPath !== null,
          input: inputPath !== null ? inputPath : input,
        };
        sendRequest<protocol.TransformRequest, protocol.TransformResponse>(refs, request, (error, response) => {
          if (error) return callback(new Error(error), null);
          let errors = replaceDetailsInMessages(response!.errors, details);
          let warnings = replaceDetailsInMessages(response!.warnings, details);
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
        try { pushLogFlags(flags, options, {}, isTTY, transformLogLevelDefault) } catch { }
        const error = extractErrorMessageV8(e, streamIn, details, void 0, '');
        sendRequest(refs, { command: 'error', flags, error }, () => {
          error.detail = details.load(error.detail);
          callback(failureErrorWithLog('Transform failed', [error], []), null);
        });
      }
    };
    if (typeof input === 'string' && input.length > 1024 * 1024) {
      let next = start;
      start = () => fs.writeFile(input, next);
    }
    start(null);
  };

  let formatMessages: StreamService['formatMessages'] = ({ callName, refs, messages, options, callback }) => {
    let result = sanitizeMessages(messages, 'messages', null, '');
    if (!options) throw new Error(`Missing second argument in ${callName}() call`);
    let keys: OptionKeys = {};
    let kind = getFlag(options, keys, 'kind', mustBeString);
    let color = getFlag(options, keys, 'color', mustBeBoolean);
    let terminalWidth = getFlag(options, keys, 'terminalWidth', mustBeInteger);
    checkForInvalidFlags(options, keys, `in ${callName}() call`);
    if (kind === void 0) throw new Error(`Missing "kind" in ${callName}() call`);
    if (kind !== 'error' && kind !== 'warning') throw new Error(`Expected "kind" to be "error" or "warning" in ${callName}() call`);
    let request: protocol.FormatMsgsRequest = {
      command: 'format-msgs',
      messages: result,
      isWarning: kind === 'warning',
    }
    if (color !== void 0) request.color = color;
    if (terminalWidth !== void 0) request.terminalWidth = terminalWidth;
    sendRequest<protocol.FormatMsgsRequest, protocol.FormatMsgsResponse>(refs, request, (error, response) => {
      if (error) return callback(new Error(error), null);
      callback(null, response!.messages);
    });
  };

  return {
    readFromStdout,
    afterClose,
    service: {
      buildOrServe,
      transform,
      formatMessages,
    },
  };
}

// This stores JavaScript objects on the JavaScript side and temporarily
// substitutes them with an integer that can be passed through the Go side
// and back. That way we can associate JavaScript objects with Go objects
// even if the JavaScript objects aren't serializable. And we also avoid
// the overhead of serializing large JavaScript objects.
interface ObjectStash {
  load(id: number): any;
  store(value: any): number;
}

function createObjectStash(): ObjectStash {
  const map = new Map<number, any>();
  let nextID = 0;
  return {
    load(id) {
      return map.get(id);
    },
    store(value) {
      if (value === void 0) return -1;
      const id = nextID++;
      map.set(id, value);
      return id;
    },
  };
}

function extractCallerV8(e: Error, streamIn: StreamIn, ident: string): () => types.Note | undefined {
  let note: types.Note | undefined
  let tried = false
  return () => {
    if (tried) return note
    tried = true
    try {
      let lines = (e.stack + '').split('\n')
      lines.splice(1, 1)
      let location = parseStackLinesV8(streamIn, lines, ident)
      if (location) {
        note = { text: e.message, location }
        return note
      }
    } catch {
    }
  }
}

function extractErrorMessageV8(e: any, streamIn: StreamIn, stash: ObjectStash | null, note: types.Note | undefined, pluginName: string): types.Message {
  let text = 'Internal error'
  let location: types.Location | null = null

  try {
    text = ((e && e.message) || e) + '';
  } catch {
  }

  // Optionally attempt to extract the file from the stack trace, works in V8/node
  try {
    location = parseStackLinesV8(streamIn, (e.stack + '').split('\n'), '')
  } catch {
  }

  return { pluginName, text, location, notes: note ? [note] : [], detail: stash ? stash.store(e) : -1 }
}

function parseStackLinesV8(streamIn: StreamIn, lines: string[], ident: string): types.Location | null {
  let at = '    at '

  // Check to see if this looks like a V8 stack trace
  if (streamIn.readFileSync && !lines[0].startsWith(at) && lines[1].startsWith(at)) {
    for (let i = 1; i < lines.length; i++) {
      let line = lines[i]
      if (!line.startsWith(at)) continue
      line = line.slice(at.length)
      while (true) {
        // Unwrap a function name
        let match = /^(?:new |async )?\S+ \((.*)\)$/.exec(line)
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
          let contents
          try {
            contents = streamIn.readFileSync(match[1], 'utf8')
          } catch {
            break
          }
          let lineText = contents.split(/\r\n|\r|\n|\u2028|\u2029/)[+match[2] - 1] || ''
          let column = +match[3] - 1
          let length = lineText.slice(column, column + ident.length) === ident ? ident.length : 0
          return {
            file: match[1],
            namespace: 'file',
            line: +match[2],
            column: protocol.encodeUTF8(lineText.slice(0, column)).length,
            length: protocol.encodeUTF8(lineText.slice(column, column + length)).length,
            lineText: lineText + '\n' + lines.slice(1).join('\n'),
            suggestion: '',
          }
        }
        break
      }
    }
  }

  return null;
}

function failureErrorWithLog(text: string, errors: types.Message[], warnings: types.Message[]): types.BuildFailure {
  let limit = 5
  let summary = errors.length < 1 ? '' : ` with ${errors.length} error${errors.length < 2 ? '' : 's'}:` +
    errors.slice(0, limit + 1).map((e, i) => {
      if (i === limit) return '\n...';
      if (!e.location) return `\nerror: ${e.text}`;
      let { file, line, column } = e.location;
      let pluginText = e.pluginName ? `[plugin: ${e.pluginName}] ` : '';
      return `\n${file}:${line}:${column}: error: ${pluginText}${e.text}`;
    }).join('');
  let error: any = new Error(`${text}${summary}`);
  error.errors = errors;
  error.warnings = warnings;
  return error;
}

function replaceDetailsInMessages(messages: types.Message[], stash: ObjectStash): types.Message[] {
  for (const message of messages) {
    message.detail = stash.load(message.detail);
  }
  return messages;
}

function sanitizeLocation(location: types.PartialMessage['location'], where: string): types.Message['location'] {
  if (location == null) return null;

  let keys: OptionKeys = {};
  let file = getFlag(location, keys, 'file', mustBeString);
  let namespace = getFlag(location, keys, 'namespace', mustBeString);
  let line = getFlag(location, keys, 'line', mustBeInteger);
  let column = getFlag(location, keys, 'column', mustBeInteger);
  let length = getFlag(location, keys, 'length', mustBeInteger);
  let lineText = getFlag(location, keys, 'lineText', mustBeString);
  let suggestion = getFlag(location, keys, 'suggestion', mustBeString);
  checkForInvalidFlags(location, keys, where);

  return {
    file: file || '',
    namespace: namespace || '',
    line: line || 0,
    column: column || 0,
    length: length || 0,
    lineText: lineText || '',
    suggestion: suggestion || '',
  };
}

function sanitizeMessages(messages: types.PartialMessage[], property: string, stash: ObjectStash | null, fallbackPluginName: string): types.Message[] {
  let messagesClone: types.Message[] = [];
  let index = 0;

  for (const message of messages) {
    let keys: OptionKeys = {};
    let pluginName = getFlag(message, keys, 'pluginName', mustBeString);
    let text = getFlag(message, keys, 'text', mustBeString);
    let location = getFlag(message, keys, 'location', mustBeObjectOrNull);
    let notes = getFlag(message, keys, 'notes', mustBeArray);
    let detail = getFlag(message, keys, 'detail', canBeAnything);
    let where = `in element ${index} of "${property}"`;
    checkForInvalidFlags(message, keys, where);

    let notesClone: types.Note[] = [];
    if (notes) {
      for (const note of notes) {
        let noteKeys: OptionKeys = {};
        let noteText = getFlag(note, noteKeys, 'text', mustBeString);
        let noteLocation = getFlag(note, noteKeys, 'location', mustBeObjectOrNull);
        checkForInvalidFlags(note, noteKeys, where);
        notesClone.push({
          text: noteText || '',
          location: sanitizeLocation(noteLocation, where),
        });
      }
    }

    messagesClone.push({
      pluginName: pluginName || fallbackPluginName,
      text: text || '',
      location: sanitizeLocation(location, where),
      notes: notesClone,
      detail: stash ? stash.store(detail) : -1,
    });
    index++;
  }

  return messagesClone;
}

function sanitizeStringArray(values: any[], property: string): string[] {
  const result: string[] = [];
  for (const value of values) {
    if (typeof value !== 'string') throw new Error(`${JSON.stringify(property)} must be an array of strings`);
    result.push(value);
  }
  return result;
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
