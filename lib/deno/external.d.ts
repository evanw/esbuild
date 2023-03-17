declare module "https://deno.land/x/denoflate@1.2.1/mod.ts" {
  export function gunzip(input: Uint8Array): Uint8Array
}

// This is used by "worker.ts"
declare let onmessage: (event: any) => void
