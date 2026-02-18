# esbuild-mcp

An [MCP (Model Context Protocol)](https://modelcontextprotocol.io/) server that exposes esbuild's core capabilities as MCP tools, allowing AI assistants to transform TypeScript/JSX, bundle files, run incremental builds, start a dev server with live reload, watch for file changes, and analyze build output.

## Tools

### `esbuild_transform`
Transform source code (TypeScript, JSX, CSS, etc.) to JavaScript or CSS.

**Inputs:** `code` (required), `loader`, `sourcefile`, `tsconfigRaw`, `banner`, `footer`, plus common options below.

### `esbuild_build`
Bundle entry point files. Returns output contents in memory (does not write to disk by default).

**Inputs:** `entryPoints` (required), plus all build and common options below.

### `esbuild_context`
Create an esbuild context for incremental builds. Performs a `rebuild()` and returns the result, then disposes the context.

**Inputs:** Same as `esbuild_build` — `entryPoints` (required), plus all build and common options.

### `esbuild_serve`
Start an esbuild development server with live reload. Returns the host and port.

**Inputs:** All `esbuild_build` inputs, plus serve-specific options: `port`, `host`, `servedir`, `keyfile`, `certfile`, `fallback`.

### `esbuild_watch`
Start esbuild in watch mode for automatic rebuilds on file changes. Returns the initial build result.

**Inputs:** Same as `esbuild_build` — `entryPoints` (required), plus all build and common options.

### `esbuild_analyze_metafile`
Parse and display a bundle analysis metafile as human-readable text.

**Inputs:** `metafile` (JSON string from `esbuild_build`), `verbose`

### `esbuild_format_messages`
Format esbuild error or warning messages for display.

**Inputs:** `messages` (array of esbuild Message objects), `kind` (`error` | `warning`)

### Common options (shared by all build/transform tools)
`format`, `target`, `platform`, `minify`, `minifyWhitespace`, `minifyIdentifiers`, `minifySyntax`, `define`, `pure`, `keepNames`, `drop`, `dropLabels`, `charset`, `lineLimit`, `treeShaking`, `ignoreAnnotations`, `jsx`, `jsxFactory`, `jsxFragment`, `jsxImportSource`, `jsxDev`, `jsxSideEffects`, `sourcemap`, `sourceRoot`, `sourcesContent`, `legalComments`, `globalName`, `supported`, `mangleProps`, `reserveProps`, `mangleQuoted`, `mangleCache`, `logLevel`, `logLimit`, `logOverride`, `color`

### Build-only options (shared by build, context, watch, serve)
`entryPoints`, `bundle`, `splitting`, `metafile`, `outdir`, `outfile`, `outbase`, `outExtensions`, `publicPath`, `entryNames`, `chunkNames`, `assetNames`, `external`, `packages`, `alias`, `resolveExtensions`, `mainFields`, `conditions`, `preserveSymlinks`, `nodePaths`, `tsconfig`, `loader`, `inject`, `banner`, `footer`, `stdin`, `write`, `allowOverwrite`, `absWorkingDir`

## Setup

```bash
cd mcp
npm install
npm run build
```

## Usage

### Native esbuild (default)

```json
{
  "mcpServers": {
    "esbuild": {
      "command": "node",
      "args": ["/path/to/esbuild/mcp/dist/index.js"]
    }
  }
}
```

### esbuild-wasm (no native binary required)

```json
{
  "mcpServers": {
    "esbuild-wasm": {
      "command": "node",
      "args": ["/path/to/esbuild/mcp/dist/index.js"],
      "env": { "ESBUILD_WASM": "1" }
    }
  }
}
```

When `ESBUILD_WASM=1` is set, the server uses `esbuild-wasm` instead of the native binary. This is useful in restricted environments (e.g., Docker containers without native binaries).

## Development

```bash
npm run build       # Compile TypeScript
npm start           # Start with native esbuild
npm run start:wasm  # Start with esbuild-wasm
```
