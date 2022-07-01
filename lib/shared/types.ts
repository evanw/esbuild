export type Platform = 'browser' | 'node' | 'neutral';
export type Format = 'iife' | 'cjs' | 'esm';
export type Loader = 'js' | 'jsx' | 'ts' | 'tsx' | 'css' | 'json' | 'text' | 'base64' | 'file' | 'dataurl' | 'binary' | 'copy' | 'default';
export type LogLevel = 'verbose' | 'debug' | 'info' | 'warning' | 'error' | 'silent';
export type Charset = 'ascii' | 'utf8';
export type Drop = 'console' | 'debugger';

interface CommonOptions {
  /** Documentation: https://esbuild.github.io/api/#sourcemap */
  sourcemap?: boolean | 'linked' | 'inline' | 'external' | 'both';
  /** Documentation: https://esbuild.github.io/api/#legal-comments */
  legalComments?: 'none' | 'inline' | 'eof' | 'linked' | 'external';
  /** Documentation: https://esbuild.github.io/api/#source-root */
  sourceRoot?: string;
  /** Documentation: https://esbuild.github.io/api/#sources-content */
  sourcesContent?: boolean;

  /** Documentation: https://esbuild.github.io/api/#format */
  format?: Format;
  /** Documentation: https://esbuild.github.io/api/#globalName */
  globalName?: string;
  /** Documentation: https://esbuild.github.io/api/#target */
  target?: string | string[];
  /** Documentation: https://esbuild.github.io/api/#supported */
  supported?: Record<string, boolean>;

  /** Documentation: https://esbuild.github.io/api/#mangle-props */
  mangleProps?: RegExp;
  /** Documentation: https://esbuild.github.io/api/#mangle-props */
  reserveProps?: RegExp;
  /** Documentation: https://esbuild.github.io/api/#mangle-props */
  mangleQuoted?: boolean;
  /** Documentation: https://esbuild.github.io/api/#mangle-props */
  mangleCache?: Record<string, string | false>;
  /** Documentation: https://esbuild.github.io/api/#drop */
  drop?: Drop[];
  /** Documentation: https://esbuild.github.io/api/#minify */
  minify?: boolean;
  /** Documentation: https://esbuild.github.io/api/#minify */
  minifyWhitespace?: boolean;
  /** Documentation: https://esbuild.github.io/api/#minify */
  minifyIdentifiers?: boolean;
  /** Documentation: https://esbuild.github.io/api/#minify */
  minifySyntax?: boolean;
  /** Documentation: https://esbuild.github.io/api/#charset */
  charset?: Charset;
  /** Documentation: https://esbuild.github.io/api/#tree-shaking */
  treeShaking?: boolean;
  /** Documentation: https://esbuild.github.io/api/#ignore-annotations */
  ignoreAnnotations?: boolean;

  /** Documentation: https://esbuild.github.io/api/#jsx */
  jsx?: 'transform' | 'preserve';
  /** Documentation: https://esbuild.github.io/api/#jsx-factory */
  jsxFactory?: string;
  /** Documentation: https://esbuild.github.io/api/#jsx-fragment */
  jsxFragment?: string;

  /** Documentation: https://esbuild.github.io/api/#define */
  define?: { [key: string]: string };
  /** Documentation: https://esbuild.github.io/api/#pure */
  pure?: string[];
  /** Documentation: https://esbuild.github.io/api/#keep-names */
  keepNames?: boolean;

  /** Documentation: https://esbuild.github.io/api/#color */
  color?: boolean;
  /** Documentation: https://esbuild.github.io/api/#log-level */
  logLevel?: LogLevel;
  /** Documentation: https://esbuild.github.io/api/#log-limit */
  logLimit?: number;
  /** Documentation: https://esbuild.github.io/api/#log-override */
  logOverride?: Record<string, LogLevel>;
}

