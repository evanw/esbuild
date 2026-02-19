import type { Plugin } from "esbuild-wasm";

export interface NpmResolverOptions {
    /**
     * Dependencies from package.json (name → version).
     * Used to pin CDN URLs to specific versions.
     */
    dependencies?: Record<string, string>;

    /**
     * CDN base URL. Default: "https://esm.sh"
     */
    cdnUrl?: string;

    /**
     * Resolution mode:
     * - "external": Rewrite bare imports to CDN URLs and mark as external (fast, output has CDN imports)
     * - "bundle": Fetch ESM from CDN and inline into bundle (slower, fully self-contained output)
     * Default: "external"
     */
    mode?: "external" | "bundle";

    /**
     * Target environment for esm.sh bundles.
     * Default: "es2022"
     */
    target?: string;
}

/** Packages that should not be resolved via CDN (handled by the runtime) */
const BUILTIN_MODULES = new Set([
    "assert", "buffer", "child_process", "cluster", "console", "constants",
    "crypto", "dgram", "dns", "domain", "events", "fs", "http", "http2",
    "https", "module", "net", "os", "path", "perf_hooks", "process",
    "punycode", "querystring", "readline", "repl", "stream", "string_decoder",
    "sys", "timers", "tls", "tty", "url", "util", "v8", "vm", "wasi",
    "worker_threads", "zlib",
]);

function isBuiltinModule(name: string): boolean {
    return BUILTIN_MODULES.has(name) || name.startsWith("node:");
}

function isBareImport(importPath: string): boolean {
    return (
        !importPath.startsWith(".") &&
        !importPath.startsWith("/") &&
        !importPath.startsWith("http://") &&
        !importPath.startsWith("https://") &&
        !importPath.startsWith("data:")
    );
}

/**
 * Parse a bare import into package name and subpath.
 * Examples:
 *   "react"           → { name: "react", subpath: "" }
 *   "react/jsx-runtime" → { name: "react", subpath: "/jsx-runtime" }
 *   "@org/pkg"        → { name: "@org/pkg", subpath: "" }
 *   "@org/pkg/utils"  → { name: "@org/pkg", subpath: "/utils" }
 */
function parseBareImport(importPath: string): { name: string; subpath: string } {
    if (importPath.startsWith("@")) {
        const parts = importPath.split("/");
        const name = parts.slice(0, 2).join("/");
        const subpath = parts.length > 2 ? "/" + parts.slice(2).join("/") : "";
        return { name, subpath };
    }

    const slashIndex = importPath.indexOf("/");
    if (slashIndex === -1) {
        return { name: importPath, subpath: "" };
    }

    return {
        name: importPath.slice(0, slashIndex),
        subpath: importPath.slice(slashIndex),
    };
}

/**
 * Build a CDN URL for a package.
 * Examples:
 *   buildCdnUrl("https://esm.sh", "react", "18.2.0", "", "es2022")
 *     → "https://esm.sh/react@18.2.0?target=es2022"
 *   buildCdnUrl("https://esm.sh", "react", "^18.0.0", "/jsx-runtime", "es2022")
 *     → "https://esm.sh/react@18.0.0/jsx-runtime?target=es2022"
 */
export function buildCdnUrl(
    cdnBase: string,
    packageName: string,
    version: string | undefined,
    subpath: string,
    target: string
): string {
    const versionSuffix = version ? `@${cleanVersion(version)}` : "";
    return `${cdnBase}/${packageName}${versionSuffix}${subpath}?target=${target}`;
}

/** Strip semver range prefixes like ^ and ~ */
function cleanVersion(version: string): string {
    return version.replace(/^[~^>=<]/, "");
}

export function createNpmResolverPlugin(options: NpmResolverOptions = {}): Plugin {
    const {
        dependencies = {},
        cdnUrl = "https://esm.sh",
        mode = "external",
        target = "es2022",
    } = options;

    // Merge dependencies and devDependencies-style version maps
    const allDeps = { ...dependencies };

    // Cache for bundled mode
    const bundleCache = new Map<string, string>();

    return {
        name: "npm-resolver",
        setup(build) {
            const NAMESPACE = "npm-cdn";

            build.onResolve({ filter: /.*/ }, async (args) => {
                // Only handle bare imports
                if (!isBareImport(args.path)) return undefined;

                // Skip Node builtins
                if (isBuiltinModule(args.path)) {
                    return { path: args.path, external: true };
                }

                // Skip if already in our namespace (sub-dependencies from CDN)
                if (args.namespace === NAMESPACE) {
                    // Resolve relative CDN imports
                    if (args.path.startsWith("./") || args.path.startsWith("../") || args.path.startsWith("/")) {
                        const resolvedUrl = new URL(args.path, `${cdnUrl}/${args.importer}`).href;
                        if (mode === "external") {
                            return { path: resolvedUrl, external: true };
                        }
                        return { path: resolvedUrl, namespace: NAMESPACE };
                    }
                }

                const { name, subpath } = parseBareImport(args.path);
                const version = allDeps[name];
                const url = buildCdnUrl(cdnUrl, name, version, subpath, target);

                if (mode === "external") {
                    return { path: url, external: true };
                }

                // Bundle mode: fetch and inline
                return { path: url, namespace: NAMESPACE };
            });

            if (mode === "bundle") {
                build.onLoad({ filter: /.*/, namespace: NAMESPACE }, async (args) => {
                    if (bundleCache.has(args.path)) {
                        return { contents: bundleCache.get(args.path)!, loader: "js" };
                    }

                    const response = await fetch(args.path);
                    if (!response.ok) {
                        return {
                            errors: [{ text: `Failed to fetch ${args.path}: ${response.status} ${response.statusText}` }],
                        };
                    }

                    const contents = await response.text();
                    bundleCache.set(args.path, contents);

                    return {
                        contents,
                        loader: "js",
                        resolveDir: ".",
                    };
                });
            }
        },
    };
}
