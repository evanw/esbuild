export declare type Target = 'esnext' | 'es6' | 'es2015' | 'es2016' | 'es2017' | 'es2018' | 'es2019' | 'es2020';
export declare type Platform = 'browser' | 'node';
export declare type Format = 'iife' | 'cjs';
export declare type Loader = 'js' | 'jsx' | 'ts' | 'tsx' | 'json' | 'text' | 'base64';

export interface Options {
  name?: string;
  bundle?: boolean;
  outfile?: string;
  outdir?: string;
  sourcemap?: boolean;
  errorLimit?: number;
  target?: Target;
  platform?: Platform;
  format?: Format;
  external?: string[];

  minify?: boolean;
  minifyWhitespace?: boolean;
  minifyIdentifiers?: boolean;
  minifySyntax?: boolean;

  jsxFactory?: string;
  jsxFragment?: string;
  define?: { [key: string]: string };
  loader?: { [ext: string]: Loader };

  entryPoints: string[];
}

export interface Message {
  text: string;
  location: null | {
    file: string;
    line: string;
    column: string;
  };
}

export interface Success {
  stderr: string;
  warnings: Message[];
}

export interface Failure extends Error {
  stderr: string;
  errors: Message[];
  warnings: Message[];
}

// This function invokes the "esbuild" command-line tool for you. It returns
// a promise that is either resolved with a "Success" object or rejected with a
// "Failure" object.
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
export declare function build(options: Options): Promise<Success>;
