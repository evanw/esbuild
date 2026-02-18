import { getEsbuild } from "../esbuild-api.js";
import { formatErrorResponse } from "../errors.js";
import { BuildOnlySchema, CommonSchema, prepareBuildOptions } from "./schemas.js";
const WatchSchema = { ...BuildOnlySchema, ...CommonSchema };
export function registerWatchTool(server) {
    server.tool("esbuild_watch", "Start esbuild in watch mode for automatic rebuilds on file changes. Returns the initial build result.", WatchSchema, async (args) => {
        const esbuild = await getEsbuild();
        try {
            const opts = prepareBuildOptions(args);
            opts.bundle = opts.bundle ?? true;
            opts.write = opts.write ?? false;
            const ctx = await esbuild.context(opts);
            await ctx.watch();
            const result = await ctx.rebuild();
            return {
                content: [{
                        type: "text",
                        text: JSON.stringify({
                            watching: true,
                            outputFiles: (result.outputFiles ?? []).map((f) => ({
                                path: f.path,
                                text: f.text,
                            })),
                            warnings: result.warnings,
                            errors: result.errors,
                        }, null, 2),
                    }],
            };
        }
        catch (err) {
            return formatErrorResponse(err);
        }
    });
}
