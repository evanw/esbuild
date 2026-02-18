# esbuild Product Ideas

Future features and product opportunities identified during the Feb 2026 codebase audit. Ideas are grouped by theme and include rationale tied to existing code structure.

---

## Distribution & Ecosystem

### 1. CDN-Based Distribution

Replace the 26 `npm/@esbuild/<platform>` packages with a single package that downloads the platform binary from a CDN at install time, with a WASM fallback for unsupported environments. This would eliminate the per-release publish of 26 packages, reduce npm metadata overhead for users, and make it possible to ship new platforms without a package registry publish. Existing `esbuild-wasm` already proves the WASM fallback path works.

### 2. Plugin Marketplace

The esbuild plugin API is mature, well-documented, and widely used — but there is no centralized discovery mechanism. A plugin registry (e.g., `registry.esbuild.dev`) with search, compatibility tags (esbuild version, platform), and install counts would surface the existing ecosystem and reduce the friction of finding plugins for common use cases (MDX, SASS, SVG, etc.).

### 3. JSR / Deno-First Distribution

Publish `@esbuild/core` to JSR with a native WASM fallback so Deno users can `deno add jsr:@esbuild/core` without relying on npm compatibility shims. The current Deno integration is pinned to v1.24.0 (2+ years old) due to an ARM64 WASM crash. Fixing the crash and publishing to JSR would formally support the Deno ecosystem.

### 4. Bun Compatibility Build

Bun uses esbuild internally. An official Bun-optimized build — potentially with Bun-specific API bindings — would reduce the gap between esbuild and Bun's internal bundler, give Bun users a supported upgrade path, and create a marketing opportunity as Bun's ecosystem grows.

### 5. Embedded Binaries for Top 4 Platforms

Bundle the binaries for `linux-x64`, `darwin-x64`, `darwin-arm64`, and `win32-x64` directly into a single npm package. This would support offline-first installs (air-gapped CI, Docker builds without npm access) for the platforms that cover the vast majority of users, without requiring the full 26-package architecture.

---

## Developer Tooling

### 6. VS Code Extension via MCP

Use the existing MCP server as the backend for a VS Code extension that provides inline diagnostics, config validation, and live bundle analysis. When a config file changes, the extension could show estimated output size, warn about missing externals, and surface tree-shaking opportunities — all powered by the MCP `build` tool without spawning a new process.

### 7. esbuild Config Coach

An AI-powered wizard (enabled by the MCP server) that accepts a project description and outputs an optimized esbuild configuration with explanation. Input: "React app, TypeScript, targeting Chrome 100+, with code splitting." Output: recommended `build()` options with comments explaining each choice. The MCP server already exposes the API surface needed; the coach would add a reasoning layer on top.

### 8. Webpack / Rollup Migration Tooling

A conversion tool that takes a `webpack.config.js` or `rollup.config.js` and produces an equivalent esbuild configuration, with an interactive guide for plugins that have no direct equivalent. Most webpack options map to esbuild options in documented ways; a tool that handles the common cases would lower the migration barrier significantly.

### 9. Build Analyzer Dashboard

A web-based tool where users upload an esbuild metafile and receive: dead code recommendations, code-splitting opportunities, unused export detection, and module size visualization. Free tier for one-off analysis; CI tier for tracking bundle size regressions across commits. The metafile format is already documented and stable; the analysis logic is the missing piece.

### 10. Bundler Playground

A browser-based playground powered by `esbuild-wasm` where users can write input code, choose options, and see the bundled output in real time. Side-by-side minification comparison (esbuild vs. unminified) and shareable permalink URLs would make it useful for debugging and documentation. `esbuild-wasm` already works in the browser; a thin UI is the only addition needed.

---

## Parser & Optimizer Improvements

### 11. `@supports` Branch Elimination

When a specific browser target is known, `@supports` blocks for features not supported by that target could be unconditionally included, and blocks for features always supported could be unconditionally removed. The CSS parser already parses `@supports` conditions; what is missing is an evaluator that maps conditions to compat table entries.

### 12. CSS `@layer` Reordering

The CSS parser handles `@layer` declarations correctly, but cascade-order optimization and deduplication of redundant layer declarations are not implemented. With `@layer`, earlier declarations have lower priority — reordering layers for size or deduplicating empty layers could produce smaller output.

### 13. Container Query Name Localization

A code comment in `internal/css_parser/css_parser.go:1755` notes that container query name localization is partially started. Completing this would allow esbuild to scope container names in CSS modules the same way it scopes class names, preventing naming collisions in large projects.

### 14. `calc()` Minification

`internal/css_parser/css_reduce_calc.go` reduces `calc()` structure but never simplifies the arithmetic result. Implementing constant folding for `calc()` expressions — `calc(1px + 0px)` → `1px`, `calc(100% - 0px)` → `100%` — would be a low-effort, high-value minification improvement for CSS-heavy projects.

### 15. Auto-Updating Compat Tables

A GitHub Actions cron job that runs `make update-compat-table` weekly and opens a PR when the upstream kangax, caniuse-lite, or MDN tables have changed. This would replace the current manual process and ensure compat data stays current without requiring maintainer intervention for routine updates.

### 16. Modular Runtime Helpers

Break `internal/runtime/runtime.go` into individually tree-shakable helper modules with explicit dependency declarations. Currently all 40+ helpers are compiled into every build. Modular helpers would allow builds that don't use class syntax, async/await, or spread operators to skip the corresponding helpers entirely.

---

## Testing & Quality

### 17. Snapshot Testing 2.0

Replace the current 628KB golden-file approach with per-test snapshot files or a structured JSON format that supports semantic diffs. The goal: approve individual test changes without regenerating the entire snapshot corpus. A good prior art is Jest's `--updateSnapshot` with per-test tracking and interactive approval in the terminal.

### 18. Benchmark Suite

Add `testing.B` benchmarks for parse time, bundle time, and print time for representative inputs (large React app, large CSS file, TypeScript monorepo). Track benchmark results in CI to catch performance regressions before release. esbuild's performance is its core value proposition; there is currently no automated guard on it.

### 19. Fuzzing Infrastructure

Add Go native `testing/fuzz` targets for the JS parser, CSS parser, and JSON parser. Seed the corpus from existing test inputs. Run in CI with a short budget (`-fuzztime=30s`) to catch obvious panics, and separately in a dedicated fuzzing environment for deeper coverage. The parsers accept arbitrary user input; systematic edge-case discovery is overdue.

### 20. Regression Test Generator

A tool that automatically creates a minimal test case from a closed GitHub issue, tagged with the issue number. When a user reports "input X produces wrong output Y," the tool formats a bundler test case with the issue number as a comment and adds it to the test suite. This would build up a regression corpus tied directly to the issue tracker.
