# esbuild Tech Debt

Findings from a full codebase audit (Feb 2026). Items are grouped by severity with direct file/line references for navigation.

---

## Critical (P0)

### 1. 27 npm Platform Packages

**Files:** `Makefile:594-623`, `npm/@esbuild/*/package.json`

Each release requires sequentially publishing 27 platform-specific packages plus the main package, with version synchronization across all of them. The `validate-builds` target cross-checks all 27 against the npm registry per release, adding ~40 minutes to each publish cycle. Every `npm install esbuild` fetches metadata for all 27 optional packages regardless of platform.

This architecture was a pragmatic choice when npm optional dependencies were the only viable mechanism, but the operational overhead now significantly slows the release cadence.

### 2. Custom Go Compiler Patch

**Files:** `Makefile:58-82`, `go.version`

The release process downloads and patches Go source code to strip `ctxt.buildinfo()` in order to avoid false-positive CVE reports from security scanners. The Makefile comment reads: *"avoid poorly-implemented 'security' tools from breaking esbuild"*. This adds ~10 minutes to every release and requires manually tracking Go security patches to stay current. There is no upstream path to fix this — it depends on the security scanner ecosystem improving.

### 3. `visitExprInOut`: 3,650-line Monolithic Function

**File:** `internal/js_parser/js_parser.go:13485-17134`

This single function mixes constant folding, syntax lowering, TypeScript substitution, JSX transformation, and import lowering. It is the primary reason `js_parser.go` is 641KB / 18,788 lines — the largest file in the codebase. The function's size makes it difficult to reason about, test in isolation, or extend without risk of unintended interactions between the passes it conflates.

### 4. TypeScript Backtracking "GROSS HACK"

**File:** `internal/js_parser/ts_parser.go:940-978`

Labeled "GROSS HACK" in a code comment, this implements a full parser state clone to handle the ambiguous grammar case `y = a ? (b) : c => d : e;`. The comment explicitly warns that any parser state added elsewhere that is not also cloned here will silently break TypeScript parsing in this edge case. This is a latent correctness risk with no automated guard.

### 5. CJS/ESM Interop Complexity

**Files:** `internal/linker/linker.go:1900-1980`, GitHub issue #1591

The interop between CommonJS and ESM modules is explicitly described in code comments as "extremely complex and subtle." It requires `__toESM` and `__toCommonJS` runtime wrappers with a 4-step export resolution process. This complexity is load-bearing and very difficult to simplify without breaking existing behavior, but it is a significant source of bugs and makes the linker hard to modify.

### 6. Bundler Test Parallelism

**Files:** `internal/bundler_tests/`

Benchmarks (6), fuzz tests (`FuzzParseJS`, `FuzzParseCSS`), and `t.Parallel()` (182 tests) now exist for the parsers. However, bundler tests still run sequentially due to the snapshot mutex preventing parallelism.

---

## High (P1)

### 7. Compat Table Manual Regeneration

**Files:** `internal/compat/js_table.go` (945 lines), `internal/compat/css_table.go` (422 lines), `compat-table/src/`

`make check-compat-table` target now exists for CI staleness detection, but the manual regeneration process itself is unchanged. The compat tables still merge three external data sources (kangax, caniuse-lite, MDN) through a manual process that requires resolving conflicts between them. The generated Go files are committed to the repo and must be manually regenerated when upstream tables update. The Firefox 120 gradient edge case required a hardcoded override (commit `ac54f06d`) because the automated process produced incorrect output — the merge logic fragility still exists.

### 8. TDZ Performance Workarounds

**File:** `internal/js_parser/js_parser_lower_class.go:2058-2071`

Two engine bugs require active workarounds that add complexity to class lowering:

