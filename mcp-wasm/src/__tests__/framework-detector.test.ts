import { describe, it, expect } from "vitest";
import { detectFramework, mergeConfig } from "../framework-detector.js";

describe("detectFramework", () => {
    it("detects Vite from vite.config.ts", () => {
        const files = new Set(["vite.config.ts", "src/main.tsx", "package.json"]);
        const pkg = { devDependencies: { vite: "^5.0.0" } };
        const result = detectFramework(files, pkg);
        expect(result.name).toBe("vite");
        expect(result.entryPoint).toBe("src/main.tsx");
        expect(result.config.jsx).toBe("automatic");
    });

    it("detects Vite from devDependencies", () => {
        const files = new Set(["src/main.ts", "package.json"]);
        const pkg = { devDependencies: { vite: "^5.0.0" } };
        const result = detectFramework(files, pkg);
        expect(result.name).toBe("vite");
    });

    it("detects Vite with React jsxImportSource", () => {
        const files = new Set(["vite.config.ts", "src/main.tsx"]);
        const pkg = { dependencies: { react: "^18.0.0" }, devDependencies: { vite: "^5.0.0" } };
        const result = detectFramework(files, pkg);
        expect(result.config.jsxImportSource).toBe("react");
    });

    it("detects Vite with Preact jsxImportSource", () => {
        const files = new Set(["vite.config.ts", "src/main.tsx"]);
        const pkg = { dependencies: { preact: "^10.0.0" }, devDependencies: { vite: "^5.0.0" } };
        const result = detectFramework(files, pkg);
        expect(result.config.jsxImportSource).toBe("preact");
    });

    it("detects Next.js from next.config.js", () => {
        const files = new Set(["next.config.js", "src/app/page.tsx"]);
        const pkg = { dependencies: { next: "^14.0.0", react: "^18.0.0" } };
        const result = detectFramework(files, pkg);
        expect(result.name).toBe("nextjs");
        expect(result.entryPoint).toBe("src/app/page.tsx");
        expect(result.config.external).toContain("next");
    });

    it("detects Next.js from dependencies", () => {
        const files = new Set(["pages/index.tsx"]);
        const pkg = { dependencies: { next: "^14.0.0" } };
        const result = detectFramework(files, pkg);
        expect(result.name).toBe("nextjs");
        expect(result.entryPoint).toBe("pages/index.tsx");
    });

    it("detects CRA from react-scripts", () => {
        const files = new Set(["src/index.tsx", "public/index.html"]);
        const pkg = { dependencies: { "react-scripts": "5.0.0", react: "^18.0.0" } };
        const result = detectFramework(files, pkg);
        expect(result.name).toBe("cra");
        expect(result.entryPoint).toBe("src/index.tsx");
        expect(result.config.define?.["process.env.NODE_ENV"]).toBe('"development"');
    });

    it("detects Remix from @remix-run/dev", () => {
        const files = new Set(["app/root.tsx", "package.json"]);
        const pkg = { devDependencies: { "@remix-run/dev": "^2.0.0" } };
        const result = detectFramework(files, pkg);
        expect(result.name).toBe("remix");
        expect(result.entryPoint).toBe("app/root.tsx");
    });

    it("falls back to plain for unknown projects", () => {
        const files = new Set(["src/index.ts", "package.json"]);
        const pkg = { dependencies: {} };
        const result = detectFramework(files, pkg);
        expect(result.name).toBe("plain");
        expect(result.entryPoint).toBe("src/index.ts");
    });

    it("detects plain with React auto-JSX", () => {
        const files = new Set(["src/index.tsx"]);
        const pkg = { dependencies: { react: "^18.0.0" } };
        const result = detectFramework(files, pkg);
        expect(result.name).toBe("plain");
        expect(result.config.jsx).toBe("automatic");
        expect(result.config.jsxImportSource).toBe("react");
    });

    it("works with arrays instead of Set", () => {
        const files = ["vite.config.ts", "src/main.tsx"];
        const pkg = { devDependencies: { vite: "^5.0.0" } };
        const result = detectFramework(files, pkg);
        expect(result.name).toBe("vite");
    });

    it("all frameworks include metafile and sourcemap in config", () => {
        const testCases: Array<{ files: Set<string>; pkg: { dependencies?: Record<string, string>; devDependencies?: Record<string, string> } }> = [
            { files: new Set(["vite.config.ts", "src/main.tsx"]), pkg: { devDependencies: { vite: "^5" } } },
            { files: new Set(["next.config.js", "src/app/page.tsx"]), pkg: { dependencies: { next: "^14" } } },
            { files: new Set(["src/index.tsx"]), pkg: { dependencies: { "react-scripts": "5" } } },
            { files: new Set(["src/index.ts"]), pkg: {} },
        ];

        for (const { files, pkg } of testCases) {
            const result = detectFramework(files, pkg);
            expect(result.config.metafile).toBe(true);
            expect(result.config.sourcemap).toBe(true);
            expect(result.config.bundle).toBe(true);
            expect(result.config.write).toBe(false);
        }
    });
});

describe("mergeConfig", () => {
    it("user overrides take precedence", () => {
        const detected = { format: "esm" as const, target: "es2020", minify: false };
        const overrides = { minify: true, target: "esnext" };
        const merged = mergeConfig(detected, overrides);
        expect(merged.minify).toBe(true);
        expect(merged.target).toBe("esnext");
        expect(merged.format).toBe("esm");
    });

    it("preserves detected config when no overrides", () => {
        const detected = { format: "esm" as const, jsx: "automatic" as const };
        const merged = mergeConfig(detected, {});
        expect(merged).toEqual(detected);
    });
});
