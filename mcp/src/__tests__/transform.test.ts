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
});