export interface BuildOptions extends CommonOptions {
  /** Documentation: https://esbuild.github.io/api/#bundle */
  bundle?: boolean;
  /** Documentation: https://esbuild.github.io/api/#splitting */
  splitting?: boolean;
  /** Documentation: https://esbuild.github.io/api/#preserve-symlinks */
  preserveSymlinks?: boolean;
  /** Documentation: https://esbuild.github.io/api/#outfile */
  outfile?: string;
  /** Documentation: https://esbuild.github.io/api/#metafile */
  metafile?: boolean;
  /** Documentation: https://esbuild.github.io/api/#outdir */
  outdir?: string;
  /** Documentation: https://esbuild.github.io/api/#outbase */
  outbase?: string;
  /** Documentation: https://esbuild.github.io/api/#platform */
  platform?: Platform;
  /** Documentation: https://esbuild.github.io/api/#external */
  external?: string[];
  /** Documentation: https://esbuild.github.io/api/#loader */
  loader?: { [ext: string]: Loader };
  /** Documentation: https://esbuild.github.io/api/#resolve-extensions */
  resolveExtensions?: string[];
  /** Documentation: https://esbuild.github.io/api/#mainFields */
  mainFields?: string[];
  /** Documentation: https://esbuild.github.io/api/#conditions */
  conditions?: string[];
  /** Documentation: https://esbuild.github.io/api/#write */
  write?: boolean;
  /** Documentation: https://esbuild.github.io/api/#allow-overwrite */
  allowOverwrite?: boolean;
  /** Documentation: https://esbuild.github.io/api/#tsconfig */
  tsconfig?: string;
  /** Documentation: https://esbuild.github.io/api/#out-extension */
  outExtension?: { [ext: string]: string };
  /** Documentation: https://esbuild.github.io/api/#public-path */
  publicPath?: string;
  /** Documentation: https://esbuild.github.io/api/#entry-names */
  entryNames?: string;
  /** Documentation: https://esbuild.github.io/api/#chunk-names */
  chunkNames?: string;
  /** Documentation: https://esbuild.github.io/api/#asset-names */
  assetNames?: string;
  /** Documentation: https://esbuild.github.io/api/#inject */
  inject?: string[];
  /** Documentation: https://esbuild.github.io/api/#banner */
  banner?: { [type: string]: string };
  /** Documentation: https://esbuild.github.io/api/#footer */
  footer?: { [type: string]: string };
  /** Documentation: https://esbuild.github.io/api/#incremental */
  incremental?: boolean;
  /** Documentation: https://esbuild.github.io/api/#entry-points */
  entryPoints?: string[] | Record<string, string>;
  /** Documentation: https://esbuild.github.io/api/#stdin */
  stdin?: StdinOptions;
  /** Documentation: https://esbuild.github.io/plugins/ */
  plugins?: Plugin[];
  /** Documentation: https://esbuild.github.io/api/#working-directory */
  absWorkingDir?: string;
  /** Documentation: https://esbuild.github.io/api/#node-paths */
  nodePaths?: string[]; // The "NODE_PATH" variable from Node.js
  /** Documentation: https://esbuild.github.io/api/#watch */
  watch?: boolean | WatchMode;
}

export interface WatchMode {
  onRebuild?: (error: BuildFailure | null, result: BuildResult | null) => void;
}

export interface StdinOptions {
  contents: string;
  resolveDir?: string;
  sourcefile?: string;
  loader?: Loader;
}

export interface Message {
  id: string;
  pluginName: string;
  text: string;
  location: Location | null;
  notes: Note[];

  /**
   * Optional user-specified data that is passed through unmodified. You can
   * use this to stash the original error, for example.
   */
  detail: any;
}

export interface Note {
  text: string;
  location: Location | null;
}

export interface Location {
  file: string;
  namespace: string;
  /** 1-based */
  line: number;
  /** 0-based, in bytes */
  column: number;
  /** in bytes */
  length: number;
  lineText: string;
  suggestion: string;
}

export interface OutputFile {
  path: string;
  /** "text" as bytes */
  contents: Uint8Array;
  /** "contents" as text */
  text: string;
}

export interface BuildInvalidate {
  (): Promise<BuildIncremental>;
  dispose(): void;
}

export interface BuildIncremental extends BuildResult {
  rebuild: BuildInvalidate;
}

export interface BuildResult {
  errors: Message[];
  warnings: Message[];
  /** Only when "write: false" */
  outputFiles?: OutputFile[];
  /** Only when "incremental: true" */
  rebuild?: BuildInvalidate;
  /** Only when "watch: true" */
  stop?: () => void;
  /** Only when "metafile: true" */
  metafile?: Metafile;
  /** Only when "mangleCache" is present */
  mangleCache?: Record<string, string | false>;
}

export interface BuildFailure extends Error {
  errors: Message[];
  warnings: Message[];
}

/** Documentation: https://esbuild.github.io/api/#serve-arguments */
export interface ServeOptions {
  port?: number;
  host?: string;
  servedir?: string;
  onRequest?: (args: ServeOnRequestArgs) => void;
}

export interface ServeOnRequestArgs {
  remoteAddress: string;
  method: string;
  path: string;
  status: number;
  /** The time to generate the response, not to send it */
  timeInMS: number;
}

/** Documentation: https://esbuild.github.io/api/#serve-return-values */
export interface ServeResult {
  port: number;
  host: string;
  wait: Promise<void>;
  stop: () => void;
}

export interface TransformOptions extends CommonOptions {
  tsconfigRaw?: string | {
    compilerOptions?: {
      jsxFactory?: string,
      jsxFragmentFactory?: string,
      useDefineForClassFields?: boolean,
      importsNotUsedAsValues?: 'remove' | 'preserve' | 'error',
      preserveValueImports?: boolean,
    },
  };

