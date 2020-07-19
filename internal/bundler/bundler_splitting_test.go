package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
)

func TestSplittingSharedES6IntoES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import {foo} from "./shared.js"
				console.log(foo)
			`,
			"/b.js": `
				import {foo} from "./shared.js"
				console.log(foo)
			`,
			"/shared.js": `export let foo = 123`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			IsBundling:    true,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
		expected: map[string]string{
			"/out/a.js": `import {
  foo
} from "./chunk.xL6KqlYO.js";

// /a.js
console.log(foo);
`,
			"/out/b.js": `import {
  foo
} from "./chunk.xL6KqlYO.js";

// /b.js
console.log(foo);
`,
			"/out/chunk.xL6KqlYO.js": `// /shared.js
let foo = 123;

export {
  foo
};
`,
		},
	})
}

func TestSplittingSharedCommonJSIntoES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				const {foo} = require("./shared.js")
				console.log(foo)
			`,
			"/b.js": `
				const {foo} = require("./shared.js")
				console.log(foo)
			`,
			"/shared.js": `exports.foo = 123`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			IsBundling:    true,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
		expected: map[string]string{
			"/out/a.js": `import {
  require_shared
} from "./chunk.xL6KqlYO.js";

// /a.js
const {foo} = require_shared();
console.log(foo);
`,
			"/out/b.js": `import {
  require_shared
} from "./chunk.xL6KqlYO.js";

// /b.js
const {foo: foo2} = require_shared();
console.log(foo2);
`,
			"/out/chunk.xL6KqlYO.js": `// /shared.js
var require_shared = __commonJS((exports) => {
  exports.foo = 123;
});

export {
  require_shared
};
`,
		},
	})
}

func TestSplittingDynamicES6IntoES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import("./foo.js").then(({bar}) => console.log(bar))
			`,
			"/foo.js": `
				export let bar = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
		expected: map[string]string{
			"/out/entry.js": `// /entry.js
import("./foo.js").then(({bar: bar2}) => console.log(bar2));
`,
			"/out/foo.js": `// /foo.js
let bar = 123;
export {
  bar
};
`,
		},
	})
}

func TestSplittingDynamicCommonJSIntoES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import("./foo.js").then(({default: {bar}}) => console.log(bar))
			`,
			"/foo.js": `
				exports.bar = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
		expected: map[string]string{
			"/out/entry.js": `// /entry.js
import("./foo.js").then(({default: {bar}}) => console.log(bar));
`,
			"/out/foo.js": `// /foo.js
var require_foo = __commonJS((exports) => {
  exports.bar = 123;
});
export default require_foo();
`,
		},
	})
}

func TestSplittingDynamicAndNotDynamicES6IntoES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {bar as a} from "./foo.js"
				import("./foo.js").then(({bar: b}) => console.log(a, b))
			`,
			"/foo.js": `
				export let bar = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
		expected: map[string]string{
			"/out/entry.js": `import {
  bar
} from "./chunk.-fk8OGuR.js";

// /entry.js
import("./foo.js").then(({bar: b}) => console.log(bar, b));
`,
			"/out/foo.js": `import {
  bar
} from "./chunk.-fk8OGuR.js";

// /foo.js
export {
  bar
};
`,
			"/out/chunk.-fk8OGuR.js": `// /foo.js
let bar = 123;

export {
  bar
};
`,
		},
	})
}

func TestSplittingDynamicAndNotDynamicCommonJSIntoES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {bar as a} from "./foo.js"
				import("./foo.js").then(({default: {bar: b}}) => console.log(a, b))
			`,
			"/foo.js": `
				exports.bar = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			IsBundling:    true,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
		expected: map[string]string{
			"/out/entry.js": `import {
  require_foo
} from "./chunk.-fk8OGuR.js";

// /entry.js
const foo = __toModule(require_foo());
import("./foo.js").then(({default: {bar: b}}) => console.log(foo.bar, b));
`,
			"/out/foo.js": `import {
  require_foo
} from "./chunk.-fk8OGuR.js";

// /foo.js
export default require_foo();
`,
			"/out/chunk.-fk8OGuR.js": `// /foo.js
var require_foo = __commonJS((exports) => {
  exports.bar = 123;
});

export {
  require_foo
};
`,
		},
	})
}

func TestSplittingAssignToLocal(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import {foo, setFoo} from "./shared.js"
				setFoo(123)
				console.log(foo)
			`,
			"/b.js": `
				import {foo} from "./shared.js"
				console.log(foo)
			`,
			"/shared.js": `
				export let foo
				export function setFoo(value) {
					foo = value
				}
			`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			IsBundling:    true,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
		expected: map[string]string{
			"/out/a.js": `import {
  foo,
  setFoo
} from "./chunk.xL6KqlYO.js";

// /a.js
setFoo(123);
console.log(foo);
`,
			"/out/b.js": `import {
  foo
} from "./chunk.xL6KqlYO.js";

// /b.js
console.log(foo);
`,
			"/out/chunk.xL6KqlYO.js": `// /shared.js
let foo;
function setFoo(value) {
  foo = value;
}

export {
  foo,
  setFoo
};
`,
		},
	})
}

func TestSplittingSideEffectsWithoutDependencies(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import {a} from "./shared.js"
				console.log(a)
			`,
			"/b.js": `
				import {b} from "./shared.js"
				console.log(b)
			`,
			"/shared.js": `
				export let a = 1
				export let b = 2
				console.log('side effect')
			`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			IsBundling:    true,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
		expected: map[string]string{
			"/out/a.js": `import "./chunk.xL6KqlYO.js";

// /shared.js
let a = 1;

// /a.js
console.log(a);
`,
			"/out/b.js": `import "./chunk.xL6KqlYO.js";

// /shared.js
let b = 2;

// /b.js
console.log(b);
`,
			"/out/chunk.xL6KqlYO.js": `// /shared.js
console.log("side effect");
`,
		},
	})
}

