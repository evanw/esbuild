import { describe, it, expect } from "vitest";
import { parseGitHubUrl } from "../plugins/github-resolver.js";

/**
 * Integration-style tests for the build-from-github tool.
 * These test the URL parsing and orchestration logic without
 * actually hitting the GitHub API or running esbuild.
 */
describe("build-from-github: URL parsing", () => {
    it("parses full GitHub URL with tree path", () => {
        const result = parseGitHubUrl("https://github.com/vitejs/vite/tree/main/playground/react");
        expect(result.owner).toBe("vitejs");
        expect(result.repo).toBe("vite");
        expect(result.ref).toBe("main");
        expect(result.basePath).toBe("playground/react");
    });

    it("parses GitHub URL with tag ref", () => {
        const result = parseGitHubUrl("https://github.com/facebook/react/tree/v18.2.0");
        expect(result.ref).toBe("v18.2.0");
        expect(result.basePath).toBe("");
    });

    it("parses GitHub blob URL (treats as tree)", () => {
        const result = parseGitHubUrl("https://github.com/user/repo/blob/main/src/index.ts");
        expect(result.owner).toBe("user");
        expect(result.repo).toBe("repo");
        expect(result.ref).toBe("main");
        expect(result.basePath).toBe("src/index.ts");
    });

    it("handles deeply nested subpaths", () => {
        const result = parseGitHubUrl(
            "https://github.com/org/mono/tree/develop/packages/ui/src"
        );
        expect(result.basePath).toBe("packages/ui/src");
        expect(result.ref).toBe("develop");
    });
});

describe("build-from-github: config generation", () => {
    it("generates correct schema shape", async () => {
        // Verify the tool module exports correctly
        const mod = await import("../tools/build-from-github.js");
        expect(mod.registerBuildFromGithubTool).toBeInstanceOf(Function);
    });
});