  sourcefile?: string;
  loader?: Loader;
  banner?: string;
  footer?: string;
}

export interface TransformResult {
  code: string;
  map: string;
  warnings: Message[];
  /** Only when "mangleCache" is present */
  mangleCache?: Record<string, string | false>;
}

export interface TransformFailure extends Error {
  errors: Message[];
  warnings: Message[];
}

export interface Plugin {
  name: string;
  setup: (build: PluginBuild) => (void | Promise<void>);
}

export interface PluginBuild {
  initialOptions: BuildOptions;
  resolve(path: string, options?: ResolveOptions): Promise<ResolveResult>;

  onStart(callback: () =>
    (OnStartResult | null | void | Promise<OnStartResult | null | void>)): void;
  onEnd(callback: (result: BuildResult) =>
    (void | Promise<void>)): void;
  onResolve(options: OnResolveOptions, callback: (args: OnResolveArgs) =>
    (OnResolveResult | null | undefined | Promise<OnResolveResult | null | undefined>)): void;
  onLoad(options: OnLoadOptions, callback: (args: OnLoadArgs) =>
    (OnLoadResult | null | undefined | Promise<OnLoadResult | null | undefined>)): void;

  // This is a full copy of the esbuild library in case you need it
  esbuild: {
    serve: typeof serve,
    build: typeof build,
    buildSync: typeof buildSync,
    transform: typeof transform,
    transformSync: typeof transformSync,
    formatMessages: typeof formatMessages,
    formatMessagesSync: typeof formatMessagesSync,
    analyzeMetafile: typeof analyzeMetafile,
    analyzeMetafileSync: typeof analyzeMetafileSync,
    initialize: typeof initialize,
    version: typeof version,
  };
}

export interface ResolveOptions {
  pluginName?: string;
  importer?: string;
  namespace?: string;
  resolveDir?: string;
  kind?: ImportKind;
  pluginData?: any;
}

export interface ResolveResult {
  errors: Message[];
  warnings: Message[];

  path: string;
  external: boolean;
  sideEffects: boolean;
  namespace: string;
  suffix: string;
  pluginData: any;
}

export interface OnStartResult {
  errors?: PartialMessage[];
  warnings?: PartialMessage[];
}

export interface OnResolveOptions {
  filter: RegExp;
  namespace?: string;
}

export interface OnResolveArgs {
  path: string;
  importer: string;
  namespace: string;
  resolveDir: string;
  kind: ImportKind;
  pluginData: any;
}

export type ImportKind =
  | 'entry-point'

  // JS
  | 'import-statement'
  | 'require-call'
  | 'dynamic-import'
  | 'require-resolve'

  // CSS
  | 'import-rule'
  | 'url-token'

export interface OnResolveResult {
  pluginName?: string;

  errors?: PartialMessage[];
  warnings?: PartialMessage[];

  path?: string;
  external?: boolean;
  sideEffects?: boolean;
  namespace?: string;
  suffix?: string;
  pluginData?: any;

  watchFiles?: string[];
  watchDirs?: string[];
}

export interface OnLoadOptions {
  filter: RegExp;
  namespace?: string;
}

export interface OnLoadArgs {
  path: string;
  namespace: string;
  suffix: string;
  pluginData: any;
}

export interface OnLoadResult {
  pluginName?: string;

  errors?: PartialMessage[];
  warnings?: PartialMessage[];

  contents?: string | Uint8Array;
  resolveDir?: string;
  loader?: Loader;
  pluginData?: any;

  watchFiles?: string[];
  watchDirs?: string[];
}

export interface PartialMessage {
  id?: string;
  pluginName?: string;
  text?: string;
  location?: Partial<Location> | null;
  notes?: PartialNote[];
  detail?: any;
}

export interface PartialNote {
  text?: string;
  location?: Partial<Location> | null;
}

export interface Metafile {
  inputs: {
    [path: string]: {
      bytes: number
      imports: {
        path: string
        kind: ImportKind
      }[]
    }
  }
  outputs: {
    [path: string]: {
      bytes: number
      inputs: {
        [path: string]: {
          bytesInOutput: number
        }
      }
      imports: {
        path: string
        kind: ImportKind
      }[]
      exports: string[]
      entryPoint?: string
    }
  }
}

export interface FormatMessagesOptions {
  kind: 'error' | 'warning';
  color?: boolean;
  terminalWidth?: number;
}

export interface AnalyzeMetafileOptions {
  color?: boolean;
  verbose?: boolean;
}

/**
 * This function invokes the "esbuild" command-line tool for you. It returns a
 * promise that either resolves with a "BuildResult" object or rejects with a
 * "BuildFailure" object.
 *
 * - Works in node: yes
 * - Works in browser: yes
 *
 * Documentation: https://esbuild.github.io/api/#build-api
 */
