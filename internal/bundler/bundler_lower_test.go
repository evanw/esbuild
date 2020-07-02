// This file contains tests for "lowering" syntax, which means converting it to
// older JavaScript. For example, "a ** b" becomes a call to "Math.pow(a, b)"
// when lowered. Which syntax is lowered is determined by the language target.

package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/parser"
)

func TestLowerOptionalCatchNameCollisionNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				try {}
				catch { var e, e2 }
				var e3
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ES2018,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `try {
} catch (e4) {
  var e, e2;
}
var e3;
`,
		},
	})
}

func TestLowerObjectSpreadNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.jsx": `
				let tests = [
					{...a, ...b},
					{a, b, ...c},
					{...a, b, c},
					{a, ...b, c},
					{a, b, ...c, ...d, e, f, ...g, ...h, i, j},
				]
				let jsx = [
					<div {...a} {...b}/>,
					<div a b {...c}/>,
					<div {...a} b c/>,
					<div a {...b} c/>,
					<div a b {...c} {...d} e f {...g} {...h} i j/>,
				]
			`,
		},
		entryPaths: []string{"/entry.jsx"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ES2017,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `let tests = [
  __assign(__assign({}, a), b),
  __assign({a, b}, c),
  __assign(__assign({}, a), {b, c}),
  __assign(__assign({a}, b), {c}),
  __assign(__assign(__assign(__assign(__assign(__assign({a, b}, c), d), {e, f}), g), h), {i, j})
];
let jsx = [
  React.createElement("div", __assign(__assign({}, a), b)),
  React.createElement("div", __assign({
    a: true,
    b: true
  }, c)),
  React.createElement("div", __assign(__assign({}, a), {
    b: true,
    c: true
  })),
  React.createElement("div", __assign(__assign({
    a: true
  }, b), {
    c: true
  })),
  React.createElement("div", __assign(__assign(__assign(__assign(__assign(__assign({
    a: true,
    b: true
  }, c), d), {
    e: true,
    f: true
  }), g), h), {
    i: true,
    j: true
  }))
];
`,
		},
	})
}

func TestLowerExponentiationOperatorNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				let tests = {
					// Exponentiation operator
					0: a ** b ** c,
					1: (a ** b) ** c,

					// Exponentiation assignment operator
					2: a **= b,
					3: a.b **= c,
					4: a[b] **= c,
					5: a().b **= c,
					6: a()[b] **= c,
					7: a[b()] **= c,
					8: a()[b()] **= c,

					// These all should not need capturing (no object identity)
					9: a[0] **= b,
					10: a[false] **= b,
					11: a[null] **= b,
					12: a[void 0] **= b,
					13: a[123n] **= b,
					14: a[this] **= b,

					// These should need capturing (have object identitiy)
					15: a[/x/] **= b,
					16: a[{}] **= b,
					17: a[[]] **= b,
					18: a[() => {}] **= b,
					19: a[function() {}] **= b,
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ES2015,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: "/entry.js: error: Big integer literals are from ES2020 and transforming them to ES2015 is not supported\n",
		expected: map[string]string{
			"/out.js": `var _a, _b, _c, _d, _e, _f, _g, _h, _i, _j;
