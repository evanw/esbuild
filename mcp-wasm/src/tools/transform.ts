import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { getEsbuildWasm } from "../wasm-api.js";
import { formatErrorResponse } from "../errors.js";
import { TransformOnlySchema, CommonSchema, prepareBuildOptions } from "./schemas.js";

const TransformSchema = { ...TransformOnlySchema, ...CommonSchema };

export function registerTransformTool(server: McpServer): void {
  server.tool(
    "esbuild_wasm_transform",
    "Transform source code (TypeScript, JSX, CSS, etc.) to JavaScript or CSS using esbuild-wasm",
    TransformSchema,
    async (args) => {
      const esbuild = await getEsbuildWasm();

      try {
        const { code, ...rest } = prepareBuildOptions(args) as any;
        rest.loader = rest.loader ?? "ts";

        const result = await esbuild.transform(code, rest);

        const output: Record<string, unknown> = {
          code: result.code,
          warnings: result.warnings,
        };
        if (result.map) output.map = result.map;
        if (result.mangleCache) output.mangleCache = result.mangleCache;

        return {
          content: [{
            type: "text",
            text: JSON.stringify(output, null, 2),
          }],
        };
      } catch (err: unknown) {
        return formatErrorResponse(err);
      }
    }
  );
}
