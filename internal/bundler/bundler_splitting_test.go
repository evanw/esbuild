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
} from "./chunk.n2y-pUDL.js";

// /a.js
console.log(foo);
`,
			"/out/b.js": `import {
  foo
} from "./chunk.n2y-pUDL.js";

// /b.js
console.log(foo);
`,
			"/out/chunk.n2y-pUDL.js": `// /shared.js
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
} from "./chunk.n2y-pUDL.js";

// /a.js
const {foo} = require_shared();
console.log(foo);
`,
			"/out/b.js": `import {
  require_shared
} from "./chunk.n2y-pUDL.js";

// /b.js
const {foo: foo2} = require_shared();
console.log(foo2);
`,
			"/out/chunk.n2y-pUDL.js": `// /shared.js
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
} from "./chunk.t6dktdAy.js";

// /entry.js
import("./foo.js").then(({bar: b}) => console.log(bar, b));
`,
			"/out/foo.js": `import {
  bar
} from "./chunk.t6dktdAy.js";

// /foo.js
export {
  bar
};
`,
			"/out/chunk.t6dktdAy.js": `// /foo.js
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
} from "./chunk.t6dktdAy.js";

// /entry.js
const foo = __toModule(require_foo());
import("./foo.js").then(({default: {bar: b}}) => console.log(foo.bar, b));
`,
			"/out/foo.js": `import {
  require_foo
} from "./chunk.t6dktdAy.js";

// /foo.js
export default require_foo();
`,
			"/out/chunk.t6dktdAy.js": `// /foo.js
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
} from "./chunk.n2y-pUDL.js";

// /a.js
setFoo(123);
console.log(foo);
`,
			"/out/b.js": `import {
  foo
} from "./chunk.n2y-pUDL.js";

// /b.js
console.log(foo);
`,
			"/out/chunk.n2y-pUDL.js": `// /shared.js
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