let tests = {
  0: __pow(a, __pow(b, c)),
  1: __pow(__pow(a, b), c),
  2: a = __pow(a, b),
  3: a.b = __pow(a.b, c),
  4: a[b] = __pow(a[b], c),
  5: (_a = a()).b = __pow(_a.b, c),
  6: (_b = a())[b] = __pow(_b[b], c),
  7: a[_c = b()] = __pow(a[_c], c),
  8: (_d = a())[_e = b()] = __pow(_d[_e], c),
  9: a[0] = __pow(a[0], b),
  10: a[false] = __pow(a[false], b),
  11: a[null] = __pow(a[null], b),
  12: a[void 0] = __pow(a[void 0], b),
  13: a[123n] = __pow(a[123n], b),
  14: a[this] = __pow(a[this], b),
  15: a[_f = /x/] = __pow(a[_f], b),
  16: a[_g = {}] = __pow(a[_g], b),
  17: a[_h = []] = __pow(a[_h], b),
  18: a[_i = () => {
  }] = __pow(a[_i], b),
  19: a[_j = function() {
  }] = __pow(a[_j], b)
};
`,
		},
	})
}

func TestLowerPrivateFieldAssignments2015NoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#x
					unary() {
						this.#x++
						this.#x--
						++this.#x
						--this.#x
					}
					binary() {
						this.#x = 1
						this.#x += 1
						this.#x -= 1
						this.#x *= 1
						this.#x /= 1
						this.#x %= 1
						this.#x **= 1
						this.#x <<= 1
						this.#x >>= 1
						this.#x >>>= 1
						this.#x &= 1
						this.#x |= 1
						this.#x ^= 1
						this.#x &&= 1
						this.#x ||= 1
						this.#x ??= 1
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ES2015,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `var _x;
class Foo {
  constructor() {
    _x.set(this, void 0);
  }
  unary() {
    var _a, _b;
    __privateSet(this, _x, (_a = +__privateGet(this, _x)) + 1), _a;
    __privateSet(this, _x, (_b = +__privateGet(this, _x)) - 1), _b;
    __privateSet(this, _x, +__privateGet(this, _x) + 1);
    __privateSet(this, _x, +__privateGet(this, _x) - 1);
  }
  binary() {
    var _a;
    __privateSet(this, _x, 1);
    __privateSet(this, _x, __privateGet(this, _x) + 1);
    __privateSet(this, _x, __privateGet(this, _x) - 1);
    __privateSet(this, _x, __privateGet(this, _x) * 1);
    __privateSet(this, _x, __privateGet(this, _x) / 1);
    __privateSet(this, _x, __privateGet(this, _x) % 1);
    __privateSet(this, _x, __pow(__privateGet(this, _x), 1));
    __privateSet(this, _x, __privateGet(this, _x) << 1);
    __privateSet(this, _x, __privateGet(this, _x) >> 1);
    __privateSet(this, _x, __privateGet(this, _x) >>> 1);
    __privateSet(this, _x, __privateGet(this, _x) & 1);
    __privateSet(this, _x, __privateGet(this, _x) | 1);
    __privateSet(this, _x, __privateGet(this, _x) ^ 1);
    __privateGet(this, _x) && __privateSet(this, _x, 1);
    __privateGet(this, _x) || __privateSet(this, _x, 1);
    (_a = __privateGet(this, _x)) != null ? _a : __privateSet(this, _x, 1);
  }
}
_x = new WeakMap();
`,
		},
	})
}

func TestLowerPrivateFieldAssignments2019NoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#x
					unary() {
						this.#x++
						this.#x--
						++this.#x
						--this.#x
					}
					binary() {
						this.#x = 1
						this.#x += 1
						this.#x -= 1
						this.#x *= 1
						this.#x /= 1
						this.#x %= 1
						this.#x **= 1
						this.#x <<= 1
						this.#x >>= 1
						this.#x >>>= 1
						this.#x &= 1
						this.#x |= 1
						this.#x ^= 1
						this.#x &&= 1
						this.#x ||= 1
						this.#x ??= 1
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ES2019,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `var _x;
class Foo {
  constructor() {
    _x.set(this, void 0);
  }
  unary() {
    var _a, _b;
    __privateSet(this, _x, (_a = +__privateGet(this, _x)) + 1), _a;
    __privateSet(this, _x, (_b = +__privateGet(this, _x)) - 1), _b;
    __privateSet(this, _x, +__privateGet(this, _x) + 1);
    __privateSet(this, _x, +__privateGet(this, _x) - 1);
  }
  binary() {
    var _a;
    __privateSet(this, _x, 1);
    __privateSet(this, _x, __privateGet(this, _x) + 1);
    __privateSet(this, _x, __privateGet(this, _x) - 1);
    __privateSet(this, _x, __privateGet(this, _x) * 1);
    __privateSet(this, _x, __privateGet(this, _x) / 1);
    __privateSet(this, _x, __privateGet(this, _x) % 1);
    __privateSet(this, _x, __privateGet(this, _x) ** 1);
    __privateSet(this, _x, __privateGet(this, _x) << 1);
    __privateSet(this, _x, __privateGet(this, _x) >> 1);
    __privateSet(this, _x, __privateGet(this, _x) >>> 1);
    __privateSet(this, _x, __privateGet(this, _x) & 1);
    __privateSet(this, _x, __privateGet(this, _x) | 1);
    __privateSet(this, _x, __privateGet(this, _x) ^ 1);
    __privateGet(this, _x) && __privateSet(this, _x, 1);
    __privateGet(this, _x) || __privateSet(this, _x, 1);
    (_a = __privateGet(this, _x)) != null ? _a : __privateSet(this, _x, 1);
  }
}
_x = new WeakMap();
`,
		},
	})
}

func TestLowerPrivateFieldAssignments2020NoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#x
					unary() {
						this.#x++
						this.#x--
						++this.#x
						--this.#x
					}
					binary() {
						this.#x = 1
						this.#x += 1
						this.#x -= 1
						this.#x *= 1
						this.#x /= 1
						this.#x %= 1
						this.#x **= 1
						this.#x <<= 1
						this.#x >>= 1
						this.#x >>>= 1
						this.#x &= 1
						this.#x |= 1
						this.#x ^= 1
						this.#x &&= 1
						this.#x ||= 1
						this.#x ??= 1
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ES2020,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `var _x;
class Foo {
  constructor() {
    _x.set(this, void 0);
  }
  unary() {
    var _a, _b;
    __privateSet(this, _x, (_a = +__privateGet(this, _x)) + 1), _a;
    __privateSet(this, _x, (_b = +__privateGet(this, _x)) - 1), _b;
    __privateSet(this, _x, +__privateGet(this, _x) + 1);
    __privateSet(this, _x, +__privateGet(this, _x) - 1);
  }
  binary() {
    __privateSet(this, _x, 1);
    __privateSet(this, _x, __privateGet(this, _x) + 1);
    __privateSet(this, _x, __privateGet(this, _x) - 1);
    __privateSet(this, _x, __privateGet(this, _x) * 1);
    __privateSet(this, _x, __privateGet(this, _x) / 1);
    __privateSet(this, _x, __privateGet(this, _x) % 1);
    __privateSet(this, _x, __privateGet(this, _x) ** 1);
    __privateSet(this, _x, __privateGet(this, _x) << 1);
    __privateSet(this, _x, __privateGet(this, _x) >> 1);
    __privateSet(this, _x, __privateGet(this, _x) >>> 1);
    __privateSet(this, _x, __privateGet(this, _x) & 1);
    __privateSet(this, _x, __privateGet(this, _x) | 1);
    __privateSet(this, _x, __privateGet(this, _x) ^ 1);
    __privateGet(this, _x) && __privateSet(this, _x, 1);
    __privateGet(this, _x) || __privateSet(this, _x, 1);
    __privateGet(this, _x) ?? __privateSet(this, _x, 1);
  }
}
_x = new WeakMap();
`,
		},
	})
}

func TestLowerPrivateFieldAssignmentsNextNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#x
					unary() {
						this.#x++
						this.#x--
						++this.#x
						--this.#x
					}
					binary() {
						this.#x = 1
						this.#x += 1
						this.#x -= 1
						this.#x *= 1
						this.#x /= 1
						this.#x %= 1
						this.#x **= 1
						this.#x <<= 1
						this.#x >>= 1
						this.#x >>>= 1
						this.#x &= 1
						this.#x |= 1
						this.#x ^= 1
						this.#x &&= 1
						this.#x ||= 1
						this.#x ??= 1
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `class Foo {
  #x;
  unary() {
    this.#x++;
    this.#x--;
    ++this.#x;
    --this.#x;
  }
  binary() {
    this.#x = 1;
    this.#x += 1;
    this.#x -= 1;
    this.#x *= 1;
    this.#x /= 1;
    this.#x %= 1;
    this.#x **= 1;
    this.#x <<= 1;
    this.#x >>= 1;
    this.#x >>>= 1;
    this.#x &= 1;
    this.#x |= 1;
    this.#x ^= 1;
    this.#x &&= 1;
    this.#x ||= 1;
    this.#x ??= 1;
  }
}
`,
		},
	})
}

func TestLowerPrivateFieldOptionalChain2019NoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#x
					foo() {
						this?.#x.y
						this?.y.#x
						this.#x?.y
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ES2019,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `var _x;
class Foo {
  constructor() {
    _x.set(this, void 0);
  }
  foo() {
    var _a;
    this == null ? void 0 : __privateGet(this, _x).y;
    this == null ? void 0 : __privateGet(this.y, _x);
    (_a = __privateGet(this, _x)) == null ? void 0 : _a.y;
  }
}
_x = new WeakMap();
`,
		},
	})
}

func TestLowerPrivateFieldOptionalChain2020NoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#x
					foo() {
						this?.#x.y
						this?.y.#x
						this.#x?.y
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ES2020,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `var _x;
class Foo {
  constructor() {
    _x.set(this, void 0);
  }
  foo() {
    this == null ? void 0 : __privateGet(this, _x).y;
    this == null ? void 0 : __privateGet(this.y, _x);
    __privateGet(this, _x)?.y;
  }
}
_x = new WeakMap();
`,
		},
	})
}

func TestLowerPrivateFieldOptionalChainNextNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#x
					foo() {
						this?.#x.y
						this?.y.#x
						this.#x?.y
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `class Foo {
  #x;
  foo() {
    this?.#x.y;
    this?.y.#x;
    this.#x?.y;
  }
}
`,
		},
	})
}

func TestTSLowerPrivateFieldOptionalChain2015NoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class Foo {
					#x
					foo() {
						this?.#x.y
						this?.y.#x
						this.#x?.y
					}
				}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ES2015,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `var _x;
