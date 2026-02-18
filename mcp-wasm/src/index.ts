#!/usr/bin/env node
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { registerInitializeTool } from "./tools/initialize.js";
import { registerStatusTool } from "./tools/status.js";
import { registerTransformTool } from "./tools/transform.js";
import { registerBuildTool } from "./tools/build.js";
import { registerAnalyzeTool } from "./tools/analyze.js";
import { registerFormatMessagesTool } from "./tools/format-messages.js";
import { registerContextTool } from "./tools/context.js";

const server = new McpServer({
  name: "esbuild-wasm-mcp",
  version: "0.27.3",
});

registerInitializeTool(server);
registerStatusTool(server);
registerTransformTool(server);
registerBuildTool(server);
registerAnalyzeTool(server);
registerFormatMessagesTool(server);
registerContextTool(server);

const transport = new StdioServerTransport();
await server.connect(transport);
