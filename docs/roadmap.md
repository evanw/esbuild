# esbuild Roadmap

Prioritized roadmap based on the Feb 2026 tech debt audit and product ideas review. Items within each quarter are roughly ordered by impact-to-effort ratio.

See [tech-debt.md](./tech-debt.md) for detailed debt descriptions and [product-ideas.md](./product-ideas.md) for expanded feature rationale.

---

## Q1 2026 — Foundation (Already Unblocked)

These items are independently actionable, require no architectural changes, and have clear acceptance criteria.

- [ ] **Bump Node.js minimum: ≥18 → ≥20**
  Node.js 18 LTS reaches end-of-life in April 2026. Dropping it removes the need to maintain compatibility shims for APIs added in Node 20 and simplifies the CI matrix.

- [ ] **Bump Go minimum: 1.13 → 1.18**
  Go 1.13 is over four years old. Moving to 1.18 unlocks generics (useful for the feature flags refactor in P2-18), `any` type alias, `errors.Join`, and modern stdlib improvements. The current floor is the most significant self-imposed constraint in the codebase.

- [ ] **Add fuzzing to JS/CSS/JSON parsers**
  Use Go native `testing/fuzz`. Seed corpus from existing test inputs. Run with `-fuzztime=30s` in CI to catch panics; run extended sessions out-of-band. See [tech-debt.md #6](./tech-debt.md) and [product-ideas.md #19](./product-ideas.md).

- [ ] **Parallelize bundler test suite**
  Add `t.Parallel()` to bundler tests. Expected 4–6× speedup on typical developer hardware. The bottleneck is that 1,061 tests currently run sequentially. See [tech-debt.md #6](./tech-debt.md).

- [ ] **Add parse/bundle/print benchmarks**
  Add `testing.B` benchmarks for representative inputs and run them in CI. esbuild's performance is its core differentiator; automated regression detection is essential. See [product-ideas.md #18](./product-ideas.md).

- [ ] **MCP server: add tests and fix error handling**
  The MCP server has 0 tests and uses generic `catch` blocks throughout. Add test coverage for the existing tools, replace untyped catches with typed errors, and fix the known schema errors (`define` type, `loader` enum, `format` missing `preserve`). See [tech-debt.md #11](./tech-debt.md).

---

## Q2 2026 — Distribution & Platform

- [ ] **Survey community on low-value platforms**
  Identify candidates for deprecation (mips64el, s390x, ppc64 are the most likely) via a community survey. Deprecating even 3–4 platforms reduces the per-release publish burden meaningfully.

- [ ] **Fix Deno ARM64 WASM crash**
  The Deno integration is pinned to v1.24.0 (released mid-2022) due to an ARM64 crash. Unblocking this is a prerequisite for JSR distribution and re-engaging the Deno ecosystem. See [product-ideas.md #3](./product-ideas.md).

- [ ] **CDN distribution prototype**
  Prototype replacing `optionalDependencies` with a CDN binary fetch at install time, with `esbuild-wasm` as a fallback. This is architectural groundwork for eventually retiring the 26-package structure. See [tech-debt.md #1](./tech-debt.md) and [product-ideas.md #1](./product-ideas.md).

- [ ] **Auto-update compat tables via GitHub Actions cron**
  Add a weekly cron job that runs `make update-compat-table` and opens a PR when upstream tables (kangax, caniuse-lite, MDN) have changed. Eliminates the manual regeneration step. See [tech-debt.md #7](./tech-debt.md) and [product-ideas.md #15](./product-ideas.md).

---

## Q2–Q3 2026 — Parser & Optimizer

- [ ] **Split `visitExprInOut` into focused functions**
  Extract constant folding, syntax lowering, TypeScript substitution, and JSX transformation into separate functions called from a thin coordinator. The goal is to make each pass independently testable and to reduce `js_parser.go` below 10,000 lines. See [tech-debt.md #3](./tech-debt.md).

- [ ] **Document TypeScript backtracking hack with assertions**
  Enumerate all parser state that is cloned in the backtracking hack at `ts_parser.go:940-978`. Add a comment listing each field and a test that would fail if a new field is added without updating the clone. See [tech-debt.md #4](./tech-debt.md).

- [ ] **`@supports` branch elimination**
  When a browser target is known, evaluate `@supports` conditions against the compat table and remove impossible branches at build time. See [product-ideas.md #11](./product-ideas.md).

- [ ] **`calc()` minification**
  Implement constant folding in `css_reduce_calc.go` to simplify arithmetic results: `calc(1px + 0px)` → `1px`. Low effort, visible impact for CSS-heavy builds. See [tech-debt.md #19](./tech-debt.md) and [product-ideas.md #14](./product-ideas.md).

- [ ] **Container query name localization**
  Complete the partially-started implementation noted in `css_parser.go:1755`. Scope container names in CSS modules the same way class names are scoped. See [product-ideas.md #13](./product-ideas.md).

- [ ] **Expand CSS compat table coverage**
  Add entries for Container Queries, `@layer`, `@scope`, and `@view-transition`. The current CSS compat table covers 13 features; these additions would bring it to ~20 and cover features that are now widely supported.

---

## Q3–Q4 2026 — Ecosystem

- [ ] **Plugin registry MVP**
  Launch a minimal plugin discovery site that indexes npm packages tagged with `esbuild-plugin`. Display compatibility metadata, install counts, and links to source. See [product-ideas.md #2](./product-ideas.md).

- [ ] **MCP server v2**
  Add `serve()`, `watch()`, and `context()` API coverage. Add plugin support. Complete all option schemas. This makes the MCP server production-ready and unblocks the VS Code extension. See [tech-debt.md #11](./tech-debt.md) and [product-ideas.md #6](./product-ideas.md).

- [ ] **Manual chunk labels (unblock cyclic chunk imports)**
  Complete the deferred work at `linker.go:550-601`. Manual chunk labels are a frequently requested feature that is blocked by the unfinished lazy module initialization path. See [tech-debt.md #15](./tech-debt.md).

- [ ] **VS Code extension powered by MCP**
  After MCP server v2 is complete, build a VS Code extension that uses the MCP server for inline diagnostics, config validation, and bundle size analysis. See [product-ideas.md #6](./product-ideas.md).

---

## Long-Term (2027+)

These items require significant architectural work or depend on external ecosystem changes.

- [ ] **Modular runtime helpers**
  Break `runtime.go` into individually tree-shakable helper modules with explicit dependency declarations. Requires changing how the runtime is compiled into builds. See [tech-debt.md #17](./tech-debt.md) and [product-ideas.md #16](./product-ideas.md).

- [ ] **CSS parser modularization**
  Extract at-rule handlers from the 2,486-line `css_parser.go` monolith into focused files. Target: no single CSS file exceeds 1,000 lines.

- [ ] **CDN distribution GA — retire 26 npm packages**
  After the CDN prototype proves reliable, migrate all users to the single-package + CDN model and deprecate the `@esbuild/<platform>` packages. Requires a multi-cycle migration window with backward compatibility. See [tech-debt.md #1](./tech-debt.md) and [product-ideas.md #1](./product-ideas.md).

- [ ] **Remove custom Go compiler hack**
  The custom Go compiler patch exists to avoid false-positive CVE reports from security scanners. This can be removed once the security scanner ecosystem improves or an upstream Go mechanism for stripping build info is available. See [tech-debt.md #2](./tech-debt.md).

- [ ] **WASM performance optimization track**
  Systematically close the gap between the native binary and the WASM build. The WASM build is currently ~10× slower for large inputs, limiting its usefulness in browser-based tooling. This requires profiling, WASM-specific code paths, and possibly a streaming compilation API.

---

## Dependency Map

```
Node ≥20 bump
    └── (unblocks) modern API usage in lib/npm/node.ts

Go ≥1.18 bump
    └── (unblocks) generics for feature flag refactor (P2-18)
    └── (unblocks) errors.Join for MCP error handling

Fix Deno ARM64 WASM crash
    └── (unblocks) JSR distribution

MCP server v2
    └── (unblocks) VS Code extension
    └── (unblocks) Config Coach

Manual chunk labels
    └── (requires) lazy module initialization completion
    └── (unblocks) cyclic chunk import fix

CDN distribution prototype
    └── (leads to) CDN distribution GA
    └── (leads to) retire 26 npm packages
```
