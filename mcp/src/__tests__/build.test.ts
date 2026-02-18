import { describe, it, expect } from 'vitest';
import * as esbuild from 'esbuild';

describe('esbuild_build', () => {
  it('bundles with write: false', async () => {
    const result = await esbuild.build({
      stdin: { contents: 'export const x = 1', loader: 'js' },
      write: false,
      bundle: true,
    });
    expect(result.outputFiles).toBeDefined();
    expect(result.outputFiles!.length).toBeGreaterThan(0);
  });

  it('generates metafile', async () => {
    const result = await esbuild.build({
      stdin: { contents: 'export const x = 1', loader: 'js' },
      write: false,
      metafile: true,
    });
    expect(result.metafile).toBeDefined();
  });

  it('handles build errors gracefully', async () => {
    await expect(esbuild.build({
      entryPoints: ['/nonexistent/file.js'],
      write: false,
    })).rejects.toThrow();
  });
});
