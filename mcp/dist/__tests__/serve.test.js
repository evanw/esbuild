import { describe, it, expect } from 'vitest';
import * as esbuild from 'esbuild';
describe('esbuild_serve', () => {
    it('starts a dev server and returns host/port', async () => {
        const ctx = await esbuild.context({
            stdin: { contents: 'export const x = 1', loader: 'js' },
            bundle: true,
            write: false,
        });
        try {
            const result = await ctx.serve({ port: 0 });
            expect(result.hosts).toBeDefined();
            expect(result.port).toBeGreaterThan(0);
        }
        finally {
            await ctx.dispose();
        }
    });
    it('serves with servedir option', async () => {
        const ctx = await esbuild.context({
            stdin: { contents: 'export const y = 2', loader: 'js' },
            bundle: true,
            write: false,
        });
        try {
            const result = await ctx.serve({ port: 0, servedir: '.' });
            expect(result.port).toBeGreaterThan(0);
        }
        finally {
            await ctx.dispose();
        }
    });
});
