import { describe, it, expect, vi, beforeEach } from "vitest";
import { parseGitHubUrl, createGitHubResolverPlugin } from "../plugins/github-resolver.js";

describe("parseGitHubUrl", () => {
    it("parses a simple GitHub URL", () => {
        const result = parseGitHubUrl("https://github.com/facebook/react");
        expect(result).toEqual({
            owner: "facebook",
            repo: "react",
            ref: "main",
            basePath: "",
        });
    });

    it("parses a GitHub URL with branch", () => {
        const result = parseGitHubUrl("https://github.com/facebook/react/tree/v18.2.0");
        expect(result).toEqual({
            owner: "facebook",
            repo: "react",
            ref: "v18.2.0",
            basePath: "",
        });
    });

    it("parses a GitHub URL with branch and subpath", () => {
        const result = parseGitHubUrl(
            "https://github.com/vitejs/vite/tree/main/playground/react"
        );
        expect(result).toEqual({
            owner: "vitejs",
            repo: "vite",
            ref: "main",
            basePath: "playground/react",
        });
    });

    it("parses shorthand owner/repo", () => {
        const result = parseGitHubUrl("facebook/react");
        expect(result).toEqual({
            owner: "facebook",
            repo: "react",
            ref: "main",
            basePath: "",
        });
    });

    it("strips .git suffix", () => {
        const result = parseGitHubUrl("https://github.com/user/repo.git");
        expect(result).toEqual({
            owner: "user",
            repo: "repo",
            ref: "main",
            basePath: "",
        });
    });

    it("throws on invalid URLs", () => {
        expect(() => parseGitHubUrl("https://github.com/")).toThrow("Invalid GitHub URL");
    });
});

describe("createGitHubResolverPlugin", () => {
    it("returns a valid esbuild plugin", () => {
        const plugin = createGitHubResolverPlugin({
            owner: "test",
            repo: "repo",
            ref: "main",
        });
        expect(plugin.name).toBe("github-resolver");
        expect(plugin.setup).toBeInstanceOf(Function);
    });

    it("creates a plugin with all options", () => {
        const plugin = createGitHubResolverPlugin({
            owner: "org",
            repo: "project",
            ref: "v1.0.0",
            token: "ghp_test123",
            basePath: "packages/core",
        });
        expect(plugin.name).toBe("github-resolver");
    });
});
