# esbuild Tech Debt

Findings from a full codebase audit (Feb 2026). Items are grouped by severity with direct file/line references for navigation.

---

## Critical (P0)

### 1. 26 npm Platform Packages

**Files:** `Makefile:352-490`, `npm/@esbuild/*/package.json`

Each release requires sequentially publishing 26 platform-specific packages plus the main package, with version synchronization across all of them. The `validate-builds` target cross-checks all 26 against the npm registry per release, adding ~40 minutes to each publish cycle. Every `npm install esbuild` fetches metadata for all 26 optional packages regardless of platform.

This architecture was a pragmatic choice when npm optional dependencies were the only viable mechanism, but the operational overhead now significantly slows the release cadence.

### 2. Custom Go Compiler Patch

**Files:** `Makefile:58-82`, `go.version`

The release process downloads and patches Go source code to strip `ctxt.buildinfo()` in order to avoid false-positive CVE reports from security scanners. The Makefile comment reads: *"avoid poorly-implemented 'security' tools from breaking esbuild"*. This adds ~10 minutes to every release and requires manually tracking Go security patches to stay current. There is no upstream path to fix this — it depends on the security scanner ecosystem improving.

### 3. `visitExprInOut`: 2,478-line Monolithic Function

**File:** `internal/js_parser/js_parser.go:13485`

This single function mixes constant folding, syntax lowering, TypeScript substitution, JSX transformation, and import lowering. It is the primary reason `js_parser.go` is 657KB / ~18,600 lines — the largest file in the codebase. The function's size makes it difficult to reason about, test in isolation, or extend without risk of unintended interactions between the passes it conflates.

### 4. TypeScript Backtracking "GROSS HACK"

**File:** `internal/js_parser/ts_parser.go:940-978`

Labeled "GROSS HACK" in a code comment, this implements a full parser state clone to handle the ambiguous grammar case `y = a ? (b) : c => d : e;`. The comment explicitly warns that any parser state added elsewhere that is not also cloned here will silently break TypeScript parsing in this edge case. This is a latent correctness risk with no automated guard.

### 5. CJS/ESM Interop Complexity

**Files:** `internal/linker/linker.go:1900-1980`, GitHub issue #1591

The interop between CommonJS and ESM modules is explicitly described in code comments as "extremely complex and subtle." It requires `__toESM`, `__toCommonJS`, and `__toCJS` runtime wrappers with a 4-step export resolution process. This complexity is load-bearing and very difficult to simplify without breaking existing behavior, but it is a significant source of bugs and makes the linker hard to modify.

### 6. ~~Zero Benchmarks and~~ Zero Fuzzing

**Files:** `internal/bundler_tests/`, `internal/js_parser/`, `internal/css_parser/`

~~The project has 0 `testing.B` benchmark functions.~~ **PARTIALLY RESOLVED**: 6 benchmarks now exist — 3 JS parser (`BenchmarkParseJS`, `BenchmarkParseTypeScript`, `BenchmarkParseJSX`) + 3 CSS parser (`BenchmarkParseCSS`, `BenchmarkParseCSSMinify`, `BenchmarkPrintCSS`). 182 parser tests now run with `t.Parallel()` (130 js_parser + 52 css_parser).

**Remaining**: Still zero fuzz tests — the parsers accept arbitrary user input with no systematic edge-case discovery. Bundler tests still run sequentially (snapshot mutex prevents parallelism).

---

## High (P1)

### 7. Compat Table Manual Regeneration

**Files:** `internal/compat/js_table.go` (936 lines), `internal/compat/css_table.go` (422 lines), `compat-table/src/`

**PARTIALLY RESOLVED**: `make check-compat-table` target now exists for CI staleness detection.

**Remaining**: The manual regeneration process itself is unchanged. The compat tables still merge three external data sources (kangax, caniuse-lite, MDN) through a manual process that requires resolving conflicts between them. The generated Go files are committed to the repo and must be manually regenerated when upstream tables update. The Firefox 120 gradient edge case required a hardcoded override (commit `ac54f06d`) because the automated process produced incorrect output — the merge logic fragility still exists.

### 8. TDZ Performance Workarounds

**File:** `internal/js_parser/js_parser_lower_class.go:2050-2072`

Two engine bugs require active workarounds that add complexity to class lowering:

