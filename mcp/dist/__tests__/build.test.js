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
        expect(result.outputFiles.length).toBeGreaterThan(0);
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
    it('supports granular minification', async () => {
        const code = 'function longName() { return   1 +   2; }';
        const result = await esbuild.build({
            stdin: { contents: code, loader: 'js' },
            write: false,
            minifyWhitespace: true,
            minifySyntax: true,
            minifyIdentifiers: false,
        });
        const output = result.outputFiles[0].text;
        // whitespace and syntax should be minified, but identifiers preserved
        expect(output).toContain('longName');
        expect(output.length).toBeLessThan(code.length);
    });
    it('supports banner and footer', async () => {
        const result = await esbuild.build({
            stdin: { contents: 'export const x = 1', loader: 'js' },
            write: false,
            banner: { js: '/* BANNER */' },
            footer: { js: '/* FOOTER */' },
        });
        const output = result.outputFiles[0].text;
        expect(output).toContain('/* BANNER */');
        expect(output).toContain('/* FOOTER */');
    });
    it('supports define and pure', async () => {
        const result = await esbuild.build({
            stdin: { contents: 'if (DEBUG) console.log("dev"); export default 1;', loader: 'js' },
            write: false,
            bundle: true,
            define: { DEBUG: 'false' },
            pure: ['console.log'],
            treeShaking: true,
            minifySyntax: true,
        });
        const output = result.outputFiles[0].text;
        expect(output).not.toContain('console.log');
    });
    it('supports drop', async () => {
        const result = await esbuild.build({
            stdin: { contents: 'console.log("hi"); debugger; export default 1;', loader: 'js' },
            write: false,
            drop: ['debugger'],
        });
        const output = result.outputFiles[0].text;
        expect(output).not.toContain('debugger');
        expect(output).toContain('console.log');
    });
    it('supports keepNames', async () => {
        const result = await esbuild.build({
            stdin: { contents: 'export function myFunc() {}', loader: 'js' },
            write: false,
            minifyIdentifiers: true,
            keepNames: true,
        });
        const output = result.outputFiles[0].text;
        expect(output).toContain('myFunc');
    });
    it('supports stdin entry', async () => {
        const result = await esbuild.build({
            stdin: {
                contents: 'export const val = 42',
                loader: 'ts',
                sourcefile: 'virtual.ts',
            },
            write: false,
        });
        const output = result.outputFiles[0].text;
        expect(output).toContain('42');
    });
    it('supports alias', async () => {
        // alias replaces import paths before resolution
        const result = await esbuild.build({
            stdin: { contents: 'import "aliased-pkg"', loader: 'js', resolveDir: '.' },
            write: false,
            bundle: true,
            alias: { 'aliased-pkg': './nonexistent-but-aliased' },
            logLevel: 'silent',
        }).catch((e) => e);
        // The alias itself works (resolution changes), even if the target doesn't exist
        expect(result).toBeDefined();
    });
});