export declare function build(options: BuildOptions & { write: false }): Promise<BuildResult & { outputFiles: OutputFile[] }>;
export declare function build(options: BuildOptions & { incremental: true, metafile: true }): Promise<BuildIncremental & { metafile: Metafile }>;
export declare function build(options: BuildOptions & { incremental: true }): Promise<BuildIncremental>;
export declare function build(options: BuildOptions & { metafile: true }): Promise<BuildResult & { metafile: Metafile }>;
export declare function build(options: BuildOptions): Promise<BuildResult>;

/**
 * This function is similar to "build" but it serves the resulting files over
 * HTTP on a localhost address with the specified port.
 *
 * - Works in node: yes
 * - Works in browser: no
 *
 * Documentation: https://esbuild.github.io/api/#serve
 */
export declare function serve(serveOptions: ServeOptions, buildOptions: BuildOptions): Promise<ServeResult>;

/**
 * This function transforms a single JavaScript file. It can be used to minify
 * JavaScript, convert TypeScript/JSX to JavaScript, or convert newer JavaScript
 * to older JavaScript. It returns a promise that is either resolved with a
 * "TransformResult" object or rejected with a "TransformFailure" object.
 *
 * - Works in node: yes
 * - Works in browser: yes
 *
 * Documentation: https://esbuild.github.io/api/#transform-api
 */
export declare function transform(input: string, options?: TransformOptions): Promise<TransformResult>;

/**
 * Converts log messages to formatted message strings suitable for printing in
 * the terminal. This allows you to reuse the built-in behavior of esbuild's
 * log message formatter. This is a batch-oriented API for efficiency.
 *
 * - Works in node: yes
 * - Works in browser: yes
 */
export declare function formatMessages(messages: PartialMessage[], options: FormatMessagesOptions): Promise<string[]>;

/**
 * Pretty-prints an analysis of the metafile JSON to a string. This is just for
 * convenience to be able to match esbuild's pretty-printing exactly. If you want
 * to customize it, you can just inspect the data in the metafile yourself.
 *
 * - Works in node: yes
 * - Works in browser: yes
 *
 * Documentation: https://esbuild.github.io/api/#analyze
 */
export declare function analyzeMetafile(metafile: Metafile | string, options?: AnalyzeMetafileOptions): Promise<string>;

/**
 * A synchronous version of "build".
 *
 * - Works in node: yes
 * - Works in browser: no
 *
 * Documentation: https://esbuild.github.io/api/#build-api
 */
export declare function buildSync(options: BuildOptions & { write: false }): BuildResult & { outputFiles: OutputFile[] };
export declare function buildSync(options: BuildOptions): BuildResult;

/**
 * A synchronous version of "transform".
 *
 * - Works in node: yes
 * - Works in browser: no
 *
 * Documentation: https://esbuild.github.io/api/#transform-api
 */
export declare function transformSync(input: string, options?: TransformOptions): TransformResult;

/**
 * A synchronous version of "formatMessages".
 *
 * - Works in node: yes
 * - Works in browser: no
 */
export declare function formatMessagesSync(messages: PartialMessage[], options: FormatMessagesOptions): string[];

/**
 * A synchronous version of "analyzeMetafile".
 *
 * - Works in node: yes
 * - Works in browser: no
 *
 * Documentation: https://esbuild.github.io/api/#analyze
 */
export declare function analyzeMetafileSync(metafile: Metafile | string, options?: AnalyzeMetafileOptions): string;

/**
 * This configures the browser-based version of esbuild. It is necessary to
 * call this first and wait for the returned promise to be resolved before
 * making other API calls when using esbuild in the browser.
 *
 * - Works in node: yes
 * - Works in browser: yes ("options" is required)
 *
 * Documentation: https://esbuild.github.io/api/#running-in-the-browser
 */
export declare function initialize(options: InitializeOptions): Promise<void>;

export interface InitializeOptions {
  /**
   * The URL of the "esbuild.wasm" file. This must be provided when running
   * esbuild in the browser.
   */
  wasmURL?: string

  /**
   * The result of calling "new WebAssembly.Module(buffer)" where "buffer"
   * is a typed array or ArrayBuffer containing the binary code of the
   * "esbuild.wasm" file.
   *
   * You can use this as an alternative to "wasmURL" for environments where it's
   * not possible to download the WebAssembly module.
   */
  wasmModule?: WebAssembly.Module

  /**
   * By default esbuild runs the WebAssembly-based browser API in a web worker
   * to avoid blocking the UI thread. This can be disabled by setting "worker"
   * to false.
   */
  worker?: boolean
}

export let version: string;
