import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { getEsbuild } from "../esbuild-api.js";
import { formatErrorResponse } from "../errors.js";

const TransformSchema = {
  code: z.string().describe("Source code to transform"),
  loader: z
    .enum(["js", "jsx", "ts", "tsx", "css", "local-css", "json", "text", "base64", "binary", "dataurl", "copy", "default", "empty", "file"])
    .optional()
    .describe("Loader to use (default: ts)"),
  target: z
    .string()
    .optional()
    .describe("Target environment (e.g. es2020, esnext, es2015)"),
  format: z.enum(["iife", "cjs", "esm"]).optional().describe("Output format"),
  minify: z
    .boolean()
    .optional()
    .describe("Minify whitespace, syntax, and identifiers"),
  sourcemap: z.boolean().optional().describe("Include inline source map"),
  jsx: z
    .enum(["transform", "preserve", "automatic"])
    .optional()
    .describe("JSX handling mode"),
  tsconfigRaw: z.string().optional().describe("Raw tsconfig JSON override"),
};

export function registerTransformTool(server: McpServer): void {
  server.tool(
    "esbuild_transform",
    "Transform source code (TypeScript, JSX, CSS, etc.) to JavaScript or CSS using esbuild",
    TransformSchema,
    async (args) => {
      const esbuild = await getEsbuild();

      try {
        const result = await esbuild.transform(args.code, {
          loader: args.loader ?? "ts",
          target: args.target,
          format: args.format,
          minify: args.minify,
          sourcemap: args.sourcemap ? "inline" : undefined,
          jsx: args.jsx,
          tsconfigRaw: args.tsconfigRaw,
        });

        const output: Record<string, unknown> = {
          code: result.code,
          warnings: result.warnings,
        };
        if (result.map) output.map = result.map;

        return {
          content: [
            {
              type: "text",
              text: JSON.stringify(output, null, 2),
            },
          ],
        };
      } catch (err: unknown) {
        return formatErrorResponse(err);
      }
    }
  );
}