- **JavaScriptCore** (webkit bug #199866): quadratic time in variable count with 1,000% slowdown in pathological cases
- **V8** (chromium bug #13723): 10% slowdown from TDZ checks

The current workaround hoists top-level exported symbols outside the closure. This is fragile: if the surrounding wrapping structure changes, the workaround may silently stop working. Both upstream bugs are years old with no resolution timeline.

### 9. Snapshot Test Brittleness

**Files:** `internal/bundler_tests/snapshots/` (14 files, 628KB, 26,743 lines)

Golden-output snapshot tests catch regressions but are painful to maintain. Any code change that affects output — including whitespace, comment formatting, or symbol naming — requires re-running with `UPDATE_SNAPSHOTS=1` and manually inspecting diffs across potentially hundreds of test cases. `snapshots_default.txt` alone is 158KB covering 300 test cases. There is no mechanism to approve individual test changes without regenerating the whole set.

### 10. Option Validation Duplication

**Files:** `lib/shared/common.ts` (76KB), `pkg/cli/cli_impl.go`, `pkg/api/api.go`

There are 150+ option validation functions in `common.ts` that are duplicated in the CLI Go implementation. Go enums in `pkg/api/api.go` and TypeScript types in `lib/shared/types.ts` are maintained separately with no code generation or schema linking between them. Schema drift is a recurring source of subtle bugs.

### 11. MCP Server Gaps

**Files:** `mcp/src/`

18 vitest tests across 6 test files. 7 MCP tools registered: `esbuild_transform`, `esbuild_build`, `esbuild_analyze_metafile`, `esbuild_format_messages`, `esbuild_context`, `esbuild_serve`, `esbuild_watch`. Typed error handling via shared `formatErrorResponse()` helper in `errors.ts`. Coverage ~65%.

**Remaining**: Plugin support still missing. `esbuild.analyzeMetafile()` returns empty string on invalid JSON rather than throwing — this edge case is not handled.

---

## Medium (P2)

### 12. `linker.go`: 7,279-line File

**File:** `internal/linker/linker.go`

The largest file in the audit scope. It implements 4-phase export resolution, parallel goroutine coordination, and CSS stub population. Its size makes it difficult to navigate and increases the risk of unintended interactions between the phases.

### 13. Serve API Listener Hack

**File:** `pkg/api/serve_other.go:931-936`

The `hackListener` struct includes a 50ms sleep to work around a Linux TCP RST issue. This is a time-based race workaround — not a principled fix — and may fail under load or on slow systems.

### 14. Cyclic Chunk Import Deferral

**File:** `internal/linker/linker.go:550-601`

A code comment explicitly states: "work hasn't been finished yet." This blocks support for manual chunk labels, which is a feature users have requested. The partial implementation means the code path exists but is not reachable in practice.

### 15. Property Mangling Ignores Dead Code

**File:** `internal/linker/linker.go:406,479`

The code comments explicitly note that property mangling "does not currently account for live vs. dead code." This means mangled property names may be assigned to code that tree-shaking would otherwise eliminate, producing incorrect output in some configurations.

### 16. `runtime.go` Monolith

**File:** `internal/runtime/runtime.go` (604 lines)

Over 40 runtime helpers are defined in a single string template with conditional ES5 paths. Because helpers are embedded as a string rather than as separate Go files, they cannot be tree-shaken independently. Every build pays the cost of including the full runtime template even when only a few helpers are needed.

### 17. `bundler_default_test.go`: 9,389-line File

**File:** `internal/bundler_tests/bundler_default_test.go`

300 tests in a single file with no grouping by feature area and no parallelism. This makes it difficult to find related tests, run subsets, and adds to the serial test suite runtime.

---

## New Findings (Feb 2026 fix run)

1. **JSFeature compile-time assertion quirk**: The overflow guard for the `uint64` feature flags couldn't use `1 << Using` as originally planned because `Using` is already a bitmask value (`1 << 60`), not a raw iota. `uint64(Using) * 2` was used instead, which correctly overflows when iota reaches 63. See `internal/compat/js_table.go:125-133`.

2. **`analyzeMetafile()` silent failure**: `esbuild.analyzeMetafile()` returns an empty string on invalid JSON input rather than throwing. The MCP analyze tool now uses `formatErrorResponse()` but this silent-return edge case is still not explicitly handled.
