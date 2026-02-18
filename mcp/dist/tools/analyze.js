import { z } from "zod";
import { getEsbuild } from "../esbuild-api.js";
import { formatErrorResponse } from "../errors.js";
const AnalyzeSchema = {
    metafile: z
        .string()
        .describe("JSON metafile string from esbuild_build (with metafile: true)"),
    verbose: z
        .boolean()
        .optional()
        .describe("Show all imports, not just top-level"),
};
export function registerAnalyzeTool(server) {
    server.tool("esbuild_analyze_metafile", "Parse and display a bundle analysis metafile as human-readable text", AnalyzeSchema, async (args) => {
        const esbuild = await getEsbuild();
        try {
            const metafile = typeof args.metafile === "string"
                ? JSON.parse(args.metafile)
                : args.metafile;
            const analysis = await esbuild.analyzeMetafile(metafile, {
                verbose: args.verbose,
            });
            return {
                content: [
                    {
                        type: "text",
                        text: analysis,
                    },
                ],
            };
        }
        catch (err) {
            return formatErrorResponse(err);
        }
    });
}
