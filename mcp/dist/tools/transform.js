import { getEsbuild } from "../esbuild-api.js";
import { formatErrorResponse } from "../errors.js";
import { TransformOnlySchema, CommonSchema, prepareBuildOptions } from "./schemas.js";
const TransformSchema = { ...TransformOnlySchema, ...CommonSchema };
export function registerTransformTool(server) {
    server.tool("esbuild_transform", "Transform source code (TypeScript, JSX, CSS, etc.) to JavaScript or CSS using esbuild", TransformSchema, async (args) => {
        const esbuild = await getEsbuild();
        try {
            const { code, ...rest } = prepareBuildOptions(args);
            rest.loader = rest.loader ?? "ts";
            const result = await esbuild.transform(code, rest);
            const output = {
                code: result.code,
                warnings: result.warnings,
            };
            if (result.map)
                output.map = result.map;
            if (result.mangleCache)
                output.mangleCache = result.mangleCache;
            return {
                content: [
                    {
                        type: "text",
                        text: JSON.stringify(output, null, 2),
                    },
                ],
            };
        }
        catch (err) {
            return formatErrorResponse(err);
        }
    });
}
