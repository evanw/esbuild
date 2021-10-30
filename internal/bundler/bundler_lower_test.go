// This file contains tests for "lowering" syntax, which means converting it to
// older JavaScript. For example, "a ** b" becomes a call to "Math.pow(a, b)"
// when lowered. Which syntax is lowered is determined by the language target.

package bundler

import (
	"testing"

	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
)

var lower_suite = suite{
	name: "lower",
}

func TestLowerOptionalCatchNameCollisionNoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				try {}
				catch { var e, e2 }
				var e3
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			UnsupportedJSFeatures: es(2018),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerObjectSpreadNoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			UnsupportedJSFeatures: es(2017),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerExponentiationOperatorNoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			UnsupportedJSFeatures: es(2015),
			AbsOutputFile:         "/out.js",
		},
		expectedScanLog: `entry.js: error: Big integer literals are not available in the configured target environment
`,
	})
}

func TestLowerPrivateFieldAssignments2015NoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			UnsupportedJSFeatures: es(2015),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerPrivateFieldAssignments2019NoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			UnsupportedJSFeatures: es(2019),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerPrivateFieldAssignments2020NoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			UnsupportedJSFeatures: es(2020),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerPrivateFieldAssignmentsNextNoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			AbsOutputFile: "/out.js",
		},
	})
}

