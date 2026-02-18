import * as esbuildWasm from "esbuild-wasm";
export interface WasmInitOptions {
    wasmURL?: string;
    wasmModule?: string;
    worker?: boolean;
}
export interface WasmState {
    status: "uninitialized" | "initializing" | "ready" | "error";
    version: string | null;
    options: WasmInitOptions | null;
    error: string | null;
    initializedAt: string | null;
}
export declare function initializeWasm(opts?: WasmInitOptions): Promise<WasmState>;
export declare function getState(): WasmState;
export declare function getEsbuildWasm(): Promise<typeof esbuildWasm>;
