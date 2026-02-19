import type { BuildOptions } from "esbuild-wasm";

export type FrameworkName = "vite" | "nextjs" | "cra" | "remix" | "plain";

export interface DetectedFramework {
    name: FrameworkName;
    entryPoint: string;
    config: BuildOptions;
    description: string;
}

interface PackageJson {
    dependencies?: Record<string, string>;
    devDependencies?: Record<string, string>;
    scripts?: Record<string, string>;
}

/**
 * Detect the framework used by a project based on its file listing and package.json.
 *
 * @param files Set of file paths present in the repo (relative to project root)
 * @param packageJson Parsed package.json contents
 * @returns Detected framework information with an appropriate esbuild config
 */
export function detectFramework(
    files: Set<string> | string[],
    packageJson: PackageJson
): DetectedFramework {
    const fileSet = files instanceof Set ? files : new Set(files);
    const deps = packageJson.dependencies ?? {};
    const devDeps = packageJson.devDependencies ?? {};
    const allDeps = { ...devDeps, ...deps };

    // 1. Vite
    if (
        fileSet.has("vite.config.ts") ||
        fileSet.has("vite.config.js") ||
        fileSet.has("vite.config.mjs") ||
        allDeps["vite"]
    ) {
        const entryPoint = findFirst(fileSet, [
            "src/main.tsx",
            "src/main.ts",
            "src/main.jsx",
            "src/main.js",
            "src/index.tsx",
            "src/index.ts",
            "index.html",
        ]);

        return {
            name: "vite",
            entryPoint,
            description: "Vite project detected",
            config: buildViteConfig(entryPoint, allDeps),
        };
    }

    // 2. Next.js
    if (
        fileSet.has("next.config.ts") ||
        fileSet.has("next.config.js") ||
        fileSet.has("next.config.mjs") ||
        deps["next"]
    ) {
        const entryPoint = findFirst(fileSet, [
            "src/app/page.tsx",
            "src/app/page.ts",
            "src/pages/index.tsx",
            "src/pages/index.ts",
            "app/page.tsx",
            "pages/index.tsx",
            "pages/index.js",
        ]);

        return {
            name: "nextjs",
            entryPoint,
            description: "Next.js project detected",
            config: buildNextConfig(entryPoint, allDeps),
        };
    }

    // 3. Create React App
    if (deps["react-scripts"] || devDeps["react-scripts"]) {
        const entryPoint = findFirst(fileSet, [
            "src/index.tsx",
            "src/index.ts",
            "src/index.jsx",
            "src/index.js",
        ]);

        return {
            name: "cra",
            entryPoint,
            description: "Create React App project detected",
            config: buildCraConfig(entryPoint, allDeps),
        };
    }

    // 4. Remix
    if (
        fileSet.has("remix.config.js") ||
        fileSet.has("remix.config.ts") ||
        allDeps["@remix-run/dev"]
    ) {
        const entryPoint = findFirst(fileSet, [
            "app/root.tsx",
            "app/root.jsx",
            "app/entry.client.tsx",
            "app/entry.client.jsx",
        ]);

        return {
            name: "remix",
            entryPoint,
            description: "Remix project detected",
            config: buildRemixConfig(entryPoint, allDeps),
        };
    }

    // 5. Plain / custom
    const entryPoint = findFirst(fileSet, [
        "src/index.tsx",
        "src/index.ts",
        "src/index.jsx",
        "src/index.js",
        "src/main.tsx",
        "src/main.ts",
        "src/main.jsx",
        "src/main.js",
        "index.ts",
        "index.js",
        "main.ts",
        "main.js",
    ]);

    return {
        name: "plain",
        entryPoint,
        description: "Plain/custom project detected",
        config: buildPlainConfig(entryPoint, allDeps),
    };
}

function findFirst(files: Set<string>, candidates: string[]): string {
    for (const candidate of candidates) {
        if (files.has(candidate)) return candidate;
    }
    // Return the first candidate as a fallback
    return candidates[0];
}

/**
 * Merge detected config with user-provided overrides.
 * User overrides always take precedence.
 */
export function mergeConfig(
    detected: BuildOptions,
    overrides: Partial<BuildOptions>
): BuildOptions {
    return { ...detected, ...overrides };
}

// --- Framework-specific esbuild configs ---

function baseConfig(entryPoint: string): BuildOptions {
    return {
        entryPoints: [entryPoint],
        bundle: true,
        write: false,
        metafile: true,
        target: "es2020",
        format: "esm",
        sourcemap: true,
        resolveExtensions: [".tsx", ".ts", ".jsx", ".js", ".mjs", ".json", ".css"],
        loader: {
            ".png": "dataurl",
            ".jpg": "dataurl",
            ".jpeg": "dataurl",
            ".gif": "dataurl",
            ".svg": "text",
            ".woff": "dataurl",
            ".woff2": "dataurl",
            ".ttf": "dataurl",
            ".eot": "dataurl",
        },
    };
}

function buildViteConfig(entryPoint: string, deps: Record<string, string>): BuildOptions {
    return {
        ...baseConfig(entryPoint),
        jsx: "automatic",
        jsxImportSource: deps["react"] ? "react" : deps["preact"] ? "preact" : undefined,
        define: {
            "import.meta.env.DEV": "true",
            "import.meta.env.PROD": "false",
            "import.meta.env.MODE": '"development"',
        },
    };
}

function buildNextConfig(entryPoint: string, _deps: Record<string, string>): BuildOptions {
    return {
        ...baseConfig(entryPoint),
        jsx: "automatic",
        jsxImportSource: "react",
        platform: "browser",
        external: ["next", "next/*", "react", "react-dom", "react/*", "react-dom/*"],
        define: {
            "process.env.NODE_ENV": '"development"',
        },
    };
}

function buildCraConfig(entryPoint: string, _deps: Record<string, string>): BuildOptions {
    return {
        ...baseConfig(entryPoint),
        jsx: "automatic",
        jsxImportSource: "react",
        define: {
            "process.env.NODE_ENV": '"development"',
            "process.env.PUBLIC_URL": '""',
        },
    };
}

function buildRemixConfig(entryPoint: string, _deps: Record<string, string>): BuildOptions {
    return {
        ...baseConfig(entryPoint),
        jsx: "automatic",
        jsxImportSource: "react",
        platform: "browser",
        external: ["@remix-run/*", "react", "react-dom"],
        define: {
            "process.env.NODE_ENV": '"development"',
        },
    };
}

function buildPlainConfig(entryPoint: string, deps: Record<string, string>): BuildOptions {
    const config = baseConfig(entryPoint);

    // Auto-detect JSX if React or Preact is a dependency
    if (deps["react"] || deps["preact"]) {
        config.jsx = "automatic";
        config.jsxImportSource = deps["react"] ? "react" : "preact";
    }

    return config;
}
