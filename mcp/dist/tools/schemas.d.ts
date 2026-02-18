import { z } from "zod";
/** Options shared by both build and transform APIs */
export declare const CommonSchema: {
    format: z.ZodOptional<z.ZodEnum<{
        iife: "iife";
        cjs: "cjs";
        esm: "esm";
    }>>;
    target: z.ZodOptional<z.ZodUnion<readonly [z.ZodString, z.ZodArray<z.ZodString>]>>;
    platform: z.ZodOptional<z.ZodEnum<{
        browser: "browser";
        node: "node";
        neutral: "neutral";
    }>>;
    minify: z.ZodOptional<z.ZodBoolean>;
    minifyWhitespace: z.ZodOptional<z.ZodBoolean>;
    minifyIdentifiers: z.ZodOptional<z.ZodBoolean>;
    minifySyntax: z.ZodOptional<z.ZodBoolean>;
    define: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodString>>;
    pure: z.ZodOptional<z.ZodArray<z.ZodString>>;
    keepNames: z.ZodOptional<z.ZodBoolean>;
    drop: z.ZodOptional<z.ZodArray<z.ZodEnum<{
        console: "console";
        debugger: "debugger";
    }>>>;
    dropLabels: z.ZodOptional<z.ZodArray<z.ZodString>>;
    charset: z.ZodOptional<z.ZodEnum<{
        ascii: "ascii";
        utf8: "utf8";
    }>>;
    lineLimit: z.ZodOptional<z.ZodNumber>;
    treeShaking: z.ZodOptional<z.ZodBoolean>;
    ignoreAnnotations: z.ZodOptional<z.ZodBoolean>;
    jsx: z.ZodOptional<z.ZodEnum<{
        transform: "transform";
        preserve: "preserve";
        automatic: "automatic";
    }>>;
    jsxFactory: z.ZodOptional<z.ZodString>;
    jsxFragment: z.ZodOptional<z.ZodString>;
    jsxImportSource: z.ZodOptional<z.ZodString>;
    jsxDev: z.ZodOptional<z.ZodBoolean>;
    jsxSideEffects: z.ZodOptional<z.ZodBoolean>;
    sourcemap: z.ZodOptional<z.ZodUnion<readonly [z.ZodBoolean, z.ZodEnum<{
        linked: "linked";
        inline: "inline";
        external: "external";
        both: "both";
    }>]>>;
    sourceRoot: z.ZodOptional<z.ZodString>;
    sourcesContent: z.ZodOptional<z.ZodBoolean>;
    legalComments: z.ZodOptional<z.ZodEnum<{
        linked: "linked";
        inline: "inline";
        external: "external";
        none: "none";
        eof: "eof";
    }>>;
    globalName: z.ZodOptional<z.ZodString>;
    supported: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodBoolean>>;
    mangleProps: z.ZodOptional<z.ZodString>;
    reserveProps: z.ZodOptional<z.ZodString>;
    mangleQuoted: z.ZodOptional<z.ZodBoolean>;
    mangleCache: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodUnion<readonly [z.ZodString, z.ZodLiteral<false>]>>>;
    logLevel: z.ZodOptional<z.ZodEnum<{
        verbose: "verbose";
        debug: "debug";
        info: "info";
        warning: "warning";
        error: "error";
        silent: "silent";
    }>>;
    logLimit: z.ZodOptional<z.ZodNumber>;
    logOverride: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodEnum<{
        verbose: "verbose";
        debug: "debug";
        info: "info";
        warning: "warning";
        error: "error";
        silent: "silent";
    }>>>;
    color: z.ZodOptional<z.ZodBoolean>;
};
/** Options specific to build/context/watch/serve (file-based) APIs */
export declare const BuildOnlySchema: {
    entryPoints: z.ZodUnion<readonly [z.ZodArray<z.ZodString>, z.ZodRecord<z.ZodString, z.ZodString>]>;
    bundle: z.ZodOptional<z.ZodBoolean>;
    splitting: z.ZodOptional<z.ZodBoolean>;
    metafile: z.ZodOptional<z.ZodBoolean>;
    outdir: z.ZodOptional<z.ZodString>;
    outfile: z.ZodOptional<z.ZodString>;
    outbase: z.ZodOptional<z.ZodString>;
    outExtensions: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodString>>;
    publicPath: z.ZodOptional<z.ZodString>;
    entryNames: z.ZodOptional<z.ZodString>;
    chunkNames: z.ZodOptional<z.ZodString>;
    assetNames: z.ZodOptional<z.ZodString>;
    external: z.ZodOptional<z.ZodArray<z.ZodString>>;
    packages: z.ZodOptional<z.ZodEnum<{
        external: "external";
        bundle: "bundle";
    }>>;
    alias: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodString>>;
    resolveExtensions: z.ZodOptional<z.ZodArray<z.ZodString>>;
    mainFields: z.ZodOptional<z.ZodArray<z.ZodString>>;
    conditions: z.ZodOptional<z.ZodArray<z.ZodString>>;
    preserveSymlinks: z.ZodOptional<z.ZodBoolean>;
    nodePaths: z.ZodOptional<z.ZodArray<z.ZodString>>;
    tsconfig: z.ZodOptional<z.ZodString>;
    loader: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodEnum<{
        text: "text";
        default: "default";
        js: "js";
        jsx: "jsx";
        ts: "ts";
        tsx: "tsx";
        css: "css";
        "local-css": "local-css";
        json: "json";
        base64: "base64";
        binary: "binary";
        dataurl: "dataurl";
        copy: "copy";
        empty: "empty";
        file: "file";
    }>>>;
    inject: z.ZodOptional<z.ZodArray<z.ZodString>>;
    banner: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodString>>;
    footer: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodString>>;
    stdin: z.ZodOptional<z.ZodObject<{
        contents: z.ZodString;
        resolveDir: z.ZodOptional<z.ZodString>;
        sourcefile: z.ZodOptional<z.ZodString>;
        loader: z.ZodOptional<z.ZodEnum<{
            text: "text";
            default: "default";
            js: "js";
            jsx: "jsx";
            ts: "ts";
            tsx: "tsx";
            css: "css";
            "local-css": "local-css";
            json: "json";
            base64: "base64";
            binary: "binary";
            dataurl: "dataurl";
            copy: "copy";
            empty: "empty";
            file: "file";
        }>>;
    }, z.core.$strip>>;
    write: z.ZodOptional<z.ZodBoolean>;
    allowOverwrite: z.ZodOptional<z.ZodBoolean>;
    absWorkingDir: z.ZodOptional<z.ZodString>;
};
/** Options specific to the transform API */
export declare const TransformOnlySchema: {
    code: z.ZodString;
    loader: z.ZodOptional<z.ZodEnum<{
        text: "text";
        default: "default";
        js: "js";
        jsx: "jsx";
        ts: "ts";
        tsx: "tsx";
        css: "css";
        "local-css": "local-css";
        json: "json";
        base64: "base64";
        binary: "binary";
        dataurl: "dataurl";
        copy: "copy";
        empty: "empty";
        file: "file";
    }>>;
    banner: z.ZodOptional<z.ZodString>;
    footer: z.ZodOptional<z.ZodString>;
    sourcefile: z.ZodOptional<z.ZodString>;
    tsconfigRaw: z.ZodOptional<z.ZodString>;
};
/** Options specific to the serve API */
export declare const ServeOnlySchema: {
    port: z.ZodOptional<z.ZodNumber>;
    host: z.ZodOptional<z.ZodString>;
    servedir: z.ZodOptional<z.ZodString>;
    keyfile: z.ZodOptional<z.ZodString>;
    certfile: z.ZodOptional<z.ZodString>;
    fallback: z.ZodOptional<z.ZodString>;
};
/**
 * Convert mangleProps/reserveProps string patterns to RegExp
 * and return a clean options object ready for esbuild.
 */
export declare function prepareBuildOptions(args: Record<string, unknown>): Record<string, unknown>;
