import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { getEsbuild } from "../esbuild-api.js";

const BuildSchema = {
  entryPoints: z
    .array(z.string())
    .describe("File paths relative to CWD to use as entry points"),
  bundle: z
    .boolean()
    .optional()
    .describe("Bundle imports into output (default: true)"),
  format: z.enum(["iife", "cjs", "esm"]).optional().describe("Output format"),
  target: z
    .string()
    .optional()
    .describe("Target environment (e.g. es2020, esnext)"),
  platform: z
    .enum(["browser", "node", "neutral"])
    .optional()
    .describe("Target platform"),
  minify: z.boolean().optional().describe("Minify output"),
  splitting: z
    .boolean()
    .optional()
    .describe("Enable code splitting (ESM only)"),
  external: z
    .array(z.string())
    .optional()
    .describe("Package names to exclude from bundle"),
  define: z
    .record(z.string(), z.string())
    .optional()
    .describe("Global identifier replacements"),
  metafile: z.boolean().optional().describe("Include bundle analysis metafile"),
  sourcemap: z.boolean().optional().describe("Generate source maps"),
};

export function registerBuildTool(server: McpServer): void {
  server.tool(
    "esbuild_build",
    "Bundle entry point files using esbuild. Returns output contents in memory (does not write to disk).",
    BuildSchema,
    async (args) => {
      const esbuild = await getEsbuild();

      try {
        const result = await esbuild.build({
          entryPoints: args.entryPoints,
          bundle: args.bundle ?? true,
          format: args.format,
          target: args.target,
          platform: args.platform,
          minify: args.minify,
          splitting: args.splitting,
          external: args.external,
          define: args.define,
          metafile: args.metafile,
          sourcemap: args.sourcemap,
          write: false,
        });

        const output: Record<string, unknown> = {
          outputFiles: (result.outputFiles ?? []).map((f) => ({
            path: f.path,
            text: f.text,
          })),
          warnings: result.warnings,
          errors: result.errors,
        };
        if (result.metafile) output.metafile = result.metafile;

        return {
          content: [
            {
              type: "text",
              text: JSON.stringify(output, null, 2),
            },
          ],
        };
      } catch (err: unknown) {
        const error = err as { errors?: unknown[]; warnings?: unknown[]; message?: string };
        return {
          content: [
            {
              type: "text",
              text: JSON.stringify(
                {
                  errors: error.errors ?? [{ text: error.message ?? String(err) }],
                  warnings: error.warnings ?? [],
                },
                null,
                2
              ),
            },
          ],
          isError: true,
        };
      }
    }
  );
}
