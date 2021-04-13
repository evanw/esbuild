declare module 'https://deno.land/x/compress@v0.3.3/mod.ts' {
  export interface InflateOptions {
    windowBits?: number;
    dictionary?: Uint8Array;
    chunkSize?: number;
    to?: string;
    raw?: boolean;
  }

  export function gunzip(input: Uint8Array, options?: InflateOptions): Uint8Array
}
