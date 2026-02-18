import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { getState } from "../wasm-api.js";

export function registerStatusTool(server: McpServer): void {
  server.tool(
    "esbuild_wasm_status",
    "Check the current esbuild-wasm initialization status, version, options, and any errors",
    {},
    async () => {
      return {
        content: [{
          type: "text",
          text: JSON.stringify(getState(), null, 2),
        }],
      };
    }
  );
}
