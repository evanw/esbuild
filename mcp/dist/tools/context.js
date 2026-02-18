import { z } from "zod";
import { getEsbuild } from "../esbuild-api.js";
const ContextSchema = {
    entryPoints: z.array(z.string()).describe("File paths to use as entry points"),
    bundle: z.boolean().optional().describe("Bundle imports into output (default: true)"),
    format: z.enum(["iife", "cjs", "esm"]).optional().describe("Output format"),
    target: z.string().optional().describe("Target environment"),
    platform: z.enum(["browser", "node", "neutral"]).optional().describe("Target platform"),
    minify: z.boolean().optional().describe("Minify output"),
    external: z.array(z.string()).optional().describe("Package names to exclude"),
    define: z.record(z.string(), z.string()).optional().describe("Global identifier replacements"),
    sourcemap: z.boolean().optional().describe("Generate source maps"),
    outdir: z.string().optional().describe("Output directory"),
    outfile: z.string().optional().describe("Output file"),
};
export function registerContextTool(server) {
    server.tool("esbuild_context", "Create an esbuild context for incremental builds. Returns the build result. The context is disposed after use.", ContextSchema, async (args) => {
        const esbuild = await getEsbuild();
        try {
            const ctx = await esbuild.context({
                entryPoints: args.entryPoints,
                bundle: args.bundle ?? true,
                format: args.format,
                target: args.target,
                platform: args.platform,
                minify: args.minify,
                external: args.external,
                define: args.define,
                sourcemap: args.sourcemap,
                outdir: args.outdir,
                outfile: args.outfile,
                write: false,
            });
            try {
                const result = await ctx.rebuild();
                const output = {
                    outputFiles: (result.outputFiles ?? []).map((f) => ({
                        path: f.path,
                        text: f.text,
                    })),
                    warnings: result.warnings,
                    errors: result.errors,
                };
                return {
                    content: [{
                            type: "text",
                            text: JSON.stringify(output, null, 2),
                        }],
                };
            }
            finally {
                await ctx.dispose();
            }
        }
        catch (err) {
            const error = err;
            return {
                content: [{
                        type: "text",
                        text: JSON.stringify({
                            errors: error.errors ?? [{ text: error.message ?? String(err) }],
                            warnings: error.warnings ?? [],
                        }, null, 2),
                    }],
                isError: true,
            };
        }
    });
}
