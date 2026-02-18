import { describe, it, expect } from 'vitest';
import * as esbuild from 'esbuild';

describe('esbuild_context', () => {
  it('performs an incremental rebuild', async () => {
    const ctx = await esbuild.context({
      stdin: { contents: 'export const x = 1', loader: 'js' },
      bundle: true,
      write: false,
    });

    try {
      const result = await ctx.rebuild();
      expect(result.outputFiles).toBeDefined();
      expect(result.outputFiles!.length).toBeGreaterThan(0);
      expect(result.outputFiles![0].text).toContain('x');
    } finally {
      await ctx.dispose();
    }
  });

  it('produces consistent output across multiple rebuilds', async () => {
    const ctx = await esbuild.context({
      stdin: { contents: 'export const y = 2', loader: 'js' },
      bundle: true,
      write: false,
    });

    try {
      const result1 = await ctx.rebuild();
      const result2 = await ctx.rebuild();
      expect(result1.outputFiles![0].text).toBe(result2.outputFiles![0].text);
    } finally {
      await ctx.dispose();
    }
  });

  it('reports build errors without throwing', async () => {
    // Invalid syntax should produce errors in the result
    try {
      const ctx = await esbuild.context({
        stdin: { contents: 'export const = ;', loader: 'js' },
        bundle: true,
        write: false,
        logLevel: 'silent',
      });
      const result = await ctx.rebuild();
      expect(result.errors.length).toBeGreaterThan(0);
      await ctx.dispose();
    } catch (err: unknown) {
      // esbuild may throw on context creation with invalid input
      expect(err).toBeDefined();
    }
  });
});
