export type Platform = 'browser' | 'node' | 'neutral';
export type Format = 'iife' | 'cjs' | 'esm';
export type Loader = 'js' | 'jsx' | 'ts' | 'tsx' | 'css' | 'json' | 'text' | 'base64' | 'file' | 'dataurl' | 'binary' | 'default';
export type LogLevel = 'info' | 'warning' | 'error' | 'silent';
export type Charset = 'ascii' | 'utf8';
export type TreeShaking = true | 'ignore-annotations';

interface CommonOptions {
  sourcemap?: boolean | 'inline' | 'external' | 'both';
  sourcesContent?: boolean;

  format?: Format;
  globalName?: string;
  target?: string | string[];

  minify?: boolean;
  minifyWhitespace?: boolean;
  minifyIdentifiers?: boolean;
  minifySyntax?: boolean;
  charset?: Charset;
  treeShaking?: TreeShaking;

  jsxFactory?: string;
  jsxFragment?: string;
  define?: { [key: string]: string };
  pure?: string[];
  avoidTDZ?: boolean;
  keepNames?: boolean;
  banner?: string;
  footer?: string;

  color?: boolean;
  logLevel?: LogLevel;
  errorLimit?: number;
}

export interface BuildOptions extends CommonOptions {
  bundle?: boolean;
  splitting?: boolean;
  outfile?: string;
  metafile?: string;
  outdir?: string;
  outbase?: string;
  platform?: Platform;
  external?: string[];
  loader?: { [ext: string]: Loader };
  resolveExtensions?: string[];
  mainFields?: string[];
  write?: boolean;
  tsconfig?: string;
  outExtension?: { [ext: string]: string };
  publicPath?: string;
  inject?: string[];
  incremental?: boolean;
  entryPoints?: string[];
  stdin?: StdinOptions;
  plugins?: Plugin[];
}

export interface StdinOptions {
  contents: string;
  resolveDir?: string;
  sourcefile?: string;
  loader?: Loader;
}

export interface Message {
  text: string;
  location: Location | null;

  // Optional user-specified data that is passed through unmodified. You can
  // use this to stash the original error, for example.
  detail: any;
}

export interface Location {
  file: string;
  namespace: string;
  line: number; // 1-based
  column: number; // 0-based, in bytes
  length: number; // in bytes
  lineText: string;
}

export interface OutputFile {
  path: string;
  contents: Uint8Array; // "text" as bytes
  text: string; // "contents" as text
}

export interface BuildInvalidate {
  (): Promise<BuildIncremental>;
  dispose(): void;
}

export interface BuildIncremental extends BuildResult {
  rebuild: BuildInvalidate;
}

export interface BuildResult {
  warnings: Message[];
  outputFiles?: OutputFile[]; // Only when "write: false"
  rebuild?: BuildInvalidate; // Only when "incremental" is true
}

export interface BuildFailure extends Error {
  errors: Message[];
  warnings: Message[];
}

export interface ServeOptions {
  port?: number;
  host?: string;
  onRequest?: (args: ServeOnRequestArgs) => void;
}

export interface ServeOnRequestArgs {
  remoteAddress: string;
  method: string;
  path: string;
  status: number;
  timeInMS: number; // The time to generate the response, not to send it
}

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
    },
  };

  sourcefile?: string;
  loader?: Loader;
}

export interface TransformResult {
  code: string;
  map: string;
  warnings: Message[];
}

export interface TransformFailure extends Error {
  errors: Message[];
  warnings: Message[];
}

export interface Plugin {
  name: string;
  setup: (build: PluginBuild) => void;
}

export interface PluginBuild {
  onResolve(options: OnResolveOptions, callback: (args: OnResolveArgs) =>
    (OnResolveResult | null | undefined | Promise<OnResolveResult | null | undefined>)): void;
  onLoad(options: OnLoadOptions, callback: (args: OnLoadArgs) =>
    (OnLoadResult | null | undefined | Promise<OnLoadResult | null | undefined>)): void;
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
}

