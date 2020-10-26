export type Platform = 'browser' | 'node';
export type Format = 'iife' | 'cjs' | 'esm';
export type Loader = 'js' | 'jsx' | 'ts' | 'tsx' | 'css' | 'json' | 'text' | 'base64' | 'file' | 'dataurl' | 'binary';
export type LogLevel = 'info' | 'warning' | 'error' | 'silent';
export type Strict = 'nullish-coalescing' | 'optional-chaining' | 'class-fields';
export type Charset = 'ascii' | 'utf8';

interface CommonOptions {
  sourcemap?: boolean | 'inline' | 'external';
  format?: Format;
  globalName?: string;
  target?: string | string[];
  strict?: boolean | Strict[];

  minify?: boolean;
  minifyWhitespace?: boolean;
  minifyIdentifiers?: boolean;
  minifySyntax?: boolean;
  charset?: Charset;

  jsxFactory?: string;
  jsxFragment?: string;
  define?: { [key: string]: string };
  pure?: string[];
  avoidTDZ?: boolean;

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
  platform?: Platform;
  color?: boolean;
  external?: string[];
  loader?: { [ext: string]: Loader };
  resolveExtensions?: string[];
  mainFields?: string[];
  write?: boolean;
  tsconfig?: string;
  outExtension?: { [ext: string]: string };
  publicPath?: string;
  inject?: string[];

  entryPoints?: string[];
  stdin?: StdinOptions;
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
}

export interface Location {
  file: string;
  line: number; // 1-based
  column: number; // 0-based, in bytes
  length: number; // in bytes
  lineText: string;
}

export interface OutputFile {
  path: string;
  contents: Uint8Array;
}

export interface BuildResult {
  warnings: Message[];
  outputFiles?: OutputFile[]; // Only when "write: false"
}

export interface BuildFailure extends Error {
  errors: Message[];
  warnings: Message[];
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
  js: string;
  jsSourceMap: string;
  warnings: Message[];
}

export interface TransformFailure extends Error {
  errors: Message[];
  warnings: Message[];
}

// This is the type information for the "metafile" JSON format
export interface Metadata {
  inputs: {
    [path: string]: {
      bytes: number
      imports: {
        path: string
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
      }[]
    }
  }
}

export interface Service {
  build(options: BuildOptions): Promise<BuildResult>;
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
export declare function build(options: BuildOptions): Promise<BuildResult>;

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