- **JavaScriptCore** (webkit bug #199866): quadratic time in variable count with 1,000% slowdown in pathological cases
- **V8** (chromium bug #13723): 10% slowdown from TDZ checks

The current workaround hoists top-level exported symbols outside the closure. This is fragile: if the surrounding wrapping structure changes, the workaround may silently stop working. Both upstream bugs are years old with no resolution timeline.

### 9. Snapshot Test Brittleness

**Files:** `internal/bundler_tests/snapshots/` (14 files, 628KB, 26,743 lines)

Golden-output snapshot tests catch regressions but are painful to maintain. Any code change that affects output — including whitespace, comment formatting, or symbol naming — requires re-running with `UPDATE_SNAPSHOTS=1` and manually inspecting diffs across potentially hundreds of test cases. `snapshots_default.txt` alone is 158KB covering 302 test cases. There is no mechanism to approve individual test changes without regenerating the whole set.

### 10. Option Validation Duplication

**Files:** `lib/shared/common.ts` (76KB), `pkg/cli/cli_impl.go`, `pkg/api/api.go`

There are 150+ option validation functions in `common.ts` that are duplicated in the CLI Go implementation. Go enums in `pkg/api/api.go` and TypeScript types in `lib/shared/types.ts` are maintained separately with no code generation or schema linking between them. Schema drift is a recurring source of subtle bugs.

### 11. MCP Server Gaps

**Files:** `mcp/src/`

**PARTIALLY RESOLVED**: 14 vitest tests added across 4 test files (`transform`, `build`, `analyze`, `format-messages`). `context()` API now exposed via `esbuild_context` tool. Loader enum fixed (8 → 15 values), build schema expanded with `outdir`, `outfile`, `loader`, `treeShaking`, `jsx`. Coverage improved from ~30% to ~50%.

**Remaining**: `serve()`, `watch()`, and plugin support still missing. Generic `catch` blocks still used throughout (note: `esbuild.analyzeMetafile()` returns empty string on invalid JSON rather than throwing — the generic catch may mask this). Typed errors not yet implemented.

---

## Medium (P2)

### 12. `linker.go`: 7,279-line File

**File:** `internal/linker/linker.go`

The largest file in the audit scope. It implements 4-phase export resolution, parallel goroutine coordination, and CSS stub population. Its size makes it difficult to navigate and increases the risk of unintended interactions between the phases.

### 13. Serve API Listener Hack

**File:** `pkg/api/serve_other.go:931-936`

The `hackListener` struct includes a 50ms sleep to work around a Linux TCP RST issue. This is a time-based race workaround — not a principled fix — and may fail under load or on slow systems.

### 14. CSS Nesting Expansion Limit — RESOLVED

**File:** `internal/css_parser/css_nesting.go:277-282`

~~An arbitrary cap of `0xFF00` on CSS nesting expansion with no comment explaining the basis for this specific value.~~ The cap now has a detailed comment explaining its rationale.

### 15. Cyclic Chunk Import Deferral

**File:** `internal/linker/linker.go:550-601`

A code comment explicitly states: "work hasn't been finished yet." This blocks support for manual chunk labels, which is a feature users have requested. The partial implementation means the code path exists but is not reachable in practice.

### 16. Property Mangling Ignores Dead Code

**File:** `internal/linker/linker.go:406,479`

The code comments explicitly note that property mangling "does not currently account for live vs. dead code." This means mangled property names may be assigned to code that tree-shaking would otherwise eliminate, producing incorrect output in some configurations.

### 17. `runtime.go` Monolith

**File:** `internal/runtime/runtime.go` (604 lines)

Over 40 runtime helpers are defined in a single string template with conditional ES5 paths. Because helpers are embedded as a string rather than as separate Go files, they cannot be tree-shaken independently. Every build pays the cost of including the full runtime template even when only a few helpers are needed.

### 18. JS Feature Flags: uint64 Limit — RESOLVED

**File:** `internal/compat/js_table.go:125-133`

~~52 JS feature flags are packed into a `uint64`.~~ **Correction**: There are actually 61 features (iota 0-60), leaving only 3 bits of capacity. Now has a warning comment in the const block (`js_table.go:125-127`) and a compile-time overflow assertion (`const _ = uint64(Using) * 2` at `js_table.go:130-133`). Note: `Using` is already a bitmask value (`1 << 60`), not a raw iota, so `uint64(Using) * 2` correctly overflows when iota reaches 63.

### 19. `calc()` Produces Limited Simplification

**File:** `internal/css_parser/css_reduce_calc.go`

**PARTIALLY RESOLVED**: Multi-term product simplification now works — `calc(2 * 3px * 4)` → `24px`. Previous code only handled exactly 2 terms; now handles any N terms with ≤1 unit.

**Remaining**: Additive simplification is still incomplete. The sum simplifier (`calcSum.partiallySimplify`) correctly combines same-unit terms but doesn't eliminate zero-valued terms — `calc(1px + 0px)` stays as `1px + 0px` instead of simplifying to `1px`.

### 20. `bundler_default_test.go`: 9,389-line File

**File:** `internal/bundler_tests/bundler_default_test.go`

302 tests in a single file with no grouping by feature area and no parallelism. This makes it difficult to find related tests, run subsets, and adds to the serial test suite runtime.

---

## New Findings (Feb 2026 fix run)

These were discovered during the 8-agent parallel fix run that addressed items above.

1. **JSFeature compile-time assertion quirk**: The overflow guard for #18 couldn't use `1 << Using` as originally planned because `Using` is already a bitmask value (`1 << 60`), not a raw iota. `uint64(Using) * 2` was used instead, which correctly overflows when iota reaches 63.

2. **`analyzeMetafile()` silent failure**: `esbuild.analyzeMetafile()` returns an empty string on invalid JSON input rather than throwing. The MCP analyze tool's generic catch block may mask this behavior — returning error text when it should return empty.

3. **`calc()` zero-term elimination gap**: The additive simplifier (`calcSum.partiallySimplify`) correctly combines same-unit terms but doesn't eliminate zero-valued terms. For example, `calc(1px + 0px)` remains `1px + 0px` instead of simplifying to `1px`.
