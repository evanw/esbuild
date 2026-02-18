import type * as esbuildType from "esbuild";

let api: typeof esbuildType;

export async function getEsbuild(): Promise<typeof esbuildType> {
  if (api) return api;

  if (process.env.ESBUILD_WASM === "1") {
    const esbuildWasm = await import("esbuild-wasm");
    await esbuildWasm.initialize({ worker: false });
    api = esbuildWasm as unknown as typeof esbuildType;
  } else {
    api = await import("esbuild");
  }

  return api;
}
