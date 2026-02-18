import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { getEsbuild } from "../esbuild-api.js";
import { formatErrorResponse } from "../errors.js";

const LocationSchema = z
  .object({
    file: z.string().optional(),
    namespace: z.string().optional(),
    line: z.number().optional(),
    column: z.number().optional(),
    length: z.number().optional(),
    lineText: z.string().optional(),
    suggestion: z.string().optional(),
  })
  .optional()
  .nullable();

const MessageSchema = z.object({
  id: z.string().optional(),
  pluginName: z.string().optional(),
  text: z.string(),
  location: LocationSchema,
  notes: z
    .array(
      z.object({
        text: z.string(),
        location: LocationSchema,
      })
    )
    .optional(),
  detail: z.unknown().optional(),
});

const FormatMessagesSchema = {
  messages: z
    .array(MessageSchema)
    .describe("Array of esbuild Message objects to format"),
  kind: z.enum(["error", "warning"]).describe("Message kind"),
};

export function registerFormatMessagesTool(server: McpServer): void {
  server.tool(
    "esbuild_format_messages",
    "Format esbuild error or warning messages for display with context and colors",
    FormatMessagesSchema,
    async (args) => {
      const esbuild = await getEsbuild();

      try {
        // formatMessages expects esbuild.Message[] but we have a compatible shape
        const formatted = await esbuild.formatMessages(
          args.messages as Parameters<typeof esbuild.formatMessages>[0],
          { kind: args.kind, color: false }
        );

        return {
          content: [
            {
              type: "text",
              text: JSON.stringify(formatted, null, 2),
            },
          ],
        };
      } catch (err: unknown) {
        return formatErrorResponse(err);
      }
    }
  );
}
