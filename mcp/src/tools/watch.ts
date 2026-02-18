import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { getEsbuild } from "../esbuild-api.js";

const WatchSchema = {
  entryPoints: z.array(z.string()).describe("File paths to use as entry points"),
  bundle: z.boolean().optional().describe("Bundle imports into output (default: true)"),
  format: z.enum(["iife", "cjs", "esm"]).optional().describe("Output format"),
  outdir: z.string().optional().describe("Output directory"),
  outfile: z.string().optional().describe("Output file"),
  minify: z.boolean().optional().describe("Minify output"),
  platform: z.enum(["browser", "node", "neutral"]).optional().describe("Target platform"),
};

export function registerWatchTool(server: McpServer): void {
  server.tool(
    "esbuild_watch",
    "Start esbuild in watch mode for automatic rebuilds on file changes. Returns the initial build result.",
    WatchSchema,
    async (args) => {
      const esbuild = await getEsbuild();

      try {
        const ctx = await esbuild.context({
          entryPoints: args.entryPoints,
          bundle: args.bundle ?? true,
          format: args.format,
          outdir: args.outdir,
          outfile: args.outfile,
          minify: args.minify,
          platform: args.platform,
          write: false,
        });

        await ctx.watch();
        const result = await ctx.rebuild();

        return {
          content: [{
            type: "text",
            text: JSON.stringify({
              watching: true,
              outputFiles: (result.outputFiles ?? []).map((f) => ({
                path: f.path,
                text: f.text,
              })),
              warnings: result.warnings,
              errors: result.errors,
            }, null, 2),
          }],
        };
      } catch (err: unknown) {
        const error = err as { errors?: unknown[]; warnings?: unknown[]; message?: string };
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
    }
  );
}
