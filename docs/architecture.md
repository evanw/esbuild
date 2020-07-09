* [Architecture](#architecture)
    * [Design principles](#design-principles)
* [Overview](#overview)
    * [Scan phase](#scan-phase)
    * [Compile phase](#compile-phase)
* [Notes about parsing](#notes-about-parsing)
    * [Symbols and scopes](#symbols-and-scopes)
    * [Constant folding](#constant-folding)
    * [TypeScript parsing](#typescript-parsing)
* [Notes about linking](#notes-about-linking)
    * [CommonJS linking](#commonjs-linking)
    * [ES6 linking](#es6-linking)
    * [Hybrid CommonJS and ES6 modules](#hybrid-commonjs-and-es6-modules)
    * [Scope hoisting](#scope-hoisting)
    * [Converting ES6 imports to CommonJS imports](#converting-es6-imports-to-commonjs-imports)
    * [The runtime library](#the-runtime-library)
    * [Tree shaking](#tree-shaking)
    * [Code splitting](#code-splitting)
* [Notes about printing](#notes-about-printing)

# Architecture Documentation

This document covers how esbuild's bundler works. It's intended to aid in understanding the code, in understanding what tricks esbuild uses to improve performance, and hopefully to enable people to modify the code.

Note that there are some design decisions that have been made differently than other bundlers for performance reasons. These decisions may make the code harder to work with. Keep in mind that this project is an experiment in progress, and is not the result of a comprehensive survey of implementation techniques. The way things work now is not necessarily the best way of doing things.

### Design principles

* **Maximize parallelism**

    Most of the time should be spent doing fully parallelizable work. This can be observed by taking a CPU trace using the `--trace=[file]` flag and viewing it using `go tool trace [file]`.

* **Avoid doing unnecessary work**

    For example, many bundlers have intermediate stages where they write out JavaScript code and read it back in using another tool. This work is unnecessary because if the tools used the same data structures, no conversion would be needed.

* **Transparently support both ES6 and CommonJS module syntax**

    The parser in esbuild processes a superset of both ES6 and CommonJS modules. It doesn't distinguish between ES6 modules and other modules so you can use both ES6 and CommonJS syntax in the same file if you'd like.

* **Try to do as few full-AST passes as possible for better cache locality**

    Compilers usually have many more passes because separate passes makes code easier to understand and maintain. There are currently only three full-AST passes in esbuild because individual passes have been merged together as much as possible:

    1. Lexing + parsing + scope setup + symbol declaration
    2. Symbol binding + constant folding + syntax lowering + syntax mangling
    3. Printing + source map generation

* **Structure things to permit a "watch mode" where compilation can happen incrementally**

    Incremental builds mean only rebuilding changed files to the greatest extent possible. This means not re-running any of the full-AST passes on unchanged files. Data structures that live across builds must be immutable to allow sharing. Unfortunately the Go type system can't enforce this, so care must be taken to uphold this as the code evolves.

## Overview

<p align="center"><img src="../images/build-pipeline.png" alt="Diagram of build pipeline" width="752"></p>

The build pipeline has two main phases: scan and compile. These both reside in [bundler.go](../internal/bundler/bundler.go).

### Scan phase

This phase starts with a set of entry points and traverses the dependency graph to find all modules that need to be in the bundle. This is implemented in `bundler.ScanBundle()` as a parallel worklist algorithm. The worklist starts off being the list of entry points. Each file in the list is parsed into an AST on a separate goroutine and may add more files to the worklist if it has any dependencies (either ES6 `import` statements, ES6 `import()` expressions, or CommonJS `require()` expressions). Scanning continues until the worklist is empty.

### Compile phase

This phase creates a bundle for each entry point, which involves first "linking" imports with exports, then converting the parsed ASTs back into JavaScript, then concatenating them together to form the final bundled file. This happens in `(*Bundle).Compile()`.

## Notes about parsing

The parser is separate from the lexer. The lexer is called on the fly as the file is parsed instead of lexing the entire input ahead of time. This is necessary due to certain syntactical features such as regular expressions vs. the division operator and JSX elements vs. the less-than operator, where which token is parsed depends on the semantic context.

Lexer lookahead has been kept to one token in almost all cases with the notable exception of TypeScript, which requires arbitrary lookahead to parse correctly. All such cases are in methods called `trySkipTypeScript*WithBacktracking()` in the parser.

The parser includes a lot of transformations, all of which have been condensed into just two passes for performance:

1. The first pass does lexing and parsing, sets up the scope tree, and declares all symbols in their respective scopes.

2. The second pass binds all identifiers to their respective symbols using the scope tree, substitutes compile-time definitions for their values, performs constant folding, does lowering of syntax if we're targeting an older version of JavaScript, and performs syntax mangling/compression if we're doing a production build.

Note that, from experience, the overhead of syscalls in import path resolution is appears to be very high. Caching syscall results in the resolver and the file system implementation is a very sizable speedup.

### Symbols and scopes

A symbol is a way to refer to an identifier in a precise way. Symbols are referenced using a 64-bit identifier instead of using the name, which makes them easy to refer to without worrying about scope. For example, the parser can generate new symbols without worrying about name collisions. All identifiers reference a symbol, even "unbound" ones that don't have a matching declaration. Symbols have to be declared in a separate pass from the pass that binds identifiers to symbols because JavaScript has "variable hoisting" where a child scope can declare a hoisted symbol that can become bound to identifiers in parent and sibling scopes.

Symbols for the whole file are stored in a flat top-level array. That way you can easily traverse over all symbols in the file without traversing the AST. That also lets us easily create a modified AST where the symbols have been changed without affecting the original immutable AST. Because symbols are identified by their index into the top-level symbol array, we can just clone the array to clone the symbols and we don't need to worry about rewiring all of the symbol references.

The scope tree is not attached to the AST because it's really only needed to pass information from the first pass to the second pass. The scope tree is instead temporarily mapped onto the AST within the parser. This is done by having the first and second passes both call `pushScope*()` and `popScope()` the same number of times in the same order. Specifically the first pass calls `pushScopeForParsePass()` which appends the pushed scope to `scopesInOrder`, and the second pass calls `pushScopeForVisitPass()` which reads off the scope to push from `scopesInOrder`.

This is mostly pretty straightforward except for a few places where the parser has pushed a scope and is in the middle of parsing a declaration only to discover that it's not a declaration after all. This happens in TypeScript when a function is forward-declared without a body, and in JavaScript when it's ambiguous whether a parenthesized expression is an arrow function or not until we reach the `=>` token afterwards. This would be solved by doing three passes instead of two so we finish parsing before starting to set up scopes and declare symbols, but we're trying to do this in just two passes. So instead we call `popAndDiscardScope()` or `popAndFlattenScope()` instead of `popScope()` to modify the scope tree later if our assumptions turn out to be incorrect.

### Constant folding

The constant folding and compile-time definition substitution is pretty minimal but is enough to handle libraries such as React which contain code like this:

```js
if (process.env.NODE_ENV === 'production') {
  module.exports = require('./cjs/react.production.min.js');
} else {
  module.exports = require('./cjs/react.development.js');
}
```

Using `--define:process.env.NODE_ENV="production"` on the command line will cause `process.env.NODE_ENV === 'production'` to become `"production" === 'production'` which will then become `true`. The parser then treats the `else` branch as dead code, which means it ignores calls to `require()` and `import()` inside that branch. The `react.development.js` module is never included in the dependency graph.

### TypeScript parsing

TypeScript parsing has been implemented by augmenting the existing JavaScript parser. Most of it just involves skipping over type declarations as if they are whitespace. Enums, namespaces, and TypeScript-only class features such as parameter properties must all be converted to JavaScript syntax, which happens in the second parser pass. I've attempted to match what the TypeScript compiler does as close as is reasonably possible.

One TypeScript subtlety is that unused imports in TypeScript code must be removed, since they may be type-only imports. And if all imports in an import statement are removed, the whole import statement itself must also be removed. This has semantic consequences because the import may have side effects. However, it's important for correctness because this is how the TypeScript compiler itself works. The imported package itself may not actually even exist on disk since it may only come from a `declare` statement. Tracking used imports is handled by the `tsUseCounts` field in the parser.

## Notes about linking

The main goal of linking is to merge multiple modules into a single file so that imports from one module can reference exports from another module. This is accomplished in several different ways depending on the import and export features used.

Linking performs an optimization called "tree shaking". This is also known as "dead code elimination" and removes unreferenced code from the bundle to reduce bundle size. Tree shaking is always active and cannot be disabled.

Finally, linking may also involve dividing the input code among multiple chunks. This is known as "code splitting" and both allows lazy loading of code and sharing code between multiple entry points. It's disabled by default in esbuild but can be enabled with the `--splitting` flag.

This will all be described in more detail below.

### CommonJS linking

If a module uses any CommonJS features (e.g. references `exports`, references `module`, or uses a top-level `return` statement) then it's considered a CommonJS module. This means it's represented as a separate closure within the bundle. This is similar to how Webpack normally works.

Here's a simplified example to explain what this looks like:

<table>
<tr><th>foo.js</th><th>bar.js</th><th>bundle.js</th></tr>
<tr><td>

```js
exports.fn = () => 123
```

</td><td>

```js
const foo = require('./foo')
console.log(foo.fn())
```

</td><td>

```js
let __commonJS = (callback, module) => () => {
  if (!module) {
    module = {exports: {}};
    callback(module.exports, module);
  }
  return module.exports;
};

// foo.js
var require_foo = __commonJS((exports) => {
  exports.fn = () => 123;
});

// bar.js
const foo = require_foo();
console.log(foo.fn());
```

</td></tr>
</table>

The benefit of bundling modules this way is for compatibility. This emulates exactly [how node itself will run your module](https://nodejs.org/api/modules.html#modules_the_module_wrapper).

### ES6 linking

If a module doesn't use any CommonJS features, then it's considered an ES6 module. This means it's represented as part of a cross-module scope that may contain many other ES6 modules. This is often known as "scope hoisting" and is how Rollup normally works.

Here's a simplified example to explain what this looks like:

<table>
<tr><th>foo.js</th><th>bar.js</th><th>bundle.js</th></tr>
<tr><td>

```js
export const fn = () => 123
```

</td><td>

```js
import {fn} from './foo'
console.log(fn())
```

</td><td>

```js
// foo.js
const fn = () => 123;

// bar.js
console.log(fn());
```

</td></tr>
</table>

The benefit of distinguishing between CommonJS and ES6 modules is that bundling ES6 modules is more efficient, both because the generated code is smaller and because symbols are statically bound instead of dynamically bound, which has less overhead at run time.

ES6 modules also allow for "tree shaking" optimizations which remove unreferenced code from the bundle. For example, if the call to `fn()` is commented out in the above example, the variable `fn` will be omitted from the bundle since it's not used and its definition doesn't have any side effects. This is possible with ES6 modules but not with CommonJS because ES6 imports are bound at compile time while CommonJS imports are bound at run time.

### Hybrid CommonJS and ES6 modules

These two syntaxes are supported side-by-side as transparently as possible. This means you can use both CommonJS syntax (`exports` and `module` assignments and `require()` calls) and ES6 syntax (`import` and `export` statements and `import()` expressions) in the same module. The ES6 imports will be converted to `require()` calls and the ES6 exports will be converted to getters on that module's `exports` object.

### Scope hoisting

Scope hoisting (the merging of all scopes in a module group into a single scope) is implemented using symbol merging. Each imported symbol is merged with the corresponding exported symbol so that they become the same symbol in the output, which means they both get the same name. Symbol merging is possible because each symbol has a `Link` field that, when used, forwards to another symbol. The implementation of `MergeSymbols()` essentially just links one symbol to the other one. Whenever the printer sees a symbol reference it must call `FollowSymbols()` to get to the symbol at the end of the link chain, which represents the final merged symbol. This is similar to the [union-find data structure](https://en.wikipedia.org/wiki/Disjoint-set_data_structure) if you're familiar with it.

During bundling, the symbol maps from all files are merged into a single giant symbol map, which allows symbols to be merged across files. The symbol map is represented as an array-of-arrays and a symbol reference is represented as two indices, one for the outer array and one for the inner array. The array-of-arrays representation is convenient because the parser produces a single symbol array for each file. Merging them all into a single map is as simple as making an array of the symbol arrays for each file. Each source file is identified using an incrementing index allocated during the scanning phase, so the index of the outer array is just the index of the source file.

### Converting ES6 imports to CommonJS imports

One complexity around scope hoisting is that references to ES6 imports may either be a bare identifier (i.e. statically bound) or a property access off of a `require()` call (i.e. dynamically bound) depending on whether the imported module is a CommonJS-style module or not. This information isn't known yet when we're still parsing the file so we are unable to determine whether to create `EIdentifier` or `EDot` AST nodes for these imports.

To handle this, references to ES6 imports use the special `EImportIdentifier` AST node. Later during linking we can decide if these references to a symbol need to be turned into a property access and, if so, fill in the `NamespaceAlias` field on the symbol. The printer checks that field for `EImportIdentifier` expressions and, if present, prints a property access instead of an identifier. This avoids having to do another full-AST traversal just to replace identifiers with property accesses before printing.

### The runtime library

This library contains support code that is needed to implement various aspects of JavaScript transformation and bundling. For example, it contains the `__commonJS()` helper function for wrapping CommonJS modules and the `__decorate()` helper function for implementing TypeScript decorators. The code lives in a single string in [runtime.go](../internal/runtime/runtime.go). It's automatically included in every build and esbuild's tree shaking feature automatically strips out unused code. If you need to add a helper function for esbuild to call, it should be added to this library.

### Tree shaking

The goal of tree shaking is to remove code that will never be used from the final bundle, which reduces download and parse time. Tree shaking treats the input files as a graph. Each node in the graph is a top-level statement, which is called a "part" in the code. Tree shaking is a graph traversal that starts from the entry point and marks all traversed parts for inclusion.

Each part may declare symbols, reference symbols, and depend on other files. Parts are also marked as either having side effects or not. For example, the statement `let foo = 123` does not have side effects because, if nothing needs `foo`, the statement can be removed without any observable difference. But the statement `let foo = bar()` does have side effects because even if nothing needs `foo`, the call to `bar()` cannot be removed without changing the meaning of the code.

If part A references a symbol declared in part B, the graph has an edge from A to B. References can span across files due to ES6 imports and exports. And if part A depends on file C, the graph has an edge from A to every part in C with side effects. A part depends on a file if it contains an ES6 `import` statement, a CommonJS `require()` call, or an ES6 `import()` expression.

Tree shaking begins by visiting all parts in the entry point file with side effects, and continues traversing along graph edges until no more new parts are reached. Once the traversal has finished, only parts that were reached during the traversal are included in the bundle. All other parts are excluded.

Here's an example to make this easier to visualize:

<p align="center"><img src="../images/tree-shaking.png" alt="Diagram of tree shaking" width="793"></p>

There are three input files: `index.js`, `config.js`, and `net.js`. Tree shaking traverses along all graph edges from `index.js` (the entry point). The two types of edges are shown with different arrows. Solid arrows are edges due to parts with side effects. These parts must be included regardless of whether the symbols they declare are used or not. Dashed arrows are edges from symbol references to the parts that declare those symbols. These parts don't have side effects and are only included if symbol they declare is referenced.

The final bundle only includes the code visited during the tree shaking traversal. That looks like this:

```js
// net.js
function get(url) {
  return fetch(url).then((r) => r.text());
}

// config.js
let session = Math.random();
let api = "/api?session=";
function load() {
  return get(api + session);
}

// index.js
let el = document.getElementById("el");
load().then((x) => el.textContent = x);
```

### Code splitting

Code splitting analyzes bundles with multiple entry points and divides code into chunks such that a) a given piece of code is only ever in one chunk and b) each entry point doesn't download code that it will never use. Note that the target of each dynamic `import()` expression is considered an additional entry point.

Splitting shared code into separate chunks means that downloading the code for two entry points only downloads the shared code once. It also allows code that's only needed for an asynchronous `import()` dependency to be lazily loaded.

Code splitting is implemented as an advanced form of tree shaking. The tree shaking traversal described above is run once for each entry point. Every part (i.e. node in the graph) stores all of the entry points that reached it during the traversal for that entry point. Then the combination of entry points for a given part determines what chunk that part ends up in.

To continue the tree shaking example above, let's add a second entry point called `settings.js` that uses a different but overlapping set of parts. Tree shaking is run again starting from this new entry point:

<p align="center"><img src="../images/code-splitting-1.png" alt="Diagram of code splitting" width="793"></p>

These two tree shaking passes result in three chunks: all parts only reachable from `index.js`, all parts only reachable from `settings.js`, and all parts reachable from both `index.js` and `settings.js`. Parts belonging to the three chunks are colored red, blue, and purple in the visualization below:

<p align="center"><img src="../images/code-splitting-2.png" alt="Diagram of code splitting" width="793"></p>

After all chunks are identified, the chunks are linked together by automatically generating import and export statements for references to symbols that are declared in another chunk. Import statements must also be inserted for chunks that don't have any exported symbols. This represents shared code with side effects, and code with side effects must be retained.

Here are the final code splitting chunks for this example after linking:

<table>
<tr><th>Chunk for index.js</th><th>Chunk for settings.js</th><th>Chunk for shared code</th></tr>
<tr><td>

```js
import {
  api,
  session
} from "./chunk.js";

// net.js
function get(url) {
  return fetch(url).then((r) => r.text());
}

// config.js
function load() {
  return get(api + session);
}

// index.js
let el = document.getElementById("el");
load().then((x) => el.textContent = x);
```

</td><td>

```js
import {
  api,
  session
} from "./chunk.js";

// net.js
function put(url, body) {
  fetch(url, {method: "PUT", body});
}

// config.js
function save(value) {
  return put(api + session, value);
}

// settings.js
let it = document.getElementById("it");
it.oninput = () => save(it.value);
```

</td><td>

```js
// config.js
let session = Math.random();
let api = "/api?session=";

export {
  api,
  session
};
```

</td></tr>
</table>

There is one additional complexity to code splitting due to how ES6 module boundaries work. Code splitting must not be allowed to move an assignment to a module-local variable into a separate chunk from the declaration of that variable. ES6 imports are read-only and cannot be assigned to, so doing this will cause the assignment to crash at run time.

To illustrate the problem, consider these three files:

<table>
<tr><th>entry1.js</th><th>entry2.js</th><th>data.js</th></tr>
<tr><td>

```js
import {data} from './data'
console.log(data)
```

</td><td>

```js
import {setData} from './data'
setData(123)
```

</td><td>

```js
export let data
export function setData(value) {
  data = value
}
```

</td></tr>
</table>

If the two entry points `entry1.js` and `entry2.js` are bundled with the code splitting algorithm described above, the result will be this invalid code:

<table>
<tr><th>Chunk for entry1.js</th><th>Chunk for entry2.js</th><th>Chunk for shared code</th></tr>
<tr><td>

```js
import {
  data
} from "./chunk.js";

// entry1.js
console.log(data);
```

</td><td>

```js
import {
  data
} from "./chunk.js";

// data.js
function setData(value) {
  data = value;
}

// entry2.js
setData(123);
```

</td><td>

```js
// data.js
let data;

export {
  data
};
```

</td></tr>
</table>

The assignment `data = value` will crash at run time with `TypeError: Assignment to constant variable`. To fix this, we must make sure that assignment ends up in the same chunk as the declaration `let data`.

This is done by unioning the entry point sets of the parts with the assignments and the parts with the symbol declarations together. That way all of those parts are marked as reachable from all entry points that can reach any of those parts. This is only relevant for locally-declared symbols so each module can be processed independently.

The grouping of parts can be non-trivial because there may be many parts involved and many assignments to different variables. Grouping is done by finding connected components on the graph where nodes are parts and edges are cross-part assignments.

With this algorithm, the function `setData` in our example moves into the chunk of shared code after being bundled with code splitting:

<table>
<tr><th>Chunk for entry1.js</th><th>Chunk for entry2.js</th><th>Chunk for shared code</th></tr>
<tr><td>

```js
import {
  data
} from "./chunk.js";

// entry1.js
console.log(data);
```

</td><td>

```js
import {
  setData
} from "./chunk.js";

// entry2.js
setData(123);
```

</td><td>

```js
// data.js
let data;
function setData(value) {
  data = value;
}

export {
  data,
  setData
};
```

</td></tr>
</table>

This code no longer contains assignments to cross-chunk variables.

## Notes about printing

The printer converts JavaScript ASTs back into JavaScript source code. This is mainly intended to be consumed by the JavaScript VM for execution, with a secondary goal of being readable enough to debug when minification is disabled. It's not intended to be used as a code formatting tool and does not make complex formatting decisions. It handles the insertion of parentheses to preserve operator precedence as appropriate.

Each file is printed independently from other files, so files can be printed in parallel. This extends to source map generation. As each file is printed, the printer builds up a "source map chunk" which is a [VLQ](https://en.wikipedia.org/wiki/Variable-length_quantity)-encoded sequence of source map offsets assuming the output file starts with the AST currently being printed. That source map chunk will later be "rebased" to start at the correct offset when all source map chunks are joined together. This is done by rewriting the first item in the sequence, which happens in `AppendSourceMapChunk()`.

The current AST representation uses a single integer offset per AST node to store the location information. This is the index of the starting byte for that syntax construct in the original source file. Using this representation means that it's not possible to merge ASTs from two separate files and still have source maps work. That's not a problem since AST printing is fully parallelized in esbuild, but is something to keep in mind when modifying the code.
