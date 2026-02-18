import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { getEsbuild } from "../esbuild-api.js";

const ServeSchema = {
  entryPoints: z.array(z.string()).describe("File paths to use as entry points"),
  bundle: z.boolean().optional().describe("Bundle imports into output (default: true)"),
  format: z.enum(["iife", "cjs", "esm"]).optional().describe("Output format"),
  port: z.number().optional().describe("Port to serve on (default: auto)"),
  host: z.string().optional().describe("Host to serve on (default: 0.0.0.0)"),
  servedir: z.string().optional().describe("Directory to serve static files from"),
  outdir: z.string().optional().describe("Output directory"),
};

export function registerServeTool(server: McpServer): void {
  server.tool(
    "esbuild_serve",
    "Start an esbuild development server with live reload. Returns the host and port.",
    ServeSchema,
    async (args) => {
      const esbuild = await getEsbuild();

      try {
        const ctx = await esbuild.context({
          entryPoints: args.entryPoints,
          bundle: args.bundle ?? true,
          format: args.format,
          outdir: args.outdir,
          write: true,
        });

        const result = await ctx.serve({
          port: args.port,
          host: args.host,
          servedir: args.servedir,
        });

        return {
          content: [{
            type: "text",
            text: JSON.stringify({ hosts: result.hosts, port: result.port }, null, 2),
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
