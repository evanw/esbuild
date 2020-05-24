export declare type Target = 'esnext' | 'es6' | 'es2015' | 'es2016' | 'es2017' | 'es2018' | 'es2019' | 'es2020';
export declare type Platform = 'browser' | 'node';
export declare type Format = 'iife' | 'cjs' | 'esm';
export declare type Loader = 'js' | 'jsx' | 'ts' | 'tsx' | 'json' | 'text' | 'base64' | 'dataurl';

interface CommonOptions {
  sourcemap?: boolean | 'inline' | 'external';
  sourcefile?: string;
  errorLimit?: number;
  target?: Target;

  minify?: boolean;
  minifyWhitespace?: boolean;
  minifyIdentifiers?: boolean;
  minifySyntax?: boolean;

  jsxFactory?: string;
  jsxFragment?: string;
  define?: { [key: string]: string };
}

export interface BuildOptions extends CommonOptions {
  name?: string;
  bundle?: boolean;
  outfile?: string;
  outdir?: string;
  platform?: Platform;
  format?: Format;
  color?: boolean;
  external?: string[];
  loader?: { [ext: string]: Loader };

  entryPoints: string[];

  // This defaults to "pipe" which exposes a property called "stderr" on the
  // result. This can be set to "inherit" instead to forward the stderr of the
  // esbuild process to the current process's stderr.
  stdio?: 'pipe' | 'ignore' | 'inherit' | ('pipe' | 'ignore' | 'inherit' | number | null | undefined)[];
}

export interface Message {
  text: string;
  location: null | {
    file: string;
    line: string;
    column: string;
  };
}

export interface BuildResult {
  stderr: string;
  warnings: Message[];
}

export interface BuildFailure extends Error {
  stderr: string;
  errors: Message[];
  warnings: Message[];
}

// This function invokes the "esbuild" command-line tool for you. It returns
// a promise that is either resolved with a "BuildResult" object or rejected
// with a "BuildFailure" object.
//
// Example usage:
//
//   const esbuild = require('esbuild')
//   const fs = require('fs')
//
//   esbuild.build({
//     entryPoints: ['./example.ts'],
//     minify: true,
//     bundle: true,
//     outfile: './example.min.js',
//   }).then(
//     ({ stderr, warnings }) => {
//       const output = fs.readFileSync('./example.min.js', 'utf8')
//       console.log('success', { output, stderr, warnings })
//     },
//     ({ stderr, errors, warnings }) => {
//       console.error('failure', { stderr, errors, warnings })
//     }
//   )
//
export declare function build(options: BuildOptions): Promise<BuildResult>;

// This starts "esbuild" as a long-lived child process that is then reused, so
// you can call methods on the service many times without the overhead of
// starting up a new child process each time.
export declare function startService(): Promise<Service>;

interface Service {
  // This function transforms a single JavaScript file. It can be used to minify
  // JavaScript, convert TypeScript/JSX to JavaScript, or convert newer JavaScript
  // to older JavaScript. It returns a promise that is either resolved with a
  // "TransformResult" object or rejected with a "TransformFailure" object.
  transform(file: string, options: TransformOptions): Promise<TransformResult>;

  // This stops the service, which kills the long-lived child process. Any
  // pending requests will be aborted.
  stop(): void;
}

export interface TransformOptions extends CommonOptions {
  loader?: Loader;
}

export interface TransformResult {
  js?: string;
  jsSourceMap?: string;
  warnings: Message[];
}

export interface TransformFailure extends Error {
  errors: Message[];
  warnings: Message[];
}
