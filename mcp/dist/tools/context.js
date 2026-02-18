import { getEsbuild } from "../esbuild-api.js";
import { formatErrorResponse } from "../errors.js";
import { BuildOnlySchema, CommonSchema, prepareBuildOptions } from "./schemas.js";
const ContextSchema = { ...BuildOnlySchema, ...CommonSchema };
export function registerContextTool(server) {
    server.tool("esbuild_context", "Create an esbuild context for incremental builds. Returns the build result. The context is disposed after use.", ContextSchema, async (args) => {
        const esbuild = await getEsbuild();
        try {
            const opts = prepareBuildOptions(args);
            opts.bundle = opts.bundle ?? true;
            opts.write = opts.write ?? false;
            const ctx = await esbuild.context(opts);
            try {
                const result = await ctx.rebuild();
                const output = {
                    outputFiles: (result.outputFiles ?? []).map((f) => ({
                        path: f.path,
                        text: f.text,
                    })),
                    warnings: result.warnings,
                    errors: result.errors,
                };
                if (result.mangleCache)
                    output.mangleCache = result.mangleCache;
                return {
                    content: [{
                            type: "text",
                            text: JSON.stringify(output, null, 2),
                        }],
                };
            }
            finally {
                await ctx.dispose();
            }
        }
        catch (err) {
            return formatErrorResponse(err);
        }
    });
}
