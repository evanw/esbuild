import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { getEsbuildWasm } from "../wasm-api.js";
import { formatErrorResponse } from "../errors.js";
import { parseGitHubUrl, createGitHubResolverPlugin } from "../plugins/github-resolver.js";
import { createNpmResolverPlugin } from "../plugins/npm-resolver.js";
import { detectFramework, mergeConfig, type FrameworkName } from "../framework-detector.js";

const BuildFromGithubSchema = {
    repoUrl: z.string().describe(
        'GitHub repo URL (e.g. "https://github.com/user/repo" or "https://github.com/user/repo/tree/branch/subpath")'
    ),
    ref: z.string().optional().describe(
        "Branch, tag, or commit SHA (default: parsed from URL or 'main')"
    ),
    entryPoint: z.string().optional().describe(
        "Override auto-detected entry point (e.g. 'src/main.tsx')"
    ),
    framework: z.enum(["vite", "nextjs", "cra", "remix", "plain"]).optional().describe(
        "Override auto-detected framework"
    ),
    npmMode: z.enum(["external", "bundle"]).optional().describe(
        'How to handle npm dependencies: "external" rewrites to CDN URLs (fast), "bundle" inlines them (self-contained). Default: "external"'
    ),
    minify: z.boolean().optional().describe("Minify output (default: false)"),
    target: z.string().optional().describe('Target environment (default: "es2020")'),
    format: z.enum(["iife", "cjs", "esm"]).optional().describe('Output format (default: "esm")'),
    define: z.record(z.string(), z.string()).optional().describe("Global identifier replacements"),
    external: z.array(z.string()).optional().describe("Additional external packages"),
    githubToken: z.string().optional().describe(
        "GitHub personal access token (optional, increases rate limit from 60 to 5000 req/hr)"
    ),
};

/**
 * Fetch a file from GitHub, returning null if not found.
 */
async function fetchGitHubFile(
    owner: string,
    repo: string,
    ref: string,
    filePath: string,
    token?: string
): Promise<string | null> {
    const url = `https://raw.githubusercontent.com/${owner}/${repo}/${ref}/${filePath}`;
    const headers: Record<string, string> = {};
    if (token) headers["Authorization"] = `token ${token}`;

    const response = await fetch(url, { headers });
    if (!response.ok) return null;
    return response.text();
}

/**
 * Fetch the directory listing from GitHub API to get file paths.
 */
async function fetchRepoTree(
    owner: string,
    repo: string,
    ref: string,
    basePath: string,
    token?: string
): Promise<string[]> {
    const apiUrl = `https://api.github.com/repos/${owner}/${repo}/git/trees/${ref}?recursive=1`;
    const headers: Record<string, string> = {
        "Accept": "application/vnd.github.v3+json",
    };
    if (token) headers["Authorization"] = `token ${token}`;

    const response = await fetch(apiUrl, { headers });
    if (!response.ok) {
        throw new Error(`Failed to fetch repo tree: ${response.status} ${response.statusText}`);
    }

    const data = await response.json() as { tree: Array<{ path: string; type: string }> };
    let files = data.tree
        .filter((item) => item.type === "blob")
        .map((item) => item.path);

    // Filter to basePath if specified
    if (basePath) {
        files = files
            .filter((f) => f.startsWith(basePath + "/") || f === basePath)
            .map((f) => f.slice(basePath.length + 1))
            .filter((f) => f.length > 0);
    }

    return files;
}

export function registerBuildFromGithubTool(server: McpServer): void {
    server.tool(
        "build_from_github",
        "Build a frontend application from a GitHub repository URL. Auto-detects the framework (Vite, Next.js, CRA, Remix, or plain) and bundles using esbuild-wasm entirely in-memory.",
        BuildFromGithubSchema,
        async (args) => {
            try {
                const esbuild = await getEsbuildWasm();

                // 1. Parse GitHub URL
                const parsed = parseGitHubUrl(args.repoUrl);
                const owner = parsed.owner;
                const repo = parsed.repo;
                const ref = args.ref || parsed.ref;
                const basePath = parsed.basePath;
                const token = args.githubToken;

                // 2. Fetch repo file tree
                const files = await fetchRepoTree(owner, repo, ref, basePath, token);

                // 3. Fetch package.json
                const pkgJsonPath = basePath ? `${basePath}/package.json` : "package.json";
                const pkgJsonContent = await fetchGitHubFile(owner, repo, ref, pkgJsonPath, token);
                const packageJson = pkgJsonContent ? JSON.parse(pkgJsonContent) : {};

                // 4. Detect framework or use override
                const detected = detectFramework(new Set(files), packageJson);
                const frameworkName: FrameworkName = args.framework || detected.name;

                // Re-detect with forced framework if overridden
                const frameworkResult = args.framework
                    ? { ...detected, name: args.framework }
                    : detected;

                // 5. Determine entry point
                const entryPoint = args.entryPoint || frameworkResult.entryPoint;

                // 6. Build config with overrides
                const userOverrides: Record<string, unknown> = {};
                if (args.minify !== undefined) userOverrides.minify = args.minify;
                if (args.target) userOverrides.target = args.target;
                if (args.format) userOverrides.format = args.format;
                if (args.define) userOverrides.define = { ...(frameworkResult.config.define ?? {}), ...args.define };
                if (args.external) {
                    userOverrides.external = [
                        ...(frameworkResult.config.external ?? []),
                        ...args.external,
                    ];
                }

                const buildConfig = mergeConfig(frameworkResult.config, {
                    ...userOverrides,
                    entryPoints: [entryPoint],
                });

                // 7. Create plugins
                const githubPlugin = createGitHubResolverPlugin({
                    owner,
                    repo,
                    ref,
                    token,
                    basePath,
                });

                const allDeps = {
                    ...(packageJson.devDependencies ?? {}),
                    ...(packageJson.dependencies ?? {}),
                };

                const npmPlugin = createNpmResolverPlugin({
                    dependencies: allDeps,
                    mode: args.npmMode || "external",
                    target: args.target || "es2020",
                });

                // 8. Run the build
                const result = await esbuild.build({
                    ...buildConfig,
                    plugins: [githubPlugin, npmPlugin],
                    write: false,
                    bundle: true,
                    metafile: true,
                } as any);

                // 9. Format output
                const output = {
                    framework: {
                        detected: frameworkName,
                        description: frameworkResult.description,
                        entryPoint,
                    },
                    repo: {
                        owner,
                        repo,
                        ref,
                        basePath: basePath || undefined,
                        fileCount: files.length,
                    },
                    outputFiles: (result.outputFiles ?? []).map((f) => ({
                        path: f.path,
                        size: f.contents.length,
                        text: f.text,
                    })),
                    metafile: result.metafile,
                    warnings: result.warnings,
                    errors: result.errors,
                };

                return {
                    content: [{
                        type: "text" as const,
                        text: JSON.stringify(output, null, 2),
                    }],
                };
            } catch (err: unknown) {
                return formatErrorResponse(err);
            }
        }
    );
}