func TestSplittingNestedDirectories(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/pages/pageA/page.js": `
				import x from "../shared.js"
				console.log(x)
			`,
			"/Users/user/project/src/pages/pageB/page.js": `
				import x from "../shared.js"
				console.log(-x)
			`,
			"/Users/user/project/src/pages/shared.js": `
				export default 123
			`,
		},
		entryPaths: []string{
			"/Users/user/project/src/pages/pageA/page.js",
			"/Users/user/project/src/pages/pageB/page.js",
		},
		options: config.Options{
			IsBundling:    true,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/Users/user/project/out",
		},
		expected: map[string]string{
			"/Users/user/project/out/pageA/page.js": `import {
  shared_default
} from "../chunk.UcWke4C2.js";

// /Users/user/project/src/pages/pageA/page.js
console.log(shared_default);
`,
			"/Users/user/project/out/pageB/page.js": `import {
  shared_default
} from "../chunk.UcWke4C2.js";

// /Users/user/project/src/pages/pageB/page.js
console.log(-shared_default);
`,
			"/Users/user/project/out/chunk.UcWke4C2.js": `// /Users/user/project/src/pages/shared.js
var shared_default = 123;

export {
  shared_default
};
`,
		},
	})
}

func TestSplittingCircularReferenceIssue251(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				export * from './b.js';
				export var p = 5;
			`,
			"/b.js": `
				export * from './a.js';
				export var q = 6;
			`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			IsBundling:    true,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
		expected: map[string]string{
			"/out/a.js": `import {
  p,
  q
} from "./chunk.xL6KqlYO.js";

// /a.js
export {
  p,
  q
};
`,
			"/out/b.js": `import {
  p,
  q
} from "./chunk.xL6KqlYO.js";

// /b.js
export {
  p,
  q
};
`,
			"/out/chunk.xL6KqlYO.js": `// /b.js
var q = 6;

// /a.js
var p = 5;

export {
  p,
  q
};
`,
		},
	})
}

func TestSplittingMissingLazyExport(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import {foo} from './common.js'
				console.log(foo())
			`,
			"/b.js": `
				import {bar} from './common.js'
				console.log(bar())
			`,
			"/common.js": `
				import * as ns from './empty.js'
				export function foo() { return [ns, ns.missing] }
				export function bar() { return [ns.missing] }
			`,
			"/empty.js": `
				// This forces the module into ES6 mode without importing or exporting anything
				import.meta
			`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			IsBundling:    true,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
		expected: map[string]string{
			"/out/a.js": `import "./chunk.xL6KqlYO.js";

// /empty.js
const empty_exports = {};

// /common.js
function foo() {
  return [empty_exports, void 0];
}

// /a.js
console.log(foo());
`,
			"/out/b.js": `import "./chunk.xL6KqlYO.js";

// /common.js
function bar() {
  return [void 0];
}

// /b.js
console.log(bar());
`,
			"/out/chunk.xL6KqlYO.js": `// /empty.js

// /common.js
`,
		},
	})
}

func TestSplittingReExportIssue273(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				export const a = 1
			`,
			"/b.js": `
				export { a } from './a'
			`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			IsBundling:    true,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
		expected: map[string]string{
			"/out/a.js": `import {
  a
} from "./chunk.xL6KqlYO.js";

// /a.js
export {
  a
};
`,
			"/out/b.js": `import {
  a
} from "./chunk.xL6KqlYO.js";

// /b.js
export {
  a
};
`,
			"/out/chunk.xL6KqlYO.js": `// /a.js
const a = 1;

export {
  a
};
`,
		},
	})
}

func TestSplittingDynamicImportIssue272(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import('./b')
			`,
			"/b.js": `
				export default 1
			`,
		},
		entryPaths: []string{"/a.js", "/b.js"},
		options: config.Options{
			IsBundling:    true,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
		expected: map[string]string{
			"/out/a.js": `// /a.js
import("./b.js");
`,
			"/out/b.js": `// /b.js
var b_default = 1;
export {
  b_default as default
};
`,
		},
	})
}

func TestSplittingDynamicImportOutsideSourceTreeIssue264(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry1.js": `
				import('package')
			`,
			"/Users/user/project/src/entry2.js": `
				import('package')
			`,
			"/Users/user/project/node_modules/package/index.js": `
				console.log('imported')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry1.js", "/Users/user/project/src/entry2.js"},
		options: config.Options{
			IsBundling:    true,
			CodeSplitting: true,
			OutputFormat:  config.FormatESModule,
			AbsOutputDir:  "/out",
		},
		expected: map[string]string{
			"/out/src/entry1.js": `// /Users/user/project/src/entry1.js
import("../node_modules/package/index.js");
`,
			"/out/src/entry2.js": `// /Users/user/project/src/entry2.js
import("../node_modules/package/index.js");
`,
			"/out/node_modules/package/index.js": `// /Users/user/project/node_modules/package/index.js
console.log("imported");
`,
		},
	})
}
