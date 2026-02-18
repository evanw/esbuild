# esbuild-mcp

An [MCP (Model Context Protocol)](https://modelcontextprotocol.io/) server that exposes esbuild's core capabilities as MCP tools, allowing AI assistants to transform TypeScript/JSX, bundle files, and analyze build output.

## Tools

### `esbuild_transform`
Transform source code (TypeScript, JSX, CSS, etc.) to JavaScript or CSS.

**Inputs:** `code`, `loader`, `target`, `format`, `minify`, `sourcemap`, `jsx`, `tsconfigRaw`

### `esbuild_build`
Bundle entry point files. Returns output contents in memory (does not write to disk).

**Inputs:** `entryPoints`, `bundle`, `format`, `target`, `platform`, `minify`, `splitting`, `external`, `define`, `metafile`, `sourcemap`

### `esbuild_analyze_metafile`
Parse and display a bundle analysis metafile as human-readable text.

**Inputs:** `metafile` (JSON string from `esbuild_build`), `verbose`

### `esbuild_format_messages`
Format esbuild error or warning messages for display.

**Inputs:** `messages` (array of esbuild Message objects), `kind` (`error` | `warning`)

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