func TestLowerPrivateFieldOptionalChain2019NoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			UnsupportedJSFeatures: es(2019),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerPrivateFieldOptionalChain2020NoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			UnsupportedJSFeatures: es(2020),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerPrivateFieldOptionalChainNextNoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSLowerPrivateFieldOptionalChain2015NoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			UnsupportedJSFeatures: es(2015),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestTSLowerPrivateStaticMembers2015NoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class Foo {
					static #x
					static get #y() {}
					static set #y(x) {}
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
		options: config.Options{
			UnsupportedJSFeatures: es(2015),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestTSLowerPrivateFieldAndMethodAvoidNameCollision2015(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			Mode:                  config.ModeBundle,
			UnsupportedJSFeatures: es(2015),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerPrivateGetterSetter2015(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			Mode:                  config.ModeBundle,
			UnsupportedJSFeatures: es(2015),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerPrivateGetterSetter2019(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			Mode:                  config.ModeBundle,
			UnsupportedJSFeatures: es(2019),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerPrivateGetterSetter2020(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			Mode:                  config.ModeBundle,
			UnsupportedJSFeatures: es(2020),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerPrivateGetterSetterNext(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestLowerPrivateMethod2019(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			Mode:                  config.ModeBundle,
			UnsupportedJSFeatures: es(2019),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerPrivateMethod2020(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			Mode:                  config.ModeBundle,
			UnsupportedJSFeatures: es(2020),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerPrivateMethodNext(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestLowerPrivateClassExpr2020NoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			UnsupportedJSFeatures: es(2020),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerPrivateMethodWithModifiers2020(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			Mode:                  config.ModeBundle,
			UnsupportedJSFeatures: es(2020),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerAsync2016NoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			UnsupportedJSFeatures: es(2016),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerAsync2017NoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			UnsupportedJSFeatures: es(2017),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerAsyncThis2016CommonJS(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				exports.foo = async () => this
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:                  config.ModeBundle,
			UnsupportedJSFeatures: es(2016),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerAsyncThis2016ES6(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export {bar} from "./other"
				export let foo = async () => this
			`,
			"/other.js": `
				export let bar = async () => {}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:                  config.ModeBundle,
			UnsupportedJSFeatures: es(2016),
			AbsOutputFile:         "/out.js",
		},
		expectedScanLog: `entry.js: warning: Top-level "this" will be replaced with undefined since this file is an ECMAScript module
entry.js: note: This file is considered an ECMAScript module because of the "export" keyword here
`,
	})
}

func TestLowerAsyncES5(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import './fn-stmt'
				import './fn-expr'
				import './arrow-1'
				import './arrow-2'
				import './export-def-1'
				import './export-def-2'
				import './obj-method'
			`,
			"/fn-stmt.js":      `async function foo() {}`,
			"/fn-expr.js":      `(async function() {})`,
			"/arrow-1.js":      `(async () => {})`,
			"/arrow-2.js":      `(async x => {})`,
			"/export-def-1.js": `export default async function foo() {}`,
			"/export-def-2.js": `export default async function() {}`,
			"/obj-method.js":   `({async foo() {}})`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:                  config.ModeBundle,
			UnsupportedJSFeatures: es(5),
			AbsOutputFile:         "/out.js",
		},
		expectedScanLog: `arrow-1.js: error: Transforming async functions to the configured target environment is not supported yet
arrow-2.js: error: Transforming async functions to the configured target environment is not supported yet
export-def-1.js: error: Transforming async functions to the configured target environment is not supported yet
export-def-2.js: error: Transforming async functions to the configured target environment is not supported yet
fn-expr.js: error: Transforming async functions to the configured target environment is not supported yet
fn-stmt.js: error: Transforming async functions to the configured target environment is not supported yet
obj-method.js: error: Transforming async functions to the configured target environment is not supported yet
obj-method.js: error: Transforming object literal extensions to the configured target environment is not supported yet
`,
	})
}

func TestLowerAsyncSuperES2017NoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Derived extends Base {
					async test(key) {
						return [
							await super.foo,
							await super[key],
							await ([super.foo] = [0]),
							await ([super[key]] = [0]),

							await (super.foo = 1),
							await (super[key] = 1),
							await (super.foo += 2),
							await (super[key] += 2),

							await ++super.foo,
							await ++super[key],
							await super.foo++,
							await super[key]++,

							await super.foo.name,
							await super[key].name,
							await super.foo?.name,
							await super[key]?.name,

							await super.foo(1, 2),
							await super[key](1, 2),
							await super.foo?.(1, 2),
							await super[key]?.(1, 2),

							await (() => super.foo)(),
							await (() => super[key])(),
							await (() => super.foo())(),
							await (() => super[key]())(),
						]
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			UnsupportedJSFeatures: es(2017),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerAsyncSuperES2016NoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Derived extends Base {
					async test(key) {
						return [
							await super.foo,
							await super[key],
							await ([super.foo] = [0]),
							await ([super[key]] = [0]),

							await (super.foo = 1),
							await (super[key] = 1),
							await (super.foo += 2),
							await (super[key] += 2),

							await ++super.foo,
							await ++super[key],
							await super.foo++,
							await super[key]++,

							await super.foo.name,
							await super[key].name,
							await super.foo?.name,
							await super[key]?.name,

							await super.foo(1, 2),
							await super[key](1, 2),
							await super.foo?.(1, 2),
							await super[key]?.(1, 2),

							await (() => super.foo)(),
							await (() => super[key])(),
							await (() => super.foo())(),
							await (() => super[key]())(),
						]
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			UnsupportedJSFeatures: es(2016),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerClassField2020NoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			UnsupportedJSFeatures: es(2020),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerClassFieldNextNoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			AbsOutputFile: "/out.js",
		},
	})
}

func TestTSLowerClassField2020NoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			UnsupportedJSFeatures: es(2020),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestTSLowerClassPrivateFieldNextNoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			AbsOutputFile: "/out.js",
		},
	})
}

func TestLowerClassFieldStrictTsconfigJson2020(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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
		options: config.Options{
			Mode:                  config.ModeBundle,
			UnsupportedJSFeatures: es(2020),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestTSLowerClassFieldStrictTsconfigJson2020(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import loose from './loose'
				import strict from './strict'
				console.log(loose, strict)
			`,
			"/loose/index.ts": `
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
			"/strict/index.ts": `
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
		options: config.Options{
			Mode:                  config.ModeBundle,
			UnsupportedJSFeatures: es(2020),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestTSLowerObjectRest2017NoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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

				// Check for used return values
				({ ...x } = x);
				for ({ ...x } = x; 0; ) ;
				console.log({ ...x } = x);
				console.log({ x, ...xx } = { x });
				console.log({ x: { ...xx } } = { x });
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			UnsupportedJSFeatures: es(2017),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestTSLowerObjectRest2018NoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
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

				// Check for used return values
				({ ...x } = x);
				for ({ ...x } = x; 0; ) ;
				console.log({ ...x } = x);
				console.log({ x, ...xx } = { x });
				console.log({ x: { ...xx } } = { x });
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			UnsupportedJSFeatures: es(2018),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestClassSuperThisIssue242NoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				export class A {}

				export class B extends A {
					#e: string
					constructor(c: { d: any }) {
						super()
						this.#e = c.d ?? 'test'
					}
					f() {
						return this.#e
					}
				}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		options: config.Options{
			UnsupportedJSFeatures: es(2019),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerExportStarAsNameCollisionNoBundle(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export * as ns from 'path'
				let ns = 123
				export {ns as sn}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			UnsupportedJSFeatures: es(2019),
			AbsOutputFile:         "/out.js",
		},
	})
}

func TestLowerExportStarAsNameCollision(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import * as test from './nested'
				console.log(test.foo, test.oof)
				export * as ns from 'path1'
				let ns = 123
				export {ns as sn}
			`,
			"/nested.js": `
				export * as foo from 'path2'
				let foo = 123
				export {foo as oof}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:                  config.ModeBundle,
			UnsupportedJSFeatures: es(2019),
			AbsOutputFile:         "/out.js",
			ExternalModules: config.ExternalModules{
				NodeModules: map[string]bool{
					"path1": true,
					"path2": true,
				},
			},
		},
	})
}

func TestLowerStrictModeSyntax(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import './for-in'
			`,
			"/for-in.js": `
				if (test)
					for (var a = b in {}) ;
				for (var x = y in {}) ;
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestLowerForbidStrictModeSyntax(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import './with'
				import './delete-1'
				import './delete-2'
				import './delete-3'
			`,
			"/with.js": `
				with (x) y
			`,
			"/delete-1.js": `
				delete x
			`,
			"/delete-2.js": `
				delete (y)
			`,
			"/delete-3.js": `
				delete (1 ? z : z)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			OutputFormat:  config.FormatESModule,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `delete-1.js: error: Delete of a bare identifier cannot be used with the "esm" output format due to strict mode
delete-2.js: error: Delete of a bare identifier cannot be used with the "esm" output format due to strict mode
with.js: error: With statements cannot be used with the "esm" output format due to strict mode
`,
	})
}

func TestLowerPrivateClassFieldOrder(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#foo = 123 // This must be set before "bar" is initialized
					bar = this.#foo
				}
				console.log(new Foo().bar === 123)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:                  config.ModePassThrough,
			AbsOutputFile:         "/out.js",
			UnsupportedJSFeatures: compat.ClassPrivateField,
		},
	})
}

func TestLowerPrivateClassMethodOrder(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					bar = this.#foo()
					#foo() { return 123 } // This must be set before "bar" is initialized
				}
				console.log(new Foo().bar === 123)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:                  config.ModePassThrough,
			AbsOutputFile:         "/out.js",
			UnsupportedJSFeatures: compat.ClassPrivateMethod,
		},
	})
}