export interface OnResolveResult {
  pluginName?: string;

  errors?: PartialMessage[];
  warnings?: PartialMessage[];

  path?: string;
  external?: boolean;
  namespace?: string;
}

export interface OnLoadOptions {
  filter: RegExp;
  namespace?: string;
}

export interface OnLoadArgs {
  path: string;
  namespace: string;
}

export interface OnLoadResult {
  pluginName?: string;

  errors?: PartialMessage[];
  warnings?: PartialMessage[];

  contents?: string | Uint8Array;
  resolveDir?: string;
  loader?: Loader;
}

export interface PartialMessage {
  text?: string;
  location?: Partial<Location> | null;
  detail?: any;
}

export type MetadataImportKind =
  // JS
  | 'import-statement'
  | 'require-call'
  | 'dynamic-import'
  | 'require-resolve'

  // CSS
  | 'import-rule'
  | 'url-token'

// This is the type information for the "metafile" JSON format
export interface Metadata {
  inputs: {
    [path: string]: {
      bytes: number
      imports: {
        path: string
        kind: MetadataImportKind
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
        kind: MetadataImportKind
      }[]
      exports: string[]
    }
  }
}

export interface Service {
  build(options: BuildOptions & { write: false }): Promise<BuildResult & { outputFiles: OutputFile[] }>;
  build(options: BuildOptions & { incremental: true }): Promise<BuildIncremental>;
  build(options: BuildOptions): Promise<BuildResult>;
  serve(serveOptions: ServeOptions, buildOptions: BuildOptions): Promise<ServeResult>;
  transform(input: string, options?: TransformOptions): Promise<TransformResult>;

  // This stops the service, which kills the long-lived child process. Any
  // pending requests will be aborted.
  stop(): void;
}

// This function invokes the "esbuild" command-line tool for you. It returns a
// promise that either resolves with a "BuildResult" object or rejects with a
// "BuildFailure" object.
//
// Works in node: yes
// Works in browser: no
export declare function build(options: BuildOptions & { write: false }): Promise<BuildResult & { outputFiles: OutputFile[] }>;
export declare function build(options: BuildOptions & { incremental: true }): Promise<BuildIncremental>;
export declare function build(options: BuildOptions): Promise<BuildResult>;

// This function is similar to "build" but it serves the resulting files over
// HTTP on a localhost address with the specified port.
//
// Works in node: yes
// Works in browser: no
export declare function serve(serveOptions: ServeOptions, buildOptions: BuildOptions): Promise<ServeResult>;

// This function transforms a single JavaScript file. It can be used to minify
// JavaScript, convert TypeScript/JSX to JavaScript, or convert newer JavaScript
// to older JavaScript. It returns a promise that is either resolved with a
// "TransformResult" object or rejected with a "TransformFailure" object.
//
// Works in node: yes
// Works in browser: no
export declare function transform(input: string, options?: TransformOptions): Promise<TransformResult>;

// A synchronous version of "build".
//
// Works in node: yes
// Works in browser: no
export declare function buildSync(options: BuildOptions & { write: false }): BuildResult & { outputFiles: OutputFile[] };
export declare function buildSync(options: BuildOptions): BuildResult;

// A synchronous version of "transform".
//
// Works in node: yes
// Works in browser: no
export declare function transformSync(input: string, options?: TransformOptions): TransformResult;

// This starts "esbuild" as a long-lived child process that is then reused, so
// you can call methods on the service many times without the overhead of
// starting up a new child process each time.
//
// Works in node: yes
// Works in browser: yes ("options" is required)
export declare function startService(options?: ServiceOptions): Promise<Service>;

export interface ServiceOptions {
  // The URL of the "esbuild.wasm" file. This must be provided when running
  // esbuild in the browser.
  wasmURL?: string

  // By default esbuild runs the WebAssembly-based browser API in a web worker
  // to avoid blocking the UI thread. This can be disabled by setting "worker"
  // to false.
  worker?: boolean
}

export let version: string;
