import { describe, it, expect } from 'vitest';
import * as esbuild from 'esbuild';
describe('esbuild_watch', () => {
    it('starts watch mode and returns initial build', async () => {
        const ctx = await esbuild.context({
            stdin: { contents: 'export const x = 1', loader: 'js' },
            bundle: true,
            write: false,
        });
        try {
            await ctx.watch();
            const result = await ctx.rebuild();
            expect(result.outputFiles).toBeDefined();
            expect(result.outputFiles.length).toBeGreaterThan(0);
        }
        finally {
            await ctx.dispose();
        }
    });
    it('handles watch with minify', async () => {
        const ctx = await esbuild.context({
            stdin: { contents: 'export const longVariableName = 1', loader: 'js' },
            bundle: true,
            minify: true,
            write: false,
        });
        try {
            await ctx.watch();
            const result = await ctx.rebuild();
            expect(result.outputFiles).toBeDefined();
        }
        finally {
            await ctx.dispose();
        }
    });
});