func TestLowerPrivateClassAccessorOrder(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					bar = this.#foo
					get #foo() { return 123 } // This must be set before "bar" is initialized
				}
				console.log(new Foo().bar === 123)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:                  config.ModePassThrough,
			AbsOutputFile:         "/out.js",
			UnsupportedJSFeatures: compat.ClassPrivateAccessor,
		},
	})
}

func TestLowerPrivateClassStaticFieldOrder(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					static #foo = 123 // This must be set before "bar" is initialized
					static bar = Foo.#foo
				}
				console.log(Foo.bar === 123)

				class FooThis {
					static #foo = 123 // This must be set before "bar" is initialized
					static bar = this.#foo
				}
				console.log(FooThis.bar === 123)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:                  config.ModePassThrough,
			AbsOutputFile:         "/out.js",
			UnsupportedJSFeatures: compat.ClassPrivateStaticField,
		},
	})
}

func TestLowerPrivateClassStaticMethodOrder(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					static bar = Foo.#foo()
					static #foo() { return 123 } // This must be set before "bar" is initialized
				}
				console.log(Foo.bar === 123)

				class FooThis {
					static bar = this.#foo()
					static #foo() { return 123 } // This must be set before "bar" is initialized
				}
				console.log(FooThis.bar === 123)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:                  config.ModePassThrough,
			AbsOutputFile:         "/out.js",
			UnsupportedJSFeatures: compat.ClassPrivateStaticMethod,
		},
	})
}

func TestLowerPrivateClassStaticAccessorOrder(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					static bar = Foo.#foo
					static get #foo() { return 123 } // This must be set before "bar" is initialized
				}
				console.log(Foo.bar === 123)

				class FooThis {
					static bar = this.#foo
					static get #foo() { return 123 } // This must be set before "bar" is initialized
				}
				console.log(FooThis.bar === 123)
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:                  config.ModePassThrough,
			AbsOutputFile:         "/out.js",
			UnsupportedJSFeatures: compat.ClassPrivateStaticAccessor,
		},
	})
}

func TestLowerPrivateClassBrandCheckUnsupported(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#foo
					#bar
					baz() {
						return [
							this.#foo,
							this.#bar,
							#foo in this,
						]
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:                  config.ModePassThrough,
			AbsOutputFile:         "/out.js",
			UnsupportedJSFeatures: compat.ClassPrivateBrandCheck,
		},
	})
}

func TestLowerPrivateClassBrandCheckSupported(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class Foo {
					#foo
					#bar
					baz() {
						return [
							this.#foo,
							this.#bar,
							#foo in this,
						]
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModePassThrough,
			AbsOutputFile: "/out.js",
		},
	})
}

func TestLowerTemplateObject(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				x = () => [
					tag` + "`x`" + `,
					tag` + "`\\xFF`" + `,
					tag` + "`\\x`" + `,
					tag` + "`\\u`" + `,
				]
				y = () => [
					tag` + "`x${y}z`" + `,
					tag` + "`\\xFF${y}z`" + `,
					tag` + "`x${y}\\z`" + `,
					tag` + "`x${y}\\u`" + `,
				]
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:                  config.ModePassThrough,
			AbsOutputFile:         "/out.js",
			UnsupportedJSFeatures: compat.TemplateLiteral,
		},
	})
}

// See https://github.com/evanw/esbuild/issues/1424 for more information
func TestLowerPrivateClassFieldStaticIssue1424(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class T {
					#a() { return 'a'; }
					#b() { return 'b'; }
					static c;
					d() { console.log(this.#a()); }
				}
				new T().d();
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:          config.ModeBundle,
			AbsOutputFile: "/out.js",
		},
	})
}

// See https://github.com/evanw/esbuild/issues/1493 for more information
func TestLowerNullishCoalescingAssignmentIssue1493(t *testing.T) {
	lower_suite.expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				class A {
					#a;
					f() {
						this.#a ??= 1;
					}
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		options: config.Options{
			Mode:                  config.ModeBundle,
			AbsOutputFile:         "/out.js",
			UnsupportedJSFeatures: compat.LogicalAssignment,
		},
	})
}
