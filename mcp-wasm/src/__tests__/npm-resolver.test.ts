import { describe, it, expect } from "vitest";
import { createNpmResolverPlugin, buildCdnUrl } from "../plugins/npm-resolver.js";

describe("buildCdnUrl", () => {
    it("builds a basic CDN URL", () => {
        const url = buildCdnUrl("https://esm.sh", "react", "18.2.0", "", "es2022");
        expect(url).toBe("https://esm.sh/react@18.2.0?target=es2022");
    });

    it("builds a URL with subpath", () => {
        const url = buildCdnUrl("https://esm.sh", "react", "18.2.0", "/jsx-runtime", "es2022");
        expect(url).toBe("https://esm.sh/react@18.2.0/jsx-runtime?target=es2022");
    });

    it("builds a URL for scoped packages", () => {
        const url = buildCdnUrl("https://esm.sh", "@emotion/styled", "11.0.0", "", "es2020");
        expect(url).toBe("https://esm.sh/@emotion/styled@11.0.0?target=es2020");
    });

    it("strips semver range prefix", () => {
        const url = buildCdnUrl("https://esm.sh", "react", "^18.2.0", "", "es2022");
        expect(url).toBe("https://esm.sh/react@18.2.0?target=es2022");
    });

    it("handles missing version", () => {
        const url = buildCdnUrl("https://esm.sh", "lodash", undefined, "", "es2022");
        expect(url).toBe("https://esm.sh/lodash?target=es2022");
    });
});

describe("createNpmResolverPlugin", () => {
    it("returns a valid esbuild plugin", () => {
        const plugin = createNpmResolverPlugin();
        expect(plugin.name).toBe("npm-resolver");
        expect(plugin.setup).toBeInstanceOf(Function);
    });

    it("accepts custom CDN URL", () => {
        const plugin = createNpmResolverPlugin({
            cdnUrl: "https://cdn.skypack.dev",
        });
        expect(plugin.name).toBe("npm-resolver");
    });

    it("accepts dependencies map", () => {
        const plugin = createNpmResolverPlugin({
            dependencies: {
                react: "^18.2.0",
                "react-dom": "^18.2.0",
            },
        });
        expect(plugin.name).toBe("npm-resolver");
    });

    it("accepts bundle mode", () => {
        const plugin = createNpmResolverPlugin({
            mode: "bundle",
        });
        expect(plugin.name).toBe("npm-resolver");
    });
});
