import { describe, it, expect } from 'vitest';
import * as esbuild from 'esbuild';

describe('esbuild_transform', () => {
  it('transforms TypeScript to JavaScript', async () => {
    const result = await esbuild.transform('const x: number = 1', { loader: 'ts' });
    expect(result.code).toContain('const x = 1');
  });

  it('transforms JSX', async () => {
    const result = await esbuild.transform('<div>hello</div>', { loader: 'jsx' });
    expect(result.code).toBeDefined();
  });

  it('minifies code', async () => {
    const result = await esbuild.transform('const   x   =   1', { minify: true });
    expect(result.code.length).toBeLessThan('const   x   =   1'.length);
  });

  it('handles invalid syntax', async () => {
    await expect(esbuild.transform('const const const', { loader: 'js' })).rejects.toThrow();
  });

  it('produces source map when requested', async () => {
    const result = await esbuild.transform('const x = 1', { sourcemap: 'inline' });
    expect(result.code).toContain('sourceMappingURL');
  });

  it('supports granular minification', async () => {
    const code = 'function longName() { return   1 +   2; }';
    const result = await esbuild.transform(code, {
      minifyWhitespace: true,
      minifySyntax: true,
      minifyIdentifiers: false,
      loader: 'js',
    });
    expect(result.code).toContain('longName');
    expect(result.code.length).toBeLessThan(code.length);
  });

  it('supports banner and footer (string)', async () => {
    const result = await esbuild.transform('const x = 1', {
      banner: '/* BANNER */',
      footer: '/* FOOTER */',
      loader: 'js',
    });
    expect(result.code).toContain('/* BANNER */');
    expect(result.code).toContain('/* FOOTER */');
  });

  it('supports define and pure', async () => {
    const result = await esbuild.transform(
      'if (DEBUG) console.log("dev"); export default 1;',
      {
        loader: 'js',
        define: { DEBUG: 'false' },
        pure: ['console.log'],
        treeShaking: true,
        minifySyntax: true,
      },
    );
    expect(result.code).not.toContain('console.log');
  });

  it('supports full sourcemap enum', async () => {
    const result = await esbuild.transform('const x = 1', {
      sourcemap: 'external',
      loader: 'js',
    });
    expect(result.map).toBeDefined();
    expect(result.code).not.toContain('sourceMappingURL');
  });

  it('supports platform option', async () => {
    const result = await esbuild.transform('export default 1', {
      platform: 'node',
      format: 'cjs',
      loader: 'js',
    });
    expect(result.code).toContain('module.exports');
  });

  it('supports drop', async () => {
    const result = await esbuild.transform('console.log("hi"); debugger;', {
      loader: 'js',
      drop: ['debugger'],
    });
    expect(result.code).not.toContain('debugger');
  });

  it('supports keepNames', async () => {
    const result = await esbuild.transform('export function myFunc() {}', {
      loader: 'js',
      minifyIdentifiers: true,
      keepNames: true,
    });
    expect(result.code).toContain('myFunc');
  });
});