class Foo {
  constructor() {
    _x.set(this, void 0);
  }
  foo() {
    var _a;
    this == null ? void 0 : __privateGet(this, _x).y;
    this == null ? void 0 : __privateGet(this.y, _x);
    (_a = __privateGet(this, _x)) == null ? void 0 : _a.y;
  }
}
_x = new WeakMap();
`,
		},
	})
}

func TestTSLowerPrivateStaticMembers2015NoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class Foo {
					static #x
					static get #y() {}
					static set #y() {}
					static #z() {}
					foo() {
						Foo.#x += 1
						Foo.#y += 1
						Foo.#z()
					}
				}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ES2015,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `var _x, _y, y_get, y_set, _z, z_fn;
class Foo {
  foo() {
    __privateSet(Foo, _x, __privateGet(Foo, _x) + 1);
    __privateSet(Foo, _y, __privateGet(Foo, _y, y_get) + 1, y_set);
    __privateMethod(Foo, _z, z_fn).call(Foo);
  }
}
_x = new WeakMap();
_y = new WeakSet();
y_get = function() {
};
y_set = function() {
};
_z = new WeakSet();
z_fn = function() {
};
_x.set(Foo, void 0);
_y.add(Foo);
_z.add(Foo);
`,
		},
	})
}

func TestTSLowerPrivateFieldAndMethodAvoidNameCollision2015(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class WeakMap {
					#x
				}
				class WeakSet {
					#y() {}
				}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
			Target:     config.ES2015,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.ts
var _x;
class WeakMap2 {
  constructor() {
    _x.set(this, void 0);
  }
}
_x = new WeakMap();
var _y, y_fn;
class WeakSet2 {
  constructor() {
    _y.add(this);
  }
}
_y = new WeakSet();
y_fn = function() {
};
`,
		},
	})
}

