let api;
export async function getEsbuild() {
    if (api)
        return api;
    if (process.env.ESBUILD_WASM === "1") {
        const esbuildWasm = await import("esbuild-wasm");
        await esbuildWasm.initialize({ worker: false });
        api = esbuildWasm;
    }
    else {
        api = await import("esbuild");
    }
    return api;
}
