export declare type Platform = 'browser' | 'node';
export declare type Format = 'iife' | 'cjs' | 'esm';
export declare type Loader = 'js' | 'jsx' | 'ts' | 'tsx' | 'json' | 'text' | 'base64' | 'file' | 'dataurl' | 'binary';
export declare type LogLevel = 'info' | 'warning' | 'error' | 'silent';
export declare type Strict = 'nullish-coalescing' | 'class-fields';

export interface CommonOptions {
  sourcemap?: boolean | 'inline' | 'external';
  target?: string | string[];
  strict?: boolean | Strict[];

  minify?: boolean;
  minifyWhitespace?: boolean;
  minifyIdentifiers?: boolean;
  minifySyntax?: boolean;

  jsxFactory?: string;
  jsxFragment?: string;
  define?: { [key: string]: string };
  pure?: string[];

  color?: boolean;
  logLevel?: LogLevel;
  errorLimit?: number;
}

export interface BuildOptions extends CommonOptions {
  globalName?: string;
  bundle?: boolean;
  splitting?: boolean;
  outfile?: string;
  metafile?: string;
  outdir?: string;
  platform?: Platform;
  format?: Format;
  color?: boolean;
  external?: string[];
  loader?: { [ext: string]: Loader };
  resolveExtensions?: string[];
  write?: boolean;

  entryPoints: string[];
}

export interface Message {
  text: string;
  location: null | {
    file: string;
    line: number; // 1-based
    column: number; // 0-based, in bytes
    length: number; // in bytes
    lineText: string;
  };
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
    }
  }
}

// This function invokes the "esbuild" command-line tool for you. It returns a
// promise that either resolves with a "BuildResult" object or rejects with a
// "BuildFailure" object.
export declare function build(options: BuildOptions): Promise<BuildResult>;

// This function transforms a single JavaScript file. It can be used to minify
// JavaScript, convert TypeScript/JSX to JavaScript, or convert newer JavaScript
// to older JavaScript. It returns a promise that is either resolved with a
// "TransformResult" object or rejected with a "TransformFailure" object.
export declare function transform(input: string, options: TransformOptions): Promise<TransformResult>;

export declare function buildSync(options: BuildOptions): BuildResult;
export declare function transformSync(input: string, options: TransformOptions): TransformResult;

// This starts "esbuild" as a long-lived child process that is then reused, so
// you can call methods on the service many times without the overhead of
// starting up a new child process each time.
export declare function startService(): Promise<Service>;

export interface Service {
  build(options: BuildOptions): Promise<BuildResult>;
  transform(input: string, options: TransformOptions): Promise<TransformResult>;

  // This stops the service, which kills the long-lived child process. Any
  // pending requests will be aborted.
  stop(): void;
}
