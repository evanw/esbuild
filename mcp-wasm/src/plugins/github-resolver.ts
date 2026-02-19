import type { Plugin, Loader } from "esbuild-wasm";
import { posix as path } from "node:path";

export interface GitHubResolverOptions {
  owner: string;
  repo: string;
  ref: string;
  token?: string;
  basePath?: string;
}

interface CacheEntry {
  content: string;
  loader: Loader;
}

const EXTENSION_TO_LOADER: Record<string, Loader> = {
  ".ts": "ts",
  ".tsx": "tsx",
  ".js": "js",
  ".jsx": "jsx",
  ".mjs": "js",
  ".mts": "ts",
  ".cjs": "js",
  ".cts": "ts",
  ".css": "css",
  ".json": "json",
  ".txt": "text",
  ".svg": "text",
  ".html": "text",
};

const RESOLVE_EXTENSIONS = [".tsx", ".ts", ".jsx", ".js", ".mjs", ".mts", ".json", ".css"];

/**
 * Parse a GitHub URL into owner, repo, ref, and optional subpath.
 * Supports:
 *   https://github.com/owner/repo
 *   https://github.com/owner/repo/tree/branch
 *   https://github.com/owner/repo/tree/branch/subpath
 *   owner/repo
 */
export function parseGitHubUrl(url: string): {
  owner: string;
  repo: string;
  ref: string;
  basePath: string;
} {
  // Handle shorthand "owner/repo"
  if (!url.includes("://") && /^[a-zA-Z0-9_.-]+\/[a-zA-Z0-9_.-]+/.test(url)) {
    const parts = url.split("/");
    return {
      owner: parts[0],
      repo: parts[1],
      ref: parts[3] || "main",
      basePath: parts.slice(4).join("/") || "",
    };
  }

  const parsed = new URL(url);
  const segments = parsed.pathname.replace(/^\//, "").replace(/\.git$/, "").split("/");

  if (segments.length < 2) {
    throw new Error(`Invalid GitHub URL: ${url}`);
  }

  const owner = segments[0];
  const repo = segments[1];
  // /tree/branch/subpath or /blob/branch/subpath
  const ref = segments.length >= 4 ? segments[3] : "main";
  const basePath = segments.length >= 5 ? segments.slice(4).join("/") : "";

  return { owner, repo, ref, basePath };
}

function getLoader(filePath: string): Loader {
  const ext = path.extname(filePath).toLowerCase();
  return EXTENSION_TO_LOADER[ext] || "default";
}

function getRawUrl(owner: string, repo: string, ref: string, filePath: string): string {
  return `https://raw.githubusercontent.com/${owner}/${repo}/${ref}/${filePath}`;
}

export function createGitHubResolverPlugin(options: GitHubResolverOptions): Plugin {
  const { owner, repo, ref, token, basePath = "" } = options;
  const cache = new Map<string, CacheEntry>();
  const notFoundCache = new Set<string>();

  async function fetchFile(filePath: string): Promise<CacheEntry | null> {
    if (notFoundCache.has(filePath)) return null;
    if (cache.has(filePath)) return cache.get(filePath)!;

    const url = getRawUrl(owner, repo, ref, filePath);
    const headers: Record<string, string> = {
      "Accept": "application/vnd.github.v3.raw",
    };
    if (token) {
      headers["Authorization"] = `token ${token}`;
    }

    const response = await fetch(url, { headers });

    if (!response.ok) {
      if (response.status === 404) {
        notFoundCache.add(filePath);
        return null;
      }
      throw new Error(
        `GitHub fetch failed for ${filePath}: ${response.status} ${response.statusText}`
      );
    }

    const content = await response.text();
    const loader = getLoader(filePath);
    const entry: CacheEntry = { content, loader };
    cache.set(filePath, entry);
    return entry;
  }

  /**
   * Try to resolve a file path, attempting extension resolution if needed.
   * Returns the resolved full path and cache entry, or null if not found.
   */
  async function resolveFile(
    filePath: string
  ): Promise<{ resolvedPath: string; entry: CacheEntry } | null> {
    // Try exact match first
    const exact = await fetchFile(filePath);
    if (exact) return { resolvedPath: filePath, entry: exact };

    // Try adding extensions
    for (const ext of RESOLVE_EXTENSIONS) {
      const withExt = filePath + ext;
      const entry = await fetchFile(withExt);
      if (entry) return { resolvedPath: withExt, entry };
    }

    // Try as directory with /index.{ext}
    for (const ext of RESOLVE_EXTENSIONS) {
      const indexPath = path.join(filePath, `index${ext}`);
      const entry = await fetchFile(indexPath);
      if (entry) return { resolvedPath: indexPath, entry };
    }

    return null;
  }

  return {
    name: "github-resolver",
    setup(build) {
      // Namespace for GitHub-resolved files
      const NAMESPACE = "github";

      // Resolve the initial entry points
      build.onResolve({ filter: /.*/ }, async (args) => {
        // Skip bare imports (npm packages) — let npm-resolver handle them
        if (
          !args.path.startsWith(".") &&
          !args.path.startsWith("/") &&
          args.namespace !== NAMESPACE
        ) {
          return undefined;
        }

        let targetPath: string;

        if (args.namespace === NAMESPACE) {
          // Resolve relative to the importer within the repo
          const importerDir = path.dirname(args.importer);
          targetPath = path.normalize(path.join(importerDir, args.path));
        } else if (args.path.startsWith("/")) {
          // Absolute path within repo
          targetPath = path.join(basePath, args.path.slice(1));
        } else if (args.path.startsWith(".")) {
          // Relative to entry — resolve against basePath
          targetPath = path.normalize(path.join(basePath, args.path));
        } else {
          return undefined;
        }

        // Remove leading slash if any
        targetPath = targetPath.replace(/^\//, "");

        const result = await resolveFile(targetPath);
        if (result) {
          return {
            path: result.resolvedPath,
            namespace: NAMESPACE,
          };
        }

        return undefined;
      });

      // Load files from the GitHub cache
      build.onLoad({ filter: /.*/, namespace: NAMESPACE }, async (args) => {
        const entry = cache.get(args.path);
        if (entry) {
          return {
            contents: entry.content,
            loader: entry.loader,
            resolveDir: path.dirname(args.path),
          };
        }

        // Shouldn't happen, but try fetching
        const fetched = await fetchFile(args.path);
        if (fetched) {
          return {
            contents: fetched.content,
            loader: fetched.loader,
            resolveDir: path.dirname(args.path),
          };
        }

        return { errors: [{ text: `File not found in repo: ${args.path}` }] };
      });
    },
  };
}
