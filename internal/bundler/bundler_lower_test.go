// This file contains tests for "lowering" syntax, which means converting it to
// older JavaScript. For example, "a ** b" becomes a call to "Math.pow(a, b)"
// when lowered. Which syntax is lowered is determined by the language target.

package bundler

import (
	"testing"

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
			Target:     parser.ES2018,
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
			Target:     parser.ES2017,
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
			Target:     parser.ES2015,
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