func TestLowerPrivateGetterSetter2015(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					get #foo() { return this.foo }
					set #bar(val) { this.bar = val }
					get #prop() { return this.prop }
					set #prop(val) { this.prop = val }
					foo(fn) {
						fn().#foo
						fn().#bar = 1
						fn().#prop
						fn().#prop = 2
					}
					unary(fn) {
						fn().#prop++;
						fn().#prop--;
						++fn().#prop;
						--fn().#prop;
					}
					binary(fn) {
						fn().#prop = 1;
						fn().#prop += 1;
						fn().#prop -= 1;
						fn().#prop *= 1;
						fn().#prop /= 1;
						fn().#prop %= 1;
						fn().#prop **= 1;
						fn().#prop <<= 1;
						fn().#prop >>= 1;
						fn().#prop >>>= 1;
						fn().#prop &= 1;
						fn().#prop |= 1;
						fn().#prop ^= 1;
						fn().#prop &&= 1;
						fn().#prop ||= 1;
						fn().#prop ??= 1;
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
			Target:     config.ES2015,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
var _foo, foo_get, _bar, bar_set, _prop, prop_get, prop_set;
class Foo {
  constructor() {
    _foo.add(this);
    _bar.add(this);
    _prop.add(this);
  }
  foo(fn) {
    __privateGet(fn(), _foo, foo_get);
    __privateSet(fn(), _bar, 1, bar_set);
    __privateGet(fn(), _prop, prop_get);
    __privateSet(fn(), _prop, 2, prop_set);
  }
  unary(fn) {
    var _a, _b, _c, _d, _e, _f;
    __privateSet(_a = fn(), _prop, (_b = +__privateGet(_a, _prop, prop_get)) + 1, prop_set), _b;
    __privateSet(_c = fn(), _prop, (_d = +__privateGet(_c, _prop, prop_get)) - 1, prop_set), _d;
    __privateSet(_e = fn(), _prop, +__privateGet(_e, _prop, prop_get) + 1, prop_set);
    __privateSet(_f = fn(), _prop, +__privateGet(_f, _prop, prop_get) - 1, prop_set);
  }
  binary(fn) {
    var _a, _b, _c, _d, _e, _f, _g, _h, _i, _j, _k, _l, _m, _n, _o, _p;
    __privateSet(fn(), _prop, 1, prop_set);
    __privateSet(_a = fn(), _prop, __privateGet(_a, _prop, prop_get) + 1, prop_set);
    __privateSet(_b = fn(), _prop, __privateGet(_b, _prop, prop_get) - 1, prop_set);
    __privateSet(_c = fn(), _prop, __privateGet(_c, _prop, prop_get) * 1, prop_set);
    __privateSet(_d = fn(), _prop, __privateGet(_d, _prop, prop_get) / 1, prop_set);
    __privateSet(_e = fn(), _prop, __privateGet(_e, _prop, prop_get) % 1, prop_set);
    __privateSet(_f = fn(), _prop, __pow(__privateGet(_f, _prop, prop_get), 1), prop_set);
    __privateSet(_g = fn(), _prop, __privateGet(_g, _prop, prop_get) << 1, prop_set);
    __privateSet(_h = fn(), _prop, __privateGet(_h, _prop, prop_get) >> 1, prop_set);
    __privateSet(_i = fn(), _prop, __privateGet(_i, _prop, prop_get) >>> 1, prop_set);
    __privateSet(_j = fn(), _prop, __privateGet(_j, _prop, prop_get) & 1, prop_set);
    __privateSet(_k = fn(), _prop, __privateGet(_k, _prop, prop_get) | 1, prop_set);
    __privateSet(_l = fn(), _prop, __privateGet(_l, _prop, prop_get) ^ 1, prop_set);
    __privateGet(_m = fn(), _prop, prop_get) && __privateSet(_m, _prop, 1, prop_set);
    __privateGet(_n = fn(), _prop, prop_get) || __privateSet(_n, _prop, 1, prop_set);
    (_p = __privateGet(_o = fn(), _prop, prop_get)) != null ? _p : __privateSet(_o, _prop, 1, prop_set);
  }
}
_foo = new WeakSet();
foo_get = function() {
  return this.foo;
};
_bar = new WeakSet();
bar_set = function(val) {
  this.bar = val;
};
_prop = new WeakSet();
prop_get = function() {
  return this.prop;
};
prop_set = function(val) {
  this.prop = val;
};
`,
		},
	})
}

func TestLowerPrivateGetterSetter2019(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					get #foo() { return this.foo }
					set #bar(val) { this.bar = val }
					get #prop() { return this.prop }
					set #prop(val) { this.prop = val }
					foo(fn) {
						fn().#foo
						fn().#bar = 1
						fn().#prop
						fn().#prop = 2
					}
					unary(fn) {
						fn().#prop++;
						fn().#prop--;
						++fn().#prop;
						--fn().#prop;
					}
					binary(fn) {
						fn().#prop = 1;
						fn().#prop += 1;
						fn().#prop -= 1;
						fn().#prop *= 1;
						fn().#prop /= 1;
						fn().#prop %= 1;
						fn().#prop **= 1;
						fn().#prop <<= 1;
						fn().#prop >>= 1;
						fn().#prop >>>= 1;
						fn().#prop &= 1;
						fn().#prop |= 1;
						fn().#prop ^= 1;
						fn().#prop &&= 1;
						fn().#prop ||= 1;
						fn().#prop ??= 1;
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
			Target:     config.ES2019,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
var _foo, foo_get, _bar, bar_set, _prop, prop_get, prop_set;
class Foo {
  constructor() {
    _foo.add(this);
    _bar.add(this);
    _prop.add(this);
  }
  foo(fn) {
    __privateGet(fn(), _foo, foo_get);
    __privateSet(fn(), _bar, 1, bar_set);
    __privateGet(fn(), _prop, prop_get);
    __privateSet(fn(), _prop, 2, prop_set);
  }
  unary(fn) {
    var _a, _b, _c, _d, _e, _f;
    __privateSet(_a = fn(), _prop, (_b = +__privateGet(_a, _prop, prop_get)) + 1, prop_set), _b;
    __privateSet(_c = fn(), _prop, (_d = +__privateGet(_c, _prop, prop_get)) - 1, prop_set), _d;
    __privateSet(_e = fn(), _prop, +__privateGet(_e, _prop, prop_get) + 1, prop_set);
    __privateSet(_f = fn(), _prop, +__privateGet(_f, _prop, prop_get) - 1, prop_set);
  }
  binary(fn) {
    var _a, _b, _c, _d, _e, _f, _g, _h, _i, _j, _k, _l, _m, _n, _o, _p;
    __privateSet(fn(), _prop, 1, prop_set);
    __privateSet(_a = fn(), _prop, __privateGet(_a, _prop, prop_get) + 1, prop_set);
    __privateSet(_b = fn(), _prop, __privateGet(_b, _prop, prop_get) - 1, prop_set);
    __privateSet(_c = fn(), _prop, __privateGet(_c, _prop, prop_get) * 1, prop_set);
    __privateSet(_d = fn(), _prop, __privateGet(_d, _prop, prop_get) / 1, prop_set);
    __privateSet(_e = fn(), _prop, __privateGet(_e, _prop, prop_get) % 1, prop_set);
    __privateSet(_f = fn(), _prop, __privateGet(_f, _prop, prop_get) ** 1, prop_set);
    __privateSet(_g = fn(), _prop, __privateGet(_g, _prop, prop_get) << 1, prop_set);
    __privateSet(_h = fn(), _prop, __privateGet(_h, _prop, prop_get) >> 1, prop_set);
    __privateSet(_i = fn(), _prop, __privateGet(_i, _prop, prop_get) >>> 1, prop_set);
    __privateSet(_j = fn(), _prop, __privateGet(_j, _prop, prop_get) & 1, prop_set);
    __privateSet(_k = fn(), _prop, __privateGet(_k, _prop, prop_get) | 1, prop_set);
    __privateSet(_l = fn(), _prop, __privateGet(_l, _prop, prop_get) ^ 1, prop_set);
    __privateGet(_m = fn(), _prop, prop_get) && __privateSet(_m, _prop, 1, prop_set);
    __privateGet(_n = fn(), _prop, prop_get) || __privateSet(_n, _prop, 1, prop_set);
    (_p = __privateGet(_o = fn(), _prop, prop_get)) != null ? _p : __privateSet(_o, _prop, 1, prop_set);
  }
}
_foo = new WeakSet();
foo_get = function() {
  return this.foo;
};
_bar = new WeakSet();
bar_set = function(val) {
  this.bar = val;
};
_prop = new WeakSet();
prop_get = function() {
  return this.prop;
};
prop_set = function(val) {
  this.prop = val;
};
`,
		},
	})
}

