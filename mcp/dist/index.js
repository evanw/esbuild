import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { registerTransformTool } from "./tools/transform.js";
import { registerBuildTool } from "./tools/build.js";
import { registerAnalyzeTool } from "./tools/analyze.js";
import { registerFormatMessagesTool } from "./tools/format-messages.js";
const server = new McpServer({
    name: "esbuild-mcp",
    version: "0.27.3",
});
registerTransformTool(server);
registerBuildTool(server);
registerAnalyzeTool(server);
registerFormatMessagesTool(server);
const transport = new StdioServerTransport();
await server.connect(transport);
