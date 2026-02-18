# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

esbuild is an extremely fast JavaScript/CSS bundler and minifier written in Go. It exposes APIs for Go, Node.js, Deno, and the browser (via WebAssembly). The project prioritizes performance through parallelism, minimal AST passes, and avoiding unnecessary work.

## Build & Test Commands

```bash
# Build
make                    # Build esbuild binary (or: go build ./cmd/esbuild)

# Test
make test-go            # Go tests only (fast, primary development loop)
make test               # Go + JS API + plugin + e2e tests (uses -j6)
make test-all           # Everything including slow tests (deno, wasm, yarnpnp)

# Single Go test
go test -run TestName ./internal/bundler_tests   # Run one bundler test
go test -run TestName ./pkg/api                  # Run one API test
go test -run TestName ./internal/css_parser      # Run one CSS parser test

# Race detector (not on by default, macOS/Linux x64/arm64 only)
ESBUILD_RACE=-race make test-go

# Update bundler snapshot tests
UPDATE_SNAPSHOTS=1 make test-go

# Lint & format
make vet-go             # Go vet
make fmt-go             # Check Go formatting
make no-filepath        # Verify no "path/filepath" usage (forbidden, see golang.org/issue/43768)

# TypeScript lib type checking
make lib-typecheck
```

## Architecture

### Two-Phase Build Pipeline

1. **Scan phase** (`bundler.ScanBundle()`): Parallel worklist traversal of the dependency graph from entry points. Each file is parsed on a separate goroutine.

2. **Compile phase** (`(*Bundle).Compile()`): Links imports with exports, converts ASTs back to JavaScript/CSS, concatenates into final bundles.

### Three Full-AST Passes

1. Lexing + parsing + scope setup + symbol declaration
2. Symbol binding + constant folding + syntax lowering + syntax mangling
3. Printing + source map generation

### Key Packages

- `cmd/esbuild/` — CLI entry point and stdio service protocol
- `pkg/api/` — Public Go API (`Build`, `Transform`, `Serve`)
- `pkg/cli/` — CLI argument parsing
- `internal/bundler/` — Core bundling orchestration (scan + compile)
- `internal/js_parser/` — JS/TS/JSX parser (~656KB, largest file)
- `internal/js_lexer/` — JS tokenizer (on-demand, not ahead-of-time)
- `internal/js_printer/` — JS code generation
- `internal/js_ast/` — JS AST node definitions
- `internal/css_parser/`, `css_lexer/`, `css_printer/`, `css_ast/` — CSS pipeline (mirrors JS)
- `internal/linker/` — Module linking and scope hoisting
- `internal/resolver/` — Module resolution with syscall caching
- `internal/bundler_tests/` — Bundler snapshot tests
- `internal/renamer/` — Symbol minification
- `internal/runtime/` — Injected helper code
- `internal/compat/` — JS/CSS feature compatibility tables

### JS/TS API Layer

- `lib/npm/node.ts` — Node.js API entry (spawns Go binary, communicates via stdio)
- `lib/shared/stdio_protocol.ts` — Binary protocol between JS and Go
- `lib/shared/types.ts` — TypeScript type definitions (copied to `npm/esbuild/lib/main.d.ts`)

### npm Package Structure

- `npm/esbuild/` — Main package with postinstall that downloads platform binary
- `npm/@esbuild/<platform>/` — 29 platform-specific optional dependency packages
- `npm/esbuild-wasm/` — WebAssembly alternative

## Important Conventions

- **No `path/filepath`**: Use `path` package only. The `filepath` package is forbidden due to [golang.org/issue/43768](https://golang.org/issue/43768) (OS-specific path separators cause cross-platform bugs). Enforced by `make no-filepath`.
- **Go 1.13 compatibility**: Deliberately maintained. Do not use newer Go features or upgrade `golang.org/x/sys`.
- **Immutable data across builds**: Data structures shared between incremental/watch builds must be immutable. Go's type system doesn't enforce this.
- **Symbols are 64-bit IDs**: Identifiers reference symbols by index into a flat per-file array, not by name. This avoids name collision issues and enables efficient cloning.
- **Lexer is on-demand**: The parser calls the lexer as needed (not pre-tokenized) because JS has context-dependent tokenization (regex vs division, JSX vs less-than).
- **Snapshot tests**: Bundler test expected outputs live in `internal/bundler_tests/snapshots/*.txt`. Update with `UPDATE_SNAPSHOTS=1 make test-go`, then inspect the diff.
- **Custom Go compiler for releases**: The Makefile builds a modified Go compiler that strips buildinfo to avoid false-positive CVE reports from security scanners.

## Release Process

1. Bump version in `version.txt`
2. Copy version to `CHANGELOG.md` header
3. Run `make platform-all` to update all `package.json` files
4. Commit and push — triggers `publish.yml` GitHub Action (trusted publishing to npm)