func TestLowerPrivateGetterSetter2020(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					get #foo() { return this.foo }
					set #bar(val) { this.bar = val }
					get #prop() { return this.prop }
					set #prop(val) { this.prop = val }
					foo(fn) {
						fn().#foo
						fn().#bar = 1
						fn().#prop
						fn().#prop = 2
					}
					unary(fn) {
						fn().#prop++;
						fn().#prop--;
						++fn().#prop;
						--fn().#prop;
					}
					binary(fn) {
						fn().#prop = 1;
						fn().#prop += 1;
						fn().#prop -= 1;
						fn().#prop *= 1;
						fn().#prop /= 1;
						fn().#prop %= 1;
						fn().#prop **= 1;
						fn().#prop <<= 1;
						fn().#prop >>= 1;
						fn().#prop >>>= 1;
						fn().#prop &= 1;
						fn().#prop |= 1;
						fn().#prop ^= 1;
						fn().#prop &&= 1;
						fn().#prop ||= 1;
						fn().#prop ??= 1;
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
			Target:     config.ES2020,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
var _foo, foo_get, _bar, bar_set, _prop, prop_get, prop_set;
class Foo {
  constructor() {
    _foo.add(this);
    _bar.add(this);
    _prop.add(this);
  }
  foo(fn) {
    __privateGet(fn(), _foo, foo_get);
    __privateSet(fn(), _bar, 1, bar_set);
    __privateGet(fn(), _prop, prop_get);
    __privateSet(fn(), _prop, 2, prop_set);
  }
  unary(fn) {
    var _a, _b, _c, _d, _e, _f;
    __privateSet(_a = fn(), _prop, (_b = +__privateGet(_a, _prop, prop_get)) + 1, prop_set), _b;
    __privateSet(_c = fn(), _prop, (_d = +__privateGet(_c, _prop, prop_get)) - 1, prop_set), _d;
    __privateSet(_e = fn(), _prop, +__privateGet(_e, _prop, prop_get) + 1, prop_set);
    __privateSet(_f = fn(), _prop, +__privateGet(_f, _prop, prop_get) - 1, prop_set);
  }
  binary(fn) {
    var _a, _b, _c, _d, _e, _f, _g, _h, _i, _j, _k, _l, _m, _n, _o;
    __privateSet(fn(), _prop, 1, prop_set);
    __privateSet(_a = fn(), _prop, __privateGet(_a, _prop, prop_get) + 1, prop_set);
    __privateSet(_b = fn(), _prop, __privateGet(_b, _prop, prop_get) - 1, prop_set);
    __privateSet(_c = fn(), _prop, __privateGet(_c, _prop, prop_get) * 1, prop_set);
    __privateSet(_d = fn(), _prop, __privateGet(_d, _prop, prop_get) / 1, prop_set);
    __privateSet(_e = fn(), _prop, __privateGet(_e, _prop, prop_get) % 1, prop_set);
    __privateSet(_f = fn(), _prop, __privateGet(_f, _prop, prop_get) ** 1, prop_set);
    __privateSet(_g = fn(), _prop, __privateGet(_g, _prop, prop_get) << 1, prop_set);
    __privateSet(_h = fn(), _prop, __privateGet(_h, _prop, prop_get) >> 1, prop_set);
    __privateSet(_i = fn(), _prop, __privateGet(_i, _prop, prop_get) >>> 1, prop_set);
    __privateSet(_j = fn(), _prop, __privateGet(_j, _prop, prop_get) & 1, prop_set);
    __privateSet(_k = fn(), _prop, __privateGet(_k, _prop, prop_get) | 1, prop_set);
    __privateSet(_l = fn(), _prop, __privateGet(_l, _prop, prop_get) ^ 1, prop_set);
    __privateGet(_m = fn(), _prop, prop_get) && __privateSet(_m, _prop, 1, prop_set);
    __privateGet(_n = fn(), _prop, prop_get) || __privateSet(_n, _prop, 1, prop_set);
    __privateGet(_o = fn(), _prop, prop_get) ?? __privateSet(_o, _prop, 1, prop_set);
  }
}
_foo = new WeakSet();
foo_get = function() {
  return this.foo;
};
_bar = new WeakSet();
bar_set = function(val) {
  this.bar = val;
};
_prop = new WeakSet();
prop_get = function() {
  return this.prop;
};
prop_set = function(val) {
  this.prop = val;
};
`,
		},
	})
}

func TestLowerPrivateGetterSetterNext(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					get #foo() { return this.foo }
					set #bar(val) { this.bar = val }
					get #prop() { return this.prop }
					set #prop(val) { this.prop = val }
					foo(fn) {
						fn().#foo
						fn().#bar = 1
						fn().#prop
						fn().#prop = 2
					}
					unary(fn) {
						fn().#prop++;
						fn().#prop--;
						++fn().#prop;
						--fn().#prop;
					}
					binary(fn) {
						fn().#prop = 1;
						fn().#prop += 1;
						fn().#prop -= 1;
						fn().#prop *= 1;
						fn().#prop /= 1;
						fn().#prop %= 1;
						fn().#prop **= 1;
						fn().#prop <<= 1;
						fn().#prop >>= 1;
						fn().#prop >>>= 1;
						fn().#prop &= 1;
						fn().#prop |= 1;
						fn().#prop ^= 1;
						fn().#prop &&= 1;
						fn().#prop ||= 1;
						fn().#prop ??= 1;
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
class Foo {
  get #foo() {
    return this.foo;
  }
  set #bar(val) {
    this.bar = val;
  }
  get #prop() {
    return this.prop;
  }
  set #prop(val) {
    this.prop = val;
  }
  foo(fn) {
    fn().#foo;
    fn().#bar = 1;
    fn().#prop;
    fn().#prop = 2;
  }
  unary(fn) {
    fn().#prop++;
    fn().#prop--;
    ++fn().#prop;
    --fn().#prop;
  }
  binary(fn) {
    fn().#prop = 1;
    fn().#prop += 1;
    fn().#prop -= 1;
    fn().#prop *= 1;
    fn().#prop /= 1;
    fn().#prop %= 1;
    fn().#prop **= 1;
    fn().#prop <<= 1;
    fn().#prop >>= 1;
    fn().#prop >>>= 1;
    fn().#prop &= 1;
    fn().#prop |= 1;
    fn().#prop ^= 1;
    fn().#prop &&= 1;
    fn().#prop ||= 1;
    fn().#prop ??= 1;
  }
}
`,
		},
	})
}

