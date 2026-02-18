import { getEsbuild } from "../esbuild-api.js";
import { formatErrorResponse } from "../errors.js";
import { BuildOnlySchema, CommonSchema, ServeOnlySchema, prepareBuildOptions } from "./schemas.js";
const ServeSchema = { ...BuildOnlySchema, ...CommonSchema, ...ServeOnlySchema };
export function registerServeTool(server) {
    server.tool("esbuild_serve", "Start an esbuild development server with live reload. Returns the host and port.", ServeSchema, async (args) => {
        const esbuild = await getEsbuild();
        try {
            const { port, host, servedir, keyfile, certfile, fallback, ...buildArgs } = prepareBuildOptions(args);
            buildArgs.bundle = buildArgs.bundle ?? true;
            buildArgs.write = buildArgs.write ?? true;
            const ctx = await esbuild.context(buildArgs);
            const serveOptions = {};
            if (port !== undefined)
                serveOptions.port = port;
            if (host !== undefined)
                serveOptions.host = host;
            if (servedir !== undefined)
                serveOptions.servedir = servedir;
            if (keyfile !== undefined)
                serveOptions.keyfile = keyfile;
            if (certfile !== undefined)
                serveOptions.certfile = certfile;
            if (fallback !== undefined)
                serveOptions.fallback = fallback;
            const result = await ctx.serve(serveOptions);
            return {
                content: [{
                        type: "text",
                        text: JSON.stringify({ hosts: result.hosts, port: result.port }, null, 2),
                    }],
            };
        }
        catch (err) {
            return formatErrorResponse(err);
        }
    });
}
