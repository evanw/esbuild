import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { registerTransformTool } from "./tools/transform.js";
import { registerBuildTool } from "./tools/build.js";
import { registerAnalyzeTool } from "./tools/analyze.js";
import { registerFormatMessagesTool } from "./tools/format-messages.js";
import { registerContextTool } from "./tools/context.js";
import { registerServeTool } from "./tools/serve.js";
import { registerWatchTool } from "./tools/watch.js";
const server = new McpServer({
    name: "esbuild-mcp",
    version: "0.27.3",
});
registerTransformTool(server);
registerBuildTool(server);
registerAnalyzeTool(server);
registerFormatMessagesTool(server);
registerContextTool(server);
registerServeTool(server);
registerWatchTool(server);
const transport = new StdioServerTransport();
await server.connect(transport);