func TestLowerPrivateMethod2019(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#field
					#method() {}
					baseline() {
						a().foo
						b().foo(x)
						c()?.foo(x)
						d().foo?.(x)
						e()?.foo?.(x)
					}
					privateField() {
						a().#field
						b().#field(x)
						c()?.#field(x)
						d().#field?.(x)
						e()?.#field?.(x)
						f()?.foo.#field(x).bar()
					}
					privateMethod() {
						a().#method
						b().#method(x)
						c()?.#method(x)
						d().#method?.(x)
						e()?.#method?.(x)
						f()?.foo.#method(x).bar()
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
			Target:     config.ES2019,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
var _field, _method, method_fn;
class Foo {
  constructor() {
    _field.set(this, void 0);
    _method.add(this);
  }
  baseline() {
    var _a, _b, _c, _d, _e;
    a().foo;
    b().foo(x);
    (_a = c()) == null ? void 0 : _a.foo(x);
    (_c = (_b = d()).foo) == null ? void 0 : _c.call(_b, x);
    (_e = (_d = e()) == null ? void 0 : _d.foo) == null ? void 0 : _e.call(_d, x);
  }
  privateField() {
    var _a, _b, _c, _d, _e, _f, _g, _h;
    __privateGet(a(), _field);
    __privateGet(_a = b(), _field).call(_a, x);
    (_b = c()) == null ? void 0 : __privateGet(_b, _field).call(_b, x);
    (_d = __privateGet(_c = d(), _field)) == null ? void 0 : _d.call(_c, x);
    (_f = (_e = e()) == null ? void 0 : __privateGet(_e, _field)) == null ? void 0 : _f.call(_e, x);
    (_g = f()) == null ? void 0 : __privateGet(_h = _g.foo, _field).call(_h, x).bar();
  }
  privateMethod() {
    var _a, _b, _c, _d, _e, _f, _g, _h;
    __privateMethod(a(), _method, method_fn);
    __privateMethod(_a = b(), _method, method_fn).call(_a, x);
    (_b = c()) == null ? void 0 : __privateMethod(_b, _method, method_fn).call(_b, x);
    (_d = __privateMethod(_c = d(), _method, method_fn)) == null ? void 0 : _d.call(_c, x);
    (_f = (_e = e()) == null ? void 0 : __privateMethod(_e, _method, method_fn)) == null ? void 0 : _f.call(_e, x);
    (_g = f()) == null ? void 0 : __privateMethod(_h = _g.foo, _method, method_fn).call(_h, x).bar();
  }
}
_field = new WeakMap();
_method = new WeakSet();
method_fn = function() {
};
`,
		},
	})
}

func TestLowerPrivateMethod2020(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#field
					#method() {}
					baseline() {
						a().foo
						b().foo(x)
						c()?.foo(x)
						d().foo?.(x)
						e()?.foo?.(x)
					}
					privateField() {
						a().#field
						b().#field(x)
						c()?.#field(x)
						d().#field?.(x)
						e()?.#field?.(x)
						f()?.foo.#field(x).bar()
					}
					privateMethod() {
						a().#method
						b().#method(x)
						c()?.#method(x)
						d().#method?.(x)
						e()?.#method?.(x)
						f()?.foo.#method(x).bar()
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
			Target:     config.ES2020,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
var _field, _method, method_fn;
class Foo {
  constructor() {
    _field.set(this, void 0);
    _method.add(this);
  }
  baseline() {
    a().foo;
    b().foo(x);
    c()?.foo(x);
    d().foo?.(x);
    e()?.foo?.(x);
  }
  privateField() {
    var _a, _b, _c, _d, _e, _f, _g;
    __privateGet(a(), _field);
    __privateGet(_a = b(), _field).call(_a, x);
    (_b = c()) == null ? void 0 : __privateGet(_b, _field).call(_b, x);
    (_d = __privateGet(_c = d(), _field)) == null ? void 0 : _d.call(_c, x);
    ((_e = e()) == null ? void 0 : __privateGet(_e, _field))?.(x);
    (_f = f()) == null ? void 0 : __privateGet(_g = _f.foo, _field).call(_g, x).bar();
  }
  privateMethod() {
    var _a, _b, _c, _d, _e, _f, _g;
    __privateMethod(a(), _method, method_fn);
    __privateMethod(_a = b(), _method, method_fn).call(_a, x);
    (_b = c()) == null ? void 0 : __privateMethod(_b, _method, method_fn).call(_b, x);
    (_d = __privateMethod(_c = d(), _method, method_fn)) == null ? void 0 : _d.call(_c, x);
    ((_e = e()) == null ? void 0 : __privateMethod(_e, _method, method_fn))?.(x);
    (_f = f()) == null ? void 0 : __privateMethod(_g = _f.foo, _method, method_fn).call(_g, x).bar();
  }
}
_field = new WeakMap();
_method = new WeakSet();
method_fn = function() {
};
`,
		},
	})
}

func TestLowerPrivateMethodNext(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#field
					#method() {}
					baseline() {
						a().foo
						b().foo(x)
						c()?.foo(x)
						d().foo?.(x)
						e()?.foo?.(x)
					}
					privateField() {
						a().#field
						b().#field(x)
						c()?.#field(x)
						d().#field?.(x)
						e()?.#field?.(x)
						f()?.foo.#field(x).bar()
					}
					privateMethod() {
						a().#method
						b().#method(x)
						c()?.#method(x)
						d().#method?.(x)
						e()?.#method?.(x)
						f()?.foo.#method(x).bar()
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
class Foo {
  #field;
  #method() {
  }
  baseline() {
    a().foo;
    b().foo(x);
    c()?.foo(x);
    d().foo?.(x);
    e()?.foo?.(x);
  }
  privateField() {
    a().#field;
    b().#field(x);
    c()?.#field(x);
    d().#field?.(x);
    e()?.#field?.(x);
    f()?.foo.#field(x).bar();
  }
  privateMethod() {
    a().#method;
    b().#method(x);
    c()?.#method(x);
    d().#method?.(x);
    e()?.#method?.(x);
    f()?.foo.#method(x).bar();
  }
}
`,
		},
	})
}

func TestLowerPrivateClassExpr2020NoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export let Foo = class {
					#field
					#method() {}
					static #staticField
					static #staticMethod() {}
					foo() {
						this.#field = this.#method()
						Foo.#staticField = Foo.#staticMethod()
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ES2020,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `var _field, _method, method_fn, _a, _staticField, _staticMethod, staticMethod_fn;
export let Foo = (_a = class {
  constructor() {
    _field.set(this, void 0);
    _method.add(this);
  }
  foo() {
    __privateSet(this, _field, __privateMethod(this, _method, method_fn).call(this));
    __privateSet(Foo, _staticField, __privateMethod(Foo, _staticMethod, staticMethod_fn).call(Foo));
  }
}, _field = new WeakMap(), _method = new WeakSet(), method_fn = function() {
}, _staticField = new WeakMap(), _staticMethod = new WeakSet(), staticMethod_fn = function() {
}, _staticField.set(_a, void 0), _staticMethod.add(_a), _a);
`,
		},
	})
}

func TestLowerPrivateMethodWithModifiers2020(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					*#g() {}
					async #a() {}
					async *#ag() {}

					static *#sg() {}
					static async #sa() {}
					static async *#sag() {}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
			Target:     config.ES2020,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
var _g, g_fn, _a, a_fn, _ag, ag_fn, _sg, sg_fn, _sa, sa_fn, _sag, sag_fn;
class Foo {
  constructor() {
    _g.add(this);
    _a.add(this);
    _ag.add(this);
  }
}
_g = new WeakSet();
g_fn = function* () {
};
_a = new WeakSet();
a_fn = async function() {
};
_ag = new WeakSet();
ag_fn = async function* () {
};
_sg = new WeakSet();
sg_fn = function* () {
};
_sa = new WeakSet();
sa_fn = async function() {
};
_sag = new WeakSet();
sag_fn = async function* () {
};
_sg.add(Foo);
_sa.add(Foo);
_sag.add(Foo);
`,
		},
	})
}

func TestLowerAsync2016NoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				async function foo(bar) {
					await bar
					return [this, arguments]
				}
				class Foo {async foo() {}}
				export default [
					foo,
					Foo,
					async function() {},
					async () => {},
					{async foo() {}},
					class {async foo() {}},
					function() {
						return async (bar) => {
							await bar
							return [this, arguments]
						}
					},
				]
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ES2016,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `function foo(bar) {
  return __async(this, arguments, function* () {
    yield bar;
    return [this, arguments];
  });
}
class Foo {
  foo() {
    return __async(this, [], function* () {
    });
  }
}
export default [
  foo,
  Foo,
  function() {
    return __async(this, [], function* () {
    });
  },
  () => __async(this, [], function* () {
  }),
  {foo() {
    return __async(this, [], function* () {
    });
  }},
  class {
    foo() {
      return __async(this, [], function* () {
      });
    }
  },
  function() {
    return (bar) => __async(this, arguments, function* () {
      yield bar;
      return [this, arguments];
    });
  }
];
`,
		},
	})
}

