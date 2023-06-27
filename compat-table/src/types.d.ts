// Allow TypeScript to import untyped ".js" files
declare module '*.js' {
  let any: any
  export = any
}
