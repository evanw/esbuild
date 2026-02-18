import { z } from "zod";
const LoaderEnum = z.enum([
    "js", "jsx", "ts", "tsx", "css", "local-css", "json", "text",
    "base64", "binary", "dataurl", "copy", "default", "empty", "file",
]);
const LogLevelEnum = z.enum(["verbose", "debug", "info", "warning", "error", "silent"]);
const SourcemapEnum = z.union([
    z.boolean(),
    z.enum(["linked", "inline", "external", "both"]),
]);
const LegalCommentsEnum = z.enum(["none", "inline", "eof", "linked", "external"]);
const CharsetEnum = z.enum(["ascii", "utf8"]);
const FormatEnum = z.enum(["iife", "cjs", "esm"]);
const PlatformEnum = z.enum(["browser", "node", "neutral"]);
const JsxEnum = z.enum(["transform", "preserve", "automatic"]);
const DropEnum = z.enum(["console", "debugger"]);
const PackagesEnum = z.enum(["bundle", "external"]);
/** Options shared by both build and transform APIs */
export const CommonSchema = {
    format: FormatEnum.optional().describe("Output format"),
    target: z.union([z.string(), z.array(z.string())]).optional()
        .describe("Target environment(s) (e.g. es2020, esnext, chrome100)"),
    platform: PlatformEnum.optional().describe("Target platform"),
    minify: z.boolean().optional().describe("Minify whitespace, syntax, and identifiers"),
    minifyWhitespace: z.boolean().optional().describe("Minify whitespace only"),
    minifyIdentifiers: z.boolean().optional().describe("Minify identifiers only"),
    minifySyntax: z.boolean().optional().describe("Minify syntax only"),
    define: z.record(z.string(), z.string()).optional()
        .describe("Global identifier replacements (e.g. {\"DEBUG\": \"false\"})"),
    pure: z.array(z.string()).optional()
        .describe("Function calls to mark as pure for tree shaking (e.g. [\"console.log\"])"),
    keepNames: z.boolean().optional().describe("Preserve .name on functions and classes"),
    drop: z.array(DropEnum).optional().describe("Remove console/debugger statements"),
    dropLabels: z.array(z.string()).optional().describe("Remove labeled statements with these labels"),
    charset: CharsetEnum.optional().describe("Output character set (default: ascii)"),
    lineLimit: z.number().optional().describe("Soft line width limit for wrapping"),
    treeShaking: z.boolean().optional().describe("Enable tree shaking"),
    ignoreAnnotations: z.boolean().optional()
        .describe("Ignore /* @__PURE__ */ and sideEffects annotations"),
    jsx: JsxEnum.optional().describe("JSX handling mode"),
    jsxFactory: z.string().optional().describe("JSX factory function (e.g. React.createElement)"),
    jsxFragment: z.string().optional().describe("JSX fragment (e.g. React.Fragment)"),
    jsxImportSource: z.string().optional().describe("JSX import source for automatic runtime"),
    jsxDev: z.boolean().optional().describe("Use development JSX runtime"),
    jsxSideEffects: z.boolean().optional().describe("Do not treat JSX as side-effect free"),
    sourcemap: SourcemapEnum.optional().describe("Source map mode (true, 'linked', 'inline', 'external', 'both')"),
    sourceRoot: z.string().optional().describe("Source root for source maps"),
    sourcesContent: z.boolean().optional().describe("Include sources content in source maps"),
    legalComments: LegalCommentsEnum.optional().describe("How to handle legal comments (/* @license */)"),
    globalName: z.string().optional().describe("Global variable name for IIFE format"),
    supported: z.record(z.string(), z.boolean()).optional()
        .describe("Override feature support (e.g. {\"bigint\": true})"),
    mangleProps: z.string().optional()
        .describe("Regex pattern for property names to mangle (e.g. \"^_\")"),
    reserveProps: z.string().optional()
        .describe("Regex pattern for property names to exclude from mangling"),
    mangleQuoted: z.boolean().optional().describe("Also mangle quoted property names"),
    mangleCache: z.record(z.string(), z.union([z.string(), z.literal(false)])).optional()
        .describe("Mangle cache for stable renaming across builds"),
    logLevel: LogLevelEnum.optional().describe("Log level"),
    logLimit: z.number().optional().describe("Max number of log messages (0 = unlimited)"),
    logOverride: z.record(z.string(), LogLevelEnum).optional()
        .describe("Override log level for specific message IDs"),
    color: z.boolean().optional().describe("Enable ANSI color in log messages"),
};
/** Options specific to build/context/watch/serve (file-based) APIs */
export const BuildOnlySchema = {
    entryPoints: z.union([
        z.array(z.string()),
        z.record(z.string(), z.string()),
    ]).describe("Entry points: array of file paths or {outName: filePath} record"),
    bundle: z.boolean().optional().describe("Bundle imports into output (default: true)"),
    splitting: z.boolean().optional().describe("Enable code splitting (ESM only)"),
    metafile: z.boolean().optional().describe("Include bundle analysis metafile"),
    outdir: z.string().optional().describe("Output directory for multiple entry points"),
    outfile: z.string().optional().describe("Output file for single entry point"),
    outbase: z.string().optional().describe("Base path for output file names"),
    outExtensions: z.record(z.string(), z.string()).optional()
        .describe("Output file extension mapping (e.g. {\".js\": \".mjs\"})"),
    publicPath: z.string().optional().describe("URL prefix for asset file paths"),
    entryNames: z.string().optional().describe("Template for entry point output names (e.g. \"[dir]/[name]-[hash]\")"),
    chunkNames: z.string().optional().describe("Template for chunk output names"),
    assetNames: z.string().optional().describe("Template for asset output names"),
    external: z.array(z.string()).optional().describe("Package names or globs to exclude from bundle"),
    packages: PackagesEnum.optional().describe("How to handle packages: 'bundle' or 'external'"),
    alias: z.record(z.string(), z.string()).optional()
        .describe("Package import aliases (e.g. {\"oldpkg\": \"newpkg\"})"),
    resolveExtensions: z.array(z.string()).optional()
        .describe("Extensions to try when resolving (e.g. [\".tsx\", \".ts\", \".jsx\", \".js\"])"),
    mainFields: z.array(z.string()).optional()
        .describe("Package.json fields to try (e.g. [\"module\", \"main\"])"),
    conditions: z.array(z.string()).optional()
        .describe("Package.json export conditions (e.g. [\"development\", \"module\"])"),
    preserveSymlinks: z.boolean().optional().describe("Do not resolve symlinks"),
    nodePaths: z.array(z.string()).optional().describe("Additional module resolution paths"),
    tsconfig: z.string().optional().describe("Path to tsconfig.json"),
    loader: z.record(z.string(), LoaderEnum).optional()
        .describe("File extension to loader mapping (e.g. {\".png\": \"dataurl\"})"),
    inject: z.array(z.string()).optional()
        .describe("Files to inject into all modules (e.g. [\"./polyfill.js\"])"),
    banner: z.record(z.string(), z.string()).optional()
        .describe("Text to prepend per output type (e.g. {\"js\": \"/* banner */\"})"),
    footer: z.record(z.string(), z.string()).optional()
        .describe("Text to append per output type (e.g. {\"js\": \"/* footer */\"})"),
    stdin: z.object({
        contents: z.string().describe("Source code contents"),
        resolveDir: z.string().optional().describe("Directory for resolving imports"),
        sourcefile: z.string().optional().describe("File name for error messages"),
        loader: LoaderEnum.optional().describe("Loader for stdin contents"),
    }).optional().describe("Use stdin as entry point instead of files"),
    write: z.boolean().optional().describe("Write output files to disk"),
    allowOverwrite: z.boolean().optional().describe("Allow output files to overwrite input files"),
    absWorkingDir: z.string().optional().describe("Absolute working directory path"),
};
/** Options specific to the transform API */
export const TransformOnlySchema = {
    code: z.string().describe("Source code to transform"),
    loader: LoaderEnum.optional().describe("Loader to use (default: ts)"),
    banner: z.string().optional().describe("Text to prepend to output"),
    footer: z.string().optional().describe("Text to append to output"),
    sourcefile: z.string().optional().describe("File name for error messages and source maps"),
    tsconfigRaw: z.string().optional().describe("Raw tsconfig JSON override"),
};
/** Options specific to the serve API */
export const ServeOnlySchema = {
    port: z.number().optional().describe("Port to serve on (default: auto)"),
    host: z.string().optional().describe("Host to serve on (default: 0.0.0.0)"),
    servedir: z.string().optional().describe("Directory to serve static files from"),
    keyfile: z.string().optional().describe("Path to TLS key file for HTTPS"),
    certfile: z.string().optional().describe("Path to TLS certificate file for HTTPS"),
    fallback: z.string().optional().describe("Fallback HTML file for SPA routing (e.g. \"index.html\")"),
};
/**
 * Convert mangleProps/reserveProps string patterns to RegExp
 * and return a clean options object ready for esbuild.
 */
export function prepareBuildOptions(args) {
    const opts = { ...args };
    if (typeof opts.mangleProps === "string") {
        opts.mangleProps = new RegExp(opts.mangleProps);
    }
    if (typeof opts.reserveProps === "string") {
        opts.reserveProps = new RegExp(opts.reserveProps);
    }
    return opts;
}