func TestLowerAsync2017NoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				async function foo(bar) {
					await bar
					return arguments
				}
				class Foo {async foo() {}}
				export default [
					foo,
					Foo,
					async function() {},
					async () => {},
					{async foo() {}},
					class {async foo() {}},
					function() {
						return async (bar) => {
							await bar
							return [this, arguments]
						}
					},
				]
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ES2017,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `async function foo(bar) {
  await bar;
  return arguments;
}
class Foo {
  async foo() {
  }
}
export default [
  foo,
  Foo,
  async function() {
  },
  async () => {
  },
  {async foo() {
  }},
  class {
    async foo() {
    }
  },
  function() {
    return async (bar) => {
      await bar;
      return [this, arguments];
    };
  }
];
`,
		},
	})
}

func TestLowerAsyncThis2016CommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				exports.foo = async () => this
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
			Target:     config.ES2016,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
var require_entry = __commonJS((exports) => {
  exports.foo = () => __async(exports, [], function* () {
    return exports;
  });
});
export default require_entry();
`,
		},
	})
}

func TestLowerAsyncThis2016ES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export let foo = async () => this
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
			Target:     config.ES2016,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /entry.js
let foo = () => __async(void 0, [], function* () {
  return void 0;
});
export {
  foo
};
`,
		},
	})
}

func TestLowerClassFieldStrict2020NoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#foo = 123
					#bar
					foo = 123
					bar
					static #s_foo = 123
					static #s_bar
					static s_foo = 123
					static s_bar
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ES2020,
			Strict: config.StrictOptions{
				ClassFields: true,
			},
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `var _foo, _bar, _s_foo, _s_bar;
class Foo {
  constructor() {
    _foo.set(this, 123);
    _bar.set(this, void 0);
    __publicField(this, "foo", 123);
    __publicField(this, "bar", void 0);
  }
}
_foo = new WeakMap();
_bar = new WeakMap();
_s_foo = new WeakMap();
_s_bar = new WeakMap();
_s_foo.set(Foo, 123);
_s_bar.set(Foo, void 0);
__publicField(Foo, "s_foo", 123);
__publicField(Foo, "s_bar", void 0);
`,
		},
	})
}

func TestLowerClassField2020NoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#foo = 123
					#bar
					foo = 123
					bar
					static #s_foo = 123
					static #s_bar
					static s_foo = 123
					static s_bar
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ES2020,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `var _foo, _bar, _s_foo, _s_bar;
class Foo {
  constructor() {
    _foo.set(this, 123);
    _bar.set(this, void 0);
    this.foo = 123;
    this.bar = void 0;
  }
}
_foo = new WeakMap();
_bar = new WeakMap();
_s_foo = new WeakMap();
_s_bar = new WeakMap();
_s_foo.set(Foo, 123);
_s_bar.set(Foo, void 0);
Foo.s_foo = 123;
Foo.s_bar = void 0;
`,
		},
	})
}

func TestLowerClassFieldStrictNextNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#foo = 123
					#bar
					foo = 123
					bar
					static #s_foo = 123
					static #s_bar
					static s_foo = 123
					static s_bar
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ESNext,
			Strict: config.StrictOptions{
				ClassFields: true,
			},
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `class Foo {
  #foo = 123;
  #bar;
  foo = 123;
  bar;
  static #s_foo = 123;
  static #s_bar;
  static s_foo = 123;
  static s_bar;
}
`,
		},
	})
}

func TestLowerClassFieldNextNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#foo = 123
					#bar
					foo = 123
					bar
					static #s_foo = 123
					static #s_bar
					static s_foo = 123
					static s_bar
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ESNext,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `class Foo {
  #foo = 123;
  #bar;
  foo = 123;
  bar;
  static #s_foo = 123;
  static #s_bar;
  static s_foo = 123;
  static s_bar;
}
`,
		},
	})
}

func TestTSLowerClassFieldStrict2020NoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class Foo {
					#foo = 123
					#bar
					foo = 123
					bar
					static #s_foo = 123
					static #s_bar
					static s_foo = 123
					static s_bar
				}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ES2020,
			Strict: config.StrictOptions{
				ClassFields: true,
			},
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `var _foo, _bar, _s_foo, _s_bar;
class Foo {
  constructor() {
    _foo.set(this, 123);
    _bar.set(this, void 0);
    __publicField(this, "foo", 123);
    __publicField(this, "bar", void 0);
  }
}
_foo = new WeakMap();
_bar = new WeakMap();
_s_foo = new WeakMap();
_s_bar = new WeakMap();
_s_foo.set(Foo, 123);
_s_bar.set(Foo, void 0);
__publicField(Foo, "s_foo", 123);
__publicField(Foo, "s_bar", void 0);
`,
		},
	})
}

func TestTSLowerClassField2020NoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class Foo {
					#foo = 123
					#bar
					foo = 123
					bar
					static #s_foo = 123
					static #s_bar
					static s_foo = 123
					static s_bar
				}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ES2020,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `var _foo, _bar, _s_foo, _s_bar;
class Foo {
  constructor() {
    _foo.set(this, 123);
    _bar.set(this, void 0);
    this.foo = 123;
  }
}
_foo = new WeakMap();
_bar = new WeakMap();
_s_foo = new WeakMap();
_s_bar = new WeakMap();
_s_foo.set(Foo, 123);
_s_bar.set(Foo, void 0);
Foo.s_foo = 123;
`,
		},
	})
}

func TestTSLowerClassPrivateFieldStrictNextNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class Foo {
					#foo = 123
					#bar
					foo = 123
					bar
					static #s_foo = 123
					static #s_bar
					static s_foo = 123
					static s_bar
				}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ESNext,
			Strict: config.StrictOptions{
				ClassFields: true,
			},
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `class Foo {
  #foo = 123;
  #bar;
  foo = 123;
  bar;
  static #s_foo = 123;
  static #s_bar;
  static s_foo = 123;
  static s_bar;
}
`,
		},
	})
}

func TestTSLowerClassPrivateFieldNextNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class Foo {
					#foo = 123
					#bar
					foo = 123
					bar
					static #s_foo = 123
					static #s_bar
					static s_foo = 123
					static s_bar
				}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ESNext,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `class Foo {
  constructor() {
    this.#foo = 123;
    this.foo = 123;
  }
  #foo;
  #bar;
  static #s_foo = 123;
  static #s_bar;
}
Foo.s_foo = 123;
`,
		},
	})
}

