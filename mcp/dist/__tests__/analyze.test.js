import { describe, it, expect } from 'vitest';
import * as esbuild from 'esbuild';
describe('esbuild_analyze_metafile', () => {
    it('analyzes a metafile', async () => {
        const buildResult = await esbuild.build({
            stdin: { contents: 'export const x = 1', loader: 'js' },
            write: false,
            metafile: true,
        });
        const analysis = await esbuild.analyzeMetafile(buildResult.metafile);
        expect(analysis).toBeDefined();
        expect(typeof analysis).toBe('string');
    });
    it('supports verbose mode', async () => {
        const buildResult = await esbuild.build({
            stdin: { contents: 'export const x = 1', loader: 'js' },
            write: false,
            metafile: true,
        });
        const analysis = await esbuild.analyzeMetafile(buildResult.metafile, { verbose: true });
        expect(analysis).toBeDefined();
    });
    it('returns empty string for invalid metafile input', async () => {
        const result = await esbuild.analyzeMetafile('not json');
        expect(result).toBe('');
    });
});