func TestLowerClassFieldStrictTsconfigJson2020(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import loose from './loose'
				import strict from './strict'
				console.log(loose, strict)
			`,
			"/loose/index.js": `
				export default class {
					foo
				}
			`,
			"/loose/tsconfig.json": `
				{
					"compilerOptions": {
						"useDefineForClassFields": false
					}
				}
			`,
			"/strict/index.js": `
				export default class {
					foo
				}
			`,
			"/strict/tsconfig.json": `
				{
					"compilerOptions": {
						"useDefineForClassFields": true
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
			Target:     config.ES2020,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `// /loose/index.js
class loose_default {
  constructor() {
    this.foo = void 0;
  }
}

// /strict/index.js
class strict_default {
  constructor() {
    __publicField(this, "foo", void 0);
  }
}

// /entry.js
console.log(loose_default, strict_default);
`,
		},
	})
}

func TestTSLowerObjectRest2017NoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				const { ...local_const } = {};
				let { ...local_let } = {};
				var { ...local_var } = {};
				let arrow_fn = ({ ...x }) => { };
				let fn_expr = function ({ ...x } = default_value) {};
				let class_expr = class { method(x, ...[y, { ...z }]) {} };

				function fn_stmt({ a = b(), ...x }, { c = d(), ...y }) {}
				class class_stmt { method({ ...x }) {} }
				namespace ns { export let { ...x } = {} }
				try { } catch ({ ...catch_clause }) {}

				for (const { ...for_in_const } in { abc }) {}
				for (let { ...for_in_let } in { abc }) {}
				for (var { ...for_in_var } in { abc }) ;
				for (const { ...for_of_const } of [{}]) ;
				for (let { ...for_of_let } of [{}]) x()
				for (var { ...for_of_var } of [{}]) x()
				for (const { ...for_const } = {}; x; x = null) {}
				for (let { ...for_let } = {}; x; x = null) {}
				for (var { ...for_var } = {}; x; x = null) {}
				for ({ ...x } in { abc }) {}
				for ({ ...x } of [{}]) {}
				for ({ ...x } = {}; x; x = null) {}

				({ ...assign } = {});
				({ obj_method({ ...x }) {} });
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ES2017,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `var _o, _p;
const local_const = __rest({}, []);
let local_let = __rest({}, []);
var local_var = __rest({}, []);
let arrow_fn = (_a) => {
  var x2 = __rest(_a, []);
};
let fn_expr = function(_b = default_value) {
  var x2 = __rest(_b, []);
};
let class_expr = class {
  method(x2, ..._c) {
    var [y, _d] = _c, z = __rest(_d, []);
  }
};
function fn_stmt(_e, _f) {
  var {a = b()} = _e, x2 = __rest(_e, ["a"]);
  var {c = d()} = _f, y = __rest(_f, ["c"]);
}
class class_stmt {
  method(_g) {
    var x2 = __rest(_g, []);
  }
}
var ns;
(function(ns2) {
  ns2.x = __rest({}, []);
})(ns || (ns = {}));
try {
} catch (_h) {
  let catch_clause = __rest(_h, []);
}
for (const _i in {abc}) {
  const for_in_const = __rest(_i, []);
}
for (let _j in {abc}) {
  let for_in_let = __rest(_j, []);
}
for (var _k in {abc}) {
  var for_in_var = __rest(_k, []);
  ;
}
for (const _l of [{}]) {
  const for_of_const = __rest(_l, []);
  ;
}
for (let _m of [{}]) {
  let for_of_let = __rest(_m, []);
  x();
}
for (var _n of [{}]) {
  var for_of_var = __rest(_n, []);
  x();
}
for (const for_const = __rest({}, []); x; x = null) {
}
for (let for_let = __rest({}, []); x; x = null) {
}
for (var for_var = __rest({}, []); x; x = null) {
}
for (_o in {abc}) {
  x = __rest(_o, []);
}
for (_p of [{}]) {
  x = __rest(_p, []);
}
for (x = __rest({}, []); x; x = null) {
}
assign = __rest({}, []);
({obj_method(_q) {
  var x2 = __rest(_q, []);
}});
`,
		},
	})
}

func TestTSLowerObjectRest2018NoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				const { ...local_const } = {};
				let { ...local_let } = {};
				var { ...local_var } = {};
				let arrow_fn = ({ ...x }) => { };
				let fn_expr = function ({ ...x } = default_value) {};
				let class_expr = class { method(x, ...[y, { ...z }]) {} };

				function fn_stmt({ a = b(), ...x }, { c = d(), ...y }) {}
				class class_stmt { method({ ...x }) {} }
				namespace ns { export let { ...x } = {} }
				try { } catch ({ ...catch_clause }) {}

				for (const { ...for_in_const } in { abc }) {}
				for (let { ...for_in_let } in { abc }) {}
				for (var { ...for_in_var } in { abc }) ;
				for (const { ...for_of_const } of [{}]) ;
				for (let { ...for_of_let } of [{}]) x()
				for (var { ...for_of_var } of [{}]) x()
				for (const { ...for_const } = {}; x; x = null) {}
				for (let { ...for_let } = {}; x; x = null) {}
				for (var { ...for_var } = {}; x; x = null) {}
				for ({ ...x } in { abc }) {}
				for ({ ...x } of [{}]) {}
				for ({ ...x } = {}; x; x = null) {}

				({ ...assign } = {});
				({ obj_method({ ...x }) {} });
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     config.ES2018,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `const {...local_const} = {};
let {...local_let} = {};
var {...local_var} = {};
let arrow_fn = ({...x2}) => {
};
let fn_expr = function({...x2} = default_value) {
};
let class_expr = class {
  method(x2, ...[y, {...z}]) {
  }
};
function fn_stmt({a = b(), ...x2}, {c = d(), ...y}) {
}
class class_stmt {
  method({...x2}) {
  }
}
var ns;
(function(ns2) {
  ({...ns2.x} = {});
})(ns || (ns = {}));
try {
} catch ({...catch_clause}) {
}
for (const {...for_in_const} in {abc}) {
}
for (let {...for_in_let} in {abc}) {
}
for (var {...for_in_var} in {abc})
  ;
for (const {...for_of_const} of [{}])
  ;
for (let {...for_of_let} of [{}])
  x();
for (var {...for_of_var} of [{}])
  x();
for (const {...for_const} = {}; x; x = null) {
}
for (let {...for_let} = {}; x; x = null) {
}
for (var {...for_var} = {}; x; x = null) {
}
for ({...x} in {abc}) {
}
for ({...x} of [{}]) {
}
for ({...x} = {}; x; x = null) {
}
({...assign} = {});
({obj_method({...x2}) {
}});
`,
		},
	})
}
